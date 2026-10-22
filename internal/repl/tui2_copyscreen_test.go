package repl

// tui2_copyscreen_test.go 钉住 TUI-2（docs/21f_TUI2_COPY_MODE_TASK.md §6）在 repl
// 这一侧的那一半：复制模式取整场会话正文的翻页、上限、顺序与显示名。
//
// TUI 那一侧（排版、选区、剪贴板）在 internal/tui/tui2_copy_screen_test.go。
// 这里直接读库，用的是 copy_test.go 已经铺好的 cpyNew 与临时库。

import (
	"context"
	"errors"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
)

// TestTui2CopyScreenAll 钉住：整场会话按时间正序、只有 user/agent 正文、
// 用户标 User、成员带 displayName、system/task 不进复制模式。
func TestTui2CopyScreenAll(t *testing.T) {
	r := cpyNew(t, []cpyMsg{
		{conversation.KindUser, "一"},
		{conversation.KindAgent, "二"},
		{conversation.KindSystem, "会话建立"},
		{conversation.KindTask, "工具行"},
		{conversation.KindTaskResult, "工具结果"},
		{conversation.KindAgent, "三"},
	})
	// 显示名每次现读配置（与 /log 等屏幕输出同一条规则）：配置里有名字就用名字。
	r.cfg = &config.Config{Agents: map[string]config.AgentConfig{
		"ag_a": {DisplayName: "甲"},
	}}

	lines, truncated, err := r.CopyScreen(context.Background())
	if err != nil {
		t.Fatalf("CopyScreen 返回错误：%v", err)
	}
	if truncated {
		t.Error("会话只有 3 条正文，不该标记截断")
	}
	want := []CopyLine{
		{User: true, Text: "一"},
		{Who: "甲", Text: "二"},
		{Who: "甲", Text: "三"},
	}
	if len(lines) != len(want) {
		t.Fatalf("应当取到 %d 条，实际 %d 条：%+v", len(want), len(lines), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("第 %d 条 = %+v，期望 %+v", i+1, lines[i], want[i])
		}
	}
}

// TestTui2CopyScreenPaging 钉住跨页：最新一端连着两页系统面包屑时，
// 最早的正文仍然取得到，顺序仍是时间正序。翻页复用 recentCopyable。
func TestTui2CopyScreenPaging(t *testing.T) {
	msgs := []cpyMsg{
		{conversation.KindUser, "甲"},
		{conversation.KindAgent, "乙"},
	}
	for i := 0; i < copyPage*2; i++ {
		msgs = append(msgs, cpyMsg{conversation.KindSystem, "面包屑"})
	}
	r := cpyNew(t, msgs)

	lines, truncated, err := r.CopyScreen(context.Background())
	if err != nil {
		t.Fatalf("CopyScreen 返回错误：%v", err)
	}
	if truncated {
		t.Error("两条正文远小于上限，不该标记截断")
	}
	if len(lines) != 2 || lines[0].Text != "甲" || lines[1].Text != "乙" {
		t.Errorf("跨页之后应当是 [甲 乙]，实际 %+v", lines)
	}
}

// TestTui2CopyScreenTruncation 钉住上限：多要的那一条用来分辨「刚好满」与
// 「不止」；截断时保留最近 limit 条，顺序不变。limit 由调用方给，
// CopyScreen 传 CopyScreenMax，测试用小值，不必真插两千行。
func TestTui2CopyScreenTruncation(t *testing.T) {
	r := cpyNew(t, []cpyMsg{
		{conversation.KindUser, "一"},
		{conversation.KindAgent, "二"},
		{conversation.KindAgent, "三"},
		{conversation.KindUser, "四"},
		{conversation.KindAgent, "五"},
	})
	ctx := context.Background()

	t.Run("over_limit", func(t *testing.T) {
		msgs, truncated, err := r.copyScreenMessages(ctx, r.cur.ID, 3)
		if err != nil {
			t.Fatalf("copyScreenMessages 返回错误：%v", err)
		}
		if !truncated {
			t.Error("5 条正文只取 3 条，应当标记截断")
		}
		if len(msgs) != 3 || msgs[0].Text != "三" || msgs[1].Text != "四" || msgs[2].Text != "五" {
			t.Errorf("截断时应当保留最近 3 条 [三 四 五]，实际 %+v", tui2Texts(msgs))
		}
	})

	t.Run("exact_limit", func(t *testing.T) {
		msgs, truncated, err := r.copyScreenMessages(ctx, r.cur.ID, 5)
		if err != nil {
			t.Fatalf("copyScreenMessages 返回错误：%v", err)
		}
		if truncated {
			t.Error("刚好 5 条时不该标记截断")
		}
		if len(msgs) != 5 || msgs[0].Text != "一" {
			t.Errorf("刚好满时应当原样返回 5 条，实际 %+v", tui2Texts(msgs))
		}
	})
}

// TestTui2CopyScreenEmpty 钉住「没有可复制的消息」的两条路与持久化故障的分叉：
// 没有当前会话、会话里没有可复制正文都返回 ErrCopyEmpty（TUI 都答同一句话）；
// svc 为 nil 仍返回 msgNoStore（那不是没内容，是故障）。
func TestTui2CopyScreenEmpty(t *testing.T) {
	ctx := context.Background()

	t.Run("no_conv", func(t *testing.T) {
		svc, _ := newTestService(t)
		r := New(nil, svc, nil, nil, nil, RulesContext{})
		if _, _, err := r.CopyScreen(ctx); !errors.Is(err, ErrCopyEmpty) {
			t.Errorf("没有当前会话时应当返回 ErrCopyEmpty，实际 %v", err)
		}
	})

	t.Run("no_content", func(t *testing.T) {
		r := cpyNew(t, []cpyMsg{
			{conversation.KindSystem, "会话建立"},
			{conversation.KindTask, "工具行"},
			{conversation.KindAgent, "   "},
		})
		if _, _, err := r.CopyScreen(ctx); !errors.Is(err, ErrCopyEmpty) {
			t.Errorf("只有面包屑与空白正文时应当返回 ErrCopyEmpty，实际 %v", err)
		}
	})

	t.Run("no_store", func(t *testing.T) {
		r := New(nil, nil, nil, nil, nil, RulesContext{})
		cur := conversation.Conversation{ID: "x"}
		r.cur = &cur
		if _, _, err := r.CopyScreen(ctx); err == nil || err.Error() != msgNoStore {
			t.Errorf("svc 为 nil 时应当返回 %q，实际 %v", msgNoStore, err)
		}
	})
}

// tui2Texts 把消息正文摘出来，失败信息里看得懂。
func tui2Texts(msgs []conversation.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Text
	}
	return out
}
