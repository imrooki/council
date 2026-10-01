package turn

// 本文件覆盖 10a 契约 §3.4（2026-09-21 新增）：SessionInfo 的签名改成
// (ctx, convID, agentID)，读回不看内存，回库读一次；读回不写进这条
// (会话, 成员) 的 gate.st，也不做指纹校验；读失败照实返回「没有」，
// 不 panic、不上抛。

import (
	"context"
	"errors"
	"testing"
	"time"

	"council/internal/conversation"
)

// erroringSessionStore 是一个只用来让 Service.ProviderSession 必然报错的
// 最小 conversation.Store 实现，专门验证「回库读失败」这条路径。除
// ProviderSession 外的方法都不会被本文件的测试调用到：SessionInfo 只转发
// ProviderSession；本文件顺带验证的 Ask（见文末）在这条 (会话, 成员) 的
// gate.st 里没有记录时，读回失败会立刻 r.refuse，走不到 Conversation /
// Messages 那些方法。
// WriteTx 同理，两条路径都不涉及写事务，返回的错误不会被使用到。
type erroringSessionStore struct{}

var errSessionStoreAlwaysFails = errors.New("erroringSessionStore: ProviderSession always fails, by design")

func (erroringSessionStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return errors.New("erroringSessionStore: WriteTx should not be reached by these tests")
}
func (erroringSessionStore) Conversation(ctx context.Context, id string) (conversation.Conversation, error) {
	return conversation.Conversation{}, conversation.ErrConversationNotFound
}
func (erroringSessionStore) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	return nil, nil
}
func (erroringSessionStore) Participants(ctx context.Context, convID string, includeInactive bool) ([]conversation.Participant, error) {
	return nil, nil
}
func (erroringSessionStore) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]conversation.Message, error) {
	return nil, nil
}
func (erroringSessionStore) MaxSeq(ctx context.Context, convID string) (int64, error) { return 0, nil }
func (erroringSessionStore) Cursor(ctx context.Context, convID, agentID string) (conversation.Cursor, error) {
	return conversation.Cursor{}, conversation.ErrNoCursor
}
func (erroringSessionStore) Cursors(ctx context.Context, convID string) ([]conversation.Cursor, error) {
	return nil, nil
}
func (erroringSessionStore) ProviderSession(ctx context.Context, convID, agentID string) (conversation.ProviderSession, bool, error) {
	return conversation.ProviderSession{}, false, errSessionStoreAlwaysFails
}

// AllProviderSessions 不属于本假实现的既定失败面：它存在的唯一理由是让
// Service.ProviderSession 这一条单行读路径必然出错（类型注释开头写明的
// 「只用来让 Service.ProviderSession 必然报错」），其余方法（Cursor、
// Messages、Participants、Conversations……）一律答「没有/空」而不是报错。
// 本文件目前没有用例走全量读这条路，答「空」与上面那些同伴一致，
// 不为它另编一种失败语义。
func (erroringSessionStore) AllProviderSessions(ctx context.Context) ([]conversation.ProviderSession, error) {
	return nil, nil
}

// InflightFor 为 Store.InflightFor 补的方法。本假实现 AllInflight 恒答空，这里恒答没有。
func (erroringSessionStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	return conversation.InflightTurn{}, false, nil
}

// AllInflight 同样不属于本假实现的既定失败面（唯一理由是让
// Service.ProviderSession 必然报错），答「空」与 AllProviderSessions
// 一致，不为它另编一种失败语义。
func (erroringSessionStore) AllInflight(ctx context.Context) ([]conversation.InflightTurn, error) {
	return nil, nil
}

// UsageSamples 只为满足接口而存在：erroringSessionStore 的既定失败面
// 只在 ProviderSession 一个方法上，本文件的用例不走预报路径。
func (erroringSessionStore) UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

// TurnUsages 是 Step 10 D 片之一给 Store 加的读方法（/cost 用），只为满足
// 接口而存在：erroringSessionStore 的既定失败面只在 ProviderSession 一个
// 方法上（类型注释开头写明的"只用来让 Service.ProviderSession 必然报错"），
// 本文件的用例（SessionInfo 只转发 ProviderSession；Ask 在 gate.st 没有
// 记录时读回失败会立刻 refuse）不会走到 turn_usage 读路径，答"空"与
// AllProviderSessions / AllInflight 两个同伴一致，不为它另编一种失败语义。
func (erroringSessionStore) TurnUsages(ctx context.Context, convID, agentID string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

// TestSessionInfoFallsBackToStoreWhenNotInMemory 覆盖「内存里没有、库里有
// 一条会话」：ok 为真，且字段等于库里的值。这条对应 2026-09-21 真机冒烟
// 抓到的缺陷（10a §3.4）：重启后第一次 /session 曾经显示成 fresh，而会话
// 明明在库里。种子数据经 seedProviderSession（定义在
// runner_readback_test.go）写入，Runner 的 r.gates 保持空。
func TestSessionInfoFallsBackToStoreWhenNotInMemory(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	seededAt := time.Now().UTC().Truncate(time.Second)
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "seeded-session-id",
		CursorSeq:         3,
		CostTotalUSD:      1.23,
		HaveCostTotal:     true,
		LastTurnAt:        seededAt,
		IdentityFP:        "whatever-fp",
		Spoken:            true,
	})

	r := newTestRunner(t, cfg, svc)
	// r.gate(key) 是 get-or-create，调用它本身就会往 map 里塞一条，所以这条
	// 前置检查必须在第一次 r.gate(...) 调用之前、直接读 r.gates 字段做——
	// 下面马上要调的 r.SessionInfo(...) 内部会调 gate()，这里的检查必须先于它。
	if len(r.gates) != 0 {
		t.Fatalf("前置条件不成立：r.gates 应当为空，实际 %d 条", len(r.gates))
	}

	info, ok := r.SessionInfo(context.Background(), convID, testAgentClaude)
	if !ok {
		t.Fatal("ok 应当为真：会话明明在库里")
	}
	if info.ProviderSession != "seeded-session-id" {
		t.Errorf("ProviderSession = %q，期望等于库里的值", info.ProviderSession)
	}
	if !info.LastTurnAt.Equal(seededAt) {
		t.Errorf("LastTurnAt = %v，期望等于库里的值 %v", info.LastTurnAt, seededAt)
	}
}

// TestSessionInfoNotFoundWhenNeitherMemoryNorStoreHasIt 覆盖「内存里没有、
// 库里也没有」：ok 为假。
func TestSessionInfoNotFoundWhenNeitherMemoryNorStoreHasIt(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	_, ok := r.SessionInfo(context.Background(), convID, testAgentClaude)
	if ok {
		t.Error("这个 agent 从没开过口，ok 应当为假")
	}
}

// TestSessionInfoOnStoreErrorReturnsNotFoundWithoutPanic 覆盖「读库出错时
// -> ok == false 且不 panic」：runner.go 的注释原话「/session 是一条给人看
// 的命令，一行读失败不该让整张表打不出来」。用 erroringSessionStore 逼
// ProviderSession 必然返回错误；本用例若真的 panic 会让 go test 这个进程
// 直接崩掉，测试跑到这里而没有崩，本身就是「没有 panic」这条断言成立的
// 证据。
func TestSessionInfoOnStoreErrorReturnsNotFoundWithoutPanic(t *testing.T) {
	cfg := newTestConfig()
	svc := conversation.NewService(erroringSessionStore{})
	r := newTestRunner(t, cfg, svc)

	info, ok := r.SessionInfo(context.Background(), "some-conv", testAgentClaude)
	if ok {
		t.Error("store 读回报错时 ok 应当为假")
	}
	if info != (SessionInfo{}) {
		t.Errorf("store 读回报错时 SessionInfo 应当是零值，实际 %+v", info)
	}
}

// TestSessionInfoDoesNotWriteBackIntoMemory 覆盖「读回不写进这条 (会话, 成员)
// 的 gate.st」：调一次 SessionInfo 之后，白盒直接看这条 key 的 gate.st，
// 仍然是 nil——10a §3.4 明写「读回不产生任何副作用」，代价是下一次 Ask
// 仍然要独立地再读一次库、并且做指纹校验（那是 Ask 的职责，不是 SessionInfo
// 的）。
//
// 这里不能像「全新 Runner」那组用例一样断言 len(r.gates)==0：SessionInfo
// 内部本身就要调 r.gate(key) 取闸（TryLock 一下），gate() 是 get-or-create，
// 调用完之后这条 key 必然已经在 r.gates 里——map 长度变成 1 是正常的、
// 不代表任何东西被写坏。真正要问的是「这条 gate 的 st 字段有没有被填」，
// 所以断言换成 r.gate(key).st == nil。
func TestSessionInfoDoesNotWriteBackIntoMemory(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "seeded-session-id",
		LastTurnAt:        time.Now().UTC(),
		IdentityFP:        "whatever-fp",
	})

	r := newTestRunner(t, cfg, svc)
	if _, ok := r.SessionInfo(context.Background(), convID, testAgentClaude); !ok {
		t.Fatal("前置条件不成立：应当能读到这条会话")
	}
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	if r.gate(key).st != nil {
		t.Errorf("SessionInfo 之后这条 (会话, 成员) 的 gate.st = %+v，期望 nil（读回不该写进内存）", r.gate(key).st)
	}
}

// TestAskRefusesRatherThanFreshOnProviderSessionReadError 不在任务书的必测
// 清单里，顺手用上面的 erroringSessionStore（成本很低）记录一处实现与 10a
// 契约字面不符的地方，详见最终报告的实现问题清单：
//
// 10a §3.2 原文是「读回失败或 ok == false 时按 Fresh 处理」，把「读失败」
// 与「没读到」并列成同一种处置。但 runner.go 里 `loadErr != nil` 那一支走
// 的是 `r.refuse(ctx, &rec, nil, loadErr)`——直接拒绝这一轮（OutcomeRefused，
// 不起进程），而不是像 `!found` 那样退回 Fresh（`st = &sessionState{}`
// 继续往下走、正常起一条新会话）。
//
// 这里只钉住「实际发生了什么」，不代表判定哪一边该改，留给报告里的人判断。
func TestAskRefusesRatherThanFreshOnProviderSessionReadError(t *testing.T) {
	cfg := newTestConfig()
	svc := conversation.NewService(erroringSessionStore{})
	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), "some-conv", testAgentClaude)
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("实测 Outcome = %v，期望 OutcomeRefused（这是当前实现的行为，不是本用例断言"+
			"「这样才对」——见最终报告：10a §3.2 说读回失败该按 Fresh 处理）", rec.Outcome)
	}
	if !errors.Is(err, errSessionStoreAlwaysFails) {
		t.Fatalf("errors.Is(err, errSessionStoreAlwaysFails) 应当为真，实际: %v", err)
	}
	if len(fs.calls) != 0 {
		t.Errorf("Spawner 被调用了 %d 次，期望 0：无论走 Fresh 还是 Refused，这一轮都不该为「读库失败」本身花钱", len(fs.calls))
	}
}
