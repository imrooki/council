// 本文件是 Council Step 10 D 片之一（/cost）的测试腿产出，覆盖冻结契约
// （E:\multi_model_cowork\step10d1_contract.md）第 6 节验收清单的第 6-9
// 条：Store.TurnUsages 的 SQL 行为。
//
// 放在 internal/store、用真 *store.Store，不用 internal/repl 或
// internal/conversation 的假 Store：那两份的 fakeStore.WriteTx 不做真实
// 回滚，凡是要断言"库里真的有/没有某一行、某列是什么值"的用例放在那里会
// 退化成恒真的空用例（冻结契约给测试腿的提醒原话，本项目已经栽过两次）。
// 第 6、7、8、9 条恰好都属于这一类。
//
// 复用 usage_test.go 已有的 tstBaseUsage / tstRecordUsage 以及 open_test.go
// 的 tstStore / tstCtx / tstNewConv，不另起一套构造函数。
//
// 背对背声明：写这份文件时实现方正在并行写 internal/store/read.go 的
// Store.TurnUsages。本文件没有读那份进行中的实现——断言全部来自冻结契约
// 4.1 节的文字要求，以及本包已有的 UsageSamples 实现／测试作为"体例"参照
// （不是猜实现细节）。Store.TurnUsages 现在不存在，本文件现在编译不过是
// 预期的。
package store

import (
	"fmt"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/turn"
)

// TestTurnUsagesDoesNotFilterEmptyUsageRow 是验收清单第 6 条：
// "Store.TurnUsages 不过滤空用量行"。造一行 Lost 行的真实形状——Seq 与
// 五个可空用量列、两个可空金额列全部是 nil——断言它读得回来。
//
// 这条专门挡"照抄 UsageSamples 把 input_tokens IS NOT NULL 一起抄过来"：
// 那样的实现会把这一行悄悄滤掉，TurnUsages 读回 0 行而不是 1 行；
// /cost 存在的意义就是这一类行（Step 10 A 片新写的 OutcomeLost）要能被
// 正确读到，读的时候多一道 UsageSamples 专用的过滤条件正是要挡的坏实现。
func TestTurnUsagesDoesNotFilterEmptyUsageRow(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "空用量行读回")

	u := tstBaseUsage(conv.ID, "alpha", "turn-empty-usage-1")
	u.Seq = nil
	u.InputTokens, u.CacheCreate, u.CacheRead = nil, nil, nil
	u.OutputTokens, u.ThinkingTokens = nil, nil
	u.CostUSD, u.CostCumulativeUSD = nil, nil
	u.Outcome = int(turn.OutcomeLost)

	if err := tstRecordUsage(t, ctx, s, u); err != nil {
		t.Fatalf("RecordUsage 失败: %v", err)
	}

	got, err := s.TurnUsages(ctx, conv.ID, "", -1)
	if err != nil {
		t.Fatalf("TurnUsages 失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("TurnUsages 读回 %d 行，期望 1 行（空用量行不该被 UsageSamples 那条 "+
			"input_tokens IS NOT NULL 过滤误伤）", len(got))
	}
	if got[0].TurnID != u.TurnID {
		t.Errorf("TurnID = %q，期望 %q", got[0].TurnID, u.TurnID)
	}
	if got[0].InputTokens != nil {
		t.Errorf("InputTokens = %d，期望 nil（不该被收成 0）", *got[0].InputTokens)
	}
	if got[0].Seq != nil {
		t.Errorf("Seq = %d，期望 nil", *got[0].Seq)
	}
}

// TestTurnUsagesAgentIDEmptyMeansWholeConversation 是验收清单第 7 条：
// 两个成员各写一行，agentID 传空串读回两行，指定其中一个读回恰好一行。
//
// 两条断言缺一不可：一个「agentID 参数完全不生效、恒答全部」的坏实现，
// 空串那一半会侥幸通过（本来就该是全部）；一个「agentID 空串被当成字面量
// 去匹配 agent_id 等于空字符串」的坏实现，指定 alpha 那一半会侥幸通过、
// 空串那一半会读回 0 行。两条断言合起来才能把这两类坏实现都挡住。
func TestTurnUsagesAgentIDEmptyMeansWholeConversation(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "空 agentID 取整条会话")

	a := tstBaseUsage(conv.ID, "alpha", "turn-scope-a")
	a.CreatedAt = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	b := tstBaseUsage(conv.ID, "beta", "turn-scope-b")
	b.CreatedAt = time.Date(2026, 9, 21, 10, 5, 0, 0, time.UTC)

	if err := tstRecordUsage(t, ctx, s, a); err != nil {
		t.Fatalf("RecordUsage(alpha) 失败: %v", err)
	}
	if err := tstRecordUsage(t, ctx, s, b); err != nil {
		t.Fatalf("RecordUsage(beta) 失败: %v", err)
	}

	all, err := s.TurnUsages(ctx, conv.ID, "", -1)
	if err != nil {
		t.Fatalf("TurnUsages(agentID=\"\") 失败: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("空 agentID 读回 %d 行，期望 2 行（整条会话）: %+v", len(all), all)
	}

	only, err := s.TurnUsages(ctx, conv.ID, "alpha", -1)
	if err != nil {
		t.Fatalf("TurnUsages(agentID=alpha) 失败: %v", err)
	}
	if len(only) != 1 {
		t.Fatalf("指定 agentID=alpha 读回 %d 行，期望恰好 1 行: %+v", len(only), only)
	}
	if only[0].AgentID != "alpha" {
		t.Errorf("读回的一行 AgentID = %q，期望 alpha", only[0].AgentID)
	}
}

// TestTurnUsagesOrderedAscendingByCreatedAt 是验收清单第 8 条：造三行
// created_at 递增但插入顺序打乱（C、A、B），断言读回来是 created_at 升序
// （A、B、C），不是插入顺序、也不是降序。
//
// 插入顺序特意不是 A、B、C：如果实现偷懒直接按 rowid/插入顺序返回，
// 读回来的会是 C、A、B，与期望的 A、B、C 在第一个元素就对不上。
func TestTurnUsagesOrderedAscendingByCreatedAt(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "升序读回")

	base := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	rowA := tstBaseUsage(conv.ID, "alpha", "turn-order-a")
	rowA.CreatedAt = base
	rowB := tstBaseUsage(conv.ID, "alpha", "turn-order-b")
	rowB.CreatedAt = base.Add(1 * time.Minute)
	rowC := tstBaseUsage(conv.ID, "alpha", "turn-order-c")
	rowC.CreatedAt = base.Add(2 * time.Minute)

	for _, u := range []conversation.TurnUsage{rowC, rowA, rowB} {
		if err := tstRecordUsage(t, ctx, s, u); err != nil {
			t.Fatalf("RecordUsage(%s) 失败: %v", u.TurnID, err)
		}
	}

	got, err := s.TurnUsages(ctx, conv.ID, "", -1)
	if err != nil {
		t.Fatalf("TurnUsages 失败: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("读回 %d 行，期望 3 行: %+v", len(got), got)
	}
	wantOrder := []string{rowA.TurnID, rowB.TurnID, rowC.TurnID}
	for i, want := range wantOrder {
		if got[i].TurnID != want {
			t.Errorf("第 %d 行 TurnID = %q，期望 %q（读回顺序应当是 created_at 升序，"+
				"不是插入顺序、也不是降序）", i, got[i].TurnID, want)
		}
	}
}

// TestTurnUsagesLimitTakesNewestTailStillAscending 是验收清单第 9 条：
// 造五行，limit=2，断言拿到的是 created_at 最大的那两条（不是最早的两
// 条），且它们之间仍然是升序。
//
// 这条挡的是"ORDER BY created_at ASC LIMIT 2"这种直觉写法——那样取到的是
// 开头两条（最早的），不是契约要求的"最近 N 条"。正确实现通常要先按
// created_at DESC 取 N 条再倒回升序，这条断言同时验证了"取对了哪两条"
// 与"取到之后顺序仍然是升序"两件事。
func TestTurnUsagesLimitTakesNewestTailStillAscending(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "limit 取末尾 N 条")

	base := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	rows := make([]conversation.TurnUsage, 0, 5)
	for i := 0; i < 5; i++ {
		u := tstBaseUsage(conv.ID, "alpha", fmt.Sprintf("turn-limit-%d", i))
		u.CreatedAt = base.Add(time.Duration(i) * time.Minute)
		rows = append(rows, u)
	}
	for _, u := range rows {
		if err := tstRecordUsage(t, ctx, s, u); err != nil {
			t.Fatalf("RecordUsage(%s) 失败: %v", u.TurnID, err)
		}
	}

	got, err := s.TurnUsages(ctx, conv.ID, "", 2)
	if err != nil {
		t.Fatalf("TurnUsages(limit=2) 失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit=2 读回 %d 行，期望 2 行: %+v", len(got), got)
	}
	if got[0].TurnID != rows[3].TurnID || got[1].TurnID != rows[4].TurnID {
		t.Fatalf("limit=2 读回 [%s, %s]，期望 [%s, %s]（created_at 最大的两条，"+
			"且彼此间仍是升序）", got[0].TurnID, got[1].TurnID, rows[3].TurnID, rows[4].TurnID)
	}
}
