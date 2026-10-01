package tui

// 本文件覆盖冻结契约 RB-A v1.2 §2.6 的 W1：真 Run，担保 run.go 里装观察者的那一行真的接上了。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3143bc8 的源码。
//
// 形状照 tui1d_run_ctx_test.go：os.Pipe 两端做 in / stdout，读 goroutine 在 Run 之前起；
// 收尾顺序也照它（cancel、等 Run、关 inW、inR、outW、等读 goroutine、关 outR，每步 5 秒超时）。
// 管道上渲染器拿不到终端尺寸，活区按 0 列裁切、看不到表头文字；往 inW 写一条 XTWINOPS 窗口尺寸报告
// ESC[8;24;100t，ultraviolet 解成 WindowSizeEvent、bubbletea 转成 WindowSizeMsg，渲染器才按 100x24 出帧。
// Run 的选项冻结，不许加 WithWindowSize，所以这是管道上唯一能让表头文字进 stdout 的办法。

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// rbaFlat 是 W1 比对用的形状：剥掉 ANSI 转义，再删掉所有 ASCII 空格
// （渲染器可能用光标移动跳过没变的空格，不押整行重写）。
func rbaFlat(raw string) string {
	return strings.ReplaceAll(tui1bStripANSI(raw), " ", "")
}

// TestRbaW1RunWiresObserver：真 Run 下，/round 在飞时表头出现段信息，并且计时走了一拍。
//
// 【为什么是子串】这是终端字节流，不是一行字符串；断言的串是一整个表头去掉空格后的全部内容，
// 前后都有别的帧。这是本刀唯一的子串断言。
// 【判别力】删掉 run.go 里 F 那一行（#19）、或装一个 send 是空函数的适配器（#21）→ 表头永远没有段，
// 第 3 条红，而第 1、2 条（前提）照样绿；E5 从不装 tick（#32）→ 第 4 条红；`n > 1` 写成 `n >= 1`（#39）
// → 表头是「发言轮1/1…」，第 3 条红。F 的 r != nil 门被删由既有 TestTui1dRunFinalIsRealStdout 抓。
// 【判别不了的】F 那一行写成 go p.Send 或经带缓冲 channel 转发（契约 §4「只在审查时看」）：本条照样绿。
func TestRbaW1RunWiresObserver(t *testing.T) {
	t.Setenv("RBA_FAKE_SLEEP_MS", "2500")
	g := rbaNewTuiRig(t)

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提不成立：os.Pipe() 造输入管道失败：%v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = inW.Close()
		_ = inR.Close()
		t.Fatalf("前提不成立：os.Pipe() 造输出管道失败：%v", err)
	}

	// 读 goroutine 必须在 Run 之前起来：管道缓冲有限，没人读时渲染器会堵住。
	buf := &tui1dBuf{}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(buf, outR)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- Run(ctx, g.r, g.lw, "test", inR, outW) }()

	tornDown := false
	teardown := func() {
		if tornDown {
			return
		}
		tornDown = true
		cancel()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
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

	flatHas := func(d time.Duration, want string) bool {
		return tui1dWaitFor(buf, d, func(raw string) bool { return strings.Contains(rbaFlat(raw), want) })
	}
	write := func(what, s string) {
		t.Helper()
		if _, err := inW.Write([]byte(s)); err != nil {
			t.Fatalf("前提不成立：往输入管道写 %s 失败：%v", what, err)
		}
	}

	// 第 1 条（前提）：事件循环起来了，窗口尺寸报告让表头文字进了 stdout。
	if !tui1dWaitFor(buf, 5*time.Second, func(raw string) bool { return strings.Contains(raw, tui1dESC+"[J") }) {
		t.Fatalf("前提不成立：5 秒内 stdout 管道里没有出现渲染器的 ESC[J；内容 %q", buf.String())
	}
	write("窗口尺寸报告", tui1dESC+"[8;24;100t")
	if !flatHas(5*time.Second, "counciltest空闲") {
		t.Fatalf("前提不成立：写入窗口尺寸报告后 5 秒内没有看到空闲表头；去 ANSI 去空格后的内容 %q", rbaFlat(buf.String()))
	}

	// 第 2 条（前提）：输入经管道进到 REPL。
	write("/use", "/use "+g.convID+"\r")
	if !flatHas(5*time.Second, "当前会话:") {
		t.Fatalf("前提不成立：写入 /use 后 5 秒内没有看到「当前会话:」；内容 %q", rbaFlat(buf.String()))
	}

	// 第 3 条（主断言）：真 Run 装上了观察者，表头出现段信息。
	write("/round", "/round rba_a rba_b --go\r")
	if !flatHas(10*time.Second, "counciltest正在运行发言轮回来0/2") {
		t.Fatalf("写入 /round 后 10 秒内表头没有出现段信息「council test   正在运行  发言轮  回来 0/2」："+
			"run.go 没有在 Run 里把观察者装到 REPL 上（或装的适配器不投递）。去 ANSI 去空格后的内容 %q", rbaFlat(buf.String()))
	}

	// 第 4 条（主断言）：真 Run 上 E5 的 tick 装上了、跑了一拍。
	if !flatHas(10*time.Second, "counciltest正在运行发言轮回来0/21s") {
		t.Errorf("10 秒内表头没有走到 1s（「council test   正在运行  发言轮  回来 0/2  1s」）：计时没有装上或没有续订。"+
			"去 ANSI 去空格后的内容 %q", rbaFlat(buf.String()))
	}

	// 第 5 条（收尾前）：这一轮跑完，子进程都回来了，不留给别的用例。
	if !flatHas(10*time.Second, "发言轮结算") {
		t.Errorf("10 秒内没有看到「发言轮结算」：这一轮没有跑完；内容 %q", rbaFlat(buf.String()))
	}

	teardown()
}
