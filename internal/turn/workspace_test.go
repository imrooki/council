package turn

// workspace_test.go 覆盖 11c_STEP8B_WORKSPACE_CONTRACT.md 第 8.1 节验收里
// 落在 internal/turn 包的几条：agentWorkDir 两档各一例、READ 那一档路径与
// 本片改动之前逐字节相同（验收第 5 条）、改档位触发漂移且 identityDriftError
// 点名"命令行"（验收第 6 条），外加 RulesReport 按 (provider, tier) 分组
// 这一具体场景（11_STEP8_SPEC.md 第 3.3 节、11c 契约第 6 节）——按 provider
// 分组时会把同一家的第二档悄悄盖掉，这是本片要修的原始 bug，值得单独钉住，
// 不能只靠"零值 fail-closed"那类间接用例去覆盖。

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

// TestAgentWorkDir 覆盖 11c 契约第 4 节与第 8.1 节验收第 5 条。
func TestAgentWorkDir(t *testing.T) {
	const base = `C:\council-data\workdir`

	t.Run("READ 忽略 convID，与本片改动之前逐字节相同", func(t *testing.T) {
		// 本片改动之前，agentWorkDir 这段逻辑就是 runner.go 里内联的
		// filepath.Join(r.workDir, agentID)——这里直接写出那条公式当期望值，
		// 正面断言"既有会话不漂移"（11c 契约第 1 节、第 4 节的核心承诺）。
		want := filepath.Join(base, "alice")
		got := agentWorkDir(base, "conv-a", "alice", provider.PermissionRead)
		if got != want {
			t.Fatalf("agentWorkDir(READ) = %q，期望与本片改动之前的公式逐字节相同 %q", got, want)
		}

		// 换一个 convID，READ 档的路径必须原样不变——这是"只读成员共用同一个
		// 工作目录，省下按会话隔离那次冷启动"的直接断言。
		got2 := agentWorkDir(base, "conv-b", "alice", provider.PermissionRead)
		if got2 != got {
			t.Errorf("READ 档换一个 convID 不该改变工作目录：conv-a 算出 %q，conv-b 算出 %q", got, got2)
		}
	})

	t.Run("WORKSPACE 按 convID 隔离，两条会话互不可见", func(t *testing.T) {
		gotA := agentWorkDir(base, "conv-a", "alice", provider.PermissionWorkspace)
		gotB := agentWorkDir(base, "conv-b", "alice", provider.PermissionWorkspace)
		if gotA == gotB {
			t.Fatalf("同一个 agent 的两条不同会话，WORKSPACE 工作目录不该相同，都是 %q", gotA)
		}
		wantA := filepath.Join(base, "conv-a", "alice")
		if gotA != wantA {
			t.Errorf("agentWorkDir(WORKSPACE, conv-a) = %q，期望 %q", gotA, wantA)
		}
	})
}

// TestIdentityDriftErrorNamesCommandLineOnTierChange 覆盖 11c 契约第 5 节
// 与第 8.1 节验收第 6 条：同一个 agent 从 read 改成 workspace，工作目录
// 真的变了（agentWorkDir），指纹必须不同，且 identityDriftError 点名的
// 字段里要有"命令行"。
//
// 这里不断言"只有命令行变了"：permission 那一段（fpFieldTier）同样会变，
// 这是符合预期的两处同时变化，不是本用例要"修"的误报——11c 契约本身没有
// 承诺换档位时只有一个字段会变，只承诺"命令行"必须在点名列表里，见
// 任务报告里对这条验收的措辞说明。
func TestIdentityDriftErrorNamesCommandLineOnTierChange(t *testing.T) {
	p, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("provider.Lookup(claude) 失败: %v", err)
	}
	ac := config.AgentConfig{
		DisplayName: "Alice",
		Provider:    "claude",
		Model:       "claude-fable-5-1",
		Effort:      "high",
		Role:        "只做结构力学审阅",
		Permissions: config.PermissionRead,
	}
	baseWorkDir := t.TempDir()
	const convID = "conv-tier-change"

	oldFP, err := identityFPFor(p, "alice", ac, baseWorkDir, convID, provider.PermissionRead, RulesBlocks{})
	if err != nil {
		t.Fatalf("identityFPFor(read) 失败: %v", err)
	}
	newFP, err := identityFPFor(p, "alice", ac, baseWorkDir, convID, provider.PermissionWorkspace, RulesBlocks{})
	if err != nil {
		t.Fatalf("identityFPFor(workspace) 失败: %v", err)
	}
	if oldFP == newFP {
		t.Fatal("从 read 改成 workspace，指纹必须不同——工作目录真的变了（第 1 节实测），这不是该修的误报")
	}

	driftErr := identityDriftError(oldFP, newFP)
	if !errors.Is(driftErr, ErrIdentityDrift) {
		t.Fatalf("identityDriftError 的返回值应当能被 errors.Is(err, ErrIdentityDrift) 认出，实际: %v", driftErr)
	}
	if !strings.Contains(driftErr.Error(), "命令行") {
		t.Errorf("identityDriftError 应当点名「命令行」，实际: %v", driftErr)
	}
}

// TestRulesReportGroupsByProviderAndTier 覆盖 11_STEP8_SPEC.md 第 3.3 节与
// 11c 契约第 6 节要修的原始 bug：同一家 provider 下一个 agent 是 read、
// 另一个是 workspace 时，两档必须各占一行，不能按 provider id 字典序只取
// 一个代表把另一档盖掉。grok 是这里最尖锐的例子：它拒绝 workspace，若字典序
// 撞上的话旧实现会把 grok 本来好好的 read 行也印成"不适用"。
func TestRulesReportGroupsByProviderAndTier(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: "claude-fake"},
			"grok":   {Command: "grok-fake"},
		},
		Agents: map[string]config.AgentConfig{
			"c_read": {DisplayName: "CRead", Provider: "claude", Model: "claude-fable-5-1", Permissions: config.PermissionRead},
			"c_ws":   {DisplayName: "CWorkspace", Provider: "claude", Model: "claude-fable-5-1", Permissions: config.PermissionWorkspace},
			"g_read": {DisplayName: "GRead", Provider: "grok", Model: "grok-4.6", Permissions: config.PermissionRead},
			"g_ws":   {DisplayName: "GWorkspace", Provider: "grok", Model: "grok-4.6", Permissions: config.PermissionWorkspace},
		},
	}
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	rows := r.RulesReport()
	if len(rows) != 4 {
		t.Fatalf("期望 4 行（claude 两档 + grok 两档），实际 %d 行: %+v", len(rows), rows)
	}

	idx := make(map[string]ProviderRulesInfo, len(rows))
	for _, row := range rows {
		idx[row.ProviderID+"/"+string(row.Tier)] = row
	}

	claudeRead, ok := idx["claude/read"]
	if !ok {
		t.Fatal("缺 claude/read 那一行")
	}
	claudeWS, ok := idx["claude/workspace"]
	if !ok {
		t.Fatal("缺 claude/workspace 那一行")
	}
	if claudeRead.HeadroomNote != "" || claudeWS.HeadroomNote != "" {
		t.Errorf("claude 两档都放行 workspace，探针都应当组得出命令行: read=%+v workspace=%+v", claudeRead, claudeWS)
	}

	grokRead, ok := idx["grok/read"]
	if !ok {
		t.Fatal("缺 grok/read 那一行")
	}
	grokWS, ok := idx["grok/workspace"]
	if !ok {
		t.Fatal("缺 grok/workspace 那一行")
	}
	if grokRead.HeadroomNote != "" {
		t.Errorf("grok 的 read 那一行不该受 workspace 那一行组不出探针命令行的影响，实际: %+v", grokRead)
	}
	if grokWS.HeadroomNote == "" {
		t.Error("grok 证明不了写入边界，workspace 那一行的探针应当组不出命令行，HeadroomNote 应当非空")
	}
}
