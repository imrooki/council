//go:build windows

package store

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// dataDirLock 持有 Windows 上 <data_dir>/council.lock 的排他文件锁。
//
// 这一支是真实生效的：LockFileEx 加 LOCKFILE_EXCLUSIVE_LOCK 是操作系统级的字节范围锁，
// 由内核维护，不依赖任何用户态协议；进程异常终止（含 taskkill /T /F，本项目把它当
// 常规路径，见 internal/turn/killtree_windows.go）时，内核会连带关闭该进程持有的全部
// 文件句柄，锁随之释放，不需要谁去手工清理一个 pid 文件。
type dataDirLock struct {
	handle windows.Handle
	path   string
}

// acquireDataDirLock 在 dataDir 下打开（必要时创建）council.lock，
// 用 LockFileEx 配 LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY 立即取排他锁。
//
// 用 FAIL_IMMEDIATELY 而不是阻塞等待：这把锁要防的是「第二个 council 悄悄打开
// 同一个 data_dir」（未决 #8 的实测：静默打开、静默写入、退出码 0，付两次入场费、
// 留一条谁也找不到的孤儿 provider 会话、游标错位），用户要的是一句立刻说清楚的话
// ——「这个 data_dir 已被占用」——而不是被卡住等一个永远不会来的锁。
//
// 锁的是 council.lock 这个专用文件，绝不是 council.db 本身：本包测试里有多处
// 在 Open 之前或期间用自己的连接直接打开同一个库文件（schema_v2_test.go /
// schema_v3_test.go 的造库辅助、open_test.go 的 tstRawDB），锁库文件会把这些
// 合法用法一起挡住。
func acquireDataDirLock(dataDir string) (*dataDirLock, error) {
	path := filepath.Join(dataDir, lockFileName)

	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("编码锁文件路径 %s: %w", path, err)
	}

	// GENERIC_READ|GENERIC_WRITE 加 FILE_SHARE_READ|FILE_SHARE_WRITE：
	// 允许别的进程照常打开这个文件（否则第二个进程连"看到自己被锁住"这件事都做不到，
	// CreateFile 会先于 LockFileEx 报错，错误信息就说不清是谁占用的），
	// 真正的排他性由下面的 LockFileEx 一个字节范围锁来保证。
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件 %s: %w", path, err)
	}

	// 锁文件本身不存内容，只借用它的字节范围锁语义：锁定第 0 字节的 1 字节即可。
	var ol windows.Overlapped
	lockErr := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1, 0,
		&ol,
	)
	if lockErr != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf(
			"%w: 锁文件 %s 已被另一个进程持有；同一个 data_dir 上只能同时跑一个 council 实例，"+
				"请先确认没有其它 council 进程还在使用这个数据目录再重试: %v",
			ErrDataDirLocked, path, lockErr,
		)
	}

	return &dataDirLock{handle: handle, path: path}, nil
}

// release 解锁并关闭句柄。幂等：重复调用（包括对同一把锁调用两次）直接返回 nil，
// 不会因为 handle 已经关过而报错——open.go 里锁的归属会从"本次 Open 调用"转交给
// "构造出来的 Store"，转交之后 Store.Close 才是真正负责释放的一方。
func (l *dataDirLock) release() error {
	if l == nil || l.handle == 0 {
		return nil
	}
	var ol windows.Overlapped
	unlockErr := windows.UnlockFileEx(l.handle, 0, 1, 0, &ol)
	closeErr := windows.CloseHandle(l.handle)
	l.handle = 0
	if unlockErr != nil {
		return fmt.Errorf("解锁 %s: %w", l.path, unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭锁文件句柄 %s: %w", l.path, closeErr)
	}
	return nil
}
