package repl

// x39f_configreload_windows_test.go 覆盖 X-39f 契约 2.4：/config reload 预览
// 提示按后果分 C1 / C2。文件名以 _windows_test.go 结尾，因为要用只在
// configcmd_windows_test.go 里定义的 newConfigcmdFixture。
//
// 不引用新 API，在 7343f0c 上能编译。行帮手与字面量复用
// x39f_driftwording_test.go 里的定义。

import (
	"os"
	"strings"
	"testing"
)

// x39fConfigFixture 起 newConfigcmdFixture 并种好会话 conv_a。
func x39fConfigFixture(t *testing.T) *configcmdFixture {
	t.Helper()
	isolateGlobalRules(t)
	fx := newConfigcmdFixture(t)
	fx.store.seedConversation("conv_a", "x39f config reload 用例")
	return fx
}

// x39fConfigReload 敲一次 /config reload（不带 --apply），返回输出。
func x39fConfigReload(t *testing.T, fx *configcmdFixture) string {
	t.Helper()
	out, err := runREPLWithRules(t, fx.cfg, fx.svc, fx.rn, fx.rulesCtx(), "/config reload\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}
	return out
}

// TestX39fConfigReloadOnlyUnparsable 是 K-1：第 5 类一行，C1(1)。
func TestX39fConfigReloadOnlyUnparsable(t *testing.T) {
	fx := x39fConfigFixture(t)
	x39fSeed(fx.store, "conv_a", "ag_claude", x39fBadFP)
	x39fRequireKinds(t, fx.rn, map[string]int{"conv_a/ag_claude": 5})

	out := x39fConfigReload(t, fx)
	x39fWantLine(t, out, x39fC1(1))
}

// TestX39fConfigReloadOnlyFieldsChanged 是 K-1c：第 6 类一行，C1(1)。
func TestX39fConfigReloadOnlyFieldsChanged(t *testing.T) {
	fx := x39fConfigFixture(t)
	x39fSeed(fx.store, "conv_a", "ag_claude", x39fKind6FP())
	x39fRequireKinds(t, fx.rn, map[string]int{"conv_a/ag_claude": 6})

	out := x39fConfigReload(t, fx)
	x39fWantLine(t, out, x39fC1(1))
}

// TestX39fConfigReloadOnlyAgentGone 是 K-2：只有第 1 类一行，C2。
func TestX39fConfigReloadOnlyAgentGone(t *testing.T) {
	fx := x39fConfigFixture(t)
	x39fSeed(fx.store, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, fx.rn, map[string]int{"conv_a/ghost": 1})

	out := x39fConfigReload(t, fx)
	x39fWantLine(t, out, x39fC2)
	x39fForbid(t, out, "入场费")
}

// TestX39fConfigReloadOnlyPermissionRefused 是 K-2b：预览用的新配置里
// ag_claude 权限档改成 full，那一行落第 3 类（被拒类），C2。
func TestX39fConfigReloadOnlyPermissionRefused(t *testing.T) {
	fx := x39fConfigFixture(t)
	x39fSeed(fx.store, "conv_a", "ag_claude", x39fBadFP)

	body := renderConfigTOML(fx.fields)
	const from, to = `permissions = "read"`, `permissions = "full"`
	if n := strings.Count(body, from); n != 1 {
		t.Fatalf("前提不成立：renderConfigTOML 里 %q 应当恰好出现 1 次，实际 %d 次", from, n)
	}
	if err := os.WriteFile(fx.configPath, []byte(strings.Replace(body, from, to, 1)), 0o644); err != nil {
		t.Fatalf("改写配置文件失败: %v", err)
	}

	out := x39fConfigReload(t, fx)
	// 前提检查：预览名单里那一行确实是第 3 类（printConfigDrift 不改）。
	if !x39fHasLinePrefix(out, "  conv_a / ag_claude: 权限档不放行，算不出指纹: ") {
		t.Fatalf("前提不成立：预览名单里 conv_a/ag_claude 应当落第 3 类；完整输出:\n%s", out)
	}
	x39fWantLine(t, out, x39fC2)
	x39fForbid(t, out, "入场费")
}

// TestX39fConfigReloadNoRows 是 K-3：无行，C2。
func TestX39fConfigReloadNoRows(t *testing.T) {
	fx := x39fConfigFixture(t)
	x39fRequireKinds(t, fx.rn, map[string]int{})

	out := x39fConfigReload(t, fx)
	x39fWantLine(t, out, x39fC2)
	x39fForbid(t, out, "入场费")
}

// TestX39fConfigReloadMixedCountsOnlyRebuild 是 K-4：第 5 类 + 第 1 类，
// C1(1)，计数不含被拒类。
func TestX39fConfigReloadMixedCountsOnlyRebuild(t *testing.T) {
	fx := x39fConfigFixture(t)
	x39fSeed(fx.store, "conv_a", "ag_claude", x39fBadFP)
	x39fSeed(fx.store, "conv_a", "ghost", x39fBadFP)
	x39fRequireKinds(t, fx.rn, map[string]int{"conv_a/ag_claude": 5, "conv_a/ghost": 1})

	out := x39fConfigReload(t, fx)
	x39fWantLine(t, out, x39fC1(1))
}
