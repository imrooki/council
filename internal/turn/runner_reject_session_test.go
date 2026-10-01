package turn

// runner_reject_session_test.go 覆盖未决 #20 的裁决：RecordTurn 的
// session==nil 从「作废，删行」改成「不碰这一行」，真正的作废挪到 Ask 里
// 「从库读回、指纹不符」那一支显式调 Service.DropProviderSession。
//
// 判据统一是**库里那一行还在不在、内容是否原样**（svc.ProviderSession 读回），
// 不是屏幕输出、不是返回值——这是任务书原话的要求，也是 #20 repro 本身的
// 观察方式（改一个不相关的配置字段、拒绝之后 provider_sessions 空了）。
//
// 三条覆盖：
//   - 早期拒绝（起进程之前，st 从未被赋值）之后，已建立的会话原样还在；
//   - 进程内指纹漂移（ErrIdentityDrift）拒绝之后，原会话原样还在，
//     这是裁决原文点名「必须仍然安全」的那一支；
//   - 从库读回的指纹不符、这一轮重建之后又失败（不是成功）时，旧行必须
//     已经在 Ask 读回的那一刻被显式删掉，不能留到 RecordTurn 结尾——因为
//     结尾这时候 session 是 nil，新语义下什么都不会做。这一条是本条裁决
//     引入的显式删除本身的正面证据：去掉 runner.go 里那次显式
//     DropProviderSession 调用，这条用例会红。

import (
	"context"
	"errors"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
)

// TestEarlyRejectionLeavesEstablishedSessionUntouched 是 #20 repro 本身的
// 直接回归：一个 agent 正常跑一轮建起会话，然后只把配置里的 permissions 从
// read 改成 full（配置本身合法，但 TranslatePermission 会拒——11c 第二片
// 之后 workspace 已经放行，full 才是仍然被拒的那一档），再问一次。
// 拒绝发生在 Ask 里 st 从未被赋值的那一支（TranslatePermission 那道闸在
// 读回 provider 会话之前），旧语义会把已建立的会话静默删掉；新语义下这一行
// 必须原样还在。
func TestEarlyRejectionLeavesEstablishedSessionUntouched(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮，建立会话")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮（建立会话）失败: %v", err)
	}
	before, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("前置条件不成立：第一轮之后应当读得到 provider session，实际 ok=%v err=%v", ok, err)
	}
	if before.ProviderSessionID == "" {
		t.Fatal("前置条件不成立：第一轮应当已经建立了一个非空的 provider session id")
	}

	// 配置本身合法，只是编排器不放行 full 这一档（06 第 2 节）——
	// TranslatePermission 会拒，拒绝发生在读回 provider 会话之前，st 是 nil。
	ac := cfg.Agents[testAgentClaude]
	ac.Permissions = config.PermissionFull
	cfg.Agents[testAgentClaude] = ac

	postUser(t, svc, convID, "第二轮，权限档已经不合法")
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if !errors.Is(err, ErrTierNotOffered) {
		t.Fatalf("errors.Is(err, ErrTierNotOffered) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("Spawner 累计调用次数 = %d，期望仍是 1（第二轮起进程之前就被拒绝）", len(fs.calls))
	}

	after, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("被拒绝之后这一行不该消失，实际 ok=%v err=%v（未决 #20 就是这个 bug：与会话无关的拒绝把已经花过钱的会话静默删掉）", ok, err)
	}
	if after.ProviderSessionID != before.ProviderSessionID {
		t.Errorf("被拒绝之后 provider_session_id 变了：之前 %q，之后 %q", before.ProviderSessionID, after.ProviderSessionID)
	}
	if after.CursorSeq != before.CursorSeq {
		t.Errorf("被拒绝之后 cursor_seq 变了：之前 %d，之后 %d", before.CursorSeq, after.CursorSeq)
	}
}

// TestIdentityDriftRefusalLeavesSessionUntouched 覆盖裁决原文点名「必须仍然
// 安全」的那一支：进程内已有记录（gate.st 里有）指纹不符时拒绝
// （ErrIdentityDrift），它传给 persistTurn 的 st 带着非空 ProviderSessionID，
// 走的是 SetProviderSession 原值回写，从未经过 session==nil 分支——这里要
// 证明「拒绝而不是静默重建」这条设计没有被 #20 的改动自己拆掉：拒绝的同时
// 会话必须原样还在，不能被删掉，否则下一轮照样重建，等于白拒。
func TestIdentityDriftRefusalLeavesSessionUntouched(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮，建立会话")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮（建立会话）失败: %v", err)
	}
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	// r.gate(key) 是 get-or-create，本身永远非 nil，所以不能再用
	// 「这个 key 在不在 map 里」当断言——那会对任何 key 都恒真、静默失效。
	// 必须换成看这条 gate 的 st 是不是真的被填了一条 provider 会话，这才是
	// 「第一轮之后进程内确实建立了一条 provider 会话」这件事的可判别证据。
	memSt := r.gate(key).st
	if memSt == nil || memSt.providerSession == "" {
		t.Fatal("前置条件不成立：第一轮之后这条 (会话, 成员) 的 gate.st 应当已经有一条非空的 provider 会话（进程内指纹漂移这一支要求它在内存里）")
	}
	before, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("前置条件不成立：第一轮之后应当读得到 provider session，实际 ok=%v err=%v", ok, err)
	}

	// 改一个进指纹的字段（Model），触发进程内指纹漂移：这条 (会话, 成员) 的
	// gate.st 里已有这一条，与新算出来的指纹不符，走 ErrIdentityDrift 那一支，
	// 不是从库读回那一支。
	ac := cfg.Agents[testAgentClaude]
	ac.Model = "claude-fable-5-1-changed"
	cfg.Agents[testAgentClaude] = ac

	postUser(t, svc, convID, "第二轮，model 已经改了")
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if !errors.Is(err, ErrIdentityDrift) {
		t.Fatalf("errors.Is(err, ErrIdentityDrift) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("Spawner 累计调用次数 = %d，期望仍是 1：进程内指纹漂移必须在起进程之前拒绝", len(fs.calls))
	}

	after, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("身份漂移拒绝之后这一行不该消失，实际 ok=%v err=%v", ok, err)
	}
	if after.ProviderSessionID != before.ProviderSessionID {
		t.Errorf("身份漂移拒绝之后 provider_session_id 变了：之前 %q，之后 %q（说明拒绝没能阻止会话被静默改动）", before.ProviderSessionID, after.ProviderSessionID)
	}
	if after.IdentityFP != before.IdentityFP {
		t.Errorf("身份漂移拒绝之后 identity_fp 变了：之前 %q，之后 %q（拒绝必须原值回写，不能被新指纹覆盖，否则下一轮就不会再检测到漂移）", before.IdentityFP, after.IdentityFP)
	}
}

// TestFromStoreIdentityMismatchThenFailureDropsStaleRowImmediately 覆盖
// 「从库读回的那条不符 -> 作废重建」这一支，且这一轮**随后失败**（不是
// 成功）。这是本条裁决新增的显式删除本身要证明的场景：若这一轮成功，
// SetProviderSession 的 upsert 会覆盖旧行，测不出「删」这个动作到底发生了
// 没有；只有失败轮才会暴露旧语义（session=nil 顺带删）与新语义（nil 不碰）
// 之间的真实差异——新语义下，若不在读回的那一刻显式删，这条旧行会在这一轮
// 失败之后继续留在库里，指纹已经不符却没有任何进程认领，正是未决 #20 要防
// 的孤儿状态。
func TestFromStoreIdentityMismatchThenFailureDropsStaleRowImmediately(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "问题，触发从库读回")

	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "stale-session-id-about-to-be-orphaned",
		CursorSeq:         99,
		CostTotalUSD:      100.0,
		HaveCostTotal:     true,
		IdentityFP:        "a-fingerprint-from-a-different-config",
		Spoken:            true,
	})

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(spawnResult{err: errors.New("exec: \"claude-fake\": executable file not found")})
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("这一轮应当报错（spawn 失败）")
	}
	if rec.Outcome != OutcomeSpawnFailed {
		t.Fatalf("Outcome = %v，期望 OutcomeSpawnFailed", rec.Outcome)
	}
	if !rec.Fresh {
		t.Fatal("前置条件不成立：指纹不符按 Fresh 重建，这一轮应当是 Fresh")
	}

	// 旁证：session_rebuilt 面包屑必须已经发出——它与显式删除是同一处代码
	// 路径（#18 加的面包屑、#20 加的显式删除，紧挨着做）。
	history, histErr := svc.History(context.Background(), convID, 0, 100)
	if histErr != nil {
		t.Fatalf("读历史失败: %v", histErr)
	}
	foundRebuiltNotice := false
	for i := range history {
		if history[i].Meta[MetaEvent] == "session_rebuilt" {
			foundRebuiltNotice = true
		}
	}
	if !foundRebuiltNotice {
		t.Fatal("库里没有找到 session_rebuilt 面包屑：显式删除应当与它同一时刻发生")
	}

	// 核心断言：旧行必须已经真的没了，不能留到这一轮失败之后。
	_, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("ProviderSession 报错，期望 ok=false 且 err=nil: %v", err)
	}
	if ok {
		t.Error("指纹不符重建、这一轮又失败之后，旧行仍然存在——说明显式 DropProviderSession 没有在读回的那一刻生效，留下了一条孤儿会话（未决 #20 要防的场景）")
	}
}
