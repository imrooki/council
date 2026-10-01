package turn

// 本文件覆盖 K 节「会话状态」组里需要走完整 Ask 流程才能验证的部分：
// 首轮 Fresh 与 session_opening 面包屑、Drifted 检测、skipSelf 在
// Runner 层面的实际效果。session.go 里的纯函数（指纹编解码、TTL 判据）
// 已经在 session_test.go 里单独测过，这里只测「整条流程串起来之后」的行为。

import (
	"context"
	"errors"
	"testing"

	"council/internal/conversation"
)

// TestFreshFirstTurnPostsSessionOpeningBreadcrumb 覆盖 D 节判定点二与 09 H 节
// 第 4 步：首轮 Fresh 为真，且在起进程之前必须先发一条 session_opening
// 面包屑。断言面包屑的 sender 是 conversation.ReservedSenderSystem、
// 三个脑子列为空（保留 sender 的硬规矩）、Meta["event"] 与
// Meta["about_agent"] 都在。
func TestFreshFirstTurnPostsSessionOpeningBreadcrumb(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if !rec.Fresh {
		t.Error("首轮 Record.Fresh 应当为真")
	}
	if rec.System == nil {
		t.Fatal("Record.System 为 nil，期望首轮发一条 session_opening 面包屑")
	}
	sys := rec.System
	if sys.SenderID != conversation.ReservedSenderSystem {
		t.Errorf("面包屑 SenderID = %q，期望 %q", sys.SenderID, conversation.ReservedSenderSystem)
	}
	if !sys.SenderBrainEmpty() {
		t.Errorf("面包屑的三个脑子列应当全空，实际 provider=%q model=%q effort=%q", sys.SenderProvider, sys.SenderModel, sys.SenderEffort)
	}
	if sys.Kind != conversation.KindSystem {
		t.Errorf("面包屑 Kind = %q，期望 %q", sys.Kind, conversation.KindSystem)
	}
	event, ok := sys.Meta[MetaEvent]
	if !ok || event != "session_opening" {
		t.Errorf("Meta[%q] = %v，期望 %q", MetaEvent, event, "session_opening")
	}
	about, ok := sys.Meta[MetaAboutAgent]
	if !ok || about != testAgentClaude {
		t.Errorf("Meta[%q] = %v，期望 %q", MetaAboutAgent, about, testAgentClaude)
	}
}

// TestDriftedWhenReturnedSessionIDDiffersFromSent 覆盖 D 节配套规则 3：
// 回传的 id 与送出去的 id 不一样 -> Record.Drifted = true。
//
// 这条用真实字节就能免费构造，不用编：grokProvider.BuildTurn 首轮会现铸
// 一个随机 v4 UUID 写进 PlannedSessionID，而 grok-t1.json 这份真实探针的
// sessionId 是 c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31——新铸的随机 UUID
// 与这个固定值不可能相同，所以 grok 首轮天然就是一次 Drifted（这也是
// 09 D 节「grok 的 uuid 在 BuildTurn 内部铸，不向上冒泡」这条设计下的
// 真实副作用，不是本测试编出来的巧合）。
//
// 为免与「claude 场景下人为构造 Drifted」混淆，这里只断言 grok 首轮的
// Drifted，并且额外用 claude 走一遍「白盒预置一个不同的旧 id」的路径，
// 两条路径互相印证同一个字段的两种触发方式。
func TestDriftedWhenReturnedSessionIDDiffersFromSent(t *testing.T) {
	t.Run("grok 首轮：预铸的 uuid 与探针里固定的 sessionId 必然不同", func(t *testing.T) {
		cfg := newTestConfig()
		svc := newTestService(t)
		convID := newConv(t, svc)
		joinAgent(t, svc, convID, testAgentGrok)
		postUser(t, svc, convID, "第一轮")

		r := newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		fs.enqueueSuccess(grokT1JSON)
		r.spawn = fs.spawn

		rec, err := r.Ask(context.Background(), convID, testAgentGrok)
		if err != nil {
			t.Fatalf("Ask 失败: %v", err)
		}
		if !rec.Drifted {
			t.Error("Drifted 应当为真：BuildTurn 现铸的随机 UUID 不可能等于 grok-t1.json 里固定的 sessionId")
		}
	})

	t.Run("claude 续轮：白盒预置一个与回传值不同的旧 session id", func(t *testing.T) {
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
			t.Fatalf("首轮建立会话失败: %v", err)
		}

		// 白盒把已存的 providerSession 改成一个claude-t2.json 回传值不同的
		// 旧 id：claude-t2.json 的 session_id 与 t1 相同（同一场会话），
		// 所以要触发漂移，必须让「送出去的 id」与这份回传值不同——直接改
		// sessionState 里记的旧 id 即可，无需引入第二份不同会话的探针。
		key := sessionKey{convID: convID, agentID: testAgentClaude}
		// 本用例单 goroutine，预置时没有并发，不取 g.mu。
		r.gate(key).st.providerSession = "a-stale-session-id-that-differs-from-t2"

		postUser(t, svc, convID, "第二轮")
		fs.enqueueSuccess(claudeT2JSON)
		rec, err := r.Ask(context.Background(), convID, testAgentClaude)
		if err != nil {
			t.Fatalf("第二轮失败: %v", err)
		}
		if !rec.Drifted {
			t.Error("Drifted 应当为真：送出去的旧 id 与回传的 t2 session_id 不同")
		}
	})
}

// TestSkipSelfExcludesOwnPriorSpeechOnResumeButNotOnFresh 覆盖 D 节「skipSelf
// 不是第三个判定」推导出的行为：首轮（新会话）必须重放这个 agent 自己过去
// 的发言；resume 那一轮（已有 providerSession）必须跳过。
//
// 构造：先用 grok（testAgentGrok）说一轮建立会话；再手工往库里插一条
// 「本该由它自己说过的话」（用真实场景等价物：另一个由它自己发过的
// KindAgent 消息），验证第二轮（resume）的 delta 里不含这条自我发言，
// 而首轮（尚是新会话）时如果重新起一条会话，会含它过去的发言。
func TestSkipSelfExcludesOwnPriorSpeechOnResumeButNotOnFresh(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)

	postUser(t, svc, convID, "第一轮：请开口")
	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(grokT1JSON) // grok-t1.json 的 text 是 "OK"
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentGrok); err != nil {
		t.Fatalf("首轮失败: %v", err)
	}

	// 现在 zulu（grok）已经有 providerSession，之前它自己说的 "OK" 已经
	// 落库（seq 3：user/system/agent）。追加一条新的用户消息，第二轮
	// （resume）的 Delivered 不应当包含 zulu 自己那条 "OK"。
	postUser(t, svc, convID, "第二轮：resume")
	fs.enqueueSuccess(grokT2JSON)
	rec, err := r.Ask(context.Background(), convID, testAgentGrok)
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}
	history, err := svc.History(context.Background(), convID, 0, 100)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	var ownSpeechSeq int64 = -1
	for _, m := range history {
		if m.SenderID == testAgentGrok && m.Kind == conversation.KindAgent {
			ownSpeechSeq = m.Seq
			break
		}
	}
	if ownSpeechSeq < 0 {
		t.Fatal("没找到 zulu 自己首轮说的那条消息，前置条件不成立")
	}
	for _, seq := range rec.Delivered {
		if seq == ownSpeechSeq {
			t.Errorf("resume 那一轮的 Delivered 里出现了 zulu 自己上一轮的发言（seq=%d），skipSelf 应当过滤掉它", ownSpeechSeq)
		}
	}

	// 反例：另开一场新会话（不同 convID），zulu 在其中还没说过话，
	// 这一轮是 fresh，若这场新会话里已经有 zulu 自己「说过」的话（模拟
	// 跨会话残留数据不会被错误共享），新会话应当重放它——但由于两场会话
	// 天然隔离，这里改为直接验证「同一个新会话里，fresh 轮的 skipSelf
	// 参数为 false」这条更贴近实现的等价表述：调用 filterDelta 本身。
	freshMsgs := []conversation.Message{
		{Seq: 1, SenderID: testAgentGrok, Kind: conversation.KindAgent, Text: "假设这是这个 agent 在全新会话里已经说过的话"},
	}
	visibleFresh := filterDelta(freshMsgs, testAgentGrok, false)
	if len(visibleFresh) != 1 {
		t.Errorf("skipSelf=false（新会话）时应当重放 agent 自己的发言，实际可见集合长度=%d", len(visibleFresh))
	}
	visibleResume := filterDelta(freshMsgs, testAgentGrok, true)
	if len(visibleResume) != 0 {
		t.Errorf("skipSelf=true（resume）时应当跳过 agent 自己的发言，实际可见集合长度=%d", len(visibleResume))
	}
}

// TestIdentityDriftOnDisplayNameChange 记录一个实测行为与文档注释不一致的
// 地方（详见最终报告的实现问题清单，四段式那一条）：render.go 第 27-28 行
// 的注释说「这里故意不把 to.DisplayName / to.Role 写进正文：DisplayName
// 不在 IdentityFingerprint 里」，但 renderIdentity（render.go）把
// DisplayName 拼进了身份块（"You are <DisplayName> (id=...).")，而这个
// 身份块整个被喂进了 IdentityFingerprint。于是改 DisplayName 会改变指纹，
// 从而触发 ErrIdentityDrift——这与该行注释字面上的断言矛盾。
//
// 这条行为本身很可能是对的（DisplayName 确实被塞进
// --append-system-prompt-file 一类身份 flag，改了就是改缓存前缀），
// 这里只钉住"实际发生了什么"，不代表判定它是 bug 还是文档写错，
// 留给报告里的人判断修哪一边。
func TestIdentityDriftOnDisplayNameChange(t *testing.T) {
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

	// 只改 DisplayName，Model/Role/Effort/Permissions 全部原样不动。
	ac := cfg.Agents[testAgentClaude]
	ac.DisplayName = ac.DisplayName + "-改名了"
	cfg.Agents[testAgentClaude] = ac
	postUser(t, svc, convID, "第二轮，只改了显示名")

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused（这是当前实测行为，不是本用例在断言"+
			"「这样才对」——见最终报告）", rec.Outcome)
	}
	if !errors.Is(err, ErrIdentityDrift) {
		t.Fatalf("只改 DisplayName 之后，实测会触发 ErrIdentityDrift，实际: %v", err)
	}
}

// TestConvAndAgentPairsAreIndependentSessions 覆盖 K 节「会话状态」组：
// (convA, X) 与 (convB, X) 是两条互不相干的会话——sessionKey 以
// (convID, agentID) 为键，同一个 agent 在两场不同会话里必须各自 Fresh，
// 互不干扰。
func TestConvAndAgentPairsAreIndependentSessions(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convA := newConv(t, svc)
	convB := newConv(t, svc)
	joinAgent(t, svc, convA, testAgentClaude)
	joinAgent(t, svc, convB, testAgentClaude)
	postUser(t, svc, convA, "会话 A 的问题")
	postUser(t, svc, convB, "会话 B 的问题")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	recA, err := r.Ask(context.Background(), convA, testAgentClaude)
	if err != nil {
		t.Fatalf("会话 A 失败: %v", err)
	}
	recB, err := r.Ask(context.Background(), convB, testAgentClaude)
	if err != nil {
		t.Fatalf("会话 B 失败: %v", err)
	}
	if !recA.Fresh || !recB.Fresh {
		t.Errorf("两场会话里同一个 agent 都应当各自 Fresh=true，实际 A=%v B=%v", recA.Fresh, recB.Fresh)
	}
}
