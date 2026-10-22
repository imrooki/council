package headless_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/headless"
)

// optionsCfg 是 options_test 专用的一份手写配置。它不落盘、不过 Validate：
// Parse 只用到 Agents / Teams / Summary 三张表。
func optionsCfg(withSummary bool) *config.Config {
	cfg := &config.Config{
		Agents: map[string]config.AgentConfig{
			"alpha": {DisplayName: "Alpha", Provider: "claude", Model: "opus", Effort: "high",
				Permissions: config.PermissionRead, Mode: config.ModeDiscussion},
			"beta": {DisplayName: "Beta", Provider: "claude", Model: "sonnet", Effort: "high",
				Permissions: config.PermissionRead, Mode: config.ModeDiscussion},
			"judge": {DisplayName: "Judge", Provider: "claude", Model: "opus", Effort: "high",
				Permissions: config.PermissionRead, Mode: config.ModeJudge},
		},
		Teams: map[string]config.TeamConfig{
			"duo": {DisplayName: "Duo", Members: []string{"beta", "alpha"}},
		},
	}
	if withSummary {
		cfg.Summary = config.SummaryConfig{Agent: "judge"}
	}
	return cfg
}

// TestParse_用法错误 覆盖任务书 §6 点名的六类用法错误，以及超时与
// round 下关汇总。每一条都要求错误类型是 UsageError，退出码映射为 2。
func TestParse_用法错误(t *testing.T) {
	cases := []struct {
		name string
		args []string
		cfg  *config.Config
		want string
	}{
		{"未知成员", []string{"-p", "q", "-members", "nope"}, optionsCfg(true), "nope"},
		{"预算低于下限", []string{"-p", "q", "-budget-usd", "0.01"}, optionsCfg(true), "0.5"},
		{"round 无汇总成员", []string{"-p", "q", "-mode", "round"}, optionsCfg(false), "汇总"},
		{"members 与 team 同时给", []string{"-p", "q", "-members", "alpha", "-team", "duo"}, optionsCfg(true), "-members"},
		{"p 与位置参数同时给", []string{"-p", "q", "hello"}, optionsCfg(true), "-p"},
		{"mode 非法", []string{"-p", "q", "-mode", "debate"}, optionsCfg(true), "-mode"},
		{"ask 下 rounds 不为 1", []string{"-p", "q", "-rounds", "2"}, optionsCfg(true), "-rounds"},
		{"round 下 summary none", []string{"-p", "q", "-mode", "round", "-summary", "none"}, optionsCfg(true), "none"},
		{"timeout 不为正", []string{"-p", "q", "-timeout", "0s"}, optionsCfg(true), "-timeout"},
		{"未知团队", []string{"-p", "q", "-team", "ghost"}, optionsCfg(true), "ghost"},
		{"未知汇总成员", []string{"-p", "q", "-summary", "ghost"}, optionsCfg(true), "ghost"},
		{"全 judge 名单", []string{"-p", "q", "-members", "judge"}, optionsCfg(true), "judge"},
		{"p 空串加位置参数", []string{"-p", "", "hello"}, optionsCfg(true), "-p"},
		{"members 空串加 team", []string{"-p", "q", "-members", "", "-team", "duo"}, optionsCfg(true), "-members"},
		{"members 空名单", []string{"-p", "q", "-members", ""}, optionsCfg(true), "名单"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := headless.Parse(tc.args, tc.cfg)
			if err == nil {
				t.Fatalf("Parse(%v) 应当返回用法错误，实际 nil", tc.args)
			}
			var ue *headless.UsageError
			if !errors.As(err, &ue) {
				t.Fatalf("Parse(%v) 的错误类型应是 *headless.UsageError，实际 %T: %v", tc.args, err, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Parse(%v) 的错误里应含 %q，实际 %q", tc.args, tc.want, err.Error())
			}
			if got := headless.ExitCode(err); got != 2 {
				t.Errorf("用法错误的退出码应为 2，实际 %d", got)
			}
		})
	}
}

// TestParse_默认值与位置参数 钉住 flag 默认值和 `--` 后的 prompt 写法。
// 默认值只在 Parse 这一层定，Run 不会再改。
func TestParse_默认值与位置参数(t *testing.T) {
	opts, err := headless.Parse([]string{"--", "一加一", "等于几"}, optionsCfg(true))
	if err != nil {
		t.Fatalf("Parse 位置参数失败: %v", err)
	}
	if opts.Prompt != "一加一 等于几" {
		t.Errorf("位置参数应拼成 %q，实际 %q", "一加一 等于几", opts.Prompt)
	}
	if opts.Mode != "ask" {
		t.Errorf("-mode 默认应为 ask，实际 %q", opts.Mode)
	}
	if opts.Rounds != 1 {
		t.Errorf("-rounds 默认应为 1，实际 %d", opts.Rounds)
	}
	if opts.BudgetUSD != 5 {
		t.Errorf("-budget-usd 默认应为 5，实际 %v", opts.BudgetUSD)
	}
	if opts.Timeout != 10*time.Minute {
		t.Errorf("-timeout 默认应为 10m，实际 %v", opts.Timeout)
	}
	if opts.JSON || opts.Verbose {
		t.Errorf("-json / -verbose 默认应为 false，实际 %v / %v", opts.JSON, opts.Verbose)
	}
}

// TestParse_团队 钉 D2 的 flag 层：-team 原样落在 Options.Team 里；
// 成员如何展开、按什么顺序、如何去重，由 resolve_test.go 直接测纯函数。
func TestParse_团队(t *testing.T) {
	opts, err := headless.Parse([]string{"-p", "q", "-team", "duo"}, optionsCfg(true))
	if err != nil {
		t.Fatalf("Parse -team 失败: %v", err)
	}
	if opts.Team != "duo" {
		t.Errorf("Team 应为 duo，实际 %q", opts.Team)
	}
}
