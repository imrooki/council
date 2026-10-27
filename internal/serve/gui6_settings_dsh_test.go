package serve

// gui6_settings_dsh_test.go 钉住设置页的 dsh 形态：整页覆盖、左侧分类
// 导航、右侧独立滚动、六个分类页（第 6 类是「本机 agent」），以及
// 遮罩 / Esc / 右上 × 三条关闭路径。
// 真正的点击、预览、保存与深浅色截图由 CDP 验收覆盖。

import (
	"strings"
	"testing"
)

func TestGui6IndexSettingsDshLayout(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 整页外壳：左导航 + 右内容，不是居中小面板。
		`class="settings-panel"`,
		`class="settings-nav"`,
		`class="settings-nav-list"`,
		`class="settings-nav-foot"`,
		`class="settings-content"`,
		`class="settings-content-head"`,
		`id="settings-body" class="settings-body"`,
		`width: 188px;`,
		`height: 100%;`,
		`padding: 24px;`,
		// 六个分类，导航与页面一一对应。
		`data-settings-section="general"`,
		`data-settings-section="members"`,
		`data-settings-section="workspaces"`,
		`data-settings-section="providers"`,
		`data-settings-section="local"`,
		`data-settings-section="about"`,
		`data-settings-page="general"`,
		`data-settings-page="members"`,
		`data-settings-page="workspaces"`,
		`data-settings-page="providers"`,
		`data-settings-page="local"`,
		`data-settings-page="about"`,
		`>通用</span>`,
		`>成员</span>`,
		`>工作区</span>`,
		`>提供方</span>`,
		`>本机 agent</span>`,
		`>关于</span>`,
		// 通用：数据目录、配置文件、schema 版本、配置层。
		`id="settings-data-dir"`,
		`id="settings-config-path"`,
		`id="settings-schema-version"`,
		`id="settings-layers"`,
		// 成员、工作区、提供方、关于的列表与字段。
		`id="settings-agents-body"`,
		`id="settings-teams"`,
		`id="settings-workspaces"`,
		`id="settings-providers"`,
		`id="settings-local-level"`,
		`id="settings-local-state"`,
		`id="settings-local-desc"`,
		`id="settings-local-note"`,
		`id="settings-about-version"`,
		`id="settings-about-build"`,
		`id="settings-about-mode"`,
		// 关闭路径与高亮。
		`id="settings-close" class="settings-close"`,
		`aria-label="关闭设置"`,
		`settingsClose.addEventListener('click', closeSettings)`,
		`settingsOverlay.addEventListener('click', (event) => {`,
		`if (event.target === settingsOverlay) closeSettings();`,
		`if (event.key !== 'Escape') return;`,
		`settingsNavItems`,
		`aria-current`,
		`settingsBody.scrollTop = 0`,
		`document.body.classList.add('settings-open')`,
		`document.body.classList.remove('settings-open')`,
		// 分类切换与只读入口不变。
		`item.addEventListener('click', () => selectSettingsSection(item.dataset.settingsSection))`,
		`openSettingsButton.hidden = !state.writable`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
	if n := strings.Count(html, "innerHTML"); n != 0 {
		t.Errorf("index.html 里出现了 %d 处 innerHTML，前端必须保持 0 处", n)
	}
	// 导航项与分类页各 6 个：多一个或少一个都说明两边的 id 对不上。
	if n := strings.Count(html, `data-settings-section="`); n != 6 {
		t.Errorf("导航项 = %d，期望 6 个", n)
	}
	if n := strings.Count(html, `data-settings-page="`); n != 6 {
		t.Errorf("分类页 = %d，期望 6 个", n)
	}
}
