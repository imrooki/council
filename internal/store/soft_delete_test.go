// soft_delete_test.go 覆盖软删会话的 store 侧行为：删的是可见性，不是数据。
// 每一行都在断言「读不出来」的同时断言「库里还在」，这两件事必须同时成立——
// 只测前者会让「改成物理 DELETE」也全绿。
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"council/internal/conversation"
)

// tstSoftDeleteSeed 给一场会话把六条读路径都铺上数据：成员、消息、游标、
// 用量、provider 会话、在飞记录。删除之后逐条断言这些读路径都看不见它。
func tstSoftDeleteSeed(t *testing.T, s *Store, conv conversation.Conversation) {
	t.Helper()
	ctx := tstCtx(t)

	tstJoin(t, ctx, s, conversation.Participant{
		ConvID: conv.ID, AgentID: "alpha", DisplayName: "Alpha",
		Provider: "claude", Active: true,
	})
	tstJoin(t, ctx, s, conversation.Participant{
		ConvID: conv.ID, AgentID: "beta", DisplayName: "Beta",
		Provider: "codex", Active: true,
	})
	tstAppend(t, ctx, s, tstUserMsg(conv.ID, "第一条"))
	tstAppend(t, ctx, s, conversation.Message{
		ConvID: conv.ID, SenderID: "alpha",
		SenderProvider: "claude", SenderModel: "opus",
		Kind: conversation.KindAgent, Text: "回复",
	})
	tstSetCursor(t, ctx, s, conversation.Cursor{
		ConvID: conv.ID, AgentID: "alpha", Seq: 2, Pending: []int64{2},
	})

	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		if err := tx.RecordUsage(ctx, conversation.TurnUsage{
			TurnID: "turn-" + conv.ID, ConvID: conv.ID, AgentID: "alpha",
			Seq: tstInt64Ptr(2), Outcome: 1, InputTokens: tstIntPtr(100),
			CostUSD: tstFloat64Ptr(0.125), CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if err := tx.SetProviderSession(ctx, conversation.ProviderSession{
			ConvID: conv.ID, AgentID: "alpha", ProviderSessionID: "ps-" + conv.ID,
			CursorSeq: 2, CostTotalUSD: 0.5, HaveCostTotal: true,
			LastTurnAt: time.Now().UTC(), IdentityFP: "fp", Spoken: true,
		}); err != nil {
			return err
		}
		return tx.BeginTurn(ctx, conversation.InflightTurn{
			TurnID: "inflight-" + conv.ID, ConvID: conv.ID, AgentID: "alpha",
			Provider: "claude", PlannedSessionID: "planned", CursorSeq: 2,
			ScannedTo: 2, Phase: conversation.PhaseSpawned,
		})
	})
	if err != nil {
		t.Fatalf("给会话 %s 铺数据失败: %v", conv.ID, err)
	}
}

// TestSoftDeleteConversationHidesEveryReadPath 钉住：
//  1. 软删后 Conversations / Conversation(id) 看不见；
//  2. 按 convID 的成员、消息、seq、游标、用量、provider 会话、在飞记录都读不出来；
//  3. 库里的行一条没少，title / team_id / cwd 没被改，deleted_at 是可解析的时间戳；
//  4. 别的会话完全不受影响。
func TestSoftDeleteConversationHidesEveryReadPath(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)

	gone := tstNewConv(t, ctx, s, "要被删的会话")
	kept := tstNewConv(t, ctx, s, "留下的会话")
	tstSoftDeleteSeed(t, s, gone)

	// 删之前每条读路径都能看到它，否则下面的「看不见」没有判别力。
	if _, err := s.Conversation(ctx, gone.ID); err != nil {
		t.Fatalf("删除前 Conversation 就失败: %v", err)
	}
	if msgs, err := s.Messages(ctx, gone.ID, 0, 0); err != nil || len(msgs) != 2 {
		t.Fatalf("删除前 Messages = %v, err=%v，期望 2 条", msgs, err)
	}

	if err := s.SoftDeleteConversation(ctx, gone.ID); err != nil {
		t.Fatalf("SoftDeleteConversation 失败: %v", err)
	}

	t.Run("Conversations 不含它", func(t *testing.T) {
		list, err := s.Conversations(ctx)
		if err != nil {
			t.Fatalf("Conversations 失败: %v", err)
		}
		if len(list) != 1 || list[0].ID != kept.ID {
			t.Errorf("Conversations = %+v，期望只剩 %s", list, kept.ID)
		}
	})

	t.Run("Conversation(id) 报 ErrConversationNotFound", func(t *testing.T) {
		got, err := s.Conversation(ctx, gone.ID)
		if !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("Conversation(已删) 的 err = %v，期望 ErrConversationNotFound", err)
		}
		if got.ID != "" || got.Title != "" {
			t.Errorf("Conversation(已删) 返回了 %+v，期望零值而不是空标题会话", got)
		}
	})

	t.Run("成员读不出来", func(t *testing.T) {
		for _, includeInactive := range []bool{false, true} {
			parts, err := s.Participants(ctx, gone.ID, includeInactive)
			if err != nil {
				t.Fatalf("Participants(includeInactive=%v) 失败: %v", includeInactive, err)
			}
			if len(parts) != 0 {
				t.Errorf("Participants(includeInactive=%v) = %+v，期望空切片", includeInactive, parts)
			}
		}
	})

	t.Run("消息与 seq 读不出来", func(t *testing.T) {
		msgs, err := s.Messages(ctx, gone.ID, 0, 0)
		if err != nil {
			t.Fatalf("Messages 失败: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("Messages = %+v，期望空切片", msgs)
		}
		seq, err := s.MaxSeq(ctx, gone.ID)
		if err != nil {
			t.Fatalf("MaxSeq 失败: %v", err)
		}
		if seq != 0 {
			t.Errorf("MaxSeq = %d，期望 0", seq)
		}
	})

	t.Run("游标读不出来", func(t *testing.T) {
		if _, err := s.Cursor(ctx, gone.ID, "alpha"); !errors.Is(err, conversation.ErrNoCursor) {
			t.Errorf("Cursor(已删) 的 err = %v，期望 ErrNoCursor", err)
		}
		list, err := s.Cursors(ctx, gone.ID)
		if err != nil {
			t.Fatalf("Cursors 失败: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("Cursors = %+v，期望空切片", list)
		}
	})

	t.Run("用量读不出来", func(t *testing.T) {
		usages, err := s.TurnUsages(ctx, gone.ID, "", -1)
		if err != nil {
			t.Fatalf("TurnUsages 失败: %v", err)
		}
		if len(usages) != 0 {
			t.Errorf("TurnUsages = %+v，期望空切片", usages)
		}
	})

	t.Run("provider 会话与在飞记录按 convID 读不出来", func(t *testing.T) {
		if _, found, err := s.ProviderSession(ctx, gone.ID, "alpha"); err != nil || found {
			t.Errorf("ProviderSession(已删) = found=%v, err=%v，期望 false, nil", found, err)
		}
		if _, found, err := s.InflightFor(ctx, gone.ID, "alpha"); err != nil || found {
			t.Errorf("InflightFor(已删) = found=%v, err=%v，期望 false, nil", found, err)
		}
	})

	t.Run("库里的行一条没少、旧列没被改", func(t *testing.T) {
		raw := tstRawDB(t, s.Path())
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations`); got != 2 {
			t.Errorf("conversations 行数 = %d，期望 2（软删不物理删）", got)
		}
		var (
			title, createdAt, teamID, cwd, deletedAt string
		)
		if err := raw.QueryRow(
			`SELECT title, created_at, team_id, cwd, deleted_at FROM conversations WHERE id = ?`, gone.ID,
		).Scan(&title, &createdAt, &teamID, &cwd, &deletedAt); err != nil {
			t.Fatalf("读已删会话的原始行失败: %v", err)
		}
		if title != gone.Title || createdAt != formatTime(gone.CreatedAt) || teamID != "" || cwd != "" {
			t.Errorf("已删会话的旧列 = title=%q created_at=%q team_id=%q cwd=%q，期望原值",
				title, createdAt, teamID, cwd)
		}
		if _, err := time.Parse(time.RFC3339Nano, deletedAt); err != nil {
			t.Errorf("deleted_at = %q 不是可解析的 RFC3339Nano 时间: %v", deletedAt, err)
		}
		for _, tc := range []struct {
			table string
			want  int64
		}{
			{"participants", 2},
			{"messages", 2},
			{"cursors", 1},
			{"turn_usage", 1},
			{"provider_sessions", 1},
			{"inflight_turns", 1},
		} {
			if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM `+tc.table); got != tc.want {
				t.Errorf("%s 行数 = %d，期望 %d（软删不许碰这些表）", tc.table, got, tc.want)
			}
		}
		if got := tstScanInt(t, raw,
			`SELECT COUNT(*) FROM conversations WHERE id = ? AND deleted_at IS NULL`, kept.ID); got != 1 {
			t.Errorf("留下的会话 deleted_at 不是 NULL，期望它完全没被碰")
		}
	})

	t.Run("别的会话完全不受影响", func(t *testing.T) {
		got, err := s.Conversation(ctx, kept.ID)
		if err != nil {
			t.Fatalf("Conversation(留下的会话) 失败: %v", err)
		}
		if got.ID != kept.ID || got.Title != kept.Title {
			t.Errorf("留下的会话 = %+v，期望原值 %+v", got, kept)
		}
	})
}

// TestSoftDeleteConversationIsIdempotent 钉住幂等语义：
// 同一会话删第二次返回 nil、不改写第一次的 deleted_at；未知 id 才报
// ErrConversationNotFound；重开库之后删除仍然生效。
func TestSoftDeleteConversationIsIdempotent(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "只删一次")
	raw := tstRawDB(t, s.Path())

	if err := s.SoftDeleteConversation(ctx, conv.ID); err != nil {
		t.Fatalf("第一次 SoftDeleteConversation 失败: %v", err)
	}
	var first string
	if err := raw.QueryRow(`SELECT deleted_at FROM conversations WHERE id = ?`, conv.ID).Scan(&first); err != nil {
		t.Fatalf("读第一次的 deleted_at 失败: %v", err)
	}

	if err := s.SoftDeleteConversation(ctx, conv.ID); err != nil {
		t.Errorf("第二次 SoftDeleteConversation = %v，期望 nil（幂等）", err)
	}
	var second string
	if err := raw.QueryRow(`SELECT deleted_at FROM conversations WHERE id = ?`, conv.ID).Scan(&second); err != nil {
		t.Fatalf("读第二次的 deleted_at 失败: %v", err)
	}
	if second != first {
		t.Errorf("重复删除把 deleted_at 从 %q 改成了 %q，期望保留第一次的时间", first, second)
	}
	if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations WHERE id = ?`, conv.ID); got != 1 {
		t.Errorf("重复删除后行数 = %d，期望 1", got)
	}

	const unknown = "conv-never-existed"
	if err := s.SoftDeleteConversation(ctx, unknown); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("SoftDeleteConversation(未知 id) = %v，期望 ErrConversationNotFound", err)
	}
	if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations`); got != 1 {
		t.Errorf("删未知 id 之后 conversations 行数 = %d，期望仍为 1", got)
	}

	closeFn()
	s2, _ := tstOpenAt(t, dir)
	ctx2 := tstCtx(t)
	if _, err := s2.Conversation(ctx2, conv.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("重开后 Conversation(已删) 的 err = %v，期望 ErrConversationNotFound", err)
	}
	if list, err := s2.Conversations(ctx2); err != nil || len(list) != 0 {
		t.Errorf("重开后 Conversations = %+v, err=%v，期望空列表", list, err)
	}
	var third string
	if err := tstRawDB(t, s2.Path()).QueryRow(
		`SELECT deleted_at FROM conversations WHERE id = ?`, conv.ID).Scan(&third); err != nil {
		t.Fatalf("重开后读 deleted_at 失败: %v", err)
	}
	if third != first {
		t.Errorf("重开后 deleted_at = %q，期望 %q（迁移/重开不许改写它）", third, first)
	}
}
