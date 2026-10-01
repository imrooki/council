package turn

// step10a4_outcome_lost_test.go 覆盖 Step 10 A 片之四冻结契约第 6 节
// 验收清单第 10 条：OutcomeLost 的值钉死为 9，且 Ingested() == false
// （丢失的轮没有把 delta 送达，游标绝不能推）。

import "testing"

func TestOutcomeLostValueAndIngested(t *testing.T) {
	if int(OutcomeLost) != 9 {
		t.Fatalf("OutcomeLost = %d，期望 9（turn_usage.outcome 存的是整数，"+
			"值必须钉死为字面量，不许接着 iota 往下数）", int(OutcomeLost))
	}
	if OutcomeLost.Ingested() {
		t.Error("OutcomeLost.Ingested() 应当是 false：丢失的轮没有把 delta 送达，游标绝不能推")
	}
}
