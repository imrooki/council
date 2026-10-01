//go:build windows

package repl

import (
	"os"

	"golang.org/x/sys/windows"
)

// IsTerminal 报告这个句柄是不是真控制台。
// GetConsoleMode 成功即 true：管道、普通文件、nil 都不是控制台。
//
// 只调 GetConsoleMode 问一次，不改控制台模式。
// 这个 nil 判断是**冗余的，测试抓不住它被删掉**，留着只为把意图写明。
// 2026-09-23 变异实测：拆掉它之后 TestIsTerminalNilReturnsFalse 仍然全绿。
// 根因是 (*os.File).fd()（Go 源码 os/file_unix.go:70-73，Windows 同构）自己就判 nil，
// 返回 ^uintptr(0)，于是 GetConsoleMode 拿到一个全 1 的无效句柄、失败、照样返回 false。
// 别把这条当成有用例担保的防线。
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}
