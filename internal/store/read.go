package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"council/internal/conversation"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// visibleConv 是「这条会话没有被软删」的 SQL 条件。每一个按 convID 的读入口
// 都必须带上它：deleted_at 只是标记，会话行、消息、成员、用量都还在库里，
// 少一个条件，已删会话就会从某一条侧门重新露出来。参数是外层表名，用来把
// conversations.id 与那一行的 conv_id 对上；只许传本文件里写死的表名。
//
// 用 EXISTS 而不是 JOIN：这些查询本来只读一张表，加 JOIN 会让每一列都要
// 判断歧义，而且 LIMIT / ORDER BY 的语义也得跟着重看一遍。EXISTS 只加一个
// 半连接，读出来的行数与列都保持原样。
func visibleConv(table string) string {
	return `EXISTS (SELECT 1 FROM conversations c WHERE c.id = ` + table + `.conv_id AND c.deleted_at IS NULL)`
}

// 全库读的例外：AllProviderSessions、AllInflight 与 UsageSamples 不按 convID
// 查，它们回答的是库级的执行状态与成本样本。软删一个会话不能把它的在飞轮
// 从崩溃恢复里藏起来，也不该让一次删除改写同 provider 的历史样本；这些行
// 没有对外的 convID 入口（带 id 的入口都先经 Conversation，已删即 404），
// 所以不过滤。

// Conversation 按 id 取一场会话。不存在时返回包装了 ErrConversationNotFound 的错误。
// 已软删的会话按不存在处理：对外是 404，不能只返回一场空标题的会话。
func (s *Store) Conversation(ctx context.Context, id string) (conversation.Conversation, error) {
	row := s.read.QueryRowContext(ctx,
		`SELECT id, title, created_at, team_id, cwd FROM conversations WHERE id = ? AND deleted_at IS NULL`, id,
	)
	c, err := scanConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Conversation{}, fmt.Errorf("conversation %s: %w", id, conversation.ErrConversationNotFound)
	}
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("get conversation %s: %w", id, err)
	}
	return c, nil
}

// Conversations 列出全部未删除的会话，按 created_at 倒序、id 升序。空结果是空切片不是 nil。
func (s *Store) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT id, title, created_at, team_id, cwd FROM conversations WHERE deleted_at IS NULL ORDER BY created_at DESC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	out := make([]conversation.Conversation, 0)
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("list conversations: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return out, nil
}

// Participants 列出一场会话的成员。includeInactive 为 false 时只返回仍活跃的。
// 结果按 agent_id 升序。空结果是空切片不是 nil。
// 已软删的会话按「没有成员」处理，不把成员行留给调用方去拼。
func (s *Store) Participants(ctx context.Context, convID string, includeInactive bool) ([]conversation.Participant, error) {
	q := `SELECT conv_id, agent_id, display_name, provider, active, joined_at, left_at FROM participants WHERE conv_id = ? AND ` + visibleConv("participants")
	args := []any{convID}
	if !includeInactive {
		q += ` AND active = 1`
	}
	q += ` ORDER BY agent_id ASC`

	rows, err := s.read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list participants %s: %w", convID, err)
	}
	defer rows.Close()

	out := make([]conversation.Participant, 0)
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, fmt.Errorf("list participants %s: %w", convID, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list participants %s: %w", convID, err)
	}
	return out, nil
}

// Messages 返回 seq 严格大于 afterSeq 的消息，按 seq 升序。
// limit 小于等于 0 表示不限。空结果是空切片不是 nil。
// 已软删的会话返回空切片：消息行还在，但对外的会话已经不存在。
func (s *Store) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]conversation.Message, error) {
	const cols = `id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort, recipients, kind, text, thought, parent_id, created_at, meta`
	q := `SELECT ` + cols + ` FROM messages WHERE conv_id = ? AND ` + visibleConv("messages") + ` AND seq > ? ORDER BY seq ASC`

	var (
		rows *sql.Rows
		err  error
	)
	if limit > 0 {
		rows, err = s.read.QueryContext(ctx, q+` LIMIT ?`, convID, afterSeq, limit)
	} else {
		rows, err = s.read.QueryContext(ctx, q, convID, afterSeq)
	}
	if err != nil {
		return nil, fmt.Errorf("list messages %s: %w", convID, err)
	}
	defer rows.Close()

	out := make([]conversation.Message, 0)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("list messages %s: %w", convID, err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list messages %s: %w", convID, err)
	}
	return out, nil
}

// MaxSeq 返回一场会话当前最大 seq。没有消息时返回 0。
// 已软删的会话按「没有消息」处理：它对外已经不存在，seq 不该再续。
func (s *Store) MaxSeq(ctx context.Context, convID string) (int64, error) {
	var seq int64
	if err := s.read.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM messages WHERE conv_id = ? AND `+visibleConv("messages"), convID,
	).Scan(&seq); err != nil {
		return 0, fmt.Errorf("max seq %s: %w", convID, err)
	}
	return seq, nil
}

// Cursor 取某个 agent 在一场会话里的游标。读不到时返回包装了 ErrNoCursor 的错误。
// 已软删的会话读不到游标：它的执行状态仍在库里，但不能经 convID 再被读出来。
func (s *Store) Cursor(ctx context.Context, convID, agentID string) (conversation.Cursor, error) {
	row := s.read.QueryRowContext(ctx,
		`SELECT conv_id, agent_id, seq, pending, updated_at FROM cursors WHERE conv_id = ? AND agent_id = ? AND `+visibleConv("cursors"),
		convID, agentID,
	)
	c, err := scanCursor(row)
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Cursor{}, fmt.Errorf("cursor %s/%s: %w", convID, agentID, conversation.ErrNoCursor)
	}
	if err != nil {
		return conversation.Cursor{}, fmt.Errorf("get cursor %s/%s: %w", convID, agentID, err)
	}
	return c, nil
}

// Cursors 列出一场会话的全部游标，按 agent_id 升序。空结果是空切片不是 nil。
// 已软删的会话返回空切片，理由同 Cursor。
func (s *Store) Cursors(ctx context.Context, convID string) ([]conversation.Cursor, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT conv_id, agent_id, seq, pending, updated_at FROM cursors WHERE conv_id = ? AND `+visibleConv("cursors")+` ORDER BY agent_id ASC`,
		convID,
	)
	if err != nil {
		return nil, fmt.Errorf("list cursors %s: %w", convID, err)
	}
	defer rows.Close()

	out := make([]conversation.Cursor, 0)
	for rows.Next() {
		c, err := scanCursor(rows)
		if err != nil {
			return nil, fmt.Errorf("list cursors %s: %w", convID, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cursors %s: %w", convID, err)
	}
	return out, nil
}

// ProviderSession 读回一条 provider 会话。第二个返回值为 false 表示
// 这个 agent 在这条会话里还没开过口（或上一次的状态已被作废）。
// 读不到行是 ok=false、err=nil，不是错误；解析失败才是错误。
// 会话被软删时同样返回 ok=false：行还在库里（恢复对账要用），但 convID
// 已不再是一条可见的读路径。
// 不把缺行和「行在但某列是空的」混成同一种结果 —— 后者该由调用方
// 按「五样同生共死」整份作废，而不是在这里悄悄当成没开过口。
func (s *Store) ProviderSession(ctx context.Context, convID, agentID string) (conversation.ProviderSession, bool, error) {
	row := s.read.QueryRowContext(ctx, `
SELECT conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd,
       have_cost_total, last_turn_at, identity_fp, spoken, baseline_suspect
FROM provider_sessions WHERE conv_id = ? AND agent_id = ? AND `+visibleConv("provider_sessions"),
		convID, agentID,
	)
	ps, err := scanProviderSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.ProviderSession{}, false, nil
	}
	if err != nil {
		return conversation.ProviderSession{}, false, fmt.Errorf("get provider session %s/%s: %w", convID, agentID, err)
	}
	return ps, true, nil
}

// AllProviderSessions 列出全库的 provider 会话，按 conv_id, agent_id 升序。
// 与单行版本 ProviderSession 同构：去掉 WHERE、加 ORDER BY，复用同一个
// scanProviderSession——cost_total_usd 可空与 last_turn_at 解析这两处
// 不该有第二份写法。空结果是空切片不是 nil，与本文件其余 List 类方法一致。
func (s *Store) AllProviderSessions(ctx context.Context) ([]conversation.ProviderSession, error) {
	rows, err := s.read.QueryContext(ctx, `
SELECT conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd,
       have_cost_total, last_turn_at, identity_fp, spoken, baseline_suspect
FROM provider_sessions ORDER BY conv_id, agent_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list provider sessions: %w", err)
	}
	defer rows.Close()

	out := make([]conversation.ProviderSession, 0)
	for rows.Next() {
		ps, err := scanProviderSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list provider sessions: %w", err)
		}
		out = append(out, ps)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list provider sessions: %w", err)
	}
	return out, nil
}

// AllInflight 列出 inflight_turns 全表，按 started_at 升序。
// 升序是为了按开跑时间从早到晚重放；没有 WHERE，代价是每次读完整张表，
// 不适合在热路径上反复调用。十二列的解析只放在 scanInflightTurn
// （全部 NOT NULL，只有 started_at 走 parseTime，phase 从 TEXT 收成
// InflightPhase），避免和以后的单行读取各写一份。空结果是空切片不是 nil，
// 与 Conversations、Participants、Messages、Cursors、AllProviderSessions 一致。
func (s *Store) AllInflight(ctx context.Context) ([]conversation.InflightTurn, error) {
	rows, err := s.read.QueryContext(ctx, `
SELECT turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id,
       cursor_seq, scanned_to, started_at, phase, tier, work_dir
FROM inflight_turns ORDER BY started_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list inflight turns: %w", err)
	}
	defer rows.Close()

	out := make([]conversation.InflightTurn, 0)
	for rows.Next() {
		t, err := scanInflightTurn(rows)
		if err != nil {
			return nil, fmt.Errorf("list inflight turns: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list inflight turns: %w", err)
	}
	return out, nil
}

// InflightFor 按 (conv_id, agent_id) 定点读一行在飞记录。走读句柄，不进写事务。
// 没有这一行是 ok=false、err=nil，不是错误；解析失败才是错误。与 ProviderSession 同构。
// 已软删的会话按「没有在飞记录」处理：行仍由 AllInflight 交给启动对账，
// 那一条库级读路径不能因为一次软删就丢掉一轮可能已经花掉的钱。
func (s *Store) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	row := s.read.QueryRowContext(ctx, `
SELECT turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id,
       cursor_seq, scanned_to, started_at, phase, tier, work_dir
FROM inflight_turns WHERE conv_id = ? AND agent_id = ? AND `+visibleConv("inflight_turns"),
		convID, agentID,
	)
	t, err := scanInflightTurn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.InflightTurn{}, false, nil
	}
	if err != nil {
		return conversation.InflightTurn{}, false, fmt.Errorf("get inflight turn %s/%s: %w", convID, agentID, err)
	}
	return t, true, nil
}

// UsageSamples 见 conversation.Store 上同名方法的注释。
// 读句柄、不进写事务。七个可空用量列 NULL 保持 nil 指针，不收成 0。
func (s *Store) UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]conversation.TurnUsage, error) {
	if limit == 0 {
		return []conversation.TurnUsage{}, nil
	}
	const q = `
SELECT turn_id, conv_id, agent_id, provider_session_id, seq, outcome,
       input_tokens, cache_create, cache_read, output_tokens, thinking_tokens,
       cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes,
       delivered_lo, delivered_hi, created_at, identity_fp, round_id
FROM turn_usage
WHERE agent_id = ? AND identity_fp = ? AND input_tokens IS NOT NULL
ORDER BY created_at DESC, turn_id DESC`
	var (
		rows *sql.Rows
		err  error
	)
	if limit > 0 {
		rows, err = s.read.QueryContext(ctx, q+` LIMIT ?`, agentID, identityFP, limit)
	} else {
		rows, err = s.read.QueryContext(ctx, q, agentID, identityFP)
	}
	if err != nil {
		return nil, fmt.Errorf("usage samples %s: %w", agentID, err)
	}
	defer rows.Close()

	out := make([]conversation.TurnUsage, 0)
	for rows.Next() {
		u, err := scanTurnUsage(rows)
		if err != nil {
			return nil, fmt.Errorf("usage samples %s: %w", agentID, err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage samples %s: %w", agentID, err)
	}
	return out, nil
}

// TurnUsages 见 conversation.Store 上同名方法的注释。
// 读句柄、不进写事务。七个可空用量列 NULL 保持 nil 指针，不收成 0，解析只走 scanTurnUsage。
// 已软删的会话返回空切片：账还在库里（成本对账要用），但不再是一条可见的会话。
//
// 排序与 UsageSamples 相反：那边取「最近的样本」，这边是给人看一条会话的账，
// 账要从头往后读，所以 created_at 升序。第二键 turn_id 让同一毫秒内两行的顺序确定，
// 否则 golden 用例会随机红。
//
// 绝不加 input_tokens IS NOT NULL。UsageSamples 有那道是因为它要拿样本去缩放；
// 这里要的是全部行，包括那些什么都不知道的行。滤掉就是又一次把「不知道」藏起来。
//
// limit 与 UsageSamples 同一套：0 要零条，负数不限，正数取最近这么多条。
// 正数不能写成 ORDER BY ASC LIMIT n：那是开头 n 条，不是末尾。
// 这里用子查询先按倒序取末尾 n 条，外层再排回升序。两层的第二键都是 turn_id，
// 同一毫秒内「最近」与「读出来的先后」才是同一套顺序。
func (s *Store) TurnUsages(ctx context.Context, convID, agentID string, limit int) ([]conversation.TurnUsage, error) {
	if limit == 0 {
		return []conversation.TurnUsage{}, nil
	}
	const cols = `turn_id, conv_id, agent_id, provider_session_id, seq, outcome,
       input_tokens, cache_create, cache_read, output_tokens, thinking_tokens,
       cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes,
       delivered_lo, delivered_hi, created_at, identity_fp, round_id`
	where := `WHERE conv_id = ? AND ` + visibleConv("turn_usage")
	args := []any{convID}
	if agentID != "" {
		where += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	var q string
	if limit > 0 {
		q = `SELECT ` + cols + ` FROM (
SELECT ` + cols + `
FROM turn_usage
` + where + `
ORDER BY created_at DESC, turn_id DESC
LIMIT ?)
ORDER BY created_at ASC, turn_id ASC`
		args = append(args, limit)
	} else {
		q = `SELECT ` + cols + `
FROM turn_usage
` + where + `
ORDER BY created_at ASC, turn_id ASC`
	}
	rows, err := s.read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("turn usages %s: %w", convID, err)
	}
	defer rows.Close()

	out := make([]conversation.TurnUsage, 0)
	for rows.Next() {
		u, err := scanTurnUsage(rows)
		if err != nil {
			return nil, fmt.Errorf("turn usages %s: %w", convID, err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("turn usages %s: %w", convID, err)
	}
	return out, nil
}

func scanTurnUsage(sc rowScanner) (conversation.TurnUsage, error) {
	var (
		turnID, convID, agentID, providerSessionID, createdAt, identityFP, roundID string
		outcome, coldStart, wallMS, promptBytes                                    int
		seq, inputTokens, cacheCreate, cacheRead, outputTokens, thinkingTokens     sql.NullInt64
		deliveredLo, deliveredHi                                                   sql.NullInt64
		costUSD, costCumulative                                                    sql.NullFloat64
	)
	if err := sc.Scan(
		&turnID, &convID, &agentID, &providerSessionID, &seq, &outcome,
		&inputTokens, &cacheCreate, &cacheRead, &outputTokens, &thinkingTokens,
		&costUSD, &costCumulative, &coldStart, &wallMS, &promptBytes,
		&deliveredLo, &deliveredHi, &createdAt, &identityFP, &roundID,
	); err != nil {
		return conversation.TurnUsage{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return conversation.TurnUsage{}, fmt.Errorf("turn usage %s created_at: %w", turnID, err)
	}
	u := conversation.TurnUsage{
		TurnID:            turnID,
		ConvID:            convID,
		AgentID:           agentID,
		ProviderSessionID: providerSessionID,
		Outcome:           outcome,
		ColdStart:         coldStart != 0,
		WallMS:            wallMS,
		PromptBytes:       promptBytes,
		CreatedAt:         ts,
		IdentityFP:        identityFP,
		RoundID:           roundID,
	}
	u.Seq = int64PtrFromNull(seq)
	u.InputTokens = intPtrFromNull(inputTokens)
	u.CacheCreate = intPtrFromNull(cacheCreate)
	u.CacheRead = intPtrFromNull(cacheRead)
	u.OutputTokens = intPtrFromNull(outputTokens)
	u.ThinkingTokens = intPtrFromNull(thinkingTokens)
	u.DeliveredLo = int64PtrFromNull(deliveredLo)
	u.DeliveredHi = int64PtrFromNull(deliveredHi)
	if costUSD.Valid {
		v := costUSD.Float64
		u.CostUSD = &v
	}
	if costCumulative.Valid {
		v := costCumulative.Float64
		u.CostCumulativeUSD = &v
	}
	return u, nil
}

func intPtrFromNull(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func int64PtrFromNull(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func scanConversation(sc rowScanner) (conversation.Conversation, error) {
	var id, title, createdAt, teamID, cwd string
	if err := sc.Scan(&id, &title, &createdAt, &teamID, &cwd); err != nil {
		return conversation.Conversation{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return conversation.Conversation{}, fmt.Errorf("conversation %s created_at: %w", id, err)
	}
	return conversation.Conversation{ID: id, Title: title, CreatedAt: ts, TeamID: teamID, Cwd: cwd}, nil
}

func scanParticipant(sc rowScanner) (conversation.Participant, error) {
	var (
		convID, agentID, displayName, provider, joinedAt string
		active                                           int
		leftAt                                           sql.NullString
	)
	if err := sc.Scan(&convID, &agentID, &displayName, &provider, &active, &joinedAt, &leftAt); err != nil {
		return conversation.Participant{}, err
	}
	joined, err := parseTime(joinedAt)
	if err != nil {
		return conversation.Participant{}, fmt.Errorf("participant %s/%s joined_at: %w", convID, agentID, err)
	}
	p := conversation.Participant{
		ConvID:      convID,
		AgentID:     agentID,
		DisplayName: displayName,
		Provider:    provider,
		Active:      active != 0,
		JoinedAt:    joined,
	}
	if leftAt.Valid && leftAt.String != "" {
		t, err := parseTime(leftAt.String)
		if err != nil {
			return conversation.Participant{}, fmt.Errorf("participant %s/%s left_at: %w", convID, agentID, err)
		}
		p.LeftAt = &t
	}
	return p, nil
}

func scanMessage(sc rowScanner) (conversation.Message, error) {
	var (
		id, convID, senderID, senderProvider, senderModel, senderEffort, recipients, kind, text, thought, createdAt, meta string
		seq                                                                                                               int64
		parentID                                                                                                          sql.NullString
	)
	if err := sc.Scan(&id, &convID, &seq, &senderID, &senderProvider, &senderModel, &senderEffort, &recipients, &kind, &text, &thought, &parentID, &createdAt, &meta); err != nil {
		return conversation.Message{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("message %s created_at: %w", id, err)
	}
	recs, err := decodeStringSlice(recipients)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("message %s recipients: %w", id, err)
	}
	metaMap, err := decodeMeta(meta)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("message %s meta: %w", id, err)
	}
	parent := ""
	if parentID.Valid {
		parent = parentID.String
	}
	return conversation.Message{
		ID:             id,
		ConvID:         convID,
		Seq:            seq,
		SenderID:       senderID,
		SenderProvider: senderProvider,
		SenderModel:    senderModel,
		SenderEffort:   senderEffort,
		Recipients:     recs,
		Kind:           conversation.MessageKind(kind),
		Text:           text,
		Thought:        thought,
		ParentID:       parent,
		CreatedAt:      ts,
		Meta:           metaMap,
	}, nil
}

func scanCursor(sc rowScanner) (conversation.Cursor, error) {
	var convID, agentID, pending, updatedAt string
	var seq int64
	if err := sc.Scan(&convID, &agentID, &seq, &pending, &updatedAt); err != nil {
		return conversation.Cursor{}, err
	}
	ts, err := parseTime(updatedAt)
	if err != nil {
		return conversation.Cursor{}, fmt.Errorf("cursor %s/%s updated_at: %w", convID, agentID, err)
	}
	pend, err := decodeInt64Slice(pending)
	if err != nil {
		return conversation.Cursor{}, fmt.Errorf("cursor %s/%s pending: %w", convID, agentID, err)
	}
	return conversation.Cursor{
		ConvID:    convID,
		AgentID:   agentID,
		Seq:       seq,
		Pending:   pend,
		UpdatedAt: ts,
	}, nil
}

func scanProviderSession(sc rowScanner) (conversation.ProviderSession, error) {
	var (
		convID, agentID, providerSessionID, lastTurnAt, identityFP string
		cursorSeq                                                  int64
		costTotal                                                  sql.NullFloat64
		haveCostTotal, spoken, baselineSuspect                     int
	)
	if err := sc.Scan(
		&convID, &agentID, &providerSessionID, &cursorSeq, &costTotal,
		&haveCostTotal, &lastTurnAt, &identityFP, &spoken, &baselineSuspect,
	); err != nil {
		return conversation.ProviderSession{}, err
	}
	ts, err := parseTime(lastTurnAt)
	if err != nil {
		return conversation.ProviderSession{}, fmt.Errorf("provider session %s/%s last_turn_at: %w", convID, agentID, err)
	}
	ps := conversation.ProviderSession{
		ConvID:            convID,
		AgentID:           agentID,
		ProviderSessionID: providerSessionID,
		CursorSeq:         cursorSeq,
		HaveCostTotal:     haveCostTotal != 0,
		LastTurnAt:        ts,
		IdentityFP:        identityFP,
		Spoken:            spoken != 0,
		// baseline_suspect 在库里是 INTEGER，Go 侧是 bool，与 spoken 同一条理由。
		BaselineSuspect: baselineSuspect != 0,
	}
	// cost_total_usd 为 NULL 时保持 float64 零值。HaveCostTotal 才是
	// 「见过金额」的权威；不要用「是不是 0」去反推，grok / agy 从未报过金额。
	if costTotal.Valid {
		ps.CostTotalUSD = costTotal.Float64
	}
	return ps, nil
}

// scanInflightTurn 映射 inflight_turns 的 12 列。每一列在 DDL 里都是 NOT NULL，
// 用 sql.Null* 只会在读路径上多一次 Valid 判断，和写入侧的非空约束重复。
// 唯一要付的解析代价是 started_at：库存 TEXT，经 parseTime 变成 time.Time。
// phase 先按 string 扫出再收成 InflightPhase，扫描时不调用 Valid：非法值留在
// 结构体里给调用方处理；在这里改写或直接报错会让全表列举因为一行坏相位失败。
func scanInflightTurn(sc rowScanner) (conversation.InflightTurn, error) {
	var (
		turnID           string
		convID           string
		agentID          string
		provider         string
		plannedSessionID string
		resumeSessionID  string
		cursorSeq        int64
		scannedTo        int64
		startedAt        string
		phaseStr         string
		tier             string
		workDir          string
	)
	if err := sc.Scan(
		&turnID, &convID, &agentID, &provider, &plannedSessionID, &resumeSessionID,
		&cursorSeq, &scannedTo, &startedAt, &phaseStr, &tier, &workDir,
	); err != nil {
		return conversation.InflightTurn{}, err
	}
	ts, err := parseTime(startedAt)
	if err != nil {
		return conversation.InflightTurn{}, fmt.Errorf("inflight turn %s started_at: %w", turnID, err)
	}
	return conversation.InflightTurn{
		TurnID:           turnID,
		ConvID:           convID,
		AgentID:          agentID,
		Provider:         provider,
		PlannedSessionID: plannedSessionID,
		ResumeSessionID:  resumeSessionID,
		CursorSeq:        cursorSeq,
		ScannedTo:        scannedTo,
		StartedAt:        ts,
		Phase:            conversation.InflightPhase(phaseStr),
		Tier:             tier,
		WorkDir:          workDir,
	}, nil
}
