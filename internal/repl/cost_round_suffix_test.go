package repl

// P2：/cost --round 支持界面上显示的末 6 位后缀；歧义时报错而不是合并两轮。

import (
	"testing"

	"council/internal/conversation"
)

func TestFilterCostByRoundSuffix(t *testing.T) {
	rows := []conversation.TurnUsage{
		{RoundID: "round-abcdef123456", AgentID: "a"},
		{RoundID: "round-zzz", AgentID: "b"},
	}
	got, err := filterCostByRound(rows, "123456")
	if err != nil || len(got) != 1 || got[0].AgentID != "a" {
		t.Fatalf("后缀匹配 = %+v, err=%v", got, err)
	}

	ambiguous := []conversation.TurnUsage{
		{RoundID: "aaa-456", AgentID: "a"},
		{RoundID: "bbb-456", AgentID: "b"},
	}
	if _, err := filterCostByRound(ambiguous, "456"); err == nil {
		t.Fatal("后缀命中两个不同轮次时应报错")
	}
}
