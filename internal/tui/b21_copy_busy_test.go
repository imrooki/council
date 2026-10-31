package tui

// B21：/copy 的取数在跑时要串行化 Submit；复制模式取数要有代号；
// copyDoneMsg 不能被复制模式吞掉。

import (
	"strings"
	"testing"

	"council/internal/repl"
)

func TestB21CopyCommandBlocksSubmit(t *testing.T) {
	sub := &tui1eCopySub{reply: "x"}
	m, _ := tui1eNew(t, sub)
	m = tui1bType(t, m, "/copy")

	key := tui1bEnter()
	m, _ = tui1bStep(t, m, key)
	if !m.copying {
		t.Fatal("/copy 取数期间 copying 应为 true")
	}

	m = tui1bType(t, m, "hello")
	m, _ = tui1bStep(t, m, tui1bEnter())
	if m.busy {
		t.Error("/copy 取数期间不应启动一轮 Submit")
	}
	if n := sub.count(); n != 0 {
		t.Errorf("/copy 取数期间 Submit 被调用了 %d 次", n)
	}
	if !strings.Contains(m.status, "正在复制") {
		t.Errorf("状态栏 = %q，期望提示正在复制", m.status)
	}
}

func TestB21CopyDoneIsNotSwallowedByCopyScreen(t *testing.T) {
	m, _ := tui1eNew(t, &tui1eCopySub{})
	m.copyScr = copyScreenState{on: true}
	m.copying = true

	m, _ = tui1bStep(t, m, copyDoneMsg{text: "abc"})
	if m.copying {
		t.Error("copyDoneMsg 之后 copying 仍为 true")
	}
	if !strings.Contains(m.status, "已复制") {
		t.Errorf("复制模式开着时 copyDoneMsg 被吞了，状态栏 = %q", m.status)
	}
}

func TestB21CopyScreenLoadTokenDropsStaleResult(t *testing.T) {
	m, _ := tui1eNew(t, &tui1eCopySub{})
	m.copySeq = 2
	m.copyScr = copyScreenState{loading: true, loadSeq: 2}

	m, _ = tui1bStep(t, m, copyScreenLoadedMsg{
		seq:   1,
		lines: []repl.CopyLine{{Who: "a", Text: "stale"}},
	})
	if m.copyScr.on {
		t.Fatal("旧代号的取数结果把用户拽进了复制模式")
	}

	m, _ = tui1bStep(t, m, copyScreenLoadedMsg{
		seq:   2,
		lines: []repl.CopyLine{{Who: "a", Text: "fresh"}},
	})
	if !m.copyScr.on {
		t.Fatal("当前代号的取数结果没有进入复制模式")
	}
}
