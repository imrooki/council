package tui

// 本文件覆盖 AT1 冻结契约（E:\multi_model_cowork\at1_contract.md，v1.1 + W1b 修订）§2.3 的 P1：
// 真 tea.Program、真 REPL、真库；候选经真 replAdapter 在 Submit 的 goroutine 上取回、经 submitDoneMsg 进事件循环。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3d134ee 的源码。
// 本文件引用本刀新增的 atLines、mentionSource、replAdapter.MentionCandidates，在 HEAD 上整包编译不过，
// 这是契约 §2.0 写明的预期红。
//
// 装置（冻结）：rbaNewTuiRig（成员 rba_a、rba_b、rba_j，显示名都是 X，没有总结成员）；
// 计数 Submitter 包住 replAdapter，MentionCandidates 委托 replAdapter（不直接调 g.r），这样 run.go 的那个方法有担保。

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
)

// atxCountSub 是 P1 的计数 Submitter：Submit 在锁里记下文字再委托 inner。
type atxCountSub struct {
	inner replAdapter

	mu    sync.Mutex
	texts []string
}

// 编译期核对：计数 Submitter 满足 mentionSource，submitCmd 才会经它取候选。
var _ mentionSource = (*atxCountSub)(nil)

func (s *atxCountSub) Submit(ctx context.Context, text string) error {
	s.mu.Lock()
	s.texts = append(s.texts, text)
	s.mu.Unlock()
	return s.inner.Submit(ctx, text)
}

func (s *atxCountSub) IsQuit(err error) bool { return s.inner.IsQuit(err) }

func (s *atxCountSub) MentionCandidates(ctx context.Context) ([]repl.MentionCandidate, error) {
	return s.inner.MentionCandidates(ctx)
}

func (s *atxCountSub) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

// atxEntry 是 spy 记下的一条 Update：种类 KEY 或 DONE，外加 Update 之后的行、值、表头。
type atxEntry struct {
	kind   string
	lines  []string
	value  string
	header string
}

func (e atxEntry) String() string {
	return fmt.Sprintf("%s 行=%q 值=%q | %s", e.kind, e.lines, e.value, e.header)
}

// atxSpy 包住 Model：Init、View 委托；Update 先委托、断回 Model 存回、再记日志、返回 spy 自己。
// 只记 tea.KeyPressMsg 与 submitDoneMsg。Update / View 只在事件循环 goroutine 上跑；日志加锁。
type atxSpy struct {
	m   Model
	mu  sync.Mutex
	log []atxEntry
}

func (s *atxSpy) Init() tea.Cmd  { return s.m.Init() }
func (s *atxSpy) View() tea.View { return s.m.View() }

func (s *atxSpy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := s.m.Update(msg)
	if nm, ok := next.(Model); ok {
		s.m = nm
	}
	var kind string
	switch msg.(type) {
	case tea.KeyPressMsg:
		kind = "KEY"
	case submitDoneMsg:
		kind = "DONE"
	default:
		return s, cmd
	}
	e := atxEntry{kind: kind, lines: s.m.atLines(), value: s.m.ta.Value(), header: s.m.chrome()[0]}
	s.mu.Lock()
	s.log = append(s.log, e)
	s.mu.Unlock()
	return s, cmd
}

func (s *atxSpy) entries() []atxEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]atxEntry(nil), s.log...)
}

func atxCountKind(log []atxEntry, kind string) int {
	n := 0
	for _, e := range log {
		if e.kind == kind {
			n++
		}
	}
	return n
}

func atxDump(log []atxEntry) string {
	var b strings.Builder
	for i, e := range log {
		fmt.Fprintf(&b, "%3d %s\n", i, e.String())
	}
	return b.String()
}

// atxWaitLog 在 d 之内轮询日志，cond 为真返回 true。
func atxWaitLog(spy *atxSpy, d time.Duration, cond func(log []atxEntry) bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond(spy.entries()) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestAtxProgramCompletes 是 P1。
//
// 【判别力】这是跨并发原语的那一条：候选经真 replAdapter → 真 REPL → 真库，在 Submit 的 goroutine 上取回、
// 经 submitDoneMsg 进事件循环；按键经真事件循环。A36（replAdapter 返回 nil）→ 第一句行为 nil；
// A28 / A29 / A30 / A37 → 第 b+2 条的值或行不对（A37 下回车提交了，值是空串）；A7 → 第 b+4 条行不为 nil。
// 「回车没有提交」是缺席断言（等 300ms 之后收到的文字仍只有 /use、DONE 仍 1 条），
// 对照是最后同装置那次回车真的提交（收到 "@rba_b @"），证明这个装置里回车确实会走到 Submit。
func TestAtxProgramCompletes(t *testing.T) {
	g := rbaNewTuiRig(t)
	sub := &atxCountSub{inner: replAdapter{r: g.r}}

	ctx, cancel := context.WithCancel(context.Background())
	m := New(Options{Submit: sub, Out: g.lw, Version: "t", Ctx: ctx})
	spy := &atxSpy{m: m}
	p := tea.NewProgram(spy,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutSignalHandler(),
		tea.WithWindowSize(100, 24),
	)
	g.r.SetRoundObserver(newRoundObserver(p.Send))
	g.lw.SetEmit(func(line string) { p.Send(printLine(line)) })
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_, _ = p.Run()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：cancel 之后 5 秒 Program.Run 仍没有返回")
		}
	})

	// send 是「发 k」：p.Send 之后等日志条数加一（5 秒，前提）。
	send := func(what string, k tea.Msg) {
		t.Helper()
		n := len(spy.entries())
		p.Send(k)
		if !atxWaitLog(spy, 5*time.Second, func(log []atxEntry) bool { return len(log) >= n+1 }) {
			t.Fatalf("前提不成立：发 %s 之后 5 秒日志没有多出一条；日志:\n%s", what, atxDump(spy.entries()))
		}
	}

	useCmd := "/use " + g.convID
	p.Send(tea.PasteMsg{Content: useCmd})
	p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !atxWaitLog(spy, 10*time.Second, func(log []atxEntry) bool { return atxCountKind(log, "DONE") >= 1 }) {
		t.Fatalf("前提不成立：10 秒内没有等到 /use 的 submitDone；日志:\n%s", atxDump(spy.entries()))
	}
	b := len(spy.entries())

	send("@", tea.KeyPressMsg{Code: '@', Text: "@"})
	log := spy.entries()
	wantOpen := []string{"> @rba_a  X", "  @rba_b  X", "  @rba_j  X"}
	if !reflect.DeepEqual(log[b].lines, wantOpen) {
		t.Errorf("敲 @：第 %d 条的行应当是 %q，实际 %q；日志:\n%s", b, wantOpen, log[b].lines, atxDump(log))
	}

	send("↓", tea.KeyPressMsg{Code: tea.KeyDown})
	send("回车", tea.KeyPressMsg{Code: tea.KeyEnter})
	log = spy.entries()
	if e := log[b+2]; e.value != "@rba_b " || e.lines != nil || e.header != "council t   空闲" {
		t.Errorf("↓ 回车：第 %d 条应当是 值 %q、行 nil、表头 %q，实际 %s；日志:\n%s",
			b+2, "@rba_b ", "council t   空闲", e.String(), atxDump(log))
	}

	send("@", tea.KeyPressMsg{Code: '@', Text: "@"})
	send("Esc", tea.KeyPressMsg{Code: tea.KeyEscape})
	log = spy.entries()
	if e := log[b+4]; e.value != "@rba_b @" || e.lines != nil {
		t.Errorf("@ Esc：第 %d 条应当是 值 %q、行 nil，实际 %s；日志:\n%s", b+4, "@rba_b @", e.String(), atxDump(log))
	}

	time.Sleep(300 * time.Millisecond)
	if got, want := sub.got(), []string{useCmd}; !reflect.DeepEqual(got, want) {
		t.Errorf("补全用的回车不应当提交：收到的文字应当整列是 %q，实际 %q", want, got)
	}
	if n := atxCountKind(spy.entries(), "DONE"); n != 1 {
		t.Errorf("补全用的回车不应当提交：DONE 应当恰好 1 条，实际 %d 条；日志:\n%s", n, atxDump(spy.entries()))
	}

	// 对照：列表收起之后再回车，真的提交。
	p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !atxWaitLog(spy, 10*time.Second, func(log []atxEntry) bool { return atxCountKind(log, "DONE") >= 2 }) {
		t.Fatalf("对照：10 秒内没有等到第 2 条 submitDone；日志:\n%s", atxDump(spy.entries()))
	}
	if got, want := sub.got(), []string{useCmd, "@rba_b @"}; !reflect.DeepEqual(got, want) {
		t.Errorf("对照：收到的文字应当整列是 %q，实际 %q", want, got)
	}
}
