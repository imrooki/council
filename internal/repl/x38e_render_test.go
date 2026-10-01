package repl

// 本文件是 X-38e 冻结契约第 2 节 R1-R3 的验收：/round 屏幕的用量限定语
// 跟着 Settle 的新口径（五个用量全无数才算用量未知）走，措辞改成
// 「k 名成员用量未知，合计只含其余人」。
//
// internal/repl 回放不了夹具（Runner.spawn 未导出，fakeSpawner 在 turn 包的
// 测试文件里），所以手搓 turn.Record，经真 turn.Settle 再交给 renderSettlement。
// 每条新行为断言都配一条只差一处的对照（R1 对 R1c，R2 对 R2c）。

import (
	"testing"

	"council/internal/provider"
	"council/internal/turn"
)

// ---------------------------------------------------------------------------
// R1. 真 Settle 进 renderSettlement，整段输出冻结（HEAD 红；R1c HEAD 绿）
// ---------------------------------------------------------------------------

func TestX38eRenderSettlementThroughRealSettle(t *testing.T) {
	five := []string{"cache_create", "input_tokens", "cache_read", "output_tokens", "thinking_tokens"}
	grokOK := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true, Cold: true,
		Usage: provider.Usage{InputTokens: 20625, CacheCreate: nil, CacheRead: 0, OutputTokens: 723, ThinkingTokens: 722, Unreported: []string{"cache_create"}}}
	noUsage := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true,
		Usage: provider.Usage{Unreported: append([]string(nil), five...)}}
	envelope := turn.Record{Outcome: turn.OutcomeProviderFailed, UsageParsed: true,
		Usage: provider.Usage{Unreported: append([]string(nil), five...)}}

	out := renderSettlement(turn.Settle([]turn.Record{grokOK, noUsage, envelope}), 1, nil, "发言轮")
	const want = "发言轮结算\n确知起进程 3\n分不清进程是否起来 0\n用量 输入 20625  输出 723  cache_read 0  cache_create 未知  thinking 722（2 名成员用量未知，合计只含其余人）\n金额 未知（3 名成员未报金额）\n冷启动 预报 1  实测 1（2 名成员冷暖未知，只数其余人）\n"
	if out != want {
		t.Errorf("R1 整段输出不符\n实际: %q\n期望: %q", out, want)
	}

	// R1c：noUsage 与 envelope 各只把 Unreported 换成 [cache_create]，其余不变。
	// 它们报了四列（值都是 0），不是用量未知，用量行不带限定语。
	noUsageC := noUsage
	noUsageC.Usage.Unreported = []string{"cache_create"}
	envelopeC := envelope
	envelopeC.Usage.Unreported = []string{"cache_create"}
	outC := renderSettlement(turn.Settle([]turn.Record{grokOK, noUsageC, envelopeC}), 1, nil, "发言轮")
	const wantC = "发言轮结算\n确知起进程 3\n分不清进程是否起来 0\n用量 输入 20625  输出 723  cache_read 0  cache_create 未知  thinking 722\n金额 未知（3 名成员未报金额）\n冷启动 预报 1  实测 1（1 名成员冷暖未知，只数其余人）\n"
	if outC != wantC {
		t.Errorf("R1c 整段输出不符\n实际: %q\n期望: %q", outC, wantC)
	}
}

// ---------------------------------------------------------------------------
// R2. 限定语整句（HEAD 红；R2c HEAD 绿）
// ---------------------------------------------------------------------------

func TestX38eRenderSettlementNoteWording(t *testing.T) {
	out := renderSettlement(turn.RoundSettlement{Spawned: 3, UsageUnknown: 1, InputTokens: 100, HaveIn: true}, 0, nil, "发言轮")
	const want = "发言轮结算\n确知起进程 3\n分不清进程是否起来 0\n用量 输入 100  输出 未知  cache_read 未知  cache_create 未知  thinking 未知（1 名成员用量未知，合计只含其余人）\n金额 未知\n冷启动 预报 0  实测 未知\n"
	if out != want {
		t.Errorf("R2 整段输出不符\n实际: %q\n期望: %q", out, want)
	}

	outC := renderSettlement(turn.RoundSettlement{Spawned: 3, UsageUnknown: 0, InputTokens: 100, HaveIn: true}, 0, nil, "发言轮")
	const wantC = "发言轮结算\n确知起进程 3\n分不清进程是否起来 0\n用量 输入 100  输出 未知  cache_read 未知  cache_create 未知  thinking 未知\n金额 未知\n冷启动 预报 0  实测 未知\n"
	if outC != wantC {
		t.Errorf("R2c 整段输出不符\n实际: %q\n期望: %q", outC, wantC)
	}
}

// ---------------------------------------------------------------------------
// R3. usageUnknownNote 直调（HEAD 上前两条红，后两条绿）
// ---------------------------------------------------------------------------

func TestX38eUsageUnknownNoteLiteral(t *testing.T) {
	cases := []struct {
		k    int
		want string
	}{
		{1, "（1 名成员用量未知，合计只含其余人）"},
		{2, "（2 名成员用量未知，合计只含其余人）"},
		{0, ""},
		{-1, ""},
	}
	for _, c := range cases {
		if got := usageUnknownNote(c.k); got != c.want {
			t.Errorf("usageUnknownNote(%d) = %q，期望 %q", c.k, got, c.want)
		}
	}
}
