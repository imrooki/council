package turn

// 本文件提供一个有状态的假 conversation.Store + conversation.Tx 实现：
// recordingStore。它只服务于一类断言，真库（fixtures_test.go 的
// newTestService，用的是真正的 store.Open）做不到的那类：
//
//   - conversation.Store 接口没有给 turn_usage 表配读方法（store.go 原话：
//     「Tx 存在的理由是事务边界，不是为了给每张表配一组 CRUD」），所以
//     写进去的 TurnUsage 结构体——尤其是七个可空字段各自是不是 nil——
//     在真库路径下无从读回。唯一的读法是拦截 tx.RecordUsage 收到的参数本身。
//   - 「RecordTurn 到底被调用了几次、每次传的 msg/session 是什么形状」
//     同样只能靠拦截 Tx 方法的调用序列反推：conversation.Service 是具体
//     类型，Runner.svc 是 *conversation.Service 字段，不是接口，没法换一个
//     spy 实现直接拦截 Service.RecordTurn 本身。但 Service.RecordTurn 的
//     控制流是已读过的公开源码（internal/conversation/service.go）：
//     msg!=nil 时先调 tx.RequireActiveParticipant，失败就直接 return，
//     根本不会走到 tx.AppendMessage / tx.RecordUsage；msg==nil 时跳过那一步
//     直接调 tx.RecordUsage。这两条路径在 Tx 方法的调用序列上留下的痕迹
//     不同，因此可以结构性地反推「RecordTurn 被调用了几次、各自是哪一种
//     形状」，不需要真的拦到 Service.RecordTurn 这一层。
//
// 能用真库验证的（读回游标/cost基线/last_turn_at/身份指纹两个分支）仍然
// 用 newTestService，与本包既有测试保持同一套约定——这不是图省事，是因为
// 那部分行为本来就经 svc.ProviderSession / Record 的可观测字段暴露了，
// 不需要拦截调用序列。

import (
	"context"
	"fmt"
	"sort"
	"time"

	"council/internal/conversation"
)

// requireActiveCall 记下一次 RequireActiveParticipant 调用收到的参数。
// 它是「RecordTurn 被调用时 msg 是不是非 nil」的结构性证据：Service.RecordTurn
// 只在 msg!=nil 且 SenderID 不是保留 sender 时才会调这个方法。
type requireActiveCall struct {
	convID, agentID string
}

// recordingStore 是 conversation.Store 与 conversation.Tx 的有状态假实现。
// 单个会话、单个 provider_sessions 映射，够用即可——本类型不追求覆盖
// 真库的全部边界情况（那是 internal/store 自己测试的职责），只求能撑起
// 一次完整的 Ask 流程并把关键调用记下来。
type recordingStore struct {
	conv conversation.Conversation

	participants map[string]*conversation.Participant // agentID -> 当前状态
	messages     []conversation.Message
	nextSeq      int64

	providerSessions map[sessionKey]conversation.ProviderSession

	// inflightTurns 是 Step 10 给 Tx/Store 加的在飞记录状态，键是 turn_id。
	// 与 providerSessions 同一个理由——本假实现本来就在维护这份状态，
	// BeginTurn/SetInflightPhase/ClearInflight/AllInflight 四个方法都读写
	// 同一份数据，不能有的记账、有的却退化成不维护状态，那会让
	// 「这一轮在不在飞」出现两个互相矛盾的答案（AllProviderSessions 那条
	// 注释的原话）。
	inflightTurns map[string]conversation.InflightTurn

	// 调用记录：每一项是这次调用收到的参数快照（值拷贝，不是指针，
	// 后续调用改不动已经记下的这一份）。
	appendMessageCalls       []conversation.Message
	requireActiveCalls       []requireActiveCall
	recordUsageCalls         []conversation.TurnUsage
	setProviderSessionCalls  []conversation.ProviderSession
	dropProviderSessionCalls []sessionKey
	setCursorCalls           []conversation.Cursor
	setConversationTeamCalls []setConversationTeamCall

	// 契约 §3.15 的调用序列断言（BeginTurn -> spawn -> phase=spawned ->
	// phase=returned -> WriteTx -> ... -> ClearInflight）要靠这三份记账核对
	// 调用发生过、且参数对，与 dropProviderSessionCalls 同一个体例。
	beginTurnCalls        []conversation.InflightTurn
	setInflightPhaseCalls []setInflightPhaseCall
	clearInflightCalls    []string
}

// setConversationTeamCall 记录一次 Tx.SetConversationTeam 调用收到的参数
// （Step 9 片 2a 新增，供 BindTeam 相关用例核对调用发生过、且参数对）。
type setConversationTeamCall struct {
	convID string
	teamID string
}

// setInflightPhaseCall 记录一次 Tx.SetInflightPhase 调用收到的参数。
type setInflightPhaseCall struct {
	turnID string
	phase  conversation.InflightPhase
}

// newRecordingStore 造一个空的 recordingStore，会话 id 固定，测试之间互不
// 干扰（每个测试都新建一份自己的实例）。conv.Title 留给 CreateConversation
// 填——这样调用方仍然用既有的 newConv(t, svc) 建造函数，不必另起一套。
func newRecordingStore() *recordingStore {
	return &recordingStore{
		conv:             conversation.Conversation{ID: "rec-conv-1"},
		participants:     make(map[string]*conversation.Participant),
		providerSessions: make(map[sessionKey]conversation.ProviderSession),
		inflightTurns:    make(map[string]conversation.InflightTurn),
	}
}

// ---------------------------------------------------------------------------
// conversation.Store
// ---------------------------------------------------------------------------

func (s *recordingStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return fn(ctx, s)
}

func (s *recordingStore) Conversation(ctx context.Context, id string) (conversation.Conversation, error) {
	if id != s.conv.ID {
		return conversation.Conversation{}, conversation.ErrConversationNotFound
	}
	return s.conv, nil
}

func (s *recordingStore) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	return []conversation.Conversation{s.conv}, nil
}

func (s *recordingStore) Participants(ctx context.Context, convID string, includeInactive bool) ([]conversation.Participant, error) {
	out := make([]conversation.Participant, 0, len(s.participants))
	for _, p := range s.participants {
		if !includeInactive && !p.Active {
			continue
		}
		out = append(out, *p)
	}
	// 排序只是为了输出稳定、便于调试；本文件的测试都按 AgentID 直接查找，
	// 不依赖这里的顺序，但稳定的顺序不该是运气。
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out, nil
}

func (s *recordingStore) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]conversation.Message, error) {
	out := make([]conversation.Message, 0)
	for _, m := range s.messages {
		if m.Seq > afterSeq {
			out = append(out, m)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *recordingStore) MaxSeq(ctx context.Context, convID string) (int64, error) {
	return s.nextSeq, nil
}

func (s *recordingStore) Cursor(ctx context.Context, convID, agentID string) (conversation.Cursor, error) {
	// Step 7 起不再往 cursors 表写 agent 的会话游标（游标已经搬进
	// provider_sessions.cursor_seq），Runner 也从不读这个方法。恒定返回
	// ErrNoCursor，与真库在没有行时的行为一致。
	return conversation.Cursor{}, conversation.ErrNoCursor
}

func (s *recordingStore) Cursors(ctx context.Context, convID string) ([]conversation.Cursor, error) {
	return nil, nil
}

func (s *recordingStore) ProviderSession(ctx context.Context, convID, agentID string) (conversation.ProviderSession, bool, error) {
	ps, ok := s.providerSessions[sessionKey{convID: convID, agentID: agentID}]
	return ps, ok, nil
}

// AllProviderSessions 从 s.providerSessions 这份唯一的数据源里排序返回。
// 本假实现本来就在维护一组有状态的 provider 会话（ProviderSession 就读它），
// 新增的全量读法若另编一份数据或者退化成 nil，会让同一个假实现对
// 「这条会话在不在库里」给出两个互相矛盾的答案。
func (s *recordingStore) AllProviderSessions(ctx context.Context) ([]conversation.ProviderSession, error) {
	out := make([]conversation.ProviderSession, 0, len(s.providerSessions))
	for _, ps := range s.providerSessions {
		out = append(out, ps)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConvID != out[j].ConvID {
			return out[i].ConvID < out[j].ConvID
		}
		return out[i].AgentID < out[j].AgentID
	})
	return out, nil
}

// InflightFor 为 Store.InflightFor 补的方法，读同一份 inflightTurns map。
func (s *recordingStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	for _, t := range s.inflightTurns {
		if t.ConvID == convID && t.AgentID == agentID {
			return t, true, nil
		}
	}
	return conversation.InflightTurn{}, false, nil
}

// AllInflight 从 s.inflightTurns 这份唯一的数据源里按 StartedAt 排序返回，
// 与 AllProviderSessions 同一个理由：本假实现本来就在维护这份状态，
// 另编一份数据或者退化成 nil 会让「这一轮在不在飞」出现两个互相矛盾的答案。
func (s *recordingStore) AllInflight(ctx context.Context) ([]conversation.InflightTurn, error) {
	out := make([]conversation.InflightTurn, 0, len(s.inflightTurns))
	for _, t := range s.inflightTurns {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}

// UsageSamples 只为满足接口而存在。recordUsageCalls 只记录 RecordUsage
// 收到的写入参数，本文件的用例不走预报路径，这里不做读回、不加记录。
func (s *recordingStore) UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

// TurnUsages 是 Step 10 D 片之一给 Store 加的读方法（/cost 用），只为满足
// 接口而存在，与上面 UsageSamples 同一个理由：本文件靠 recordUsageCalls
// 直接检查 RecordUsage 收到的参数（值拷贝，调用序列/形状类断言），不经过
// Store 接口把 turn_usage 读回来。
//
// 失效条件：哪天有一条用例要验证"记过的 turn_usage 经 TurnUsages 读得回来"
// （而不是只检查 RecordUsage 收到的写入参数），要把这里改成从
// recordUsageCalls 过滤 + 按 (conv_id, agent_id) 挑 + 排序，不能继续恒答
// nil——那样会把"没读到"和"压根没记过"混成一回事，与 AllProviderSessions
// 那条注释是同一条纪律。
func (s *recordingStore) TurnUsages(ctx context.Context, convID, agentID string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// conversation.Tx
// ---------------------------------------------------------------------------

func (s *recordingStore) CreateConversation(ctx context.Context, title string) (conversation.Conversation, error) {
	s.conv.Title = title
	if s.conv.CreatedAt.IsZero() {
		s.conv.CreatedAt = time.Now().UTC()
	}
	return s.conv, nil
}

func (s *recordingStore) AppendMessage(ctx context.Context, m conversation.Message) (conversation.Message, error) {
	s.nextSeq++
	m.Seq = s.nextSeq
	if m.ID == "" {
		m.ID = fmt.Sprintf("recording-msg-%d", m.Seq)
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	s.messages = append(s.messages, m)
	s.appendMessageCalls = append(s.appendMessageCalls, m)
	return m, nil
}

func (s *recordingStore) AddParticipant(ctx context.Context, p conversation.Participant) error {
	cp := p
	cp.Active = true
	cp.LeftAt = nil
	s.participants[p.AgentID] = &cp
	return nil
}

func (s *recordingStore) DeactivateParticipant(ctx context.Context, convID, agentID string, at time.Time) error {
	p, ok := s.participants[agentID]
	if !ok {
		return fmt.Errorf("participant %s in conversation %s: %w", agentID, convID, conversation.ErrParticipantNotFound)
	}
	p.Active = false
	t := at
	p.LeftAt = &t
	return nil
}

func (s *recordingStore) RequireActiveParticipant(ctx context.Context, convID, agentID string) error {
	s.requireActiveCalls = append(s.requireActiveCalls, requireActiveCall{convID: convID, agentID: agentID})
	p, ok := s.participants[agentID]
	if !ok || !p.Active {
		return fmt.Errorf("participant %s in conversation %s: %w", agentID, convID, conversation.ErrParticipantNotFound)
	}
	return nil
}

func (s *recordingStore) SetCursor(ctx context.Context, c conversation.Cursor) error {
	s.setCursorCalls = append(s.setCursorCalls, c)
	return nil
}

func (s *recordingStore) RecordUsage(ctx context.Context, u conversation.TurnUsage) error {
	s.recordUsageCalls = append(s.recordUsageCalls, u)
	return nil
}

func (s *recordingStore) SetProviderSession(ctx context.Context, ps conversation.ProviderSession) error {
	s.setProviderSessionCalls = append(s.setProviderSessionCalls, ps)
	s.providerSessions[sessionKey{convID: ps.ConvID, agentID: ps.AgentID}] = ps
	return nil
}

func (s *recordingStore) DropProviderSession(ctx context.Context, convID, agentID string) (bool, error) {
	key := sessionKey{convID: convID, agentID: agentID}
	s.dropProviderSessionCalls = append(s.dropProviderSessionCalls, key)
	_, existed := s.providerSessions[key]
	delete(s.providerSessions, key)
	return existed, nil
}

// SetConversationTeam 是 Step 9 片 2a 给 Tx 加的写方法（Service.BindTeam
// 用）。有记账，不是空桩：不写成
// `func (...) SetConversationTeam(...) error { return nil }`——那是一条
// 永远测不出问题的假路（12 契约 3.9 第 5 条任务书原话）。
func (s *recordingStore) SetConversationTeam(ctx context.Context, convID, teamID string) error {
	s.setConversationTeamCalls = append(s.setConversationTeamCalls, setConversationTeamCall{
		convID: convID, teamID: teamID,
	})
	return nil
}

// BeginTurn 记账并把这一行插进 s.inflightTurns：契约 §3.15 的调用序列
// 断言（BeginTurn -> spawn -> phase=spawned -> phase=returned -> WriteTx ->
// ... -> ClearInflight）要靠这份记账核对 BeginTurn 确实发生过、且发生在
// spawn 之前。与 SetProviderSession 同一个体例：既记账、也回写状态，
// 不是「记账不等于生效」那条教训里的假路。
func (s *recordingStore) BeginTurn(ctx context.Context, t conversation.InflightTurn) error {
	s.beginTurnCalls = append(s.beginTurnCalls, t)
	s.inflightTurns[t.TurnID] = t
	return nil
}

// SetInflightPhase 记账并原地改 phase，供「phase=spawned -> phase=returned」
// 这类顺序断言核对参数、也供读回用。
func (s *recordingStore) SetInflightPhase(ctx context.Context, turnID string, phase conversation.InflightPhase) error {
	s.setInflightPhaseCalls = append(s.setInflightPhaseCalls, setInflightPhaseCall{turnID: turnID, phase: phase})
	if t, ok := s.inflightTurns[turnID]; ok {
		t.Phase = phase
		s.inflightTurns[turnID] = t
	}
	return nil
}

// ClearInflight 记账并删除，existed 如实反映删之前这一行在不在——与
// DropProviderSession 同体例，不能对「这一行在不在」给出两个矛盾的答案。
func (s *recordingStore) ClearInflight(ctx context.Context, turnID string) (bool, error) {
	s.clearInflightCalls = append(s.clearInflightCalls, turnID)
	_, existed := s.inflightTurns[turnID]
	delete(s.inflightTurns, turnID)
	return existed, nil
}

// agentAppendCount 数 appendMessageCalls 里 Kind==KindAgent 的条目——这是
// 「RecordTurn 被调用时 msg 非 nil 且真正落库」的证据。系统面包屑
// （session_opening / turn_failed / post_rejected）与用户消息也会经同一个
// AppendMessage 落地，必须按 Kind 过滤掉才不会把它们误算成 agent 的发言。
func (s *recordingStore) agentAppendCount() int {
	n := 0
	for _, m := range s.appendMessageCalls {
		if m.Kind == conversation.KindAgent {
			n++
		}
	}
	return n
}
