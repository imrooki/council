// Command council 是 Council 的命令行入口。这里只做三件事：解析全局 flag、
// 建好 root context 与信号处理、把结果收敛成一个退出码。
// 业务逻辑全在 internal/app，那样 app.Run 可以脱离进程与 TTY 直接测。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"council/internal/app"
	"council/internal/repl"
)

// version 是构建版本号。发布时用 -ldflags "-X main.version=<版本>" 注入，
// 平时保持这个占位值。
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
	fs := flag.NewFlagSet("council", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml")
	showVersion := fs.Bool("version", false, "打印版本号后退出，不加载配置")
	verbose := fs.Bool("verbose", false, "详细输出：每轮的用量、金额、冷暖、provider 会话、预算表与结算全部显示（默认简洁；进去之后也可以用 /verbose 切换）")
	noTUI := fs.Bool("no-tui", false, "不用 TUI，走行式 REPL（TUI 在你的终端上出问题时的退路）")
	noAuto := fs.Bool("no-auto", false, "直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）")
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
		fmt.Fprintf(os.Stderr, "council 不接受位置参数，多出来的是: %v\n", fs.Args())
		fs.Usage()
		return exitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// 两个句柄都要判。只判 stdin 的话，council > out.txt 会让 TUI 往文件里画。
	useTUI := !*noTUI && repl.IsTerminal(os.Stdin) && repl.IsTerminal(os.Stdout)

	// 硬退 goroutine 只在行式模式下注册。
	//
	// TUI 模式下不注册那个 os.Exit(130) 的 goroutine。
	// os.Exit 不跑任何 defer，也不给 bubbletea 还原终端的机会。
	// raw 之后 os.Exit 漏掉的是用户终端回不来：
	// 无回显、光标乱，必须 reset。
	// TUI 下 Ctrl-C 是按键，不产生 SIGINT。这个 goroutine 只会等到 SIGTERM，
	// 而 SIGTERM 已经由上面的 signal.NotifyContext 交给 tea.WithContext(ctx)，
	// 走干净退出。所以 TUI 模式下不注册它；行式模式下原样保留。
	if !useTUI {
		// 第二次 Ctrl+C 立即硬退。os/signal 是广播语义：同一个信号会发给每一个注册过的
		// channel，所以这里再注册一个 channel 不会把第一次信号从 NotifyContext 手上抢走 ——
		// 第一次信号两边各收一份，NotifyContext 负责取消 ctx 走优雅退出，这个 goroutine
		// 只把它数掉；读到第二个才 os.Exit(130)。
		// 顺序上先 NotifyContext 再 Notify：万一信号恰好落在两次注册之间，
		// 也是 NotifyContext 先收到，优雅退出这条路不会丢。
		hard := make(chan os.Signal, 2)
		signal.Notify(hard, os.Interrupt)
		go func() {
			<-hard
			<-hard
			os.Exit(exitInterrupted)
		}()
	}

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
	}); err != nil {
		// app.Run 已经把给人看的错误写进 stderr 了，这里只决定退出码，不再重复打印。
		return exitConfigFailed
	}
	return exitOK
}
