package repl

// rules_test.go 覆盖 11a 契约 5.1（/rules）、5.2（/rules reload [--rebuild]）
// 与 5.3（/session rebuild）。
//
// 这几条命令都要读真正的 config.LoadRules，而它在 rulesFile 为空时仍会
// 去解析全局规则路径（环境变量 COUNCIL_GLOBAL_RULES -> ~/.council/COUNCIL.md）。
// 每个用例都必须用 t.Setenv 把它指到一个确定不存在的路径，否则测试会去
// 读运行这份测试的人自己的 ~/.council/COUNCIL.md——那是这台机器上的真实
// 文件，内容与是否存在完全不可控，会让测试结果随机漂移。

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/turn"
)

// isolateGlobalRules 把 COUNCIL_GLOBAL_RULES 指到一个确定不存在的路径，
// 让本文件里"没有全局规则"的用例可控，不受运行测试这台机器上真实的
// ~/.council/COUNCIL.md 内容影响。
func isolateGlobalRules(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "no-such-global-rules.md")
	t.Setenv(config.EnvGlobalRules, path)
	return path
}

// rulesTestConfig 造一份三个 agent 分别挂在 claude/grok/agy 上的配置——
// 与 newTestREPLConfig（pv_alpha 等假 provider id）不同，/rules 要报的
// CleanInstrEnv 与命令行余量来自 internal/provider 里真正登记的三个
// 适配器，必须用它们认得的 provider id。
func rulesTestConfig() *config.Config {
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: "claude-fake"},
			"grok":   {Command: "grok-fake"},
			"agy":    {Command: "agy-fake"},
		},
		Agents: map[string]config.AgentConfig{
			"ag_claude": {DisplayName: "Alice", Provider: "claude", Model: "claude-fable-5-1", Permissions: config.PermissionRead},
			"ag_grok":   {DisplayName: "Zulu", Provider: "grok", Model: "grok-4.6", Permissions: config.PermissionRead},
			"ag_agy":    {DisplayName: "Nova", Provider: "agy", Model: "gemini-3.8-flash-high", Permissions: config.PermissionRead},
		},
	}
}

// runREPLWithRules 起一个带真正 *turn.Runner 与 RulesContext 的 REPL 跑到
// 结束，带看门狗。与 ask_test.go 的 runREPLRunner 是同一个模式，多一个
// rulesCtx 参数。
func runREPLWithRules(t *testing.T, cfg *config.Config, svc *conversation.Service, rn *turn.Runner, rulesCtx RulesContext, input string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	r := New(cfg, svc, rn, strings.NewReader(input), &out, rulesCtx)

	done := make(chan error, 1)
	go func() { done <- r.Run(context.Background()) }()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(testREPLTimeout):
		t.Fatalf("REPL.Run 在 %v 内没有返回", testREPLTimeout)
		return "", nil
	}
}

// ============================================================
// /rules（不带参数）
// ============================================================

// TestRulesRequiresRunnerNotConversation 覆盖：/rules 只需要 rn 非空，
// 不需要先选一个会话——这是它与 /ask、/session 的区别，命令表里没给它
// needsConv。svc 非空但没有当前会话、rn 为 nil 时应当答 msgNoRunner，
// 而不是 msgNoConv（如果实现漏掉这一点、误给 /rules 加上 needsConv，
// 这里会先答 msgNoConv）。
func TestRulesRequiresRunnerNotConversation(t *testing.T) {
	isolateGlobalRules(t)
	svc, _ := newTestService(t)

	out, err := runREPLWithRules(t, newTestREPLConfig(), svc, nil, RulesContext{}, "/rules\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}
	if !strings.Contains(out, msgNoRunner) {
		t.Errorf("没有答 msgNoRunner；完整输出:\n%s", out)
	}
	if strings.Contains(out, msgNoConv) {
		t.Errorf("/rules 不该要求先选会话，却答出了 msgNoConv；完整输出:\n%s", out)
	}
}

// TestRulesShowsFileStatusByteCountsAndProviderRows 覆盖 5.1 给的主要
// 格式点：两个规则文件的路径/在不在/字节数、合计、逐 provider 一行
// （隔离位 + 命令行余量），且局部规则未配置时显示"未配置"而不是
// "不存在"（两者是不同的正常状态，配置里没写 rules_file 与写了但文件
// 缺失必须分得开）。
func TestRulesShowsFileStatusByteCountsAndProviderRows(t *testing.T) {
	isolateGlobalRules(t)
	cfg := rulesTestConfig()
	svc, _ := newTestService(t)

	const globalText = "全局规则正文"
	rn := turn.NewRunner(cfg, svc, t.TempDir(), turn.RulesBlocks{Global: globalText})
	rulesCtx := RulesContext{
		ConfigPath: filepath.Join(t.TempDir(), "council.toml"),
		Rules: config.Rules{
			GlobalPath:  "/fake/home/.council/COUNCIL.md",
			GlobalText:  globalText,
			GlobalFound: true,
			// LocalPath 空串：配置里没写 rules_file，这是"未配置"，
			// 不是"写了但文件不存在"。
		},
	}

	out, err := runREPLWithRules(t, cfg, svc, rn, rulesCtx, "/rules\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	if !strings.Contains(out, "/fake/home/.council/COUNCIL.md") {
		t.Errorf("应当显示全局规则的解析后路径；完整输出:\n%s", out)
	}
	if !strings.Contains(out, "(未配置)") {
		t.Errorf("局部规则没配置时应当显示占位符“未配置”；完整输出:\n%s", out)
	}
	if strings.Count(out, "不存在") != 0 {
		t.Errorf("全局规则确实存在、局部规则是未配置而不是缺文件，不该出现“不存在”；完整输出:\n%s", out)
	}
	wantTotal := len(globalText)
	if !strings.Contains(out, fmt.Sprintf("合计 %d 字节", wantTotal)) {
		t.Errorf("应当显示合计 %d 字节；完整输出:\n%s", wantTotal, out)
	}

	for _, id := range []string{"claude", "grok", "agy"} {
		if !strings.Contains(out, id) {
			t.Errorf("provider %s 应当出现在报告里；完整输出:\n%s", id, out)
		}
	}
	if !strings.Contains(out, "唯一指令源") {
		t.Errorf("claude 应当显示为唯一指令源；完整输出:\n%s", out)
	}
	if !strings.Contains(out, "叠加在用户全局规则之上") {
		t.Errorf("grok 应当显示为叠加在用户全局规则之上；完整输出:\n%s", out)
	}
	if !strings.Contains(out, "未验证，按叠加处理") {
		t.Errorf("agy 应当显示为未验证、按最坏情况（叠加）处理；完整输出:\n%s", out)
	}
	if !strings.Contains(out, "进缓存前缀，首轮付") {
		t.Errorf("claude 与 grok 应当显示为进缓存前缀、首轮付；完整输出:\n%s", out)
	}
	if !strings.Contains(out, "无缓存，按轮数乘着付") {
		t.Errorf("agy 应当显示为无缓存、按轮数乘着付；完整输出:\n%s", out)
	}
	if !strings.Contains(out, "命令行余量") {
		t.Errorf("每一行都应当报命令行余量；完整输出:\n%s", out)
	}
	if strings.Contains(out, "身份已变") {
		t.Errorf("库里没有任何 provider 会话，不该出现漂移那一行；完整输出:\n%s", out)
	}
}

// TestRulesLastLineOnlyAppearsWhenDriftExists 覆盖：漂移审计那一行只在
// 有漂移时才出现（0 条时整行不印，不制造噪声）。用同一个 Runner 先跑
// 一次干净的（无漂移），再往库里塞一条身份指纹解不开的会话之后重跑一次，
// 证明这一行确实是随漂移的有无出现/消失，不是恒定打印的固定文本。
func TestRulesLastLineOnlyAppearsWhenDriftExists(t *testing.T) {
	isolateGlobalRules(t)
	cfg := rulesTestConfig()
	svc, f := newTestService(t)
	rn := turn.NewRunner(cfg, svc, t.TempDir(), turn.RulesBlocks{})

	before, err := runREPLWithRules(t, cfg, svc, rn, RulesContext{}, "/rules\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if strings.Contains(before, "身份已变") {
		t.Fatalf("前置条件不成立：库里还没有任何 provider 会话，不该有漂移；完整输出:\n%s", before)
	}

	f.seedConversation("conv_drift_0000000000000000000000", "漂移用例")
	f.seedProviderSession(conversation.ProviderSession{
		ConvID:     "conv_drift_0000000000000000000000",
		AgentID:    "ag_claude",
		IdentityFP: "这不是合法编码",
	})

	after, err := runREPLWithRules(t, cfg, svc, rn, RulesContext{}, "/rules\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(after, "身份已变的 provider 会话: 1 条") {
		t.Errorf("塞入一条解不开的指纹之后应当报出 1 条漂移；完整输出:\n%s", after)
	}
}

// TestCacheLabelNeverNamesAProvider 覆盖 11a 契约 5.1 与本文件顶部注释的
// 硬约束："不要在 repl 里写任何 provider 专有的话术"。cacheLabel 取代的
// 那句旧 pricingNote 固定文本恰恰点名了 agy，这条测试直接对三态枚举的
// 每一种取值断言：输出里不含 claude / grok / agy 这三个字，且三档的文本
// 必须两两不同——否则用户分不清哪家有缓存收益、哪家没有。
func TestCacheLabelNeverNamesAProvider(t *testing.T) {
	cases := []provider.Support{provider.SupportYes, provider.SupportNo, provider.SupportUnknown}
	seen := make(map[string]bool, len(cases))
	for _, s := range cases {
		label := cacheLabel(s)
		if label == "" {
			t.Errorf("cacheLabel(%v) 为空", s)
		}
		for _, name := range []string{"claude", "grok", "agy"} {
			if strings.Contains(label, name) {
				t.Errorf("cacheLabel(%v) = %q 不该出现 provider 名字 %q", s, label, name)
			}
		}
		if seen[label] {
			t.Errorf("cacheLabel(%v) = %q 与另一档撞了，三档必须分得开", s, label)
		}
		seen[label] = true
	}
}

// ============================================================
// /rules reload [--rebuild]
// ============================================================

// TestRulesReloadWithoutRebuildDoesNotChangeState 覆盖契约 5.2 与验收
// 第 9 条：不带 --rebuild 只报告，调用前后 Runner 手里的规则必须相同——
// 直接比较 rn.Rules()，而不是只看屏幕输出像不像"没变"。
func TestRulesReloadWithoutRebuildDoesNotChangeState(t *testing.T) {
	isolateGlobalRules(t)
	cfg := rulesTestConfig()
	svc, _ := newTestService(t)

	localPath := filepath.Join(t.TempDir(), "COUNCIL.md")
	if err := os.WriteFile(localPath, []byte("旧的局部规则"), 0o644); err != nil {
		t.Fatalf("写局部规则文件失败: %v", err)
	}
	loaded, err := config.LoadRules(filepath.Join(t.TempDir(), "council.toml"), localPath)
	if err != nil {
		t.Fatalf("LoadRules 失败: %v", err)
	}
	oldBlocks := turn.RulesBlocks{Global: loaded.GlobalText, Local: loaded.LocalText}
	rn := turn.NewRunner(cfg, svc, t.TempDir(), oldBlocks)
	rulesCtx := RulesContext{ConfigPath: loaded.LocalPath, RulesFile: localPath, Rules: loaded}
	// ConfigPath 直接传 localPath 的绝对路径没有意义（它不是配置文件），
	// 但 rulesFile 本身已经是绝对路径，LoadRules 解析相对路径时不会用到
	// ConfigPath 的目录部分，所以这里的 ConfigPath 具体是什么无关紧要。

	// 改大局部规则文件，制造"重读会看到不同内容"的局面。
	if err := os.WriteFile(localPath, []byte("新的局部规则，比旧的长很多很多很多"), 0o644); err != nil {
		t.Fatalf("重写局部规则文件失败: %v", err)
	}

	out, err := runREPLWithRules(t, cfg, svc, rn, rulesCtx, "/rules reload\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(out, "要应用请敲 /rules reload --rebuild") {
		t.Errorf("不带 --rebuild 应当提示如何应用；完整输出:\n%s", out)
	}
	if got := rn.Rules(); got != oldBlocks {
		t.Errorf("不带 --rebuild 不该改变 Runner 手里的规则，实际从 %+v 变成了 %+v", oldBlocks, got)
	}
}

// TestRulesReloadRebuildAppliesAndRebuildsDriftedSessions 覆盖带
// --rebuild 的那一半：重读、SetRules、对每一条漂移的会话调 Rebuild，
// 且之后 RulesContext.Rules 更新为新读到的那一份（否则之后的 /rules
// 会显示旧的）。
//
// 漂移的种子行故意用一个解不开的 IdentityFP，而不是费力去算"旧规则下
// 的真实指纹"——解不开的指纹与规则变了同属「会重建、付入场费」那一类
// （X-39f 的 DriftFingerprintUnparsable / DriftFieldsChanged，AskRefused 为假），
// 这里只是借用最省事的一种构造方式来驱动同一条路径，具体原因文本已经在
// internal/turn/drift_test.go 里逐项测过；「会被拒、不花钱」那一类的措辞在 x39f_driftwording_test.go。
func TestRulesReloadRebuildAppliesAndRebuildsDriftedSessions(t *testing.T) {
	isolateGlobalRules(t)
	cfg := rulesTestConfig()
	svc, f := newTestService(t)
	rn := turn.NewRunner(cfg, svc, t.TempDir(), turn.RulesBlocks{})

	const convID = "conv_rebuild_00000000000000000000"
	f.seedConversation(convID, "reload --rebuild 用例")
	f.seedProviderSession(conversation.ProviderSession{
		ConvID:     convID,
		AgentID:    "ag_claude",
		IdentityFP: "恒漂移的坏编码",
	})

	localPath := filepath.Join(t.TempDir(), "COUNCIL.md")
	if err := os.WriteFile(localPath, []byte("新读到的局部规则"), 0o644); err != nil {
		t.Fatalf("写局部规则文件失败: %v", err)
	}
	rulesCtx := RulesContext{RulesFile: localPath}

	out, err := runREPLWithRules(t, cfg, svc, rn, rulesCtx, "/rules reload --rebuild\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(out, "已重建") || !strings.Contains(out, "ag_claude") {
		t.Errorf("应当报告已重建 conv_rebuild.../ag_claude 这一条；完整输出:\n%s", out)
	}
	if _, ok, _ := f.ProviderSession(context.Background(), convID, "ag_claude"); ok {
		t.Error("--rebuild 之后这条 provider 会话应当被整行删掉")
	}
	if got := rn.Rules().Local; got != "新读到的局部规则" {
		t.Errorf("--rebuild 之后 Runner 手里的规则应当换成新读到的那份，实际 Local=%q", got)
	}
}

// TestRulesReloadReadFailureChangesNothing 覆盖"读文件失败时两种形态都
// 不改任何状态"：rules_file 指向一个目录（Windows/Unix 上对目录调
// os.ReadFile 都会报错，不是"文件不存在"那条正常路径），LoadRules 必须
// 报错，命令必须原样把错误打出来，且不改 Runner 手里的规则。
func TestRulesReloadReadFailureChangesNothing(t *testing.T) {
	isolateGlobalRules(t)
	cfg := rulesTestConfig()
	svc, _ := newTestService(t)

	oldBlocks := turn.RulesBlocks{Global: "不该被改动的规则"}
	rn := turn.NewRunner(cfg, svc, t.TempDir(), oldBlocks)
	rulesCtx := RulesContext{RulesFile: t.TempDir()} // 指向一个目录，必然读失败

	out, err := runREPLWithRules(t, cfg, svc, rn, rulesCtx, "/rules reload --rebuild\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil（领域错误只打印，不是 I/O 失败），实际 %v", err)
	}
	if !strings.Contains(out, "重读规则文件失败") {
		t.Errorf("应当报告重读失败；完整输出:\n%s", out)
	}
	if got := rn.Rules(); got != oldBlocks {
		t.Errorf("读文件失败时不该改变 Runner 手里的规则，实际从 %+v 变成了 %+v", oldBlocks, got)
	}
}

// ============================================================
// /session rebuild <agentID>
// ============================================================

// TestSessionRebuildUnknownAgentRefuses 覆盖：agentID 不在当前会话的
// 活跃成员里时报错，不动任何状态。
func TestSessionRebuildUnknownAgentRefuses(t *testing.T) {
	svc, f := newTestService(t)
	rn := turn.NewRunner(rulesTestConfig(), svc, t.TempDir(), turn.RulesBlocks{})
	const convID = "conv_session_rebuild_unknown00000000"
	f.seedConversation(convID, "未知成员")

	out, err := runREPLWithRules(t, rulesTestConfig(), svc, rn, RulesContext{}, "/use "+convID+"\n/session rebuild ghost\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(out, "不在当前会话的活跃成员里") {
		t.Errorf("应当报告 ghost 不是活跃成员；完整输出:\n%s", out)
	}
}

// TestSessionRebuildOnFreshMemberSaysNoExistingSession 覆盖 existed=false
// 的措辞：一个活跃成员从没被 /ask 过，rebuild 应当说"本来就没有"，
// 而不是伪装成真的丢弃了什么。
func TestSessionRebuildOnFreshMemberSaysNoExistingSession(t *testing.T) {
	svc, f := newTestService(t)
	rn := turn.NewRunner(rulesTestConfig(), svc, t.TempDir(), turn.RulesBlocks{})
	const convID = "conv_session_rebuild_fresh000000000"
	f.seedConversation(convID, "从没开过口")
	f.seedParticipant(t, convID, "ag_claude")

	out, err := runREPLWithRules(t, rulesTestConfig(), svc, rn, RulesContext{}, "/use "+convID+"\n/session rebuild ag_claude\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(out, "本来就没有开着的会话") {
		t.Errorf("应当说本来就没有开着的会话；完整输出:\n%s", out)
	}
}

// TestSessionRebuildDropsExistingSession 覆盖成功路径：五样全丢的措辞，
// 且库里那一行确实被删掉了。
func TestSessionRebuildDropsExistingSession(t *testing.T) {
	svc, f := newTestService(t)
	rn := turn.NewRunner(rulesTestConfig(), svc, t.TempDir(), turn.RulesBlocks{})
	const convID = "conv_session_rebuild_drop0000000000"
	f.seedConversation(convID, "有会话")
	f.seedParticipant(t, convID, "ag_claude")
	f.seedProviderSession(conversation.ProviderSession{
		ConvID:     convID,
		AgentID:    "ag_claude",
		IdentityFP: "whatever",
	})

	out, err := runREPLWithRules(t, rulesTestConfig(), svc, rn, RulesContext{}, "/use "+convID+"\n/session rebuild ag_claude\n")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(out, "已丢弃") || !strings.Contains(out, "五样全丢") {
		t.Errorf("应当报告五样全丢；完整输出:\n%s", out)
	}
	if _, ok, _ := f.ProviderSession(context.Background(), convID, "ag_claude"); ok {
		t.Error("rebuild 之后这一行应当被整行删掉")
	}
}
