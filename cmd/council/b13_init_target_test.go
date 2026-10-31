package main

// B13：cwd 下有 .council/config.toml 时，init 不能把全局配置写进项目层。
// 项目层的 data_dir 在合并时会被忽略，写进去等于“成功但永不生效”。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestB13InitTargetsUserConfigNotProjectLayer(t *testing.T) {
	initTestEnv(t)
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COUNCIL_CONFIG", "")
	t.Chdir(project)

	projectCfg := filepath.Join(project, ".council", "config.toml")
	if err := os.MkdirAll(filepath.Dir(projectCfg), 0o755); err != nil {
		t.Fatalf("建项目层目录: %v", err)
	}
	if err := os.WriteFile(projectCfg, []byte("data_dir = '"+project+"'\n"), 0o644); err != nil {
		t.Fatalf("写项目层配置: %v", err)
	}

	out, errOut, code := initRun(t, "", "--json")
	if code != exitOK {
		t.Fatalf("init --json 退出码 = %d，stderr=%s", code, errOut)
	}
	var report initReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("init --json 输出不是合法 JSON: %v\n%s", err, out)
	}
	want := filepath.Join(home, ".council", "config.toml")
	if filepath.Clean(report.ConfigPath) != filepath.Clean(want) {
		t.Fatalf("init 目标 = %q，期望用户配置 %q（不是项目层 %q）", report.ConfigPath, want, projectCfg)
	}
}
