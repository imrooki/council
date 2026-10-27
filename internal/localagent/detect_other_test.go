//go:build !windows

package localagent

// detect_other_test.go 钉住非 Windows 平台的契约：枚举返回 ErrUnsupported，
// 不 panic、不偷偷退化成空列表。空列表会被读成「本机没有 agent」，
// 与「这个平台看不了」是两件事。

import (
	"errors"
	"testing"
)

func TestEnumerateUnsupportedOnThisPlatform(t *testing.T) {
	processes, err := Enumerate()
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Enumerate 的错误 = %v，期望 ErrUnsupported", err)
	}
	if processes != nil {
		t.Errorf("Enumerate 返回了 %d 条进程，期望 nil", len(processes))
	}
}

func TestScanObserveUnsupportedOnThisPlatform(t *testing.T) {
	agents, err := Scan(LevelObserve, Enumerate)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Scan(observe) 的错误 = %v，期望 ErrUnsupported", err)
	}
	if len(agents) != 0 {
		t.Errorf("Scan(observe) 返回了 %d 个 agent，期望 0 个", len(agents))
	}
}
