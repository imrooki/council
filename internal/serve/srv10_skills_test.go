package serve

// srv10_skills_test.go 覆盖 SRV10 的两件事：
//  1. 导入路径上的 8 KiB 大小闸：.claude/agents 的正文超过
//     config.MaxAgentPromptBytes 时跳过该成员、留一条默认可见的提示，
//     同目录里正常大小的成员照常导入（§4.1）；
//  2. skills 只读导入：.claude/skills 扫到的元数据进 /api/options 的
//     skills 字段（{id,name,description,source}），不执行、不注入提示（§4.2）。
//
// 端到端那条从 Run 进去，覆盖 runWritable 的扫描与装配；只测 handler 的话，
// getJSON 只能证明结构体会编码，证明不了启动时真的扫了。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
)

// TestSrv10ClaudeImportPromptLimitBoundary 钉住大小闸的边界：> 8192 字节跳过
// 并给出与任务书一致的提示，恰好 8192 字节照常导入；跳过不影响同目录里别的
// 成员，也不写进 merged（它不该出现在 /api/options 的 Claude 段）。
func TestSrv10ClaudeImportPromptLimitBoundary(t *testing.T) {
	srv8IsolateClaudeHome(t)
	projectDir := t.TempDir()

	edgePrompt := strings.Repeat("y", config.MaxAgentPromptBytes)
	bigPrompt := strings.Repeat("x", config.MaxAgentPromptBytes+1)
	srv8WriteClaudeAgent(t, projectDir, "edge.md", "name: edge\ndescription: 恰好到上限\n", edgePrompt)
	srv8WriteClaudeAgent(t, projectDir, "big.md", "name: big\ndescription: 超过一字节\n", bigPrompt)
	srv8WriteClaudeAgent(t, projectDir, "ok.md", "name: ok\ndescription: 正常成员\n", "正常正文")

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{"claude": {Command: "x"}},
		Agents:    map[string]config.AgentConfig{},
	}
	got := importClaudeAgents(cfg, projectDir)

	edge, ok := got.agents["edge"]
	if !ok {
		t.Fatalf("恰好 %d 字节的 prompt 应当照常导入，实际 agents=%v", config.MaxAgentPromptBytes, got.agents)
	}
	if len(edge.Prompt) != config.MaxAgentPromptBytes {
		t.Errorf("恰好到上限的 prompt 被改动了：%d 字节", len(edge.Prompt))
	}
	if _, ok := got.agents["ok"]; !ok {
		t.Errorf("同一目录里正常大小的 ok 应当照常导入：%v", got.agents)
	}
	if _, ok := got.agents["big"]; ok {
		t.Errorf("超过上限的 big 不该出现在导入结果里")
	}
	if _, ok := cfg.Agents["big"]; ok {
		t.Errorf("超过上限的 big 不该被合并进 cfg.Agents")
	}
	if _, ok := got.merged["big"]; ok {
		t.Errorf("超过上限的 big 不该进 merged（/api/options 会把它当成已导入）")
	}

	want := fmt.Sprintf(`跳过 claude agent "big"：系统提示 %d 字节，超过 %d 上限`,
		len(bigPrompt), config.MaxAgentPromptBytes)
	if len(got.warnings) != 1 {
		t.Fatalf("应当恰好有 1 条跳过提示，实际 %v", got.warnings)
	}
	if got.warnings[0] != want {
		t.Errorf("跳过提示 = %q，期望 %q", got.warnings[0], want)
	}
}

// TestSrv10SkillsAndOversizeAgentReachOptions 从 Run 进去：启动目录里同时有
// 一个项目级 skill、一个正常 agent、一个超长 agent。断言
//   - stderr 默认打印大小闸的跳过提示（不藏在 -verbose 后面）；
//   - /api/options 的 skills 是真实 JSON 数组，字段与 source 都对；
//   - 正常 agent 进了 agents，超长的没有。
func TestSrv10SkillsAndOversizeAgentReachOptions(t *testing.T) {
	srv8IsolateClaudeHome(t)
	projectDir := t.TempDir()

	skillDir := filepath.Join(projectDir, ".claude", "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("建 skill 目录失败: %v", err)
	}
	skillBody := "---\nname: deploy-staging\ndescription: 部署到预发\n---\n\n按步骤部署。\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		t.Fatalf("写 SKILL.md 失败: %v", err)
	}

	srv8WriteClaudeAgent(t, projectDir, "ok.md", "name: ok\ndescription: 正常成员\n", "正常正文")
	bigPrompt := strings.Repeat("x", config.MaxAgentPromptBytes+1)
	srv8WriteClaudeAgent(t, projectDir, "big.md", "name: big\ndescription: 超大成员\n", bigPrompt)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := filepath.Join(dir, "config.toml")
	cfgBody := fmt.Sprintf("data_dir = '%s'\n\n[providers.claude]\ncommand = '%s'\n",
		dataDir, os.Args[0])
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	t.Setenv(config.EnvGlobalRules, filepath.Join(dir, "no-global-rules.md"))
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")

	// 扫描的 cwd 就是 serve 的启动目录；t.Chdir 在用例结束时自动恢复。
	t.Chdir(projectDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newSyncBuffer()
	errOut := newSyncBuffer()
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Options{
			Addr:       "127.0.0.1:0",
			ConfigPath: cfgPath,
			RW:         true,
			out:        out,
			errOut:     errOut,
		}, "test-version")
	}()

	select {
	case <-out.wrote:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("10 秒内没有打印启动行")
	}
	started := out.String()
	idx := strings.Index(started, "http://")
	if idx < 0 {
		cancel()
		t.Fatalf("启动行里没有 URL: %q", started)
	}
	url := strings.TrimSpace(started[idx:])

	// 大小闸的提示必须默认可见，且文案与任务书一致。
	logs := errOut.String()
	wantLog := fmt.Sprintf(`跳过 claude agent "big"：系统提示 %d 字节，超过 %d 上限`,
		len(bigPrompt), config.MaxAgentPromptBytes)
	if !strings.Contains(logs, wantLog) {
		t.Errorf("启动 stderr 里没有大小闸提示 %q：\n%s", wantLog, logs)
	}

	var opts optionsResponse
	getJSON(t, url+"/api/options", http.StatusOK, &opts)

	for _, agent := range opts.Agents {
		if agent.ID == "big" {
			t.Errorf("/api/options 不该出现超长的 big：%+v", opts.Agents)
		}
	}
	if ok := srv8AgentByID(t, opts, "ok"); ok.Source != agentSourceClaude {
		t.Errorf("正常 agent ok 的 source = %q，期望 %q", ok.Source, agentSourceClaude)
	}

	if len(opts.Skills) != 1 {
		t.Fatalf("options.skills 应当恰好 1 条，实际 %+v", opts.Skills)
	}
	skill := opts.Skills[0]
	if skill.ID != "deploy" || skill.Name != "deploy-staging" ||
		skill.Description != "部署到预发" || skill.Source != skillSourceClaude {
		t.Errorf("skill 字段不对: %+v", skill)
	}

	// 原始 JSON：skills 必须是数组（不是 null），四个字段都在。
	res, err := http.Get(url + "/api/options")
	if err != nil {
		t.Fatalf("GET /api/options 失败: %v", err)
	}
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Fatalf("读 /api/options 响应失败: %v", err)
	}
	t.Logf("GET /api/options -> %s", body)

	var raw struct {
		Skills []map[string]json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("按原始 JSON 解析 skills 失败: %v\n%s", err, body)
	}
	if len(raw.Skills) != 1 {
		t.Fatalf("原始 JSON 里 skills 有 %d 条，期望 1：%s", len(raw.Skills), body)
	}
	for _, key := range []string{"id", "name", "description", "source"} {
		if _, ok := raw.Skills[0][key]; !ok {
			t.Errorf("skill 的原始 JSON 缺少 %q 键: %v", key, raw.Skills[0])
		}
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("取消 ctx 之后 Run 返回错误: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("取消 ctx 之后 Run 超过 10 秒没有返回")
	}
}

// TestSrv10OptionsSkillsEmptyIsArray 钉住空表编码：没有 skill 时 /api/options
// 必须给 "skills":[]，不是 null——前端按数组渲染，null 会变成一类要单独兜底
// 的输入。这条不走 Run：装配方没填 Skills 时 handler 的行为也要稳定。
func TestSrv10OptionsSkillsEmptyIsArray(t *testing.T) {
	env := newWriteTestEnv(t)

	res, err := http.Get(env.http.URL + "/api/options")
	if err != nil {
		t.Fatalf("GET /api/options 失败: %v", err)
	}
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Fatalf("读 /api/options 响应失败: %v", err)
	}
	if !strings.Contains(string(body), `"skills":[]`) {
		t.Errorf(`没有 skill 时响应里应当有 "skills":[]，实际：%s`, body)
	}
}
