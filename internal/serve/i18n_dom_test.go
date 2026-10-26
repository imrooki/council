package serve

// i18n_dom_test.go 把「父元素与子元素同时挂同一个 data-i18n」挡在测试里。
//
// 这是 10-26 那次修复的同类：applyStaticI18n 对每个带 data-i18n 的元素各写
// 一次自己的第一个文本节点，父子同键就会渲染两遍（当时的导出按钮变成
// 「导出 导出」）。静态扫描只跟标签栈，不看 CSS/JS；页面里合法的
// 「父按钮 + 子后缀」用的是不同 key（ui.messages.exportDialogue 与
// ...Hint），不会被误报。

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var (
	// 先把 <script> 整段抹成等量换行：里面的 JS 字符串有 '<'，会让标签
	// 扫描把 `a < b` 当成标签。保留换行数，报错行号仍然是真文件的行号。
	scriptBlockRe = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	htmlTagRe     = regexp.MustCompile(`(?s)<(/)?([a-zA-Z][\w:-]*)((?:"[^"]*"|'[^']*'|[^>"'])*)>`)
	i18nAttrRe    = regexp.MustCompile(`data-i18n="([^"]*)"`)
)

var voidTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

func blankScriptBlocks(html string) string {
	return scriptBlockRe.ReplaceAllStringFunc(html, func(block string) string {
		return strings.Repeat("\n", strings.Count(block, "\n"))
	})
}

// nestedI18nFindings 返回同一 key 挂在祖先与后代上的每一处（行号 + 标签）。
// 返回值是给人看的描述，测试只关心长度与内容；单独抽出来是为了能用一段
// 人造 HTML 证明扫描器真的会报。
func nestedI18nFindings(html string) []string {
	html = blankScriptBlocks(html)
	type frame struct {
		tag  string
		key  string
		line int
	}
	var stack []frame
	var out []string
	for _, m := range htmlTagRe.FindAllStringSubmatchIndex(html, -1) {
		closing := m[2] >= 0 && html[m[2]:m[3]] == "/"
		tag := strings.ToLower(html[m[4]:m[5]])
		attrs := ""
		if m[6] >= 0 {
			attrs = html[m[6]:m[7]]
		}
		if closing {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].tag == tag {
					stack = stack[:i]
					break
				}
			}
			continue
		}

		key := ""
		if km := i18nAttrRe.FindStringSubmatch(attrs); km != nil {
			key = km[1]
		}
		line := 1 + strings.Count(html[:m[0]], "\n")
		if key != "" {
			for _, f := range stack {
				if f.key == key {
					out = append(out, fmt.Sprintf(
						"line %d <%s data-i18n=%q> inside line %d <%s data-i18n=%q>",
						line, tag, key, f.line, f.tag, f.key))
				}
			}
		}

		if voidTags[tag] || strings.HasSuffix(strings.TrimSpace(attrs), "/") {
			continue
		}
		stack = append(stack, frame{tag: tag, key: key, line: line})
	}
	return out
}

// TestNestedI18nScannerDetectsDuplicateKey 证明扫描器能抓到已知那类 bug，
// 同时不误报「父元素 + 不同 key 的子元素」。
func TestNestedI18nScannerDetectsDuplicateKey(t *testing.T) {
	bad := `<button data-i18n="ui.messages.export" hidden><span data-i18n="ui.messages.export">导出</span> ▾</button>`
	if got := nestedI18nFindings(bad); len(got) != 1 {
		t.Fatalf("父子同键应报 1 处，实际 %d：%v", len(got), got)
	}
	// 跨一层：按钮 > 包装 span > 同键 span，也要抓到。
	deeper := `<div><button data-i18n="k"><span><span data-i18n="k">x</span></span></button></div>`
	if got := nestedI18nFindings(deeper); len(got) != 1 {
		t.Fatalf("隔代同键应报 1 处，实际 %d：%v", len(got), got)
	}
	good := `<button data-i18n="ui.messages.exportDialogue">仅对话<span class="menu-hint" data-i18n="ui.messages.exportDialogueHint">-对话.md</span></button>`
	if got := nestedI18nFindings(good); len(got) != 0 {
		t.Fatalf("父元素 + 不同 key 的子元素不该报，实际 %v", got)
	}
	// 两个并列的同键元素不是父子，合法。
	siblings := `<div><span data-i18n="k">a</span><span data-i18n="k">b</span></div>`
	if got := nestedI18nFindings(siblings); len(got) != 0 {
		t.Fatalf("并列同键不该报，实际 %v", got)
	}
}

// TestIndexNoNestedI18nKeys 要求 index.html 里同一个 data-i18n key 不出现在
// 同一条祖先链上。
func TestIndexNoNestedI18nKeys(t *testing.T) {
	if got := nestedI18nFindings(string(indexHTML)); len(got) != 0 {
		for _, finding := range got {
			t.Errorf("index.html: %s —— setElementText 会把两处都填一遍，页面上出现两遍同样的文案", finding)
		}
	}
}
