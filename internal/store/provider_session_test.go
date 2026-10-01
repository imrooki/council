// provider_session_test.go 覆盖 Step 7 契约 10a §2.2/§2.3 的
// Tx.SetProviderSession / Tx.DropProviderSession / Store.ProviderSession：
// upsert 语义、五样东西整体读写、CostTotalUSD 的可空三态、作废与幂等。
package store

import (
	"context"
	"testing"
	"time"

	"council/internal/conversation"
)

// tstFixedSessionTime 是本文件时间往返用的固定时刻，带真实纳秒精度，
// 与 usage_test.go 的 tstFixedUsageTime 分开取值，避免两组用例互相掩盖对方的 bug。
var tstFixedSessionTime = time.Date(2026, 9, 20, 8, 15, 30, 123456789, time.UTC)

// tstBaseSession 造一条字段齐全的 provider session，调用方按需覆盖要测的那个字段。
func tstBaseSession(convID, agentID string) conversation.ProviderSession {
	return conversation.ProviderSession{
		ConvID:            convID,
		AgentID:           agentID,
		ProviderSessionID: "sess-" + agentID,
		CursorSeq:         3,
		CostTotalUSD:      0.42,
		HaveCostTotal:     true,
		LastTurnAt:        tstFixedSessionTime,
		IdentityFP:        "fp-" + agentID,
		Spoken:            true,
	}
}

func tstSetProviderSession(t *testing.T, ctx context.Context, s *Store, ps conversation.ProviderSession) {
	t.Helper()
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, ps)
	})
	if err != nil {
		t.Fatalf("SetProviderSession(%q/%q) 失败: %v", ps.ConvID, ps.AgentID, err)
	}
}

func tstDropProviderSession(t *testing.T, ctx context.Context, s *Store, convID, agentID string) (existed bool, err error) {
	t.Helper()
	err = s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var txErr error
		existed, txErr = tx.DropProviderSession(ctx, convID, agentID)
		return txErr
	})
	return existed, err
}

// TestProviderSessionRoundTrip 覆盖五样东西逐字段的写入与读回，包括 LastTurnAt
// 跨 formatTime/parseTime 之后的精确往返（RFC3339Nano + UTC，纳秒精度要对得上）。
func TestProviderSessionRoundTrip(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "会话状态往返")
	ps := tstBaseSession(conv.ID, "alpha")

	tstSetProviderSession(t, ctx, s, ps)

	got, ok, err := s.ProviderSession(ctx, conv.ID, "alpha")
	if err != nil {
		t.Fatalf("ProviderSession 返回错误: %v", err)
	}
	if !ok {
		t.Fatal("ProviderSession 返回 ok=false，期望 true")
	}
	if got.ConvID != ps.ConvID || got.AgentID != ps.AgentID {
		t.Errorf("身份读回 (%q,%q)，期望 (%q,%q)", got.ConvID, got.AgentID, ps.ConvID, ps.AgentID)
	}
	if got.ProviderSessionID != ps.ProviderSessionID {
		t.Errorf("ProviderSessionID = %q，期望 %q", got.ProviderSessionID, ps.ProviderSessionID)
	}
	if got.CursorSeq != ps.CursorSeq {
		t.Errorf("CursorSeq = %d，期望 %d", got.CursorSeq, ps.CursorSeq)
	}
	if !got.HaveCostTotal || got.CostTotalUSD != ps.CostTotalUSD {
		t.Errorf("CostTotalUSD/HaveCostTotal = (%v,%v)，期望 (%v,%v)",
			got.CostTotalUSD, got.HaveCostTotal, ps.CostTotalUSD, ps.HaveCostTotal)
	}
	if got.IdentityFP != ps.IdentityFP {
		t.Errorf("IdentityFP = %q，期望 %q", got.IdentityFP, ps.IdentityFP)
	}
	if got.Spoken != ps.Spoken {
		t.Errorf("Spoken = %v，期望 %v", got.Spoken, ps.Spoken)
	}
	if !got.LastTurnAt.Equal(ps.LastTurnAt) {
		t.Errorf("LastTurnAt = %v，期望 %v（纳秒精度也要对得上）", got.LastTurnAt, ps.LastTurnAt)
	}
	if _, off := got.LastTurnAt.Zone(); off != 0 {
		t.Errorf("LastTurnAt 时区偏移 = %d 秒，期望 UTC（0）", off)
	}
}

// TestProviderSessionUpsertOverwritesNotInserts 覆盖 upsert 语义：
// 同一个 (conv_id, agent_id) 写两次是覆盖，不是插两行。
func TestProviderSessionUpsertOverwritesNotInserts(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "会话状态 upsert")
	first := tstBaseSession(conv.ID, "alpha")
	tstSetProviderSession(t, ctx, s, first)

	second := first
	second.ProviderSessionID = "sess-alpha-v2"
	second.CursorSeq = 9
	second.CostTotalUSD = 1.23
	second.LastTurnAt = tstFixedSessionTime.Add(time.Hour)
	second.IdentityFP = "fp-alpha-v2"
	tstSetProviderSession(t, ctx, s, second)

	raw := tstRawDB(t, s.Path())
	if n := tstScanInt(t, raw,
		`SELECT COUNT(*) FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`, conv.ID, "alpha"); n != 1 {
		t.Fatalf("写了两次之后有 %d 行，期望仍是 1 行（upsert 不是插第二行）", n)
	}

	got, ok, err := s.ProviderSession(ctx, conv.ID, "alpha")
	if err != nil || !ok {
		t.Fatalf("ProviderSession 返回 (%+v, %v, %v)", got, ok, err)
	}
	if got.ProviderSessionID != second.ProviderSessionID || got.CursorSeq != second.CursorSeq {
		t.Errorf("读回 %+v，期望后写的覆盖先写的 %+v", got, second)
	}
}

// TestProviderSessionCostTotalTriState 覆盖成本基线的可空三态：
// grok / agy 从不报金额，HaveCostTotal 恒为 false，此时 cost_total_usd 必须是 NULL；
// 「见过金额且那次恰好是 0」是另一回事，HaveCostTotal 为 true 时即使金额是 0
// 也不能写成 NULL。两者在 Go 层与 SQL 层都要能区分。
func TestProviderSessionCostTotalTriState(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "成本基线三态")

	t.Run("HaveCostTotal 为 false 时 cost_total_usd 是 NULL 不是 0", func(t *testing.T) {
		ps := tstBaseSession(conv.ID, "grok-agent")
		ps.HaveCostTotal = false
		ps.CostTotalUSD = 0
		tstSetProviderSession(t, ctx, s, ps)

		raw := tstRawDB(t, s.Path())
		typ := tstScanText(t, raw,
			`SELECT typeof(cost_total_usd) FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`,
			conv.ID, "grok-agent")
		if typ != "null" {
			t.Errorf("cost_total_usd 的 SQLite 类型是 %q，期望 null", typ)
		}

		got, ok, err := s.ProviderSession(ctx, conv.ID, "grok-agent")
		if err != nil || !ok {
			t.Fatalf("ProviderSession 返回 (%+v, %v, %v)", got, ok, err)
		}
		if got.HaveCostTotal {
			t.Error("HaveCostTotal 读回 true，期望 false")
		}
		if got.CostTotalUSD != 0 {
			t.Errorf("CostTotalUSD = %v，期望零值（HaveCostTotal 才是权威，不该用它反推）", got.CostTotalUSD)
		}
	})

	t.Run("HaveCostTotal 为 true 且金额恰好是 0 时，必须与上面区分开", func(t *testing.T) {
		ps := tstBaseSession(conv.ID, "claude-agent")
		ps.HaveCostTotal = true
		ps.CostTotalUSD = 0
		tstSetProviderSession(t, ctx, s, ps)

		raw := tstRawDB(t, s.Path())
		typ := tstScanText(t, raw,
			`SELECT typeof(cost_total_usd) FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`,
			conv.ID, "claude-agent")
		if typ == "null" {
			t.Fatal("cost_total_usd 的 SQLite 类型是 null，期望是数值（HaveCostTotal=true 时不该写 NULL）")
		}

		got, ok, err := s.ProviderSession(ctx, conv.ID, "claude-agent")
		if err != nil || !ok {
			t.Fatalf("ProviderSession 返回 (%+v, %v, %v)", got, ok, err)
		}
		if !got.HaveCostTotal {
			t.Error("HaveCostTotal 读回 false，期望 true")
		}
		if got.CostTotalUSD != 0 {
			t.Errorf("CostTotalUSD = %v，期望 0", got.CostTotalUSD)
		}
	})
}

// TestDropProviderSessionRemovesRow 覆盖作废：删掉之后 Store.ProviderSession
// 必须返回 ok=false，而不是留下一行字段为空的记录（那会与「还没开过口」长得一样）。
func TestDropProviderSessionRemovesRow(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "作废会话状态")
	ps := tstBaseSession(conv.ID, "alpha")
	tstSetProviderSession(t, ctx, s, ps)

	if _, ok, err := s.ProviderSession(ctx, conv.ID, "alpha"); err != nil || !ok {
		t.Fatalf("作废之前应当读得到，实际 ok=%v err=%v", ok, err)
	}

	existed, err := tstDropProviderSession(t, ctx, s, conv.ID, "alpha")
	if err != nil {
		t.Fatalf("DropProviderSession 失败: %v", err)
	}
	if !existed {
		t.Error("existed 应当为 true：这一行删之前确实存在")
	}

	_, ok, err := s.ProviderSession(ctx, conv.ID, "alpha")
	if err != nil {
		t.Fatalf("作废之后 ProviderSession 报错，期望 ok=false 且 err=nil: %v", err)
	}
	if ok {
		t.Error("作废之后 ProviderSession 仍返回 ok=true")
	}
}

// TestDropProviderSessionOnMissingRowIsIdempotent 覆盖幂等：删一个不存在的不报错，
// 且 existed 如实报 false（不是「本来就没有」也被误报成「删掉了一行」）。
func TestDropProviderSessionOnMissingRowIsIdempotent(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "作废一个从没写过的")

	existed, err := tstDropProviderSession(t, ctx, s, conv.ID, "never-existed")
	if err != nil {
		t.Errorf("删一个不存在的 provider session 不该报错，实际: %v", err)
	}
	if existed {
		t.Error("existed 应当为 false：这一行从来没存在过")
	}
}

// TestDropProviderSessionSucceedsOnCorruptedRow 是任务 A 修的那个真实缺陷的
// 回归用例：先读一次再删的旧实现会在这里失败 —— scanProviderSession 解析
// last_turn_at 时对一个不合法的时间字符串会报错，那次读的失败会让整条
// DropProviderSession 跟着失败。而 /session rebuild 存在的意义恰恰是让用户
// 能清掉这类损坏状态，不能因为状态已经坏了就连清都清不掉。
//
// 直接用底层连接（tstRawDB）绕过 SetProviderSession 写入一行 last_turn_at
// 是垃圾字符串的记录，模拟「已经损坏」的库；先证明这一行确实读不出来
// （Store.ProviderSession 报错，不是简单的 ok=false），再证明
// Tx.DropProviderSession 仍然能把它删掉、existed=true、err=nil，
// 且删除之后 ProviderSession 恢复成正常的 ok=false（不再报错）。
func TestDropProviderSessionSucceedsOnCorruptedRow(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "损坏的 provider session")

	raw := tstRawDB(t, s.Path())
	if _, err := raw.ExecContext(ctx, `
INSERT INTO provider_sessions (
    conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd,
    have_cost_total, last_turn_at, identity_fp, spoken
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conv.ID, "alpha", "sess-corrupt", 0, nil, 0, "这不是一个合法的 RFC3339 时间戳", "fp", 0,
	); err != nil {
		t.Fatalf("造损坏行失败: %v", err)
	}

	if _, _, err := s.ProviderSession(ctx, conv.ID, "alpha"); err == nil {
		t.Fatal("前置条件不成立：这一行的 last_turn_at 应当解析失败，ProviderSession 应当报错")
	}

	existed, err := tstDropProviderSession(t, ctx, s, conv.ID, "alpha")
	if err != nil {
		t.Fatalf("DropProviderSession 对损坏行应当照常成功，实际报错: %v（先读一次再删的旧实现会在这里失败）", err)
	}
	if !existed {
		t.Error("existed 应当为 true：删之前这一行确实存在，只是读不出来")
	}

	if _, ok, err := s.ProviderSession(ctx, conv.ID, "alpha"); err != nil || ok {
		t.Errorf("删掉损坏行之后 ProviderSession 应当是 ok=false err=nil，实际 ok=%v err=%v", ok, err)
	}
}

// TestProviderSessionUnwrittenReturnsNotOK 覆盖：读一个从没写过的返回 ok=false
// 且 error 为 nil——这是「还没开过口」，不是一次读取失败。
func TestProviderSessionUnwrittenReturnsNotOK(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "从没写过 provider session")

	got, ok, err := s.ProviderSession(ctx, conv.ID, "alpha")
	if err != nil {
		t.Errorf("从没写过应当 ok=false 且 err=nil，实际报错: %v", err)
	}
	if ok {
		t.Errorf("从没写过应当 ok=false，实际 true，got=%+v", got)
	}
}
