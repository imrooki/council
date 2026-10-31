//go:build !windows

package store

// B4：Unix 的 flock 锁的是 inode。只锁 council.lock 时，删掉文件再新建一个
// 同名文件就能绕过排他锁；目录 inode 不会被这样替换。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestB4UnlinkLockFileDoesNotDropDataDirLock(t *testing.T) {
	dir := tstDataDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建 data_dir: %v", err)
	}
	first, err := acquireDataDirLock(dir)
	if err != nil {
		t.Fatalf("第一次取锁失败: %v", err)
	}
	defer func() {
		if err := first.release(); err != nil {
			t.Errorf("释放第一把锁失败: %v", err)
		}
	}()

	lockPath := filepath.Join(dir, lockFileName)
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("删除锁文件以模拟绕过: %v", err)
	}

	second, err := acquireDataDirLock(dir)
	if err == nil {
		_ = second.release()
		t.Fatal("删掉 council.lock 后仍能拿到第二把锁；目录锁没有生效（B4）")
	}
	if !errors.Is(err, ErrDataDirLocked) {
		t.Fatalf("第二把锁应报 ErrDataDirLocked，实际: %v", err)
	}
}
