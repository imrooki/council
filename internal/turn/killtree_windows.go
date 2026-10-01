//go:build windows

package turn

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func prepareCmd(_ *exec.Cmd) {
	// Windows 不靠进程组：taskkill /T 自己会走子进程树。
	// 不在这里设 CREATE_NEW_PROCESS_GROUP —— 那会让 Ctrl+C 不再传到垫片，
	// 与 Command 被杀之后的兜底路径叠床架屋。
}

// taskkillPath 返回 killProcessTree 要起的 taskkill.exe 的完整路径（CL1，19d §7 (d)）。
// 包级变量只为测试：换成返回错误的函数，模拟找不到 taskkill（oby 的 L5b）；换成指向
// 假文件的函数，证明 killProcessTree 起的就是它返回的那个路径（clx 的正对照）。
var taskkillPath = systemTaskkillPath

// systemTaskkillPath 是系统目录（GetSystemDirectory，一般是 C:\Windows\System32）下的
// taskkill.exe。不按 PATH 找：PATH 里排在前面的目录（或当前目录）有一个叫 taskkill 的
// .exe、.cmd、.bat，按名字起就会执行它，而它拿到的是要杀的 pid。也不读 SystemRoot：
// 系统目录由系统调用给出，不随环境变量变，没有「SystemRoot 为空」这一种情形。系统
// 调用失败时返回错误，killProcessTree 照样不起任何程序，绝不退回按 PATH 找。
func systemTaskkillPath() (string, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "taskkill.exe"), nil
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	// /T 杀树，/F 强制。Windows 上 exec.CommandContext 杀掉的往往只是
	// claude.cmd / grok.cmd 那层垫片，真正干活的 node.exe 是它的子进程，
	// 会活着把这一轮写进 CLI 自己的会话文件（09 G 节第 7 行）。
	// 尽力而为：taskkill 失败不把 Spawn 改判成功，Runner 仍置 MaybeAdvanced。
	// 取不到 taskkill 的路径时直接返回错误、不起任何程序；三处调用方（spawn.go、
	// discovery_exec.go、loginstatus_exec.go）都不看返回值，接着调 cmd.Process.Kill。
	path, err := taskkillPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(path, "/T", "/F", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
