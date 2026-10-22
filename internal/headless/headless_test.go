package headless_test

// 本文件用 internal/app/smoke_test.go 的同一套装置做 headless 的端到端：
// 测试二进制在 COUNCIL_FAKE_CLI=1 时扮演 provider CLI，配置里的 command 写
// os.Args[0]；临时配置 + 临时 data_dir，走真正的 app.Run 装配与真 SQLite。
// 测试二进制自己当 provider 是为了不起真 CLI、不花额度。
//
// 假 CLI 支持三种测试钩子（都走环境变量，子进程原样继承）：
// COUNCIL_FAKE_CALLS 是计数文件，每次调用追加一个字节；COUNCIL_FAKE_FAIL_AFTER
// 让第 N 次之后的调用回失败 JSON；COUNCIL_FAKE_SLEEP_AFTER / _MS 让第 N 次之后
// 的调用睡一会儿。它们只服务「总结轮失败/超时/取消」这几条，不参与生产逻辑。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"council/internal/app"
	"council/internal/config"
	"council/internal/headless"
)

func TestMain(m *testing.M) {
	if os.Getenv("COUNCIL_FAKE_CLI") == "1" {
		runFakeProviderCLI()
		return
	}
	os.Exit(m.Run())
}

func envInt(name string) int {
	n, _ := strconv.Atoi(os.Getenv(name))
	return n
}

// fakeCallNumber 把这次调用记进计数文件，并返回从 1 开始的序号。
// 一个字节的 O_APPEND 写足够让多个并发子进程不互相覆盖。
func fakeCallNumber() int {
	path := os.Getenv("COUNCIL_FAKE_CALLS")
	if path == "" {
		return 0
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0
	}
	_, _ = f.Write([]byte("x"))
	_ = f.Close()
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return len(b)
}

// runFakeProviderCLI 按调用参数判断是哪一家：grok 的 BuildTurn 带
// --system-prompt-override，claude 带 --append-system-prompt-file。
func runFakeProviderCLI() {
	_, _ = io.ReadAll(os.Stdin)
	call := fakeCallNumber()

	if after := envInt("COUNCIL_FAKE_FAIL_AFTER"); after > 0 && call > after {
		fmt.Fprint(os.Stdout, `{"type":"result","subtype":"success","is_error":true,`+
			`"terminal_reason":"api_error","api_error_status":500,`+
			`"stop_reason":"stop_sequence","session_id":"fake-session","result":"boom"}`)
		os.Exit(0)
	}
	if after := envInt("COUNCIL_FAKE_SLEEP_AFTER"); after > 0 && call > after {
		ms := envInt("COUNCIL_FAKE_SLEEP_MS")
		if ms <= 0 {
			ms = 3000
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	// COUNCIL_FAKE_SLEEP_ALL_MS 让每一次调用都睡。上面的 SLEEP_AFTER 只能
	// 让第 N 次之后变慢，做不出「第一轮全员在飞」，而零回复取消那条用例
	// 需要的正是全员都还没回来。
	if ms := envInt("COUNCIL_FAKE_SLEEP_ALL_MS"); ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	// -workspace 用例要一个真的往 cwd 写文件的成员；写相对路径，
	// 落在子进程当前目录（副本根），而不是测试进程的启动目录。
	if name := os.Getenv("COUNCIL_FAKE_WRITE_FILE"); name != "" {
		if err := os.WriteFile(name, []byte("written by fake member\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "fake CLI write %s: %v\n", name, err)
			os.Exit(3)
		}
	}

	args := strings.Join(os.Args[1:], " ")
	if strings.Contains(args, "--system-prompt-override") {
		fmt.Fprint(os.Stdout, `{"text":"OK","stopReason":"end_turn",`+
			`"sessionId":"fake-grok-session","usage":{"input_tokens":2,`+
			`"cache_creation_input_tokens":0,"cache_read_input_tokens":0,`+
			`"output_tokens":4,"reasoning_tokens":0}}`)
		os.Exit(0)
	}

	text := os.Getenv("COUNCIL_FAKE_TEXT")
	if text == "" {
		text = "OK"
	}
	cost := os.Getenv("COUNCIL_FAKE_COST")
	if cost == "" {
		cost = "0.01"
	}
	body, err := json.Marshal(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake CLI: %v\n", err)
		os.Exit(3)
	}
	fmt.Fprintf(os.Stdout, `{"type":"result","subtype":"success","is_error":false,`+
		`"terminal_reason":"completed","api_error_status":null,"stop_reason":"end_turn",`+
		`"session_id":"fake-session","result":%s,"total_cost_usd":%s,`+
		`"usage":{"input_tokens":2,"cache_creation_input_tokens":10,`+
		`"cache_read_input_tokens":0,"output_tokens":4,`+
		`"output_tokens_details":{"thinking_tokens":0}}}`, body, cost)
	os.Exit(0)
}

const askTestTimeout = 20 * time.Second

func tomlQuote(s string) string { return "'" + s + "'" }

// writeAskConfig 落一份能通过 config.Validate 的配置。grokLive 为真时
// grok 的命令指向测试二进制自己，用来让一个 ReportsCost=SupportNo 的
// 适配器正常跑完（只验 cost_complete，不验金额）；为假时指向不存在的
// 路径，用来造 spawn_failed。
func writeAskConfig(t *testing.T, extra string) (string, *config.Config) {
	return writeAskConfigGrok(t, false, extra)
}

func writeAskConfigGrok(t *testing.T, grokLive bool, extra string) (string, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	grokCmd := filepath.Join(dir, "no-such-grok.exe")
	if grokLive {
		grokCmd = os.Args[0]
	}
	body := fmt.Sprintf(`data_dir = %s

[providers.claude]
command = %s

[providers.grok]
command = %s

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "headless 测试成员"
permissions = "read"
mode = "discussion"

[agents.c2]
display_name = "C2"
provider = "grok"
model = "grok-4.7"
effort = "high"
role = "headless 测试成员"
permissions = "read"
mode = "discussion"

[agents.c3]
display_name = "C3"
provider = "claude"
model = "sonnet"
effort = "high"
role = "headless 测试成员"
permissions = "read"
mode = "discussion"

[agents.cws]
display_name = "CWS"
provider = "claude"
model = "sonnet"
effort = "high"
role = "headless 测试可写成员"
permissions = "workspace"
mode = "discussion"

[agents.j1]
display_name = "J1"
provider = "claude"
model = "opus"
effort = "high"
role = "headless 测试裁判"
permissions = "read"
mode = "judge"

[agents.bad]
display_name = "Bad"
provider = "claude"
model = "opus"
effort = "high"
role = "headless 测试不可用成员"
permissions = "full"
mode = "discussion"

[agents.s1]
display_name = "S1"
provider = "claude"
model = "opus"
effort = "high"
role = "headless 测试总结人"
permissions = "read"
mode = "discussion"

%s
`, tomlQuote(dataDir), tomlQuote(os.Args[0]), tomlQuote(grokCmd), extra)
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	cfg, err := config.LoadAndValidate(cfgPath)
	if err != nil {
		t.Fatalf("测试配置不合法: %v", err)
	}
	return cfgPath, cfg
}

// runAskForTest 从 flag 开始走 app.Run 的 headless 分支，带看门狗。
// Parse 传 nil cfg，与 cmd/council 的生产路径一致：配置相关校验由
// headless.Run 拿到 cfg 后补判。
func runAskForTest(t *testing.T, ctx context.Context, cfgPath string, args []string, stdin string) (string, string, error) {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")

	opts, err := headless.Parse(args, nil)
	if err != nil {
		t.Fatalf("Parse(%v) 失败: %v", args, err)
	}
	var out, errOut bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, app.Options{
			ConfigPath: cfgPath,
			In:         strings.NewReader(stdin),
			Out:        &out,
			Err:        &errOut,
			Headless:   &opts,
		})
	}()
	select {
	case err := <-done:
		return out.String(), errOut.String(), err
	case <-time.After(askTestTimeout):
		t.Fatalf("app.Run 在 %v 内没有返回", askTestTimeout)
		return "", "", nil
	}
}

// decodeSingleResult 解出 stdout 上唯一的 JSON 对象，并确认后面没有第二个值。
func decodeSingleResult(t *testing.T, out string) (headless.Result, map[string]any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var res headless.Result
	if err := dec.Decode(&res); err != nil {
		t.Fatalf("stdout 不是合法 JSON: %v；完整输出:\n%s", err, out)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout 应当只有一个 JSON 对象，后面还有内容（err=%v，值=%v）；完整输出:\n%s", err, extra, out)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("stdout 不能解成 JSON 对象: %v；完整输出:\n%s", err, out)
	}
	return res, raw
}

// assertUsageKeys 直接看原始 JSON 的键名，避免结构体标签写错时解码也
// 一同错过去（解到同一个结构体上，错误标签只表现为零值）。
func assertUsageKeys(t *testing.T, raw map[string]any) {
	t.Helper()
	turns, ok := raw["turns"].([]any)
	if !ok || len(turns) == 0 {
		t.Fatalf("原始 JSON 里 turns 不是非空数组: %v", raw["turns"])
	}
	first, ok := turns[0].(map[string]any)
	if !ok {
		t.Fatalf("turns[0] 不是对象: %v", turns[0])
	}
	usage, ok := first["usage"].(map[string]any)
	if !ok {
		t.Fatalf("turns[0].usage 不是对象: %v", first["usage"])
	}
	for _, key := range []string{"in", "cache_create", "cache_read", "out"} {
		if _, ok := usage[key]; !ok {
			t.Errorf("usage 缺少契约键 %q；实际 %v", key, usage)
		}
	}
	if _, ok := raw["cost_complete"]; !ok {
		t.Errorf("顶层 JSON 缺少契约键 cost_complete；实际键：%v", keysOf(raw))
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestAsk_JSON单个对象与正文 A1：一个成员、-json，退 0，stdout 是单个
// JSON，turns[0].text 非空；同时钉住横幅没有混进 stdout。
func TestAsk_JSON单个对象与正文(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "1+1", "-members", "c1", "-json"}, "")
	if err != nil {
		t.Fatalf("A1：Run 应当返回 nil，实际 %v；Err:\n%s\nOut:\n%s", err, errOut, out)
	}
	if got := headless.ExitCode(err); got != 0 {
		t.Errorf("A1：ok 的退出码应为 0，实际 %d", got)
	}
	res, raw := decodeSingleResult(t, out)
	if res.Status != "ok" {
		t.Errorf("A1：status 应为 ok，实际 %q", res.Status)
	}
	if res.ConversationID == "" {
		t.Errorf("A1：conversation_id 不应为空")
	}
	if len(res.Members) != 1 || res.Members[0] != "c1" {
		t.Errorf("A1：members 应为 [c1]，实际 %v", res.Members)
	}
	if len(res.Turns) != 1 {
		t.Fatalf("A1：turns 应恰好 1 条，实际 %d；Out:\n%s", len(res.Turns), out)
	}
	if res.Turns[0].Text == "" {
		t.Errorf("A1：turns[0].text 不应为空；turns[0]=%+v", res.Turns[0])
	}
	if res.Turns[0].AgentID != "c1" || res.Turns[0].Round != 1 || res.Turns[0].Outcome != "ok" {
		t.Errorf("A1：turns[0] 的身份应为 c1/第1轮/ok，实际 %+v", res.Turns[0])
	}
	if res.Turns[0].Model != "opus" || res.Turns[0].Effort != "high" {
		t.Errorf("A1：model/effort 应取配置的 opus/high，实际 %q/%q", res.Turns[0].Model, res.Turns[0].Effort)
	}
	if res.Turns[0].Usage.In == 0 || res.Turns[0].Usage.Out == 0 {
		t.Errorf("A1：usage 应带出假 CLI 报的输入/输出，实际 %+v", res.Turns[0].Usage)
	}
	if !res.CostComplete {
		t.Errorf("A1：全是 claude 成员时 cost_complete 应为 true")
	}
	assertUsageKeys(t, raw)
	if strings.Contains(out, "配置文件:") || strings.Contains(out, "输入 /help") {
		t.Errorf("A1：headless 的 stdout 不应有横幅，实际:\n%s", out)
	}
}

// TestAsk_stdin不是终端 是 A10 的底层佐证：stdin 里的 REPL 命令不会被执行。
// 真正的 ask 分派由 cmd/council 的 TestAskDispatch_... 覆盖。
func TestAsk_stdin不是终端(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "1+1", "-members", "c1", "-json"}, "/help\n/quit\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；Err:\n%s", err, errOut)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "ok" || len(res.Turns) != 1 {
		t.Errorf("应正常走完 ask（ok、1 条 turn），实际 status=%q turns=%d", res.Status, len(res.Turns))
	}
	if strings.Contains(out, "/help") {
		t.Errorf("stdin 的内容不应出现在 stdout，实际:\n%s", out)
	}
}

// TestAsk_partial退0 partial：一个成员成功、一个起不来，status=partial，
// stderr 一行带人数，退出码仍是 0（D6）。
func TestAsk_partial退0(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,c2", "-json"}, "")
	if err != nil {
		t.Fatalf("partial 用例：Run 应当返回 nil，实际 %v；Err:\n%s", err, errOut)
	}
	if got := headless.ExitCode(err); got != 0 {
		t.Errorf("partial 的退出码应为 0，实际 %d", got)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "partial" {
		t.Errorf("status 应为 partial，实际 %q", res.Status)
	}
	if len(res.Turns) != 2 {
		t.Fatalf("turns 应含两个成员，实际 %d；Out:\n%s", len(res.Turns), out)
	}
	if res.Turns[0].Outcome != "ok" || res.Turns[1].Outcome == "ok" {
		t.Errorf("应按 c1 成功、c2 失败，实际 %q / %q", res.Turns[0].Outcome, res.Turns[1].Outcome)
	}
	if res.CostComplete {
		t.Errorf("名单里有 grok（不报金额）时 cost_complete 应为 false")
	}
	if !strings.Contains(errOut, "council: partial: 1/2 成员失败") {
		t.Errorf("stderr 应含冻结格式 partial 行，实际:\n%s", errOut)
	}
}

// TestAsk_round两轮与汇总 覆盖 round 的执行路径：两轮都跑、每轮重新取快照，
// 最后把汇总成员的话放进 summary 字段而不塞进 turns。
func TestAsk_round两轮与汇总(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "[summary]\nagent = \"s1\"\n")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c3,c1", "-mode", "round", "-rounds", "2", "-json"}, "")
	if err != nil {
		t.Fatalf("round 应当成功返回 nil，实际 %v；Err:\n%s\nOut:\n%s", err, errOut, out)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "ok" {
		t.Errorf("status 应为 ok，实际 %q", res.Status)
	}
	if len(res.Turns) != 4 {
		t.Fatalf("两个成员两轮应产生 4 条 turns，实际 %d；Out:\n%s", len(res.Turns), out)
	}
	if res.Turns[0].AgentID != "c3" || res.Turns[1].AgentID != "c1" {
		t.Errorf("turns 的发言顺序应按 -members 给出的 c3,c1，实际 %q,%q",
			res.Turns[0].AgentID, res.Turns[1].AgentID)
	}
	if res.Turns[0].Round != 1 || res.Turns[2].Round != 2 {
		t.Errorf("turns 的 round 应依次为 1、1、2、2，实际 %d/%d",
			res.Turns[0].Round, res.Turns[2].Round)
	}
	if res.Summary == "" {
		t.Errorf("配了 [summary].agent 时 summary 不应为空；Err:\n%s", errOut)
	}
}

// TestAsk_预算被成员触线 是复审第 2 条指定的形状：两个成员一轮就跨过
// 0.5（0.3 × 2），turns 保留、summary 为空、status=budget_exceeded、退 1。
func TestAsk_预算被成员触线(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_COST", "0.3")
	cfgPath, _ := writeAskConfig(t, "[summary]\nagent = \"s1\"\n")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,c3", "-mode", "round", "-rounds", "1", "-budget-usd", "0.5", "-json"}, "")
	if err == nil {
		t.Fatalf("budget_exceeded 应当返回非 nil 错误；Out:\n%s", out)
	}
	if got := headless.ExitCode(err); got != 1 {
		t.Errorf("budget_exceeded 的退出码应为 1，实际 %d（err=%v）", got, err)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "budget_exceeded" {
		t.Errorf("status 应为 budget_exceeded，实际 %q", res.Status)
	}
	if len(res.Turns) != 2 {
		t.Errorf("turns 应保留两个成员，实际 %d", len(res.Turns))
	}
	if res.Summary != "" {
		t.Errorf("成员轮已触线时不应再跑总结，summary 应为空，实际 %q", res.Summary)
	}
	if res.CostUSD < 0.5 {
		t.Errorf("cost_usd 应已累计到触线金额，实际 %v", res.CostUSD)
	}
	if !strings.Contains(errOut, "预算触线") {
		t.Errorf("stderr 应说明预算触线，实际:\n%s", errOut)
	}
}

// TestAsk_预算被总结推过线 覆盖复审第 2 条的真正漏点：发言轮还在线下
// （0.2 × 2 = 0.4），总结再加 0.2 才跨过 0.5。必须重查一次，否则会报 ok。
func TestAsk_预算被总结推过线(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_COST", "0.2")
	cfgPath, _ := writeAskConfig(t, "[summary]\nagent = \"s1\"\n")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,c3", "-mode", "round", "-rounds", "1", "-budget-usd", "0.5", "-json"}, "")
	if err == nil {
		t.Fatalf("总结把总额推过线时应当返回非 nil 错误；Out:\n%s", out)
	}
	if got := headless.ExitCode(err); got != 1 {
		t.Errorf("budget_exceeded 的退出码应为 1，实际 %d（err=%v）", got, err)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "budget_exceeded" {
		t.Errorf("status 应为 budget_exceeded，实际 %q", res.Status)
	}
	if len(res.Turns) != 2 {
		t.Errorf("turns 应保留两个发言成员，实际 %d", len(res.Turns))
	}
	if res.Summary == "" {
		t.Errorf("发言轮未触线时总结应当已经跑完，summary 不应为空；Err:\n%s", errOut)
	}
	if res.CostUSD < 0.5 {
		t.Errorf("cost_usd 应包含总结的金额，实际 %v", res.CostUSD)
	}
}

// TestAsk_汇总失败算partial 是复审第 5 条：round 的汇总真尝试过却没拿到
// 正文，计入失败；预算/取消跳过的汇总不算。
func TestAsk_汇总失败算partial(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	t.Setenv("COUNCIL_FAKE_CALLS", calls)
	t.Setenv("COUNCIL_FAKE_FAIL_AFTER", "2")
	cfgPath, _ := writeAskConfig(t, "[summary]\nagent = \"s1\"\n")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,c3", "-mode", "round", "-rounds", "1", "-json"}, "")
	if err != nil {
		t.Fatalf("汇总失败是 partial，不应返回错误，实际 %v；Err:\n%s", err, errOut)
	}
	if got := headless.ExitCode(err); got != 0 {
		t.Errorf("汇总失败的 partial 退出码应为 0，实际 %d", got)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "partial" {
		t.Errorf("status 应为 partial，实际 %q", res.Status)
	}
	if res.Summary != "" {
		t.Errorf("汇总失败时 summary 应为空，实际 %q", res.Summary)
	}
	if !strings.Contains(errOut, "汇总成员 s1 没有回复") {
		t.Errorf("stderr 应点名汇总成员没回复，实际:\n%s", errOut)
	}
	if !strings.Contains(errOut, "council: partial:") {
		t.Errorf("stderr 应含 partial 行，实际:\n%s", errOut)
	}
	// 汇总轮不进 turns，所以 JSON 必须自带席位计数，而且要与 stderr 同源：
	// 两个发言成员 + 一个汇总席位 = 3，其中失败 1 个。
	if res.Failed != 1 || res.Total != 3 {
		t.Errorf("failed/total 应为 1/3，实际 %d/%d", res.Failed, res.Total)
	}
	if !strings.Contains(errOut, "partial: 1/3 成员失败") {
		t.Errorf("stderr 的 partial 行应与 JSON 的 failed/total 同源（1/3），实际:\n%s", errOut)
	}
}

// TestAsk_慢总结超时仍partial 是复审第 1 条：发言轮已有结果，总结被整体
// 超时打断时按 partial 退 0，不能把整次调用打成 error。
func TestAsk_慢总结超时仍partial(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COUNCIL_FAKE_CALLS", filepath.Join(dir, "calls"))
	t.Setenv("COUNCIL_FAKE_SLEEP_AFTER", "2")
	t.Setenv("COUNCIL_FAKE_SLEEP_MS", "3000")
	cfgPath, _ := writeAskConfig(t, "[summary]\nagent = \"s1\"\n")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,c3", "-timeout", "1200ms", "-json"}, "")
	if err != nil {
		t.Fatalf("总结超时不应返回错误（已有发言结果），实际 %v；Err:\n%s", err, errOut)
	}
	if got := headless.ExitCode(err); got != 0 {
		t.Errorf("总结超时的 partial 退出码应为 0，实际 %d", got)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "partial" {
		t.Errorf("status 应为 partial，实际 %q", res.Status)
	}
	if len(res.Turns) != 2 {
		t.Errorf("turns 应保留两个发言成员，实际 %d", len(res.Turns))
	}
	if res.Summary != "" {
		t.Errorf("总结被超时打断时 summary 应为空，实际 %q", res.Summary)
	}
}

// TestAsk_慢总结外部取消仍partial 是复审第 1 条的取消一侧。
func TestAsk_慢总结外部取消仍partial(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	t.Setenv("COUNCIL_FAKE_CALLS", calls)
	t.Setenv("COUNCIL_FAKE_SLEEP_AFTER", "2")
	t.Setenv("COUNCIL_FAKE_SLEEP_MS", "3000")
	cfgPath, _ := writeAskConfig(t, "[summary]\nagent = \"s1\"\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if b, err := os.ReadFile(calls); err == nil && len(b) >= 3 {
				cancel()
				return
			}
			select {
			case <-done:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()

	out, errOut, err := runAskForTest(t, ctx, cfgPath,
		[]string{"-p", "q", "-members", "c1,c3", "-json"}, "")
	if err != nil {
		t.Fatalf("总结被取消不应返回错误（已有发言结果），实际 %v；Err:\n%s", err, errOut)
	}
	if got := headless.ExitCode(err); got != 0 {
		t.Errorf("总结被取消的 partial 退出码应为 0，实际 %d", got)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "partial" {
		t.Errorf("status 应为 partial，实际 %q", res.Status)
	}
	if len(res.Turns) != 2 {
		t.Errorf("turns 应保留两个发言成员，实际 %d", len(res.Turns))
	}
}

// TestAsk_整轮被拒退1 是复审第 6 条：每一条都是 refused+Err，说明整轮
// 根本没跑起来，按 error 退 1，不是 partial。
// TestAsk_成员级全拒仍partial 钉 D6：全员被成员级原因拒绝（FULL 权限、
// prompt 过长等）是「有成员失败」，不是「整轮没跑起来」，所以是 partial
// 退 0。真正的整轮失败（mint round id / 写 round_started 失败）的判据由
// TestAllRefusedWithErr 在包内直接钉住：那种预填记录没有 TurnID。
func TestAsk_成员级全拒仍partial(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "bad", "-json"}, "")
	if err != nil {
		t.Fatalf("成员级全拒不应返回错误（D6 算 partial），实际 %v；Err:\n%s", err, errOut)
	}
	if got := headless.ExitCode(err); got != 0 {
		t.Errorf("成员级全拒的退出码应为 0，实际 %d", got)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "partial" {
		t.Errorf("status 应为 partial，实际 %q", res.Status)
	}
	if len(res.Turns) != 1 || res.Turns[0].Outcome == "ok" {
		t.Errorf("应保留一条未成功的 turn 记录，实际 %+v", res.Turns)
	}
	if strings.Contains(errOut, "整轮没有跑起来") {
		t.Errorf("成员级拒绝不该报成整轮失败，stderr:\n%s", errOut)
	}
}

// TestAsk_第一轮零回复取消退1 是契约 §2.4 的「一条回复都没有就取消」：
// 取消时 Runner 仍会为每个在飞成员留下一条 OutcomeCanceled 记录，turns
// 非空但正文全空，这种情况必须是 error/1，不能算 partial。
func TestAsk_第一轮零回复取消退1(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	t.Setenv("COUNCIL_FAKE_CALLS", calls)
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "5000")
	cfgPath, _ := writeAskConfig(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			// 两个成员都已起进程（计数文件里有两笔）时取消，保证取消发生时
			// 还没有任何一条回复落地。
			if b, err := os.ReadFile(calls); err == nil && len(b) >= 2 {
				cancel()
				return
			}
			select {
			case <-done:
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}()

	out, errOut, err := runAskForTest(t, ctx, cfgPath,
		[]string{"-p", "q", "-members", "c1,c3", "-json"}, "")
	if err == nil {
		t.Fatalf("第一轮零回复被取消应当返回错误；Out:\n%s", out)
	}
	if got := headless.ExitCode(err); got != 1 {
		t.Errorf("零回复取消的退出码应为 1，实际 %d（err=%v）", got, err)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "error" {
		t.Errorf("status 应为 error，实际 %q", res.Status)
	}
	for _, tr := range res.Turns {
		if tr.Text != "" {
			t.Errorf("取消时不该有任何正文，实际 %q", tr.Text)
		}
	}
	if !strings.Contains(errOut, "尚无成员回复") {
		t.Errorf("stderr 应说明尚无成员回复，实际:\n%s", errOut)
	}
}

// TestAsk_cost_complete含汇总成员 钉复审第 3 条：汇总成员也是这一趟花的钱，
// 它不报金额时 cost_complete 必须是 false，哪怕发言名单全是 claude。
func TestAsk_cost_complete含汇总成员(t *testing.T) {
	cfgPath, _ := writeAskConfigGrok(t, true, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,c3", "-summary", "c2", "-json"}, "")
	if err != nil {
		t.Fatalf("汇总成员不报金额不应让 Run 失败，实际 %v；Err:\n%s", err, errOut)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "ok" {
		t.Errorf("status 应为 ok，实际 %q；Err:\n%s", res.Status, errOut)
	}
	if res.Summary == "" {
		t.Fatalf("汇总成员 c2 应给出正文；Err:\n%s", errOut)
	}
	if res.CostComplete {
		t.Errorf("发言名单全是 claude，但汇总成员 c2 是 grok（不报金额），cost_complete 必须为 false")
	}
}

// TestAsk_cost_complete标注 是复审第 3 条：grok 适配器不报金额，
// cost_complete 必须是 false，但执行本身仍是 ok，不拒绝也不报错。
func TestAsk_cost_complete标注(t *testing.T) {
	cfgPath, _ := writeAskConfigGrok(t, true, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,c2", "-json"}, "")
	if err != nil {
		t.Fatalf("含不报金额成员时 Run 应当成功，实际 %v；Err:\n%s", err, errOut)
	}
	res, raw := decodeSingleResult(t, out)
	if res.Status != "ok" {
		t.Errorf("status 应为 ok，实际 %q；Err:\n%s", res.Status, errOut)
	}
	if res.CostComplete {
		t.Errorf("名单含 grok（ReportsCost=SupportNo）时 cost_complete 应为 false")
	}
	if len(res.Turns) != 2 || res.Turns[1].Outcome != "ok" {
		t.Errorf("两个成员都应正常完成，实际 turns=%+v", res.Turns)
	}
	if res.Turns[1].AgentID != "c2" {
		t.Errorf("turns[1] 应为 c2，实际 %q", res.Turns[1].AgentID)
	}
	if _, ok := raw["cost_complete"]; !ok {
		t.Errorf("原始 JSON 缺少 cost_complete 键；实际：%v", keysOf(raw))
	}
}

// TestAsk_文本模式金额标注 钉文本模式也如实标出不完整金额。
func TestAsk_文本模式金额标注(t *testing.T) {
	cfgPath, _ := writeAskConfigGrok(t, true, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,c2"}, "")
	if err != nil {
		t.Fatalf("文本模式 Run 应当成功，实际 %v；Err:\n%s", err, errOut)
	}
	for _, want := range []string{"状态: ok", "金额:", "（不完全：1 位成员不报金额）", "c1 第 1 轮"} {
		if !strings.Contains(out, want) {
			t.Errorf("文本输出缺少 %q；完整输出:\n%s", want, out)
		}
	}
	if strings.Contains(out, "配置文件:") {
		t.Errorf("文本模式也不应带横幅，实际:\n%s", out)
	}
}

// TestAsk_judge不无声消失：混合名单里的裁判不进 turns，但 stderr 必须
// 点名说这一轮不问它。
func TestAsk_judge不无声消失(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1,j1", "-json"}, "")
	if err != nil {
		t.Fatalf("含裁判的名单应当成功，实际 %v；Err:\n%s", err, errOut)
	}
	res, _ := decodeSingleResult(t, out)
	if len(res.Turns) != 1 || res.Turns[0].AgentID != "c1" {
		t.Errorf("裁判不应发言，turns 应只有 c1，实际 %+v", res.Turns)
	}
	if !strings.Contains(errOut, "裁判 j1 本命令不问") {
		t.Errorf("stderr 应点名裁判不问，实际:\n%s", errOut)
	}
}

// TestAsk_conversation续接：第二次 -conversation 接上同一会话。
func TestAsk_conversation续接(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "first", "-members", "c1", "-json"}, "")
	if err != nil {
		t.Fatalf("第一次 ask 失败: %v；Err:\n%s", err, errOut)
	}
	first, _ := decodeSingleResult(t, out)

	out, errOut, err = runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "second", "-members", "c1", "-conversation", first.ConversationID, "-json"}, "")
	if err != nil {
		t.Fatalf("续接 ask 失败: %v；Err:\n%s", err, errOut)
	}
	second, _ := decodeSingleResult(t, out)
	if second.ConversationID != first.ConversationID {
		t.Errorf("续接应复用会话 %q，实际 %q", first.ConversationID, second.ConversationID)
	}
	if second.Status != "ok" || len(second.Turns) != 1 {
		t.Errorf("续接后应正常完成一轮，实际 status=%q turns=%d", second.Status, len(second.Turns))
	}
}

// TestAsk_未知conversation退1：未知 id 是执行错误，不是用法错误。
func TestAsk_未知conversation退1(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "c1", "-conversation", "conv_nope", "-json"}, "")
	if err == nil {
		t.Fatalf("未知会话应当返回错误；Out:\n%s", out)
	}
	if got := headless.ExitCode(err); got != 1 {
		t.Errorf("未知会话的退出码应为 1，实际 %d（err=%v）", got, err)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "error" {
		t.Errorf("status 应为 error，实际 %q", res.Status)
	}
	if !strings.Contains(errOut, "conv_nope") {
		t.Errorf("stderr 应点名未知会话，实际:\n%s", errOut)
	}
}

// TestAsk_用法错误退2 校验失败：未知成员由 headless.Run 拿到 cfg 后判出，
// 退 2，stderr 点名该 id，stdout 不写任何结果。
func TestAsk_用法错误退2(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, context.Background(), cfgPath,
		[]string{"-p", "q", "-members", "nope", "-json"}, "")
	if err == nil {
		t.Fatalf("未知成员应当返回错误；Out:\n%s", out)
	}
	if got := headless.ExitCode(err); got != 2 {
		t.Errorf("用法错误的退出码应为 2，实际 %d（err=%v）", got, err)
	}
	if out != "" {
		t.Errorf("用法错误时 stdout 应为空，实际:\n%s", out)
	}
	if !strings.Contains(errOut, "nope") {
		t.Errorf("stderr 应点名未知成员 nope，实际:\n%s", errOut)
	}
}
