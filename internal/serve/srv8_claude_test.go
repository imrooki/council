package serve

// srv8_claude_test.go 覆盖 SRV8（docs/25b_SRV8_CLAUDE_AGENTS_RUNNABLE_TASK.md
// §5）：serve -rw 扫到的 Claude agents 在运行时合并进 cfg.Agents 之后，
// /api/options 的来源、/api/ask 的校验与执行，以及缺 provider 时的整体跳过。
//
// 这些用例刻意都从 importClaudeAgents 或 Run 进去，而不是手工往 WriteDeps
// 里塞 ClaudeAgents：本片要证明的正是「扫描结果被合并进内存配置」这一步，
// 手工塞数据只能证明 handler 会展示它。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/store"
	"council/internal/turn"
)

// srv8IsolateClaudeHome 把用户主目录指到空目录。ScanClaudeAgents 会扫
// ~/.claude/agents，测试不能读开发机上真实存在的 agent：否则同一份用例
// 在不同机器上会看到不同的成员表，断言也不再稳定。
func srv8IsolateClaudeHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

// srv8WriteClaudeAgent 在项目的 .claude/agents 下落一份 agent 文件。
func srv8WriteClaudeAgent(t *testing.T, projectDir, fileName, frontmatter, prompt string) {
	t.Helper()
	dir := filepath.Join(projectDir, ".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建 Claude agents 目录失败: %v", err)
	}
	body := "---\n" + frontmatter + "---\n\n" + prompt + "\n"
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(body), 0o644); err != nil {
		t.Fatalf("写 Claude agent 文件失败: %v", err)
	}
}

// srv8ClaudeAgentBody 是测试里最常见的那份 critic 定义：只读、模型 sonnet。
func srv8ClaudeAgentBody() (frontmatter, prompt string) {
	return "name: critic\ndescription: 从 Claude 导入的批评者\nmodel: sonnet\ntools: Read, Grep\n",
		"你是批评者，只挑问题。"
}

// srv8Env 是「照 runWritable 的顺序装配、但可以指定配置正文」的写模式环境。
type srv8Env struct {
	*writeTestEnv
	cfgPath  string
	dataDir  string
	imported claudeImport
}

// newSrv8Env 落配置、加载、调用生产代码 importClaudeAgents 完成合并，
// 再照 runWritable 的顺序装配 Service/Runner 与 HTTP handler。
func newSrv8Env(t *testing.T, withClaudeProvider bool, agentsTOML, projectDir string) *srv8Env {
	t.Helper()

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	var body strings.Builder
	fmt.Fprintf(&body, "data_dir = '%s'\n\n", dataDir)
	if withClaudeProvider {
		fmt.Fprintf(&body, "[providers.claude]\ncommand = '%s'\n\n", os.Args[0])
	}
	body.WriteString(agentsTOML)

	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(body.String()), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	// 与既有写模式用例同一条纪律：不能读开发机上的全局规则文件。
	t.Setenv(config.EnvGlobalRules, filepath.Join(dir, "no-global-rules.md"))
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")

	cfg, err := config.LoadAndValidate(cfgPath)
	if err != nil {
		t.Fatalf("加载测试配置失败: %v", err)
	}
	rules, err := config.LoadRules(cfgPath, cfg.RulesFile)
	if err != nil {
		t.Fatalf("加载测试规则失败: %v", err)
	}

	imported := importClaudeAgents(cfg, projectDir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试库失败: %v", err)
		}
	})

	svc := conversation.NewService(st)
	svc.SetThoughtPolicy(cfg.StoreThought)
	rn := turn.NewRunner(cfg, svc, filepath.Join(dataDir, "workdir"), turn.RulesBlocks{
		Global: rules.GlobalText,
		Local:  rules.LocalText,
	})

	srv := NewServer(st, "test-version", modelsFromConfig(cfg))
	srv.EnableWrite(WriteDeps{
		Cfg:                         cfg,
		Svc:                         svc,
		Rn:                          rn,
		DataDir:                     dataDir,
		ConfigPath:                  cfgPath,
		Cwd:                         projectDir,
		ClaudeAgents:                imported.agents,
		ClaudeAgentsMerged:          imported.merged,
		ClaudeAgentsWithoutProvider: imported.withoutProvider,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &srv8Env{
		writeTestEnv: &writeTestEnv{store: st, srv: srv, http: ts},
		cfgPath:      cfgPath,
		dataDir:      dataDir,
		imported:     imported,
	}
}

// srv8Options 取一次 /api/options。
func srv8Options(t *testing.T, env *srv8Env) optionsResponse {
	t.Helper()
	var got optionsResponse
	getJSON(t, env.http.URL+"/api/options", http.StatusOK, &got)
	return got
}

// srv8AgentByID 在 options 的 agents 里找一条；找不到直接失败。
func srv8AgentByID(t *testing.T, opts optionsResponse, id string) agentOption {
	t.Helper()
	for _, agent := range opts.Agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("options.agents 里没有 %q：%+v", id, opts.Agents)
	return agentOption{}
}

// postAskSrv8 发一次 /api/ask 并返回 job id。走 HTTP 而不是直接调 handler，
// 与工作台真实路径一致。
func postAskSrv8(t *testing.T, env *srv8Env, members []string) string {
	t.Helper()
	return postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":  "SRV8 测试",
		"members": members,
		"rounds":  1,
	})
}

// testSrv8ClaudeAgentRunsViaAsk 是任务书 §5.1：配了 [providers.claude] 与
// 项目 .claude/agents/critic.md 时，POST /api/ask 选 critic 必须 202，
// 并且成员真的由 fake provider 跑完。
//
// 这一条从 Run 进去，覆盖 runWritable 里的扫描 + 合并 + 装配顺序，
// 而不是只测合并函数。
func TestSrv8ClaudeAgentRunsViaAsk(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	srv8IsolateClaudeHome(t)

	projectDir := t.TempDir()
	frontmatter, prompt := srv8ClaudeAgentBody()
	srv8WriteClaudeAgent(t, projectDir, "critic.md", frontmatter, prompt)

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
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Options{
			Addr:       "127.0.0.1:0",
			ConfigPath: cfgPath,
			RW:         true,
			out:        out,
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

	// 合并后 /api/options 必须能列出 critic，并且来源仍是 claude。
	var opts optionsResponse
	getJSON(t, url+"/api/options", http.StatusOK, &opts)
	critic := srv8AgentByID(t, opts, "critic")
	if critic.Source != agentSourceClaude {
		t.Errorf("critic.source = %q，期望 %q", critic.Source, agentSourceClaude)
	}
	if critic.Provider != "claude" || critic.Model != "sonnet" ||
		critic.Permission != "read" || critic.Mode != "discussion" {
		t.Errorf("critic 的映射字段不对: %+v", critic)
	}

	payload, err := json.Marshal(map[string]any{
		"prompt":  "让 critic 说一句",
		"members": []string{"critic"},
		"rounds":  1,
	})
	if err != nil {
		t.Fatalf("编码 ask 请求失败: %v", err)
	}
	status, _, raw := postRaw(t, url+"/api/ask", "application/json", string(payload))
	if status != http.StatusAccepted {
		t.Fatalf("POST /api/ask members=[critic] = %d，期望 202；body=%s", status, raw)
	}
	var reply struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil || reply.JobID == "" {
		t.Fatalf("202 响应不合法: err=%v body=%s", err, raw)
	}

	sseCtx, sseCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer sseCancel()
	reader := newSSEReader(t, sseCtx, url+"/api/jobs/"+reply.JobID+"/events")
	defer reader.close()
	frames := reader.readUntil(t, "done")

	var convID string
	var sawCritic bool
	for _, frame := range frames {
		switch frame.Event {
		case "started":
			var ev startedEvent
			if err := json.Unmarshal(frame.Data, &ev); err != nil {
				t.Fatalf("started.data 不是合法 JSON: %v", err)
			}
			convID = ev.ConversationID
		case "turn":
			var ev turnEvent
			if err := json.Unmarshal(frame.Data, &ev); err != nil {
				t.Fatalf("turn.data 不是合法 JSON: %v", err)
			}
			if ev.AgentID == "critic" && ev.Outcome == "ok" && ev.Text == "OK" {
				sawCritic = true
			}
		}
	}
	if !sawCritic {
		t.Fatalf("SSE 里没有 critic 的成功发言；原始报文：\n%s", reader.rawText())
	}
	var done doneEvent
	if err := json.Unmarshal(frames[len(frames)-1].Data, &done); err != nil {
		t.Fatalf("done.data 不是合法 JSON: %v", err)
	}
	if done.Status != "ok" || done.Total != 1 || done.Failed != 0 {
		t.Errorf("done = %+v，期望 ok/0 failed/1 total", done)
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

	// SSE 只是观察面；最后再从库里确认 critic 的回复真的落盘了。
	st, err := store.OpenReadOnly(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Run 返回后只读打开库失败: %v", err)
	}
	defer func() { _ = st.Close() }()
	msgs, err := st.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("读会话 %s 的消息失败: %v", convID, err)
	}
	for _, m := range msgs {
		if m.Kind == conversation.KindAgent && m.SenderID == "critic" && m.Text == "OK" {
			return
		}
	}
	t.Errorf("库里没有 critic 的回复：%+v", msgs)
}

// TestSrv8ClaudeAgentsSkippedWithoutProvider 是任务书 §5.2：没有
// [providers.claude] 时整体跳过——/api/options 不出现该 agent，
// /api/ask 选它回 400，而且提示里说清楚缺的是 provider。
func TestSrv8ClaudeAgentsSkippedWithoutProvider(t *testing.T) {
	srv8IsolateClaudeHome(t)
	projectDir := t.TempDir()
	frontmatter, prompt := srv8ClaudeAgentBody()
	srv8WriteClaudeAgent(t, projectDir, "critic.md", frontmatter, prompt)

	env := newSrv8Env(t, false, "", projectDir)

	if env.imported.warning == "" {
		t.Fatal("缺 [providers.claude] 时应当有一条可读提示")
	}
	if !strings.Contains(env.imported.warning, "[providers.claude]") ||
		!strings.Contains(env.imported.warning, "critic") {
		t.Errorf("缺 provider 的提示没有点名 provider 与成员：%q", env.imported.warning)
	}
	if _, merged := env.srv.write.Cfg.Agents["critic"]; merged {
		t.Error("缺 [providers.claude] 时 critic 不该被合并进 cfg.Agents")
	}
	if len(env.srv.write.ClaudeAgents) != 0 {
		t.Errorf("缺 provider 时不该把 Claude agents 交给 /api/options：%+v", env.srv.write.ClaudeAgents)
	}

	opts := srv8Options(t, env)
	for _, agent := range opts.Agents {
		if agent.ID == "critic" {
			t.Errorf("/api/options 不该出现 critic：%+v", opts.Agents)
		}
	}

	body, _ := json.Marshal(map[string]any{"prompt": "选一个不存在的成员", "members": []string{"critic"}})
	status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
	if status != http.StatusBadRequest {
		t.Fatalf("缺 provider 时 POST /api/ask members=[critic] = %d，期望 400；body=%s", status, raw)
	}
	message := string(raw)
	if !strings.Contains(message, "[providers.claude]") {
		t.Errorf("400 的提示没有说清缺 [providers.claude]：%s", raw)
	}
	if !strings.Contains(message, "critic") {
		t.Errorf("400 的提示没有点名 critic：%s", raw)
	}

	env.srv.jobs.mu.Lock()
	jobCount := len(env.srv.jobs.jobs)
	env.srv.jobs.mu.Unlock()
	if jobCount != 0 {
		t.Errorf("被拒的请求建出了 %d 个任务，期望 0", jobCount)
	}
}

// TestSrv8ClaudeNameConflictConfigWins 是任务书 §5.3：配置里已有 critic 时
// 用配置那一份（model / role 都来自配置），且 /api/options 里只有一条。
func TestSrv8ClaudeNameConflictConfigWins(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	srv8IsolateClaudeHome(t)
	projectDir := t.TempDir()
	frontmatter, prompt := srv8ClaudeAgentBody()
	srv8WriteClaudeAgent(t, projectDir, "critic.md", frontmatter, prompt)

	agentsTOML := `
[agents.critic]
display_name = "配置里的 Critic"
provider = "claude"
model = "config-model"
effort = "high"
role = "配置里的角色"
permissions = "read"
mode = "discussion"
`
	env := newSrv8Env(t, true, agentsTOML, projectDir)

	ac := env.srv.write.Cfg.Agents["critic"]
	if ac.Model != "config-model" || ac.Role != "配置里的角色" {
		t.Errorf("同名时应当保留配置的定义，实际 model=%q role=%q", ac.Model, ac.Role)
	}
	if _, ok := env.imported.agents["critic"]; ok {
		t.Error("与配置同名的 Claude agent 不该出现在合并结果里")
	}
	if _, ok := env.srv.write.ClaudeAgents["critic"]; ok {
		t.Error("与配置同名的 Claude agent 不该交给 /api/options 的 Claude 段")
	}

	opts := srv8Options(t, env)
	count := 0
	for _, agent := range opts.Agents {
		if agent.ID != "critic" {
			continue
		}
		count++
		if agent.Source != agentSourceConfig {
			t.Errorf("冲突时 critic.source = %q，期望 %q", agent.Source, agentSourceConfig)
		}
		if agent.Model != "config-model" {
			t.Errorf("冲突时 critic.model = %q，期望配置里的 config-model", agent.Model)
		}
	}
	if count != 1 {
		t.Errorf("/api/options 里 critic 出现 %d 条，期望恰好 1 条：%+v", count, opts.Agents)
	}

	jobID := postAskSrv8(t, env, []string{"critic"})
	if got := waitJobStatus(t, env.writeTestEnv, jobID); got != "ok" {
		t.Errorf("配置版 critic 的 ask status = %q，期望 ok", got)
	}
}

// TestSrv8ClaudeMergeKeepsConfigAgentsWorking 是任务书 §5.4：合并之后既有
// 配置成员的行为不变——普通 agent 仍能选中、执行、落库。
func TestSrv8ClaudeMergeKeepsConfigAgentsWorking(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	srv8IsolateClaudeHome(t)

	projectDir := t.TempDir()
	frontmatter, prompt := srv8ClaudeAgentBody()
	srv8WriteClaudeAgent(t, projectDir, "critic.md", frontmatter, prompt)

	agentsTOML := `
[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "既有配置成员"
permissions = "read"
mode = "discussion"
`
	env := newSrv8Env(t, true, agentsTOML, projectDir)

	opts := srv8Options(t, env)
	if len(opts.Agents) != 2 {
		t.Fatalf("合并后 options.agents 有 %d 条，期望 c1 + critic：%+v", len(opts.Agents), opts.Agents)
	}
	if c1 := srv8AgentByID(t, opts, "c1"); c1.Source != agentSourceConfig || c1.Model != "opus" {
		t.Errorf("既有配置成员 c1 被合并影响了：%+v", c1)
	}
	if critic := srv8AgentByID(t, opts, "critic"); critic.Source != agentSourceClaude {
		t.Errorf("critic.source = %q，期望 %q", critic.Source, agentSourceClaude)
	}

	jobID := postAskSrv8(t, env, []string{"c1"})
	if got := waitJobStatus(t, env.writeTestEnv, jobID); got != "ok" {
		t.Fatalf("既有配置成员 c1 的 ask status = %q，期望 ok", got)
	}
	convID := ""
	convs, err := env.store.Conversations(context.Background())
	if err != nil {
		t.Fatalf("读会话列表失败: %v", err)
	}
	for _, conv := range convs {
		msgs, err := env.store.Messages(context.Background(), conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("读会话 %s 的消息失败: %v", conv.ID, err)
		}
		for _, m := range msgs {
			if m.Kind == conversation.KindAgent && m.SenderID == "c1" && m.Text == "OK" {
				convID = conv.ID
			}
		}
	}
	if convID == "" {
		t.Error("合并之后 c1 的回复没有落库")
	}
}

// TestSrv8ReadOnlyUnaffectedByClaudeAgents 是任务书 §5.5：只读模式没有
// 工作台写接口；仓库里有没有 .claude/agents 都改变不了这一点。
func TestSrv8ReadOnlyUnaffectedByClaudeAgents(t *testing.T) {
	env := newServeTestEnv(t, nil)

	res, err := http.Get(env.http.URL + "/api/options")
	if err != nil {
		t.Fatalf("GET /api/options 失败: %v", err)
	}
	rawOptions, _ := readAllAndClose(res)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式 GET /api/options = %d，期望 405；body=%s", res.StatusCode, rawOptions)
	}
	if !strings.Contains(string(rawOptions), "-rw") {
		t.Errorf("只读模式 /api/options 的说明没有告诉用户用 -rw：%s", rawOptions)
	}

	status, _, rawAsk := postRaw(t, env.http.URL+"/api/ask", "application/json",
		`{"prompt":"x","members":["critic"]}`)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式 POST /api/ask = %d，期望 405；body=%s", status, rawAsk)
	}

	var health map[string]any
	getJSON(t, env.http.URL+"/api/health", http.StatusOK, &health)
	if _, exists := health["writable"]; exists {
		t.Errorf("只读模式的 health 不该出现 writable 键：%v", health)
	}
}

// readAllAndClose 读空响应体；只读模式的两个 405 都需要检查文案。
func readAllAndClose(res *http.Response) ([]byte, error) {
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}
