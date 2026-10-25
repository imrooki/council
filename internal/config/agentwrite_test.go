package config

// agentwrite_test.go 覆盖设置页写配置文件的两层：纯字节的段编辑
// （upsertAgentSection / removeAgentSection），以及带备份、临时文件、
// 合并校验的 WriteAgentSection / RemoveAgentSection。
//
// 这里不碰任何真实配置文件：每个用例都在 t.TempDir() 里造一份配置。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// agentwriteProject 是项目层配置的样板：两个成员、一条注释、一个 CRLF 文件
// 用来验证行尾不会被改写。
func agentwriteProject(dataDir string) string {
	return fmt.Sprintf(`# 项目层：只写这个项目自己的成员
data_dir = '%s'

[providers.claude]
command = 'claude'

# p1 的 role 是手写的，设置页不该动它
[agents.p1]
display_name = "P1"
provider = "claude"
model = "sonnet"
role = "项目层成员"
permissions = "read"
mode = "discussion"

[agents.p2]
display_name = "P2"
provider = "claude"
model = "opus"
role = "第二个成员"
permissions = "read"
mode = "worker"
`, dataDir)
}

func agentwriteWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写 %s 失败: %v", path, err)
	}
}

func agentwriteRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", path, err)
	}
	return string(data)
}

// TestAgentSectionEditKeepsEverythingElse 钉住「只改目标文件里与这个 agent
// 相关的段」：其它成员的段、注释、空行、行尾逐字节不变；只改给定的键。
func TestAgentSectionEditKeepsEverythingElse(t *testing.T) {
	content := agentwriteProject(t.TempDir())
	// 让文件变成 CRLF：写回之后行尾必须仍是 CRLF。
	crlf := strings.ReplaceAll(content, "\n", "\r\n")
	agent := AgentConfig{
		DisplayName: "P1 改过",
		Provider:    "claude",
		Model:       "opus-5.5",
		Effort:      "max",
		Permissions: PermissionRead,
		Mode:        ModeDiscussion,
	}
	next, found, err := upsertAgentSection([]byte(crlf), "p1", agent, AgentConfig{})
	if err != nil || !found {
		t.Fatalf("upsertAgentSection = found %v, err %v", found, err)
	}
	got := string(next)

	// p2 那一段与注释必须原样还在（逐字节）。
	p2 := "[agents.p2]\r\ndisplay_name = \"P2\"\r\nprovider = \"claude\"\r\nmodel = \"opus\"\r\nrole = \"第二个成员\"\r\npermissions = \"read\"\r\nmode = \"worker\"\r\n"
	if !strings.Contains(got, p2) {
		t.Errorf("p2 段被改动了:\n%s", got)
	}
	if !strings.Contains(got, "# p1 的 role 是手写的，设置页不该动它\r\n") {
		t.Errorf("p1 上面的注释丢了:\n%s", got)
	}
	if !strings.Contains(got, "role = \"项目层成员\"\r\n") {
		t.Errorf("role 被改动了:\n%s", got)
	}
	if !strings.Contains(got, "display_name = \"P1 改过\"\r\n") ||
		!strings.Contains(got, "model = \"opus-5.5\"\r\n") ||
		!strings.Contains(got, "effort = \"max\"\r\n") {
		t.Errorf("目标字段没有写进去:\n%s", got)
	}
	if strings.Count(got, "[agents.p1]") != 1 {
		t.Errorf("p1 段出现了多次:\n%s", got)
	}
	// 全部行尾都是 CRLF，没有混进单独的 LF。
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Errorf("写回之后行尾混了 LF:\n%q", got)
	}
}

// TestAgentSectionEditRemovesClearedEffort 钉住「清空 effort 等于删掉这个键」，
// 并且缺失的键按固定顺序插在段尾。
func TestAgentSectionEditRemovesClearedEffort(t *testing.T) {
	content := agentwriteProject(t.TempDir()) + "\n[agents.p3]\ndisplay_name = \"P3\"\nprovider = \"claude\"\nmodel = \"grok\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	agent := AgentConfig{DisplayName: "P3", Provider: "claude", Model: "grok", Effort: "", Permissions: PermissionWorkspace, Mode: ModeWorker}
	next, _, err := upsertAgentSection([]byte(content), "p3", agent, AgentConfig{})
	if err != nil {
		t.Fatalf("upsertAgentSection: %v", err)
	}
	got := string(next)
	if strings.Contains(got, "effort =") {
		t.Errorf("effort 清空之后还留在文件里:\n%s", got)
	}
	// 只改了 permissions / mode 两个值；effort 那行消失；其它键保持原样。
	if !strings.Contains(got, "permissions = \"workspace\"\n") ||
		!strings.Contains(got, "mode = \"worker\"\n") {
		t.Errorf("permissions / mode 没更新:\n%s", got)
	}
}

// TestAgentSectionAppendForInheritedKeepsRole 钉住「目标文件里没有这一段时，
// 追加的整段要把继承来的 role / prompt 带过去」，否则整条覆盖会悄悄丢掉它们。
func TestAgentSectionAppendForInheritedKeepsRole(t *testing.T) {
	content := agentwriteProject(t.TempDir())
	fallback := AgentConfig{
		DisplayName: "G1",
		Provider:    "claude",
		Model:       "old",
		Role:        "继承来的角色",
		Prompt:      "第一行\n第二行",
		Permissions: PermissionRead,
		Mode:        ModeDiscussion,
	}
	agent := AgentConfig{DisplayName: "G1", Provider: "claude", Model: "new", Permissions: PermissionRead, Mode: ModeDiscussion}
	next, found, err := upsertAgentSection([]byte(content), "g1", agent, fallback)
	if err != nil || !found {
		t.Fatalf("upsertAgentSection = found %v, err %v", found, err)
	}
	got := string(next)
	if !strings.Contains(got, "[agents.g1]\n") ||
		!strings.Contains(got, "role = \"继承来的角色\"\n") ||
		!strings.Contains(got, "prompt = \"第一行\\u000A第二行\"\n") {
		t.Errorf("继承段的 role / prompt 没写进新段:\n%s", got)
	}
	// 原文仍然是新段的前缀（只追加，不重写）。
	if !strings.HasPrefix(got, content) {
		t.Errorf("追加新段改动了原有内容:\n%s", got)
	}
}

// TestAgentSectionEditEmptySectionAtEOF 钉住「空段位于文件末尾且没有结尾
// 换行」这个边角：插入键时必须先给表头补行尾，否则两行会粘成一行。
func TestAgentSectionEditEmptySectionAtEOF(t *testing.T) {
	content := "[providers.claude]\ncommand = 'c'\n[agents.x]"
	agent := AgentConfig{DisplayName: "X", Provider: "claude", Model: "m", Permissions: PermissionRead, Mode: ModeDiscussion}
	next, found, err := upsertAgentSection([]byte(content), "x", agent, AgentConfig{})
	if err != nil || !found {
		t.Fatalf("upsertAgentSection = found %v, err %v", found, err)
	}
	got := string(next)
	if !strings.Contains(got, "[agents.x]\ndisplay_name = \"X\"\n") {
		t.Errorf("空段的表头与插入的键粘在了一行:\n%q", got)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	agentwriteWrite(t, path, got)
	if _, err := Load(path); err != nil {
		t.Fatalf("写回的文件加载失败: %v\n%s", err, got)
	}
}

// TestAgentSectionEditKeepsTrailingComment 钉住「改值不删行尾注释」，同时
// 不把引号里的 #（例如 display_name = "C# 开发者"）当成注释。
func TestAgentSectionEditKeepsTrailingComment(t *testing.T) {
	content := "[agents.x]\ndisplay_name = \"C# 开发者\"\nprovider = \"claude\"\nmodel = \"old\"  # 生产用的模型\npermissions = \"read\"\nmode = \"discussion\"\n"
	agent := AgentConfig{DisplayName: "C# 开发者改名", Provider: "claude", Model: "new", Permissions: PermissionRead, Mode: ModeDiscussion}
	next, _, err := upsertAgentSection([]byte(content), "x", agent, AgentConfig{})
	if err != nil {
		t.Fatalf("upsertAgentSection: %v", err)
	}
	got := string(next)
	if !strings.Contains(got, "model = \"new\"  # 生产用的模型\n") {
		t.Errorf("行尾注释没有被保留:\n%s", got)
	}
	if !strings.Contains(got, "display_name = \"C# 开发者改名\"\n") {
		t.Errorf("引号里的 # 被误当成注释:\n%s", got)
	}
}

// TestAgentSectionEditKeepsBOM 钉住带 UTF-8 BOM 的文件：BOM 不能被当成
// 表名的一部分（那会找不到段、再追加一段同 id 的表，加载直接报重复定义），
// 也不能被顺手删掉。
func TestAgentSectionEditKeepsBOM(t *testing.T) {
	content := "\ufeff" + agentwriteProject(t.TempDir())
	agent := AgentConfig{DisplayName: "P1", Provider: "claude", Model: "opus-5.5", Permissions: PermissionRead, Mode: ModeDiscussion}
	next, found, err := upsertAgentSection([]byte(content), "p1", agent, AgentConfig{})
	if err != nil || !found {
		t.Fatalf("upsertAgentSection = found %v, err %v", found, err)
	}
	got := string(next)
	if strings.Count(got, "[agents.p1]") != 1 {
		t.Errorf("带 BOM 的文件被追加了第二段 p1:\n%s", got)
	}
	if !strings.HasPrefix(got, "\ufeff") {
		t.Errorf("BOM 被删掉了")
	}
	if !strings.Contains(got, "model = \"opus-5.5\"\n") {
		t.Errorf("目标字段没有写进去:\n%s", got)
	}
}

// TestAgentSectionDeleteKeepsOtherSections 钉住删除只移除目标段，其它段与
// 段前的注释保留。
func TestAgentSectionDeleteKeepsOtherSections(t *testing.T) {
	content := agentwriteProject(t.TempDir())
	next, found, err := removeAgentSection([]byte(content), "p1")
	if err != nil || !found {
		t.Fatalf("removeAgentSection = found %v, err %v", found, err)
	}
	got := string(next)
	if strings.Contains(got, "[agents.p1]") || strings.Contains(got, "display_name = \"P1\"") {
		t.Errorf("p1 段没删干净:\n%s", got)
	}
	if !strings.Contains(got, "[agents.p2]") || !strings.Contains(got, "role = \"第二个成员\"") {
		t.Errorf("p2 段被误删:\n%s", got)
	}
	if !strings.Contains(got, "data_dir = '") {
		t.Errorf("data_dir 行被误删:\n%s", got)
	}
	// 原文里 p1 段上方的注释讲的是 p1，应该跟着段一起消失；这里只要求
	// 文件仍能被解析（由调用方 LoadMerged 覆盖）。
	if _, found, err := removeAgentSection([]byte(got), "p1"); err != nil || found {
		t.Fatalf("二次删除同一段应当 found=false，实际 found=%v err=%v", found, err)
	}
}

// TestWriteAgentSectionBackupAndAtomicReplace 覆盖带落盘的路径：备份内容等于
// 写前原文，写后文件能加载，且不残留临时文件。
func TestWriteAgentSectionBackupAndAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := agentwriteProject(t.TempDir())
	agentwriteWrite(t, path, orig)

	agent := AgentConfig{DisplayName: "P1", Provider: "claude", Model: "opus-5.5", Permissions: PermissionRead, Mode: ModeDiscussion}
	res, err := WriteAgentSection([]Layer{{Path: path}}, "p1", agent, AgentConfig{})
	if err != nil {
		t.Fatalf("WriteAgentSection: %v", err)
	}
	if !res.Changed || res.WriteTarget != path {
		t.Fatalf("结果 = %+v，期望 Changed=true 且目标是 %s", res, path)
	}
	if res.Config == nil || res.Config.Agents["p1"].Model != "opus-5.5" {
		t.Fatalf("返回的配置没有反映新模型: %+v", res.Config)
	}
	if got := agentwriteRead(t, res.BackupPath); got != orig {
		t.Errorf("备份内容与写前原文不一致")
	}
	if base := filepath.Base(res.BackupPath); !regexp.MustCompile(`^config\.toml\.bak-\d{8}-\d{6}(-\d+)?$`).MatchString(base) {
		t.Errorf("备份文件名 = %q，期望 config.toml.bak-<YYYYMMDD-HHmmss>", base)
	}
	if got := agentwriteRead(t, path); !strings.Contains(got, "model = \"opus-5.5\"") {
		t.Errorf("目标文件没有写入新模型:\n%s", got)
	}
	temps, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if len(temps) != 0 {
		t.Errorf("残留临时文件: %v", temps)
	}
}

// TestWriteAgentSectionValidationFailureLeavesFileUntouched 钉住硬约束：
// 新内容过不了 Validate 时回 InvalidConfigError，目标文件逐字节不变，
// 也不生成备份或临时文件。
func TestWriteAgentSectionValidationFailureLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := agentwriteProject(t.TempDir())
	agentwriteWrite(t, path, orig)

	agent := AgentConfig{DisplayName: "P1", Provider: "nope", Model: "", Permissions: Permission("root"), Mode: ModeDiscussion}
	_, err := WriteAgentSection([]Layer{{Path: path}}, "p1", agent, AgentConfig{})
	var invalid *InvalidConfigError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v，期望 *InvalidConfigError", err)
	}
	for _, want := range []string{"undefined provider", "must not be empty", "invalid permissions"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误文案 %q 里缺少 %q", err.Error(), want)
		}
	}
	if got := agentwriteRead(t, path); got != orig {
		t.Errorf("校验失败之后目标文件被改动了")
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 0 {
		t.Errorf("校验失败时不该生成备份: %v", backups)
	}
	temps, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if len(temps) != 0 {
		t.Errorf("校验失败时残留临时文件: %v", temps)
	}
}

// TestRemoveAgentSectionValidationKeepsFile 钉住「删掉仍被 team 引用的成员
// 会被 Validate 挡住」：文件不变，错误说明引用了哪个成员。
func TestRemoveAgentSectionValidationKeepsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := agentwriteProject(t.TempDir()) + "\n[teams.t1]\nmembers = [\"p1\"]\n"
	agentwriteWrite(t, path, orig)

	_, err := RemoveAgentSection([]Layer{{Path: path}}, "p1")
	var invalid *InvalidConfigError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v，期望 *InvalidConfigError", err)
	}
	if !strings.Contains(err.Error(), "undefined agent") {
		t.Errorf("错误文案 = %q，期望点名 undefined agent", err.Error())
	}
	if got := agentwriteRead(t, path); got != orig {
		t.Errorf("校验失败之后目标文件被改动了")
	}
}
