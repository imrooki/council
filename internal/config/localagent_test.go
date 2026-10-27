package config

// localagent_test.go 钉住 local_agent 这个顶层键的三件事：
// 缺省是 off、分层合并遵守「高层定义才覆盖」、非法取值必须报错。
//
// 为什么合并要单独测：merge.go 是逐字段手写合并的，新字段漏加一段不会
// 报错，只会永远停在预置值上（Plan v3 §4.3 ③ 明写这个坑）。

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLocalAgentDefaultsToOff 钉住三条入口的缺省都是 off：Load、LoadMerged
// 与 Default。空串也是一种 off 表示（直接构造 Config 的调用方不经过
// applyDefaults），但 Load/LoadMerged/Default 交出来的应当是明确的 "off"。
func TestLocalAgentDefaultsToOff(t *testing.T) {
	path := tstWriteTOML(t, `[providers.claude]`, `command = "claude"`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LocalAgent != LocalAgentOff {
		t.Errorf("Load 的 local_agent = %q，期望 %q", cfg.LocalAgent, LocalAgentOff)
	}

	merged, err := LoadMerged([]Layer{{Path: path}})
	if err != nil {
		t.Fatalf("LoadMerged: %v", err)
	}
	if merged.Config.LocalAgent != LocalAgentOff {
		t.Errorf("LoadMerged 的 local_agent = %q，期望 %q", merged.Config.LocalAgent, LocalAgentOff)
	}

	if got := Default().LocalAgent; got != LocalAgentOff {
		t.Errorf("Default().LocalAgent = %q，期望 %q", got, LocalAgentOff)
	}
}

// TestLocalAgentLoadNormalizesValues 覆盖大小写与空白归一，以及「写了
// observe 真的读得出来」。写错的取值不在 Load 里被吃掉，而是留给 Validate。
func TestLocalAgentLoadNormalizesValues(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "显式 observe", raw: "observe", want: LocalAgentObserve},
		{name: "大写与空白", raw: `  OBSERVE  `, want: LocalAgentObserve},
		{name: "显式 off", raw: "off", want: LocalAgentOff},
		{name: "大写 off", raw: "OFF", want: LocalAgentOff},
		{name: "空串", raw: "", want: LocalAgentOff},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := tstWriteTOML(t,
				`local_agent = "`+tc.raw+`"`,
				`[providers.claude]`,
				`command = "claude"`,
			)
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.LocalAgent != tc.want {
				t.Errorf("local_agent = %q，期望 %q", cfg.LocalAgent, tc.want)
			}
			if err := cfg.Validate(); err != nil {
				t.Errorf("合法取值 %q 没通过 Validate: %v", tc.raw, err)
			}
		})
	}
}

// TestLocalAgentMergeHighLayerWins 覆盖「高层定义才覆盖」：项目层压过全局层，
// 项目层没写时继承全局层，两层都没写时是 off。
func TestLocalAgentMergeHighLayerWins(t *testing.T) {
	tests := []struct {
		name    string
		project string // 空串表示项目层不写 local_agent
		global  string
		want    string
	}{
		{name: "项目层 observe 压过全局 off", project: LocalAgentObserve, global: LocalAgentOff, want: LocalAgentObserve},
		{name: "项目层不写时继承全局 observe", project: "", global: LocalAgentObserve, want: LocalAgentObserve},
		{name: "项目层 off 压过全局 observe", project: LocalAgentOff, global: LocalAgentObserve, want: LocalAgentOff},
		{name: "两层都没写时默认 off", project: "", global: "", want: LocalAgentOff},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			globalPath := filepath.Join(dir, "global.toml")
			projectPath := filepath.Join(dir, "project.toml")
			global := `[providers.claude]
command = "claude"
`
			if tc.global != "" {
				global = `local_agent = "` + tc.global + `"
` + global
			}
			project := `[agents.g1]
display_name = "G1"
provider = "claude"
model = "opus"
`
			if tc.project != "" {
				project = `local_agent = "` + tc.project + `"
` + project
			}
			if err := os.WriteFile(globalPath, []byte(global), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(projectPath, []byte(project), 0o644); err != nil {
				t.Fatal(err)
			}
			merged, err := LoadMerged([]Layer{{Path: projectPath, Project: true}, {Path: globalPath}})
			if err != nil {
				t.Fatalf("LoadMerged: %v", err)
			}
			if got := merged.Config.LocalAgent; got != tc.want {
				t.Errorf("local_agent = %q，期望 %q", got, tc.want)
			}
		})
	}
}

// TestLocalAgentValidation 覆盖合法与非法取值。register / coordinate /
// collaborate 在 L1 没有实现，必须被拒，不能静默当成 off —— 那会让用户
// 以为已经开了登记或协调。
func TestLocalAgentValidation(t *testing.T) {
	for _, ok := range []string{"", LocalAgentOff, LocalAgentObserve, "OFF", " Observe "} {
		t.Run("valid_"+ok, func(t *testing.T) {
			cfg := Default()
			cfg.LocalAgent = ok
			if err := cfg.Validate(); err != nil {
				t.Errorf("local_agent=%q 应当合法，实际报错: %v", ok, err)
			}
		})
	}
	for _, bad := range []string{"on", "true", "register", "coordinate", "collaborate", "observing"} {
		t.Run("invalid_"+bad, func(t *testing.T) {
			cfg := Default()
			cfg.LocalAgent = bad
			err := cfg.Validate()
			tstRequireErrContains(t, err, "local_agent", "off / observe")
		})
	}
}
