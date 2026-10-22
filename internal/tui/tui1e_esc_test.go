package tui

// tui1e_esc_test.go 钉住 TUI-1（docs/21e_TUI1_TASK.md §2）的 Esc 四种情形。
// 纯 Model 驱动，不起 Program。只读复用 atx / tui1b 的既有辅助，不改它们。
//
// 四种情形按优先级从上到下：@ 补全列表开着（只关列表）→ 一轮正在跑（取消，一次就够）
// → 空闲且有字（清空输入框）→ 空闲且为空（解除 Ctrl-C 武装）。
// 另外两条钉住两处有意的边界：收口窗口里什么都不做；带修饰键的 Esc 维持原样。

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// tui1eEsc 是无修饰键的 Esc。用 tui1bEsc 的名字读起来更像「按了一下 Esc」，
// 这里留一个别名，失败信息里也好认。
func tui1eEsc() tea.KeyPressMsg { return tui1bEsc() }

// TestTui1eEscListOpenOnlyCloses 是情形 1：@ 补全列表开着时 Esc 只收起列表，
// 输入框里的字与状态栏都不动（21e §2「现状不变」）。
func TestTui1eEscListOpenOnlyCloses(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "@b")
	atxWantLines(t, "前提：敲 @b", m, atxB3)
	atxWantHead(t, "前提：敲 @b", m, atxH("空闲"))

	m, cmd := tui1bStep(t, m, tui1eEsc())
	atxWantNilCmd(t, "Esc", cmd)
	atxWantLines(t, "Esc", m, nil)
	atxWantValue(t, "Esc", m, "@b")
	atxWantHead(t, "Esc", m, atxH("空闲"))
}

// TestTui1eEscBusyCancelsOnce 是情形 2：一轮正在跑时 Esc 一次就取消，
// 第二次不再取消；忙时输入框里的字不被清掉（取消优先于清空）。
func TestTui1eEscBusyCancelsOnce(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)
	m = tui1bType(t, m, "草稿")
	m.busy = true
	m.status = "正在运行"
	cancels := 0
	m.cancel = func() { cancels++ }

	m, cmd := tui1bStep(t, m, tui1eEsc())
	atxWantNilCmd(t, "第一次 Esc", cmd)
	if cancels != 1 || !m.canceling {
		t.Errorf("第一次 Esc：cancel 应当被调 1 次、canceling 为 true，实际 cancel %d 次、canceling %v", cancels, m.canceling)
	}
	// 忙时表头在状态之后还接已用时间：这一轮没有真的跑起来，所以是 0s。
	atxWantHead(t, "第一次 Esc", m, atxH("正在取消  0s"))
	atxWantValue(t, "第一次 Esc", m, "草稿")

	m, cmd = tui1bStep(t, m, tui1eEsc())
	atxWantNilCmd(t, "第二次 Esc", cmd)
	if cancels != 1 {
		t.Errorf("第二次 Esc：cancel 不应当再被调用，实际累计 %d 次", cancels)
	}
	atxWantHead(t, "第二次 Esc", m, atxH("正在取消  0s"))

	// 取消之后这一轮照旧走 submitDone 收尾；canceling 为真只影响收尾动词，
	// 这里顺带钉住 Esc 没有把别的状态搅乱。
	m, _ = tui1bStep(t, m, submitDoneMsg{})
	atxWantHead(t, "submitDone 之后", m, atxH("空闲"))
	if sub.count() != 0 {
		t.Errorf("Esc 不应当提交任何东西，实际 Submit %d 次", sub.count())
	}
}

// TestTui1eEscIdleClearsInput 是情形 3：空闲、输入框有字时清空输入框、光标回开头。
func TestTui1eEscIdleClearsInput(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)
	m = tui1bType(t, m, "半截草稿")
	atxWantValue(t, "前提", m, "半截草稿")

	m, cmd := tui1bStep(t, m, tui1eEsc())
	atxWantNilCmd(t, "Esc", cmd)
	atxWantValue(t, "Esc", m, "")
	if l, c := m.ta.Line(), m.ta.Column(); l != 0 || c != 0 {
		t.Errorf("Esc 之后光标应当在 (0, 0)，实际 (%d, %d)", l, c)
	}
	atxWantHead(t, "Esc", m, atxH("空闲"))

	// 清空之后回车不提交任何东西：清空是真的清空，不是把字挪到了看不见的地方。
	m, cmd2 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd2)
	if n := sub.count(); n != 0 {
		t.Errorf("清空之后回车不应当提交，实际 Submit %d 次（收到 %q）", n, sub.text(0))
	}
	atxWantHead(t, "清空之后回车", m, atxH("空闲"))
}

// TestTui1eEscIdleEmptyDisarms 是情形 4：空闲、输入框为空时解除 Ctrl-C 的武装。
func TestTui1eEscIdleEmptyDisarms(t *testing.T) {
	m, _, _ := tui1bSized(t, nil)
	m, _ = tui1bStep(t, m, tui1bCtrl('c'))
	atxWantHead(t, "第一次 Ctrl-C", m, atxH("再按一次 Ctrl-C 退出"))
	if !m.ctrlCArmed {
		t.Fatalf("前提不成立：第一次 Ctrl-C 之后应当处于武装状态")
	}

	m, cmd := tui1bStep(t, m, tui1eEsc())
	atxWantNilCmd(t, "Esc", cmd)
	if m.ctrlCArmed {
		t.Errorf("Esc 之后不应当还武装着：再按一次 Ctrl-C 会直接退出")
	}
	atxWantHead(t, "Esc", m, atxH("空闲"))
	atxWantValue(t, "Esc", m, "")

	// 解除之后窗口内的 Ctrl-C 只重新武装，不退出。
	m, cmd = tui1bStep(t, m, tui1bCtrl('c'))
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("Esc 之后再按 Ctrl-C 应当只武装，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	atxWantHead(t, "再按 Ctrl-C", m, atxH("再按一次 Ctrl-C 退出"))
}

// TestTui1eEscKeepsBlockedDraft 钉住 X44 挡下的多行草稿不被 Esc 清掉。
//
// 21e §2 的第 3 种情形是「空闲、输入框有字 → 清空」，但 X44 冻结的那句话
// （状态栏一行「一次只能一条命令…」）的全部意义是「文字还在，改好再发」：
// 清掉之后输入框没有撤销，用户手上就没有这份草稿了。所以 Esc 在这个状态里
// 只解除 Ctrl-C 武装，不动文字，也不改那句提示。
// 既有用例 TestMlxHintPersistsAndSayEscape 第 7 步与 TestMlxHintStaysAfterFix 第 3 步
// 钉的正是这两点，这里再加一条把「为什么」写在用例里。
func TestTui1eEscKeepsBlockedDraft(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)
	m, _ = tui1bStep(t, m, tea.PasteMsg{Content: "a\n/c"})
	m, _ = tui1bStep(t, m, tui1bEnter())
	atxWantHead(t, "前提：多行提交被挡下", m, atxH(oneCommandHint))
	atxWantValue(t, "前提：多行提交被挡下", m, "a\n/c")

	m, cmd := tui1bStep(t, m, tui1eEsc())
	atxWantNilCmd(t, "Esc", cmd)
	atxWantValue(t, "Esc", m, "a\n/c")
	atxWantHead(t, "Esc", m, atxH(oneCommandHint))
	if n := sub.count(); n != 0 {
		t.Errorf("Esc 不应当提交任何东西，实际 Submit %d 次", n)
	}
}

// TestTui1eEscModifiedKeepsInput 钉住「清空只绑无修饰的 Esc」。
//
// 列表不在时，带修饰键的 Esc 此前什么都不做，19c 契约 F4 第 2 段用同装置对照钉着
// 「值不动、光标不动」。清空输入框是破坏性的动作（没有撤销），不该顺手落到
// Shift+Esc / Alt+Esc 上：Alt+Esc 在 Windows 是窗口切换，Shift+Esc 也常被终端占着。
func TestTui1eEscModifiedKeepsInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"shift", tea.KeyPressMsg{Code: tea.KeyEscape, Mod: tea.ModShift}},
		{"alt", tea.KeyPressMsg{Code: tea.KeyEscape, Mod: tea.ModAlt}},
		{"ctrl", tea.KeyPressMsg{Code: tea.KeyEscape, Mod: tea.ModCtrl}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := tui1bSized(t, nil)
			m = tui1bType(t, m, "草稿")
			m, cmd := tui1bStep(t, m, tc.key)
			atxWantNilCmd(t, "带修饰键的 Esc", cmd)
			atxWantValue(t, "带修饰键的 Esc", m, "草稿")

			// 同装置对照：同一个 Model 上换无修饰的 Esc，字就清掉了。
			m, _ = tui1bStep(t, m, tui1eEsc())
			atxWantValue(t, "同装置对照（无修饰的 Esc）", m, "")
		})
	}
}

// TestTui1eEscFinalizingDoesNothing 钉住收口窗口：Esc 什么都不做，与 Ctrl-C 在那里被挡住一致。
func TestTui1eEscFinalizingDoesNothing(t *testing.T) {
	m, _, _ := tui1bSized(t, nil)
	m = tui1bType(t, m, "草稿")
	m.finalizing = true
	m.status = "正在收口"

	m, cmd := tui1bStep(t, m, tui1eEsc())
	atxWantNilCmd(t, "收口窗口里的 Esc", cmd)
	atxWantValue(t, "收口窗口里的 Esc", m, "草稿")
	atxWantHead(t, "收口窗口里的 Esc", m, atxH("正在收口"))
}
