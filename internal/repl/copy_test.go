package repl

// copy_test.go 钉住 TUI-1（docs/21e_TUI1_TASK.md §4）在 repl 这一侧的那一半：
// /copy 的参数解析、从当前会话取哪几条正文、以及两条入口的分工。
// TUI 那一侧的取数入口是 CopyRecent；行式入口 cmdCopy 只说明「这里没有剪贴板」。

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"council/internal/command"
	"council/internal/conversation"
)

// cpyMsg 是一条种子：kind 决定它算不算可复制的正文。
type cpyMsg struct {
	kind conversation.MessageKind
	text string
}

// cpyNew 造一个「已经有当前会话」的 REPL，并把 msgs 依次落在会话里。
// 直接 AppendMessage 到假 store，绕开 Service.Post 的成员校验：
// 这里要钉的是取数规则，不是落库规则。
func cpyNew(t *testing.T, msgs []cpyMsg) *REPL {
	t.Helper()
	svc, f := newTestService(t)
	conv := f.seedConversation("cpyconv", "copy")
	ctx := context.Background()
	for i, m := range msgs {
		sender := conversation.ReservedSenderUser
		switch m.kind {
		case conversation.KindAgent:
			sender = "ag_a"
		case conversation.KindSystem:
			sender = conversation.ReservedSenderSystem
		}
		msg := conversation.Message{ConvID: conv.ID, SenderID: sender, Kind: m.kind, Text: m.text}
		if _, err := f.AppendMessage(ctx, msg); err != nil {
			t.Fatalf("前提：第 %d 条种子 AppendMessage 失败：%v", i+1, err)
		}
	}
	r := New(nil, svc, nil, nil, nil, RulesContext{})
	cur := conv
	r.cur = &cur
	return r
}

// TestCpyRecentLastMessages 钉住「最近 n 条、按时间正序、两条之间空一行」，
// 以及 system / task / task_result 不进剪贴板。
//
// 种子按 seq：user 一、agent 二、system 面包屑、task 工具、task_result 结果、user 三。
// 可复制的只有「一」「二」「三」三条，所以 /copy 1 是「三」，/copy 2 是「二\n\n三」。
func TestCpyRecentLastMessages(t *testing.T) {
	r := cpyNew(t, []cpyMsg{
		{conversation.KindUser, "一"},
		{conversation.KindAgent, "二"},
		{conversation.KindSystem, "会话建立"},
		{conversation.KindTask, "工具行"},
		{conversation.KindTaskResult, "工具结果"},
		{conversation.KindUser, "三"},
	})
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		args string
		want string
	}{
		{"default", "", "三"},
		{"one", "1", "三"},
		{"two", "2", "二\n\n三"},
		{"three", "3", "一\n\n二\n\n三"},
		// 要的比有的多：把能复制的全给出来，不报错。
		{"more_than_available", "9", "一\n\n二\n\n三"},
		// 离谱的大数也一样：预分配按页封顶，否则一行 /copy 就能把内存吃光。
		{"huge", "1000000000", "一\n\n二\n\n三"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.CopyRecent(ctx, tc.args)
			if err != nil {
				t.Fatalf("CopyRecent(%q) 返回错误：%v", tc.args, err)
			}
			if got != tc.want {
				t.Errorf("CopyRecent(%q) = %q，期望 %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestCpyRecentPaging 钉住往回翻页：超过一页（copyPage = 50）的系统面包屑之后
// 仍然能取到最早的那两条正文，而且顺序是时间正序。
//
// 【判别力】只读开头一页（不往前翻）时这里是 ErrCopyEmpty；
// 翻页方向写反（把窗口往新的一端挪）时取到的是靠后的空段。
func TestCpyRecentPaging(t *testing.T) {
	msgs := []cpyMsg{
		{conversation.KindUser, "甲"},
		{conversation.KindAgent, "乙"},
	}
	for i := 0; i < copyPage*2; i++ {
		msgs = append(msgs, cpyMsg{conversation.KindSystem, "面包屑"})
	}
	r := cpyNew(t, msgs)

	got, err := r.CopyRecent(context.Background(), "2")
	if err != nil {
		t.Fatalf("CopyRecent(\"2\") 返回错误：%v", err)
	}
	if want := "甲\n\n乙"; got != want {
		t.Errorf("跨页之后的正文 = %q，期望 %q", got, want)
	}
}

// TestCpyRecentEmpty 钉住「没有可复制的内容」：一条正文都没有、或一场空会话。
func TestCpyRecentEmpty(t *testing.T) {
	ctx := context.Background()

	t.Run("only_meta", func(t *testing.T) {
		r := cpyNew(t, []cpyMsg{
			{conversation.KindSystem, "会话建立"},
			{conversation.KindTask, "工具行"},
			{conversation.KindAgent, "   "},
		})
		if _, err := r.CopyRecent(ctx, ""); !errors.Is(err, ErrCopyEmpty) {
			t.Errorf("只有面包屑与空白正文时应当返回 ErrCopyEmpty，实际 %v", err)
		}
	})

	t.Run("no_messages", func(t *testing.T) {
		r := cpyNew(t, nil)
		if _, err := r.CopyRecent(ctx, ""); !errors.Is(err, ErrCopyEmpty) {
			t.Errorf("空会话应当返回 ErrCopyEmpty，实际 %v", err)
		}
	})
}

// TestCpyRecentUsage 钉住非法参数：负数、0、非数字、多给一个词都返回 ErrCopyUsage，
// 而且提示行与其余命令的「用法: …」同一体例。
func TestCpyRecentUsage(t *testing.T) {
	r := cpyNew(t, []cpyMsg{{conversation.KindUser, "一"}})
	ctx := context.Background()

	for _, args := range []string{"-1", "0", "abc", "1.5", "1 2", "3x"} {
		got, err := r.CopyRecent(ctx, args)
		if !errors.Is(err, ErrCopyUsage) {
			t.Errorf("CopyRecent(%q) 应当返回 ErrCopyUsage，实际 %v（正文 %q）", args, err, got)
			continue
		}
		if err.Error() != CopyUsage {
			t.Errorf("CopyRecent(%q) 的提示行 = %q，期望 %q", args, err.Error(), CopyUsage)
		}
	}
}

// TestCpyRecentPreconditions 钉住两道前置条件：答复与 runCommand 里
// needsSvc / needsConv 的逐字相同（TUI 的 /copy 不走 runCommand，得在这里自己判）。
func TestCpyRecentPreconditions(t *testing.T) {
	ctx := context.Background()

	t.Run("no_store", func(t *testing.T) {
		r := New(nil, nil, nil, nil, nil, RulesContext{})
		cur := conversation.Conversation{ID: "x"}
		r.cur = &cur
		if _, err := r.CopyRecent(ctx, ""); err == nil || err.Error() != msgNoStore {
			t.Errorf("svc 为 nil 时应当返回 %q，实际 %v", msgNoStore, err)
		}
	})

	t.Run("no_conv", func(t *testing.T) {
		svc, _ := newTestService(t)
		r := New(nil, svc, nil, nil, nil, RulesContext{})
		if _, err := r.CopyRecent(ctx, ""); err == nil || err.Error() != msgNoConv {
			t.Errorf("没有当前会话时应当返回 %q，实际 %v", msgNoConv, err)
		}
	})
}

// TestCpyLineEntryHasNoClipboard 钉住行式入口：/copy 不读库、不假装复制成功，
// 只回一句说明（剪贴板只有终端 UI 那一侧有）。
func TestCpyLineEntryHasNoClipboard(t *testing.T) {
	var buf bytes.Buffer
	r := New(nil, nil, nil, nil, &buf, RulesContext{})
	if err := r.dispatch(context.Background(), "/copy 3"); err != nil {
		t.Fatalf("/copy 3 返回错误：%v", err)
	}
	if got, want := buf.String(), MsgCopyNotInTUI+"\n"; got != want {
		t.Errorf("/copy 3 的输出 = %q，期望 %q", got, want)
	}
}

// TestCpyRegistryEntry 钉住 /copy 的元数据与它 /help 里那一行的位置。
//
// 位置按 Name 的字节序：'n' < 'p'，所以 conv 在 copy 之前（任务书写的「copy 在 conv 之前」
// 与字节序不符；/help 的顺序由 Registry.List 排序，登记顺序不参与）。
func TestCpyRegistryEntry(t *testing.T) {
	reg := newCommandRegistry()
	spec, entry, ok := reg.Lookup("copy")
	if !ok {
		t.Fatal("命令表里没有 copy")
	}
	if spec.Name != "copy" || spec.Args != "[n]" {
		t.Errorf("元数据应当是 name=copy、args=[n]，实际 %q / %q", spec.Name, spec.Args)
	}
	if spec.Summary == "" {
		t.Error("copy 的 Summary 是空的：/help 会少一列说明")
	}
	if entry.run == nil {
		t.Error("copy 没有处理函数：行式入口一敲就空指针")
	}

	specs := reg.List(command.ScopeTUI)
	idx := -1
	for i, s := range specs {
		if s.Name == "copy" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("/help 的可见集里没有 copy：%v", specs)
	}
	if idx == 0 || specs[idx-1].Name != "conv" {
		t.Errorf("copy 应当紧跟在 conv 之后（字节序 'n' < 'p'），实际前一条是 %q", specs[max(0, idx-1)].Name)
	}
	if idx+1 >= len(specs) || specs[idx+1].Name != "cost" {
		t.Errorf("copy 的后一条应当是 cost")
	}
}
