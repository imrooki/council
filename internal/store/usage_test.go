// usage_test.go 覆盖 Step 7 契约 10a §2.2 的 Tx.RecordUsage：正常插入、turn_id 冲突
// 必须报错（不是 upsert）、七个用量列的可空三态、Seq 的可空、以及外键。
//
// Store 接口只给 turn_usage 加了写方法，没有配 Go 层的读方法（10a 只在 §2.3 给
// provider_sessions 加了 Store.ProviderSession 一个读口）。所以这里全部走测试自己的
// 底层连接，用 sql.NullInt64 / sql.NullFloat64 直接 Scan——可空与非空在 Go 里天然是
// 两种取值（Valid 字段），不会像整数 0 那样被混成一回事，这正是本组用例要验的核心。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"council/internal/conversation"
)

func tstIntPtr(v int) *int             { return &v }
func tstInt64Ptr(v int64) *int64       { return &v }
func tstFloat64Ptr(v float64) *float64 { return &v }

// tstFixedUsageTime 是本文件时间往返用的固定时刻，带真实的纳秒精度，
// 这样 RFC3339Nano 的精度才算真被走到，不是凑巧对齐到秒。
var tstFixedUsageTime = time.Date(2026, 9, 20, 12, 34, 56, 789012345, time.UTC)

// tstBaseUsage 造一条字段齐全、合法的 turn_usage，调用方按需覆盖要测的那个字段。
// ProviderSessionID 按 turnID 派生，避免多个用例之间互相污染。
func tstBaseUsage(convID, agentID, turnID string) conversation.TurnUsage {
	return conversation.TurnUsage{
		TurnID:            turnID,
		ConvID:            convID,
		AgentID:           agentID,
		ProviderSessionID: "sess-" + turnID,
		Seq:               tstInt64Ptr(1),
		Outcome:           1,
		InputTokens:       tstIntPtr(100),
		CacheCreate:       tstIntPtr(0),
		CacheRead:         tstIntPtr(0),
		OutputTokens:      tstIntPtr(50),
		ThinkingTokens:    tstIntPtr(10),
		CostUSD:           tstFloat64Ptr(0.01),
		CostCumulativeUSD: tstFloat64Ptr(0.02),
		ColdStart:         false,
		WallMS:            1200,
		PromptBytes:       4096,
		DeliveredLo:       tstInt64Ptr(1),
		DeliveredHi:       tstInt64Ptr(1),
		CreatedAt:         tstFixedUsageTime,
	}
}

// tstRecordUsage 在一个写事务里调 Tx.RecordUsage。
func tstRecordUsage(t *testing.T, ctx context.Context, s *Store, u conversation.TurnUsage) error {
	t.Helper()
	return s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.RecordUsage(ctx, u)
	})
}

// tstUsageRow 是从 turn_usage 直接 Scan 出来的一行。七个可空列与 seq 用 sql.Null*
// 保留三态：绝不能把它们收成裸 int64/float64，那样 NULL 与 0 在测试里也会长得一样，
// 而这正是本组用例要拆穿的事。
type tstUsageRow struct {
	convID            string
	agentID           string
	providerSessionID string
	seq               sql.NullInt64
	outcome           int64
	inputTokens       sql.NullInt64
	cacheCreate       sql.NullInt64
	cacheRead         sql.NullInt64
	outputTokens      sql.NullInt64
	thinkingTokens    sql.NullInt64
	costUSD           sql.NullFloat64
	costCumulativeUSD sql.NullFloat64
	coldStart         int64
	wallMS            int64
	promptBytes       int64
	deliveredLo       sql.NullInt64
	deliveredHi       sql.NullInt64
	createdAt         string
}

func tstScanUsageRow(t *testing.T, db *sql.DB, turnID string) tstUsageRow {
	t.Helper()
	var r tstUsageRow
	err := db.QueryRow(`
SELECT conv_id, agent_id, provider_session_id, seq, outcome,
       input_tokens, cache_create, cache_read, output_tokens, thinking_tokens,
       cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes,
       delivered_lo, delivered_hi, created_at
FROM turn_usage WHERE turn_id = ?`, turnID).Scan(
		&r.convID, &r.agentID, &r.providerSessionID, &r.seq, &r.outcome,
		&r.inputTokens, &r.cacheCreate, &r.cacheRead, &r.outputTokens, &r.thinkingTokens,
		&r.costUSD, &r.costCumulativeUSD, &r.coldStart, &r.wallMS, &r.promptBytes,
		&r.deliveredLo, &r.deliveredHi, &r.createdAt,
	)
	if err != nil {
		t.Fatalf("读 turn_usage(turn_id=%q) 失败: %v", turnID, err)
	}
	return r
}

// tstUsageRowCount 数 turn_usage 里某个 turn_id 的行数，用来断言「冲突时没有插成第二行」。
func tstUsageRowCount(t *testing.T, db *sql.DB, turnID string) int64 {
	t.Helper()
	return tstScanInt(t, db, `SELECT COUNT(*) FROM turn_usage WHERE turn_id = ?`, turnID)
}

// TestRecordUsageInsertAndReadBack 覆盖最基本的一条：正常插入之后读得回来，
// 身份三列、Outcome、时间、以及一个可空数值列都原样往返。
func TestRecordUsageInsertAndReadBack(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "记账往返")
	u := tstBaseUsage(conv.ID, "alpha", "turn-roundtrip-1")

	if err := tstRecordUsage(t, ctx, s, u); err != nil {
		t.Fatalf("RecordUsage 失败: %v", err)
	}

	raw := tstRawDB(t, s.Path())
	got := tstScanUsageRow(t, raw, u.TurnID)

	if got.convID != u.ConvID || got.agentID != u.AgentID || got.providerSessionID != u.ProviderSessionID {
		t.Errorf("身份三列读回 (%q,%q,%q)，期望 (%q,%q,%q)",
			got.convID, got.agentID, got.providerSessionID, u.ConvID, u.AgentID, u.ProviderSessionID)
	}
	if !got.seq.Valid || got.seq.Int64 != *u.Seq {
		t.Errorf("Seq 读回 %+v，期望有效值 %d", got.seq, *u.Seq)
	}
	if got.outcome != int64(u.Outcome) {
		t.Errorf("Outcome = %d，期望 %d", got.outcome, u.Outcome)
	}
	if !got.inputTokens.Valid || got.inputTokens.Int64 != int64(*u.InputTokens) {
		t.Errorf("InputTokens 读回 %+v，期望 %d", got.inputTokens, *u.InputTokens)
	}
	if !got.costUSD.Valid || got.costUSD.Float64 != *u.CostUSD {
		t.Errorf("CostUSD 读回 %+v，期望 %v", got.costUSD, *u.CostUSD)
	}
	if !got.deliveredLo.Valid || got.deliveredLo.Int64 != *u.DeliveredLo {
		t.Errorf("DeliveredLo 读回 %+v，期望 %d", got.deliveredLo, *u.DeliveredLo)
	}
	gotCreated, err := parseTime(got.createdAt)
	if err != nil {
		t.Fatalf("created_at 解析失败: %v", err)
	}
	if !gotCreated.Equal(u.CreatedAt) {
		t.Errorf("CreatedAt 往返 = %v，期望 %v", gotCreated, u.CreatedAt)
	}
}

// TestRecordUsageDuplicateTurnIDFails 锁住契约的硬性要求：同一个 turn_id 写两次必须
// 报错，不许 upsert。理由见 10a §2.2：写两次说明调用方把一轮记了两遍，那是 bug 不是重试。
func TestRecordUsageDuplicateTurnIDFails(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "重复 turn_id")
	const turnID = "turn-dup-1"

	first := tstBaseUsage(conv.ID, "alpha", turnID)
	if err := tstRecordUsage(t, ctx, s, first); err != nil {
		t.Fatalf("第一次 RecordUsage 失败: %v", err)
	}

	second := tstBaseUsage(conv.ID, "alpha", turnID)
	second.InputTokens = tstIntPtr(999) // 换一个值：证明是真被拒绝，不是巧合地相等
	if err := tstRecordUsage(t, ctx, s, second); err == nil {
		t.Fatal("同一个 turn_id 写了第二次竟然成功了：契约要求报错，不许 upsert")
	}

	raw := tstRawDB(t, s.Path())
	if n := tstUsageRowCount(t, raw, turnID); n != 1 {
		t.Fatalf("turn_id %q 有 %d 行，期望仍是 1 行（冲突不该插成第二行）", turnID, n)
	}
	got := tstScanUsageRow(t, raw, turnID)
	if !got.inputTokens.Valid || got.inputTokens.Int64 != int64(*first.InputTokens) {
		t.Errorf("冲突之后 InputTokens 读回 %+v，期望仍是第一次写的 %d（不是 upsert）",
			got.inputTokens, *first.InputTokens)
	}
}

// TestRecordUsageNullableTriState 是本步的核心：七个可空用量列必须能表达
// 「没报」（NULL）与「报了、恰好是 0」（0）两件不同的事，绝不能把 NULL 读成 0。
//
// 每个子用例对应 10a §1.1 里点名的一个真实场景，而不是随手编的数字：
// agy 的 usage 里根本没有 cache_creation；grok 的 cache_creation_input_tokens 恒为 0
// 而 cache_read 明明从 0 涨到 20608；三份失败原件里 claude 与 agy 的五项是货真价实的 0。
// 在 v1 的 NOT NULL DEFAULT 0 下这三种情况长得一模一样，v2 必须能区分。
func TestRecordUsageNullableTriState(t *testing.T) {
	nullInts := func(got tstUsageRow) map[string]sql.NullInt64 {
		return map[string]sql.NullInt64{
			"input_tokens":    got.inputTokens,
			"cache_create":    got.cacheCreate,
			"cache_read":      got.cacheRead,
			"output_tokens":   got.outputTokens,
			"thinking_tokens": got.thinkingTokens,
		}
	}

	cases := []struct {
		name   string
		mutate func(u *conversation.TurnUsage)
		check  func(t *testing.T, got tstUsageRow)
	}{
		{
			name: "七个用量列全 nil 时库里全是 NULL",
			mutate: func(u *conversation.TurnUsage) {
				u.InputTokens, u.CacheCreate, u.CacheRead = nil, nil, nil
				u.OutputTokens, u.ThinkingTokens = nil, nil
				u.CostUSD, u.CostCumulativeUSD = nil, nil
			},
			check: func(t *testing.T, got tstUsageRow) {
				for name, col := range nullInts(got) {
					if col.Valid {
						t.Errorf("%s 读回有效值 %d，期望 NULL", name, col.Int64)
					}
				}
				if got.costUSD.Valid {
					t.Errorf("cost_usd 读回 %v，期望 NULL", got.costUSD.Float64)
				}
				if got.costCumulativeUSD.Valid {
					t.Errorf("cost_cumulative_usd 读回 %v，期望 NULL", got.costCumulativeUSD.Float64)
				}
			},
		},
		{
			name: "七个用量列全指向 0 时库里全是 0，不是 NULL",
			mutate: func(u *conversation.TurnUsage) {
				u.InputTokens, u.CacheCreate, u.CacheRead = tstIntPtr(0), tstIntPtr(0), tstIntPtr(0)
				u.OutputTokens, u.ThinkingTokens = tstIntPtr(0), tstIntPtr(0)
				u.CostUSD, u.CostCumulativeUSD = tstFloat64Ptr(0), tstFloat64Ptr(0)
			},
			check: func(t *testing.T, got tstUsageRow) {
				for name, col := range nullInts(got) {
					if !col.Valid || col.Int64 != 0 {
						t.Errorf("%s 读回 %+v，期望有效值 0", name, col)
					}
				}
				if !got.costUSD.Valid || got.costUSD.Float64 != 0 {
					t.Errorf("cost_usd 读回 %+v，期望有效值 0", got.costUSD)
				}
				if !got.costCumulativeUSD.Valid || got.costCumulativeUSD.Float64 != 0 {
					t.Errorf("cost_cumulative_usd 读回 %+v，期望有效值 0", got.costCumulativeUSD)
				}
			},
		},
		{
			// 实测事实（10a §1.1）：agy 的 usage 里根本没有 cache_creation 这一项。
			name: "agy 场景：有 input/output，cache_create 是 NULL 不是 0",
			mutate: func(u *conversation.TurnUsage) {
				u.InputTokens, u.OutputTokens = tstIntPtr(200), tstIntPtr(80)
				u.CacheCreate, u.CacheRead = nil, nil
				u.ThinkingTokens = nil
			},
			check: func(t *testing.T, got tstUsageRow) {
				if got.cacheCreate.Valid {
					t.Errorf("agy 场景下 cache_create 读回有效值 %d，期望 NULL（这家根本不报这一项）", got.cacheCreate.Int64)
				}
				if !got.inputTokens.Valid || got.inputTokens.Int64 != 200 {
					t.Errorf("input_tokens 读回 %+v，期望有效值 200", got.inputTokens)
				}
			},
		},
		{
			// 实测事实：grok 的 cache_creation_input_tokens 恒为 0，而 cache_read
			// 明明从 0 涨到 20608。要拆穿的是「恒为 0」与「NULL」不能长得一样。
			name: "grok 场景：cache_create 是货真价实的 0，cache_read 涨到 20608",
			mutate: func(u *conversation.TurnUsage) {
				u.CacheCreate = tstIntPtr(0)
				u.CacheRead = tstIntPtr(20608)
			},
			check: func(t *testing.T, got tstUsageRow) {
				if !got.cacheCreate.Valid || got.cacheCreate.Int64 != 0 {
					t.Errorf("cache_create 读回 %+v，期望有效值 0（不是 NULL）", got.cacheCreate)
				}
				if !got.cacheRead.Valid || got.cacheRead.Int64 != 20608 {
					t.Errorf("cache_read 读回 %+v，期望有效值 20608", got.cacheRead)
				}
			},
		},
		{
			// 实测事实：三份失败原件里 claude 与 agy 的五项是货真价实的 0，不是没报。
			name: "失败轮场景：五项用量是货真价实的 0，Seq 为 NULL",
			mutate: func(u *conversation.TurnUsage) {
				u.InputTokens, u.CacheCreate, u.CacheRead = tstIntPtr(0), tstIntPtr(0), tstIntPtr(0)
				u.OutputTokens, u.ThinkingTokens = tstIntPtr(0), tstIntPtr(0)
				u.Seq = nil
				u.Outcome = 0
			},
			check: func(t *testing.T, got tstUsageRow) {
				for name, col := range nullInts(got) {
					if !col.Valid || col.Int64 != 0 {
						t.Errorf("失败轮 %s 读回 %+v，期望有效值 0（不是 NULL）", name, col)
					}
				}
				if got.seq.Valid {
					t.Errorf("失败轮 seq 读回有效值 %d，期望 NULL", got.seq.Int64)
				}
			},
		},
		{
			name: "DeliveredLo/Hi 为 nil 表示空批次，两列都是 NULL",
			mutate: func(u *conversation.TurnUsage) {
				u.DeliveredLo, u.DeliveredHi = nil, nil
			},
			check: func(t *testing.T, got tstUsageRow) {
				if got.deliveredLo.Valid || got.deliveredHi.Valid {
					t.Errorf("空批次 delivered_lo/hi 读回 (%+v, %+v)，期望都是 NULL", got.deliveredLo, got.deliveredHi)
				}
			},
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := tstStore(t)
			ctx := tstCtx(t)
			conv := tstNewConv(t, ctx, s, "可空三态")
			turnID := fmt.Sprintf("turn-nullable-%d", i)
			u := tstBaseUsage(conv.ID, "alpha", turnID)
			tc.mutate(&u)

			if err := tstRecordUsage(t, ctx, s, u); err != nil {
				t.Fatalf("RecordUsage 失败: %v", err)
			}
			got := tstScanUsageRow(t, tstRawDB(t, s.Path()), turnID)
			tc.check(t, got)
		})
	}
}

// TestRecordUsageForeignKeyRejectsMissingConversation 照 schema_test.go 里
// 「SetCursor 指向不存在的会话」那条的写法：conv_id 指向不存在的会话必须失败，
// 而且失败之后没有留下任何行。
func TestRecordUsageForeignKeyRejectsMissingConversation(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	u := tstBaseUsage("conv-does-not-exist", "alpha", "turn-orphan-1")

	if err := tstRecordUsage(t, ctx, s, u); err == nil {
		t.Fatal("指向不存在会话的 turn_usage 竟然插进去了：外键没生效")
	}

	raw := tstRawDB(t, s.Path())
	if n := tstUsageRowCount(t, raw, u.TurnID); n != 0 {
		t.Errorf("失败之后 turn_usage 里仍有 %d 行，期望 0 行", n)
	}
}
