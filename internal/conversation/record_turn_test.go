// record_turn_test.go 覆盖 Step 7 契约 10a §2.4 的 Service.RecordTurn，在
// internal/conversation 这一层只测两类不需要真实事务语义的东西：
//
//  1. 事务外的校验（TurnID 为空、msg 与 Seq 是否为 nil 不匹配、以及与 Post 完全相同的
//     那一组）——这些在进入 WriteTx 之前就返回，fakeStore 足够验证。
//  2. 四件事的调用顺序，靠 toctou_test.go 里那份 callSpy 扩出来的三个新桩记录下来。
//     它照着 TestPostChecksParticipantInsideWriteTx 的思路：验的是结构（顺序、
//     谁在什么条件下被跳过），不是真实的原子性。
//
// 真正的事务回滚（任一步失败、整轮回滚）只有真库能验，见
// internal/store/record_turn_test.go——那边用真实的 *store.Store 而不是这份 fakeStore，
// 因为 fakeStore 给 Step 7 三个新方法配的只是空桩，证明不了任何回滚行为。
package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// callSpy 的三个新桩：把 Step 7 加的三个 Tx 方法也记进调用序列。
// callSpy 本体定义在 toctou_test.go；Go 允许同一个类型的方法分散在同包的多个文件里。
func (c *callSpy) RecordUsage(ctx context.Context, u TurnUsage) error {
	c.calls = append(c.calls, "RecordUsage")
	return c.fakeStore.RecordUsage(ctx, u)
}

func (c *callSpy) SetProviderSession(ctx context.Context, s ProviderSession) error {
	c.calls = append(c.calls, "SetProviderSession")
	return c.fakeStore.SetProviderSession(ctx, s)
}

func (c *callSpy) DropProviderSession(ctx context.Context, convID, agentID string) (bool, error) {
	c.calls = append(c.calls, "DropProviderSession")
	return c.fakeStore.DropProviderSession(ctx, convID, agentID)
}

// TestRecordTurnCallOrderSuccess 锁住「四件事同一个 WriteTx」里的调用顺序：
// 活跃成员判定 -> 追加消息 -> 记用量 -> 更新会话状态，全部在同一次 WriteTx 之内。
// 顺序不对、或者哪一步被挪到了事务外，这条会先红。
func TestRecordTurnCallOrderSuccess(t *testing.T) {
	spy := newCallSpy()
	svc := NewService(spy)
	ctx := context.Background()

	conv, err := svc.Create(ctx, "RecordTurn 调用顺序")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	joinAgent(t, svc, conv.ID, "alpha")

	spy.calls = nil
	msg := Message{
		ConvID: conv.ID, SenderID: "alpha", Kind: KindAgent, Text: "一句话",
		SenderProvider: "claude", SenderModel: "opus",
	}
	seq := int64(1)
	u := TurnUsage{TurnID: "turn-call-order-1", ConvID: conv.ID, AgentID: "alpha", Seq: &seq, Outcome: 1}
	session := ProviderSession{ConvID: conv.ID, AgentID: "alpha", ProviderSessionID: "sess-1"}

	if _, err := svc.RecordTurn(ctx, &msg, u, &session, false); err != nil {
		t.Fatalf("RecordTurn 应当成功，实际: %v", err)
	}

	got := strings.Join(spy.calls, " -> ")
	const want = "WriteTx:begin -> RequireActiveParticipant -> AppendMessage -> RecordUsage -> SetProviderSession -> WriteTx:end"
	if got != want {
		t.Errorf("调用序列不对。\n实际: %s\n期望: %s", got, want)
	}
}

// TestRecordTurnCallOrderSessionNilTouchesNothing 覆盖未决 #20 之后的新语义：
// session 为 nil 时既不调 SetProviderSession 也不调 DropProviderSession——
// 「不碰这一行」，不是「删行」。这条同时用 msg=nil（失败轮），顺带覆盖失败轮
// 不经过 RequireActiveParticipant / AppendMessage 这两步。
//
// 本用例此前名为 TestRecordTurnCallOrderSessionNil，断言的是旧语义（session
// 为 nil 时走 DropProviderSession）。旧语义正是未决 #20 记录的那个 bug 的
// 直接成因：起进程之前就拒绝的那几支传下去的 session 本来就是 nil，与
// 「决定作废会话」无关，却被这条断言锁死的行为无条件删掉了一条可能还在的
// 会话。这里不是为了让用例变绿而绕开断言，是判定「它原本想验的东西」已经
// 被裁决判定为错误行为之后，把断言改成正确行为。
func TestRecordTurnCallOrderSessionNilTouchesNothing(t *testing.T) {
	spy := newCallSpy()
	svc := NewService(spy)
	ctx := context.Background()

	conv, err := svc.Create(ctx, "session=nil 调用顺序")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}

	spy.calls = nil
	u := TurnUsage{TurnID: "turn-call-order-2", ConvID: conv.ID, AgentID: "alpha", Seq: nil, Outcome: 0}

	if _, err := svc.RecordTurn(ctx, nil, u, nil, false); err != nil {
		t.Fatalf("RecordTurn 应当成功，实际: %v", err)
	}

	got := strings.Join(spy.calls, " -> ")
	const want = "WriteTx:begin -> RecordUsage -> WriteTx:end"
	if got != want {
		t.Errorf("调用序列不对。\n实际: %s\n期望: %s", got, want)
	}
}

// TestRecordTurnCallOrderStopsAtParticipantCheck 覆盖：活跃成员判定失败时，
// RecordUsage 与 SetProviderSession/DropProviderSession 都不该被调用到——
// 「任一步失败整轮回滚」首先要求失败之后不再继续做后面几步。
func TestRecordTurnCallOrderStopsAtParticipantCheck(t *testing.T) {
	spy := newCallSpy()
	svc := NewService(spy)
	ctx := context.Background()

	conv, err := svc.Create(ctx, "判定失败调用顺序")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}

	spy.calls = nil
	msg := Message{
		ConvID: conv.ID, SenderID: "从没加入过", Kind: KindAgent, Text: "不该落库",
		SenderProvider: "claude", SenderModel: "opus",
	}
	seq := int64(1)
	u := TurnUsage{TurnID: "turn-call-order-3", ConvID: conv.ID, AgentID: "从没加入过", Seq: &seq, Outcome: 1}

	_, err = svc.RecordTurn(ctx, &msg, u, nil, false)
	if !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("errors.Is(err, ErrParticipantNotFound) 应当为真，实际: %v", err)
	}

	for _, c := range spy.calls {
		if c == "RecordUsage" || c == "SetProviderSession" || c == "DropProviderSession" {
			t.Errorf("活跃成员判定没过却仍然调用了 %s：%s", c, strings.Join(spy.calls, " -> "))
		}
	}
}

// TestRecordTurnValidation 覆盖事务外的那组校验：TurnID 为空、Seq 与 msg 是否为 nil
// 不匹配、以及与 Post 完全相同的四条（会话不存在、Kind 非法、SenderID 为空、
// 保留 sender 带脑子、KindAgent 缺脑子）。每条只破坏一处，其余合法
// （照 TestPostRejects 的写法），防的是「为了少写几行绕开 Post 的校验」。
func TestRecordTurnValidation(t *testing.T) {
	cases := []struct {
		name    string
		build   func(conv Conversation) (*Message, TurnUsage)
		wantErr error
	}{
		{
			name: "TurnID 为空串",
			build: func(conv Conversation) (*Message, TurnUsage) {
				return nil, TurnUsage{ConvID: conv.ID, AgentID: "alpha", Seq: nil}
			},
			wantErr: ErrMissingTurnID,
		},
		{
			name: "msg 非 nil 但 Seq 是 nil",
			build: func(conv Conversation) (*Message, TurnUsage) {
				m := Message{
					ConvID: conv.ID, SenderID: "alpha", Kind: KindAgent, Text: "缺 seq",
					SenderProvider: "claude", SenderModel: "opus",
				}
				return &m, TurnUsage{TurnID: "turn-validate-1", ConvID: conv.ID, AgentID: "alpha", Seq: nil}
			},
			wantErr: ErrTurnSeqMismatch,
		},
		{
			name: "msg 为 nil 但 Seq 非 nil",
			build: func(conv Conversation) (*Message, TurnUsage) {
				seq := int64(1)
				return nil, TurnUsage{TurnID: "turn-validate-2", ConvID: conv.ID, AgentID: "alpha", Seq: &seq}
			},
			wantErr: ErrTurnSeqMismatch,
		},
		{
			name: "会话不存在",
			build: func(conv Conversation) (*Message, TurnUsage) {
				m := Message{
					ConvID: "conv-does-not-exist", SenderID: "alpha", Kind: KindAgent, Text: "孤儿轮",
					SenderProvider: "claude", SenderModel: "opus",
				}
				seq := int64(1)
				return &m, TurnUsage{TurnID: "turn-validate-3", ConvID: "conv-does-not-exist", AgentID: "alpha", Seq: &seq}
			},
			wantErr: ErrConversationNotFound,
		},
		{
			name: "Kind 非法",
			build: func(conv Conversation) (*Message, TurnUsage) {
				m := Message{
					ConvID: conv.ID, SenderID: "alpha", Kind: MessageKind("bogus"), Text: "非法 kind",
					SenderProvider: "claude", SenderModel: "opus",
				}
				seq := int64(1)
				return &m, TurnUsage{TurnID: "turn-validate-4", ConvID: conv.ID, AgentID: "alpha", Seq: &seq}
			},
			wantErr: ErrInvalidKind,
		},
		{
			name: "SenderID 为空",
			build: func(conv Conversation) (*Message, TurnUsage) {
				m := Message{
					ConvID: conv.ID, SenderID: "", Kind: KindAgent, Text: "谁说的",
					SenderProvider: "claude", SenderModel: "opus",
				}
				seq := int64(1)
				return &m, TurnUsage{TurnID: "turn-validate-5", ConvID: conv.ID, AgentID: "alpha", Seq: &seq}
			},
			wantErr: ErrParticipantNotFound,
		},
		{
			name: "保留 sender 带脑子",
			build: func(conv Conversation) (*Message, TurnUsage) {
				m := Message{
					ConvID: conv.ID, SenderID: ReservedSenderUser, Kind: KindUser, Text: "人话混进了脑子",
					SenderProvider: "claude",
				}
				seq := int64(1)
				return &m, TurnUsage{TurnID: "turn-validate-6", ConvID: conv.ID, AgentID: "alpha", Seq: &seq}
			},
			wantErr: ErrInvalidSender,
		},
		{
			name: "KindAgent 缺 provider/model",
			build: func(conv Conversation) (*Message, TurnUsage) {
				m := Message{ConvID: conv.ID, SenderID: "alpha", Kind: KindAgent, Text: "没带脑子"}
				seq := int64(1)
				return &m, TurnUsage{TurnID: "turn-validate-7", ConvID: conv.ID, AgentID: "alpha", Seq: &seq}
			},
			wantErr: ErrMissingSenderBrain,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, f, conv := newServiceForTest(t)
			joinAgent(t, svc, conv.ID, "alpha")

			before := f.messageCount(conv.ID)
			msg, u := tc.build(conv)
			_, err := svc.RecordTurn(context.Background(), msg, u, nil, false)
			if err == nil {
				t.Fatal("RecordTurn 应当报错，实际返回 nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("errors.Is(err, %v) 应当为真，实际错误: %v", tc.wantErr, err)
			}
			if after := f.messageCount(conv.ID); after != before {
				t.Errorf("校验没过却写了消息：%d -> %d", before, after)
			}
		})
	}
}
