package turn

// 本文件覆盖任务书「RecordTurn 的调用」一组：用 recordingStore（见
// recording_store_test.go）记下的调用序列断言 Runner 到底往
// Service.RecordTurn 递了几次、每次的 msg / session 是什么形状，以及
// TurnUsage 的可空七项、DeliveredLo/Hi、以及「不写 cursors」这条硬约束。
//
// 判据的推导方式（不是猜的，是 internal/conversation/service.go 的公开
// 控制流决定的）：
//   - msg != nil 且 SenderID 不是保留 sender 时，RecordTurn 的写事务里
//     第一步就是 tx.RequireActiveParticipant；失败会让函数立刻 return，
//     tx.AppendMessage 与 tx.RecordUsage 都不会被调用。
//   - msg == nil 时完全跳过参与者校验与 AppendMessage，直接调
//     tx.RecordUsage。
//   - session != nil 时调 tx.SetProviderSession；session == nil 时两个都不调
//     ——未决 #20 之后 nil 读作「这一轮不碰 provider_sessions 这一行」，
//     不再是「作废，删行」。tx.DropProviderSession 现在只在 Service 自己的
//     DropProviderSession 方法（Runner.Rebuild 用）里被调，RecordTurn 内部
//     从未调用它。
// 于是「RecordTurn 被调用了几次、各自是什么形状」可以从
// RequireActiveParticipant / AppendMessage(Kind=Agent) / RecordUsage /
// SetProviderSession 这几个计数器结构性地反推出来。

import (
	"context"
	"errors"
	"testing"
	"time"

	"council/internal/conversation"
)

// TestRecordTurnCallShape_Success 覆盖「成功轮：一次 RecordTurn，msg 非
// nil，session 非 nil」。
func TestRecordTurnCallShape_Success(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if rec.Outcome != OutcomeSpoke {
		t.Fatalf("Outcome = %v，期望 OutcomeSpoke", rec.Outcome)
	}

	if got := rs.agentAppendCount(); got != 1 {
		t.Errorf("Kind=Agent 的 AppendMessage 调用次数 = %d，期望 1（msg 非 nil 且落库）", got)
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	if rs.recordUsageCalls[0].Seq == nil {
		t.Error("成功轮的 TurnUsage.Seq 应当非 nil：它指向这一轮产生的那条消息")
	}
	if len(rs.setProviderSessionCalls) != 1 {
		t.Errorf("SetProviderSession 调用次数 = %d，期望 1（session 非 nil）", len(rs.setProviderSessionCalls))
	}
	if len(rs.dropProviderSessionCalls) != 0 {
		t.Errorf("DropProviderSession 调用次数 = %d，期望 0", len(rs.dropProviderSessionCalls))
	}
}

// TestRecordTurnCallShape_FailureWithExistingSession 覆盖「失败轮（超时 /
// spawn 失败 / 解析失败）：一次 RecordTurn，msg 为 nil，session 为当前会话
// （会话还在）」。先用一轮成功建立会话，再用超时让第二轮失败，断言
// AppendMessage(Kind=Agent) 的计数停在 1（没有第二条 agent 发言落库），
// RecordUsage 与 SetProviderSession 各新增一次，且这一次的 TurnUsage.Seq
// 为 nil、ProviderSessionID 等于已经建立的那个会话 id——后者是 makeTurnUsage
// 从 st 回填的（rec.ProviderSession 在纯超时路径上是空的，回填逻辑见
// persistTurn 的注释），对应 09 验收表「provider_session_id 指得上」。
func TestRecordTurnCallShape_FailureWithExistingSession(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮，建立会话")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮（建立会话）失败: %v", err)
	}
	establishedSessionID := rs.setProviderSessionCalls[0].ProviderSessionID
	if establishedSessionID == "" {
		t.Fatal("前置条件不成立：第一轮应当已经建立了一个非空的 provider session id")
	}

	postUser(t, svc, convID, "第二轮，触发超时")
	fs.enqueue(spawnResult{stdout: []byte{}, killed: true, err: context.DeadlineExceeded})
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("第二轮应当报错（超时）")
	}
	if rec.Outcome != OutcomeTimeout {
		t.Fatalf("Outcome = %v，期望 OutcomeTimeout", rec.Outcome)
	}

	if got := rs.agentAppendCount(); got != 1 {
		t.Errorf("Kind=Agent 的 AppendMessage 累计调用次数 = %d，期望仍然是 1（这一轮 msg 为 nil，没有新的发言落库）", got)
	}
	if len(rs.recordUsageCalls) != 2 {
		t.Fatalf("RecordUsage 累计调用次数 = %d，期望 2", len(rs.recordUsageCalls))
	}
	second := rs.recordUsageCalls[1]
	if second.Seq != nil {
		t.Errorf("失败轮的 TurnUsage.Seq 应当为 nil，实际指向 %d", *second.Seq)
	}
	if second.ProviderSessionID != establishedSessionID {
		t.Errorf("失败轮 TurnUsage.ProviderSessionID = %q，期望等于已建立的会话 id %q（会话还在，账要指得上是哪条会话）", second.ProviderSessionID, establishedSessionID)
	}
	if len(rs.setProviderSessionCalls) != 2 {
		t.Errorf("SetProviderSession 累计调用次数 = %d，期望 2（session 为当前会话，会话还在）", len(rs.setProviderSessionCalls))
	}
	if len(rs.dropProviderSessionCalls) != 0 {
		t.Errorf("DropProviderSession 调用次数 = %d，期望 0（这一支不该作废会话）", len(rs.dropProviderSessionCalls))
	}
}

// TestRecordTurnCallShape_BootstrapFailure_SessionNeverEstablished 覆盖
// 「bootstrap 且失败：session 为 nil（会话从未建立）」。全新会话的第一轮
// 就 spawn 失败，st.providerSession 从未被赋过值，persistTurn 里
// snapshotProviderSession 因此返回 nil。
//
// 未决 #20 之后 nil 读作「这一轮不碰这一行」：SetProviderSession 与
// DropProviderSession 都不会被调用——这一行本来就不存在（真正的
// bootstrap，不是从库读回后指纹不符被作废重建），什么都不做与「删一个
// 不存在的行」结果一样，但机制已经改了，不能再断言走了 Drop 那一支。
// 本用例此前断言 dropProviderSessionCalls==1，那是旧语义（nil 即删）
// 留下的痕迹，按新语义改成两者都是 0。
func TestRecordTurnCallShape_BootstrapFailure_SessionNeverEstablished(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮就失败")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{err: errors.New("exec: \"claude-fake\": executable file not found")})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("这一轮应当报错")
	}
	if rec.Outcome != OutcomeSpawnFailed {
		t.Fatalf("Outcome = %v，期望 OutcomeSpawnFailed", rec.Outcome)
	}
	if !rec.Fresh {
		t.Fatal("前置条件不成立：全新会话的第一轮应当是 Fresh")
	}

	if got := rs.agentAppendCount(); got != 0 {
		t.Errorf("Kind=Agent 的 AppendMessage 调用次数 = %d，期望 0", got)
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]
	if u.Seq != nil {
		t.Errorf("TurnUsage.Seq 应当为 nil，实际指向 %d", *u.Seq)
	}
	if u.ProviderSessionID != "" {
		t.Errorf("TurnUsage.ProviderSessionID = %q，期望空串（会话从未建立）", u.ProviderSessionID)
	}
	if len(rs.setProviderSessionCalls) != 0 {
		t.Errorf("SetProviderSession 调用次数 = %d，期望 0", len(rs.setProviderSessionCalls))
	}
	if len(rs.dropProviderSessionCalls) != 0 {
		t.Errorf("DropProviderSession 调用次数 = %d，期望 0（未决 #20 之后 session=nil 不再触发删行，这一行本来就不存在，RecordTurn 什么都不用做）", len(rs.dropProviderSessionCalls))
	}
}

// TestRecordTurnCallShape_PostRejectedCallsRecordTurnTwice 覆盖「post_rejected
// 调两次：第一次带 msg 被拒，第二次 msg=nil 把用量写上，session 仍是当前
// 会话」。构造手法与既有 runner_outcomes_test.go 的 TestOutcomePostRejected
// 相同（起飞前把 agent /leave 掉，事务内的 RequireActiveParticipant 必然
// 拒掉第一条 Post），换成 recordingStore 是为了拿到调用次数与形状的证据，
// 那条既有测试证的是 Record 字段本身的语义，不重复。
func TestRecordTurnCallShape_PostRejectedCallsRecordTurnTwice(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "在 alice 说话之前，把她踢出这场会话")

	if err := svc.Leave(context.Background(), convID, testAgentClaude, time.Now().UTC()); err != nil {
		t.Fatalf("Leave 失败: %v", err)
	}

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当返回错误（Post 被拒）")
	}
	if rec.Outcome != OutcomePostRejected {
		t.Fatalf("Outcome = %v，期望 OutcomePostRejected", rec.Outcome)
	}

	// 第一次尝试（msg != nil）：RequireActiveParticipant 恰好被调用一次，
	// 且失败——这是 msg 非 nil 那次调用留下的唯一痕迹（AppendMessage 与
	// RecordUsage 都因为它失败而从未被调用，属于同一个事务，回滚前的语句
	// 不会有副作用）。
	if len(rs.requireActiveCalls) != 1 {
		t.Fatalf("RequireActiveParticipant 调用次数 = %d，期望 1（第一次带 msg 的尝试）", len(rs.requireActiveCalls))
	}
	if rs.requireActiveCalls[0] != (requireActiveCall{convID: convID, agentID: testAgentClaude}) {
		t.Errorf("RequireActiveParticipant 的参数 = %+v，期望 (convID=%q, agentID=%q)", rs.requireActiveCalls[0], convID, testAgentClaude)
	}
	if got := rs.agentAppendCount(); got != 0 {
		t.Errorf("Kind=Agent 的 AppendMessage 调用次数 = %d，期望 0（第一次尝试因参与者校验失败而回滚，从未到达 AppendMessage）", got)
	}

	// 第二次尝试（msg == nil）：跳过参与者校验，直接写用量与会话状态。
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1（只有第二次尝试到达这一步）", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]
	if u.Seq != nil {
		t.Errorf("TurnUsage.Seq 应当为 nil，实际指向 %d", *u.Seq)
	}
	if u.Outcome != int(OutcomePostRejected) {
		t.Errorf("TurnUsage.Outcome = %d，期望 %d（int(OutcomePostRejected)）", u.Outcome, int(OutcomePostRejected))
	}
	if len(rs.setProviderSessionCalls) != 1 {
		t.Fatalf("SetProviderSession 调用次数 = %d，期望 1（session 仍是当前会话）", len(rs.setProviderSessionCalls))
	}
	if rs.setProviderSessionCalls[0].ProviderSessionID == "" {
		t.Error("SetProviderSession 收到的会话 id 为空——provider 已经吃下这批 delta，会话应当仍然存在")
	}
	if len(rs.dropProviderSessionCalls) != 0 {
		t.Errorf("DropProviderSession 调用次数 = %d，期望 0", len(rs.dropProviderSessionCalls))
	}
}

// TestRecordTurnUsageNullableFields_GrokCacheCreateUnreported 覆盖「TurnUsage
// 的可空七项」用 grok 的分支：grokUsageToUsage 无条件把 "cache_create" 记进
// Unreported（它的 cache_creation_input_tokens 恒为 0，即使 cache_read 真的
// 从 0 涨到过 20608），makeTurnUsage 因此必须把 CacheCreate 收成 nil；其余
// 四项（InputTokens/CacheRead/OutputTokens/ThinkingTokens）grok 是如实报了的，
// 必须是指针，且值与 grokT1JSON 里的原始字节吻合
// （input=20625+cache_read=0=20625；cache_read=0；output=723；
// reasoning_tokens=722，即本包口径下的 ThinkingTokens）。
func TestRecordTurnUsageNullableFields_GrokCacheCreateUnreported(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(grokT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentGrok); err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]

	if u.CacheCreate != nil {
		t.Errorf("grok 的 CacheCreate 应当是 nil（cache_creation_input_tokens 恒为 0 这件事本身就该进 Unreported，不能让 0 冒充「没建缓存」），实际 %d", *u.CacheCreate)
	}
	mustIntPtr := func(name string, p *int, want int) {
		t.Helper()
		if p == nil {
			t.Errorf("%s 应当是非 nil 的指针（grok 如实报了这一项），实际 nil", name)
			return
		}
		if *p != want {
			t.Errorf("%s = %d，期望 %d", name, *p, want)
		}
	}
	mustIntPtr("InputTokens", u.InputTokens, 20625)
	mustIntPtr("CacheRead", u.CacheRead, 0)
	mustIntPtr("OutputTokens", u.OutputTokens, 723)
	mustIntPtr("ThinkingTokens", u.ThinkingTokens, 722)
}

// TestRecordTurnUsageNullableFields_ClaudeFailureZerosAreReal 覆盖「TurnUsage
// 的可空七项」的另一半：失败轮 claude 的五项是真实的 0，不是 NULL。
// claudeBadModelOut 的 usage 子对象是一个真实存在、字段齐全的对象（只是数值
// 都是 0），claudeUsageToUsage 在 u!=nil 的分支里不会往 Unreported 加任何
// 东西，所以这五项都必须是指向 0 的指针，而不是 nil。
func TestRecordTurnUsageNullableFields_ClaudeFailureZerosAreReal(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "用一个不存在的 model 名触发失败")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{stdout: []byte(claudeBadModelOut), exitCode: 1})
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err == nil {
		t.Fatal("这一轮应当报错")
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]

	mustZeroPtr := func(name string, p *int) {
		t.Helper()
		if p == nil {
			t.Errorf("%s 应当是非 nil 的指针（claude-badmodel.out 的 usage 对象是真实存在的，只是数值为 0），实际 nil", name)
			return
		}
		if *p != 0 {
			t.Errorf("%s = %d，期望 0", name, *p)
		}
	}
	mustZeroPtr("InputTokens", u.InputTokens)
	mustZeroPtr("CacheCreate", u.CacheCreate)
	mustZeroPtr("CacheRead", u.CacheRead)
	mustZeroPtr("OutputTokens", u.OutputTokens)
	mustZeroPtr("ThinkingTokens", u.ThinkingTokens)
}

// TestRecordTurnDeliveredLoHi 覆盖「DeliveredLo/Hi 取 Delivered 的首尾；空
// 批次两个都是 nil」。非空批次：首轮 bootstrap 一次性挑中两条用户消息，
// Lo 是较早的 seq、Hi 是较晚的 seq，且两者不同（比只测「单条消息时 Lo==Hi」
// 更能证明「取的是首尾」而不是巧合）。空批次：紧接着再问一次、期间没有新
// 内容，ErrNoDelta 那一轮的 TurnUsage 两项都必须是 nil。
func TestRecordTurnDeliveredLoHi(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	m1 := postUser(t, svc, convID, "问题一")
	m2 := postUser(t, svc, convID, "问题二")
	if m2.Seq <= m1.Seq {
		t.Fatalf("前置条件不成立：m2.Seq=%d 应当大于 m1.Seq=%d", m2.Seq, m1.Seq)
	}

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	first := rs.recordUsageCalls[0]
	if first.DeliveredLo == nil || first.DeliveredHi == nil {
		t.Fatalf("非空批次的 DeliveredLo/Hi 不该是 nil，实际 Lo=%v Hi=%v", first.DeliveredLo, first.DeliveredHi)
	}
	if *first.DeliveredLo != m1.Seq {
		t.Errorf("DeliveredLo = %d，期望等于最早那条 seq=%d", *first.DeliveredLo, m1.Seq)
	}
	if *first.DeliveredHi != m2.Seq {
		t.Errorf("DeliveredHi = %d，期望等于最晚那条 seq=%d", *first.DeliveredHi, m2.Seq)
	}

	// 第二次 Ask：没有新内容，ErrNoDelta，空批次。
	rec2, err := r.Ask(context.Background(), convID, testAgentClaude)
	if !errors.Is(err, ErrNoDelta) {
		t.Fatalf("errors.Is(err, ErrNoDelta) 应当为真，实际: %v", err)
	}
	_ = rec2
	if len(rs.recordUsageCalls) != 2 {
		t.Fatalf("RecordUsage 累计调用次数 = %d，期望 2", len(rs.recordUsageCalls))
	}
	second := rs.recordUsageCalls[1]
	if second.DeliveredLo != nil || second.DeliveredHi != nil {
		t.Errorf("空批次的 DeliveredLo/Hi 应当都是 nil，实际 Lo=%v Hi=%v", second.DeliveredLo, second.DeliveredHi)
	}
}

// TestRecordTurnNeverWritesCursorsTable 覆盖「不写 cursors：整轮之后假
// store 的 SetCursor 一次都没被调用过」。跑一个成功轮 + 一个 ErrNoDelta 轮
// + 一个失败轮，三种形状都过一遍，SetCursor 的调用计数应当自始至终停在 0——
// 游标已经搬进 provider_sessions.cursor_seq，10_STEP7_SPEC.md 第 3 节
// 明写「两处都写就有了两个事实源」。
func TestRecordTurnNeverWritesCursorsTable(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	postUser(t, svc, convID, "第一轮")
	fs.enqueueSuccess(claudeT1JSON)
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	if len(rs.setCursorCalls) != 0 {
		t.Fatalf("成功轮之后 SetCursor 调用次数 = %d，期望 0", len(rs.setCursorCalls))
	}

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); !errors.Is(err, ErrNoDelta) {
		t.Fatalf("第二轮应当是 ErrNoDelta，实际: %v", err)
	}
	if len(rs.setCursorCalls) != 0 {
		t.Fatalf("ErrNoDelta 轮之后 SetCursor 调用次数 = %d，期望 0", len(rs.setCursorCalls))
	}

	postUser(t, svc, convID, "第三轮，触发解析失败")
	fs.enqueue(spawnResult{stdout: []byte{}, exitCode: 1})
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err == nil {
		t.Fatal("第三轮应当报错（Unparsable）")
	}
	if len(rs.setCursorCalls) != 0 {
		t.Fatalf("失败轮之后 SetCursor 调用次数 = %d，期望 0", len(rs.setCursorCalls))
	}
}
