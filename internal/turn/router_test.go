package turn

// 本文件覆盖 10a 契约 §3.3 与 10_STEP7_SPEC.md §6 的 Router：纯函数性
// （同一批消息 + 同一收件人，Select 与 Render 各自调两次结果相同；源码
// 扫描不出现 time.Now / rand. / time.Since）、增量模式复用 filterDelta 的
// 两条过滤规则、bootstrap 模式的三条规则（标题、全部 user 消息、每个其他
// 活跃成员只留窗口内最后一条）、bootstrap 不等于全量历史、thought 永不
// 进正文。

import (
	"os"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
)

// routerFixtureMessages 构造一批有代表性的消息：两条用户消息；一个「已经
// 说了三轮」的成员 bob（bootstrap 只该留第三轮）；一条系统面包屑（两种
// 模式都不该看见）；以及 selfID 自己发的一条消息（增量模式要跳过，
// bootstrap 模式也不该把它当成「其他成员」纳入）。bob 第三轮那条消息带了
// Thought，用来验证「thought 永不进正文」。
func routerFixtureMessages(selfID string) []conversation.Message {
	return []conversation.Message{
		{Seq: 1, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "用户问题一"},
		{Seq: 2, SenderID: "bob", Kind: conversation.KindAgent, Text: "bob 第一轮，bootstrap 不该看见这条"},
		{Seq: 3, SenderID: "bob", Kind: conversation.KindAgent, Text: "bob 第二轮，bootstrap 不该看见这条"},
		{Seq: 4, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "用户问题二"},
		{Seq: 5, SenderID: "bob", Kind: conversation.KindAgent, Text: "bob 第三轮，bootstrap 只该留这条", Thought: "bob 的私下斟酌，永远不该进正文"},
		{Seq: 6, SenderID: conversation.ReservedSenderSystem, Kind: conversation.KindSystem, Text: "系统面包屑，两种模式都不该看见"},
		{Seq: 7, SenderID: selfID, Kind: conversation.KindAgent, Text: "自己上一轮说的话"},
	}
}

// TestRouterSelectAndRenderAreDeterministic 覆盖验收表「Router 纯函数」：
// 同一批消息、同一收件人，Select 与 Render 各自调两次，结果必须逐字节 /
// 逐元素相同。bootstrap 与增量两种模式各测一遍。
func TestRouterSelectAndRenderAreDeterministic(t *testing.T) {
	ms := routerFixtureMessages("alice")
	ac := config.AgentConfig{DisplayName: "Alice"}
	r := NewRouter(false)

	for _, bootstrap := range []bool{true, false} {
		sel1 := r.Select(ms, ac, "alice", bootstrap)
		sel2 := r.Select(ms, ac, "alice", bootstrap)
		if len(sel1) != len(sel2) {
			t.Fatalf("bootstrap=%v: 两次 Select 长度不同 %d vs %d", bootstrap, len(sel1), len(sel2))
		}
		for i := range sel1 {
			if sel1[i].Seq != sel2[i].Seq {
				t.Errorf("bootstrap=%v: 第 %d 条 Seq 不同 %d vs %d", bootstrap, i, sel1[i].Seq, sel2[i].Seq)
			}
		}

		rend1 := r.Render(sel1, "会话标题", bootstrap)
		rend2 := r.Render(sel2, "会话标题", bootstrap)
		if rend1 != rend2 {
			t.Fatalf("bootstrap=%v: 两次 Render 不同:\n第一次: %q\n第二次: %q", bootstrap, rend1, rend2)
		}
	}
}

// TestRouterSourceNeverCallsClockOrRandom 是 10a 契约明写的 conformance
// 测试：源码扫描 router.go，断言不出现 time.Now、rand.、time.Since。照既有
// conformance_test.go / render_test.go 的写法，用带括号或带点号的形式匹配，
// 避免命中本测试自己文档注释里提到的这几个名字。
func TestRouterSourceNeverCallsClockOrRandom(t *testing.T) {
	path := turnSourceFile(t, "router.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", path, err)
	}
	src := string(data)
	forbidden := []string{"time.Now(", "rand.", "time.Since("}
	for _, needle := range forbidden {
		if strings.Contains(src, needle) {
			t.Errorf("%s 里出现了 %q：Router 必须是纯函数，函数体里不得含时钟或随机源（10_STEP7_SPEC.md 第 1 节）", path, needle)
		}
	}
}

// TestRouterIncrementalSkipsSystemAndSelf 覆盖增量模式（bootstrap=false）
// 复用 filterDelta 的两条规则：跳过 KindSystem；跳过自己上一轮的发言。
func TestRouterIncrementalSkipsSystemAndSelf(t *testing.T) {
	ms := routerFixtureMessages("alice")
	r := NewRouter(false)
	selected := r.Select(ms, config.AgentConfig{}, "alice", false)

	for _, m := range selected {
		if m.Kind == conversation.KindSystem {
			t.Errorf("增量模式的 Select 结果里出现了 KindSystem: %+v", m)
		}
		if m.SenderID == "alice" {
			t.Errorf("增量模式必须跳过自己上一轮的发言，实际出现了: %+v", m)
		}
	}
}

// TestRouterBootstrapFollowsThreeRules 覆盖 10_STEP7_SPEC.md §6.3 bootstrap
// 模式的三条规则：渲染结果含会话标题；含全部 KindUser 消息；每个其他活跃
// 成员只含窗口内最后一条 KindAgent 消息（bob 说了三轮，只有第三轮该留下）。
// 另外断言 bootstrap 的结果不等于全量历史的渲染——这是 04 §4「不要把整个
// 对话倒给它」的直接检验，也是 Step 6 最难看的那处被修掉的证明。
func TestRouterBootstrapFollowsThreeRules(t *testing.T) {
	// selfID 用一个全新加入、自己从没说过话的成员 carol：routerFixtureMessages
	// 末尾那条「自己上一轮说的话」在这里就是 senderID=carol 的一条
	// KindAgent——它既不是 KindUser，也不算「其他活跃成员」的发言（bootstrap
	// 规则第 3 条只挑「其他」成员），所以不该出现在结果里。这与增量模式的
	// skipSelf 是两码事：bootstrap 规则本身没有 skipSelf 这个概念，selfID
	// 自己的消息单纯地哪条规则都不占。
	const selfID = "carol"
	const title = "会话标题：测试用"
	ms := routerFixtureMessages(selfID)

	r := NewRouter(false)
	selected := r.Select(ms, config.AgentConfig{}, selfID, true)
	rendered := r.Render(selected, title, true)

	if !strings.Contains(rendered, title) {
		t.Errorf("bootstrap 渲染结果里没有会话标题，实际: %q", rendered)
	}
	if !strings.Contains(rendered, "用户问题一") || !strings.Contains(rendered, "用户问题二") {
		t.Errorf("bootstrap 应当含全部 KindUser 消息，实际: %q", rendered)
	}
	if strings.Contains(rendered, "bob 第一轮") || strings.Contains(rendered, "bob 第二轮") {
		t.Errorf("bootstrap 只该留 bob 窗口内最后一条 KindAgent 消息，实际混进了更早的几条: %q", rendered)
	}
	if !strings.Contains(rendered, "bob 第三轮") {
		t.Errorf("bootstrap 应当留下 bob 窗口内最后一条 KindAgent 消息，实际: %q", rendered)
	}
	if strings.Contains(rendered, "系统面包屑") {
		t.Errorf("bootstrap 不该含 KindSystem 的内容，实际: %q", rendered)
	}
	if strings.Contains(rendered, "自己上一轮说的话") {
		t.Errorf("bootstrap 不该含 selfID 自己发的消息，实际: %q", rendered)
	}

	// 不等于全量历史：把同一批消息按 Step 6「把整场对话重放一遍」那种做法
	// （RenderDelta，fresh 时 skipSelf=false）渲染，与 bootstrap 的结果比较，
	// 必须不同——如果相等，说明挑选规则形同虚设。
	full := RenderDelta(ms, config.AgentConfig{}, selfID, false)
	if rendered == full {
		t.Fatal("bootstrap 的渲染结果与全量历史重放完全相同，04 第 4 节要求的正是二者不同")
	}
	if !strings.Contains(full, "bob 第一轮") {
		t.Fatal("前置条件不成立：全量历史重放应当含 bob 更早的发言，否则上面那条「不相等」的比较没有意义")
	}
}

// TestRouterThoughtNeverEntersBody 覆盖 §6.2「thought 的路由开关」：Step 7
// 恒传 routeThought=false，任何一条消息的 Thought 字段都不该出现在渲染
// 结果里，无论 bootstrap 还是增量模式。
func TestRouterThoughtNeverEntersBody(t *testing.T) {
	const thoughtText = "bob 的私下斟酌，永远不该进正文"
	ms := routerFixtureMessages("alice")
	r := NewRouter(false)

	for _, bootstrap := range []bool{true, false} {
		selected := r.Select(ms, config.AgentConfig{}, "alice", bootstrap)
		rendered := r.Render(selected, "标题", bootstrap)
		if strings.Contains(rendered, thoughtText) {
			t.Errorf("bootstrap=%v: 渲染结果里出现了 thought 的内容，实际: %q", bootstrap, rendered)
		}
	}
}
