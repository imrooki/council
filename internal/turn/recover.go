package turn

import (
	"context"
	"fmt"
	"sort"

	"council/internal/conversation"
)

// LostNoticeCap 是启动横幅逐行点名的条数上限。再多就折叠成一个计数：
// 一次积压不该把横幅刷成一屏。放在 turn 而不是写进横幅函数，是为了
// 全仓只有这一个出处，横幅与测试钉同一个数。
const LostNoticeCap = 10

// LostTurn 是一条「上次退出时还在飞」的记录，以及怎么读它。
type LostTurn struct {
	Turn     conversation.InflightTurn
	Reason   string // 人话，进横幅与面包屑
	OrphanID string // 点得出名字的孤儿会话 id；点不出时是空串
}

// AuditInflight 把 inflight_turns 全表读成「上次退出时还在飞」的清单。
//
// 纯读，不写任何东西。不导出给 REPL，不接任何命令。
//
// inflight_turns 是一轮开始时写的。一次飞行中的 /ask 期间做无锁读，
// 会读到那一轮自己的在飞记录，把一条正在跑的轮报成丢失。
// 假阳性会让这个审计被人忽略，等于没有。
// 所以本方法的安全性不来自任何无锁分析，只来自「结构性地只在启动时、
// app.Run 进 REPL 之前跑一次」。哪天它能中途调用，这条保证同时作废。
//
// provider_sessions 在一轮结束时才写。那是另一张表的时序，
// 不是本方法能无锁读的理由。
//
// 返回按 (ConvID, AgentID) 升序，保证输出稳定、可以写 golden 用例。
// 不依赖 Service.AllInflight 本身是否已经排好序（它按 started_at 排，
// 那是它的实现细节，不是本方法能引用的契约）。
func (r *Runner) AuditInflight(ctx context.Context) ([]LostTurn, error) {
	rows, err := r.svc.AllInflight(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit inflight: %w", err)
	}
	out := make([]LostTurn, 0, len(rows))
	for _, t := range rows {
		out = append(out, classifyLost(t))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Turn.ConvID != out[j].Turn.ConvID {
			return out[i].Turn.ConvID < out[j].Turn.ConvID
		}
		return out[i].Turn.AgentID < out[j].Turn.AgentID
	})
	return out, nil
}

// classifyLost 把一条在飞记录读成横幅与面包屑用的人话。
//
// begun 读作「进程没起、没花钱」。spawned 与 returned 都读作「起过进程」。
// 三态之外 fail-closed，按起过进程处理：宁可多说一次可能花了钱。
// OrphanID 的取法与 phase 分开：returned 与 spawned 点名同一条会话，
// 变的只是措辞（正文有没有解析成功）。
func classifyLost(t conversation.InflightTurn) LostTurn {
	orphan := ""
	reason := ""
	switch t.Phase {
	case conversation.PhaseBegun:
		reason = "进程没起，只是记录残留"
	case conversation.PhaseReturned:
		orphan = orphanSessionID(t)
		reason = "provider 确实说了话，正文没能落库"
	case conversation.PhaseSpawned:
		orphan = orphanSessionID(t)
		reason = spawnedReason(t, orphan)
	default:
		// 非法 phase 不点名任何 id：值本身已经不可信，拿它去 resume
		// 比不点名更糟。按起过进程处理是 fail-closed。
		reason = fmt.Sprintf("phase 值不合法（%s），按起过进程处理", t.Phase)
	}
	// read 档不写盘。WORKSPACE 及以上真会写，崩溃时那个目录里可能留着
	// 半截文件。这里什么都不删、什么都不回滚，只把已知的事实记下来。
	if t.Tier != "read" {
		reason += fmt.Sprintf("；工作目录 %s 可能有半截落盘的文件", t.WorkDir)
	}
	return LostTurn{Turn: t, Reason: reason, OrphanID: orphan}
}

// orphanSessionID 点得出名字时返回那个 id，点不出时返回空串。
//
// 先 ResumeSessionID：那是本轮送出去 resume 的会话，provider 侧若多了一轮，
// 多在这一条上。它为空才退到 PlannedSessionID（bootstrap 轮预铸的那个）。
// 两个都空是 claude 与 agy 的 bootstrap 崩溃，那才是真正叫不出名字的孤儿，
// 空串留给调用方去说「点不出名字」，不许把空串印进句子里糊过去。
func orphanSessionID(t conversation.InflightTurn) string {
	if t.ResumeSessionID != "" {
		return t.ResumeSessionID
	}
	return t.PlannedSessionID
}

func spawnedReason(t conversation.InflightTurn, orphan string) string {
	if t.ResumeSessionID != "" {
		return fmt.Sprintf("起过进程，结局未知；provider 会话 %s 可能已经多了一轮", orphan)
	}
	if t.PlannedSessionID != "" {
		return fmt.Sprintf("起过进程，结局未知；provider 侧可能有一条孤儿会话 %s", orphan)
	}
	return "起过进程，结局未知；这条 provider 会话点不出名字"
}

// SettleLostTurns 把审计出来的在飞记录逐条补上账、标基线、删行。
//
// 每条一笔事务，一条失败不拖累其余，错误收进 errs 继续下一条。
// marked 只数真正把 baseline_suspect 置成 1 的轮，取自
// LostSettlement.SuspectMarked，不用 phase 反推：bootstrap 轮的 phase
// 是 spawned，但库里根本没有 provider_sessions 那一行可标。
//
// 绝不把 PlannedSessionID 拿去 resume，绝不推游标。对账不写 cursor_seq
// （整行回写时那一列保持读回来的原值），OrphanID 只进面包屑与屏幕，
// 不进任何 TurnRequest。
//
// ProviderSession 的读回发生在事务之外。安全性由「只在启动时、进 REPL
// 之前跑一次」保证，与 AuditInflight 同一条理由：此刻没有飞行中的 /ask
// 会改那一行。
func (r *Runner) SettleLostTurns(ctx context.Context, lost []LostTurn) (marked int, errs []error) {
	for _, row := range lost {
		settled, err := r.settleOne(ctx, row)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if settled.SuspectMarked {
			marked++
		}
	}
	return marked, errs
}

// settleOne 组一条对账的三个入参。begun 全传 nil，只删行。
// 其余 phase 补 turn_usage、发面包屑；provider 会话行在才整行回写。
//
// 两个调用方：启动时的 SettleLostTurns，与 /session rebuild 经 SettleStaleInflight
// 在持有那一 key 的闸时结清单条。事务之外那次 ProviderSession 读回的安全性，
// 前者靠「进 REPL 之前」，后者靠闸，理由见 SettleStaleInflight。
func (r *Runner) settleOne(ctx context.Context, row LostTurn) (conversation.LostSettlement, error) {
	t := row.Turn
	if t.Phase == conversation.PhaseBegun {
		got, err := r.svc.SettleLostTurn(ctx, t.TurnID, nil, nil, nil)
		if err != nil {
			return got, fmt.Errorf("settle lost turn %s: %w", t.TurnID, err)
		}
		return got, nil
	}

	u := lostUsage(t)
	msg := lostNoticeMessage(row)

	var session *conversation.ProviderSession
	loaded, found, err := r.svc.ProviderSession(ctx, t.ConvID, t.AgentID)
	if err != nil {
		return conversation.LostSettlement{}, fmt.Errorf("settle lost turn %s: read provider session: %w", t.TurnID, err)
	}
	if found {
		// 整行拷贝、只改 BaselineSuspect。结构体必须整个读、整个写，
		// 不给任何一个字段单独的入口；只填几个字段等于把其余的抹成零值，
		// 游标、成本基线、指纹会一起丢。
		// found == false（bootstrap 轮崩溃）传 nil，不碰那张表，
		// 但 turn_usage 照补：bootstrap 正是最贵的那一轮，不补的话那笔钱
		// 在库里根本不存在。
		copied := loaded
		copied.BaselineSuspect = true
		session = &copied
	}

	got, err := r.svc.SettleLostTurn(ctx, t.TurnID, &msg, &u, session)
	if err != nil {
		return got, fmt.Errorf("settle lost turn %s: %w", t.TurnID, err)
	}
	return got, nil
}

// lostUsage 组一条「结局未知」的用量行。
//
// Seq 必须是 nil：这一轮没有产生 agent 消息，面包屑不是「这一轮说的话」。
// 五个 token 列与两个 cost 列全 nil：崩溃之后这些数不知道，写成 0 会让
// 成本视图把「没报」读成「实测为零」。
//
// ColdStart、WallMS、PromptBytes 只能填 0。这三列 NOT NULL DEFAULT 0，
// 在 Lost 行上 0 读作「不知道」而不是「零」。列不可空，所以这条纪律只能
// 由读的那一侧承担：/cost 必须把 Lost 行排除在冷启动计数之外；
// prompt_bytes == 0 正好让 Budget 的取样自动跳过 Lost 行
// （prompt_bytes == 0 的样本不能用来缩放）。两处在这里是一致的，
// 不许把这三列改成可空来「修好」。
func lostUsage(t conversation.InflightTurn) conversation.TurnUsage {
	return conversation.TurnUsage{
		TurnID:            t.TurnID,
		ConvID:            t.ConvID,
		AgentID:           t.AgentID,
		ProviderSessionID: t.ResumeSessionID,
		Seq:               nil,
		Outcome:           int(OutcomeLost),
		InputTokens:       nil,
		CacheCreate:       nil,
		CacheRead:         nil,
		OutputTokens:      nil,
		ThinkingTokens:    nil,
		CostUSD:           nil,
		CostCumulativeUSD: nil,
		ColdStart:         false,
		WallMS:            0,
		PromptBytes:       0,
		DeliveredLo:       nil,
		DeliveredHi:       nil,
		CreatedAt:         t.StartedAt,
		IdentityFP:        "",
		RoundID:           "",
	}
}

// lostNoticeMessage 组一条 KindSystem 面包屑。三个脑子列留空：
// 这条消息走 SettleLostTurn，它与 Post、RecordTurn 一样，保留 sender
// 带脑子会以 ErrInvalidSender 拒绝。messages 上的 CHECK 只约束 'user'，
// system 这一半没有 SQL 兜底。
func lostNoticeMessage(row LostTurn) conversation.Message {
	t := row.Turn
	meta := map[string]any{
		MetaEvent:      eventTurnLost,
		MetaAboutAgent: t.AgentID,
	}
	if row.OrphanID != "" {
		meta[MetaOrphanSession] = row.OrphanID
	}
	text := fmt.Sprintf("成员 %s 的一轮在上次退出时没记上账：%s", t.AgentID, row.Reason)
	return conversation.Message{
		ConvID:   t.ConvID,
		SenderID: conversation.ReservedSenderSystem,
		Kind:     conversation.KindSystem,
		Text:     text,
		Meta:     meta,
	}
}
