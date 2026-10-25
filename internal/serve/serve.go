// Package serve 实现 `council serve`：默认是只读的本地看板；
// 带 -rw 时拿 council.lock 进写模式，网页里可以跑一次议会。
//
// 两种模式的边界是刻意的：默认只做三件事——解析 serve 自己的 flag、把配置
// 解析成 data_dir 并用 store.OpenReadOnly 打开库、把 HTTP 服务挂起来。它不写库、
// 不跑迁移、不碰 council.lock，也不调用任何 provider —— 看板开着的时候 TUI 必须
// 照常能用（docs/21a_SRV1_SERVE_TASK.md §1、§2）。-rw 则相反：它必须拿排他锁、
// 跑迁移、构造 Service 与 Runner，因为跑议会要写库、要起 provider 进程
// （docs/21d_SRV4_M2_TASK.md §2）。两种模式共用同一套只读端点。
package serve

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/store"
)

// defaultAddr 是看板的默认监听地址。只绑回环，不提供改绑 0.0.0.0 的开关：
// 本片没有登录/鉴权，把它暴露到局域网等于把整个会话库交给同网段的所有人。
const defaultAddr = "127.0.0.1:8765"

// Options 是 `council serve` 的全部 flag。字段只由 Parse 填。
// out / errOut 只给测试替换标准流，正常路径留 nil 表示 os.Stdout / os.Stderr。
type Options struct {
	Addr string
	Open bool
	// App 为 true 时用 Chromium 的 --app 模式打开看板（只对 Windows 有意义）。
	// 与 Open 同时给时以 App 为准，并在启动后打一行说明；见 openDashboard。
	// 默认 false，行为与原来逐字节一致。
	App        bool
	ConfigPath string
	// Lang 是 -lang 的原值：zh / en / auto。只交给 i18n.Resolve，解析时
	// 还要与环境变量、配置里的 language 排优先级，所以这里保留字符串，
	// 不在 Parse 阶段就定成具体语言。
	Lang    string
	Verbose bool
	// RW 为 true 时走写模式：store.Open 拿 council.lock、跑迁移，并注册
	// POST /api/ask 与任务 SSE。默认 false，行为与 SRV1 的只读看板逐字节一致。
	RW bool

	out    io.Writer
	errOut io.Writer
}

// Parse 解析 serve 的私有 flag，并在解析阶段就拒掉非回环地址。
//
// 地址校验放在这里而不是 net.Listen 之后：退 2 的语义是「命令行用错了」，
// 等监听失败再报就混进了运行时错误（同一个端口被占用也该退 1）。
// 这样 `council serve -addr 0.0.0.0:8765` 在碰任何文件之前就被挡下。
func Parse(args []string) (Options, error) {
	var opts Options
	fs := newFlagSet(&opts)
	// flag 自己写的错误与用法由 cmd/council 负责打印，这里只把错误带回去。
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if fs.NArg() > 0 {
		return Options{}, fmt.Errorf("不接受位置参数，多出来的是: %v", fs.Args())
	}
	if err := validateLoopbackAddr(opts.Addr); err != nil {
		return Options{}, err
	}
	return opts, nil
}

// newFlagSet 同时给 Parse 与 Usage 用，保证两处登记的 flag 逐字一致。
// 分开写两份的结果是用法里少了新 flag、或者默认值改了而说明没改。
func newFlagSet(opts *Options) *flag.FlagSet {
	fs := flag.NewFlagSet("council serve", flag.ContinueOnError)
	fs.StringVar(&opts.Addr, "addr", defaultAddr,
		"监听地址，必须是回环地址（127.0.0.0/8、::1 或 localhost）")
	fs.BoolVar(&opts.Open, "open", false,
		"启动后调用系统浏览器打开看板；打不开只提示，不影响服务")
	fs.BoolVar(&opts.App, "app", false,
		"用 Edge/Chrome 的应用窗口打开看板，没有地址栏与标签页（仅 Windows；找不到浏览器或启动失败时退回 -open 的行为）")
	fs.StringVar(&opts.ConfigPath, "config", "",
		"配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml")
	fs.StringVar(&opts.Lang, "lang", "auto",
		"界面语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）")
	fs.BoolVar(&opts.Verbose, "verbose", false,
		"每个请求打一行到 stderr")
	fs.BoolVar(&opts.RW, "rw", false,
		"写模式：拿 council.lock，网页可以跑议会（会花你的额度；此时 TUI 不能再开）")
	return fs
}

// Usage 返回 `council serve -h` 的完整用法。
func Usage() string {
	var b strings.Builder
	fmt.Fprintln(&b, "用法: council serve [flags]")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "默认只读看板：在浏览器里看本机的会话、消息、每轮用量与费用。")
	fmt.Fprintln(&b, "不写库、不跑迁移、不抢 council.lock，TUI 开着时也能读同一个库。")
	fmt.Fprintln(&b, "-rw 写模式：拿 council.lock、可迁移，网页里可以跑一次议会并花你的额度。")
	fmt.Fprintln(&b, "拿不到锁时直接退出；写模式开着时不要再开 TUI。")
	fmt.Fprintln(&b, "只允许绑定回环地址；Ctrl-C 优雅关闭。")
	fmt.Fprintln(&b, "-app（仅 Windows）：用 Edge 或 Chrome 的 --app 窗口打开看板，没有地址栏、没有标签页。")
	fmt.Fprintln(&b, "  窗口默认 1440x900，profile 在 <data_dir>\\ui-profile，不碰你日常浏览器的会话与扩展。")
	fmt.Fprintln(&b, "  找不到 Edge/Chrome 或启动失败时打印一行说明，退回 -open 的行为；与 -open 同给时以 -app 为准。")
	fmt.Fprintln(&b, "  窗口是独立的浏览器进程：serve 退出之后它仍然开着，要关请自己关那个窗口。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Flags:")
	var opts Options
	fs := newFlagSet(&opts)
	fs.SetOutput(&b)
	fs.PrintDefaults()
	return b.String()
}

// validateLoopbackAddr 只放行三样：127.0.0.0/8、::1、localhost。
//
// 空 host（":8765"）也是拒绝的：那等于监听所有网卡，与 0.0.0.0 是同一件事，
// 只是写起来更隐蔽。
func validateLoopbackAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("地址 %q 不合法，应写成 127.0.0.1:8765: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("地址 %q 没有指定主机；必须写回环地址（例如 127.0.0.1:8765）", addr)
	}
	if port == "" {
		return fmt.Errorf("地址 %q 没有指定端口", addr)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return fmt.Errorf("地址 %q 的端口 %q 不合法，必须是 0 到 65535 的整数", addr, port)
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("只允许绑定回环地址（127.0.0.0/8、::1、localhost），%q 不是回环地址", host)
	}
	return nil
}

// isLoopbackHost 只认字面量形式的回环主机。
// 不查 DNS：一个名字解析到 127.0.0.1 不代表它一定只解析到那里，
// 也不想为了让校验通过去碰网络。
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Run 是 `council serve` 的进程边界。默认走只读那条：解析配置文件 ->
// 求 data_dir -> store.OpenReadOnly -> 监听 -> 等 Ctrl-C -> 优雅关闭；
// -rw 时改由 runWritable 拿排他锁、装配 Service/Runner 之后走同一个
// 监听与关闭流程。错误原样交回 cmd/council，由它打一行到 stderr 并映射退出码。
func Run(ctx context.Context, opts Options, version string) error {
	if err := validateLoopbackAddr(opts.Addr); err != nil {
		return err
	}
	out := opts.out
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.errOut
	if errOut == nil {
		errOut = os.Stderr
	}

	// 写模式与只读模式的差别全部在「先怎么打开库、装配什么」这一段，HTTP
	// 生命周期是同一套，所以两种模式最后都落到 listenAndServe。分支放在
	// resolveDataDir 之前：写模式需要完整配置（agents/providers），不能走
	// 只读那条「配置缺失也回落」的路。
	if opts.RW {
		return runWritable(ctx, opts, version, out, errOut)
	}

	resolved, err := resolveDataDirFull(opts.ConfigPath)
	if err != nil {
		return err
	}
	dataDir, models, notices := resolved.dataDir, resolved.models, resolved.notices
	// 合并提示只在详细档打印。只读模式同样可能读到「项目层写了 data_dir
	// 被忽略」，不说出来用户会以为看板打开了项目指定的库。
	if opts.Verbose {
		for _, note := range notices {
			fmt.Fprintf(errOut, "council serve: %s\n", note)
		}
	}

	st, err := store.OpenReadOnly(ctx, dataDir)
	if err != nil {
		return err
	}
	// 只读句柄关闭失败没有可靠的上报去处（Run 已经把返回值留给真正的启动错误），
	// 但还是要调 Close 释放连接池。忽略这一个错误不会让看板读错数据。
	defer func() { _ = st.Close() }()

	srv := NewServer(st, version, models)
	srv.Verbose = opts.Verbose
	srv.SetLanguage(opts.Lang, resolved.language)

	return listenAndServe(ctx, opts.Addr, srv.Handler(),
		browserOptions{App: opts.App, Open: opts.Open, DataDir: dataDir},
		out, errOut, nil)
}

// listenAndServe 是两种模式共用的 HTTP 生命周期：监听、打印启动行、
// 按 -app / -open 打开看板、等 Ctrl-C 后 5 秒上限优雅关闭。
//
// 把这一段抽出来不是为了少几行，而是为了让「只读模式与写模式在 HTTP 层
// 逐字节一致」这件事有一个唯一的落点：模式差异只允许出现在 handler 与
// 库的打开方式上，不能顺手在写模式里换一套关闭语义。
//
// browse 带着 -app / -open 与 -app 用的 data_dir；打开逻辑全部收在
// openDashboard 里，两种模式的打开顺序与失败回落因此不可能各写一套。
//
// drain 是写模式专属的最后一步：HTTP 连接关完之后，后台议会可能还在跑，
// 而 listenAndServe 的调用方紧接着就会关库放锁。drain 负责在关库之前
// 等它们结束（超时则显式取消）。只读模式传 nil，关闭语义与 SRV1 一致。
func listenAndServe(ctx context.Context, addr string, handler http.Handler, browse browserOptions, out, errOut io.Writer, drain func(context.Context) error) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	url := listenURL(ln.Addr())
	fmt.Fprintf(out, "council serve: %s\n", url)
	// -app 的 profile 只增不减，启动时顺手清一次（B5）。放在打开窗口之前：
	// 清理与浏览器往 profile 里写新文件不会互相干扰。两种模式都走这一个
	// 调用点，不存在「只读清了、-rw 忘了清」的分叉。目录不存在时它什么都不做，
	// -app 之外因此没有可观察的副作用。
	cleanUIOProfileAtStartup(browse.DataDir, errOut)
	// 放在监听成功之后：端口都占不上时，没有任何理由先去碰浏览器。
	openDashboard(url, browse, errOut)

	httpSrv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(ln) }()

	// drainBeforeReturn 必须在每一条返回路径上跑：正常 Ctrl-C、Shutdown
	// 超时、以及 Serve 自己异常退出，调用方的 defer 都会立刻关库放锁。
	// 生产上限 10 秒；drainJobs 自己负责「到期就 cancelJobs，再等 2 秒」。
	drainBeforeReturn := func() {
		if drain == nil {
			return
		}
		drainCtx, drainCancel := context.WithTimeout(context.Background(), jobDrainTimeout)
		drainErr := drain(drainCtx)
		drainCancel()
		if drainErr != nil {
			fmt.Fprintf(errOut, "council serve: %v\n", drainErr)
		}
	}

	select {
	case err := <-serveErr:
		drainBeforeReturn()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case <-ctx.Done():
		// 不能用已经取消的 ctx 去 Shutdown：那样第一个检查就会发现 ctx 结束、
		// 直接返回，在飞请求一个都等不到。5 秒上限来自任务书 §3。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := httpSrv.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			// 5 秒内还有连接没结束（典型：一条长议会的 SSE 还在推流）。
			// 先强制关连接，但绝不能在 drain 之前返回：调用方的 defer
			// 会立刻关库放锁，后台那一轮就静默失败了。
			_ = httpSrv.Close()
		}
		<-serveErr
		drainBeforeReturn()
		if shutdownErr != nil {
			return fmt.Errorf("关闭 HTTP 服务（5 秒上限）: %w", shutdownErr)
		}
		return nil
	}
}

// resolveDataDir 复刻 app.Run「配置文件 -> data_dir」那一段，但不走首次运行。
//
// 首次运行会写配置、建目录，那两件事都在本片的「不写库、不建目录」之外。
// 所以一份配置都没有时：不生成配置，直接按环境变量/内置默认回落，
// 让 OpenReadOnly 用「库文件不存在，先跑一次 council」把用户指回终端。
// 用户显式 -config 指了一个不存在的文件则不回落：那多半是打错了，
// 静默换用一个别的数据目录会让看板看起来「空库」，比报错更难查。
//
// 配置走合并版（项目层 + 全局层），与 app.Run 同一个口径：data_dir 只认
// 非项目层，所以项目里写了它也不会让看板去开另一个库。
//
// notices 与 app.Run / serve -rw 同源，只给 Verbose 档；只读模式不打印它们
// 也不会少读一份配置，打印出来只是让人知道项目里那个 data_dir 为什么没生效。
func resolveDataDir(configPath string) (string, map[string]string, []string, error) {
	resolved, err := resolveDataDirFull(configPath)
	return resolved.dataDir, resolved.models, resolved.notices, err
}

// resolvedServeConfig 是只读看板启动时从配置里取出的全部事实。language 是
// 配置里的原值（zh / en / auto / 空串），最终语言由 Server.SetLanguage
// 交给 i18n.Resolve 求，不在这一层猜。
type resolvedServeConfig struct {
	dataDir  string
	models   map[string]string
	notices  []string
	language string
}

// resolveDataDirFull 是 resolveDataDir 的实现。保留 resolveDataDir 那个
// 四返回值签名，是为了不动既有测试与调用方；新代码用这个结构体版本，
// 多一个 language 字段不必把参数列表继续拉长。
func resolveDataDirFull(configPath string) (resolvedServeConfig, error) {
	layers, err := config.ResolveConfigLayers(configPath)
	if err != nil {
		return resolvedServeConfig{}, fmt.Errorf("无法确定配置文件位置: %w", err)
	}
	// ResolveConfigLayers 保证至少一条（一份都不存在时是全局位置的占位层）。
	primary := layers[0].Path

	if _, statErr := os.Stat(primary); statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return resolvedServeConfig{}, fmt.Errorf("查看配置文件 %s: %w", primary, statErr)
		}
		if configPath != "" || os.Getenv(config.EnvConfigPath) != "" {
			return resolvedServeConfig{}, fmt.Errorf("配置文件不存在: %s", primary)
		}
		dataDir, err := config.ResolveDataDir("")
		if err != nil {
			return resolvedServeConfig{}, fmt.Errorf("解析数据目录失败: %w", err)
		}
		// 一份配置都没有：没有 language 可读，Resolve 会看环境变量与系统区域。
		return resolvedServeConfig{dataDir: dataDir}, nil
	}

	merged, err := config.LoadMerged(layers)
	if err != nil {
		return resolvedServeConfig{}, fmt.Errorf("加载配置失败 (%s): %w", primary, err)
	}
	dataDir, err := config.ResolveDataDir(merged.Config.DataDir)
	if err != nil {
		return resolvedServeConfig{}, fmt.Errorf("解析数据目录失败: %w", err)
	}
	models := make(map[string]string, len(merged.Config.Agents))
	for id, ac := range merged.Config.Agents {
		models[id] = ac.Model
	}
	return resolvedServeConfig{
		dataDir:  dataDir,
		models:   models,
		notices:  merged.Notices,
		language: merged.Config.Language,
	}, nil
}

// listenURL 把监听器的实际地址拼成浏览器能打开的 URL。
// 端口写 0 时 listener 会给出真实分到的端口，这里必须用它而不是原始 flag。
func listenURL(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://" + addr.String()
	}
	return "http://" + net.JoinHostPort(host, port)
}

// openBrowser 是包级变量，测试可以换掉它；生产走系统默认浏览器。
var openBrowser = openSystemBrowser

// openSystemBrowser 按平台调用「用默认程序打开这个 URL」的命令。
// 只 Start 不 Wait：浏览器进程活多久与本服务无关，Wait 会把它们绑在一起。
func openSystemBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// rundll32 不经过 cmd.exe，URL 里的 & 不会被当成命令分隔符。
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
