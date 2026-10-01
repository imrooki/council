//go:build !windows

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// dataDirLock 持有非 Windows 平台上 <data_dir>/council.lock 的排他文件锁。
//
// 这一支用的是 flock(2)（LOCK_EX | LOCK_NB），在 linux / darwin / *bsd 这些
// syscall 包提供 Flock 的平台上是真实生效的，不是留空的占位实现：
// 第二个进程在同一个 data_dir 上调用 Open 会立即拿不到锁、报错退出。
//
// 但本项目两台开发机都是 Windows（COMPUTERNAME = LL 与 LONG），这一支只做过
// go vet 与交叉编译层面的类型检查，没有跑过真机验证——不确定的是 flock 在这些
// 平台上、对异常终止（kill -9 等价物）之后的释放时机是否与 Windows 那支的实测
// 结论一致，未验证之前不要假定它完全等价。见任务书要求：不许让人以为跨平台
// 都同等程度地守得住。
type dataDirLock struct {
	file *os.File
	path string
}

// acquireDataDirLock 打开（必要时创建）dataDir 下的 council.lock，
// 用 flock(LOCK_EX|LOCK_NB) 立即取排他锁，语义与 Windows 那支的
// LOCKFILE_FAIL_IMMEDIATELY 对齐：拿不到就立刻报错，不阻塞等待。
func acquireDataDirLock(dataDir string) (*dataDirLock, error) {
	path := filepath.Join(dataDir, lockFileName)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件 %s: %w", path, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf(
			"%w: 锁文件 %s 已被另一个进程持有；同一个 data_dir 上只能同时跑一个 council 实例，"+
				"请先确认没有其它 council 进程还在使用这个数据目录再重试: %v",
			ErrDataDirLocked, path, err,
		)
	}

	return &dataDirLock{file: f, path: path}, nil
}

// release 解锁并关闭文件句柄。幂等：重复调用直接返回 nil。
// 锁的归属会从"本次 Open 调用"转交给"构造出来的 Store"，转交之后
// Store.Close 才是真正负责释放的一方，见 open.go 里 lockOwned 那段注释。
func (l *dataDirLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil {
		return fmt.Errorf("解锁 %s: %w", l.path, unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭锁文件 %s: %w", l.path, closeErr)
	}
	return nil
}
