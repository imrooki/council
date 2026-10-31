package tui

// P2：TUI 本地 /copy 也要认全角斜杠。

import "testing"

func TestCopyCommandArgsFullWidthSlash(t *testing.T) {
	args, ok := copyCommandArgs("／copy 2")
	if !ok || args != "2" {
		t.Fatalf("copyCommandArgs(全角) = (%q, %v)，期望 (\"2\", true)", args, ok)
	}
}
