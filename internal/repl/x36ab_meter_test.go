package repl

import (
	"regexp"
	"strings"
	"testing"

	"council/internal/provider"
	"council/internal/turn"
)

// 本文件是 x36ab（失败轮的冷暖不是实测值）契约第 9 节 B、E 两组验收。
// newOutREPL 与 numericTokens 来自 ask_test.go。
// 每一条「新行为」断言都配一个只差一个字段的对照组，证明同一套夹具在另一种取值下
// 确实打出冷 / 暖 / 数字。

// x36abMeter 跑一次 writeAskMeter，返回整段输出。
func x36abMeter(t *testing.T, rec turn.Record) string {
	t.Helper()
	r, buf := newOutREPL()
	if err := writeAskMeter(r, rec); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	return buf.String()
}

// x36abHasLine 判断输出里有没有一整行恰好等于 want（不是子串）。
func x36abHasLine(out, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

// x36abMustContain / x36abMustNotContain 失败时把整段输出贴出来。
func x36abMustContain(t *testing.T, label, out, sub string) {
	t.Helper()
	if !strings.Contains(out, sub) {
		t.Errorf("%s: 输出应含 %q；完整输出:\n%s", label, sub, out)
	}
}

func x36abMustNotContain(t *testing.T, label, out, sub string) {
	t.Helper()
	if strings.Contains(out, sub) {
		t.Errorf("%s: 输出不应含 %q；完整输出:\n%s", label, sub, out)
	}
}

// ============================================================
// B. writeAskMeter
// ============================================================

// B1：ProviderFailed、用量已解析 → 冷暖未知，用量照打。
func TestX36abMeterProviderFailedColdUnknown(t *testing.T) {
	rec := turn.Record{Outcome: turn.OutcomeProviderFailed, UsageParsed: true, Cold: false,
		Usage: provider.Usage{InputTokens: 4242, CacheRead: 0}}
	out := x36abMeter(t, rec)
	x36abMustContain(t, "B1", out, "实测=未知")
	x36abMustNotContain(t, "B1", out, "实测=暖")
	x36abMustNotContain(t, "B1", out, "实测=冷")
	x36abMustContain(t, "B1", out, "in=4242")
	x36abMustContain(t, "B1", out, "冷暖 预报=暖 实测=未知")

	// 对照甲：结局换成 Spoke、Cold=true → 实测=冷。
	a := rec
	a.Outcome = turn.OutcomeSpoke
	a.Cold = true
	outA := x36abMeter(t, a)
	x36abMustContain(t, "B1 对照甲", outA, "实测=冷")
	x36abMustNotContain(t, "B1 对照甲", outA, "实测=未知")

	// 对照乙：Spoke、CacheRead=777、Cold=false → 实测=暖。
	b := rec
	b.Outcome = turn.OutcomeSpoke
	b.Usage.CacheRead = 777
	outB := x36abMeter(t, b)
	x36abMustContain(t, "B1 对照乙", outB, "实测=暖")
	x36abMustNotContain(t, "B1 对照乙", outB, "实测=未知")
}

// B2：Unparsable、用量已解析（空会话 id 那条路的真实形状）→ 冷暖未知，用量照打。
func TestX36abMeterUnparsableParsedColdUnknown(t *testing.T) {
	rec := turn.Record{Outcome: turn.OutcomeUnparsable, UsageParsed: true, Cold: false,
		Usage: provider.Usage{InputTokens: 5151}}
	out := x36abMeter(t, rec)
	x36abMustContain(t, "B2", out, "实测=未知")
	x36abMustNotContain(t, "B2", out, "实测=暖")
	x36abMustContain(t, "B2", out, "in=5151")

	// 对照：只把结局换成 Spoke → 实测=暖。
	c := rec
	c.Outcome = turn.OutcomeSpoke
	outC := x36abMeter(t, c)
	x36abMustContain(t, "B2 对照", outC, "实测=暖")
	x36abMustNotContain(t, "B2 对照", outC, "实测=未知")
}

func x36abB3Rec() turn.Record {
	return turn.Record{
		Outcome:     turn.OutcomeTimeout,
		UsageParsed: false,
		Usage:       provider.Usage{InputTokens: 99999}, // 故意塞垃圾数字
		WallMS:      31337,
		PromptBytes: 4099,
		ScannedTo:   5003,
	}
}

const (
	x36abUsageUnknownLine = "用量 未知（没解析到用量）"
	x36abB3ColdLine       = "冷暖 预报=暖 实测=未知  wall_ms=31337  prompt_bytes=4099  scanned_to=5003"
)

// B3：没解析到用量 → 用量整行换成未知、不打任何数字；冷暖行其余几格原样。
func TestX36abMeterUnparsedUsageLineIsUnknown(t *testing.T) {
	out := x36abMeter(t, x36abB3Rec())
	if !x36abHasLine(out, x36abUsageUnknownLine) {
		t.Errorf("B3: 输出应有整行 %q；完整输出:\n%s", x36abUsageUnknownLine, out)
	}
	x36abMustNotContain(t, "B3", out, "99999")
	x36abMustNotContain(t, "B3", out, "in=")
	if !x36abHasLine(out, x36abB3ColdLine) {
		t.Errorf("B3: 输出应有整行 %q；完整输出:\n%s", x36abB3ColdLine, out)
	}
	// 契约 3.6：新文本不许含 ASCII 数字。输出里的每个数字片段都必须来自夹具的三个计时字段。
	allowed := map[string]bool{"31337": true, "4099": true, "5003": true}
	for _, tok := range numericTokens(out) {
		if !allowed[tok] {
			t.Errorf("B3: 输出里出现了夹具之外的数字 %q；完整输出:\n%s", tok, out)
		}
	}

	// 对照甲（非真实，只改 UsageParsed）：同一个 Record 但已解析 → 照打数字。
	a := x36abB3Rec()
	a.UsageParsed = true
	outA := x36abMeter(t, a)
	x36abMustContain(t, "B3 对照甲", outA, "in=99999")
	if x36abHasLine(outA, x36abUsageUnknownLine) {
		t.Errorf("B3 对照甲: 已解析时不该印用量未知；完整输出:\n%s", outA)
	}

	// 对照乙（非真实，只改 Outcome）：Spoke、未解析 → 仍是未知、不打 99999。
	// 两个对照合起来挡「用量行按结局门控」。
	b := x36abB3Rec()
	b.Outcome = turn.OutcomeSpoke
	outB := x36abMeter(t, b)
	if !x36abHasLine(outB, x36abUsageUnknownLine) {
		t.Errorf("B3 对照乙: 输出应有整行 %q；完整输出:\n%s", x36abUsageUnknownLine, outB)
	}
	x36abMustNotContain(t, "B3 对照乙", outB, "99999")
}

// B4：cache_read 标了未报的成功轮 → 冷暖未知。本刀不断言这一例的用量行（契约 3.7）。
func TestX36abMeterCacheReadUnreportedColdUnknown(t *testing.T) {
	rec := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true, Cold: true,
		Usage: provider.Usage{Unreported: []string{"cache_read"}, CacheRead: 0}}
	out := x36abMeter(t, rec)
	x36abMustContain(t, "B4", out, "实测=未知")
	x36abMustNotContain(t, "B4", out, "实测=冷")

	// 对照：只去掉 Unreported → 实测=冷。
	c := rec
	c.Usage.Unreported = nil
	outC := x36abMeter(t, c)
	x36abMustContain(t, "B4 对照", outC, "实测=冷")
	x36abMustNotContain(t, "B4 对照", outC, "实测=未知")
}

// B5：意外冷启动行只在冷暖确知时打，判据仍是 UnexpectedCold。
func TestX36abMeterUnexpectedColdGatedByKnown(t *testing.T) {
	// 非真实：ProviderFailed 上的 UnexpectedCold=true。
	pf := turn.Record{Outcome: turn.OutcomeProviderFailed, UsageParsed: true, UnexpectedCold: true}
	x36abMustNotContain(t, "B5 ProviderFailed", x36abMeter(t, pf), "意外冷启动")

	// 对照甲：Spoke、冷、UnexpectedCold=true → 打。
	a := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true, Cold: true, UnexpectedCold: true,
		Usage: provider.Usage{CacheRead: 0}}
	x36abMustContain(t, "B5 对照甲", x36abMeter(t, a), "意外冷启动")

	// 对照乙：新会话第一轮的真实形状，预报冷、实测冷、UnexpectedCold=false → 不打。
	// 挡「把意外冷启动的判据换成 known && cold」。
	b := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true, Cold: true, ColdExpected: true, UnexpectedCold: false,
		Usage: provider.Usage{CacheRead: 0}}
	x36abMustNotContain(t, "B5 对照乙", x36abMeter(t, b), "意外冷启动")

	// 对照丙（非真实，审查后补）：冷暖确知但为暖、UnexpectedCold=true → 仍打。
	// 真实 Record 上 UnexpectedCold 蕴含 Cold，所以门写成 known 还是 cold 在真实数据上等价；
	// 这一例把两者拆开，钉住契约 §3.3 要求的 known。
	c := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true, Cold: false, UnexpectedCold: true,
		Usage: provider.Usage{CacheRead: 555}}
	x36abMustContain(t, "B5 对照丙", x36abMeter(t, c), "意外冷启动")
}

// B6：行序 —— 用量未知在金额之前，金额在冷暖之前。
func TestX36abMeterLineOrderWhenUsageUnknown(t *testing.T) {
	out := x36abMeter(t, x36abB3Rec())
	iu := strings.Index(out, "用量 未知")
	ic := strings.Index(out, "金额")
	iw := strings.Index(out, "冷暖")
	if iu < 0 || ic < 0 || iw < 0 {
		t.Fatalf("B6: 三个标记没有全部出现（用量未知=%d 金额=%d 冷暖=%d）；完整输出:\n%s", iu, ic, iw, out)
	}
	if !(iu < ic && ic < iw) {
		t.Errorf("B6: 行序应为 用量 < 金额 < 冷暖，实际 %d/%d/%d；完整输出:\n%s", iu, ic, iw, out)
	}
}

// B7：未知那一支不许把预报也吞掉或写死。
func TestX36abMeterForecastCellKeptWhenUnknown(t *testing.T) {
	rec := turn.Record{Outcome: turn.OutcomeProviderFailed, UsageParsed: true, Cold: false, ColdExpected: true,
		Usage: provider.Usage{InputTokens: 4242, CacheRead: 0}}
	x36abMustContain(t, "B7", x36abMeter(t, rec), "冷暖 预报=冷 实测=未知")
	// 对照：只把 ColdExpected 改回 false → 预报=暖。
	rec.ColdExpected = false
	x36abMustContain(t, "B7 对照", x36abMeter(t, rec), "冷暖 预报=暖 实测=未知")
}

// ============================================================
// E. renderSettlement
// ============================================================

var x36abForecastPattern = regexp.MustCompile(`冷启动\s*预报\s*([0-9]+)`)

// x36abColdLine 取以「冷启动」开头的那一行；必须恰好一行。
// 顺带做 E9：那一行能被既有用例的正则抓到，抓到的是 4。
func x36abColdLine(t *testing.T, label string, s turn.RoundSettlement) string {
	t.Helper()
	out := renderSettlement(s, 4, nil, "发言轮")
	var found []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "冷启动") {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: 以「冷启动」开头的行有 %d 行，期望 1；完整输出:\n%s", label, len(found), out)
	}
	m := x36abForecastPattern.FindStringSubmatch(found[0])
	if m == nil || m[1] != "4" {
		t.Errorf("%s (E9): 冷启动那一行没被正则抓到预报 4：%q", label, found[0])
	}
	return found[0]
}

func x36abColdLineIs(t *testing.T, label string, s turn.RoundSettlement, wantTail string) string {
	t.Helper()
	line := x36abColdLine(t, label, s)
	if !strings.HasSuffix(line, wantTail) {
		t.Errorf("%s: 冷启动那一行应以 %q 结尾，实际 %q", label, wantTail, line)
	}
	// 渲染格式串不改（契约 6），整行可算。
	if want := "冷启动 预报 4  " + wantTail; line != want {
		t.Errorf("%s: 冷启动整行应为 %q，实际 %q", label, want, line)
	}
	return line
}

func x36abNoParen(t *testing.T, label, line string) {
	t.Helper()
	if strings.Contains(line, "（") {
		t.Errorf("%s: 冷启动那一行不应含全角括号（限定语）：%q", label, line)
	}
}

func TestX36abRenderE1ExcludedIsColdUnknownOnly(t *testing.T) {
	s := turn.RoundSettlement{Spawned: 9, SpawnFailed: 5, UsageUnknown: 1, ColdUnknown: 3, Cold: 2, HaveCacheRead: true}
	x36abColdLineIs(t, "E1", s, "实测 2（3 名成员冷暖未知，只数其余人）")
}

func TestX36abRenderE2NoKnownMemberIsUnknown(t *testing.T) {
	s := turn.RoundSettlement{Spawned: 2, UsageUnknown: 1, ColdUnknown: 2, Cold: 0, HaveCacheRead: true}
	line := x36abColdLineIs(t, "E2", s, "实测 未知")
	if strings.Contains(line, "实测 0") {
		t.Errorf("E2: 冷启动那一行不应含 %q：%q", "实测 0", line)
	}
	x36abNoParen(t, "E2", line)
}

func TestX36abRenderE3NoQualifierWhenNoneExcluded(t *testing.T) {
	s := turn.RoundSettlement{Spawned: 2, Cold: 1, HaveCacheRead: true}
	line := x36abColdLineIs(t, "E3", s, "实测 1")
	x36abNoParen(t, "E3", line)
}

func TestX36abRenderE4QualifierWithoutUsageUnknown(t *testing.T) {
	s := turn.RoundSettlement{Spawned: 3, UsageUnknown: 0, ColdUnknown: 1, Cold: 1, HaveCacheRead: true}
	x36abColdLineIs(t, "E4", s, "实测 1（1 名成员冷暖未知，只数其余人）")
}

func TestX36abRenderE5UsageUnknownAloneAddsNoQualifier(t *testing.T) {
	// 非真实：真实 Settle 不会给出 UsageUnknown > ColdUnknown。
	s := turn.RoundSettlement{Spawned: 3, UsageUnknown: 2, ColdUnknown: 0, Cold: 1, HaveCacheRead: true}
	line := x36abColdLineIs(t, "E5", s, "实测 1")
	x36abNoParen(t, "E5", line)
}

func TestX36abRenderE6CacheReadUnknownNoQualifier(t *testing.T) {
	s := turn.RoundSettlement{Spawned: 3, UsageUnknown: 0, ColdUnknown: 1, Cold: 0, HaveCacheRead: false}
	line := x36abColdLineIs(t, "E6", s, "实测 未知")
	x36abNoParen(t, "E6", line)
}

func TestX36abRenderE7KnownNGateUsesColdUnknownOnly(t *testing.T) {
	s := turn.RoundSettlement{Spawned: 2, SpawnFailed: 1, UsageUnknown: 1, ColdUnknown: 1, Cold: 0, HaveCacheRead: true}
	x36abColdLineIs(t, "E7", s, "实测 0（1 名成员冷暖未知，只数其余人）")
}

func TestX36abRenderE8ThroughRealSettle(t *testing.T) {
	warm := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true, Cold: false,
		Usage: provider.Usage{CacheRead: 700}}
	timeout := turn.Record{Outcome: turn.OutcomeTimeout, UsageParsed: false}
	s := turn.Settle([]turn.Record{warm, timeout})
	x36abColdLineIs(t, "E8", s, "实测 0（1 名成员冷暖未知，只数其余人）")

	// 对照：Timeout 换成第二个暖的 Spoke → 没有人冷暖未知，裸印 0。
	s2 := turn.Settle([]turn.Record{warm, warm})
	line := x36abColdLineIs(t, "E8 对照", s2, "实测 0")
	x36abNoParen(t, "E8 对照", line)
}
