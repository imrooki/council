package provider_test

// CXH 契约 §2.0 的夹具块 F1 与期望块 G1（逐字照抄契约，由脚本从契约取出）。

import (
	"strings"

	"council/internal/provider"
)

// cxhBigModelMessages 生成 gpt-6-sol 那一行 model_messages 的值：合成内容（不是 codex 的真实指令文本），
// 键形状照真输出；22000 字节，含 JSON 转义的引号与换行、花括号方括号、中文，
// 以及一个嵌套的诱饵对象（带 slug / visibility / supported_reasoning_levels 三个键）。
func cxhBigModelMessages() string {
	chunk := `line with \"quotes\", braces {}, brackets [], a key \"slug\":\"cxh-decoy\" and 中文\n`
	return `{"instructions_template":"` + strings.Repeat(chunk, 250) + `","instructions_variables":null,"approvals":{"on_request":null,"never":null},"decoy":{"slug":"cxh-decoy","visibility":"list","supported_reasoning_levels":[{"effort":"low"}]},"token_budget":{"reminder_threshold_tokens":6144}}`
}

// cxhCodexHead 是第 1 条记录（gpt-6-sol，真输出裁剪）到 model_messages 键为止。
const cxhCodexHead = `{"models":[{"slug":"gpt-6-sol","display_name":"GPT-6-Sol","description":"Workhorse model for coding and everyday work.","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low","description":"Fast responses with lighter reasoning"},{"effort":"medium","description":"Balances speed and reasoning depth for everyday tasks"},{"effort":"high","description":"Greater reasoning depth for complex problems"},{"effort":"xhigh","description":"Extra high reasoning depth for complex problems"},{"effort":"max","description":"Maximum reasoning depth for the hardest problems"},{"effort":"ultra","description":"Maximum reasoning with automatic task delegation"}],"visibility":"list","supported_in_api":true,"priority":0,"model_messages":`

// 两条 visibility 为 list 的 custom/ 记录（真输出裁剪），R2 要单独删掉它们。
const cxhRecGrokCustom = `{"slug":"custom/local-proxy-grok-4.7","display_name":"Grok 4.7 (local proxy)","default_reasoning_level":"high","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},`
const cxhRecGeminiCustom = `{"slug":"custom/local-proxy-gemini-3.8-flash-high","display_name":"Gemini 3.8 Flash High (local proxy)","default_reasoning_level":"high","supported_reasoning_levels":[{"effort":"high"}],"visibility":"list"},`

// cxhCodexTail 从 gpt-6-sol 的 model_messages 之后接到文件末尾：
// 真输出裁剪的 gpt-6-luna、gpt-reserve（hide）、gpt-5.6-sol、gpt-5.5、两条 custom/（list）、
// custom/qwen3.8-27b（hide，display_name 换成了合成值）；其后全是合成记录：
// claude-cxh-9（display_name 空、强度含 turbo 与 LOW）、cxh-bell（display_name 含 BEL、没有强度键）、
// cxh-min（只有 minimal 与 low）、customer-cxh（custom 没有斜杠）、slug 含 ESC、slug 空、没有 visibility 键。
const cxhCodexTail = `,"context_window":272000},` +
	`{"slug":"gpt-6-luna","display_name":"GPT-6-Luna","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"}],"visibility":"list"},` +
	`{"slug":"gpt-reserve","display_name":"GPT-Reserve","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"}],"visibility":"hide"},` +
	`{"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}],"visibility":"list"},` +
	`{"slug":"gpt-5.5","display_name":"GPT-5.5","default_reasoning_level":"xhigh","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},` +
	cxhRecGrokCustom +
	`{"slug":"custom/qwen3.8-27b","display_name":"Qwen custom","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"minimal"}],"visibility":"hide"},` +
	cxhRecGeminiCustom +
	`{"slug":"claude-cxh-9","display_name":"","supported_reasoning_levels":[{"effort":"low"},{"effort":"turbo"},{"effort":"high"},{"effort":"LOW"}],"visibility":"list"},` +
	`{"slug":"cxh-bell","display_name":"Cxh\u0007Bell","visibility":"list"},` +
	`{"slug":"cxh-min","display_name":"Cxh Min","supported_reasoning_levels":[{"effort":"minimal"},{"effort":"low"}],"visibility":"list"},` +
	`{"slug":"customer-cxh","display_name":"Customer Cxh","visibility":"list"},` +
	`{"slug":"cxh\u001bx","display_name":"Esc","visibility":"list"},` +
	`{"slug":"","display_name":"Empty","visibility":"list"},` +
	`{"slug":"cxh-novis","display_name":"No Vis","supported_reasoning_levels":[{"effort":"high"}]}` +
	`]}` + "\n"

// cxhCodexFixture 是整份夹具：24948 字节，一行，末尾一个换行。
func cxhCodexFixture() string {
	return cxhCodexHead + cxhBigModelMessages() + cxhCodexTail
}

// cxhWantCodex 是 P2 的 10 行期望，顺序同夹具。Alias 一律空；Efforts 一档都没有时是 nil。
func cxhWantCodex() []provider.DiscoveredModel {
	row := func(model, name string, efforts []string, custom bool) provider.DiscoveredModel {
		return provider.DiscoveredModel{Provider: "codex", Model: model, Name: name, Efforts: efforts, Source: provider.ModelSourceCodexCatalog, Custom: custom}
	}
	six := []string{"low", "medium", "high", "xhigh", "max", "ultra"}
	return []provider.DiscoveredModel{
		row("gpt-6-sol", "GPT-6-Sol", six, false),
		row("gpt-6-luna", "GPT-6-Luna", []string{"low", "medium", "high", "xhigh", "max"}, false),
		row("gpt-5.6-sol", "GPT-5.6-Sol", six, false),
		row("gpt-5.5", "GPT-5.5", []string{"low", "medium", "high", "xhigh"}, false),
		row("custom/local-proxy-grok-4.7", "Grok 4.7 (local proxy)", []string{"low", "medium", "high", "xhigh"}, true),
		row("custom/local-proxy-gemini-3.8-flash-high", "Gemini 3.8 Flash High (local proxy)", []string{"high"}, true),
		row("claude-cxh-9", "", []string{"low", "high"}, false),
		row("cxh-bell", "", nil, false),
		row("cxh-min", "Cxh Min", []string{"minimal", "low"}, false),
		row("customer-cxh", "Customer Cxh", nil, false),
	}
}

// cxhSuggestedGolden 是 P3 的期望块：P2 解析出的非 custom 行，每行打五个值（格式同 MD1 的 P7）。
const cxhSuggestedGolden = `gpt-6-sol
    def="high" id="gpt-6-sol-high" idmax="gpt-6-sol-max" dn="GPT-6-Sol" dnmax="GPT-6-Sol max"
gpt-6-luna
    def="high" id="gpt-6-luna-high" idmax="gpt-6-luna-max" dn="GPT-6-Luna" dnmax="GPT-6-Luna max"
gpt-5.6-sol
    def="high" id="gpt-5.6-sol-high" idmax="gpt-5.6-sol-max" dn="GPT-5.6-Sol" dnmax="GPT-5.6-Sol max"
gpt-5.5
    def="high" id="gpt-5.5-high" idmax="gpt-5.5-max" dn="GPT-5.5" dnmax="GPT-5.5 max"
claude-cxh-9
    def="high" id="codex-cxh-9-high" idmax="codex-cxh-9-max" dn="Claude Cxh 9" dnmax="Claude Cxh 9 max"
cxh-bell
    def="" id="cxh-bell" idmax="cxh-bell-max" dn="Cxh Bell" dnmax="Cxh Bell max"
cxh-min
    def="minimal" id="cxh-min-minimal" idmax="cxh-min-max" dn="Cxh Min minimal" dnmax="Cxh Min max"
customer-cxh
    def="" id="customer-cxh" idmax="customer-cxh-max" dn="Customer Cxh" dnmax="Customer Cxh max"
`

// cxhRejectRows 是 P4 的表：每一行都得不到结果，只差报哪个哨兵。
var cxhRejectRows = []struct {
	no   int
	in   string
	want string
}{
	{1, "not json", "Unparseable"},
	{2, "", "Unparseable"},
	{3, "null", "Unparseable"},
	{4, "{}", "Unparseable"},
	{5, `{"models":null}`, "Unparseable"},
	{6, `[{"slug":"gpt-6-sol","visibility":"list"}]`, "Unparseable"},
	{7, `{"models":{"slug":"gpt-6-sol","visibility":"list"}}`, "Unparseable"},
	{8, `{"models":[]}`, "Empty"},
	{9, `{"models":[{"slug":"gpt-reserve","visibility":"hide"}]}`, "Empty"},
	{10, `{"models":[{"slug":"custom/local-proxy-grok-4.7","visibility":"list"}]}`, "Empty"},
	{11, `{"models":[{"slug":"","visibility":"list"}]}`, "Empty"},
	{12, `{"models":[{"slug":"a\u0007b","visibility":"list"}]}`, "Empty"},
	{13, `{"models":[{"slug":"gpt-6-sol"}]}`, "Empty"},
	{14, `{"models":[{"slug":"gpt-6-sol","visibility":"List"}]}`, "Empty"},
}
