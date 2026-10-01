package repl

// CXH 契约 §2.0 的夹具块 F1、F2 与期望块 G2（逐字照抄契约，由脚本从契约取出）。

import (
	"strings"
	"testing"
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

// cxhCodexFixture 是整份夹具：24916 字节，一行，末尾一个换行。
func cxhCodexFixture() string {
	return cxhCodexHead + cxhBigModelMessages() + cxhCodexTail
}

// cxhCodexPath 是 cxh 配置里 codex 的 command；假执行器按它查表。
const cxhCodexPath = "cxh-codex.exe"

// cxhConfigTemplate 是 repl 用例的配置文件；<dataDir> 由 mdxConfigText 换成真路径。
// codex 与 grok 两家；gpt-5.5 以 high 已配成 agent（识别表里藏起那一行、不给编号）。
const cxhConfigTemplate = `# cxh 原有注释：这一行与下面每个字节都不许被改动
data_dir = '<dataDir>'

[providers.codex]
command = 'cxh-codex.exe'

[providers.grok]
command = 'mdx-grok.exe'

[agents."grok-4.7"]
display_name = 'Grok 4.7'
provider     = 'grok'
model        = 'grok-4.7'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents."gpt-5.5-high"]
display_name = 'GPT-5.5'
provider     = 'codex'
model        = 'gpt-5.5'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'
`

// cxhOneGpt55 是只有 gpt-5.5 一条记录的 codex 输出（R3）。
const cxhOneGpt55 = `{"models":[{"slug":"gpt-5.5","display_name":"GPT-5.5","default_reasoning_level":"xhigh","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"}]}` + "\n"

// cxhOnlyHiddenAndCustom 是 R6 empty 的 codex 输出：一条 hide、一条 list 的 custom/。
const cxhOnlyHiddenAndCustom = `{"models":[{"slug":"gpt-reserve","visibility":"hide"},{"slug":"custom/local-proxy-grok-4.7","display_name":"Grok 4.7 (local proxy)","visibility":"list"}]}` + "\n"

// cxhCodexOnlyTemplate 是 R8 的配置文件：只有 codex 一家；[agents."gpt-5.5-high"] 那一段与 cxhConfigTemplate 里的逐字相同。
const cxhCodexOnlyTemplate = `# cxh 原有注释：这一行与下面每个字节都不许被改动
data_dir = '<dataDir>'

[providers.codex]
command = 'cxh-codex.exe'

[agents."gpt-5.5-high"]
display_name = 'GPT-5.5'
provider     = 'codex'
model        = 'gpt-5.5'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'
`

// cxhGpt55AndGrokCustom 是 R8 的 codex 输出：cxhOneGpt55 里的那一条 gpt-5.5，再接一条 list 的 custom/（与 cxhRecGrokCustom 同一条记录，没有末尾逗号）。
const cxhGpt55AndGrokCustom = `{"models":[{"slug":"gpt-5.5","display_name":"GPT-5.5","default_reasoning_level":"xhigh","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},{"slug":"custom/local-proxy-grok-4.7","display_name":"Grok 4.7 (local proxy)","default_reasoning_level":"high","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"}]}` + "\n"

// cxhNewRig 用 MD1 的装置（mdxNewRig：真 SQLite、真配置文件、真 Runner、假执行器），
// 再给假执行器登记 codex 的输出。
func cxhNewRig(t *testing.T, tmpl, codexOut string) *mdxRig {
	t.Helper()
	g := mdxNewRig(t, tmpl)
	g.exec.set(cxhCodexPath, mdxResp{out: []byte(codexOut)})
	return g
}

// cxhNote 是夹具里有 2 条 list 的 custom/ 记录时，识别表之后多出的那一行。
const cxhNote = "codex 目录里还有 2 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员"

// cxhNoteOne 是只剩 1 条时的那一行（R2 one）。
const cxhNoteOne = "codex 目录里还有 1 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员"

// cxhListBlock 是 R1 的「识别部分」（配置候选之后的全部行）。
const cxhListBlock = `正在识别模型: codex, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                      MODEL                   EFFORTS                          SOURCE
+1   codex     gpt-6-sol-high          gpt-6-sol               low|medium|high|xhigh|max|ultra  codex 目录
+2   codex     gpt-6-luna-high         gpt-6-luna              low|medium|high|xhigh|max        codex 目录
+3   codex     gpt-5.6-sol-high        gpt-5.6-sol             low|medium|high|xhigh|max|ultra  codex 目录
+4   codex     codex-cxh-9-high        claude-cxh-9            low|high                         codex 目录
+5   codex     cxh-bell                cxh-bell                -                                codex 目录
+6   codex     cxh-min-minimal         cxh-min                 minimal|low                      codex 目录
+7   codex     customer-cxh            customer-cxh            -                                codex 目录
+8   grok      grok-4.6                grok-4.6                -                                本地配置
+9   grok      grok-4.5                grok-4.5                -                                本地配置
+10  grok      grok-4.3                grok-4.3                -                                本地配置
+11  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -                                本地配置
+12  grok      grok-build              grok-build              -                                本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
` + cxhNote

// cxhGrokOnlyTable 是 codex 一行都没显示时的识别表（R3、R6 共用），提示行与 MD1 的原句逐字相同。
const cxhGrokOnlyTable = `识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO  PROVIDER  ID                      MODEL                   EFFORTS  SOURCE
+1  grok      grok-4.6                grok-4.6                -        本地配置
+2  grok      grok-4.5                grok-4.5                -        本地配置
+3  grok      grok-4.3                grok-4.3                -        本地配置
+4  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -        本地配置
+5  grok      grok-build              grok-build              -        本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档`

// cxhAllConfiguredBlock 是 R8 的识别部分：codex 识别出的可选行都已配成 agent，只剩 1 条 custom/。
const cxhAllConfiguredBlock = `正在识别模型: codex
识别出的模型都已经在配置里了
` + cxhNoteOne

// cxhGrokDownBlock 是 R9 的识别部分：grok 识别失败，表里只有 codex 的行（各列宽度只由 codex 的行定，
// 与 cxhListBlock 不同），说明行在失败行之前。
const cxhGrokDownBlock = `正在识别模型: codex, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO  PROVIDER  ID                MODEL         EFFORTS                          SOURCE
+1  codex     gpt-6-sol-high    gpt-6-sol     low|medium|high|xhigh|max|ultra  codex 目录
+2  codex     gpt-6-luna-high   gpt-6-luna    low|medium|high|xhigh|max        codex 目录
+3  codex     gpt-5.6-sol-high  gpt-5.6-sol   low|medium|high|xhigh|max|ultra  codex 目录
+4  codex     codex-cxh-9-high  claude-cxh-9  low|high                         codex 目录
+5  codex     cxh-bell          cxh-bell      -                                codex 目录
+6  codex     cxh-min-minimal   cxh-min       minimal|low                      codex 目录
+7  codex     customer-cxh      customer-cxh  -                                codex 目录
用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
` + cxhNote + `
识别 grok 的模型失败: model discovery command failed: cxh grok down
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh`

// cxhRefusalPairs 是 R4：每对是一条命令与它恰好打出的一行。
const cxhRefusalPairs = `/add +1 --effort minimal
gpt-6-sol 支持的强度是 low|medium|high|xhigh|max|ultra，没有 minimal

/add +2 --effort ultra
gpt-6-luna 支持的强度是 low|medium|high|xhigh|max，没有 ultra

/add +4 --effort turbo
claude-cxh-9 支持的强度是 low|high，没有 turbo

/add +1 --effort HIGH
gpt-6-sol 支持的强度是 low|medium|high|xhigh|max|ultra，没有 HIGH

/add +5 --effort high
cxh-bell 没有可选的强度，不能带 --effort

/add +8 --effort high
grok 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort`

// cxhAddSteps 是 R5 依次敲的六条命令与各自加出的 id。
var cxhAddSteps = []struct{ cmd, id string }{
	{"/add +1", "gpt-6-sol-high"},
	{"/add +1 --effort ultra", "gpt-6-sol-ultra"},
	{"/add +3", "gpt-5.6-sol-high"},
	{"/add +4", "codex-cxh-9-high"},
	{"/add +5", "cxh-bell"},
	{"/add +6", "cxh-min-minimal"},
}

// cxhAppendAll 是 R5 六步之后配置文件相对原文多出的字节。
const cxhAppendAll = "\n# council /add 追加：codex 识别出的 gpt-6-sol（codex 目录）\n[agents.gpt-6-sol-high]\ndisplay_name = \"GPT-6-Sol\"\nprovider = \"codex\"\nmodel = \"gpt-6-sol\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 gpt-6-sol（codex 目录）\n[agents.gpt-6-sol-ultra]\ndisplay_name = \"GPT-6-Sol ultra\"\nprovider = \"codex\"\nmodel = \"gpt-6-sol\"\neffort = \"ultra\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 gpt-5.6-sol（codex 目录）\n[agents.\"gpt-5.6-sol-high\"]\ndisplay_name = \"GPT-5.6-Sol\"\nprovider = \"codex\"\nmodel = \"gpt-5.6-sol\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 claude-cxh-9（codex 目录）\n[agents.codex-cxh-9-high]\ndisplay_name = \"Claude Cxh 9\"\nprovider = \"codex\"\nmodel = \"claude-cxh-9\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 cxh-bell（codex 目录）\n[agents.cxh-bell]\ndisplay_name = \"Cxh Bell\"\nprovider = \"codex\"\nmodel = \"cxh-bell\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 cxh-min（codex 目录）\n[agents.cxh-min-minimal]\ndisplay_name = \"Cxh Min minimal\"\nprovider = \"codex\"\nmodel = \"cxh-min\"\neffort = \"minimal\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"

// cxhAfterBlock 是 R5 六步之后再敲 /add 的识别部分（不再识别，没有「正在识别模型」行）。
const cxhAfterBlock = `识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                      MODEL                   EFFORTS                    SOURCE
+2   codex     gpt-6-luna-high         gpt-6-luna              low|medium|high|xhigh|max  codex 目录
+7   codex     customer-cxh            customer-cxh            -                          codex 目录
+8   grok      grok-4.6                grok-4.6                -                          本地配置
+9   grok      grok-4.5                grok-4.5                -                          本地配置
+10  grok      grok-4.3                grok-4.3                -                          本地配置
+11  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -                          本地配置
+12  grok      grok-build              grok-build              -                          本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
` + cxhNote

// cxhHelpAddLine、cxhHelpJoinLine 是 /help 输出里的两整行（COMMAND 列按 tabwriter 补到 12 个字符）。
const (
	cxhHelpAddLine  = "/add        把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别"
	cxhHelpJoinLine = "/join       同 /add"
)
