// mcp.go 是 `council mcp` 的进程边界：解析私有 flag、建 root context、
// 把装配交给 app.Run（MCP 分支），最后收敛退出码。
//
// 与 ask / serve 的唯一差别在退出码：MCP 宿主不看被调用进程的退出码，
// 启动失败（配置读不出来、库打不开）也退 0，把原因留在 stderr。用法错误
// 仍按其它子命令的惯例退 2 —— 那是人在终端里敲错了，不是宿主调用失败。
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"council/internal/app"
	"council/internal/i18n"
	"council/internal/mcp"
)

func runMCP(args []string) int {
	lang := applyLanguageFlag(prescanFlag(args, "lang"), prescanFlag(args, "config"))
	text := func(key, fallback string) string {
		return i18n.TF(lang, key, fallback)
	}

	fs := flag.NewFlagSet("council mcp", flag.ContinueOnError)
	// flag 包自己的输出丢掉：帮助与解析错误都由下面这一份 usage 统一打印，
	// 否则 -h 会出现两段格式不同的说明。
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", text("cli.mcp.flag.config",
		"配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml"))
	langFlag := fs.String("lang", "auto", text("cli.mcp.flag.lang",
		"工具输出语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）"))
	verbose := fs.Bool("verbose", false, text("cli.mcp.flag.verbose",
		"每个请求与每次工具调用都写一行到 stderr"))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, mcpUsageLang(lang))
			return exitOK
		}
		fmt.Fprintf(os.Stderr, "council mcp: %v\n", err)
		fmt.Fprint(os.Stderr, mcpUsageLang(lang))
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprint(os.Stderr, i18n.TF(lang, "cli.mcp.err.positional",
			"council mcp 不接受位置参数，多出来的是: %v\n", fs.Args()))
		fmt.Fprint(os.Stderr, mcpUsageLang(lang))
		return exitUsage
	}

	// -lang 的结果要在建 ctx 之前落地：applyLanguageFlag 同时把值写进
	// COUNCIL_LANG，后续任何读环境的代码拿到的是同一次解析。
	resolved := applyLanguageFlag(*langFlag, *configPath)

	// MCP 是被宿主拉起来的长驻进程：第二次 Ctrl-C 直接 130，与 ask / serve
	// 同一套 hardExit。stdin 被宿主关掉是更常见的退出方式。
	ctx, stop := newRootContext(true)
	defer stop()

	err := app.Run(ctx, app.Options{
		ConfigPath: *configPath,
		Version:    version,
		In:         os.Stdin,
		Out:        os.Stdout,
		Err:        os.Stderr,
		Verbose:    *verbose,
		MCP:        &mcp.Options{Lang: resolved},
		Lang:       resolved,
	})
	if err != nil {
		// app.Run 已经把给人看的错误写进 stderr；这里只补前缀，不重复内容。
		// 退 0 是刻意的：宿主不会因为它非 0 就重试或展示得更好。
		fmt.Fprintf(os.Stderr, "council mcp: %v\n", err)
	}
	return exitOK
}

// mcpUsageLang 是 -h 与用法错误共用的那一段。flag 包自己的输出被 flag
// 默认写 stderr，这里再打一份完整的说明，保证两处不会漂成两套。
func mcpUsageLang(lang i18n.Lang) string {
	return i18n.TF(lang, "cli.mcp.usage", `用法:
  council mcp [flags]

在 stdin / stdout 上跑一个 MCP 服务器：JSON-RPC 2.0，一行一条消息。
stdout 只有协议消息，所有日志走 stderr；stdin 读到底、跑完在飞的工具后退出。
同一个 data_dir 只能有一个写进程，别在 TUI 或 serve -rw 开着时再开它。

flags:
  -config <path>       配置文件路径，与全局 -config 含义相同
  -lang zh|en|auto     工具输出语言，默认 auto
  -verbose             每个请求与每次工具调用都写一行 stderr

工具:
  council_ask          问议会一轮（可指定成员、预算、工作目录、续接会话）
  council_models       列出可用成员（配置 + .claude/agents 导入）
  council_round        多轮议会 + 汇总（1..5 轮）
`)
}
