// record_turn_test.go 覆盖 Step 7 契约 10a §2.4 的 Service.RecordTurn：
// 四件事同一个 WriteTx、失败轮记账、Seq 覆盖、与 Post 同样严的校验、
// 整轮回滚、session=nil 不碰这一行（未决 #20 订正，原文是「走 Drop」）、
// 以及「本步不再往 cursors 表写游标」。
//
// 这里特意用真实的 *Store 而不是 internal/conversation 那份 fakeStore：
// fakeStore 给 Step 7 三个新方法配的只是空桩（不持久化、不冲突检测），
// 证明不了真实的事务原子性。四表一事务、失败整轮回滚，只有真库能验。
// 调用顺序层面的结构性验证见 internal/conversation/record_turn_test.go。
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"council/internal/conversation"
)

// tstAgentMsg 造一条带脑子的 KindAgent 消息，供 RecordTurn 的成功路径用。
func tstAgentMsg(convID, senderID, text string) conversation.Message {
	return conversation.Message{
		ConvID:         convID,
		SenderID:       senderID,
		Kind:           conversation.KindAgent,
		Text:           text,
		SenderProvider: "claude",
		SenderModel:    "opus",
	}
}

// tstMessageCount 数一个会话里的消息行数。
func tstMessageCount(t *testing.T, ctx context.Context, s *Store, convID string) int {
	t.Helper()
	got, err := s.Messages(ctx, convID, 0, 0)
	if err != nil {
		t.Fatalf("读消息失败: %v", err)
	}
	return len(got)
}

// tstJoinAlpha 把 "alpha" 加进会话，供只关心行为、不关心成员细节的用例复用。
func tstJoinAlpha(t *testing.T, ctx context.Context, s *Store, convID string) {
	t.Helper()
	tstJoin(t, ctx, s, conversation.Participant{
		ConvID: convID, AgentID: "alpha", Provider: "claude", Active: true,
		JoinedAt: time.Now().UTC().Add(-time.Hour).Truncate(time.Second),
	})
}

// TestRecordTurnSuccessWritesThreeTablesAndSkipsCursors 覆盖成功路径：
// messages、turn_usage、provider_sessions 各多一行且互相指得上；cursors 一行都不多
// （10a §1.1：游标已经搬进 provider_sessions.cursor_seq，两处都写就有了两个事实源）。
// 顺带验证 turn_usage.seq 以 store 实际分配的 seq 为准，不是调用方猜的值。
func TestRecordTurnSuccessWritesThreeTablesAndSkipsCursors(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "RecordTurn 成功路径")
	tstJoinAlpha(t, ctx, s, conv.ID)

	msg := tstAgentMsg(conv.ID, "alpha", "这一轮成功了")
	u := tstBaseUsage(conv.ID, "alpha", "turn-record-success-1")
	// 调用方故意猜错 Seq：RecordTurn 必须以 store 实际分配的 seq 为准，
	// 不能把调用方猜的这个 999 抄进 turn_usage（10a §2.4 的行内注释）。
	u.Seq = tstInt64Ptr(999)
	session := tstBaseSession(conv.ID, "alpha")

	stored, err := svc.RecordTurn(ctx, &msg, u, &session, false)
	if err != nil {
		t.Fatalf("RecordTurn 失败: %v", err)
	}
	if stored == nil {
		t.Fatal("成功路径返回的消息是 nil")
	}
	if stored.Seq != 1 {
		t.Fatalf("返回的消息 Seq = %d，期望 1（会话里第一条）", stored.Seq)
	}

	t.Run("messages 多了一行", func(t *testing.T) {
		if n := tstMessageCount(t, ctx, s, conv.ID); n != 1 {
			t.Errorf("messages 有 %d 行，期望 1 行", n)
		}
	})

	t.Run("turn_usage 多了一行且 seq 是 store 分配的那个，不是调用方猜的 999", func(t *testing.T) {
		raw := tstRawDB(t, s.Path())
		got := tstScanUsageRow(t, raw, u.TurnID)
		if !got.seq.Valid || got.seq.Int64 != stored.Seq {
			t.Errorf("turn_usage.seq 读回 %+v，期望有效值 %d", got.seq, stored.Seq)
		}
		if got.seq.Valid && got.seq.Int64 == 999 {
			t.Error("turn_usage.seq 是调用方猜的 999，说明 RecordTurn 没有用真正落库的 seq 覆盖它")
		}
	})

	t.Run("provider_sessions 多了一行且字段与传入的一致", func(t *testing.T) {
		got, ok, err := s.ProviderSession(ctx, conv.ID, "alpha")
		if err != nil || !ok {
			t.Fatalf("ProviderSession 返回 (%+v, %v, %v)", got, ok, err)
		}
		if got.ProviderSessionID != session.ProviderSessionID || got.CursorSeq != session.CursorSeq {
			t.Errorf("provider_sessions 读回 %+v，期望与传入的 %+v 一致", got, session)
		}
	})

	t.Run("cursors 表里这个 (conv, agent) 仍然没有行", func(t *testing.T) {
		_, err := s.Cursor(ctx, conv.ID, "alpha")
		if !errors.Is(err, conversation.ErrNoCursor) {
			t.Errorf("Cursor 返回 %v，期望能被 errors.Is 认出 ErrNoCursor（RecordTurn 不该写 cursors 表）", err)
		}
	})
}

// TestRecordTurnFailedTurnRecordsUsageWithNilSeq 覆盖失败轮：
// msg 为 nil 时 messages 不增行，turn_usage 仍然多一行且 seq 为 NULL。
func TestRecordTurnFailedTurnRecordsUsageWithNilSeq(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "RecordTurn 失败路径")

	before := tstMessageCount(t, ctx, s, conv.ID)
	u := tstBaseUsage(conv.ID, "alpha", "turn-record-failed-1")
	u.Seq = nil
	u.Outcome = 0

	stored, err := svc.RecordTurn(ctx, nil, u, nil, false)
	if err != nil {
		t.Fatalf("失败轮 RecordTurn 本身应当成功（记账不是发言），实际: %v", err)
	}
	if stored != nil {
		t.Errorf("msg 为 nil 时返回的消息应当是 nil，实际 %+v", stored)
	}
	if after := tstMessageCount(t, ctx, s, conv.ID); after != before {
		t.Errorf("失败轮之后 messages 从 %d 变成了 %d，期望不变", before, after)
	}

	raw := tstRawDB(t, s.Path())
	got := tstScanUsageRow(t, raw, u.TurnID)
	if got.seq.Valid {
		t.Errorf("失败轮 turn_usage.seq 读回有效值 %d，期望 NULL", got.seq.Int64)
	}
}

// TestRecordTurnValidationMirrorsPost 覆盖「与 Post 同样严」的那组校验，
// 逐条只破坏一处，其余合法，免得断言被别的校验抢先命中（照 TestPostRejects 的写法）。
// 这组防的是「为了少写几行绕开 Post 的校验」。
func TestRecordTurnValidationMirrorsPost(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "RecordTurn 校验")
	tstJoinAlpha(t, ctx, s, conv.ID)

	cases := []struct {
		name    string
		msg     *conversation.Message
		u       conversation.TurnUsage
		wantErr error
	}{
		{
			name: "TurnID 为空串",
			msg:  nil,
			u: func() conversation.TurnUsage {
				u := tstBaseUsage(conv.ID, "alpha", "")
				u.Seq = nil
				return u
			}(),
			wantErr: conversation.ErrMissingTurnID,
		},
		{
			name: "msg 非 nil 但 Seq 是 nil",
			msg: func() *conversation.Message {
				m := tstAgentMsg(conv.ID, "alpha", "seq 不对")
				return &m
			}(),
			u: func() conversation.TurnUsage {
				u := tstBaseUsage(conv.ID, "alpha", "turn-validate-seq-mismatch-1")
				u.Seq = nil
				return u
			}(),
			wantErr: conversation.ErrTurnSeqMismatch,
		},
		{
			name:    "msg 为 nil 但 Seq 非 nil",
			msg:     nil,
			u:       tstBaseUsage(conv.ID, "alpha", "turn-validate-seq-mismatch-2"), // 默认 Seq 非 nil
			wantErr: conversation.ErrTurnSeqMismatch,
		},
		{
			name: "会话不存在",
			msg: func() *conversation.Message {
				m := tstAgentMsg("conv-does-not-exist", "alpha", "孤儿轮")
				return &m
			}(),
			u:       tstBaseUsage(conv.ID, "alpha", "turn-validate-no-conv-1"),
			wantErr: conversation.ErrConversationNotFound,
		},
		{
			name: "Kind 非法",
			msg: func() *conversation.Message {
				m := tstAgentMsg(conv.ID, "alpha", "非法 kind")
				m.Kind = conversation.MessageKind("bogus")
				return &m
			}(),
			u:       tstBaseUsage(conv.ID, "alpha", "turn-validate-bad-kind-1"),
			wantErr: conversation.ErrInvalidKind,
		},
		{
			name: "SenderID 为空",
			msg: func() *conversation.Message {
				m := tstAgentMsg(conv.ID, "", "谁说的")
				return &m
			}(),
			u:       tstBaseUsage(conv.ID, "alpha", "turn-validate-empty-sender-1"),
			wantErr: conversation.ErrParticipantNotFound,
		},
		{
			name: "保留 sender 带脑子",
			msg: func() *conversation.Message {
				m := tstUserMsg(conv.ID, "人话里混进了脑子")
				m.SenderProvider = "claude"
				return &m
			}(),
			u:       tstBaseUsage(conv.ID, "alpha", "turn-validate-reserved-brain-1"),
			wantErr: conversation.ErrInvalidSender,
		},
		{
			name: "KindAgent 缺 provider/model",
			msg: func() *conversation.Message {
				return &conversation.Message{
					ConvID: conv.ID, SenderID: "alpha", Kind: conversation.KindAgent, Text: "没带脑子",
				}
			}(),
			u:       tstBaseUsage(conv.ID, "alpha", "turn-validate-missing-brain-1"),
			wantErr: conversation.ErrMissingSenderBrain,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeMsgs := tstMessageCount(t, ctx, s, conv.ID)
			_, err := svc.RecordTurn(ctx, tc.msg, tc.u, nil, false)
			if err == nil {
				t.Fatal("RecordTurn 应当报错，实际返回 nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("errors.Is(err, %v) 应当为真，实际错误: %v", tc.wantErr, err)
			}
			if after := tstMessageCount(t, ctx, s, conv.ID); after != beforeMsgs {
				t.Errorf("校验不过却写了消息：%d -> %d", beforeMsgs, after)
			}
		})
	}
}

// TestRecordTurnRollsBackWhenParticipantCheckFails 覆盖整轮回滚的第一种触发方式：
// 让 AppendMessage 必然失败（/leave 之后事务内的 RequireActiveParticipant 会拒），
// 断言 turn_usage 与 provider_sessions 都没有新增行——四件事同一个事务，
// 任一步失败整轮回滚。
func TestRecordTurnRollsBackWhenParticipantCheckFails(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "RecordTurn /leave 之后整轮回滚")
	tstJoinAlpha(t, ctx, s, conv.ID)
	tstLeave(t, ctx, s, conv.ID, "alpha", time.Now().UTC())

	msg := tstAgentMsg(conv.ID, "alpha", "退出之后还想发言")
	u := tstBaseUsage(conv.ID, "alpha", "turn-record-leave-1")
	session := tstBaseSession(conv.ID, "alpha")

	_, err := svc.RecordTurn(ctx, &msg, u, &session, false)
	if !errors.Is(err, conversation.ErrParticipantNotFound) {
		t.Fatalf("errors.Is(err, ErrParticipantNotFound) 应当为真，实际: %v", err)
	}

	if n := tstMessageCount(t, ctx, s, conv.ID); n != 0 {
		t.Errorf("messages 有 %d 行，期望 0 行（整轮回滚）", n)
	}
	raw := tstRawDB(t, s.Path())
	if n := tstUsageRowCount(t, raw, u.TurnID); n != 0 {
		t.Errorf("turn_usage 有 %d 行，期望 0 行（整轮回滚）", n)
	}
	if _, ok, err := s.ProviderSession(ctx, conv.ID, "alpha"); err != nil || ok {
		t.Errorf("provider_sessions 应当没有行，实际 ok=%v err=%v", ok, err)
	}
}

// TestRecordTurnDuplicateTurnIDRollsBackWholeTurn 覆盖整轮回滚的第二种、也是更硬的
// 一种触发方式：AppendMessage 先成功分配了 seq，RecordUsage 才因为 turn_id 冲突失败。
// 必须证明连刚插进去的那条消息也被撤销——不能出现「消息留下、账没记上」的半吊子状态，
// 也不能让回滚掉的那次白白占用一个 seq 号。
func TestRecordTurnDuplicateTurnIDRollsBackWholeTurn(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "重复 turn_id 拖累整轮回滚")
	tstJoinAlpha(t, ctx, s, conv.ID)

	const turnID = "turn-record-dup-1"
	firstMsg := tstAgentMsg(conv.ID, "alpha", "第一次，应当成功")
	firstUsage := tstBaseUsage(conv.ID, "alpha", turnID)
	if _, err := svc.RecordTurn(ctx, &firstMsg, firstUsage, nil, false); err != nil {
		t.Fatalf("第一次 RecordTurn 应当成功，实际: %v", err)
	}
	if n := tstMessageCount(t, ctx, s, conv.ID); n != 1 {
		t.Fatalf("第一次之后 messages 有 %d 行，期望 1 行", n)
	}

	secondMsg := tstAgentMsg(conv.ID, "alpha", "第二次，用了同一个 turn_id，应当整轮回滚")
	secondUsage := tstBaseUsage(conv.ID, "alpha", turnID)
	_, err := svc.RecordTurn(ctx, &secondMsg, secondUsage, nil, false)
	if err == nil {
		t.Fatal("重复 turn_id 的第二次 RecordTurn 竟然成功了")
	}

	if n := tstMessageCount(t, ctx, s, conv.ID); n != 1 {
		t.Errorf("第二次失败之后 messages 有 %d 行，期望仍是 1 行（它的 AppendMessage 必须跟着回滚）", n)
	}
	raw := tstRawDB(t, s.Path())
	if n := tstUsageRowCount(t, raw, turnID); n != 1 {
		t.Errorf("turn_usage 里 turn_id=%q 有 %d 行，期望仍是 1 行", turnID, n)
	}

	// 回滚之后 seq 计数器不该被烧掉：下一条真消息应当拿到 2，而不是 3。
	thirdMsg := tstAgentMsg(conv.ID, "alpha", "第三条，验证 seq 没被回滚的那次白白占掉")
	stored, err := svc.RecordTurn(ctx, &thirdMsg, tstBaseUsage(conv.ID, "alpha", "turn-record-dup-2"), nil, false)
	if err != nil {
		t.Fatalf("第三次 RecordTurn 应当成功，实际: %v", err)
	}
	if stored.Seq != 2 {
		t.Errorf("第三条消息的 Seq = %d，期望 2（回滚的那次不该占用 seq）", stored.Seq)
	}
}

// TestRecordTurnSessionNilLeavesProviderSessionUntouched 覆盖 session 为 nil
// 的分支：先建立一条会话状态，再用 session=nil 调一次，断言那条会话状态原样
// 还在，不受影响。
//
// 本用例此前名为 TestRecordTurnSessionNilDropsProviderSession，断言的是相反的
// 结果（session=nil 之后这一行应当被删掉）——那是 RecordTurn 冻结契约的旧
// 语义，被未决 #20 坐实为一个真实 bug：起进程之前就拒绝的那几支传下去的
// session 本来就是 nil，只是「这一轮没碰过会话状态」，与「决定作废」无关，
// 却被旧实现无条件删掉，导致一次与会话无关的拒绝抹掉一条已经花过钱的会话。
// 这条断言因此要反过来：session=nil 现在必须是「不碰」，不是「删」。
//
// 本文件在 internal/store 目录下，#20 的任务范围原本不包含这个目录；但这条
// 断言直接编码了 #20 要移除的那个行为，不改的话 go test ./... 过不了全绿，
// 所以按测试专用的最小例外处理：只改了这一个测试函数的断言与注释，没有碰
// internal/store 的任何实现代码（DropProviderSession 本身、事务、schema 都
// 原样未动）。
func TestRecordTurnSessionNilLeavesProviderSessionUntouched(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "session=nil 不碰这一行")
	tstJoinAlpha(t, ctx, s, conv.ID)

	firstMsg := tstAgentMsg(conv.ID, "alpha", "先建立一条会话状态")
	firstSession := tstBaseSession(conv.ID, "alpha")
	if _, err := svc.RecordTurn(ctx, &firstMsg, tstBaseUsage(conv.ID, "alpha", "turn-drop-1"), &firstSession, false); err != nil {
		t.Fatalf("第一次 RecordTurn 失败: %v", err)
	}
	before, ok, err := s.ProviderSession(ctx, conv.ID, "alpha")
	if err != nil || !ok {
		t.Fatalf("第一次之后应当读得到 provider session，实际 ok=%v err=%v", ok, err)
	}

	// 第二轮 session 传 nil：这里用失败轮（msg=nil）演示，同时顺带验证失败轮
	// 也能正常带着 session=nil 一起提交，不影响用量记账。
	u := tstBaseUsage(conv.ID, "alpha", "turn-drop-2")
	u.Seq = nil
	if _, err := svc.RecordTurn(ctx, nil, u, nil, false); err != nil {
		t.Fatalf("第二次 RecordTurn 失败: %v", err)
	}

	after, ok, err := s.ProviderSession(ctx, conv.ID, "alpha")
	if err != nil || !ok {
		t.Fatalf("session=nil 之后这一行应当原样还在，实际 ok=%v err=%v", ok, err)
	}
	if after.ProviderSessionID != before.ProviderSessionID || after.CursorSeq != before.CursorSeq {
		t.Errorf("session=nil 之后这一行的内容变了：之前 %+v，之后 %+v", before, after)
	}
}
