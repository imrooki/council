// 本文件将来落到 internal/turn/render_test.go（与既有的 render_test.go 合并，
// 或作为独立文件并存——由实现方决定；本文件只新增 11a 契约第 3.1 节的用例，
// 不重复既有 RenderDelta 的用例）。
//
// 覆盖 11a_STEP8A_API_CONTRACT.md 3.1 节 RulesBlocks 与 renderIdentity（加参）：
// 验收表第 1 条"空规则逐字节不变"是全篇最关键的一条断言。只照契约写，
// 不看 internal/turn/render.go 的实现——那是本片改动、任务书明令不读的文件。
// golden 字符串按契约 3.1 给出的确切拼接规则手工推出，不是抄自任何已有测试
// 或已有实现的输出。
package turn

import (
	"strings"
	"testing"

	"council/internal/config"
)

// tstIdentityAgentID / tstIdentityAgentConfig 是本文件 renderIdentity 用例
// 共用的固定输入。DisplayName 与 Role 都故意非空：契约没有定义 DisplayName
// 为空串时 name 应当取什么值（这是一处契约缺口，见报告），本文件的用例
// 一律绕开这个未定义地带，不去猜一个行为再钉成断言。
const tstIdentityAgentID = "alice"

func tstIdentityAgentConfig(role string) config.AgentConfig {
	return config.AgentConfig{
		DisplayName: "Alice",
		Provider:    "claude",
		Role:        role,
	}
}

// ---------------------------------------------------------------------------
// 验收表第 1 条：空规则时逐字节不变
// ---------------------------------------------------------------------------

// TestRenderIdentityEmptyRulesByteIdenticalToPreContractOutput 是本片改动
// 最关键的一条断言（契约 3.1"硬性要求"与验收表第 1 条）：rules 两段都为空时，
// renderIdentity 的输出必须与本片改动之前逐字节相同。
//
// golden 按契约给出的确切拼接规则手工推出：
//
//	"You are " + name + " (id=" + agentID + ").\n"
//	"Provider: " + ac.Provider
//	若 ac.Role != ""： "\n" + ac.Role
//
// 两段规则都为空串时，"若 rules.Global != ...""若 rules.Local != ..." 两个
// 分支都不生效，输出恰好只剩"今天已有的那段"。
func TestRenderIdentityEmptyRulesByteIdenticalToPreContractOutput(t *testing.T) {
	ac := tstIdentityAgentConfig("只做结构力学审阅")
	want := "You are Alice (id=alice).\nProvider: claude\n只做结构力学审阅"

	got := renderIdentity(tstIdentityAgentID, ac, RulesBlocks{})
	if got != want {
		t.Fatalf("renderIdentity(空规则) = %q\n期望（golden，按契约 3.1 手工推出）: %q", got, want)
	}
}

// TestRenderIdentityEmptyRulesEmptyRoleByteIdentical 覆盖 Role 为空串的分支：
// "若 ac.Role != "": "\n" + ac.Role"——Role 为空串时这一整段都不出现，
// 输出在 "Provider: claude" 处截止，没有多余的换行。
func TestRenderIdentityEmptyRulesEmptyRoleByteIdentical(t *testing.T) {
	ac := tstIdentityAgentConfig("")
	want := "You are Alice (id=alice).\nProvider: claude"

	got := renderIdentity(tstIdentityAgentID, ac, RulesBlocks{})
	if got != want {
		t.Fatalf("renderIdentity(空规则, 空 Role) = %q\n期望: %q", got, want)
	}
}

// TestRenderIdentityIsPureFunction 覆盖契约 3.1 末句："renderIdentity 仍然是
// 纯函数：不含时钟、不含会话 id、不含轮次。"同样的输入两次调用必须逐字节相同。
func TestRenderIdentityIsPureFunction(t *testing.T) {
	ac := tstIdentityAgentConfig("只做结构力学审阅")
	rules := RulesBlocks{Global: "全局第一条", Local: "局部第一条"}

	a := renderIdentity(tstIdentityAgentID, ac, rules)
	b := renderIdentity(tstIdentityAgentID, ac, rules)
	if a != b {
		t.Fatalf("同样的输入两次调用不相同:\n第一次: %q\n第二次: %q", a, b)
	}
}

// ---------------------------------------------------------------------------
// RulesBlocks 的拼接：共享在前、专有在后（契约 3.1 与 11_STEP8_SPEC.md 1.3）
// ---------------------------------------------------------------------------

// TestRenderIdentityGlobalOnly 覆盖只有全局规则时的确切拼接：
// "# Council rules (global)\n" + rules.Global + "\n\n" + 今天已有的那段。
func TestRenderIdentityGlobalOnly(t *testing.T) {
	ac := tstIdentityAgentConfig("只做结构力学审阅")
	rules := RulesBlocks{Global: "全局规则正文"}

	want := "# Council rules (global)\n" + "全局规则正文" + "\n\n" +
		"You are Alice (id=alice).\nProvider: claude\n只做结构力学审阅"

	got := renderIdentity(tstIdentityAgentID, ac, rules)
	if got != want {
		t.Fatalf("renderIdentity(仅全局规则) = %q\n期望: %q", got, want)
	}
}

// TestRenderIdentityLocalOnly 覆盖只有局部规则时的确切拼接：
// "# Council rules (project)\n" + rules.Local + "\n\n" + 今天已有的那段。
func TestRenderIdentityLocalOnly(t *testing.T) {
	ac := tstIdentityAgentConfig("只做结构力学审阅")
	rules := RulesBlocks{Local: "局部规则正文"}

	want := "# Council rules (project)\n" + "局部规则正文" + "\n\n" +
		"You are Alice (id=alice).\nProvider: claude\n只做结构力学审阅"

	got := renderIdentity(tstIdentityAgentID, ac, rules)
	if got != want {
		t.Fatalf("renderIdentity(仅局部规则) = %q\n期望: %q", got, want)
	}
}

// TestRenderIdentityGlobalThenLocalThenIdentity 覆盖两段规则同时非空时的顺序：
// 全局在前、局部在后、agent 自己的身份块最后——未决 #1 已测缓存按内容命中，
// 共享前缀越长越省，这个顺序本身零成本（契约 3.1"顺序为什么是共享在前、
// 专有在后"）。
func TestRenderIdentityGlobalThenLocalThenIdentity(t *testing.T) {
	ac := tstIdentityAgentConfig("只做结构力学审阅")
	rules := RulesBlocks{Global: "全局规则正文", Local: "局部规则正文"}

	want := "# Council rules (global)\n" + "全局规则正文" + "\n\n" +
		"# Council rules (project)\n" + "局部规则正文" + "\n\n" +
		"You are Alice (id=alice).\nProvider: claude\n只做结构力学审阅"

	got := renderIdentity(tstIdentityAgentID, ac, rules)
	if got != want {
		t.Fatalf("renderIdentity(全局+局部) = %q\n期望: %q", got, want)
	}
}

// TestRenderIdentityRulesGlobalEmptyLocalNonEmpty 覆盖分支独立生效：
// Global 为空、Local 非空时，"# Council rules (global)" 那一段必须完全不出现，
// 不能靠 Global 恰好是空串就退化成打印一个空标题。
func TestRenderIdentityRulesGlobalEmptyLocalNonEmpty(t *testing.T) {
	ac := tstIdentityAgentConfig("")
	rules := RulesBlocks{Global: "", Local: "只有局部"}

	got := renderIdentity(tstIdentityAgentID, ac, rules)
	if want := "# Council rules (global)"; strings.Contains(got, want) {
		t.Errorf("Global 为空串时不该出现 %q，实际: %q", want, got)
	}
	want := "# Council rules (project)\n只有局部\n\nYou are Alice (id=alice).\nProvider: claude"
	if got != want {
		t.Fatalf("renderIdentity(Global 空, Local 非空) = %q\n期望: %q", got, want)
	}
}
