package turn

// 本文件覆盖 09 F 节「cost 差值」的五条规则与 K 节「cost 差值」组：
// claude 的 total_cost_usd 是会话累计值，Runner 必须算差值；grok/agy 没有
// 金额字段时 CostUSDDelta 必须是 nil 而不是 0；BaselineSuspect 时一律 nil；
// Drifted 时基线归零；累计值回退时 nil 且永不为负、永不 max(0,delta)。

import (
	"context"
	"math"
	"testing"
)

// approxEqual 用容差比较浮点差值：09 K 节原话「浮点比较用容差」。
func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// TestCostDeltaClaudeThreeTurns 覆盖 K 节「t1/t2/t3 依次喂进同一条会话，
// 差值分别约 0.9552 / 0.394589 / 0.014058」。三个数都是从真实探针的
// total_cost_usd 累计值反推出来的差：
//
//	t1: 0.9551999999999999 - 0            = 0.9552   （首轮，基线记 0）
//	t2: 1.349789            - 0.9551999999999999 = 0.3945890000000001
//	t3: 1.3638469999999998  - 1.349789    = 0.0140579999999999...
func TestCostDeltaClaudeThreeTurns(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	fixtures := []string{claudeT1JSON, claudeT2JSON, claudeT3JSON}
	want := []float64{0.9552, 0.394589, 0.014058}
	const tol = 1e-6

	for i, fx := range fixtures {
		postUser(t, svc, convID, "第"+string(rune('1'+i))+"轮")
		fs.enqueueSuccess(fx)
		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if err != nil {
			t.Fatalf("第 %d 轮失败: %v", i+1, err)
		}
		if rec.CostUSDDelta == nil {
			t.Fatalf("第 %d 轮 CostUSDDelta 为 nil，期望约 %v", i+1, want[i])
		}
		if !approxEqual(*rec.CostUSDDelta, want[i], tol) {
			t.Errorf("第 %d 轮 CostUSDDelta = %v，期望约 %v（容差 %v）", i+1, *rec.CostUSDDelta, want[i], tol)
		}
	}
}

// TestCostDeltaNilNotZeroWhenProviderReportsNoMoney 覆盖 F 节规则 1：
// grok / agy 根本不报金额字段 -> CostUSDDelta 必须是 nil，不是 0。
// 用 != nil 断言而不是 == 0，是因为这条规则要挡的恰恰是「用 0 冒充没有」
// 这个具体错误——0 和 nil 在数值上都读作「没有花钱」，但只有 nil 诚实。
func TestCostDeltaNilNotZeroWhenProviderReportsNoMoney(t *testing.T) {
	t.Run("grok", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentGrok)
		postUser(t, svc, convID, "问题")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueueSuccess(grokT1JSON)
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentGrok)
		if err != nil {
			t.Fatalf("Ask 失败: %v", err)
		}
		if rec.CostUSDDelta != nil {
			t.Errorf("grok 的 CostUSDDelta 应当是 nil，实际 %v", *rec.CostUSDDelta)
		}
	})

	t.Run("agy", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentAgy)
		postUser(t, svc, convID, "问题")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueueSuccess(agyT1JSON)
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentAgy)
		if err != nil {
			t.Fatalf("Ask 失败: %v", err)
		}
		if rec.CostUSDDelta != nil {
			t.Errorf("agy 的 CostUSDDelta 应当是 nil，实际 %v", *rec.CostUSDDelta)
		}
	})
}

// TestCostDeltaNilWhenBaselineSuspect 覆盖 F 节规则 3：上一轮结局未知
// （超时或解析不了，那一轮可能真的计了费而我们没记上）-> 本轮 CostUSDDelta
// 一律 nil，即使这一轮本身解析得清清楚楚、累计值看起来完全合理。
func TestCostDeltaNilWhenBaselineSuspect(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	// 首轮成功，建立基线（costTotal=0.9552，haveCostTotal=true）。
	postUser(t, svc, convID, "第一轮")
	fs.enqueueSuccess(claudeT1JSON)
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}

	// 第二轮解析不出来（喂空 stdout）：结局未知，baselineSuspect 被置位。
	// rec2.BaselineSuspect 报的是「进入第二轮之前」的旧值（Ask 在 spawn 之前
	// 就把它从 st.baselineSuspect 里抄出来了，见 runner.go），此刻自然是
	// false（上一轮即第一轮是成功的）——真正被这次失败置位的是内部状态
	// r.gate(key).st.baselineSuspect，供**下一轮**读，所以直接白盒读它来确认
	// 这次失败确实留下了痕迹，而不是错误地断言 rec2 自己的这个字段。
	postUser(t, svc, convID, "第二轮，故意喂一个解析不出来的 stdout")
	fs.enqueue(spawnResult{stdout: []byte{}, exitCode: 1})
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err == nil {
		t.Fatal("第二轮应当报错（Unparsable）")
	}
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	// 本用例单 goroutine，读取时没有并发，不取 g.mu：Ask 已经串行跑完两轮，
	// 这里只是事后白盒检查它留下的内部状态。
	if !r.gate(key).st.baselineSuspect {
		t.Fatal("第二轮失败之后内部状态的 baselineSuspect 应当为真，前置条件不成立")
	}

	// 第三轮正常解析成功（claude-t2.json），但基线已经可疑
	// （rec3.BaselineSuspect 报的正是「进入这一轮时」的旧值，此刻应为 true），
	// 差值必须是 nil，即使 t2 的累计值（1.349789）看起来完全合理、
	// 大于 t1 的累计值。
	postUser(t, svc, convID, "第三轮")
	fs.enqueueSuccess(claudeT2JSON)
	rec3, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第三轮失败: %v", err)
	}
	if !rec3.BaselineSuspect {
		t.Error("第三轮的 Record.BaselineSuspect 应当为真（反映进入这一轮之前、上一轮失败留下的可疑标记）")
	}
	if rec3.CostUSDDelta != nil {
		t.Errorf("基线可疑之后那一轮 CostUSDDelta 应当是 nil，实际 %v", *rec3.CostUSDDelta)
	}
	// 第三轮成功之后，commitSession 会把内部状态的 baselineSuspect 清回
	// false，供第四轮读——这里用内部状态验证，不与上面 rec3 字段的语义混淆。
	// 本用例单 goroutine，读取时没有并发，不取 g.mu。
	if r.gate(key).st.baselineSuspect {
		t.Error("第三轮成功之后内部状态的 baselineSuspect 应当被清掉，供下一轮读")
	}
}

// TestCostDeltaResetToZeroBaselineOnDrift 覆盖 F 节规则 4：Drifted 为真
// （回传 id 与送出去的不同）-> 基线先归零再算，本轮差值即累计值，
// 不做这一步的话差值会是一个负数（0.955 - 1.364 这种）。
//
// 用 grok 首轮天然的 Drifted（BuildTurn 现铸的随机 uuid 不可能等于探针
// 里固定的 sessionId，见 runner_session_test.go 的同款推理）来构造，
// 不需要另外编造。grok 首轮同时也是 CostBasisNone，所以这里改用两次 claude
// 调用、白盒篡改旧 id 的方式，让「有金额」与「漂移」同时成立。
func TestCostDeltaResetToZeroBaselineOnDrift(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	postUser(t, svc, convID, "第一轮")
	fs.enqueueSuccess(claudeT1JSON) // 建立基线 costTotal=0.9552
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}

	// 白盒改动已存的旧 id，让下一轮判定为 Drifted。
	// 本用例单 goroutine，预置时没有并发，不取 g.mu。
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	r.gate(key).st.providerSession = "stale-id-that-differs-from-t2-session-id"

	postUser(t, svc, convID, "第二轮，故意漂移")
	fs.enqueueSuccess(claudeT2JSON) // 累计值 1.349789，小于 0.955 就会是负数（若不归零）
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}
	if !rec.Drifted {
		t.Fatal("这一轮应当被判为 Drifted，前置条件不成立")
	}
	if rec.CostUSDDelta == nil {
		t.Fatal("Drifted 时 CostUSDDelta 不应当是 nil：基线归零之后差值即本轮累计值")
	}
	const tol = 1e-6
	if !approxEqual(*rec.CostUSDDelta, 1.349789, tol) {
		t.Errorf("Drifted 时 CostUSDDelta = %v，期望约等于本轮累计值 1.349789（基线已归零）", *rec.CostUSDDelta)
	}
	if *rec.CostUSDDelta < 0 {
		t.Errorf("CostUSDDelta 不许为负，实际 %v", *rec.CostUSDDelta)
	}
}

// TestCostDeltaNilWhenCumulativeRegresses 覆盖 F 节规则 5：本轮累计值 <
// 基线（provider 侧把会话重置了，我们没察觉）-> CostUSDDelta 必须是 nil，
// 且永不写负数、永不 max(0, delta)——用同一份 claude-t2.json 建一个偏高的
// 假基线（白盒直接改 sessionState.costTotal），验证「累计值回退」这条路径，
// 不需要等一份真实会自动重置的探针，这条判据本身就是纯算术，可以直接构造。
func TestCostDeltaNilWhenCumulativeRegresses(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	postUser(t, svc, convID, "第一轮")
	fs.enqueueSuccess(claudeT1JSON) // 建立基线 costTotal=0.9552
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}

	// 白盒把基线人为抬高到比 t2 的累计值还大，模拟「provider 侧重置了会话，
	// 我们没察觉」（Drifted 判据没有命中——id 仍是同一个）。
	// 本用例单 goroutine，预置时没有并发，不取 g.mu。
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	r.gate(key).st.costTotal = 999.0
	r.gate(key).st.haveCostTotal = true

	postUser(t, svc, convID, "第二轮")
	fs.enqueueSuccess(claudeT2JSON) // 累计值只有 1.349789，远小于人为抬高的基线 999
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}
	if rec.Drifted {
		t.Fatal("这一轮不该被判为 Drifted（id 没变），前置条件不成立")
	}
	if rec.CostUSDDelta != nil {
		t.Errorf("累计值回退时 CostUSDDelta 应当是 nil，实际 %v（绝不能是负数，也绝不能是 max(0,delta) 的 0）", *rec.CostUSDDelta)
	}
}
