package turn

// 未决 #25 的背对背验收测试：写这份文件时（#25 落地前）r.cfg 没有拆锁
// （片 1 只拆了 rules 那一半），结构上已经允许并发 Ask 之后，SetConfig
// 与在飞的 Ask 之间没有任何互斥——下面用例 1/2/3 当时都是靠这个缺口
// 稳定复现红的。#25 落地后 cfgMu 已经补上（见各用例自己的文档注释），
// 另外三条转为回归哨兵，用例 4（本来就没有可利用的缺口）从写下第一天
// 起就是绿的。本文件的用例与断言原样保留，不删、不改判据。
//
// 冻结任务书给的做法是「假 Spawner 睡一段时间造出在飞，SetConfig 在它
// 睡着时换配置」——与 gate_concurrency_test.go 里 SetRules 那条用例同一个
// 模子。本文件第一条用例动手之后发现这个模子在这里不成立，原因写在
// nowBarrier 的注释里：写这份文件时（#25 落地前）runner.go 的 AskUpTo
// 里全部三处是裸读 r.cfg（#25 落地后已经改成读入场时一次性快照下来
// 的本地变量 cfg，不再直接读 r.cfg——按符号定位，不钉行号，理由见
// 本段末尾那句）——
// 「ac, ok := r.cfg.Agents[agentID]」、thoughtVisibleFor 里读 r.cfg.Teams、
// 「cmd.Path = r.cfg.Providers[ac.Provider].Command」——都发生在
// 「r.spawn(spawnCtx, cmd)」被调用之前，sleep 能覆盖的只是 spawn 调用之后
// 的时间，那时这三处早就读完、结果已经拷进本地变量，SetConfig 再晚也
// 够不着。本文件改用 r.now——askUpToWith 里那句
// 「rec.ColdExpected = coldExpectedAt(r.now(), ...)」是唯一一处正好夹在
// 「已读 r.cfg.Agents」与「还没读 r.cfg.Providers / r.cfg.Teams」之间、
// 且本来就是测试可替换字段（NewRunner 的默认值是 time.Now，
// fixtures_test.go 的 fakeClock 已经在替换同一个字段）的调用点——当同步
// 挂钩。这不是走后门，只是把「控制时钟读数」的既有口子挪用成「控制这一
// 时刻」。（本文件写作时读到的具体行号：283/402/445/511/528——这些数字
// 只是写作当时的定位辅助，会随实现变动漂移，不作为契约引用；下面用得着
// 的地方一律用符号名指代，不钉行号。）
//
// 判定「同一份配置」不靠 -race（本机没有 gcc，CGO_ENABLED=0 用不了）：
// 靠的是让 cfg1 / cfg2 在两个不同字段上分别留痕——ac.Model（直接进
// IdentityFingerprint，也进 argv 的 --model）与 Providers[...].Command
// （只决定「cmd.Path = r.cfg.Providers[...]」那处回填的 cmd.Path，不进
// 指纹）——再比对 Spawner 真实收到的 Command 与落库的 identity_fp，
// 分别来自哪一份。
//
// 脆弱点留给下一个人：nowBarrier(1)/nowBarrier(2) 靠的是「AskUpTo 成功
// 路径上第一次调用 r.now() 正好是 coldExpectedAt 那一句、正好只被调一次」
// 这个此刻为真的事实。若未来有人把 coldExpectedAt 挪了位置、或者在它
// 之前新插一次 r.now() 调用，本文件的用例会在错误的时刻放行 SetConfig
// （可能提前，也可能 ac 已经读完之后才放，届时用例会从「红」变成看似
// 无意义的「绿」而不报任何编译错误或明显的运行时错误）——排查时先看
// 这一条，而不是先怀疑 cfgMu 的实现。

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/provider"
)

// ---------------------------------------------------------------------------
// nowBarrier：把 Runner.now 变成一个可控的同步点
// ---------------------------------------------------------------------------

// nowBarrier 的前 want 次调用会阻塞在 <-release，直到调用方看到 arrivedCh
// 已关闭（说明 want 次调用都已经到达）、做完它要做的事、再关闭 release。
// 第 want+1 次及以后的调用（同一轮后续还会用到 r.now 的地方，例如
// commitSession 里的 lastTurnAt、makeTurnUsage 的 createdAt、下一轮的
// AskUpTo）看到 release 已关闭，直接穿过，不再阻塞——channel 关闭之后
// 全部接收方永远立刻返回，不需要额外的一次性开关。
type nowBarrier struct {
	mu        sync.Mutex
	arrived   int
	want      int
	fired     bool
	arrivedCh chan struct{}
	release   chan struct{}
}

func newNowBarrier(want int) *nowBarrier {
	return &nowBarrier{want: want, arrivedCh: make(chan struct{}), release: make(chan struct{})}
}

func (g *nowBarrier) now() time.Time {
	g.mu.Lock()
	g.arrived++
	if g.arrived >= g.want && !g.fired {
		g.fired = true
		close(g.arrivedCh)
	}
	g.mu.Unlock()
	<-g.release
	return time.Now()
}

// waitArrived 等 want 次调用都到达，超时则先放行（避免卡住的 goroutine
// 永久泄漏）再让用例失败。
func (g *nowBarrier) waitArrived(t *testing.T, timeout time.Duration, msg string) {
	t.Helper()
	select {
	case <-g.arrivedCh:
	case <-time.After(timeout):
		close(g.release)
		t.Fatal(msg)
	}
}

// setConfigWithTimeout 在独立 goroutine 里调 SetConfig，带超时兜底。
//
// 这不是多余的防御：若实现给 SetConfig 加了一把需要整轮持有的锁（例如
// 参照 rulesMu 的写法但作用域理解错了，变成「Ask 从头到尾持读锁，
// SetConfig 等整轮结束才能拿写锁」），这里会先于本用例其余断言暴露——
// SetConfig 被一轮在飞的 Ask 挡住，而任务书要求的是「快照粒度是轮，
// 不是持锁整轮」，挡住本身就是不该发生的行为。
func setConfigWithTimeout(t *testing.T, r *Runner, cfg *config.Config, timeout time.Duration, onTimeout func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		r.SetConfig(cfg)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		if onTimeout != nil {
			onTimeout()
		}
		t.Fatalf("SetConfig 在 %v 内没有返回，疑似被一轮在飞的 Ask 挡住了", timeout)
	}
}

// argFlagValue 在 argv 里找 flag 后面紧跟的那个值，与 gate_concurrency_test.go
// 的 resumeArg 同体例（那个只认 --resume），这里换成任意 flag 作参数，
// 不在已经判给「不许碰」的文件里重复定义同名函数。
func argFlagValue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// 用例 1：一轮在飞时 SetConfig，那一轮不许串味（本片的要害）
// ---------------------------------------------------------------------------

// TestSetConfigDuringInFlightRoundDoesNotLeakIntoThatRound 钉单个 agent 的
// 单轮：cmd.Path（AskUpTo 里「cmd.Path = r.cfg.Providers[ac.Provider].Command」
// 那处单独裸读 r.cfg.Providers，claude.go 的 BuildTurn 从不自己填 Path，
// 见该文件的 Command{} 字面量只给 Args/Dir/Env/Stdin/Files/Transport/
// Dropped，Path 留空——AskUpTo 里这处回填是它唯一的来源）应当仍与这一轮
// 早先算出的 identity_fp、argv 里的 --model（两者都来自
// 「ac, ok := r.cfg.Agents[agentID]」读到、此后只用本地变量 ac 的同一次读）
// 出自同一份配置。
//
// 写这条用例时（#25 落地前）没有 cfgMu：nowBarrier 把 SetConfig 精确
// 插进「ac 已读」与「cmd.Path 那处回填未读」之间，后者会读到 SetConfig
// 换上去的新配置，与已经用旧配置算好的 identity_fp/argv 对不上——这就
// 是本用例断言要抓的「串味」，当时稳定红。#25 落地后 cfgMu 补上：
// AskUpTo 一进场对 r.cfg 取一次 cfgMu 读锁、拷进本地变量 cfg 就放锁
// （runner.go），下游 ac, ok := cfg.Agents[agentID] 与
// cmd.Path = cfg.Providers[ac.Provider].Command 读的都是这份快照，不
// 再直接读 r.cfg——nowBarrier 卡的这个窗口里 SetConfig 再也够不着。
// 本用例现在是回归哨兵，断言原样保留：谁把这份快照拆散、改回逐处
// 裸读 r.cfg，这里会先红。
func TestSetConfigDuringInFlightRoundDoesNotLeakIntoThatRound(t *testing.T) {
	cfg1 := newTestConfig()
	cfg2 := newTestConfig()
	ac2 := cfg2.Agents[testAgentClaude]
	ac2.Model = "claude-fable-5-1-NEW" // 进指纹（IdentityFingerprint 直接吃 ac.Model），也进 argv 的 --model
	cfg2.Agents[testAgentClaude] = ac2
	cfg2.Providers["claude"] = config.ProviderConfig{Command: "claude-fake-NEW"} // 只影响 cmd.Path 的回填，不进指纹

	svc, dbPath := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "task-1")

	r := newTestRunner(t, cfg1, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	gate := newNowBarrier(1)
	r.now = gate.now

	var wg sync.WaitGroup
	var rec1 Record
	var err1 error
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec1, err1 = r.Ask(context.Background(), convID, testAgentClaude)
	}()

	gate.waitArrived(t, 5*time.Second,
		"round1 没有在 5s 内到达 r.now 同步点（AskUpTo 里 coldExpectedAt(r.now(), ...) 那一句），前置条件不成立，下面的断言无意义")

	setConfigWithTimeout(t, r, cfg2, 2*time.Second, func() { close(gate.release) })
	close(gate.release)
	wg.Wait()

	if err1 != nil {
		t.Fatalf("round1 Ask 失败: %v", err1)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("期望 round1 恰好一次 Spawner 调用，got %d", len(fs.calls))
	}

	// 断言 A（写这条用例时、#25 落地前曾经稳定红；#25 落地后是回归
	// 哨兵）：cmd.Path 期望仍是 cfg1 的，与这一轮其余部分（identity_fp、
	// argv 的 --model）保持同一份配置。
	gotPath := fs.calls[0].cmd.Path
	if gotPath != cfg1.Providers["claude"].Command {
		t.Errorf("round1 的 cmd.Path = %q，期望仍是旧配置 %q——"+
			"round1 早先已经用 cfg1 算出了 identity_fp 与 argv 里的 --model，"+
			"此刻 cmd.Path 却已经看到 SetConfig 换上去的 cfg2："+
			"同一轮里两处用了两份配置，这正是未决 #25 补掉的那个缺口；"+
			"这条用例现在是回归哨兵，它红说明那份轮内快照被拆散了",
			gotPath, cfg1.Providers["claude"].Command)
	}

	// 断言 B（衬托断言 A 的对照组，预期今天就应该成立）：argv 里的
	// --model 与 identity_fp 一样，来自同一次「ac, ok := r.cfg.Agents[agentID]」
	// 读到的 ac，理论上不可能因为 SetConfig 的时机而撕裂——这条如果也红，
	// 说明前提假设错了。
	gotModel, found := argFlagValue(fs.calls[0].cmd.Args, "--model")
	if !found {
		t.Fatalf("round1 argv 里没有 --model，argv=%v", fs.calls[0].cmd.Args)
	}
	if gotModel != cfg1.Agents[testAgentClaude].Model {
		t.Errorf("round1 argv 里的 --model = %q，期望 cfg1 的 %q（对照组也红了，说明"+
			"ac 并非只读一次，前提假设需要重新核对）", gotModel, cfg1.Agents[testAgentClaude].Model)
	}

	p, err := provider.Lookup(cfg1.Agents[testAgentClaude].Provider)
	if err != nil {
		t.Fatalf("provider.Lookup 失败: %v", err)
	}
	tier, err := TranslatePermission(cfg1.Agents[testAgentClaude].Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission 失败: %v", err)
	}
	wantFP, err := identityFPFor(p, testAgentClaude, cfg1.Agents[testAgentClaude], r.workDir, convID, tier, RulesBlocks{})
	if err != nil {
		t.Fatalf("独立计算 cfg1 指纹失败: %v", err)
	}
	if rec1.IdentityFP != wantFP {
		t.Errorf("rec1.IdentityFP = %q，期望等于独立用 cfg1 算出的指纹 %q", rec1.IdentityFP, wantFP)
	}
	_, dbFP := tstTurnUsageRoundAndIdentity(t, dbPath, rec1.TurnID)
	if dbFP != wantFP {
		t.Errorf("turn_usage.identity_fp = %q，期望等于独立用 cfg1 算出的指纹 %q", dbFP, wantFP)
	}

	// 下一轮：Rebuild（模拟 /config reload --apply 真实的调用序列，与
	// gate_concurrency_test.go 的 SetRules 用例同一套体例）再 Ask，
	// 这次没有并发的 SetConfig 插在中间，cmd.Path / argv / identity_fp
	// 三者应该一致地是 cfg2。
	if _, err := r.Rebuild(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("Rebuild 失败: %v", err)
	}
	postUser(t, svc, convID, "task-2")
	fs.enqueueSuccess(claudeT1JSON)
	rec2, err2 := r.Ask(context.Background(), convID, testAgentClaude)
	if err2 != nil {
		t.Fatalf("round2 Ask 失败: %v", err2)
	}
	if len(fs.calls) != 2 {
		t.Fatalf("期望累计两次 Spawner 调用，got %d", len(fs.calls))
	}
	gotPath2 := fs.calls[1].cmd.Path
	if gotPath2 != cfg2.Providers["claude"].Command {
		t.Errorf("round2 的 cmd.Path = %q，期望是新配置 %q", gotPath2, cfg2.Providers["claude"].Command)
	}
	gotModel2, found2 := argFlagValue(fs.calls[1].cmd.Args, "--model")
	if !found2 || gotModel2 != cfg2.Agents[testAgentClaude].Model {
		t.Errorf("round2 argv 里的 --model = %q(found=%v)，期望新配置的 %q", gotModel2, found2, cfg2.Agents[testAgentClaude].Model)
	}
	p2, err := provider.Lookup(cfg2.Agents[testAgentClaude].Provider)
	if err != nil {
		t.Fatalf("provider.Lookup 失败: %v", err)
	}
	wantFP2, err := identityFPFor(p2, testAgentClaude, cfg2.Agents[testAgentClaude], r.workDir, convID, tier, RulesBlocks{})
	if err != nil {
		t.Fatalf("独立计算 cfg2 指纹失败: %v", err)
	}
	if rec2.IdentityFP != wantFP2 {
		t.Errorf("rec2.IdentityFP = %q，期望等于独立用 cfg2 算出的指纹 %q", rec2.IdentityFP, wantFP2)
	}
}

// ---------------------------------------------------------------------------
// 用例 2：两个不同 agent 并发 Ask，中途 SetConfig
// ---------------------------------------------------------------------------

// TestConcurrentDifferentAgentsSetConfigMidFlightEachRoundStaysInternallyConsistent
// 与上一条的区别：上一条钉的是单个 agent「一轮内部」的裂缝，用
// nowBarrier(1) 逼出。这一条验的是同一个缺口在两个不同 sessionKey
// （不同 agent，片 1 之后真的并发）上是否各自独立复现——即「片 1 按
// key 拆的闸，没有意外地把这条缺口串行化 / 掩盖掉」。两条并发的 goroutine
// 各自有自己的局部变量 ac，彼此之间没有共享可变状态（除了都在读同一个
// r.cfg 指针），所以本质上是「用例 1 × 2、真并发」，不是一种新的裂缝
// 形状——老实说清楚，不夸大。本用例只对每一轮各自的 cfg1 负责，从未
// 比较过 recA 与 recB 谁的值等于谁，也不要求两轮谁用旧谁用新（它们本来
// 就可能各自落在换配置的两侧；本用例用 nowBarrier(2) 把两轮都摆在换配置
// 之后，只是为了同时验证两个 key 各自独立复现同一个缺口，不代表「两轮
// 必须一致」是这里的断言）。
//
// 写这条用例时（#25 落地前）确实红过，两个 agent 各自独立复现了同一个
// 缺口：claude 这一轮 cmd.Path = "claude-fake-NEW"，期望仍是旧配置
// "claude-fake"；grok 这一轮 cmd.Path = "grok-fake-NEW"，期望仍是旧配置
// "grok-fake"——两个 key 各自独立复现同一个缺口，本质是「用例 1 × 2、
// 真并发」，证明片 1 按 key 拆的闸没有意外把这条缺口串行化掉，没有
// 夸大成「两个 key 互相踩」。#25 落地后 cfgMu 补上（AskUpTo 一进场对
// r.cfg 取一次读锁、拷进本地变量 cfg，两条 goroutine 各自拿到的都是
// 整份快照），本用例转绿，现在是回归哨兵：它当初真的在两个 key 上
// 各自独立红过，这一点本身就是它的判别力所在。
func TestConcurrentDifferentAgentsSetConfigMidFlightEachRoundStaysInternallyConsistent(t *testing.T) {
	cfg1 := newTestConfig()
	cfg2 := newTestConfig()
	acA := cfg2.Agents[testAgentClaude]
	acA.Model = "claude-fable-5-1-NEW"
	cfg2.Agents[testAgentClaude] = acA
	acB := cfg2.Agents[testAgentGrok]
	acB.Model = "grok-4.6-NEW"
	cfg2.Agents[testAgentGrok] = acB
	cfg2.Providers["claude"] = config.ProviderConfig{Command: "claude-fake-NEW"}
	cfg2.Providers["grok"] = config.ProviderConfig{Command: "grok-fake-NEW"}

	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	joinAgent(t, svc, convID, testAgentGrok)
	postUser(t, svc, convID, "task-for-both")

	r := newTestRunner(t, cfg1, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	cs.enqueue(testAgentClaude, spawnResult{stdout: []byte(minimalClaudeSuccess("SESSION-A", "hi")), exitCode: 0})
	cs.enqueue(testAgentGrok, spawnResult{stdout: []byte(minimalGrokSuccess("SESSION-B", "hi")), exitCode: 0})

	gate := newNowBarrier(2)
	r.now = gate.now

	var wg sync.WaitGroup
	var recA, recB Record
	var errA, errB error
	wg.Add(2)
	go func() {
		defer wg.Done()
		recA, errA = r.Ask(context.Background(), convID, testAgentClaude)
	}()
	go func() {
		defer wg.Done()
		recB, errB = r.Ask(context.Background(), convID, testAgentGrok)
	}()

	gate.waitArrived(t, 5*time.Second, "两条并发 Ask 没有都在 5s 内到达各自的 r.now 同步点")
	setConfigWithTimeout(t, r, cfg2, 2*time.Second, func() { close(gate.release) })
	close(gate.release)
	wg.Wait()

	if errA != nil {
		t.Fatalf("agent claude 这一路 Ask 失败: %v", errA)
	}
	if errB != nil {
		t.Fatalf("agent grok 这一路 Ask 失败: %v", errB)
	}

	callsA := cs.callsFor(testAgentClaude)
	callsB := cs.callsFor(testAgentGrok)
	if len(callsA) != 1 {
		t.Fatalf("期望 claude 这一路恰好一次 Spawner 调用，got %d", len(callsA))
	}
	if len(callsB) != 1 {
		t.Fatalf("期望 grok 这一路恰好一次 Spawner 调用，got %d", len(callsB))
	}

	if got := callsA[0].cmd.Path; got != cfg1.Providers["claude"].Command {
		t.Errorf("claude 这一轮 cmd.Path = %q，期望仍是旧配置 %q", got, cfg1.Providers["claude"].Command)
	}
	if got, found := argFlagValue(callsA[0].cmd.Args, "--model"); !found || got != cfg1.Agents[testAgentClaude].Model {
		t.Errorf("claude 这一轮 argv 的 --model = %q(found=%v)，期望旧配置 %q", got, found, cfg1.Agents[testAgentClaude].Model)
	}

	if got := callsB[0].cmd.Path; got != cfg1.Providers["grok"].Command {
		t.Errorf("grok 这一轮 cmd.Path = %q，期望仍是旧配置 %q", got, cfg1.Providers["grok"].Command)
	}
	if got, found := argFlagValue(callsB[0].cmd.Args, "--model"); !found || got != cfg1.Agents[testAgentGrok].Model {
		t.Errorf("grok 这一轮 argv 的 --model = %q(found=%v)，期望旧配置 %q", got, found, cfg1.Agents[testAgentGrok].Model)
	}

	pClaude, err := provider.Lookup(cfg1.Agents[testAgentClaude].Provider)
	if err != nil {
		t.Fatalf("provider.Lookup(claude) 失败: %v", err)
	}
	tierClaude, err := TranslatePermission(cfg1.Agents[testAgentClaude].Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission(claude) 失败: %v", err)
	}
	wantFPA, err := identityFPFor(pClaude, testAgentClaude, cfg1.Agents[testAgentClaude], r.workDir, convID, tierClaude, RulesBlocks{})
	if err != nil {
		t.Fatalf("独立计算 claude 指纹失败: %v", err)
	}
	if recA.IdentityFP != wantFPA {
		t.Errorf("recA.IdentityFP = %q，期望 %q", recA.IdentityFP, wantFPA)
	}

	pGrok, err := provider.Lookup(cfg1.Agents[testAgentGrok].Provider)
	if err != nil {
		t.Fatalf("provider.Lookup(grok) 失败: %v", err)
	}
	tierGrok, err := TranslatePermission(cfg1.Agents[testAgentGrok].Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission(grok) 失败: %v", err)
	}
	wantFPB, err := identityFPFor(pGrok, testAgentGrok, cfg1.Agents[testAgentGrok], r.workDir, convID, tierGrok, RulesBlocks{})
	if err != nil {
		t.Fatalf("独立计算 grok 指纹失败: %v", err)
	}
	if recB.IdentityFP != wantFPB {
		t.Errorf("recB.IdentityFP = %q，期望 %q", recB.IdentityFP, wantFPB)
	}
}

// ---------------------------------------------------------------------------
// 用例 3：AuditDrift / RulesReport 与 SetConfig 并发
// ---------------------------------------------------------------------------

// TestRulesReportStaysWholeGenerationUnderConcurrentSetConfig 起若干
// goroutine 反复调 RulesReport 与 AuditDrift，同时另一条 goroutine 反复
// SetConfig 在 cfg1/cfg2 之间来回换，断言不 panic、不死锁（超时兜底），
// 且每一次 RulesReport() 的输出整份来自单一配置生成，不出现一行来自
// cfg1、另一行来自 cfg2 的「半新半旧」。
//
// 「半新半旧」的观察点：cfg2 把三个 provider 的 Command 统一加长同一个
// 后缀，probeHeadroom（rules_report.go）里那句
// 「remaining := maxCommandLineBytes - CommandLineBytes(path, cmd.Args)」
// 算出的 HeadroomBytes 因此对每一行都恰好减少同一个量——用这个量把一行
// 的 HeadroomBytes 精确分类成「来自 cfg1」或「来自 cfg2」。
//
// 任务书原文预判「这条可能今天就绿：RulesReport 现在是同步调用，没有并发
// 调用方」。第一版实现（flipper 只转固定 400 次）确实测出了绿，但那是假
// 阳性：flipper 那 400 次紧凑的 SetConfig 微秒级跑完，早在 8 个读者线程
// 跑出 setup 之前就已经全部换完，全程没有真正与 RulesReport 的读窗口重叠，
// 「2400 次读、0 次半新半旧」只是没测到，不是测到了没问题。改成按
// stop channel 收工（读者与审计都跑完 flipper 才停，实测贯穿约 10 秒、
// 上亿次 SetConfig）之后，结果翻红：约 75%（5 次独立运行分别是 1780～
// 1819 / 2400）的 RulesReport() 输出出现半新半旧。写这条用例时（#25
// 落地前）如实报告：这条当时是红的，任务书这一处的预判不成立——
// RulesReport 里 rulesInfoFor 逐行调 probeHeadroom，每行各自裸读一次
// r.cfg.Agents / r.cfg.Providers，行与行之间没有任何快照，天然就是半
// 新半旧的温床，比用例 1/2 挑的窄窗口容易撞见得多。
//
// #25 落地后：RulesReport 一进来取一次 rulesMu/cfgMu 读锁、拷走整份
// 快照、往下传，rulesInfoFor/probeHeadroom 不再自己取锁——这条用例
// 现在是回归哨兵，不是「预期会红」的用例。它当初真的红过、又被修
// 绿，这一点本身就是它的判别力所在：一条从未红过的用例证明不了
// RulesReport 曾经不安全，这条能。本函数末尾 len(mixed)==0 那个 else
// 分支的注释记着修复前后的具体数字（1780～1819 / 2400 到 0/2400，以
// 及变异回逐行取快照复现 1788/2400），此处不重复。
func TestRulesReportStaysWholeGenerationUnderConcurrentSetConfig(t *testing.T) {
	cfg1 := newTestConfig()
	cfg2 := newTestConfig()
	for _, id := range []string{"claude", "grok", "agy"} {
		pc := cfg2.Providers[id]
		pc.Command = pc.Command + "-EXTRA-SUFFIX-FOR-CFG2"
		cfg2.Providers[id] = pc
	}

	svc := newTestService(t)

	// 独立跑两个 Runner，各自只吃一份配置，拿「纯 cfg1」与「纯 cfg2」的
	// 参照行——同一个 workDir 才能让两边的探针命令行长度只因为 Providers
	// 的 Command 不同而不同，不掺进 workDir 路径长度这个无关变量。
	r1 := newTestRunner(t, cfg1, svc)
	r2 := newTestRunnerSameWorkDir(r1, cfg2, svc)
	ref1 := r1.RulesReport()
	ref2 := r2.RulesReport()
	if len(ref1) == 0 || len(ref1) != len(ref2) {
		t.Fatalf("参照行数不对：len(ref1)=%d len(ref2)=%d，前置条件不成立", len(ref1), len(ref2))
	}
	byKey1 := make(map[rulesReportKey]int, len(ref1))
	for _, row := range ref1 {
		byKey1[rulesReportKey{providerID: row.ProviderID, tier: row.Tier}] = row.HeadroomBytes
	}
	byKey2 := make(map[rulesReportKey]int, len(ref2))
	for _, row := range ref2 {
		byKey2[rulesReportKey{providerID: row.ProviderID, tier: row.Tier}] = row.HeadroomBytes
	}
	for k, b1 := range byKey1 {
		b2, ok := byKey2[k]
		if !ok {
			t.Fatalf("cfg2 的参照行里缺 %+v", k)
		}
		if b1 == b2 {
			t.Fatalf("%+v 这一行 cfg1/cfg2 的 HeadroomBytes 都是 %d，两份配置区分不出来，前置条件不成立", k, b1)
		}
	}

	r := newTestRunnerSameWorkDir(r1, cfg1, svc)

	const readers = 8
	const reads = 300
	const auditors = 4

	type mixedRecord struct {
		worker int
		round  int
		rows   []ProviderRulesInfo
	}
	var mu sync.Mutex
	var mixed []mixedRecord

	// flipper 按「读者都跑完再停」而不是固定次数循环：固定次数（例如 400
	// 次紧凑的 SetConfig）跑完只要微秒级，而 8 个读者 × 300 次 RulesReport()
	// 要跑掉数秒——固定次数版本几乎必然在第一个读者跑出 setup 之前就已经
	// 全部换完配置，等于全程没有真正与读者重叠，「2400 次读、0 次半新
	// 半旧」这个数字会变成没有意义的空心通过。改成读 stop 这个 channel，
	// 配合下面记录的 flipCount 与耗时，让「flipper 是否真的贯穿了整个
	// 读取窗口」这件事可核验，不是假设。
	stop := make(chan struct{})
	var flipCount int64
	flipStart := time.Now()
	var flipWG sync.WaitGroup
	flipWG.Add(1)
	go func() {
		defer flipWG.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			if i%2 == 0 {
				r.SetConfig(cfg2)
			} else {
				r.SetConfig(cfg1)
			}
			i++
			atomic.AddInt64(&flipCount, 1)
		}
	}()

	var readerWG sync.WaitGroup
	for w := 0; w < readers; w++ {
		w := w
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			for i := 0; i < reads; i++ {
				rows := r.RulesReport()
				gotOld, gotNew := false, false
				for _, row := range rows {
					key := rulesReportKey{providerID: row.ProviderID, tier: row.Tier}
					switch row.HeadroomBytes {
					case byKey1[key]:
						gotOld = true
					case byKey2[key]:
						gotNew = true
					default:
						// 两份参照都对不上：既不是纯 cfg1 也不是纯 cfg2，
						// 比「半新半旧」更糟，同样记下来。
						gotOld, gotNew = true, true
					}
				}
				if gotOld && gotNew {
					mu.Lock()
					mixed = append(mixed, mixedRecord{worker: w, round: i, rows: append([]ProviderRulesInfo(nil), rows...)})
					mu.Unlock()
				}
			}
		}()
	}

	var auditWG sync.WaitGroup
	for w := 0; w < auditors; w++ {
		auditWG.Add(1)
		go func() {
			defer auditWG.Done()
			for i := 0; i < reads; i++ {
				if _, err := r.AuditDrift(context.Background()); err != nil {
					t.Errorf("AuditDrift 返回了错误: %v", err)
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		readerWG.Wait()
		auditWG.Wait()
		close(stop) // 读者与审计都跑完了，flipper 才收工——保证它贯穿了整个读取窗口
		flipWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(concurrencyWatchdogBudget()):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("%v 内没有跑完，疑似死锁。全部 goroutine 现场：\n%s", concurrencyWatchdogBudget(), buf[:n])
	}

	flipElapsed := time.Since(flipStart)
	t.Logf("flipper 跑了 %v、共 %d 次 SetConfig——这两个数字是「flipper 真的贯穿了整个读取窗口」"+
		"这条前提的证据，不是装饰：flipper 现在按 stop channel 收工，不是固定次数，"+
		"所以它的耗时应当与读者/审计的耗时同一量级，不会是几微秒就提前跑完",
		flipElapsed, atomic.LoadInt64(&flipCount))

	if len(mixed) > 0 {
		t.Errorf("%d/%d 次 RulesReport() 输出里出现了半新半旧（同一次输出里既有 cfg1 的行、又有 cfg2 的行）",
			len(mixed), readers*reads)
		for i, m := range mixed {
			if i >= 3 {
				t.Logf("……还有 %d 次省略", len(mixed)-3)
				break
			}
			t.Logf("第 %d 次撞见半新半旧：worker=%d round=%d", i, m.worker, m.round)
			for _, row := range m.rows {
				t.Logf("  provider=%s tier=%v HeadroomBytes=%d HeadroomNote=%q", row.ProviderID, row.Tier, row.HeadroomBytes, row.HeadroomNote)
			}
		}
	} else {
		// 未决 #25 落地前实测：5 次独立运行，每次 2400 次读取，分别有
		// 1780/1787/1793/1814/1819 次半新半旧，稳定在 74%-76%——那时
		// RulesReport 逐行裸读 r.cfg，行与行之间没有快照，len(mixed)==0
		// 几乎不会发生。
		//
		// #25 落地后（同一提交顺带关掉 #26；RulesReport 一进来取一次
		// rulesMu/cfgMu 读锁、拷走整份快照、往下传，rulesInfoFor/
		// probeHeadroom 不再自己取锁）：4 次独立运行全是 0/2400，
		// 结构上应当每次走到这个 else 分支，4 次独立运行都走到了。
		// 变异回去验证过因果：把 rulesInfoFor 改成逐行重新取一次快照，
		// 同一份用例立即测出 1788/2400，回到修复前的量级。
		//
		// 所以 len(mixed)==0 现在不是"这次抽样没抽中"，是"整张表来自
		// 同一份快照"这条结构性保证的直接体现——上面那句记录 flipper
		// 耗时与次数的 t.Logf 就是证据：flipper 真的贯穿了整个读取
		// 窗口，不是提前跑完，0 次半新半旧才有意义。断言本身
		// （len(mixed) 必须为 0）留着当回归哨兵：谁把 RulesReport 改回
		// 逐行取锁，这条用例会红，且红出来的次数会回到 1780 量级
		// （2400 次读取里约 74%-76%）。
		t.Logf("%d 次 RulesReport() 读取，0 次半新半旧——整张表来自同一份"+
			"快照，这是 #25 落地后的结构性保证，不是抽样没抽中"+
			"（修复前 5 次独立运行稳定在约 74%%-76%% 半新半旧，"+
			"变异回逐行取快照能立即复现 1788/2400）",
			readers*reads)
	}
}

// ---------------------------------------------------------------------------
// 用例 4：锁顺序不成环
// ---------------------------------------------------------------------------

// TestNoDeadlockUnderMixedConcurrentOperations 混着并发跑 Ask / SessionInfo /
// Rebuild / SetRules / SetConfig / AuditDrift / RulesReport 若干轮，带超时
// 兜底，超时即判失败并 dump 全部 goroutine 现场。
//
// 这条能证明什么、不能证明什么必须说清楚：它能在死锁真的发生时让本用例
// 超时红，附带可诊断的 goroutine 现场；它不能证明「不存在死锁」——锁
// 顺序有没有被破坏是静态性质，压力测试只是抽样，抽不中不代表没有。
//
// 写这条用例时（#25 落地前）cfgMu 根本不存在（未决 #25 本身要补的那
// 个缺口），本用例当时能压到的只有既有的两把锁：g.mu（sessionGate，
// 按 key）与 rulesMu。理论上必然绿（上一段已说明这条用例能证明什么、
// 不能证明什么）——那时 SetConfig 是无锁裸写，不可能跟谁抢锁抢出死锁；
// 这条用例从写下的第一天起就是绿的，跟本文件另外三条（写下时红、
// #25 落地后才转绿）不是一回事——它没有「曾经红过又被修绿」这层判
// 别力，写在这里纯粹是留一个回归哨兵：#25 落地、cfgMu 真正加上之后
// 复跑这份完全相同的用例，若新增的加锁路径破坏了「g.mu 先于
// rulesMu/cfgMu，且没有任何路径持有后两者再取 g.mu」这条全局顺序，
// 这里会先炸出来。
func TestNoDeadlockUnderMixedConcurrentOperations(t *testing.T) {
	cfg1 := newTestConfig()
	cfg2 := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	joinAgent(t, svc, convID, testAgentGrok)

	r := newTestRunner(t, cfg1, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn

	const iterations = 60

	var seedMu sync.Mutex
	seed := 0
	nextSeed := func() int {
		seedMu.Lock()
		defer seedMu.Unlock()
		seed++
		return seed
	}

	var wg sync.WaitGroup

	askLoop := func(agentID string, sample func(sessionID, text string) string) {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			n := nextSeed()
			postUser(t, svc, convID, fmt.Sprintf("mix-%s-%d", agentID, n))
			cs.enqueue(agentID, spawnResult{stdout: []byte(sample(fmt.Sprintf("s-%s-%d", agentID, n), "ok")), exitCode: 0})
			_, _ = r.Ask(context.Background(), convID, agentID)
		}
	}
	wg.Add(2)
	go askLoop(testAgentClaude, minimalClaudeSuccess)
	go askLoop(testAgentGrok, minimalGrokSuccess)

	sessionInfoLoop := func(agentID string) {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			r.SessionInfo(context.Background(), convID, agentID)
		}
	}
	wg.Add(2)
	go sessionInfoLoop(testAgentClaude)
	go sessionInfoLoop(testAgentGrok)

	// Rebuild 用 g.mu.TryLock()：askLoop 的两个 goroutine 各自整轮持有同一把
	// g.mu，Rebuild 撞上在飞的 Ask 时立刻拿 ErrTurnInFlight 弹回，不排队等。
	// 这意味着这两条 rebuildLoop 绝大多数迭代会在微秒级弹回，真正走到
	// g.mu 已拿到、执行 DropProviderSession 那条路径的次数其实很少——
	// 它们在本用例里主要起「不断敲门」的搅局作用，不是在充分覆盖
	// 「拿到锁之后那条路径」与其它操作的交叉。TryLock 本身的性质决定了
	// 它不可能是死锁的一方（拿不到就退，不会持有等待），把它摆在这里是
	// 陪跑，不是本用例覆盖率的主力。
	rebuildLoop := func(agentID string) {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _ = r.Rebuild(context.Background(), convID, agentID)
		}
	}
	wg.Add(2)
	go rebuildLoop(testAgentClaude)
	go rebuildLoop(testAgentGrok)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				r.SetRules(RulesBlocks{Global: "MIX-A"})
			} else {
				r.SetRules(RulesBlocks{Global: "MIX-B"})
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				r.SetConfig(cfg2)
			} else {
				r.SetConfig(cfg1)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _ = r.AuditDrift(context.Background())
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = r.RulesReport()
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(concurrencyWatchdogBudget()):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("%v 内没有跑完，疑似死锁。全部 goroutine 现场：\n%s", concurrencyWatchdogBudget(), buf[:n])
	}
}

// concurrencyWatchdogBudget 是这两个并发用例「疑似死锁」看门狗的时限。
//
// 为什么不是固定的 30 秒：这两条用例的负载随 CPU 争用变化很大 —— 24 核开发机
// 上 7.8 秒跑完，GitHub 的 2 vCPU runner 上 30 秒不够，于是在 CI 上稳定报
// 「疑似死锁」而本地永远过（release workflow 第一次 dry run 就撞上了）。
//
// 看门狗要抓的是「真的卡死」，不是「这台机器慢」。所以按核数放大：
// 单核 4 倍、双核 3 倍、四核 2 倍，四核以上维持 30 秒。
func concurrencyWatchdogBudget() time.Duration {
	switch n := runtime.NumCPU(); {
	case n <= 1:
		return 120 * time.Second
	case n == 2:
		return 90 * time.Second
	case n <= 4:
		return 60 * time.Second
	default:
		return 30 * time.Second
	}
}
