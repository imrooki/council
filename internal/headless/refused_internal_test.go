package headless

import (
	"errors"
	"testing"

	"council/internal/turn"
)

// TestAllRefusedWithErr_TurnID判据 钉住「整轮没跑起来」与「成员级全拒」的分界。
//
// RunRoundPrepared 在 mint round id 或写 round_started 失败时预填的记录不带
// TurnID；成员一旦进了执行，即使被 FULL 权限或超长 prompt 拒绝，记录也带着
// TurnID。这条判据宽一格就会把成员级失败提成 error，窄一格就会把真故障
// 降级成 partial，所以两种形状都要有用例。
func TestAllRefusedWithErr_TurnID判据(t *testing.T) {
	boom := errors.New("boom")

	cases := []struct {
		name string
		recs []turn.Record
		want bool
	}{
		{
			name: "空切片不算整轮失败",
			recs: nil,
			want: false,
		},
		{
			name: "整轮预填：全 Refused、带 Err、没有 TurnID",
			recs: []turn.Record{
				{AgentID: "c1", Outcome: turn.OutcomeRefused, Err: boom},
				{AgentID: "c3", Outcome: turn.OutcomeRefused, Err: boom},
			},
			want: true,
		},
		{
			name: "成员级拒绝：带 TurnID 就不是整轮失败",
			recs: []turn.Record{
				{AgentID: "c1", TurnID: "t1", Outcome: turn.OutcomeRefused, Err: boom},
				{AgentID: "c3", TurnID: "t2", Outcome: turn.OutcomeRefused, Err: boom},
			},
			want: false,
		},
		{
			name: "混入一条成功也不是整轮失败",
			recs: []turn.Record{
				{AgentID: "c1", Outcome: turn.OutcomeRefused, Err: boom},
				{AgentID: "c3", Outcome: turn.OutcomeSpoke},
			},
			want: false,
		},
		{
			name: "Refused 但没带 Err 不算整轮失败",
			recs: []turn.Record{
				{AgentID: "c1", Outcome: turn.OutcomeRefused},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		if got := allRefusedWithErr(tc.recs); got != tc.want {
			t.Errorf("%s: allRefusedWithErr = %v，期望 %v", tc.name, got, tc.want)
		}
	}
}
