//go:build !windows

package repl

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// IsTerminal 报告这个句柄是不是真正的终端。nil 或探测失败都当不是。
//
// 只看 ModeCharDevice 会把 /dev/null 也当成终端（B20），`council </dev/null`
// 于是进 TUI 后报 bubbletea 的 cancelable reader 错误。这里用与 bubbletea
// 同一族的 term.IsTerminal 做真正的 isatty 判断。
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(f.Fd())
}
