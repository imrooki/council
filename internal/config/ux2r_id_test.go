package config

// 验收 31（validate.go 这一侧）：id 允许点号（契约第 5、6.4 节）。
// 背对背：断言只来自冻结契约 E:\multi_model_cowork\ux2r_contract.md 的文字。

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

// ux2rFrozenIDPattern 是契约第 5 节冻结的那一条。
const ux2rFrozenIDPattern = `^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$`

var ux2rIDCases = []struct {
	id   string
	want bool
}{
	{"grok-4.7", true},
	{"grok-4.7.", false},
	{".grok", false},
	{"gk-", true},
	{"gk_", true},
	{"a..b", true},
	{"a.-", true},
	{"a", true},
	{"a.", false},
	{"-gk", false},
}

// ux2rIDRejected 报告 Validate 的结果里有没有对 path 这个 id 的「does not match」错误。
func ux2rIDRejected(t *testing.T, err error, path string) bool {
	t.Helper()
	if err == nil {
		return false
	}
	var ves ValidationErrors
	if !errors.As(err, &ves) {
		t.Fatalf("Validate 返回的应当是 ValidationErrors，实际 %T: %v", err, err)
	}
	for _, ve := range ves {
		if ve.Path == path && strings.Contains(ve.Msg, "does not match") {
			return true
		}
	}
	return false
}

// 验收 31（validate 侧）：常量就是冻结的那一条；providers / agents / teams 三张表
// 经 Validate 各判一次正反例。
func TestUx2rValidateIDPattern(t *testing.T) {
	if idPattern != ux2rFrozenIDPattern {
		t.Fatalf("idPattern 应当是 %q，实际 %q", ux2rFrozenIDPattern, idPattern)
	}
	re := regexp.MustCompile(idPattern)
	for _, c := range ux2rIDCases {
		if got := re.MatchString(c.id); got != c.want {
			t.Errorf("idPattern 对 %q 判 %v，期望 %v", c.id, got, c.want)
		}

		cfg := tstValidConfig()
		cfg.Agents[c.id] = cfg.Agents["theory"]
		if got := !ux2rIDRejected(t, cfg.Validate(), "agents."+c.id); got != c.want {
			t.Errorf("Validate 对 agent id %q 判合法=%v，期望 %v", c.id, got, c.want)
		}

		cfg = tstValidConfig()
		cfg.Providers[c.id] = ProviderConfig{Command: "cmd-ux2r"}
		if got := !ux2rIDRejected(t, cfg.Validate(), "providers."+c.id); got != c.want {
			t.Errorf("Validate 对 provider id %q 判合法=%v，期望 %v", c.id, got, c.want)
		}

		cfg = tstValidConfig()
		cfg.Teams[c.id] = TeamConfig{Members: []string{"theory"}}
		if got := !ux2rIDRejected(t, cfg.Validate(), "teams."+c.id); got != c.want {
			t.Errorf("Validate 对 team id %q 判合法=%v，期望 %v", c.id, got, c.want)
		}
	}
}

// 一份含点号 id 的配置整体能过 Validate：agent 引用点号 provider、team 引用点号 agent。
func TestUx2rValidateDottedIDsEndToEnd(t *testing.T) {
	cfg := tstValidConfig()
	cfg.Providers["pv.x"] = ProviderConfig{Command: "cmd-ux2r"}
	a := cfg.Agents["theory"]
	a.Provider = "pv.x"
	cfg.Agents["grok-4.7"] = a
	cfg.Teams["t.1"] = TeamConfig{Members: []string{"grok-4.7", "theory"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("含点号 id 的配置应当通过校验，实际: %v", err)
	}
}
