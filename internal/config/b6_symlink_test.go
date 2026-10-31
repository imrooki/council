//go:build !windows

package config

// B6：原子写用 Rename 覆盖目标路径，如果目标本身是符号链接，Rename 会把
// 链接替换成普通文件。写之前必须解析到真实目标，链接本身保留。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func b6WriteTarget(t *testing.T, dir, body string) (link, real string) {
	t.Helper()
	real = filepath.Join(dir, "real.toml")
	if err := os.WriteFile(real, []byte(body), 0o644); err != nil {
		t.Fatalf("写真实配置失败: %v", err)
	}
	link = filepath.Join(dir, "config.toml")
	if err := os.Symlink(filepath.Base(real), link); err != nil {
		t.Fatalf("建符号链接失败: %v", err)
	}
	return link, real
}

func b6AssertSymlink(t *testing.T, link, real string) {
	t.Helper()
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat 链接失败: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("写入之后 %s 不再是符号链接（B6）", link)
	}
	got, err := os.ReadFile(real)
	if err != nil {
		t.Fatalf("读真实目标失败: %v", err)
	}
	if !strings.Contains(string(got), "P1 改过") && !strings.Contains(string(got), "p2") {
		t.Fatalf("真实目标没有收到写入: %s", got)
	}
}

func TestB6AppendBlockKeepsSymlink(t *testing.T) {
	dir := t.TempDir()
	link, real := b6WriteTarget(t, dir, "data_dir = '"+dir+"'\n")
	if _, _, err := AppendBlock(link, "\n[agents.p2]\ndisplay_name = \"P2\"\n"); err != nil {
		t.Fatalf("AppendBlock 失败: %v", err)
	}
	b6AssertSymlink(t, link, real)
}

func TestB6WriteAgentSectionKeepsSymlink(t *testing.T) {
	dir := t.TempDir()
	body := "data_dir = '" + dir + "'\n\n[providers.claude]\ncommand = 'claude'\n\n" +
		"[agents.p1]\ndisplay_name = \"P1\"\nprovider = \"claude\"\nmodel = \"sonnet\"\n" +
		"permissions = \"read\"\nmode = \"discussion\"\n"
	link, real := b6WriteTarget(t, dir, body)
	_, err := WriteAgentSection([]Layer{{Path: link}}, "p1", AgentConfig{
		DisplayName: "P1 改过",
		Provider:    "claude",
		Model:       "opus",
		Permissions: PermissionRead,
		Mode:        ModeDiscussion,
	}, AgentConfig{})
	if err != nil {
		t.Fatalf("WriteAgentSection 失败: %v", err)
	}
	b6AssertSymlink(t, link, real)
}
