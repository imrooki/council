package main

import (
	"os"
	"path/filepath"
	"testing"

	"council/internal/i18n"
)

// langConfigFile 写一份最小可加载配置，language 可控。
func langConfigFile(t *testing.T, language string) string {
	t.Helper()
	body := ""
	if language != "" {
		body += `language = "` + language + "\"\n"
	}
	body += `[providers.claude]
command = "claude"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	return path
}

// TestCLILanguagePriority 覆盖交互入口的四级优先级：flag > COUNCIL_LANG >
// 配置 > 系统区域。配置只在前面都没有明确值时才会被读。
func TestCLILanguagePriority(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv(i18n.EnvLang, "")

	enConfig := langConfigFile(t, "en")
	zhConfig := langConfigFile(t, "zh")

	tests := []struct {
		name   string
		flag   string
		env    string
		config string
		want   i18n.Lang
	}{
		{name: "flag 压过配置", flag: "zh", config: enConfig, want: i18n.LangZh},
		{name: "flag auto 时读配置", flag: "auto", config: enConfig, want: i18n.LangEn},
		{name: "flag 为空时环境变量压过配置", flag: "", env: "en", config: zhConfig, want: i18n.LangEn},
		{name: "环境变量为空时配置生效", flag: "auto", env: "", config: enConfig, want: i18n.LangEn},
		{name: "非法 flag 回落默认", flag: "fr", env: "en", config: enConfig, want: i18n.LangZh},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(i18n.EnvLang, tc.env)
			if got := cliLanguage(tc.flag, tc.config); got != tc.want {
				t.Errorf("cliLanguage(%q, config) = %q，期望 %q", tc.flag, got, tc.want)
			}
		})
	}
}

// TestCLILanguageFallsBackToSystemLocale 覆盖配置缺失时继续看系统区域。
func TestCLILanguageFallsBackToSystemLocale(t *testing.T) {
	t.Setenv(i18n.EnvLang, "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := cliLanguage("", filepath.Join(t.TempDir(), "missing.toml")); got != i18n.LangEn {
		t.Errorf("缺失配置时 cliLanguage = %q，期望 en（LANG=en_US.UTF-8）", got)
	}
}

// TestReadConfigLanguageIgnoresBrokenConfig 钉住「读配置失败不阻塞启动」：
// 坏配置返回空串，不 panic、不把错误变成一句与语言有关的启动失败。
func TestReadConfigLanguageIgnoresBrokenConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("this is not toml = [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readConfigLanguage(path); got != "" {
		t.Errorf("坏配置的 language = %q，期望空串", got)
	}
}

// unsetLangEnv 清掉 COUNCIL_LANG 并让它真的「不存在」，而不是空串。
// os.Getenv 分不清这两者，设置页的 LanguageEnv 用 Getenv 也分不清；
// 但「进程里没有这个变量」是本次修复要钉住的事实，所以用 LookupEnv 断言。
func unsetLangEnv(t *testing.T) {
	t.Helper()
	t.Setenv(i18n.EnvLang, "")
	if err := os.Unsetenv(i18n.EnvLang); err != nil {
		t.Fatalf("清空 %s 失败: %v", i18n.EnvLang, err)
	}
}

// TestApplyLanguageFlagLeavesEnvAloneWhenNotExplicit 覆盖 a / b：未传 -lang
// 与 -lang auto 都不写 COUNCIL_LANG，但返回值仍按配置解析（消息文案用）。
// 这是「设置页改语言必须重启」的根因：自己写进去的环境变量压住了配置。
func TestApplyLanguageFlagLeavesEnvAloneWhenNotExplicit(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	unsetLangEnv(t)

	// 配置说 en：没有显式 -lang 时，返回值应当跟随配置。
	path := langConfigFile(t, "en")
	tests := []struct {
		name string
		flag string
	}{
		{name: "未传 -lang", flag: ""},
		{name: "-lang auto", flag: "auto"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyLanguageFlag(tc.flag, path); got != i18n.LangEn {
				t.Errorf("applyLanguageFlag(%q) = %q，期望 en（跟随配置）", tc.flag, got)
			}
			if value, ok := os.LookupEnv(i18n.EnvLang); ok {
				t.Errorf("%s 被写成了 %q，期望不设置", i18n.EnvLang, value)
			}
		})
	}
}

// TestApplyLanguageFlagExplicitWritesEnvironment 覆盖 c：显式 -lang en 写进
// COUNCIL_LANG（子进程继承），且这个值压过配置里的 zh，真的生效。
func TestApplyLanguageFlagExplicitWritesEnvironment(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	unsetLangEnv(t)

	// 配置说 zh，显式 -lang en 必须赢。
	path := langConfigFile(t, "zh")
	if got := applyLanguageFlag("en", path); got != i18n.LangEn {
		t.Fatalf("applyLanguageFlag(\"en\") = %q，期望 en", got)
	}
	if value, ok := os.LookupEnv(i18n.EnvLang); !ok || value != "en" {
		t.Errorf("%s = %q（存在=%v），期望 en", i18n.EnvLang, value, ok)
	}
	// 模拟子进程 / 后续解析：没有 flag 时，环境变量让 en 继续生效。
	if got := i18n.Resolve("", ""); got != i18n.LangEn {
		t.Errorf("写回后 i18n.Resolve(\"\", \"\") = %q，期望 en", got)
	}
}
