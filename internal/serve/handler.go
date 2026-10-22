package serve

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/store"
)

// indexHTML 是内嵌的单文件前端。零构建：没有 npm、没有打包器，
// 改完这个文件重新编译一次 council 就是新看板。
//
//go:embed static/index.html
var indexHTML []byte

// defaultMessagesLimit 与 maxMessagesLimit 来自任务书 §4：
// 不传 limit 时给 200 条，传了也不允许超过 1000。
const (
	defaultMessagesLimit = 200
	maxMessagesLimit     = 1000
)

// Server 是看板的 HTTP 侧。models 是 agent id -> 模型名，来自配置里的
// [agents.*].model：turn_usage 表没有 model 列，配置是唯一不必另写 SQL
// 就能拿到的模型名来源。配置没加载（serve 在默认配置缺失时的回落路径）时
// 这个 map 是空的，响应里 model 就是空串，而不是编一个值。
type Server struct {
	store   *store.Store
	version string
	models  map[string]string

	// Verbose 为 true 时每个请求往 Log 写一行。
	Verbose bool
	// Log 是 verbose 行的出口；nil 表示 os.Stderr。
	Log io.Writer

	// write 非 nil 表示 -rw 写模式已经装配好：有配置、Service 与 Runner，
	// /api/ask 与 /api/jobs 才会真正工作。只读模式保持 nil，两个端点回 405。
	write *WriteDeps
	// jobs 是写模式的内存任务表；只有 EnableWrite 之后才非 nil。
	jobs *jobTable
	// jobTTL 是任务结束后在内存里保留的时长，零值表示 defaultJobTTL。
	// 做成字段而不是常量，测试才能用几十毫秒验证 60 秒的清理，不必真等。
	jobTTL time.Duration

	// jobCtx 是所有后台议会的父 ctx。它刻意不是 HTTP 请求的 ctx：浏览器
	// 断开只该取消订阅，不该取消已经花钱的一轮。进程要退出时由 cancelJobs
	// 显式取消——把「静默丢一轮」降级成「明确取消一轮」。
	jobCtx     context.Context
	cancelJobs context.CancelFunc
	// jobsWG 让关闭流程能等到所有后台议会真正结束之后才关库放锁。
	// 不加它时 HTTP Shutdown 只等连接，没 SSE 读者的 job 会继续用已经
	// 被 st.Close() 关掉的 Service/Runner，那一轮就静默丢了。
	jobsWG sync.WaitGroup
}

// NewServer 构造看板 handler。models 会被复制一份，调用方之后改自己的 map
// 不会影响正在跑的请求。
func NewServer(st *store.Store, version string, models map[string]string) *Server {
	copied := make(map[string]string, len(models))
	for id, model := range models {
		copied[id] = model
	}
	return &Server{store: st, version: version, models: copied}
}

// Handler 返回完整的 HTTP 路由。
//
// 路由手写而不是用 Go 1.22 的 "GET /api/..." 模式：那种模式在方法不符时
// 由 ServeMux 直接写纯文本 405，而本片要求所有 API 错误都是
// {"error":"..."}。手写一份分支换来的是 404 / 405 也保持同一个 JSON 形状。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/conversations", s.handleConversations)
	mux.HandleFunc("/api/conversations/", s.handleConversationPath)
	// 写模式的入口在只读模式下也登记：任务书要求非 -rw 时它们回 405 并说明
	// 原因，而不是被 /api/ 的兜底变成「没有这个接口」的 404。是否真正可用由
	// handler 里的 s.write == nil 判断，路由本身不按模式分叉。
	mux.HandleFunc("/api/ask", s.handleAsk)
	mux.HandleFunc("/api/jobs/", s.handleJobPath)
	mux.HandleFunc("/api/", s.handleAPINotFound)
	mux.HandleFunc("/api", s.handleAPINotFound)
	mux.HandleFunc("/", s.handleIndex)
	return s.logRequests(mux)
}

// logRequests 在 -verbose 下给每个请求写一行。
// 默认不写：看板本身每 3 秒轮询两次，打开 -verbose 之后 stderr 也会跟着刷。
func (s *Server) logRequests(next http.Handler) http.Handler {
	if !s.Verbose {
		return next
	}
	logFile := s.Log
	if logFile == nil {
		logFile = os.Stderr
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		fmt.Fprintf(logFile, "%s %s %d %s\n",
			r.Method, r.URL.RequestURI(), rec.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush 把 SSE 需要的 flush 透传下去。没有这一行时，-verbose 下每个请求
// 都被 statusRecorder 包一层，SSE handler 的 w.(http.Flusher) 断言会失败：
// 看板会拒绝推流，而 -verbose 只是日志开关，不该改变协议能力。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// handleIndex 只认根路径与 /index.html。其余路径走 404：
// 内嵌的只有这一个文件，不假装自己是一个静态站点。
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
}

type healthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	DB      string `json:"db"`
	// Writable 只在 -rw 时为 true，omitempty 让只读模式的响应与 SRV1
	// 逐字节一致（既有 health 用例钉死了恰好三个键）。
	Writable bool `json:"writable,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{
		OK:       true,
		Version:  s.version,
		DB:       s.store.Path(),
		Writable: s.write != nil,
	})
}

type conversationSummary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Cwd          string    `json:"cwd"`
	TeamID       string    `json:"team_id"`
	CreatedAt    time.Time `json:"created_at"`
	Members      int       `json:"members"`
	LastActivity time.Time `json:"last_activity"`
	LastSeq      int64     `json:"last_seq"`
	MessageCount int       `json:"message_count"`
}

type memberJSON struct {
	AgentID     string `json:"agent_id"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Active      bool   `json:"active"`
}

type conversationDetail struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Cwd          string       `json:"cwd"`
	TeamID       string       `json:"team_id"`
	CreatedAt    time.Time    `json:"created_at"`
	LastActivity time.Time    `json:"last_activity"`
	LastSeq      int64        `json:"last_seq"`
	MessageCount int          `json:"message_count"`
	Members      []memberJSON `json:"members"`
}

// handleConversations 返回全部会话的摘要，按最后活动倒序。
//
// 成员数只数活跃成员，与 /resume 的口径一致（repl/resume.go）：软删除的成员
// 仍留在 participants 里，把它算进「当前成员」会让 /leave 之后人数不掉。
// 最后活动与条数都从 Messages 读整段算：本包不另写 COUNT/MAX 的 SQL，
// 复用现有读方法是本片的硬约束。会话数量是本地看板的规模，这个代价可接受；
// 前端也不会每 3 秒拉一次整个列表（见 static/index.html 的刷新节奏）。
func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	ctx := r.Context()
	convs, err := s.store.Conversations(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取会话列表失败: "+err.Error())
		return
	}

	out := make([]conversationSummary, 0, len(convs))
	for _, conv := range convs {
		parts, err := s.store.Participants(ctx, conv.ID, false)
		if err != nil {
			writeError(w, http.StatusInternalServerError,
				fmt.Sprintf("读取会话 %s 的成员失败: %v", conv.ID, err))
			return
		}
		msgs, err := s.store.Messages(ctx, conv.ID, 0, 0)
		if err != nil {
			writeError(w, http.StatusInternalServerError,
				fmt.Sprintf("读取会话 %s 的消息失败: %v", conv.ID, err))
			return
		}
		lastActivity := conv.CreatedAt
		lastSeq := int64(0)
		if n := len(msgs); n > 0 {
			lastActivity = msgs[n-1].CreatedAt
			lastSeq = msgs[n-1].Seq
		}
		out = append(out, conversationSummary{
			ID:           conv.ID,
			Title:        conv.Title,
			Cwd:          conv.Cwd,
			TeamID:       conv.TeamID,
			CreatedAt:    conv.CreatedAt,
			Members:      len(parts),
			LastActivity: lastActivity,
			LastSeq:      lastSeq,
			MessageCount: len(msgs),
		})
	}

	// 时间相同时依次按创建时间、id 排，保证同一份库每次请求的顺序相同。
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastActivity.Equal(out[j].LastActivity) {
			return out[i].LastActivity.After(out[j].LastActivity)
		}
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	writeJSON(w, http.StatusOK, out)
}

// handleConversationPath 处理 /api/conversations/{id} 一棵子树。
// 路径只按 / 切开：id 是库里的 TEXT 主键（现有实现是十六进制串），
// 不需要为它做通配或正则。
func (s *Server) handleConversationPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/conversations/")
	if rest == "" || strings.HasSuffix(rest, "/") {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	segments := strings.Split(rest, "/")
	switch len(segments) {
	case 1:
		s.handleConversationDetail(w, r, segments[0])
	case 2:
		switch segments[1] {
		case "messages":
			s.handleMessages(w, r, segments[0])
		case "usage":
			s.handleUsage(w, r, segments[0])
		default:
			writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		}
	default:
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
	}
}

// handleConversationDetail 返回单个会话与全部成员。
//
// 成员用 includeInactive=true：响应里带 active 字段，只有把退出过的成员
// 也带回来，这个字段才有信息量。会话列表的 members 数仍只数活跃成员。
func (s *Server) handleConversationDetail(w http.ResponseWriter, r *http.Request, id string) {
	if !requireGET(w, r) {
		return
	}
	ctx := r.Context()
	conv, ok := s.lookupConversation(w, r, id)
	if !ok {
		return
	}
	parts, err := s.store.Participants(ctx, conv.ID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("读取会话 %s 的成员失败: %v", conv.ID, err))
		return
	}
	msgs, err := s.store.Messages(ctx, conv.ID, 0, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("读取会话 %s 的消息失败: %v", conv.ID, err))
		return
	}
	lastActivity := conv.CreatedAt
	lastSeq := int64(0)
	if n := len(msgs); n > 0 {
		lastActivity = msgs[n-1].CreatedAt
		lastSeq = msgs[n-1].Seq
	}

	members := make([]memberJSON, 0, len(parts))
	for _, p := range parts {
		members = append(members, memberJSON{
			AgentID:     p.AgentID,
			DisplayName: p.DisplayName,
			Provider:    p.Provider,
			Active:      p.Active,
		})
	}
	writeJSON(w, http.StatusOK, conversationDetail{
		ID:           conv.ID,
		Title:        conv.Title,
		Cwd:          conv.Cwd,
		TeamID:       conv.TeamID,
		CreatedAt:    conv.CreatedAt,
		LastActivity: lastActivity,
		LastSeq:      lastSeq,
		MessageCount: len(msgs),
		Members:      members,
	})
}

type messageJSON struct {
	Seq       int64     `json:"seq"`
	SenderID  string    `json:"sender_id"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type messagesResponse struct {
	Messages []messageJSON `json:"messages"`
	LastSeq  int64         `json:"last_seq"`
}

// handleMessages 返回 after 之后的一页消息，按 seq 升序。
//
// last_seq 是这一页最后一条的 seq；这一页为空时回显 after。不返回会话的
// 最大 seq：limit 截断时（默认 200、最多 1000）返回最大值会让前端直接跳过
// 中间没读到的消息。前端拿这个值继续 after= 轮询，翻页才是连续的。
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request, id string) {
	if !requireGET(w, r) {
		return
	}
	ctx := r.Context()
	conv, ok := s.lookupConversation(w, r, id)
	if !ok {
		return
	}
	after, limit, err := parseMessagesQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	msgs, err := s.store.Messages(ctx, conv.ID, after, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("读取会话 %s 的消息失败: %v", conv.ID, err))
		return
	}
	out := make([]messageJSON, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, messageJSON{
			Seq:       m.Seq,
			SenderID:  m.SenderID,
			Kind:      string(m.Kind),
			Text:      m.Text,
			CreatedAt: m.CreatedAt,
		})
	}
	lastSeq := after
	if n := len(out); n > 0 {
		lastSeq = out[n-1].Seq
	}
	writeJSON(w, http.StatusOK, messagesResponse{Messages: out, LastSeq: lastSeq})
}

// parseMessagesQuery 校验 after / limit。缺省分别是 0 与 200。
//
// limit 超过上限按非法处理而不是悄悄截成 1000：用户写 5000 就是想要 5000 条，
// 静默改成 1000 会让「为什么少了几条」变成一个查不出来的问题。
// 任务书 §4 说 after/limit 非法返回 400，这一句在这里落地。
func parseMessagesQuery(r *http.Request) (int64, int, error) {
	after := int64(0)
	limit := defaultMessagesLimit
	q := r.URL.Query()

	if values, ok := q["after"]; ok {
		if len(values) != 1 || values[0] == "" {
			return 0, 0, fmt.Errorf("after 必须是一个非负整数")
		}
		n, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || n < 0 {
			return 0, 0, fmt.Errorf("after 必须是一个非负整数，实际是 %q", values[0])
		}
		after = n
	}

	if values, ok := q["limit"]; ok {
		if len(values) != 1 || values[0] == "" {
			return 0, 0, fmt.Errorf("limit 必须是 1 到 %d 之间的整数", maxMessagesLimit)
		}
		n, err := strconv.Atoi(values[0])
		if err != nil || n < 1 || n > maxMessagesLimit {
			return 0, 0, fmt.Errorf("limit 必须是 1 到 %d 之间的整数，实际是 %q", maxMessagesLimit, values[0])
		}
		limit = n
	}
	return after, limit, nil
}

type agentUsageJSON struct {
	AgentID     string  `json:"agent_id"`
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Turns       int     `json:"turns"`
	In          int64   `json:"in"`
	CacheCreate int64   `json:"cache_create"`
	CacheRead   int64   `json:"cache_read"`
	Out         int64   `json:"out"`
	CostUSD     float64 `json:"cost_usd"`
	// CostReported 说明这一行的 cost_usd 是不是真金额。false 时前端必须
	// 显示「不报」而不是 0：不报金额的 provider，0 表示「没有这个数」，
	// 不是「免费」。
	CostReported bool `json:"cost_reported"`
}

type usageResponse struct {
	CostUSD  float64          `json:"cost_usd"`
	Complete bool             `json:"complete"`
	PerAgent []agentUsageJSON `json:"per_agent"`
}

// handleUsage 返回一条会话的账。
//
// complete 沿用 headless 的 cost_complete 口径（internal/headless costGap）：
// 名单里只要有一位的 provider 不是 ReportsCost=SupportYes，合计就只是
// 「报过金额的那些轮之和」，必须标成不完整。没报金额的轮不是 0 元。
// 这里把退出过的成员也数进名单：他们的轮次仍在 total 里，漏掉他们会让
// complete 错误地变成 true。
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request, id string) {
	if !requireGET(w, r) {
		return
	}
	ctx := r.Context()
	conv, ok := s.lookupConversation(w, r, id)
	if !ok {
		return
	}
	resp, err := s.buildUsage(ctx, conv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("读取会话 %s 的账失败: %v", conv.ID, err))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) buildUsage(ctx context.Context, convID string) (usageResponse, error) {
	parts, err := s.store.Participants(ctx, convID, true)
	if err != nil {
		return usageResponse{}, err
	}
	usages, err := s.store.TurnUsages(ctx, convID, "", -1)
	if err != nil {
		return usageResponse{}, err
	}

	byAgent := make(map[string]*agentUsageJSON, len(parts))
	ensure := func(agentID string) *agentUsageJSON {
		row := byAgent[agentID]
		if row == nil {
			row = &agentUsageJSON{AgentID: agentID}
			byAgent[agentID] = row
		}
		return row
	}

	participantIDs := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		participantIDs[p.AgentID] = struct{}{}
		row := ensure(p.AgentID)
		row.Provider = p.Provider
		row.Model = s.models[p.AgentID]
	}

	total := 0.0
	for _, u := range usages {
		row := ensure(u.AgentID)
		row.Turns++
		if u.InputTokens != nil {
			row.In += int64(*u.InputTokens)
		}
		if u.CacheCreate != nil {
			row.CacheCreate += int64(*u.CacheCreate)
		}
		if u.CacheRead != nil {
			row.CacheRead += int64(*u.CacheRead)
		}
		if u.OutputTokens != nil {
			row.Out += int64(*u.OutputTokens)
		}
		if u.CostUSD != nil {
			row.CostUSD += *u.CostUSD
			total += *u.CostUSD
		}
	}

	ids := make([]string, 0, len(byAgent))
	for id := range byAgent {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	perAgent := make([]agentUsageJSON, 0, len(ids))
	for _, id := range ids {
		row := byAgent[id]
		// 这一行的金额可不可信，只看它自己那一家报不报：合计的 complete
		// 是整个会话的口径，行内必须逐行给出，否则前端只能把 0 画成免费。
		row.CostReported = reportsCost(row.Provider)
		perAgent = append(perAgent, *row)
	}

	complete := len(parts) > 0
	for _, p := range parts {
		if !reportsCost(p.Provider) {
			complete = false
			break
		}
	}
	// 账里有、participants 里没有的 agent（例如被外部删过行）同样让
	// complete 不成立：这一行的金额来自谁、报不报，配置与成员表都答不上来。
	if complete {
		for _, u := range usages {
			if _, ok := participantIDs[u.AgentID]; !ok {
				complete = false
				break
			}
		}
	}

	return usageResponse{CostUSD: total, Complete: complete, PerAgent: perAgent}, nil
}

// reportsCost 回答「这一家会不会报金额」。找不到适配器按「不报」处理，
// 与 headless.costGap 对未知 provider 的处置一致：不知道就不许说完整。
func reportsCost(providerID string) bool {
	p, err := provider.Lookup(providerID)
	if err != nil {
		return false
	}
	return p.Capabilities().ReportsCost == provider.SupportYes
}

// lookupConversation 取会话；不存在写 404，其它错误写 500。
func (s *Server) lookupConversation(w http.ResponseWriter, r *http.Request, id string) (conversation.Conversation, bool) {
	conv, err := s.store.Conversation(r.Context(), id)
	if errors.Is(err, conversation.ErrConversationNotFound) {
		writeError(w, http.StatusNotFound, "会话不存在: "+id)
		return conversation.Conversation{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取会话失败: "+err.Error())
		return conversation.Conversation{}, false
	}
	return conv, true
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	writeError(w, http.StatusMethodNotAllowed, "只支持 GET，看板是只读的")
	return false
}
