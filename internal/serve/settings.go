package serve

// settings.go 是 GUI 路线第 6 片「设置页」的后端。它是整个仓库里唯一会
// 写用户配置文件的 HTTP 路径，所以每一条限制都刻意写在这里，而不是散在
// 前端或 config 包里：
//
//   - 只写 layers[0]（当前生效的最高优先层），绝不把合并结果整体落盘；
//   - 写之前先备份原文，路径回给前端显示；
//   - 写之前先把新内容当「临时最高层」合并 + Validate 一遍，失败回 400
//     且目标文件一个字节没动（config.WriteAgentSection 负责）；
//   - 写完先替换 s.write.Cfg（展示用），再分级热换（settings_reload.go）：
//     mode 与七个身份字段都能当场生效，响应 reloaded=true。身份字段
//     （display_name / role / provider / model / effort / prompt /
//     permissions）的全部代价是「重置该成员的 provider 会话」——它们进
//     身份指纹，不重置的话旧会话下一轮必然撞漂移。响应里的
//     sessions_reset 就是删掉的行数，note 明写「上下文不再续接」。
//     新增 / 删除成员、以及「其它成员在磁盘上变了」仍然要下次重启 serve
//     才真正生效，响应 reloaded=false 并在 restart_reasons 里点名原因。
//     有轮次在跑时：身份字段的改动在**写盘之前**就回 409 拒绝，mode 之类
//     不碰会话的改动照旧写盘但不热换（reloaded=false + 原因）。
//
// providers 与 teams 在本片只读：provider 的 command 指向登录过的 CLI，
// 改错等于把成员换成另一个账户或另一个程序，风险比改一个模型名高一个
// 数量级；teams 的改动会牵动summary 与 round 的成员校验，留给以后的片。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
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

// settingsProviderJSON 是设置页里一行 provider。
//
// 从 0.4.0 起 provider 可以在设置页里增删改：provider 只是一个 id 加一条
// 可执行文件路径，没有别的字段，写进配置就是 [providers.<id>] command。
type settingsProviderJSON struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	// Args / Env 是 provider 级的额外参数与环境变量覆盖：args 追加在适配器
	// 生成的 argv 之后，env 覆盖进子进程环境。两者都进身份指纹。
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
	// Models 是这个 provider 的模型候选（用户填的），成员表单拿它做下拉。
	Models []string `json:"models,omitempty"`
	// Exists 表示 command 指向的东西在这台机器上找得到（绝对路径 stat，
	// 否则走 PATH 查找）。它只是给界面画一个状态点，不参与任何校验：
	// 命令放在别处、或由别的启动脚本注入，都是合法的配置。
	Exists bool `json:"exists"`
	// Writable 表示这一条写在可写目标里（用户自己加的）；false 表示它来自
	// 低优先级层，设置页改不了它，只能去那份文件改。
	Writable bool `json:"writable"`
	// Agents 是引用这个 provider 的成员 id（排序后）。删除前要把它们列给
	// 用户看：删掉被引用的 provider 会让配置校验失败。
	Agents []string `json:"agents,omitempty"`
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
	// Language 是配置里的原值（zh / en / auto），供设置页的下拉显示；
	// LanguageEffective 是本次进程实际生效的语言，用于「实际生效：…」；
	// LanguageFlag 只在启动时显式给了 -lang zh/en 时非空：那种情况下
	// 配置里的选择不会立刻改变界面，界面要把这个事实说出来。
	// LanguageEnv 同理，对应环境变量 COUNCIL_LANG：它压在配置之上、-lang
	// 之下；不说明的话用户会以为下拉框坏了。
	Language          string `json:"language"`
	LanguageEffective string `json:"language_effective"`
	LanguageFlag      string `json:"language_flag,omitempty"`
	LanguageEnv       string `json:"language_env,omitempty"`
	// LocalAgent 是本机 agent 模块的层级（off / observe / coordinate），
	// 供设置页第 6 类那一行下拉显示。它是配置里的顶层键、不进身份指纹。
	//
	// 注意它只是**配置值**，不是本进程的行为：serve 本身不做任何进程扫描，
	// 也不申请租约（council who / council claim 是各自读配置的独立进程）。
	LocalAgent string `json:"local_agent"`
	// LocalAgentSupported 是这一版二进制接受的层级全集，前端据此画下拉。
	// 放在响应里而不是 JS 里另抄一份：抄了就会与 config 的枚举走失，
	// 而配置拒绝未知取值（加一档要改代码，不该只改前端）。
	LocalAgentSupported []string `json:"local_agent_supported"`
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

// settingsProviderRequest 是 POST /api/settings/providers 的请求体。
// provider 只有一个字段：command 是启动成员进程时使用的可执行文件。
type settingsProviderRequest struct {
	ProviderID string   `json:"provider_id"`
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Env        []string `json:"env"`
	Models     []string `json:"models"`
}

// settingsWriteResponse 是保存 / 删除成功的响应。
//
// reloaded 回答的是「Runner 已经换成本次写盘的新配置了吗」，而不是
// 「磁盘上的配置变了吗」。A2 之前它恒为 false；现在只要本次改动落在可热更
// 字段（mode 与七个身份字段）、无轮次在跑、且 SetConfig 成功就为 true。
//
// sessions_reset 是这次热换删掉的 provider_sessions 行数：改 mode 恒为 0，
// 改身份字段时是该成员在**全部会话**里的记录数（0 表示它还没开过口）。
// 只要它大于 0，用户就必须知道「这个人的上下文不再续接」——结果区与 note
// 都要说这句话。
//
// restart_reasons 只在 reloaded=false 且确实有改动时出现：它逐条点名
// 为什么必须重启（例如 "新增成员 p9"），前端结果区原样显示。
type settingsWriteResponse struct {
	BackupPath     string   `json:"backup_path"`
	WriteTarget    string   `json:"write_target"`
	Reloaded       bool     `json:"reloaded"`
	SessionsReset  int      `json:"sessions_reset"`
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
// 变化：A2/A3 的可热更字段热换把它换成磁盘上的新配置（mode 与七个身份
// 字段；身份字段还会重置该成员的 provider 会话）；「从模型添加」那条既有
// 热路径也会换它。需要重启的设置页改动不碰它——/api/ask 的校验与执行必须
// 和 Runner 看同一份，否则会出现「设置页刚加的成员校验通过、Runner 不
// 认识它」的错位。
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
		WriteTarget:       layers[0].Path,
		Agents:            []settingsAgentJSON{},
		Teams:             []settingsTeamJSON{},
		Providers:         []settingsProviderJSON{},
		ConfigPath:        s.write.ConfigPath,
		DataDir:           s.write.DataDir,
		Reload:            settingsReloadPolicy(),
		Language:          config.LanguageAuto,
		LanguageEffective: string(s.Language()),
		LanguageFlag:      s.LanguageFlag(),
		LanguageEnv:       s.LanguageEnv(),
		LocalAgent:        config.LocalAgentOff,
		LocalAgentSupported: []string{
			config.LocalAgentOff, config.LocalAgentObserve, config.LocalAgentCoordinate,
		},
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
		resp.Language = config.LanguageValue(cfg.Language)
		resp.LocalAgent = config.LocalAgentValue(cfg.LocalAgent)
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
		writableProviders := s.writableProviderIDs()
		for _, id := range providerIDs {
			provider := cfg.Providers[id]
			resp.Providers = append(resp.Providers, settingsProviderJSON{
				ID:       id,
				Command:  provider.Command,
				Args:     provider.Args,
				Env:      provider.Env,
				Models:   provider.Models,
				Exists:   commandResolves(provider.Command),
				Writable: writableProviders[id],
				Agents:   agentsUsingProvider(cfg.Agents, id),
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

	// A3：有轮次在跑时，身份字段的改动在写盘之前就拒绝。
	//
	// 这次改动必须与「重置该成员的 provider 会话」同时生效，而重置只能在
	// 没有轮次时做。若先照旧写盘、再退化成「需要重启」，用户会得到一份
	// 「磁盘上是新 model、Runner 还是旧的、会话还在」的中间态，而且下一次
	// 保存之前谁也看不出它没生效。拒绝时一个字节都不写，用户等这一轮跑完
	// 再点一次保存即可。
	//
	// 这道闸放在字段校验之前：拒绝时什么都没写，用户重新保存时会拿到真正
	// 的校验错误；反过来先写一遍再校验，等于把文件动了才拒绝——那正是
	// 这一闸要避免的事。
	//
	// 只拦身份字段。mode 这类不碰会话、不进指纹的字段照旧写盘后降级为
	// 「reloaded=false + 有轮次正在运行」，那是既有行为，也不会留下危险状态。
	//
	// 这道闸与写盘之间不抢 reloadMu：恰在这几毫秒里有新的一轮进表时，写盘
	// 会照旧发生，随后 settingsHotReload 里的 runningCount 闸会发现它并退化
	// 成 reloaded=false——会话一行都不删，落盘的新配置等重启生效。那正是这次
	// 改动之前的状态，不会更糟；为了消掉这个窗口去阻塞所有 /api/ask 不划算。
	if s.runningJobs() > 0 && settingsSaveResetsSessions(s.runCfg(), id, settingsAgentAfterSave(agent, fallback)) {
		writeError(w, http.StatusConflict,
			"有轮次正在运行：这次改动会重置成员 "+id+
				" 的 provider 会话，必须在没有轮次时保存。等它结束后再保存，或重启 council serve。本次没有写入任何配置文件。")
		return
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
	resetSessions := false
	sessionsReset := 0
	if len(reasons) == 0 {
		// 本次改动全部落在可热更字段（mode 或身份字段）。热换本身还要过
		// 「没有轮次在跑」这道闸；有 job 在跑时退化成需要重启，绝不打断
		// 进行中的议会，也绝不删会话。ctx 去掉请求取消：保存已经用户确认，
		// 浏览器中途断开不该把「删会话」这一步留在半路。
		resetSessions = settingsIdentityChanged(oldRun, res.Config, id)
		n, reloadErr := s.settingsHotReload(context.WithoutCancel(r.Context()), res.Config, id, resetSessions)
		if reloadErr != nil {
			switch {
			case errors.Is(reloadErr, errSettingsJobsRunning),
				errors.Is(reloadErr, errSettingsNoRunner):
				reasons = append(reasons, reloadErr.Error())
			case errors.Is(reloadErr, errSettingsSessionReset):
				// 会话没删成，RunCfg 也没换（重置排在 SetConfig 之前）。
				// 不能套用下面那句「已回滚」——这里根本没换过。
				writeError(w, http.StatusInternalServerError, fmt.Sprintf(
					"配置已写入 %s（备份：%s），但没有热加载：%v。旧的 Runner 配置仍然生效，重启 council serve 后新配置可生效。",
					res.WriteTarget, res.BackupPath, reloadErr))
				return
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
		sessionsReset = n
	}

	resp := settingsWriteResponse{
		BackupPath:  res.BackupPath,
		WriteTarget: res.WriteTarget,
	}
	if len(reasons) == 0 {
		resp.Reloaded = true
		resp.SessionsReset = sessionsReset
		if resetSessions {
			resp.Note = settingsHotReloadResetNote(sessionsReset)
		} else {
			resp.Note = settingsHotReloadNote
		}
	} else {
		resp.RestartReasons = reasons
		resp.Note = settingsRestartNote(reasons)
	}
	writeJSON(w, http.StatusOK, resp)
}

// settingsLanguageRequest 是 POST /api/settings/language 的请求体。
// 只接受 zh / en / auto（config 包的 Validate 兜底，非法值会带着「文件
// 未改动」的 400 回来）。
type settingsLanguageRequest struct {
	Language string `json:"language"`
}

// handleSettingsLanguageSave 把界面语言写进配置，并立即换掉本次进程的
// 文案表。language 不是 agent 字段、不进身份指纹，所以它不需要重启，
// 也不走 A2 的 agent 字段分级表：那张表只比较 agents 段。
//
// 写盘仍然走 config.WriteLanguage（= WriteAgentSection 的同一套临时文件 +
// 合并校验 + 备份 + Rename）：只写 layers[0]，其它段一个字节不动。
func (s *Server) handleSettingsLanguageSave(w http.ResponseWriter, r *http.Request) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	var req settingsLanguageRequest
	if !decodeSettingsBody(w, r, &req) {
		return
	}
	// 空值不会经过下拉到达这里；把它当成「把配置重置成 auto」太隐蔽了，
	// 明确回 400，让调用方自己写 auto。
	if strings.TrimSpace(req.Language) == "" {
		writeError(w, http.StatusBadRequest, "language 不能为空（可选 zh / en / auto）")
		return
	}
	// 与 agent 保存共用一把锁：两次并发写盘不该各自基于一份旧内容算目标。
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.currentCfg() == nil {
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法保存语言")
		return
	}

	res, err := config.WriteLanguage(layers, req.Language)
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	if !res.Changed {
		writeJSON(w, http.StatusOK, settingsWriteResponse{
			BackupPath:  res.BackupPath,
			WriteTarget: res.WriteTarget,
			Note:        "语言没有变化：文件未改动，也没有生成备份。",
		})
		return
	}

	// 展示配置换成磁盘上的新内容；Runner 不碰。Language 只影响 /api/i18n，
	// 换掉 Cfg 只是为了让设置页与下一轮 agent 保存看到同一份事实。
	s.adoptReloadedConfig(res.Config)
	// 启动时显式的 -lang 仍然优先：保存配置不能悄悄推翻命令行。
	s.SetLanguage(s.LanguageFlag(), res.Config.Language)
	// 起作用的若不是刚写进去的那一种，成功提示不能说「已切换」：
	// -lang 与 COUNCIL_LANG 都压在配置之上，用户必须看到真正的原因。
	note := "语言已保存并立即生效：界面文案已切换，无需重启 council serve"
	if flag := s.LanguageFlag(); flag != "" {
		note = "语言已保存；但启动参数 -lang " + flag +
			" 仍优先，界面文案要重启 serve 并去掉该参数后才会切换。"
	} else if env := s.LanguageEnv(); env != "" {
		note = "语言已保存；但环境变量 COUNCIL_LANG=" + env +
			" 仍优先，界面文案要清掉它并重启 serve 后才会切换。"
	}
	writeJSON(w, http.StatusOK, settingsWriteResponse{
		BackupPath:  res.BackupPath,
		WriteTarget: res.WriteTarget,
		Reloaded:    true,
		Note:        note,
	})
}

// settingsLocalAgentRequest 是 POST /api/settings/local-agent 的请求体。
// 只接受 off / observe / coordinate（config 包的 Validate 兜底，非法值会
// 带着「文件未改动」的 400 回来）。
type settingsLocalAgentRequest struct {
	Level string `json:"level"`
}

// handleSettingsLocalAgentSave 把本机 agent 模块的层级写进配置。
//
// 它与语言那条同构：单个顶层键、不进身份指纹、不碰任何成员的 provider 会话，
// 所以不需要重启本进程、也不走 A2 的 agent 字段分级表。
//
// 但「立即生效」的边界必须说清（note 里写明）：council who / council claim /
// council release 每次运行都读一次配置，下一次调用就按新层级走；而正在跑的
// council mcp（宿主拉起的 stdio 进程）在启动时读了一次 Cfg，要重启才看得到。
// serve 自己不做任何扫描，所以本进程没有任何东西需要重新装配。
func (s *Server) handleSettingsLocalAgentSave(w http.ResponseWriter, r *http.Request) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	var req settingsLocalAgentRequest
	if !decodeSettingsBody(w, r, &req) {
		return
	}
	// 空值不会经过下拉到达这里；当成「把配置重置成 off」太隐蔽了，明确回
	// 400，让调用方自己写 off（与语言那条同一条纪律）。
	if strings.TrimSpace(req.Level) == "" {
		writeError(w, http.StatusBadRequest,
			"level 不能为空（可选 off / observe / coordinate）")
		return
	}
	// 与 agent / 语言保存共用一把锁：两次并发写盘不该各自基于一份旧内容算目标。
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.currentCfg() == nil {
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法保存本机 agent 层级")
		return
	}

	res, err := config.WriteLocalAgent(layers, req.Level)
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	if !res.Changed {
		writeJSON(w, http.StatusOK, settingsWriteResponse{
			BackupPath:  res.BackupPath,
			WriteTarget: res.WriteTarget,
			Note:        "本机 agent 层级没有变化：文件未改动，也没有生成备份。",
		})
		return
	}

	// 展示配置换成磁盘上的新内容；Runner 不碰（它不读这个键）。
	s.adoptReloadedConfig(res.Config)
	writeJSON(w, http.StatusOK, settingsWriteResponse{
		BackupPath:  res.BackupPath,
		WriteTarget: res.WriteTarget,
		Reloaded:    true,
		Note: "本机 agent 层级已保存（" + config.LocalAgentValue(req.Level) +
			"）：council who / council claim / council release 下一次运行就按它走；" +
			"正在跑的 council mcp 与 TUI 在启动时读过一次配置，要重启才会看到新层级。" +
			"本进程不做任何进程扫描，也不申请租约。",
	})
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
	s.handleSettingsDeletePath(w, r, "/api/settings/agents/", s.handleSettingsAgentDelete)
}

// handleSettingsTeamPath 处理 /api/settings/teams/{id}/delete。
func (s *Server) handleSettingsTeamPath(w http.ResponseWriter, r *http.Request) {
	s.handleSettingsDeletePath(w, r, "/api/settings/teams/", s.handleSettingsTeamDelete)
}

// handleSettingsProviderPath 处理 /api/settings/providers/{id}/delete。
func (s *Server) handleSettingsProviderPath(w http.ResponseWriter, r *http.Request) {
	s.handleSettingsDeletePath(w, r, "/api/settings/providers/", s.handleSettingsProviderDelete)
}

// handleSettingsDeletePath 是两条 /delete 路径共用的形状检查：恰好
// {prefix}{id}/delete，多一段少一段都是 404。
func (s *Server) handleSettingsDeletePath(
	w http.ResponseWriter,
	r *http.Request,
	prefix string,
	deleteFn func(http.ResponseWriter, *http.Request, string),
) {
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == "" || strings.HasSuffix(rest, "/") {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	segments := strings.Split(rest, "/")
	if len(segments) != 2 || segments[1] != "delete" {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	deleteFn(w, r, segments[0])
}

// handleSettingsProviderSave 新增或修改一个 provider。
//
// upsert 语义与 agent 一致：id 已存在就是改 command，不存在就是新增一段
// [providers.<id>]。provider 只有一个字段，所以没有「待重启字段」那套分级，
// 一律按「重启后生效」说清楚——cmd.Path 是 BuildTurn 时从 RunCfg 取的，
// 已经跑着的 Runner 不会因为文件变了就换。
func (s *Server) handleSettingsProviderSave(w http.ResponseWriter, r *http.Request) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	var req settingsProviderRequest
	if !decodeSettingsBody(w, r, &req) {
		return
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cur := s.currentCfg()
	if cur == nil {
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法保存 provider")
		return
	}

	id := strings.TrimSpace(req.ProviderID)
	if id == "" {
		writeError(w, http.StatusBadRequest, "provider_id 不能为空：新增要给出一个 id，修改要给出现有 id")
		return
	}
	if problem := config.ProviderIDProblem(id); problem != "" {
		writeError(w, http.StatusBadRequest, problem)
		return
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		writeError(w, http.StatusBadRequest, "command 不能为空：它是启动成员进程时使用的可执行文件路径")
		return
	}

	models, modelErr := cleanProviderModels(req.Models)
	if modelErr != "" {
		writeError(w, http.StatusBadRequest, modelErr)
		return
	}
	args, argErr := cleanProviderArgs(req.Args)
	if argErr != "" {
		writeError(w, http.StatusBadRequest, argErr)
		return
	}
	env, envErr := cleanProviderEnv(req.Env)
	if envErr != "" {
		writeError(w, http.StatusBadRequest, envErr)
		return
	}

	res, err := config.WriteProviderSection(layers, id, config.ProviderConfig{
		Command: command,
		Args:    args,
		Env:     env,
		Models:  models,
	})
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	if !res.Changed {
		writeJSON(w, http.StatusOK, settingsWriteResponse{
			BackupPath:  res.BackupPath,
			WriteTarget: res.WriteTarget,
			Note:        "配置没有变化：文件未改动，也没有生成备份。",
		})
		return
	}
	s.adoptReloadedConfig(res.Config)
	writeJSON(w, http.StatusOK, settingsWriteResponse{
		BackupPath:     res.BackupPath,
		WriteTarget:    res.WriteTarget,
		RestartReasons: []string{"provider " + id + " 的命令变了"},
		Note: "已写入 provider " + id + "。" + settingsReloadNote +
			"：正在跑的议会仍用启动时那份配置里的命令；已经建立过会话的成员，改完命令后建议对它敲 /session rebuild。",
	})
}

// cleanProviderArgs 去掉空行（界面上多敲一行回车不该变成空参数），并对
// 数量与长度做与人话一致的检查。真正的最终判定在 config.Validate，这里
// 只是把最常见的输入错误换成本地化的说明。
func cleanProviderArgs(raw []string) ([]string, string) {
	out := make([]string, 0, len(raw))
	for _, arg := range raw {
		arg = strings.TrimRight(arg, "\r")
		if strings.TrimSpace(arg) == "" {
			continue
		}
		out = append(out, arg)
	}
	if len(out) > config.MaxProviderArgs {
		return nil, fmt.Sprintf("参数最多 %d 条，现在有 %d 条", config.MaxProviderArgs, len(out))
	}
	for _, arg := range out {
		if len(arg) > config.MaxProviderEntryBytes {
			return nil, fmt.Sprintf("单个参数超过 %d 字节", config.MaxProviderEntryBytes)
		}
	}
	return out, ""
}

// cleanProviderModels 去掉空行、去重，保持用户填写的顺序（模型顺序有意义：
// 第一条常是默认模型）。数量与长度上限与 args / env 同一套。
func cleanProviderModels(raw []string) ([]string, string) {
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, model := range raw {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if seen[model] {
			continue
		}
		seen[model] = true
		if len(model) > config.MaxProviderEntryBytes {
			return nil, fmt.Sprintf("模型名 %q 超过 %d 字节", model, config.MaxProviderEntryBytes)
		}
		out = append(out, model)
	}
	if len(out) > config.MaxProviderModels {
		return nil, fmt.Sprintf("模型最多 %d 个，现在有 %d 个", config.MaxProviderModels, len(out))
	}
	return out, ""
}

// cleanProviderEnv 与 cleanProviderArgs 同一套：去空行，再按 NAME=VALUE 校验。
func cleanProviderEnv(raw []string) ([]string, string) {
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if problem := config.EnvEntryProblem(entry); problem != "" {
			return nil, "环境变量「" + entry + "」不合法：" + problem +
				"：要写成 NAME=VALUE，NAME 只能用字母、数字与下划线，且不以数字开头"
		}
		if len(entry) > config.MaxProviderEntryBytes {
			return nil, fmt.Sprintf("环境变量条目超过 %d 字节", config.MaxProviderEntryBytes)
		}
		out = append(out, entry)
	}
	if len(out) > config.MaxProviderEnv {
		return nil, fmt.Sprintf("环境变量最多 %d 条，现在有 %d 条", config.MaxProviderEnv, len(out))
	}
	return out, ""
}

// handleSettingsProviderDelete 从写目标里删掉 [providers.<id>]。
func (s *Server) handleSettingsProviderDelete(w http.ResponseWriter, r *http.Request, id string) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cur := s.currentCfg()
	if cur == nil {
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法删除 provider")
		return
	}
	if _, ok := cur.Providers[id]; !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("provider 不存在: %s", id))
		return
	}
	// 先看引用：删掉被 agent 引用的 provider 会让整份配置校验失败。与其让
	// 用户看到「新配置没有通过校验」，不如直接点名是哪几个成员在用它。
	if users := agentsUsingProvider(cur.Agents, id); len(users) > 0 {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("不能删除 provider %s：成员 %s 还在用它。先把这些成员的 provider 改成别的，或连同成员一起删掉。",
				id, strings.Join(users, "、")))
		return
	}

	res, err := config.RemoveProviderSection(layers, id)
	if errors.Is(err, config.ErrProviderSectionMissing) {
		where := config.ProviderDefinedIn(layers, id)
		hint := "它定义在低优先级的配置层里，本接口只写第一层"
		if len(where) > 0 {
			hint = "它定义在 " + strings.Join(where, "、") + "，本接口只写 " + layers[0].Path
		}
		writeError(w, http.StatusConflict,
			fmt.Sprintf("不能删除 %s：%s。请直接改那份文件，删掉对应的 [providers.%s] 段", id, hint, id))
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
			Note:        "配置没有变化：文件未改动，也没有生成备份。",
		})
		return
	}
	s.adoptReloadedConfig(res.Config)
	writeJSON(w, http.StatusOK, settingsWriteResponse{
		BackupPath:     res.BackupPath,
		WriteTarget:    res.WriteTarget,
		RestartReasons: []string{"删除了 provider " + id},
		Note:           "已删除 provider " + id + "。" + settingsReloadNote,
	})
}

// writableProviderIDs 返回写目标里定义过的 provider id 集合。设置页用它
// 区分「用户加的」与「继承自低优先级层的」：后者不该在这里删。
func (s *Server) writableProviderIDs() map[string]bool {
	out := map[string]bool{}
	layers, err := s.settingsLayers()
	if err != nil || len(layers) == 0 || layers[0].Path == "" {
		return out
	}
	cfg, err := config.Load(layers[0].Path)
	if err != nil || cfg == nil {
		return out
	}
	for id := range cfg.Providers {
		out[id] = true
	}
	return out
}

// agentsUsingProvider 返回引用了 provider 的成员 id（排序后）。
func agentsUsingProvider(agents map[string]config.AgentConfig, providerID string) []string {
	var out []string
	for id, agent := range agents {
		if agent.Provider == providerID {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// commandResolves 判断 command 指向的东西在这台机器上找不找得到。
//
// 只用于界面上的状态点：绝对/相对路径就 stat，裸名字走 PATH 查找
// （exec.LookPath 在 Windows 上会按 PATHEXT 补 .exe/.cmd）。查不到不算错误，
// 配置本身合法——命令可能由别的启动脚本提供。
func commandResolves(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	if strings.ContainsAny(command, `\/:`) {
		info, err := os.Stat(command)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(command)
	return err == nil
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
