package repl

import (
	"context"
	"strings"
	"testing"

	"council/internal/conversation"
)

// seedBrainMessage 直接塞一条带脑子的消息，绕过 Service 的校验。
// 和 seedMessage 一样是「证明输出来自库」用的：/log 的断言必须看得见库里的东西，
// 而不是某条命令自己的回显。
func seedBrainMessage(t *testing.T, f *fakeStore, convID, senderID string,
	kind conversation.MessageKind, text, provider, model, effort string) {
	t.Helper()
	_, err := f.AppendMessage(context.Background(), conversation.Message{
		ConvID:         convID,
		SenderID:       senderID,
		Kind:           kind,
		Text:           text,
		SenderProvider: provider,
		SenderModel:    model,
		SenderEffort:   effort,
	})
	if err != nil {
		t.Fatalf("塞入带脑子的消息失败: %v", err)
	}
}

// lineContaining 返回 out 里第一条含 needle 的行，找不到就让测试失败。
// 表格断言要落在**某一行**上：整份输出里出现某个字符串，不能证明它出现在对的那一行。
func lineContaining(t *testing.T, out, needle string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("输出里没有含 %q 的行；完整输出:\n%s", needle, out)
	return ""
}

// TestBareTextMatchesSayWhenBlocked 锁住契约里那条硬性要求：
// 裸文本与 /say 必须共用同一套前置条件。
//
// 断言方式是「两条路径的输出逐字节相同」，而不是去比对某句提示的措辞 ——
// 措辞不属于契约面（08a），但两条路径答得一不一样属于。
// 这里只覆盖被前置条件拦下的两种局面：那时两者都不产生随机 id，输出才可比。
// 真正发出去的情形见 TestBareTextAndSayStoreTheSameMessage。
func TestBareTextMatchesSayWhenBlocked(t *testing.T) {
	const text = "推导一下 <project> 的位移场"

	t.Run("持久化未就绪", func(t *testing.T) {
		bare, err := runREPLSvc(t, nil, text+"\n")
		if err != nil {
			t.Fatalf("裸文本输入下 Run 应当返回 nil，实际: %v", err)
		}
		say, err := runREPLSvc(t, nil, "/say "+text+"\n")
		if err != nil {
			t.Fatalf("/say 输入下 Run 应当返回 nil，实际: %v", err)
		}
		if bare != say {
			t.Errorf("裸文本与 /say 的输出不一致。\n裸文本:\n%s\n/say:\n%s", bare, say)
		}
	})

	t.Run("还没选会话", func(t *testing.T) {
		svc, f := newTestService(t)
		f.seedConversation("conv_zzz", "存在但没被选中的会话")

		bare, err := runREPLSvc(t, svc, text+"\n")
		if err != nil {
			t.Fatalf("裸文本输入下 Run 应当返回 nil，实际: %v", err)
		}
		say, err := runREPLSvc(t, svc, "/say "+text+"\n")
		if err != nil {
			t.Fatalf("/say 输入下 Run 应当返回 nil，实际: %v", err)
		}
		if bare != say {
			t.Errorf("裸文本与 /say 的输出不一致。\n裸文本:\n%s\n/say:\n%s", bare, say)
		}
		// 顺带锁住：没选会话时不该悄悄写进别的会话里。
		if n := len(f.msgs["conv_zzz"]); n != 0 {
			t.Errorf("没选会话却往 conv_zzz 写了 %d 条消息", n)
		}
	})
}

// TestBareTextPostsAsUser 锁住本次改动的主行为：
// 非空、不以 / 开头的一行，直接成为一条 user 消息。
func TestBareTextPostsAsUser(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_bare", "裸文本用例")

	const text = "板的边界条件取四边简支"
	if _, err := runREPLSvc(t, svc, "/use conv_bare\n"+text+"\n"); err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}

	msgs := f.msgs["conv_bare"]
	if len(msgs) != 1 {
		t.Fatalf("会话里有 %d 条消息，期望 1 条", len(msgs))
	}
	m := msgs[0]
	if m.Text != text {
		t.Errorf("正文是 %q，期望 %q", m.Text, text)
	}
	if m.SenderID != conversation.ReservedSenderUser {
		t.Errorf("sender 是 %q，期望 %q", m.SenderID, conversation.ReservedSenderUser)
	}
	if m.Kind != conversation.KindUser {
		t.Errorf("kind 是 %q，期望 %q", m.Kind, conversation.KindUser)
	}
	// 人不是模型：裸文本发言一律不带脑子。
	if !m.SenderBrainEmpty() {
		t.Errorf("裸文本发言竟然带了脑子: provider=%q model=%q effort=%q",
			m.SenderProvider, m.SenderModel, m.SenderEffort)
	}
}

// TestBareTextAndSayStoreTheSameMessage 是「两条路径等价」在真发得出去时的版本。
// 输出里带随机会话 id 与 seq，没法逐字节比，所以比落库的内容。
func TestBareTextAndSayStoreTheSameMessage(t *testing.T) {
	const text = "同一句话，两条路径"

	svc, f := newTestService(t)
	f.seedConversation("conv_aaa", "裸文本那条路")
	f.seedConversation("conv_bbb", "/say 那条路")

	if _, err := runREPLSvc(t, svc, "/use conv_aaa\n"+text+"\n"); err != nil {
		t.Fatalf("裸文本一轮 Run 失败: %v", err)
	}
	if _, err := runREPLSvc(t, svc, "/use conv_bbb\n/say "+text+"\n"); err != nil {
		t.Fatalf("/say 一轮 Run 失败: %v", err)
	}

	bare := f.msgs["conv_aaa"]
	say := f.msgs["conv_bbb"]
	if len(bare) != 1 || len(say) != 1 {
		t.Fatalf("两条路径各应落 1 条消息，实际 %d 与 %d", len(bare), len(say))
	}
	if bare[0].Text != say[0].Text {
		t.Errorf("正文不一致: 裸文本 %q vs /say %q", bare[0].Text, say[0].Text)
	}
	if bare[0].SenderID != say[0].SenderID {
		t.Errorf("sender 不一致: 裸文本 %q vs /say %q", bare[0].SenderID, say[0].SenderID)
	}
	if bare[0].Kind != say[0].Kind {
		t.Errorf("kind 不一致: 裸文本 %q vs /say %q", bare[0].Kind, say[0].Kind)
	}
}

// TestSlashSayEscapesLeadingSlash 锁住 /say 在裸文本能发言之后保留下来的唯一理由：
// 它是转义出口，用来发以 / 开头的字面量。
func TestSlashSayEscapesLeadingSlash(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_esc", "转义用例")

	if _, err := runREPLSvc(t, svc, "/use conv_esc\n/say /foo\n"); err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}

	msgs := f.msgs["conv_esc"]
	if len(msgs) != 1 {
		t.Fatalf("会话里有 %d 条消息，期望 1 条", len(msgs))
	}
	if msgs[0].Text != "/foo" {
		t.Errorf("正文是 %q，期望字面量 %q", msgs[0].Text, "/foo")
	}
}

// TestUnknownSlashCommandIsNotAMessage 是转义用例的反面：
// 直接敲 /foo 走的是命令表，查不到就提示未知命令，**不能**被当成发言存起来。
// 少了这一条，「所有输入都发言」的实现也能让上一条用例通过。
func TestUnknownSlashCommandIsNotAMessage(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_unk", "未知命令用例")

	if _, err := runREPLSvc(t, svc, "/use conv_unk\n/foo\n"); err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}
	if n := len(f.msgs["conv_unk"]); n != 0 {
		t.Errorf("未知命令竟然被存成了 %d 条消息", n)
	}
}

// TestBlankLinesPostNothing 锁住空行仍然是 no-op —— 裸文本发言不得把空行也发出去。
// 只含空白的行同样算空行：dispatch 是先 TrimSpace 再判断的。
func TestBlankLinesPostNothing(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_blank", "空行用例")

	if _, err := runREPLSvc(t, svc, "/use conv_blank\n\n   \n\t\n"); err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}
	if n := len(f.msgs["conv_blank"]); n != 0 {
		t.Errorf("空行竟然被存成了 %d 条消息", n)
	}
}

// TestLogShowsRecordedBrain 锁住 /log 显示的是**消息里记着的**脑子。
func TestLogShowsRecordedBrain(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_log", "脑子渲染用例")

	seedBrainMessage(t, f, "conv_log", "ag_alpha", conversation.KindAgent, "三段齐全的一句", "claude", "opus", "high")
	seedBrainMessage(t, f, "conv_log", "ag_mid", conversation.KindAgent, "没有 effort 的一句", "grok", "grok-4.6", "")
	f.seedMessage(t, "conv_log", conversation.ReservedSenderUser, conversation.KindUser, "人说的一句")

	out, err := runREPLSvc(t, svc, "/use conv_log\n/log\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}

	if line := lineContaining(t, out, "三段齐全的一句"); !strings.Contains(line, "claude/opus/high") {
		t.Errorf("三段齐全的消息那一行没有显示完整的脑子: %s", line)
	}
	// 缺 effort 时只有那一段是占位符，不是整格空 ——
	// 「这个 provider 没有 effort 这个概念」和「脑子没记」是两回事。
	if line := lineContaining(t, out, "没有 effort 的一句"); !strings.Contains(line, "grok/grok-4.6/-") {
		t.Errorf("缺 effort 的消息那一行脑子渲染不对: %s", line)
	}
	// user 消息没有脑子可记，那一格是占位符，不该冒出任何 provider 名。
	userLine := lineContaining(t, out, "人说的一句")
	for _, leaked := range []string{"claude", "grok", "opus"} {
		if strings.Contains(userLine, leaked) {
			t.Errorf("user 消息那一行漏出了 %q: %s", leaked, userLine)
		}
	}
}

// TestWhoShowsConfigBrain 锁住 /who 显示的是**配置里当前的**脑子。
func TestWhoShowsConfigBrain(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_who", "成员脑子用例")
	f.seedParticipant(t, "conv_who", "ag_alpha")
	// ag_zeta 在测试配置里 effort 是空的，用来覆盖「只缺一段」那一支。
	f.seedParticipant(t, "conv_who", "ag_zeta")
	// 这个 agent 不在配置里：它是被删掉但仍留在旧会话里的成员，必须仍然显示、不报错。
	f.seedParticipant(t, "conv_who", "ag_gone")

	out, err := runREPLSvc(t, svc, "/use conv_who\n/who\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}

	if line := lineContaining(t, out, "ag_alpha"); !strings.Contains(line, "pv_alpha/mod-one/eff-one") {
		t.Errorf("ag_alpha 那一行没有显示配置里的脑子: %s", line)
	}
	if line := lineContaining(t, out, "ag_zeta"); !strings.Contains(line, "pv_zeta/mod-three/-") {
		t.Errorf("ag_zeta 那一行脑子渲染不对（它在配置里没有 effort）: %s", line)
	}
	goneLine := lineContaining(t, out, "ag_gone")
	for _, leaked := range []string{"pv_alpha", "pv_mid", "pv_zeta", "mod-one", "mod-two", "mod-three"} {
		if strings.Contains(goneLine, leaked) {
			t.Errorf("配置里不存在的成员那一行漏出了 %q: %s", leaked, goneLine)
		}
	}
}

// TestLogAndWhoReadBrainFromDifferentSources 是这次改动真正要防的那个陷阱。
//
// 同一个 agent：消息里记着旧脑子，配置里写着新脑子。
// /log 必须显示旧的（历史不能被追溯性重写），/who 必须显示新的（那是现状）。
// 两者渲染成同一个值，就说明有人把脑子当成 participant 的属性在读了。
func TestLogAndWhoReadBrainFromDifferentSources(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_split", "两个数据源用例")
	f.seedParticipant(t, "conv_split", "ag_alpha")

	// 配置里 ag_alpha 是 pv_alpha/mod-one/eff-one（见 newTestREPLConfig）。
	// 这条消息记的是它换脑子之前说的话。
	const oldBrain = "old_pv/old_mod/low"
	seedBrainMessage(t, f, "conv_split", "ag_alpha", conversation.KindAgent,
		"换脑子之前说的话", "old_pv", "old_mod", "low")

	const configBrain = "pv_alpha/mod-one/eff-one"

	logOut, err := runREPLSvc(t, svc, "/use conv_split\n/log\n")
	if err != nil {
		t.Fatalf("/log 一轮 Run 失败: %v", err)
	}
	whoOut, err := runREPLSvc(t, svc, "/use conv_split\n/who\n")
	if err != nil {
		t.Fatalf("/who 一轮 Run 失败: %v", err)
	}

	if !strings.Contains(logOut, oldBrain) {
		t.Errorf("/log 没有显示消息里记着的旧脑子 %q；输出:\n%s", oldBrain, logOut)
	}
	if strings.Contains(logOut, configBrain) {
		t.Errorf("/log 显示了配置里的脑子 %q —— 历史被追溯性重写了；输出:\n%s", configBrain, logOut)
	}
	if !strings.Contains(whoOut, configBrain) {
		t.Errorf("/who 没有显示配置里当前的脑子 %q；输出:\n%s", configBrain, whoOut)
	}
	if strings.Contains(whoOut, oldBrain) {
		t.Errorf("/who 显示了消息里的旧脑子 %q —— 它读错了数据源；输出:\n%s", oldBrain, whoOut)
	}
}
