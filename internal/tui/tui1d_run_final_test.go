package tui

// 本文件补 Run 这一层的一处零覆盖：Run 交给 runWith 的第三个参数（final）必须是 stdout 本身。
//
// runWith 的第 1 步在 p.Run() 之前把 LineWriter 里 pending 的启动行（启动横幅、在飞对账、
// 漂移通知）直接写到 final。审查员实测：把 run.go 里的 runWith(p, out, stdout) 改成
// runWith(p, out, io.Discard)，tui 与 app 两个包全绿 —— 启动行整批消失而没有任何用例发现。
// 此前的看法是「Run 收的是真句柄，单测进不去」。但 Run 收的是 *os.File，os.Pipe() 给的
// 也是 *os.File，本文件就走这条路。
//
// ─── 为什么管道两端能把真 Run 调起来（bubbletea v2.0.9 源码，Windows） ─────────
//
//   - tty_windows.go 的 initInput：两个分支都先判 term.IsTerminal(fd)。管道句柄上
//     GetConsoleMode 失败，于是不 MakeRaw、不 SetConsoleMode，ttyInput / ttyOutput 都是 nil，
//     真控制台一个字节都不碰；tea.go 的 Run 也因此跳过 GetSize，宽高是 0，
//     与 A1 / A2 用缓冲做输出时一样。
//   - tea.go 的 Run 只在 input 为 nil 时才去开 TTY；这里 input 是 inR，非 nil。
//   - ultraviolet 的 NewCancelReader：Fd 不等于 os.Stdin.Fd()，退到 muesli/cancelreader 的
//     NewReader，同一个判断再退到 fallbackCancelReader。它的 Cancel 恒返回 false，
//     所以 shutdown 不等读循环，Run 返回时 readLoop 那个 goroutine 还堵在 inR 的 Read 上。
//     它只在管道读到 EOF 时退出 —— 收尾必须先关写端 inW，光关 inR 解不开那次 Read。
//   - r 传 nil：replAdapter 只在 Submit 里解引用 r，IsQuit 走包级函数 repl.IsQuit。
//     本用例不往 inW 写任何字节，不会有提交。

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestTui1dRunFinalIsRealStdout：真 Run、管道做 in 与 stdout，启动行在事件循环之前直写 stdout。
//
// 【判别力，逐句】
//   - 5 秒内去 ANSI 后依次出现「横幅甲」「横幅乙」：final 换成 io.Discard（M-final-discard）
//     或 os.Stderr（M-final-stderr）时，启动行根本不进 outW，管道里只有渲染器的输出，红。
//     第 1 步改回 p.Send(printLine(line))（M-step1-via-send，TUI-1d 修掉的死锁）时，
//     Send 在 Run 之前、同一个 goroutine 上永远堵住，管道里一个字都没有，5 秒截止红；
//     t.Cleanup 里的 cancel 让 Send 从 ctx.Done 那一支回来，包不会挂住。
//     断言能不能为假：能 —— 管道 outW 只有两个写者：第 1 步的 Fprintln(final, ...) 与
//     渲染器；三个变异都让前者不写 outW，而渲染器不会凭空印出 pending 里的行。
//   - 哨兵 HELLO 带着 ESC[L 前缀出现：证明这台装置里经事件循环的 insertAbove 确实打到了
//     outW，下一句「横幅在第一个 ESC[L 之前」因此不是在零个 ESC[L 上空转。
//   - 原始字节恰好以「横幅甲\n横幅乙\n」开头，两条横幅都在第一个 ESC[L 之前、自身不带
//     ESC[L 前缀：钉住横幅是第 1 步直写的、不是经事件循环补吐的。第 1 步与 p.Run() 在同一个
//     goroutine 上先后执行，Run 开始之前没有任何别的写者碰 outW，所以开头这一句是确定性的。
//   - cancel 之后 5 秒内 Run 返回：错误值只记录不强求（预期是 ErrProgramKilled 包着
//     context.Canceled）。
//
// 【第一句判别不了、由第三句拦的】启动行经事件循环补吐、不死锁的写法（第 1 步的出口换成
// go p.Send(printLine(line))）：横幅照样出现在管道里，第一句可能是绿的；但它们带着 ESC[L 前缀、
// 排在渲染器的首帧字节之后，第三句的「开头」「第一个 ESC[L 之前」「无前缀」三处都红（已实测）。
//
// 【判别不了的】Run 传给 tea.WithOutput 的句柄换成别的、而 final 仍是 stdout：句一、句三照样绿，
// 句二的哨兵会在 5 秒截止红，但那是顺带的，本文件不为它负责，也没有实测。
func TestTui1dRunFinalIsRealStdout(t *testing.T) {
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提没成立：os.Pipe() 造输入管道失败：%v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = inW.Close()
		_ = inR.Close()
		t.Fatalf("前提没成立：os.Pipe() 造输出管道失败：%v", err)
	}

	// 读 goroutine 必须在 Run 之前起来：管道缓冲有限，没人读时第 1 步的直写和渲染器都会堵住。
	buf := &tui1dBuf{}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(buf, outR)
	}()

	ctx, cancel := context.WithCancel(context.Background())

	// 启动横幅在 Run 之前写进 LineWriter：出口还是 nil，两条完整行进 pending。
	lw := NewLineWriter(nil)
	tui1cWrite(t, lw, "横幅甲\n横幅乙\n")

	runDone := make(chan error, 1)
	go func() { runDone <- Run(ctx, nil, lw, "test", inR, outW) }()

	// 收尾顺序：取消根 ctx，等 Run 返回，先关 inW（让 readLoop 的 Read 读到 EOF 退出），
	// 再关 inR；关 outW 让读 goroutine 的 io.Copy 读到 EOF，等它结束，最后关 outR。
	// 每一步等待都带超时，坏实现下也不会把整包挂住。
	var runErr error
	runReturned := false
	waitRun := func() bool {
		if runReturned {
			return true
		}
		select {
		case runErr = <-runDone:
			runReturned = true
		case <-time.After(5 * time.Second):
		}
		return runReturned
	}
	tornDown := false
	teardown := func() {
		if tornDown {
			return
		}
		tornDown = true
		cancel()
		if !waitRun() {
			t.Errorf("收尾：cancel() 之后 5 秒 Run 仍没有返回")
		}
		_ = inW.Close()
		_ = inR.Close()
		_ = outW.Close()
		select {
		case <-readDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：关掉 outW 之后 5 秒读 goroutine 仍没有结束")
		}
		_ = outR.Close()
	}
	t.Cleanup(teardown)

	// 句一：启动横幅出现在 stdout 管道里。
	if !tui1dWaitFor(buf, 5*time.Second, tui1dBannersShown) {
		t.Fatalf("5 秒内 stdout 管道里（去 ANSI）没有依次出现 %q、%q。\n"+
			"Run 必须把 stdout 本身作为 final 交给 runWith：第 1 步在 p.Run() 之前把 pending 的启动行直写 final；\n"+
			"final 换成 io.Discard 或别的句柄，启动横幅就整批消失；第 1 步改经 p.Send，则在 Run 之前同一个 goroutine 上堵死。\n"+
			"管道里的内容（去 ANSI）是 %q", "横幅甲", "横幅乙", tui1bStripANSI(buf.String()))
	}

	// 句二：哨兵经事件循环的 insertAbove 打出，带 ESC[L 前缀。
	tui1cWrite(t, lw, "HELLO\n")
	if !tui1dWaitFor(buf, 5*time.Second, func(raw string) bool {
		return strings.Contains(raw, tui1dIL+"HELLO")
	}) {
		raw := buf.String()
		t.Fatalf("哨兵 HELLO 5 秒内没有带着 ESC[L 前缀出现在 stdout 管道里：装置里看不到经事件循环的插入，\n"+
			"下面「横幅在第一个 ESC[L 之前」那一句会空转。去 ANSI 后有没有 HELLO：%v。原始输出是 %q",
			strings.Contains(tui1bStripANSI(raw), "HELLO"), raw)
	}

	// 句三：横幅是第 1 步直写的，不是经事件循环补吐的。
	raw := buf.String()
	if want := "横幅甲\n横幅乙\n"; !strings.HasPrefix(raw, want) {
		t.Errorf("stdout 管道的原始字节应当恰好以 %q 开头（第 1 步在 p.Run() 之前直写，那时没有别的写者），\n"+
			"实际开头是 %q", want, raw[:min(len(raw), 80)])
	}
	firstIL := strings.Index(raw, tui1dIL)
	for _, banner := range []string{"横幅甲", "横幅乙"} {
		i := strings.Index(raw, banner)
		if i < 0 {
			t.Errorf("原始字节里没有 %q", banner)
			continue
		}
		if firstIL < 0 || i > firstIL {
			t.Errorf("%q 在原始字节的 %d，第一个 ESC[L 在 %d：启动横幅必须在任何经事件循环的插入之前出现",
				banner, i, firstIL)
		}
		if i >= len(tui1dIL) && raw[i-len(tui1dIL):i] == tui1dIL {
			t.Errorf("%q 前面紧挨着 ESC[L：它是经事件循环的 insertAbove 打出来的，不是第 1 步直写 final 的", banner)
		}
	}

	// 句四：取消根 ctx 之后 Run 返回。
	cancel()
	if !waitRun() {
		t.Fatalf("cancel() 之后 5 秒 Run 没有返回")
	}
	t.Logf("Run 返回 %v", runErr)

	teardown()
}
