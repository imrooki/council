// council-launch 是桌面快捷方式的目标：双击才启动，不做服务、不开机自启。
//
// 为什么要单独一个 exe，而不是让快捷方式直接指向 council.exe：
// council.exe 是控制台程序，直接启动会先弹一个黑窗口。窗口要在创建时就隐藏，
// 而这只对「启动者」可见——快捷方式自己做不到。先试过 VBS，被 Windows Script
// Host 的编码问题绊住（无 BOM 的 UTF-8 会被按 ANSI 读，中文注释的奇数字节吃掉
// 行尾，报错位置还指在别的行）。用 Go 写十行代码就没有这层不确定性。
//
// 行为：
//   - 已有 council 实例在跑 → 只开一个应用窗口指过去，不再起第二个进程
//     （-rw 会撞 council.lock）；
//   - 否则隐藏启动 `council serve -rw -app`（或 -ro 时的 `serve -app`）。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	defaultExe = `%USERPROFILE%\bin\council.exe`
	dashboard  = "http://127.0.0.1:8765"
	// defaultCwd 是工作台启动时的当前目录。
	//
	// 不能让它沿用启动器所在目录：那样左下角的项目会显示成 bin，
	// 而工作台的目录 chip、会话归属、新建会话都跟着它走。
	defaultCwd = `<repo>`
)

func main() {
	mode := "rw"
	if len(os.Args) > 1 {
		mode = strings.ToLower(os.Args[1])
	}

	exe := os.Getenv("COUNCIL_EXE")
	if exe == "" {
		exe = defaultExe
	}
	if _, err := os.Stat(exe); err != nil {
		message("找不到 council.exe：\n" + exe)
		os.Exit(1)
	}

	if running(exe) {
		// 已有实例：只补一个窗口。用户在任务栏点回来时不该看到「又起了一个」。
		openWindow()
		return
	}

	args := []string{"serve", "-app"}
	if mode != "ro" {
		args = []string{"serve", "-rw", "-app"}
	}
	start(exe, args)
}

// running 用可执行文件路径匹配，而不是按进程名。
// 同名进程可能是别的目录里的旧构建，那种情况各起各的反而更糟。
func running(exe string) bool {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq council.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "council.exe")
}

// createNoWindow 是 CREATE_NO_WINDOW。
//
// 只设 HideWindow 在 Windows 11 上不够：控制台程序启动时会把控制台交给
// 「默认终端」（通常是 Windows Terminal），那一层握手会无视 SW_HIDE，
// 结果是弹出一个标签页。CREATE_NO_WINDOW 干脆不分配控制台，也就没有窗口可弹。
const createNoWindow = 0x08000000

// start 以无控制台方式启动子进程。
func start(exe string, args []string) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = workingDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		message("启动失败：" + err.Error())
		os.Exit(1)
	}
	// 不 Wait：启动器退出，serve 继续在后台跑。
	_ = cmd.Process.Release()
}

// workingDir 决定工作台把哪个目录当成项目。环境变量优先，
// 便于以后把快捷方式指向别的仓库而不用重新编译。
func workingDir() string {
	if dir := os.Getenv("COUNCIL_CWD"); dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	if st, err := os.Stat(defaultCwd); err == nil && st.IsDir() {
		return defaultCwd
	}
	return ""
}

// openWindow 复用 serve 已有的端口，只开一个没有地址栏的窗口。
func openWindow() {
	chrome := findChromium()
	if chrome == "" {
		message("找不到 Edge 或 Chrome，请手动打开 " + dashboard)
		return
	}
	cmd := exec.Command(chrome, "--app="+dashboard, "--window-size=1440,900",
		"--no-first-run", "--no-default-browser-check")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		message("打开窗口失败：" + err.Error())
		return
	}
	_ = cmd.Process.Release()
}

func findChromium() string {
	roots := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")}
	rels := []string{
		`Google\Chrome\Application\chrome.exe`,
		`Microsoft\Edge\Application\msedge.exe`,
	}
	for _, rel := range rels {
		for _, root := range roots {
			if root == "" {
				continue
			}
			p := filepath.Join(root, rel)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// message 用原生对话框而不是往 stderr 写：这个程序由双击启动，
// 没有控制台可看。
func message(text string) {
	_ = exec.Command("mshta",
		fmt.Sprintf(`javascript:alert('%s');close()`, strings.ReplaceAll(text, "'", `\'`))).Run()
}
