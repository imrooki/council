package main

// SRV4 §6.1 的退出码证明：同一个 data_dir 已经被一个进程拿着锁时，
// `council serve -rw` 必须退 1（不是 2、也不是 0），错误里要出现
// 「另一个 council 正在用这个 data_dir」与「关掉它再试」两句话。
//
// 这里从 run() 进而不是直接调 serve.Run：任务书说的是「退 1」，
// 退出码映射发生在 main.go 的 runServe，只有这一层能证明它确实是 1。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/store"
)

func TestServeRWFailsWhenDataDirLocked(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")

	// 先让一个 Store 拿住 data_dir 的排他锁，扮演「另一个 council」。
	// 同一测试进程里的第二个文件句柄也会被 Windows 的 LockFileEx /
	// Unix 的 flock 挡住（internal/store/lock_test.go 已钉住这一点）。
	holder, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("准备持锁的 Store 失败: %v", err)
	}
	defer func() {
		if err := holder.Close(); err != nil {
			t.Errorf("关闭持锁的 Store 失败: %v", err)
		}
	}()

	cfgPath := filepath.Join(dir, "config.toml")
	body := fmt.Sprintf("data_dir = '%s'\n", dataDir)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	// 规则文件默认在用户主目录；不隔离的话这台机器上的 COUNCIL.md
	// 会进身份指纹，测试结果依赖开发机环境。
	t.Setenv("COUNCIL_GLOBAL_RULES", filepath.Join(dir, "no-global-rules.md"))

	_, stderr, code := captureRun(t, func() int {
		return run([]string{"serve", "-rw", "-addr", "127.0.0.1:0", "-config", cfgPath})
	})
	if code != exitConfigFailed {
		t.Fatalf("serve -rw 拿不到锁时退出码 = %d，期望 %d；stderr:\n%s", code, exitConfigFailed, stderr)
	}
	for _, phrase := range []string{"另一个 council 正在用这个 data_dir", "关掉它再试"} {
		if !strings.Contains(stderr, phrase) {
			t.Errorf("stderr 缺少关键短语 %q，实际:\n%s", phrase, stderr)
		}
	}
}
