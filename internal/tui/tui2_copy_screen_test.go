package tui

// tui2_copy_screen_test.go 钉住 TUI-2（docs/21f_TUI2_COPY_MODE_TASK.md §7）的九条验收：
// 进入/退出、没有内容时不进整屏、中文选区映射、反向拖动、越界夹取、滚动清选区、
// 空选区不复制、复制后不退出、非复制模式不抢鼠标。
//
// 纯 Model 驱动，不起 Program。鼠标事件照渲染器的路径走：View().OnMouse(ev) 交回
// 一条 Cmd，跑出来的 copyScreenMouseMsg 再交给 Update —— 与 bubbletea v2.0.9
// 的 cursed_renderer.onMouse + tea.go 的 `go p.Send(cmd())` 同一条路。
// 剪贴板纪律同 tui1e：SetClipboard 那条 Cmd 一律不执行，只判非 nil。

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
)

// tui2Sub 是带复制模式能力的假 Submitter。
// 另外实现 CopyRecent 并计数：/copy --screen 必须走复制模式，不能掉进 /copy 的取数路径。
type tui2Sub struct {
	tui1bSubmitter
	lines     []repl.CopyLine
	truncated bool
	err       error
	screenN   int
	recentN   int
}

func (s *tui2Sub) CopyScreen(context.Context) ([]repl.CopyLine, bool, error) {
	s.screenN++
	return s.lines, s.truncated, s.err
}

func (s *tui2Sub) CopyRecent(context.Context, string) (string, error) {
	s.recentN++
	return "", nil
}

// tui2Fixture 是两条消息，排成五行正文：
//
//	[你]        行 0
//	中文abc     行 1
//	(空行)      行 2
//	[alice]     行 3
//	第二行      行 4
func tui2Fixture() []repl.CopyLine {
	return []repl.CopyLine{
		{User: true, Text: "中文abc"},
		{Who: "alice", Text: "第二行"},
	}
}

// tui2Many 是 n 条单行消息，用来造出超过一屏的正文（n=30 时 89 行）。
func tui2Many(n int) []repl.CopyLine {
	out := make([]repl.CopyLine, n)
	for i := range out {
		out[i] = repl.CopyLine{Who: "m", Text: "行"}
	}
	return out
}

// tui2CtrlG 是进复制模式的键。
var tui2CtrlG = tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}

// tui2New 造一个 80x24、装好假 Submitter 的 Model。
func tui2New(t *testing.T, sub Submitter) Model {
	t.Helper()
	m := New(Options{
		Submit:  sub,
		Out:     tui1bNopDrainer{},
		Version: "0.0.0-test",
	})
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

// tui2Loaded 从取数 Cmd 的产出里取出唯一的 copyScreenLoadedMsg。
func tui2Loaded(t *testing.T, cmd tea.Cmd) copyScreenLoadedMsg {
	t.Helper()
	tr := tui1bRun(t, cmd)
	var out []copyScreenLoadedMsg
	for _, msg := range tr.msgs {
		if d, ok := msg.(copyScreenLoadedMsg); ok {
			out = append(out, d)
		}
	}
	if len(out) != 1 {
		t.Fatalf("Cmd 产出里应当恰好 1 条 copyScreenLoadedMsg，实际 %d 条：%v", len(out), tr.log)
	}
	return out[0]
}

// tui2Enter 是 Ctrl+G 进整屏并处理取数结果。
func tui2Enter(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := tui1bStep(t, m, tui2CtrlG)
	if cmd == nil {
		t.Fatalf("Ctrl+G 应当交回取数 Cmd")
	}
	if m.copyScr.on {
		t.Fatalf("前提不成立：取数还没回来就进了整屏")
	}
	m, _ = tui1bStep(t, m, tui2Loaded(t, cmd))
	if !m.copyScr.on {
		t.Fatalf("前提不成立：取数成功之后应当进入复制模式")
	}
	return m
}

func tui2Click(x, y int) tea.MouseMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func tui2Motion(x, y int) tea.MouseMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func tui2Release(x, y int) tea.MouseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// tui2Mouse 走渲染器同一条路：OnMouse → Cmd → copyScreenMouseMsg → Update。
func tui2Mouse(t *testing.T, m Model, ev tea.MouseMsg) (Model, tea.Cmd) {
	t.Helper()
	on := m.View().OnMouse
	if on == nil {
		t.Fatalf("复制模式下 View().OnMouse 是 nil，鼠标事件不会送到 Update")
	}
	cmd := on(ev)
	if cmd == nil {
		t.Fatalf("OnMouse 没有交回 Cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Fatalf("OnMouse 的 Cmd 产出 nil")
	}
	return tui1bStep(t, m, msg)
}

// tui2Drag 是一次完整拖选：按下、拖动、松开。
func tui2Drag(t *testing.T, m Model, x0, y0, x1, y1 int) (Model, tea.Cmd) {
	t.Helper()
	m, _ = tui2Mouse(t, m, tui2Click(x0, y0))
	m, _ = tui2Mouse(t, m, tui2Motion(x1, y1))
	return tui2Mouse(t, m, tui2Release(x1, y1))
}

// tui2Frame 是复制模式那一帧的逐行文本。
func tui2Frame(m Model) []string { return strings.Split(m.View().Content, "\n") }

func tui2Top(m Model) string {
	f := tui2Frame(m)
	return f[0]
}

func tui2Bottom(m Model) string {
	f := tui2Frame(m)
	return f[len(f)-1]
}

func tui2WantTop(t *testing.T, where string, m Model, want string) {
	t.Helper()
	if got := tui2Top(m); got != want {
		t.Errorf("%s：顶栏应当是 %q，实际 %q", where, want, got)
	}
}

func tui2WantTopHas(t *testing.T, where string, m Model, want string) {
	t.Helper()
	if got := tui2Top(m); !strings.Contains(got, want) {
		t.Errorf("%s：顶栏应当包含 %q，实际 %q", where, want, got)
	}
}

func tui2WantBottom(t *testing.T, where string, m Model, want string) {
	t.Helper()
	if got := tui2Bottom(m); got != want {
		t.Errorf("%s：页脚应当是 %q，实际 %q", where, want, got)
	}
}

// TestTui2EnterExit 是 §7 第 1 条：Ctrl+G 进入时 View 的三个字段一起变，
// Esc 退出后全部复原，输入框内容与光标一格不差。
func TestTui2EnterExit(t *testing.T) {
	sub := &tui2Sub{lines: tui2Fixture()}
	m := tui2New(t, sub)
	m = tui1bType(t, m, "草稿")
	m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	value, line, col := m.ta.Value(), m.ta.Line(), m.ta.Column()
	if value != "草稿" || line != 0 || col != 1 {
		t.Fatalf("前提不成立：输入框应当是 %q、光标 (0,1)，实际 %q、( %d,%d )", "草稿", value, line, col)
	}

	m, cmd := tui1bStep(t, m, tui2CtrlG)
	if cmd == nil || m.copyScr.on {
		t.Fatalf("Ctrl+G：应当交回取数 Cmd 且还没进整屏，cmd == nil：%v，on：%v", cmd == nil, m.copyScr.on)
	}
	m, _ = tui1bStep(t, m, tui2Loaded(t, cmd))
	if !m.copyScr.on {
		t.Fatalf("取数成功之后应当进入复制模式")
	}
	if sub.screenN != 1 {
		t.Errorf("CopyScreen 应当被调 1 次，实际 %d 次", sub.screenN)
	}

	v := m.View()
	if !v.AltScreen {
		t.Error("复制模式下 View().AltScreen 应当是 true（临时整屏）")
	}
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("复制模式下 MouseMode 应当是 CellMotion，实际 %v", v.MouseMode)
	}
	if v.OnMouse == nil {
		t.Error("复制模式下 View().OnMouse 不应当是 nil")
	}
	tui2WantTop(t, "刚进复制模式", m, "复制模式  ↑↓/PgUp/PgDn 滚动  Esc 退出  拖动选择")
	tui2WantBottom(t, "刚进复制模式", m, "第 1–5 行 / 共 5 行")

	m, cmd = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	atxWantNilCmd(t, "Esc 退出复制模式", cmd)
	if m.copyScr.on {
		t.Error("Esc 之后应当已经退出复制模式")
	}
	v = m.View()
	if v.AltScreen {
		t.Error("退出之后 AltScreen 必须回到 false，否则消息回不到终端回滚区")
	}
	if v.MouseMode != tea.MouseModeNone {
		t.Errorf("退出之后 MouseMode 必须是 None（终端原生选中要还回去），实际 %v", v.MouseMode)
	}
	if v.OnMouse != nil {
		t.Error("退出之后 View().OnMouse 必须回到 nil")
	}
	atxWantValue(t, "退出之后输入框内容", m, value)
	if l, c := m.ta.Line(), m.ta.Column(); l != line || c != col {
		t.Errorf("退出之后光标应当在 (%d,%d)，实际 (%d,%d)", line, col, l, c)
	}

	// q 同样退出（复制模式里没有输入框，q 不输入文本）。
	m = tui2Enter(t, m)
	m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: 'q'})
	if m.copyScr.on || m.View().AltScreen {
		t.Error("按 q 之后应当退出复制模式")
	}

	// Ctrl+C 照旧：第一次只武装（不退出、不进一半），2 秒内第二次才退出程序。
	m = tui2Enter(t, m)
	m, cmd = tui1bStep(t, m, tui1bCtrl('c'))
	if !m.copyScr.on {
		t.Error("复制模式里第一次 Ctrl-C 只该武装，不该退出复制模式")
	}
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("第一次 Ctrl-C 应当只交回到期 Cmd，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	m, cmd = tui1bStep(t, m, tui1bCtrl('c'))
	if !ccxIsQuit(cmd) {
		t.Error("第二次 Ctrl-C 应当退出程序（与 21e 的连按两次一致）")
	}
}

// TestTui2EnterWithoutMessages 是 §7 第 2 条：没有可复制内容时不进整屏。
//
// 两条路都钉：会话空 / 没有当前会话（repl 都返回 ErrCopyEmpty）与 Submitter
// 根本没有复制模式能力。loading 期间也不能提前把 AltScreen 打开。
func TestTui2EnterWithoutMessages(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		m := tui2New(t, &tui2Sub{err: repl.ErrCopyEmpty})
		m, cmd := tui1bStep(t, m, tui2CtrlG)
		if cmd == nil {
			t.Fatalf("Ctrl+G 应当仍然发出取数 Cmd")
		}
		if m.View().AltScreen {
			t.Fatalf("取数还没回来就开了整屏")
		}
		m, _ = tui1bStep(t, m, tui2Loaded(t, cmd))
		if m.copyScr.on {
			t.Error("没有可复制的消息时不该进整屏")
		}
		if v := m.View(); v.AltScreen || v.MouseMode != tea.MouseModeNone || v.OnMouse != nil {
			t.Errorf("没有可复制的消息时 View 必须原样：AltScreen=%v MouseMode=%v OnMouse nil=%v",
				v.AltScreen, v.MouseMode, v.OnMouse == nil)
		}
		atxWantHead(t, "没有可复制的消息", m, atxH(repl.MsgCopyNoContent))
	})

	t.Run("no_capability", func(t *testing.T) {
		m := tui2New(t, &tui1bSubmitter{})
		m, cmd := tui1bStep(t, m, tui2CtrlG)
		atxWantNilCmd(t, "没有复制模式能力时", cmd)
		atxWantHead(t, "没有复制模式能力时", m, atxH("复制模式不可用"))
		if m.View().AltScreen {
			t.Error("没有复制模式能力时不该开整屏")
		}
	})

	t.Run("busy", func(t *testing.T) {
		// 一轮在跑时 REPL 的输出会经 tea.Println 打进终端；在备用屏上插行会
		// 把整屏搅乱。这是实现自己加的门（任务书没写），这里把它钉住。
		m := tui2New(t, &tui2Sub{lines: tui2Fixture()})
		m.busy = true
		m, cmd := tui1bStep(t, m, tui2CtrlG)
		atxWantNilCmd(t, "忙时 Ctrl+G", cmd)
		if m.copyScr.on || m.copyScr.loading {
			t.Error("忙时不该发起复制模式取数")
		}
		atxWantHead(t, "忙时 Ctrl+G", m, atxH("一轮正在跑，等它回来再复制  0s"))
	})
}

// TestTui2SelectionChinese 是 §7 第 3 条：拖选的中文逐字一致，而且不是按字节切。
//
// 「中文」两个 rune、六个字节：若按字节下标切，拼出来的不是合法 UTF-8，
// 或者会多出半个字；顶栏的「已复制 N 个字符」也会变成 6。两个断言一起钉。
func TestTui2SelectionChinese(t *testing.T) {
	t.Run("cell_range", func(t *testing.T) {
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		// 正文第 1 行（终端 y=2）是「中文abc」：中占 0–1 格、文占 2–3 格。
		m, _ = tui2Mouse(t, m, tui2Click(0, 2))
		m, _ = tui2Mouse(t, m, tui2Motion(3, 2))
		if got := m.copySelectedText(); got != "中文" {
			t.Errorf("拖动中的选区文本 = %q，期望 %q（按字节切会得到别的字节串）", got, "中文")
		}
		if !utf8.ValidString(m.copySelectedText()) {
			t.Errorf("选区文本不是合法 UTF-8：%q", m.copySelectedText())
		}
		tui2WantTopHas(t, "拖动中", m, "已选中 2 个字符")

		m, cmd := tui2Mouse(t, m, tui2Release(3, 2))
		if cmd == nil {
			t.Errorf("松开左键应当交回写剪贴板的 Cmd（不执行）")
		}
		if got := m.copySelectedText(); got != "中文" {
			t.Errorf("松手之后选区文本 = %q，期望 %q", got, "中文")
		}
		// 6 个字节 vs 2 个 rune：hint 用 utf8.RuneCountInString，按字节算会写成 6。
		tui2WantTopHas(t, "松手之后", m, "已复制 2 个字符")
	})

	t.Run("start_inside_wide_rune", func(t *testing.T) {
		// x=1 是「中」的右半格。按 rune 下标直接切会切出半个字符；
		// 按显示宽度映射应当把整个「中」带上。
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		m, cmd := tui2Drag(t, m, 1, 2, 4, 2)
		if cmd == nil {
			t.Fatalf("松开左键应当交回写剪贴板的 Cmd")
		}
		if got, want := m.copySelectedText(), "中文a"; got != want {
			t.Errorf("从宽字符右半格开始拖，选区文本 = %q，期望 %q", got, want)
		}
	})
}

// TestTui2ReverseSelectionSame 是 §7 第 4 条：反向拖动结果与正向一致。
func TestTui2ReverseSelectionSame(t *testing.T) {
	const want = "文abc\n\n[alice]\n第二行"

	forward := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
	forward, fcmd := tui2Drag(t, forward, 2, 2, 5, 5)
	reverse := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
	reverse, rcmd := tui2Drag(t, reverse, 5, 5, 2, 2)

	if fcmd == nil || rcmd == nil {
		t.Fatalf("两次拖选都应当交回写剪贴板的 Cmd，正向 nil：%v，反向 nil：%v", fcmd == nil, rcmd == nil)
	}
	if got := forward.copySelectedText(); got != want {
		t.Errorf("正向拖选的选区文本 = %q，期望 %q", got, want)
	}
	if got := reverse.copySelectedText(); got != want {
		t.Errorf("反向拖选的选区文本 = %q，期望 %q（反向必须与正向逐字一致）", got, want)
	}
	if ft, rt := tui2Top(forward), tui2Top(reverse); ft != rt {
		t.Errorf("反向与正向的顶栏应当一致，正向 %q，反向 %q", ft, rt)
	}
}

// TestTui2SelectionClamp 是 §7 第 5 条：行尾之外的列夹到最后一个 rune，
// 正文区之外的 y 夹到首/末行。
func TestTui2SelectionClamp(t *testing.T) {
	t.Run("past_line_end", func(t *testing.T) {
		// 行 1 是「中文abc」。从 x=4（'a'）拖到 x=999（行尾之外，夹到最后一个
		// rune 'c'）应当取到 "abc"；零位移的单击已经不复制了（见空选区用例）。
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		m, cmd := tui2Drag(t, m, 4, 2, 999, 2)
		if cmd == nil {
			t.Fatalf("松开左键应当交回写剪贴板的 Cmd")
		}
		if got, want := m.copySelectedText(), "abc"; got != want {
			t.Errorf("行尾之外的列应当夹到最后一个 rune，选区文本 = %q，期望 %q", got, want)
		}
		tui2WantTopHas(t, "行尾之外", m, "已复制 3 个字符")
	})

	t.Run("outside_body", func(t *testing.T) {
		// y=0 是顶栏，应当夹到正文第一行；y=99 在页脚下方，夹到正文最后一行。
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		m, cmd := tui2Drag(t, m, 0, 0, 0, 99)
		if cmd == nil {
			t.Fatalf("松开左键应当交回写剪贴板的 Cmd")
		}
		want := "[你]\n中文abc\n\n[alice]\n第"
		if got := m.copySelectedText(); got != want {
			t.Errorf("正文区之外的 y 夹到首末行之后的选区文本 = %q，期望 %q", got, want)
		}
	})
}

// TestTui2ScrollClearsSelection 是 §7 第 6 条：PgUp/PgDn 与滚轮改变首行，
// 选区被清空（旧坐标已经指向别的文字，不清会复制错）。
func TestTui2ScrollClearsSelection(t *testing.T) {
	m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Many(30)}))
	// 30 条单行消息：30*2 + 29 个空行 = 89 行；一屏 22 行。
	tui2WantBottom(t, "初始", m, "第 1–22 行 / 共 89 行")

	m, _ = tui2Mouse(t, m, tui2Click(0, 1))
	m, _ = tui2Mouse(t, m, tui2Motion(2, 2))
	if !m.copyScr.sel {
		t.Fatalf("前提不成立：按下并拖动之后应当有选区")
	}

	m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.copyScr.sel {
		t.Error("PgDn 之后选区应当被清空")
	}
	if got := m.copyFirst(); got != 22 {
		t.Errorf("PgDn 之后首行应当是 22，实际 %d", got)
	}
	tui2WantBottom(t, "PgDn", m, "第 23–44 行 / 共 89 行")

	m, _ = tui2Mouse(t, m, tea.MouseWheelMsg{X: 0, Y: 10, Button: tea.MouseWheelUp})
	if got := m.copyFirst(); got != 21 {
		t.Errorf("滚轮向上之后首行应当是 21，实际 %d", got)
	}
	m, _ = tui2Mouse(t, m, tea.MouseWheelMsg{X: 0, Y: 10, Button: tea.MouseWheelDown})
	if got := m.copyFirst(); got != 22 {
		t.Errorf("滚轮向下之后首行应当是 22，实际 %d", got)
	}

	m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if got := m.copyFirst(); got != 0 {
		t.Errorf("PgUp 之后首行应当是 0，实际 %d", got)
	}
	m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if got := m.copyFirst(); got != 89-22 {
		t.Errorf("End 之后首行应当是 %d，实际 %d", 89-22, got)
	}
	tui2WantBottom(t, "End", m, "第 68–89 行 / 共 89 行")
	m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyHome})
	if got := m.copyFirst(); got != 0 {
		t.Errorf("Home 之后首行应当是 0，实际 %d", got)
	}
}

// TestTui2EmptySelectionNoCopy 是 §7 第 7 条：空选区松手不写剪贴板、不提示。
//
// 三种空选区：落在两条消息之间的空行上；没有按下就松开（drag 为 false）；
// 按下与松开落在同一行同一格（单击不拖，选区的两端都含，不特判会复制一个字符）。
func TestTui2EmptySelectionNoCopy(t *testing.T) {
	defaultTop := "复制模式  ↑↓/PgUp/PgDn 滚动  Esc 退出  拖动选择"

	t.Run("blank_line", func(t *testing.T) {
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		// 正文第 2 行（终端 y=3）是两条消息之间的空行。
		m, cmd := tui2Drag(t, m, 0, 3, 5, 3)
		atxWantNilCmd(t, "空行上的拖选", cmd)
		if m.copyStatusOn {
			t.Error("空选区不应当写复制提示")
		}
		tui2WantTop(t, "空行上的拖选", m, defaultTop)
	})

	t.Run("release_without_press", func(t *testing.T) {
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		m, cmd := tui2Mouse(t, m, tui2Release(0, 2))
		atxWantNilCmd(t, "没有按下就松开", cmd)
		if m.copyStatusOn {
			t.Error("没有选区时松开不应当写复制提示")
		}
		tui2WantTop(t, "没有按下就松开", m, defaultTop)
	})

	t.Run("click_no_drag", func(t *testing.T) {
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		m, _ = tui2Mouse(t, m, tui2Click(0, 2))
		m, cmd := tui2Mouse(t, m, tui2Release(0, 2))
		atxWantNilCmd(t, "同一格按下再松开", cmd)
		if m.copyStatusOn {
			t.Error("单击不拖不应当写复制提示")
		}
		if m.copyScr.sel {
			t.Error("零位移不应当留下选区")
		}
		tui2WantTop(t, "同一格按下再松开", m, defaultTop)
	})
}

// TestTui2StaysInCopyModeAfterCopy 是 §7 第 8 条：复制之后仍留在复制模式，
// 提示 2 秒到期后换回「已选中 N 个字符」，Esc 才退出。
func TestTui2StaysInCopyModeAfterCopy(t *testing.T) {
	m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
	m, cmd := tui2Drag(t, m, 0, 2, 3, 2)
	if cmd == nil {
		t.Fatalf("松开左键应当交回写剪贴板的 Cmd（不执行）")
	}
	if !m.copyScr.on {
		t.Fatalf("复制之后必须仍在复制模式")
	}
	v := m.View()
	if !v.AltScreen || v.MouseMode != tea.MouseModeCellMotion || v.OnMouse == nil {
		t.Errorf("复制之后整屏与鼠标捕获都应当还在：AltScreen=%v MouseMode=%v OnMouse nil=%v",
			v.AltScreen, v.MouseMode, v.OnMouse == nil)
	}

	// 复制提示到期之后回到「已选中」，人还在复制模式里。
	m, _ = tui1bStep(t, m, copyStatusExpireMsg{gen: m.copyStatusGen})
	tui2WantTopHas(t, "提示到期", m, "已选中 2 个字符")
	if !m.copyScr.on {
		t.Error("提示到期不应当退出复制模式")
	}

	m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.copyScr.on || m.View().AltScreen {
		t.Error("Esc 之后应当退出复制模式并关掉整屏")
	}
}

// TestTui2NonCopyViewKeepsMouseOff 是 §7 第 9 条（回归）：非复制模式下
// View 必须 AltScreen=false + MouseModeNone + OnMouse=nil。
//
// 这是「不抢终端原生选中」的硬保证：只要有一帧漏了 None，终端就不再自己管
// 选中与滚动了。覆盖没收到窗口尺寸、收过尺寸、忙、取数中四种帧，外加
// 进出复制模式一次之后的复位。
func TestTui2NonCopyViewKeepsMouseOff(t *testing.T) {
	fresh := New(Options{Submit: &tui1bSubmitter{}, Out: tui1bNopDrainer{}, Version: "0.0.0-test"})
	sized, _, _ := tui1bSized(t, nil)
	busy := sized
	busy.busy = true
	loading := tui2New(t, &tui2Sub{lines: tui2Fixture()})
	loading, cmd := tui1bStep(t, loading, tui2CtrlG)
	if cmd == nil || !loading.copyScr.loading {
		t.Fatalf("前提不成立：Ctrl+G 之后应当处在取数中")
	}

	for _, tc := range []struct {
		name string
		m    Model
	}{
		{"fresh", fresh},
		{"sized", sized},
		{"busy", busy},
		{"loading", loading},
	} {
		v := tc.m.View()
		if v.AltScreen {
			t.Errorf("%s：非复制模式 AltScreen 必须是 false", tc.name)
		}
		if v.MouseMode != tea.MouseModeNone {
			t.Errorf("%s：非复制模式 MouseMode 必须是 MouseModeNone，实际 %v", tc.name, v.MouseMode)
		}
		if v.OnMouse != nil {
			t.Errorf("%s：非复制模式 OnMouse 必须是 nil", tc.name)
		}
	}

	// 进出复制模式一次之后也必须复位：渲染器只比较相邻两帧，漏一帧就回不去了。
	m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
	m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	v := m.View()
	if v.AltScreen || v.MouseMode != tea.MouseModeNone || v.OnMouse != nil {
		t.Errorf("退出复制模式之后必须复位：AltScreen=%v MouseMode=%v OnMouse nil=%v",
			v.AltScreen, v.MouseMode, v.OnMouse == nil)
	}
}

// TestTui2CopyCommandScreenEntry 钉住 §3 的第二条入口：/copy --screen 与 Ctrl+G
// 走同一条取数路径，不能掉进 /copy 的 CopyRecent，也不能把输入框里的命令留到退出之后。
func TestTui2CopyCommandScreenEntry(t *testing.T) {
	sub := &tui2Sub{lines: tui2Fixture()}
	m := tui2New(t, sub)
	m = tui1bType(t, m, "/copy --screen")

	m, cmd := tui1bStep(t, m, tui1bEnter())
	if cmd == nil {
		t.Fatalf("/copy --screen 回车应当交回取数 Cmd")
	}
	if m.copyScr.on {
		t.Fatalf("取数还没回来就进了整屏")
	}
	atxWantValue(t, "/copy --screen 回车之后输入框", m, "")

	m, _ = tui1bStep(t, m, tui2Loaded(t, cmd))
	if !m.copyScr.on {
		t.Fatalf("/copy --screen 的取数成功之后应当进入复制模式")
	}
	if sub.screenN != 1 || sub.recentN != 0 {
		t.Errorf("/copy --screen 应当走 CopyScreen 1 次、不走 CopyRecent，实际 %d 次 / %d 次",
			sub.screenN, sub.recentN)
	}
	if !m.View().AltScreen {
		t.Error("/copy --screen 进入之后应当是整屏")
	}
}

// TestTui2TruncatedNote 钉住 §6 的上限说明：只装了最近 CopyScreenMax 条时，
// 顶栏要说出来，不能假装整场都在。
func TestTui2TruncatedNote(t *testing.T) {
	m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture(), truncated: true}))
	want := fmt.Sprintf("只加载了最近 %d 条", repl.CopyScreenMax)
	tui2WantTopHas(t, "截断", m, want)
}

// TestTui2EnterCancelledWhileLoading 钉住取数窗口里的两条边界：
//
//   - Esc 取消这次进入，迟到的结果不能再把用户拽进整屏；
//   - Enter 被挡住。取数 Cmd 与 Submit 若并发跑，会同时读同一个 REPL，
//     而 REPL 的命令本来是靠事件循环串行派发的，没有为并发准备。
func TestTui2EnterCancelledWhileLoading(t *testing.T) {
	sub := &tui2Sub{lines: tui2Fixture()}
	m := tui2New(t, sub)
	m, cmd := tui1bStep(t, m, tui2CtrlG)
	if cmd == nil || !m.copyScr.loading {
		t.Fatalf("前提不成立：Ctrl+G 之后应当处在取数中")
	}

	m, enterCmd := tui1bStep(t, m, tui1bEnter())
	atxWantNilCmd(t, "取数期间的 Enter", enterCmd)
	if n := sub.count(); n != 0 {
		t.Errorf("取数期间的 Enter 不应当提交，实际 Submit %d 次", n)
	}

	m, escCmd := tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	atxWantNilCmd(t, "取数期间的 Esc", escCmd)
	if m.copyScr.loading {
		t.Error("取数期间的 Esc 应当取消这次进入")
	}

	// 取消之后迟到的结果：丢掉，不进整屏。
	m, _ = tui1bStep(t, m, tui2Loaded(t, cmd))
	if m.copyScr.on || m.View().AltScreen {
		t.Error("已经取消的取数结果不应当再把界面拽进整屏")
	}
}

// TestTui2ClickNoDragNoCopy 钉住复审第 1 条：零位移不是选区。
//
// 选区的两端都是含的，不特判时按下与松开在同一格会复制一个字符。
// 单击不复制；拖出去再拖回原点松开同样不复制（零位移按位置判断，不看途中）。
func TestTui2ClickNoDragNoCopy(t *testing.T) {
	defaultTop := "复制模式  ↑↓/PgUp/PgDn 滚动  Esc 退出  拖动选择"

	t.Run("click", func(t *testing.T) {
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		m, _ = tui2Mouse(t, m, tui2Click(0, 2))
		m, cmd := tui2Mouse(t, m, tui2Release(0, 2))
		atxWantNilCmd(t, "同一格按下再松开", cmd)
		if m.copyStatusOn {
			t.Error("单击不拖不应当写复制提示")
		}
		if m.copyScr.sel {
			t.Error("零位移不应当留下选区")
		}
		tui2WantTop(t, "同一格按下再松开", m, defaultTop)
	})

	t.Run("drag_back", func(t *testing.T) {
		m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
		m, _ = tui2Mouse(t, m, tui2Click(0, 2))
		m, _ = tui2Mouse(t, m, tui2Motion(3, 2))
		if got := m.copySelectedText(); got != "中文" {
			t.Fatalf("前提不成立：拖到第 3 格时选区应当是 %q，实际 %q", "中文", got)
		}
		m, cmd := tui2Mouse(t, m, tui2Release(0, 2))
		atxWantNilCmd(t, "拖回原点再松开", cmd)
		if m.copyStatusOn {
			t.Error("拖回原点等于零位移，不应当写复制提示")
		}
		if m.copyScr.sel {
			t.Error("零位移不应当留下选区")
		}
		tui2WantTop(t, "拖回原点再松开", m, defaultTop)
	})
}

// TestTui2LongLineDragToRightEdge 钉住复审第 2 条：拖到可见区最右列按行尾算。
//
// 200 个 a 的行被显示截断到 80 列；鼠标 x 最大只能到 79，不按行尾算就只复制
// 前 80 个字符。终端宽 80（tui2New 的 WindowSizeMsg）。
func TestTui2LongLineDragToRightEdge(t *testing.T) {
	long := strings.Repeat("a", 200)
	m := tui2Enter(t, tui2New(t, &tui2Sub{lines: []repl.CopyLine{{User: true, Text: long}}}))
	m, cmd := tui2Drag(t, m, 0, 2, 79, 2)
	if cmd == nil {
		t.Fatalf("松开左键应当交回写剪贴板的 Cmd")
	}
	if got := m.copySelectedText(); got != long {
		t.Errorf("拖到右缘应当复制到行尾（%d 个 a），实际 %d 个字符：%q", len(long), len([]rune(got)), got)
	}
	tui2WantTopHas(t, "长行", m, "已复制 200 个字符")
}

// TestTui2CtrlCFeedbackInCopyMode 钉住复审第 3 条：复制模式没有普通状态栏，
// 第一次 Ctrl-C 的「再按一次 Ctrl-C 退出」必须出现在顶栏；第二次才退出；
// 武装到期之后顶栏回到复制模式自己的提示。
func TestTui2CtrlCFeedbackInCopyMode(t *testing.T) {
	defaultTop := "复制模式  ↑↓/PgUp/PgDn 滚动  Esc 退出  拖动选择"

	m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
	tui2WantTop(t, "刚进复制模式", m, defaultTop)

	m, cmd := tui1bStep(t, m, tui1bCtrl('c'))
	if cmd == nil || ccxIsQuit(cmd) {
		t.Fatalf("第一次 Ctrl-C 应当只交回到期 Cmd，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	tui2WantTopHas(t, "第一次 Ctrl-C", m, "再按一次 Ctrl-C 退出")
	if !m.copyScr.on {
		t.Error("第一次 Ctrl-C 不应当退出复制模式")
	}

	m, cmd = tui1bStep(t, m, tui1bCtrl('c'))
	if !ccxIsQuit(cmd) {
		t.Error("第二次 Ctrl-C 应当退出程序（与 21e 的连按两次一致）")
	}
	tui2WantTopHas(t, "第二次 Ctrl-C", m, "正在退出")

	// 不按第二次、让武装到期：顶栏回到「拖动选择」，不留一句作废的提示。
	m2 := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
	m2, _ = tui1bStep(t, m2, tui1bCtrl('c'))
	m2, _ = tui1bStep(t, m2, ctrlCExpireMsg{gen: m2.ctrlCGen})
	tui2WantTop(t, "武装到期", m2, defaultTop)
}

// TestTui2LoadingEscDisarms 钉住复审第 4 条：取数中按过 Ctrl-C 再按 Esc 取消，
// 必须连 Ctrl-C 的武装一起解除，回到进屏前的干净状态。
func TestTui2LoadingEscDisarms(t *testing.T) {
	m := tui2New(t, &tui2Sub{lines: tui2Fixture()})
	m, cmd := tui1bStep(t, m, tui2CtrlG)
	if cmd == nil || !m.copyScr.loading {
		t.Fatalf("前提不成立：Ctrl+G 之后应当处在取数中")
	}

	m, _ = tui1bStep(t, m, tui1bCtrl('c'))
	if !m.ctrlCArmed {
		t.Fatalf("前提不成立：取数中第一次 Ctrl-C 应当武装")
	}
	atxWantHead(t, "取数中武装", m, atxH("再按一次 Ctrl-C 退出"))

	m, escCmd := tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	atxWantNilCmd(t, "取数中取消", escCmd)
	if m.copyScr.loading {
		t.Error("取数中的 Esc 应当取消这次进入")
	}
	if m.ctrlCArmed {
		t.Error("取消之后不应当还武装着 Ctrl-C：退出后 2 秒内再按一次会直接退出")
	}
	atxWantHead(t, "取消之后", m, atxH("空闲"))

	// 取消之后迟到的取数结果仍然丢掉。
	m, _ = tui1bStep(t, m, tui2Loaded(t, cmd))
	if m.copyScr.on {
		t.Error("已经取消的取数结果不应当再进整屏")
	}
}

// TestTui2RightReleaseNoCopy 钉住复审第 5 条：松开事件只认左键（与 X10 的
// MouseNone），右键松开不能结束左键拖选、更不能顺手复制。
func TestTui2RightReleaseNoCopy(t *testing.T) {
	defaultTop := "复制模式  ↑↓/PgUp/PgDn 滚动  Esc 退出  拖动选择"

	m := tui2Enter(t, tui2New(t, &tui2Sub{lines: tui2Fixture()}))
	m, _ = tui2Mouse(t, m, tui2Click(0, 2))
	m, cmd := tui2Mouse(t, m, tea.MouseReleaseMsg{X: 3, Y: 2, Button: tea.MouseRight})
	atxWantNilCmd(t, "左键按下之后的右键松开", cmd)
	if m.copyStatusOn {
		t.Error("右键松开不应当写复制提示")
	}
	if !m.copyScr.drag {
		t.Error("右键松开不应当结束左键拖选")
	}
	tui2WantTop(t, "右键松开", m, defaultTop)

	// 同装置对照：换成左键松开才复制。
	m, cmd = tui2Mouse(t, m, tui2Release(3, 2))
	if cmd == nil {
		t.Fatalf("左键松开应当交回写剪贴板的 Cmd")
	}
	if got := m.copySelectedText(); got != "中文" {
		t.Errorf("左键松开之后的选区文本 = %q，期望 %q", got, "中文")
	}
}
