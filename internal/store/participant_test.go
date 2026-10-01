package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"council/internal/conversation"
)

// TestRequireActiveParticipant 覆盖事务内活跃成员判定的四种局面。
//
// 这个方法是为未决 #13 加的：Service.Post 原先在事务外用读句柄查一次活跃成员，
// 与写事务之间有窗口。判定挪进事务之后，窗口不存在了。
// 「判定发生在事务内」这条结构性质由 internal/conversation 的 TOCTOU 用例锁；
// 这里锁的是判定本身的语义。
func TestRequireActiveParticipant(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "活跃成员判定用例")
	other := tstNewConv(t, ctx, s, "另一场会话")

	mustWrite := func(t *testing.T, fn func(ctx context.Context, tx conversation.Tx) error) {
		t.Helper()
		if err := s.WriteTx(ctx, fn); err != nil {
			t.Fatalf("写事务失败: %v", err)
		}
	}
	mustWrite(t, func(ctx context.Context, tx conversation.Tx) error {
		return tx.AddParticipant(ctx, conversation.Participant{
			ConvID:   conv.ID,
			AgentID:  "alpha",
			Provider: "claude",
			Active:   true,
			JoinedAt: time.Now().UTC(),
		})
	})

	// check 把判定包进一个写事务里跑一次，返回它的结果。
	check := func(convID, agentID string) error {
		return s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
			return tx.RequireActiveParticipant(ctx, convID, agentID)
		})
	}

	t.Run("活跃成员通过", func(t *testing.T) {
		if err := check(conv.ID, "alpha"); err != nil {
			t.Errorf("活跃成员应当通过，实际: %v", err)
		}
	})

	t.Run("从没加入过的被拒", func(t *testing.T) {
		err := check(conv.ID, "从没加入过")
		if !errors.Is(err, conversation.ErrParticipantNotFound) {
			t.Errorf("应当返回包了 ErrParticipantNotFound 的错误，实际: %v", err)
		}
	})

	t.Run("同名但在别的会话里被拒", func(t *testing.T) {
		// 判定必须同时看 conv_id：只按 agent_id 查的话，
		// 一个 agent 在 A 会话活跃就能往 B 会话发言。
		err := check(other.ID, "alpha")
		if !errors.Is(err, conversation.ErrParticipantNotFound) {
			t.Errorf("跨会话应当被拒，实际: %v", err)
		}
	})

	t.Run("退出之后被拒", func(t *testing.T) {
		mustWrite(t, func(ctx context.Context, tx conversation.Tx) error {
			return tx.DeactivateParticipant(ctx, conv.ID, "alpha", time.Now().UTC())
		})
		err := check(conv.ID, "alpha")
		if !errors.Is(err, conversation.ErrParticipantNotFound) {
			t.Errorf("已退出的成员应当被拒，实际: %v", err)
		}
	})

	t.Run("重新加入之后又通过", func(t *testing.T) {
		// 退出是软删除，行还在、active = 0。重新加入把它激活回来，
		// 判定必须跟着变 —— 若实现只查「行存在」而不查 active，上一条就会漏，
		// 而这一条会掩盖那个漏洞，所以两条要一起看。
		mustWrite(t, func(ctx context.Context, tx conversation.Tx) error {
			return tx.AddParticipant(ctx, conversation.Participant{
				ConvID:   conv.ID,
				AgentID:  "alpha",
				Provider: "claude",
				Active:   true,
				JoinedAt: time.Now().UTC(),
			})
		})
		if err := check(conv.ID, "alpha"); err != nil {
			t.Errorf("重新加入后应当通过，实际: %v", err)
		}
	})
}
