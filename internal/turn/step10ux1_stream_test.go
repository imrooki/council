package turn

// 本文件覆盖冻结契约 E:\multi_model_cowork\step10ux1_contract.md 第 6 节
// 验收 1-6：RunRoundPreparedStream / RunRoundWithStream 与新回调类型
// RoundProgress 的行为，以及三个既有方法（RunRound / RunRoundPrepared /
// RunRoundWith）签名不变。
//
// 背对背纪律：本文件只依赖契约第 3 节冻结的形状——
//   type RoundProgress func(rec Record)
//   func (r *Runner) RunRoundPreparedStream(ctx context.Context, convID string, plan Plan, maxParallel int, snap RoundSnap, onDone RoundProgress) []Record
//   func (r *Runner) RunRoundWithStream(ctx context.Context, convID string, plan Plan, maxParallel int, roundID string, out []Record, snap RoundSnap, onDone RoundProgress) []Record
// 这些符号此刻还不存在（实现腿在独立写 scheduler.go），编译不过是预期
// 状态，不为了让它过编译而去改任何实现文件。
//
// 复用本包既有夹具，不改它们一个字节：newFiveAgentConfig / fiveAgentIDs /
// freshPlanFiveSpeakers / successJSONFor（runround_test.go），
// newTestService / newConv / joinAgent / postUser（fixtures_test.go），
// raceSpawner / agentIDFromCmd（gate_concurrency_test.go）。
//
// 泄漏披露：写本文件之前，本会话已经完整读过一次 scheduler.go（320 行，
// 不含任何 Stream 符号），随后一次 grep 命中该文件里出现了
// RunRoundPreparedStream——两次读取之间实现腿已经写入了工作树。round.go
// 也在本会话中途被读到过一次完整版本，其中已经含 RunRoundPreparedStream
// 调用与逐条渲染回调（这处泄漏发生在读取当时，不确定是会话开始前就有
// 还是会话中途写入）。本文件的断言只依据契约文本，不依据这两处读到的
// 实现细节；docs/00_DECISIONS.md 与 scheduler.go 之后没有再读。

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"council/internal/provider"
)

// ---------------------------------------------------------------------------
// barrierSpawner：让至少两个成员真正「同时」返回，验收 2 的前提
// ---------------------------------------------------------------------------

// barrierSpawner 是本文件专用的假 Spawner：want 次调用全部到达之后
// （每个成员各调一次 spawn，在 askUpToWith 内部各自的 goroutine 里），
// 统一在 <-release 上放行——放行之前谁都不能提前返回。这就是契约 6.2
// 明写的前提：「必须让至少两个成员同时返回」。
//
// 不复用 raceSpawner：raceSpawner 只支持「各自睡各自的时长」，没有「全部
// 到齐再一起放行」这个语义，硬凑达不到「同时」，只能凑近似。
type barrierSpawner struct {
	// successJSON 按 agentID 造一份成功回放的 JSON，调用方（本文件）用
	// successJSONFor（runround_test.go）当实参，这里不重新依赖 config 包。
	successJSON func(agentID string) string

	mu        sync.Mutex
	want      int
	arrived   int
	fired     bool
	arrivedCh chan struct{}
	release   chan struct{}
}

func newBarrierSpawner(want int, successJSON func(agentID string) string) *barrierSpawner {
	return &barrierSpawner{
		successJSON: successJSON,
		want:        want,
		arrivedCh:   make(chan struct{}),
		release:     make(chan struct{}),
	}
}

func (b *barrierSpawner) spawn(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
	b.mu.Lock()
	b.arrived++
	if b.arrived >= b.want && !b.fired {
		b.fired = true
		close(b.arrivedCh)
	}
	b.mu.Unlock()

	<-b.release

	agentID := agentIDFromCmd(c)
	return []byte(b.successJSON(agentID)), nil, 0, false, nil
}

// waitArrived 等到 want 次调用全部到达（或超时）。超时时顺手把 release
// 关掉，不让已经卡在 <-release 上的 goroutine 泄漏到下一条用例。
func (b *barrierSpawner) waitArrived(t *testing.T, timeout time.Duration, msg string) {
	t.Helper()
	select {
	case <-b.arrivedCh:
	case <-time.After(timeout):
		close(b.release)
		t.Fatal(msg)
	}
}

// ---------------------------------------------------------------------------
// 验收 1：回调对每个成员恰好一次
// ---------------------------------------------------------------------------

func TestRunRoundPreparedStreamCallsOnDoneOnceEachMember(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "ux1-onedone-seed")

	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "UX1A-"+id, "ok-"+id)), exitCode: 0})
	}

	snap, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("TakeRoundSnap 失败: %v", err)
	}

	var mu sync.Mutex
	calls := map[string]int{}
	onDone := func(rec Record) {
		mu.Lock()
		calls[rec.AgentID]++
		mu.Unlock()
	}

	recs := r.RunRoundPreparedStream(context.Background(), convID, freshPlanFiveSpeakers(), 0, snap, onDone)
	if len(recs) != len(ids) {
		t.Fatalf("RunRoundPreparedStream 返回 %d 条 Record，期望 %d", len(recs), len(ids))
	}
	for _, rec := range recs {
		if rec.Outcome != OutcomeSpoke {
			t.Fatalf("agent %q 的 Outcome=%v，期望 OutcomeSpoke（前置条件不成立）", rec.AgentID, rec.Outcome)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != len(ids) {
		t.Fatalf("onDone 一共覆盖了 %d 个不同的 AgentID，期望 %d 个；calls=%v", len(calls), len(ids), calls)
	}
	for _, id := range ids {
		if n := calls[id]; n != 1 {
			t.Errorf("agent %q 的 onDone 被调用 %d 次，期望恰好 1 次", id, n)
		}
	}
}

// ---------------------------------------------------------------------------
// 验收 2：回调是串行的——这是本刀唯一会抓到真 bug 的用例
// ---------------------------------------------------------------------------
//
// 前提（契约 6.2 明写）：必须让至少两个成员真正同时返回，否则串行断言在
// 串行执行下恒真。用 barrierSpawner 让全部成员一起到达 spawn、一起放行。
//
// 检测重入：进入回调时 atomic.AddInt32 把计数加一，若结果 > 1 说明另一个
// 回调此刻也在跑，标一个 violated 标志（不在非测试 goroutine 里调
// t.Fatalf——那会 panic）；中间 sleep 100ms 把重入窗口撑开；退出前减一。
//
// 「前提真的成立」的见证：不能用「相邻两次进入回调的时间间隔 < sleep」
// 当见证——好实现把回调串行化之后，第 i+1 次进入必然发生在第 i 次退出
// 之后，间隔天然 >= sleep，这个前提对好实现永远不成立，用错误的判据会把
// 好实现误判成「没有制造出并发」。真正的见证是：把 (entry, exit) 按 entry
// 排序，断言至少存在相邻一对满足 entry[i+1] - exit[i] < 15ms——意思是
// 「第 i+1 个成员的回调几乎是在第 i 个成员的回调一放开锁就立刻接上」，
// 这只有在它们本来就排着队等同一把锁（即本来就想同时进来）时才会发生；
// 若两个成员原本就是先后很久才完成，接力间隔不会这么短。好实现下这个
// 见证必然成立（回调之间没有多余的等待）；没有这个见证就说明本用例没有
// 真的制造出「同时返回」，断言什么都没测到，Fatal 出来而不是让它悄悄绿过。
//
// maxParallel 传 0，避开信号量：sem 会人为限制并发进入 spawn 的 goroutine
// 数目，与本用例要测的「N 个成员一起返回」无关，传 0 更干净。
func TestRunRoundPreparedStreamOnDoneSerializedUnderConcurrentReturns(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "ux1-serial-seed")

	r := newTestRunner(t, cfg, svc)
	bs := newBarrierSpawner(len(ids), func(agentID string) string {
		return successJSONFor(cfg, agentID, "UX1B-"+agentID, "ok-"+agentID)
	})
	r.spawn = bs.spawn

	snap, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("TakeRoundSnap 失败: %v", err)
	}

	const holdInCallback = 100 * time.Millisecond

	var inCB int32
	var violated int32
	type span struct{ entry, exit time.Time }
	var spanMu sync.Mutex
	var spans []span

	onDone := func(rec Record) {
		c := atomic.AddInt32(&inCB, 1)
		if c > 1 {
			atomic.StoreInt32(&violated, 1)
		}
		entry := time.Now()
		time.Sleep(holdInCallback)
		exit := time.Now()
		atomic.AddInt32(&inCB, -1)
		spanMu.Lock()
		spans = append(spans, span{entry: entry, exit: exit})
		spanMu.Unlock()
	}

	done := make(chan []Record, 1)
	go func() {
		done <- r.RunRoundPreparedStream(context.Background(), convID, freshPlanFiveSpeakers(), 0, snap, onDone)
	}()

	bs.waitArrived(t, 5*time.Second,
		"5 个成员没有都在 5s 内到达假 Spawner——若 RunRoundPreparedStream 没有按冻结形状"+
			"真的并发扇出（每个成员各起一个 goroutine），这里会先死锁超时，"+
			"而不是本用例其余断言的问题")
	close(bs.release)

	var recs []Record
	select {
	case recs = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("RunRoundPreparedStream 在放行之后没有在 10s 内返回")
	}

	if len(recs) != len(ids) {
		t.Fatalf("RunRoundPreparedStream 返回 %d 条 Record，期望 %d", len(recs), len(ids))
	}
	for _, rec := range recs {
		if rec.Outcome != OutcomeSpoke {
			t.Errorf("agent %q 的 Outcome=%v，期望 OutcomeSpoke（前置条件不成立）", rec.AgentID, rec.Outcome)
		}
	}

	// 前提见证：先证明本用例真的制造出了「排队等同一把锁」的局面，
	// 不然下面「没有重入」的断言毫无意义（串行执行下恒真）。
	spanMu.Lock()
	ss := append([]span(nil), spans...)
	spanMu.Unlock()
	if len(ss) != len(ids) {
		t.Fatalf("onDone 一共记到 %d 段 (entry,exit)，期望 %d 段", len(ss), len(ids))
	}
	sort.Slice(ss, func(i, j int) bool { return ss[i].entry.Before(ss[j].entry) })
	const handoffSlack = 15 * time.Millisecond
	witnessed := false
	var minGap time.Duration = -1
	for i := 1; i < len(ss); i++ {
		gap := ss[i].entry.Sub(ss[i-1].exit)
		if minGap < 0 || gap < minGap {
			minGap = gap
		}
		if gap < handoffSlack {
			witnessed = true
		}
	}
	if !witnessed {
		t.Fatalf("前置条件不成立：没有观测到「一个成员的回调一退出、下一个几乎立刻接上」这个现象"+
			"（相邻 entry/exit 的最小间隔是 %v，期望至少一对 < %v）——说明本用例没有真的制造出"+
			"「至少两个成员同时返回」的局面，下面关于重入的断言测不到任何东西", minGap, handoffSlack)
	}

	if atomic.LoadInt32(&violated) != 0 {
		t.Fatalf("检测到重入：在某个时刻，onDone 同时有超过 1 个实例在执行——" +
			"回调没有被串行化，这正是契约 6.2 要抓的那个 bug")
	}
}

// ---------------------------------------------------------------------------
// 验收 3：回调收到的是值副本，且发生在 out[i] = rec 之后
// ---------------------------------------------------------------------------
//
// 「回调里改收到的 rec、断言最终 out 没被改」这条断言本身，在冻结签名
// type RoundProgress func(rec Record) 下是结构上不可能为假的：Go 对
// 非指针类型的函数参数恒按值传递，任何满足这个签名的实现，不论内部
// out[i] 还是局部变量传给 onDone，调用点都会先拷贝一份，回调对拷贝的
// 标量字段赋值无论如何都不可能写回调用方的 out[i]——除非签名本身被改成
// *Record，那样根本编译不过、不会走到运行期。这半条断言仍然写，
// 当一份回归/文档用途的钉子，报告里如实标注它抓不到实现质量问题。
//
// 真正有判别力的是另一半：回调收到的 rec 必须是 askUpToWith 真正跑完之后
// 的那条结果，不能是 RunRoundPreparedStream 一开始预填、还没被覆盖的
// 「哨兵」记录（scheduler.go 原文 RunRoundPrepared 的预填循环：
// out[i] = Record{ConvID: convID, AgentID: id, Outcome: OutcomeRefused}，
// TurnID 留空）。若实现把「先调用 onDone、再执行 out[i] = rec」的顺序
// 写反，回调这里看到的就会是预填的哨兵值（TurnID 空串、Outcome=Refused），
// 与一次真正成功之后的值（TurnID 非空、Outcome=Spoke）在这两个字段上
// 截然不同，本用例能抓住这个顺序错误。
func TestRunRoundPreparedStreamOnDoneSeesFinalRecordNotStalePrefill(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "ux1-copy-seed")

	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "UX1C-"+id, "ok-"+id)), exitCode: 0})
	}

	snap, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("TakeRoundSnap 失败: %v", err)
	}

	var mu sync.Mutex
	seen := make(map[string]Record, len(ids))
	onDone := func(rec Record) {
		mu.Lock()
		seen[rec.AgentID] = rec
		mu.Unlock()
		// 值副本的另一半（结构上恒真，见上方函数注释）：本地改一个标量
		// 字段，若真的传的是 &out[i]，这里会污染最终结果。
		rec.Outcome = OutcomeRefused
		rec.AgentID = "MUTATED-BY-TEST-CALLBACK"
	}

	recs := r.RunRoundPreparedStream(context.Background(), convID, freshPlanFiveSpeakers(), 0, snap, onDone)
	if len(recs) != len(ids) {
		t.Fatalf("RunRoundPreparedStream 返回 %d 条 Record，期望 %d", len(recs), len(ids))
	}
	for _, rec := range recs {
		if rec.Outcome != OutcomeSpoke {
			t.Fatalf("agent %q 最终 Outcome=%v，期望 OutcomeSpoke（前置条件不成立，"+
				"下面关于值副本与顺序的断言都无法验证任何事）", rec.AgentID, rec.Outcome)
		}
		if rec.AgentID == "MUTATED-BY-TEST-CALLBACK" {
			t.Errorf("最终 out 里的 AgentID 被回调的本地改动污染了——" +
				"onDone 传的不是值副本，可能是 &out[i]")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != len(ids) {
		t.Fatalf("onDone 一共被调用 %d 次，期望 %d 次", len(seen), len(ids))
	}
	for _, rec := range recs {
		got, ok := seen[rec.AgentID]
		if !ok {
			t.Fatalf("onDone 从未收到过 agent %q 的回调（它的本地记录已经被同一个闭包里"+
				"的另一次调用污染了 AgentID，说明值副本这半条本身就没有保住）", rec.AgentID)
		}
		if got.TurnID == "" {
			t.Errorf("agent %q 回调收到的 TurnID 是空串——像是 RunRoundPreparedStream/"+
				"RunRoundWithStream 预填、还没被真结果覆盖的那份哨兵值，说明 onDone 可能"+
				"在 out[i] = rec 之前就被调用了", rec.AgentID)
		}
		if got.TurnID != rec.TurnID {
			t.Errorf("agent %q 回调收到的 TurnID=%q，与最终 out 里的 TurnID=%q 不一致",
				rec.AgentID, got.TurnID, rec.TurnID)
		}
		if got.Outcome != OutcomeSpoke {
			t.Errorf("agent %q 回调收到的 Outcome=%v，期望 OutcomeSpoke——"+
				"如果这里仍是预填的 OutcomeRefused，说明 onDone 被调用得太早，"+
				"在 out[i] = rec 完成之前", rec.AgentID, got.Outcome)
		}
	}
}

// ---------------------------------------------------------------------------
// 验收 4：onDone 为 nil 不 panic
// ---------------------------------------------------------------------------
//
// 这条用例只能抓「实现漏了 if onDone != nil 判断」这一类最直接的疏漏——
// 一旦漏了，nil 函数值被调用会直接 panic，本用例的 recover 会抓到并
// Fatalf。抓不到更细的东西，如实标注。
func TestRunRoundPreparedStreamNilOnDoneDoesNotPanic(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "ux1-nil-seed")

	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "UX1D-"+id, "ok-"+id)), exitCode: 0})
	}

	snap, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("TakeRoundSnap 失败: %v", err)
	}

	var recs []Record
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("RunRoundPreparedStream(onDone=nil) panic: %v", rec)
			}
		}()
		recs = r.RunRoundPreparedStream(context.Background(), convID, freshPlanFiveSpeakers(), 0, snap, nil)
	}()

	if len(recs) != len(ids) {
		t.Fatalf("RunRoundPreparedStream(onDone=nil) 返回 %d 条 Record，期望 %d", len(recs), len(ids))
	}
	for _, rec := range recs {
		if rec.Outcome != OutcomeSpoke {
			t.Errorf("agent %q 的 Outcome=%v，期望 OutcomeSpoke——onDone=nil 不该改变既有的"+
				"正常业务行为", rec.AgentID, rec.Outcome)
		}
	}
}

// ---------------------------------------------------------------------------
// 验收 5：既有的三个方法签名没变
// ---------------------------------------------------------------------------
//
// 编译期证据放前面：把三个方法值赋给按契约冻结的原始函数类型的局部变量，
// 类型对不上会直接编译失败，不需要真的调用就证明了签名一个字没变。
// 运行期证据跟在后面：三个方法各真调用一次，确认转调 *Stream 变体、
// 传 nil 之后的实际行为与改造之前一致，不只是类型对得上。
func TestExistingRunRoundSignaturesStillCallableUnchanged(t *testing.T) {
	var (
		_ func(ctx context.Context, convID string, plan Plan, maxParallel int) []Record                                               = (*Runner)(nil).RunRound
		_ func(ctx context.Context, convID string, plan Plan, maxParallel int, snap RoundSnap) []Record                               = (*Runner)(nil).RunRoundPrepared
		_ func(ctx context.Context, convID string, plan Plan, maxParallel int, roundID string, out []Record, snap RoundSnap) []Record = (*Runner)(nil).RunRoundWith
	)

	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn

	// RunRound：原签名，不额外取快照。
	convA := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convA, id)
	}
	postUser(t, svc, convA, "ux1-sig-a")
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "UX1SIGA-"+id, "ok-"+id)), exitCode: 0})
	}
	recsA := r.RunRound(context.Background(), convA, freshPlanFiveSpeakers(), 0)
	if len(recsA) != len(ids) {
		t.Fatalf("RunRound 返回 %d 条 Record，期望 %d", len(recsA), len(ids))
	}
	for _, rec := range recsA {
		if rec.Outcome != OutcomeSpoke {
			t.Errorf("RunRound: agent %q 的 Outcome=%v，期望 OutcomeSpoke", rec.AgentID, rec.Outcome)
		}
	}

	// RunRoundPrepared：原签名，调用方先取 snap。
	convB := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convB, id)
	}
	postUser(t, svc, convB, "ux1-sig-b")
	snapB, err := r.TakeRoundSnap(context.Background(), convB)
	if err != nil {
		t.Fatalf("TakeRoundSnap(convB) 失败: %v", err)
	}
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "UX1SIGB-"+id, "ok-"+id)), exitCode: 0})
	}
	recsB := r.RunRoundPrepared(context.Background(), convB, freshPlanFiveSpeakers(), 0, snapB)
	if len(recsB) != len(ids) {
		t.Fatalf("RunRoundPrepared 返回 %d 条 Record，期望 %d", len(recsB), len(ids))
	}
	for _, rec := range recsB {
		if rec.Outcome != OutcomeSpoke {
			t.Errorf("RunRoundPrepared: agent %q 的 Outcome=%v，期望 OutcomeSpoke", rec.AgentID, rec.Outcome)
		}
	}

	// RunRoundWith：原签名，调用方自己铸 roundID、预填 out。
	convC := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convC, id)
	}
	postUser(t, svc, convC, "ux1-sig-c")
	snapC, err := r.TakeRoundSnap(context.Background(), convC)
	if err != nil {
		t.Fatalf("TakeRoundSnap(convC) 失败: %v", err)
	}
	roundID, err := newV4UUID()
	if err != nil {
		t.Fatalf("newV4UUID 失败: %v", err)
	}
	outC := make([]Record, len(ids))
	for i, id := range ids {
		outC[i] = Record{ConvID: convC, AgentID: id, RoundID: roundID, Outcome: OutcomeRefused}
	}
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "UX1SIGC-"+id, "ok-"+id)), exitCode: 0})
	}
	recsC := r.RunRoundWith(context.Background(), convC, freshPlanFiveSpeakers(), 0, roundID, outC, snapC)
	if len(recsC) != len(ids) {
		t.Fatalf("RunRoundWith 返回 %d 条 Record，期望 %d", len(recsC), len(ids))
	}
	for _, rec := range recsC {
		if rec.Outcome != OutcomeSpoke {
			t.Errorf("RunRoundWith: agent %q 的 Outcome=%v，期望 OutcomeSpoke", rec.AgentID, rec.Outcome)
		}
		if rec.RoundID != roundID {
			t.Errorf("RunRoundWith: agent %q 的 RoundID=%q，期望调用方传入的 %q", rec.AgentID, rec.RoundID, roundID)
		}
	}
}

// ---------------------------------------------------------------------------
// 验收 6：回调早于最终返回——全部 onDone 必须在函数返回之前跑完
// ---------------------------------------------------------------------------
//
// 不用时间戳比较（那种写法证明不了「异步、没等」这类坏形状：如果实现是
// go onDone(rec) 之后不 wait，时间戳仍然可能因为调度巧合排在返回之前）。
// 改用「返回时点数」：onDone 里先 sleep 一小段再把计数加一，如果实现是
// 异步触发、没有等它们全部完成就返回，RunRoundPreparedStream 返回的那一刻
// 大概率还没等到 sleep 结束，计数会明显小于成员数；若 onDone 严格在各自
// 的 goroutine 里同步执行、又被 wg.Wait() 等到，返回时计数必然已经等于
// 成员数——这是 sync.WaitGroup 的 happens-before 保证，不是运气。
func TestRunRoundPreparedStreamAllOnDoneCallsCompleteBeforeReturn(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "ux1-beforereturn-seed")

	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "UX1E-"+id, "ok-"+id)), exitCode: 0})
	}

	snap, err := r.TakeRoundSnap(context.Background(), convID)
	if err != nil {
		t.Fatalf("TakeRoundSnap 失败: %v", err)
	}

	const widen = 30 * time.Millisecond
	var count int32
	onDone := func(rec Record) {
		time.Sleep(widen)
		atomic.AddInt32(&count, 1)
	}

	recs := r.RunRoundPreparedStream(context.Background(), convID, freshPlanFiveSpeakers(), 0, snap, onDone)

	got := atomic.LoadInt32(&count)
	if got != int32(len(ids)) {
		t.Fatalf("RunRoundPreparedStream 返回时 onDone 只完成了 %d/%d 次——"+
			"说明有的 onDone 调用被丢进了一个没有等待的 goroutine，函数在它们跑完"+
			"之前就返回了，违反契约「回调早于最终返回」", got, len(ids))
	}
	if len(recs) != len(ids) {
		t.Fatalf("RunRoundPreparedStream 返回 %d 条 Record，期望 %d", len(recs), len(ids))
	}
}
