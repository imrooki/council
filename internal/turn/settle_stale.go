package turn

import (
	"context"
	"fmt"

	"council/internal/conversation"
)

// StaleSettlement 是 SettleStaleInflight 的结果。
//
// Found 为假：库里没有这一 (会话, 成员) 的在飞记录，什么都没做（err 为 nil 时）。
// Found 为真：Lost 是查到的那一行与 classifyLost 给它的人话；err 为 nil 时
// Settlement 是 SettleLostTurn 的回报，err 非 nil 时这一行没结清、库里什么都没改。
type StaleSettlement struct {
	Found      bool
	Lost       LostTurn
	Settlement conversation.LostSettlement
}

// SettleStaleInflight 把 (convID, agentID) 一条陈旧的在飞记录，按启动对账的同一条路结清。
// 只由 /session rebuild 调用：结清只在用户明确敲了命令时发生。
//
// 取 g.mu 用 TryLock，拿不到就返回包了 ErrTurnInFlight 的错误，什么都不读、
// 什么都不写。本进程里这一 key 的每一轮都在同一把 g.mu 下写入在飞记录，
// 并在释放闸之前清掉或留成陈旧行；闸拿到了，查到的行就不属于任何一轮正在跑的调用。
// 拿不到就拒绝、不等，与 Rebuild 同体例：一轮最长 5 分钟，REPL 不该为一条
// 维护命令卡住这么久；那一行此刻是正在跑的那一轮自己的，不是陈旧的。
//
// 结清走 settleOne(classifyLost(行))，与启动时 SettleLostTurns 逐条走的是同一个函数：
// turn_usage 补一行 OutcomeLost、turn_lost 面包屑、provider_sessions 整行读回再标
// baseline_suspect、删在飞记录，四件一个事务；begun 行（进程没起、没花钱）只删行，
// 那是启动对账对 begun 的同一种处置。不许另写一份，也不许绕过 settleOne 直接删
// inflight_turns 的行：一次可能花了钱的轮次不能在只追加的事实源里消失。
//
// 13 §1.4 / §3.5 给启动对账的豁免是「它与 RecordTurn 在结构上不可能并发」。
// 这里不靠「只在启动时跑」，靠的是这一 key 的闸：provider_sessions 与
// inflight_turns 上这一 (会话, 成员) 的每一个写入方（askUpToWith、Rebuild、本方法）
// 都在同一把 g.mu 之下，启动对账在进 REPL 之前跑。settleOne 在事务之外读回
// ProviderSession，这一读一写之间没有别人会改那一行，理由同此。
//
// 结清成功后把 g.st 置 nil：面包屑追加了、库里的基线可能刚标成可疑，内存快照已经过期，
// 下一轮要从库里重新读回。与启动对账之后内存为空是同一个状态。
// 结清失败时 g.st 不动：库里什么都没改。
//
// 读在飞记录失败返回错误、什么都不做：「读不出来」是「不知道」，不是「没有」。
func (r *Runner) SettleStaleInflight(ctx context.Context, convID, agentID string) (StaleSettlement, error) {
	g := r.gate(sessionKey{convID: convID, agentID: agentID})
	if !g.mu.TryLock() {
		return StaleSettlement{}, fmt.Errorf("settle stale inflight %s/%s: %w", convID, agentID, ErrTurnInFlight)
	}
	defer g.mu.Unlock()

	row, found, err := r.svc.InflightFor(ctx, convID, agentID)
	if err != nil {
		return StaleSettlement{}, fmt.Errorf("settle stale inflight %s/%s: read: %w", convID, agentID, err)
	}
	if !found {
		return StaleSettlement{}, nil
	}

	lost := classifyLost(row)
	got, err := r.settleOne(ctx, lost)
	if err != nil {
		return StaleSettlement{Found: true, Lost: lost}, fmt.Errorf("settle stale inflight %s/%s: %w", convID, agentID, err)
	}
	g.st = nil
	return StaleSettlement{Found: true, Lost: lost, Settlement: got}, nil
}
