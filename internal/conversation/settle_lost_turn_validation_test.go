package conversation

import (
	"context"
	"errors"
	"testing"
)

// 本文件补 9e85bea 返修轮里给 Service.SettleLostTurn 加的三样校验：它们是
// 评审来回时补的，不在冻结契约的验收清单里，落地时也没有配对的测试腿。
// 三样都在 service.go 的 SettleLostTurn 里、都在进 WriteTx 之前：
//
//  1. msg.SenderID == "" 拒绝，返回包了 ErrParticipantNotFound 的错误
//     （Tx.AppendMessage 不查 sender，messages.sender_id 是 TEXT NOT NULL，
//     空串过得了这一列）。
//  2. 保留 sender 带脑子拒绝：isReservedSender(msg.SenderID) &&
//     !msg.SenderBrainEmpty() 时返回包了 ErrInvalidSender 的错误
//     （messages 上的 CHECK 只约束 'user'，管不到 'system'，SQL 一层兜不住）。
//  3. applyThoughtPolicy 已接上：msg != nil 时先取一份处理过的副本再往下用。
//
// 判别力的核心与 369f400 的 inflight_validation_test.go 一致：前两条校验
// 若真的挡在事务之前，fakeStore.writeTxCalls 在被拒绝的调用前后应当不变，
// 且没有消息落进 f.msgs——只断言「返回了错误」测不出「照常开事务、让底层
// 报错」这种坏实现。第三条校验判据是落库的那一行本身，SettleLostTurn 的
// 返回值 LostSettlement 不含消息内容，只能直接查 fakeStore.msgs。
//
// SettleLostTurn 与 Post / RecordTurn 不同：它不查 Conversation()、不查
// msg.Kind.Valid()、也不过 RequireActiveParticipant——本文件的用例只摆
// 这三样校验各自要求的最小合法输入，其余字段能省则省，免得断言被别的
// 校验抢先命中。u 与 session 全部传 nil：SettleLostTurn 的三处身份比对
// （msg.ConvID vs u.ConvID 等）都以 "u != nil" / "session != nil" 为前提，
// 传 nil 就绕开了与本文件无关的那组校验。

// TestSettleLostTurnRejectsEmptySenderID 断言 msg.SenderID 为空时
// SettleLostTurn 在进写事务之前就拒绝：errors.Is 命中 ErrParticipantNotFound，
// 写事务没被打开，也没有消息落进 fakeStore。
func TestSettleLostTurnRejectsEmptySenderID(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	beforeTx := f.writeTxCalls
	beforeMsgs := f.messageCount(conv.ID)

	msg := &Message{
		ConvID:   conv.ID,
		SenderID: "",
		Kind:     KindSystem,
		Text:     "空 sender 的对账面包屑",
	}
	_, err := svc.SettleLostTurn(context.Background(), "turn-empty-sender", msg, nil, nil)
	if !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("errors.Is(err, ErrParticipantNotFound) 应当为真，实际: %v", err)
	}
	if f.writeTxCalls != beforeTx {
		t.Errorf("空 SenderID 校验没过时不应当开写事务，writeTxCalls 从 %d 变成了 %d", beforeTx, f.writeTxCalls)
	}
	if after := f.messageCount(conv.ID); after != beforeMsgs {
		t.Errorf("空 SenderID 校验没过时不应当写入消息，条数从 %d 变成了 %d", beforeMsgs, after)
	}
}

// TestSettleLostTurnAcceptsNonReservedSenderWithBrain 是两重对照组：
//
//  1. 对 TestSettleLostTurnRejectsEmptySenderID 的正例——非空 SenderID 不该
//     被那道闸拦下，确实走到了 WriteTx，消息也落了库。少这条，「空 SenderID
//     被拒」那条用例配一个「无论如何都报错」的坏实现也会绿。
//  2. 对保留 sender 带脑子那道闸的加固：SenderID 是普通 agent（非保留），
//     却带着 SenderProvider/SenderModel，如果 isReservedSender(...) 那半个
//     判断被误删或误改（变成不分是否保留、只要带脑子就拒），这条用例会先红，
//     比只测「system 带脑子拒 / system 不带脑子放行」这一对更早抓住那类变异。
func TestSettleLostTurnAcceptsNonReservedSenderWithBrain(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	beforeTx := f.writeTxCalls

	msg := &Message{
		ConvID:         conv.ID,
		SenderID:       "alpha",
		Kind:           KindAgent,
		Text:           "非保留 sender 带脑子的对账面包屑",
		SenderProvider: "claude",
		SenderModel:    "opus",
	}
	_, err := svc.SettleLostTurn(context.Background(), "turn-non-reserved-with-brain", msg, nil, nil)
	if err != nil {
		t.Fatalf("SettleLostTurn 应当成功，实际: %v", err)
	}
	if f.writeTxCalls != beforeTx+1 {
		t.Errorf("合法输入应当走一次 WriteTx，writeTxCalls 从 %d 变成了 %d", beforeTx, f.writeTxCalls)
	}
	stored := f.msgs[conv.ID]
	if len(stored) != 1 {
		t.Fatalf("应当落库 1 条消息，实际 %d 条: %+v", len(stored), stored)
	}
	if stored[0].SenderID != "alpha" {
		t.Errorf("落库消息 SenderID = %q，应当是 %q", stored[0].SenderID, "alpha")
	}
}

// TestSettleLostTurnRejectsReservedSenderWithBrain 断言保留 sender
// （ReservedSenderSystem）带着非空 provider/model/effort 时，SettleLostTurn
// 在进写事务之前就拒绝：errors.Is 命中 ErrInvalidSender，写事务没被打开，
// 也没有消息落进 fakeStore。
//
// 用 ReservedSenderSystem 而不是 ReservedSenderUser：messages 表上的 CHECK
// 只约束 'user'，'system' 这一半没有 SQL 兜底，全靠这一层 Go 代码——本条
// 用例专盯没有兜底的那一半，与 reserved_sender_test.go 里 Post/RecordTurn
// 的两条同体例。
func TestSettleLostTurnRejectsReservedSenderWithBrain(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	beforeTx := f.writeTxCalls
	beforeMsgs := f.messageCount(conv.ID)

	msg := &Message{
		ConvID:         conv.ID,
		SenderID:       ReservedSenderSystem,
		Kind:           KindSystem,
		Text:           "带脑子的 system 面包屑",
		SenderProvider: "claude",
		SenderModel:    "opus",
		SenderEffort:   "high",
	}
	_, err := svc.SettleLostTurn(context.Background(), "turn-reserved-with-brain", msg, nil, nil)
	if !errors.Is(err, ErrInvalidSender) {
		t.Fatalf("errors.Is(err, ErrInvalidSender) 应当为真，实际: %v", err)
	}
	if f.writeTxCalls != beforeTx {
		t.Errorf("带脑子的保留 sender 校验没过时不应当开写事务，writeTxCalls 从 %d 变成了 %d", beforeTx, f.writeTxCalls)
	}
	if after := f.messageCount(conv.ID); after != beforeMsgs {
		t.Errorf("带脑子的保留 sender 校验没过时不应当写入消息，条数从 %d 变成了 %d", beforeMsgs, after)
	}
}

// TestSettleLostTurnAcceptsReservedSenderWithoutBrain 与上一条成对：
// ReservedSenderSystem 不带脑子时必须放行，确实走到了 WriteTx、消息也落了库。
// 少这一条，「system 带脑子必拒」那条用例配一个「无论如何都拒绝 system」的
// 坏实现也会绿——这正是任务书点名要求的那一对。
func TestSettleLostTurnAcceptsReservedSenderWithoutBrain(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	beforeTx := f.writeTxCalls

	msg := &Message{
		ConvID:   conv.ID,
		SenderID: ReservedSenderSystem,
		Kind:     KindSystem,
		Text:     "不带脑子的 system 面包屑",
	}
	_, err := svc.SettleLostTurn(context.Background(), "turn-reserved-without-brain", msg, nil, nil)
	if err != nil {
		t.Fatalf("SettleLostTurn 应当成功，实际: %v", err)
	}
	if f.writeTxCalls != beforeTx+1 {
		t.Errorf("合法输入应当走一次 WriteTx，writeTxCalls 从 %d 变成了 %d", beforeTx, f.writeTxCalls)
	}
	stored := f.msgs[conv.ID]
	if len(stored) != 1 {
		t.Fatalf("应当落库 1 条消息，实际 %d 条: %+v", len(stored), stored)
	}
	if stored[0].SenderID != ReservedSenderSystem {
		t.Errorf("落库消息 SenderID = %q，应当是 %q", stored[0].SenderID, ReservedSenderSystem)
	}
}

// TestSettleLostTurnDropsThoughtWhenPolicyOff 覆盖 applyThoughtPolicy 已接上
// SettleLostTurn 这件事的丢弃那一半。判据是真正落库的那一行（查
// fakeStore.msgs，不是 SettleLostTurn 的返回值——LostSettlement 不含消息
// 内容），而不是只看函数有没有报错：
//
//   - 落库消息的 Thought 必须是空串，Meta[MetaThoughtDropped] 必须是 true；
//   - 调用方手里那份 *Message 必须原样未动——这是真判据：一个直接改
//     *msg 而不是取副本的坏实现会让这条断言变红。用非 nil 且带一个已有
//     键的 Meta（不用 nil）：Meta 为 nil 时 applyThoughtPolicy 会新建一个
//     map，就地改与取副本在「调用方的 nil 变成了非 nil」这件事上长得一样，
//     测不出区别；非 nil 的 Meta 才能让「调用方的 map 内容被悄悄污染」
//     这件事可能被观察到。
func TestSettleLostTurnDropsThoughtWhenPolicyOff(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	svc.SetThoughtPolicy(false)

	callerMeta := map[string]any{"existing_key": "existing_value"}
	msg := &Message{
		ConvID:   conv.ID,
		SenderID: ReservedSenderSystem,
		Kind:     KindSystem,
		Text:     "带 thought 的对账面包屑",
		Thought:  testThoughtText,
		Meta:     callerMeta,
	}

	_, err := svc.SettleLostTurn(context.Background(), "turn-thought-off", msg, nil, nil)
	if err != nil {
		t.Fatalf("SettleLostTurn 应当成功，实际: %v", err)
	}

	stored := f.msgs[conv.ID]
	if len(stored) != 1 {
		t.Fatalf("应当落库 1 条消息，实际 %d 条: %+v", len(stored), stored)
	}
	if stored[0].Thought != "" {
		t.Errorf("落库消息 Thought = %q，关掉存储开关后期望空串", stored[0].Thought)
	}
	if stored[0].Meta[MetaThoughtDropped] != true {
		t.Errorf("落库消息 Meta[%q] = %v，期望 true", MetaThoughtDropped, stored[0].Meta[MetaThoughtDropped])
	}
	if stored[0].Meta["existing_key"] != "existing_value" {
		t.Errorf("落库消息 Meta 应当保留调用方原有的键，实际: %+v", stored[0].Meta)
	}

	// 调用方（本测试函数）手里的 msg 必须原样未动。
	if msg.Thought != testThoughtText {
		t.Errorf("调用方的 msg.Thought 被 SettleLostTurn 改动了，现在是 %q，期望仍是 %q", msg.Thought, testThoughtText)
	}
	if len(callerMeta) != 1 {
		t.Fatalf("调用方原始 Meta 被改动了，现在有 %d 个键: %+v", len(callerMeta), callerMeta)
	}
	if _, ok := callerMeta[MetaThoughtDropped]; ok {
		t.Errorf("调用方原始 Meta 不该被写入 %q，实际: %+v", MetaThoughtDropped, callerMeta)
	}
	// 与上面几条各自拦不同的坏实现：callerMeta 这个变量在「就地改 *msg」下也
	// 会保持不变（它本来就是旧 map，取副本还是就地改，旧变量指向的内容都不会
	// 变），真正会被「就地改 *msg」坑到的是调用方经指针 msg.Meta 看到的内容——
	// 这里再经 msg.Meta（而不是 callerMeta 这个局部变量）复核一遍。
	if _, ok := msg.Meta[MetaThoughtDropped]; ok {
		t.Errorf("调用方经 msg.Meta 看到的内容不该被写入 %q，实际: %+v", MetaThoughtDropped, msg.Meta)
	}
}

// TestSettleLostTurnKeepsThoughtWhenPolicyOn 与上一条成对：显式
// SetThoughtPolicy(true)（存）时，非空 Thought 要原样落库，Meta 里不出现
// MetaThoughtDropped。只测丢弃那一半的话，一个「无论如何都清空 Thought」
// 的坏实现也会绿。
func TestSettleLostTurnKeepsThoughtWhenPolicyOn(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	svc.SetThoughtPolicy(true)

	msg := &Message{
		ConvID:   conv.ID,
		SenderID: ReservedSenderSystem,
		Kind:     KindSystem,
		Text:     "带 thought 的对账面包屑",
		Thought:  testThoughtText,
	}

	_, err := svc.SettleLostTurn(context.Background(), "turn-thought-on", msg, nil, nil)
	if err != nil {
		t.Fatalf("SettleLostTurn 应当成功，实际: %v", err)
	}

	stored := f.msgs[conv.ID]
	if len(stored) != 1 {
		t.Fatalf("应当落库 1 条消息，实际 %d 条: %+v", len(stored), stored)
	}
	if stored[0].Thought != testThoughtText {
		t.Errorf("落库消息 Thought = %q，开启存储策略后期望原样存住 %q", stored[0].Thought, testThoughtText)
	}
	if _, ok := stored[0].Meta[MetaThoughtDropped]; ok {
		t.Errorf("开启存储策略后 Meta 不该出现 %q，实际 Meta = %+v", MetaThoughtDropped, stored[0].Meta)
	}
}
