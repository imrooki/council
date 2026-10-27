// council-launch 是桌面快捷方式的目标：双击才启动，不做服务、不开机自启。
//
// 为什么要单独一个 exe，而不是让快捷方式直接指向 council.exe：
// council.exe 是控制台程序，直接启动会先弹一个黑窗口。窗口要在创建时就隐藏，
// 而这只对「启动者」可见——快捷方式自己做不到。先试过 VBS，被 Windows Script
// Host 的编码问题绊住（无 BOM 的 UTF-8 会被按 ANSI 读，中文注释的奇数字节吃掉
// 行尾，报错位置还指在别的行）。用 Go 写十行代码就没有这层不确定性。
//
// 用法：council-launch [rw|ro] [工作目录]
//   rw（默认） serve -rw -app
//   ro         serve -app
//   工作目录   不给就用 COUNCIL_CWD，再不行用启动器自己所在的目录。
//
// 刻意不写死任何绝对路径：安装位置因人而异，写死了既泄露本机用户名，
// 也让别人克隆之后直接不能用。要找的 exe 按 COUNCIL_EXE → 同目录 → PATH 查找。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const dashboard = "http://127.0.0.1:8765"

// createNoWindow 是 CREATE_NO_WINDOW。
//
// 只设 HideWindow 在 Windows 11 上不够：控制台程序启动时会把控制台交给
// 「默认终端」（通常是 Windows Terminal），那一层握手会无视 SW_HIDE，
// 结果是弹出一个标签页。CREATE_NO_WINDOW 干脆不分配控制台，也就没有窗口可弹。
const createNoWindow = 0x08000000

func main() {
	mode := "rw"
	if len(os.Args) > 1 {
		mode = strings.ToLower(os.Args[1])
	}

	exe, err := resolveExe()
	if err != nil {
		message("找不到 council.exe：\n" + err.Error())
		os.Exit(1)
	}

	if running() {
		// 已有实例：只补一个窗口。用户在任务栏点回来时不该看到「又起了一个」。
		openWindow()
		return
	}

	args := []string{"serve", "-app"}
	if mode != "ro" {
		args = []string{"serve", "-rw", "-app"}
	}
	start(exe, args, workingDir())
}

// resolveExe 依次尝试：环境变量、与本启动器同目录、PATH。
func resolveExe() (string, error) {
	if p := os.Getenv("COUNCIL_EXE"); p != "" {
		return p, checkFile(p)
	}
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), "council.exe")
		if checkFile(p) == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("council.exe"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("已尝试 COUNCIL_EXE、启动器同目录、PATH")
}

func checkFile(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%s 是目录", p)
	}
	return nil
}

// workingDir 决定工作台把哪个目录当成项目。
// 优先级：命令行第二个参数 → COUNCIL_CWD → 启动器所在目录。
func workingDir() string {
	if len(os.Args) > 2 {
		if dir := os.Args[2]; isDir(dir) {
			return dir
		}
	}
	if dir := os.Getenv("COUNCIL_CWD"); isDir(dir) {
		return dir
	}
	if self, err := os.Executable(); err == nil {
		return filepath.Dir(self)
	}
	return ""
}

func isDir(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// running 用进程名判断。同名但不同路径的旧构建也算「在跑」——
// 那台机器上本来就已经占着 8765，再起一个只会撞端口。
func running() bool {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq council.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "council.exe")
}

// start 以无控制台方式启动子进程。
func start(exe string, args []string, dir string) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		message("启动失败：" + err.Error())
		os.Exit(1)
	}
	// 不 Wait：启动器退出，serve 继续在后台跑。
	_ = cmd.Process.Release()
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
			if checkFile(p) == nil {
				return p
			}
		}
	}
	return ""
}

// message 用原生对话框而不是往 stderr 写：这个程序由双击启动，没有控制台可看。
func message(text string) {
	_ = exec.Command("mshta",
		fmt.Sprintf(`javascript:alert('%s');close()`, strings.ReplaceAll(text, "'", `\'`))).Run()
}
