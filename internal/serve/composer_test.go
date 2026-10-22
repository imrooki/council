package serve

// composer_test.go 覆盖 SRV5（docs/22a_SRV5_COMPOSER_BACKEND_TASK.md §5）：
// /api/options、/api/ask 的 cwd / workspace / context、/api/jobs/{id}/cancel
// 与 turn_started 事件。
//
// 这里注册了一个测试专用的 provider：它把 Runner 真正交给它的 WorkDir 与
// Prompt 记进文件，再让子进程在自己的 cwd 里写一个 <agent>-cwd.txt。
// "Runner 真的在那个目录下跑"因此有两份独立证据——一份来自测试进程收到的
// 请求字段，一份来自子进程自己的 cwd。既有 fake claude CLI 只能证明
// "跑完了"，证明不了"在哪里跑"。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/headless"
	"council/internal/provider"
	"council/internal/store"
	"council/internal/turn"
)

// composerProbeProviderID 是测试 provider 的 id。它必须满足 config 的
// idPattern（小写、连字符在中间），并与测试配置里的 [providers.*] 同名。
const composerProbeProviderID = "composer-probe"

func init() { provider.Register(composerProbeProvider{}) }

// composerProbeProvider 是最小可用的 provider 适配器：BuildTurn 记录输入，
// ParseTurn 永远回一条成功结果。它不解析 stdout，所以子进程写什么都可以；
// 子进程唯一的职责是把真实 cwd 写进 <agentID>-cwd.txt。
type composerProbeProvider struct{}

func (composerProbeProvider) ID() string { return composerProbeProviderID }

func (composerProbeProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		SessionID:            provider.SessionIDReturnedOnFirstTurn,
		Effort:               provider.EffortPerTurnFlag,
		Identity:             provider.IdentityPromptPreamble,
		Prompt:               provider.PromptTransportArgv,
		Permissions:          []provider.PermissionTier{provider.PermissionRead, provider.PermissionWorkspace},
		UsageFormula:         "input_tokens",
		OutputSchema:         provider.SupportNo,
		MaxTurnsFlag:         provider.SupportNo,
		ReportsCost:          provider.SupportYes,
		ReportsCacheCreate:   provider.SupportNo,
		ReportsObservedModel: provider.SupportNo,
		CleanInstructionEnv:  provider.SupportNo,
		CacheAcrossTurns:     provider.SupportNo,
		WriteBoundary:        provider.SupportYes,
	}
}

// composerProbeRecordMu 守记录文件的追加。同一轮里两个成员会并发进
// BuildTurn，记录文件是同一个时不能让两次写入互相穿插。
var composerProbeRecordMu sync.Mutex

// composerProbeRecord 把一段值追加进环境变量指定的文件。变量为空表示
// 这次测试不关心这一项；记录失败不影响轮次执行（测试会读不到文件而红，
// 那是断言失败，不是执行失败）。
func composerProbeRecord(envName, value string) {
	path := strings.TrimSpace(os.Getenv(envName))
	if path == "" {
		return
	}
	composerProbeRecordMu.Lock()
	defer composerProbeRecordMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(value + "\n")
	_ = f.Close()
}

func (composerProbeProvider) BuildTurn(req provider.TurnRequest) (provider.Command, error) {
	if req.WorkDir == "" {
		return provider.Command{}, provider.ErrNoWorkDir
	}
	if req.Prompt != "" {
		composerProbeRecord("COUNCIL_PROBE_PROMPT_FILE", req.Prompt)
	}
	composerProbeRecord("COUNCIL_PROBE_WORKDIR_FILE", req.WorkDir)

	cmd := composerProbeCommand(req.AgentID)
	cmd.Dir = req.WorkDir
	return cmd, nil
}

func (composerProbeProvider) ParseTurn([]byte) (provider.TurnResult, error) {
	return provider.TurnResult{
		Text:            "PROBE-OK",
		ProviderSession: "composer-probe-session",
		Completion:      provider.CompletionComplete,
		Usage:           provider.Usage{InputTokens: 3, OutputTokens: 2},
	}, nil
}

// composerProbeCommand 组一条无害的命令：正常成员把真实 cwd 写进
// <agentID>-cwd.txt，id 为 slow 的成员睡够 30 秒，给取消用例留出窗口。
// 30 秒不是超时——取消一到，spawn 会杀掉整棵进程树。
func composerProbeCommand(agentID string) provider.Command {
	slow := agentID == "slow"
	if runtime.GOOS == "windows" {
		if slow {
			return provider.Command{Path: composerShellPath(), Args: []string{"/c", "ping -n 30 127.0.0.1 >NUL"}}
		}
		return provider.Command{Path: composerShellPath(), Args: []string{"/c", "cd > " + agentID + "-cwd.txt"}}
	}
	if slow {
		return provider.Command{Path: composerShellPath(), Args: []string{"-c", "sleep 30"}}
	}
	return provider.Command{Path: composerShellPath(), Args: []string{"-c", "pwd > " + agentID + "-cwd.txt"}}
}

// composerShellPath 是子进程用的 shell。配置里的 provider command 也写它，
// 但 BuildTurn 返回的 Path 已经显式给出，配置那一栏只是为了让
// config.Validate 的 "command 不能为空" 通过。
func composerShellPath() string {
	if runtime.GOOS == "windows" {
		if shell := strings.TrimSpace(os.Getenv("COMSPEC")); shell != "" {
			return shell
		}
		return "cmd.exe"
	}
	return "sh"
}

// composerEnv 复用 write_test.go 的写模式环境，只多两样：本次启动实际
// 读的配置文件路径与 serve 的默认 cwd（/api/options 要原样回它们）。
type composerEnv struct {
	*writeTestEnv
	cfgPath string
	dataDir string
}

// composerAgentBlock 生成一个 composer-probe 成员的 TOML 片段。
func composerAgentBlock(id, display, permission string) string {
	return fmt.Sprintf(`
[agents.%s]
display_name = "%s"
provider = "%s"
model = "probe-model"
effort = "high"
role = "工作台测试成员"
permissions = "%s"
mode = "discussion"
`, id, display, composerProbeProviderID, permission)
}

// composerClaudeAgentBlock 生成一个走既有 fake claude CLI 的成员。
func composerClaudeAgentBlock(id, display string) string {
	return fmt.Sprintf(`
[agents.%s]
display_name = "%s"
provider = "claude"
model = "opus"
effort = "high"
role = "工作台测试成员"
permissions = "read"
mode = "discussion"
`, id, display)
}

// newComposerEnv 照 newWriteTestEnv 的顺序装配，并把 ConfigPath / Cwd
// 填进 WriteDeps——/api/options 的 config_path 与 cwd 就是从那里来的。
func newComposerEnv(t *testing.T, agentsTOML, extraTOML, cwd string) *composerEnv {
	t.Helper()

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	body := fmt.Sprintf(`data_dir = '%s'

[providers.claude]
command = '%s'

[providers.%s]
command = '%s'

%s
%s
`, dataDir, os.Args[0], composerProbeProviderID, composerShellPath(), agentsTOML, extraTOML)
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写工作台测试配置失败: %v", err)
	}
	// 与 writeAskConfig 同一条纪律：不能读开发机上的全局规则，否则同一份
	// 用例在不同机器上会算出不同的身份指纹。
	t.Setenv(config.EnvGlobalRules, filepath.Join(dir, "no-global-rules.md"))
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")

	cfg, err := config.LoadAndValidate(cfgPath)
	if err != nil {
		t.Fatalf("加载工作台测试配置失败: %v", err)
	}
	rules, err := config.LoadRules(cfgPath, cfg.RulesFile)
	if err != nil {
		t.Fatalf("加载工作台测试规则失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("打开工作台测试库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭工作台测试库失败: %v", err)
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
		Cfg:        cfg,
		Svc:        svc,
		Rn:         rn,
		DataDir:    dataDir,
		ConfigPath: cfgPath,
		Cwd:        cwd,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &composerEnv{
		writeTestEnv: &writeTestEnv{store: st, srv: srv, http: ts},
		cfgPath:      cfgPath,
		dataDir:      dataDir,
	}
}

// waitResult 等 job 结束并取回 headless 写出的 Result。
func (e *composerEnv) waitResult(t *testing.T, id string) (headless.Result, error) {
	t.Helper()
	j, ok := e.srv.jobs.get(id)
	if !ok {
		t.Fatalf("任务 %s 不在表里", id)
	}
	select {
	case <-j.done:
	case <-time.After(20 * time.Second):
		t.Fatalf("任务 %s 20 秒内没有结束", id)
	}
	return j.outcome()
}

// composerReadFile 读一个测试产物；不存在时直接失败，避免断言里出现
// 一个空的 strings.Contains 假通过。
func composerReadFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", path, err)
	}
	return string(raw)
}

// TestOptionsEndpointReadOnly405AndWritableShape 是任务书 §5.1：
// 只读 405；-rw 200 且字段齐全、agents 按 id 字典序。
func TestOptionsEndpointReadOnly405AndWritableShape(t *testing.T) {
	t.Run("只读 405", func(t *testing.T) {
		env := newServeTestEnv(t, nil)
		res, err := http.Get(env.http.URL + "/api/options")
		if err != nil {
			t.Fatalf("GET /api/options 失败: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("只读模式 GET /api/options = %d，期望 405", res.StatusCode)
		}
		var body map[string]any
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("405 响应不是合法 JSON: %v", err)
		}
		msg, _ := body["error"].(string)
		if !strings.Contains(msg, "-rw") {
			t.Errorf("405 的说明没有告诉用户用 -rw: %q", msg)
		}
	})

	t.Run("-rw 200 字段齐全且按 id 排序", func(t *testing.T) {
		wantCwd := t.TempDir()
		env := newComposerEnv(t,
			composerAgentBlock("b", "B", "workspace")+composerAgentBlock("a", "A", "read"),
			`
[teams.review]
members = ["a", "b"]

[summary]
agent = "a"
`, wantCwd)

		var got optionsResponse
		getJSON(t, env.http.URL+"/api/options", http.StatusOK, &got)

		if got.Cwd != wantCwd {
			t.Errorf("options.cwd = %q，期望 serve 的 %q", got.Cwd, wantCwd)
		}
		if got.ConfigPath != env.cfgPath {
			t.Errorf("options.config_path = %q，期望 %q", got.ConfigPath, env.cfgPath)
		}
		if got.SummaryAgent != "a" {
			t.Errorf("options.summary_agent = %q，期望 a", got.SummaryAgent)
		}
		if len(got.Agents) != 2 {
			t.Fatalf("options.agents 有 %d 条，期望 2: %+v", len(got.Agents), got.Agents)
		}
		if got.Agents[0].ID != "a" || got.Agents[1].ID != "b" {
			t.Errorf("options.agents 顺序 = [%s %s]，期望 [a b]（按 id 字典序）",
				got.Agents[0].ID, got.Agents[1].ID)
		}
		a := got.Agents[0]
		if a.DisplayName != "A" || a.Provider != composerProbeProviderID ||
			a.Model != "probe-model" || a.Effort != "high" ||
			a.Permission != "read" || a.Mode != "discussion" || !a.Active {
			t.Errorf("agent a 的字段不完整或口径不对: %+v", a)
		}
		if got.Agents[1].Permission != "workspace" {
			t.Errorf("agent b 的 permission = %q，期望配置里的 workspace", got.Agents[1].Permission)
		}
		if len(got.Teams) != 1 || got.Teams[0].ID != "review" {
			t.Fatalf("options.teams = %+v，期望只有 review", got.Teams)
		}
		if strings.Join(got.Teams[0].Members, ",") != "a,b" {
			t.Errorf("review 的 members = %v，期望 [a b]", got.Teams[0].Members)
		}

		// 原始 JSON 的键必须一个不少：前端直接按这些键取值。
		var raw map[string]json.RawMessage
		getJSON(t, env.http.URL+"/api/options", http.StatusOK, &raw)
		for _, key := range []string{"cwd", "config_path", "summary_agent", "agents", "teams"} {
			if _, ok := raw[key]; !ok {
				t.Errorf("options 响应缺少键 %q: %v", key, raw)
			}
		}
	})
}

// TestAskCwdValidationAndRunnerWorkDir 是任务书 §5.2：不存在的目录 400、
// 相对路径 400、合法目录 200 且 Runner 真的在那个目录下跑。
func TestAskCwdValidationAndRunnerWorkDir(t *testing.T) {
	env := newComposerEnv(t, composerAgentBlock("c1", "C1", "read"), "", "")

	t.Run("相对路径 400", func(t *testing.T) {
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json",
			`{"prompt":"x","members":["c1"],"cwd":"relative/dir"}`)
		if status != http.StatusBadRequest {
			t.Fatalf("相对 cwd = %d，期望 400；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "绝对路径") {
			t.Errorf("400 的说明没有说清 cwd 必须是绝对路径: %s", raw)
		}
	})

	t.Run("不存在的目录 400", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "no-such-dir")
		body, _ := json.Marshal(map[string]any{
			"prompt": "x", "members": []string{"c1"}, "cwd": missing,
		})
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
		if status != http.StatusBadRequest {
			t.Fatalf("不存在的 cwd = %d，期望 400；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "不存在") {
			t.Errorf("400 的说明没有点名目录不存在: %s", raw)
		}
	})

	t.Run("合法目录真的在那里跑", func(t *testing.T) {
		target := t.TempDir()
		body, _ := json.Marshal(map[string]any{
			"prompt": "在指定目录里跑", "members": []string{"c1"}, "rounds": 1, "cwd": target,
		})
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
		if status != http.StatusAccepted {
			t.Fatalf("合法 cwd = %d，期望 202；body=%s", status, raw)
		}
		var reply struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil || reply.JobID == "" {
			t.Fatalf("202 响应不合法: err=%v body=%s", err, raw)
		}
		res, err := env.waitResult(t, reply.JobID)
		if err != nil {
			t.Fatalf("指定 cwd 的一轮失败: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("指定 cwd 的一轮 status = %q，期望 ok", res.Status)
		}
		// 子进程自己的 cwd：它在 cwd 里写了 c1-cwd.txt，内容就是它看到的目录。
		content := composerReadFile(t, filepath.Join(target, "c1-cwd.txt"))
		if !strings.Contains(content, target) {
			t.Errorf("子进程看到的 cwd 是 %q，期望包含 %q", strings.TrimSpace(content), target)
		}
	})
}

// TestAskWorkspaceRequiresGitRepoRoot 是任务书 §5.3：workspace=true 而
// 目录不是 git 仓库 → 400；是仓库时走既有 workspace 路径（成员在副本里跑）。
func TestAskWorkspaceRequiresGitRepoRoot(t *testing.T) {
	env := newComposerEnv(t, composerAgentBlock("c1", "C1", "read"), "", "")

	t.Run("不是仓库 400", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"prompt": "x", "members": []string{"c1"}, "workspace": true, "cwd": t.TempDir(),
		})
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
		if status != http.StatusBadRequest {
			t.Fatalf("workspace=true 指向非仓库 = %d，期望 400；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "git") {
			t.Errorf("400 的说明没有点名 git 仓库: %s", raw)
		}
	})

	t.Run("是仓库时在副本里跑", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("本机没有 git，跳过副本用例")
		}
		repo := t.TempDir()
		composerGit(t, repo, "init")
		composerGit(t, repo, "-c", "user.name=t", "-c", "user.email=t@t",
			"commit", "--allow-empty", "-m", "init")

		body, _ := json.Marshal(map[string]any{
			"prompt": "在副本里跑", "members": []string{"c1"}, "rounds": 1,
			"workspace": true, "cwd": repo,
		})
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
		if status != http.StatusAccepted {
			t.Fatalf("workspace=true = %d，期望 202；body=%s", status, raw)
		}
		var reply struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil || reply.JobID == "" {
			t.Fatalf("202 响应不合法: err=%v body=%s", err, raw)
		}
		res, err := env.waitResult(t, reply.JobID)
		if err != nil {
			t.Fatalf("workspace 一轮失败: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("workspace 一轮 status = %q，期望 ok", res.Status)
		}
		if res.Workspace == nil || res.Workspace.Copy == "" {
			t.Fatalf("结果里没有 workspace 副本: %+v", res.Workspace)
		}
		if _, statErr := os.Stat(res.Workspace.Copy); statErr != nil {
			t.Fatalf("workspace 副本 %s 不存在: %v", res.Workspace.Copy, statErr)
		}
		// 成员跑在副本里：副本里有子进程写的 cwd 文件，原仓库里没有。
		copyCwd := composerReadFile(t, filepath.Join(res.Workspace.Copy, "c1-cwd.txt"))
		if !strings.Contains(copyCwd, res.Workspace.Copy) {
			t.Errorf("副本里子进程看到的 cwd 是 %q，期望包含 %q", strings.TrimSpace(copyCwd), res.Workspace.Copy)
		}
		if _, statErr := os.Stat(filepath.Join(repo, "c1-cwd.txt")); statErr == nil {
			t.Errorf("原仓库里出现了成员写的 c1-cwd.txt：成员没有跑在副本里")
		}
		if out := composerGit(t, repo, "status", "--porcelain"); out != "" {
			t.Errorf("原仓库被写脏了：\n%s", out)
		}
	})
}

// TestAskContextValidationAndPromptPrefix 是任务书 §5.4：超 20 条 400、
// 相对路径 400；正常时 prompt 里出现那段前置说明。
func TestAskContextValidationAndPromptPrefix(t *testing.T) {
	env := newComposerEnv(t, composerAgentBlock("c1", "C1", "read"), "", "")

	t.Run("超 20 条 400", func(t *testing.T) {
		paths := make([]string, 0, maxContextPaths+1)
		for i := 0; i <= maxContextPaths; i++ {
			paths = append(paths, filepath.Join(t.TempDir(), fmt.Sprintf("f%d.go", i)))
		}
		body, _ := json.Marshal(map[string]any{
			"prompt": "x", "members": []string{"c1"}, "context": paths,
		})
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
		if status != http.StatusBadRequest {
			t.Fatalf("context %d 条 = %d，期望 400；body=%s", len(paths), status, raw)
		}
		if !strings.Contains(string(raw), "20") {
			t.Errorf("400 的说明没有点名 20 条上限: %s", raw)
		}
	})

	t.Run("相对路径 400", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"prompt": "x", "members": []string{"c1"}, "context": []string{"main.go"},
		})
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
		if status != http.StatusBadRequest {
			t.Fatalf("相对 context = %d，期望 400；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "绝对路径") {
			t.Errorf("400 的说明没有说清 context 必须是绝对路径: %s", raw)
		}
	})

	t.Run("单条超 512 字节 400", func(t *testing.T) {
		long := filepath.Join(t.TempDir(), strings.Repeat("a", maxContextPathBytes+1))
		body, _ := json.Marshal(map[string]any{
			"prompt": "x", "members": []string{"c1"}, "context": []string{long},
		})
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
		if status != http.StatusBadRequest {
			t.Fatalf("超长 context = %d，期望 400；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "512") {
			t.Errorf("400 的说明没有点名 512 字节上限: %s", raw)
		}
	})

	t.Run("正常时拼进 prompt", func(t *testing.T) {
		promptFile := filepath.Join(t.TempDir(), "prompt.txt")
		t.Setenv("COUNCIL_PROBE_PROMPT_FILE", promptFile)

		first := filepath.Join(t.TempDir(), "main.go")
		second := filepath.Join(t.TempDir(), "internal")
		body, _ := json.Marshal(map[string]any{
			"prompt":  "解释这两处",
			"members": []string{"c1"},
			"rounds":  1,
			"cwd":     t.TempDir(),
			"context": []string{first, second},
		})
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
		if status != http.StatusAccepted {
			t.Fatalf("带 context 的请求 = %d，期望 202；body=%s", status, raw)
		}
		var reply struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil || reply.JobID == "" {
			t.Fatalf("202 响应不合法: err=%v body=%s", err, raw)
		}
		res, err := env.waitResult(t, reply.JobID)
		if err != nil || res.Status != "ok" {
			t.Fatalf("带 context 的一轮失败: status=%q err=%v", res.Status, err)
		}

		recorded := composerReadFile(t, promptFile)
		for _, want := range []string{
			"相关资料（请自行按路径读取）：\n",
			"- " + first,
			"- " + second,
			"解释这两处",
		} {
			if !strings.Contains(recorded, want) {
				t.Errorf("provider 收到的 prompt 里没有 %q；实际记录:\n%s", want, recorded)
			}
		}
		// 成员看到的是「本轮指令 + 历史」。资料清单只进落库的那条 user
		// 消息（它才决定会话标题之外的内容），所以顺序判据要对着历史里
		// 的那一次问题，而不是指令里那次。
		header := strings.Index(recorded, "相关资料（请自行按路径读取）：\n")
		if header < 0 {
			t.Fatalf("记录里没有资料清单:\n%s", recorded)
		}
		if lastQuestion := strings.LastIndex(recorded, "解释这两处"); header > lastQuestion {
			t.Errorf("资料清单出现在问题正文之后:\n%s", recorded)
		}
	})
}

// TestCancelJobKeepsCompletedRepliesAndEndsSSE 是任务书 §5.5：慢成员还在
// 飞时取消，job 必须结束、已经跑完的成员回复仍在库里、SSE 收到 done。
func TestCancelJobKeepsCompletedRepliesAndEndsSSE(t *testing.T) {
	env := newComposerEnv(t,
		composerAgentBlock("fast", "Fast", "read")+composerAgentBlock("slow", "Slow", "read"),
		"", t.TempDir())

	jobID := postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":  "取消测试",
		"members": []string{"fast", "slow"},
		"rounds":  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader := newSSEReader(t, ctx, env.http.URL+"/api/jobs/"+jobID+"/events")
	defer reader.close()

	// started 在第一位成员起进程之前就发，用它拿 convID 去轮询库。
	var convID string
	for convID == "" {
		frame, ok := reader.next(t)
		if !ok {
			t.Fatal("SSE 在 started 之前就结束了")
		}
		if frame.Event != "started" {
			continue
		}
		var started startedEvent
		if err := json.Unmarshal(frame.Data, &started); err != nil {
			t.Fatalf("started.data 不是合法 JSON: %v", err)
		}
		convID = started.ConversationID
	}

	// 等 fast 的回复进库：此刻 slow 仍在跑，取消才有判别力。
	deadline := time.Now().Add(15 * time.Second)
	fastStored := false
	for !fastStored {
		msgs, err := env.store.Messages(context.Background(), convID, 0, 0)
		if err != nil {
			t.Fatalf("读会话消息失败: %v", err)
		}
		for _, m := range msgs {
			if m.Kind == conversation.KindAgent && m.SenderID == "fast" && m.Text == "PROBE-OK" {
				fastStored = true
			}
		}
		if fastStored {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("15 秒内 fast 的回复没有落库")
		}
		time.Sleep(20 * time.Millisecond)
	}

	status, ctype, raw := postRaw(t, env.http.URL+"/api/jobs/"+jobID+"/cancel", "application/json", "{}")
	if status != http.StatusAccepted {
		t.Fatalf("POST cancel = %d，期望 202；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("cancel 的 Content-Type = %q", ctype)
	}
	var cancelled cancelResponse
	if err := json.Unmarshal(raw, &cancelled); err != nil || !cancelled.Cancelled {
		t.Fatalf("cancel 响应 = %s，期望 {\"cancelled\":true}", raw)
	}

	// 重复取消幂等。
	status, _, raw = postRaw(t, env.http.URL+"/api/jobs/"+jobID+"/cancel", "application/json", "{}")
	if status != http.StatusAccepted || !strings.Contains(string(raw), "true") {
		t.Errorf("第二次 cancel = %d %s，期望 202 且 cancelled=true", status, raw)
	}

	frames := reader.readUntil(t, "done")
	last := frames[len(frames)-1]
	var done doneEvent
	if err := json.Unmarshal(last.Data, &done); err != nil {
		t.Fatalf("done.data 不是合法 JSON: %v", err)
	}
	// 取消沿用 headless 既有口径（契约 §2.4）：至少一条回复已经拿到时是
	// partial，一条都没有才是 error。这里 fast 已经落库，所以必须是 partial。
	if done.Status != "partial" {
		t.Errorf("取消后的 done.status = %q，期望 partial（已有成员回复）", done.Status)
	}
	if done.Total != 2 {
		t.Errorf("done.total = %d，期望 2", done.Total)
	}

	if _, err := env.waitResult(t, jobID); err != nil && done.Status != "partial" {
		t.Errorf("取消后 job 返回错误: %v", err)
	}

	// 取消之后再读一次库：fast 的回复必须还在。
	msgs, err := env.store.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("取消后读会话消息失败: %v", err)
	}
	stillThere := false
	for _, m := range msgs {
		if m.Kind == conversation.KindAgent && m.SenderID == "fast" && m.Text == "PROBE-OK" {
			stillThere = true
		}
	}
	if !stillThere {
		t.Errorf("取消之后 fast 的回复从库里消失了：%+v", msgs)
	}

	// 未知 job 是 404，且与"已结束的 job"区分开。
	res, err := http.Post(env.http.URL+"/api/jobs/no-such-job/cancel", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("取消未知 job 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("取消未知 job = %d，期望 404", res.StatusCode)
	}
}

// TestTurnStartedBeforeTurnForEachMember 是任务书 §5.6：两个成员各一条
// turn_started，且都在自己的 turn 之前到达；回调为 nil 时既有 headless
// 测试全绿（由既有用例覆盖，本用例不重复）。
func TestTurnStartedBeforeTurnForEachMember(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	// 让每个成员都睡够：两条 turn_started 必须在任何一条 turn 之前发出，
	// 否则「思考中」这个信号在并发轮次里就是随机的。
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "800")

	env := newComposerEnv(t,
		composerClaudeAgentBlock("a", "A")+composerClaudeAgentBlock("b", "B"),
		"", t.TempDir())

	jobID := postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":  "两个成员",
		"members": []string{"a", "b"},
		"rounds":  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader := newSSEReader(t, ctx, env.http.URL+"/api/jobs/"+jobID+"/events")
	defer reader.close()
	frames := reader.readUntil(t, "done")
	t.Logf("SSE 原始报文：\n%s", reader.rawText())

	startIndex := make(map[string]int)
	turnIndex := make(map[string]int)
	firstTurn := -1
	for i, frame := range frames {
		switch frame.Event {
		case "turn_started":
			var ev turnStartedEvent
			if err := json.Unmarshal(frame.Data, &ev); err != nil {
				t.Fatalf("turn_started.data 不是合法 JSON: %v；%s", err, frame.Data)
			}
			if ev.AgentID == "" {
				t.Errorf("turn_started.data 缺少 agent_id: %s", frame.Data)
			}
			if _, dup := startIndex[ev.AgentID]; dup {
				t.Errorf("成员 %s 在单轮里收到多条 turn_started", ev.AgentID)
			}
			startIndex[ev.AgentID] = i
		case "turn":
			var ev turnEvent
			if err := json.Unmarshal(frame.Data, &ev); err != nil {
				t.Fatalf("turn.data 不是合法 JSON: %v；%s", err, frame.Data)
			}
			turnIndex[ev.AgentID] = i
			if firstTurn < 0 {
				firstTurn = i
			}
		}
	}

	if len(startIndex) != 2 {
		t.Fatalf("turn_started 有 %d 个成员，期望 2：%v", len(startIndex), startIndex)
	}
	for _, id := range []string{"a", "b"} {
		si, ok := startIndex[id]
		if !ok {
			t.Fatalf("成员 %s 没有 turn_started", id)
		}
		ti, ok := turnIndex[id]
		if !ok {
			t.Fatalf("成员 %s 没有 turn", id)
		}
		if si > ti {
			t.Errorf("成员 %s 的 turn_started（第 %d 帧）在 turn（第 %d 帧）之后", id, si, ti)
		}
		if firstTurn >= 0 && si > firstTurn {
			t.Errorf("成员 %s 的 turn_started（第 %d 帧）在第一条 turn（第 %d 帧）之后", id, si, firstTurn)
		}
	}
}

// TestReadOnlyComposerWriteEndpoints405 是任务书 §5.7：只读模式下三个
// 写接口（/api/ask、/api/options、/api/jobs/{id}/cancel）全 405。
func TestReadOnlyComposerWriteEndpoints405(t *testing.T) {
	env := newServeTestEnv(t, nil)

	check := func(t *testing.T, status int, raw []byte, what string) {
		t.Helper()
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d，期望 405；body=%s", what, status, raw)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("%s 的 405 不是合法 JSON: %v；%s", what, err, raw)
		}
		msg, _ := body["error"].(string)
		if !strings.Contains(msg, "-rw") {
			t.Errorf("%s 的 405 没有告诉用户用 -rw: %q", what, msg)
		}
	}

	status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", `{"prompt":"x"}`)
	check(t, status, raw, "POST /api/ask")

	res, err := http.Get(env.http.URL + "/api/options")
	if err != nil {
		t.Fatalf("GET /api/options 失败: %v", err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	check(t, res.StatusCode, raw, "GET /api/options")

	// 取消端点按浏览器可能发出的形状打：空 body、不带 Content-Type。
	cancelRes, err := http.Post(env.http.URL+"/api/jobs/some-job/cancel", "", nil)
	if err != nil {
		t.Fatalf("POST /api/jobs/.../cancel 失败: %v", err)
	}
	raw, _ = io.ReadAll(cancelRes.Body)
	cancelRes.Body.Close()
	check(t, cancelRes.StatusCode, raw, "POST /api/jobs/{id}/cancel")
}

// composerGit 跑一条 git 命令；失败直接失败用例。测试里调用要带
// -c user.name / user.email，本机 git 没配身份时 commit 会拒。
func composerGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v 失败: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
