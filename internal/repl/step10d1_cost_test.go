package repl

// 本文件是 Council Step 10 D 片之一（/cost）的测试腿产出，覆盖冻结契约
// （E:\multi_model_cowork\step10d1_contract.md）第 6 节验收清单的第 1-5、
// 10、11 条：
//
//   - 1-5 是纯函数 summarizeCost 的用例，输入直接构造
//     []conversation.TurnUsage，不碰任何库——这是本刀判别力最高、成本
//     最低的一组。
//   - 10、11 是 /cost 命令本身的输出，走真实的 REPL 分派（fakeStore
//     的读写路径见 internal/repl/conversation_test.go 里新加的
//     TurnUsages / seedTurnUsage）。
//
// 第 6-9 条是 Store.TurnUsages 的 SQL 行为，用真 *store.Store，
// 放在 internal/store/step10d1_turnusages_test.go——因为本包 fakeStore
// 的 WriteTx 不做真实回滚，断言"库里真的有/没有某一行"在这里会退化成
// 恒真的空用例（本项目已经为这类教训付过两次账）。
//
// 背对背声明：写这份文件时实现方正在并行写 internal/repl/cost.go、改
// internal/conversation/{store.go,service.go}、internal/store/read.go、
// internal/repl/commands.go。本文件全程没有读那几个文件改动之后的内容——
// 契约第 3、6 节冻结的签名与验收清单是唯一依据。summarizeCost、costRow
// 现在都不存在，本文件现在编译不过是预期的（契约正文原话）。

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/turn"
)

// ============================================================
// 验收 1-5：summarizeCost 纯函数
// ============================================================

// TestSummarizeCostNilCostNeverCountedAsZero 是验收清单第 1 条，契约原话
// "这一条是本刀的要害，要直接钉它"。
//
// 只断言合计恰好等于 0.25 是不够的：一个把三行 nil 悄悄当 0 加进去的坏
// 实现，合计同样会算出 0.25（0.25+0+0+0），骗得过"合计对不对"这一项。
// CostKnown/CostUnknown 两个计数器才是真正拆穿它的地方——把 nil 当 0 加的
// 实现通常也会把这四行全记成"有数"（CostKnown=4, CostUnknown=0），
// 而不是如实分成 1 与 3。三条断言合起来才堵死这条路。
func TestSummarizeCostNilCostNeverCountedAsZero(t *testing.T) {
	at := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	known := 0.25
	rows := []conversation.TurnUsage{
		{TurnID: "cost-known", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CostUSD: &known, CreatedAt: at},
		{TurnID: "cost-nil-1", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at.Add(1 * time.Minute)},
		{TurnID: "cost-nil-2", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at.Add(2 * time.Minute)},
		{TurnID: "cost-nil-3", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at.Add(3 * time.Minute)},
	}

	got := summarizeCost(rows)
	if len(got) != 1 {
		t.Fatalf("summarizeCost 返回 %d 组，期望 1 组（单一 AgentID）: %+v", len(got), got)
	}
	row := got[0]
	if row.CostUSD != 0.25 {
		t.Errorf("CostUSD 合计 = %v，期望恰好 0.25", row.CostUSD)
	}
	if row.CostKnown != 1 {
		t.Errorf("CostKnown = %d，期望 1", row.CostKnown)
	}
	if row.CostUnknown != 3 {
		t.Errorf("CostUnknown = %d，期望 3", row.CostUnknown)
	}
}

// TestSummarizeCostNilTokenNeverCountedAsZero 是验收清单第 2 条：token 版的
// 第 1 条。一行五个 token 全部已知，三行全部是 nil；断言 InputTokens 合计
// 只含那一行已知值，且 TokensUnknown 只数中那三行。
func TestSummarizeCostNilTokenNeverCountedAsZero(t *testing.T) {
	at := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	in, cc, cr, out, think := 100, 10, 5, 50, 20
	rows := []conversation.TurnUsage{
		{
			TurnID: "tok-known", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at,
			InputTokens: &in, CacheCreate: &cc, CacheRead: &cr, OutputTokens: &out, ThinkingTokens: &think,
		},
		{TurnID: "tok-nil-1", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at.Add(1 * time.Minute)},
		{TurnID: "tok-nil-2", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at.Add(2 * time.Minute)},
		{TurnID: "tok-nil-3", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at.Add(3 * time.Minute)},
	}

	got := summarizeCost(rows)
	if len(got) != 1 {
		t.Fatalf("summarizeCost 返回 %d 组，期望 1 组: %+v", len(got), got)
	}
	row := got[0]
	if row.InputTokens != 100 {
		t.Errorf("InputTokens 合计 = %d，期望 100（只含那一行非 nil 的）", row.InputTokens)
	}
	if row.TokensUnknown != 3 {
		t.Errorf("TokensUnknown = %d，期望 3", row.TokensUnknown)
	}
}

// TestSummarizeCostPartiallyKnownTokenRowStillSumsKnownParts 补第 2 条没有
// 单独点名、但契约 4.2 明写的一个分支："部分已知的那几个数仍然照常进和
// （它们是真实测量值），只是这一行同时被标记为有缺口。"
//
// 这条要挡的坏实现是"只要五个指针里有一个 nil，就把整行弃掉不加"——那样
// InputTokens 合计会变成 0，而不是这一行真实测得的 50。
func TestSummarizeCostPartiallyKnownTokenRowStillSumsKnownParts(t *testing.T) {
	at := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	in := 50
	rows := []conversation.TurnUsage{
		{
			TurnID: "tok-partial", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at,
			InputTokens: &in, // 其余四个 token 指针留 nil。
		},
	}

	got := summarizeCost(rows)
	if len(got) != 1 {
		t.Fatalf("summarizeCost 返回 %d 组，期望 1 组: %+v", len(got), got)
	}
	row := got[0]
	if row.InputTokens != 50 {
		t.Errorf("InputTokens 合计 = %d，期望 50（部分已知的列仍要进和，不因同一行里有别的 nil 就整行弃掉）", row.InputTokens)
	}
	if row.TokensUnknown != 1 {
		t.Errorf("TokensUnknown = %d，期望 1（五个指针里只要有一个 nil 就算一次，不多算）", row.TokensUnknown)
	}
}

// TestSummarizeCostOutcomeLostCountsSeparatelyAndNeverSums 是验收清单第 3
// 条。只放一行 Lost 起不到判别作用（Turns==1 对任何"数不数 Lost"的实现都
// 可能凑巧成立），所以配一行正常的 Spoke 行：Turns 必须是 2，Lost 与
// Spoke 必须分开计数，且 Lost 行本来就全是 nil 的用量/金额绝不能进和。
func TestSummarizeCostOutcomeLostCountsSeparatelyAndNeverSums(t *testing.T) {
	at := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	in, cost := 100, 0.5
	rows := []conversation.TurnUsage{
		{
			TurnID: "lost-spoke", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), CreatedAt: at,
			InputTokens: &in, CostUSD: &cost,
		},
		{
			// Lost 行的真实形状：Seq 与全部用量/金额列都是 nil（冻结契约 0 节
			// 原话："OutcomeLost……全部 token 与金额列都是 NULL"）。
			TurnID: "lost-1", AgentID: "ag1", Outcome: int(turn.OutcomeLost), CreatedAt: at.Add(1 * time.Minute),
		},
	}

	got := summarizeCost(rows)
	if len(got) != 1 {
		t.Fatalf("summarizeCost 返回 %d 组，期望 1 组: %+v", len(got), got)
	}
	row := got[0]
	if row.Turns != 2 {
		t.Errorf("Turns = %d，期望 2（丢失轮也要计进总轮数，不然用户会以为这条会话只跑了 1 轮）", row.Turns)
	}
	if row.Lost != 1 {
		t.Errorf("Lost = %d，期望 1", row.Lost)
	}
	if row.Spoke != 1 {
		t.Errorf("Spoke = %d，期望 1", row.Spoke)
	}
	if row.InputTokens != 100 {
		t.Errorf("InputTokens 合计 = %d，期望恰好 100（不含 lost 那行）", row.InputTokens)
	}
	if row.CostUSD != 0.5 {
		t.Errorf("CostUSD 合计 = %v，期望恰好 0.5（不含 lost 那行）", row.CostUSD)
	}
}

// TestSummarizeCostOutcomeRefusedCountsSeparatelyFromLost 是验收清单第 4
// 条：Refused 与 Lost 混在一起造，断言两个计数器互不覆盖、互不合并。
func TestSummarizeCostOutcomeRefusedCountsSeparatelyFromLost(t *testing.T) {
	at := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	rows := []conversation.TurnUsage{
		{TurnID: "refused-1", AgentID: "ag1", Outcome: int(turn.OutcomeRefused), CreatedAt: at},
		{TurnID: "lost-1", AgentID: "ag1", Outcome: int(turn.OutcomeLost), CreatedAt: at.Add(1 * time.Minute)},
	}

	got := summarizeCost(rows)
	if len(got) != 1 {
		t.Fatalf("summarizeCost 返回 %d 组，期望 1 组: %+v", len(got), got)
	}
	row := got[0]
	if row.Refused != 1 {
		t.Errorf("Refused = %d，期望 1", row.Refused)
	}
	if row.Lost != 1 {
		t.Errorf("Lost = %d，期望 1", row.Lost)
	}
	if row.Turns != 2 {
		t.Errorf("Turns = %d，期望 2", row.Turns)
	}
}

// TestSummarizeCostColdStartsOnlyCountsIngestedRows 是验收清单第 5 条，
// 契约点名的分水岭用例。前三行照契约原话构造（Timeout/cold=false、
// Lost/cold=false、Spoke/cold=true，此时 ColdStarts 应为 1），第四行是
// 分水岭："Outcome 不是 Ingested，但 ColdStart 是 true"——一个只看
// ColdStart 不看 Outcome 的坏实现会把它也数进去。
//
// 本用例额外加了第五行 PostRejected/cold=true，把期望值从契约原话的 1
// 提到 2：Ingested() 是 "Spoke || PostRejected" 两支，只验 Spoke 那一支
// 会放过"漏掉 PostRejected"这一种同样错误的实现。
func TestSummarizeCostColdStartsOnlyCountsIngestedRows(t *testing.T) {
	at := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	// x38bd：冷启动只数 cache_read 非 NULL 的 Ingested 行；三行冷启动行都补上 cache_read，
	// 分水岭行也补，否则它被 CacheRead 门挡掉，不再能测 Ingested 门。
	csCacheRead := 0
	rows := []conversation.TurnUsage{
		{TurnID: "cs-timeout-cold-false", AgentID: "ag1", Outcome: int(turn.OutcomeTimeout), ColdStart: false, CreatedAt: at},
		{TurnID: "cs-lost-cold-false", AgentID: "ag1", Outcome: int(turn.OutcomeLost), ColdStart: false, CreatedAt: at.Add(1 * time.Minute)},
		{TurnID: "cs-spoke-cold-true", AgentID: "ag1", Outcome: int(turn.OutcomeSpoke), ColdStart: true, CacheRead: &csCacheRead, CreatedAt: at.Add(2 * time.Minute)},
		// 分水岭：Outcome 不是 Ingested，ColdStart 却是 true。这一行绝不能被数。
		{TurnID: "cs-timeout-cold-true", AgentID: "ag1", Outcome: int(turn.OutcomeTimeout), ColdStart: true, CacheRead: &csCacheRead, CreatedAt: at.Add(3 * time.Minute)},
		{TurnID: "cs-postrejected-cold-true", AgentID: "ag1", Outcome: int(turn.OutcomePostRejected), ColdStart: true, CacheRead: &csCacheRead, CreatedAt: at.Add(4 * time.Minute)},
	}

	got := summarizeCost(rows)
	if len(got) != 1 {
		t.Fatalf("summarizeCost 返回 %d 组，期望 1 组: %+v", len(got), got)
	}
	row := got[0]
	if row.ColdStarts != 2 {
		t.Errorf("ColdStarts = %d，期望 2（cs-spoke-cold-true 与 cs-postrejected-cold-true；"+
			"cs-timeout-cold-true 的 cold_start=1 在 Outcome 不是 Ingested 时读作"+
			"「不知道冷暖」，不能被数）", row.ColdStarts)
	}
}

// ============================================================
// 验收 10-11：/cost 命令本身
// ============================================================

// replTurnOutputs 跑一段多行输入，按提示符 prompt 把整段输出切成"每一行
// 各自产生的那一段"，第 i 个元素对应 lines[i] 的输出。
//
// Run 的循环形状是"先打提示符、再读一行、再执行"，所以对 N 行输入，完整
// 输出形如 prompt + out(line1) + prompt + out(line2) + ... + prompt +
// out(lineN) + prompt + "\n" + 告别语，一共 N+1 个 prompt。用 strings.Split
// 直接切，不必知道 prompt/告别语各自的字节数，也不用手动去拼"提示符 +
// 告别语"这个后缀——那种写法在这里要跑两条命令时会漏算中间那个 prompt。
//
// 前提：命令输出本身不含字面的 "council> "。这是控制台读循环自己的提示符
// 文本，本包目前没有任何命令会把它打印成会话内容的一部分。
func replTurnOutputs(t *testing.T, svc *conversation.Service, lines ...string) []string {
	t.Helper()
	input := strings.Join(lines, "\n") + "\n"
	out, err := runREPLSvc(t, svc, input)
	if err != nil {
		t.Fatalf("Run 返回错误: %v；原始输出:\n%s", err, out)
	}
	parts := strings.Split(out, prompt)
	if len(parts) != len(lines)+2 {
		t.Fatalf("输出被提示符切成 %d 段，期望 %d 段（%d 条命令 + 结尾）；原始输出:\n%s",
			len(parts), len(lines)+2, len(lines), out)
	}
	return parts[1 : len(parts)-1]
}

// TestCmdCostEmptyStoreShowsMessageNotHeader 是验收清单第 10 条的前半句：
// "库干净时打「本会话还没有记过账」且不出现表头"。
//
// 用 replTurnOutputs 拿到 /cost 单独产生的那一段文本，与契约 4.3 原句逐字
// 相等——这一句断言同时钉死了两件事：消息文本对，以及除了这句话之外什么
// 都没多打（尤其是没有表头行）。
func TestCmdCostEmptyStoreShowsMessageNotHeader(t *testing.T) {
	svc, f := newTestService(t)
	const convID = "conv_cost_empty_0000000000000000000"
	f.seedConversation(convID, "cost 空会话")

	outs := replTurnOutputs(t, svc, "/use "+convID, "/cost")
	got := outs[1]
	want := "本会话还没有记过账\n"
	if got != want {
		t.Errorf("/cost 空库输出 = %q，期望恰好 %q（不打空表头）", got, want)
	}
}

// TestCmdCostAmountUnknownIsSeparateColumnFromMeasuredZero 是验收清单第
// 10 条的后半句："「金额不知道」必须是独立的一栏，不许折进金额那一栏"。
//
// 列的措辞与宽度不进契约，没法靠猜字符串断言"有没有一栏叫这个名字"，所以
// 用差分法：造两条会话，形状完全一样（相同 AgentID、相同 Outcome、相同
// CreatedAt、相同轮数），唯一的区别是 CostUSD 是"三轮都精确测得 0"还是
// "三轮都没报金额（nil）"。如果 /cost 真的把「不知道」当成独立一栏，这两
// 种情况的输出必然不同；如果实现只打印 CostUSD 的数值（两边合计都是
// 0.0）而不看 CostKnown/CostUnknown，两段输出会变得逐字节相同——这正是
// 契约那句"把 3 轮未知金额显示成 $0.00 的表，比没有这张表更糟"要挡的
// 坏实现，而这条差分断言能直接抓到它。
func TestCmdCostAmountUnknownIsSeparateColumnFromMeasuredZero(t *testing.T) {
	svc, f := newTestService(t)
	const convKnownZero = "conv_cost_known_zero_00000000000000"
	const convUnknown = "conv_cost_unknown_000000000000000000"
	f.seedConversation(convKnownZero, "三轮金额精确为零")
	f.seedConversation(convUnknown, "三轮金额完全未知")

	at := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		knownZero := 0.0
		f.seedTurnUsage(conversation.TurnUsage{
			TurnID: fmt.Sprintf("turn-known-zero-%d", i), ConvID: convKnownZero, AgentID: "ag_cost",
			Outcome: int(turn.OutcomeSpoke), CostUSD: &knownZero,
			CreatedAt: at.Add(time.Duration(i) * time.Minute),
		})
		f.seedTurnUsage(conversation.TurnUsage{
			TurnID: fmt.Sprintf("turn-unknown-%d", i), ConvID: convUnknown, AgentID: "ag_cost",
			Outcome: int(turn.OutcomeSpoke), CostUSD: nil,
			CreatedAt: at.Add(time.Duration(i) * time.Minute),
		})
	}

	zeroOuts := replTurnOutputs(t, svc, "/use "+convKnownZero, "/cost")
	unknownOuts := replTurnOutputs(t, svc, "/use "+convUnknown, "/cost")

	if zeroOuts[1] == unknownOuts[1] {
		t.Errorf("三轮金额精确为零与三轮金额完全未知，/cost 输出竟然逐字节相同：\n%s\n"+
			"「金额不知道」必须是独立一栏，不许折进金额那一栏——两种情况渲染成同一张表"+
			"说明实现没有区分 CostKnown/CostUnknown。", zeroOuts[1])
	}
}

// TestCmdCostRoundFilterOnlyCountsThatRound 是验收清单第 11 条：
// "--round 过滤：两个 round 各若干行，断言只统计指定的那个。"
//
// 同样用差分法，而且带一道反向哨兵：
//
//  1. 对照会话 convGround 只装 round-keep 的两行；测试会话 convMixed 额外
//     混进 round-drop 的三行（不同 AgentID，一旦漏过滤，行数、AgentID 都
//     会变）。/cost --round round-keep 在 convMixed 上的输出，理应与
//     convGround 上不带过滤的 /cost 输出逐字节相同——这是主断言。
//  2. 反向哨兵：convMixed 上不带 --round 的 /cost 输出，必须与带
//     --round round-keep 的输出不同。没有这一条，若 --round 解析整个失败
//     导致命令拒绝或库读空（两边都印出「本会话还没有记过账」），第 1 条的
//     相等断言会被这种"两边都错但恰好相等"的情况蒙混过去，测不出
//     --round 到底有没有真的在过滤。
func TestCmdCostRoundFilterOnlyCountsThatRound(t *testing.T) {
	svc, f := newTestService(t)
	const convGround = "conv_cost_ground_000000000000000000"
	const convMixed = "conv_cost_mixed_0000000000000000000"
	f.seedConversation(convGround, "只含目标轮的对照组")
	f.seedConversation(convMixed, "混了另一轮的测试组")

	at := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	in1, in2 := 111, 222
	keepRows := []conversation.TurnUsage{
		{
			TurnID: "turn-keep-1", AgentID: "ag_solo", Outcome: int(turn.OutcomeSpoke),
			RoundID: "round-keep", InputTokens: &in1, CreatedAt: at,
		},
		{
			TurnID: "turn-keep-2", AgentID: "ag_solo", Outcome: int(turn.OutcomeTimeout),
			RoundID: "round-keep", InputTokens: &in2, CreatedAt: at.Add(1 * time.Minute),
		},
	}
	dropRows := []conversation.TurnUsage{
		{
			TurnID: "turn-drop-1", AgentID: "ag_other", Outcome: int(turn.OutcomeSpoke),
			RoundID: "round-drop", CreatedAt: at.Add(2 * time.Minute),
		},
		{
			TurnID: "turn-drop-2", AgentID: "ag_other", Outcome: int(turn.OutcomeLost),
			RoundID: "round-drop", CreatedAt: at.Add(3 * time.Minute),
		},
		{
			TurnID: "turn-drop-3", AgentID: "ag_other", Outcome: int(turn.OutcomeRefused),
			RoundID: "round-drop", CreatedAt: at.Add(4 * time.Minute),
		},
	}
	for _, u := range keepRows {
		ground := u
		ground.ConvID = convGround
		f.seedTurnUsage(ground)
		mixed := u
		mixed.ConvID = convMixed
		f.seedTurnUsage(mixed)
	}
	for _, u := range dropRows {
		mixed := u
		mixed.ConvID = convMixed
		f.seedTurnUsage(mixed)
	}

	groundOuts := replTurnOutputs(t, svc, "/use "+convGround, "/cost")
	mixedFilteredOuts := replTurnOutputs(t, svc, "/use "+convMixed, "/cost --round round-keep")
	if groundOuts[1] != mixedFilteredOuts[1] {
		t.Errorf("--round round-keep 在混合会话上的输出与只含该轮的对照会话不一致，"+
			"说明 round-drop 的三行没有被正确排除在外。\n对照（只有 round-keep）:\n%s\n"+
			"过滤后（混合会话 --round round-keep）:\n%s", groundOuts[1], mixedFilteredOuts[1])
	}

	mixedUnfilteredOuts := replTurnOutputs(t, svc, "/use "+convMixed, "/cost")
	if mixedUnfilteredOuts[1] == mixedFilteredOuts[1] {
		t.Errorf("混合会话上带 --round round-keep 与不带过滤的输出一样：\n%s\n"+
			"说明 --round 没有真的在过滤（可能是空操作或解析失败后退化成全量）。", mixedFilteredOuts[1])
	}
}
