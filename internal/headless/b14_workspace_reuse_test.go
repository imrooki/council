package headless

// B14：追问会复用 convID，workspace 副本路径不能只按 convID 拼，
// 否则同一会话的第二次 -workspace 调用必定报「副本已存在」。

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func b14GitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v 失败: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestB14PrepareWorkspaceAllowsRepeatedConversation(t *testing.T) {
	repo := b14GitRepo(t)
	dataDir := t.TempDir()
	const convID = "conv-reused"

	first, err := prepareWorkspace(context.Background(), repo, dataDir, convID)
	if err != nil {
		t.Fatalf("第一次建副本失败: %v", err)
	}
	second, err := prepareWorkspace(context.Background(), repo, dataDir, convID)
	if err != nil {
		t.Fatalf("同一会话第二次建副本失败（B14）: %v", err)
	}
	if first == second {
		t.Fatalf("两次副本路径相同: %s", first)
	}
	base := filepath.Base(second)
	if !strings.HasPrefix(base, convID+"-") {
		t.Errorf("副本目录名 = %q，期望以 %q 开头", base, convID+"-")
	}
}
