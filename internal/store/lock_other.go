//go:build !windows

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// dataDirLock 持有非 Windows 平台上 data_dir 的排他锁。
//
// 锁的对象是 data_dir 目录本身，council.lock 只作为兼容旧二进制的第二道锁：
// flock(2) 锁的是 inode，删掉 council.lock 再新建一个同名文件就能绕过单文件
// 锁（B4）。目录 inode 不会被「删掉再建」替换，所以目录锁才是权威；保留
// 单文件锁是为了让还在跑旧版本的进程与新版互相看得见。
//
// 这一支在 Linux 上由 go test 覆盖；darwin / *bsd 的 flock 语义按 POSIX
// 对齐，但本项目没有真机验证。
type dataDirLock struct {
	dir  *os.File
	file *os.File
	path string
}

// acquireDataDirLock 先锁 dataDir 目录，再锁 council.lock。
//
// 两步都用 LOCK_EX|LOCK_NB，拿不到立刻报错，与 Windows 那支的
// LOCKFILE_FAIL_IMMEDIATELY 对齐。顺序不能反：旧实现只锁文件，先锁文件
// 会让「删文件再建」在第二步之前就绕过目录锁。
func acquireDataDirLock(dataDir string) (*dataDirLock, error) {
	path := filepath.Join(dataDir, lockFileName)

	dir, err := os.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("打开 data_dir %s: %w", dataDir, err)
	}
	if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = dir.Close()
		return nil, fmt.Errorf(
			"%w: data_dir %s 已被另一个进程锁定（council.lock）；同一个 data_dir 上只能同时跑一个 council 实例，"+
				"请先确认没有其它 council 进程还在使用这个数据目录再重试: %v",
			ErrDataDirLocked, dataDir, err,
		)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		_ = syscall.Flock(int(dir.Fd()), syscall.LOCK_UN)
		_ = dir.Close()
		return nil, fmt.Errorf("打开锁文件 %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		_ = syscall.Flock(int(dir.Fd()), syscall.LOCK_UN)
		_ = dir.Close()
		return nil, fmt.Errorf(
			"%w: 锁文件 %s 已被另一个进程持有；同一个 data_dir 上只能同时跑一个 council 实例，"+
				"请先确认没有其它 council 进程还在使用这个数据目录再重试: %v",
			ErrDataDirLocked, path, err,
		)
	}

	return &dataDirLock{dir: dir, file: f, path: path}, nil
}

// release 依次解锁文件与目录，再关闭句柄。幂等。
func (l *dataDirLock) release() error {
	if l == nil || (l.dir == nil && l.file == nil) {
		return nil
	}
	var firstErr error
	if l.file != nil {
		if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("解锁 %s: %w", l.path, err)
		}
		if err := l.file.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("关闭锁文件 %s: %w", l.path, err)
		}
		l.file = nil
	}
	if l.dir != nil {
		if err := syscall.Flock(int(l.dir.Fd()), syscall.LOCK_UN); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("解锁 data_dir: %w", err)
		}
		if err := l.dir.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("关闭 data_dir: %w", err)
		}
		l.dir = nil
	}
	return firstErr
}
