// schema_lag_ui_test.go 钉住 A3 的只读看板提示条：黄条存在、文案里的两个
// 版本号来自 /api/health、关闭状态只存 sessionStorage。
// 真正的点击与刷新行为由浏览器验收覆盖；这里防的是「改代码时把整段删掉」
// 以及「顺手把状态存进 localStorage」（那样升级之后提示会被人为记住）。
package serve

import (
	"strings"
	"testing"
)

// TestSchemaLagBannerInIndex 检查提示条的 DOM 锚点、文案、数据来源与关闭键。
func TestSchemaLagBannerInIndex(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		`id="schema-lag"`,
		`id="schema-lag-text"`,
		`id="schema-lag-close"`,
		// 数据来源：只认 health 的三个字段，缺字段时不猜版本号。
		`health.schema_lag !== true`,
		`health.schema_version`,
		`health.schema_expected_version`,
		`Number.isInteger(dbVersion)`,
		`Number.isInteger(expectedVersion)`,
		// 任务书点名的文案。
		`'库是 v' + dbVersion`,
		`'，程序是 v' + expectedVersion`,
		`council serve -rw`,
		`（或任意一次新版 CLI）即可升级`,
		// 关闭状态：sessionStorage，同一会话内不再出现。
		`window.sessionStorage.getItem(SCHEMA_LAG_DISMISS_KEY)`,
		`window.sessionStorage.setItem(SCHEMA_LAG_DISMISS_KEY, '1')`,
		`const SCHEMA_LAG_DISMISS_KEY = 'council.serve.schemaLagDismissed'`,
		// 只读模式才由 health 画它；提示条本身不写库。
		`renderSchemaLag(health)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// 关闭键绝不能落到 localStorage：升级后 lag 自然消失，但 localStorage
	// 会跨会话记住「关过」这件事，和「别存 localStorage」的要求相反。
	if strings.Contains(html, "localStorage.setItem(SCHEMA_LAG_DISMISS_KEY") ||
		strings.Contains(html, "localStorage.getItem(SCHEMA_LAG_DISMISS_KEY") {
		t.Error("schema_lag 的关闭状态用了 localStorage，要求存在 sessionStorage")
	}
	if n := strings.Count(html, "innerHTML"); n != 0 {
		t.Errorf("index.html 里出现了 %d 处 innerHTML，提示条也必须用 textContent", n)
	}
}
