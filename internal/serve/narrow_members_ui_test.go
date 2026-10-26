// narrow_members_ui_test.go 钉 10-26 修复轮的三处前端结构：
//
//  1. 右栏被拖窄（< 384px）之后成员表退化，而不是横向滚动：隐藏「轮」、
//     把 IN/OUT 合并成「令牌」、操作列只留图标；
//  2. 成员副行单行省略 + title 全文，窄栏隐藏缓存说明；
//  3. 换语言不再整页 reload，启动时用 /api/jobs/current 把进度面板接回来。
//
// 真实观感与像素级宽度由 CDP 验收覆盖（见 .handoff 的记录）；这里只防
// 「改前端时把整段删掉」，与 layout_ui_test / schema_lag_ui_test 同体例。
package serve

import (
	"strings"
	"testing"
)

// indexHTMLLF 把内嵌的 index.html 折成 LF 再比对。checkout 是
// core.autocrlf=true，工作区里的文件是 CRLF；字符串锚点里写多行片段时
// 两边行尾必须一致，否则测试读的是换行符而不是内容。
func indexHTMLLF() string {
	return strings.ReplaceAll(string(indexHTML), "\r\n", "\n")
}

func TestIndexNarrowInspectorDegradesTable(t *testing.T) {
	html := indexHTMLLF()

	// 阈值与观察器：384 是默认宽度（RESIZE_LIMITS.inspector.fallback）。
	for _, want := range []string{
		`const INSPECTOR_NARROW_WIDTH = 384;`,
		`new window.ResizeObserver(syncInspectorNarrow).observe(inspectorPanel)`,
		`inspectorPanel.classList.toggle('is-narrow', inspectorNarrow())`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// 窄栏布局：四列。轮 / IN / OUT 隐藏，「令牌」出现，操作列只留图标。
	for _, want := range []string{
		`#members-table thead th.col-tokens`,
		`#members-table tbody td.col-tokens { display: none; }`,
		`.inspector.is-narrow #members-table { table-layout: fixed; }`,
		`.inspector.is-narrow #members-table thead th.col-round`,
		`.inspector.is-narrow #members-table tbody td.col-round`,
		`.inspector.is-narrow #members-table thead th.col-in`,
		`.inspector.is-narrow #members-table tbody td.col-in`,
		`.inspector.is-narrow #members-table thead th.col-out`,
		`.inspector.is-narrow #members-table tbody td.col-out { display: none; }`,
		`.inspector.is-narrow #members-table thead th.col-tokens,
  .inspector.is-narrow #members-table tbody td.col-tokens { display: table-cell; }`,
		`.inspector.is-narrow #members-table .member-action-label { display: none; }`,
		`.inspector.is-narrow #members-table .member-action-icon { display: inline; }`,
		`td.num {
    font-size: 11px;
    overflow: hidden;
    text-overflow: ellipsis;
  }`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// 表头七格（含窄栏才出现的令牌列），顺序与行里的单元格一致。
	for _, want := range []string{
		`<th class="col-member" data-i18n="ui.members.colMember">成员</th>`,
		`<th class="col-round" data-i18n="ui.members.colTurns">轮</th>`,
		`<th class="col-in">IN</th>`,
		`<th class="col-out">OUT</th>`,
		`<th class="col-tokens" title="输入 / 输出令牌（窄栏合并显示）"`,
		`data-i18n="ui.members.colTokens" data-i18n-title="ui.members.colTokensTitle">令牌</th>`,
		`<th class="col-cost" data-i18n="ui.members.colCost">金额</th>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// 行结构：同一份数据既画 IN / OUT 两格，也画合并后的「令牌」一格。
	for _, want := range []string{
		`addCell(row, inText + ' / ' + outText, 'num col-tokens',`,
		`addCell(row, inText, 'num col-in',`,
		`addCell(row, outText, 'num col-out',`,
		`addCell(row, String(turns), 'num col-round',`,
		`t('ui.members.tokensTitle', '输入 {in} / 输出 {out}'`,
		`t('ui.members.turnsTitle', '轮数 {n}'`,
		`t('ui.members.inTitle', '输入令牌 {n}'`,
		`t('ui.members.outTitle', '输出令牌 {n}'`,
		`t('ui.members.costTitle', '费用 {cost}'`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	// 图标按钮：× / ↺ 只是可见文案，真正的语义在 aria-label 与 title 里，
	// 两者都用同一句带成员名字的话。
	for _, want := range []string{
		`icon.className = 'member-action-icon';`,
		`icon.textContent = member.active ? '×' : '↺';`,
		`label.className = 'member-action-label';`,
		`action.title = actionLabel;`,
		`action.setAttribute('aria-label', actionLabel);`,
		`t('ui.members.removeAria', '移除成员 {name}', { name: memberName })`,
		`t('ui.members.rejoinAria', '重新加入成员 {name}', { name: memberName })`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	if n := strings.Count(html, "innerHTML"); n != 0 {
		t.Errorf("index.html 里出现了 %d 处 innerHTML，前端必须保持 0 处", n)
	}
}

func TestIndexMemberSubLineEllipsis(t *testing.T) {
	html := indexHTMLLF()

	// 单行省略的实现：文本层绝对定位，不参与表格列宽计算，长模型名既不会
	// 折成 6–8 行，也不会把表格撑出横向滚动。
	sub := cssBlock(t, html, ".member-sub {")
	for _, want := range []string{
		"position: relative;",
		"height: 1.5em;",
	} {
		if !strings.Contains(sub, want) {
			t.Errorf(".member-sub 缺少 %q：副行必须固定一行高", want)
		}
	}
	text := cssBlock(t, html, ".member-sub-text {")
	for _, want := range []string{
		"position: absolute;",
		"right: 0;",
		"left: 0;",
		"overflow: hidden;",
		"white-space: nowrap;",
		"text-overflow: ellipsis;",
	} {
		if !strings.Contains(text, want) {
			t.Errorf(".member-sub-text 缺少 %q：副行必须单行省略", want)
		}
	}

	// title 是全文（含被藏起来的缓存说明），不是屏幕上那份被省略的文本。
	for _, want := range []string{
		`sub.title = subBits.map((bit) => bit.text).join(' · ');`,
		`if (cacheNote) subBits.push({ text: cacheNote, className: 'member-cache' });`,
		`.inspector.is-narrow .member-sub-text .member-cache,`,
		`.inspector.is-narrow .member-sub-text .member-cache-sep { display: none; }`,
		`(bit.className === 'member-cache' ? ' member-cache-sep' : '')`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
}

func TestIndexLanguageSwitchDoesNotReload(t *testing.T) {
	html := indexHTMLLF()

	// 换语言原地重画，不再整页 reload。
	if strings.Contains(html, "window.location.reload") {
		t.Error("index.html 里仍有 window.location.reload：换语言会清空正在跑的进度面板")
	}
	for _, want := range []string{
		`function applyLanguageToPage()`,
		`applyStaticI18n();`,
		`await loadI18n();`,
		`applyLanguageToPage();`,
		`const switched = LANG !== before;`,
		`t('ui.settings.languageNotSwitched'`,
		`t('ui.settings.languageSwitched'`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
}

func TestIndexReattachesRunningJobOnStartup(t *testing.T) {
	html := indexHTMLLF()

	for _, want := range []string{
		`async function reattachRunningJob()`,
		`reply = await api('/api/jobs/current');`,
		`if (!job || !job.job_id) return false;`,
		`if (state.askSource || state.askRunning) return false;`,
		`state.runSummaryID = job.summary === 'none' ? '' : (job.summary || state.summaryAgent);`,
		`state.runBudget = job.budget_usd;`,
		`await selectConversation(job.conversation_id);`,
		`subscribeJob(job.job_id);`,
		`t('ui.composer.resumedAt'`,
		// 启动顺序：health 之后、会话列表刷新之前重挂，避免先选中第一条会话。
		`loadI18n()
    .then(() => loadHealth())
    .then(() => reattachRunningJob())
    .then(() => refreshConversations())`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
}
