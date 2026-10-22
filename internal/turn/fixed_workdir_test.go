package turn

// 片 2 的固定工作目录断言：read 与 workspace 两档在固定模式下拿到同一个
// 副本根；非固定模式仍走 agentWorkDir 的原公式。只测纯路径、指纹与角色
// 目录，不起进程、不碰库。

import (
	"path/filepath"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

func fixedWorkDirTestConfig() *config.Config {
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: "claude"},
		},
		Agents: map[string]config.AgentConfig{
			"alice": {
				DisplayName: "Alice",
				Provider:    "claude",
				Model:       "claude-fable-5-1",
				Effort:      "high",
				Role:        "只读审阅",
				Permissions: config.PermissionRead,
			},
			"bob": {
				DisplayName: "Bob",
				Provider:    "claude",
				Model:       "claude-fable-5-1",
				Effort:      "high",
				Role:        "可以改文件",
				Permissions: config.PermissionWorkspace,
			},
		},
	}
}

func TestFixedWorkDirReadAndWorkspaceShareCopyRoot(t *testing.T) {
	dataDir := t.TempDir()
	copyDir := filepath.Join(dataDir, "workspaces", "conv-fixed")
	workBase := filepath.Join(dataDir, "workdir")
	cfg := fixedWorkDirTestConfig()

	r, err := NewRunnerFixedWorkDir(cfg, nil, copyDir, workBase, RulesBlocks{})
	if err != nil {
		t.Fatalf("NewRunnerFixedWorkDir 失败: %v", err)
	}
	for _, tc := range []struct {
		name  string
		agent string
		tier  provider.PermissionTier
	}{
		{"read", "alice", provider.PermissionRead},
		{"workspace", "bob", provider.PermissionWorkspace},
	} {
		if got := r.dirFor("conv-fixed", tc.agent, tc.tier); got != copyDir {
			t.Errorf("固定模式 %s 档的 cwd 应为副本根 %q，实际 %q", tc.name, copyDir, got)
		}
	}

	// 非固定模式必须逐字节保持既有公式：READ 忽略 convID，
	// WORKSPACE 把 convID 拼进去。
	normal := NewRunner(cfg, nil, workBase, RulesBlocks{})
	if got, want := normal.dirFor("conv-a", "alice", provider.PermissionRead), filepath.Join(workBase, "alice"); got != want {
		t.Errorf("非固定 READ 的 cwd 应为 %q，实际 %q", want, got)
	}
	if got, want := normal.dirFor("conv-a", "bob", provider.PermissionWorkspace), filepath.Join(workBase, "conv-a", "bob"); got != want {
		t.Errorf("非固定 WORKSPACE 的 cwd 应为 %q，实际 %q", want, got)
	}
}

func TestFixedWorkDirRoleFileStaysOutsideCopy(t *testing.T) {
	dataDir := t.TempDir()
	copyDir := filepath.Join(dataDir, "workspaces", "conv-fixed")
	workBase := filepath.Join(dataDir, "workdir")
	r, err := NewRunnerFixedWorkDir(fixedWorkDirTestConfig(), nil, copyDir, workBase, RulesBlocks{})
	if err != nil {
		t.Fatalf("NewRunnerFixedWorkDir 失败: %v", err)
	}

	want := filepath.Join(workBase, "alice")
	if got := r.roleDirFor("alice"); got != want {
		t.Errorf("固定模式的角色文件目录应为 %q，实际 %q", want, got)
	}
	if got := r.workDir; got != workBase {
		t.Errorf("固定模式的 workDir 应为 %q，实际 %q", workBase, got)
	}
	if got := NewRunner(fixedWorkDirTestConfig(), nil, workBase, RulesBlocks{}).roleDirFor("alice"); got != "" {
		t.Errorf("非固定模式的角色文件目录应为空，实际 %q", got)
	}
}

// TestFixedWorkDirIdentityFingerprintUsesResolvedDir 覆盖任务书 §5.4 点名的
// identityFPAt：workspace 档在固定副本根与普通 agentWorkDir 下的角色文件
// 路径不同，指纹必须不同，证明身份块里用的确实是传进去的那对 cwd/roleDir；
// read 与 workspace 两档也都能在同一个固定目录上算出指纹。
func TestFixedWorkDirIdentityFingerprintUsesResolvedDir(t *testing.T) {
	dataDir := t.TempDir()
	copyDir := filepath.Join(dataDir, "workspaces", "conv-fixed")
	workBase := filepath.Join(dataDir, "workdir")
	cfg := fixedWorkDirTestConfig()
	p, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("provider.Lookup(claude) 失败: %v", err)
	}

	// workspace 档：普通模式的角色文件在 <workBase>\<conv>\<agent>，
	// 固定模式在 <workBase>\<agent>；路径进 claude argv，指纹必须不同。
	wsAC := cfg.Agents["bob"]
	fixedFP, err := identityFPAt(p, "bob", wsAC, copyDir, filepath.Join(workBase, "bob"), provider.PermissionWorkspace, RulesBlocks{})
	if err != nil {
		t.Fatalf("identityFPAt(固定目录) 失败: %v", err)
	}
	normalDir := agentWorkDir(workBase, "conv-fixed", "bob", provider.PermissionWorkspace)
	normalFP, err := identityFPAt(p, "bob", wsAC, normalDir, "", provider.PermissionWorkspace, RulesBlocks{})
	if err != nil {
		t.Fatalf("identityFPAt(普通目录) 失败: %v", err)
	}
	if fixedFP == normalFP {
		t.Errorf("固定副本 cwd %q 与普通 agentWorkDir %q 的指纹不应相同", copyDir, normalDir)
	}

	readFP, err := identityFPAt(p, "alice", cfg.Agents["alice"], copyDir, filepath.Join(workBase, "alice"), provider.PermissionRead, RulesBlocks{})
	if err != nil {
		t.Fatalf("identityFPAt(read 档) 失败: %v", err)
	}
	if readFP == "" || fixedFP == "" {
		t.Errorf("固定模式下 read/workspace 的指纹都不应为空")
	}
}

func TestFixedWorkDirRejectsNonAbsolutePaths(t *testing.T) {
	cfg := fixedWorkDirTestConfig()
	for _, tc := range []struct {
		name     string
		dir      string
		workBase string
	}{
		{"空 dir", "", filepath.Join(t.TempDir(), "workdir")},
		{"相对 dir", "rel/x", filepath.Join(t.TempDir(), "workdir")},
		{"空 workBase", filepath.Join(t.TempDir(), "workspaces", "conv"), ""},
		{"相对 workBase", filepath.Join(t.TempDir(), "workspaces", "conv"), "workdir"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewRunnerFixedWorkDir(cfg, nil, tc.dir, tc.workBase, RulesBlocks{}); err == nil {
				t.Fatalf("NewRunnerFixedWorkDir(%q, %q) 应当返回错误", tc.dir, tc.workBase)
			}
		})
	}
}
