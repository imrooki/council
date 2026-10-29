package config

// providerwrite_test.go 钉住「带 args / env 的 provider 段」的写盘形状：
// 只改这一段、空数组不落盘、行尾注释保留、改完还能通过 Validate。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProviderFixture(t *testing.T, body string) []Layer {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置: %v", err)
	}
	return []Layer{{Path: path}}
}

const providerFixtureBase = `# 顶层注释
[providers.claude]
command = 'claude'  # 这是用户写的说明

[agents.reviewer]
provider = "claude"
model = "opus"
permissions = "read"
mode = "discussion"
`

func TestWriteProviderSectionAddsArgsAndEnv(t *testing.T) {
	layers := writeProviderFixture(t, providerFixtureBase)
	res, err := WriteProviderSection(layers, "deepseek", ProviderConfig{
		Command: `C:\Tools\deepseek.exe`,
		Args:    []string{"--fast", "--json"},
		Env:     []string{"DEEPSEEK_KEY=abc", "DEEPSEEK_HOME=C:\\tmp"},
	})
	if err != nil {
		t.Fatalf("写 provider: %v", err)
	}
	if !res.Changed || res.BackupPath == "" {
		t.Fatalf("Changed=%v BackupPath=%q", res.Changed, res.BackupPath)
	}
	body, err := os.ReadFile(layers[0].Path)
	if err != nil {
		t.Fatalf("读回配置: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"[providers.deepseek]",
		`command = 'C:\Tools\deepseek.exe'`,
		`args = ['--fast', '--json']`,
		`env = ['DEEPSEEK_KEY=abc', 'DEEPSEEK_HOME=C:\tmp']`,
		"# 顶层注释",
		"[providers.claude]",
		`command = 'claude'  # 这是用户写的说明`,
		"[agents.reviewer]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("配置里缺少 %q:\n%s", want, text)
		}
	}
	if got := res.Config.Providers["deepseek"].Args; len(got) != 2 || got[0] != "--fast" {
		t.Errorf("校验后的 Args = %v", got)
	}
}

func TestWriteProviderSectionKeepsCommentWhenCommandChanges(t *testing.T) {
	layers := writeProviderFixture(t, providerFixtureBase)
	if _, err := WriteProviderSection(layers, "claude", ProviderConfig{
		Command: "claude-v2",
		Args:    []string{"--profile", "work"},
	}); err != nil {
		t.Fatalf("改 provider: %v", err)
	}
	body, _ := os.ReadFile(layers[0].Path)
	text := string(body)
	if !strings.Contains(text, "command = 'claude-v2'  # 这是用户写的说明") {
		t.Errorf("改值时行尾注释丢了:\n%s", text)
	}
	if !strings.Contains(text, `args = ['--profile', 'work']`) {
		t.Errorf("args 没有插进已有段:\n%s", text)
	}
	if strings.Count(text, "[providers.claude]") != 1 {
		t.Errorf("段被复制了:\n%s", text)
	}
}

func TestWriteProviderSectionDropsEmptyArray(t *testing.T) {
	layers := writeProviderFixture(t, providerFixtureBase+`
[providers.extra]
command = 'extra'
args = ['--old']
env = ['OLD=1']
`)
	// 清空 args / env：两行都应从文件里消失，而不是留下 args = []。
	if _, err := WriteProviderSection(layers, "extra", ProviderConfig{Command: "extra"}); err != nil {
		t.Fatalf("清空: %v", err)
	}
	body, _ := os.ReadFile(layers[0].Path)
	text := string(body)
	if strings.Contains(text, "args =") || strings.Contains(text, "env =") {
		t.Errorf("空数组没有从文件里删掉:\n%s", text)
	}
	if !strings.Contains(text, "[providers.extra]") || !strings.Contains(text, "command = 'extra'") {
		t.Errorf("段本身被弄丢了:\n%s", text)
	}
}

func TestProviderValidationRejectsBadArgsAndEnv(t *testing.T) {
	cfg := Default()
	cfg.Providers["p"] = ProviderConfig{Command: "x", Env: []string{"NOVALUE"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("env 没有等号却通过了校验")
	}
	cfg.Providers["p"] = ProviderConfig{Command: "x", Args: []string{""}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("空参数却通过了校验")
	}
	cfg.Providers["p"] = ProviderConfig{Command: "x", Args: make([]string, MaxProviderArgs+1)}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("参数条数超限却通过了校验")
	}
	cfg.Providers["p"] = ProviderConfig{Command: "x", Args: []string{"ok"}, Env: []string{"A=1"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("合法配置被拒: %v", err)
	}
}

func TestEnvEntryProblem(t *testing.T) {
	ok := []string{"A=1", "_A=1", "PATH2=C:\\x", "A="}
	bad := []string{"", "A", "=1", "1A=1", "A-B=1", "A B=1", "A=B\x00C"}
	for _, entry := range ok {
		if problem := EnvEntryProblem(entry); problem != "" {
			t.Errorf("%q 应该合法，却报 %q", entry, problem)
		}
	}
	for _, entry := range bad {
		if problem := EnvEntryProblem(entry); problem == "" {
			t.Errorf("%q 应该被拒", entry)
		}
	}
}
