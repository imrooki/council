package tui

// 本文件覆盖 AT1-FU 冻结契约（E:\multi_model_cowork\at1fu_contract.md，v1.2）§2 的 F1-F5：
// @ 补全的落地后跟进。纯 Model 驱动，不起 Program。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 4908f13 的源码。
// 本文件只引用 HEAD 已有的名字，在 HEAD 上编译得过；HEAD 上 F1 第 2、3、4 步红，F2-F5 绿（契约 §2.0）。
//
// 纪律（契约 §2.0）：包级标识符一律以 atf 开头；只读复用 atx / tui1b 的辅助，不改它们；
// 不读 m.at、m.members；写 m.ta 只在 F1 第 5、6 步，各调一次 m.ta.KeyMap.Paste.SetKeys("ctrl+v")。
// 「值」= m.ta.Value()；「行」= m.atLines()；「帧」= m.frameHeight()；「光标」= (m.ta.Line(), m.ta.Column())；「表头」= m.chrome()[0]。
//
// 剪贴板禁令（契约 §2.0，硬性）：按 Ctrl-V 与 Ctrl+Shift+C 之后拿到的 Cmd 一律不执行。
// 它们在本文件里只有三种用法：与 nil 比较；作为 reflect.ValueOf 的唯一实参并紧跟 .Pointer()；传给 atxWantNilCmd。
// 失败消息里只报「cmd == nil」这个布尔值，不把 Cmd 本身放进任何格式化实参。
// 每一段各起一个新 Model（atxNew），不从同一个 Model 分叉：textarea 的缓冲是共享切片。

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
)

// atfB3sel1 是 @b 之后按一次无修饰 ↓ 的整列：选中标记在第二项（契约 §3，整串冻结）。
var atfB3sel1 = []string{
	"  @b1",
	"> @beta   贝塔",
	"  @bravo  Bravo",
}

// atfCtrlV 是 Ctrl-V 键（textarea 默认键表里的 ctrl+v）。
var atfCtrlV = tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}

// atfCtrlShiftC 是 Ctrl+Shift+C 键（textarea 默认键表里的 CopySelection，写剪贴板）。
var atfCtrlShiftC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModShift}

// ═══ F1 ═══════════════════════════════════════════════════════════════

// TestAtfCtrlVUnbound 是 F1（R0）：New 里解绑 textarea 的 Paste，按 Ctrl-V 不再交回读剪贴板的 Cmd。
//
// 【判别力】第 2 步与第 1 步只差「经没经过 New」：第 1 步证明 bubbles 默认启用，第 2 步的 false 只能来自 New。
// 第 3、4 步的「Cmd 为 nil」能为假（A53 去掉 Unbind 时 Cmd 是 textarea.Paste），对照是第 5、6 步：
// 同装置只差 SetKeys 那一行，证明 Ctrl-V 这个键确实到了 textarea、绑定在时 Cmd 就是 textarea.Paste，所以 nil 不是「键没到」。
// A53 下第 2、3、4 步红；A56（Unbind 换成 SetEnabled(false)）下第 2、3、4 步照绿，只有第 5 步的前提 Fatalf：
// SetKeys 只换 keys、不清 disabled，所以正对照只许单调一次 SetKeys，不许 SetEnabled、不许整体赋值。
// 第 4 步的「值不变」「行仍 B3」在 A53 下照绿（Paste 返回之前什么都没改），杀手是 Cmd 那一句。
// 第 7 步恒真（契约 §7 (i)）：Cmd 不执行，今天的 Submit 只在 Cmd 里调用，只登记不删。
func TestAtfCtrlVUnbound(t *testing.T) {
	pastePtr := reflect.ValueOf(textarea.Paste).Pointer()

	// 第 1 步（前提）：bubbles 默认启用 Paste。
	if !textarea.New().KeyMap.Paste.Enabled() {
		t.Fatalf("第 1 步（前提）：textarea.New() 的 Paste 应当是启用的，实际未启用；bubbles 默认值变了，本用例的对照失效")
	}

	// 第 2 步：新 Model 的 Paste 已解绑。
	m2, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	if m2.ta.KeyMap.Paste.Enabled() {
		t.Errorf("第 2 步：新 Model 的 m.ta.KeyMap.Paste 应当已解绑（Enabled() == false），实际仍启用")
	}

	// 第 3 步：列表不在，敲 x 再按 Ctrl-V。
	m3, sub3 := atxNew(t, 80, 24, atxCands(atxMembers))
	m3 = tui1bType(t, m3, "x")
	m3, cmd3 := tui1bStep(t, m3, atfCtrlV)
	atxWantNilCmd(t, "第 3 步：列表不在时按 Ctrl-V", cmd3)
	atxWantValue(t, "第 3 步：列表不在时按 Ctrl-V", m3, "x")

	// 第 4 步：列表开着，敲 @b 再按 Ctrl-V。
	m4, sub4 := atxNew(t, 80, 24, atxCands(atxMembers))
	m4 = tui1bType(t, m4, "@b")
	atxWantLines(t, "第 4 步（前提）：敲 @b", m4, atxB3)
	m4, cmd4 := tui1bStep(t, m4, atfCtrlV)
	atxWantNilCmd(t, "第 4 步：列表开着时按 Ctrl-V", cmd4)
	atxWantValue(t, "第 4 步：列表开着时按 Ctrl-V", m4, "@b")
	atxWantLines(t, "第 4 步：列表开着时按 Ctrl-V", m4, atxB3)

	// 第 5 步：正对照（列表不在）。只调一次 SetKeys，紧跟着判 Enabled()。
	m5, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m5.ta.KeyMap.Paste.SetKeys("ctrl+v")
	if !m5.ta.KeyMap.Paste.Enabled() {
		t.Fatalf("第 5 步（前提）：SetKeys 之后 Paste 应当启用（New 里若用 SetEnabled(false) 代替 Unbind，SetKeys 不清 disabled，这里就红）")
	}
	m5 = tui1bType(t, m5, "x")
	m5, cmd5 := tui1bStep(t, m5, atfCtrlV)
	if reflect.ValueOf(cmd5).Pointer() != pastePtr {
		t.Errorf("第 5 步：Paste 绑定在时按 Ctrl-V，Cmd 应当是 textarea.Paste（只比函数指针，不调用），实际不是（cmd == nil：%v）", cmd5 == nil)
	}
	atxWantValue(t, "第 5 步：Paste 绑定在时按 Ctrl-V", m5, "x")

	// 第 6 步：正对照（列表开着）。写法同第 5 步。
	m6, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m6.ta.KeyMap.Paste.SetKeys("ctrl+v")
	if !m6.ta.KeyMap.Paste.Enabled() {
		t.Fatalf("第 6 步（前提）：SetKeys 之后 Paste 应当启用（New 里若用 SetEnabled(false) 代替 Unbind，SetKeys 不清 disabled，这里就红）")
	}
	m6 = tui1bType(t, m6, "@b")
	atxWantLines(t, "第 6 步（前提）：敲 @b", m6, atxB3)
	m6, cmd6 := tui1bStep(t, m6, atfCtrlV)
	if reflect.ValueOf(cmd6).Pointer() != pastePtr {
		t.Errorf("第 6 步：Paste 绑定在、列表开着时按 Ctrl-V，Cmd 应当是 textarea.Paste（只比函数指针，不调用），实际不是（cmd == nil：%v）", cmd6 == nil)
	}
	atxWantValue(t, "第 6 步：Paste 绑定在、列表开着时按 Ctrl-V", m6, "@b")
	atxWantLines(t, "第 6 步：Paste 绑定在、列表开着时按 Ctrl-V", m6, atxB3)

	// 第 7 步：第 3、4 步的 Model 没有提交（恒真，见【判别力】）。
	if n := sub3.count(); n != 0 {
		t.Errorf("第 7 步：第 3 步的 Model 的 Submit 应当是 0 次，实际 %d 次", n)
	}
	if n := sub4.count(); n != 0 {
		t.Errorf("第 7 步：第 4 步的 Model 的 Submit 应当是 0 次，实际 %d 次", n)
	}
}

// ═══ F2 ═══════════════════════════════════════════════════════════════

// TestAtfModifiedEnterTab 是 F2（R1）：列表开着时带修饰键的回车与 Tab 同样只补全、不提交。
//
// 【判别力】第 1 段「没提交」是缺席断言，对照是第 2 段同装置（只差列表开没开）：
// Shift+Enter、Alt+Enter 在列表不在时真的提交（先跑 Cmd 再读计数，X44 教训）；
// Shift+Tab 在列表不在时什么都不做，所以第 1 段的 "hi @bravo " 只能来自截获。
// A52（B4 只截无修饰键）下：shift_enter、alt_enter 第 1 段提交了 "hi @br"；shift_tab 第 1 段值没补全、行没收、帧 9。三个子测试全红。
// 这里执行的 Cmd 只有这三个键返回的：补全的光标闪烁、提交的 Sequence、或 nil，都不经剪贴板绑定。
func TestAtfModifiedEnterTab(t *testing.T) {
	cases := []struct {
		name    string
		key     tea.KeyPressMsg
		submits bool
	}{
		{"shift_enter", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}, true},
		{"alt_enter", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}, true},
		{"shift_tab", tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 第 1 段：列表开着。
			m, sub := atxNew(t, 80, 24, atxCands(atxMembers))
			m = tui1bType(t, m, "hi @br")
			atxWantLines(t, "第 1 段（前提）：敲 hi @br", m, []string{"> @bravo  Bravo"})
			m, cmd := tui1bStep(t, m, c.key)
			tr := tui1bRun(t, cmd)
			if !atxNoSubmitDone(tr) {
				t.Errorf("第 1 段：列表开着时按键不应当提交，产出里却有 submitDone：%q", tr.log)
			}
			if n := sub.count(); n != 0 {
				t.Errorf("第 1 段：Submit 应当是 0 次，实际 %d 次", n)
			}
			atxWantValue(t, "第 1 段：列表开着时按键", m, "hi @bravo ")
			atxWantLines(t, "第 1 段：列表开着时按键", m, nil)
			atxWantFrame(t, "第 1 段：列表开着时按键", m, 8)

			// 第 2 段：对照（列表不在）。另起新 Model。
			m2, sub2 := atxNew(t, 80, 24, atxCands(atxMembers))
			m2 = tui1bType(t, m2, "hi @br")
			m2, _ = tui1bStep(t, m2, tui1bEsc())
			atxWantLines(t, "第 2 段（前提）：敲 hi @br 再按 Esc", m2, nil)
			m2, cmd2 := tui1bStep(t, m2, c.key)
			tr2 := tui1bRun(t, cmd2)
			if c.submits {
				if n := sub2.count(); n != 1 {
					t.Errorf("第 2 段：列表不在时按键应当提交，Submit 应当是 1 次，实际 %d 次", n)
				}
				if got := sub2.text(0); got != "hi @br" {
					t.Errorf("第 2 段：Submit 收到的应当是 %q，实际 %q", "hi @br", got)
				}
				if atxNoSubmitDone(tr2) {
					t.Errorf("第 2 段：产出里应当有 submitDone，实际 %q", tr2.log)
				}
				atxWantValue(t, "第 2 段：列表不在时按键", m2, "")
			} else {
				if n := sub2.count(); n != 0 {
					t.Errorf("第 2 段：列表不在时 Shift+Tab 不应当提交，Submit 实际 %d 次", n)
				}
				if len(tr2.log) != 0 {
					t.Errorf("第 2 段：列表不在时 Shift+Tab 的产出应当为空，实际 %q", tr2.log)
				}
				atxWantValue(t, "第 2 段：列表不在时按键", m2, "hi @br")
			}
			atxWantLines(t, "第 2 段：列表不在时按键", m2, nil)
		})
	}
}

// ═══ F3 ═══════════════════════════════════════════════════════════════

// TestAtfCtrlTAtLeftEdge 是 F3（R2）：光标在行尾、@ 紧贴光标左边时按 Ctrl-T，@ 被挪走，列表收起。
//
// 【判别力】第 2 步「行 nil」是缺席断言：前提（第 1 步）是同一个 Model 按键之前列表开着；
// 值与光标证明 Ctrl-T 到了 textarea；第 3 步证明 Ctrl-T 本身不收列表（@ 与它左边不动时照开），
// 所以第 2 步的收起来自 @ 被挪走。A54（@ 那一处与左边界那一处同时去掉）下第 2 步行 == atxALL、帧 14，F3 红。
// 只去掉其中一处（A41、A55）F3 照绿，是等价变异（契约 §4）。
func TestAtfCtrlTAtLeftEdge(t *testing.T) {
	ctrlT := tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}

	// 第 1 步（前提）：敲两个空格加 @，列表开着。
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "  @")
	atxWantLines(t, "第 1 步（前提）：敲两个空格加 @", m, atxALL)

	// 第 2 步：按 Ctrl-T（Cmd 不断言）。
	m, _ = tui1bStep(t, m, ctrlT)
	atxWantValue(t, "第 2 步：按 Ctrl-T", m, " @ ")
	if l, c := m.ta.Line(), m.ta.Column(); l != 0 || c != 3 {
		t.Errorf("第 2 步：按 Ctrl-T 之后光标应当是 (0, 3)，实际 (%d, %d)", l, c)
	}
	atxWantLines(t, "第 2 步：按 Ctrl-T", m, nil)
	atxWantFrame(t, "第 2 步：按 Ctrl-T", m, 8)

	// 第 3 步：对照。@ 与它左边不动时，Ctrl-T 只对调前缀里的两个字，列表照开。
	m3, _ := atxNew(t, 80, 24, []repl.MentionCandidate{{ID: "ab"}, {ID: "ba"}})
	m3 = tui1bType(t, m3, "  @ab")
	atxWantLines(t, "第 3 步（前提）：敲两个空格加 @ab", m3, []string{"> @ab"})
	m3, _ = tui1bStep(t, m3, ctrlT)
	atxWantValue(t, "第 3 步：按 Ctrl-T", m3, "  @ba")
	if l, c := m3.ta.Line(), m3.ta.Column(); l != 0 || c != 5 {
		t.Errorf("第 3 步：按 Ctrl-T 之后光标应当是 (0, 5)，实际 (%d, %d)", l, c)
	}
	atxWantLines(t, "第 3 步：按 Ctrl-T", m3, []string{"> @ba"})
}

// ═══ F4 ═══════════════════════════════════════════════════════════════

// TestAtfModifiedListKeys 是 F4（R5）：列表开着时带修饰键的 ↑、↓、Esc 与无修饰时一样归列表。
//
// 【判别力】第 1 段的「行」能为假，是杀手：A57、A58 下带修饰键的 ↑、↓ 掉进 textarea，单行里什么都不做，
// 选中项不动（shift_up、alt_up 行仍是 atfB3sel1；shift_down、alt_down 行仍是 atxB3）。
// ↑ 的两个子测试先按一次无修饰 ↓：选中项在第一项时 ↑ 本来就不动，不先下移就分不开 A57。
// Esc 两个子测试的「行 nil」「帧 8」是缺席断言，前提是同一个 Model 按键之前行 == atxB3；A59 下行仍 == atxB3、帧 11。
// 第 2 段是同装置对照（只差列表开没开）：列表不在时这六个键不动值、不动光标、不开列表，
// 所以第 1 段里选中项的移动与列表的收起只能来自列表分支。
// 第 1 段的 Cmd == nil 在 A57-A59 下照绿，杀手是「行」那一句。
func TestAtfModifiedListKeys(t *testing.T) {
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	cases := []struct {
		name      string
		key       tea.KeyPressMsg
		preDown   bool
		wantLines []string
		wantFrame int
	}{
		{"shift_up", tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}, true, atxB3, 11},
		{"alt_up", tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt}, true, atxB3, 11},
		{"shift_down", tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}, false, atfB3sel1, 11},
		{"alt_down", tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt}, false, atfB3sel1, 11},
		{"shift_esc", tea.KeyPressMsg{Code: tea.KeyEscape, Mod: tea.ModShift}, false, nil, 8},
		{"alt_esc", tea.KeyPressMsg{Code: tea.KeyEscape, Mod: tea.ModAlt}, false, nil, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 第 1 段：列表开着。
			m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
			m = tui1bType(t, m, "@b")
			atxWantLines(t, "第 1 段（前提）：敲 @b", m, atxB3)
			if c.preDown {
				m, _ = tui1bStep(t, m, down)
				atxWantLines(t, "第 1 段（前提）：先按一次无修饰 ↓", m, atfB3sel1)
			}
			m, cmd := tui1bStep(t, m, c.key)
			atxWantNilCmd(t, "第 1 段：列表开着时按键", cmd)
			atxWantValue(t, "第 1 段：列表开着时按键", m, "@b")
			atxWantLines(t, "第 1 段：列表开着时按键", m, c.wantLines)
			atxWantFrame(t, "第 1 段：列表开着时按键", m, c.wantFrame)

			// 第 2 段：对照（列表不在）。另起新 Model。
			m2, _ := atxNew(t, 80, 24, atxCands(atxMembers))
			m2 = tui1bType(t, m2, "@b")
			m2, _ = tui1bStep(t, m2, tui1bEsc())
			atxWantLines(t, "第 2 段（前提）：敲 @b 再按 Esc", m2, nil)
			if c.preDown {
				m2, _ = tui1bStep(t, m2, down)
			}
			m2, cmd2 := tui1bStep(t, m2, c.key)
			atxWantNilCmd(t, "第 2 段：列表不在时按键", cmd2)
			atxWantValue(t, "第 2 段：列表不在时按键", m2, "@b")
			if l, col := m2.ta.Line(), m2.ta.Column(); l != 0 || col != 2 {
				t.Errorf("第 2 段：列表不在时按键之后光标应当是 (0, 2)，实际 (%d, %d)", l, col)
			}
			atxWantLines(t, "第 2 段：列表不在时按键", m2, nil)
			atxWantFrame(t, "第 2 段：列表不在时按键", m2, 8)
		})
	}
}

// ═══ F5 ═══════════════════════════════════════════════════════════════

// TestAtfCtrlShiftCClosesList 是 F5（R5）：列表开着时 Ctrl+Shift+C 只收起列表，不退出。
// 按下之后返回的 Cmd 一律不执行（复制写的是终端剪贴板：TUI-1 起是 tea.SetClipboard，
// 由 Program 发 OSC52；空闲 Ctrl-C 第一次只交回到期 tick，第二次才退出）。
//
// 【判别力】第 2 步的「Cmd == nil」与「行 nil」都能为假：A60（列表分支的 Ctrl-C 收窄成 msg.Mod == tea.ModCtrl）下
// 这个键掉进空闲的 Ctrl-C 分支，第一次只武装：Cmd 是到期 tick、表头是「再按一次 Ctrl-C 退出」、行仍 == atxB3，第 2 步两处红。
// 第 3 步是同装置对照（只差列表开没开）：证明这个键确实到了 Council 的拦截、列表不在时不再是退出，
// 所以第 2 步的 nil 不是「键没到」。第 3 步比 tea.Quit 的函数指针；Cmd 为 nil 时指针是 0，同样判红。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C。
// TUI-1（docs/21e_TUI1_TASK.md §3）：Ctrl+Shift+C 改成「复制输入框里的字 + 状态栏报字数」，
// 第 3 步的对照因此从「连按两次退出」改成「连按两次都是复制、都不是退出」。
// 它仍然证明这个键到得了 Council 的拦截（表头与 Cmd 都变了），只是去向换成了复制。
// 第 1 步证明这个键在 textarea 里就是 CopySelection（前提不变：Council 自己截走它，
// textarea 才收不到、才不写它自己那份剪贴板）。不断言 Submit 次数（Cmd 不执行，计数恒为 0，契约 §2.5 末句）。
func TestAtfCtrlShiftCClosesList(t *testing.T) {
	// 第 1 步（前提）。
	if s := atfCtrlShiftC.String(); s != "ctrl+shift+c" {
		t.Fatalf("第 1 步（前提）：键的 String() 应当是 %q，实际 %q", "ctrl+shift+c", s)
	}
	if got := textarea.New().KeyMap.CopySelection.Keys(); !reflect.DeepEqual(got, []string{"ctrl+shift+c"}) {
		t.Fatalf("第 1 步（前提）：textarea 默认 CopySelection 的键应当是 %q，实际 %q", []string{"ctrl+shift+c"}, got)
	}

	// 第 2 步：列表开着。
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@b")
	atxWantLines(t, "第 2 步（前提）：敲 @b", m, atxB3)
	m, cmd := tui1bStep(t, m, atfCtrlShiftC)
	atxWantNilCmd(t, "第 2 步：列表开着时按 Ctrl+Shift+C", cmd)
	atxWantLines(t, "第 2 步：列表开着时按 Ctrl+Shift+C", m, nil)
	atxWantValue(t, "第 2 步：列表开着时按 Ctrl+Shift+C", m, "@b")

	// 第 3 步：对照（列表不在）。另起新 Model。
	// TUI-1 起这个键在列表不在时是复制：状态栏换成「已复制 2 个字符」，不武装、不退出。
	m3, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m3 = tui1bType(t, m3, "@b")
	m3, _ = tui1bStep(t, m3, tui1bEsc())
	atxWantLines(t, "第 3 步（前提）：敲 @b 再按 Esc", m3, nil)
	m3, cmdArm := tui1bStep(t, m3, atfCtrlShiftC)
	if cmdArm == nil || reflect.ValueOf(cmdArm).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
		t.Errorf("第 3 步：列表不在时 Ctrl+Shift+C 应当是复制、不是退出（cmd == nil：%v）", cmdArm == nil)
	}
	atxWantHead(t, "第 3 步：列表不在时第一次 Ctrl+Shift+C", m3, atxH("已复制 2 个字符"))
	m3, cmd3 := tui1bStep(t, m3, atfCtrlShiftC)
	if cmd3 == nil || reflect.ValueOf(cmd3).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
		t.Errorf("第 3 步：再按一次仍应当是复制、不是退出（cmd == nil：%v）", cmd3 == nil)
	}
	atxWantHead(t, "第 3 步：列表不在时第二次 Ctrl+Shift+C", m3, atxH("已复制 2 个字符"))
	atxWantValue(t, "第 3 步：列表不在时按 Ctrl+Shift+C", m3, "@b")
}
