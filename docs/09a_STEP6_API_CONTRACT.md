# Step 6 API 契约（冻结）

`09_STEP6_SPEC.md` 说「做什么」，本文件说「导出的符号长什么样」。
多个执行者并行写不同的包，这是它们之间唯一的接口约定。
**实现者不得改动本文件里的签名；觉得签名有问题，退回来说，不要自行改。**

08a 留下的教训已经吸收进来了：本文件末尾有「不属于契约面的东西」一节，
那些东西实现可以自由决定，测试不得对它们做字面断言。本文件里出现的 Go 代码
只给签名与必须逐字照抄的常量/结构；没有给出函数体的地方，意味着实现方式不冻结，
但输入输出的形状冻结。

## internal/provider

契约层，三家适配器的公共接口。**不导入 `internal/conversation`，不导入
`internal/store`，不导入 `database/sql`，不起进程。**

### capabilities.go

```go
package provider

// Support 是三态能力位。02 第 1.2 节的硬约束：Capabilities 的每一项都要能表达
// Unsupported 与 Unknown，绝不用一个默认值假装支持。零值是 Unknown，
// 漏填一项的后果是「未实测」而不是「支持」。
type Support int

const (
	SupportUnknown Support = iota
	SupportYes
	SupportNo
)

// SessionIDPolicy 说明一家 CLI 的会话 id 从哪来（D-6）。
//
// Step 6 的编排层一次都不读它：判定「要不要新建会话」只看
// TurnRequest.ProviderSession 是不是空串。它在这里是给 Step 9 排并发、
// 给启动自检、给 conformance 测试用的元数据。不声明就等于撒谎，所以仍然要声明。
// 有一条 conformance 测试扫 internal/turn 的源文件，断言这个类型名一次都不出现
// 在编排层里（09_STEP6_SPEC.md D 节）。
type SessionIDPolicy int

const (
	SessionIDUnknown       SessionIDPolicy = iota
	SessionIDPreassignable                 // grok --session-id <uuid>：spawn 前可定，须是合法 UUID 且不得已存在
	SessionIDReturnedOnFirstTurn           // claude --resume / agy --conversation：只能从输出里取回
	SessionIDUnsupported
)

type EffortPolicy int

const (
	EffortUnknown     EffortPolicy = iota
	EffortPerTurnFlag              // claude --effort / agy --effort：真的会放上命令行
	// EffortProcessExternalConfig: 命令行传了也没用，真正生效的在进程外的配置里。
	// grok 经本机 CPA 就是这一档（02 第 1.1 节实测：能力门静默丢弃，退出码仍是 0），
	// 所以它的 BuildTurn 干脆不发 effort flag —— 明知会被吞还发，是在给自己伪造证据。
	EffortProcessExternalConfig
	EffortBakedIntoModelLabel
	EffortUnsupported
)

// String 给 Meta["effort_policy"] 用，取值 unknown / per_turn_flag /
// process_external_config / baked_into_model_label / unsupported。
func (p EffortPolicy) String() string

type IdentityPolicy int

const (
	IdentityUnknown             IdentityPolicy = iota
	IdentitySystemPromptAppend                 // claude --append-system-prompt：追加，盖不住用户全局规则文件
	IdentitySystemPromptOverride               // grok --system-prompt-override：替换
	IdentityPromptPreamble                     // 没有可用的文本 flag，只能把身份写进首轮正文（Step 6 的 agy）
	IdentityAgentPreset                        // agy --agent 收的是预设名，不是任意文本
	IdentityUnsupported
)

// PromptTransport 说明正文怎么递给子进程。它是一等能力项而不是实现细节：
// Windows 命令行上限约 32767 字符，而 Council 的典型用法就是贴一段需求进去（未决 #12）；
// 且 .cmd / .bat 垫片由 cmd.exe 承接，Go 不做 cmd.exe 转义，正文里的 & | % ^ " 是
// 数据相关的风险 —— 同一个 agent 对某些句子成功、对另一些失败。
// 三家在 -p 模式下读不读 stdin 本机未实测，所以零值是 Unknown，
// 09_STEP6_SPEC.md A 节探针 2 测它。
type PromptTransport int

const (
	PromptTransportUnknown PromptTransport = iota
	PromptTransportArgv
	PromptTransportStdin
	PromptTransportFile
)

// PermissionTier 是 06 第 2 节的三档。定义在本包而不是引用 internal/config，
// 是为了不让 provider 依赖配置层；翻译由 internal/turn 做一次，方向是 turn -> provider。
type PermissionTier string

const (
	PermissionRead      PermissionTier = "read"
	PermissionWorkspace PermissionTier = "workspace"
	PermissionFull      PermissionTier = "full"
)

// Capabilities 描述一家 CLI 对 Council 声明了什么、没声明什么。
//
// UsageFormula 是本次修订新增的字段（09_STEP6_SPEC.md F 节的裁决）：
// turn_usage.input_tokens 存的是「本轮完整 prompt」，但三家的计算公式不是同一个，
// 而是三条不同的算法（claude 三项相加、grok 两项相加、agy 原样）。这条公式曾经
// 只写在 Usage 类型的 Go 注释里，现在提到 Capabilities 上、随 Capabilities() 的
// 实现一起声明，一处定义、机器可读，Step 7 做成本视图时第一件要读的就是这个字段。
// 内容是人类可读的公式描述，不是可执行表达式，例如：
//
//	"input_tokens + cache_creation_input_tokens + cache_read_input_tokens"
//	"input_tokens + cache_read_input_tokens"
//	"input_tokens"
type Capabilities struct {
	SessionID      SessionIDPolicy
	Effort         EffortPolicy
	Identity       IdentityPolicy
	Prompt         PromptTransport
	MaxPromptBytes int              // 0 表示不限。Argv 传输时这是硬上限，不是建议值
	Permissions    []PermissionTier // 本适配器这一步真会翻译的档；没列出的一律拒绝，不静默降档
	UsageFormula   string           // 本轮完整 prompt 的计算公式，人类可读，一处定义

	OutputSchema         Support
	MaxTurnsFlag         Support
	ReportsCost          Support // 只有 claude 是 SupportYes
	ReportsCacheCreate   Support // agy 是 SupportNo：它的 usage 里根本没有这一项
	ReportsObservedModel Support // 输出里看得出实际服务的型号；agy 是 SupportNo

	// CleanInstructionEnv 报告这家 CLI 能不能被隔离出用户的全局规则文件（D-5 / 06 第 4 节）。
	//
	// **2026-09-21 订正**：本行原写「三家全部 SupportUnknown：本机没实测过」。
	// 已经测过了，而且三家分出了三种取值：
	//   claude = SupportYes —— --safe-mode 关掉 CLAUDE.md 自动发现、skills、
	//     plugins、hooks、MCP，而明写保留 auth、model、built-in tools 与 permissions。
	//     它绕开了原本那个死结（改 HOME 隔离规则文件会把登录态一起带走）。
	//     实测：身份块里的标记照常出现，用户全局规则的痕迹消失。
	//   grok = SupportNo —— 不是「还没测」，是测过了做不到。grok inspect 直接
	//     列出它读用户的 ~/.claude/Claude.md 与 settings.json 外加 51 个用户 skill；
	//     重定向 CLAUDE_CONFIG_DIR 无效，--system-prompt-override 也压不住。
	//   agy = SupportUnknown —— 仍然没测。
	//
	// 这一位现在是**三态都用上了**的字段，也是「未实测就不声明支持」这条纪律
	// 唯一一处三种取值同时出现的地方。测试应当钉住「每一位都有依据」，
	// 而不是钉住某一个具体取值。
	CleanInstructionEnv Support

	// WriteBoundary 报告这家 CLI 能不能证明自己把文件写限制在指定目录内。
	//
	// **2026-09-21 新增（11c_STEP8B_WORKSPACE_CONTRACT.md 第 2 节）**：这一位
	// 不是「有没有权限 flag」，是「那道 flag 实测围不围得住」——本节上面
	// CleanInstructionEnv 那次订正就是在防同一类混淆（flag 写上了，有效权限
	// 却是「flag 与用户配置文件的合集」）。claude = SupportYes（--restricted
	// 实测围得住，目录内写得进、兄弟目录被拒）；agy = SupportYes（--add-dir
	// 之外被拒，拒绝理由记在 denied_actions；2026-09-21 第二片起 Council
	// 解析这个字段，见 agy.go 与 11c_STEP8B_WORKSPACE_CONTRACT.md 第 7 节）；
	// grok = SupportNo（--deny 是工具级黑名单，路径
	// 模式静默不生效）。零值 SupportUnknown 同样 fail-closed：新适配器声明
	// 了 workspace 却漏填这一位，会被 validateRequest 拒掉。
	WriteBoundary Support
}
```

### turn.go

```go
package provider

// TurnRequest 是一轮调用的全部输入。适配器不得从别处读任何东西来补它。
type TurnRequest struct {
	AgentID     string
	DisplayName string
	Model       string // 请求值（opus / grok-4.6），Council 核心不解释（02 第 2 节）
	Effort      string // 同上；EffortProcessExternalConfig 的适配器必须丢弃它并记进 Command.Dropped
	Role        string // 身份文本，来自 AgentConfig.Role，由 turn 包渲染成完整身份块
	Permission  PermissionTier

	// ProviderSession 是 CLI 那边的会话 id。
	// 空串是「这一轮要新建会话」的唯一信号，也是适配器层唯一一处
	// 「首轮与后续轮」的区别（09_STEP6_SPEC.md D 节）。只有 BuildTurn 读它。
	ProviderSession string

	// Prompt 是已渲染好的 delta，适配器不得改写它的任何一个字节。
	Prompt string

	// WorkDir 是子进程的 cwd，必须非空。空值时 BuildTurn 返回 ErrNoWorkDir：
	// 继承 council 的启动目录会让三家 CLI 读到 cwd 下的项目级指令文件，
	// 入场费与行为随启动目录漂移、不可复现，而且拿到写盘权限的成员会把文件写进
	// 那个仓库（直接违反 06 第 4 节）。
	WorkDir string

	MaxTurns int // 0 表示不设；只有 MaxTurnsFlag == SupportYes 才会被用上（06 第 5 节）
}

// Command 是翻译好的命令行，纯数据，Runner 照着它起进程。
// 不经任何 shell：Go 的 exec 直接起可执行文件，Args 逐元素传、绝不拼成一个字符串
// （ProviderConfig.Command 是单个可执行文件路径；若用 strings.Fields 切，
// "C:\Program Files\...\grok.cmd" 会被空格切断，去执行一个不存在的 C:\Program）。
type Command struct {
	Path  string // 来自 config.ProviderConfig.Command，已 LookPath 解析
	Args  []string
	Dir   string   // 来自 TurnRequest.WorkDir，必须非空
	Env   []string // 完整环境，不是增量。nil 表示原样继承 —— Step 6 一律 nil，不改 HOME
	Stdin []byte   // Transport 是 Stdin 时非空；否则必须是 nil

	// PlannedSessionID 只有 SessionIDPreassignable 的适配器会填：它刚铸了一个 UUID
	// 并写进了 Args。它只在两处场合被读取，除此之外不承担任何其它职责：
	//  1. 与 TurnResult.ProviderSession 做漂移比对（09_STEP6_SPEC.md D 节配套规则 3）；
	//  2. 失败时在诊断信息里点名这个可能的孤儿会话。
	// 它从不被当成「我们已经有会话了」：没有被输出确认过的 id 一律作废，下一轮重铸。
	PlannedSessionID string

	Transport PromptTransport
	Dropped   []DroppedOption
}

// DroppedOption 记下因为 provider 不支持而没能放上命令行的请求项。
// 它不是错误，但必须可见：静默丢弃正是 grok 那个 effort 坑的根源（02 第 1.1 节）。
// 它整体进 messages.meta["dropped"]。
type DroppedOption struct {
	Option string // "effort" / "max_turns" / "output_schema"
	Value  string
	Reason string
}

// Completion 说明这一轮的正文完整不完整。它与「成功 / 失败」是两件事：
// 截断了的回答是一次成功的调用，delta 确实已经被 provider 吃下去了。
type Completion int

const (
	CompletionUnknown   Completion = iota
	CompletionComplete             // claude stop_reason=end_turn / grok stopReason=end_turn / agy status=SUCCESS
	CompletionTruncated            // 顶到 max tokens 或 --max-turns：正文有效但话说了一半，必须标出来
	CompletionEmpty                // 成功但正文是空串：照样落库、照样计费，但屏幕上要响亮地说这一轮是空的
)

type CostBasis int

const (
	CostBasisNone              CostBasis = iota // grok / agy：订阅制，输出里根本没有金额字段
	CostBasisCumulativeSession                  // claude：total_cost_usd 是会话累计值
	CostBasisPerTurn                            // 目前没有一家是这个。留着是为了别把 None 当成 PerTurn
)

// Usage 是归一化到「本轮」的用量。
//
// InputTokens 统一定义为本轮完整 prompt 的规模。三家各自的计算公式不写死在这里，
// 而是由各适配器在 Capabilities.UsageFormula 里声明（见 capabilities.go 的注释），
// 一处定义。三家的原始 usage 对象整个存进 messages.meta["usage_raw"]，将来改口径
// 可回算。跨 provider 比较之前，先看 Meta["usage_raw"] 与各自的 UsageFormula——
// 这一列的数字来自三条不同的算法，不是同一个数量级的噪声。
//
// CacheCreate / CacheRead 是 InputTokens 的子划分。
// ThinkingTokens 已经包含在 OutputTokens 里，任何视图都不许把两者相加
// （grok 723 含 722、agy 55 含 54 均实测；claude 三轮 thinking 恒为 0，
// 包含关系未能被证实，按同规则处理）。
type Usage struct {
	InputTokens    int
	CacheCreate    *int // nil = 这家根本没报这一项（agy）。绝不用 0 冒充（05 第 3 节）
	CacheRead      int
	OutputTokens   int
	ThinkingTokens int

	// CostUSDTotal 是 provider 报的金额原值。claude 是会话累计值，
	// 名字里带 Total 是为了让任何人拿到它都先想起来这一点。差值由 turn.Runner 算。
	CostUSDTotal *float64
	CostBasis    CostBasis

	// Unreported 列出上面哪些 0 是「provider 没报这个数」而不是「实测为 0」。
	// 实测两处：agy 的 usage 里完全没有 cache_creation 这一项；
	// grok 的 cache_creation_input_tokens 在 t1 与 t2 都是 0，而 t2 明明读了 20608 的
	// 缓存 —— 两处都必须进这张表，否则成本视图会显示这两家从不建缓存。
	// turn_usage.cache_create 是 NOT NULL DEFAULT 0，这张表是 Step 6 唯一能说实话的
	// 地方，它整体进 messages.meta["usage_unreported"]。
	Unreported []string
}

// TurnResult 是 ParseTurn 从一段 stdout 单独能诚实知道的全部。
//
// 三家的取值表、顶层键的逐字清单、以及三条绝对禁令（会话 id 只许取指定的键、
// token 只许取 usage 不许取 modelUsage、num_turns 不可用于任何判定），
// 逐字取自 docs/05_MESSAGE_PROTOCOL.md 第 3 节与 E:\Isolating_test\council\step0-probe
// 的七份原件，以那两处为准，本文件不重复抄一遍会走失同步的表。
type TurnResult struct {
	Text            string
	Thought         string
	ProviderSession string // 三家必须都填；grok 填的是它自己铸的、被 CLI 原样回显的那个。空则 ErrNoSessionID
	ObservedModel   string // modelUsage 恰好一个键时取那个键；零个或多于一个时留空（见 ModelKeys）
	// ModelKeys 是 modelUsage 的键数。**只有多于一个键才是 ambiguous**，
	// 零个键不是「有歧义」而是「这家根本不报实测型号」，那一档是 requested。
	// 两者必须分开：agy 没有 modelUsage，claude 失败时 modelUsage 是 {}，
	// 把它们都打成 ambiguous 会让每一条 agy 发言都带着一个假的歧义标记。
	// （本行此前写的是「!= 1 时记 ambiguous」，与 09_STEP6_SPEC.md 的
	// 「多于一个键时」冲突，以 09 为准，2026-09-21 订正。）
	ModelKeys int

	// UsageRaw 是这家 provider 的 usage 子对象**原始字节**，原样不动。
	//
	// 它不是可有可无的冗余：09 F 节与 05 §3 都承诺「三家的原始 usage 对象整个进
	// messages.meta["usage_raw"]，将来改口径可回算」，而 Usage 那个结构体是**归一化
	// 之后**的结果 —— 三家的 InputTokens 由三条不同公式算出，一旦公式改了，
	// 光有归一化结果回算不出原值。没有这个字段，编排层手里就只有归一化结果，
	// 那条承诺无法兑现（2026-09-21 发现：这是契约与实现之间的一个洞，不是实现的疏忽）。
	//
	// usage 键缺席时为 nil，不要填 []byte("{}") 冒充 —— 那会让「没有 usage」
	// 和「usage 是空对象」在库里长成同一个东西。
	UsageRaw json.RawMessage
	Completion      Completion
	StopRaw         string // 原始判别串原样带回。失败形状零观测，丢掉它就只能重跑才能重新分类
	Usage           Usage

	// DeniedActions / DeniedActionsRaw 是 2026-09-21 第二片新增（11c 契约
	// 第 7 节：agy 的 denied_actions 从「已知缺口」改判成「阻塞」并修掉）。
	// 用法与 UsageRaw 同一个理由：DeniedAction{Action, DisplayName} 归一化
	// agy 实测到的形状，Raw 原样兜底。今天只有 agy 会填，claude/grok 恒为 nil。
	DeniedActions    []DeniedAction
	DeniedActionsRaw json.RawMessage
}
```

### provider.go

```go
package provider

import "errors"

// Provider 是一层 flag 适配器（02 第 1 节）：把一次 Council 调用翻译成这个 CLI 的
// headless 命令行，再把它的 JSON 输出翻译回 Council 的消息与用量。
//
// 它不认识 Canonical Conversation，不认识 SQLite，也不起进程 —— 起进程是
// turn.Runner 的事，这样每个适配器都能在不 spawn 任何东西的情况下被单测。
// 实现必须无状态、可被同 provider 的多个 agent 并发复用（04 第 0 节规则 2：
// 每个 agent 一条独立的流）：任何需要从 BuildTurn 带到 ParseTurn 的东西一律走
// Command / TurnResult 这两个值传递，绝不放进接收者。
type Provider interface {
	ID() string
	Capabilities() Capabilities

	// BuildTurn 把一次调用翻译成命令行。
	//
	// req.ProviderSession 是不是空串，是「这一轮要不要新建会话」的唯一信号，
	// 也是整个适配器层唯一一处「首轮与后续轮」的区别（09_STEP6_SPEC.md D 节）。
	// 除会话 flag 外的每一个 flag 每轮都原样再发一遍，值只由 TurnRequest 决定。
	// 不做「首轮发身份、后续不发」那种优化 —— 那恰恰是在改缓存前缀。
	//
	// 配置要求了这个 CLI 给不了的东西，返回包了对应哨兵的错误，绝不静默降级：
	//   权限档不在 Capabilities.Permissions 里      -> ErrUnsupportedTier
	//   req.WorkDir 为空                            -> ErrNoWorkDir
	//   Transport 是 Argv 且 prompt 超 MaxPromptBytes -> ErrPromptTooLong
	//   Capabilities 里该项仍是 Unknown 而这一轮需要它 -> ErrCapabilityUnknown（不猜）
	BuildTurn(req TurnRequest) (Command, error)

	// ParseTurn 只吃 stdout，是纯函数：同样的字节必须给出同样的 TurnResult。
	//
	// 它刻意不收 exitCode 与 stderr：收了适配器就会忍不住拿它们做兜底，而三家
	// 失败时的输出形状零观测（09_STEP6_SPEC.md A 节探针 1 之前），那种兜底只能是
	// 编造的。退出码由 Runner 判。
	//
	// 三种返回：
	//   已知的成功形状            -> (TurnResult, nil)，Completion 区分 Complete / Truncated / Empty
	//   provider 自报失败        -> 包了 ErrTurnFailed 的错误，StopRaw 与 Usage 仍尽量填上
	//                               （钱花了，账要记；但绝不把错误正文当成回答返回）
	//   连结构都认不出           -> 包了 ErrUnparseableOutput 的错误
	// 成功形状但会话 id 为空     -> 包了 ErrNoSessionID 的错误：静默把空串当 id 存进去，
	//                               下一轮就是一次冷启动，这种错误必须响亮。
	ParseTurn(stdout []byte) (TurnResult, error)
}

// Register 在各适配器文件的 init() 里登记自己。删掉一个适配器文件就等于把它下线，
// 不用改第二处。重复登记直接 panic：那是编译期就该发现的编码错误，不该留到运行时。
func Register(p Provider)

// Lookup 按 provider id 取适配器。
//
// 没有适配器是一种正常状态，不是配置错误：配置里写了 codex 而 Step 0 从没测过它，
// council 照常启动、横幅上点名，只是那个 agent 开不了口。找不到时返回包了
// ErrUnknownProvider 的错误，信息里列出已登记的 id。
func Lookup(id string) (Provider, error)

// IDs 按字典序返回已登记的 provider id。给启动横幅与报错列候选用，顺序必须稳定。
func IDs() []string

var (
	ErrUnknownProvider   = errors.New("provider not registered")
	ErrUnsupportedTier   = errors.New("permission tier not supported by this provider")
	ErrNoWorkDir         = errors.New("turn request carries no work dir")
	ErrPromptTooLong     = errors.New("prompt exceeds this provider transport limit")
	ErrTurnFailed        = errors.New("provider reported a failed turn")
	ErrUnparseableOutput = errors.New("provider output is not a known success shape")
	ErrNoSessionID       = errors.New("provider output carries no session id")
	ErrCapabilityUnknown = errors.New("provider capability is unknown, refusing to guess")
)
```

调用方一律用 `errors.Is` 判断，实现必须用 `%w` 包装这些哨兵，不要让调用方去匹配
错误字符串（与 08a 对 `internal/conversation` 哨兵的要求同一套规矩）。

## internal/turn

编排层。**导入 `internal/provider`、`internal/conversation`、`internal/config`；
不导入 `database/sql`，不导入 `internal/store`。**

### record.go

**本文件各代码块里的 `import` 清单是指示性的，不是逐字的。**
Go 对未使用的 import 是编译错误，而本契约按「一个文件一段」拆开展示，
某一段用不到的包会留在清单里。实现时按**这个文件真正用到的**写，不要原样照搬。
（独立核对指出下面这段原本多带了 `context` 与 `errors`，已删。）

```go
package turn

import (
	"time"

	"council/internal/conversation"
	"council/internal/provider"
)

// Outcome 区分一轮的结局。它们对「游标推不推」与「会话状态推不推」的处置不一样，
// 而这两件事的判据不一样 —— 这是 Step 6 相对天真实现最重要的一处区分：
// 天真实现只有「成功」和「失败」，没有第三种叫「provider 侧已经发生了，
// 但我们没记上」。
type Outcome int

const (
	OutcomeUnknown Outcome = iota
	// OutcomeSpoke: 正文已落进 Canonical Conversation。正常结局。
	OutcomeSpoke
	// OutcomePostRejected: provider 跑完了、钱花了、会话已经吃下这批 delta，
	// 但 Post 被拒（典型：这一轮在飞行中用户 /leave，事务内的 RequireActiveParticipant
	// 必然拒掉这条 Post）。不重试：重试每次都能拿到回答、每次都写不进去，只是重复烧钱。
	// 正文不许无声蒸发：整条打到屏幕上，并追加一条 KindSystem 面包屑。
	OutcomePostRejected
	// OutcomeProviderFailed: 进程跑完、JSON 认得出来，但 CLI 自报失败。
	// 用量与会话 id 仍然有效（钱花了），但正文绝不落库。
	// 09_STEP6_SPEC.md G 节：这条判据 [未验证]，等 A 节探针 1 补样本。
	OutcomeProviderFailed
	OutcomeUnparsable
	OutcomeTimeout
	OutcomeSpawnFailed
	// OutcomeRefused: 起进程之前就拒绝了 —— 权限档不支持、prompt 超长、
	// 身份指纹与本会话不符、没有 WorkDir、批次会改写已投递的历史。
	// 失败要响亮，而且要在花钱之前失败。
	OutcomeRefused
)

// Ingested 报告「有证据表明 provider 已经把这批 delta 吃进它的会话」。
// 这是游标推进的唯一判据，是对 04 第 6 节「成功返回才推进」的一处有意收窄。
func (o Outcome) Ingested() bool {
	return o == OutcomeSpoke || o == OutcomePostRejected
}

// Record 是「一轮」的完整记账。它就是 Step 7 那个四表一事务的入参：
// messages / cursors / turn_usage / provider_sessions 要写的每一个值都在这里，
// Step 7 不需要回头再问任何 provider。
type Record struct {
	ConvID  string
	AgentID string
	Outcome Outcome

	// ---- messages ----
	// System 是这一轮追加的 KindSystem 面包屑，没有则为 nil。三种事件，由
	// Meta["event"] 区分：session_opening（起进程之前发）、turn_failed（跑完但
	// 没产生发言）、post_rejected（provider 吃下了，本地记不上）。
	// 它的 sender 是 conversation.ReservedSenderSystem，三个脑子列留空。
	System *conversation.Message
	// Message 是 agent 这一轮说的话，已落库。Outcome 不是 OutcomeSpoke 时为 nil。
	Message *conversation.Message
	// DroppedText 只在 OutcomePostRejected 时非空：付过钱、写不进去的那段正文。
	DroppedText string

	// ---- cursors ----
	Delivered   []int64 // 真正渲染进 prompt 的那些 seq，升序去重
	ScannedTo   int64   // 本轮扫描到的最大 seq，spawn 之前定下
	PromptBytes int     // 送出去的正文字节数

	// ---- provider_sessions ----
	ProviderSession string
	LastTurnAt      time.Time
	Fresh           bool // 这一轮开了一条新的 provider 会话
	// Drifted: 送出去的 id 与回来的 id 不一样 —— CLI 悄悄开了新会话，前缀已经断了，
	// 钱已经花了。这是本设计里唯一能在事后抓到静默重建的钩子。
	Drifted bool

	// ---- turn_usage ----
	Usage provider.Usage
	// CostUSDDelta 是算好的单轮差值。nil 有三种读法，靠 Meta 区分，绝不猜一个数：
	// provider 不报金额；上一轮结局未知、基线可疑；本轮累计值 < 基线。
	// 永不写负数，也永不 max(0, delta)。
	CostUSDDelta    *float64
	BaselineSuspect bool // 上一轮的结局是未知的，本轮差值一律 nil 并记 Meta
	Cold            bool // Usage.CacheRead == 0，只在成功路径上赋值；非 Ingested 的轮上是没赋过的零值。读冷暖经 MeasuredCold（X-36ab）
	// ColdExpected 是 spawn 之前的预报：新会话，或距 LastTurnAt 已超 CacheTTL。
	// 它与 Fresh 刻意分开 —— TTL 过期只让下一轮贵，不使会话失效、不改会话 id、不重建。
	ColdExpected bool
	// UnexpectedCold = !ColdExpected && Cold。Usage.Unreported 含 "cache_read" 时
	// 它没有意义，别拿它告警。
	UnexpectedCold bool
	// MaybeAdvanced: 超时被杀。Windows 上被杀掉的往往只是 .cmd 垫片，真正干活的
	// node.exe 会活着把这一轮写进 CLI 自己的会话文件。本轮判 Ingested()==false，
	// 但这个标记要进 Meta 与屏幕。
	MaybeAdvanced bool

	WallMS int // Runner 自己计时，不取 JSON 里任何时长字段

	// Err 是 Outcome 非 OutcomeSpoke 时的原因，可被 errors.Is 认出哨兵。
	// 第 5-9 类结局在屏幕上要展示的退出码 / stderr 头 / 可执行文件路径等诊断信息，
	// 折进这个错误值的格式化消息里，Record 不为它们另开字段。
	Err error
}

// Meta 键。写进库的东西必须有一个共享的名字。
const (
	MetaEvent               = "event"                 // system 消息：session_opening | turn_failed | post_rejected
	MetaAboutAgent          = "about_agent"           // system 消息：这件事关于谁
	MetaEffortPolicy        = "effort_policy"         // per_turn_flag | process_external_config | ...
	MetaEffortSource        = "effort_source"         // 恒为 requested
	MetaEffortFlagOmitted   = "effort_flag_omitted"   // bool：发了也会被吞，所以没发
	MetaObservedModel       = "observed_model"        // claude-fable-5-1 / grok-4.6-build；agy 不写这个键
	MetaModelSource         = "model_source"          // requested | observed | ambiguous；ambiguous 只在 ModelKeys > 1 时
	MetaProviderSession     = "provider_session_id"   // Step 7 退休
	MetaCostCumulativeUSD   = "cost_cumulative_usd"   // claude 的累计原值。Step 7 退休
	MetaCostBaselineSuspect = "cost_baseline_suspect" // bool
	MetaUsageRaw            = "usage_raw"             // TurnResult.UsageRaw 的原始字节，原样；绝不是归一化后的 Usage
	MetaUsageUnreported     = "usage_unreported"      // []string，哪些 0 是「没报」
	MetaCompletion          = "completion"            // complete | truncated | empty | unknown
	MetaStopRaw             = "stop_raw"               // 原始 stop_reason / stopReason / status
	MetaDelivered           = "delivered_seqs"         // []int64
	MetaPromptBytes         = "prompt_bytes"
	MetaDropped             = "dropped"      // []DroppedOption
	MetaTextDropped         = "text_dropped" // bool，只在 post_rejected 的面包屑上
)
```

### session.go

```go
package turn

import (
	"time"

	"council/internal/provider"
)

// CacheTTL 来自 D-4 实测：claude 三轮的 cache_creation 全部落在
// ephemeral_1h_input_tokens，ephemeral_5m 恒为 0。
//
// 硬性规则（09_STEP6_SPEC.md G 节）：CacheTTL 绝不进入「是否重建会话」的判据，
// 只作为下一轮的价格预报。缓存过期不是会话过期。
const CacheTTL = time.Hour

// sessionKey 是 (会话, agent)。绝不以 provider 为键：两个 agent 即使同一家
// provider 也各有各的流（04 第 0 节规则 2）。
//
// 这是包内私有类型，不是导出契约，但它的字段形状决定了 Runner 状态表的正确性
// 论证是否成立，所以仍然写在这里供实现参考。
type sessionKey struct {
	convID  string
	agentID string
}

// sessionState 是一条 provider 会话在本进程内的全部状态。
//
// 这几个字段同生共死是有意的：会话 id 活在内存里，若把游标落了库，重启之后
// agent 会拿着「我已经读过 seq 1..N」的账本走进一条全空的新会话，而库里没有
// 任何一层看得出不对。Step 7 把它们放进同一个 WriteTx，那时它们才一起获得持久性
// （09_STEP6_SPEC.md F 节、L 节 #7）。同样是包内私有类型，理由同上。
type sessionState struct {
	providerSession string
	cursor          int64
	costTotal       float64 // claude 的会话累计金额基线
	haveCostTotal   bool
	baselineSuspect bool // 上一轮结局未知，基线可能已经落后
	lastTurnAt      time.Time
	identityFP      string // 建立这条会话时的身份指纹
	spoken          bool   // 这个 agent 在这条会话里已经说过话（决定面包屑的措辞）
}

// IdentityFingerprint 是「这条 provider 会话的前缀由什么决定」的摘要。
// 前缀不只是对话内容：身份文本、model、权限档（它决定 claude 的工具定义）都在
// 系统提示里，换任一项都让缓存作废，而换个 flag 继续 --resume 恰恰是最容易悄悄
// 干的事。
//
// effortFlagEmitted 是「实际放上命令行的那个值」，不是 AgentConfig.Effort。
// 这个词的选择独立地杜绝了一种情况：EffortProcessExternalConfig 的 provider
// 这一位恒为空串，用户怎么改配置都不触发漂移；而 EffortPerTurnFlag 的改了就该漂移。
//
// 绝不含 ObservedModel：provider 换个型号别名（opus -> claude-fable-5-1）就
// 触发一次重建，那是白烧钱。
func IdentityFingerprint(role, model, effortFlagEmitted string, tier provider.PermissionTier) string
```

### permission.go

```go
package turn

import (
	"council/internal/config"
	"council/internal/provider"
)

// TranslatePermission 把 config.Permission 翻译成 provider.PermissionTier。
//
// **2026-09-21 订正（11c_STEP8B_WORKSPACE_CONTRACT.md 第 3.3 节）**：本函数
// 原本只放行 read，workspace 与 full 都在起进程之前被拒绝，哨兵叫
// ErrTierNotInStep6。Step 8 第二片按 06 第 2 节那张实测过的写入边界表把
// workspace 解开了：read 与 workspace 都放行，只有 full（以及拼错的档）
// 仍然被拒——这是编排策略（06 第 2 节：FULL 档不给编排器自动使用），不是
// provider 能力问题，与 provider.validateRequest 里那道写入边界闸并存，
// 两道闸拦的不是同一件事。哨兵改名为 ErrTierNotOffered，旧名字里的
// 「Step 6」已经不准。
func TranslatePermission(p config.Permission) (provider.PermissionTier, error)
```

### runner.go

```go
package turn

import (
	"context"
	"errors"
	"sync"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// Spawner 起一个进程并把 stdout / stderr 分开收回来。
//
// 三条硬约束：
//  1. 不合并 stderr：合并会让 JSON 解析吃到诊断文本。
//  2. stdin 必须接 null device，绝不接 os.Stdin：repl.go 的 readLines goroutine
//     正阻塞在同一个 os.Stdin 上，抢起来胜负随时序漂移。
//  3. 超时后杀进程树：Windows 上 exec.CommandContext 杀掉的往往只是 .cmd 垫片，
//     真正干活的 node.exe 是它的子进程。实现用 taskkill /T /F /PID 兜底，
//     尽力而为，并把 killed 回给 Runner 去置 Record.MaybeAdvanced。
//
// 抽成字段是为了测试能塞一个回放探针 JSON 的假实现，不必真去调 CLI。
type Spawner func(ctx context.Context, c provider.Command) (stdout, stderr []byte, exitCode int, killed bool, err error)

// Runner 是 Step 6 的编排层：挑批次、渲染、翻译、起进程、计时、解析、算差值、
// 落库、推进会话状态。零值不可用，必须经 NewRunner 构造。
type Runner struct {
	cfg     *config.Config
	svc     *conversation.Service
	spawn   Spawner
	now     func() time.Time
	workDir string // Council 自己的干净目录，data_dir 下，绝不是 council 的启动目录
	timeout time.Duration

	// 一把锁管住全部状态，并且在整轮调用期间持有。于是 04 第 5 节
	// 「目标 agent 正在跑时不并发调用同一会话」自动成立。代价是不同 agent
	// 也被串行化，Step 9 要扇出时换成按 sessionKey 的锁（09 L 节 #17）。
	mu   sync.Mutex
	sess map[sessionKey]*sessionState
}

// NewRunner 用给定的配置、持久化入口与工作目录基路径构造 Runner。
// workDir 是所有 agent 共用的基目录（<data_dir>/workdir），Ask 内部为每个
// agent 计算 filepath.Join(workDir, agentID) 作为 TurnRequest.WorkDir。
func NewRunner(cfg *config.Config, svc *conversation.Service, workDir string) *Runner

// Ask 让 agentID 在 convID 里说一轮话，返回这一轮的完整记账。
//
// 即使返回 error，Record 也有意义：Outcome、Usage、ProviderSession、
// MaybeAdvanced 都已填好，调用方据此决定屏幕上说什么、要不要重发。
//
// 顺序固定，没有任何按 provider 或按「第几轮」的分支，完整八步见
// 09_STEP6_SPEC.md H 节。
func (r *Runner) Ask(ctx context.Context, convID, agentID string) (Record, error)

// ---------------------------------------------------------------------------
// 订正记录（2026-09-22，Step 9 片 1「拆锁」落地）
//
// 本节冻结的 Ask 三参数签名**一个字不改**，仍然是上面那一行。
// 片 1 新增的契约面在这里登记，不改动上面已冻结的条目：
//
//   1. 新增方法（Ask 成为它的薄封装，自己铸一个 roundID 再调它）：
//
//        func (r *Runner) AskUpTo(ctx context.Context, convID, agentID string,
//                                 upTo int64, roundID string) (Record, error)
//
//      upTo = 0 表示不设上界；upTo > 0 时只在**内存里**按 seq 截断本轮批次，
//      不改 conversation.Store.Messages 的签名。
//      roundID **必须由调用方铸好传入**，AskUpTo 自己不铸 —— 扇出时要有人在
//      开闸之前铸一个 id 发给全部 N 个成员，否则 N 个成员各铸各的，将来按轮
//      聚合会拿到 N 个不同的 id。Ask 自己铸一个，于是那一列永远有值。
//      roundID 只记账：绝不进 prompt、绝不进身份块、绝不进指纹。
//
//      upTo 保证的是**一轮内的确定性**：同一批 delta 不依赖谁先答完。
//      它**不保证**跨轮重发的逐字节相同 —— 一个 MaybeAdvanced 的成员在下一轮
//      重发时 upTo 变了，那一批里会多出这一轮另外 N-1 个人的发言。
//      后者在只追加的事实源上本来就做不到。
//
//   2. 新增字段 Record.RoundID string（本片还没有 turn_usage.round_id 列,
//      所以它只活在 Record 里，不写库；那一列是片 2 的事）。
//
//   3. 新增字段 SessionInfo.InFlight bool。闸拿不到（这条会话正有一轮在飞）时,
//      SessionInfo 不阻塞，改从库读**上一次提交**的状态并置 InFlight = true。
//      **「从没开过口」仍然返回 SessionInfo{}, false**，这条语义一个字不改
//      （11d 3.2 的「ok == false 算必冷」照旧成立）；InFlight 只在 ok == true
//      那一支有意义。
//
//   4. 新增哨兵 ErrTurnInFlight。Rebuild 用 TryLock，拿不到就返回包了它的错误
//      且什么都不做 —— 尤其不许先 DropProviderSession 再发现拿不到闸。
//
//   5. 下面 Runner 结构体里列的 `mu sync.Mutex` + `sess map[sessionKey]*sessionState`
//      **已被替换**为 `gatesMu sync.Mutex` + `gates map[sessionKey]*sessionGate`
//      与 `rulesMu sync.RWMutex`。它们是包内私有字段、不是导出契约，
//      登记在这里只是因为上面那段代码块照抄了它们。
//
//   6. （2026-09-27，Step 10 A 之五，`8c00ec9`）新增字段 SessionInfo.BaselineSuspect bool：
//      这条 provider 会话的成本基线可疑（上次崩溃或结局未知，下一轮 /ask 的金额是「不知道」）。
//      内存分支读 sessionState，另两个分支读库。/session 据此印 BASELINE 列（suspect / ok）。
//      Ask 在起进程之前拒绝「有别人的在飞记录」的成员，哨兵是 conversation.ErrInflightConflict
//      （见 13a_S10_A5_CONTRACT.md）。
// ---------------------------------------------------------------------------

// SessionInfo 返回 (convID, agentID) 这条 provider 会话在本进程内存里的当前
// 状态，供 /session 命令使用。ok 为 false 表示这个 agent 在这条会话里还没开过口
// —— 下一轮 Ask 会是 Fresh。这是一个纯读方法，除了拿锁读一眼内部状态表之外
// 不做任何状态变更，不触发任何 provider 调用。
func (r *Runner) SessionInfo(convID, agentID string) (SessionInfo, bool)

// SessionInfo 是 /session 命令需要的只读快照。
type SessionInfo struct {
	ProviderSession string
	LastTurnAt      time.Time
	// ColdExpected 是查询这一刻的即时判断：距 LastTurnAt 是否已超过 CacheTTL。
	// 与 Record.ColdExpected 同一个判据，但这里是随时可查，不依赖真的起一轮调用。
	ColdExpected bool
}

var (
	ErrNoDelta       = errors.New("nothing new to send to this agent")
	ErrAgentUnknown  = errors.New("agent is not in the config")
	ErrIdentityDrift = errors.New("identity or brain changed since this session was opened")
	ErrPrefixRewrite = errors.New("batch would rewrite history already delivered to this agent")
	// ErrTierNotOffered **2026-09-21 改名自 ErrTierNotInStep6**
	// （11c_STEP8B_WORKSPACE_CONTRACT.md 第 3.3 节）：旧名字里的「Step 6」
	// 已经不准，workspace 在 Step 8 第二片放行了，这个哨兵现在只挡 full
	// 与拼错的档。
	ErrTierNotOffered = errors.New("this permission tier is not offered to the orchestrator")
)
```

**这里曾经还有一个 `ErrNoAdapter`（「这个 agent 的 provider 没有注册适配器」），已删。**
那是同一个状况的第二个哨兵：`provider.Lookup(id)` 本来就为此返回
`provider.ErrUnknownProvider`，registry 是「有没有适配器」这件事的唯一权威。
`turn` 只需把它 `%w` 包上去，调用方 `errors.Is(err, provider.ErrUnknownProvider)` 即可。
造第二个哨兵会让调用方必须同时认两个，而 `09` 规格里写的也是前者
（独立核对时发现这是 `09` 与 `09a` 之间唯一一处硬冲突，按 `09` 裁定）。

**`ErrAgentUnknown` 与它不是一回事**，保留：前者是「配置里根本没有这个 agent id」，
后者是「agent 在配置里，但它写的 provider 没有适配器」。两种状况给用户的下一步不同
（改 `/agents` 看有哪些 vs 换一个 provider 或等 Step 7 补适配器）。

`ErrTierNotOffered`（改名自 `ErrTierNotInStep6`，见上）也是 `permission.go` 里
`TranslatePermission` 返回的那个错误，两处引用同一个变量，不要在 `permission.go`
里另声明一份同名不同值的哨兵。

### render.go

```go
package turn

import (
	"council/internal/config"
	"council/internal/conversation"
)

// RenderDelta 把一批 canonical 消息渲染成这一轮的正文。
//
// 纯函数，这是 Step 6 守 04 第 0 节规则 1 的结构保证：函数体里不得含
// time.Now、不得含游标数字、不得含重试次数、不得含本轮序号、不得依赖 map
// 迭代顺序。messages 表只追加、行永不改写，所以同一个 seq 区间第二次渲染
// 必然得到同一串字节 —— 这是结构性的，不是靠自觉。用 golden 文件锁住。
//
// 过滤规则集中在这一个函数里：跳过 KindSystem（04 第 2 节把内部事件与
// provider 诊断列进「不应当路由的」），skipSelf 为真时跳过这个 agent 自己
// 说过的话。
//
// skipSelf 不是第二个判定：它由 (ProviderSession != "") 导出，与「是不是新
// 会话」同一个谓词。一条全新的 provider 会话里什么都没有，包括这个 agent
// 自己过去的发言，所以新会话那一轮必须重放自己；resume 那一轮则必须跳过。
func RenderDelta(ms []conversation.Message, to config.AgentConfig, selfID string, skipSelf bool) string
```

## internal/conversation 的改动

只加符号与校验，不改任何已冻结的签名，不碰 `internal/store`，不改 schema。

```go
package conversation

// ReservedSenderSystem 是 Council 自己记账时用的 sender id，裁定未决 #14 的
// 后一半。
//
// 它与 ReservedSenderUser 同类：不是 participant，不需要 join，不出现在成员
// 列表里，三个脑子列必须为空，Post 对它跳过 RequireActiveParticipant。
//
// 为什么不拿「那个 agent 的 id」当 sender：一条「你的会话开了」不是那个 agent
// 说的话，是 Council 替它记的账；挂在它名下，/log 里会显示成它说的，Step 7
// 的 Router 还会把它当成那个成员的观点转给别人。更要命的是：非保留 id 要过
// 事务内的 RequireActiveParticipant，而这类事件恰恰常常发生在刚 /leave 或
// 正被重配的成员身上，那样 08a 明令必须留下的那条记录一条都留不下。
//
// 实测已核：internal/store/schema.go 的 CHECK 是
// sender_id <> 'user' OR (sender_provider='' AND sender_model='' AND sender_effort=''),
// 只约束 'user'，sender_id 也没有外键，kind 的 CHECK 含 'system'。所以
// sender_id='system' 带三个空脑子列完全合法，本步不需要改 schema。配套把
// "system" 也加进 internal/config/validate.go 的保留 agent id（现在只保留了
// "user"），否则配置里一个叫 system 的 agent 会和它撞。
const ReservedSenderSystem = "system"

// ErrMissingSenderBrain 是 08a「给 Step 6 的硬性要求（MUST）」的落点：
// KindAgent 消息必须带 SenderProvider 与 SenderModel（SenderEffort 仍可空 ——
// 有的 provider 没有这个概念）。
//
// 它要挡的是 08a 点名的那个洞：Step 5 两层校验拦的都是 sender_id，反方向一条
// 都没有，适配器漏填三列照样落库，而渲染侧把三列全空渲染成「不适用」的占位符
// —— 漏填于是落在一个代码自己认定为合法的显示上，不报错、测试全绿，而
// Canonical Conversation 只追加，这批消息的归属此后无法回填。
var ErrMissingSenderBrain = errors.New("agent message carries no provider or model")

// Post 的事务外校验在 Step 5 的基础上加两条，其余一字不动：
//
//  1. SenderID 是 ReservedSenderSystem 时，三个脑子字段必须全空（与 user 那条
//     同规矩，违反时返回包了 ErrInvalidSender 的错误）；事务内也跳过
//     RequireActiveParticipant。
//  2. Kind 是 KindAgent 时，SenderProvider 与 SenderModel 必须非空，否则返回
//     包了 ErrMissingSenderBrain 的错误。
//
// 第二条是按 Kind 立的第一条规矩，与现有那条按 SenderID 的并存（未决 #14 的
// 前一半）。签名不变。
func (s *Service) Post(ctx context.Context, m Message) (Message, error)
```

## internal/config 的改动

没有导出符号的形状变化：`Config` / `AgentConfig` / `Permission` / `Mode` 等
类型的字段不动。变化是纯行为性的：

- `internal/config/validate.go`：`"system"` 与 `"user"` 一同列为保留 agent id，
  additive，旧配置照常加载。
- `internal/config/load.go`：加载时若某 agent 的 provider 的 `EffortPolicy` 是
  `EffortProcessExternalConfig` 而 `Effort` 非空，往 `Err` 追加一行警告文本
  （具体措辞不在契约面）。不是错误，不拦启动。

## 对 08a 冻结签名的修订请求

按 08a 自己写下的规矩「实现者不得改动本文件里的签名；觉得签名有问题，退回来说，
不要自行改」，下面这条改动**不是**自行更改，而是一条明文的、可被单独否决的修订
请求。

`internal/repl` 目前的冻结签名：

```go
func New(cfg *config.Config, svc *conversation.Service, in io.Reader, out io.Writer) *REPL
```

**请求改为：**

```go
func New(cfg *config.Config, svc *conversation.Service, rn *turn.Runner, in io.Reader, out io.Writer) *REPL
```

理由：`/ask` 与 `/session` 两条新命令需要经 `rn` 调用编排层，REPL 没有别的路径
拿到 `*turn.Runner`。`rn` 为 nil 时两条新命令一律回一句「provider 调用未就绪」，
**不得 panic**，与现有 `svc == nil` 的兜底同风格。

### internal/repl 的其余改动（不涉及冻结签名，实现自由决定字面量）

`REPL` 结构体加一个字段，`command` 结构体加一个字段。**两个都要加** ——
只改 `New` 的签名而不给 `REPL` 加字段，命令函数就拿不到 Runner：
回调签名是 `func(ctx context.Context, r *REPL, args cmdArgs) error`，
`r` 是唯一的通路。（独立核对时发现契约漏了前者，补上。）

```go
type REPL struct {
	cfg  *config.Config
	svc  *conversation.Service
	rn   *turn.Runner // 本次新增；nil 表示 provider 调用未就绪
	in   io.Reader
	out  io.Writer
	cmds map[string]command

	cur *conversation.Conversation

	inDraft       bool
	draft         []string
	draftPoisoned bool
}

type command struct {
	name        string
	summary     string
	needsSvc    bool
	needsConv   bool
	needsRunner bool // 需要 *turn.Runner；本次新增
	run         func(ctx context.Context, r *REPL, args cmdArgs) error
}
```

`runCommand` 的前置条件检查顺序：`needsSvc` 与 `needsConv` 沿用 Step 5 的顺序
（先答持久化未就绪，再答没有当前会话），`needsRunner` 排在两者之后 —— 持久化
没起来或没选会话时，「provider 调用未就绪」这句话还轮不到说。

新命令表项（措辞不在契约面，行为在）：

- `/ask <agent-id> [文本]`：`needsSvc`、`needsConv`、`needsRunner` 全真。
  带文本时先经与 `/say`、裸文本共用的同一个 `postUserText` 函数落一条 `user`
  消息，再调 `rn.Ask(ctx, r.cur.ID, agentID)`；不带文本时直接调 `Ask`。
- `/session`：`needsSvc`、`needsConv`、`needsRunner` 全真。遍历
  `r.svc.Participants(ctx, r.cur.ID, false)`，对每个活跃成员调
  `rn.SessionInfo(r.cur.ID, agentID)` 并打印。

### internal/app 的改动（行为性，签名不冻结）

`app.Run` 在建好 `svc` 之后、进 REPL 之前：为每个 `cfg.Agents` 建
`<data_dir>/workdir/<agent-id>/`（`os.MkdirAll`），再用
`filepath.Join(dataDir, "workdir")` 作为 `workDir` 构造
`turn.NewRunner(cfg, svc, workDir)`，把返回值传进
`repl.New(cfg, svc, rn, in, out)`。目录建不出来时按现有「致命错误、写 `Err`、
不进 REPL」的风格处理，与 `store.Open` 失败同一套。`app.Options` 本身不需要
新字段——`turn.Runner` 和 `svc` 一样是 `Run` 内部拿已解析出的 `cfg` / `dataDir`
现造的，不经外部注入。

## 不属于契约面的东西

实现可以自由决定，**测试不得对它们做字面断言**：

- 命令输出的具体措辞、表格列名列序、空字段占位符
- `sessionKey` / `sessionState` 的字段命名（形状已在 `session.go` 一节给出
  供参考，但它们不是导出符号，跨包代码不应该、也不能引用到它们）
- `Capabilities.UsageFormula` 的具体字符串措辞（内容要点——用了哪些字段相加
  ——在契约内；用词与格式不在）
- `Spawner` 的生产实现细节（`taskkill` 的具体参数、输出封顶的字节数）
- 错误信息的具体中英文措辞（但必须能被 `errors.Is` 认出哨兵）
- `/ask`、`/session` 屏幕输出的具体排版

## 符号禁令（写代码时必须遵守）

代码文件里（含注释、字符串字面量、打印输出、标识符）禁用装饰性符号：
Emoji U+1F300-U+1FAFF、Dingbats U+2700-U+27BF、杂项符号 U+2600-U+26FF、
杂项符号与箭头 U+2B00-U+2BFF、带圈字母数字 U+2460-U+24FF、U+FE0F、U+200D。

放行：希腊字母、数学算符 U+2200-U+22FF、度数正负乘除微符号、上下标、
普通箭头 U+2190-U+21FF、中文、ASCII 制表符。

原因：本机控制台默认 GBK，打印 emoji 会崩或乱码，落进日志后污染 grep 与 diff。
