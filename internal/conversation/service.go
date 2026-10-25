package conversation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Service 在 Store 之上提供领域动作：建会话、动态 participant、发消息、
// 投递对象解析、读历史。它不认识 SQL，也不认识配置文件，只调 Store 接口。
// 零值不可用，必须经 NewService 构造。
type Service struct {
	store Store

	// dropThought 控制 Post / RecordTurn 是否把消息的 Thought 字段落成空串
	// （未决 #9(a)）。零值 false 必须读作「不丢」，也就是默认存 thought——
	// 这个字段故意反着写：全仓三十多处 NewService(...) 调用点（几乎全是
	// 测试）从不调 SetThoughtPolicy，如果字段正着写成 storeThought bool，
	// 它们会在零值 false 下静默开始丢 thought，往每条消息的 Meta 里塞
	// thought_dropped，而这与设计默认值（Config.StoreThought 默认 true）
	// 正好相反，且没有任何报错——与 schema.go 里那条裸 bool 默认值的坑是
	// 同一种病，只是长在 Service 这一层。dropThought 反过来：零值 false
	// 天然就是「不丢」，与默认值同向，不需要在这一层再对付一次。
	dropThought bool
}

// NewService 用给定的 Store 构造 Service。
func NewService(s Store) *Service {
	return &Service{store: s}
}

// SetThoughtPolicy 配置 Post / RecordTurn / SettleLostTurn 此后是否把非空 Thought 落成空串
// （未决 #9(a)）。store 为 false 时丢弃：Thought 列仍照写空串（不动 schema、
// 不迁移），并在那条消息的 Meta 写 MetaThoughtDropped = true——但只在这条
// 消息真的带着非空 thought 时才写这个键，理由见 MetaThoughtDropped 的注释。
//
// 只由 internal/app 在装配阶段按 cfg.StoreThought 调一次；仿
// internal/turn.Runner.SetRules 的形状——构造之后再配置，而不是塞进
// NewService 的参数列表，因为 08a_STEP5_API_CONTRACT.md:159 已经冻结了
// NewService(s Store) *Service 这个签名。本方法是契约面的新增，不是零契约
// 改动，已在 08a 补一条订正登记；Post / RecordTurn 的签名没变，但行为随
// 这个开关变了，08a 同一条订正里一并说明。
//
// 不加锁：与 Runner.SetRules 同样的道理——今天只有 REPL 那一条 goroutine
// 会在启动时调它一次，之后不再变。哪天真有并发调用者，要加的锁不止这一处。
//
// 只影响此后调用 Post / RecordTurn / SettleLostTurn 时写入的行：不回溯，已经落库的 thought
// 不动（事实源只追加，05 §6 的保留期清理是另一件事，不归本方法）。
func (s *Service) SetThoughtPolicy(store bool) {
	s.dropThought = !store
}

// applyThoughtPolicy 按 s.dropThought 决定要不要清空 m.Thought，是一个无
// 副作用的纯函数：返回处理后的副本，不修改调用方持有的 m 或它的 Meta。
//
// 之所以必须返回副本而不是就地改：Post 的入参 m Message 虽然是值类型，
// 但 m.Meta 是引用类型的 map；RecordTurn 的入参是 *Message，若直接改
// *msg，会改到调用方（internal/turn.Runner.Ask）自己持有的那份消息结构，
// 而那个 goroutine 在这次调用之后可能还要读它。往 Meta 里加键之前必须先
// 克隆一份，否则会连调用方原本已有的 Meta 内容一起被这次调用悄悄污染。
//
// 只在「策略要求丢」且「这条消息确实带着非空 thought」两者都成立时才动
// 手；否则原样返回 m，包括 Meta 为 nil 的情况——不为了"预留位置"而把一个
// 本该是 nil 的 Meta 换成空 map。
func (s *Service) applyThoughtPolicy(m Message) Message {
	if !s.dropThought || m.Thought == "" {
		return m
	}
	m.Thought = ""
	meta := make(map[string]any, len(m.Meta)+1)
	for k, v := range m.Meta {
		meta[k] = v
	}
	meta[MetaThoughtDropped] = true
	m.Meta = meta
	return m
}

// Create 新建一个会话。返回的 Conversation 是补全后的，ID 与 CreatedAt 由 store 填。
// Create 的签名在 08a 契约里冻结，所以带目录的创建是另开的 CreateIn。
func (s *Service) Create(ctx context.Context, title string) (Conversation, error) {
	return s.CreateIn(ctx, title, "")
}

// CreateIn 新建一个会话，并在 cwd 非空时把项目目录盖成 cwd。
// 建会话和盖章必须在同一个事务里，否则中间崩一下会留下一个没盖章的会话。
// cwd 为空时不调 SetConversationCwd，WriteTx 次数与 CreateConversation 调用与只建会话时一样。
func (s *Service) CreateIn(ctx context.Context, title, cwd string) (Conversation, error) {
	var created Conversation
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		c, err := tx.CreateConversation(ctx, title)
		if err != nil {
			return err
		}
		if cwd != "" {
			if err := tx.SetConversationCwd(ctx, c.ID, cwd); err != nil {
				return err
			}
		}
		c.Cwd = cwd
		created = c
		return nil
	})
	if err != nil {
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	return created, nil
}

// Join 把 p 加进 convID 这个会话。p.ConvID 为空时用参数 convID 补上；
// 已退出过的同一个 agent 再次加入是重新激活，不是插第二行，这由 store 保证
// （docs/08_STEP5_SPEC.md G 节第 1 条）。
func (s *Service) Join(ctx context.Context, convID string, p Participant) error {
	// 参数 convID 是权威：它是方法的显式参数，调用方把它当作准。
	// 无条件覆盖 p.ConvID，避免「传了 B 却加进 A」这类静默错位（08a 原本只说「为空时补」，
	// 没规定不一致怎么办，这里定为参数优先）。
	p.ConvID = convID
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		return tx.AddParticipant(ctx, p)
	})
	if err != nil {
		return fmt.Errorf("join participant %q: %w", p.AgentID, err)
	}
	return nil
}

// Leave 让 agentID 退出 convID 这个会话。退出是软删除，永不删消息：
// 退出后它说过的话仍读得到（docs/05_MESSAGE_PROTOCOL.md 第 5 节）。
func (s *Service) Leave(ctx context.Context, convID, agentID string, at time.Time) error {
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		return tx.DeactivateParticipant(ctx, convID, agentID, at)
	})
	if err != nil {
		return fmt.Errorf("leave participant %q: %w", agentID, err)
	}
	return nil
}

// BindTeam 把会话绑到 teamID。Create 的签名不动(08a 冻结)；
// 已经开着的会话中途绑 team 走这里。改绑只影响此后的 delta，
// 不改已投递的前缀。
//
// teamID 为空串时拒绝，不解绑。空串在 conversations.team_id 上
// 读作「没绑」，不是一个 team；用它当解绑会让「从未绑过」和
// 「绑过又解开」在库里长得一样，而面包屑事件名契约只给了
// team_bound，解绑没有对应的事件可写。绑定改变了此后所有轮次
// 的可见性规则，事实源里必须看得出来它是什么时候变的。
// 解绑若要做，应另开方法和另开事件，不塞进本方法。
//
// SetConversationTeam 与这条 team_bound 面包屑必须在同一个
// 写事务里：分两个事务的话，中间崩一下就会留下「team 已绑但
// 没有任何记录说它何时绑的」。不能走 Post：Post 自己开
// WriteTx，套进去就不是同一个事务。
func (s *Service) BindTeam(ctx context.Context, convID, teamID string) error {
	if teamID == "" {
		return fmt.Errorf("bind team: %w", ErrEmptyTeamID)
	}
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.SetConversationTeam(ctx, convID, teamID); err != nil {
			return err
		}
		_, err := tx.AppendMessage(ctx, Message{
			ConvID:   convID,
			SenderID: ReservedSenderSystem,
			Kind:     KindSystem,
			Text:     fmt.Sprintf("会话绑定 team %s", teamID),
			Meta: map[string]any{
				MetaEvent:  EventTeamBound,
				MetaTeamID: teamID,
			},
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("bind team %q: %w", teamID, err)
	}
	return nil
}

// Post 追加一条消息，返回补全后的 Message（ID、Seq、CreatedAt 由 store 填）。
//
// 校验分两处，这是有意的：
//
// **事务外**（便宜、不需要原子性，失败时连写事务都不必开）：会话存在；Kind 合法；
// SenderID 非空；**保留 sender**（user 与 system）不得携带脑子三列；
// **Kind 为 KindAgent 时必须带 SenderProvider 与 SenderModel**（Effort 可空）。
// 会话存在这一条留在外面是为了给出比事务内失败更友好的错误；v7 起会话可以
// 被软删，这个检查与写事务之间确实多了一个窗口。事务内的 requireConversation
// 是最终裁量，它不按 deleted_at 过滤 —— 软删只改可见性，已经开始的一轮仍要
// 能把自己的结果落库，不能因为一次删除丢掉一条已经付费的回复。
//
// **事务内**（与追加消息同一个事务）：SenderID 不是 ReservedSenderUser 时，
// 它必须是该会话当前的活跃 participant。这一条必须在事务内，理由见
// Tx.RequireActiveParticipant 的注释（未决 #13）。
//
// 任何一条不过都一律不落库。
func (s *Service) Post(ctx context.Context, m Message) (Message, error) {
	// 存储开关（未决 #9(a)）在这里应用一次：m 是值类型的局部参数，改它不影响
	// 调用方。放在最前面，早于下面的校验——Thought 与那些校验（Kind、
	// SenderID、脑子三列）互不相干，谁先谁后都不改变校验结果，放前面只是
	// 让「落库的东西是什么形状」在函数一开始就定下来，不必在校验之间穿插。
	m = s.applyThoughtPolicy(m)
	if _, err := s.store.Conversation(ctx, m.ConvID); err != nil {
		return Message{}, fmt.Errorf("post message: %w", err)
	}
	if !m.Kind.Valid() {
		return Message{}, fmt.Errorf("post message: kind %q: %w", m.Kind, ErrInvalidKind)
	}
	if m.SenderID == "" {
		return Message{}, fmt.Errorf("post message: empty sender id: %w", ErrParticipantNotFound)
	}
	if isReservedSender(m.SenderID) && !m.SenderBrainEmpty() {
		// 两个保留 sender 都没有脑子可记：人不是模型，Council 自己的记账也不是模型。
		// 这三个字段是「发言当时用的 provider/model/effort」，对它们毫无意义；
		// 真出现说明上层把某个 agent 的回复当成用户发言或系统事件写进来了。
		// 响亮地失败，而不是静默存下一条自相矛盾的记录。
		//
		// messages 表上那条 CHECK 只约束 'user'，管不到 'system' ——
		// 所以 system 这一半**只有这一层**，不像 user 那样有结构兜底。
		return Message{}, fmt.Errorf(
			"post message: sender %q carries provider/model/effort: %w", m.SenderID, ErrInvalidSender)
	}
	// 按 Kind 立的第一条规矩（原未决 #14 的前一半，08a 的 MUST）：
	// agent 的发言必须带得出是哪个脑子说的。Effort 仍可空 —— 有的 provider 没有这个概念，
	// 而 provider 与 model 是三家都一定有的。
	//
	// 不加这条的话，Step 6 的适配器漏填三列会静默落库、渲染成「不适用」的占位符、
	// 测试全绿，而用户要的那件事当场落空且事后无法回填。
	if m.Kind == KindAgent && (m.SenderProvider == "" || m.SenderModel == "") {
		return Message{}, fmt.Errorf(
			"post message: agent sender %q: provider=%q model=%q: %w",
			m.SenderID, m.SenderProvider, m.SenderModel, ErrMissingSenderBrain)
	}

	// Recipients 非空时在**事务外**校验存在性，直接复用已经存在的
	// Service.Recipients，零契约新增。
	//
	// 不要照未决 #13 的先例把它搬进事务。#13 拦的是「活跃」，那个属性
	// 会被 /leave 改变，读到「活跃」之后、提交之前用户可能先行退出，
	// 所以有 TOCTOU 窗口。这里校验的是「存在」，不是「活跃」，而 Leave
	// 只置 active=false 并记 left_at，participants 行从不删除——存在性
	// 单调不减，这个口径下窗口不存在。搬进事务要解冻 Tx 接口，换来零表达力。
	if len(m.Recipients) > 0 {
		if _, err := s.Recipients(ctx, m); err != nil {
			return Message{}, fmt.Errorf("post message: %w", err)
		}
	}

	var stored Message
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		// 活跃成员判定在**事务内**做，与追加消息同一个事务。
		//
		// 原先是在事务外用读句柄查一次再另起写事务，中间有窗口：并发发言时
		// 读到「活跃」之后、提交之前，用户 /leave 可能先行提交，这条消息照样落库，
		// 事后 /who 显示它已退出、left_at 还早于这条消息的 created_at，
		// 而没有任何一层会报错。Step 6 起真有并发，所以现在挪进来（未决 #13）。
		//
		// 两个保留 sender 都不是 participant，不需要 join 就能说话，所以跳过。
		// system 这一条尤其要紧：会话重建、一轮失败这类事件，恰恰常常发生在
		// 刚 /leave 或正被重配的成员身上；若让它去过活跃成员校验，
		// 08a 明令必须留下的那条记录就一条都留不下。
		if !isReservedSender(m.SenderID) {
			if err := tx.RequireActiveParticipant(ctx, m.ConvID, m.SenderID); err != nil {
				return err
			}
		}
		out, err := tx.AppendMessage(ctx, m)
		if err != nil {
			return err
		}
		stored = out
		return nil
	})
	if err != nil {
		return Message{}, fmt.Errorf("post message: %w", err)
	}
	return stored, nil
}

// RecordTurn 把一轮的全部结果写进库：追加消息（若有）、记用量、更新会话状态，
// 四件事同一个 WriteTx，任一步失败整轮回滚。
//
// 这是本步唯一允许写 turn_usage 与 provider_sessions 的入口。
// 不要在别处直接开 WriteTx 调那两个 Tx 方法 —— 那样「五样同生共死」
// 就退化成一条靠人遵守的约定。本步也不再往 cursors 表写 agent 的会话游标，
// 游标已经搬进 provider_sessions.cursor_seq；两处都写就有了两个事实源。
//
// msg 为 nil 表示这一轮没有产生发言（失败轮），此时 TurnUsage.Seq 必须是 nil。
//
// session 为 nil 表示这一轮不碰 provider_sessions 这一行——不写、也不删。
//
// 订正（2026-09-21，未决 #20）：这一条原文是「session 为 nil 表示这一轮
// 之后会话作废，此时内部走 DropProviderSession」。那个语义是错的：调用方
// （runner.go）在起进程之前就拒绝的那几支——agent 不存在、provider 没适配器、
// 权限档不放行、WorkDir 为空、指纹探针出错——传下去的 session 本来就是 nil，
// 与「这一轮决定作废会话」毫无关系，只是「这一轮压根没碰过会话状态」。旧
// 代码把这两件事共用同一个 nil，于是这些早期拒绝会把一条老早就建好、这一轮
// 根本没读过的 provider 会话静默删掉：只改一个不相关的配置字段（例如把
// permissions 从 read 改成 workspace）就会把已经花过钱的会话抹掉，改回来要
// 重付一次入场费，而且没有任何一层会说（实测坐实，见 00_DECISIONS.md #20）。
//
// 现在 nil 只表示「不碰」。真正的「作废」改成调用方在做出那个决定的那一刻
// 显式调 Service.DropProviderSession（例如 runner.go 里从库读回、指纹不符
// 那一支）——那一刻恰好还持有旧指纹，能点名到底哪一项变了，与 #18 加
// session_rebuilt 面包屑是同一个时机，两件事现在放在一起做。ErrIdentityDrift
// 那一支（进程内已有记录指纹不符）不受影响：它传的 session 带着非空
// ProviderSessionID，走的是 SetProviderSession 原值回写，从未经过 nil 分支。
//
// 调用方义务：msg 非 nil 且 AppendMessage 失败时（含事务内
// RequireActiveParticipant 拒掉，即 OutcomePostRejected 那一支），
// 整个事务回滚，库里不会留下 turn_usage 行。钱已经花了，账仍要记 ——
// 调用方必须改用 msg=nil 再调一次 RecordTurn 把用量写上。
// 失败轮也要记账，与 Post 失败会回滚整个事务，这两件事在同一个入口里
// 对不上；不在这里偷偷拆成「消息回滚、用量提交」，那会让「任一步失败
// 整轮回滚」变成有例外的约定。
//
// 返回落库后的消息（msg 为 nil 时返回 nil）。
// expectInflight 是调用方控制标志，不写入 TurnUsage、不落库。同一笔 WriteTx
// 在追加消息、记用量、更新 provider 会话之后再 ClearInflight(u.TurnID)。
// 为 true 而这一行不存在时返回 ErrInflightMissing，整笔回滚，避免在
// 「调用方断言有在飞记录、库里却没有」时静默提交。为 false 时删不到也照常提交。
func (s *Service) RecordTurn(ctx context.Context, msg *Message, u TurnUsage, session *ProviderSession, expectInflight bool) (*Message, error) {
	if u.TurnID == "" {
		return nil, fmt.Errorf("record turn: %w", ErrMissingTurnID)
	}
	if (msg == nil) != (u.Seq == nil) {
		return nil, fmt.Errorf("record turn: %w", ErrTurnSeqMismatch)
	}

	// 存储开关（未决 #9(a)）在这里应用一次，与 Post 呼应——那两处的校验
	// 本来就逐条重复，这条也不例外。只重新指向一份处理后的副本，不改调用方
	// （internal/turn.Runner.Ask）手里那份 *Message：applyThoughtPolicy 传值
	// 进去、传值出来，msg 这个局部变量此后指向的是副本，调用方自己持有的
	// 那个 Message 结构体原样未动。下面所有引用 msg 的地方（校验、
	// AppendMessage、比对）就自动用上了处理后的版本，不需要再改一处。
	if msg != nil {
		redacted := s.applyThoughtPolicy(*msg)
		msg = &redacted
	}

	if msg != nil {
		// 事务外的校验必须与 Post 逐条相同，一条都不能少。
		// Step 6 的真机冒烟证明 /leave 之后那一轮真的会走 RequireActiveParticipant
		// 被拒，而被拒正是 OutcomePostRejected 这个结局存在的理由；
		// 若这里为了少写几行绕开，那条路径对 RecordTurn 就失效了。
		if _, err := s.store.Conversation(ctx, msg.ConvID); err != nil {
			return nil, fmt.Errorf("record turn: %w", err)
		}
		if !msg.Kind.Valid() {
			return nil, fmt.Errorf("record turn: kind %q: %w", msg.Kind, ErrInvalidKind)
		}
		if msg.SenderID == "" {
			return nil, fmt.Errorf("record turn: empty sender id: %w", ErrParticipantNotFound)
		}
		if isReservedSender(msg.SenderID) && !msg.SenderBrainEmpty() {
			return nil, fmt.Errorf(
				"record turn: sender %q carries provider/model/effort: %w", msg.SenderID, ErrInvalidSender)
		}
		if msg.Kind == KindAgent && (msg.SenderProvider == "" || msg.SenderModel == "") {
			return nil, fmt.Errorf(
				"record turn: agent sender %q: provider=%q model=%q: %w",
				msg.SenderID, msg.SenderProvider, msg.SenderModel, ErrMissingSenderBrain)
		}
	}

	// 三个入参各自带着一份 (会话, agent)，这里比对一致，否则本入口就白设了。
	//
	// msg 用 ConvID + SenderID，u 用 ConvID + AgentID，session 用 ConvID + AgentID ——
	// 三处互不相干。调用方一处笔误（比如把上一轮的 agent 留在 u.AgentID 里），
	// 消息会记在甲名下、用量与会话状态记在乙名下：四张表各自都合法，
	// 外键也拦不住，事后只能靠人眼比对才看得出来。
	// RecordTurn 存在的理由就是让「五样同生共死」由结构保证而不是靠人记住，
	// 少了这道比对它就退回成一条约定。（2026-09-21 由测试作者指出。）
	//
	// 两处细节：
	// 一是放在上面那组「与 Post 逐条相同」的校验之后，让更具体的错误先出 ——
	// SenderID 为空该报 ErrParticipantNotFound，不该被这条笼统的比对盖住。
	// 二是 SenderID 只在**非保留 sender** 时才与 u.AgentID 比：保留 sender
	// （user / system）不是 participant，一条 system 面包屑的 SenderID 本来就
	// 不等于这一轮的 agent，拿它去比是把两种身份混成一种。ConvID 无此例外。
	if msg != nil {
		if msg.ConvID != u.ConvID {
			return nil, fmt.Errorf(
				"record turn: message conv %q but usage conv %q: %w",
				msg.ConvID, u.ConvID, ErrTurnIdentityMismatch)
		}
		if !isReservedSender(msg.SenderID) && msg.SenderID != u.AgentID {
			return nil, fmt.Errorf(
				"record turn: message sender %q but usage agent %q: %w",
				msg.SenderID, u.AgentID, ErrTurnIdentityMismatch)
		}
	}
	if session != nil && (session.ConvID != u.ConvID || session.AgentID != u.AgentID) {
		return nil, fmt.Errorf(
			"record turn: session is (%s, %s) but usage is (%s, %s): %w",
			session.ConvID, session.AgentID, u.ConvID, u.AgentID, ErrTurnIdentityMismatch)
	}

	var stored Message
	var hasMsg bool
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		if msg != nil {
			if !isReservedSender(msg.SenderID) {
				if err := tx.RequireActiveParticipant(ctx, msg.ConvID, msg.SenderID); err != nil {
					return err
				}
			}
			out, err := tx.AppendMessage(ctx, *msg)
			if err != nil {
				return err
			}
			stored = out
			hasMsg = true
			// AppendMessage 忽略调用方传入的 Seq，落库序号以 store 分配的为准。
			// 前面只校验了指针是否存在（失败轮必须是 nil），这里改成指向真正的消息，
			// 否则 usage.seq 会写成调用方猜的那个数，成功轮对不上它所产生的那条消息。
			seq := stored.Seq
			u.Seq = &seq
		}
		if err := tx.RecordUsage(ctx, u); err != nil {
			return err
		}
		// 未决 #20 裁决：session 为 nil 表示「这一轮不碰这一行」，不再是
		// 「作废，删行」——真正的作废现在由调用方在决定的那一刻显式调
		// Service.DropProviderSession（不经过这个入口）。这里因此只在
		// session 非 nil 时才写；nil 时什么都不做，让调用方早于本轮就
		// 存在、本轮根本没碰过的那一行原样留着。
		if session != nil {
			if err := tx.SetProviderSession(ctx, *session); err != nil {
				return err
			}
		}
		// ClearInflight 依据的是这一轮已经决定的最终结果，所以放在
		// AppendMessage、RecordUsage、SetProviderSession 之后。前三步任一
		// 出错就提前 return，这一步不会执行，整笔事务（含 inflight 行）一起
		// 回滚。提前删会留下「消息与用量没写成、飞行记录却已消失」的半途状态。
		existed, err := tx.ClearInflight(ctx, u.TurnID)
		if err != nil {
			return err
		}
		if expectInflight && !existed {
			return fmt.Errorf("record turn: turn %s: %w", u.TurnID, ErrInflightMissing)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("record turn: %w", err)
	}
	if !hasMsg {
		return nil, nil
	}
	return &stored, nil
}

// Recipients 解析一条消息的投递对象（docs/08_STEP5_SPEC.md G 节第 4、5 条）。
//
// m.Recipients 非空即定向投递：逐个校验收件人确实是该会话的 participant，
// 有一个不是就返回包了 ErrParticipantNotFound 的错误（不静默丢弃），
// 全都在就原样返回。
//
// m.Recipients 为空即广播：返回当前活跃 participant 的 id，按字典序排好。
// 注意「当前」是读的时候算的，不是写的时候固化的 —— 消息行里存的仍是空数组。
func (s *Service) Recipients(ctx context.Context, m Message) ([]string, error) {
	if len(m.Recipients) > 0 {
		// 含已退出的成员：定向发给一个退了的 agent 仍是合法的解析结果，
		// G 节第 5 条要拦的是「根本不存在的 agent」。
		all, err := s.store.Participants(ctx, m.ConvID, true)
		if err != nil {
			return nil, fmt.Errorf("resolve recipients: list participants: %w", err)
		}
		known := make(map[string]struct{}, len(all))
		for _, p := range all {
			known[p.AgentID] = struct{}{}
		}
		for _, id := range m.Recipients {
			if _, ok := known[id]; !ok {
				return nil, fmt.Errorf("resolve recipients: %q: %w", id, ErrParticipantNotFound)
			}
		}
		return m.Recipients, nil
	}

	active, err := s.store.Participants(ctx, m.ConvID, false)
	if err != nil {
		return nil, fmt.Errorf("resolve recipients: list active participants: %w", err)
	}
	ids := make([]string, 0, len(active))
	for _, p := range active {
		ids = append(ids, p.AgentID)
	}
	// 排序是为了顺序稳定：store 的返回顺序变了也不该让调用方与测试跟着变。
	sort.Strings(ids)
	return ids, nil
}

// History 按游标读历史：返回 seq 严格大于 afterSeq 的消息，按 seq 升序，最多 limit 条。
func (s *Service) History(ctx context.Context, convID string, afterSeq int64, limit int) ([]Message, error) {
	return s.store.Messages(ctx, convID, afterSeq, limit)
}

// 下面三个是读路径的转发。它们存在的理由是分层：
// 上层（REPL）只认 Service 这一个持久化入口，不该为了列个会话就自己去拿 Store。
// 08a 第一版把 Service 冻结成六个方法，漏了这三个，导致 REPL 编译不过 —— 那是契约的漏洞，
// 不是实现者的错，已补进契约。
//
// 这三个方法刻意不做「会话是否存在」的校验：读路径每次多查一次会话纯属浪费，
// 不存在的会话自然会读出空结果。要判存在与否，调用方自己去查。

// Conversations 列出全部未归档会话，顺序由 store 保证稳定（created_at 倒序）。
func (s *Service) Conversations(ctx context.Context) ([]Conversation, error) {
	return s.store.Conversations(ctx)
}

// Participants 列出一场会话的成员。includeInactive 决定是否含已退出的。
func (s *Service) Participants(ctx context.Context, convID string, includeInactive bool) ([]Participant, error) {
	return s.store.Participants(ctx, convID, includeInactive)
}

// MaxSeq 返回一场会话当前最大 seq。供「看最近 n 条」这类需要先定位起点的调用使用。
func (s *Service) MaxSeq(ctx context.Context, convID string) (int64, error) {
	return s.store.MaxSeq(ctx, convID)
}

// Conversation 按 id 取一场会话。不存在时返回包了 ErrConversationNotFound 的错误。
// 纯读、不开事务：与 Store.Conversation 同一条读路径，不经过 WriteTx。
func (s *Service) Conversation(ctx context.Context, id string) (Conversation, error) {
	return s.store.Conversation(ctx, id)
}

// ProviderSession 读回一条 provider 会话。第二个返回值为 false 表示
// 这个 agent 在这条会话里还没开过口（或上一次的状态已被作废）。
// 纯读、不开事务：与 Store.ProviderSession 同一条读路径，不经过 WriteTx。
func (s *Service) ProviderSession(ctx context.Context, convID, agentID string) (ProviderSession, bool, error) {
	return s.store.ProviderSession(ctx, convID, agentID)
}

// AllProviderSessions 只是直通。Service 存在的意义是把多表写入收进一个事务，
// 纯读没有那个需求，包一层只为让调用方不必拿到 Store 本身。
func (s *Service) AllProviderSessions(ctx context.Context) ([]ProviderSession, error) {
	return s.store.AllProviderSessions(ctx)
}

// AllInflight 只是直通，理由同 AllProviderSessions。
func (s *Service) AllInflight(ctx context.Context) ([]InflightTurn, error) {
	return s.store.AllInflight(ctx)
}

// InflightFor 只是直通，理由同 AllInflight。
func (s *Service) InflightFor(ctx context.Context, convID, agentID string) (InflightTurn, bool, error) {
	return s.store.InflightFor(ctx, convID, agentID)
}

// UsageSamples 只是直通。Service 存在的意义是把多表写入收进一个事务，
// 纯读没有那个需求，包一层只为让调用方不必拿到 Store 本身。
func (s *Service) UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]TurnUsage, error) {
	return s.store.UsageSamples(ctx, agentID, identityFP, limit)
}

// TurnUsage 只是直通，理由同 AllInflight。
func (s *Service) TurnUsage(ctx context.Context, convID, agentID string, limit int) ([]TurnUsage, error) {
	return s.store.TurnUsages(ctx, convID, agentID, limit)
}

// DropProviderSession 删掉一条 provider 会话，供 /session rebuild 用。
//
// existed 直接取 Tx.DropProviderSession 的返回值（它由 DELETE 自身的
// RowsAffected 决定），不再先读一次 ProviderSession 判存在。
//
// 这处改动修的是一个真实缺陷：先读一次要经 scanProviderSession 解析
// last_turn_at 等列，一行损坏的记录（例如 last_turn_at 存了 parseTime
// 解不出来的字符串）会让那次读先失败，进而让整条 DropProviderSession 失败 ——
// 而 /session rebuild 存在的意义恰恰是让用户能清掉这类损坏状态。改成让
// DELETE 自己报，既不解析任何列，也不需要额外一次查询，同一个损坏行现在
// 能被正常删掉。
//
// 不存在这一行不是错误：对一条本来就没有的会话敲 rebuild，existed=false
// 原样如实报告即可。
func (s *Service) DropProviderSession(ctx context.Context, convID, agentID string) (existed bool, err error) {
	err = s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		var txErr error
		existed, txErr = tx.DropProviderSession(ctx, convID, agentID)
		return txErr
	})
	if err != nil {
		return false, fmt.Errorf("drop provider session %s/%s: %w", convID, agentID, err)
	}
	return existed, nil
}

// SettleLostTurn 把一条「上次退出时还在飞」的记录补上账并删行。
//
// msg、u、session 三者都可为 nil，各自表示「这一件跳过」。三者全 nil
// 就是只删行：phase == begun（进程没起、没花钱）与会话已经不在（3.9）
// 两支共用这条路径，不为它们各加一个 bool。
//
// 与 RecordTurn 的实质差别只有一处：u.Seq 保持调用方传进来的值，
// 绝不改写成刚追加的那条消息的 seq。Lost 行的 seq 必须是 NULL。
// 这一轮没有产生 agent 消息；面包屑是 system 消息，不是「这一轮说的话」。
// RecordTurn 里那句 (msg == nil) != (u.Seq == nil) 以及事务内改写 u.Seq，
// 正是本方法不能复用它的原因。这里也不改 RecordTurn 去迁就。
//
// ClearInflight 报 existed == false 不是错误，填进 Existed 返回。
// RecordTurn 把它当结构性 bug（BeginTurn 被绕过了，expectInflight 为真
// 就回滚）。对账这一侧它只意味着「别人已经处理过了」。结构上不可能发生
// （两个调用方：启动时进 REPL 之前跑一次；/session rebuild 在持有那一
// (会话, 成员) 的闸时结清单条，同一 key 没有别的写入方），真发生了也该跳过不该报错。
func (s *Service) SettleLostTurn(ctx context.Context, turnID string,
	msg *Message, u *TurnUsage, session *ProviderSession) (LostSettlement, error) {
	if turnID == "" {
		return LostSettlement{}, fmt.Errorf("settle lost turn: %w", ErrMissingTurnID)
	}

	if msg != nil {
		// 本方法是第三条追加消息的路。今天正当载荷的 Thought 是空串，
		// 所以接不接看不出差别，而这正是它会被漏掉的原因。
		// 存储开关是用户在配置里表达的意愿，按入口逐个接是唯一能保证它不漏的做法。
		//
		// 只重新指向一份处理后的副本，不改调用方手里那份 *Message：
		// applyThoughtPolicy 传值进去、传值出来，msg 这个局部变量此后指向的是副本，
		// 调用方自己持有的那个 Message 结构体原样未动。下面的校验与 AppendMessage
		// 用的就是这份副本。
		redacted := s.applyThoughtPolicy(*msg)
		msg = &redacted
	}

	if msg != nil && msg.SenderID == "" {
		// AppendMessage 不查 sender。messages.sender_id 是 TEXT NOT NULL，
		// 空串过得了这一列。与 Post、RecordTurn 同一道，空 sender 在进事务之前拒绝。
		return LostSettlement{}, fmt.Errorf(
			"settle lost turn: empty sender id: %w", ErrParticipantNotFound)
	}
	if msg != nil && isReservedSender(msg.SenderID) && !msg.SenderBrainEmpty() {
		// 与 Post、RecordTurn 逐条相同的那道校验。SQL 拦不住：messages 上的
		// CHECK 只约束 sender_id = 'user'，system 带着脑子列照样落得进去。
		// 本方法是第三个会追加消息的公开入口，少这一道就等于给 system 面包屑
		// 开了一个后门，而「人不是模型，没有脑子可记」这条规矩是按 sender 立的。
		return LostSettlement{}, fmt.Errorf(
			"settle lost turn: sender %q carries provider/model/effort: %w", msg.SenderID, ErrInvalidSender)
	}

	// 三个入参各自带着一份 (会话, agent)，这里比对一致，否则本入口就白设了。
	// 与 RecordTurn 同一条理由：对不上时消息记在甲名下、用量与会话状态记在
	// 乙名下，四张表各自都合法，外键拦不住。
	//
	// 只比非 nil 的那几份。某一份为 nil 是「这一件跳过」，没有身份可对。
	// SenderID 不与 AgentID 比：面包屑的 sender 是保留的 system，本来就
	// 不等于这一轮的 agent。
	if msg != nil && u != nil && msg.ConvID != u.ConvID {
		return LostSettlement{}, fmt.Errorf(
			"settle lost turn: message conv %q but usage conv %q: %w",
			msg.ConvID, u.ConvID, ErrTurnIdentityMismatch)
	}
	if session != nil && u != nil && (session.ConvID != u.ConvID || session.AgentID != u.AgentID) {
		return LostSettlement{}, fmt.Errorf(
			"settle lost turn: session is (%s, %s) but usage is (%s, %s): %w",
			session.ConvID, session.AgentID, u.ConvID, u.AgentID, ErrTurnIdentityMismatch)
	}
	if session != nil && msg != nil && (session.ConvID != msg.ConvID) {
		return LostSettlement{}, fmt.Errorf(
			"settle lost turn: session conv %q but message conv %q: %w",
			session.ConvID, msg.ConvID, ErrTurnIdentityMismatch)
	}

	var existed bool
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		if msg != nil {
			if _, err := tx.AppendMessage(ctx, *msg); err != nil {
				return err
			}
		}
		// 不改 u.Seq。AppendMessage 分配的 seq 属于面包屑，不属于这一轮。
		if u != nil {
			if err := tx.RecordUsage(ctx, *u); err != nil {
				return err
			}
		}
		if session != nil {
			if err := tx.SetProviderSession(ctx, *session); err != nil {
				return err
			}
		}
		var err error
		existed, err = tx.ClearInflight(ctx, turnID)
		return err
	})
	if err != nil {
		// 会话不存在时，事务里第一个碰 conversations 的写操作就会返回
		// ErrConversationNotFound。AppendMessage、RecordUsage、SetProviderSession
		// 各自第一步都是 requireConversation，turn_usage.conv_id 上还有外键兜底。
		// 判定一律用 errors.Is，不依赖是哪一步报的：那个顺序会变，这个哨兵不会。
		// 第一笔已经整笔回滚，不会留半截状态。
		// 这是一次真实的账目损失，不许假装补上了：不写面包屑、不写 turn_usage，
		// 只再开一笔把在飞记录删掉，免得下一轮 BeginTurn 撞上唯一索引。
		if errors.Is(err, ErrConversationNotFound) {
			var cleared bool
			clearErr := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
				var txErr error
				cleared, txErr = tx.ClearInflight(ctx, turnID)
				return txErr
			})
			if clearErr != nil {
				return LostSettlement{}, fmt.Errorf("settle lost turn %s: clear after missing conversation: %w", turnID, clearErr)
			}
			return LostSettlement{ConvMissing: true, Existed: cleared}, nil
		}
		return LostSettlement{}, fmt.Errorf("settle lost turn %s: %w", turnID, err)
	}

	out := LostSettlement{Existed: existed}
	// SuspectMarked 只在调用方真传了一行、且这一笔成功提交时为真。
	// 传 nil 是「没有那一行可标」（bootstrap），不是「标过了」。
	if session != nil {
		out.SuspectMarked = true
	}
	return out, nil
}

// BeginTurn 单独开一笔写事务，插入一行 inflight_turns 然后立刻提交。
//
// 这一行的全部价值就是「进程在两个事务之间死掉时它还留得下」。若把它并进
// 后面 RecordTurn 那一笔，崩溃落在两者之间时库里仍然一行都没有，本方法
// 等于没写。所以它必须先于后面的事务单独提交。本地 SQLite 写是亚毫秒级的，
// 多一次提交不构成可以合并的理由。
//
// TurnID 为空、Phase 不在三态之内都在进事务之前拒绝。InflightPhase 的零值
// 是空串，不是 begun：SQL 的 DEFAULT 'begun' 只在插入语句省略这一列时生效，
// 到不了 Go 这一侧。漏填若拖过这一关，空串会被当成一条合法的在飞记录写下，
// 下次启动对账只能猜该重跑还是该当已经付过钱。
//
// 不做「这一行在不在」的预读。插入冲突由 Tx.BeginTurn 自己报：同一
// (conv_id, agent_id) 上已有一行在飞是调用方的 bug，这里再查一次只会把
// 「冲突」说成「已经在」，把该暴露的状态藏起来。
func (s *Service) BeginTurn(ctx context.Context, t InflightTurn) error {
	if t.TurnID == "" {
		return fmt.Errorf("begin turn: %w", ErrMissingTurnID)
	}
	if !t.Phase.Valid() {
		return fmt.Errorf("begin turn: phase %q is not a valid inflight phase", t.Phase)
	}
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		return tx.BeginTurn(ctx, t)
	})
	if err != nil {
		return fmt.Errorf("begin turn %s: %w", t.TurnID, err)
	}
	return nil
}

// SetInflightPhase 单独开一笔写事务，只改这一行的 phase，然后立刻提交。
//
// 与 BeginTurn 同一条理由：phase 本身就是「死在下一步之前，库里还留得下
// 走到了哪一档」的标记。并进别的事务，进程死在两个落点之间时读到的仍是
// 上一档，标记白写。本地 SQLite 写是亚毫秒级的。
//
// turnID 为空、phase 不在三态之内都在进事务之前拒绝，理由同 BeginTurn：
// 零值空串不是 begun，SQL 的 DEFAULT 到不了这一侧。
//
// 不做「这一行在不在」的预读。Tx.SetInflightPhase 影响 0 行时自己返回
// ErrInflightMissing；先 SELECT 一次只会多一次查询，并把「行不在」说成
// 本方法自己的判断，而哨兵已经由那一层定义好了。
func (s *Service) SetInflightPhase(ctx context.Context, turnID string, phase InflightPhase) error {
	if turnID == "" {
		return fmt.Errorf("set inflight phase: %w", ErrMissingTurnID)
	}
	if !phase.Valid() {
		return fmt.Errorf("set inflight phase: phase %q is not a valid inflight phase", phase)
	}
	err := s.store.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		return tx.SetInflightPhase(ctx, turnID, phase)
	})
	if err != nil {
		return fmt.Errorf("set inflight phase %s: %w", turnID, err)
	}
	return nil
}

// isReservedSender 报告 id 是不是 Council 的保留 sender。
//
// 保留 sender 不是 participant：不需要 join、不出现在成员列表里、不带脑子、
// 也不过活跃成员校验。目前有两个：ReservedSenderUser（人）与
// ReservedSenderSystem（Council 自己的记账）。
//
// 集中成一个函数而不是到处写 `id == A || id == B`：加第三个保留 id 时
// 只改这里；散着写一定会漏掉某一处，而漏掉的那处表现为「这条记录写不进去」，
// 只在特定时序下暴露。
func isReservedSender(id string) bool {
	return id == ReservedSenderUser || id == ReservedSenderSystem
}
