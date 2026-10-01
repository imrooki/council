package repl

// 本文件是 X-35a（/cost --round 不再把「轮次未知」的行静默滤掉）的测试腿产出，
// 覆盖冻结契约 E:\multi_model_cowork\x35a_cost_round_contract.md（第三版）
// 第 7 节验收清单第 1-10 条；另加一条契约外补强 TestX35aAgentFilterNeverAppendsTrailer，
// 依据第 4 节「不带 --round 的输出一个字节都不许变」覆盖 /cost <agent-id> 形态。
//
// 背对背声明：写这份文件时实现腿在并行按同一份契约改 internal/repl/cost.go 的
// cmdCost。本文件只依据契约正文与 HEAD（aea771f，相对契约 ground truth 89b0c7c
// 只多了文档提交）上已有的符号写成，没有读过实现腿的任何产出。
//
// 夹具约定（契约原话的硬要求）：
//   - round id 一律不带数字（round-alpha / round-beta / round-nope）：
//     要断言的计数是个位数，round id 带数字会让「含那个数」的断言恒真。
//     agent id 同理用 ag_a / ag_b / ag_c。
//   - 追加行与空结果那一句断言整行逐字相等，不只查子串。
//
// 命令输出走真实的 REPL 分派，复用 step10d1_cost_test.go 的 replTurnOutputs
// 与 conversation_test.go 的 fakeStore（seedConversation / seedTurnUsage）。

import (
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/turn"
)

// 契约第 4 节冻结的逐字格式串。全角括号是 U+FF08 / U+FF09，照抄契约字节，不重敲。
const (
	x35aEmptyRoundFmt   = "这一轮（round_id=%s）没有记账行\n"
	x35aNoLedger        = "本会话还没有记过账\n"
	x35aTrailerPlain    = "未计入本轮：另有 %d 行 round_id 为空，不知道属于哪一轮；不带 --round 的 /cost 能看到它们"
	x35aTrailerWithLost = "未计入本轮：另有 %d 行 round_id 为空，不知道属于哪一轮（其中 %d 行是崩溃后启动对账补的账）；不带 --round 的 /cost 能看到它们"
	x35aTrailerMarker   = "未计入本轮"
)

// x35aRow 造一行 turn_usage。CreatedAt 按 seq 递增，保证 fakeStore 读回的顺序
// 与种入顺序一致；用量与金额列全留 nil，本刀不关心它们。
func x35aRow(seq int, agentID, roundID string, outcome turn.Outcome) conversation.TurnUsage {
	at := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	return conversation.TurnUsage{
		TurnID:    "x35a-turn-" + string(rune('a'+seq)),
		AgentID:   agentID,
		RoundID:   roundID,
		Outcome:   int(outcome),
		CreatedAt: at.Add(time.Duration(seq) * time.Minute),
	}
}

// x35aRunCost 新开一条会话、种入 rows、/use 之后跑一条 cost 命令，返回那条命令
// 单独产生的输出。每次调用都是一个新的 fakeStore，用例之间互不串数据。
func x35aRunCost(t *testing.T, rows []conversation.TurnUsage, costLine string) string {
	t.Helper()
	svc, f := newTestService(t)
	const convID = "conv_xa_cost_round_aaaaaaaaaaaaaaaaaa"
	f.seedConversation(convID, "x35a cost round")
	for _, u := range rows {
		u.ConvID = convID
		f.seedTurnUsage(u)
	}
	outs := replTurnOutputs(t, svc, "/use "+convID, costLine)
	return outs[1]
}

// x35aLines 把一条命令的输出切成行。输出必须以换行结尾：少了结尾换行说明
// 最后一行被拼进了提示符，逐行相等的断言会在错误的地方失败，这里先拦下。
func x35aLines(t *testing.T, out string) []string {
	t.Helper()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("输出不以换行结尾: %q", out)
	}
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

// x35aFields 把一行表格按空白切成列（tabwriter 用空格补齐）。
// 调用方先断言过总行数与表头位置，这里只做切分。
func x35aFields(line string) []string {
	return strings.Fields(line)
}

// x35aCoreFixture 是验收第 1 条的夹具，第 6 条原样复用（契约要求第 6 条的夹具
// 必须含空 round 行）。
//
// 两行空 round 挂在第三个成员 ag_c 上，不挂在 A、B 上：一个自己写过滤、让空串
// 也「匹配」的实现（契约第 4 节第 2 点禁止的那个）会让表格多出 ag_c 一行，
// 第 1 条的总行数断言就翻红。挂在 A、B 上的话，它只改 TURNS，也抓得到，但
// 放第三个成员让失败信息更直白。
func x35aCoreFixture() []conversation.TurnUsage {
	return []conversation.TurnUsage{
		x35aRow(0, "ag_a", "round-alpha", turn.OutcomeSpoke),
		x35aRow(1, "ag_b", "round-alpha", turn.OutcomeSpoke),
		x35aRow(2, "ag_c", "", turn.OutcomeLost),
		x35aRow(3, "ag_c", "", turn.OutcomeSpoke),
	}
}

// TestX35aRoundReportsUnknownRowsWithLostCount 是验收第 1 条（要害），并兼第 9、
// 10 条。
//
// unknown=2 与 unknownLost=1 取不同的数：一个把两个计数打反、或括号里填 unknown
// 的实现，整行逐字相等会失败。改之前的实现一个字都不说，末行是 ag_b 的表格行，
// 这条必红。
func TestX35aRoundReportsUnknownRowsWithLostCount(t *testing.T) {
	out := x35aRunCost(t, x35aCoreFixture(), "/cost --round round-alpha")
	lines := x35aLines(t, out)

	// 总行数 = 表头 1 + 数据行 2（A、B）+ 追加行 1。这一条让「恰好一行」可以为假：
	// 追加行打两遍、多一个空行、多出一行 ag_c 都会让它变。
	if len(lines) != 4 {
		t.Fatalf("输出 %d 行，期望 4 行（表头 + ag_a + ag_b + 追加行）；原始输出:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "AGENT") {
		t.Errorf("第 1 行不是表头: %q；原始输出:\n%s", lines[0], out)
	}

	// 第 1 条：末尾恰好是带括号的那一版，整行逐字相等。
	wantTrailer := strings.Replace(strings.Replace(x35aTrailerWithLost, "%d", "2", 1), "%d", "1", 1)
	if lines[3] != wantTrailer {
		t.Errorf("末行 = %q\n期望   %q\n原始输出:\n%s", lines[3], wantTrailer, out)
	}
	if n := strings.Count(out, x35aTrailerMarker); n != 1 {
		t.Errorf("「%s」出现 %d 次，期望恰好 1 次；原始输出:\n%s", x35aTrailerMarker, n, out)
	}

	// 第 9 条：追加行在表头之后。与上面「末行逐字相等」判别力重叠：追加行若写在
	// tw.Flush() 之前，它会跑到表格上方，上面那条已经红了。这里单独钉位置，
	// 让失败信息直接说出是顺序错了。
	headerAt, trailerAt := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "AGENT") && headerAt < 0 {
			headerAt = i
		}
		if strings.HasPrefix(l, x35aTrailerMarker) && trailerAt < 0 {
			trailerAt = i
		}
	}
	if headerAt < 0 || trailerAt < 0 || trailerAt <= headerAt {
		t.Errorf("追加行（第 %d 行）不在表头（第 %d 行）之后；原始输出:\n%s", trailerAt, headerAt, out)
	}

	// 第 10 条：空 round 的行不进表格的任何一个数。表格只有 ag_a、ag_b 两行，
	// 各自 TURNS=1、LOST=0，合计 TURNS=2。
	// 结构上几乎恒真：表格由冻结不改的 filterCostByRound + summarizeCost 产出；
	// 只有实现绕开 filterCostByRound 自己写一个放行空串的过滤时它才会红。
	wantAgents := []string{"ag_a", "ag_b"}
	totalTurns := 0
	for i, want := range wantAgents {
		cols := x35aFields(lines[1+i])
		if len(cols) < 5 {
			t.Fatalf("表格第 %d 行列数 %d 不足: %q", i+1, len(cols), lines[1+i])
		}
		if cols[0] != want {
			t.Errorf("表格第 %d 行 AGENT = %q，期望 %q；原始输出:\n%s", i+1, cols[0], want, out)
		}
		if cols[1] != "1" {
			t.Errorf("%s 的 TURNS = %q，期望 1（空 round 行不许计入）", want, cols[1])
		}
		if cols[3] != "0" {
			t.Errorf("%s 的 LOST = %q，期望 0（空 round 的 Lost 行不许计入）", want, cols[3])
		}
		if cols[1] == "1" {
			totalTurns++
		}
	}
	if totalTurns != 2 {
		t.Errorf("表格 TURNS 合计按行核得 %d，期望 2", totalTurns)
	}

	// 额外一道差分：--round 模式下表格部分（去掉追加行）必须与「只含 round-alpha
	// 两行的会话」上不带参数的 /cost 逐字节相同。追加行不许改动表格本身。
	ground := x35aRunCost(t, x35aCoreFixture()[:2], "/cost")
	tablePart := strings.Join(lines[:3], "\n") + "\n"
	if tablePart != ground {
		t.Errorf("--round 模式的表格部分与只含本轮行的对照不同。\n对照:\n%s\n本用例表格部分:\n%s", ground, tablePart)
	}
}

// TestX35aRoundUnknownWithoutLostUsesPlainTrailer 是验收第 2 条（覆盖分支）：
// 空 round 行里没有 Lost 时打不带括号的那一版。
func TestX35aRoundUnknownWithoutLostUsesPlainTrailer(t *testing.T) {
	rows := []conversation.TurnUsage{
		x35aRow(0, "ag_a", "round-alpha", turn.OutcomeSpoke),
		x35aRow(1, "ag_c", "", turn.OutcomeSpoke),
	}
	out := x35aRunCost(t, rows, "/cost --round round-alpha")
	lines := x35aLines(t, out)
	if len(lines) != 3 {
		t.Fatalf("输出 %d 行，期望 3 行（表头 + ag_a + 追加行）；原始输出:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "AGENT") {
		t.Errorf("第 1 行不是表头: %q", lines[0])
	}
	want := strings.Replace(x35aTrailerPlain, "%d", "1", 1)
	if lines[2] != want {
		t.Errorf("末行 = %q\n期望   %q\n原始输出:\n%s", lines[2], want, out)
	}
}

// TestX35aRoundNoTrailerWhenOnlyOtherRoundsFiltered 是验收第 3 条：
// 没有空 round 的行、只有别的 round 的行时，一个字都不多打。
//
// 把「被过滤掉的行数」当 unknown 的实现在这里会算出 1 并打出追加行，这条必红。
func TestX35aRoundNoTrailerWhenOnlyOtherRoundsFiltered(t *testing.T) {
	rows := []conversation.TurnUsage{
		x35aRow(0, "ag_a", "round-alpha", turn.OutcomeSpoke),
		x35aRow(1, "ag_b", "round-alpha", turn.OutcomeSpoke),
		x35aRow(2, "ag_c", "round-beta", turn.OutcomeLost),
	}
	out := x35aRunCost(t, rows, "/cost --round round-alpha")
	if strings.Contains(out, x35aTrailerMarker) {
		t.Errorf("没有空 round 行，输出却含「%s」；原始输出:\n%s", x35aTrailerMarker, out)
	}
	// 同时钉住形状：表头 + A + B，没有第四行。上面那条是契约点名的断言，
	// 这一条防的是换了措辞的多余行。
	lines := x35aLines(t, out)
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "AGENT") {
		t.Errorf("期望恰好 3 行（表头 + ag_a + ag_b）；原始输出:\n%s", out)
	}
}

// TestX35aRoundTypoSaysThisRoundHasNoRows 是验收第 4 条：round id 敲错，会话里
// 有账但这一轮没有。输出恰好是一行空结果那一句——整串相等同时蕴含「不含
// 本会话还没有记过账」「不含 AGENT」两条；两条否定断言仍单独写出，让失败信息
// 直接指出是哪一种错。改前打的是「本会话还没有记过账」，必红。
func TestX35aRoundTypoSaysThisRoundHasNoRows(t *testing.T) {
	rows := []conversation.TurnUsage{
		x35aRow(0, "ag_a", "round-alpha", turn.OutcomeSpoke),
		x35aRow(1, "ag_b", "round-alpha", turn.OutcomeSpoke),
	}
	out := x35aRunCost(t, rows, "/cost --round round-nope")
	want := strings.Replace(x35aEmptyRoundFmt, "%s", "round-nope", 1)
	if out != want {
		t.Errorf("输出 = %q\n期望恰好 %q", out, want)
	}
	if strings.Contains(out, strings.TrimSuffix(x35aNoLedger, "\n")) {
		t.Errorf("会话有账，输出却说「本会话还没有记过账」: %q", out)
	}
	if strings.Contains(out, "AGENT") {
		t.Errorf("空结果打出了表头: %q", out)
	}
}

// TestX35aRoundOnEmptyConversationStillSaysNoLedger 是验收第 5 条（覆盖分支）：
// 过滤前就是空的，那句话此时是真的，照旧打。
func TestX35aRoundOnEmptyConversationStillSaysNoLedger(t *testing.T) {
	out := x35aRunCost(t, nil, "/cost --round round-alpha")
	if out != x35aNoLedger {
		t.Errorf("输出 = %q，期望恰好 %q", out, x35aNoLedger)
	}
}

// TestX35aNoRoundFlagNeverAppendsTrailer 是验收第 6 条：不带 --round 时不打追加行。
// 夹具必须含空 round 行（这里原样用第 1 条的），否则「不带 --round 也追加」的
// 坏实现在这里没有东西可追加，断言就没牙了。契约点名：这是那个坏实现在包内的
// 唯一防线。
func TestX35aNoRoundFlagNeverAppendsTrailer(t *testing.T) {
	rows := x35aCoreFixture()
	// 先确认夹具前提成立：确实含空 round 行。前提被改掉时这里先红，不让下面
	// 的否定断言悄悄变成恒真。
	unknown := 0
	for _, u := range rows {
		if u.RoundID == "" {
			unknown++
		}
	}
	if unknown == 0 {
		t.Fatalf("夹具不含空 round 行，本用例失去判别力")
	}

	out := x35aRunCost(t, rows, "/cost")
	if strings.Contains(out, x35aTrailerMarker) {
		t.Errorf("不带 --round 的 /cost 打出了「%s」；原始输出:\n%s", x35aTrailerMarker, out)
	}
	lines := x35aLines(t, out)
	want := len(summarizeCost(rows)) + 1
	if len(lines) != want {
		t.Errorf("输出 %d 行，期望 %d 行（len(summarizeCost(全部行)) + 表头）；原始输出:\n%s", len(lines), want, out)
	}
	if !strings.HasPrefix(lines[0], "AGENT") {
		t.Errorf("第 1 行不是表头: %q", lines[0])
	}
}

// TestX35aTypoRoundWithOnlyLostUnknownPrintsBothLines 是验收第 7 条（组合）：
// 这一轮一行都没有，但会话里有一行空 round 的 Lost。输出恰好两行、顺序固定。
// 「空结果那一支提前 return」漏第二行；「往下走打空表」多出表头——整串相等两种都抓。
func TestX35aTypoRoundWithOnlyLostUnknownPrintsBothLines(t *testing.T) {
	rows := []conversation.TurnUsage{
		x35aRow(0, "ag_a", "round-alpha", turn.OutcomeSpoke),
		x35aRow(1, "ag_b", "round-alpha", turn.OutcomeSpoke),
		x35aRow(2, "ag_c", "", turn.OutcomeLost),
	}
	out := x35aRunCost(t, rows, "/cost --round round-nope")
	want := strings.Replace(x35aEmptyRoundFmt, "%s", "round-nope", 1) +
		strings.Replace(strings.Replace(x35aTrailerWithLost, "%d", "1", 1), "%d", "1", 1) + "\n"
	if out != want {
		t.Errorf("输出 = %q\n期望恰好 %q", out, want)
	}
}

// TestX35aUnknownLostCountsOnlyEmptyRoundLost 是验收第 8 条：unknownLost 只数
// 空 round 行里的 Lost。两行 Lost 都带真实 round id（模拟写侧修好之后），
// 空 round 行只有一行 Spoke，所以期望的是不带括号那一版、另有 1 行。
//
// 数全部 Lost 的实现会打「其中 2 行」；数「被过滤掉的行里的 Lost」的实现会打
// 「其中 1 行」——两者都带括号，整行逐字相等都红。
func TestX35aUnknownLostCountsOnlyEmptyRoundLost(t *testing.T) {
	rows := []conversation.TurnUsage{
		x35aRow(0, "ag_a", "round-alpha", turn.OutcomeSpoke),
		x35aRow(1, "ag_b", "round-alpha", turn.OutcomeLost),
		x35aRow(2, "ag_c", "round-beta", turn.OutcomeLost),
		x35aRow(3, "ag_c", "", turn.OutcomeSpoke),
	}
	out := x35aRunCost(t, rows, "/cost --round round-alpha")
	lines := x35aLines(t, out)
	if len(lines) != 4 {
		t.Fatalf("输出 %d 行，期望 4 行（表头 + ag_a + ag_b + 追加行）；原始输出:\n%s", len(lines), out)
	}
	want := strings.Replace(x35aTrailerPlain, "%d", "1", 1)
	if lines[3] != want {
		t.Errorf("末行 = %q\n期望   %q\n原始输出:\n%s", lines[3], want, out)
	}
}

// TestX35aAgentFilterNeverAppendsTrailer 是契约外的补强，依据第 4 节「不带 --round
// 的输出一个字节都不许变」：/cost <agent-id> 也是不带 --round 的形态，第 6 条只
// 点了无参那一种。夹具里 ag_c 的两行恰好都是空 round，一个在 agent 模式下也
// 数空 round 并追加的实现会在这里多打一行。
func TestX35aAgentFilterNeverAppendsTrailer(t *testing.T) {
	out := x35aRunCost(t, x35aCoreFixture(), "/cost ag_c")
	if strings.Contains(out, x35aTrailerMarker) {
		t.Errorf("/cost <agent-id> 打出了「%s」；原始输出:\n%s", x35aTrailerMarker, out)
	}
	lines := x35aLines(t, out)
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "AGENT") {
		t.Errorf("期望恰好 2 行（表头 + ag_c）；原始输出:\n%s", out)
	}
}
