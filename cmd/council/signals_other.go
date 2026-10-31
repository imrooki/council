//go:build !windows

package main

import (
	"os"
	"syscall"
)

// terminationSignals 是「请优雅退出」的信号集合。POSIX 上 SIGTERM 是
// 服务端最常用的停止信号；只监听 os.Interrupt 会让 SIGTERM 直接按系统
// 默认动作杀掉进程，serve 来不及 drain 后台议会（S3）。
func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
