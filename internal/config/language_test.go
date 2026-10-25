package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLanguageDefaultsToAuto 钉住三条入口的默认值一致：Load、LoadMerged
// 与 Default 都不写 language 时是 auto，而不是空串。
func TestLanguageDefaultsToAuto(t *testing.T) {
	path := tstWriteTOML(t, `[providers.claude]`, `command = "claude"`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Language != LanguageAuto {
		t.Errorf("Load 的 language = %q，期望 auto", cfg.Language)
	}

	merged, err := LoadMerged([]Layer{{Path: path}})
	if err != nil {
		t.Fatalf("LoadMerged: %v", err)
	}
	if merged.Config.Language != LanguageAuto {
		t.Errorf("LoadMerged 的 language = %q，期望 auto", merged.Config.Language)
	}

	if got := Default().Language; got != LanguageAuto {
		t.Errorf("Default().Language = %q，期望 auto", got)
	}
}

// TestLanguageMergeHighLayerWins 覆盖「高层定义才覆盖」：项目层写 en 压过
// 全局层 zh；项目层不写时继承全局层；项目层写 auto 也压过全局层 en。
func TestLanguageMergeHighLayerWins(t *testing.T) {
	tests := []struct {
		name    string
		project string // 空串表示项目层不写 language
		global  string
		want    string
	}{
		{name: "项目层覆盖全局层", project: "en", global: "zh", want: "en"},
		{name: "项目层不写时继承", project: "", global: "en", want: "en"},
		{name: "项目层写 auto 也覆盖", project: "auto", global: "en", want: "auto"},
		{name: "两层都没写时默认", project: "", global: "", want: "auto"},
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
				global = `language = "` + tc.global + `"
` + global
			}
			project := `[agents.g1]
display_name = "G1"
provider = "claude"
model = "opus"
`
			if tc.project != "" {
				project = `language = "` + tc.project + `"
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
			if got := merged.Config.Language; got != tc.want {
				t.Errorf("language = %q，期望 %q", got, tc.want)
			}
		})
	}
}

// TestLanguageValidation 覆盖合法值与非法值。空串被当作 auto（直接构造
// 的 Config 不经过 Load），非法值必须报出 language 路径。
func TestLanguageValidation(t *testing.T) {
	for _, ok := range []string{"", LanguageAuto, LanguageZh, LanguageEn, "ZH", " En "} {
		t.Run("valid_"+ok, func(t *testing.T) {
			cfg := Default()
			cfg.Language = ok
			if err := cfg.Validate(); err != nil {
				t.Errorf("language=%q 应当合法，实际报错: %v", ok, err)
			}
		})
	}
	for _, bad := range []string{"fr", "english", "zh-CN", "auto " + "en"} {
		t.Run("invalid_"+bad, func(t *testing.T) {
			cfg := Default()
			cfg.Language = bad
			err := cfg.Validate()
			tstRequireErrContains(t, err, "language", "不支持的语言")
		})
	}
}

// TestWriteLanguageReplacesTopLevelAndKeepsBytes 钉住「只改 language 这一行」：
// 注释、agent 段与行尾都不动，备份等于写前原文，返回的配置是合并后的新值。
func TestWriteLanguageReplacesTopLevelAndKeepsBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "# 顶部注释\r\n" +
		"data_dir = 'E:\\council-data'  # 行尾注释\r\n" +
		"language = \"zh\"\r\n" +
		"\r\n" +
		"[providers.claude]\r\n" +
		"command = \"claude\"\r\n" +
		"\r\n" +
		"[agents.p1]\r\n" +
		"display_name = \"P1\"\r\n" +
		"provider = \"claude\"\r\n" +
		"model = \"opus\"\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := WriteLanguage([]Layer{{Path: path}}, "en")
	if err != nil {
		t.Fatalf("WriteLanguage: %v", err)
	}
	if !res.Changed || res.WriteTarget != path {
		t.Fatalf("结果 = %+v，期望 Changed=true 且目标是 %s", res, path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(orig, `language = "zh"`, `language = "en"`, 1)
	if string(got) != want {
		t.Errorf("写后文件与期望不符\n实际:\n%q\n期望:\n%q", got, want)
	}
	if res.Config == nil || res.Config.Language != LanguageEn {
		t.Fatalf("返回配置的 language = %+v，期望 en", res.Config)
	}
	backup, err := os.ReadFile(res.BackupPath)
	if err != nil {
		t.Fatalf("读备份: %v", err)
	}
	if string(backup) != orig {
		t.Errorf("备份内容与写前原文不一致")
	}
	temps, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if len(temps) != 0 {
		t.Errorf("残留临时文件: %v", temps)
	}
}

// TestWriteLanguageInsertsBeforeFirstTable 覆盖目标文件没有 language 的
// 常见情形：必须插在第一个表头之前，绝不能追加到某个 [agents.x] 段里。
func TestWriteLanguageInsertsBeforeFirstTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "# 注释\n\n[providers.claude]\ncommand = \"claude\"\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := WriteLanguage([]Layer{{Path: path}}, "zh")
	if err != nil {
		t.Fatalf("WriteLanguage: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# 注释\n\nlanguage = \"zh\"\n\n[providers.claude]\ncommand = \"claude\"\n"
	if string(got) != want {
		t.Errorf("写后文件与期望不符\n实际:\n%q\n期望:\n%q", got, want)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("写后文件加载失败: %v", err)
	}
	if cfg.Language != LanguageZh {
		t.Errorf("Load 的 language = %q，期望 zh", cfg.Language)
	}
	if _, ok := cfg.Providers["claude"]; !ok {
		t.Errorf("写 language 之后 providers 段丢了: %+v", cfg.Providers)
	}
	if !res.Changed {
		t.Errorf("Changed = false，期望 true")
	}
}

// TestWriteLanguageAppendsWhenNoTables 覆盖没有表的最小配置文件：直接追加
// 在末尾，并保留原文原有的换行习惯。
func TestWriteLanguageAppendsWhenNoTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "# 只有注释，没有表\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteLanguage([]Layer{{Path: path}}, "en"); err != nil {
		t.Fatalf("WriteLanguage: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := orig + "language = \"en\"\n"
	if string(got) != want {
		t.Errorf("写后文件 = %q，期望 %q", got, want)
	}
}

// TestWriteLanguageInvalidLeavesFileUntouched 钉住校验失败的文件安全：
// 不写、不备份、不留临时文件，错误是 InvalidConfigError。
func TestWriteLanguageInvalidLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "language = \"zh\"\n\n[providers.claude]\ncommand = \"claude\"\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := WriteLanguage([]Layer{{Path: path}}, "fr")
	var invalid *InvalidConfigError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v，期望 *InvalidConfigError", err)
	}
	if !strings.Contains(err.Error(), "language") {
		t.Errorf("错误文案里没有 language: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != orig {
		t.Errorf("校验失败之后文件被改动了")
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 0 {
		t.Errorf("校验失败时不该生成备份: %v", backups)
	}
	temps, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if len(temps) != 0 {
		t.Errorf("校验失败时残留临时文件: %v", temps)
	}
}

// TestWriteLanguageUnchangedSkipsWrite 覆盖「值本来就一样」：不写文件、
// 不备份，Changed=false。
func TestWriteLanguageUnchangedSkipsWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "language = \"en\"\n\n[providers.claude]\ncommand = \"claude\"\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := WriteLanguage([]Layer{{Path: path}}, "EN")
	if err != nil {
		t.Fatalf("WriteLanguage: %v", err)
	}
	if res.Changed || res.BackupPath != "" {
		t.Errorf("结果 = %+v，期望 Changed=false 且无备份", res)
	}
}
