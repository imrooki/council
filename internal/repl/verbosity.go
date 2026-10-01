package repl

import (
	"context"
	"fmt"
	"time"

	"council/internal/turn"
)

const (
	msgVerboseUsage = "用法: /verbose 在简洁与详细之间切换；/verbose on 切到详细；/verbose off 切到简洁"
	msgVerboseOn    = "已切到详细输出：用量、金额、冷暖、会话 id、预算表与结算全部显示；/verbose off 回到简洁"
	msgConciseOn    = "已切到简洁输出：只显示成员的回复、用时，以及关系到钱或内容丢失的提示；/verbose on 切到详细"
)

// 输出档位只有两档，格式只有一份。
//
// 详细档是今天的输出，一个字节不改。简洁档是 #45 第二节的清单：成员头加正文，
// 关系到钱或内容丢失的提示照打；用量、金额、冷暖、会话 id、seq、
// 预算表（带 --go 时）、结算不打。
// 零值是详细，所以直接 New 出来的 REPL（既有测试）行为不变；
// app 装配时按 Options.Verbose 设定（默认简洁）。
// 档位判断散在各个既有打印点里（writeAskRecord、writeAskMeter、runOneRound、cmdRound、postUserText），
// 没有第二套渲染函数。理由见 round.go：两份格式会分叉，而分叉的那一天没有任何测试会红。
//
// SetConcise 设定输出档位：on 为 true 是简洁，false 是详细。
// 与 SetRoundObserver 同一条纪律，只许在没有命令在跑时调用
// （app 在 Run / tui.Run 之前调；/verbose 本身是一条命令，命令是串行派发的）。
// /round 的回调在 turn 的 worker goroutine 上读 r.concise，那时 REPL 主 goroutine
// 阻塞在 RunRoundPreparedStream 里，不会有人写它，所以不加锁。
func (r *REPL) SetConcise(on bool) {
	r.concise = on
}

// clock 取总用时用的时钟。now 为 nil 时用 time.Now。
// 只有 cmdRound 调它（总用时）。测试经 now 注入假时钟。
func (r *REPL) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// displayName 取屏幕上的成员名，每次现读 r.cfg，不缓存。
// /config reload --apply（configcmd.go 换掉 r.cfg 指针）之后，新配置里的名字立刻生效；
// 缓存会让屏幕上的名字停在旧配置。配置里没有这个 id（成员还在、配置删了），
// 或 display_name 为空时，回落到 id。只用于屏幕，不进 prompt
// （internal/turn/router.go 仍用 SenderID）。
func (r *REPL) displayName(id string) string {
	if ac, ok := r.cfg.Agents[id]; ok && ac.DisplayName != "" {
		return ac.DisplayName
	}
	return id
}

// cmdVerbose 切换或设定输出档位。不带参数是切换；on / off 是设定
// （只认小写、只认恰好一个参数）；别的一律打用法、不改档位。
// 无前置条件：不需要会话、持久化或 Runner。
func cmdVerbose(_ context.Context, r *REPL, args cmdArgs) error {
	switch {
	case len(args.fields) == 0:
		r.concise = !r.concise
	case len(args.fields) == 1 && args.fields[0] == "on":
		r.concise = false
	case len(args.fields) == 1 && args.fields[0] == "off":
		r.concise = true
	default:
		return r.printf("%s\n", msgVerboseUsage)
	}
	if r.concise {
		return r.printf("%s\n", msgConciseOn)
	}
	return r.printf("%s\n", msgVerboseOn)
}

// FormatWall 是子进程用时的唯一格式。负数当 0；不足一分钟向零截断到十分之一秒
// （不四舍五入：59999 印 59.9s，不是 60.0s）；一分钟起交给 FormatElapsed。
// 简洁档的成员头、总用时那一行、TUI 段终态块的时间格（tui.wallText 在它后面接「 子进程」）共用它。
func FormatWall(ms int) string {
	if ms < 0 {
		ms = 0
	}
	if ms < 60000 {
		return fmt.Sprintf("%d.%ds", ms/1000, (ms%1000)/100)
	}
	return FormatElapsed(time.Duration(ms) * time.Millisecond)
}

// FormatElapsed 把已用时间印成表头上的那一格。
// 向零截断到秒；负数夹成 0。不足 1 分钟印 Ns，不足 1 小时印 NmSSs，再长印 NhMMm（丢掉秒）。
// 从 internal/tui/roundobs.go 原样搬来。TUI 表头、在跑行、收尾行经 tui.formatElapsed 委托调用它，所以 TUI 的字节不变。
func FormatElapsed(d time.Duration) string {
	s := int64(d / time.Second)
	if s < 0 {
		s = 0
	}
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh%02dm", s/3600, (s%3600)/60)
}

// OutcomeWord 把结局印成六个显示宽以内的状态词。未知值归「未知」。
// 从 internal/tui/roundobs.go 原样搬来。简洁档失败行的结局词与 TUI 看板、段终态块的状态词是同一份。
func OutcomeWord(o turn.Outcome) string {
	switch o {
	case turn.OutcomeSpoke:
		return "完成"
	case turn.OutcomePostRejected:
		return "未入库"
	case turn.OutcomeProviderFailed:
		return "报错"
	case turn.OutcomeUnparsable:
		return "认不出"
	case turn.OutcomeTimeout:
		return "超时"
	case turn.OutcomeSpawnFailed:
		return "没起来"
	case turn.OutcomeRefused:
		return "拒绝"
	case turn.OutcomeCanceled:
		return "取消"
	case turn.OutcomeLost:
		return "丢失"
	default:
		return "未知"
	}
}
