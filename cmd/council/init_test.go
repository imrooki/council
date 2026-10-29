package main

// init_test.go 覆盖 `council init` 的两条路：只读检测（--json）与写配置（--apply）。
//
// 判别点：
//   - 检测这条路**什么都不写** —— 桌面向导第一次打开时不该在用户家里留东西；
//   - 写配置这条路**先验证再落盘**、**不覆盖已有文件**、坏输入一律退 2。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

// initStubDetect 是固定的四家，顺序与生产 detectOrder 一致
// （claude、grok、agy、codex）：前两家装了，后两家没有。
func initStubDetect() []config.DetectedProvider {
	return []config.DetectedProvider{
		{ID: "claude", Path: `C:\bin\claude.exe`},
		{ID: "grok", Path: `C:\bin\grok.exe`},
		{ID: "agy"},
		{ID: "codex", Path: `C:\bin\codex.exe`},
	}
}

// initTestEnv 换掉检测与两个执行器；用例结束自动还原。
// 执行器一律返回「识别失败」，用例因此不碰用户机器上的真 CLI。
func initTestEnv(t *testing.T) {
	t.Helper()
	cleanLangEnv(t)
	oldDetect, oldExec, oldLogin := initDetect, initExec, initLoginExec
	t.Cleanup(func() {
		initDetect, initExec, initLoginExec = oldDetect, oldExec, oldLogin
	})
	initDetect = initStubDetect
	initExec = func(_ context.Context, _ provider.DiscoveryCommand) ([]byte, error) {
		return nil, errors.New("stub discovery failure")
	}
	initLoginExec = func(_ context.Context, _ provider.DiscoveryCommand) (int, []byte, []byte, error) {
		return 0, []byte("Logged in using an API key"), nil, nil
	}
}

// initRun 跑一次命令，返回两路输出与退出码。
func initRun(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := runInitWith(args, strings.NewReader(stdin), &out, &errOut)
	return out.String(), errOut.String(), code
}

// TestInitJSONDetectsWithoutWriting 钉住「检测不落盘」与 JSON 形状。
func TestInitJSONDetectsWithoutWriting(t *testing.T) {
	initTestEnv(t)
	target := filepath.Join(t.TempDir(), "config.toml")

	out, errOut, code := initRun(t, "", "--config", target, "--json")
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 0；stderr=%s", code, errOut)
	}
	var report initReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("输出不是 JSON: %v；%s", err, out)
	}
	if report.ConfigExists {
		t.Error("干净目录里 config_exists 应为 false")
	}
	if report.ConfigPath != target {
		t.Errorf("config_path = %q，期望 %q", report.ConfigPath, target)
	}
	if len(report.Providers) != 4 {
		t.Fatalf("providers = %d，期望 4（固定顺序 claude/grok/agy/codex）", len(report.Providers))
	}
	if report.Providers[0].ID != "claude" || !report.Providers[0].Found {
		t.Errorf("第一家应是找到的 claude: %+v", report.Providers[0])
	}
	if report.Providers[2].Found {
		t.Errorf("agy 没装，found 应为 false: %+v", report.Providers[2])
	}
	if report.Providers[1].Error == "" {
		t.Error("识别失败时 error 应当写出来，而不是假装 0 个模型")
	}
	if report.Providers[3].Login != "api_key" {
		t.Errorf("codex 的登录状态 = %q，期望 api_key（登录状态命令认出 API key）", report.Providers[3].Login)
	}
	if report.Providers[0].Login == "" {
		t.Error("登录状态不该是空串：查不出也要明确写成 unknown")
	}
	if report.Providers[0].LoginText == "" {
		t.Error("login_text 不该为空：界面上要有一句话解释登录状态")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("检测不该创建配置文件：%v", err)
	}
}

// TestInitApplyWritesValidConfig 是正向：按勾选写出一份能被正常加载的配置。
func TestInitApplyWritesValidConfig(t *testing.T) {
	initTestEnv(t)
	target := filepath.Join(t.TempDir(), "config.toml")
	payload := `{"providers":[
		{"id":"claude","agents":[{"id":"reviewer","display_name":"Reviewer","model":"claude-opus-5-5"}]},
		{"id":"codex","agents":[{"id":"critic","model":"gpt-6.1-sol","permissions":"workspace"}]}
	]}`

	out, errOut, code := initRun(t, payload, "--config", target, "--apply", "-", "--json")
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 0；stderr=%s", code, errOut)
	}
	var report initApplyReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("输出不是 JSON: %v；%s", err, out)
	}
	if !report.OK || report.ConfigPath != target {
		t.Fatalf("报告 = %+v", report)
	}
	if strings.Join(report.Providers, ",") != "claude,codex" {
		t.Errorf("providers = %v", report.Providers)
	}
	if strings.Join(report.Agents, ",") != "reviewer,critic" {
		t.Errorf("agents = %v", report.Agents)
	}

	cfg, err := config.LoadAndValidate(target)
	if err != nil {
		t.Fatalf("生成的配置没能通过加载与校验: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("agents = %d，期望 2", len(cfg.Agents))
	}
	critic := cfg.Agents["critic"]
	if critic.Provider != "codex" || critic.Model != "gpt-6.1-sol" {
		t.Errorf("critic = %+v", critic)
	}
	if critic.DisplayName != "critic" {
		t.Errorf("没给 display_name 时应当退回 id，实际 %q", critic.DisplayName)
	}
	if critic.Permissions != config.PermissionWorkspace {
		t.Errorf("permissions = %q，期望 workspace", critic.Permissions)
	}
	if cfg.Agents["reviewer"].Permissions != config.PermissionRead {
		t.Errorf("默认权限应当是 read，实际 %q", cfg.Agents["reviewer"].Permissions)
	}
	if cfg.DataDir != filepath.Join(filepath.Dir(target), "data") {
		t.Errorf("data_dir = %q，期望配置文件旁边的 data", cfg.DataDir)
	}
}

// TestInitApplyRefusesExisting 钉住「不覆盖」：已有配置一个字节都不动。
func TestInitApplyRefusesExisting(t *testing.T) {
	initTestEnv(t)
	target := filepath.Join(t.TempDir(), "config.toml")
	original := "# 我自己写的\n[providers.claude]\ncommand = 'claude'\n"
	if err := os.WriteFile(target, []byte(original), 0o600); err != nil {
		t.Fatalf("准备既有配置失败: %v", err)
	}

	payload := `{"providers":[{"id":"claude","agents":[{"id":"a1","model":"m"}]}]}`
	_, errOut, code := initRun(t, payload, "--config", target, "--apply", "-")
	if code != exitConfigFailed {
		t.Fatalf("退出码 = %d，期望 %d", code, exitConfigFailed)
	}
	if !strings.Contains(errOut, "已存在") {
		t.Errorf("stderr 没说清为什么不写:\n%s", errOut)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读回配置失败: %v", err)
	}
	if string(body) != original {
		t.Errorf("既有配置被改动了:\n%s", body)
	}
}

// TestInitApplyRejectsBadInput 覆盖用法错误：一律退 2，且不留下文件。
func TestInitApplyRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"不是 JSON", `{`, "JSON"},
		{"没选 provider", `{"providers":[]}`, "至少要选一个"},
		{"provider 没装", `{"providers":[{"id":"agy","agents":[{"id":"a1","model":"m"}]}]}`, "没有检测到"},
		{"成员 id 重复", `{"providers":[{"id":"claude","agents":[{"id":"a1","model":"m"},{"id":"a1","model":"m2"}]}]}`, "重复"},
		{"成员没给 model", `{"providers":[{"id":"claude","agents":[{"id":"a1"}]}]}`, "模型"},
		{"id 不合法", `{"providers":[{"id":"claude","agents":[{"id":"A B","model":"m"}]}]}`, "id"},
		{"权限写错", `{"providers":[{"id":"claude","agents":[{"id":"a1","model":"m","permissions":"write"}]}]}`, "permissions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initTestEnv(t)
			target := filepath.Join(t.TempDir(), "config.toml")
			_, errOut, code := initRun(t, tc.payload, "--config", target, "--apply", "-")
			if code != exitUsage {
				t.Fatalf("退出码 = %d，期望 2；stderr=%s", code, errOut)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr 里没有 %q:\n%s", tc.want, errOut)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Errorf("被拒的请求不该留下配置文件: %v", err)
			}
		})
	}
}

// TestInitApplyValidatesBeforeWriting 钉住「先验证再落盘」：一个能通过入参
// 检查、却过不了 config.Validate 的成员 id（user 是保留名）必须让整次写入失败，
// 而且文件不存在。
func TestInitApplyValidatesBeforeWriting(t *testing.T) {
	initTestEnv(t)
	target := filepath.Join(t.TempDir(), "config.toml")
	payload := `{"providers":[{"id":"claude","agents":[{"id":"user","model":"m"}]}]}`

	_, errOut, code := initRun(t, payload, "--config", target, "--apply", "-")
	if code != exitConfigFailed {
		t.Fatalf("退出码 = %d，期望 %d；stderr=%s", code, exitConfigFailed, errOut)
	}
	if !strings.Contains(errOut, "校验") {
		t.Errorf("stderr 没说是校验没过:\n%s", errOut)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("校验失败不该留下配置文件: %v", err)
	}
}

// TestInitPositionalRejected 钉住「不接受位置参数」（拼错的子命令不该被静默忽略）。
func TestInitPositionalRejected(t *testing.T) {
	initTestEnv(t)
	_, errOut, code := initRun(t, "", "setup")
	if code != exitUsage {
		t.Fatalf("退出码 = %d，期望 2", code)
	}
	if !strings.Contains(errOut, "位置参数") {
		t.Errorf("stderr 没说清:\n%s", errOut)
	}
}
