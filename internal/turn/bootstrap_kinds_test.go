package turn

// bootstrap_kinds_test.go 覆盖 docs/11d_STEP8C_CONTRACT.md 第 2.8 节第 6/7 条：
// KindTask 在 selectBootstrap 里无条件收（与 KindUser 同一个分支），以及
// KindTaskResult 与 KindAgent 共用同一个“每个其他成员最后一条发言”的槽位。
//
// 背对背纪律：这两条分支是 11d 2.3 节新裁决的行为，本文件的断言全部来自
// 契约文字，不读 selectBootstrap 当前是否已经这样实现。

import (
	"strings"
	"testing"

	"council/internal/conversation"
)

// TestSelectBootstrapIncludesBroadcastKindTask 覆盖 2.8 第 6 条前半：一条
// 广播的 KindTask（Recipients 留空，SenderID="user"——2.3 节：KindTask 与
// KindUser 同档，因为它就是一条有结构的用户发言），每个成员的 bootstrap
// 结果都要含它，且 Router.Render 出的正文含任务描述。
//
// 用两个不同的 selfID（gk、cl）覆盖“每个成员”，而不是只测一个，
// 因为 2.8 第 6 条原话是“每个成员的 bootstrap 结果都含该 seq”。
func TestSelectBootstrapIncludesBroadcastKindTask(t *testing.T) {
	const taskText = "请在 README 末尾加一行 TASK-OK"
	ms := []conversation.Message{
		{Seq: 1, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindTask, Text: taskText},
	}

	for _, selfID := range []string{"gk", "cl"} {
		selected := selectBootstrap(ms, selfID)
		if !containsSeq(selected, 1) {
			t.Fatalf("selfID=%q 的 bootstrap 结果里没有广播的 KindTask（seq=1），实际: %+v", selfID, selected)
		}
	}

	r := NewRouter(false)
	selected := selectBootstrap(ms, "gk")
	rendered := r.Render(selected, "", true)
	if !strings.Contains(rendered, taskText) {
		t.Fatalf("Render 出的正文没有含任务描述 %q，实际: %q", taskText, rendered)
	}
}

// TestSelectBootstrapDirectedKindTaskOnlyToRecipient 覆盖 2.8 第 6 条后半：
// 一条 Recipients=["gk"] 的 KindTask，只有 gk 挑得到——11d 2.3 节原话
// “定向能力仍在，只是 8c-4 不用它”，这条用例钉住 recipientOK 对 KindTask
// 同样生效。
func TestSelectBootstrapDirectedKindTaskOnlyToRecipient(t *testing.T) {
	ms := []conversation.Message{
		{Seq: 1, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindTask,
			Text: "只给 gk 的任务", Recipients: []string{"gk"}},
	}

	if got := selectBootstrap(ms, "gk"); !containsSeq(got, 1) {
		t.Errorf("gk 的 bootstrap 应当挑到定向给它的 KindTask，实际: %+v", got)
	}
	if got := selectBootstrap(ms, "cl"); containsSeq(got, 1) {
		t.Errorf("cl 的 bootstrap 不该挑到定向给 gk 的 KindTask，实际: %+v", got)
	}
}

// TestSelectBootstrapKindTaskResultSharesSlotWithKindAgent 覆盖 2.8 第 7 条：
// 同一个 sender 先发 KindAgent 再发 KindTaskResult，别人的 bootstrap 里
// 只该出现后者，不是两条都在——11d 2.3 节裁决：那个槽位按成员数封顶，
// 给 KindTaskResult 开第二个槽就是把上界翻倍，所以 KindTaskResult 与
// KindAgent 共用同一个槽（“每个其他成员在窗口内最后一条发言”）。
func TestSelectBootstrapKindTaskResultSharesSlotWithKindAgent(t *testing.T) {
	ms := []conversation.Message{
		{Seq: 1, SenderID: "bob", Kind: conversation.KindAgent, Text: "bob 的讨论发言"},
		{Seq: 2, SenderID: "bob", Kind: conversation.KindTaskResult, Text: "bob 的任务结果统计：改了 1 个文件"},
	}

	selected := selectBootstrap(ms, "carol")
	if containsSeq(selected, 1) {
		t.Fatalf("bob 更早的 KindAgent（seq=1）不该与 KindTaskResult 共存在同一个 bootstrap 里，实际: %+v", selected)
	}
	if !containsSeq(selected, 2) {
		t.Fatalf("bob 的 KindTaskResult（seq=2）应当出现在 bootstrap 里，实际: %+v", selected)
	}
	if len(selected) != 1 {
		t.Fatalf("bob 只该占一个槽，bootstrap 结果应当恰好 1 条，实际 %d 条: %+v", len(selected), selected)
	}
}
