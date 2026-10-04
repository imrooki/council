package tui

// 本文件覆盖冻结契约 RB-B v1.1 §2.6 的 Q1：finalize 窗口里的 Ctrl-C，真 runWith。
//
// 只在 Windows 上编译（文件名后缀）：退出尾巴的字节钉子与 X-39e A1 相同，只在 Windows 上实测过
// （bubbletea v2.0.9 在非 Windows、无 ttyInput 时把 "\n" 映射成 "\r\n"）。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD a728cee 的源码。
// 装置照 x39e_rig_windows_test.go 的形状但自己搭：LineWriter、假 Submitter、拉宽 400ms 的 Println、真 Program、runWith。
// 源码里不写 ESC 字面量，一律用 tui1dESC 拼。
//
// 本文件引用本刀新增的 Speakers / Results 字段，在 HEAD 上整包编译不过，这是契约 §2 写明的预期红。

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
	"council/internal/turn"
)

// rbbQuitSub 是 §2.6 的假 Submitter：经 LineWriter 写 REPL 文本，经观察者发三种事件。p 在 NewProgram 之后赋值。
type rbbQuitSub struct {
	p  *tea.Program
	lw *LineWriter
}

func (s *rbbQuitSub) Submit(ctx context.Context, text string) error {
	obs := newRoundObserver(s.p.Send)
	ids := []string{"a", "b", "c"}
	_, _ = s.lw.Write([]byte("本轮问 3 个: a, b, c\n"))
	obs.RoundStart(repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 3, Speakers: append([]string(nil), ids...)})
	res := make([]repl.MemberDoneEvent, 0, len(ids))
	for _, id := range ids {
		time.Sleep(30 * time.Millisecond)
		_, _ = s.lw.Write([]byte("outcome spoke  agent " + id + "\n"))
		ev := repl.MemberDoneEvent{Label: "发言轮", AgentID: id, Outcome: turn.OutcomeSpoke, WallMS: 30, RoundID: "0123456789abcdef"}
		obs.MemberDone(ev)
		res = append(res, ev)
	}
	_, _ = s.lw.Write([]byte("发言轮结算\n"))
	obs.RoundEnd(repl.RoundEndEvent{Label: "发言轮", K: 1, N: 1, Results: res})
	return nil
}

func (s *rbbQuitSub) IsQuit(error) bool { return false }

// TestRbbQ1CtrlCBlockedInFinalizeWindow：窗口里连按两次 Ctrl-C 仍被挡；finalize 之后连按两次正常退出，尾巴与 X-39e A1 相同。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里连发两次。
// 窗口里发两次：新语义下这一支既不退出也不武装，连发两次仍应被挡，用来抓住「窗口里不挡」的回归。
//
// 【为什么是子串】终端字节流（契约 §2 通用纪律的例外）。
// 【判别力】能为假：窗口里不挡 Ctrl-C（#56）→ 第 1 条红（100ms 内 runWith 返回；原型实测尾巴里出现 ESC[1 q、
// ?25h 挪到换行之前、帧 11 行、清屏变 ESC[10A）；finalize 不清光标钉（#59）→ 退出时仍带真光标，
// 第 3 条的 pinTail / x39eClear(8) 红。对照：X-39e A1（没有看板时同一尾巴）。
func TestRbbQ1CtrlCBlockedInFinalizeWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	buf := &tui1dBuf{}
	lw := NewLineWriter(nil)
	sub := &rbbQuitSub{lw: lw}
	m := New(Options{Submit: sub, Out: lw, Version: "t", Ctx: ctx, Println: rbbWiden(400 * time.Millisecond)})
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(buf),
		tea.WithoutSignalHandler(),
		tea.WithWindowSize(80, 24),
	)
	sub.p = p
	done := make(chan error, 1)
	go func() { done <- runWith(p, lw, buf) }()

	rule := strings.Repeat("─", 80)
	if !tui1dWaitFor(buf, 5*time.Second, func(raw string) bool { return strings.Contains(tui1bStripANSI(raw), rule) }) {
		t.Fatalf("前提不成立：5 秒内缓冲里没有出现 80 个字符的横线；缓冲（去 ANSI）%q", tui1bStripANSI(buf.String()))
	}
	p.Send(tea.PasteMsg{Content: "/round a b c --go"})
	p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !tui1dWaitFor(buf, 5*time.Second, func(raw string) bool { return strings.Contains(tui1bStripANSI(raw), "/round 结束") }) {
		t.Fatalf("前提不成立：5 秒内缓冲里没有出现收尾行「/round 结束」；缓冲末 300 字节 %q", tui1dTail(buf.String(), 300))
	}

	// 第 1 条。窗口里连发两次，两次都不退出、不武装。
	p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	select {
	case err := <-done:
		t.Fatalf("finalize 窗口里的 Ctrl-C 应当被挡住，runWith 却在 100ms 内返回了（err=%v）；缓冲末 300 字节 %q", err, tui1dTail(buf.String(), 300))
	case <-time.After(100 * time.Millisecond):
	}

	// 第 2 条。收口之后空闲，连发两次才退出。
	time.Sleep(500 * time.Millisecond)
	p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("finalize 之后的空闲 Ctrl-C：runWith 应当返回 nil，实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("finalize 之后的空闲 Ctrl-C：5 秒内 runWith 没有返回；缓冲末 300 字节 %q", tui1dTail(buf.String(), 300))
	}

	// 第 3 条。
	got := buf.String()
	pinHead := "council t   正在退出" + tui1dESC + "[21G"
	pinTail := "\r\n\n\n\n\n\n\n" + tui1dESC + "[J" + tui1dESC + "[?25h" + tui1dESC + "[?2004l"
	idx := strings.LastIndex(got, pinTail)
	if idx < 0 {
		t.Fatalf("退出尾巴的形状变了（光标不在帧末行、或 ?25h 挪了位置）：缓冲末 300 字节 %q", tui1dTail(got, 300))
	}
	if !strings.HasSuffix(got[:idx], pinHead) {
		t.Errorf("pinTail 之前应当以 %q 结尾（最后一帧是 8 行、表头「正在退出」），实际末 120 字节 %q", pinHead, tui1dTail(got[:idx], 120))
	}
	if rest := got[idx+len(pinTail):]; rest != x39eClear(8) {
		t.Errorf("pinTail 之后的整串应当恰好是 %q（帧高 8），实际 %q", x39eClear(8), rest)
	}
}
