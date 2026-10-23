package provider

import "encoding/json"

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

	// SystemPrompt 是 AgentConfig.Prompt，也就是 Claude Code agent 文件
	// frontmatter 之后的那段正文（外部系统提示）。它与 Prompt 是两回事：
	// Prompt 是这一轮的对话增量，每轮不同；SystemPrompt 属于成员身份，
	// 每轮相同，改了它就该作废旧 provider 会话。
	//
	// 今天只有 claude 消费它；其它适配器必须原样忽略（它们的 argv 不许
	// 因为多出这个字段而变化）。claude 的载体见 claude.go：不走 argv，
	// 因为这段正文天然多行，而 claude 在本机是 npm 的 .cmd 垫片，cmd.exe
	// 会把 argv 里的换行当成命令分隔符（未决 #16）。
	SystemPrompt string

	// WorkDir 是子进程的 cwd，必须非空。空值时 BuildTurn 返回 ErrNoWorkDir：
	// 继承 council 的启动目录会让三家 CLI 读到 cwd 下的项目级指令文件，
	// 入场费与行为随启动目录漂移、不可复现，而且拿到写盘权限的成员会把文件写进
	// 那个仓库（直接违反 06 第 4 节）。
	WorkDir string

	// RoleDir 是 council-role.txt 的落点目录。空串表示落在 WorkDir 里，
	// 这是非副本模式的既有形状；固定工作目录（-workspace 副本）模式下
	// 它被设成 <data_dir>\workdir\<agentID>，把角色文件挡在副本外面。
	// 只有需要落文件的适配器（今天只有 claude）读它。
	RoleDir string

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

	// Files 是起进程之前必须先落到磁盘上的输入，键是文件路径，值是内容。
	// 存在的唯一理由见 claude.go 的注释：Windows 上 .cmd / .bat 垫片的命令行
	// 由 cmd.exe 再解析一遍，argv 元素里的换行会被当成命令分隔符。
	// 键必须是绝对路径（适配器用 filepath.Join(req.WorkDir, ...) 拼）。
	// nil 表示这一轮不需要落任何文件。
	Files map[string][]byte

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
// 一处定义。只有成功轮的原始 usage 对象整个存进 messages.meta["usage_raw"]
// （失败轮也可能带回 UsageRaw，但 runner.go 的 absorbParse 不吸收它），将来改
// 口径可回算。跨 provider 比较之前，先看 Meta["usage_raw"] 与各自的
// UsageFormula——这一列的数字来自三条不同的算法，不是同一个数量级的噪声。
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
	// 它整体进 messages.meta["usage_unreported"]；turn_usage 的用量列可空，列在这里的
	// 那一格写 NULL（turn.makeTurnUsage），/ask 的用量行印 ?（turn.Record.ReportedUsage）。
	// （Step 6 时 cache_create 列还是 NOT NULL DEFAULT 0，这张表曾是唯一能说实话的地方；早已不是。）
	Unreported []string
}

// DeniedAction 是「一次工具调用在 headless 模式下被自动拒绝」这件事的归一化形状。
//
// 由来见 agy.go 的 ParseTurn 与 E:\<tmp-root>\council\oq-agy-ws\RESULTS.md：
// agy 在 WORKSPACE 档越界写盘时，denied_actions 非空但 status 仍是 SUCCESS、
// response 被整段清空——这与「这一轮真的没话说」在字节上完全一样（未决 #17
// 记的同形陷阱）。不解析这个字段，Council 只能报「本轮为空」，而实际发生的是
// 「越界被拒、界内可能已经写了一半」；界内的事已经做完了，只是模型的汇报没了。
//
// 这不是 agy 专有的概念：claude 的 permission_denials 字段大概率是同一件事
// （claude.go claudePermissionArgs 的注释与本次任务书都说 2026-09-21 实测过
// 它非空），但本机迄今抓到、逐字节归档进 fixtures_test.go 的每一份 claude
// 原件（claudeT1/T2/T3JSON、claudeBadModelJSON）里这个键都是空数组 []——
// 那次「非空」的实测没有留下可归档的原始字节，元素形状没有第一手证据，本片
// 不敢照抄 agy 的两字段结构去猜 claude 的形状——猜错一个没见过的形状，后果
// 和「根本不解析」一样，都是把假数据当成真相记进只追加的事实源。所以这里
// 只归一化 agy 一家；claude 那边留着 TurnResult 的 DeniedActionsRaw 兜底
// （原始字节不丢），DeniedActions 这个结构化字段留空。grok 没有任何对应
// 字段。claude 侧的解析是后续独立一片的事，本片不做。
type DeniedAction struct {
	Action      string // agy: denied_actions[i].action，如 "write_file"
	DisplayName string // agy: denied_actions[i].display_name，如 "WriteToFile"
}

// TurnResult 是 ParseTurn 从一段 stdout 单独能诚实知道的全部。
//
// 三家的取值表、顶层键的逐字清单、以及三条绝对禁令（会话 id 只许取指定的键、
// token 只许取 usage 不许取 modelUsage、num_turns 不可用于任何判定），
// 逐字取自 docs/05_MESSAGE_PROTOCOL.md 第 3 节与 E:\<tmp-root>\council\step0-probe
// 的七份原件，以那两处为准，本文件不重复抄一遍会走失同步的表。
type TurnResult struct {
	Text            string
	Thought         string
	ProviderSession string // 三家必须都填；grok 填的是它自己铸的、被 CLI 原样回显的那个。空则 ErrNoSessionID
	ObservedModel   string // modelUsage 恰好一个键时取那个键；零个或多于一个时留空（见 ModelKeys）
	// ModelKeys 是 modelUsage 的键数。**只有多于一个键才是 ambiguous**：
	// 0 个键不是「有歧义」，是「这家根本不报实测型号」，那一档走 requested。
	// agy 没有 modelUsage、claude 失败时 modelUsage 是 {}，把它们都打成
	// ambiguous 会让每一条 agy 发言都带着一个假的歧义标记。
	// （本行原写「!= 1 时记 ambiguous」，与 09_STEP6_SPEC.md 冲突，
	// 以 09 为准，2026-09-21 随 09a 一起订正。）
	ModelKeys  int
	Completion Completion
	StopRaw    string // 原始判别串原样带回。失败形状零观测，丢掉它就只能重跑才能重新分类
	Usage      Usage

	// UsageRaw 是这家 provider 的 usage 子对象原始字节，原样不动。
	//
	// 它不是 Usage 的冗余备份：Usage 是归一化之后的结果，而三家的 InputTokens
	// 由三条不同公式算出（见 Capabilities.UsageFormula）。公式一旦要改，
	// 光有归一化结果回算不出原值。09 F 节与 05 第 3 节都承诺了「原始 usage
	// 对象整个进 messages.meta[usage_raw]，将来改口径可回算」，没有这个字段
	// 那条承诺就是空头的 —— 编排层手里只有归一化结果，没别的可写。今天兑现
	// 的只有成功轮：失败轮也可能带回 UsageRaw，但 runner.go 的 absorbParse 不
	// 吸收它，只有成功轮经 agentMeta 进 meta.usage_raw。
	//
	// usage 这个键缺席或为 null 时是 nil，不要填空对象冒充：那会让
	// 「没有 usage」与「usage 是空对象」在库里长成同一个东西。
	UsageRaw json.RawMessage

	// DeniedActions 是这一轮里被 headless 模式自动拒绝的工具调用，归一化形状
	// 见 DeniedAction 的注释。空切片（nil）与「这家没有这个概念」不做区分——
	// 三家里目前只有 agy 会填它，claude/grok 恒为 nil，调用方不能靠它反推
	// 「这家到底有没有这个能力」，那是 Capabilities 该声明的事，本片没有
	// 新开一个能力位（见 agy.go WriteBoundary 注释里的取舍）。
	DeniedActions []DeniedAction

	// DeniedActionsRaw 是 denied_actions 这个顶层键的原始字节，原样不动，
	// 用法与 UsageRaw 同一个理由：DeniedAction 只归一化了 agy 实测到的两个
	// 键，将来这个字段里多出别的键（或者 claude 的 permission_denials 补上
	// 实测形状之后要复用这套归一化），原始字节留得住，回算得回来。
	// 键缺席或为 null 时是 nil。
	DeniedActionsRaw json.RawMessage
}
