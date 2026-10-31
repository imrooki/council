//go:build windows

package turn

// jobobject_windows.go 是 S3 的 Windows 兜底：桌面版被 TerminateProcess 硬杀、
// 甚至 Electron 自己崩掉时，Go 进程没有任何机会跑 defer 或杀 provider 进程树。
// 把每个 CLI 子进程挂进一个 JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE 的 Job Object，
// 最后一个句柄（父进程）消失时由内核带走整棵树，不依赖父进程执行清理代码。

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObject 持有 Job Object 的句柄。Close 之后内核会杀掉仍然属于这个 job
// 的所有进程（包括 CLI 留下的后台子进程）。
type jobObject struct {
	handle windows.Handle
}

func (j *jobObject) Close() {
	if j == nil || j.handle == 0 {
		return
	}
	_ = windows.CloseHandle(j.handle)
	j.handle = 0
}

// attachKillOnCloseJob 把 pid 挂进一个新 job。失败时返回错误，调用方按
// best-effort 继续跑：没有 Job Object 也不该让这一轮直接失败（例如
// AssignProcessToJobObject 在某些受限容器里会被拒绝）。
func attachKillOnCloseJob(pid int) (processJobCloser, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("attach job: invalid pid %d", pid)
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}
	closeOnError := func(err error) (processJobCloser, error) {
		_ = windows.CloseHandle(job)
		return nil, err
	}

	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return closeOnError(fmt.Errorf("SetInformationJobObject: %w", err))
	}

	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return closeOnError(fmt.Errorf("OpenProcess(%d): %w", pid, err))
	}
	defer windows.CloseHandle(proc)

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		return closeOnError(fmt.Errorf("AssignProcessToJobObject(%d): %w", pid, err))
	}
	return &jobObject{handle: job}, nil
}
