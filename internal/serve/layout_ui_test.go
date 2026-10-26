package serve

// layout_ui_test.go 钉住 10-26 修复轮的排版锚点。真实观感由 CDP 截图与
// 尺寸测量验收（见 .handoff 记录）；这里只防「改前端时把这一条整段删掉」，
// 与 schema_lag_ui_test / gui9b 的写法同体例。

import (
	"strings"
	"testing"
)

// cssBlock 取 selector 对应的第一条规则体。index.html 里的选择器都只出现
// 一次（同名规则的后续补丁用更具体的选择器），所以按第一次出现截到下一个
// 右花括号就够了。
func cssBlock(t *testing.T, html, selector string) string {
	t.Helper()
	start := strings.Index(html, selector)
	if start < 0 {
		t.Fatalf("index.html 里找不到选择器 %q", selector)
	}
	end := strings.Index(html[start:], "}")
	if end < 0 {
		t.Fatalf("选择器 %q 的规则没有闭合", selector)
	}
	return html[start : start+end]
}

func TestIndexPanelTitleTruncates(t *testing.T) {
	block := cssBlock(t, string(indexHTML), ".side-head h2 {")
	for _, want := range []string{
		"min-width: 0;",
		"overflow: hidden;",
		"text-overflow: ellipsis;",
		"white-space: nowrap;",
	} {
		if !strings.Contains(block, want) {
			t.Errorf(".side-head h2 缺少 %q：右栏窄到 260px 时英文标题会压到「+ 添加成员」底下", want)
		}
	}
}

func TestIndexInspectorFitsEnglishTable(t *testing.T) {
	html := string(indexHTML)
	if n := strings.Count(html, "var(--inspector-w, 384px)"); n != 2 {
		t.Errorf("固定右栏宽度的两处 fallback（.workspace 与 #app.no-sidebar）应读 384px，实际出现 %d 次", n)
	}
	if !strings.Contains(html, "inspector: { min: 260, max: 640, fallback: 384 }") {
		t.Error("拖动分栏的默认宽度回退值也必须是 384，否则双击归位会跳回放不下英文表格的 352")
	}
}

func TestIndexExportMenuSizesToContent(t *testing.T) {
	block := cssBlock(t, string(indexHTML), ".export-wrap .popover {")
	if strings.Contains(block, "width: 180px") {
		t.Error("导出菜单还留着 180px 固定宽：英文长句会被折成三行、-dialogue.md 被拆开")
	}
	if !strings.Contains(block, "width: max-content;") {
		t.Error("导出菜单宽度应按内容走（max-content），否则英文菜单会换行错位")
	}
	hint := cssBlock(t, string(indexHTML), ".export-menu .menu-hint {")
	if !strings.Contains(hint, "white-space: nowrap;") {
		t.Error(".menu-hint 必须 nowrap：-dialogue.md 不能在连字符处断行")
	}
}

func TestIndexMemberChipsLeaveRoomForPlus(t *testing.T) {
	block := cssBlock(t, string(indexHTML), "#member-chips {")
	if !strings.Contains(block, "max-width: calc(100% - 34px);") {
		t.Error("成员 chip 列表要让出「+」的宽度，否则窄窗口里加号会被挤到单独一行")
	}
}
