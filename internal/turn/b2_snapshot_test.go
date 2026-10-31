// B2 回归：身份指纹必须用本轮入场时拍下的配置快照，不能在算完 ac 之后
// 再回头读 r.cfg 取 provider args/env。后者既是 -race 能抓到的数据竞争，
// 也会让 argv 用旧配置、指纹用新配置，改了 provider 参数会话照样续。

package turn

import (
	"context"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

func TestB2FingerprintUsesRoundSnapshot(t *testing.T) {
	cfg1 := newTestConfig()
	cfg2 := newTestConfig()
	cfg2.Providers["claude"] = config.ProviderConfig{
		Command: "claude",
		Args:    []string{"--profile", "new"},
	}

	svc, _ := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "task-b2")

	r := newTestRunner(t, cfg1, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	p, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("provider.Lookup: %v", err)
	}
	tier, err := TranslatePermission(cfg1.Agents[testAgentClaude].Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission: %v", err)
	}
	fpFor := func(cfg *config.Config) string {
		ac := cfg.Agents[testAgentClaude]
		fp, fpErr := identityFPAt(p, testAgentClaude, ac,
			r.dirFor(convID, testAgentClaude, tier),
			r.roleDirFor(testAgentClaude),
			tier, RulesBlocks{}, providerExtras(cfg, ac.Provider)...)
		if fpErr != nil {
			t.Fatalf("identityFPAt: %v", fpErr)
		}
		return fp
	}
	want1, want2 := fpFor(cfg1), fpFor(cfg2)
	if want1 == want2 {
		t.Fatal("前提不成立：两份配置的指纹相同，测不出串味")
	}

	// 在 Ask 的整个窗口里不停换配置。旧实现会在指纹那一步裸读 r.cfg，
	// 与这里的写并发（-race 红），并且可能拼出一个既不是 cfg1 也不是
	// cfg2 的混合指纹。
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				r.SetConfig(cfg2)
				r.SetConfig(cfg1)
			}
		}
	}()

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	close(stop)
	<-done
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if rec.IdentityFP != want1 && rec.IdentityFP != want2 {
		t.Fatalf("IdentityFP = %q，既不等于 cfg1 的 %q 也不等于 cfg2 的 %q："+
			"这是同一轮里 argv/指纹用了两份配置的混合值（B2）", rec.IdentityFP, want1, want2)
	}
}
