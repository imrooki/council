// x35b_sample_test.go 覆盖冻结契约 X-35b（预报只拿「成功轮」当缩放样本）第 7 节验收清单。
//
// pickUsageSample 是包内未导出的纯函数，这里直接喂 []conversation.TurnUsage，
// 顺序与调用约定一致：最新的在前。每条都按返回的 TurnID 断言，不只看 ok ——
// 只看 ok 的话，「选中了失败轮 A」与「选中了成功轮 B」都是 true，挡不住要害。
//
// 失败轮按真实情况造：InputTokens 指向 0。provider.go 记的 2026-09-20 失败探针里
// 失败输出的 usage 五项都是货真价实的 0，Runner 在失败返回前照样吸收落库，
// 所以 SQL 那一侧 input_tokens IS NOT NULL 拦不住它；造成 nil 就测不到本刀的病。
package turn

import (
	"reflect"
	"testing"

	"council/internal/conversation"
	"council/internal/provider"
)

// x35bInt 返回指向 v 的指针。失败轮的 &0 与成功轮的正数都靠它造。
func x35bInt(v int) *int { return &v }

// x35bRow 造一条「合格行」：InputTokens 非 nil、PromptBytes > 0。
// 合格性写死在这里，是为了让每条用例里唯一变化的是 Outcome 与 ColdStart，
// 否则某条行被跳过的原因可能是字节或 nil，而不是结局，用例就测偏了。
func x35bRow(id string, o Outcome, tokens int, cold bool) conversation.TurnUsage {
	return conversation.TurnUsage{
		TurnID:      id,
		Outcome:     int(o),
		InputTokens: x35bInt(tokens),
		PromptBytes: 4096,
		ColdStart:   cold,
	}
}

// x35bCheck 统一断言：ok 与 TurnID 都要对；ok=false 时返回值必须是零值。
// 零值也要查：一个实现可能返回 (某行, false)，调用方今天先看 found，
// 但契约写的是 (零值, false)，不钉住就放过一个半对的实现。
func x35bCheck(t *testing.T, got conversation.TurnUsage, ok bool, wantOK bool, wantID string) {
	t.Helper()
	if ok != wantOK {
		t.Fatalf("ok = %v，期望 %v（返回 TurnID=%q）", ok, wantOK, got.TurnID)
	}
	if !wantOK {
		if !reflect.DeepEqual(got, conversation.TurnUsage{}) {
			t.Fatalf("ok=false 但返回值不是零值：TurnID=%q Outcome=%d", got.TurnID, got.Outcome)
		}
		return
	}
	if got.TurnID != wantID {
		t.Fatalf("返回 TurnID=%q（Outcome=%d），期望 %q", got.TurnID, got.Outcome, wantID)
	}
}

// 第 1 条（要害，SupportYes）：最新的 ProviderFailed 行冷暖与预报相同（都为暖），
// 只有 Ingested 闸能把它挡下。改前实现返回 A。
func TestX35bSupportYesSkipsProviderFailed(t *testing.T) {
	samples := []conversation.TurnUsage{
		x35bRow("A", OutcomeProviderFailed, 0, false),
		x35bRow("B", OutcomeSpoke, 1200, false),
	}
	got, ok := pickUsageSample(samples, provider.SupportYes, false)
	x35bCheck(t, got, ok, true, "B")
}

// 第 2 条：空会话 id 那条失败路径落的是 Unparsable，形状相同。改前实现返回 A。
func TestX35bSupportYesSkipsUnparsable(t *testing.T) {
	samples := []conversation.TurnUsage{
		x35bRow("A", OutcomeUnparsable, 0, false),
		x35bRow("B", OutcomeSpoke, 1200, false),
	}
	got, ok := pickUsageSample(samples, provider.SupportYes, false)
	x35bCheck(t, got, ok, true, "B")
}

// 第 3 条（要害，SupportNo）：SupportNo 不看冷暖，失败轮直接被选中。
// 只给 SupportYes 加闸的实现在这里仍返回 A。
func TestX35bSupportNoSkipsProviderFailed(t *testing.T) {
	samples := []conversation.TurnUsage{
		x35bRow("A", OutcomeProviderFailed, 0, false),
		x35bRow("B", OutcomeSpoke, 1200, false),
	}
	got, ok := pickUsageSample(samples, provider.SupportNo, false)
	x35bCheck(t, got, ok, true, "B")
}

// 第 4 条：PostRejected 是 Ingested，算样本。唯一一条样本就是它，
// 把判据写成「只要 Spoke」的实现在这里返回 (零值, false)。
func TestX35bSupportYesAcceptsPostRejected(t *testing.T) {
	samples := []conversation.TurnUsage{
		x35bRow("PR", OutcomePostRejected, 900, true),
	}
	got, ok := pickUsageSample(samples, provider.SupportYes, true)
	x35bCheck(t, got, ok, true, "PR")
}

// 第 5 条（覆盖分支）：SupportNo 这一支同样收 PostRejected。
func TestX35bSupportNoAcceptsPostRejected(t *testing.T) {
	samples := []conversation.TurnUsage{
		x35bRow("PR", OutcomePostRejected, 900, false),
	}
	got, ok := pickUsageSample(samples, provider.SupportNo, false)
	x35bCheck(t, got, ok, true, "PR")
}

// 第 6 条：只有失败轮。cold=false 且该行 ColdStart=false 是有意的 ——
// 写成 cold=true 的话，冷暖比较本来就会把它滤掉，改前也返回 false，成了空用例。
func TestX35bOnlyFailedTurnYieldsNoSample(t *testing.T) {
	samples := []conversation.TurnUsage{
		x35bRow("A", OutcomeProviderFailed, 0, false),
	}
	got, ok := pickUsageSample(samples, provider.SupportYes, false)
	x35bCheck(t, got, ok, false, "")
}

// 第 7 条：判据是 Ingested() 而不是黑名单。Timeout 与零值 OutcomeUnknown
// 都不在「ProviderFailed、Unparsable」黑名单里，黑名单实现会选中它们。
func TestX35bNonIngestedOutsideBlacklistRejected(t *testing.T) {
	cases := []struct {
		name string
		o    Outcome
	}{
		{"Timeout", OutcomeTimeout},
		{"UnknownZeroValue", OutcomeUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			samples := []conversation.TurnUsage{
				x35bRow("X", c.o, 700, false),
			}
			got, ok := pickUsageSample(samples, provider.SupportYes, false)
			x35bCheck(t, got, ok, false, "")
		})
	}
}

// 第 7 条的延伸（清单外，按 Ingested() 的定义推出）：枚举 0..9 每个结局，
// 两支都只收 Spoke 与 PostRejected。期望值手写，不调 Ingested() 算 ——
// 用被测判据去算期望，断言就恒真了。
func TestX35bEveryOutcomeBothBranches(t *testing.T) {
	type want struct {
		o      Outcome
		accept bool
	}
	table := []want{
		{OutcomeUnknown, false},
		{OutcomeSpoke, true},
		{OutcomePostRejected, true},
		{OutcomeProviderFailed, false},
		{OutcomeUnparsable, false},
		{OutcomeTimeout, false},
		{OutcomeSpawnFailed, false},
		{OutcomeRefused, false},
		{OutcomeCanceled, false},
		{OutcomeLost, false},
	}
	for _, cache := range []provider.Support{provider.SupportYes, provider.SupportNo} {
		for _, w := range table {
			samples := []conversation.TurnUsage{
				x35bRow("X", w.o, 700, false),
			}
			got, ok := pickUsageSample(samples, cache, false)
			wantID := ""
			if w.accept {
				wantID = "X"
			}
			if ok != w.accept || got.TurnID != wantID {
				t.Errorf("cache=%d outcome=%d：得到 (TurnID=%q, ok=%v)，期望 (TurnID=%q, ok=%v)",
					cache, w.o, got.TurnID, ok, wantID, w.accept)
			}
		}
	}
}

// 第 8 条（回归钉，改前改后都成立）：SupportUnknown 不取样，哪怕样本里有合格的 Spoke 行。
func TestX35bSupportUnknownStillNoSample(t *testing.T) {
	samples := []conversation.TurnUsage{
		x35bRow("S", OutcomeSpoke, 1200, false),
	}
	for _, cold := range []bool{false, true} {
		got, ok := pickUsageSample(samples, provider.SupportUnknown, cold)
		x35bCheck(t, got, ok, false, "")
	}
}

// 第 9 条（回归钉，改前改后都成立）：InputTokens 为 nil 或 PromptBytes <= 0 的 Spoke 行仍跳过，
// 两支都走到更旧的合格行 G。
func TestX35bUnscalableSpokeStillSkipped(t *testing.T) {
	nilTokens := x35bRow("NIL", OutcomeSpoke, 0, false)
	nilTokens.InputTokens = nil
	zeroBytes := x35bRow("ZERO", OutcomeSpoke, 1000, false)
	zeroBytes.PromptBytes = 0
	negBytes := x35bRow("NEG", OutcomeSpoke, 1000, false)
	negBytes.PromptBytes = -1
	good := x35bRow("G", OutcomeSpoke, 1000, false)

	samples := []conversation.TurnUsage{nilTokens, zeroBytes, negBytes, good}
	for _, cache := range []provider.Support{provider.SupportYes, provider.SupportNo} {
		got, ok := pickUsageSample(samples, cache, false)
		x35bCheck(t, got, ok, true, "G")
	}
}
