package repl

// configcmd_test.go 覆盖 docs/11d_STEP8C_CONTRACT.md 第 4.6 节（8c-3：
// /config 与 /config reload [--apply]）的六条验收。
//
// 背对背纪律：本文件写作期间没有读过、也没有主动去找 configcmd.go /
// Runner.SetConfig 的任何实现代码——它们此刻应当还不存在。写作期间在
// 本会话共享的 scratchpad 根目录下看到了两个文件名
// grok_task_configcmd.txt（22:25）与 grok_resp_configcmd.txt（22:37），
// 与本次任务的落点撞名；这两个文件没有被打开读取，只看到了文件名与
// 时间戳，已经如实写进交卷报告，不在此重复。
//
// 判据一律是 turn.Runner.Ask 的 (Record, error)（Outcome / Fresh /
// errors.Is）与 conversation 库里 provider_sessions 那一行在不在、
// IdentityFP 变没变，不是屏幕上打印的具体措辞（09a「不属于契约面的
// 东西」一节 + 本任务书的硬性要求）。/config 与 /config reload
// [--apply] 只能经 REPL 命令触发，这部分必须驱动真正的 REPL.Run；
// 但凡能绕开屏幕文字、直接问 turn.Runner 或 fakeStore 的地方，一律绕开。
//
// 全部六个用例都需要 Runner.Ask 真正跑完一整轮（起进程、解析、落库），
// 而不是在起进程之前就被拒绝：
//   - internal/turn 的 Spawner 字段是包内私有的，本包（internal/repl）
//     没有注入假 Spawner 的办法（那是 internal/turn 测试专用的
//     fakeSpawner，定义在 package turn 内部）。
//   - 更关键的是：即使能从外部塞一条 provider_sessions 种子行，也测不出
//     第 2 条要测的"同一个 /ask 被 ErrIdentityDrift 拒绝"——Runner.Ask
//     只有在 (convID, agentID) 这把 key 已经在 r.sess（进程内内存）里时
//     才会走"指纹不符 -> 拒绝"这条分支；只在 fakeStore 里种一行、不经过
//     一次真正的 Ask，r.sess 里没有这一条，Ask 会落进"读回、指纹不符 ->
//     悄悄作废重建"的另一条分支（runner.go 里 postRebuildNotice 那一支），
//     那是不报错、静默重建，与契约要的"拒绝"是两种不同的结局。
//   - 于是本文件借用了本项目自己在 internal/turn 测试里同样用过的技巧
//     （runner_outcomes_test.go 的 TestOutcomeShimNewlineArgIsRefusedNotSpawnFailed
//     等用例：不换 Spawner，直接把 provider 的可执行文件指向一个假的
//     本地脚本），只是这里的假脚本会正常退出并吐出一段合法的 JSON，
//     让一整轮真的"跑完"，而不是被某道闸拦下。假脚本是一个内容固定的
//     .cmd 垫片：不管收到什么参数，一律在 stdout 打印同一段满足
//     claudeSuccess 判据的 JSON。真机上（COMPUTERNAME=LONG）已经用一段
//     独立的实验程序验证过：Go 的 exec.Command 传一个绝对路径的 .cmd
//     文件可以被直接起来、stdout 能被完整捕获、JSON 能被
//     encoding/json.Unmarshal 正常解析，不需要经过 cmd.exe /c 的额外包装。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// configcmdAskTimeout 是 askDirect 每次调用给的墙钟上限。假脚本几乎立即
// 退出，这个值只是一个远大于实际耗时的安全网，不是本文件里真正的等待
// 时间。
const configcmdAskTimeout = 15 * time.Second

// configcmdSuccessSessionID 是假脚本回显的 provider 会话 id，固定字面量：
// 两处假脚本（原始 command 与"换了路径"之后的第二份）都回显同一个值，
// 是为了让"换 command 之后同一条会话继续暖续用"这件事有意义——
// 会话 id 必须跨两份垫片保持一致，否则 CLI 侧看起来就像真的换了新会话。
const configcmdSuccessSessionID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"

// fakeClaudeStdout 是假 claude 垫片打印到 stdout 的那段 JSON 的 Go 结构体
// 形式，字段与 internal/provider/claude.go 的 claudeOutput /
// claudeSuccess 判据逐条对应：
//
//	Type == "result" && !IsError && Subtype == "success" &&
//	TerminalReason == "completed" && (StopReason == "end_turn" || "max_tokens")
//	&& SessionID != ""
//
// 用结构体 + json.Marshal 而不是手写 JSON 字面量，是为了不必自己操心
// 转义；生成的紧凑 JSON 里只有字母、数字、冒号、逗号、大括号、双引号，
// 全部对 cmd.exe 的批处理解释器安全（不含 % ^ & | < > 这些会被特殊解释
// 的字符）。
type fakeClaudeStdout struct {
	Type           string `json:"type"`
	Subtype        string `json:"subtype"`
	IsError        bool   `json:"is_error"`
	TerminalReason string `json:"terminal_reason"`
	StopReason     string `json:"stop_reason"`
	SessionID      string `json:"session_id"`
	Result         string `json:"result"`
	Usage          struct {
		InputTokens              int `json:"input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		OutputTokens             int `json:"output_tokens"`
	} `json:"usage"`
}

// writeFakeClaudeExe 在 path 处写一个真实可执行的 .cmd 垫片：不管收到
// 什么参数，一律在 stdout 打印同一段满足 claudeSuccess 判据的 JSON，
// session_id 固定为 sessionID。
//
// 这不是在猜 configcmd.go 的实现，是在借 spawnProcess（生产代码，
// internal/turn/spawn.go）本来就会去调用的那个可执行文件路径——
// cfg.Providers["claude"].Command 指向哪里，spawnProcess 就真的去起
// 哪里，与 8c-3 新增的任何东西无关。
func writeFakeClaudeExe(t *testing.T, path, sessionID string) string {
	t.Helper()
	out := fakeClaudeStdout{
		Type:           "result",
		Subtype:        "success",
		IsError:        false,
		TerminalReason: "completed",
		StopReason:     "end_turn",
		SessionID:      sessionID,
		Result:         "ok",
	}
	out.Usage.InputTokens = 1
	out.Usage.CacheCreationInputTokens = 1
	out.Usage.CacheReadInputTokens = 0
	out.Usage.OutputTokens = 1

	body, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("序列化假 claude 输出失败: %v", err)
	}
	script := "@echo off\n" + "echo " + string(body) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("写假 claude 垫片 %s 失败: %v", path, err)
	}
	return path
}

// configcmdFields 是拼一份 config.toml 正文所需的可变字段，每条测试从
// configcmdBaseFields 起手，只改自己关心的那一项，其余保持一致——
// 这样"到底改了哪一项"在测试代码里一眼可辨，也避免多个字段互相牵连。
type configcmdFields struct {
	dataDir  string
	exePath  string
	model    string
	provider string // agents.ag_claude.provider 的值
}

// renderConfigTOML 把 configcmdFields 拼成一份可以喂给 config.Load 的
// TOML 正文。只有一个 provider（claude）与一个 agent（ag_claude），
// 权限固定 read——本文件六条用例都不需要 workspace/full，权限档不是
// 本块契约要测的东西。
func renderConfigTOML(f configcmdFields) string {
	return fmt.Sprintf(`data_dir = %q
store_thought = true

[providers.claude]
command = %q

[agents.ag_claude]
display_name = "Alice"
provider = %q
model = %q
permissions = "read"
`, f.dataDir, f.exePath, f.provider, f.model)
}

// configcmdFixture 是六条用例共用的前置：一份写在磁盘上的 config.toml、
// 一份用 config.LoadAndValidate 从同一个路径读出来的 *config.Config、
// 一个内存假 Store 支撑的 Service、以及用同一份 cfg 构造出来的
// *turn.Runner。
//
// cfg 与 configPath 特意都对外暴露：/config reload 要重读的是同一个
// 路径（RulesContext.ConfigPath），而 cfg 要与 turn.NewRunner、
// repl.New 共用同一个指针——这与 app.go:139/159 把同一个 *config.Config
// 分别交给 NewRunner 与 repl.New 是同一件事的测试版本。
type configcmdFixture struct {
	cfg        *config.Config
	configPath string
	fields     configcmdFields
	svc        *conversation.Service
	store      *fakeStore
	rn         *turn.Runner
}

// newConfigcmdFixture 造好上面那套前置。dataDir 与 workDir 都建在
// t.TempDir() 下，互不相干：dataDir 只是配置里的一个字符串字段，本文件
// 的用例不依赖它被真正使用（沿用 4.3 节的口径：data_dir 变了就整体拒绝，
// 至于它平时具体控制什么不是本块契约要测的）；workDir 是 Runner 真正
// 落 WORKSPACE/READ 工作目录的地方，必须是一个真实存在、进程有权限
// 读写的目录。
func newConfigcmdFixture(t *testing.T) *configcmdFixture {
	t.Helper()
	dir := t.TempDir()

	fields := configcmdFields{
		dataDir:  filepath.Join(dir, "data"),
		exePath:  writeFakeClaudeExe(t, filepath.Join(dir, "fake-claude.cmd"), configcmdSuccessSessionID),
		model:    "claude-fable-5-1",
		provider: "claude",
	}

	configPath := filepath.Join(dir, "council.toml")
	if err := os.WriteFile(configPath, []byte(renderConfigTOML(fields)), 0o644); err != nil {
		t.Fatalf("写初始配置文件失败: %v", err)
	}

	cfg, err := config.LoadAndValidate(configPath)
	if err != nil {
		t.Fatalf("初始配置应当能通过 LoadAndValidate，实际: %v", err)
	}

	svc, store := newTestService(t)
	rn := turn.NewRunner(cfg, svc, filepath.Join(dir, "workdir"), turn.RulesBlocks{})

	return &configcmdFixture{cfg: cfg, configPath: configPath, fields: fields, svc: svc, store: store, rn: rn}
}

// rewriteConfig 用新的字段集合重写磁盘上的 config.toml，模拟"用户在外部
// 编辑器改了配置文件，回来敲 /config reload"。不改 f.cfg 那个内存指针：
// 是否重新读取、读到之后要不要生效，是 /config reload [--apply] 自己的
// 职责，测试不能替它做这一步。
func (f *configcmdFixture) rewriteConfig(t *testing.T, fields configcmdFields) {
	t.Helper()
	if err := os.WriteFile(f.configPath, []byte(renderConfigTOML(fields)), 0o644); err != nil {
		t.Fatalf("重写配置文件失败: %v", err)
	}
}

// rulesCtx 返回一份只填了 ConfigPath 的 RulesContext——/config reload
// 要重读的"同一个路径"，契约里除了 RulesContext.ConfigPath 没有第二个
// 候选（app.go:158 把同一个 path 同时交给 config.LoadAndValidate 与
// repl.RulesContext.ConfigPath）。这是本文件在契约留白处做的选择，
// 已经写进交卷报告的契约缺口清单。
func (f *configcmdFixture) rulesCtx() RulesContext {
	return RulesContext{ConfigPath: f.configPath}
}

// askDirect 直接调 f.rn.Ask，绕开 REPL 的 /ask 命令。/config 与
// /config reload [--apply] 只能经 REPL 命令触发，但"/ask 这一轮的
// outcome 是什么"不需要、也不应该靠猜屏幕文字去反推——Runner.Ask 本身
// 就会把 Outcome / Fresh / error 原样交回来，这是本任务书点名要求的
// 判据。
func askDirect(t *testing.T, rn *turn.Runner, convID, agentID string) (turn.Record, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), configcmdAskTimeout)
	defer cancel()
	return rn.Ask(ctx, convID, agentID)
}

// mustProviderSession 读回 (convID, agentID) 这一行 provider 会话，
// 不存在或读失败都直接 Fatal——六条用例里凡是调用它的地方，都已经先跑过
// 一轮成功的 Ask，这一行按契约必须在。
func mustProviderSession(t *testing.T, store *fakeStore, convID, agentID string) conversation.ProviderSession {
	t.Helper()
	ps, ok, err := store.ProviderSession(context.Background(), convID, agentID)
	if err != nil {
		t.Fatalf("读 provider 会话 (%s, %s) 失败: %v", convID, agentID, err)
	}
	if !ok {
		t.Fatalf("(%s, %s) 应当已经有一行 provider 会话", convID, agentID)
	}
	return ps
}

// ---------------------------------------------------------------------------
// 4.6 第 1 条：改 model，/config reload（不带 --apply）只报告、不换状态。
// ---------------------------------------------------------------------------

// TestConfigReloadReportsDriftWithoutApplyDoesNotChangeBehavior 覆盖
// 4.6 第 1 条：改某 agent 的 model 之后 /config reload 报出这个成员会
// 漂移，但默认不换任何状态——随后 /ask 仍按旧配置成功、不触发漂移。
//
// "报出这个成员会漂移"这一半用 out 里含 ag_claude 这个 id 做弱断言
// （具体措辞契约未冻结，见交卷报告的契约缺口清单）；"行为不变"这一半
// 是本用例真正的判据，全部走 rn.AuditDrift 与 rn.Ask 的返回值，不看
// 屏幕。
func TestConfigReloadReportsDriftWithoutApplyDoesNotChangeBehavior(t *testing.T) {
	fx := newConfigcmdFixture(t)
	const convID = "conv_cfg_model_00000000000000000000"
	fx.store.seedConversation(convID, "改 model 的漂移预告")
	fx.store.seedParticipant(t, convID, "ag_claude")
	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第一轮，建立身份指纹")

	rec1, err1 := askDirect(t, fx.rn, convID, "ag_claude")
	if err1 != nil || rec1.Outcome != turn.OutcomeSpoke {
		t.Fatalf("前置的第一轮应当成功，实际 outcome=%v err=%v", rec1.Outcome, err1)
	}
	psBefore := mustProviderSession(t, fx.store, convID, "ag_claude")

	changed := fx.fields
	changed.model = "claude-fable-5-9"
	fx.rewriteConfig(t, changed)

	out, err := runREPLWithRules(t, fx.cfg, fx.svc, fx.rn, fx.rulesCtx(), "/config reload\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(out, "ag_claude") {
		t.Errorf("不带 --apply 的 reload 报告里应当点名会漂移的成员 ag_claude（具体措辞未冻结，"+
			"这里只锁 id 出现）；完整输出:\n%s", out)
	}

	// 行为判据：不带 --apply 不该换 Runner 手里的配置，AuditDrift 用的
	// 还是原来的那份，应当仍是 0 条（若 reload 错误地把新配置永久换上，
	// 这里会因为 model 真的变了而报出 1 条）。
	rows, auditErr := fx.rn.AuditDrift(context.Background())
	if auditErr != nil {
		t.Fatalf("AuditDrift 失败: %v", auditErr)
	}
	if len(rows) != 0 {
		t.Errorf("不带 --apply 之后 Runner 手里的配置不该变，AuditDrift 应当仍是 0 条，实际 %d 条: %+v", len(rows), rows)
	}

	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第二轮，验证没有漂移")
	rec2, err2 := askDirect(t, fx.rn, convID, "ag_claude")
	if err2 != nil {
		t.Fatalf("不带 --apply 之后 /ask 应当仍然按旧配置成功，实际报错: %v", err2)
	}
	if rec2.Outcome != turn.OutcomeSpoke {
		t.Fatalf("不带 --apply 之后 outcome 应为 OutcomeSpoke，实际 %v", rec2.Outcome)
	}
	if rec2.Fresh {
		t.Errorf("不带 --apply 不该触发重建，这一轮应当是暖续用（Fresh=false），实际 Fresh=true")
	}
	psAfter := mustProviderSession(t, fx.store, convID, "ag_claude")
	if psAfter.IdentityFP != psBefore.IdentityFP {
		t.Errorf("不带 --apply 时身份指纹不该变，之前 %q 之后 %q", psBefore.IdentityFP, psAfter.IdentityFP)
	}
}

// ---------------------------------------------------------------------------
// 4.6 第 2 条：--apply 之后同一个 /ask 被 ErrIdentityDrift 拒绝；
// /session rebuild 之后下一轮走 Fresh 并成功。
// ---------------------------------------------------------------------------

// TestConfigReloadApplyRejectsThenSessionRebuildGoesFresh 覆盖 4.6 第 2
// 条。前半段（--apply 之后被拒）必须先有一轮"真的跑过"的 Ask，把这条
// 会话的指纹放进 Runner 的进程内内存（r.sess）——只在 fakeStore 里种一行
// 是不够的，Ask 只有在内存里已经有这一条时才会走"指纹不符就拒绝"这条
// 分支，否则会落进"读回、指纹不符就静默作废重建"的另一条分支，那是
// 契约里 4.4/4.2 场景之外的另一种既有行为，测不出本条要测的东西。
func TestConfigReloadApplyRejectsThenSessionRebuildGoesFresh(t *testing.T) {
	fx := newConfigcmdFixture(t)
	const convID = "conv_cfg_apply_00000000000000000000"
	fx.store.seedConversation(convID, "apply 拒绝与 rebuild")
	fx.store.seedParticipant(t, convID, "ag_claude")
	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第一轮，建立身份指纹")

	rec1, err1 := askDirect(t, fx.rn, convID, "ag_claude")
	if err1 != nil || rec1.Outcome != turn.OutcomeSpoke {
		t.Fatalf("前置的第一轮应当成功，实际 outcome=%v err=%v", rec1.Outcome, err1)
	}
	psBefore := mustProviderSession(t, fx.store, convID, "ag_claude")

	changed := fx.fields
	changed.model = "claude-fable-5-8"
	fx.rewriteConfig(t, changed)

	if _, err := runREPLWithRules(t, fx.cfg, fx.svc, fx.rn, fx.rulesCtx(), "/config reload --apply\n"); err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第二轮，应当被身份漂移拒绝")
	rec2, err2 := askDirect(t, fx.rn, convID, "ag_claude")
	if !errors.Is(err2, turn.ErrIdentityDrift) {
		t.Fatalf("--apply 之后同一个 /ask 应当被 ErrIdentityDrift 拒绝，实际 err=%v", err2)
	}
	if rec2.Outcome != turn.OutcomeRefused {
		t.Errorf("被身份漂移拒绝时 Outcome 应为 OutcomeRefused，实际 %v", rec2.Outcome)
	}
	psStillOld := mustProviderSession(t, fx.store, convID, "ag_claude")
	if psStillOld.IdentityFP != psBefore.IdentityFP {
		t.Errorf("被拒绝的这一轮不该改动库里已经落盘的指纹，之前 %q 现在 %q", psBefore.IdentityFP, psStillOld.IdentityFP)
	}

	if _, err := runREPLWithRules(t, fx.cfg, fx.svc, fx.rn, fx.rulesCtx(),
		"/use "+convID+"\n/session rebuild ag_claude\n"); err != nil {
		t.Fatalf("/session rebuild 那一轮 Run 失败: %v", err)
	}

	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第三轮，rebuild 之后应当走 Fresh")
	rec3, err3 := askDirect(t, fx.rn, convID, "ag_claude")
	if err3 != nil {
		t.Fatalf("/session rebuild 之后 /ask 应当成功，实际报错: %v", err3)
	}
	if rec3.Outcome != turn.OutcomeSpoke {
		t.Fatalf("/session rebuild 之后 outcome 应为 OutcomeSpoke，实际 %v", rec3.Outcome)
	}
	if !rec3.Fresh {
		t.Errorf("/session rebuild 之后下一轮应当走 Fresh，实际 Fresh=false")
	}
	psNew := mustProviderSession(t, fx.store, convID, "ag_claude")
	if psNew.IdentityFP == psBefore.IdentityFP {
		t.Errorf("rebuild 之后新指纹应当反映已经生效的新 model，不该与最早那份旧指纹相同")
	}
}

// ---------------------------------------------------------------------------
// 4.6 第 3 条：脑裂断言——新增 agent，--apply 之后 /agents 列得出它。
// ---------------------------------------------------------------------------

// TestConfigReloadApplyUpdatesBothRunnerAndREPLConfig 覆盖 4.6 第 3 条，
// 也就是契约 4.2 的直接验收：--apply 必须同时换掉 Runner.SetConfig 与
// REPL 自己的 r.cfg 这两处持有者。
//
// 这条断言只有在"--apply"与"/agents"落在**同一个 REPL.Run 调用**里才有
// 意义：r.cfg 是 *REPL 这个结构体自己的字段，每次 New(...) 都会用调用方
// 传入的 cfg 参数重新赋值一次；如果分两次 runREPLWithRules 调用，第二次
// 传的还是测试变量里那个更早的 *config.Config 指针，即使第一次调用里
// --apply 真的换过 REPL 内部那个字段，第二个全新的 REPL 实例也无从得知，
// /agents 就会看着一份注定不含新 agent 的配置，把"只换了 Runner 那一份"
// 的 bug 也测成通过——这恰恰是契约 4.2 点名批评的那种测不出东西的验收。
// 所以本用例把 --apply 与 /agents 写在同一行输入里，一次 Run 内完成。
//
// 订正一（变异测试暴露）：最早那一版断言是 strings.Contains(out, "ag_new")，
// 其中 out 是 "/config reload --apply\n/agents\n" 的**整段**输出。这是一条
// 空用例：把 --apply 那一支里 "同步换掉 REPL 自己的 r.cfg" 这一步删掉之后，
// 这条断言照样绿——因为 /config reload --apply 自己的报告本来就会点名
// "配置里新增的 agent"，把新增的 id 逐个打出来，"ag_new" 这个子串在那一行
// 报告里就已经出现，与 /agents 到底看到了新配置还是旧配置完全无关。
//
// 订正二（订正一的修法本身翻了车）：订正一改用"只跑 reload、不跑 /agents"
// 的第二个 REPL 实例做前缀对照，指望 outCombined 以 outReloadOnly 为前缀。
// 这个假设不成立：REPL 每读完一行都会先打一次提示符 "council> " 再读下一
// 行；只跑一条命令的那次，最后一个 "council> " 后面紧跟的是 EOF 触发的告别
// 行（"\n再见\n"）；跑两条命令的那次，同一个位置后面紧跟的却是 /agents 的
// 表格。两段恰好在最后一个提示符处分叉，"reload-only 是 combined 的前缀"
// 从一开始就不成立，正确代码下也会被 t.Fatalf 那道守卫拦下、判成失败。
//
// 现在这版不做任何前缀算术，换一个不需要猜 configcmd.go 措辞、也不需要
// 关心 REPL 提示符/告别行细节的锚点：/agents 的表头
// "ID  PROVIDER  MODEL  EFFORT  PERMISSIONS  MODE"（commands.go 里
// cmdAgents 的 fmt.Fprintln 那一行，8c-3 之外的既有代码，不是本次要测的
// 东西）。"PERMISSIONS" 这个词只会出现在这一行表头里——reload 报告说的是
// "配置里新增的 agent" 这类话，不会打印英文列名——所以找到它最后一次出现
// 的位置，只在它之后的那一段里找 ag_new，就等价于只在 /agents 真正打印的
// 表格里找 ag_new，不会被 reload 报告里提前出现的同一个 agent id 污染。
func TestConfigReloadApplyUpdatesBothRunnerAndREPLConfig(t *testing.T) {
	fx := newConfigcmdFixture(t)

	newAgentTOML := renderConfigTOML(fx.fields) + `
[agents.ag_new]
display_name = "Nova"
provider = "claude"
model = "claude-fable-5-1"
permissions = "read"
`
	if err := os.WriteFile(fx.configPath, []byte(newAgentTOML), 0o644); err != nil {
		t.Fatalf("重写配置文件失败: %v", err)
	}

	out, err := runREPLWithRules(t, fx.cfg, fx.svc, fx.rn, fx.rulesCtx(), "/config reload --apply\n/agents\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	// 先确认 /agents 真的跑到了、表头真的出现过，这一步失败说明命令本身
	// 没被认出来或者中途出了别的错，与脑裂断言无关，必须先排除。
	headerIdx := strings.LastIndex(out, "PERMISSIONS")
	if headerIdx < 0 {
		t.Fatalf("没有找到 /agents 的表头（\"PERMISSIONS\" 这一列），/agents 大概没有正常"+
			"跑完；完整输出:\n%s", out)
	}
	agentsTable := out[headerIdx:]

	// 脑裂断言（契约 4.2 的直接验收）：只在表头之后的那一段——也就是
	// /agents 真正打印的表格本体——里找 ag_new，不看它前面 reload 报告
	// 那一段。只换 Runner 那一份时，/agents 读的是 REPL 自己那份没被换过
	// 的旧 r.cfg，表格里只会有 ag_claude 一行，这一段里找不到 ag_new——
	// 即使 reload 报告本身已经点过名（那一段在表头之前，已经被排除在外，
	// 不算数）。这正是两次订正要修的东西：老断言在整段 out 里找，
	// 这一版只在表头之后找。
	if !strings.Contains(agentsTable, "ag_new") {
		t.Errorf("脑裂断言（契约 4.2 的直接验收）：--apply 之后同一个 REPL 会话里，"+
			"/agents 表头之后的表格本体应当列得出新增的 ag_new；这条断言只在 REPL 自己的 "+
			"r.cfg 也被换掉时才会通过——只换 Runner 那一份会让表格里只有 ag_claude 一行，"+
			"这里找不到 ag_new，哪怕 reload 报告自己已经点过名（那一段在表头之前，"+
			"本断言故意不看）。\n表头之后的那一段:\n%s", agentsTable)
	}
}

// ---------------------------------------------------------------------------
// 4.6 第 4 条：改 data_dir，reload 整体拒绝，随后 /ask 仍能正常跑完一轮。
// ---------------------------------------------------------------------------

// TestConfigReloadRejectsDataDirChangeButAskStillWorks 覆盖 4.6 第 4
// 条。"状态一个字没换"这一半完全走行为断言（AuditDrift 为 0、随后
// /ask 暖续用成功、指纹不变），不依赖 /agents 的输出——契约 4.2 明确
// 指出那条断言是恒真的，测不出东西。
//
// "reload 被拒绝"本身这一件事，除了屏幕文字之外没有第二个可观察的信号：
// data_dir 不参与 identityFPFor 的输入（workDir 是 Runner 构造时定死的
// 基路径，与 cfg.DataDir 是两回事），所以哪怕 reload 错误地放行了这次
// 改动，AuditDrift 也测不出来——这里对屏幕输出的依赖是必要的，不是可以
// 绕开的偷懒，已经写进交卷报告的契约缺口清单。
func TestConfigReloadRejectsDataDirChangeButAskStillWorks(t *testing.T) {
	fx := newConfigcmdFixture(t)
	const convID = "conv_cfg_datadir_0000000000000000000"
	fx.store.seedConversation(convID, "data_dir 改动应当被整体拒绝")
	fx.store.seedParticipant(t, convID, "ag_claude")
	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第一轮，建立身份指纹")

	rec1, err1 := askDirect(t, fx.rn, convID, "ag_claude")
	if err1 != nil || rec1.Outcome != turn.OutcomeSpoke {
		t.Fatalf("前置的第一轮应当成功，实际 outcome=%v err=%v", rec1.Outcome, err1)
	}
	psBefore := mustProviderSession(t, fx.store, convID, "ag_claude")

	changed := fx.fields
	changed.dataDir = filepath.Join(filepath.Dir(fx.fields.dataDir), "another-data-dir")
	fx.rewriteConfig(t, changed)

	for _, line := range []string{"/config reload\n", "/config reload --apply\n"} {
		out, err := runREPLWithRules(t, fx.cfg, fx.svc, fx.rn, fx.rulesCtx(), line)
		if err != nil {
			t.Fatalf("%q 那一轮 Run 失败: %v", line, err)
		}
		if !strings.Contains(out, "data_dir") {
			t.Errorf("%q 应当点名 data_dir 是这次整体拒绝的原因（假设：拒绝理由里会提到这个"+
				"字面键名，见交卷报告的契约缺口清单）；完整输出:\n%s", line, out)
		}
	}

	rows, auditErr := fx.rn.AuditDrift(context.Background())
	if auditErr != nil {
		t.Fatalf("AuditDrift 失败: %v", auditErr)
	}
	if len(rows) != 0 {
		t.Errorf("data_dir 改动被拒绝之后 Runner 手里的配置不该变，AuditDrift 应当仍是 0 条，实际 %d 条: %+v", len(rows), rows)
	}

	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第二轮，验证仍能正常跑完")
	rec2, err2 := askDirect(t, fx.rn, convID, "ag_claude")
	if err2 != nil {
		t.Fatalf("data_dir 改动被拒绝之后 /ask 仍应正常跑完一轮，实际报错: %v", err2)
	}
	if rec2.Outcome != turn.OutcomeSpoke {
		t.Fatalf("outcome 应为 OutcomeSpoke，实际 %v", rec2.Outcome)
	}
	if rec2.Fresh {
		t.Errorf("状态没变，这一轮应当是暖续用（Fresh=false），实际 Fresh=true")
	}
	psAfter := mustProviderSession(t, fx.store, convID, "ag_claude")
	if psAfter.IdentityFP != psBefore.IdentityFP {
		t.Errorf("身份指纹不该变，之前 %q 之后 %q", psBefore.IdentityFP, psAfter.IdentityFP)
	}
}

// ---------------------------------------------------------------------------
// 4.6 第 5 条：换 provider.command，--apply 后屏幕告警，且 AuditDrift 为 0。
// ---------------------------------------------------------------------------

// TestConfigReloadApplyWarnsOnProviderCommandChangeWithZeroDrift 覆盖
// 4.6 第 5 条，也是契约 4.3 点名的"指纹补不到的洞"：PrefixArgsDigest
// 只摘 cmd.Args，Command.Path 从不进指纹，所以换一个可执行文件路径，
// AuditDrift 一个字都不会报。这条用例要同时成立两件事，缺一都证明不了
// 告警在补这个洞：
//  1. screen 上确实出现了告警（否则这个洞对用户完全不可见）；
//  2. AuditDrift 确实是 0 条（否则告警可能只是在重复 AuditDrift 已经
//     报过的东西，不是在补它测不到的洞）。
//
// 换的第二份垫片内容与第一份一模一样（同一个 session_id），用来证明
// "换路径之后同一条会话还能暖续用"——这正是这个洞的现实后果：即使
// 可执行文件真的换了，Runner 认为什么都没变。
func TestConfigReloadApplyWarnsOnProviderCommandChangeWithZeroDrift(t *testing.T) {
	fx := newConfigcmdFixture(t)
	const convID = "conv_cfg_command_000000000000000000"
	fx.store.seedConversation(convID, "换 provider.command")
	fx.store.seedParticipant(t, convID, "ag_claude")
	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第一轮，建立会话")

	rec1, err1 := askDirect(t, fx.rn, convID, "ag_claude")
	if err1 != nil || rec1.Outcome != turn.OutcomeSpoke {
		t.Fatalf("前置的第一轮应当成功，实际 outcome=%v err=%v", rec1.Outcome, err1)
	}

	secondExePath := filepath.Join(filepath.Dir(fx.fields.exePath), "fake-claude-v2.cmd")
	writeFakeClaudeExe(t, secondExePath, configcmdSuccessSessionID)

	changed := fx.fields
	changed.exePath = secondExePath
	fx.rewriteConfig(t, changed)

	out, err := runREPLWithRules(t, fx.cfg, fx.svc, fx.rn, fx.rulesCtx(), "/config reload --apply\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(out, "command") {
		t.Errorf("provider.command 变了应当响亮告警（假设：告警文本里会出现这个字面键名，"+
			"见交卷报告的契约缺口清单）；完整输出:\n%s", out)
	}
	if !strings.Contains(out, "rebuild") {
		t.Errorf("契约 4.3 明写告警文本要建议 /session rebuild；完整输出:\n%s", out)
	}

	rows, auditErr := fx.rn.AuditDrift(context.Background())
	if auditErr != nil {
		t.Fatalf("AuditDrift 失败: %v", auditErr)
	}
	if len(rows) != 0 {
		t.Errorf("换 provider.command 不该被 AuditDrift 报出漂移（它正是指纹补不到的那个洞），"+
			"实际 %d 条: %+v", len(rows), rows)
	}

	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第二轮，换路径之后")
	rec2, err2 := askDirect(t, fx.rn, convID, "ag_claude")
	if err2 != nil {
		t.Fatalf("换 command 之后 /ask 应当仍然成功，实际报错: %v", err2)
	}
	if rec2.Outcome != turn.OutcomeSpoke {
		t.Fatalf("outcome 应为 OutcomeSpoke，实际 %v", rec2.Outcome)
	}
	if rec2.Fresh {
		t.Errorf("换 command 不该触发重建，实际 Fresh=true")
	}
}

// ---------------------------------------------------------------------------
// 4.6 第 6 条：agent.provider 指向不存在的名字，reload 逐条打印校验
// 错误，随后 /ask 仍按旧配置正常跑完一轮。
// ---------------------------------------------------------------------------

// TestConfigReloadPrintsValidationErrorsAndKeepsOldConfig 覆盖 4.6 第 6
// 条。"逐条打印校验错误"这一半断言的是 internal/config/validate.go 里
// 已经冻结的措辞（ValidationError.Error() 与 Validate() 里
// "agent %q references undefined provider %q" 这句话本来就是既有代码，
// 不是 8c-3 要新写的东西），不是在猜 configcmd.go 会怎么措辞它自己的
// 外层文字。
func TestConfigReloadPrintsValidationErrorsAndKeepsOldConfig(t *testing.T) {
	fx := newConfigcmdFixture(t)
	const convID = "conv_cfg_badprovider_00000000000000"
	fx.store.seedConversation(convID, "agent.provider 指向不存在的名字")
	fx.store.seedParticipant(t, convID, "ag_claude")
	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第一轮，建立身份指纹")

	rec1, err1 := askDirect(t, fx.rn, convID, "ag_claude")
	if err1 != nil || rec1.Outcome != turn.OutcomeSpoke {
		t.Fatalf("前置的第一轮应当成功，实际 outcome=%v err=%v", rec1.Outcome, err1)
	}
	psBefore := mustProviderSession(t, fx.store, convID, "ag_claude")

	changed := fx.fields
	changed.provider = "no-such-provider"
	fx.rewriteConfig(t, changed)

	for _, line := range []string{"/config reload\n", "/config reload --apply\n"} {
		out, err := runREPLWithRules(t, fx.cfg, fx.svc, fx.rn, fx.rulesCtx(), line)
		if err != nil {
			t.Fatalf("%q 那一轮 Run 失败: %v", line, err)
		}
		if !strings.Contains(out, "references undefined provider") {
			t.Errorf("%q 应当逐条打印 Validate() 产出的校验错误（这句措辞来自"+
				"internal/config/validate.go 里已经冻结的代码，不是本次新写的）；完整输出:\n%s", line, out)
		}
		if !strings.Contains(out, `"no-such-provider"`) {
			t.Errorf("%q 的校验错误应当点名那个不存在的 provider 名字；完整输出:\n%s", line, out)
		}
	}

	rows, auditErr := fx.rn.AuditDrift(context.Background())
	if auditErr != nil {
		t.Fatalf("AuditDrift 失败: %v", auditErr)
	}
	if len(rows) != 0 {
		t.Errorf("非法配置被拒绝之后 Runner 手里的配置不该变，AuditDrift 应当仍是 0 条，实际 %d 条: %+v", len(rows), rows)
	}

	fx.store.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "第二轮，仍按旧配置跑完")
	rec2, err2 := askDirect(t, fx.rn, convID, "ag_claude")
	if err2 != nil {
		t.Fatalf("非法配置被拒绝之后 /ask 仍应按旧配置正常跑完一轮，实际报错: %v", err2)
	}
	if rec2.Outcome != turn.OutcomeSpoke {
		t.Fatalf("outcome 应为 OutcomeSpoke，实际 %v", rec2.Outcome)
	}
	if rec2.Fresh {
		t.Errorf("非法配置被拒绝之后状态没变，这一轮应当是暖续用（Fresh=false），实际 Fresh=true")
	}
	psAfter := mustProviderSession(t, fx.store, convID, "ag_claude")
	if psAfter.IdentityFP != psBefore.IdentityFP {
		t.Errorf("身份指纹不该变，之前 %q 之后 %q", psBefore.IdentityFP, psAfter.IdentityFP)
	}
}
