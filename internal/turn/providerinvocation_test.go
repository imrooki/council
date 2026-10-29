package turn

// providerinvocation_test.go 钉住 [providers.<id>] 的 args / env 真的会落到
// 进程上，并且进身份指纹 —— 只写进配置文件、spawn 时不用，是最难发现的
// 那种「配置里看着对、行为不对」。

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

func TestApplyProviderInvocationAppendsArgsAndMergesEnv(t *testing.T) {
	cmd := provider.Command{Path: "cli", Args: []string{"--adapter-flag"}}
	applyProviderInvocation(&cmd, config.ProviderConfig{
		Args: []string{"--user-flag", "value"},
		Env:  []string{"COUNCIL_TEST_PROVIDER_ENV=1"},
	})
	wantArgs := []string{"--adapter-flag", "--user-flag", "value"}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("Args = %v，期望 %v", cmd.Args, wantArgs)
	}
	for i := range wantArgs {
		if cmd.Args[i] != wantArgs[i] {
			t.Fatalf("Args = %v，期望 %v", cmd.Args, wantArgs)
		}
	}
	// Env 是完整环境：必须包含 PATH，再带覆盖项，且覆盖项只出现一次。
	var sawPath, sawOverride int
	for _, entry := range cmd.Env {
		if envKey(entry) == envKey("PATH=") || strings.HasPrefix(envKey(entry), "PATH=") {
			sawPath++
		}
		if strings.HasPrefix(entry, "COUNCIL_TEST_PROVIDER_ENV=") {
			sawOverride++
		}
	}
	if sawPath != 1 {
		t.Errorf("PATH 出现了 %d 次，期望 1 次（覆盖语义不去重会出问题）", sawPath)
	}
	if sawOverride != 1 {
		t.Errorf("覆盖项出现了 %d 次，期望 1 次", sawOverride)
	}
}

func TestApplyProviderInvocationNoopWhenEmpty(t *testing.T) {
	cmd := provider.Command{Path: "cli", Args: []string{"--a"}}
	before := append([]string(nil), cmd.Args...)
	applyProviderInvocation(&cmd, config.ProviderConfig{Command: "cli"})
	if strings.Join(cmd.Args, ",") != strings.Join(before, ",") {
		t.Errorf("没有 args/env 时不该动 argv: %v", cmd.Args)
	}
	if cmd.Env != nil {
		t.Errorf("没有 env 时不该设置 Env: %v", cmd.Env)
	}
}

func TestMergeProviderEnvOverridesCaseInsensitivelyOnWindows(t *testing.T) {
	base := []string{"PATH=C:\\a", "KEEP=1"}
	merged := mergeProviderEnv(base, []string{"PATH=C:\\b"})
	var paths []string
	for _, entry := range merged {
		if strings.HasPrefix(entry, "PATH=") || strings.HasPrefix(entry, "Path=") {
			paths = append(paths, entry)
		}
	}
	if runtime.GOOS == "windows" {
		if len(paths) != 1 || paths[0] != "PATH=C:\\b" {
			t.Errorf("Windows 上应只留覆盖后的 PATH，实际 %v", paths)
		}
	} else if len(paths) != 2 {
		t.Errorf("非 Windows 上大小写敏感，应保留两条，实际 %v", paths)
	}
	if !strings.Contains(strings.Join(merged, ","), "KEEP=1") {
		t.Errorf("未被覆盖的变量丢了: %v", merged)
	}
}

func TestProviderInvocationExtrasAffectFingerprint(t *testing.T) {
	cfg := newTestConfig()
	cfg.Providers["claude"] = config.ProviderConfig{
		Command: "claude",
		Args:    []string{"--profile", "work"},
		Env:     []string{"A=1"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置不合法: %v", err)
	}
	ac := cfg.Agents[testAgentClaude]
	p, err := provider.Lookup(ac.Provider)
	if err != nil {
		t.Fatalf("provider.Lookup: %v", err)
	}

	plain, err := identityFPAt(p, testAgentClaude, ac, "D:\\work", "", provider.PermissionRead, RulesBlocks{})
	if err != nil {
		t.Fatalf("无 extras 的指纹: %v", err)
	}
	withExtras, err := identityFPAt(p, testAgentClaude, ac, "D:\\work", "", provider.PermissionRead, RulesBlocks{},
		providerExtras(cfg, "claude")...)
	if err != nil {
		t.Fatalf("带 extras 的指纹: %v", err)
	}
	if plain == withExtras {
		t.Fatalf("加了 provider args/env 指纹却没变：改了启动参数会静默沿用旧会话")
	}
	// 同样的 extras 必须稳定：同一条会话里逐轮算出来的值不能漂。
	again, err := identityFPAt(p, testAgentClaude, ac, "D:\\work", "", provider.PermissionRead, RulesBlocks{},
		providerExtras(cfg, "claude")...)
	if err != nil {
		t.Fatalf("重复计算: %v", err)
	}
	if again != withExtras {
		t.Fatalf("同一份配置两次算出的指纹不同: %q vs %q", withExtras, again)
	}
	// args 与 env 要能区分开：只有 env 的配置不该与只有 args 的配置撞上。
	onlyArgs := config.ProviderConfig{Command: "claude", Args: []string{"A=1"}}
	onlyEnv := config.ProviderConfig{Command: "claude", Env: []string{"A=1"}}
	if strings.Join(ProviderInvocationExtras(onlyArgs), "|") == strings.Join(ProviderInvocationExtras(onlyEnv), "|") {
		t.Fatalf("args 与 env 的段标记没起作用")
	}
}

func TestProviderInvocationExtrasEmptyWhenUnset(t *testing.T) {
	if extras := ProviderInvocationExtras(config.ProviderConfig{Command: "x"}); extras != nil {
		t.Fatalf("空配置应返回 nil，实际 %v", extras)
	}
	if extras := providerExtras(nil, "claude"); extras != nil {
		t.Fatalf("nil 配置应返回 nil，实际 %v", extras)
	}
}

func TestProviderEnvIsInheritedByDefault(t *testing.T) {
	// 没有 env 时子进程照旧继承全部环境（这一步只是把既有约定写下来）。
	merged := mergeProviderEnv(os.Environ(), nil)
	if len(merged) != len(os.Environ()) {
		t.Fatalf("没有覆盖项时不该改变环境长度: %d vs %d", len(merged), len(os.Environ()))
	}
}
