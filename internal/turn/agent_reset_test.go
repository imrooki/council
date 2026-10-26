package turn

// agent_reset_test.go 覆盖 A3 的 Runner.ForgetAgentSessions：按成员清掉
// 所有会话的内存快照、别的成员一条不动、有闸被在飞一轮持有时 fail-closed。

import (
	"errors"
	"testing"
)

// tstRunnerWithGates 造一个只带闸表的 Runner。本方法的语义只涉及 gates 与
// g.st，不需要配置、Service 或工作目录；用零值 Runner 而不是 NewRunner，
// 是为了让用例失败时只可能指向本方法。
func tstRunnerWithGates() *Runner {
	return &Runner{gates: make(map[sessionKey]*sessionGate)}
}

func TestForgetAgentSessionsClearsOnlyTargetAgent(t *testing.T) {
	r := tstRunnerWithGates()
	a1 := r.gate(sessionKey{convID: "c1", agentID: "alpha"})
	a2 := r.gate(sessionKey{convID: "c2", agentID: "alpha"})
	b1 := r.gate(sessionKey{convID: "c1", agentID: "beta"})
	a1.st = &sessionState{providerSession: "sess-a1", identityFP: "fp-old-a"}
	a2.st = &sessionState{providerSession: "sess-a2", identityFP: "fp-old-a"}
	b1.st = &sessionState{providerSession: "sess-b1", identityFP: "fp-old-b"}

	cleared, err := r.ForgetAgentSessions("alpha")
	if err != nil {
		t.Fatalf("ForgetAgentSessions(alpha) 报错: %v", err)
	}
	if cleared != 2 {
		t.Errorf("清掉的快照数 = %d，期望 2（alpha 的两条会话）", cleared)
	}
	if a1.st != nil || a2.st != nil {
		t.Errorf("alpha 的快照没清干净: c1=%+v c2=%+v", a1.st, a2.st)
	}
	if b1.st == nil || b1.st.providerSession != "sess-b1" {
		t.Errorf("beta 的快照被一起清了: %+v", b1.st)
	}
	// 闸本身绝不能从 gates 里删掉：那片注释（sessionGate 的不变量）说的就是
	// 「删了就有两条 goroutine 各建一把闸，等于没有闸」。
	if got := r.gate(sessionKey{convID: "c1", agentID: "alpha"}); got != a1 {
		t.Error("alpha 在 c1 的闸被换掉了：gate 必须永远保留，只清 st")
	}
}

func TestForgetAgentSessionsEmptyAndMissingAreNoOps(t *testing.T) {
	r := tstRunnerWithGates()
	if cleared, err := r.ForgetAgentSessions(""); err != nil || cleared != 0 {
		t.Errorf("空 id = (%d, %v)，期望 (0, nil)", cleared, err)
	}
	if cleared, err := r.ForgetAgentSessions("nobody"); err != nil || cleared != 0 {
		t.Errorf("没有快照的成员 = (%d, %v)，期望 (0, nil)", cleared, err)
	}
}

// TestForgetAgentSessionsRefusesWhenGateHeld 钉住 fail-closed：任何一把闸
// 被在飞一轮持有时整体拒绝，且**一把都不许清**——清一半会留下「两条会话
// 里一条能说话、一条撞漂移」这种最难解释的状态。
func TestForgetAgentSessionsRefusesWhenGateHeld(t *testing.T) {
	r := tstRunnerWithGates()
	free := r.gate(sessionKey{convID: "c1", agentID: "alpha"})
	busy := r.gate(sessionKey{convID: "c2", agentID: "alpha"})
	free.st = &sessionState{providerSession: "sess-free", identityFP: "fp-old"}
	busy.st = &sessionState{providerSession: "sess-busy", identityFP: "fp-old"}

	busy.mu.Lock()
	cleared, err := r.ForgetAgentSessions("alpha")
	busy.mu.Unlock()

	if !errors.Is(err, ErrTurnInFlight) {
		t.Fatalf("err = %v，期望包着 ErrTurnInFlight", err)
	}
	if cleared != 0 {
		t.Errorf("被拒绝时报告清掉 %d 条，期望 0", cleared)
	}
	if free.st == nil || busy.st == nil {
		t.Errorf("被拒绝时仍然清了快照: free=%+v busy=%+v", free.st, busy.st)
	}
}
