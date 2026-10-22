package headless

// workspace.go 实现 -workspace 的 git worktree 副本：校验仓库根、建副本、
// 收 diff、用固定 Runner 做针对性漂移审计。它不清理副本（契约 §3.3）：
// 路径原样返回，由人决定 git apply 还是手动清。本文件不碰 store。

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"council/internal/repl"
	"council/internal/turn"
)

// maxWorkspaceDiffBytes 是契约 §2.2 的 1 MiB 上限。超过它不把整段 diff
// 塞进 JSON，改写进旁边一个文件，用 diff_path 指过去。
const maxWorkspaceDiffBytes = 1 << 20

// WorkspaceRepoRoot 是 workspaceRepoRoot 的导出版本，给 `council serve -rw`
// 在**建任务之前**做同一道校验用：工作台请求里 workspace=true 而 cwd 不是
// git 仓库根时，必须在 HTTP 层回 400，不能先开一个后台 job 再让 SSE 报错。
//
// 只导出这一道校验，不导出建副本：副本必须等会话 id 出来才能拼路径，
// 那是 headless.Run 内部的事；serve 若自己复制一份 prepareWorkspace，
// 两处对「同名副本已存在」「data_dir 为空」的处置早晚会漂。
func WorkspaceRepoRoot(ctx context.Context, dir string) (string, error) {
	return workspaceRepoRoot(ctx, dir)
}

// workspaceRepoRoot 校验 dir 是 git 仓库根，返回规范化后的根路径。
// rev-parse 的输出与入参都走 repl.NormalizeProjectDir：与 /resume 的项目
// 归属判断同一个口径，避免同一个仓库因盘符大小写或末尾分隔符被判成两个。
//
// 不是仓库根是用法的错（退 2）；git 本身找不到或 ctx 已结束是执行失败
// （退 1），不能假装成“路径打错了”。
func workspaceRepoRoot(ctx context.Context, dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", usagef("-workspace 需要一个目录")
	}
	out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("找不到 git 可执行文件: %w", err)
		}
		return "", usagef("-workspace %s 不是 git 仓库根: %v", dir, err)
	}
	top := repl.NormalizeProjectDir(strings.TrimSpace(string(out)))
	want := repl.NormalizeProjectDir(dir)
	if top == "" || want == "" || !repl.SameProjectDir(top, want) {
		return "", usagef("-workspace %s 不是 git 仓库根（git rev-parse 返回 %q）", dir, strings.TrimSpace(string(out)))
	}
	return top, nil
}

// prepareWorkspace 在 <dataDir>\workspaces\<convID> 建一份 HEAD 的分离
// worktree。路径已存在时先报错并让用户手动删：这里绝不 rm -rf 别人的
// 东西，也不猜那份目录是不是上一次的副本。
func prepareWorkspace(ctx context.Context, repoDir, dataDir, convID string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("data_dir 为空，无法放 workspace 副本")
	}
	if strings.TrimSpace(convID) == "" {
		return "", fmt.Errorf("会话 id 为空，无法放 workspace 副本")
	}
	workspacesDir := filepath.Join(dataDir, "workspaces")
	copyDir := filepath.Join(workspacesDir, convID)
	if _, err := os.Lstat(copyDir); err == nil {
		return "", fmt.Errorf("workspace 副本已存在: %s；确认不再需要后手动删掉再重试", copyDir)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("检查 workspace 副本路径 %s: %w", copyDir, err)
	}
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		return "", fmt.Errorf("建 workspace 目录 %s: %w", workspacesDir, err)
	}
	if _, err := runGit(ctx, repoDir, "worktree", "add", "--detach", copyDir, "HEAD"); err != nil {
		// runGit 已把 git 的 stderr 与完整参数拼进 error；这里不再重复
		// “git worktree add”前缀。
		return "", err
	}
	return copyDir, nil
}

// collectDiff 在副本里把全部改动（含未跟踪文件）先 add -A，再取
// diff --cached 与 status --porcelain。不 apply、不 reset；副本原样留着。
func collectDiff(ctx context.Context, copyDir string) (string, []string, error) {
	if _, err := runGit(ctx, copyDir, "add", "-A"); err != nil {
		return "", nil, err
	}
	diff, err := runGit(ctx, copyDir, "diff", "--cached")
	if err != nil {
		return "", nil, err
	}
	status, err := runGit(ctx, copyDir, "status", "--porcelain")
	if err != nil {
		return "", nil, err
	}
	return string(diff), parseStatusPaths(status), nil
}

// parseStatusPaths 取 porcelain 输出里的路径列。第一列两个字节是状态，
// 第三列是空格；重命名/复制行是 "旧 -> 新"，取新路径。空切片而不是 nil：
// JSON 里 files 是数组，不是 null。
func parseStatusPaths(out []byte) []string {
	files := make([]string, 0)
	text := strings.ReplaceAll(string(out), "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = strings.TrimSpace(path[i+4:])
		}
		if path == "" {
			continue
		}
		files = append(files, path)
	}
	return files
}

// collectWorkspace 把副本里的改动收成 Result.Workspace。collectDiff 用
// 一个不跟随 runCtx 的 30 秒 ctx：整体超时/取消只该停成员进程，不该让
// 已经落盘的文件从结果里消失；30 秒是给 git 在异常仓库上的上界。
func (s *askState) collectWorkspace(runCtx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(runCtx), 30*time.Second)
	defer cancel()
	diff, files, err := collectDiff(ctx, s.copyDir)
	if err != nil {
		return fmt.Errorf("收集 workspace 改动失败: %w", err)
	}
	if len(diff) > maxWorkspaceDiffBytes {
		path := filepath.Join(s.deps.DataDir, "workspaces", s.convID+".diff")
		if err := os.WriteFile(path, []byte(diff), 0o644); err != nil {
			return fmt.Errorf("写 workspace diff 文件 %s: %w", path, err)
		}
		s.workspace = &WorkspaceResult{
			Copy:          s.copyDir,
			Files:         files,
			DiffTruncated: true,
			DiffPath:      path,
		}
		return nil
	}
	s.workspace = &WorkspaceResult{Copy: s.copyDir, Diff: diff, Files: files}
	return nil
}

// auditWorkspaceDrift 是 -workspace 模式下的针对性启动审计。全局 AuditDrift
// 在 app.Run 里用普通 Runner 算 agentWorkDir 的指纹，固定副本下那既会漏掉
// normal→workspace 的重建，也会把 workspace→workspace 的相同指纹误报成
// 漂移；这里改用已经构造好的 fixed Runner，只保留本次 convID 的行。
//
// 审计是纯读，失败只记一行、不阻止执行：少一句提醒不该让一次已经付过
// 副本代价的调用直接失败。
func (s *askState) auditWorkspaceDrift(ctx context.Context) {
	rows, err := s.rn.AuditDrift(ctx)
	if err != nil {
		fmt.Fprintf(s.errOut, "council: 工作区漂移审计失败: %v\n", err)
		return
	}
	// 只报这次真的会跑的成员：会话里还留着别的成员的 provider session 时，
	// 它们的指纹变化与本次调用无关，报出来就是一笔并不存在的入场费。
	// 汇总成员也算「这次会跑」，所以一起放进来。
	inRun := make(map[string]struct{}, len(s.members)+1)
	for _, id := range s.members {
		inRun[id] = struct{}{}
	}
	if s.summaryID != "" {
		inRun[s.summaryID] = struct{}{}
	}
	mine := make([]turn.DriftRow, 0, len(rows))
	for _, row := range rows {
		if row.ConvID != s.convID {
			continue
		}
		if _, ok := inRun[row.AgentID]; !ok {
			continue
		}
		mine = append(mine, row)
	}
	if len(mine) == 0 {
		return
	}
	n1, n2 := 0, 0
	for _, row := range mine {
		if row.AskRefused() {
			n2++
		} else {
			n1++
		}
	}
	if n1 > 0 {
		fmt.Fprintf(s.errOut, "council: 注意: 这条会话有 %d 个 provider 会话的身份已变，本次会各重建一次（每条一次入场费）\n", n1)
	}
	if n2 > 0 {
		fmt.Fprintf(s.errOut, "council: 注意: 这条会话有 %d 个 provider 会话按当前配置会被拒，不花钱\n", n2)
	}
	for _, row := range mine {
		fmt.Fprintf(s.errOut, "  成员 %s: %s\n", row.AgentID, row.Reason)
	}
}

// runGit 起一条 git 命令，成功只回 stdout，失败把 stderr 拼进 error。
// 不能 CombinedOutput：worktree add 成功时也会往 stderr 写
// “Preparing worktree”，混进来会被 workspaceRepoRoot 当成 rev-parse
// 的返回路径、被 collectDiff 当成 status 路径。调用方不再重复拼
// "git add -A: git add -A" 这类前缀。
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return []byte(stdout.String()), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return []byte(stdout.String()), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return []byte(stdout.String()), nil
}
