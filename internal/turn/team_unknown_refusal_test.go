// team_unknown_refusal_test.go 覆盖 docs/12_STEP9_CONTRACT.md 3.6 与 3.9 第 6
// 条：会话绑定的 team 在配置里被删掉之后，下一轮 /ask 必须在起进程之前
// 拒绝，一分钱不花，且已经建立的 provider 会话不能被这次与会话本身无关
// 的拒绝顺带删掉——与 runner_reject_session_test.go 里未决 #20 那三条是
// 同一体例、同一批判据（库里那一行还在不在、内容是否原样），只是触发
// 拒绝的哨兵换成了 ErrTeamUnknown。
//
// 冻结形状：var ErrTeamUnknown = errors.New(...)（任务书原话，导出）。
//
// 插入点已由 coordinator 在 12_STEP9_CONTRACT.md 3.6（订正版）裁定：
// ErrTeamUnknown 检查落在指纹算出来**之后**——结构上只能这样，这条检查要
// 读 conv.TeamID，而 conv 是在 readAfter 之后才取的（runner.go 里
// r.svc.Conversation 那一行），fp 早在会话状态解析之前就算完了。也因此
// 这一轮落库的 turn_usage.identity_fp 必须非空：这一轮确实算到了指纹，
// 是在「身份已经定下、但这条会话绑的 team 查不到」这一步才倒下的，记成
// 空串就是撒谎。这条断言与片 2a 的
// TestAskRefusedBeforeFingerprintLeavesIdentityFPEmpty 是一对：那条立了
// 「空串 = 没算到」，这条立「算到了 = 非空」，两条合起来才把 identity_fp
// 这一列的语义钉住，缺一条都是半句话。
package turn

import (
	"context"
	"errors"
	"testing"

	"council/internal/config"
)

func TestAskRefusesWhenBoundTeamRemovedFromConfig(t *testing.T) {
	cfg := newTestConfig()
	cfg.Teams = map[string]config.TeamConfig{
		"project": {DisplayName: "<project> 讨论组", Members: []string{testAgentClaude}},
	}
	svc, dbPath := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮，建立会话")

	if err := svc.BindTeam(context.Background(), convID, "project"); err != nil {
		t.Fatalf("BindTeam 失败: %v", err)
	}

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

	// 把配置里的 team 删掉：会话仍然绑着 "project"（conversations.team_id 不受
	// 配置变化影响，那是库里的一列），但 cfg.Teams 里已经查不到它了。
	delete(cfg.Teams, "project")

	postUser(t, svc, convID, "第二轮，绑定的 team 已经从配置里删掉")
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)

	if !errors.Is(err, ErrTeamUnknown) {
		t.Fatalf("errors.Is(err, ErrTeamUnknown) 应当为真，实际: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if rec.WallMS != 0 {
		t.Fatalf("WallMS = %d，期望 0——进程根本没起，钱没花", rec.WallMS)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("Spawner 累计调用次数 = %d，期望仍是 1（第二轮起进程之前就该被拒绝，一次都不该再调）", len(fs.calls))
	}
	if rec.TurnID == "" {
		t.Fatal("前置条件不成立：TurnID 为空，persistTurn 靠它判断要不要落库，下面查 turn_usage 无从查起")
	}

	// coordinator 裁决：ErrTeamUnknown 落在指纹算出来之后，这一轮落库的
	// identity_fp 必须非空——与片 2a 的
	// TestAskRefusedBeforeFingerprintLeavesIdentityFPEmpty（空串 = 没算到）
	// 是同一列语义的另一半。
	_, identityFP := tstTurnUsageRoundAndIdentity(t, dbPath, rec.TurnID)
	if identityFP == "" {
		t.Error("turn_usage.identity_fp 是空串，期望非空——这一轮在身份指纹算出来之后才因为" +
			"team 查不到被拒绝，空串在这里的含义是「没算到指纹」（片 2a 立的），" +
			"这一轮确实算到了，记成空串就是撒谎")
	}

	after, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("team 未知被拒绝之后这一行不该消失，实际 ok=%v err=%v"+
			"（与未决 #20 同一类 bug：与会话本身无关的拒绝把已经花过钱的会话静默删掉）", ok, err)
	}
	if after.ProviderSessionID != before.ProviderSessionID {
		t.Errorf("被拒绝之后 provider_session_id 变了：之前 %q，之后 %q", before.ProviderSessionID, after.ProviderSessionID)
	}
	if after.CursorSeq != before.CursorSeq {
		t.Errorf("被拒绝之后 cursor_seq 变了：之前 %d，之后 %d"+
			"（说明第二轮的新消息被静默推进了游标，那条消息以后再也不会送给这个 agent 了）", before.CursorSeq, after.CursorSeq)
	}
	if after.IdentityFP != before.IdentityFP {
		t.Errorf("被拒绝之后 identity_fp 变了：之前 %q，之后 %q", before.IdentityFP, after.IdentityFP)
	}
}
