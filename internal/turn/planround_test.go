package turn

// 本文件是 Step 9 片 3a（docs/12_STEP9_CONTRACT.md 4.1 节）的测试，只测
// Plan 与 PlanRound 这一对纯函数。背对背写：实现方（grok）与本文件在同一个
// 冻结契约（docs/12_STEP9_CONTRACT.md，HEAD = 05ca15f）下独立开工，互不读
// 对方产出。落笔时 Plan / PlanRound / SkipReason 一个符号都不存在，本文件
// 编译不过是预期状态，不是错误。
//
// 冻结的形状（4.1 节原文）：
//
//	type Plan struct {
//		Speakers []string
//		Judges   []string
//		Skipped  []SkipReason
//	}
//
//	func PlanRound(members []conversation.Participant,
//		agents map[string]config.AgentConfig, requested []string) Plan
//
// SkipReason 的具体字段由实现方裁决，契约只要求「每一条都带一个可读的理由」，
// 且理由要能分辨三类起因。本文件因此只钉行为，不钉 SkipReason 的字段名或
// 枚举字面量：凡是需要检查一条 Skipped 记录的地方，只做 len(plan.Skipped)
// 计数、或者用 reflect.DeepEqual 比较两个 SkipReason 值是否相等，从不解开
// 它的内部字段、也不用子串匹配它的文本（那条坑本项目已经栽过，见任务书）。
//
// 命名前缀 p3a* 是本片测试专用的 fixture，与包里其它测试文件（router_test.go、
// setconfig_concurrency_test.go 等）的 helper 分开，避免今后谁在别的文件里
// 顺手复用出跨片耦合。
//
// 关于「map 迭代顺序必然红」这条硬约束，本文件用两条互补的用例分别覆盖：
//   - TestPlanRoundSpeakersAndJudgesSortedEvenWhenInputIsReverseOrdered：
//     结构性、单次调用就确定判定的检测——把 members 按字典序整体倒序喂进去，
//     一个「直接遍历 members（或遍历 agents 但恰好没打乱这个特定顺序）再
//     append、不排序」的实现会原样吐出倒序，必然不是字典序，一次调用就能
//     抓到，不依赖 Go 运行时的随机性。
//   - TestPlanRoundDeterministicAcrossRepeatedCalls：概率放大型检测，专门
//     堵「结构性检测覆盖不到、但确实靠 range 一个 map 拼结果」的实现——
//     Go 的 map 迭代顺序是每次 range 语句执行时现场决定的随机起点，同一个
//     map 对象在同一个进程里被 range 两次，两次遍历顺序本身就大概率不同；
//     用足够大的元素数把「偶然连续多次凑巧一样」的概率压到可以忽略，而不是
//     指望多跑几次靠运气撞见一次不同。两条用例的推理写在各自函数前的注释里。
//
// 本文件发现的、契约没有钉死的歧义，逐条列在文件末尾的注释块里，不在这里
// 重复。

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// ---------------------------------------------------------------------------
// 共用 fixture：p3a 前缀，只在本文件内使用。
// ---------------------------------------------------------------------------

// p3aP 造一个最小的、Active 的 participant。PlanRound 的输入契约没有说清楚
// Active == false 的成员该怎么处理（见文末歧义 3），本文件所有用例统一只
// 构造 Active: true 的成员，不去触碰那条未定义行为。
func p3aP(agentID string) conversation.Participant {
	return conversation.Participant{AgentID: agentID, Active: true}
}

func p3aParticipants(ids []string) []conversation.Participant {
	out := make([]conversation.Participant, len(ids))
	for i, id := range ids {
		out[i] = p3aP(id)
	}
	return out
}

// p3aAgentConfig 造一份最小可用的 AgentConfig，只有 Mode 是调用方关心的
// 变量；provider 固定用 "claude"（已注册、既有用例广泛复用，不需要真的
// 起进程）。
func p3aAgentConfig(mode config.Mode) config.AgentConfig {
	return config.AgentConfig{
		DisplayName: "p3a-agent",
		Provider:    "claude",
		Model:       "claude-fable-5-1",
		Effort:      "high",
		Role:        "p3a 测试身份，正文不作数",
		Permissions: config.PermissionRead,
		Mode:        mode,
	}
}

// p3aIDs 造 n 个形如 "<prefix>00".."<prefix>NN" 的 id，零填充两位数字，
// 保证 fmt 输出的顺序与字符串字典序一致——核对期望值时不用心算字符串比较。
func p3aIDs(prefix string, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%02d", prefix, i)
	}
	return ids
}

// p3aSpeakerIDs / p3aJudgeIDs 是放大用例共用的固定名单：8 个 discussion、
// 6 个 judge。8! = 40320、6! = 720——任何「直接遍历一个 map 再 append、
// 不排序」的实现，遍历结果偶然凑成排好序的样子的概率分别只有约
// 1/40320 与 1/720。这两个数字是后面两条放大用例（结构性 + 重复调用）
// 共用的分母。
func p3aSpeakerIDs() []string { return p3aIDs("spk", 8) }
func p3aJudgeIDs() []string   { return p3aIDs("jud", 6) }

func p3aAgentsMap(speakerIDs, judgeIDs []string) map[string]config.AgentConfig {
	agents := make(map[string]config.AgentConfig, len(speakerIDs)+len(judgeIDs))
	for _, id := range speakerIDs {
		agents[id] = p3aAgentConfig(config.ModeDiscussion)
	}
	for _, id := range judgeIDs {
		agents[id] = p3aAgentConfig(config.ModeJudge)
	}
	return agents
}

// p3aReversedSorted 把 ss 拷贝一份、按字典序升序排好，再整体倒过来。
func p3aReversedSorted(ss []string) []string {
	out := append([]string{}, ss...)
	sort.Strings(out)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// assertPlanInvariants 钉住 4.1 节的硬约束里与「单次调用返回值本身」有关的
// 那部分：Speakers 与 Judges 各自严格递增（字典序 + 不重复）、互不相交。
// 本文件几乎每条用例都会先调它一遍，再往下断言各自关心的内容。
func assertPlanInvariants(t *testing.T, plan Plan) {
	t.Helper()
	for i := 1; i < len(plan.Speakers); i++ {
		if plan.Speakers[i-1] >= plan.Speakers[i] {
			t.Fatalf("Speakers 不是严格递增的字典序，下标 %d 与 %d 处逆序或重复：%v",
				i-1, i, plan.Speakers)
		}
	}
	for i := 1; i < len(plan.Judges); i++ {
		if plan.Judges[i-1] >= plan.Judges[i] {
			t.Fatalf("Judges 不是严格递增的字典序，下标 %d 与 %d 处逆序或重复：%v",
				i-1, i, plan.Judges)
		}
	}
	seen := make(map[string]struct{}, len(plan.Speakers))
	for _, id := range plan.Speakers {
		seen[id] = struct{}{}
	}
	for _, id := range plan.Judges {
		if _, ok := seen[id]; ok {
			t.Fatalf("Speakers 与 Judges 有交集，都含 %q：Speakers=%v Judges=%v",
				id, plan.Speakers, plan.Judges)
		}
	}
}

// ---------------------------------------------------------------------------
// 用例 1（要求覆盖点 1）：requested == nil 时的全员划分，judge 不进 Speakers。
// ---------------------------------------------------------------------------

func TestPlanRoundAllMembersSplitSpeakersAndJudges(t *testing.T) {
	// members 故意不按字典序传入，顺带验证输出不依赖输入顺序。
	members := []conversation.Participant{
		p3aP("dave"), p3aP("alice"), p3aP("carol"), p3aP("bob"),
	}
	agents := map[string]config.AgentConfig{
		"alice": p3aAgentConfig(config.ModeDiscussion),
		"bob":   p3aAgentConfig(config.ModeJudge),
		"carol": p3aAgentConfig(config.ModeManager),
		"dave":  p3aAgentConfig(config.ModeDiscussion),
	}

	plan := PlanRound(members, agents, nil)
	assertPlanInvariants(t, plan)

	wantSpeakers := []string{"alice", "carol", "dave"}
	wantJudges := []string{"bob"}
	if !slices.Equal(plan.Speakers, wantSpeakers) {
		t.Fatalf("Speakers 不对：got=%v want=%v", plan.Speakers, wantSpeakers)
	}
	if !slices.Equal(plan.Judges, wantJudges) {
		t.Fatalf("Judges 不对：got=%v want=%v", plan.Judges, wantJudges)
	}
	if len(plan.Skipped) != 0 {
		t.Fatalf("四个成员全部配置齐全、全员请求，不该有任何 Skipped，实际 %+v", plan.Skipped)
	}
}

// ---------------------------------------------------------------------------
// 用例 2（要求覆盖点 2）：ModeManager 与 ModeDiscussion 同待遇，正向断言。
// ---------------------------------------------------------------------------

// TestPlanRoundManagerModeTreatedSameAsDiscussion 用「对调测试」而不是单纯
// 「manager 也在 Speakers 里」：把 manager 这个 mode 分别挂在 agentA 与
// agentB 身上跑两遍，断言两次的 Plan 逐字节相同。
//
// 为什么对调比单独断言更强：如果实现里藏着任何按身份而非按 mode 语义的分支
// （例如「manager 一定排在 Speakers 最后」这种不小心引入的特判），把 manager
// 换到另一个 id 身上就会让那个特判换个目标发作，两次的 Speakers 就会变成
// 两个不同的子集，对调前后不再相等；而 manager 与 discussion 真正同待遇的
// 实现里，两次的 Plan 必然逐字节相同——它们只看「这个 id 该不该进
// Speakers」，不看「哪个 id 恰好是 manager」。
func TestPlanRoundManagerModeTreatedSameAsDiscussion(t *testing.T) {
	members := []conversation.Participant{p3aP("agentA"), p3aP("agentB")}

	agentsManagerIsA := map[string]config.AgentConfig{
		"agentA": p3aAgentConfig(config.ModeManager),
		"agentB": p3aAgentConfig(config.ModeDiscussion),
	}
	agentsManagerIsB := map[string]config.AgentConfig{
		"agentA": p3aAgentConfig(config.ModeDiscussion),
		"agentB": p3aAgentConfig(config.ModeManager),
	}

	planManagerIsA := PlanRound(members, agentsManagerIsA, nil)
	planManagerIsB := PlanRound(members, agentsManagerIsB, nil)
	assertPlanInvariants(t, planManagerIsA)
	assertPlanInvariants(t, planManagerIsB)

	wantSpeakers := []string{"agentA", "agentB"}
	if !slices.Equal(planManagerIsA.Speakers, wantSpeakers) {
		t.Fatalf("manager 应该和 discussion 一样进 Speakers：got=%v want=%v",
			planManagerIsA.Speakers, wantSpeakers)
	}
	if len(planManagerIsA.Judges) != 0 {
		t.Fatalf("本用例没有任何 judge，Judges 应该为空，实际 %v", planManagerIsA.Judges)
	}
	if len(planManagerIsA.Skipped) != 0 {
		t.Fatalf("两个成员都配置齐全，Skipped 应该为空，实际 %+v", planManagerIsA.Skipped)
	}

	if !reflect.DeepEqual(planManagerIsA, planManagerIsB) {
		t.Fatalf("manager 与 discussion 待遇不对称：把 manager 从 agentA 换到 agentB 后 Plan 变了\n"+
			"manager=agentA 时: %+v\nmanager=agentB 时: %+v", planManagerIsA, planManagerIsB)
	}
}

// ---------------------------------------------------------------------------
// 用例 3（要求覆盖点 3 + map 顺序放大 · 概率侧）。
// ---------------------------------------------------------------------------

// TestPlanRoundDeterministicAcrossRepeatedCalls 是「同一份输入多次调用，
// 三个切片逐字节相同」这条硬约束的放大版。
//
// 「多跑几次不够可靠」到底说的是什么：Go 的 map 迭代顺序不是进程级的固定
// 种子，是每次执行 range 语句时现场决定一个随机起点（运行时 fastrand）。
// 这意味着同一个 map 对象在同一次测试里被 range 两次，两次遍历顺序本身就
// 大概率不同——不需要改动任何输入字节，光是连续调用两次 PlanRound(同一份
// members/agents) 就足以暴露「结果依赖 map 迭代顺序」这个 bug，前提是元素
// 够多。反过来，如果元素太少（比如就三四个 agent），一个乱序的实现也有
// 不小的概率连续两三次凑巧吐出同一个顺序，看起来像通过了，这就是任务书
// 点名的那个陷阱。
//
// 本用例的对策：
//  1. 用 p3aSpeakerIDs/p3aJudgeIDs（8 + 6 个 agent）加上 6 个不在 agents
//     配置里的杂散成员，让 Speakers、Judges、Skipped 三个切片各自都有
//     足够多的元素（8、6、6），对应的「偶然排对」概率分母分别是
//     8!=40320、6!=720、6!=720。
//  2. 不是只调两次比较，是调 30 次，每次都与第一次的结果整体
//     reflect.DeepEqual。一个真正依赖 map 顺序的实现，连续 30 次都凑巧
//     等于第一次的概率上界大约是 (1/720)^29（取三者里最小的分母、且
//     只算 Skipped 这一路，Speakers/Judges 的分母更大、约束更强）——
//     这不是「多跑几次走运抓到」，是把假阴性的概率压到可以放心当成
//     不会发生，而不是必须发生。真正做到「必然红」需要证明实现没有
//     排序这一结构性事实，本用例退而求其次，把「跑不出假绿」的概率
//     做到可忽略；结构性的那一半由下一条用例
//     （TestPlanRoundSpeakersAndJudgesSortedEvenWhenInputIsReverseOrdered）
//     补上。
func TestPlanRoundDeterministicAcrossRepeatedCalls(t *testing.T) {
	speakerIDs := p3aSpeakerIDs()
	judgeIDs := p3aJudgeIDs()
	agents := p3aAgentsMap(speakerIDs, judgeIDs)

	strayIDs := p3aIDs("stray", 6) // 不在 agents 里，专门喂给 Skipped。
	allIDs := append(append(append([]string{}, speakerIDs...), judgeIDs...), strayIDs...)
	members := p3aParticipants(allIDs)

	first := PlanRound(members, agents, nil)
	assertPlanInvariants(t, first)
	if len(first.Skipped) != len(strayIDs) {
		t.Fatalf("前置条件不成立，本用例测不出想测的东西：len(Skipped)=%d，期望 %d"+
			"（每个不在 agents 里的杂散成员各产生一条 Skipped）", len(first.Skipped), len(strayIDs))
	}

	const trials = 30
	for i := 0; i < trials; i++ {
		got := PlanRound(members, agents, nil)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("第 %d/%d 次调用与第一次不同，说明结果依赖 map 迭代顺序（纯函数不允许，"+
				"见 4.1 节「不依赖 map 迭代顺序」）\n第一次: %+v\n第 %d 次: %+v",
				i+1, trials, first, i+1, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 用例 4（map 顺序放大 · 结构性侧，单次调用确定判定）。
// ---------------------------------------------------------------------------

// TestPlanRoundSpeakersAndJudgesSortedEvenWhenInputIsReverseOrdered 补上
// 用例 3 覆盖不到的那一类实现：直接遍历 members 这个切片（不是遍历 agents
// 这个 map）再 append，不排序。这类实现的输出顺序就是 members 的输入顺序，
// 与 Go 的 map 随机性完全无关，重复调用多少次都不会自己暴露——用例 3 的
// 放大对它不起作用。
//
// 对策不是概率放大，是把输入选成对这类实现最不利的那一个：把 members
// 整体按字典序降序传入。不排序的实现会原样吐出降序，对 8 个或 6 个以上的
// 元素而言，降序几乎不可能恰好也是升序（只有全部元素相同才会，这里显然
// 不成立），所以这一步不需要重复调用，一次就能确定性地判定——要么实现
// 真的排序了（结果是升序，与输入顺序无关），要么没排序（结果保留了降序
// 或降序的某种打乱，必然不是升序）。
//
// 这条用例与用例 3 合起来才覆盖「遍历 members 切片不排序」与「遍历 agents
// 这个 map 不排序」两类实现，任务书说的「想清楚怎么放大」在本文件里是
// 这两条用例分别负责一半。
func TestPlanRoundSpeakersAndJudgesSortedEvenWhenInputIsReverseOrdered(t *testing.T) {
	speakerIDs := p3aSpeakerIDs()
	judgeIDs := p3aJudgeIDs()
	agents := p3aAgentsMap(speakerIDs, judgeIDs)

	allIDs := append(append([]string{}, speakerIDs...), judgeIDs...)
	members := p3aParticipants(p3aReversedSorted(allIDs))

	plan := PlanRound(members, agents, nil)
	assertPlanInvariants(t, plan)

	wantSpeakers := append([]string{}, speakerIDs...)
	sort.Strings(wantSpeakers)
	wantJudges := append([]string{}, judgeIDs...)
	sort.Strings(wantJudges)

	if !slices.Equal(plan.Speakers, wantSpeakers) {
		t.Fatalf("members 整体按字典序倒序喂入时 Speakers 不是正序：got=%v want=%v", plan.Speakers, wantSpeakers)
	}
	if !slices.Equal(plan.Judges, wantJudges) {
		t.Fatalf("members 整体按字典序倒序喂入时 Judges 不是正序：got=%v want=%v", plan.Judges, wantJudges)
	}
}

// ---------------------------------------------------------------------------
// 用例 5（要求覆盖点 4 前半）：requested 非 nil 时，点名的进、没点名的不进。
// ---------------------------------------------------------------------------

func TestPlanRoundRequestedFiltersToNamedMembers(t *testing.T) {
	members := []conversation.Participant{p3aP("alice"), p3aP("bob"), p3aP("carol")}
	agents := map[string]config.AgentConfig{
		"alice": p3aAgentConfig(config.ModeDiscussion),
		"bob":   p3aAgentConfig(config.ModeJudge),
		"carol": p3aAgentConfig(config.ModeDiscussion),
	}

	plan := PlanRound(members, agents, []string{"alice"})
	assertPlanInvariants(t, plan)

	if !slices.Contains(plan.Speakers, "alice") {
		t.Fatalf("被点名的 alice 应该出现在 Speakers 里，实际 Speakers=%v", plan.Speakers)
	}
	if slices.Contains(plan.Speakers, "carol") || slices.Contains(plan.Judges, "carol") {
		t.Fatalf("没被点名的 carol 不该出现在 Speakers 或 Judges 里，实际 plan=%+v", plan)
	}
	if slices.Contains(plan.Speakers, "bob") || slices.Contains(plan.Judges, "bob") {
		t.Fatalf("没被点名的 bob（即使 mode=judge）也不该出现在 Speakers 或 Judges 里，实际 plan=%+v", plan)
	}
	// 没被点名的 carol / bob 会不会各自进 Skipped、用什么理由，契约没写清楚
	// （见文末歧义 2）。本用例故意不断言 Skipped 的内容，只断言契约原文
	// 钉死的「点名的进、没点名的不进 Speakers/Judges」这一半。
}

// ---------------------------------------------------------------------------
// 用例 6（要求覆盖点 4 后半）：requested 点了名单外的 id -> Skipped。
// ---------------------------------------------------------------------------

// TestPlanRoundRequestedUnknownIDAddsSkippedEntry 用「加一个名单外的 id
// 前后对比」而不是直接断言 len(Skipped) 的绝对值：用例 5 已经说明「没被
// 点名的成员会不会也进 Skipped」是未定义行为，如果这里直接断言
// len(Skipped) == 1，就意味着偷偷认定了 bob/carol 不进 Skipped，那不是
// 这条用例该管的事。用差值而不是绝对值，就不需要对那条未定义行为表态。
func TestPlanRoundRequestedUnknownIDAddsSkippedEntry(t *testing.T) {
	members := []conversation.Participant{p3aP("alice")}
	agents := map[string]config.AgentConfig{"alice": p3aAgentConfig(config.ModeDiscussion)}

	baseline := PlanRound(members, agents, []string{"alice"})
	withGhost := PlanRound(members, agents, []string{"alice", "ghost-not-a-member"})
	assertPlanInvariants(t, baseline)
	assertPlanInvariants(t, withGhost)

	if !slices.Equal(baseline.Speakers, withGhost.Speakers) {
		t.Fatalf("requested 里多一个不是成员的 id 不该影响 Speakers：baseline=%v withGhost=%v",
			baseline.Speakers, withGhost.Speakers)
	}
	if len(withGhost.Skipped) != len(baseline.Skipped)+1 {
		t.Fatalf("requested 里多一个不是成员的 id（ghost-not-a-member）应该让 Skipped 恰好多一条："+
			"baseline len=%d withGhost len=%d", len(baseline.Skipped), len(withGhost.Skipped))
	}
}

// ---------------------------------------------------------------------------
// 用例 7（要求覆盖点 5）：成员不在 agents 配置里 -> Skipped。
// ---------------------------------------------------------------------------

func TestPlanRoundMemberMissingFromAgentsConfigIsSkipped(t *testing.T) {
	members := []conversation.Participant{p3aP("alice"), p3aP("ghost-unconfigured")}
	agents := map[string]config.AgentConfig{"alice": p3aAgentConfig(config.ModeDiscussion)}

	plan := PlanRound(members, agents, nil)
	assertPlanInvariants(t, plan)

	if slices.Contains(plan.Speakers, "ghost-unconfigured") || slices.Contains(plan.Judges, "ghost-unconfigured") {
		t.Fatalf("不在 agents 配置里的成员不该出现在 Speakers 或 Judges 里，实际 plan=%+v", plan)
	}
	if len(plan.Skipped) != 1 {
		t.Fatalf("恰好一个成员不在配置里，Skipped 应该恰好一条，实际 len=%d，plan=%+v", len(plan.Skipped), plan)
	}
	if !slices.Contains(plan.Speakers, "alice") {
		t.Fatalf("配置齐全的 alice 应该正常进 Speakers，实际 Speakers=%v", plan.Speakers)
	}
}

// ---------------------------------------------------------------------------
// 用例 8（冻结形状段落的难点）：不同原因的 SkipReason 要能区分开。
// ---------------------------------------------------------------------------

// TestPlanRoundSkipReasonDistinguishesCause 是任务书点名的难点：在不知道
// SkipReason 字段名的前提下，断言「不同原因产生的理由互相区分得开」。
//
// 用的方向：同一个 id "x" 在两个场景里分别因为两种不同原因被跳过——
//
//	场景 A：x 是成员，但 agents 里没有它的配置（「不在配置里」）。
//	场景 B：x 根本不是成员，只是被 requested 点了名（「点了名单外的 id」）。
//
// 用同一个 id 是关键，也是任务书给的方向里没直接写、但推一步就会踩到的坑：
// 如果 SkipReason 里带着 id 字段，两个「不同成员」各自的 SkipReason 不相等
// 只能证明「id 不一样」，证明不了「原因不一样」——一个只存 id、完全不存
// 原因的 SkipReason 也能让这种写法的断言通过，而那正是契约要求「每一条都
// 带一个可读的理由」想拦住的那种退化实现。把 id 控制成同一个，剩下能让
// 两个 SkipReason 不相等的就只有「原因」本身。
//
// 反例对照（negative control）：同一个场景（A）重复跑一遍，两次的 SkipReason
// 必须相等。没有这一条，一个理由里偷偷塞了时间戳、随机数或指针的实现，会
// 让「A 与 B 不相等」这条断言因为错误的原因通过——看起来分辨出了原因，
// 实际分辨出的只是两次调用之间不受控的噪声。
func TestPlanRoundSkipReasonDistinguishesCause(t *testing.T) {
	scenarioA := func() SkipReason {
		members := []conversation.Participant{p3aP("x")}
		agents := map[string]config.AgentConfig{}
		plan := PlanRound(members, agents, nil)
		if len(plan.Skipped) != 1 {
			t.Fatalf("场景 A 前置条件不成立：len(Skipped)=%d，期望 1", len(plan.Skipped))
		}
		return plan.Skipped[0]
	}
	scenarioB := func() SkipReason {
		members := []conversation.Participant{p3aP("y")}
		agents := map[string]config.AgentConfig{
			"x": p3aAgentConfig(config.ModeDiscussion),
			"y": p3aAgentConfig(config.ModeDiscussion),
		}
		// y 必须也在 requested 里点上名：否则 y 是「配置齐全但没被点名」的
		// 成员，而「没被点名的成员会不会也进 Skipped」正是歧义 2 没裁定的
		// 那件事——万一实现方选了「也进 Skipped」，len(Skipped) 就会变成 2，
		// 这条用例会因为一个与「理由能不能分辨」无关的原因失败，或者更糟，
		// 取到的 Skipped[0] 其实是 y 的理由而不是 x 的。把 y 也点上名，
		// 让「y 有效地被请求且配置齐全」在两种裁法下都只对应「进 Speakers」，
		// x 就是这个场景里唯一可能出现在 Skipped 里的成员。
		plan := PlanRound(members, agents, []string{"x", "y"})
		if !slices.Contains(plan.Speakers, "y") {
			t.Fatalf("场景 B 前置条件不成立：y 被点名且配置齐全，应该进 Speakers，实际 Speakers=%v", plan.Speakers)
		}
		if len(plan.Skipped) != 1 {
			t.Fatalf("场景 B 前置条件不成立：len(Skipped)=%d，期望 1", len(plan.Skipped))
		}
		return plan.Skipped[0]
	}

	a1 := scenarioA()
	b := scenarioB()
	a2 := scenarioA()

	if !reflect.DeepEqual(a1, a2) {
		t.Fatalf("反例对照失败：同一个场景（x 不在配置里）重复跑两次，SkipReason 应该相等，"+
			"实际不相等：第一次=%+v 第二次=%+v", a1, a2)
	}
	if reflect.DeepEqual(a1, b) {
		t.Fatalf("同一个 id \"x\" 因为两种不同原因被跳过（不在配置里 / requested 点了名单外的 id），"+
			"SkipReason 应该能区分开，实际两者相等：%+v", a1)
	}
}

// ---------------------------------------------------------------------------
// 用例 9（要求覆盖点 7 + 契约验收 2）：改 mode 不触发重建。
// ---------------------------------------------------------------------------

// TestPlanRoundModeChangeMovesMemberButNotFingerprint 钉住「Mode 不进指纹」
// 这条今天已经成立、但没有专门用例守着的事实：把同一个 agent 从 discussion
// 改成 judge，PlanRound 的输出要跟着变（从 Speakers 挪到 Judges），但
// IdentityFingerprint 一个字不能变。
//
// 指纹怎么算直接调 identityFPFor——与 Ask/AuditDrift 唯一的指纹计算入口
// 同一份代码（session.go 里的既有注释），不经过 Runner/SQLite：本用例只关心
// 「换 mode 是否触发漂移」，不需要真的起一条会话。
func TestPlanRoundModeChangeMovesMemberButNotFingerprint(t *testing.T) {
	base := config.AgentConfig{
		DisplayName: "Alice",
		Provider:    "claude",
		Model:       "claude-fable-5-1",
		Effort:      "high",
		Role:        "p3a 指纹用例，正文不作数",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	}

	p, err := provider.Lookup(base.Provider)
	if err != nil {
		t.Fatalf("provider.Lookup(%q) 失败: %v", base.Provider, err)
	}
	tier, err := TranslatePermission(base.Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission(%q) 失败: %v", base.Permissions, err)
	}
	workDir := t.TempDir()

	fpDiscussion, err := identityFPFor(p, "alice", base, workDir, "conv-p3a", tier, RulesBlocks{})
	if err != nil {
		t.Fatalf("identityFPFor（discussion）失败: %v", err)
	}

	judged := base
	judged.Mode = config.ModeJudge
	fpJudge, err := identityFPFor(p, "alice", judged, workDir, "conv-p3a", tier, RulesBlocks{})
	if err != nil {
		t.Fatalf("identityFPFor（judge）失败: %v", err)
	}

	if fpDiscussion != fpJudge {
		t.Fatalf("只改 Mode（discussion -> judge），identity_fp 不该变，实际 discussion=%q judge=%q",
			fpDiscussion, fpJudge)
	}

	members := []conversation.Participant{p3aP("alice")}
	planDiscussion := PlanRound(members, map[string]config.AgentConfig{"alice": base}, nil)
	planJudge := PlanRound(members, map[string]config.AgentConfig{"alice": judged}, nil)
	assertPlanInvariants(t, planDiscussion)
	assertPlanInvariants(t, planJudge)

	if !slices.Contains(planDiscussion.Speakers, "alice") || slices.Contains(planDiscussion.Judges, "alice") {
		t.Fatalf("mode=discussion 时 alice 应该在 Speakers、不在 Judges，实际 plan=%+v", planDiscussion)
	}
	if slices.Contains(planJudge.Speakers, "alice") || !slices.Contains(planJudge.Judges, "alice") {
		t.Fatalf("mode=judge 时 alice 应该在 Judges、不在 Speakers，实际 plan=%+v", planJudge)
	}
}

// ---------------------------------------------------------------------------
// 歧义清单（写进代码里，报告里逐条复述一遍，两处不冲突时以此处为准）。
// ---------------------------------------------------------------------------
//
// 1. Skipped 的顺序：契约只要求「同一份输入多次调用逐字节相同」（确定性），
//    没有要求 Skipped 也按字典序——Speakers/Judges 的「字典序」是 4.1 节
//    明文钉的，Skipped 没有同款文字。本文件因此没有对 Skipped 的顺序做
//    任何断言（既没测它排序，也没测它保留 members 的输入顺序），只测它的
//    「集合」性质（len、以及某个 SkipReason 值是否等于另一个）。
//
// 2. requested 非 nil 时，没被点名、但本身配置齐全的成员，会不会也各进一条
//    Skipped：契约原文「requested 非 nil 时：点名的进、没点名的不进」只说了
//    「不进 Speakers/Judges」，没说「不进/进 Skipped」。本文件的
//    TestPlanRoundRequestedFiltersToNamedMembers 与
//    TestPlanRoundRequestedUnknownIDAddsSkippedEntry 都刻意不对这件事表态
//    （后者用差值而不是绝对值断言 Skipped 条数，正是为了绕开这条歧义）。
//
// 3. Active == false 的 participant：contract 没有说 PlanRound 自己要不要
//    按 Active 过滤成员，还是这件事已经由调用方（REPL/Runner）在传参之前
//    做完。本文件所有用例统一只构造 Active: true 的成员，不测这条路径。
//
// 4. 「mode 决定它不发言」是「冻结的形状」段落里 SkipReason 要能分辨的三类
//    起因之一，但 docs/12_STEP9_CONTRACT.md 4.1 节的正文只明确钉了
//    ModeJudge（进 Judges，不算 Skipped）与 ModeManager（同 discussion）
//    两种 mode 的待遇，对 ModeWorker / ModeDiscussionAndWorker 一个字没提；
//    docs/02_AGENT_PROVIDER_ARCHITECTURE.md 第 2 节反而明写「今天没有任何
//    一处按 Mode 分支的行为」（2026-09-21 订正时的原话）。五个 Mode 里
//    唯一可能对应「mode 决定它不发言」的候选是 ModeWorker（纯 worker、
//    不含 discussion 能力），但这是我的推测，不是契约钉死的结论——也可能
//    是 ModeDiscussionAndWorker，或者这条理由今天根本不会被
//    PlanRound 产生（类似 no_delta，预留给以后）。本文件没有为这条起因
//    写任何正向或反向用例，避免替实现方裁决一个契约没写清楚的分支；
//    TestPlanRoundSkipReasonDistinguishesCause 只用另外两类（不在配置里 /
//    requested 点了名单外的 id）证明「理由能区分开」，不依赖这第三类。
//
// 5. requested 非 nil 但是空切片（[]string{}）：与 requested == nil（全员）
//    是不是同一种行为，契约没写。空切片按字面意思读是「明确点了 0 个人」，
//    但也可能被实现方当成「没点名」处理。本文件没有测这个输入。
//
// 6. requested 里出现重复 id（例如 ["alice", "alice"]）：契约没说要不要
//    去重、去重后算不算「点名」。本文件没有测这个输入。
//
// 7. no_delta：4.1 节写「Skipped 的理由集合必须包含 no_delta」，但
//    PlanRound 是纯函数、拿不到会话历史，产生不出这个理由——契约原文自己
//    也说这一档判定发生在 Budget 阶段。任务书明确要求不要为它写
//    「期望 PlanRound 产出 no_delta」的用例，本文件照办，一条都没写。
