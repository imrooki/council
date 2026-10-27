// Command council 是 Council 的命令行入口。这里只做三件事：解析全局 flag、
// 建好 root context 与信号处理、把结果收敛成一个退出码。
// 业务逻辑全在 internal/app，那样 app.Run 可以脱离进程与 TTY 直接测。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"council/internal/app"
	"council/internal/headless"
	"council/internal/i18n"
	"council/internal/repl"
	"council/internal/serve"
)

// version 是构建版本号。**不要手敲**：用 scripts/build.ps1，它按
// `<VERSION 文件>+git.<提交短哈希>[.dirty]` 生成并注入。
//
// 为什么强制走脚本：手工编的「0.2.0-1025g / 1025h」这类串既看不出对应哪份
// 源码，也会在下一个人手里继续发散。带提交哈希的版本号能反查到代码，
// 工作区有改动时还会标 dirty，不至于把未提交的改动当成正式构建。
// 直接 `go build` 会保留下面这个占位值，一望即知不是正式产物。
var version = "0.0.0-dev"

// 退出码约定。规格 E 只定了三个：正常 0、配置加载或校验失败 1、第二次 Ctrl+C 硬退 130。
// flag 用法错误的 2 规格未定，这里按 Go 标准库 flag.ExitOnError 的惯例取，待编排者确认。
const (
	exitOK           = 0
	exitConfigFailed = 1
	exitUsage        = 2
	exitInterrupted  = 130
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run 只是把退出码从 os.Exit 里剥出来，并不是为单元测试设计的：
// 它硬编码了进程的标准流，还注册了一个进程级的信号 channel 且从不 Stop，
// 同一进程里调两次会叠加注册。真正可测的是 app.Run。
func run(args []string) int {
	// ask 必须在全局 flag 解析之前拦下：否则 -p、-members 这些 ask 私有
	// flag 会先撞上 council 的 FlagSet。council 不带参数、-no-tui、foo
	// 三条老路一个字节都不经过这里。
	if len(args) > 0 && args[0] == "ask" {
		return runAsk(args[1:])
	}
	// serve 同理。它的 -addr / -open 与全局 flag 没有同义关系，-verbose
	// 的语义也不同（那边是每轮用量，这边是每请求一行），不能交给全局 FlagSet。
	if len(args) > 0 && args[0] == "serve" {
		return runServe(args[1:])
	}
	// commands 同理，而且它必须走到全局 FlagSet 之前：-json 与任何全局 flag
	// 没有同义关系；这条路径也不读配置、不建库、不建 REPL，退出码全是它自己的。
	if len(args) > 0 && args[0] == "commands" {
		return runCommands(args[1:])
	}
	// mcp 也要在全局 FlagSet 之前：它是一条长驻的 stdio 服务，没有 TUI / REPL，
	// 退出码约定也与宿主相关（见 mcp.go）。
	if len(args) > 0 && args[0] == "mcp" {
		return runMCP(args[1:])
	}

	// 语言要在建 FlagSet 之前定：-h 在 Parse 里就返回 ErrHelp，那时再按解析
	// 结果选说明文字已经晚了。-lang 的值与 -config 的值先按字面预扫一遍，
	// 剩下的优先级（环境变量、配置里的 language、系统区域）仍由 cliLanguage
	// 一次排完；解析成功后 applyLanguageFlag 只在显式 -lang 时把值写进
	// COUNCIL_LANG（auto 不写，避免压住配置）。
	prescanLang := cliLanguage(prescanFlag(args, "lang"), prescanFlag(args, "config"))
	text := func(key, fallback string) string {
		return i18n.TF(prescanLang, key, fallback)
	}

	fs := flag.NewFlagSet("council", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", text("cli.flag.config", "配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml"))
	lang := fs.String("lang", "auto",
		text("cli.flag.lang", "界面语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）"))
	showVersion := fs.Bool("version", false, text("cli.flag.version", "打印版本号后退出，不加载配置"))
	verbose := fs.Bool("verbose", false, text("cli.flag.verbose", "详细输出：每轮的用量、金额、冷暖、provider 会话、预算表与结算全部显示（默认简洁；进去之后也可以用 /verbose 切换）"))
	noTUI := fs.Bool("no-tui", false, text("cli.flag.noTUI", "不用 TUI，走行式 REPL（TUI 在你的终端上出问题时的退路）"))
	noAuto := fs.Bool("no-auto", false, text("cli.flag.noAuto", "直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）"))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	if *showVersion {
		fmt.Fprintf(os.Stdout, "council %s\n", version)
		return exitOK
	}

	// 多余的位置参数不静默忽略：council foo 里的 foo 没有任何含义，
	// 照常进 REPL 会让用户以为它生效了。按 Go 惯例报错退 2。
	if fs.NArg() > 0 {
		fmt.Fprint(os.Stderr, i18n.TF(prescanLang, "cli.error.positional", "council 不接受位置参数，多出来的是: %v\n", fs.Args()))
		fs.Usage()
		return exitUsage
	}

	// 两个句柄都要判。只判 stdin 的话，council > out.txt 会让 TUI 往文件里画。
	useTUI := !*noTUI && repl.IsTerminal(os.Stdin) && repl.IsTerminal(os.Stdout)

	// 显式 -lang 的解析结果写回 COUNCIL_LANG：子进程拿到的是同一次 Resolve
	// 的结果，不必再各自排一次优先级。auto / 未传不写环境变量：那会让配置
	// 永远被一个自己造出来的 COUNCIL_LANG 压住，改配置只能靠重启。
	// 返回值同时注入 app.Options.Lang（横幅、REPL、TUI 都用它），避免两处
	// 各解析一次。
	resolvedLang := applyLanguageFlag(*lang, *configPath)

	// 硬退 goroutine 只在行式模式下注册。理由见 newRootContext 里的注释。
	ctx, stop := newRootContext(!useTUI)
	defer stop()

	if err := app.Run(ctx, app.Options{
		ConfigPath: *configPath,
		Version:    version,
		In:         os.Stdin,
		Out:        os.Stdout,
		Err:        os.Stderr,
		TUI:        useTUI,
		InFile:     os.Stdin,
		OutFile:    os.Stdout,
		Verbose:    *verbose,
		NoAuto:     *noAuto,
		Lang:       resolvedLang,
	}); err != nil {
		// app.Run 已经把给人看的错误写进 stderr 了，这里只决定退出码，不再重复打印。
		return exitConfigFailed
	}
	return exitOK
}

// runAsk 是 `council ask` 的进程边界：解析私有 flag、建 root context、
// 把装配交给 app.Run，最后只做退出码映射。真正的执行在 internal/headless。
func runAsk(args []string) int {
	// ask 不走全局 FlagSet，语言要在这里自己落地：预扫 -lang / -config，
	// 求出的结果只在显式 -lang 时写进 COUNCIL_LANG；配置里的 language
	// 在这一步就能生效（cliLanguage 会只读地读一次），再作为 descLang
	// 交给 ParseLang / UsageLang。
	lang := applyLanguageFlag(prescanFlag(args, "lang"), prescanFlag(args, "config"))
	// 配置还没加载，这一趟只能做与配置无关的校验；未知成员、round 缺汇总
	// 这类要靠配置的用法错误，由 headless.Run 拿到 cfg 后补判，同样退 2。
	opts, err := headless.ParseLang(args, nil, lang)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, headless.UsageLang(lang))
			return exitOK
		}
		fmt.Fprintf(os.Stderr, "council ask: %v\n", err)
		fmt.Fprint(os.Stderr, headless.UsageLang(lang))
		return exitUsage
	}

	ctx, stop := newRootContext(true)
	defer stop()

	err = app.Run(ctx, app.Options{
		ConfigPath: opts.ConfigPath,
		Version:    version,
		In:         os.Stdin,
		Out:        os.Stdout,
		Err:        os.Stderr,
		Verbose:    opts.Verbose,
		Headless:   &opts,
	})
	// app.Run / headless.Run 已经把给人看的错误写进 stderr；这里只按
	// UsageError 与其余错误分 2 / 1，不再重复打印。
	return headless.ExitCode(err)
}

// runServe 是 `council serve` 的进程边界：解析私有 flag、建 root context、
// 把只读看板交给 internal/serve，最后做退出码映射。用法错误退 2；
// 配置、库、监听这类启动/运行失败退 1。
func runServe(args []string) int {
	lang := applyLanguageFlag(prescanFlag(args, "lang"), prescanFlag(args, "config"))
	opts, err := serve.ParseLang(args, lang)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, serve.UsageLang(lang))
			return exitOK
		}
		fmt.Fprintf(os.Stderr, "council serve: %v\n", err)
		fmt.Fprint(os.Stderr, serve.UsageLang(lang))
		return exitUsage
	}

	// hardExit 与 ask 同体例：第二次 Ctrl-C 直接 130，不陪 http.Server
	// 等那 5 秒的 Shutdown 上限。第一次 Ctrl-C 走 NotifyContext，serve 收到
	// ctx.Done 之后走优雅关闭。
	ctx, stop := newRootContext(true)
	defer stop()

	if err := serve.Run(ctx, opts, version); err != nil {
		fmt.Fprintf(os.Stderr, "council serve: %v\n", err)
		return exitConfigFailed
	}
	return exitOK
}

// runCommands 是 `council commands` 的进程边界：只读一份静态命令表，
// 不加载配置、不建 REPL、不碰 TTY。
//
// 不带 -json 时按 /help 的列格式打印，方便人直接看；-json 打印元数据，
// 给 GUI 的命令面板（M2）与 dsh 工具用。两种输出出自同一个注册表，
// 不需要另写一份命令清单，也不可能与 /help 走失同步。
//
// 写法照 ask / serve 的体例：私有 FlagSet、用法错误退 2、-h 退 0。
func runCommands(args []string) int {
	// 纯文本表的摘要跟着界面语言走（与 /help 共用 tui.cmd.*）；-json 是元数据
	// 契约，原样交出注册表里的值，一个字都不换。commands 没有 -lang，语言按
	// 环境变量 / 配置里的 language / 系统区域求；只读、失败即忽略。
	lang := cliLanguage("", "")
	text := func(key, fallback string) string {
		return i18n.TF(lang, key, fallback)
	}
	fs := flag.NewFlagSet("council commands", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, text("cli.commands.flag.json", "按 JSON 打印全部命令的元数据（name / aliases / args / summary / detail / scopes），给 GUI 面板与 dsh 工具用"))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	// 位置参数没有含义，静默忽略会让用户以为它生效了（与全局 flag 同一条纪律）。
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, text("cli.commands.error.positional", "council commands 不接受位置参数，多出来的是: %v\n"), fs.Args())
		fs.Usage()
		return exitUsage
	}

	specs := repl.CommandSpecs()
	if !*asJSON {
		// 纯文本与 /help 同源（TUISpecs 与同一份 WriteCommandTableLang），
		// JSON 用全目录。两者今天内容相同，但将来出现 TUI 专有命令时，
		// 纯文本不该跟着 JSON 一起变。
		if err := repl.WriteCommandTableLang(os.Stdout, repl.TUISpecs(), lang); err != nil {
			fmt.Fprintf(os.Stderr, "council commands: %v\n", err)
			return exitConfigFailed
		}
		return exitOK
	}

	enc := json.NewEncoder(os.Stdout)
	// 元数据里有 <全名|编号|前缀> 这种尖括号。转义成 \u003c 仍是合法 JSON，
	// 但这份输出既是给程序读的、也是给人核对字段的，没有必要牺牲可读性。
	enc.SetEscapeHTML(false)
	if err := enc.Encode(specs); err != nil {
		fmt.Fprintf(os.Stderr, "council commands: %v\n", err)
		return exitConfigFailed
	}
	return exitOK
}

// newRootContext 建 root context，并在 hardExit 时注册「第二次 Ctrl-C 硬退」。
//
// TUI 模式下不注册那个 os.Exit(130) 的 goroutine。
// os.Exit 不跑任何 defer，也不给 bubbletea 还原终端的机会。
// raw 之后 os.Exit 漏掉的是用户终端回不来：无回显、光标乱，必须 reset。
// TUI 下 Ctrl-C 是按键，不产生 SIGINT。这个 goroutine 只会等到 SIGTERM，
// 而 SIGTERM 已经由 signal.NotifyContext 交给 tea.WithContext(ctx)，
// 走干净退出。所以 TUI 模式下不注册它；行式 REPL 与 ask 模式原样保留。
//
// 第二次 Ctrl+C 立即硬退。os/signal 是广播语义：同一个信号会发给每一个注册过的
// channel，所以这里再注册一个 channel 不会把第一次信号从 NotifyContext 手上抢走 ——
// 第一次信号两边各收一份，NotifyContext 负责取消 ctx 走优雅退出，这个 goroutine
// 只把它数掉；读到第二个才 os.Exit(130)。
// 顺序上先 NotifyContext 再 Notify：万一信号恰好落在两次注册之间，
// 也是 NotifyContext 先收到，优雅退出这条路不会丢。
func newRootContext(hardExit bool) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	if hardExit {
		hard := make(chan os.Signal, 2)
		signal.Notify(hard, os.Interrupt)
		go func() {
			<-hard
			<-hard
			os.Exit(exitInterrupted)
		}()
	}
	return ctx, stop
}
