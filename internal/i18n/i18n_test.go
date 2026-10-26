package i18n

import (
	"sort"
	"strings"
	"testing"
)

// TestCatalogsHaveSameKeys 是本片唯一的防漂移闸：中英两份目录的 key 集合
// 必须逐条相同。少一条英文时页面会静默退回中文（T 不 panic 的代价），
// 没有这条用例就没人会发现。
func TestCatalogsHaveSameKeys(t *testing.T) {
	if len(zh) != len(en) {
		t.Fatalf("中英目录 key 数不同：zh=%d en=%d", len(zh), len(en))
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("英文目录缺少 key %q", key)
		}
	}
	for key := range en {
		if _, ok := zh[key]; !ok {
			t.Errorf("中文目录多出（英文独有）key %q", key)
		}
	}

	keys := Keys()
	if len(keys) != len(zh) {
		t.Fatalf("Keys() 返回 %d 条，期望 %d 条", len(keys), len(zh))
	}
	if !sort.StringsAreSorted(keys) {
		t.Errorf("Keys() 未按字典序排序: %v", keys)
	}
	for _, key := range keys {
		if key == "" {
			t.Error("Keys() 里出现了空字符串")
		}
	}
}

// TestTMissingKeyReturnsKey 钉住「缺翻译不许让程序崩」：回退到 key 本身。
func TestTMissingKeyReturnsKey(t *testing.T) {
	const missing = "ui.does.not.exist"
	for _, lang := range []Lang{LangZh, LangEn, Lang("fr"), ""} {
		if got := T(lang, missing); got != missing {
			t.Errorf("T(%q, %q) = %q，期望 key 本身", lang, missing, got)
		}
	}
	if got := T(LangEn, "ui.messages.copy"); got != "Copy" {
		t.Errorf("T(en, ui.messages.copy) = %q，期望 Copy", got)
	}
}

// TestUIMessagesOnlyContainsUIKeys 把「服务端文案不进浏览器」这条契约
// 落成可执行断言：目录里的每一条 key 目前都必须是 ui. 前缀，将来加服务端
// 文案时这张表不能跟着变大。
func TestUIMessagesOnlyContainsUIKeys(t *testing.T) {
	for _, lang := range []Lang{LangZh, LangEn} {
		messages := UIMessages(lang)
		if len(messages) != len(zh) && lang == LangZh {
			t.Errorf("UIMessages(zh) 返回 %d 条，期望全部 %d 条（本片目录全是界面文案）",
				len(messages), len(zh))
		}
		for key := range messages {
			if len(key) < 3 || key[:3] != "ui." {
				t.Errorf("UIMessages(%q) 里出现了非界面 key %q", lang, key)
			}
		}
	}
}

// TestResolvePriority 覆盖四级优先级的每一条边界：flag > COUNCIL_LANG >
// config > 系统区域。
func TestResolvePriority(t *testing.T) {
	tests := []struct {
		name       string
		flag       string
		env        string
		config     string
		localeVars map[string]string
		want       Lang
	}{
		{
			name: "flag 压过环境变量与配置",
			flag: "en", env: "zh", config: "zh",
			localeVars: map[string]string{"LANG": "zh_CN.UTF-8"},
			want:       LangEn,
		},
		{
			name: "flag 为空时环境变量压过配置",
			flag: "", env: "en", config: "zh",
			localeVars: map[string]string{"LANG": "zh_CN.UTF-8"},
			want:       LangEn,
		},
		{
			name: "flag 与环境变量都没有时配置生效",
			flag: "auto", env: "auto", config: "en",
			localeVars: map[string]string{"LANG": "zh_CN.UTF-8"},
			want:       LangEn,
		},
		{
			name: "三级都没有时看系统区域 LANG",
			flag: "", env: "", config: "auto",
			localeVars: map[string]string{"LANG": "en_US.UTF-8"},
			want:       LangEn,
		},
		{
			name: "LC_ALL 压过 LANG",
			flag: "", env: "", config: "",
			localeVars: map[string]string{"LC_ALL": "en_GB.UTF-8", "LANG": "zh_CN.UTF-8"},
			want:       LangEn,
		},
		{
			name: "LC_ALL=C 视为没有区域信息，继续看 LANG",
			flag: "", env: "", config: "",
			localeVars: map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"},
			want:       LangEn,
		},
		{
			name: "一切都没有时默认中文",
			flag: "", env: "", config: "",
			want: LangZh,
		},
		{
			name: "大小写与空白归一",
			flag: "  EN  ", env: "", config: "",
			want: LangEn,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setLocaleEnv(t, tc.localeVars)
			t.Setenv(EnvLang, tc.env)
			if got := Resolve(tc.flag, tc.config); got != tc.want {
				t.Errorf("Resolve(%q, %q) = %q，期望 %q", tc.flag, tc.config, got, tc.want)
			}
		})
	}
}

// TestResolveInvalidFallsBackToDefault 钉住「非 en/zh 一律按默认处理」：
// 非法值不再往下一级看，直接中文；auto 才是「继续往下」。
func TestResolveInvalidFallsBackToDefault(t *testing.T) {
	tests := []struct {
		name   string
		flag   string
		env    string
		config string
	}{
		{name: "flag 乱写", flag: "fr", env: "en", config: "en"},
		{name: "环境变量乱写", flag: "", env: "klingon", config: "en"},
		{name: "配置乱写", flag: "", env: "", config: "ja"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setLocaleEnv(t, map[string]string{"LANG": "en_US.UTF-8"})
			t.Setenv(EnvLang, tc.env)
			if got := Resolve(tc.flag, tc.config); got != Default {
				t.Errorf("Resolve(%q, %q) = %q，期望默认 %q", tc.flag, tc.config, got, Default)
			}
		})
	}
}

// TestResolveSystemLocaleVariants 覆盖 localeBase 的几种真实写法。
func TestResolveSystemLocaleVariants(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Lang
	}{
		{name: "codeset", raw: "en_US.UTF-8", want: LangEn},
		{name: "modifier", raw: "zh_CN.UTF-8@cjk", want: LangZh},
		{name: "dash", raw: "en-GB", want: LangEn},
		{name: "bare", raw: "zh", want: LangZh},
		{name: "unsupported", raw: "fr_FR.UTF-8", want: LangZh},
		{name: "posix", raw: "POSIX", want: LangZh},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setLocaleEnv(t, map[string]string{"LC_ALL": tc.raw})
			t.Setenv(EnvLang, "")
			if got := Resolve("", ""); got != tc.want {
				t.Errorf("Resolve 在 LC_ALL=%q 时 = %q，期望 %q", tc.raw, got, tc.want)
			}
		})
	}
}

// setLocaleEnv 清掉三个可能影响系统区域判断的变量，再写本用例需要的值。
// 不清理时，开发机自己的 LANG 会让「默认中文」这类用例时红时绿。
func setLocaleEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		t.Setenv(name, "")
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
}

// TestEnglishPluralOneForms 钉住单数变体：英文界面里「1 messages」「1 members」
// 是 i18n 改造漏掉的一类。约定 <key> 放复数、<key>.one 放单数；中文没有单复数，
// 两份目录里的值必须一致（否则调用处还得按语言分支）。
func TestEnglishPluralOneForms(t *testing.T) {
	// 界面上这些 key 都带 {n} + 可数名词，必须有单数变体。
	for _, key := range []string{
		"ui.conversation.members",
		"ui.conversation.messages",
		"ui.messages.copied",
		"ui.settings.workspaceCount",
		"ui.sidebar.workspaceCountTitle",
		"ui.preset.missingMembers",
		"ui.preset.unrunnableMembers",
		"ui.composer.skillsTitle",
	} {
		one, ok := en[key+".one"]
		if !ok {
			t.Errorf("英文目录缺少单数变体 %q", key+".one")
			continue
		}
		if _, ok := zh[key+".one"]; !ok {
			t.Errorf("中文目录缺少 %q（两份目录的 key 必须一致）", key+".one")
		}
		if one == en[key] {
			t.Errorf("英文 %q 的单数与复数写法相同（%q），单数变体没有起作用", key, one)
		}
	}

	// 反向：每个 .one 都必须有本体，中文两端同值。
	for key, one := range en {
		if !strings.HasSuffix(key, ".one") {
			continue
		}
		base := strings.TrimSuffix(key, ".one")
		if _, ok := en[base]; !ok {
			t.Errorf("英文目录里 %q 没有对应的复数条目 %q", key, base)
			continue
		}
		if zh[key] != zh[base] {
			t.Errorf("中文没有单复数，%q 与 %q 必须同值：%q vs %q", key, base, zh[key], zh[base])
		}
		if one == en[base] {
			t.Errorf("英文 %q 与本体 %q 完全相同，等于没加变体", key, base)
		}
	}
}

// TestExportHeaderBullets 钉住导出 Markdown 文件头的那组列表项：它们都由
// buildMarkdown 逐行推进同一个列表，任何一条少了 "- " 前缀都会把列表截成
// 两段（英文的 ui.export.scope 曾经就是如此）。
func TestExportHeaderBullets(t *testing.T) {
	for _, key := range []string{
		"ui.export.conversationLabel",
		"ui.export.directoryLabel",
		"ui.export.membersLabel",
		"ui.export.exportedAtLabel",
		"ui.export.scope",
	} {
		for _, lang := range []Lang{LangZh, LangEn} {
			if got := T(lang, key); !strings.HasPrefix(got, "- ") {
				t.Errorf("T(%q, %q) = %q，应以 \"- \" 开头（导出文件头的列表项）", lang, key, got)
			}
		}
	}
}
