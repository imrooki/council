package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"council/internal/i18n"
	"council/internal/repl"
)

// Submitter 是 Model 需要的 REPL 能力。
// 用接口隔开具体 REPL，测试时不必启动它依赖的存储层。
type Submitter interface {
	Submit(ctx context.Context, text string) error
	IsQuit(err error) bool
}

// Drainer 是 Model 退出前需要的输出缓冲能力。
type Drainer interface {
	SetEmit(EmitFunc)
	Flush()
}

// submitDoneMsg 把后台提交的结果送回事件循环。
// members 是提交之后在同一个 goroutine 上取的 @ 补全候选（见 submitCmd）；err 非 nil 时不取，为 nil。
type submitDoneMsg struct {
	err     error
	members []repl.MentionCandidate
}

// mentionSource 是 Submitter 可以另外实现的能力：列出 @ 补全的候选。
// 不并进 Submitter：既有的假 Submitter 都没有这个方法，没实现的一律当作没有候选。
type mentionSource interface {
	MentionCandidates(ctx context.Context) ([]repl.MentionCandidate, error)
}

// atState 是输入框 @ 补全的状态（00_DECISIONS #47 补充，AT1）。零值是「没有在补全」。
// row、col 是那个 @ 在 textarea 里的行与 rune 下标；prefix 是 @ 与光标之间的字，原样；
// sel 是选中项在筛选结果里的下标。
type atState struct {
	armed  bool
	row    int
	col    int
	prefix string
	sel    int
}

// atMaxRows 是补全列表最多显示的候选行数。
const atMaxRows = 5

// Options 是 New 的入参。
type Options struct {
	Submit  Submitter
	Out     Drainer
	Version string

	// Println 把一行文本变成推进终端原生回滚区的命令。
	Println func(string) tea.Cmd

	// Ctx 是每条命令的 ctx 的父。nil 时退化成 context.Background()。
	// 必须传 tea.WithContext 用的那个根 ctx：根被取消时，
	// 在飞命令的 ctx 要跟着取消，才会走到 spawn.go 的 killProcessTree。
	Ctx context.Context

	// Now 是表头计时用的时钟。nil 时用 time.Now；测试注入假钟用。
	Now func() time.Time

	// Lang 是界面语言，由 app 经 RunLang 注入；零值按中文处理。
	// 状态栏、复制提示、空态这些短句都经 Model.t 取，缺翻译时回落到
	// 中文原文，不会把 tui.* 的内部 key 打给用户。
	Lang i18n.Lang
}

// Model 是 Council 的内联终端界面状态。
// 方法使用值接收者，每次 Update 都返回更新后的值。
type Model struct {
	// ctx 是每条命令的 ctx 的父，来自 Options.Ctx。
	// nil 在 New 里填成 context.Background()，beginSubmit 里再兜一次。
	//
	// 调用方必须传 tea.WithContext 用的那个根 ctx。根被取消
	// （SIGTERM、关窗）时，这里派生出去的 runCtx 跟着取消，
	// spawn.go 的 ctx.Done 分支才会跑 killProcessTree。
	// Windows 上父进程死了不会带走子进程；这一环断了，
	// provider CLI 变成孤儿，跑满 5 分钟硬超时，继续烧 token，
	// 且这笔钱对不上任何记录。
	ctx context.Context

	sub     Submitter
	out     Drainer
	version string
	println func(string) tea.Cmd

	ta       textarea.Model
	focusCmd tea.Cmd

	width  int
	height int
	// termHeight 是最近一条 WindowSizeMsg 带来的真终端高度。
	// 那条消息的宽或高 <= 0 时它是 0。上面的 height 会被兜底成 24，不能拿来判「终端高度已知」。
	// 用在三处：退出清屏（run.go 的 exitClearSeq）、看板行数上限（boardLimit）、@ 补全列表的行数（atLines）。
	// 后两处决定 View 画几行：它为 0 时看板行数不再增长、补全列表不上屏。
	termHeight int
	busy       bool
	cancel     context.CancelFunc
	status     string

	// now 是表头计时的时钟，来自 Options.Now。nil 在 New 里填成 time.Now，
	// beginSubmit 里再兜一次。
	now timeFunc
	// runStart 是本次提交取到的起点。已用时长是 tick 时刻减去它，不按拍累加。
	runStart time.Time
	// elapsed 是最近一拍算出来的已用时长。提交时清零，不忙时不印。
	elapsed time.Duration
	// tickGen 是本次提交的 tick 代次。旧代次的拍丢掉。
	tickGen uint64
	// tickPending 为 true 表示这一次提交还没装过 tick。
	// 装在提交之后的下一条 Update 上，不装在 beginSubmit 返回的 Cmd 里。
	tickPending bool
	// run 是本次提交的当前段。beginSubmit 与 submitDone 时清空；RoundEnd 不清，
	// 段一直留到下一段的 RoundStart 或这次提交结束。
	run roundRun

	// rows 是当前段的看板行，顺序跟名单。改某一行之前先整表拷贝。
	rows []boardRow
	// segStart 是收到本段带名单的 RoundStart 时的时钟。在跑行从这里起算。
	segStart time.Time
	// floor 是本次提交里看板已占的行数。只在带名单的 RoundStart 上增长；正常路径上只在收口时归零。
	// 例外：出错或退出那条 Sequence 的两次 Send 之间若进来一次 Enter，beginSubmit 会把它清零、
	// 帧在没钉光标的情况下变矮（RB-A 起就不挡这个间隙里的 Enter，见 00_DECISIONS #41 RB-B 落地记录）。
	floor int
	// runSeen 为真表示本次提交收到过忙时、带非空名单的 RoundStart。
	runSeen bool
	// backN 是本次提交里 MemberDone 的累计次数，跨段相加。
	backN int
	// rosterN 是本次提交里各段名单长度的总和。
	rosterN int
	// canceling 为真表示本次提交里忙时已连按两次 Ctrl-C、取消生效。收尾动词看它，不看 status 文字。
	canceling bool
	// ctrlCArmed 为真表示正在等窗口内的第二次 Ctrl-C。第一次只武装，不退出、不取消。
	ctrlCArmed bool
	// ctrlCKind 是这一次武装等的动作：退出或取消本轮。未武装时是 ctrlCNone。
	ctrlCKind ctrlCKind
	// ctrlCAt 是这一次武装的第一次按下时刻，跟 ctrlCWindow 比。
	ctrlCAt time.Time
	// ctrlCGen 是武装代次。到期消息带装它时的代次，对不上就丢掉。
	ctrlCGen uint64
	// ctrlCPrev 是武装前的状态栏。解除时状态栏仍是武装提示才写回去。
	ctrlCPrev string
	// copyStatusOn 为真表示状态栏此刻显示的是「已复制 N 个字符」这条提示（见 copy.go）。
	copyStatusOn bool
	// copyStatusGen 是复制提示的代次。到期消息带写它时的代次，对不上就丢掉。
	copyStatusGen uint64
	// copyStatusPrev 是写复制提示前的状态栏，到期且提示仍在屏幕上时写回它。
	copyStatusPrev string
	// copyStatusText 是当前这条复制提示本身。到期时要靠它判断状态栏有没有被别的事件改写。
	copyStatusText string
	// copyScr 是复制模式（TUI-2，见 copyscreen.go）的状态。零值是「不在复制模式」，
	// 所以非复制路径上 View 不会开备用屏，也不会挂 OnMouse。
	copyScr copyScreenState
	// finalizing 为真表示处在 submitDone 到收口之间的窗口。窗口里 Ctrl-C 与 Enter 被挡住。
	finalizing bool
	// pinCursor 为真时 View 把光标钉在表头行首。
	pinCursor bool

	// members 是 @ 补全的候选快照，只在 submitDoneMsg 里整份换掉。
	members []repl.MentionCandidate
	// at 是 @ 补全的状态。
	at atState

	// lang 是界面语言，来自 Options.Lang。零值空串按中文处理。
	lang i18n.Lang
}

// boardRow 是看板上的一名成员。done 为假时还在跑，事件留在零值。
type boardRow struct {
	id   string
	done bool
	ev   repl.MemberDoneEvent
}

// timeFunc 是表头时钟的形状。字段单独起名，避免和 time.Now 的写法缠在一起。
type timeFunc func() time.Time

// New 构造一个已经聚焦、但尚未启动事件循环的 Model。
func New(opts Options) Model {
	println := opts.Println
	if println == nil {
		println = func(line string) tea.Cmd {
			return tea.Println(line)
		}
	}

	ta := textarea.New()
	// Enter 由外层提交；多行输入只通过 Ctrl-J 插入。
	ta.KeyMap.InsertNewline.Unbind()
	// Ctrl-V 不交给 textarea 读剪贴板：它的 Paste 读完之后交回一条未导出的 pasteMsg，
	// 那条消息走 updateInput 最后的 default 分支，不经 tea.PasteMsg 那一支，
	// 既不收起 @ 补全列表，也不把状态栏换成「粘贴了 N 个字符」（X44 的提示靠这一步换掉）。
	// 解绑之后粘贴只剩终端的括号粘贴（tea.PasteMsg）一条路；终端把 Ctrl-V 原样交给程序时，它什么都不做。
	ta.KeyMap.Paste.Unbind()
	// Ctrl+Shift+C 的复制改由 Model 自己接管（见 copy.go 的 copyInput）：它要先取文本、
	// 再自己发 tea.SetClipboard，才能顺带把「复制了多少字」写进状态栏。
	// textarea 自带的 CopySelection 只写剪贴板、不给任何反馈，留着就是第二份逻辑
	// （而且两份会各写一次剪贴板）。解绑之后键位上再也没有第二个处理者。
	ta.KeyMap.CopySelection.Unbind()
	// Focus 是指针方法，必须在局部变量上调用后再把完整值存入 Model。
	focusCmd := ta.Focus()

	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	return Model{
		ctx:      ctx,
		sub:      opts.Submit,
		out:      opts.Out,
		version:  opts.Version,
		println:  println,
		ta:       ta,
		focusCmd: focusCmd,
		status:   i18n.TF(opts.Lang, "tui.status.idle", "空闲"),
		now:      now,
		lang:     opts.Lang,
	}
}

// t 取一条状态栏或提示文案。fallback 传中文原文：语言为中文或目录缺条时
// 输出与改造前逐字节相同。
func (m Model) t(key, fallback string, args ...any) string {
	return i18n.TF(m.lang, key, fallback, args...)
}

// printLine 把一行已经切好的输出变成 bubbletea 内部的打印消息。
// 调用方把它交给 (*tea.Program).Send：事件循环在 Update 之前按到达顺序处理，
// 不经「每个 Cmd 一个 goroutine」那条会乱序的路。
// 除了空串换成一个空格，不改写行内容。
func printLine(line string) tea.Msg {
	if line == "" {
		// bubbletea 的 insertAbove 对空串直接返回，空行会从屏幕上消失；印一个空格保住分段。
		line = " "
	}
	return tea.Println(line)()
}

// Init 启动构造阶段取得的焦点命令。
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.focusCmd)
}

// Update 处理终端消息并返回下一份状态。
//
// tick 不装在 beginSubmit 返回的 Cmd 上：那条必须仍是回显接 submitCmd 的 Sequence。
// 这里包一层：记下处理前是否「忙且还没装 tick」，处理完仍忙、代次没变，才把 tick 并进返回的 Cmd。
// 开始提交的那次 Enter 处理前还不忙，所以那一次不装。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	arm := m.busy && m.tickPending
	gen := m.tickGen
	next, cmd := m.update(msg)
	out, ok := next.(Model)
	if !ok || !arm || !out.busy || out.tickGen != gen {
		return next, cmd
	}
	out.tickPending = false
	return out, tea.Batch(cmd, elapsedTickCmd(out.tickGen))
}

// update 是 Update 去掉「提交后下一条再装 tick」之后的本体。
func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case elapsedTickMsg:
		// 只在忙、且是本代的拍时改已用时长并续订。不碰 status。
		if !m.busy || msg.gen != m.tickGen {
			return m, nil
		}
		m.elapsed = msg.at.Sub(m.runStart)
		return m, elapsedTickCmd(m.tickGen)

	case ctrlCExpireMsg:
		// 代次对得上、且还武装着，才解除并恢复状态栏。旧代次或已经解除的丢掉。
		if !m.ctrlCArmed || msg.gen != m.ctrlCGen {
			return m, nil
		}
		m = m.ctrlCDisarm()
		return m, nil

	case copyStatusExpireMsg:
		// 复制提示到期（见 copy.go）。代次对不上、或状态栏已被别的事件改写，都不写回。
		return m.copyStatusExpire(msg), nil

	case copyScreenLoadedMsg:
		// 复制模式（见 copyscreen.go）的取数结果。空结果不进整屏，只改状态栏。
		return m.copyScreenLoaded(msg)

	case roundStartMsg:
		if !m.busy {
			return m, nil
		}
		m.run = roundRun{
			has:     true,
			label:   msg.ev.Label,
			k:       msg.ev.K,
			n:       msg.ev.N,
			members: msg.ev.Members,
		}
		if len(msg.ev.Speakers) > 0 {
			m.runSeen = true
			m.rosterN += len(msg.ev.Speakers)
			rows := make([]boardRow, len(msg.ev.Speakers))
			for i, id := range msg.ev.Speakers {
				rows[i] = boardRow{id: id}
			}
			m.rows = rows
			clock := m.now
			if clock == nil {
				clock = time.Now
			}
			m.segStart = clock()
			want := len(msg.ev.Speakers)
			lim := m.boardLimit()
			if want > m.floor && want <= lim {
				m.floor = want
			}
		}
		return m, nil

	case memberDoneMsg:
		if !m.busy || !m.run.has {
			return m, nil
		}
		m.run.back++
		if m.run.roundID == "" {
			m.run.roundID = msg.ev.RoundID
		}
		m.backN++
		for i := range m.rows {
			if m.rows[i].id == msg.ev.AgentID && !m.rows[i].done {
				rows := append([]boardRow(nil), m.rows...)
				rows[i].done = true
				rows[i].ev = msg.ev
				m.rows = rows
				break
			}
		}
		return m, nil

	case roundEndMsg:
		return m, nil

	case boardFinalizeMsg:
		if !m.finalizing {
			return m, nil
		}
		m.finalizing = false
		m.pinCursor = false
		m.rows = nil
		m.floor = 0
		m.runSeen = false
		m.backN = 0
		m.rosterN = 0
		m.tickGen++
		return m, nil

	default:
		return m.updateInput(msg)
	}
}

// updateInput 处理表头四种消息以外的输入。这四种在上面已经返回，不交给 textarea。
func (m Model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 复制模式（copyscreen.go）在整屏上，按键与鼠标都归它；只有 WindowSizeMsg
	// 交回下面既有的分支（宽高要更新，退出之后活区才对）。取数期间也算半个
	// 复制模式：键盘要挡住，免得 Enter 与 CopyScreen 并发进同一个 REPL。
	// 这一段必须在 @ 补全列表那一支前面：复制模式里没有输入框，列表也不在屏幕上。
	if m.copyScr.on || m.copyScr.loading {
		if next, cmd, handled := m.updateCopyScreen(msg); handled {
			return next, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		width := msg.Width
		if width <= 0 {
			width = 80
		}
		height := msg.Height
		if height <= 0 {
			height = 24
		}
		m.width = width
		m.height = height
		if msg.Width > 0 && msg.Height > 0 {
			m.termHeight = msg.Height
		} else {
			m.termHeight = 0
		}
		m.ta.SetWidth(max(20, width-4))
		return m, nil

	case submitDoneMsg:
		// 提交结束每条路径都会自己设 status，这里只解除武装，不恢复旧状态栏。
		m = m.ctrlCClear()
		m.members = msg.members
		m.busy = false
		m.cancel = nil
		m.run = roundRun{}
		if m.sub.IsQuit(msg.err) {
			m.status = m.t("tui.status.exiting", "正在退出")
			return m, m.quitCmd()
		}
		if msg.err != nil {
			m.status = m.t("tui.status.errorExiting", "发生错误，正在退出")
			errorCmd := m.println(msg.err.Error())
			return m, tea.Sequence(errorCmd, m.quitCmd())
		}
		if !m.runSeen {
			m.status = m.t("tui.status.idle", "空闲")
			return m, nil
		}
		line := m.closingLine()
		m.status = m.t("tui.status.idle", "空闲")
		m.finalizing = true
		m.pinCursor = m.floor > 0
		return m, tea.Sequence(m.println(line), func() tea.Msg { return boardFinalizeMsg{} })

	case copyDoneMsg:
		// TUI 本地执行的 /copy 回来了（见 copy.go 的 beginCopy / copyDone）。
		return m.copyDone(msg)

	case tea.KeyPressMsg:
		// @ 补全列表在屏幕上时，这六个键归它，不交给下面的既有分支：
		// Ctrl-C 与 Esc 只收起列表（不退出、不取消、不改状态栏）；上下键移动选中项（到头不回绕）；
		// Tab 与回车用选中项补全，回车在这里不提交。列表不在屏幕上时一律走既有分支。
		// 列表在屏幕上时，先解除 Ctrl-C 武装（只在处于武装时才恢复状态栏），再归列表。
		// 修饰键残影（ctrlCKeepsArm）不算别的键，跳过解除。
		// Ctrl-C 在这里只收起列表，不武装、不退出、不取消。
		if m.atVisible() {
			if !ctrlCKeepsArm(msg) {
				m = m.ctrlCDisarm()
			}
			switch {
			case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':
				m.at = atState{}
				return m, nil
			case msg.Code == tea.KeyEscape:
				m.at = atState{}
				return m, nil
			case msg.Code == tea.KeyUp:
				if m.at.sel > 0 {
					m.at.sel--
				}
				return m, nil
			case msg.Code == tea.KeyDown:
				if m.at.sel < len(m.atMatches())-1 {
					m.at.sel++
				}
				return m, nil
			case msg.Code == tea.KeyTab, msg.Code == tea.KeyEnter:
				return m.atAccept()
			}
		}
		switch {
		case isCopyScreenKey(msg):
			// Ctrl+G 进复制模式（TUI-2，见 copyscreen.go）。必须排在 Ctrl-C 与
			// 「输入框复制」那两支前面：它在 textarea 里没有绑定，落到 default
			// 会被当成普通按键吞掉。@ 补全列表开着时也照样进：复制模式是另一屏，
			// 退出后列表按原样回来。
			return m.startCopyScreen()

		case isCopyInputKey(msg):
			// 输入框里的复制。必须排在 Ctrl-C 那一支前面：后者的判据是
			// `Contains(ModCtrl) && Code == 'c'`，Ctrl+Shift+C 也满足，会让复制键
			// 掉进「第一次武装退出」里去（19c 契约 A60 记的正是这个形状）。
			// 列表开着时上面的分支已经截走这个键（只收起列表），到不了这里。
			//
			// 先算文本：空内容时连 Ctrl-C 的武装都不解除 —— 21e §3 要求这种情况
			// 「不改状态栏」，而解除武装会把「再按一次 Ctrl-C 退出」换回复制前的文案，
			// 等于一次空复制把用户正在等的第二次 Ctrl-C 撤掉了。
			text := m.copyInputText()
			if text == "" {
				return m, nil
			}
			m = m.ctrlCDisarm()
			return m.copyInput(text)

		case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':
			// 列表不在屏幕上时的 Ctrl-C。语义见 ctrlCPress：复制模式下也走这一段。
			return m.ctrlCPress()

		case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'j':
			m = m.ctrlCDisarm()
			m.ta.InsertRune('\n')
			m = m.atRecheck()
			return m, nil

		case msg.Code == tea.KeyEnter:
			m = m.ctrlCDisarm()
			if m.busy {
				m.status = m.t("tui.status.busyEnter", "一轮正在跑，先等它回来或连按两次 Ctrl-C 取消")
				return m, nil
			}
			if m.finalizing {
				return m, nil
			}
			return m.beginSubmit()

		case msg.Code == tea.KeyEscape:
			// 列表开着时 Esc 在上面那一支已经截走（只收起列表），到不了这里。
			m = m.ctrlCDisarm()
			// 只有不带修饰键的 Esc 才有 21e 新增的两种动作（取消一轮、清空输入框）。
			// 带修饰键的 Esc 保持原样：列表开着时它与无修饰时完全一样（19c F4 第 1 段钉着），
			// 列表不在时它此前什么都不做，F4 第 2 段用同装置对照钉着「值不动、光标不动」。
			// 清空是破坏性的（输入框没有撤销），绑到组合键上也更容易误触：
			// Alt+Esc 在 Windows 是窗口切换、Ctrl+Esc 是开始菜单。
			// 锁定键位不算修饰键：Windows 控制台会把 NumLock 这类状态也放进 Mod。
			locks := tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
			if msg.Mod&^locks != 0 {
				return m, nil
			}
			// 其余四种情形按优先级从上到下（21e §2）：
			// 1. 收口窗口：什么都不做，与 Ctrl-C 在同一个窗口里被挡住一致。
			if m.finalizing {
				return m, nil
			}
			// 2. 一轮正在跑：取消它，一次就够（Ctrl-C 那条路仍然要连按两次，保留不动）。
			//    已经取消过的不重复取消：cancel 第二次没有意义，而状态栏会闪一下。
			if m.busy {
				if m.canceling {
					return m, nil
				}
				if m.cancel != nil {
					m.cancel()
				}
				m.status = m.t("tui.status.canceling", "正在取消")
				m.canceling = true
				return m, nil
			}
			// 3. 空闲、输入框有字：清空输入框、光标回开头，并关掉补全。
			//    X44 挡下的多行草稿不算这一类：那句话的全部意义是「文字还在，改好再发」，
			//    清掉之后用户没有任何办法找回这段文字（输入框没有撤销）。
			//    整段只有空白时也算「有字」，用户敲了换行同样希望一次收干净。
			if m.ta.Value() != "" {
				if m.status == m.t("tui.hint.oneCommand", oneCommandHint) {
					return m, nil
				}
				m.ta.Reset()
				m.at = atState{}
				return m, nil
			}
			// 4. 空闲、输入框为空：Ctrl-C 的武装已经在上面解除，别的什么都不做。
			return m, nil

		default:
			// 修饰键残影不算别的键，不解除武装；其余键先解除（只在处于武装时才恢复状态栏）。
			if !ctrlCKeepsArm(msg) {
				m = m.ctrlCDisarm()
			}
			var cmd tea.Cmd
			m.ta, cmd = m.ta.Update(msg)
			m = m.atRecheck()
			if !m.at.armed && msg.Text == "@" {
				m = m.atArm()
			}
			return m, cmd
		}

	case tea.PasteMsg:
		m = m.ctrlCDisarm()
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		m.status = m.t("tui.status.pasted", "粘贴了 %d 个字符", utf8.RuneCountInString(msg.Content))
		m.at = atState{}
		return m, cmd

	default:
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		return m, cmd
	}
}

// View 生成内联活区；复制模式（copyscreen.go）下生成临时整屏。
func (m Model) View() tea.View {
	// 复制模式是唯一开备用屏与鼠标捕获的帧。渲染器逐帧比较 lastView
	// （bubbletea v2.0.9 的 cursed_renderer.flush）：退出那一帧改回
	// AltScreen=false + MouseModeNone，终端原生选中随之恢复。
	if m.copyScr.on {
		v := tea.NewView(m.copyScreenContent())
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.OnMouse = copyScreenOnMouse
		return v
	}

	pop := m.atLines()
	parts := append(append(m.chrome(), m.ta.View()), pop...)
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	v := tea.NewView(content)
	// Council 使用内联模式，活区之外的原生回滚区必须保留。
	v.AltScreen = false
	// 平时必须显式声明不捕获鼠标：这是「不抢终端原生选中」的硬保证。
	// MouseMode 的零值确实是 None，但这里写出来是给下一刀看的 ——
	// 删掉这一行而某处又忘了复位，终端原生选中就废了，而且不会有任何编译错误。
	v.MouseMode = tea.MouseModeNone
	v.OnMouse = nil
	if m.pinCursor {
		v.Cursor = tea.NewCursor(0, 0)
	} else if len(pop) > 0 {
		v.Cursor = m.atCursor()
	}
	return v
}

// atCursor 是补全列表在屏幕上时钉住的真光标：落在输入框里正在打字的那一格。
// 钉它是为了缩高（收起列表、筛短列表）时不留残影：渲染器从上一帧结束时的光标行开始缩，
// 残影行数 = max(0, 光标行 - (新帧高 - 1))；列表在输入框下方，光标钉在输入框里，这个数恒为 0。
// 不钉时光标停在上一帧最后改动的那一行（往往是列表最后一行），收起多少行就留多少行残影。
// ta 的副本关掉虚拟光标之后 Cursor() 才给出位置；只动副本，Model 里的 textarea 不变。
func (m Model) atCursor() *tea.Cursor {
	ta := m.ta
	ta.SetVirtualCursor(false)
	c := ta.Cursor()
	if c == nil {
		return tea.NewCursor(0, 0)
	}
	c.Y += len(m.chrome())
	c.Shape = tea.CursorBar
	c.Blink = true
	c.Color = nil
	return c
}

// chrome 返回活区里 textarea 上方的行：表头、横线，再加 floor 行看板。
// View、frameHeight、atCursor、atLines 都从它取这几行或它的行数；textarea 下方的 @ 补全列表不在这里（见 atLines）。
// 往表头与横线之外再加固定行时，boardLimit 里写死的 2（表头与横线）要跟着改。
func (m Model) chrome() []string {
	width := m.width
	if width < 20 {
		width = 20
	}
	ruleWidth := min(width, 120)

	header := fmt.Sprintf("council %s   %s", m.version, headerStatus(m.busy, m.status, m.run, m.elapsed))
	rule := strings.Repeat("─", ruleWidth)
	lines := []string{header, rule}
	if m.floor == 0 {
		return lines
	}
	board := make([]string, m.floor)
	if len(m.rows) <= m.floor {
		idW := 0
		for _, row := range m.rows {
			if n := ansi.StringWidth(row.id); n > idW {
				idW = n
			}
		}
		used := m.elapsed - m.segStart.Sub(m.runStart)
		for i, row := range m.rows {
			if row.done {
				board[i] = memberDoneLine(row.ev, idW)
			} else {
				board[i] = memberRunningLine(row.id, idW, used)
			}
		}
	}
	for i, line := range board {
		board[i] = ansi.Truncate(line, width, "")
	}
	return append(lines, board...)
}

// boardLimit 是看板行数上限。终端高未知时为 0；窗口变矮不改已经占下的 floor。
func (m Model) boardLimit() int {
	if m.termHeight <= 0 {
		return 0
	}
	return m.termHeight - (2 + m.ta.Height()) - 1
}

// closingLine 是无错收尾那一行。时间取此刻的时钟减去本次提交起点，计数跨本提交的全部分段。
func (m Model) closingLine() string {
	verb := m.t("tui.round.verbDone", "结束")
	if m.canceling {
		verb = m.t("tui.round.verbCancelled", "已取消")
	}
	clock := m.now
	if clock == nil {
		clock = time.Now
	}
	return m.t("tui.round.closing", "/round %s  %s  回来 %d/%d", verb, formatElapsed(clock().Sub(m.runStart)), m.backN, m.rosterN)
}

// frameHeight 是 View 的行数：chrome 的行数、textarea 的高度，再加 @ 补全列表的行数（列表不在屏幕上时为 0）。
// 不数 View 里的换行：退出清屏要的是「帧有几行」，与帧里写了什么无关。
func (m Model) frameHeight() int {
	return len(m.chrome()) + m.ta.Height() + len(m.atLines())
}

// atArm 在刚敲进一个 @ 之后开始补全：光标前一格是 @、它的左边界合法、此刻不忙也不在收口窗口。
func (m Model) atArm() Model {
	if m.busy || m.finalizing || m.ta.HasSelection() {
		return m
	}
	line := []rune(m.atLine())
	col := m.ta.Column()
	if col < 1 || col > len(line) || line[col-1] != '@' {
		return m
	}
	if col >= 2 && !repl.MentionBoundaryOK(line[col-2]) {
		return m
	}
	m.at = atState{armed: true, row: m.ta.Line(), col: col - 1}
	return m
}

// atRecheck 在输入框变过之后重判补全还成不成立，不成立就收起。前缀变了选中项回到第一个。
func (m Model) atRecheck() Model {
	if !m.at.armed {
		return m
	}
	if m.ta.HasSelection() || m.ta.Line() != m.at.row {
		m.at = atState{}
		return m
	}
	line := []rune(m.atLine())
	col := m.ta.Column()
	if col <= m.at.col || m.at.col >= len(line) || col > len(line) || line[m.at.col] != '@' {
		m.at = atState{}
		return m
	}
	if m.at.col > 0 && !repl.MentionBoundaryOK(line[m.at.col-1]) {
		m.at = atState{}
		return m
	}
	for _, r := range line[m.at.col+1 : col] {
		if !repl.MentionTokenRune(r) {
			m.at = atState{}
			return m
		}
	}
	if p := string(line[m.at.col+1 : col]); p != m.at.prefix {
		m.at.prefix = p
		m.at.sel = 0
	}
	return m
}

// atLine 是光标所在的那一行。
func (m Model) atLine() string {
	lines := strings.Split(m.ta.Value(), "\n")
	if row := m.ta.Line(); row >= 0 && row < len(lines) {
		return lines[row]
	}
	return ""
}

// atMatches 是按前缀筛过的候选：id 以前缀的小写形式开头。顺序同 members。
func (m Model) atMatches() []repl.MentionCandidate {
	p := strings.ToLower(m.at.prefix)
	var out []repl.MentionCandidate
	for _, c := range m.members {
		if strings.HasPrefix(c.ID, p) {
			out = append(out, c)
		}
	}
	return out
}

// atVisible 报告补全列表此刻在不在屏幕上。只有在屏幕上时上下键、Tab、回车、Esc、Ctrl-C 才归它。
func (m Model) atVisible() bool {
	return len(m.atLines()) > 0
}

// atLines 是补全列表的行，放在输入框下方。不在补全、终端高未知、没有匹配、或放不下一行时为 nil。
func (m Model) atLines() []string {
	if !m.at.armed || m.termHeight <= 0 {
		return nil
	}
	list := m.atMatches()
	if len(list) == 0 {
		return nil
	}
	avail := m.termHeight - 1 - (len(m.chrome()) + m.ta.Height())
	show := min(len(list), atMaxRows, avail)
	if show < 1 {
		return nil
	}
	sel := min(max(m.at.sel, 0), len(list)-1)
	top := max(0, sel-show+1)
	width := min(max(m.width, 20), 120)
	idW := 0
	for _, c := range list {
		idW = max(idW, ansi.StringWidth(c.ID))
	}
	lines := make([]string, 0, show+1)
	for i := top; i < top+show; i++ {
		mark := "  "
		if i == sel {
			mark = "> "
		}
		line := mark + "@" + list[i].ID
		if list[i].Name != "" {
			line = mark + "@" + padCells(list[i].ID, idW) + "  " + list[i].Name
		}
		lines = append(lines, ansi.Truncate(line, width, ""))
	}
	if len(list) > show && avail > show {
		lines = append(lines, ansi.Truncate(m.t("tui.at.count", "  共 %d 个，上下键翻看", len(list)), width, ""))
	}
	return lines
}

// atAccept 用选中的候选替换 @ 与已输入的前缀：删掉它们，再插入 "@<id> "。光标落在那个空格之后。
func (m Model) atAccept() (tea.Model, tea.Cmd) {
	list := m.atMatches()
	pick := list[min(max(m.at.sel, 0), len(list)-1)]
	n := 1 + len([]rune(m.at.prefix))
	var cmds []tea.Cmd
	for i := 0; i < n; i++ {
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		cmds = append(cmds, cmd)
	}
	m.ta.InsertString("@" + pick.ID + " ")
	m.at = atState{}
	return m, tea.Batch(cmds...)
}

// oneCommandHint 是多行提交被挡下时状态栏的那句话（00_DECISIONS #44）。
const oneCommandHint = "一次只能一条命令，逐条回车；整段当消息发，首行只写 /say"

// beginSubmit 把当前输入定稿到回滚区，并在后台启动一次提交。
//
// 多行输入里第 2 行起有斜杠命令时不提交（repl.HasLaterCommandLine）：输入框原样保留、
// 不回显、不进忙，只把状态栏换成 oneCommandHint。这句提示留到下一次改写状态栏的事件
// （成功提交、粘贴、列表不在屏幕上时的空闲第二次 Ctrl-C）为止。空闲第一次 Ctrl-C 会把它临时换成
// 「再按一次 Ctrl-C 退出」，到期或按别的键后恢复成这句。打字、tick、窗口尺寸变化都不动它，
// @ 补全列表在屏幕上时归列表的六个键（Ctrl-C、Esc、上、下、Tab、回车）也不动它。
func (m Model) beginSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimRight(m.ta.Value(), "\r\n")
	if strings.TrimSpace(text) == "" {
		return m, nil
	}
	if repl.HasLaterCommandLine(text) {
		m.status = m.t("tui.hint.oneCommand", oneCommandHint)
		return m, nil
	}

	lines := strings.Split(text, "\n")
	cmds := make([]tea.Cmd, 0, len(lines)+1)
	for _, line := range lines {
		cmds = append(cmds, m.println("> "+line))
	}

	// /copy 在 TUI 这一侧执行，不交给 Submit：剪贴板是终端的事（tea.SetClipboard 是一条
	// 往终端写 OSC52 的 Cmd），REPL 的出口只有 io.Writer，命令函数拿不到 Program。
	// 参数解析与取数仍在 registry.go 的 CopyRecent 里，两条入口只有一份。
	// 回显照打（与其余命令一样），输入框照清，结果由 copyDoneMsg 写剪贴板并改状态栏。
	if next, cmd, ok := m.beginCopy(text, cmds); ok {
		return next, cmd
	}

	m.ta.Reset()
	m.at = atState{}
	m.busy = true
	m.status = m.t("tui.status.running", "正在运行")
	m.run = roundRun{}
	m.rows = nil
	m.floor = 0
	m.runSeen = false
	m.backN = 0
	m.rosterN = 0
	m.canceling = false
	clock := m.now
	if clock == nil {
		clock = time.Now
	}
	m.runStart = clock()
	m.elapsed = 0
	m.tickGen++
	m.tickPending = true
	baseCtx := m.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	runCtx, cancel := context.WithCancel(baseCtx)
	m.cancel = cancel
	cmds = append(cmds, submitCmd(m.sub, runCtx, baseCtx, text))
	return m, tea.Sequence(cmds...)
}

// quitCmd 同步接管并排空输出，再严格按顺序打印这些行并退出。
//
// 必须在 Update 里同步做完，不能靠 p.Send：Update 正被事件循环调用，
// 没人消费 p.msgs（无缓冲），同步 Send 直接死锁；就算不死锁，
// tea.Sequence(flushCmd, tea.Quit) 里 flush 发出的消息也可能在 Quit
// 被处理之后才到，那时程序已经在退，消息丢掉。
// 所以这里换成本地收集器再 Flush，收到的行随后由 Sequence 按顺序打出去。
// 必须 Sequence 不能 Batch：Batch 并发跑，顺序不保证。
func (m Model) quitCmd() tea.Cmd {
	var rest []string
	if m.out != nil {
		m.out.SetEmit(func(line string) {
			rest = append(rest, line)
		})
		m.out.Flush()
	}

	cmds := make([]tea.Cmd, 0, len(rest)+1)
	for _, line := range rest {
		cmds = append(cmds, m.println(line))
	}
	cmds = append(cmds, tea.Quit)
	return tea.Sequence(cmds...)
}

// submitCmd 在 Bubble Tea 管理的命令 goroutine 中调用 Submit。
// Submit 无错返回之后，在同一个 goroutine 上接着取 @ 补全的候选（sub 实现了 mentionSource 时）：
// 与命令串行，不与任何 Submit 并发读 REPL；也不在 Update 里读库。
// 取候选用 listCtx（Model 的根 ctx），不用 ctx：这一条命令被 Ctrl-C 取消过时 ctx 已经取消，读库会失败。
// 取失败时候选为 nil，列表不弹。
func submitCmd(sub Submitter, ctx, listCtx context.Context, text string) tea.Cmd {
	return func() tea.Msg {
		err := sub.Submit(ctx, text)
		if err != nil {
			return submitDoneMsg{err: err}
		}
		var members []repl.MentionCandidate
		if src, ok := sub.(mentionSource); ok {
			if list, lerr := src.MentionCandidates(listCtx); lerr == nil {
				members = list
			}
		}
		return submitDoneMsg{members: members}
	}
}
