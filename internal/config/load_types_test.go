package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadRejectsTypeMismatch 覆盖规格 D-1 的后半句：
// MetaData.Undecoded 只管「键不认识」，管不到「键认识但类型不对」。
// 这一类由 TOML 解码器自己报错，必须单独有用例，否则容易误以为查了未知键就覆盖了。
func TestLoadRejectsTypeMismatch(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantHint string
	}{
		{
			name: "members 给了字符串而不是数组",
			body: `
[providers.claude]
command = "claude"

[agents.theory]
provider = "claude"
model    = "opus"

[teams.project]
members = "theory"
`,
			wantHint: "members",
		},
		{
			name: "permissions 给了整数而不是字符串",
			body: `
[providers.claude]
command = "claude"

[agents.theory]
provider    = "claude"
model       = "opus"
permissions = 1
`,
			wantHint: "permissions",
		},
		{
			name: "data_dir 给了数组而不是字符串",
			body: `
data_dir = ["E:\\council-data"]

[providers.claude]
command = "claude"
`,
			wantHint: "data_dir",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("写测试配置失败: %v", err)
			}

			_, err := Load(path)
			if err == nil {
				t.Fatalf("类型不匹配的配置应当报错，实际返回 nil")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("错误信息应当指出出错的键 %q，实际是: %v", tc.wantHint, err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("错误信息应当含配置文件路径 %q，实际是: %v", path, err)
			}
		})
	}
}

// TestResolveConfigPathDoesNotFallBackOnMissingFile 覆盖规格 C 节新增的一条：
// 空串才回落，非空但文件不存在必须原样返回并由 Load 报错，
// 绝不能悄悄改用下一级来源。用户打错一个字却跑了默认配置，是最难查的一类问题。
func TestResolveConfigPathDoesNotFallBackOnMissingFile(t *testing.T) {
	fallback := filepath.Join(t.TempDir(), "env-config.toml")
	if err := os.WriteFile(fallback, []byte("[providers.claude]\ncommand = \"claude\"\n"), 0o600); err != nil {
		t.Fatalf("写回落配置失败: %v", err)
	}
	t.Setenv(EnvConfigPath, fallback)

	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")

	got, err := ResolveConfigPath(missing)
	if err != nil {
		t.Fatalf("ResolveConfigPath 不应当因文件不存在而报错: %v", err)
	}
	if got != missing {
		t.Fatalf("应当原样返回高优先级路径 %q，实际是 %q（回落到了环境变量）", missing, got)
	}

	if _, err := Load(got); err == nil {
		t.Fatalf("对不存在的配置文件，Load 应当报错而不是静默换一份配置")
	}
}
