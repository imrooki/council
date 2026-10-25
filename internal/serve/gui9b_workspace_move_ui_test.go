// gui9b_workspace_move_ui_test.go 钉 B1 在 index.html 里的结构锚点。
// 真实点击、Alt+↑/↓ 与刷新后顺序保持由 CDP 验收覆盖；这里只防「改前端时
// 把入口、只读门或「到边不刷新」的分支整段删掉」。
package serve

import (
	"strings"
	"testing"
)

func TestGui9bIndexWorkspaceMoveMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 悬停动作里两个方向按钮，带键盘提示。
		`['up', '↑', 'Alt+↑'],`,
		`['down', '↓', 'Alt+↓'],`,
		`move.dataset.workspaceAction = direction;`,
		`move.title = (direction === 'up' ? '上移' : '下移') + '（' + shortcut + '）';`,
		// 端点与 moved:false 语义。
		`'/api/workspaces/' + encodeURIComponent(group.id) + '/move'`,
		`direction: direction === 'up' ? 'up' : 'down',`,
		`if (!moved || moved.moved !== true) {`,
		`'」已经在最' + (direction === 'up' ? '上' : '下') + '面'`,
		// 组标题可聚焦 + Alt+方向键。
		`attachWorkspaceMoveKeys(head, group)`,
		`head.tabIndex = 0`,
		`if (event.isComposing || !event.altKey || event.ctrlKey || event.metaKey) return;`,
		`if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return;`,
		`if (group.id === UNGROUPED_WORKSPACE_ID) return;`,
		// 只读模式不画按钮、也不挂快捷键。
		`if (!state.writable || group.id === UNGROUPED_WORKSPACE_ID) return;`,
		`if (state.writable && group.id !== UNGROUPED_WORKSPACE_ID) {`,
		// 重画之后把焦点放回被移动的组。
		`focusWorkspaceHeader(group.id)`,
		`function focusWorkspaceHeader(id)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// 移动按钮只能出现在写模式分支里：不能只靠点了之后 405。
	start := strings.Index(html, "function renderWorkspaceActions(group) {")
	end := strings.Index(html, "// hideWorkspaceError")
	if start < 0 || end <= start {
		t.Fatalf("renderWorkspaceActions 区块定位失败（start=%d end=%d）", start, end)
	}
	block := html[start:end]
	if !strings.Contains(block, "moveWorkspace(group, direction)") {
		t.Errorf("renderWorkspaceActions 没有接 moveWorkspace")
	}

	if n := strings.Count(html, "innerHTML"); n != 0 {
		t.Errorf("index.html 里出现了 %d 处 innerHTML，前端必须保持 0 处", n)
	}
}
