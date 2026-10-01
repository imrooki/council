// thought_policy_test.go 覆盖未决 #9(a)：Service 的存储开关。
//
// 判据始终是 Post / RecordTurn 返回的那条消息本身（Thought 字段与
// Meta[MetaThoughtDropped]），因为 Service 这一层用的是内存假 Store，
// 「读回落库那一行」在这一层等价于「看返回值」；真正读一遍持久化库的
// 端到端验收放在 internal/turn（真实 SQLite），见那边的
// thought_policy_test.go。
package conversation

import (
	"context"
	"testing"
)

const testThoughtText = "这是一段非空的思维链摘要，供未决 #9(a) 的用例使用"

// TestServiceStoresThoughtByDefault 是默认路径的钉子：NewService 之后不调
// SetThoughtPolicy，Thought 必须原样穿过 Post，Meta 里不出现
// MetaThoughtDropped——多数调用方（尤其是全仓其它测试）就是这个不设默认经
// NewService 直接构造的路径，零值必须等于「存」。
func TestServiceStoresThoughtByDefault(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	joinAgent(t, svc, conv.ID, "alpha")

	got, err := svc.Post(context.Background(), Message{
		ConvID:         conv.ID,
		SenderID:       "alpha",
		Kind:           KindAgent,
		Text:           "正文",
		Thought:        testThoughtText,
		SenderProvider: "grok",
		SenderModel:    "grok-4.6",
	})
	if err != nil {
		t.Fatalf("Post 失败: %v", err)
	}
	if got.Thought != testThoughtText {
		t.Errorf("Thought = %q，默认策略下期望原样存住 %q", got.Thought, testThoughtText)
	}
	if _, ok := got.Meta[MetaThoughtDropped]; ok {
		t.Errorf("默认策略下 Meta 不该出现 %q，实际 Meta = %+v", MetaThoughtDropped, got.Meta)
	}
}

// TestServiceDropsThoughtWhenPolicyOff 覆盖关掉开关的那一半：Thought 落成
// 空串（不是不落这一行——消息本身仍然落库），且 Meta[MetaThoughtDropped] == true。
func TestServiceDropsThoughtWhenPolicyOff(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	svc.SetThoughtPolicy(false)
	joinAgent(t, svc, conv.ID, "alpha")

	got, err := svc.Post(context.Background(), Message{
		ConvID:         conv.ID,
		SenderID:       "alpha",
		Kind:           KindAgent,
		Text:           "正文",
		Thought:        testThoughtText,
		SenderProvider: "grok",
		SenderModel:    "grok-4.6",
	})
	if err != nil {
		t.Fatalf("Post 失败: %v", err)
	}
	if got.Thought != "" {
		t.Errorf("Thought = %q，关掉存储开关后期望落成空串", got.Thought)
	}
	dropped, ok := got.Meta[MetaThoughtDropped]
	if !ok {
		t.Fatalf("关掉存储开关后 Meta 里没有 %q，实际 Meta = %+v", MetaThoughtDropped, got.Meta)
	}
	if dropped != true {
		t.Errorf("Meta[%q] = %v，期望 true", MetaThoughtDropped, dropped)
	}
}

// TestServiceDoesNotMarkDroppedWhenThoughtAlreadyEmpty 钉住「只在真的丢掉了
// 非空 thought 时才写」这条：Thought 本来就是空串（这家 provider 没有这个
// 概念，或者这条本来就是不产生 thought 的消息），关掉开关不该凭空造出一条
// thought_dropped=true——那会把「本来没有」误报成「我们丢了」，正是未决
// #11 里 sender_effort 的坑在这里的翻版。
func TestServiceDoesNotMarkDroppedWhenThoughtAlreadyEmpty(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	svc.SetThoughtPolicy(false)
	joinAgent(t, svc, conv.ID, "alpha")

	got, err := svc.Post(context.Background(), Message{
		ConvID:         conv.ID,
		SenderID:       "alpha",
		Kind:           KindAgent,
		Text:           "正文",
		Thought:        "",
		SenderProvider: "agy",
		SenderModel:    "gemini-3.8-flash-high",
	})
	if err != nil {
		t.Fatalf("Post 失败: %v", err)
	}
	if _, ok := got.Meta[MetaThoughtDropped]; ok {
		t.Errorf("Thought 本来就是空串，Meta 不该出现 %q，实际 Meta = %+v", MetaThoughtDropped, got.Meta)
	}
}

// TestServiceThoughtPolicyDoesNotMutateCallerMessage 断言 applyThoughtPolicy
// 是无副作用的：Post 收的是值类型 Message，但 Meta 是引用类型的 map，
// 若实现就地往调用方的 Meta 塞键而不是先克隆，调用方自己持有的那份 map
// 会被这次调用悄悄污染。这里显式在 Post 之后检查调用方手里的原始 map，
// 它必须还是调用前的样子。
func TestServiceThoughtPolicyDoesNotMutateCallerMessage(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	svc.SetThoughtPolicy(false)
	joinAgent(t, svc, conv.ID, "alpha")

	callerMeta := map[string]any{"existing_key": "existing_value"}
	_, err := svc.Post(context.Background(), Message{
		ConvID:         conv.ID,
		SenderID:       "alpha",
		Kind:           KindAgent,
		Text:           "正文",
		Thought:        testThoughtText,
		SenderProvider: "grok",
		SenderModel:    "grok-4.6",
		Meta:           callerMeta,
	})
	if err != nil {
		t.Fatalf("Post 失败: %v", err)
	}
	if len(callerMeta) != 1 {
		t.Fatalf("调用方原始 Meta 被改动了，现在有 %d 个键: %+v", len(callerMeta), callerMeta)
	}
	if _, ok := callerMeta[MetaThoughtDropped]; ok {
		t.Errorf("调用方原始 Meta 不该被写入 %q，实际: %+v", MetaThoughtDropped, callerMeta)
	}
}

// TestRecordTurnAppliesThoughtPolicy 覆盖 RecordTurn 那一半——两处校验本来
// 就逐条重复，这条也不例外——同时顺带钉住 B 类风险：RecordTurn 收的是
// *Message，不能就地改调用方手里的那个结构体（internal/turn.Runner.Ask
// 之后可能还要读它）。
func TestRecordTurnAppliesThoughtPolicy(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	svc.SetThoughtPolicy(false)
	joinAgent(t, svc, conv.ID, "alpha")
	ctx := context.Background()

	msg := Message{
		ConvID:         conv.ID,
		SenderID:       "alpha",
		Kind:           KindAgent,
		Text:           "正文",
		Thought:        testThoughtText,
		SenderProvider: "grok",
		SenderModel:    "grok-4.6",
	}
	seq := int64(1)
	u := TurnUsage{TurnID: "turn-thought-policy-1", ConvID: conv.ID, AgentID: "alpha", Seq: &seq, Outcome: 1}
	session := ProviderSession{ConvID: conv.ID, AgentID: "alpha", ProviderSessionID: "sess-1"}

	stored, err := svc.RecordTurn(ctx, &msg, u, &session, false)
	if err != nil {
		t.Fatalf("RecordTurn 失败: %v", err)
	}
	if stored == nil {
		t.Fatal("RecordTurn 返回的 *Message 是 nil")
	}
	if stored.Thought != "" {
		t.Errorf("落库消息 Thought = %q，关掉存储开关后期望空串", stored.Thought)
	}
	if stored.Meta[MetaThoughtDropped] != true {
		t.Errorf("落库消息 Meta[%q] = %v，期望 true", MetaThoughtDropped, stored.Meta[MetaThoughtDropped])
	}

	// 调用方（这里就是本测试函数）手里的 msg 必须原样未动：RecordTurn 不许
	// 就地改调用方传进来的 *Message。
	if msg.Thought != testThoughtText {
		t.Errorf("调用方的 msg.Thought 被 RecordTurn 改动了，现在是 %q，期望仍是 %q", msg.Thought, testThoughtText)
	}
	if _, ok := msg.Meta[MetaThoughtDropped]; ok {
		t.Errorf("调用方的 msg.Meta 被 RecordTurn 写入了 %q，实际: %+v", MetaThoughtDropped, msg.Meta)
	}
}
