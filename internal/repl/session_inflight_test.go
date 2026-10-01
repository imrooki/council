package repl

// session_inflight_test.go 补 Step 9 片 1 里两条一个用例都没有的消费分支
// （ask.go 的 cmdSession 对 info.InFlight 印 busy、cmdSessionRebuild 对
// errors.Is(err, turn.ErrTurnInFlight) 印那句"什么都没丢"）。本文件不改
// ask.go 或 internal/turn 任何一行，只加测试。
//
// 「在飞」这个状态没法靠 REPL 自己的单 goroutine 读循环造出来：用户不可能
// 在 /ask 阻塞时再敲 /session。做法是另起一条 goroutine，直接对同一个
// *turn.Runner 调 Ask，让它占住 (conv, agent) 的闸，同时把 /session、
// /session rebuild 喂给跑在当前 goroutine 里的 REPL（ask_test.go 的
// runREPLRunner 能塞一个真正的 *turn.Runner，本文件照它的体例走）。
//
// 本包拿不到 turn.Runner 内部的 spawn 字段（未导出，turn 包也没给任何测试
// 用的导出钩子/setter），没法像 internal/turn 包自己的用例
// （gate_concurrency_test.go 的 raceSpawner）那样塞一个纯内存、可控睡眠的
// 假 Spawner；也不该为此往 internal/turn 加一个只给测试用的导出口子——
// 那是实现文件，且会给生产代码留一个死键。改用本仓库已有先例的
// helper-process 套路（internal/app/smoke_test.go 的 TestMain，契约
// docs/13_STEP10_CONTRACT.md 6.1 明文背书）：测试二进制自己在环境变量
// COUNCIL_FAKE_CLI=1 时改行扮演 provider CLI，config 里
// Providers["claude"].Command 写 os.Args[0]（测试二进制自己，是 .exe 不是
// .cmd，不会撞 internal/turn/spawn.go 那道 ErrShimNewlineArg 闸）。
// 扮演的那一支只看 argv 里有没有 --resume：没有（首轮 bootstrap）立刻
// 应答，有（第二轮，接上首轮会话）先 time.Sleep(inFlightSleep) 再应答——
// 跨平台成立，不需要 .cmd、不需要 ping、不需要 //go:build windows。

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// inFlightAgentID 是本文件专用的测试 agent id，provider 是真正登记过的
// "claude"（internal/provider 的 registry 里有，见 provider.Lookup），
// 这样 Ask 不会在 provider.Lookup 那一步就被 refuse 掉，真的会走到 spawn。
// 这与 ask_test.go 的 newTestRunner 故意反过来：那边用 newTestREPLConfig
// 里的 pv_alpha 之类未登记的 provider，是为了不起子进程也能测前置条件；
// 本文件恰恰需要真的起一次子进程才能制造"在飞"这个状态。
const inFlightAgentID = "ag_wait"

// inFlightSleep 是假 CLI 收到 --resume 之后的睡眠时长。
//
// 选 700ms 不是编的：本机实测（下面两条用例各自连跑 5 次，见任务报告）
// 整条用例稳定落在 0.75-0.76s。没有单独量过"第二轮从起 goroutine 到
// Ask 返回"这一段本身的开销，只能反推：0.75s 总耗时减掉 700ms 的
// time.Sleep，首轮 spawn（一次起子进程）、第二轮 spawn 的进程创建开销、
// 与两次 REPL 探测（runREPLRunner）合计约 50ms——这个测试二进制本身
// 不小，Windows 上把它当子进程拉起来的开销比一个 .cmd 垫片明显更高，
// 这 50ms 的大头在这里，不在 time.Sleep 本身。inFlightProbeDelay（见下）
// 取 350ms，落在 700ms 睡眠窗口正中间，距子进程真正起来（几十毫秒内）
// 与睡完（700ms）两头都留了 250ms 以上的余量，不是卡着时间凑数。
const inFlightSleep = 700 * time.Millisecond

// inFlightProbeDelay 是起完 goroutine 之后、开始探测之前的等待：确保假
// CLI 已经真正进了 time.Sleep 的区间、闸已经被占住，而不是探测代码跑得
// 比子进程启动还快。取 inFlightSleep 的一半（350ms），前后各留出足够
// 余量，不贴着任何一头的边界。
const inFlightProbeDelay = inFlightSleep / 2

// fakeClaudeSuccessJSON 逐字节抄自 internal/turn/fixtures_test.go 的常量
// claudeT1JSON（同一份字节 internal/app/smoke_test.go 也整段复制过一份，
// 常量名同样是 fakeClaudeSuccessJSON；两处的注释都说明它本身又逐字来自
// E:\<tmp-root>\council\step0-probe\claude-t1.json，claude 冷启动首轮的
// 真实抓包）。不手写：internal/provider/claude.go 的 ParseTurn
// （claudeSuccess 函数）是 fail-closed 的五字段合取判据，手写的 JSON 极
// 容易漏掉其中一个字段而不自知，契约 6.1 明写这条。三处独立复制、不跨包
// 引用是既有约定（这几个常量都是 _test.go 里的包内私有量）；将来重新抓
// claude-t1.json 时，internal/provider/fixtures_test.go、
// internal/turn/fixtures_test.go、internal/app/smoke_test.go 与本文件四处
// 要一起改。
const fakeClaudeSuccessJSON = `{"duration_api_ms":1939,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":0.9551999999999999,"usage":{"input_tokens":2,"cache_creation_input_tokens":47749,"cache_read_input_tokens":0,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":47749,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":47749,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":47749},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":2,"outputTokens":4,"cacheReadInputTokens":0,"cacheCreationInputTokens":47749,"webSearchRequests":0,"costUSD":0.9551999999999999,"contextWindow":1000000,"maxOutputTokens":64000,"thinkingTokens":0,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"OK","ttft_ms":2016,"type":"result","duration_ms":3566,"uuid":"78205525-edf7-485a-b1d4-c9cbccd75462","ttft_stream_ms":1306,"time_to_request_ms":98,"first_content_frame_ms":1307,"queued_turn_count":0,"result_index":0}`

// TestMain 是本包的 helper-process 套路：COUNCIL_FAKE_CLI=1 时，这个测试
// 二进制本身被 turn.Runner 当作 provider CLI 起来（newInFlightConfig 把
// Providers["claude"].Command 设成 os.Args[0]），这时不跑 go test 的用例
// 集，改去扮演一次「provider CLI 的一轮调用」；否则原样 os.Exit(m.Run())，
// 对本包其余既有用例零影响。
//
// 体例抄自 internal/app/smoke_test.go 的 TestMain（同一份契约
// docs/13_STEP10_CONTRACT.md 6.1 明写的做法）；internal/repl 包此前没有
// TestMain（现查过零命中），这是本包新增的第一个、也是唯一一个。
func TestMain(m *testing.M) {
	if os.Getenv("COUNCIL_FAKE_CLI") == "1" {
		runFakeClaudeCLI()
		return
	}
	os.Exit(m.Run())
}

// runFakeClaudeCLI 扮演一次「claude CLI 的一轮调用」，只给本文件下面两条
// 在飞用例用。argv 里没有 --resume 就是首轮 bootstrap，立刻应答；有
// --resume 就是接上首轮的第二轮，先睡 inFlightSleep 再应答——这样才能在
// 第二轮的窗口内稳定制造"在飞"状态。不解析其余参数：本文件不验证 claude
// 命令行拼得对不对，那件事 internal/provider 与 internal/turn 已经有专门
// 用例钉着。
func runFakeClaudeCLI() {
	_, _ = io.ReadAll(os.Stdin)
	resume := false
	for _, a := range os.Args[1:] { // os.Args[0] 是这个测试二进制自己的路径，不是 claude 的 flag
		if a == "--resume" {
			resume = true
			break
		}
	}
	if resume {
		time.Sleep(inFlightSleep)
	}
	fmt.Fprint(os.Stdout, fakeClaudeSuccessJSON)
	os.Exit(0)
}

// newInFlightConfig 造一份只有一个 agent 的配置：provider 是登记过的
// "claude"，Command 是测试二进制自己（os.Args[0]）。权限档用 read，跟
// claudePermissionArgs 对 read 发哪几个 flag 无关——Command 现在是 .exe
// 不是 .cmd，spawnProcess 里那道只查 .cmd/.bat 参数换行的 ErrShimNewlineArg
// 闸本来就不会命中，选 read 只是图最简单，不是在绕这道闸。
func newInFlightConfig() *config.Config {
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: os.Args[0]},
		},
		Agents: map[string]config.AgentConfig{
			inFlightAgentID: {
				DisplayName: "Wait",
				Provider:    "claude",
				Model:       "claude-fable-5-1",
				Effort:      "high",
				Role:        "在飞用例专用",
				Permissions: config.PermissionRead,
				Mode:        config.ModeDiscussion,
			},
		},
	}
}

// newInFlightRunner 把 svc、config 拼成一个真正能起子进程（其实是重新拉起
// 自己）的 *turn.Runner，并设好 COUNCIL_FAKE_CLI=1——claude 适配器的
// Command.Env 恒为 nil（原样继承），子进程会带着这个变量，t.Setenv 会在
// 用例结束时自动还原，不污染同包其它用例。
func newInFlightRunner(t *testing.T, svc *conversation.Service) *turn.Runner {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	root := t.TempDir()
	cfg := newInFlightConfig()
	return turn.NewRunner(cfg, svc, filepath.Join(root, "workdir"), turn.RulesBlocks{})
}

// nextColumnFor 在 out 里找含 agentID 的那一行，按空白切成字段，返回最后
// 一个字段——即 cmdSession 按 "%s\t%s\t%s\t%s\n" 写的 NEXT 列
// （AGENT / PROVIDER_SESSION / LAST_TURN_AT / NEXT）。
//
// 不直接对整份输出做 strings.Contains(out, "busy")：那是本任务书点名的
// 坑——busy 是个短词，可能被别处的输出撞见，必须先定位到这个 agent 的
// 那一行，再看指定的列。AGENT 列是 ASCII id（不含空白），PROVIDER_SESSION
// 要么是占位符 "-" 要么是不含空白的会话 id，LAST_TURN_AT 是 formatTime
// 产出的 RFC3339（不含内部空格）或占位符，这三列都不会在中间引入多余的
// 空白字段，所以"最后一个字段"稳定对应 NEXT，不依赖 tabwriter 具体的
// 列宽或对齐字节数。
func nextColumnFor(t *testing.T, out, agentID string) string {
	t.Helper()
	line := lineContaining(t, out, agentID)
	fields := strings.Fields(line)
	if len(fields) == 0 {
		t.Fatalf("agent %q 那一行切不出任何字段；行内容: %q", agentID, line)
	}
	if fields[0] != agentID {
		t.Fatalf("agent %q 那一行的第一个字段是 %q，不是 agent id 本身，"+
			"说明找错了行或者列序变了；行内容: %q", agentID, fields[0], line)
	}
	return fields[len(fields)-1]
}

// TestSessionNextColumnBusyDuringInFlightThenWarm 覆盖 cmdSession 里
// info.InFlight 的消费分支：一轮在飞时 /session 的 NEXT 列必须是 busy，
// 在飞状态结束之后同一个 /session 必须印 warm——不是 busy，也不是
// cold（本用例的第二轮几秒钟前才提交，远没到 CacheTTL）。
// 只测 busy 那一半测不出"恒印 busy"的坏实现，这里的 warm 对照就是防它的。
func TestSessionNextColumnBusyDuringInFlightThenWarm(t *testing.T) {
	svc, f := newTestService(t)
	rn := newInFlightRunner(t, svc)

	// 不走 REPL 的 /new 与 /join：runREPLRunner 内部固定用
	// newTestREPLConfig()（ag_alpha / ag_mid / ag_zeta 那三个，provider
	// 是未登记的 pv_*）构造 REPL，与本文件传给 *turn.Runner 的
	// newInFlightConfig()（唯一的 agent 是 ag_wait，provider 是真登记过的
	// claude）是两份不同的配置——REPL 那份不认得 ag_wait，/join 会被
	// TestJoinRejectsUnknownAgent 锁住的那条防线拒绝。cmdSession /
	// cmdSessionRebuild 只读 r.svc.Participants 与 r.rn，从不读 r.cfg，
	// 所以直接用 fakeStore 的 seedConversation / seedParticipant 把这一行
	// 塞进库里，跳过 /join 对 REPL 那份配置的依赖，两份配置的差异就不再
	// 影响这两条命令的行为。
	const convID = "conv_inflight_busy0000000000000000"
	f.seedConversation(convID, "在飞用例")
	f.seedParticipant(t, convID, inFlightAgentID)

	// 先跑通一轮，把 provider 会话建立起来——否则第二轮（也是在飞那一轮）
	// 会落进 ok==false 那一支，/session 印的是 fresh 不是 busy，
	// 问不到这条用例要问的问题（任务书原话）。
	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "seed-msg")
	first, err := rn.Ask(context.Background(), convID, inFlightAgentID)
	if err != nil {
		t.Fatalf("建立首轮会话失败: %v；Record=%+v", err, first)
	}
	if !first.Fresh {
		t.Fatalf("前置条件不成立：首轮应当是 Fresh，实际不是: %+v", first)
	}

	// 第二轮要有 delta 才会真的起 spawn（没有 delta 会撞 ErrNoDelta，
	// 起进程之前就返回，根本进不了睡眠区间）。
	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "second-msg")

	// fakeStore（conversation_test.go）从 2026-09-25 起加了锁，这条 goroutine 与
	// 下面主 goroutine 里的 /session 同时碰 svc/fakeStore 不再有并发 map 读写的问题
	// （原先没有锁，这段时间线是为了避开它写的。它在墙钟上成立，但时间上错开不构成
	// happens-before，-race 照样报）。
	// 下面的时间线现在只说明探测为什么落在在飞窗口里：这条 goroutine 对 store 的访问只集中在两
	// 段——起步时的 History/Conversation 读（Ask 顶部，几毫秒内做完）与
	// spawn 真正返回之后的 RecordTurn 写（子进程睡完 inFlightSleep=700ms
	// 才会发生）；中间那段睡眠期里它完全不碰 svc。下面的探测统一发生在
	// inFlightProbeDelay=350ms（inFlightSleep 的一半），正好落在这两段
	// 之间的空档，不与它们重叠。
	// （闸本身 g.mu.Lock() 是 AskUpTo 进场做的第一件事，在任何 store 访问
	// 之前——goroutine 一被调度到就几乎立刻拿到闸，350ms 的余量是留给
	// "goroutine 何时真正开始跑、子进程何时真正起来"，不是留给 time.Sleep
	// 本身——那只要 0ms 就够，本文件顶部 inFlightSleep 的注释量过这笔账。）
	var wg sync.WaitGroup
	var secondErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, secondErr = rn.Ask(context.Background(), convID, inFlightAgentID)
	}()
	t.Cleanup(wg.Wait) // 即使中途 Fatalf，也不让这条 goroutine 泄漏到别的用例

	time.Sleep(inFlightProbeDelay)

	busyOut, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session\n")
	if err != nil {
		t.Fatalf("在飞时 /session 的 Run 失败: %v；输出:\n%s", err, busyOut)
	}
	if got := nextColumnFor(t, busyOut, inFlightAgentID); got != "busy" {
		t.Errorf("在飞时 /session 里 %s 那一行 NEXT 列是 %q，期望 busy；完整输出:\n%s",
			inFlightAgentID, got, busyOut)
	}

	wg.Wait()
	if secondErr != nil {
		t.Fatalf("第二轮 Ask 失败: %v", secondErr)
	}

	// 对照组：在飞状态结束之后，同一个 /session 必须印 warm。
	warmOut, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session\n")
	if err != nil {
		t.Fatalf("不在飞时 /session 的 Run 失败: %v；输出:\n%s", err, warmOut)
	}
	if got := nextColumnFor(t, warmOut, inFlightAgentID); got != "warm" {
		t.Errorf("不在飞时 /session 里 %s 那一行 NEXT 列是 %q，期望 warm；完整输出:\n%s",
			inFlightAgentID, got, warmOut)
	}
}

// TestSessionRebuildDuringInFlightRoundIsRejectedRowUnchangedThenSucceeds
// 覆盖 cmdSessionRebuild 里 errors.Is(err, turn.ErrTurnInFlight) 的消费
// 分支：一轮在飞时 /session rebuild 必须被拒绝、且库里那一行原值不变
// （"什么都没丢"不能只信屏幕，要查库）；在飞状态结束之后，同一个
// /session rebuild 对同一个 agent 应当真的成功、把那一行丢掉——这条
// 对照防的是"这个 Runner 对这个 agent 恒拒绝 rebuild"一类坏实现，
// 不然只测被拒的那一半测不出真正的因果关系。
func TestSessionRebuildDuringInFlightRoundIsRejectedRowUnchangedThenSucceeds(t *testing.T) {
	svc, f := newTestService(t)
	rn := newInFlightRunner(t, svc)

	// 同上一条用例：绕开 /join 对 REPL 那份（与 Runner 无关的）
	// newTestREPLConfig() 的依赖，直接塞库。
	const convID = "conv_inflight_rebuild000000000000000"
	f.seedConversation(convID, "在飞用例")
	f.seedParticipant(t, convID, inFlightAgentID)

	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "seed-msg")
	if first, err := rn.Ask(context.Background(), convID, inFlightAgentID); err != nil {
		t.Fatalf("建立首轮会话失败: %v；Record=%+v", err, first)
	}
	before, found, err := svc.ProviderSession(context.Background(), convID, inFlightAgentID)
	if err != nil || !found {
		t.Fatalf("首轮之后读不到 provider_sessions 行: found=%v err=%v", found, err)
	}

	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "second-msg")

	// 探测落在在飞窗口里的时间线同上一条用例（map 的并发安全现在由 fakeStore 的锁担保）：这条 goroutine 只在起步
	// （几毫秒内）与 spawn 真正返回之后（inFlightSleep=700ms）碰
	// svc/fakeStore，inFlightProbeDelay=350ms 的探测落在两者之间的空档，
	// 不重叠。
	var wg sync.WaitGroup
	var secondErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, secondErr = rn.Ask(context.Background(), convID, inFlightAgentID)
	}()
	t.Cleanup(wg.Wait)

	time.Sleep(inFlightProbeDelay)

	rebuildOut, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session rebuild "+inFlightAgentID+"\n")
	if err != nil {
		t.Fatalf("在飞时 /session rebuild 的 Run 失败: %v；输出:\n%s", err, rebuildOut)
	}

	// 不锁那句人话的逐字措辞（09a 契约「不属于契约面的东西」）。只锁两件事：
	//  (1) 不是通用的"重建失败"那条——ask.go 里 errors.Is(err, ErrTurnInFlight)
	//      这条分支若被删掉，会退到 r.reportErr("重建失败", err) 这一支;
	//  (2) 含"什么都没丢"——这是 ask.go cmdSessionRebuild 里那句话唯一的语义
	//      锚点，原文就是这四个字，锚它是因为它就是"没有副作用"这件事本身的
	//      措辞，不是随便挑的排版；换一种说法但意思不变的实现会被这条锚点
	//      放过，但不会被误伤成"重建失败"。
	if strings.Contains(rebuildOut, "重建失败") {
		t.Errorf("在飞时 /session rebuild 不该走到通用的重建失败分支；完整输出:\n%s", rebuildOut)
	}
	if !strings.Contains(rebuildOut, "什么都没丢") {
		t.Errorf("在飞时 /session rebuild 的输出里应当含有“什么都没丢”这个语义片段；完整输出:\n%s", rebuildOut)
	}

	// 判据是读回来的那一行，不是屏幕输出：此刻第二轮还在睡眠里，还没有
	// 提交任何东西，立刻读库必须仍是首轮提交的原值。
	mid, found, err := svc.ProviderSession(context.Background(), convID, inFlightAgentID)
	if err != nil || !found {
		t.Fatalf("rebuild 被拒之后读不到那一行: found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(mid, before) {
		t.Fatalf("rebuild 被拒之后那一行变了：\n  before=%+v\n  mid   =%+v", before, mid)
	}

	wg.Wait()
	if secondErr != nil {
		t.Fatalf("第二轮 Ask 失败: %v", secondErr)
	}

	// 对照组：在飞状态结束之后，rebuild 应当真的成功——查库，不信屏幕。
	successOut, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session rebuild "+inFlightAgentID+"\n")
	if err != nil {
		t.Fatalf("不在飞时 /session rebuild 的 Run 失败: %v；输出:\n%s", err, successOut)
	}
	if strings.Contains(successOut, "什么都没丢") {
		t.Errorf("不在飞时 /session rebuild 不该再显示「在飞」那句话；完整输出:\n%s", successOut)
	}
	if _, stillFound, err := svc.ProviderSession(context.Background(), convID, inFlightAgentID); err != nil {
		t.Fatalf("查库失败: %v", err)
	} else if stillFound {
		t.Errorf("不在飞时 rebuild 应当真的丢弃这一行，实际它还在")
	}
}
