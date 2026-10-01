package tui

// 本文件覆盖 AT1 冻结契约（E:\multi_model_cowork\at1_contract.md，v1.1 + W1b 修订）§2.4 的 R1：
// 真 tui.Run（os.Pipe 做 in 与 stdout），担保 Run 装上的 replAdapter 取到了候选，且补全用的回车不提交。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3d134ee 的源码。
// 本文件本身不引用新标识符，但与本包其余 atx 文件同包编译，在 HEAD 上一起编译不过（契约 §2.0 的预期红）。
//
// 装置形状照 rba_wiring_test.go：读 goroutine 在 Run 之前起；收尾照它的 :63-85（cancel、等 Run、关 inW、inR、outW、
// 等读 goroutine、关 outR，每步 5 秒超时），挂进 t.Cleanup。管道上要先写一条 XTWINOPS 窗口尺寸报告，表头文字才进 stdout。

import (
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// TestAtxRunPipe 是 R1。
//
// 【为什么是子串】终端字节流，同 RB-A W1：剥 ANSI、删空格之后找子串。
// 【v1.1 的等待步】「当前会话:」是 Submit 里打的，那一刻还在忙，候选随之后的 submitDoneMsg 才到；
// 回车之后表头的「空闲」只由 submitDoneMsg 那一支写回，所以等到第一个「当前会话:」之后出现空闲表头再敲 @，
// 否则 @ 可能落在忙的 Model 上、不开，主断言 1 假红（主代理已定保留，§7 (m)）。
// 只在第一个「当前会话:」之后找：启动时的空闲表头在它前面，整串找是恒真。
// 【判别力】主断言 1 之前有同装置的缺席前提（还没敲 @ 时没有 >@rba_aX）。A36（replAdapter 返回 nil）→ 主断言 1 等不到；
// A37（回车不截）→ 第一次回车提交了 "@"，用户消息多出一条；A28 / A29 → 主断言 2 的正文不对。
// 主断言 2 本身能为假：用户消息整列是 seed-msg 与补全后的整句，两次回车都提交或补全错都不等。
// 只看用户消息，不看 agent 消息（CH1 若把零值改成自动回复，这里仍成立）。
func TestAtxRunPipe(t *testing.T) {
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

	flat := func() string { return rbaFlat(buf.String()) }
	flatHas := func(d time.Duration, want string) bool {
		return tui1dWaitFor(buf, d, func(raw string) bool { return strings.Contains(rbaFlat(raw), want) })
	}
	write := func(what, s string) {
		t.Helper()
		if _, err := inW.Write([]byte(s)); err != nil {
			t.Fatalf("前提不成立：往输入管道写 %s 失败：%v", what, err)
		}
	}

	if !tui1dWaitFor(buf, 5*time.Second, func(raw string) bool { return strings.Contains(raw, tui1dESC+"[J") }) {
		t.Fatalf("前提不成立：5 秒内 stdout 管道里没有出现渲染器的 ESC[J；内容 %q", buf.String())
	}
	write("窗口尺寸报告", tui1dESC+"[8;24;100t")
	if !flatHas(5*time.Second, "counciltest空闲") {
		t.Fatalf("前提不成立：写入窗口尺寸报告后 5 秒内没有看到空闲表头；内容 %q", flat())
	}

	write("/use", "/use "+g.convID+"\r")
	if !flatHas(5*time.Second, "当前会话:") {
		t.Fatalf("前提不成立：写入 /use 后 5 秒内没有看到「当前会话:」；内容 %q", flat())
	}
	// v1.1：等第一个「当前会话:」之后出现空闲表头（submitDoneMsg 已进事件循环，候选已到）。
	idleAfterUse := tui1dWaitFor(buf, 5*time.Second, func(raw string) bool {
		f := rbaFlat(raw)
		i := strings.Index(f, "当前会话:")
		return i >= 0 && strings.Contains(f[i:], "counciltest空闲")
	})
	if !idleAfterUse {
		t.Fatalf("前提不成立：「当前会话:」之后 5 秒内没有出现空闲表头；内容 %q", flat())
	}

	if strings.Contains(flat(), ">@rba_aX") {
		t.Fatalf("前提不成立：还没敲 @ 时 stdout 里已经有 >@rba_aX；内容 %q", flat())
	}
	write("@", "@")
	if !flatHas(5*time.Second, ">@rba_aX") {
		t.Fatalf("主断言 1：敲 @ 之后 5 秒内 stdout 里没有出现候选行 >@rba_aX：真 Run 装上的 replAdapter 没有取到候选。内容 %q", flat())
	}

	write("回车", "\r")
	time.Sleep(300 * time.Millisecond)
	write("hi 回车", "hi\r")
	if !flatHas(5*time.Second, "已记下seq2") {
		t.Fatalf("前提不成立：写入 hi 回车之后 5 秒内没有看到「已记下 seq 2」；内容 %q", flat())
	}
	time.Sleep(200 * time.Millisecond)

	msgs, err := g.svc.History(context.Background(), g.convID, 0, 100)
	if err != nil {
		t.Fatalf("前提不成立：读历史失败：%v", err)
	}
	var user []string
	for _, m := range msgs {
		if m.SenderID == conversation.ReservedSenderUser {
			user = append(user, m.Text)
		}
	}
	if want := []string{"seed-msg", "@rba_a hi"}; !reflect.DeepEqual(user, want) {
		t.Errorf("主断言 2：用户消息正文应当整列是 %q（第一次回车只补全、第二次回车提交补全后的整句），实际 %q", want, user)
	}

	teardown()
}
