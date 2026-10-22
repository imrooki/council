package headless_test

// A7/A8/A9：-workspace 的 git worktree 副本。A8 的成员由测试二进制扮演，
// 按 COUNCIL_FAKE_WRITE_FILE 往自己的 cwd 写一个文件；cwd 是不是副本根，
// 看文件最后落在哪就知道。A9 同时钉角色文件不能落进副本。

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/headless"
)

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s 失败: %v；输出:\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// newTestGitRepo 建一个有一次提交的临时仓库。提交身份显式给，
// 不依赖本机 git config（这台机器没配 user.name / user.email）。
func newTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitTest(t, dir, "init")
	runGitTest(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	return dir
}

func TestAsk_workspace空目录退2(t *testing.T) {
	_, err := headless.Parse([]string{"-p", "q", "-workspace", ""}, nil)
	if err == nil {
		t.Fatal("-workspace 空串应当返回用法错误")
	}
	if got := headless.ExitCode(err); got != 2 {
		t.Errorf("-workspace 空串的退出码应为 2，实际 %d（err=%v）", got, err)
	}
}

// TestAsk_workspaceNonGit退2 A7：不是 git 仓库根是用法错误，退 2，
// 且不写任何结果到 stdout。
func TestAsk_workspaceNonGit退2(t *testing.T) {
	cfgPath, _ := writeAskConfig(t, "")
	notRepo := t.TempDir()
	out, errOut, err := runAskForTest(t, t.Context(), cfgPath,
		[]string{"-p", "q", "-members", "c1", "-workspace", notRepo, "-json"}, "")
	if err == nil {
		t.Fatalf("非 git 目录应当返回错误；Out:\n%s", out)
	}
	if got := headless.ExitCode(err); got != 2 {
		t.Errorf("非 git 目录的退出码应为 2，实际 %d（err=%v）", got, err)
	}
	if out != "" {
		t.Errorf("用法错误时 stdout 应为空，实际:\n%s", out)
	}
	if !strings.Contains(errOut, "不是 git 仓库根") || !strings.Contains(errOut, notRepo) {
		t.Errorf("stderr 应点名不是仓库根的目录，实际:\n%s", errOut)
	}
}

// TestAsk_workspace副本与diff A8/A9：成员在副本里建文件，原仓库干净，
// files/diff 带得出来；角色文件在 data_dir\workdir 下，不在副本里。
func TestAsk_workspace副本与diff(t *testing.T) {
	repo := newTestGitRepo(t)
	cfgPath, cfg := writeAskConfig(t, "")
	const memberFile = "created_by_member.txt"
	t.Setenv("COUNCIL_FAKE_WRITE_FILE", memberFile)

	out, errOut, err := runAskForTest(t, t.Context(), cfgPath,
		[]string{"-p", "q", "-members", "c1,cws", "-workspace", repo, "-json"}, "")
	if err != nil {
		t.Fatalf("workspace ask 失败: %v；Err:\n%s\nOut:\n%s", err, errOut, out)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "ok" {
		t.Errorf("status 应为 ok，实际 %q；Err:\n%s", res.Status, errOut)
	}
	if res.Workspace == nil {
		t.Fatalf("A8：-workspace 时 Result.Workspace 不应为 nil；Out:\n%s", out)
	}
	if !strings.Contains(errOut, "-workspace 为本次调用新建 worktree 副本") {
		t.Errorf("A8：stderr 应说明副本与冷启动，实际:\n%s", errOut)
	}
	if len(res.Turns) != 2 {
		t.Errorf("A8：read 与 workspace 两个成员都应发言，实际 turns=%d", len(res.Turns))
	}
	wantCopy := filepath.Join(cfg.DataDir, "workspaces", res.ConversationID)
	if filepath.Clean(res.Workspace.Copy) != filepath.Clean(wantCopy) {
		t.Errorf("A8：副本路径应为 %q，实际 %q", wantCopy, res.Workspace.Copy)
	}

	memberPath := filepath.Join(res.Workspace.Copy, memberFile)
	if _, statErr := os.Stat(memberPath); statErr != nil {
		t.Errorf("A8：副本里应有成员建的文件 %s，实际: %v", memberPath, statErr)
	}
	// 两个成员（read + workspace）都必须落在副本根。若 workspace 档还走
	// 旧的 agentWorkDir，文件会出现在 data_dir\workdir\<convID>\cws 下。
	for _, stale := range []string{
		filepath.Join(cfg.DataDir, "workdir", "c1", memberFile),
		filepath.Join(cfg.DataDir, "workdir", res.ConversationID, "cws", memberFile),
	} {
		if _, statErr := os.Stat(stale); !os.IsNotExist(statErr) {
			t.Errorf("A8：文件不应落在非副本目录 %s，实际 stat: %v", stale, statErr)
		}
	}

	status := runGitTest(t, repo, "status", "--short")
	if strings.TrimSpace(status) != "" {
		t.Errorf("A8：原仓库应保持干净，实际 git status --short:\n%s", status)
	}

	foundFile := false
	for _, f := range res.Workspace.Files {
		if f == memberFile {
			foundFile = true
			break
		}
	}
	if !foundFile {
		t.Errorf("A8：workspace.files 应含 %q，实际 %v", memberFile, res.Workspace.Files)
	}
	if !strings.Contains(res.Workspace.Diff, "+++") || !strings.Contains(res.Workspace.Diff, memberFile) {
		t.Errorf("A8：workspace.diff 应含 +++ 与 %q，实际:\n%s", memberFile, res.Workspace.Diff)
	}

	// A9：角色文件必须回到 data_dir\workdir\<agentID>，副本里任何位置
	// 都不许有。只看根目录会漏掉成员建子目录后写进去的那一份。
	var roleInCopy string
	walkErr := filepath.WalkDir(res.Workspace.Copy, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "council-role.txt" {
			roleInCopy = path
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("A9：遍历副本失败: %v", walkErr)
	}
	if roleInCopy != "" {
		t.Errorf("A9：副本里不应有 council-role.txt，实际出现在 %s", roleInCopy)
	}
	for _, agentID := range []string{"c1", "cws"} {
		rolePath := filepath.Join(cfg.DataDir, "workdir", agentID, "council-role.txt")
		if _, statErr := os.Stat(rolePath); statErr != nil {
			t.Errorf("A9：角色文件应在 %s，实际: %v", rolePath, statErr)
		}
	}
}

// TestAsk_workspace不报假漂移 复审第 1 条：workspace→workspace（副本被删、
// 同一 convID 重建）时，固定 Runner 算出的指纹不变，不能出现“身份已变”的
// 假警告。旧实现用普通 Runner 审，会在这里误报。
func TestAsk_workspace不报假漂移(t *testing.T) {
	repo := newTestGitRepo(t)
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, t.Context(), cfgPath,
		[]string{"-p", "first", "-members", "cws", "-workspace", repo, "-json"}, "")
	if err != nil {
		t.Fatalf("第一次 workspace ask 失败: %v；Err:\n%s", err, errOut)
	}
	first, _ := decodeSingleResult(t, out)
	if first.Workspace == nil {
		t.Fatalf("第一次 workspace ask 没有返回 Workspace")
	}

	// 模拟契约 §3.3 的副本被人工删掉：目录没了，worktree 元数据也 prune。
	if err := os.RemoveAll(first.Workspace.Copy); err != nil {
		t.Fatalf("删除副本失败: %v", err)
	}
	runGitTest(t, repo, "worktree", "prune")

	out, errOut, err = runAskForTest(t, t.Context(), cfgPath,
		[]string{"-p", "second", "-members", "cws", "-workspace", repo, "-conversation", first.ConversationID, "-json"}, "")
	if err != nil {
		t.Fatalf("第二次 workspace ask 失败: %v；Err:\n%s", err, errOut)
	}
	res, _ := decodeSingleResult(t, out)
	if res.Status != "ok" {
		t.Errorf("第二次 workspace 的 status 应为 ok，实际 %q；Err:\n%s", res.Status, errOut)
	}
	// 只认审计真正会打的短语。这里不能拿裸词「漂移」当判据：启动说明里带
	// 副本路径，而 t.TempDir() 的目录名包含本测试名（TestAsk_workspace不报假漂移），
	// 裸词会被自己的路径命中。
	for _, bad := range []string{"身份已变", "会各重建一次", "工作区漂移审计失败"} {
		if strings.Contains(errOut, bad) {
			t.Errorf("A9 假漂移：副本重建后不应出现 %q 警告，实际 stderr:\n%s", bad, errOut)
		}
	}
}

// TestAsk_workspace真漂移会告警 复审第 1 条的另一半：normal→workspace 时
// 普通 Runner 建下的会话指纹确实变了，针对性审计必须告警；旧实现用普通
// Runner 审反而会漏掉它。
func TestAsk_workspace真漂移会告警(t *testing.T) {
	repo := newTestGitRepo(t)
	cfgPath, _ := writeAskConfig(t, "")
	out, errOut, err := runAskForTest(t, t.Context(), cfgPath,
		[]string{"-p", "first", "-members", "cws", "-json"}, "")
	if err != nil {
		t.Fatalf("第一次普通 ask 失败: %v；Err:\n%s", err, errOut)
	}
	first, _ := decodeSingleResult(t, out)

	_, errOut, err = runAskForTest(t, t.Context(), cfgPath,
		[]string{"-p", "second", "-members", "cws", "-workspace", repo, "-conversation", first.ConversationID, "-json"}, "")
	if err != nil {
		t.Fatalf("切换到 workspace 的 ask 失败: %v；Err:\n%s", err, errOut)
	}
	if !strings.Contains(errOut, "身份已变") {
		t.Errorf("normal→workspace 应提示身份已变/重建，实际 stderr:\n%s", errOut)
	}
}
