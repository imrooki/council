package repl

// 本文件覆盖 internal/repl 里两个尚未落地的纯渲染函数 renderBudget /
// renderSettlement（契约 docs/12_STEP9_CONTRACT.md §4.6a.2a / §4.6a.3 /
// §4.6a.5）。背对背：实现方此刻在并行写这两个函数，本文件写完时它们还
// 不存在，编译不过（undefined: renderBudget / undefined: renderSettlement）
// 是预期结果，不是本文件的问题。
//
// 断言原则（呼应 ask_test.go 顶部那段注释的同一条纪律）：不知道实现方
// 最终选的具体排版、列序、标点，所以只断言“性质”——某个占位词在不在、
// 某个数字在不在、两段输出之间某处文字是否相同——不断言整段字面输出。
// 每条用例的注释里都写清楚它挡的是哪种坏实现，以及为什么选的断言方式
// 对那种坏实现有判别力。

import (
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"council/internal/turn"
)

// intPtr 是 BudgetRow.InputTokens / ArgvHeadroom 用的整数指针 helper。
// package repl 里 ask_test.go 已经定义了 floatPtr，这里不重复定义，直接复用。
func intPtr(v int) *int { return &v }

// agentRowLines 从渲染结果里挑出含 agentID 的那些行拼接返回，用于把断言
// 限定在“某一行自己的文字”上，不被表头、表尾、其它行的合法数字污染。
//
// 前提假设：一个成员的字段渲染在同一行（横向表格），这是 CLI 表格的常见
// 排法。如果实现方选了纵向卡片排法，这个假设不成立，相关用例可能假
// 失败——这一点已经写进最终报告。
func agentRowLines(out, agentID string) string {
	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, agentID) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// bareZeroPattern 认“独立的数字 0”：前后不是数字/字母/下划线的那个 0。
// 用 \b 而不是子串匹配，这样 “12345” “100” 这类合法数字不会被误判命中
// （\b0\b 只匹配整个词就是 “0” 的那一个 token，不会命中 “100” “3.50”
// 这类数字内部的 0）。
var bareZeroPattern = regexp.MustCompile(`\b0\b`)

// extractZeroTemplate 从 s 里 [start, end) 处的那个 “0” 出发，向两侧各扩到
// 最近的 ASCII 数字或换行为止，取出它贴身的文字模板（不含扩到的那个数字，
// 换行则纳入）。只按 ASCII 字节判定数字/换行，不会切碎多字节的中文字符——
// UTF-8 里非 ASCII 字节的最高位总是 1，判定“是不是 0-9 这几个 ASCII 字节”
// 不会在多字节字符中间停下或把一个汉字劈成两半。
func extractZeroTemplate(s string, start, end int) string {
	ps := start
	for ps > 0 {
		c := s[ps-1]
		if c == '\n' || (c >= '0' && c <= '9') {
			break
		}
		ps--
	}
	pe := end
	for pe < len(s) {
		c := s[pe]
		if c >= '0' && c <= '9' {
			break
		}
		if c == '\n' {
			pe++
			break
		}
		pe++
	}
	return s[ps:start] + "0" + s[end:pe]
}

// windowAround 取 s 中围绕 [byteStart, byteStart+matchLen) 这段字节区间、
// 前后各扩 beforeRunes / afterRunes 个 rune 的窗口文字，用于在数字附近找
// 限定语而不依赖具体表格列宽。不能直接按字节偏移量前后扩，因为中文字符
// 是多字节，字节窗口会切碎汉字；换成 rune 级别的窗口就没有这个问题。
func windowAround(s string, byteStart, matchLen, beforeRunes, afterRunes int) string {
	runes := []rune(s)
	runeStart := utf8.RuneCountInString(s[:byteStart])
	matchRunes := utf8.RuneCountInString(s[byteStart : byteStart+matchLen])

	start := runeStart - beforeRunes
	if start < 0 {
		start = 0
	}
	end := runeStart + matchRunes + afterRunes
	if end > len(runes) {
		end = len(runes)
	}
	if start > end {
		return ""
	}
	return string(runes[start:end])
}

// hasHanOrLetter 判断字符串里是否含中文汉字或 ASCII 字母，用来分辨“这段
// 窗口里有没有真正的说明文字”，而不是一串数字或标点。
func hasHanOrLetter(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// TestRenderBudget_UncalibratedPrintsPlaceholderNotZero 挡的坏实现：把
// Calibrated == false 行里 nil 的 InputTokens / CostUSD 印成字面 0，而不是
// 契约 4.6a.3 要求的占位词“未标定”。
//
// 判别力设计：只造一行，这一行唯一会产生数字的字段是 ArgvHeadroom，给一个
// 不含数字 0 的显眼正数 12345，让这一行里不会有任何“合法”的 0 来源。
// 用 agentRowLines 把断言限定在这一行自己的文字上——避免表尾“另有 k 名
// 未标定”这类合计文字，或别的无关行，里可能出现的合法 0 造成误报（这正是
// 任务书提醒要当心的那类误报：断言不含 0 在表头/合计行必然出现 0 时会
// 误报，这里用行级隔离而不是全局“不含 0”来避开它），再用 \b0\b 判定
// “这一行里有没有出现独立的 0”。
func TestRenderBudget_UncalibratedPrintsPlaceholderNotZero(t *testing.T) {
	row := turn.BudgetRow{
		AgentID:      "ag-uncal",
		Provider:     "pv-x",
		Calibrated:   false,
		InputTokens:  nil,
		CostUSD:      nil,
		ArgvHeadroom: intPtr(12345),
	}

	out := renderBudget([]turn.BudgetRow{row})

	if !strings.Contains(out, "未标定") {
		t.Fatalf("renderBudget 对 Calibrated == false 的行应印“未标定”，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "ag-uncal") {
		t.Fatalf("renderBudget 输出里找不到 ag-uncal，无法定位该行，实际输出：\n%s", out)
	}

	rowText := agentRowLines(out, "ag-uncal")
	if bareZeroPattern.MatchString(rowText) {
		t.Fatalf("renderBudget 把 ag-uncal 这一行 nil 的字段印成了裸 0，该行文字：\n%s", rowText)
	}
}

// TestRenderBudget_CalibratedNoCost_TokenAndBasisStillPrinted 挡的坏实现：
// (a) 因为 CostUSD == nil 就把整行当成未标定处理，连 token 一起藏起来；
// (b) 只印“已标定”三个字，不点名 Basis 那个具体的 turn_id。
func TestRenderBudget_CalibratedNoCost_TokenAndBasisStillPrinted(t *testing.T) {
	row := turn.BudgetRow{
		AgentID:      "ag-cal-nocost",
		Provider:     "pv-y",
		Calibrated:   true,
		InputTokens:  intPtr(4321),
		CostUSD:      nil,
		Basis:        "turn-abc-123",
		ArgvHeadroom: intPtr(100),
	}

	out := renderBudget([]turn.BudgetRow{row})

	if !strings.Contains(out, "4321") {
		t.Fatalf("renderBudget 应照印已标定行的 InputTokens=4321，不许因为没金额就藏起 token，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "turn-abc-123") {
		t.Fatalf("renderBudget 应点名 Basis=turn-abc-123，不能只印“已标定”三个字，实际输出：\n%s", out)
	}
}

// TestRenderBudget_ArgvHeadroomThreeStates 挡的坏实现：把负余量夹到 0，
// 或者把 nil 余量印成 0。
//
// 三行都给 Calibrated=true 且各自的 InputTokens 用同一个不含 0 的占位值 1，
// 避免“未标定”分支与本用例要测的余量分支互相干扰。nil 余量那一行用
// agentRowLines 做行级隔离，理由与上一条用例相同。
func TestRenderBudget_ArgvHeadroomThreeStates(t *testing.T) {
	rows := []turn.BudgetRow{
		{AgentID: "ag-pos", Provider: "pv", Calibrated: true, InputTokens: intPtr(1), Basis: "t1", ArgvHeadroom: intPtr(9876)},
		{AgentID: "ag-neg", Provider: "pv", Calibrated: true, InputTokens: intPtr(1), Basis: "t2", ArgvHeadroom: intPtr(-543)},
		{AgentID: "ag-nil", Provider: "pv", Calibrated: true, InputTokens: intPtr(1), Basis: "t3", ArgvHeadroom: nil},
	}

	out := renderBudget(rows)

	if !strings.Contains(out, "9876") {
		t.Fatalf("renderBudget 应照印正余量 9876，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "-543") {
		t.Fatalf("renderBudget 应照印负余量 -543，不许夹到 0，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "ag-nil") {
		t.Fatalf("renderBudget 输出里找不到 ag-nil，无法定位该行，实际输出：\n%s", out)
	}

	nilRow := agentRowLines(out, "ag-nil")
	if !strings.Contains(nilRow, "余量未知") {
		t.Fatalf("renderBudget 对 ArgvHeadroom == nil 应在该行印“余量未知”，该行文字：\n%s", nilRow)
	}
	if bareZeroPattern.MatchString(nilRow) {
		t.Fatalf("renderBudget 把 ag-nil 这一行 nil 的余量印成了裸 0，该行文字：\n%s", nilRow)
	}
}

// TestRenderBudget_NoDeltaMemberStaysOnTable 挡的坏实现：把 NoDelta == true
// 的成员从表里滤掉，只印真正要发言的那些行。
func TestRenderBudget_NoDeltaMemberStaysOnTable(t *testing.T) {
	rows := []turn.BudgetRow{
		{AgentID: "ag-normal", Provider: "pv", Calibrated: true, InputTokens: intPtr(10), Basis: "t1"},
		{AgentID: "ag-nodelta", Provider: "pv", Calibrated: true, InputTokens: intPtr(20), Basis: "t2", NoDelta: true},
	}

	out := renderBudget(rows)

	if !strings.Contains(out, "ag-normal") {
		t.Fatalf("renderBudget 应含 ag-normal 那一行，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "ag-nodelta") {
		t.Fatalf("renderBudget 不许把 NoDelta 成员从表里滤掉，实际输出：\n%s", out)
	}
}

// TestRenderSettlement_CacheReadUnknownAndColdZero_NoBarezero 是本组主断言
// （契约 4.6a.8 验收 7）。挡的坏实现：cache_read 全员未报（HaveCacheRead ==
// false）、且 Cold == 0（“没人够格被数”）时，把 cold 那一格原样印成裸 0，
// 让人读成“确实没有冷启动”。
//
// 判别力设计（差分模板法，因为不知道实现方选的具体标签/排版）：
//
//   - 场景 A（本用例真正要测的场景）：HaveCacheRead=false, CacheRead=0,
//     Cold=0。
//   - 场景 B（对照场景）：只把 HaveCacheRead 改成 true、CacheRead 改成
//     888，其余字段与场景 A 完全相同。场景 B 里 Cold=0 是一个“合法”的
//     裸 0——已经有人报了 cache_read、且他们里没人冷启动，这时 cold=0
//     不是“没人够格被数”，照契约是可以裸印的。场景 B 因此天然示范了
//     “这套实现平时怎么印 cold=0”。
//   - 把其余所有计数字段都设成不含数字 0 的值（Spawned=3, SpawnFailed=1,
//     UsageUnknown=1, CostUnreported=1, forecastCold=4, unexpectedCold 给
//     一个非空名单，token 与金额都给不含 0 的值），这样场景 B 的整段输出
//     里“独立的 0”有且只有 Cold 这一个来源——这是差分手法成立的前提，
//     用代码里的 precondition 检查兜底：场景 B 里 \b0\b 必须恰好命中
//     1 次，不满足就直接 Fatal 说明前提被破坏，不勉强继续判断（也可能是
//     实现方比契约要求的更保守，见最终报告）。
//   - 从场景 B 里那唯一的 0 出发，向前/向后各扩到最近的 ASCII 数字或换行
//     为止，取出它贴身的文字模板 template（比如 “cold: ” + “0” + “\n”，
//     具体贴身文字取决于实现方的排版，本测试不需要知道）。
//   - 断言场景 A 的输出不包含这个 template。
//
// 为什么这有判别力：如果实现方对 cold 的渲染完全不管 HaveCacheRead，两种
// 场景里 cold 那一格贴身的文字会逐字节相同，template 一定能在场景 A 里
// 找到，用例失败——精确挡住“裸印 cold=0”这个坏实现。如果实现方按契约改
// 成印“未知”或带限定语，场景 A 里 cold 那一格贴身的文字就会跟场景 B 不
// 一样（多了限定语、换了词），template 在场景 A 里就搜不到，用例通过。
// 这个判定既不依赖猜“未知”两个字具体印在哪，也不会被“印了限定语的 0”
// 误判成裸 0——因为 template 是贴身文字加 0 一起比较，多了限定语文字之后
// template 就不再是场景 A 输出的子串。
func TestRenderSettlement_CacheReadUnknownAndColdZero_NoBarezero(t *testing.T) {
	base := turn.RoundSettlement{
		Spawned:        3,
		SpawnFailed:    1,
		UsageUnknown:   1,
		CostUnreported: 1,
		Cold:           0,

		InputTokens:    7777,
		OutputTokens:   555,
		CacheCreate:    111,
		ThinkingTokens: 222,

		HaveIn:          true,
		HaveOut:         true,
		HaveCacheCreate: true,
		HaveThinking:    true,
		HaveCost:        true,
		CostUSD:         3.5,
	}

	sceneA := base
	sceneA.HaveCacheRead = false
	sceneA.CacheRead = 0

	sceneB := base
	sceneB.HaveCacheRead = true
	sceneB.CacheRead = 888

	forecastCold := 4
	unexpectedCold := []string{"ag-surprise"}

	// label 两处给同一个值（"发言轮"）：本用例比较的是场景 A 与场景 B 之间
	// cache_read/cold 渲染的差异，不是比较两种轮次标签，两处标签不同会在
	// 输出最前面多引入一条无关差异，混进差分法要比较的"其余部分"。
	outA := renderSettlement(sceneA, forecastCold, unexpectedCold, "发言轮")
	outB := renderSettlement(sceneB, forecastCold, unexpectedCold, "发言轮")

	if !strings.Contains(outA, "未知") {
		t.Fatalf("renderSettlement 对 HaveCacheRead == false 应印“未知”，实际输出：\n%s", outA)
	}
	if !strings.Contains(outA, "7777") {
		t.Fatalf("renderSettlement 应照印 HaveIn == true 的 InputTokens=7777，证明不是把所有列都印成未知，实际输出：\n%s", outA)
	}

	matches := bareZeroPattern.FindAllStringIndex(outB, -1)
	if len(matches) != 1 {
		t.Fatalf("差分法前提不成立：对照输出（场景 B）里独立的 0 应恰好 1 处（本用例特意布置成只有 cold 会是 0），实际 %d 处，无法据此判定场景 A 是否裸印 cold=0。场景 B 输出：\n%s", len(matches), outB)
	}

	template := extractZeroTemplate(outB, matches[0][0], matches[0][1])
	if strings.Contains(outA, template) {
		t.Fatalf("renderSettlement 把 cold=0 裸印了出来（场景 A 与场景 B 里 cold 单元格贴身文字逐字节相同）：模板 %q\n场景 A：\n%s\n场景 B：\n%s", template, outA, outB)
	}
}

// TestRenderSettlement_SpawnFailedNotMergedIntoSpawned 挡的坏实现：把
// spawn_failed 并进 spawned 一起印（比如渲染 spawned+spawn_failed 的和，
// 或者干脆不单独列 spawn_failed）。
//
// 判别力设计：除 Spawned=2、SpawnFailed=1 之外的字段都避开 3 这个值，这样
// “输出里出现字符串 3”基本上只能来自“把两者相加”这种坏实现。
//
// 残留风险（写进最终报告）：如果实现方另外印一个“共尝试 N 人”之类的衍生
// 合计，且那个合计恰好也是 3，子串断言分不出这是“合并”还是另一个无关的
// 合计——这个风险没有完全消掉，只是尽量把其它数字都错开了 3。
func TestRenderSettlement_SpawnFailedNotMergedIntoSpawned(t *testing.T) {
	s := turn.RoundSettlement{
		Spawned:        2,
		SpawnFailed:    1,
		UsageUnknown:   0,
		CostUnreported: 0,
		Cold:           0,
		InputTokens:    500,
		HaveIn:         true,
	}

	out := renderSettlement(s, 0, nil, "发言轮")

	if !strings.Contains(out, "2") {
		t.Fatalf("renderSettlement 应含 Spawned=2，实际输出：\n%s", out)
	}
	if !strings.Contains(out, "1") {
		t.Fatalf("renderSettlement 应含 SpawnFailed=1，实际输出：\n%s", out)
	}
	if strings.Contains(out, "3") {
		t.Fatalf("renderSettlement 不许把 spawn_failed 并进 spawned（2+1=3 不应出现），实际输出：\n%s", out)
	}
}

// TestRenderSettlement_UsageUnknownQualifiesAggregate 挡的坏实现：印一个
// 只覆盖已解析用量成员的部分和（100），却不说明它是部分和，让人把 100
// 读成全体 5 个成员的合计。
//
// 判别力设计：只是“100”和“2”都出现还不够——“2”完全可能来自别的无关计数。
// 用 windowAround 把断言收紧到“100”字面出现处附近的一小段窗口，要求窗口
// 内同时含数字 2 与至少一个中文/字母字符（证明真有说明文字，不是又一个
// 孤立数字）。窗口宽度两侧各留了宽裕的余量，覆盖契约给的例句“（2 名成员
// 用量未知，合计只含其余人）”这类长度的限定语，也给别的 Have*=false 占
// 位词留出空间。
//
// 局限（写进最终报告）：hasHanOrLetter 这一半判别力有限——这段输出本来就
// 全是中文，“窗口内有没有中文”本身不太可能为假；真正有判别力的是“2 是否
// 落在 100 附近的窗口内”这一半。
func TestRenderSettlement_UsageUnknownQualifiesAggregate(t *testing.T) {
	s := turn.RoundSettlement{
		Spawned:      5,
		UsageUnknown: 2,
		InputTokens:  100,
		HaveIn:       true,
	}

	out := renderSettlement(s, 0, nil, "发言轮")

	if !strings.Contains(out, "100") {
		t.Fatalf("renderSettlement 应含 InputTokens 合计 100，实际输出：\n%s", out)
	}

	idx := strings.Index(out, "100")
	if idx < 0 {
		t.Fatalf("renderSettlement 找不到 100，实际输出：\n%s", out)
	}
	window := windowAround(out, idx, len("100"), 30, 150)

	if !strings.Contains(window, "2") {
		t.Fatalf("renderSettlement 的 100 附近应出现用量未知的成员数 2，不能只印部分和 100 而不说明，实际输出：\n%s\n窗口：%q", out, window)
	}
	if !hasHanOrLetter(window) {
		t.Fatalf("renderSettlement 的 100 附近应带文字限定语，不能只是数字，实际输出：\n%s\n窗口：%q", out, window)
	}
}

// TestRenderSettlement_LabelPrefixesFirstLineOnly 钉住片 3d 第二刀新加的
// 第四个参数 label（round.go 现查：renderSettlement 用
// fmt.Fprintf(&b, "%s结算\n", label) 在输出最前面多印一行，之后的每一行
// 不再引用 label）。挡的坏实现有两种：
//
//   - (a) label 没有真的印到输出里（参数被吞掉、被忽略、或者两种标签
//     印出来的字样不可区分）；
//   - (b) label 顺手影响到了标题行之外的其它文字（比如格式化字符串被
//     改坏，或者 label 被拼接进了别处）。
//
// 判别力设计：同一份 RoundSettlement/forecastCold/unexpectedCold 入参，
// 只把 label 从 "发言轮" 换成 "裁判轮" 分别渲染两次（与生产代码
// round.go 里 runOneRound 实际传的两个字面量一致，现查 line 148/165）。
//
//   - 断言一、二挡坏实现 (a) 的"没印出来"那一半：分别要求各自输出含
//     "发言轮结算" / "裁判轮结算"。
//   - 断言三挡坏实现 (a) 的"两种标签印出来的东西分不清"那一半：要求
//     label="裁判轮" 的输出不含 "发言轮结算"——只有断言一二的话，如果
//     实现干脆把两种标签字样都写死印出来（不真的看参数），断言一二仍然
//     会通过，挡不住这种坏实现；加上这条"排他"断言才有判别力。
//   - 断言四挡坏实现 (b)：把两段输出各自按第一个换行切开
//     （strings.Cut(out, "\n")），只保留"第一行之后"的剩余部分比较，
//     要求逐字节相同。因为两次调用的 RoundSettlement/forecastCold/
//     unexpectedCold 完全相同，唯一的入参差异是 label，而 label 按现查
//     的实现只用在标题行，所以正确实现下"标题行之后"的剩余部分必然
//     相同；如果 label 泄漏进了其它行、或者顺手改坏了其它行的格式化
//     字符串，剩余部分就会不同，用例失败。
func TestRenderSettlement_LabelPrefixesFirstLineOnly(t *testing.T) {
	s := turn.RoundSettlement{
		Spawned:      2,
		SpawnFailed:  1,
		UsageUnknown: 1,
		InputTokens:  999,
		HaveIn:       true,
		HaveCost:     true,
		CostUSD:      1.25,
	}
	forecastCold := 1
	unexpectedCold := []string{"ag-x"}

	outSpeak := renderSettlement(s, forecastCold, unexpectedCold, "发言轮")
	outJudge := renderSettlement(s, forecastCold, unexpectedCold, "裁判轮")

	if !strings.Contains(outSpeak, "发言轮结算") {
		t.Fatalf("label=\"发言轮\" 时输出应含“发言轮结算”，实际输出：\n%s", outSpeak)
	}
	if !strings.Contains(outJudge, "裁判轮结算") {
		t.Fatalf("label=\"裁判轮\" 时输出应含“裁判轮结算”，实际输出：\n%s", outJudge)
	}
	if strings.Contains(outJudge, "发言轮结算") {
		t.Fatalf("label=\"裁判轮\" 时输出不该含“发言轮结算”（这一条挡住“两种标签字样都写死印出来、"+
			"不真的看参数”这种坏实现），实际输出：\n%s", outJudge)
	}

	_, restSpeak, okSpeak := strings.Cut(outSpeak, "\n")
	_, restJudge, okJudge := strings.Cut(outJudge, "\n")
	if !okSpeak || !okJudge {
		t.Fatalf("两段输出都应当不止一行（第一行是标题行，之后还有正文），实际：\n发言轮=%q\n裁判轮=%q", outSpeak, outJudge)
	}
	if restSpeak != restJudge {
		t.Fatalf("除标题行外，两段输出的其余部分应当逐字节相同（同一份 RoundSettlement/forecastCold/"+
			"unexpectedCold 入参，唯一差异只有 label），实际不同，说明 label 泄漏进了标题行之外的地方：\n"+
			"发言轮剩余部分：\n%s\n裁判轮剩余部分：\n%s", restSpeak, restJudge)
	}
}

// ============================================================
// F. renderBudget 的 k == 0（验收 18）
// ============================================================

// TestRenderBudget_AnotherUncalibratedLineOmittedWhenZero 挡的坏实现：
// renderBudget 把"另有 k 名未标定"这一行在 k==0 时也印出来（"另有 0 名
// 未标定"），而不是任务书要求的整行省略。
//
// 场景 A：两行都 Calibrated=true（k=0），断言输出不含"另有"。
// 场景 B（对照组）：两行里有一行 Calibrated=false（k=1），断言输出含
// "另有 1 名未标定"——这条字面文案是任务书原句直接给出的占位词，不是
// 猜的。两个场景都用两行而不是一行，避免"只有一个成员时怎么措辞"这种
// 边界情况让实现方选了别的、更自然的单数句式（比如"另有一名"而不是
// "另有 1 名"），那样字面匹配会跟本用例的判别力无关地误报。
func TestRenderBudget_AnotherUncalibratedLineOmittedWhenZero(t *testing.T) {
	allCalibrated := []turn.BudgetRow{
		{AgentID: "ag-cal-1", Provider: "pv", Calibrated: true, InputTokens: intPtr(10), Basis: "t1"},
		{AgentID: "ag-cal-2", Provider: "pv", Calibrated: true, InputTokens: intPtr(20), Basis: "t2"},
	}
	outAllCal := renderBudget(allCalibrated)
	if strings.Contains(outAllCal, "另有") {
		t.Fatalf("全员已标定（k=0）时不该出现「另有」这一行，实际输出：\n%s", outAllCal)
	}

	mixed := []turn.BudgetRow{
		{AgentID: "ag-cal-1", Provider: "pv", Calibrated: true, InputTokens: intPtr(10), Basis: "t1"},
		{AgentID: "ag-uncal-1", Provider: "pv", Calibrated: false},
	}
	outMixed := renderBudget(mixed)
	if !strings.Contains(outMixed, "另有 1 名未标定") {
		t.Fatalf("对照组：k=1 时应当出现「另有 1 名未标定」，实际输出：\n%s", outMixed)
	}
}
