package repl

// 本文件是 X-38bd（/cost 不再把「没报」印成 0）的测试腿产出，覆盖冻结契约
// x38bd_cost_unknown_contract.md（v2.2）第 4 节验收 A、B、D 三组。契约落地后已删，
// 语义与局限记在 docs/00_DECISIONS.md #38(b)(d)(f)。B16 与 D1 是 v2.2 落地审查后补的。
//
// 背对背声明：写这份文件时实现腿在并行按同一份契约改 internal/repl/cost.go。
// 本文件只依据契约正文与 HEAD（65ad2eb）上已有的符号写成：只用 summarizeCost、
// costRow.ColdStarts 两个既有符号，新计数字段的名字由实现方定，这里一概不碰。
//
// 断言纪律：
//   - 表格断言取某成员那一行，strings.Fields 切开，先断言恰好 14 格，再按下标逐格整格相等。
//     期望值一律写成字面量，不调 formatCostUSD / summarizeCost 去算期望。
//   - 每条表格用例同时断言输出行数（表头 + 成员数），多一行少一行都藏不住；
//     每条都断言表头逐项等于冻结串切开的 14 个名字。
//   - 「新行为」的每一格都有只差一处的对照（见各用例注释里的「对照」）。
//
// 复用帮手：x35a_cost_round_test.go 的 x35aRunCost（每次新开 fakeStore 与会话）、
// x35aLines（要求以换行结尾再切行）。

import (
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/turn"
)

// x38bdHeader 是契约第 3 节冻结的表头字面量（源码里列之间一个 tab）。
const x38bdHeader = "AGENT\tTURNS\tSPOKE\tLOST\tREFUSED\tIN\tCACHE_READ\tCACHE_CREATE\tOUT\tCOST_USD\tCOST_UNKNOWN\tCOLD\tUSAGE_UNKNOWN\tCOLD_UNKNOWN"

// x38bdCols 是表格的列数。表头与每一数据行都必须恰好这么多格。
const x38bdCols = 14

func x38bdInt(v int) *int         { return &v }
func x38bdF64(v float64) *float64 { return &v }
func x38bdAt(seq int) time.Time {
	return time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Minute)
}
func x38bdTurnID(seq int) string  { return "x38bd-turn-" + string(rune('a'+seq)) }
func x38bdOut(o turn.Outcome) int { return int(o) }

// x38bdShape 按契约 §4.0 的标准夹具表造一行。数值逐格照抄契约，未列出的字段取零值。
// 每次调用都新建指针，行与行之间不共享底层变量。
func x38bdShape(t *testing.T, name, agentID string, seq int) conversation.TurnUsage {
	t.Helper()
	u := conversation.TurnUsage{TurnID: x38bdTurnID(seq), AgentID: agentID, CreatedAt: x38bdAt(seq)}
	switch name {
	case "S1":
		u.Outcome = x38bdOut(turn.OutcomeSpoke)
		u.InputTokens, u.CacheRead, u.CacheCreate, u.OutputTokens, u.ThinkingTokens = x38bdInt(100), x38bdInt(10), x38bdInt(1), x38bdInt(1000), x38bdInt(5)
		u.CostUSD, u.ColdStart = x38bdF64(0.1), true
	case "S2":
		u.Outcome = x38bdOut(turn.OutcomeSpoke)
		u.InputTokens, u.CacheRead, u.CacheCreate, u.OutputTokens, u.ThinkingTokens = x38bdInt(200), x38bdInt(20), x38bdInt(2), x38bdInt(2000), x38bdInt(6)
		u.CostUSD, u.ColdStart = x38bdF64(0.2), false
	case "G1":
		u.Outcome = x38bdOut(turn.OutcomeSpoke)
		u.InputTokens, u.CacheRead, u.OutputTokens, u.ThinkingTokens = x38bdInt(100), x38bdInt(10), x38bdInt(1000), x38bdInt(5)
		u.ColdStart = true
	case "G2":
		u.Outcome = x38bdOut(turn.OutcomeSpoke)
		u.InputTokens, u.CacheRead, u.OutputTokens, u.ThinkingTokens = x38bdInt(200), x38bdInt(20), x38bdInt(2000), x38bdInt(6)
		u.ColdStart = false
	case "PR":
		u.Outcome = x38bdOut(turn.OutcomePostRejected)
		u.InputTokens, u.CacheRead, u.CacheCreate, u.OutputTokens, u.ThinkingTokens = x38bdInt(7), x38bdInt(3), x38bdInt(1), x38bdInt(70), x38bdInt(1)
		u.CostUSD, u.ColdStart = x38bdF64(0.05), true
	case "PF":
		u.Outcome = x38bdOut(turn.OutcomeProviderFailed)
		u.InputTokens, u.CacheRead, u.CacheCreate, u.OutputTokens, u.ThinkingTokens = x38bdInt(0), x38bdInt(0), x38bdInt(0), x38bdInt(0), x38bdInt(0)
		u.ColdStart = false
	case "M":
		u.Outcome = x38bdOut(turn.OutcomeSpoke)
		u.ColdStart = true
	case "Mcr0":
		// 契约外补强：M 只把 CacheRead 换成指向 0 的指针，是 A1 对照与 B9 对照共用的形状。
		u.Outcome = x38bdOut(turn.OutcomeSpoke)
		u.CacheRead = x38bdInt(0)
		u.ColdStart = true
	case "T":
		u.Outcome = x38bdOut(turn.OutcomeTimeout)
	case "T0":
		u.Outcome = x38bdOut(turn.OutcomeTimeout)
		u.InputTokens, u.CacheRead, u.CacheCreate, u.OutputTokens, u.ThinkingTokens = x38bdInt(0), x38bdInt(0), x38bdInt(0), x38bdInt(0), x38bdInt(0)
		u.CostUSD = x38bdF64(0)
	case "L":
		u.Outcome = x38bdOut(turn.OutcomeLost)
	case "SF":
		u.Outcome = x38bdOut(turn.OutcomeSpawnFailed)
	case "R":
		u.Outcome = x38bdOut(turn.OutcomeRefused)
	case "R0":
		// 契约 §4.0 R0（v2.2）：79547ea 之前写进库的 Refused 行，四列是指向 0 的指针，cache_create 与金额是 NULL。
		u.Outcome = x38bdOut(turn.OutcomeRefused)
		u.InputTokens, u.CacheRead, u.OutputTokens, u.ThinkingTokens = x38bdInt(0), x38bdInt(0), x38bdInt(0), x38bdInt(0)
		u.ColdStart = false
	case "C":
		u.Outcome = x38bdOut(turn.OutcomeCanceled)
	case "U":
		u.Outcome = x38bdOut(turn.OutcomeUnparsable)
	case "Mc":
		u.Outcome = x38bdOut(turn.OutcomeSpoke)
		u.CostUSD, u.ColdStart = x38bdF64(0.01), true
	default:
		t.Fatalf("未知夹具形状 %q", name)
	}
	return u
}

// x38bdRows 把一串形状名造成同一个成员的若干行，CreatedAt 依次递增。
func x38bdRows(t *testing.T, agentID string, names ...string) []conversation.TurnUsage {
	t.Helper()
	out := make([]conversation.TurnUsage, 0, len(names))
	for i, n := range names {
		out = append(out, x38bdShape(t, n, agentID, i))
	}
	return out
}

// x38bdTable 切开一段 /cost 输出：断言恰好 1 + wantMembers 行、表头逐项等于冻结串，
// 返回 AGENT -> 14 格。某一行格数不是 14 当场报错并跳过它（下标断言在错位的行上没有意义）。
func x38bdTable(t *testing.T, label, out string, wantMembers int) map[string][]string {
	t.Helper()
	lines := x35aLines(t, out)
	if len(lines) != 1+wantMembers {
		t.Fatalf("%s: 输出 %d 行，期望 %d 行（表头 + %d 个成员）；原始输出:\n%s", label, len(lines), 1+wantMembers, wantMembers, out)
	}
	wantHeader := strings.Split(x38bdHeader, "\t")
	gotHeader := strings.Fields(lines[0])
	if len(gotHeader) != len(wantHeader) {
		t.Errorf("%s: 表头 %d 格，期望 %d 格: %q", label, len(gotHeader), len(wantHeader), lines[0])
	} else {
		for i := range wantHeader {
			if gotHeader[i] != wantHeader[i] {
				t.Errorf("%s: 表头第 %d 格 = %q，期望 %q；表头行: %q", label, i, gotHeader[i], wantHeader[i], lines[0])
			}
		}
	}
	rows := make(map[string][]string, wantMembers)
	for _, l := range lines[1:] {
		f := strings.Fields(l)
		if len(f) != x38bdCols {
			t.Errorf("%s: 数据行 %d 格，期望 %d 格: %q；原始输出:\n%s", label, len(f), x38bdCols, l, out)
			continue
		}
		rows[f[0]] = f
	}
	return rows
}

// x38bdColName 用于失败信息：下标 -> 列名。
var x38bdColName = strings.Split(x38bdHeader, "\t")

// x38bdAssertRow 按下标逐格整格相等。want 必须恰好 14 格（写错期望当场报工装故障）。
func x38bdAssertRow(t *testing.T, label string, rows map[string][]string, want []string, out string) {
	t.Helper()
	if len(want) != x38bdCols {
		t.Fatalf("%s: 期望行写成了 %d 格（工装故障）", label, len(want))
	}
	got, ok := rows[want[0]]
	if !ok {
		t.Errorf("%s: 表里没有成员 %q 那一行；原始输出:\n%s", label, want[0], out)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: %s 的 %s（下标 %d）= %q，期望 %q；原始输出:\n%s", label, want[0], x38bdColName[i], i, got[i], want[i], out)
		}
	}
}

// ============================================================
// A. 冷启动：summarizeCost 直接调，断言 ColdStarts
// ============================================================

func x38bdColdStarts(t *testing.T, rows []conversation.TurnUsage) int {
	t.Helper()
	got := summarizeCost(rows)
	if len(got) != 1 {
		t.Fatalf("summarizeCost 返回 %d 组，期望 1 组: %+v", len(got), got)
	}
	return got[0].ColdStarts
}

// TestX38bdA1ColdStartNeedsCacheRead 是验收 A1：usage 对象缺失的成功轮（M：Spoke、
// ColdStart true、CacheRead NULL）不算冷启动。
// 对照（只差 CacheRead 一处）：CacheRead 改成指向 0 → 1。没有对照的话，一个把
// ColdStarts 恒记 0 的实现也能让主断言绿。
func TestX38bdA1ColdStartNeedsCacheRead(t *testing.T) {
	if got := x38bdColdStarts(t, x38bdRows(t, "ag_cost", "M")); got != 0 {
		t.Errorf("A1: 只有 M，ColdStarts = %d，期望 0（cache_read 为 NULL 时 cold_start 是用没填的 0 算出来的）", got)
	}
	if got := x38bdColdStarts(t, x38bdRows(t, "ag_cost", "Mcr0")); got != 1 {
		t.Errorf("A1 对照: M 的 CacheRead 改成 &0，ColdStarts = %d，期望 1", got)
	}
}

// TestX38bdA2PostRejectedColdCounts 是验收 A2：PostRejected 是 Ingested，冷启动照数。
// 漏掉 PostRejected 那一支的实现得 0。
func TestX38bdA2PostRejectedColdCounts(t *testing.T) {
	if got := x38bdColdStarts(t, x38bdRows(t, "ag_cost", "PR")); got != 1 {
		t.Errorf("A2: 只有 PR，ColdStarts = %d，期望 1", got)
	}
}

// TestX38bdA3TimeoutColdNotCounted 是验收 A3：Timeout、ColdStart true、CacheRead &0 → 0。
// 这一行 CacheRead 非 NULL，所以挡它的只能是 Ingested 门（A1 挡不住的那一种坏实现）。
// 对照（只差 Outcome 一处）：同样的字段、Outcome 换成 Spoke → 1。
func TestX38bdA3TimeoutColdNotCounted(t *testing.T) {
	timeout := conversation.TurnUsage{
		TurnID: x38bdTurnID(0), AgentID: "ag_cost", Outcome: x38bdOut(turn.OutcomeTimeout),
		ColdStart: true, CacheRead: x38bdInt(0), CreatedAt: x38bdAt(0),
	}
	if got := x38bdColdStarts(t, []conversation.TurnUsage{timeout}); got != 0 {
		t.Errorf("A3: Timeout/ColdStart true/CacheRead &0，ColdStarts = %d，期望 0", got)
	}
	spoke := timeout
	spoke.Outcome = x38bdOut(turn.OutcomeSpoke)
	if got := x38bdColdStarts(t, []conversation.TurnUsage{spoke}); got != 1 {
		t.Errorf("A3 对照: 同一行 Outcome 换成 Spoke，ColdStarts = %d，期望 1", got)
	}
}

// TestX38bdLegacyFieldsStillCountRefused 是契约 §4 D1（v2.2 升入），依据 §1.2「不改 TokensUnknown 与
// CostUnknown 两个字段的语义与计算」与 §2.5「CostUnknown 字段仍按今天数（含 Refused）」。
// 既有用例钉这两个字段时夹具里没有 Refused 行，把 Refused 从这两个字段里剔掉的实现
// 在既有用例与 §4 全部验收下都活着（屏幕已经不印这两个字段）；这里补上。
// 对照（只差一行）：只有 S1 时两者都是 0，证明多出来的 1 来自 R。
func TestX38bdLegacyFieldsStillCountRefused(t *testing.T) {
	base := summarizeCost(x38bdRows(t, "ag_cost", "S1"))
	if len(base) != 1 || base[0].CostUnknown != 0 || base[0].TokensUnknown != 0 {
		t.Fatalf("对照: 只有 S1，期望 CostUnknown=0、TokensUnknown=0，实得 %+v", base)
	}
	got := summarizeCost(x38bdRows(t, "ag_cost", "S1", "R"))
	if len(got) != 1 {
		t.Fatalf("summarizeCost 返回 %d 组，期望 1 组: %+v", len(got), got)
	}
	if got[0].CostUnknown != 1 {
		t.Errorf("S1 R: CostUnknown = %d，期望 1（字段语义不变，仍数 Refused）", got[0].CostUnknown)
	}
	if got[0].TokensUnknown != 1 {
		t.Errorf("S1 R: TokensUnknown = %d，期望 1（字段语义不变，仍数 Refused）", got[0].TokensUnknown)
	}
}

// ============================================================
// B. 表格：经真实的 /cost 命令
// ============================================================

// x38bdCase 是一条单成员表格用例：形状序列 + 14 格期望（下标 0 是 AGENT）。
type x38bdCase struct {
	name   string
	shapes []string
	want   []string
}

// x38bdCases 逐条照抄契约 §4 B 表；TURNS、SPOKE、LOST、REFUSED 按契约「照行数直接数」手算成字面量。
//
// 只差一处的对照对（「新行为」的每一格都至少出现在一对里）：
//   - B5（R R）对 B6（T T）：只差结局。钉「Refused 不适用、不出 ?、不进未知计数」。
//   - B6（T T）对 B6 对照（T0 T0）：只差 NULL 与指向 0 的指针。钉「?」的门是报了的轮数而不是合计为 0。
//   - B3（S1 S2 T）对 B4（S1 S2 R）：只差第三行的结局。钉 COST_UNKNOWN 不数 Refused。
//   - B3 对 B7（S1 S2 L）、B10（S1 S2 PF）：只差第三行。钉 Lost 是适用行；PF 有用量、冷暖未知。
//   - B9（M）对 B12（Mc）：只差 CostUSD。钉 USAGE_UNKNOWN 不看金额、COST_USD 的门看金额。
//   - B9（M）对 B9 对照（Mcr0，契约外补强）：只差 CacheRead。钉 COLD / COLD_UNKNOWN 的门看 CacheRead。
//   - B6（T T）对 B13（T R）：只差一行结局。钉 ? 的门是「适用行数 > 0」而不是「没有 Refused」。
//   - B5（R R）对 B14（C）、B15（U）：只差结局。钉 Canceled、Unparsable 是适用行。
//   - B8b（PR）对 B8c（T PR）：多一行 T。钉 PostRejected 是适用行（B8c 的 SPOKE 是 0，门只能靠 PR 打开）。
var x38bdCases = []x38bdCase{
	{"B1", []string{"S1", "S2"},
		[]string{"ag_cost", "2", "2", "0", "0", "300", "30", "3", "3000", "0.30", "0", "1", "0", "0"}},
	{"B2", []string{"G1", "G2"},
		[]string{"ag_cost", "2", "2", "0", "0", "300", "30", "?", "3000", "?", "2", "1", "0", "0"}},
	{"B3", []string{"S1", "S2", "T"},
		[]string{"ag_cost", "3", "2", "0", "0", "300", "30", "3", "3000", "0.30", "1", "1", "1", "1"}},
	{"B4", []string{"S1", "S2", "R"},
		[]string{"ag_cost", "3", "2", "0", "1", "300", "30", "3", "3000", "0.30", "0", "1", "0", "0"}},
	{"B5", []string{"R", "R"},
		[]string{"ag_cost", "2", "0", "0", "2", "0", "0", "0", "0", "0.00", "0", "0", "0", "0"}},
	{"B6", []string{"T", "T"},
		[]string{"ag_cost", "2", "0", "0", "0", "?", "?", "?", "?", "?", "2", "?", "2", "2"}},
	{"B6c", []string{"T0", "T0"},
		[]string{"ag_cost", "2", "0", "0", "0", "0", "0", "0", "0", "0.00", "0", "?", "0", "2"}},
	{"B7", []string{"S1", "S2", "L"},
		[]string{"ag_cost", "3", "2", "1", "0", "300", "30", "3", "3000", "0.30", "1", "1", "1", "1"}},
	{"B7b", []string{"L"},
		[]string{"ag_cost", "1", "0", "1", "0", "?", "?", "?", "?", "?", "1", "?", "1", "1"}},
	{"B7c", []string{"SF"},
		[]string{"ag_cost", "1", "0", "0", "0", "?", "?", "?", "?", "?", "1", "?", "1", "1"}},
	{"B8", []string{"M", "S2"},
		[]string{"ag_cost", "2", "2", "0", "0", "200", "20", "2", "2000", "0.20", "1", "0", "1", "1"}},
	{"B8b", []string{"PR"},
		[]string{"ag_cost", "1", "0", "0", "0", "7", "3", "1", "70", "0.05", "0", "1", "0", "0"}},
	{"B9", []string{"M"},
		[]string{"ag_cost", "1", "1", "0", "0", "?", "?", "?", "?", "?", "1", "?", "1", "1"}},
	// 契约外补强：B9 只差 CacheRead（NULL → &0）。CR 那一格变成 0、COLD 变成 1、两个新列都变成 0。
	{"B9c", []string{"Mcr0"},
		[]string{"ag_cost", "1", "1", "0", "0", "?", "0", "?", "?", "?", "1", "1", "0", "0"}},
	{"B10", []string{"S1", "S2", "PF"},
		[]string{"ag_cost", "3", "2", "0", "0", "300", "30", "3", "3000", "0.30", "1", "1", "0", "1"}},
	{"B8c", []string{"T", "PR"},
		[]string{"ag_cost", "2", "0", "0", "0", "7", "3", "1", "70", "0.05", "1", "1", "1", "1"}},
	{"B12", []string{"Mc"},
		[]string{"ag_cost", "1", "1", "0", "0", "?", "?", "?", "?", "0.01", "0", "?", "1", "1"}},
	{"B13", []string{"T", "R"},
		[]string{"ag_cost", "2", "0", "0", "1", "?", "?", "?", "?", "?", "1", "?", "1", "1"}},
	{"B14", []string{"C"},
		[]string{"ag_cost", "1", "0", "0", "0", "?", "?", "?", "?", "?", "1", "?", "1", "1"}},
	{"B15", []string{"U"},
		[]string{"ag_cost", "1", "0", "0", "0", "?", "?", "?", "?", "?", "1", "?", "1", "1"}},
	// B16（v2.2）期望与 B13 逐格相同：R0 的指向 0 的指针不许进 IN / CR / OUT 的「报了的轮数」，否则那三格从 ? 变成 0。
	{"B16", []string{"T", "R0"},
		[]string{"ag_cost", "2", "0", "0", "1", "?", "?", "?", "?", "?", "1", "?", "1", "1"}},
}

// TestX38bdTable 跑 B1-B16（含 B6 对照、B7b、B7c、B8b、B8c 与补强 B9c），每条一个成员、一次 /cost。
// 子测试名就是契约里的编号，-run 'TestX38bdTable/B6c' 可以单跑。
func TestX38bdTable(t *testing.T) {
	for _, c := range x38bdCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			out := x35aRunCost(t, x38bdRows(t, "ag_cost", c.shapes...), "/cost")
			rows := x38bdTable(t, c.name, out, 1)
			x38bdAssertRow(t, c.name, rows, c.want, out)
		})
	}
}

// TestX38bdB6PairDiffers 是契约点名的 B6 与 B6 对照差分：两段 /cost 输出必须不同。
// 两段各自的逐格断言在 TestX38bdTable 的 B6、B6c 里；这里只钉差分本身。
func TestX38bdB6PairDiffers(t *testing.T) {
	nullOut := x35aRunCost(t, x38bdRows(t, "ag_cost", "T", "T"), "/cost")
	zeroOut := x35aRunCost(t, x38bdRows(t, "ag_cost", "T0", "T0"), "/cost")
	if nullOut == zeroOut {
		t.Errorf("两轮 Timeout 用量全 NULL 与两轮用量实测为 0，/cost 输出逐字节相同：\n%s", nullOut)
	}
}

// TestX38bdB1HeaderIsFrozenLiteral 是契约 B1 的表头断言，单独一条让失败信息直指表头。
// 与 x38bdTable 里每条用例都做的表头检查重叠，那是有意的。
func TestX38bdB1HeaderIsFrozenLiteral(t *testing.T) {
	out := x35aRunCost(t, x38bdRows(t, "ag_cost", "S1", "S2"), "/cost")
	lines := x35aLines(t, out)
	want := strings.Split(x38bdHeader, "\t")
	if len(want) != x38bdCols {
		t.Fatalf("冻结表头切出 %d 个名字，期望 %d（工装故障）", len(want), x38bdCols)
	}
	got := strings.Fields(lines[0])
	if len(got) != len(want) {
		t.Fatalf("表头 %d 格，期望 %d 格: %q", len(got), len(want), lines[0])
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("表头第 %d 格 = %q，期望 %q", i, got[i], want[i])
		}
	}
}

// TestX38bdB11TokenColumnsIndependent 是验收 B11：一次 /cost 五个成员，每人一行 Spoke、
// CostUSD &0.01、ColdStart false，只有一个 token 列非 NULL。
// 每个成员只在一列上有数，所以任意两列「报了的轮数」对调，都会让某人的一格在数与 ? 之间翻转；
// ag_th 只有 THINKING 有数，USAGE_UNKNOWN 必须是 0（五列都看）；ag_cr 是唯一冷暖确知的，
// COLD 是 0 不是 ?、COLD_UNKNOWN 是 0，与其余四人的 ? / 1 互为对照。
func TestX38bdB11TokenColumnsIndependent(t *testing.T) {
	type member struct {
		agent string
		set   func(u *conversation.TurnUsage)
		want  []string
	}
	members := []member{
		{"ag_in", func(u *conversation.TurnUsage) { u.InputTokens = x38bdInt(11) },
			[]string{"ag_in", "1", "1", "0", "0", "11", "?", "?", "?", "0.01", "0", "?", "0", "1"}},
		{"ag_cr", func(u *conversation.TurnUsage) { u.CacheRead = x38bdInt(12) },
			[]string{"ag_cr", "1", "1", "0", "0", "?", "12", "?", "?", "0.01", "0", "0", "0", "0"}},
		{"ag_cc", func(u *conversation.TurnUsage) { u.CacheCreate = x38bdInt(13) },
			[]string{"ag_cc", "1", "1", "0", "0", "?", "?", "13", "?", "0.01", "0", "?", "0", "1"}},
		{"ag_out", func(u *conversation.TurnUsage) { u.OutputTokens = x38bdInt(14) },
			[]string{"ag_out", "1", "1", "0", "0", "?", "?", "?", "14", "0.01", "0", "?", "0", "1"}},
		{"ag_th", func(u *conversation.TurnUsage) { u.ThinkingTokens = x38bdInt(15) },
			[]string{"ag_th", "1", "1", "0", "0", "?", "?", "?", "?", "0.01", "0", "?", "0", "1"}},
	}
	var rows []conversation.TurnUsage
	for i, m := range members {
		u := conversation.TurnUsage{
			TurnID: x38bdTurnID(i), AgentID: m.agent, Outcome: x38bdOut(turn.OutcomeSpoke),
			CostUSD: x38bdF64(0.01), ColdStart: false, CreatedAt: x38bdAt(i),
		}
		m.set(&u)
		rows = append(rows, u)
	}
	out := x35aRunCost(t, rows, "/cost")
	table := x38bdTable(t, "B11", out, len(members))
	for _, m := range members {
		x38bdAssertRow(t, "B11", table, m.want, out)
	}
}
