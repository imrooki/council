package serve

// models.go 是工作台的「识别本机模型」入口（GUI 第 8 片）：
//
//   - GET  /api/models：按配置里有的那几家跑一次模型识别（不提问、不花钱），
//     把 claude / codex / grok / agy 当前可用的模型列出来，含 codex 的
//     custom/ 行（经 本地转发服务 转发的 grok、gemini 就在那里面）；
//   - POST /api/conversations/{id}/members/from-model：把识别出来的一行写成
//     配置里的一个 agent，再让它加入会话——效果与 TUI 的 /add <模型名> 相同
//     （AD1），只是入口在网页里。
//
// 前端「+ 添加成员」菜单里，配置里已有的成员走 GUI4 的 /members；只有本机
// 识别到、配置里还没有的模型才走这里。两条路最后都落到同一张 participants 表。
//
// 写配置复用设置页那条路（config.WriteAgentSection + 备份 + 校验），不自己
// 拼 TOML：会漂的第二个写入点比少一个功能更糟。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/turn"
)

// modelDiscoveryTimeout 是每一家的识别超时，与首次运行、/add 同值（20 秒）。
// 独立成变量是为了测试能压到毫秒级。
var modelDiscoveryTimeout = 20 * time.Second

// modelDiscoveryExec 是识别用的执行器，生产是 turn.RunDiscovery；测试换成夹具。
var modelDiscoveryExec provider.DiscoveryExec = turn.RunDiscovery

// discoverProviderModels 是一家 provider 的「起进程 + 解析」，生产是
// provider.DiscoverModels。单独留一个变量，测试就能整段换掉，不必去伪造
// 每家 CLI 的输出格式。
var discoverProviderModels = provider.DiscoverModels

// modelCacheTTL 是识别结果的缓存时长。加成员时要确认「这一行确实识别得到」，
// 不能拿一个几分钟前的快照去写配置；超过这个时长就重新识别那一家。
const modelCacheTTL = 5 * time.Minute

// discoveredProvider 是一家的识别结果。Err 单独留着，一家坏掉不影响其余几家。
type discoveredProvider struct {
	ID     string
	Models []provider.DiscoveredModel
	Err    error
}

// modelJSON 是 /api/models 的一行。Efforts 只有 claude 与 codex 有；Custom
// 标出 codex 的 custom/ 行，前端据此加一句「经 本地转发服务 转发」的说明。
type modelJSON struct {
	Provider      string   `json:"provider"`
	Model         string   `json:"model"`
	Name          string   `json:"name"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"default_effort"`
	SuggestedID   string   `json:"suggested_id"`
	Custom        bool     `json:"custom"`
	// InConfig 表示配置里已经有同一个模型（不看强度）：前端把它画成不可再
	// 加，并给出已有成员的名字。
	InConfig bool   `json:"in_config"`
	AgentID  string `json:"agent_id,omitempty"`
}

type modelProviderJSON struct {
	ID    string `json:"id"`
	Error string `json:"error,omitempty"`
}

type modelsResponse struct {
	Models    []modelJSON         `json:"models"`
	Providers []modelProviderJSON `json:"providers"`
}

// addModelRequest 是 POST .../members/from-model 的请求体。
type addModelRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
	AgentID  string `json:"agent_id"`
}

// discoverConfigured 并行识别配置里声明过的每一家。固定的 claude、grok、
// agy、codex 顺序在前，其余按字典序；配置里没有的 provider 不跑——那是
// 用户明确没装的一家，跑它只会多一条错误。
func (s *Server) discoverConfigured(ctx context.Context) []discoveredProvider {
	cfg := s.currentCfg()
	if cfg == nil {
		return nil
	}
	detected := make(map[string]string, 4)
	for _, d := range config.DetectProviders() {
		if d.Path != "" {
			detected[d.ID] = d.Path
		}
	}
	ids := make([]string, 0, len(cfg.Providers))
	for id := range cfg.Providers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return providerOrder(ids[i]) < providerOrder(ids[j]) || (providerOrder(ids[i]) == providerOrder(ids[j]) && ids[i] < ids[j])
	})

	results := make([]discoveredProvider, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		path := detected[id]
		if path == "" {
			path = cfg.Providers[id].Command
		}
		wg.Add(1)
		go func(i int, id, path string) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, modelDiscoveryTimeout)
			defer cancel()
			models, err := discoverProviderModels(cctx, id, path, modelDiscoveryExec)
			results[i] = discoveredProvider{ID: id, Models: models, Err: err}
		}(i, id, path)
	}
	wg.Wait()
	return results
}

// providerOrder 是固定显示顺序，其余 provider 排在后面。
func providerOrder(id string) int {
	switch id {
	case "claude":
		return 0
	case "codex":
		return 1
	case "grok":
		return 2
	case "agy":
		return 3
	}
	return 9
}

// cacheModels 记下这次识别结果，供加成员时校验。
func (s *Server) cacheModels(rows []discoveredProvider) {
	s.modelsMu.Lock()
	s.modelsAt = time.Now()
	s.modelsDB = rows
	s.modelsMu.Unlock()
}

// cachedModels 交出最近一次识别结果。过期或没有就返回 nil。
func (s *Server) cachedModels() []discoveredProvider {
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()
	if s.modelsDB == nil || time.Since(s.modelsAt) > modelCacheTTL {
		return nil
	}
	return s.modelsDB
}

// findDiscoveredModel 找一家的一行；缓存里没有就只重跑这一家。
func (s *Server) findDiscoveredModel(ctx context.Context, providerID, model string) (provider.DiscoveredModel, error) {
	rows := s.cachedModels()
	for _, row := range rows {
		if row.ID != providerID {
			continue
		}
		for _, m := range row.Models {
			if m.Model == model {
				return m, nil
			}
		}
		return provider.DiscoveredModel{}, fmt.Errorf("%s 识别结果里没有模型 %q", providerID, model)
	}
	// 只跑这一家：用户点的是一个刚在页面上看到的模型，重跑全部四家没必要。
	cfg := s.currentCfg()
	if cfg == nil {
		return provider.DiscoveredModel{}, errors.New("写模式没有加载配置")
	}
	if _, ok := cfg.Providers[providerID]; !ok {
		return provider.DiscoveredModel{}, fmt.Errorf("配置里没有 %s 这一家", providerID)
	}
	path := ""
	for _, d := range config.DetectProviders() {
		if d.ID == providerID && d.Path != "" {
			path = d.Path
		}
	}
	if path == "" {
		path = cfg.Providers[providerID].Command
	}
	cctx, cancel := context.WithTimeout(ctx, modelDiscoveryTimeout)
	defer cancel()
	models, err := discoverProviderModels(cctx, providerID, path, modelDiscoveryExec)
	if err != nil {
		return provider.DiscoveredModel{}, fmt.Errorf("%s 识别失败：%v", providerID, err)
	}
	for _, m := range models {
		if m.Model == model {
			return m, nil
		}
	}
	return provider.DiscoveredModel{}, fmt.Errorf("%s 识别结果里没有模型 %q", providerID, model)
}

// modelForConfig 把识别结果换算成要写进配置的那一行：强度按该模型自己支持
// 的档校验，id 取建议值或用户给的名字。
func modelForConfig(m provider.DiscoveredModel, effort string) (config.AgentConfig, string, error) {
	switch m.Provider {
	case "claude", "codex":
		if effort == "" {
			effort = m.DefaultEffort()
		} else {
			found := false
			for _, e := range m.Efforts {
				if e == effort {
					found = true
					break
				}
			}
			if !found {
				if len(m.Efforts) == 0 {
					return config.AgentConfig{}, "", fmt.Errorf("%s 没有可选的强度，不能带 effort", m.Model)
				}
				return config.AgentConfig{}, "", fmt.Errorf("%s 支持的强度是 %s，没有 %s", m.Model, strings.Join(m.Efforts, "|"), effort)
			}
		}
	default:
		// grok 的强度写在它自己的配置里，agy 写在模型名里；council 这一层不收。
		if effort != "" {
			return config.AgentConfig{}, "", fmt.Errorf("%s 的强度不在 council 里选，不能带 effort", m.Provider)
		}
		effort = ""
	}

	display := strings.TrimSpace(m.Name)
	if display == "" {
		display = m.Model
	}
	return config.AgentConfig{
		DisplayName: display,
		Provider:    m.Provider,
		Model:       m.Model,
		Effort:      effort,
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	}, effort, nil
}

// sameBrainAgent 找配置里 provider + model + effort 都一样的那一个成员。
// 有就不要重复写：AD1 的 /add 也是这个口径。
func sameBrainAgent(cfg *config.Config, providerID, model, effort string) string {
	if cfg == nil {
		return ""
	}
	for id, ac := range cfg.Agents {
		if ac.Provider == providerID && ac.Model == model && ac.Effort == effort {
			return id
		}
	}
	return ""
}

// handleModels 认出本机每一家的模型。只读模式没有这个接口：识别要读配置里的
// provider 清单，没有那份配置时列出一张空表比 405 更容易被误读成「一个都没装」。
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有模型识别接口：用 council serve -rw 启动才能识别本机模型")
		return
	}
	if !requireGET(w, r) {
		return
	}
	rows := s.discoverConfigured(r.Context())
	s.cacheModels(rows)

	cfg := s.currentCfg()
	resp := modelsResponse{Models: []modelJSON{}, Providers: []modelProviderJSON{}}
	for _, row := range rows {
		status := modelProviderJSON{ID: row.ID}
		if row.Err != nil {
			status.Error = row.Err.Error()
		}
		resp.Providers = append(resp.Providers, status)
		for _, m := range row.Models {
			item := modelJSON{
				Provider:      m.Provider,
				Model:         m.Model,
				Name:          m.Name,
				Efforts:       m.Efforts,
				DefaultEffort: m.DefaultEffort(),
				SuggestedID:   m.SuggestedID(m.DefaultEffort()),
				Custom:        m.Custom,
			}
			if item.Efforts == nil {
				item.Efforts = []string{}
			}
			if id := sameBrainAgent(cfg, m.Provider, m.Model, item.DefaultEffort); id != "" {
				item.InConfig = true
				item.AgentID = id
			}
			resp.Models = append(resp.Models, item)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleConversationMemberFromModel 把识别出来的一行写成配置成员并加入会话。
//
// 顺序与 AD1 一致：先校验这一行确实识别得到、强度合法、id 不撞名，再写文件
// （config.WriteAgentSection 自带备份与「写后加载校验」），然后换展示配置、
// 把新配置交给 Runner，最后 Join。任何一步失败都不 Join。
func (s *Server) handleConversationMemberFromModel(w http.ResponseWriter, r *http.Request, convID string) {
	if !s.memberWriteGate(w, r) {
		return
	}
	var req addModelRequest
	if !decodeMemberJSON(w, r, &req) {
		return
	}
	if _, ok := s.lookupConversation(w, r, convID); !ok {
		return
	}

	providerID := strings.TrimSpace(req.Provider)
	model := strings.TrimSpace(req.Model)
	if providerID == "" || model == "" {
		writeError(w, http.StatusBadRequest, "provider 与 model 不能为空")
		return
	}
	m, err := s.findDiscoveredModel(r.Context(), providerID, model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	agent, effort, err := modelForConfig(m, strings.TrimSpace(req.Effort))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	cur := s.currentCfg()
	if cur == nil {
		writeError(w, http.StatusInternalServerError, "写模式没有加载配置，无法新增成员")
		return
	}
	if id := sameBrainAgent(cur, providerID, model, effort); id != "" {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("配置里已经有同一个模型、同一强度的成员 %s，直接在菜单里选它", id))
		return
	}

	id := strings.TrimSpace(req.AgentID)
	if id == "" {
		id = m.SuggestedID(effort)
	}
	if problem := config.NewAgentIDProblem(cur, id); problem != "" {
		writeError(w, http.StatusBadRequest, problem)
		return
	}

	layers, err := s.settingsLayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.settingsMu.Lock()
	res, err := config.WriteAgentSection(layers, id, agent, config.AgentConfig{})
	if err != nil {
		s.settingsMu.Unlock()
		s.writeSettingsError(w, err)
		return
	}
	s.settingsMu.Unlock()

	if res.Changed {
		// 新成员要立刻能跑：选他那个人不会愿意为此重启服务。只「新增」的时候
		// 换 Runner 的配置是安全的——已有成员的模型与强度没变，指纹不变，
		// provider 会话不受影响；这也正是设置页「改已有成员要重启」没有做进
		// 来的原因。改已有成员仍走设置页那条路。
		s.applyAddedAgent(res.Config, id)
	}

	participant := conversation.Participant{
		AgentID:     id,
		DisplayName: agent.DisplayName,
		Provider:    agent.Provider,
		Active:      true,
	}
	if err := s.write.Svc.Join(r.Context(), convID, participant); err != nil {
		if errors.Is(err, conversation.ErrConversationNotFound) {
			writeError(w, http.StatusNotFound, "会话不存在: "+convID)
			return
		}
		writeError(w, http.StatusInternalServerError, "加入成员失败: "+err.Error())
		return
	}

	w.Header().Set("X-Council-Agent-ID", id)
	s.conversationMembers(w, r, convID)
}

// applyAddedAgent 换展示配置，并把同一份交给 Runner，让刚写进文件的成员
// 马上可执行。Runner 为空（直接构造 Server 的测试）时只换展示配置。
func (s *Server) applyAddedAgent(cfg *config.Config, agentID string) {
	if cfg == nil || s.write == nil {
		return
	}
	if cfg.Agents == nil {
		cfg.Agents = map[string]config.AgentConfig{}
	}
	s.cfgMu.Lock()
	s.write.Cfg = cfg
	s.write.RunCfg = cfg
	s.cfgMu.Unlock()
	if s.write.Rn != nil {
		s.write.Rn.SetConfig(cfg)
	}
}

// decodeMemberJSON 读一个 JSON 对象，上限与 GUI4 的加人请求体相同。
func decodeMemberJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxMemberBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON: "+err.Error())
		return false
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求体只能有一个 JSON 对象")
		return false
	}
	return true
}
