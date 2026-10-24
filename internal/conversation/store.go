package conversation

import (
	"context"
	"errors"
	"time"
)

// Store 是 Canonical Conversation 的持久化出口。
//
// 接口定义在消费方这一侧（conversation）而不是实现方（internal/store），是有意的：
// Go 的惯例是消费方定义接口，这样领域层能在测试里换一个内存实现，
// 不必拖着 SQLite 跑（docs/08_STEP5_SPEC.md B 节）。实现方满足签名即可，
// 不需要、也不应该反过来让本包去导入它。
//
// 写操作一律在 WriteTx 里完成，读操作走读句柄、不进写事务。
type Store interface {
	// WriteTx 把 fn 里的所有写操作放进同一个写事务。fn 返回非 nil 就整体回滚，
	// fn 里 panic 同样要回滚而不是泄漏事务。fn 内部只许用 Tx 上的方法，不许自己拿连接。
	WriteTx(ctx context.Context, fn func(context.Context, Tx) error) error

	// Conversation 按 id 取会话。不存在时返回包了 ErrConversationNotFound 的错误。
	// 已软删的会话同样按不存在处理。
	Conversation(ctx context.Context, id string) (Conversation, error)
	// Conversations 列出全部未软删的会话，按创建时间倒序。
	Conversations(ctx context.Context) ([]Conversation, error)
	// Participants 列出某会话的成员。includeInactive 为 false 时只返回活跃成员。
	Participants(ctx context.Context, convID string, includeInactive bool) ([]Participant, error)
	// Messages 取 seq 严格大于 afterSeq 的消息，按 seq 升序，最多 limit 条。
	// 没有消息时返回空切片而不是 nil。
	Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]Message, error)
	// MaxSeq 返回某会话当前的最大 seq；空会话返回 0。
	MaxSeq(ctx context.Context, convID string) (int64, error)
	// Cursor 取某个 agent 在某会话上的游标。没有时返回包了 ErrNoCursor 的错误。
	Cursor(ctx context.Context, convID, agentID string) (Cursor, error)
	// Cursors 列出某会话的全部游标。
	Cursors(ctx context.Context, convID string) ([]Cursor, error)

	// ProviderSession 读回一条 provider 会话。第二个返回值为 false 表示
	// 这个 agent 在这条会话里还没开过口（或上一次的状态已被作废）。
	ProviderSession(ctx context.Context, convID, agentID string) (ProviderSession, bool, error)

	// AllProviderSessions 列出全库的 provider 会话，按 (conv_id, agent_id) 升序。
	// 只读，给启动漂移审计与 /rules 用。库里一条都没有时返回空切片而不是 nil，
	// 也不是错误 —— 「一条都没有」是正常状态，不该逼调用方去分辨 nil 与空。
	AllProviderSessions(ctx context.Context) ([]ProviderSession, error)

	// AllInflight 列出全库正在飞的轮，按 started_at 升序。只读，给恢复扫描
	// （下一刀 AuditInflight/SettleLostTurns）与诊断用。库里一条都没有时
	// 返回空切片而不是 nil。
	AllInflight(ctx context.Context) ([]InflightTurn, error)

	// InflightFor 读回 (convID, agentID) 那一行在飞记录。第二个返回值 false
	// 表示没有这一行，不是错误。走 (conv_id, agent_id) 的唯一索引，是给每一轮
	// 起进程之前的那道检查用的定点读，不像 AllInflight 那样读完整张表。
	// 一个 (conv, agent) 最多一行，由 v5 的唯一索引保证。
	InflightFor(ctx context.Context, convID, agentID string) (InflightTurn, bool, error)

	// UsageSamples 按 (agent_id, identity_fp) 取 turn_usage，created_at 倒序、
	// turn_id 倒序作第二键，最多 limit 条。
	//
	// 指纹必须进键：指纹变了，前缀作废，规模不可比。拿旧指纹的样本去预报
	// 新指纹的一轮，是拿另一份前缀的数字冒充。
	//
	// 样本口径：input_tokens 非 NULL 即算，包括 OutcomePostRejected。
	// 钱花了、用量也记了，排除它往低估偏。prompt_bytes == 0 的行仍返回：
	// 它们是样本，只是不能当缩放的除数，跳过是调用方的事。
	//
	// limit > 0 最多取这么多条。limit == 0 是「要零条」，返回空切片。
	// limit < 0 不限：Budget 要跳过 prompt_bytes == 0 继续找更早的合格行，
	// 先 LIMIT 再在内存里丢会把「最近一条不能用」误报成「没有样本」。
	// 代价是该指纹下全部非 NULL 用量行进内存。不编一个「最近 N 条」的常数
	// 来假装够用。空结果是空切片不是 nil。
	UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]TurnUsage, error)

	// TurnUsages 按 conv_id 取 turn_usage，created_at 升序、turn_id 升序作第二键。
	// agentID 非空时再按 agent_id 过滤；空串表示整条会话，不是去匹配空的 agent_id。
	//
	// 与 UsageSamples 不是同一条路。那边要样本去缩放，所以丢掉 input_tokens 为 NULL 的行；
	// 这边是给人看一条会话的账，那些什么都不知道的行（含 OutcomeLost）正是要读出来的。
	// 滤掉它们就是把「不知道」藏起来。
	//
	// limit 与 UsageSamples 同一套：limit > 0 最多取这么多条，limit == 0 返回空切片，
	// limit < 0 不限。limit > 0 取的是末尾这么多条（最近的），不是开头。
	// 空结果是空切片不是 nil。
	TurnUsages(ctx context.Context, convID, agentID string, limit int) ([]TurnUsage, error)
}

// Tx 是一个写事务里能做的全部动作。
//
// Tx 存在的理由是事务边界，不是为了给每张表配一组 CRUD：Step 7 的「记一轮」
// 要把消息、游标、turn_usage、provider_sessions 四张表写在同一个事务里
// （docs/08_STEP5_SPEC.md A 节那张表）。届时加的是本接口上的两个方法
// RecordUsage 与 SetProviderSession，不是一次重构；而把 *sql.Tx 漏到 store
// 之外去拼这四件事，会让这层抽象当场作废。
type Tx interface {
	// CreateConversation 建一个新会话，返回补全后的 Conversation。
	CreateConversation(ctx context.Context, title string) (Conversation, error)
	// SetConversationTeam 把会话绑到 teamID。空串拒绝，不是解绑：
	// 空串在 team_id 列上读作「没绑」，理由见实现注释。
	SetConversationTeam(ctx context.Context, convID, teamID string) error
	// SetConversationCwd 把会话的项目目录设成 cwd。空串允许，表示清空（读作没绑项目），
	// 与 SetConversationTeam 拒绝空串不同。会话不存在时返回包了 ErrConversationNotFound 的错误。
	SetConversationCwd(ctx context.Context, convID, cwd string) error
	// AppendMessage 追加一条消息，返回补全后的 Message：
	// ID、Seq、CreatedAt 由 store 填，传进来的这三个字段一律忽略。
	AppendMessage(ctx context.Context, m Message) (Message, error)
	// AddParticipant 加入成员。同一个 (ConvID, AgentID) 之前退出过的话，
	// 这是重新激活，不是插第二行（docs/08_STEP5_SPEC.md G 节第 1 条）。
	AddParticipant(ctx context.Context, p Participant) error
	// DeactivateParticipant 软删除成员：active 置 0、left_at 记为 at，不动任何消息。
	DeactivateParticipant(ctx context.Context, convID, agentID string, at time.Time) error
	// RequireActiveParticipant 在**同一个写事务内**确认 agentID 是 convID 当前的活跃成员，
	// 不是就返回包了 ErrParticipantNotFound 的错误。形状与 store 内部的
	// requireConversation 一致。
	//
	// 它必须在事务内，这是本方法存在的全部理由（未决 #13）：Service.Post 原先用读句柄
	// 先查一次活跃成员、再另起写事务追加消息，两者之间有窗口。Step 6 起有并发发言之后，
	// 读到「活跃」之后、提交之前用户 /leave 先行提交，这条消息照样落库 ——
	// 事后 /who 显示它已退出、left_at 还早于这条消息的 created_at，没有任何一层会报错。
	RequireActiveParticipant(ctx context.Context, convID, agentID string) error
	// SetCursor 按 (ConvID, AgentID) upsert 游标的 seq 与 pending。
	SetCursor(ctx context.Context, c Cursor) error

	// RecordUsage 插入一行 turn_usage。TurnID 冲突时返回错误，不 upsert：
	// 同一个 turn_id 写两次说明调用方把一轮记了两遍，那是 bug 不是重试。
	RecordUsage(ctx context.Context, u TurnUsage) error

	// SetProviderSession 按 (ConvID, AgentID) upsert 一条 provider 会话的全部状态。
	// 收全字段结构体，不提供部分更新的入口。
	SetProviderSession(ctx context.Context, s ProviderSession) error

	// DropProviderSession 删掉一条 provider 会话。作废走这里，
	// 不要用「写一个空的 ProviderSession」来表达作废 —— 那会留下一行
	// provider_session_id 为空串的记录，与「还没开过会话」在库里长得一样。
	//
	// existed 报告这一行本来存在不存在。实现必须直接用 DELETE 自身的结果
	// （例如 RowsAffected）来回答，不能先读一次再删 —— 读要解析这一行的
	// 每一列，一行损坏的记录会让读先失败，而 DropProviderSession 存在的
	// 意义之一就是清掉这类损坏状态（internal/store/tx.go 的实现注释）。
	DropProviderSession(ctx context.Context, convID, agentID string) (existed bool, err error)

	// BeginTurn 插入一行 inflight_turns，记下「这一轮开始飞了」。
	// (conv_id, agent_id) 上有唯一索引，同一个 agent 在同一会话里
	// 不能有两行同时在飞；冲突直接报错，不 upsert —— 同时两行在飞
	// 是 bug（同一个 agent 被并发起了两次轮），不是可以覆盖的重试。
	// (conv_id, agent_id) 冲突时返回的错误链里带 ErrInflightConflict，
	// 同时保留底层驱动错误。
	BeginTurn(ctx context.Context, t InflightTurn) error

	// SetInflightPhase 把 turn_id 对应的那一行 inflight_turns 的 phase
	// 改成 phase。turn_id 不存在时报错，不静默通过：phase 写入发生在
	// 轮内，此时记录必然存在（BeginTurn 已经先插过），查不到说明状态
	// 已经丢了，这正是要暴露的问题，不能悄悄放过去。
	SetInflightPhase(ctx context.Context, turnID string, phase InflightPhase) error

	// ClearInflight 删掉 turn_id 对应的那一行 inflight_turns，标志这一轮
	// 已经有了最终结果（无论成功失败），不用再算作「在飞」。existed 报告
	// 删之前这一行在不在，用于 RecordTurn 判断「调用方以为有在飞记录、
	// 库里却没有」这种不一致。
	ClearInflight(ctx context.Context, turnID string) (existed bool, err error)
}

// 哨兵错误。调用方一律用 errors.Is 判断，实现必须用 %w 包装它们，
// 不要让调用方去匹配错误字符串。
var (
	ErrConversationNotFound = errors.New("conversation not found")
	ErrParticipantNotFound  = errors.New("participant not found")
	ErrInvalidKind          = errors.New("invalid message kind")
	// ErrInvalidSender 表示 sender 与它携带的字段自相矛盾。
	// 目前唯一的情形：ReservedSenderUser 带上了 SenderProvider / SenderModel / SenderEffort。
	// 人不是模型，没有脑子可记；真出现说明上层把 agent 的回复当成用户发言写进来了。
	ErrInvalidSender = errors.New("invalid message sender")
	// ErrMissingSenderBrain 是 08a「给 Step 6 的硬性要求（MUST）」的落点：
	// KindAgent 消息必须带 SenderProvider 与 SenderModel（SenderEffort 仍可空 ——
	// 有的 provider 没有 effort 这个概念）。
	//
	// 它要挡的是 08a 点名的那个洞：Step 5 的两层校验拦的都是 sender_id，
	// 反方向一条都没有，适配器漏填三列照样落库；而渲染侧把三列全空渲染成占位符，
	// 占位符的定义又是「不适用」—— 于是漏填落在一个代码自己认定为合法的显示上，
	// 不报错、测试全绿，而 Canonical Conversation 只追加，归属此后无法回填。
	//
	// 这是按 Kind 立的第一条规矩，与按 SenderID 那条并存（原未决 #14 的前一半）。
	ErrMissingSenderBrain = errors.New("agent message carries no provider or model")
	ErrNoCursor           = errors.New("cursor not found")
	ErrTurnSeqMismatch    = errors.New("turn usage seq does not match the message")
	ErrMissingTurnID      = errors.New("turn usage carries no turn id")
	// ErrTurnIdentityMismatch: RecordTurn 的三个入参各自带着一份 (会话, agent)，
	// 而它们对不上。不比对的话四张表各自都会写成合法行，只是记在了不同的人名下，
	// 外键拦不住，事后只能靠人眼比对才看得出来。
	ErrTurnIdentityMismatch = errors.New("turn record parts disagree on conversation or agent")
	// ErrInflightMissing 表示调用方按「这一行 inflight_turns 应该存在」的
	// 假设去操作，但库里查不到。两处会触发：
	// (a) Tx.SetInflightPhase 更新一个不存在的 turn_id；
	// (b) Service.RecordTurn 收到 expectInflight=true，但 ClearInflight
	//     报告 existed=false。
	// 两处共用同一个哨兵，因为语义是同一件事 ——「这一轮本该留着一条在飞
	// 记录，但它不在了」，不必为几乎同一个情形另造第二个名字。
	// 不能静默处理：这代表状态被破坏（进程崩溃前没清、或代码某处漏写了），
	// 悄悄放过会让下一次恢复扫描看到一个本不该存在的空洞。
	ErrInflightMissing = errors.New("inflight turn record is missing")
	// ErrInflightConflict 表示同一 (会话, 成员) 已有一条不属于本轮的在飞记录。
	// 两个产出方共用这一个哨兵：Tx.BeginTurn 撞 (conv_id, agent_id) 唯一索引，
	// 以及 turn 包起进程之前的检查。所以屏幕上是同一句话。
	// turn_id 主键冲突不是这个哨兵。
	ErrInflightConflict = errors.New("an unsettled inflight turn already exists for this conversation and agent")
	// ErrEmptyTeamID 表示 BindTeam / SetConversationTeam 收到了空串。
	// 空串在 conversations.team_id 上读作「没绑」，不是一个 team。
	ErrEmptyTeamID = errors.New("empty team id")
)
