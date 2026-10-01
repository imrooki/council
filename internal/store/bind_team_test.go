// 本文件覆盖 docs/12_STEP9_CONTRACT.md 3.9 第 4 条（片 2a 的一部分）：
// Service.BindTeam 的完整验收——TeamID 读回是新值、会话里多了一条
// event=team_bound 的 system 面包屑、关闭 store 重开一次之后 TeamID 还在
// （证明真的落库了，不是只改了内存）。
//
// 用真实 *Store 而不是 internal/conversation 那份 fakeStore：第三条断言
// 要求关闭再重开同一个数据目录，只有真库能验证这件事——fakeStore 活在
// 进程内存里，"关闭"这个动作对它没有意义（与 record_turn_test.go 顶部
// 注释"四表一事务、失败整轮回滚只有真库能验"是同一条理由）。
package store

import (
	"testing"

	"council/internal/conversation"
)

func TestBindTeamPersistsTeamIDAndLeavesBreadcrumb(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)

	conv := tstNewConv(t, ctx, s, "BindTeam 用例")

	if err := svc.BindTeam(ctx, conv.ID, "team_a"); err != nil {
		t.Fatalf("BindTeam 失败: %v", err)
	}

	t.Run("Conversation 读回的 TeamID 是新值", func(t *testing.T) {
		got, err := svc.Conversation(ctx, conv.ID)
		if err != nil {
			t.Fatalf("读会话失败: %v", err)
		}
		if got.TeamID != "team_a" {
			t.Errorf("TeamID = %q，期望 %q", got.TeamID, "team_a")
		}
	})

	t.Run("会话里多了一条 event=team_bound 的 system 面包屑", func(t *testing.T) {
		msgs, err := svc.History(ctx, conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("读历史失败: %v", err)
		}
		var found *conversation.Message
		for i := range msgs {
			if msgs[i].Kind == conversation.KindSystem &&
				msgs[i].Meta != nil && msgs[i].Meta["event"] == "team_bound" {
				found = &msgs[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("没找到 event=team_bound 的 system 消息，历史里有 %d 条: %+v", len(msgs), msgs)
		}
		if found.SenderID != conversation.ReservedSenderSystem {
			t.Errorf("面包屑 SenderID = %q，期望 %q（09 L#14 定死的形状）",
				found.SenderID, conversation.ReservedSenderSystem)
		}
	})

	// 幂等地关一次：closeFn 内部用 sync.Once 包着，t.Cleanup 里还会再调一次
	// 也不会重复关闭或报错。
	closeFn()

	t.Run("关掉 Store 重开一次再读，TeamID 还是新值", func(t *testing.T) {
		reopenCtx := tstCtx(t)
		reopened, err := Open(reopenCtx, dir)
		if err != nil {
			t.Fatalf("重开失败: %v", err)
		}
		t.Cleanup(func() {
			if err := reopened.Close(); err != nil {
				t.Errorf("关闭重开的库失败: %v", err)
			}
		})
		got, err := reopened.Conversation(reopenCtx, conv.ID)
		if err != nil {
			t.Fatalf("重开之后读会话失败: %v", err)
		}
		if got.TeamID != "team_a" {
			t.Errorf("重开之后 TeamID = %q，期望仍是 %q（说明没有真的落库，只改了内存）",
				got.TeamID, "team_a")
		}
	})
}
