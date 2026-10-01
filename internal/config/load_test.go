package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// tstWriteTOML 把若干行写成一个临时配置文件并返回完整路径。
// 逐行拼接而不是用带首换行的原始字符串，行号才可预测（语法错误用例依赖这一点）。
func tstWriteTOML(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写临时配置文件失败: %v", err)
	}
	return path
}

// tstRequireErrContains 断言 err 非 nil，且错误文本包含全部给定片段。
// 用 strings.Contains 而不是全文等值，避免措辞一变全线飘红。
func tstRequireErrContains(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望返回错误，实际返回 nil")
	}
	msg := err.Error()
	for _, p := range parts {
		if !strings.Contains(msg, p) {
			t.Errorf("错误信息里缺少片段 %q，实际错误为: %s", p, msg)
		}
	}
}

func TestLoadValidFile(t *testing.T) {
	path := tstWriteTOML(t,
		`data_dir = 'E:\council-data'`,
		``,
		`[providers.claude]`,
		`command = "claude"`,
		``,
		`[providers.grok]`,
		`command = 'C:\tools\grok.exe'`,
		``,
		`[agents.theory]`,
		`display_name = "Theory"`,
		`provider     = "claude"`,
		`model        = "opus"`,
		`effort       = "high"`,
		`role         = "只做一阶剪切变形理论的推导与校核"`,
		`permissions  = "workspace"`,
		`mode         = "worker"`,
		``,
		`[teams.fsdt]`,
		`display_name = "FSDT 讨论组"`,
		`members      = ["theory"]`,
	)

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load 合法文件失败: %v", err)
	}
	if got == nil {
		t.Fatal("Load 返回的 *Config 是 nil")
	}

	want := &Config{
		DataDir: `E:\council-data`,
		// 这份测试配置没有写 store_thought，Load 要按未决 #9(a) 的默认值
		// 填成 true；TestLoadDefaultsStoreThoughtToTrue 单独钉住这条default，
		// 这里只是让 TestLoadValidFile 的整体 DeepEqual 不因为漏了这一个
		// 字段而红。
		StoreThought: true,
		Providers: map[string]ProviderConfig{
			"claude": {Command: "claude"},
			"grok":   {Command: `C:\tools\grok.exe`},
		},
		Agents: map[string]AgentConfig{
			"theory": {
				DisplayName: "Theory",
				Provider:    "claude",
				Model:       "opus",
				Effort:      "high",
				Role:        "只做一阶剪切变形理论的推导与校核",
				Permissions: PermissionWorkspace,
				Mode:        ModeWorker,
			},
		},
		Teams: map[string]TeamConfig{
			"fsdt": {DisplayName: "FSDT 讨论组", Members: []string{"theory"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load 结果与期望不符\n实际: %+v\n期望: %+v", got, want)
	}
}

// TestLoadRejectsUnknownKey 覆盖规格 D-1：未知键一律报错，且错误里要有该键的完整点分路径。
func TestLoadRejectsUnknownKey(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		wantKeys []string
	}{
		{
			name: "顶层未知键",
			lines: []string{
				`data_dir = 'E:\council-data'`,
				`unknown_top_key = "x"`,
			},
			wantKeys: []string{"unknown_top_key"},
		},
		{
			name: "表内未知键",
			lines: []string{
				`[providers.claude]`,
				`command = "claude"`,
				``,
				`[agents.theory]`,
				`provider = "claude"`,
				`model    = "opus"`,
				`efort    = "high"`,
			},
			wantKeys: []string{"agents.theory.efort"},
		},
		{
			name: "多个未知键要一起列出",
			lines: []string{
				`stray_root = 1`,
				``,
				`[providers.claude]`,
				`command  = "claude"`,
				`argument = "x"`,
			},
			wantKeys: []string{"stray_root", "providers.claude.argument"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := tstWriteTOML(t, tc.lines...)
			_, err := Load(path)
			tstRequireErrContains(t, err, tc.wantKeys...)
		})
	}
}

// TestLoadSyntaxErrorMentionsLine 覆盖规格 G：TOML 语法错误的错误信息里含行号。
// 坏行故意放在第 3 行，BurntSushi/toml 的 ParseError 文本形如 "toml: line 3: ..."。
func TestLoadSyntaxErrorMentionsLine(t *testing.T) {
	path := tstWriteTOML(t,
		`data_dir = 'E:\council-data'`,
		``,
		`this is not toml`,
		``,
		`[providers.claude]`,
		`command = "claude"`,
	)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load 语法错误的文件时期望报错，实际返回 nil")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "line 3") {
		t.Errorf("语法错误信息里没有出现行号 line 3，实际错误为: %s", err.Error())
	}
}

// TestLoadMissingFileMentionsPath 覆盖契约：文件不存在的错误信息里要有完整路径，
// 调用方据此提示用户先建配置。
func TestLoadMissingFileMentionsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-config.toml")
	_, err := Load(path)
	tstRequireErrContains(t, err, path)
}

// TestLoadFillsDefaults 覆盖规格 D-5 与 D-6 的缺省：permissions 空填 read，mode 空填 discussion。
func TestLoadFillsDefaults(t *testing.T) {
	path := tstWriteTOML(t,
		`[providers.claude]`,
		`command = "claude"`,
		``,
		`[agents.theory]`,
		`provider = "claude"`,
		`model    = "opus"`,
	)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	agent, ok := cfg.Agents["theory"]
	if !ok {
		t.Fatalf("Load 结果里没有 agent theory，实际为 %+v", cfg.Agents)
	}
	if agent.Permissions != PermissionRead {
		t.Errorf("permissions 缺省值 = %q，期望 %q", agent.Permissions, PermissionRead)
	}
	if agent.Mode != ModeDiscussion {
		t.Errorf("mode 缺省值 = %q，期望 %q", agent.Mode, ModeDiscussion)
	}
}

// TestLoadNormalizesCase 覆盖契约：Permissions 与 Mode 在 Load 里就规范成小写。
func TestLoadNormalizesCase(t *testing.T) {
	tests := []struct {
		name            string
		permissions     string
		mode            string
		wantPermissions Permission
		wantMode        Mode
	}{
		{
			name:            "全大写 READ 与首字母大写 Discussion",
			permissions:     "READ",
			mode:            "Discussion",
			wantPermissions: PermissionRead,
			wantMode:        ModeDiscussion,
		},
		{
			name:            "大小写混杂的 WorkSpace 与 JUDGE",
			permissions:     "WorkSpace",
			mode:            "JUDGE",
			wantPermissions: PermissionWorkspace,
			wantMode:        ModeJudge,
		},
		{
			name:            "带下划线的 Discussion_And_Worker",
			permissions:     "Full",
			mode:            "Discussion_And_Worker",
			wantPermissions: PermissionFull,
			wantMode:        ModeDiscussionAndWorker,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := tstWriteTOML(t,
				`[providers.claude]`,
				`command = "claude"`,
				``,
				`[agents.theory]`,
				`provider    = "claude"`,
				`model       = "opus"`,
				`permissions = "`+tc.permissions+`"`,
				`mode        = "`+tc.mode+`"`,
			)
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load 失败: %v", err)
			}
			agent := cfg.Agents["theory"]
			if agent.Permissions != tc.wantPermissions {
				t.Errorf("permissions = %q，期望规范成 %q", agent.Permissions, tc.wantPermissions)
			}
			if agent.Mode != tc.wantMode {
				t.Errorf("mode = %q，期望规范成 %q", agent.Mode, tc.wantMode)
			}
		})
	}
}

// TestLoadDefaultsStoreThoughtToTrue 钉住未决 #9(a) 的核心坑：配置文件完全
// 不写 store_thought 这个键时，Load 读出来的必须是 true。
//
// StoreThought 是裸 bool，toml.Decode 只在用户写了这个键时才覆盖字段，
// 没写就保留 Decode 之前已经在结构体里的值。若 Load 从 cfg := Config{} 的
// 自然零值 false 开始 Decode，这条用例就会红——那正是本条要拦住的静默失败：
// 用户省略该键得到的是设计默认值的反面，而且没有任何报错。
func TestLoadDefaultsStoreThoughtToTrue(t *testing.T) {
	path := tstWriteTOML(t,
		`[providers.claude]`,
		`command = "claude"`,
	)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if !cfg.StoreThought {
		t.Errorf("StoreThought = false，配置文件完全没写 store_thought 时期望缺省为 true")
	}
}

// TestLoadStoreThoughtExplicitFalse 覆盖反方向：显式写 store_thought = false
// 必须原样读出 false，不能被 Load 在 Decode 之前预置的默认值盖住。
// 与上一条合起来才证明 Decode 真的读到了这个键，而不是预置值恰好没被碰过。
func TestLoadStoreThoughtExplicitFalse(t *testing.T) {
	path := tstWriteTOML(t,
		`store_thought = false`,
		``,
		`[providers.claude]`,
		`command = "claude"`,
	)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.StoreThought {
		t.Errorf("StoreThought = true，配置文件显式写了 store_thought = false，期望原样读出 false")
	}
}

// TestLoadAbsentMapsAreEmptyNotNil 断言三张表在配置文件里缺席时拿到的是非 nil 的空 map。
// 调用方（REPL 的 /providers 之类）可以直接遍历，不必先判空。
func TestLoadAbsentMapsAreEmptyNotNil(t *testing.T) {
	path := tstWriteTOML(t, `data_dir = 'E:\council-data'`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	tests := []struct {
		name  string
		size  int
		isNil bool
	}{
		{name: "providers", size: len(cfg.Providers), isNil: cfg.Providers == nil},
		{name: "agents", size: len(cfg.Agents), isNil: cfg.Agents == nil},
		{name: "teams", size: len(cfg.Teams), isNil: cfg.Teams == nil},
	}
	for _, tc := range tests {
		t.Run(tc.name+" 非 nil 且为空", func(t *testing.T) {
			if tc.isNil {
				t.Errorf("%s 是 nil，期望是非 nil 的空 map", tc.name)
			}
			if tc.size != 0 {
				t.Errorf("%s 的长度 = %d，期望 0", tc.name, tc.size)
			}
		})
	}
}
