package tui

// 本文件覆盖冻结契约「X-39e」（E:\multi_model_cowork\x39e_contract.md，v1.2）
// 第 2.1 节的 A1-A6：真的 *tea.Program 跑 runWith，看缓冲的原始字节尾部。
//
// 只在 Windows 上编译（文件名后缀）：bubbletea v2.0.9 tea.go:1084 在非 Windows、
// 无 ttyInput 时把 "\n" 映射成 "\r\n"，本文件的字节钉子只在 Windows 上实测过。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成。
//
// 断言「不写」一律用这个形状：缓冲里最后一个 ESC[?2004l（bubbletea close() 的最后一步）
// 确实存在，而且它之后没有任何 0x1B。不用子串断言（契约 2 节）。
//
// 源码里不写 ESC 字面量，一律用 tui1dESC 拼。

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// x39eBPOff 是 bubbletea close() 写的最后一个序列：关括号粘贴。
var x39eBPOff = tui1dESC + "[?2004l"

// x39eRigState 是一台跑着 runWith 的真程序。
type x39eRigState struct {
	lw     *LineWriter
	p      *tea.Program
	cancel context.CancelFunc
	done   chan error
	buf    *tui1dBuf
}

// x39eRig 照 tui1dStart 的形状起一台真程序（契约 2.0）。
// sized 为真时加 tea.WithWindowSize(80, 24)，否则宽 0（管道上 GetSize 被跳过）。
// out 是渲染器的输出，final 是 runWith 的兜底出口，buf 是就绪信号所看的缓冲。
func x39eRig(t *testing.T, sub Submitter, sized bool, out io.Writer, final io.Writer, buf *tui1dBuf) *x39eRigState {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	lw := NewLineWriter(nil)
	tui1cWrite(t, lw, "横幅甲\n横幅乙\n")

	m := New(Options{Submit: sub, Out: lw, Version: "t", Ctx: ctx})
	opts := []tea.ProgramOption{
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
	}
	if sized {
		opts = append(opts, tea.WithWindowSize(80, 24))
	}
	p := tea.NewProgram(m, opts...)

	done := make(chan error, 1)
	go func() { done <- runWith(p, lw, final) }()
	return &x39eRigState{lw: lw, p: p, cancel: cancel, done: done, buf: buf}
}

// waitReady 是 sized 装置的就绪信号（契约 2.0，冻结）：去 ANSI 后出现 80 个 ─。
// 横线宽度是 min(max(width,20),120)，没收到 {80,24} 时只有 20 个；
// 出现 80 个说明 Model 已经处理了尺寸，termHeight 是 24。
// 这个等待同时保证启动时排队的 DECRQM 查询已经在第一次 flush 里发出去。
func (r *x39eRigState) waitReady(t *testing.T) {
	t.Helper()
	rule := strings.Repeat("─", 80)
	if !tui1dWaitFor(r.buf, 5*time.Second, func(raw string) bool {
		return strings.Contains(tui1bStripANSI(raw), rule)
	}) {
		t.Fatalf("前提没成立：5 秒内缓冲里没有出现 80 个字符的横线，Model 没处理 {80,24} 或渲染器没出帧。\n"+
			"缓冲（去 ANSI）是 %q", tui1bStripANSI(r.buf.String()))
	}
}

// wait 等 runWith 返回，最多 5 秒。
func (r *x39eRigState) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("runWith 5 秒内没有返回。缓冲末 300 字节是 %q", tui1dTail(r.buf.String(), 300))
		return nil
	}
}

// x39eCtrlC 是空闲时的 Ctrl-C。
func x39eCtrlC() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl} }

// x39eAfterLastBPOff 取缓冲里最后一个 ESC[?2004l 之后的全部字节；标记不存在时 ok 为假。
func x39eAfterLastBPOff(got string) (rest string, ok bool) {
	idx := strings.LastIndex(got, x39eBPOff)
	if idx < 0 {
		return "", false
	}
	return got[idx+len(x39eBPOff):], true
}

// x39eAssertNothingAfterClose 断言「不写」：最后一个 ESC[?2004l 存在，且它之后没有 0x1B。
// 先查标记存在：缺了这一句，标记不在时「之后没有 ESC」恒真。
func x39eAssertNothingAfterClose(t *testing.T, got, why string) {
	t.Helper()
	rest, ok := x39eAfterLastBPOff(got)
	if !ok {
		t.Fatalf("前提没成立：缓冲里没有 ESC[?2004l，bubbletea 的 close() 没走到或形状变了。末 200 字节是 %q",
			tui1dTail(got, 200))
	}
	if strings.IndexByte(rest, 0x1b) >= 0 {
		t.Errorf("%s\n最后一个 ESC[?2004l 之后出现了转义序列：%q。缓冲末 200 字节是 %q", why, rest, tui1dTail(got, 200))
	}
}

// ═══ A1 ═══════════════════════════════════════════════════════════

// A1：空闲连按两次 Ctrl-C（正常退出），bubbletea 收尾之后整串恰好是 x39eClear(8)。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里连发两次。
//
// 【三句三种红，文案各不相同】
//  1. 前提钉子 pinTail：bubbletea close() 的收尾形状（\r 加 7 个 \n 走到帧末行、只擦末行）。
//     升级 bubbletea 后这句先红，t.Fatalf，不再看后面。
//  2. 表头钉子 pinHead：Council 的最后一帧（Version "t"、状态「正在退出」、第 21 列）。
//  3. 主断言：pinTail 之后整串等于 x39eClear(8)。HEAD 上是空串；H 写成 0 或 1 时是 "\r"+ESC[J。
//
// 能不能为假：主断言在 HEAD 上是红的（pinTail 之后什么都没有）。
// 只差一处的对照：A6，同一装置不带 WithWindowSize（宽 0），那里要求一个字节都不写。
func TestX39eRigIdleCtrlCClearsAfterClose(t *testing.T) {
	buf := &tui1dBuf{}
	r := x39eRig(t, tui1dNopSubmitter{}, true, buf, buf, buf)
	r.waitReady(t)

	r.p.Send(x39eCtrlC())
	r.p.Send(x39eCtrlC())
	if err := r.wait(t); err != nil {
		t.Fatalf("前提没成立：空闲连按两次 Ctrl-C 走 QuitMsg，runWith 应当返回 nil，实际是 %v", err)
	}

	got := buf.String()
	pinHead := "council t   正在退出" + tui1dESC + "[21G"
	pinTail := "\r\n\n\n\n\n\n\n" + tui1dESC + "[J" + tui1dESC + "[?25h" + tui1dESC + "[?2004l"

	idx := strings.LastIndex(got, pinTail)
	if idx < 0 {
		t.Fatalf("bubbletea 退出收尾的形状变了（光标不再停在帧末行）；升级 bubbletea 后先看 exitClearSeq 的前提还成不成立。\n"+
			"缓冲末 200 字节是 %q", tui1dTail(got, 200))
	}
	if !strings.HasSuffix(got[:idx], pinHead) {
		t.Errorf("Council 的表头、状态文案或 Version 变了，同步改契约 §3.3 的 pinHead；与 bubbletea 无关。\n"+
			"pinTail 之前的末 120 字节是 %q，应当以 %q 结尾", tui1dTail(got[:idx], 120), pinHead)
	}
	if rest := got[idx+len(pinTail):]; rest != x39eClear(8) {
		t.Errorf("Run 之后没有写对清屏序列：bubbletea 收尾之后的整串是 %q，应当恰好是 %q（帧高 8，上移 7 行）",
			rest, x39eClear(8))
	}
}

// ═══ A2 ═══════════════════════════════════════════════════════════

// A2：取消根 ctx（被杀的路）不清屏；残段照样由第 4 步写到 final。
//
// 【判别力】门 (2) 放行 Killed、或删掉 err 门时，残段之前会多出 ESC[7A\r ESC[J，
// 整个尾部与「最后一个 ESC[?2004l 之后恰好是残段」两句都红。
// HEAD 上绿（HEAD 什么都不写）。
// 这里就绪之后才取消，帧已上屏，放行 Killed「碰巧」擦对了；本条钉的是契约的选择
// （被杀一律不清），真正危险的「第一次 flush 之前被杀」靠同一道门挡住，不追时序。
func TestX39eRigCancelWritesNothing(t *testing.T) {
	buf := &tui1dBuf{}
	r := x39eRig(t, tui1dNopSubmitter{}, true, buf, buf, buf)
	r.waitReady(t)

	tui1cWrite(t, r.lw, "残段乙")
	r.cancel()
	err := r.wait(t)
	if !errors.Is(err, tea.ErrProgramKilled) {
		t.Fatalf("前提没成立：取消根 ctx 之后 runWith 应当返回包着 ErrProgramKilled 的错误，实际是 %v", err)
	}

	got := buf.String()
	wantTail := tui1dESC + "[J" + tui1dESC + "[?25h" + tui1dESC + "[?2004l" + "残段乙\n"
	if !strings.HasSuffix(got, wantTail) {
		t.Errorf("被杀的路不许写清屏：缓冲应当以 %q 结尾，末 200 字节是 %q", wantTail, tui1dTail(got, 200))
	}
	rest, ok := x39eAfterLastBPOff(got)
	if !ok {
		t.Fatalf("前提没成立：缓冲里没有 ESC[?2004l。末 200 字节是 %q", tui1dTail(got, 200))
	}
	if rest != "残段乙\n" {
		t.Errorf("最后一个 ESC[?2004l 之后应当恰好是残段 %q，实际是 %q（被杀的路不清屏，契约 1.1 R1 门 (2)）",
			"残段乙\n", rest)
	}
}

// ═══ A3 ═══════════════════════════════════════════════════════════

// A3：出错退出（Submit 返回错误，Model 自己 Quit）也走到清屏。
//
// 【判别力】删掉清屏时尾部停在 ESC[?2004l，红。HEAD 上红。
// 对照：A4 同样经 Enter 走 Cmd，只是 Submit 里 panic，那里要求一个字节都不写。
func TestX39eRigErrorPathClears(t *testing.T) {
	buf := &tui1dBuf{}
	r := x39eRig(t, &tui1bSubmitter{err: errors.New("坏了")}, true, buf, buf, buf)
	r.waitReady(t)

	r.p.Send(tea.KeyPressMsg{Code: 'x', Text: "x"})
	r.p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if err := r.wait(t); err != nil {
		t.Fatalf("前提没成立：出错退出是 Model 自己 Quit，runWith 应当返回 nil，实际是 %v", err)
	}

	got := buf.String()
	if want := tui1dESC + "[?2004l" + x39eClear(8); !strings.HasSuffix(got, want) {
		t.Errorf("出错退出之后没有写对清屏序列：缓冲应当以 %q 结尾，末 200 字节是 %q", want, tui1dTail(got, 200))
	}
}

// ═══ A4 ═══════════════════════════════════════════════════════════

// x39ePanicSubmitter 在 Submit 里 panic，模拟 Cmd goroutine 里的 panic。
type x39ePanicSubmitter struct{}

func (x39ePanicSubmitter) Submit(context.Context, string) error { panic("x39e") }
func (x39ePanicSubmitter) IsQuit(error) bool                    { return false }

// A4：Cmd goroutine 里 panic，Run 返回真 Model 加 Killed+Panic；不许写清屏。
//
// 【判别力】这是「门 (2) 挡住 panic」的真路径证据：这条路门 (1) 挡不住（Model 是真的）。
// 删掉 err 门时尾部多出 ESC[7A\r ESC[J，红。HEAD 上绿。
// bubbletea 会往 os.Stderr 打 Caught panic 和栈，预期噪声。
func TestX39eRigPanicWritesNothing(t *testing.T) {
	buf := &tui1dBuf{}
	r := x39eRig(t, x39ePanicSubmitter{}, true, buf, buf, buf)
	r.waitReady(t)

	r.p.Send(tea.KeyPressMsg{Code: 'x', Text: "x"})
	r.p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	err := r.wait(t)
	if !(errors.Is(err, tea.ErrProgramKilled) && errors.Is(err, tea.ErrProgramPanic)) {
		t.Fatalf("前提没成立：Cmd 里 panic 时 runWith 应当返回 Killed+Panic，实际是 %v", err)
	}

	x39eAssertNothingAfterClose(t, buf.String(),
		"Cmd goroutine 里 panic 时栈已经写进 stderr、光标位置未知，不许写清屏（契约 1.1 R1 门 (2)）。")
}

// ═══ A5 ═══════════════════════════════════════════════════════════

// A5：final 是管道（*os.File 但不是控制台）时 finalSupportsVT 为假，不写清屏。
//
// 【判别力】finalSupportsVT 恒真、或 runWith 不调它时，尾部多出清屏，红。HEAD 上绿。
// 对照：A1，同样的空闲连按两次 Ctrl-C，final 换成缓冲（非 *os.File 当支持），要写清屏。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里连发两次。
func TestX39eRigPipeFinalSkipsWithoutVT(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提没成立：os.Pipe 失败：%v", err)
	}
	t.Cleanup(func() {
		_ = pw.Close()
		_ = pr.Close()
	})

	buf := &tui1dBuf{}
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(buf, pr)
		close(copyDone)
	}()

	r := x39eRig(t, tui1dNopSubmitter{}, true, pw, pw, buf)
	r.waitReady(t)

	r.p.Send(x39eCtrlC())
	r.p.Send(x39eCtrlC())
	if err := r.wait(t); err != nil {
		t.Fatalf("前提没成立：空闲连按两次 Ctrl-C 走 QuitMsg，runWith 应当返回 nil，实际是 %v", err)
	}
	_ = pw.Close()
	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("关掉管道写端之后 5 秒 io.Copy 仍没有结束")
	}

	got := buf.String()
	if !strings.HasSuffix(got, x39eBPOff) {
		t.Errorf("final 是管道时不许写清屏：缓冲应当以 ESC[?2004l 结尾，末 200 字节是 %q", tui1dTail(got, 200))
	}
	x39eAssertNothingAfterClose(t, got,
		"final 是管道，GetConsoleMode 失败，finalSupportsVT 应当为假（契约 1.1 V1 门 (4)）。")
}

// ═══ A6 ═══════════════════════════════════════════════════════════

// A6：宽 0（不带 WithWindowSize）时渲染器什么都没画，不许写清屏。
//
// 就绪信号改为缓冲里出现 ESC[J（与 tui1d_run_ctx_test.go 同一判据）。
// 就绪后测试自己先发一条 {0,0}、再连发两次 Ctrl-C：同一个 goroutine 上这三次 Send 经无缓冲 chan
// 按先后到达，保证 Model 在 Ctrl-C 之前已经处理过 {0,0}，「门用兜底的 height」这个变异
// 每次都红，不靠排期。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里连发两次。
//
// 【判别力】门 (3) 用 m.height（兜底 24）时尾部多出 ESC[7A\r ESC[J，红。HEAD 上绿。
// 对照：A1，只差 WithWindowSize(80, 24)。
func TestX39eRigZeroWidthWritesNothing(t *testing.T) {
	buf := &tui1dBuf{}
	r := x39eRig(t, tui1dNopSubmitter{}, false, buf, buf, buf)
	if !tui1dWaitFor(buf, 5*time.Second, func(raw string) bool {
		return strings.Contains(raw, tui1dESC+"[J")
	}) {
		t.Fatalf("前提没成立：5 秒内缓冲里没有出现渲染器的 ESC[J。缓冲是 %q", buf.String())
	}

	r.p.Send(tea.WindowSizeMsg{Width: 0, Height: 0})
	r.p.Send(x39eCtrlC())
	r.p.Send(x39eCtrlC())
	if err := r.wait(t); err != nil {
		t.Fatalf("前提没成立：空闲连按两次 Ctrl-C 走 QuitMsg，runWith 应当返回 nil，实际是 %v", err)
	}

	got := buf.String()
	wantTail := tui1dESC + "[A" + tui1dESC + "[J" + tui1dESC + "[?25h" + tui1dESC + "[?2004l"
	if !strings.HasSuffix(got, wantTail) {
		t.Errorf("宽 0 时不许写清屏：缓冲应当以 %q 结尾，末 200 字节是 %q", wantTail, tui1dTail(got, 200))
	}
	x39eAssertNothingAfterClose(t, got,
		"宽 0 时 termHeight 是 0，门 (3) 必须挡住；兜底后的 height（24）不能拿来判（契约 1.1 M2、R1）。")
}
