package tui

// P2：终端高度 ≤ 2 时复制屏不能画出超过终端的行数。

import (
	"strings"
	"testing"
)

func TestCopyScreenContentFitsTinyHeight(t *testing.T) {
	for _, height := range []int{1, 2} {
		m, _ := tui1eNew(t, &tui1eCopySub{})
		m.height = height
		m.copyScr = copyScreenState{on: true, lines: []string{"a", "b", "c"}}
		got := m.copyScreenContent()
		lines := strings.Count(got, "\n") + 1
		if lines > height {
			t.Errorf("height=%d 时复制屏画了 %d 行: %q", height, lines, got)
		}
	}
}
