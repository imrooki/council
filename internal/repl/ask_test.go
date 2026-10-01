package repl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/turn"
)

// 本文件覆盖 internal/repl 新增的两条命令 /ask 与 /session，以及它们共用的
// writeAskRecord / writeAskMeter / askCause 这三个纯展示 / 纯翻译函数。
//
// 09a 契约「不属于契约面的东西」一节明令：命令输出的具体措辞、表格列名列序、
// 空字段占位符、/ask 与 /session 屏幕输出的具体排版，测试不得字面断言。
// 本文件的断言原则是：查库看有没有该有的消息、看数字对不对、看某个语义标记
// 在不在，不锁排版、不锁措辞的字面全文（除了个别地方——已在注释里说明为什么
// 那一处例外）。

// runREPLRunner 是 conversation_test.go 里 runREPLSvc 的变体：能塞一个真正的
// *turn.Runner，用于覆盖 rn != nil 的路径。rn == nil 的路径已经由 runREPLSvc
// 覆盖，不重复。看门狗与 panic 兜底照抄 runREPLSvc，理由同它。
func runREPLRunner(t *testing.T, svc *conversation.Service, rn *turn.Runner, input string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	r := New(newTestREPLConfig(), svc, rn, strings.NewReader(input), &out, RulesContext{})

	done := make(chan error, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				done <- fmt.Errorf("REPL.Run panic: %v", rec)
			}
		}()
		done <- r.Run(context.Background())
	}()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(testREPLTimeout):
		t.Fatalf("REPL.Run 在 %v 内没有返回", testREPLTimeout)
		return "", nil
	}
}

// newTestRunner 造一个真正的 *turn.Runner，用 newTestREPLConfig 里那三个
// agent（ag_alpha / ag_mid / ag_zeta），它们的 provider 分别是
// pv_alpha / pv_mid / pv_zeta。
//
// internal/provider 的 registry 只登记了 claude / grok / agy 三个 id
// （internal/provider/{claude,grok,agy}.go 的 init() 各自调一次 Register），
// pv_alpha 这类 id 不在其中。于是对这份配置里任何一个 agent 调 rn.Ask，
// 都会在 provider.Lookup 那一步（runner.go:118-124）被 r.refuse 掉：
// 不会真起子进程，也碰不到 r.sess（sessionKey 的读写在 Lookup 判定
// 之后才发生，runner.go:145-146），refuse 本身也不经过 svc 写任何东西
// （runner.go:389-393）。这让我们能在不起子进程、不用假 Spawner 的前提下，
// 真实地跑通 needsRunner 通过之后那一段命令逻辑——这正是任务边界里
// 「只测不需要 spawn 就能验的点」那句话要求的做法。
func newTestRunner(t *testing.T, svc *conversation.Service) *turn.Runner {
	t.Helper()
	return turn.NewRunner(newTestREPLConfig(), svc, t.TempDir(), turn.RulesBlocks{})
}

// newOutREPL 造一个只用来接收输出的 REPL，供直接调用 writeAskRecord /
// writeAskMeter 这两个不经过 dispatch 的纯展示函数。cfg/svc/rn/in 全部
// 留空：这两个函数只碰 r.out。
func newOutREPL() (*REPL, *bytes.Buffer) {
	var buf bytes.Buffer
	r := New(nil, nil, nil, nil, &buf, RulesContext{})
	return r, &buf
}

func floatPtr(v float64) *float64 { return &v }

// numericTokenPattern 抓输出里所有形如 123 或 1.5 的数字片段。
var numericTokenPattern = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

func numericTokens(s string) []string {
	return numericTokenPattern.FindAllString(s, -1)
}

// uuidLikePattern 认一个标准的 8-4-4-4-12 UUID 字面量，用来断言「这一格
// 不像是一个会话 id」，而不必知道 SessionInfo.ProviderSession 具体格式。
var uuidLikePattern = regexp.MustCompile(
	`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// ============================================================
// 前置条件三连：needsSvc -> needsConv -> needsRunner
// ============================================================

// TestAskSessionPreconditionOrder 钉住 runCommand 里三档前置条件的检查顺序
// （commands.go 的 runCommand 注释、09a 契约「repl.New 请求修订」一节）：
// 持久化没起来时，连「有没有当前会话」都无从谈起；没选会话时，
// provider 调用未就绪这句话还轮不到说。三档各写一条，且每一条都特意
// 制造「后面几档的条件其实是满足的」这种局面，这样答对答案的唯一原因
// 只能是顺序对了，而不是巧合——如果只在两档都不满足时测，两种实现顺序
// 会给出一样的答案，测不出谁先谁后。
func TestAskSessionPreconditionOrder(t *testing.T) {
	t.Run("svc 为 nil 时先答持久化未就绪", func(t *testing.T) {
		// rn 故意给一个真的（非 nil），而不是也留 nil：
		// 只有 svc=nil 且 rn!=nil 时，「先查 svc」与「先查 rn」两种实现顺序
		// 才会给出不同的答案（前者答 msgNoStore，后者会因为 rn 非空而放行、
		// 转而去查 needsConv，答出别的东西）。两者都是 nil 的话测不出顺序。
		rn := turn.NewRunner(newTestREPLConfig(), nil, t.TempDir(), turn.RulesBlocks{})
		for _, line := range []string{"/ask ag_alpha hi\n", "/session\n"} {
			out, err := runREPLRunner(t, nil, rn, line)
			if err != nil {
				t.Fatalf("%q 下 Run 应当返回 nil，实际 %v", line, err)
			}
			if !strings.Contains(out, msgNoStore) {
				t.Errorf("%q 没有答 msgNoStore；完整输出:\n%s", line, out)
			}
			if strings.Contains(out, msgNoConv) || strings.Contains(out, msgNoRunner) {
				t.Errorf("%q 答出了不该在这一档出现的提示；完整输出:\n%s", line, out)
			}
		}
	})

	t.Run("svc 有但没有当前会话时先答先选一个会话", func(t *testing.T) {
		// rn 这里留 nil，理由与上面对称：只有 rn 也不满足时，才能证明
		// 「没选会话」确实排在「provider 未就绪」前面被回答——
		// rn 非 nil 的话 needsRunner 那一档天然通过，测不出谁先谁后。
		svc, f := newTestService(t)
		f.seedConversation("seeded_no_current_000000000000", "库里有会话但没被选中")

		for _, line := range []string{"/ask ag_alpha hi\n", "/session\n"} {
			out, err := runREPLSvc(t, svc, line)
			if err != nil {
				t.Fatalf("%q 下 Run 应当返回 nil，实际 %v", line, err)
			}
			if !strings.Contains(out, msgNoConv) {
				t.Errorf("%q 没有答 msgNoConv；完整输出:\n%s", line, out)
			}
			if strings.Contains(out, msgNoStore) || strings.Contains(out, msgNoRunner) {
				t.Errorf("%q 答出了不该在这一档出现的提示；完整输出:\n%s", line, out)
			}
		}
		if n := f.totalMessages(); n != 0 {
			t.Errorf("没有当前会话时不该写入任何消息，实际写了 %d 条", n)
		}
	})

	t.Run("svc 与当前会话都有但 rn 为 nil 时答 provider 调用未就绪", func(t *testing.T) {
		svc, f := newTestService(t)
		out, err := runREPLSvc(t, svc, "/new 会话\n/ask ag_alpha hi\n/session\n")
		if err != nil {
			t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
		}
		if !strings.Contains(out, msgNoRunner) {
			t.Errorf("没有答 msgNoRunner；完整输出:\n%s", out)
		}
		if strings.Contains(out, msgNoStore) || strings.Contains(out, msgNoConv) {
			t.Errorf("答出了不该在这一档出现的提示；完整输出:\n%s", out)
		}
		// rn 为 nil 时 runCommand 在调 cmdAsk 之前就拦下了，/ask 带的那句
		// "hi" 不该有机会经 postUserText 落库。
		if n := f.totalMessages(); n != 0 {
			t.Errorf("rn 为 nil 时 /ask 不该落库，实际写了 %d 条", n)
		}
	})
}

// ============================================================
// /ask 的参数与正文处理
// ============================================================

// TestAskWithoutAgentIDShowsUsageAndContinues 锁住 cmdAsk 的第一条防线：
// 不带 agent-id 只给用法提示、返回 nil，不让 REPL 退出。
// 用 /help 接在后面验证"没退出"：如果 /ask 返回了非 nil 错误或错误地
// 触发了 errQuit，/help 就不会再被处理，输出里也就看不到它的痕迹。
func TestAskWithoutAgentIDShowsUsageAndContinues(t *testing.T) {
	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)

	out, err := runREPLRunner(t, svc, rn, "/new 会话\n/ask\n/help\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}
	if n := f.totalMessages(); n != 0 {
		t.Errorf("/ask 不带参数不该落库，实际写了 %d 条", n)
	}
	// /help 遍历命令表打印，/quit 一定在表里；这行出现就证明 REPL 在
	// /ask 的用法提示之后确实继续读了下一行、执行了 /help，而不是碰巧
	// 也返回了 nil 但其实已经在某处死循环或提前结束。
	if !strings.Contains(out, "/quit") {
		t.Errorf("/ask 之后应当能看到 /help 的输出（含 /quit），说明 REPL 没有退出；完整输出:\n%s", out)
	}
}

// TestAskBodyPreservesInternalWhitespace 锁住 ask.go 顶部注释里的硬性要求：
// 正文取 restOf，不是 strings.Fields 再 Join；正文中间的连续空白必须原样
// 保留。这里塞两个制表符与三个空格两种不同的连续空白，只要有一处被压缩，
// 断言就会失败。
func TestAskBodyPreservesInternalWhitespace(t *testing.T) {
	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)

	const body = "alpha\t\tbeta   gamma"
	const convID = "conv001" // /new 走假实现的 CreateConversation，第一个 id 固定是这个

	// x1 成员闸落地后 /ask 要求 agent 是本会话的活跃成员，先 /join 一次；
	// 本用例验的是"正文内部空白原样保留"，与成员状态无关，/join 只是让
	// 后面的 /ask 能走到 postUserText，不影响这条用例本身的判据。
	out, err := runREPLRunner(t, svc, rn, "/new 会话\n/join ag_alpha\n/ask ag_alpha "+body+"\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	msgs := f.msgs[convID]
	if len(msgs) != 1 {
		t.Fatalf("应当恰好落 1 条消息，实际 %d 条；完整输出:\n%s", len(msgs), out)
	}
	if msgs[0].Text != body {
		t.Errorf("落库的正文是 %q，期望原样保留内部空白的 %q", msgs[0].Text, body)
	}
}

// TestAskWithoutTextPostsNoMessage 锁住「/ask <id> 不带文本时不落任何
// user 消息」。09a 契约写明不带文本时直接调 Ask，本用例里 ag_alpha 的
// provider 不在 registry 里，Ask 会在起进程之前就被拒绝（见 newTestRunner
// 的注释），全程不经过 svc，所以 MaxSeq 前后必须相等。
func TestAskWithoutTextPostsNoMessage(t *testing.T) {
	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)

	out, err := runREPLRunner(t, svc, rn, "/new 会话\n/ask ag_alpha\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}
	after, err := svc.MaxSeq(context.Background(), "conv001")
	if err != nil {
		t.Fatalf("读 MaxSeq 失败: %v", err)
	}
	if after != 0 {
		t.Errorf("/ask 不带文本时 seq 涨到了 %d，说明落库了一条不该有的消息", after)
	}
	if n := f.totalMessages(); n != 0 {
		t.Errorf("/ask 不带文本不该写入任何消息，实际写了 %d 条", n)
	}
}

// TestAskWithTextPostsSameShapeAsSay 是本文件最重要的一条：锁住
// 「/say、裸文本、多行草稿、/ask 带文本」四条路必须共用 postUserText、
// 发出去的消息形状必须一模一样（ask.go 顶部注释）。
//
// 用同一段文本分别走 /say 与 /ask，比较两条消息除 Seq / ID / ConvID /
// CreatedAt（这四个字段两条路径天然不同）之外的每一个字段。
func TestAskWithTextPostsSameShapeAsSay(t *testing.T) {
	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)

	const sayConv = "convsay00000000000000000000000000"
	const askConv = "convask00000000000000000000000000"
	f.seedConversation(sayConv, "会话甲：走 /say")
	f.seedConversation(askConv, "会话乙：走 /ask")
	// x1 成员闸落地后 /ask 要求 agent 是本会话的活跃成员，直接塞一个活跃
	// participant（不经 /join，理由与本包其它用例一致：/join 的回显也含
	// agent id，没必要为了满足前置条件去多打一行不相关的输出）。本用例验
	// 的是"落库的消息形状与 /say 一致"，与成员状态本身无关。
	f.seedParticipant(t, askConv, "ag_alpha")

	const text = "四条路发出去的消息形状必须一模一样"

	if _, err := runREPLRunner(t, svc, rn, "/use "+sayConv+"\n/say "+text+"\n"); err != nil {
		t.Fatalf("/say 一轮 Run 失败: %v", err)
	}
	if _, err := runREPLRunner(t, svc, rn, "/use "+askConv+"\n/ask ag_alpha "+text+"\n"); err != nil {
		t.Fatalf("/ask 一轮 Run 失败: %v", err)
	}

	sayMsgs := f.msgs[sayConv]
	askMsgs := f.msgs[askConv]
	if len(sayMsgs) != 1 || len(askMsgs) != 1 {
		t.Fatalf("两条路径各应落 1 条消息，实际 /say=%d 条 /ask=%d 条", len(sayMsgs), len(askMsgs))
	}
	// ag_alpha 的 provider 不在 registry 里，Ask 会被拒但不影响这条已经
	// 落库的 user 消息；顺带证明拒绝路径没有额外写进任何面包屑消息。
	if n := len(sayMsgs) + len(askMsgs); n != 2 {
		t.Fatalf("两条会话加起来应当恰好 2 条消息，实际 %d 条", n)
	}

	strip := func(m conversation.Message) conversation.Message {
		m.ConvID, m.ID = "", ""
		m.Seq = 0
		m.CreatedAt = time.Time{}
		return m
	}
	got, want := strip(askMsgs[0]), strip(sayMsgs[0])
	if !reflect.DeepEqual(got, want) {
		t.Errorf("/ask 带文本落下的消息形状与 /say 不一致。\n/ask: %+v\n/say: %+v", got, want)
	}
}

// ============================================================
// /session
// ============================================================

// sessionOutputDelta 起一个会话、塞 nParticipants 个活跃成员（用 fakeStore
// 直接塞，不经 /join——/join 自己的回显也含 agent id，混进同一段输出会让
// 「/session 打了什么」这件事没法单独看清），跑一次带 /session 的输入，
// 再跑一次同样前缀但用空行代替 /session 的输入，返回两次输出的字节数之差。
// 这样就把 /session 单独贡献的那一段输出的体量隔离出来，不必知道它的
// 具体排版或列名。
func sessionOutputDelta(t *testing.T, nParticipants int) int {
	t.Helper()

	agentIDs := []string{"ag_alpha", "ag_mid", "ag_zeta"}
	if nParticipants > len(agentIDs) {
		t.Fatalf("sessionOutputDelta: 最多只有 %d 个测试 agent 可用", len(agentIDs))
	}

	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)
	const convID = "conv_session_delta0000000000000000"
	f.seedConversation(convID, "delta 用例")
	for i := 0; i < nParticipants; i++ {
		f.seedParticipant(t, convID, agentIDs[i])
	}

	without, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n\n")
	if err != nil {
		t.Fatalf("不带 /session 的一轮 Run 失败: %v", err)
	}
	with, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session\n")
	if err != nil {
		t.Fatalf("带 /session 的一轮 Run 失败: %v", err)
	}
	return len(with) - len(without)
}

// TestSessionNoActiveMembersGivesHintNotTable 锁住「会话里没有活跃成员时
// 给一句提示，不打空表」。用没有成员（delta 应当很短，只是一句提示）与
// 有两个成员（delta 应当明显更长，含表头与两行数据）做对照，
// 不去猜提示或表格具体长什么样，只看体量差异的方向对不对。
func TestSessionNoActiveMembersGivesHintNotTable(t *testing.T) {
	emptyDelta := sessionOutputDelta(t, 0)
	twoMemberDelta := sessionOutputDelta(t, 2)

	if emptyDelta <= 0 {
		t.Errorf("没有成员时 /session 应当至少给一句提示，delta 却是 %d", emptyDelta)
	}
	if !(emptyDelta < twoMemberDelta) {
		t.Errorf("没有成员时的输出体量（%d 字节）应当明显小于两个成员时（%d 字节），"+
			"否则说明没有成员时也打印了类似表格的东西", emptyDelta, twoMemberDelta)
	}
}

// TestSessionListsMembersInAgentIDOrder 锁住 cmdSession 显式做过的排序：
// 返回顺序不在契约里，但输出必须稳定，按 agent id 字典序。
// 塞入顺序故意反着来（ag_zeta 先、ag_alpha 后），如果实现漏掉排序、
// 直接按 Participants 返回的顺序打印，这条用例会抓到。
func TestSessionListsMembersInAgentIDOrder(t *testing.T) {
	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)
	const convID = "conv_session_order00000000000000000"
	f.seedConversation(convID, "排序用例")
	f.seedParticipant(t, convID, "ag_zeta")
	f.seedParticipant(t, convID, "ag_alpha")

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	atAlpha := strings.Index(out, "ag_alpha")
	atZeta := strings.Index(out, "ag_zeta")
	if atAlpha < 0 || atZeta < 0 {
		t.Fatalf("输出里应当同时出现 ag_alpha 与 ag_zeta；完整输出:\n%s", out)
	}
	if !(atAlpha < atZeta) {
		t.Errorf("ag_alpha 应当排在 ag_zeta 前面（字典序），实际位置 alpha=%d zeta=%d；完整输出:\n%s",
			atAlpha, atZeta, out)
	}
}

// TestSessionNeverAskedShowsFreshNotEmptyID 锁住 cmdSession 注释里点名的
// 那个陷阱：ok 为 false（这个 agent 还没开过口）要显示成一个明确的
// "还没开始"状态，不能显示成一个空的会话 id——空 id 看起来像"有会话
// 但 id 丢了"，会被误读成 bug。
//
// 断言方式三选一都不锁排版：(1) 这一行不出现任何形似 UUID 的字符串；
// (2) 不出现 warm / cold 这两个只有真正建立过会话才会有的状态词；
// (3) 出现 "fresh"——这个词不是随便挑的排版，是 ask.go 里 cmdSession
// 的函数注释原话："要明确显示成 fresh，不要显示成空会话 id 了事"，
// 也是当前源码里唯一的实现（runner.go SessionInfo 的 ok==false 分支）。
// 它是这条行为要求本身的一部分，不是可以自由改的排版措辞。
func TestSessionNeverAskedShowsFreshNotEmptyID(t *testing.T) {
	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)
	const convID = "conv_session_fresh00000000000000000"
	f.seedConversation(convID, "从没被问过的成员")
	f.seedParticipant(t, convID, "ag_alpha")

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	line := lineContaining(t, out, "ag_alpha")
	if uuidLikePattern.MatchString(line) {
		t.Errorf("从没被 /ask 过的成员那一行出现了形似会话 id 的字符串，"+
			"应当显示成明确的“还没开始”状态: %s", line)
	}
	if strings.Contains(line, "warm") || strings.Contains(line, "cold") {
		t.Errorf("从没被 /ask 过的成员不该显示成 warm 或 cold（那是已经建立过会话的状态）: %s", line)
	}
	if !strings.Contains(line, "fresh") {
		t.Errorf("从没被 /ask 过的成员那一行应当显示 fresh；完整输出:\n%s", out)
	}
}

// TestSessionExcludesLeftMembers 锁住 cmdSession 只问活跃成员
// （Participants 的 includeInactive=false）：已 /leave 的人不再出现在
// /session 里。直接调 DeactivateParticipant 让它退出，绕开 /leave 的回显
// （回显里也含 agent id，会污染这里要做的 Contains 断言）。
func TestSessionExcludesLeftMembers(t *testing.T) {
	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)
	const convID = "conv_session_left0000000000000000000"
	f.seedConversation(convID, "退出用例")
	f.seedParticipant(t, convID, "ag_alpha")
	f.seedParticipant(t, convID, "ag_zeta")
	if err := f.DeactivateParticipant(context.Background(), convID, "ag_zeta", time.Now().UTC()); err != nil {
		t.Fatalf("塞入退出状态失败: %v", err)
	}

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}
	if !strings.Contains(out, "ag_alpha") {
		t.Errorf("仍然活跃的 ag_alpha 应当出现在 /session 里；完整输出:\n%s", out)
	}
	if strings.Contains(out, "ag_zeta") {
		t.Errorf("已经退出的 ag_zeta 不该出现在 /session 里；完整输出:\n%s", out)
	}
}

// ============================================================
// askCause：哨兵翻译
// ============================================================

// TestAskCauseRecognizesSentinelsThroughWrapping 证明 askCause 用
// errors.Is 认哨兵，不是比较 Error() 字符串。做法：把每个哨兵包一层
// fmt.Errorf("...: %w", sentinel)，包完之后 Error() 文本已经变了
// （多了前缀），如果 askCause 是在比字符串，包完就认不出来、会退到
// 默认分支原样打印 wrapped.Error()；用 errors.Is 则不受影响。
// 顺带断言五个哨兵翻出五句互不相同的话，不然用户没法区分状况。
func TestAskCauseRecognizesSentinelsThroughWrapping(t *testing.T) {
	cases := []struct {
		name     string
		sentinel error
	}{
		{"no delta", turn.ErrNoDelta},
		{"agent unknown", turn.ErrAgentUnknown},
		{"identity drift", turn.ErrIdentityDrift},
		{"prefix rewrite", turn.ErrPrefixRewrite},
		{"tier not offered", turn.ErrTierNotOffered},
	}

	seen := make(map[string]string) // 翻出来的话 -> 是哪个哨兵翻出来的，抓重复
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			direct := askCause(tc.sentinel)
			wrapped := fmt.Errorf("上下文前缀: %w", tc.sentinel)
			got := askCause(wrapped)

			if got != direct {
				t.Errorf("包了一层 %%w 之后 askCause 认不出这个哨兵了："+
					"直接传是 %q，包一层是 %q；应当用 errors.Is 而不是比字符串", direct, got)
			}
			if got == wrapped.Error() {
				t.Errorf("askCause 退到了默认分支，原样打印了 wrapped.Error() = %q，"+
					"说明它没有认出这个哨兵", got)
			}
			if prev, ok := seen[direct]; ok {
				t.Errorf("哨兵 %q 与 %q 翻出了同一句话 %q，用户没法区分两种状况", prev, tc.name, direct)
			}
			seen[direct] = tc.name
		})
	}
}

// TestAskCauseFallsBackToErrorStringForUnknownError 覆盖 askCause 的反面：
// 传一个不匹配任何哨兵的错误，原样返回它的 Error()——第 5-9 类结局的
// 退出码 / stderr 头 / 可执行文件路径已经折进这个错误值的格式化消息里
// （askCause 函数注释），不能被吞掉或替换成一句通用提示。
func TestAskCauseFallsBackToErrorStringForUnknownError(t *testing.T) {
	err := errors.New("一个不属于任何哨兵的普通错误：exit code 7, stderr 头 ...")
	if got := askCause(err); got != err.Error() {
		t.Errorf("askCause(未知错误) = %q，应当原样返回 err.Error() = %q", got, err.Error())
	}
}

// ============================================================
// writeAskRecord / writeAskMeter：纯展示函数，直接构造 turn.Record 调用
// ============================================================

// TestWriteAskRecordShowsDroppedTextVerbatim 锁住 OutcomePostRejected 的
// 核心要求：DroppedText 是付过钱、写不进库的那段正文，必须一字不差地
// 整段出现在输出里，一个字节都不许少（writeFullText 的函数注释）。
//
// 三个变体各防一个具体的坑：
//   - "居中"：正文里混入前后空格、换行、百分号与形似格式占位符的片段
//     （100%、%d、%%）。如果实现把它错传给某个 printf 系的函数当格式串用，
//     会因为参数不匹配而输出走样（例如冒出 %!d(MISSING)），逐字节 Contains
//     就会失败——这是防 writeFullText 被换成 r.printf(s) 的那个坑。
//   - "尾部有空格"：正文最后一个字符是空格而不是换行。writeFullText 只在
//     s 不以 "\n" 结尾时才补一个 "\n"；如果实现在写之前顺手 TrimRight 或
//     TrimSpace 了一下，尾部的空格会消失但因为长度变化不影响"补不补
//     换行"这条判断，容易被漏测。用 dropped+"\n" 作锚点，能同时抓住
//     "尾部空格被削掉"这一种改法。
//   - "已经以换行结尾"：防止 writeFullText 不管三七二十一地追加换行，
//     导致库里本就以 \n 结尾的正文在屏幕上变成多一个空行
//     （ask.go:238 那条 HasSuffix 判断的另一半）。
func TestWriteAskRecordShowsDroppedTextVerbatim(t *testing.T) {
	cases := []struct {
		name    string
		dropped string
	}{
		{
			name:    "居中的换行、百分号与格式占位符",
			dropped: "  第一行末尾有空格   \n第二行含 100% 与 %d 和 %%\n  第三行前面有缩进",
		},
		{
			name:    "尾部有空格而不是换行",
			dropped: "尾部有两个空格  ",
		},
		{
			name:    "正文本身已经以换行结尾",
			dropped: "以换行结尾\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := turn.Record{
				ConvID:      "conv1",
				AgentID:     "agent_dropped",
				Outcome:     turn.OutcomePostRejected,
				DroppedText: tc.dropped,
			}
			r, buf := newOutREPL()
			if err := writeAskRecord(r, rec); err != nil {
				t.Fatalf("writeAskRecord 失败: %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, tc.dropped) {
				t.Errorf("被拒的正文没有一字不差地出现在输出里。\n期望包含:\n%q\n实际输出:\n%q", tc.dropped, out)
			}

			// writeFullText 只在正文不以 "\n" 结尾时才补一个：已经以换行
			// 结尾的正文，期望的落点就是它本身（恰好一个尾部换行）；
			// 不以换行结尾的正文，期望落点是"正文 + 一个换行"。
			// 用这条更长的锚点核对，能抓住"尾部空白被 TrimRight/TrimSpace
			// 悄悄削掉"这件事——只比对 tc.dropped 本身，削掉尾部空格之后
			// 剩下的前缀仍然是原正文的子串，光是 Contains 会漏判。
			wantExact := tc.dropped
			if !strings.HasSuffix(tc.dropped, "\n") {
				wantExact = tc.dropped + "\n"
			}
			if !strings.Contains(out, wantExact) {
				t.Errorf("正文尾部的空白或换行被动过了。\n期望紧跟着出现:\n%q\n实际输出:\n%q", wantExact, out)
			}
			// 不许比期望的落点多出一个换行：正文已经以 \n 结尾时不该被
			// 再补一个，否则屏幕上会多出一个空行（ask.go:238 的另一半）。
			if strings.Contains(out, wantExact+"\n") {
				t.Errorf("正文之后出现了多余的空行，说明尾部被多补了一个 \\n。\n实际输出:\n%q", out)
			}
		})
	}
}

// TestWriteAskRecordMentionsMaybeAdvanced 用差分法验证 MaybeAdvanced：
// 同一份 Record，只翻这一个字段，比较两次输出。要求 (1) 确实产生了差异
// （字段被显示出来了，不是被悄悄吞掉）；(2) 关闭时的每一行在打开之后
// 依然都在（证明差异只是新增了一行，不是把别的内容改写了）；
// (3) 新增的那句话里含"吃下"——这是 ask.go 里这句话独有的字，
// 08a 排除的是"逐字比对整句话"，这里断言的是传达没传达这层意思。
func TestWriteAskRecordMentionsMaybeAdvanced(t *testing.T) {
	base := turn.Record{ConvID: "c", AgentID: "a", Outcome: turn.OutcomeTimeout}
	withFlag := base
	withFlag.MaybeAdvanced = true

	r1, buf1 := newOutREPL()
	if err := writeAskRecord(r1, base); err != nil {
		t.Fatalf("writeAskRecord 失败: %v", err)
	}
	r2, buf2 := newOutREPL()
	if err := writeAskRecord(r2, withFlag); err != nil {
		t.Fatalf("writeAskRecord 失败: %v", err)
	}
	without, with := buf1.String(), buf2.String()

	if with == without {
		t.Fatalf("MaybeAdvanced 打开之后输出与关闭时完全一样，说明这个标记没有被显示出来")
	}
	for _, line := range strings.Split(strings.TrimRight(without, "\n"), "\n") {
		if line != "" && !strings.Contains(with, line) {
			t.Errorf("关闭 MaybeAdvanced 时的这一行在打开之后消失了: %q", line)
		}
	}
	if !strings.Contains(with, "吃下") {
		t.Errorf("MaybeAdvanced 为真时输出里应当传达"+
			"“provider 那边可能已经吃下这批内容”的意思，实际:\n%s", with)
	}
}

// TestWriteAskRecordMentionsDeniedActions 锁住本片新增的行为：Record.DeniedActions
// 非空时，输出必须点名被拒的工具调用（action 与 display_name 都要出现），
// 且这一行不依赖 rec.Message 非空——failClosed 路径（这里用 OutcomeProviderFailed
// 模拟）同样可能带着非空的 DeniedActions（absorbParse 先于失败判定运行），
// 屏幕上同样要说出来，不能只在 OutcomeSpoke 那一支才显示。
func TestWriteAskRecordMentionsDeniedActions(t *testing.T) {
	base := turn.Record{ConvID: "c", AgentID: "a", Outcome: turn.OutcomeProviderFailed}
	withDenied := base
	withDenied.DeniedActions = []provider.DeniedAction{
		{Action: "write_file", DisplayName: "WriteToFile"},
	}

	r1, buf1 := newOutREPL()
	if err := writeAskRecord(r1, base); err != nil {
		t.Fatalf("writeAskRecord 失败: %v", err)
	}
	r2, buf2 := newOutREPL()
	if err := writeAskRecord(r2, withDenied); err != nil {
		t.Fatalf("writeAskRecord 失败: %v", err)
	}
	without, with := buf1.String(), buf2.String()

	if with == without {
		t.Fatalf("DeniedActions 非空时输出与为空时完全一样，说明这件事没有被显示出来")
	}
	if !strings.Contains(with, "write_file") || !strings.Contains(with, "WriteToFile") {
		t.Errorf("输出里应当同时点名 action 与 display_name；实际:\n%s", with)
	}
	if !strings.Contains(with, "被拒") {
		t.Errorf("输出里应当出现“被拒”，让用户看得出这是拒绝而不是别的事；实际:\n%s", with)
	}
	// 不许因为有 denied actions 就暗示整轮失败：这条信息是附加事实，
	// 不是结局判定，措辞里必须留有余地（不是判死刑）。
	if !strings.Contains(with, "不代表") && !strings.Contains(with, "可能已经") {
		t.Errorf("输出里应当说明界内动作可能已经完成、不代表整轮失败；实际:\n%s", with)
	}
	// denied_actions 的长度是下界不是清点：实测一条「1 件界内 + 2 件越界」的
	// 指令跑完，两个越界文件一个都没落盘，而这个字段只有 1 条
	// （oq-agy-denied-count/RESULTS.md，11c 第 7.1 节）。措辞若写成
	// 「一共有 N 个」，那个数字会因为反复出现而被当成量过的清点。
	if !strings.Contains(with, "可能不止") {
		t.Errorf("输出必须限定这个条数是 provider 报的、可能不全；实际:\n%s", with)
	}
}

// TestWriteAskRecordMentionsDrifted 同上，验证 Drifted。新增内容应当含
// "悄悄开了新会话"：这是 Drifted 那句话里描述"CLI 私自重建了会话"这件事
// 的核心措辞，Record.Drifted 字段自己的注释也是这几个字
// （record.go:72-74），不是随手挑的排版。
func TestWriteAskRecordMentionsDrifted(t *testing.T) {
	base := turn.Record{ConvID: "c", AgentID: "a", Outcome: turn.OutcomeSpoke}
	withFlag := base
	withFlag.Drifted = true

	r1, buf1 := newOutREPL()
	if err := writeAskRecord(r1, base); err != nil {
		t.Fatalf("writeAskRecord 失败: %v", err)
	}
	r2, buf2 := newOutREPL()
	if err := writeAskRecord(r2, withFlag); err != nil {
		t.Fatalf("writeAskRecord 失败: %v", err)
	}
	without, with := buf1.String(), buf2.String()

	if with == without {
		t.Fatalf("Drifted 打开之后输出与关闭时完全一样，说明这个标记没有被显示出来")
	}
	if !strings.Contains(with, "悄悄开了新会话") {
		t.Errorf("Drifted 为真时输出里应当说明 CLI 悄悄开了新会话，实际:\n%s", with)
	}
}

// TestWriteAskRecordEmptyTurnNote 锁住 Message != nil 且 Message.Text 为
// 空串时要说"本轮为空"这个意思。用正文非空的一轮做对照，证明两者输出
// 确实不同，而且各自的正文没有串到对方那边去。
func TestWriteAskRecordEmptyTurnNote(t *testing.T) {
	emptyRec := turn.Record{
		Outcome: turn.OutcomeSpoke,
		Message: &conversation.Message{Seq: 1, SenderID: "ag1", Text: ""},
	}
	nonEmptyRec := turn.Record{
		Outcome: turn.OutcomeSpoke,
		Message: &conversation.Message{Seq: 1, SenderID: "ag1", Text: "这一轮说了话"},
	}

	r1, buf1 := newOutREPL()
	if err := writeAskRecord(r1, emptyRec); err != nil {
		t.Fatalf("writeAskRecord 失败: %v", err)
	}
	r2, buf2 := newOutREPL()
	if err := writeAskRecord(r2, nonEmptyRec); err != nil {
		t.Fatalf("writeAskRecord 失败: %v", err)
	}
	emptyOut, nonEmptyOut := buf1.String(), buf2.String()

	if emptyOut == nonEmptyOut {
		t.Fatalf("正文为空与正文非空两轮渲染出了完全一样的输出")
	}
	if !strings.Contains(nonEmptyOut, "这一轮说了话") {
		t.Errorf("正文非空时输出里应当包含正文本身；实际:\n%s", nonEmptyOut)
	}
	if strings.Contains(emptyOut, "这一轮说了话") {
		t.Errorf("正文为空的那次输出不该出现另一条用例的正文；实际:\n%s", emptyOut)
	}

	// completionNote 是纯函数，直接单测更精确：本轮为空这句话具体是什么
	// 由它决定，写作 writeAskRecord 只是原样转发。
	if note := completionNote(emptyRec); note == "" {
		t.Errorf("Message.Text 为空串时 completionNote 应当给出一句非空的提示，实际是空串")
	}
}

// TestCompletionNoteVariants 覆盖 completionNote 的四种输入：Meta 标记
// 截断、Meta 标记为空、Meta 正常、没有 Meta。前两者应当有提示，
// 后两者不该有——不然每一条正常消息都会被打上多余的注解。
func TestCompletionNoteVariants(t *testing.T) {
	cases := []struct {
		name      string
		meta      map[string]any
		wantEmpty bool
	}{
		{"截断", map[string]any{turn.MetaCompletion: "truncated"}, false},
		{"标记为空但正文非空", map[string]any{turn.MetaCompletion: "empty"}, false},
		{"正常完成", map[string]any{turn.MetaCompletion: "complete"}, true},
		{"没有 Meta", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := turn.Record{Message: &conversation.Message{Text: "非空正文", Meta: tc.meta}}
			note := completionNote(rec)
			if tc.wantEmpty && note != "" {
				t.Errorf("期望没有提示，实际是 %q", note)
			}
			if !tc.wantEmpty && note == "" {
				t.Errorf("期望有一句提示，实际是空串")
			}
		})
	}
}

// TestWriteAskMeterSuppressedForRefusedAndUnknown 锁住 OutcomeRefused /
// OutcomeUnknown 不打用量行（起进程之前就停的，没有用量可报）。
// 塞进一个显眼的数字与一个非 nil 的金额，断言 writeAskMeter 整个不写
// 任何东西；再用另一个结局做正对照，证明同样的数字在别的结局下确实
// 会被打出来——不然"没打印"也可能只是因为函数本身坏了。
func TestWriteAskMeterSuppressedForRefusedAndUnknown(t *testing.T) {
	for _, oc := range []turn.Outcome{turn.OutcomeRefused, turn.OutcomeUnknown} {
		rec := turn.Record{
			Outcome:      oc,
			Usage:        provider.Usage{InputTokens: 99999},
			CostUSDDelta: floatPtr(4.25),
		}
		r, buf := newOutREPL()
		if err := writeAskMeter(r, rec); err != nil {
			t.Fatalf("writeAskMeter 失败: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("结局 %v 起进程之前就停了，不该打印任何用量行，实际输出:\n%s", oc, buf.String())
		}
	}

	// x36ab：没解析过用量的轮不再打数字，对照组改用「失败但用量已解析」的真实形状。
	control := turn.Record{Outcome: turn.OutcomeProviderFailed, UsageParsed: true, Usage: provider.Usage{InputTokens: 99999}}
	r, buf := newOutREPL()
	if err := writeAskMeter(r, control); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	if !strings.Contains(buf.String(), "99999") {
		t.Errorf("对照组 OutcomeProviderFailed（用量已解析）应当照常打用量，实际:\n%s", buf.String())
	}
}

// TestWriteAskMeterOnlyPrintsSuppliedNumbers 给 Usage 与三个计时字段各塞
// 一个互不相同、非零的数字，抓输出里所有数字片段，断言每一个都在我们
// 塞进去的白名单里。CostUSDDelta 留 nil、BaselineSuspect 留 false，
// 这两种情况下金额那一行不该有任何数字——如果实现在这种局面下编出一个
// 数字（哪怕是 0），这里就会抓到一个不在白名单里的数字。
func TestWriteAskMeterOnlyPrintsSuppliedNumbers(t *testing.T) {
	cacheCreate := 7713
	rec := turn.Record{
		Outcome:     turn.OutcomeSpoke,
		UsageParsed: true, // x36ab：真实的 Spoke 轮用量恒已解析
		Usage: provider.Usage{
			InputTokens:    48151,
			CacheCreate:    &cacheCreate,
			CacheRead:      12013,
			OutputTokens:   55511,
			ThinkingTokens: 4201,
		},
		WallMS:      90007,
		PromptBytes: 30011,
		ScannedTo:   60013,
	}

	r, buf := newOutREPL()
	if err := writeAskMeter(r, rec); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	out := buf.String()

	whitelist := map[string]bool{
		"48151": true, "7713": true, "12013": true, "55511": true, "4201": true,
		"90007": true, "30011": true, "60013": true,
	}
	for _, tok := range numericTokens(out) {
		if !whitelist[tok] {
			t.Errorf("输出里出现了一个不是我们塞进去的数字 %q，疑似编造的金额或用量；完整输出:\n%s", tok, out)
		}
	}
	for tok := range whitelist {
		if !strings.Contains(out, tok) {
			t.Errorf("塞进去的数字 %q 没有出现在输出里；完整输出:\n%s", tok, out)
		}
	}
}

// TestWriteAskMeterCacheCreateNilIsNotZero 锁住 agy 那种"根本没有这一项"
// 的局面：Usage.CacheCreate 为 nil 时那一格不能显示成 0，会让人误以为
// provider 报过、测出来是 0（record.go 里 CacheCreate 字段自己的注释）。
// 不锁占位符具体是什么字符（08a 排除"空字段占位符"），只锁住行为：
// nil 与显式的 0 必须渲染出不同的东西，且 nil 那一侧不能出现
// "cache_create=0" 这个具体的、代表"测出来确实是 0"的写法。
func TestWriteAskMeterCacheCreateNilIsNotZero(t *testing.T) {
	zero := 0
	// x36ab：用量行只在 UsageParsed 时打数字，两组都设成已解析。
	withNil := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true, Usage: provider.Usage{CacheCreate: nil}}
	withZero := turn.Record{Outcome: turn.OutcomeSpoke, UsageParsed: true, Usage: provider.Usage{CacheCreate: &zero}}

	r1, buf1 := newOutREPL()
	if err := writeAskMeter(r1, withNil); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	r2, buf2 := newOutREPL()
	if err := writeAskMeter(r2, withZero); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	outNil, outZero := buf1.String(), buf2.String()

	if outNil == outZero {
		t.Fatalf("CacheCreate 为 nil（没有这一项）与显式的 0（测出来是 0）渲染成了同一行，" +
			"调用方分不清这两种状况")
	}
	if strings.Contains(outNil, "cache_create=0") {
		t.Errorf("CacheCreate 为 nil 时那一格显示成了 0；完整输出:\n%s", outNil)
	}
	if !strings.Contains(outZero, "cache_create=0") {
		t.Errorf("对照组：CacheCreate 显式为 0 时应当显示 0，实际没有；完整输出:\n%s", outZero)
	}
}

// TestWriteAskMeterCostNilVariants 覆盖 CostUSDDelta == nil 的两种子情形：
// BaselineSuspect 为真（上一轮结局未知，基线可疑）与为假（provider 单纯
// 没报金额）。两种都不许编出一个数字（断言不含 "USD"，真金额那一行
// 的格式恒是 "%g USD"），而且两种要能被用户区分开——不然"不知道花了
// 多少"与"上一轮结局都不确定"这两种性质不同的状况会被读成一回事。
func TestWriteAskMeterCostNilVariants(t *testing.T) {
	usage := provider.Usage{InputTokens: 1}
	suspect := turn.Record{Outcome: turn.OutcomeSpoke, Usage: usage, BaselineSuspect: true}
	plain := turn.Record{Outcome: turn.OutcomeSpoke, Usage: usage, BaselineSuspect: false}

	r1, buf1 := newOutREPL()
	if err := writeAskMeter(r1, suspect); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	r2, buf2 := newOutREPL()
	if err := writeAskMeter(r2, plain); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	outSuspect, outPlain := buf1.String(), buf2.String()

	if outSuspect == outPlain {
		t.Errorf("基线可疑与普通未报两种局面渲染成了同一行，用户没法区分这两种状况")
	}
	for _, out := range []string{outSuspect, outPlain} {
		if strings.Contains(out, "USD") {
			t.Errorf("CostUSDDelta 为 nil 时不该出现任何带 USD 的金额；完整输出:\n%s", out)
		}
	}
}

// TestWriteAskMeterCostReportedShowsExactNumber 是正对照：CostUSDDelta
// 非 nil 时，那个具体数字应当原样出现，证明上面几条"不许出现数字"的
// 用例不是因为函数整体不打印数字，而是专门在 nil 的情形下不编。
func TestWriteAskMeterCostReportedShowsExactNumber(t *testing.T) {
	rec := turn.Record{
		Outcome:      turn.OutcomeSpoke,
		Usage:        provider.Usage{InputTokens: 1},
		CostUSDDelta: floatPtr(0.4225),
	}
	r, buf := newOutREPL()
	if err := writeAskMeter(r, rec); err != nil {
		t.Fatalf("writeAskMeter 失败: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "0.4225") || !strings.Contains(out, "USD") {
		t.Errorf("CostUSDDelta 非 nil 时应当原样打出金额与 USD 单位；实际:\n%s", out)
	}
}
