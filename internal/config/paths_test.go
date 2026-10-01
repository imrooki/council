package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// tstPinPathEnv 把两个路径相关的环境变量钉成给定值。
// 开发机上可能已经设了 COUNCIL_CONFIG / COUNCIL_DATA_DIR，不钉死会让用例随机漂移。
// t.Setenv 在子测试结束时自动还原，不要改成 os.Setenv。
func tstPinPathEnv(t *testing.T, configPath, dataDir string) {
	t.Helper()
	t.Setenv(EnvConfigPath, configPath)
	t.Setenv(EnvDataDir, dataDir)
}

// tstUnsetEnv 先用 t.Setenv 登记还原，再把变量真正取消设置。
// 这样可以区分「变量未设置」与「变量是空串」两种情形，同时保留自动还原。
func tstUnsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "placeholder")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("取消环境变量 %s 失败: %v", key, err)
	}
}

// tstFakeHome 把主目录指向一个临时目录，并返回该目录。
// Windows 上 os.UserHomeDir 读 USERPROFILE，类 Unix 上读 HOME，两个都设。
func tstFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	return home
}

func TestResolveConfigPath(t *testing.T) {
	tests := []struct {
		name string
		// setup 负责布置环境，返回传给 ResolveConfigPath 的 flag 值与期望结果。
		setup func(t *testing.T) (flagValue, want string)
	}{
		{
			name: "flag 优先级最高",
			setup: func(t *testing.T) (string, string) {
				tstFakeHome(t)
				tstPinPathEnv(t, "", "")
				p := filepath.Join(t.TempDir(), "from-flag.toml")
				return p, p
			},
		},
		{
			name: "flag 为空时取环境变量 COUNCIL_CONFIG",
			setup: func(t *testing.T) (string, string) {
				tstFakeHome(t)
				p := filepath.Join(t.TempDir(), "from-env.toml")
				tstPinPathEnv(t, p, "")
				return "", p
			},
		},
		{
			name: "flag 与环境变量都无时取主目录默认",
			setup: func(t *testing.T) (string, string) {
				home := tstFakeHome(t)
				tstUnsetEnv(t, EnvConfigPath)
				return "", filepath.Join(home, ".council", "config.toml")
			},
		},
		{
			name: "flag 覆盖环境变量",
			setup: func(t *testing.T) (string, string) {
				tstFakeHome(t)
				envPath := filepath.Join(t.TempDir(), "from-env.toml")
				tstPinPathEnv(t, envPath, "")
				flagPath := filepath.Join(t.TempDir(), "from-flag.toml")
				return flagPath, flagPath
			},
		},
		{
			name: "环境变量为空串时回落主目录默认而不是被空串吃掉",
			setup: func(t *testing.T) (string, string) {
				home := tstFakeHome(t)
				tstPinPathEnv(t, "", "")
				return "", filepath.Join(home, ".council", "config.toml")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flagValue, want := tc.setup(t)
			got, err := ResolveConfigPath(flagValue)
			if err != nil {
				t.Fatalf("ResolveConfigPath(%q) 返回错误: %v", flagValue, err)
			}
			if got != want {
				t.Errorf("ResolveConfigPath(%q) = %q，期望 %q", flagValue, got, want)
			}
		})
	}
}

func TestResolveDataDir(t *testing.T) {
	tests := []struct {
		name string
		// setup 返回传给 ResolveDataDir 的配置值与期望结果。
		setup func(t *testing.T) (cfgDataDir, want string)
		// fromDefault 标出期望值取自 DefaultDataDir 的两条。
		fromDefault bool
	}{
		{
			name: "配置里的 data_dir 优先级最高",
			setup: func(t *testing.T) (string, string) {
				envDir := t.TempDir()
				tstPinPathEnv(t, "", envDir)
				cfgDir := t.TempDir()
				return cfgDir, cfgDir
			},
		},
		{
			name: "配置为空时取环境变量 COUNCIL_DATA_DIR",
			setup: func(t *testing.T) (string, string) {
				envDir := t.TempDir()
				tstPinPathEnv(t, "", envDir)
				return "", envDir
			},
		},
		{
			name:        "配置与环境变量都无时取 DefaultDataDir",
			fromDefault: true,
			setup: func(t *testing.T) (string, string) {
				tstUnsetEnv(t, EnvDataDir)
				return "", DefaultDataDir
			},
		},
		{
			name:        "环境变量为空串时回落 DefaultDataDir 而不是被空串吃掉",
			fromDefault: true,
			setup: func(t *testing.T) (string, string) {
				tstPinPathEnv(t, "", "")
				return "", DefaultDataDir
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfgDataDir, want := tc.setup(t)
			got, err := ResolveDataDir(cfgDataDir)
			if tc.fromDefault && !filepath.IsAbs(DefaultDataDir) {
				// DefaultDataDir 是 D-9 给那台 Windows 机器定的值，别的平台上它不是绝对路径：
				// 这时没有默认数据目录，必须响亮失败并点名来源，而不是拿一个相对路径去建库。
				// Windows 上走不到这一支，下面的断言与原来逐字相同。
				// 连带钉住返回值为空、错误里带着默认值原文：只查「DefaultDataDir」几个字的话，
				// 空串环境变量没回落、默认值被改成别的都照样绿（2026-09-25 验证者实测四个变异活下来）。
				if err == nil || got != "" ||
					!strings.Contains(err.Error(), "DefaultDataDir") ||
					!strings.Contains(err.Error(), strconv.Quote(DefaultDataDir)) {
					t.Fatalf("非 Windows 上 ResolveDataDir(%q) 应当返回空串、报错并点名 DefaultDataDir 与它的值 %q，实际 got=%q err=%v",
						cfgDataDir, DefaultDataDir, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveDataDir(%q) 返回错误: %v", cfgDataDir, err)
			}
			if got != want {
				t.Errorf("ResolveDataDir(%q) = %q，期望 %q", cfgDataDir, got, want)
			}
			if !filepath.IsAbs(got) {
				t.Errorf("ResolveDataDir(%q) 返回的 %q 不是绝对路径", cfgDataDir, got)
			}
		})
	}
}

// TestResolveDataDirRejectsRelative 覆盖规格 C：data_dir 必须是绝对路径。
// 两个来源各一个负例 —— 来自配置文件的值与来自环境变量的值。
func TestResolveDataDirRejectsRelative(t *testing.T) {
	tests := []struct {
		name       string
		cfgDataDir string
		envDataDir string
	}{
		{
			name:       "配置来源的相对路径报错",
			cfgDataDir: filepath.Join("relative", "council-data"),
			envDataDir: "",
		},
		{
			name:       "环境变量来源的相对路径报错",
			cfgDataDir: "",
			envDataDir: filepath.Join("relative", "council-data"),
		},
		{
			name:       "配置来源的当前目录相对路径报错",
			cfgDataDir: "council-data",
			envDataDir: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tstPinPathEnv(t, "", tc.envDataDir)
			if _, err := ResolveDataDir(tc.cfgDataDir); err == nil {
				t.Fatalf("ResolveDataDir(%q) 在 %s=%q 时期望报错，实际返回 nil",
					tc.cfgDataDir, EnvDataDir, tc.envDataDir)
			}
		})
	}
}
