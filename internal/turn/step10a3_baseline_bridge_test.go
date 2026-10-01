package turn

// 本文件覆盖 Step 10 A 片之三冻结契约第 6 节验收清单的第 7、8 条：
// baselineSuspect 过桥。
//
// 库层的往返（SetProviderSession/ProviderSession 逐字节写读这一列）已经被
// internal/store/inflight_test.go 的 TestSetProviderSessionRoundTripsBaselineSuspect
// 盖住了（契约第 6 节第 7 条明说不要重写）。本文件测的是两个桥接点本身：
// snapshotProviderSession 把 st.baselineSuspect 抄进
// conversation.ProviderSession.BaselineSuspect（写方向），sessionStateFrom
// 把 conversation.ProviderSession.BaselineSuspect 抄回 st.baselineSuspect
// （读方向）——这两个函数都是包内私有的纯函数，直接调用是最短、
// 最不会被别的分支掩盖的判据。

import (
	"context"
	"testing"

	"council/internal/conversation"
)

// TestSnapshotProviderSessionCopiesBaselineSuspect 覆盖写方向：
// st.baselineSuspect == true 时，snapshotProviderSession 交出的
// *conversation.ProviderSession.BaselineSuspect 也必须是 true。
// 同时用 false 做对照，防止一个恒定返回 true 的坏实现蒙混过关。
func TestSnapshotProviderSessionCopiesBaselineSuspect(t *testing.T) {
	suspect := &sessionState{
		providerSession: "sess-1",
		cursor:          3,
		baselineSuspect: true,
	}
	got := snapshotProviderSession("conv-1", "agent-1", suspect)
	if got == nil {
		t.Fatal("snapshotProviderSession 返回 nil，前置条件不成立（providerSession 非空时不该返回 nil）")
	}
	if !got.BaselineSuspect {
		t.Error("st.baselineSuspect == true 时，snapshotProviderSession 交出的 BaselineSuspect 应当是 true，实际 false" +
			"（这是本刀两个桥接点之一：不补上这一行，一次超时之后的重启会让「基线可疑」这件事凭空消失）")
	}

	clean := &sessionState{
		providerSession: "sess-1",
		cursor:          3,
		baselineSuspect: false,
	}
	got2 := snapshotProviderSession("conv-1", "agent-1", clean)
	if got2 == nil {
		t.Fatal("snapshotProviderSession 返回 nil，前置条件不成立")
	}
	if got2.BaselineSuspect {
		t.Error("st.baselineSuspect == false 时，snapshotProviderSession 交出的 BaselineSuspect 应当是 false，实际 true" +
			"（对照组：挡的是「恒定写 true」这种坏实现）")
	}
}

// TestSessionStateFromCopiesBaselineSuspect 覆盖读方向：
// conversation.ProviderSession.BaselineSuspect == true 时，
// sessionStateFrom 交出的 *sessionState.baselineSuspect 也必须是 true。
// 同样用 false 做对照。
func TestSessionStateFromCopiesBaselineSuspect(t *testing.T) {
	ps := conversation.ProviderSession{
		ConvID:            "conv-1",
		AgentID:           "agent-1",
		ProviderSessionID: "sess-1",
		BaselineSuspect:   true,
	}
	got := sessionStateFrom(ps)
	if got == nil {
		t.Fatal("sessionStateFrom 返回 nil")
	}
	if !got.baselineSuspect {
		t.Error("ps.BaselineSuspect == true 时，sessionStateFrom 交出的 baselineSuspect 应当是 true，实际 false" +
			"（本刀两个桥接点的另一半：不补上这一行，重启读回来的会话永远显示成「基线正常」）")
	}

	ps2 := ps
	ps2.BaselineSuspect = false
	got2 := sessionStateFrom(ps2)
	if got2 == nil {
		t.Fatal("sessionStateFrom 返回 nil")
	}
	if got2.baselineSuspect {
		t.Error("ps.BaselineSuspect == false 时，sessionStateFrom 交出的 baselineSuspect 应当是 false，实际 true")
	}
}

// TestBaselineSuspectSurvivesRestart_RequiresBothBridgePoints 覆盖验收第 8
// 条，本刀唯一"不补桥接点就必红"的用例：一轮失败（非 fresh）之后，库里
// 那一行的 baseline_suspect 必须读出 1；下一轮用新构造的 Runner（模拟重启，
// 内存快照是空的，但共用同一个真实 store）从库读回，rec.BaselineSuspect
// == true 且 rec.CostUSDDelta == nil，Meta 里的 cost_baseline_suspect 也
// 必须是 true。
//
// 少了写方向的桥接点：persistTurn 传给 RecordTurn 的 session 里
// BaselineSuspect 恒为 false，库里那一列永远是 0，第三轮读回 st 时
// baselineSuspect 自然是 false。少了读方向的桥接点：库里那一列即使是 1，
// sessionStateFrom 也读不出来，同样落空。两处都补，这条用例才会绿。
func TestBaselineSuspectSurvivesRestart_RequiresBothBridgePoints(t *testing.T) {
	rts := newRealTurnStore(t)
	cfg := newTestConfig()
	convID := newConv(t, rts.svc)
	joinAgent(t, rts.svc, convID, testAgentClaude)
	postUser(t, rts.svc, convID, "第一轮，建立会话")

	r1 := newTestRunner(t, cfg, rts.svc)
	fs1 := &fakeSpawner{}
	fs1.enqueueSuccess(claudeT1JSON)
	r1.spawn = fs1.spawn
	if _, err := r1.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮（建立会话）失败: %v", err)
	}

	postUser(t, rts.svc, convID, "第二轮，触发超时")
	fs1.enqueue(spawnResult{stdout: []byte{}, killed: true, err: context.DeadlineExceeded})
	rec2, err := r1.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("第二轮应当报错（超时）")
	}
	if rec2.Outcome != OutcomeTimeout {
		t.Fatalf("第二轮 Outcome = %v，期望 OutcomeTimeout", rec2.Outcome)
	}
	// 不在这里断言 rec2.BaselineSuspect：那个字段问的是"这一轮开始时，上一轮
	// 留下的状态"（record.go 字段注释原话），赋值发生在 askUpToWith 早期
	// （bootstrap/ColdExpected 那一批旁边），比 failClosed 把
	// st.baselineSuspect 改成 true 要早——第二轮是刚失败的那一轮自己，它
	// 开始时上一轮（第一轮）是成功的，所以 rec2.BaselineSuspect 正确的值
	// 是 false，不是本用例最初误写的 true（这条订正已经写进契约的订正
	// 记录，供同类误读参考）。真正要测的两个判据在下面：库里这一行的
	// baseline_suspect 读出 1（写方向的桥），以及第三轮"重启"之后读回
	// rec3.BaselineSuspect（读方向的桥）。

	raw := rts.rawDB(t)
	var baselineSuspectCol int
	if err := raw.QueryRow(
		`SELECT baseline_suspect FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`,
		convID, testAgentClaude,
	).Scan(&baselineSuspectCol); err != nil {
		t.Fatalf("直接查库失败: %v", err)
	}
	if baselineSuspectCol != 1 {
		t.Fatalf("库里 provider_sessions.baseline_suspect = %d，期望 1"+
			"（写方向的桥接点没补：persistTurn 传给 RecordTurn 的 session.BaselineSuspect 恒为 false）", baselineSuspectCol)
	}

	// 模拟重启：新 Runner，同一个 workDir，同一个真实 store，内存快照清零。
	r2 := newTestRunnerSameWorkDir(r1, cfg, rts.svc)
	postUser(t, rts.svc, convID, "第三轮，重启之后")
	fs2 := &fakeSpawner{}
	fs2.enqueueSuccess(claudeT2JSON)
	r2.spawn = fs2.spawn

	rec3, err := r2.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第三轮应当成功，实际: %v", err)
	}
	if !rec3.BaselineSuspect {
		t.Error("rec3.BaselineSuspect 应当是 true：库里第二轮留下的可疑标记，读方向的桥接点没补就会在这里丢掉")
	}
	if rec3.CostUSDDelta != nil {
		t.Errorf("rec3.CostUSDDelta 应当是 nil（基线可疑时不猜差值），实际 %v", *rec3.CostUSDDelta)
	}
	if rec3.Message == nil {
		t.Fatal("第三轮成功却没有落库的消息，前置条件不成立")
	}
	got, ok := rec3.Message.Meta[MetaCostBaselineSuspect]
	if !ok {
		t.Fatal("第三轮消息的 Meta 里没有 cost_baseline_suspect 这个键")
	}
	if got != true {
		t.Errorf("Meta[cost_baseline_suspect] = %v，期望 true", got)
	}
}
