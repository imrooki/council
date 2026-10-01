package turn

// recipient_test.go 覆盖 docs/11d_STEP8C_CONTRACT.md 第 2.8 节第 2/3/4/5 条：
// 导入图闸（ParseMentions 不得出现在 internal/turn 源码里）、删掉 mention
// 加投分支之后的正面行为变更断言、缩窄前提的逐字节 golden，以及
// recipientOK 在 bootstrap 与增量两条路径上的收窄。
//
// 背对背纪律：recipientOK 以及 selectBootstrap / filterDelta 里那一句
// `if !recipientOK(m, selfID) { continue }` 是本次 8c-1 正在改的代码。
// 本文件的断言全部来自契约 2.2 / 2.3 节的文字，不依赖某一份具体实现的
// 写法——尤其是第 4 条的 golden fixture，刻意不含任何 "@token"，这样
// 无论 mention 加投分支删没删、recipientOK 加没加，结果按契约保证都应当
// 逐字节相同。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
)

// ---------------------------------------------------------------------------
// 2.8 第 2 条：导入图闸
// ---------------------------------------------------------------------------

// TestTurnSourceNeverCallsParseMentions 覆盖 2.8 第 2 条：internal/turn 的
// 非测试源码里不许出现 ParseMentions 这个名字。11d 1.2 节的裁决是
// “internal/turn 不解析 mention，一个字都不解析”，解析器只落在
// internal/repl；本用例是这道结构闸的看门人，防的是将来有人把解析器又
// 抄一份进 turn（而不是防 import，import 环本身是编译期保证，不用测）。
//
// 复用 conformance_test.go 已经建好的 turnPackageSourceDir，与
// TestSessionIDPolicyNeverAppearsInOrchestrationLayer 同一套定位与扫描逻辑。
func TestTurnSourceNeverCallsParseMentions(t *testing.T) {
	dir := turnPackageSourceDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录 %s 失败: %v", dir, err)
	}
	const needle = "ParseMentions"
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", path, err)
		}
		checked++
		if strings.Contains(string(data), needle) {
			t.Errorf("%s 里出现了 %s：internal/turn 不解析 mention，一个字都不解析"+
				"（11d 契约 1.2 节 / 2.8 第 2 条），解析器只能落在 internal/repl", path, needle)
		}
	}
	if checked == 0 {
		t.Fatal("一个非测试的 .go 文件都没扫到，这条用例本身没有意义，检查 turnPackageSourceDir 是否指对了目录")
	}
}

// ---------------------------------------------------------------------------
// 2.8 第 3 条：行为变更的正面断言（用例名必须含 BehaviorChange，不许叫 FixesBug）
// ---------------------------------------------------------------------------

// TestSelectBootstrapMentionAddBehaviorChange 覆盖 2.8 第 3 条：一条
// KindAgent、正文含 "@gk" 的消息，在删掉 mention 加投分支之前会经那个
// 分支被收进 gk 的 bootstrap；删掉之后不会。11d 2.2 节明写这是一次
// “少投方向”的语义收窄，不是修 bug，所以用例名带 BehaviorChange。
//
// bob 发两条消息：seq=2 提到 "@gk" 但不是 bob 窗口内最后一条；seq=3 是
// bob 窗口内最后一条，且不提任何人。这样拆成两条是为了把“因为被提到而
// 收”与“因为是窗口内最后一条发言而收”这两条规则分开——如果只用一条
// bob 发言同时占两条规则，两条规则会同时命中，测不出契约要的这次删除
// （2.8 第 3 条的意图）。
func TestSelectBootstrapMentionAddBehaviorChange(t *testing.T) {
	const selfID = "gk"
	ms := []conversation.Message{
		{Seq: 1, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "开场"},
		{Seq: 2, SenderID: "bob", Kind: conversation.KindAgent, Text: "问一下 @gk 这个对不对，但这不是我最后一条"},
		{Seq: 3, SenderID: "bob", Kind: conversation.KindAgent, Text: "这是 bob 窗口内最后一条，没提任何人"},
	}

	selected := selectBootstrap(ms, selfID)
	if containsSeq(selected, 2) {
		t.Fatalf("gk 的 bootstrap 结果里出现了 seq=2：11d 2.2 节裁决删掉 mention 加投分支之后，"+
			"仅仅被 @提到、且不是发送者窗口内最后一条的消息不该再进 bootstrap。完整结果: %+v", selected)
	}
	if !containsSeq(selected, 3) {
		t.Fatalf("gk 的 bootstrap 结果里应当仍含 bob 窗口内最后一条（seq=3），实际: %+v", selected)
	}
}

// ---------------------------------------------------------------------------
// 2.8 第 4 条：缩窄前提的逐字节 golden
// ---------------------------------------------------------------------------

// noMentionFixture 造一批 Recipients 全空、且正文不含任何 "@token" 的消息。
//
// fixture 里为什么不能有 "@token"：第 3 条钉住的行为变更（mention 加投
// 分支删除）是本片有意做的语义收窄（11d 2.2 节），一旦 fixture 里出现
// "@xxx"，本条 golden 就会在“改动前后结果不同”与“golden 要求逐字节相同”
// 之间自相矛盾。缩窄到这个前提之后，recipientOK 恒为 true（Recipients
// 为空）、被删掉的 mention 加投分支对这批消息也从来不是 no-op 以外的结果，
// 挑选结果理应逐字节相同。
func noMentionFixture() []conversation.Message {
	return []conversation.Message{
		{Seq: 1, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "问题一"},
		{Seq: 2, SenderID: "bob", Kind: conversation.KindAgent, Text: "bob 第一轮，bootstrap 不该看见这条"},
		{Seq: 3, SenderID: "bob", Kind: conversation.KindAgent, Text: "bob 窗口内最后一条"},
		{Seq: 4, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "问题二"},
		{Seq: 5, SenderID: conversation.ReservedSenderSystem, Kind: conversation.KindSystem, Text: "系统面包屑"},
	}
}

// TestSelectBootstrapByteIdenticalWithoutMentions 覆盖 2.8 第 4 条的 bootstrap
// 一半：挑选出的 Seq 集合与 Router.Render 渲染出的正文，在这批不含
// "@token" 的消息上必须与本片改动之前逐字节相同。
func TestSelectBootstrapByteIdenticalWithoutMentions(t *testing.T) {
	const selfID = "carol"
	const title = "会话标题"
	ms := noMentionFixture()

	r := NewRouter(false)
	selected := r.Select(ms, config.AgentConfig{}, selfID, true)

	wantSeqs := []int64{1, 3, 4}
	if len(selected) != len(wantSeqs) {
		t.Fatalf("Select 挑出 %d 条，期望 %d 条: %+v", len(selected), len(wantSeqs), selected)
	}
	for i, want := range wantSeqs {
		if selected[i].Seq != want {
			t.Fatalf("第 %d 条 Seq = %d，期望 %d；完整结果: %+v", i, selected[i].Seq, want, selected)
		}
	}

	rendered := r.Render(selected, title, true)
	want := title + "\n\n" +
		"user: 问题一" + "\n\n" +
		"bob: bob 窗口内最后一条" + "\n\n" +
		"user: 问题二"
	if rendered != want {
		t.Fatalf("渲染结果与改动前不逐字节相同:\n实际: %q\n期望: %q", rendered, want)
	}
}

// TestFilterDeltaByteIdenticalWithoutMentions 覆盖 2.8 第 4 条的增量一半：
// filterDelta 的可见集合、以及 RenderDelta 渲染出的正文，在这批消息上必须
// 与本片改动之前逐字节相同（recipientOK 恒为 true，等价于改动前只有
// KindSystem/skipSelf 两条过滤——filterDelta 不做“每成员最后一条”那种
// 收窄，bob 的两条消息都该在）。
func TestFilterDeltaByteIdenticalWithoutMentions(t *testing.T) {
	const selfID = "carol"
	ms := noMentionFixture()

	visible := filterDelta(ms, selfID, false)
	wantSeqs := []int64{1, 2, 3, 4}
	if len(visible) != len(wantSeqs) {
		t.Fatalf("filterDelta 挑出 %d 条，期望 %d 条: %+v", len(visible), len(wantSeqs), visible)
	}
	for i, want := range wantSeqs {
		if visible[i].Seq != want {
			t.Fatalf("第 %d 条 Seq = %d，期望 %d", i, visible[i].Seq, want)
		}
	}

	rendered := RenderDelta(ms, config.AgentConfig{}, selfID, false)
	want := "user: 问题一" + "\n\n" +
		"bob: bob 第一轮，bootstrap 不该看见这条" + "\n\n" +
		"bob: bob 窗口内最后一条" + "\n\n" +
		"user: 问题二"
	if rendered != want {
		t.Fatalf("RenderDelta 渲染结果与改动前不逐字节相同:\n实际: %q\n期望: %q", rendered, want)
	}
}

// ---------------------------------------------------------------------------
// 2.8 第 5 条：recipientOK 收窄
// ---------------------------------------------------------------------------

// TestRecipientOKNarrowsBothPaths 覆盖 2.8 第 5 条：一条 Recipients=["gk2"]
// 的消息，gk 的 bootstrap 与增量两条路径都挑不到，gk2 两条路径都挑得到。
func TestRecipientOKNarrowsBothPaths(t *testing.T) {
	ms := []conversation.Message{
		{Seq: 1, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser,
			Text: "只给 gk2 看", Recipients: []string{"gk2"}},
	}

	if got := selectBootstrap(ms, "gk"); containsSeq(got, 1) {
		t.Errorf("gk 的 bootstrap 不该挑到定向给 gk2 的消息，实际: %+v", got)
	}
	if got := selectBootstrap(ms, "gk2"); !containsSeq(got, 1) {
		t.Errorf("gk2 的 bootstrap 应当挑到定向给它自己的消息，实际: %+v", got)
	}

	if got := filterDelta(ms, "gk", false); containsSeq(got, 1) {
		t.Errorf("gk 的增量路径不该挑到定向给 gk2 的消息，实际: %+v", got)
	}
	if got := filterDelta(ms, "gk2", false); !containsSeq(got, 1) {
		t.Errorf("gk2 的增量路径应当挑到定向给它自己的消息，实际: %+v", got)
	}
}

// containsSeq 是本文件与 bootstrap_kinds_test.go 共用的小工具：判断一批
// 消息里是否含某个 Seq，避免每条用例都重新写一遍线性扫描。
func containsSeq(ms []conversation.Message, seq int64) bool {
	for _, m := range ms {
		if m.Seq == seq {
			return true
		}
	}
	return false
}
