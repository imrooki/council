package serve

// i18n_test.go 覆盖 Web 侧的语言切面：
//
//  1. GET /api/i18n 在只读模式下也返回当前语言的完整界面文案表；
//  2. 两种语言各返回一份，lang 字段与文案都跟着 SetLanguage 走；
//  3. 设置页的 POST /api/settings/language 只改 layers[0] 的顶层 language，
//     保存后本次进程的 /api/i18n 立即切换；
//  4. 非法语言回 400 且文件一个字节不动；只读模式回 405。

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/i18n"
	"council/internal/store"
)

// newI18nReadOnlyServer 造一个只读看板（没有 -rw 的 WriteDeps）。
func newI18nReadOnlyServer(t *testing.T, langFlag, configLanguage string) *Server {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("建测试库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试库失败: %v", err)
		}
	})
	srv := NewServer(st, "test-version", nil)
	srv.SetLanguage(langFlag, configLanguage)
	return srv
}

// TestAPII18nReturnsBothLanguages 覆盖两种语言各一份、lang 字段正确。
func TestAPII18nReturnsBothLanguages(t *testing.T) {
	t.Setenv(i18n.EnvLang, "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	tests := []struct {
		name       string
		flag       string
		config     string
		wantLang   string
		wantRun    string
		wantArchvd string
	}{
		{
			name: "英文", flag: "en", config: "auto",
			wantLang: "en", wantRun: "Run a round", wantArchvd: "Archived",
		},
		{
			name: "中文", flag: "zh", config: "en",
			wantLang: "zh", wantRun: "跑一轮", wantArchvd: "已归档",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newI18nReadOnlyServer(t, tc.flag, tc.config)
			ts := newTestServer(t, srv)

			var resp i18nResponse
			getJSON(t, ts.URL+"/api/i18n", 200, &resp)
			if resp.Lang != tc.wantLang {
				t.Errorf("lang = %q，期望 %q", resp.Lang, tc.wantLang)
			}
			if got := resp.Messages["ui.composer.run"]; got != tc.wantRun {
				t.Errorf("ui.composer.run = %q，期望 %q", got, tc.wantRun)
			}
			if got := resp.Messages["ui.sidebar.archived"]; got != tc.wantArchvd {
				t.Errorf("ui.sidebar.archived = %q，期望 %q", got, tc.wantArchvd)
			}
			if len(resp.Messages) != len(i18n.UIMessages(i18n.Lang(tc.wantLang))) {
				t.Errorf("消息表条目数 = %d，期望完整界面表", len(resp.Messages))
			}
			for key := range resp.Messages {
				if !strings.HasPrefix(key, "ui.") {
					t.Errorf("消息表里出现了非界面 key %q", key)
				}
			}
		})
	}
}

// TestServeParseLangFlag 覆盖 `council serve -lang` 的解析：默认 auto，
// 显式值原样交给 i18n.Resolve（不在 Parse 阶段定成具体语言）。
func TestServeParseLangFlag(t *testing.T) {
	opts, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if opts.Lang != "auto" {
		t.Errorf("默认 -lang = %q，期望 auto", opts.Lang)
	}
	opts, err = Parse([]string{"-lang", "en"})
	if err != nil {
		t.Fatalf("Parse(-lang en): %v", err)
	}
	if opts.Lang != "en" {
		t.Errorf("-lang en 解析成 %q", opts.Lang)
	}
}

// TestAPII18nIsReadOnlyGET 钉住方法与模式：只读看板必须有这个端点，
// 非 GET 回 JSON 405（与其它只读端点同一套形状）。
func TestAPII18nIsReadOnlyGET(t *testing.T) {
	srv := newI18nReadOnlyServer(t, "", "")
	ts := newTestServer(t, srv)

	status, _, body := postRaw(t, ts.URL+"/api/i18n", "application/json", "{}")
	if status != 405 {
		t.Fatalf("POST /api/i18n = %d，期望 405；body=%s", status, body)
	}
	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err != nil || errResp.Error == "" {
		t.Errorf("405 响应不是 {\"error\":...}: %s", body)
	}
}

// TestSettingsLanguageSaveSwitchesImmediately 覆盖设置页写盘 + 热切换：
// 只动项目层、写后 /api/i18n 与本进程语言都换成 en，全局层字节不变。
func TestSettingsLanguageSaveSwitchesImmediately(t *testing.T) {
	t.Setenv(i18n.EnvLang, "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	globalBefore := gui6Read(t, env.globalConfig)

	settings := gui6Settings(t, env)
	if settings.Language != config.LanguageAuto {
		t.Errorf("初始 language = %q，期望 auto", settings.Language)
	}
	if settings.LanguageEffective != "zh" {
		t.Errorf("初始实际语言 = %q，期望 zh", settings.LanguageEffective)
	}

	status, raw := gui6Post(t, env.http.URL+"/api/settings/language", `{"language":"en"}`)
	if status != 200 {
		t.Fatalf("保存语言 = %d，期望 200；body=%s", status, raw)
	}
	var res settingsWriteResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("响应不是合法 JSON: %v；%s", err, raw)
	}
	if !res.Reloaded {
		t.Errorf("保存语言后 reloaded = false，期望 true（不需要重启）")
	}
	if res.BackupPath == "" {
		t.Errorf("保存语言没有返回备份路径: %+v", res)
	}

	after := gui6Read(t, env.projectConfig)
	if !strings.Contains(after, `language = "en"`) {
		t.Errorf("项目层没有写入 language = \"en\":\n%s", after)
	}
	// 只多出 language 这一行（以及它后面的空行），其余内容逐字节保留。
	if got := strings.Replace(after, "language = \"en\"\n\n", "", 1); got != before {
		t.Errorf("项目层的其它内容被改动了\n写前:\n%s\n写后:\n%s", before, after)
	}
	if got := gui6Read(t, env.globalConfig); got != globalBefore {
		t.Errorf("全局层被改动了")
	}
	// language 必须写在第一个表头之前。
	firstTable := strings.Index(after, "[agents.")
	if firstTable < 0 {
		firstTable = len(after)
	}
	if langAt := strings.Index(after, `language = "en"`); langAt < 0 || langAt > firstTable {
		t.Errorf("language 没有写在表头之前:\n%s", after)
	}

	var i18nResp i18nResponse
	getJSON(t, env.http.URL+"/api/i18n", 200, &i18nResp)
	if i18nResp.Lang != "en" {
		t.Errorf("/api/i18n lang = %q，期望 en（保存后立即可用）", i18nResp.Lang)
	}
	if got := i18nResp.Messages["ui.composer.run"]; got != "Run a round" {
		t.Errorf("保存后 ui.composer.run = %q，期望 Run a round", got)
	}

	settings = gui6Settings(t, env)
	if settings.Language != config.LanguageEn || settings.LanguageEffective != "en" {
		t.Errorf("保存后 settings.language/effective = %q/%q，期望 en/en",
			settings.Language, settings.LanguageEffective)
	}
}

// TestSettingsLanguageInvalidLeavesFileUntouched 覆盖非法值：400，目标文件
// 不变，不留备份与临时文件，进程语言也不变。
func TestSettingsLanguageInvalidLeavesFileUntouched(t *testing.T) {
	t.Setenv(i18n.EnvLang, "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)

	status, raw := gui6Post(t, env.http.URL+"/api/settings/language", `{"language":"fr"}`)
	if status != 400 {
		t.Fatalf("非法语言 = %d，期望 400；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "language") {
		t.Errorf("400 文案里没有指出 language: %s", raw)
	}
	if got := gui6Read(t, env.projectConfig); got != before {
		t.Errorf("非法语言之后目标文件被改动了")
	}
	backups, _ := filepath.Glob(env.projectConfig + ".bak-*")
	if len(backups) != 0 {
		t.Errorf("非法语言不该生成备份: %v", backups)
	}
	temps, _ := filepath.Glob(filepath.Join(filepath.Dir(env.projectConfig), "*.tmp-*"))
	if len(temps) != 0 {
		t.Errorf("非法语言残留临时文件: %v", temps)
	}
	var i18nResp i18nResponse
	getJSON(t, env.http.URL+"/api/i18n", 200, &i18nResp)
	if i18nResp.Lang != "zh" {
		t.Errorf("失败后语言 = %q，期望仍为 zh", i18nResp.Lang)
	}
}

// TestSettingsLanguageReadOnlyRejected 钉住只读模式 405。
func TestSettingsLanguageReadOnlyRejected(t *testing.T) {
	srv := newI18nReadOnlyServer(t, "", "")
	ts := newTestServer(t, srv)

	status, _, body := postRaw(t, ts.URL+"/api/settings/language", "application/json", `{"language":"en"}`)
	if status != 405 {
		t.Fatalf("只读模式保存语言 = %d，期望 405；body=%s", status, body)
	}
	if !strings.Contains(string(body), "只读模式") {
		t.Errorf("405 说明里没有「只读模式」: %s", body)
	}
}

// newTestServer 是 i18n 用例的小工具：httptest.Server + 自动关闭。
func newTestServer(t *testing.T, srv *Server) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}
