package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// ctrlCWindow 是连按两次 Ctrl-C 的窗口。第二次按下与第一次的间隔 <= 它才生效。
const ctrlCWindow = 2 * time.Second

// ctrlCKind 是这一次武装要等的第二次动作：退出，或取消本轮。
type ctrlCKind int

const (
	ctrlCNone ctrlCKind = iota
	ctrlCQuit
	ctrlCCancel
)

// ctrlCExpireMsg 是窗口到期。gen 是装这一拍时的武装代次，到点原样写进消息。
type ctrlCExpireMsg struct {
	gen uint64
}

// ctrlCArmHint 是武装时写进状态栏的那句话。
func ctrlCArmHint(kind ctrlCKind) string {
	if kind == ctrlCCancel {
		return "再按一次 Ctrl-C 取消本轮"
	}
	return "再按一次 Ctrl-C 退出"
}

// ctrlCClock 取武装用的时刻：m.now 非 nil 用它，否则 time.Now。
func (m Model) ctrlCClock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// ctrlCArm 武装成 kind：记下当前状态栏、换成提示、记时刻、代次加一。
// 已经武装且状态栏仍是提示时，记下的旧状态保持不变。
// 超窗后到期消息还没到，再按一次会重新武装；这时状态栏还是提示，不能把它当成要恢复的旧状态。
//
// 开头先把复制提示收回去（copyStatusSettle）：它是一句 2 秒后要自己消失的文案，
// 记进 ctrlCPrev 之后，复制那条 tick 会以为状态栏已被改写而不再恢复，随后本条的
// tick 又把作废的文案写回来 —— 「已复制 N 个字符」就永久挂在状态栏上了。
func (m Model) ctrlCArm(kind ctrlCKind) Model {
	m = m.copyStatusSettle()
	if !m.ctrlCArmed || m.status != ctrlCArmHint(m.ctrlCKind) {
		m.ctrlCPrev = m.status
	}
	m.ctrlCArmed = true
	m.ctrlCKind = kind
	m.ctrlCAt = m.ctrlCClock()
	m.ctrlCGen++
	m.status = ctrlCArmHint(kind)
	return m
}

// ctrlCDisarm 解除武装。状态栏仍是这次武装的提示时，恢复成记下的旧状态。
func (m Model) ctrlCDisarm() Model {
	if m.ctrlCArmed && m.status == ctrlCArmHint(m.ctrlCKind) {
		m.status = m.ctrlCPrev
	}
	m.ctrlCArmed = false
	m.ctrlCKind = ctrlCNone
	m.ctrlCPrev = ""
	return m
}

// ctrlCClear 只解除武装，不碰状态栏。提交结束那一支每条路径都会自己设 status。
func (m Model) ctrlCClear() Model {
	m.ctrlCArmed = false
	m.ctrlCKind = ctrlCNone
	m.ctrlCPrev = ""
	return m
}

// ctrlCFresh 报告这次按下是不是窗口内、且种类相符的第二次。
// 未武装、已超窗、或武装种类与当前忙闲不符，都不是。
func (m Model) ctrlCFresh(kind ctrlCKind) bool {
	if !m.ctrlCArmed || m.ctrlCKind != kind {
		return false
	}
	return m.ctrlCClock().Sub(m.ctrlCAt) <= ctrlCWindow
}

// ctrlCKeepsArm 报告这个键是不是修饰键残影：不算「别的键」，不解除武装。
//
// github.com/charmbracelet/ultraviolet（go.mod 锁定 v0.0.0-20260811164956-006e29f97886）
// 在 Windows 上开 ENABLE_VIRTUAL_TERMINAL_INPUT 后，对每条按下记录只取其字符
// （terminal_reader_windows.go:104-106 的 buf.WriteRune(kevent.Char)；行号随依赖升级会漂）。
// 单独按下的 Ctrl、Shift、Alt 若也送来一条按下记录，字符是 0，被写成 NUL。解码器把 NUL 解成
// KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl}（decoder.go:1128-1132；设了 flagCtrlAt 时是
// {Code: '@', Mod: tea.ModCtrl}）。按住 Ctrl 的自动重复也会产生这种 NUL。
// 若把它当成别的键解除武装，「按 Ctrl-C、松开 Ctrl、再按 Ctrl-C」时第二次按 Ctrl 会先撤掉武装，
// 用户永远退不出，忙时也取消不了。左右修饰键本身同样不算别的键。
// 代价：用户真按的 Ctrl+Space、Ctrl+2、Ctrl+@ 与残影无法区分，也一并不解除武装；
// 它们本身不退出也不取消，之后 2 秒内再按 Ctrl-C 仍算第二次，与用户连按两次 Ctrl-C 的意图一致。
func ctrlCKeepsArm(msg tea.KeyPressMsg) bool {
	switch msg.Code {
	case tea.KeyLeftShift, tea.KeyLeftAlt, tea.KeyLeftCtrl, tea.KeyLeftSuper, tea.KeyLeftHyper, tea.KeyLeftMeta,
		tea.KeyRightShift, tea.KeyRightAlt, tea.KeyRightCtrl, tea.KeyRightSuper, tea.KeyRightHyper, tea.KeyRightMeta,
		tea.KeyIsoLevel3Shift, tea.KeyIsoLevel5Shift:
		return true
	}
	if msg.Text != "" {
		return false
	}
	if msg.Code != tea.KeySpace && msg.Code != '@' {
		return false
	}
	locks := tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	return msg.Mod&^locks == tea.ModCtrl
}

// ctrlCExpireCmd 装一拍到期。gen 是装的时候的代次，到点原样写进消息。
func ctrlCExpireCmd(gen uint64) tea.Cmd {
	return tea.Tick(ctrlCWindow, func(time.Time) tea.Msg {
		return ctrlCExpireMsg{gen: gen}
	})
}

// ctrlCPress 是 Ctrl-C 的本体：普通输入与复制模式（copyscreen.go）两条入口共用。
//
// 抽出来只为让复制模式复用同一段，21e 的语义一条不改：收口窗口里照旧挡住、
// 不武装；其余情况要 2 秒内连按两次，第一次只武装并改状态栏，第二次才退出
// 或取消本轮；已经取消过的这一轮再按也不重新武装。
func (m Model) ctrlCPress() (Model, tea.Cmd) {
	if m.finalizing {
		return m, nil
	}
	if m.busy {
		if m.canceling {
			return m, nil
		}
		if m.ctrlCFresh(ctrlCCancel) {
			m = m.ctrlCDisarm()
			if m.cancel != nil {
				m.cancel()
			}
			m.status = "正在取消"
			m.canceling = true
			return m, nil
		}
		m = m.ctrlCArm(ctrlCCancel)
		return m, ctrlCExpireCmd(m.ctrlCGen)
	}
	if m.ctrlCFresh(ctrlCQuit) {
		m = m.ctrlCDisarm()
		m.status = "正在退出"
		return m, m.quitCmd()
	}
	m = m.ctrlCArm(ctrlCQuit)
	return m, ctrlCExpireCmd(m.ctrlCGen)
}
