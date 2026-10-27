package serve

// gui10_localagent_ui_test.go 钉住设置页第 6 类「本机 agent」的前端形态。
//
// 这里验的是结构而不是像素：导航项与分类页成对存在、层级下拉的三档带
// data-i18n、实例表与租约表有渲染函数、轮询只在「这一页可见 + 层级不是 off」
// 时启动。真正的点击与保存由 Go 侧端点用例（gui10_localagent_settings_test.go、
// localagent_test.go）与 CDP 验收各自覆盖一半。
//
// 负向断言不再是「index.html 里不许出现 /api/local-agents」（L2 的第二步就是
// 加它），而是**它的调用点必须唯一且被 off 挡住**：`setInterval` 只有一处，
// 且那一处由 syncLocalAgentsPolling 的层级/可见性判断包着。

import (
	"strings"
	"testing"

	"council/internal/i18n"
)

func TestGui10IndexLocalAgentSection(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 导航项与分类页成对。
		`data-settings-section="local"`,
		`data-settings-page="local"`,
		`aria-labelledby="settings-local-title"`,
		`id="settings-local-title"`,
		// 层级下拉的三档，标签走 i18n（缺 key 时设置页会打出 ui.xxx）。
		`id="settings-local-level" class="settings-select"`,
		`<option value="off" data-i18n="ui.settings.localLevelOff">`,
		`<option value="observe" data-i18n="ui.settings.localLevelObserve">`,
		`<option value="coordinate" data-i18n="ui.settings.localLevelCoordinate">`,
		// 状态、说明与固定文案。
		`id="settings-local-state"`,
		`id="settings-local-desc"`,
		`id="settings-local-note"`,
		`data-i18n="ui.settings.localNoModelCall"`,
		`data-i18n="ui.settings.localBoundary"`,
		// agy 的灰区必须在页面上说出来，不能只写在计划里。
		`data-i18n="ui.settings.localAgyNote"`,
		// 实例表与租约表。
		`id="settings-local-instances"`,
		`id="settings-local-instances-empty"`,
		`id="settings-local-instances-off"`,
		`id="settings-local-refresh"`,
		`id="settings-local-leases"`,
		`id="settings-local-leases-group"`,
		// 租约申请表单（P3 的看板侧入口）：三个控件 + 一个按钮，默认不自动申请。
		`id="settings-local-claim-resource"`,
		`id="settings-local-claim-mode"`,
		`id="settings-local-claim-ttl"`,
		`id="settings-local-claim"`,
		`postJSON('/api/local-agents/lease', {`,
		`postJSON('/api/local-agents/lease/release', { lease_id: leaseID })`,
		// L4 只有文案、没有自动触发（P6）。
		`data-i18n="ui.settings.localL4Note"`,
		`ui.settings.localColEvidence`,
		`ui.settings.localColResource`,
		// 渲染与保存都接上了。
		`if (next === 'local') renderSettingsLocalAgent(state.settings);`,
		`renderSettingsLocalAgent(settings);`,
		`saveSettingsLocalAgent(settingsLocalLevel.value);`,
		`postJSON('/api/settings/local-agent', { level: level })`,
		// 轮询：唯一入口 + 唯一入口点。
		`function syncLocalAgentsPolling()`,
		`if (next === 'local') renderSettingsLocalAgent(state.settings);`,
		`syncLocalAgentsPolling();`,
		`stopLocalAgentsPolling();`,
		`document.addEventListener('visibilitychange'`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// 轮询只许有一处 setInterval，而且是 3 秒（与看板既有节奏一致）。
	if n := strings.Count(html, "window.setInterval(loadLocalAgents, 3000)"); n != 1 {
		t.Errorf("loadLocalAgents 的 setInterval 出现 %d 次，期望恰好 1 次", n)
	}
	// off / 页面隐藏这两道门必须写在 syncLocalAgentsPolling 里，否则关着也会扫。
	sync := gui10FunctionBody(t, html, "function syncLocalAgentsPolling()")
	for _, want := range []string{"level === 'off'", "document.hidden", "state.settingsSection === 'local'"} {
		if !strings.Contains(sync, want) {
			t.Errorf("syncLocalAgentsPolling 里缺少判断 %q:\n%s", want, sync)
		}
	}
	if strings.Contains(sync, "setInterval") && !strings.Contains(sync, "if (!visible || level === 'off')") {
		t.Errorf("syncLocalAgentsPolling 在 off 分支之前就起了轮询:\n%s", sync)
	}
	// 关面板必须停：否则设置页收起之后还在每 3 秒打一次 /api/local-agents。
	closeBody := gui10FunctionBody(t, html, "function closeSettings()")
	if !strings.Contains(closeBody, "stopLocalAgentsPolling();") {
		t.Errorf("closeSettings 没有停掉本机实例轮询:\n%s", closeBody)
	}
	if n := strings.Count(html, "renderSettingsLocalAgent("); n < 3 {
		t.Errorf("renderSettingsLocalAgent 被调用 %d 处，期望至少 3 处（切页 / 整页渲染 / 保存后重画）", n)
	}

	// 申请租约只许由点击触发：调用**只**出现在那个 click 监听器里
	// （"async function claimLocalAgentLease()" 是定义，不算调用）。
	if n := strings.Count(html, "=> claimLocalAgentLease()"); n != 1 {
		t.Errorf("claimLocalAgentLease 的点击接线出现 %d 次，期望恰好 1 次（多一处就意味着有地方会自己申请租约）", n)
	}
	if n := strings.Count(html, "claimLocalAgentLease()") - 1; n != 1 {
		t.Errorf("claimLocalAgentLease 调用出现 %d 次（已减去定义），期望恰好 1 次", n)
	}
	// 交还同理：只在租约行的按钮里调，渲染循环之外不许出现。
	if n := strings.Count(html, "releaseLocalAgentLease(lease.lease_id)"); n != 1 {
		t.Errorf("releaseLocalAgentLease 调用出现 %d 次，期望恰好 1 次", n)
	}
}

// gui10FunctionBody 粗略截出一段函数体：从签名到下一次出现 "\n  function "
// 或 "\n  async function " 为止。前端是零构建的单文件，没有语法树可用；
// 这里只需要「这段文字里有没有那句话」，不需要精确作用域。
func gui10FunctionBody(t *testing.T, html, signature string) string {
	t.Helper()
	start := strings.Index(html, signature)
	if start < 0 {
		t.Fatalf("index.html 里没有 %q", signature)
	}
	rest := html[start+len(signature):]
	end := len(rest)
	for _, marker := range []string{"\n  function ", "\n  async function ", "\n  if (", "\n  document."} {
		if i := strings.Index(rest, marker); i >= 0 && i < end {
			end = i
		}
	}
	return rest[:end]
}

// TestGui10LocalAgentKeysInBothCatalogs 把本页用到的 key 逐条在中英目录里
// 对一遍。i18n_ui_keys_test 已经扫了 index.html 里的 ui.* 字面量，这里补的
// 是「值不许是空串」——空串能过 key 齐平的那两条测试，却会让页面出现一片
// 空白。
func TestGui10LocalAgentKeysInBothCatalogs(t *testing.T) {
	keys := []string{
		"ui.settings.navLocal",
		"ui.settings.localIntro",
		"ui.settings.localStatusTitle",
		"ui.settings.localLevel",
		"ui.settings.localLevelHint",
		"ui.settings.localLevelOff",
		"ui.settings.localLevelObserve",
		"ui.settings.localLevelCoordinate",
		"ui.settings.localDescOff",
		"ui.settings.localDescObserve",
		"ui.settings.localDescCoordinate",
		"ui.settings.localNoModelCall",
		"ui.settings.localCommands",
		"ui.settings.localClaimLock",
		"ui.settings.localBoundaryTitle",
		"ui.settings.localBoundary",
		"ui.settings.localAgyNote",
		"ui.settings.localSaved",
		"ui.settings.localSaveFailed",
		"ui.settings.localUnchanged",
		"ui.settings.localReadonly",
		"ui.settings.localRestartNote",
	}
	zh := i18n.UIMessages(i18n.LangZh)
	en := i18n.UIMessages(i18n.LangEn)
	for _, key := range keys {
		if v, ok := zh[key]; !ok || strings.TrimSpace(v) == "" {
			t.Errorf("中文目录缺 %q 或值为空", key)
		}
		if v, ok := en[key]; !ok || strings.TrimSpace(v) == "" {
			t.Errorf("英文目录缺 %q 或值为空", key)
		}
	}
}

// TestGui10LocalAgentKeysHaveNoDeadCopy 钉住反方向：本片前缀下的每条文案都
// 必须在页面里真的被引用。死键是漂移的温床 —— 页面改了、文案忘了删，下一次
// 改动就会以为这条还有用（`ui.settings.localCoordinatesOnly` 就是这么留下的）。
//
// 只扫 ui.settings.local 前缀：全库有十个按 .one / .other 后缀动态拼的键，
// 它们本来就不出现在 HTML 字面量里，全扫会误报。
func TestGui10LocalAgentKeysHaveNoDeadCopy(t *testing.T) {
	html := string(indexHTML)
	zh := i18n.UIMessages(i18n.LangZh)
	checked := 0
	for key := range zh {
		if !strings.HasPrefix(key, "ui.settings.local") {
			continue
		}
		checked++
		if !strings.Contains(html, key) {
			t.Errorf("目录里的 %q 在 index.html 里没有任何引用（死键），要么用上它，要么删掉", key)
		}
	}
	if checked < 30 {
		t.Fatalf("只扫到 %d 条本机 agent 文案，前缀可能写错了", checked)
	}
}

// TestGui10LocalAgentPageSaysWhatItDoesNotDo 是文案底线：这一页必须自己
// 说清「默认关闭、不扫描、不控制进程、零模型调用、agy 不自动拉起」。
// 这些不是装饰：用户是靠这几句话判断该不该打开开关的。
func TestGui10LocalAgentPageSaysWhatItDoesNotDo(t *testing.T) {
	zh := i18n.UIMessages(i18n.LangZh)
	joined := zh["ui.settings.localIntro"] + zh["ui.settings.localNoModelCall"] +
		zh["ui.settings.localBoundary"] + zh["ui.settings.localAgyNote"] +
		zh["ui.settings.localClaimLock"]
	for _, want := range []string{
		"默认关闭",
		"不扫描",
		"模型调用：0",
		"不启动 / 结束 / 注入任何进程",
		"灰区",
		"不会自动拉起",
		"council.lock",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("本机 agent 页的说明里没有 %q；拼起来是:\n%s", want, joined)
		}
	}
}
