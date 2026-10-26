package tui

// 本文件是 TUI-1（docs/21e_TUI1_TASK.md）的复制那一半：输入框里的 ctrl+shift+c、
// /copy 命令，以及两者共用的「已复制 N 个字符」状态栏。
//
// 两件事都要「写剪贴板 + 在状态栏说复制了多少字 + 2 秒后自己收回去」。
// 过期沿用 ctrlc.go 的那一套消息与代际计数，不另起一套定时器：同一时刻只有一条
// 复制提示在飞，上一条的 tick 必须能被下一条作废，否则旧 tick 会把新文案清掉。

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"council/internal/i18n"
	"council/internal/repl"
)

// copyStatusWindow 是「已复制 N 个字符」在状态栏停留的时长。
// 与 Ctrl-C 的 2 秒窗口同一个数：状态栏上同一个位置的两句临时提示，
// 时长不一样只会让人以为其中一句是卡住了。
const copyStatusWindow = 2 * time.Second

// copyStatusExpireMsg 是复制提示到期。gen 是写这条提示时的代次，到点原样带回来。
type copyStatusExpireMsg struct{ gen uint64 }

// copyStatusExpireCmd 装一拍到期。gen 是装的时候的代次，到点原样写进消息。
func copyStatusExpireCmd(gen uint64) tea.Cmd {
	return tea.Tick(copyStatusWindow, func(time.Time) tea.Msg {
		return copyStatusExpireMsg{gen: gen}
	})
}

// copyHint 是状态栏那句话。N 按 rune 数：一个汉字算 1，不按字节，
// 否则「已复制 6 个字符」对两个汉字是句看不懂的话（utf8.RuneCountInString）。
func copyHint(lang i18n.Lang, text string) string {
	return i18n.TF(lang, "tui.copy.hint", "已复制 %d 个字符", utf8.RuneCountInString(text))
}

// copyStatusShow 把状态栏换成复制提示，并装一拍到期。
//
// 记下旧文案的规则与 ctrlc.go 的 ctrlCArm 同一条：状态栏此刻已经是一条复制提示时，
// 不要把它当成「复制前的文案」记下来，否则到期恢复出来的是上一句
// 「已复制 N 个字符」，而那一句再也不会过期。代次加一让上一条提示的 tick 作废，
// 于是「2 秒内再复制一次」只认最后一次。
func (m Model) copyStatusShow(text string) (Model, tea.Cmd) {
	if !m.copyStatusOn || m.status != m.copyStatusText {
		m.copyStatusPrev = m.status
	}
	m.copyStatusOn = true
	m.copyStatusText = text
	m.copyStatusGen++
	m.status = text
	return m, copyStatusExpireCmd(m.copyStatusGen)
}

// copyStatusExpire 处理到期：代次对得上、状态栏仍是这条提示，才写回复制前的文案。
// 过期 tick 可能晚于下一条提示（代次对不上，丢掉），也可能晚于别的事件改写状态栏
// （"正在取消" 这类文案比一句过期的复制提示重要，不写回）。
func (m Model) copyStatusExpire(msg copyStatusExpireMsg) Model {
	if !m.copyStatusOn || msg.gen != m.copyStatusGen {
		return m
	}
	if m.status == m.copyStatusText {
		m.status = m.copyStatusPrev
	}
	m.copyStatusOn = false
	m.copyStatusText = ""
	m.copyStatusPrev = ""
	return m
}

// copyStatusSettle 在别的事件要「记下旧状态栏、稍后写回」之前，先把复制提示收回去。
//
// 为什么需要它：Ctrl-C 的武装会把当前状态栏记进 ctrlCPrev，到期再写回。状态栏恰好是
// 复制提示时，记下的就是一句 2 秒后该消失的文案；而复制那条 tick 先到、发现状态栏
// 已被改写就只清标记不恢复，随后 Ctrl-C 的 tick 把作废的文案写回来，于是
// 「已复制 N 个字符」永久挂在状态栏上，再也不会过期
// （复审复现的顺序：copy → Ctrl-C 一次 → 两个 tick）。
//
// 做法：状态栏仍是复制提示时写回复制前的文案，并作废还在飞的到期 tick（代次加一）。
// 这样别人记下的「旧状态栏」永远不是一句自己会消失的提示。幂等：没在显示复制提示时
// 什么都不做。
func (m Model) copyStatusSettle() Model {
	if !m.copyStatusOn {
		return m
	}
	if m.status == m.copyStatusText {
		m.status = m.copyStatusPrev
	}
	m.copyStatusOn = false
	m.copyStatusText = ""
	m.copyStatusPrev = ""
	m.copyStatusGen++
	return m
}

// isCopyInputKey 报告这个键是不是「复制输入框里的字」：Ctrl+Shift+C。
//
// 键位与 textarea 的默认 CopySelection 同一个（19c 契约 §0 R5 冻结：这个形状的
// String() 是 "ctrl+shift+c"）。New 里已经把那个绑定解掉了，所以这条判断是唯一入口：
// 自己取文本、自己发 tea.SetClipboard，才能顺带给出「复制了多少字」的反馈。
func isCopyInputKey(msg tea.KeyPressMsg) bool {
	return msg.Code == 'c' && msg.Mod.Contains(tea.ModCtrl) && msg.Mod.Contains(tea.ModShift)
}

// copyInputText 返回这一次复制该拿走的文本：有选区取选区，没有选区取整段输入。
// 独立成方法是为了让用例能在不碰剪贴板的前提下核对正文（见 tui1e_copy_test.go）。
func (m Model) copyInputText() string {
	if text := m.ta.SelectedText(); text != "" {
		return text
	}
	return m.ta.Value()
}

// copyInput 处理输入框里的 ctrl+shift+c，text 是这次要复制的正文。
//
// 空内容时什么都不做：不写剪贴板、也不改状态栏。写一个空串到剪贴板会把用户
// 之前复制的东西抹掉，而屏幕上没有任何理由说明它为什么没了。
// 调用方在调它之前不要碰 Ctrl-C 的武装：空内容连状态栏都不许改，解除武装会把
// 「再按一次 Ctrl-C 退出」换回旧文案（见 updateInput 里那一支）。
func (m Model) copyInput(text string) (tea.Model, tea.Cmd) {
	if text == "" {
		return m, nil
	}
	var expire tea.Cmd
	m, expire = m.copyStatusShow(copyHint(m.lang, text))
	return m, tea.Batch(tea.SetClipboard(text), expire)
}

// copySource 是 Submitter 可以另外实现的能力：取最近 n 条可复制的会话正文。
//
// 不并进 Submitter：既有的假 Submitter 都没有这个方法。没实现时 /copy 交给 REPL 的
// 既有路径（与行式入口同一条，答复是「行式入口没有剪贴板」），不会静默吞掉这条命令。
type copySource interface {
	CopyRecent(ctx context.Context, args string) (string, error)
}

// copyDoneMsg 是 TUI 本地执行的 /copy 的结果。text 是要写剪贴板的正文；
// err 非 nil 时是要显示给用户的一行（ErrCopyEmpty 放状态栏，其余打到回滚区）。
type copyDoneMsg struct {
	text string
	err  error
}

// copyCmd 在命令 goroutine 上向 REPL 取正文。
// 不在 Update 里读库：History 会走 SQLite，在事件循环上查一次就能把打字卡住。
func copyCmd(src copySource, ctx context.Context, args string) tea.Cmd {
	return func() tea.Msg {
		text, err := src.CopyRecent(ctx, args)
		return copyDoneMsg{text: text, err: err}
	}
}

// copyCommandArgs 报告 text 是不是一条可以在 TUI 本地执行的 /copy，以及命令名之后的参数。
//
// 只认整段恰好是这一行的情况：多行提交里出现 /copy 时交给 REPL 的既有路径。
// 本地执行要先把输入框清空，若让多行正文里的一条 /copy 走这里，同一段正文的其余行
// 会被这条命令一起吞掉 —— 那正是 X44 的多行守卫要挡的形状。
func copyCommandArgs(text string) (string, bool) {
	if strings.Contains(text, "\n") {
		return "", false
	}
	trimmed := strings.TrimSpace(text)
	fields := strings.Fields(trimmed)
	if len(fields) == 0 || fields[0] != "/copy" {
		return "", false
	}
	// fields[0] 只可能是 ASCII 的 "/copy"，字节切片安全。
	return strings.TrimSpace(trimmed[len(fields[0]):]), true
}

// beginCopy 尝试把这次提交当成 /copy 在本地执行。
// cmds 是已经拼好的回显命令；ok 为 false 时调用方按普通提交继续
// （不是 /copy，或者这个 Submitter 没有剪贴板能力）。
//
// 不进忙态：这一条只读库，没有一轮在跑，状态栏留给 copyDoneMsg 换成「已复制 N 个字符」。
//
// /copy --screen 是复制模式（copyscreen.go）的第二条入口，与 Ctrl+G 走同一个
// startCopyScreen。参数形状仍由上面的 copyCommandArgs 一处解析。输入框照其余命令
// 清空：命令已经按回车送出去了，退出复制模式之后它不该又冒回来。
func (m Model) beginCopy(text string, cmds []tea.Cmd) (tea.Model, tea.Cmd, bool) {
	args, ok := copyCommandArgs(text)
	if !ok {
		return m, nil, false
	}
	if args == "--screen" {
		// 没有复制模式能力的 Submitter（测试里的假件）落到下面既有的 /copy 路径：
		// "--screen" 会被当成非法条数，答复与今天逐字相同。
		if _, ok := m.sub.(copyScreenSource); ok {
			m.ta.Reset()
			m.at = atState{}
			next, load := m.startCopyScreen()
			if load == nil {
				return next, tea.Sequence(cmds...), true
			}
			return next, tea.Sequence(append(cmds, load)...), true
		}
	}
	src, ok := m.sub.(copySource)
	if !ok {
		return m, nil, false
	}
	m.ta.Reset()
	m.at = atState{}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return m, tea.Sequence(append(cmds, copyCmd(src, ctx, args))...), true
}

// copyDone 处理 /copy 的结果。
//
// 先把 Ctrl-C 的武装解除：命令结束是一次「别的事件」，而复制提示会改写状态栏，
// 留着武装的话到期恢复出来的是一句已经作废的「再按一次 Ctrl-C …」。
func (m Model) copyDone(msg copyDoneMsg) (tea.Model, tea.Cmd) {
	m = m.ctrlCDisarm()
	if msg.err != nil {
		if errors.Is(msg.err, repl.ErrCopyEmpty) {
			m.status = msg.err.Error()
			return m, nil
		}
		// 参数非法与读库失败按命令错误的既有体例打到回滚区，不退出、不改状态栏。
		return m, m.println(msg.err.Error())
	}
	var expire tea.Cmd
	m, expire = m.copyStatusShow(copyHint(m.lang, msg.text))
	return m, tea.Batch(tea.SetClipboard(msg.text), expire)
}
