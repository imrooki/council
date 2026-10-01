//go:build windows

package turn

// clx_killtree_windows_test.go 是 CL1（一批小修补，契约 §2.3）第 6 条在 turn 包的验收：killProcessTree 只起
// 系统目录里的 taskkill.exe，不按 PATH 找（19d §7 (d)）；取不到路径时返回错误、什么都不起，绝不退回按 PATH 找。
// 装置：t.TempDir() 里一个假的 taskkill.cmd（只把收到的参数写进同目录的标记文件、以 0 退出，不杀任何进程），
// 它的目录排在 PATH 最前；C8 另用 Go 现编一个假的 taskkill.exe（同样只写标记文件）排在 PATH 最前。被杀的是测试
// 自己起的一个 cmd 循环（obx_loginstatus_exec_test.go 的 obxStatusFake、obxLoopLines，只读复用）。真 taskkill
// 只会对这个循环的 pid 起。每条用例结束时（t.Cleanup）删掉循环的开关文件、Kill 并等它退出，不留残留 cmd.exe。
// 本包的测试没有 t.Parallel，换包级变量 taskkillPath 不会与别的用例并发。

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// clxKillWait 是等循环退出的上限：真 taskkill 杀掉它通常不到 1 s。
const clxKillWait = 10 * time.Second

// clxFake 是一套装置：假 taskkill 的路径与标记文件、被杀的循环与它的退出信号。
type clxFake struct {
	path   string
	marker string
	pid    int
	done   chan struct{} // 循环退出（cmd.Wait 返回）时关闭
}

// clxWriteFakeCmd 在 dir 里写一个名叫 name 的假 taskkill（.cmd，只把收到的参数写进同目录的 clx-marker.txt、
// 以 0 退出），返回它的路径与标记文件的路径。
func clxWriteFakeCmd(t *testing.T, dir, name string) (path, marker string) {
	t.Helper()
	path, marker = filepath.Join(dir, name), filepath.Join(dir, "clx-marker.txt")
	script := strings.Join([]string{"@echo off", `>"%~dp0clx-marker.txt" echo %*`, "exit /b 0"}, "\r\n") + "\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatalf("写假 taskkill %s: %v", name, err)
	}
	return path, marker
}

// clxPrependPath 把 dir 排在 PATH 最前，再核前提：此时 exec.LookPath("taskkill") 解析到的就是 want
// （按名字找就会执行它），否则判前提不成立。
func clxPrependPath(t *testing.T, dir, want string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got, err := exec.LookPath("taskkill"); err != nil || !strings.EqualFold(got, want) {
		t.Fatalf("前提不成立：PATH 最前放了假的 taskkill，exec.LookPath(\"taskkill\") 应当解析到 %s，实际 %q（err %v）", want, got, err)
	}
}

// clxStartLoop 起一个会一直等的 cmd 循环，返回它的 pid 与退出信号；用例结束时删开关文件、Kill 并等它退出。
func clxStartLoop(t *testing.T) (int, chan struct{}) {
	t.Helper()
	on := filepath.Join(t.TempDir(), "loop.on")
	if err := os.WriteFile(on, []byte("x"), 0o644); err != nil {
		t.Fatalf("写 loop.on: %v", err)
	}
	p := obxStatusFake(t, obxLoopLines(`exist "`+on+`"`)...)
	cmd := exec.Command(p)
	if err := cmd.Start(); err != nil {
		t.Fatalf("起循环: %v", err)
	}
	pid := cmd.Process.Pid
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = os.Remove(on)
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(clxKillWait):
			t.Errorf("清理：循环（pid %d）在 %v 内没有退出", pid, clxKillWait)
		}
	})
	return pid, done
}

// clxNewFake 写假的 taskkill.cmd、把它的目录排在 PATH 最前（核 LookPath 的前提），再起一个会一直等的 cmd 循环。
func clxNewFake(t *testing.T) *clxFake {
	t.Helper()
	dir := t.TempDir()
	f := &clxFake{}
	f.path, f.marker = clxWriteFakeCmd(t, dir, "taskkill.cmd")
	clxPrependPath(t, dir, f.path)
	f.pid, f.done = clxStartLoop(t)
	return f
}

// clxMarkerExists 报告标记文件 marker 在不在，即写它的假 taskkill 有没有被执行过。
func clxMarkerExists(t *testing.T, marker string) bool {
	t.Helper()
	_, err := os.Stat(marker)
	if err == nil {
		return true
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("查标记文件: %v", err)
	}
	return false
}

// clxSetSeam 把 taskkillPath 换成 fn，用例结束时还原。
func clxSetSeam(t *testing.T, fn func() (string, error)) {
	t.Helper()
	old := taskkillPath
	t.Cleanup(func() { taskkillPath = old })
	taskkillPath = fn
}

// clxWaitDone 等循环退出，最多 clxKillWait。
func clxWaitDone(t *testing.T, pid int, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(clxKillWait):
		t.Errorf("该杀的循环（pid %d）在 %v 内没有退出：killProcessTree 没有起真的 taskkill", pid, clxKillWait)
	}
}

// TestClxKillTreeIgnoresPathTaskkill 是 C4：假的 taskkill.cmd 排在 PATH 最前，killProcessTree 不执行它，
// 而且该杀的循环真被杀掉了（起的是系统目录里的 taskkill.exe）。
// 【判别力】V8（按名字起 taskkill，HEAD 的写法）：执行了假的，标记出现、循环没死，两处红；V9（systemTaskkillPath
// 按 PATH 找）同样两处红；V12（接缝的默认值换成按 PATH 找）同样。同装置的正对照是 TestClxKillTreeUsesSeam 第 1 步：
// 接缝指向这个假文件时标记出现，证明标记机制有效、「标记不存在」不是假文件没法执行。
func TestClxKillTreeIgnoresPathTaskkill(t *testing.T) {
	f := clxNewFake(t)
	if err := killProcessTree(f.pid); err != nil {
		t.Errorf("killProcessTree 应当返回 nil（系统目录里的 taskkill 杀掉了循环），实际 %v", err)
	}
	clxWaitDone(t, f.pid, f.done)
	if clxMarkerExists(t, f.marker) {
		t.Errorf("killProcessTree 执行了 PATH 最前的假 taskkill（标记文件 %s 出现了）", f.marker)
	}
}

// TestClxKillTreeUsesSeam 是 C5：同一装置，killProcessTree 起的就是接缝返回的那个路径，参数是 /T /F /PID <pid>。
//   - 第 1 步：接缝指向 PATH 最前的那个假 taskkill.cmd（TestClxKillTreeIgnoresPathTaskkill、
//     TestClxKillTreeSeamErrorNoFallback 的正对照：同一个文件被执行时标记出现）。
//   - 第 2 步：接缝指向另一个 t.TempDir() 里、名字不叫 taskkill 的假 clx-fake-tk.cmd（这个目录不进 PATH），
//     证明起的是完整路径本身，不是去掉目录之后再按名字找。
//
// 【判别力】V11（killProcessTree 不经接缝、直接调 systemTaskkillPath）：起的是真 taskkill，两步的标记都不出现，红；
// X1（按 filepath.Base(path) 起，即去掉目录再按 PATH 找）：第 1 步照绿（名字与目录都在 PATH 上），第 2 步按名字
// 找不到 clx-fake-tk.cmd，返回错误、标记不出现，红。
func TestClxKillTreeUsesSeam(t *testing.T) {
	f := clxNewFake(t)
	want := "/T /F /PID " + strconv.Itoa(f.pid) + "\r\n"

	clxSetSeam(t, func() (string, error) { return f.path, nil })
	if err := killProcessTree(f.pid); err != nil {
		t.Errorf("第 1 步：假 taskkill 以 0 退出，killProcessTree 应当返回 nil，实际 %v", err)
	}
	if b, err := os.ReadFile(f.marker); err != nil {
		t.Errorf("第 1 步：接缝指向 PATH 上的假 taskkill 时它应当被执行、写出标记文件，实际读不到：%v", err)
	} else if got := string(b); got != want {
		t.Errorf("第 1 步：假 taskkill 收到的参数应当是 %q，实际 %q", want, got)
	}

	// 第 2 步另起一个循环：变异下第 1 步可能已经起了真 taskkill、杀掉了 f.pid，这一步若再起真 taskkill，
	// 对象仍是测试自己起的、活着的进程，不会是一个已经退出、可能被系统复用的 pid。
	pid2, _ := clxStartLoop(t)
	want2 := "/T /F /PID " + strconv.Itoa(pid2) + "\r\n"
	off, offMarker := clxWriteFakeCmd(t, t.TempDir(), "clx-fake-tk.cmd")
	if _, err := exec.LookPath("clx-fake-tk"); err == nil {
		t.Fatalf("前提不成立：clx-fake-tk.cmd 的目录不在 PATH 上，按名字却找得到它")
	}
	clxSetSeam(t, func() (string, error) { return off, nil })
	if err := killProcessTree(pid2); err != nil {
		t.Errorf("第 2 步：假 clx-fake-tk.cmd 以 0 退出，killProcessTree 应当返回 nil，实际 %v", err)
	}
	if b, err := os.ReadFile(offMarker); err != nil {
		t.Errorf("第 2 步：接缝指向不在 PATH 上的 %s 时它应当被执行、写出标记文件，实际读不到：%v", off, err)
	} else if got := string(b); got != want2 {
		t.Errorf("第 2 步：假 clx-fake-tk.cmd 收到的参数应当是 %q，实际 %q", want2, got)
	}
}

// TestClxKillTreeSeamErrorNoFallback 是 C6：同一装置，接缝返回错误时 killProcessTree 原样返回这个错误、
// 什么都不起，不退回按 PATH 找（假的 taskkill.cmd 就在 PATH 最前）。
// 【判别力】V10（取不到路径时退回按名字起 taskkill）：执行了假的，标记出现，返回值也不再是那个错误，红；
// V11 下起的是真 taskkill、返回 nil，红。正对照同 TestClxKillTreeUsesSeam 第 1 步（标记机制有效）。
func TestClxKillTreeSeamErrorNoFallback(t *testing.T) {
	f := clxNewFake(t)
	errNo := errors.New("clx: no taskkill")
	clxSetSeam(t, func() (string, error) { return "", errNo })
	if err := killProcessTree(f.pid); !errors.Is(err, errNo) {
		t.Errorf("接缝返回错误时 killProcessTree 应当返回这个错误，实际 %v", err)
	}
	if clxMarkerExists(t, f.marker) {
		t.Errorf("接缝返回错误时 killProcessTree 退回按 PATH 起了假的 taskkill（标记文件 %s 出现了）", f.marker)
	}
}

// TestClxSystemTaskkillPath 是 C7：systemTaskkillPath 给的是 %SystemRoot%\System32\taskkill.exe（本机核对用），
// PATH 最前有假的 taskkill.cmd 时也不变；接缝的默认值就是它。最后把 SystemRoot 改成一个不存在的目录，
// systemTaskkillPath 的结果仍不变（它不读 SystemRoot）。
// 前提：SystemRoot 非空（核对用）。
// 【判别力】V9（按 PATH 找：给的是假文件）、V12（默认值不是 systemTaskkillPath）；X2（用 SystemRoot 拼 System32：
// 本机前几步照绿，改 SystemRoot 之后结果跟着变），红。
func TestClxSystemTaskkillPath(t *testing.T) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		t.Fatalf("前提不成立：SystemRoot 为空，没有可核对的系统目录")
	}
	f := clxNewFake(t)
	got, err := systemTaskkillPath()
	if err != nil {
		t.Fatalf("systemTaskkillPath 应当成功，实际 %v", err)
	}
	if want := filepath.Join(root, "System32", "taskkill.exe"); !strings.EqualFold(got, want) {
		t.Errorf("systemTaskkillPath 应当是 %s（不分大小写），实际 %s", want, got)
	}
	if strings.EqualFold(got, f.path) {
		t.Errorf("systemTaskkillPath 给的是 PATH 最前的假文件 %s", got)
	}
	if st, err := os.Stat(got); err != nil || st.IsDir() {
		t.Errorf("%s 应当是一个存在的文件（err %v）", got, err)
	}
	if reflect.ValueOf(taskkillPath).Pointer() != reflect.ValueOf(systemTaskkillPath).Pointer() {
		t.Errorf("接缝 taskkillPath 的默认值应当是 systemTaskkillPath")
	}

	const bogus = `Z:\clx-nope`
	t.Setenv("SystemRoot", bogus)
	again, err := systemTaskkillPath()
	if err != nil || again != got {
		t.Errorf("SystemRoot 改成 %s 之后 systemTaskkillPath 应当仍是 %s，实际 %q（err %v）", bogus, got, again, err)
	}
}

// clxFakeExeSrc 是 C8 现编的假 taskkill.exe 的源码：只把收到的参数（以空格连接）写进同目录的 clx-marker.txt。
const clxFakeExeSrc = `package main

import (
	"os"
	"path/filepath"
	"strings"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(exe), "clx-marker.txt"), []byte(strings.Join(os.Args[1:], " ")), 0o644)
}
`

// clxBuildFakeExe 用 go build 把 clxFakeExeSrc 编成 dir 里的 taskkill.exe，返回它的路径。go 按 PATH 找（go test
// 起测试二进制时把 GOROOT/bin 排在 PATH 最前）。编译用的 GOCACHE、GOTMPDIR 与输出目录都在本用例的 t.TempDir() 里；
// 另设 GOENV=off、GOFLAGS 为空、GOWORK=off、GOPROXY=off、GOTOOLCHAIN=local。源码只用标准库，不碰模块缓存。
func clxBuildFakeExe(t *testing.T, dir string) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("前提不成立：找不到 go，编不了假的 taskkill.exe：%v", err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(clxFakeExeSrc), 0o644); err != nil {
		t.Fatalf("写假 taskkill.exe 的源码: %v", err)
	}
	gotmp := filepath.Join(src, "gotmp")
	if err := os.Mkdir(gotmp, 0o755); err != nil {
		t.Fatalf("建 GOTMPDIR: %v", err)
	}
	exe := filepath.Join(dir, "taskkill.exe")
	cmd := exec.Command(gobin, "build", "-o", exe, "main.go")
	cmd.Dir = src
	cmd.Env = append(os.Environ(),
		"GOCACHE="+filepath.Join(src, "gocache"), "GOTMPDIR="+gotmp, "GOENV=off", "GOFLAGS=", "GOWORK=off",
		"GOPROXY=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编假的 taskkill.exe 失败: %v\n%s", err, out)
	}
	return exe
}

// TestClxKillTreeIgnoresPathTaskkillExe 是 C8：PATH 最前放一个真能运行的假 taskkill.exe（Go 现编，只写标记文件），
// killProcessTree 不执行它，该杀的循环真被杀掉了。同一装置接着把接缝指向这个 exe（正对照）：它被执行、标记文件
// 内容是 /T /F /PID <pid>，证明这个 exe 能跑、「标记不存在」来自 killProcessTree 没起它。
// 与 C4 的区别：C4 的假文件是 taskkill.cmd，按 taskkill.exe 这个全名找时绕得过它；这里排在 PATH 最前的就叫 taskkill.exe。
// 【判别力】X1（按 filepath.Base(path) 即 taskkill.exe 起）、V8、V9、V12：执行了假的，标记出现、循环没死，红；
// V11（不经接缝）：正对照那一步标记不出现，红。
func TestClxKillTreeIgnoresPathTaskkillExe(t *testing.T) {
	dir := t.TempDir()
	exe := clxBuildFakeExe(t, dir)
	marker := filepath.Join(dir, "clx-marker.txt")
	clxPrependPath(t, dir, exe)
	pid, done := clxStartLoop(t)

	if err := killProcessTree(pid); err != nil {
		t.Errorf("killProcessTree 应当返回 nil（系统目录里的 taskkill 杀掉了循环），实际 %v", err)
	}
	clxWaitDone(t, pid, done)
	if clxMarkerExists(t, marker) {
		t.Errorf("killProcessTree 执行了 PATH 最前的假 taskkill.exe（标记文件 %s 出现了）", marker)
	}

	// 正对照另起一个循环：变异下这一步若起了真 taskkill，对象仍是测试自己起的、活着的进程（理由同 C5 第 2 步）。
	pid2, _ := clxStartLoop(t)
	clxSetSeam(t, func() (string, error) { return exe, nil })
	if err := killProcessTree(pid2); err != nil {
		t.Errorf("正对照：假 taskkill.exe 以 0 退出，killProcessTree 应当返回 nil，实际 %v", err)
	}
	b, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("正对照：接缝指向假 taskkill.exe 时它应当被执行、写出标记文件，实际读不到：%v", err)
	}
	if got, want := string(b), "/T /F /PID "+strconv.Itoa(pid2); got != want {
		t.Errorf("正对照：假 taskkill.exe 收到的参数应当是 %q，实际 %q", want, got)
	}
}
