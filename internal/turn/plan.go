package turn

import (
	"fmt"
	"sort"

	"council/internal/config"
	"council/internal/conversation"
)

// Plan 是一轮要唤醒谁、谁当裁判、谁被跳过。
//
// Speakers 与 Judges 按字典序，且互不相交：一个 id 不会同时出现在两个切片里。
// 不相交靠分类 switch 一次落入一个分支，不靠事后再滤一遍——事后过滤会把
// 写错的分支藏过去。
//
// 三个切片都不为 nil：空名单是长度为 0 的切片。nil 与空切片的字节表示不同，
// 纯函数的「跑两次逐字节相同」不应建立在调用方用哪一种比较上。
type Plan struct {
	Speakers []string
	Judges   []string
	Skipped  []SkipReason
}

// SkipKind 是跳过一条候选的机器可分档。屏幕上印的是 SkipReason.Reason，
// 后面 Budget 按档聚合的是 Kind。两套拆开的理由：契约 4.1 要把 no_delta
// 单独一档（结算行的 N 只数真正起了进程的），而给人看的句子会改；拿句子
// 当档，文案一改聚合就对不上。
//
// 用字符串而不是 iota：契约把这一档的名字写成 no_delta，日志和测试要对上
// 这个词；iota 在中间插一个常量就会让已落盘的数字全部错位。代价：拼错档名
// 编译器不拦，所以本包只通过下面这组常量赋值，不在别处写字面量。
type SkipKind string

const (
	// SkipNotInConfig：members / requested 里的 id 在 agents 里查不到。
	SkipNotInConfig SkipKind = "not_in_config"
	// SkipNotOnRoster：requested 点了一个不在 members 里的 id。
	SkipNotOnRoster SkipKind = "not_on_roster"
	// SkipMode：mode 决定它本轮不发言。PlanRound 只在 Mode 不是五值枚举
	// 之一时发出这一档（fail-closed）。ModeJudge 不走这里，走 Judges。
	SkipMode SkipKind = "mode"
	// SkipNoDelta：本轮 Select 之后没有可投递的新内容。契约 4.1 要求
	// Skipped 的理由集合包含这一档。
	//
	// PlanRound 自己永远不产生它。判它需要 Select 的结果和游标，而
	// PlanRound 的签名里没有会话历史、没有游标、没有消息。给它加一个
	// 历史参数会把本函数从纯分类变成要喂数据的函数，本片的全部价值
	// 就是它是纯的。产生这一档的是片 3c 的 Budget：起进程之前 Select
	// 已经跑得完，那时才拿得到「有没有 delta」。
	SkipNoDelta SkipKind = "no_delta"
)

// SkipReason 是 Plan.Skipped 的一条。
//
// 形状切成 AgentID + Kind + Reason，而不是一个裸 string：
//   - AgentID 点名是谁，Budget 和屏幕都要这个键；
//   - Kind 是封闭分档，给后面按档聚合；
//   - Reason 是一句人能读的话。验收要求「每条都有可读理由」，Kind 的
//     英文字面量（not_in_config）不算可读理由。
//
// 不做成 error：跳过是计划的一部分，不是调用失败。
type SkipReason struct {
	AgentID string
	Kind    SkipKind
	Reason  string
}

// PlanRound 按 Mode 把一轮的候选收成 Speakers / Judges / Skipped。
//
// requested 为 nil 表示全员（members 里出现过的每个 AgentID）；非 nil
// 表示调用方已经点好名，本函数不解析任何文本。空切片（非 nil）不是全员，
// 是「点了零个人」——与签名注释里「nil 表示全员」同一条界。
//
// 本函数不解析任何文本、不看时钟、不看随机源、不遍历 agents map
// （只按 id 查找）。同一份输入跑两次，三个切片逐字节相同。
//
// 分类：
//   - ModeJudge 进 Judges，不进 Speakers。裁判要看的是本轮结论，
//     不能与 Speakers 共用同一个 upTo（契约 4.7）；不进 Speakers
//     是这条时间线的结构保证，不只是角色不同。
//   - ModeDiscussion、ModeWorker、ModeDiscussionAndWorker、ModeManager
//     进 Speakers。
//   - ModeManager 与 ModeDiscussion 完全同待遇，是有意的剩余项：
//     02 §2 只说了 manager 与 judge 判别器同型，没说它做什么；
//     编一个就是发明没设计过的语义。代价：本步会按讨论者唤醒
//     manager，付一轮的钱。
//   - ModeWorker 与 ModeDiscussionAndWorker 同样按发言者收，同一条
//     理由：07a 冻了这两个值，但没有写下它们在一轮发言里做什么。
//     按「worker 不发言」收就是发明。代价：会唤醒 worker；三个 mode
//     对本函数的输出相同，这个冗余留着。
//   - 五值之外（含空串）进 Skipped，Kind=SkipMode。Validate 本该拦住，
//     调用方仍可能不经 Load 塞一份 AgentConfig，这里 fail-closed。
//
// members 是调用方交来的候选名单，本函数不二次过滤 Active。
// Participant 零值的 Active 是 false，若这里拦，只填了 AgentID 的
// 调用会得到空 Speakers，等于把「没置位」读成「已退出」。活不活由
// 调用方拉 Participants 时的 includeInactive 决定。
//
// 本函数是包级函数，不是 *Runner 的方法：它不需要 Runner 的任何状态。
// 做成方法会让以后的人顺手读 r.cfg 或会话，纯函数性就从结构上没了。
func PlanRound(members []conversation.Participant, agents map[string]config.AgentConfig, requested []string) Plan {
	roster := make(map[string]struct{}, len(members))
	unique := make([]string, 0, len(members))
	for _, p := range members {
		if _, ok := roster[p.AgentID]; ok {
			continue
		}
		roster[p.AgentID] = struct{}{}
		unique = append(unique, p.AgentID)
	}

	speakers := make([]string, 0)
	judges := make([]string, 0)
	skipped := make([]SkipReason, 0)

	addSkip := func(id string, kind SkipKind, reason string) {
		skipped = append(skipped, SkipReason{AgentID: id, Kind: kind, Reason: reason})
	}

	var candidates []string
	if requested == nil {
		candidates = unique
	} else {
		seenReq := make(map[string]struct{}, len(requested))
		for _, id := range requested {
			if _, dup := seenReq[id]; dup {
				continue
			}
			seenReq[id] = struct{}{}
			if _, on := roster[id]; !on {
				addSkip(id, SkipNotOnRoster, fmt.Sprintf("%s 不在成员名单上", id))
				continue
			}
			candidates = append(candidates, id)
		}
	}

	seenCand := make(map[string]struct{}, len(candidates))
	for _, id := range candidates {
		if _, dup := seenCand[id]; dup {
			continue
		}
		seenCand[id] = struct{}{}
		ac, ok := agents[id]
		if !ok {
			addSkip(id, SkipNotInConfig, fmt.Sprintf("%s 不在配置里", id))
			continue
		}
		switch ac.Mode {
		case config.ModeJudge:
			judges = append(judges, id)
		case config.ModeDiscussion, config.ModeWorker, config.ModeDiscussionAndWorker, config.ModeManager:
			speakers = append(speakers, id)
		default:
			addSkip(id, SkipMode, fmt.Sprintf("%s 的 mode=%q 本轮不发言", id, ac.Mode))
		}
	}

	sort.Strings(speakers)
	sort.Strings(judges)
	sort.Slice(skipped, func(i, j int) bool {
		if skipped[i].AgentID != skipped[j].AgentID {
			return skipped[i].AgentID < skipped[j].AgentID
		}
		if skipped[i].Kind != skipped[j].Kind {
			return skipped[i].Kind < skipped[j].Kind
		}
		return skipped[i].Reason < skipped[j].Reason
	})

	return Plan{Speakers: speakers, Judges: judges, Skipped: skipped}
}
