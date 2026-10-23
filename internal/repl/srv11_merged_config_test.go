package repl

// srv11_merged_config_test.go 覆盖 SRV11 §B：合并配置模式下 /config reload 与
// /add 不再用单文件 config.Load 当判据。
//
// 两条用例都从真实的两层布局出发：项目层 .council/config.toml 只写 [agents.*]，
// 全局层 ~/.council/config.toml 写 data_dir 与 [providers.claude]。cwd 与
// HOME/USERPROFILE 都临时改到 t.TempDir() 下，ResolveConfigLayers("") 才会
// 真的求出项目层 + 全局层——不注入假层，因为要证明的正是「真实求层 + LoadMerged」
// 这套接法在 repl 里成立。
//
// 【判别力】两条用例在改动前都是红的：
//   - reload 用单文件 config.Load 读项目层，缺 provider，Validate 失败，
//     打「配置非法」而不是「已应用新配置」；
//   - /add 的守卫用单文件 config.Load 与内存里的合并结果比，永远不等，
//     打「配置文件在上次加载之后被改过」把一次合法追加拒掉。

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/store"
	"council/internal/turn"
)

// srv11Rig 是 §B 两条用例共用的装置：真 SQLite、真配置文件、真 Runner，
// 外加一个已经 /use 过的 REPL。
type srv11Rig struct {
	dir         string
	projectDir  string
	projectPath string
	globalPath  string
	out         *bytes.Buffer
	r           *REPL
	convID      string
	ctx         context.Context
}

// srv11NewRig 造出「项目层 + 全局层」并装配 REPL，装配方式与 app.Run 一致：
// 同一个 *config.Config 指针交给 turn.NewRunner 与 repl.New，RulesContext
// 只填启动时生效的最高优先层路径。
func srv11NewRig(t *testing.T) *srv11Rig {
	t.Helper()
	isolateGlobalRules(t)

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(home, ".council"), 0o755); err != nil {
		t.Fatalf("建临时 home 失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".council"), 0o755); err != nil {
		t.Fatalf("建临时项目目录失败: %v", err)
	}

	// 识别用的假执行器按 Path 查表，claude 那一行登记的是 os.Args[0]（与
	// mdx 装置同一套夹具）；这里只借用它的夹具，不真起进程。
	exe := os.Args[0]
	if strings.ContainsAny(exe, "'\n\r") {
		t.Fatalf("测试可执行文件路径里含引号或换行，不能写进 TOML 字面串: %q", exe)
	}
	globalPath := filepath.Join(home, ".council", "config.toml")
	if err := os.WriteFile(globalPath, []byte(`data_dir = '`+filepath.Join(dir, "data")+`'
store_thought = true

[providers.claude]
command = '`+exe+`'

[agents.glob_a]
provider = 'claude'
model = 'claude-global'
permissions = 'read'
`), 0o644); err != nil {
		t.Fatalf("写全局配置失败: %v", err)
	}
	// 项目层只有 [agents.*]：provider 来自全局层，这正是改动前会被误拒的写法。
	if err := os.WriteFile(filepath.Join(projectDir, ".council", "config.toml"), []byte(`[agents.proj_a]
provider = 'claude'
model = 'claude-project'
permissions = 'read'
`), 0o644); err != nil {
		t.Fatalf("写项目配置失败: %v", err)
	}

	// 求层与 app/serve 用的同一个函数：cwd 决定项目层，HOME/USERPROFILE 决定
	// 全局层。COUNCIL_CONFIG 必须清空，否则 ResolveConfigLayers 会把它当成
	// 「用户点名的一份文件」而不再合并。
	t.Chdir(projectDir)
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	layers, err := config.ResolveConfigLayers("")
	if err != nil {
		t.Fatalf("ResolveConfigLayers 失败: %v", err)
	}
	if len(layers) != 2 || !layers[0].Project || layers[1].Path != globalPath {
		t.Fatalf("前提：应当求出项目层 + 全局层两层，实际 %+v", layers)
	}
	if want := filepath.Join(projectDir, ".council", "config.toml"); layers[0].Path != want {
		t.Fatalf("前提：项目层路径 = %q，期望 %q", layers[0].Path, want)
	}
	merged, err := config.LoadMerged(layers)
	if err != nil {
		t.Fatalf("前提：两层配置应当合并成功，实际 %v", err)
	}
	if _, ok := merged.Config.Agents["glob_a"]; !ok {
		t.Fatalf("前提：合并结果里应当有全局 agent glob_a，实际 %+v", merged.Config.Agents)
	}

	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(dir, "data"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := conversation.NewService(st)
	conv, err := svc.Create(ctx, "srv11 合并配置")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	rn := turn.NewRunner(merged.Config, svc, filepath.Join(dir, "workdir"), turn.RulesBlocks{})

	out := &bytes.Buffer{}
	r := New(merged.Config, svc, rn, strings.NewReader(""), out, RulesContext{ConfigPath: layers[0].Path})
	ex := mdxNewExec()
	r.discover = ex.run
	r.discoverTimeout = 200 * time.Millisecond // 与 mdx 装置同值；识别是假的，不会真等

	rig := &srv11Rig{
		dir:         dir,
		projectDir:  projectDir,
		projectPath: layers[0].Path,
		globalPath:  globalPath,
		out:         out,
		r:           r,
		convID:      conv.ID,
		ctx:         ctx,
	}
	out.Reset()
	if err := r.Submit(ctx, "/use "+conv.ID); err != nil {
		t.Fatalf("/use 失败: %v", err)
	}
	out.Reset()
	return rig
}

// run 喂一行命令，返回这一行的输出增量（按行），与 mdx 装置同一套切法。
func (g *srv11Rig) run(t *testing.T, line string) []string {
	t.Helper()
	before := g.out.Len()
	if err := g.r.Submit(g.ctx, line); err != nil {
		t.Fatalf("%q 失败: %v", line, err)
	}
	return mdxLines(g.out.String()[before:])
}

// TestSrv11ConfigReloadMergesProjectAndGlobalLayers 是 §B 第 1 条：项目层只有
// [agents.proj_a]、全局层有 [providers.claude] 与 [agents.glob_a] 时，
// /config reload --apply 必须成功，而且 reload 之后 /agents 仍列得出全局 agent。
//
// 断言分三层：屏幕（成功/没有误报非法）、/agents 表格（用户可见的成员列表）、
// 内存（r.cfg 与 r.cfgLayers / r.rulesFileBase 都换成了合并结果）。
func TestSrv11ConfigReloadMergesProjectAndGlobalLayers(t *testing.T) {
	g := srv11NewRig(t)

	reload := g.run(t, "/config reload --apply")
	if !mdxHasLine(reload, mdxAppliedLine) {
		t.Fatalf("reload 应当成功并打 %q，实际输出:\n%s", mdxAppliedLine, strings.Join(reload, "\n"))
	}
	joined := strings.Join(reload, "\n")
	for _, forbidden := range []string{"配置非法", "重读配置文件失败", "data_dir 与 rules_file 只能重启生效"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("合并模式下 reload 不该出现 %q；完整输出:\n%s", forbidden, joined)
		}
	}
	// 任务书要求诊断文案带上是哪几层合并的（详细档）。
	layerLine := ""
	for _, line := range reload {
		if strings.HasPrefix(line, "配置来自 2 层") {
			layerLine = line
			break
		}
	}
	if layerLine == "" {
		t.Fatalf("详细档应当有一行「配置来自 2 层…」；完整输出:\n%s", joined)
	}
	for _, path := range []string{g.projectPath, g.globalPath} {
		if !strings.Contains(layerLine, path) {
			t.Errorf("层诊断那一行应当点名 %q，实际 %q", path, layerLine)
		}
	}

	// 用户可见的成员列表：/agents 的表头之后必须同时有项目层与全局层的 agent。
	agents := strings.Join(g.run(t, "/agents"), "\n")
	headerIdx := strings.Index(agents, "PERMISSIONS")
	if headerIdx < 0 {
		t.Fatalf("/agents 没有打出表头，完整输出:\n%s", agents)
	}
	table := agents[headerIdx:]
	for _, id := range []string{"proj_a", "glob_a"} {
		if !strings.Contains(table, id) {
			t.Errorf("reload 之后 /agents 仍应列出 %q；表头之后:\n%s", id, table)
		}
	}

	// 内存：配置换成合并后的那一份，层与 rules_file 基准一起换。
	if _, ok := g.r.cfg.Agents["glob_a"]; !ok {
		t.Errorf("reload 之后内存配置里应当仍有全局 agent glob_a，实际 %+v", g.r.cfg.Agents)
	}
	if _, ok := g.r.cfg.Providers["claude"]; !ok {
		t.Errorf("reload 之后内存配置里应当仍有全局 provider claude，实际 %+v", g.r.cfg.Providers)
	}
	wantLayers := []string{g.projectPath, g.globalPath}
	if !reflect.DeepEqual(g.r.cfgLayers, wantLayers) {
		t.Errorf("r.cfgLayers = %q，期望 %q", g.r.cfgLayers, wantLayers)
	}
	if want := filepath.Dir(g.projectPath); g.r.rulesFileBase != want {
		t.Errorf("r.rulesFileBase = %q，期望 %q（没有任何一层写 rules_file 时取最高优先层目录）", g.r.rulesFileBase, want)
	}
}

// TestSrv11AddWritesOnlyTopLayer 是 §B 第 2 条：同样的两层布局下 /add 不再被
// 守卫误拒；写回只动项目层，全局文件一个字节都不变；内存里换成新的合并结果，
// 全局 agent / provider 不丢。
func TestSrv11AddWritesOnlyTopLayer(t *testing.T) {
	g := srv11NewRig(t)

	projectBefore, err := os.ReadFile(g.projectPath)
	if err != nil {
		t.Fatalf("读项目配置失败: %v", err)
	}
	globalBefore, err := os.ReadFile(g.globalPath)
	if err != nil {
		t.Fatalf("读全局配置失败: %v", err)
	}
	before := make(map[string]config.AgentConfig, len(g.r.cfg.Agents))
	for id, ac := range g.r.cfg.Agents {
		before[id] = ac
	}

	// 先拍识别快照（假执行器），再 /add +1。识别结果第一行是 claude 的
	// opus-5.5-1m-high（与 mdx 的 R11a 一样），它的 provider 只在全局层。
	list := g.run(t, "/add")
	if !mdxHasLine(list, "正在识别模型: claude") {
		t.Fatalf("/add 应当列出识别结果，实际输出:\n%s", strings.Join(list, "\n"))
	}
	lines := g.run(t, "/add +1")
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "配置文件在上次加载之后被改过") {
		t.Fatalf("合并模式下 /add 不该被守卫误拒；完整输出:\n%s", joined)
	}
	if !strings.Contains(joined, "已把 agent ") {
		t.Fatalf("/add +1 应当写入成功，实际输出:\n%s", joined)
	}

	// 新增的那一个 id：从前后两份 Agents 的差集里取，不猜 SuggestedID 的拼法。
	var added []string
	for id := range g.r.cfg.Agents {
		if _, ok := before[id]; !ok {
			added = append(added, id)
		}
	}
	if len(added) != 1 {
		t.Fatalf("/add 之后应当恰好新增一个 agent，实际 %v", added)
	}
	newID := added[0]

	projectAfter, err := os.ReadFile(g.projectPath)
	if err != nil {
		t.Fatalf("读项目配置失败: %v", err)
	}
	globalAfter, err := os.ReadFile(g.globalPath)
	if err != nil {
		t.Fatalf("读全局配置失败: %v", err)
	}
	// 写回只动最高优先层：项目文件是追加（原字节仍是前缀），全局文件逐字节不变。
	if !bytes.HasPrefix(projectAfter, projectBefore) {
		t.Errorf("项目配置应当只追加、不重写；详见文件:\n%s", projectAfter)
	}
	if !strings.Contains(string(projectAfter), newID) {
		t.Errorf("新 agent %q 应当写进项目配置，实际:\n%s", newID, projectAfter)
	}
	if !bytes.Equal(globalAfter, globalBefore) {
		t.Errorf("写回只能落最高优先层：全局配置被改动了\n之前:\n%s\n之后:\n%s", globalBefore, globalAfter)
	}
	// 合并结果绝不能被整体落盘：项目文件里不该出现只属于全局层的 provider 段。
	if strings.Contains(string(projectAfter), "[providers.claude]") {
		t.Errorf("项目配置里不该被抄进全局层的 provider；实际:\n%s", projectAfter)
	}

	// 内存里的新配置仍是合并结果：新 agent 与全局 agent / provider 同时在场。
	if _, ok := g.r.cfg.Agents["glob_a"]; !ok {
		t.Errorf("写入之后内存配置里应当仍有全局 agent glob_a，实际 %+v", g.r.cfg.Agents)
	}
	if _, ok := g.r.cfg.Providers["claude"]; !ok {
		t.Errorf("写入之后内存配置里应当仍有全局 provider claude，实际 %+v", g.r.cfg.Providers)
	}
	if got, want := g.r.cfg.Agents[newID].Provider, "claude"; got != want {
		t.Errorf("新 agent 的 provider = %q，期望 %q", got, want)
	}
	if wantLayers := []string{g.projectPath, g.globalPath}; !reflect.DeepEqual(g.r.cfgLayers, wantLayers) {
		t.Errorf("r.cfgLayers = %q，期望 %q", g.r.cfgLayers, wantLayers)
	}
}
