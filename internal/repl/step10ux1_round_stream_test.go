package repl

// 本文件覆盖冻结契约 E:\multi_model_cowork\step10ux1_contract.md 第 6 节
// 验收 9-10：/round --go 逐条输出早于结算、且结算没有因为改成流式而丢。
//
// 背对背纪律：本文件依赖的是 round.go 里 runOneRound 尚不存在的新行为
// （把 RunRoundPrepared 换成 RunRoundPreparedStream，逐条渲染从「全部
// 回来之后」挪进回调），此刻这一行为还没有被实现腿完整落地，本文件的
// 用例会先编译通过、再以「红」收场，直到 round.go 落地——预期状态。
//
// 判别力来源：纯粹按最终输出里的文本顺序断言「成员段在结算段之前」对
// 旧的「全部回来再一起打」的实现同样为真（旧实现也是先打每个成员、
// 再打结算，只是打的时刻都在最后），没有判别力，只能当回归钉写在终态
// 部分。真正有判别力的是中途探测：让一个成员快（首轮 bootstrap，假 CLI
// 立即回）、另一个慢（已经问过一次，这一轮带 --resume，假 CLI 睡
// inFlightSleep=700ms），在两者之间的窗口里探一次输出快照。
//
// 探测不能拿"输出里有没有出现 rx_fast 这个字符串"当判据——runOneRound
// 在 --go 判断之前就已经把预算表（renderBudget，每行都印 AgentID）和
// 起进程之前的阻塞提示打出来了，这两样与是否流式无关，旧实现在这个
// 窗口里同样会有 rx_fast 这个子串，用它当锚点是恒真的空断言。真正只有
// 流式实现才会在这个窗口里产出的东西是 writeAskRecord 打的那一行——它
// 唯一的、验收 7/8 已经验证过只出自这一处的锚点是 "outcome"（ask.go /
// round.go 都复用同一份 writeAskRecord，renderBudget / renderSettlement /
// argvRefusal 的文本里都没有这个词）。旧实现要等两个成员都回来才会调用
// 一次 writeAskRecord，所以探测点上不会有任何 "outcome"；流式实现应当
// 已经因为快成员回来而打过一次 "outcome"（且那一行里是 rx_fast，不是
// 还没回来的 rx_slow），结算段还没出现（结算必须等两个成员都回来）。
//
// 观测手段：与 session_inflight_test.go 同一条路子（本包已有 TestMain +
// COUNCIL_FAKE_CLI=1，不新增 TestMain），但这里需要两个不同的测试 agent
// （一快一慢），session_inflight_test.go 的 newInFlightConfig 只有一个，
// 所以本文件另起一份自己的两 agent 配置，不改那个文件。
//
// 本文件用真实 *store.Store（internal/turn 那套 fixtures 的路子），不用本包的
// newTestService。当初的理由是 fakeStore（conversation_test.go）内部 map 不加锁、
// 两个成员并发 BeginTurn 会撞并发 map 写；fakeStore 自 2026-09-25 起加了锁，
// 这条理由已不成立，留着真 store 是因为它本来就更接近生产路径。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/store"
	"council/internal/turn"
)

const (
	roundStreamAgentFast = "rx_fast" // 从没被问过，/round 里对它是首轮 bootstrap，假 CLI 立即回
	roundStreamAgentSlow = "rx_slow" // 提前单独问过一次，/round 里带 --resume，假 CLI 睡 700ms
)

// newRoundStreamConfig 造一份两个 agent 的配置，provider 都是登记过的
// "claude"，Command 是测试二进制自己（os.Args[0]，与 session_inflight_test.go
// 同一条路子），这样两个 agent 在 /round 里都会真的走到 spawn，不会在
// provider.Lookup 那一步就被 refuse 掉。
func newRoundStreamConfig() *config.Config {
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: os.Args[0]},
		},
		Agents: map[string]config.AgentConfig{
			roundStreamAgentFast: {
				DisplayName: "Fast",
				Provider:    "claude",
				Model:       "claude-fable-5-1",
				Effort:      "high",
				Role:        "ux1 round 流式用例",
				Permissions: config.PermissionRead,
				Mode:        config.ModeDiscussion,
			},
			roundStreamAgentSlow: {
				DisplayName: "Slow",
				Provider:    "claude",
				Model:       "claude-fable-5-1",
				Effort:      "high",
				Role:        "ux1 round 流式用例",
				Permissions: config.PermissionRead,
				Mode:        config.ModeDiscussion,
			},
		},
	}
}

// newRoundStreamService 造一个真实的 SQLite store 包一层
// conversation.Service，不用本包的 fakeStore（理由见文件头，原先的「不加锁」
// 已在 2026-09-25 修掉）。做法抄 internal/turn/fixtures_test.go 的 newTestService。
func newRoundStreamService(t *testing.T) *conversation.Service {
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
	return conversation.NewService(st)
}

// newRoundStreamRunner 把 svc、newRoundStreamConfig 拼成一个真正能起子
// 进程（其实是重新拉起自己）的 *turn.Runner，并设好 COUNCIL_FAKE_CLI=1
// （t.Setenv 用例结束时自动还原，不污染同包其它用例，与
// session_inflight_test.go 的 newInFlightRunner 同一条路子）。
func newRoundStreamRunner(t *testing.T, svc *conversation.Service) *turn.Runner {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	// -race 构建的测试二进制退出前默认睡 1 秒（race 运行时的 GORACE atexit_sleep_ms=1000），
	// 假 CLI 就是它自己，于是快成员在 350ms 探测点之前回不来，本用例在 -race 下必红（2026-09-25 实测
	// 3/3）。子进程继承环境（spawn.go cmd.Env 为 nil），这里把它关掉；不开 -race 时这个变量没有作用。
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	root := t.TempDir()
	return turn.NewRunner(newRoundStreamConfig(), svc, filepath.Join(root, "workdir"), turn.RulesBlocks{})
}

// TestRoundGoPerMemberOutputPrecedesSettlement 覆盖验收 9（逐条输出早于
// 结算）与验收 10（结算没有因为改流式而丢）。
func TestRoundGoPerMemberOutputPrecedesSettlement(t *testing.T) {
	svc := newRoundStreamService(t)
	rn := newRoundStreamRunner(t, svc)
	ctx := context.Background()

	conv, err := svc.Create(ctx, "ux1 round 流式用例")
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	convID := conv.ID

	for _, id := range []string{roundStreamAgentFast, roundStreamAgentSlow} {
		if err := svc.Join(ctx, convID, conversation.Participant{
			AgentID: id, Provider: "test", JoinedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("加入 %q 失败: %v", id, err)
		}
	}
	if _, err := svc.Post(ctx, conversation.Message{
		ConvID: convID, SenderID: conversation.ReservedSenderUser,
		Kind: conversation.KindUser, Text: "seed-msg",
	}); err != nil {
		t.Fatalf("post seed-msg 失败: %v", err)
	}

	// 让慢成员提前问过一次，建立 provider 会话——这样 /round 里对它的
	// 那一轮才会带 --resume，触发假 CLI 的 700ms 睡眠。快成员从没问过，
	// /round 里对它是首轮 bootstrap，假 CLI 立即回。
	firstSlow, err := rn.Ask(ctx, convID, roundStreamAgentSlow)
	if err != nil {
		t.Fatalf("建立慢成员首轮会话失败: %v；Record=%+v", err, firstSlow)
	}
	if !firstSlow.Fresh {
		t.Fatalf("前置条件不成立：慢成员首轮应当是 Fresh，实际不是: %+v", firstSlow)
	}
	if _, err := svc.Post(ctx, conversation.Message{
		ConvID: convID, SenderID: conversation.ReservedSenderUser,
		Kind: conversation.KindUser, Text: "second-msg",
	}); err != nil {
		t.Fatalf("post second-msg 失败: %v", err)
	}

	var out syncBuffer
	cfg := newRoundStreamConfig()
	r := New(cfg, svc, rn,
		strings.NewReader("/use "+convID+"\n/round "+roundStreamAgentFast+" "+roundStreamAgentSlow+" --go\n"),
		&out, RulesContext{})

	done := runREPLInBackground(r)

	// 与 session_inflight_test.go 同一个标定值：inFlightSleep 的一半，
	// 前后各留足够余量。
	time.Sleep(inFlightProbeDelay)

	select {
	case err := <-done:
		t.Fatalf("REPL.Run 在探测点（%v）之前就返回了（err=%v）——说明慢成员那一轮没有真的"+
			"带上 --resume 进入 700ms 睡眠窗口，前置条件不成立，下面的断言无法证明任何事；"+
			"完整输出:\n%s", inFlightProbeDelay, err, out.String())
	default:
	}

	snap := out.String()
	// 不能用「snap 里有没有 rx_fast」当判据：renderBudget 在 --go 判断
	// 之前就已经把预算表（每行都印 AgentID）和阻塞提示打出来了，这两样
	// 与是否流式无关，旧实现在这个窗口里同样会有这个子串——用它当锚点
	// 是恒真的空断言（不分好坏实现）。真正只有流式实现才会在这个窗口里
	// 产出的是 writeAskRecord 打的那一行，唯一、已经在验收 7/8 里验证过
	// 只出自这一处的锚点是 "outcome"。
	if !strings.Contains(snap, "outcome") {
		t.Fatalf("探测点（%v，落在慢成员 700ms 睡眠窗口正中间）的输出里还没出现 writeAskRecord "+
			"才会打的 \"outcome\"——如果实现是「全部回来之后再一起打」的旧形状，这条用例应当在"+
			"这里失败（旧实现此刻只会有预算表与阻塞提示，不会有任何一条 outcome 行）；"+
			"实际输出:\n%q", inFlightProbeDelay, snap)
	}
	if strings.Contains(snap, "发言轮结算") {
		t.Fatalf("前置条件不成立：探测点的输出里已经出现了结算段锚点「发言轮结算」"+
			"（round_render_test.go / round_test.go 已经用这个锚点钉过 renderSettlement 的输出），"+
			"说明这一轮已经跑完，本次探测没有真的卡在慢成员还没回来的窗口里；实际输出:\n%q", snap)
	}
	// 探测点上唯一应当出现过的 outcome 行属于快成员：慢成员此刻不可能
	// 已经回来（它在 700ms 睡眠窗口正中间）。
	outcomeLine := lineContaining(t, snap, "outcome")
	if !strings.Contains(outcomeLine, roundStreamAgentFast) {
		t.Errorf("探测点的 outcome 行里没有出现快成员 %q：%q", roundStreamAgentFast, outcomeLine)
	}
	if strings.Contains(outcomeLine, roundStreamAgentSlow) {
		t.Errorf("探测点的 outcome 行里出现了慢成员 %q——它此刻不可能已经回来："+
			"%q", roundStreamAgentSlow, outcomeLine)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("REPL.Run 失败: %v；完整输出:\n%s", err, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("REPL.Run 没有在预期时间内结束；完整输出:\n%s", out.String())
	}

	final := out.String()
	// 验收 10：汇总没丢。
	if !strings.Contains(final, "发言轮结算") {
		t.Errorf("最终输出里应当出现结算段（锚点「发言轮结算」）；完整输出:\n%s", final)
	}
	// 验收 9 的终态版本：只当回归钉——最终输出里"成员段在结算段之前"这个
	// 顺序对旧的「全部回来再打」实现同样成立，没有判别力，判别力在上面
	// 的中途探测。这里用 "outcome" 而不是裸 agent id 当锚点，理由同上面
	// 中途探测那一段：agent id 会先出现在预算表里，用它当锚点定位不到
	// "成员段"这个概念本身。
	idxOutcome := strings.Index(final, "outcome")
	idxSettle := strings.Index(final, "发言轮结算")
	if idxOutcome < 0 || idxSettle < 0 {
		t.Fatalf("最终输出里应当同时出现 outcome 行与结算段；完整输出:\n%s", final)
	}
	if !(idxOutcome < idxSettle) {
		t.Errorf("第一条 outcome 行没有出现在结算段之前；完整输出:\n%s", final)
	}
}
