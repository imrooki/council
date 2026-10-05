package turn

// 本文件覆盖 K 节「游标」组：ScannedTo 的取法、ErrNoDelta 时游标照常推进、
// 以及 ErrPrefixRewrite 这条防线本身。

import (
	"context"
	"errors"
	"testing"
	"time"

	"council/internal/conversation"
)

// TestErrNoDeltaStillAdvancesCursor 覆盖 09 F 节「过滤完一条都不剩时……
// 游标照常推到 ScannedTo」：连着敲两次 /ask 同一个 agent、中间没有新内容，
// 第二次应当拿到 ErrNoDelta，且不调用 Spawner；之后如果真的有新内容，
// 第三次必须只送新增的那一部分，证明第二次确实推动了游标而不是原地打转。
func TestErrNoDeltaStillAdvancesCursor(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一条问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("首轮失败: %v", err)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("首轮之后 Spawner 调用次数 = %d，期望 1", len(fs.calls))
	}

	// 首轮成功后，库里唯一的新增行是 alice 自己的回复（skipSelf 会跳过它）
	// 和 session_opening 面包屑（KindSystem 恒被跳过）——没有别人新说话，
	// 第二次 Ask 应当拿到 ErrNoDelta。
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if !errors.Is(err, ErrNoDelta) {
		t.Fatalf("errors.Is(err, ErrNoDelta) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("ErrNoDelta 的 Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("ErrNoDelta 这一轮不该调用 Spawner，调用次数从 1 变成了 %d", len(fs.calls))
	}

	// 现在追加一条新消息，第三次 Ask 必须只把这条新内容送出去——如果第二次
	// 的 ErrNoDelta 没有把游标推到 ScannedTo，这里会把 alice 自己的回复也
	// 重新扫进来（skipSelf 已经生效所以不会出现在 delta 里），更直接的钉法是
	// 断言 rec2.ScannedTo 严格大于第二次的 rec.ScannedTo。
	postUser(t, svc, convID, "第二条问题，第三轮应当只看到这一条")
	fs.enqueueSuccess(claudeT2JSON)
	rec2, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第三轮失败: %v", err)
	}
	if rec2.Outcome != OutcomeSpoke {
		t.Fatalf("第三轮 Outcome = %v，期望 OutcomeSpoke", rec2.Outcome)
	}
	if rec2.ScannedTo <= rec.ScannedTo {
		t.Errorf("第三轮 ScannedTo=%d 应当严格大于 ErrNoDelta 那一轮的 ScannedTo=%d，说明游标被推进了", rec2.ScannedTo, rec.ScannedTo)
	}
}

// TestScannedToIsNotDeliveredMax 覆盖 K 节游标组的核心用例：ScannedTo 取
// 本轮扫描到的最大 seq，不是 Delivered 的最大值，也不是 MaxSeq(conv)——
// 构造 A 飞行中、B 拿到 seq 12、A 的回复拿到 seq 13 这种交错场景（这里用
// 「alice 说话期间 zulu 插了一句」简化复现同样的交错关系）：alice 这一轮
// 送出去的 delta 里最后一条是别人插进来的话，但 alice 自己说的话（会被
// 过滤成不可见）产生的 seq 不该被算进 Delivered，也不该让 ScannedTo 停在
// Delivered 的最大值之前。
func TestScannedToIsNotDeliveredMax(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	joinAgent(t, svc, convID, testAgentGrok)

	postUser(t, svc, convID, "问题一") // seq 1

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("alice 首轮失败: %v", err)
	}
	// 此刻库里有：seq1 user、seq2 system(session_opening)、seq3 alice 的回复。

	// zulu 插一句（模拟"B 拿到 seq"）。
	postUser(t, svc, convID, "问题二，zulu 先看到") // seq 4

	// alice 再被问一次：这一轮的 scanned 包含 seq2(system,过滤)、seq3(alice
	// 自己,skipSelf 过滤，因为此时 alice 已经有 providerSession 了)、seq4。
	// Delivered 只含 seq4，但 ScannedTo 必须等于 scanned 里的最大 seq，
	// 即使那个最大值来自一条被过滤掉的行都不影响——这里凑巧最大值就是
	// seq4，所以额外验证「即使中间插了会被过滤掉的行，ScannedTo 也不会
	// 掉在 Delivered 的最大值之前」这条不等式恒成立，而不是恰好相等。
	fs.enqueueSuccess(claudeT2JSON)
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("alice 第二轮失败: %v", err)
	}
	if len(rec.Delivered) == 0 {
		t.Fatal("Delivered 为空，前置条件不成立")
	}
	maxDelivered := rec.Delivered[len(rec.Delivered)-1]
	if rec.ScannedTo < maxDelivered {
		t.Errorf("ScannedTo=%d 小于 Delivered 的最大值=%d，游标会永久漏投被过滤掉的行", rec.ScannedTo, maxDelivered)
	}
	// alice 自己的回复（seq3）必须被过滤掉，不出现在 Delivered 里。
	for _, seq := range rec.Delivered {
		if seq == 3 {
			t.Errorf("Delivered 里出现了 seq=3（alice 自己上一轮的回复），skipSelf 应当过滤掉它: %+v", rec.Delivered)
		}
	}
}

// prefixRewriteStore 是一个刻意违反 conversation.Store 契约的假实现，
// 只用于下面这一条测试。它的存在理由写在 TestErrPrefixRewrite 的注释里：
// runner.go 的 ErrPrefixRewrite 防线检查的是「scanned[0].Seq <= st.cursor」，
// 而真实 store（以及任何遵守 Store.Messages 契约——「取 seq 严格大于
// afterSeq 的消息」——的实现）在数学上不可能让这个条件成立：Messages
// 只会返回 Seq 严格大于所传 afterSeq 的行，而 Ask 传的 afterSeq 正是
// st.cursor 本身。这条防线因此是一条「不该发生」的纯防御性代码
// （09 K 节原话：「编排层的 bug，不该发生」），要触发它，唯一办法是让底层
// store 本身违反自己的契约——这正是这个假实现存在的唯一目的。
//
// 它同时实现 conversation.Store 与 conversation.Tx（WriteTx 直接把自己
// 转发进去），因为本测试不会真正走到写路径，写路径的方法体都只是安全的
// stub。
type prefixRewriteStore struct {
	msgs []conversation.Message
}

func (s *prefixRewriteStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return fn(ctx, s)
}

func (s *prefixRewriteStore) Conversation(ctx context.Context, id string) (conversation.Conversation, error) {
	return conversation.Conversation{ID: id}, nil
}

func (s *prefixRewriteStore) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	return nil, nil
}

func (s *prefixRewriteStore) Participants(ctx context.Context, convID string, includeInactive bool) ([]conversation.Participant, error) {
	return nil, nil
}

// Messages 是这个假实现存在的唯一原因：无论调用方传的 afterSeq 是多少，
// 一律原样吐出预置的消息，哪怕它们的 Seq 小于等于 afterSeq——这就是
// 「违反契约」的具体做法。
func (s *prefixRewriteStore) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]conversation.Message, error) {
	return s.msgs, nil
}

func (s *prefixRewriteStore) MaxSeq(ctx context.Context, convID string) (int64, error) {
	return int64(len(s.msgs)), nil
}

func (s *prefixRewriteStore) Cursor(ctx context.Context, convID, agentID string) (conversation.Cursor, error) {
	return conversation.Cursor{}, conversation.ErrNoCursor
}

func (s *prefixRewriteStore) Cursors(ctx context.Context, convID string) ([]conversation.Cursor, error) {
	return nil, nil
}

func (s *prefixRewriteStore) CreateConversation(ctx context.Context, title string) (conversation.Conversation, error) {
	return conversation.Conversation{}, nil
}

func (s *prefixRewriteStore) AppendMessage(ctx context.Context, m conversation.Message) (conversation.Message, error) {
	return m, nil
}

func (s *prefixRewriteStore) AddParticipant(ctx context.Context, p conversation.Participant) error {
	return nil
}

func (s *prefixRewriteStore) DeactivateParticipant(ctx context.Context, convID, agentID string, at time.Time) error {
	return nil
}

func (s *prefixRewriteStore) RequireActiveParticipant(ctx context.Context, convID, agentID string) error {
	return nil
}

func (s *prefixRewriteStore) SetCursor(ctx context.Context, c conversation.Cursor) error {
	return nil
}

// 下面三个是 Step 7 给 Tx 与 Store 加的方法。这个假实现存在的唯一目的是
// 违反 Store.Messages 的契约（无论 afterSeq 是什么都回同一批），
// 用来验 ErrPrefixRewrite 那条防线；它不参与任何持久化路径，
// 所以三个都是空实现。有状态的假实现由 Step 7 的测试另写。
func (s *prefixRewriteStore) ProviderSession(ctx context.Context, convID, agentID string) (conversation.ProviderSession, bool, error) {
	return conversation.ProviderSession{}, false, nil
}

// AllProviderSessions 同上，本假实现只为违反 Store.Messages 的契约而生，
// 不维护 provider 会话状态，nil 即可。
func (s *prefixRewriteStore) AllProviderSessions(ctx context.Context) ([]conversation.ProviderSession, error) {
	return nil, nil
}

// UsageSamples 同上，本假实现只为违反 Store.Messages 的契约而生，
// 不维护 usage 状态，nil 即可。
func (s *prefixRewriteStore) UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

// TurnUsages 是 Step 10 D 片之一给 Store 加的读方法（/cost 用），同上面
// UsageSamples 一样，本假实现只为违反 Store.Messages 的契约而生（见类型
// 注释），不维护 usage 状态，nil 即可。
func (s *prefixRewriteStore) TurnUsages(ctx context.Context, convID, agentID string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

func (s *prefixRewriteStore) RecordUsage(ctx context.Context, u conversation.TurnUsage) error {
	return nil
}

func (s *prefixRewriteStore) SetProviderSession(ctx context.Context, ps conversation.ProviderSession) error {
	return nil
}

func (s *prefixRewriteStore) DropProviderSession(ctx context.Context, convID, agentID string) (bool, error) {
	return false, nil
}

// SetConversationTeam 是 Step 9 片 2a 给 Tx 加的写方法。这个假实现只为
// 违反 Store.Messages 的契约而生（见类型注释），与 team 无关，照此文件
// 对其它不相干方法的既有体例（RecordUsage / SetProviderSession /
// DropProviderSession 都是空桩），保持空桩，不额外造状态。
func (s *prefixRewriteStore) SetConversationTeam(ctx context.Context, convID, teamID string) error {
	return nil
}

// SetConversationCwd：turn 包不调用 CreateIn 和 SetConversationCwd，这里只为满足 conversation.Tx 接口；哪天 turn 包开始建带目录的会话，要改成记账。
func (s *prefixRewriteStore) SetConversationCwd(ctx context.Context, convID, cwd string) error {
	return nil
}

// 下面四个是 Step 10 给 Tx 与 Store 加的方法（inflight_turns 那一片）。
// 与上面 SetConversationTeam 同一个理由：这个假实现只为违反
// Store.Messages 的契约而生，与在飞记录无关，保持空桩，不额外造状态。
//
// 失效条件（Step 10 A 片之三订正时特地点名，供以后复用这个类型的人核对）：
// 本类型只服务 TestErrPrefixRewrite 这一条用例，而 ErrPrefixRewrite 那道
// 闸（"scanned[0].Seq <= st.cursor"）在 askUpToWith 里的位置结构性地早于
// BeginTurn（落点 A 在 cmd.Dir 回填之后，远在这道闸的读历史步骤之后），
// 所以这条用例永远走不到 BeginTurn，四个空桩至今没有变红。若哪天这个
// 类型被复用于一条会真正跑到 BeginTurn 之后（即 spawn 会被调用）的测试，
// ClearInflight 恒答 existed=false 会让 Service.RecordTurn 判定
// ErrInflightMissing 并回滚整轮——与 internal/repl/conversation_test.go
// 的 fakeStore 曾经踩过的红一模一样，届时必须换成与那份修法相同的
// 有状态实现（记账用的 map[turnID]InflightTurn，SetInflightPhase 查不到
// 要报 conversation.ErrInflightMissing），不能继续假设"不会走到"。
func (s *prefixRewriteStore) BeginTurn(ctx context.Context, t conversation.InflightTurn) error {
	return nil
}

func (s *prefixRewriteStore) SetInflightPhase(ctx context.Context, turnID string, phase conversation.InflightPhase) error {
	return nil
}

func (s *prefixRewriteStore) ClearInflight(ctx context.Context, turnID string) (bool, error) {
	return false, nil
}

// InflightFor 为 Store.InflightFor 补的方法。本假实现 AllInflight 恒答空，这里恒答没有。
func (s *prefixRewriteStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	return conversation.InflightTurn{}, false, nil
}

func (s *prefixRewriteStore) AllInflight(ctx context.Context) ([]conversation.InflightTurn, error) {
	return nil, nil
}

// TestErrPrefixRewrite 构造一个最小 seq 小于等于当前游标的批次：白盒直接
// 往这条 (会话, 成员) 的 gate.st 里塞一条已经有会话（非 fresh）、游标为 5 的 sessionState，
// 而底层 store（上面那个刻意违反契约的假实现）无论传入的 afterSeq 是什么
// 都会原样吐出预置的两条消息。
//
// 两条消息、而不是一条，是必需的：readAfter 自己也有一道「seq 没有前进就
// fail-closed」的防线（runner.go 原话「再读只会原地打转」），它比较的是
// 「这一页最后一条的 Seq」与「这一页开始时的 afterSeq」。只放一条 Seq<=游标
// 的消息会先撞上那道防线、报一个不同的错误，走不到本用例真正要测的
// ErrPrefixRewrite。所以构造成 [{Seq:3}, {Seq:10}]、游标=5：
// 第一条 3<=5 满足 Ask 层「scanned[0].Seq <= st.cursor」，最后一条 10>5
// 满足 readAfter 内部「至少前进了一格」，两道闸各自读到自己要读的那个值。
func TestErrPrefixRewrite(t *testing.T) {
	cfg := newTestConfig()
	fake := &prefixRewriteStore{
		msgs: []conversation.Message{
			{ConvID: "c1", Seq: 3, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "本该已经投递过的一条"},
			{ConvID: "c1", Seq: 10, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "让 readAfter 自己的前进检查满意"},
		},
	}
	svc := conversation.NewService(fake)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	ac := cfg.Agents[testAgentClaude]
	fp := fingerprintFor(t, r, "c1", testAgentClaude, ac)
	key := sessionKey{convID: "c1", agentID: testAgentClaude}
	// 本用例单 goroutine，预置时没有并发，不取 g.mu。
	r.gate(key).st = &sessionState{
		providerSession: "existing-session-id",
		cursor:          5, // 严格大于假 store 吐出来的第一条 Seq=3
		identityFP:      fp,
		spoken:          true,
	}

	rec, err := r.Ask(context.Background(), "c1", testAgentClaude)
	if !errors.Is(err, ErrPrefixRewrite) {
		t.Fatalf("errors.Is(err, ErrPrefixRewrite) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if len(fs.calls) != 0 {
		t.Fatalf("ErrPrefixRewrite 这一支不该调用 Spawner，实际调用了 %d 次", len(fs.calls))
	}
}
