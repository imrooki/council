package turn

// 本文件覆盖 10a 契约 §3.1 与 10_STEP7_SPEC.md §9 验收表：Record.TurnID
// 无论结局如何都有值、两轮的 TurnID 不相同、形状是合法 v4 UUID（版本位与
// 变体位正确）。TurnID 在 Ask 一进入就铸（newV4UUID()，record.go），甚至
// 比「这个 agent 存不存在」都判得早，所以哪怕这一轮在起进程之前就被拒绝，
// 它也已经有值——这正是 turn_usage 主键从「(conv_id, agent_id, seq)」改成
// 「turn_id」之后，失败轮也记得上账的前提（10a §1）。

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

// v4UUIDPattern 逐段核对 8-4-4-4-12 的形状、版本位（第 3 段第一位必须是
// "4"）与变体位（第 4 段第一位必须是 8/9/a/b，RFC 4122 变体 1）。
// newV4UUID()（record.go）用小写十六进制格式化，这里只认小写。
var v4UUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func assertV4UUID(t *testing.T, id string) {
	t.Helper()
	if !v4UUIDPattern.MatchString(id) {
		t.Errorf("TurnID = %q 不是合法的 v4 UUID（期望 8-4-4-4-12，版本位为 4，变体位为 8/9/a/b）", id)
	}
}

// TestTurnIDPresentAndValidAcrossOutcomes 覆盖「每一轮都有值，无论结局
// 如何」：成功、provider 自报失败、起进程之前拒绝、超时，四支结局各测一次。
// 四支复用既有测试文件（runner_outcomes_test.go）里已经验证过的构造手法，
// 这里只加一层 TurnID 断言，不重新验证 Outcome 本身的判定逻辑——那不是
// 本文件的职责。
func TestTurnIDPresentAndValidAcrossOutcomes(t *testing.T) {
	t.Run("成功（OutcomeSpoke）", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "问题")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueueSuccess(claudeT1JSON)
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if err != nil {
			t.Fatalf("Ask 失败: %v", err)
		}
		if rec.TurnID == "" {
			t.Fatal("成功轮 TurnID 为空")
		}
		assertV4UUID(t, rec.TurnID)
	})

	t.Run("provider 自报失败（OutcomeProviderFailed）", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "用一个不存在的 model 名触发失败")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueue(spawnResult{stdout: []byte(claudeBadModelOut), exitCode: 1})
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if err == nil {
			t.Fatal("这一轮应当报错")
		}
		if rec.Outcome != OutcomeProviderFailed {
			t.Fatalf("Outcome = %v，前置条件不成立，期望 OutcomeProviderFailed", rec.Outcome)
		}
		if rec.TurnID == "" {
			t.Fatal("provider 自报失败那一轮 TurnID 为空——账不该依赖 messages 产出的 seq，失败轮也必须记得上")
		}
		assertV4UUID(t, rec.TurnID)
	})

	t.Run("起进程之前拒绝（OutcomeRefused）", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		postUser(t, svc, convID, "hi")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, "ghost-agent")
		if !errors.Is(err, ErrAgentUnknown) {
			t.Fatalf("errors.Is(err, ErrAgentUnknown) 应当为真，实际: %v", err)
		}
		if rec.Outcome != OutcomeRefused {
			t.Fatalf("Outcome = %v，前置条件不成立", rec.Outcome)
		}
		if rec.TurnID == "" {
			t.Fatal("被拒绝的那一轮 TurnID 也应当有值——TurnID 在 Ask 一进入就铸，比「这个 agent 存不存在」的判断更早")
		}
		assertV4UUID(t, rec.TurnID)
	})

	t.Run("超时（OutcomeTimeout）", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "触发超时")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueue(spawnResult{stdout: []byte{}, killed: true, err: context.DeadlineExceeded})
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if err == nil {
			t.Fatal("这一轮应当报错")
		}
		if rec.Outcome != OutcomeTimeout {
			t.Fatalf("Outcome = %v，前置条件不成立", rec.Outcome)
		}
		if rec.TurnID == "" {
			t.Fatal("超时那一轮 TurnID 为空")
		}
		assertV4UUID(t, rec.TurnID)
	})
}

// TestTurnIDDiffersAcrossTurns 覆盖「两轮的 TurnID 不相同」：同一个 agent、
// 同一场会话连续两轮，TurnID 必须各铸各的，不能复用上一轮的值——复用会让
// turn_usage 主键冲突，Tx.RecordUsage 的契约明写「同一个 turn_id 写两次
// 说明调用方把一轮记了两遍，那是 bug 不是重试」。
func TestTurnIDDiffersAcrossTurns(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	postUser(t, svc, convID, "第一轮")
	fs.enqueueSuccess(claudeT1JSON)
	rec1, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}

	postUser(t, svc, convID, "第二轮")
	fs.enqueueSuccess(claudeT2JSON)
	rec2, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}

	if rec1.TurnID == rec2.TurnID {
		t.Fatalf("两轮的 TurnID 相同: %q", rec1.TurnID)
	}
	assertV4UUID(t, rec1.TurnID)
	assertV4UUID(t, rec2.TurnID)
}
