package main

// P2：init 的相对 -config 与 data_dir 都应当归一成绝对路径；文本模式不能
// 对不存在的配置文件也说「已存在」。

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRelativeConfigUsesAbsoluteDataDir(t *testing.T) {
	initTestEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)

	out, errOut, code := initRun(t, "", "--config", "my.toml", "--json")
	if code != exitOK {
		t.Fatalf("init --json 退出码 = %d，stderr=%s", code, errOut)
	}
	var report initReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if !filepath.IsAbs(report.ConfigPath) {
		t.Errorf("ConfigPath = %q，期望绝对路径", report.ConfigPath)
	}
	if !filepath.IsAbs(report.DataDir) {
		t.Errorf("DataDir = %q，期望绝对路径（相对路径会让 init --apply 校验失败）", report.DataDir)
	}
}

func TestInitTextReportsMissingConfig(t *testing.T) {
	initTestEnv(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "missing.toml")
	out, _, code := initRun(t, "", "--config", target)
	if code != exitOK {
		t.Fatalf("init 退出码 = %d", code)
	}
	if !strings.Contains(out, "不存在") {
		t.Errorf("不存在的配置文件应显示「不存在」，实际:\n%s", out)
	}
	if strings.Contains(out, "已存在") {
		t.Errorf("不存在的配置文件仍显示「已存在」:\n%s", out)
	}
}
