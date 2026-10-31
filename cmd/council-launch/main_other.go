//go:build !windows

// council-launch 是 Windows 桌面快捷方式的目标：双击启动看板，不弹控制台。
// 其它平台没有这个入口，但 `go build ./...` 需要每个包至少有一个可编译文件
// （审查 T1）。保留一个明确拒绝的入口，比让构建在 Linux/macOS 上直接失败好：
// 前者能跑通静态检查，后者会把整个仓库的 CI 挡在门外。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "council-launch 仅支持 Windows")
	os.Exit(1)
}
