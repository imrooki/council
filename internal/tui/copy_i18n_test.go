package tui

// P2：复制相关状态/错误文案走 i18n，英文界面不能弹中文。

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"council/internal/i18n"
	"council/internal/repl"
)

func TestCopyMessagesUseEnglish(t *testing.T) {
	pr := &tui1bPrinter{}
	m := New(Options{
		Submit:  &tui1eCopySub{},
		Out:     tui1bNopDrainer{},
		Version: "0.0.0-test",
		Println: pr.cmd,
		Lang:    i18n.LangEn,
	})
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	next, _ := m.copyDone(copyDoneMsg{err: repl.ErrCopyEmpty})
	m = next.(Model)
	if m.status != "No messages to copy" {
		t.Errorf("英文 ErrCopyEmpty 状态 = %q", m.status)
	}

	m.copyScr = copyScreenState{loading: true, loadSeq: 1}
	m, _ = m.copyScreenLoaded(copyScreenLoadedMsg{seq: 1, err: repl.ErrCopyEmpty})
	if m.status != "No messages to copy" {
		t.Errorf("英文复制屏 ErrCopyEmpty 状态 = %q", m.status)
	}

	m.copying = true
	next, cmd := m.copyDone(copyDoneMsg{err: repl.ErrCopyUsage})
	_ = next.(Model)
	tr := tui1bRun(t, cmd)
	if len(tr.msgs) == 0 {
		t.Fatal("ErrCopyUsage 应打印一行用法")
	}
	if got := pr.snapshot(); len(got) != 1 || got[0] != "Usage: /copy [n]" {
		t.Errorf("英文 ErrCopyUsage 输出 = %q", got)
	}
}
