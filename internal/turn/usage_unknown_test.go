package turn

// 本文件是背对背测试的测试方产出，钉住 turn_usage 表七个可空用量列的
// NULL / 0 语义修复契约。修复背景：一轮被信号打死之后，库里那行本该是
// 七列全 NULL（「不知道」），但写入侧此前会把 InputTokens/CacheRead/
// OutputTokens/ThinkingTokens 四项误写成指向 0 的指针（「实测为 0」）——
// NULL 与 0 在这张表上不是同一件事，混淆会让「进程被打死、根本没有 usage
// JSON」与「provider 真的报了 0」在库里长成同一个形状。
//
// 冻结契约里实现方要新增的落地缝（本文件不实现、只依赖）：
//   Record.UsageParsed bool
//   absorbParse 内部置 rec.UsageParsed = true
//   解析失败那支对 absorbParse 的调用加门：
//     !errors.Is(parseErr, provider.ErrUnparseableOutput) 时才调
//   makeTurnUsage 里 !rec.UsageParsed 时四个 token 列与 CacheCreate 一律 nil
//
// 本文件的断言只看 conversation.TurnUsage 里七个可空字段是不是 nil
// （经 recordingStore 拦截 RecordUsage 收到的参数），不直接引用
// Record.UsageParsed 这个实现细节——那是实现方的内部状态，测试只认外部
// 可观测的结果。这样写的副作用是本文件不引用 Record.UsageParsed，修复前
// 就能编译、能真跑：2026-09-21 在修复落地前实测过一次，方向一四条
// （Refused_AgentNotInConfig / Timeout_Killed / SpawnFailed_ProcessNeverStarted /
// UnparsableStdout_NotJSON）各红在 InputTokens/CacheRead/OutputTokens/
// ThinkingTokens 四项（实际值 0），CacheCreate/CostUSD/CostCumulativeUSD
// 三列在修复前已经是 nil、不报错；方向二两条（AgyProviderFailed_ZerosAreReal /
// ErrNoSessionID_RealUsageNotNulled）与既有 ClaudeFailureZerosAreReal 一起绿。
// 修复落地后方向一四条应当转绿，其余不变——最终交付前请重新跑一遍确认。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"council/internal/conversation"
	"council/internal/provider"
)

// mustReplaceOnce 在一份真实探针原件上做一处最小、可追溯的字节改动（例如把
// 会话 id 字段清空），用 strings.Count 强制断言 old 在 s 里恰好出现一次。
// 与 internal/provider/helpers_test.go 的同名函数逻辑一致，但测试文件不参与
// 包的可导出符号，两个测试包（provider_test 与本包）没法共享，只能各留一份。
func mustReplaceOnce(t *testing.T, s, old, new string) string {
	t.Helper()
	if n := strings.Count(s, old); n != 1 {
		t.Fatalf("mustReplaceOnce: %q occurs %d times in fixture, want exactly 1", old, n)
	}
	return strings.Replace(s, old, new, 1)
}

// mustNilInt / mustNilFloat 断言一个可空用量列是 nil——与既有
// record_turn_calls_test.go 里 mustIntPtr / mustZeroPtr 两个局部闭包的方向
// 相反（那两个断言「非 nil」），本文件多数用例要断言的是反方向，两边互不
// 依赖，各写各的。
func mustNilInt(t *testing.T, name string, p *int) {
	t.Helper()
	if p != nil {
		t.Errorf("%s 应当是 nil（这一轮根本没解析出任何 usage，NULL 读作「不知道」，不是「实测为 0」），实际 %d", name, *p)
	}
}

func mustNilFloat(t *testing.T, name string, p *float64) {
	t.Helper()
	if p != nil {
		t.Errorf("%s 应当是 nil，实际 %v", name, *p)
	}
}

// mustSevenNull 断言 turn_usage 的七个可空列全是 nil。老实说：其中只有
// InputTokens/CacheRead/OutputTokens/ThinkingTokens 四项是本次修复真正
// 触碰的——CacheCreate/CostUSD/CostCumulativeUSD 在这几条「从没解析过」的
// 路径上修复之前就已经是 nil（CacheCreate 是 *int 零值，CostUSD 只在成功轮
// 才会被 computeCostDelta 赋值，CostCumulativeUSD 抄的是 rec.Usage.CostUSDTotal，
// 同样只有解析过才会非 nil）。断这三项是回归哨兵，不是本次修复的判据；
// 真正会因为「漏加 `if !rec.UsageParsed` 这道门」而翻红的是前四项。
func mustSevenNull(t *testing.T, u conversation.TurnUsage) {
	t.Helper()
	mustNilInt(t, "InputTokens", u.InputTokens)
	mustNilInt(t, "CacheCreate", u.CacheCreate)
	mustNilInt(t, "CacheRead", u.CacheRead)
	mustNilInt(t, "OutputTokens", u.OutputTokens)
	mustNilInt(t, "ThinkingTokens", u.ThinkingTokens)
	mustNilFloat(t, "CostUSD", u.CostUSD)
	mustNilFloat(t, "CostCumulativeUSD", u.CostCumulativeUSD)
}

// ---------------------------------------------------------------------------
// 方向一：从没解析过 -> 七列必须全 NULL
// ---------------------------------------------------------------------------

// TestUsageUnknown_Refused_AgentNotInConfig 覆盖 OutcomeRefused：给一个不在
// config 里的 agent id，Ask 在 cfg.Agents 查表那一步就拒绝，Spawner 一次都
// 不该被调用（09 G 节「失败要在花钱之前失败」）。这是契约点 (2) 第二句点名
// 的「最便宜的复现路径」：它与 OutcomeUnparsable 里「空 ProviderSession」
// 那一支走的是同一条写入路径（refuse -> persistTurn -> makeTurnUsage），
// 同病——如果实现方把判据错误地键在「是不是走了 refuse」这类分支上而不是
// UsageParsed，这条最省事的路径反而最容易被漏测。
//
// 变异：如果实现方漏掉 makeTurnUsage 里 `if !rec.UsageParsed` 这道门（四个
// token 列继续用旧的 reportedInt 直通逻辑），本用例会红：InputTokens 等
// 四项会变成指向 0 的指针，而不是期望的 nil。
func TestUsageUnknown_Refused_AgentNotInConfig(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	postUser(t, svc, convID, "hi")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, "not-a-configured-agent")
	if err == nil {
		t.Fatal("Ask 应当报错（agent 不在 config 里）")
	}
	if !errors.Is(err, ErrAgentUnknown) {
		t.Fatalf("err = %v，期望包着 ErrAgentUnknown", err)
	}
	// 先做粗粒度的空转防护，再看指针：Outcome/调用次数任何一项不对，下面的
	// nil 断言就是在一个错误的前提上通过，看起来像测到了、其实什么都没测。
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if len(fs.calls) != 0 {
		t.Fatalf("Spawner 调用次数 = %d，期望 0（起进程之前就该拒绝，钱不该花）", len(fs.calls))
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]
	if u.Outcome != int(OutcomeRefused) {
		t.Fatalf("TurnUsage.Outcome = %d，期望 %d（int(OutcomeRefused)）", u.Outcome, int(OutcomeRefused))
	}
	mustSevenNull(t, u)
}

// TestUsageUnknown_Timeout_Killed 覆盖 OutcomeTimeout：一轮被信号打死
// （killed=true），进程根本没能把 JSON 打印出来，ParseTurn 从未被调用。
// 这正是任务书背景段落原话描述的那个原始故障复现场景。
//
// 变异：漏掉 `if !rec.UsageParsed` 门 -> 四个 token 列变成指向 0 的指针。
func TestUsageUnknown_Timeout_Killed(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{killed: true, err: context.DeadlineExceeded})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当报错（超时被杀）")
	}
	if rec.Outcome != OutcomeTimeout {
		t.Fatalf("Outcome = %v，期望 OutcomeTimeout", rec.Outcome)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("Spawner 调用次数 = %d，期望 1（进程确实起过，只是被杀，不是起进程之前拒绝）", len(fs.calls))
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]
	if u.Outcome != int(OutcomeTimeout) {
		t.Fatalf("TurnUsage.Outcome = %d，期望 %d（int(OutcomeTimeout)）", u.Outcome, int(OutcomeTimeout))
	}
	mustSevenNull(t, u)
}

// TestUsageUnknown_SpawnFailed_ProcessNeverStarted 覆盖 OutcomeSpawnFailed：
// 连进程都没起来（exec.Command 直接报错），更不可能有 usage JSON。
//
// 变异：漏掉 `if !rec.UsageParsed` 门 -> 四个 token 列变成指向 0 的指针。
func TestUsageUnknown_SpawnFailed_ProcessNeverStarted(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{err: errors.New("exec: \"claude-fake\": executable file not found in %PATH%")})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当报错（进程没起来）")
	}
	if rec.Outcome != OutcomeSpawnFailed {
		t.Fatalf("Outcome = %v，期望 OutcomeSpawnFailed", rec.Outcome)
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]
	if u.Outcome != int(OutcomeSpawnFailed) {
		t.Fatalf("TurnUsage.Outcome = %d，期望 %d（int(OutcomeSpawnFailed)）", u.Outcome, int(OutcomeSpawnFailed))
	}
	mustSevenNull(t, u)
}

// TestUsageUnknown_UnparsableStdout_NotJSON 补第四条路径，不在任务书「至少
// 三条」的清单里，是本文件另外加的：前三条用例走的都是 absorbParse 从未被
// 调用过的路径（Ask 在 p.ParseTurn(stdout) 之前就返回了），测不出「解析
// 失败那一支对 absorbParse 的调用加一道门」这道门本身漏没漏加——如果实现方
// 对所有 parseErr!=nil 都无条件调用 absorbParse（漏掉
// `!errors.Is(parseErr, provider.ErrUnparseableOutput)` 这道门），前三条
// 一个都测不出来：只有真的走到 p.ParseTurn(stdout) 并且认不出形状，才能
// 证明这道门真的挡住了 absorbParse。
//
// 用 claudeNotFoundResumeStderr：09 A 节记录的真实观测字节（claude --resume
// 一个不存在的会话 id 时，stdout 是一段普通文本，不是合法 JSON），不是编的
// 假字符串，与既有 TestRecordTurnNeverWritesCursorsTable 第三轮用空字节
// 触发同一类失败的手法呼应，但这里换成真实字节、且专门断言七列 NULL。
//
// 变异：如果实现方漏掉这道门，ParseTurn 在这一支按契约点 4 的不变量
// 恒返回零值 TurnResult{}（本包 internal/provider/parse_zero_result_test.go
// 钉住这条不变量），absorbParse 会把这个零值 Usage 抄进 rec.Usage、
// UsageParsed 被误置 true，四个 token 列从 nil 变成指向 0 的指针，本用例
// 会红。
func TestUsageUnknown_UnparsableStdout_NotJSON(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{stdout: []byte(claudeNotFoundResumeStderr), exitCode: 1})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当报错（stdout 不是 JSON）")
	}
	if !errors.Is(err, provider.ErrUnparseableOutput) {
		t.Fatalf("err = %v，期望包着 provider.ErrUnparseableOutput", err)
	}
	if rec.Outcome != OutcomeUnparsable {
		t.Fatalf("Outcome = %v，期望 OutcomeUnparsable", rec.Outcome)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("Spawner 调用次数 = %d，期望 1（进程确实起过，只是吐出的不是 JSON）", len(fs.calls))
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]
	if u.Outcome != int(OutcomeUnparsable) {
		t.Fatalf("TurnUsage.Outcome = %d，期望 %d（int(OutcomeUnparsable)）", u.Outcome, int(OutcomeUnparsable))
	}
	mustSevenNull(t, u)
}

// ---------------------------------------------------------------------------
// 方向二：真报了 0 的那一轮 -> 四列必须是 0，不是 NULL
// ---------------------------------------------------------------------------
//
// 既有 record_turn_calls_test.go 的
// TestRecordTurnUsageNullableFields_ClaudeFailureZerosAreReal 已经用
// claudeBadModelOut（走 ErrTurnFailed）覆盖了这一侧，且必须继续原样通过——
// 本文件不重复、不修改它，只在这里补 agy 侧的同类，证明「四列真实为 0」
// 不是 claudeUsageToUsage 一家的巧合。

// TestUsageUnknown_AgyProviderFailed_ZerosAreReal 用 agyBadModelOut：
// status="ERROR"，走 ErrTurnFailed（不是 ErrUnparseableOutput），usage
// 子对象真实存在、四个数值字段（input/cache_read/output/thinking）全是
// 真实的 0。absorbParse 的门在这一支必须放行
// （!errors.Is(parseErr, provider.ErrUnparseableOutput) 为真），这四列
// 必须是指向 0 的指针，不是 nil。
//
// CacheCreate 是唯一的例外，与 claude 那条既有用例不同：agy 的 usage 里
// 根本没有 cache_creation 这一项，agyUsageToUsage 无条件把 "cache_create"
// 记进 Unreported（agy.go 第 294 行，与「有没有解析过」无关），这一列本来
// 就该恒为 nil。照抄 claude 那条既有用例的 mustZeroPtr("CacheCreate", …)
// 会在这里把这个 provider 差异误判成 bug。
//
// 变异：如果实现方漏掉 `rec.UsageParsed = true` 这一行（absorbParse 被
// 调用了，但忘了置位），makeTurnUsage 仍然会走 `!rec.UsageParsed` 那一支，
// 把这四个本该是「真实 0」的列错误地清成 nil——本用例会红。这是本文件里
// 唯一一条能抓住这个具体遗漏的用例：前面方向一的四条用例本来就期望 nil，
// 这个遗漏对它们而言是「意外地」仍然正确，测不出来。
func TestUsageUnknown_AgyProviderFailed_ZerosAreReal(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "用一个不存在的 model 名触发失败")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{stdout: []byte(agyBadModelOut), exitCode: 1})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentAgy)
	if err == nil {
		t.Fatal("Ask 应当报错")
	}
	// 先钉住 Outcome：不是 OutcomeProviderFailed 的话，说明这一轮根本没有
	// 走到「provider 自报失败」这条分支（例如误配置导致 nova 被 refuse
	// 掉），下面对「真实 0」的断言就是在错误的前提上通过。
	if rec.Outcome != OutcomeProviderFailed {
		t.Fatalf("Outcome = %v，期望 OutcomeProviderFailed", rec.Outcome)
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]

	mustZeroInt := func(name string, p *int) {
		t.Helper()
		if p == nil {
			t.Errorf("%s 应当是非 nil 的指针（agy-badmodel 的 usage 对象真实存在，只是数值为 0），实际 nil", name)
			return
		}
		if *p != 0 {
			t.Errorf("%s = %d，期望 0", name, *p)
		}
	}
	mustZeroInt("InputTokens", u.InputTokens)
	mustZeroInt("CacheRead", u.CacheRead)
	mustZeroInt("OutputTokens", u.OutputTokens)
	mustZeroInt("ThinkingTokens", u.ThinkingTokens)
	if u.CacheCreate != nil {
		t.Errorf("CacheCreate 应当是 nil（agy 的 usage 里根本没有 cache_creation 这一项，与「有没有解析过」无关），实际 %d", *u.CacheCreate)
	}
}

// ---------------------------------------------------------------------------
// 契约点 (2)：判据不是 Outcome 枚举
// ---------------------------------------------------------------------------

// TestUsageUnknown_ErrNoSessionID_RealUsageNotNulled 钉住契约点 (2) 第一句：
// 「OutcomeUnparsable 里混着「解析成功但 ProviderSession 是空串」那一支，
// 它的 Usage 是真花过钱的，不该被抹成 NULL」。
//
// 用 claudeT1JSON 挖空 session_id 这一个字段（mustReplaceOnce，同一份真实
// 探针原件只改一处，可追溯，做法与 internal/provider/helpers_test.go 一致）。
// claude.ParseTurn 在 claudeSuccess(raw)==true 但 raw.SessionID=="" 时返回
// 已经填好 Usage 的 base，外加 ErrNoSessionID——不是 ErrUnparseableOutput，
// 也不是 ErrTurnFailed。runner.go 里这条错误落进 `parseErr != nil` 分支，
// 归类成 OutcomeUnparsable。
//
// 这是本文件里唯一一条能在「实现方把判据错误地键在 Outcome 枚举上（例如
// `if rec.Outcome == OutcomeUnparsable { 清空 }`），而不是键在 UsageParsed /
// errors.Is(parseErr, ErrUnparseableOutput) 上」这个具体错误上翻车的用例：
// 前面方向一的四条用例全部走的是 OutcomeRefused/Timeout/SpawnFailed/
// Unparsable-但-ErrUnparseableOutput，没有一条能证明「按 Outcome 枚举分支」
// 与「按 UsageParsed 分支」在 OutcomeUnparsable 这一个 Outcome 值内部
// 还分裂成两种截然不同的正确答案。
//
// 变异：若实现方按 Outcome 枚举判定，这条真实花过钱的用量
// （InputTokens=47751、CacheCreate=47749、OutputTokens=4）会被错误地清成
// nil，本用例会红。
func TestUsageUnknown_ErrNoSessionID_RealUsageNotNulled(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题")

	blanked := mustReplaceOnce(t, claudeT1JSON,
		`"session_id":"2a90d449-a348-497a-8812-e73e74d6898e"`,
		`"session_id":""`)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{stdout: []byte(blanked), exitCode: 0})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当报错（session_id 为空）")
	}
	if !errors.Is(err, provider.ErrNoSessionID) {
		t.Fatalf("err = %v，期望包着 provider.ErrNoSessionID", err)
	}
	if rec.Outcome != OutcomeUnparsable {
		t.Fatalf("Outcome = %v，期望 OutcomeUnparsable", rec.Outcome)
	}
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("RecordUsage 调用次数 = %d，期望 1", len(rs.recordUsageCalls))
	}
	u := rs.recordUsageCalls[0]

	mustIntPtrEq := func(name string, p *int, want int) {
		t.Helper()
		if p == nil {
			t.Errorf("%s 应当是非 nil 的指针（这一轮真花过钱：claudeT1JSON 的 usage 是真实数据，本测试只挖空了 session_id 这一个字段），实际 nil", name)
			return
		}
		if *p != want {
			t.Errorf("%s = %d，期望 %d", name, *p, want)
		}
	}
	mustIntPtrEq("InputTokens", u.InputTokens, 2+47749+0)
	mustIntPtrEq("CacheCreate", u.CacheCreate, 47749)
	mustIntPtrEq("CacheRead", u.CacheRead, 0)
	mustIntPtrEq("OutputTokens", u.OutputTokens, 4)
	mustIntPtrEq("ThinkingTokens", u.ThinkingTokens, 0)
}
