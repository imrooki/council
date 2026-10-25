package serve

// permission_test.go 钉住「整轮权限覆盖」这一层：roundCfg 只改克隆出来的
// 那一份，参与成员与汇总成员都覆盖，空档位原样返回启动配置。

import (
	"testing"

	"council/internal/config"
)

func TestRoundCfgOverridesMembersAndSummary(t *testing.T) {
	cfg := &config.Config{
		Agents: map[string]config.AgentConfig{
			"a": {Provider: "claude", Model: "opus", Permissions: config.PermissionRead},
			"b": {Provider: "claude", Model: "haiku", Permissions: config.PermissionFull},
		},
		Summary: config.SummaryConfig{Agent: "b"},
	}

	got := roundCfg(cfg, []string{"a"}, "", "workspace")
	if got == cfg {
		t.Fatal("有权限档时返回了同一个指针，原配置会被改坏")
	}
	if got.Agents["a"].Permissions != config.PermissionWorkspace {
		t.Errorf("成员 a 的权限 = %q，期望 workspace", got.Agents["a"].Permissions)
	}
	// summary 为空串表示「用配置里的默认汇总成员」，那一个也要覆盖。
	if got.Agents["b"].Permissions != config.PermissionWorkspace {
		t.Errorf("默认汇总成员 b 的权限 = %q，期望 workspace", got.Agents["b"].Permissions)
	}
	if cfg.Agents["a"].Permissions != config.PermissionRead {
		t.Errorf("原配置被改了：a = %q", cfg.Agents["a"].Permissions)
	}

	// 没有覆盖时必须原样返回：一次升级不能悄悄改变现有轮次的行为。
	if roundCfg(cfg, []string{"a"}, "", "") != cfg {
		t.Error("空权限档返回了克隆，期望原指针")
	}

	// summary = none：这一轮没有汇总成员，不去动配置里的默认那一个。
	got = roundCfg(cfg, []string{"a"}, "none", "read")
	if got.Agents["b"].Permissions != config.PermissionFull {
		t.Errorf("summary=none 时动了默认汇总成员：b = %q", got.Agents["b"].Permissions)
	}
}
