package conversation

import "testing"

// TestMessageKindValid 锁住 MessageKind.Valid 的判定集合：
// 只有契约里列出的五个值合法；空串、大小写变体、拼写变体都不合法。
func TestMessageKindValid(t *testing.T) {
	valid := []MessageKind{KindUser, KindAgent, KindSystem, KindTask, KindTaskResult}
	for _, k := range valid {
		t.Run("合法 "+string(k), func(t *testing.T) {
			if !k.Valid() {
				t.Errorf("MessageKind(%q).Valid() = false，它是契约里的五个合法值之一", string(k))
			}
		})
	}

	invalid := []MessageKind{
		"",
		" ",
		"User",
		"AGENT",
		"System",
		"Task",
		"Task_Result",
		"TASK_RESULT",
		"taskresult",
		"task-result",
		"task_results",
		"users",
		"user ",
		" user",
		"assistant",
		"unknown",
	}
	for _, k := range invalid {
		t.Run("非法 "+string(k), func(t *testing.T) {
			if k.Valid() {
				t.Errorf("MessageKind(%q).Valid() = true，它不属于五个合法值", string(k))
			}
		})
	}
}

// TestMessageKindLiterals 锁住五个常量的字面值。
// 这些字符串会原样落进 SQLite 的 CHECK 约束与库里的行，
// 改字面值等于改已有数据的格式，属于契约面而不是实现细节。
func TestMessageKindLiterals(t *testing.T) {
	want := []struct {
		kind    MessageKind
		literal string
	}{
		{KindUser, "user"},
		{KindAgent, "agent"},
		{KindSystem, "system"},
		{KindTask, "task"},
		{KindTaskResult, "task_result"},
	}
	for _, w := range want {
		if string(w.kind) != w.literal {
			t.Errorf("常量的字面值是 %q，契约要求 %q", string(w.kind), w.literal)
		}
	}
}

// TestReservedSenderUser 锁住保留 sender id 的字面值。
// 它既是 Post 的放行条件，也是 config 里被拒绝的 agent id（agents.user is reserved），
// 两处必须是同一个字符串。
func TestReservedSenderUser(t *testing.T) {
	if ReservedSenderUser != "user" {
		t.Errorf("ReservedSenderUser = %q，契约要求 %q", ReservedSenderUser, "user")
	}
}
