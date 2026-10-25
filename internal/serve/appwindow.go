package serve

// appwindow.go 是 `council serve -app` 的落点：用 Chromium 的 --app 模式
// 打开看板，得到一个没有地址栏、没有标签页的窗口，看起来像独立桌面应用。
//
// 探测只用 Windows 上的固定安装位置，不查注册表、不扫 PATH、不引入新依赖：
// 这个开关面向「本机装了 Edge 或 Chrome」的常见情况，探不到时退回系统默认
// 浏览器就够了（需求明确不报错退出，服务本身必须照常开着）。

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// appWindowSize 是应用窗口的初始大小。1440x900 放得下看板的
// 「会话列表 + 消息流 + 成员/费用」三栏，又不会在 1080p 屏上超出可用高度。
const appWindowSize = "1440,900"

// appProfileDirName 是 -app 专用 Chromium profile 在 data_dir 下的目录名。
// 给它单独一份 profile 有两个理由：一是不与用户日常浏览器的会话、扩展、
// 历史混在一起（那个 profile 里可能登着各种账号）；二是这个窗口的历史
// 与缓存全部落在 data_dir 里，不会写进用户自己的浏览器配置。
// Chromium 会自己建这个目录；它不碰库、不碰 council.lock，只读模式的
// 「不写库」边界不受影响。目录里 7 天没动过的条目会在 serve 启动时清掉
// （profile_cleanup.go），目录本身始终留着：serve 退出后窗口可能还开着。
const appProfileDirName = "ui-profile"

// browserOptions 是「启动后怎么打开看板」的输入：-app、-open 两个 flag，
// 外加 -app 拼 --user-data-dir 需要的 data_dir。Run 与 runWritable 都按
// 同一口径填，只读与 -rw 因此走完全相同的打开逻辑。
type browserOptions struct {
	App     bool
	Open    bool
	DataDir string
}

// openDashboard 在监听成功、启动行已经打印之后决定怎么打开看板。
//
// 两个 flag 同时给时以 -app 为准：先打一行说明再起应用窗口。
// -app 起不来（非 Windows、找不到 Edge/Chrome、进程启动失败）不向上报错，
// 只打一行原因并退回 -open 的行为——看板已经在监听了，用户把 URL 复制到
// 浏览器里照样能看，没有理由因为窗口模式失败就让整个 serve 退出。
//
// 关闭窗口与 serve 之间没有任何关联：这里只 Start 不 Wait，serve 退出时
// 窗口仍然是独立的浏览器进程，这不是没关干净。
func openDashboard(url string, browse browserOptions, errOut io.Writer) {
	openDefault := browse.Open
	if browse.App {
		if browse.Open {
			fmt.Fprintln(errOut, "council serve: 同时给了 -app 与 -open，按 -app 处理")
		}
		err := openAppWindow(url, browse.DataDir)
		if err == nil {
			// 应用窗口起来了就到此为止：即使同时给了 -open，也不再额外
			// 弹一个默认浏览器窗口——那正是「以 -app 为准」的意思。
			return
		}
		fmt.Fprintf(errOut, "council serve: %v；改用系统默认浏览器\n", err)
		openDefault = true
	}
	if !openDefault {
		return
	}
	if err := openBrowser(url); err != nil {
		fmt.Fprintf(errOut, "council serve: 打不开浏览器：%v\n", err)
	}
}

// openAppWindow 是包级变量，测试可以换掉它；生产走 startAppWindow。
var openAppWindow = startAppWindow

// browserInstall 是一家 Chromium 浏览器的一个安装位置。Name 只用于给人看
// 的错误提示（「启动 Edge 失败」比一个裸路径好认），不参与选择逻辑。
type browserInstall struct {
	Name string
	Path string
}

// chromiumCandidates 按优先级拼出要探测的安装位置。Program Files 的两个
// 目录从环境变量取而不是写死 C:：系统盘不是 C: 的机器上写死就等于永远
// 找不到浏览器。非 Windows 上这两个变量通常不存在，chromiumCandidatesFor
// 会回落到 C: 下的标准位置——那段代码在非 Windows 上不会被调用
// （startAppWindow 先挡了 GOOS），保留回落只是让纯函数有确定结果，便于测试。
func chromiumCandidates() []browserInstall {
	return chromiumCandidatesFor(os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"))
}

// chromiumCandidatesFor 的查找顺序就是需求给的顺序：
//
//	Edge (x86) -> Edge (64 位) -> Chrome (64 位) -> Chrome (x86)
//
// Edge 排在 Chrome 前面，因为 Windows 10/11 自带 Edge，命中率最高；
// Edge 先试 Program Files (x86) 是照需求的写法，也与 Edge 安装器在
// 64 位系统上仍把文件放进 x86 目录的实际情况一致。Chrome 反过来先试
// 64 位目录：它的安装器在有 64 位版本时优先装进 Program Files。
func chromiumCandidatesFor(programFiles, programFilesX86 string) []browserInstall {
	pf := strings.TrimSpace(programFiles)
	if pf == "" {
		pf = `C:\Program Files`
	}
	pfx86 := strings.TrimSpace(programFilesX86)
	if pfx86 == "" {
		pfx86 = `C:\Program Files (x86)`
	}
	return []browserInstall{
		{Name: "Edge", Path: windowsPath(pfx86, "Microsoft", "Edge", "Application", "msedge.exe")},
		{Name: "Edge", Path: windowsPath(pf, "Microsoft", "Edge", "Application", "msedge.exe")},
		{Name: "Chrome", Path: windowsPath(pf, "Google", "Chrome", "Application", "chrome.exe")},
		{Name: "Chrome", Path: windowsPath(pfx86, "Google", "Chrome", "Application", "chrome.exe")},
	}
}

// windowsPath 用反斜杠拼一个 Windows 路径。
//
// 这里刻意不用 filepath.Join：它按构建宿主的分隔符拼接，在非 Windows 上
// 会产出正斜杠，而这些路径只用于 Windows 上启动进程。用字符串拼接让这段
// 纯函数在任何平台上都得到同一个结果，测试也不会因为宿主系统而漂移。
func windowsPath(base string, parts ...string) string {
	base = strings.TrimRight(base, `\/`)
	return base + `\` + strings.Join(parts, `\`)
}

// findChromium 返回第一个确实存在的安装。顺序即优先级，不排序、不合并：
// 同一家浏览器的多个安装位置只应启动一个，多起一个窗口就是两个看板。
func findChromium(candidates []browserInstall, exists func(string) bool) (browserInstall, bool) {
	for _, install := range candidates {
		if exists(install.Path) {
			return install, true
		}
	}
	return browserInstall{}, false
}

// fileExists 只判断「这个路径上有一个非目录的东西」。不做可执行校验：
// Windows 上 .exe 的扩展名已经说明问题，多做一次解析只会让真机上
// 某些受控目录里的文件被误判成不可用。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// appWindowArgs 拼 Chromium 的启动参数。顺序固定，测试逐项断言：
//
//   - --app=<url>：无地址栏、无标签页的窗口，这就是「像独立应用」的来源；
//   - --window-size：看板三栏的初始大小，省得每次打开都先手动拉宽；
//   - --user-data-dir：独立 profile。不显式指定的话，--app 会挂到用户日常
//     浏览器的 profile 上，那个窗口的历史、会话与扩展就和日常浏览混在一起；
//   - --no-first-run / --no-default-browser-check：新 profile 第一次启动时
//     Chromium 会弹引导页与「设为默认浏览器」提示，正好盖住刚打开的看板；
//   - --disable-sync：上面两条挡不住 Edge 的账号同步弹窗。独立 profile 虽然
//     是空的，Edge 仍会经 Windows 账号自动登录，第一次打开就弹「在所有设备上
//     同步浏览数据」盖住看板。关掉同步之后这条提示不再出现（真机 2026-10-25 验）。
func appWindowArgs(url, dataDir string) []string {
	return []string{
		"--app=" + url,
		"--window-size=" + appWindowSize,
		"--user-data-dir=" + filepath.Join(dataDir, appProfileDirName),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
	}
}

// startAppWindow 是生产的 -app 启动入口，只把进程环境的真实值交给
// launchAppWindow。单独留这一层，是为了让测试不必碰真实浏览器。
func startAppWindow(url, dataDir string) error {
	return launchAppWindow(url, dataDir, runtime.GOOS, chromiumCandidates(), fileExists, startDetached)
}

// launchAppWindow 是可测的核心：平台、候选安装、文件探测与启动器全部从
// 参数进来。返回的 error 都是能直接打给人看的一句话，由 openDashboard
// 加上「改用系统默认浏览器」后打印。
func launchAppWindow(url, dataDir, goos string, candidates []browserInstall,
	exists func(string) bool, start func(*exec.Cmd) error) error {
	if goos != "windows" {
		// 不是错误退出：openDashboard 会把它变成一行说明并退回默认浏览器。
		return errors.New("-app 只在 Windows 上有意义")
	}
	install, ok := findChromium(candidates, exists)
	if !ok {
		return errors.New("找不到 Edge 或 Chrome")
	}
	// Chromium 的 stdout/stderr 故意不接：那是浏览器自己的日志噪音，
	// 混进 serve 的终端会让人以为服务出了问题。nil 即丢弃。
	cmd := exec.Command(install.Path, appWindowArgs(url, dataDir)...)
	if err := start(cmd); err != nil {
		return fmt.Errorf("启动 %s 失败: %w", install.Name, err)
	}
	return nil
}

// startDetached 只 Start 不 Wait，等待放到后台 goroutine 里。
//
// 这是「进程要分离」这条要求的落点：Wait 会把浏览器进程与 serve 绑在
// 一起（关窗口 → serve 收到子进程退出；serve 退出 → 窗口被一起收走），
// 而两者都必须是独立进程。后台 Wait 只为回收句柄，不关心退出码。
func startDetached(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
