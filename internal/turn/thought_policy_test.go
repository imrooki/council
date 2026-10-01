// thought_policy_test.go 覆盖两件事：
//
//  1. 未决 #9(a) 存储开关的端到端验收判据（任务书原话）：关掉开关跑一轮
//     grok，直接读库核对那一行 thought 为空且 meta 里有 thought_dropped；
//     再把开关打开跑一轮，同一位置必须非空。两个方向都要有，缺了后半，
//     一次「这轮 grok 本来就没 thought」的巧合会把验收骗过去——这里两条
//     用例共用同一份 grokT1JSON（真实带 thought 的原件），只是策略不同，
//     不是靠运气凑出「刚好这轮有/没有 thought」。
//  2. 顺带补上未决 #2 留下的覆盖缺口：断言生产路径的 routeThought 确实是
//     false，判据是送出去的字节（argv 与 stdin），不是注释、不是返回值。
package turn

import (
	"context"
	"strings"
	"testing"

	"council/internal/conversation"
)

// findAgentMessage 从 History 结果里找出 sender 是 agentID 的那一条，
// 供直接读库的端到端用例用。找不到直接 Fatal——那本身就是一种失败。
func findAgentMessage(t *testing.T, msgs []conversation.Message, agentID string) conversation.Message {
	t.Helper()
	for _, m := range msgs {
		if m.SenderID == agentID {
			return m
		}
	}
	t.Fatalf("History 结果里没有 sender=%q 的消息，实际 %d 条", agentID, len(msgs))
	return conversation.Message{}
}

// TestAskStoresThoughtByDefault 是默认方向：Service 不设策略（零值 dropThought
// = false = 存），Ask 一轮 grok，直接读库核对 Thought 非空、Meta 里没有
// thought_dropped。判据是 svc.History 读回来的那一行，不是 rec.Message——
// 后者只是 AppendMessage 的返回值，不是"再读一次库"。
func TestAskStoresThoughtByDefault(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(grokT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentGrok); err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}

	msgs, err := svc.History(context.Background(), convID, 0, 100)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	got := findAgentMessage(t, msgs, testAgentGrok)
	if got.Thought == "" {
		t.Error("默认策略（存）下，读回库的 Thought 是空串，期望非空")
	}
	if _, ok := got.Meta[conversation.MetaThoughtDropped]; ok {
		t.Errorf("默认策略下 Meta 不该出现 thought_dropped，实际: %+v", got.Meta)
	}
}

// TestAskDropsThoughtWhenPolicyOff 是关掉方向：同一份 grokT1JSON，Service
// 显式 SetThoughtPolicy(false)，读回库的那一行 Thought 必须是空串，
// Meta[thought_dropped] 必须是 true。与上一条合起来才是任务书要求的
// 「两个方向都要测」，缺一半会被「这轮本来就没 thought」的巧合骗过去。
func TestAskDropsThoughtWhenPolicyOff(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	svc.SetThoughtPolicy(false)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(grokT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentGrok); err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}

	msgs, err := svc.History(context.Background(), convID, 0, 100)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	got := findAgentMessage(t, msgs, testAgentGrok)
	if got.Thought != "" {
		t.Errorf("关掉存储开关后读回库的 Thought = %q，期望空串", got.Thought)
	}
	if dropped, ok := got.Meta[conversation.MetaThoughtDropped]; !ok || dropped != true {
		t.Errorf("关掉存储开关后 Meta[thought_dropped] = %v（存在=%v），期望 true", dropped, ok)
	}
}

// thoughtNeedle 是 grokT1JSON 里 thought 字段独有的一段文本：只在 thought
// 里出现，不出现在 text（"OK"）里，也不会出现在下面测试自己写的 user 消息
// 里——测试的 user 消息故意不提这串数字或这句话，否则「thought 被路由」
// 与「用户自己就是这么写的」会分不清，反向对照与正向断言都会失去意义。
const thoughtNeedle = "remember the number 8635 and reply with exactly"

// TestRouterProductionPathDoesNotLeakThoughtIntoOutgoingBytes 补上未决 #2
// 留下的覆盖缺口：runner.go 生产路径传给 Router 的是 NewRouter(routeThought)，
// routeThought 由 thoughtVisibleFor 现算；今天没有任何一条用例断言过它真的
// 不泄漏。本用例的会话没绑 team（conv.TeamID == ""），thoughtVisibleFor
// 算出来恒为 false，若这一支悄悄漏成 true（不管是 Router 那一层硬改，还是
// thoughtVisibleFor 在空 teamID 上算错），全套用例照样绿——这条用例存在的
// 意义就是堵死这个「静态推断成立、但从未实测」的缺口。
//
// 判据是**送出去的字节**：zulu（grok）先说一轮带 thought 的话，落库；
// 第二个成员（分别覆盖 stdin 通路的 claude 与 argv 通路的 agy）再问一轮，
// bootstrap 会把 zulu 那条消息选进它的 prompt。断言那个成员真正起进程时
// 的 Command——Args、Stdin、Files——里都不出现 thoughtNeedle。
func TestRouterProductionPathDoesNotLeakThoughtIntoOutgoingBytes(t *testing.T) {
	cases := []struct {
		name    string
		agentID string
		fixture string
	}{
		{"claude：prompt 走 stdin", testAgentClaude, claudeT1JSON},
		{"agy：prompt 走 argv", testAgentAgy, agyT1JSON},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig()
			svc := newTestService(t)
			convID := newConv(t, svc)
			joinAgent(t, svc, convID, testAgentGrok)
			joinAgent(t, svc, convID, tc.agentID)
			postUser(t, svc, convID, "请两位各自核对这条记录")

			r := newTestRunner(t, cfg, svc)
			fs := &fakeSpawner{}
			fs.enqueueSuccess(grokT1JSON)
			fs.enqueueSuccess(tc.fixture)
			r.spawn = fs.spawn

			if _, err := r.Ask(context.Background(), convID, testAgentGrok); err != nil {
				t.Fatalf("zulu 第一轮 Ask 失败: %v", err)
			}
			if _, err := r.Ask(context.Background(), convID, tc.agentID); err != nil {
				t.Fatalf("%s 第二轮 Ask 失败: %v", tc.agentID, err)
			}

			if len(fs.calls) != 2 {
				t.Fatalf("期望恰好两次 spawn 调用，实际 %d 次", len(fs.calls))
			}
			cmd := fs.calls[1].cmd

			for i, a := range cmd.Args {
				if strings.Contains(a, thoughtNeedle) {
					t.Errorf("%s 的 argv[%d] 里出现了 thought 文本: %q", tc.agentID, i, a)
				}
			}
			if strings.Contains(string(cmd.Stdin), thoughtNeedle) {
				t.Errorf("%s 的 stdin 里出现了 thought 文本", tc.agentID)
			}
			for path, content := range cmd.Files {
				if strings.Contains(string(content), thoughtNeedle) {
					t.Errorf("%s 落盘文件 %s 里出现了 thought 文本", tc.agentID, path)
				}
			}
		})
	}
}
