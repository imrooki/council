package tui

// 本文件是 TUI-2（docs/21f_TUI2_COPY_MODE_TASK.md）的复制模式：
// Ctrl+G 或 /copy --screen 进一块临时整屏，拖选复制，Esc / q 退出还原。
//
// 三条纪律写在这里，免得以后被「顺手优化」掉：
//
//  1. 非复制模式的每一帧都必须是 AltScreen=false + MouseModeNone。Council 是内联的，
//     消息用 tea.Println 进了终端回滚区，终端原生选中必须归终端管。只有复制模式
//     那一帧才开备用屏与 CellMotion；退出那一帧改回 None，bubbletea v2.0.9 的
//     渲染器逐帧比较 lastView（cursed_renderer.go 的 flush），会真的写/撤
//     鼠标模式序列，并真的 enter/leave alt screen。
//
//  2. 正文行是纯文本，选区是「显示列」。列到 rune 的映射一律用
//     github.com/mattn/go-runewidth 按显示宽度算：中文占两格，一个汉字的两格
//     都指向同一个 rune。按字节或 rune 下标切，中文行会切错。
//
//  3. 复制提示沿用 TUI-1 的 copyStatusShow / copyStatusExpire / copyHint（copy.go）：
//     同一句话、同一个 2 秒窗口、同一份 rune 计数，不另起一套。

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"council/internal/repl"
)

// copyPoint 是正文里的一个位置：第 line 行（0 起）的第 cell 个显示格（0 起）。
//
// cell 是显示格，不是 rune 下标：一个汉字占两格，落在哪一格都指向同一个 rune。
// 存显示格而不是 rune 下标，是为了让鼠标坐标与行宽用同一个坐标系；
// 真正切文本时才过一遍 copyRuneStartAtCell / copyRuneEndAtCell。
type copyPoint struct {
	line int
	cell int
}

// copyScreenState 是复制模式的状态。零值是「不在复制模式」。
type copyScreenState struct {
	// on 为真表示已经进了整屏；View 只看它决定这一帧是不是 alt screen + 鼠标捕获。
	on bool
	// loading 为真表示 Ctrl+G / /copy --screen 已经发出取数命令、还没回来。
	//
	// 取数不能放在 Update 里同步做：History 走 SQLite，在事件循环上查一次
	// 就能把打字卡住（与 copy.go 的 copyCmd 同一个理由）。而「没有可复制的
	// 消息」又不许先进整屏再退出来（任务书 §3），所以先记 loading，
	// 结果回来再决定进不进。
	loading bool
	// lines 是排版好的正文行：纯文本，[名字] 行与两条之间的空行都在里面。
	// 生成之后只读，Update 之间按值拷贝共享同一份底层数组。
	lines []string
	// first 是正文区第一行的下标。滚动只改它。
	first int
	// anchor 是按下左键时的位置，cur 是拖动/松开时的位置。两者归一化之后才是选区。
	anchor copyPoint
	cur    copyPoint
	// sel 为真表示按下过（松开之后仍保留：复制完不退出，也方便看选中了什么）。
	sel bool
	// drag 为真表示左键还按着。只有它能让 MouseMotion 扩展选区。
	drag bool
	// truncated 为真表示会话更长，只装了最近 repl.CopyScreenMax 条，顶栏要说明。
	truncated bool
	// entryStatus 是进屏那一刻的状态栏。复制模式里没有普通状态栏，顶栏要在
	// m.status 被别的事件改掉时把它显示出来（目前只有 Ctrl-C 的武装/退出）；
	// 与它相同就说明状态栏还是进屏前那一句，不该占顶栏。
	entryStatus string
}

// clearSelection 清掉选区。滚动之后必须清：首行变了，旧坐标指向的是别的文字。
func (s *copyScreenState) clearSelection() {
	s.sel = false
	s.drag = false
	s.anchor = copyPoint{}
	s.cur = copyPoint{}
}

// copyScreenSource 是 Submitter 可以另外实现的能力：取整场会话的正文，供复制模式排版。
//
// 与 copySource 同一个体例：不并进 Submitter —— 既有的假 Submitter 都没有这个方法，
// 没实现的一律当作没有复制模式。生产实现是 run.go 的 replAdapter。
type copyScreenSource interface {
	CopyScreen(ctx context.Context) ([]repl.CopyLine, bool, error)
}

// copyScreenLoadedMsg 是取数命令的结果。err 非 nil 时 lines 无意义。
type copyScreenLoadedMsg struct {
	lines     []repl.CopyLine
	truncated bool
	err       error
}

// copyScreenLoadCmd 在命令 goroutine 上向 REPL 取正文，不在 Update 里读库。
func copyScreenLoadCmd(src copyScreenSource, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		lines, truncated, err := src.CopyScreen(ctx)
		return copyScreenLoadedMsg{lines: lines, truncated: truncated, err: err}
	}
}

// copyScreenMouseMsg 把 View.OnMouse 收到的鼠标事件送回 Update。
//
// bubbletea v2.0.9 只在最后一帧的 OnMouse 非 nil 时把鼠标事件交给它
// （cursed_renderer.go 的 onMouse），所以这条路径只在复制模式里存在。
// 消息只带事件本身，不碰视图闭包里的状态：Model 是值类型，Update 才是唯一的写入口。
type copyScreenMouseMsg struct{ ev tea.MouseMsg }

// copyScreenOnMouse 是复制模式那一帧的 OnMouse。
func copyScreenOnMouse(msg tea.MouseMsg) tea.Cmd {
	return func() tea.Msg { return copyScreenMouseMsg{ev: msg} }
}

// isCopyScreenKey 报告这个键是不是进入复制模式的 Ctrl+G。
func isCopyScreenKey(msg tea.KeyPressMsg) bool {
	return msg.Code == 'g' && msg.Mod.Contains(tea.ModCtrl)
}

// startCopyScreen 发出取数命令。两处入口共用：Ctrl+G 与 /copy --screen。
//
// 忙时与收口窗口不进去：复制模式是静止的一屏，而一轮在跑时 REPL 的输出
// 会经 LineWriter → tea.Println 打进终端；在备用屏上插行会把这一屏搅乱，
// 退出之后活区也对不上。任务书只写了「没有内容时不进」，这一条是实现在这里加的。
func (m Model) startCopyScreen() (Model, tea.Cmd) {
	if m.copyScr.on || m.copyScr.loading {
		return m, nil
	}
	if m.busy || m.finalizing {
		m.status = "一轮正在跑，等它回来再复制"
		return m, nil
	}
	src, ok := m.sub.(copyScreenSource)
	if !ok {
		m.status = "复制模式不可用"
		return m, nil
	}
	// 进屏是「别的事件」：两条临时状态都要收干净 —— Ctrl-C 的武装（否则退出
	// 复制模式之后状态栏留着一句 2 秒内有效的「再按一次 Ctrl-C 退出」）、
	// 上一条复制提示（否则复制模式的顶栏会先挂着上一次 /copy 的「已复制 N 个
	// 字符」两秒，看起来像是刚复制过）。
	m = m.copyStatusSettle()
	m = m.ctrlCDisarm()
	m.copyScr = copyScreenState{loading: true}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return m, copyScreenLoadCmd(src, ctx)
}

// copyScreenLoaded 处理取数结果。
//
// 空结果（没有当前会话、会话里没有可复制正文）不进整屏：状态栏一句
// 「没有可复制的消息」，与 /copy 空会话的答复是同一个常量（repl.MsgCopyNoContent）。
func (m Model) copyScreenLoaded(msg copyScreenLoadedMsg) (Model, tea.Cmd) {
	if !m.copyScr.loading {
		// 取数期间 Model 已经换过一轮（重复的 Ctrl+G 被挡在 startCopyScreen），
		// 迟到的结果丢掉，不能把用户拽进整屏。
		return m, nil
	}
	m.copyScr = copyScreenState{}
	if msg.err != nil {
		if errors.Is(msg.err, repl.ErrCopyEmpty) {
			m.status = repl.MsgCopyNoContent
			return m, nil
		}
		// 读库失败按命令错误的既有体例打到回滚区，不退出、不改状态栏。
		return m, m.println(msg.err.Error())
	}
	lines := copyScreenText(msg.lines)
	if len(lines) == 0 {
		m.status = repl.MsgCopyNoContent
		return m, nil
	}
	m = m.ctrlCDisarm()
	m.copyScr = copyScreenState{
		on:          true,
		lines:       lines,
		truncated:   msg.truncated,
		entryStatus: m.status,
	}
	return m, nil
}

// copyScreenExit 退出复制模式，回到内联活区。
//
// 输入框与光标不在这里恢复：复制模式的按键一条都不经过 textarea（见
// updateCopyScreen 的 default 分支），所以它们从来没被改过，回写反而会把
// 复制期间收到的窗口尺寸变化丢掉。
//
// 两条临时状态要收干净：复制提示（否则退出后状态栏挂着一条 2 秒后自己消失的文案）
// 与 Ctrl-C 武装（否则退出后 2 秒内再按一次 Ctrl-C 会直接退出，用户没在等它）。
func (m Model) copyScreenExit() (Model, tea.Cmd) {
	m.copyScr = copyScreenState{}
	m = m.copyStatusSettle()
	m = m.ctrlCDisarm()
	return m, nil
}

// copyScreenText 把消息排成整屏的正文行。
//
// 成员发言前一行 [名字]，用户是 [你]，两条之间空一行（任务书 §4）。
// 消息正文里的换行拆成多行；制表符换成四个空格 —— 制表符在终端里跳到下一个
// 8 格制表位，列号没法拿它做选区映射，展开之后屏幕上的格数与选区的格数
// 才是同一个坐标系。ANSI 序列一并剥掉：复制出来要是干净文字，屏幕也不该被
// 消息里的转义序列搅乱。
func copyScreenText(msgs []repl.CopyLine) []string {
	var out []string
	for i, c := range msgs {
		if i > 0 {
			out = append(out, "")
		}
		who := c.Who
		if c.User {
			who = "你"
		}
		out = append(out, "["+who+"]")
		out = append(out, copyPlainLines(c.Text)...)
	}
	return out
}

// copyPlainLines 把一条消息正文拆成屏幕行：只换行、不折行（任务书把「行」当正文行用）。
// 空正文也要占一行，否则 [名字] 下面会直接接上分隔空行，看起来像没说话。
func copyPlainLines(text string) []string {
	text = ansi.Strip(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimRight(text, "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	if text == "" {
		return []string{""}
	}
	return strings.Split(text, "\n")
}

// updateCopyScreen 处理复制模式里的输入。handled 为 false 时交回既有路径。
//
// 只有 WindowSizeMsg 交回去：宽高要更新，textarea 的宽度也要跟着，
// 退出之后活区才对。其余消息一律吞掉 —— 复制模式里没有输入框，
// 打字、粘贴、@ 补全都不该落到 textarea 上，否则退出时「原样恢复」就不成立。
func (m Model) updateCopyScreen(msg tea.Msg) (Model, tea.Cmd, bool) {
	// 取数期间也把键盘挡住：Enter 若在这时落到 beginSubmit 上，就会有一条
	// Submit 与 CopyScreen 并发读同一个 REPL，而 REPL 没有为并发准备
	// （命令本来是靠事件循环串行派发的）。Esc 取消这次进入，Ctrl-C 照旧。
	if m.copyScr.loading {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			return m, nil, false
		case tea.KeyPressMsg:
			if msg.Code == tea.KeyEscape {
				// 迟到的取数结果会被 copyScreenLoaded 的 loading 检查丢掉。
				// 取消要回到进屏前的干净状态：Ctrl-C 若在这里武装过，
				// 不解除的话退出后 2 秒内再按一次就会直接退出程序。
				m.copyScr = copyScreenState{}
				m = m.copyStatusSettle()
				m = m.ctrlCDisarm()
				return m, nil, true
			}
			if msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c' {
				next, cmd := m.ctrlCPress()
				return next, cmd, true
			}
		}
		return m, nil, true
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil, false

	case tea.KeyPressMsg:
		// Ctrl-C 与 21e 完全一致：收口窗口挡住，忙时第一次武装、第二次取消本轮，
		// 空闲时第一次武装、第二次退出。复制模式不改变这套语义（任务书 §3）。
		// 放在最前面，免得 'q'、方向键的分支先把它截走。
		if msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c' {
			next, cmd := m.ctrlCPress()
			return next, cmd, true
		}
		switch {
		case msg.Code == tea.KeyEscape:
			// Esc 在复制模式里只退出复制模式；21e 的取消/清空语义只在
			// 非复制模式下有效，这里到不了 updateInput 的 Esc 分支。
			next, cmd := m.copyScreenExit()
			return next, cmd, true
		case msg.Code == 'q' && !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt):
			// 复制模式没有输入框，q 不输入文本，直接退出（任务书 §3）。
			next, cmd := m.copyScreenExit()
			return next, cmd, true
		case msg.Code == tea.KeyUp:
			next, cmd := m.copyScreenScroll(-1)
			return next, cmd, true
		case msg.Code == tea.KeyDown:
			next, cmd := m.copyScreenScroll(1)
			return next, cmd, true
		case msg.Code == tea.KeyPgUp:
			next, cmd := m.copyScreenScroll(-m.copyBodyHeight())
			return next, cmd, true
		case msg.Code == tea.KeyPgDown:
			next, cmd := m.copyScreenScroll(m.copyBodyHeight())
			return next, cmd, true
		case msg.Code == tea.KeyHome:
			next, cmd := m.copyScreenScrollTo(0)
			return next, cmd, true
		case msg.Code == tea.KeyEnd:
			// MaxInt 会被夹到「最后一屏的首行」：End 要看到的是末行，不是空屏。
			next, cmd := m.copyScreenScrollTo(math.MaxInt)
			return next, cmd, true
		}
		// 其余按键（含 Ctrl+Shift+C、Tab、字母）一律吞掉：没有输入框，
		// 也不能让它们把 textarea 改掉。
		return m, nil, true

	case copyScreenMouseMsg:
		next, cmd := m.copyScreenMouse(msg.ev)
		return next, cmd, true

	default:
		// 其它消息（PasteMsg、不可达的 submitDoneMsg 等）在复制模式里没有意义。
		// 不交给 textarea，免得退出之后输入框里多出东西。
		return m, nil, true
	}
}

// copyScreenMouse 处理鼠标事件。
//
// 只有左键拖选；滚轮同 ↑/↓（任务书 §4）。
// copyScreenMouse 处理鼠标事件。
//
// 只有左键拖选；滚轮同 ↑/↓（任务书 §4）。
// 松开只认左键与 MouseNone：X10 编码把松开报成 MouseNone（ultraviolet 的
// parseMouseButton），只认 MouseLeft 会让老终端上永远复制不了；但右键/中键的
// 松开不能结束左键拖选 —— 左键按下之后按右键松开，不该顺手复制一段。
// 注意 X10 分不出是哪个键松开（一律 MouseNone），这条保证在现代终端上成立：
// bubbletea 请求的是 SGR 扩展编码，Button 是准的。
func (m Model) copyScreenMouse(ev tea.MouseMsg) (Model, tea.Cmd) {
	mouse := ev.Mouse()
	switch ev.(type) {
	case tea.MouseWheelMsg:
		switch mouse.Button {
		case tea.MouseWheelUp:
			return m.copyScreenScroll(-1)
		case tea.MouseWheelDown:
			return m.copyScreenScroll(1)
		}
		return m, nil

	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return m, nil
		}
		return m.copyScreenPress(mouse)

	case tea.MouseMotionMsg:
		// 不查 Button：CellMotion 模式只在按住键时上报移动，有的终端这一位
		// 会缺；drag 已经说明「按下过左键、还没松开」。
		if !m.copyScr.drag {
			return m, nil
		}
		s := m.copyScr
		s.cur = m.copyPointAt(mouse.X, mouse.Y)
		m.copyScr = s
		return m, nil

	case tea.MouseReleaseMsg:
		if mouse.Button != tea.MouseLeft && mouse.Button != tea.MouseNone {
			return m, nil
		}
		return m.copyScreenRelease(mouse)
	}
	return m, nil
}

// copyScreenPress 落选区的起点，并解除 TUI-1 的复制提示：
// 用户又开始选了，上一句「已复制 N 个字符」是过期信息，不能再盖住「已选中 N 个字符」。
func (m Model) copyScreenPress(mouse tea.Mouse) (Model, tea.Cmd) {
	m = m.copyStatusSettle()
	s := m.copyScr
	p := m.copyPointAt(mouse.X, mouse.Y)
	s.anchor, s.cur = p, p
	s.sel, s.drag = true, true
	m.copyScr = s
	return m, nil
}

// copyScreenRelease 松开左键：按行优先取文本，空选区不复制、不提示
// （copyInput 同一条：写空串会把用户之前复制的东西抹掉）。
// 复制之后不退出复制模式，方便继续选（任务书 §5）。
//
// 零位移（按下与松开落在同一行同一格）不算选区：选区的两端都是含的，
// 不特判的话单击一下就会复制一个字符，顶栏还会说「已复制 1 个字符」。
func (m Model) copyScreenRelease(mouse tea.Mouse) (Model, tea.Cmd) {
	s := m.copyScr
	if !s.drag {
		return m, nil
	}
	s.cur = m.copyPointAt(mouse.X, mouse.Y)
	s.drag = false
	if s.cur == s.anchor {
		s.clearSelection()
		m.copyScr = s
		return m, nil
	}
	m.copyScr = s

	text := m.copySelectedText()
	if text == "" {
		// 空行上的拖动等等：不留一个看起来选中了、其实取不出字的状态。
		s.clearSelection()
		m.copyScr = s
		return m, nil
	}
	// 先解除 Ctrl-C 武装：复制提示会改写状态栏，留着武装的话到期恢复出来的
	// 是一句已经作废的「再按一次 Ctrl-C …」（copyDone 同一条纪律）。
	m = m.ctrlCDisarm()
	var expire tea.Cmd
	m, expire = m.copyStatusShow(copyHint(text))
	return m, tea.Batch(tea.SetClipboard(text), expire)
}

// copyPointAt 把终端坐标换成正文位置。
//
// y 先按正文区夹取（表头之上算第一行，页脚之下算最后一行）—— 任务书 §5 的
// 竖越界规则。x 分两种：拖到可见区最右列（x >= 终端宽-1）按该行行尾算，
// 这样一条比屏幕宽的长行也能一次选到行尾；其余按显示宽度夹到 [0, 宽-1]，
// 行尾之外仍是最后一个 rune。空行没有格子可落，cell 恒为 0，文本自然是空串。
func (m Model) copyPointAt(x, y int) copyPoint {
	s := m.copyScr
	n := len(s.lines)
	if n == 0 {
		return copyPoint{}
	}
	body := m.copyBodyHeight()
	row := min(max(y-1, 0), body-1)
	line := min(m.copyFirst()+row, n-1)
	cell := 0
	if w := runewidth.StringWidth(s.lines[line]); w > 0 {
		switch {
		case x >= m.copyWidth()-1:
			// 可见区最右列 = 这一行的行尾。长行被显示截断之后，鼠标永远
			// 到不了 80 格以外的列；不这样映射就永远选不到行尾。
			cell = w - 1
		default:
			cell = min(max(x, 0), w-1)
		}
	}
	return copyPoint{line: line, cell: cell}
}

// copySelectedText 按行优先取选区文本：
// 起点行从 x 到行尾，中间整行，终点行从行首到 x；同一行取两点之间。
// 反向拖动先把两个端点按（行、格）归一化，结果与正向一致。
func (m Model) copySelectedText() string {
	s := m.copyScr
	if !s.sel || len(s.lines) == 0 {
		return ""
	}
	a, b := s.anchor, s.cur
	if a.line > b.line || (a.line == b.line && a.cell > b.cell) {
		a, b = b, a
	}
	a.line = min(max(a.line, 0), len(s.lines)-1)
	b.line = min(max(b.line, 0), len(s.lines)-1)
	// 两端都含的区间在 a == b 时仍然会取到一个 rune；零位移不是选区
	// （单击不复制，见 copyScreenRelease），这里先把它归一成空串。
	if a == b {
		return ""
	}
	if a.line == b.line {
		return copyLineCells(s.lines[a.line], a.cell, b.cell)
	}
	parts := make([]string, 0, b.line-a.line+1)
	parts = append(parts, copyLineFrom(s.lines[a.line], a.cell))
	parts = append(parts, s.lines[a.line+1:b.line]...)
	parts = append(parts, copyLineThrough(s.lines[b.line], b.cell))
	return strings.Join(parts, "\n")
}

// copySelectionRunes 是顶栏与 copyHint 共用的 rune 计数：一个汉字算 1，不按字节。
func (m Model) copySelectionRunes() int {
	return utf8.RuneCountInString(m.copySelectedText())
}

// highlightRange 返回第 line 行该反显的显示格区间（闭区间），以及这一行有没有选中部分。
func (s copyScreenState) highlightRange(line int) (from, to int, ok bool) {
	if !s.sel {
		return 0, 0, false
	}
	a, b := s.anchor, s.cur
	if a.line > b.line || (a.line == b.line && a.cell > b.cell) {
		a, b = b, a
	}
	if a == b {
		// 零位移不是选区，不反显（与 copySelectedText 的空串一致）。
		return 0, 0, false
	}
	switch {
	case line < a.line || line > b.line:
		return 0, 0, false
	case a.line == b.line:
		return a.cell, b.cell, true
	case line == a.line:
		return a.cell, math.MaxInt, true
	case line == b.line:
		return 0, b.cell, true
	default:
		return 0, math.MaxInt, true
	}
}

// copyScreenScroll 滚动 delta 行并清掉选区。滚轮、↑↓、PgUp/PgDn 共用。
func (m Model) copyScreenScroll(delta int) (Model, tea.Cmd) {
	s := m.copyScr
	maxFirst := max(0, len(s.lines)-m.copyBodyHeight())
	s.first = min(max(m.copyFirst()+delta, 0), maxFirst)
	s.clearSelection()
	m.copyScr = s
	return m, nil
}

// copyScreenScrollTo 把首行滚到指定位置并清掉选区。Home / End 用。
func (m Model) copyScreenScrollTo(first int) (Model, tea.Cmd) {
	s := m.copyScr
	maxFirst := max(0, len(s.lines)-m.copyBodyHeight())
	s.first = min(max(first, 0), maxFirst)
	s.clearSelection()
	m.copyScr = s
	return m, nil
}

// copyBodyHeight 是正文区的行数：整屏去掉顶栏与页脚各一行。
func (m Model) copyBodyHeight() int {
	h := m.height - 2
	if h < 1 {
		h = 1
	}
	return h
}

// copyWidth 是当前可用的终端列数。没收到过 WindowSizeMsg 时按 80 算，
// 与 New 之后的兜底一致；View 在下一帧窗口尺寸到了就会改。
func (m Model) copyWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	return w
}

// copyFirst 是夹取之后的正文首行。窗口变矮、正文变短都在这里兜住，
// 不单独维护「first 合法」这个不变量。
func (m Model) copyFirst() int {
	s := m.copyScr
	return min(max(s.first, 0), max(0, len(s.lines)-m.copyBodyHeight()))
}

// copyScreenContent 是复制模式那一帧的正文。
//
// 顶栏一行、正文区固定 copyBodyHeight 行（不足补空行，页脚才会落在屏幕最后
// 一行）、页脚一行。每行都按终端宽度截断：备用屏一行只占一格坐标，
// 一条长消息若在屏幕上折行，y 坐标就和正文行对不上了。
//
// 显示截断不改变复制内容：拖到可见区最右列时按行尾算（见 copyPointAt），
// 所以截断只影响「看得见多少」，不把长行截短之后再复制出去。
func (m Model) copyScreenContent() string {
	s := m.copyScr
	width := m.copyWidth()
	body := m.copyBodyHeight()
	first := m.copyFirst()
	total := len(s.lines)
	last := min(first+body, total)

	parts := make([]string, 0, body+2)
	parts = append(parts, ansi.Truncate(m.copyScreenTop(), width, ""))
	for i := first; i < last; i++ {
		line := ansi.Truncate(s.lines[i], width, "")
		if from, to, ok := s.highlightRange(i); ok {
			line = copyLineHighlighted(line, from, to)
		}
		parts = append(parts, line)
	}
	for len(parts) < 1+body {
		parts = append(parts, "")
	}

	a, b := 0, 0
	if total > 0 {
		a, b = first+1, last
	}
	parts = append(parts, fmt.Sprintf("第 %d–%d 行 / 共 %d 行", a, b, total))
	return strings.Join(parts, "\n")
}

// copyScreenTop 是顶栏那一行（任务书 §4 的格式，§6 的截断说明缀在后面）。
//
// 那一格按优先级显示：TUI-1 的复制提示（「已复制 N 个字符」，2 秒后自己消失）
// → 复制模式期间被别的事件改掉的状态栏 → 「已选中 N 个字符」→「拖动选择」。
// 中间这一档是必要的：复制模式里没有普通状态栏，第一次 Ctrl-C 的
// 「再按一次 Ctrl-C 退出」不显示出来，用户就完全看不到反馈。
func (m Model) copyScreenTop() string {
	sel := "拖动选择"
	if n := m.copySelectionRunes(); n > 0 {
		sel = fmt.Sprintf("已选中 %d 个字符", n)
	}
	switch {
	case m.copyStatusOn:
		sel = m.copyStatusText
	case m.status != "" && m.status != m.copyScr.entryStatus:
		// 与进屏时相同说明状态栏还是进屏前那一句，不该占顶栏；不同才是
		// 复制模式期间发生的事（Ctrl-C 武装、正在退出）。
		sel = m.status
	}
	top := "复制模式  ↑↓/PgUp/PgDn 滚动  Esc 退出  " + sel
	if m.copyScr.truncated {
		top += fmt.Sprintf("  只加载了最近 %d 条", repl.CopyScreenMax)
	}
	return top
}

// copyLineHighlighted 把一行里 [from, to] 这些显示格用反显包起来。
// 只加 SGR 7/27，不加任何颜色：正文仍是纯文本，复制出来的字符串不含转义序列。
func copyLineHighlighted(line string, from, to int) string {
	rs := []rune(line)
	i := copyRuneStartAtCell(rs, from)
	j := copyRuneEndAtCell(rs, to)
	if i >= j {
		return line
	}
	return string(rs[:i]) + "\x1b[7m" + string(rs[i:j]) + "\x1b[27m" + string(rs[j:])
}

// copyLineCells 取一行里 [from, to] 两个显示格之间的文本（两端都含）。
func copyLineCells(line string, from, to int) string {
	rs := []rune(line)
	i := copyRuneStartAtCell(rs, from)
	j := copyRuneEndAtCell(rs, to)
	if j < i {
		j = i
	}
	return string(rs[i:j])
}

// copyLineFrom 取一行从第 cell 格到行尾的文本。
func copyLineFrom(line string, cell int) string {
	rs := []rune(line)
	return string(rs[copyRuneStartAtCell(rs, cell):])
}

// copyLineThrough 取一行从行首到第 cell 格的文本（含这一格）。
func copyLineThrough(line string, cell int) string {
	rs := []rune(line)
	return string(rs[:copyRuneEndAtCell(rs, cell)])
}

// copyRuneStartAtCell 返回第 cell 个显示格落在哪个 rune 上。
//
// 一个宽字符占两格，落在它的右半格也返回它自己的下标：从汉字中间开始拖，
// 也把整个汉字选进来，不会切出半个 UTF-8 字符。零宽字符不占格，
// 跟着前一个字走（见 copyRuneEndAtCell）。
func copyRuneStartAtCell(rs []rune, cell int) int {
	col := 0
	for i, r := range rs {
		w := runewidth.RuneWidth(r)
		if w < 0 {
			w = 0
		}
		if cell < col+w {
			return i
		}
		col += w
	}
	return len(rs)
}

// copyRuneEndAtCell 返回第 cell 个显示格之后的 rune 下标（不含）。
//
// 宽字符按整个字算；零宽字符（组合符号）跟在前一个字后面一起带上，
// 否则从中间切时会把组合符号留在选区外面，复制出来是拆开的字。
func copyRuneEndAtCell(rs []rune, cell int) int {
	col := 0
	for i, r := range rs {
		w := runewidth.RuneWidth(r)
		if w < 0 {
			w = 0
		}
		if w > 0 && cell < col+w {
			j := i + 1
			for j < len(rs) && runewidth.RuneWidth(rs[j]) <= 0 {
				j++
			}
			return j
		}
		col += w
	}
	return len(rs)
}
