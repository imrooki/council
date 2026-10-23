package repl

// skills_test.go 覆盖只读的 /skills：列出 .claude/skills 扫到的元数据、
// 空列表的一句话、没有描述时用目录名兜底，以及「只读命令不需要持久化」。
//
// 全部目录都在临时 HOME / 临时项目里造，不读这台机器上真实的 ~/.claude：
// ScanClaudeSkills 会先扫用户级，不隔离 HOME 的话用例结果取决于开发机。

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillsTestHome 把用户主目录指到临时目录，避免 /skills 扫到真实的 ~/.claude。
// HOME 给 Unix，USERPROFILE 给 Windows —— os.UserHomeDir 按平台取其中一个。
func skillsTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// skillsTestWriteSkill 在 <root>/.claude/skills/<dir>/SKILL.md 落一份 skill。
func skillsTestWriteSkill(t *testing.T, root, dir, frontmatter, body string) {
	t.Helper()
	path := filepath.Join(root, ".claude", "skills", dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建 skill 目录 %s 失败: %v", filepath.Dir(path), err)
	}
	content := "---\n" + frontmatter + "---\n\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写 %s 失败: %v", path, err)
	}
}

// TestSkillsListsScannedSkills 钉住 /skills 的输出形状：每条 skill 一行，
// 第一列是 Name、第二列是 Description；没有描述时第二列是目录名（ID）。
// 两条入口都跑一遍，证明 TUI 的 Submit 与行式 dispatch 走的是同一份命令表。
//
// 【判别力】把 cmdSkills 改成只打 Name 时第二条用例的目录名兜底红；漏掉
// oneLine 时多行 description 会把第三条拆成两行、行数断言红；把第二列
// 写成 skill.Name 时 plain-front 的兜底断言红（它期望目录名 plain）。
func TestSkillsListsScannedSkills(t *testing.T) {
	for _, entry := range []string{"dispatch", "submit"} {
		t.Run(entry, func(t *testing.T) {
			skillsTestHome(t)
			project := t.TempDir()
			// 扫描器按目录名排序，且 home 里没有同名项：deploy、multi、plain 依次输出。
			skillsTestWriteSkill(t, project, "deploy",
				"name: deploy-staging\ndescription: 部署到预发\n", "按步骤部署。")
			skillsTestWriteSkill(t, project, "plain", "name: plain-front\n", "")
			skillsTestWriteSkill(t, project, "multi",
				"description: |\n  第一行\n  第二行\n", "正文")

			var buf bytes.Buffer
			// svc 为 nil：只读命令不依赖持久化，也不该被 runCommand 前置条件拦下。
			r := New(nil, nil, nil, nil, &buf, RulesContext{})
			r.SetProjectDir(project)
			var err error
			if entry == "dispatch" {
				err = r.dispatch(context.Background(), "/skills")
			} else {
				err = r.Submit(context.Background(), "/skills")
			}
			if err != nil {
				t.Fatalf("/skills 返回 %v", err)
			}

			lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
			if len(lines) != 3 {
				t.Fatalf("/skills 有 %d 行，期望 3 行（每条 skill 一行）：\n%s", len(lines), buf.String())
			}
			want := []struct{ name, desc string }{
				{"deploy-staging", "部署到预发"},
				{"multi", "第一行 第二行"},     // 块标量 description 必须压成一行
				{"plain-front", "plain"}, // 没有描述 -> 目录名
			}
			for i, w := range want {
				if !strings.HasPrefix(lines[i], w.name) {
					t.Errorf("第 %d 行 = %q，期望以 %q 开头", i+1, lines[i], w.name)
					continue
				}
				if got := strings.TrimSpace(strings.TrimPrefix(lines[i], w.name)); got != w.desc {
					t.Errorf("第 %d 行 /%s 的第二列 = %q，期望 %q（整行 %q）",
						i+1, w.name, got, w.desc, lines[i])
				}
			}
		})
	}
}

// TestSkillsEmptyMessage 钉住空列表的答复：一句固定的中文，不是表头也不是
// 静默成功。HOME 与项目里都没有 .claude/skills，扫描结果为 0。
func TestSkillsEmptyMessage(t *testing.T) {
	skillsTestHome(t)
	var buf bytes.Buffer
	r := New(nil, nil, nil, nil, &buf, RulesContext{})
	r.SetProjectDir(t.TempDir())

	if err := r.dispatch(context.Background(), "/skills"); err != nil {
		t.Fatalf("/skills 返回 %v", err)
	}
	if got, want := buf.String(), msgNoSkills+"\n"; got != want {
		t.Errorf("/skills 的空列表输出 = %q，期望 %q", got, want)
	}
}

// TestSkillsRegistryEntry 钉住命令元数据：Args 是空串（/skills 不接受参数）、
// Summary 是单行；表项没有任何持久化 / provider 前置条件 —— 三者只要有一个
// 被设上，svc 或 rn 为 nil 的入口就会回「未就绪」，而 /skills 只读磁盘。
func TestSkillsRegistryEntry(t *testing.T) {
	spec, entry, ok := newCommandRegistry().Lookup("skills")
	if !ok {
		t.Fatal("/skills 没有登记在命令表里")
	}
	if spec.Args != "" {
		t.Errorf("/skills 的 Args = %q，期望空串", spec.Args)
	}
	if spec.Summary == "" || strings.ContainsAny(spec.Summary, "\r\n") {
		t.Errorf("/skills 的 Summary 不是一行非空说明: %q", spec.Summary)
	}
	if entry.needsSvc || entry.needsConv || entry.needsRunner {
		t.Errorf("/skills 被挂上了前置条件 needsSvc=%v needsConv=%v needsRunner=%v，它只读磁盘，都不需要",
			entry.needsSvc, entry.needsConv, entry.needsRunner)
	}
	if entry.run == nil {
		t.Error("/skills 没有处理函数")
	}
}
