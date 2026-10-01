package repl

import (
	"strings"
	"testing"

	"council/internal/provider"
	"council/internal/turn"
)

// 本文件是 X-38a 契约第 4 节 B 组验收：writeAskMeter 的用量行认「provider 没报」。
// 验收判据一律是以「用量 」开头的那一行整行相等；另有少量对整段输出的子串检查，
// 只用于夹具自检或补充断言（例如垃圾值不许出现在输出里）。
// newOutREPL 与 numericTokens 来自 ask_test.go。

func x38aPtr(v int) *int { return &v }

// x38aRun 跑一次 writeAskMeter，返回整段输出。
func x38aRun(t *testing.T, rec turn.Record) string {
	t.Helper()
	r, buf := newOutREPL()
	if err := writeAskMeter(r, rec); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	return buf.String()
}

// x38aUsageLine 取出以「用量 」开头的那一行；恰好一行，否则判失败。
func x38aUsageLine(t *testing.T, label, out string) string {
	t.Helper()
	var hits []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "用量 ") {
			hits = append(hits, line)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("%s: 以「用量 」开头的行应恰好一行，实际 %d 行；完整输出:\n%s", label, len(hits), out)
	}
	return hits[0]
}

// x38aExpectLine 断言 rec 的用量行整行等于 want，返回整段输出。
func x38aExpectLine(t *testing.T, label string, rec turn.Record, want string) string {
	t.Helper()
	out := x38aRun(t, rec)
	if got := x38aUsageLine(t, label, out); got != want {
		t.Errorf("%s: 用量行\n  实际 %q\n  应为 %q\n完整输出:\n%s", label, got, want, out)
	}
	return out
}

// x38aRec 组一个 UsageParsed 为真的 Record。
func x38aRec(oc turn.Outcome, u provider.Usage) turn.Record {
	return turn.Record{Outcome: oc, UsageParsed: true, Usage: u}
}

// x38aA1Usage 是 A1 的五个互不相同的非零值。
func x38aA1Usage(unreported []string) provider.Usage {
	return provider.Usage{InputTokens: 11, CacheCreate: x38aPtr(22), CacheRead: 33, OutputTokens: 44,
		ThinkingTokens: 55, Unreported: unreported}
}

var x38aAllNames = []string{"input_tokens", "cache_create", "cache_read", "output_tokens", "thinking_tokens"}

const (
	x38aB1Line      = "用量 in=20625 cache_create=? cache_read=20608 out=134 thinking=5"
	x38aB1CtlLine   = "用量 in=20625 cache_create=0 cache_read=20608 out=134 thinking=5"
	x38aAllQLine    = "用量 in=? cache_create=? cache_read=? out=? thinking=?"
	x38aUnknownLine = "用量 未知（没解析到用量）"
)

func x38aGrokUsage() provider.Usage {
	return provider.Usage{InputTokens: 20625, CacheCreate: x38aPtr(0), CacheRead: 20608, OutputTokens: 134,
		ThinkingTokens: 5, Unreported: []string{"cache_create"}}
}

// ============================================================
// B1. grok 形状：cache_create 标了未报 → ?；对照去掉 Unreported → 0
// ============================================================

func TestX38aMeterGrokCacheCreateUnreported(t *testing.T) {
	x38aExpectLine(t, "B1", x38aRec(turn.OutcomeSpoke, x38aGrokUsage()), x38aB1Line)

	ctl := x38aGrokUsage()
	ctl.Unreported = nil
	x38aExpectLine(t, "B1 对照（去掉 Unreported）", x38aRec(turn.OutcomeSpoke, ctl), x38aB1CtlLine)
}

// B1 经 /round 那条路：writeAskRecord -> writeAskMeter，同一行。
func TestX38aAskRecordGrokCacheCreateUnreported(t *testing.T) {
	run := func(rec turn.Record) string {
		r, buf := newOutREPL()
		if err := writeAskRecord(r, rec); err != nil {
			t.Fatalf("writeAskRecord 失败: %v", err)
		}
		return buf.String()
	}
	rec := x38aRec(turn.OutcomeSpoke, x38aGrokUsage())
	if got := x38aUsageLine(t, "B1 writeAskRecord", run(rec)); got != x38aB1Line {
		t.Errorf("B1 writeAskRecord: 用量行 %q，应为 %q", got, x38aB1Line)
	}
	ctl := x38aGrokUsage()
	ctl.Unreported = nil
	if got := x38aUsageLine(t, "B1 writeAskRecord 对照", run(x38aRec(turn.OutcomeSpoke, ctl))); got != x38aB1CtlLine {
		t.Errorf("B1 writeAskRecord 对照: 用量行 %q，应为 %q", got, x38aB1CtlLine)
	}
}

// ============================================================
// B2. usage 对象缺失：五个全列进 Unreported、数值塞垃圾 → 五个 ?，垃圾不出现
// ============================================================

func x38aB2Usage(unreported []string) provider.Usage {
	return provider.Usage{InputTokens: 4242, CacheCreate: x38aPtr(4343), CacheRead: 4444, OutputTokens: 4545,
		ThinkingTokens: 4646, Unreported: unreported}
}

func TestX38aMeterAllUnreportedPrintsQuestionMarks(t *testing.T) {
	garbage := []string{"4242", "4343", "4444", "4545", "4646"}
	// 计时三格给互不相同的非零值，于是「输出里的每个数字都来自夹具」可以精确判。
	rec := x38aRec(turn.OutcomeSpoke, x38aB2Usage(x38aAllNames))
	rec.WallMS, rec.PromptBytes, rec.ScannedTo = 7001, 7002, 7003
	out := x38aExpectLine(t, "B2", rec, x38aAllQLine)
	for _, g := range garbage {
		if strings.Contains(out, g) {
			t.Errorf("B2: 输出不应含垃圾值 %s；完整输出:\n%s", g, out)
		}
	}
	allowed := map[string]bool{"7001": true, "7002": true, "7003": true}
	for _, tok := range numericTokens(out) {
		if !allowed[tok] {
			t.Errorf("B2: 输出里出现了计时三格之外的数字 %q；完整输出:\n%s", tok, out)
		}
	}

	// 对照（只改 Unreported）：五个垃圾值全照打，证明上面的「不含」能为假。
	ctl := rec
	ctl.Usage = x38aB2Usage(nil)
	outCtl := x38aExpectLine(t, "B2 对照（Unreported 空）", ctl,
		"用量 in=4242 cache_create=4343 cache_read=4444 out=4545 thinking=4646")
	for _, g := range garbage {
		if !strings.Contains(outCtl, g) {
			t.Errorf("B2 对照: 输出应含 %s；完整输出:\n%s", g, outCtl)
		}
	}
}

// ============================================================
// B3. 逐个指标：只那一格是 ?
// ============================================================

func TestX38aMeterSingleMetricUnreported(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"input_tokens", "用量 in=? cache_create=22 cache_read=33 out=44 thinking=55"},
		{"cache_read", "用量 in=11 cache_create=22 cache_read=? out=44 thinking=55"},
		{"output_tokens", "用量 in=11 cache_create=22 cache_read=33 out=? thinking=55"},
		{"thinking_tokens", "用量 in=11 cache_create=22 cache_read=33 out=44 thinking=?"},
	}
	for _, c := range cases {
		x38aExpectLine(t, "B3 "+c.name, x38aRec(turn.OutcomeSpoke, x38aA1Usage([]string{c.name})), c.want)
	}
	// 对照：同一夹具 Unreported 空 → 五格全是数。
	x38aExpectLine(t, "B3 对照（Unreported 空）", x38aRec(turn.OutcomeSpoke, x38aA1Usage(nil)),
		"用量 in=11 cache_create=22 cache_read=33 out=44 thinking=55")
}

// ============================================================
// B4. Unreported 空、CacheCreate nil → cache_create=?
// ============================================================

func TestX38aMeterCacheCreateNilPrintsQuestionMark(t *testing.T) {
	u := x38aA1Usage(nil)
	u.CacheCreate = nil
	x38aExpectLine(t, "B4", x38aRec(turn.OutcomeSpoke, u),
		"用量 in=11 cache_create=? cache_read=33 out=44 thinking=55")

	// 对照（只改 CacheCreate）：&22 → cache_create=22。
	x38aExpectLine(t, "B4 对照（CacheCreate=&22）", x38aRec(turn.OutcomeSpoke, x38aA1Usage(nil)),
		"用量 in=11 cache_create=22 cache_read=33 out=44 thinking=55")
}

// ============================================================
// B5. 全都报了：与本刀之前逐字节相同（回归钉）
// ============================================================

func TestX38aMeterAllReportedUnchanged(t *testing.T) {
	u := provider.Usage{InputTokens: 48151, CacheCreate: x38aPtr(7713), CacheRead: 12013, OutputTokens: 55511,
		ThinkingTokens: 4201}
	x38aExpectLine(t, "B5", x38aRec(turn.OutcomeSpoke, u),
		"用量 in=48151 cache_create=7713 cache_read=12013 out=55511 thinking=4201")
}

// ============================================================
// B6. 没解析到用量 → 未知（回归钉）
// ============================================================

func TestX38aMeterUnparsedStillUnknown(t *testing.T) {
	rec := turn.Record{Outcome: turn.OutcomeTimeout, UsageParsed: false, Usage: x38aA1Usage(nil)}
	out := x38aExpectLine(t, "B6", rec, x38aUnknownLine)
	for _, g := range []string{"11", "22", "33", "44", "55"} {
		if strings.Contains(out, g) {
			t.Errorf("B6: 未解析时不应打出夹具数字 %s；完整输出:\n%s", g, out)
		}
	}
	// 对照（只翻 UsageParsed）：用量行变成五格数字。
	ctl := rec
	ctl.UsageParsed = true
	x38aExpectLine(t, "B6 对照（UsageParsed=true）", ctl,
		"用量 in=11 cache_create=22 cache_read=33 out=44 thinking=55")
	// 对照（Unreported 全列、未解析）：仍是未知，不是五个 ?。
	ctl2 := rec
	ctl2.Usage = x38aA1Usage(x38aAllNames)
	x38aExpectLine(t, "B6 对照（未解析、五个全列）", ctl2, x38aUnknownLine)
}

// ============================================================
// B7. 结局不影响用量行
// ============================================================

func TestX38aMeterOutcomeDoesNotGateUsageLine(t *testing.T) {
	spoke := x38aRec(turn.OutcomeSpoke, x38aGrokUsage())
	failed := x38aRec(turn.OutcomeProviderFailed, x38aGrokUsage())
	outSpoke := x38aExpectLine(t, "B7 Spoke", spoke, x38aB1Line)
	outFailed := x38aExpectLine(t, "B7 ProviderFailed", failed, x38aB1Line)
	// 证明结局这个字段确实送到了函数里：冷暖行在两种结局下不同（Spoke 实测已知，ProviderFailed 未知）。
	if outSpoke == outFailed {
		t.Errorf("B7: 换了结局整段输出却一样，夹具的结局没有生效；输出:\n%s", outSpoke)
	}
	if !strings.Contains(outFailed, "实测=未知") || strings.Contains(outSpoke, "实测=未知") {
		t.Errorf("B7: 冷暖行应只在 ProviderFailed 下为未知；Spoke:\n%s\nProviderFailed:\n%s", outSpoke, outFailed)
	}

	// ProviderFailed、去掉 Unreported → cache_create=0（报了的 0 在失败轮也照打）。
	ctl := x38aGrokUsage()
	ctl.Unreported = nil
	x38aExpectLine(t, "B7 对照（ProviderFailed、去掉 Unreported）", x38aRec(turn.OutcomeProviderFailed, ctl), x38aB1CtlLine)
}

// ============================================================
// B8. 报了的真实 0 印 0
// ============================================================

func TestX38aMeterReportedZerosPrintZero(t *testing.T) {
	zeros := provider.Usage{InputTokens: 0, CacheCreate: x38aPtr(0), CacheRead: 0, OutputTokens: 0, ThinkingTokens: 0}
	rec := x38aRec(turn.OutcomeProviderFailed, zeros)
	x38aExpectLine(t, "B8", rec, "用量 in=0 cache_create=0 cache_read=0 out=0 thinking=0")

	// 对照（只改 Unreported，五个名字全列）→ 五个 ?。
	ctl := rec
	ctl.Usage.Unreported = x38aAllNames
	x38aExpectLine(t, "B8 对照（五个全列）", ctl, x38aAllQLine)

	// 同一组 0 在 Spoke 下也印 0（与结局无关）。
	x38aExpectLine(t, "B8 Spoke", x38aRec(turn.OutcomeSpoke, zeros), "用量 in=0 cache_create=0 cache_read=0 out=0 thinking=0")
}
