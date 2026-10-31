//go:build windows

package main

import "os"

// terminationSignals 在 Windows 上只有 Ctrl-C。SIGTERM 不是这里能收到的
// 停止方式；桌面版走 stdin EOF（见 serve_stdin.go），Job Object 负责带走
// 子进程树（S3）。
func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
