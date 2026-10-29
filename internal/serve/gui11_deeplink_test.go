package serve

// gui11_deeplink_test.go 钉住工作台这一侧的深链契约。
//
// 桌面版把 council://conversation/<id> 翻译成 `<工作台地址>?conversation=<id>`
// （desktop/main.js 的 switchToConversation）。工作台必须认这个参数，
// 否则那条深链就是一个「打开了窗口但什么也没发生」的哑操作。
//
// 这里只验结构：真正的点击与新装应用的行为由 CDP 验收覆盖。

import (
	"strings"
	"testing"
)

func TestIndexSupportsConversationDeepLink(t *testing.T) {
	html := string(indexHTML)

	for _, want := range []string{
		// 读取与清理参数的那个函数。
		`function takeDeepLinkConversation()`,
		`url.searchParams.get('conversation')`,
		// 取完就抹掉：留着会让之后每次刷新都把用户拽回同一个会话。
		`url.searchParams.delete('conversation')`,
		`window.history.replaceState(`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 缺少深链相关的 %q", want)
		}
	}

	// 调用点必须在 refreshConversations 里、而且在「选第一个会话」之前：
	// 深链优先于「随便挑一个」。
	body := gui10FunctionBody(t, html, "async function refreshConversations()")
	deep := strings.Index(body, "takeDeepLinkConversation()")
	first := strings.Index(body, "await selectConversation(shown[0].id)")
	if deep < 0 {
		t.Fatalf("refreshConversations 没有消费深链参数:\n%s", body)
	}
	if first >= 0 && deep > first {
		t.Fatalf("深链判断排在了「选第一个会话」之后，等于永远轮不到它:\n%s", body)
	}
	// 点名了哪个会话就去哪个：必须在列表里核对过 id 再用。
	if !strings.Contains(body, "lists.active.concat(lists.archived).some((c) => c.id === wanted)") {
		t.Errorf("深链没有先核对会话是否存在:\n%s", body)
	}
}
