//go:build !windows

package repl

import "os"

// IsTerminal 报告这个句柄是不是字符设备。nil 或 Stat 失败都当不是。
//
// 本函数在非 Windows 上**是真的做判断**：「这是不是终端」在哪个平台上都要答对，
// 否则 TUI 会在管道里被拉起来。
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
