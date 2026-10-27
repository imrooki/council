// Package config 实现 Council 的配置加载、缺省填充与校验。
package config

import (
	"sort"
	"strings"
)

// DefaultDataDir 是 D-9 定的默认数据目录。这是一个机器相关的值，
// 本项目当前只在一台机器上跑；将来跨机器时改这里，不要在调用处散落判断。
const DefaultDataDir = `E:\council-data`

const (
	// EnvConfigPath 是覆盖配置文件路径的环境变量名。
	EnvConfigPath = "COUNCIL_CONFIG"
	// EnvDataDir 是覆盖数据目录的环境变量名。
	EnvDataDir = "COUNCIL_DATA_DIR"
)

// 界面语言的合法取值。与 internal/i18n 的语言常量同值，但本包不 import
// i18n：配置解析层只认识字符串，语言解析属于界面层。
const (
	LanguageAuto = "auto"
	LanguageZh   = "zh"
	LanguageEn   = "en"
)

// local_agent 的合法取值。三档逐级授权：off（默认）→ observe → coordinate。
//
// 为什么不在这里预先写出 register / collaborate：那两档（L2 的登记、L4 的
// 模型协作）在 v1 没有实现，写进合法枚举等于让用户配置一个「看起来开着、
// 实际什么都不做」的层级。校验拒绝它们，比静默降级成 off 诚实。coordinate
// 是 L2 实现了租约之后才加进来的：它今天真的能做的只有「申请/交还资源租约」，
// 不会再是空壳。
//
// 空串与 off 同义：Load 会把空串归一成 off（applyDefaults），但直接构造的
// Config（测试、旧调用方）可能没有这个字段；读取方对任何未实现的取值都按
// off 处理（fail closed），不会因为读到空串就把扫描或协调打开。
const (
	LocalAgentOff = "off"
	// LocalAgentObserve 是只读观察：council who / council_who 会枚举进程。
	LocalAgentObserve = "observe"
	// LocalAgentCoordinate 在观察之外允许 council claim / release 申请租约。
	// 它包含观察（协调的资源事实来自同一份进程视图），但 observe **不**包含
	// 它 —— 开观察不会顺手给出协调权。
	LocalAgentCoordinate = "coordinate"
)

// Permission 是 agent 的权限档。
type Permission string

const (
	PermissionRead      Permission = "read"
	PermissionWorkspace Permission = "workspace"
	PermissionFull      Permission = "full"
)

// Mode 是 agent 的工作模式。
type Mode string

const (
	ModeDiscussion          Mode = "discussion"
	ModeWorker              Mode = "worker"
	ModeDiscussionAndWorker Mode = "discussion_and_worker"
	ModeJudge               Mode = "judge"
	ModeManager             Mode = "manager"
)

// Config 是 Council 的根配置。
type Config struct {
	DataDir string `toml:"data_dir"`
	// RulesFile 是局部规则文件的路径。空串表示没有局部规则，这是正常情况。
	// 相对路径相对配置文件所在目录解析，不是相对当前工作目录 ——
	// 按 cwd 发现会让入场费与行为随启动目录漂移、不可复现。
	RulesFile string `toml:"rules_file"`
	// StoreThought 控制要不要把 provider 的 thought/思维链摘要存进
	// messages.thought 列。默认 true（未决 #9(a)）。
	//
	// 这是一个裸 bool，且默认值是 true —— 危险的那一种：toml.Decode 只在
	// 用户写了这个键时才覆盖字段，没写就保留 Decode 之前已经在结构体里的值。
	// 若 Load 不在 Decode 之前把它预置成 true，用户省略这个键时会静默读出
	// false，正好是默认值的反面，而且没有任何报错（load.go 里有实现细节）。
	// Default() 同理要显式写 true，不能指望这个字段的自然零值。
	//
	// 关掉之后已经落库的 thought 不回溯（不洗历史，事实源只追加）；这一轮起
	// 新写的行，Thought 列照写空串，那条消息的 Meta 会记
	// conversation.MetaThoughtDropped = true——只在真的丢掉了非空 thought 时
	// 才写，空串不许同时表示「这家 provider 没有 thought 这个概念」与
	// 「我们按用户要求丢了」（未决 #11 同一个裁法）。
	//
	// validate.go 不校验它，与 RulesFile 同待遇：这是一个开关，不是一个
	// 需要跨引用或格式校验的值。
	StoreThought bool `toml:"store_thought"`
	// Language 是界面语言：zh / en / auto。默认 auto（跟随系统区域）。
	// 它不是 agent 字段、不进身份指纹，只影响界面文案；Web 设置页可以在
	// 运行期改它并立刻生效（见 internal/serve/settings.go 的语言端点）。
	// 空串按 auto 处理：直接构造的 Config（测试 / 旧调用方）没有这个字段。
	Language string `toml:"language"`
	// LocalAgent 是本机 agent 模块（观察 + 资源租约）的总开关，
	// 取值见 LocalAgentOff / LocalAgentObserve / LocalAgentCoordinate，默认 off。
	//
	// 为什么是顶层键而不是 [local_agent] 表：这一项只有一个枚举值，复用的
	// 是 language / store_thought 那条「高优先级层写了才覆盖」的合并规则；
	// 而本仓库的配置写盘实现只认识 [agents.x] 表与顶层的 language
	// （internal/config/langwrite.go），为一张单键表再造一套 TOML 表插入
	// 逻辑不值得。
	//
	// 为什么默认必须是 off：S-01 要求关着的时候零扫描、零后台、零行为变化。
	// 这个键缺席时 council who / MCP 的 council_who 一次进程都不枚举，
	// council claim 直接拒绝，只打印一句「未开启」。它不触发任何后台循环、
	// 端口或 IPC。
	//
	// 它与 language 一样不进身份指纹：改它不影响任何成员的 provider 会话。
	LocalAgent string                    `toml:"local_agent"`
	Providers  map[string]ProviderConfig `toml:"providers"`
	Agents     map[string]AgentConfig    `toml:"agents"`
	Teams      map[string]TeamConfig     `toml:"teams"`
	// Summary 是 [summary] 表，#45 第二节。
	// 要写成表而不是顶层键：TOML 的顶层键必须写在所有表之前，
	// 维护者 要能把这张表追加到文件末尾。
	// 零值（没有这张表、空表、agent = ""）= 没配。
	// Validate 的两条规则见 validate.go；默认 role 见 DefaultSummaryRole。
	Summary SummaryConfig `toml:"summary"`
}

// SummaryConfig 是 [summary] 表。只有一个键 agent。
// 唯一的开关就是有没有配 agent：多成员 /round 之后由这个 agent
// 出一段「总的来看」，只给用户看。
type SummaryConfig struct {
	Agent string `toml:"agent"`
}

// DefaultSummaryRole 是总结成员的 role 为空时由 Load 填进去的那一句
// （不要求 维护者 手写长 role）。它进身份块、进指纹：改这句话会让每个
// 总结成员各重建一次会话（冷启动一次），其余成员不受影响。
// 只填空值，用户自己写了 role 就用用户的。
const DefaultSummaryRole = "你是这场讨论的总结人，不参与讨论。每次被叫到时，读用户的问题和各成员的发言，做一致性判断：他们在哪些点上一致，在哪些点上有分歧、各自的依据是什么，有没有人答非所问或明显出错。你的话只给用户看，其余成员看不到；不要替任何一方继续论证，不要提出新的方案。用中文回答：先用一句话给出总的判断，再分条列出一致之处与分歧之处，写得简短。"

// ProviderConfig 描述一个外部 provider 的启动方式。
type ProviderConfig struct {
	Command string `toml:"command"`
}

// MaxAgentPromptBytes 是 AgentConfig.Prompt 的硬上限（8 KiB）。
//
// 为什么要有上限：这段正文来自 Claude Code agent 文件的 frontmatter 之后，
// 每轮都会原样进系统提示。无界的整段文件粘贴（或误把整个 md 塞进来）会让
// 每一轮的入场费悄悄翻几倍，而屏幕上只看得到成员名。
//
// 为什么不截断：截断会让成员实际看到的提示与 .claude/agents 里的文件内容
// 不一致，而且这种不一致没有任何一层会报出来——用户改的是文件，跑的是
// 另一段话。超限就报校验错误，让人去删内容。
//
// 8 KiB 是任务书定的值，不是实测出来的临界点；放这里而不是散在 Validate
// 里，是因为它同时是给用户看的契约（错误文案与文档都引用同一个数）。
const MaxAgentPromptBytes = 8 << 10

// AgentConfig 描述一个 agent。
type AgentConfig struct {
	DisplayName string     `toml:"display_name"`
	Provider    string     `toml:"provider"`
	Model       string     `toml:"model"`
	Effort      string     `toml:"effort"`
	Role        string     `toml:"role"`
	Permissions Permission `toml:"permissions"`
	Mode        Mode       `toml:"mode"`
	// Prompt 是整段外部系统提示，主要来源是「从 Claude Code 只读导入」：
	// frontmatter 之后的正文就是 Claude 那边的系统提示。
	//
	// provider 层只有 claude 消费它（拼进 --append-system-prompt-file 指向的
	// 文件，每轮都带）；codex / grok / agy 不消费，它们的 argv 不因为这个
	// 字段而改变。它计入身份指纹：改了 .claude/agents/x.md 的正文等于换了
	// 一副脑子，旧 provider 会话必须作废。
	//
	// 上限见 MaxAgentPromptBytes；Validate 拒绝超限值，绝不静默截断。
	Prompt string `toml:"prompt"`
}

// TeamConfig 描述一个由 agent id 组成的团队。
type TeamConfig struct {
	DisplayName string   `toml:"display_name"`
	Members     []string `toml:"members"`
	// ThoughtVisibleTo 列出哪些 Mode 能在 Render 正文里看到历史消息的 Thought。
	// 空切片与 nil 都表示谁都看不到，不要把它们区分开。典型值 ["judge"]。
	//
	// 用 []Mode 而不是裸 bool：04 第 3 节要表达的是「哪些人看得到」，
	// 一个全局 bool 表达不了「只有裁判看得到」这个典型用法。
	//
	// 判别器只有 Mode。未决 #7 已裁：Role 进身份块、进指纹，拿它分支
	// 等于让「把某人改成裁判」变成一次 ErrIdentityDrift 加一次入场费。
	//
	// 本字段不进 IdentityFingerprint 的任何一段。thought 只进 Render
	// 出来的正文，不进身份块、不进前缀：它让此后的 delta 变大，不改
	// 已经送出去的前缀，按 04 §0 规则 1 是合法的追加。反过来也不回溯
	// ——已经投递过的消息不会被补上 thought，因为那就是改写前缀。
	// 下一个人很可能出于「保险」把它塞进指纹，那一塞就是每改一次开关
	// 全队重付入场费（5 人量级 90k-240k token）。
	ThoughtVisibleTo []Mode `toml:"thought_visible_to"`
}

// ValidationError 用点分 Path 定位到出错的键，例如 agents.theory.provider。
type ValidationError struct {
	Path string
	Msg  string
}

// Error 返回 path: msg 形式的单条校验错误。
func (e ValidationError) Error() string {
	return e.Path + ": " + e.Msg
}

// ValidationErrors 一次带回全部校验错误。Error 按 Path 字典序逐行拼接。
type ValidationErrors []ValidationError

// Error 把各条错误按 Path 字典序排序后逐行拼接，每行形如 path: msg。
// 同 Path 时再按 Msg 排序，避免 map 遍历带来的不稳定顺序。
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	errs := make(ValidationErrors, len(e))
	copy(errs, e)
	sort.SliceStable(errs, func(i, j int) bool {
		if errs[i].Path != errs[j].Path {
			return errs[i].Path < errs[j].Path
		}
		return errs[i].Msg < errs[j].Msg
	})
	lines := make([]string, len(errs))
	for i, err := range errs {
		lines[i] = err.Error()
	}
	return strings.Join(lines, "\n")
}
