package turn

// 本文件覆盖 8c-2（/round 有界扇出 + OutcomeCanceled）里落在 internal/turn
// 这一层的那部分契约（11d 3.8）：/round 本身（cmdRound 的串行循环、
// coldCount 预报）活在 internal/repl，那边的测试覆盖循环层面的行为；
// 这里只钉 Runner.Ask 自己的那半句——「取消来自上层」与「墙钟到点」必须
// 分开记账，且父 ctx 被取消不能把一条已经在库里的 provider 会话冲掉
// （3.4 那条修复的直接断言）。/round 的循环靠这半句才能在取消时正确停下：
// cmdRound 用 rec.Outcome == turn.OutcomeCanceled 决定打哪一句话，
// runner.go 判错了，/round 打出来的话就跟着错。
//
// 用 recordingStore（见 recording_store_test.go，本包既有文件，这里只读它
// 定义的类型，不改它）而不是 newTestService 的真库：本组断言要看的是
// turn_usage 这一行的 Outcome 具体是哪个整数、SetProviderSession 有没有
// 被再次调用、调用时传的 id 是不是原来那个——这些只能靠拦截 Tx 方法的
// 调用序列拿到，真库路径下 conversation.Store 没有给 turn_usage 配读方法
// （recording_store_test.go 顶部注释原话）。

import (
	"context"
	"testing"

	"council/internal/conversation"
	"council/internal/provider"
)

// TestOutcomeCanceledLiteralValue 钉住 record.go 的字面量约束：OutcomeCanceled
// 必须是 8，不能因为有人把它改回 iota 递推而悄悄变成别的数——库里的整数一旦
// 落盘就不能改口径，record.go 自己的注释把这条写得很明白（9 留给 OutcomeLost）。
// 顺带钉住 Ingested()：这一支与 OutcomeTimeout 暴露面相同，钱花没花不确定。
func TestOutcomeCanceledLiteralValue(t *testing.T) {
	if int(OutcomeCanceled) != 8 {
		t.Fatalf("int(OutcomeCanceled) = %d，期望字面量 8", int(OutcomeCanceled))
	}
	if OutcomeCanceled == OutcomeTimeout {
		t.Fatal("OutcomeCanceled 不该与 OutcomeTimeout 相等，否则 /round 没法把两者分开记账")
	}
	if int(OutcomeTimeout) != 5 {
		t.Fatalf("前置条件不成立：int(OutcomeTimeout) = %d，期望 5（本文件其余用例靠这个数区分两支）", int(OutcomeTimeout))
	}
	if OutcomeCanceled.Ingested() {
		t.Error("OutcomeCanceled 的 Ingested() 应当为假：这一轮有没有被 provider 吃下并不确定")
	}
}

// TestOutcomeCanceledVsTimeout 是本文件的主体：先用一轮真实成功建立一条暖
// 会话，再用一个会在被调用的那一刻就取消父 ctx 的假 Spawner 触发第二轮，
// 断言（1）Outcome 是 OutcomeCanceled 不是 OutcomeTimeout，
// （2）turn_usage 那一行的 Outcome 整数与之一致，
// （3）provider_sessions 那一行仍在、id 没被冲掉——3.4 修复的直接断言：
// failClosed 内部落库早已用 context.Background()，父 ctx 死不死不影响
// 这次回填。
//
// 对照组用完全相同的 killed/err 组合，唯一区别是不调 cancel()：断言必须
// 翻回 OutcomeTimeout。少了这一支，上面的断言测不出「取决于父 ctx 是否
// 取消」，只能证明「这份假 Spawner 的结果总是被判成 canceled」，那有可能
// 只是巧合或者实现忽略了 ctx.Err()、恒定返回某一个结局。
func TestOutcomeCanceledVsTimeout(t *testing.T) {
	t.Run("父 ctx 被取消：判 canceled，不冲掉已有会话", func(t *testing.T) {
		rs := newRecordingStore()
		svc := conversation.NewService(rs)
		cfg := newTestConfig()
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "第一轮，建立暖会话")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueueSuccess(claudeT1JSON)
		r.spawn = fs.spawn
		if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
			t.Fatalf("建立暖会话的首轮失败: %v", err)
		}
		if len(rs.setProviderSessionCalls) != 1 {
			t.Fatalf("前置条件不成立：首轮之后 SetProviderSession 调用次数 = %d，期望 1", len(rs.setProviderSessionCalls))
		}
		establishedSessionID := rs.setProviderSessionCalls[0].ProviderSessionID
		if establishedSessionID == "" {
			t.Fatal("前置条件不成立：首轮应当建立一个非空的会话 id")
		}

		postUser(t, svc, convID, "第二轮，将在飞行中被取消")

		cancelCtx, cancel := context.WithCancel(context.Background())
		// 这份假 Spawner 模拟「用户在这一轮飞行中按 Ctrl-C」：main.go 的
		// NotifyContext 取消的是传给 Ask 的父 ctx，不是 spawnCtx 自己的
		// 超时定时器。被调用的那一刻就 cancel()，再返回 context.Canceled——
		// 对应 runner.go 判据 `errors.Is(spawnErr, context.Canceled)` 那一支。
		r.spawn = func(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
			cancel()
			return nil, nil, -1, false, context.Canceled
		}

		rec, err := r.Ask(cancelCtx, convID, testAgentClaude)
		if err == nil {
			t.Fatal("被取消的这一轮应当返回错误")
		}
		if rec.Outcome != OutcomeCanceled {
			t.Fatalf("Outcome = %v（int=%d），期望 OutcomeCanceled（=8）；"+
				"变成 OutcomeTimeout 说明 runner.go 弄丢了「取消来自上层」与「墙钟到点」的区分",
				rec.Outcome, int(rec.Outcome))
		}

		if len(rs.recordUsageCalls) != 2 {
			t.Fatalf("RecordUsage 累计调用次数 = %d，期望 2", len(rs.recordUsageCalls))
		}
		second := rs.recordUsageCalls[1]
		if second.Outcome != int(OutcomeCanceled) {
			t.Errorf("turn_usage 第二行 Outcome = %d，期望 int(OutcomeCanceled)=%d，不是 int(OutcomeTimeout)=%d",
				second.Outcome, int(OutcomeCanceled), int(OutcomeTimeout))
		}

		// 3.4 修复的直接断言：库里那一行仍在，且 id 没被冲掉。
		if len(rs.setProviderSessionCalls) != 2 {
			t.Fatalf("SetProviderSession 累计调用次数 = %d，期望 2（会话还在，failClosed 仍回填当前状态）",
				len(rs.setProviderSessionCalls))
		}
		if got := rs.setProviderSessionCalls[1].ProviderSessionID; got != establishedSessionID {
			t.Errorf("取消这一轮回填的 ProviderSessionID = %q，期望仍是建立时的 %q（父 ctx 取消不该冲掉已有会话 id）",
				got, establishedSessionID)
		}
		gotPS, ok := rs.providerSessions[sessionKey{convID: convID, agentID: testAgentClaude}]
		if !ok {
			t.Fatal("provider_sessions 那一行不见了：父 ctx 取消不该删掉已有会话")
		}
		if gotPS.ProviderSessionID != establishedSessionID {
			t.Errorf("provider_sessions.ProviderSessionID = %q，期望仍是 %q", gotPS.ProviderSessionID, establishedSessionID)
		}
	})

	t.Run("对照组：同样的假结果，父 ctx 没取消时仍判 timeout", func(t *testing.T) {
		rs := newRecordingStore()
		svc := conversation.NewService(rs)
		cfg := newTestConfig()
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "第一轮，建立暖会话")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueueSuccess(claudeT1JSON)
		r.spawn = fs.spawn
		if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
			t.Fatalf("建立暖会话的首轮失败: %v", err)
		}
		establishedSessionID := rs.setProviderSessionCalls[0].ProviderSessionID

		postUser(t, svc, convID, "第二轮，纯超时，父 ctx 全程未取消")

		// 与上面那支一字不差的返回值（killed=false, err=context.Canceled 走的
		// 同一条判据分支），唯一变量是这次不调 cancel()：父 ctx 始终存活。
		r.spawn = func(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
			return nil, nil, -1, false, context.Canceled
		}

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if err == nil {
			t.Fatal("这一轮应当返回错误")
		}
		if rec.Outcome != OutcomeTimeout {
			t.Fatalf("Outcome = %v（int=%d），期望 OutcomeTimeout（=5）：父 ctx 没取消，"+
				"同样的假结果不该被判成 canceled", rec.Outcome, int(rec.Outcome))
		}
		second := rs.recordUsageCalls[len(rs.recordUsageCalls)-1]
		if second.Outcome != int(OutcomeTimeout) {
			t.Errorf("turn_usage 最后一行 Outcome = %d，期望 int(OutcomeTimeout)=%d", second.Outcome, int(OutcomeTimeout))
		}
		if got := rs.setProviderSessionCalls[len(rs.setProviderSessionCalls)-1].ProviderSessionID; got != establishedSessionID {
			t.Errorf("超时这一轮回填的 ProviderSessionID = %q，期望仍是 %q", got, establishedSessionID)
		}
	})
}

// TestOutcomeCanceledStillCommitsSuccessDespiteDeadParentCtx 覆盖 3.4 修复的
// 另一半：父 ctx 在 spawn 返回之前就被取消，但 provider 其实已经把这一轮
// 跑完（真实世界对应「用户按下 Ctrl-C 的同时，进程已经在写最后几个字节」）。
// runner.go 的注释原话：「成功返回之后落库一律用 context.Background()」——
// 这一支要证明的正是「父 ctx 死不死不改变这一事实」，与上面「取消导致失败」
// 那半句合起来才是完整的修复。
func TestOutcomeCanceledStillCommitsSuccessDespiteDeadParentCtx(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "唯一一轮：父 ctx 在 spawn 期间被取消，但 provider 已经吃完了")

	r := newTestRunner(t, cfg, svc)
	cancelCtx, cancel := context.WithCancel(context.Background())
	r.spawn = func(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
		cancel()
		return []byte(claudeT1JSON), nil, 0, false, nil
	}

	rec, err := r.Ask(cancelCtx, convID, testAgentClaude)
	if err != nil {
		t.Fatalf("provider 已经成功返回，这一轮不该报错: %v", err)
	}
	if rec.Outcome != OutcomeSpoke {
		t.Fatalf("Outcome = %v，期望 OutcomeSpoke：父 ctx 死不死不该影响「provider 已经吃完」这一事实", rec.Outcome)
	}
	if got := rs.agentAppendCount(); got != 1 {
		t.Errorf("Kind=Agent 的 AppendMessage 调用次数 = %d，期望 1：正文应当已经落库", got)
	}
	if len(rs.setProviderSessionCalls) != 1 || rs.setProviderSessionCalls[0].ProviderSessionID == "" {
		t.Fatalf("会话应当已经落库，实际 setProviderSessionCalls=%+v", rs.setProviderSessionCalls)
	}
}
