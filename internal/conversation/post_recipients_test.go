package conversation

// post_recipients_test.go 覆盖 docs/11d_STEP8C_CONTRACT.md 第 2.6 节与
// 第 2.8 节第 8 条：Service.Post 在事务外新增的 Recipients 存在性校验——
// 复用既有的 Service.Recipients（已有 5 条用例覆盖，参见本包
// service_test.go 的 TestRecipientsUnknownAgent 等），契约原话“零契约
// 新增”。
//
// 背对背纪律：Post 加这段校验是本次 8c-1 正在改的代码。写这份测试时读过
// 的 service.go 现状（141-208 行）里 Post 完全不检查 Recipients 是否
// 存在，这与契约“此刻要么不存在、要么是半成品”的预期一致——本文件不是
// 从任何半成品实现里抄断言，全部来自 2.6 节文字：校验点在 Kind/SenderID/
// 脑子三列那几条既有校验之后、事务外，直接复用 s.Recipients。这条用例
// 现在大概率是红的，实现落地后才会绿。

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestPostRejectsUnknownRecipient 覆盖 2.8 第 8 条：Recipients=["ghost"]
// 被拒、不落库（前后 MaxSeq 不变），错误里点名 ghost 且可被
// errors.Is(err, ErrParticipantNotFound) 认出。
//
// 用 KindUser + ReservedSenderUser：这条消息不需要 join 任何 agent 就能
// 先过 Kind / SenderID / 脑子三列那几条既有校验（2.6 节：新校验加在它们
// 之后），把 Recipients 存在性这一条单独隔离出来测，不与其它校验分支
// 混在一起断言。
func TestPostRejectsUnknownRecipient(t *testing.T) {
	svc, _, conv := newServiceForTest(t)

	before, err := svc.MaxSeq(context.Background(), conv.ID)
	if err != nil {
		t.Fatalf("Post 之前读 MaxSeq 失败: %v", err)
	}

	got, err := svc.Post(context.Background(), Message{
		ConvID:     conv.ID,
		SenderID:   ReservedSenderUser,
		Kind:       KindUser,
		Text:       "只给 ghost 看",
		Recipients: []string{"ghost"},
	})
	if err == nil {
		t.Fatalf("Post 应当因为 Recipients 里的 ghost 不存在而报错，实际返回消息 %+v", got)
	}
	if !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("errors.Is(err, ErrParticipantNotFound) 应当为真，实际错误: %v", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("错误文本应当点名 ghost，实际: %v", err)
	}

	after, err := svc.MaxSeq(context.Background(), conv.ID)
	if err != nil {
		t.Fatalf("Post 之后读 MaxSeq 失败: %v", err)
	}
	if after != before {
		t.Errorf("被拒的 Post 不应当改变 MaxSeq，之前 %d，之后 %d", before, after)
	}
}

// TestPostRejectsUnknownRecipientAmongKnown 补一条更贴近真实 /to 用法的例子：
// Recipients 里混了一个存在的 participant（alpha）与一个不存在的
// （ghost）。既有的 Service.Recipients 契约（本包 service_test.go 已有
// 5 条用例覆盖）是“有一个不是就整体拒绝”，这里断言 Post 复用同一个方法
// 之后继承了同样的“部分非法即整体拒绝”，而不是静默丢掉不合法的那个 id、
// 只把 alpha 存进去。
func TestPostRejectsUnknownRecipientAmongKnown(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	joinAgent(t, svc, conv.ID, "alpha")

	before := f.messageCount(conv.ID)
	_, err := svc.Post(context.Background(), Message{
		ConvID:     conv.ID,
		SenderID:   ReservedSenderUser,
		Kind:       KindUser,
		Text:       "给 alpha 和 ghost",
		Recipients: []string{"alpha", "ghost"},
	})
	if !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("errors.Is(err, ErrParticipantNotFound) 应当为真，实际错误: %v", err)
	}
	if after := f.messageCount(conv.ID); after != before {
		t.Errorf("被拒的 Post 不应当写入消息，条数从 %d 变成了 %d", before, after)
	}
}

// TestPostAcceptsKnownRecipients 是上面两条“拒绝”用例的正例对照：
// Recipients 全部存在时，Post 必须照常成功——否则没法分清“校验太严，
// 把合法输入也拦了”与“校验没加”这两种红。
func TestPostAcceptsKnownRecipients(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	joinAgent(t, svc, conv.ID, "alpha")
	joinAgent(t, svc, conv.ID, "mike")

	got, err := svc.Post(context.Background(), Message{
		ConvID:     conv.ID,
		SenderID:   ReservedSenderUser,
		Kind:       KindUser,
		Text:       "给 alpha 和 mike",
		Recipients: []string{"alpha", "mike"},
	})
	if err != nil {
		t.Fatalf("Recipients 全部存在时 Post 应当成功，实际: %v", err)
	}
	if got.Seq < 1 {
		t.Errorf("返回的 Seq 是 %d，store 分配的 seq 从 1 起", got.Seq)
	}
	if n := f.messageCount(conv.ID); n != 1 {
		t.Errorf("store 里应当有 1 条消息，实际 %d 条", n)
	}
}
