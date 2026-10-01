//go:build !windows

package tui

import "io"

// finalSupportsVT 在非 Windows 上恒为 true：终端都解释 VT 序列。
func finalSupportsVT(io.Writer) bool { return true }
