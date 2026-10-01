package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// callSpy 包一层 fakeStore，把关心的几次调用按顺序记下来。
//
// 它存在的理由：「退出的成员发不了言」这条断言，在校验放事务外和放事务内两种实现下
// 都成立，验不出区别。要锁住未决 #13 的修复，得直接断言**调用顺序**：
// 活跃成员判定必须发生在 WriteTx 打开之后、追加消息之前。
type callSpy struct {
	*fakeStore
	calls []string
}

func newCallSpy() *callSpy {
	return &callSpy{fakeStore: newFakeStore()}
}

func (c *callSpy) WriteTx(ctx context.Context, fn func(context.Context, Tx) error) error {
	c.calls = append(c.calls, "WriteTx:begin")
	// 把 spy 自己当 Tx 传下去，这样闭包里的每一次 Tx 调用也会被记下来。
	err := c.fakeStore.WriteTx(ctx, func(ctx context.Context, _ Tx) error {
		return fn(ctx, c)
	})
	c.calls = append(c.calls, "WriteTx:end")
	return err
}

func (c *callSpy) RequireActiveParticipant(ctx context.Context, convID, agentID string) error {
	c.calls = append(c.calls, "RequireActiveParticipant")
	return c.fakeStore.RequireActiveParticipant(ctx, convID, agentID)
}

func (c *callSpy) AppendMessage(ctx context.Context, m Message) (Message, error) {
	c.calls = append(c.calls, "AppendMessage")
	return c.fakeStore.AppendMessage(ctx, m)
}

func (c *callSpy) Participants(ctx context.Context, convID string, includeInactive bool) ([]Participant, error) {
	c.calls = append(c.calls, "Participants")
	return c.fakeStore.Participants(ctx, convID, includeInactive)
}

// TestPostChecksParticipantInsideWriteTx 锁住未决 #13 的修复：
// agent 发言时，活跃成员判定必须在写事务内、且在追加消息之前。
//
// 放在事务外的话，并发发言时读到「活跃」之后、提交之前用户 /leave 先行提交，
// 这条消息照样落库，事后 /who 显示它已退出、left_at 还早于这条消息的 created_at，
// 没有任何一层会报错。本机 -race 跑不起来（无 gcc），复现不了那个竞态，
// 所以改为锁住结构：只要判定在事务里，那个窗口就不存在。
func TestPostChecksParticipantInsideWriteTx(t *testing.T) {
	spy := newCallSpy()
	svc := NewService(spy)
	ctx := context.Background()

	conv, err := svc.Create(ctx, "TOCTOU 用例")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	if err := svc.Join(ctx, conv.ID, Participant{AgentID: "alpha", JoinedAt: testNow}); err != nil {
		t.Fatalf("加入失败: %v", err)
	}

	spy.calls = nil // 只看 Post 这一次的调用序列
	if _, err := svc.Post(ctx, Message{
		ConvID:   conv.ID,
		SenderID: "alpha",
		Kind:     KindAgent,
		Text:     "事务内判定",
		// 脑子必须带：KindAgent 不带脑子会先撞 ErrMissingSenderBrain，
		// 那样这条用例根本走不到它要断言的调用序列。
		SenderProvider: "claude",
		SenderModel:    "opus",
	}); err != nil {
		t.Fatalf("Post 应当成功，实际: %v", err)
	}

	got := strings.Join(spy.calls, " -> ")
	const want = "WriteTx:begin -> RequireActiveParticipant -> AppendMessage -> WriteTx:end"
	if got != want {
		t.Errorf("调用序列不对。\n实际: %s\n期望: %s", got, want)
	}

	// 反面：不得再在事务外用读句柄查一遍活跃成员。留着那一次查询，
	// 窗口就还在，只是多了一层看起来像在防它的代码。
	for _, c := range spy.calls {
		if c == "Participants" {
			t.Errorf("Post 仍然在事务外调用了 Participants，TOCTOU 窗口没被关掉：%s", got)
		}
	}
}

// TestPostSkipsParticipantCheckForUser 锁住保留 sender 的例外：
// 人不是 participant，不需要 join 就能说话，所以不该为它去查活跃成员。
func TestPostSkipsParticipantCheckForUser(t *testing.T) {
	spy := newCallSpy()
	svc := NewService(spy)
	ctx := context.Background()

	conv, err := svc.Create(ctx, "保留 sender 用例")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}

	spy.calls = nil
	if _, err := svc.Post(ctx, Message{
		ConvID:   conv.ID,
		SenderID: ReservedSenderUser,
		Kind:     KindUser,
		Text:     "人说的话",
	}); err != nil {
		t.Fatalf("Post 应当成功，实际: %v", err)
	}

	got := strings.Join(spy.calls, " -> ")
	const want = "WriteTx:begin -> AppendMessage -> WriteTx:end"
	if got != want {
		t.Errorf("保留 sender 不该触发成员判定。\n实际: %s\n期望: %s", got, want)
	}
}

// TestPostRollsBackWhenParticipantCheckFails 锁住判定失败时整个事务回滚、一行不落库。
// 判定既然在事务内，它的失败就该由事务语义兜住，而不是靠调用方记得不要接着写。
func TestPostRollsBackWhenParticipantCheckFails(t *testing.T) {
	spy := newCallSpy()
	svc := NewService(spy)
	ctx := context.Background()

	conv, err := svc.Create(ctx, "回滚用例")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	before := spy.messageCount(conv.ID)

	_, err = svc.Post(ctx, Message{
		ConvID:         conv.ID,
		SenderID:       "从没加入过",
		Kind:           KindAgent,
		Text:           "不该落库",
		SenderProvider: "claude",
		SenderModel:    "opus",
	})
	if err == nil {
		t.Fatal("非成员发言竟然成功了")
	}
	if !errors.Is(err, ErrParticipantNotFound) {
		t.Errorf("错误应当能被 errors.Is 认出 ErrParticipantNotFound，实际: %v", err)
	}
	if got := spy.messageCount(conv.ID); got != before {
		t.Errorf("判定失败却落了库：消息数从 %d 变成 %d", before, got)
	}
	// AppendMessage 根本不该被调用到。
	for _, c := range spy.calls {
		if c == "AppendMessage" {
			t.Errorf("成员判定没过却仍然调用了 AppendMessage：%s", strings.Join(spy.calls, " -> "))
		}
	}
}
