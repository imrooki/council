package serve

// localagent_lease_test.go 覆盖看板侧的两个租约写端点
// （POST /api/local-agents/lease 与 …/lease/release）。
//
// 它们是 council claim / release 的第二个入口，所以钉的东西与 CLI 那侧对齐：
//   - 申请要 coordinate，观察档一律 403，而且**一个字节都不写进库**；
//   - 冲突是 409 + 统一错误形状，错误里带持有者（前端直接显示这句话）；
//   - 交还不看层级：关小开关不该把已经发出去的授权锁死；
//   - 只读 405 / 非 POST 405 / 非 JSON 415，与设置页那套写闸逐条一致。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/localagent"
	"council/internal/store"
)

// leasePost 发一次租约写请求，返回状态码与响应体。
func leasePost(t *testing.T, env *serveTestEnv, path string, payload map[string]any) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("编码请求失败: %v", err)
	}
	status, _, raw := postRaw(t, env.http.URL+path, "application/json", string(body))
	return status, raw
}

// TestPostLeaseGrantsAndAppearsInView 是正向链路：申请成功之后，
// GET /api/local-agents 的租约段里就有它，并且带 overlaps_cwd。
func TestPostLeaseGrantsAndAppearsInView(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentCoordinate)

	status, raw := leasePost(t, env, "/api/local-agents/lease", map[string]any{
		"resource":    srv.write.Cwd,
		"mode":        "write",
		"ttl_seconds": 600,
	})
	if status != http.StatusOK {
		t.Fatalf("申请 = %d，期望 200；body=%s", status, raw)
	}
	var result localAgentLeaseResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("响应不是合法 JSON: %v；%s", err, raw)
	}
	if !result.Granted || result.Lease == nil {
		t.Fatalf("响应 = %+v，期望 granted 且有 lease", result)
	}
	if result.Lease.OwnerKind != store.LeaseOwnerDashboard {
		t.Errorf("owner_kind = %q，期望 dashboard", result.Lease.OwnerKind)
	}
	if result.Lease.OwnerID != "dashboard" {
		t.Errorf("owner_id = %q，期望默认 dashboard", result.Lease.OwnerID)
	}
	if result.Lease.Mode != "write" {
		t.Errorf("mode = %q，期望 write", result.Lease.Mode)
	}
	// granted_at 是给 `council claim -server` 还原租约用的：少了它那条路
	// 会解析失败（真机上就是这么发现的）。
	if result.Lease.GrantedAt == "" || result.Lease.ExpiresAt == "" {
		t.Errorf("租约行缺少时间字段: %+v", result.Lease)
	}
	if !result.Lease.OverlapsCwd {
		t.Error("申请的正是看板启动目录，overlaps_cwd 应为 true")
	}

	resp := getLocalAgents(t, env)
	if len(resp.Leases) != 1 || resp.Leases[0].LeaseID != result.Lease.LeaseID {
		t.Errorf("列表里的租约 = %+v，期望刚申请的那条", resp.Leases)
	}
	if resp.Cwd != srv.write.Cwd {
		t.Errorf("响应里的 cwd = %q，期望 %q", resp.Cwd, srv.write.Cwd)
	}
}

// TestPostLeaseRequiresCoordinate 钉住层级门：observe 下 403，且库里一条都没有。
func TestPostLeaseRequiresCoordinate(t *testing.T) {
	env, _ := newLocalAgentEnv(t, config.LocalAgentObserve)

	status, raw := leasePost(t, env, "/api/local-agents/lease", map[string]any{"resource": t.TempDir()})
	if status != http.StatusForbidden {
		t.Fatalf("observe 下申请 = %d，期望 403；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "coordinate") {
		t.Errorf("403 的说明没有告诉用户怎么开启: %s", raw)
	}
	grants, err := env.store.GrantedLeases(context.Background())
	if err != nil {
		t.Fatalf("GrantedLeases: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("被拒的申请写进了库: %+v", grants)
	}
}

// TestPostLeaseConflictIs409 钉住冲突形状：统一错误 + 409，且错误里说清持有者。
func TestPostLeaseConflictIs409(t *testing.T) {
	env, _ := newLocalAgentEnv(t, config.LocalAgentCoordinate)
	resource := t.TempDir()

	if status, raw := leasePost(t, env, "/api/local-agents/lease", map[string]any{
		"resource": resource, "owner": "first",
	}); status != http.StatusOK {
		t.Fatalf("第一次申请 = %d；body=%s", status, raw)
	}
	status, raw := leasePost(t, env, "/api/local-agents/lease", map[string]any{
		"resource": resource, "owner": "second",
	})
	if status != http.StatusConflict {
		t.Fatalf("冲突 = %d，期望 409；body=%s", status, raw)
	}
	for _, want := range []string{"first", "已被占用"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("409 的说明里没有 %q: %s", want, raw)
		}
	}
}

// TestPostLeaseValidation 覆盖三种用法错误：资源、模式、ttl。
func TestPostLeaseValidation(t *testing.T) {
	env, _ := newLocalAgentEnv(t, config.LocalAgentCoordinate)
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"空资源", map[string]any{"resource": "  "}, "资源不能为空"},
		{"端口越界", map[string]any{"resource": "tcp:70000"}, "端口"},
		{"模式写错", map[string]any{"resource": "x", "mode": "rw"}, "mode"},
		{"ttl 太短", map[string]any{"resource": "x", "ttl_seconds": 1}, "ttl"},
		{"ttl 太长", map[string]any{"resource": "x", "ttl_seconds": 100000}, "ttl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := leasePost(t, env, "/api/local-agents/lease", tc.payload)
			if status != http.StatusBadRequest {
				t.Fatalf("状态码 = %d，期望 400；body=%s", status, raw)
			}
			if !strings.Contains(string(raw), tc.want) {
				t.Errorf("说明里没有 %q: %s", tc.want, raw)
			}
		})
	}

	// 超大请求体：与设置页同一条上限（maxSettingsBody = 16 KiB），
	// 读之前就拒，不先吃完再报错。
	t.Run("超大请求体", func(t *testing.T) {
		big := strings.Repeat("a", maxSettingsBody+1)
		status, raw := leasePost(t, env, "/api/local-agents/lease", map[string]any{"resource": big})
		if status != http.StatusBadRequest {
			t.Fatalf("状态码 = %d，期望 400；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "上限") {
			t.Errorf("说明里没有提到上限: %s", raw)
		}
	})
}

// TestPostLeaseWriteGate 覆盖三道闸：只读 405、非 POST 405、非 JSON 415。
func TestPostLeaseWriteGate(t *testing.T) {
	t.Run("只读模式 405", func(t *testing.T) {
		env := newServeTestEnv(t, nil)
		status, _, raw := postRaw(t, env.http.URL+"/api/local-agents/lease", "application/json", `{"resource":"x"}`)
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("状态码 = %d，期望 405；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "只读") {
			t.Errorf("405 的说明 = %s", raw)
		}
	})

	t.Run("GET 405", func(t *testing.T) {
		env, _ := newLocalAgentEnv(t, config.LocalAgentCoordinate)
		status, body := getRaw(t, env.http.URL+"/api/local-agents/lease")
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("GET 状态码 = %d，期望 405；body=%s", status, body)
		}
	})

	t.Run("非 JSON 415", func(t *testing.T) {
		env, _ := newLocalAgentEnv(t, config.LocalAgentCoordinate)
		status, _, raw := postRaw(t, env.http.URL+"/api/local-agents/lease", "text/plain", `{"resource":"x"}`)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("状态码 = %d，期望 415；body=%s", status, raw)
		}
	})
}

// TestPostLeaseRelease 覆盖交还：按 lease_id 交还、重复交还、未知 id。
func TestPostLeaseRelease(t *testing.T) {
	env, _ := newLocalAgentEnv(t, config.LocalAgentCoordinate)
	ctx := context.Background()

	status, raw := leasePost(t, env, "/api/local-agents/lease", map[string]any{"resource": t.TempDir()})
	if status != http.StatusOK {
		t.Fatalf("申请 = %d；body=%s", status, raw)
	}
	var granted localAgentLeaseResult
	if err := json.Unmarshal(raw, &granted); err != nil || granted.Lease == nil {
		t.Fatalf("申请响应解析失败: %v；%s", err, raw)
	}

	status, raw = leasePost(t, env, "/api/local-agents/lease/release", map[string]any{"lease_id": granted.Lease.LeaseID})
	if status != http.StatusOK {
		t.Fatalf("交还 = %d，期望 200；body=%s", status, raw)
	}
	var released localAgentLeaseResult
	if err := json.Unmarshal(raw, &released); err != nil {
		t.Fatalf("交还响应解析失败: %v；%s", err, raw)
	}
	if released.Granted || len(released.Released) != 1 || released.Released[0].LeaseID != granted.Lease.LeaseID {
		t.Errorf("交还响应 = %+v", released)
	}
	if row, err := env.store.LeaseEvents(ctx, granted.Lease.LeaseID); err != nil || len(row) != 2 {
		t.Errorf("事件 = %+v, err=%v；期望 granted + released", row, err)
	}

	// 重复交还：200 + 空列表（不是错误）。
	status, raw = leasePost(t, env, "/api/local-agents/lease/release", map[string]any{"lease_id": granted.Lease.LeaseID})
	if status != http.StatusOK {
		t.Fatalf("重复交还 = %d，期望 200；body=%s", status, raw)
	}
	var again localAgentLeaseResult
	if err := json.Unmarshal(raw, &again); err != nil {
		t.Fatalf("重复交还解析失败: %v；%s", err, raw)
	}
	if len(again.Released) != 0 {
		t.Errorf("重复交还不该再有 released: %+v", again.Released)
	}

	// 未知 id：404。
	status, raw = leasePost(t, env, "/api/local-agents/lease/release", map[string]any{"lease_id": "no-such"})
	if status != http.StatusNotFound {
		t.Errorf("未知 id 交还 = %d，期望 404；body=%s", status, raw)
	}

	// 两个目标都给：400。
	if status, raw := leasePost(t, env, "/api/local-agents/lease/release", map[string]any{
		"lease_id": "x", "resource": "y",
	}); status != http.StatusBadRequest {
		t.Errorf("两个目标 = %d，期望 400；body=%s", status, raw)
	}
}

// TestPostLeaseReleaseSurvivesLevelOff 钉住「关小开关不锁死授权」：
// 申请之后把层级改回 off，交还仍然成功。
func TestPostLeaseReleaseSurvivesLevelOff(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentCoordinate)
	status, raw := leasePost(t, env, "/api/local-agents/lease", map[string]any{"resource": t.TempDir()})
	if status != http.StatusOK {
		t.Fatalf("申请 = %d；body=%s", status, raw)
	}
	var granted localAgentLeaseResult
	if err := json.Unmarshal(raw, &granted); err != nil || granted.Lease == nil {
		t.Fatalf("申请响应解析失败: %v；%s", err, raw)
	}

	srv.cfgMu.Lock()
	srv.write.Cfg.LocalAgent = config.LocalAgentOff
	srv.cfgMu.Unlock()

	status, raw = leasePost(t, env, "/api/local-agents/lease/release", map[string]any{"lease_id": granted.Lease.LeaseID})
	if status != http.StatusOK {
		t.Fatalf("层级关掉后交还 = %d，期望 200；body=%s", status, raw)
	}
	var released localAgentLeaseResult
	if err := json.Unmarshal(raw, &released); err != nil || len(released.Released) != 1 {
		t.Errorf("交还结果 = %+v, err=%v", released, err)
	}
}

// TestPostLeaseByResourceAndOwner 覆盖按资源交还：只释放自己名字的那条。
func TestPostLeaseByResourceAndOwner(t *testing.T) {
	env, _ := newLocalAgentEnv(t, config.LocalAgentCoordinate)
	ctx := context.Background()
	resource := t.TempDir()
	key, err := localagent.NormalizeResource(resource)
	if err != nil {
		t.Fatalf("规范化资源失败: %v", err)
	}

	// 两条 read 租约（可共存），分属两个人。
	for _, owner := range []string{"me", "other"} {
		if status, raw := leasePost(t, env, "/api/local-agents/lease", map[string]any{
			"resource": resource, "mode": "read", "owner": owner,
		}); status != http.StatusOK {
			t.Fatalf("申请(%s) = %d；body=%s", owner, status, raw)
		}
	}

	status, raw := leasePost(t, env, "/api/local-agents/lease/release", map[string]any{
		"resource": resource, "owner": "me",
	})
	if status != http.StatusOK {
		t.Fatalf("按资源交还 = %d；body=%s", status, raw)
	}
	var released localAgentLeaseResult
	if err := json.Unmarshal(raw, &released); err != nil {
		t.Fatalf("解析失败: %v；%s", err, raw)
	}
	if len(released.Released) != 1 || released.Released[0].OwnerID != "me" {
		t.Errorf("交还了 %+v，期望只有 me 那条", released.Released)
	}
	grants, err := env.store.GrantedLeases(ctx)
	if err != nil {
		t.Fatalf("GrantedLeases: %v", err)
	}
	if len(grants) != 1 || grants[0].OwnerID != "other" {
		t.Errorf("剩余租约 = %+v，期望只有 other 那条（资源键 %s）", grants, key)
	}
}

// TestPrintLeaseNotice 覆盖 P5 的启动提醒：只在 coordinate 下、只对写租约、
// 只对与启动目录重叠的那些说话。
func TestPrintLeaseNotice(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentCoordinate)
	ctx := context.Background()
	cwd := srv.write.Cwd

	writeLease, err := env.store.ClaimLease(ctx, store.LeaseRequest{
		ResourceKey: mustKey(t, cwd),
		OwnerKind:   store.LeaseOwnerCLI,
		OwnerID:     "someone",
		Access:      store.LeaseWrite,
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("写租约: %v", err)
	}
	// 读租约与不相干目录的写租约都不该出现在提醒里。读租约要放在另一个
	// 资源上：同一个目录上写完再读本来就是冲突（读共享、写独占）。
	for _, req := range []store.LeaseRequest{
		{ResourceKey: mustKey(t, t.TempDir()), OwnerKind: store.LeaseOwnerCLI, OwnerID: "reader", Access: store.LeaseRead, TTL: time.Hour},
		{ResourceKey: mustKey(t, t.TempDir()), OwnerKind: store.LeaseOwnerCLI, OwnerID: "elsewhere", Access: store.LeaseWrite, TTL: time.Hour},
	} {
		if _, err := env.store.ClaimLease(ctx, req); err != nil {
			t.Fatalf("ClaimLease(%+v): %v", req, err)
		}
	}

	var buf strings.Builder
	printLeaseNotice(ctx, env.store, srv.write.Cfg, cwd, &buf)
	out := buf.String()
	if !strings.Contains(out, writeLease.ID) || !strings.Contains(out, "someone") {
		t.Errorf("提醒里没有写租约: %s", out)
	}
	if strings.Contains(out, "reader") || strings.Contains(out, "elsewhere") {
		t.Errorf("提醒里混进了读租约或别的目录: %s", out)
	}
	if !strings.Contains(out, "不会阻止任何进程写文件") {
		t.Errorf("提醒没有说清边界: %s", out)
	}

	// observe：一句都不打（连库都不读）。
	buf.Reset()
	observeCfg := *srv.write.Cfg
	observeCfg.LocalAgent = config.LocalAgentObserve
	printLeaseNotice(ctx, env.store, &observeCfg, cwd, &buf)
	if buf.Len() != 0 {
		t.Errorf("observe 下不该打印租约提醒: %s", buf.String())
	}
}

// mustKey 是测试里的 NormalizeResource 断言版。
func mustKey(t *testing.T, raw string) string {
	t.Helper()
	key, err := localagent.NormalizeResource(raw)
	if err != nil {
		t.Fatalf("NormalizeResource(%q): %v", raw, err)
	}
	return key
}

var _ = fmt.Sprintf
