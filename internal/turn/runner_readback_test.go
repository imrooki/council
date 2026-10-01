package turn

// 本文件覆盖任务书「Runner 读回」与「指纹的两个分支」两组。用的是真库
// （newTestService，fixtures_test.go 里已经在用真正的 store.Open），不是
// recordingStore：这部分行为本来就经 svc.ProviderSession 与 Record 的可
// 观测字段暴露了（游标是否生效体现在 Delivered 里、cost 基线是否生效体现
// 在 CostUSDDelta 里、指纹分支体现在 Outcome/Fresh/Command.Args 里），不需要
// 拦截调用序列，用真库更贴近生产路径，也不必再自己维护一份参与者/消息的
// 内存实现。
//
// 「模拟重启」用两个独立的 *Runner 实例、共享同一个 *conversation.Service
// （从而共享同一个底层 SQLite 文件）：第二个 Runner 的 r.gates 是全新的空
// map，这与真实重启后的状态完全一致，比手工构造 sessionState 更贴近「读回」
// 这件事本身要验证的东西。只有需要精确控制 cursor_seq / cost_total_usd /
// identity_fp 单个字段的场景（游标生效、TTL、指纹不符）才改用直接 seed：
// seedProviderSession 调用 svc.RecordTurn(ctx, nil, u, &session)——10a §2.4
// 明写这是「本步唯一允许写 turn_usage 与 provider_sessions 的入口」，seed
// 没有绕开它，只是不经过一次完整的 Ask。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// claudeFixtureSessionID 是 claudeT1JSON / claudeT2JSON / claudeT3JSON 共享
// 的 session_id（同一场会话的三轮），fixtures_test.go 的注释里已经点过。
// 这里单独命名，是因为「读回」这组测试要精确控制 Drifted 是否触发——
// seed 一条与真实回传值相同的 id，才能让 Drifted=false，不干扰本组要测的
// 游标/基线/指纹这几件事。
const claudeFixtureSessionID = "2a90d449-a348-497a-8812-e73e74d6898e"

// seedProviderSession 直接调 svc.RecordTurn(ctx, nil, u, &session) 把一条
// ProviderSession 写进库，不经过完整的 Ask 流程。ps.ConvID / ps.AgentID 由
// 本函数补上，调用方不必重复填。TurnID 用 (convID, agentID) 拼一个固定值：
// 同一个 (convID, agentID) 在单个测试里只 seed 一次，不会与 Ask 真正产生的
// 随机 UUID 冲突。
func seedProviderSession(t *testing.T, svc *conversation.Service, convID, agentID string, ps conversation.ProviderSession) {
	t.Helper()
	ps.ConvID = convID
	ps.AgentID = agentID
	u := conversation.TurnUsage{
		TurnID:  "seed-" + convID + "-" + agentID,
		ConvID:  convID,
		AgentID: agentID,
		Outcome: int(OutcomeRefused), // 占位：这不是一次真实的轮次
	}
	if _, err := svc.RecordTurn(context.Background(), nil, u, &ps, false); err != nil {
		t.Fatalf("seedProviderSession 失败: %v", err)
	}
}

// TestAskReadsBackProviderSessionAcrossRestart 覆盖「gate.st 为空、库里有一条
// 会话时：Ask 读回它，不是 Fresh，Record.Fresh 为假，不发 session_opening
// 面包屑，且 BuildTurn 收到的 ProviderSession 等于库里那个 id」。
func TestAskReadsBackProviderSessionAcrossRestart(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮：建立会话")

	rA := newTestRunner(t, cfg, svc)
	fsA := &fakeSpawner{}
	fsA.enqueueSuccess(claudeT1JSON)
	rA.spawn = fsA.spawn
	recA, err := rA.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第一个 Runner 建立会话失败: %v", err)
	}
	if recA.ProviderSession == "" {
		t.Fatal("前置条件不成立：第一个 Runner 应当已经建立了一个非空的 provider session id")
	}

	// 模拟重启：全新的 Runner，gates 是空 map，但指向同一个 svc（同一个库）、
	// 同一条 workDir——真实重启读的是同一份配置，data_dir 不会变，
	// newTestRunnerSameWorkDir 复用 rA.workDir 而不是各自现铸一个 t.TempDir()，
	// 否则指纹第七段会把「同一进程重启」误判成「配置真的变了」。
	postUser(t, svc, convID, "第二轮：模拟重启之后的下一句")
	rB := newTestRunnerSameWorkDir(rA, cfg, svc)
	// 坑：r.gate(key) 是 get-or-create，调用它本身就会往 map 里塞一条，所以
	// 这条前置检查必须直接读 rB.gates 字段、在第一次 rB.gate(...) 调用之前做，
	// 不能先调 gate() 再断言 len(gates)==0——那两件事互斥：len(gates)==0 只在
	// 第一次 gate() 调用之前成立，gate(key).st==nil 只在调用之后才能问，两者
	// 不可能同时拿来验证同一个时间点，这里选前者。
	if len(rB.gates) != 0 {
		t.Fatalf("前置条件不成立：全新 Runner 的 gates 应当为空，实际 %d 条", len(rB.gates))
	}
	fsB := &fakeSpawner{}
	fsB.enqueueSuccess(claudeT2JSON)
	rB.spawn = fsB.spawn

	recB, err := rB.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第二个 Runner（模拟重启后）失败: %v", err)
	}
	if recB.Fresh {
		t.Error("读回了库里的会话，这一轮不该是 Fresh")
	}
	if recB.System != nil {
		t.Errorf("读回了库里的会话，不该发 session_opening 面包屑，实际: %+v", recB.System)
	}
	if len(fsB.calls) != 1 {
		t.Fatalf("期望 Spawner 恰好被调用 1 次，实际 %d 次", len(fsB.calls))
	}
	cmd := fsB.calls[0].cmd
	foundResume := false
	for i, a := range cmd.Args {
		if a == "--resume" && i+1 < len(cmd.Args) && cmd.Args[i+1] == recA.ProviderSession {
			foundResume = true
		}
	}
	if !foundResume {
		t.Errorf("BuildTurn 传给 Spawner 的 Command.Args 里没有 \"--resume %s\"，实际 Args: %v", recA.ProviderSession, cmd.Args)
	}
}

// TestAskFreshWhenNothingInStoreYet 覆盖「库里没有时按 Fresh 走」。这条与
// runner_session_test.go 的 TestFreshFirstTurnPostsSessionOpeningBreadcrumb
// 重叠（那条测的是同一个初始状态），这里保留一个最小版本只是为了让「读回」
// 这一组的清单在本文件内自成一体，不必跳到别的文件去对应这一条。
func TestAskFreshWhenNothingInStoreYet(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一次开口，库里还没有任何一条 provider_sessions")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if !rec.Fresh {
		t.Error("库里没有这个 agent 的任何记录，这一轮应当是 Fresh")
	}
	if rec.System == nil {
		t.Error("Fresh 那一轮应当发 session_opening 面包屑")
	}
}

// TestReadBackCursorExcludesAlreadyDeliveredSeqs 覆盖「读回的游标生效：库里
// cursor_seq = N 时，这一轮的 delta 不含 seq <= N 的消息」。
func TestReadBackCursorExcludesAlreadyDeliveredSeqs(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	m1 := postUser(t, svc, convID, "问题一，游标之前，不该出现在 delta 里")
	m2 := postUser(t, svc, convID, "问题二，游标之后，应当出现在 delta 里")

	// r 必须先造出来：指纹第七段依赖 r.workDir，种子指纹要跟 Ask 真正
	// 用的这条 workDir 算出来的一致，否则会被一次「命令行」漂移误伤
	// （两个临时目录不一样，不是任何真实配置变化）。
	r := newTestRunner(t, cfg, svc)
	fp := fingerprintFor(t, r, convID, testAgentClaude, cfg.Agents[testAgentClaude])
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: claudeFixtureSessionID, // 与 claudeT2JSON 回传值相同，避免触发 Drifted 干扰本用例
		CursorSeq:         m1.Seq,                 // 游标正好停在第一条上：它已经投递过
		LastTurnAt:        time.Now().UTC(),
		IdentityFP:        fp,
		Spoken:            true,
	})

	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT2JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	for _, seq := range rec.Delivered {
		if seq <= m1.Seq {
			t.Errorf("Delivered 里出现了 seq=%d，应当已被游标（cursor_seq=%d）排除", seq, m1.Seq)
		}
	}
	found := false
	for _, seq := range rec.Delivered {
		if seq == m2.Seq {
			found = true
		}
	}
	if !found {
		t.Errorf("Delivered 应当含游标之后的 seq=%d，实际: %v", m2.Seq, rec.Delivered)
	}
}

// TestReadBackLastTurnAtDrivesColdExpected 覆盖「读回的 last_turn_at 生效：
// 用可控时钟把它推到超过 CacheTTL，断言 ColdExpected 为真；没超为假」。
func TestReadBackLastTurnAtDrivesColdExpected(t *testing.T) {
	t.Run("超过 CacheTTL", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "问题")

		// r 先造出来：种子指纹要与 Ask 真正用的 r.workDir 一致，理由见
		// fingerprintFor 的注释。
		r := newTestRunner(t, cfg, svc)
		fp := fingerprintFor(t, r, convID, testAgentClaude, cfg.Agents[testAgentClaude])
		clock := newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
		old := clock.now().Add(-(CacheTTL + time.Minute))
		seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
			ProviderSessionID: claudeFixtureSessionID,
			LastTurnAt:        old,
			IdentityFP:        fp,
			Spoken:            true,
		})

		r.now = clock.now
		fs := &fakeSpawner{}
		fs.enqueueSuccess(claudeT2JSON)
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if err != nil {
			t.Fatalf("Ask 失败: %v", err)
		}
		if !rec.ColdExpected {
			t.Error("读回的 last_turn_at 已经超过 CacheTTL，ColdExpected 应当为真")
		}
	})

	t.Run("未超过 CacheTTL", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "问题")

		r := newTestRunner(t, cfg, svc)
		fp := fingerprintFor(t, r, convID, testAgentClaude, cfg.Agents[testAgentClaude])
		clock := newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
		recent := clock.now().Add(-(CacheTTL / 2))
		seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
			ProviderSessionID: claudeFixtureSessionID,
			LastTurnAt:        recent,
			IdentityFP:        fp,
			Spoken:            true,
		})

		r.now = clock.now
		fs := &fakeSpawner{}
		fs.enqueueSuccess(claudeT2JSON)
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if err != nil {
			t.Fatalf("Ask 失败: %v", err)
		}
		if rec.ColdExpected {
			t.Error("读回的 last_turn_at 距今未超过 CacheTTL，ColdExpected 应当为假")
		}
	})
}

// TestReadBackCostBaselineProducesDeltaNotRawTotal 覆盖「读回的 cost 基线
// 生效：库里 cost_total_usd 与 have_cost_total=true 时，这一轮的
// CostUSDDelta 是差值而不是本轮累计值」——对应规格第 4 节那个「只丢钱不
// 报错」的漏：若基线没被读回，重启后第一次差值会把整个会话的累计额当成
// 单轮成本记上去，而且这个数字看起来完全合理，不会有任何一层报错。
//
// 用两个 Runner 共享同一个 svc（模拟重启）而不是手工 seed：claudeT1JSON
// 与 claudeT2JSON 的 session_id 相同，第一个 Runner 建立的基线
// （0.9551999999999999）必须让第二个 Runner 在完全没有内存状态的情况下
// 读回，才是这条规则真正要挡的场景。
func TestReadBackCostBaselineProducesDeltaNotRawTotal(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮")

	rA := newTestRunner(t, cfg, svc)
	fsA := &fakeSpawner{}
	fsA.enqueueSuccess(claudeT1JSON)
	rA.spawn = fsA.spawn
	if _, err := rA.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一个 Runner 失败: %v", err)
	}

	postUser(t, svc, convID, "第二轮，模拟重启之后")
	rB := newTestRunnerSameWorkDir(rA, cfg, svc)
	fsB := &fakeSpawner{}
	fsB.enqueueSuccess(claudeT2JSON)
	rB.spawn = fsB.spawn

	rec, err := rB.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第二个 Runner 失败: %v", err)
	}
	if rec.CostUSDDelta == nil {
		t.Fatal("CostUSDDelta 为 nil")
	}
	const tol = 1e-6
	wantDelta := 1.349789 - 0.9551999999999999
	if !approxEqual(*rec.CostUSDDelta, wantDelta, tol) {
		t.Errorf("CostUSDDelta = %v，期望约 %v（差值，不是本轮累计值）", *rec.CostUSDDelta, wantDelta)
	}
	if approxEqual(*rec.CostUSDDelta, 1.349789, tol) {
		t.Fatal("CostUSDDelta 等于本轮累计值，说明基线没有从库里读回——重启后第一次差值会把整个会话的累计额当成单轮成本记上去（10_STEP7_SPEC.md 第 3.2 节）")
	}

	ps, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("ProviderSession 读回失败: ok=%v err=%v", ok, err)
	}
	if !ps.HaveCostTotal {
		t.Error("HaveCostTotal 应当为真")
	}
	if !approxEqual(ps.CostTotalUSD, 1.349789, tol) {
		t.Errorf("库里的 cost_total_usd = %v，期望等于最后一轮的累计值 1.349789", ps.CostTotalUSD)
	}
}

// TestIdentityDriftInMemoryRefusesWithoutSpawning 覆盖「指纹的两个分支」
// 之一：进程内已有的那条指纹不符 -> Ask 返回 ErrIdentityDrift，拒绝，且
// Spawner 一次都没被调用。白盒直接在这条 (会话, 成员) 的 gate.st 里放一条
// 记录，不需要真的跑一轮去建立它——这正是本用例要单独测的意义：进程内已有
// 记录指纹不符，必须走拒绝，不能落到「从库读回」那一支的「作废重建」处置上。
func TestIdentityDriftInMemoryRefusesWithoutSpawning(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	key := sessionKey{convID: convID, agentID: testAgentClaude}
	// 本用例单 goroutine，预置时没有并发，不取 g.mu。
	r.gate(key).st = &sessionState{
		providerSession: "an-existing-in-memory-session-id",
		identityFP:      "deliberately-wrong-fingerprint",
		spoken:          true,
	}

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if !errors.Is(err, ErrIdentityDrift) {
		t.Fatalf("errors.Is(err, ErrIdentityDrift) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if len(fs.calls) != 0 {
		t.Fatalf("Spawner 被调用了 %d 次，期望 0：进程内已有记录的指纹不符必须在起进程之前拒绝", len(fs.calls))
	}
}

// TestIdentityMismatchFromStoreRebuildsFreshAndEvictsOldRow 覆盖「指纹的
// 两个分支」之二：从库读回的那条指纹不符 -> 不报错，按 Fresh 重建
// （Record.Fresh 为真、发 session_opening、BuildTurn 收到的 ProviderSession
// 是空串），并且旧会话被作废（游标、cost 基线、会话 id 全部不留）。
//
// 种下的旧行游标推得很高（99，远大于本轮唯一一条消息的 seq）、基线抬得
// 很高（100.0）：
//   - 游标 99 没作废 -> readAfter(99) 扫不到任何新内容 -> ErrNoDelta ->
//     Spawner 一次都不会被调用（下面断言恰好调用了 1 次，直接反证游标已废，
//     这一条判据是判别性的：两种处置会产生不同的可观测结果）。
//   - CostUSDDelta 那条断言弱一些，如实说明：当前实现在指纹不符时会重新
//     构造一个全新的 &sessionState{}（haveCostTotal 天然是 false），所以
//     这里测到的「delta 约等于本轮原始累计值」在「基线被显式作废」与
//     「基线从未被写进这个新对象」两种实现方式下结果相同，不能单独区分
//     两者；它是防「未来重构不慎把 loaded 的基线也搬进新对象」这类回归的
//     守卫，不是对当前实现「确实执行了一次作废动作」的判别性证据。
func TestIdentityMismatchFromStoreRebuildsFreshAndEvictsOldRow(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	m1 := postUser(t, svc, convID, "问题，应当出现在 bootstrap 的 delta 里")

	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "stale-session-id-from-a-previous-run",
		CursorSeq:         99,
		CostTotalUSD:      100.0,
		HaveCostTotal:     true,
		LastTurnAt:        time.Now().UTC(),
		IdentityFP:        "a-fingerprint-from-a-different-config",
		Spoken:            true,
	})

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 应当成功（指纹不符按 Fresh 重建处理，不报错），实际: %v", err)
	}
	if !rec.Fresh {
		t.Error("Record.Fresh 应当为真：指纹不符必须按 Fresh 重建")
	}
	if rec.System == nil || rec.System.Meta[MetaEvent] != "session_opening" {
		t.Errorf("应当发一条 session_opening 面包屑，实际 System=%+v", rec.System)
	}

	// 未决 #18 第 4 点：静默作废重建要变响亮。这一轮除了 rec.System 挂着的
	// session_opening（Ask 流程里发生在更后面，覆盖了 rec.System 这个字段），
	// 还应当有一条更早发出、只落在事实源里的 session_rebuilt 面包屑——
	// 种子指纹是一个解不开的坏编码，所以 Reason 必须是「指纹格式已升级」。
	history, histErr := svc.History(context.Background(), convID, 0, 100)
	if histErr != nil {
		t.Fatalf("读历史失败: %v", histErr)
	}
	var rebuiltMsg *conversation.Message
	for i := range history {
		if history[i].Meta[MetaEvent] == "session_rebuilt" {
			rebuiltMsg = &history[i]
			break
		}
	}
	if rebuiltMsg == nil {
		t.Fatal("库里没有找到一条 session_rebuilt 面包屑：静默作废重建应当被记成一条响亮的事实")
	}
	if !strings.Contains(rebuiltMsg.Text, "指纹格式已升级") {
		t.Errorf("session_rebuilt 面包屑正文 = %q，期望点名「指纹格式已升级」（种子指纹是坏编码，解不开）", rebuiltMsg.Text)
	}
	if !strings.Contains(rebuiltMsg.Text, testAgentClaude) {
		t.Errorf("session_rebuilt 面包屑正文 = %q，期望点名 agent id %q", rebuiltMsg.Text, testAgentClaude)
	}

	if len(fs.calls) != 1 {
		t.Fatalf("期望 Spawner 恰好被调用 1 次，实际 %d 次", len(fs.calls))
	}
	for i, a := range fs.calls[0].cmd.Args {
		if a == "--resume" {
			t.Fatalf("Command.Args 里出现了 --resume（下标 %d）：指纹不符按 Fresh 重建时 BuildTurn 应当收到空的 ProviderSession，claude 只在非空时才发这个 flag，实际 Args: %v", i, fs.calls[0].cmd.Args)
		}
	}
	foundM1 := false
	for _, seq := range rec.Delivered {
		if seq == m1.Seq {
			foundM1 = true
		}
	}
	if !foundM1 {
		t.Errorf("Delivered 应当含 seq=%d：旧游标 99 必须被作废，否则这条消息会被当成「已经投递过」而永远漏掉", m1.Seq)
	}
	if rec.CostUSDDelta == nil {
		t.Fatal("CostUSDDelta 为 nil：旧基线 100.0 没有被作废，被当成「累计值回退」处理了")
	}
	const tol = 1e-6
	if !approxEqual(*rec.CostUSDDelta, 0.9551999999999999, tol) {
		t.Errorf("CostUSDDelta = %v，期望约等于本轮原始累计值 0.9552（旧基线已归零）", *rec.CostUSDDelta)
	}

	ps, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("ProviderSession 读回失败: ok=%v err=%v", ok, err)
	}
	wantFP := fingerprintFor(t, r, convID, testAgentClaude, cfg.Agents[testAgentClaude])
	if ps.IdentityFP != wantFP {
		t.Errorf("库里的 identity_fp = %q，期望被新配置的指纹 %q 覆盖", ps.IdentityFP, wantFP)
	}
	if ps.ProviderSessionID == "stale-session-id-from-a-previous-run" {
		t.Error("库里的 provider_session_id 仍是种下去的旧值，旧会话没有被作废")
	}
}
