package tui

// 本文件是冻结契约 RB-A v1.2（E:\multi_model_cowork\rb_a_contract.md）§2.0 / §2.3 的 tui 包装置：
// TestMain、假 CLI、JSON 常量、真 REPL 装置与真 Program 装置（带日志的 spy）。只放装置，不放用例。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3143bc8 的源码。
// 本文件引用本刀新增的 roundStartMsg 等标识符，在 HEAD 上整包编译不过，这是契约 §2 写明的预期红。
//
// 假 CLI 的套路与 internal/repl/session_inflight_test.go 相同：测试二进制自己在
// COUNCIL_FAKE_CLI=1 时扮演 claude CLI，config 里 Providers["claude"].Command 写 os.Args[0]。
// 子进程的环境原样继承（internal/turn/spawn.go 的 cmd.Env = nil），所以用例里 t.Setenv 的
// RBA_FAKE_SLEEP_MS / RBA_FAKE_RESUME_SESSION 能传到假 CLI。本包此前没有 TestMain（现查零命中）。

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/repl"
	"council/internal/store"
	"council/internal/turn"
)

// rbaFakeClaudeSuccessJSON 逐字节抄自 internal/repl/session_inflight_test.go 的
// fakeClaudeSuccessJSON（那一份又逐字来自 internal/turn/fixtures_test.go 的 claudeT1JSON，
// 最终来自 E:\Isolating_test\council\step0-probe\claude-t1.json，claude 冷启动首轮的真实抓包）。
// 不手写：internal/provider/claude.go 的 ParseTurn 是 fail-closed 的五字段合取判据，
// 手写的 JSON 极容易漏掉其中一个字段而不自知。三处（现在是多处）独立复制、不跨包引用是既有约定。
// 这是第 5 份拷贝：internal/provider/fixtures_test.go、internal/turn/fixtures_test.go、
// internal/app/smoke_test.go、internal/repl/session_inflight_test.go 之后，本文件是第五处；
// 将来重新抓 claude-t1.json 时五处要一起改。
const rbaFakeClaudeSuccessJSON = `{"duration_api_ms":1939,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":0.9551999999999999,"usage":{"input_tokens":2,"cache_creation_input_tokens":47749,"cache_read_input_tokens":0,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":47749,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":47749,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":47749},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":2,"outputTokens":4,"cacheReadInputTokens":0,"cacheCreationInputTokens":47749,"webSearchRequests":0,"costUSD":0.9551999999999999,"contextWindow":1000000,"maxOutputTokens":64000,"thinkingTokens":0,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"OK","ttft_ms":2016,"type":"result","duration_ms":3566,"uuid":"78205525-edf7-485a-b1d4-c9cbccd75462","ttft_stream_ms":1306,"time_to_request_ms":98,"first_content_frame_ms":1307,"queued_turn_count":0,"result_index":0}`

// rbaFakeSessionID 是上面 JSON 里的 session_id，RBA_FAKE_RESUME_SESSION 替换的就是它。
const rbaFakeSessionID = "2a90d449-a348-497a-8812-e73e74d6898e"

// TestMain：COUNCIL_FAKE_CLI=1 时扮演一次 claude CLI 调用，否则原样跑用例集。
// 对本包其余既有用例零影响（它们不设这个环境变量）。
func TestMain(m *testing.M) {
	if os.Getenv("COUNCIL_FAKE_CLI") == "1" {
		rbaFakeCLI()
		return
	}
	os.Exit(m.Run())
}

// rbaFakeCLI 扮演一次 claude CLI 调用：读完 stdin；RBA_FAKE_SLEEP_MS 能解析成正整数时先睡；
// RBA_FAKE_RESUME_SESSION 非空且 argv 里有 --resume 时，把 JSON 里的会话 id 换成它（只换第一次出现）；
// 打出 JSON，退出码 0。
func rbaFakeCLI() {
	_, _ = io.ReadAll(os.Stdin)
	if ms, err := strconv.Atoi(os.Getenv("RBA_FAKE_SLEEP_MS")); err == nil && ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	out := rbaFakeClaudeSuccessJSON
	if sid := os.Getenv("RBA_FAKE_RESUME_SESSION"); sid != "" {
		for _, a := range os.Args[1:] {
			if a == "--resume" {
				out = strings.Replace(out, rbaFakeSessionID, sid, 1)
				break
			}
		}
	}
	fmt.Fprint(os.Stdout, out)
	os.Exit(0)
}

// ─── 真 REPL 装置 ─────────────────────────────────────────────────────

// rbaTuiRig 是 tui 包的真 REPL 装置：真库、真 Runner、真 REPL，REPL 的 out 是 *LineWriter。
type rbaTuiRig struct {
	svc    *conversation.Service
	rn     *turn.Runner
	r      *repl.REPL
	lw     *LineWriter
	convID string
}

// rbaTuiConfig 与 repl 包装置同一份配置：rba_a、rba_b 讨论模式，rba_j 裁判模式。
func rbaTuiConfig() *config.Config {
	agent := func(mode config.Mode) config.AgentConfig {
		return config.AgentConfig{
			DisplayName: "X",
			Provider:    "claude",
			Model:       "claude-fable-5-1",
			Effort:      "high",
			Role:        "rba",
			Permissions: config.PermissionRead,
			Mode:        mode,
		}
	}
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: os.Args[0]},
		},
		Agents: map[string]config.AgentConfig{
			"rba_a": agent(config.ModeDiscussion),
			"rba_b": agent(config.ModeDiscussion),
			"rba_j": agent(config.ModeJudge),
		},
	}
}

// rbaNewTuiRig 搭装置：Setenv、store.Open 到 t.TempDir() 下、Runner、建会话、
// 按 rba_a/rba_b/rba_j 加入、发 seed-msg、造 REPL（out 是新 LineWriter，出口为 nil）。
// svc 与 convID 交回给用例（O3b 要调 svc.Leave）。
func rbaNewTuiRig(t *testing.T) *rbaTuiRig {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "council-data"))
	if err != nil {
		t.Fatalf("装置：store.Open 失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("装置：关闭 store 失败: %v", err)
		}
	})
	svc := conversation.NewService(st)
	cfg := rbaTuiConfig()
	rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})

	conv, err := svc.Create(ctx, "rba")
	if err != nil {
		t.Fatalf("装置：建会话失败: %v", err)
	}
	for _, id := range []string{"rba_a", "rba_b", "rba_j"} {
		if err := svc.Join(ctx, conv.ID, conversation.Participant{
			AgentID: id, Provider: "claude", JoinedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("装置：加入 %s 失败: %v", id, err)
		}
	}
	if _, err := svc.Post(ctx, conversation.Message{
		ConvID: conv.ID, SenderID: conversation.ReservedSenderUser,
		Kind: conversation.KindUser, Text: "seed-msg",
	}); err != nil {
		t.Fatalf("装置：发种子消息失败: %v", err)
	}

	lw := NewLineWriter(nil)
	r := repl.New(cfg, svc, rn, nil, lw, repl.RulesContext{})
	return &rbaTuiRig{svc: svc, rn: rn, r: r, lw: lw, convID: conv.ID}
}

// ─── 真 Program 装置：带日志的 spy ─────────────────────────────────────

// rbaEntry 是 spy 记下的一条 Update。kind 取 P（printLine 正文）、RS、MD、RE、DONE、TICK。
// header 是这条 Update 之后的 chrome()[0]。
type rbaEntry struct {
	kind   string
	text   string
	rs     repl.RoundStartEvent
	md     repl.MemberDoneEvent
	re     repl.RoundEndEvent
	err    error
	tick   elapsedTickMsg
	header string
}

func (e rbaEntry) String() string {
	switch e.kind {
	case "P":
		return fmt.Sprintf("P %q | %s", e.text, e.header)
	case "RS":
		return fmt.Sprintf("RS %+v | %s", e.rs, e.header)
	case "MD":
		return fmt.Sprintf("MD %+v | %s", e.md, e.header)
	case "RE":
		return fmt.Sprintf("RE %+v | %s", e.re, e.header)
	case "DONE":
		return fmt.Sprintf("DONE err=%v | %s", e.err, e.header)
	case "TICK":
		return fmt.Sprintf("TICK gen=%d | %s", e.tick.gen, e.header)
	}
	return e.kind
}

// rbaSpy 包住 Model：Init、View 委托；Update 先委托、把返回值断回 Model 存回去，再按消息类型记一条日志。
// Update 必须返回 spy 自己（指针），否则 bubbletea 在第一次 Update 之后就把 spy 换掉、日志从此静默。
// Update / View 只在事件循环 goroutine 上跑，m 不加锁；日志由测试 goroutine 并发读，加锁。
type rbaSpy struct {
	m   Model
	mu  sync.Mutex
	log []rbaEntry
}

func (s *rbaSpy) Init() tea.Cmd  { return s.m.Init() }
func (s *rbaSpy) View() tea.View { return s.m.View() }

func (s *rbaSpy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := s.m.Update(msg)
	if nm, ok := next.(Model); ok {
		s.m = nm
	}
	var e rbaEntry
	switch v := msg.(type) {
	case roundStartMsg:
		e = rbaEntry{kind: "RS", rs: v.ev}
	case memberDoneMsg:
		e = rbaEntry{kind: "MD", md: v.ev}
	case roundEndMsg:
		e = rbaEntry{kind: "RE", re: v.ev}
	case submitDoneMsg:
		e = rbaEntry{kind: "DONE", err: v.err}
	case elapsedTickMsg:
		e = rbaEntry{kind: "TICK", tick: v}
	default:
		if fmt.Sprintf("%T", msg) != "tea.printLineMessage" {
			return s, cmd
		}
		e = rbaEntry{kind: "P", text: reflect.ValueOf(msg).FieldByName("messageBody").String()}
	}
	e.header = s.m.chrome()[0]
	s.mu.Lock()
	s.log = append(s.log, e)
	s.mu.Unlock()
	return s, cmd
}

func (s *rbaSpy) entries() []rbaEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]rbaEntry(nil), s.log...)
}

// rbaProg 是一台跑着的真 Program。
type rbaProg struct {
	p   *tea.Program
	spy *rbaSpy
}

// rbaStartProg 按 §2.0 的顺序起真 Program：先装观察者，再 SetEmit，再 go p.Run()。
// 收尾 cancel ctx 并等 Run 返回（5 秒超时）。
func rbaStartProg(t *testing.T, g *rbaTuiRig) *rbaProg {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	m := New(Options{Submit: replAdapter{r: g.r}, Out: g.lw, Version: "t", Ctx: ctx})
	spy := &rbaSpy{m: m}
	p := tea.NewProgram(spy,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutSignalHandler(),
		tea.WithWindowSize(100, 24),
	)
	g.r.SetRoundObserver(newRoundObserver(p.Send))
	g.lw.SetEmit(func(line string) { p.Send(printLine(line)) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = p.Run()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：cancel 之后 5 秒 Program.Run 仍没有返回")
		}
	})
	return &rbaProg{p: p, spy: spy}
}

// submit 提交一条命令：粘贴文本再按 Enter。
func (pr *rbaProg) submit(text string) {
	pr.p.Send(tea.PasteMsg{Content: text})
	pr.p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// waitFor 在 d 之内轮询日志，cond 为真返回 true。
func (pr *rbaProg) waitFor(d time.Duration, cond func(log []rbaEntry) bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond(pr.spy.entries()) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// rbaCount 数日志里某种条目的条数。
func rbaCount(log []rbaEntry, kind string) int {
	n := 0
	for _, e := range log {
		if e.kind == kind {
			n++
		}
	}
	return n
}

// waitDone 等到日志里有 n 条 submitDone；等不到就 Fatalf（这是前提，不是断言对象）。
func (pr *rbaProg) waitDone(t *testing.T, n int, d time.Duration) {
	t.Helper()
	if !pr.waitFor(d, func(log []rbaEntry) bool { return rbaCount(log, "DONE") >= n }) {
		t.Fatalf("前提不成立：%v 之内没有等到第 %d 条 submitDone；日志:\n%s", d, n, rbaLogDump(pr.spy.entries()))
	}
}

// useConv 提交 /use 并等到第 1 条 submitDone。
func (pr *rbaProg) useConv(t *testing.T, convID string) {
	t.Helper()
	pr.submit("/use " + convID)
	pr.waitDone(t, 1, 10*time.Second)
}

// rbaLogDump 把日志印成可读形式，失败信息用。
func rbaLogDump(log []rbaEntry) string {
	var b strings.Builder
	for i, e := range log {
		fmt.Fprintf(&b, "%3d %s\n", i, e.String())
	}
	return b.String()
}

// rbaAfterEcho 返回回显那条 P（"> "+cmd）之后的日志；找不到时返回 nil 与 false。
func rbaAfterEcho(log []rbaEntry, cmd string) ([]rbaEntry, bool) {
	for i, e := range log {
		if e.kind == "P" && e.text == "> "+cmd {
			return log[i+1:], true
		}
	}
	return nil, false
}
