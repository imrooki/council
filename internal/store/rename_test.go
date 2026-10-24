// rename_test.go 覆盖 GUI 路线第 3 片（3a）在 store 层新增的动作：
// RenameConversation 只改 conversations.title，且走真实写事务。
//
// 标题本身的长度 / 空白校验属于 HTTP 层的请求校验，不在这里重复：
// store 只回答「这一列现在是什么」，不解释什么样的标题算合法。
package store

import (
	"context"
	"errors"
	"testing"

	"council/internal/conversation"
)

// TestRenameConversationUpdatesTitle 钉住核心行为：改名之后单条读、列表读
// 与底层行都是新标题，而 id、created_at、team_id、cwd 与消息一条都没动。
func TestRenameConversationUpdatesTitle(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "旧标题")
	if err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetConversationTeam(ctx, conv.ID, "team-keep")
	}); err != nil {
		t.Fatalf("绑定 team 失败: %v", err)
	}
	tstCwdSet(t, ctx, s, conv.ID, `D:\keep-me`)
	tstAppend(t, ctx, s, tstUserMsg(conv.ID, "领养这条消息"))

	const renamed = "新标题 · rename"

	if err := s.RenameConversation(ctx, conv.ID, renamed); err != nil {
		t.Fatalf("RenameConversation(%s) 失败: %v", conv.ID, err)
	}

	got, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("改名后读会话失败: %v", err)
	}
	if got.Title != renamed {
		t.Errorf("Conversation 的 Title = %q，期望 %q", got.Title, renamed)
	}
	if got.ID != conv.ID {
		t.Errorf("ID = %q，期望 %q", got.ID, conv.ID)
	}
	if !got.CreatedAt.Equal(conv.CreatedAt) {
		t.Errorf("CreatedAt = %s，期望 %s（改名不该改创建时间）", got.CreatedAt, conv.CreatedAt)
	}
	if got.TeamID != "team-keep" {
		t.Errorf("TeamID = %q，期望 team-keep（改名不该解绑 team）", got.TeamID)
	}
	if got.Cwd != `D:\keep-me` {
		t.Errorf("Cwd = %q，期望 D:\\keep-me（改名不该改项目目录）", got.Cwd)
	}

	list, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("改名后列会话失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("conversations 有 %d 行，期望 1 行（改名不是新建）", len(list))
	}
	if list[0].Title != renamed {
		t.Errorf("Conversations 里的 Title = %q，期望 %q", list[0].Title, renamed)
	}

	// 底层行同样要是新标题：只改列表读的那份缓存不是改库。
	raw := tstRawDB(t, s.Path())
	if got := tstScanText(t, raw, `SELECT title FROM conversations WHERE id = ?`, conv.ID); got != renamed {
		t.Errorf("库里的 title = %q，期望 %q", got, renamed)
	}
	msgs, err := s.Messages(ctx, conv.ID, 0, 0)
	if err != nil {
		t.Fatalf("改名后读消息失败: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Text != "领养这条消息" {
		t.Errorf("改名之后消息 = %+v，期望那条唯一消息原样还在", msgs)
	}
}

// TestRenameConversationUnknownID 确认不存在的 id 走哨兵而不是静默成功：
// HTTP 层据此回 404，而不是先回 200 再让用户对着没变的标题发愣。
func TestRenameConversationUnknownID(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "唯一存在的会话")
	before := tstCwdConvCount(t, s)

	err := s.RenameConversation(ctx, "conv-does-not-exist", "不该落库的标题")
	if !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Fatalf("未知 id：errors.Is(ErrConversationNotFound) = false，实际错误: %v", err)
	}
	if after := tstCwdConvCount(t, s); after != before {
		t.Errorf("conversations 行数从 %d 变成 %d，期望不变", before, after)
	}
	got, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("读原有会话失败: %v", err)
	}
	if got.Title != "唯一存在的会话" {
		t.Errorf("原有会话标题 = %q，期望不变", got.Title)
	}
}

// TestRenameConversationEmptyTitleIsStorePassThrough 说明 store 不拦空标题：
// 那是 HTTP 的请求校验（400），store 只负责持久化。这条用例把边界写清楚，
// 免得以后有人在 store 里补一个校验、让 CLI 与网页对同一列有两条规则。
func TestRenameConversationEmptyTitleIsStorePassThrough(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "有标题")
	if err := s.RenameConversation(ctx, conv.ID, ""); err != nil {
		t.Fatalf("RenameConversation 传空串失败: %v", err)
	}
	got, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("读会话失败: %v", err)
	}
	if got.Title != "" {
		t.Errorf("Title = %q，期望空串（store 不做 HTTP 层的非空校验）", got.Title)
	}
}
