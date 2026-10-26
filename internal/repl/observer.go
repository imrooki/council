package repl

import (
	"council/internal/i18n"
	"council/internal/turn"
)

// RoundObserver 接收 /round 每一段的三种事件。nil 表示不观察，行式路径不装。
//
// 调用纪律（实现方必须遵守，TUI 适配器依赖它）：
//   - RoundStart / RoundEnd 在执行 Submit 的那个 goroutine 上调用，不持任何锁。
//   - MemberDone 通常在 turn 包的 worker goroutine 上、RunRoundWithStream 的 progressMu 里调用；
//     铸 round id 失败或 round_started 写不进时，turn 改走 notifyEach，在执行 Submit 的那个
//     goroutine 上同步调用、不持锁。两种情形在同一段内都串行。实现只许做 O(1) 的轻活
//     （例如投递一条小消息），不许回调 Runner 或 REPL，不许等待任何要取 progressMu 的东西。
//   - 事件是值，接收方可以留着用，与发射方不共享任何可变内存。
type RoundObserver interface {
	RoundStart(ev RoundStartEvent)
	MemberDone(ev MemberDoneEvent)
	RoundEnd(ev RoundEndEvent)
}

// RoundStartEvent 在打出「本轮问 N 个: ...」之后、起进程之前发出。
type RoundStartEvent struct {
	Label    string   // "发言轮"、"裁判轮" 或 "总结轮"，即 runOneRound 的 label 参数
	K        int      // 发言轮：第 K 轮（1 起）；裁判轮与总结轮：0
	N        int      // 发言轮：本条命令的 --rounds 总数；裁判轮与总结轮：0
	Members  int      // len(plan.Speakers)，skipNoDelta 之后、真正起进程的人数
	Speakers []string // plan.Speakers 的新分配拷贝，顺序即起进程的顺序；len 恒等于 Members，不与 plan 共享底层数组
}

// MemberDoneEvent 在一个成员的明细文本全部写完之后发出。只带小值，不带 turn.Record。
type MemberDoneEvent struct {
	Label         string       // 同本段 RoundStartEvent.Label，已按界面语言取好
	AgentID       string       // rec.AgentID
	Outcome       turn.Outcome // rec.Outcome
	MaybeAdvanced bool         // rec.MaybeAdvanced
	Drifted       bool         // rec.Drifted
	Dropped       bool         // rec.DroppedText != ""
	WallMS        int          // rec.WallMS
	RoundID       string       // rec.RoundID（铸 id 失败时是空串）
	Lang          i18n.Lang    // 发出事件时的界面语言；零值按中文处理（TUI 观察者不改它）
}

// RoundEndEvent 在本段结算文本打完之后发出。简洁档下结算块不打（至多一行「分不清进程是否起来 N（…）」），发射时刻仍在同一位置。
type RoundEndEvent struct {
	Label   string
	K       int
	N       int
	Results []MemberDoneEvent // 本段每个成员的结果，新分配；按 Speakers 的顺序（即 turn 返回 recs 的顺序），不按回来的顺序
	Concise bool              // 发出这个事件时 REPL 的输出档位是简洁；零值是详细
	// Lang 是发出事件时的界面语言。TUI 的段终态块（结局词、标记、回来计数）
	// 靠它取文案；零值按中文处理，既有测试直接构造事件时仍是中文。
	Lang i18n.Lang
}

// SetRoundObserver 晚绑定观察者（照 LineWriter.SetEmit 的先例），传 nil 即卸下。
// 不是并发安全的：只许在还没有 Submit 在跑的时候调用（TUI 在 p.Run 之前装）。
func (r *REPL) SetRoundObserver(obs RoundObserver) {
	r.roundObs = obs
}
