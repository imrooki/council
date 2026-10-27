package serve

// localagent_test.go 覆盖 GET /api/local-agents（光在设置页第 6 类里画实例表
// 还不会让这条路径被证明过）：
//
//   - off 时**一次枚举都不做**，而且连在飞成员都不读：库里明明有一条在飞的
//     turn，响应里 managed 也必须是空表（S-01）；
//   - observe 时两段并列出现，managed 没有 PID（钩子未做）、observed 有；
//   - coordinate 时才查租约，observe 不查；
//   - 扫描失败不是 500，而是 200 + observed_error（与 /api/models 同构）；
//   - 只读模式 405，非 GET 405；
//   - 响应里没有完整映像路径（S-09）。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/localagent"
	"council/internal/store"
	"council/internal/turn"
)

// newTestHTTPServer 把一个 handler 挂到 httptest 上并在测试结束时关掉。
func newTestHTTPServer(t *testing.T, srv *Server) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// getRaw 发一次 GET，返回状态码与原始响应体（形状类断言要看原文）。
func getRaw(t *testing.T, url string) (int, []byte) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s 失败: %v", url, err)
	}
	defer func() {
		if cerr := res.Body.Close(); cerr != nil {
			t.Errorf("关闭 %s 的响应体失败: %v", url, cerr)
		}
	}()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读 %s 的响应失败: %v", url, err)
	}
	return res.StatusCode, body
}

// newLocalAgentEnv 造一份能过 Validate 的最小配置（带指定层级）、一个真库，
// 并按 runWritable 的顺序装配写模式 server。返回的 server 供测试换枚举接缝。
func newLocalAgentEnv(t *testing.T, level string) (*serveTestEnv, *Server) {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := filepath.Join(dir, "config.toml")
	body := fmt.Sprintf(`data_dir = '%s'
local_agent = '%s'

[providers.claude]
command = '%s'

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
permissions = "read"
mode = "discussion"
`, dataDir, level, os.Args[0])
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}

	cfg, err := config.LoadAndValidate(cfgPath)
	if err != nil {
		t.Fatalf("加载测试配置失败: %v", err)
	}
	rules, err := config.LoadRules(cfgPath, cfg.RulesFile)
	if err != nil {
		t.Fatalf("加载测试规则失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试库失败: %v", err)
		}
	})

	svc := conversation.NewService(st)
	rn := turn.NewRunner(cfg, svc, filepath.Join(dataDir, "workdir"), turn.RulesBlocks{
		Global: rules.GlobalText,
		Local:  rules.LocalText,
	})
	srv := NewServer(st, "test-version", nil)
	srv.EnableWrite(WriteDeps{
		Cfg:        cfg,
		Svc:        svc,
		Rn:         rn,
		DataDir:    dataDir,
		ConfigPath: cfgPath,
		Layers:     []config.Layer{{Path: cfgPath}},
		// Cwd 与生产一致：看板进程的启动目录。租约行的 overlaps_cwd 用它比对。
		Cwd: dir,
	})
	ts := newTestHTTPServer(t, srv)
	return &serveTestEnv{store: st, http: ts}, srv
}

// seedInflight 写一条在飞的 turn：off 用例要用它证明「即使库里有记录，
// 关闭时也不会去读」。返回会话 id 与这一轮的开始时间，合并用例要拿它们
// 造同一条进程事实。
func seedInflight(t *testing.T, st *store.Store) (convID string, started time.Time) {
	t.Helper()
	ctx := context.Background()
	started = time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)
	err := st.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		conv, err := tx.CreateConversation(ctx, "本机 agent 测试会话")
		if err != nil {
			return err
		}
		convID = conv.ID
		return tx.BeginTurn(ctx, conversation.InflightTurn{
			TurnID:    "turn-localagent-1",
			ConvID:    conv.ID,
			AgentID:   "c1",
			Provider:  "claude",
			CursorSeq: 0,
			ScannedTo: 0,
			StartedAt: started,
			Phase:     conversation.PhaseBegun,
			Tier:      "read",
			WorkDir:   `D:\work\proj`,
		})
	})
	if err != nil {
		t.Fatalf("写在飞记录失败: %v", err)
	}
	return convID, started
}

// getLocalAgents 读一次接口并把响应解出来。
func getLocalAgents(t *testing.T, env *serveTestEnv) localAgentsResponse {
	t.Helper()
	var resp localAgentsResponse
	getJSON(t, env.http.URL+"/api/local-agents", http.StatusOK, &resp)
	return resp
}

// TestLocalAgentsOffSkipsEverything 是本片最重要的一条：层级 off 时既枚举
// 进程，也不读库 —— 库里有一条在飞记录，响应里的 managed 仍然空。
func TestLocalAgentsOffSkipsEverything(t *testing.T) {
	// 子测试名不能为空：t.TempDir() 会把测试名拼进路径，而路径里的 # 会被
	// SQLite 的 file: URI 当成片段起点（store 侧已修，这里也不给自己找事）。
	for _, tc := range []struct {
		name, level string
	}{
		{"显式 off", config.LocalAgentOff},
		{"空串", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, srv := newLocalAgentEnv(t, tc.level)
			seedInflight(t, env.store)
			srv.localAgentEnum = func() ([]localagent.Process, error) {
				t.Fatal("层级不是 observe / coordinate 时不许枚举进程（S-01）")
				return nil, nil
			}

			resp := getLocalAgents(t, env)
			if resp.LocalAgent != string(localagent.LevelOff) {
				t.Errorf("local_agent = %q，期望 off（未实现的层级按 off 处理）", resp.LocalAgent)
			}
			if len(resp.Managed) != 0 || resp.Managed == nil {
				t.Errorf("managed = %+v，期望空数组（off 时不读库）", resp.Managed)
			}
			if len(resp.Observed) != 0 || resp.Observed == nil {
				t.Errorf("observed = %+v，期望空数组", resp.Observed)
			}
			if len(resp.Leases) != 0 || resp.Leases == nil {
				t.Errorf("leases = %+v，期望空数组", resp.Leases)
			}
			if resp.LeasesSupported {
				t.Error("off 时不该声称支持租约")
			}
			if resp.ScannedAt != "" {
				t.Errorf("scanned_at = %q，期望空串（没有扫描发生就不要写时间戳）", resp.ScannedAt)
			}
		})
	}

	// 未来层级（register）写在配置里过不了 Validate，但直接构造的 Config
	// 可以带进来：那一侧必须 fail closed 成 off，不能当成 observe 打开采集面。
	t.Run("未知层级也按 off", func(t *testing.T) {
		env, srv := newLocalAgentEnv(t, config.LocalAgentOff)
		seedInflight(t, env.store)
		srv.cfgMu.Lock()
		srv.write.Cfg.LocalAgent = "register"
		srv.cfgMu.Unlock()
		srv.localAgentEnum = func() ([]localagent.Process, error) {
			t.Fatal("未知层级不许枚举进程")
			return nil, nil
		}
		resp := getLocalAgents(t, env)
		if resp.LocalAgent != string(localagent.LevelOff) {
			t.Errorf("local_agent = %q，期望折成 off", resp.LocalAgent)
		}
		if len(resp.Managed) != 0 {
			t.Errorf("未知层级不该读库: %+v", resp.Managed)
		}
	})
}

// TestLocalAgentsObserveListsBothSections 覆盖 observe：managed 来自在飞表、
// observed 来自进程枚举，两段并列、不靠 PID 合并。
func TestLocalAgentsObserveListsBothSections(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentObserve)
	seedInflight(t, env.store)

	started := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	calls := 0
	srv.localAgentEnum = func() ([]localagent.Process, error) {
		calls++
		return []localagent.Process{
			{PID: 300, PPID: 200, ExeName: "claude.exe", StartedAt: started},
			{PID: 301, PPID: 1, ExeName: "codex.exe"},
			{PID: 302, PPID: 1, ExeName: "node.exe"},
		}, nil
	}

	resp := getLocalAgents(t, env)
	if calls != 1 {
		t.Errorf("枚举次数 = %d，期望 1", calls)
	}
	if resp.LocalAgent != string(localagent.LevelObserve) {
		t.Errorf("local_agent = %q，期望 observe", resp.LocalAgent)
	}

	if len(resp.Managed) != 1 {
		t.Fatalf("managed = %+v，期望 1 行", resp.Managed)
	}
	managed := resp.Managed[0]
	if managed.Source != string(localagent.SourceManaged) || managed.Kind != "claude" {
		t.Errorf("managed 行 = %+v，期望 source=managed kind=claude", managed)
	}
	if managed.PID != 0 {
		t.Errorf("managed 行的 pid = %d，期望 0（钩子未做，不许编）", managed.PID)
	}
	if managed.TurnID != "turn-localagent-1" || managed.AgentID != "c1" || managed.WorkDir != `D:\work\proj` {
		t.Errorf("managed 行身份字段 = %+v", managed)
	}
	if managed.StartedAt != "2026-10-27T09:30:00Z" {
		t.Errorf("managed.started_at = %q，期望 UTC RFC3339", managed.StartedAt)
	}
	if len(managed.Evidence) != 1 || managed.Evidence[0] != "source=inflight_turns" {
		t.Errorf("managed.evidence = %v", managed.Evidence)
	}

	// 三个进程里只有两个是四家 CLI：node.exe 不确认（L1 的规则表）。
	if len(resp.Observed) != 2 {
		t.Fatalf("observed = %+v，期望 2 行（node.exe 不算）", resp.Observed)
	}
	first := resp.Observed[0]
	if first.Kind != "claude" || first.PID != 300 || first.Certainty == "" {
		t.Errorf("第一行 = %+v，期望 claude/pid=300 带确定性", first)
	}
	if first.StartedAt != "2026-10-27T10:00:00Z" {
		t.Errorf("observed.started_at = %q", first.StartedAt)
	}
	if !containsString(first.Evidence, "exe=claude.exe") {
		t.Errorf("evidence = %v，期望含 exe=claude.exe", first.Evidence)
	}
	for _, row := range resp.Observed {
		if row.Source != string(localagent.SourceObserved) {
			t.Errorf("observed 行的 source = %q", row.Source)
		}
		if row.WorkDir != "" {
			t.Errorf("observed 行不该有 work_dir（拿不到 cwd，不猜）: %+v", row)
		}
	}
	if resp.LeasesSupported {
		t.Error("observe 不该声称支持租约：申请要 coordinate")
	}
	if len(resp.Leases) != 0 {
		t.Errorf("observe 不该查租约，却返回 %+v", resp.Leases)
	}
	if resp.ScannedAt == "" {
		t.Error("observe 时 scanned_at 不该为空")
	}
}

// TestLocalAgentsCoordinateIncludesLeases 覆盖第三档：租约表只在 coordinate
// 出现，并且只列仍然有效的那几条。
func TestLocalAgentsCoordinateIncludesLeases(t *testing.T) {
	env, _ := newLocalAgentEnv(t, config.LocalAgentCoordinate)
	ctx := context.Background()
	lease, err := env.store.ClaimLease(ctx, store.LeaseRequest{
		ResourceKey: `d:\work\shared`,
		OwnerKind:   store.LeaseOwnerCLI,
		OwnerID:     "someone",
		Access:      store.LeaseWrite,
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("ClaimLease 失败: %v", err)
	}

	resp := getLocalAgents(t, env)
	if !resp.LeasesSupported {
		t.Fatal("coordinate 时 leases_supported 应为 true")
	}
	if len(resp.Leases) != 1 {
		t.Fatalf("leases = %+v，期望 1 条", resp.Leases)
	}
	got := resp.Leases[0]
	if got.LeaseID != lease.ID || got.ResourceKey != `d:\work\shared` ||
		got.OwnerID != "someone" || got.Mode != "write" || got.Generation != 1 {
		t.Errorf("租约行 = %+v", got)
	}
	if got.ExpiresAt == "" {
		t.Error("expires_at 不该为空")
	}
}

// TestLocalAgentsScanErrorIsNot500 钉住失败形态：200 + observed_error，
// observed 是空数组，managed 照常（与 /api/models 单家失败同构）。
func TestLocalAgentsScanErrorIsNot500(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentObserve)
	seedInflight(t, env.store)
	srv.localAgentEnum = func() ([]localagent.Process, error) {
		return nil, fmt.Errorf("Toolhelp32 失败")
	}

	resp := getLocalAgents(t, env)
	if !strings.Contains(resp.ObservedError, "Toolhelp32") {
		t.Errorf("observed_error = %q，期望带上原始错误", resp.ObservedError)
	}
	if len(resp.Observed) != 0 || resp.Observed == nil {
		t.Errorf("observed = %+v，期望空数组", resp.Observed)
	}
	if len(resp.Managed) != 1 {
		t.Errorf("扫描失败不该影响 managed 段: %+v", resp.Managed)
	}
}

// TestLocalAgentsMergesManagedProcessByPID 是 ⑨ 的正面用例：钩子记下的 PID
// 与进程表里的一条对上之后，这一轮从 managed 段挪进 observed 段并升级成
// managed —— 一条实例只画一次，而且带着 turn / conv / agent 身份。
func TestLocalAgentsMergesManagedProcessByPID(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentObserve)
	convID, started := seedInflight(t, env.store)
	srv.localAgentPIDs.Observe(convID, "c1", 300)
	srv.localAgentEnum = func() ([]localagent.Process, error) {
		return []localagent.Process{{PID: 300, PPID: 1, ExeName: "claude.exe", StartedAt: started}}, nil
	}

	resp := getLocalAgents(t, env)
	if len(resp.Managed) != 0 {
		t.Errorf("合并成功之后 managed 段不该再有一条：%+v", resp.Managed)
	}
	if len(resp.Observed) != 1 {
		t.Fatalf("observed = %+v，期望 1 行", resp.Observed)
	}
	row := resp.Observed[0]
	if row.Source != string(localagent.SourceManaged) {
		t.Errorf("source = %q，期望 managed（PID 对上就是 Council 自己的进程）", row.Source)
	}
	if row.TurnID != "turn-localagent-1" || row.ConversationID != convID || row.AgentID != "c1" {
		t.Errorf("合并结果缺身份: %+v", row)
	}
	if row.MergeEvidence != "pid" {
		t.Errorf("merge_evidence = %q，期望 pid", row.MergeEvidence)
	}
	if !strings.Contains(strings.Join(row.Evidence, " "), "merged=pid") {
		t.Errorf("evidence = %v，期望带上 merged=pid", row.Evidence)
	}
}

// TestLocalAgentsMergesThroughCmdShim 覆盖 .cmd 垫片：钩子拿到的是 cmd.exe
// 的 PID，真正干活的是它唯一的 claude.exe 子进程（本机实测的那条路径）。
func TestLocalAgentsMergesThroughCmdShim(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentObserve)
	convID, started := seedInflight(t, env.store)
	srv.localAgentPIDs.Observe(convID, "c1", 500)
	srv.localAgentEnum = func() ([]localagent.Process, error) {
		return []localagent.Process{
			{PID: 500, PPID: 1, ExeName: "cmd.exe", StartedAt: started},
			{PID: 501, PPID: 500, ExeName: "claude.exe", StartedAt: started},
		}, nil
	}

	resp := getLocalAgents(t, env)
	if len(resp.Managed) != 0 || len(resp.Observed) != 1 {
		t.Fatalf("垫片合并后应当只剩一条 observed: managed=%+v observed=%+v", resp.Managed, resp.Observed)
	}
	row := resp.Observed[0]
	if row.PID != 501 || row.Source != string(localagent.SourceManaged) {
		t.Errorf("应当把 501 认成这一轮: %+v", row)
	}
	if row.MergeEvidence != "shim=cmd.exe" {
		t.Errorf("merge_evidence = %q，期望 shim=cmd.exe", row.MergeEvidence)
	}
}

// TestLocalAgentsObserveProcessStartRespectsLevel 钉住钩子那一侧的层级判断：
// off 时它一个字节都不记；切到 coordinate 之后立刻开始记（同一次进程内）。
func TestLocalAgentsObserveProcessStartRespectsLevel(t *testing.T) {
	_, srv := newLocalAgentEnv(t, config.LocalAgentOff)
	srv.observeProcessStart("c1", "a1", 1234)
	if srv.localAgentPIDs.Len() != 0 {
		t.Fatal("层级 off 时钩子记了 PID（S-01）")
	}

	srv.cfgMu.Lock()
	srv.write.Cfg.LocalAgent = config.LocalAgentCoordinate
	srv.cfgMu.Unlock()
	srv.observeProcessStart("c1", "a1", 1234)
	if pid, ok := srv.localAgentPIDs.Lookup("c1", "a1"); !ok || pid != 1234 {
		t.Errorf("coordinate 时钩子没记下 PID（pid=%d ok=%v）", pid, ok)
	}
}

// TestLocalAgentsLeaseOverlapsCwd 覆盖 P5 的页面提醒：与看板启动目录相同或
// 在其子目录里的租约要标 overlaps_cwd，别的目录不标。
func TestLocalAgentsLeaseOverlapsCwd(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentCoordinate)
	ctx := context.Background()

	cwdKey, err := localagent.NormalizeResource(srv.write.Cwd)
	if err != nil {
		t.Fatalf("规范化 Cwd 失败: %v", err)
	}
	subKey, err := localagent.NormalizeResource(filepath.Join(srv.write.Cwd, "sub"))
	if err != nil {
		t.Fatalf("规范化子目录失败: %v", err)
	}
	otherKey, err := localagent.NormalizeResource(t.TempDir())
	if err != nil {
		t.Fatalf("规范化别的目录失败: %v", err)
	}
	for i, key := range []string{cwdKey, subKey, otherKey} {
		if _, err := env.store.ClaimLease(ctx, store.LeaseRequest{
			ResourceKey: key,
			OwnerKind:   store.LeaseOwnerCLI,
			OwnerID:     fmt.Sprintf("owner-%d", i),
			Access:      store.LeaseWrite,
			TTL:         time.Hour,
		}); err != nil {
			t.Fatalf("ClaimLease(%s) 失败: %v", key, err)
		}
	}

	resp := getLocalAgents(t, env)
	if len(resp.Leases) != 3 {
		t.Fatalf("leases = %+v，期望 3 条", resp.Leases)
	}
	got := map[string]bool{}
	for _, lease := range resp.Leases {
		got[lease.ResourceKey] = lease.OverlapsCwd
	}
	if !got[cwdKey] {
		t.Error("与看板同一目录的租约没有标 overlaps_cwd")
	}
	if !got[subKey] {
		t.Error("看板目录的子目录租约没有标 overlaps_cwd（父子关系也算重叠）")
	}
	if got[otherKey] {
		t.Error("不相干的目录被标成了 overlaps_cwd")
	}
}

// TestLocalAgentsHasNoImagePath 钉住 S-09：HTTP 响应里不许出现完整映像路径。
func TestLocalAgentsHasNoImagePath(t *testing.T) {
	env, srv := newLocalAgentEnv(t, config.LocalAgentObserve)
	srv.localAgentEnum = func() ([]localagent.Process, error) {
		return []localagent.Process{{
			PID: 400, ExeName: "claude.exe",
			ExePath: `C:\Users\someone\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\bin\claude.exe`,
		}}, nil
	}

	// POST 也走同一个门（只支持 GET），顺带证明它不会触发扫描。
	if status, _, postBody := postRaw(t, env.http.URL+"/api/local-agents", "application/json", `{}`); status != http.StatusMethodNotAllowed {
		t.Fatalf("POST 状态码 = %d，期望 405；body=%s", status, postBody)
	}
	status, body := getRaw(t, env.http.URL+"/api/local-agents")
	if status != http.StatusOK {
		t.Fatalf("GET 状态码 = %d，期望 200", status)
	}
	// 注意：响应里有 cwd（看板自己的启动目录，给它当申请租约的默认值），
	// 所以这里不能笼统地禁 "AppData" —— 临时目录本来就在 AppData 下。
	// 要钉的是「那个进程的映像路径没有漏出来」。
	for _, leaked := range []string{"exe_path", "anthropic-ai", "node_modules", "npm\\"} {
		if strings.Contains(string(body), leaked) {
			t.Errorf("响应里出现了映像路径的一部分 %q（S-09 禁止）:\n%s", leaked, body)
		}
	}
	if !strings.Contains(string(body), `"evidence":["exe=claude.exe"`) {
		t.Errorf("evidence 形状不对:\n%s", body)
	}
}

// TestLocalAgentsReadOnlyIs405 钉住只读模式：没有 -rw 就没有这个接口。
func TestLocalAgentsReadOnlyIs405(t *testing.T) {
	env := newServeTestEnv(t, nil)
	status, raw := getRaw(t, env.http.URL+"/api/local-agents")
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式状态码 = %d，期望 405；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "只读") {
		t.Errorf("405 的说明 = %s，期望说清只读模式", raw)
	}
}

// TestLocalAgentsResponseShapeIsStable 钉住 JSON 的键：三个数组永远存在，
// 空的时候是 []，不是 null；off 时没有 scanned_at。
func TestLocalAgentsResponseShapeIsStable(t *testing.T) {
	env, _ := newLocalAgentEnv(t, config.LocalAgentOff)
	_, body := getRaw(t, env.http.URL+"/api/local-agents")
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Fatalf("响应不是 JSON 对象: %v；%s", err, body)
	}
	for _, key := range []string{"local_agent", "managed", "observed", "leases", "leases_supported"} {
		raw, ok := generic[key]
		if !ok {
			t.Errorf("响应里缺少 %q", key)
			continue
		}
		if key == "local_agent" || key == "leases_supported" {
			continue
		}
		if string(raw) != "[]" {
			t.Errorf("%s = %s，期望空数组 []", key, raw)
		}
	}
	if _, ok := generic["scanned_at"]; ok {
		t.Error("off 时不该出现 scanned_at")
	}
}
