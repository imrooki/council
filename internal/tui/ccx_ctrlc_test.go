package tui

// ccx_ctrlc_test.go 钉住 Ctrl-C 连按两次才退出或取消。纯 Model 驱动，不起 Program。
// 到期不执行武装返回的 tea.Tick，直接送 ctrlCExpireMsg。

import (
	"context"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type ccxSub struct{}

func (ccxSub) Submit(context.Context, string) error { return nil }
func (ccxSub) IsQuit(error) bool                    { return false }

func ccxCtrlC() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl} }

func ccxIsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	return reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

func ccxNew(t *testing.T, now *time.Time) Model {
	t.Helper()
	m := New(Options{Submit: ccxSub{}, Now: func() time.Time { return *now }})
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func ccxWant(t *testing.T, step string, m Model, status string, armed bool) {
	t.Helper()
	if m.status != status || m.ctrlCArmed != armed {
		t.Errorf("%s：status 期望 %q 实际 %q；armed 期望 %v 实际 %v", step, status, m.status, armed, m.ctrlCArmed)
	}
}

// TestCcxIdleDoubleCtrlCQuits 钉住语义 3：空闲第一次只武装，窗口内第二次退出。
func TestCcxIdleDoubleCtrlCQuits(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m, cmd := tui1bStep(t, m, ccxCtrlC())
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("第 1 步：第一次 Ctrl-C 的 Cmd 应当非 nil 且不是退出，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	ccxWant(t, "第 1 步", m, "再按一次 Ctrl-C 退出", true)

	now = now.Add(time.Second)
	m, cmd = tui1bStep(t, m, ccxCtrlC())
	if !ccxIsQuit(cmd) {
		t.Errorf("第 2 步：窗口内第二次应当退出（cmd == nil：%v）", cmd == nil)
	}
	ccxWant(t, "第 2 步", m, "正在退出", false)
}

// TestCcxWindowBoundary 钉住语义 5 的 <=，以及超窗重新武装后到期恢复真正的旧状态。
func TestCcxWindowBoundary(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m, _ = tui1bStep(t, m, ccxCtrlC())
	now = now.Add(ctrlCWindow)
	m, cmd := tui1bStep(t, m, ccxCtrlC())
	if !ccxIsQuit(cmd) {
		t.Errorf("第 1 步：间隔恰好 2s 应当退出（cmd == nil：%v）", cmd == nil)
	}
	ccxWant(t, "第 1 步", m, "正在退出", false)

	now = time.Date(2026, 10, 4, 1, 0, 0, 0, time.UTC)
	m = ccxNew(t, &now)
	m, _ = tui1bStep(t, m, ccxCtrlC())
	now = now.Add(ctrlCWindow + time.Nanosecond)
	m, cmd = tui1bStep(t, m, ccxCtrlC())
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("第 2 步：超出 1ns 不应当退出，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	ccxWant(t, "第 2 步", m, "再按一次 Ctrl-C 退出", true)
	m, cmd = tui1bStep(t, m, ctrlCExpireMsg{gen: m.ctrlCGen})
	if cmd != nil {
		t.Errorf("第 3 步：到期 Cmd 应当是 nil")
	}
	ccxWant(t, "第 3 步", m, "空闲", false)
}

// TestCcxExpireDisarms 钉住语义 6a：当前代次到期解除；旧代次忽略；解除后再按一次只武装。
func TestCcxExpireDisarms(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m, _ = tui1bStep(t, m, ccxCtrlC())
	m, _ = tui1bStep(t, m, ctrlCExpireMsg{gen: m.ctrlCGen})
	ccxWant(t, "第 1 步", m, "空闲", false)
	m, cmd := tui1bStep(t, m, ccxCtrlC())
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("第 2 步：到期后再按一次应当只武装，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	ccxWant(t, "第 2 步", m, "再按一次 Ctrl-C 退出", true)

	m = ccxNew(t, &now)
	m, _ = tui1bStep(t, m, ccxCtrlC())
	m, cmd = tui1bStep(t, m, ctrlCExpireMsg{gen: m.ctrlCGen - 1})
	if cmd != nil {
		t.Errorf("第 3 步：旧代次到期 Cmd 应当是 nil")
	}
	ccxWant(t, "第 3 步", m, "再按一次 Ctrl-C 退出", true)
}

// TestCcxOtherKeyDisarms 钉住语义 6b：普通键解除并恢复，再按一次只武装。
func TestCcxOtherKeyDisarms(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m, _ = tui1bStep(t, m, ccxCtrlC())
	m, _ = tui1bStep(t, m, tui1bKey('a'))
	ccxWant(t, "第 1 步", m, "空闲", false)
	if got := m.ta.Value(); got != "a" {
		t.Errorf("第 1 步：输入框期望 %q，实际 %q", "a", got)
	}
	m, cmd := tui1bStep(t, m, ccxCtrlC())
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("第 2 步：再按一次应当只武装，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	ccxWant(t, "第 2 步", m, "再按一次 Ctrl-C 退出", true)
}

// TestCcxSelectToolDoesNotQuit 只钉住 Model 层：Ctrl+Insert 作为「别的键」会解除武装，
// 所以「Ctrl-C、Ctrl+Insert、Ctrl-C」不退出。
// 背景：Cherry Studio 划词助手（selection-hook）每次取词先发 Ctrl+Insert，剪贴板约 100ms 没变化再发一次 Ctrl+C。
// 真实终端里这串序列会不会原样到达程序没有实测。xterm.js 源码显示带 Ctrl 或 Shift 的 Insert 不发任何字节
// （VS Code 的终端基于 xterm.js，未在用户的 VS Code 上实测）；若如此，在 VS Code 里每次划词只到一个 Ctrl-C，
// 2 秒内划词两次仍会退出（忙时会取消）。
// selection-hook 支持按程序名过滤（setGlobalFilterMode），在划词工具里排除终端程序可从源头避免；
// Cherry Studio 设置里是否提供、叫什么，以实物为准。
func TestCcxSelectToolDoesNotQuit(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	insert := tea.KeyPressMsg{Code: tea.KeyInsert, Mod: tea.ModCtrl}
	var cmd tea.Cmd
	for i, key := range []tea.KeyPressMsg{ccxCtrlC(), insert, ccxCtrlC()} {
		now = now.Add(100 * time.Millisecond)
		m, cmd = tui1bStep(t, m, key)
		if ccxIsQuit(cmd) {
			t.Errorf("第 %d 步：不应当退出", i+1)
		}
	}
	ccxWant(t, "第 3 步", m, "再按一次 Ctrl-C 退出", true)
}

// TestCcxPasteDisarms 钉住语义 6c：粘贴解除并改状态栏，再按一次只武装。
func TestCcxPasteDisarms(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m, _ = tui1bStep(t, m, ccxCtrlC())
	m, _ = tui1bStep(t, m, tea.PasteMsg{Content: "xy"})
	ccxWant(t, "第 1 步", m, "粘贴了 2 个字符", false)
	m, cmd := tui1bStep(t, m, ccxCtrlC())
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("第 2 步：再按一次应当只武装，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	ccxWant(t, "第 2 步", m, "再按一次 Ctrl-C 退出", true)
}

// TestCcxBusyCancelAndEnter 钉住语义 4 与 7：忙时连按两次才取消，第三次不动；Enter 换新提示。
func TestCcxBusyCancelAndEnter(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m.busy = true
	m.status = "正在运行"
	n := 0
	m.cancel = func() { n++ }

	m, cmd := tui1bStep(t, m, ccxCtrlC())
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("第 1 步：第一次应当只武装，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	if n != 0 || m.canceling {
		t.Errorf("第 1 步：cancel 期望 0 实际 %d；canceling 期望 false 实际 %v", n, m.canceling)
	}
	ccxWant(t, "第 1 步", m, "再按一次 Ctrl-C 取消本轮", true)

	now = now.Add(time.Second)
	m, cmd = tui1bStep(t, m, ccxCtrlC())
	if cmd != nil {
		t.Errorf("第 2 步：第二次取消的 Cmd 应当是 nil")
	}
	if n != 1 || !m.canceling {
		t.Errorf("第 2 步：cancel 期望 1 实际 %d；canceling 期望 true 实际 %v", n, m.canceling)
	}
	ccxWant(t, "第 2 步", m, "正在取消", false)

	m, cmd = tui1bStep(t, m, ccxCtrlC())
	if cmd != nil || n != 1 || !m.canceling {
		t.Errorf("第 3 步：应当什么都不做，cmd == nil：%v，cancel 期望 1 实际 %d，canceling %v", cmd == nil, n, m.canceling)
	}
	ccxWant(t, "第 3 步", m, "正在取消", false)

	m = ccxNew(t, &now)
	m.busy = true
	m.status = "正在运行"
	m, cmd = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("第 4 步：忙时 Enter 的 Cmd 应当是 nil")
	}
	ccxWant(t, "第 4 步", m, "一轮正在跑，先等它回来或连按两次 Ctrl-C 取消", false)
}

// TestCcxSubmitDoneClearsArm 钉住语义 6d：忙时武装后 submitDone 解除，之后单按只武装退出。
func TestCcxSubmitDoneClearsArm(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m.busy = true
	m.status = "正在运行"
	m, _ = tui1bStep(t, m, ccxCtrlC())
	m, cmd := tui1bStep(t, m, submitDoneMsg{})
	if cmd != nil || m.busy {
		t.Errorf("第 1 步：submitDone 后应当不忙且 Cmd 为 nil，busy %v，cmd == nil：%v", m.busy, cmd == nil)
	}
	ccxWant(t, "第 1 步", m, "空闲", false)

	m, cmd = tui1bStep(t, m, ccxCtrlC())
	if cmd == nil || ccxIsQuit(cmd) {
		t.Errorf("第 2 步：再按一次应当只武装退出，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
	}
	ccxWant(t, "第 2 步", m, "再按一次 Ctrl-C 退出", true)
}

// TestCcxFinalizingBlocks 钉住语义 2：收口窗口里 Ctrl-C 被挡，不武装。
func TestCcxFinalizingBlocks(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m.finalizing = true
	m.status = "收口中"
	m, cmd := tui1bStep(t, m, ccxCtrlC())
	if cmd != nil {
		t.Errorf("第 1 步：Cmd 应当是 nil")
	}
	ccxWant(t, "第 1 步", m, "收口中", false)
}

// TestCcxOneCommandHintRestored 钉住语义 6a：oneCommandHint 被第一次 Ctrl-C 换掉，到期后恢复。
func TestCcxOneCommandHintRestored(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m.status = oneCommandHint
	m, _ = tui1bStep(t, m, ccxCtrlC())
	ccxWant(t, "第 1 步", m, "再按一次 Ctrl-C 退出", true)
	m, _ = tui1bStep(t, m, ctrlCExpireMsg{gen: m.ctrlCGen})
	ccxWant(t, "第 2 步", m, oneCommandHint, false)
}

// TestCcxOtherKeysRearm 钉住语义 6b：Enter、Esc、Ctrl-J 解除武装，随后窗口内的 Ctrl-C 只重新武装。
func TestCcxOtherKeysRearm(t *testing.T) {
	keys := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}},
		{"ctrlj", tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}},
	}
	for _, k := range keys {
		t.Run(k.name, func(t *testing.T) {
			now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
			m := ccxNew(t, &now)
			m, _ = tui1bStep(t, m, ccxCtrlC())
			now = now.Add(100 * time.Millisecond)
			m, _ = tui1bStep(t, m, k.key)
			if m.ctrlCArmed {
				t.Errorf("%s：这一键应当解除武装", k.name)
			}
			now = now.Add(100 * time.Millisecond)
			m, cmd := tui1bStep(t, m, ccxCtrlC())
			if cmd == nil || ccxIsQuit(cmd) {
				t.Errorf("%s：随后的 Ctrl-C 应当只重新武装，cmd == nil：%v，是退出：%v", k.name, cmd == nil, ccxIsQuit(cmd))
			}
			ccxWant(t, k.name, m, "再按一次 Ctrl-C 退出", true)
		})
	}
}

// TestCcxBusyExpireRestores 钉住忙时到期：状态栏恢复成武装前的「正在运行」。
func TestCcxBusyExpireRestores(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	m := ccxNew(t, &now)
	m.busy = true
	m.status = "正在运行"
	m, _ = tui1bStep(t, m, ccxCtrlC())
	m, _ = tui1bStep(t, m, ctrlCExpireMsg{gen: m.ctrlCGen})
	ccxWant(t, "到期", m, "正在运行", false)
}

// TestCcxModifierGhostKeepsArm 钉住修饰键残影不解除武装：中间那一键之后窗口内的 Ctrl-C 仍是第二次。
func TestCcxModifierGhostKeepsArm(t *testing.T) {
	ghosts := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"nul_space", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl}},
		{"nul_at", tea.KeyPressMsg{Code: '@', Mod: tea.ModCtrl}},
		{"left_ctrl", tea.KeyPressMsg{Code: tea.KeyLeftCtrl}},
		{"nul_numlock", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl | tea.ModNumLock}},
	}
	for _, g := range ghosts {
		t.Run(g.name, func(t *testing.T) {
			now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
			m := ccxNew(t, &now)
			var cmd tea.Cmd
			for _, key := range []tea.KeyPressMsg{ccxCtrlC(), g.key, ccxCtrlC()} {
				now = now.Add(100 * time.Millisecond)
				m, cmd = tui1bStep(t, m, key)
			}
			if !ccxIsQuit(cmd) {
				t.Errorf("%s：第三键应当退出（cmd == nil：%v）", g.name, cmd == nil)
			}
			ccxWant(t, g.name, m, "正在退出", false)
		})
	}

	t.Run("busy_nul", func(t *testing.T) {
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		m := ccxNew(t, &now)
		m.busy = true
		m.status = "正在运行"
		n := 0
		m.cancel = func() { n++ }
		ghost := tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl}
		for _, key := range []tea.KeyPressMsg{ccxCtrlC(), ghost, ccxCtrlC()} {
			now = now.Add(100 * time.Millisecond)
			m, _ = tui1bStep(t, m, key)
		}
		if n != 1 {
			t.Errorf("忙态：cancel 期望 1，实际 %d", n)
		}
		ccxWant(t, "忙态", m, "正在取消", false)
	})

	t.Run("letter_disarms", func(t *testing.T) {
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		m := ccxNew(t, &now)
		var cmd tea.Cmd
		for _, key := range []tea.KeyPressMsg{ccxCtrlC(), tui1bKey('a'), ccxCtrlC()} {
			now = now.Add(100 * time.Millisecond)
			m, cmd = tui1bStep(t, m, key)
		}
		if cmd == nil || ccxIsQuit(cmd) {
			t.Errorf("普通字符应当解除武装，第三键只重新武装，cmd == nil：%v，是退出：%v", cmd == nil, ccxIsQuit(cmd))
		}
		ccxWant(t, "普通字符", m, "再按一次 Ctrl-C 退出", true)
	})

	disarms := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"ctrl_shift_space", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl | tea.ModShift}},
		{"alt_space", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModAlt}},
		{"ctrl_space_text", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl, Text: " "}},
	}
	for _, d := range disarms {
		t.Run(d.name, func(t *testing.T) {
			now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
			m := ccxNew(t, &now)
			var cmd tea.Cmd
			for _, key := range []tea.KeyPressMsg{ccxCtrlC(), d.key, ccxCtrlC()} {
				now = now.Add(100 * time.Millisecond)
				m, cmd = tui1bStep(t, m, key)
			}
			if cmd == nil || ccxIsQuit(cmd) {
				t.Errorf("%s：应当解除武装，第三键只重新武装，cmd == nil：%v，是退出：%v", d.name, cmd == nil, ccxIsQuit(cmd))
			}
			ccxWant(t, d.name, m, "再按一次 Ctrl-C 退出", true)
		})
	}
}
