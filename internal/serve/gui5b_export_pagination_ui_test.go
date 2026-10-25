// gui5b_export_pagination_ui_test.go 钉 B3 / B4 在 index.html 里的结构锚点：
//
//   - 导出按 after=lastSeq 分页（不再是 after=0 一页到顶）；
//   - 50000 上限、空页 / 不足一页 / lastSeq 不前进三种停止条件；
//   - 达到上限时导出件顶部与界面提示都写「已截断」；
//   - 菜单三项都在，默认「Markdown（含系统事件）」仍排第一；
//   - 仅对话版本过滤 kind === "system"、账表照旧、文件名带 -对话。
//
// 真正的点菜单、下载与文件内容（2500 条、无 system 行）由 CDP 验收覆盖。
package serve

import (
	"strings"
	"testing"
)

func TestGui5bIndexExportPaginationMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 常量：单页 1000，总上限 50000。EXPORT_LIMIT 的旧锚点仍保留。
		`const EXPORT_LIMIT = 1000;`,
		`const EXPORT_TOTAL_LIMIT = 50000;`,
		// 分页循环：第一页 after=0，之后用 after=lastSeq。
		`async function collectExportMessages(encoded, options)`,
		`'/api/conversations/' + encoded + '/messages?after=0&limit=' + EXPORT_LIMIT`,
		`'/messages?after=' + after +`,
		`'&limit=' + EXPORT_LIMIT;`,
		`if (batch.length === 0) break;`,
		`if (!Number.isFinite(lastSeq) || lastSeq <= after) break;`,
		`if (batch.length < EXPORT_LIMIT) break;`,
		// 上限：先装下剩余配额，跨页时标截断；正好到上限则探测一次。
		`if (batch.length > room) {`,
		`truncated = true;`,
		`const probe = await api('/api/conversations/' + encoded +`,
		`truncated = more.length > 0;`,
		`return { messages: collected, truncated: truncated, lastSeq: after, limit: maxTotal };`,
		// 导出的入口换成收集器，不再是一页 after=0。
		`collectExportMessages(encoded),`,
		// 导出件顶部写「已截断」；界面提示同口径。
		`'> **已截断**：导出的消息达到 ' + limit +`,
		`'（消息超过 ' + limit + ' 条上限，已截断）'`,
		// JSON 也带截断标记与上限。
		`truncated: !!(options && options.truncated),`,
		`message_limit: Number(page && page.limit) || EXPORT_TOTAL_LIMIT,`,
		// 三项菜单：默认含系统事件排第一，仅对话是第三项。
		`data-export="md">Markdown（含系统事件）`,
		`data-export="json">JSON`,
		`data-export="md-dialogue">仅对话（不含系统事件）`,
		// 仅对话：过滤 kind === "system"，账表照旧；文件名加 -对话。
		`message.kind !== 'system'`,
		`'- 范围：仅对话（不含系统事件）'`,
		`'-对话.' + extension`,
		`dialogue ? 'dialogue' : ''`,
		`const dialogueOnly = !!(options && options.dialogue);`,
		`if (dialogueOnly) {`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// 菜单顺序：Markdown（含系统事件）在最前，仅对话在最后。
	md := strings.Index(html, `data-export="md"`)
	json := strings.Index(html, `data-export="json"`)
	dialogue := strings.Index(html, `data-export="md-dialogue"`)
	if md < 0 || json < 0 || dialogue < 0 {
		t.Fatalf("导出菜单三项定位失败：md=%d json=%d dialogue=%d", md, json, dialogue)
	}
	if !(md < json && json < dialogue) {
		t.Errorf("导出菜单顺序不是 Markdown → JSON → 仅对话：%d / %d / %d", md, json, dialogue)
	}

	// 仅对话的实现必须真的按 kind 过滤，且账表仍用原来的 rows（账不筛选）。
	start := strings.Index(html, "function buildMarkdown(")
	end := strings.Index(html, "function buildExportJSON(")
	if start < 0 || end <= start {
		t.Fatalf("buildMarkdown 区块定位失败（start=%d end=%d）", start, end)
	}
	block := html[start:end]
	if !strings.Contains(block, "const messages = dialogueOnly") ||
		!strings.Contains(block, "filter((message) => message.kind !== 'system')") {
		t.Errorf("buildMarkdown 没有按 kind !== 'system' 过滤仅对话消息")
	}
	if !strings.Contains(block, "exportAccountRows(entries)") ||
		!strings.Contains(block, "## 账") {
		t.Errorf("buildMarkdown 的账表不应被仅对话过滤掉")
	}

	// 导出区块仍不看 state.writable（B3/B4 没有改变「导出是读操作」）。
	if start := strings.Index(html, "// ---- 导出会话：Markdown / JSON ----"); start >= 0 {
		if end := strings.Index(html[start:], "// ---- 会话成员管理：加人 / 踢人 ----"); end > 0 {
			if strings.Contains(html[start:start+end], "state.writable") {
				t.Errorf("导出区块里出现了 state.writable：导出必须保持只读可用")
			}
		}
	}

	if n := strings.Count(html, "innerHTML"); n != 0 {
		t.Errorf("index.html 里出现了 %d 处 innerHTML，前端必须保持 0 处", n)
	}
}
