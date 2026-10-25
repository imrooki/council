// gui9_workspace_ui_test.go 钉住工作区分组这一片在 index.html 里的结构锚点。
// 真正的点击、折叠持久化、拖拽与只读差异由 CDP 验收覆盖；这里只防「改代码
// 时把入口、文案或只读分支整段删掉」。
package serve

import (
	"strings"
	"testing"
)

// TestGui9IndexWorkspaceMarkup 检查左栏分组的骨架、两个视图分开的折叠键、
// 三个写接口的路径、拖拽的 native HTML5 形状，以及只读模式不画动作的分支。
func TestGui9IndexWorkspaceMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 列表底部的入口与删除确认框。
		`id="workspace-create"`,
		`id="workspace-delete-confirm"`,
		`button.textContent = t('ui.sidebar.workspaceCreate', '+ 新建工作区')`,
		// 折叠状态按视图分开存。
		`const WORKSPACE_COLLAPSE_KEYS = {`,
		`active: 'council.workspaces.collapsed.active'`,
		`archived: 'council.workspaces.collapsed.archived'`,
		`readWorkspaceCollapse('active')`,
		`readWorkspaceCollapse('archived')`,
		`persistWorkspaceCollapse(view, collapsed)`,
		// 分组顺序：未分组先入列，工作区按 sort_order 再按 name。
		`name: t('ui.sidebar.workspaceUngrouped', '未分组')`,
		`(Number(left.sort_order) || 0) - (Number(right.sort_order) || 0)`,
		`return a < b ? -1 : (a > b ? 1 : 0)`,
		// 组标题行：箭头 + 名字 + 条数 + 悬停动作。
		`toggle.textContent = collapsed ? '▸' : '▾'`,
		`name.textContent = group.name`,
		`count.textContent = workspaceCountText(group)`,
		`if (state.writable && group.id !== UNGROUPED_WORKSPACE_ID)`,
		`rename.dataset.workspaceAction = 'rename'`,
		`remove.dataset.workspaceAction = 'delete'`,
		// 搜索过滤后为空的组不画。
		`if (filtering && group.conversations.length === 0) continue`,
		// 三个写接口与 409 文案。
		`'/api/workspaces'`,
		`'/api/workspaces/' + encodeURIComponent(id) + '/rename'`,
		`'/api/workspaces/' + encodeURIComponent(id) + '/delete'`,
		`'/api/conversations/' + encodeURIComponent(conversation.id) + '/workspace'`,
		`workspace_id: targetID === UNGROUPED_WORKSPACE_ID ? null : targetID`,
		`t('ui.sidebar.workspaceNameTaken', '名字已被占用')`,
		// 删除工作区必须写明「只解除归属，不删除会话」。
		`只解除归属，不删除会话`,
		// 拖拽：写模式才加 draggable，标题行是放置目标。
		`item.setAttribute('draggable', 'true')`,
		`if (state.writable) attachConversationDrag(item, conversation)`,
		`if (state.writable) appendConversationMenu(item, conversation)`,
		`attachWorkspaceDropTarget(head, group)`,
		`head.addEventListener('dragover'`,
		`head.addEventListener('drop'`,
		`conversationList.querySelectorAll('.workspace-head.drop-target')`,
		// 工作区列表与两份会话列表一起刷新；只读模式也要读到它。
		`async function loadWorkspaces()`,
		`const [lists] = await Promise.all([`,
		`await loadWorkspaces()`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
	if n := strings.Count(html, "innerHTML"); n != 0 {
		t.Errorf("index.html 里出现了 %d 处 innerHTML，前端必须保持 0 处", n)
	}
	// 只读模式不能只靠「点了再报错」：新建入口与组级动作都必须有
	// state.writable 这一道门。两条锚点分开找，不依赖文件的行尾格式。
	if !strings.Contains(html, "function renderWorkspaceCreate()") ||
		!strings.Contains(html, "workspaceCreateBox.hidden = true;") {
		t.Errorf("renderWorkspaceCreate 缺少只读分支（隐藏整个新建入口）")
	}
}
