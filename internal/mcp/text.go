package mcp

import (
	"council/internal/i18n"
)

// messages 是 tools/call 返回文本用的文案表。
//
// 为什么不进 internal/i18n：那张表是给人读的界面文案（Web / REPL / CLI），
// 改一句话就要考虑界面观感；MCP 的工具输出是给宿主模型与调用方解析的
// 半结构化文本，它自己有一套稳定的形状。两套文案混在一张表里，任何一次
// 界面措辞调整都会顺手改变 MCP 的输出，而 MCP 的输出没有截图的验收环节。
//
// 语言由调用参数或 -lang / 配置决定；服务端自己打给 stderr 的日志不跟它走
// （日志是给操作者看的，与仓库其它日志一样固定中文）。
type messages struct {
	conversation  string // 会话: %s
	status        string // 状态: %s
	statusPartial string // 状态: partial（失败 %d/%d）
	errorLine     string // 错误: %s

	memberHead     string // 成员 %s（%s）:
	memberHeadSame string // 成员 %s:
	noText         string // （无正文：%s）
	summaryHead    string // ## 汇总
	summaryEmpty   string // （没有汇总正文）
	roundHead      string // ## 第 %d 轮
	truncated      string // …（已截断）

	costLine   string // 费用: $%.4f
	incomplete string // （不完整）

	modelsHead      string // 成员（%d）:
	modelsEmpty     string // （配置里没有任何成员）
	modelRow        string // - id=%s display_name=%q provider=%s model=%s permissions=%s source=%s
	modelNoModel    string // （默认）
	importedSkipped string // 注意: %d 个 .claude/agents 成员没有导入（缺 [providers.claude]）：%s
	noConfig        string // 配置未加载，无法列出成员
	listSep         string // 名单分隔符（中英标点不同）

	errNoConfig      string // 配置未加载，无法执行
	errNotReady      string // 持久化或 provider 调用未就绪
	errPrompt        string // prompt 不能为空
	errBudget        string // budget_usd 不得小于 0.5，实际 %v
	errRoundsAsk     string // council_ask 的 rounds 只能是 1，实际 %d
	errRoundsRange   string // rounds 必须在 1..5 之间，实际 %d
	errUnknownMember string // 未知成员 %s：配置里没有这些 agent（可用：%s）
	errMembersEmpty  string // members 给了但名单是空的
	errLang          string // lang 只接受 "zh" 或 "en"，实际 %q
	errCwdAbs        string // cwd 必须是绝对路径：%s
	errCwdDir        string // cwd 不是存在的目录：%s
	errNoMembers     string // 配置里没有任何成员
	errArgs          string // 参数不合法: %v
	errRunFailed     string // %s 失败: %v
	errNoResult      string // %s 没有返回结果
}

var zhMessages = messages{
	conversation:  "会话: %s",
	status:        "状态: %s",
	statusPartial: "状态: partial（失败 %d/%d）",
	errorLine:     "错误: %s",

	memberHead:     "成员 %s（%s）:",
	memberHeadSame: "成员 %s:",
	noText:         "（无正文：%s）",
	summaryHead:    "## 汇总",
	summaryEmpty:   "（没有汇总正文）",
	roundHead:      "## 第 %d 轮",
	truncated:      "…（已截断）",

	costLine:   "费用: $%.4f",
	incomplete: "（不完整）",

	modelsHead:      "成员（%d）:",
	modelsEmpty:     "（配置里没有任何成员）",
	modelRow:        "- id=%s display_name=%q provider=%s model=%s permissions=%s source=%s",
	modelNoModel:    "（默认）",
	importedSkipped: "注意: %d 个 .claude/agents 成员没有导入（配置缺 [providers.claude]）：%s",
	noConfig:        "配置未加载，无法列出成员",
	listSep:         "、",

	errNoConfig:      "配置未加载，无法执行",
	errNotReady:      "持久化或 provider 调用未就绪，无法执行",
	errPrompt:        "prompt 不能为空",
	errBudget:        "budget_usd 不得小于 0.5，实际 %v",
	errRoundsAsk:     "council_ask 的 rounds 只能是 1，实际 %d；要多轮请用 council_round",
	errRoundsRange:   "rounds 必须在 1..5 之间，实际 %d",
	errUnknownMember: "未知成员 %s：配置里没有这些 agent（可用：%s）",
	errMembersEmpty:  "members 给了但名单是空的",
	errLang:          `lang 只接受 "zh" 或 "en"，实际 %q`,
	errCwdAbs:        "cwd 必须是绝对路径：%s",
	errCwdDir:        "cwd 不是存在的目录：%s",
	errNoMembers:     "配置里没有任何成员：先在配置里加 agent，或显式给出 members",
	errArgs:          "参数不合法: %v",
	errRunFailed:     "%s 失败: %v",
	errNoResult:      "%s 没有返回结果",
}

var enMessages = messages{
	conversation:  "conversation: %s",
	status:        "status: %s",
	statusPartial: "status: partial (%d/%d failed)",
	errorLine:     "error: %s",

	memberHead:     "member %s (%s):",
	memberHeadSame: "member %s:",
	noText:         "(no text: %s)",
	summaryHead:    "## Summary",
	summaryEmpty:   "(no summary text)",
	roundHead:      "## Round %d",
	truncated:      "... (truncated)",

	costLine:   "Cost: $%.4f",
	incomplete: " (incomplete)",

	modelsHead:      "members (%d):",
	modelsEmpty:     "(no members in the config)",
	modelRow:        "- id=%s display_name=%q provider=%s model=%s permissions=%s source=%s",
	modelNoModel:    "(default)",
	importedSkipped: "note: %d .claude/agents members were not imported because [providers.claude] is missing: %s",
	noConfig:        "config not loaded; cannot list members",
	listSep:         ", ",

	errNoConfig:      "config not loaded; cannot run",
	errNotReady:      "storage or provider calls are not ready; cannot run",
	errPrompt:        "prompt must not be empty",
	errBudget:        "budget_usd must be at least 0.5, got %v",
	errRoundsAsk:     "council_ask only accepts rounds = 1, got %d; use council_round for multiple rounds",
	errRoundsRange:   "rounds must be between 1 and 5, got %d",
	errUnknownMember: "unknown member %s: no such agent in the config (available: %s)",
	errMembersEmpty:  "members was given but the list is empty",
	errLang:          `lang only accepts "zh" or "en", got %q`,
	errCwdAbs:        "cwd must be an absolute path: %s",
	errCwdDir:        "cwd is not an existing directory: %s",
	errNoMembers:     "the config has no members: add an agent to the config or pass members explicitly",
	errArgs:          "invalid arguments: %v",
	errRunFailed:     "%s failed: %v",
	errNoResult:      "%s returned no result",
}

func messagesFor(lang i18n.Lang) *messages {
	if lang == i18n.LangEn {
		return &enMessages
	}
	return &zhMessages
}
