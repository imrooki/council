package turn

// render.go 的测试面：RenderDelta 是本包唯一的纯函数边界（09 H 节），
// 而缓存前缀稳定性（D-4）整个建立在这个函数「同一批消息两次调用逐字节相同」
// 这条结构性保证上。09 render.go 一节原话：「这是结构性的，不是靠自觉」——
// 本文件的用例就是把这句话变成可执行的断言，而不是重新描述一遍实现在做什么。

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
)

func sampleMessages() []conversation.Message {
	// Seq 故意不连续（3、5、8）：过滤规则不许依赖 seq 连号，也不许依赖切片下标。
	return []conversation.Message{
		{Seq: 3, SenderID: "user", Kind: conversation.KindUser, Text: "问题一"},
		{Seq: 5, SenderID: "alice", Kind: conversation.KindAgent, Text: "回答一"},
		{Seq: 6, SenderID: conversation.ReservedSenderSystem, Kind: conversation.KindSystem, Text: "系统面包屑，不该出现在正文里"},
		{Seq: 8, SenderID: "zulu", Kind: conversation.KindAgent, Text: "回答二"},
	}
}

// TestRenderDeltaDeterministic 钉住 D-4：同一批消息两次调用 RenderDelta 必须
// 逐字节相同。这不是「大概率相同」，是要求 Go 的 == 比较为真——字符串比较
// 本身就是逐字节的，选它做断言就是选中了最严格的那种相等。
func TestRenderDeltaDeterministic(t *testing.T) {
	ms := sampleMessages()
	ac := config.AgentConfig{DisplayName: "Zulu", Provider: "grok", Role: "只做数值验算"}

	a := RenderDelta(ms, ac, "zulu", true)
	b := RenderDelta(ms, ac, "zulu", true)
	if a != b {
		t.Fatalf("同一批消息两次渲染不相同:\n第一次: %q\n第二次: %q", a, b)
	}
}

// TestRenderDeltaIgnoresToParameter 覆盖 render.go 的明文注释：to 留在冻结
// 签名里只是为了不必改 Step 7 的 Router 接口，函数体里故意 `_ = to`，
// DisplayName / Role 都不参与渲染。这里换两个 DisplayName 完全不同的 to，
// 断言渲染结果不变——如果将来有人手滑把 to.DisplayName 拼进正文，
// 这条用例会在这里而不是在生产环境里第一次发现「换个显示名就打断缓存前缀」。
func TestRenderDeltaIgnoresToParameter(t *testing.T) {
	ms := sampleMessages()
	acA := config.AgentConfig{DisplayName: "甲", Role: "role-a"}
	acB := config.AgentConfig{DisplayName: "乙又长又不同的显示名", Role: "role-b-完全不同"}

	a := RenderDelta(ms, acA, "zulu", true)
	b := RenderDelta(ms, acB, "zulu", true)
	if a != b {
		t.Fatalf("换一个 to 参数（DisplayName/Role 完全不同）渲染结果变了:\nA: %q\nB: %q", a, b)
	}
}

// TestRenderDeltaSkipsKindSystem 覆盖 04 第 2 节：内部事件与 provider 诊断
// 不应当路由给任何 agent。sampleMessages 里 seq=6 的那条是 KindSystem，
// 正文里必须一个字都不出现。
func TestRenderDeltaSkipsKindSystem(t *testing.T) {
	ms := sampleMessages()
	ac := config.AgentConfig{}
	got := RenderDelta(ms, ac, "zulu", false)
	if strings.Contains(got, "系统面包屑") {
		t.Errorf("KindSystem 的内容泄漏进了正文: %q", got)
	}
}

// TestRenderDeltaSkipSelf 覆盖 D 节判定点二的推导结果：skipSelf 为真时，
// 这个 agent 自己过去说过的话必须从正文里消失（resume 那一轮，CLI 那边已经
// 有这些话了，重放既费钱又让模型看见自己的回声）；skipSelf 为假时
// （新会话那一轮）必须重放，包括它自己过去的发言。
func TestRenderDeltaSkipSelf(t *testing.T) {
	ms := sampleMessages() // 含 alice 在 seq=5 的发言
	ac := config.AgentConfig{}

	withSkip := RenderDelta(ms, ac, "alice", true)
	if strings.Contains(withSkip, "回答一") {
		t.Errorf("skipSelf=true 时不该出现 alice 自己过去的发言，实际: %q", withSkip)
	}
	if !strings.Contains(withSkip, "回答二") {
		t.Errorf("skipSelf=true 时别人（zulu）的发言应当仍在，实际: %q", withSkip)
	}

	withoutSkip := RenderDelta(ms, ac, "alice", false)
	if !strings.Contains(withoutSkip, "回答一") {
		t.Errorf("skipSelf=false（新会话）时应当重放 alice 自己过去的发言，实际: %q", withoutSkip)
	}
}

// TestRenderDeltaEmptyVisibleReturnsEmptyString 覆盖「过滤完一条都不剩」的
// 边界：全部消息都是 KindSystem，或全部都是这个 agent 自己说的且 skipSelf，
// 结果必须是空字符串，而不是例如一个只含分隔符的字符串——runner.go 用
// `len(visible) == 0` 判 ErrNoDelta，若这里返回非空但无意义的字符串，
// 上层的空 delta 判断就会被绕过。
func TestRenderDeltaEmptyVisibleReturnsEmptyString(t *testing.T) {
	onlySystem := []conversation.Message{
		{Seq: 1, SenderID: conversation.ReservedSenderSystem, Kind: conversation.KindSystem, Text: "x"},
	}
	if got := RenderDelta(onlySystem, config.AgentConfig{}, "alice", false); got != "" {
		t.Errorf("全部是 KindSystem 时应当渲染成空串，实际 %q", got)
	}

	onlySelf := []conversation.Message{
		{Seq: 1, SenderID: "alice", Kind: conversation.KindAgent, Text: "自己说的"},
	}
	if got := RenderDelta(onlySelf, config.AgentConfig{}, "alice", true); got != "" {
		t.Errorf("skipSelf 且只有自己的发言时应当渲染成空串，实际 %q", got)
	}
}

// TestFilterDeltaMatchesRenderDelta 覆盖 render.go 的明文要求：filterDelta
// 是 RenderDelta 与 Runner 填 Delivered 共用的同一个过滤函数。这里不是重新
// 断言两处「恰好算出一样的结果」（那是同一份代码，永远一样，测了等于没测），
// 而是断言 filterDelta 返回的可见消息集合，其 Seq 与文本，与从 RenderDelta
// 渲染结果里能读出来的行一一对应——这是防「两处各自维护一份过滤逻辑、
// 某次改动只改了一处」的回归网，不是防「同一个函数调用两次」。
func TestFilterDeltaMatchesRenderDelta(t *testing.T) {
	ms := sampleMessages()
	const selfID = "zulu"
	const skipSelf = false

	visible := filterDelta(ms, selfID, skipSelf)
	rendered := RenderDelta(ms, config.AgentConfig{}, selfID, skipSelf)

	renderedLines := strings.Split(rendered, "\n\n")
	if len(renderedLines) != len(visible) {
		t.Fatalf("filterDelta 给出 %d 条可见消息，RenderDelta 渲染出 %d 行，应当一一对应\nvisible=%+v\nrendered=%q",
			len(visible), len(renderedLines), visible, rendered)
	}
	for i, m := range visible {
		want := m.SenderID + ": " + m.Text
		if renderedLines[i] != want {
			t.Errorf("第 %d 行 = %q，期望 %q", i, renderedLines[i], want)
		}
		if m.Kind == conversation.KindSystem {
			t.Errorf("filterDelta 的可见集合里出现了 KindSystem: %+v", m)
		}
	}
}

// TestRenderDeltaRetryIsByteIdentical 是 D-4 最贴近真实使用场景的一条：
// 首发失败后（游标没推），追加两条新消息再重发，第一次发送的字节必须与
// 「重新扫描到同一批已投递内容」时逐字节相同——04 第 6 节「重发必须是同一批
// 内容、同样的顺序」在这里被当成可执行断言，而不是停留在注释里的一句话。
func TestRenderDeltaRetryIsByteIdentical(t *testing.T) {
	first := sampleMessages()
	firstRender := RenderDelta(first, config.AgentConfig{}, "alice", false)

	// 模拟「首发失败，游标没推，之后又有新消息进来」：把 first 原样保留在切片
	// 前部，只在末尾追加，验证前缀部分重新渲染仍然逐字节相同。
	extended := append(append([]conversation.Message{}, first...),
		conversation.Message{Seq: 9, SenderID: "user", Kind: conversation.KindUser, Text: "追加的新问题"},
	)
	secondRenderPrefixPart := RenderDelta(extended[:len(first)], config.AgentConfig{}, "alice", false)

	if firstRender != secondRenderPrefixPart {
		t.Fatalf("同一批历史消息重新渲染的前缀部分变了（打断缓存前缀不会报错，只会更贵）:\n首发: %q\n重发前缀: %q",
			firstRender, secondRenderPrefixPart)
	}
}

// TestRenderDeltaSourceNeverCallsTimeNow 是结构性保证的最后一道闸：
// render.go 的顶部注释明写「函数体里不得含 time.Now」，K 节测试矩阵原话
// 「扫源码断言 render.go 里不出现 time.Now」。靠人读代码复核这条容易在下一次
// 改动时失守，所以直接扫文件文本。
func TestRenderDeltaSourceNeverCallsTimeNow(t *testing.T) {
	path := turnSourceFile(t, "render.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", path, err)
	}
	// 用 "time.Now(" 而不是 "time.Now"：render.go 自己的文档注释里就写着
	// 「函数体里不得含 time.Now」这句话本身，裸子串扫描会把这行注释当成命中，
	// 那是本测试的假阳性，不是实现的问题。真正要挡的是被调用（带括号），
	// 一个字符串常量或注释提到这个名字不构成违反。
	if strings.Contains(string(data), "time.Now(") {
		t.Errorf("%s 里出现了对 time.Now() 的调用，这会打破 RenderDelta 的纯函数保证（09 render.go 一节）", path)
	}
}

// turnSourceFile 返回 internal/turn 包内某个源文件的绝对路径，供源码扫描类
// 用例使用。用 runtime.Caller 定位本测试文件自己的路径、取其目录，而不是
// 硬编码相对路径，是为了不管 go test 从哪个工作目录被拉起都能定位到同一份
// 源文件（本文件与 render.go 同目录，这是 package turn 的白盒测试文件必然
// 成立的前提，不是额外假设）。
func turnSourceFile(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller 失败，无法定位 internal/turn 包目录")
	}
	return filepath.Join(filepath.Dir(file), name)
}
