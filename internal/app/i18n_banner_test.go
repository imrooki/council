package app

// i18n_banner_test.go 钉住启动横幅的中英两版：
//
//   - writeBanner（旧入口）与 writeBannerLang(zh) 必须逐字节相同 —— 默认
//     中文输出不变是硬约束；
//   - en 版不能出现汉字，且关键行是英文；
//   - 两版的行数相同（语言只换措辞，不增减启动信息的条目）。

import (
	"bytes"
	"strings"
	"testing"
	"unicode"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/i18n"
)

func i18nBanner(t *testing.T, lang i18n.Lang) string {
	t.Helper()
	var buf bytes.Buffer
	convs := []conversation.Conversation{
		{ID: "c1", Title: "标题", Cwd: ""},
		{ID: "c2", Title: "另一个", Cwd: `D:\proj`},
	}
	writeBannerLang(&buf, lang, "1.2.3", `C:\cfg\config.toml`, `C:\data`, `C:\data\council.db`,
		convs, `D:\proj`, &config.Config{})
	return buf.String()
}

func bannerHasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestBannerChineseUnchanged(t *testing.T) {
	var old bytes.Buffer
	writeBanner(&old, "1.2.3", `C:\cfg\config.toml`, `C:\data`, `C:\data\council.db`,
		[]conversation.Conversation{{ID: "c1"}, {ID: "c2", Cwd: `D:\proj`}}, `D:\proj`, &config.Config{})
	if got := i18nBanner(t, i18n.LangZh); got != old.String() {
		t.Fatalf("横幅中文输出变了\n新:\n%s\n旧:\n%s", got, old.String())
	}
}

func TestBannerEnglishHasNoChinese(t *testing.T) {
	out := i18nBanner(t, i18n.LangEn)
	if bannerHasHan(out) {
		t.Fatalf("英文横幅里出现汉字:\n%s", out)
	}
	for _, want := range []string{
		"Config file:",
		"Data dir:",
		"Database:",
		"Project:",
		"Type /help",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("英文横幅缺少 %q:\n%s", want, out)
		}
	}
	zhLines := strings.Count(i18nBanner(t, i18n.LangZh), "\n")
	enLines := strings.Count(out, "\n")
	if zhLines != enLines {
		t.Errorf("中英横幅行数不同：zh=%d en=%d", zhLines, enLines)
	}
}
