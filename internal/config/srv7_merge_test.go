package config

// srv7_merge_test.go 覆盖 SRV7 §5.1-§5.3：配置合并、data_dir 规则、解析顺序，
// 以及合并后校验错误标出来自哪一层。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// srv7Write 把内容写进 path（自动建父目录）。
func srv7Write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写文件 %s 失败: %v", path, err)
	}
}

// TestSrv7LoadMergedInheritsAndOverrides 是 §5.1：项目层只写 [agents.b]、
// [teams.over] 与 [summary] 时，全局的 providers 与没提到的条目全部继承；
// 同名条目由项目层整条覆盖。
//
// 同时钉住 Load 没有被改动：对项目文件单独 Load 不应看见全局层的东西。
func TestSrv7LoadMergedInheritsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	globalPath := filepath.Join(dir, "global.toml")
	projectPath := filepath.Join(dir, "project.toml")

	srv7Write(t, globalPath, `data_dir = '`+dataDir+`'
store_thought = false

[providers.claude]
command = "claude"

[providers.grok]
command = "grok"

[agents.a]
display_name = "A"
provider = "claude"
model = "opus"
permissions = "read"
mode = "discussion"
role = "全局 a"

[agents.b]
display_name = "B-global"
provider = "claude"
model = "opus"
permissions = "read"
mode = "discussion"
role = "全局 b"

[teams.keep]
display_name = "全局组"
members = ["a"]

[teams.over]
display_name = "全局组"
members = ["a"]

[summary]
agent = "a"
`)

	srv7Write(t, projectPath, `rules_file = "project-rules.md"

[agents.b]
display_name = "B-project"
provider = "grok"
model = "grok-4.7"
permissions = "read"
mode = "discussion"
role = "项目 b"
prompt = "项目里的系统提示"

[teams.over]
display_name = "项目组"
members = ["b"]

[summary]
agent = "b"
`)

	merged, err := LoadMerged([]Layer{
		{Path: projectPath, Project: true},
		{Path: globalPath},
	})
	if err != nil {
		t.Fatalf("合并配置失败: %v", err)
	}
	cfg := merged.Config

	// 全局写了 store_thought = false，项目没写：必须继承 false。
	// 若实现给每层先填了缺省 true 再按「项目层赢」覆盖，这里会错成 true。
	if cfg.StoreThought {
		t.Errorf("store_thought 应当继承全局层的 false，实际是 true")
	}
	if cfg.DataDir != dataDir {
		t.Errorf("data_dir = %q，期望继承全局层的 %q", cfg.DataDir, dataDir)
	}
	if cfg.RulesFile != "project-rules.md" {
		t.Errorf("rules_file = %q，期望项目层的 project-rules.md", cfg.RulesFile)
	}
	if merged.RulesFileBase != dir {
		t.Errorf("RulesFileBase = %q，期望定义 rules_file 的那一层目录 %q", merged.RulesFileBase, dir)
	}

	if len(cfg.Providers) != 2 {
		t.Errorf("providers 应当继承 claude 与 grok 两条，实际 %v", cfg.Providers)
	}
	for _, id := range []string{"claude", "grok"} {
		if _, ok := cfg.Providers[id]; !ok {
			t.Errorf("providers 里缺少继承来的 %q", id)
		}
	}

	if len(cfg.Agents) != 2 {
		t.Fatalf("agents 应当有 a（全局）与 b（项目）两条，实际 %v", cfg.Agents)
	}
	a := cfg.Agents["a"]
	if a.DisplayName != "A" || a.Provider != "claude" || a.Model != "opus" || a.Role != "全局 a" {
		t.Errorf("没被覆盖的 agent a 丢了全局层字段: %+v", a)
	}
	b := cfg.Agents["b"]
	if b.DisplayName != "B-project" || b.Provider != "grok" || b.Model != "grok-4.7" || b.Role != "项目 b" {
		t.Errorf("同名 agent b 应当整条被项目层覆盖，实际 %+v", b)
	}
	if b.Prompt != "项目里的系统提示" {
		t.Errorf("AgentConfig.Prompt 没从 TOML 读进来，实际 %q", b.Prompt)
	}
	if a.Prompt != "" {
		t.Errorf("a 没写 prompt，不该凭空多出内容: %q", a.Prompt)
	}

	if len(cfg.Teams) != 2 {
		t.Fatalf("teams 应当有 keep 与 over 两条，实际 %v", cfg.Teams)
	}
	if got := cfg.Teams["keep"].Members; len(got) != 1 || got[0] != "a" {
		t.Errorf("没被覆盖的 team keep 丢了全局层成员: %v", got)
	}
	if got := cfg.Teams["over"].Members; len(got) != 1 || got[0] != "b" {
		t.Errorf("同名 team over 应当整条被项目层覆盖，实际 %v", got)
	}
	if cfg.Summary.Agent != "b" {
		t.Errorf("summary.agent = %q，期望项目层覆盖成 b", cfg.Summary.Agent)
	}

	// §5.7：Load 仍然只读一份文件，不因为新增了 LoadMerged 就偷偷合并。
	projectOnly, err := Load(projectPath)
	if err != nil {
		t.Fatalf("Load(项目文件) 失败: %v", err)
	}
	if len(projectOnly.Agents) != 1 || projectOnly.Agents["b"].DisplayName != "B-project" {
		t.Errorf("Load 应当只看到项目文件里的 b，实际 %v", projectOnly.Agents)
	}
	if len(projectOnly.Providers) != 0 {
		t.Errorf("Load 不应当把全局层的 providers 合并进来，实际 %v", projectOnly.Providers)
	}
	if projectOnly.DataDir != "" {
		t.Errorf("Load 不应当把全局层的 data_dir 合并进来，实际 %q", projectOnly.DataDir)
	}
}

// TestSrv7LoadMergedIgnoresProjectDataDir 是 §5.2：项目层写 data_dir 也忽略，
// 取全局那份；只合并项目层时 data_dir 为空（交给 ResolveDataDir 回落）。
// 非法（相对）的项目 data_dir 同样只是忽略，不许把整份配置判死。
func TestSrv7LoadMergedIgnoresProjectDataDir(t *testing.T) {
	dir := t.TempDir()
	globalData := filepath.Join(dir, "global-data")
	projectData := filepath.Join(dir, "project-data")
	globalPath := filepath.Join(dir, "global.toml")
	projectPath := filepath.Join(dir, "project.toml")

	srv7Write(t, globalPath, `data_dir = '`+globalData+`'

[providers.claude]
command = "claude"

[agents.a]
provider = "claude"
model = "opus"
`)

	srv7Write(t, projectPath, `data_dir = '`+projectData+`'

[providers.claude]
command = "claude"

[agents.b]
provider = "claude"
model = "opus"
`)

	merged, err := LoadMerged([]Layer{
		{Path: projectPath, Project: true},
		{Path: globalPath},
	})
	if err != nil {
		t.Fatalf("合并配置失败: %v", err)
	}
	if merged.Config.DataDir != globalData {
		t.Errorf("data_dir = %q，期望忽略项目层后取全局的 %q", merged.Config.DataDir, globalData)
	}
	if len(merged.Notices) != 1 {
		t.Fatalf("应当恰好一条「项目 data_dir 被忽略」的提示，实际 %v", merged.Notices)
	}
	if !strings.Contains(merged.Notices[0], projectPath) || !strings.Contains(merged.Notices[0], "data_dir") {
		t.Errorf("提示里应当有项目文件路径与 data_dir，实际 %q", merged.Notices[0])
	}

	// 只合并项目层：data_dir 被忽略后是空串，不是项目里那个值。
	only, err := LoadMerged([]Layer{{Path: projectPath, Project: true}})
	if err != nil {
		t.Fatalf("只合并项目层失败: %v", err)
	}
	if only.Config.DataDir != "" {
		t.Errorf("只合并项目层时 data_dir 应当为空串，实际 %q", only.Config.DataDir)
	}

	// 相对路径的 data_dir 在项目层里是非法的，但既然要忽略，就不该影响校验。
	invalidPath := filepath.Join(dir, "invalid-project.toml")
	srv7Write(t, invalidPath, `data_dir = "relative-data"

[providers.claude]
command = "claude"

[agents.b]
provider = "claude"
model = "opus"
`)
	if _, err := LoadMerged([]Layer{{Path: invalidPath, Project: true}}); err != nil {
		t.Errorf("项目层里被忽略的非法 data_dir 不该让合并失败: %v", err)
	}
}

// TestSrv7ConfigLayerPrecedence 是 §5.3：-config / COUNCIL_CONFIG 胜过
// .council/，.council/ 胜过 ~/.council/；项目层只取离 cwd 最近的一份。
//
// 全部目录都在临时 HOME 与临时仓库里造，不碰真目录；t.Chdir 让
// ResolveConfigLayers 看到真实的 cwd 链。
func TestSrv7ConfigLayerPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(EnvConfigPath, "")

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("建假仓库 .git 失败: %v", err)
	}
	rootConfig := filepath.Join(repo, ".council", "config.toml")
	nearestConfig := filepath.Join(repo, "proj", ".council", "config.toml")
	globalConfig := filepath.Join(home, ".council", "config.toml")
	cwd := filepath.Join(repo, "proj", "sub")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("建 cwd 失败: %v", err)
	}

	srv7Write(t, globalConfig, `[providers.claude]
command = "claude"

[agents.same]
provider = "claude"
model = "opus"
role = "全局"
`)
	srv7Write(t, rootConfig, `[agents.root-only]
provider = "claude"
model = "opus"
`)
	srv7Write(t, nearestConfig, `[agents.same]
provider = "claude"
model = "opus"
role = "最近的项目层"
`)
	t.Chdir(cwd)

	layers, err := ResolveConfigLayers("")
	if err != nil {
		t.Fatalf("ResolveConfigLayers 失败: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("应当合并项目层与全局层两层，实际 %+v", layers)
	}
	if layers[0].Path != nearestConfig || !layers[0].Project {
		t.Errorf("第一层应当是最近的项目配置 %q（Project=true），实际 %+v", nearestConfig, layers[0])
	}
	if layers[1].Path != globalConfig || layers[1].Project {
		t.Errorf("第二层应当是全局配置 %q（Project=false），实际 %+v", globalConfig, layers[1])
	}

	merged, err := LoadMerged(layers)
	if err != nil {
		t.Fatalf("合并配置失败: %v", err)
	}
	if got := merged.Config.Agents["same"].Role; got != "最近的项目层" {
		t.Errorf("同名 agent 应当由最近的项目层胜出，role = %q", got)
	}
	if _, ok := merged.Config.Agents["root-only"]; ok {
		t.Errorf("仓库根那份项目配置不该被合并：只取离 cwd 最近的一份")
	}

	// 只有项目层、没有全局层时也要能解析出项目那一层，不能坚持要求全局存在。
	if err := os.Remove(globalConfig); err != nil {
		t.Fatalf("删全局配置失败: %v", err)
	}
	layers, err = ResolveConfigLayers("")
	if err != nil {
		t.Fatalf("无全局层时 ResolveConfigLayers 失败了: %v", err)
	}
	if len(layers) != 1 || layers[0].Path != nearestConfig || !layers[0].Project {
		t.Errorf("没有全局层时应当只剩最近的项目层，实际 %+v", layers)
	}

	// COUNCIL_CONFIG 与 -config 同级：用户点名的文件独占，不叠项目层。
	explicitPath := filepath.Join(t.TempDir(), "explicit.toml")
	srv7Write(t, explicitPath, `[providers.claude]
command = "claude"

[agents.explicit-only]
provider = "claude"
model = "opus"
`)
	t.Setenv(EnvConfigPath, explicitPath)
	layers, err = ResolveConfigLayers("")
	if err != nil {
		t.Fatalf("COUNCIL_CONFIG 下 ResolveConfigLayers 失败: %v", err)
	}
	if len(layers) != 1 || layers[0].Path != explicitPath {
		t.Fatalf("COUNCIL_CONFIG 应当独占，实际 %+v", layers)
	}
	merged, err = LoadMerged(layers)
	if err != nil {
		t.Fatalf("合并 COUNCIL_CONFIG 失败: %v", err)
	}
	if _, ok := merged.Config.Agents["explicit-only"]; !ok {
		t.Errorf("COUNCIL_CONFIG 指定的配置里的 agent 应当生效")
	}
	if _, ok := merged.Config.Agents["same"]; ok {
		t.Errorf("COUNCIL_CONFIG 生效时不该再合并项目层里的 same")
	}

	// -config 比环境变量更高：两个都给时用 -config。
	flagPath := filepath.Join(t.TempDir(), "flag.toml")
	srv7Write(t, flagPath, `[providers.claude]
command = "claude"

[agents.flag-only]
provider = "claude"
model = "opus"
`)
	layers, err = ResolveConfigLayers(flagPath)
	if err != nil {
		t.Fatalf("-config 下 ResolveConfigLayers 失败: %v", err)
	}
	if len(layers) != 1 || layers[0].Path != flagPath {
		t.Fatalf("-config 应当压过 COUNCIL_CONFIG，实际 %+v", layers)
	}

	// §2：合并后的校验错误要带上是哪一层的文件。
	t.Run("校验错误标出来自哪一层", func(t *testing.T) {
		dir := t.TempDir()
		badGlobal := filepath.Join(dir, "good-global.toml")
		badProject := filepath.Join(dir, "bad-project.toml")
		srv7Write(t, badGlobal, `[providers.claude]
command = "claude"
`)
		srv7Write(t, badProject, `[agents.ghost]
provider = "no_such_provider"
model = "opus"
`)
		_, err := LoadMerged([]Layer{
			{Path: badProject, Project: true},
			{Path: badGlobal},
		})
		if err == nil {
			t.Fatalf("引用不存在的 provider 应当校验失败")
		}
		msg := err.Error()
		for _, want := range []string{"agents.ghost.provider", "no_such_provider", badProject} {
			if !strings.Contains(msg, want) {
				t.Errorf("校验错误里缺少 %q，实际:\n%s", want, msg)
			}
		}
		if strings.Contains(msg, badGlobal) {
			t.Errorf("错误出在项目层，不该把它算到全局层 %q 头上:\n%s", badGlobal, msg)
		}
	})
}
