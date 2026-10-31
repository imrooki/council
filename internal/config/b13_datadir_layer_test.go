package config

// B13：WriteDataDir 不能写进项目层；项目层的 data_dir 在合并时被忽略。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestB13WriteDataDirSkipsProjectLayer(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project.toml")
	global := filepath.Join(dir, "global.toml")
	body := "[providers.claude]\ncommand = 'claude'\n"
	if err := os.WriteFile(project, []byte(body), 0o644); err != nil {
		t.Fatalf("写项目层: %v", err)
	}
	if err := os.WriteFile(global, []byte(body), 0o644); err != nil {
		t.Fatalf("写全局层: %v", err)
	}
	layers := []Layer{{Path: project, Project: true}, {Path: global}}
	target := filepath.Join(dir, "data-from-settings")

	res, err := WriteDataDir(layers, target)
	if err != nil {
		t.Fatalf("WriteDataDir: %v", err)
	}
	if res.WriteTarget != global {
		t.Errorf("WriteTarget = %q，期望全局层 %q", res.WriteTarget, global)
	}
	if got, err := os.ReadFile(project); err != nil || strings.Contains(string(got), "data_dir") {
		t.Errorf("项目层被写了 data_dir: %v\n%s", err, got)
	}
	if got, err := os.ReadFile(global); err != nil || !strings.Contains(string(got), "data_dir") {
		t.Errorf("全局层没有写到 data_dir: %v\n%s", err, got)
	}

	merged, err := LoadMerged(layers)
	if err != nil {
		t.Fatalf("LoadMerged: %v", err)
	}
	if merged.Config.DataDir != target {
		t.Errorf("合并后的 data_dir = %q，期望 %q", merged.Config.DataDir, target)
	}
}

func TestB13WriteDataDirRefusesProjectOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.toml")
	if err := os.WriteFile(path, []byte("[providers.claude]\ncommand = 'claude'\n"), 0o644); err != nil {
		t.Fatalf("写项目层: %v", err)
	}
	_, err := WriteDataDir([]Layer{{Path: path, Project: true}}, filepath.Join(t.TempDir(), "data"))
	if !errors.Is(err, ErrNoTrustedWriteLayer) {
		t.Fatalf("只有项目层时应返回 ErrNoTrustedWriteLayer，实际: %v", err)
	}
}
