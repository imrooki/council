package main

// B8：release 在数据目录还没有库时不能建库、建目录、拿 council.lock。
// 先 OpenReadOnly 探测；只有库存在时才交给写入口。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/store"
)

func TestB8ReleaseWithoutDatabaseDoesNotCreateAnything(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfg := filepath.Join(dir, "config.toml")
	body := "data_dir = " + `"` + strings.ReplaceAll(dataDir, `\`, `\\`) + `"` + "\n" +
		"[providers.claude]\ncommand = 'claude'\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatalf("写配置: %v", err)
	}

	deps := leaseDeps{
		Open: func(context.Context, string) (*store.Store, error) {
			t.Fatal("库不存在时 release 调用了写入口（会建库/拿锁）")
			return nil, nil
		},
	}
	out, errOut, code := releaseRun(t, deps, "-config", cfg, "-lease", "lease-does-not-exist")
	if code != exitOK {
		t.Fatalf("没有库时 release 应正常退出，code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "还没有库") {
		t.Errorf("提示里没有说明库不存在: %s", out)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Errorf("release 建出了 data_dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, lockFileNameForTest())); !os.IsNotExist(err) {
		t.Errorf("release 碰了 council.lock: %v", err)
	}
}

func TestB8ReleaseWithoutDataDirReportsConfigError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COUNCIL_CONFIG", "")
	_, errOut, code := releaseRun(t, leaseDeps{Open: store.Open}, "-lease", "x")
	if code != exitConfigFailed {
		t.Fatalf("没有 data_dir 时 code=%d，期望 %d；stderr=%s", code, exitConfigFailed, errOut)
	}
	if !strings.Contains(errOut, "data_dir") {
		t.Errorf("错误里没有点名 data_dir: %s", errOut)
	}
}

// lockFileNameForTest 只用于断言文件不存在；store 的常量不在 main 包导出。
func lockFileNameForTest() string { return "council.lock" }
