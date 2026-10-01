// team_thought_visible_to_drift_test.go 覆盖 docs/12_STEP9_CONTRACT.md 3.8 与
// 3.9 第 8 条：只改 team 的 thought_visible_to，不触发身份漂移。
//
// 这条用例存在的理由是任务书原话：下一个人很可能出于"保险"把这个开关塞
// 进指纹，那一塞就是每改一次全队重付入场费（5 人量级 90k-240k token）。
//
// 关键设计决定：ThoughtVisibleTo 从 nil 改成 []config.Mode{config.ModeDiscussion}——
// 恰好是收件人 alice 自己的 Mode，不是随便挑一个不相关的 mode（比如
// judge）。这样 thoughtVisible(team, alice.Mode) 这个布尔值本身也从 false
// 变成 true，不只是底层的 []Mode 切片变了。如果未来的实现把"这个开关"塞
// 进指纹，不管塞的是原始切片本身，还是 thoughtVisible 算出来的那个布尔
// 值，这条用例都能抓到；如果像早先一版草稿那样改成一个与 alice 无关的
// mode（比如 judge），布尔值全程是 false，"塞计算出的布尔值"那种误实现
// 会因为布尔值从未变化而蒙混过关——那是一次真实会被抓到的空用例，这里
// 记一笔，提醒别人不要重犯。
//
// 断言用的是 rec1.IdentityFP 与 rec2.IdentityFP（Ask 内部真实算出来的值），
// 不是拿 identityFPFor 单独调两次去比较：identityFPFor 的冻结签名
// （p, agentID, ac, workDir, convID, tier, rules）本来就不收 TeamConfig，
// 脱离 Ask 单独调用来比较"前后指纹"对本用例要防的那类回归是天然测不到
// 的——不管 team 怎么改，两次独立调用传的 ac/rules 都相同，结果必然相等，
// 什么都没验。只有走真实的 Ask 两轮，才会真正暴露"实现是否偷偷把
// team 相关的东西塞进了这一轮算出来的指纹"这件事。
package turn

import (
	"context"
	"testing"

	"council/internal/config"
)

func TestChangingTeamThoughtVisibleToDoesNotDrift(t *testing.T) {
	cfg := newTestConfig()
	cfg.Teams = map[string]config.TeamConfig{
		"project": {DisplayName: "<project> 讨论组", Members: []string{testAgentClaude}},
	}
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	if err := svc.BindTeam(context.Background(), convID, "project"); err != nil {
		t.Fatalf("BindTeam 失败: %v", err)
	}
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	fs.enqueueSuccess(claudeT2JSON) // 同一会话第二轮，session_id 与 t1 相同。
	r.spawn = fs.spawn

	rec1, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第一轮 Ask 失败: %v", err)
	}
	if rec1.Outcome != OutcomeSpoke {
		t.Fatalf("Outcome = %v，期望 OutcomeSpoke（前置条件不成立，下面的比较无意义）", rec1.Outcome)
	}
	if rec1.IdentityFP == "" {
		t.Fatal("前置条件不成立：第一轮应当算出非空指纹")
	}
	before, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("前置条件不成立：第一轮之后应当读得到 provider session，实际 ok=%v err=%v", ok, err)
	}

	// 只改 team 的 thought_visible_to，team 的其它字段、agent 配置、规则
	// 一律不动。alice 的 Mode 是 discussion（newTestConfig 定的），这里把
	// 它自己的 mode 放进策略——理由见文件顶部注释。
	team := cfg.Teams["project"]
	team.ThoughtVisibleTo = []config.Mode{config.ModeDiscussion}
	cfg.Teams["project"] = team

	postUser(t, svc, convID, "第二轮，只改了 team 的 thought_visible_to")
	rec2, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第二轮 Ask 失败（说明只改 thought_visible_to 被误判成了身份漂移）: %v", err)
	}
	if rec2.Outcome != OutcomeSpoke {
		t.Fatalf("Outcome = %v，期望 OutcomeSpoke", rec2.Outcome)
	}
	if rec2.Fresh {
		t.Error("第二轮不该走 Fresh：改 thought_visible_to 不该触发会话重建" +
			"（它不进身份块、不进指纹，只进 Render 出来的正文，04 §0 规则 1）")
	}
	if rec2.IdentityFP != rec1.IdentityFP {
		t.Errorf("只改 team.ThoughtVisibleTo，两轮算出的 IdentityFP 逐字节不同："+
			"第一轮 %q，第二轮 %q——说明这个开关被塞进了指纹", rec1.IdentityFP, rec2.IdentityFP)
	}

	after, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil || !ok {
		t.Fatalf("第二轮之后应当仍读得到 provider session，实际 ok=%v err=%v", ok, err)
	}
	if after.ProviderSessionID != before.ProviderSessionID {
		t.Errorf("provider_sessions 的会话 id 变了：之前 %q，之后 %q——说明发生了重建",
			before.ProviderSessionID, after.ProviderSessionID)
	}
	if after.IdentityFP != before.IdentityFP {
		t.Errorf("provider_sessions 那一行落库的 identity_fp 变了：之前 %q，之后 %q",
			before.IdentityFP, after.IdentityFP)
	}
}
