package app

// 本文件是 Step 10 C 片的 C1：唯一一条证明「这七个包接得起来」的端到端烟测。
//
// 为什么必须存在：现查过，turn.NewRunner 在 internal/app/app.go 的 Run 里（rn =
// turn.NewRunner(...) 那一句，按符号定位，不钉行号——立的是「不钉行号」这条规矩
// 本身，不是这个号今天准不准，与本文件其余「现查」引用同一体例）出现一次，而
// internal/app 的既有测试里零命中——没有任何一条用例真的把 config /
// conversation / repl / store / turn / provider 这条链子从头跑到尾。这不是假设性
// 风险：commit 08f6b7e 改了三个签名、漏改四处调用点，被另一个包的编译失败掩盖住，
// 就那么提交出去了（现查 git log，commit 08b9e5f "feat(step8a): Council 自己的
// 规则文件进身份块与指纹，补 argv 总长闸"）。单元测试各自 mock 掉了邻居，谁也没拦住
// 这类「装配对不对」的失败；这条用例专门堵这个缝隙，其余正确性问题仍旧留给各包
// 自己的用例。
//
// 形状：helper-process 套路（标准库 os/exec 自己的测试、docker/docker 等大量项目
// 用同一招）。测试二进制在 COUNCIL_FAKE_CLI=1 时不跑 go test 的用例集，改扮演一个
// provider CLI：吃掉 argv 与 stdin，往 stdout 打一份逐字节抄来的成功 JSON，退出码 0。
// 配置里 provider 的 command 写 os.Args[0]（测试二进制自己），它是 .exe 不是 .cmd，
// 不会撞 internal/turn/spawn.go 里那道 ErrShimNewlineArg 闸（那道闸只查 .cmd/.bat）。

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/turn"

	_ "modernc.org/sqlite"
)

// TestMain 是本包目前唯一的一个（现查过 internal/app/app_test.go 没有 TestMain，
// 零命中，所以在这里新写而不是拆成第二个）。COUNCIL_FAKE_CLI=1 时整个测试二进制
// 改行去扮演 provider CLI；否则原样跑 go test 的用例集，对包里其余测试零影响。
func TestMain(m *testing.M) {
	if os.Getenv("COUNCIL_FAKE_CLI") == "1" {
		runFakeProviderCLI()
		return
	}
	os.Exit(m.Run())
}

// runFakeProviderCLI 扮演一次「provider CLI 的一轮调用」：读掉 argv（不解析，
// 这条烟测要验的是装配对不对，不是 claude 适配器的命令行拼得对不对——那件事
// internal/provider 与 internal/turn 已经有专门的用例钉着）与 stdin，
// 往 stdout 打一份成功 JSON，退出码 0。S10D_FAKE_STDOUT_FILE 非空时改吐该文件
// 的字节（读不到就退出码 3），供 /log 标记用例喂 grok / agy 形状的原件。
func runFakeProviderCLI() {
	_, _ = io.ReadAll(os.Stdin)
	if p := os.Getenv("S10D_FAKE_STDOUT_FILE"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "S10D_FAKE_STDOUT_FILE: %v\n", err)
			os.Exit(3)
		}
		_, _ = os.Stdout.Write(b)
		os.Exit(0)
	}
	fmt.Fprint(os.Stdout, fakeClaudeSuccessJSON)
	os.Exit(0)
}

// fakeClaudeSuccessJSON 逐字节抄自 internal/turn/fixtures_test.go 的常量
// claudeT1JSON（那份注释自己说明它又逐字来自
// E:\Isolating_test\council\step0-probe\claude-t1.json，claude 冷启动首轮的真实抓包；
// internal/provider/fixtures_test.go 里也有同一份字节，常量名同样是 claudeT1JSON）。
//
// 为什么抄字节而不是手写一份「看起来像」的 JSON：internal/provider/claude.go 的
// ParseTurn（claudeSuccess 函数）是 fail-closed 的五字段合取判据——
// type=="result" && !IsError && Subtype=="success" && TerminalReason=="completed"
// && StopReason ∈ {end_turn,max_tokens} && APIErrorStatus 缺席/null。手写的 JSON
// 极容易漏掉其中一个字段而不自知，那正是这条烟测要防的「看似接得上、实则某个字段
// 拼错」，也正是这份任务书特别强调「不要手写」的原因。
//
// 两份既有常量都是 _test.go 里的包内私有量（provider_test 与 turn_test），
// 跨包引用不到，只能整段复制字节；将来重新抓 claude-t1.json 时，
// internal/provider/fixtures_test.go、internal/turn/fixtures_test.go 与本文件
// 三处要一起改，改漏一处会让这条烟测继续用一份过期的抓包。
const fakeClaudeSuccessJSON = `{"duration_api_ms":1939,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":0.9551999999999999,"usage":{"input_tokens":2,"cache_creation_input_tokens":47749,"cache_read_input_tokens":0,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":47749,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":47749,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":47749},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":2,"outputTokens":4,"cacheReadInputTokens":0,"cacheCreationInputTokens":47749,"webSearchRequests":0,"costUSD":0.9551999999999999,"contextWindow":1000000,"maxOutputTokens":64000,"thinkingTokens":0,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"OK","ttft_ms":2016,"type":"result","duration_ms":3566,"uuid":"78205525-edf7-485a-b1d4-c9cbccd75462","ttft_stream_ms":1306,"time_to_request_ms":98,"first_content_frame_ms":1307,"queued_turn_count":0,"result_index":0}`

// TestSmokeAllPackagesWireTogether 驱动一次完整的 app.Run：
// /new -> /join -> 裸文本 -> /ask -> /log -> /quit，
// provider CLI 换成测试二进制自己（COUNCIL_FAKE_CLI=1）。
// 断言全部是行为判据——直接查 SQLite 库，不看屏幕输出，因为屏幕格式属于
// 「随便改，测试不许钉」的那一类（各命令自己的用例已经钉过），这条用例只关心
// 「跑完之后库里躺着的事实对不对」。
func TestSmokeAllPackagesWireTogether(t *testing.T) {
	isolateAppEnv(t)
	// 让本测试二进制被 Runner 起进程时能认出自己该扮演 provider CLI。
	// Command.Env 在 claude 适配器里恒为 nil（原样继承），子进程会带着这个变量。
	t.Setenv("COUNCIL_FAKE_CLI", "1")

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")

	cfgBody := fmt.Sprintf(`data_dir = %s

[providers.claude]
command = %s

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "冒烟测试用的假成员，不代表任何真实角色"
permissions = "read"
mode = "discussion"
`, tomlLiteral(dataDir), tomlLiteral(os.Args[0]))
	cfgPath := writeTestConfig(t, dir, cfgBody)

	script := strings.Join([]string{
		"/new t",
		"/join c1",
		"裸文本消息，验证端到端",
		"/ask c1",
		"/log",
		"/quit",
	}, "\n") + "\n"

	ctx := context.Background()
	out, errOut, err := runAppForTest(t, ctx, cfgPath, strings.NewReader(script))
	if err != nil {
		t.Fatalf("app.Run 应当返回 nil，实际返回 %v；\nOut:\n%s\nErr:\n%s", err, out, errOut)
	}

	// dbFileName 是 internal/store 包内未导出的常量（值为 "council.db"），
	// 跨包引不到，这里按 open.go 的注释原样写出字面量。
	dbPath := filepath.Join(dataDir, "council.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("打开 %s 失败：%v", dbPath, err)
	}
	defer db.Close()

	// 断言 1：messages 里 user 与 agent 各一条，agent 那条三个脑子列非空。
	//
	// 不按位置取 msgs[0]/msgs[1]：现查 internal/turn/runner.go 的 askUpToWith，
	// 冷启动（bootstrap，即这个 agent 在这个会话里从未开过口，正是本用例的场景）
	// 在真正起进程之前会先追加一条 KindSystem 的 "session_opening" 面包屑
	// （sender 是 conversation.ReservedSenderSystem），所以 messages 实际至少
	// 3 条（user、system、agent），agent 不一定是第二条。这条面包屑本身
	// 不在本用例的断言范围内（不管 meta.event、不管它自己的 sender/脑子列），
	// 只按 kind 过滤取 user 与 agent 两行，不假设总行数与顺序，多出来的行
	// 原样记进 t.Logf，不静默吞掉也不硬去适配。
	type msgRow struct {
		seq      int64
		kind     string
		senderID string
		provider string
		model    string
		effort   string
	}
	rows, err := db.QueryContext(ctx, `SELECT seq, kind, sender_id, sender_provider, sender_model, sender_effort FROM messages ORDER BY seq`)
	if err != nil {
		t.Fatalf("查询 messages 失败：%v", err)
	}
	var msgs []msgRow
	for rows.Next() {
		var m msgRow
		if scanErr := rows.Scan(&m.seq, &m.kind, &m.senderID, &m.provider, &m.model, &m.effort); scanErr != nil {
			rows.Close()
			t.Fatalf("扫 messages 一行失败：%v", scanErr)
		}
		msgs = append(msgs, m)
	}
	if closeErr := rows.Err(); closeErr != nil {
		t.Fatalf("遍历 messages 失败：%v", closeErr)
	}
	rows.Close()

	var userRows, agentRows []msgRow
	for _, m := range msgs {
		switch m.kind {
		case "user":
			userRows = append(userRows, m)
		case "agent":
			agentRows = append(agentRows, m)
		}
	}
	if len(userRows) != 1 || len(agentRows) != 1 {
		t.Fatalf("messages 应恰好含 1 条 user 与 1 条 agent，实际 user=%d agent=%d；全部 %d 行：%+v\nOut:\n%s\nErr:\n%s",
			len(userRows), len(agentRows), len(msgs), msgs, out, errOut)
	}
	t.Logf("messages 共 %d 行（含非 user/agent 的面包屑等）：%+v", len(msgs), msgs)
	userMsg, agentMsg := userRows[0], agentRows[0]

	if userMsg.senderID != "user" {
		t.Errorf("user 消息的 sender_id 应为保留 sender \"user\"，实际 %q", userMsg.senderID)
	}
	if userMsg.provider != "" || userMsg.model != "" || userMsg.effort != "" {
		t.Errorf("user 消息不应带脑子列，实际 provider=%q model=%q effort=%q", userMsg.provider, userMsg.model, userMsg.effort)
	}

	if agentMsg.senderID != "c1" {
		t.Errorf("agent 消息的 sender_id 应为 \"c1\"，实际 %q", agentMsg.senderID)
	}
	// 三列全部非空：internal/turn/runner.go 的 askUpToWith 构造 agent 消息
	// 那段（SenderProvider/SenderModel/SenderEffort）把它们原样填成 ac.Provider /
	// ac.Model / ac.Effort（请求时声明的档位，不是实测档位），三者在本用例的配置里
	// 都非空（provider=claude model=opus effort=high），claude 的 Capabilities().Effort
	// 是 EffortPerTurnFlag（不丢弃），所以这里不放松成"允许 effort 为空"。
	if agentMsg.provider == "" || agentMsg.model == "" || agentMsg.effort == "" {
		t.Errorf("agent 消息的三个脑子列都应非空，实际 provider=%q model=%q effort=%q", agentMsg.provider, agentMsg.model, agentMsg.effort)
	}

	// 断言 2：turn_usage 恰好一行，outcome == int(turn.OutcomeSpoke)。
	var outcome int
	usageRowCount := 0
	uRows, err := db.QueryContext(ctx, `SELECT outcome FROM turn_usage`)
	if err != nil {
		t.Fatalf("查询 turn_usage 失败：%v", err)
	}
	for uRows.Next() {
		usageRowCount++
		if scanErr := uRows.Scan(&outcome); scanErr != nil {
			uRows.Close()
			t.Fatalf("扫 turn_usage 一行失败：%v", scanErr)
		}
	}
	if closeErr := uRows.Err(); closeErr != nil {
		t.Fatalf("遍历 turn_usage 失败：%v", closeErr)
	}
	uRows.Close()

	if usageRowCount != 1 {
		t.Fatalf("turn_usage 应恰好 1 行，实际 %d 行；\nOut:\n%s\nErr:\n%s", usageRowCount, out, errOut)
	}
	if outcome != int(turn.OutcomeSpoke) {
		t.Errorf("turn_usage.outcome 应为 %d（OutcomeSpoke），实际 %d", int(turn.OutcomeSpoke), outcome)
	}

	// 断言 3：provider_sessions 一行，cursor_seq 应等于 userMsg.seq。
	//
	// 这一条实测过一次才定下来，任务书原文「cursor_seq 等于 agent 那条消息之前的
	// 最大 seq」按字面（MAX(seq) WHERE seq < agentMsg.seq）算出来是 2——把断言 1
	// 里那条 session_opening 面包屑也算了进去——但真机实测 cursor_seq 是 1，
	// 不是 2。现查 internal/turn/record.go 的 Record.ScannedTo 字段注释与
	// runner.go 的 askUpToWith 之后找到原因：ScannedTo（"本轮扫描到的最大
	// seq，spawn 之前定下"）是在 readAfter 扫描、按 upTo 截断之后就已经
	// 算死的，而 session_opening 面包屑要等到之后的 bootstrap 分支（router
	// 选完之后那句 r.postSystem(ctx, convID, agentID, eventOpening, ...)）
	// 才被追加进 messages 表——也就是说追加面包屑这个动作本身发生在
	// "这一轮已经扫到哪里"被定下**之后**，所以面包屑自己的 seq 不会被计入
	// 这一轮的 cursor。持久化的 cursor_seq 就是这个 ScannedTo（commitSession
	// 里 st.cursor = rec.ScannedTo，snapshotProviderSession 把 st.cursor
	// 填进 CursorSeq），在本用例的时序下恰好等于 userMsg.seq。
	// 这属于"规格字面描述与实现细节不完全一致"的一处发现，报告里会再提一遍。
	cursorWant := userMsg.seq

	type sessRow struct {
		providerSessionID string
		cursorSeq         int64
	}
	var sessions []sessRow
	sRows, err := db.QueryContext(ctx, `SELECT provider_session_id, cursor_seq FROM provider_sessions`)
	if err != nil {
		t.Fatalf("查询 provider_sessions 失败：%v", err)
	}
	for sRows.Next() {
		var s sessRow
		if scanErr := sRows.Scan(&s.providerSessionID, &s.cursorSeq); scanErr != nil {
			sRows.Close()
			t.Fatalf("扫 provider_sessions 一行失败：%v", scanErr)
		}
		sessions = append(sessions, s)
	}
	if closeErr := sRows.Err(); closeErr != nil {
		t.Fatalf("遍历 provider_sessions 失败：%v", closeErr)
	}
	sRows.Close()

	if len(sessions) != 1 {
		t.Fatalf("provider_sessions 应恰好 1 行，实际 %d 行：%+v；\nOut:\n%s\nErr:\n%s", len(sessions), sessions, out, errOut)
	}
	if sessions[0].providerSessionID == "" {
		t.Errorf("provider_sessions.provider_session_id 不应为空")
	}
	if sessions[0].cursorSeq != cursorWant {
		t.Errorf("provider_sessions.cursor_seq 应等于本轮 readAfter 扫到的最大 seq（本用例的时序下"+
			"就是那条 user 消息的 seq，不含 session_opening 面包屑——面包屑是在 ScannedTo 定下之后"+
			"才追加的）（%d），实际 %d", cursorWant, sessions[0].cursorSeq)
	}

	// 断言 4：inflight_turns。这张表属于 Step 10 A，现查（grep 全仓库 .go 文件）
	// 零命中，此刻还不存在——不为它造表，也不让这条用例因此变红，跳过并说明。
	var tableName string
	tErr := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='inflight_turns'`).Scan(&tableName)
	switch {
	case tErr == sql.ErrNoRows:
		t.Log("断言 4 跳过：inflight_turns 表目前不存在（Step 10 A 尚未落地），不视为失败")
	case tErr != nil:
		t.Fatalf("查询 sqlite_master 失败：%v", tErr)
	default:
		var inflightCount int
		if scanErr := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inflight_turns`).Scan(&inflightCount); scanErr != nil {
			t.Fatalf("查询 inflight_turns 失败：%v", scanErr)
		}
		if inflightCount != 0 {
			t.Errorf("一轮成功的 /ask 跑完之后 inflight_turns 应为空，实际 %d 行", inflightCount)
		}
	}
}
