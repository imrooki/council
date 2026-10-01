package turn

// 本文件是 Step 9 片 3d（docs/12_STEP9_CONTRACT.md 4.6a）RunRoundPrepared 的
// 背对背测试。实现方此刻在独立写 internal/turn/scheduler.go 里的同一个符号，
// 本文件与它互不读对方产出。冻结契约（任务书原话）：
//
//	func (r *Runner) RunRoundPrepared(ctx context.Context, convID string, plan Plan, maxParallel int, snap RoundSnap) []Record
//
// 这个符号此刻不存在，本文件编译不过（undefined: RunRoundPrepared）是预期状态，
// 不为了让它过编译去改任何实现文件，也不去 stub 一个假的。
//
// 本文件只加这一个文件，不改 scheduler.go / runround_test.go / recording_store_test.go
// 等既有文件一个字节；复用它们已经定义好的同包符号（raceSpawner、
// recordingStore、newFiveAgentConfig、fiveAgentIDs、successJSONFor、
// freshPlanFiveSpeakers、argFlagValue、newTestService/newConv/joinAgent/postUser/
// newTestRunner 等），体例参考 runround_test.go 与 round_cancel_test.go。

import (
	"context"
	"strings"
	"sync"
	"testing"

	"council/internal/conversation"
)

// ---------------------------------------------------------------------------
// 本文件共用的小断言辅助
// ---------------------------------------------------------------------------

// assertAgentIDOrderMatches 断言 recs 与 wantIDs 逐位相同：长度、顺序、AgentID。
// 供本文件多条用例共用，不重复写这段循环。
func assertAgentIDOrderMatches(t *testing.T, recs []Record, wantIDs []string, label string) {
	t.Helper()
	if len(recs) != len(wantIDs) {
		t.Fatalf("%s 返回 %d 条 Record，期望 %d", label, len(recs), len(wantIDs))
	}
	for i, want := range wantIDs {
		if recs[i].AgentID != want {
			t.Fatalf("%s 第 %d 位 AgentID = %q，期望 %q——返回顺序应当与 plan.Speakers 一致，不是完成顺序",
				label, i, recs[i].AgentID, want)
		}
	}
}

// assertAllOutcomeSpoke 断言 recs 里每一条的 Outcome 都是 OutcomeSpoke。
// 多条用例把它当前置条件：Outcome 不对时，后面对身份块/顺序/相等性的断言
// 都无从谈起，先在这里 Fatal 比在更下面看一个不相关的失败要清楚。
func assertAllOutcomeSpoke(t *testing.T, recs []Record, label string) {
	t.Helper()
	for _, rec := range recs {
		if rec.Outcome != OutcomeSpoke {
			t.Fatalf("%s 里 agent %q 的 Outcome = %v（期望 OutcomeSpoke）——前置条件不成立，"+
				"下面的断言测不到东西", label, rec.AgentID, rec.Outcome)
		}
	}
}

// ---------------------------------------------------------------------------
// 用例 1（本组主断言）：快照在手，中途改规则不影响这一轮——4.6a 验收 2a
// ---------------------------------------------------------------------------
//
// 判据：这一轮真正送给 provider 的身份块，来自 snap.Rules（TakeRoundSnap
// 那一刻生效的规则），不是 RunRoundPrepared 被调用那一刻 Runner 里的实时规则。
//
// 身份块走的是 provider.TurnRequest.Role，不是 Prompt：renderIdentity
// （render.go）把 RulesBlocks 拼进 Role，各家适配器再把 Role 放上不同的载体——
// grok（grok.go:99）直接 `--system-prompt-override <Role>` 进 argv，是三家里
// 唯一把身份块整段原样放进一个能直接断言的 argv 值的一家（claude 走
// --append-system-prompt-file 落一个临时文件，agy 拼进首轮正文）。选一个
// grok 成员（newFiveAgentConfig 里 i=1 的 agent-b，provider 按 i%2 轮换），
// 用 raceSpawner 记下真实调用的 provider.Command，argFlagValue 精确取出
// --system-prompt-override 的值，不去猜它落在 Stdin 还是某个别的 flag 里。
func TestRunRoundPreparedUsesSnapRulesNotLiveRulesAtCallTime(t *testing.T) {
	cfg := newFiveAgentConfig()
	const speaker = "agent-b" // provider 按 i%2 轮换，i=1 是 grok

	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, speaker)
	postUser(t, svc, convID, "prepared-rules-seed")

	r := newTestRunner(t, cfg, svc)

	const oldMarker = "RULES-MARK-OLD-7f3e1a9c"
	const newMarker = "RULES-MARK-NEW-2c9d84f1"

	// 步骤 1-2：先放一份旧规则，再取快照。
	r.SetRules(RulesBlocks{Global: oldMarker})
	plan := Plan{Speakers: []string{speaker}, Judges: []string{}, Skipped: []SkipReason{}}
	snap, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("TakeRoundSnap 失败: %v", err)
	}
	if !strings.Contains(snap.Rules.Global, oldMarker) {
		t.Fatalf("前置条件不成立：snap.Rules.Global = %q，里面没有旧规则特征串 %q——"+
			"TakeRoundSnap 没有如预期取到当时生效的规则，下面的用例测不到东西",
			snap.Rules.Global, oldMarker)
	}

	// 步骤 3：拿到预报，模拟 /round 真实流程里"先出预报再执行"这一步。
	rows, err := r.BudgetWith(context.Background(), convID, plan, snap)
	if err != nil {
		t.Fatalf("BudgetWith 失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("BudgetWith 返回 %d 行，期望 1", len(rows))
	}

	// 步骤 4：模拟用户在预报之后、执行之前敲了 /rules reload --apply。
	r.SetRules(RulesBlocks{Global: newMarker})
	if !strings.Contains(r.Rules().Global, newMarker) {
		t.Fatalf("前置条件不成立：SetRules 换新规则之后，r.Rules().Global 里读不到新规则特征串 %q——"+
			"说明 SetRules 本身没生效，与 RunRoundPrepared 无关，先修这一步", newMarker)
	}

	cs := newRaceSpawner()
	r.spawn = cs.spawn
	cs.enqueue(speaker, spawnResult{stdout: []byte(successJSONFor(cfg, speaker, "SESS-PREP-1", "ok")), exitCode: 0})

	// 步骤 5：RunRoundPrepared 收到的是步骤 2 那份旧快照。
	recs := r.RunRoundPrepared(context.Background(), convID, plan, 0, snap)
	assertAgentIDOrderMatches(t, recs, []string{speaker}, "RunRoundPrepared")
	assertAllOutcomeSpoke(t, recs, "RunRoundPrepared")

	calls := cs.callsFor(speaker)
	if len(calls) != 1 {
		t.Fatalf("agent %q 的 Spawner 调用次数 = %d，期望恰好 1", speaker, len(calls))
	}
	role, ok := argFlagValue(calls[0].cmd.Args, "--system-prompt-override")
	if !ok {
		t.Fatalf("grok 的 argv 里没有找到 --system-prompt-override，身份块的传递方式与预期（grok.go:99）不符，"+
			"argv=%v", calls[0].cmd.Args)
	}

	// 步骤 6：成对断言。只断言含旧串的话，一个"新旧规则都拼进身份块"的坏实现也能过；
	// 只断言不含新串的话，一个"身份块整个是空的/坏掉的"的坏实现也能过。
	if !strings.Contains(role, oldMarker) {
		t.Errorf("这一轮送给 provider 的身份块里没有旧规则特征串 %q——期望 RunRoundPrepared 按 snap.Rules"+
			"（TakeRoundSnap 那一刻的规则）渲染。实际身份块：\n%s", oldMarker, role)
	}
	if strings.Contains(role, newMarker) {
		t.Errorf("这一轮送给 provider 的身份块里出现了新规则特征串 %q——RunRoundPrepared 不该去读调用它那一刻"+
			"Runner 里的实时规则，应当只用传入的 snap。实际身份块：\n%s", newMarker, role)
	}
}

// ---------------------------------------------------------------------------
// 用例 2：一轮一个 roundID，且不是空串；两次调用各铸各的
// ---------------------------------------------------------------------------
//
// 判据：同一次 RunRoundPrepared 调用里，全部 Speakers 的 Record.RoundID 两两
// 相等且非空（防「没铸」或「每个成员各铸各的」）；两次独立调用之间的 roundID
// 不同（防「铸了一个常量」或「整个 Runner 共用一个」）。
//
// 每次调用前都重新 TakeRoundSnap：若两次共用同一份陈旧快照，第二轮的 upTo
// 仍停在第一轮之前，第一轮各成员的游标已经推过 upTo，Select 在 [cursor, upTo]
// 区间里可能空手而归，走到既有的 ErrNoDelta refuse 路径——那样第二轮的
// Outcome 会变得不可预测，混淆了本用例真正要测的东西（roundID 的铸造规则，
// 不是 Outcome）。RoundID 无论 Outcome 是什么都必须非空（record.go 原话：
// 「Ask 铸了再传，这一列永远非空」），但为了让两轮都干净地成功、把断言
// 焦点留在 roundID 上，这里仍然按真实流程重取快照。
func TestRunRoundPreparedRoundIDPerCallUniqueNonEmpty(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()[:3] // agent-a, agent-b, agent-c，字典序

	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "prepared-roundid-seed")

	r := newTestRunner(t, cfg, svc)
	plan := Plan{Speakers: append([]string{}, ids...), Judges: []string{}, Skipped: []SkipReason{}}

	cs := newRaceSpawner()
	r.spawn = cs.spawn

	snap1, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("第一次 TakeRoundSnap 失败: %v", err)
	}
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS-RID1-"+id, "ok1-"+id)), exitCode: 0})
	}
	recs1 := r.RunRoundPrepared(context.Background(), convID, plan, 0, snap1)
	assertAgentIDOrderMatches(t, recs1, ids, "第一次 RunRoundPrepared")

	roundID1 := recs1[0].RoundID
	if roundID1 == "" {
		t.Fatal("recs1[0].RoundID 是空串，期望非空")
	}
	for i, rec := range recs1 {
		if rec.RoundID != roundID1 {
			t.Errorf("recs1[%d]（agent %q）的 RoundID = %q，期望与 recs1[0] 相同的 %q——"+
				"同一轮全部成员应当共用同一个 roundID", i, rec.AgentID, rec.RoundID, roundID1)
		}
	}

	snap2, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("第二次 TakeRoundSnap 失败: %v", err)
	}
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS-RID2-"+id, "ok2-"+id)), exitCode: 0})
	}
	recs2 := r.RunRoundPrepared(context.Background(), convID, plan, 0, snap2)
	assertAgentIDOrderMatches(t, recs2, ids, "第二次 RunRoundPrepared")

	roundID2 := recs2[0].RoundID
	if roundID2 == "" {
		t.Fatal("recs2[0].RoundID 是空串，期望非空")
	}
	for i, rec := range recs2 {
		if rec.RoundID != roundID2 {
			t.Errorf("recs2[%d]（agent %q）的 RoundID = %q，期望与 recs2[0] 相同的 %q",
				i, rec.AgentID, rec.RoundID, roundID2)
		}
	}

	if roundID1 == roundID2 {
		t.Fatalf("两次 RunRoundPrepared 铸出的 roundID 相同：%q——期望每次调用各铸各的，"+
			"否则要么是铸了一个常量，要么是整个 Runner 共用一个 roundID 字段", roundID1)
	}
}

// ---------------------------------------------------------------------------
// 用例 3：RunRoundPrepared 自己不取快照——不调 MaxSeq
// ---------------------------------------------------------------------------
//
// 生产代码里 .MaxSeq( 的调用点：本文件作者独立查过 internal/turn 全部非测试
// .go 文件（budget.go / scheduler.go / runner.go / spawn.go / session.go /
// render.go / plan.go / record.go / permission.go / rules_report.go /
// drift.go / router.go），唯一一处真正的调用是 budget.go:62
// `r.svc.MaxSeq(ctx, convID)`，在 TakeRoundSnap 内部；scheduler.go 里出现的
// 全部是注释提到"MaxSeq"这个词，没有第二处调用。往外查到
// internal/repl（ask.go/commands.go/round.go）也调 svc.MaxSeq，但那是 REPL
// 层，不在 RunRoundPrepared 的调用链上。conversation.Service 除了
// Service.MaxSeq 自身转发给 store 那一行，Post/RecordTurn/Join 等方法内部
// 都不碰 MaxSeq（service.go 通篇只有那一处字符串命中）。这条判据成立。
//
// 判据：用同包 recordingStore（recording_store_test.go）为基础包一层
// 计数——recordingStore 自己的 MaxSeq 方法不记调用次数，本文件照它的风格
// 另包一层，不改 recording_store_test.go 一个字节。取 delta 而不是绝对值：
// TakeRoundSnap 本身合法地调一次 MaxSeq，把「调用前」的计数读出来，
// RunRoundPrepared 跑完之后必须与它相等。
type maxSeqCountingStore struct {
	*recordingStore

	mu    sync.Mutex
	calls int
}

func newMaxSeqCountingStore() *maxSeqCountingStore {
	return &maxSeqCountingStore{recordingStore: newRecordingStore()}
}

func (s *maxSeqCountingStore) MaxSeq(ctx context.Context, convID string) (int64, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.recordingStore.MaxSeq(ctx, convID)
}

func (s *maxSeqCountingStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestRunRoundPreparedDoesNotCallMaxSeqItself(t *testing.T) {
	rs := newMaxSeqCountingStore()
	svc := conversation.NewService(rs)
	cfg := newFiveAgentConfig()
	const speaker = "agent-a"

	convID := newConv(t, svc)
	joinAgent(t, svc, convID, speaker)
	postUser(t, svc, convID, "prepared-maxseq-seed")

	r := newTestRunner(t, cfg, svc)
	plan := Plan{Speakers: []string{speaker}, Judges: []string{}, Skipped: []SkipReason{}}

	snap, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("TakeRoundSnap 失败: %v", err)
	}
	if rs.callCount() == 0 {
		t.Fatal("前置条件不成立：TakeRoundSnap 应当至少调用一次 MaxSeq，这里计到 0——" +
			"计数挂钩本身没接对，下面「delta 为 0」的断言没有意义")
	}
	before := rs.callCount()

	cs := newRaceSpawner()
	r.spawn = cs.spawn
	cs.enqueue(speaker, spawnResult{stdout: []byte(successJSONFor(cfg, speaker, "SESS-NOMAXSEQ", "ok")), exitCode: 0})

	_ = r.RunRoundPrepared(context.Background(), convID, plan, 0, snap)

	// 前置条件：这一轮真的走到了起进程那一步。不然「MaxSeq 计数不变」可能只是
	// 因为整轮在更早的地方短路了（例如提前 refuse），而不是因为
	// RunRoundPrepared 真的没有在 askUpToWith 路径上的任何一处重新取快照——
	// 那样的话本用例的判别力是假的。claude/recordingStore 组合能走到 spawn
	// 这一步，round_cancel_test.go 已经证明过同一条路径。
	if n := len(cs.callsFor(speaker)); n != 1 {
		t.Fatalf("前置条件不成立：agent %q 的 Spawner 调用次数 = %d，期望 1——"+
			"这一轮没有走完到起进程那一步，「MaxSeq 计数不变」证明不了任何事", speaker, n)
	}

	after := rs.callCount()
	if after != before {
		t.Errorf("RunRoundPrepared 这一次调用期间 Store.MaxSeq 的调用次数从 %d 变成 %d（多了 %d 次），"+
			"期望不变——契约第 1 条：RunRoundPrepared 收到调用方已经取好的 snap，自己不再取快照"+
			"（不调 TakeRoundSnap、不调 MaxSeq）", before, after, after-before)
	}
}

// ---------------------------------------------------------------------------
// 用例 4：RunRound 与「TakeRoundSnap + RunRoundPrepared」两步等价
// ---------------------------------------------------------------------------
//
// 「同一份输入」不能是同一个 Runner 对同一条会话先后跑 RunRound 再跑两步版：
// 第一次跑完，5 个成员已经各自建立了 provider 会话、游标也推进了，第二次会把
// 第一次的回复当成新 delta，两条路径的输入结构性不同（与 runround_test.go
// TestRunRoundSameFixtureTwice... 的理由同源）。改用同一个 Runner + 两条独立
// 会话，种子文本相同、按相同顺序 post。
//
// 只比长度、顺序、每行的 AgentID 与 Outcome；RoundID 与 TurnID 必然不同
// （两条路径各铸各的），不断言它们相等。比较之前先各自断言全员 OutcomeSpoke——
// 否则「Outcome 一致」可能只是「两边都被拒了」这种没有判别力的巧合。
//
// 有一点契约本身没法在这里测到，只记一句：RunRound 是先铸 roundID 再
// TakeRoundSnap，TakeRoundSnap 失败时返回的 Record 会带着 roundID 与 Err；
// 两步版里 TakeRoundSnap 的错误由调用方直接拿到 error，不经过 RunRoundPrepared
// 铸出一批 Record。这条分支只在 TakeRoundSnap 失败时才分道扬镳，本用例走的
// 是成功路径，测不到这条差异。
func TestRunRoundPreparedEquivalentToRunRoundTwoStep(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	setupConv := func() string {
		convID := newConv(t, svc)
		for _, id := range ids {
			joinAgent(t, svc, convID, id)
		}
		postUser(t, svc, convID, "prepared-equiv-seed")
		return convID
	}
	convA := setupConv()
	convB := setupConv()

	planA := freshPlanFiveSpeakers()
	planB := freshPlanFiveSpeakers()

	// A：RunRound 一步到位。
	csA := newRaceSpawner()
	r.spawn = csA.spawn
	for _, id := range ids {
		csA.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS-EQ-A-"+id, "ok-a-"+id)), exitCode: 0})
	}
	recsA := r.RunRound(context.Background(), convA, planA, 0)
	assertAllOutcomeSpoke(t, recsA, "RunRound（一步版）")

	// B：TakeRoundSnap + RunRoundPrepared 拆成两步。
	snapB, err := r.TakeRoundSnap(context.Background(), convB)
	if err != nil {
		t.Fatalf("TakeRoundSnap 失败: %v", err)
	}
	csB := newRaceSpawner()
	r.spawn = csB.spawn
	for _, id := range ids {
		csB.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS-EQ-B-"+id, "ok-b-"+id)), exitCode: 0})
	}
	recsB := r.RunRoundPrepared(context.Background(), convB, planB, 0, snapB)
	assertAllOutcomeSpoke(t, recsB, "TakeRoundSnap+RunRoundPrepared（两步版）")

	if len(recsA) != len(recsB) {
		t.Fatalf("长度不一致：RunRound=%d 条，两步版=%d 条", len(recsA), len(recsB))
	}
	for i := range recsA {
		if recsA[i].AgentID != recsB[i].AgentID {
			t.Errorf("第 %d 位 AgentID 不一致：RunRound=%q，两步版=%q", i, recsA[i].AgentID, recsB[i].AgentID)
		}
		if recsA[i].Outcome != recsB[i].Outcome {
			t.Errorf("agent %q 的 Outcome 不一致：RunRound=%v，两步版=%v",
				recsA[i].AgentID, recsA[i].Outcome, recsB[i].Outcome)
		}
	}
}
