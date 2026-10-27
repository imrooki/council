package main

// leaseclient.go 是 `council claim -server` / `council release -server` 的
// 回环 HTTP 客户端。
//
// 为什么需要它：租约存在 council.db 里，而一个 data_dir 同时只有一个写进程。
// 桌面壳、或者你自己开着的 `serve -rw`，正握着 council.lock 时，命令行走路
// 数据库那条路一定失败 —— 但那个进程里的看板**本来就有**写权限，跑的也是
// 同一套租约逻辑。于是这里复用它：显式给出地址，POST 到
// /api/local-agents/lease。
//
// 四条边界：
//   - **只连回环地址**：这是本机事实，发到远端主机既没有意义也不该允许；
//   - **必须显式给 -server**：自动探测端口会打到不相干的本机服务上，而
//     「走谁的库」这件事不该由程序猜；
//   - **不伪造同源头**：看板的写闸只挡跨站**浏览器**请求（checkWriteOrigin
//     对缺失的 Origin / Sec-Fetch-Site 放行，那是给非浏览器客户端留的），
//     所以这里不带 Origin 是正常调用，不是绕过；
//   - **不改语义**：走 HTTP 与走数据库拿到的租约、退出码、文案完全一致，
//     唯一的差别是 `-json` 里多一个 "via":"server"。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"council/internal/localagent"
	"council/internal/store"
)

// leaseServerTimeFormat 与看板响应里的时间格式一致（UTC 的 RFC3339）。
const leaseServerTimeFormat = time.RFC3339

// leaseWireRow 是看板返回的一行租约。
//
// 与 internal/serve 的 localAgentLeaseJSON 字段一一对应 —— 这里刻意不 import
// serve：命令行不该因为「打印一行租约」而依赖整个 Web 层（who 当初就是因为
// 同一个理由不 import serve）。字段名就是契约，serve 侧改了这边会解析失败，
// 用例会红。
type leaseWireRow struct {
	LeaseID     string `json:"lease_id"`
	ResourceKey string `json:"resource_key"`
	OwnerKind   string `json:"owner_kind"`
	OwnerID     string `json:"owner_id"`
	Mode        string `json:"mode"`
	Generation  int64  `json:"generation"`
	GrantedAt   string `json:"granted_at"`
	ExpiresAt   string `json:"expires_at"`
}

// leaseWire 是申请 / 交还两个端点的响应形状（交还时 granted=false、released 非空）。
type leaseWire struct {
	Granted    bool           `json:"granted"`
	LocalAgent string         `json:"local_agent,omitempty"`
	Lease      *leaseWireRow  `json:"lease,omitempty"`
	Released   []leaseWireRow `json:"released,omitempty"`
}

// leaseEndpointError 是看板错误响应的形状（仓库统一的 {"error": "..."}）。
type leaseEndpointError struct {
	Error string `json:"error"`
}

// toStoreLease 把线上的一行还原成 store.Lease，让两个入口共用同一套打印。
//
// State 由调用方给：申请成功是 granted，交还成功是 released。心跳时间没有
// 传（打印用不到），留零值。
func (w leaseWireRow) toStoreLease(state store.LeaseState) (store.Lease, error) {
	grantedAt, err := time.Parse(leaseServerTimeFormat, w.GrantedAt)
	if err != nil {
		return store.Lease{}, fmt.Errorf("租约 %s 的 granted_at 不合法（%q）: %w", w.LeaseID, w.GrantedAt, err)
	}
	expiresAt, err := time.Parse(leaseServerTimeFormat, w.ExpiresAt)
	if err != nil {
		return store.Lease{}, fmt.Errorf("租约 %s 的 expires_at 不合法（%q）: %w", w.LeaseID, w.ExpiresAt, err)
	}
	return store.Lease{
		ID:          w.LeaseID,
		ResourceKey: w.ResourceKey,
		OwnerKind:   w.OwnerKind,
		OwnerID:     w.OwnerID,
		Access:      store.LeaseAccess(w.Mode),
		Generation:  w.Generation,
		State:       state,
		GrantedAt:   grantedAt,
		ExpiresAt:   expiresAt,
	}, nil
}

// normalizeLeaseServer 校验 -server 的取值并去掉末尾斜杠。
//
// 只放行 http/https 且主机是回环（localhost、127.0.0.0/8、::1）。这条检查
// 不是形式主义：租约是本机事实，把它 POST 到别的机器上既没有意义，也会让
// 「同一台机器上的 agent 互相避让」这个前提直接失效。
func normalizeLeaseServer(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", fmt.Errorf("-server 不能为空（例如 http://127.0.0.1:8765）")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("-server %q 不是合法 URL: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("-server %q 的协议必须是 http 或 https", raw)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("-server %q 没有主机名（例如 http://127.0.0.1:8765）", raw)
	}
	if !isLoopbackHost(host) {
		return "", fmt.Errorf("-server %q 不是回环地址：租约只在本机有意义，不往别的机器发", raw)
	}
	return trimmed, nil
}

// isLoopbackHost 判断主机名是否是本机回环。
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// postLeaseJSON 发一次写请求，返回状态码与响应体。
//
// 非 2xx 也把 body 交回去：看板的错误文案（冲突时带持有者与到期时间）就在
// 那个 body 里，调用方要原样显示它，不要自己编一句。
func postLeaseJSON(ctx context.Context, server, path string, payload any) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("编码请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: leaseTimeout}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return res.StatusCode, nil, fmt.Errorf("读响应失败: %w", err)
	}
	return res.StatusCode, raw, nil
}

// decodeLeaseError 取出看板错误响应里的那句话；取不到就返回原文。
func decodeLeaseError(raw []byte) string {
	var payload leaseEndpointError
	if err := json.Unmarshal(raw, &payload); err == nil && strings.TrimSpace(payload.Error) != "" {
		return strings.TrimSpace(payload.Error)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "（看板没有返回内容）"
	}
	return text
}

// whoSnapshot 是 GET /api/local-agents 的响应，字段与 internal/serve 的
// localAgentsResponse 一一对应（同样刻意不 import serve：命令行不该因为
// 「读一份快照」而依赖整个 Web 层）。
//
// 为什么 `council who` 需要它：PID 合并只发生在持锁的那个进程里（⑨ 的
// 钩子表在内存里），跨进程读库无论如何都拿不到。要看合并结果，就得问它。
type whoSnapshot struct {
	LocalAgent    string               `json:"local_agent"`
	Managed       []whoSnapshotProcess `json:"managed"`
	Observed      []whoSnapshotProcess `json:"observed"`
	Leases        []leaseWireRow       `json:"leases"`
	LeasesSupport bool                 `json:"leases_supported"`
	ManagedError  string               `json:"managed_error"`
	ObservedError string               `json:"observed_error"`
	LeasesError   string               `json:"leases_error"`
	ScannedAt     string               `json:"scanned_at"`
	Cwd           string               `json:"cwd"`
}

// whoSnapshotProcess 是快照里的一行进程。
type whoSnapshotProcess struct {
	Kind           string   `json:"kind"`
	Source         string   `json:"source"`
	Certainty      string   `json:"certainty"`
	PID            int      `json:"pid"`
	PPID           int      `json:"ppid"`
	ExeName        string   `json:"exe_name"`
	StartedAt      string   `json:"started_at"`
	WorkDir        string   `json:"work_dir"`
	Evidence       []string `json:"evidence"`
	ConversationID string   `json:"conversation_id"`
	AgentID        string   `json:"agent_id"`
	TurnID         string   `json:"turn_id"`
	Phase          string   `json:"phase"`
	MergeEvidence  string   `json:"merge_evidence"`
}

// getWhoSnapshot 向看板要一份本机 agent 快照（只读 GET，不过写闸）。
func getWhoSnapshot(ctx context.Context, server string) (whoSnapshot, error) {
	var snapshot whoSnapshot
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/local-agents", nil)
	if err != nil {
		return snapshot, err
	}
	client := &http.Client{Timeout: leaseTimeout}
	res, err := client.Do(req)
	if err != nil {
		return snapshot, err
	}
	defer func() {
		_ = res.Body.Close()
	}()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return snapshot, fmt.Errorf("读响应失败: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return snapshot, fmt.Errorf("看板返回 %d：%s", res.StatusCode, decodeLeaseError(raw))
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return snapshot, fmt.Errorf("看板返回的 JSON 看不懂: %w", err)
	}
	return snapshot, nil
}

// runWhoOverServer 是 `council who -server` 的实现：拿一份快照，按与直读本机
// 完全相同的三段格式打印。
//
// 为什么值得多这一条路：PID 合并只发生在持锁的那个进程里（⑨ 的钩子表在
// 内存），跨进程读库永远看不到「这一轮对应哪个进程」。快照里有。
func runWhoOverServer(out, errOut io.Writer, text func(key, fallback string, args ...any) string,
	server string, asJSON bool) int {
	ctx, cancel := context.WithTimeout(context.Background(), whoScanTimeout)
	defer cancel()

	snapshot, err := getWhoSnapshot(ctx, server)
	if err != nil {
		fmt.Fprint(errOut, text("cli.who.serverFailed",
			"council who: 问不到 %s 的快照：%v（地址就是看板启动时打印的那一行）\n", server, err))
		return exitConfigFailed
	}

	managed := make([]whoManagedRow, 0, len(snapshot.Managed))
	for _, row := range snapshot.Managed {
		managed = append(managed, whoManagedRow{
			ConversationID: row.ConversationID,
			AgentID:        row.AgentID,
			TurnID:         row.TurnID,
			Provider:       row.Kind,
			Phase:          row.Phase,
			StartedAt:      row.StartedAt,
			WorkDir:        row.WorkDir,
			Source:         row.Source,
		})
	}
	observed := make([]whoObservedRow, 0, len(snapshot.Observed))
	for _, row := range snapshot.Observed {
		observed = append(observed, whoObservedRow{
			Kind:           row.Kind,
			PID:            row.PID,
			PPID:           row.PPID,
			ExeName:        row.ExeName,
			StartedAt:      row.StartedAt,
			Source:         row.Source,
			Certainty:      row.Certainty,
			MergeEvidence:  row.MergeEvidence,
			ConversationID: row.ConversationID,
			AgentID:        row.AgentID,
			TurnID:         row.TurnID,
		})
	}
	leases := make([]whoLeaseRow, 0, len(snapshot.Leases))
	for _, row := range snapshot.Leases {
		leases = append(leases, whoLeaseRow{
			LeaseID:     row.LeaseID,
			ResourceKey: row.ResourceKey,
			OwnerKind:   row.OwnerKind,
			OwnerID:     row.OwnerID,
			Mode:        row.Mode,
			Generation:  row.Generation,
			ExpiresAt:   row.ExpiresAt,
		})
	}

	report := whoReport{
		LocalAgent: snapshot.LocalAgent,
		Managed:    managed,
		Observed:   observed,
		Leases:     leases,
		Via:        "server",
	}
	// 快照自己带的三段错误照搬：看板那边读库失败时，这里也不该显示成「没有」。
	report.ManagedError = snapshot.ManagedError
	report.ObservedError = snapshot.ObservedError
	report.LeasesError = snapshot.LeasesError

	if asJSON {
		return writeWhoJSON(out, errOut, report)
	}
	// 文本路径沿用同一个 writeWhoText：三段格式、空段提示、错误提示都只有
	// 一份实现。错误在这里被折成 error，文本段按原样打印那些句子。
	view := whoView{
		text:       text,
		managed:    report.Managed,
		agents:     report.Observed,
		leases:     report.Leases,
		servedFrom: server,
	}
	if report.ManagedError != "" {
		view.managedErr = errors.New(report.ManagedError)
	}
	if report.ObservedError != "" {
		view.observedErr = errors.New(report.ObservedError)
	}
	if report.LeasesError != "" {
		view.leasesErr = errors.New(report.LeasesError)
	}
	writeWhoText(out, view)
	return exitOK
}

// runClaimOverServer 是 `council claim -server` 的实现。
//
// 退出码与直接写库那条路保持一致：0 拿到、3 被拒（占用 / 未开启）、2 用法、
// 1 出错。看板已经把「为什么被拒」写成人话了，这里原样转述，不自己再编一句。
func runClaimOverServer(out, errOut io.Writer, text func(key, fallback string, args ...any) string,
	server, resource string, access localagent.AccessMode, ttl time.Duration, owner string, asJSON bool) int {
	ctx, cancel := context.WithTimeout(context.Background(), leaseTimeout)
	defer cancel()

	status, raw, err := postLeaseJSON(ctx, server, "/api/local-agents/lease", map[string]any{
		"resource":    resource,
		"mode":        string(access),
		"ttl_seconds": int(ttl / time.Second),
		"owner":       owner,
	})
	if err != nil {
		fmt.Fprint(errOut, text("cli.lease.serverFailed",
			"%s: 连不上 %s：%v（地址就是看板启动时打印的那一行）\n", "council claim", server, err))
		return exitConfigFailed
	}

	switch status {
	case http.StatusOK:
		var wire leaseWire
		if err := json.Unmarshal(raw, &wire); err != nil || wire.Lease == nil {
			fmt.Fprint(errOut, text("cli.lease.serverBadReply",
				"%s: 看板返回了看不懂的内容: %s\n", "council claim", string(raw)))
			return exitConfigFailed
		}
		lease, err := wire.Lease.toStoreLease(store.LeaseGranted)
		if err != nil {
			fmt.Fprint(errOut, text("cli.lease.serverBadReply",
				"%s: 看板返回了看不懂的内容: %v\n", "council claim", err))
			return exitConfigFailed
		}
		if asJSON {
			row := toLeaseRow(lease)
			return writeJSONReport(out, errOut, "council claim", claimReport{
				Granted:    true,
				LocalAgent: wire.LocalAgent,
				Resource:   lease.ResourceKey,
				Mode:       string(lease.Access),
				Owner:      owner,
				Via:        "server",
				Lease:      &row,
			}, exitOK)
		}
		printClaimGranted(out, text, lease)
		return exitOK

	case http.StatusConflict:
		notice := decodeLeaseError(raw)
		if asJSON {
			return writeJSONReport(out, errOut, "council claim", claimReport{
				Granted:  false,
				Resource: resource,
				Mode:     string(access),
				Owner:    owner,
				Via:      "server",
				Denied:   "busy",
				Notice:   notice,
			}, exitLeaseDenied)
		}
		fmt.Fprint(errOut, text("cli.claim.busyServer", "资源被占用，没有拿到租约：\n  %s\n", notice))
		return exitLeaseDenied

	case http.StatusForbidden:
		// 层级没开（看板那句已经说了怎么开）。跨站闸不会命中：本客户端不带 Origin。
		notice := decodeLeaseError(raw)
		if asJSON {
			return writeJSONReport(out, errOut, "council claim", claimReport{
				Granted:  false,
				Resource: resource,
				Mode:     string(access),
				Owner:    owner,
				Via:      "server",
				Denied:   "not_enabled",
				Notice:   notice,
			}, exitLeaseDenied)
		}
		fmt.Fprintln(errOut, notice)
		return exitLeaseDenied

	case http.StatusBadRequest:
		fmt.Fprintln(errOut, decodeLeaseError(raw))
		return exitUsage
	}
	fmt.Fprint(errOut, text("cli.claim.failed", "council claim: %v\n", decodeLeaseError(raw)))
	return exitConfigFailed
}

// runReleaseOverServer 是 `council release -server` 的实现。
func runReleaseOverServer(out, errOut io.Writer, text func(key, fallback string, args ...any) string,
	server, leaseID, resource, owner string, asJSON bool) int {
	ctx, cancel := context.WithTimeout(context.Background(), leaseTimeout)
	defer cancel()

	payload := map[string]any{}
	if leaseID != "" {
		payload["lease_id"] = leaseID
	} else {
		payload["resource"] = resource
		payload["owner"] = owner
	}

	status, raw, err := postLeaseJSON(ctx, server, "/api/local-agents/lease/release", payload)
	if err != nil {
		fmt.Fprint(errOut, text("cli.lease.serverFailed",
			"%s: 连不上 %s：%v（地址就是看板启动时打印的那一行）\n", "council release", server, err))
		return exitConfigFailed
	}

	switch status {
	case http.StatusOK:
		var wire leaseWire
		if err := json.Unmarshal(raw, &wire); err != nil {
			fmt.Fprint(errOut, text("cli.lease.serverBadReply",
				"%s: 看板返回了看不懂的内容: %s\n", "council release", string(raw)))
			return exitConfigFailed
		}
		rows := make([]leaseRow, 0, len(wire.Released))
		for _, item := range wire.Released {
			lease, err := item.toStoreLease(store.LeaseReleased)
			if err != nil {
				fmt.Fprint(errOut, text("cli.lease.serverBadReply",
					"%s: 看板返回了看不懂的内容: %v\n", "council release", err))
				return exitConfigFailed
			}
			rows = append(rows, toLeaseRow(lease))
		}
		if len(rows) == 0 {
			// 与直接写库那条路同义：按 id 交还且已经交还过 → 0；
			// 按资源交还却一条都没匹配到 → 3（没做成）。
			notice := text("cli.release.nothingServer",
				"看板那边没有可交还的租约（可能已经交还过，或 owner 不匹配）。")
			if asJSON {
				code := exitOK
				if leaseID == "" {
					code = exitLeaseDenied
				}
				return writeJSONReport(out, errOut, "council release", releaseReport{
					LocalAgent: wire.LocalAgent,
					Resource:   resource,
					Via:        "server",
					Released:   []leaseRow{},
					Notice:     notice,
				}, code)
			}
			if leaseID == "" {
				fmt.Fprintln(errOut, notice)
				return exitLeaseDenied
			}
			fmt.Fprintln(out, notice)
			return exitOK
		}
		if asJSON {
			return writeJSONReport(out, errOut, "council release", releaseReport{
				LocalAgent: wire.LocalAgent,
				Resource:   resource,
				Via:        "server",
				Released:   rows,
			}, exitOK)
		}
		printReleaseRows(out, text, rows)
		return exitOK

	case http.StatusNotFound:
		notice := decodeLeaseError(raw)
		if asJSON {
			return writeJSONReport(out, errOut, "council release", releaseReport{
				Resource: resource,
				Via:      "server",
				Released: []leaseRow{},
				Notice:   notice,
			}, exitLeaseDenied)
		}
		fmt.Fprintln(errOut, notice)
		return exitLeaseDenied

	case http.StatusBadRequest:
		fmt.Fprintln(errOut, decodeLeaseError(raw))
		return exitUsage
	}
	fmt.Fprint(errOut, text("cli.release.failed", "council release: %v\n", decodeLeaseError(raw)))
	return exitConfigFailed
}
