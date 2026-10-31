package store

// P2：事务内的 requireConversation 必须过滤软删会话，否则事务外查到、
// 事务内被软删的窗口里正文仍会落库。

import (
	"context"
	"errors"
	"testing"
	"time"

	"council/internal/conversation"
)

func TestSoftDeletedConversationRejectsAppendInsideTx(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "soft delete write")
	if err := s.SoftDeleteConversation(ctx, conv.ID); err != nil {
		t.Fatalf("SoftDeleteConversation: %v", err)
	}

	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		_, err := tx.AppendMessage(ctx, conversation.Message{
			ConvID:    conv.ID,
			SenderID:  conversation.ReservedSenderSystem,
			Kind:      conversation.KindSystem,
			Text:      "should not land",
			CreatedAt: time.Now().UTC(),
		})
		return err
	})
	if !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Fatalf("软删会话仍允许 AppendMessage，err=%v", err)
	}
}
