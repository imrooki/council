package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"council/internal/conversation"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Tx 是一次写事务上可调用的全部动作。
type Tx struct {
	tx *sql.Tx
}

var _ conversation.Tx = (*Tx)(nil)

// WriteTx 把 fn 里的所有写操作放进同一个 BEGIN IMMEDIATE 事务。
// fn 返回非 nil 就整体回滚并把该错误原样返回。
// fn 发生 panic 时也会回滚，然后继续抛出 panic。
func (s *Store) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	sqlTx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = sqlTx.Rollback()
		}
		if p := recover(); p != nil {
			panic(p)
		}
	}()

	if err := fn(ctx, &Tx{tx: sqlTx}); err != nil {
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("commit write tx: %w", err)
	}
	committed = true
	return nil
}

// CreateConversation 新建一场会话。ID 与 CreatedAt 由 store 生成。
func (t *Tx) CreateConversation(ctx context.Context, title string) (conversation.Conversation, error) {
	id, err := newID()
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	createdAt := formatTime(time.Now().UTC())
	parsed, err := parseTime(createdAt)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	if _, err := t.tx.ExecContext(ctx,
		`INSERT INTO conversations (id, title, created_at) VALUES (?, ?, ?)`,
		id, title, createdAt,
	); err != nil {
		return conversation.Conversation{}, fmt.Errorf("insert conversation: %w", err)
	}
	return conversation.Conversation{
		ID:        id,
		Title:     title,
		CreatedAt: parsed,
	}, nil
}

// SetConversationTeam 把会话绑到 teamID。空串拒绝，不是解绑。
// 空串在 conversations.team_id 上读作「没绑」，不是一个 team；
// 用它当解绑会让「从未绑过」和「绑过又解开」在库里长得一样，
// 而面包屑事件名只给了 team_bound，解绑没有对应的事件可写，
// 事实源里会留下一次无记录的可见性规则变化。解绑若要做，
// 应另开方法和另开事件，不塞进本方法。
func (t *Tx) SetConversationTeam(ctx context.Context, convID, teamID string) error {
	if teamID == "" {
		return fmt.Errorf("set conversation team: %w", conversation.ErrEmptyTeamID)
	}
	if err := t.requireConversation(ctx, convID); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx,
		`UPDATE conversations SET team_id = ? WHERE id = ?`,
		teamID, convID,
	); err != nil {
		return fmt.Errorf("set conversation team %s: %w", convID, err)
	}
	return nil
}

// SetConversationCwd 把会话的项目目录设成 cwd。
// 空串允许：空串在 cwd 列上就是「没绑」这一个含义，清空是合法动作，
// 没有需要保护的第二种含义。与 SetConversationTeam 拒绝空串不同。
func (t *Tx) SetConversationCwd(ctx context.Context, convID, cwd string) error {
	if err := t.requireConversation(ctx, convID); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx,
		`UPDATE conversations SET cwd = ? WHERE id = ?`,
		cwd, convID,
	); err != nil {
		return fmt.Errorf("set conversation cwd %s: %w", convID, err)
	}
	return nil
}

// renameTx 是 RenameConversation 在 WriteTx 里需要的最小事务能力。
//
// 这条方法刻意不放进 conversation.Tx：改标题目前只有 serve 的
// POST /api/conversations/{id}/rename 一个调用方，而扩接口会强迫
// internal/turn 与 internal/repl 里的每个假 Store+Tx 实现都补一个它们
// 永远不会调用的方法。用包内断言把「只有本包认识它」这件事写在类型上，
// 比让领域接口背一个持久化细节更便宜。
type renameTx interface {
	renameConversation(ctx context.Context, convID, title string) error
}

// RenameConversation 改一场会话的标题，走写事务。只改 title 一列：
// created_at、team_id、cwd 与消息都不动，标题不是一次会话的重新定义。
//
// 会话不存在时返回包了 conversation.ErrConversationNotFound 的错误，
// 让 HTTP 层能把 404 与「库坏了」分开。存在性检查与 UPDATE 在同一个
// 事务里（同一个写句柄上的 BEGIN IMMEDIATE），中间没有可乘之窗。
func (s *Store) RenameConversation(ctx context.Context, convID, title string) error {
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(renameTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持重命名", tx)
		}
		return impl.renameConversation(ctx, convID, title)
	})
	if err != nil {
		return fmt.Errorf("rename conversation %s: %w", convID, err)
	}
	return nil
}

// renameConversation 是 RenameConversation 的事务内动作，见 renameTx 的注释。
func (t *Tx) renameConversation(ctx context.Context, convID, title string) error {
	if err := t.requireConversation(ctx, convID); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx,
		`UPDATE conversations SET title = ? WHERE id = ?`,
		title, convID,
	); err != nil {
		return fmt.Errorf("update conversation title %s: %w", convID, err)
	}
	return nil
}

// ---- 软删除会话 ----

// softDeleteTx 是 SoftDeleteConversation 在 WriteTx 里需要的最小事务能力。
// 与 renameTx 同一个理由：这个方法目前只有 serve 的一个端点调用，
// 放进 conversation.Tx 会逼 turn / repl 里每个假 Store+Tx 都补一个它们
// 永远不会调用的方法。用包内断言把「只有本包认识它」写在类型上。
type softDeleteTx interface {
	softDeleteConversation(ctx context.Context, convID string) error
}

// SoftDeleteConversation 把一场会话标成已删除：只写 deleted_at，不动
// title、cwd、team_id，也不动消息、成员、用量与执行状态。它是软删，
// 不是物理删除 —— 库里的记录一条都不少，所有读入口改由
// conversations.deleted_at IS NULL 把它们挡在外面。
//
// 幂等：同一场会话删第二次不报错、也不改写第一次的时间戳。行根本不存在
// 才返回包了 ErrConversationNotFound 的错误。HTTP 层要区分「已删」与
// 「从未存在」时用自己的可见性检查（Conversation 已把已删读成不存在），
// 而不是让这里把「已经删了」说成错误。
//
// 会话不存在时返回包了 ErrConversationNotFound 的错误，让 HTTP 层能把
// 404 与「库坏了」分开。存在性检查与 UPDATE 在同一个写事务里
// （同一个写句柄上的 BEGIN IMMEDIATE），中间没有可乘之窗。
func (s *Store) SoftDeleteConversation(ctx context.Context, convID string) error {
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(softDeleteTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持软删除会话", tx)
		}
		return impl.softDeleteConversation(ctx, convID)
	})
	if err != nil {
		return fmt.Errorf("soft delete conversation %s: %w", convID, err)
	}
	return nil
}

// softDeleteConversation 是 SoftDeleteConversation 的事务内动作。
//
// 先读 deleted_at 再 UPDATE，而不是直接 UPDATE ... WHERE deleted_at IS NULL
// 看 RowsAffected：那样无法区分「行不存在」与「已经删过」，而这两件事
// 在本方法里必须分开 —— 前者是 ErrConversationNotFound，后者是幂等成功。
// 多一次主键查询换来的是正确语义，不是防御性代码。
func (t *Tx) softDeleteConversation(ctx context.Context, convID string) error {
	var deletedAt sql.NullString
	err := t.tx.QueryRowContext(ctx,
		`SELECT deleted_at FROM conversations WHERE id = ?`, convID,
	).Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("conversation %s: %w", convID, conversation.ErrConversationNotFound)
	}
	if err != nil {
		return fmt.Errorf("check conversation %s: %w", convID, err)
	}
	if deletedAt.Valid {
		// 已经删过：不改写时间戳（它回答的是「第一次被删是什么时候」），
		// 也不把重试当成错误。
		return nil
	}
	if _, err := t.tx.ExecContext(ctx,
		`UPDATE conversations SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
		formatTime(time.Now().UTC()), convID,
	); err != nil {
		return fmt.Errorf("update conversation deleted_at %s: %w", convID, err)
	}
	return nil
}

// AppendMessage 追加一条消息。ID、Seq、CreatedAt 由 store 填写，调用方传入的这三个字段一律忽略。
func (t *Tx) AppendMessage(ctx context.Context, m conversation.Message) (conversation.Message, error) {
	if !m.Kind.Valid() {
		return conversation.Message{}, fmt.Errorf("message kind %q: %w", m.Kind, conversation.ErrInvalidKind)
	}
	if err := t.requireConversation(ctx, m.ConvID); err != nil {
		return conversation.Message{}, err
	}

	var next int64
	if err := t.tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq),0)+1 FROM messages WHERE conv_id = ?`, m.ConvID,
	).Scan(&next); err != nil {
		return conversation.Message{}, fmt.Errorf("分配 seq: %w", err)
	}

	id, err := newID()
	if err != nil {
		return conversation.Message{}, fmt.Errorf("append message: %w", err)
	}
	createdAt := formatTime(time.Now().UTC())
	parsed, err := parseTime(createdAt)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("append message: %w", err)
	}

	recJSON, err := encodeStringSlice(m.Recipients)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("append message: %w", err)
	}
	metaJSON, err := encodeMeta(m.Meta)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("append message: %w", err)
	}
	recs, err := decodeStringSlice(recJSON)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("append message: %w", err)
	}
	metaMap, err := decodeMeta(metaJSON)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("append message: %w", err)
	}

	parent := sql.NullString{String: m.ParentID, Valid: m.ParentID != ""}
	if _, err := t.tx.ExecContext(ctx, `
INSERT INTO messages (
    id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort, recipients, kind, text, thought, parent_id, created_at, meta
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, m.ConvID, next, m.SenderID, m.SenderProvider, m.SenderModel, m.SenderEffort, recJSON, string(m.Kind), m.Text, m.Thought, parent, createdAt, metaJSON,
	); err != nil {
		return conversation.Message{}, fmt.Errorf("insert message: %w", err)
	}

	return conversation.Message{
		ID:             id,
		ConvID:         m.ConvID,
		Seq:            next,
		SenderID:       m.SenderID,
		SenderProvider: m.SenderProvider,
		SenderModel:    m.SenderModel,
		SenderEffort:   m.SenderEffort,
		Recipients:     recs,
		Kind:           m.Kind,
		Text:           m.Text,
		Thought:        m.Thought,
		ParentID:       m.ParentID,
		CreatedAt:      parsed,
		Meta:           metaMap,
	}, nil
}

// AddParticipant 把 agent 加入会话。若该 agent 曾经退出，则重新激活同一行，而不是插入第二行。
func (t *Tx) AddParticipant(ctx context.Context, p conversation.Participant) error {
	if err := t.requireConversation(ctx, p.ConvID); err != nil {
		return err
	}
	joined := p.JoinedAt
	if joined.IsZero() {
		joined = time.Now().UTC()
	} else {
		joined = joined.UTC()
	}
	joinedAt := formatTime(joined)

	if _, err := t.tx.ExecContext(ctx, `
INSERT INTO participants (conv_id, agent_id, display_name, provider, active, joined_at, left_at)
VALUES (?, ?, ?, ?, 1, ?, NULL)
ON CONFLICT (conv_id, agent_id) DO UPDATE SET
    display_name = excluded.display_name,
    provider = excluded.provider,
    active = 1,
    joined_at = excluded.joined_at,
    left_at = NULL`,
		p.ConvID, p.AgentID, p.DisplayName, p.Provider, joinedAt,
	); err != nil {
		return fmt.Errorf("add participant: %w", err)
	}
	return nil
}

// DeactivateParticipant 软删除成员：active 置 0、left_at 置传入时间，不删除该 agent 的任何消息。
func (t *Tx) DeactivateParticipant(ctx context.Context, convID, agentID string, at time.Time) error {
	res, err := t.tx.ExecContext(ctx,
		`UPDATE participants SET active = 0, left_at = ? WHERE conv_id = ? AND agent_id = ?`,
		formatTime(at), convID, agentID,
	)
	if err != nil {
		return fmt.Errorf("deactivate participant: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("deactivate participant rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("participant %s in conversation %s: %w", agentID, convID, conversation.ErrParticipantNotFound)
	}
	return nil
}

// SetCursor 按 (conv_id, agent_id) upsert 游标。
func (t *Tx) SetCursor(ctx context.Context, c conversation.Cursor) error {
	if err := t.requireConversation(ctx, c.ConvID); err != nil {
		return err
	}
	updated := c.UpdatedAt
	if updated.IsZero() {
		updated = time.Now().UTC()
	} else {
		updated = updated.UTC()
	}
	pendingJSON, err := encodeInt64Slice(c.Pending)
	if err != nil {
		return fmt.Errorf("set cursor: %w", err)
	}
	if _, err := t.tx.ExecContext(ctx, `
INSERT INTO cursors (conv_id, agent_id, seq, pending, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (conv_id, agent_id) DO UPDATE SET
    seq = excluded.seq,
    pending = excluded.pending,
    updated_at = excluded.updated_at`,
		c.ConvID, c.AgentID, c.Seq, pendingJSON, formatTime(updated),
	); err != nil {
		return fmt.Errorf("set cursor: %w", err)
	}
	return nil
}

func (t *Tx) requireConversation(ctx context.Context, id string) error {
	var dummy string
	err := t.tx.QueryRowContext(ctx, `SELECT id FROM conversations WHERE id = ?`, id).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("conversation %s: %w", id, conversation.ErrConversationNotFound)
	}
	if err != nil {
		return fmt.Errorf("check conversation %s: %w", id, err)
	}
	return nil
}

// RequireActiveParticipant 在本事务内确认 agentID 是 convID 当前的活跃成员。
// 查不到、或查到但已退出（active = 0），都返回包了 ErrParticipantNotFound 的错误。
//
// 它必须在事务内，这是本方法存在的全部理由：Service.Post 原先在事务外用读句柄
// 查一次活跃成员再另起写事务，两者之间有窗口 —— 并发发言时读到「活跃」之后、
// 提交之前用户 /leave 先行提交，这条消息照样落库，事后 /who 显示它已退出、
// left_at 还早于这条消息的 created_at，而没有任何一层会报错（原未决 #13）。
//
// 条件里的 active = 1 不能省：退出是软删除，行还在，只判存在会把退出的成员放过去。
func (t *Tx) RequireActiveParticipant(ctx context.Context, convID, agentID string) error {
	var dummy string
	err := t.tx.QueryRowContext(ctx, `SELECT agent_id FROM participants WHERE conv_id = ? AND agent_id = ? AND active = 1`, convID, agentID).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("participant %s in conversation %s: %w", agentID, convID, conversation.ErrParticipantNotFound)
	}
	if err != nil {
		return fmt.Errorf("check participant %s in conversation %s: %w", agentID, convID, err)
	}
	return nil
}

// RecordUsage 插入一行 turn_usage。TurnID 冲突时返回错误，不 upsert：
// 同一个 turn_id 写两次说明调用方把一轮记了两遍，那是 bug 不是重试。
//
// 七个可空用量列必须按指针原样写成 NULL 或整数，绝不能把 nil 收成 0。
// Step 6 的真实字节里三种情况在 v1 里长得一样：agy 根本没有 cache_creation；
// grok 的 cache_creation_input_tokens 恒为 0 而 cache_read 从 0 涨到 20608；
// 失败轮里 claude 与 agy 的五项是货真价实的 0。NULL 与 0 不是一回事。
func (t *Tx) RecordUsage(ctx context.Context, u conversation.TurnUsage) error {
	if err := t.requireConversation(ctx, u.ConvID); err != nil {
		return err
	}
	created := u.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	} else {
		created = created.UTC()
	}
	cold := 0
	if u.ColdStart {
		cold = 1
	}
	if _, err := t.tx.ExecContext(ctx, `
INSERT INTO turn_usage (
    turn_id, conv_id, agent_id, provider_session_id, seq, outcome,
    input_tokens, cache_create, cache_read, output_tokens, thinking_tokens,
    cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes,
    delivered_lo, delivered_hi, created_at, identity_fp, round_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.TurnID, u.ConvID, u.AgentID, u.ProviderSessionID,
		nullInt64FromInt64Ptr(u.Seq), u.Outcome,
		nullInt64FromIntPtr(u.InputTokens),
		nullInt64FromIntPtr(u.CacheCreate),
		nullInt64FromIntPtr(u.CacheRead),
		nullInt64FromIntPtr(u.OutputTokens),
		nullInt64FromIntPtr(u.ThinkingTokens),
		nullFloat64FromPtr(u.CostUSD),
		nullFloat64FromPtr(u.CostCumulativeUSD),
		cold, u.WallMS, u.PromptBytes,
		nullInt64FromInt64Ptr(u.DeliveredLo),
		nullInt64FromInt64Ptr(u.DeliveredHi),
		formatTime(created),
		u.IdentityFP,
		u.RoundID,
	); err != nil {
		return fmt.Errorf("record usage %s: %w", u.TurnID, err)
	}
	return nil
}

// SetProviderSession 按 (ConvID, AgentID) upsert 一条 provider 会话的全部状态。
// 收全字段结构体，不提供部分更新的入口：五样东西必须同生共死，
// 只更新其中一样（尤其是只换会话 id 而留下旧成本基线）会让下一轮差值
// 变成负数，computeCostDelta 返回 nil，那一笔真实花掉的钱从账上消失。
//
// HaveCostTotal 为 false 时 cost_total_usd 写成 NULL，不写 0。
// grok / agy 恒为 false，0 会看起来像「测过、就是零」。
func (t *Tx) SetProviderSession(ctx context.Context, s conversation.ProviderSession) error {
	if err := t.requireConversation(ctx, s.ConvID); err != nil {
		return err
	}
	lastTurn := s.LastTurnAt
	if lastTurn.IsZero() {
		lastTurn = time.Now().UTC()
	} else {
		lastTurn = lastTurn.UTC()
	}
	haveCost := 0
	if s.HaveCostTotal {
		haveCost = 1
	}
	spoken := 0
	if s.Spoken {
		spoken = 1
	}
	baselineSuspect := 0
	if s.BaselineSuspect {
		baselineSuspect = 1
	}
	var costTotal any
	if s.HaveCostTotal {
		costTotal = s.CostTotalUSD
	}
	if _, err := t.tx.ExecContext(ctx, `
INSERT INTO provider_sessions (
    conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd,
    have_cost_total, last_turn_at, identity_fp, spoken, baseline_suspect
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (conv_id, agent_id) DO UPDATE SET
    provider_session_id = excluded.provider_session_id,
    cursor_seq = excluded.cursor_seq,
    cost_total_usd = excluded.cost_total_usd,
    have_cost_total = excluded.have_cost_total,
    last_turn_at = excluded.last_turn_at,
    identity_fp = excluded.identity_fp,
    spoken = excluded.spoken,
    baseline_suspect = excluded.baseline_suspect`,
		s.ConvID, s.AgentID, s.ProviderSessionID, s.CursorSeq, costTotal,
		haveCost, formatTime(lastTurn), s.IdentityFP, spoken, baselineSuspect,
	); err != nil {
		return fmt.Errorf("set provider session %s/%s: %w", s.ConvID, s.AgentID, err)
	}
	return nil
}

// DropProviderSession 删掉一条 provider 会话。作废走这里，
// 不要用「写一个空的 ProviderSession」来表达作废 —— 那会留下一行
// provider_session_id 为空串的记录，与「还没开过会话」在库里长得一样，
// 读回的 ok 是 true，五样东西却是空的，Fresh 判定会走错分支。
// 行本来就不在时视为已经作废，不报错。
//
// existed 由 ExecContext 的 RowsAffected 直接报，不是先 SELECT 一次判存在
// 再 DELETE：/session rebuild 存在的意义就是清掉损坏状态（例如 last_turn_at
// 存了一个 parseTime 解不出来的字符串），而先读一次要经 scanProviderSession
// 解析这一行的每一列，一行损坏的记录会让那次读先失败，进而让删也失败 ——
// 于是最需要 rebuild 的那一行反而 rebuild 不掉。DELETE 本身不解析任何列，
// 只按主键匹配，不受列内容是否合法影响。
func (t *Tx) DropProviderSession(ctx context.Context, convID, agentID string) (existed bool, err error) {
	res, err := t.tx.ExecContext(ctx,
		`DELETE FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`,
		convID, agentID,
	)
	if err != nil {
		return false, fmt.Errorf("drop provider session %s/%s: %w", convID, agentID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("drop provider session %s/%s: rows affected: %w", convID, agentID, err)
	}
	return n > 0, nil
}

// BeginTurn 插入一行 inflight_turns，记下这一轮已经开始。
// 用 INSERT 不用 upsert：(conv_id, agent_id) 上的唯一索引冲突时把数据库错误原样交回。
// 同一会话里同一个 agent 同时两行在飞是调用方的 bug。覆盖掉旧行会丢掉还没落库的
// cursor_seq、scanned_to 和 phase，恢复会从错误断点接着跑。
// 这和 RecordUsage 拒绝同一个 turn_id 写两遍是同一条理由。
func (t *Tx) BeginTurn(ctx context.Context, turn conversation.InflightTurn) error {
	if err := t.requireConversation(ctx, turn.ConvID); err != nil {
		return err
	}
	started := turn.StartedAt
	if started.IsZero() {
		started = time.Now().UTC()
	} else {
		started = started.UTC()
	}
	if _, err := t.tx.ExecContext(ctx, `
INSERT INTO inflight_turns (
    turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id,
    cursor_seq, scanned_to, started_at, phase, tier, work_dir
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		turn.TurnID, turn.ConvID, turn.AgentID, turn.Provider,
		turn.PlannedSessionID, turn.ResumeSessionID,
		turn.CursorSeq, turn.ScannedTo, formatTime(started), string(turn.Phase),
		turn.Tier, turn.WorkDir,
	); err != nil {
		// inflight_turns 上除主键外唯一的 UNIQUE 就是 (conv_id, agent_id)，
		// 所以 SQLITE_CONSTRAINT_UNIQUE（2067）在这里只可能是它。译在 store
		// 这一层：只有本包认识 modernc（open.go 文件头）。主键冲突
		// （SQLITE_CONSTRAINT_PRIMARYKEY，1555）不译：两次用同一个 turn_id
		// 是调用方记重了，不是「别人的在飞记录」。两个 %w 都留着，哨兵和
		// 驱动错误同时在链上。
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return fmt.Errorf("begin turn %s: %w: %w", turn.TurnID, conversation.ErrInflightConflict, err)
		}
		return fmt.Errorf("begin turn %s: %w", turn.TurnID, err)
	}
	return nil
}

// SetInflightPhase 把已有 inflight 行的 phase 改成新值。
// 影响 0 行时返回 ErrInflightMissing：phase 写在轮内，这一行应当已经在。
// 查不到说明 inflight 状态丢了。当成成功会让后面的恢复按旧 phase 继续。
func (t *Tx) SetInflightPhase(ctx context.Context, turnID string, phase conversation.InflightPhase) error {
	res, err := t.tx.ExecContext(ctx,
		`UPDATE inflight_turns SET phase = ? WHERE turn_id = ?`,
		string(phase), turnID,
	)
	if err != nil {
		return fmt.Errorf("set inflight phase %s: %w", turnID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set inflight phase %s: rows affected: %w", turnID, err)
	}
	if n == 0 {
		return fmt.Errorf("inflight turn %s: %w", turnID, conversation.ErrInflightMissing)
	}
	return nil
}

// ClearInflight 按 turn_id 删掉 inflight 行。一轮正常结束或放弃都走这里。
// existed 由 ExecContext 的 RowsAffected 直接报，不是先 SELECT 一次判存在
// 再 DELETE。
func (t *Tx) ClearInflight(ctx context.Context, turnID string) (existed bool, err error) {
	res, err := t.tx.ExecContext(ctx,
		`DELETE FROM inflight_turns WHERE turn_id = ?`,
		turnID,
	)
	if err != nil {
		return false, fmt.Errorf("clear inflight %s: %w", turnID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("clear inflight %s: rows affected: %w", turnID, err)
	}
	return n > 0, nil
}

func nullInt64FromIntPtr(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func nullInt64FromInt64Ptr(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func nullFloat64FromPtr(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}
