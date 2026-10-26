package repl

import (
	"context"
	"strings"
	"time"

	"council/internal/conversation"
	"council/internal/turn"
)

// summaryLabel 是总结段的标签。TUI 表头、详细档结算块标题（总结轮结算）、
// 段终态块标题都用它，与「发言轮」「裁判轮」同一体例。
const summaryLabel = "总结轮"

const (
	msgSummaryNotConfigured = "总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）"
	msgSummaryNoReply       = "总的来看：没有总结（这条 /round 没有成员回复）"
)

// runSummary 在发言轮与（若有）裁判轮都结束、且没有提前返回之后决定要不要总结。
// 判据按这个顺序，走到哪条就停：
//  1. 带了 --no-summary：调用方在进来之前已经返回，本函数看不到这条。
//  2. 本条命令里真正送进 RunRoundPreparedStream 的成员（asked，按 AgentID 去重）
//     少于 2 个：不总结，一行都不打。
//  3. ctx 已取消（Ctrl-C 已经发生）：不总结，一行都不打。
//  4. 没配总结成员（sumID 为空）：简洁档打一行提示，详细档一行都不打。
//  5. 这些成员里没有一个这一条命令里落了库的发言（rec.Message != nil）：
//     打一行「没有总结（这条 /round 没有成员回复）」，不 Join、不起进程、不花钱。
//  6. 总结成员已退出：打一行说明，不拉回、不总结。
//  7. 否则：取一次新快照，必要时 Join，若有成员最后一次没有回复就先打一行注明，再跑总结段。
//
// 第 1-6 条都不拍快照，第 1-5 条都不读成员表。两条既有用例数 Store.MaxSeq
// （发言轮、裁判轮各一次），多拍一次快照会让这两条数 MaxSeq 的既有用例变红；
// 第 1-5 条不读成员表是设计约束，没有用例数它。
//
// asked 是本条命令里真正送进 RunRoundPreparedStream 的记录（发言轮各段与裁判轮），
// 按 AgentID 去重、以第一次出现的顺序排（发言轮是字典序，裁判随后）。
// 注明行的判据恰好是 rec.Message == nil，不枚举结局：PostRejected 也要注明。
// 「最后一次没有回复」取的是每个成员在 asked 里的最后一条记录。--rounds N 时
// 前面几轮说过话、最后一轮失败的成员也要注明，总结看到的是它前几轮的话。
// 总结成员看不到 system 消息（filterDelta / selectBootstrap 都丢 KindSystem），
// 失败只能由 Council 注明（#45）。
//
// 已退出不拉回，与 /team use 同一裁决：/leave 是用户的明确意图，对总结成员来说
// 它正好是「这个会话里别再总结」的开关。不在表里才 Join，Join 不打任何行
// （/who 里看得到它，屏幕上它的名字随后出现在总结头里）。
//
// sumID 来自 cmdRound 的 snap0.Cfg。这里的 snap 是同一条命令里再拍的一次。
// 命令串行，/config reload 只在两条命令之间换配置，所以两次的 Cfg 是同一个指针
// （sumID 非空时 snap.Cfg 非 nil）。
//
// 总结段走与裁判轮同一条路（runOneRound 到 askUpToWith）。A 之五在飞闸、指纹、
// BeginTurn、超时、RecordTurn 一道都没绕过。返回的 bool 不看：能走到这里说明
// 前面已有成员落了发言，CO1 的 spawned 已经是真。
// 总结的屏幕位置在 CO1 defer 打的总用时之前。
func runSummary(ctx context.Context, r *REPL, asked []turn.Record, sumID string) error {
	var order []string
	last := make(map[string]turn.Record, len(asked))
	replied := false
	for _, rec := range asked {
		if _, seen := last[rec.AgentID]; !seen {
			order = append(order, rec.AgentID)
		}
		last[rec.AgentID] = rec
		if rec.Message != nil {
			replied = true
		}
	}
	if len(order) < 2 {
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	if sumID == "" {
		if r.concise {
			return r.printf("%s\n", r.t("tui.summary.notConfigured", msgSummaryNotConfigured))
		}
		return nil
	}
	if !replied {
		return r.printf("%s\n", r.t("tui.summary.noReply", msgSummaryNoReply))
	}
	parts, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return r.reportErr("round", err)
	}
	joined := false
	for _, p := range parts {
		if p.AgentID != sumID {
			continue
		}
		if !p.Active {
			return r.printf("%s\n", r.t("tui.summary.agentLeft", "总的来看：没有总结（%s 已退出当前会话；要恢复总结用 /join %s）", sumID, sumID))
		}
		joined = true
	}
	snap, err := r.rn.TakeRoundSnap(ctx, r.cur.ID)
	if err != nil {
		return r.reportErr("round", err)
	}
	if !joined {
		ac := snap.Cfg.Agents[sumID]
		if err := r.svc.Join(ctx, r.cur.ID, conversation.Participant{
			ConvID:      r.cur.ID,
			AgentID:     sumID,
			DisplayName: ac.DisplayName,
			Provider:    ac.Provider,
			Active:      true,
			JoinedAt:    time.Now().UTC(),
		}); err != nil {
			return r.reportErr("round", err)
		}
	}
	var missing []string
	for _, id := range order {
		if rec := last[id]; rec.Message == nil {
			missing = append(missing, r.displayName(id)+"（"+OutcomeWordLang(r.lang, rec.Outcome)+"）")
		}
	}
	if len(missing) > 0 {
		head := r.t("tui.summary.missingHead", "这些成员最后一次没有回复，总结看不到他们这一次的话：")
		sep := r.t("tui.list.sep", "、")
		if err := r.write(head + strings.Join(missing, sep) + "\n"); err != nil {
			return err
		}
	}
	plan := turn.Plan{
		Speakers: []string{sumID},
		Judges:   []string{},
		Skipped:  []turn.SkipReason{},
	}
	_, err = runOneRound(ctx, r, plan, true, r.t("tui.round.labelSummary", summaryLabel), 0, 0, snap, writeSummaryRecord, nil)
	return err
}
