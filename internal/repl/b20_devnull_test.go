//go:build !windows

package repl

// B20：/dev/null 是字符设备，但不是终端。旧实现只看 ModeCharDevice，
// `council </dev/null` 会被拉进 TUI。

import (
	"os"
	"testing"
)

func TestB20DevNullIsNotTerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("打开 %s: %v", os.DevNull, err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Fatal("/dev/null 被当成了终端（B20）")
	}
}
