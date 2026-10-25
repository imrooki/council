// Package conversation 定义 Canonical Conversation 的领域类型与领域动作。
//
// 本包是领域层：它不认识 SQL，也不认识配置文件。持久化由 Store 接口给出
// （见 store.go），实现在 internal/store 里。本包不导入 database/sql，
// 也不导入 internal/store；依赖方向是 internal/store -> internal/conversation，
// 反向不成立（docs/08_STEP5_SPEC.md B 节）。
package conversation

import "time"

// MessageKind 是消息的种类。它是字符串而不是整数：council 是一个本地优先的工具，
// 用户会自己拿 SQLite 查看器打开库看，库里写 "agent" 比写 2 有用得多
// （docs/08_STEP5_SPEC.md D 节）。
type MessageKind string

// 五个合法的消息种类。messages.kind 上的 CHECK 约束就是这五个值。
const (
	KindUser       MessageKind = "user"
	KindAgent      MessageKind = "agent"
	KindSystem     MessageKind = "system"
	KindTask       MessageKind = "task"
	KindTaskResult MessageKind = "task_result"
)

// InflightPhase 是 inflight_turns.phase 的三态。做成具名字符串而不是整数或裸 string：
// 和 MessageKind 一样，打开库能直接读到 "begun"，同时 Valid 能拒掉第四态。
// 零值是空串，不是 PhaseBegun。列上的 DEFAULT 'begun' 只在 SQL 插入时生效，
// 领域侧漏写 Phase 不会自动变成 begun。三态的恢复动作相反，合成一档时
// Begun 会被当成已经打过调用而多付一次，Returned 会被当成可以重跑
// 而丢掉已经付费拿到的正文。
type InflightPhase string

// 三个合法阶段，对应 inflight_turns.phase 的取值。
const (
	// PhaseBegun 表示记录写下了，进程还没起（BeginTurn 在 r.spawn 之前）。
	// 这一态还没向 provider 开口；当成已经打过调用去恢复，会把没发生的花费记成待结算。
	PhaseBegun InflightPhase = "begun"
	// PhaseSpawned 表示进程起过了，还没拿到可解析的回复。
	// 不能当作没开始（调用可能已经计费），也不能把没有可解析正文当成已经可以落库。
	PhaseSpawned InflightPhase = "spawned"
	// PhaseReturned 表示 provider 回复解析成功了，但正文还没落库。
	// 降回 spawned 再跑，已解析的正文会丢，或和重试结果写成两条消息。
	PhaseReturned InflightPhase = "returned"
)

// ReservedSenderUser 是人在对话里的 sender id。它不是 participant，
// 不需要 join，也不出现在成员列表里，但可以作为 Message.SenderID。
const ReservedSenderUser = "user"

// ReservedSenderSystem 是 Council 自己记账时用的 sender id：
// 会话建立、会话重建、一轮失败这类**事件**，而不是任何人或任何 agent 说的话。
//
// 它与 ReservedSenderUser 同类：不是 participant，不需要 join，不出现在成员列表里，
// 三个脑子列必须为空，Post 对它跳过 RequireActiveParticipant。
//
// 为什么不拿「那个 agent 自己的 id」当 sender（docs/09a）：
// 一条「你的会话开了」不是那个 agent 说的话，是 Council 替它记的账；
// 挂在它名下，/log 里会显示成它说的，Step 7 的 Router 还会把它当成那个成员的观点
// 转给别人。更要命的是非保留 id 要过事务内的 RequireActiveParticipant，
// 而这类事件恰恰常常发生在刚 /leave 或正被重配的成员身上 ——
// 那样 08a 明令必须留下的那条记录一条都留不下。
//
// 已核实不需要改 schema：messages 上的 CHECK 只约束 'user'，sender_id 没有外键，
// kind 的 CHECK 本来就含 'system'。配套把 "system" 也列进 config 的保留 agent id。
const ReservedSenderSystem = "system"

// MetaThoughtDropped 是消息 Meta 里的键：true 表示这条消息原本带着非空
// Thought，但 Service 按当时生效的存储策略（未决 #9(a)，见 Service.SetThoughtPolicy）
// 把它落成了空串。只在真的丢掉了非空内容时才写这个键——空串不许同时表示
// 「这家 provider 没有 thought 这个概念」与「我们按用户要求丢了」，这与
// 未决 #11 里 sender_effort 的裁法是同一个道理。
//
// 命名与 internal/turn/record.go 的 MetaTextDropped（post_rejected 那条
// system 面包屑专用）对齐，含义类似但场景不同，两者不是同一个键。
// 常量声明在这里而不是 internal/turn：turn 依赖 conversation，反过来不行，
// 这个键由 conversation.Service 自己决定要不要写，下游包不该替它定义。
const MetaThoughtDropped = "thought_dropped"

// MetaEvent 是 system 面包屑 Meta 里区分事件种类的键。
// 与 internal/turn.MetaEvent 是同一个字符串。本包不能导入 turn
// (turn 依赖本包)，BindTeam 要写这条面包屑，只能在这边再声明一次。
const MetaEvent = "event"

// EventTeamBound 是 BindTeam 写进 MetaEvent 的值。
const EventTeamBound = "team_bound"

// MetaTeamID 是 BindTeam 写下的 system 面包屑里所绑 team 的键。
// 与 conversations.team_id 列同名，读回时不必再翻一份对照表。
const MetaTeamID = "team_id"

// Conversation 是一次会话。ID 与 CreatedAt 由 store 在事务里填。
type Conversation struct {
	ID        string
	Title     string
	CreatedAt time.Time
	// TeamID 是本会话绑的 team。空串表示没绑。
	// DDL 是 NOT NULL DEFAULT ''，空串就是「没绑」这一个含义，没有第二种，
	// 所以不用指针、不用可空类型。
	TeamID string
	// Cwd 是项目目录，创建会话时启动目录的规范化绝对路径。
	// 空串表示没绑（老会话，或取不到启动目录）。
	// DDL 是 NOT NULL DEFAULT ''，空串就是「没绑」这一个含义，不用指针。
	Cwd string
	// Archived 说明这场会话已经被归档：它只是从会话列表里收起来，
	// 详情、消息、用量与续接一概照旧（归档不阻止继续对话）。
	// 库里存的是 conversations.archived_at（可空时间戳），这里收成 bool：
	// 调用方只关心「是不是归档了」，第一次归档的时刻没有对外含义。
	Archived bool
}

// Participant 是会话里的一个成员。participant 不等于 provider：
// 多个 participant 可以共用一个 provider（docs/05_MESSAGE_PROTOCOL.md 第 1 节）。
// 退出是软删除 —— Active 置 false、LeftAt 记时间，它说过的话仍留在消息里。
type Participant struct {
	ConvID      string
	AgentID     string
	DisplayName string
	Provider    string
	Active      bool
	JoinedAt    time.Time
	LeftAt      *time.Time // nil 表示还在
}

// Message 是 Canonical Conversation 里的一条消息。
// SenderID 是 participant id，不是 provider 名。
type Message struct {
	ID       string
	ConvID   string
	Seq      int64 // 由 store 在事务里分配，调用方传进来的值一律忽略
	SenderID string

	// SenderProvider / SenderModel / SenderEffort 是这一次发言当时用的「脑子」，
	// 是该条消息的属性，不是 participant 的属性。
	//
	// 理由见 docs/00_DECISIONS.md D-4 与 docs/02_AGENT_PROVIDER_ARCHITECTURE.md 第 4 节：
	// 换 model 或 effort 必须重建会话，所以同一个 agent 在一场对话里可能换过脑子。
	// 把它记在 participants 表上，等于把它过去说过的话追溯性地重新标注成新脑子说的。
	//
	// SenderID 是保留 id ReservedSenderUser 时，这三个字段必须为空
	// （Service.Post 会拦，messages 表上也有 CHECK 兜底）。
	//
	// 两层拦的都是 **SenderID**，与 Kind 无关。这一点容易读反，所以写全：
	// Kind 是 KindUser 但 sender 是某个 agent 的消息**不受**这条约束；
	// 而 SenderID 是 ReservedSenderUser 的消息**一律受**这条约束，Kind 是什么都照拦。
	// Step 6 若要按 Kind 立规矩（例如要求 KindAgent 必须带脑子），得另加一层。
	// Step 5 只铺列与渲染，Step 6 由 provider 适配器填充；届时填进来的是请求时声明的档位，
	// 不是实测档位 —— grok 的 effort 由它自己的 config 决定，命令行参数会被静默丢弃
	// （docs/02_AGENT_PROVIDER_ARCHITECTURE.md 第 1.1 节，EffortPolicy 为「进程外配置」）。
	SenderProvider string
	SenderModel    string
	SenderEffort   string

	Recipients []string // 空 = 广播给当前活跃成员
	Kind       MessageKind
	Text       string
	Thought    string // provider 的思维链摘要，默认存但默认不路由（docs/04_CONTEXT_ROUTER.md 第 3 节）
	ParentID   string // 空表示没有父消息
	CreatedAt  time.Time
	Meta       map[string]any
}

// Cursor 是某个 agent 在某个会话上的投递游标：Seq 是已成功投递并被处理的最大 seq，
// Pending 是已发出、尚未确认的 seq（docs/04_CONTEXT_ROUTER.md 第 1 节与第 6 节）。
// Step 5 只提供游标的读写，不决定送什么。
type Cursor struct {
	ConvID    string
	AgentID   string
	Seq       int64
	Pending   []int64
	UpdatedAt time.Time
}

// TurnUsage 是一轮的记账，与 turn_usage 表一一对应。
//
// 七个可空的用量字段用 *int / *float64：nil 表示「这家 provider 没报这个数」，
// 指向 0 表示「实测为 0」。这两件事在 Step 6 的真实字节里都出现过，
// 用 0 冒充 nil 会让成本视图显示 agy 与 grok 从不建缓存。
// Step 6 证明了三种情况在 v1 里长得一样：agy 的 usage 里根本没有
// cache_creation；grok 的 cache_creation_input_tokens 恒为 0 而
// cache_read 从 0 涨到 20608；三份失败原件里 claude 与 agy 的五项
// 是货真价实的 0。读的时候绝不能把 NULL 收成 0。
type TurnUsage struct {
	TurnID            string // 调用方铸的 v4 UUID，一轮一个
	ConvID            string
	AgentID           string
	ProviderSessionID string
	Seq               *int64 // 成功轮指向它产生的消息；失败轮 nil
	Outcome           int    // turn.Outcome 的整数值

	InputTokens    *int
	CacheCreate    *int
	CacheRead      *int
	OutputTokens   *int
	ThinkingTokens *int

	CostUSD           *float64 // 算好的单轮差值。nil 有三种读法，见 09a Record.CostUSDDelta
	CostCumulativeUSD *float64 // provider 报的累计原值，原样

	ColdStart   bool
	WallMS      int
	PromptBytes int
	DeliveredLo *int64 // 本轮渲染进 prompt 的 seq 区间；空批次两个都是 nil
	DeliveredHi *int64
	CreatedAt   time.Time
	// IdentityFP 是这一轮算出来的身份指纹。空串读作「这一轮没算到指纹」，
	// 不是「指纹是空的」——见 turn.Record.IdentityFP 的注释。
	IdentityFP string
	// RoundID 是这一轮所属的扇出门票，与 turn.Record.RoundID 同一份。
	RoundID string
}

// InflightTurn 是一轮已经开始、正文还没落进 messages 的在途记录，
// 字段与 inflight_turns 的 12 列一一对应、顺序相同。不放进 TurnUsage：
// 那边是回合结束后的账，这一行要活过 BeginTurn 到落库之间的进程崩溃。
//
// Phase 是 InflightPhase 三态，不是布尔：
// PhaseBegun 记录写下了，进程还没起（BeginTurn 在 r.spawn 之前）；
// PhaseSpawned 进程起过了，还没拿到可解析的回复；
// PhaseReturned provider 回复解析成功了，但正文还没落库。
// 合成一态时，Begun 会被当成已经打过调用而多付一次，Returned 会被当成
// 可以重跑而丢掉已经付费拿到的正文。Phase 的零值是空串，不是 begun：
// SQL 的 DEFAULT 'begun' 不会出现在这个 Go 值里，漏写要靠 Valid 拦住。
type InflightTurn struct {
	TurnID           string
	ConvID           string
	AgentID          string
	Provider         string
	PlannedSessionID string
	ResumeSessionID  string
	CursorSeq        int64
	ScannedTo        int64
	StartedAt        time.Time
	Phase            InflightPhase
	Tier             string
	WorkDir          string
}

// ProviderSession 是一条 provider 会话在库里的全部状态。
//
// 这个结构体必须整个读、整个写，不给任何一个字段单独的 setter。
// 五样东西（ProviderSessionID / CursorSeq / CostTotalUSD / LastTurnAt /
// IdentityFP）同生共死：读回时缺任何一样，五样全部作废，该 agent 按 Fresh 处理。
// 理由见 10_STEP7_SPEC.md 第 4 节 —— 其中最要命的是成本基线：
// 重建意味着 provider 那边是一条新会话、累计计数器从零开始，
// 若只作废会话 id 而留着旧基线，下一轮的差值是负数，computeCostDelta 返回 nil，
// 那一笔真实花掉的钱就静默从账上消失了。
type ProviderSession struct {
	ConvID            string
	AgentID           string
	ProviderSessionID string
	CursorSeq         int64
	CostTotalUSD      float64
	HaveCostTotal     bool // false 表示还没见过任何金额（grok / agy 恒为 false）
	LastTurnAt        time.Time
	IdentityFP        string
	Spoken            bool
	// BaselineSuspect 不是第六样，而是关于那五样（ProviderSessionID /
	// CursorSeq / CostTotalUSD / LastTurnAt / IdentityFP）的标记，
	// 不改变、也不参与「五样同生共死」的读回规则。并进那五样一起作废，
	// 一次可疑标记就会丢掉仍然有效的会话；把它当成可以单独缺失的列，
	// 坏基线又会继续参与下一轮差额。
	BaselineSuspect bool
}

// LostSettlement 是 SettleLostTurn 一笔对账的结果。
//
// 三个字段各自独立。SuspectMarked 不等于「phase != begun」：
// bootstrap 轮崩溃时根本没有 provider_sessions 那一行可标。
type LostSettlement struct {
	// ConvMissing 为真表示这条会话已经不在了：面包屑与 turn_usage 都跳过了。
	ConvMissing bool
	// SuspectMarked 为真表示确实把某一行 provider_sessions.baseline_suspect 置成了 1。
	// 它不等于「phase != begun」：bootstrap 轮崩溃时根本没有那一行可标。
	SuspectMarked bool
	// Existed 报告 ClearInflight 删之前这一行还在不在。
	Existed bool
}

// SenderBrainEmpty 报告这条消息有没有记下发言当时的脑子。
//
// 只问「是不是三个都空」，不要求三个同时填满：有的 provider 根本没有 effort 这个概念
// （docs/02_AGENT_PROVIDER_ARCHITECTURE.md 第 1.1 节），只填 provider 与 model 是合法的。
func (m Message) SenderBrainEmpty() bool {
	return m.SenderProvider == "" && m.SenderModel == "" && m.SenderEffort == ""
}

// Valid 报告这个 kind 是否属于五个合法值之一。
func (k MessageKind) Valid() bool {
	switch k {
	case KindUser, KindAgent, KindSystem, KindTask, KindTaskResult:
		return true
	default:
		return false
	}
}

// Valid 报告这个阶段是否属于三态之一。
// 非法值若拖到崩溃恢复才发现，调用方只能猜该重跑还是该落库；写入前拒绝，
// 多一次判断，换回来的是空串不会被当成默认的 begun。
func (p InflightPhase) Valid() bool {
	switch p {
	case PhaseBegun, PhaseSpawned, PhaseReturned:
		return true
	default:
		return false
	}
}
