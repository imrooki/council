package repl

// mdx_addmodel_fixture_test.go 是 MD1（模型识别 + /add +编号）repl 用例的共用装置，
// 对应冻结契约 md1_contract.md v1.1 的 §2.0 与 §2.4 抬头。
//
// 本文件里的夹具字节、配置文本与各段期望输出都由脚本从契约原文逐字取出，
// 不手抄；改契约时整份重新生成，不要在这里手改某一行。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/store"
	"council/internal/turn"
)

// mdxClaudeFixture 是契约 §2.0 的 claude 夹具（1586 字节）：第 1 行 system 帧，
// 第 2 行 request_id 不匹配的诱饵回应，第 3 行真回应。
const mdxClaudeFixture = `{"type":"system","subtype":"commands_changed","commands":[]}
{"type":"control_response","response":{"subtype":"success","request_id":"mdx-other","response":{"models":[{"value":"decoy","resolvedModel":"claude-decoy-9","displayName":"Decoy"}]}}}
{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[{"value":"default","resolvedModel":"claude-opus-5-5[1m]","displayName":"Default (recommended)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"opus[1m]","resolvedModel":"claude-opus-5-5[1m]","displayName":"Opus (1M context)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"claude-fable-5-1","resolvedModel":"claude-fable-5-1","displayName":"Fable","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"sonnet","resolvedModel":"claude-sonnet-5","displayName":"Sonnet","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"sonnet[1m]","resolvedModel":"claude-sonnet-5[1m]","displayName":"Sonnet 5 (1M context)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"haiku","resolvedModel":"claude-haiku-4-5-20251001","displayName":"Haiku"},{"value":"opus-legacy","resolvedModel":"claude-opus-4-1","displayName":"Opus 4.1","supportedEffortLevels":["high"],"disabled":true},{"value":"mdx-lowmed","resolvedModel":"claude-mdx-lowmed-2-0","displayName":"Mdx","supportedEffortLevels":["low","medium"]},{"value":"claude-mdx-plain-1-2","displayName":"Plain","supportedEffortLevels":["high","max"]}]},"pending_permission_requests":[]}}` + "\n"

// mdxAgyFixture 是契约 §2.0 的 agy 夹具（1823 字节，实跑原件）。
const mdxAgyFixture = `{"conversation_id":"","status":"SUCCESS","response":"gemini-3.8-flash-high\tGemini 3.8 Flash (High)\ngemini-3.8-flash-medium\tGemini 3.8 Flash (Medium)\ngemini-3.8-flash-low\tGemini 3.8 Flash (Low)\ngemini-3.7-flash-high\tGemini 3.7 Flash (High)\ngemini-3.7-flash-medium\tGemini 3.7 Flash (Medium)\ngemini-3.7-flash-low\tGemini 3.7 Flash (Low)\ngemini-3.6-flash-high\tGemini 3.6 Flash (High)\ngemini-3.6-flash-medium\tGemini 3.6 Flash (Medium)\ngemini-3.6-flash-low\tGemini 3.6 Flash (Low)\ngemini-3.1-pro-high\tGemini 3.1 Pro (High)\ngemini-3.1-pro-low\tGemini 3.1 Pro (Low)\nclaude-sonnet-4-6\tClaude Sonnet 4.6 (Thinking)\nclaude-opus-4-6-thinking\tClaude Opus 4.6 (Thinking)\ngpt-oss-120b-medium\tGPT-OSS 120B (Medium)\n","duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0},"command":{"name":"models","data":{"models":[{"id":"gemini-3.8-flash-high","label":"Gemini 3.8 Flash (High)"},{"id":"gemini-3.8-flash-medium","label":"Gemini 3.8 Flash (Medium)"},{"id":"gemini-3.8-flash-low","label":"Gemini 3.8 Flash (Low)"},{"id":"gemini-3.7-flash-high","label":"Gemini 3.7 Flash (High)"},{"id":"gemini-3.7-flash-medium","label":"Gemini 3.7 Flash (Medium)"},{"id":"gemini-3.7-flash-low","label":"Gemini 3.7 Flash (Low)"},{"id":"gemini-3.6-flash-high","label":"Gemini 3.6 Flash (High)"},{"id":"gemini-3.6-flash-medium","label":"Gemini 3.6 Flash (Medium)"},{"id":"gemini-3.6-flash-low","label":"Gemini 3.6 Flash (Low)"},{"id":"gemini-3.1-pro-high","label":"Gemini 3.1 Pro (High)"},{"id":"gemini-3.1-pro-low","label":"Gemini 3.1 Pro (Low)"},{"id":"claude-sonnet-4-6","label":"Claude Sonnet 4.6 (Thinking)"},{"id":"claude-opus-4-6-thinking","label":"Claude Opus 4.6 (Thinking)"},{"id":"gpt-oss-120b-medium","label":"GPT-OSS 120B (Medium)"}]}}}` + "\n"

// mdxGrokFixture 是契约 §2.0 的 grok 夹具（191 字节，实跑原件）。
const mdxGrokFixture = `Model 'grok-4.7' is using its own API key.

Default model: grok-4.7

Available models:
  * grok-4.7 (default)
  - grok-4.6
  - grok-4.5
  - grok-4.3
  - grok-composer-2.5-fast
  - grok-build` + "\n"

// mdxConfigTemplate 是契约 §2.0 的 repl 用例配置文件；<dataDir> 与 <exe> 两个占位符
// 由 mdxConfigText 换成真路径。
const mdxConfigTemplate = `# mdx 原有注释：这一行与下面每个字节都不许被改动
data_dir = '<dataDir>'

[providers.claude]
command = '<exe>'

[providers.agy]
command = 'mdx-agy.exe'

[providers.grok]
command = 'mdx-grok.exe'

[agents."opus-5.0"]
display_name = 'Opus 5.0'
provider     = 'claude'
model        = 'opus[1m]'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents."fable-5.1"]
display_name = 'Fable 5.1'
provider     = 'claude'
model        = 'claude-fable-5-1'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents."sonnet-4.6"]
display_name = 'Sonnet 4.6'
provider     = 'claude'
model        = 'claude-sonnet-4-6'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents."grok-4.7"]
display_name = 'Grok 4.7'
provider     = 'grok'
model        = 'grok-4.7'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents.agy-flash-high]
display_name = 'Agy Gemini 3.8 Flash (High)'
provider     = 'agy'
model        = 'Gemini 3.8 Flash (High)'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[teams.all]
display_name = '全员'
members = ['opus-5.0', 'grok-4.7']` + "\n"

// mdxGrokOnlyTemplate 是 R11 (c) 的配置文件（契约整段冻结）。
const mdxGrokOnlyTemplate = `data_dir = '<dataDir>'

[providers.grok]
command = 'mdx-grok.exe'

[agents."grok-4.7"]
provider = 'grok'
model = 'grok-4.7'

[agents."grok-4.6"]
provider = 'grok'
model = 'grok-4.6'

[agents."grok-4.5"]
provider = 'grok'
model = 'grok-4.5'

[agents."grok-4.3"]
provider = 'grok'
model = 'grok-4.3'

[agents."grok-composer-2.5-fast"]
provider = 'grok'
model = 'grok-composer-2.5-fast'

[agents.grok-build]
provider = 'grok'
model = 'grok-build'` + "\n"

// mdxAppendAll 是契约 §3.5：R3 全部七步之后文件相对原文多出的字节。
const mdxAppendAll = "\n# council /add 追加：claude 识别出的 claude-sonnet-5（订阅）\n[agents.sonnet-5-high]\ndisplay_name = \"Sonnet 5\"\nprovider = \"claude\"\nmodel = \"claude-sonnet-5\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：claude 识别出的 claude-sonnet-5（订阅）\n[agents.sonnet-5-max]\ndisplay_name = \"Sonnet 5 max\"\nprovider = \"claude\"\nmodel = \"claude-sonnet-5\"\neffort = \"max\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：claude 识别出的 claude-mdx-lowmed-2-0（订阅）\n[agents.\"mdx-lowmed-2.0-low\"]\ndisplay_name = \"Mdx Lowmed 2.0 low\"\nprovider = \"claude\"\nmodel = \"claude-mdx-lowmed-2-0\"\neffort = \"low\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：claude 识别出的 claude-haiku-4-5-20251001（订阅）\n[agents.\"haiku-4.5\"]\ndisplay_name = \"Haiku 4.5\"\nprovider = \"claude\"\nmodel = \"claude-haiku-4-5-20251001\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：grok 识别出的 grok-4.6（本地配置）\n[agents.g2]\ndisplay_name = \"Grok 4.6\"\nprovider = \"grok\"\nmodel = \"grok-4.6\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：claude 识别出的 claude-mdx-plain-1-2（订阅）\n[agents.mdx-plain-max]\ndisplay_name = \"Mdx Plain 1.2 max\"\nprovider = \"claude\"\nmodel = \"claude-mdx-plain-1-2\"\neffort = \"max\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"

// 以下是各条用例的期望行，逐字取自契约的代码块。
const (
	mdxR1Block = `正在识别模型: agy, claude, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                       MODEL                         EFFORTS                    SOURCE
+1   agy       gemini-3.8-flash-medium  Gemini 3.8 Flash (Medium)     -                          订阅
+2   agy       gemini-3.8-flash-low     Gemini 3.8 Flash (Low)        -                          订阅
+3   agy       gemini-3.7-flash-high    Gemini 3.7 Flash (High)       -                          订阅
+4   agy       gemini-3.7-flash-medium  Gemini 3.7 Flash (Medium)     -                          订阅
+5   agy       gemini-3.7-flash-low     Gemini 3.7 Flash (Low)        -                          订阅
+6   agy       gemini-3.6-flash-high    Gemini 3.6 Flash (High)       -                          订阅
+7   agy       gemini-3.6-flash-medium  Gemini 3.6 Flash (Medium)     -                          订阅
+8   agy       gemini-3.6-flash-low     Gemini 3.6 Flash (Low)        -                          订阅
+9   agy       gemini-3.1-pro-high      Gemini 3.1 Pro (High)         -                          订阅
+10  agy       gemini-3.1-pro-low       Gemini 3.1 Pro (Low)          -                          订阅
+11  agy       agy-sonnet-4.6-thinking  Claude Sonnet 4.6 (Thinking)  -                          走 agy 额度
+12  agy       agy-opus-4.6-thinking    Claude Opus 4.6 (Thinking)    -                          走 agy 额度
+13  agy       gpt-oss-120b-medium      GPT-OSS 120B (Medium)         -                          走 agy 额度
+14  claude    sonnet-5-high            claude-sonnet-5               low|medium|high|xhigh|max  订阅
+15  claude    sonnet-5-1m-high         claude-sonnet-5[1m]           low|medium|high|xhigh|max  订阅
+16  claude    haiku-4.5                claude-haiku-4-5-20251001     -                          订阅
+17  claude    mdx-lowmed-2.0-low       claude-mdx-lowmed-2-0         low|medium                 订阅
+18  claude    mdx-plain-1.2-high       claude-mdx-plain-1-2          high|max                   订阅
+19  grok      grok-4.6                 grok-4.6                      -                          本地配置
+20  grok      grok-4.5                 grok-4.5                      -                          本地配置
+21  grok      grok-4.3                 grok-4.3                      -                          本地配置
+22  grok      grok-composer-2.5-fast   grok-composer-2.5-fast        -                          本地配置
+23  grok      grok-build               grok-build                    -                          本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档`
	mdxR2Step3Block = `识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                       MODEL                         EFFORTS                    SOURCE
+1   agy       gemini-3.8-flash-medium  Gemini 3.8 Flash (Medium)     -                          订阅
+2   agy       gemini-3.8-flash-low     Gemini 3.8 Flash (Low)        -                          订阅
+3   agy       gemini-3.7-flash-high    Gemini 3.7 Flash (High)       -                          订阅
+4   agy       gemini-3.7-flash-medium  Gemini 3.7 Flash (Medium)     -                          订阅
+5   agy       gemini-3.7-flash-low     Gemini 3.7 Flash (Low)        -                          订阅
+6   agy       gemini-3.6-flash-high    Gemini 3.6 Flash (High)       -                          订阅
+7   agy       gemini-3.6-flash-medium  Gemini 3.6 Flash (Medium)     -                          订阅
+8   agy       gemini-3.6-flash-low     Gemini 3.6 Flash (Low)        -                          订阅
+9   agy       gemini-3.1-pro-high      Gemini 3.1 Pro (High)         -                          订阅
+10  agy       gemini-3.1-pro-low       Gemini 3.1 Pro (Low)          -                          订阅
+11  agy       agy-sonnet-4.6-thinking  Claude Sonnet 4.6 (Thinking)  -                          走 agy 额度
+12  agy       agy-opus-4.6-thinking    Claude Opus 4.6 (Thinking)    -                          走 agy 额度
+13  agy       gpt-oss-120b-medium      GPT-OSS 120B (Medium)         -                          走 agy 额度
+15  claude    sonnet-5-1m-high         claude-sonnet-5[1m]           low|medium|high|xhigh|max  订阅
+16  claude    haiku-4.5                claude-haiku-4-5-20251001     -                          订阅
+17  claude    mdx-lowmed-2.0-low       claude-mdx-lowmed-2-0         low|medium                 订阅
+18  claude    mdx-plain-1.2-high       claude-mdx-plain-1-2          high|max                   订阅
+19  grok      grok-4.6                 grok-4.6                      -                          本地配置
+20  grok      grok-4.5                 grok-4.5                      -                          本地配置
+21  grok      grok-4.3                 grok-4.3                      -                          本地配置
+22  grok      grok-composer-2.5-fast   grok-composer-2.5-fast        -                          本地配置
+23  grok      grok-build               grok-build                    -                          本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档`
	mdxR4Pairs = `/add +x
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +3 --id
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +3 --foo
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add --refresh x
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +3 --id a --id b
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +3 --effort a --effort b
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +99
识别结果里没有 +99，不带参数的 /add 能列出现有编号

/add +0
识别结果里没有 +0，不带参数的 /add 能列出现有编号

/add +99999999999999999999
识别结果里没有 +99999999999999999999，不带参数的 /add 能列出现有编号

/add +16 --effort high
claude-haiku-4-5-20251001 没有可选的强度，不能带 --effort

/add +14 --effort ultra
claude-sonnet-5 支持的强度是 low|medium|high|xhigh|max，没有 ultra

/add +19 --effort high
grok 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort

/add +3 --effort high
agy 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort

/add +14 --id Bad
id Bad 不合法（要匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$）；用 --id <新 id> 换一个

/add +14 --id user
id user 是保留名；用 --id <新 id> 换一个

/add +14 --id nul
id nul 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个

/add +14 --id com1.x
id com1.x 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个

/add +14 --id opus-5.0
配置里已经有 agent opus-5.0；用 --id <新 id> 换一个

/add +14 --id opus-5_0
id opus-5_0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个

/add +14 --id opus_5.0
id opus_5.0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个

/add +
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add 1,+2
配置里没有可加入的 +2，不带参数的 /add 能列出可加的`
	mdxR6EachKind = `正在识别模型: agy, claude, grok
识别 agy 的模型失败: 一个模型都没列出来（没登录？）
识别 claude 的模型失败: 超时（200ms 内没有回来）
识别 grok 的模型失败: model discovery command failed: mdx boom
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh`
	mdxR6Unparseable = `正在识别模型: agy, claude, grok
识别 agy 的模型失败: 输出认不出来（CLI 版本变了？）
识别 claude 的模型失败: 输出认不出来（CLI 版本变了？）
识别 grok 的模型失败: 一个模型都没列出来（没登录？）
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh`
	mdxR6AllDown = `正在识别模型: agy, claude, grok
识别 agy 的模型失败: model discovery command failed: mdx agy down
识别 claude 的模型失败: model discovery command failed: mdx claude down
识别 grok 的模型失败: model discovery command failed: mdx grok down
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh`
	mdxR6OneDown = `正在识别模型: agy, claude, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                       MODEL                         EFFORTS                    SOURCE
+1   agy       gemini-3.8-flash-medium  Gemini 3.8 Flash (Medium)     -                          订阅
+2   agy       gemini-3.8-flash-low     Gemini 3.8 Flash (Low)        -                          订阅
+3   agy       gemini-3.7-flash-high    Gemini 3.7 Flash (High)       -                          订阅
+4   agy       gemini-3.7-flash-medium  Gemini 3.7 Flash (Medium)     -                          订阅
+5   agy       gemini-3.7-flash-low     Gemini 3.7 Flash (Low)        -                          订阅
+6   agy       gemini-3.6-flash-high    Gemini 3.6 Flash (High)       -                          订阅
+7   agy       gemini-3.6-flash-medium  Gemini 3.6 Flash (Medium)     -                          订阅
+8   agy       gemini-3.6-flash-low     Gemini 3.6 Flash (Low)        -                          订阅
+9   agy       gemini-3.1-pro-high      Gemini 3.1 Pro (High)         -                          订阅
+10  agy       gemini-3.1-pro-low       Gemini 3.1 Pro (Low)          -                          订阅
+11  agy       agy-sonnet-4.6-thinking  Claude Sonnet 4.6 (Thinking)  -                          走 agy 额度
+12  agy       agy-opus-4.6-thinking    Claude Opus 4.6 (Thinking)    -                          走 agy 额度
+13  agy       gpt-oss-120b-medium      GPT-OSS 120B (Medium)         -                          走 agy 额度
+14  claude    sonnet-5-high            claude-sonnet-5               low|medium|high|xhigh|max  订阅
+15  claude    sonnet-5-1m-high         claude-sonnet-5[1m]           low|medium|high|xhigh|max  订阅
+16  claude    haiku-4.5                claude-haiku-4-5-20251001     -                          订阅
+17  claude    mdx-lowmed-2.0-low       claude-mdx-lowmed-2-0         low|medium                 订阅
+18  claude    mdx-plain-1.2-high       claude-mdx-plain-1-2          high|max                   订阅
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
识别 grok 的模型失败: model discovery command failed: mdx grok down
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh`
	mdxR11aBlock = `配置里还没有可以 /add 的成员
正在识别模型: claude
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO  PROVIDER  ID                  MODEL                      EFFORTS                    SOURCE
+1  claude    opus-5.5-1m-high    claude-opus-5-5[1m]        low|medium|high|xhigh|max  订阅
+2  claude    fable-5.1-high      claude-fable-5-1           low|medium|high|xhigh|max  订阅
+3  claude    sonnet-5-high       claude-sonnet-5            low|medium|high|xhigh|max  订阅
+4  claude    sonnet-5-1m-high    claude-sonnet-5[1m]        low|medium|high|xhigh|max  订阅
+5  claude    haiku-4.5           claude-haiku-4-5-20251001  -                          订阅
+6  claude    mdx-lowmed-2.0-low  claude-mdx-lowmed-2-0      low|medium                 订阅
+7  claude    mdx-plain-1.2-high  claude-mdx-plain-1-2       high|max                   订阅
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档`
	mdxR11cBlock = `正在识别模型: grok
识别出的模型都已经在配置里了`
	mdxR12Block = `正在识别模型: agy, claude, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                       MODEL                         EFFORTS                    SOURCE
+1   agy       gemini-3.8-flash-medium  Gemini 3.8 Flash (Medium)     -                          订阅
+2   agy       gemini-3.8-flash-low     Gemini 3.8 Flash (Low)        -                          订阅
+3   agy       gemini-3.7-flash-high    Gemini 3.7 Flash (High)       -                          订阅
+4   agy       gemini-3.7-flash-medium  Gemini 3.7 Flash (Medium)     -                          订阅
+5   agy       gemini-3.7-flash-low     Gemini 3.7 Flash (Low)        -                          订阅
+6   agy       gemini-3.6-flash-high    Gemini 3.6 Flash (High)       -                          订阅
+7   agy       gemini-3.6-flash-medium  Gemini 3.6 Flash (Medium)     -                          订阅
+8   agy       gemini-3.6-flash-low     Gemini 3.6 Flash (Low)        -                          订阅
+9   agy       gemini-3.1-pro-high      Gemini 3.1 Pro (High)         -                          订阅
+10  agy       gemini-3.1-pro-low       Gemini 3.1 Pro (Low)          -                          订阅
+11  agy       agy-sonnet-4.6-thinking  Claude Sonnet 4.6 (Thinking)  -                          走 agy 额度
+12  agy       agy-opus-4.6-thinking    Claude Opus 4.6 (Thinking)    -                          走 agy 额度
+13  agy       gpt-oss-120b-medium      GPT-OSS 120B (Medium)         -                          走 agy 额度
+14  claude    opus-5.5-1m-high         claude-opus-5-5[1m]           low|medium|high|xhigh|max  订阅
+15  claude    sonnet-5-high            claude-sonnet-5               low|medium|high|xhigh|max  订阅
+16  claude    sonnet-5-1m-high         claude-sonnet-5[1m]           low|medium|high|xhigh|max  订阅
+17  claude    haiku-4.5                claude-haiku-4-5-20251001     -                          订阅
+18  claude    mdx-lowmed-2.0-low       claude-mdx-lowmed-2-0         low|medium                 订阅
+19  claude    mdx-plain-1.2-high       claude-mdx-plain-1-2          high|max                   订阅
+20  grok      grok-4.6                 grok-4.6                      -                          本地配置
+21  grok      grok-4.5                 grok-4.5                      -                          本地配置
+22  grok      grok-4.3                 grok-4.3                      -                          本地配置
+23  grok      grok-composer-2.5-fast   grok-composer-2.5-fast        -                          本地配置
+24  grok      grok-build               grok-build                    -                          本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档`
	mdxR14CancelBlock = `正在识别模型: agy, claude, grok
识别被取消；下次 /add 会重新识别`
)

// 既有字面量（不属于本刀，用来定位「配置候选」那一段的结尾）。
const (
	mdxCandHeader  = "NO  AGENT           PROVIDER  DISPLAY_NAME"
	mdxCandHint    = "用 /add <编号|全名|前缀> 加入，多个用空格或逗号隔开，例如 /add 1,2"
	mdxAllPresent  = "配置里的 agent 都已经在当前会话里了"
	mdxAppliedLine = "已应用新配置"
)

const (
	mdxAgyPath  = "mdx-agy.exe"
	mdxGrokPath = "mdx-grok.exe"
)

// mdxBakPattern 是契约 §2.4 抬头给的 <bak> 文件名形状。
var mdxBakPattern = regexp.MustCompile(`^config\.toml\.bak-add-\d{8}-\d{6}(-\d+)?$`)

// mdxSplit 把一段期望文本切成行。
func mdxSplit(s string) []string {
	return strings.Split(s, "\n")
}

// mdxLines 把一次 dispatch 的输出增量按 "\n" 切行；末尾那个换行不产生空行。
func mdxLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// mdxConfigText 把模板里的 <dataDir>、<exe> 换成真路径（TOML 单引号字面串原样拼）。
func mdxConfigText(tmpl, dir string) string {
	s := strings.ReplaceAll(tmpl, "<dataDir>", filepath.Join(dir, "data"))
	return strings.ReplaceAll(s, "<exe>", os.Args[0])
}

// mdxReplaceOnce 替换恰好一处；不是恰好一处就是装置的前提坏了。
func mdxReplaceOnce(t *testing.T, s, old, repl string) string {
	t.Helper()
	if n := strings.Count(s, old); n != 1 {
		t.Fatalf("前提：%q 应当恰好出现一处，实际 %d 处", old, n)
	}
	return strings.Replace(s, old, repl, 1)
}

// mdxResp 是假执行器对一个 Path 的回应。
type mdxResp struct {
	out     []byte
	err     error
	timeout bool // 照契约 §2.0 第 4 步的超时写法：等 ctx 结束，3 s 防挂
}

// mdxExec 是契约 §2.0 的假执行器，按 c.Path 查表。每次调用的四步顺序冻结：
// 记录调用（加锁）-> 钩子 -> ctx 已结束就返回 ctx.Err() -> 按查表结果返回。
type mdxExec struct {
	mu    sync.Mutex
	calls []provider.DiscoveryCommand
	table map[string]mdxResp
	hook  func(n int, c provider.DiscoveryCommand)
}

func mdxNewExec() *mdxExec {
	return &mdxExec{table: map[string]mdxResp{
		os.Args[0]:  {out: []byte(mdxClaudeFixture)},
		mdxAgyPath:  {out: []byte(mdxAgyFixture)},
		mdxGrokPath: {out: []byte(mdxGrokFixture)},
	}}
}

func (e *mdxExec) set(path string, r mdxResp) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.table[path] = r
}

func (e *mdxExec) run(ctx context.Context, c provider.DiscoveryCommand) ([]byte, error) {
	// 第 1 步：记下这一次调用。调用次数的断言只看这一步。
	e.mu.Lock()
	e.calls = append(e.calls, c)
	n := len(e.calls)
	hook := e.hook
	resp, ok := e.table[c.Path]
	e.mu.Unlock()
	// 第 2 步：钩子（只有 R14 装）。
	if hook != nil {
		hook(n, c)
	}
	// 第 3 步：ctx 已经结束就照真执行器的样子返回 ctx 的错误。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 第 4 步：按查表结果返回。
	if !ok {
		return nil, fmt.Errorf("mdx: no fixture for path %q", c.Path)
	}
	if resp.timeout {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
			return nil, errors.New("mdx: ctx never cancelled")
		}
	}
	if resp.err != nil {
		return nil, resp.err
	}
	return append([]byte(nil), resp.out...), nil
}

func (e *mdxExec) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

func (e *mdxExec) callsCopy() []provider.DiscoveryCommand {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]provider.DiscoveryCommand(nil), e.calls...)
}

// mdxRigOpts 是 R9 要的两种残缺装置。
type mdxRigOpts struct {
	noRunner bool
	noPath   bool
}

// mdxRig 是契约 §2.4 抬头的共用装置：真 SQLite、真配置文件、真 Runner。
type mdxRig struct {
	t       *testing.T
	ctx     context.Context
	dir     string
	cfgPath string
	out     *bytes.Buffer
	r       *REPL
	svc     *conversation.Service
	conv    conversation.Conversation
	exec    *mdxExec
}

func mdxNewRig(t *testing.T, cfgTmpl string) *mdxRig {
	t.Helper()
	return mdxNewRigOpts(t, cfgTmpl, mdxRigOpts{})
}

func mdxNewRigOpts(t *testing.T, cfgTmpl string, opts mdxRigOpts) *mdxRig {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	ctx := context.Background()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(mdxConfigText(cfgTmpl, dir)), 0o644); err != nil {
		t.Fatalf("写配置文件: %v", err)
	}
	cfg, err := config.LoadAndValidate(cfgPath)
	if err != nil {
		t.Fatalf("前提：配置应当能加载并通过校验: %v", err)
	}
	st, err := store.Open(ctx, filepath.Join(dir, "data"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := conversation.NewService(st)
	conv, err := svc.Create(ctx, "mdx")
	if err != nil {
		t.Fatalf("svc.Create: %v", err)
	}
	var rn *turn.Runner
	if !opts.noRunner {
		rn = turn.NewRunner(cfg, svc, filepath.Join(dir, "workdir"), turn.RulesBlocks{})
	}
	rc := RulesContext{ConfigPath: cfgPath}
	if opts.noPath {
		rc = RulesContext{}
	}
	out := &bytes.Buffer{}
	r := New(cfg, svc, rn, strings.NewReader(""), out, rc)
	ex := mdxNewExec()
	r.discover = ex.run
	r.discoverTimeout = 200 * time.Millisecond
	g := &mdxRig{t: t, ctx: ctx, dir: dir, cfgPath: cfgPath, out: out, r: r, svc: svc, conv: conv, exec: ex}
	g.run("/use " + conv.ID)
	return g
}

// run 用装置的 ctx 喂一行，返回这一行的输出增量（按行）。
func (g *mdxRig) run(line string) []string {
	g.t.Helper()
	return g.runCtx(g.ctx, line)
}

// runCtx 喂一行；dispatch 的 panic 在这里接住并判红，只挂当前（子）测试。
func (g *mdxRig) runCtx(ctx context.Context, line string) []string {
	g.t.Helper()
	before := g.out.Len()
	var panicked any
	var err error
	func() {
		defer func() { panicked = recover() }()
		err = g.r.dispatch(ctx, line)
	}()
	if panicked != nil {
		g.t.Fatalf("dispatch(%q) panic: %v", line, panicked)
	}
	if err != nil {
		g.t.Fatalf("dispatch(%q) 返回错误: %v", line, err)
	}
	return mdxLines(g.out.String()[before:])
}

// afterCandidates 返回 /add 输出里「配置候选」之后的行（契约说的「识别部分」）。
// 配置候选以既有提示行或「都已经在当前会话里了」结尾；找不到就是输出形状坏了。
func (g *mdxRig) afterCandidates(lines []string) []string {
	g.t.Helper()
	for i, l := range lines {
		if l == mdxCandHint || l == mdxAllPresent {
			return lines[i+1:]
		}
	}
	g.t.Fatalf("输出里找不到配置候选的结尾行：%q", lines)
	return nil
}

func (g *mdxRig) readFile() []byte {
	g.t.Helper()
	b, err := os.ReadFile(g.cfgPath)
	if err != nil {
		g.t.Fatalf("读配置文件: %v", err)
	}
	return b
}

func (g *mdxRig) writeFile(b []byte) {
	g.t.Helper()
	if err := os.WriteFile(g.cfgPath, b, 0o644); err != nil {
		g.t.Fatalf("写配置文件: %v", err)
	}
}

// dirNames 是 dir 里的全部条目名（排序）。
func (g *mdxRig) dirNames() []string {
	g.t.Helper()
	ents, err := os.ReadDir(g.dir)
	if err != nil {
		g.t.Fatalf("ReadDir: %v", err)
	}
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// bakNames 是 dir 里名字匹配 <bak> 形状的文件（排序）。
func (g *mdxRig) bakNames() []string {
	g.t.Helper()
	var out []string
	for _, n := range g.dirNames() {
		if mdxBakPattern.MatchString(n) {
			out = append(out, n)
		}
	}
	return out
}

// newBak 返回 before 之后新出现的那一个备份文件的完整路径；不是恰好一个就判红。
func (g *mdxRig) newBak(before []string) string {
	g.t.Helper()
	old := make(map[string]bool, len(before))
	for _, n := range before {
		old[n] = true
	}
	var fresh []string
	for _, n := range g.bakNames() {
		if !old[n] {
			fresh = append(fresh, n)
		}
	}
	if len(fresh) != 1 {
		g.t.Fatalf("应当恰好新出现一个备份文件，实际 %q", fresh)
	}
	return filepath.Join(g.dir, fresh[0])
}

// participants 是当前会话的全部成员行（含已退出的）。
func (g *mdxRig) participants() []conversation.Participant {
	g.t.Helper()
	ps, err := g.svc.Participants(g.ctx, g.conv.ID, true)
	if err != nil {
		g.t.Fatalf("Participants: %v", err)
	}
	return ps
}

// activeIDs 是当前会话在场成员的 id（排序）。
func (g *mdxRig) activeIDs() []string {
	g.t.Helper()
	var out []string
	for _, p := range g.participants() {
		if p.Active {
			out = append(out, p.AgentID)
		}
	}
	sort.Strings(out)
	return out
}

// joinedLines 是 /add +编号 成功时恰好打的两行。
func (g *mdxRig) joinedLines(id, bak string) []string {
	return []string{
		"已把 agent " + id + " 追加进 " + g.cfgPath + "（原文件备份在 " + bak + "）",
		id + " 已加入会话 " + g.conv.ID,
	}
}

// mdxWantLines 整行逐字比较两组行；不等时把两边逐行印出来。
func mdxWantLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	var b strings.Builder
	n := len(got)
	if len(want) > n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		var gl, wl string
		if i < len(got) {
			gl = got[i]
		} else {
			gl = "<缺>"
		}
		if i < len(want) {
			wl = want[i]
		} else {
			wl = "<多>"
		}
		mark := "  "
		if gl != wl {
			mark = "!!"
		}
		fmt.Fprintf(&b, "%s %2d got  %q\n   %2d want %q\n", mark, i, gl, i, wl)
	}
	t.Errorf("%s 不等（got %d 行，want %d 行）:\n%s", what, len(got), len(want), b.String())
}

// mdxHasLine 报告 lines 里是否有与 want 整行相等的一行。
func mdxHasLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

// TestMdxFixturesIntact 核对夹具字节数与契约 §2.0 写的一致，防止生成脚本或编辑链路改坏夹具。
// 【判别力】这是对测试自身的结构性核对，不对应任何被测变异。
func TestMdxFixturesIntact(t *testing.T) {
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"claude", len(mdxClaudeFixture), 1586},
		{"agy", len(mdxAgyFixture), 1823},
		{"grok", len(mdxGrokFixture), 191},
	} {
		if c.got != c.want {
			t.Errorf("%s 夹具 %d 字节，契约写的是 %d", c.name, c.got, c.want)
		}
	}
	if n := len(mdxSplit(mdxR1Block)); n != 27 {
		t.Errorf("R1 期望块 %d 行，契约写的是 27", n)
	}
	if n := len(mdxSplit(mdxR2Step3Block)); n != 25 {
		t.Errorf("R2 第 3 步期望块 %d 行，契约写的是 25", n)
	}
}
