package turn

import (
	"fmt"
	"testing"
	"time"

	"council/internal/provider"
)

// 本文件是 X-38a 契约第 4 节 A1、A2 两组验收：Record.ReportedUsage 与写库逐列一致。
// A2 的判据是包内既有的 makeTurnUsage，不是 ReportedUsage 自己。

// x38aNames 是三家适配器写进 Unreported 的五个指标名，顺序与 x38aCols 对齐。
var x38aNames = [5]string{"input_tokens", "cache_create", "cache_read", "output_tokens", "thinking_tokens"}

// x38aColNames 是 ReportedUsage 五个字段的名字，只用于失败消息。
var x38aColNames = [5]string{"Input", "CacheCreate", "CacheRead", "Output", "Thinking"}

func x38aIntPtr(v int) *int { return &v }

// x38aCols 把 ReportedUsage 按 x38aNames 的顺序摊成五个指针。
func x38aCols(ru ReportedUsage) [5]*int {
	return [5]*int{ru.Input, ru.CacheCreate, ru.CacheRead, ru.Output, ru.Thinking}
}

// x38aPtrStr 把一个 *int 写成可读的形状，失败消息用。
func x38aPtrStr(p *int) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("&%d", *p)
}

// x38aA1Rec 是 A1 的基准夹具：五个数互不相同且非零，挡「指标名对错了位」。
func x38aA1Rec(parsed bool, unreported []string) Record {
	return Record{
		Outcome:     OutcomeSpoke,
		UsageParsed: parsed,
		Usage: provider.Usage{
			InputTokens:    11,
			CacheCreate:    x38aIntPtr(22),
			CacheRead:      33,
			OutputTokens:   44,
			ThinkingTokens: 55,
			Unreported:     unreported,
		},
	}
}

// x38aCheck 逐格比对：want 的每一格为 nil 表示期望 nil，否则期望非 nil 且值相等。
func x38aCheck(t *testing.T, label string, ru ReportedUsage, want [5]*int) {
	t.Helper()
	got := x38aCols(ru)
	for i := 0; i < 5; i++ {
		g, w := got[i], want[i]
		switch {
		case w == nil && g != nil:
			t.Errorf("%s: %s 应为 nil，实际 %s", label, x38aColNames[i], x38aPtrStr(g))
		case w != nil && g == nil:
			t.Errorf("%s: %s 应为 &%d，实际 nil", label, x38aColNames[i], *w)
		case w != nil && g != nil && *w != *g:
			t.Errorf("%s: %s 应为 &%d，实际 &%d", label, x38aColNames[i], *w, *g)
		}
	}
}

// x38aFull 是五格依次 11、22、33、44、55。
func x38aFull() [5]*int {
	return [5]*int{x38aIntPtr(11), x38aIntPtr(22), x38aIntPtr(33), x38aIntPtr(44), x38aIntPtr(55)}
}

// x38aFullExcept 是 x38aFull 把第 i 格换成 nil。
func x38aFullExcept(i int) [5]*int {
	w := x38aFull()
	w[i] = nil
	return w
}

// ============================================================
// A1. ReportedUsage 表
// ============================================================

func TestX38aReportedUsageTable(t *testing.T) {
	var allNil [5]*int

	type row struct {
		label string
		rec   Record
		want  [5]*int
	}
	rows := []row{
		{"未解析（垃圾值）", x38aA1Rec(false, nil), allNil},
		{"解析、Unreported 空", x38aA1Rec(true, nil), x38aFull()},
		{"只含 input_tokens", x38aA1Rec(true, []string{"input_tokens"}), x38aFullExcept(0)},
		{"只含 cache_create", x38aA1Rec(true, []string{"cache_create"}), x38aFullExcept(1)},
		{"只含 cache_read", x38aA1Rec(true, []string{"cache_read"}), x38aFullExcept(2)},
		{"只含 output_tokens", x38aA1Rec(true, []string{"output_tokens"}), x38aFullExcept(3)},
		{"只含 thinking_tokens", x38aA1Rec(true, []string{"thinking_tokens"}), x38aFullExcept(4)},
		{"五个全列（usage 对象缺失）", x38aA1Rec(true, x38aNames[:]), allNil},
	}

	// Unreported 空、CacheCreate: nil → 只有 CacheCreate 为 nil。
	ccNil := x38aA1Rec(true, nil)
	ccNil.Usage.CacheCreate = nil
	rows = append(rows, row{"Unreported 空、CacheCreate nil", ccNil, x38aFullExcept(1)})

	// grok 形状：CacheCreate: &0、Unreported: [cache_create] → CacheCreate 为 nil（不是指向 0）。
	grok := x38aA1Rec(true, []string{"cache_create"})
	grok.Usage.CacheCreate = x38aIntPtr(0)
	rows = append(rows, row{"grok 形状", grok, x38aFullExcept(1)})

	// 实测为 0 不是 NULL：五个都非 nil 且指向 0。
	zeros := Record{Outcome: OutcomeSpoke, UsageParsed: true, Usage: provider.Usage{
		InputTokens: 0, CacheCreate: x38aIntPtr(0), CacheRead: 0, OutputTokens: 0, ThinkingTokens: 0}}
	rows = append(rows, row{"全部实测为 0", zeros,
		[5]*int{x38aIntPtr(0), x38aIntPtr(0), x38aIntPtr(0), x38aIntPtr(0), x38aIntPtr(0)}})

	for _, r := range rows {
		x38aCheck(t, "A1 "+r.label, r.rec.ReportedUsage(), r.want)
	}

	// 对照：grok 形状只去掉 Unreported → CacheCreate 指向 0（证明 grok 行的 nil 来自 Unreported，
	// 不是来自值为 0）。
	grokCtl := grok
	grokCtl.Usage.Unreported = nil
	x38aCheck(t, "A1 grok 对照（去掉 Unreported）", grokCtl.ReportedUsage(),
		[5]*int{x38aIntPtr(11), x38aIntPtr(0), x38aIntPtr(33), x38aIntPtr(44), x38aIntPtr(55)})

	// 对照：未解析那一行只翻 UsageParsed → 五格全有数（证明第一行的全 nil 来自 UsageParsed）。
	x38aCheck(t, "A1 未解析对照（翻 UsageParsed）", x38aA1Rec(true, nil).ReportedUsage(), x38aFull())
}

// A1 另一条：CacheCreate 的指针不与 Record 共享。
func TestX38aReportedUsageCacheCreateNotShared(t *testing.T) {
	rec := x38aA1Rec(true, nil)
	orig := rec.Usage.CacheCreate
	ru := rec.ReportedUsage()
	if ru.CacheCreate == nil {
		t.Fatalf("前提不成立：Unreported 空、CacheCreate=&22 时 ru.CacheCreate 应非 nil")
	}
	if *ru.CacheCreate != 22 {
		t.Fatalf("前提不成立：ru.CacheCreate 应指向 22，实际 %d", *ru.CacheCreate)
	}
	if ru.CacheCreate == orig {
		t.Errorf("ru.CacheCreate 与 rec.Usage.CacheCreate 是同一个指针（应新分配）")
	}
	*ru.CacheCreate = 999
	if *orig != 22 {
		t.Errorf("改 ru.CacheCreate 改到了 Record：*rec.Usage.CacheCreate = %d，应仍是 22", *orig)
	}
	if *rec.Usage.CacheCreate != 22 {
		t.Errorf("改 ru.CacheCreate 改到了 Record：*rec.Usage.CacheCreate = %d，应仍是 22", *rec.Usage.CacheCreate)
	}
	// 再取一次，仍是 22（写 999 没有写回 Record）。
	if again := rec.ReportedUsage(); again.CacheCreate == nil || *again.CacheCreate != 22 {
		t.Errorf("写 999 之后再取一次 ReportedUsage，CacheCreate 应仍是 &22，实际 %s", x38aPtrStr(again.CacheCreate))
	}
}

// ============================================================
// A2. 与写库逐列一致（判据是 makeTurnUsage）
// ============================================================

// x38aCompareWithStore 对一个 Record 比对 ReportedUsage 与 makeTurnUsage 的五列，
// 返回不一致的格数；colNil / colSet 统计 makeTurnUsage 这一侧每列 nil 与非 nil 出现的次数。
func x38aCompareWithStore(t *testing.T, label string, rec Record, colNil, colSet *[5]int) int {
	t.Helper()
	at := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	u := makeTurnUsage(&rec, nil, at)
	ru := rec.ReportedUsage()
	store := [5]*int{u.InputTokens, u.CacheCreate, u.CacheRead, u.OutputTokens, u.ThinkingTokens}
	got := x38aCols(ru)
	bad := 0
	for i := 0; i < 5; i++ {
		s, g := store[i], got[i]
		if s == nil {
			colNil[i]++
		} else {
			colSet[i]++
		}
		switch {
		case (s == nil) != (g == nil):
			t.Errorf("A2 %s: %s 与库不一致：库 %s，屏幕 %s", label, x38aColNames[i], x38aPtrStr(s), x38aPtrStr(g))
			bad++
		case s != nil && *s != *g:
			t.Errorf("A2 %s: %s 值与库不一致：库 %d，屏幕 %d", label, x38aColNames[i], *s, *g)
			bad++
		}
	}
	return bad
}

func TestX38aReportedUsageMatchesStore(t *testing.T) {
	outcomes := []Outcome{OutcomeSpoke, OutcomeProviderFailed}
	type ccCase struct {
		name string
		mk   func() *int
	}
	ccs := []ccCase{
		{"nil", func() *int { return nil }},
		{"&0", func() *int { return x38aIntPtr(0) }},
		{"&22", func() *int { return x38aIntPtr(22) }},
	}
	var colNil, colSet [5]int
	n := 0
	for _, oc := range outcomes {
		for _, parsed := range []bool{false, true} {
			for mask := 0; mask < 32; mask++ {
				var unrep []string
				for i := 0; i < 5; i++ {
					if mask&(1<<i) != 0 {
						unrep = append(unrep, x38aNames[i])
					}
				}
				for _, cc := range ccs {
					rec := Record{
						Outcome:     oc,
						UsageParsed: parsed,
						Usage: provider.Usage{
							InputTokens:    11,
							CacheCreate:    cc.mk(),
							CacheRead:      33,
							OutputTokens:   44,
							ThinkingTokens: 55,
							Unreported:     unrep,
						},
					}
					label := fmt.Sprintf("outcome=%d parsed=%v unreported=%v cache_create=%s", oc, parsed, unrep, cc.name)
					x38aCompareWithStore(t, label, rec, &colNil, &colSet)
					n++
				}
			}
		}
	}
	if n != 384 {
		t.Fatalf("A2 扫描组合数应为 384，实际 %d", n)
	}
	// 扫描不能退化：库这一侧每一列都必须既出现过 nil 又出现过非 nil，
	// 否则「逐列一致」只比了一种状态。
	for i := 0; i < 5; i++ {
		if colNil[i] == 0 || colSet[i] == 0 {
			t.Errorf("A2 扫描退化：%s 列 nil 出现 %d 次、非 nil 出现 %d 次", x38aColNames[i], colNil[i], colSet[i])
		}
	}
}

// A2 补：Unreported 含重复名与不认识的名字时，屏幕仍与写库逐列一致。
// 契约 §2 只按五个名字判「含」，不认识的名字两侧都应忽略。
func TestX38aReportedUsageMatchesStoreOddUnreported(t *testing.T) {
	var colNil, colSet [5]int
	cases := [][]string{
		{"cost_usd"},
		{"cache_read", "cache_read"},
		{"Input_Tokens", "input_tokens "},
		{},
	}
	for _, unrep := range cases {
		for _, parsed := range []bool{false, true} {
			rec := x38aA1Rec(parsed, unrep)
			x38aCompareWithStore(t, fmt.Sprintf("parsed=%v unreported=%q", parsed, unrep), rec, &colNil, &colSet)
		}
	}
}
