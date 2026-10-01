package conversation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"
)

// testNow 是全部测试用的固定时刻，UTC。
var testNow = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

// messagesCall 记录一次 Store.Messages 调用的参数，供「转调 Store」的用例断言。
type messagesCall struct {
	convID   string
	afterSeq int64
	limit    int
}

// setConversationTeamCall 记录一次 Tx.SetConversationTeam 调用收到的参数
// （Step 9 片 2a 新增，供 BindTeam 相关用例核对调用发生过、且参数对）。
type setConversationTeamCall struct {
	convID string
	teamID string
}

// fakeStore 是 Store 与 Tx 的内存实现，只在本包的测试里用。
//
// 领域层的测试**不导入 internal/store**：那会把 conversation 的测试绑到 SQLite 上，
// 也违反「internal/store 导入 internal/conversation，反向不导入」这条分层约束。
//
// 这个假实现刻意在三处模仿真实 store 的行为，否则相关用例会退化成空断言：
//  1. Participants 按插入顺序返回，不排序 -> Recipients 的字典序只能由 Service 自己保证。
//  2. AddParticipant 对已存在的 (conv, agent) 是重新激活而不是插第二行（规格 G.1）。
//  3. Participants 对不存在的会话返回空切片而不是错误 -> Post 的「会话不存在」
//     只能靠 Service 显式调 Conversation 查出来，不能靠 Participants 顺带报错。
//
// 没有加锁：所有用例都在单个 goroutine 里驱动它。repl 包那份副本不同：/round 起多个 worker
// 并发读写，2026-09-25 起加了锁（internal/repl/conversation_test.go）。
type fakeStore struct {
	convs   []Conversation
	convIdx map[string]int
	parts   map[string][]*Participant
	msgs    map[string][]Message
	cursors map[string]map[string]Cursor

	nextID int

	writeTxCalls  int
	createTitles  []string
	messagesCalls []messagesCall

	// setConversationTeamCalls 记 Step 9 片 2a 新增的 Tx.SetConversationTeam
	// 每一次调用；本假实现不维护 conv.TeamID 的回读（Conversation 本身还没有
	// 那个字段，加不加由片 2a 的冻结形状决定，不是这份假实现该猜的事），
	// 只负责记账，让调用序列/参数类的断言有东西可查。
	setConversationTeamCalls []setConversationTeamCall

	// inflight 是本假实现维护的在飞记录状态，键是 turn_id。下面
	// BeginTurn/SetInflightPhase/ClearInflight/AllInflight 那组方法的旧
	// 注释写着"本文件的用例一条都不走 RecordTurn 的 expectInflight 分支
	// ……有状态的假实现由下一刀接 BeginTurn 调用点的测试另写"——那个
	// "下一刀"就是 Step 10 A 片之三，本条订正顺手把它补上，不是因为本包
	// 现在有用例会踩到（本包确实一条都不传 expectInflight=true，参见
	// internal/store/inflight_recordturn_test.go 文件头的说明：真正需要
	// 观察回滚效果的用例特意放在 internal/store，因为本包的 WriteTx 不做
	// 真实回滚），而是让这四个方法不再是一条"answer恒为没有"的死路——
	// internal/repl 那份同体例的假实现之前就是保持空桩才在这一刀踩了红
	// （四条用例：TestConfigReloadApplyWarnsOnProviderCommandChangeWithZeroDrift
	// 等）。对本包现有用例是安全的：没有任何现存用例调用过 BeginTurn，
	// 所以 ClearInflight 对它们仍然如实报 existed=false，行为与改之前
	// 逐字节相同。
	inflight map[string]InflightTurn
}

var (
	_ Store = (*fakeStore)(nil)
	_ Tx    = (*fakeStore)(nil)
)

func newFakeStore() *fakeStore {
	return &fakeStore{
		convIdx:  make(map[string]int),
		parts:    make(map[string][]*Participant),
		msgs:     make(map[string][]Message),
		cursors:  make(map[string]map[string]Cursor),
		inflight: make(map[string]InflightTurn),
	}
}

// id 生成确定性 id。真实 store 用 crypto/rand，这里要的是可预期。
func (f *fakeStore) id(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s%03d", prefix, f.nextID)
}

func (f *fakeStore) WriteTx(ctx context.Context, fn func(context.Context, Tx) error) error {
	f.writeTxCalls++
	// 真实实现会在 fn 返回错误时回滚。这里不做回滚：
	// 事务语义属于 internal/store 的测试面，本包只关心 Service 有没有走 WriteTx。
	return fn(ctx, f)
}

func (f *fakeStore) Conversation(ctx context.Context, id string) (Conversation, error) {
	if i, ok := f.convIdx[id]; ok {
		return f.convs[i], nil
	}
	return Conversation{}, fmt.Errorf("fake store: 会话 %q: %w", id, ErrConversationNotFound)
}

func (f *fakeStore) Conversations(ctx context.Context) ([]Conversation, error) {
	// 规格 F：按 created_at 倒序。
	out := make([]Conversation, 0, len(f.convs))
	for i := len(f.convs) - 1; i >= 0; i-- {
		out = append(out, f.convs[i])
	}
	return out, nil
}

func (f *fakeStore) Participants(ctx context.Context, convID string, includeInactive bool) ([]Participant, error) {
	out := make([]Participant, 0, len(f.parts[convID]))
	for _, p := range f.parts[convID] {
		if !p.Active && !includeInactive {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

func (f *fakeStore) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]Message, error) {
	f.messagesCalls = append(f.messagesCalls, messagesCall{convID: convID, afterSeq: afterSeq, limit: limit})
	out := make([]Message, 0)
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
	return int64(len(f.msgs[convID])), nil
}

// ProviderSession 是 Step 7 给 Store 加的读方法。本假实现恒答「没有」：
// 现有用例测的是 Post 与读路径，不涉及会话读回。真正覆盖它的用例
// 由 Step 7 的测试另写，那时这个桩要换成有状态的实现。
// 下面三个是 Step 7 给 Tx 加的写方法。本假实现同时扮演 Store 与 Tx，
// 而现有用例一条都不走 RecordTurn，所以三个都是空实现。
// 有状态的假实现由 Step 7 的测试另写。
func (f *fakeStore) RecordUsage(ctx context.Context, u TurnUsage) error { return nil }

func (f *fakeStore) SetProviderSession(ctx context.Context, ps ProviderSession) error { return nil }

// DropProviderSession 恒答「本来就不存在」：本假实现从不维护 provider 会话
// 状态（ProviderSession 恒答「没有」），existed 恒为 false 与它保持一致，
// 不然两个方法会对「这一行在不在」给出互相矛盾的答案。
func (f *fakeStore) DropProviderSession(ctx context.Context, convID, agentID string) (bool, error) {
	return false, nil
}

// 下面四个是 Step 10 给 Tx 与 Store 加的方法（inflight_turns 那一片）。
// 曾经的注释写着"本文件的用例一条都不走 RecordTurn 的 expectInflight
// 分支……有状态的假实现由下一刀接 BeginTurn 调用点的测试另写"——
// Step 10 A 片之三就是那个"下一刀"，理由见上面 inflight 字段的注释。
func (f *fakeStore) BeginTurn(ctx context.Context, t InflightTurn) error {
	f.inflight[t.TurnID] = t
	return nil
}

// SetInflightPhase 照 Tx 接口的契约（store.go 原话"turn_id 不存在时报错，
// 不静默通过"）：查不到就返回 ErrInflightMissing，不是恒定成功。
func (f *fakeStore) SetInflightPhase(ctx context.Context, turnID string, phase InflightPhase) error {
	it, ok := f.inflight[turnID]
	if !ok {
		return fmt.Errorf("fake store: 在飞记录 %q: %w", turnID, ErrInflightMissing)
	}
	it.Phase = phase
	f.inflight[turnID] = it
	return nil
}

// ClearInflight 如实报告删之前这一行在不在，与 DropProviderSession 同体例。
func (f *fakeStore) ClearInflight(ctx context.Context, turnID string) (bool, error) {
	_, existed := f.inflight[turnID]
	delete(f.inflight, turnID)
	return existed, nil
}

// InflightFor 为 Store.InflightFor 补的方法，读同一份 inflight map。
func (f *fakeStore) InflightFor(ctx context.Context, convID, agentID string) (InflightTurn, bool, error) {
	for _, it := range f.inflight {
		if it.ConvID == convID && it.AgentID == agentID {
			return it, true, nil
		}
	}
	return InflightTurn{}, false, nil
}

// AllInflight 按插入顺序返回：本包没有依赖顺序的用例。
func (f *fakeStore) AllInflight(ctx context.Context) ([]InflightTurn, error) {
	out := make([]InflightTurn, 0, len(f.inflight))
	for _, it := range f.inflight {
		out = append(out, it)
	}
	return out, nil
}

// SetConversationTeam 是 Step 9 片 2a 给 Tx 加的写方法（Service.BindTeam 用）。
// 与上面几个「Step 7 空桩」不同，这里有记账：不写成
// `func (...) SetConversationTeam(...) error { return nil }`——那是一条
// 永远测不出问题的假路，本项目已经为「恒真的空用例」付过账
// （见 12 契约 3.9 第 5 条任务书原话）。
func (f *fakeStore) SetConversationTeam(ctx context.Context, convID, teamID string) error {
	f.setConversationTeamCalls = append(f.setConversationTeamCalls, setConversationTeamCall{
		convID: convID, teamID: teamID,
	})
	return nil
}

func (f *fakeStore) ProviderSession(ctx context.Context, convID, agentID string) (ProviderSession, bool, error) {
	return ProviderSession{}, false, nil
}

// AllProviderSessions 与上面的 ProviderSession 一样恒答「没有」：
// 本假实现从不维护 provider 会话的状态，nil 与「查过、库里确实空」在这里
// 是同一件事，编一份假数据出来反而会跟 ProviderSession 的恒定应答对不上。
func (f *fakeStore) AllProviderSessions(ctx context.Context) ([]ProviderSession, error) {
	return nil, nil
}

// UsageSamples 只为满足接口而存在，与上面 AllProviderSessions 的「恒答没有」
// 同一个理由：本假实现的 RecordUsage 从不落地保存，本文件的用例不走预报路径。
func (f *fakeStore) UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]TurnUsage, error) {
	return nil, nil
}

// TurnUsages 是 Step 10 D 片之一给 Store 加的读方法（/cost 用）。与上面
// UsageSamples 同一个理由恒答「没有」：本假实现的 RecordUsage 从不落地保存，
// 本包（Service.RecordTurn / SettleLostTurn 等）的用例只断言「WriteTx 被调用、
// 参数对」，不需要经 Store 接口把 turn_usage 读回来。
//
// 失效条件：哪天本包出现一条要断言「记过的账经 Service.TurnUsage 读得回来」
// 的用例，RecordUsage 与这里要一起改成有状态（RecordUsage 开始真的把 u 存
// 进一个切片，这里再按 convID/agentID 过滤 + 排序），不能只改其中一个——
// 那会让「这一行在库里还是不在」出现两个矛盾的答案，与本文件
// AllProviderSessions 那条注释是同一条纪律。/cost 的命令层用例不在本包，
// 放在 internal/repl，那边的 fakeStore 已经因为这个理由改成有状态的了
// （见 internal/repl/conversation_test.go 的同名方法与它的注释）。
func (f *fakeStore) TurnUsages(ctx context.Context, convID, agentID string, limit int) ([]TurnUsage, error) {
	return nil, nil
}

func (f *fakeStore) Cursor(ctx context.Context, convID, agentID string) (Cursor, error) {
	if byAgent, ok := f.cursors[convID]; ok {
		if c, ok := byAgent[agentID]; ok {
			return c, nil
		}
	}
	return Cursor{}, fmt.Errorf("fake store: 游标 (%s, %s): %w", convID, agentID, ErrNoCursor)
}

func (f *fakeStore) Cursors(ctx context.Context, convID string) ([]Cursor, error) {
	byAgent := f.cursors[convID]
	ids := make([]string, 0, len(byAgent))
	for id := range byAgent {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Cursor, 0, len(ids))
	for _, id := range ids {
		out = append(out, byAgent[id])
	}
	return out, nil
}

func (f *fakeStore) CreateConversation(ctx context.Context, title string) (Conversation, error) {
	c := Conversation{ID: f.id("conv"), Title: title, CreatedAt: testNow}
	f.convIdx[c.ID] = len(f.convs)
	f.convs = append(f.convs, c)
	f.createTitles = append(f.createTitles, title)
	return c, nil
}

func (f *fakeStore) AppendMessage(ctx context.Context, m Message) (Message, error) {
	if _, ok := f.convIdx[m.ConvID]; !ok {
		return Message{}, fmt.Errorf("fake store: 追加消息到会话 %q: %w", m.ConvID, ErrConversationNotFound)
	}
	// 契约：ID / Seq / CreatedAt 由 store 填，传进来的一律忽略。
	m.ID = f.id("msg")
	m.Seq = int64(len(f.msgs[m.ConvID])) + 1
	m.CreatedAt = testNow
	if m.Recipients == nil {
		m.Recipients = []string{}
	}
	if m.Meta == nil {
		m.Meta = map[string]any{}
	}
	f.msgs[m.ConvID] = append(f.msgs[m.ConvID], m)
	return m, nil
}

func (f *fakeStore) AddParticipant(ctx context.Context, p Participant) error {
	if p.ConvID == "" {
		// Tx.AddParticipant 的唯一会话来源就是 p.ConvID，空值说明调用方没填。
		return errors.New("fake store: AddParticipant 收到空的 ConvID")
	}
	for _, ex := range f.parts[p.ConvID] {
		if ex.AgentID != p.AgentID {
			continue
		}
		// 规格 G.1：加入一个已退出的 agent 是重新激活，不是插入第二行。
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
	for _, ex := range f.parts[convID] {
		if ex.AgentID != agentID {
			continue
		}
		// 规格 G.2：软删除，不动任何消息。
		ex.Active = false
		left := at
		ex.LeftAt = &left
		return nil
	}
	return fmt.Errorf("fake store: 退出 (%s, %s): %w", convID, agentID, ErrParticipantNotFound)
}

// RequireActiveParticipant 判「这一行存在且 Active」，与真 store 的 SQL 条件一致。
// 真 store 把它放在事务里是为了消除 TOCTOU（未决 #13）；
// 这份假实现没有事务，也就无法复现那个竞态 —— 它只保证语义一致，
// 竞态本身要由真 store 的用例去验。
func (f *fakeStore) RequireActiveParticipant(ctx context.Context, convID, agentID string) error {
	for _, ex := range f.parts[convID] {
		if ex.AgentID == agentID && ex.Active {
			return nil
		}
	}
	return fmt.Errorf("fake store: 活跃成员 (%s, %s): %w", convID, agentID, ErrParticipantNotFound)
}

func (f *fakeStore) SetCursor(ctx context.Context, c Cursor) error {
	if f.cursors[c.ConvID] == nil {
		f.cursors[c.ConvID] = make(map[string]Cursor)
	}
	if c.Pending == nil {
		c.Pending = []int64{}
	}
	f.cursors[c.ConvID][c.AgentID] = c
	return nil
}

// messageCount 是给断言用的便捷读法，不属于 Store 接口。
func (f *fakeStore) messageCount(convID string) int {
	return len(f.msgs[convID])
}

// newServiceForTest 造一个 Service 和一个空会话。会话经 Service.Create 建立，
// 顺带覆盖了「Create 走 WriteTx」这条路径。
func newServiceForTest(t *testing.T) (*Service, *fakeStore, Conversation) {
	t.Helper()
	f := newFakeStore()
	svc := NewService(f)
	conv, err := svc.Create(context.Background(), "测试会话")
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	return svc, f, conv
}

// joinAgent 让 agentID 加入 convID。p.ConvID 显式填上：
// Tx.AddParticipant 只有 p.ConvID 一个会话来源，Service 若另外用 convID 参数覆盖它，值也一样。
func joinAgent(t *testing.T, svc *Service, convID, agentID string) {
	t.Helper()
	err := svc.Join(context.Background(), convID, Participant{
		ConvID:   convID,
		AgentID:  agentID,
		Provider: "fake",
		JoinedAt: testNow,
	})
	if err != nil {
		t.Fatalf("Join %q 失败: %v", agentID, err)
	}
}

// TestCreateDelegatesToStore 断言 Create 是转调：走一次 WriteTx，
// 把 title 原样交给 Tx.CreateConversation，并把补全后的会话返回上来。
func TestCreateDelegatesToStore(t *testing.T) {
	f := newFakeStore()
	svc := NewService(f)

	const title = "FSDT 讨论"
	conv, err := svc.Create(context.Background(), title)
	if err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}

	if f.writeTxCalls != 1 {
		t.Errorf("Create 应当走一次 WriteTx，实际 %d 次", f.writeTxCalls)
	}
	if len(f.createTitles) != 1 || f.createTitles[0] != title {
		t.Errorf("Tx.CreateConversation 收到的标题是 %v，应当是 [%q]", f.createTitles, title)
	}
	if conv.Title != title {
		t.Errorf("返回的会话标题是 %q，应当是 %q", conv.Title, title)
	}
	if conv.ID == "" {
		t.Error("返回的会话 id 为空，store 填的 id 没有传上来")
	}
	if _, err := f.Conversation(context.Background(), conv.ID); err != nil {
		t.Errorf("会话没有真的落进 store: %v", err)
	}
}

// TestPostRejects 覆盖 Post 写之前的四项校验，每个用例只破坏一处，
// 其余字段都是合法的，免得断言被别的校验抢先命中。
func TestPostRejects(t *testing.T) {
	cases := []struct {
		name  string
		build func(conv Conversation) Message
		check func(t *testing.T, err error)
	}{
		{
			name: "会话不存在",
			build: func(conv Conversation) Message {
				return Message{
					ConvID:   "conv-does-not-exist",
					SenderID: ReservedSenderUser,
					Kind:     KindUser,
					Text:     "有人吗",
				}
			},
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrConversationNotFound) {
					t.Fatalf("errors.Is(err, ErrConversationNotFound) 应当为真，实际错误: %v", err)
				}
			},
		},
		{
			name: "Kind 是五个值之外的字符串",
			build: func(conv Conversation) Message {
				return Message{
					ConvID:   conv.ID,
					SenderID: ReservedSenderUser,
					Kind:     MessageKind("shout"),
					Text:     "喂",
				}
			},
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrInvalidKind) {
					t.Fatalf("errors.Is(err, ErrInvalidKind) 应当为真，实际错误: %v", err)
				}
			},
		},
		{
			name: "Kind 为空串",
			build: func(conv Conversation) Message {
				return Message{
					ConvID:   conv.ID,
					SenderID: ReservedSenderUser,
					Kind:     MessageKind(""),
					Text:     "喂",
				}
			},
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrInvalidKind) {
					t.Fatalf("errors.Is(err, ErrInvalidKind) 应当为真，实际错误: %v", err)
				}
			},
		},
		{
			name: "SenderID 为空",
			build: func(conv Conversation) Message {
				return Message{
					ConvID:   conv.ID,
					SenderID: "",
					Kind:     KindUser,
					Text:     "谁说的",
				}
			},
			// 契约的四个哨兵里没有一个对应「SenderID 为空」，所以这里只断言
			// 确实报了错、而且没有被误判成另外两类问题。哪个哨兵合适由编排者决定。
			check: func(t *testing.T, err error) {
				if errors.Is(err, ErrConversationNotFound) || errors.Is(err, ErrInvalidKind) {
					t.Fatalf("空 SenderID 被误判成会话或 kind 的问题: %v", err)
				}
			},
		},
		{
			name: "SenderID 既不是 user 也不是活跃 participant",
			build: func(conv Conversation) Message {
				// 带上脑子：否则先撞 ErrMissingSenderBrain，走不到成员校验，
				// 这条用例就不再测它声称测的东西了。
				return Message{
					ConvID:         conv.ID,
					SenderID:       "ghost",
					Kind:           KindAgent,
					Text:           "我不在这个会话里",
					SenderProvider: "claude",
					SenderModel:    "opus",
				}
			},
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrParticipantNotFound) {
					t.Fatalf("errors.Is(err, ErrParticipantNotFound) 应当为真，实际错误: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, f, conv := newServiceForTest(t)
			joinAgent(t, svc, conv.ID, "alpha")

			before := f.messageCount(conv.ID)
			got, err := svc.Post(context.Background(), tc.build(conv))
			if err == nil {
				t.Fatalf("Post 应当报错，实际返回消息 %+v", got)
			}
			tc.check(t, err)
			if after := f.messageCount(conv.ID); after != before {
				t.Errorf("校验没过的 Post 不应当写入消息，条数从 %d 变成了 %d", before, after)
			}
		})
	}
}

// TestPostAccepts 覆盖两条正例：user 不需要 join 就能发言；活跃 participant 能发言。
func TestPostAccepts(t *testing.T) {
	cases := []struct {
		name     string
		senderID string
		kind     MessageKind
		// 脑子逐例给：KindAgent 必须带（否则撞 ErrMissingSenderBrain），
		// 保留 sender 必须不带（否则撞 ErrInvalidSender）。两条规则方向相反，
		// 同一张表里正好把它们一起钉住。
		provider string
		model    string
	}{
		{name: "user 不需要 join", senderID: ReservedSenderUser, kind: KindUser},
		{name: "活跃 participant", senderID: "alpha", kind: KindAgent, provider: "claude", model: "opus"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, f, conv := newServiceForTest(t)
			joinAgent(t, svc, conv.ID, "alpha")

			const text = "一阶剪切变形理论的位移场"
			got, err := svc.Post(context.Background(), Message{
				ConvID:         conv.ID,
				SenderID:       tc.senderID,
				Kind:           tc.kind,
				Text:           text,
				SenderProvider: tc.provider,
				SenderModel:    tc.model,
			})
			if err != nil {
				t.Fatalf("Post 应当成功，实际: %v", err)
			}
			if got.Seq < 1 {
				t.Errorf("返回的 Seq 是 %d，store 分配的 seq 从 1 起", got.Seq)
			}
			if got.ID == "" {
				t.Error("返回的消息 id 为空，store 填的 id 没有传上来")
			}
			if got.SenderID != tc.senderID || got.Text != text {
				t.Errorf("返回的消息内容被改动了: %+v", got)
			}
			if n := f.messageCount(conv.ID); n != 1 {
				t.Errorf("store 里应当有 1 条消息，实际 %d 条", n)
			}
		})
	}
}

// TestPostRejectsLeftParticipant 是容易漏的一条：Leave 之后不能再发言。
// 先证明退出前发得出去，再证明退出后被拒，这样失败时能分清是哪一步坏了。
func TestPostRejectsLeftParticipant(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	joinAgent(t, svc, conv.ID, "alpha")

	msg := Message{
		ConvID: conv.ID, SenderID: "alpha", Kind: KindAgent, Text: "退出前",
		SenderProvider: "claude", SenderModel: "opus",
	}
	if _, err := svc.Post(context.Background(), msg); err != nil {
		t.Fatalf("退出前 Post 应当成功，实际: %v", err)
	}

	if err := svc.Leave(context.Background(), conv.ID, "alpha", testNow); err != nil {
		t.Fatalf("Leave 失败: %v", err)
	}

	before := f.messageCount(conv.ID)
	if _, err := svc.Post(context.Background(), Message{
		ConvID:         conv.ID,
		SenderID:       "alpha",
		Kind:           KindAgent,
		Text:           "退出后",
		SenderProvider: "claude",
		SenderModel:    "opus",
	}); !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("已退出的 participant 再发言应当命中 ErrParticipantNotFound，实际: %v", err)
	}
	if after := f.messageCount(conv.ID); after != before {
		t.Errorf("被拒的 Post 不应当写入消息，条数从 %d 变成了 %d", before, after)
	}

	// 规格 G.2：退出是软删除，它退出前说过的话还在。
	history, err := svc.History(context.Background(), conv.ID, 0, 10)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	if len(history) != 1 || history[0].Text != "退出前" {
		t.Errorf("退出不应当删掉历史消息，实际读回 %+v", history)
	}
}

// TestRecipientsBroadcastSorted 断言 Recipients 为空时返回当前活跃成员且按字典序。
// 成员故意按乱序插入，假实现也故意不排序，所以顺序只能由 Service 保证。
func TestRecipientsBroadcastSorted(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	for _, id := range []string{"zulu", "alpha", "mike"} {
		joinAgent(t, svc, conv.ID, id)
	}

	// 先确认假实现真的没排序，否则下面的断言是空的。
	raw, err := f.Participants(context.Background(), conv.ID, false)
	if err != nil {
		t.Fatalf("假实现的 Participants 失败: %v", err)
	}
	if len(raw) != 3 || raw[0].AgentID != "zulu" {
		t.Fatalf("假实现应当按插入顺序返回成员，实际: %+v", raw)
	}

	got, err := svc.Recipients(context.Background(), Message{ConvID: conv.ID, SenderID: ReservedSenderUser, Kind: KindUser})
	if err != nil {
		t.Fatalf("Recipients 失败: %v", err)
	}
	want := []string{"alpha", "mike", "zulu"}
	assertStrings(t, "广播收件人", got, want)
}

// TestRecipientsExcludesLeft 断言广播只算当前活跃成员，退出的不再收件。
func TestRecipientsExcludesLeft(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	for _, id := range []string{"zulu", "alpha", "mike"} {
		joinAgent(t, svc, conv.ID, id)
	}
	if err := svc.Leave(context.Background(), conv.ID, "mike", testNow); err != nil {
		t.Fatalf("Leave 失败: %v", err)
	}

	got, err := svc.Recipients(context.Background(), Message{ConvID: conv.ID, SenderID: ReservedSenderUser, Kind: KindUser})
	if err != nil {
		t.Fatalf("Recipients 失败: %v", err)
	}
	assertStrings(t, "退出后的广播收件人", got, []string{"alpha", "zulu"})
}

// TestRecipientsExplicit 断言定向投递：全部存在时原样返回，包括原来的顺序。
func TestRecipientsExplicit(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	for _, id := range []string{"zulu", "alpha", "mike"} {
		joinAgent(t, svc, conv.ID, id)
	}

	want := []string{"zulu", "alpha"}
	got, err := svc.Recipients(context.Background(), Message{
		ConvID:     conv.ID,
		SenderID:   ReservedSenderUser,
		Kind:       KindUser,
		Recipients: []string{"zulu", "alpha"},
	})
	if err != nil {
		t.Fatalf("Recipients 失败: %v", err)
	}
	assertStrings(t, "定向收件人", got, want)
}

// TestRecipientsUnknownAgent 断言定向给一个不存在的 agent 要报错（规格 G.5），
// 不是静默丢弃、也不是静默过滤掉那个 id。
func TestRecipientsUnknownAgent(t *testing.T) {
	svc, _, conv := newServiceForTest(t)
	joinAgent(t, svc, conv.ID, "alpha")

	got, err := svc.Recipients(context.Background(), Message{
		ConvID:     conv.ID,
		SenderID:   ReservedSenderUser,
		Kind:       KindUser,
		Recipients: []string{"alpha", "ghost"},
	})
	if !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("errors.Is(err, ErrParticipantNotFound) 应当为真，实际 err=%v got=%v", err, got)
	}
}

// TestJoinReactivates 断言重新加入一个退出过的 agent 之后，它又能出现在广播收件人里。
func TestJoinReactivates(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	joinAgent(t, svc, conv.ID, "alpha")
	joinAgent(t, svc, conv.ID, "mike")

	if err := svc.Leave(context.Background(), conv.ID, "mike", testNow); err != nil {
		t.Fatalf("Leave 失败: %v", err)
	}
	got, err := svc.Recipients(context.Background(), Message{ConvID: conv.ID, SenderID: ReservedSenderUser, Kind: KindUser})
	if err != nil {
		t.Fatalf("Recipients 失败: %v", err)
	}
	assertStrings(t, "退出后的广播收件人", got, []string{"alpha"})

	joinAgent(t, svc, conv.ID, "mike")
	got, err = svc.Recipients(context.Background(), Message{ConvID: conv.ID, SenderID: ReservedSenderUser, Kind: KindUser})
	if err != nil {
		t.Fatalf("重新加入后 Recipients 失败: %v", err)
	}
	assertStrings(t, "重新加入后的广播收件人", got, []string{"alpha", "mike"})

	// 规格 G.1：重新加入不产生第二行。
	all, err := f.Participants(context.Background(), conv.ID, true)
	if err != nil {
		t.Fatalf("Participants 失败: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("含已退出在内应当只有 2 个 participant，实际 %d 个: %+v", len(all), all)
	}

	// 重新加入之后它又能发言了。
	if _, err := svc.Post(context.Background(), Message{
		ConvID:         conv.ID,
		SenderID:       "mike",
		Kind:           KindAgent,
		Text:           "我回来了",
		SenderProvider: "claude",
		SenderModel:    "opus",
	}); err != nil {
		t.Errorf("重新加入之后应当能发言，实际: %v", err)
	}
}

// TestHistoryDelegates 断言 History 是转调 Store.Messages：三个参数原样传下去，
// 返回值原样传上来。
func TestHistoryDelegates(t *testing.T) {
	svc, f, conv := newServiceForTest(t)
	for _, text := range []string{"第一条", "第二条", "第三条"} {
		if _, err := svc.Post(context.Background(), Message{
			ConvID:   conv.ID,
			SenderID: ReservedSenderUser,
			Kind:     KindUser,
			Text:     text,
		}); err != nil {
			t.Fatalf("准备消息失败: %v", err)
		}
	}

	before := len(f.messagesCalls)
	got, err := svc.History(context.Background(), conv.ID, 1, 2)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	if len(f.messagesCalls) != before+1 {
		t.Fatalf("History 应当正好调一次 Store.Messages，调用记录从 %d 条变成 %d 条", before, len(f.messagesCalls))
	}
	call := f.messagesCalls[len(f.messagesCalls)-1]
	if call.convID != conv.ID || call.afterSeq != 1 || call.limit != 2 {
		t.Errorf("传给 Store.Messages 的参数是 %+v，应当是 {convID:%s afterSeq:1 limit:2}", call, conv.ID)
	}
	if len(got) != 2 {
		t.Fatalf("应当读回 2 条消息，实际 %d 条: %+v", len(got), got)
	}
	if got[0].Seq != 2 || got[1].Seq != 3 {
		t.Errorf("读回的 seq 应当是 2 与 3，实际 %d 与 %d", got[0].Seq, got[1].Seq)
	}
	if got[0].Text != "第二条" || got[1].Text != "第三条" {
		t.Errorf("读回的消息内容不对: %+v", got)
	}
}

// assertStrings 比较两个字符串切片，顺序敏感。
func assertStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s 应当是 %v，实际 %v", what, want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s 应当是 %v，实际 %v", what, want, got)
		}
	}
}
