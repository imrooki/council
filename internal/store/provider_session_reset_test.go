package store

// provider_session_reset_test.go 覆盖 A3 新增的 Store.ResetProviderSessions：
// 只删目标成员、跨全部会话、行数如实、别的成员与别的表一行不动、
// 空 id 与只读库明确报错。

import (
	"testing"

	"council/internal/conversation"
)

// TestResetProviderSessionsOnlyTargetAgentAllConversations 是本方法的核心
// 断言：一个成员在两条会话里都有记录时一次删干净，另一个成员那一行原样
// 还在——「只重置被改的那个成员」是这个功能对用户的承诺，不只是实现细节。
func TestResetProviderSessionsOnlyTargetAgentAllConversations(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	convA := tstNewConv(t, ctx, s, "重置会话 A")
	convB := tstNewConv(t, ctx, s, "重置会话 B")

	tstSetProviderSession(t, ctx, s, tstBaseSession(convA.ID, "alpha"))
	tstSetProviderSession(t, ctx, s, tstBaseSession(convB.ID, "alpha"))
	tstSetProviderSession(t, ctx, s, tstBaseSession(convA.ID, "beta"))

	n, err := s.ResetProviderSessions(ctx, "alpha")
	if err != nil {
		t.Fatalf("ResetProviderSessions(alpha) 失败: %v", err)
	}
	if n != 2 {
		t.Errorf("删掉的行数 = %d，期望 2（alpha 在两条会话里各一行）", n)
	}

	for _, conv := range []conversation.Conversation{convA, convB} {
		if _, ok, err := s.ProviderSession(ctx, conv.ID, "alpha"); err != nil || ok {
			t.Errorf("会话 %s 里 alpha 的 provider_sessions 仍在: ok=%v err=%v", conv.ID, ok, err)
		}
	}
	// beta 一行都不许动：重置是按成员（agent_id）而不是按会话扫全表。
	got, ok, err := s.ProviderSession(ctx, convA.ID, "beta")
	if err != nil || !ok {
		t.Fatalf("beta 的记录被一起删掉了: ok=%v err=%v", ok, err)
	}
	if got.ProviderSessionID != "sess-beta" || got.IdentityFP != "fp-beta" {
		t.Errorf("beta 的记录被改动了: %+v", got)
	}

	// 两张会话本身还在：这个动作只碰 provider_sessions 一张表。
	for _, conv := range []conversation.Conversation{convA, convB} {
		if _, err := s.Conversation(ctx, conv.ID); err != nil {
			t.Errorf("会话 %s 被动了: %v", conv.ID, err)
		}
	}
}

// TestResetProviderSessionsMissingRowIsZero 覆盖幂等：这个成员还没说过话
// （或已经被重置过）时返回 0 行、不报错。设置页对「从没开过会话的成员」
// 改 model 走的正是这条路径。
func TestResetProviderSessionsMissingRowIsZero(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "重置一个没说过话的")

	n, err := s.ResetProviderSessions(ctx, "never-spoke")
	if err != nil {
		t.Fatalf("重置一个没有记录的成员不该报错: %v", err)
	}
	if n != 0 {
		t.Errorf("删掉的行数 = %d，期望 0", n)
	}

	tstSetProviderSession(t, ctx, s, tstBaseSession(conv.ID, "alpha"))
	if n, err = s.ResetProviderSessions(ctx, "alpha"); err != nil || n != 1 {
		t.Fatalf("第一次重置 = (%d, %v)，期望 (1, nil)", n, err)
	}
	if n, err = s.ResetProviderSessions(ctx, "alpha"); err != nil || n != 0 {
		t.Errorf("重复重置 = (%d, %v)，期望 (0, nil)（幂等，不是报错）", n, err)
	}
}

// TestResetProviderSessionsRejectsEmptyAgentID 挡住空 id：它只会来自调用方
// 传错变量，静默返回 0 行会把一次误调用伪装成「本来就没有会话」。
func TestResetProviderSessionsRejectsEmptyAgentID(t *testing.T) {
	s, _ := tstStore(t)

	if _, err := s.ResetProviderSessions(tstCtx(t), ""); err == nil {
		t.Error("空 agent id 应当报错，实际 nil")
	}
}

// TestResetProviderSessionsReadOnlyStoreFails 钉住只读模式：没有写句柄时
// 必须报错而不是返回 0 行——「删了 0 行」会被上层记成一次成功的重置。
func TestResetProviderSessionsReadOnlyStoreFails(t *testing.T) {
	dir := tstDataDir(t)
	w, closeW := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, w, "只读库里的会话")
	tstSetProviderSession(t, ctx, w, tstBaseSession(conv.ID, "alpha"))
	closeW()

	ro := tstReadOnlyAt(t, dir)
	if _, err := ro.ResetProviderSessions(ctx, "alpha"); err == nil {
		t.Error("只读 Store 上的 ResetProviderSessions 应当报错，实际 nil")
	}
}
