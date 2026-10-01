package turn

import (
	"fmt"
	"strings"
	"testing"

	"council/internal/provider"
)

// 本文件是 x36ab（失败轮的冷暖不是实测值）契约第 9 节 A、C、D 三组验收。
// 每一条「新行为」断言都配一个只差一个字段的对照组：同一套夹具换一个取值，
// 确实会打出冷 / 暖 / 数字，证明断言不是结构上恒真。

// x36abOutcomeName 给每个结局一个可读的名字，失败信息里能看出是哪一行红了。
var x36abOutcomeName = map[Outcome]string{
	OutcomeUnknown:        "Unknown",
	OutcomeSpoke:          "Spoke",
	OutcomePostRejected:   "PostRejected",
	OutcomeProviderFailed: "ProviderFailed",
	OutcomeUnparsable:     "Unparsable",
	OutcomeTimeout:        "Timeout",
	OutcomeSpawnFailed:    "SpawnFailed",
	OutcomeRefused:        "Refused",
	OutcomeCanceled:       "Canceled",
	OutcomeLost:           "Lost",
}

// x36abAllOutcomes 显式列出全部十个结局，不用 0..9 循环：
// Canceled 与 Lost 是字面量 8、9，将来若有人插一个新值，循环会悄悄漏掉或多出。
var x36abAllOutcomes = []Outcome{
	OutcomeUnknown, OutcomeSpoke, OutcomePostRejected, OutcomeProviderFailed,
	OutcomeUnparsable, OutcomeTimeout, OutcomeSpawnFailed, OutcomeRefused,
	OutcomeCanceled, OutcomeLost,
}

func x36abIntPtr(v int) *int { return &v }

// ============================================================
// A. MeasuredCold 真值表
// ============================================================

type x36abMCRow struct {
	name      string
	rec       Record
	wantCold  bool
	wantKnown bool
}

func x36abMCRec(oc Outcome, parsed bool, unreported []string, cold bool) Record {
	return Record{
		Outcome:     oc,
		UsageParsed: parsed,
		Usage:       provider.Usage{Unreported: unreported},
		Cold:        cold,
	}
}

func TestX36abMeasuredColdTruthTable(t *testing.T) {
	rows := []x36abMCRow{
		// Ingested 且冷暖确知：cold 原样取 rec.Cold。这四行是后面所有「未知」行的对照组。
		{"Spoke/已解析/cold=true", x36abMCRec(OutcomeSpoke, true, nil, true), true, true},
		{"Spoke/已解析/cold=false", x36abMCRec(OutcomeSpoke, true, nil, false), false, true},
		{"PostRejected/已解析/cold=true", x36abMCRec(OutcomePostRejected, true, nil, true), true, true},
		{"PostRejected/已解析/cold=false", x36abMCRec(OutcomePostRejected, true, nil, false), false, true},

		{"ProviderFailed/已解析/cold=false", x36abMCRec(OutcomeProviderFailed, true, nil, false), false, false},
		// 非真实：专挡「known 为假也把 rec.Cold 原样返回」。
		{"ProviderFailed/已解析/cold=true（非真实）", x36abMCRec(OutcomeProviderFailed, true, nil, true), false, false},
		// 空会话 id 那条路的真实形状是 Cold=false；这里用 true 让「known 为真」的坏实现露出来。
		{"Unparsable/已解析/cold=true", x36abMCRec(OutcomeUnparsable, true, nil, true), false, false},

		// cache_read 标了未报：与第一行只差 Unreported。
		{"Spoke/已解析/Unreported=[cache_read]/cold=true", x36abMCRec(OutcomeSpoke, true, []string{"cache_read"}, true), false, false},
		{"PostRejected/已解析/Unreported=[cache_read]/cold=true", x36abMCRec(OutcomePostRejected, true, []string{"cache_read"}, true), false, false},
		{"Spoke/已解析/Unreported=[cache_create,cache_read]/cold=true", x36abMCRec(OutcomeSpoke, true, []string{"cache_create", "cache_read"}, true), false, false},
		// 没解析过：与第一行只差 UsageParsed。
		{"Spoke/未解析/cold=true", x36abMCRec(OutcomeSpoke, false, nil, true), false, false},
		{"PostRejected/未解析/cold=true", x36abMCRec(OutcomePostRejected, false, nil, true), false, false},

		// grok / agy 每个成功轮的真实形状：Unreported 非空但不含 cache_read，冷暖仍确知。
		{"Spoke/已解析/Unreported=[cache_create]/cold=true", x36abMCRec(OutcomeSpoke, true, []string{"cache_create"}, true), true, true},
		{"Spoke/已解析/Unreported=[cache_create]/cold=false", x36abMCRec(OutcomeSpoke, true, []string{"cache_create"}, false), false, true},
	}
	// 非 Ingested 结局：UsageParsed 两种取值都写。只写「否」的话 UsageParsed 条件
	// 先把它挡掉，Ingested() 条件等于没测。Cold 一律给 true，让「照读 rec.Cold」露出来。
	for _, oc := range []Outcome{OutcomeTimeout, OutcomeCanceled, OutcomeSpawnFailed, OutcomeUnparsable,
		OutcomeLost, OutcomeRefused, OutcomeUnknown, OutcomeProviderFailed} {
		for _, parsed := range []bool{true, false} {
			rows = append(rows, x36abMCRow{
				name:      fmt.Sprintf("%s/UsageParsed=%v/cold=true", x36abOutcomeName[oc], parsed),
				rec:       x36abMCRec(oc, parsed, nil, true),
				wantCold:  false,
				wantKnown: false,
			})
		}
	}

	for _, row := range rows {
		cold, known := row.rec.MeasuredCold()
		if cold != row.wantCold || known != row.wantKnown {
			t.Errorf("[%s] MeasuredCold() = (cold=%v, known=%v)，期望 (cold=%v, known=%v)",
				row.name, cold, known, row.wantCold, row.wantKnown)
		}
	}
}

// ============================================================
// C. Settle
// ============================================================

// x36abMainGroup 是契约 9.C 的主组十条。每条用量已解析的都给互不相同的非零 InputTokens。
func x36abMainGroup() []Record {
	return []Record{
		/* 1 */ {Outcome: OutcomeSpoke, UsageParsed: true, Cold: true,
			Usage: provider.Usage{Unreported: []string{"cache_create"}, InputTokens: 1, CacheRead: 0}},
		/* 2 */ {Outcome: OutcomeSpoke, UsageParsed: true, Cold: false,
			Usage: provider.Usage{InputTokens: 2, CacheRead: 700}},
		/* 3 */ {Outcome: OutcomePostRejected, UsageParsed: true, Cold: true,
			Usage: provider.Usage{InputTokens: 4, CacheRead: 0}},
		/* 4 非真实：挡「直接读 rec.Cold」 */ {Outcome: OutcomeProviderFailed, UsageParsed: true, Cold: true,
			Usage: provider.Usage{InputTokens: 1000, CacheRead: 0}},
		/* 5 */ {Outcome: OutcomeProviderFailed, UsageParsed: true, Cold: false,
			Usage: provider.Usage{InputTokens: 20000, CacheRead: 555}},
		/* 6 */ {Outcome: OutcomeSpoke, UsageParsed: true, Cold: true,
			Usage: provider.Usage{Unreported: []string{"cache_read"}, InputTokens: 300, CacheRead: 0}},
		/* 7 */ {Outcome: OutcomeUnparsable, UsageParsed: true, Cold: false,
			Usage: provider.Usage{InputTokens: 50000, CacheRead: 0}},
		/* 8 垃圾数字，不该进合计 */ {Outcome: OutcomeTimeout, UsageParsed: false, Cold: false,
			Usage: provider.Usage{InputTokens: 99999, CacheRead: 0}},
		/* 9 */ {Outcome: OutcomeSpawnFailed, UsageParsed: false, Cold: false},
		/* 10 */ {Outcome: OutcomeRefused, UsageParsed: false, Cold: false},
	}
}

func TestX36abSettleMainGroup(t *testing.T) {
	s := Settle(x36abMainGroup())
	checks := []struct {
		name      string
		got, want int
	}{
		{"Spawned", s.Spawned, 8},
		{"SpawnFailed", s.SpawnFailed, 1},
		{"UsageUnknown", s.UsageUnknown, 1},
		{"ColdUnknown", s.ColdUnknown, 5}, // 第 4、5、6、7、8 条
		{"Cold", s.Cold, 2},               // 第 1、3 条
		{"InputTokens", s.InputTokens, 71307},
		{"CacheRead", s.CacheRead, 1255},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("主组 %s = %d，期望 %d；完整结算 %+v", c.name, c.got, c.want, s)
		}
	}
	if !s.HaveIn {
		t.Errorf("主组 HaveIn 应为 true")
	}
	if !s.HaveCacheRead {
		t.Errorf("主组 HaveCacheRead 应为 true")
	}
}

// TestX36abSettleMainGroupControls 是主组的对照：每一组只改主组里的一个字段，
// 证明同一套夹具在另一种取值下 Cold / ColdUnknown 确实会动，不是恒为 2 / 5。
func TestX36abSettleMainGroupControls(t *testing.T) {
	cases := []struct {
		name                  string
		mutate                func(recs []Record)
		wantCold, wantUnknown int
	}{
		// 第 4 条（ProviderFailed、Cold=true）只把结局换成 Spoke：它应当被数成冷。
		{"第4条结局换成Spoke", func(r []Record) { r[3].Outcome = OutcomeSpoke }, 3, 4},
		// 第 6 条只去掉 Unreported：cache_read 已报，Cold=true 被数成冷。
		{"第6条去掉Unreported", func(r []Record) { r[5].Usage.Unreported = nil }, 3, 4},
		// 第 8 条只把 UsageParsed 改成 true（Timeout 仍非 Ingested）：ColdUnknown 不变，UsageUnknown 归零。
		{"第8条UsageParsed改true", func(r []Record) { r[7].UsageParsed = true }, 2, 5},
		// 第 7 条只把结局换成 Spoke（Cold=false）：冷暖确知为暖，ColdUnknown 少一个、Cold 不变。
		{"第7条结局换成Spoke", func(r []Record) { r[6].Outcome = OutcomeSpoke }, 2, 4},
	}
	for _, c := range cases {
		recs := x36abMainGroup()
		c.mutate(recs)
		s := Settle(recs)
		if s.Cold != c.wantCold || s.ColdUnknown != c.wantUnknown {
			t.Errorf("对照 %s: Cold=%d ColdUnknown=%d，期望 Cold=%d ColdUnknown=%d",
				c.name, s.Cold, s.ColdUnknown, c.wantCold, c.wantUnknown)
		}
		if s.Spawned != 8 {
			t.Errorf("对照 %s: Spawned=%d，期望 8", c.name, s.Spawned)
		}
	}
}

func x36abTimeoutUnparsed() Record {
	return Record{Outcome: OutcomeTimeout, UsageParsed: false, Cold: false,
		Usage: provider.Usage{InputTokens: 99999}}
}

func TestX36abSettleSecondGroupUnparsedCountsAsColdUnknown(t *testing.T) {
	s := Settle([]Record{x36abTimeoutUnparsed(), x36abTimeoutUnparsed()})
	if s.Spawned != 2 || s.UsageUnknown != 2 || s.ColdUnknown != 2 || s.Cold != 0 {
		t.Errorf("第二组：Spawned=%d UsageUnknown=%d ColdUnknown=%d Cold=%d，期望 2/2/2/0",
			s.Spawned, s.UsageUnknown, s.ColdUnknown, s.Cold)
	}
	// 对照：第二条只把结局换成 Spoke 并置 UsageParsed、Cold（真实的冷启动成功轮），
	// ColdUnknown 与 Cold 都要跟着动。
	ctrl := x36abTimeoutUnparsed()
	ctrl.Outcome = OutcomeSpoke
	ctrl.UsageParsed = true
	ctrl.Cold = true
	s2 := Settle([]Record{x36abTimeoutUnparsed(), ctrl})
	if s2.ColdUnknown != 1 || s2.Cold != 1 || s2.UsageUnknown != 1 {
		t.Errorf("第二组对照：ColdUnknown=%d Cold=%d UsageUnknown=%d，期望 1/1/1",
			s2.ColdUnknown, s2.Cold, s2.UsageUnknown)
	}
}

func x36abThirdGroupRec(oc Outcome) Record {
	return Record{
		Outcome:     oc,
		UsageParsed: true,
		Cold:        false,
		Usage: provider.Usage{
			InputTokens:    9,
			CacheRead:      555,
			OutputTokens:   7,
			ThinkingTokens: 5,
			CacheCreate:    x36abIntPtr(3),
		},
	}
}

func x36abCheckThirdGroupTokens(t *testing.T, label string, s RoundSettlement) {
	t.Helper()
	if !s.HaveIn || s.InputTokens != 9 {
		t.Errorf("%s: HaveIn=%v InputTokens=%d，期望 true/9", label, s.HaveIn, s.InputTokens)
	}
	if !s.HaveCacheRead || s.CacheRead != 555 {
		t.Errorf("%s: HaveCacheRead=%v CacheRead=%d，期望 true/555", label, s.HaveCacheRead, s.CacheRead)
	}
	if !s.HaveOut || s.OutputTokens != 7 {
		t.Errorf("%s: HaveOut=%v OutputTokens=%d，期望 true/7", label, s.HaveOut, s.OutputTokens)
	}
	if !s.HaveThinking || s.ThinkingTokens != 5 {
		t.Errorf("%s: HaveThinking=%v ThinkingTokens=%d，期望 true/5", label, s.HaveThinking, s.ThinkingTokens)
	}
	if !s.HaveCacheCreate || s.CacheCreate != 3 {
		t.Errorf("%s: HaveCacheCreate=%v CacheCreate=%d，期望 true/3", label, s.HaveCacheCreate, s.CacheCreate)
	}
}

func TestX36abSettleThirdGroupUnknownColdStillSumsAllFiveMetrics(t *testing.T) {
	s := Settle([]Record{x36abThirdGroupRec(OutcomeProviderFailed)})
	if s.Spawned != 1 || s.ColdUnknown != 1 || s.Cold != 0 {
		t.Errorf("第三组：Spawned=%d ColdUnknown=%d Cold=%d，期望 1/1/0", s.Spawned, s.ColdUnknown, s.Cold)
	}
	x36abCheckThirdGroupTokens(t, "第三组", s)

	// 对照：只把结局换成 Spoke。冷暖变成确知（ColdUnknown=0），五个合计不变 ——
	// 证明 ColdUnknown 在这组夹具上确实会随结局动，而合计与冷暖无关。
	s2 := Settle([]Record{x36abThirdGroupRec(OutcomeSpoke)})
	if s2.ColdUnknown != 0 {
		t.Errorf("第三组对照：ColdUnknown=%d，期望 0", s2.ColdUnknown)
	}
	x36abCheckThirdGroupTokens(t, "第三组对照", s2)
}

func TestX36abSettleSweepWiresMeasuredCold(t *testing.T) {
	unrep := [][]string{nil, {"cache_create"}, {"cache_read"}, {"cache_create", "cache_read"}}
	n := 0
	for _, oc := range x36abAllOutcomes {
		for _, parsed := range []bool{false, true} {
			for _, u := range unrep {
				for _, cold := range []bool{false, true} {
					n++
					rec := Record{Outcome: oc, UsageParsed: parsed, Cold: cold,
						Usage: provider.Usage{Unreported: u, InputTokens: 11}}
					name := fmt.Sprintf("%s/parsed=%v/unrep=%s/cold=%v",
						x36abOutcomeName[oc], parsed, strings.Join(u, "+"), cold)
					s := Settle([]Record{rec})
					mc, mk := rec.MeasuredCold()
					switch s.Spawned {
					case 1:
						if (s.ColdUnknown == 1) != !mk {
							t.Errorf("[%s] ColdUnknown=%d 但 MeasuredCold known=%v", name, s.ColdUnknown, mk)
						}
						if (s.Cold == 1) != (mk && mc) {
							t.Errorf("[%s] Cold=%d 但 MeasuredCold=(%v,%v)", name, s.Cold, mc, mk)
						}
						if s.UsageUnknown > s.ColdUnknown {
							t.Errorf("[%s] UsageUnknown=%d > ColdUnknown=%d", name, s.UsageUnknown, s.ColdUnknown)
						}
						// 冷暖确知人数 + ColdUnknown == Spawned；单成员下即两者之和不超过 1。
						knownN := 0
						if mk {
							knownN = 1
						}
						if s.ColdUnknown+knownN != s.Spawned {
							t.Errorf("[%s] ColdUnknown(%d)+确知(%d) != Spawned(%d)", name, s.ColdUnknown, knownN, s.Spawned)
						}
					case 0:
						if s.ColdUnknown != 0 || s.Cold != 0 {
							t.Errorf("[%s] 未计入 Spawned 却有 ColdUnknown=%d Cold=%d", name, s.ColdUnknown, s.Cold)
						}
						switch oc {
						case OutcomeSpawnFailed, OutcomeRefused, OutcomeUnknown:
						default:
							t.Errorf("[%s] Spawned=0，但这个结局应当计入 Spawned", name)
						}
					default:
						t.Errorf("[%s] 单成员 Spawned=%d", name, s.Spawned)
					}
				}
			}
		}
	}
	if n != 10*2*4*2 {
		t.Fatalf("扫描组合数 %d，期望 160", n)
	}
}

// ============================================================
// D. roundFinishedMeta
// ============================================================

// x36abIntKey 取 extra[key]，要求键存在且类型是 int。键不存在与值为 0 必须分得开。
func x36abIntKey(t *testing.T, label string, extra map[string]any, key string) (int, bool) {
	t.Helper()
	v, ok := extra[key]
	if !ok {
		t.Errorf("%s: extra 里没有键 %q；extra=%v", label, key, extra)
		return 0, false
	}
	n, isInt := v.(int)
	if !isInt {
		t.Errorf("%s: extra[%q] 的类型是 %T，期望 int", label, key, v)
		return 0, false
	}
	return n, true
}

func TestX36abRoundFinishedMetaMainGroup(t *testing.T) {
	extra, text := roundFinishedMeta("rid-x36ab", x36abMainGroup())
	want := map[string]int{"cold_unknown": 5, "cold": 2, "usage_unknown": 1, "input_tokens": 71307}
	for k, w := range want {
		if n, ok := x36abIntKey(t, "D1", extra, k); ok && n != w {
			t.Errorf("D1: extra[%q]=%d，期望 %d", k, n, w)
		}
	}
	if !strings.HasSuffix(text, " cold=2 cold_unknown=5") {
		t.Errorf("D1: 文本应以 %q 结尾，实际 %q", " cold=2 cold_unknown=5", text)
	}
}

func TestX36abRoundFinishedMetaZeroKeyPresent(t *testing.T) {
	rec := Record{Outcome: OutcomeSpoke, UsageParsed: true, Cold: false,
		Usage: provider.Usage{CacheRead: 900}}
	extra, text := roundFinishedMeta("rid-x36ab", []Record{rec})
	if n, ok := x36abIntKey(t, "D2", extra, "cold_unknown"); ok && n != 0 {
		t.Errorf("D2: extra[cold_unknown]=%d，期望 0", n)
	}
	if n, ok := x36abIntKey(t, "D2", extra, "usage_unknown"); ok && n != 0 {
		t.Errorf("D2: extra[usage_unknown]=%d，期望 0", n)
	}
	if !strings.HasSuffix(text, " cold=0 cold_unknown=0") {
		t.Errorf("D2: 文本应以 %q 结尾，实际 %q", " cold=0 cold_unknown=0", text)
	}
	// 契约 5.2 冻结：cold_unknown 之前的部分一个字节不改。这一例每一格都能算出来，钉整串。
	wantText := "一轮结束 round_id=rid-x36ab spawned=1 spawn_failed=0 usage_unknown=0 in=0 out=0 cache_read=900 cache_create=? thinking=0 cost_usd=? cost_unreported=1 cold=0 cold_unknown=0"
	if text != wantText {
		t.Errorf("D2: 整行文本\n实际 %q\n期望 %q", text, wantText)
	}

	// 对照：只把结局换成 ProviderFailed，cold_unknown 变成 1 —— 证明这个键的值会动，
	// 上面的 0 不是一个写死的常数。
	rec.Outcome = OutcomeProviderFailed
	extra2, text2 := roundFinishedMeta("rid-x36ab", []Record{rec})
	if n, ok := x36abIntKey(t, "D2 对照", extra2, "cold_unknown"); ok && n != 1 {
		t.Errorf("D2 对照: extra[cold_unknown]=%d，期望 1", n)
	}
	if !strings.HasSuffix(text2, " cold=0 cold_unknown=1") {
		t.Errorf("D2 对照: 文本应以 %q 结尾，实际 %q", " cold=0 cold_unknown=1", text2)
	}
}

func TestX36abRoundFinishedMetaSecondGroup(t *testing.T) {
	extra, text := roundFinishedMeta("rid-x36ab", []Record{x36abTimeoutUnparsed(), x36abTimeoutUnparsed()})
	if n, ok := x36abIntKey(t, "D3", extra, "cold_unknown"); ok && n != 2 {
		t.Errorf("D3: extra[cold_unknown]=%d，期望 2", n)
	}
	if n, ok := x36abIntKey(t, "D3", extra, "usage_unknown"); ok && n != 2 {
		t.Errorf("D3: extra[usage_unknown]=%d，期望 2", n)
	}
	if !strings.HasSuffix(text, " cold=0 cold_unknown=2") {
		t.Errorf("D3: 文本应以 %q 结尾，实际 %q", " cold=0 cold_unknown=2", text)
	}
}
