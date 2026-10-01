package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"council/internal/repl"
	"council/internal/turn"
)

// 三种事件在事件循环里的样子。字段名 ev 冻结，测试腿按它构造与比较。
type roundStartMsg struct{ ev repl.RoundStartEvent }
type memberDoneMsg struct{ ev repl.MemberDoneEvent }
type roundEndMsg struct{ ev repl.RoundEndEvent }

// elapsedTickMsg 是表头计时的一拍。gen 是发出这一拍时的代次，at 是 tea.Tick 交来的时刻。
type elapsedTickMsg struct {
	gen uint64
	at  time.Time
}

// roundObserver 把 repl 的轮次事件原样包成上面三种消息交给 send。
// 生产上 send 就是 (*tea.Program).Send。不持任何锁、不读写任何共享状态、不调 REPL / Runner。
type roundObserver struct{ send func(tea.Msg) }

func newRoundObserver(send func(tea.Msg)) roundObserver { return roundObserver{send: send} }

func (o roundObserver) RoundStart(ev repl.RoundStartEvent) { o.send(roundStartMsg{ev: ev}) }
func (o roundObserver) MemberDone(ev repl.MemberDoneEvent) { o.send(memberDoneMsg{ev: ev}) }

// RoundEnd 把段终态块打进回滚区，再发 roundEndMsg。
// TUI 从事件上的 Concise 知道档位：不读任何全局变量、不回调 REPL，适配器仍无状态。
// 简洁档下每个成员头已经带了名字、结局与用时，段终态块是重复的，所以不发块。
// roundEndMsg 两档都发。活区看板、表头、收尾行不受档位影响。详细档与 RB-B 逐字节相同。
func (o roundObserver) RoundEnd(ev repl.RoundEndEvent) {
	if !ev.Concise {
		for _, line := range endBlockLines(ev) {
			o.send(printLine(line))
		}
	}
	o.send(roundEndMsg{ev: ev})
}

// boardFinalizeMsg 是无错收尾窗口的收口。正常路径上唯一让活区变矮的消息（例外见 Model.floor 的注释）。
type boardFinalizeMsg struct{}

// endBlockLines 把一段的终态收成回滚块。Results 为空时返回 nil。
// 纯函数：标题用 segmentCell，成员行与活区共用同一组格式函数，行不截断。
func endBlockLines(ev repl.RoundEndEvent) []string {
	if len(ev.Results) == 0 {
		return nil
	}
	rid := ""
	for _, res := range ev.Results {
		if res.RoundID != "" {
			rid = res.RoundID
			break
		}
	}
	lines := make([]string, 0, 1+len(ev.Results))
	lines = append(lines, segmentCell(roundRun{
		has:     true,
		label:   ev.Label,
		k:       ev.K,
		n:       ev.N,
		members: len(ev.Results),
		back:    len(ev.Results),
		roundID: rid,
	}))
	idW := memberIDWidth(ev.Results)
	for _, res := range ev.Results {
		lines = append(lines, memberDoneLine(res, idW))
	}
	return lines
}

// memberIDWidth 是一组结果里 AgentID 的最大显示宽度。
func memberIDWidth(results []repl.MemberDoneEvent) int {
	w := 0
	for _, res := range results {
		if n := ansi.StringWidth(res.AgentID); n > w {
			w = n
		}
	}
	return w
}

// memberDoneLine 是终态行：状态词、id、时间格，标记非空时再接标记串。
func memberDoneLine(ev repl.MemberDoneEvent, idW int) string {
	line := "  " + padCells(outcomeWord(ev.Outcome), 6) + "  " + padCells(ev.AgentID, idW) + "  " + outcomeTime(ev)
	if marks := outcomeMarks(ev); marks != "" {
		line += "  " + marks
	}
	return line
}

// memberRunningLine 是在跑行。已用时间由调用方按本段起点算好。
func memberRunningLine(id string, idW int, used time.Duration) string {
	return "  " + padCells("在跑", 6) + "  " + padCells(id, idW) + "  " + formatElapsed(used)
}

// outcomeWord 委托 repl.OutcomeWord；格式只有一份，简洁档成员头与此共用。
func outcomeWord(o turn.Outcome) string {
	return repl.OutcomeWord(o)
}

// outcomeTime 印时间格。没起来是问号；拒绝、丢失、未知是横线；其余是子进程时长。
func outcomeTime(ev repl.MemberDoneEvent) string {
	switch ev.Outcome {
	case turn.OutcomeSpawnFailed:
		return "?"
	case turn.OutcomeRefused, turn.OutcomeLost:
		return "-"
	case turn.OutcomeSpoke, turn.OutcomePostRejected, turn.OutcomeProviderFailed, turn.OutcomeUnparsable, turn.OutcomeTimeout, turn.OutcomeCanceled:
		return wallText(ev.WallMS)
	default:
		return "-"
	}
}

// wallText 委托 repl.FormatWall；格式只有一份，简洁档成员头与此共用。
func wallText(ms int) string {
	return repl.FormatWall(ms) + " 子进程"
}

// outcomeMarks 按「可能已吃下、正文见上方、前缀已断」的顺序拼标记。
func outcomeMarks(ev repl.MemberDoneEvent) string {
	var marks []string
	if ev.MaybeAdvanced {
		marks = append(marks, "可能已吃下")
	}
	if ev.Dropped {
		marks = append(marks, "正文见上方")
	}
	if ev.Drifted {
		marks = append(marks, "(前缀已断)")
	}
	return strings.Join(marks, "  ")
}

// padCells 在 s 后面补 ASCII 空格，直到显示宽度达到 w。已经够宽则原样返回。
func padCells(s string, w int) string {
	n := ansi.StringWidth(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// formatElapsed 委托 repl.FormatElapsed；格式只有一份，简洁档成员头与此共用。
func formatElapsed(d time.Duration) string {
	return repl.FormatElapsed(d)
}

// roundRun 是本次提交里当前这一段的表头材料。零值 has 为 false，表示还没有段。
type roundRun struct {
	has     bool
	label   string
	k       int
	n       int
	members int
	back    int
	roundID string
}

// headerStatus 按契约 §3.1 拼状态格。不忙时只有 status；忙时在有段处接段，再接已用时间。
func headerStatus(busy bool, status string, run roundRun, elapsed time.Duration) string {
	if !busy {
		return status
	}
	cell := status
	if run.has {
		cell += "  " + segmentCell(run)
	}
	return cell + "  " + formatElapsed(elapsed)
}

// segmentCell 拼段：label、仅 n > 1 时的 k/n、仅 roundID 非空时的末 6 位、回来计数。
func segmentCell(run roundRun) string {
	seg := run.label
	if run.n > 1 {
		seg += fmt.Sprintf(" %d/%d", run.k, run.n)
	}
	if run.roundID != "" {
		seg += " (" + last6(run.roundID) + ")"
	}
	seg += fmt.Sprintf("  回来 %d/%d", run.back, run.members)
	return seg
}

// last6 取 roundID 的末 6 个字节。不足 6 时整串。roundID 是 ASCII。
func last6(s string) string {
	if len(s) > 6 {
		return s[len(s)-6:]
	}
	return s
}

// elapsedTickCmd 装一拍。gen 是装的时候的代次，到点原样写进消息。
func elapsedTickCmd(gen uint64) tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return elapsedTickMsg{gen: gen, at: t}
	})
}
