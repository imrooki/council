package turn

// 本文件覆盖 09_STEP6_SPEC.md G 节那张结局表，每一行至少一条用例，
// 逐行断言四件事：落不落正文、推不推游标、推不推会话状态、发哪一条面包屑
// （G 节表头）。行号注释里的数字对应 G 节表格从上到下的第几行。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
)

// TestOutcomeSpoke 覆盖 G 节第 1 行：五信号全对 + 有会话 id -> OutcomeSpoke，
// 正文落库、Ingested() 为真、Record.Message 非 nil。
func TestOutcomeSpoke(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "请审阅这段推导")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if rec.Outcome != OutcomeSpoke {
		t.Fatalf("Outcome = %v，期望 OutcomeSpoke", rec.Outcome)
	}
	if !rec.Outcome.Ingested() {
		t.Error("OutcomeSpoke 的 Ingested() 应当为真")
	}
	if rec.Message == nil {
		t.Fatal("Record.Message 为 nil，期望非 nil")
	}
	if rec.Message.Text != "OK" {
		t.Errorf("落库正文 = %q，期望 %q（claude-t1 的 result 字段）", rec.Message.Text, "OK")
	}
	if rec.ProviderSession == "" {
		t.Error("ProviderSession 为空，claude-t1 的 session_id 应当被填进去")
	}
}

// TestOutcomeProviderFailed 覆盖 G 节第 5 行，用 A 节探针 1 的真实字节
// （claude-badmodel.out）：provider 自报失败 -> OutcomeProviderFailed，
// Text 绝不落库（Record.Message 为 nil），但 Usage 与 ProviderSession
// 仍有值（钱花了，账要记），Ingested() 为假。
//
// 这条钉住 09 A 节点名的反例：claude 那份失败样本的 subtype 仍然是
// "success"——只看 subtype 会把这一轮的错误正文当成回答收进只追加的事实源。
// 用真实探针字节而不是手写一个 is_error:true 的 JSON，是因为 K 节前置条件
// 明写「本组用例必须用这三份真实字节，不得自造失败 JSON」。
func TestOutcomeProviderFailed(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "用一个不存在的 model 名触发失败")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{stdout: []byte(claudeBadModelOut), exitCode: 1})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当返回错误")
	}
	if rec.Outcome != OutcomeProviderFailed {
		t.Fatalf("Outcome = %v，期望 OutcomeProviderFailed（这份样本 subtype=\"success\" 但 is_error=true/terminal_reason=\"api_error\"/api_error_status=404/stop_reason=\"stop_sequence\"，只看 subtype 会误判成功）", rec.Outcome)
	}
	if rec.Outcome.Ingested() {
		t.Error("OutcomeProviderFailed 的 Ingested() 应当为假")
	}
	if rec.Message != nil {
		t.Fatalf("Record.Message 应当为 nil（钱花了但正文绝不落库），实际: %+v", rec.Message)
	}
	if rec.BaselineSuspect {
		t.Error("首轮失败时 fresh 仍是 true，不该置 BaselineSuspect（那是给「已有会话记录」的后续轮用的）")
	}
	// 用量与会话 id 仍尽量填上：claude-badmodel.out 里 session_id 是
	// 9568d2d7-...，usage 五项都是真实的 0（那一轮请求发出前就被拒了）。
	if rec.ProviderSession == "" {
		t.Error("ProviderSession 应当仍被填上（钱花了，账要记），即使这一轮失败")
	}

	// 确认库里真的没有落一条 alice 说的话：History 里只应看到 user 那条问题。
	history, err := svc.History(context.Background(), convID, 0, 100)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	for _, m := range history {
		if m.SenderID == testAgentClaude {
			t.Errorf("provider 自报失败却在事实源里留下了一条 %s 的发言: %+v", testAgentClaude, m)
		}
	}
}

// TestOutcomeUnparsable 覆盖 G 节第 6 行：JSON 认不出来 -> OutcomeUnparsable。
// 用 09 A 节「探针推翻了什么」记录的真实观测（claude --resume 一个不存在的
// id 时 stdout 是 0 字节，错误只在 stderr 纯文本里）——emptyStdout 这条闸
// 本身就是被这份观测逼出来的，这里直接喂空字节，不需要额外编造。
func TestOutcomeUnparsable(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "触发一次 stdout 为空")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{stdout: []byte{}, stderr: []byte(claudeNotFoundResumeStderr), exitCode: 1})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当返回错误")
	}
	if rec.Outcome != OutcomeUnparsable {
		t.Fatalf("Outcome = %v，期望 OutcomeUnparsable", rec.Outcome)
	}
	if rec.Outcome.Ingested() {
		t.Error("OutcomeUnparsable 的 Ingested() 应当为假")
	}
	if rec.Message != nil {
		t.Error("Record.Message 应当为 nil")
	}
}

// TestOutcomeTimeoutKilled 覆盖 G 节第 7 行的一支：假 Spawner 返回
// killed=true -> OutcomeTimeout 且 MaybeAdvanced=true，Ingested() 仍为假。
// 09 G 节第 7 行的理由：Windows 上杀掉的往往只是 .cmd 垫片，真正干活的
// node.exe 可能活着把这一轮写进 CLI 自己的会话文件，所以标 MaybeAdvanced
// 而不是当作「什么都没发生」。
func TestOutcomeTimeoutKilled(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "触发超时")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{stdout: []byte{}, killed: true, err: context.DeadlineExceeded})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当返回错误")
	}
	if rec.Outcome != OutcomeTimeout {
		t.Fatalf("Outcome = %v，期望 OutcomeTimeout", rec.Outcome)
	}
	if !rec.MaybeAdvanced {
		t.Error("killed=true 时 MaybeAdvanced 应当为真")
	}
	if rec.Outcome.Ingested() {
		t.Error("OutcomeTimeout 的 Ingested() 应当为假（丢 delta 比重复更糟，但重复是有记录的）")
	}
}

// TestOutcomeTimeoutDeadlineExceededWithoutKilled 覆盖同一行的另一支：
// killed 字段本身是 false，但 spawnErr 是 context.DeadlineExceeded——
// runner.go 的判据是 `killed || errors.Is(spawnErr, context.DeadlineExceeded)
// || errors.Is(spawnErr, context.Canceled)`，两个信号任一为真都要走这条分支，
// 不能只认 killed 这一个字段。
func TestOutcomeTimeoutDeadlineExceededWithoutKilled(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "触发超时（killed 字段为假）")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{stdout: []byte{}, killed: false, err: context.DeadlineExceeded})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当返回错误")
	}
	if rec.Outcome != OutcomeTimeout {
		t.Fatalf("Outcome = %v，期望 OutcomeTimeout（即使 killed=false，DeadlineExceeded 本身就该判超时）", rec.Outcome)
	}
	if !rec.MaybeAdvanced {
		t.Error("MaybeAdvanced 应当为真：即使没有被杀，超时本身就意味着 provider 侧可能已经把这一轮写完了")
	}
}

// TestOutcomeSpawnFailed 覆盖 G 节第 8 行：进程压根没起来（不在 PATH、
// CPA 睡着、立刻非零退出）-> OutcomeSpawnFailed，MaybeAdvanced 为假
// （与超时不同：这一支根本没有一个可能活着的子进程）。
func TestOutcomeSpawnFailed(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "触发 spawn 失败")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{err: errors.New("exec: \"claude-fake\": executable file not found in %PATH%")})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当返回错误")
	}
	if rec.Outcome != OutcomeSpawnFailed {
		t.Fatalf("Outcome = %v，期望 OutcomeSpawnFailed", rec.Outcome)
	}
	if rec.MaybeAdvanced {
		t.Error("OutcomeSpawnFailed 时 MaybeAdvanced 应当为假：进程从没起来，不存在「可能活着的子进程」")
	}
	if rec.Outcome.Ingested() {
		t.Error("OutcomeSpawnFailed 的 Ingested() 应当为假")
	}
}

// TestOutcomeCommandLineTooLongIsRefusedNotSpawnFailed 覆盖未决 #19：
// spawnProcess 里的命令行总长闸（ErrCommandLineTooLong）命中时，Outcome
// 必须是 OutcomeRefused、WallMS 必须是 0，不能落进 OutcomeSpawnFailed——
// 那道闸命中时一个进程都没起（它比 exec.Command 更早），"起了进程但失败"
// 与"起进程之前拒绝"是两件事，spawn_failed 的计数不该被这类拒绝污染。
//
// 用真实的 spawnProcess（不换成 fakeSpawner），用一段真正超大的 Role 顶大
// grok 的整条命令行——grok 把 Role 整段放进 --system-prompt-override 的值
// （provider/grok.go），BuildTurn 本身不检查 Role 的长度（它只用
// checkArgvPrompt 查 Prompt），所以这道闸只会在 spawnProcess 里、真正组出
// 命令行之后才触发，是行为测试，不是靠假实现直接返回哨兵。
func TestOutcomeCommandLineTooLongIsRefusedNotSpawnFailed(t *testing.T) {
	cfg := newTestConfig()
	ac := cfg.Agents[testAgentGrok]
	ac.Role = strings.Repeat("R", maxCommandLineBytes+500)
	cfg.Agents[testAgentGrok] = ac

	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)
	postUser(t, svc, convID, "触发命令行总长闸")

	r := newTestRunner(t, cfg, svc)
	// 不换 r.spawn：默认就是生产用的 spawnProcess（NewRunner 里赋的值），
	// 这道闸恰恰长在那里面，换成假实现就测不到它。

	rec, err := r.Ask(context.Background(), convID, testAgentGrok)
	if !errors.Is(err, ErrCommandLineTooLong) {
		t.Fatalf("errors.Is(err, ErrCommandLineTooLong) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused（未决 #19：这道闸命中时一个进程都没起）", rec.Outcome)
	}
	if rec.WallMS != 0 {
		t.Errorf("WallMS = %d，期望 0：这道闸命中时没有等过任何真实子进程", rec.WallMS)
	}
	if rec.Outcome.Ingested() {
		t.Error("OutcomeRefused 的 Ingested() 应当为假")
	}
}

// TestOutcomeShimNewlineArgIsRefusedNotSpawnFailed 覆盖未决 #21：与 #19 一模
// 一样的分类问题，只是闸不同——spawnProcess 里 .cmd/.bat 垫片换行闸
// （ErrShimNewlineArg）命中时同样一个进程都没起，却在 #19 之前一直落进
// OutcomeSpawnFailed。改法与 #19 相同：归 OutcomeRefused，WallMS 清零。
//
// 用真实的 spawnProcess（不换成 fakeSpawner），把 grok 的 provider 可执行
// 文件配置指向一个不存在的 *.cmd 路径：exec.LookPath 找不到这个文件，
// path 原样保留（spawn.go 的行为：LookPath 失败就用原始 c.Path），
// filepath.Ext 判成 ".cmd"。renderIdentityCore 拼出来的身份文本本身就是
// 多行（"You are X (id=y).\nProvider: z..."），grok 把它整段放进
// --system-prompt-override 的值（provider/grok.go），argv 里天然带着换行——
// 不需要额外构造超长或带换行的 Role，默认测试配置就够触发这道闸。
func TestOutcomeShimNewlineArgIsRefusedNotSpawnFailed(t *testing.T) {
	cfg := newTestConfig()
	pc := cfg.Providers["grok"]
	pc.Command = "grok-fake-shim.cmd" // 不存在的路径，LookPath 会失败，扩展名仍是 .cmd
	cfg.Providers["grok"] = pc

	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)
	postUser(t, svc, convID, "触发 .cmd 垫片换行闸")

	r := newTestRunner(t, cfg, svc)
	// 不换 r.spawn：这道闸长在生产用的 spawnProcess 里，换成假实现测不到它。

	rec, err := r.Ask(context.Background(), convID, testAgentGrok)
	if !errors.Is(err, ErrShimNewlineArg) {
		t.Fatalf("errors.Is(err, ErrShimNewlineArg) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused（未决 #21：与 #19 同一个分类问题）", rec.Outcome)
	}
	if rec.WallMS != 0 {
		t.Errorf("WallMS = %d，期望 0：这道闸命中时没有等过任何真实子进程", rec.WallMS)
	}
	if rec.Outcome.Ingested() {
		t.Error("OutcomeRefused 的 Ingested() 应当为假")
	}
}

// TestOutcomeRefusedSpawnerNeverCalled 覆盖 G 节第 9 行里"起进程之前就拒绝"
// 的四种具体成因，每种都断言 Spawner 一次都没被调用过——09 G 节原话「失败要
// 响亮，而且要在花钱之前失败」，验证它的唯一办法就是证明没花钱：fakeSpawner
// 的调用计数器必须停在 0。
func TestOutcomeRefusedSpawnerNeverCalled(t *testing.T) {
	t.Run("agent 不在配置里", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		postUser(t, svc, convID, "hi")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, "ghost-agent")
		if rec.Outcome != OutcomeRefused {
			t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
		}
		if !errors.Is(err, ErrAgentUnknown) {
			t.Fatalf("errors.Is(err, ErrAgentUnknown) 应当为真，实际: %v", err)
		}
		if len(fs.calls) != 0 {
			t.Fatalf("Spawner 被调用了 %d 次，期望 0 次", len(fs.calls))
		}
	})

	t.Run("full 权限档不被编排器提供", func(t *testing.T) {
		// 用 full 而不是 workspace：11c 第二片放行了 claude 的 workspace 档，
		// 只有 full（以及拼错的档）仍然会在 TranslatePermission 那一层
		// 被拒——这条用例要测的是"起进程之前拒绝"，不是"这一档配对了
		// 哪个具体权限值"。
		cfg := newTestConfig()
		ac := cfg.Agents[testAgentClaude]
		ac.Permissions = "full"
		cfg.Agents[testAgentClaude] = ac
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "hi")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if rec.Outcome != OutcomeRefused {
			t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
		}
		if !errors.Is(err, ErrTierNotOffered) {
			t.Fatalf("errors.Is(err, ErrTierNotOffered) 应当为真，实际: %v", err)
		}
		if len(fs.calls) != 0 {
			t.Fatalf("Spawner 被调用了 %d 次，期望 0 次", len(fs.calls))
		}
	})

	t.Run("WorkDir 为空", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "hi")

		// 直接用空 workDir 构造 Runner（不经 newTestRunner，那个辅助函数总是给
		// 一个非空目录），复现 09 B 节「workDir 为空时 Ask 内部计算出的
		// agentWorkDir 也会是相对路径」这条场景。
		r := NewRunner(cfg, svc, "", RulesBlocks{})
		fs := &fakeSpawner{}
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if rec.Outcome != OutcomeRefused {
			t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
		}
		if !errors.Is(err, provider.ErrNoWorkDir) {
			t.Fatalf("errors.Is(err, provider.ErrNoWorkDir) 应当为真，实际: %v", err)
		}
		if len(fs.calls) != 0 {
			t.Fatalf("Spawner 被调用了 %d 次，期望 0 次", len(fs.calls))
		}
	})

	t.Run("身份指纹漂移", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentClaude)
		postUser(t, svc, convID, "第一轮")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueueSuccess(claudeT1JSON)
		r.spawn = fs.spawn

		if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
			t.Fatalf("建立会话的首轮失败: %v", err)
		}
		if len(fs.calls) != 1 {
			t.Fatalf("首轮之后 Spawner 调用次数 = %d，期望 1", len(fs.calls))
		}

		// 会话开着的时候改 model：身份指纹必须变化，第二轮必须被拒绝。
		ac := cfg.Agents[testAgentClaude]
		ac.Model = "a-different-model"
		cfg.Agents[testAgentClaude] = ac
		postUser(t, svc, convID, "第二轮，但脑子已经不一样了")

		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if rec.Outcome != OutcomeRefused {
			t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
		}
		if !errors.Is(err, ErrIdentityDrift) {
			t.Fatalf("errors.Is(err, ErrIdentityDrift) 应当为真，实际: %v", err)
		}
		if len(fs.calls) != 1 {
			t.Fatalf("身份漂移被拒绝的这一轮不该调用 Spawner，调用次数从 1 变成了 %d", len(fs.calls))
		}
	})

	t.Run("探针组不出命令行（identityFPFor 失败）", func(t *testing.T) {
		// agy 的 Identity 策略是 IdentityPromptPreamble：BuildTurn 把 Role 拼进
		// 探针正文最前面（agy.go 里 ProviderSession == "" 那一支），一段真正
		// 超大的 Role 会让探针自己的 checkArgvPrompt 撞上 ErrPromptTooLong——
		// 这是 identityFPFor 在起进程之前就失败的一条真实、自然的触发路径，
		// 不是拿假 Provider 硬造错误。
		cfg := newTestConfig()
		ac := cfg.Agents[testAgentAgy]
		ac.Role = strings.Repeat("R", 40000) // 单独已超 32767，探针拼上正文之后更超
		cfg.Agents[testAgentAgy] = ac

		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentAgy)
		postUser(t, svc, convID, "hi")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentAgy)
		if rec.Outcome != OutcomeRefused {
			t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
		}
		if !errors.Is(err, provider.ErrPromptTooLong) {
			t.Fatalf("errors.Is(err, provider.ErrPromptTooLong) 应当为真（探针组命令行时撞上的就是这道闸），实际: %v", err)
		}
		if len(fs.calls) != 0 {
			t.Fatalf("Spawner 被调用了 %d 次，期望 0 次：identityFPFor 失败必须在起进程之前拒绝，不许吞", len(fs.calls))
		}
	})
}

// TestOutcomePostRejected 覆盖 G 节第 4 行：provider 跑完了、钱花了、会话
// 已经吃下这批 delta，但 Post 被拒——典型做法（任务书原话）：这一轮之前把该
// agent /leave 掉，事务内的 RequireActiveParticipant 必然拒掉这条 Post。
//
// 断言四件事：DroppedText 等于 provider 那一轮的正文；Message 为 nil；
// Ingested() 为真（钱已经花了、provider 已经吃下）；会话状态仍然推进
// （commitSession 仍被调用，下一轮不再是 Fresh）。
func TestOutcomePostRejected(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "在 alice 说话之前，把她踢出这场会话")

	// 让 alice 处于「在配置里、但不是这场会话的活跃成员」状态：
	// Ask 的前置检查全部只看 cfg.Agents，不看 participant 表，
	// 所以能一路走到 spawn 成功，只在最后 svc.Post 那一步撞上
	// RequireActiveParticipant。
	if err := svc.Leave(context.Background(), convID, testAgentClaude, time.Now().UTC()); err != nil {
		t.Fatalf("Leave 失败: %v", err)
	}

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("Ask 应当返回错误（Post 被拒）")
	}
	if rec.Outcome != OutcomePostRejected {
		t.Fatalf("Outcome = %v，期望 OutcomePostRejected", rec.Outcome)
	}
	if !rec.Outcome.Ingested() {
		t.Error("OutcomePostRejected 的 Ingested() 应当为真：provider 已经吃下这批 delta，钱已经花了")
	}
	if rec.Message != nil {
		t.Fatalf("Record.Message 应当为 nil，实际: %+v", rec.Message)
	}
	if rec.DroppedText != "OK" {
		t.Errorf("DroppedText = %q，期望等于 provider 那一轮的正文 %q", rec.DroppedText, "OK")
	}
	if !errors.Is(err, conversation.ErrParticipantNotFound) {
		t.Errorf("errors.Is(err, conversation.ErrParticipantNotFound) 应当为真，实际: %v", err)
	}
	// 会话状态照常推进：下一轮不再是 Fresh。用 SessionInfo 验证（它是纯读
	// 方法，09a 明写「不触发任何 provider 调用」，用它不会多花一次 spawn）。
	info, ok := r.SessionInfo(context.Background(), convID, testAgentClaude)
	if !ok {
		t.Fatal("SessionInfo 应当能查到这条会话（会话状态照常推进），实际查不到")
	}
	if info.ProviderSession == "" {
		t.Error("SessionInfo.ProviderSession 为空，会话状态应当已经推进")
	}
}
