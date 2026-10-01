package turn

// 本文件覆盖片 3b roundFinishedMeta / roundSpawned 的结算口径（scheduler.go）。
// 这次改的是"旧口径从来没有用例担保过"这个空白：全量测试跑绿只说明没有
// 用例盯着这两个函数，不说明口径对。
//
// 背对背纪律：本文件按任务书给的契约写断言，不读 roundFinishedMeta /
// roundSpawned 的函数体（scheduler.go 148 行往后没有读过）。roundFinishedMeta
// 是包内私有纯函数（签名见 scheduler.go 133 行调用处：
// func roundFinishedMeta(roundID string, recs []Record) (map[string]any, string)），
// 不碰数据库、不起进程，直接构造 []Record 调用即可，不需要 Service/Runner，
// 仿 meta_test.go 的 TestModelSourceOfPureFunction 体例。
//
// 断言红了不许回头改断言迁就实现——红了就是实现与契约对不上，报告里如实写。

import (
	"strings"
	"testing"

	"council/internal/provider"
)

// ---------------------------------------------------------------------------
// 本文件专用小工具，rl 前缀（round ledger），避免与包内其它测试文件的同名
// helper 冲突（已用 grep 核实本包没有 intPtr/floatPtr/metaInt 这几个名字）。
// ---------------------------------------------------------------------------

// rlIntPtr 铸一个 *int。
func rlIntPtr(v int) *int { return &v }

// rlFloatPtr 铸一个 *float64。
func rlFloatPtr(v float64) *float64 { return &v }

// rlMetaInt 从 roundFinishedMeta 直接返回的 map[string]any 里取一个整数键。
// 直接调用纯函数拿到的是内存里的原生值，没有经过 store 的
// json.Marshal/Unmarshal 往返（那个往返才会把 int 变成 float64，见
// meta_test.go 的 metaFloat64Slice 注释），但契约没有钉死实现具体写的是
// int 还是 int64 还是 float64，这里三种都认。取不到或类型不对时把整张表
// 转出来 Fatalf，不静默当 0——静默当 0 会把"键名对不上"与"键值恰好是 0"
// 这两种完全不同的坏实现混成同一种断言失败，报告里就分不清是哪一种。
func rlMetaInt(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		t.Fatalf("Meta[%q] 取不到整数，实际类型 %T，完整表: %#v", key, m[key], m)
		return 0
	}
}

// rlAssertLabelIsQuestionMarkNotZero 断言文本行里 " <label>=?" 出现、
// " <label>=0" 不出现。前缀空格是为了精确卡到"键=值"这个 token 的边界，
// 不误配到别的键名里的子串。两个方向都断言，不是只查 "=?" 存在——契约 E
// 要挡的坏实现恰恰是"把被省略的指标印成 0"，只查 "=?" 存在挡不住一个
// 同时把两种都打印出来的实现（不太可能，但"不出现 =0"这一半才是真正
// 把 ? 和 0 分开的那一句）。
func rlAssertLabelIsQuestionMarkNotZero(t *testing.T, text, label string) {
	t.Helper()
	qmark := " " + label + "=?"
	zero := " " + label + "=0"
	if !strings.Contains(text, qmark) {
		t.Errorf("文本里没有 %q：%s", qmark, text)
	}
	if strings.Contains(text, zero) {
		t.Errorf("文本里不该出现 %q（省略的指标要印成 ?，不是 0）：%s", zero, text)
	}
}

// ---------------------------------------------------------------------------
// 用例 1：spawn_failed 单列，不进 spawned，不进 cost_unreported
// ---------------------------------------------------------------------------

// TestRoundLedgerSpawnFailedNotCountedAsSpawnedOrCostUnreported 挡的是
// 「把 OutcomeSpawnFailed 也算进 spawned（或者算进 refused 那一档，效果上
// 等价于没单列 spawn_failed）」这类坏实现。契约 A/B：OutcomeSpawnFailed
// 收了三件分不开的事（不在 PATH / 服务睡着 / 起来了立刻非零退出），只有
// 第三种真起过进程，Record 上没有字段能拆开，分不清就不算 spawned，
// 单独进新键 spawn_failed；它也不进 cost_unreported（那一项只数"确知起过
// 进程"里 CostUSDDelta==nil 的成员）。
//
// 三个数都断言，不是只断言 spawned：只断言 spawned==1 的话，一个把
// OutcomeSpawnFailed 直接归进 OutcomeRefused 那一类（因而也不进
// spawn_failed）的坏实现同样能让 spawned==1 通过——spawn_failed 那条断言
// 才能把这种坏实现和正确实现分开。
func TestRoundLedgerSpawnFailedNotCountedAsSpawnedOrCostUnreported(t *testing.T) {
	spoke := Record{
		Outcome:      OutcomeSpoke,
		UsageParsed:  true,
		CostUSDDelta: rlFloatPtr(0.5),
	}
	spawnFailed := Record{
		Outcome:      OutcomeSpawnFailed,
		CostUSDDelta: nil,
	}
	refused := Record{
		Outcome: OutcomeRefused,
	}

	extra, _ := roundFinishedMeta("round-1", []Record{spoke, spawnFailed, refused})

	if got := rlMetaInt(t, extra, "spawned"); got != 1 {
		t.Errorf("spawned = %d，期望 1（只有 OutcomeSpoke 那一条确知起过进程；OutcomeRefused 起进程之前就停了，OutcomeSpawnFailed 分不清起没起过、不算）", got)
	}
	if got := rlMetaInt(t, extra, "spawn_failed"); got != 1 {
		t.Errorf("spawn_failed = %d，期望 1（OutcomeSpawnFailed 必须单独计数，不许被并进 spawned 或悄悄归进 refused 那一类）", got)
	}
	if got := rlMetaInt(t, extra, "cost_unreported"); got != 0 {
		t.Errorf("cost_unreported = %d，期望 0（唯一算进 spawned 的成员报了金额；OutcomeSpawnFailed 那条 CostUSDDelta 虽是 nil，但它本身不算确知起过，不该被计进 cost_unreported）", got)
	}
}

// ---------------------------------------------------------------------------
// 用例 2：usage_unknown 数的是 spawned 里五个用量一个数都没有的，它们不进合计
// ---------------------------------------------------------------------------

// TestRoundLedgerUsageUnknownExcludesUnparsedMembersFromTotals 挡的是
// 「不看 UsageParsed，只要算进 spawned 就把 Usage 字段的值加进合计」这类
// 坏实现。X-38e 之后 usage_unknown 数「已经算进 spawned、但 ReportedUsage()
// 五个指针全 nil」的成员（UsageParsed==false 是其中一种），这些成员对任何
// 用量合计都没有贡献。本用例的夹具用的是 UsageParsed==false 那一种。
//
// 第二条成员的 InputTokens 故意塞一个非零的 999：如果它被错误地加进了
// 合计，input_tokens 就会是 1009 而不是 10——这条断言同时证明了"没解析的
// 行不进合计"，是本用例最锋利的一句。
func TestRoundLedgerUsageUnknownExcludesUnparsedMembersFromTotals(t *testing.T) {
	parsed := Record{
		Outcome:     OutcomeSpoke,
		UsageParsed: true,
		Usage:       provider.Usage{InputTokens: 10},
	}
	unparsed := Record{
		Outcome:     OutcomeSpoke,
		UsageParsed: false,
		Usage:       provider.Usage{InputTokens: 999},
	}

	extra, _ := roundFinishedMeta("round-2", []Record{parsed, unparsed})

	if got := rlMetaInt(t, extra, "spawned"); got != 2 {
		t.Errorf("spawned = %d，期望 2（两条都是 OutcomeSpoke，确知起过进程，这与 UsageParsed 是两件事）", got)
	}
	if got := rlMetaInt(t, extra, "usage_unknown"); got != 1 {
		t.Errorf("usage_unknown = %d，期望 1（只有第二条 UsageParsed=false）", got)
	}
	if got := rlMetaInt(t, extra, "input_tokens"); got != 10 {
		t.Errorf("input_tokens = %d，期望 10（不是 1009）：UsageParsed=false 那条的 999 不该进合计，这正是本用例要抓的坏实现", got)
	}
}

// ---------------------------------------------------------------------------
// 用例 3：标了 Unreported 的成员不贡献，哪怕字段值非零——本批最锋利的一条
// ---------------------------------------------------------------------------

// TestRoundLedgerUnreportedMemberContributesNothingEvenIfNonzero 挡的是
// 「没检查 Unreported，字段值是多少就照单加进合计」这类坏实现。契约 D：
// 某成员把某指标写进了 Unreported，这个成员对这个指标就不作贡献，哪怕
// 字段值不是 0——因为 0 在这里可能读作"实测为零"，而 Unreported 里的 0
// 读作"没填"，两者不能混。
//
// 两个子测试都故意让"标了 Unreported"的那一方字段值远大于另一方
// （B=100 对 A=7、B=50 对 A=3），不用 0：如果 B 的字段值也是 0，一个没认
// Unreported 的坏实现与认了的正确实现会算出同一个数，测试就没有判别力。
// 用 100/50 之后，没认 Unreported 的坏实现会得到 107/53，与期望的 7/3
// 一眼就能分开。
func TestRoundLedgerUnreportedMemberContributesNothingEvenIfNonzero(t *testing.T) {
	t.Run("cache_create", func(t *testing.T) {
		a := Record{
			Outcome:     OutcomeSpoke,
			UsageParsed: true,
			Usage: provider.Usage{
				CacheCreate: rlIntPtr(7),
				Unreported:  nil,
			},
		}
		b := Record{
			Outcome:     OutcomeSpoke,
			UsageParsed: true,
			Usage: provider.Usage{
				CacheCreate: rlIntPtr(100),
				Unreported:  []string{"cache_create"},
			},
		}
		extra, _ := roundFinishedMeta("round-3a", []Record{a, b})
		if got := rlMetaInt(t, extra, "cache_create"); got != 7 {
			t.Errorf("cache_create = %d，期望 7：B 标了 Unreported，它的 100 不该被加进来；107 就是没认 Unreported 的坏实现算出来的数", got)
		}
	})

	t.Run("input_tokens", func(t *testing.T) {
		a := Record{
			Outcome:     OutcomeSpoke,
			UsageParsed: true,
			Usage: provider.Usage{
				InputTokens: 3,
				Unreported:  nil,
			},
		}
		b := Record{
			Outcome:     OutcomeSpoke,
			UsageParsed: true,
			Usage: provider.Usage{
				InputTokens: 50,
				Unreported:  []string{"input_tokens"},
			},
		}
		extra, _ := roundFinishedMeta("round-3b", []Record{a, b})
		if got := rlMetaInt(t, extra, "input_tokens"); got != 3 {
			t.Errorf("input_tokens = %d，期望 3：B 标了 Unreported，它的 50 不该被加进来；53 就是没认 Unreported 的坏实现算出来的数", got)
		}
	})
}

// ---------------------------------------------------------------------------
// 用例 4：一个指标全员未报时，键整个消失，文本印 "?" 不是 "0"
// ---------------------------------------------------------------------------

// TestRoundLedgerMetricOmittedWhenNobodyReportsIt 挡两类坏实现：一类是
// 「全员未报某指标时把键写成 0」（契约 D：一个指标一个成员都没报过时，
// 这个键整个不出现在 Meta 里，不是写成 0）；另一类是文本行把被省略的指标
// 印成 "0" 而不是 "?"（契约 E）。
//
// 用 map 的双返回值形式判 `_, ok := extra[key]; ok` 是否为 false，不判
// "值等于 0"——判等于 0 的话，一个把未报写成 0 的坏实现照样能骗过去。
func TestRoundLedgerMetricOmittedWhenNobodyReportsIt(t *testing.T) {
	allUnreported := []string{"input_tokens", "output_tokens", "cache_read", "cache_create", "thinking_tokens"}
	r1 := Record{
		Outcome:     OutcomeSpoke,
		UsageParsed: true,
		Usage:       provider.Usage{Unreported: append([]string(nil), allUnreported...)},
	}
	r2 := Record{
		Outcome:     OutcomeSpoke,
		UsageParsed: true,
		Usage:       provider.Usage{Unreported: append([]string(nil), allUnreported...)},
	}

	extra, text := roundFinishedMeta("round-4", []Record{r1, r2})

	for _, key := range []string{"input_tokens", "output_tokens", "cache_read", "cache_create", "thinking_tokens"} {
		if v, ok := extra[key]; ok {
			t.Errorf("Meta[%q] 不该出现：两个成员都把它标进了 Unreported，全员未报的指标键要整个消失，不是写成 0；实际值 %#v", key, v)
		}
	}

	// 文本行：被省略的指标渲染成 "?"，不是 "0"（契约 E）。标签拼写取自
	// 第一遍跑的观察输出（本函数注释顶部已说明背对背只挡实现读取，不挡
	// 观察输出——文本格式允许先跑一遍再回填，数值断言不允许）：
	// "in=? out=? cache_read=? cache_create=? thinking=?"。
	for _, label := range []string{"in", "out", "cache_read", "cache_create", "thinking"} {
		rlAssertLabelIsQuestionMarkNotZero(t, text, label)
	}
}

// ---------------------------------------------------------------------------
// 用例 5：cache_create 的第二道门——指针是 nil 也跳过，即便没标 Unreported
// ---------------------------------------------------------------------------

// TestRoundLedgerCacheCreateNilPointerSkipsSecondGate 挡的是「只检查
// Unreported 那一道门，不检查指针是否为 nil」这类坏实现。契约 D 明说
// cache_create 要过两道门：不在 Unreported 里，并且 CacheCreate != nil。
// 这里故意让 CacheCreate 是 nil 但没有标进 Unreported（模拟"字段缺席但
// 没人记得标未报"这种半吊子输入）：只查第一道门的坏实现会在这种输入下
// 把这个键错误地写出来（或者解引用 nil 崩溃），第二道门缺失才会暴露。
func TestRoundLedgerCacheCreateNilPointerSkipsSecondGate(t *testing.T) {
	r := Record{
		Outcome:     OutcomeSpoke,
		UsageParsed: true,
		Usage: provider.Usage{
			CacheCreate: nil,
			Unreported:  nil,
		},
	}

	extra, _ := roundFinishedMeta("round-5", []Record{r})

	if v, ok := extra["cache_create"]; ok {
		t.Errorf("Meta[\"cache_create\"] 不该出现：CacheCreate 是 nil 指针，即便没有标进 Unreported，第二道门也该挡住它；实际值 %#v", v)
	}
}

// ---------------------------------------------------------------------------
// 用例 6：cost_usd 没人报时键不存在，cost_unreported 计数，文本印 "?"
// ---------------------------------------------------------------------------

// TestRoundLedgerCostUsdAbsentWhenNobodyReportsMoney 挡的是「cost_usd
// 没人报时写成 0」这类坏实现（契约 F：cost_usd 这个键只在至少一名成员
// 报了金额，即 CostUSDDelta != nil 时才出现在 Meta 里），以及文本行把
// 它印成 "0" 而不是 "?"（契约 E）。顺带钉住 cost_unreported：两个确知
// 起过进程的成员都没报金额，两个都要被计进去。
func TestRoundLedgerCostUsdAbsentWhenNobodyReportsMoney(t *testing.T) {
	r1 := Record{Outcome: OutcomeSpoke, UsageParsed: true, CostUSDDelta: nil}
	r2 := Record{Outcome: OutcomeSpoke, UsageParsed: true, CostUSDDelta: nil}

	extra, text := roundFinishedMeta("round-6", []Record{r1, r2})

	if v, ok := extra["cost_usd"]; ok {
		t.Errorf("Meta[\"cost_usd\"] 不该出现：两个成员都没报金额；实际值 %#v", v)
	}
	if got := rlMetaInt(t, extra, "cost_unreported"); got != 2 {
		t.Errorf("cost_unreported = %d，期望 2（两个成员都确知起过进程，且 CostUSDDelta 都是 nil）", got)
	}

	// 文本行：cost_usd 没人报时印成 "?"，不是 "0"（契约 E；标签拼写同样
	// 取自第一遍跑的观察输出 "cost_usd=?"）。
	rlAssertLabelIsQuestionMarkNotZero(t, text, "cost_usd")
}
