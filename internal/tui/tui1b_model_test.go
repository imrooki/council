package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// 本文件覆盖冻结契约第 6 节的 14 条验收（原验收 2 测的是 outputLineMsg 那条路，已由 TUI-1d 连同那条路一起删掉）。第 6.1 节那三条在
// internal/repl/tui1b_isquit_test.go。
//
// 全部用合成 Msg 驱动，不需要 TTY。
//
// ─── 两件写在前面的事 ───────────────────────────────────────────────
//
// 一、怎么把一个 tea.Cmd「跑到底」。
//
// tea.Quit 是个函数值，Go 里函数不可比较，直接拿返回的 Cmd 去比 tea.Quit
// 连编译都过不了。所以判「这条 Cmd 会不会让程序退出」只能把它跑起来看产出什么 Msg。
// 而 tea.Sequence / tea.Batch 跑出来的不是叶子 Msg，是一个装着子 Cmd 的容器消息，
// 真正的事件循环会再把它展开。tui1bRun 就是照事件循环的做法把它展开到底。
// 依据见 tui1bRun 上方的注释。
//
// 二、Model 的字段名没被冻结。
//
// 契约 5.1 写的是 `type Model struct{ /* 你定 */ }`，只有 New / Init / Update /
// View 四个方法和 Options 的四个字段是冻结面。验收 1、5、7 却是对着内部状态说的
// （「Model 内部宽度」「输入框被清空」「输入框的值里多了一个 \n」）。
// 这里一律不按字段名读：
//   - 宽高走反射按「值」配对（tui1bHasIntPair），外加一条从 View 文本上看的旁证；
//   - 忙闲与输入框内容走行为证明 —— 下一次按 Enter 时，注入的 Submitter
//     有没有被调、收到的是什么文本。
// 代价是这几条比直接读字段弱一点，逐条标在用例上方。

// ─── 注入给 Model 的假件 ────────────────────────────────────────────

// tui1bSubmitter 是注入的假 Submitter，记下每一次调用的文本与 ctx。
// Submit 会被 bubbletea 在别的 goroutine 上跑，所以加锁。
type tui1bSubmitter struct {
	mu    sync.Mutex
	calls int
	texts []string
	ctxs  []context.Context

	// err 是 Submit 的返回值；quit 是 IsQuit 的返回值。两者都由用例摆好。
	err  error
	quit bool
}

func (f *tui1bSubmitter) Submit(ctx context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.texts = append(f.texts, text)
	f.ctxs = append(f.ctxs, ctx)
	return f.err
}

func (f *tui1bSubmitter) IsQuit(error) bool { return f.quit }

func (f *tui1bSubmitter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *tui1bSubmitter) text(i int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < 0 || i >= len(f.texts) {
		return ""
	}
	return f.texts[i]
}

func (f *tui1bSubmitter) ctx(i int) context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < 0 || i >= len(f.ctxs) {
		return nil
	}
	return f.ctxs[i]
}

// tui1bPrintedMsg 是假 Println 造出来的那条 Cmd 跑起来之后产出的 Msg。
//
// 为什么不在 Println 被调用的当下就记顺序：契约 5.3 的 quitCmd 是在 Update 里
// 同步把 m.println(line) 一条条调完，再把 tea.Quit 缀在后面。调用时刻的顺序
// 无论实现用 Sequence 还是 Batch 都一样，拿它做验收 10 的顺序断言是恒真的。
// 真正分得开的是「这些 Cmd 跑起来之后，产出的 Msg 谁先谁后」。
type tui1bPrintedMsg struct{ line string }

// tui1bPrinter 是注入给 Options.Println 的记录器。
// built 记的是构造时刻（Update 里调了几次 println、参数是什么），
// 执行时刻的顺序由 tui1bRun 收集。
type tui1bPrinter struct {
	mu    sync.Mutex
	built []string
}

func (p *tui1bPrinter) cmd(line string) tea.Cmd {
	p.mu.Lock()
	p.built = append(p.built, line)
	p.mu.Unlock()
	return func() tea.Msg { return tui1bPrintedMsg{line: line} }
}

func (p *tui1bPrinter) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.built...)
}

// tui1bNopDrainer 是不产出任何东西的 Drainer。
// 只有验收 10 需要真的 *LineWriter，其余用例用它，好让 quitCmd 的产出干净。
type tui1bNopDrainer struct{}

func (tui1bNopDrainer) SetEmit(EmitFunc) {}
func (tui1bNopDrainer) Flush()           {}

// ─── 把一条 tea.Cmd 跑到底 ──────────────────────────────────────────

// tui1bCmdType 是 tea.Cmd 的反射类型。
//
// tea.Sequence 产出的容器消息类型 sequenceMsg 在 bubbletea 里是**未导出**的
// （commands.go:30 `type sequenceMsg []Cmd`），包外没法 type-switch。
// tea.BatchMsg 是导出的（commands.go:21）。两者底层都是 []tea.Cmd，
// 所以认法是：先试导出的 BatchMsg，剩下的按「元素类型是 tea.Cmd 的切片」认。
var tui1bCmdType = reflect.TypeOf(tea.Cmd(nil))

// tui1bTrace 是一条 Cmd 展开到底之后的结果。
type tui1bTrace struct {
	// msgs 是真正会落到 Update 的叶子 Msg，按产出顺序。
	msgs []tea.Msg
	// log 是同一串 Msg 的可读形式，顺序断言用它，失败信息才看得懂。
	log []string
	// sawBatch 记展开途中有没有撞见 tea.BatchMsg。
	sawBatch bool
}

func (tr tui1bTrace) hasQuit() bool {
	for _, m := range tr.msgs {
		if _, ok := m.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

// tui1bRun 把一条 tea.Cmd 按 bubbletea 事件循环的语义展开到底，
// 按顺序收集最终会落到 Update 的叶子 Msg。
//
// 照抄的是 bubbletea v2.0.9 的三段源码：
//
//  1. commands.go:36-54 compactCmds —— tea.Sequence 与 tea.Batch 都走它。
//     nil 的子 Cmd 被丢掉；剩下 0 条返回 nil；剩下 1 条**直接把那条 Cmd 原样返回**，
//     不套容器；2 条以上才返回一个产出 sequenceMsg / BatchMsg 的 Cmd。
//     所以「缓冲是空的 + Ctrl-C」这种情况，quitCmd 返回的就是 tea.Quit 本身，
//     跑一下直接拿到 QuitMsg，外面没有任何一层容器。这里必须能处理裸叶子。
//
//  2. tea.go:900-924 execSequenceMsg —— 按切片顺序**逐条**跑，nil 跳过，
//     子结果若还是 Batch / sequence 就递归展开，否则当叶子发给 Update。
//     这里的 sequence 分支就是照这个写的。
//
//  3. tea.go:926-966 execBatchMsg —— 每条子 Cmd 各起一个 goroutine 并发跑，
//     跑完 wg.Wait。**顺序没有任何保证。**
//
// 第 3 条决定了这里的一个刻意选择：撞见 BatchMsg 时按**倒序**跑它的子 Cmd。
// 倒序是 execBatchMsg 允许的一种合法排期。要是这里也顺序跑，
// tea.Batch(printCmd, tea.Quit) 在本测试里产出的顺序会和 tea.Sequence 一模一样，
// 验收 10 那条顺序断言就永远是绿的 —— 正好放过它唯一要抓的那个 bug。
// 另外把 sawBatch 记下来，让失败信息能直接说「你用了 Batch」。
func tui1bRun(t *testing.T, cmd tea.Cmd) tui1bTrace {
	t.Helper()

	var tr tui1bTrace
	var walk func(c tea.Cmd, depth int)
	walk = func(c tea.Cmd, depth int) {
		if c == nil {
			return
		}
		// 自引用的 Cmd 会让这里无限递归。撞上限就当成失败，不要把测试挂死。
		if depth > 16 {
			t.Fatalf("Cmd 嵌套超过 16 层，疑似自引用；已收到 %v", tr.log)
		}
		if len(tr.msgs) > 256 {
			t.Fatalf("Cmd 产出超过 256 条 Msg，疑似死循环；已收到 %v", tr.log)
		}

		msg := c()
		if msg == nil {
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			tr.sawBatch = true
			for i := len(batch) - 1; i >= 0; i-- {
				walk(batch[i], depth+1)
			}
			return
		}
		if seq, ok := tui1bCmdSlice(msg); ok {
			for _, sub := range seq {
				walk(sub, depth+1)
			}
			return
		}
		tr.msgs = append(tr.msgs, msg)
		tr.log = append(tr.log, tui1bLabel(msg))
	}
	walk(cmd, 0)
	return tr
}

// tui1bCmdSlice 认出 tea.Sequence 造出来的那个未导出的容器消息：
// 形状是「元素类型恰好是 tea.Cmd 的切片」。调用方必须先把 tea.BatchMsg 挑走，
// 否则 BatchMsg 也会落进这里（它的底层类型同样是 []tea.Cmd）。
func tui1bCmdSlice(msg tea.Msg) ([]tea.Cmd, bool) {
	rv := reflect.ValueOf(msg)
	if !rv.IsValid() || rv.Kind() != reflect.Slice || rv.Type().Elem() != tui1bCmdType {
		return nil, false
	}
	out := make([]tea.Cmd, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		c, _ := rv.Index(i).Interface().(tea.Cmd)
		out = append(out, c)
	}
	return out, true
}

func tui1bLabel(msg tea.Msg) string {
	switch m := msg.(type) {
	case tui1bPrintedMsg:
		return "print:" + m.line
	case tea.QuitMsg:
		return "quit"
	case submitDoneMsg:
		return "submitDone"
	}
	return "other:" + reflect.TypeOf(msg).String()
}

// ─── 驱动 Model 的小工具 ────────────────────────────────────────────

// tui1bStep 走一步 Update，并把返回的 tea.Model 断回具体的 Model。
// 契约 5.1 要求 Model 是值类型、方法用值接收者；返回指针是违约，这里要读得出来。
func tui1bStep(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update 返回的不是 tui.Model 值，而是 %T；契约 5.1 要求值类型、值接收者", next)
	}
	return got, cmd
}

func tui1bKey(r rune) tea.KeyPressMsg {
	// textarea 的 default 分支只在 msg.Text 非空时才插字符
	// （bubbles v2.2.1 textarea.go 的 `if msg.Text == "" { break }`），
	// 所以造可打印字符必须同时给 Code 和 Text。
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func tui1bCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Mod: tea.ModCtrl, Code: r}
}

func tui1bEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func tui1bEsc() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyEscape} }

// tui1bType 把一串可打印字符敲进输入框。
//
// 故意丢掉 textarea 返回的 Cmd 不跑：textarea.Update 末尾是 tea.Batch(cmds...)，
// 光标位置变了会捎上一条 virtualCursor.Blink()，那是个等 BlinkSpeed（默认约 530ms）
// 的 Cmd。跑它只会让测试变慢，且与本刀的验收无关。
func tui1bType(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = tui1bStep(t, m, tui1bKey(r))
	}
	return m
}

// tui1bNew 造一个装好假件的 Model。out 为 nil 时用不产出任何东西的 Drainer。
func tui1bNew(t *testing.T, out Drainer) (Model, *tui1bSubmitter, *tui1bPrinter) {
	t.Helper()
	sub := &tui1bSubmitter{}
	pr := &tui1bPrinter{}
	if out == nil {
		out = tui1bNopDrainer{}
	}
	m := New(Options{
		Submit:  sub,
		Out:     out,
		Version: "0.0.0-test",
		Println: pr.cmd,
	})
	return m, sub, pr
}

// tui1bSized 造一个已经收过窗口尺寸的 Model。
func tui1bSized(t *testing.T, out Drainer) (Model, *tui1bSubmitter, *tui1bPrinter) {
	t.Helper()
	m, sub, pr := tui1bNew(t, out)
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, sub, pr
}

// ─── 不按字段名看内部状态的两个手段 ──────────────────────────────────

// tui1bIntFields 读出 Model 顶层所有整数类字段的值，按字段下标索引。
//
// 只读不写，所以未导出字段也没问题：reflect.Value.Int() 不查 flagRO，
// 查它的是 Interface() 与各种 Set。这里一层都不往下钻，
// 免得把 textarea.Model 里的一堆整数也算进来。
func tui1bIntFields(m Model) map[int]int64 {
	out := map[int]int64{}
	rv := reflect.ValueOf(m)
	if rv.Kind() != reflect.Struct {
		return out
	}
	for i := 0; i < rv.NumField(); i++ {
		switch f := rv.Field(i); f.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			out[i] = f.Int()
		}
	}
	return out
}

// tui1bHasIntPair 问：有没有**同一个**字段，在 a 里是 wantA、在 b 里是 wantB。
// 要求同一个字段两次都对上，比单看一次难碰巧撞上。
func tui1bHasIntPair(a, b Model, wantA, wantB int64) bool {
	fa, fb := tui1bIntFields(a), tui1bIntFields(b)
	for i, va := range fa {
		if va == wantA && fb[i] == wantB {
			return true
		}
	}
	return false
}

const tui1bESC = 0x1b

// tui1bStripANSI 剥掉 CSI 与 OSC 转义序列，好按显示内容数宽度。
// 用状态机而不是正则，是为了不在源码里写转义字面量。
func tui1bStripANSI(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != tui1bESC {
			b.WriteRune(rs[i])
			continue
		}
		i++
		if i >= len(rs) {
			break
		}
		switch rs[i] {
		case '[': // CSI：吃到 0x40-0x7E 的终止字节
			for i++; i < len(rs); i++ {
				if rs[i] >= 0x40 && rs[i] <= 0x7e {
					break
				}
			}
		case ']': // OSC：吃到 BEL，或者 ESC 反斜杠
			for i++; i < len(rs); i++ {
				if rs[i] == 0x07 {
					break
				}
				if rs[i] == tui1bESC && i+1 < len(rs) && rs[i+1] == '\\' {
					i++
					break
				}
			}
		}
	}
	return b.String()
}

// tui1bHasLineOfWidth 问：View 的正文里有没有一行**恰好**这么多个字符。
func tui1bHasLineOfWidth(content string, want int) bool {
	for _, line := range strings.Split(tui1bStripANSI(content), "\n") {
		if len([]rune(strings.TrimRight(line, "\r"))) == want {
			return true
		}
	}
	return false
}

// ═══ 验收 1 ═══════════════════════════════════════════════════════

// 验收 1：WindowSizeMsg{0,0} 之后内部宽高是 80 / 24，{100,30} 之后是 100 / 30。
//
// 【判别力】能抓「拿到 0 就原样存下来」这个坑（契约 5.2 点名的那条）。
// 反射按值配对而不按字段名，理由见文件开头第二段。
// 若实现把宽高存成 int 以外的类型（比如 uint 或者塞进一个嵌套结构体），
// 本条会红成假阳性 —— 失败信息里把所有整数字段的值列出来，好让人一眼分辨。
func TestTui1bAcc1WindowSizeFallsBackTo80x24(t *testing.T) {
	base, _, _ := tui1bNew(t, nil)
	zero, cmd := tui1bStep(t, base, tea.WindowSizeMsg{Width: 0, Height: 0})
	if cmd != nil {
		t.Errorf("WindowSizeMsg 应当返回 nil Cmd（契约 5.2），实际非 nil")
	}
	big, _ := tui1bStep(t, base, tea.WindowSizeMsg{Width: 100, Height: 30})

	if !tui1bHasIntPair(zero, big, 80, 100) {
		t.Errorf("没有哪个整数字段在 {0,0} 之后是 80、在 {100,30} 之后是 100；"+
			"宽度兜底没做或者没存进 Model。{0,0} 之后的整数字段 = %v，{100,30} 之后 = %v",
			tui1bIntFields(zero), tui1bIntFields(big))
	}
	if !tui1bHasIntPair(zero, big, 24, 30) {
		t.Errorf("没有哪个整数字段在 {0,0} 之后是 24、在 {100,30} 之后是 30；"+
			"高度兜底没做或者没存进 Model。{0,0} 之后的整数字段 = %v，{100,30} 之后 = %v",
			tui1bIntFields(zero), tui1bIntFields(big))
	}
}

// 验收 1 的旁证：宽度有没有真的进到 View。
//
// 【代理断言，可能脆】契约 5.5 冻结了活区第二行是一条宽度 min(width,120) 的横线，
// 但没冻结用哪个字符画，也没说会不会加样式。这里剥掉 ANSI 之后按字符数找
// 「存在一行恰好 80 / 100 个字符」。实现若用别的方式排版（比如给横线留边距），
// 这条会红而 Model 其实是对的 —— 失败时把 View 正文原样打出来，好当场裁。
func TestTui1bAcc1WidthReachesTheView(t *testing.T) {
	base, _, _ := tui1bNew(t, nil)

	zero, _ := tui1bStep(t, base, tea.WindowSizeMsg{Width: 0, Height: 0})
	if got := zero.View().Content; !tui1bHasLineOfWidth(got, 80) {
		t.Errorf("{0,0} 之后 View 里没有一行恰好 80 个字符（契约 5.5 的横线宽度 min(width,120)，"+
			"width 兜底成 80）。View 正文如下（这是代理断言，也可能是我数宽度的方式太死）:\n%s", got)
	}

	big, _ := tui1bStep(t, base, tea.WindowSizeMsg{Width: 100, Height: 30})
	if got := big.View().Content; !tui1bHasLineOfWidth(got, 100) {
		t.Errorf("{100,30} 之后 View 里没有一行恰好 100 个字符。View 正文如下"+
			"（这是代理断言）:\n%s", got)
	}
}

// ═══ 验收 3 ═══════════════════════════════════════════════════════

// 验收 3：busy 期间按 Enter 不提交。本刀最重要的一条。
//
// 【判别力，也是最容易写空的一条】三处防空转：
//  1. 先把第一次提交的 Cmd 真跑一遍，让计数从 0 变成 1。不这么做的话
//     「0 没有变成 1」也满足「次数没有增加」，坏实现照样绿。
//  2. 第二次按 Enter 之前**重新把输入框填满**。第一次提交之后输入框被 Reset 清空，
//     不重新填，空输入那条分支自己就会拦住提交，忙闲判断有没有写都看不出来。
//  3. 第二次的返回 Cmd 也跑一遍。漏了忙闲判断的实现会把 submitCmd 放进返回的 Cmd 里，
//     不跑就看不见 Submit 被调。
func TestTui1bAcc3BusyEnterDoesNotSubmit(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)

	m = tui1bType(t, m, "hello")
	m, cmd := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd)
	if sub.count() != 1 {
		t.Fatalf("前提没成立：第一次提交之后 Submit 的调用次数是 %d，应当是 1", sub.count())
	}

	// 不喂 submitDoneMsg，Model 仍然是 busy。
	m = tui1bType(t, m, "again")
	before := sub.count()
	_, cmd2 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd2)

	if got := sub.count(); got != before {
		t.Errorf("busy 期间按 Enter 之后 Submit 的调用次数从 %d 变成了 %d，一次都不许被调；"+
			"第二次收到的文本是 %q", before, got, sub.text(1))
	}
}

// ═══ 验收 4 ═══════════════════════════════════════════════════════

// 验收 4：空输入（含只有空白）按 Enter 不提交。
//
// 【判别力】空串那一格结构上确实很难为假 —— textarea 是空的，多数实现都会拦住。
// 有牙的是只有空白那一格：契约 5.4 第 1 步要求 TrimSpace 之后为空就什么都不做，
// 少写这个 Trim 的实现会把 "   " 提交出去。
func TestTui1bAcc4EmptyEnterDoesNotSubmit(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed string
	}{
		{"完全空", ""},
		{"只有空白", "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, sub, _ := tui1bSized(t, nil)
			m = tui1bType(t, m, tc.seed)

			_, cmd := tui1bStep(t, m, tui1bEnter())
			tr := tui1bRun(t, cmd)

			if sub.count() != 0 {
				t.Errorf("输入框里是 %q 时按 Enter 调了 Submit %d 次（收到 %q），应当一次都不调",
					tc.seed, sub.count(), sub.text(0))
			}
			if tr.hasQuit() {
				t.Errorf("空输入按 Enter 产出了 QuitMsg，不该退出；跑出来的是 %v", tr.log)
			}
		})
	}
}

// ═══ 验收 5 ═══════════════════════════════════════════════════════

// 验收 5 之一：正常提交调一次、文本对、之后 busy。
//
// 【判别力】busy 那半段用行为证明（契约没给 busy 的访问器）：
// 重新填满输入框再按一次 Enter，次数仍是 1 才说明 busy 立住了。
func TestTui1bAcc5SubmitCallsSubmitterOnceAndGoesBusy(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)

	m = tui1bType(t, m, "hello")
	m, cmd := tui1bStep(t, m, tui1bEnter())
	if cmd == nil {
		t.Fatalf("非空输入按 Enter 返回了 nil Cmd，契约 5.4 要求返回回显与 submitCmd 的序列")
	}
	tr := tui1bRun(t, cmd)

	if sub.count() != 1 {
		t.Fatalf("Submit 被调了 %d 次，应当恰好 1 次", sub.count())
	}
	if got := sub.text(0); got != "hello" {
		t.Errorf("Submit 收到的是 %q，应当是 %q", got, "hello")
	}
	if tr.log[len(tr.log)-1] != "submitDone" {
		t.Errorf("提交序列跑到最后产出的是 %v，最后一条应当是 submitDoneMsg", tr.log)
	}

	// busy 的行为证据。
	m = tui1bType(t, m, "second")
	_, cmd2 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd2)
	if sub.count() != 1 {
		t.Errorf("提交之后 Model 没有进入 busy：又按一次 Enter 把 Submit 调到了 %d 次（收到 %q）",
			sub.count(), sub.text(1))
	}
}

// 验收 5 之二：提交之后输入框被清空。
//
// 【判别力】契约没给输入框的访问器，这里用行为证明：
// 先喂一条 submitDoneMsg 解除 busy，再**什么都不打**直接按 Enter。
// 输入框要是没被 Reset，里面还躺着 "hello"，这一下会把它再提交一次。
func TestTui1bAcc5SubmitClearsTheInputBox(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)

	m = tui1bType(t, m, "hello")
	m, cmd := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd)
	if sub.count() != 1 {
		t.Fatalf("前提没成立：Submit 的调用次数是 %d，应当是 1", sub.count())
	}

	m, _ = tui1bStep(t, m, submitDoneMsg{err: nil})

	_, cmd2 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd2)
	if got := sub.count(); got != 1 {
		t.Errorf("提交之后输入框没有被清空：没打任何字就按 Enter，Submit 的调用次数变成了 %d，"+
			"第二次收到的是 %q", got, sub.text(1))
	}
}

// ═══ 验收 6 ═══════════════════════════════════════════════════════

// 验收 6：多行提交逐字节保留 "a\nb"。
//
// 【判别力】换行被压成空格、被丢掉、被换成 "\r\n"，三种都红。
// 两条路各走一遍，因为「把 a\nb 放进输入框」有两种自然做法，
// 而它们经过的是 Update 里不同的分支：
//   - 键盘路：'a'、Ctrl+J、'b'，走的是契约 5.2 的 Ctrl+J 分支；
//   - 粘贴路：tea.PasteMsg{Content: "a\nb"}，走的是 5.2 的 PasteMsg 分支。
//     这条顺带钉住 5.2 那句「转给 textarea 就行，不要自己再插一遍」：
//     插两遍的话收到的会是 "a\nba\nb"。
func TestTui1bAcc6MultilineSubmitKeepsNewlineByteForByte(t *testing.T) {
	t.Run("键盘路 Ctrl+J", func(t *testing.T) {
		m, sub, _ := tui1bSized(t, nil)
		m = tui1bType(t, m, "a")
		m, _ = tui1bStep(t, m, tui1bCtrl('j'))
		m = tui1bType(t, m, "b")

		_, cmd := tui1bStep(t, m, tui1bEnter())
		tui1bRun(t, cmd)

		if sub.count() != 1 {
			t.Fatalf("Submit 被调了 %d 次，应当恰好 1 次", sub.count())
		}
		if got := sub.text(0); got != "a\nb" {
			t.Errorf("Submit 收到的是 %q，应当逐字节等于 %q", got, "a\nb")
		}
	})

	t.Run("粘贴路 PasteMsg", func(t *testing.T) {
		m, sub, _ := tui1bSized(t, nil)
		m, _ = tui1bStep(t, m, tea.PasteMsg{Content: "a\nb"})

		_, cmd := tui1bStep(t, m, tui1bEnter())
		tui1bRun(t, cmd)

		if sub.count() != 1 {
			t.Fatalf("Submit 被调了 %d 次，应当恰好 1 次", sub.count())
		}
		if got := sub.text(0); got != "a\nb" {
			t.Errorf("Submit 收到的是 %q，应当逐字节等于 %q（收到 %q 说明粘贴的内容进去了两遍）",
				got, "a\nb", got)
		}
	})
}

// 验收 6 附带：契约 5.4 第 2 步的回显。
//
// 【判别力】钉住「按行回显、每行前缀 "> "」这条冻结文本，
// 以及回显必须排在 submitCmd 前面（5.4 第 5 步要求 tea.Sequence）。
// 单行被拼成一行、前缀写成别的、回显跑到 Submit 后面，三种都红。
func TestTui1bAcc6SubmitEchoesEachLineBeforeSubmitting(t *testing.T) {
	m, _, pr := tui1bSized(t, nil)
	m, _ = tui1bStep(t, m, tea.PasteMsg{Content: "a\nb"})

	_, cmd := tui1bStep(t, m, tui1bEnter())
	tr := tui1bRun(t, cmd)

	if want := []string{"> a", "> b"}; !reflect.DeepEqual(pr.snapshot(), want) {
		t.Errorf("回显调 Println 的参数是 %q，契约 5.4 第 2 步要求按行回显、每行前缀 %q，"+
			"即 %q", pr.snapshot(), "> ", want)
	}
	if want := []string{"print:> a", "print:> b", "submitDone"}; !reflect.DeepEqual(tr.log, want) {
		t.Errorf("提交返回的 Cmd 跑出来是 %v，应当是 %v（回显要排在 submitCmd 之前）", tr.log, want)
	}
	if tr.sawBatch {
		t.Errorf("提交返回的是 tea.Batch，契约 5.4 第 5 步要求 tea.Sequence；Batch 不保证顺序，"+
			"回显可能落到 Submit 之后。跑出来的顺序是 %v", tr.log)
	}
}

// ═══ 验收 7 ═══════════════════════════════════════════════════════

// 验收 7：Ctrl+J 插换行、不提交。
//
// 【判别力】前半段（不提交、不退出）有牙：把换行错绑到 Enter 之外的其它分支上，
// 或者让 Ctrl+J 落进「其余按键转给 textarea」而 textarea 又把它当提交，都会红。
// 后半段「输入框的值里多了一个 \n」没有访问器，用行为证明：
// Ctrl+J 前后各打一个字，提交之后 Submitter 收到的应当是 "a\nb" 而不是 "ab"。
func TestTui1bAcc7CtrlJInsertsNewlineWithoutSubmitting(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)
	m = tui1bType(t, m, "a")

	m, cmd := tui1bStep(t, m, tui1bCtrl('j'))
	tr := tui1bRun(t, cmd)

	if sub.count() != 0 {
		t.Errorf("Ctrl+J 之后 Submit 被调了 %d 次（收到 %q），应当一次都不调", sub.count(), sub.text(0))
	}
	if tr.hasQuit() {
		t.Errorf("Ctrl+J 产出了 QuitMsg，不该退出；跑出来的是 %v", tr.log)
	}

	// 换行真的进去了没有：靠下一次提交看。
	m = tui1bType(t, m, "b")
	_, cmd2 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd2)
	if got := sub.text(0); got != "a\nb" {
		t.Errorf("Ctrl+J 之后提交拿到的是 %q，应当是 %q —— 换行没插进输入框", got, "a\nb")
	}
}

// ═══ 验收 8 ═══════════════════════════════════════════════════════

// 验收 8：busy 时连按两次 Ctrl-C 取消本轮、不退出。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
//
// 【判别力】契约 5.1 的 Options 里没有 ctx 这一项，5.4 第 4 步却写
// context.WithCancel(m.ctx)，所以「注入的 cancel」其实没地方注入。
// 唯一能观测的把手是假 Submitter 在 Submit 里收到的那个 ctx。
// 三段都有牙：
//   - Ctrl-C 之前 ctx.Err() 必须是 nil，否则「取消了」这件事恒成立；
//   - Ctrl-C 之后必须是 context.Canceled；
//   - 返回的 Cmd 跑出来不许有 QuitMsg。
func TestTui1bAcc8BusyCtrlCCancelsWithoutQuitting(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)

	m = tui1bType(t, m, "hello")
	m, cmd := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd)
	if sub.count() != 1 {
		t.Fatalf("前提没成立：Submit 的调用次数是 %d，应当是 1", sub.count())
	}

	ctx := sub.ctx(0)
	if ctx == nil {
		t.Fatalf("Submit 收到的 ctx 是 nil，契约 5.4 第 4 步要求传本轮 context.WithCancel 出来的那个")
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("Ctrl-C 之前 Submit 拿到的 ctx 就已经是 %v，本条断言会恒真，前提没控制住", err)
	}

	m, cmdArm := tui1bStep(t, m, tui1bCtrl('c'))
	if err := ctx.Err(); err != nil {
		t.Errorf("busy 时第一次 Ctrl-C 之后，本轮 ctx 的 Err() 是 %v，应当仍是 nil", err)
	}
	if cmdArm == nil || reflect.ValueOf(cmdArm).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
		t.Errorf("busy 时第一次 Ctrl-C 应当只武装、不退出（cmd == nil：%v）", cmdArm == nil)
	}
	_, cmd2 := tui1bStep(t, m, tui1bCtrl('c'))
	if err := ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("busy 时连按两次 Ctrl-C 之后，本轮 ctx 的 Err() 是 %v，应当是 context.Canceled", err)
	}
	tr := tui1bRun(t, cmd2)
	if tr.hasQuit() {
		t.Errorf("busy 时按 Ctrl-C 产出了 QuitMsg，契约 5.2 要求不退出、返回 nil Cmd；跑出来的是 %v", tr.log)
	}
}

// ═══ 验收 9 ═══════════════════════════════════════════════════════

// 验收 9：不忙时连按两次 Ctrl-C 退出。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
//
// 【判别力】这条钉的是「跑到最后产出 QuitMsg」。直接比 Cmd 与 tea.Quit
// 在 Go 里编译不过（函数不可比较），所以必须把 Cmd 跑开。
// 缓冲是空的，按 compactCmds 的收缩规则（0 条返回 nil，1 条原样返回），
// quitCmd 这时返回的就是 tea.Quit 本身，跑出来是一条裸的 QuitMsg。
func TestTui1bAcc9IdleCtrlCQuits(t *testing.T) {
	m, _, _ := tui1bSized(t, nil)

	m, cmdArm := tui1bStep(t, m, tui1bCtrl('c'))
	if cmdArm == nil || reflect.ValueOf(cmdArm).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
		t.Fatalf("不忙时第一次 Ctrl-C 应当只武装、不退出（cmd == nil：%v）", cmdArm == nil)
	}
	_, cmd := tui1bStep(t, m, tui1bCtrl('c'))
	if cmd == nil {
		t.Fatalf("不忙时第二次 Ctrl-C 返回了 nil Cmd，契约 5.2 要求返回 quitCmd()")
	}
	tr := tui1bRun(t, cmd)
	if !tr.hasQuit() {
		t.Errorf("不忙时连按两次 Ctrl-C 返回的 Cmd 跑到最后没有产出 tea.QuitMsg；跑出来的是 %v", tr.log)
	}
	// 缓冲空、Drainer 什么都不吐，按契约 5.3 就只该有一条 tea.Quit。
	// 多出来的东西说明退出路上还顺手干了别的。
	if want := []string{"quit"}; !reflect.DeepEqual(tr.log, want) {
		t.Errorf("缓冲是空的，不忙时连按两次 Ctrl-C 跑出来是 %v，应当只有 %v", tr.log, want)
	}
}

// ═══ 验收 10 ═══════════════════════════════════════════════════════

// 验收 10：退出前把缓冲吐干净，而且吐在 Quit 之前。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
//
// 【判别力，本刀唯一能抓「用了 Batch 而不是 Sequence」的一条】
// 三种坏实现分别被三句抓住：
//   - 压根没 Flush / 没换出口 -> 记录器一条都收不到，顺序断言少两项，红。
//   - 把 flush 做成一条 Cmd 放进 Sequence（契约 5.3 点名的坑）-> 那时出口还是 nil，
//     残段进 pending 不出来，记录器仍然收不到，红。
//   - 用 tea.Batch 而不是 tea.Sequence -> tui1bRun 对 BatchMsg 按倒序跑
//     （execBatchMsg 允许的一种合法排期），顺序变成 quit 在前，红；
//     sawBatch 再直接把原因说出来。
//
// 缓冲里故意放「一个完整行 + 一段不以换行结尾的残段」：
// LineWriter 的 SetEmit 换上非 nil 出口时会把 pending 里的完整行补吐，
// Flush 再交出残段，所以两条都该在 Quit 之前出去，且完整行在前。
func TestTui1bAcc10QuitDrainsBufferBeforeQuitting(t *testing.T) {
	lw := NewLineWriter(nil)
	m, _, pr := tui1bSized(t, lw)

	if _, err := lw.Write([]byte("完整行\n残段")); err != nil {
		t.Fatalf("往 LineWriter 里写失败: %v", err)
	}

	m, cmdArm := tui1bStep(t, m, tui1bCtrl('c'))
	if cmdArm == nil || reflect.ValueOf(cmdArm).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
		t.Fatalf("不忙时第一次 Ctrl-C 应当只武装、不退出（cmd == nil：%v）", cmdArm == nil)
	}
	_, cmd := tui1bStep(t, m, tui1bCtrl('c'))
	if cmd == nil {
		t.Fatalf("不忙时第二次 Ctrl-C 返回了 nil Cmd，缓冲里还有东西没吐")
	}
	tr := tui1bRun(t, cmd)

	want := []string{"print:完整行", "print:残段", "quit"}
	if !reflect.DeepEqual(tr.log, want) {
		t.Errorf("退出序列跑出来是 %v，应当是 %v —— 缓冲里的东西必须在 QuitMsg 之前全部出去，"+
			"而且顺序不能乱", tr.log, want)
	}
	if tr.sawBatch {
		t.Errorf("quitCmd 用的是 tea.Batch。契约 5.3 要求 tea.Sequence：Batch 的子 Cmd 各起一个" +
			" goroutine 并发跑（bubbletea tea.go execBatchMsg），顺序没有保证，" +
			"残段可能落到 tea.Quit 之后，那时程序已经在退、消息丢掉")
	}
	if want := []string{"完整行", "残段"}; !reflect.DeepEqual(pr.snapshot(), want) {
		t.Errorf("Println 被调用的参数是 %q，应当是 %q", pr.snapshot(), want)
	}
}

// ═══ 验收 11 ═══════════════════════════════════════════════════════

// 验收 11：submitDoneMsg 带 quit 哨兵时退出，且 busy 变回 false。
//
// 【判别力】这一条最初只断言「跑出来有 QuitMsg」，变异验证把它抓了个现行：
// 契约 5.2 的第 2 支（IsQuit 为真）和第 3 支（err 非 nil 且不是 quit）**都会退出**，
// 差别只在第 3 支要先把错误文本打出去。所以「有 QuitMsg」分不开这两支 ——
// 把第 2 支整个删掉，用例照样绿。改成断言产出**恰好**是一条 quit：
// 落进第 3 支的话会多出一条 print，红。
// busy 变回 false 那半段用行为证明：重新打字按 Enter，Submit 的调用次数要能涨到 2。
// busy 若卡在 true，次数停在 1，红。
func TestTui1bAcc11SubmitDoneWithQuitSentinelQuits(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)
	sub.quit = true

	m = tui1bType(t, m, "hello")
	m, cmd := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd)
	if sub.count() != 1 {
		t.Fatalf("前提没成立：Submit 的调用次数是 %d，应当是 1", sub.count())
	}

	m, cmd2 := tui1bStep(t, m, submitDoneMsg{err: errors.New("哨兵错误")})
	if cmd2 == nil {
		t.Fatalf("IsQuit 为真时 submitDoneMsg 返回了 nil Cmd，契约 5.2 要求返回 quitCmd()")
	}
	tr := tui1bRun(t, cmd2)
	if !tr.hasQuit() {
		t.Errorf("IsQuit 为真时 submitDoneMsg 返回的 Cmd 跑到最后没有产出 tea.QuitMsg；跑出来的是 %v", tr.log)
	}
	if want := []string{"quit"}; !reflect.DeepEqual(tr.log, want) {
		t.Errorf("IsQuit 为真时 submitDoneMsg 跑出来是 %v，应当只有 %v；多出来的 print 说明"+
			"走的是契约 5.2 的第 3 支（err 非 nil 且不是 quit，要先打错误文本），"+
			"而不是第 2 支（IsQuit 为真，直接 quitCmd）", tr.log, want)
	}

	// busy 变回 false 的行为证据。
	m = tui1bType(t, m, "again")
	_, cmd3 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd3)
	if got := sub.count(); got != 2 {
		t.Errorf("submitDoneMsg 之后 busy 没有回到 false：重新打字按 Enter，"+
			"Submit 的调用次数是 %d，应当涨到 2", got)
	}
}

// ═══ 验收 12 ═══════════════════════════════════════════════════════

// 验收 12：IsQuit 为假、err 为 nil 时不退出，busy 变回 false，返回 nil Cmd。
//
// 【判别力】nil Cmd 那一句抓「顺手返回了点什么」；
// busy 那一句同样用行为证明。
func TestTui1bAcc12SubmitDoneWithoutErrorGoesIdle(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)
	sub.quit = false

	m = tui1bType(t, m, "hello")
	m, cmd := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd)
	if sub.count() != 1 {
		t.Fatalf("前提没成立：Submit 的调用次数是 %d，应当是 1", sub.count())
	}

	m, cmd2 := tui1bStep(t, m, submitDoneMsg{err: nil})
	if cmd2 != nil {
		tr := tui1bRun(t, cmd2)
		t.Errorf("err 为 nil 的 submitDoneMsg 返回了非 nil Cmd，契约 5.2 第 4 支要求返回 nil；"+
			"跑出来的是 %v", tr.log)
	}

	m = tui1bType(t, m, "again")
	_, cmd3 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd3)
	if got := sub.count(); got != 2 {
		t.Errorf("submitDoneMsg 之后 busy 没有回到 false：重新打字按 Enter，"+
			"Submit 的调用次数是 %d，应当涨到 2", got)
	}
}

// ═══ 验收 13 ═══════════════════════════════════════════════════════

// 验收 13：Esc 不退出、不提交。
//
// 【判别力】抓「照着别的 TUI 的习惯把 Esc 绑成退出」。
// 契约点名了理由：输入到一半误触 Esc 直接退出，草稿就没了。
// TUI-1（docs/21e_TUI1_TASK.md §2）之后「草稿还在」这一句反过来了：空闲、输入框有字时
// Esc 清空输入框。这条用例仍然钉住它原本要钉的两件事（不退出、不提交），
// 并补上清空本身与「清空之后回车什么都不发」——后者才是「不提交」在提交入口上的证明。
func TestTui1bAcc13EscapeDoesNotQuit(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)
	m = tui1bType(t, m, "半截草稿")

	m, cmd := tui1bStep(t, m, tui1bEsc())
	tr := tui1bRun(t, cmd)

	if tr.hasQuit() {
		t.Errorf("按 Esc 产出了 tea.QuitMsg，契约 5.2 要求本刀什么都不做；跑出来的是 %v", tr.log)
	}
	if sub.count() != 0 {
		t.Errorf("按 Esc 调了 Submit %d 次（收到 %q），应当一次都不调", sub.count(), sub.text(0))
	}

	// TUI-1 §2：空闲、输入框有字时 Esc 清空输入框，光标回到开头。
	if got := m.ta.Value(); got != "" {
		t.Errorf("Esc 之后输入框应当是空的（TUI-1 §2），实际 %q", got)
	}
	if l, c := m.ta.Line(), m.ta.Column(); l != 0 || c != 0 {
		t.Errorf("Esc 之后光标应当在 (0, 0)，实际 (%d, %d)", l, c)
	}

	// 清空之后回车不提交任何东西。这一句是「按 Esc 不提交」在提交入口上的证明：
	// 光看按键那一刻的 Submit 计数，分不出「没提交」与「提交了空串」。
	_, cmd2 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd2)
	if n := sub.count(); n != 0 {
		t.Errorf("Esc 清空之后回车不应当提交，实际 Submit %d 次（收到 %q）", n, sub.text(0))
	}
}

// ═══ 验收 14 ═══════════════════════════════════════════════════════

// 验收 14：View() 是内联视图，AltScreen 为 false。
//
// 【判别力弱】AltScreen 是 bool 零值，实现什么都不写它也是 false。
// 这条能抓的只有「凭 v1 的记忆写了 v.AltScreen = true」。
// 契约 5.5 要求显式写出来并注释，那一层这里看不见（编译产物里没有注释）。
// 顺手把「收过尺寸」和「没收过尺寸」两种都看一遍。
func TestTui1bAcc14ViewIsInline(t *testing.T) {
	fresh, _, _ := tui1bNew(t, nil)
	if fresh.View().AltScreen {
		t.Errorf("没收过 WindowSizeMsg 时 View().AltScreen 是 true，本刀要内联，必须是 false")
	}

	sized, _, _ := tui1bSized(t, nil)
	if sized.View().AltScreen {
		t.Errorf("收过 WindowSizeMsg 之后 View().AltScreen 是 true，本刀要内联，必须是 false")
	}
}

// ═══ 验收 15 ═══════════════════════════════════════════════════════

// 验收 15：没收到过 WindowSizeMsg 就调 View() 不 panic。
//
// 【判别力】抓「拿 width 去算 strings.Repeat 或切片而没兜底」这类崩法。
// 用 recover 接住，是为了 panic 不把同一个包里其它用例一起带走。
// 顺带把 Init() 也调一下：契约 5.6 要求它返回构造时攒下的 focusCmd，
// 这里只看它不崩，不跑返回的 Cmd（那是 textarea 光标闪烁的定时器）。
func TestTui1bAcc15ViewSurvivesZeroWidth(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("没收到过 WindowSizeMsg 就调 View() 时 panic: %v", r)
		}
	}()

	m, _, _ := tui1bNew(t, nil)
	m.Init()
	_ = m.View().Content

	// 显式送一条 {0,0}（非 TTY 下 bubbletea 就是这么送的），再画一次。
	zero, _ := tui1bStep(t, m, tea.WindowSizeMsg{Width: 0, Height: 0})
	_ = zero.View().Content
}
