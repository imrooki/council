package turn

import (
	"crypto/rand"
	"fmt"
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
	// OutcomeCanceled = 8: 取消来自上层（用户按 Ctrl-C，main.go 的 NotifyContext
	// 取消了父 ctx），不是墙钟到点。与 OutcomeTimeout 的暴露面完全一样
	// （进程跑过、结局未知、maybeAdvanced=true），变的只是标签，让记账分得开。
	//
	// 值必须写成字面量，不许接着 iota 往下数：Step 10 也要在这个块末尾追加一个
	// OutcomeLost，两个追加者互不知道对方，而落地顺序可以反过来（Step 10 的 A 片
	// 允许提前落地）。turn_usage.outcome 存的是整数，顺序一反同一个 8 就有两种
	// 理解，两台机器迁移出来的库会互相读错。9 留给 OutcomeLost，不许占用。
	OutcomeCanceled Outcome = 8
	// OutcomeLost = 9: 进程崩在一轮中间，启动对账补上的那一行。
	// 值必须写成字面量，不许接着 iota 往下数，与上面 OutcomeCanceled 是同一条
	// 理由：turn_usage.outcome 存的是整数，块里已经把 9 留给它。
	//
	// 它不进 Ingested()。丢失的轮没有把 delta 送达，游标绝不能推；
	// 加进去会让一次崩溃把游标推过一段没人看见的正文。
	OutcomeLost Outcome = 9
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
	// TurnID 是这一轮的身份，Ask 一进来就铸（v4 UUID），无论结局如何都有值。
	// turn_usage 的主键是它，于是记账不再依赖 messages 产出的 seq，
	// 失败轮也记得上。
	TurnID string
	// RoundID 是这一轮所属的扇出门票。Ask 自己铸一个再交给 AskUpTo；
	// 扇出时由调用方在开闸之前铸一个、发给全部 N 个成员。
	// 只记账，绝不进 prompt、绝不进身份块、绝不进指纹。
	// 写入 turn_usage.round_id。Ask 铸了再传，这一列永远非空。
	RoundID string
	// IdentityFP 是这一轮算出来的身份指纹，写入 turn_usage.identity_fp。
	//
	// 空串读作「这一轮没算到指纹」，不是「指纹是空的」。
	// fp 算出来之前就失败的路径只能是空串：配置为空、agent 不在配置里、
	// provider 没适配器、权限档不放行、WorkDir 为空、建目录失败、
	// identityFPFor 自己报错。DDL 是 NOT NULL DEFAULT ''，没有 NULL
	// 这个选项可用 —— 这与未决 #22「不知道就写 NULL 不写 0」是同一类
	// 问题的不同处置：列的可空性由契约定死了(docs/12_STEP9_CONTRACT.md 3.3)，
	// 不是这一片能改的。
	IdentityFP string

	ConvID  string
	AgentID string
	Outcome Outcome

	// ---- messages ----
	// System 是这一轮追加的 KindSystem 面包屑，没有则为 nil。四种事件，由
	// Meta["event"] 区分：session_opening（起进程之前发）、turn_failed（跑完但
	// 没产生发言）、post_rejected（provider 吃下了，本地记不上）、
	// session_rebuilt（读回的 provider 会话因指纹不符被作废重建，未决 #18）。
	// 后者与 session_opening 可能同一轮各发一条——Record.System 只留得住
	// 最后一条（session_opening 在 Ask 流程里发生在更后面），第一条仍会
	// 落进只追加的事实源，只是不经这个字段回显。
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

	// DeniedActions 是这一轮里被 headless 模式自动拒绝的工具调用（provider 层
	// 归一化形状见 provider.DeniedAction 的注释）。今天只有 agy 会填它。
	// 它不参与 Outcome 判定：界内的事可能已经做完，判成失败是另一种谎
	// （agy.go 与 docs/11c_STEP8B_WORKSPACE_CONTRACT.md 第 7 节）。它是一条
	// 附加事实，只用来解释「正文为什么是空的」，不改变这一轮的结局。
	DeniedActions []provider.DeniedAction

	// ---- turn_usage ----
	Usage provider.Usage
	// UsageParsed 表示 ParseTurn 是否交出过认得出形状的结果（absorbParse 是否跑过）。
	// 它不是「用量是不是零」：没解析过是「不知道」（必须写 nil），解析过且为 0
	// 才是实测为零（必须写指向 0 的指针）。判据只能是「解析过没有」。
	UsageParsed bool
	// CostUSDDelta 是算好的单轮差值。nil 有三种读法，靠 Meta 区分，绝不猜一个数：
	// provider 不报金额；上一轮结局未知、基线可疑；本轮累计值 < 基线。
	// 永不写负数，也永不 max(0, delta)。
	CostUSDDelta    *float64
	BaselineSuspect bool // 上一轮的结局是未知的，本轮差值一律 nil 并记 Meta
	// Cold 取 Usage.CacheRead == 0，只在 askUpToWith 的成功路径上赋值：失败的各条路
	// 经 failClosed 返回，都早于这一步。所以只有结局是 OutcomeSpoke / OutcomePostRejected
	// 的轮上它才赋过值；其余结局上它是没赋过的零值 false，不是「确知不冷」。
	// Usage.Unreported 含 "cache_read" 时它赋过值但没有含义（那个 0 是没填）。
	// 展示与结算（writeAskMeter、Settle）判冷暖一律经 MeasuredCold，不直接读这个字段。
	// 写侧原样用它：UnexpectedCold 的计算紧跟在赋值之后；makeTurnUsage 把它落成
	// turn_usage.cold_start，库的读者（/cost、取样）自己按 Ingested 门控。
	Cold bool
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

// MeasuredCold 报告这一轮的冷暖是不是实测值，以及实测是冷是暖。
// known 为 false 时 cold 恒为 false，调用方不许读它。
//
// known 当且仅当以下三条同时成立：
//  1. rec.Outcome.Ingested()：Cold 只在 askUpToWith 的成功路径上赋值，失败的各条路
//     经 failClosed 提前返回、早于那一步，结局被定成 OutcomeSpoke / OutcomePostRejected
//     都在那一步之后；其余结局上的 Cold 是没赋过的零值。
//  2. rec.UsageParsed：没解析过用量就没有 CacheRead 可判。
//  3. rec.Usage.Unreported 不含 "cache_read"：标了未报时 CacheRead 是没填的 0。
//
// 非 Ingested 的轮不论 CacheRead 是多少都是未知。
func (rec Record) MeasuredCold() (cold, known bool) {
	if !rec.Outcome.Ingested() || !rec.UsageParsed || unreportedHas(rec.Usage.Unreported, "cache_read") {
		return false, false
	}
	return rec.Cold, true
}

// ReportedUsage 是一轮用量里「有数」的那几项，与 makeTurnUsage 写进 turn_usage 的五个可空列一一对应：
// 指针为 nil 读作库里的 NULL —— 没解析到用量、provider 标了未报、或 CacheCreate 本来就是 nil。
type ReportedUsage struct {
	Input, CacheCreate, CacheRead, Output, Thinking *int
}

// ReportedUsage 按写库那五列的规则返回这一轮报过的用量。非 nil 指针都是新分配的值。
func (rec Record) ReportedUsage() ReportedUsage {
	var ru ReportedUsage
	if !rec.UsageParsed {
		return ru
	}
	if !unreportedHas(rec.Usage.Unreported, "input_tokens") {
		v := rec.Usage.InputTokens
		ru.Input = &v
	}
	if !unreportedHas(rec.Usage.Unreported, "cache_read") {
		v := rec.Usage.CacheRead
		ru.CacheRead = &v
	}
	if !unreportedHas(rec.Usage.Unreported, "output_tokens") {
		v := rec.Usage.OutputTokens
		ru.Output = &v
	}
	if !unreportedHas(rec.Usage.Unreported, "thinking_tokens") {
		v := rec.Usage.ThinkingTokens
		ru.Thinking = &v
	}
	if !unreportedHas(rec.Usage.Unreported, "cache_create") && rec.Usage.CacheCreate != nil {
		v := *rec.Usage.CacheCreate
		ru.CacheCreate = &v
	}
	return ru
}

// Meta 键。写进库的东西必须有一个共享的名字。
const (
	MetaEvent               = "event"                 // system 消息：session_opening | turn_failed | post_rejected | session_rebuilt | round_started | round_finished | turn_lost
	MetaAboutAgent          = "about_agent"           // system 消息：这件事关于谁；轮次面包屑不写这个键
	MetaRoundID             = "round_id"              // round_started / round_finished；与 turn_usage.round_id 同一值
	MetaUpTo                = "upTo"                  // round_started：本轮 Speakers 共用的 seq 上界
	MetaSpeakers            = "speakers"              // round_started：本轮实际扇出的成员名单
	MetaEffortPolicy        = "effort_policy"         // per_turn_flag | process_external_config | ...
	MetaEffortSource        = "effort_source"         // 恒为 requested
	MetaEffortFlagOmitted   = "effort_flag_omitted"   // bool：发了也会被吞，所以没发
	MetaObservedModel       = "observed_model"        // claude-fable-5-1 / grok-4.6-build；agy 不写这个键
	MetaModelSource         = "model_source"          // requested | observed | ambiguous
	MetaProviderSession     = "provider_session_id"   // Step 7 退休
	MetaCostCumulativeUSD   = "cost_cumulative_usd"   // claude 的累计原值。Step 7 退休
	MetaCostBaselineSuspect = "cost_baseline_suspect" // bool
	MetaUsageRaw            = "usage_raw"             // provider 的原始 usage 对象，原样
	MetaUsageUnreported     = "usage_unreported"      // []string，哪些 0 是「没报」
	MetaCompletion          = "completion"            // complete | truncated | empty | unknown
	MetaStopRaw             = "stop_raw"              // 原始 stop_reason / stopReason / status
	MetaDelivered           = "delivered_seqs"        // []int64
	MetaPromptBytes         = "prompt_bytes"
	MetaDropped             = "dropped"            // []DroppedOption
	MetaTextDropped         = "text_dropped"       // bool，只在 post_rejected 的面包屑上
	MetaDeniedActions       = "denied_actions"     // []provider.DeniedAction，今天只有 agy 会写
	MetaDeniedActionsRaw    = "denied_actions_raw" // provider 这个键的原始字节，原样
	// 空串不写这个键：写下一个空值，读回的人会以为缺了 id，而空恰恰是
	// 「点不出名字」本身。id 只进面包屑与屏幕，不进任何 TurnRequest，
	// 也不拿去 resume。
	MetaOrphanSession = "orphan_session_id" // 只在 turn_lost 面包屑上、且 OrphanID 非空时才写
)

// newV4UUID 铸一个 RFC 4122 version 4 UUID（8-4-4-4-12）。
//
// 与 provider/uuid.go 是同一份算法的两处实现，因为那边是包内私有；
// 两处都不许换成第三方库。不引入 github.com/google/uuid：那是
// modernc.org/sqlite 的传递依赖，不是我们的 API。
func newV4UUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
