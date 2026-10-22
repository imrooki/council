package tui

// tui1e_copy_test.go 钉住 TUI-1（docs/21e_TUI1_TASK.md）的 §3（ctrl+shift+c 的复制反馈）
// 与 §4（/copy）。纯 Model 驱动，不起 Program。
//
// 剪贴板纪律：按键与 /copy 交回来的 Cmd 一律不执行。它写的是终端剪贴板
// （tea.SetClipboard 由 Program 发 OSC52），这里只判 nil 与 tea.Quit 的函数指针；
// 所以「复制了哪一段正文」用 copyInputText / CopyRecent 的返回值与状态栏的 rune 数来钉，
// 不去碰真剪贴板。

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
)

// tui1eCopyKey 是 Ctrl+Shift+C（textarea 默认键表里的 CopySelection，19c 契约 §0 R5 冻结的形状）。
var tui1eCopyKey = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModShift}

// tui1eCopySub 是带 /copy 能力的假 Submitter：Submit / IsQuit 复用 tui1bSubmitter，
// 另外记下 CopyRecent 收到的参数并交回用例摆好的结果。
type tui1eCopySub struct {
	tui1bSubmitter
	args  []string
	reply string
	err   error
}

func (s *tui1eCopySub) CopyRecent(_ context.Context, args string) (string, error) {
	s.args = append(s.args, args)
	return s.reply, s.err
}

// tui1eNew 造一个装了 tui1eCopySub 的 Model，并收过 80x24。
func tui1eNew(t *testing.T, sub *tui1eCopySub) (Model, *tui1bPrinter) {
	t.Helper()
	pr := &tui1bPrinter{}
	m := New(Options{
		Submit:  sub,
		Out:     tui1bNopDrainer{},
		Version: "0.0.0-test",
		Println: pr.cmd,
	})
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, pr
}

// tui1eDone 从一条 Cmd 的产出里取唯一的 copyDoneMsg。
// 它不执行复制那条 Cmd 本身：取数（CopyRecent）跑的是假件，写剪贴板的 Cmd 留在返回值里。
func tui1eDone(t *testing.T, cmd tea.Cmd) copyDoneMsg {
	t.Helper()
	tr := tui1bRun(t, cmd)
	var out []copyDoneMsg
	for _, msg := range tr.msgs {
		if d, ok := msg.(copyDoneMsg); ok {
			out = append(out, d)
		}
	}
	if len(out) != 1 {
		t.Fatalf("前提不成立：产出里应当恰好 1 条 copyDoneMsg，实际 %d 条：%v", len(out), tr.log)
	}
	return out[0]
}

// TestTui1eCopyKeyTakesSelectionThenAll 是 §3 的前两条：有选区复制选区，没有选区复制全部；
// N 按 rune 数（"中文abc" 是 5 个 rune、9 个字节，按字节算会得到 9）。
func TestTui1eCopyKeyTakesSelectionThenAll(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		m, _ := tui1eNew(t, &tui1eCopySub{})
		m = tui1bType(t, m, "中文abc")
		if got := m.copyInputText(); got != "中文abc" {
			t.Fatalf("前提：没有选区时该复制的正文应当是 %q，实际 %q", "中文abc", got)
		}

		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, tui1eCopyKey)
		if cmd == nil || ccxIsQuit(cmd) {
			t.Errorf("复制应当交回一条写剪贴板的 Cmd（不执行），cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
		}
		atxWantHead(t, "复制全部", m, atxH("已复制 5 个字符"))
		atxWantValue(t, "复制全部（输入框不动）", m, "中文abc")
	})

	t.Run("selection", func(t *testing.T) {
		m, _ := tui1eNew(t, &tui1eCopySub{})
		m = tui1bType(t, m, "中文abc")
		for i := 0; i < 2; i++ {
			m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
		}
		if !m.ta.HasSelection() {
			t.Fatalf("前提不成立：Shift+← 两次之后应当有选区")
		}
		if got := m.copyInputText(); got != "bc" {
			t.Fatalf("有选区时该复制的正文应当是 %q，实际 %q", "bc", got)
		}

		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, tui1eCopyKey)
		if cmd == nil || ccxIsQuit(cmd) {
			t.Errorf("复制选区应当交回一条写剪贴板的 Cmd（不执行），cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
		}
		atxWantHead(t, "复制选区", m, atxH("已复制 2 个字符"))
		atxWantValue(t, "复制选区（不改文本，也不删选区）", m, "中文abc")
	})
}

// TestTui1eCopyKeyEmptyDoesNothing 钉住 §3 最后一条：空内容不写剪贴板、不改状态栏。
func TestTui1eCopyKeyEmptyDoesNothing(t *testing.T) {
	m, _ := tui1eNew(t, &tui1eCopySub{})
	m.status = "别的文案"

	m, cmd := tui1bStep(t, m, tui1eCopyKey)
	atxWantNilCmd(t, "空输入框上的 Ctrl+Shift+C", cmd)
	atxWantHead(t, "空输入框上的 Ctrl+Shift+C", m, atxH("别的文案"))
}

// TestTui1eCopyStatusExpires 钉住 §3 的过期规则：2 秒后恢复成复制前的文案，
// 期间再复制一次只认最后一次（旧代次的到期消息作废），状态栏被别的事件改写过就不写回。
func TestTui1eCopyStatusExpires(t *testing.T) {
	t.Run("restore", func(t *testing.T) {
		m, _ := tui1eNew(t, &tui1eCopySub{})
		m = tui1bType(t, m, "abcd")
		m, _ = tui1bStep(t, m, tui1eCopyKey)
		atxWantHead(t, "复制之后", m, atxH("已复制 4 个字符"))

		m, cmd := tui1bStep(t, m, copyStatusExpireMsg{gen: m.copyStatusGen})
		atxWantNilCmd(t, "到期", cmd)
		atxWantHead(t, "到期", m, atxH("空闲"))
	})

	t.Run("latest_wins", func(t *testing.T) {
		m, _ := tui1eNew(t, &tui1eCopySub{})
		m = tui1bType(t, m, "abcd")
		m, _ = tui1bStep(t, m, tui1eCopyKey)
		gen1 := m.copyStatusGen

		m = tui1bType(t, m, "e")
		m, _ = tui1bStep(t, m, tui1eCopyKey)
		gen2 := m.copyStatusGen
		if gen2 == gen1 {
			t.Fatalf("前提不成立：第二次复制应当换一个代次，两次都是 %d", gen1)
		}
		atxWantHead(t, "第二次复制之后", m, atxH("已复制 5 个字符"))

		// 旧代次的到期消息晚到：不能把新文案清掉。
		m, _ = tui1bStep(t, m, copyStatusExpireMsg{gen: gen1})
		atxWantHead(t, "旧代次到期", m, atxH("已复制 5 个字符"))

		// 当前代次到期才恢复，而且恢复的是第一次复制之前的文案。
		m, _ = tui1bStep(t, m, copyStatusExpireMsg{gen: gen2})
		atxWantHead(t, "新代次到期", m, atxH("空闲"))
	})

	t.Run("overwritten", func(t *testing.T) {
		m, _ := tui1eNew(t, &tui1eCopySub{})
		m = tui1bType(t, m, "abcd")
		m, _ = tui1bStep(t, m, tui1eCopyKey)
		gen := m.copyStatusGen

		// 别的事件改写了状态栏：过期 tick 不许把它盖回「空闲」。
		m, _ = tui1bStep(t, m, tea.PasteMsg{Content: "xy"})
		atxWantHead(t, "粘贴之后", m, atxH("粘贴了 2 个字符"))
		m, _ = tui1bStep(t, m, copyStatusExpireMsg{gen: gen})
		atxWantHead(t, "到期", m, atxH("粘贴了 2 个字符"))
	})
}

// TestTui1eCopyCommand 钉住 §4：/copy 在 TUI 本地执行（不进 Submit），
// 默认 1 条、/copy 3 三条、空会话提示、非法参数提示。
func TestTui1eCopyCommand(t *testing.T) {
	t.Run("default_one", func(t *testing.T) {
		sub := &tui1eCopySub{reply: "中文"}
		m, pr := tui1eNew(t, sub)
		m = tui1bType(t, m, "/copy")

		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, tui1bEnter())
		if m.busy {
			t.Errorf("/copy 不应当把界面置成忙：它只读库，没有一轮在跑")
		}
		atxWantValue(t, "回车之后输入框", m, "")
		if n := sub.count(); n != 0 {
			t.Errorf("/copy 不应当交给 Submit，实际 Submit %d 次（收到 %q）", n, sub.text(0))
		}
		atxWantHead(t, "回车之后", m, atxH("空闲"))

		m, cmd = tui1bStep(t, m, tui1eDone(t, cmd))
		if cmd == nil || ccxIsQuit(cmd) {
			t.Errorf("复制应当交回一条写剪贴板的 Cmd（不执行），cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
		}
		// 取数发生在命令 goroutine 上，所以参数断言放在跑完之后。
		if len(sub.args) != 1 || sub.args[0] != "" {
			t.Errorf("CopyRecent 收到的参数应当是 \"\"（不带参数 = 1 条），实际 %q", sub.args)
		}
		atxWantHead(t, "复制完成", m, atxH("已复制 2 个字符"))
		if got := pr.snapshot(); len(got) != 1 || got[0] != "> /copy" {
			t.Errorf("回显应当恰好是 [\"> /copy\"]，实际 %q", got)
		}
	})

	t.Run("three", func(t *testing.T) {
		sub := &tui1eCopySub{reply: "a\n\nb\n\nc"}
		m, _ := tui1eNew(t, sub)
		m = tui1bType(t, m, "/copy 3")

		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, tui1bEnter())
		m, _ = tui1bStep(t, m, tui1eDone(t, cmd))
		if len(sub.args) != 1 || sub.args[0] != "3" {
			t.Errorf("CopyRecent 收到的参数应当是 %q，实际 %q", "3", sub.args)
		}
		// "a\n\nb\n\nc" 是 7 个 rune，按字节算也是 7 —— 这条用例钉的是条数与拼接，
		// rune 计数由 TestTui1eCopyKeyTakesSelectionThenAll 的中文用例钉住。
		atxWantHead(t, "复制三条", m, atxH("已复制 7 个字符"))
	})

	t.Run("empty", func(t *testing.T) {
		sub := &tui1eCopySub{err: repl.ErrCopyEmpty}
		m, _ := tui1eNew(t, sub)
		m = tui1bType(t, m, "/copy")

		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, tui1bEnter())
		m, cmd = tui1bStep(t, m, tui1eDone(t, cmd))
		atxWantNilCmd(t, "没有可复制的消息", cmd)
		atxWantHead(t, "没有可复制的消息", m, atxH("没有可复制的消息"))
	})

	t.Run("usage", func(t *testing.T) {
		sub := &tui1eCopySub{err: repl.ErrCopyUsage}
		m, pr := tui1eNew(t, sub)
		m = tui1bType(t, m, "/copy -1")

		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, tui1bEnter())
		m, cmd = tui1bStep(t, m, tui1eDone(t, cmd))
		if len(sub.args) != 1 || sub.args[0] != "-1" {
			t.Errorf("CopyRecent 收到的参数应当是 %q，实际 %q", "-1", sub.args)
		}
		if cmd == nil {
			t.Errorf("参数非法应当打到回滚区一行用法，Cmd 不应当是 nil")
		}
		atxWantHead(t, "参数非法不改状态栏", m, atxH("空闲"))
		if got := pr.snapshot(); len(got) != 2 || got[0] != "> /copy -1" || got[1] != "用法: /copy [n]" {
			t.Errorf("回滚区应当恰好有 [\"> /copy -1\" \"用法: /copy [n]\"]，实际 %q", got)
		}
	})

	// 多行正文里的 /copy 不本地执行：本地执行会先把整段输入清掉，
	// 同一段正文的其余行会被这条命令一起吞掉（X44 的多行守卫要挡的就是这个形状）。
	t.Run("multiline_not_local", func(t *testing.T) {
		sub := &tui1eCopySub{reply: "不该被取走"}
		m, _ := tui1eNew(t, sub)
		m = tui1bType(t, m, "/copy 3")
		m, _ = tui1bStep(t, m, tui1bCtrl('j'))
		m = tui1bType(t, m, "第二行")

		_, cmd := tui1bStep(t, m, tui1bEnter())
		tui1bRun(t, cmd)
		if len(sub.args) != 0 {
			t.Errorf("多行正文不应当在 TUI 本地执行 /copy，实际 CopyRecent 被调 %d 次", len(sub.args))
		}
		if n := sub.count(); n != 1 {
			t.Fatalf("多行正文应当照旧交给 Submit，实际 Submit %d 次", n)
		}
		if got, want := sub.text(0), "/copy 3\n第二行"; got != want {
			t.Errorf("Submit 收到的应当是 %q，实际 %q", want, got)
		}
	})
}

// TestTui1eCopyThenCtrlCExpires 钉住复审报告的 bug：复制提示被 Ctrl-C 的到期消息永久写回。
//
// 复现顺序（复审给的）：copy → 一次 Ctrl-C（只武装）→ 两个到期 tick。
// 修之前：复制那条 tick 先到、发现状态栏已是武装提示就只清标记不恢复；Ctrl-C 的 tick
// 后到，把 ctrlCPrev 里记下的「已复制 N 个字符」写回来，而 copyStatusOn 已为 false，
// 那句话再也不会过期。修法见 copyStatusSettle：武装之前先把复制提示收回去。
// 两个 tick 的先后各走一遍：tea.Tick 的到达顺序没有保证，只钉一种会漏。
func TestTui1eCopyThenCtrlCExpires(t *testing.T) {
	for _, tc := range []struct {
		name        string
		copyTickFst bool
	}{
		{"copy_tick_first", true},
		{"ctrl_tick_first", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := tui1eNew(t, &tui1eCopySub{})
			m = tui1bType(t, m, "abcd")
			m, _ = tui1bStep(t, m, tui1eCopyKey)
			atxWantHead(t, "复制之后", m, atxH("已复制 4 个字符"))
			copyGen := m.copyStatusGen

			m, _ = tui1bStep(t, m, tui1bCtrl('c'))
			if !m.ctrlCArmed {
				t.Fatalf("前提不成立：第一次 Ctrl-C 之后应当处于武装状态")
			}
			atxWantHead(t, "武装之后", m, atxH("再按一次 Ctrl-C 退出"))
			ctrlGen := m.ctrlCGen

			copyTick := copyStatusExpireMsg{gen: copyGen}
			ctrlTick := ctrlCExpireMsg{gen: ctrlGen}
			if tc.copyTickFst {
				m, _ = tui1bStep(t, m, copyTick)
				m, _ = tui1bStep(t, m, ctrlTick)
			} else {
				m, _ = tui1bStep(t, m, ctrlTick)
				m, _ = tui1bStep(t, m, copyTick)
			}

			// 最终必须是复制前的文案，而且不能还停在「已复制 …」。
			atxWantHead(t, "两个 tick 走完", m, atxH("空闲"))
			if m.ctrlCArmed {
				t.Errorf("两个 tick 走完不应当还武装着")
			}
			if m.copyStatusOn {
				t.Errorf("两个 tick 走完之后不应当还挂着复制提示的标记")
			}
		})
	}

	// 同一形状在状态栏本来是别的文案时也要回到那句文案，而不是复制提示。
	t.Run("from_other_status", func(t *testing.T) {
		m, _ := tui1eNew(t, &tui1eCopySub{})
		m.status = "别的文案"
		m = tui1bType(t, m, "abcd")
		m, _ = tui1bStep(t, m, tui1eCopyKey)
		copyGen := m.copyStatusGen
		m, _ = tui1bStep(t, m, tui1bCtrl('c'))
		ctrlGen := m.ctrlCGen
		m, _ = tui1bStep(t, m, copyStatusExpireMsg{gen: copyGen})
		m, _ = tui1bStep(t, m, ctrlCExpireMsg{gen: ctrlGen})
		atxWantHead(t, "两个 tick 走完", m, atxH("别的文案"))
	})
}

// TestTui1eCopyEmptyKeepsArmed 钉住复审报告的第二个 bug：空复制不许撤掉 Ctrl-C 的武装。
//
// 21e §3 对空内容的要求是「不写剪贴板、不改状态栏」；解除武装会把
// 「再按一次 Ctrl-C 退出」换回旧文案，等于替用户取消了正在等的那次退出。
func TestTui1eCopyEmptyKeepsArmed(t *testing.T) {
	m, _ := tui1eNew(t, &tui1eCopySub{})
	m, _ = tui1bStep(t, m, tui1bCtrl('c'))
	if !m.ctrlCArmed {
		t.Fatalf("前提不成立：第一次 Ctrl-C 之后应当处于武装状态")
	}
	atxWantHead(t, "武装之后", m, atxH("再按一次 Ctrl-C 退出"))

	m, cmd := tui1bStep(t, m, tui1eCopyKey)
	atxWantNilCmd(t, "空输入框上的 Ctrl+Shift+C", cmd)
	if !m.ctrlCArmed {
		t.Errorf("空复制不应当解除 Ctrl-C 的武装：再按一次 Ctrl-C 会只重新武装、不退出")
	}
	atxWantHead(t, "空复制之后", m, atxH("再按一次 Ctrl-C 退出"))

	// 对照：同一装置上换有内容的输入框，复制照常发生，武装照旧被解除。
	m = tui1bType(t, m, "x")
	m, cmd = tui1bStep(t, m, tui1eCopyKey)
	if cmd == nil {
		t.Errorf("有内容时应当交回一条写剪贴板的 Cmd")
	}
	atxWantHead(t, "有内容时复制", m, atxH("已复制 1 个字符"))
	if m.ctrlCArmed {
		t.Errorf("有内容时复制应当解除 Ctrl-C 的武装")
	}
}
