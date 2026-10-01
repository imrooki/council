//go:build windows

package tui

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// finalSupportsVT 报告 w 能不能解释 VT 转义序列。只查询，不调用 SetConsoleMode。
//
// w 不是 *os.File（测试里的缓冲）时当作支持：写进去的字节原样留在缓冲里。
// 是 *os.File 时问一次 GetConsoleMode：查询失败（管道、普通文件、nil 句柄）或
// ENABLE_VIRTUAL_TERMINAL_PROCESSING 没开，就是不支持。这时 Run 已经恢复了原控制台模式，
// 写进去的 ESC 会被原样打印出来，所以宁可不擦。
// 同一个查询的另一处用法：internal/repl/console_windows.go 的 IsTerminal。
func finalSupportsVT(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return true
	}
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		return false
	}
	return mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0
}
