package serve

// settingsedit.go：设置页里原本「只读展示」的三处，现在都能改——
// 汇总成员（[summary] agent）、团队（[teams.<id>]）、数据目录（顶层 data_dir）。
//
// 三者与 agent / provider 走同一条写盘路径：只写 layers[0]、临时文件先校验、
// 通过后备份再替换。写完之后一律「重启 serve 生效」——它们都不在热更分级表
// 里，说了立即生效就是骗人。

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"council/internal/config"
)

// settingsSummaryRequest 是 POST /api/settings/summary 的请求体。
type settingsSummaryRequest struct {
	Agent string `json:"agent"`
}

// settingsTeamRequest 是 POST /api/settings/teams 的请求体。
type settingsTeamRequest struct {
	TeamID      string   `json:"team_id"`
	DisplayName string   `json:"display_name"`
	Members     []string `json:"members"`
}

// settingsDataDirRequest 是 POST /api/settings/data-dir 的请求体。
type settingsDataDirRequest struct {
	DataDir string `json:"data_dir"`
}

// handleSettingsSummarySave 设置 [summary] agent；空串表示关掉汇总。
func (s *Server) handleSettingsSummarySave(w http.ResponseWriter, r *http.Request) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	var req settingsSummaryRequest
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
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法保存汇总成员")
		return
	}
	agentID := strings.TrimSpace(req.Agent)
	if agentID != "" {
		if _, ok := cur.Agents[agentID]; !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("没有这个成员: %s", agentID))
			return
		}
		// 汇总在后台自己跑，必须是只读成员：写盘前就拒绝，别等 Validate。
		if cur.Agents[agentID].Permissions != config.PermissionRead {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("成员 %s 的 permissions 不是 read：汇总会在没人看着的时候跑，只能用只读成员", agentID))
			return
		}
	}

	res, err := config.WriteSummaryAgent(layers, agentID)
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
	note := "已关闭汇总。"
	if agentID != "" {
		note = "汇总成员已设为 " + agentID + "。"
	}
	writeJSON(w, http.StatusOK, settingsWriteResponse{
		BackupPath:     res.BackupPath,
		WriteTarget:    res.WriteTarget,
		RestartReasons: []string{"汇总成员变了"},
		Note:           note + settingsReloadNote,
	})
}

// handleSettingsTeamSave 新增或修改一个 team。upsert 语义与 agent 一致。
func (s *Server) handleSettingsTeamSave(w http.ResponseWriter, r *http.Request) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	var req settingsTeamRequest
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
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法保存团队")
		return
	}

	id := strings.TrimSpace(req.TeamID)
	if id == "" {
		writeError(w, http.StatusBadRequest, "team_id 不能为空")
		return
	}
	if problem := config.ProviderIDProblem(id); problem != "" {
		// team id 与 provider/agent 同一条 id 规则；错误文案里写的也是同一个规则。
		writeError(w, http.StatusBadRequest, strings.Replace(problem, "provider id", "team id", 1))
		return
	}
	members, dup := cleanMemberIDs(req.Members, cur)
	if dup != "" {
		writeError(w, http.StatusBadRequest, dup)
		return
	}
	if len(members) == 0 {
		writeError(w, http.StatusBadRequest, "至少要勾一个成员：空名单的团队没法用")
		return
	}

	prev, exists := cur.Teams[id]
	team := config.TeamConfig{DisplayName: strings.TrimSpace(req.DisplayName), Members: members}
	// 更细的字段（mode / thought_visible_to）不从表单来：写盘只改
	// display_name 与 members，文件里其余键原样保留，不需要在这里搬运。
	_ = prev
	_ = exists

	res, err := config.WriteTeamSection(layers, id, team)
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
		RestartReasons: []string{"团队 " + id + " 变了"},
		Note:           "已保存团队 " + id + "。" + settingsReloadNote,
	})
}

// handleSettingsTeamDelete 删除 [teams.<id>]。
func (s *Server) handleSettingsTeamDelete(w http.ResponseWriter, r *http.Request, id string) {
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
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法删除团队")
		return
	}
	if _, ok := cur.Teams[id]; !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("团队不存在: %s", id))
		return
	}
	res, err := config.RemoveTeamSection(layers, id)
	if errors.Is(err, config.ErrTeamSectionMissing) {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("不能删除 %s：它定义在低优先级的配置层里，本接口只写 %s", id, layers[0].Path))
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
		RestartReasons: []string{"删除了团队 " + id},
		Note:           "已删除团队 " + id + "。" + settingsReloadNote,
	})
}

// handleSettingsDataDirSave 改顶层 data_dir。改完要重启才生效：数据目录在
// 进程启动时就打开了，运行期换目录等于让窗口后面的库凭空搬家。
func (s *Server) handleSettingsDataDirSave(w http.ResponseWriter, r *http.Request) {
	if !s.settingsWriteGate(w, r) {
		return
	}
	var req settingsDataDirRequest
	if !decodeSettingsBody(w, r, &req) {
		return
	}
	dir := strings.TrimSpace(req.DataDir)
	if dir == "" {
		writeError(w, http.StatusBadRequest, "data_dir 不能为空：留空会退回到默认目录，请直接填一个绝对路径")
		return
	}
	if !filepath.IsAbs(dir) {
		writeError(w, http.StatusBadRequest, "data_dir 必须是绝对路径")
		return
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		writeError(w, http.StatusBadRequest, "这个路径是一个文件，不是目录")
		return
	}

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := config.WriteDataDir(layers, dir)
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
		RestartReasons: []string{"数据目录变了"},
		Note: "已把 data_dir 改成 " + dir + "。" +
			"下次启动 council serve / 桌面版才会用新目录；已有的库不会自动搬过去，" +
			"需要的话先把旧目录里的文件复制到新目录。",
	})
}

// cleanMemberIDs 规范化 members 名单：去空白、去重、检查每个 id 都存在。
// 返回第一个问题（重复或未定义）的说明，没有问题时说明为空。
func cleanMemberIDs(raw []string, cfg *config.Config) (members []string, problem string) {
	seen := make(map[string]bool, len(raw))
	members = make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if seen[id] {
			return nil, fmt.Sprintf("成员 %s 在名单里出现了两次", id)
		}
		seen[id] = true
		if _, ok := cfg.Agents[id]; !ok {
			return nil, fmt.Sprintf("没有这个成员: %s", id)
		}
		members = append(members, id)
	}
	sort.Strings(members)
	return members, ""
}
