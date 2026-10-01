package repl

// x39f_driftwording_test.go 覆盖 X-39f 契约 2.3：/rules、/rules reload
// [--rebuild]、/session rebuild 的措辞按后果分两类，以及 /session rebuild
// 放宽的闸（只认本会话、精确 id、第 1 类）与审计失败时 fail-closed。
//
// 不引用新 API（DriftKind / AskRefused），在 7343f0c 上能编译。
// 种子是否落进了预期的类别，用 Reason 文本（冻结不改）在跑命令之前核一遍
// （x39fRequireKinds），免得种子落错类让断言空转；这只是测试自己的前提
// 检查，不是生产代码的分类方式。
//
// 本文件是 repl 包里 x39f 行帮手的唯一定义处，x39f_configreload_windows_test.go
// 复用它们。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/turn"
)

// ------------------------------------------------------------
// 冻结字面量（契约第 3 节，%d/%s/%v 代入后去掉结尾换行）
// ------------------------------------------------------------

func x39fR1(n int) string {
	return fmt.Sprintf("身份已变的 provider 会话: %d 条（要重建才能接着用，各付一次入场费）", n)
}

func x39fR2(n int) string {
	return fmt.Sprintf("按当前配置 /ask 会被拒的 provider 会话: %d 条（不花钱）", n)
}

const (
	x39fP1 = "要应用请敲 /rules reload --rebuild（会重建上列会话，各付一次入场费）"
	x39fP3 = "要应用请敲 /rules reload --rebuild（上列会话只会被清掉，不花钱）"
	x39fP4 = "要应用请敲 /rules reload --rebuild"
	x39fC2 = "要应用请敲 /config reload --apply（会换掉 REPL 与 Runner 两处持有的配置）"
)

func x39fP2(n1, n2 int) string {
	return fmt.Sprintf("要应用请敲 /rules reload --rebuild（其中 %d 条会重建，各付一次入场费；另 %d 条只会被清掉，不花钱）", n1, n2)
}

func x39fB1(convID, agentID string, existed bool) string {
	return fmt.Sprintf("已重建 %s/%s（existed=%v）", convID, agentID, existed)
}

func x39fB2(convID, agentID string, existed bool) string {
	return fmt.Sprintf("已清掉 %s/%s（existed=%v；按当前配置 /ask 会被拒，不花钱）", convID, agentID, existed)
}

func x39fC1(n int) string {
	return fmt.Sprintf("要应用请敲 /config reload --apply（会换掉 REPL 与 Runner 两处持有的配置；其中 %d 条身份已变的会话要重建才能接着用，各付一次入场费）", n)
}

func x39fS0(agentID string) string {
	return fmt.Sprintf("%s 不在当前会话的活跃成员里，用 /who 看看有哪些", agentID)
}

func x39fS1(agentID string) string {
	return fmt.Sprintf("已丢弃 %s 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢），下一轮按首轮计价", agentID)
}

func x39fS2(agentID string) string {
	return fmt.Sprintf("已清掉 %s 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢）；按当前配置对它 /ask 会被拒，不花钱", agentID)
}

// ------------------------------------------------------------
// 行帮手
// ------------------------------------------------------------

// x39fLines 把输出按 "\n" 切行，每行反复剥掉开头的 REPL 提示符。
func x39fLines(out string) []string {
	raw := strings.Split(out, "\n")
	lines := make([]string, len(raw))
	for i, l := range raw {
		for strings.HasPrefix(l, prompt) {
			l = strings.TrimPrefix(l, prompt)
		}
		lines[i] = l
	}
	return lines
}

// x39fLineIndex 返回第一行全等于 want 的行号，没有则 -1。
func x39fLineIndex(out, want string) int {
	for i, l := range x39fLines(out) {
		if l == want {
			return i
		}
	}
	return -1
}

// x39fHasLinePrefix 判断是否存在一行以 prefix 开头。
func x39fHasLinePrefix(out, prefix string) bool {
	for _, l := range x39fLines(out) {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

// x39fWantLine 断言存在一行全等于 want。
func x39fWantLine(t *testing.T, out, want string) {
	t.Helper()
	if x39fLineIndex(out, want) < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", want, out)
	}
}

// x39fForbid 断言整段输出不含任何一个禁用词。
func x39fForbid(t *testing.T, out string, words ...string) {
	t.Helper()
	for _, w := range words {
		if strings.Contains(out, w) {
			t.Errorf("输出不该含 %q；完整输出:\n%s", w, out)
		}
	}
}

// x39fForbidLinePrefix 断言没有任何一行以 prefix 开头。
func x39fForbidLinePrefix(t *testing.T, out, prefix string) {
	t.Helper()
	if x39fHasLinePrefix(out, prefix) {
		t.Errorf("不该有以 %q 开头的行；完整输出:\n%s", prefix, out)
	}
}

// x39fWantLinePrefix 断言存在一行以 prefix 开头。
func x39fWantLinePrefix(t *testing.T, out, prefix string) {
	t.Helper()
	if !x39fHasLinePrefix(out, prefix) {
		t.Errorf("应当有一行以 %q 开头；完整输出:\n%s", prefix, out)
	}
}

// ------------------------------------------------------------
// 种子与配置配方
// ------------------------------------------------------------

// x39fBadFP 是任意非编码串：配置里有这个 agent 时落第 5 类。
const x39fBadFP = "坏编码"

// x39fKind6FP 能解、但各字段都与当前配置不同：落第 6 类。
func x39fKind6FP() string {
	return turn.IdentityFingerprint("old role", "old-model", "", provider.PermissionRead, turn.RulesBlocks{}, "old-digest")
}

// x39fSeed 给 (convID, agentID) 直接种一行 provider 会话。
func x39fSeed(f *fakeStore, convID, agentID, fp string) {
	f.seedProviderSession(conversation.ProviderSession{
		ConvID:            convID,
		AgentID:           agentID,
		ProviderSessionID: "x39f-sess-" + agentID,
		IdentityFP:        fp,
	})
}

// x39fHasRow 查 fakeStore 里 (convID, agentID) 那一行在不在。
func x39fHasRow(t *testing.T, f *fakeStore, convID, agentID string) bool {
	t.Helper()
	_, ok, err := f.ProviderSession(context.Background(), convID, agentID)
	if err != nil {
		t.Fatalf("读 provider 会话 (%s, %s) 失败: %v", convID, agentID, err)
	}
	return ok
}

// x39fCfgProviderBroken：ag_grok 的 provider 名改坏（第 2 类被拒）。
func x39fCfgProviderBroken() *config.Config {
	cfg := rulesTestConfig()
	ac := cfg.Agents["ag_grok"]
	ac.Provider = "no-such-provider-xyz"
	cfg.Agents["ag_grok"] = ac
	return cfg
}

// x39fCfgRefusedMix：ag_claude 权限 full（第 3 类）、ag_agy 的 Role 超大
// （第 4 类）；ag_grok 保持原样（给它种第 6 类指纹）。
func x39fCfgRefusedMix() *config.Config {
	cfg := rulesTestConfig()
	ac := cfg.Agents["ag_claude"]
	ac.Permissions = config.PermissionFull
	cfg.Agents["ag_claude"] = ac
	ag := cfg.Agents["ag_agy"]
	ag.Role = strings.Repeat("R", 40000)
	cfg.Agents["ag_agy"] = ag
	return cfg
}

// x39fEnv 是一条用例的前置：Service、fakeStore、Runner，REPL 与 Runner
// 用同一个 cfg 指针；已种好会话 conv_a。
type x39fEnv struct {
	cfg *config.Config
	svc *conversation.Service
	f   *fakeStore
	rn  *turn.Runner
}

func x39fNewEnv(t *testing.T, cfg *config.Config) *x39fEnv {
	t.Helper()
	isolateGlobalRules(t)
	svc, f := newTestService(t)
	rn := turn.NewRunner(cfg, svc, t.TempDir(), turn.RulesBlocks{})
	f.seedConversation("conv_a", "x39f 用例")
	return &x39fEnv{cfg: cfg, svc: svc, f: f, rn: rn}
}

func (e *x39fEnv) run(t *testing.T, rulesCtx RulesContext, input string) string {
	t.Helper()
	out, err := runREPLWithRules(t, e.cfg, e.svc, e.rn, rulesCtx, input)
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}
	return out
}

// x39fReasonClass 按 Reason 文本（冻结不改）推出类别，只给前提检查用。
func x39fReasonClass(reason string) int {
	switch {
	case reason == "配置里已无此 agent":
		return 1
	case strings.HasPrefix(reason, "provider 不可用，算不出指纹: "):
		return 2
	case strings.HasPrefix(reason, "权限档不放行，算不出指纹: "):
		return 3
	case strings.HasPrefix(reason, "算不出指纹: "):
		return 4
	case reason == "指纹格式不可解":
		return 5
	case reason != "":
		return 6
	default:
		return 0
	}
}

// x39fRequireKinds 在跑命令之前核一遍：AuditDrift 恰好报出 want 里列的
// 那些 (convID/agentID)，各自落在期望的类别里。
func x39fRequireKinds(t *testing.T, rn *turn.Runner, want map[string]int) {
	t.Helper()
	rows, err := rn.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("前提检查：AuditDrift 失败: %v", err)
	}
	if len(rows) != len(want) {
		t.Fatalf("前提检查：期望 %d 行漂移，实际 %d 行: %+v", len(want), len(rows), rows)
	}
	for _, row := range rows {
		key := row.ConvID + "/" + row.AgentID
		k, ok := want[key]
		if !ok {
			t.Fatalf("前提检查：多出一行 %s: %q", key, row.Reason)
		}
		if got := x39fReasonClass(row.Reason); got != k {
			t.Fatalf("前提检查：%s 应落第 %d 类，实际第 %d 类（Reason=%q）", key, k, got, row.Reason)
		}
	}
}

// ------------------------------------------------------------
// 注入存储错误（S-9、B-3）
// ------------------------------------------------------------

var x39fErrInjected = errors.New("x39f 注入的存储错误")

// x39fAuditErrStore 只让 AllProviderSessions 报错（AuditDrift 失败）。
type x39fAuditErrStore struct{ *fakeStore }

func (x39fAuditErrStore) AllProviderSessions(ctx context.Context) ([]conversation.ProviderSession, error) {
	return nil, x39fErrInjected
}

// x39fWriteErrStore 只让 WriteTx 报错（Rebuild 里的 DropProviderSession 失败）。
type x39fWriteErrStore struct{ *fakeStore }

func (x39fWriteErrStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return x39fErrInjected
}

var (
	_ conversation.Store = x39fAuditErrStore{}
	_ conversation.Store = x39fWriteErrStore{}
)

// x39fNewEnvWithStore 与 x39fNewEnv 相同，只是 Service 包在 wrap(f) 上。
func x39fNewEnvWithStore(t *testing.T, cfg *config.Config, wrap func(*fakeStore) conversation.Store) *x39fEnv {
	t.Helper()
	isolateGlobalRules(t)
	f := newFakeStore()
	svc := conversation.NewService(wrap(f))
	rn := turn.NewRunner(cfg, svc, t.TempDir(), turn.RulesBlocks{})
	f.seedConversation("conv_a", "x39f 用例")
	return &x39fEnv{cfg: cfg, svc: svc, f: f, rn: rn}
}

// x39fLocalRulesCtx 写一个局部规则文件，返回指向它的 RulesContext
// （照 rules_test.go 的 --rebuild 用例）。
func x39fLocalRulesCtx(t *testing.T) RulesContext {
	t.Helper()
	localPath := filepath.Join(t.TempDir(), "COUNCIL.md")
	if err := os.WriteFile(localPath, []byte("x39f 新读到的局部规则"), 0o644); err != nil {
		t.Fatalf("写局部规则文件失败: %v", err)
	}
	return RulesContext{RulesFile: localPath}
}

// ============================================================
// /rules
// ============================================================

// TestX39fRulesOnlyRebuildKind 是 R1：只有第 5 类一行。
func TestX39fRulesOnlyRebuildKind(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 5})

	out := e.run(t, RulesContext{}, "/rules\n")
	x39fWantLine(t, out, x39fR1(1))
	x39fForbid(t, out, "会被拒")
}

// TestX39fRulesOnlyAgentGone 是 R2：只有第 1 类一行。
func TestX39fRulesOnlyAgentGone(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ghost": 1})

	out := e.run(t, RulesContext{}, "/rules\n")
	x39fWantLine(t, out, x39fR2(1))
	x39fForbid(t, out, "入场费", "身份已变")
}

// TestX39fRulesBothKinds 是 R3：两行第 5 类 + 一行第 1 类，R1(2) 在 R2(1) 之前。
func TestX39fRulesBothKinds(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ag_grok", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 5, "conv_a/ag_grok": 5, "conv_a/ghost": 1})

	out := e.run(t, RulesContext{}, "/rules\n")
	i1 := x39fLineIndex(out, x39fR1(2))
	i2 := x39fLineIndex(out, x39fR2(1))
	if i1 < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", x39fR1(2), out)
	}
	if i2 < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", x39fR2(1), out)
	}
	if i1 >= 0 && i2 >= 0 && i1 >= i2 {
		t.Errorf("R1 应当在 R2 之前（R1 第 %d 行，R2 第 %d 行）；完整输出:\n%s", i1, i2, out)
	}
}

// TestX39fRulesOnlyProviderUnavailable 是 R2b：只有第 2 类一行。
func TestX39fRulesOnlyProviderUnavailable(t *testing.T) {
	e := x39fNewEnv(t, x39fCfgProviderBroken())
	x39fSeed(e.f, "conv_a", "ag_grok", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_grok": 2})

	out := e.run(t, RulesContext{}, "/rules\n")
	x39fWantLine(t, out, x39fR2(1))
	x39fForbid(t, out, "入场费", "身份已变")
}

// TestX39fRulesOnlyFieldsChanged 是 R1c：只有第 6 类一行。
func TestX39fRulesOnlyFieldsChanged(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fKind6FP())
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 6})

	out := e.run(t, RulesContext{}, "/rules\n")
	x39fWantLine(t, out, x39fR1(1))
	x39fForbid(t, out, "会被拒")
}

// TestX39fRulesNoRows 是 R4：无行时两句都不印。
func TestX39fRulesNoRows(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fRequireKinds(t, e.rn, map[string]int{})

	out := e.run(t, RulesContext{}, "/rules\n")
	x39fForbid(t, out, "身份已变", "会被拒")
}

// ============================================================
// /rules reload（预览）
// ============================================================

// TestX39fRulesReloadPreviewOnlyRebuild 是 P-1：只有第 5 类，P1 原句。
func TestX39fRulesReloadPreviewOnlyRebuild(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 5})

	out := e.run(t, RulesContext{}, "/rules reload\n")
	x39fWantLine(t, out, x39fP1)
}

// TestX39fRulesReloadPreviewOnlyFieldsChanged 是 P-1c：只有第 6 类，P1 原句。
func TestX39fRulesReloadPreviewOnlyFieldsChanged(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fKind6FP())
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 6})

	out := e.run(t, RulesContext{}, "/rules reload\n")
	x39fWantLine(t, out, x39fP1)
	x39fForbid(t, out, "只会被清掉")
}

// TestX39fRulesReloadPreviewMixed 是 P-2：两行第 5 类 + 一行第 1 类，P2(2, 1)。
func TestX39fRulesReloadPreviewMixed(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ag_grok", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 5, "conv_a/ag_grok": 5, "conv_a/ghost": 1})

	out := e.run(t, RulesContext{}, "/rules reload\n")
	x39fWantLine(t, out, x39fP2(2, 1))
}

// TestX39fRulesReloadPreviewOnlyAgentGone 是 P-3：只有第 1 类，P3。
func TestX39fRulesReloadPreviewOnlyAgentGone(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ghost": 1})

	out := e.run(t, RulesContext{}, "/rules reload\n")
	x39fWantLine(t, out, x39fP3)
	x39fForbid(t, out, "入场费")
}

// TestX39fRulesReloadPreviewOnlyProviderUnavailable 是 P-3b：只有第 2 类，P3。
func TestX39fRulesReloadPreviewOnlyProviderUnavailable(t *testing.T) {
	e := x39fNewEnv(t, x39fCfgProviderBroken())
	x39fSeed(e.f, "conv_a", "ag_grok", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_grok": 2})

	out := e.run(t, RulesContext{}, "/rules reload\n")
	x39fWantLine(t, out, x39fP3)
	x39fForbid(t, out, "入场费")
}

// TestX39fRulesReloadPreviewNoRows 是 P-4：无行，P4（整行，不带括号）。
func TestX39fRulesReloadPreviewNoRows(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fRequireKinds(t, e.rn, map[string]int{})

	out := e.run(t, RulesContext{}, "/rules reload\n")
	x39fWantLine(t, out, x39fP4)
	x39fForbid(t, out, "入场费")
}

// ============================================================
// /rules reload --rebuild
// ============================================================

// TestX39fRulesReloadRebuildMixed 是 B-1：第 5 类走 B1，第 1 类走 B2，两行都删。
func TestX39fRulesReloadRebuildMixed(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 5, "conv_a/ghost": 1})

	out := e.run(t, x39fLocalRulesCtx(t), "/rules reload --rebuild\n")
	x39fWantLine(t, out, x39fB1("conv_a", "ag_claude", true))
	x39fWantLine(t, out, x39fB2("conv_a", "ghost", true))
	x39fForbidLinePrefix(t, out, "已重建 conv_a/ghost")
	if x39fHasRow(t, e.f, "conv_a", "ag_claude") {
		t.Errorf("--rebuild 之后 conv_a/ag_claude 那一行应当已删")
	}
	if x39fHasRow(t, e.f, "conv_a", "ghost") {
		t.Errorf("--rebuild 之后 conv_a/ghost 那一行应当已删（被拒类照删，只改措辞）")
	}
}

// TestX39fRulesReloadRebuildProviderUnavailable 是 B-1b：第 2 类走 B2。
func TestX39fRulesReloadRebuildProviderUnavailable(t *testing.T) {
	e := x39fNewEnv(t, x39fCfgProviderBroken())
	x39fSeed(e.f, "conv_a", "ag_grok", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_grok": 2, "conv_a/ag_claude": 5})

	out := e.run(t, x39fLocalRulesCtx(t), "/rules reload --rebuild\n")
	x39fWantLine(t, out, x39fB2("conv_a", "ag_grok", true))
	x39fWantLine(t, out, x39fB1("conv_a", "ag_claude", true))
	x39fForbidLinePrefix(t, out, "已重建 conv_a/ag_grok")
}

// TestX39fRulesReloadRebuildFieldsChanged 是 B-1c：第 6 类走 B1。
func TestX39fRulesReloadRebuildFieldsChanged(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fKind6FP())
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 6})

	out := e.run(t, x39fLocalRulesCtx(t), "/rules reload --rebuild\n")
	x39fWantLine(t, out, x39fB1("conv_a", "ag_claude", true))
	x39fForbidLinePrefix(t, out, "已清掉 ")
}

// TestX39fRulesReloadRebuildFailurePrefix 是 B-3：WriteTx 恒报错，被拒类
// 清掉失败用「清掉 … 失败」前缀，会重建类仍用「重建 … 失败」。
func TestX39fRulesReloadRebuildFailurePrefix(t *testing.T) {
	e := x39fNewEnvWithStore(t, rulesTestConfig(), func(f *fakeStore) conversation.Store {
		return x39fWriteErrStore{f}
	})
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ghost": 1, "conv_a/ag_claude": 5})

	out := e.run(t, x39fLocalRulesCtx(t), "/rules reload --rebuild\n")
	x39fWantLinePrefix(t, out, "清掉 conv_a/ghost 失败: ")
	x39fForbidLinePrefix(t, out, "重建 conv_a/ghost 失败")
	x39fWantLinePrefix(t, out, "重建 conv_a/ag_claude 失败: ")
	// 附带断言：WriteTx 恒报错时恒真，只证明注入确实生效。
	if !x39fHasRow(t, e.f, "conv_a", "ghost") || !x39fHasRow(t, e.f, "conv_a", "ag_claude") {
		t.Errorf("WriteTx 报错时两行都应当还在")
	}
}

// ============================================================
// /session rebuild
// ============================================================

// TestX39fSessionRebuildNonMemberAgentGoneClears 是 S-1：非成员、第 1 类，
// 放宽的闸放行，打 S2，行已删，之后 AuditDrift 为 0 行。
func TestX39fSessionRebuildNonMemberAgentGoneClears(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ghost": 1})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ghost\n")
	x39fWantLine(t, out, x39fS2("ghost"))
	if x39fHasRow(t, e.f, "conv_a", "ghost") {
		t.Errorf("/session rebuild ghost 之后那一行应当已删")
	}
	rows, err := e.rn.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 失败: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("清掉之后 AuditDrift 应当为 0 行，实际: %+v", rows)
	}
}

// TestX39fSessionRebuildNonMemberNoRow 是 S-2：非成员、库里没有行，S0。
func TestX39fSessionRebuildNonMemberNoRow(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fRequireKinds(t, e.rn, map[string]int{})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild nobody\n")
	x39fWantLine(t, out, x39fS0("nobody"))
	x39fForbid(t, out, "已清掉", "本来就没有")
}

// TestX39fSessionRebuildNonMemberUnparsableRefused 是 S-3：非成员、第 5 类，S0，行还在。
func TestX39fSessionRebuildNonMemberUnparsableRefused(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 5})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ag_claude\n")
	x39fWantLine(t, out, x39fS0("ag_claude"))
	if !x39fHasRow(t, e.f, "conv_a", "ag_claude") {
		t.Errorf("闸不放行时那一行应当还在")
	}
}

// TestX39fSessionRebuildNonMemberOtherKindsRefused 是 S-3c：非成员、第 3/4/6 类，
// 三条命令各打 S0，三行都还在。
func TestX39fSessionRebuildNonMemberOtherKindsRefused(t *testing.T) {
	e := x39fNewEnv(t, x39fCfgRefusedMix())
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ag_agy", x39fBadFP)
	x39fSeed(e.f, "conv_a", "ag_grok", x39fKind6FP())
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 3, "conv_a/ag_agy": 4, "conv_a/ag_grok": 6})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ag_claude\n/session rebuild ag_agy\n/session rebuild ag_grok\n")
	for _, id := range []string{"ag_claude", "ag_agy", "ag_grok"} {
		x39fWantLine(t, out, x39fS0(id))
		if !x39fHasRow(t, e.f, "conv_a", id) {
			t.Errorf("闸不放行时 conv_a/%s 那一行应当还在", id)
		}
	}
}

// TestX39fSessionRebuildNonMemberProviderUnavailableRefused 是 S-3b：
// 非成员、第 2 类（AskRefused 为真但不是第 1 类），S0，行还在。
func TestX39fSessionRebuildNonMemberProviderUnavailableRefused(t *testing.T) {
	e := x39fNewEnv(t, x39fCfgProviderBroken())
	x39fSeed(e.f, "conv_a", "ag_grok", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_grok": 2})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ag_grok\n")
	x39fWantLine(t, out, x39fS0("ag_grok"))
	if !x39fHasRow(t, e.f, "conv_a", "ag_grok") {
		t.Errorf("闸不放行时那一行应当还在")
	}
}

// TestX39fSessionRebuildAgentGoneOtherConvRefused 是 S-4：第 1 类行在 conv_b，
// 当前会话 conv_a，S0，conv_b 那一行还在。
func TestX39fSessionRebuildAgentGoneOtherConvRefused(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	e.f.seedConversation("conv_b", "x39f 另一条会话")
	x39fSeed(e.f, "conv_b", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_b/ghost": 1})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ghost\n")
	x39fWantLine(t, out, x39fS0("ghost"))
	if !x39fHasRow(t, e.f, "conv_b", "ghost") {
		t.Errorf("conv_b/ghost 那一行不该被 conv_a 里的命令删掉")
	}
}

// TestX39fSessionRebuildPrefixNotAccepted 是 S-5：前缀 gho 不放行。
func TestX39fSessionRebuildPrefixNotAccepted(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ghost": 1})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild gho\n")
	x39fWantLine(t, out, x39fS0("gho"))
	if !x39fHasRow(t, e.f, "conv_a", "ghost") {
		t.Errorf("前缀不该放行，ghost 那一行应当还在")
	}
}

// TestX39fSessionRebuildCaseNotFolded 是 S-5b：GHOST 不折叠成 ghost。
func TestX39fSessionRebuildCaseNotFolded(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ghost": 1})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild GHOST\n")
	x39fWantLine(t, out, x39fS0("GHOST"))
	x39fForbid(t, out, "本来就没有")
	if !x39fHasRow(t, e.f, "conv_a", "ghost") {
		t.Errorf("大小写不该折叠，ghost 那一行应当还在")
	}
}

// TestX39fSessionRebuildGoneButMember 是 S-6：已删但仍是活跃成员，S2，行已删。
func TestX39fSessionRebuildGoneButMember(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	e.f.seedParticipant(t, "conv_a", "ghost")
	x39fSeed(e.f, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ghost": 1})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ghost\n")
	x39fWantLine(t, out, x39fS2("ghost"))
	if x39fHasRow(t, e.f, "conv_a", "ghost") {
		t.Errorf("/session rebuild ghost 之后那一行应当已删")
	}
}

// TestX39fSessionRebuildMemberUnparsable 是 S-7：活跃成员、第 5 类，S1。
func TestX39fSessionRebuildMemberUnparsable(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	e.f.seedParticipant(t, "conv_a", "ag_claude")
	x39fSeed(e.f, "conv_a", "ag_claude", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 5})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ag_claude\n")
	x39fWantLine(t, out, x39fS1("ag_claude"))
	x39fForbid(t, out, "已清掉")
}

// TestX39fSessionRebuildMemberFieldsChanged 是 S-7c：活跃成员、第 6 类，S1。
func TestX39fSessionRebuildMemberFieldsChanged(t *testing.T) {
	e := x39fNewEnv(t, rulesTestConfig())
	e.f.seedParticipant(t, "conv_a", "ag_claude")
	x39fSeed(e.f, "conv_a", "ag_claude", x39fKind6FP())
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_claude": 6})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ag_claude\n")
	x39fWantLine(t, out, x39fS1("ag_claude"))
	x39fForbid(t, out, "已清掉")
}

// TestX39fSessionRebuildAuditFailureFailsClosed 是 S-9：AuditDrift 报错时
// 成员与非成员都不调 Rebuild，报「身份漂移审计失败」，行还在。
func TestX39fSessionRebuildAuditFailureFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		agentID string
		member  bool
	}{
		{"a_成员", "ag_claude", true},
		{"b_非成员", "ghost", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := x39fNewEnvWithStore(t, rulesTestConfig(), func(f *fakeStore) conversation.Store {
				return x39fAuditErrStore{f}
			})
			if tc.member {
				e.f.seedParticipant(t, "conv_a", tc.agentID)
			}
			x39fSeed(e.f, "conv_a", tc.agentID, x39fBadFP)
			if _, err := e.rn.AuditDrift(context.Background()); !errors.Is(err, x39fErrInjected) {
				t.Fatalf("前提不成立：AuditDrift 应当报注入的错误，实际 %v", err)
			}

			out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild "+tc.agentID+"\n")
			x39fWantLinePrefix(t, out, "身份漂移审计失败: ")
			x39fForbid(t, out, "已丢弃", "已清掉")
			if !x39fHasRow(t, e.f, "conv_a", tc.agentID) {
				t.Errorf("审计失败时不该调 Rebuild，conv_a/%s 那一行应当还在", tc.agentID)
			}
		})
	}
}

// TestX39fSessionRebuildMemberProviderUnavailable 是 S-8：活跃成员、第 2 类，
// 成功文案看 AskRefused（不是看 Kind==AgentGone），打 S2。
func TestX39fSessionRebuildMemberProviderUnavailable(t *testing.T) {
	e := x39fNewEnv(t, x39fCfgProviderBroken())
	e.f.seedParticipant(t, "conv_a", "ag_grok")
	x39fSeed(e.f, "conv_a", "ag_grok", x39fBadFP)
	x39fRequireKinds(t, e.rn, map[string]int{"conv_a/ag_grok": 2})

	out := e.run(t, RulesContext{}, "/use conv_a\n/session rebuild ag_grok\n")
	x39fWantLine(t, out, x39fS2("ag_grok"))
	if x39fHasRow(t, e.f, "conv_a", "ag_grok") {
		t.Errorf("成员走 found 的路，Rebuild 之后那一行应当已删")
	}
}
