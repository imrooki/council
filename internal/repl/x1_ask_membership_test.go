package repl

// 本文件是测试腿交付物，覆盖冻结契约 E:\multi_model_cowork\x1_ask_membership_contract.md
// 第 6 节的十条验收：cmdAsk 在起进程之前查一次「这个 agent 是不是本会话的
// 活跃成员」，不是成员就不许走到 r.rn.Ask。
//
// 背对背纪律：本文件没有读 internal/repl/ask.go 里 cmdAsk 的新版正文，
// 断言全部依据契约第 4.1 节贴出的 cmdAsk 旧版逐字原文与第 5、6 节的文字
// 推出。契约允许读的 cmdSession / writeAskRecord / askCause 三个函数本文件
// 也没有直接调用——这十条验收只需要经 REPL 的公开命令入口驱动，不需要
// 单独调用它们。
//
// 核心观测手段——「Ask 一次都没被调」怎么判：
// internal/turn/runner.go 的 Runner.Ask -> askUpToWith 从最顶上铸出
// TurnID 起，几乎每一条返回路径（refuse、failClosed、成功路径）最终都会
// 经 persistTurn 或直接调 Service.RecordTurn，写一条 turn_usage 行——
// 哪怕这一轮在 cfg.Agents 查找、或 provider.Lookup 那一步就被拒绝也一样
// （runner.go 现查：refuse 调 persistTurn，persistTurn 只要 rec.TurnID 非空
// 就调 svc.RecordTurn；TurnID 在 askUpToWith 一进来、任何检查之前就已经铸
// 好）。conversation.Service.RecordTurn 只有在 msg（agent 的回复消息）非
// nil 时才会走 RequireActiveParticipant 那道成员校验（service.go 现查）；
// 这些起进程之前的拒绝传的 msg 恒为 nil，所以这条 turn_usage 写入完全不
// 依赖 agent 是不是本会话的成员——这正是它能在本文件里当「Ask 被调过」判据
// 的原因，也是为什么它能覆盖到「配置里根本没有这个 id」这种连 Runner 自己
// 的配置查找都过不去的极端情形。
// TestAskGateDetectionChannelSelfTest 直接绕开 cmdAsk、直接调 rn.Ask，
// 实测证明了这条channel在本文件用到的两种夹具形状下确实成立。
//
// agent id 的选取：A/B/C1 三档共用同一个 id（newTestREPLConfig 里已有的
// ag_alpha），只改它在各自那条会话里的成员状态；C2 用一个确定不在
// newTestREPLConfig 配置里的 id。选同一个 id 是为了验收 9（三档答复必须
// 互不相同）能把"换了个 id 所以文本恰好不同"这种巧合排除掉——比较前会把
// 各自用到的 id 统一替换成同一个占位符再比。
//
// Runner 用 ask_test.go 已有的 newTestRunner（provider "pv_alpha" 不在
// internal/provider 的全局 registry 里），REPL 用 ask_test.go 已有的
// runREPLRunner（内部固定 New(newTestREPLConfig(), ...)）——两者背后是
// 同一份 newTestREPLConfig()，"配置里有没有这个 id"在 REPL 与 Runner 两侧
// 看到的是一致的答案，不会出现"REPL 认得、Runner 不认得"这种两份配置对不上
// 的假象。
//
// 泄漏披露：写作过程中为了确认"正在问"“已退出”“配置里没有”“列出成员失败”
// 这几个候选词会不会在别处出现，对 internal 目录做过一次不限定文件的
// strings 检索，副作用是看到了 internal/repl/ask.go 当前版本的若干行片段
// （含 cmdAsk 内部两行、以及 askCause 一个分支的具体措辞）。详细时间线与
// 对断言设计的影响写在最终报告里；本文件里所有依赖这几个候选词的断言都已
// 改成不依赖那次意外看到的具体字面文本（例如 C2 那一档特意不对"配置里
// 没有"/"/agents" 断言，理由见 TestAskUnconfiguredAgentNeverCallsAskAndDiffersFromNotJoined
// 的注释），只用结构性信号（turn_usage 计数、与其它档的差异比较）判别。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"council/internal/conversation"
	"council/internal/turn"
)

// x1MemberAgentID 是 A / B / C1 三档共用的 agent id，取自 newTestREPLConfig
// 已有的三个 agent 之一。选 ag_alpha 只是习惯（ask_test.go 大量既有用例
// 也用它），不是必须。
const x1MemberAgentID = "ag_alpha"

// x1UnconfiguredAgentID 是 C2 档专用的 id，确定不在 newTestREPLConfig() 的
// 三个 agent（ag_alpha / ag_mid / ag_zeta）里。
const x1UnconfiguredAgentID = "ag_not_in_config"

// x1TurnUsageCount 读回 (convID, agentID) 名下当前的 turn_usage 行数。
// 这是本文件判定「Ask 有没有被调」的核心观测手段，理由见文件顶部注释与
// TestAskGateDetectionChannelSelfTest。
func x1TurnUsageCount(t *testing.T, svc *conversation.Service, convID, agentID string) int {
	t.Helper()
	rows, err := svc.TurnUsage(context.Background(), convID, agentID, -1)
	if err != nil {
		t.Fatalf("读 TurnUsage 失败: %v", err)
	}
	return len(rows)
}

// x1NewActiveMemberFixture 造一套「A 档：活跃成员」的夹具：ag_alpha 是这条
// 会话当前的活跃 participant。
func x1NewActiveMemberFixture(t *testing.T) (svc *conversation.Service, f *fakeStore, rn *turn.Runner, convID string) {
	t.Helper()
	svc, f = newTestService(t)
	rn = newTestRunner(t, svc)
	convID = "conv_x1_active000000000000000000000000"
	f.seedConversation(convID, "A：活跃成员")
	f.seedParticipant(t, convID, x1MemberAgentID)
	return svc, f, rn, convID
}

// x1NewConfiguredNotJoinedFixture 造一套「C1 档：配置里有、没加入」的夹具：
// ag_alpha 在 newTestREPLConfig() 里，但这条会话从未 seedParticipant 过它。
func x1NewConfiguredNotJoinedFixture(t *testing.T) (svc *conversation.Service, f *fakeStore, rn *turn.Runner, convID string) {
	t.Helper()
	svc, f = newTestService(t)
	rn = newTestRunner(t, svc)
	convID = "conv_x1_notjoined0000000000000000000000"
	f.seedConversation(convID, "C1：配置里有、未加入")
	return svc, f, rn, convID
}

// x1NewLeftMemberFixture 造一套「B 档：曾加入、已 /leave」的夹具：直接调
// AddParticipant 再 DeactivateParticipant，绕开 /leave 的回显（回显里也含
// agent id，混进输出会污染本文件的 Contains 断言，做法与 round_test.go /
// session_inflight_test.go 一致）。
func x1NewLeftMemberFixture(t *testing.T) (svc *conversation.Service, f *fakeStore, rn *turn.Runner, convID string) {
	t.Helper()
	svc, f = newTestService(t)
	rn = newTestRunner(t, svc)
	convID = "conv_x1_left00000000000000000000000000"
	f.seedConversation(convID, "B：曾加入、已退出")
	f.seedParticipant(t, convID, x1MemberAgentID)
	if err := f.DeactivateParticipant(context.Background(), convID, x1MemberAgentID, replTestNow); err != nil {
		t.Fatalf("塞入退出状态失败: %v", err)
	}
	return svc, f, rn, convID
}

// x1NewUnconfiguredFixture 造一套「C2 档：配置里根本没有」的夹具。这条
// 会话本身是干净的，x1UnconfiguredAgentID 也从未出现在 newTestREPLConfig()
// 里，不需要任何 seedParticipant。
func x1NewUnconfiguredFixture(t *testing.T) (svc *conversation.Service, f *fakeStore, rn *turn.Runner, convID string) {
	t.Helper()
	svc, f = newTestService(t)
	rn = newTestRunner(t, svc)
	convID = "conv_x1_unconfigured00000000000000000000"
	f.seedConversation(convID, "C2：配置里根本没有")
	return svc, f, rn, convID
}

// ============================================================
// 验收 1、2：活跃成员，行为不变
// ============================================================

// TestAskActiveMemberBehaviorUnchanged 覆盖验收 1：活跃成员不带文本时，
// 行为要与改动前完全一样——打出「正在问 <id> ...」，且真的走到 Ask。
//
// 判别力：正在问那一行来自契约 4.1 贴出的 cmdAsk 旧版原文，一个字没改
// （契约 3 节禁止改它），只是位置可能挪到成员闸之后；turn_usage 行数
// +1 是独立信号，证明"打出了这句话"与"真的调用了 Ask"两件事都成立——
// 只查前者测不出"闸把它当成员放行了、但后面又把 Ask 短路掉了"这种坏实现，
// 只查后者测不出"闸位置插错了、放行之后没有打招呼就直接调用"这种坏实现。
func TestAskActiveMemberBehaviorUnchanged(t *testing.T) {
	svc, _, rn, convID := x1NewActiveMemberFixture(t)
	before := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+x1MemberAgentID+"\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	if !strings.Contains(out, "正在问 "+x1MemberAgentID+" ...") {
		t.Errorf("活跃成员应当照旧打出「正在问 %s ...」；完整输出:\n%s", x1MemberAgentID, out)
	}

	after := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)
	if after != before+1 {
		t.Fatalf("验收 1 的核心：活跃成员应当真的走到 r.rn.Ask（哪怕随后被 provider.Lookup 拒绝，"+
			"也会经 refuse -> persistTurn -> RecordUsage 落下一条 turn_usage 行）。"+
			"调用前 %d 条、调用后 %d 条，期望恰好 +1；完整输出:\n%s", before, after, out)
	}
}

// TestAskActiveMemberWithTextProceedsToAsk 覆盖验收 2：带文本时 user 消息
// 照常落库，然后走到 Ask。
//
// 判别力：消息形状（sender/kind/text）证明落库路径没有被闸绕过或改写；
// turn_usage +1 证明落库之后确实继续往下走到了 Ask，不是落完库就提前
// return——这条闸必须放在 postUserText 之前（契约 5.5），但不能连带把
// 后面「落完库继续问」这一步也误伤掉。
func TestAskActiveMemberWithTextProceedsToAsk(t *testing.T) {
	svc, f, rn, convID := x1NewActiveMemberFixture(t)
	const body = "你好"
	before := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+x1MemberAgentID+" "+body+"\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	msgs := f.msgs[convID]
	if len(msgs) != 1 {
		t.Fatalf("活跃成员带文本应当恰好落 1 条 user 消息，实际 %d 条；完整输出:\n%s", len(msgs), out)
	}
	if msgs[0].Text != body {
		t.Errorf("落库正文是 %q，期望 %q", msgs[0].Text, body)
	}
	if msgs[0].SenderID != conversation.ReservedSenderUser {
		t.Errorf("落库 sender 是 %q，期望保留的 %q", msgs[0].SenderID, conversation.ReservedSenderUser)
	}
	if msgs[0].Kind != conversation.KindUser {
		t.Errorf("落库 kind 是 %q，期望 %q", string(msgs[0].Kind), string(conversation.KindUser))
	}

	after := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)
	if after != before+1 {
		t.Fatalf("验收 2：带文本的活跃成员同样应当真的走到 Ask，调用前 %d 条、调用后 %d 条，"+
			"期望恰好 +1；完整输出:\n%s", before, after, out)
	}
}

// ============================================================
// 验收 3、4：配置里有、未加入
// ============================================================

// TestAskConfiguredNotJoinedNeverCallsAskAndNamesJoin 覆盖验收 3、4、10
// 里 C1 那一部分：Ask 一次都不该被调，答复要点名这个 id 并提到 /join，
// 且不该出现「正在问」。
//
// "/join" 子串的风险已排查：本用例的输入只有 "/use <convID>\n/ask
// ag_alpha\n" 两行，不经过 /team use、/leave，commands.go 里另外两处会
// 打印 "/join" 的分支（cmdTeamUse 的"已退出，未自动拉回"提示、cmdJoin
// 自己的成功回显）都不在这条路径上，能打出 "/join" 的只可能是本条要测的
// 成员闸自己。
func TestAskConfiguredNotJoinedNeverCallsAskAndNamesJoin(t *testing.T) {
	svc, _, rn, convID := x1NewConfiguredNotJoinedFixture(t)
	before := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+x1MemberAgentID+"\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	after := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)
	if after != before {
		t.Fatalf("验收 3 的核心：%s 在配置里但没加入本会话，Ask 一次都不该被调，"+
			"turn_usage 行数不该变化。调用前 %d 条、调用后 %d 条；完整输出:\n%s",
			x1MemberAgentID, before, after, out)
	}
	if strings.Contains(out, "正在问") {
		t.Errorf("验收 10：拒绝路径上不该出现「正在问」；完整输出:\n%s", out)
	}
	if !strings.Contains(out, x1MemberAgentID) {
		t.Errorf("验收 4：答复应当点名 %s；完整输出:\n%s", x1MemberAgentID, out)
	}
	if !strings.Contains(out, "/join") {
		t.Errorf("验收 4：答复应当提到 /join；完整输出:\n%s", out)
	}
}

// ============================================================
// 验收 5：曾加入、已退出
// ============================================================

// TestAskLeftMemberNeverCallsAskAndDiffersFromNotJoined 覆盖验收 5、10：
// 已 /leave 的成员，Ask 一次都不该被调，且答复要点名这个 id、不该出现
// 「正在问」，还要与 C1（配置里有、未加入）的答复不同——这是契约 5.1
// 最后一段点名的坑：Participants 的 includeInactive 若被漏传成 false，
// "已退出"会掉进"没加入"那一支，两档答复会逐字节相同。
func TestAskLeftMemberNeverCallsAskAndDiffersFromNotJoined(t *testing.T) {
	svc, _, rn, convID := x1NewLeftMemberFixture(t)
	before := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+x1MemberAgentID+"\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	after := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)
	if after != before {
		t.Fatalf("验收 5 的核心：已退出的成员，Ask 一次都不该被调。调用前 %d 条、调用后 %d 条；"+
			"完整输出:\n%s", before, after, out)
	}
	if strings.Contains(out, "正在问") {
		t.Errorf("验收 10：拒绝路径上不该出现「正在问」；完整输出:\n%s", out)
	}
	if !strings.Contains(out, x1MemberAgentID) {
		t.Errorf("答复应当点名 %s；完整输出:\n%s", x1MemberAgentID, out)
	}

	svcC1, _, rnC1, c1ConvID := x1NewConfiguredNotJoinedFixture(t)
	c1Out, err := runREPLRunner(t, svcC1, rnC1, "/use "+c1ConvID+"\n/ask "+x1MemberAgentID+"\n")
	if err != nil {
		t.Fatalf("对照组（C1）Run 失败: %v", err)
	}
	if out == c1Out {
		t.Errorf("验收 5 明写「与第 4 条的答复不同」：同一个 agent id，「已退出」与「配置里有、"+
			"未加入」两档答复逐字节相同，用户没法区分「退出过」与「从没来过」；"+
			"\n已退出:\n%s\n未加入:\n%s", out, c1Out)
	}
}

// ============================================================
// 验收 6：配置里根本没有
// ============================================================

// TestAskUnconfiguredAgentNeverCallsAskAndDiffersFromNotJoined 覆盖验收
// 6、10：配置里完全没有的 id，Ask 一次都不该被调，且答复与 C1 不同、
// 不该出现「正在问」。
//
// 这一档特意不对具体措辞（比如是否含"配置里没有"/"/agents"这类词）做
// 字面断言：09a 契约明令命令输出的具体措辞不属于契约面；更关键的是，
// 若坏实现根本没有闸、直接把 agentID 送进 r.rn.Ask，askUpToWith 会在
// cfg.Agents 查找失败时以 turn.ErrAgentUnknown 拒绝，askCause 对这个
// 哨兵的既有翻译在语义上与闸本身想表达的意思高度重合（都是"配置里没有
// 这个 agent"这个意思），候选词很容易两边都命中——对这一档断言候选词
// 测不出「闸真的挡在 Ask 前面」，因为一个没有闸的坏实现也可能打出语义
// 相近的话（只是经由 Ask 被拒绝这条路径，而不是经由闸；这一段最初是在
// 没有读过 ask.go 实际措辞的情况下写的判断，后来验证 ask.go 的三处
// mutation 时顺带读到了两边的原文才确认：闸自己的话点名了 id，askCause
// 的话没有——不完全相同，但"闸真的挡在 Ask 前面"这件事本来就不该靠猜
// 两句话像不像来证明，仍然只信下面两个结构性信号）。这一档真正有判别力
// 的信号只有两个：下面的 turn_usage 计数（闸挡住时 Ask 一次都不会被调，
// 进而不会留下任何新行；没有闸时哪怕被 ErrAgentUnknown 拒绝，也会经
// refuse -> persistTurn 留下一条新行），以及「与 C1 的答复必须不同」
// 这条结构性比较。
func TestAskUnconfiguredAgentNeverCallsAskAndDiffersFromNotJoined(t *testing.T) {
	svc, _, rn, convID := x1NewUnconfiguredFixture(t)
	before := x1TurnUsageCount(t, svc, convID, x1UnconfiguredAgentID)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+x1UnconfiguredAgentID+"\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	after := x1TurnUsageCount(t, svc, convID, x1UnconfiguredAgentID)
	if after != before {
		t.Fatalf("验收 6 的核心：配置里根本没有这个 id，Ask 一次都不该被调（哪怕 Runner 自己的"+
			"配置查找本来就会拒绝它，闸也必须在 REPL 这一层就先拦下，不能指望 Runner 兜底）。"+
			"调用前 %d 条、调用后 %d 条；完整输出:\n%s", before, after, out)
	}
	if strings.Contains(out, "正在问") {
		t.Errorf("验收 10：拒绝路径上不该出现「正在问」；完整输出:\n%s", out)
	}

	svcC1, _, rnC1, c1ConvID := x1NewConfiguredNotJoinedFixture(t)
	c1Out, err := runREPLRunner(t, svcC1, rnC1, "/use "+c1ConvID+"\n/ask "+x1MemberAgentID+"\n")
	if err != nil {
		t.Fatalf("对照组（C1）Run 失败: %v", err)
	}
	normalizedC2 := strings.ReplaceAll(out, x1UnconfiguredAgentID, "<AGENT>")
	normalizedC1 := strings.ReplaceAll(c1Out, x1MemberAgentID, "<AGENT>")
	if normalizedC2 == normalizedC1 {
		t.Errorf("验收 6 明写「与第 4 条的答复不同」：把各自的 agent id 换成同一个占位符之后，"+
			"「配置里根本没有」与「配置里有、未加入」两档答复逐字节相同，用户没法区分这两种状况；"+
			"\n配置里没有（已替换 id）:\n%s\n配置里有未加入（已替换 id）:\n%s", normalizedC2, normalizedC1)
	}
}

// ============================================================
// 验收 7：非成员带文本，正文不落库
// ============================================================

// TestAskNonMemberTextNeverPersisted 覆盖验收 7：非成员带文本，那句「你好」
// 一条都不该落库。用 C1（配置里有、未加入）代表"非成员"这个大类——契约
// 5.5 要求闸放在 postUserText 之前，这个次序对 B、C1 两档是同一条代码
// 路径，选其一验证即可，不必每一档各测一遍这同一件事。
func TestAskNonMemberTextNeverPersisted(t *testing.T) {
	svc, f, rn, convID := x1NewConfiguredNotJoinedFixture(t)
	beforeSeq, err := svc.MaxSeq(context.Background(), convID)
	if err != nil {
		t.Fatalf("读 MaxSeq 失败: %v", err)
	}
	beforeTotal := f.totalMessages()

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+x1MemberAgentID+" 你好\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	afterSeq, err := svc.MaxSeq(context.Background(), convID)
	if err != nil {
		t.Fatalf("读 MaxSeq 失败: %v", err)
	}
	if afterSeq != beforeSeq {
		t.Fatalf("验收 7 的核心：非成员带文本，那句「你好」不该落库。MaxSeq 调用前 %d、调用后 %d；"+
			"完整输出:\n%s", beforeSeq, afterSeq, out)
	}
	if got := f.totalMessages(); got != beforeTotal {
		t.Errorf("总消息数调用前 %d、调用后 %d，应当不变；完整输出:\n%s", beforeTotal, got, out)
	}
}

// ============================================================
// 验收 8：Participants 出错，fail-closed
// ============================================================

// errParticipantsStore 包一层 *fakeStore，只在 Participants 上恒定返回
// 错误，其余方法原样透传给内嵌的 fakeStore（同 round_test.go
// countingMaxSeqStore 的体例）。Service.Participants 是对 Store.Participants
// 的直通（不经 WriteTx），所以这层包装能真正拦到 Service.Participants
// 的调用。
type errParticipantsStore struct {
	*fakeStore
}

var _ conversation.Store = (*errParticipantsStore)(nil)

var errX1Participants = errors.New("x1 用例注入的列成员失败")

func (e *errParticipantsStore) Participants(ctx context.Context, convID string, includeInactive bool) ([]conversation.Participant, error) {
	return nil, errX1Participants
}

// TestAskParticipantsErrorFailsClosed 覆盖验收 8：Participants 出错时不许
// fail-open，Ask 一次都不该被调，且要打出「列出成员失败」——契约 5.2 明写
// 这句话与 cmdWho 同一句，commands.go 里 cmdWho、cmdSay 的定向发言分支、
// cmdTeamUse 都已经在用这同一句话应对 Participants 出错，这里按契约要求
// 断言它可以放心地字面比对，不是自造措辞。
func TestAskParticipantsErrorFailsClosed(t *testing.T) {
	inner := newFakeStore()
	es := &errParticipantsStore{fakeStore: inner}
	svc := conversation.NewService(es)
	rn := newTestRunner(t, svc)

	const convID = "conv_x1_participants_err000000000000000"
	es.seedConversation(convID, "验收 8：Participants 出错")

	before := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+x1MemberAgentID+"\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	after := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)
	if after != before {
		t.Fatalf("验收 8 的核心：Participants 出错时不许 fail-open，Ask 一次都不该被调。"+
			"调用前 %d 条、调用后 %d 条；完整输出:\n%s", before, after, out)
	}
	if !strings.Contains(out, "列出成员失败") {
		t.Errorf("Participants 出错时应当打印「列出成员失败」（契约 5.2 明令与 cmdWho 同一句）；"+
			"完整输出:\n%s", out)
	}
	if strings.Contains(out, "正在问") {
		t.Errorf("验收 10：这条拒绝路径上也不该出现「正在问」；完整输出:\n%s", out)
	}
}

// ============================================================
// 验收 9、10：三档互不相同 + 拒绝路径不含「正在问」（汇总版）
// ============================================================

// TestAskThreeTiersProduceMutuallyDistinctReplies 是验收 9、10 的汇总版：
// B、C1、C2 三档各自独立跑一遍，加一条「只 /use 不 /ask」的基线证明三档
// 答复确实各自产生了实质内容（不然"互不相同"可能只是因为长度巧合，
// 测不出任何东西），再把各自用到的 agent id 统一替换成同一个占位符两两
// 比较。三条具体验收（3/4、5、6）里已经各自做过一次"与 C1 不同"的比较，
// 这里补齐 B 与 C2 之间的比较，凑成完整的三档两两不同。
func TestAskThreeTiersProduceMutuallyDistinctReplies(t *testing.T) {
	svcB, _, rnB, convB := x1NewLeftMemberFixture(t)
	outB, err := runREPLRunner(t, svcB, rnB, "/use "+convB+"\n/ask "+x1MemberAgentID+"\n")
	if err != nil {
		t.Fatalf("B 档 Run 失败: %v", err)
	}

	svcC1, _, rnC1, convC1 := x1NewConfiguredNotJoinedFixture(t)
	outC1, err := runREPLRunner(t, svcC1, rnC1, "/use "+convC1+"\n/ask "+x1MemberAgentID+"\n")
	if err != nil {
		t.Fatalf("C1 档 Run 失败: %v", err)
	}

	svcC2, _, rnC2, convC2 := x1NewUnconfiguredFixture(t)
	outC2, err := runREPLRunner(t, svcC2, rnC2, "/use "+convC2+"\n/ask "+x1UnconfiguredAgentID+"\n")
	if err != nil {
		t.Fatalf("C2 档 Run 失败: %v", err)
	}

	baselineB, err := runREPLRunner(t, svcB, rnB, "/use "+convB+"\n\n")
	if err != nil {
		t.Fatalf("B 档基线 Run 失败: %v", err)
	}
	baselineC1, err := runREPLRunner(t, svcC1, rnC1, "/use "+convC1+"\n\n")
	if err != nil {
		t.Fatalf("C1 档基线 Run 失败: %v", err)
	}
	baselineC2, err := runREPLRunner(t, svcC2, rnC2, "/use "+convC2+"\n\n")
	if err != nil {
		t.Fatalf("C2 档基线 Run 失败: %v", err)
	}
	if len(outB) <= len(baselineB) {
		t.Fatalf("B 档：带 /ask 的输出（%d 字节）应当比只 /use 的基线（%d 字节）长，"+
			"否则说明这一档根本没有产生实质答复，下面「互不相同」的比较没有意义", len(outB), len(baselineB))
	}
	if len(outC1) <= len(baselineC1) {
		t.Fatalf("C1 档：带 /ask 的输出（%d 字节）应当比只 /use 的基线（%d 字节）长", len(outC1), len(baselineC1))
	}
	if len(outC2) <= len(baselineC2) {
		t.Fatalf("C2 档：带 /ask 的输出（%d 字节）应当比只 /use 的基线（%d 字节）长", len(outC2), len(baselineC2))
	}

	normB := strings.ReplaceAll(outB, x1MemberAgentID, "<AGENT>")
	normC1 := strings.ReplaceAll(outC1, x1MemberAgentID, "<AGENT>")
	normC2 := strings.ReplaceAll(outC2, x1UnconfiguredAgentID, "<AGENT>")

	pairs := []struct {
		name        string
		left, right string
	}{
		{"B vs C1", normB, normC1},
		{"C1 vs C2", normC1, normC2},
		{"B vs C2", normB, normC2},
	}
	for _, p := range pairs {
		if p.left == p.right {
			t.Errorf("验收 9：%s 两档在替换掉各自 agent id 之后逐字节相同，"+
				"用户没法区分这两种状况；文本:\n%s", p.name, p.left)
		}
	}

	for name, out := range map[string]string{"B": outB, "C1": outC1, "C2": outC2} {
		if strings.Contains(out, "正在问") {
			t.Errorf("验收 10：%s 档的拒绝路径上出现了「正在问」；完整输出:\n%s", name, out)
		}
	}
}

// ============================================================
// 自证：本文件判定「Ask 有没有被调」的观测手段本身是有效的
// ============================================================

// TestAskGateDetectionChannelSelfTest 不经过 cmdAsk，直接调 rn.Ask，证明：
// 在本文件用到的两种夹具形状下（配置里有但未加入 / 配置里根本没有），
// Ask 真的被调用一次，就会恰好留下一条新的 turn_usage 行。这是上面所有
// "turn_usage 行数不变 => Ask 没被调"断言成立的前提——如果这条前提不成立
// （比如这两种夹具形状下 Ask 被拒绝时根本不写 turn_usage），上面那些用例
// 就算通过也证明不了任何事，会变成"因为夹具本身测不出东西所以永远绿"的
// 假阳性。
func TestAskGateDetectionChannelSelfTest(t *testing.T) {
	t.Run("配置里有但未加入时直接调Ask会留下一条新turn_usage行", func(t *testing.T) {
		svc, _, rn, convID := x1NewConfiguredNotJoinedFixture(t)
		before := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)
		if _, err := rn.Ask(context.Background(), convID, x1MemberAgentID); err == nil {
			t.Fatalf("本自证用例的前提是 pv_alpha 未在 provider registry 注册、Ask 必然被拒绝；" +
				"实际 Ask 返回了 nil error，前提不成立，下面的计数没有意义")
		}
		after := x1TurnUsageCount(t, svc, convID, x1MemberAgentID)
		if after != before+1 {
			t.Fatalf("直接调 rn.Ask 应当恰好留下 1 条新 turn_usage 行，实际 before=%d after=%d", before, after)
		}
	})

	t.Run("配置里完全没有这个agent时直接调Ask同样会留下一条新turn_usage行", func(t *testing.T) {
		svc, _, rn, convID := x1NewUnconfiguredFixture(t)
		before := x1TurnUsageCount(t, svc, convID, x1UnconfiguredAgentID)
		if _, err := rn.Ask(context.Background(), convID, x1UnconfiguredAgentID); err == nil {
			t.Fatalf("本自证用例的前提是 Ask 必然被拒绝（agent 不在 cfg.Agents 里）；" +
				"实际 Ask 返回了 nil error，前提不成立")
		}
		after := x1TurnUsageCount(t, svc, convID, x1UnconfiguredAgentID)
		if after != before+1 {
			t.Fatalf("直接调 rn.Ask 应当恰好留下 1 条新 turn_usage 行，实际 before=%d after=%d", before, after)
		}
	})
}
