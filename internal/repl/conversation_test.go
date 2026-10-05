package repl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
)

// replTestNow 是本文件用的固定时刻，UTC。
var replTestNow = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

// fakeStore 是 conversation.Store 与 conversation.Tx 的内存实现。
//
// REPL 的测试不拉起真的 SQLite：这一层要验的是命令分派与提示，
// 库的行为归 internal/store 自己的测试。internal/conversation 的测试里有一份同源的假实现，
// 那份在 package conversation 里，测试文件不能跨包复用，所以这里是第二份。两份已经分叉：
// 这一份加了锁（见下），那一份没有，它的用例都在单个 goroutine 里驱动。
//
// 加锁（2026-09-25，#41 RB-B 剩余项 (j)）：原先的理由是「断言都发生在 Run 已经从 done channel
// 返回之后」，这只管得住断言与写者之间；/round 起多个 worker，各 worker 之间同时读写这里的 map，
// 与断言无关（-count=200 偶发 fatal error: concurrent map writes，-race 报在 AppendMessage、
// BeginTurn / ClearInflight、SetProviderSession / ProviderSession 上）。
// 做法：每个读写状态的方法在入口取 mu，互相调用的部分拆成「调用方持锁」的内部函数。
// WriteTx 不取锁：fn 会回调本对象的 Tx 方法，Mutex 不可重入。所以这里只保证每个方法内部
// 不撕裂，不模仿真 store 的事务隔离（一个事务里的几次写对别的 goroutine 仍是逐次可见的），
// 与加锁之前一样。
type fakeStore struct {
	mu sync.Mutex

	convs   []conversation.Conversation
	convIdx map[string]int
	parts   map[string][]*conversation.Participant
	msgs    map[string][]conversation.Message
	cursors map[string]map[string]conversation.Cursor

	// providerSessions 是本假实现里唯一有状态地维护 provider 会话的地方，
	// 键是 (conv_id, agent_id)。Step 8 之前这几个方法全是恒答「没有」的
	// 空桩：本包的用例从不需要驱动 /rules 或 /session rebuild 这类
	// 真正读写 provider 会话的命令。加了这两条命令之后再恒答「没有」，
	// 每一条相关用例都测不出真实行为，所以补成有状态的实现。
	// 与其它字段一样由 mu 保护。
	providerSessions map[string]map[string]conversation.ProviderSession

	// setConversationTeamCalls 记 Step 9 片 2a 新增的 Tx.SetConversationTeam
	// 每一次调用的参数，供相关用例核对调用发生过、且参数对。
	setConversationTeamCalls []conversationTeamCall

	// inflight 是本假实现维护的在飞记录状态，键是 turn_id。Step 10 A 片之一/
	// 之二落地时这四个方法还可以恒答「没有」——那时 Runner.askUpToWith
	// 从不调用 BeginTurn，本包的用例自然一条都走不到这条分支。之三接上
	// BeginTurn/SetInflightPhase/ClearInflight 的真实调用点之后，本包任何
	// 一条驱动 /ask 的用例都会经 Runner.Ask -> Service.RecordTurn 传
	// expectInflight=true；ClearInflight 若继续恒答 existed=false，
	// Service.RecordTurn 会判定 ErrInflightMissing 并回滚整轮——这正是
	// 之三订正时实测踩到的那类红（TestConfigReloadApplyWarnsOnProviderCommandChangeWithZeroDrift
	// 等四条）。四个方法读写同一份数据，理由与上面 providerSessions 那段
	// 注释相同：不能有的方法看真实状态、有的却恒答「没有」，那会让「这一行
	// 在不在」出现两个互相矛盾的答案。
	inflight map[string]conversation.InflightTurn

	// usages 是本假实现维护的 turn_usage 状态，按插入顺序存放。Step 10 D 片
	// 之一之前，RecordUsage 是空桩、UsageSamples 恒答「没有」——本包那时没有
	// 任何命令会经 Store 接口把 turn_usage 读回来。/cost 落地之后这不再成立：
	// cmdCost 的命令层用例需要种几行 turn_usage、再经真实的 /cost 分派把它们
	// 读回来验证输出，继续恒答「没有」会让这些用例全部测不出差别（12 契约
	// "恒真的空用例"那条教训，见下面 SetConversationTeam 那条注释引的原话）。
	usages []conversation.TurnUsage

	nextID int
}

// conversationTeamCall 记录一次 Tx.SetConversationTeam 调用收到的参数。
type conversationTeamCall struct {
	convID string
	teamID string
}

var (
	_ conversation.Store = (*fakeStore)(nil)
	_ conversation.Tx    = (*fakeStore)(nil)
)

func newFakeStore() *fakeStore {
	return &fakeStore{
		convIdx:          make(map[string]int),
		parts:            make(map[string][]*conversation.Participant),
		msgs:             make(map[string][]conversation.Message),
		cursors:          make(map[string]map[string]conversation.Cursor),
		providerSessions: make(map[string]map[string]conversation.ProviderSession),
		inflight:         make(map[string]conversation.InflightTurn),
	}
}

// seedProviderSession 直接塞一条 provider 会话，绕过一整轮 Ask。
// /rules 与 /session rebuild 的用例靠它构造「已经有一条会话」的前提，
// 不必真的驱动一次 Runner.Ask。
func (f *fakeStore) seedProviderSession(ps conversation.ProviderSession) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putProviderSession(ps)
}

// putProviderSession 是 seedProviderSession 与 SetProviderSession 共用的写入，调用方持 f.mu。
func (f *fakeStore) putProviderSession(ps conversation.ProviderSession) {
	if f.providerSessions[ps.ConvID] == nil {
		f.providerSessions[ps.ConvID] = make(map[string]conversation.ProviderSession)
	}
	f.providerSessions[ps.ConvID][ps.AgentID] = ps
}

// seedTurnUsage 直接塞一行 turn_usage，绕过一整轮 Ask/RecordTurn。
// /cost 的命令层用例需要精确控制 CreatedAt 顺序、RoundID、以及各可空字段
// 的 nil/非 nil 形状，直接调 f.RecordUsage 也能达到同样效果（下面就是
// append），这个 helper 只是让调用点读起来像"种数据"而不是"记一轮"，
// 与 seedProviderSession / seedMessage 同一个体例。
func (f *fakeStore) seedTurnUsage(u conversation.TurnUsage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usages = append(f.usages, u)
}

// id 铸下一个 id，调用方持 f.mu。
func (f *fakeStore) id(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s%03d", prefix, f.nextID)
}

// seedConversation 直接塞一个指定 id 的会话，绕过 /new。
// /use 的前缀匹配用例需要两个 id 共享前缀，所以 id 必须由测试指定。
func (f *fakeStore) seedConversation(id, title string) conversation.Conversation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.putConversation(id, title)
}

// putConversation 是 seedConversation 与 CreateConversation 共用的写入，调用方持 f.mu。
func (f *fakeStore) putConversation(id, title string) conversation.Conversation {
	c := conversation.Conversation{ID: id, Title: title, CreatedAt: replTestNow}
	f.convIdx[c.ID] = len(f.convs)
	f.convs = append(f.convs, c)
	return c
}

// seedMessage 直接塞一条消息，绕过 /say。
// /log 的用例靠它证明输出来自库，而不是 /say 自己的回显。
func (f *fakeStore) seedMessage(t *testing.T, convID, senderID string, kind conversation.MessageKind, text string) conversation.Message {
	t.Helper()
	m, err := f.AppendMessage(context.Background(), conversation.Message{
		ConvID:   convID,
		SenderID: senderID,
		Kind:     kind,
		Text:     text,
	})
	if err != nil {
		t.Fatalf("塞入消息失败: %v", err)
	}
	return m
}

// seedParticipant 直接塞一个活跃 participant，绕过 /join。
// /who 的用例靠它证明输出来自库，而不是 /join 的回显。
func (f *fakeStore) seedParticipant(t *testing.T, convID, agentID string) {
	t.Helper()
	err := f.AddParticipant(context.Background(), conversation.Participant{
		ConvID:   convID,
		AgentID:  agentID,
		Provider: "fake",
		JoinedAt: replTestNow,
	})
	if err != nil {
		t.Fatalf("塞入 participant 失败: %v", err)
	}
}

func (f *fakeStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return fn(ctx, f)
}

func (f *fakeStore) Conversation(ctx context.Context, id string) (conversation.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i, ok := f.convIdx[id]; ok {
		return f.convs[i], nil
	}
	return conversation.Conversation{}, fmt.Errorf("fake store: 会话 %q: %w", id, conversation.ErrConversationNotFound)
}

func (f *fakeStore) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]conversation.Conversation, 0, len(f.convs))
	for i := len(f.convs) - 1; i >= 0; i-- {
		out = append(out, f.convs[i])
	}
	return out, nil
}

func (f *fakeStore) Participants(ctx context.Context, convID string, includeInactive bool) ([]conversation.Participant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]conversation.Participant, 0, len(f.parts[convID]))
	for _, p := range f.parts[convID] {
		if !p.Active && !includeInactive {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

func (f *fakeStore) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]conversation.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]conversation.Message, 0)
	for _, m := range f.msgs[convID] {
		if m.Seq <= afterSeq {
			continue
		}
		out = append(out, m)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeStore) MaxSeq(ctx context.Context, convID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.msgs[convID])), nil
}

// RecordUsage 是 Step 7 给 Tx 加的写方法。曾经的注释写着"repl 的用例不走
// RecordTurn，空实现即可"——那时确实没有命令会经 Store 接口读回
// turn_usage。Step 10 D 片之一起改成真正存状态（append 进 f.usages）：
// /cost 的命令层用例需要经 Runner.Ask 之外的路径（seedTurnUsage）或者
// 真实驱动一轮之后，把种下/记下的行经 TurnUsages 读回来验证渲染——继续
// 空实现会让 TurnUsages 恒答"没有"，/cost 的表格类用例全部测不出差别
// （12 契约"恒真的空用例"那条教训，见下面 SetConversationTeam 的注释）。
// 本包目前没有任何既有用例断言"RecordUsage 是空操作"，加了写入不会
// 改变它们的行为。
func (f *fakeStore) RecordUsage(ctx context.Context, u conversation.TurnUsage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usages = append(f.usages, u)
	return nil
}

// SetProviderSession / DropProviderSession / ProviderSession / AllProviderSessions
// 从 Step 8 起是有状态的：/rules 与 /session rebuild 要靠它们构造「已经有
// 一条 provider 会话」的前提、并验证 rebuild 真的把它删掉了。状态就是
// f.providerSessions 这一份，四个方法读写同一份数据，不能有的走「恒答
// 没有」有的却读真实状态——那会让 ProviderSession 与 DropProviderSession
// 对「这一行在不在」给出互相矛盾的答案。
func (f *fakeStore) SetProviderSession(ctx context.Context, ps conversation.ProviderSession) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putProviderSession(ps)
	return nil
}

func (f *fakeStore) DropProviderSession(ctx context.Context, convID, agentID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	byAgent := f.providerSessions[convID]
	if byAgent == nil {
		return false, nil
	}
	_, existed := byAgent[agentID]
	delete(byAgent, agentID)
	return existed, nil
}

// 下面四个是 Step 10 给 Tx 与 Store 加的方法（inflight_turns 那一片）。
// 曾经的注释写着「repl 包的用例不走 RecordTurn 的 expectInflight 分支
// （/say 等命令走 Service.Post，不是 RecordTurn），全部是空桩，不额外
// 造状态」——这句话在之二写下时是真的（那时 expectInflight 三处硬编码
// false）。之三接上 BeginTurn 之后它变假了：本包任何一条驱动 /ask 的用例
// 都会经 Runner.Ask -> Service.RecordTurn 传 expectInflight=true，
// 四个方法因此必须是有状态的实现，理由见上面 inflight 字段的注释。
func (f *fakeStore) BeginTurn(ctx context.Context, t conversation.InflightTurn) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inflight[t.TurnID] = t
	return nil
}

// SetInflightPhase 照 Tx 接口的契约（store.go 原话「turn_id 不存在时报错，
// 不静默通过」）：查不到就返回 conversation.ErrInflightMissing，不是恒定
// 成功——静默通过会让「状态已经丢了」这种本该响亮的错误在本包的测试里
// 永远测不出来。
func (f *fakeStore) SetInflightPhase(ctx context.Context, turnID string, phase conversation.InflightPhase) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	it, ok := f.inflight[turnID]
	if !ok {
		return fmt.Errorf("fake store: 在飞记录 %q: %w", turnID, conversation.ErrInflightMissing)
	}
	it.Phase = phase
	f.inflight[turnID] = it
	return nil
}

// ClearInflight 如实报告删之前这一行在不在，与 DropProviderSession 同体例：
// 不能对「这一行在不在」给出两个互相矛盾的答案。existed 如实为 false 时，
// Service.RecordTurn 会不会因此报 ErrInflightMissing 完全由调用方传的
// expectInflight 决定，本假实现不替调用方做这个判断。
func (f *fakeStore) ClearInflight(ctx context.Context, turnID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, existed := f.inflight[turnID]
	delete(f.inflight, turnID)
	return existed, nil
}

// InflightFor 为 Store.InflightFor 补的方法，读同一份 inflight map。
func (f *fakeStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range f.inflight {
		if it.ConvID == convID && it.AgentID == agentID {
			return it, true, nil
		}
	}
	return conversation.InflightTurn{}, false, nil
}

// AllInflight 按插入顺序返回即可：本包目前没有依赖顺序的用例，真实实现
// 按 started_at 升序是给恢复扫描用的，不是本假实现要模仿的重点。
func (f *fakeStore) AllInflight(ctx context.Context) ([]conversation.InflightTurn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]conversation.InflightTurn, 0, len(f.inflight))
	for _, it := range f.inflight {
		out = append(out, it)
	}
	return out, nil
}

// SetConversationTeam 是 Step 9 片 2a 给 Tx 加的写方法（/team use、
// Service.BindTeam 用）。第二次改：第一版只记账、从不回写 f.convs，
// 那同样是一条测不出问题的假路——Conversation() 读的是 f.convs，只记账
// 不回写的话，「绑定生效了没有」这类断言对它恒真（片 2c 测试方指出）。
// 现在两件都做：记账证明「调用发生过、参数对」，回写证明「效果读得回来」。
// convID 查不到、teamID 为空两种情况都返回错误而不是静默成功，
// 与真实现 internal/store/tx.go 的 SetConversationTeam 同体例——
// 假实现不该比真实现松。
func (f *fakeStore) SetConversationTeam(ctx context.Context, convID, teamID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if teamID == "" {
		return fmt.Errorf("fake store: set conversation team: %w", conversation.ErrEmptyTeamID)
	}
	i, ok := f.convIdx[convID]
	if !ok {
		return fmt.Errorf("fake store: 会话 %q: %w", convID, conversation.ErrConversationNotFound)
	}
	f.setConversationTeamCalls = append(f.setConversationTeamCalls, conversationTeamCall{
		convID: convID, teamID: teamID,
	})
	f.convs[i].TeamID = teamID
	return nil
}

// SetConversationCwd 与真实现同体例，写回 f.convs 才读得回来；只记账的假实现对「盖章生效了没有」恒真。
func (f *fakeStore) SetConversationCwd(ctx context.Context, convID, cwd string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i, ok := f.convIdx[convID]
	if !ok {
		return fmt.Errorf("fake store: 会话 %q: %w", convID, conversation.ErrConversationNotFound)
	}
	f.convs[i].Cwd = cwd
	return nil
}

func (f *fakeStore) ProviderSession(ctx context.Context, convID, agentID string) (conversation.ProviderSession, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	byAgent := f.providerSessions[convID]
	if byAgent == nil {
		return conversation.ProviderSession{}, false, nil
	}
	ps, ok := byAgent[agentID]
	return ps, ok, nil
}

// AllProviderSessions 按 (conv_id, agent_id) 升序返回，与 internal/store
// 真实实现的顺序保持一致，测试才能同样依赖这个顺序。
func (f *fakeStore) AllProviderSessions(ctx context.Context) ([]conversation.ProviderSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]conversation.ProviderSession, 0)
	for _, byAgent := range f.providerSessions {
		for _, ps := range byAgent {
			out = append(out, ps)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConvID != out[j].ConvID {
			return out[i].ConvID < out[j].ConvID
		}
		return out[i].AgentID < out[j].AgentID
	})
	return out, nil
}

// UsageSamples 只为满足接口而存在，与 TurnUsages 不走同一条路：本包目前
// 没有命令会调用预报路径（/round 的 renderBudget 直接吃合成数据，见
// round.go 里那条注释——"internal/repl 的假 Store 让 UsageSamples 返回
// nil, nil"），所以继续恒答"没有"是安全的。RecordUsage 从 Step 10 D 片
// 之一起已经真正存状态（见 f.usages），若哪天有用例要连 UsageSamples 也
// 读到这份状态，要在这里补过滤 + 排序逻辑，不能继续恒答 nil——那样会把
// "没读到"和"压根没记过"混成一回事。
func (f *fakeStore) UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

// TurnUsages 是 Step 10 D 片之一给 Store 加的读方法（/cost 用），按
// (conv_id, agent_id) 过滤 + created_at/turn_id 升序 + limit 语义，与
// internal/store 真实实现的共享签名同构（冻结契约 3 节）。这是本文件里
// 唯一需要读回 turn_usage 的路径——上面的 UsageSamples 不需要——所以只有
// 这一个方法做真正的过滤/排序，RecordUsage 与 seedTurnUsage 共用同一份
// f.usages 状态。
//
// 失效条件：这份状态只由 RecordUsage 与 seedTurnUsage 两处写入；哪天本包
// 出现第三处直接操纵 turn_usage 形状的写法，要跟着一起改，否则会出现
// "这一行在库里还是不在"的两个矛盾答案——与本文件 providerSessions /
// inflight 两段注释是同一条纪律。
func (f *fakeStore) TurnUsages(ctx context.Context, convID, agentID string, limit int) ([]conversation.TurnUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit == 0 {
		return []conversation.TurnUsage{}, nil
	}
	matched := make([]conversation.TurnUsage, 0, len(f.usages))
	for _, u := range f.usages {
		if u.ConvID != convID {
			continue
		}
		if agentID != "" && u.AgentID != agentID {
			continue
		}
		matched = append(matched, u)
	}
	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].CreatedAt.Before(matched[j].CreatedAt)
		}
		return matched[i].TurnID < matched[j].TurnID
	})
	// limit > 0 取"最近"这么多条，而结果要求是升序——取的是排序之后的
	// 末尾 limit 条，不是开头 limit 条。与 internal/store 真实实现同一条
	// 语义（冻结契约 4.1）。limit < 0 不限，上面已经是全量。
	if limit > 0 && len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}
	return matched, nil
}

func (f *fakeStore) Cursor(ctx context.Context, convID, agentID string) (conversation.Cursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if byAgent, ok := f.cursors[convID]; ok {
		if c, ok := byAgent[agentID]; ok {
			return c, nil
		}
	}
	return conversation.Cursor{}, fmt.Errorf("fake store: 游标 (%s, %s): %w", convID, agentID, conversation.ErrNoCursor)
}

func (f *fakeStore) Cursors(ctx context.Context, convID string) ([]conversation.Cursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	byAgent := f.cursors[convID]
	ids := make([]string, 0, len(byAgent))
	for id := range byAgent {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]conversation.Cursor, 0, len(ids))
	for _, id := range ids {
		out = append(out, byAgent[id])
	}
	return out, nil
}

func (f *fakeStore) CreateConversation(ctx context.Context, title string) (conversation.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.putConversation(f.id("conv"), title), nil
}

func (f *fakeStore) AppendMessage(ctx context.Context, m conversation.Message) (conversation.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.convIdx[m.ConvID]; !ok {
		return conversation.Message{}, fmt.Errorf("fake store: 追加消息到会话 %q: %w", m.ConvID, conversation.ErrConversationNotFound)
	}
	m.ID = f.id("msg")
	m.Seq = int64(len(f.msgs[m.ConvID])) + 1
	m.CreatedAt = replTestNow
	if m.Recipients == nil {
		m.Recipients = []string{}
	}
	if m.Meta == nil {
		m.Meta = map[string]any{}
	}
	f.msgs[m.ConvID] = append(f.msgs[m.ConvID], m)
	return m, nil
}

func (f *fakeStore) AddParticipant(ctx context.Context, p conversation.Participant) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p.ConvID == "" {
		return errors.New("fake store: AddParticipant 收到空的 ConvID")
	}
	for _, ex := range f.parts[p.ConvID] {
		if ex.AgentID != p.AgentID {
			continue
		}
		ex.Active = true
		ex.LeftAt = nil
		ex.JoinedAt = p.JoinedAt
		if p.DisplayName != "" {
			ex.DisplayName = p.DisplayName
		}
		if p.Provider != "" {
			ex.Provider = p.Provider
		}
		return nil
	}
	fresh := p
	fresh.Active = true
	fresh.LeftAt = nil
	f.parts[p.ConvID] = append(f.parts[p.ConvID], &fresh)
	return nil
}

func (f *fakeStore) DeactivateParticipant(ctx context.Context, convID, agentID string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ex := range f.parts[convID] {
		if ex.AgentID != agentID {
			continue
		}
		ex.Active = false
		left := at
		ex.LeftAt = &left
		return nil
	}
	return fmt.Errorf("fake store: 退出 (%s, %s): %w", convID, agentID, conversation.ErrParticipantNotFound)
}

// RequireActiveParticipant 判「这一行存在且 Active」，与真 store 的 SQL 条件一致。
func (f *fakeStore) RequireActiveParticipant(ctx context.Context, convID, agentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ex := range f.parts[convID] {
		if ex.AgentID == agentID && ex.Active {
			return nil
		}
	}
	return fmt.Errorf("fake store: 活跃成员 (%s, %s): %w", convID, agentID, conversation.ErrParticipantNotFound)
}

func (f *fakeStore) SetCursor(ctx context.Context, c conversation.Cursor) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cursors[c.ConvID] == nil {
		f.cursors[c.ConvID] = make(map[string]conversation.Cursor)
	}
	if c.Pending == nil {
		c.Pending = []int64{}
	}
	f.cursors[c.ConvID][c.AgentID] = c
	return nil
}

// totalMessages 数所有会话里的消息总数，用来证明某条命令没有偷偷写进去。
func (f *fakeStore) totalMessages() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, list := range f.msgs {
		n += len(list)
	}
	return n
}

// newStepFiveCommands 是 Step 5 新增的八个命令，连同一份够用的参数。
// 参数取自 newTestREPLConfig 里的 agent id，这样 /join 与 /leave 不会先撞上「配置里没有这个 agent」。
func newStepFiveCommands() []struct {
	name string
	line string
} {
	return []struct {
		name string
		line string
	}{
		{name: "new", line: "/new 新会话"},
		{name: "conv", line: "/conv"},
		{name: "use", line: "/use conv001"},
		{name: "join", line: "/join ag_alpha"},
		{name: "leave", line: "/leave ag_alpha"},
		{name: "who", line: "/who"},
		{name: "say", line: "/say 一句话"},
		{name: "log", line: "/log"},
	}
}

// runREPLSvc 起一个带 Service 的 REPL 跑到结束，带看门狗。
// svc 传 nil 就是「持久化未就绪」的那条路径。
func runREPLSvc(t *testing.T, svc *conversation.Service, input string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	// rn 传 nil：本用例一条 provider 命令都不碰，/ask 与 /session 在这里
	// 一律答「provider 调用未就绪」。真正覆盖它们的用例在 ask_test.go。
	r := New(newTestREPLConfig(), svc, nil, strings.NewReader(input), &out, RulesContext{})

	done := make(chan error, 1)
	go func() {
		// Run 在另一个 goroutine 里跑，它 panic 的话本测试进程会直接挂掉，
		// 看不出是哪条命令的问题。这里把 panic 收成一个普通错误交回来，
		// 调用方原本就断言「Run 返回 nil」，于是 panic 会变成一条可读的失败。
		defer func() {
			if rec := recover(); rec != nil {
				done <- fmt.Errorf("REPL.Run panic: %v", rec)
			}
		}()
		done <- r.Run(context.Background())
	}()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(testREPLTimeout):
		t.Fatalf("REPL.Run 在 %v 内没有返回", testREPLTimeout)
		return "", nil
	}
}

// newTestService 造一个跑在内存假实现上的 Service。
func newTestService(t *testing.T) (*conversation.Service, *fakeStore) {
	t.Helper()
	f := newFakeStore()
	return conversation.NewService(f), f
}

// TestStepFiveCommandsWithoutService 锁住契约里的兜底：svc 为 nil 时，
// 八个新命令都要给出答复、不 panic、Run 仍然返回 nil。
//
// 断言不碰那句话的措辞（08a 的「不属于契约面的东西」把措辞排除在外），只锁两件事：
//  1. 确实产生了输出（和「一个空行」比字节数，两者的提示符数量相同）；
//  2. 输出不等于未知命令的输出，也就是命令确实被认出来了、走了自己的兜底分支。
func TestStepFiveCommandsWithoutService(t *testing.T) {
	blankLine, err := runREPLSvc(t, nil, "\n")
	if err != nil {
		t.Fatalf("空行输入下 Run 应当返回 nil，实际: %v", err)
	}
	unknown, err := runREPLSvc(t, nil, "/zz_not_a_command\n")
	if err != nil {
		t.Fatalf("未知命令下 Run 应当返回 nil，实际: %v", err)
	}

	for _, tc := range newStepFiveCommands() {
		t.Run(tc.name, func(t *testing.T) {
			// runREPLSvc 会把 Run 里的 panic 收成错误，所以下面这句 err 断言
			// 同时覆盖了「不 panic」与「返回 nil」两条。
			out, err := runREPLSvc(t, nil, tc.line+"\n")
			if err != nil {
				t.Fatalf("svc 为 nil 时 Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
			}
			if len(out) <= len(blankLine) {
				t.Errorf("%q 没有产生任何答复；输出:\n%s", tc.line, out)
			}
			if out == unknown {
				t.Errorf("%q 被当成了未知命令，说明它没有注册进命令表；输出:\n%s", tc.line, out)
			}
		})
	}
}

// TestStepFiveCommandsPanicSafety 把八个命令一次全喂进去，证明 Run 不 panic、
// 读完输入后仍然正常结束。上一个用例是逐条跑的，这一条锁的是它们混在一起也不炸。
func TestStepFiveCommandsPanicSafety(t *testing.T) {
	var input strings.Builder
	for _, tc := range newStepFiveCommands() {
		input.WriteString(tc.line + "\n")
	}

	if _, err := runREPLSvc(t, nil, input.String()); err != nil {
		t.Fatalf("svc 为 nil 时连续执行八个新命令应当平安返回 nil，实际: %v", err)
	}
}

// TestNewSayLogRoundTrip 是本步 REPL 的主干路径：
// /new 建会话并切过去 -> /join 把配置里的 agent 加进来 -> /say 以 user 身份发言 -> /log 能看到。
//
// 输出断言只用关键片段；真正的证据是假实现里落下的那条消息，
// 这样即便 /say 自己回显了文本，断言也不会变空。
func TestNewSayLogRoundTrip(t *testing.T) {
	svc, f := newTestService(t)

	const marker = "hello-from-user"
	out, err := runREPLSvc(t, svc, "/new <project> 讨论\n/join ag_alpha\n/who\n/say "+marker+"\n/log\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	// /new 走的是假实现的 CreateConversation，第一个 id 是 conv001。
	const convID = "conv001"
	if _, err := f.Conversation(context.Background(), convID); err != nil {
		t.Fatalf("/new 没有建出会话: %v；输出:\n%s", err, out)
	}

	msgs, err := f.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("读假实现里的消息失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("/say 应当正好写入 1 条消息，实际 %d 条；输出:\n%s", len(msgs), out)
	}
	got := msgs[0]
	if got.Text != marker {
		t.Errorf("写入的正文是 %q，应当是 %q", got.Text, marker)
	}
	if got.SenderID != conversation.ReservedSenderUser {
		t.Errorf("/say 的 sender 是 %q，应当是保留的 %q", got.SenderID, conversation.ReservedSenderUser)
	}
	if got.Kind != conversation.KindUser {
		t.Errorf("/say 的 kind 是 %q，应当是 %q", string(got.Kind), string(conversation.KindUser))
	}
	if got.Seq != 1 {
		t.Errorf("第一条消息的 seq 是 %d，应当是 1", got.Seq)
	}

	// /join 的证据同样取自假实现，不依赖 /who 的排版。
	parts, err := f.Participants(context.Background(), convID, false)
	if err != nil {
		t.Fatalf("读假实现里的 participant 失败: %v", err)
	}
	if len(parts) != 1 || parts[0].AgentID != "ag_alpha" || !parts[0].Active {
		t.Fatalf("/join 之后应当有一个活跃的 ag_alpha，实际: %+v；输出:\n%s", parts, out)
	}

	// 输出侧只要求 /log 把正文显示出来、/who 把 agent id 显示出来。
	for _, want := range []string{marker, "ag_alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出里缺少 %q；完整输出:\n%s", want, out)
		}
	}
	// /log 不显示 thought（规格 H）。这里的消息本来就没有 thought，
	// 所以只能锁住一件事：正文出现过。thought 的过滤由 Step 8 的开关再验。
}

// TestLogShowsAppendedOrder 断言 /log 按 seq 升序显示，而不是倒序。
func TestLogShowsAppendedOrder(t *testing.T) {
	svc, _ := newTestService(t)

	out, err := runREPLSvc(t, svc, "/new 顺序\n/say first-line\n/say second-line\n/say third-line\n/log\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	// 三条正文都出现在 /log 的输出里，且相对顺序是写入顺序。
	// 用最后一次出现的位置比较：前面的 /say 可能各自回显过一次。
	prev := -1
	for _, want := range []string{"first-line", "second-line", "third-line"} {
		at := strings.LastIndex(out, want)
		if at < 0 {
			t.Fatalf("输出里缺少 %q；完整输出:\n%s", want, out)
		}
		if at <= prev {
			t.Fatalf("%q 的位置 %d 没有排在前一条之后，/log 应当按 seq 升序；完整输出:\n%s", want, at, out)
		}
		prev = at
	}
}

// TestWhoReadsFromStore 证明 /who 的输出来自库。
// /who 是八个新命令里唯一只能从输出上看结果的（其余几个都能去假实现里查状态），
// 所以这里的成员是直接塞进去的，没有 /join 回显过它们的 id。
func TestWhoReadsFromStore(t *testing.T) {
	svc, f := newTestService(t)
	conv := f.seedConversation("aa1"+"00000000000000000000000000000", "已有会话")
	f.seedParticipant(t, conv.ID, "ag_alpha")
	f.seedParticipant(t, conv.ID, "ag_zeta")

	out, err := runREPLSvc(t, svc, "/use aa1\n/who\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}
	for _, want := range []string{"ag_alpha", "ag_zeta"} {
		if !strings.Contains(out, want) {
			t.Errorf("/who 的输出里缺少成员 %q；完整输出:\n%s", want, out)
		}
	}
	// 规格 G.3：user 不是 participant，不出现在 /who 里。
	// 只有在没有任何成员叫这个名字时这条断言才有意义，上面两个 id 都不含 "user"。
	if strings.Contains(out, conversation.ReservedSenderUser) {
		t.Errorf("/who 不应当把保留的 %q 当成 participant 列出来；完整输出:\n%s",
			conversation.ReservedSenderUser, out)
	}
}

// TestLogReadsFromStore 证明 /log 的输出来自库。
// 上一个用例里那条消息是 /say 刚发的，实现若只回显 /say 而 /log 什么都不打，
// 断言也会通过；这里的两条消息是直接塞进假实现的，没有任何命令回显过它们，
// 所以它们只能由 /log 读出来。
func TestLogReadsFromStore(t *testing.T) {
	svc, f := newTestService(t)
	conv := f.seedConversation("aa1"+"00000000000000000000000000000", "已有会话")
	f.seedMessage(t, conv.ID, conversation.ReservedSenderUser, conversation.KindUser, "seeded-line-1")
	f.seedMessage(t, conv.ID, "ag_alpha", conversation.KindAgent, "seeded-line-2")

	out, err := runREPLSvc(t, svc, "/use aa1\n/log\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	first := strings.Index(out, "seeded-line-1")
	second := strings.Index(out, "seeded-line-2")
	if first < 0 || second < 0 {
		t.Fatalf("/log 应当打印库里已有的两条消息；完整输出:\n%s", out)
	}
	if first > second {
		t.Errorf("/log 应当按 seq 升序打印，实际 seq=1 的那条出现在后面；完整输出:\n%s", out)
	}
	// 规格 H：/log 显示 seq、sender、kind、文本。这里只认 sender，
	// seq 与 kind 的呈现形式（数字还是别的写法、列名叫什么）不属于契约面。
	if !strings.Contains(out, "ag_alpha") {
		t.Errorf("/log 应当显示发言者，输出里找不到 ag_alpha；完整输出:\n%s", out)
	}
}

// TestCommandsWithoutCurrentConversation 锁住规格 H 的兜底：
// 没有当前会话时，/say /log /who /join /leave 给提示，而不是报错退出、也不是当成未知命令。
func TestCommandsWithoutCurrentConversation(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{name: "say", line: "/say 没人听得见"},
		{name: "log", line: "/log"},
		{name: "who", line: "/who"},
		{name: "join", line: "/join ag_alpha"},
		{name: "leave", line: "/leave ag_alpha"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, f := newTestService(t)
			// 库里有会话，但 REPL 没有 /new 也没有 /use，所以「当前会话」仍然是空的。
			f.seedConversation("seeded0000000000000000000000001", "已存在的会话")

			blankLine, err := runREPLSvc(t, svc, "\n")
			if err != nil {
				t.Fatalf("空行输入下 Run 应当返回 nil，实际: %v", err)
			}
			unknown, err := runREPLSvc(t, svc, "/zz_not_a_command\n")
			if err != nil {
				t.Fatalf("未知命令下 Run 应当返回 nil，实际: %v", err)
			}

			out, err := runREPLSvc(t, svc, tc.line+"\n")
			if err != nil {
				t.Fatalf("没有当前会话时 Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
			}
			if len(out) <= len(blankLine) {
				t.Errorf("%q 没有给出任何提示；输出:\n%s", tc.line, out)
			}
			if out == unknown {
				t.Errorf("%q 被当成了未知命令；输出:\n%s", tc.line, out)
			}
			if n := f.totalMessages(); n != 0 {
				t.Errorf("没有当前会话时不应当写入消息，实际写了 %d 条", n)
			}
		})
	}
}

// TestUsePrefixMatching 覆盖 /use 的两种前缀情形。
// 两个种子会话的 id 共享前缀 "aa"，各自的第三个字符不同，
// 所以断言只认 "aa1" / "aa2" 这三个字符，不依赖实现是打全 id 还是打短 id。
func TestUsePrefixMatching(t *testing.T) {
	const (
		idA = "aa1" + "00000000000000000000000000000"
		idB = "aa2" + "00000000000000000000000000000"
	)

	t.Run("前缀不唯一时报错并列出候选", func(t *testing.T) {
		svc, f := newTestService(t)
		f.seedConversation(idA, "会话甲")
		f.seedConversation(idB, "会话乙")

		out, err := runREPLSvc(t, svc, "/use aa\n/say 这条不该被写进去\n")
		if err != nil {
			t.Fatalf("前缀不唯一时 Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
		}
		for _, want := range []string{"aa1", "aa2"} {
			if !strings.Contains(out, want) {
				t.Errorf("候选列表里缺少 %q；完整输出:\n%s", want, out)
			}
		}
		// 没选中就不该有当前会话，后面那条 /say 必须落空。
		if n := f.totalMessages(); n != 0 {
			t.Errorf("/use 前缀不唯一时不应当切到任何会话，但随后的 /say 写入了 %d 条消息", n)
		}
	})

	t.Run("前缀唯一时切过去", func(t *testing.T) {
		svc, f := newTestService(t)
		f.seedConversation(idA, "会话甲")
		f.seedConversation(idB, "会话乙")

		const marker = "landed-in-a"
		out, err := runREPLSvc(t, svc, "/use aa1\n/say "+marker+"\n")
		if err != nil {
			t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
		}

		msgs, err := f.Messages(context.Background(), idA, 0, 0)
		if err != nil {
			t.Fatalf("读假实现里的消息失败: %v", err)
		}
		if len(msgs) != 1 || msgs[0].Text != marker {
			t.Fatalf("/use aa1 之后的 /say 应当落进会话 %s，实际: %+v；输出:\n%s", idA, msgs, out)
		}
		if n := f.totalMessages(); n != 1 {
			t.Errorf("消息总数应当是 1，实际 %d，说明还写进了别的会话", n)
		}
	})
}

// TestConvListsConversations 断言 /conv 能把库里的会话列出来。
// 只认 id 的可辨识片段，不认列名与排版（08a 把它们排除在契约之外）。
func TestConvListsConversations(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("aa1"+"00000000000000000000000000000", "会话甲")
	f.seedConversation("aa2"+"00000000000000000000000000000", "会话乙")

	out, err := runREPLSvc(t, svc, "/conv\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}
	for _, want := range []string{"aa1", "aa2"} {
		if !strings.Contains(out, want) {
			t.Errorf("/conv 的输出里缺少会话 %q；完整输出:\n%s", want, out)
		}
	}
}

// TestJoinRejectsUnknownAgent 断言 /join 一个配置里没有的 agent 要报错，
// 而且不会把它塞进 participant 表。
func TestJoinRejectsUnknownAgent(t *testing.T) {
	svc, f := newTestService(t)

	out, err := runREPLSvc(t, svc, "/new 会话\n/join ag_not_in_config\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	parts, err := f.Participants(context.Background(), "conv001", true)
	if err != nil {
		t.Fatalf("读假实现里的 participant 失败: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("配置里没有的 agent 不应当被加进会话，实际: %+v；输出:\n%s", parts, out)
	}
}

// TestLeaveIsSoftDelete 断言 /leave 之后消息还在（规格 G.2 的软删除）。
func TestLeaveIsSoftDelete(t *testing.T) {
	svc, f := newTestService(t)

	const marker = "said-before-leaving"
	out, err := runREPLSvc(t, svc, "/new 会话\n/join ag_alpha\n/say "+marker+"\n/leave ag_alpha\n/log\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	msgs, err := f.Messages(context.Background(), "conv001", 0, 0)
	if err != nil {
		t.Fatalf("读假实现里的消息失败: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Text != marker {
		t.Fatalf("退出不应当删掉消息，实际: %+v；输出:\n%s", msgs, out)
	}
	if !strings.Contains(out, marker) {
		t.Errorf("/leave 之后 /log 仍然应当读得到那条消息；完整输出:\n%s", out)
	}

	parts, err := f.Participants(context.Background(), "conv001", true)
	if err != nil {
		t.Fatalf("读假实现里的 participant 失败: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("含已退出在内应当有 1 个 participant，实际: %+v", parts)
	}
	if parts[0].Active {
		t.Errorf("/leave 之后 active 应当为 false，实际: %+v", parts[0])
	}
	if parts[0].LeftAt == nil {
		t.Errorf("/leave 之后 left_at 不应当为空，实际: %+v", parts[0])
	}
}
