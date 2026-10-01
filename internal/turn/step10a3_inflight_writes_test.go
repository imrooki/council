package turn

// 本文件覆盖 Step 10 A 片之三冻结契约（E:\multi_model_cowork\step10a3_contract.md）
// 第 6 节验收清单的第 1-6 条：askUpToWith 是否真的按顺序写了 begun/spawned/returned
// 三条 phase、BeginTurn 与 phase=spawned 是否严格早于 spawn、两个失败注入点是否
// "起进程之前拒绝、不起进程"、起进程之前的拒绝是否仍然记账、以及成功路径上
// expectInflight 是否真的传了 true（不是照抄旧代码的字面量 false）。
//
// 第 6 条第 6、7 两类已经被 internal/store/inflight_recordturn_test.go 与
// internal/store/inflight_test.go 盖住（ErrInflightMissing 的整轮回滚语义、
// baseline_suspect 列本身的往返），契约与任务书都明说不要重写；本文件测的是
// "上层"：askUpToWith 有没有在正确的地方调用、传对参数。
//
// 背对背纪律：本文件写作期间没有读取、也没有依赖 internal/turn/runner.go 或
// internal/conversation/service.go 在本刀改动之后的任何新增代码——它们此刻
// （契约冻结时）还是改动之前的状态，本文件只按 E 盘那份冻结契约的文字写。

import (
	"context"
	"errors"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
)

// TestAskWritesBeginTurnAndSpawnedPhaseStrictlyBeforeSpawn 覆盖验收第 1、2 条：
// 一轮成功时 BeginTurn 与 phase=spawned 都必须严格早于 spawn 被调用，
// phase=returned 必须晚于 spawn，且全轮结束时 ClearInflight 恰好被调用一次。
//
// 判据落在可观测的序列上，不是猜时序：spawn 本身换成一个会在"被调用的那一刻"
// 拍下 recordingStore 两份调用记账当前长度的闭包（不用 fakeSpawner——它只管
// 回放结果，不管时序）。若 BeginTurn 或 phase=spawned 的写入被实现挪到了
// spawn 之后，这里拍到的快照长度会是 0，断言就会失败；写在 spawn 之前，
// 快照长度必然 >= 1。
func TestAskWritesBeginTurnAndSpawnedPhaseStrictlyBeforeSpawn(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)

	var (
		spawnCalled              bool
		beginTurnCountAtSpawn    int
		spawnedPhaseCountAtSpawn int
	)
	r.spawn = func(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
		spawnCalled = true
		beginTurnCountAtSpawn = len(rs.beginTurnCalls)
		spawnedPhaseCountAtSpawn = len(rs.setInflightPhaseCalls)
		return []byte(claudeT1JSON), nil, 0, false, nil
	}

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if rec.Outcome != OutcomeSpoke {
		t.Fatalf("Outcome = %v，期望 OutcomeSpoke", rec.Outcome)
	}
	if !spawnCalled {
		t.Fatal("spawn 从未被调用，前置条件不成立")
	}

	if beginTurnCountAtSpawn != 1 {
		t.Errorf("spawn 被调用的那一刻，BeginTurn 已经发生的次数 = %d，期望 1（BeginTurn 必须严格早于 spawn）", beginTurnCountAtSpawn)
	}
	if spawnedPhaseCountAtSpawn != 1 {
		t.Errorf("spawn 被调用的那一刻，SetInflightPhase 已经发生的次数 = %d，期望 1（phase=spawned 必须严格早于 spawn）", spawnedPhaseCountAtSpawn)
	}
	if len(rs.setInflightPhaseCalls) > 0 && rs.setInflightPhaseCalls[0].phase != conversation.PhaseSpawned {
		t.Errorf("spawn 之前写的那一条 phase = %q，期望 %q", rs.setInflightPhaseCalls[0].phase, conversation.PhaseSpawned)
	}

	if len(rs.beginTurnCalls) != 1 {
		t.Fatalf("BeginTurn 总调用次数 = %d，期望 1", len(rs.beginTurnCalls))
	}
	if rs.beginTurnCalls[0].Phase != conversation.PhaseBegun {
		t.Errorf("BeginTurn 写入的 Phase = %q，期望 %q", rs.beginTurnCalls[0].Phase, conversation.PhaseBegun)
	}
	if rs.beginTurnCalls[0].TurnID != rec.TurnID {
		t.Errorf("BeginTurn 写入的 TurnID = %q，期望等于这一轮的 TurnID %q", rs.beginTurnCalls[0].TurnID, rec.TurnID)
	}

	if len(rs.setInflightPhaseCalls) != 2 {
		t.Fatalf("SetInflightPhase 总调用次数 = %d，期望 2（spawned 与 returned 各一次）", len(rs.setInflightPhaseCalls))
	}
	if rs.setInflightPhaseCalls[1].phase != conversation.PhaseReturned {
		t.Errorf("第二次 SetInflightPhase 写入的 phase = %q，期望 %q（它是本轮唯一的第二次调用，必然晚于 spawn）",
			rs.setInflightPhaseCalls[1].phase, conversation.PhaseReturned)
	}
	for i, c := range rs.setInflightPhaseCalls {
		if c.turnID != rec.TurnID {
			t.Errorf("第 %d 次 SetInflightPhase 的 turnID = %q，期望 %q", i, c.turnID, rec.TurnID)
		}
	}

	if len(rs.clearInflightCalls) != 1 || rs.clearInflightCalls[0] != rec.TurnID {
		t.Errorf("ClearInflight 调用记账 = %v，期望恰好一次、turnID = %q", rs.clearInflightCalls, rec.TurnID)
	}
}

// TestBeginTurnFieldsAcrossTwoTurns 覆盖 BeginTurn 收到的 InflightTurn 12
// 个字段里最容易被写串的两对（契约 §2.1 tstBaseInflight 注释点名的同一类坑）：
// PlannedSessionID/ResumeSessionID、CursorSeq/ScannedTo。
//
// 用 grok 而不是 claude：grok 是 SessionIDPreassignable，首轮会现铸一个
// PlannedSessionID 写进命令行；claude 恰恰相反，首轮什么都不预铸，用 claude
// 测这个字段只会让它永远是空串，测不出"写串了"这种坏实现。两轮应当是这一对
// 字段的镜像：首轮 Planned 非空、Resume 为空（还没有会话可续）；第二轮反过来
// （Resume 非空、Planned 为空，grok 续会话时不会再预铸）。
func TestBeginTurnFieldsAcrossTwoTurns(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)
	m1 := postUser(t, svc, convID, "第一轮")

	clock := newFakeClock(time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC))
	r := newTestRunner(t, cfg, svc)
	r.now = clock.now
	fs := &fakeSpawner{}
	fs.enqueueSuccess(grokT1JSON)
	r.spawn = fs.spawn

	rec1, err := r.Ask(context.Background(), convID, testAgentGrok)
	if err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	if rec1.Outcome != OutcomeSpoke {
		t.Fatalf("第一轮 Outcome = %v，期望 OutcomeSpoke", rec1.Outcome)
	}
	if len(rs.beginTurnCalls) != 1 || len(fs.calls) != 1 {
		t.Fatalf("前置条件不成立：BeginTurn 调用 %d 次，spawn 调用 %d 次，都期望 1", len(rs.beginTurnCalls), len(fs.calls))
	}
	first := rs.beginTurnCalls[0]
	firstCmd := fs.calls[0].cmd

	wantTier, tErr := TranslatePermission(cfg.Agents[testAgentGrok].Permissions)
	if tErr != nil {
		t.Fatalf("TranslatePermission 失败: %v", tErr)
	}

	if first.TurnID != rec1.TurnID {
		t.Errorf("第一轮 TurnID = %q，期望 %q", first.TurnID, rec1.TurnID)
	}
	if first.ConvID != convID {
		t.Errorf("第一轮 ConvID = %q，期望 %q", first.ConvID, convID)
	}
	if first.AgentID != testAgentGrok {
		t.Errorf("第一轮 AgentID = %q，期望 %q", first.AgentID, testAgentGrok)
	}
	if first.Provider != "grok" {
		t.Errorf("第一轮 Provider = %q，期望 %q", first.Provider, "grok")
	}
	if first.PlannedSessionID == "" {
		t.Error("第一轮 PlannedSessionID 为空，期望非空（grok 首轮会现铸一个）")
	}
	if first.PlannedSessionID != firstCmd.PlannedSessionID {
		t.Errorf("第一轮 PlannedSessionID = %q，期望与真正发给 spawn 的命令行一致 %q", first.PlannedSessionID, firstCmd.PlannedSessionID)
	}
	if first.ResumeSessionID != "" {
		t.Errorf("第一轮 ResumeSessionID = %q，期望空串（首轮没有会话可续，若这里非空说明与 PlannedSessionID 写串了）", first.ResumeSessionID)
	}
	if first.CursorSeq != 0 {
		t.Errorf("第一轮 CursorSeq = %d，期望 0（st.cursor 的初始值）", first.CursorSeq)
	}
	if first.ScannedTo != rec1.ScannedTo {
		t.Errorf("第一轮 ScannedTo = %d，期望等于这一轮的 rec.ScannedTo %d", first.ScannedTo, rec1.ScannedTo)
	}
	if first.ScannedTo != m1.Seq {
		t.Errorf("第一轮 ScannedTo = %d，期望等于唯一那条用户消息的 seq %d", first.ScannedTo, m1.Seq)
	}
	if !first.StartedAt.Equal(clock.now()) {
		t.Errorf("第一轮 StartedAt = %v，期望等于 r.now() 当时的值 %v", first.StartedAt, clock.now())
	}
	if first.Phase != conversation.PhaseBegun {
		t.Errorf("第一轮 Phase = %q，期望 %q", first.Phase, conversation.PhaseBegun)
	}
	if first.Tier != string(wantTier) {
		t.Errorf("第一轮 Tier = %q，期望 %q", first.Tier, string(wantTier))
	}
	if first.WorkDir == "" || first.WorkDir != firstCmd.Dir {
		t.Errorf("第一轮 WorkDir = %q，期望与真正发给 spawn 的 cmd.Dir 一致 %q（且非空）", first.WorkDir, firstCmd.Dir)
	}

	clock.advance(2 * time.Hour)
	m2 := postUser(t, svc, convID, "第二轮")
	fs.enqueueSuccess(grokT2JSON)
	rec2, err := r.Ask(context.Background(), convID, testAgentGrok)
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}
	if len(rs.beginTurnCalls) != 2 {
		t.Fatalf("第二轮之后 BeginTurn 累计调用次数 = %d，期望 2", len(rs.beginTurnCalls))
	}
	second := rs.beginTurnCalls[1]
	secondCmd := fs.calls[1].cmd

	if second.TurnID != rec2.TurnID {
		t.Errorf("第二轮 TurnID = %q，期望 %q", second.TurnID, rec2.TurnID)
	}
	if secondCmd.PlannedSessionID != "" {
		t.Fatalf("前置条件不成立：第二轮（续会话）真正发给 spawn 的 cmd.PlannedSessionID 应当为空，实际 %q", secondCmd.PlannedSessionID)
	}
	if second.PlannedSessionID != "" {
		t.Errorf("第二轮 PlannedSessionID = %q，期望空串（grok 续会话不会再预铸，若非空说明与 ResumeSessionID 写串了）", second.PlannedSessionID)
	}
	if second.ResumeSessionID == "" {
		t.Error("第二轮 ResumeSessionID 为空，期望非空（应当是第一轮建立的那个会话 id）")
	}
	if second.ResumeSessionID != rec1.ProviderSession {
		t.Errorf("第二轮 ResumeSessionID = %q，期望等于第一轮建立的会话 id %q", second.ResumeSessionID, rec1.ProviderSession)
	}
	if second.CursorSeq != first.ScannedTo {
		t.Errorf("第二轮 CursorSeq = %d，期望等于第一轮结束时的游标（第一轮 ScannedTo）%d（若与 ScannedTo 写串会在这里露出来）",
			second.CursorSeq, first.ScannedTo)
	}
	if second.ScannedTo != m2.Seq {
		t.Errorf("第二轮 ScannedTo = %d，期望等于第二条用户消息的 seq %d", second.ScannedTo, m2.Seq)
	}
	if second.CursorSeq == second.ScannedTo {
		t.Fatalf("前置条件不成立：本用例要求 CursorSeq(%d) 与 ScannedTo(%d) 在第二轮取值不同，否则测不出两者被写串",
			second.CursorSeq, second.ScannedTo)
	}
	if !second.StartedAt.Equal(clock.now()) {
		t.Errorf("第二轮 StartedAt = %v，期望等于 r.now() 当时的值 %v", second.StartedAt, clock.now())
	}
	if second.WorkDir != secondCmd.Dir {
		t.Errorf("第二轮 WorkDir = %q，期望与 cmd.Dir 一致 %q", second.WorkDir, secondCmd.Dir)
	}
}

// TestBeginTurnFailure_RefusesBeforeSpawningAndKeepsUsageAccounting 覆盖
// 验收第 3 条：BeginTurn 报错时，Outcome 必须是 OutcomeRefused，且假
// Spawner 一次都没被调用过（判据是"进程没起"，不是错误文本）。
func TestBeginTurnFailure_RefusesBeforeSpawningAndKeepsUsageAccounting(t *testing.T) {
	rs := newRecordingStore()
	injectedErr := errors.New("模拟 BeginTurn 写库失败")
	injectingStore := &beginTurnFailingStore{recordingStore: rs, err: injectedErr}
	svc := conversation.NewService(injectingStore)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON) // 若坏实现真起了进程，这一条会被吃掉，len(fs.calls) 就会变成 1
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当报错")
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("errors.Is(err, injectedErr) 应当为真，实际: %v", err)
	}
	if len(fs.calls) != 0 {
		t.Fatalf("fakeSpawner 被调用了 %d 次，期望 0 次（BeginTurn 失败绝不能继续往下走去起进程）", len(fs.calls))
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Errorf("RecordUsage 调用次数 = %d，期望 1（起进程之前的拒绝仍要记账）", len(rs.recordUsageCalls))
	}
}

// TestSetInflightPhaseSpawnedFailure_RefusesBeforeSpawningAndClearsBegunRow
// 覆盖验收第 4 条：SetInflightPhase(spawned) 报错时，Outcome 必须是
// OutcomeRefused，且假 Spawner 一次都没被调用过；顺带验证 BeginTurn 已经
// 写下的那一行没有变成孤儿——此刻它是 phase=begun，随后 refuse 记账时
// began 已经是 true，ClearInflight 应当把这一行删掉（不需要它真的"飞"过，
// begun 也是一种需要清理的状态）。
func TestSetInflightPhaseSpawnedFailure_RefusesBeforeSpawningAndClearsBegunRow(t *testing.T) {
	rs := newRecordingStore()
	injectedErr := errors.New("模拟 SetInflightPhase(spawned) 写库失败")
	injectingStore := &setInflightPhaseFailingStore{recordingStore: rs, err: injectedErr}
	svc := conversation.NewService(injectingStore)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当报错")
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("errors.Is(err, injectedErr) 应当为真，实际: %v", err)
	}
	if len(fs.calls) != 0 {
		t.Fatalf("fakeSpawner 被调用了 %d 次，期望 0 次", len(fs.calls))
	}

	if len(rs.beginTurnCalls) != 1 {
		t.Fatalf("前置条件不成立：BeginTurn 应当已经成功写过一次，实际 %d 次", len(rs.beginTurnCalls))
	}
	if len(rs.clearInflightCalls) != 1 || rs.clearInflightCalls[0] != rec.TurnID {
		t.Errorf("ClearInflight 调用记账 = %v，期望恰好一次、turnID = %q"+
			"（began 此刻已经是 true，refuse 记账时应当把这一行清掉，不留孤儿）", rs.clearInflightCalls, rec.TurnID)
	}
}

// TestAskEarlyRefusal_TurnUsageStillCommittedDespiteMissingInflightRow 覆盖
// 验收第 5 条：起进程之前的拒绝（这里用"配置里没有这个 agent"）之后，
// expectInflight 必须是 false，turn_usage 照常记上一行——不能因为这一轮
// 从未 BeginTurn、没有在飞记录可清，就让 RecordTurn 因 ErrInflightMissing
// 整笔回滚。
//
// 必须用真库：recordingStore.WriteTx 不做真实回滚（fn 已经直接改了内存
// 状态，返回错误也不撤销），expectInflight 传错时唯一能观察到的差异就是
// "事务真的回滚了没有"，这件事只有真实 *store.Store 才做得出来。
//
// 说明：今天（本刀落地之前）persistTurn 本来就把 expectInflight 硬编码成
// 字面量 false，这条用例此刻已经是绿的，不是"红-绿"对；它守的是契约明写
// 的那个回归方向——"该传 false 却传了 true，会让一轮被拒绝的、本该记账的
// 轮整笔回滚"——一旦实现把 began 接错（比如初值给成 true），这里就会变红。
func TestAskEarlyRefusal_TurnUsageStillCommittedDespiteMissingInflightRow(t *testing.T) {
	rts := newRealTurnStore(t)
	cfg := newTestConfig()
	convID := newConv(t, rts.svc)
	// 不 joinAgent：agentID 干脆不在配置里，最简单的"起进程之前拒绝"。

	r := newTestRunner(t, cfg, rts.svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	const unknownAgent = "no-such-agent"
	rec, err := r.Ask(context.Background(), convID, unknownAgent)
	if err == nil {
		t.Fatal("Ask 应当报错")
	}
	if !errors.Is(err, ErrAgentUnknown) {
		t.Fatalf("errors.Is(err, ErrAgentUnknown) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if len(fs.calls) != 0 {
		t.Fatalf("fakeSpawner 被调用了 %d 次，期望 0 次", len(fs.calls))
	}
	if rec.TurnID == "" {
		t.Fatal("前置条件不成立：TurnID 应当已经铸好了（persistTurn 靠它判断要不要落库）")
	}

	if n := rts.turnUsageRowCount(t, rec.TurnID); n != 1 {
		t.Errorf("turn_usage 里 turn_id=%q 有 %d 行，期望 1 行——若 expectInflight 被错误地传成 true，"+
			"ErrInflightMissing 会让这一笔整轮回滚，这里就会读到 0 行", rec.TurnID, n)
	}
}

// TestAskSuccessPath_ExpectInflightTrueSurfacesMissingInflightRow 覆盖验收
// 第 6 条在"上层"这一侧的核心：askUpToWith 主路径（成功解析之后那一次
// RecordTurn）真的把 expectInflight 传成了 true，不是照抄旧代码的字面量
// false。ErrInflightMissing 本身的回滚语义已经被
// internal/store/inflight_recordturn_test.go 测过（契约第 6 节第 6 条明说
// 不要重写），这里只验证"传对了没有"。
//
// 手法：把 ClearInflight 换成恒定报告 existed=false 的版本（不管这一行
// 实际在不在），模拟"RecordTurn 调用 ClearInflight 的那一刻这一行已经不在
// 了"这种极端时序。若 expectInflight 正确地传了 began（此刻是 true），
// Service.RecordTurn 会返回 ErrInflightMissing，askUpToWith 的主路径会把
// 这当成"post 失败"处理，Outcome 变成 OutcomePostRejected；若坏实现仍然
// 硬编码 false，这一步会被无声放过，Outcome 停在 OutcomeSpoke——那正是
// 这条断言要拆穿的坏实现。
func TestAskSuccessPath_ExpectInflightTrueSurfacesMissingInflightRow(t *testing.T) {
	rs := newRecordingStore()
	injectingStore := &clearInflightNeverExistsStore{recordingStore: rs}
	svc := conversation.NewService(injectingStore)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当报错：ClearInflight 恒报 existed=false，expectInflight 若正确传了 true 就该在这里报 ErrInflightMissing")
	}
	if !errors.Is(err, conversation.ErrInflightMissing) {
		t.Errorf("errors.Is(err, ErrInflightMissing) 应当为真，实际: %v"+
			"（若这里是其它错误，说明 askUpToWith 成功路径上的 expectInflight 没有正确传 true，很可能被硬编码成了 false）", err)
	}
	if rec.Outcome != OutcomePostRejected {
		t.Errorf("Outcome = %v，期望 OutcomePostRejected（provider 已经吃下这一轮，只是本地记不上）", rec.Outcome)
	}
	if len(rs.clearInflightCalls) == 0 {
		t.Error("ClearInflight 从未被调用，前置条件不成立")
	}
}

// TestAskFailClosedPath_ClearsInflightRowAfterBeginTurnSucceeded 覆盖失败
// 路径（BeginTurn 已经成功、随后 spawn 失败）不会留下孤儿在飞记录：
// persistTurn 那一次 RecordTurn 调用仍然要走到 ClearInflight，把
// BeginTurn/SetInflightPhase(spawned) 写下的那一行清掉。
//
// 这条不是"expectInflight 真的等于 began"的判别性证据——existed 为 true
// 时 expectInflight 传 true 或 false 结果都一样（都不报错、都会清行），
// rec.Err 在 failClosed 里又已经被原始失败原因占了（`rec.Err = err` 发生
// 在 persistTurn 之前），RecordTurn 内部若真的因为 expectInflight 传错而
// 报错，也会被 persistTurn 里"rec.Err == nil 才覆盖"那条判断吞掉，从外部
// 观察不出来。这一条局限已经写进交付报告。这里测的是更朴素、但同样真实的
// 一件事：即使失败，也不能把 BeginTurn 写下的行永远留在库里。
func TestAskFailClosedPath_ClearsInflightRowAfterBeginTurnSucceeded(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{err: errors.New("exec: \"claude-fake\": executable file not found")})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("这一轮应当报错（spawn 失败）")
	}
	if rec.Outcome != OutcomeSpawnFailed {
		t.Fatalf("Outcome = %v，期望 OutcomeSpawnFailed", rec.Outcome)
	}

	if len(rs.beginTurnCalls) != 1 {
		t.Fatalf("前置条件不成立：BeginTurn 应当已经成功写过一次，实际 %d 次", len(rs.beginTurnCalls))
	}
	if len(rs.setInflightPhaseCalls) != 1 || rs.setInflightPhaseCalls[0].phase != conversation.PhaseSpawned {
		t.Fatalf("前置条件不成立：应当恰好写过一次 phase=spawned，实际 %+v", rs.setInflightPhaseCalls)
	}
	if len(rs.clearInflightCalls) != 1 || rs.clearInflightCalls[0] != rec.TurnID {
		t.Errorf("ClearInflight 调用记账 = %v，期望恰好一次、turnID = %q（spawn 失败也不该留下孤儿在飞记录）",
			rs.clearInflightCalls, rec.TurnID)
	}
}
