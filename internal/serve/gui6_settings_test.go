package serve

// gui6_settings_test.go 覆盖 GUI 路线第 6 片「设置页」的 Go 侧：
//
//  1. 保存 agent 只改目标文件里的那一段，另一层配置文件逐字节不变；
//  2. 保存前先备份，备份内容等于写前原文；
//  3. 校验失败回 400，目标文件一个字节不动、不留备份与临时文件；
//  4. 删除只移除目标段，其它段保留；删不存在的回 404，删继承来的回 409；
//  5. 写完之后 /api/settings 与 /api/options 都反映新值；已有成员的身份
//     字段改动当场热换进 RunCfg（A3），新增成员仍然只在展示配置里；
//  6. 只读模式下三个设置端点全部 405。
//
// 每个用例都在 t.TempDir() 里造「项目层 + 全局层」两份配置，并把 HOME /
// USERPROFILE 指到临时目录：绝不读写开发机真实的 ~/.council 或仓库自己的
// .council。浏览器侧（打开设置、预览、保存提示、刷新后仍是新值）由 CDP
// 验收覆盖，结果记在交付报告里；这里不实现 JS 解释器。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/store"
	"council/internal/turn"
)

// gui6Env 是两层配置的写模式环境。
type gui6Env struct {
	*writeTestEnv
	projectDir    string
	projectConfig string
	globalConfig  string
	home          string
	dataDir       string
	layers        []config.Layer
}

// newGui6Env 照 runWritable 的顺序装配：ResolveConfigLayers（项目层在前、
// 全局层在后）-> LoadMerged -> LoadRules -> store.Open -> Service/Runner ->
// EnableWrite。cwd 切到临时项目目录，测试结束再切回去。
func newGui6Env(t *testing.T) *gui6Env {
	t.Helper()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	project := filepath.Join(base, "project")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(filepath.Join(home, ".council"), 0o755); err != nil {
		t.Fatalf("建临时 HOME 失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".council"), 0o755); err != nil {
		t.Fatalf("建临时项目目录失败: %v", err)
	}
	// .git 让 projectDirs 停在 project：否则向上走到盘符根，可能把开发机上
	// 别的 .council 当成本项目的一部分。
	if err := os.WriteFile(filepath.Join(project, ".git"), []byte("gitdir: fake\n"), 0o644); err != nil {
		t.Fatalf("写临时 .git 失败: %v", err)
	}

	globalPath := filepath.Join(home, ".council", "config.toml")
	projectPath := filepath.Join(project, ".council", "config.toml")
	globalBody := fmt.Sprintf(`# 全局层：data_dir 只在这一层生效
data_dir = '%s'

[providers.claude]
command = '%s'

[agents.g1]
display_name = "Global One"
provider = "claude"
model = "opus"
role = "全局层成员"
permissions = "read"
mode = "discussion"

[teams.t1]
display_name = "Team 1"
members = ["g1"]

[summary]
agent = "g1"
`, dataDir, os.Args[0])
	projectBody := `# 项目层：只写这个项目自己的成员
[agents.p1]
display_name = "Project One"
provider = "claude"
model = "sonnet"
role = "项目层成员"
permissions = "read"
mode = "discussion"

[agents.p2]
display_name = "Project Two"
provider = "claude"
model = "opus"
role = "待删除成员"
permissions = "read"
mode = "worker"
`
	if err := os.WriteFile(globalPath, []byte(globalBody), 0o644); err != nil {
		t.Fatalf("写全局配置失败: %v", err)
	}
	if err := os.WriteFile(projectPath, []byte(projectBody), 0o644); err != nil {
		t.Fatalf("写项目配置失败: %v", err)
	}

	// 环境隔离：HOME / USERPROFILE 指到临时目录，COUNCIL_CONFIG 与
	// COUNCIL_DATA_DIR 清空，全局规则文件指到一个不存在的路径。
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")
	t.Setenv(config.EnvGlobalRules, filepath.Join(base, "no-global-rules.md"))

	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("读当前目录失败: %v", err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatalf("切到临时项目目录失败: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prevCwd); err != nil {
			t.Errorf("恢复工作目录失败: %v", err)
		}
	})

	layers, err := config.ResolveConfigLayers("")
	if err != nil {
		t.Fatalf("解析配置层失败: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("配置层 = %+v，期望项目层 + 全局层两层", layers)
	}
	if layers[0].Path != projectPath || !layers[0].Project {
		t.Fatalf("第一层 = %+v，期望项目层 %s", layers[0], projectPath)
	}
	if layers[1].Path != globalPath || layers[1].Project {
		t.Fatalf("第二层 = %+v，期望全局层 %s", layers[1], globalPath)
	}

	merged, err := config.LoadMerged(layers)
	if err != nil {
		t.Fatalf("加载合并配置失败: %v", err)
	}
	rules, err := config.LoadRules(merged.RulesFileBase, merged.Config.RulesFile)
	if err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}
	cfg := merged.Config

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
	// 与生产一致地跑一次 Claude 导入；临时 HOME 与项目里都没有 .claude，
	// 所以结果是 0 个成员，不会给断言引入额外变量。
	claude := importClaudeAgents(cfg, project)

	srv := NewServer(st, "test-version", modelsFromConfig(cfg))
	srv.EnableWrite(WriteDeps{
		Cfg:                         cfg,
		Svc:                         svc,
		Rn:                          rn,
		DataDir:                     dataDir,
		ConfigPath:                  layers[0].Path,
		Layers:                      layers,
		Cwd:                         project,
		ClaudeAgents:                claude.agents,
		ClaudeAgentsMerged:          claude.merged,
		ClaudeAgentsWithoutProvider: claude.withoutProvider,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &gui6Env{
		writeTestEnv:  &writeTestEnv{store: st, srv: srv, http: ts},
		projectDir:    project,
		projectConfig: projectPath,
		globalConfig:  globalPath,
		home:          home,
		dataDir:       dataDir,
		layers:        layers,
	}
}

func gui6Read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", path, err)
	}
	return string(data)
}

// gui6Post 发一次保存 / 删除请求，返回状态码与响应体。
func gui6Post(t *testing.T, url, body string) (int, []byte) {
	t.Helper()
	status, _, raw := postRaw(t, url, "application/json", body)
	return status, raw
}

// gui6Save 保存一个 agent 并断言 200，返回响应。
func gui6Save(t *testing.T, env *gui6Env, payload map[string]any) settingsWriteResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("编码请求失败: %v", err)
	}
	status, raw := gui6Post(t, env.http.URL+"/api/settings/agents", string(body))
	if status != http.StatusOK {
		t.Fatalf("保存 agent = %d，期望 200；body=%s", status, raw)
	}
	var res settingsWriteResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("保存响应不是合法 JSON: %v；%s", err, raw)
	}
	return res
}

// gui6Settings 读一次设置快照。
func gui6Settings(t *testing.T, env *gui6Env) settingsResponse {
	t.Helper()
	var resp settingsResponse
	getJSON(t, env.http.URL+"/api/settings", http.StatusOK, &resp)
	return resp
}

// gui6OptionsAgents 读 /api/options 的成员表。
func gui6OptionsAgents(t *testing.T, env *gui6Env) []agentOption {
	t.Helper()
	var resp optionsResponse
	getJSON(t, env.http.URL+"/api/options", http.StatusOK, &resp)
	return resp.Agents
}

func gui6AgentByID(t *testing.T, agents []settingsAgentJSON, id string) settingsAgentJSON {
	t.Helper()
	for _, agent := range agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("设置响应里没有 agent %q: %+v", id, agents)
	return settingsAgentJSON{}
}

func gui6OptionByID(t *testing.T, agents []agentOption, id string) agentOption {
	t.Helper()
	for _, agent := range agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("/api/options 里没有 agent %q: %+v", id, agents)
	return agentOption{}
}

func gui6Backups(t *testing.T, target string) []string {
	t.Helper()
	paths, err := filepath.Glob(target + ".bak-*")
	if err != nil {
		t.Fatalf("查找备份失败: %v", err)
	}
	sort.Strings(paths)
	return paths
}

func gui6DirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录 %s 失败: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// gui6AssertNoWriteArtefacts 断言目标目录里没有多出备份或临时文件。
func gui6AssertNoWriteArtefacts(t *testing.T, env *gui6Env, before []string) {
	t.Helper()
	if got := gui6DirNames(t, filepath.Dir(env.projectConfig)); !gui6SameStrings(got, before) {
		t.Errorf("配置目录里的文件变了: 之前 %v，现在 %v", before, got)
	}
}

func gui6SameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestGui6SaveAgentEditsOnlyTargetFile 是本片的核心断言：保存只改目标文件
// 里这个 agent 的段，全局层逐字节不变；新增与「改写继承来的成员」同样
// 只落在项目层。
func TestGui6SaveAgentEditsOnlyTargetFile(t *testing.T) {
	env := newGui6Env(t)
	globalBefore := gui6Read(t, env.globalConfig)
	projectBefore := gui6Read(t, env.projectConfig)
	p2Block := "[agents.p2]\ndisplay_name = \"Project Two\"\nprovider = \"claude\"\nmodel = \"opus\"\nrole = \"待删除成员\"\npermissions = \"read\"\nmode = \"worker\"\n"
	if !strings.Contains(projectBefore, p2Block) {
		t.Fatalf("测试配置里找不到 p2 段:\n%s", projectBefore)
	}

	t.Run("改项目层已有成员", func(t *testing.T) {
		res := gui6Save(t, env, map[string]any{
			"agent_id":     "p1",
			"display_name": "Project One 改过",
			"provider":     "claude",
			"model":        "opus-5.5",
			"effort":       "max",
			"permissions":  "read",
			"mode":         "discussion",
		})
		if res.WriteTarget != env.projectConfig {
			t.Errorf("write_target = %q，期望项目层 %q", res.WriteTarget, env.projectConfig)
		}
		if res.BackupPath == "" {
			t.Errorf("备份路径为空")
		}
		// A3：这个成员没有历史 provider 会话，但 display_name / model / effort
		// 都是身份字段——保存当场热换（reloaded=true），会话重置数为 0。
		if !res.Reloaded {
			t.Errorf("reloaded = false，期望 true（身份字段热换，Runner 已换新配置）")
		}
		if res.SessionsReset != 0 {
			t.Errorf("sessions_reset = %d，期望 0（这个成员还没开过口）", res.SessionsReset)
		}
		if !strings.Contains(res.Note, "立即生效") || !strings.Contains(res.Note, "无需重启") {
			t.Errorf("note = %q，期望说明已经生效且无需重启", res.Note)
		}
		if got := env.srv.runCfg().Agents["p1"].Model; got != "opus-5.5" {
			t.Errorf("RunCfg 里 p1 的 model = %q，期望热换后的 opus-5.5", got)
		}

		if got := gui6Read(t, env.globalConfig); got != globalBefore {
			t.Errorf("全局层配置文件被改动了:\n%s", got)
		}
		projectAfter := gui6Read(t, env.projectConfig)
		for _, want := range []string{
			"display_name = \"Project One 改过\"\n",
			"model = \"opus-5.5\"\n",
			"effort = \"max\"\n",
			"role = \"项目层成员\"\n",
		} {
			if !strings.Contains(projectAfter, want) {
				t.Errorf("项目层里缺少 %q:\n%s", want, projectAfter)
			}
		}
		if strings.Contains(projectAfter, "model = \"sonnet\"") {
			t.Errorf("旧模型名还在项目层里:\n%s", projectAfter)
		}
		if !strings.Contains(projectAfter, p2Block) {
			t.Errorf("p2 段被改动了:\n%s", projectAfter)
		}
		if strings.Count(projectAfter, "[agents.p1]") != 1 {
			t.Errorf("p1 段出现了多次:\n%s", projectAfter)
		}
		// 项目层里本来没有 data_dir：全局层的那一行不能被搬过来。
		if strings.Contains(projectAfter, "data_dir") {
			t.Errorf("项目层被写进了 data_dir:\n%s", projectAfter)
		}
	})

	t.Run("新增成员只追加到项目层", func(t *testing.T) {
		gui6Save(t, env, map[string]any{
			"agent_id":     "p3",
			"display_name": "Project Three",
			"provider":     "claude",
			"model":        "grok-4",
			"effort":       "",
			"permissions":  "workspace",
			"mode":         "worker",
		})
		if got := gui6Read(t, env.globalConfig); got != globalBefore {
			t.Errorf("新增成员改动了全局层:\n%s", got)
		}
		projectAfter := gui6Read(t, env.projectConfig)
		if !strings.Contains(projectAfter, "[agents.p3]\n") ||
			!strings.Contains(projectAfter, "permissions = \"workspace\"\n") ||
			!strings.Contains(projectAfter, "mode = \"worker\"\n") {
			t.Errorf("项目层里没有完整的新成员:\n%s", projectAfter)
		}
		if !strings.Contains(projectAfter, p2Block) {
			t.Errorf("新增成员改动了 p2 段:\n%s", projectAfter)
		}
	})

	t.Run("改写继承来的成员也只动项目层", func(t *testing.T) {
		gui6Save(t, env, map[string]any{
			"agent_id":     "g1",
			"display_name": "Global One 改过",
			"provider":     "claude",
			"model":        "opus-5.5",
			"effort":       "",
			"permissions":  "read",
			"mode":         "discussion",
		})
		if got := gui6Read(t, env.globalConfig); got != globalBefore {
			t.Errorf("改写继承成员时全局层被改动了:\n%s", got)
		}
		projectAfter := gui6Read(t, env.projectConfig)
		if !strings.Contains(projectAfter, "[agents.g1]\n") {
			t.Fatalf("项目层里没有 g1 的覆盖段:\n%s", projectAfter)
		}
		// 整条覆盖会丢掉继承来的 role，所以覆盖段必须把 role 带过来。
		if !strings.Contains(projectAfter, "role = \"全局层成员\"\n") {
			t.Errorf("g1 的覆盖段丢了继承来的 role:\n%s", projectAfter)
		}
		settings := gui6Settings(t, env)
		if got := gui6AgentByID(t, settings.Agents, "g1"); got.Model != "opus-5.5" {
			t.Errorf("合并配置里 g1 的 model = %q，期望 opus-5.5", got.Model)
		}
	})
}

// TestGui6SaveAgentWritesBackup 钉住备份：文件存在、内容等于写前原文、
// 名字符合 <文件名>.bak-<YYYYMMDD-HHmmss>，且响应里给出的就是那个路径。
func TestGui6SaveAgentWritesBackup(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	if got := gui6Backups(t, env.projectConfig); len(got) != 0 {
		t.Fatalf("开始前就有备份: %v", got)
	}

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "opus-5.5",
		"effort":       "",
		"permissions":  "read",
		"mode":         "discussion",
	})
	backups := gui6Backups(t, env.projectConfig)
	if len(backups) != 1 {
		t.Fatalf("备份文件 = %v，期望恰好一个", backups)
	}
	if res.BackupPath != backups[0] {
		t.Errorf("响应的 backup_path = %q，磁盘上是 %q", res.BackupPath, backups[0])
	}
	if got := gui6Read(t, res.BackupPath); got != before {
		t.Errorf("备份内容与写前原文不一致:\n备份:\n%s\n原文:\n%s", got, before)
	}
	base := filepath.Base(res.BackupPath)
	if !regexp.MustCompile(`^config\.toml\.bak-\d{8}-\d{6}(-\d+)?$`).MatchString(base) {
		t.Errorf("备份文件名 = %q，期望 config.toml.bak-<YYYYMMDD-HHmmss>", base)
	}
	if got := gui6Read(t, env.projectConfig); got == before {
		t.Errorf("保存之后目标文件没有变化")
	}
}

// TestGui6ValidationFailureLeavesFileUntouched 钉住三条校验失败路径：
// 未知 provider、空 model、非法 permission。每一条都必须回 400、目标文件
// 与另一层逐字节不变、配置目录里不多出任何备份或临时文件。
func TestGui6ValidationFailureLeavesFileUntouched(t *testing.T) {
	env := newGui6Env(t)
	projectBefore := gui6Read(t, env.projectConfig)
	globalBefore := gui6Read(t, env.globalConfig)
	dirBefore := gui6DirNames(t, filepath.Dir(env.projectConfig))

	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{
			name: "未知 provider",
			payload: map[string]any{
				"agent_id": "p1", "display_name": "P1", "provider": "nope",
				"model": "opus", "permissions": "read", "mode": "discussion",
			},
			want: "undefined provider",
		},
		{
			name: "空 model",
			payload: map[string]any{
				"agent_id": "p1", "display_name": "P1", "provider": "claude",
				"model": "", "permissions": "read", "mode": "discussion",
			},
			want: "must not be empty",
		},
		{
			name: "非法 permission",
			payload: map[string]any{
				"agent_id": "p1", "display_name": "P1", "provider": "claude",
				"model": "opus", "permissions": "root", "mode": "discussion",
			},
			want: "invalid permissions",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatalf("编码请求失败: %v", err)
			}
			status, raw := gui6Post(t, env.http.URL+"/api/settings/agents", string(body))
			if status != http.StatusBadRequest {
				t.Fatalf("状态码 = %d，期望 400；body=%s", status, raw)
			}
			if !strings.Contains(string(raw), tc.want) {
				t.Errorf("400 的说明 = %s，期望包含 %q", raw, tc.want)
			}
			if got := gui6Read(t, env.projectConfig); got != projectBefore {
				t.Errorf("校验失败之后项目层被改动了")
			}
			if got := gui6Read(t, env.globalConfig); got != globalBefore {
				t.Errorf("校验失败之后全局层被改动了")
			}
			gui6AssertNoWriteArtefacts(t, env, dirBefore)
		})
	}
}

// TestGui6DeleteAgentRemovesOnlyItsSection 钉住删除的三种结果：
// 目标层里的段被移除且其它段保留；不存在的 id 回 404；继承来的成员回 409
// （它不在写目标里，本接口删不掉）。
func TestGui6DeleteAgentRemovesOnlyItsSection(t *testing.T) {
	env := newGui6Env(t)
	globalBefore := gui6Read(t, env.globalConfig)
	projectBefore := gui6Read(t, env.projectConfig)
	p1Block := "[agents.p1]\ndisplay_name = \"Project One\"\nprovider = \"claude\"\nmodel = \"sonnet\"\nrole = \"项目层成员\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	if !strings.Contains(projectBefore, p1Block) {
		t.Fatalf("测试配置里找不到 p1 段:\n%s", projectBefore)
	}

	status, raw := gui6Post(t, env.http.URL+"/api/settings/agents/p2/delete", `{}`)
	if status != http.StatusOK {
		t.Fatalf("删除 p2 = %d，期望 200；body=%s", status, raw)
	}
	var res settingsWriteResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("删除响应不是合法 JSON: %v；%s", err, raw)
	}
	if res.BackupPath == "" || res.WriteTarget != env.projectConfig {
		t.Errorf("删除响应 = %+v，期望带备份路径与项目层目标", res)
	}
	if got := gui6Read(t, res.BackupPath); got != projectBefore {
		t.Errorf("删除前的备份内容与原文不一致")
	}
	if got := gui6Read(t, env.globalConfig); got != globalBefore {
		t.Errorf("删除改动了全局层:\n%s", got)
	}
	projectAfter := gui6Read(t, env.projectConfig)
	if strings.Contains(projectAfter, "[agents.p2]") || strings.Contains(projectAfter, "待删除成员") {
		t.Errorf("p2 段没删干净:\n%s", projectAfter)
	}
	if !strings.Contains(projectAfter, p1Block) {
		t.Errorf("删除 p2 时改动了 p1 段:\n%s", projectAfter)
	}
	settings := gui6Settings(t, env)
	for _, agent := range settings.Agents {
		if agent.ID == "p2" {
			t.Errorf("删除之后 /api/settings 里还有 p2")
		}
	}

	// 删不存在的 id：404，文件不动。
	beforeGhost := gui6Read(t, env.projectConfig)
	status, raw = gui6Post(t, env.http.URL+"/api/settings/agents/ghost/delete", `{}`)
	if status != http.StatusNotFound {
		t.Fatalf("删除不存在的 agent = %d，期望 404；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "agent 不存在") {
		t.Errorf("404 的说明 = %s，期望点名「agent 不存在」", raw)
	}
	if got := gui6Read(t, env.projectConfig); got != beforeGhost {
		t.Errorf("删不存在的 agent 改动了文件")
	}

	// 删继承来的 g1：409，说明它定义在另一份文件里，文件不动。
	beforeInherited := gui6Read(t, env.projectConfig)
	status, raw = gui6Post(t, env.http.URL+"/api/settings/agents/g1/delete", `{}`)
	if status != http.StatusConflict {
		t.Fatalf("删除继承成员 = %d，期望 409；body=%s", status, raw)
	}
	var conflict struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &conflict); err != nil {
		t.Fatalf("409 响应不是合法 JSON: %v；%s", err, raw)
	}
	// 错误体是 JSON，Windows 路径里的反斜杠会被转义；断言解码之后的文案。
	if !strings.Contains(conflict.Error, "定义在") || !strings.Contains(conflict.Error, env.globalConfig) {
		t.Errorf("409 的说明 = %q，期望点名全局层路径 %q", conflict.Error, env.globalConfig)
	}
	if got := gui6Read(t, env.projectConfig); got != beforeInherited {
		t.Errorf("删继承成员改动了项目层")
	}
	if got := gui6Read(t, env.globalConfig); got != globalBefore {
		t.Errorf("删继承成员改动了全局层")
	}
}

// TestGui6SettingsAndOptionsReflectWrite 钉住「写完之后展示层立刻更新、
// 执行层按分级跟着走」：/api/settings 与 /api/options 反映新配置；已有成员
// 的身份字段改动当场热换（RunCfg 也换成新值）；设置页新增的成员在重启前
// 只在展示配置里，/api/ask 不接受它。
func TestGui6SettingsAndOptionsReflectWrite(t *testing.T) {
	env := newGui6Env(t)

	settings := gui6Settings(t, env)
	if len(settings.Layers) != 2 {
		t.Fatalf("layers = %+v，期望两层", settings.Layers)
	}
	if settings.Layers[0].Path != env.projectConfig || !settings.Layers[0].Project || !settings.Layers[0].Exists {
		t.Errorf("第一层 = %+v，期望存在的项目层 %s", settings.Layers[0], env.projectConfig)
	}
	if settings.Layers[1].Path != env.globalConfig || settings.Layers[1].Project || !settings.Layers[1].Exists {
		t.Errorf("第二层 = %+v，期望存在的全局层 %s", settings.Layers[1], env.globalConfig)
	}
	if settings.WriteTarget != env.projectConfig {
		t.Errorf("write_target = %q，期望 %q", settings.WriteTarget, env.projectConfig)
	}
	if settings.ConfigPath != env.projectConfig {
		t.Errorf("config_path = %q，期望 %q", settings.ConfigPath, env.projectConfig)
	}
	if settings.DataDir != env.dataDir {
		t.Errorf("data_dir = %q，期望 %q", settings.DataDir, env.dataDir)
	}
	if settings.SummaryAgent != "g1" {
		t.Errorf("summary_agent = %q，期望 g1", settings.SummaryAgent)
	}
	if len(settings.Teams) != 1 || settings.Teams[0].ID != "t1" ||
		len(settings.Teams[0].Members) != 1 || settings.Teams[0].Members[0] != "g1" {
		t.Errorf("teams = %+v，期望 t1 -> [g1]", settings.Teams)
	}
	var hasClaudeProvider bool
	for _, provider := range settings.Providers {
		if provider.ID == "claude" && provider.Command != "" {
			hasClaudeProvider = true
		}
	}
	if !hasClaudeProvider {
		t.Errorf("providers 里没有 claude: %+v", settings.Providers)
	}
	for _, id := range []string{"p1", "p2", "g1"} {
		if got := gui6AgentByID(t, settings.Agents, id); got.Source != agentSourceConfig {
			t.Errorf("%s 的 source = %q，期望 %q", id, got.Source, agentSourceConfig)
		}
	}
	if got := gui6AgentByID(t, settings.Agents, "p1"); got.Model != "sonnet" {
		t.Fatalf("保存前 p1 的 model = %q，期望 sonnet", got.Model)
	}
	if got := gui6OptionByID(t, gui6OptionsAgents(t, env), "p1"); got.Model != "sonnet" {
		t.Fatalf("保存前 /api/options 里 p1 的 model = %q，期望 sonnet", got.Model)
	}

	gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "opus-5.5",
		"effort":       "",
		"permissions":  "read",
		"mode":         "discussion",
	})
	settings = gui6Settings(t, env)
	if got := gui6AgentByID(t, settings.Agents, "p1"); got.Model != "opus-5.5" {
		t.Errorf("保存后 /api/settings 里 p1 的 model = %q，期望 opus-5.5", got.Model)
	}
	if got := gui6OptionByID(t, gui6OptionsAgents(t, env), "p1"); got.Model != "opus-5.5" {
		t.Errorf("保存后 /api/options 里 p1 的 model = %q，期望 opus-5.5", got.Model)
	}
	// A3：model 是身份字段，保存当场热换——Runner 手里那份也换了。
	// 代价是该成员的 provider 会话被重置（这个成员还没开过口，所以是 0 条）。
	if got := env.srv.runCfg().Agents["p1"].Model; got != "opus-5.5" {
		t.Errorf("RunCfg 里 p1 的 model = %q，期望热换后的 opus-5.5", got)
	}
	if got := env.srv.currentCfg().Agents["p1"].Model; got != "opus-5.5" {
		t.Errorf("展示配置里 p1 的 model = %q，期望 opus-5.5", got)
	}

	// 新增的成员只出现在展示配置里；/api/ask 仍按启动配置校验，必须 400，
	// 不能出现「选项里能选、Runner 不认识」的静默失败。
	gui6Save(t, env, map[string]any{
		"agent_id":     "p9",
		"display_name": "Project Nine",
		"provider":     "claude",
		"model":        "grok-4",
		"effort":       "",
		"permissions":  "read",
		"mode":         "discussion",
	})
	if got := gui6OptionByID(t, gui6OptionsAgents(t, env), "p9"); got.Source != agentSourceConfig {
		t.Errorf("新增 p9 的 source = %q，期望 %q", got.Source, agentSourceConfig)
	}
	status, raw := gui6Post(t, env.http.URL+"/api/ask", `{"prompt":"hi","members":["p9"]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("/api/ask 用新成员 = %d，期望 400（Runner 不重建）；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "p9") {
		t.Errorf("400 的说明 = %s，期望点名 p9", raw)
	}

	// 删除 p2 之后两个展示接口都不再列出它。
	status, raw = gui6Post(t, env.http.URL+"/api/settings/agents/p2/delete", `{}`)
	if status != http.StatusOK {
		t.Fatalf("删除 p2 = %d，期望 200；body=%s", status, raw)
	}
	for _, agent := range gui6Settings(t, env).Agents {
		if agent.ID == "p2" {
			t.Errorf("/api/settings 里仍有已删除的 p2")
		}
	}
	for _, agent := range gui6OptionsAgents(t, env) {
		if agent.ID == "p2" {
			t.Errorf("/api/options 里仍有已删除的 p2")
		}
	}
}

// TestGui6ReadOnlySettingsIs405 钉住共同要求：不带 -rw 时三个设置端点都由
// s.write == nil 挡在门外，回 405 并说明怎么开写模式。
func TestGui6ReadOnlySettingsIs405(t *testing.T) {
	env := newServeTestEnv(t, nil)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		allow  string
	}{
		{"读取设置", http.MethodGet, "/api/settings", "", http.MethodGet},
		{"保存 agent", http.MethodPost, "/api/settings/agents", `{"agent_id":"x"}`, http.MethodPost},
		{"删除 agent", http.MethodPost, "/api/settings/agents/x/delete", `{}`, http.MethodPost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				status int
				ctype  string
				raw    []byte
			)
			if tc.method == http.MethodGet {
				res, err := http.Get(env.http.URL + tc.path)
				if err != nil {
					t.Fatalf("GET %s 失败: %v", tc.path, err)
				}
				defer res.Body.Close()
				status = res.StatusCode
				ctype = res.Header.Get("Content-Type")
				if res.Header.Get("Allow") != tc.allow {
					t.Errorf("Allow = %q，期望 %q", res.Header.Get("Allow"), tc.allow)
				}
				raw, err = io.ReadAll(res.Body)
				if err != nil {
					t.Fatalf("读 GET 响应失败: %v", err)
				}
			} else {
				status, ctype, raw = postRaw(t, env.http.URL+tc.path, "application/json", tc.body)
			}
			if status != http.StatusMethodNotAllowed {
				t.Fatalf("状态码 = %d，期望 405；body=%s", status, raw)
			}
			if ctype != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", ctype)
			}
			message := string(raw)
			if !strings.Contains(message, "只读") || !strings.Contains(message, "-rw") {
				t.Errorf("405 的说明 = %s，期望说清只读模式并提示 -rw", message)
			}
		})
	}
}

// TestGui6SettingsWriteGuard 复用 /api/ask 的同一套写闸：非 JSON → 415，
// 跨站 → 403；被拒的请求一个字节都不许写进配置文件。
func TestGui6SettingsWriteGuard(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	url := env.http.URL + "/api/settings/agents"

	if status, _, raw := postRaw(t, url, "text/plain", `{}`); status != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain = %d，期望 415；body=%s", status, raw)
	}
	if status, _, raw := postRaw(t, url, "", `{}`); status != http.StatusUnsupportedMediaType {
		t.Errorf("无 Content-Type = %d，期望 415；body=%s", status, raw)
	}
	body := `{"agent_id":"p1","display_name":"P1","provider":"claude","model":"opus","permissions":"read","mode":"discussion"}`
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("发请求失败: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("跨站 Origin = %d，期望 403；body=%s", res.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "跨站") {
		t.Errorf("403 的说明没有提到跨站: %s", raw)
	}
	if got := gui6Read(t, env.projectConfig); got != before {
		t.Errorf("被写闸拒绝的请求改动了配置文件")
	}
}

// TestGui6DeleteOverrideRevealsLowerLayer 钉住分层语义：项目层删掉的只是
// 「覆盖段」，低优先级层里还有同名定义时合并后它仍然可见。响应必须把这件事
// 说清楚，否则用户会以为删除没生效。
func TestGui6DeleteOverrideRevealsLowerLayer(t *testing.T) {
	env := newGui6Env(t)
	global := gui6Read(t, env.globalConfig)
	global += "\n[agents.p1]\ndisplay_name = \"Global P1\"\nprovider = \"claude\"\nmodel = \"opus\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	if err := os.WriteFile(env.globalConfig, []byte(global), 0o644); err != nil {
		t.Fatalf("追加全局层 p1 失败: %v", err)
	}

	status, raw := gui6Post(t, env.http.URL+"/api/settings/agents/p1/delete", `{}`)
	if status != http.StatusOK {
		t.Fatalf("删除项目层覆盖段 = %d，期望 200；body=%s", status, raw)
	}
	var res settingsWriteResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("删除响应不是合法 JSON: %v；%s", err, raw)
	}
	if !strings.Contains(res.Note, "仍然可见") {
		t.Errorf("note = %q，期望说明低优先级层仍有同名定义", res.Note)
	}
	if got := gui6Read(t, env.projectConfig); strings.Contains(got, "[agents.p1]") {
		t.Errorf("项目层里仍有 p1 段:\n%s", got)
	}
	settings := gui6Settings(t, env)
	if got := gui6AgentByID(t, settings.Agents, "p1"); got.Model != "opus" {
		t.Errorf("回落到全局层之后 p1 的 model = %q，期望 opus", got.Model)
	}
}

// TestGui6IndexSettingsMarkup 钉 index.html 里的结构锚点与三条界面规则：
// 预览确认框写着「将写入 <路径>」、成功提示显示备份路径与 note、
// 页面里没有 innerHTML。真正的点击流程由 CDP 验收覆盖。
func TestGui6IndexSettingsMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 入口与三个面板。
		`id="open-settings"`,
		`id="settings-overlay"`,
		`id="settings-layers"`,
		`id="settings-write-target"`,
		`id="settings-agents-body"`,
		`id="settings-teams"`,
		`id="settings-form-overlay"`,
		`id="settings-preview"`,
		`id="settings-preview-target"`,
		`id="settings-preview-diff-body"`,
		`id="settings-result"`,
		// 设置接口与写盘动作。provider 与 agent 共用同一段提交逻辑：
		// 端点由 sectionPath 推出来（providers / agents），删除再接 /delete。
		`api('/api/settings')`,
		`const sectionPath = isProvider ? 'providers' : 'agents'`,
		`'/api/settings/' + sectionPath`,
		`'/api/settings/' + sectionPath + '/' + encodeURIComponent(preview.id) + '/delete'`,
		`await postJSON(url, isDelete ? {} : preview.payload)`,
		// 提供方页可以增删改（0.4.0）。
		`id="settings-provider-add"`,
		`id="settings-provider-overlay"`,
		`function renderSettingsProviders()`,
		`function openProviderForm(`,
		`function previewProviderSave()`,
		`function openProviderDelete(`,
		// 预览文案：目标路径、写前备份、字段新旧对比。
		`t('ui.settings.previewWriteTarget',`,
		`t('ui.settings.previewBackup'`,
		`change.old !== change.new`,
		// 成功提示：备份路径 + 服务端 note（含「重启后生效」）。
		`t('ui.settings.saveBackup', '备份：{path}', { path: result.backup_path })`,
		`lines.push(result.note)`,
		// 只读模式隐藏入口；claude 来源的成员只读。
		`openSettingsButton.hidden = !state.writable`,
		`button.disabled = readOnly`,
		// 刷新之后 /api/options 立刻重读，工作台成员列表与新配置一致。
		`await loadOptions()`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
	if strings.Contains(html, "innerHTML") {
		t.Errorf("index.html 里出现了 innerHTML，前端必须保持 0 处")
	}
}
