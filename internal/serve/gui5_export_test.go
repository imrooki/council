package serve

// gui5_export_test.go 覆盖 GUI 路线第 5 片「导出会话」的 Go 侧。这一片是
// 纯前端：没有新增接口、没有新增依赖，前端只把现有三个只读接口的响应
// 拼成 Markdown / JSON 再落盘。所以这里钉两件事：
//
//  1. index.html 的结构锚点：导出按钮 / 菜单两项 / Markdown 与 JSON 的
//     固定形状 / 文件名规则 / Blob 下载与撤销，以及「导出不看 state.writable」。
//  2. 前端导出所依赖的接口契约：只读 server 上 detail + messages
//     (after=0&limit=1000) + usage 三份数据齐全，条数、金额、complete 与
//     cost_reported 的原样语义都在，且增量那一页确实会缺早期消息。
//
// 真正的浏览器行为（点菜单、真下载 .md/.json、JSON.parse、无 JS 异常）由
// Chrome CDP 验收覆盖，结果记在交付报告里；这里不实现 JS 解释器。

import (
	"math"
	"net/http"
	"strings"
	"testing"

	"council/internal/conversation"
)

// TestGui5IndexExportMarkup 钉 index.html 里的结构锚点与两条硬约束
// （零 innerHTML、导出区块不看写模式）。
func TestGui5IndexExportMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 入口：会话头右侧的「导出 ▾」与两项菜单。
		`id="export-button"`,
		`id="export-menu"`,
		`data-export="md">Markdown`,
		`data-export="json">JSON`,
		`导出`,
		// 没有选中会话时按钮隐藏。
		`exportButton.hidden = state.currentId === null`,
		// 三个现有只读接口，导出时现拉全量（不用轮询那份增量）。
		`'/api/conversations/' + encoded + '/messages?after=0&limit=' + EXPORT_LIMIT`,
		`'/api/conversations/' + encoded + '/usage'`,
		`const EXPORT_LIMIT = 1000;`,
		// Markdown 的固定骨架。
		`'# ' + ((detail && detail.title) || '(无标题)')`,
		`'- 会话：'`,
		`'- 目录：' + ((detail && detail.cwd) || '未绑定')`,
		`'- 成员：' + exportMemberLine(entries)`,
		`'- 导出时间：' + exportDateTime(now)`,
		`'| 成员 | 轮 | IN | OUT | 费用 |'`,
		`'|---|---:|---:|---:|---:|'`,
		`'（这个会话还没有消息）'`,
		`'合计 ' + formatUSD(cost) + '（' + suffix + '）'`,
		`'金额完整'`,
		`'不完整：有成员不报金额'`,
		// 不报金额的成员写「不报」，不能写 0。
		`stat.cost_reported === false ? '不报'`,
		// JSON 顶层形状：原对象整份放进去，不裁剪字段。
		`format: 'council.export/1',`,
		`exported_at: now.toISOString(),`,
		`conversation: detail,`,
		`members: (detail && detail.members) || [],`,
		`messages: (page && page.messages) || [],`,
		`usage: usage,`,
		// 文件名：清洗 + 60 字上限 + 空标题回退 council-<id 前 8 位>。
		`function sanitizeFilenamePart(raw)`,
		`Array.from(collapsed.trim()).slice(0, 60)`,
		`('council-' + String(conversationID || '').slice(0, 8))`,
		`return base + '-' + exportStamp(date) + '.' + extension;`,
		// 下载：Blob + 临时 <a download> + revokeObjectURL。
		`new Blob([text], { type: mime })`,
		`URL.createObjectURL(blob)`,
		`link.download = filename`,
		`URL.revokeObjectURL(url)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
	if strings.Contains(html, "innerHTML") {
		t.Errorf("index.html 里出现了 innerHTML，前端必须保持 0 处")
	}

	// 导出区块必须整段不看 state.writable：导出是读操作，只读模式也要能用。
	start := strings.Index(html, "// ---- 导出会话：Markdown / JSON ----")
	end := strings.Index(html, "// ---- 会话成员管理：加人 / 踢人 ----")
	if start < 0 || end <= start {
		t.Fatalf("index.html 的导出区块定位失败（start=%d end=%d）", start, end)
	}
	if block := html[start:end]; strings.Contains(block, "state.writable") {
		t.Errorf("导出区块里出现了 state.writable：导出是读操作，不该被写模式门住")
	}
}

// TestGui5ExportReadOnlyDataSources 在只读 server（没有 -rw）上验证导出要用的
// 三份数据：字段名与前端读的一一对应，条数与账的每行数字和种子一致，且
// after=0 的全量页比增量页多出早期消息——这正是「导出不能复用轮询那份
// 增量」的原因。
func TestGui5ExportReadOnlyDataSources(t *testing.T) {
	models := map[string]string{"alpha": "opus", "beta": "grok-4"}
	env := newServeTestEnv(t, models)

	// 标题里带 Windows 文件名非法字符：CDP 验收时用它检查清洗后的文件名。
	conv := env.createConversation(t, `导出: 标题/带*非法?字符`)
	env.join(t, conv.ID, "alpha", "Alpha", "claude")
	env.join(t, conv.ID, "beta", "Beta", "grok")

	env.message(t, conv.ID, "user", "第一问\n第二行")
	env.message(t, conv.ID, "alpha", "alpha 的回答")
	env.message(t, conv.ID, "beta", "beta 的回答")

	env.usage(t, conversation.TurnUsage{
		TurnID:       "turn-alpha",
		ConvID:       conv.ID,
		AgentID:      "alpha",
		Outcome:      1,
		InputTokens:  intPtr(1820),
		OutputTokens: intPtr(430),
		CostUSD:      floatPtr(0.0421),
	})
	// grok 不报金额：cost_usd 为 nil，导出的费用列必须写「不报」。
	env.usage(t, conversation.TurnUsage{
		TurnID:       "turn-beta",
		ConvID:       conv.ID,
		AgentID:      "beta",
		Outcome:      1,
		InputTokens:  intPtr(120),
		OutputTokens: intPtr(30),
	})

	// 1) 详情：标题、cwd 与成员（含 provider / model）。
	var detail conversationDetail
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID, http.StatusOK, &detail)
	if detail.Title != conv.Title || detail.Cwd != "" {
		t.Errorf("导出头用的 detail = title %q cwd %q，期望 %q 与空串",
			detail.Title, detail.Cwd, conv.Title)
	}
	if len(detail.Members) != 2 {
		t.Fatalf("detail.members 有 %d 位，期望 2", len(detail.Members))
	}
	byID := map[string]memberJSON{}
	for _, member := range detail.Members {
		byID[member.AgentID] = member
	}
	if got := byID["alpha"]; got.DisplayName != "Alpha" || got.Provider != "claude" || got.Model != "opus" {
		t.Errorf("alpha 的成员快照 = %+v，期望 Alpha/claude/opus", got)
	}
	if got := byID["beta"]; got.DisplayName != "Beta" || got.Provider != "grok" || got.Model != "grok-4" {
		t.Errorf("beta 的成员快照 = %+v，期望 Beta/grok/grok-4", got)
	}

	// 2) 全量消息页：导出用 after=0&limit=1000，条数与 last_seq 都要对得上。
	var page messagesResponse
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/messages?after=0&limit=1000",
		http.StatusOK, &page)
	if len(page.Messages) != 3 {
		t.Fatalf("全量页有 %d 条，期望 3", len(page.Messages))
	}
	if page.LastSeq != 3 {
		t.Errorf("全量页 last_seq = %d，期望 3", page.LastSeq)
	}
	for i, sender := range []string{"user", "alpha", "beta"} {
		if page.Messages[i].SenderID != sender {
			t.Errorf("第 %d 条的 sender_id = %q，期望 %q", i, page.Messages[i].SenderID, sender)
		}
		if page.Messages[i].Text == "" {
			t.Errorf("第 %d 条正文是空的，导出会缺一条内容", i)
		}
	}
	if got := page.Messages[0].Text; got != "第一问\n第二行" {
		t.Errorf("第一条正文 = %q，导出必须原样保留换行", got)
	}

	// 增量页（轮询用的形状）只有 after 之后的那一条：拿它导出会漏掉前两条。
	var tail messagesResponse
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/messages?after=2",
		http.StatusOK, &tail)
	if len(tail.Messages) != 1 || tail.Messages[0].SenderID != "beta" {
		t.Errorf("after=2 的增量页 = %+v，期望只有 beta 那一条", tail.Messages)
	}

	// 3) 账：每行数字与合计，以及「不报」的原样语义。
	var usage usageResponse
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/usage", http.StatusOK, &usage)
	if len(usage.PerAgent) != 2 {
		t.Fatalf("usage.per_agent 有 %d 行，期望 2", len(usage.PerAgent))
	}
	rows := map[string]agentUsageJSON{}
	for _, row := range usage.PerAgent {
		rows[row.AgentID] = row
	}
	alpha := rows["alpha"]
	if alpha.Turns != 1 || alpha.In != 1820 || alpha.Out != 430 {
		t.Errorf("alpha 的账 = %+v，期望 turns=1 in=1820 out=430", alpha)
	}
	if !alpha.CostReported || math.Abs(alpha.CostUSD-0.0421) > 1e-9 {
		t.Errorf("alpha 的金额 = %+v，期望报 0.0421", alpha)
	}
	beta := rows["beta"]
	if beta.CostReported {
		t.Errorf("beta 的 cost_reported = true，期望 false（groK 不报金额）")
	}
	if beta.CostUSD != 0 {
		t.Errorf("beta 的 cost_usd = %v，期望 0（没报金额，不是免费）", beta.CostUSD)
	}
	if usage.Complete {
		t.Errorf("有成员不报金额时 complete = true，期望 false")
	}
	if math.Abs(usage.CostUSD-0.0421) > 1e-9 {
		t.Errorf("usage.cost_usd = %v，期望 0.0421", usage.CostUSD)
	}
	// 合计必须等于「报了金额的那些行之和」，导出里的合计就取这个值。
	var sum float64
	for _, row := range usage.PerAgent {
		if row.CostReported {
			sum += row.CostUSD
		}
	}
	if math.Abs(sum-usage.CostUSD) > 1e-9 {
		t.Errorf("合计 %v 与逐行之和 %v 不一致", usage.CostUSD, sum)
	}
}
