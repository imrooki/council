package tui

// 本文件补 Run 这一层的另一处零覆盖（未决 #39(d)）：Run 造 Model 时写的 Ctx: ctx。
//
// tui1c_wiring_test.go 文件尾记着：把这一行写成 Ctx: context.Background()，或者漏掉这个字段，
// 本包全绿。验收 8 用假 Submitter 证明「Model 会把 Options.Ctx 当成每轮 ctx 的父」，没经过 Run；
// 验收 5、6 用手工构造的 replAdapter 证明「会转发」，也没经过 Run。中间那一行赋值没人看着，
// 而它断了的后果是钱：根 ctx 被取消时在飞命令不跟着取消，spawn.go 的 killProcessTree 不跑，
// provider CLI 变成孤儿跑满硬超时继续烧 token。
//
// ─── 装置 ────────────────────────────────────────────────────────────
//
//   - 真 Run，管道两端做 in 与 stdout，理由与收尾顺序见 tui1d_run_final_test.go 文件头。
//     与那一条不同的是本文件**往 inW 写字节**：readLoop 从管道读到的字节经 ultraviolet 的
//     解码器变成按键（decoder.go：0x0D 在没有 flagCtrlM 时解成 KeyEnter；0x0A 会解成 Ctrl-J，
//     Model 拿它插换行，不提交），所以输入写成 "/conv\r"。
//   - 真 *repl.REPL：repl.New 收的是具体类型 *conversation.Service，但 conversation.NewService
//     收的是导出的 Store 接口（conversation/store.go）。假 Store 内嵌 conversation.Store（nil），
//     只覆盖 Conversations 一个方法 —— 这条路上只有它会被调到，别的方法被调到会 panic，
//     那本身就说明路径变了。
//   - 选 /conv：命令表里它只标 needsSvc，不要当前会话、不要 runner，svc 非 nil 就进 cmdConv；
//     cmdConv 调 r.svc.Conversations(ctx)，Service.Conversations 原样转给 store.Conversations(ctx)。
//     于是 ctx 的链路是 Model.beginSubmit 的 runCtx → submitCmd → replAdapter.Submit →
//     REPL.Submit → runCommand → cmdConv → Service → 假 Store，中间没有任何一层换 ctx。
//   - 假 Store 的 Conversations 进门时把 ctx 交出来，然后在 ctx.Done 与 release 之间阻塞。
//     release 只在收尾时关：坏实现下 ctx 永远不取消，靠它把那个命令 goroutine 放出来，不留 goroutine。
//   - 不起任何 provider 进程，不碰真控制台，r 的 rn 为 nil、out 为 io.Discard。

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/repl"
)

// tui1dCtxStore 是只实现了 Conversations 的假 Store，用来观测命令手里的 ctx。
type tui1dCtxStore struct {
	conversation.Store

	// entered 收到进门时的 ctx。容量 1：只预期进门一次，多出来的不阻塞调用方。
	entered chan context.Context
	// returned 收到 Conversations 返回时的错误（ctx.Done 那一支是 ctx.Err()，release 那一支是 nil）。
	returned chan error
	// release 在收尾时关，把坏实现下永远等不到 ctx.Done 的那次调用放出来。
	release chan struct{}
}

func (s *tui1dCtxStore) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	select {
	case s.entered <- ctx:
	default:
	}
	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-s.release:
	}
	select {
	case s.returned <- err:
	default:
	}
	return nil, err
}

// TestTui1dRunCtxReachesInFlightCommand：真 Run 下，取消根 ctx，在飞命令手里的 ctx 跟着取消。
//
// 【判别力，逐句】
//   - 前提一：5 秒内 stdout 管道里出现渲染器的 ESC[J。只为让输入在事件循环起来之后才写，
//     不是断言对象。管道里看不到表头文字（实测），所以不认表头。
//   - 前提二：写入 "/conv\r" 之后 5 秒内假 Store 被调到，且那时 ctx 还没取消。
//     这一条不成立说明输入路径或命令路径变了，按「前提没成立」报，不冒充 ctx 的失败。
//     三个变异都只改 ctx 的来源，不动这条路径，所以变异下它照样成立。
//   - 断言：cancel() 根 ctx 之后 5 秒内，假 Store 从 ctx.Done 那一支返回，错误是 context.Canceled。
//     断言能不能为假：能，而且是确定性的，不是概率问题 ——
//     Ctx: context.Background()（M-ctx-background）或漏掉 Ctx 字段（M-ctx-missing，New 里退化成
//     Background）时，runCtx 是 Background 的子孙，取消一个与它无关的根 ctx 对它没有任何影响；
//     这条路上另一个能取消 runCtx 的是 Model.cancel，只在 Ctrl-C 且 busy 时调用，本用例不发 Ctrl-C。
//     于是假 Store 一直阻塞到 5 秒截止，红。replAdapter.Submit 把 ctx 换成 Background
//     （M-adapter-background）同理，红 —— tui1c 文件尾第三条「归人工盯」也由这一句顺带看着。
//
// 【判别不了的】Run 交给 tea.WithContext 的 ctx 换成别的、而 Options.Ctx 仍是根 ctx：
// 在飞命令照样被取消，上面那句断言绿；末尾「cancel 之后 5 秒 Run 返回」可能顺带红，
// 但那是顺带的，本文件不为它负责，也没有实测。
func TestTui1dRunCtxReachesInFlightCommand(t *testing.T) {
	store := &tui1dCtxStore{
		entered:  make(chan context.Context, 1),
		returned: make(chan error, 1),
		release:  make(chan struct{}),
	}
	r := repl.New(nil, conversation.NewService(store), nil, nil, io.Discard, repl.RulesContext{})

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

	// 读 goroutine 必须在 Run 之前起来：管道缓冲有限，没人读时渲染器会堵住。
	buf := &tui1dBuf{}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(buf, outR)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	lw := NewLineWriter(nil)

	runDone := make(chan error, 1)
	go func() { runDone <- Run(ctx, r, lw, "test", inR, outW) }()

	// 收尾顺序：取消根 ctx；放掉假 Store（坏实现下它还在等一个永远不来的 ctx.Done）；
	// 等 Run 返回；先关 inW（让 readLoop 的 Read 读到 EOF 退出），再关 inR；
	// 关 outW 让读 goroutine 的 io.Copy 读到 EOF，等它结束，最后关 outR。
	// 每一步等待都带超时，坏实现下也不会把整包挂住。
	var releaseOnce sync.Once
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
		releaseOnce.Do(func() { close(store.release) })
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

	// 前提一：渲染器出过帧，事件循环在跑。
	// 已实测：这台装置的管道里看不到表头文字，渲染器只写出一串「抹到屏尾」ESC[J，所以认它。
	// 原因推断是管道上 GetSize 被跳过、宽度为 0，活区按 0 列裁切（未单独核实）。
	if !tui1dWaitFor(buf, 5*time.Second, func(raw string) bool {
		return strings.Contains(raw, tui1dESC+"[J")
	}) {
		t.Fatalf("前提没成立：5 秒内 stdout 管道里没有出现渲染器的 ESC[J。管道里的内容是 %q", buf.String())
	}

	// 前提二：输入经管道进到 Model，提交走到假 Store。
	if _, err := inW.Write([]byte("/conv\r")); err != nil {
		t.Fatalf("前提没成立：往输入管道写 /conv 失败：%v", err)
	}
	var got context.Context
	select {
	case got = <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("前提没成立：写入 %q 之后 5 秒假 Store 的 Conversations 没有被调到。\n"+
			"这说明管道输入没有解成按键、或 /conv 没走到 cmdConv，与 ctx 无关；本用例测不下去。\n"+
			"stdout 管道里的内容（去 ANSI）是 %q", "/conv\\r", tui1bStripANSI(buf.String()))
	}
	if err := got.Err(); err != nil {
		t.Fatalf("前提没成立：还没取消根 ctx，命令手里的 ctx 已经是 %v", err)
	}

	// 断言：取消根 ctx，在飞命令从 ctx.Done 那一支回来，错误是 context.Canceled。
	cancel()
	select {
	case err := <-store.returned:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("取消根 ctx 之后，在飞命令返回的错误是 %v，应当是 context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("取消根 ctx 之后 5 秒，在飞命令（/conv 阻塞在假 Store 里）仍没有返回：它手里的 ctx 不是根 ctx 的子孙。\n"+
			"Run 造 Model 时必须写 Ctx: ctx（交给 tea.WithContext 的同一个根 ctx）；写成 context.Background() 或漏掉这个字段，\n"+
			"根 ctx 被取消（SIGTERM、关窗）时在飞命令不跟着取消，spawn.go 的 killProcessTree 不跑，\n"+
			"provider CLI 变成孤儿跑满硬超时继续烧 token。replAdapter.Submit 把 ctx 换掉也是这个症状。\n"+
			"此刻命令手里那个 ctx 的 Err() 是 %v", got.Err())
	}

	if !waitRun() {
		t.Fatalf("cancel() 之后 5 秒 Run 没有返回")
	}
	t.Logf("Run 返回 %v", runErr)

	teardown()
}
