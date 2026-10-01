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
	SessionIDUnknown             SessionIDPolicy = iota
	SessionIDPreassignable                       // grok --session-id <uuid>：spawn 前可定，须是合法 UUID 且不得已存在
	SessionIDReturnedOnFirstTurn                 // claude --resume / agy --conversation：只能从输出里取回
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
func (p EffortPolicy) String() string {
	switch p {
	case EffortPerTurnFlag:
		return "per_turn_flag"
	case EffortProcessExternalConfig:
		return "process_external_config"
	case EffortBakedIntoModelLabel:
		return "baked_into_model_label"
	case EffortUnsupported:
		return "unsupported"
	default:
		return "unknown"
	}
}

type IdentityPolicy int

const (
	IdentityUnknown              IdentityPolicy = iota
	IdentitySystemPromptAppend                  // claude --append-system-prompt：追加，盖不住用户全局规则文件
	IdentitySystemPromptOverride                // grok --system-prompt-override：替换
	IdentityPromptPreamble                      // 没有可用的文本 flag，只能把身份写进首轮正文（Step 6 的 agy）
	IdentityAgentPreset                         // agy --agent 收的是预设名，不是任意文本
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
	// 这条注释原本写着「三家全部 SupportUnknown：本机没实测过」，那是隔离
	// 实测做完之前的状态，现在已经过期：三家分别给出了实测结论——
	// claude 经 --safe-mode 做到了（SupportYes），grok 试过重定向配置目录、
	// 换系统提示两条路都没能隔离掉（SupportNo），agy 仍未测（SupportUnknown）。
	// 具体依据见各适配器 Capabilities() 里的注释，TestRegistry_CapabilitiesDeclareEveryField
	// 按这三个取值逐一钉住。
	CleanInstructionEnv Support

	// CacheAcrossTurns 说的是「同一条会话续到第二轮时，前一轮送过的前缀
	// 还算不算钱」。这一位是三家各自在真机上实测出来的，不是从别处推的，
	// 也不是照 02 第 1.1 节的能力表抄的：
	//
	//   - claude = SupportYes：D-4 的 Step 0 探针，三轮 resume，
	//     T1 冷启动 cache_create=47749 cache_read=0；
	//     T3 resume  cache_create=95    cache_read=47752——
	//     前缀进了缓存，第三轮几乎是白送的。
	//   - grok = SupportYes：同一条会话的直接证据是 Step 0 探针本身，
	//     T1 冷启动 input=20625 cache_read=0，T2 resume input=250 cache_read=20608；
	//     00_DECISIONS.md 未决 #1 另有跨会话印证（两条互不相干的会话，
	//     身份块与首条正文逐字节相同时，第二次 cache_read=19584 而第一次是 0，
	//     证明缓存按内容命中、不按会话续不续）；Step 8 真机冒烟
	//     （E:\Isolating_test\council\step8a-smoke\RESULTS.md）同一轮次里
	//     也看到 cache_read=128、cache_create=0，三处互相印证。
	//   - agy = SupportNo：00_DECISIONS.md 未决 #17 的真机观察，同一条会话
	//     续到第二轮，input_tokens 从 16195 涨到 32722 而 cache_read 仍是 0；
	//     Step 8 真机冒烟同一份 RESULTS.md 里同一轮次同样是 cache_read=0
	//     （in=16316）。它没有跨轮缓存收益，规则字节数在它身上是按轮数乘着付的。
	//
	// 为什么这一位要单独声明，不能靠现有字段拼出来：它直接决定一份规则
	// 文件（或任何稳定前缀）的成本模型是「首轮付一次，后面几乎白送」还是
	// 「每一轮都要重付一整份」——这两种模型相差可以是几十倍的 token 账单，
	// 而 UsageFormula 只管一轮内部怎么加总，不管跨轮是否命中缓存。
	// 11a 契约 5.1 曾经因为没有这一位而在 repl 里印一句写死的固定文本，
	// 那是本项目一直在防的「没标定就不新造能力位」这条纪律下的权宜之计；
	// 现在三家都有了实测支撑，继续印同一句话反而是把已知的区分抹平。
	CacheAcrossTurns Support

	// WriteBoundary 说的是「这家能不能证明自己把文件写限制在指定目录内」。
	//
	// 这一位不是「有没有权限 flag」，是「那道 flag 实测围不围得住」 ——
	// 06 第 2 节那张表被推翻过一次就是因为混淆了这两件事：flag 写上了，
	// 有效权限却是「flag 与用户配置文件的合集」，READ 档照样建得出文件。
	//
	// 零值是 SupportUnknown，所以一个新适配器若在 Permissions 里列了 workspace
	// 却没声明这一位，会被 validateRequest 拒掉 —— 漏填的后果是「未实测」
	// 而不是「支持」。这是有意的（11c 契约第 2 节）。
	WriteBoundary Support

	// ModelDiscovery 说的是「这家 CLI 能不能、以什么方式列出它能用的模型」
	// （02 第 1.2 节设计过，#45 补上）。零值 ModelDiscoveryUnknown，
	// 与其它能力位同一条纪律：漏填是未实测，不是支持。
	// 取值定义与三家的依据见 discovery.go。
	ModelDiscovery ModelDiscovery
}
