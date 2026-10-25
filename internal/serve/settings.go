package serve

// settings.go 是 GUI 路线第 6 片「设置页」的后端。它是整个仓库里唯一会
// 写用户配置文件的 HTTP 路径，所以每一条限制都刻意写在这里，而不是散在
// 前端或 config 包里：
//
//   - 只写 layers[0]（当前生效的最高优先层），绝不把合并结果整体落盘；
//   - 写之前先备份原文，路径回给前端显示；
//   - 写之前先把新内容当「临时最高层」合并 + Validate 一遍，失败回 400
//     且目标文件一个字节没动（config.WriteAgentSection 负责）；
//   - 写完先替换 s.write.Cfg（展示用）。A2 起再分级：只碰 mode 且没有
//     轮次在跑时，把同一份配置热换进 RunCfg 与 Runner，响应
//     reloaded=true；碰到 display_name / role / provider / model / effort /
//     prompt / permissions、或新增 / 删除成员时，仍然要下次重启 serve 才
//     真正生效，响应 reloaded=false 并在 restart_reasons 里点名原因
//     （display_name / role 会进身份指纹，分级表与热换实现在
//     settings_reload.go）。
//
// providers 与 teams 在本片只读：provider 的 command 指向登录过的 CLI，
// 改错等于把成员换成另一个账户或另一个程序，风险比改一个模型名高一个
// 数量级；teams 的改动会牵动summary 与 round 的成员校验，留给以后的片。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"

	"council/internal/config"
)

// maxSettingsBody 是保存 agent 请求体的上限。表单字段都很短（id、模型名、
// 显示名），16 KiB 已经足够容纳一段较长的显示名或 effort；超过它的请求体
// 不是一个 agent 定义，没必要读完再拒。
const maxSettingsBody = 16 << 10

// settingsReloadNote 是「必须重启」时 note 的后半句。它说明两件事：
// 配置已经写进磁盘；正在跑的 Runner 没有被热换。
// 成功热换的 note 是 settingsHotReloadNote，两句话不能混用。
const settingsReloadNote = "新配置在下次重启 council serve 后生效：Runner 不会重建，当前进程与正在跑的议会仍用启动时的那份配置"

// settingsLayerJSON 是 GET /api/settings 里的一层配置。
//
// Exists 为 false 表示这一层文件不存在：它仍然出现在列表里（用户需要知道
// 「这一层本该在哪」），但不能写；write_target 永远指向第一层。
type settingsLayerJSON struct {
	Path    string `json:"path"`
	Project bool   `json:"project"`
	Exists  bool   `json:"exists"`
}

// settingsProviderJSON 是只读的 provider 展示。本片不写 providers：它只
// 用来给表单里的 provider 下拉提供合法取值，以及让用户核对「这个 id 到底
// 指向哪条命令」。
type settingsProviderJSON struct {
	ID      string `json:"id"`
	Command string `json:"command"`
}

// settingsAgentJSON 是设置页里一行 agent。字段名用 permissions（复数），
// 与配置文件、保存请求体一致；/api/options 用 permission（单数），那是
// 工作台自己的形状，两处不要混。
type settingsAgentJSON struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	Permissions string `json:"permissions"`
	Mode        string `json:"mode"`
	// Source 与 /api/options 同一个词表：config（配置文件里）或 claude
	// （从 .claude/agents 只读导入）。claude 来源的成员在本片不能被编辑
	// 或删除：写进配置文件会让它静默盖住 .claude 里的定义，删也删不掉
	// 那边的事实源。
	Source string `json:"source"`
	// Runnable 与 /api/options 同义：这个 id 在 Runner 当前的 RunCfg 里
	// 存在吗。设置页新增、但还没重启的成员是 false——保存它的任何字段
	// 都仍然需要重启，预览不能把它说成热更。
	Runnable bool `json:"runnable"`
	// PendingRestartFields 列出「展示配置里已经改了、RunCfg 还没换」的
	// 必须重启字段（例如 ["model"]）。保存前预览把这一份算进去，避免
	// 上一次待重启的改动被预览漏掉。
	PendingRestartFields []string `json:"pending_restart_fields,omitempty"`
}

// settingsTeamJSON 是只读的 team 展示。
type settingsTeamJSON struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Members     []string `json:"members"`
}

// settingsResponse 是 GET /api/settings 的完整响应。
//
// providers 是契约之外新增的只读字段：表单需要一个合法的 provider 列表，
// 而本片明确不提供 providers 写接口——把已有值列出来不增加写风险。
type settingsResponse struct {
	Layers       []settingsLayerJSON    `json:"layers"`
	WriteTarget  string                 `json:"write_target"`
	Agents       []settingsAgentJSON    `json:"agents"`
	Teams        []settingsTeamJSON     `json:"teams"`
	SummaryAgent string                 `json:"summary_agent"`
	DataDir      string                 `json:"data_dir"`
	ConfigPath   string                 `json:"config_path"`
	Providers    []settingsProviderJSON `json:"providers"`
	// Reload 是 A2 的字段分级表。前端保存前的预览靠它判断「立即生效」
	// 还是「需要重启」，不在 JS 里另抄一份字段名单。
	Reload settingsReloadJSON `json:"reload"`
}

// settingsAgentRequest 是 POST /api/settings/agents 的请求体。
//
// mode 是契约之外的可选字段：契约里的六个字段只够改模型与显示名，而本片
// 的界面表单要能编辑 mode（表格里也展示了它）。不传 mode 时按「保持原值」
// 处理，所以老调用方（严格按六个字段发）的行为不变。
type settingsAgentRequest struct {
	AgentID     string `json:"agent_id"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	Permissions string `json:"permissions"`
	Mode        string `json:"mode"`
}

// settingsWriteResponse 是保存 / 删除成功的响应。
//
// reloaded 回答的是「Runner 已经换成本次写盘的新配置了吗」，而不是
// 「磁盘上的配置变了吗」。A2 之前它恒为 false；现在只有可热更字段
// （当前只有 mode）、无轮次在跑、且 SetConfig 成功时才为 true。
//
// restart_reasons 只在 reloaded=false 且确实有改动时出现：它逐条点名
// 为什么必须重启（例如 "model 变了"），前端结果区原样显示。
type settingsWriteResponse struct {
	BackupPath     string   `json:"backup_path"`
	WriteTarget    string   `json:"write_target"`
	Reloaded       bool     `json:"reloaded"`
	RestartReasons []string `json:"restart_reasons,omitempty"`
	Note           string   `json:"note"`
}

// currentCfg 返回展示用的合并配置。设置页保存之后这份指针会被换成磁盘上
// 的新内容（见 adoptReloadedConfig），所以读它必须经过这里，不能直接读
// s.write.Cfg —— 保存与 /api/options 可能同时在跑。
func (s *Server) currentCfg() *config.Config {
	if s.write == nil {
		return nil
	}
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.write.Cfg
}

// runCfg 返回 Runner 当前真正在用的那一份配置。启动之后它只在两种情况下
// 变化：A2 的可热更字段（mode）热换把它换成磁盘上的新配置；「从模型添加」
// 那条既有热路径也会换它。需要重启的设置页改动不碰它——/api/ask 的校验与
// 执行必须和 Runner 看同一份，否则会出现「设置页刚加的成员校验通过、
// Runner 不认识它」的错位。
func (s *Server) runCfg() *config.Config {
	if s.write == nil {
		return nil
	}
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	if s.write.RunCfg != nil {
		return s.write.RunCfg
	}
	return s.write.Cfg
}

// isClaudeAgent 判断这个 id 是不是从 .claude/agents 导入的成员。
func (s *Server) isClaudeAgent(id string) bool {
	if s.write == nil {
		return false
	}
	_, ok := s.write.ClaudeAgentsMerged[id]
	return ok
}

// adoptReloadedConfig 把刚校验通过的配置换成展示配置。
//
// Claude 导入只在启动时扫过一次，重新 LoadMerged 出来的配置里没有它们；
// 不把启动时导入的那些补回去，保存一次设置就会让工作台里的 Claude 成员
// 凭空消失（直到重启）。合并规则不变：配置里已有的同名 id 优先。
func (s *Server) adoptReloadedConfig(cfg *config.Config) {
	if cfg == nil || s.write == nil {
		return
	}
	for id, ac := range s.write.ClaudeAgents {
		if cfg.Agents == nil {
			cfg.Agents = make(map[string]config.AgentConfig, len(s.write.ClaudeAgents))
		}
		if _, ok := cfg.Agents[id]; ok {
			continue
		}
		cfg.Agents[id] = ac
	}
	s.cfgMu.Lock()
	s.write.Cfg = cfg
	s.cfgMu.Unlock()
}

// settingsLayers 返回本次启动实际合并的配置层。
//
// 生产装配（runWritable）会把 ResolveConfigLayers 的结果放进 WriteDeps；
// 直接构造 WriteDeps 的测试没填 Layers，此时退化成「ConfigPath 这一份」。
// 两者都没有就报错，绝不在这里按 cwd / HOME 重新解析：设置页要写的那份
// 文件必须与本次启动实际读的那份一致，猜一个路径比报错危险得多。
func (s *Server) settingsLayers() ([]config.Layer, error) {
	if s.write == nil {
		return nil, errors.New("只读模式没有设置接口")
	}
	if len(s.write.Layers) > 0 {
		return s.write.Layers, nil
	}
	if s.write.ConfigPath != "" {
		return []config.Layer{{Path: s.write.ConfigPath}}, nil
	}
	return nil, errors.New("设置接口缺少配置层：装配时没有提供 Layers 或 ConfigPath")
}

// handleSettings 返回合并后的配置快照（只读）。它只在 -rw 下可用：只读
// 模式没有完整配置，也没有写目标，返回一份空表会让用户以为配置一片空白。
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有设置接口：用 council serve -rw 启动才能查看与编辑配置")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}
	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := settingsResponse{
		WriteTarget: layers[0].Path,
		Agents:      []settingsAgentJSON{},
		Teams:       []settingsTeamJSON{},
		Providers:   []settingsProviderJSON{},
		ConfigPath:  s.write.ConfigPath,
		DataDir:     s.write.DataDir,
		Reload:      settingsReloadPolicy(),
	}
	for _, layer := range layers {
		exists := false
		if _, statErr := os.Stat(layer.Path); statErr == nil {
			exists = true
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			// 读不了（权限等）时按「存在」处理：它确实在那里，只是这次
			// 看不到详情，与 ResolveConfigLayers 对非 ENOENT 的处置一致。
			exists = true
		}
		resp.Layers = append(resp.Layers, settingsLayerJSON{
			Path:    layer.Path,
			Project: layer.Project,
			Exists:  exists,
		})
	}

	cfg := s.currentCfg()
	if cfg != nil {
		resp.SummaryAgent = cfg.Summary.Agent
		if resp.DataDir == "" {
			resp.DataDir = cfg.DataDir
		}
		// 一次请求只取一次 RunCfg：热换可能正把指针换掉，逐行读会让
		// runnable / pending 字段与 agent 行对不上。
		run := s.runCfg()
		for _, opt := range s.displayAgentOptions(cfg) {
			runnable, pending := settingsPendingRestart(run, cfg, opt.ID)
			resp.Agents = append(resp.Agents, settingsAgentJSON{
				ID:                   opt.ID,
				DisplayName:          opt.DisplayName,
				Provider:             opt.Provider,
				Model:                opt.Model,
				Effort:               opt.Effort,
				Permissions:          opt.Permission,
				Mode:                 opt.Mode,
				Source:               opt.Source,
				Runnable:             runnable,
				PendingRestartFields: pending,
			})
		}

		teamIDs := make([]string, 0, len(cfg.Teams))
		for id := range cfg.Teams {
			teamIDs = append(teamIDs, id)
		}
		sort.Strings(teamIDs)
		for _, id := range teamIDs {
			members := cfg.Teams[id].Members
			if members == nil {
				members = []string{}
			}
			resp.Teams = append(resp.Teams, settingsTeamJSON{
				ID:          id,
				DisplayName: cfg.Teams[id].DisplayName,
				Members:     members,
			})
		}

		providerIDs := make([]string, 0, len(cfg.Providers))
		for id := range cfg.Providers {
			providerIDs = append(providerIDs, id)
		}
		sort.Strings(providerIDs)
		for _, id := range providerIDs {
			resp.Providers = append(resp.Providers, settingsProviderJSON{
				ID:      id,
				Command: cfg.Providers[id].Command,
			})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSettingsAgentSave 新增或修改一个 agent。upsert 语义：id 在展示配置
// 里已存在就是改，不存在就是加（新增 id 要先过 config.NewAgentIDProblem
// 的五道检查）。
//
// 门控顺序与其它写接口逐条对齐：只读 405 → 非 POST 405 → JSON + 同源
// （415 / 403）→ 请求体是单个合法对象（400）→ 字段归一化 → 写盘。
func (s *Server) handleSettingsAgentSave(w http.ResponseWriter, r *http.Request) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	var req settingsAgentRequest
	if !decodeSettingsBody(w, r, &req) {
		return
	}
	// 从读当前配置到写盘、换展示配置，整个过程串行：两次并发保存不该
	// 各自基于一份过期的旧内容算「目标文件里有没有这一段」。
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cur := s.currentCfg()
	if cur == nil {
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法保存 agent")
		return
	}

	id := strings.TrimSpace(req.AgentID)
	if id == "" {
		writeError(w, http.StatusBadRequest, "agent_id 不能为空：新增要给出一个 id，修改要给出现有 id")
		return
	}
	prev, exists := cur.Agents[id]
	if exists {
		if s.isClaudeAgent(id) {
			writeError(w, http.StatusBadRequest, claudeAgentReadOnlyMessage(id))
			return
		}
	} else if problem := config.NewAgentIDProblem(cur, id); problem != "" {
		// 这里同时挡下：id 不合规、保留名、Windows 设备名、与已有成员
		// 点号/下划线/连字符折叠后撞名。新增的 id 会变成工作目录名，
		// 这几道检查必须在写文件之前跑。
		writeError(w, http.StatusBadRequest, problem)
		return
	}

	agent := config.AgentConfig{
		DisplayName: strings.TrimSpace(req.DisplayName),
		Provider:    strings.TrimSpace(req.Provider),
		Model:       strings.TrimSpace(req.Model),
		Effort:      strings.TrimSpace(req.Effort),
		Permissions: config.Permission(strings.ToLower(strings.TrimSpace(req.Permissions))),
		Mode:        config.Mode(strings.ToLower(strings.TrimSpace(req.Mode))),
	}
	// 空 permissions / mode 按「保持原值」处理（新增时取 Load 的缺省值）。
	// 空的 effort 表示「没有 effort」，写文件时会删掉那一行。provider 与
	// model 不在这里补：它们必须由调用方给出，空值交给 config.Validate
	// 报出与手写配置完全一样的那条错误。
	if agent.Permissions == "" {
		if exists && prev.Permissions != "" {
			agent.Permissions = prev.Permissions
		} else {
			agent.Permissions = config.PermissionRead
		}
	}
	if agent.Mode == "" {
		if exists && prev.Mode != "" {
			agent.Mode = prev.Mode
		} else {
			agent.Mode = config.ModeDiscussion
		}
	}
	// fallback 只在「目标文件里还没有这一段」时使用：继承自低优先级层的
	// 成员被改写时，role / prompt 要从当前生效的那一份带过去，否则整条
	// 覆盖会悄悄丢掉它们。
	fallback := config.AgentConfig{}
	if exists {
		fallback = prev
	}

	res, err := config.WriteAgentSection(layers, id, agent, fallback)
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	if !res.Changed {
		// 文件逐字节没变：没有可热的改动，也没有可回滚的东西。
		// reloaded 仍是 false——这次请求根本没有换过 Runner。
		writeJSON(w, http.StatusOK, settingsWriteResponse{
			BackupPath:  res.BackupPath,
			WriteTarget: res.WriteTarget,
			Note:        "配置没有变化：文件未改动，也没有生成备份。" + settingsReloadNote,
		})
		return
	}

	// RunCfg 必须在 adopt 之前取：adopt 只换展示配置，但这里拍下的是
	// 「Runner 此刻真正在用的那一份」，分级判定拿它当基准。
	oldRun := s.runCfg()
	s.adoptReloadedConfig(res.Config)

	reasons := settingsRestartReasons(oldRun, res.Config, id)
	if len(reasons) == 0 && !settingsHotChanged(oldRun, res.Config, id) {
		// 文件字节变了，但合并后的运行配置与 RunCfg 逐字段相同（例如给
		// 本来就用默认 mode 的段补上一行显式 mode）。没有可热的改动，
		// 也不该假装热加载过；settingsHotFieldNames 为空时所有真实改动
		// 都已经在 reasons 里，只剩这一种纯格式变化会走到这里。
		writeJSON(w, http.StatusOK, settingsWriteResponse{
			BackupPath:  res.BackupPath,
			WriteTarget: res.WriteTarget,
			Note:        settingsRuntimeUnchangedNote,
		})
		return
	}
	if len(reasons) == 0 {
		// 本次只碰了可热更字段。热换本身还要过「没有轮次在跑」这道闸；
		// 有 job 在跑时退化成需要重启，绝不打断进行中的议会。
		if reloadErr := s.settingsHotReload(res.Config); reloadErr != nil {
			switch {
			case errors.Is(reloadErr, errSettingsJobsRunning),
				errors.Is(reloadErr, errSettingsNoRunner):
				reasons = append(reasons, reloadErr.Error())
			default:
				// 文件与展示配置都已经换了，回滚它们会制造「磁盘上写着 A、
				// 界面显示 B」的更坏状态。RunCfg 已在 settingsHotReload
				// 里回滚，这里只把事实说清楚。
				writeError(w, http.StatusInternalServerError, fmt.Sprintf(
					"配置已写入 %s（备份：%s），但热加载失败，已回滚到旧的 Runner 配置；重启 council serve 后新配置可生效：%v",
					res.WriteTarget, res.BackupPath, reloadErr))
				return
			}
		}
	}

	resp := settingsWriteResponse{
		BackupPath:  res.BackupPath,
		WriteTarget: res.WriteTarget,
	}
	if len(reasons) == 0 {
		resp.Reloaded = true
		resp.Note = settingsHotReloadNote
	} else {
		resp.RestartReasons = reasons
		resp.Note = settingsRestartNote(reasons)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSettingsAgentDelete 从写目标里删掉一个 agent 的段。
func (s *Server) handleSettingsAgentDelete(w http.ResponseWriter, r *http.Request, id string) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	// 与保存共用同一把锁：删除也要「读当前配置 -> 写盘 -> 换展示配置」
	// 串行，否则并发的新增可能落进一份马上被删除覆盖的旧内容上。
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cur := s.currentCfg()
	if cur == nil {
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法删除 agent")
		return
	}
	if _, ok := cur.Agents[id]; !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent 不存在: %s", id))
		return
	}
	if s.isClaudeAgent(id) {
		writeError(w, http.StatusBadRequest, claudeAgentReadOnlyMessage(id))
		return
	}

	res, err := config.RemoveAgentSection(layers, id)
	if errors.Is(err, config.ErrAgentSectionMissing) {
		// 成员存在、但不在写目标里：它继承自低优先级层。删目标层删不掉它，
		// 只能去定义它的那一份删。404 在这里是错的（用户看到的是「有」），
		// 409 才说得清「状态冲突」。
		where := config.AgentDefinedIn(layers, id)
		hint := "它定义在低优先级的配置层里，本接口只写第一层"
		if len(where) > 0 {
			hint = "它定义在 " + strings.Join(where, "、") + "，本接口只写 " + layers[0].Path
		}
		writeError(w, http.StatusConflict,
			fmt.Sprintf("不能删除 %s：%s。请直接改那份文件，删掉对应的 [agents.%s] 段", id, hint, id))
		return
	}
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	if !res.Changed {
		writeJSON(w, http.StatusOK, settingsWriteResponse{
			BackupPath:  res.BackupPath,
			WriteTarget: res.WriteTarget,
			Note:        "配置没有变化：文件未改动，也没有生成备份。" + settingsReloadNote,
		})
		return
	}
	s.adoptReloadedConfig(res.Config)

	// A2：新增 / 删除成员一律不热换。删除尤其不能热换——已经发出的会话、
	// 汇总配置和 team 引用都按旧名单建立，换掉 Runner 的成员表不会让那些
	// 事实一起消失，只会让下一次 ask 的报错变成一句无法解释的「未知成员」。
	reasons := []string{"删除了成员 " + id}
	note := settingsRestartNote(reasons)
	if res.Config != nil {
		// 目标层删掉了，但低优先级层可能也有同名定义：合并之后这个成员
		// 仍然可见。不说这一句，用户会以为删除没生效。
		if _, still := res.Config.Agents[id]; still {
			note = "已从 " + res.WriteTarget + " 删除这一段，但低优先级配置层里仍有同名定义，合并后它仍然可见。" + settingsRestartNote(reasons)
		}
	}
	writeJSON(w, http.StatusOK, settingsWriteResponse{
		BackupPath:     res.BackupPath,
		WriteTarget:    res.WriteTarget,
		RestartReasons: reasons,
		Note:           note,
	})
}

// handleSettingsAgentPath 处理 /api/settings/agents/{id}/delete。
// 只认恰好两段，多一段少一段都是 404；路径本身不合法时也回 JSON。
func (s *Server) handleSettingsAgentPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/settings/agents/")
	if rest == "" || strings.HasSuffix(rest, "/") {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	segments := strings.Split(rest, "/")
	if len(segments) != 2 || segments[1] != "delete" {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	s.handleSettingsAgentDelete(w, r, segments[0])
}

// settingsWriteGate 是设置页两个写端点共用的前四道门，顺序与 rename /
// delete / members 逐条一致：只读 405 → 非 POST 405 → JSON（415）→
// 同源（403）。返回 false 时错误响应已经写好。
func (s *Server) settingsWriteGate(w http.ResponseWriter, r *http.Request) bool {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有设置接口：用 council serve -rw 启动才能编辑配置")
		return false
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "只支持 POST")
		return false
	}
	if err := checkWriteRequest(r); err != nil {
		status := http.StatusForbidden
		var guardErr *writeGuardError
		if errors.As(err, &guardErr) {
			status = guardErr.status
		}
		writeError(w, status, err.Error())
		return false
	}
	return true
}

// decodeSettingsBody 读一个 JSON 对象并拒绝多余内容。成功返回 true；
// 失败时已经写好 400 响应（与 rename / member 的请求体规则同一套）。
func decodeSettingsBody(w http.ResponseWriter, r *http.Request, out any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSettingsBody))
	if err := dec.Decode(out); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("请求体超过 %d 字节上限", maxSettingsBody))
			return false
		}
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return false
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求体只能有一个 JSON 对象")
		return false
	}
	return true
}

// writeSettingsError 把 config 包的写盘错误翻译成 HTTP 状态：
//   - 新内容过不了校验 → 400（用户输入的问题，文件未改动）；
//   - 写目标不存在 / 目标文件里没有这一段 → 409（状态冲突，不是语法错误）；
//   - 其余（读文件、备份、Rename 失败）→ 500。
func (s *Server) writeSettingsError(w http.ResponseWriter, err error) {
	var invalid *config.InvalidConfigError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest,
			"新配置没有通过校验，文件未改动："+invalid.Error())
	case errors.Is(err, config.ErrWriteTargetMissing),
		errors.Is(err, config.ErrAgentSectionMissing):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "写入配置失败: "+err.Error())
	}
}

// claudeAgentReadOnlyMessage 说明 claude 来源的成员为什么不能在这里改。
func claudeAgentReadOnlyMessage(id string) string {
	return fmt.Sprintf(
		"agent %s 来自 .claude/agents，是只读外部来源：改成配置文件里的同 id 定义会静默盖住 Claude 那边的事实源，删除也无法生效。请直接改 .claude/agents 下的文件，或换一个 id 新增一个配置成员", id)
}
