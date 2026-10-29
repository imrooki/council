package serve

// gui6b_providers_test.go：提供方在设置页里可增删改（0.4.0）。
//
// 这一片最容易出事的不是「写没写进去」，而是「写的时候有没有顺手动别的东西」：
// 配置里还有 agents、teams、summary、注释与行尾，一次 provider 保存必须只
// 改 [providers.<id>] 这一段。下面每条用例都同时断言「预期的改动生效」与
// 「不该动的地方一个字节没变」。

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"council/internal/config"
)

// providerByID 在设置响应里找一个 provider。
func providerByID(t *testing.T, providers []settingsProviderJSON, id string) settingsProviderJSON {
	t.Helper()
	for _, p := range providers {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("设置响应里没有 provider %q: %+v", id, providers)
	return settingsProviderJSON{}
}

// providerSave 保存一个 provider 并返回状态码与响应体。
func providerSave(t *testing.T, env *gui6Env, payload map[string]any) (int, settingsWriteResponse, string) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("编码请求失败: %v", err)
	}
	status, raw := gui6Post(t, env.http.URL+"/api/settings/providers", string(body))
	var res settingsWriteResponse
	if status == http.StatusOK {
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatalf("保存响应不是合法 JSON: %v；%s", err, raw)
		}
	}
	return status, res, string(raw)
}

func TestGui6bAddProviderWritesOnlyTargetFile(t *testing.T) {
	env := newGui6Env(t)
	globalBefore := gui6Read(t, env.globalConfig)
	projectBefore := gui6Read(t, env.projectConfig)

	code, res, raw := providerSave(t, env, map[string]any{
		"provider_id": "deepseek",
		"command":     `C:\Tools\deepseek\deepseek.cmd`,
	})
	if code != http.StatusOK {
		t.Fatalf("新增 provider = %d，期望 200；body=%s", code, raw)
	}
	if res.WriteTarget != env.projectConfig {
		t.Errorf("write_target = %q，期望项目层 %q", res.WriteTarget, env.projectConfig)
	}
	if res.BackupPath == "" {
		t.Errorf("没有生成备份")
	}
	if res.Reloaded {
		t.Errorf("reloaded = true，期望 false：provider 改动要重启 serve 才生效")
	}
	if !strings.Contains(res.Note, "重启") {
		t.Errorf("note = %q，期望说明需要重启", res.Note)
	}
	if got := gui6Read(t, env.globalConfig); got != globalBefore {
		t.Errorf("全局层被改动了:\n%s", got)
	}

	projectAfter := gui6Read(t, env.projectConfig)
	if !strings.Contains(projectAfter, "[providers.deepseek]") {
		t.Fatalf("项目层里没有新增的 provider 段:\n%s", projectAfter)
	}
	if !strings.Contains(projectAfter, `command = 'C:\Tools\deepseek\deepseek.cmd'`) {
		t.Errorf("command 没有按单引号字面量写入:\n%s", projectAfter)
	}
	// 原有内容必须原样保留。
	for _, want := range []string{
		"# 项目层：只写这个项目自己的成员",
		"[agents.p1]",
		`display_name = "Project One"`,
		"[agents.p2]",
		`role = "待删除成员"`,
	} {
		if !strings.Contains(projectAfter, want) {
			t.Errorf("项目层里丢了 %q:\n%s", want, projectAfter)
		}
	}
	if strings.Count(projectAfter, "[agents.p1]") != 1 {
		t.Errorf("p1 段被复制了:\n%s", projectAfter)
	}

	// 设置页马上能看到它，并且标成「可写」。
	settings := gui6Settings(t, env)
	p := providerByID(t, settings.Providers, "deepseek")
	if !p.Writable {
		t.Errorf("新增的 provider 没有被标成 writable")
	}
	if p.Command != `C:\Tools\deepseek\deepseek.cmd` {
		t.Errorf("command = %q", p.Command)
	}
	if p.Exists {
		t.Errorf("exists = true，期望 false：这个路径在测试机上不存在")
	}
	if len(p.Agents) != 0 {
		t.Errorf("agents = %v，期望空", p.Agents)
	}

	// 全局层的 claude 仍在，且不是可写的。
	inherited := providerByID(t, settings.Providers, "claude")
	if inherited.Writable {
		t.Errorf("全局层的 provider 被标成了 writable")
	}
	if len(inherited.Agents) == 0 {
		t.Errorf("claude 被 p1/p2/g1 引用，agents 不该是空的")
	}
	_ = projectBefore
}

func TestGui6bUpdateProviderKeepsComment(t *testing.T) {
	env := newGui6Env(t)
	// 先在项目层里放一段带行尾注释的 provider。
	project := gui6Read(t, env.projectConfig)
	project += "\n[providers.local]\ncommand = 'old-binary'  # 我这台机器上的路径\n"
	if err := writeFileForTest(env.projectConfig, project); err != nil {
		t.Fatalf("写项目配置失败: %v", err)
	}
	reloadForTest(t, env)

	code, res, raw := providerSave(t, env, map[string]any{
		"provider_id": "local",
		"command":     "new-binary",
	})
	if code != http.StatusOK {
		t.Fatalf("修改 provider = %d，期望 200；body=%s", code, raw)
	}
	if res.BackupPath == "" {
		t.Errorf("没有生成备份")
	}
	after := gui6Read(t, env.projectConfig)
	if !strings.Contains(after, "command = 'new-binary'  # 我这台机器上的路径") {
		t.Errorf("值改了但行尾注释被丢了:\n%s", after)
	}
	if strings.Count(after, "[providers.local]") != 1 {
		t.Errorf("local 段出现了多次:\n%s", after)
	}
}

func TestGui6bDeleteProviderRefusesWhenReferenced(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.globalConfig)

	status, raw := gui6Post(t, env.http.URL+"/api/settings/providers/claude/delete", "{}")
	if status != http.StatusConflict {
		t.Fatalf("删除被引用的 provider = %d，期望 409；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "p1") || !strings.Contains(string(raw), "g1") {
		t.Errorf("错误消息没有点名引用它的成员: %s", raw)
	}
	if got := gui6Read(t, env.globalConfig); got != before {
		t.Errorf("被拒绝的删除改动了配置:\n%s", got)
	}
	// 它定义在全局层，本来也不该被项目层删掉——这条用例证明拒绝发生在写盘之前。
	if !strings.Contains(string(raw), "不能删除") {
		t.Errorf("错误消息不像人话: %s", raw)
	}
}

func TestGui6bDeleteProviderInheritedIsConflict(t *testing.T) {
	env := newGui6Env(t)
	// 全局层加一个没人引用的 provider：项目层删不掉它，因为它不在写目标里。
	global := gui6Read(t, env.globalConfig)
	global += "\n[providers.lonely]\ncommand = 'nobody-uses-me'\n"
	if err := writeFileForTest(env.globalConfig, global); err != nil {
		t.Fatalf("写全局配置失败: %v", err)
	}
	reloadForTest(t, env)

	status, raw := gui6Post(t, env.http.URL+"/api/settings/providers/lonely/delete", "{}")
	if status != http.StatusConflict {
		t.Fatalf("删除继承来的 provider = %d，期望 409；body=%s", status, raw)
	}
	// 消息要点名「它定义在哪份文件、本接口只写哪份」，用户才知道去改哪里。
	// JSON 里反斜杠是转义的，直接搜原文会漏，先解出 error 字段再比。
	msg := errorMessage(t, raw)
	if !strings.Contains(msg, "定义在") || !strings.Contains(msg, env.globalConfig) {
		t.Errorf("错误消息没有说明它定义在别的层: %s", msg)
	}
}

func TestGui6bDeleteProviderRemovesSection(t *testing.T) {
	env := newGui6Env(t)
	if code, _, raw := providerSave(t, env, map[string]any{
		"provider_id": "temp",
		"command":     "temp-binary",
	}); code != http.StatusOK {
		t.Fatalf("新增失败: %d %s", code, raw)
	}
	status, raw := gui6Post(t, env.http.URL+"/api/settings/providers/temp/delete", "{}")
	if status != http.StatusOK {
		t.Fatalf("删除 provider = %d，期望 200；body=%s", status, raw)
	}
	after := gui6Read(t, env.projectConfig)
	if strings.Contains(after, "[providers.temp]") {
		t.Errorf("删除后段还在:\n%s", after)
	}
	if !strings.Contains(after, "[agents.p1]") {
		t.Errorf("删除 provider 时把别的段删掉了:\n%s", after)
	}
	for _, p := range gui6Settings(t, env).Providers {
		if p.ID == "temp" {
			t.Fatalf("设置响应里还能看到被删的 provider")
		}
	}
}

func TestGui6bProviderArgsAndEnv(t *testing.T) {
	env := newGui6Env(t)
	code, _, raw := providerSave(t, env, map[string]any{
		"provider_id": "wrapped",
		"command":     "wrapped-cli",
		"args":        []string{"--mode", "fast", "  "},
		"env":         []string{"WRAPPED_TOKEN=abc", "  ", "WRAPPED_HOME=C:\\tmp\\w"},
	})
	if code != http.StatusOK {
		t.Fatalf("保存带参数的 provider = %d，期望 200；body=%s", code, raw)
	}
	after := gui6Read(t, env.projectConfig)
	for _, want := range []string{
		"[providers.wrapped]",
		`command = 'wrapped-cli'`,
		`args = ['--mode', 'fast']`,
		`env = ['WRAPPED_TOKEN=abc', 'WRAPPED_HOME=C:\tmp\w']`,
	} {
		if !strings.Contains(after, want) {
			t.Errorf("配置里缺少 %q:\n%s", want, after)
		}
	}
	// 空行被丢掉，不该在文件里留一个空参数。
	if strings.Contains(after, "'  '") {
		t.Errorf("空白参数被写进了配置:\n%s", after)
	}

	// 读回来：三个字段都在。
	var got *settingsProviderJSON
	for i := range gui6Settings(t, env).Providers {
		p := gui6Settings(t, env).Providers[i]
		if p.ID == "wrapped" {
			got = &p
		}
	}
	if got == nil {
		t.Fatalf("设置响应里没有 wrapped")
	}
	if len(got.Args) != 2 || got.Args[0] != "--mode" {
		t.Errorf("args = %v", got.Args)
	}
	if len(got.Env) != 2 || got.Env[0] != "WRAPPED_TOKEN=abc" {
		t.Errorf("env = %v", got.Env)
	}

	// 清空 args / env：那两行要从文件里消失，而不是留下 args = []。
	code, _, raw = providerSave(t, env, map[string]any{
		"provider_id": "wrapped",
		"command":     "wrapped-cli",
		"args":        []string{},
		"env":         []string{},
	})
	if code != http.StatusOK {
		t.Fatalf("清空参数 = %d；body=%s", code, raw)
	}
	cleared := gui6Read(t, env.projectConfig)
	if strings.Contains(cleared, "args =") || strings.Contains(cleared, "env =") {
		t.Errorf("清空之后 args/env 行还留着:\n%s", cleared)
	}
	if !strings.Contains(cleared, "command = 'wrapped-cli'") {
		t.Errorf("清空参数时把 command 也弄丢了:\n%s", cleared)
	}
}

func TestGui6bProviderModels(t *testing.T) {
	env := newGui6Env(t)
	code, _, raw := providerSave(t, env, map[string]any{
		"provider_id": "cpa",
		"command":     "cpa-cli",
		"models":      []string{"cpa-large", "  ", "cpa-small", "cpa-large"},
	})
	if code != http.StatusOK {
		t.Fatalf("保存带模型的 provider = %d；body=%s", code, raw)
	}
	after := gui6Read(t, env.projectConfig)
	if !strings.Contains(after, `models = ['cpa-large', 'cpa-small']`) {
		t.Errorf("models 没写进配置（空白与重复应被去掉）:\n%s", after)
	}
	settings := gui6Settings(t, env)
	var got *settingsProviderJSON
	for i := range settings.Providers {
		if settings.Providers[i].ID == "cpa" {
			p := settings.Providers[i]
			got = &p
		}
	}
	if got == nil || len(got.Models) != 2 || got.Models[0] != "cpa-large" {
		t.Fatalf("设置响应里的 models = %+v", got)
	}

	// 清空之后这一行要从文件里消失。
	if code, _, raw := providerSave(t, env, map[string]any{
		"provider_id": "cpa",
		"command":     "cpa-cli",
		"models":      []string{},
	}); code != http.StatusOK {
		t.Fatalf("清空模型 = %d；body=%s", code, raw)
	}
	if strings.Contains(gui6Read(t, env.projectConfig), "models =") {
		t.Errorf("清空后 models 行还在")
	}
}
func TestGui6bProviderEnvValidation(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	for _, entry := range []string{"NOEQUALS", "1ABC=x", "A B=x", "=x"} {
		code, _, raw := providerSave(t, env, map[string]any{
			"provider_id": "bad-env",
			"command":     "x",
			"env":         []string{entry},
		})
		if code != http.StatusBadRequest {
			t.Errorf("env %q = %d，期望 400；body=%s", entry, code, raw)
			continue
		}
		if !strings.Contains(raw, "环境变量") {
			t.Errorf("env %q 的错误消息不友好: %s", entry, raw)
		}
	}
	if got := gui6Read(t, env.projectConfig); got != before {
		t.Errorf("校验失败却改了文件:\n%s", got)
	}
}

func TestGui6bProviderValidation(t *testing.T) {
	env := newGui6Env(t)
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"空 id", map[string]any{"provider_id": "  ", "command": "x"}, "不能为空"},
		{"非法 id", map[string]any{"provider_id": "Bad ID", "command": "x"}, "provider id"},
		{"空 command", map[string]any{"provider_id": "ok", "command": "   "}, "command 不能为空"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := gui6Read(t, env.projectConfig)
			code, _, raw := providerSave(t, env, tc.payload)
			if code != http.StatusBadRequest {
				t.Fatalf("= %d，期望 400；body=%s", code, raw)
			}
			if !strings.Contains(raw, tc.want) {
				t.Errorf("错误消息 = %s，期望包含 %q", raw, tc.want)
			}
			if got := gui6Read(t, env.projectConfig); got != before {
				t.Errorf("校验失败却改了文件:\n%s", got)
			}
		})
	}
}

func TestGui6bProviderDeletePathShape(t *testing.T) {
	env := newGui6Env(t)
	for _, path := range []string{
		"/api/settings/providers/x/delete/extra",
		"/api/settings/providers/x/remove",
		"/api/settings/providers/",
	} {
		status, raw := gui6Post(t, env.http.URL+path, "{}")
		if status != http.StatusNotFound {
			t.Errorf("%s = %d，期望 404；body=%s", path, status, raw)
		}
	}
}

// errorMessage 从 {"error":"..."} 里取出真正的消息。
func errorMessage(t *testing.T, raw []byte) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("错误响应不是合法 JSON: %v；%s", err, raw)
	}
	return body.Error
}

// writeFileForTest 直接改一份配置，用于构造「文件里已经有这一段」的前置状态。
// 改完必须 reloadForTest，否则展示配置还是旧的。
func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// reloadForTest 让服务重新合并一次配置层，模拟「下一次启动」看到的配置。
// 只换展示配置，不动 Runner —— provider 改动本来就要重启才生效。
func reloadForTest(t *testing.T, env *gui6Env) {
	t.Helper()
	merged, err := config.LoadMerged(env.layers)
	if err != nil {
		t.Fatalf("重新加载合并配置失败: %v", err)
	}
	env.srv.adoptReloadedConfig(merged.Config)
}
