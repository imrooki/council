package config

// srv7_claude_test.go 覆盖 SRV7 §5.4-§5.6：Claude agents 的扫描顺序、递归、
// 就近覆盖、同层重名、必填字段，以及 tools -> permissions 的映射。
//
// 全部目录都在临时 HOME / 临时仓库里造，不读这台机器上真实的 ~/.claude。

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// srv7ClaudeIsolateHome 把用户主目录指到临时目录，避免扫描碰到真实 HOME。
func srv7ClaudeIsolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// srv7SortedClaudeIDs 给失败信息一个稳定的 id 顺序。
func srv7SortedClaudeIDs(agents map[string]AgentConfig) []string {
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// srv7ClaudeBody 生成一个带 frontmatter 的 Claude agent 文件内容。
func srv7ClaudeBody(frontmatter, prompt string) string {
	return "---\n" + frontmatter + "---\n\n" + prompt + "\n"
}

// TestSrv7ClaudeScanUserAndProject 是 §5.4：用户级与项目级都能读到；
// name 才是身份（不来自文件名）；缺 description 的被跳过并留提示；
// 与配置里的 agent 同名时配置优先。
func TestSrv7ClaudeScanUserAndProject(t *testing.T) {
	home := srv7ClaudeIsolateHome(t)
	project := t.TempDir()

	// 文件名故意与 name 不同：断言的是 frontmatter 的 name，不是文件名。
	srv7Write(t, filepath.Join(home, ".claude", "agents", "x.md"), srv7ClaudeBody(
		"name: user-agent\ndescription: 用户级成员\ntools: Read, Grep\nmodel: sonnet\n",
		"你是用户级 agent。"))
	srv7Write(t, filepath.Join(project, ".claude", "agents", "y.md"), srv7ClaudeBody(
		"name: project-agent\ndescription: 项目级成员\ntools: Read\n",
		"你是项目级 agent。"))
	srv7Write(t, filepath.Join(project, ".claude", "agents", "wrong-name.md"), srv7ClaudeBody(
		"name: identity-from-frontmatter\ndescription: 文件名不参与身份\n",
		"正文"))
	srv7Write(t, filepath.Join(project, ".claude", "agents", "broken.md"), srv7ClaudeBody(
		"name: broken-agent\n",
		"缺 description，应当被跳过。"))

	got := ScanClaudeAgents(project, nil)
	if len(got.Agents) != 3 {
		t.Fatalf("应当读到 3 个 agent（用户级、项目级、frontmatter 命名那个），实际 %v", srv7SortedClaudeIDs(got.Agents))
	}
	for _, id := range []string{"user-agent", "project-agent", "identity-from-frontmatter"} {
		if _, ok := got.Agents[id]; !ok {
			t.Errorf("缺少 agent %q；实际 %v", id, srv7SortedClaudeIDs(got.Agents))
		}
	}
	for _, wrong := range []string{"x", "y", "wrong-name", "broken-agent"} {
		if _, ok := got.Agents[wrong]; ok {
			t.Errorf("%q 不该作为 id 出现（id 必须来自 frontmatter 的 name）", wrong)
		}
	}

	user := got.Agents["user-agent"]
	if user.Provider != "claude" || user.Mode != ModeDiscussion || user.Model != "sonnet" {
		t.Errorf("user-agent 的固定映射不对: %+v", user)
	}
	if user.Role != "用户级成员" {
		t.Errorf("description 应当映射成 role，实际 %q", user.Role)
	}
	if user.Permissions != PermissionRead {
		t.Errorf("只有 Read/Grep 的 tools 应当映射成 read，实际 %q", user.Permissions)
	}
	if user.Prompt != "你是用户级 agent。" {
		t.Errorf("frontmatter 之后的正文应当存进 Prompt，实际 %q", user.Prompt)
	}
	if got.Agents["project-agent"].Prompt != "你是项目级 agent。" {
		t.Errorf("项目级 agent 的正文没有存进 Prompt: %q", got.Agents["project-agent"].Prompt)
	}

	// 缺 description 的文件必须留一条 Verbose 提示，不能无声消失。
	found := false
	for _, note := range got.Notices {
		if strings.Contains(note, "broken.md") && strings.Contains(note, "description") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("缺 description 的文件应当有提示，实际 %v", got.Notices)
	}

	// 与配置里的 agent 同名：配置优先，Claude 那条跳过并提示。
	withReserved := ScanClaudeAgents(project, map[string]AgentConfig{"project-agent": {Model: "from-config"}})
	if _, ok := withReserved.Agents["project-agent"]; ok {
		t.Errorf("与配置同名的 project-agent 应当被跳过")
	}
	if _, ok := withReserved.Agents["user-agent"]; !ok {
		t.Errorf("不冲突的 user-agent 仍应保留")
	}
	conflictNotice := false
	for _, note := range withReserved.Notices {
		if strings.Contains(note, "project-agent") && strings.Contains(note, "配置优先") {
			conflictNotice = true
			break
		}
	}
	if !conflictNotice {
		t.Errorf("同名冲突应当留一条提示，实际 %v", withReserved.Notices)
	}
}

// TestSrv7ClaudeScanNearestOverridesAndRecurses 是 §5.5：嵌套目录里的文件要
// 递归读到；同名时离 cwd 近的胜出；同一层内重名按文件路径字典序取第一个。
func TestSrv7ClaudeScanNearestOverridesAndRecurses(t *testing.T) {
	srv7ClaudeIsolateHome(t)
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("建假仓库 .git 失败: %v", err)
	}

	// 仓库根：dup 的远版本 + 只存在于根层的 only-far。
	srv7Write(t, filepath.Join(repo, ".claude", "agents", "dup.md"), srv7ClaudeBody(
		"name: dup\ndescription: 仓库根版本\n", "far"))
	srv7Write(t, filepath.Join(repo, ".claude", "agents", "only-far.md"), srv7ClaudeBody(
		"name: only-far\ndescription: 只在仓库根\n", "far"))

	// 中间层：dup 的中间版本（cwd 层也有 dup，所以中间层不该胜出）。
	srv7Write(t, filepath.Join(repo, "a", ".claude", "agents", "mid.md"), srv7ClaudeBody(
		"name: dup\ndescription: 中间层版本\n", "mid"))

	// cwd 层：dup 的近版本放在三层嵌套子目录里，证明递归扫描。
	cwd := filepath.Join(repo, "a", "b")
	srv7Write(t, filepath.Join(cwd, ".claude", "agents", "deep", "team", "near.md"), srv7ClaudeBody(
		"name: dup\ndescription: cwd 版本\n", "near"))
	srv7Write(t, filepath.Join(cwd, ".claude", "agents", "recursive.md"), srv7ClaudeBody(
		"name: recursive\ndescription: 递归读到的成员\n", "body"))

	// 同一层里两个文件同名：按完整路径字典序，aaa 在 zzz 前面。
	srv7Write(t, filepath.Join(cwd, ".claude", "agents", "aaa", "one.md"), srv7ClaudeBody(
		"name: same-layer\ndescription: 字典序在前\n", "first"))
	srv7Write(t, filepath.Join(cwd, ".claude", "agents", "zzz", "two.md"), srv7ClaudeBody(
		"name: same-layer\ndescription: 字典序在后\n", "second"))

	got := ScanClaudeAgents(cwd, nil)

	if role := got.Agents["dup"].Role; role != "cwd 版本" {
		t.Errorf("同名 agent 应当由离 cwd 最近的定义胜出，实际 role = %q", role)
	}
	if _, ok := got.Agents["recursive"]; !ok {
		t.Errorf("cwd 的 .claude/agents 子目录里的文件没被递归读到")
	}
	if _, ok := got.Agents["only-far"]; !ok {
		t.Errorf("仓库根层的非重名 agent 也应当被读到")
	}
	if role := got.Agents["same-layer"].Role; role != "字典序在前" {
		t.Errorf("同一层重名应当按文件路径字典序取第一个，实际 role = %q", role)
	}

	duplicateNotice := false
	for _, note := range got.Notices {
		if strings.Contains(note, "same-layer") && strings.Contains(note, "重复定义") {
			duplicateNotice = true
			break
		}
	}
	if !duplicateNotice {
		t.Errorf("同一层重名应当留一条提示，实际 %v", got.Notices)
	}
}

// TestSrv7ClaudeToolsPermissionMapping 是 §5.6：tools 含 Write/Edit/Bash 的
// 映射成 workspace，否则 read；覆盖逗号分隔、行内列表、缩进列表与省略四种写法。
func TestSrv7ClaudeToolsPermissionMapping(t *testing.T) {
	home := srv7ClaudeIsolateHome(t)
	dir := filepath.Join(home, ".claude", "agents")

	cases := []struct {
		name string
		fm   string
		want Permission
	}{
		{"csv-read", "tools: Read, Grep\n", PermissionRead},
		{"csv-edit", "tools: Read, Edit\n", PermissionWorkspace},
		{"csv-bash-scoped", "tools: Bash(git commit:*)\n", PermissionWorkspace},
		{"inline-write", "tools: [Read, Write]\n", PermissionWorkspace},
		{"inline-read", "tools: [Read, Grep]\n", PermissionRead},
		{"block-bash", "tools:\n  - Read\n  - Bash\n", PermissionWorkspace},
		{"block-read", "tools:\n  - Read\n  - Glob\n", PermissionRead},
		{"lowercase", "tools: read, edit\n", PermissionWorkspace},
		{"comment", "tools: Read, Write # 写盘\n", PermissionWorkspace},
		{"quoted-csv", "tools: \"Read, Edit\"\n", PermissionWorkspace},
		{"omitted", "", PermissionRead},
	}
	for _, tc := range cases {
		body := srv7ClaudeBody("name: "+tc.name+"\ndescription: 映射用例\n"+tc.fm, "正文")
		srv7Write(t, filepath.Join(dir, tc.name+".md"), body)
	}

	got := ScanClaudeAgents("", nil)
	for _, tc := range cases {
		agent, ok := got.Agents[tc.name]
		if !ok {
			t.Errorf("%s 没被读到；实际 %v", tc.name, srv7SortedClaudeIDs(got.Agents))
			continue
		}
		if agent.Permissions != tc.want {
			t.Errorf("%s 的 permissions = %q，期望 %q", tc.name, agent.Permissions, tc.want)
		}
	}
}
