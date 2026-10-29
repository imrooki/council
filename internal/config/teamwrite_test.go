package config

// teamwrite_test.go 钉住 teams / summary / data_dir 三处「原来只读」的写盘：
// 只改目标段、其它键保留、删队伍不删成员、关汇总时整表消失。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func teamFixture(t *testing.T) []Layer {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `# 顶层注释
[providers.claude]
command = 'claude'

[agents.a]
provider = "claude"
model = "opus"
permissions = "read"

[agents.b]
provider = "claude"
model = "opus"
permissions = "read"

[teams.review]
display_name = "Review"
members = ["a"]  # 先只有 a

[summary]
agent = 'a'
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置: %v", err)
	}
	return []Layer{{Path: path}}
}

func TestWriteTeamSectionUpdatesMembers(t *testing.T) {
	layers := teamFixture(t)
	res, err := WriteTeamSection(layers, "review", TeamConfig{DisplayName: "Review", Members: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("写 team: %v", err)
	}
	if !res.Changed || res.BackupPath == "" {
		t.Fatalf("Changed=%v Backup=%q", res.Changed, res.BackupPath)
	}
	text := readFile(t, layers[0].Path)
	if !strings.Contains(text, `members = ['a', 'b']  # 先只有 a`) {
		t.Errorf("members 没更新或注释丢了:\n%s", text)
	}
	if strings.Count(text, "[teams.review]") != 1 {
		t.Errorf("段被复制:\n%s", text)
	}
	if !strings.Contains(text, "# 顶层注释") || !strings.Contains(text, "[summary]") {
		t.Errorf("其它段被动了:\n%s", text)
	}
}

func TestWriteTeamSectionAddsNewTeam(t *testing.T) {
	layers := teamFixture(t)
	if _, err := WriteTeamSection(layers, "audit", TeamConfig{DisplayName: "Audit", Members: []string{"b"}}); err != nil {
		t.Fatalf("新增 team: %v", err)
	}
	text := readFile(t, layers[0].Path)
	for _, want := range []string{"[teams.audit]", `display_name = 'Audit'`, `members = ['b']`} {
		if !strings.Contains(text, want) {
			t.Errorf("缺少 %q:\n%s", want, text)
		}
	}
}

func TestRemoveTeamSection(t *testing.T) {
	layers := teamFixture(t)
	if _, err := RemoveTeamSection(layers, "review"); err != nil {
		t.Fatalf("删 team: %v", err)
	}
	text := readFile(t, layers[0].Path)
	if strings.Contains(text, "[teams.review]") {
		t.Errorf("段还在:\n%s", text)
	}
	// 删名单不等于删成员。
	if !strings.Contains(text, "[agents.a]") || !strings.Contains(text, "[agents.b]") {
		t.Errorf("删 team 把成员也删了:\n%s", text)
	}
}

func TestWriteSummaryAgentReplacesAndClears(t *testing.T) {
	layers := teamFixture(t)
	if _, err := WriteSummaryAgent(layers, "b"); err != nil {
		t.Fatalf("改汇总: %v", err)
	}
	text := readFile(t, layers[0].Path)
	if !strings.Contains(text, "agent = 'b'") {
		t.Errorf("summary.agent 没改:\n%s", text)
	}
	// 关掉汇总：整张表消失，而不是留下 agent = ''。
	if _, err := WriteSummaryAgent(layers, ""); err != nil {
		t.Fatalf("关汇总: %v", err)
	}
	text = readFile(t, layers[0].Path)
	if strings.Contains(text, "[summary]") {
		t.Errorf("关掉汇总后表还留着:\n%s", text)
	}
	if !strings.Contains(text, "[teams.review]") {
		t.Errorf("关汇总时把 team 也删了:\n%s", text)
	}
}

func TestWriteDataDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[providers.claude]\ncommand = 'claude'\n"), 0o644); err != nil {
		t.Fatalf("写配置: %v", err)
	}
	layers := []Layer{{Path: path}}
	target := filepath.Join(dir, "data2")
	if _, err := WriteDataDir(layers, target); err != nil {
		t.Fatalf("写 data_dir: %v", err)
	}
	text := readFile(t, path)
	// 值可能是单引号也可能是基本字符串（反斜杠要转义），所以按语义断言。
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("写完之后配置解析失败: %v\n%s", err, text)
	}
	if cfg.DataDir != target {
		t.Errorf("DataDir = %q，期望 %q\n%s", cfg.DataDir, target, text)
	}
	if !strings.HasPrefix(strings.TrimSpace(text), "data_dir = ") {
		t.Errorf("data_dir 应该在文件顶部（表头之前）:\n%s", text)
	}
	// 再改一次：只能有一行 data_dir。
	if _, err := WriteDataDir(layers, filepath.Join(dir, "data3")); err != nil {
		t.Fatalf("再写 data_dir: %v", err)
	}
	text = readFile(t, path)
	if strings.Count(text, "data_dir =") != 1 {
		t.Errorf("data_dir 出现了多次:\n%s", text)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s: %v", path, err)
	}
	return string(data)
}
