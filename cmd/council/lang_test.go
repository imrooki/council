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

// TestApplyLanguageFlagWritesEnvironment 钉住交互入口的落点：最终语言写回
// COUNCIL_LANG，后续片读到的是同一次 Resolve 结果。
func TestApplyLanguageFlagWritesEnvironment(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	t.Setenv(i18n.EnvLang, "")
	path := langConfigFile(t, "en")
	if got := applyLanguageFlag("auto", path); got != i18n.LangEn {
		t.Fatalf("applyLanguageFlag = %q，期望 en", got)
	}
	if got := os.Getenv(i18n.EnvLang); got != "en" {
		t.Errorf("COUNCIL_LANG = %q，期望 en", got)
	}
}
