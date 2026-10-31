package tui

// P2：@ 补全时光标停在 token 中间，接受候选不能把右侧残余留下。

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAtAcceptCursorInsideTokenDropsSuffix(t *testing.T) {
	m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
	m = tui1bType(t, m, "hi @alice")
	m = atxSteps(t, m, atxCode(tea.KeyLeft), atxCode(tea.KeyLeft), atxCode(tea.KeyLeft))
	m, _ = tui1bStep(t, m, tui1bEnter())
	got := m.ta.Value()
	if strings.Contains(got, "ice") {
		t.Fatalf("接受补全后仍留着 token 右侧残余: %q", got)
	}
	if got != "hi @alpha " {
		t.Fatalf("接受补全后的值 = %q，期望 %q", got, "hi @alpha ")
	}
}
