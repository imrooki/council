package turn

// runner_rebuild_test.go 覆盖 11a 契约 3.4 的 Runner.Rebuild：五样全丢
// （断言走的是删行 DropProviderSession，不是只清指纹）、不存在时
// existed=false 且不报错、以及丢弃之后下一轮确实走 Fresh。

import (
	"context"
	"testing"

	"council/internal/conversation"
)

// TestRebuildDropsFullRowNotOnlyFingerprint 是契约第 6 条验收的直接断言：
// 用 recordingStore 记录调用序列，证明 Rebuild 调的是 tx.DropProviderSession
// （删整行），而不是读一次、改一个字段再 SetProviderSession（只清指纹）。
func TestRebuildDropsFullRowNotOnlyFingerprint(t *testing.T) {
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)

	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "sess-to-drop",
		CursorSeq:         7,
		CostTotalUSD:      1.23,
		HaveCostTotal:     true,
		IdentityFP:        "fp-1",
		Spoken:            true,
	})
	if len(rs.setProviderSessionCalls) != 1 {
		t.Fatalf("前置条件不成立：种子会话应当恰好写入 1 次，实际 %d 次", len(rs.setProviderSessionCalls))
	}

	r := newTestRunner(t, cfg, svc)
	existed, err := r.Rebuild(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Rebuild 失败: %v", err)
	}
	if !existed {
		t.Error("existed 应当为 true：这条会话确实存在过")
	}
	if len(rs.dropProviderSessionCalls) != 1 {
		t.Fatalf("DropProviderSession 调用次数 = %d，期望 1（五样全丢必须经删行）", len(rs.dropProviderSessionCalls))
	}
	dropped := rs.dropProviderSessionCalls[0]
	if dropped.convID != convID || dropped.agentID != testAgentClaude {
		t.Errorf("DropProviderSession 的 (convID, agentID) = %+v，期望 (%s, %s)", dropped, convID, testAgentClaude)
	}
	if _, stillThere, _ := rs.ProviderSession(context.Background(), convID, testAgentClaude); stillThere {
		t.Error("Rebuild 之后这一行应当整行消失，而不是留着一份改过的记录")
	}
}

// TestRebuildOnMissingSessionIsNotAnError 覆盖「不存在不是错误」：
// 对一个从没说过话的成员敲 rebuild，existed=false 且 err=nil。
func TestRebuildOnMissingSessionIsNotAnError(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	existed, err := r.Rebuild(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("对一条从没建立过的会话调 Rebuild 不该报错，实际: %v", err)
	}
	if existed {
		t.Error("existed 应当为 false：这个 agent 在这条会话里从没开过口")
	}
}

// TestRebuildClearsInMemoryStateTooNotJustTheDatabase 覆盖「同时清掉这条
// (会话, 成员) 的 gate.st」：不通过重启进程，直接在同一个 Runner 实例上先
// 建立一条会话（这会把它放进这条 key 的 gate.st），Rebuild 之后立刻再问
// SessionInfo，必须是「没有」——如果 Rebuild 只删了库、忘了清内存，
// SessionInfo 会先看内存并从那里读到一条已经在数据库里被删掉的假会话。
//
// 拆闸之后 gate 本身永不从 r.gates 里删除（不变量见 sessionGate 的注释），
// 所以不能再用「这张 map 的长度」这类断言——gate 数量不会随 Rebuild 变化，
// 变的只是这条 key 对应的 gate.st 是不是被置回 nil，因此下面改成对具体
// 这条 key 直接断言 gate.st 的值。
func TestRebuildClearsInMemoryStateTooNotJustTheDatabase(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("首轮失败: %v", err)
	}
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	if r.gate(key).st == nil {
		t.Fatal("前置条件不成立：首轮之后这条 (会话, 成员) 应当已经有内存会话状态，实际 gate.st 为 nil")
	}

	existed, err := r.Rebuild(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Rebuild 失败: %v", err)
	}
	if !existed {
		t.Error("existed 应当为 true：首轮已经建立过这条会话")
	}
	if r.gate(key).st != nil {
		t.Errorf("Rebuild 之后这条 (会话, 成员) 的 gate.st 应当为 nil（内存快照必须被丢弃），实际非 nil: %+v", r.gate(key).st)
	}

	if _, ok := r.SessionInfo(context.Background(), convID, testAgentClaude); ok {
		t.Error("Rebuild 之后 SessionInfo 应当报告没有这条会话")
	}
}

// TestRebuildThenNextAskIsFresh 覆盖 Rebuild 存在的最终目的：丢弃之后
// 下一轮 Ask 必须重新走 Fresh 路径（Record.Fresh=true、发一条新的
// session_opening 面包屑），而不是拿着已经被删掉的会话 id 去 --resume。
func TestRebuildThenNextAskIsFresh(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	first, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("首轮失败: %v", err)
	}
	if !first.Fresh {
		t.Fatal("前置条件不成立：首轮应当是 Fresh")
	}

	if _, err := r.Rebuild(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("Rebuild 失败: %v", err)
	}

	postUser(t, svc, convID, "第二轮，rebuild 之后")
	fs.enqueueSuccess(claudeT1JSON)
	second, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("rebuild 之后的一轮失败: %v", err)
	}
	if !second.Fresh {
		t.Error("Rebuild 之后下一轮 Ask 应当是 Fresh，实际不是")
	}
	if second.System == nil {
		t.Error("Rebuild 之后下一轮应当像首轮一样发一条 session_opening 面包屑")
	}
}
