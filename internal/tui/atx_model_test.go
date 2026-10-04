package tui

// 本文件覆盖 AT1 冻结契约（E:\multi_model_cowork\at1_contract.md，v1.1 + W1b 修订）§2.2 的 M1-M15：
// 输入框敲 @ 弹出成员补全列表，纯 Model 驱动，不起 Program。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3d134ee 的源码。
// 本文件引用本刀新增的 atLines、submitDoneMsg.members、repl.MentionCandidate、mentionSource，
// 在 HEAD 上整包编译不过，这是契约 §2.0 写明的预期红。
//
// 纪律（契约 §2.0）：包级标识符一律以 atx 开头；只读复用 tui1b / tui1d 的辅助；
// 不读 m.at、m.members（内部状态，一律用行为断言）；不引用 atMaxRows 与提示行的格式串，
// 期望值是本文件自己写的字面量（契约 §2.0 的 ALL、B3 与 §2.2 各条逐字照抄）。
// 「值」= m.ta.Value()；「行」= m.atLines()；「帧」= m.frameHeight()；「光标」= m.View().Cursor；「表头」= m.chrome()[0]。

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"council/internal/repl"
)

// ─── 公用夹具（契约 §2.0，冻结） ───────────────────────────────────────

// atxMembers 是 8 个候选，已按 id 排序。
var atxMembers = []repl.MentionCandidate{
	{ID: "alpha", Name: "Alpha"},
	{ID: "b1", Name: ""},
	{ID: "beta", Name: "贝塔"},
	{ID: "bravo", Name: "Bravo"},
	{ID: "c1", Name: "C1"},
	{ID: "c2", Name: ""},
	{ID: "c3", Name: ""},
	{ID: "grok-4.7", Name: "Grok 4.7"},
}

// atxALL 是 @ 之后、80x24 的整列（6 行；id 列宽 8 = grok-4.7）。
var atxALL = []string{
	"> @alpha     Alpha",
	"  @b1",
	"  @beta      贝塔",
	"  @bravo     Bravo",
	"  @c1        C1",
	"  共 8 个，上下键翻看",
}

// atxB3 是 @b 之后的整列（id 列宽 5）。
var atxB3 = []string{
	"> @b1",
	"  @beta   贝塔",
	"  @bravo  Bravo",
}

// atxCands 返回候选的一份拷贝，免得用例之间共享底层数组。
func atxCands(list []repl.MentionCandidate) []repl.MentionCandidate {
	if list == nil {
		return nil
	}
	return append([]repl.MentionCandidate{}, list...)
}

// atxH 是表头 H(s)。
func atxH(s string) string { return "council 0.0.0-test   " + s }

// atxNew 是「新 Model(w, h, list)」：tui1bNew → WindowSizeMsg{w, h} → submitDoneMsg{members: list}。
func atxNew(t *testing.T, w, h int, list []repl.MentionCandidate) (Model, *tui1bSubmitter) {
	t.Helper()
	m, sub, _ := tui1bNew(t, nil)
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m, _ = tui1bStep(t, m, submitDoneMsg{members: list})
	return m, sub
}

// atxCode 是只带 Code 的键（方向键、Tab、退格、Delete 等）。
func atxCode(c rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c} }

// atxSteps 依次喂若干消息，丢掉 Cmd（textarea 的光标闪烁 Cmd 与本刀无关，理由同 tui1bType）。
func atxSteps(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		m, _ = tui1bStep(t, m, msg)
	}
	return m
}

func atxWantLines(t *testing.T, where string, m Model, want []string) {
	t.Helper()
	got := m.atLines()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s：行应当是 %q（nil=%v），实际 %q（nil=%v）", where, want, want == nil, got, got == nil)
	}
}

func atxWantValue(t *testing.T, where string, m Model, want string) {
	t.Helper()
	if got := m.ta.Value(); got != want {
		t.Errorf("%s：值应当是 %q，实际 %q", where, want, got)
	}
}

func atxWantFrame(t *testing.T, where string, m Model, want int) {
	t.Helper()
	if got := m.frameHeight(); got != want {
		t.Errorf("%s：帧高应当是 %d，实际 %d", where, want, got)
	}
}

func atxWantHead(t *testing.T, where string, m Model, want string) {
	t.Helper()
	if got := m.chrome()[0]; got != want {
		t.Errorf("%s：表头应当是 %q，实际 %q", where, want, got)
	}
}

func atxWantCursorNil(t *testing.T, where string, m Model) {
	t.Helper()
	if c := m.View().Cursor; c != nil {
		t.Errorf("%s：光标应当是 nil，实际 %+v", where, *c)
	}
}

// atxWantCursor 断言钉住的光标：位置 (x, y)、竖线、闪烁、颜色 nil。
func atxWantCursor(t *testing.T, where string, m Model, x, y int) {
	t.Helper()
	c := m.View().Cursor
	if c == nil {
		t.Errorf("%s：光标应当钉在 (%d, %d)，实际 nil", where, x, y)
		return
	}
	if c.X != x || c.Y != y || c.Shape != tea.CursorBar || !c.Blink || c.Color != nil {
		t.Errorf("%s：光标应当是 (%d, %d)、CursorBar、闪烁、颜色 nil，实际 (%d, %d)、形状 %v、闪烁 %v、颜色 %v",
			where, x, y, c.X, c.Y, c.Shape, c.Blink, c.Color)
	}
}

func atxWantNilCmd(t *testing.T, where string, cmd tea.Cmd) {
	t.Helper()
	if cmd != nil {
		t.Errorf("%s：Cmd 应当是 nil，实际非 nil", where)
	}
}

// atxNoSubmitDone 报告一条 Cmd 跑到底之后的产出里没有 submitDoneMsg。
func atxNoSubmitDone(tr tui1bTrace) bool {
	for _, l := range tr.log {
		if l == "submitDone" {
			return false
		}
	}
	return true
}

// ═══ M1 ═══════════════════════════════════════════════════════════════

// TestAtxOpenShowsList 是 M1：敲 @ 弹出整列，放在输入框下方，钉住竖线光标。
//
// 【判别力】前提（敲 @ 之前行 nil、帧 8、光标 nil）是同装置的缺席对照。
// A1（不钉光标）→ 光标 nil；A2（Y 不加 chrome）→ Y 0；A3（方块）→ 形状；A4（放上方）→ View 第 9 行起不是整列；
// A5（帧高不数列表）→ 帧 8；A21 / A23 / A25 / A46 → 行的整列不等。
func TestAtxOpenShowsList(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	atxWantLines(t, "前提（敲 @ 之前）", m, nil)
	atxWantFrame(t, "前提（敲 @ 之前）", m, 8)
	atxWantCursorNil(t, "前提（敲 @ 之前）", m)

	m = tui1bType(t, m, "@")
	atxWantLines(t, "敲 @", m, atxALL)

	rows := strings.Split(m.View().Content, "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(tui1bStripANSI(rows[i]), " ")
	}
	var below []string
	if len(rows) > 8 {
		below = rows[8:]
	}
	if !reflect.DeepEqual(below, atxALL) {
		t.Errorf("敲 @：View 第 9 行起（下标 8 起）应当整列是 %q，实际 %q；全部 %d 行：%q", atxALL, below, len(rows), rows)
	}
	atxWantFrame(t, "敲 @", m, 14)
	atxWantCursor(t, "敲 @", m, 7, 2)
	atxWantHead(t, "敲 @", m, atxH("空闲"))
	atxWantValue(t, "敲 @", m, "@")
}

// ═══ M2 ═══════════════════════════════════════════════════════════════

// TestAtxLeftBoundary 是 M2：@ 左边那个字决定开不开，规则与 ParseMentions 的左边界同一条。
//
// 【判别力】开与不开同一装置互为对照（只差前置键）。A12（左边界不查）→ 七个不开的全红；
// C6（非 ASCII 按低字节判）→ han 红（中 的低字节 0x2d 是 -）。
func TestAtxLeftBoundary(t *testing.T) {
	ctrlJ := tui1bCtrl('j')
	cases := []struct {
		name string
		pre  []tea.Msg
		open bool
	}{
		{"start", nil, true},
		{"space", []tea.Msg{tui1bKey(' ')}, true},
		{"paren", []tea.Msg{tui1bKey('(')}, true},
		{"han", []tea.Msg{tui1bKey('中')}, true},
		{"fullcomma", []tea.Msg{tui1bKey(rune(0xff0c))}, true},
		{"newline", []tea.Msg{tui1bKey('x'), ctrlJ}, true},
		{"letter", []tea.Msg{tui1bKey('a')}, false},
		{"upper", []tea.Msg{tui1bKey('Z')}, false},
		{"digit", []tea.Msg{tui1bKey('9')}, false},
		{"under", []tea.Msg{tui1bKey('_')}, false},
		{"dot", []tea.Msg{tui1bKey('.')}, false},
		{"dash", []tea.Msg{tui1bKey('-')}, false},
		{"at", []tea.Msg{tui1bKey('@')}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
			m = atxSteps(t, m, tc.pre...)
			m = tui1bType(t, m, "@")
			if tc.open {
				atxWantLines(t, tc.name+" 之后敲 @", m, atxALL)
			} else {
				atxWantLines(t, tc.name+" 之后敲 @", m, nil)
				atxWantFrame(t, tc.name+" 之后敲 @", m, 8)
			}
		})
	}
}

// ═══ M3 ═══════════════════════════════════════════════════════════════

// TestAtxFilter 是 M3：按 id 前缀筛，大小写不敏感。
//
// 【判别力】`@a` 钉住「前缀」不是「包含」（A44：beta、bravo 也含 a）；`R`、`GROK` 钉住小写折叠（A11）；
// x 之后的缺席与退格之后的出现互为对照（补全中没丢，只是筛空不显示）；A17（default 分支不重判）红在第一步。
// 第一步（敲 @b → B3、帧 11）同时是 M9「只喂粘贴」与 M7「→ → →」的正对照。
func TestAtxFilter(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@b")
	atxWantLines(t, "敲 @b", m, atxB3)
	atxWantFrame(t, "敲 @b", m, 11)

	m = tui1bType(t, m, "x")
	atxWantLines(t, "再敲 x", m, nil)
	atxWantFrame(t, "再敲 x", m, 8)
	atxWantCursorNil(t, "再敲 x", m)

	m = atxSteps(t, m, atxCode(tea.KeyBackspace))
	atxWantLines(t, "退格", m, atxB3)

	m = tui1bType(t, m, "R")
	atxWantLines(t, "敲 R", m, []string{"> @bravo  Bravo"})

	m2, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m2 = tui1bType(t, m2, "@GROK-4.")
	atxWantLines(t, "另起敲 @GROK-4.", m2, []string{"> @grok-4.7  Grok 4.7"})

	m3, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m3 = tui1bType(t, m3, "@a")
	atxWantLines(t, "另起敲 @a", m3, []string{"> @alpha  Alpha"})

	m4, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m4 = tui1bType(t, m4, "@c")
	atxWantLines(t, "另起敲 @c", m4, []string{"> @c1  C1", "  @c2", "  @c3"})
}

// ═══ M4 ═══════════════════════════════════════════════════════════════

// TestAtxUpDown 是 M4：上下键移动选中项，到头不回绕，窗口跟着选中项走。
//
// 【判别力】↓ 到底再 ↑ 一次钉住「到底不越界」（A9：越界之后显示被夹住看不出，↑ 一次才露馅）；
// A8（到顶还减）→ 第一次 ↑ 之后不是 ALL 或之后的整列错位；A10（前缀变了不归零）→ 敲 b 之后；
// A24（窗口不跟）、A43（↑ 写成加）、A47（标记跟 top）红在对应那一步。
// 对照（列表不在时上下键归 textarea）：a Ctrl-J b ↑ z → "az\nb"。
// Delete 那段与「↓ ↓ 之后敲 b」互为对照：前缀没变时选中项留在 beta，前缀变了归零；A50 让 Delete 之后变成 ALL。
func TestAtxUpDown(t *testing.T) {
	up, down := atxCode(tea.KeyUp), atxCode(tea.KeyDown)
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@")

	var cmd tea.Cmd
	m, cmd = tui1bStep(t, m, up)
	atxWantNilCmd(t, "第一次 ↑", cmd)
	atxWantLines(t, "第一次 ↑（到顶不动）", m, atxALL)

	for i := 0; i < 5; i++ {
		m, cmd = tui1bStep(t, m, down)
		atxWantNilCmd(t, fmt.Sprintf("↓ 第 %d 次", i+1), cmd)
	}
	atxWantLines(t, "↓ 五次", m, []string{
		"  @b1",
		"  @beta      贝塔",
		"  @bravo     Bravo",
		"  @c1        C1",
		"> @c2",
		"  共 8 个，上下键翻看",
	})

	m = atxSteps(t, m, down, down, down, down, down)
	atxWantLines(t, "再 ↓ 五次", m, []string{
		"  @bravo     Bravo",
		"  @c1        C1",
		"  @c2",
		"  @c3",
		"> @grok-4.7  Grok 4.7",
		"  共 8 个，上下键翻看",
	})

	m = atxSteps(t, m, up)
	atxWantLines(t, "↑ 一次", m, []string{
		"  @beta      贝塔",
		"  @bravo     Bravo",
		"  @c1        C1",
		"  @c2",
		"> @c3",
		"  共 8 个，上下键翻看",
	})

	m = atxSteps(t, m, down)
	for i := 0; i < 10; i++ {
		m = atxSteps(t, m, up)
	}
	atxWantLines(t, "↓ 一次再 ↑ 十次", m, atxALL)
	atxWantValue(t, "上下键之后", m, "@")

	m = atxSteps(t, m, down, down)
	m = tui1bType(t, m, "b")
	atxWantLines(t, "↓ ↓ 之后敲 b（前缀变了选中项归零）", m, atxB3)

	// 对照：列表不在时上下键归 textarea。
	c, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	c = tui1bType(t, c, "a")
	c = atxSteps(t, c, tui1bCtrl('j'))
	c = tui1bType(t, c, "b")
	c = atxSteps(t, c, up)
	c = tui1bType(t, c, "z")
	atxWantValue(t, "对照（a Ctrl-J b ↑ z）", c, "az\nb")

	// 前缀没变、选中项保留（v1.1）。
	d, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	d = tui1bType(t, d, " y")
	d = atxSteps(t, d, atxCode(tea.KeyLeft), atxCode(tea.KeyLeft))
	d = tui1bType(t, d, "@")
	d = atxSteps(t, d, down, down)
	selBeta := []string{
		"  @alpha     Alpha",
		"  @b1",
		"> @beta      贝塔",
		"  @bravo     Bravo",
		"  @c1        C1",
		"  共 8 个，上下键翻看",
	}
	atxWantValue(t, "前提（行首敲 @、↓ ↓）", d, "@ y")
	atxWantLines(t, "前提（行首敲 @、↓ ↓）", d, selBeta)
	d = atxSteps(t, d, atxCode(tea.KeyDelete))
	atxWantValue(t, "Delete 之后", d, "@y")
	atxWantLines(t, "Delete 之后（前缀没变，选中项还在 beta）", d, selBeta)
}

// ═══ M5 ═══════════════════════════════════════════════════════════════

// TestAtxAccept 是 M5：Tab 与回车用选中项补全，回车不提交。
//
// 【判别力】第 1 步「没提交」是缺席断言，对照是第 4 步同装置第二次回车真的提交（先跑 Cmd 再读计数）。
// 「再打一个 z」钉住光标落点（A29 少空格、A28 少删 @、A30 不收起）；A37（回车不截）让 enter 子步骤红、
// A38（Tab 不截）让 tab 子步骤红；A11（不折小写）让第 3 步红。
func TestAtxAccept(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"tab", atxCode(tea.KeyTab)},
		{"enter", tui1bEnter()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, sub := atxNew(t, 80, 24, atxCands(atxMembers))
			m = tui1bType(t, m, "hi @br")
			var cmd tea.Cmd
			m, cmd = tui1bStep(t, m, tc.key)
			tr := tui1bRun(t, cmd)
			if !atxNoSubmitDone(tr) {
				t.Errorf("%s 补全：产出里不应当有 submitDone，实际 %v", tc.name, tr.log)
			}
			if n := sub.count(); n != 0 {
				t.Errorf("%s 补全：Submit 应当 0 次，实际 %d 次（第一次收到 %q）", tc.name, n, sub.text(0))
			}
			atxWantValue(t, tc.name+" 补全", m, "hi @bravo ")
			atxWantLines(t, tc.name+" 补全", m, nil)
			atxWantFrame(t, tc.name+" 补全", m, 8)
			atxWantCursorNil(t, tc.name+" 补全", m)
			atxWantHead(t, tc.name+" 补全", m, atxH("空闲"))
			m = tui1bType(t, m, "z")
			atxWantValue(t, tc.name+" 补全之后敲 z", m, "hi @bravo z")
		})
	}

	t.Run("middle", func(t *testing.T) {
		m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
		m = tui1bType(t, m, "x y")
		m = atxSteps(t, m, atxCode(tea.KeyLeft), atxCode(tea.KeyLeft))
		m = tui1bType(t, m, " @c")
		m = atxSteps(t, m, atxCode(tea.KeyDown), atxCode(tea.KeyTab))
		m = tui1bType(t, m, "z")
		atxWantValue(t, "中间补全", m, "x @c2 z y")
	})

	t.Run("multiline", func(t *testing.T) {
		m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
		m = tui1bType(t, m, "hi ")
		m = atxSteps(t, m, tui1bCtrl('j'))
		m = tui1bType(t, m, "x @GR")
		c := m.View().Cursor
		if c == nil || c.X != 11 || c.Y != 3 {
			t.Errorf("多行 @GR：光标应当是 (11, 3)，实际 %+v", c)
		}
		m = atxSteps(t, m, tui1bEnter())
		m = tui1bType(t, m, "z")
		atxWantValue(t, "多行补全之后敲 z", m, "hi \nx @grok-4.7 z")
	})

	t.Run("control_submit", func(t *testing.T) {
		m, sub := atxNew(t, 80, 24, atxCands(atxMembers))
		m = tui1bType(t, m, "@b")
		m = atxSteps(t, m, atxCode(tea.KeyDown), tui1bEnter())
		atxWantValue(t, "对照：@b ↓ 回车", m, "@beta ")
		var cmd tea.Cmd
		_, cmd = tui1bStep(t, m, tui1bEnter())
		tui1bRun(t, cmd)
		if n := sub.count(); n != 1 || sub.text(0) != "@beta " {
			t.Errorf("对照：第二次回车应当提交 1 次、收到 %q，实际 %d 次、%q", "@beta ", n, sub.text(0))
		}
	})
}

// ═══ M6 ═══════════════════════════════════════════════════════════════

// TestAtxEsc 是 M6：列表开着时 Esc 只收起，不改状态栏；收起后只有新敲的 @ 才重开。
//
// 【判别力】A7（列表开着时 Esc 不截）→ 第一步行不为 nil；提示那段是「列表开着时 Esc 不改状态栏」的正面钉。
// 「敲 r → nil」与「敲 ` @` → ALL」同装置互为对照。
func TestAtxEsc(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@b")
	var cmd tea.Cmd
	m, cmd = tui1bStep(t, m, tui1bEsc())
	atxWantNilCmd(t, "Esc", cmd)
	atxWantLines(t, "Esc", m, nil)
	atxWantValue(t, "Esc", m, "@b")
	atxWantCursorNil(t, "Esc", m)

	m = tui1bType(t, m, "r")
	atxWantLines(t, "Esc 之后敲 r（不重开）", m, nil)
	m = tui1bType(t, m, " @")
	atxWantLines(t, "再敲 ` @`（新的 @ 重开）", m, atxALL)

	hint := atxH("一次只能一条命令，逐条回车；整段当消息发，首行只写 /say")
	x, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	x = tui1bType(t, x, "a")
	x = atxSteps(t, x, tui1bCtrl('j'))
	x = tui1bType(t, x, "/b")
	x = atxSteps(t, x, tui1bEnter())
	atxWantHead(t, "X44 挡下", x, hint)
	x = tui1bType(t, x, " @")
	if x.atLines() == nil {
		t.Fatalf("前提不成立：X44 提示挂着时敲 ` @`，列表应当出现")
	}
	x = atxSteps(t, x, tui1bEsc())
	atxWantHead(t, "X44 提示挂着时 Esc", x, hint)
	atxWantValue(t, "X44 提示挂着时 Esc", x, "a\n/b @")
	atxWantLines(t, "X44 提示挂着时 Esc", x, nil)
}

// ═══ M7 ═══════════════════════════════════════════════════════════════

// TestAtxCloseOnContextLoss 是 M7：光标离开 mention、打了非 token 字符、换行、选区时收起，收起后不因光标回来而重开。
//
// 【判别力】A18（不查字符类）→ 空格那段退格之后重开；A19（不查行）→ 换行那段 ↑ 之后出现 ["> @alpha  Alpha"]；
// A20（不查选区）→ 选区那段出现 ALL；A16（Ctrl-J 不重判）→ Ctrl-J 那段红；A40（<= 写成 <）→ 第二次 ← panic。
// 「→ → → 行 nil」的杀手是 A51（光标一落到 @ 后面就开，变成 B3）；它同装置的正对照是 M3 第一步
// （敲 @b → B3），值与光标都相同，只差来路。「退格两次 → 行 nil」的对照是只退格一次（ALL）。
func TestAtxCloseOnContextLoss(t *testing.T) {
	left, right := atxCode(tea.KeyLeft), atxCode(tea.KeyRight)

	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@b")
	m = atxSteps(t, m, left)
	atxWantLines(t, "@b ←（光标在 @ 与 b 之间，前缀空）", m, atxALL)
	m = atxSteps(t, m, left)
	atxWantLines(t, "再 ←（光标到 @ 前面）", m, nil)
	m = atxSteps(t, m, right, right, right)
	atxWantLines(t, "→ → →（不重开）", m, nil)
	atxWantValue(t, "→ → →", m, "@b")
	if l, c := m.ta.Line(), m.ta.Column(); l != 0 || c != 2 {
		t.Errorf("→ → →：光标应当在 (行 0, 列 2)，实际 (行 %d, 列 %d)", l, c)
	}

	s, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	s = tui1bType(t, s, "@b ")
	atxWantLines(t, "@b 空格", s, nil)
	s = atxSteps(t, s, atxCode(tea.KeyBackspace))
	atxWantLines(t, "@b 空格 退格（不回来）", s, nil)
	atxWantValue(t, "@b 空格 退格", s, "@b")

	b, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	b = tui1bType(t, b, "@b")
	b = atxSteps(t, b, atxCode(tea.KeyBackspace))
	atxWantLines(t, "@b 退格一次（对照）", b, atxALL)
	atxWantValue(t, "@b 退格一次（对照）", b, "@")
	b = atxSteps(t, b, atxCode(tea.KeyBackspace))
	atxWantLines(t, "@b 退格两次", b, nil)
	atxWantValue(t, "@b 退格两次", b, "")

	j, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	j = tui1bType(t, j, "@b")
	j = atxSteps(t, j, tui1bCtrl('j'))
	atxWantLines(t, "@b Ctrl-J", j, nil)
	j = atxSteps(t, j, atxCode(tea.KeyUp))
	atxWantLines(t, "@b Ctrl-J ↑", j, nil)

	n, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	n = tui1bType(t, n, "@alp")
	n = atxSteps(t, n, tui1bEsc(), tui1bCtrl('j'))
	n = tui1bType(t, n, "@zz")
	n = atxSteps(t, n, atxCode(tea.KeyUp))
	atxWantLines(t, "换行那段 ↑", n, nil)
	n = tui1bType(t, n, "z")
	atxWantValue(t, "换行那段 ↑ 之后敲 z（↑ 走了 textarea）", n, "@alzp\n@zz")

	sel, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	sel = tui1bType(t, sel, "@b")
	sel = atxSteps(t, sel, tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
	atxWantLines(t, "选区（Shift+←）", sel, nil)
}

// ═══ M8 ═══════════════════════════════════════════════════════════════

// TestAtxCtrlC 是 M8：列表开着时第一次 Ctrl-C 只收起；列表收起后再连按两次才退出。
//
// 【判别力】列表不在时的第一次只武装、第二次才退出；A6 让列表开着的那次就掉进空闲分支，只武装（Cmd 是到期 tick、表头「再按一次 Ctrl-C 退出」）。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
func TestAtxCtrlC(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@")
	var cmd tea.Cmd
	m, cmd = tui1bStep(t, m, tui1bCtrl('c'))
	atxWantNilCmd(t, "第一次 Ctrl-C", cmd)
	atxWantLines(t, "第一次 Ctrl-C", m, nil)
	atxWantHead(t, "第一次 Ctrl-C", m, atxH("空闲"))
	atxWantValue(t, "第一次 Ctrl-C", m, "@")

	m, cmd = tui1bStep(t, m, tui1bCtrl('c'))
	if cmd == nil || reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
		t.Errorf("第二次 Ctrl-C：应当只武装、不退出（cmd == nil：%v）", cmd == nil)
	}
	atxWantHead(t, "第二次 Ctrl-C", m, atxH("再按一次 Ctrl-C 退出"))
	m, cmd = tui1bStep(t, m, tui1bCtrl('c'))
	if tr := tui1bRun(t, cmd); !tr.hasQuit() {
		t.Errorf("第三次 Ctrl-C：产出里应当有 QuitMsg，实际 %v", tr.log)
	}
	atxWantHead(t, "第三次 Ctrl-C", m, atxH("正在退出"))
}

// ═══ M9 ═══════════════════════════════════════════════════════════════

// TestAtxPasteCloses 是 M9：粘贴一律收起；粘贴进来的 @ 不开。
//
// 【判别力】A15（粘贴不收起）→ 第一步行 == ALL（前缀没重算）。
// 「只喂粘贴 @b → 行 nil」是缺席断言，没有登记的变异杀它，钉的是 D1「粘贴进来的 @ 不开」；
// 同装置的正对照是 M3 第一步（敲 @b → B3、帧 11），两条只差敲与粘贴；值 "@b" 防止粘贴根本没进去时空绿。
func TestAtxPasteCloses(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@")
	m = atxSteps(t, m, tea.PasteMsg{Content: "br"})
	atxWantLines(t, "@ 之后粘贴 br", m, nil)
	atxWantValue(t, "@ 之后粘贴 br", m, "@br")
	atxWantHead(t, "@ 之后粘贴 br", m, atxH("粘贴了 2 个字符"))
	m = tui1bType(t, m, "a")
	atxWantLines(t, "粘贴之后敲 a", m, nil)

	p, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	p = atxSteps(t, p, tea.PasteMsg{Content: "@b"})
	atxWantLines(t, "只喂粘贴 @b", p, nil)
	atxWantFrame(t, "只喂粘贴 @b", p, 8)
	atxWantValue(t, "只喂粘贴 @b", p, "@b")
}

// ═══ M10 ══════════════════════════════════════════════════════════════

// TestAtxResize 是 M10：窗口高度决定列表显示几行，补全状态不因窗口变化而丢。
//
// 【判别力】h=14 钉住提示行的「放得下才加」（A22）与 avail 的 -1（A45 让 h=14 多出一行）；
// h=24 那步是 h=9 缺席的对照（补全没丢）。
func TestAtxResize(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@")
	steps := []struct {
		h     int
		lines []string
		frame int
	}{
		{15, atxALL, 14},
		{14, atxALL[:5], 13},
		{12, atxALL[:3], 11},
		{10, atxALL[:1], 9},
		{9, nil, 8},
		{24, atxALL, 14},
	}
	for _, st := range steps {
		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, tea.WindowSizeMsg{Width: 80, Height: st.h})
		where := fmt.Sprintf("h=%d", st.h)
		atxWantNilCmd(t, where, cmd)
		atxWantLines(t, where, m, st.lines)
		atxWantFrame(t, where, m, st.frame)
	}
	var cmd tea.Cmd
	m, cmd = tui1bStep(t, m, tea.WindowSizeMsg{Width: 0, Height: 0})
	atxWantNilCmd(t, "WindowSizeMsg{0, 0}", cmd)
	atxWantLines(t, "WindowSizeMsg{0, 0}", m, nil)
}

// ═══ M11 ══════════════════════════════════════════════════════════════

// TestAtxNoCandidates 是 M11：没有候选时敲 @ 不弹，回车照常提交。
//
// 【判别力】正对照是 M1（同装置换夹具）；A39（在补全中就截键，不看在不在屏幕上）让回车 panic（空列表取下标）。
// 「sub.count() == 1」能为假：回车被截走时是 0。
func TestAtxNoCandidates(t *testing.T) {
	for _, tc := range []struct {
		name string
		list []repl.MentionCandidate
	}{
		{"nil", nil},
		{"empty", []repl.MentionCandidate{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, sub := atxNew(t, 80, 24, tc.list)
			m = tui1bType(t, m, "@")
			atxWantLines(t, "敲 @", m, nil)
			atxWantFrame(t, "敲 @", m, 8)
			atxWantCursorNil(t, "敲 @", m)
			var cmd tea.Cmd
			_, cmd = tui1bStep(t, m, tui1bEnter())
			tui1bRun(t, cmd)
			if n := sub.count(); n != 1 || sub.text(0) != "@" {
				t.Errorf("回车：应当提交 1 次、收到 %q，实际 %d 次、%q", "@", n, sub.text(0))
			}
		})
	}
}

// ═══ M12 ══════════════════════════════════════════════════════════════

// TestAtxBusy 是 M12：忙时、收口窗口里敲 @ 不开；空闲后新敲的 @ 才开。
//
// 【判别力】A13（不看忙）让第 1 步红；A31（beginSubmit 不收起）让第 2 步红；A14（不看收口）让第 3 步红；
// 每一步的「之后新敲 ` @` 就开」是同装置的正对照。
func TestAtxBusy(t *testing.T) {
	t.Run("busy", func(t *testing.T) {
		m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
		m = tui1bType(t, m, "x")
		m = atxSteps(t, m, tui1bEnter())
		m = tui1bType(t, m, "@")
		atxWantLines(t, "忙时敲 @", m, nil)
		m = atxSteps(t, m, submitDoneMsg{members: atxCands(atxMembers)})
		atxWantLines(t, "回到空闲", m, nil)
		m = tui1bType(t, m, "b")
		atxWantLines(t, "回到空闲之后敲 b", m, nil)
		m = tui1bType(t, m, " @")
		atxWantLines(t, "空闲后新敲 ` @`", m, atxALL)
	})

	t.Run("submit_clears", func(t *testing.T) {
		m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
		m = tui1bType(t, m, "@zz")
		m = atxSteps(t, m, tui1bEnter())
		m = tui1bType(t, m, "@")
		atxWantLines(t, "补全中（没有匹配）回车提交之后敲 @", m, nil)
	})

	t.Run("finalizing", func(t *testing.T) {
		m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
		m = tui1bType(t, m, "x")
		m = atxSteps(t, m, tui1bEnter())
		m = atxSteps(t, m, roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 1, Speakers: []string{"rba_a"}}})
		m = atxSteps(t, m, submitDoneMsg{members: atxCands(atxMembers)})
		if !m.finalizing {
			t.Fatalf("前提不成立：带名单的一轮之后 submitDone，应当进入收口窗口")
		}
		m = tui1bType(t, m, "@")
		atxWantLines(t, "收口窗口里敲 @", m, nil)
		m = atxSteps(t, m, boardFinalizeMsg{})
		m = tui1bType(t, m, " @")
		atxWantLines(t, "收口之后敲 ` @`", m, atxALL)
	})
}

// ═══ M13 ══════════════════════════════════════════════════════════════

// atxSrcSub 是实现了 mentionSource 的假 Submitter：加锁记下 Submit 与 MentionCandidates 各自收到的 ctx，
// Submit 返回 subErr，MentionCandidates 返回 (list, listErr)。
type atxSrcSub struct {
	mu       sync.Mutex
	subErr   error
	list     []repl.MentionCandidate
	listErr  error
	subCtxs  []context.Context
	listCtxs []context.Context
}

// 编译期核对：假件满足生产的 mentionSource；不满足时 submitCmd 的类型断言静默失败，本用例会变成测别的东西。
var _ mentionSource = (*atxSrcSub)(nil)

func (s *atxSrcSub) Submit(ctx context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subCtxs = append(s.subCtxs, ctx)
	return s.subErr
}

func (s *atxSrcSub) IsQuit(error) bool { return false }

func (s *atxSrcSub) MentionCandidates(ctx context.Context) ([]repl.MentionCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCtxs = append(s.listCtxs, ctx)
	return atxCands(s.list), s.listErr
}

func (s *atxSrcSub) ctxs() (sub, list []context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]context.Context(nil), s.subCtxs...), append([]context.Context(nil), s.listCtxs...)
}

// atxOneDone 从一条 Cmd 的产出里取唯一的 submitDoneMsg；不是恰好一条是前提不成立。
func atxOneDone(t *testing.T, tr tui1bTrace) submitDoneMsg {
	t.Helper()
	var out []submitDoneMsg
	for _, msg := range tr.msgs {
		if d, ok := msg.(submitDoneMsg); ok {
			out = append(out, d)
		}
	}
	if len(out) != 1 {
		t.Fatalf("前提不成立：产出里应当恰好 1 条 submitDone，实际 %d 条：%v", len(out), tr.log)
	}
	return out[0]
}

// atxSrcModel 按 M13 第 1 步造 Model：Ctx 是 root，Println 是记录器，收过 80x24。
func atxSrcModel(t *testing.T, src *atxSrcSub, root context.Context) Model {
	t.Helper()
	m := New(Options{Submit: src, Out: tui1bNopDrainer{}, Version: "t", Ctx: root, Println: (&tui1bPrinter{}).cmd})
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

// TestAtxSubmitCmdFetches 是 M13：submitCmd 在 Submit 无错返回之后用根 ctx 取候选，随 submitDoneMsg 交回；
// Update 收到 submitDoneMsg 时整份替换候选（含 nil）。
//
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
// 【判别力】A33（用 runCtx 取）→ 第 1 步 ctx 不等于 root（runCtx 已被 Ctrl-C 取消，也是另一个值）；
// A34（出错也取）→ 第 2 步被调；A35（不看 err）→ 第 3 步不为 nil；A32（submitDoneMsg 不换候选）→ 第 1 步最后一句；
// A49（members 为 nil 时不换，旧名单留着）→ 第 4 步最后一句是 ALL。
// 第 4 步「行 nil」是缺席断言，对照是只差 nil 的那一条（同一序列换成 atxMembers → ALL、帧 14）：
// 第 4 步的 @ 在空格之后，左边界判据只在它上面跑，「Esc 之后、喂过 submitDoneMsg 之后再敲 ` @` 能重开」只有对照能证明。
func TestAtxSubmitCmdFetches(t *testing.T) {
	t.Run("root_ctx", func(t *testing.T) {
		root, cancel := context.WithCancel(context.Background())
		defer cancel()
		src := &atxSrcSub{list: []repl.MentionCandidate{{ID: "zz", Name: ""}}}
		m := atxSrcModel(t, src, root)
		m = tui1bType(t, m, "x")
		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, tui1bEnter())
		m, _ = tui1bStep(t, m, tui1bCtrl('c')) // 忙时第一次 Ctrl-C：只武装
		m, _ = tui1bStep(t, m, tui1bCtrl('c')) // 窗口内第二次：取消本条命令的 ctx
		done := atxOneDone(t, tui1bRun(t, cmd))
		want := []repl.MentionCandidate{{ID: "zz", Name: ""}}
		if !reflect.DeepEqual(done.members, want) {
			t.Errorf("第 1 步：submitDoneMsg.members 应当是 %#v，实际 %#v", want, done.members)
		}
		subCtxs, listCtxs := src.ctxs()
		if len(listCtxs) != 1 {
			t.Errorf("第 1 步：MentionCandidates 应当恰好被调 1 次，实际 %d 次", len(listCtxs))
		} else if listCtxs[0] != root {
			t.Errorf("第 1 步：MentionCandidates 收到的 ctx 应当就是根 ctx（同一个值），实际 %v", listCtxs[0])
		}
		if len(subCtxs) != 1 || subCtxs[0].Err() == nil {
			t.Errorf("前提不成立：Submit 应当被调 1 次且收到的 ctx 已被 Ctrl-C 取消，实际 %d 次", len(subCtxs))
		}
		m, _ = tui1bStep(t, m, done)
		m = tui1bType(t, m, "@")
		atxWantLines(t, "第 1 步：喂回 submitDoneMsg 之后敲 @", m, []string{"> @zz"})
	})

	t.Run("submit_err", func(t *testing.T) {
		src := &atxSrcSub{subErr: errors.New("x"), list: []repl.MentionCandidate{{ID: "zz", Name: ""}}}
		m := atxSrcModel(t, src, context.Background())
		m = tui1bType(t, m, "x")
		var cmd tea.Cmd
		_, cmd = tui1bStep(t, m, tui1bEnter())
		done := atxOneDone(t, tui1bRun(t, cmd))
		if done.err == nil {
			t.Fatalf("前提不成立：Submit 返回错误时 submitDoneMsg.err 应当非 nil")
		}
		if done.members != nil {
			t.Errorf("第 2 步：Submit 出错时 members 应当是 nil，实际 %#v", done.members)
		}
		if _, listCtxs := src.ctxs(); len(listCtxs) != 0 {
			t.Errorf("第 2 步：Submit 出错时 MentionCandidates 不应当被调，实际 %d 次", len(listCtxs))
		}
	})

	t.Run("list_err", func(t *testing.T) {
		src := &atxSrcSub{list: []repl.MentionCandidate{{ID: "zz", Name: ""}}, listErr: errors.New("db")}
		m := atxSrcModel(t, src, context.Background())
		m = tui1bType(t, m, "x")
		var cmd tea.Cmd
		_, cmd = tui1bStep(t, m, tui1bEnter())
		done := atxOneDone(t, tui1bRun(t, cmd))
		if _, listCtxs := src.ctxs(); len(listCtxs) != 1 {
			t.Fatalf("前提不成立：MentionCandidates 应当被调 1 次，实际 %d 次", len(listCtxs))
		}
		if done.members != nil {
			t.Errorf("第 3 步：取候选出错时 members 应当是 nil，实际 %#v", done.members)
		}
	})

	for _, tc := range []struct {
		name  string
		list  []repl.MentionCandidate
		lines []string
		frame int
	}{
		{"replace_nil", nil, nil, 8},
		{"replace_nil_control", atxCands(atxMembers), atxALL, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
			m = tui1bType(t, m, "@")
			if got := m.atLines(); !reflect.DeepEqual(got, atxALL) {
				t.Fatalf("前提不成立：敲 @ 应当是 ALL，实际 %q", got)
			}
			m = atxSteps(t, m, tui1bEsc(), submitDoneMsg{members: tc.list})
			m = tui1bType(t, m, " @")
			atxWantValue(t, tc.name, m, "@ @")
			atxWantLines(t, tc.name, m, tc.lines)
			atxWantFrame(t, tc.name, m, tc.frame)
		})
	}
}

// ═══ M14 ══════════════════════════════════════════════════════════════

// TestAtxWidth 是 M14：每行截到横线的宽度，宽度兜底 20、封顶 120。
//
// 【判别力】A26（不截）→ 宽 40 首行是整串；A27（不封 120）→ 宽 200 首行 149。
// 宽 40 的首行显示宽 39：再放一个字就 41，钉住「按显示宽度截、宽字符不劈半」。
func TestAtxWidth(t *testing.T) {
	long := []repl.MentionCandidate{{ID: "long", Name: "一二三四五六七八九十一二三四五六七八九十"}, {ID: "x", Name: ""}}

	m, _ := atxNew(t, 40, 24, atxCands(long))
	m = tui1bType(t, m, "@")
	atxWantLines(t, "宽 40", m, []string{"> @long  一二三四五六七八九十一二三四五", "  @x"})

	n, _ := atxNew(t, 10, 24, atxCands(long))
	n = tui1bType(t, n, "@")
	atxWantLines(t, "宽 10（兜底到 20）", n, []string{"> @long  一二三四五", "  @x"})

	w, _ := atxNew(t, 200, 24, []repl.MentionCandidate{{ID: "long", Name: strings.Repeat("ab", 70)}})
	w = tui1bType(t, w, "@")
	lines := w.atLines()
	if len(lines) == 0 {
		t.Fatalf("宽 200：列表应当出现，实际 nil")
	}
	if got := ansi.StringWidth(lines[0]); got != 120 {
		t.Errorf("宽 200：首行显示宽应当是 120，实际 %d（%q）", got, lines[0])
	}
}

// ═══ M15 ══════════════════════════════════════════════════════════════

// TestAtxExitClearCountsList 是 M15：exitClearSeq 按帧高清屏，帧高算上列表。
//
// 【判别力】A5（frameHeight 不数列表）→ [7A。对照：X-39e A 系列（没有列表时同一函数清 8 行）。
func TestAtxExitClearCountsList(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@")
	want := tui1dESC + "[13A\r" + tui1dESC + "[J"
	if got := exitClearSeq(m, nil); got != want {
		t.Errorf("exitClearSeq 应当是 %q（帧 14），实际 %q", want, got)
	}
}
