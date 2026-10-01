// read_test.go 覆盖 08 规格 F 节的读操作、G 节的 participant 语义、
// 游标的读写与 upsert、JSON 字段的往返，以及「重启后数据还在」。
package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"council/internal/conversation"
)

// tstSeedMessages 往会话里按顺序追加 n 条用户消息，返回补全后的那 n 条。
func tstSeedMessages(t *testing.T, ctx context.Context, s *Store, convID string, n int) []conversation.Message {
	t.Helper()
	out := make([]conversation.Message, 0, n)
	for i := 1; i <= n; i++ {
		m := tstAppend(t, ctx, s, tstUserMsg(convID, fmt.Sprintf("第 %d 条", i)))
		out = append(out, m)
	}
	return out
}

// tstSeqs 把消息切片压成 seq 列表，方便和期望值比。
func tstSeqs(msgs []conversation.Message) []int64 {
	out := make([]int64, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Seq)
	}
	return out
}

// TestMessagesAfterSeqAndLimit 覆盖游标读法：严格大于 afterSeq、按 seq 升序、limit 生效、
// limit <= 0 表示不限。
func TestMessagesAfterSeqAndLimit(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "游标读法")
	tstSeedMessages(t, ctx, s, conv.ID, 5)

	tests := []struct {
		name     string
		afterSeq int64
		limit    int
		want     []int64
	}{
		{name: "afterSeq=0 limit=0 读全部", afterSeq: 0, limit: 0, want: []int64{1, 2, 3, 4, 5}},
		{name: "limit 为负同样表示不限", afterSeq: 0, limit: -1, want: []int64{1, 2, 3, 4, 5}},
		{name: "limit 截断前 3 条", afterSeq: 0, limit: 3, want: []int64{1, 2, 3}},
		{name: "afterSeq 是严格大于", afterSeq: 2, limit: 0, want: []int64{3, 4, 5}},
		{name: "afterSeq 与 limit 同时生效", afterSeq: 2, limit: 2, want: []int64{3, 4}},
		{name: "limit 大于剩余条数时全给", afterSeq: 1, limit: 100, want: []int64{2, 3, 4, 5}},
		{name: "afterSeq 等于最大 seq 时读回空", afterSeq: 5, limit: 0, want: []int64{}},
		{name: "afterSeq 超过最大 seq 时读回空", afterSeq: 99, limit: 0, want: []int64{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.Messages(ctx, conv.ID, tc.afterSeq, tc.limit)
			if err != nil {
				t.Fatalf("Messages(%q, %d, %d) 返回错误: %v", conv.ID, tc.afterSeq, tc.limit, err)
			}
			if got == nil {
				t.Fatalf("Messages(%q, %d, %d) 返回 nil，期望空切片",
					conv.ID, tc.afterSeq, tc.limit)
			}
			if gotSeqs := tstSeqs(got); !reflect.DeepEqual(gotSeqs, tc.want) {
				t.Fatalf("Messages(%q, %d, %d) 的 seq = %v，期望 %v",
					conv.ID, tc.afterSeq, tc.limit, gotSeqs, tc.want)
			}
			for i := 1; i < len(got); i++ {
				if got[i-1].Seq >= got[i].Seq {
					t.Fatalf("结果不是按 seq 升序: %v", tstSeqs(got))
				}
			}
		})
	}
}

// TestMessagesOnEmptyConversation 覆盖「空会话读回空切片而不是 nil」。
func TestMessagesOnEmptyConversation(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "一条消息都没有的会话")

	got, err := s.Messages(ctx, conv.ID, 0, 0)
	if err != nil {
		t.Fatalf("读空会话失败: %v", err)
	}
	if got == nil {
		t.Fatal("空会话读回 nil，期望空切片")
	}
	if len(got) != 0 {
		t.Errorf("空会话读回 %d 条消息", len(got))
	}
}

// TestMaxSeq 覆盖「取某会话的最大 seq」，包括空会话与会话间互不干扰。
func TestMaxSeq(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	empty := tstNewConv(t, ctx, s, "空会话")
	filled := tstNewConv(t, ctx, s, "有三条消息的会话")
	tstSeedMessages(t, ctx, s, filled.ID, 3)

	t.Run("空会话的最大 seq 是 0", func(t *testing.T) {
		got, err := s.MaxSeq(ctx, empty.ID)
		if err != nil {
			t.Fatalf("MaxSeq 返回错误: %v", err)
		}
		if got != 0 {
			t.Errorf("MaxSeq = %d，期望 0", got)
		}
	})

	t.Run("有消息时返回最后一条的 seq", func(t *testing.T) {
		got, err := s.MaxSeq(ctx, filled.ID)
		if err != nil {
			t.Fatalf("MaxSeq 返回错误: %v", err)
		}
		if got != 3 {
			t.Errorf("MaxSeq = %d，期望 3", got)
		}
	})
}

// TestConversationsOrderedByCreatedAtDesc 覆盖规格 F：列全部会话按 created_at 倒序。
func TestConversationsOrderedByCreatedAtDesc(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)

	titles := []string{"最早建的", "中间建的", "最晚建的"}
	created := make([]conversation.Conversation, 0, len(titles))
	for _, title := range titles {
		created = append(created, tstNewConv(t, ctx, s, title))
		// Windows 的墙上时钟按时钟中断推进，粒度可能到十几毫秒。
		// 不睡一下，三个 created_at 可能一模一样，倒序断言会随机红。
		time.Sleep(20 * time.Millisecond)
	}

	got, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations 返回错误: %v", err)
	}
	if len(got) != len(titles) {
		t.Fatalf("列出 %d 个会话，期望 %d 个", len(got), len(titles))
	}

	// 首要断言：created_at 非递增。即使三个时间戳撞在一起，这条也成立。
	for i := 1; i < len(got); i++ {
		if got[i-1].CreatedAt.Before(got[i].CreatedAt) {
			t.Fatalf("第 %d 个会话的 created_at %v 早于第 %d 个的 %v，不是倒序",
				i-1, got[i-1].CreatedAt, i, got[i].CreatedAt)
		}
	}

	// 时间戳互不相同时，顺序必须正好是建会话顺序的倒序。
	distinct := !got[0].CreatedAt.Equal(got[1].CreatedAt) && !got[1].CreatedAt.Equal(got[2].CreatedAt)
	if distinct {
		for i, conv := range got {
			want := created[len(created)-1-i]
			if conv.ID != want.ID {
				t.Errorf("倒序第 %d 个是 %q（%s），期望 %q（%s）",
					i, conv.ID, conv.Title, want.ID, want.Title)
			}
		}
	}
}

// TestConversationLookup 覆盖按 id 取会话，以及取不到时的哨兵。
func TestConversationLookup(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "按 id 取回来")

	t.Run("取得到时字段原样", func(t *testing.T) {
		got, err := s.Conversation(ctx, conv.ID)
		if err != nil {
			t.Fatalf("Conversation 返回错误: %v", err)
		}
		if got.ID != conv.ID || got.Title != conv.Title {
			t.Errorf("读回 %+v，期望 %+v", got, conv)
		}
		if !got.CreatedAt.Equal(conv.CreatedAt) {
			t.Errorf("读回的 CreatedAt = %v，期望 %v", got.CreatedAt, conv.CreatedAt)
		}
	})

	t.Run("取不到时包了 ErrConversationNotFound", func(t *testing.T) {
		_, err := s.Conversation(ctx, "conv-does-not-exist")
		if !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("Conversation 返回 %v，期望能被 errors.Is 认出 ErrConversationNotFound", err)
		}
	})
}

// TestMessageJSONRoundTrip 覆盖 08a：Recipients 与 Meta 的 JSON 往返，
// nil 存成 [] / {}，读回来是空切片与空 map，不是 nil。
func TestMessageJSONRoundTrip(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "JSON 往返")

	// 先把定向收件人加成 participant：store 层要不要校验收件人，契约没写死，
	// 先 join 能让这条用例在两种实现下都成立。
	for _, agentID := range []string{"reviewer", "mechanics-expert"} {
		tstJoin(t, ctx, s, conversation.Participant{
			ConvID:   conv.ID,
			AgentID:  agentID,
			Active:   true,
			JoinedAt: time.Now().UTC().Add(-time.Hour).Truncate(time.Second),
		})
	}

	nilFields := tstUserMsg(conv.ID, "recipients 与 meta 都是 nil")
	filled := tstUserMsg(conv.ID, "recipients 与 meta 都有值")
	filled.Recipients = []string{"reviewer", "mechanics-expert"}
	filled.Meta = map[string]any{"role": "reviewer", "weight": 0.5, "pinned": true}
	filled.Thought = "这段思维链要存下来（04 §3：默认存）"
	filled.ParentID = "parent-placeholder"

	tstAppend(t, ctx, s, nilFields)
	tstAppend(t, ctx, s, filled)

	got, err := s.Messages(ctx, conv.ID, 0, 0)
	if err != nil {
		t.Fatalf("读消息失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("读回 %d 条消息，期望 2 条", len(got))
	}

	t.Run("nil 的 Recipients 读回空切片而不是 nil", func(t *testing.T) {
		if got[0].Recipients == nil {
			t.Fatal("Recipients 读回 nil，期望空切片")
		}
		if len(got[0].Recipients) != 0 {
			t.Errorf("Recipients = %v，期望空", got[0].Recipients)
		}
	})

	t.Run("nil 的 Meta 读回空 map 而不是 nil", func(t *testing.T) {
		if got[0].Meta == nil {
			t.Fatal("Meta 读回 nil，期望空 map")
		}
		if len(got[0].Meta) != 0 {
			t.Errorf("Meta = %v，期望空", got[0].Meta)
		}
	})

	t.Run("非空的 Recipients 原样往返且保持顺序", func(t *testing.T) {
		if !reflect.DeepEqual(got[1].Recipients, filled.Recipients) {
			t.Errorf("Recipients = %v，期望 %v", got[1].Recipients, filled.Recipients)
		}
	})

	t.Run("非空的 Meta 原样往返", func(t *testing.T) {
		if !reflect.DeepEqual(got[1].Meta, filled.Meta) {
			t.Errorf("Meta = %#v，期望 %#v", got[1].Meta, filled.Meta)
		}
	})

	t.Run("thought 与 parent_id 也往返", func(t *testing.T) {
		if got[1].Thought != filled.Thought {
			t.Errorf("Thought = %q，期望 %q", got[1].Thought, filled.Thought)
		}
		if got[1].ParentID != filled.ParentID {
			t.Errorf("ParentID = %q，期望 %q", got[1].ParentID, filled.ParentID)
		}
		if got[0].ParentID != "" {
			t.Errorf("没有父消息时 ParentID = %q，期望空串", got[0].ParentID)
		}
	})

	t.Run("sender 与 kind 原样往返", func(t *testing.T) {
		if got[0].SenderID != conversation.ReservedSenderUser {
			t.Errorf("SenderID = %q，期望 %q", got[0].SenderID, conversation.ReservedSenderUser)
		}
		if got[0].Kind != conversation.KindUser {
			t.Errorf("Kind = %q，期望 %q", got[0].Kind, conversation.KindUser)
		}
	})
}

// TestCursorReadWrite 覆盖游标：读不到时的哨兵、往返、以及同一 (conv, agent) 的 upsert。
func TestCursorReadWrite(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "游标")
	const agentID = "reviewer"
	updatedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

	t.Run("还没写过时包了 ErrNoCursor", func(t *testing.T) {
		_, err := s.Cursor(ctx, conv.ID, agentID)
		if !errors.Is(err, conversation.ErrNoCursor) {
			t.Errorf("Cursor 返回 %v，期望能被 errors.Is 认出 ErrNoCursor", err)
		}
	})

	t.Run("会话里一个游标都没有时列表为空", func(t *testing.T) {
		got, err := s.Cursors(ctx, conv.ID)
		if err != nil {
			t.Fatalf("Cursors 返回错误: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("列出 %d 个游标，期望 0 个", len(got))
		}
	})

	t.Run("pending 为 nil 时读回空切片而不是 nil", func(t *testing.T) {
		tstSetCursor(t, ctx, s, conversation.Cursor{
			ConvID:    conv.ID,
			AgentID:   agentID,
			Seq:       3,
			Pending:   nil,
			UpdatedAt: updatedAt,
		})
		got, err := s.Cursor(ctx, conv.ID, agentID)
		if err != nil {
			t.Fatalf("Cursor 返回错误: %v", err)
		}
		if got.Pending == nil {
			t.Fatal("Pending 读回 nil，期望空切片")
		}
		if len(got.Pending) != 0 {
			t.Errorf("Pending = %v，期望空", got.Pending)
		}
		if got.Seq != 3 {
			t.Errorf("Seq = %d，期望 3", got.Seq)
		}
		if got.ConvID != conv.ID || got.AgentID != agentID {
			t.Errorf("读回 (%q, %q)，期望 (%q, %q)", got.ConvID, got.AgentID, conv.ID, agentID)
		}
		// UpdatedAt 由调用方给还是由 store 取当前时间，08a 没写死，只断言它不是零值。
		if got.UpdatedAt.IsZero() {
			t.Error("UpdatedAt 是零值")
		}
	})

	t.Run("重复写同一个 (conv, agent) 是 upsert 不是插第二行", func(t *testing.T) {
		tstSetCursor(t, ctx, s, conversation.Cursor{
			ConvID:    conv.ID,
			AgentID:   agentID,
			Seq:       9,
			Pending:   []int64{10, 11},
			UpdatedAt: updatedAt.Add(time.Minute),
		})

		list, err := s.Cursors(ctx, conv.ID)
		if err != nil {
			t.Fatalf("Cursors 返回错误: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("写了两次之后有 %d 个游标，期望 1 个", len(list))
		}

		got, err := s.Cursor(ctx, conv.ID, agentID)
		if err != nil {
			t.Fatalf("Cursor 返回错误: %v", err)
		}
		if got.Seq != 9 {
			t.Errorf("Seq = %d，期望 9（后写的覆盖先写的）", got.Seq)
		}
		if !reflect.DeepEqual(got.Pending, []int64{10, 11}) {
			t.Errorf("Pending = %v，期望 [10 11]", got.Pending)
		}
	})

	t.Run("同一会话里不同 agent 各有一个游标", func(t *testing.T) {
		tstSetCursor(t, ctx, s, conversation.Cursor{
			ConvID:    conv.ID,
			AgentID:   "mechanics-expert",
			Seq:       1,
			UpdatedAt: updatedAt,
		})
		list, err := s.Cursors(ctx, conv.ID)
		if err != nil {
			t.Fatalf("Cursors 返回错误: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("列出 %d 个游标，期望 2 个", len(list))
		}
	})
}

// TestParticipantLifecycle 覆盖规格 G：加入、重复加入、退出（软删除）、
// 退出后消息仍在、重新加入。
func TestParticipantLifecycle(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "成员进出")
	const agentID = "reviewer"

	// 三个时间点都放在过去：这样无论实现是采用调用方传进来的 joined_at，
	// 还是自己取当前时间，「重新加入的时间不早于退出时间」这条断言都成立。
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	joinedFirst := base
	leftAt := base.Add(10 * time.Minute)
	joinedAgain := base.Add(20 * time.Minute)

	newcomer := conversation.Participant{
		ConvID:      conv.ID,
		AgentID:     agentID,
		DisplayName: "评审",
		Provider:    "claude",
		Active:      true,
		JoinedAt:    joinedFirst,
	}

	t.Run("加入之后是 active 且 LeftAt 为 nil", func(t *testing.T) {
		tstJoin(t, ctx, s, newcomer)
		ps, err := s.Participants(ctx, conv.ID, false)
		if err != nil {
			t.Fatalf("列 participant 失败: %v", err)
		}
		if len(ps) != 1 {
			t.Fatalf("participant 有 %d 个，期望 1 个", len(ps))
		}
		if !ps[0].Active {
			t.Error("刚加入的不是 active")
		}
		if ps[0].LeftAt != nil {
			t.Errorf("LeftAt = %v，期望 nil", *ps[0].LeftAt)
		}
		if ps[0].JoinedAt.IsZero() {
			t.Error("JoinedAt 是零值")
		}
		if _, off := ps[0].JoinedAt.Zone(); off != 0 {
			t.Errorf("JoinedAt 时区偏移 = %d 秒，期望 UTC（0）", off)
		}
	})

	t.Run("重复加入不产生第二行", func(t *testing.T) {
		tstJoin(t, ctx, s, newcomer)
		ps, err := s.Participants(ctx, conv.ID, true)
		if err != nil {
			t.Fatalf("列 participant 失败: %v", err)
		}
		if len(ps) != 1 {
			t.Errorf("重复加入之后 participant 有 %d 个，期望仍是 1 个", len(ps))
		}
	})

	// 这条消息是下面「退出不删历史」那条用例的证据，必须在退出之前发出去。
	spoken := tstAppend(t, ctx, s, conversation.Message{
		ConvID:   conv.ID,
		SenderID: agentID,
		Kind:     conversation.KindAgent,
		Text:     "我退出之前说过这句话",
	})

	t.Run("退出之后 active 为 false 且 LeftAt 非 nil", func(t *testing.T) {
		tstLeave(t, ctx, s, conv.ID, agentID, leftAt)

		active, err := s.Participants(ctx, conv.ID, false)
		if err != nil {
			t.Fatalf("列活跃 participant 失败: %v", err)
		}
		if len(active) != 0 {
			t.Errorf("退出之后活跃 participant 还有 %d 个，期望 0 个", len(active))
		}

		all, err := s.Participants(ctx, conv.ID, true)
		if err != nil {
			t.Fatalf("列全部 participant 失败: %v", err)
		}
		if len(all) != 1 {
			t.Fatalf("含已退出的列表有 %d 个，期望 1 个（软删除不删行）", len(all))
		}
		if all[0].Active {
			t.Error("退出之后仍然是 active")
		}
		if all[0].LeftAt == nil {
			t.Fatal("退出之后 LeftAt 仍是 nil")
		}
		if !all[0].LeftAt.Equal(leftAt) {
			t.Errorf("LeftAt = %v，期望就是传给 DeactivateParticipant 的 %v", *all[0].LeftAt, leftAt)
		}
	})

	t.Run("退出之后它发过的消息仍然读得到", func(t *testing.T) {
		msgs, err := s.Messages(ctx, conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("读消息失败: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("读回 %d 条消息，期望 1 条（退出不得删历史）", len(msgs))
		}
		if msgs[0].ID != spoken.ID || msgs[0].Seq != spoken.Seq {
			t.Errorf("读回的消息是 %+v，期望就是退出前发的那条 %+v", msgs[0], spoken)
		}
		if msgs[0].SenderID != agentID {
			t.Errorf("SenderID = %q，期望 %q", msgs[0].SenderID, agentID)
		}
		if msgs[0].Kind != conversation.KindAgent {
			t.Errorf("Kind = %q，期望 %q", msgs[0].Kind, conversation.KindAgent)
		}
	})

	t.Run("重新加入变回 active 且 LeftAt 清空", func(t *testing.T) {
		again := newcomer
		again.JoinedAt = joinedAgain
		tstJoin(t, ctx, s, again)

		all, err := s.Participants(ctx, conv.ID, true)
		if err != nil {
			t.Fatalf("列全部 participant 失败: %v", err)
		}
		if len(all) != 1 {
			t.Fatalf("重新加入之后有 %d 行，期望仍是 1 行", len(all))
		}
		if !all[0].Active {
			t.Error("重新加入之后不是 active")
		}
		if all[0].LeftAt != nil {
			t.Errorf("重新加入之后 LeftAt = %v，期望 nil", *all[0].LeftAt)
		}
		if all[0].JoinedAt.Before(leftAt) {
			t.Errorf("重新加入之后 JoinedAt = %v，早于退出时间 %v，说明加入时间没更新",
				all[0].JoinedAt, leftAt)
		}

		active, err := s.Participants(ctx, conv.ID, false)
		if err != nil {
			t.Fatalf("列活跃 participant 失败: %v", err)
		}
		if len(active) != 1 {
			t.Errorf("重新加入之后活跃 participant 有 %d 个，期望 1 个", len(active))
		}
	})
}

// TestParticipantsIncludeInactive 单独覆盖 includeInactive 的两种取值：
// 一个还在、一个已退出，两种取值各看到什么。
func TestParticipantsIncludeInactive(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "两个成员一走一留")
	joinedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

	for _, agentID := range []string{"stays", "leaves"} {
		tstJoin(t, ctx, s, conversation.Participant{
			ConvID:   conv.ID,
			AgentID:  agentID,
			Active:   true,
			JoinedAt: joinedAt,
		})
	}
	tstLeave(t, ctx, s, conv.ID, "leaves", joinedAt.Add(5*time.Minute))

	tests := []struct {
		name            string
		includeInactive bool
		want            []string
	}{
		{name: "includeInactive=false 只看到还在的", includeInactive: false, want: []string{"stays"}},
		{name: "includeInactive=true 连退出的一起看到", includeInactive: true, want: []string{"leaves", "stays"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps, err := s.Participants(ctx, conv.ID, tc.includeInactive)
			if err != nil {
				t.Fatalf("Participants 返回错误: %v", err)
			}
			if len(ps) != len(tc.want) {
				t.Fatalf("列出 %d 个 participant，期望 %d 个: %+v", len(ps), len(tc.want), ps)
			}
			// 列表顺序不属于契约面，比集合而不是比顺序。
			seen := make(map[string]bool, len(ps))
			for _, p := range ps {
				seen[p.AgentID] = true
			}
			for _, agentID := range tc.want {
				if !seen[agentID] {
					t.Errorf("结果里没有 %q: %+v", agentID, ps)
				}
			}
		})
	}
}

// TestDataSurvivesRestart 覆盖 05 §5 的「重启后恢复」：
// 写入、Close、在同一个目录上重新 Open，四类数据都还在。
func TestDataSurvivesRestart(t *testing.T) {
	dir := tstDataDir(t)
	ctx := tstCtx(t)

	first, closeFirst := tstOpenAt(t, dir)
	conv := tstNewConv(t, ctx, first, "重启之前的会话")
	tstJoin(t, ctx, first, conversation.Participant{
		ConvID:      conv.ID,
		AgentID:     "reviewer",
		DisplayName: "评审",
		Provider:    "claude",
		Active:      true,
		JoinedAt:    time.Now().UTC().Add(-time.Hour).Truncate(time.Second),
	})
	want := tstSeedMessages(t, ctx, first, conv.ID, 3)
	tstSetCursor(t, ctx, first, conversation.Cursor{
		ConvID:    conv.ID,
		AgentID:   "reviewer",
		Seq:       2,
		Pending:   []int64{3},
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	})
	closeFirst()

	second, _ := tstOpenAt(t, dir)

	t.Run("会话还在", func(t *testing.T) {
		got, err := second.Conversation(ctx, conv.ID)
		if err != nil {
			t.Fatalf("重开之后读会话失败: %v", err)
		}
		if got.Title != conv.Title {
			t.Errorf("标题 = %q，期望 %q", got.Title, conv.Title)
		}
		if !got.CreatedAt.Equal(conv.CreatedAt) {
			t.Errorf("CreatedAt = %v，期望 %v", got.CreatedAt, conv.CreatedAt)
		}
	})

	t.Run("participant 还在", func(t *testing.T) {
		ps, err := second.Participants(ctx, conv.ID, false)
		if err != nil {
			t.Fatalf("重开之后列 participant 失败: %v", err)
		}
		if len(ps) != 1 || ps[0].AgentID != "reviewer" {
			t.Fatalf("重开之后的 participant = %+v，期望只有 reviewer", ps)
		}
	})

	t.Run("消息还在且 seq 不变", func(t *testing.T) {
		got, err := second.Messages(ctx, conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("重开之后读消息失败: %v", err)
		}
		if !reflect.DeepEqual(tstSeqs(got), tstSeqs(want)) {
			t.Fatalf("重开之后的 seq = %v，期望 %v", tstSeqs(got), tstSeqs(want))
		}
		for i := range got {
			if got[i].ID != want[i].ID || got[i].Text != want[i].Text {
				t.Errorf("第 %d 条读回 (%q, %q)，期望 (%q, %q)",
					i, got[i].ID, got[i].Text, want[i].ID, want[i].Text)
			}
		}
	})

	t.Run("游标还在", func(t *testing.T) {
		got, err := second.Cursor(ctx, conv.ID, "reviewer")
		if err != nil {
			t.Fatalf("重开之后读游标失败: %v", err)
		}
		if got.Seq != 2 {
			t.Errorf("游标 Seq = %d，期望 2", got.Seq)
		}
		if !reflect.DeepEqual(got.Pending, []int64{3}) {
			t.Errorf("游标 Pending = %v，期望 [3]", got.Pending)
		}
	})

	t.Run("重开之后还能接着往下写", func(t *testing.T) {
		next := tstAppend(t, ctx, second, tstUserMsg(conv.ID, "重启之后的新消息"))
		if next.Seq != int64(len(want))+1 {
			t.Errorf("重开之后新消息的 seq = %d，期望 %d", next.Seq, len(want)+1)
		}
	})
}
