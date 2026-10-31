package main

// P2：显式 -config / COUNCIL_CONFIG 指向不存在的文件时不能静默当成 off。

import (
	"path/filepath"
	"testing"
)

func TestExplicitMissingConfigIsAnError(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := loadWhoConfig(filepath.Join(dir, "missing.toml")); err == nil {
		t.Fatal("显式 -config 指向不存在的文件，loadWhoConfig 仍返回 off")
	}

	t.Setenv("COUNCIL_CONFIG", filepath.Join(dir, "also-missing.toml"))
	if _, _, err := loadWhoConfig(""); err == nil {
		t.Fatal("COUNCIL_CONFIG 指向不存在的文件，loadWhoConfig 仍返回 off")
	}
}

func TestMissingDefaultConfigStaysOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COUNCIL_CONFIG", "")
	if level, dataDir, err := loadWhoConfig(""); err != nil || level != "off" || dataDir != "" {
		t.Fatalf("没有配置的新机器应返回 off/空目录，实际 level=%q dataDir=%q err=%v", level, dataDir, err)
	}
}
