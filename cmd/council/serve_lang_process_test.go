package main

// serve_lang_process_test.go 钉住「设置页保存语言后 /api/i18n 立刻换」。
//
// 为什么必须起真进程：根因在 runServe -> applyLanguageFlag —— 它过去把
// `-lang auto` 的解析结果也写回 COUNCIL_LANG，而环境变量的优先级高于
// 配置。internal/serve 里直接调 SetLanguage 的用例（以及它自己 t.Setenv
// 清掉环境变量之后的行为）碰不到那一步，所以只测那一层永远抓不到这个
// 回归。这里让子进程走完整的 run(["serve", "-rw", ...])，父进程按
// stdout 的 `council serve: <url>` 找到真实监听地址，再发 HTTP。
//
// 子进程就是测试二进制自己：用 -test.run 只选中 TestServeLangProcessChild，
// 环境变量 COUNCIL_TEST_SERVE_LANG_CHILD=1 表示「不要跑测试，去跑 run」。

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	serveLangChildGate     = "COUNCIL_TEST_SERVE_LANG_CHILD"
	serveLangChildAddr     = "COUNCIL_TEST_SERVE_LANG_ADDR"
	serveLangChildConfig   = "COUNCIL_TEST_SERVE_LANG_CONFIG"
	serveLangChildExplicit = "COUNCIL_TEST_SERVE_LANG_EXPLICIT"
)

// serveLangURLRe 匹配 serve 启动行。与 internal/serve 的 listenAndServe 一致。
var serveLangURLRe = regexp.MustCompile(`^council serve:\s+(https?://\S+)\s*$`)

// TestServeLangProcessChild 是子进程入口，不是正常用例。
func TestServeLangProcessChild(t *testing.T) {
	if os.Getenv(serveLangChildGate) != "1" {
		t.Skip("子进程专用：由 serve 语言热切换用例用 -test.run 拉起")
	}
	args := []string{
		"serve", "-rw",
		"-addr", os.Getenv(serveLangChildAddr),
		"-config", os.Getenv(serveLangChildConfig),
	}
	if explicit := os.Getenv(serveLangChildExplicit); explicit != "" {
		args = append(args, "-lang", explicit)
	}
	// run 会阻塞到进程被父用例结束；这里不返回。
	os.Exit(run(args))
}

// TestServeLanguageSavedSwitchesImmediately 覆盖未传 -lang 的情形：
// 配置初始 zh，POST 保存 en 之后 /api/i18n 必须立刻是 en，设置页的
// language_effective 跟配置走，且 language_flag / language_env 都不该出现。
//
// 修复前：runServe 的 applyLanguageFlag 先把 zh 写进 COUNCIL_LANG，
// 保存后 SetLanguage 重新解析时读到这个环境变量，/api/i18n 仍是 zh，
// 只能重启（重启后 applyLanguageFlag 又按新配置写回）才换。
func TestServeLanguageSavedSwitchesImmediately(t *testing.T) {
	if testing.Short() {
		t.Skip("需要起一个真实的 council serve -rw 子进程")
	}
	cfgPath := serveLangTestConfig(t, "zh")
	baseURL := startServeLangChild(t, cfgPath, "")

	var initial struct {
		Lang string `json:"lang"`
	}
	serveLangGetJSON(t, baseURL+"/api/i18n", &initial)
	if initial.Lang != "zh" {
		t.Fatalf("初始 /api/i18n lang = %q，期望 zh", initial.Lang)
	}

	saved := serveLangPost(t, baseURL+"/api/settings/language", `{"language":"en"}`)
	if !saved.Reloaded {
		t.Errorf("保存语言后 reloaded = false，期望 true（不需要重启）")
	}
	if strings.Contains(saved.Note, "启动参数") || strings.Contains(saved.Note, "COUNCIL_LANG") {
		t.Errorf("无 -lang / 无 COUNCIL_LANG 时保存提示不该说「优先」: %q", saved.Note)
	}

	var switched struct {
		Lang     string            `json:"lang"`
		Messages map[string]string `json:"messages"`
	}
	serveLangGetJSON(t, baseURL+"/api/i18n", &switched)
	if switched.Lang != "en" {
		t.Errorf("保存后 /api/i18n lang = %q，期望 en（不重启立即生效）", switched.Lang)
	}
	if got := switched.Messages["ui.composer.run"]; got != "Run a round" {
		t.Errorf("保存后 ui.composer.run = %q，期望 Run a round", got)
	}

	var settings struct {
		Language          string `json:"language"`
		LanguageEffective string `json:"language_effective"`
		LanguageFlag      string `json:"language_flag"`
		LanguageEnv       string `json:"language_env"`
	}
	serveLangGetJSON(t, baseURL+"/api/settings", &settings)
	if settings.Language != "en" || settings.LanguageEffective != "en" {
		t.Errorf("保存后 language/effective = %q/%q，期望 en/en",
			settings.Language, settings.LanguageEffective)
	}
	if settings.LanguageFlag != "" {
		t.Errorf("没有 -lang 时 language_flag = %q，期望空", settings.LanguageFlag)
	}
	if settings.LanguageEnv != "" {
		t.Errorf("没有 COUNCIL_LANG 时 language_env = %q，期望空", settings.LanguageEnv)
	}
}

// TestServeLanguageExplicitFlagStillWins 保留既有行为：显式 -lang en 启动
// 时，保存成中文仍提示「启动参数优先」，/api/i18n 也仍然是 en。
func TestServeLanguageExplicitFlagStillWins(t *testing.T) {
	if testing.Short() {
		t.Skip("需要起一个真实的 council serve -rw 子进程")
	}
	cfgPath := serveLangTestConfig(t, "zh")
	baseURL := startServeLangChild(t, cfgPath, "en")

	saved := serveLangPost(t, baseURL+"/api/settings/language", `{"language":"zh"}`)
	if !strings.Contains(saved.Note, "-lang en") || !strings.Contains(saved.Note, "仍优先") {
		t.Errorf("显式 -lang en 下保存中文的提示没有说明启动参数优先: %q", saved.Note)
	}

	var switched struct {
		Lang string `json:"lang"`
	}
	serveLangGetJSON(t, baseURL+"/api/i18n", &switched)
	if switched.Lang != "en" {
		t.Errorf("显式 -lang en 下 /api/i18n lang = %q，期望仍是 en", switched.Lang)
	}
}

// serveLangTestConfig 落一份最小可启动的 -rw 配置：初始 language 由参数给，
// provider / agent 只用于通过配置校验，本用例不会真的调用任何 CLI。
func serveLangTestConfig(t *testing.T, language string) string {
	t.Helper()
	base := t.TempDir()
	cfgPath := filepath.Join(base, "config.toml")
	body := fmt.Sprintf(`# 语言热切换测试：初始 %s，保存后应立刻换。
data_dir = '%s'
language = '%s'

[providers.claude]
command = '%s'

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
role = "语言热切换测试成员"
permissions = "read"
mode = "discussion"
`, language, filepath.Join(base, "data"), language, os.Args[0])
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	return cfgPath
}

// startServeLangChild 用测试二进制自己起一个真实的 serve -rw，返回监听地址。
// explicit 非空时追加 -lang <explicit>。
func startServeLangChild(t *testing.T, cfgPath string, explicit string) string {
	t.Helper()

	base := filepath.Dir(cfgPath)
	projectDir := filepath.Join(base, "project")
	homeDir := filepath.Join(base, "home")
	for _, dir := range []string{projectDir, homeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("建 %s 失败: %v", dir, err)
		}
	}
	stderrPath := filepath.Join(base, "serve-child.stderr.log")
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("建子进程 stderr 文件失败: %v", err)
	}

	exe, err := os.Executable()
	if err != nil {
		_ = stderrFile.Close()
		t.Fatalf("取测试二进制路径失败: %v", err)
	}
	cmd := exec.Command(exe, "-test.run", "^TestServeLangProcessChild$")
	cmd.Dir = projectDir
	cmd.Stderr = stderrFile
	cmd.Env = serveLangChildEnv(t, base, cfgPath, explicit)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stderrFile.Close()
		t.Fatalf("接子进程 stdout 失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stderrFile.Close()
		t.Fatalf("启动 serve 子进程失败: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		_ = stderrFile.Close()
	})

	urlCh := make(chan string, 1)
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if match := serveLangURLRe.FindStringSubmatch(scanner.Text()); match != nil {
				select {
				case urlCh <- match[1]:
				default:
				}
			}
		}
	}()

	select {
	case url := <-urlCh:
		return url
	case <-scanDone:
		t.Fatalf("serve 子进程在报告监听地址前退出；stderr:\n%s", readFileOrHint(stderrPath))
	case <-time.After(30 * time.Second):
		t.Fatalf("等了 30 秒，serve 子进程没有报告监听地址；stderr:\n%s", readFileOrHint(stderrPath))
	}
	return ""
}

// serveLangChildEnv 隔离出子进程环境：清掉 COUNCIL_LANG / COUNCIL_CONFIG /
// COUNCIL_DATA_DIR 与开发机 HOME，只留下显式给的测试参数。
// 「未传 -lang 时进程里没有 COUNCIL_LANG」正是本用例要复现的起点。
func serveLangChildEnv(t *testing.T, base, cfgPath, explicit string) []string {
	t.Helper()
	drop := map[string]bool{
		"COUNCIL_LANG":         true,
		"COUNCIL_CONFIG":       true,
		"COUNCIL_DATA_DIR":     true,
		"COUNCIL_GLOBAL_RULES": true,
		"COUNCIL_FAKE_CLI":     true,
		"HOME":                 true,
		"USERPROFILE":          true,
		serveLangChildGate:     true,
		serveLangChildAddr:     true,
		serveLangChildConfig:   true,
		serveLangChildExplicit: true,
	}
	env := make([]string, 0, len(os.Environ())+8)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || drop[strings.ToUpper(key)] {
			continue
		}
		env = append(env, entry)
	}
	// HOME / USERPROFILE 指到临时目录：配置虽然由 -config 独占，规则文件与
	// 可能存在的 .claude 都不该读开发机的真实主目录。
	env = append(env,
		serveLangChildGate+"=1",
		serveLangChildAddr+"=127.0.0.1:0",
		serveLangChildConfig+"="+cfgPath,
		serveLangChildExplicit+"="+explicit,
		"COUNCIL_GLOBAL_RULES="+filepath.Join(base, "no-global-rules.md"),
		"HOME="+filepath.Join(base, "home"),
		"USERPROFILE="+filepath.Join(base, "home"),
	)
	return env
}

// serveLangGetJSON 轮询 GET 直到 200：启动行先于 http.Serve 的 goroutine，
// 第一次连接可能刚好落在监听与 Serve 之间。
func serveLangGetJSON(t *testing.T, url string, out any) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastReason string
	for time.Now().Before(deadline) {
		res, err := http.Get(url)
		if err != nil {
			lastReason = err.Error()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		raw, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil {
			lastReason = readErr.Error()
		} else if res.StatusCode != http.StatusOK {
			lastReason = fmt.Sprintf("HTTP %d: %s", res.StatusCode, raw)
		} else if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("GET %s 返回的不是合法 JSON: %v；%s", url, err, raw)
		} else {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("GET %s 失败: %s", url, lastReason)
}

type serveLangWriteResponse struct {
	Reloaded bool   `json:"reloaded"`
	Note     string `json:"note"`
}

// serveLangPost 发一次设置页保存语言请求。不带 Origin：脚本 / 本机调用
// 与 curl 同类（checkWriteRequest 只对浏览器跨站头设闸）。
func serveLangPost(t *testing.T, url, body string) serveLangWriteResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造 POST %s 失败: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s 失败: %v", url, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读 POST %s 响应失败: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST %s = %d，期望 200；body=%s", url, res.StatusCode, raw)
	}
	var out serveLangWriteResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("保存响应不是合法 JSON: %v；%s", err, raw)
	}
	return out
}

func readFileOrHint(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("（读 %s 失败: %v）", path, err)
	}
	return string(raw)
}
