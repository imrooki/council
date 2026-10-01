//go:build !windows

package turn

import (
	"errors"
	"os/exec"
	"syscall"
)

func prepareCmd(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// taskkillPath 只在 Windows 上有用（killtree_windows.go 的 killProcessTree 调它）。这里留一个
// 同名变量，只为让没有构建约束的测试文件（oby_loginstatus_exec_test.go 的 L5b 换掉它）在别的
// 平台也编译得过；下面的 killProcessTree 不调它。
var taskkillPath = func() (string, error) {
	return "", errors.New("taskkill is windows-only")
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	// 负 pid：向 prepareCmd 建的那个进程组发 SIGKILL，连孩子一起带走。
	return syscall.Kill(-pid, syscall.SIGKILL)
}
