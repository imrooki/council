// tx_test.go 覆盖 08 规格 E 节的原子写单元：
// WriteTx 的提交与回滚边界、AppendMessage 的字段补全与 seq 分配、以及两个哨兵错误。
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// tstBusinessTables 是 Step 5 有写方法的四张业务表。顺序是固定的：
// map 的遍历顺序在 Go 里是随机的，按这个切片走能让失败输出稳定、可 diff。
var tstBusinessTables = []string{"conversations", "participants", "messages", "cursors"}

// tstRowCounts 用底层连接数一遍这四张表的行数，用来断言「一行都没落库」。
func tstRowCounts(t *testing.T, s *Store) map[string]int64 {
	t.Helper()
	raw := tstRawDB(t, s.Path())
	out := make(map[string]int64, len(tstBusinessTables))
	for _, table := range tstBusinessTables {
		out[table] = tstScanInt(t, raw, `SELECT COUNT(*) FROM `+table)
	}
	return out
}

// tstAssertEmptyDB 断言四张业务表全空。
func tstAssertEmptyDB(t *testing.T, s *Store, when string) {
	t.Helper()
	counts := tstRowCounts(t, s)
	for _, table := range tstBusinessTables {
		if n := counts[table]; n != 0 {
			t.Errorf("%s：表 %s 有 %d 行，期望 0 行（整体回滚）", when, table, n)
		}
	}
}

// TestWriteTxRollsBackOnError 覆盖：fn 返回错误 -> 整体回滚，一行都没落库。
func TestWriteTxRollsBackOnError(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	boom := errors.New("故意让事务失败")

	var convID string
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		conv, err := tx.CreateConversation(ctx, "会被回滚的会话")
		if err != nil {
			return err
		}
		convID = conv.ID
		if err := tx.AddParticipant(ctx, conversation.Participant{
			ConvID:   conv.ID,
			AgentID:  "reviewer",
			Provider: "claude",
			Active:   true,
			JoinedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if _, err := tx.AppendMessage(ctx, tstUserMsg(conv.ID, "这条不该落库")); err != nil {
			return err
		}
		return boom
	})

	if !errors.Is(err, boom) {
		t.Fatalf("WriteTx 返回 %v，期望能被 errors.Is 认出 %v", err, boom)
	}
	tstAssertEmptyDB(t, s, "fn 返回错误之后")

	if _, err := s.Conversation(ctx, convID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("回滚之后读会话 %q 返回 %v，期望包着 ErrConversationNotFound", convID, err)
	}
	convs, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("列会话失败: %v", err)
	}
	if len(convs) != 0 {
		t.Errorf("回滚之后还能列出 %d 个会话，期望 0 个", len(convs))
	}
}

// TestWriteTxRollsBackOnPanic 覆盖：fn panic -> 同样回滚，且 panic 继续往上抛。
// 顺带验证事务没泄漏：写句柄是单连接（D-10），事务漏掉的话后一次写会一直等连接，
// 靠 tstCtx 的超时把「挂死」变成「失败」。
func TestWriteTxRollsBackOnPanic(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	const panicMsg = "故意在事务里炸"

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("WriteTx 把 panic 吞了，期望继续向上抛")
			}
			if got := fmt.Sprint(r); !strings.Contains(got, panicMsg) {
				t.Errorf("抛上来的 panic 值是 %q，期望含 %q", got, panicMsg)
			}
		}()
		_ = s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
			if _, err := tx.CreateConversation(ctx, "panic 之前建的会话"); err != nil {
				// 这里不能用 Fatalf：Goexit 会把后面的 panic 一起跳过，用例就白跑了。
				t.Errorf("panic 之前建会话失败: %v", err)
			}
			panic(panicMsg)
		})
	}()

	tstAssertEmptyDB(t, s, "fn panic 之后")

	// 事务没泄漏的话，紧接着的一次写应当正常成功。
	var conv conversation.Conversation
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, "panic 之后的会话")
		return err
	})
	if err != nil {
		t.Fatalf("panic 之后再写失败: %v；写句柄是单连接，这通常意味着 WriteTx 在 panic 之后没有回滚，事务泄漏了", err)
	}
	if _, err := s.Conversation(ctx, conv.ID); err != nil {
		t.Errorf("panic 之后写进去的会话读不回来: %v", err)
	}
}

// TestWriteTxCommitsEverythingTogether 覆盖：一个事务里做多件事，成功后一起可见。
func TestWriteTxCommitsEverythingTogether(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	joinedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

	var (
		conv conversation.Conversation
		msgs []conversation.Message
	)
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, "一个事务四件事")
		if err != nil {
			return err
		}
		if err := tx.AddParticipant(ctx, conversation.Participant{
			ConvID:      conv.ID,
			AgentID:     "mechanics-expert",
			DisplayName: "力学专家",
			Provider:    "claude",
			Active:      true,
			JoinedAt:    joinedAt,
		}); err != nil {
			return err
		}
		first, err := tx.AppendMessage(ctx, tstUserMsg(conv.ID, "第一条"))
		if err != nil {
			return err
		}
		second, err := tx.AppendMessage(ctx, tstUserMsg(conv.ID, "第二条"))
		if err != nil {
			return err
		}
		msgs = []conversation.Message{first, second}
		return nil
	})
	if err != nil {
		t.Fatalf("WriteTx 失败: %v", err)
	}

	t.Run("同一事务里的两条消息 seq 是 1 和 2", func(t *testing.T) {
		if len(msgs) != 2 {
			t.Fatalf("事务里只拿到 %d 条消息", len(msgs))
		}
		if msgs[0].Seq != 1 || msgs[1].Seq != 2 {
			t.Errorf("两条消息的 seq = %d, %d，期望 1, 2", msgs[0].Seq, msgs[1].Seq)
		}
	})

	t.Run("提交后会话可见", func(t *testing.T) {
		got, err := s.Conversation(ctx, conv.ID)
		if err != nil {
			t.Fatalf("读会话失败: %v", err)
		}
		if got.Title != "一个事务四件事" {
			t.Errorf("标题 = %q，期望 %q", got.Title, "一个事务四件事")
		}
		if got.CreatedAt.IsZero() {
			t.Error("会话的 CreatedAt 是零值")
		}
		if _, off := got.CreatedAt.Zone(); off != 0 {
			t.Errorf("会话的 CreatedAt 时区偏移 = %d 秒，期望 UTC（0）", off)
		}
	})

	t.Run("提交后 participant 可见", func(t *testing.T) {
		ps, err := s.Participants(ctx, conv.ID, false)
		if err != nil {
			t.Fatalf("列 participant 失败: %v", err)
		}
		if len(ps) != 1 {
			t.Fatalf("participant 有 %d 个，期望 1 个", len(ps))
		}
		if ps[0].AgentID != "mechanics-expert" || ps[0].DisplayName != "力学专家" || ps[0].Provider != "claude" {
			t.Errorf("participant = %+v，期望 agent/display/provider 原样存回", ps[0])
		}
		if !ps[0].Active {
			t.Error("刚加入的 participant 不是 active")
		}
		if ps[0].LeftAt != nil {
			t.Errorf("刚加入的 participant 的 LeftAt = %v，期望 nil", *ps[0].LeftAt)
		}
	})

	t.Run("提交后两条消息都可见", func(t *testing.T) {
		got, err := s.Messages(ctx, conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("读消息失败: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("读回 %d 条消息，期望 2 条", len(got))
		}
		if got[0].Text != "第一条" || got[1].Text != "第二条" {
			t.Errorf("消息正文 = %q, %q，期望 第一条, 第二条", got[0].Text, got[1].Text)
		}
	})
}

// TestAppendMessageFillsFields 覆盖 08a：返回的是补全后的 Message，
// 调用方传进来的 ID / Seq / CreatedAt 一律忽略。
func TestAppendMessageFillsFields(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "字段补全")

	// 脏时间取「刚刚」而不是一个远古时刻：远古时刻太好分辨，
	// 连「原样采用调用方传进来的值」的实现都能蒙混过去。
	dirtyTime := time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond)
	dirty := tstUserMsg(conv.ID, "带脏值的第一条")
	dirty.ID = "脏 id，不该被采用"
	dirty.Seq = 12345
	dirty.CreatedAt = dirtyTime

	got := tstAppend(t, ctx, s, dirty)

	t.Run("ID 由 store 生成且非空", func(t *testing.T) {
		if got.ID == "" {
			t.Fatal("返回的 ID 为空")
		}
		if got.ID == dirty.ID {
			t.Errorf("返回的 ID = %q，等于传进去的脏值", got.ID)
		}
		if strings.ContainsAny(got.ID, " \t\r\n") {
			t.Errorf("返回的 ID %q 含空白字符", got.ID)
		}
	})

	t.Run("Seq 从 1 开始且忽略传入值", func(t *testing.T) {
		if got.Seq != 1 {
			t.Errorf("第一条消息的 Seq = %d，期望 1", got.Seq)
		}
	})

	t.Run("CreatedAt 非零且忽略传入值", func(t *testing.T) {
		if got.CreatedAt.IsZero() {
			t.Fatal("返回的 CreatedAt 是零值")
		}
		if got.CreatedAt.Equal(dirtyTime) {
			t.Errorf("返回的 CreatedAt = %v，等于传进去的脏值", got.CreatedAt)
		}
	})

	t.Run("落库的也是补全后的值", func(t *testing.T) {
		stored, err := s.Messages(ctx, conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("读消息失败: %v", err)
		}
		if len(stored) != 1 {
			t.Fatalf("读回 %d 条消息，期望 1 条", len(stored))
		}
		if stored[0].ID != got.ID || stored[0].Seq != got.Seq {
			t.Errorf("落库的 (id, seq) = (%q, %d)，期望与返回值 (%q, %d) 一致",
				stored[0].ID, stored[0].Seq, got.ID, got.Seq)
		}
		if !stored[0].CreatedAt.Equal(got.CreatedAt) {
			t.Errorf("落库的 CreatedAt = %v，期望与返回值 %v 相同", stored[0].CreatedAt, got.CreatedAt)
		}
		if _, off := stored[0].CreatedAt.Zone(); off != 0 {
			t.Errorf("落库的 CreatedAt 时区偏移 = %d 秒，期望 UTC（0）", off)
		}
	})

	t.Run("后续消息的 seq 连续", func(t *testing.T) {
		second := tstAppend(t, ctx, s, tstUserMsg(conv.ID, "第二条"))
		if second.Seq != 2 {
			t.Errorf("第二条消息的 Seq = %d，期望 2", second.Seq)
		}
		if second.ID == got.ID {
			t.Errorf("两条消息的 ID 撞了: %q", second.ID)
		}
	})
}

// TestAppendMessageSeqIsPerConversation 覆盖：seq 是会话内的，不是全局的。
func TestAppendMessageSeqIsPerConversation(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	first := tstNewConv(t, ctx, s, "会话甲")
	second := tstNewConv(t, ctx, s, "会话乙")

	a1 := tstAppend(t, ctx, s, tstUserMsg(first.ID, "甲的第一条"))
	a2 := tstAppend(t, ctx, s, tstUserMsg(first.ID, "甲的第二条"))
	b1 := tstAppend(t, ctx, s, tstUserMsg(second.ID, "乙的第一条"))

	if a1.Seq != 1 || a2.Seq != 2 {
		t.Errorf("会话甲的 seq = %d, %d，期望 1, 2", a1.Seq, a2.Seq)
	}
	if b1.Seq != 1 {
		t.Errorf("会话乙的第一条 seq = %d，期望 1（每个会话各自从 1 开始）", b1.Seq)
	}
}

// TestAppendMessageSentinelErrors 覆盖两个哨兵：非法 kind 与会话不存在。
func TestAppendMessageSentinelErrors(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "哨兵错误")

	tests := []struct {
		name    string
		msg     conversation.Message
		wantErr error
	}{
		{
			name: "kind 是没见过的值",
			msg: conversation.Message{
				ConvID:   conv.ID,
				SenderID: conversation.ReservedSenderUser,
				Kind:     conversation.MessageKind("bogus"),
				Text:     "非法 kind",
			},
			wantErr: conversation.ErrInvalidKind,
		},
		{
			name: "kind 是空串",
			msg: conversation.Message{
				ConvID:   conv.ID,
				SenderID: conversation.ReservedSenderUser,
				Kind:     conversation.MessageKind(""),
				Text:     "空 kind",
			},
			wantErr: conversation.ErrInvalidKind,
		},
		{
			name:    "会话不存在",
			msg:     tstUserMsg("conv-does-not-exist", "孤儿消息"),
			wantErr: conversation.ErrConversationNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var inner error
			outer := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
				_, inner = tx.AppendMessage(ctx, tc.msg)
				return inner
			})
			if !errors.Is(inner, tc.wantErr) {
				t.Errorf("AppendMessage 返回 %v，期望能被 errors.Is 认出 %v", inner, tc.wantErr)
			}
			if !errors.Is(outer, tc.wantErr) {
				t.Errorf("WriteTx 返回 %v，期望把 fn 的错误原样带出来（%v）", outer, tc.wantErr)
			}
		})
	}

	t.Run("失败的追加没有留下任何消息", func(t *testing.T) {
		got, err := s.Messages(ctx, conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("读消息失败: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("读回 %d 条消息，期望 0 条", len(got))
		}
	})
}
