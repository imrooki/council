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
	"council/internal/i18n"
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

	var out bytes.Buffer
	r := New(cfg, nil, nil, strings.NewReader(in), &out, RulesContext{})
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
