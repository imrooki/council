//go:build linux

package turn

// spawn_waitdelay_linux_test.go 钉住 S4：CLI 留下后台子进程继承 stdout 管道时，
// cmd.Wait 不能把整轮拖到那个子进程结束。Linux 才有 /bin/sh 与 setsid，所以
// 这些用例只在这一个平台跑；Windows 上对应的是 .cmd 垫片，不能这样造。

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"council/internal/provider"
)

// TestSpawnProcessReturnsAfterChildExitsWithHangingGrandchild 覆盖同进程组：
// shell 正常 exit 0，但 `sleep` 继承了 stdout。没有 WaitDelay 时 Wait 要等
// sleep 结束；有 WaitDelay 时主进程退出后最多 2 秒就收尾，且退出码仍是 0。
func TestSpawnProcessReturnsAfterChildExitsWithHangingGrandchild(t *testing.T) {
	dir := t.TempDir()
	started := time.Now()
	stdout, stderr, code, killed, err := spawnProcess(context.Background(), provider.Command{
		Path: "/bin/sh",
		Args: []string{"-c", "sleep 6 & echo done"},
		Dir:  dir,
	})
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("spawnProcess: %v；stderr=%s", err, stderr)
	}
	if killed {
		t.Error("killed = true，CLI 自己 exit 0，不该按超时处理")
	}
	if code != 0 {
		t.Errorf("exitCode = %d，期望 0", code)
	}
	if !strings.Contains(string(stdout), "done") {
		t.Errorf("stdout = %q，期望包含 done", stdout)
	}
	// 修复前是 6 秒；修复后是主进程退出 + WaitDelay=2 秒。
	if elapsed > 4*time.Second {
		t.Errorf("耗时 %v，超过 4s：Wait 还在等后台子进程关管道", elapsed)
	}
}

// TestSpawnProcessTimeoutIsBoundedWithEscapedGrandchild 覆盖 setsid 逃出进程组：
// 直接子进程被杀之后，逃出去的 sleep 仍占着管道。超时分支不能无限等 Wait。
func TestSpawnProcessTimeoutIsBoundedWithEscapedGrandchild(t *testing.T) {
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("本机没有 setsid，跳过逃出进程组的用例")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "escaped.pid")
	script := fmt.Sprintf("setsid sleep 8 & echo $! > %q; sleep 8", pidFile)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, _, killed, err := spawnProcess(ctx, provider.Command{
		Path: "/bin/sh",
		Args: []string{"-c", script},
		Dir:  dir,
	})
	elapsed := time.Since(started)

	if !killed {
		t.Error("killed = false，超时分支必须标成 killed")
	}
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("err = %v，期望 context deadline exceeded", err)
	}
	if elapsed > 6*time.Second {
		t.Errorf("耗时 %v，超过 6s：超时分支还在无限等 Wait", elapsed)
	}

	// 逃出去的 sleep 不会自己停，测试收尾必须带走它。
	if raw, readErr := os.ReadFile(pidFile); readErr == nil {
		if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil && pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}
