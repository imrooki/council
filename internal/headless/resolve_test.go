package headless

// 本文件是包内测试，直接测 Parse 的纯函数下半段：成员展开、顺序与去重。
// 这些行为不出现在 JSON 里，但 D2 的优先级就落在这里，从外部包只能绕着
// app.Run 看，测不出边界。

import (
	"reflect"
	"testing"

	"council/internal/config"
)

func resolveCfg() *config.Config {
	agent := func(mode config.Mode) config.AgentConfig {
		return config.AgentConfig{
			DisplayName: "X",
			Provider:    "claude",
			Model:       "opus",
			Permissions: config.PermissionRead,
			Mode:        mode,
		}
	}
	return &config.Config{
		Agents: map[string]config.AgentConfig{
			"gamma": agent(config.ModeDiscussion),
			"alpha": agent(config.ModeDiscussion),
			"beta":  agent(config.ModeJudge),
		},
		Teams: map[string]config.TeamConfig{
			"t": {Members: []string{"gamma", "alpha"}},
		},
	}
}

// TestResolveMembers_去重保留第一次 钉「重复 id 只留第一次出现」。
func TestResolveMembers_去重保留第一次(t *testing.T) {
	got, err := resolveMembers(Options{Members: []string{"beta", "gamma", "beta", "alpha"}}, resolveCfg())
	if err != nil {
		t.Fatalf("resolveMembers 失败: %v", err)
	}
	want := []string{"beta", "gamma", "alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("成员顺序应为 %v，实际 %v", want, got)
	}
}

// TestResolveMembers_团队顺序 钉 team 的 members 按配置顺序保留，不排序。
func TestResolveMembers_团队顺序(t *testing.T) {
	got, err := resolveMembers(Options{Team: "t"}, resolveCfg())
	if err != nil {
		t.Fatalf("resolveMembers 失败: %v", err)
	}
	want := []string{"gamma", "alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("团队顺序应为 %v，实际 %v", want, got)
	}
}

// TestResolveMembers_默认全agent字典序 钉 D2 的最后一档：没给名单也没给
// team 时，取配置里全部 agent id 的字典序。
func TestResolveMembers_默认全agent字典序(t *testing.T) {
	got, err := resolveMembers(Options{}, resolveCfg())
	if err != nil {
		t.Fatalf("resolveMembers 失败: %v", err)
	}
	want := []string{"alpha", "beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("默认名单应为字典序 %v，实际 %v", want, got)
	}
}

// TestResolveSummary_默认与none 钉 D4 的取值优先级。
func TestResolveSummary_默认与none(t *testing.T) {
	cfg := resolveCfg()
	cfg.Summary = config.SummaryConfig{Agent: "alpha"}

	if got, err := resolveSummary(Options{Mode: "ask"}, cfg); err != nil || got != "alpha" {
		t.Errorf("未给 -summary 时应取配置的 alpha，实际 %q / %v", got, err)
	}
	if got, err := resolveSummary(Options{Mode: "ask", Summary: "none"}, cfg); err != nil || got != "" {
		t.Errorf("-summary none 应关掉汇总，实际 %q / %v", got, err)
	}
	if _, err := resolveSummary(Options{Mode: "round", Summary: "none"}, cfg); err == nil {
		t.Errorf("round + -summary none 应当报用法错误")
	}
}

// TestResolveMembers_未知成员 钉 A2 在纯函数层的错误。
func TestResolveMembers_未知成员(t *testing.T) {
	if _, err := resolveMembers(Options{Members: []string{"ghost"}}, resolveCfg()); err == nil {
		t.Fatalf("未知成员应当返回错误")
	}
}
