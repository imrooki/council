package conversation

// 本文件是 Step 10 C 片的 C3：docs/00_DECISIONS.md 未决 #14 在关闭时承诺、却从未
// 兑现的两条负向用例。现查过（grep -rn "ReservedSenderSystem" --include=*_test.go
// internal/conversation/），关闭前这两条用例零命中——那条未决是在一个没兑现的
// 承诺上关掉的，本文件把承诺补上。
//
// 判据坐标是本次任务书现查出来的（不是抄未决 #14 原文——那两个坐标已经过期，
// 这里按内容定位，不钉行号，免得再重蹈覆辙）：Post 内与 RecordTurn 内
// 各有一个独立的
//
//	if isReservedSender(...) && !....SenderBrainEmpty() {
//		return ..., fmt.Errorf(..., ErrInvalidSender)
//	}
//
// 两处判据是各自独立的 if，出自两个不同的函数，不共享同一次求值：漏改其中一处
// 会让保留 sender 从那一条路悄悄带着脑子列落库，而另一条路的用例全绿掩盖不了这个
// 缺口。这正是本文件下面两条用例、以及各自独立做一次变异验证的理由。

import (
	"context"
	"errors"
	"testing"
)

// TestPostRejectsReservedSystemSenderWithBrain 钉 Post 里那道闸：
// SenderID 是保留 sender ReservedSenderSystem，却带着非空的 provider/model/effort，
// Post 必须拒绝，且返回的错误能被 errors.Is 认出 ErrInvalidSender。
//
// 为什么必须是 ReservedSenderSystem 而不是 ReservedSenderUser：messages 表上的
// CHECK 约束（internal/store/schema.go）只管到 'user' 这一半，'system' 这一半
// 没有结构兜底，全靠这一层 Go 代码——这条用例专门盯没有兜底的那一半。
func TestPostRejectsReservedSystemSenderWithBrain(t *testing.T) {
	svc, _, conv := newServiceForTest(t)

	m := Message{
		ConvID:         conv.ID,
		SenderID:       ReservedSenderSystem,
		Kind:           KindSystem,
		SenderProvider: "claude",
		SenderModel:    "opus",
		SenderEffort:   "high",
	}

	_, err := svc.Post(context.Background(), m)
	if err == nil {
		t.Fatalf("Post 应当拒绝带脑子的保留 sender %q，实际返回 nil error", ReservedSenderSystem)
	}
	if !errors.Is(err, ErrInvalidSender) {
		t.Fatalf("Post 返回的错误应能被 errors.Is 认出 ErrInvalidSender，实际: %v", err)
	}
}

// TestRecordTurnRejectsReservedSystemSenderWithBrain 钉 RecordTurn 里
// 那道独立的闸。与 Post 的那道闸是两个不同函数里的两个独立 if，各自求值——
// 只验过其中一个不能证明另一个也在，这正是本次任务书要求"两处各自独立验"的理由。
func TestRecordTurnRejectsReservedSystemSenderWithBrain(t *testing.T) {
	svc, _, conv := newServiceForTest(t)

	// RecordTurn 一进来还有两道更早的前置校验：u.TurnID 不能为空、且
	// (msg == nil) 与 (u.Seq == nil) 必须同真同假。这两道都不是本用例要
	// 测的东西，只是为了绕过去、把执行真正带到目标判据那一行，所以给足
	// 合法值：非空 TurnID，msg 非 nil 就配一个非 nil 的 Seq。
	seq := int64(1)
	msg := &Message{
		ConvID:         conv.ID,
		SenderID:       ReservedSenderSystem,
		Kind:           KindSystem,
		SenderProvider: "claude",
		SenderModel:    "opus",
		SenderEffort:   "high",
	}
	u := TurnUsage{
		TurnID:  "test-turn-c3-record-turn",
		ConvID:  conv.ID,
		AgentID: "irrelevant-agent-id", // 走到目标判据之前不看这个字段
		Seq:     &seq,
	}

	_, err := svc.RecordTurn(context.Background(), msg, u, nil, false)
	if err == nil {
		t.Fatalf("RecordTurn 应当拒绝带脑子的保留 sender %q，实际返回 nil error", ReservedSenderSystem)
	}
	if !errors.Is(err, ErrInvalidSender) {
		t.Fatalf("RecordTurn 返回的错误应能被 errors.Is 认出 ErrInvalidSender，实际: %v", err)
	}
}
