package config

// s5_provider_args_test.go 钉住 S5：provider 级 args 会追加在适配器自己的
// 权限 flag 之后，等于让任何成员（包括 READ 档）静默越过自己的权限档。
// 规则不按「这个 flag 看起来危险」猜，而是按每一家 CLI 实际能改权限的
// flag 列黑名单；未知的普通参数继续放行。

import (
	"strings"
	"testing"
)

func s5Config(t *testing.T, provider string, args []string) *Config {
	t.Helper()
	cfg := Default()
	cfg.DataDir = t.TempDir()
	cfg.Providers = map[string]ProviderConfig{
		provider: {Command: provider, Args: args},
	}
	cfg.Agents = map[string]AgentConfig{
		"a": {
			DisplayName: "A",
			Provider:    provider,
			Model:       "model",
			Permissions: PermissionRead,
			Mode:        ModeDiscussion,
		},
	}
	// Default() 自带一组示例团队，成员指向它自带的 agent；这里整份换掉，
	// 免得多出与 S5 无关的校验错误。
	cfg.Teams = map[string]TeamConfig{}
	cfg.Summary = SummaryConfig{}
	return cfg
}

func TestS5ProviderArgsRejectPermissionEscalation(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		args     []string
		want     []string
	}{
		{"claude 跳过权限", "claude", []string{"--dangerously-skip-permissions"}, []string{"providers.claude.args[0]", "permission"}},
		{"claude 改权限模式", "claude", []string{"--permission-mode=bypassPermissions"}, []string{"providers.claude.args[0]", "permission"}},
		{"claude 扩目录", "claude", []string{"--add-dir", "/tmp"}, []string{"providers.claude.args[0]", "permission"}},
		{"codex 全权沙箱", "codex", []string{"-c", "sandbox_mode=danger-full-access"}, []string{"providers.codex.args[0]", "permission"}},
		{"codex 绕过审批", "codex", []string{"--dangerously-bypass-approvals-and-sandbox"}, []string{"providers.codex.args[0]", "permission"}},
		{"grok 改权限模式", "grok", []string{"--permission-mode", "bypassPermissions"}, []string{"providers.grok.args[0]", "permission"}},
		{"agy 跳过权限", "agy", []string{"--dangerously-skip-permissions"}, []string{"providers.agy.args[0]", "permission"}},
		{"agy 改模式", "agy", []string{"--mode", "accept-edits"}, []string{"providers.agy.args[0]", "permission"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s5Config(t, tc.provider, tc.args).Validate()
			if err == nil {
				t.Fatalf("%v 应当被拒绝", tc.args)
			}
			msg := err.Error()
			for _, want := range tc.want {
				if !strings.Contains(msg, want) {
					t.Errorf("错误里缺少 %q，实际:\n%s", want, msg)
				}
			}
		})
	}
}

func TestS5ProviderArgsAllowOrdinaryFlags(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		args     []string
	}{
		{"claude 普通参数", "claude", []string{"--verbose", "--fallback-model", "sonnet"}},
		{"codex 普通 -c", "codex", []string{"-c", "model_reasoning_effort=high", "--skip-git-repo-check"}},
		{"grok 普通参数", "grok", []string{"--deny", "Write(*)", "--verbose"}},
		{"agy 普通参数", "agy", []string{"--verbose"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := s5Config(t, tc.provider, tc.args).Validate(); err != nil {
				t.Fatalf("普通参数 %v 不该被拒绝: %v", tc.args, err)
			}
		})
	}
}
