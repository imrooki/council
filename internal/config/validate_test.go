package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// tstAbsDataDir 是用例里当作「合法的绝对数据目录」的值。DefaultDataDir（D-9，`E:\council-data`）
// 在 Windows 上是绝对路径，这时原样返回它，用例与加这个函数之前逐字相同；别的平台上它不是绝对路径，
// Validate 会拒掉，于是换成本平台的一个绝对路径。只为让本包在 Linux 上也能跑（2026-09-25），
// 不涉及 D-9 本身：非 Windows 上没有默认数据目录仍是现状，见 TestResolveDataDir。
func tstAbsDataDir() string {
	if filepath.IsAbs(DefaultDataDir) {
		return DefaultDataDir
	}
	return filepath.Join(string(filepath.Separator), "council-data")
}

// tstValidConfig 返回一份每个字段都显式填满的合法配置，供负例逐项改坏。
// 注意：缺省值由 Load 填、Validate 不填（见契约「缺省在 Load 里填」），
// 所以这里的 Permissions 与 Mode 必须显式给合法枚举值，不能留空。
func tstValidConfig() *Config {
	return &Config{
		DataDir: tstAbsDataDir(),
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
				Permissions: PermissionRead,
				Mode:        ModeDiscussion,
			},
			"critic": {
				DisplayName: "Critic",
				Provider:    "grok",
				Model:       "grok-4.6",
				Effort:      "", // 规格 D-4：effort 允许为空
				Role:        "挑错",
				Permissions: PermissionWorkspace,
				Mode:        ModeJudge,
			},
		},
		Teams: map[string]TeamConfig{
			"project": {DisplayName: "<project> 讨论组", Members: []string{"theory", "critic"}},
		},
	}
}

// tstEditAgent 改 map 里某个 agent 的字段。map 取值返回的是副本，必须写回。
func tstEditAgent(cfg *Config, id string, fn func(*AgentConfig)) {
	a := cfg.Agents[id]
	fn(&a)
	cfg.Agents[id] = a
}

// tstEditProvider 改 map 里某个 provider 的字段。
func tstEditProvider(cfg *Config, id string, fn func(*ProviderConfig)) {
	p := cfg.Providers[id]
	fn(&p)
	cfg.Providers[id] = p
}

// tstEditTeam 改 map 里某个 team 的字段。
func tstEditTeam(cfg *Config, id string, fn func(*TeamConfig)) {
	tm := cfg.Teams[id]
	fn(&tm)
	cfg.Teams[id] = tm
}

// TestValidateAcceptsValidConfig 同时是防 typed-nil 陷阱的用例：
// Validate 声明的返回值是 error 接口，实现里如果写 `var errs ValidationErrors; return errs`，
// 返回的是一个非 nil 接口包着的 nil 切片，调用方的 `err != nil` 会为真。
// 这里就是按调用方的写法断言，不用 reflect 绕过。
func TestValidateAcceptsValidConfig(t *testing.T) {
	cfg := tstValidConfig()
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("合法配置的 Validate 必须返回字面 nil，实际返回: %v", err)
	}
}

// TestValidateRules 覆盖规格 D 的第 2 到第 9 条，每条至少一个负例，
// 断言错误信息里出现对应的点分路径。第 1 条（未知键）结构上归 Load，见下面单独的用例。
func TestValidateRules(t *testing.T) {
	tests := []struct {
		name string
		// mutate 把一份合法配置改成只在一处违规。
		mutate func(cfg *Config)
		// wantParts 是错误信息里必须出现的片段，通常是点分路径。
		wantParts []string
	}{
		{
			name: "D2 provider 的 command 为空",
			mutate: func(cfg *Config) {
				tstEditProvider(cfg, "grok", func(p *ProviderConfig) { p.Command = "" })
			},
			wantParts: []string{"providers.grok.command"},
		},
		{
			name: "D3 agent 引用了不存在的 provider",
			mutate: func(cfg *Config) {
				tstEditAgent(cfg, "theory", func(a *AgentConfig) { a.Provider = "ghost" })
			},
			// 规格要求指出是哪个 agent 引用了哪个不存在的 provider，所以两者都要出现。
			wantParts: []string{"agents.theory.provider", "ghost"},
		},
		{
			name: "D4 agent 的 model 为空",
			mutate: func(cfg *Config) {
				tstEditAgent(cfg, "theory", func(a *AgentConfig) { a.Model = "" })
			},
			wantParts: []string{"agents.theory.model"},
		},
		{
			name: "D5 permissions 不属于三档枚举",
			mutate: func(cfg *Config) {
				tstEditAgent(cfg, "theory", func(a *AgentConfig) { a.Permissions = Permission("admin") })
			},
			wantParts: []string{"agents.theory.permissions"},
		},
		{
			name: "D6 mode 不属于五种枚举",
			mutate: func(cfg *Config) {
				tstEditAgent(cfg, "critic", func(a *AgentConfig) { a.Mode = Mode("chat") })
			},
			wantParts: []string{"agents.critic.mode"},
		},
		{
			name: "D7 agent id 含大写字母",
			mutate: func(cfg *Config) {
				cfg.Agents["Theory"] = cfg.Agents["theory"]
			},
			wantParts: []string{"agents.Theory"},
		},
		{
			name: "D7 agent id 以下划线开头",
			mutate: func(cfg *Config) {
				cfg.Agents["_theory"] = cfg.Agents["theory"]
			},
			wantParts: []string{"agents._theory"},
		},
		{
			name: "D7 provider id 含大写字母",
			mutate: func(cfg *Config) {
				cfg.Providers["Claude"] = ProviderConfig{Command: "claude"}
			},
			wantParts: []string{"providers.Claude"},
		},
		{
			name: "D7 team id 以连字符开头",
			mutate: func(cfg *Config) {
				cfg.Teams["-project"] = TeamConfig{DisplayName: "坏 id", Members: []string{"theory"}}
			},
			wantParts: []string{"teams.-project"},
		},
		{
			name: "D8 user 是保留 id 不得用作 agent id",
			mutate: func(cfg *Config) {
				cfg.Agents["user"] = cfg.Agents["theory"]
			},
			wantParts: []string{"agents.user"},
		},
		{
			// system 与 user 同类：Canonical Conversation 的保留 sender。
			// 配置里出现同名 agent 的话，它的每一条发言都会被当成保留 sender
			// 处理 —— 跳过活跃成员校验、且一律不许带脑子，于是在 /log 里永远没有归属。
			name: "system 也是保留 id 不得用作 agent id",
			mutate: func(cfg *Config) {
				cfg.Agents["system"] = cfg.Agents["theory"]
			},
			wantParts: []string{"agents.system"},
		},
		{
			name: "D9 team 的 members 为空",
			mutate: func(cfg *Config) {
				tstEditTeam(cfg, "project", func(tm *TeamConfig) { tm.Members = nil })
			},
			wantParts: []string{"teams.project.members"},
		},
		{
			name: "D9 team 的 member 不是已定义 agent",
			mutate: func(cfg *Config) {
				tstEditTeam(cfg, "project", func(tm *TeamConfig) {
					tm.Members = []string{"theory", "ghost"}
				})
			},
			// 只断言路径。规格没要求点名是哪个成员不存在，多断言一层会变脆。
			wantParts: []string{"teams.project.members"},
		},
		{
			name: "D9 team 的 member 重复",
			mutate: func(cfg *Config) {
				tstEditTeam(cfg, "project", func(tm *TeamConfig) {
					tm.Members = []string{"theory", "critic", "theory"}
				})
			},
			wantParts: []string{"teams.project.members"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tstValidConfig()
			tc.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("期望 Validate 报错，实际返回 nil。配置为: %+v", cfg)
			}
			msg := err.Error()
			for _, part := range tc.wantParts {
				if !strings.Contains(msg, part) {
					t.Errorf("错误信息里缺少 %q，实际错误为: %s", part, msg)
				}
			}
		})
	}
}

// TestValidateReportsAllErrorsAtOnce 覆盖规格 D-10：一次报全部错误，不是遇到第一个就返回。
// 三处错分布在三张表里，互不级联，所以三条都必须出现。
func TestValidateReportsAllErrorsAtOnce(t *testing.T) {
	cfg := tstValidConfig()
	tstEditProvider(cfg, "grok", func(p *ProviderConfig) { p.Command = "" })
	tstEditAgent(cfg, "theory", func(a *AgentConfig) { a.Provider = "ghost" })
	tstEditTeam(cfg, "project", func(tm *TeamConfig) { tm.Members = nil })

	err := cfg.Validate()
	if err == nil {
		t.Fatal("三处同时出错时期望 Validate 报错，实际返回 nil")
	}
	msg := err.Error()
	// 不断言错误条数，实现多报一条不算回归；只要求这三条都在。
	for _, part := range []string{
		"providers.grok.command",
		"agents.theory.provider",
		"teams.project.members",
	} {
		if !strings.Contains(msg, part) {
			t.Errorf("批量错误里缺少 %q，实际错误为: %s", part, msg)
		}
	}
}

// TestValidationErrorErrorContainsPathAndMsg 断言单条错误能定位：路径与描述都要在文本里。
func TestValidationErrorErrorContainsPathAndMsg(t *testing.T) {
	e := ValidationError{Path: "agents.theory.model", Msg: "model 不能为空"}
	got := e.Error()
	for _, part := range []string{e.Path, e.Msg} {
		if !strings.Contains(got, part) {
			t.Errorf("ValidationError.Error() = %q，缺少片段 %q", got, part)
		}
	}
}

// TestValidationErrorsErrorIsSortedByPath 覆盖契约：ValidationErrors.Error() 按 Path 字典序拼接。
// 故意用乱序构造，断言各路径在输出里的出现次序严格递增。
// 只看次序不看分隔符，实现用换行还是分号都不影响。
func TestValidationErrorsErrorIsSortedByPath(t *testing.T) {
	errs := ValidationErrors{
		{Path: "teams.project.members", Msg: "members 不能为空"},
		{Path: "agents.theory.model", Msg: "model 不能为空"},
		{Path: "providers.grok.command", Msg: "command 不能为空"},
		{Path: "agents.critic.mode", Msg: "mode 不是合法枚举"},
	}

	got := errs.Error()
	wantOrder := []string{
		"agents.critic.mode",
		"agents.theory.model",
		"providers.grok.command",
		"teams.project.members",
	}

	prev := -1
	for _, path := range wantOrder {
		at := strings.Index(got, path)
		if at < 0 {
			t.Fatalf("ValidationErrors.Error() 里找不到路径 %q，实际输出为: %s", path, got)
		}
		if at <= prev {
			t.Errorf("路径 %q 的位置 %d 未晚于前一条的位置 %d，输出未按 Path 字典序排列: %s",
				path, at, prev, got)
		}
		prev = at
	}
}

// TestLoadAndValidateRule1UnknownKey 覆盖规格 D-1。
// 这条规则结构上不可能由 Validate 覆盖：Config 结构体里没有承载未知键的字段，
// 未知键只能在 Load 解析阶段由 toml.MetaData.Undecoded 发现。
// 用 LoadAndValidate 走完整条链路，顺带给 LoadAndValidate 一个用例。
func TestLoadAndValidateRule1UnknownKey(t *testing.T) {
	path := tstWriteTOML(t,
		`[providers.claude]`,
		`command = "claude"`,
		``,
		`[agents.theory]`,
		`provider = "claude"`,
		`model    = "opus"`,
		`efort    = "high"`,
	)

	_, err := LoadAndValidate(path)
	tstRequireErrContains(t, err, "agents.theory.efort")
}

// TestLoadAndValidateValidFile 是 LoadAndValidate 的正例：合法文件应当一路通过。
func TestLoadAndValidateValidFile(t *testing.T) {
	path := tstWriteTOML(t,
		`data_dir = '`+tstAbsDataDir()+`'`,
		``,
		`[providers.claude]`,
		`command = "claude"`,
		``,
		`[agents.theory]`,
		`display_name = "Theory"`,
		`provider     = "claude"`,
		`model        = "opus"`,
		``,
		`[teams.project]`,
		`display_name = "<project> 讨论组"`,
		`members      = ["theory"]`,
	)

	cfg, err := LoadAndValidate(path)
	if err != nil {
		t.Fatalf("LoadAndValidate 合法文件失败: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadAndValidate 返回的 *Config 是 nil")
	}
}

// TestValidateRejectsInvalidTeamMemberAfterAgentRemoved 是一个贴近真实改配置场景的回归用例：
// 删掉一个 agent 但忘了从 team 里删，必须被 D-9 抓住。
func TestValidateRejectsInvalidTeamMemberAfterAgentRemoved(t *testing.T) {
	cfg := tstValidConfig()
	delete(cfg.Agents, "critic")

	err := cfg.Validate()
	if err == nil {
		t.Fatal("team 引用了已删除的 agent，期望 Validate 报错，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "teams.project.members") {
		t.Errorf("错误信息里缺少 teams.project.members，实际错误为: %s", err.Error())
	}
}
