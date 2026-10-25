package serve

// i18n_ui_keys_test.go 把「前端写了一个 key，服务端目录里没有」变成编译期
// 之后的测试失败。T() 缺 key 时回退到调用处的中文（页面不会崩），代价是
// 一条漏翻永远静默；这里扫出 index.html 里所有 ui.* 字面量，逐个要求在
// 中英两份目录里都存在。

import (
	"regexp"
	"testing"

	"council/internal/i18n"
)

// uiKeyLiteral 抓 index.html 里任何被单/双引号包住的 ui.* key。它不要求
// 出现在 t(...) 里：ROUND_KEYS / SETTINGS_RELOAD_KEYS 这类映射表里的 key
// 同样是运行时真实用到的。
var uiKeyLiteral = regexp.MustCompile(`['"](ui\.[A-Za-z0-9_.]+)['"]`)

func TestIndexUIKeysExistInBothCatalogs(t *testing.T) {
	zh := i18n.UIMessages(i18n.LangZh)
	en := i18n.UIMessages(i18n.LangEn)
	refs := uiKeyLiteral.FindAllStringSubmatch(string(indexHTML), -1)
	if len(refs) == 0 {
		t.Fatal("index.html 里没有扫到任何 ui.* key，正则或前端实现变了")
	}
	seen := make(map[string]struct{}, len(refs))
	for _, match := range refs {
		key := match[1]
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if _, ok := zh[key]; !ok {
			t.Errorf("index.html 用了 %q，但中文目录里没有", key)
		}
		if _, ok := en[key]; !ok {
			t.Errorf("index.html 用了 %q，但英文目录里没有", key)
		}
	}
}
