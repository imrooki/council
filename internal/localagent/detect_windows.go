//go:build windows

package localagent

// detect_windows.go 是 Windows 上唯一的进程枚举实现。
//
// 只调三类系统 API，别的一律不碰：
//   - CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS) + Process32First/Next：
//     拿可执行文件名、PID、父 PID；
//   - OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION) + GetProcessTimes：
//     拿启动时间；
//   - QueryFullProcessImageName：拿映像的完整路径。
//
// 路径只用于判断「装在哪个目录」（同名 exe 可能来自 Claude Desktop / IDE，
// 见 rules.go），不读它指向的文件内容。不读命令行（Toolhelp32 也拿不到，
// 要它得上 WMI / NtQueryInformationProcess，权限与风险都上一个量级）、
// 不读环境块、内存、窗口标题、剪贴板，不枚举模块，不执行任何程序
// （S-09 / S-07）。这里的每一个系统调用都是只读的。

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// enumerate 返回本机进程快照，按 Toolhelp32 的天然顺序（PID 升序附近）。
//
// 启动时间或映像路径拿不到时留零值/空串：受保护进程与跨会话进程经常
// 打不开，那是常态，不是错误。**不伪造**任何一项，也不因为一个进程打不开
// 就放弃整份快照。
func enumerate() ([]Process, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("枚举本机进程: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, fmt.Errorf("枚举本机进程: %w", err)
	}

	processes := make([]Process, 0, 256)
	for {
		startedAt, exePath := processDetails(entry.ProcessID)
		processes = append(processes, Process{
			PID:       int(entry.ProcessID),
			PPID:      int(entry.ParentProcessID),
			ExeName:   windows.UTF16ToString(entry.ExeFile[:]),
			ExePath:   exePath,
			StartedAt: startedAt,
		})
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, fmt.Errorf("枚举本机进程: %w", err)
		}
	}
	return processes, nil
}

// processDetails 打开一次进程句柄，取创建时间与映像完整路径。
//
// 两样都拿不到（拒绝访问、进程已退出）时返回零值/空串。PROCESS_QUERY_LIMITED_INFORMATION
// 是能拿到这两项的最小权限；系统进程与其它会话的进程仍然会拒绝，那是
// 「拿不到」，不是「不存在」—— 规则会把空路径降级成 probable。
func processDetails(pid uint32) (time.Time, string) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return time.Time{}, ""
	}
	defer windows.CloseHandle(handle)

	var startedAt time.Time
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err == nil {
		if nanos := creation.Nanoseconds(); nanos > 0 {
			// 0 是 1601-01-01 附近，不可能是真实的创建时间；按「拿不到」
			// 处理，免得界面上出现一个 1601 年的进程。
			startedAt = time.Unix(0, nanos).UTC()
		}
	}

	// MAX_LONG_PATH 是 32768：长路径安装（node_modules 深层目录）也要装得下。
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	var exePath string
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err == nil {
		exePath = windows.UTF16ToString(buf[:size])
	}
	return startedAt, exePath
}
