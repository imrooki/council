package headless

// B1：预算用独立的保守口径。claude 的 delta 为 nil 时 costOf 会折成 0，
// 但预算必须继续看到「累计值相对上一基线的增长」，否则一次失败轮之后就
// 再也追不上真实花费。

import (
	"math"
	"testing"

	"council/internal/provider"
	"council/internal/turn"
)

func b1Ptr(v float64) *float64 { return &v }

func b1Usage(total float64) provider.Usage {
	return provider.Usage{CostUSDTotal: &total}
}

func TestB1BudgetTrackerCountsNilDeltaConservatively(t *testing.T) {
	var b budgetTracker
	got := b.add(turn.Record{
		AgentID:      "alice",
		CostUSDDelta: b1Ptr(0.3),
		Usage:        b1Usage(0.3),
	})
	if math.Abs(got-0.3) > 1e-9 {
		t.Fatalf("第一轮 delta = %v，期望 0.3", got)
	}

	// 失败轮 / 基线可疑：delta 为 nil，但累计值是 0.9，预算应看到 0.6。
	got = b.add(turn.Record{AgentID: "alice", Usage: b1Usage(0.9)})
	if math.Abs(got-0.6) > 1e-9 {
		t.Fatalf("nil delta 的增量 = %v，期望 0.6（0.9 − 0.3）", got)
	}

	// provider 侧重置：累计值比基线小，整笔算进来（保守，可能多算）。
	got = b.add(turn.Record{AgentID: "alice", Usage: b1Usage(0.2)})
	if math.Abs(got-0.2) > 1e-9 {
		t.Fatalf("重置后的增量 = %v，期望 0.2", got)
	}

	// 恢复可信 delta 后继续按同一基线累计。
	got = b.add(turn.Record{AgentID: "alice", CostUSDDelta: b1Ptr(0.1), Usage: b1Usage(0.3)})
	if math.Abs(got-0.1) > 1e-9 {
		t.Fatalf("恢复 delta 后的增量 = %v，期望 0.1", got)
	}
}
