package serve

// srv11_skills_ui_test.go 覆盖 SRV11 §A 里能静态钉住的那一半：工作台的技能
// chip / 弹层在 index.html 里确实存在、只读提示逐字出现、渲染一律 textContent。
//
// 「数字正确」与「点开之后列出什么」依赖真实 DOM 与 /api/options，静态字符串
// 测不出来：那两条由 CDP 真点一次的验收覆盖（见交付报告），这里只锁结构，
// 免得改着改着把 chip 的 id 或只读那句删掉却没有任何测试变红。

import (
	"strings"
	"testing"
)

// TestSrv11IndexSkillsChipMarkup 钉技能 chip 的结构与只读文案。
func TestSrv11IndexSkillsChipMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		`id="skills-wrap"`,
		`id="skills-chip"`,
		`id="skills-popover"`,
		`id="skills-list"`,
		`只读：Council 不执行 skill`,
		// 数量为 0 时 chip 不出现：默认 hidden，渲染时按 length 置位。
		`skillsWrap.hidden = total === 0`,
		// chip 文案就是「技能 N」，N 来自 /api/options.skills 的长度。
		`label.textContent = '技能 ' + total`,
		// 副行是 description，没有就退到 id。
		`skill.description || skill.id`,
		// 数据来源只有 /api/options。
		`options.skills`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// chip 的容器默认带 hidden，且 .chip-wrap 的 display:inline-flex 会盖过
	// hidden 属性的 UA 样式，所以必须有一条显式的 [hidden] 收起规则。
	if !strings.Contains(html, `<div id="skills-wrap" class="chip-wrap skills-wrap" hidden>`) {
		t.Error(`id="skills-wrap" 的容器应当默认 hidden（0 个 skill 时 chip 不出现）`)
	}
	if !strings.Contains(html, `.chip-wrap[hidden] { display: none; }`) {
		t.Error("缺少 .chip-wrap[hidden] 的收起规则，hidden 会被 inline-flex 盖掉")
	}

	// 弹层里不能出现可点的按钮或菜单项：Council 不执行 skill，列表只是展示。
	// 取到所在 section 结束为止，技能弹层正好是这一段里唯一的弹层。
	start := strings.Index(html, `id="skills-popover"`)
	if start < 0 {
		t.Fatal(`找不到 id="skills-popover"`)
	}
	end := strings.Index(html[start:], "</section>")
	if end < 0 {
		t.Fatal("技能弹层所在 section 的 HTML 片段不完整")
	}
	if strings.Contains(html[start:start+end], "menu-item") {
		t.Error("技能弹层里不该有 menu-item：skill 只能展示，不能选")
	}
}

// TestSrv11IndexHasNoInnerHTML 钉 SRV11 §A 与既有 SRV6 纪律：整个前端文件里
// innerHTML 必须保持 0 处，所有用户/接口文本一律 textContent。
func TestSrv11IndexHasNoInnerHTML(t *testing.T) {
	if n := strings.Count(string(indexHTML), "innerHTML"); n != 0 {
		t.Errorf("index.html 里 innerHTML 应当保持 0 处，实际 %d 处", n)
	}
}
