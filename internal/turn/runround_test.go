package turn

// 本文件覆盖 docs/12_STEP9_CONTRACT.md 4.2/4.8 与任务书 step9p3b_tests.md
// 的六条用例：RunRound 的真并发、一轮内的确定性（中途 post 不可见 + 两次
// 延迟顺序不同 prompt 逐字节相同）、面包屑与 round_id 串起来、一个成员
// 失败不取消别人、返回顺序按 Speakers 字典序而非完成顺序。
//
// 背对背纪律：本文件只依赖任务书冻结的形状——
//   func (r *Runner) RunRound(ctx context.Context, convID string, plan Plan, maxParallel int) []Record
// 这个符号此刻还不存在（实现方 grok 在独立写同一片），编译不过是预期状态，
// 不为了让它过编译而去改任何实现文件。
//
// 本文件可以读 setconfig_concurrency_test.go / gate_concurrency_test.go /
// turn_usage_v4_columns_test.go 当体例参考、并直接复用它们已经定义好的
// 同包符号（raceSpawner、minimalClaudeSuccess/minimalGrokSuccess、
// agentIDFromCmd、argFlagValue、newTestServiceWithPath、
// tstTurnUsageRoundAndIdentity、newNowBarrier 等），但不改动那些文件一个字节——
// 任务书点名 setconfig_concurrency_test.go 此刻另有一路在改它的注释。
//
// 关于「中途 post 的消息」这条用例（下面 TestRunRoundMidRoundMessage...）
// 用什么当同步挂钩，是本文件价值最高的一处判断，写在那个函数上方的长注释里，
// 结论是：不用 setconfig_concurrency_test.go 那个 r.now 挂钩，改用一个包一层
// 真实 *store.Store、只改写 MaxSeq/Messages 两个方法的假 Store。理由同样写在
// 那条注释里。

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/store"
)

// ---------------------------------------------------------------------------
// 五人夹具：本文件六条用例共用同一套 5 个 agent 的配置
// ---------------------------------------------------------------------------

// fiveAgentIDs 是本文件用的 5 个测试 agent id，字典序排好——Plan.Speakers
// 按契约要求就是字典序，这里直接把顺序焊死，不必每处用例各自 sort 一遍。
func fiveAgentIDs() []string {
	return []string{"agent-a", "agent-b", "agent-c", "agent-d", "agent-e"}
}

// newFiveAgentConfig 造一份 5 个 agent 的最小配置，在 claude / grok 两家
// provider 之间轮换（claude 走 stdin、grok 走 argv 的 -p，两条 transport
// 都覆盖到，见 spawn.go 顶部注释与 grok.go/agy.go 的 "-p" 用法）。
// 不用 agy：本文件不需要第三种 transport，minimalClaudeSuccess/
// minimalGrokSuccess（gate_concurrency_test.go 已有）够用，不必再造一份
// agy 的最小成功样本。
//
// 全部 5 个 agent 的 Permissions 都是 read（newTestConfig 同款裁决）：
// agentWorkDir 对 read 忽略 convID（fixtures_test.go 的 fingerprintFor 注释），
// 这个性质被 TestRunRoundSameFixtureTwiceByteIdenticalPrompts 依赖——
// 同一个 Runner 跑两个不同的会话，同一个 agentID 的 cmd.Dir 必须相同。
func newFiveAgentConfig() *config.Config {
	providers := []string{"claude", "grok"}
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: "claude-fake"},
			"grok":   {Command: "grok-fake"},
		},
		Agents: map[string]config.AgentConfig{},
	}
	for i, id := range fiveAgentIDs() {
		p := providers[i%len(providers)]
		cfg.Agents[id] = config.AgentConfig{
			DisplayName: id,
			Provider:    p,
			Model:       p + "-model",
			Effort:      "high",
			Role:        "RunRound 测试角色",
			Permissions: config.PermissionRead,
			Mode:        config.ModeDiscussion,
		}
	}
	return cfg
}

// successJSONFor 按 agentID 在 cfg 里挂的 provider，挑对应格式的最小成功样本。
func successJSONFor(cfg *config.Config, agentID, sessionID, text string) string {
	if cfg.Agents[agentID].Provider == "grok" {
		return minimalGrokSuccess(sessionID, text)
	}
	return minimalClaudeSuccess(sessionID, text)
}

// promptTextOf 从一次真实的 Spawner 调用里取出「送出去的正文」本身，不含
// --cwd / --add-dir 这类携带 workDir 的旁路参数——TestRunRoundSameFixtureTwice
// 需要跨两次调用比较正文，若连 --cwd 一起比，两次各自的 t.TempDir() 不同
// 会造成假阳性的不相等（本文件所有用例其实共用同一个 Runner/workDir，
// 这里仍然只取正文，是防御性的，不依赖这一点）。
// claude 走 stdin（spawn.go 顶部注释：「只有 claude 改走 stdin」），
// grok 走 argv 的 "-p"（grok.go:96）。
func promptTextOf(cmd provider.Command) string {
	if len(cmd.Stdin) > 0 {
		return string(cmd.Stdin)
	}
	if v, ok := argFlagValue(cmd.Args, "-p"); ok {
		return v
	}
	// 不应该走到这里：claude 用 stdin，grok 用 -p，本文件没有第三种 provider。
	// 兜底仍然给出点东西而不是空字符串，方便断言失败时看出问题出在哪一层。
	return "(既没有 stdin 也没有 -p 参数) args=" + strings.Join(cmd.Args, " ")
}

// promptContainsMarker 判断某次真实调用「送出去的字节」（argv 任一元素或
// stdin）里有没有出现 marker——任务书原话「判据是送出去的字节：argv 任一
// 元素或 stdin」，不局限于 -p 那一个参数：如果实现把正文分散写进多个参数
// （目前的三家适配器不会，但断言不该假设这一点），仍然要能抓到。
func promptContainsMarker(cmd provider.Command, marker string) bool {
	if strings.Contains(string(cmd.Stdin), marker) {
		return true
	}
	for _, a := range cmd.Args {
		if strings.Contains(a, marker) {
			return true
		}
	}
	return false
}

// completionOrder 把 ids 按 raceSpawner 记录到的真实完成时刻（timedSpawnCall.end）
// 升序排好，返回完成顺序。用于 TestRunRoundSameFixtureTwice 与
// TestRunRoundReturnOrder 断言「两次的完成顺序真的不同」「完成顺序确实
// 不等于 Speakers 顺序」这两条前置条件——不让用例的判别力建立在猜测上。
func completionOrder(cs *raceSpawner, ids []string) []string {
	type endAt struct {
		id  string
		end time.Time
	}
	ends := make([]endAt, 0, len(ids))
	for _, id := range ids {
		calls := cs.callsFor(id)
		if len(calls) == 0 {
			ends = append(ends, endAt{id: id, end: time.Time{}})
			continue
		}
		ends = append(ends, endAt{id: id, end: calls[0].end})
	}
	sort := func() {
		for i := 1; i < len(ends); i++ {
			for j := i; j > 0 && ends[j].end.Before(ends[j-1].end); j-- {
				ends[j], ends[j-1] = ends[j-1], ends[j]
			}
		}
	}
	sort()
	out := make([]string, len(ends))
	for i, e := range ends {
		out[i] = e.id
	}
	return out
}

// freshPlanFiveSpeakers 造一个只有 Speakers（5 人，字典序）的 Plan，
// Judges/Skipped 是空切片而不是 nil——Plan 自己的文档注释要求三个切片
// 都不为 nil（plan.go:17-18），本文件手搭 Plan 字面量时照办。
// 片 3d 才跑 Judges，本片契约明写「plan.Judges 本片不跑」，所以这里恒空。
func freshPlanFiveSpeakers() Plan {
	return Plan{Speakers: fiveAgentIDs(), Judges: []string{}, Skipped: []SkipReason{}}
}

// ---------------------------------------------------------------------------
// 用例 1：真并发——5 人各睡 200ms，总墙钟 < 400ms
// ---------------------------------------------------------------------------

func TestRunRoundRealConcurrencyFiveMembersUnder400ms(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "round1-seed")

	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	for _, id := range ids {
		cs.setSleep(id, 200*time.Millisecond)
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS1-"+id, "ok")), exitCode: 0})
	}

	plan := freshPlanFiveSpeakers()
	start := time.Now()
	records := r.RunRound(context.Background(), convID, plan, 0)
	elapsed := time.Since(start)
	t.Logf("5 个成员各睡 200ms（假 Spawner），RunRound 总墙钟 = %v", elapsed)

	if elapsed >= 400*time.Millisecond {
		t.Fatalf("RunRound 总墙钟 = %v，期望 < 400ms（串行会是 1000ms 以上；"+
			"contract 12 §4.9 第 3 条就是这个数）", elapsed)
	}

	if len(records) != len(ids) {
		t.Fatalf("RunRound 返回 %d 条 Record，期望 %d", len(records), len(ids))
	}
	for _, rec := range records {
		if rec.Outcome != OutcomeSpoke {
			t.Errorf("agent %q 的 Outcome = %v，期望 OutcomeSpoke", rec.AgentID, rec.Outcome)
		}
	}
	for _, id := range ids {
		if n := len(cs.callsFor(id)); n != 1 {
			t.Errorf("agent %q 的 Spawner 调用次数 = %d，期望恰好 1", id, n)
		}
	}
}

// ---------------------------------------------------------------------------
// 用例 2：一轮内确定性——中途 post 的消息这一轮谁都看不到，下一轮都看到
// ---------------------------------------------------------------------------
//
// 这是本文件判断最重的一处，展开写清楚。
//
// 【要测什么】contract 12 §4.2：RunRound 进来先取一次 upTo := MaxSeq(convID)，
// 这一个值发给全部成员各自的 AskUpTo(upTo=...)。若实现漏掉这一步（例如每个
// 成员各自现取一次 MaxSeq，或者干脆传 upTo=0 不设上界），一条在扇出期间才
// post 的消息就可能被某些跑得慢的成员看见——这正是要抓的坏实现。
//
// 【陷阱】若测试把 marker 消息 post 得太晚——晚于全部 5 个成员都已经读完
// 历史、渲染完 prompt 之后——那么无论实现对不对，5 个人都看不到 marker，
// 断言恒真，这条用例什么都没测到（任务书原话）。必须让 marker 在「至少有
// 成员还没读历史」的窗口内落库。
//
// 【为什么不用 setconfig_concurrency_test.go 的 r.now 挂钩】
// 那个挂钩利用的是 AskUpTo 内部固定调用点
// 「rec.ColdExpected = coldExpectedAt(r.now(), ...)」——这一句在 readAfter
// 之前，且是 AskUpTo 成功路径上第一次调用 r.now()。这个事实对 AskUpTo
// 本身成立，因为 AskUpTo 是已经提交、本文件已经读过源码的冻结代码，本片
// 不改它。风险不在 AskUpTo 内部，而在 RunRound 自己：RunRound 是本片新写、
// 此刻完全不存在、由另一个人独立实现的代码。若它的 orchestration 前导
// 代码（扇出 5 个 goroutine 之前那一段，例如给 round_started 面包屑打一个
// 「开始时刻」）也调用了 r.now()，那一次调用同样会卡在 nowBarrier 的
// <-release 上——而这次是 RunRound 自己唯一的那条 goroutine被卡住，它还
// 没跑到"起 5 个子 goroutine"那一步，5 个 AskUpTo 内部的 r.now() 调用根本
// 不会发生，「凑够 5 次到达」永远等不到，用例卡死在 waitArrived 的 5s
// 超时上。这不是臆测的边角情形：round_started 面包屑要不要打时间戳、
// RunRound 要不要在扇出前后各测一次耗时，都是这一片完全交给实现方裁量的
// 空间（contract 12 §4.8 只要求面包屑「带上 round_id、upTo、成员名单」，
// 没冻结实现细节），本文件没有任何依据假设 RunRound 的前导代码不碰 r.now。
// setconfig_concurrency_test.go 的那条注释自己也写明了这个挂钩「脆弱」，
// 但它脆弱的前提（AskUpTo 的调用点不变）在那个文件里是真的；搬到这里，
// 前提本身就不成立——这就是任务书要求「自己判断同样的挂钩在这里成不成立」
// 的答案：不成立，换一个不依赖 RunRound 内部有没有调用 r.now 的挂钩。
//
// 【换用的挂钩】messagesGate（见下方类型定义）：包一层真实 *store.Store，
// 只改写 Messages 与 MaxSeq 两个方法，其余全部经嵌入原样透传。Messages
// 正是 readAfter -> Service.History -> store.Messages 这条路径最终落到的
// 那一次真实 DB 读——挡住它就是直接挡住「这个成员读到几条历史」这件事本身，
// 不依赖 RunRound 内部任何未冻结的调用序列。这个挂钩只要求一件事：
// RunRound 必须让每个 Speaker 走到 AskUpTo 里那次 svc.History 调用——这是
// 任务书冻结形状明写的「每个成员各起一个 goroutine 调 AskUpTo」的直接推论，
// 不是本文件的额外假设。
//
// 副产品：messagesGate 顺手记下 MaxSeq 被调用的次数与返回值，让本用例
// 能多断言一条契约原文——「取**一次** upTo := MaxSeq(convID)」——而不只是
// 断言"谁看没看见"这个间接效果。

// messagesGate 包一层真实 *store.Store：
//   - MaxSeq：透传给真实实现，同时记调用次数与最近一次返回值；
//   - Messages：先记一次「到达」，再挡在 <-release 上，release 关闭之前
//     一律不放行（不区分是不是「前 want 次」——每一次调用都挡，见下方
//     实现；这与 setconfig_concurrency_test.go 的 nowBarrier 同一个心智
//     模型，只是挂在 Store 接口而不是 r.now 上）。
type messagesGate struct {
	*store.Store

	mu          sync.Mutex
	maxSeqCalls int
	lastMaxSeq  int64

	want      int
	arrived   int
	fired     bool
	arrivedCh chan struct{}
	release   chan struct{}
}

func newMessagesGate(st *store.Store, want int) *messagesGate {
	return &messagesGate{Store: st, want: want, arrivedCh: make(chan struct{}), release: make(chan struct{})}
}

func (g *messagesGate) MaxSeq(ctx context.Context, convID string) (int64, error) {
	n, err := g.Store.MaxSeq(ctx, convID)
	g.mu.Lock()
	g.maxSeqCalls++
	if err == nil {
		g.lastMaxSeq = n
	}
	g.mu.Unlock()
	return n, err
}

func (g *messagesGate) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]conversation.Message, error) {
	g.mu.Lock()
	g.arrived++
	if g.arrived >= g.want && !g.fired {
		g.fired = true
		close(g.arrivedCh)
	}
	g.mu.Unlock()
	<-g.release
	return g.Store.Messages(ctx, convID, afterSeq, limit)
}

func (g *messagesGate) waitArrived(t *testing.T, timeout time.Duration, msg string) {
	t.Helper()
	select {
	case <-g.arrivedCh:
	case <-time.After(timeout):
		close(g.release)
		t.Fatal(msg)
	}
}

// newGatedTestService 造一个真实 SQLite store，包一层 messagesGate 再包一层
// conversation.Service。与 fixtures_test.go 的 newTestService / 本包
// turn_usage_v4_columns_test.go 的 newTestServiceWithPath 功能一致，只是
// 中间多插了 messagesGate 这一层，另写一份而不是改那两个文件。
func newGatedTestService(t *testing.T, want int) (*conversation.Service, *messagesGate) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "council-data")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatalf("store.Open(%q) 失败: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试 store 失败: %v", err)
		}
	})
	gate := newMessagesGate(st, want)
	return conversation.NewService(gate), gate
}

func TestRunRoundMidRoundMessageInvisibleThisRoundVisibleNextRound(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc, gate := newGatedTestService(t, len(ids))
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "round2-seed")

	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS2A-"+id, "resp-1-"+id)), exitCode: 0})
	}

	plan := freshPlanFiveSpeakers()

	var records []Record
	done := make(chan struct{})
	go func() {
		records = r.RunRound(context.Background(), convID, plan, 0)
		close(done)
	}()

	gate.waitArrived(t, 5*time.Second,
		"5 个成员没有都在 5s 内到达 Store.Messages 的挂钩点——若 RunRound 自己的"+
			"扇出前导代码里有任何一次会真正读一遍历史（不应该有：readAfter 只在"+
			"每个 AskUpTo 内部各发生一次），或者 RunRound 根本没有按冻结形状"+
			"「每个成员各起一个 goroutine 调 AskUpTo」实现，这里会先死锁超时，"+
			"而不是本用例其余断言的问题")

	const marker = "MIDROUND-MARKER-2c7f9a"
	markerMsg := postUser(t, svc, convID, marker)

	close(gate.release)
	<-done

	// 契约直接断言：取**一次** upTo。
	if gate.maxSeqCalls != 1 {
		t.Errorf("Store.MaxSeq 被调用 %d 次，期望恰好 1 次——契约要求「进来先取一次"+
			" upTo := MaxSeq(convID)」，发给全部成员共用同一个值", gate.maxSeqCalls)
	}
	// 结构性前置条件：marker 的 seq 必须真的晚于 RunRound 捕获的 upTo，
	// 否则下面「这一轮看不到 marker」的断言即使红也不能怪 upTo 边界逻辑——
	// 有可能只是 marker 根本没有晚于 upTo。
	if markerMsg.Seq <= gate.lastMaxSeq {
		t.Fatalf("前置条件不成立：marker 的 seq=%d，RunRound 捕获的 upTo=%d，"+
			"marker 没有真的晚于 upTo，下面的断言无法证明任何事", markerMsg.Seq, gate.lastMaxSeq)
	}

	if len(records) != len(ids) {
		t.Fatalf("RunRound 返回 %d 条 Record，期望 %d", len(records), len(ids))
	}
	for _, rec := range records {
		if rec.Outcome != OutcomeSpoke {
			t.Fatalf("agent %q 的 Outcome = %v，期望 OutcomeSpoke（前置条件不成立）", rec.AgentID, rec.Outcome)
		}
	}
	for _, id := range ids {
		calls := cs.callsFor(id)
		if len(calls) != 1 {
			t.Fatalf("agent %q 第一轮的 Spawner 调用次数 = %d，期望 1", id, len(calls))
		}
		if promptContainsMarker(calls[0].cmd, marker) {
			t.Errorf("agent %q 第一轮送出去的字节里出现了 marker %q——"+
				"这条消息是在 upTo 快照之后才 post 的，这一轮不该看到它", id, marker)
		}
	}

	// 下一轮：不带上界的 Ask（这里直接用 RunRound 再跑一次，同一个 plan），
	// 5 个人都应该看到 marker。
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS2A-"+id, "resp-2-"+id)), exitCode: 0})
	}
	records2 := r.RunRound(context.Background(), convID, plan, 0)
	if len(records2) != len(ids) {
		t.Fatalf("第二轮 RunRound 返回 %d 条 Record，期望 %d", len(records2), len(ids))
	}
	for _, rec := range records2 {
		if rec.Outcome != OutcomeSpoke {
			t.Errorf("第二轮 agent %q 的 Outcome = %v，期望 OutcomeSpoke", rec.AgentID, rec.Outcome)
		}
	}
	for _, id := range ids {
		calls := cs.callsFor(id)
		if len(calls) != 2 {
			t.Fatalf("agent %q 累计 Spawner 调用次数 = %d，期望 2", id, len(calls))
		}
		if !promptContainsMarker(calls[1].cmd, marker) {
			t.Errorf("agent %q 第二轮送出去的字节里没有 marker %q，期望下一轮能看到它", id, marker)
		}
	}
}

// ---------------------------------------------------------------------------
// 用例 3：两次跑同一份夹具，假 Spawner 用相反的返回延迟顺序，prompt 逐字节相同
// ---------------------------------------------------------------------------
//
// 「同一份夹具」不能是同一个 Runner 对同一条会话跑两次 RunRound：第一次
// 跑完，5 个人已经各自建立了 provider 会话、游标也推进了，第二次会把第一次
// 的 5 条回复当成新 delta（且会带 --resume），prompt 与第一次结构性不同，
// 不是在测「完成顺序不同会不会扰动同一批输入的渲染结果」。
//
// 改用同一个 Runner（同一份 cfg、同一个 workDir——workDir 相同这件事本身
// 也重要：agentWorkDir 对 read 档忽略 convID，两条会话下同一个 agentID 的
// cmd.Dir 相同）+ 两条独立会话，会话历史用完全相同的种子文本按相同顺序
// post。经 render.go 核对过 RenderDelta/Router.Render 的实现：正文只由
// m.SenderID + m.Text（以及 bootstrap 时的 conv.Title）拼出来，不含
// ConvID/Seq/CreatedAt，newConv 两次都用同一个硬编码标题，所以两条独立
// 会话只要种子文本相同，正确实现下每个 agent 拿到的 prompt 应当逐字节相同——
// 这不是猜测，是读了 render.go 之后的结论。
//
// 「延迟顺序真的不同」：不只是设两组不同的 sleep 数字就完了——本函数另外
// 用 completionOrder 把两次真实观测到的完成顺序算出来，断言它们不相等，
// 前置条件不成立就 Fatal，不让用例在「其实两次顺序一样」的情况下侥幸绿过。
func TestRunRoundSameFixtureTwiceByteIdenticalPromptsUnderDifferentCompletionOrders(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	seedTexts := []string{"round3-seed-alpha", "round3-seed-bravo", "round3-seed-charlie"}
	setupConv := func() string {
		convID := newConv(t, svc)
		for _, id := range ids {
			joinAgent(t, svc, convID, id)
		}
		for _, text := range seedTexts {
			postUser(t, svc, convID, text)
		}
		return convID
	}
	convA := setupConv()
	convB := setupConv()

	plan := freshPlanFiveSpeakers()

	// 跑 A：字典序最后的（agent-e）睡最短、最先返回；最前的（agent-a）睡最长。
	csA := newRaceSpawner()
	delaysDesc := []time.Duration{240 * time.Millisecond, 180 * time.Millisecond, 120 * time.Millisecond, 60 * time.Millisecond, 0}
	for i, id := range ids {
		csA.setSleep(id, delaysDesc[i])
		csA.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS3A-"+id, "resp-A-"+id)), exitCode: 0})
	}
	r.spawn = csA.spawn
	if recs := r.RunRound(context.Background(), convA, plan, 0); len(recs) != len(ids) {
		t.Fatalf("run A: RunRound 返回 %d 条 Record，期望 %d", len(recs), len(ids))
	}

	// 跑 B：反过来，字典序最前的最先返回。
	csB := newRaceSpawner()
	delaysAsc := []time.Duration{0, 60 * time.Millisecond, 120 * time.Millisecond, 180 * time.Millisecond, 240 * time.Millisecond}
	for i, id := range ids {
		csB.setSleep(id, delaysAsc[i])
		csB.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS3B-"+id, "resp-B-"+id)), exitCode: 0})
	}
	r.spawn = csB.spawn
	if recs := r.RunRound(context.Background(), convB, plan, 0); len(recs) != len(ids) {
		t.Fatalf("run B: RunRound 返回 %d 条 Record，期望 %d", len(recs), len(ids))
	}

	orderA := completionOrder(csA, ids)
	orderB := completionOrder(csB, ids)
	t.Logf("run A 观测到的完成顺序: %v", orderA)
	t.Logf("run B 观测到的完成顺序: %v", orderB)
	if fmt.Sprint(orderA) == fmt.Sprint(orderB) {
		t.Fatalf("前置条件不成立：两次观测到的 Spawner 完成顺序相同（%v），"+
			"没有制造出不同的返回延迟顺序，这条用例测不到东西", orderA)
	}

	for _, id := range ids {
		callsA := csA.callsFor(id)
		callsB := csB.callsFor(id)
		if len(callsA) != 1 || len(callsB) != 1 {
			t.Fatalf("agent %q 的调用次数不对：run A=%d run B=%d，期望都是 1", id, len(callsA), len(callsB))
		}
		pa := promptTextOf(callsA[0].cmd)
		pb := promptTextOf(callsB[0].cmd)
		if pa != pb {
			t.Errorf("agent %q 两次拿到的 prompt 不是逐字节相同：\n  run A: %q\n  run B: %q", id, pa, pb)
		}
	}
}

// ---------------------------------------------------------------------------
// 用例 4：面包屑与 round_id 串起来
// ---------------------------------------------------------------------------

func TestRunRoundBreadcrumbsCarryMatchingNonEmptyRoundID(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc, dbPath := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "round4-seed")

	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	for _, id := range ids {
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS4-"+id, "ok-"+id)), exitCode: 0})
	}

	records := r.RunRound(context.Background(), convID, freshPlanFiveSpeakers(), 0)
	if len(records) != len(ids) {
		t.Fatalf("RunRound 返回 %d 条 Record，期望 %d", len(records), len(ids))
	}
	for _, rec := range records {
		if rec.Outcome != OutcomeSpoke {
			t.Fatalf("agent %q 的 Outcome = %v，期望 OutcomeSpoke（前置条件不成立）", rec.AgentID, rec.Outcome)
		}
	}

	msgs, err := svc.History(context.Background(), convID, 0, 10000)
	if err != nil {
		t.Fatalf("读历史失败: %v", err)
	}

	var started, finished *conversation.Message
	for i := range msgs {
		m := &msgs[i]
		if m.Kind != conversation.KindSystem {
			continue
		}
		ev, _ := m.Meta[MetaEvent].(string)
		switch ev {
		case "round_started":
			if started != nil {
				t.Fatalf("库里有不止一条 event=round_started 的 KindSystem 消息")
			}
			started = m
		case "round_finished":
			if finished != nil {
				t.Fatalf("库里有不止一条 event=round_finished 的 KindSystem 消息")
			}
			finished = m
		}
	}
	if started == nil {
		t.Fatal("库里没有找到 event=round_started 的 KindSystem 消息")
	}
	if finished == nil {
		t.Fatal("库里没有找到 event=round_finished 的 KindSystem 消息")
	}

	// 只钉任务书明确认定安全的两件事：Meta["event"] 的字符串值（已经用来
	// 挑出 started/finished），以及 Meta["round_id"] 这个键存在、非空、
	// 且 round_started 与 round_finished 两条上的值一致，并与 turn_usage
	// 里那个一致。不钉 upTo / 成员名单 / 结算数字的具体键名——那些留给
	// 实现方裁量（任务书原话）。
	startedRoundID, ok := started.Meta["round_id"].(string)
	if !ok || startedRoundID == "" {
		t.Fatalf("round_started 面包屑的 Meta[\"round_id\"] 缺失或不是非空字符串: %#v", started.Meta["round_id"])
	}
	finishedRoundID, ok := finished.Meta["round_id"].(string)
	if !ok || finishedRoundID == "" {
		t.Fatalf("round_finished 面包屑的 Meta[\"round_id\"] 缺失或不是非空字符串: %#v", finished.Meta["round_id"])
	}
	if startedRoundID != finishedRoundID {
		t.Fatalf("round_started 与 round_finished 的 round_id 不一致：%q vs %q（应当是同一轮）",
			startedRoundID, finishedRoundID)
	}

	for _, rec := range records {
		gotRoundID, _ := tstTurnUsageRoundAndIdentity(t, dbPath, rec.TurnID)
		if gotRoundID == "" {
			t.Errorf("agent %q 的 turn_usage.round_id 是空串，期望非空", rec.AgentID)
		}
		if gotRoundID != startedRoundID {
			t.Errorf("agent %q 的 turn_usage.round_id = %q，期望与面包屑的 round_id 一致：%q",
				rec.AgentID, gotRoundID, startedRoundID)
		}
	}
}

// ---------------------------------------------------------------------------
// 用例 5：一个成员失败不取消别人
// ---------------------------------------------------------------------------
//
// 陷阱（任务书原话）：若实现是「第一个错就 cancel」的形状，另外四个会拿到
// context canceled——若断言只看「四个人有没有 Record」，这两种情形分不开
// （两边都有 4 条 Record，只是 Outcome 不同）。
//
// 本用例不用 raceSpawner：raceSpawner.spawn 对 ctx 完全装聋作哑（它的
// time.Sleep(d) 不 select ctx.Done()），一个「失败就 cancel 别人」的坏实现
// 在这份假 Spawner 面前会被完全掩盖——ctx 被取消了，raceSpawner 毫不理会，
// 照样睡完、照样吐出预先塞好的成功 JSON，Outcome 会是 OutcomeSpoke，
// 与好实现的结果没有任何区别，用例恒真。
//
// 换成一个会 select ctx.Done() 的自定义 Spawner（与 round_cancel_test.go
// TestOutcomeCanceledVsTimeout 的手法同源：那个文件已经证明了 runner.go
// 会把「ctx 被取消」精确翻成 OutcomeCanceled，这里复用同一条因果链，只是
// 触发取消的源头换成「另一个成员失败」而不是「用户 Ctrl-C」）：
// 存活成员睡一段足够长的时间，期间用 select 监听 ctx.Done()——若被取消，
// 返回 killed=true + ctx.Err()，runner.go 的判据
// 「errors.Is(spawnErr, context.Canceled)」会把它标成 OutcomeCanceled，
// 与「正常成功、Outcome=OutcomeSpoke」在返回值上截然不同，好坏实现在这里
// 必然分道扬镳。
func TestRunRoundOneMemberFailureDoesNotCancelOthers(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	victim := ids[2] // agent-c，取中间那个，不占字典序首尾这两个特殊位置

	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "round5-seed")

	r := newTestRunner(t, cfg, svc)

	const survivorDelay = 200 * time.Millisecond
	var mu sync.Mutex
	survivorCalls := map[string]int{}
	var victimCalls int32

	r.spawn = func(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
		agentID := agentIDFromCmd(c)
		if agentID == victim {
			atomic.AddInt32(&victimCalls, 1)
			// 快速失败：不是超时、不是取消，是一次「进程压根没起来/起了但立刻
			// 非零退出」类型的失败（对应 runner.go 的 OutcomeSpawnFailed 分支）。
			return nil, []byte("boom: simulated provider failure"), 1, false, errors.New("simulated spawn failure")
		}
		mu.Lock()
		survivorCalls[agentID]++
		mu.Unlock()
		select {
		case <-time.After(survivorDelay):
			return []byte(successJSONFor(cfg, agentID, "SESS5-"+agentID, "ok-"+agentID)), nil, 0, false, nil
		case <-ctx.Done():
			// 好实现走不到这里（它们的 ctx 不该被别人的失败取消，这里的
			// select 会一直等到 time.After 那一支）；一旦走到这里，说明
			// 这个成员的 ctx 确实被取消了——把这一点如实翻给 runner.go，
			// 让它按自己的判据把 Outcome 标成 OutcomeCanceled/OutcomeTimeout，
			// 而不是让本文件的假 Spawner 替 runner.go 决定 Outcome。
			return nil, nil, -1, true, ctx.Err()
		}
	}

	records := r.RunRound(context.Background(), convID, freshPlanFiveSpeakers(), 0)

	if len(records) != len(ids) {
		t.Fatalf("RunRound 返回 %d 条 Record，期望 %d（失败成员也该有 Record，失败轮也要记账）",
			len(records), len(ids))
	}
	byID := make(map[string]Record, len(records))
	for _, rec := range records {
		byID[rec.AgentID] = rec
	}

	victimRec, ok := byID[victim]
	if !ok {
		t.Fatalf("返回的 Records 里缺了失败成员 %q", victim)
	}
	if victimRec.Outcome == OutcomeSpoke {
		t.Errorf("失败成员 %q 的 Outcome 是 OutcomeSpoke，期望某种失败结局", victim)
	}
	if victimRec.TurnID == "" {
		t.Errorf("失败成员 %q 的 TurnID 为空，期望非空——失败轮也要记账", victim)
	}
	if got := atomic.LoadInt32(&victimCalls); got != 1 {
		t.Fatalf("前置条件不成立：victim 的 Spawner 被调用 %d 次，期望恰好 1 次", got)
	}

	for _, id := range ids {
		if id == victim {
			continue
		}
		rec, ok := byID[id]
		if !ok {
			t.Fatalf("返回的 Records 里缺了存活成员 %q", id)
		}
		if rec.Outcome != OutcomeSpoke {
			t.Errorf("存活成员 %q 的 Outcome = %v（期望 OutcomeSpoke）——"+
				"如果这里是 OutcomeCanceled/OutcomeTimeout，说明实现在一个成员失败时"+
				"把其他人的 ctx 也取消了（「第一个错就 cancel」的坏形状），"+
				"这正是本用例要抓的缺陷，不是「四个人有没有 Record」能分辨的",
				id, rec.Outcome)
		}
		mu.Lock()
		n := survivorCalls[id]
		mu.Unlock()
		if n != 1 {
			t.Errorf("存活成员 %q 的 Spawner 被调用 %d 次，期望恰好 1 次", id, n)
		}
	}
}

// ---------------------------------------------------------------------------
// 用例 6：返回顺序确定——按 Speakers 字典序，不按完成顺序
// ---------------------------------------------------------------------------

func TestRunRoundReturnOrderMatchesSpeakersNotCompletionOrder(t *testing.T) {
	cfg := newFiveAgentConfig()
	ids := fiveAgentIDs()
	svc := newTestService(t)
	convID := newConv(t, svc)
	for _, id := range ids {
		joinAgent(t, svc, convID, id)
	}
	postUser(t, svc, convID, "round6-seed")

	r := newTestRunner(t, cfg, svc)
	cs := newRaceSpawner()
	r.spawn = cs.spawn
	// 字典序最后的（agent-e）最先完成，最前的（agent-a）最后完成——
	// 与 Speakers 顺序完全相反，让「按完成顺序返回」的坏实现在这里露馅。
	delays := []time.Duration{240 * time.Millisecond, 180 * time.Millisecond, 120 * time.Millisecond, 60 * time.Millisecond, 0}
	for i, id := range ids {
		cs.setSleep(id, delays[i])
		cs.enqueue(id, spawnResult{stdout: []byte(successJSONFor(cfg, id, "SESS6-"+id, "ok-"+id)), exitCode: 0})
	}

	records := r.RunRound(context.Background(), convID, freshPlanFiveSpeakers(), 0)

	completed := completionOrder(cs, ids)
	reversed := make([]string, len(ids))
	for i, id := range ids {
		reversed[len(ids)-1-i] = id
	}
	t.Logf("观测到的完成顺序: %v", completed)
	if fmt.Sprint(completed) != fmt.Sprint(reversed) {
		t.Fatalf("前置条件不成立：观测到的完成顺序 %v 不是 Speakers 的逆序 %v，"+
			"本用例的判别力依赖「完成顺序确实不同于 Speakers 顺序」这一点", completed, reversed)
	}

	if len(records) != len(ids) {
		t.Fatalf("RunRound 返回 %d 条 Record，期望 %d", len(records), len(ids))
	}
	for i, want := range ids {
		if records[i].AgentID != want {
			t.Fatalf("records[%d].AgentID = %q，期望 %q——"+
				"返回顺序应当是 Speakers 的字典序 %v，不是观测到的完成顺序 %v",
				i, records[i].AgentID, want, ids, completed)
		}
	}
}
