package main

// serve_stdin.go 给 `council serve` 加一条「父进程已经走了」的退出路径（S3）。
//
// 桌面版在 Windows 上结束子进程只能 TerminateProcess（child.kill()），
// serve 没有任何机会 drain 后台议会，CLI 子进程会变成孤儿继续烧钱。
// 桌面版改为保持 stdin 管道打开，退出时先关管道：serve 读到 EOF 就像收到
// 一次正常的取消信号，走 http.Server.Shutdown + drainJobs + 杀进程树。
//
// 为什么用环境变量而不是「只要 stdin 不是终端就监听 EOF」：很多脚本、
// CI、服务管理器启动 serve 时 stdin 本来就是 /dev/null 或已关闭的管道。
// 那样做会让它们刚启动就退出。这个行为必须由知道「stdin 是我的生命周期
// 管道」的启动方显式打开。

import (
	"context"
	"io"
	"os"

	"council/internal/repl"
)

// serveExitOnStdinEOFEnv 是桌面版 spawn 时设置的环境变量。
const serveExitOnStdinEOFEnv = "COUNCIL_SERVE_EXIT_ON_STDIN_EOF"

// watchServeStdinEOF 在环境变量打开且 stdin 不是控制台时，等 stdin EOF
// 后调用 stop。stdin 是控制台时不动它：那是用户正在敲键盘的终端。
func watchServeStdinEOF(stop context.CancelFunc) {
	if os.Getenv(serveExitOnStdinEOFEnv) != "1" {
		return
	}
	if repl.IsTerminal(os.Stdin) {
		return
	}
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		stop()
	}()
}
