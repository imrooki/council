package config

// B5：设置页按行改 TOML 时必须知道哪些行在多行字符串里。role/prompt 里
// 出现 "[输出格式]" 或 "model = …" 时，前者不是表头、后者不是字段，否则
// 新值会被插进 role 正文，写回成功但内容已经坏了。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestB5MultilineRoleKeepsSectionBoundary(t *testing.T) {
	dir := t.TempDir()
	content := `data_dir = ` + tomlBasicString(dir) + `

[providers.claude]
command = 'claude'

[agents.p1]
display_name = "P1"
provider = "claude"
model = "sonnet"
role = """
[输出格式]
model = "role-internal"
"""
permissions = "read"
mode = "discussion"

[agents.p2]
display_name = "P2"
provider = "claude"
model = "opus"
role = "第二个成员"
permissions = "read"
mode = "worker"
`
	next, found, err := upsertAgentSection([]byte(content), "p1", AgentConfig{
		DisplayName: "P1 改过",
		Provider:    "claude",
		Model:       "opus-5",
		Permissions: PermissionRead,
		Mode:        ModeDiscussion,
	}, AgentConfig{})
	if err != nil || !found {
		t.Fatalf("upsertAgentSection = found %v, err %v", found, err)
	}

	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, next, 0o644); err != nil {
		t.Fatalf("写回配置失败: %v", err)
	}
	merged, err := LoadMerged([]Layer{{Path: path}})
	if err != nil {
		t.Fatalf("写回后的配置解析失败（B5 把字段插进了多行字符串）: %v\n%s", err, next)
	}
	p1, ok := merged.Config.Agents["p1"]
	if !ok {
		t.Fatalf("p1 段消失:\n%s", next)
	}
	if p1.DisplayName != "P1 改过" || p1.Model != "opus-5" {
		t.Errorf("字段没有写进去: display_name=%q model=%q", p1.DisplayName, p1.Model)
	}
	if !strings.Contains(p1.Role, "[输出格式]") || !strings.Contains(p1.Role, `model = "role-internal"`) {
		t.Errorf("role 正文被改坏了: %q\n%s", p1.Role, next)
	}
	if strings.Contains(p1.Role, "permissions =") || strings.Contains(p1.Role, "mode =") {
		t.Errorf("permissions/mode 被插进了 role 正文: %q\n%s", p1.Role, next)
	}
	if p2, ok := merged.Config.Agents["p2"]; !ok || p2.DisplayName != "P2" || p2.Mode != ModeWorker {
		t.Errorf("p2 段被改坏了: %+v\n%s", p2, next)
	}
}
