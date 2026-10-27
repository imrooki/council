package serve

// localagent.go 是看板上唯一一个本机 agent 读接口：GET /api/local-agents。
//
// 四个设计约束（Plan §4.3 ⑦、§4.3 ⑧、§10、§12.1）：
//
//  1. **只在 -rw 下存在**。只读看板没有完整配置，也就没有用户的授权层级；
//     在那里回一张空表会被读成「本机没有 agent」。
//  2. **按需扫描，没有后台循环**。每个请求做一次快照；前端只在设置页第 6 类
//     可见时每 3 秒拉一次（与看板既有节奏一致），页面隐藏或离开这一页就停。
//     层级是 off 时**一次枚举都不做**（S-01），响应里只有空表与层级。
//  3. **不合并、不猜测**。managed 行来自 inflight_turns（有身份、没有 PID），
//     observed 行来自进程枚举（有 PID、没有身份）；⑨ 的 PID 钩子还没做，
//     所以两段仍然并列，不靠 PID 拼成一条（Plan §4.3 ⑨）。
//  4. **失败不 500**。与 /api/models 同构（§4.1 第 8 条）：扫描失败返回
//     200 + 空表 + observed_error，页面照常显示另一段。检测器坏掉不该让
//     看板整页报错。
//
// 为什么建在 serve 里而不是 localagent 包：这个形状是**给浏览器看的一份
// 拼装**（managed + observed + leases），而 localagent 只认进程事实；
// 把 HTTP 的 JSON 形状塞回那个包会让 CLI 也跟着依赖 Web 形状。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/localagent"
	"council/internal/store"
)

// localAgentScanTimeout 是一次请求里读库的上限。进程枚举本身是几十毫秒的
// 事；这里防的是 SQLite 正好被写事务占着时的等待（busy_timeout 是 5 秒）。
const localAgentScanTimeout = 5 * time.Second

// localAgentProcessJSON 是响应里的一行。
//
// pid 是 0 表示「不知道」：managed 行现在没有 PID（⑨ 的钩子未做），
// 编一个 0 之外的数字就是伪造事实。omitempty 让这种行干脆不出现 pid 键。
type localAgentProcessJSON struct {
	Kind      string `json:"kind"`
	Source    string `json:"source"`
	Certainty string `json:"certainty,omitempty"`
	PID       int    `json:"pid,omitempty"`
	PPID      int    `json:"ppid,omitempty"`
	ExeName   string `json:"exe_name,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	// WorkDir 只对 managed 行有意义（inflight_turns.work_dir）。observed 行
	// 一律空：本仓库没有从进程拿 cwd 的可靠办法，不猜（Plan §5.4）。
	WorkDir string `json:"work_dir,omitempty"`
	// Evidence 是判定依据的短摘要（source=… / exe=… / certainty=…）。
	// **不含完整映像路径**：S-09 允许瞬时读取它做同名消歧，但不把
	// C:\Users\... 这种路径塞进 HTTP 响应里。要原路径时用 council who。
	Evidence []string `json:"evidence"`
	// managed 行才有身份四件套；observed 行留空。合并之后，被认回来的那条
	// observed 行同样带这四件（Source 也升级成 managed）。
	ConversationID string `json:"conversation_id,omitempty"`
	AgentID        string `json:"agent_id,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	Phase          string `json:"phase,omitempty"`
	// MergeEvidence 是被合并的那几条上的依据（pid / shim=cmd.exe）；
	// 没参与合并的行没有这个键。
	MergeEvidence string `json:"merge_evidence,omitempty"`
}

// localAgentLeaseJSON 是一行有效租约（只有 coordinate 才查）。
//
// granted_at 一起给出：`council claim -server` / `council release -server` 会把
// 这一行还原成 store.Lease，再按与直接写库完全相同的一套文案打印
// （页面自己用不到这个字段）。
type localAgentLeaseJSON struct {
	LeaseID     string `json:"lease_id"`
	ResourceKey string `json:"resource_key"`
	OwnerKind   string `json:"owner_kind"`
	OwnerID     string `json:"owner_id"`
	Mode        string `json:"mode"`
	Generation  int64  `json:"generation"`
	GrantedAt   string `json:"granted_at"`
	ExpiresAt   string `json:"expires_at"`
	// OverlapsCwd 表示这条租约与看板进程的启动目录相同或互为父子：
	// 页面据此提示「另一个 agent 正在改这个目录」（Plan §11 P5）。
	OverlapsCwd bool `json:"overlaps_cwd"`
}

// localAgentsResponse 是 GET /api/local-agents 的完整形状。
//
// 三个数组永远存在（off 时是空数组而不是 null）：调用方不必为「没有这一段」
// 写第二条判空路径。leases_supported 回答「这一档层级有没有租约面」，
// 前端据此决定画不画租约表，而不是自己拿层级字符串去比。
type localAgentsResponse struct {
	LocalAgent      string                  `json:"local_agent"`
	Managed         []localAgentProcessJSON `json:"managed"`
	Observed        []localAgentProcessJSON `json:"observed"`
	Leases          []localAgentLeaseJSON   `json:"leases"`
	LeasesSupported bool                    `json:"leases_supported"`
	ManagedError    string                  `json:"managed_error,omitempty"`
	ObservedError   string                  `json:"observed_error,omitempty"`
	LeasesError     string                  `json:"leases_error,omitempty"`
	ScannedAt       string                  `json:"scanned_at,omitempty"`
	// Cwd 是看板进程的启动目录，给「申请租约」表单当默认资源用。
	// 它是本机路径，本来就显示在会话视图与设置页里，不算新增泄露面。
	Cwd string `json:"cwd,omitempty"`
}

// localAgentTime 是响应里的时间形状：UTC 的 RFC3339，零值给空串
// （系统没给启动时间，就不编一个 1970 年出来）。
func localAgentTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// localAgentEnumerator 是进程枚举的接缝：生产用 localagent.Enumerate，
// 测试换成一个会 panic 的实现来证明 off 路径没有枚举。
func (s *Server) localAgentEnumerator() localagent.Enumerator {
	if s.localAgentEnum != nil {
		return s.localAgentEnum
	}
	return localagent.Enumerate
}

// handleLocalAgents 是本机 agent 的只读视图。
func (s *Server) handleLocalAgents(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有本机 agent 接口：用 council serve -rw 启动才能查看")
		return
	}
	if !requireGET(w, r) {
		return
	}

	cfg := s.currentCfg()
	level := localagent.LevelOff
	if cfg != nil {
		level = localagent.LevelOf(cfg.LocalAgent)
	}
	resp := localAgentsResponse{
		LocalAgent:      string(level),
		Managed:         []localAgentProcessJSON{},
		Observed:        []localAgentProcessJSON{},
		Leases:          []localAgentLeaseJSON{},
		LeasesSupported: level.Claims(),
		Cwd:             s.localAgentCwd(),
	}
	// off：不枚举、不读库、不申请租约。响应里连 scanned_at 都不给 ——
	// 没有扫描发生，写一个时间戳会让人以为扫过了（S-01）。
	if !level.Scans() {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), localAgentScanTimeout)
	defer cancel()

	// 快照串行化：多个标签页各 3 秒轮询时，排队做四次快照没有意义。
	s.localAgentMu.Lock()
	resp.ScannedAt = localAgentTime(time.Now().UTC())
	// 一次请求只读一次库、枚举一次进程，两段拿的是同一时刻的事实。
	// 合并（MergeManaged）在本地做，不额外读库。
	turns, turnsErr := s.store.AllInflight(ctx)
	observation, observeErr := localagent.Observe(level, s.localAgentEnumerator())
	s.localAgentMu.Unlock()

	if turnsErr != nil {
		resp.ManagedError = turnsErr.Error()
	}
	if observeErr != nil {
		resp.ObservedError = observeErr.Error()
	}

	// 合并：把「这一轮我起的进程」与「进程表里的这一条」对上（Plan §4.3 ⑨）。
	// 对上的那条从 managed 段挪进 observed 段（并升级成 managed），
	// 一条实例只画一次。读不到在飞表时 refs 为空，合并自然退化成「不合并」。
	merged := localagent.MergeManaged(managedRefs(turns, s.localAgentPIDs), observation)
	resp.Managed = managedProcessRows(merged.Unmatched)
	resp.Observed = observedProcessRows(merged.Agents)

	// 租约只有 coordinate 才查：observe 的人不该被「库里还有一张你没开的表」
	// 这件事打扰，而且这省掉一次读。
	if resp.LeasesSupported {
		if leases, err := s.store.GrantedLeases(ctx); err != nil {
			resp.LeasesError = err.Error()
		} else {
			resp.Leases = grantedLeaseRows(leases, s.localAgentCwd())
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// localAgentLeaseRequest 是 POST /api/local-agents/lease 的请求体。
type localAgentLeaseRequest struct {
	// Resource 是要占的资源：目录路径或 tcp:<host>:<port>。
	Resource string `json:"resource"`
	// Mode 是 read | write，空串按 write（与 council claim 同一条规则：
	// 申请的默认意图是「我要动它」）。
	Mode string `json:"mode"`
	// TTLSeconds 是有效期，0 表示默认 120 秒。用法错误（太短/太长）回 400，
	// 不替用户夹到边界上。
	TTLSeconds int `json:"ttl_seconds"`
	// Owner 只是展示用的名字，空串按 "dashboard"。它不参与鉴权。
	Owner string `json:"owner"`
}

// localAgentLeaseResult 是申请成功（或交还成功）的响应。
//
// 冲突不走这个形状：它用仓库统一的 {"error": "..."} + 409，错误文案里带着
// 持有者与到期时间（store.LeaseConflictError 已经写好了那句话）。
type localAgentLeaseResult struct {
	Granted bool `json:"granted"`
	// LocalAgent 是本次请求时看到的层级，给 `council claim -server` 原样报告
	// 用：那个客户端不读配置，不该自己猜一个。
	LocalAgent string                `json:"local_agent,omitempty"`
	Lease      *localAgentLeaseJSON  `json:"lease,omitempty"`
	Released   []localAgentLeaseJSON `json:"released,omitempty"`
}

// localAgentLeaseReleaseRequest 是 POST /api/local-agents/lease/release 的请求体。
type localAgentLeaseReleaseRequest struct {
	LeaseID  string `json:"lease_id"`
	Resource string `json:"resource"`
	Owner    string `json:"owner"`
}

// localAgentLeaseGate 是三个闸门（只读 / 方法 / 写请求校验）加上层级判断。
// 返回 false 时响应已经写好。
//
// 层级判断用的是**当前配置**（不是启动时那份）：设置页刚把层级改成
// coordinate，这个端点立刻就能用 —— 这比「重启才生效」诚实，也不需要多一个
// 状态位。requiringClaims 为 false 的路径（交还）不看层级：关小开关不该让
// 已经发出去的授权无法收回（与 council release 同一条规则）。
func (s *Server) localAgentLeaseGate(w http.ResponseWriter, r *http.Request, requiringClaims bool) bool {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有租约接口：用 council serve -rw 启动才能申请")
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
	if !requiringClaims {
		return true
	}
	cfg := s.currentCfg()
	if cfg == nil || !localagent.LevelOf(cfg.LocalAgent).Claims() {
		writeError(w, http.StatusForbidden,
			`本机 agent 协调未开启：把设置页第 6 类的层级改成「资源协调」，或在配置里写 local_agent = "coordinate"`)
		return false
	}
	return true
}

// handleLocalAgentLease 是看板侧的资源申请（Plan §11 P3 的 POST /api/local-agents/lease）。
//
// 它与 council claim 做同一件事、走同一个 store 方法：区别只在身份
// （owner_kind = dashboard）与入口（回环 HTTP + 同源闸）。租约仍然是**自愿
// 遵守的提醒**，这里没有、也不该有「拦住别人」的能力。
func (s *Server) handleLocalAgentLease(w http.ResponseWriter, r *http.Request) {
	if !s.localAgentLeaseGate(w, r, true) {
		return
	}
	var req localAgentLeaseRequest
	if !decodeSettingsBody(w, r, &req) {
		return
	}
	resource, err := localagent.NormalizeResource(req.Resource)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	access, err := localagent.ParseAccessMode(req.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ttl, err := localagent.NormalizeTTL(time.Duration(req.TTLSeconds) * time.Second)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	owner := strings.TrimSpace(req.Owner)
	if owner == "" {
		owner = "dashboard"
	}

	ctx, cancel := context.WithTimeout(r.Context(), localAgentScanTimeout)
	defer cancel()
	lease, err := s.store.ClaimLease(ctx, store.LeaseRequest{
		ResourceKey: resource,
		OwnerKind:   store.LeaseOwnerDashboard,
		OwnerID:     owner,
		Access:      store.LeaseAccess(access),
		TTL:         ttl,
	})
	if err != nil {
		var conflict *store.LeaseConflictError
		if errors.As(err, &conflict) {
			writeError(w, http.StatusConflict, conflict.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows := grantedLeaseRows([]store.Lease{lease}, s.localAgentCwd())
	writeJSON(w, http.StatusOK, localAgentLeaseResult{
		Granted:    true,
		LocalAgent: string(localagent.LevelOf(s.localAgentLevelRaw())),
		Lease:      &rows[0],
	})
}

// localAgentLevelRaw 返回当前配置里的 local_agent 原值（给响应回显用）。
func (s *Server) localAgentLevelRaw() string {
	cfg := s.currentCfg()
	if cfg == nil {
		return ""
	}
	return cfg.LocalAgent
}

// handleLocalAgentLeaseRelease 交还一条租约（按 lease_id，或按资源 + 持有者）。
//
// 与申请不同，它**不看层级**：关小开关不该把已经发出去的授权锁死。
func (s *Server) handleLocalAgentLeaseRelease(w http.ResponseWriter, r *http.Request) {
	if !s.localAgentLeaseGate(w, r, false) {
		return
	}
	var req localAgentLeaseReleaseRequest
	if !decodeSettingsBody(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), localAgentScanTimeout)
	defer cancel()

	leaseID := strings.TrimSpace(req.LeaseID)
	resourceKey := strings.TrimSpace(req.Resource)
	switch {
	case leaseID == "" && resourceKey == "":
		writeError(w, http.StatusBadRequest, "需要 lease_id 或 resource 之一")
		return
	case leaseID != "" && resourceKey != "":
		writeError(w, http.StatusBadRequest, "lease_id 与 resource 只能给一个")
		return
	}

	var released []store.Lease
	if leaseID != "" {
		lease, changed, err := s.store.ReleaseLease(ctx, leaseID)
		if errors.Is(err, store.ErrLeaseNotFound) {
			writeError(w, http.StatusNotFound, "没有这条租约: "+leaseID)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if changed {
			released = append(released, lease)
		}
	} else {
		key, err := localagent.NormalizeResource(resourceKey)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		owner := strings.TrimSpace(req.Owner)
		if owner == "" {
			owner = "dashboard"
		}
		batch, err := s.store.ReleaseLeasesByOwner(ctx, key, owner)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		released = append(released, batch...)
	}

	rows := make([]localAgentLeaseJSON, 0, len(released))
	for _, lease := range released {
		rows = append(rows, grantedLeaseRows([]store.Lease{lease}, s.localAgentCwd())[0])
	}
	writeJSON(w, http.StatusOK, localAgentLeaseResult{Granted: false, Released: rows})
}

// managedRefs 把在飞轮折成合并输入：PID 从本进程的钩子表里查，查不到就是 0
// （那一轮照旧单独列在 managed 段）。
func managedRefs(turns []conversation.InflightTurn, pids *localagent.PIDRegistry) []localagent.ManagedRef {
	refs := make([]localagent.ManagedRef, 0, len(turns))
	for _, turn := range turns {
		pid, _ := pids.Lookup(turn.ConvID, turn.AgentID)
		refs = append(refs, localagent.ManagedRef{
			TurnID:    turn.TurnID,
			ConvID:    turn.ConvID,
			AgentID:   turn.AgentID,
			Provider:  turn.Provider,
			PID:       pid,
			StartedAt: turn.StartedAt,
			WorkDir:   turn.WorkDir,
			Phase:     string(turn.Phase),
		})
	}
	return refs
}

// localAgentCwd 是看板进程的启动目录（看板说「这个目录」时指的就是它）。
// WriteDeps 没填时返回空串，租约行就不会被标成「与当前目录重叠」。
func (s *Server) localAgentCwd() string {
	if s.write == nil {
		return ""
	}
	return s.write.Cwd
}

// printLeaseNotice 是 P5 的「serve 开跑之前检查租约冲突」。
//
// 三条边界：
//   - 只在层级允许协调（coordinate）时读租约表：关着的时候连这张表都不碰，
//     那是 S-01 的原话；
//   - 只提醒与启动目录**重叠**的写租约：读租约不影响别人写文件，报出来
//     只是噪声；
//   - 只打印、不拦截：租约拦不住任何进程写文件（Plan §8.2），把「已经提醒」
//     说成「已经保护」就是撒谎。库读不出来（v9 旧库没有这张表）时也不报错 ——
//     启动不该因为一条提示而失败。
func printLeaseNotice(ctx context.Context, st *store.Store, cfg *config.Config, cwd string, errOut io.Writer) {
	if st == nil || cfg == nil || cwd == "" || errOut == nil {
		return
	}
	if !localagent.LevelOf(cfg.LocalAgent).Claims() {
		return
	}
	leases, err := st.GrantedLeases(ctx)
	if err != nil {
		return
	}
	cwdKey, err := localagent.NormalizeResource(cwd)
	if err != nil {
		return
	}
	for _, lease := range leases {
		if lease.Access != store.LeaseWrite {
			continue
		}
		if !localagent.ResourceOverlaps(lease.ResourceKey, cwdKey) {
			continue
		}
		fmt.Fprintf(errOut,
			"council serve: 本机已有 %s/%s 在 %s 上持有写租约（到 %s，lease %s）；租约只是提醒，不会阻止任何进程写文件\n",
			lease.OwnerKind, lease.OwnerID, lease.ResourceKey,
			localAgentTime(lease.ExpiresAt), lease.ID)
	}
}

// managedProcessRows 把**没能合并进进程表**的在飞轮折成响应行。
//
// kind 用 provider（claude / codex / …）：这是 Council 自己知道的事实来源，
// 不从进程名反推。配上 PID 时也照实写出来（垫片没认出子进程、或进程已经
// 退出但 inflight 还没清，两种都值得看一眼）。
func managedProcessRows(refs []localagent.ManagedRef) []localAgentProcessJSON {
	rows := make([]localAgentProcessJSON, 0, len(refs))
	for _, ref := range refs {
		evidence := []string{"source=inflight_turns"}
		if ref.PID > 0 {
			evidence = append(evidence, fmt.Sprintf("pid=%d", ref.PID))
		}
		rows = append(rows, localAgentProcessJSON{
			Kind:           ref.Provider,
			Source:         string(localagent.SourceManaged),
			PID:            ref.PID,
			StartedAt:      localAgentTime(ref.StartedAt),
			WorkDir:        ref.WorkDir,
			Evidence:       evidence,
			ConversationID: ref.ConvID,
			AgentID:        ref.AgentID,
			TurnID:         ref.TurnID,
			Phase:          ref.Phase,
		})
	}
	return rows
}

// observedProcessRows 把规则表的识别结果折成响应行。
// evidence 里只说映像名与确定性，不落完整路径（见字段注释）。
//
// 被合并进来的那几条已经带上 turn_id / conv_id / agent_id 与合并依据：
// 它们同样是 observed 段的行，只是来源升级成了 managed —— 这样一条实例
// 在页面上只会出现一次。
func observedProcessRows(agents []localagent.Agent) []localAgentProcessJSON {
	rows := make([]localAgentProcessJSON, 0, len(agents))
	for _, agent := range agents {
		evidence := []string{"exe=" + agent.ExeName}
		if agent.Certainty != "" {
			evidence = append(evidence, "certainty="+string(agent.Certainty))
		}
		if agent.MergeEvidence != "" {
			evidence = append(evidence, "merged="+agent.MergeEvidence)
		}
		rows = append(rows, localAgentProcessJSON{
			Kind:           string(agent.Kind),
			Source:         string(agent.Source),
			Certainty:      string(agent.Certainty),
			PID:            agent.PID,
			PPID:           agent.PPID,
			ExeName:        agent.ExeName,
			StartedAt:      localAgentTime(agent.StartedAt),
			Evidence:       evidence,
			ConversationID: agent.ConversationID,
			AgentID:        agent.AgentID,
			TurnID:         agent.TurnID,
			MergeEvidence:  agent.MergeEvidence,
		})
	}
	return rows
}

// grantedLeaseRows 把租约折成响应行。只输出仍然有效的那一份事实
// （store.GrantedLeases 已经滤掉了过期的行）。
//
// cwd 非空时给每条租约标出 overlaps_cwd：看板自己正跑在这个目录里，
// 用户点「申请租约」之前就该看到「这里已经有人占了」（Plan §11 P5）。
func grantedLeaseRows(leases []store.Lease, cwd string) []localAgentLeaseJSON {
	normalizedCwd := ""
	if cwd != "" {
		if key, err := localagent.NormalizeResource(cwd); err == nil {
			normalizedCwd = key
		}
	}
	rows := make([]localAgentLeaseJSON, 0, len(leases))
	for _, lease := range leases {
		rows = append(rows, localAgentLeaseJSON{
			LeaseID:     lease.ID,
			ResourceKey: lease.ResourceKey,
			OwnerKind:   lease.OwnerKind,
			OwnerID:     lease.OwnerID,
			Mode:        string(lease.Access),
			Generation:  lease.Generation,
			GrantedAt:   localAgentTime(lease.GrantedAt),
			ExpiresAt:   localAgentTime(lease.ExpiresAt),
			OverlapsCwd: normalizedCwd != "" && localagent.ResourceOverlaps(lease.ResourceKey, normalizedCwd),
		})
	}
	return rows
}
