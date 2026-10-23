package config

// srv10_skills_test.go 覆盖 SRV10 §4.2/§4.3：Claude Code skills 的目录结构、
// 用户级与项目级、优先级（官方：personal over project）、frontmatter 缺省值、
// 保留名，以及目录不存在时的空结果。
//
// 全部目录都在临时 HOME / 临时仓库里造，不读这台机器上真实的 ~/.claude。

import (
	"os"
	"path/filepath"
	"testing"
)

// srv10WriteSkill 在 <root>/.claude/skills/<name>/SKILL.md 落一份 skill。
func srv10WriteSkill(t *testing.T, root, name, frontmatter, body string) {
	t.Helper()
	srv10WriteRawFile(t, filepath.Join(root, ".claude", "skills", name, "SKILL.md"),
		"---\n"+frontmatter+"---\n\n"+body+"\n")
}

// srv10WriteRawFile 直接写一段内容，用来构造「没有 frontmatter」这类文件。
func srv10WriteRawFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录 %s 失败: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件 %s 失败: %v", path, err)
	}
}

// srv10SkillByID 在结果里按目录名找一条；找不到直接失败。
func srv10SkillByID(t *testing.T, skills []Skill, id string) Skill {
	t.Helper()
	for _, skill := range skills {
		if skill.ID == id {
			return skill
		}
	}
	t.Fatalf("扫描结果里没有 skill %q：%+v", id, skills)
	return Skill{}
}

// TestSrv10SkillsScanUserAndProject 是 §4.2：用户级与项目级都能扫到；ID 取
// 目录名、Name 取 frontmatter 的 name（缺省等于目录名）、Description 取
// frontmatter 的 description；结果按优先级排序（用户级在前）。
func TestSrv10SkillsScanUserAndProject(t *testing.T) {
	home := srv7ClaudeIsolateHome(t)
	project := t.TempDir()

	srv10WriteSkill(t, project, "deploy", "name: deploy-staging\ndescription: 部署到预发\n", "按步骤部署。")
	srv10WriteSkill(t, home, "summarize", "description: 总结改动\n", "总结本次改动。")

	got := ScanClaudeSkills(project)
	if len(got) != 2 {
		t.Fatalf("应当扫到 2 个 skill（用户级 + 项目级），实际 %+v", got)
	}
	// 官方优先级 personal > project：用户级必须先出现。
	if got[0].ID != "summarize" || got[1].ID != "deploy" {
		t.Errorf("扫描顺序 = [%s %s]，期望用户级 summarize 在项目级 deploy 之前",
			got[0].ID, got[1].ID)
	}

	deploy := srv10SkillByID(t, got, "deploy")
	if deploy.Name != "deploy-staging" {
		t.Errorf("deploy.Name = %q，期望 frontmatter 的 name（deploy-staging）", deploy.Name)
	}
	if deploy.Description != "部署到预发" {
		t.Errorf("deploy.Description = %q，期望 frontmatter 的 description", deploy.Description)
	}
	wantPath := filepath.Join(project, ".claude", "skills", "deploy", "SKILL.md")
	if deploy.Path != wantPath {
		t.Errorf("deploy.Path = %q，期望 %q", deploy.Path, wantPath)
	}

	summarize := srv10SkillByID(t, got, "summarize")
	if summarize.Name != "summarize" {
		t.Errorf("没有 name 字段时 Name 应当取目录名，实际 %q", summarize.Name)
	}
	if summarize.Description != "总结改动" {
		t.Errorf("summarize.Description = %q，期望 frontmatter 的 description", summarize.Description)
	}
}

// TestSrv10SkillsPriority 是 §4.2 的优先级：同名时用户级盖住项目级；去掉
// 用户级后，离 cwd 近的项目层盖住仓库根（与 .council / agents 共用同一条
// 项目链）；两层的同名项只保留一条。
func TestSrv10SkillsPriority(t *testing.T) {
	home := srv7ClaudeIsolateHome(t)
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("建假仓库 .git 失败: %v", err)
	}
	cwd := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("建 cwd 失败: %v", err)
	}

	srv10WriteSkill(t, home, "dup", "description: 用户级版本\n", "user")
	srv10WriteSkill(t, repo, "dup", "description: 仓库根版本\n", "root")
	srv10WriteSkill(t, cwd, "dup", "description: cwd 版本\n", "near")
	// 项目链上的非重名项都要读到：根层只出现在仓库根，近层只出现在 cwd。
	srv10WriteSkill(t, repo, "only-root", "description: 只在仓库根\n", "root")
	srv10WriteSkill(t, cwd, "only-cwd", "description: 只在 cwd\n", "near")

	got := ScanClaudeSkills(cwd)
	if desc := srv10SkillByID(t, got, "dup").Description; desc != "用户级版本" {
		t.Errorf("同名时用户级应当胜出（personal over project），实际 %q", desc)
	}
	if count := srv10CountSkill(got, "dup"); count != 1 {
		t.Errorf("同名 skill 应当只出现 1 条，实际 %d 条：%+v", count, got)
	}
	// 项目链：cwd 与仓库根的非重名项都在。
	srv10SkillByID(t, got, "only-root")
	srv10SkillByID(t, got, "only-cwd")

	// 去掉用户级：最近的项目层（cwd）胜出。
	if err := os.RemoveAll(filepath.Join(home, ".claude", "skills", "dup")); err != nil {
		t.Fatalf("删除用户级 dup 失败: %v", err)
	}
	got = ScanClaudeSkills(cwd)
	if desc := srv10SkillByID(t, got, "dup").Description; desc != "cwd 版本" {
		t.Errorf("没有用户级时应当由离 cwd 最近的层胜出，实际 %q", desc)
	}

	// 再去掉 cwd：仓库根胜出（证明项目链是逐层向上读的）。
	if err := os.RemoveAll(filepath.Join(cwd, ".claude", "skills", "dup")); err != nil {
		t.Fatalf("删除 cwd 级 dup 失败: %v", err)
	}
	got = ScanClaudeSkills(cwd)
	if desc := srv10SkillByID(t, got, "dup").Description; desc != "仓库根版本" {
		t.Errorf("只剩仓库根时应当读到根层定义，实际 %q", desc)
	}
}

// srv10CountSkill 数一个 ID 在结果里出现几次。
func srv10CountSkill(skills []Skill, id string) int {
	count := 0
	for _, skill := range skills {
		if skill.ID == id {
			count++
		}
	}
	return count
}

// TestSrv10SkillsFrontmatterDefaults 是官方缺省规则：name 缺省取目录名；
// description 缺省取正文第一行非空内容。没有 frontmatter 的文件同样能导入，
// 整个文件都算正文。多行 description 的两种块标量（> 折叠、| 字面）要读对，
// metadata 这类嵌套映射里的同名键不能覆盖顶层字段。
func TestSrv10SkillsFrontmatterDefaults(t *testing.T) {
	srv7ClaudeIsolateHome(t)
	project := t.TempDir()

	srv10WriteSkill(t, project, "only-name", "name: friendly\n", "第一行说明\n第二行")
	srv10WriteSkill(t, project, "folded", "description: >\n  第一段\n  第二段\n", "正文")
	srv10WriteSkill(t, project, "literal", "description: |\n  第一行\n  第二行\n", "正文")
	srv10WriteSkill(t, project, "nested",
		"name: outer\ndescription: 顶层描述\nmetadata:\n  name: 内层名字\n  description: 内层描述\n", "正文")
	srv10WriteRawFile(t, filepath.Join(project, ".claude", "skills", "no-frontmatter", "SKILL.md"),
		"\n# 标题\n\n正文")

	got := ScanClaudeSkills(project)

	onlyName := srv10SkillByID(t, got, "only-name")
	if onlyName.Name != "friendly" {
		t.Errorf("Name = %q，期望 frontmatter 的 friendly", onlyName.Name)
	}
	if onlyName.Description != "第一行说明" {
		t.Errorf("description 缺省应取正文第一行非空内容，实际 %q", onlyName.Description)
	}

	if desc := srv10SkillByID(t, got, "folded").Description; desc != "第一段 第二段" {
		t.Errorf("description: > 应当折叠成一行，实际 %q", desc)
	}
	if desc := srv10SkillByID(t, got, "literal").Description; desc != "第一行\n第二行" {
		t.Errorf("description: | 应当保留换行，实际 %q", desc)
	}

	nested := srv10SkillByID(t, got, "nested")
	if nested.Name != "outer" || nested.Description != "顶层描述" {
		t.Errorf("metadata 里的同名键不该覆盖顶层字段：name=%q description=%q",
			nested.Name, nested.Description)
	}

	noFrontmatter := srv10SkillByID(t, got, "no-frontmatter")
	if noFrontmatter.Name != "no-frontmatter" {
		t.Errorf("没有 frontmatter 时 Name 应取目录名，实际 %q", noFrontmatter.Name)
	}
	if noFrontmatter.Description != "# 标题" {
		t.Errorf("没有 frontmatter 时 description 应取正文第一行（原样保留 Markdown），实际 %q",
			noFrontmatter.Description)
	}
}

// TestSrv10SkillsSkipsReservedAndNonSkills 钉住两类不该出现的条目：官方保留名
// （synced / anthropic-skills）与不是 skill 的目录——没有 SKILL.md 的、以及
// 藏在更深层的 SKILL.md（官方只在 <skill-name>/SKILL.md 这一层找，启动时
// 也不加载 cwd 之下的嵌套 skills）。
func TestSrv10SkillsSkipsReservedAndNonSkills(t *testing.T) {
	srv7ClaudeIsolateHome(t)
	project := t.TempDir()

	srv10WriteSkill(t, project, "synced", "description: claude.ai 同步缓存\n", "x")
	srv10WriteSkill(t, project, "Anthropic-Skills", "description: 保留命名空间\n", "x")
	srv10WriteSkill(t, project, "real", "description: 真正的 skill\n", "x")

	// 没有 SKILL.md 的目录不是 skill；它下面的 SKILL.md 也不该被递归读到。
	srv10WriteRawFile(t, filepath.Join(project, ".claude", "skills", "group", "nested", "SKILL.md"),
		"---\ndescription: 更深一层\n---\n\nx\n")
	// .claude/skills 下的散装 .md 不是 skill。
	srv10WriteRawFile(t, filepath.Join(project, ".claude", "skills", "loose.md"),
		"---\ndescription: 散装文件\n---\n\nx\n")

	got := ScanClaudeSkills(project)
	if len(got) != 1 {
		t.Fatalf("应当只剩 1 个真正的 skill，实际 %+v", got)
	}
	if got[0].ID != "real" {
		t.Errorf("唯一保留的 skill = %q，期望 real", got[0].ID)
	}
}

// TestSrv10SkillsMissingDirsReturnEmpty 是 §4.3：目录不存在（包括 cwd 为空）
// 时不报错，返回非 nil 的空切片——/api/options 要能编码成 [] 而不是 null。
func TestSrv10SkillsMissingDirsReturnEmpty(t *testing.T) {
	srv7ClaudeIsolateHome(t)

	for _, cwd := range []string{t.TempDir(), ""} {
		got := ScanClaudeSkills(cwd)
		if got == nil {
			t.Fatalf("ScanClaudeSkills(%q) 返回 nil，期望非 nil 的空切片", cwd)
		}
		if len(got) != 0 {
			t.Errorf("ScanClaudeSkills(%q) 应当为空，实际 %+v", cwd, got)
		}
	}

	// .claude/skills 是一个普通文件（而不是目录）时也只当作 0 个。
	project := t.TempDir()
	srv10WriteRawFile(t, filepath.Join(project, ".claude", "skills"), "不是目录")
	if got := ScanClaudeSkills(project); len(got) != 0 {
		t.Errorf(".claude/skills 是文件时应当为空，实际 %+v", got)
	}
}
