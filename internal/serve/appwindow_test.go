package serve

// appwindow_test.go 覆盖 `-app` 的三块可测逻辑：启动参数拼装、Edge/Chrome
// 的查找顺序、以及「找不到浏览器 / 非 Windows / 启动失败」时的回落。
// 一条用例都不真的拉起浏览器：launchAppWindow 的启动器与 fileExists 从参数
// 注入，openDashboard 用包级变量替换两个打开器。

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppWindowArgs 钉住需求里建议的窗口参数：顺序、拼写与 profile 位置
// 都要逐项对上，因为任何一个参数写错都不会报错，只会让窗口看起来不像应用
// （多出地址栏）或者用错 profile。
func TestAppWindowArgs(t *testing.T) {
	dataDir := `D:\council-data`
	got := appWindowArgs("http://127.0.0.1:8765", dataDir)
	want := []string{
		"--app=http://127.0.0.1:8765",
		"--window-size=1440,900",
		"--user-data-dir=" + filepath.Join(dataDir, "ui-profile"),
		"--no-first-run",
		"--no-default-browser-check",
	}
	if len(got) != len(want) {
		t.Fatalf("参数个数 = %d，期望 %d：%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("参数[%d] = %q，期望 %q", i, got[i], want[i])
		}
	}
	// profile 必须在 data_dir 之下，而不是用户日常浏览器的 profile 目录。
	if !strings.HasSuffix(got[2], string(filepath.Separator)+"ui-profile") {
		t.Errorf("--user-data-dir 没有落在 data_dir\\ui-profile: %q", got[2])
	}
}

// TestChromiumCandidatesOrder 钉住查找顺序：Edge 先于 Chrome，同一家里
// 先试需求写的那个目录。Path 用反斜杠拼，不依赖构建宿主的分隔符。
func TestChromiumCandidatesOrder(t *testing.T) {
	want := []browserInstall{
		{Name: "Edge", Path: `D:\PF86\Microsoft\Edge\Application\msedge.exe`},
		{Name: "Edge", Path: `D:\PF\Microsoft\Edge\Application\msedge.exe`},
		{Name: "Chrome", Path: `D:\PF\Google\Chrome\Application\chrome.exe`},
		{Name: "Chrome", Path: `D:\PF86\Google\Chrome\Application\chrome.exe`},
	}

	for _, tc := range []struct {
		name      string
		pf, pfx86 string
	}{
		{name: "常规目录", pf: `D:\PF`, pfx86: `D:\PF86`},
		{name: "目录带尾部分隔符", pf: `D:\PF\`, pfx86: `D:\PF86\`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := chromiumCandidatesFor(tc.pf, tc.pfx86)
			if len(got) != len(want) {
				t.Fatalf("候选数 = %d，期望 %d：%v", len(got), len(want), got)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("候选[%d] = %+v，期望 %+v", i, got[i], want[i])
				}
			}
		})
	}

	t.Run("环境变量缺失时回落到 C 盘标准位置", func(t *testing.T) {
		got := chromiumCandidatesFor("  ", "")
		wantPaths := []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
		if len(got) != len(wantPaths) {
			t.Fatalf("候选数 = %d，期望 %d", len(got), len(wantPaths))
		}
		for i := range wantPaths {
			if got[i].Path != wantPaths[i] {
				t.Errorf("候选[%d].Path = %q，期望 %q", i, got[i].Path, wantPaths[i])
			}
		}
	})
}

// TestFindChromium 只测「按顺序取第一个存在的」：Edge 优先、Edge 缺了退回
// Chrome、都没有时明确返回 false。
func TestFindChromium(t *testing.T) {
	candidates := []browserInstall{
		{Name: "Edge", Path: `C:\edge86\msedge.exe`},
		{Name: "Edge", Path: `C:\edge64\msedge.exe`},
		{Name: "Chrome", Path: `C:\chrome64\chrome.exe`},
	}

	tests := []struct {
		name    string
		present []string
		want    string
		wantOK  bool
	}{
		{name: "Edge 全部在场时取第一个", present: []string{`C:\edge86\msedge.exe`, `C:\edge64\msedge.exe`, `C:\chrome64\chrome.exe`}, want: `C:\edge86\msedge.exe`, wantOK: true},
		{name: "Edge 只剩 64 位", present: []string{`C:\edge64\msedge.exe`, `C:\chrome64\chrome.exe`}, want: `C:\edge64\msedge.exe`, wantOK: true},
		{name: "Edge 缺失时用 Chrome", present: []string{`C:\chrome64\chrome.exe`}, want: `C:\chrome64\chrome.exe`, wantOK: true},
		{name: "都没有", present: nil, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			present := make(map[string]bool, len(tc.present))
			for _, p := range tc.present {
				present[p] = true
			}
			got, ok := findChromium(candidates, func(p string) bool { return present[p] })
			if ok != tc.wantOK {
				t.Fatalf("findChromium ok = %v，期望 %v（got=%+v）", ok, tc.wantOK, got)
			}
			if got.Path != tc.want {
				t.Errorf("findChromium = %q，期望 %q", got.Path, tc.want)
			}
		})
	}
}

// TestLaunchAppWindowRequiresWindows 钉住「-app 只在 Windows 有意义」：
// 非 Windows 返回一句可打给人看的错误，且一个进程都不启动。
func TestLaunchAppWindowRequiresWindows(t *testing.T) {
	started := false
	err := launchAppWindow("http://127.0.0.1:8765", t.TempDir(), "linux",
		chromiumCandidatesFor(`C:\PF`, `C:\PF86`),
		func(string) bool { return true },
		func(*exec.Cmd) error { started = true; return nil })
	if err == nil {
		t.Fatal("非 Windows 上 -app 期望返回错误（由调用方降级成一行说明）")
	}
	if !strings.Contains(err.Error(), "Windows") {
		t.Errorf("错误里应当说明 Windows，实际: %v", err)
	}
	if started {
		t.Error("非 Windows 上不该启动任何进程")
	}
}

// TestLaunchAppWindowUsesFirstInstall 证明真正被拉起的进程用的是第一个
// 存在的候选（Edge），且命令行与 appWindowArgs 逐项一致。
func TestLaunchAppWindowUsesFirstInstall(t *testing.T) {
	candidates := chromiumCandidatesFor(`C:\PF`, `C:\PF86`)
	present := map[string]bool{
		candidates[0].Path: true, // Edge x86
		candidates[2].Path: true, // Chrome 64
	}
	var got *exec.Cmd
	err := launchAppWindow("http://127.0.0.1:8765", `D:\data`, "windows", candidates,
		func(p string) bool { return present[p] },
		func(cmd *exec.Cmd) error { got = cmd; return nil })
	if err != nil {
		t.Fatalf("launchAppWindow 失败: %v", err)
	}
	if got == nil {
		t.Fatal("没有调用启动器")
	}
	if got.Path != candidates[0].Path {
		t.Errorf("启动的可执行文件 = %q，期望优先的 Edge %q", got.Path, candidates[0].Path)
	}
	wantArgs := append([]string{candidates[0].Path}, appWindowArgs("http://127.0.0.1:8765", `D:\data`)...)
	if len(got.Args) != len(wantArgs) {
		t.Fatalf("Args = %v，期望 %v", got.Args, wantArgs)
	}
	for i := range wantArgs {
		if got.Args[i] != wantArgs[i] {
			t.Errorf("Args[%d] = %q，期望 %q", i, got.Args[i], wantArgs[i])
		}
	}
}

// TestLaunchAppWindowFallsBackToChrome 证明 Edge 不在时真的用 Chrome，
// 而不是报错或什么都不开。
func TestLaunchAppWindowFallsBackToChrome(t *testing.T) {
	candidates := chromiumCandidatesFor(`C:\PF`, `C:\PF86`)
	var got *exec.Cmd
	err := launchAppWindow("http://127.0.0.1:8765", t.TempDir(), "windows", candidates,
		func(p string) bool { return p == candidates[3].Path },
		func(cmd *exec.Cmd) error { got = cmd; return nil })
	if err != nil {
		t.Fatalf("launchAppWindow 失败: %v", err)
	}
	if got == nil || got.Path != candidates[3].Path {
		t.Fatalf("期望启动 Chrome %q，实际 %+v", candidates[3].Path, got)
	}
}

// TestLaunchAppWindowNoChromium 钉住「都没有」时的错误文本：openDashboard
// 会把它拼成一行说明退回默认浏览器，所以这句话必须说清是找不到浏览器。
func TestLaunchAppWindowNoChromium(t *testing.T) {
	started := false
	err := launchAppWindow("http://127.0.0.1:8765", t.TempDir(), "windows",
		chromiumCandidatesFor(`C:\PF`, `C:\PF86`),
		func(string) bool { return false },
		func(*exec.Cmd) error { started = true; return nil })
	if err == nil {
		t.Fatal("一个浏览器都没有时期望返回错误")
	}
	for _, want := range []string{"找不到", "Edge", "Chrome"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误里缺少 %q: %v", want, err)
		}
	}
	if started {
		t.Error("找不到浏览器时不该尝试启动进程")
	}
}

// TestLaunchAppWindowStartFailure 证明启动器报错会被带上浏览器名字原样交回，
// 供 openDashboard 打一行并降级，而不是静默吞掉。
func TestLaunchAppWindowStartFailure(t *testing.T) {
	candidates := chromiumCandidatesFor(`C:\PF`, `C:\PF86`)
	boom := errors.New("boom")
	err := launchAppWindow("http://127.0.0.1:8765", t.TempDir(), "windows", candidates,
		func(p string) bool { return p == candidates[1].Path },
		func(*exec.Cmd) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("错误没有包住启动器的错误: %v", err)
	}
	if !strings.Contains(err.Error(), "Edge") {
		t.Errorf("错误里应当带上是哪家浏览器: %v", err)
	}
}

// stubOpeners 换掉两个包级打开器，测试结束自动还原。本包没有并行用例，
// 所以这种替换是安全的，也与既有 openBrowser 的用法一致。
func stubOpeners(t *testing.T, app func(string, string) error, def func(string) error) {
	t.Helper()
	oldApp, oldDef := openAppWindow, openBrowser
	openAppWindow, openBrowser = app, def
	t.Cleanup(func() { openAppWindow, openBrowser = oldApp, oldDef })
}

// TestOpenDashboardAppWinsOverOpen 钉住 §3 的互斥口径：两个 flag 同时给时
// 只起应用窗口，绝不额外再弹一个默认浏览器窗口，并打一行说明。
func TestOpenDashboardAppWinsOverOpen(t *testing.T) {
	var appURL, appDataDir string
	appCalls, defaultCalls := 0, 0
	stubOpeners(t,
		func(url, dataDir string) error {
			appCalls++
			appURL, appDataDir = url, dataDir
			return nil
		},
		func(string) error {
			defaultCalls++
			return nil
		})

	var log bytes.Buffer
	openDashboard("http://127.0.0.1:8765", browserOptions{App: true, Open: true, DataDir: `D:\data`}, &log)

	if appCalls != 1 || defaultCalls != 0 {
		t.Fatalf("-app 与 -open 同给时应当只走 -app：app=%d default=%d", appCalls, defaultCalls)
	}
	if appURL != "http://127.0.0.1:8765" || appDataDir != `D:\data` {
		t.Errorf("应用窗口的入参 = (%q, %q)", appURL, appDataDir)
	}
	if !strings.Contains(log.String(), "按 -app 处理") {
		t.Errorf("应当打印一行「按 -app 处理」说明，实际:\n%s", log.String())
	}
	if strings.Count(strings.TrimSpace(log.String()), "\n") > 0 {
		t.Errorf("互斥说明应当只有一行，实际:\n%s", log.String())
	}
}

// TestOpenDashboardAppFailureFallsBackToDefault 钉住 §1 的回落：应用窗口
// 起不来时打印一行说明，然后用 -open 的方式打开，不向调用方报错。
func TestOpenDashboardAppFailureFallsBackToDefault(t *testing.T) {
	defaultCalls := 0
	var defaultURL string
	stubOpeners(t,
		func(string, string) error { return errors.New("找不到 Edge 或 Chrome") },
		func(url string) error {
			defaultCalls++
			defaultURL = url
			return nil
		})

	var log bytes.Buffer
	openDashboard("http://127.0.0.1:8765", browserOptions{App: true, DataDir: t.TempDir()}, &log)

	if defaultCalls != 1 || defaultURL != "http://127.0.0.1:8765" {
		t.Fatalf("应用窗口失败后应当退回默认浏览器一次，实际 calls=%d url=%q", defaultCalls, defaultURL)
	}
	line := log.String()
	if !strings.Contains(line, "找不到 Edge 或 Chrome") || !strings.Contains(line, "系统默认浏览器") {
		t.Errorf("应当打印一行原因并说明已改走默认浏览器，实际:\n%s", line)
	}
}

// TestOpenDashboardOpenAndNoFlags 覆盖另外两条平凡路径：只给 -open 走默认
// 浏览器；两个都不给时什么都不做。后者是默认行为，不能被 -app 的引入改变。
func TestOpenDashboardOpenAndNoFlags(t *testing.T) {
	t.Run("只给 -open", func(t *testing.T) {
		appCalls, defaultCalls := 0, 0
		stubOpeners(t,
			func(string, string) error { appCalls++; return nil },
			func(string) error { defaultCalls++; return nil })
		var log bytes.Buffer
		openDashboard("http://127.0.0.1:8765", browserOptions{Open: true}, &log)
		if appCalls != 0 || defaultCalls != 1 {
			t.Fatalf("只给 -open 时 app=%d default=%d，期望 0/1", appCalls, defaultCalls)
		}
		if log.Len() != 0 {
			t.Errorf("成功打开时不该打印任何说明，实际:\n%s", log.String())
		}
	})

	t.Run("两个都不给", func(t *testing.T) {
		appCalls, defaultCalls := 0, 0
		stubOpeners(t,
			func(string, string) error { appCalls++; return nil },
			func(string) error { defaultCalls++; return nil })
		var log bytes.Buffer
		openDashboard("http://127.0.0.1:8765", browserOptions{}, &log)
		if appCalls != 0 || defaultCalls != 0 {
			t.Fatalf("两个 flag 都不给时不该打开任何东西：app=%d default=%d", appCalls, defaultCalls)
		}
		if log.Len() != 0 {
			t.Errorf("不该打印任何说明，实际:\n%s", log.String())
		}
	})
}
