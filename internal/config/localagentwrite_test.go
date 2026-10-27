package config

// localagentwrite_test.go 钉住第 6 类设置页那一行的写盘：只改 local_agent
// 这一行、只写 layers[0]、非法取值一个字节都不动。
//
// 与 language_test.go 的用例是刻意对齐的（同一套 setTopLevelKey），差别是
// 多了一条「两个顶层键可以共存」——设置页现在会写两次，第二次不能把第一次
// 写的 language 挤掉或吞进表里。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteLocalAgentReplacesTopLevelAndKeepsBytes 覆盖「已有这一行就原地改」：
// 注释、agent 段、CRLF 行尾都不动，备份等于写前原文。
func TestWriteLocalAgentReplacesTopLevelAndKeepsBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "# 顶部注释\r\n" +
		"data_dir = 'E:\\council-data'  # 行尾注释\r\n" +
		"language = \"zh\"\r\n" +
		"local_agent = \"off\"\r\n" +
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

	res, err := WriteLocalAgent([]Layer{{Path: path}}, LocalAgentCoordinate)
	if err != nil {
		t.Fatalf("WriteLocalAgent: %v", err)
	}
	if !res.Changed || res.WriteTarget != path {
		t.Fatalf("结果 = %+v，期望 Changed=true 且目标是 %s", res, path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(orig, `local_agent = "off"`, `local_agent = "coordinate"`, 1)
	if string(got) != want {
		t.Errorf("写后文件与期望不符\n实际:\n%q\n期望:\n%q", got, want)
	}
	if res.Config == nil || res.Config.LocalAgent != LocalAgentCoordinate {
		t.Fatalf("返回配置的 local_agent = %+v，期望 coordinate", res.Config)
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

// TestWriteLocalAgentInsertsBeforeFirstTable 覆盖目标文件没有这个键：必须插在
// 第一个表头之前，绝不能追加到某个 [agents.x] 段里（那会让它变成该 agent 的
// 未知字段，而配置拒绝未知键）。
func TestWriteLocalAgentInsertsBeforeFirstTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "# 注释\n\n[providers.claude]\ncommand = \"claude\"\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := WriteLocalAgent([]Layer{{Path: path}}, LocalAgentObserve)
	if err != nil {
		t.Fatalf("WriteLocalAgent: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# 注释\n\nlocal_agent = \"observe\"\n\n[providers.claude]\ncommand = \"claude\"\n"
	if string(got) != want {
		t.Errorf("写后文件与期望不符\n实际:\n%q\n期望:\n%q", got, want)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("写后文件加载失败: %v", err)
	}
	if cfg.LocalAgent != LocalAgentObserve {
		t.Errorf("Load 的 local_agent = %q，期望 observe", cfg.LocalAgent)
	}
	if _, ok := cfg.Providers["claude"]; !ok {
		t.Errorf("写 local_agent 之后 providers 段丢了: %+v", cfg.Providers)
	}
	if !res.Changed {
		t.Errorf("Changed = false，期望 true")
	}
}

// TestWriteLanguageThenLocalAgentCoexist 是「两次写盘互不踩踏」：先写 language
// 再写 local_agent（以及反序），两个键都还在顶层，都读得出来。
func TestWriteLanguageThenLocalAgentCoexist(t *testing.T) {
	for _, order := range [][]string{{"language", "local_agent"}, {"local_agent", "language"}} {
		t.Run(strings.Join(order, "_then_"), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			if err := os.WriteFile(path,
				[]byte("[providers.claude]\ncommand = \"claude\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, key := range order {
				switch key {
				case "language":
					if _, err := WriteLanguage([]Layer{{Path: path}}, LanguageEn); err != nil {
						t.Fatalf("WriteLanguage: %v", err)
					}
				case "local_agent":
					if _, err := WriteLocalAgent([]Layer{{Path: path}}, LocalAgentCoordinate); err != nil {
						t.Fatalf("WriteLocalAgent: %v", err)
					}
				}
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("两次写盘后文件加载失败: %v", err)
			}
			if cfg.Language != LanguageEn {
				t.Errorf("language = %q，期望 en", cfg.Language)
			}
			if cfg.LocalAgent != LocalAgentCoordinate {
				t.Errorf("local_agent = %q，期望 coordinate", cfg.LocalAgent)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			if strings.Index(text, "language =") > strings.Index(text, "[providers.claude]") ||
				strings.Index(text, "local_agent =") > strings.Index(text, "[providers.claude]") {
				t.Errorf("顶层键被写进了表里:\n%s", text)
			}
		})
	}
}

// TestWriteLocalAgentUnchangedSkipsWrite 覆盖「值本来就一样」：不写文件、
// 不备份，Changed=false。大小写差异也算没变（WriteLocalAgent 会归一）。
func TestWriteLocalAgentUnchangedSkipsWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "local_agent = \"observe\"\n\n[providers.claude]\ncommand = \"claude\"\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := WriteLocalAgent([]Layer{{Path: path}}, " OBSERVE ")
	if err != nil {
		t.Fatalf("WriteLocalAgent: %v", err)
	}
	if res.Changed || res.BackupPath != "" {
		t.Errorf("结果 = %+v，期望 Changed=false 且无备份", res)
	}
}

// TestWriteLocalAgentEmptyMeansOff 钉住空串的语义：写进去的是 off，不是空串。
func TestWriteLocalAgentEmptyMeansOff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path,
		[]byte("local_agent = \"observe\"\n\n[providers.claude]\ncommand = \"claude\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteLocalAgent([]Layer{{Path: path}}, "   "); err != nil {
		t.Fatalf("WriteLocalAgent: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `local_agent = "off"`) {
		t.Errorf("空串没有写成 off:\n%s", got)
	}
}

// TestWriteLocalAgentInvalidLeavesFileUntouched 钉住校验失败的文件安全：
// 不写、不备份、不留临时文件，错误是 InvalidConfigError。
func TestWriteLocalAgentInvalidLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "local_agent = \"off\"\n\n[providers.claude]\ncommand = \"claude\"\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := WriteLocalAgent([]Layer{{Path: path}}, "register")
	var invalid *InvalidConfigError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v，期望 *InvalidConfigError", err)
	}
	if !strings.Contains(err.Error(), "local_agent") {
		t.Errorf("错误文案里没有 local_agent: %v", err)
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

// TestSetTopLevelKeyRejectsBadNames 是 setTopLevelKey 的护栏：键名会被拼进
// `key = "value"`，带引号/换行/等号的字符串等于往配置文件里注入字节。
func TestSetTopLevelKeyRejectsBadNames(t *testing.T) {
	for _, bad := range []string{"", "1abc", "local agent", `local_agent = "x"`, "local_agent\n", "local.agent"} {
		if _, _, err := setTopLevelKey([]byte(""), bad, "off"); err == nil {
			t.Errorf("setTopLevelKey 接受了非法键名 %q", bad)
		}
	}
	for _, ok := range []string{"language", "local_agent", "data-dir"} {
		if _, _, err := setTopLevelKey([]byte(""), ok, "off"); err != nil {
			t.Errorf("setTopLevelKey 拒绝了合法键名 %q: %v", ok, err)
		}
	}
}
