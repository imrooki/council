package turn

// 本文件是拆锁片（09 L 节 #17）的背对背验收测试：验的是「不同 agent 不再被
// 一把大锁串行化」「同一个 agent 仍然被串行化，且第二次真的接上第一次的
// provider 会话，不是各花一次钱」「gate 一经创建永不从 map 里删」「/session、
// Rebuild 不再被一轮在飞的 Ask 挡住 5 分钟」这些行为，不是实现细节。
//
// 背对背纪律：本文件只依赖任务书冻结的形状（sessionGate{mu, st}、
// Runner.gate(key)、Runner.gates/gatesMu、AskUpTo、SessionInfo.InFlight、
// ErrTurnInFlight、Runner.rulesMu）。这些符号在写这份文件的时刻都还不存在，
// 编译不过是预期状态——不为了让它过编译而去加这些类型或字段。
//
// 判定「两次调用是否真的并发过 / 真的被串行化」不靠 -race：本机
// CGO_ENABLED=0 没有 gcc，-race 用不了。靠的是自己记录每次 Spawner 调用
// 真实的进入/退出时刻，事后比较区间——这锁住的是行为，不是内存访问模式。

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/provider"
)

// ---------------------------------------------------------------------------
// 并发安全的 Spawner 替身
// ---------------------------------------------------------------------------
//
// 现成的 fakeSpawner（fixtures_test.go）用 f.calls = append(...) 且没有锁，
// 本文件的用例全是并发调用它的 spawn 方法，直接复用会有真实的并发写入
// 互相踩（不只是 -race 才能看见的理论竞争，是并发 append 真的可能互相
// 覆盖、丢记录）。任务书要求不改动 fakeSpawner——别的既有用例在用它——
// 所以这里另起一份，只给本文件与 askupto_test.go 用。

// timedSpawnCall 记下一次真实发生的 Spawner 调用：进入/退出的真实时刻，
// 以及送进去的 Command——用于验收 2「同一份规则 / 同一条会话」那类断言。
type timedSpawnCall struct {
	start time.Time
	end   time.Time
	cmd   provider.Command
}

// raceSpawner 按 agentID 分桶记录调用区间与可控睡眠时长，所有共享状态都
// 经 mu 保护。agentID 的识别见 agentIDFromCmd 的注释。
type raceSpawner struct {
	mu        sync.Mutex
	sleep     map[string]time.Duration
	queue     map[string][]spawnResult
	intervals map[string][]timedSpawnCall
}

func newRaceSpawner() *raceSpawner {
	return &raceSpawner{
		sleep:     make(map[string]time.Duration),
		queue:     make(map[string][]spawnResult),
		intervals: make(map[string][]timedSpawnCall),
	}
}

func (s *raceSpawner) setSleep(agentID string, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sleep[agentID] = d
}

func (s *raceSpawner) enqueue(agentID string, r spawnResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue[agentID] = append(s.queue[agentID], r)
}

// agentIDFromCmd 从 Command.Dir 反推是哪个 agent 发出的这次调用。
//
// newTestConfig 里三个测试 agent 全是 read 档，agentWorkDir 对 read 忽略
// convID，拼法是 filepath.Join(base, agentID)（session.go），于是 Dir 的
// 最后一段恰好是 agentID。这是本文件测试专用的白盒读法，不是
// provider.Command 的公开契约，只用于把并发记录的调用按 agent 分桶。
func agentIDFromCmd(cmd provider.Command) string {
	return filepath.Base(cmd.Dir)
}

func (s *raceSpawner) spawn(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
	agentID := agentIDFromCmd(c)

	s.mu.Lock()
	d := s.sleep[agentID]
	var res spawnResult
	haveRes := false
	if q := s.queue[agentID]; len(q) > 0 {
		res = q[0]
		s.queue[agentID] = q[1:]
		haveRes = true
	}
	s.mu.Unlock()

	if !haveRes {
		// 队列空了还被调用：某条用例的假设错了（多算了一次 spawn，或者
		// 忘记按 agent 分桶 enqueue）。立刻让测试看见失败，不要吞。
		return nil, nil, 0, false, errNoMoreCannedResponses
	}

	start := time.Now()
	if d > 0 {
		time.Sleep(d)
	}
	end := time.Now()

	s.mu.Lock()
	s.intervals[agentID] = append(s.intervals[agentID], timedSpawnCall{start: start, end: end, cmd: c})
	s.mu.Unlock()

	return res.stdout, res.stderr, res.exitCode, res.killed, res.err
}

// callsFor 交出某 agentID 记录到的全部调用，按真实起始时刻升序排好。
func (s *raceSpawner) callsFor(agentID string) []timedSpawnCall {
	s.mu.Lock()
	out := append([]timedSpawnCall(nil), s.intervals[agentID]...)
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].start.Before(out[j].start) })
	return out
}

// assertNoOverlap 断言某 agentID 记录到的全部调用区间两两不相交——
// 这是「同一个 agent 仍被串行化」与「gate 不被删」两条验收共用的核心判据。
func (s *raceSpawner) assertNoOverlap(t *testing.T, agentID string) {
	t.Helper()
	ivs := s.callsFor(agentID)
	for i := 1; i < len(ivs); i++ {
		if ivs[i].start.Before(ivs[i-1].end) {
			t.Fatalf("agent %q 的第 %d 次与第 %d 次 Spawner 调用区间相交："+
				"[%v, %v] 与 [%v, %v]——两次调用被并发跑了，串行化没有生效",
				agentID, i-1, i, ivs[i-1].start, ivs[i-1].end, ivs[i].start, ivs[i].end)
		}
	}
}

// resumeArg 在 argv 里找 --resume 后面跟着的那个值。三个测试 agent 只有
// claude 这一路会发 --resume（claude.go BuildTurn），够本文件的断言用。
func resumeArg(args []string) (id string, found bool) {
	for i, a := range args {
		if a == "--resume" && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// 最小可解析的成功样本（不是 fixtures_test.go 那批原件的复用）
// ---------------------------------------------------------------------------
//
// fixtures_test.go 的 claudeT1JSON / grokT1JSON 等是不逐字节改的探针原件，
// 本文件需要能自己控制 session_id（钉住「第二次是不是接上了第一次返回的
// 那个 id」），所以另写一份最小合法形状，不去改、也不去复用那批原件。

func minimalClaudeSuccess(sessionID, resultText string) string {
	// claudeSuccess（claude.go）是若干道独立判据的合取：type=="result"、
	// is_error==false、subtype=="success"、terminal_reason=="completed"、
	// api_error_status 缺席或 JSON null、stop_reason 在完成词里。
	// 漏掉 terminal_reason 会被判成失败样本——本函数第一版就漏了这项，
	// 靠 advisor 复核提醒去读 claude.go 242-271 行才补上，写进注释防止
	// 以后又漏。
	return fmt.Sprintf(
		`{"type":"result","is_error":false,"subtype":"success","stop_reason":"end_turn",`+
			`"terminal_reason":"completed","session_id":%q,"result":%q,"usage":{"input_tokens":1,`+
			`"cache_creation_input_tokens":1,"cache_read_input_tokens":0,`+
			`"output_tokens":1,"output_tokens_details":{"thinking_tokens":0}},"modelUsage":{}}`,
		sessionID, resultText,
	)
}

func minimalGrokSuccess(sessionID, text string) string {
	return fmt.Sprintf(
		`{"text":%q,"stopReason":"end_turn","sessionId":%q,`+
			`"usage":{"input_tokens":1,"cache_read_input_tokens":0,`+
			`"cache_creation_input_tokens":0,"output_tokens":1}}`,
		text, sessionID,
	)
}

// ---------------------------------------------------------------------------
// 验收 1：两个不同 agent 的 Ask 并发跑，不再被一把大锁串行化
// ---------------------------------------------------------------------------

func TestAskDifferentAgentsRunConcurrently(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	joinAgent(t, svc, convID, testAgentGrok)
	r := newTestRunner(t, cfg, svc)

	cs := newRaceSpawner()
	r.spawn = cs.spawn
	cs.setSleep(testAgentClaude, 200*time.Millisecond)
	cs.setSleep(testAgentGrok, 200*time.Millisecond)
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-CLAUDE", "hi")), exitCode: 0})
	cs.enqueue(testAgentGrok, spawnResult{stdout: []byte(minimalGrokSuccess("SESSION-GROK", "hi")), exitCode: 0})

	postUser(t, svc, convID, "task-for-both")

	var wg sync.WaitGroup
	var err1, err2 error
	wg.Add(2)
	start := time.Now()
	go func() {
		defer wg.Done()
		_, err1 = r.Ask(context.Background(), convID, testAgentClaude)
	}()
	go func() {
		defer wg.Done()
		_, err2 = r.Ask(context.Background(), convID, testAgentGrok)
	}()
	wg.Wait()
	elapsed := time.Since(start)

	if err1 != nil {
		t.Fatalf("claude 这一路 Ask 失败: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("grok 这一路 Ask 失败: %v", err2)
	}
	// 今天（拆锁之前）r.mu 在整轮 Ask 期间持有，两个 agent 也被串行化，
	// 总耗时必然 > 400ms。拆锁之后两者应各自睡 200ms、并发跑完，< 300ms。
	if elapsed >= 300*time.Millisecond {
		t.Fatalf("两个不同 agent 的 Ask 并发跑总耗时 %v，期望 < 300ms", elapsed)
	}
}

// ---------------------------------------------------------------------------
// 验收 2：同一个 agent 的两次 Ask 并发跑——串行化仍然成立，且第二次真的
// 接上第一次返回的会话 id，不是两次各花一次钱各开一条新会话
// ---------------------------------------------------------------------------

func TestAskSameAgentConcurrentCallsSerializeAndResumeSession(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)

	cs := newRaceSpawner()
	r.spawn = cs.spawn
	cs.setSleep(testAgentClaude, 200*time.Millisecond)
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-FIXED", "resp-1")), exitCode: 0})
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-FIXED", "resp-2")), exitCode: 0})

	// msg1 让「先跑到的那一轮」有 bootstrap 素材；msg2 在第一轮的 spawn
	// 睡眠期间才发出，确保它只会被「后跑到的那一轮」看见——若两轮都在
	// t=0 就读历史，bootstrap 那一轮的 selectBootstrap 会把两条消息一起
	// 吞掉，第二轮就没有 delta 可送，测不出「有没有接上会话」。
	postUser(t, svc, convID, "task-1")

	var wg sync.WaitGroup
	var err1, err2 error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err1 = r.Ask(context.Background(), convID, testAgentClaude)
	}()
	go func() {
		defer wg.Done()
		_, err2 = r.Ask(context.Background(), convID, testAgentClaude)
	}()

	time.Sleep(40 * time.Millisecond)
	postUser(t, svc, convID, "task-2")

	wg.Wait()
	if err1 != nil {
		t.Fatalf("第一个并发 Ask 失败: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("第二个并发 Ask 失败: %v", err2)
	}

	// 断言 A：两次真实 Spawner 调用的区间不相交——同一个 agent 仍被串行化。
	cs.assertNoOverlap(t, testAgentClaude)

	calls := cs.callsFor(testAgentClaude)
	if len(calls) != 2 {
		t.Fatalf("期望恰好两次 Spawner 调用（一次 bootstrap 一次 resume），got %d", len(calls))
	}
	first, second := calls[0], calls[1]

	// 断言 B（只验区间不相交是不够的那一条）：先跑到的那一轮必须是
	// bootstrap（没有 --resume），后跑到的那一轮必须带上 --resume，
	// 且值就是第一轮 canned 响应里的那个会话 id。
	// 若实现是「两次都各自 bootstrap」（gate 形同虚设），这两条至少有
	// 一条会红：先跑到的那次可能恰好也没有 --resume（正确），但后跑到的
	// 那次同样没有 --resume——因为它读到的仍是「还没建立会话」的旧状态。
	if _, found := resumeArg(first.cmd.Args); found {
		t.Fatalf("先跑到的那一轮 argv 里出现了 --resume，期望它是 bootstrap（没有会话可续）：%v", first.cmd.Args)
	}
	gotID, found := resumeArg(second.cmd.Args)
	if !found {
		t.Fatalf("后跑到的那一轮 argv 里没有 --resume，期望它接上第一轮建立的会话：%v\n"+
			"这正是任务书点名的假阳性陷阱：若两次都各自 bootstrap，区间仍然不相交，"+
			"但这里会露出马脚——第二次根本没有 --resume", second.cmd.Args)
	}
	if gotID != "SESSION-FIXED" {
		t.Fatalf("后跑到的那一轮 --resume 的值是 %q，期望等于第一轮 canned 响应里的会话 id %q", gotID, "SESSION-FIXED")
	}
}

// ---------------------------------------------------------------------------
// 验收 3：gate 一经创建永不从 map 里删——Rebuild 清的是 g.st，不是整个 gate
// ---------------------------------------------------------------------------

// TestGateIdentityStableAcrossSequentialRebuild 是最直接的钉法：一次
// Rebuild 前后，Runner.gate(key) 必须是同一个指针。
func TestGateIdentityStableAcrossSequentialRebuild(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)

	key := sessionKey{convID: convID, agentID: testAgentClaude}
	before := r.gate(key)
	if before == nil {
		t.Fatalf("r.gate(key) 返回了 nil")
	}
	if _, err := r.Rebuild(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("Rebuild 失败: %v", err)
	}
	after := r.gate(key)
	if before != after {
		t.Fatalf("Rebuild 前后 r.gate(key) 不是同一个指针：before=%p after=%p——"+
			"gate 被从 map 里删掉重建了，那条不变量不成立", before, after)
	}
}

// TestGateGetOrCreateIsStableUnderConcurrentRebuildRace 用 100 个并发
// goroutine 一边穿插 Rebuild 一边取 gate(key)，断言取到的永远是同一个指针。
// 这是「get-or-create」本身的竞态：若 Rebuild 把 gate 从 map 里删掉，
// 并发的两次 get-or-create 有概率各自新建出两个不同的 gate 对象，闸就
// 形同虚设——两条 goroutine 会拿着两把不同的锁，互相看不见对方。
func TestGateGetOrCreateIsStableUnderConcurrentRebuildRace(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)
	key := sessionKey{convID: convID, agentID: testAgentClaude}

	const n = 100
	ptrs := make([]*sessionGate, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			if i%10 == 0 {
				_, _ = r.Rebuild(context.Background(), convID, testAgentClaude)
			}
			ptrs[i] = r.gate(key)
		}()
	}
	wg.Wait()

	first := ptrs[0]
	for i, p := range ptrs {
		if p != first {
			t.Fatalf("第 %d 次 r.gate(key) 拿到了不同的指针：got %p want %p"+
				"（100 个并发 goroutine 里穿插了 10 次 Rebuild）", i, p, first)
		}
	}
}

// TestRebuildRacingWithConcurrentAsksNeverOverlapsSpawnerIntervals 是验收 3
// 给出的等价钉法：Rebuild 与两个并发 Ask 对同一个 (convID, agentID) 并发跑
// 100 轮，全部 Ask 触发的 Spawner 调用区间必须两两不相交——证明 Rebuild
// 在飞时不会让两条并发的 Ask 同时摸到同一份会话状态（gate 若被删，两条
// goroutine 会各自 get-or-create 出两个不同的 gate，串行化就没了）。
//
// 判别力为什么必须是「同一轮两个并发 Ask」，而不能只是「一个 Ask 一个
// Rebuild」：Rebuild 本身不起进程，只丢会话状态；若每轮只有一个 Ask，
// 「本轮至多一个 Spawner 区间」是循环结构（每轮末尾的 per-iteration
// wg.Wait()）单独就能保证的，跟闸有没有生效无关——闸失效时这条用例
// 照样绿，之前的版本就是这样一个恒真断言（已用变异证实：把
// Runner.gate 改成每次都新建，仍然通过）。把「同一时刻只有一个 Ask」
// 这个原本由循环结构保证的前提，改成由闸来保证，才是这条用例真正在
// 测的东西：闸正常时两个 Ask 被串行化，一个吃两条消息、另一个大概率
// 撞 ErrNoDelta，本轮至多一个区间；闸失效时两个 Ask 各拿到自己的
// gate，可能真的同时进 Spawner，本轮出现两个重叠区间，assertNoOverlap
// 转红。
//
// 循环每轮末尾有一个 per-iteration 的 wg.Wait()（见下方就地注释）：它挡的是
// 「轮与轮之间」的堆叠，不是「轮内 Ask 与 Rebuild 的并发」——后者仍然是
// 每轮三个并发 goroutine（两个 Ask + 一个 Rebuild），一字不差地对应本条
// 验收要的「并发跑 100 次」。
func TestRebuildRacingWithConcurrentAsksNeverOverlapsSpawnerIntervals(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)

	cs := newRaceSpawner()
	r.spawn = cs.spawn
	cs.setSleep(testAgentClaude, 5*time.Millisecond)

	const iterations = 100
	var wg sync.WaitGroup
	for i := 0; i < iterations; i++ {
		i := i
		// 两条消息对应两个并发 Ask：闸正常时，谁先抢到闸就把两条一起
		// 吞掉（router.Select 的 bootstrap / 增量两条路径都是一次性
		// 吃光调用方交来的全量窗口，见 router.go Select/selectBootstrap
		// 的注释），另一个大概率撞 ErrNoDelta，本轮只留下一个 Spawner
		// 区间。
		postUser(t, svc, convID, fmt.Sprintf("race-msg-%03d-a", i))
		postUser(t, svc, convID, fmt.Sprintf("race-msg-%03d-b", i))

		// 每轮 enqueue 两条，不是一条：查过 raceSpawner.spawn（本文件
		// 上方）——队列（按 agentID 分桶）一旦耗尽，spawn 直接返回
		// errNoMoreCannedResponses，且在记录 intervals 之前就 return，
		// 不会留下任何调用区间。闸正常时通常只有一次真正的 Spawner
		// 调用会消费掉一条，另一条结转到后面的轮次（本用例不比对某一
		// 轮具体的 session id，结转无害）；但闸一旦失效，两个 Ask 可能
		// 真的同时各自摸到 spawn，这时必须两条都能取到真实结果——
		// 否则后到的那个会因为队列空而在记录区间之前就返回，
		// assertNoOverlap 照样测不出重叠，判别力又在这一步被吃掉一次。
		cs.enqueue(testAgentClaude, spawnResult{
			stdout:   []byte(minimalClaudeSuccess(fmt.Sprintf("sess-%03d-a", i), "ok-a")),
			exitCode: 0,
		})
		cs.enqueue(testAgentClaude, spawnResult{
			stdout:   []byte(minimalClaudeSuccess(fmt.Sprintf("sess-%03d-b", i), "ok-b")),
			exitCode: 0,
		})

		wg.Add(3)
		go func() {
			defer wg.Done()
			_, _ = r.Ask(context.Background(), convID, testAgentClaude)
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Ask(context.Background(), convID, testAgentClaude)
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Rebuild(context.Background(), convID, testAgentClaude)
		}()

		// per-iteration 的 wg.Wait()：等这一轮的两个 Ask 与 Rebuild 都
		// 收工，再进下一轮。这三个 goroutine 本身仍然是并发起的、彼此
		// 之间没有任何同步——挡的只是「轮与轮之间」的堆叠，不是轮内的
		// 竞争，contract 12 §2.8 验收 3 要的「Rebuild 与 Ask 并发跑 100
		// 次」照样一字不差地成立。
		//
		// 不加这一行会退化成什么（这条机制在旧的「1 条消息 / 1 个 Ask」
		// 版本上实测过，不是推测）：大量消息、大量 goroutine 几乎在同一
		// 瞬间全部 post/起跑，一起抢同一把闸。第一个抢到闸的 Ask 会
		// 一次性扫完当时已经 post 的全部消息、把游标提交到最大 seq；
		// 此后绝大多数 Ask 撞 ErrNoDelta，根本走不到 Spawner。只有当
		// 某次 Rebuild 恰好抢到 TryLock（把 provider 会话连游标一起
		// 丢掉）之后，下一个 Ask 才会重新看到 delta、重新起一次
		// spawn——而 Rebuild 用的是 TryLock，在 Ask 持闸的 5ms 窗口里
		// 大概率直接弹回 ErrTurnInFlight。旧版本（100 条消息、每轮 1 个
		// Ask）净结果是整整 100 轮里只有 3 次真的触发了 Spawner 调用，
		// 下面的地板值断言会对着几乎空的样本空心通过；加上这一行之后
		// 变成 100（同样是实测数字）。本版本（每轮 2 条消息、2 个 Ask）
		// 同样删掉这一行复测过（31 次独立运行）：塌陷到 5～6 之间，
		// 从未低于 5、也没有到过两位数——机制不变（抢占—吞并—
		// ErrNoDelta—TryLock 弹回这条链路与消息/Ask 数目无关），
		// 结论同样适用：删掉这一行样本量会重新塌陷，只是塌陷到的
		// 量级比旧版本（3）高，原因是每轮多一个 Ask、多一条消息，
		// 裸赛道上「谁先抢到闸就吞光当时已 post 的一切」这条机制
		// 恰好多留出一点点缝隙让极少数几轮能各自触发一次 spawn。
		// 这直接决定了下面地板值不能再沿用旧版本的 5：5～6 与 5 太
		// 接近，旧地板在这个版本上测不出这条退化，细节见下面那段注释。
		wg.Wait()
	}
	wg.Wait()

	// 地板值：上面的 per-iteration wg.Wait() 已经把「消息被前一轮吞掉、
	// Ask 撞 ErrNoDelta、只有 Rebuild 抢赢 TryLock 才重新有 delta」这条
	// 退化路径挡掉了（本机 6 次连续跑均恰好 100）。这里的下限不是为了
	// 迁就退化路径而放低的地板，是留着当回归哨兵：谁以后把上面那行
	// per-iteration wg.Wait() 当成多余的同步点删掉，样本量会重新塌陷，
	// 测试在这里就先炸给出明确原因，而不是让 assertNoOverlap 对着一个
	// 几乎空的切片空心通过。
	//
	// 这条下限现在还担着第二份责任：本用例的判别力不再只靠「Rebuild
	// 在飞时 Ask 摸到的会话状态」，还指望每轮两个并发 Ask 真的都跑到
	// r.Ask 里面去争同一把闸——若两个 Ask 里有一个从未真正起跑（比如
	// 调度器把两次调用都排在了同一个 goroutine 之后很久才跑，或者
	// wg.Add 记错了数目），本用例会悄悄退化回「事实上只有一个 Ask」
	// 的旧版本，恒真问题原样复现而不会有任何报错——下限就是防这种
	// 退化的哨兵，不只是防「barrier 被删」这一种退化。
	//
	// 门槛定为 20，不是沿用旧版本（1 条消息/1 个 Ask）的 5：删掉上面
	// 那行 per-iteration wg.Wait() 复测过 31 次独立运行，本版本
	// （2 条消息/2 个 Ask）塌陷到的样本量落在 5～6 之间、从未低于 5——
	// 旧的 5 这道槛在本版本上测不出「barrier 被删」这条退化（5 或 6
	// 都满足 >= 5，Fatalf 根本不会触发，会直接滑进 assertNoOverlap
	// 拿一份小样本空心通过，回归哨兵形同虚设）。20 同时满足两头：
	// 比实测塌陷区间（5～6）的上沿还高出一倍以上，对着退化路径能可靠
	// 触发；比正常路径实测值（100，6 次独立运行全部恰好 100）低了 5
	// 倍，留够余量不会把正常波动误判成退化。
	calls := cs.callsFor(testAgentClaude)
	t.Logf("100 轮 Rebuild / 两个并发 Ask 竞速里，真正触发 Spawner 调用的次数 = %d", len(calls))
	if len(calls) < 20 {
		t.Fatalf("100 轮竞速里只有 %d 次真正触发了 Spawner 调用，"+
			"样本太少，assertNoOverlap 会空心通过——检查是不是消息全被前一轮吞掉了，"+
			"或者两个并发 Ask 里有一个根本没跑到", len(calls))
	}
	cs.assertNoOverlap(t, testAgentClaude)
}

// ---------------------------------------------------------------------------
// 验收 5：一轮在飞时调 SessionInfo，50ms 内返回且 InFlight == true
// ---------------------------------------------------------------------------

func TestSessionInfoRespondsQuicklyWhileRoundInFlight(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)

	cs := newRaceSpawner()
	r.spawn = cs.spawn

	// 先跑通一轮（不睡眠），把会话建立起来——避免把「bootstrap 在飞时
	// 还没有任何已确立的会话」这种边界状态也卷进这条验收，那不是它要问的问题。
	postUser(t, svc, convID, "seed-msg")
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-A", "resp-1")), exitCode: 0})
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("建立首轮会话失败: %v", err)
	}

	postUser(t, svc, convID, "second-msg")
	cs.setSleep(testAgentClaude, 300*time.Millisecond)
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-A", "resp-2")), exitCode: 0})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = r.Ask(context.Background(), convID, testAgentClaude)
	}()

	time.Sleep(60 * time.Millisecond) // 让第二轮先进 spawn 的睡眠区间，确保「在飞」
	probeStart := time.Now()
	info, ok := r.SessionInfo(context.Background(), convID, testAgentClaude)
	probeElapsed := time.Since(probeStart)

	// 今天 SessionInfo 与 Ask 抢同一把 r.mu，会被堵到那一轮结束
	// （最长 defaultAskTimeout），这里用 300ms 的睡眠即可稳定复现「远超 50ms」。
	if probeElapsed >= 50*time.Millisecond {
		t.Fatalf("一轮在飞时 SessionInfo 耗时 %v，期望 < 50ms", probeElapsed)
	}
	if !ok {
		t.Fatalf("SessionInfo 返回 ok=false，期望能看到这条已确立、此刻正在飞行中的会话")
	}
	if !info.InFlight {
		t.Fatalf("info.InFlight = false，期望 true——这一轮此刻正在跑")
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// 验收 6：一轮在飞时调 Rebuild，返回 ErrTurnInFlight，且库里那一行原值还在
// ---------------------------------------------------------------------------

func TestRebuildDuringInFlightRoundIsRejectedAndRowUnchanged(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)

	cs := newRaceSpawner()
	r.spawn = cs.spawn

	postUser(t, svc, convID, "seed-msg")
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-A", "resp-1")), exitCode: 0})
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("建立首轮会话失败: %v", err)
	}
	before, found, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !found {
		t.Fatalf("首轮之后读不到 provider_sessions 行: found=%v err=%v", found, err)
	}

	postUser(t, svc, convID, "second-msg")
	cs.setSleep(testAgentClaude, 300*time.Millisecond)
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-A", "resp-2")), exitCode: 0})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = r.Ask(context.Background(), convID, testAgentClaude)
	}()

	time.Sleep(60 * time.Millisecond) // 确保第二轮已经进了 spawn 的睡眠区间
	_, rebuildErr := r.Rebuild(context.Background(), convID, testAgentClaude)
	if !errors.Is(rebuildErr, ErrTurnInFlight) {
		t.Fatalf("一轮在飞时调 Rebuild，期望 errors.Is(err, ErrTurnInFlight)，got %v", rebuildErr)
	}

	// 判据是读回来的那一行，不是屏幕输出：此刻第二轮还在睡眠里，
	// 还没有提交任何东西，立刻读库必须仍是首轮提交的原值。
	mid, found, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !found {
		t.Fatalf("Rebuild 被拒之后读不到那一行: found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(mid, before) {
		t.Fatalf("Rebuild 被拒之后那一行变了：\n  before=%+v\n  mid   =%+v", before, mid)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// 验收 7：一轮在飞时调 SetRules，这一轮的 identity_fp 与真正送进 argv 的
// 身份块必须来自同一份规则文本，且下一轮才用上新规则
// ---------------------------------------------------------------------------
//
// 报告里会点名这条的判据缺口：Ask 在起 spawn 之前就已经把 renderIdentity
// 与 identityFPFor 都算完了（runner.go 234-244 行），本文件能稳定命中的
// 只是「SetRules 发生在 spawn 的睡眠期间」这个窗口，而不是「renderIdentity
// 与 identityFPFor 各自读 r.rules 那两行之间」的纳秒级窗口——后者黑盒测试
// 基本没法可靠命中。这里验的是弱一档但仍然有意义的契约：只要这一轮在
// SetRules 生效之前已经读过一次规则，它此后全程都不应该再被新规则污染，
// 且下一轮（配合 Rebuild，模拟 /rules reload --rebuild 的真实调用序列）
// 应该用新规则。

func TestSetRulesDuringInFlightRoundDoesNotLeakIntoThatRound(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)
	r.rules = RulesBlocks{Global: "OLD-RULES-MARKER"}

	cs := newRaceSpawner()
	r.spawn = cs.spawn
	cs.setSleep(testAgentClaude, 150*time.Millisecond)
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-1", "resp-1")), exitCode: 0})

	postUser(t, svc, convID, "task-1")

	ac := cfg.Agents[testAgentClaude]
	p, err := provider.Lookup(ac.Provider)
	if err != nil {
		t.Fatalf("provider.Lookup(%q) 失败: %v", ac.Provider, err)
	}
	tier, err := TranslatePermission(ac.Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission 失败: %v", err)
	}

	var wg sync.WaitGroup
	var rec1 Record
	var err1 error
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec1, err1 = r.Ask(context.Background(), convID, testAgentClaude)
	}()

	time.Sleep(30 * time.Millisecond) // 越过身份/指纹计算，此刻应已进了 spawn 的睡眠
	r.SetRules(RulesBlocks{Global: "NEW-RULES-MARKER"})
	wg.Wait()

	if err1 != nil {
		t.Fatalf("round1 Ask 失败: %v", err1)
	}
	_ = rec1

	calls := cs.callsFor(testAgentClaude)
	if len(calls) != 1 {
		t.Fatalf("期望 round1 恰好一次 Spawner 调用，got %d", len(calls))
	}
	rolePath := filepath.Join(agentWorkDir(r.workDir, convID, testAgentClaude, tier), "council-role.txt")
	body1, ok := calls[0].cmd.Files[rolePath]
	if !ok {
		t.Fatalf("round1 的 Command.Files 里没有身份文件 %q，Files=%v", rolePath, calls[0].cmd.Files)
	}
	if !strings.Contains(string(body1), "OLD-RULES-MARKER") {
		t.Fatalf("round1 送进 argv 的身份块里没有旧规则标记，got: %q", string(body1))
	}
	if strings.Contains(string(body1), "NEW-RULES-MARKER") {
		t.Fatalf("round1 送进 argv 的身份块里混进了新规则标记，got: %q", string(body1))
	}

	// 这一轮落进内存的 identityFP 必须与「旧规则」独立算出来的指纹一致——
	// 钉的是「fp 与真正发出去的身份块来自同一份规则文本」，不是各用各的。
	wantFPOld, err := identityFPFor(p, testAgentClaude, ac, r.workDir, convID, tier, RulesBlocks{Global: "OLD-RULES-MARKER"})
	if err != nil {
		t.Fatalf("独立计算旧规则指纹失败: %v", err)
	}
	g := r.gate(sessionKey{convID: convID, agentID: testAgentClaude})
	g.mu.Lock()
	gotFP := ""
	if g.st != nil {
		gotFP = g.st.identityFP
	}
	g.mu.Unlock()
	if gotFP != wantFPOld {
		t.Fatalf("round1 记下的 identityFP=%q，与旧规则独立算出的指纹=%q 不符"+
			"——fp 与身份块用了不同的规则快照", gotFP, wantFPOld)
	}

	// 下一轮：先 Rebuild（模拟 /rules reload --rebuild 的真实调用序列：
	// 那个命令换完规则会连带重建全部会话，不是只调 SetRules），再 Ask，
	// 应该用上新规则。
	if _, err := r.Rebuild(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("Rebuild 失败: %v", err)
	}
	postUser(t, svc, convID, "task-2")
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-2", "resp-2")), exitCode: 0})
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("round2 Ask 失败: %v", err)
	}

	calls2 := cs.callsFor(testAgentClaude)
	if len(calls2) != 2 {
		t.Fatalf("期望累计两次 Spawner 调用，got %d", len(calls2))
	}
	body2, ok := calls2[1].cmd.Files[rolePath]
	if !ok {
		t.Fatalf("round2 的 Command.Files 里没有身份文件 %q", rolePath)
	}
	if !strings.Contains(string(body2), "NEW-RULES-MARKER") {
		t.Fatalf("round2 没有用上新规则，got: %q", string(body2))
	}
}
