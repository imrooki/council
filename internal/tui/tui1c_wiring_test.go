package tui

// 本文件覆盖冻结契约「TUI 第 1 刀之三：接线与 isatty 分流」
// （E:\multi_model_cowork\tui1c_wiring_contract.md）第 4.1 节的 9 条验收。
//
// 背对背纪律：本文件的**全部断言**由测试腿在与实现腿互相看不见的情况下写成。
// 写断言期间 internal/tui/run.go 一眼没看过，internal/app/app.go、
// cmd/council/main.go、internal/tui/model.go 的新版也一眼没看过；
// 依据只有契约文本与三份已提交的代码（internal/tui/writer.go、
// internal/tui/tui1b_model_test.go、internal/repl/seam.go）。
//
// 实现落地、背对背结束之后读过 run.go 与 model.go，只改了注释，
// 没有按实现回头改任何一条断言。读后发现的两处偏差如实记在原地：
// 验收 9 的判别力比原先写的弱（见那条用例），
// 以及 Run 里「Ctx 有没有真传给 Model」这一步谁都没测到（见文件末尾）。
//
// ─── 三件写在前面的事 ───────────────────────────────────────────────
//
// 一、验收 1 为什么必须在 Run() 正文**里面**取快照。
//
// 坏实现是「先 Run 再 SetEmit」。它那次迟到的 SetEmit 照样会把 pending 里的
// 横幅原样补吐给 Send —— 只不过晚了一整个事件循环。所以只看 runWith 返回之后
// 的累计结果，好实现与坏实现一模一样，断言恒绿，正好放过它唯一要抓的那个 bug。
// 真正分得开的是**时刻**：进 Run() 的那一刻 Send 已经收到了什么。
// 假 program 因此在 Run() 入口和正文跑完两处各取一次快照。
//
// TUI-1d（E:\multi_model_cowork\tui1d_startup_contract.md）作废了原验收 1：
// 它要求进 Run() 那一刻横幅已经经 Send 交出去，对真的 tea.Program 这只能以
// 同一个 goroutine 上的死锁发生。现在的验收 1 要求那一刻 Send 为空、横幅已经
// 直写进 final；「看时刻、不看累计」这条理由不变。
//
// 二、原验收 3 按字面写不可满足，已由 lead 裁决改掉。
//
// 契约 2.2 那句「SetEmit 会补吐没送出去的行」已删，验收 3 的字面版换成
// TestTui1cAcc3bExitIsSwitchedToFinalAfterRun（钉第 3 步真正买到的东西）。
// 那个缺口本身由 TestTui1cKnownGapLostLineIsNotRecoverable 钉住，
// 裁决记在 docs/15_TUI_SPEC_MAPPING.md 第 15 节（commit d052c51）。
//
// 三、复用第 1 刀之二测试腿的假件。
//
// tui1bSubmitter / tui1bPrinter / tui1bNopDrainer / tui1bStep / tui1bRun /
// tui1bType / tui1bEnter 都在 tui1b_model_test.go 里，是已提交的代码，
// 契约 4.1 第 9 条明说可以复用。同一个包，直接用，不重新发明一份。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
)

// ─── 注入给 runWith 的假 program ────────────────────────────────────

// tui1cProgram 是注入给 runWith 的假 program（契约 3.1 的那个接口）。
//
// 契约把 program 抽成接口只有一个目的：让 runWith 的顺序卡得住
// （TUI-1d 之后：启动行在 Run 之前直写 final，运行期出口在 Run 之前装好）。
// 所以这个假件的核心不是「Send 收到了什么」，而是「在哪个时刻收到的」，
// 理由见文件开头第一段。
//
// Send 只记录、不转发给任何事件循环 —— 这正是「事件循环已死」的模型：
// 消息进来了，但没有任何人会处理它。记录纯粹是为了失败信息能说清行跑哪去了。
type tui1cProgram struct {
	mu   sync.Mutex
	sent []tea.Msg

	// body 是 Run() 的正文，由用例摆好。nil 时 Run() 直接返回。
	body func()
	// err 是 Run() 的返回值。
	err error

	runCalls int
	// atEntry 是进入 Run() 那一刻 Send 已经收到的消息。
	atEntry []tea.Msg
	// afterBody 是 body 跑完、Run() 还没返回时 Send 已经收到的消息。
	afterBody []tea.Msg
}

func (p *tui1cProgram) Send(msg tea.Msg) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, msg)
}

func (p *tui1cProgram) Run() (tea.Model, error) {
	p.mu.Lock()
	p.runCalls++
	p.atEntry = p.snapshotLocked()
	p.mu.Unlock()

	// body 里会写 LineWriter，写会经出口回到 Send，Send 要拿 p.mu。
	// 所以这里必须先放开 p.mu 再跑 body，否则自己把自己锁死。
	if p.body != nil {
		p.body()
	}

	p.mu.Lock()
	p.afterBody = p.snapshotLocked()
	p.mu.Unlock()
	return nil, p.err
}

func (p *tui1cProgram) snapshotLocked() []tea.Msg {
	return append([]tea.Msg(nil), p.sent...)
}

func (p *tui1cProgram) sentNow() []tea.Msg {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotLocked()
}

// tui1cLines 把一串 Msg 翻成可读的行。
// TUI-1d 契约 1.1 冻结了运行期出口的形状是 p.Send(printLine(line))，
// 所以非 printLine 产出的东西要在失败信息里被指出来，而不是悄悄当成空行。
//
// printLine 返回的是 bubbletea 内部不导出的打印消息，包外没法 type-switch。
// 认法：类型与 reflect.TypeOf(printLine("")) 相同，且是恰好一个字符串字段的结构体，
// 就用反射读出那个字段（读不导出字段的 .String() 是允许的）。
// 形状检查必须先做：printLine 写错成返回 Cmd（函数）时，直接取字段会 panic，
// 整个包后面的用例都不跑；这时走「非打印消息」那一支。
func tui1cLines(msgs []tea.Msg) []string {
	printType := reflect.TypeOf(printLine(""))
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			out = append(out, "非打印消息:nil")
			continue
		}
		if rv := reflect.ValueOf(m); rv.Type() == printType &&
			rv.Kind() == reflect.Struct && rv.NumField() == 1 &&
			rv.Field(0).Kind() == reflect.String {
			out = append(out, rv.Field(0).String())
			continue
		}
		out = append(out, "非打印消息:"+reflect.TypeOf(m).String())
	}
	return out
}

// tui1cWrite 往 LineWriter 里写一段，写失败直接判用例前提没成立。
func tui1cWrite(t *testing.T, lw *LineWriter, s string) {
	t.Helper()
	if _, err := lw.Write([]byte(s)); err != nil {
		t.Fatalf("往 LineWriter 里写 %q 失败: %v（writer.go 承诺这里不产生错误）", s, err)
	}
}

// ═══ 验收 1 ═══════════════════════════════════════════════════════

// 验收 1（TUI-1d 重写）：启动行在 p.Run() 之前直写 final，运行期出口在 Run 之前装好。
//
// 原验收 1 要求「进 Run() 那一刻横幅已经经 Send 交出去」—— 对真的 tea.Program，
// 那只能以死锁的方式发生：Send 在同一个 goroutine 上、事件循环起来之前调用，
// p.msgs 无缓冲，永远堵住（TUI-1d 契约 0.1）。假 program 的 Send 从不阻塞，把它藏了起来。
//
// 【判别力】三句都看**时刻**，不看累计结果：
//   - 进 Run() 那一刻 Send 一条都没收到：删掉第 1 步（只剩第 1b 步，就是旧的死锁写法）时
//     第 1b 步的 SetEmit 在 Run 之前把 pending 同步补吐给 Send，红。
//   - 进 Run() 那一刻 final 已经是「启动横幅\n」：快照在 body 的第一行拍。
//     启动行改由事件循环补吐（go out.SetEmit(...)，不死锁）时那一刻 final 是空的，红；
//     若在 runWith 返回之后才查 final，第 3 步的 SetEmit(final) 会把残留在 pending 的行
//     补吐进来，事后看一模一样，这一句就空转了。
//   - Run() 正文里写的那一行在 Run 返回前经 Send 收到，且就是 printLine("从 Run 里面写的")：
//     删掉第 1b 步（运行期的行也直写 final）时 Send 一条都收不到，红；
//     运行期出口发的是自定义消息时类型对不上，红。
func TestTui1cAcc1StartupLinesGoToFinalBeforeRun(t *testing.T) {
	lw := NewLineWriter(nil)
	tui1cWrite(t, lw, "启动横幅\n")

	var final bytes.Buffer
	var finalAtEntry string
	p := &tui1cProgram{}
	p.body = func() {
		finalAtEntry = final.String()
		tui1cWrite(t, lw, "从 Run 里面写的\n")
	}

	if err := runWith(p, lw, &final); err != nil {
		t.Fatalf("runWith 返回了非 nil 错误 %v，假 program 的 Run 返回的是 nil", err)
	}
	if p.runCalls != 1 {
		t.Fatalf("Run() 被调了 %d 次，应当恰好 1 次", p.runCalls)
	}

	if len(p.atEntry) != 0 {
		t.Errorf("进 Run() 的那一刻 Send 已经收到了 %v，应当一条都没有。\n"+
			"启动行必须在 p.Run() 之前直写 final（TUI-1d 契约 1.1 第 1 步）；在 Run 之前经 Send 交出去，\n"+
			"对真的 tea.Program 就是同一个 goroutine 上的死锁（契约 0.1）", tui1cLines(p.atEntry))
	}
	if want := "启动横幅\n"; finalAtEntry != want {
		t.Errorf("进 Run() 的那一刻 final 里是 %q，应当已经是 %q。\n"+
			"启动行必须在 p.Run() 之前写到 final，不能留到 Run 开始之后经事件循环补吐（TUI-1d 契约 1.1）。\n"+
			"runWith 返回之后 final 是 %q，Send 共收到 %v", finalAtEntry, want, final.String(), tui1cLines(p.sent))
	}

	want := printLine("从 Run 里面写的")
	if len(p.afterBody) != 1 || !reflect.DeepEqual(p.afterBody[0], want) {
		t.Errorf("Run() 正文里写完那一行、Run 还没返回时，Send 收到的是 %v，应当恰好是一条 printLine(%q)。\n"+
			"运行期出口必须在 Run 之前装好、形状是 p.Send(printLine(line))（TUI-1d 契约 1.1 第 1b 步）。\n"+
			"runWith 返回之后的累计是 %v，final 是 %q",
			tui1cLines(p.afterBody), "从 Run 里面写的", tui1cLines(p.sent), final.String())
	}
}

// ═══ 验收 2 ═══════════════════════════════════════════════════════

// 验收 2：Run 返回之后把不以换行结尾的残段吐到 final。
//
// 【判别力】三种坏实现分别被抓住：
//   - 少了第 4 步的 Flush -> final 是空的，红。
//   - Flush 排在第 3 步换出口之前 -> 残段交给还指着 Send 的旧出口，final 仍是空的，红；
//     失败信息里把 Send 收到的东西列出来，好一眼看出它跑哪去了。
//   - 把残段当成完整行吐（多带一个换行或少带）-> 逐字节比较，红。
//
// 为什么非吐不可：残段还在 LineWriter 的缓冲里、没交给任何出口，
// Run 返回之后不在这里交出去，进程一退就永久丢失（契约 2.2 与 writer.go 的 Flush 注释）。
// （这里原来还有一句「内联模式退出时活区会被抹掉、最后一帧不留在屏幕上」，是错的：
// bubbletea 只擦帧末行，残影由 X-39e 的 runWith 第 2b 步擦掉。）
func TestTui1cAcc2ResidualGoesToFinalAfterRun(t *testing.T) {
	lw := NewLineWriter(nil)
	var final bytes.Buffer
	p := &tui1cProgram{}
	p.body = func() { tui1cWrite(t, lw, "残段不以换行结尾") }

	if err := runWith(p, lw, &final); err != nil {
		t.Fatalf("runWith 返回了非 nil 错误 %v，假 program 的 Run 返回的是 nil", err)
	}

	if got, want := final.String(), "残段不以换行结尾\n"; got != want {
		t.Errorf("runWith 返回之后 final 里是 %q，应当是 %q。\n"+
			"契约 3.1 第 3 步换出口、第 4 步 Flush，两步都必须有。\n"+
			"Send 收到的是 %v —— 残段跑到那里去了就说明 Flush 排在换出口之前",
			got, want, tui1cLines(p.sent))
	}
}

// ═══ 已知缺口的回归钉（原验收 3 的字面版，已由 lead 裁决改写）═══════

// 【这条不是在验证一个好行为，是在钉住一个已知缺口。】
//
// 缺口：事件循环已经停掉、而 runWith 还没走到第 3 步的那一瞬，写进 LineWriter
// 的完整行会永久丢失 —— 不在屏幕上，不在 final 里，LineWriter 手里也没有。
//
// 根因两条，缺一不可：
//  1. writer.go 的 ingest 只在 fn == nil 时才把完整行放进 pending。runWith 第 1b 步
//     装的是非 nil 的 p.Send 包装，此后 pending 再也填不回去；
//  2. (*tea.Program).Send 不返回任何投递回执，出口无从知道这一次落空了，
//     也就没法把行退回 LineWriter。
//
// 冻结契约原先的验收 3 要求「Run 返回后 final 里有这一行」，2026-09-23 实测证伪；
// 契约 2.2 那句「SetEmit 会补吐没送出去的行」已删。裁决与残余窗口记在
// docs/15_TUI_SPEC_MAPPING.md 第 15 节（commit d052c51）。
//
// 【将来谁真把它修好了，这条会红。】那时该做的是更新 docs/15 第 15 节、
// 把这条改成验证新行为，不是把它删掉了事。
//
// 【两种红必须分得开，这是本条能留下来的前提】
// 有一种坏实现会让 final 里真的出现这一行，长得跟「缺口被修好了」一模一样：
// 只删第 1 步、留着第 3 步。那时 Run 期间出口是 nil，行进 pending，
// 第 3 步的 SetEmit 把它补吐进 final。
// 顺带纠正一个容易推错的地方：「第 1、3 步都删」不会走到 final ——
// Flush 只吐 buf 里的残段，以换行结尾的行早被 splitLocked 切进 pending 了，
// 而 pending 的唯一出口是 SetEmit 装上非 nil 的出口。
// 那种实现下 final 是空的、Send 一条都没收到。
//
// 所以「Send 确实收到了这一行」写成排在钉子前面的 t.Fatalf 前提，不是普通断言：
//   - 出口没在 p.Run() 之前装上 -> 前提先炸，文案直接把人指向验收 1，钉子不被求值；
//   - 缺口真被修好 -> 前提照样成立，钉子那句红，文案指向 docs/15。
//
// 两种红走不同语句、不同文案。而且前一种红必定伴随 TestTui1cAcc1 一起红，
// 本条不会是唯一的信号源。顺序反了这条就真成噪声，那时应当只留文档、删掉本条。
func TestTui1cKnownGapLostLineIsNotRecoverable(t *testing.T) {
	lw := NewLineWriter(nil)
	var final bytes.Buffer
	// Send 只记录、不转给任何事件循环 —— 这就是「事件循环已死」的模型。
	p := &tui1cProgram{}
	p.body = func() { tui1cWrite(t, lw, "丢不得\n") }

	if err := runWith(p, lw, &final); err != nil {
		t.Fatalf("runWith 返回了非 nil 错误 %v，假 program 的 Run 返回的是 nil", err)
	}

	// 前提，必须排在钉子前面，理由见上方「两种红必须分得开」。
	if got, want := tui1cLines(p.sentNow()), []string{"丢不得"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("前提没成立：Send 收到的是 %v，应当是 %v。\n"+
			"这不是本条要钉的缺口，是「出口没在 p.Run() 之前装上」；\n"+
			"根因看 TestTui1cAcc1StartupLinesGoToFinalBeforeRun，本条的结果这时不作数",
			got, want)
	}

	// 钉子之一：行没有落到 final 里。
	if strings.Contains(final.String(), "丢不得") {
		t.Errorf("缺口似乎被修好了：final 里出现了 %q，全文是 %q。\n"+
			"本条钉的是「交给一个落空的出口的行拿不回来」这个已知缺口。\n"+
			"如果这是一次有意的修复，请更新 docs/15_TUI_SPEC_MAPPING.md 第 15 节，\n"+
			"并把本条改成验证新行为，不要直接删掉它",
			"丢不得", final.String())
	}

	// 钉子之二：LineWriter 手里也不再有它。
	// 两条取回路径各试一遍：换新出口会补吐 pending，Flush 会吐 buf。
	// 将来谁把落空的行退回这两处之一，这里红。
	var recovered []string
	lw.SetEmit(func(line string) { recovered = append(recovered, line) })
	lw.Flush()
	if len(recovered) != 0 {
		t.Errorf("缺口似乎被修好了：runWith 结束之后换出口再 Flush，又拿回了 %q。\n"+
			"同上，请更新 docs/15 第 15 节并改写本条，不要直接删掉", recovered)
	}
}

// 验收 3 的替代品：Run 返回之后，出口必须已经换成 final。
//
// 这条钉的是第 3 步真正买到的东西（契约 2.2 的后半句）：Run 返回之后 REPL 若还有
// 输出，要直接落到屏幕上，而不是投进一个已经死掉的事件循环。
//
// 【判别力】两句都能为假：
//   - 少了第 3 步 -> 出口还指着 Send，runWith 返回之后写的行进了死掉的事件循环，
//     final 里什么都没有，红；
//   - Send 的累计条数在 runWith 返回之后又涨了 -> 同一件事的正面证据，红。
//
// 假 program 的 Send 不转发给任何人，正是契约 4.1 第 3 条设定的「事件循环已死」。
func TestTui1cAcc3bExitIsSwitchedToFinalAfterRun(t *testing.T) {
	lw := NewLineWriter(nil)
	var final bytes.Buffer
	p := &tui1cProgram{}
	p.body = func() { tui1cWrite(t, lw, "Run 期间写的\n") }

	if err := runWith(p, lw, &final); err != nil {
		t.Fatalf("runWith 返回了非 nil 错误 %v，假 program 的 Run 返回的是 nil", err)
	}

	sentBefore := len(p.sentNow())
	tui1cWrite(t, lw, "Run 之后的输出\n")

	if got := final.String(); !strings.Contains(got, "Run 之后的输出") {
		t.Errorf("runWith 返回之后再往 LineWriter 里写一整行，final 里是 %q，应当含 %q。\n"+
			"契约 3.1 第 3 步要把出口换成直写 final；漏了这一步，Run 返回之后 REPL 的输出\n"+
			"会继续投进一个已经死掉的事件循环（契约 2.2）。Send 收到的是 %v",
			got, "Run 之后的输出", tui1cLines(p.sentNow()))
	}
	if got := len(p.sentNow()); got != sentBefore {
		t.Errorf("runWith 返回之后写的那一行又进了 p.Send：Send 收到的条数从 %d 变成 %d（%v）。\n"+
			"出口必须已经换成直写 final，不能还指着已经停掉的 Program",
			sentBefore, got, tui1cLines(p.sentNow()))
	}
}

// ═══ 验收 4 ═══════════════════════════════════════════════════════

// 验收 4：runWith 原样返回 Run 的错误（包括 nil），而且出错路径上照样把缓冲吐干净。
//
// 【判别力】
//   - nil 那一支：抓「顺手把 Flush 或别的地方的错误冒出来」。
//   - 非 nil 那一支比的是**同一个值**，不是 errors.Is：契约 3.1 第 5 步冻结的是
//     裸的 return err，包一层（fmt.Errorf %w）也算违约。
//   - 残段那一句是本条真正的牙：抓 `if err != nil { return err }` 提前返回、
//     把第 3、4 步跳过去。这条路正是契约 2.2 点名的那条 —— Run 出错或 ctx 被取消时
//     Model.quitCmd 一次都没跑过，缓冲只能靠 runWith 自己吐。
//     漏了它，SIGTERM / 关窗退出时最后那段输出就永久没了。
func TestTui1cAcc4ReturnsRunErrorVerbatim(t *testing.T) {
	t.Run("Run 返回 nil 时 runWith 也返回 nil", func(t *testing.T) {
		lw := NewLineWriter(nil)
		var final bytes.Buffer
		p := &tui1cProgram{}

		if err := runWith(p, lw, &final); err != nil {
			t.Errorf("Run 返回 nil 时 runWith 返回了 %v，应当是 nil", err)
		}
	})

	t.Run("Run 返回错误时原样返回，且残段照样吐到 final", func(t *testing.T) {
		sentinel := errors.New("Run 自己炸了")
		lw := NewLineWriter(nil)
		var final bytes.Buffer
		p := &tui1cProgram{err: sentinel}
		p.body = func() { tui1cWrite(t, lw, "出错路径上的残段") }

		got := runWith(p, lw, &final)
		if got != sentinel {
			if errors.Is(got, sentinel) {
				t.Errorf("runWith 把 Run 的错误包了一层：返回的是 %v，契约 3.1 第 5 步冻结的是裸的 return err", got)
			} else {
				t.Errorf("runWith 返回的是 %v，应当原样是 Run 返回的那个错误 %v", got, sentinel)
			}
		}
		if want := "出错路径上的残段\n"; final.String() != want {
			t.Errorf("Run 出错时 final 里是 %q，应当是 %q。\n"+
				"第 3、4 步必须排在 return err 之前：Run 出错与 ctx 被取消正是 Model.quitCmd\n"+
				"一次都没跑过的那两条路径（契约 2.2），缓冲只剩 runWith 能吐。\n"+
				"Send 收到的是 %v", final.String(), want, tui1cLines(p.sent))
		}
	})
}

// ═══ 验收 5 ═══════════════════════════════════════════════════════

// tui1cREPL 造一个只走 Submit 的真 REPL。
// svc / rn 传 nil 是合法状态（repl.New 的文档写明），本用例用的三条输入
// 都不带 needsSvc / needsConv / needsRunner，走得到底。
func tui1cREPL(t *testing.T) (*repl.REPL, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	// in 传 nil：Submit 不读输入流，只有 REPL.Run 的读循环才读。
	return repl.New(nil, nil, nil, nil, &buf, repl.RulesContext{}), &buf
}

// 验收 5：replAdapter.Submit 转发给 repl.REPL.Submit。
//
// 【判别力】四条输入各卡一件事：
//   - "/help"：最普通的一条，抓「整个转发没接上」。
//   - "/say"（空参）：**最锋利的一条**。这句答复（seam.go 的 msgDraftNotInTUI）
//     只有 Submit 这条路才产生，dispatch 会进多行草稿模式。适配器要是图省事
//     转给了 dispatch，只有这条能看出来。
//   - "/一个不存在的命令"：抓「转发时把文本换成别的」——上面两条各自的答复不同，
//     换了文本三条里至少有一条对不上。
//   - "/quit"：**这一条是变异验证补进来的**。前三条的返回值都是 nil，
//     于是「适配器把错误吞掉、恒返回 nil」这种实现在 nil == nil 上照样绿 ——
//     rig 上的 MUT-G 当场抓到本用例原本是这个形状。/quit 返回哨兵错误，
//     吞错误的实现在这一条上红。
//
// 同一个 *REPL 上先后跑两条路，中间 Reset 缓冲：四条输入都不改 REPL 状态
// （不落库、不建会话、不置 inDraft，/quit 只是返回哨兵），两次的输出本该逐字节相同。
//
// 【测不到的一件事，点名】适配器有没有把**同一个 ctx** 转下去，这里看不见：
// 契约把 replAdapter 冻结成直接包 *repl.REPL，没有注入点，而这四条命令都不读 ctx。
// 适配器要是偷偷换成 context.Background()，本条仍然绿。归人工看那一行。
func TestTui1cAcc5AdapterSubmitForwardsToREPL(t *testing.T) {
	r, buf := tui1cREPL(t)
	a := replAdapter{r}
	ctx := context.Background()

	for _, tc := range []struct {
		text string
		// wantErr 为真表示这条输入本该返回非 nil 错误，那时输出可以是空的。
		wantErr bool
	}{
		{text: "/help"},
		{text: "/say"},
		{text: "/一个不存在的命令"},
		{text: "/quit", wantErr: true},
	} {
		t.Run(tc.text, func(t *testing.T) {
			buf.Reset()
			errDirect := r.Submit(ctx, tc.text)
			direct := buf.String()

			buf.Reset()
			errVia := a.Submit(ctx, tc.text)
			via := buf.String()

			// 前提：这条输入确实产生了可观测的东西，否则下面两句都是空转。
			if tc.wantErr {
				if errDirect == nil {
					t.Fatalf("前提没成立：r.Submit(ctx, %q) 返回了 nil，本条要靠非 nil 的返回值才有牙", tc.text)
				}
			} else if direct == "" {
				t.Fatalf("前提没成立：r.Submit(ctx, %q) 什么都没输出，输出比较会恒真", tc.text)
			}

			if via != direct {
				t.Errorf("replAdapter.Submit(ctx, %q) 的输出是 %q，\n"+
					"r.Submit(ctx, %q) 的输出是 %q —— 适配器必须原样转发（契约 3.1）",
					tc.text, via, tc.text, direct)
			}
			if errVia != errDirect {
				t.Errorf("replAdapter.Submit(ctx, %q) 返回 %v，r.Submit 返回 %v，应当是同一个值 —— "+
					"适配器不许吞掉错误，TUI 靠这个返回值判要不要退", tc.text, errVia, errDirect)
			}
		})
	}
}

// ═══ 验收 6 ═══════════════════════════════════════════════════════

// 验收 6：replAdapter.IsQuit 转发给 repl.IsQuit。
//
// 【判别力】四格互相咬死，任何一种「返回常量」的实现都至少踩中一格：
//   - /quit 的返回值 -> true（恒返回 false 的实现红）；
//   - 别的错误 -> false（恒返回 true 的实现红）；
//   - nil -> false（同上，顺带钉住不 panic）；
//   - 包了一层的 /quit 返回值 -> true。repl.IsQuit 用的是 errors.Is，
//     适配器要是自己写了 err == 什么，这一格红。
//
// 哨兵本身不导出（seam.go 写明了理由），所以只能从 r.Submit(ctx, "/quit")
// 手里拿一个真的。取到之后先用 repl.IsQuit 验一遍前提，免得前提本身就不成立。
func TestTui1cAcc6AdapterIsQuitForwards(t *testing.T) {
	r, _ := tui1cREPL(t)
	a := replAdapter{r}

	quitErr := r.Submit(context.Background(), "/quit")
	if quitErr == nil {
		t.Fatalf("前提没成立：/quit 应当返回哨兵错误，实际返回 nil")
	}
	if !repl.IsQuit(quitErr) {
		t.Fatalf("前提没成立：repl.IsQuit 对 /quit 的返回值 %v 是 false", quitErr)
	}

	if !a.IsQuit(quitErr) {
		t.Errorf("replAdapter.IsQuit 对 /quit 的返回值 %v 是 false，应当是 true —— "+
			"TUI 靠它判「要不要退」，判错就退不出去", quitErr)
	}
	if wrapped := fmt.Errorf("外面包一层: %w", quitErr); !a.IsQuit(wrapped) {
		t.Errorf("replAdapter.IsQuit 对包了一层的 /quit 返回值是 false，应当是 true —— " +
			"repl.IsQuit 用的是 errors.Is，适配器不该退化成 ==")
	}
	if a.IsQuit(errors.New("一个普通错误")) {
		t.Errorf("replAdapter.IsQuit 对普通错误返回了 true，应当是 false —— 会把一次失败当成退出")
	}
	if a.IsQuit(nil) {
		t.Errorf("replAdapter.IsQuit(nil) 返回了 true，应当是 false")
	}
}

// ═══ 验收 8、9：Options.Ctx ═════════════════════════════════════════

// tui1cModel 造一个装好假件的 Model，Ctx 由调用方给（传 nil 就是不设这个字段）。
// 假 Submitter / Printer / Drainer 复用 tui1b_model_test.go 的那三个。
func tui1cModel(t *testing.T, ctx context.Context) (Model, *tui1bSubmitter) {
	t.Helper()
	sub := &tui1bSubmitter{}
	pr := &tui1bPrinter{}
	m := New(Options{
		Submit:  sub,
		Out:     tui1bNopDrainer{},
		Version: "0.0.0-test",
		Println: pr.cmd,
		Ctx:     ctx,
	})
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, sub
}

// tui1cSubmitOnce 打一段字、按 Enter、把返回的 Cmd 跑到底，让假 Submitter 收到一次调用。
func tui1cSubmitOnce(t *testing.T, m Model, text string) Model {
	t.Helper()
	m = tui1bType(t, m, text)
	m, cmd := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd)
	return m
}

// 验收 8：Options.Ctx 真的成了每轮命令 ctx 的父。
//
// 【判别力，这条关系到真钱】坏实现是 Model.ctx 仍然恒为 context.Background()。
// 那时假 Submitter 手里的 ctx 是 context.Background() 的子孙，
// 取消我这个外层 ctx 对它**没有任何影响** —— 纯粹的 context 语义，不是概率问题，
// 所以这条在坏实现下一定红，不会碰巧绿。
//
// 三处防空转：
//  1. 先确认 Submit 真的被调了一次，否则 sub.ctx(0) 是 nil，后面全是空转；
//  2. 取消之前断言 ctx.Err() 是 nil。要是它本来就已经被取消（比如实现在 Cmd 跑完
//     就 defer cancel 了），「取消之后是 Canceled」这句恒真，等于没测；
//  3. 等 Done 而不是直接读 Err：父是 *cancelCtx 时传播是同步的，但不把这一点
//     押在实现细节上。等不到就超时，超时之后 Err() 仍是 nil，照样红。
//
// 生产后果见契约 3.0：根 ctx 被取消时在飞命令不跟着取消，spawn.go 的
// killProcessTree 不跑，Windows 上 provider CLI 变成孤儿跑满硬超时继续烧 token。
func TestTui1cAcc8OptionsCtxIsParentOfEachRoundCtx(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()

	m, sub := tui1cModel(t, root)
	tui1cSubmitOnce(t, m, "hello")

	if sub.count() != 1 {
		t.Fatalf("前提没成立：Submit 的调用次数是 %d，应当是 1", sub.count())
	}
	got := sub.ctx(0)
	if got == nil {
		t.Fatalf("Submit 收到的 ctx 是 nil")
	}
	if err := got.Err(); err != nil {
		t.Fatalf("取消 Options.Ctx 之前，Submit 手里那个 ctx 就已经是 %v，"+
			"「取消之后变成 Canceled」这句会恒真，前提没控制住", err)
	}

	cancel()

	select {
	case <-got.Done():
	case <-time.After(2 * time.Second):
	}
	if err := got.Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("取消 Options.Ctx 之后，Submit 手里那个 ctx 的 Err() 是 %v，应当是 context.Canceled。\n"+
			"Model.ctx 没接上 Options.Ctx（还是 context.Background()）时这条必红：\n"+
			"取消一个与它无关的 ctx，对 context.Background() 的子孙没有任何影响。\n"+
			"后果是钱：根 ctx 被取消时在飞命令不跟着取消，spawn.go 的 killProcessTree 不跑，\n"+
			"provider CLI 变成孤儿跑满硬超时继续烧 token（契约 3.0）", err)
	}
}

// 验收 9：Options.Ctx 不传时退化成 context.Background()，不 panic。
//
// 【判别力弱，点名；背对背结束后又降了一级】
//
// 原先写的是：这条只抓「New 里写了 ctx: opts.Ctx 而漏了 nil 兜底」，
// 那时 context.WithCancel(nil) 会 panic（"cannot create context from nil parent"）。
//
// 读过落地的 model.go 之后要修正：兜底有**两道** —— New 里一道，
// beginSubmit 里还有一道（baseCtx := m.ctx; if baseCtx == nil { ... }）。
// 所以本条只有在两道都被删掉时才红，比原先说的还弱。
// 不改断言，只把话说准：它现在几乎是结构上恒绿的，留着是为了两道兜底同时消失时还有人喊一声。
//
// 判「Options.Ctx 有没有真接上」全靠验收 8，本条一点忙都帮不上：
// 根本没接 Options.Ctx 的实现恒为 Background，本条照样绿。
//
// 用 recover 接住 panic，是为了它不把同一个包里其它用例一起带走；
// tui1bRun 在当前 goroutine 上同步跑 Cmd，所以这里 recover 得到。
func TestTui1cAcc9NilOptionsCtxDegradesToBackground(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Options.Ctx 不传时 panic: %v —— 契约 3.0 第 2 点要求 New 里为 nil 时填 context.Background()", r)
		}
	}()

	m, sub := tui1cModel(t, nil)
	tui1cSubmitOnce(t, m, "hello")

	if sub.count() != 1 {
		t.Fatalf("前提没成立：Submit 的调用次数是 %d，应当是 1", sub.count())
	}
	got := sub.ctx(0)
	if got == nil {
		t.Fatalf("Options.Ctx 不传时 Submit 收到的 ctx 是 nil，应当是 context.Background() 的子孙")
	}
	if err := got.Err(); err != nil {
		t.Errorf("Options.Ctx 不传时 Submit 手里那个 ctx 的 Err() 是 %v，应当是 nil", err)
	}
}

// ═══ 谁都没测到的那几步，写在这里，别让它们无声无息 ═══════════════

// 本文件到此覆盖了契约 4.1 的验收 1、2、3b、4、5、6、8、9。
// 下面这几件事本包一条用例都没有，契约 4.2 已经承认了一部分，
// 但有一条是背对背结束、读过 run.go 之后才看清的，单独点出来。
//
// 一、**Run 里那两行赋值，判别力为零 —— 这是本刀剩下的最大风险。**（写这段时如此；现已补上，见本条末尾。）
//
//	m := New(Options{ Submit: replAdapter{r: r}, Out: out, Version: version, Ctx: ctx })
//
// 验收 8 证明的是「Model 会把 Options.Ctx 当成每轮 ctx 的父」，
// 用的是假 Submitter，根本没经过 Run；验收 5、6 证明的是「replAdapter 会转发」，
// 用的是手工构造的 replAdapter，也没经过 Run。
// 两头都钉住了，**中间这一行把它们接起来的赋值没有任何用例看着**：
// 把 Ctx: ctx 写成 Ctx: context.Background()，或者干脆漏掉这个字段，
// 本包全绿。而那正是契约 3.0 说的「后果是钱」那条路。
// 契约 4.2 当时判「Run 要真句柄，单测进不去」，所以这一行归人工盯。
// **这个判断后来被推翻，这一行已有用例（2026-09-23，未决 #39(d)）**：Run 收的是 *os.File，
// tui1d_run_ctx_test.go 用 os.Pipe 两端驱动真 Run、真 *repl.REPL（假 Store 只实现 Conversations），
// 经管道提交 /conv 让它阻塞在假 Store 里，取消根 ctx 后断言它从 ctx.Done 返回。
// Ctx 写成 context.Background()、context.WithoutCancel(ctx) 或漏掉这个字段，那条用例都红。
// 同一招也钉住了 Run 把 stdout 当 final 交给 runWith（tui1d_run_final_test.go）。
//
// 二、契约 4.2 已经列出的三样：Run 本身、main.go 的 isatty 分流与
// 「TUI 下不注册硬退 goroutine」、真终端里的观感与 Ctrl-C 行为。
// （原来还有一句「不调 EnablePaste」：X-39b 之后两种模式都不调，这个函数已删。）
//
// 三、replAdapter.Submit 有没有把**同一个 ctx** 转下去，验收 5 看不见：
// 契约把 replAdapter 冻结成直接包 *repl.REPL，没有注入点，
// 而那四条命令（/help、/say、未知命令、/quit）都不读 ctx。
// 换成 context.Background() 本包照样全绿。本文件里仍然看不见；
// 但 tui1d_run_ctx_test.go 经真 Run、真 REPL 走的正是这一步，把它换成 Background 那条用例红（已实测）。
