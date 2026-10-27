package main

// who_test.go 是 `council who` 的进程边界用例：
//   - off 时一次枚举、一次开库都不许发生（注入会 panic 的实现来证明）；
//   - observe 时两段输出齐全，managed 段来自真实只读库；
//   - 库不可用时仍然打印本机进程段，退出码仍是 0；
//   - -json 的字段与空数组形状稳定；
//   - who 在全局 FlagSet 之前被分派，-h 与用法错误按 -lang 出语言。

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/localagent"
	"council/internal/store"
)

// whoTestEnv 隔离 who 会读到的进程环境：配置路径、数据目录、语言与用户
// 主目录。谁都不许让开发机上的 ~/.council/config.toml 或真实 data_dir 混进来。
func whoTestEnv(t *testing.T) {
	t.Helper()
	cleanLangEnv(t)
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
}

// whoTestConfig 写一份最小合法配置。extra 用来插 local_agent 那一行。
// data_dir 用 TOML 字面量字符串（单引号）：Windows 路径里的反斜杠按原样，
// 不必在测试里手写转义。
func whoTestConfig(t *testing.T, dataDir, extra string) string {
	t.Helper()
	body := "data_dir = '" + dataDir + "'\n"
	if extra != "" {
		body += extra + "\n"
	}
	body += "\n[providers.claude]\ncommand = 'claude'\n"
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	return path
}

// whoSeedInflight 用写模式建库、建一条会话并写一条在飞记录，然后关库。
// 返回那一轮的启动时间与会话 id，供输出断言使用。
func whoSeedInflight(t *testing.T, dataDir string) (time.Time, string) {
	t.Helper()
	started := time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("建测试库失败: %v", err)
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			t.Fatalf("关测试库失败: %v", cerr)
		}
	}()
	convID := ""
	err = st.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		conv, err := tx.CreateConversation(ctx, "who 测试会话")
		if err != nil {
			return err
		}
		convID = conv.ID
		return tx.BeginTurn(ctx, conversation.InflightTurn{
			TurnID:    "turn-1",
			ConvID:    conv.ID,
			AgentID:   "theory",
			Provider:  "claude",
			CursorSeq: 0,
			ScannedTo: 0,
			StartedAt: started,
			Phase:     conversation.PhaseBegun,
			Tier:      "read",
			WorkDir:   `C:\work`,
		})
	})
	if err != nil {
		t.Fatalf("写在飞记录失败: %v", err)
	}
	return started, convID
}

// whoPanicDeps 的每个入口一被调用就失败：off 路径必须一个都不碰。
func whoPanicDeps(t *testing.T) whoDeps {
	t.Helper()
	return whoDeps{
		Enumerate: func() ([]localagent.Process, error) {
			t.Fatal("off 路径调用了进程枚举（S-01）")
			return nil, nil
		},
		OpenReadOnly: func(context.Context, string) (*store.Store, error) {
			t.Fatal("off 路径打开了库")
			return nil, nil
		},
	}
}

// TestWhoOffDoesNotEnumerateOrOpenDB 覆盖 off 的两条硬约束：不枚举进程、
// 不开库；输出恰好一句提示，退出 0。
func TestWhoOffDoesNotEnumerateOrOpenDB(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	for _, extra := range []string{"", `local_agent = "off"`} {
		name := extra
		if name == "" {
			name = "键缺席"
		}
		t.Run(name, func(t *testing.T) {
			cfgPath := whoTestConfig(t, dataDir, extra)
			var out, errOut bytes.Buffer
			code := runWhoWith([]string{"-config", cfgPath}, whoPanicDeps(t), &out, &errOut)
			if code != exitOK {
				t.Fatalf("退出码 = %d，期望 %d（stderr: %s）", code, exitOK, errOut.String())
			}
			want := `本机 agent 未开启（config 里 local_agent = "observe" 开启观察，"coordinate" 同时开启资源租约）`
			if got := strings.TrimSpace(out.String()); got != want {
				t.Errorf("off 输出 = %q，期望恰好 %q", got, want)
			}
			if errOut.Len() != 0 {
				t.Errorf("off 不该写 stderr，实际: %s", errOut.String())
			}
		})
	}
}

// TestWhoMissingConfigMeansOff：一份配置都没有是出厂状态，等价于 off；
// 不该报错，也不该因为「没有配置文件」就去枚举进程。
func TestWhoMissingConfigMeansOff(t *testing.T) {
	whoTestEnv(t)
	missing := filepath.Join(t.TempDir(), "nope.toml")

	var out, errOut bytes.Buffer
	code := runWhoWith([]string{"-config", missing}, whoPanicDeps(t), &out, &errOut)
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 %d（stderr: %s）", code, exitOK, errOut.String())
	}
	if !strings.Contains(out.String(), "未开启") {
		t.Errorf("缺配置时应当提示未开启，实际: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("缺配置不是错误，stderr 不该有内容: %s", errOut.String())
	}
}

// TestWhoOffJSONHasEmptyArrays 钉住 off 的 JSON 形状：两段数组是 [] 而不是
// null，notice 在，local_agent 是 off。
func TestWhoOffJSONHasEmptyArrays(t *testing.T) {
	whoTestEnv(t)
	cfgPath := whoTestConfig(t, filepath.Join(t.TempDir(), "data"), "")

	var out, errOut bytes.Buffer
	code := runWhoWith([]string{"-config", cfgPath, "-json"}, whoPanicDeps(t), &out, &errOut)
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 %d（stderr: %s）", code, exitOK, errOut.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("输出不是 JSON 对象: %v\n输出: %s", err, out.String())
	}
	for _, key := range []string{"local_agent", "managed", "observed", "notice"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("JSON 缺少字段 %q：%s", key, out.String())
		}
	}
	if got := string(raw["local_agent"]); got != `"off"` {
		t.Errorf("local_agent = %s，期望 \"off\"", got)
	}
	for _, key := range []string{"managed", "observed"} {
		if got := string(raw[key]); got != "[]" {
			t.Errorf("%s = %s，期望 [] 而不是 null", key, got)
		}
	}
	if string(raw["notice"]) == `""` {
		t.Errorf("notice 是空串，期望一句开启说明")
	}
}

// TestWhoObservePrintsBothSections 覆盖 observe 的完整两段：managed 来自
// 真实只读库，observed 来自注入的进程快照，node.exe / python.exe 不出现。
func TestWhoObservePrintsBothSections(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	started, convID := whoSeedInflight(t, dataDir)
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "observe"`)

	deps := whoDeps{
		Enumerate: func() ([]localagent.Process, error) {
			return []localagent.Process{
				{PID: 200, PPID: 100, ExeName: "node.exe"},
				{PID: 300, PPID: 200, ExeName: "claude.exe", ExePath: `C:\Users\me\AppData\Roaming\npm\claude.exe`, StartedAt: started},
				{PID: 301, PPID: 1, ExeName: "python.exe"},
				{PID: 400, PPID: 1, ExeName: "codex.exe"},
			}, nil
		},
		OpenReadOnly: store.OpenReadOnly,
	}

	var out, errOut bytes.Buffer
	code := runWhoWith([]string{"-config", cfgPath}, deps, &out, &errOut)
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 %d（stderr: %s）", code, exitOK, errOut.String())
	}

	text := out.String()
	want := []string{
		"Council 管理的成员（1）",
		"会话 " + convID + " / 成员 theory / turn turn-1 / phase begun / 起于 " + started.Format(time.RFC3339),
		"本机其他 agent 进程（2）",
		"claude  pid=300 ppid=200 exe=claude.exe source=observed certainty=exact 起于 " + started.Format(time.RFC3339),
		"codex  pid=400 ppid=1 exe=codex.exe source=observed certainty=probable 起于 未知",
		"本命令看的是本机进程，不是会话成员",
	}
	for _, line := range want {
		if !strings.Contains(text, line) {
			t.Errorf("输出缺少 %q，实际:\n%s", line, text)
		}
	}
	for _, unwanted := range []string{"node.exe", "python.exe"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("通用宿主 %s 不该出现在输出里:\n%s", unwanted, text)
		}
	}
	// 占位符数量对不上时 fmt 会打印 %!EXTRA / %!v；真机上就是靠这条发现的
	// （cli.who.observedRow 加了第 8 个 %s，目录里的文案还是 7 个）。
	if strings.Contains(text, "%!") {
		t.Errorf("输出里有 fmt 的错误标记（占位符与参数数量不匹配）:\n%s", text)
	}
}

// TestWhoObserveJSON 钉住 observe 的 JSON 字段名与内容。
func TestWhoObserveJSON(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	started, convID := whoSeedInflight(t, dataDir)
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "observe"`)

	deps := whoDeps{
		Enumerate: func() ([]localagent.Process, error) {
			return []localagent.Process{
				{PID: 10, PPID: 1, ExeName: "claude.exe", ExePath: `C:\Users\me\AppData\Roaming\npm\claude.exe`, StartedAt: started},
				{PID: 20, PPID: 1, ExeName: "codex.exe"},
			}, nil
		},
		OpenReadOnly: store.OpenReadOnly,
	}

	var out, errOut bytes.Buffer
	code := runWhoWith([]string{"-config", cfgPath, "-json"}, deps, &out, &errOut)
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 %d（stderr: %s）", code, exitOK, errOut.String())
	}

	var report struct {
		LocalAgent string `json:"local_agent"`
		Managed    []struct {
			ConversationID string `json:"conversation_id"`
			AgentID        string `json:"agent_id"`
			TurnID         string `json:"turn_id"`
			Provider       string `json:"provider"`
			Phase          string `json:"phase"`
			StartedAt      string `json:"started_at"`
			WorkDir        string `json:"work_dir"`
			Source         string `json:"source"`
		} `json:"managed"`
		Observed []struct {
			Kind      string `json:"kind"`
			PID       int    `json:"pid"`
			PPID      int    `json:"ppid"`
			ExeName   string `json:"exe_name"`
			ExePath   string `json:"exe_path"`
			StartedAt string `json:"started_at"`
			Source    string `json:"source"`
			Certainty string `json:"certainty"`
		} `json:"observed"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("输出不是合法 JSON: %v\n输出: %s", err, out.String())
	}
	if report.LocalAgent != "observe" {
		t.Errorf("local_agent = %q，期望 observe", report.LocalAgent)
	}
	if len(report.Managed) != 1 {
		t.Fatalf("managed 有 %d 行，期望 1 行", len(report.Managed))
	}
	m := report.Managed[0]
	if m.ConversationID != convID || m.AgentID != "theory" || m.TurnID != "turn-1" ||
		m.Provider != "claude" || m.Phase != "begun" || m.Source != "managed" ||
		m.StartedAt != started.Format(time.RFC3339) || m.WorkDir != `C:\work` {
		t.Errorf("managed 行内容不对: %+v", m)
	}
	if len(report.Observed) != 2 {
		t.Fatalf("observed 有 %d 行，期望 2 行", len(report.Observed))
	}
	if report.Observed[0].Kind != "claude" || report.Observed[0].PID != 10 ||
		report.Observed[0].Source != "observed" || report.Observed[0].StartedAt == "" ||
		report.Observed[0].Certainty != "exact" ||
		report.Observed[0].ExePath != `C:\Users\me\AppData\Roaming\npm\claude.exe` {
		t.Errorf("observed 第一行不对: %+v", report.Observed[0])
	}
	if report.Observed[1].Kind != "codex" || report.Observed[1].StartedAt != "" ||
		report.Observed[1].Certainty != "probable" {
		t.Errorf("拿不到路径时应当标 probable 且启动时间为空串，实际: %+v", report.Observed[1])
	}
}

// TestWhoShowsLeases 覆盖第三段（L2 的租约）：只读打开 v10 库时列出仍然有效的
// 租约；没有租约时也照样打标题与「没有有效租约」，让用户分得清「没有」与
// 「这段没跑」。
func TestWhoShowsLeases(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "coordinate"`)

	// 用写模式造一条租约（也是 v10 库的第一次建立）。
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	lease, err := st.ClaimLease(ctx, store.LeaseRequest{
		ResourceKey: `d:\shared\proj`,
		OwnerKind:   store.LeaseOwnerCLI,
		OwnerID:     "someone",
		Access:      store.LeaseWrite,
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("ClaimLease 失败: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关库失败: %v", err)
	}

	deps := whoDeps{
		Enumerate:    func() ([]localagent.Process, error) { return nil, nil },
		OpenReadOnly: store.OpenReadOnly,
	}

	var out, errOut bytes.Buffer
	if code := runWhoWith([]string{"-config", cfgPath}, deps, &out, &errOut); code != exitOK {
		t.Fatalf("退出码 = %d（stderr: %s）", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{
		"本机租约（1）",
		"资源 d:\\shared\\proj / 持有者 cli/someone / 模式 write / 代数 1 / 到期 ",
		lease.ID,
		"租约是提醒，不是文件锁",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("文本输出缺少 %q，实际:\n%s", want, text)
		}
	}

	out.Reset()
	errOut.Reset()
	if code := runWhoWith([]string{"-config", cfgPath, "-json"}, deps, &out, &errOut); code != exitOK {
		t.Fatalf("JSON 退出码 = %d（stderr: %s）", code, errOut.String())
	}
	var report struct {
		Leases []struct {
			LeaseID     string `json:"lease_id"`
			ResourceKey string `json:"resource_key"`
			OwnerKind   string `json:"owner_kind"`
			OwnerID     string `json:"owner_id"`
			Mode        string `json:"mode"`
			Generation  int64  `json:"generation"`
			ExpiresAt   string `json:"expires_at"`
		} `json:"leases"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("输出不是合法 JSON: %v\n输出: %s", err, out.String())
	}
	if len(report.Leases) != 1 {
		t.Fatalf("leases 有 %d 行，期望 1 行", len(report.Leases))
	}
	got := report.Leases[0]
	if got.LeaseID != lease.ID || got.ResourceKey != `d:\shared\proj` ||
		got.OwnerKind != "cli" || got.OwnerID != "someone" ||
		got.Mode != "write" || got.Generation != 1 || got.ExpiresAt == "" {
		t.Errorf("租约行内容不对: %+v", got)
	}

	// 没有租约时：标题照打，正文说「没有有效租约」。
	emptyDir := filepath.Join(t.TempDir(), "empty-data")
	emptyCfg := whoTestConfig(t, emptyDir, `local_agent = "observe"`)
	seeded, err := store.Open(ctx, emptyDir)
	if err != nil {
		t.Fatalf("建空库失败: %v", err)
	}
	if err := seeded.Close(); err != nil {
		t.Fatalf("关空库失败: %v", err)
	}
	out.Reset()
	errOut.Reset()
	if code := runWhoWith([]string{"-config", emptyCfg}, whoDeps{
		Enumerate:    func() ([]localagent.Process, error) { return nil, nil },
		OpenReadOnly: store.OpenReadOnly,
	}, &out, &errOut); code != exitOK {
		t.Fatalf("空库退出码 = %d（stderr: %s）", code, errOut.String())
	}
	if !strings.Contains(out.String(), "本机租约（0）") ||
		!strings.Contains(out.String(), "没有有效租约") {
		t.Errorf("空租约输出不对:\n%s", out.String())
	}
}

// TestWhoDBUnavailableStillShowsProcesses 覆盖「库打不开也不崩」：
// 数据目录不存在时 managed 段说明原因，observed 段照常显示，退出 0。
func TestWhoDBUnavailableStillShowsProcesses(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "does-not-exist")
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "observe"`)

	deps := whoDeps{
		Enumerate: func() ([]localagent.Process, error) {
			return []localagent.Process{{PID: 300, PPID: 1, ExeName: "claude.exe"}}, nil
		},
		OpenReadOnly: store.OpenReadOnly,
	}

	var out, errOut bytes.Buffer
	code := runWhoWith([]string{"-config", cfgPath}, deps, &out, &errOut)
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 %d（stderr: %s）", code, exitOK, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"库不可用", "本机其他 agent 进程（1）", "claude  pid=300"} {
		if !strings.Contains(text, want) {
			t.Errorf("输出缺少 %q，实际:\n%s", want, text)
		}
	}
	if errOut.Len() != 0 {
		t.Errorf("库不可用只该写在输出里，不该进 stderr: %s", errOut.String())
	}
}

// TestWhoBrokenConfigExitsOne：非法 local_agent 不让 who 猜一个层级。
func TestWhoBrokenConfigExitsOne(t *testing.T) {
	whoTestEnv(t)
	cfgPath := whoTestConfig(t, filepath.Join(t.TempDir(), "data"), `local_agent = "on"`)

	var out, errOut bytes.Buffer
	code := runWhoWith([]string{"-config", cfgPath}, whoPanicDeps(t), &out, &errOut)
	if code != exitConfigFailed {
		t.Fatalf("退出码 = %d，期望 %d", code, exitConfigFailed)
	}
	if !strings.Contains(errOut.String(), "配置文件有问题") || !strings.Contains(errOut.String(), "local_agent") {
		t.Errorf("stderr 没有说清配置问题:\n%s", errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("配置坏掉时不该有正常输出: %s", out.String())
	}
}

// TestWhoIsDispatchedBeforeGlobalFlags 走一遍 run()：`who` 必须在全局
// FlagSet 之前被拦下，否则 -json 会成为未知 flag、who 会成为位置参数。
func TestWhoIsDispatchedBeforeGlobalFlags(t *testing.T) {
	whoTestEnv(t)
	cfgPath := whoTestConfig(t, filepath.Join(t.TempDir(), "data"), "")

	out, code := commandsRun(t, []string{"who", "-config", cfgPath})
	if code != exitOK {
		t.Fatalf("council who 退出码 = %d，期望 %d；输出: %s", code, exitOK, out)
	}
	if !strings.Contains(out, "未开启") {
		t.Errorf("输出里没有开启提示:\n%s", out)
	}
}

// TestWhoUsageEnglish 钉住 -h 的中英分叉：-lang en 下不许出现汉字。
func TestWhoUsageEnglish(t *testing.T) {
	whoTestEnv(t)
	var out, errOut bytes.Buffer
	code := runWhoWith([]string{"-h", "-lang", "en"}, whoPanicDeps(t), &out, &errOut)
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 %d", code, exitOK)
	}
	usage := errOut.String()
	if cliHasHan(usage) {
		t.Fatalf("-lang en 的 who 用法里出现汉字:\n%s", usage)
	}
	if !strings.Contains(usage, "Usage: council who") {
		t.Errorf("用法里缺少 Usage: council who:\n%s", usage)
	}
}

// TestWhoPositionalRejected 钉住位置参数退 2（与其它子命令一致）。
func TestWhoPositionalRejected(t *testing.T) {
	whoTestEnv(t)
	var out, errOut bytes.Buffer
	code := runWhoWith([]string{"extra"}, whoPanicDeps(t), &out, &errOut)
	if code != exitUsage {
		t.Fatalf("退出码 = %d，期望 %d", code, exitUsage)
	}
	if !strings.Contains(errOut.String(), "不接受位置参数") {
		t.Errorf("stderr 没有拒绝位置参数:\n%s", errOut.String())
	}
}
