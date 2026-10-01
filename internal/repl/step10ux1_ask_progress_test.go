package repl

// 本文件覆盖冻结契约 E:\multi_model_cowork\step10ux1_contract.md 第 6 节
// 验收 7-8：/ask 在起进程之前打的那一行。
//
// 背对背纪律：本文件依赖的是 ask.go 里 cmdAsk 尚不存在的新行为
// （「起进程前打一行，含 agent id」），此刻这一行还没有被实现腿写下，
// 本文件的用例会先编译通过、再以「红」收场，直到 ask.go 落地——这是
// 预期状态，不为了让它绿而去改 ask.go。
//
// 验收 7 的判据是「spawn 被调用的那一刻，已写出的输出里就有它」，不是
// 「最终输出里有它」（后者对「最后才打」的坏实现也为真）。internal/repl
// 拿不到 *turn.Runner 内部的 spawn 字段（未导出，也不该为测试给它加一个
// 导出口子——那是实现文件），没法像 internal/turn 包自己那样塞一个纯
// 内存、可控的假 Spawner。本文件复用本包已有先例的 helper-process 套路
// （session_inflight_test.go 的 TestMain + COUNCIL_FAKE_CLI=1，测试二进制
// 自己扮演 provider CLI：没有 --resume 立即回，带 --resume 睡
// inFlightSleep=700ms）——TestMain 每个包只能有一个，本文件不新增，
// 直接用 session_inflight_test.go 已经定义好的 runFakeClaudeCLI / TestMain
// / newInFlightConfig / newInFlightRunner / inFlightAgentID /
// inFlightSleep / inFlightProbeDelay，不改那个文件一个字节。

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
)

// syncBuffer 是一个加锁的 io.Writer，供「一条 goroutine 在写、另一条在读」
// 这种场景用——bytes.Buffer 本身不是并发安全的，REPL 跑在自己的 goroutine
// 里持续 Write，测试主 goroutine 要在它还没写完时就读一次快照，必须有锁。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// runREPLInBackground 起一条 goroutine 跑 r.Run，panic 时把它转成 done
// 里的一个 error 而不是让整个测试进程崩掉（同 ask_test.go 的
// runREPLRunner），但不等它结束——调用方要在它还在跑的时候去读输出。
func runREPLInBackground(r *REPL) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				done <- fmt.Errorf("REPL.Run panic: %v", rec)
			}
		}()
		done <- r.Run(context.Background())
	}()
	return done
}

// ---------------------------------------------------------------------------
// 验收 7：/ask 那一行在起进程之前——判据是 spawn 那一刻输出里已经有它
// ---------------------------------------------------------------------------
//
// 做法：先在 REPL 之外直接调一次 rn.Ask 建立首轮 provider 会话（不经
// cmdAsk，不往 out 里写任何东西——out 这时还是空的），第二条要问的消息
// 直接用 f.seedMessage 塞进库（不经 /say、不经 postUserText），这样喂给
// REPL 的只有 "/use <conv>\n/ask <id>\n"——不带文本，cmdAsk 完全不会走
// postUserText 那条路径，探测到的任何 agent id 只可能来自起进程之前那一行
// 本身，不会被"带文本时的回显"污染这条判据。
//
// 对照组：先单独跑一次只含 "/use <conv>\n" 的输入，断言那次的完整输出里
// 不含 agent id——排除"/use 自己就会印出成员 id"这种可能性，不然下面的
// 探测即便命中也证明不了是 cmdAsk 打的。
//
// 再喂一条真正经过 cmdAsk 的 /ask，这一次因为同一个 (conv, agent) 已经有
// 会话，会带 --resume，触发假 CLI 睡 700ms。趁着这 700ms 还没睡完（探测点
// 取 inFlightProbeDelay=350ms，落在窗口正中间，session_inflight_test.go
// 已经标定过这个余量），读一次输出快照：
//   - 若这一行已经在 cmdAsk 里打过、且在 r.rn.Ask 之前——此刻应当已经
//     能在快照里看到 agent id；
//   - 且此刻 Ask 还没返回，快照里不该出现 writeAskRecord 才会打的
//     "outcome"（ask.go 现查：writeAskRecord 第一条 printf 就是
//     "outcome %s  agent %s\n"，这是既有代码，本用例只读不改）。
//
// 如果实现把这一行挪到了 Ask 返回之后（或者压根没加），探测点的快照里
// 要么没有 agent id，要么 agent id 和 "outcome" 同时缺席或同时在场，
// 与「先有 id、后有 outcome」这个次序不符，会被下面的断言抓住。
func TestAskPrintsProgressLineBeforeSpawnReturns(t *testing.T) {
	svc, f := newTestService(t)
	rn := newInFlightRunner(t, svc)

	const convID = "conv_ux1_ask_progress0000000000000000"
	f.seedConversation(convID, "ux1 进度行用例")
	f.seedParticipant(t, convID, inFlightAgentID)
	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "seed-msg")

	// 建首轮会话：不经 REPL、不经 cmdAsk，out 这时还没有开始写。
	first, err := rn.Ask(context.Background(), convID, inFlightAgentID)
	if err != nil {
		t.Fatalf("建立首轮会话失败: %v；Record=%+v", err, first)
	}
	if !first.Fresh {
		t.Fatalf("前置条件不成立：首轮应当是 Fresh，实际不是: %+v", first)
	}
	// 第二轮要问的内容直接塞库，不经 postUserText——避免"带文本时的回显"
	// 混进下面的探测判据。
	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "second-msg")

	// 对照组：只跑 "/use"，证明它自己不会印出 agent id。
	controlOut, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n")
	if err != nil {
		t.Fatalf("对照组 Run 失败: %v；完整输出:\n%s", err, controlOut)
	}
	if strings.Contains(controlOut, inFlightAgentID) {
		t.Fatalf("前置条件不成立：单独一条 /use 的输出里就含 agent id %q，"+
			"下面「探测点含 agent id」这条断言就证明不了是 cmdAsk 打的；对照组完整输出:\n%s",
			inFlightAgentID, controlOut)
	}

	var out syncBuffer
	r := New(newInFlightConfig(), svc, rn,
		strings.NewReader("/use "+convID+"\n/ask "+inFlightAgentID+"\n"),
		&out, RulesContext{})

	done := runREPLInBackground(r)

	time.Sleep(inFlightProbeDelay)

	select {
	case err := <-done:
		t.Fatalf("REPL.Run 在探测点（%v）之前就返回了（err=%v）——说明第二轮没有真的带上 "+
			"--resume 进入 700ms 睡眠窗口，前置条件不成立，下面的断言无法证明任何事；"+
			"完整输出:\n%s", inFlightProbeDelay, err, out.String())
	default:
	}

	snap := out.String()
	if !strings.Contains(snap, inFlightAgentID) {
		t.Fatalf("探测点（%v，落在第二轮 700ms 睡眠窗口正中间）的输出里还没有出现 agent id %q——"+
			"起进程之前那一行要么没加、要么被挪到了 r.rn.Ask 返回之后才打；此刻的输出:\n%q",
			inFlightProbeDelay, inFlightAgentID, snap)
	}
	if strings.Contains(snap, "outcome") {
		t.Fatalf("前置条件不成立：探测点的输出里已经出现了 writeAskRecord 才会打的 \"outcome\"，"+
			"说明第二轮的 Ask 已经返回，本次探测没有真的卡在「起进程之后、返回之前」这个窗口里；"+
			"此刻的输出:\n%q", snap)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("REPL.Run 失败: %v；完整输出:\n%s", err, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("REPL.Run 在探测之后没有及时结束；完整输出:\n%s", out.String())
	}

	final := out.String()
	if !strings.Contains(final, "outcome") {
		t.Errorf("最终输出里应当出现 writeAskRecord 打的结局行；完整输出:\n%s", final)
	}
}

// ---------------------------------------------------------------------------
// 验收 8：起进程前被拒时，那一行仍然出现，且紧跟着拒绝原因
// ---------------------------------------------------------------------------
//
// 用 ask_test.go 已有的 newTestRunner：ag_alpha 的 provider "pv_alpha"
// 不在 internal/provider 的 registry 里，Ask 会在 provider.Lookup 那一步
// 被 r.refuse 掉，真的一次子进程都不起（同一份夹具，ask_test.go 好几条
// 既有用例已经在依赖这个性质，见 newTestRunner 自己的注释）。
//
// 断言两层：
//  1. agent id 第一次出现的位置早于 "outcome" 第一次出现的位置——
//     防「把这一行挪到 Ask 内部/返回之后才打」这种坏修法（4.2 的取舍，
//     契约点名要求钉住）；
//  2. agent id 所在那一整行结束之后，紧接着的下一行就是 "outcome " 开头
//     ——防中间插入别的东西、或者干脆把两件事的顺序在别处重新拆开。
func TestAskProgressLineAppearsWhenRefusedAndIsImmediatelyFollowedByReason(t *testing.T) {
	svc, f := newTestService(t)
	rn := newTestRunner(t, svc)

	// x1 成员闸落地后 /ask 要求 agent 是本会话的活跃成员，先让 ag_alpha
	// 成为活跃成员，拒绝点才仍然落在 provider.Lookup（pv_alpha 未注册），
	// 不会被成员闸提前拦下。用 seedParticipant 预置、用 /use 切过去，
	// 不用 /join：/join 自己的回显里也含 "ag_alpha"，会抢在下面
	// "正在问"那一行前面被 strings.Index 命中，污染这条用例本身要测的
	// "agent id 第一次出现的位置"这个判据。
	const convID = "conv_ux1_progress_join00000000000000000"
	f.seedConversation(convID, "验收 8 用例")
	f.seedParticipant(t, convID, "ag_alpha")

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask ag_alpha\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	idAt := strings.Index(out, "ag_alpha")
	if idAt < 0 {
		t.Fatalf("输出里没有出现 agent id ag_alpha；完整输出:\n%s", out)
	}
	outcomeAt := strings.Index(out, "outcome")
	if outcomeAt < 0 {
		t.Fatalf("输出里没有出现 writeAskRecord 的 outcome 行；完整输出:\n%s", out)
	}
	if !(idAt < outcomeAt) {
		t.Fatalf("agent id 第一次出现的位置（%d）不在 outcome 行（%d）之前——"+
			"起进程之前那一行没有出现在拒绝结果之前；完整输出:\n%s", idAt, outcomeAt, out)
	}

	firstNL := strings.IndexByte(out[idAt:], '\n')
	if firstNL < 0 {
		t.Fatalf("agent id 所在那一行没有换行结尾；完整输出:\n%q", out)
	}
	afterFirstLine := out[idAt+firstNL+1:]
	nextLine := afterFirstLine
	if nlIdx := strings.IndexByte(afterFirstLine, '\n'); nlIdx >= 0 {
		nextLine = afterFirstLine[:nlIdx]
	}
	if !strings.HasPrefix(afterFirstLine, "outcome ") {
		t.Errorf("起进程之前那一行之后，紧接着的下一行应当是 writeAskRecord 的 outcome 行"+
			"（4.2 明写接受「先打正在问、紧接着一行拒绝」这个次序），实际紧跟着的是: %q；"+
			"完整输出:\n%s", nextLine, out)
	}
}
