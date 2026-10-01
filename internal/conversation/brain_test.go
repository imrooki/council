package conversation

import (
	"context"
	"errors"
	"testing"
)

// TestPostRejectsUserCarryingBrain 锁住 Post 的第五条校验：
// 保留 sender "user" 不得携带 SenderProvider / SenderModel / SenderEffort。
//
// 人不是模型，没有脑子可记。真出现说明上层把某个 agent 的回复当成用户发言写进来了，
// 那是投递路径的错位，必须响亮地失败而不是静默存下一条自相矛盾的记录。
// 三列各自单独带一个也要拦 —— 只拦「三个都带」的话，错位写入只填了一列时会漏过去。
func TestPostRejectsUserCarryingBrain(t *testing.T) {
	cases := []struct {
		name   string
		kind   MessageKind
		mutate func(m *Message)
	}{
		{"只带 provider", KindUser, func(m *Message) { m.SenderProvider = "claude" }},
		{"只带 model", KindUser, func(m *Message) { m.SenderModel = "opus" }},
		{"只带 effort", KindUser, func(m *Message) { m.SenderEffort = "high" }},
		{"三个都带", KindUser, func(m *Message) {
			m.SenderProvider = "claude"
			m.SenderModel = "opus"
			m.SenderEffort = "high"
		}},
		// 这条锁住「Post 只看 SenderID，完全不看 Kind」。
		// docs/08_STEP5_SPEC.md 曾经把这一格写反，说 sender=user 配 kind=agent
		// 能带脑子写进去；实际 Post 照样返回 ErrInvalidSender。
		// 上面四条全是 KindUser，踩不到这个格子。
		{"kind 是 agent 也照样拦", KindAgent, func(m *Message) {
			m.SenderProvider = "claude"
			m.SenderModel = "opus"
			m.SenderEffort = "high"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, f, conv := newServiceForTest(t)
			before := f.messageCount(conv.ID)

			m := Message{
				ConvID:   conv.ID,
				SenderID: ReservedSenderUser,
				Kind:     tc.kind,
				Text:     "这句是人敲的",
			}
			tc.mutate(&m)

			if _, err := svc.Post(context.Background(), m); err == nil {
				t.Fatal("带脑子的 user 消息竟然 Post 成功了")
			} else if !errors.Is(err, ErrInvalidSender) {
				t.Errorf("错误应当能被 errors.Is 认出 ErrInvalidSender，实际: %v", err)
			}

			// 校验不过一律不落库：这条比错误类型更重要，
			// 报了错却还是写进去了，等于事实源里留了一条假记录。
			if got := f.messageCount(conv.ID); got != before {
				t.Errorf("校验不过却落了库：消息数从 %d 变成 %d", before, got)
			}
		})
	}
}

// TestPostAcceptsUserWithoutBrain 是上一条的对照组。
// 没有它的话，上面四条「被拒」可能只是因为整条路径本来就不通。
func TestPostAcceptsUserWithoutBrain(t *testing.T) {
	svc, _, conv := newServiceForTest(t)

	got, err := svc.Post(context.Background(), Message{
		ConvID:   conv.ID,
		SenderID: ReservedSenderUser,
		Kind:     KindUser,
		Text:     "不带脑子的人话",
	})
	if err != nil {
		t.Fatalf("不带脑子的 user 消息应当 Post 成功，实际: %v", err)
	}
	if !got.SenderBrainEmpty() {
		t.Errorf("user 消息读回来竟然带了脑子: %+v", got)
	}
}

// TestPostAcceptsAgentWithBrain 锁住 agent 的脑子能原样穿过 Service。
// Service 不该对 agent 的这三列做任何加工 —— 它只是把 provider 适配器填的东西转交给 store。
func TestPostAcceptsAgentWithBrain(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	joinAgent(t, svc, conv.ID, "alpha")

	got, err := svc.Post(context.Background(), Message{
		ConvID:         conv.ID,
		SenderID:       "alpha",
		Kind:           KindAgent,
		Text:           "一阶剪切变形理论",
		SenderProvider: "claude",
		SenderModel:    "opus",
		SenderEffort:   "high",
	})
	if err != nil {
		t.Fatalf("带脑子的 agent 消息应当 Post 成功，实际: %v", err)
	}
	if got.SenderProvider != "claude" || got.SenderModel != "opus" || got.SenderEffort != "high" {
		t.Errorf("agent 的脑子被 Service 改动了: provider=%q model=%q effort=%q",
			got.SenderProvider, got.SenderModel, got.SenderEffort)
	}
	if got.SenderBrainEmpty() {
		t.Error("带了脑子的消息 SenderBrainEmpty() 却是 true")
	}
}

// TestSenderBrainEmpty 单测那个判空方法本身。
// 它不要求三段同时填满：有的 provider 没有 effort 这个概念，只填两段是合法的，
// 那种消息**不是**「没记脑子」。
func TestSenderBrainEmpty(t *testing.T) {
	cases := []struct {
		name                    string
		provider, model, effort string
		want                    bool
	}{
		{"三段全空", "", "", "", true},
		{"只有 provider", "grok", "", "", false},
		{"只有 model", "", "opus", "", false},
		{"只有 effort", "", "", "high", false},
		{"缺 effort", "grok", "grok-4.6", "", false},
		{"三段齐全", "claude", "opus", "high", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Message{
				SenderProvider: tc.provider,
				SenderModel:    tc.model,
				SenderEffort:   tc.effort,
			}
			if got := m.SenderBrainEmpty(); got != tc.want {
				t.Errorf("SenderBrainEmpty() = %v，期望 %v", got, tc.want)
			}
		})
	}
}
