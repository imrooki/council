package repl

// i18n_repl_test.go 钉住交互入口的中英分叉与中文默认的逐字节兼容：
//
//   - 同一条命令在 zh / en 下输出必须不同，en 下不能出现汉字；
//   - 不注入语言（SetLang 都没调过）与注入 zh 必须逐字节相同 —— 这是
//     「默认 lang 下所有输出与改造前一致」这条硬约束在 REPL 侧的护栏。
//
// 只断言「有英文、没中文、命令名不变」，不锁整段排版：排版由
// WriteCommandTable 一份定义，逐字节比对放在中文那一条里。

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
	"unicode"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/i18n"
	"council/internal/provider"
	"council/internal/turn"
)

// hasHan 报告字符串里有没有汉字。en 输出里出现任何一个汉字都算漏翻。
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// runLang 跑一次 REPL 并注入语言；lang 为空串时连 SetLang 都不调，
// 用来与「显式 zh」对照。
func runLang(t *testing.T, ctx context.Context, cfg *config.Config, in string, lang i18n.Lang) string {
	t.Helper()
	return runLangSvc(t, ctx, cfg, nil, in, lang)
}

// runLangSvc 是 runLang 带持久化的版本：命令细则（表格、失败原因、列表）
// 需要 Service 才走得到。
func runLangSvc(t *testing.T, ctx context.Context, cfg *config.Config, svc *conversation.Service, in string, lang i18n.Lang) string {
	t.Helper()

	var out bytes.Buffer
	r := New(cfg, svc, nil, strings.NewReader(in), &out, RulesContext{})
	if lang != "" {
		r.SetLang(lang)
	}
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 返回 %v", err)
		}
	case <-time.After(testREPLTimeout):
		t.Fatalf("REPL.Run 在 %v 内没有返回", testREPLTimeout)
	}
	return out.String()
}

// TestREPLChineseDefaultIsByteIdentical 是硬约束的回归闸：不调用 SetLang 的
// 输出必须与显式 zh 逐字节相同（语言为中文时的 fallback 与目录值一致）。
func TestREPLChineseDefaultIsByteIdentical(t *testing.T) {
	cfg := newTestREPLConfig()
	for _, input := range []string{
		"/help\n",
		"/no_such_command\n",
		"/copy\n",
		"/providers\n",
		"/quit\n",
	} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			def := runLang(t, context.Background(), cfg, input, "")
			zh := runLang(t, context.Background(), cfg, input, i18n.LangZh)
			if def != zh {
				t.Fatalf("默认语言与显式 zh 输出不同\n默认:\n%s\n显式 zh:\n%s", def, zh)
			}
			if !hasHan(def) {
				t.Errorf("中文输出里连一个汉字都没有，用例失去判别力：%q", def)
			}
		})
	}
}

// TestREPLHelpEnglish 检查 /help 的英文版：摘要翻成英文、命令名与参数不动。
func TestREPLHelpEnglish(t *testing.T) {
	cfg := newTestREPLConfig()
	zh := runLang(t, context.Background(), cfg, "/help\n", i18n.LangZh)
	en := runLang(t, context.Background(), cfg, "/help\n", i18n.LangEn)

	if zh == en {
		t.Fatal("zh 与 en 的 /help 输出相同，说明没有走到语言分叉")
	}
	if hasHan(en) {
		t.Fatalf("en 的 /help 里出现汉字:\n%s", en)
	}
	for _, want := range []string{
		"List every command", // help 的摘要
		"Exit council",       // quit 的摘要
		"/help", "/quit", "/copy", "/resume",
		"/team use <team-id>", "[--rounds N]", // 参数形状必须原样
	} {
		if !strings.Contains(en, want) {
			t.Errorf("en 的 /help 缺少 %q:\n%s", want, en)
		}
	}
	if !hasHan(zh) {
		t.Fatalf("zh 的 /help 里没有汉字，说明取错了目录:\n%s", zh)
	}
}

// TestREPLRepresentativeMessagesEnglish 覆盖每次输入都可能看到的提示与
// 几条代表性命令输出：未知命令、告别行、行式 /copy、无持久化。
func TestREPLRepresentativeMessagesEnglish(t *testing.T) {
	cfg := newTestREPLConfig()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "未知命令", input: "/no_such_command\n", want: "Unknown command"},
		{name: "告别行", input: "/quit\n", want: "Bye"},
		{name: "行式 /copy", input: "/copy\n", want: "no clipboard"},
		{name: "无持久化", input: "/who\n", want: "Storage is not ready"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runLang(t, context.Background(), cfg, tc.input, i18n.LangEn)
			if hasHan(out) {
				t.Fatalf("en 输出里出现汉字:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("en 输出缺少 %q:\n%s", tc.want, out)
			}
		})
	}
}

// TestHelpSummariesMatchCatalog 防两张表走失：/help 用到的 tui.cmd.<name>.summary
// 在中文目录里的值必须与注册表里的中文摘要逐字节相同。中文目录写错一个字，
// 这里红，而不是等某个中文输出用例在别处红。
func TestHelpSummariesMatchCatalog(t *testing.T) {
	for _, spec := range TUISpecs() {
		key := "tui.cmd." + spec.Name + ".summary"
		if got := i18n.T(i18n.LangZh, key); got != spec.Summary {
			t.Errorf("%s 的中文值 = %q，注册表是 %q", key, got, spec.Summary)
		}
	}
}

// detailCommands 是本片覆盖的细则输出所需的一串命令。它先建一个会话，再逐条
// 打一遍：没有 Runner 的几条走「provider 调用未就绪」，其余走真实的表格、
// 空态与失败原因，足以让「英文里有没有汉字」这条闸有判别力。
const detailCommands = `/new bench
/add
/ask
/session
/round
/rules
/cost
/log
/to
/team use
/resume
/config
`

// TestREPLChineseDefaultIsByteIdenticalCommands 是硬约束在命令细则上的回归闸：
// 不注入语言与显式 zh 的输出必须逐字节相同。每条命令在中文下都必须至少有一个
// 汉字，否则这一条没有判别力。
func TestREPLChineseDefaultIsByteIdenticalCommands(t *testing.T) {
	cfg := newTestREPLConfig()
	defaultSvc, _ := newTestService(t)
	def := runLangSvc(t, context.Background(), cfg, defaultSvc, detailCommands, "")
	zhSvc, _ := newTestService(t)
	zh := runLangSvc(t, context.Background(), cfg, zhSvc, detailCommands, i18n.LangZh)

	if def != zh {
		t.Fatalf("默认语言与显式 zh 的命令输出不同\n默认:\n%s\n显式 zh:\n%s", def, zh)
	}
	if !hasHan(def) {
		t.Errorf("中文命令输出里连一个汉字都没有，用例失去判别力：%q", def)
	}
}

// TestREPLCommandDetailEnglishHasNoHan 是本片的主闸：同一条命令序列在 en 下
// 一个汉字都不能有，而且每个命令都要留下自己的英文片段（证明走到了那条分支，
// 不是被前一条命令的报错挡住）。
func TestREPLCommandDetailEnglishHasNoHan(t *testing.T) {
	cfg := newTestREPLConfig()
	svc, _ := newTestService(t)
	en := runLangSvc(t, context.Background(), cfg, svc, detailCommands, i18n.LangEn)

	if hasHan(en) {
		t.Fatalf("en 的命令细则里出现汉字:\n%s", en)
	}
	for _, want := range []string{
		"Created conversation",           // /new
		"add with /add <number|full name|prefix>", // /add
		"Provider calls are not ready",   // /ask、/session、/round、/rules 的前置条件
		"no accounting rows yet",         // /cost
		"has no messages yet",            // /log
		"usage: /to",                     // /to
		"Usage: /team use",               // /team use
		"No project directory; listing",  // /resume
		"Config file:",                   // /config
	} {
		if !strings.Contains(en, want) {
			t.Errorf("en 的命令细则缺少 %q:\n%s", want, en)
		}
	}
}

// TestREPLDetailHelpersEnglish 覆盖需要 Runner / 真实输出才走得到的细则文案：
// 预算表、结算块、拒绝整轮、哨兵翻译、登录状态、规则三态、配置层、恢复提示。
// 这些函数本身现在都带 Lang 变体，直接喂合成数据比驱动一次假 CLI 便宜得多。
func TestREPLDetailHelpersEnglish(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	rows := []turn.BudgetRow{
		{AgentID: "ag_a", Calibrated: true, InputTokens: intPtr(1234), Cold: true, InFlight: true},
		{AgentID: "ag_b", ArgvHeadroom: intPtr(-7)},
	}
	settled := turn.RoundSettlement{
		Spawned: 1, HaveIn: true, InputTokens: 42, HaveCost: true, CostUSD: 0.25,
		UsageUnknown: 1, ColdUnknown: 1, Cold: 0, CostUnreported: 1,
	}
	rec := turn.Record{
		AgentID: "ag_a",
		DeniedActions: []provider.DeniedAction{
			{Action: "write", DisplayName: "a.txt"},
		},
		Message: &conversation.Message{
			Text: "x",
			Meta: map[string]any{turn.MetaCompletion: "truncated"},
		},
	}
	cfg := newTestREPLConfig()
	cfg.Teams["tm_alpha"] = config.TeamConfig{
		DisplayName:      "Team Alpha",
		Members:          []string{"ag_alpha"},
		ThoughtVisibleTo: []config.Mode{config.ModeDiscussion},
	}
	cfg.StoreThought = false

	cases := []struct {
		name string
		text string
	}{
		{"预算表", renderBudgetLang(i18n.LangEn, rows)},
		{"结算块", renderSettlementLang(i18n.LangEn, settled, 2, []string{"ag_a"}, "speaking round")},
		{"拒绝整轮", argvRefusalLang(i18n.LangEn, rows)},
		{"哨兵-在飞", askCauseLang(i18n.LangEn, conversation.ErrInflightConflict)},
		{"哨兵-无新内容", askCauseLang(i18n.LangEn, turn.ErrNoDelta)},
		{"哨兵-未知成员", askCauseLang(i18n.LangEn, turn.ErrAgentUnknown)},
		{"哨兵-身份漂移", askCauseLang(i18n.LangEn, turn.ErrIdentityDrift)},
		{"哨兵-前缀改写", askCauseLang(i18n.LangEn, turn.ErrPrefixRewrite)},
		{"哨兵-档位", askCauseLang(i18n.LangEn, turn.ErrTierNotOffered)},
		{"被拒工具调用", deniedActionsNoteLang(i18n.LangEn, rec)},
		{"截断注记", completionNoteLang(i18n.LangEn, rec)},
		{"登录-订阅", LoginTextLang(i18n.LangEn, provider.LoginInfo{State: provider.LoginYes, Subscription: "pro"})},
		{"登录-API key", LoginTextLang(i18n.LangEn, provider.LoginInfo{State: provider.LoginAPIKey, KeySource: "env"})},
		{"登录-否", LoginTextLang(i18n.LangEn, provider.LoginInfo{State: provider.LoginNo})},
		{"登录-未知", LoginTextLang(i18n.LangEn, provider.LoginInfo{})},
		{"登录命令", LoginCommandLang(i18n.LangEn, "agy")},
		{"规则-隔离", isolationLabelLang(i18n.LangEn, provider.SupportNo)},
		{"规则-缓存", cacheLabelLang(i18n.LangEn, provider.SupportYes)},
		{"规则-余量", headroomLabelLang(i18n.LangEn, turn.ProviderRulesInfo{HeadroomBytes: 99})},
		{"规则-文件", ruleFileLineLang(i18n.LangEn, "x.md", false, 0)},
		{"规则-未配置", localRuleFileLineLang(i18n.LangEn, config.Rules{})},
		{"规则-thought", storeThoughtVisibilityHintLang(i18n.LangEn, cfg)},
		{"配置层", configLayersNoticeLang(i18n.LangEn, []string{"a.toml", "b.toml"})},
		{"配置层-错误前缀", configReadErrPrefixLang(i18n.LangEn, "failed", []config.Layer{{Path: "a"}, {Path: "b"}})},
		{"恢复-未绑定", resumeUnboundHint(i18n.LangEn, 2, false)},
		{"恢复-编号提示", resumeNumberHint(i18n.LangEn, false)},
		{"恢复-归档标记", resumeTitleCell(i18n.LangEn, "t", true)},
		{"识别-超时", discoveryFailureTextLang(i18n.LangEn, provider.ErrDiscoveryTimeout, time.Second)},
		{"识别-空", discoveryFailureTextLang(i18n.LangEn, provider.ErrDiscoveryEmpty, time.Second)},
		{"识别-认不出", discoveryFailureTextLang(i18n.LangEn, provider.ErrDiscoveryUnparseable, time.Second)},
		{"id 检查", config.NewAgentIDProblemLang(i18n.LangEn, cfg, "bad id!")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.text == "" {
				t.Fatalf("%s 的英文输出是空的，用例失去判别力", tc.name)
			}
			if hasHan(tc.text) {
				t.Errorf("%s 的英文输出出现汉字: %q", tc.name, tc.text)
			}
		})
	}

	// 中文一侧不能退化成英文：抽两条最有代表性的比一比。
	if zh := renderBudgetLang(i18n.LangZh, rows); !hasHan(zh) || zh == renderBudgetLang(i18n.LangEn, rows) {
		t.Errorf("预算表中文输出没有分叉: %q", zh)
	}
	if zh := askCauseLang(i18n.LangZh, turn.ErrNoDelta); !hasHan(zh) || zh == askCauseLang(i18n.LangEn, turn.ErrNoDelta) {
		t.Errorf("哨兵翻译的中文输出没有分叉: %q", zh)
	}
}

// TestOutcomeWordLangAllOutcomes 钉住 repl 侧结局词的中英分叉（TUI 看板与
// 简洁档失败行共用这一份）。
func TestOutcomeWordLangAllOutcomes(t *testing.T) {
	for _, o := range []turn.Outcome{
		turn.OutcomeUnknown, turn.OutcomeSpoke, turn.OutcomePostRejected,
		turn.OutcomeProviderFailed, turn.OutcomeUnparsable, turn.OutcomeTimeout,
		turn.OutcomeSpawnFailed, turn.OutcomeRefused, turn.OutcomeCanceled, turn.OutcomeLost,
	} {
		zh := OutcomeWordLang(i18n.LangZh, o)
		en := OutcomeWordLang(i18n.LangEn, o)
		if zh == en {
			t.Errorf("%v 的中英结局词相同: %q", o, zh)
		}
		if !hasHan(zh) || hasHan(en) {
			t.Errorf("%v 的结局词分叉不对: zh=%q en=%q", o, zh, en)
		}
	}
	if got := OutcomeWord(turn.OutcomeSpoke); got != "完成" {
		t.Errorf("OutcomeWord 旧签名的中文值 = %q，期望 完成", got)
	}
}
