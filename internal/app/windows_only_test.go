package app

import (
	"runtime"
	"testing"
)

// skipUnlessWindows 跳过那些装置本身依赖 Windows 的用例：它们要么用 .cmd
// 批处理垫片冒充 CLI（Linux 上既没有 cmd.exe，也造不出可执行的 .cmd），要么
// 把 %APPDATA% / 盘符当成固定事实。产品代码跨平台，但这些装置不是。
//
// 跳过而不是删除：Windows 上仍是完整回归；Linux CI 只负责编译与平台无关的
// 用例（审查 T2）。
func skipUnlessWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only: 装置依赖 .cmd 垫片或 Windows 默认安装路径")
	}
}
