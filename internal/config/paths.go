package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveConfigPath 按 flagValue -> 环境变量 COUNCIL_CONFIG -> ~/.council/config.toml 取值。
// 空字符串视为未设置，继续向下回落。返回的路径不保证文件存在。
// 取用户主目录失败时返回带上下文的错误。
func ResolveConfigPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if env := os.Getenv(EnvConfigPath); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve default config path: user home dir: %w", err)
	}
	return filepath.Join(home, ".council", "config.toml"), nil
}

// ResolveDataDir 按 cfgDataDir -> 环境变量 COUNCIL_DATA_DIR -> DefaultDataDir 取值。
// 空字符串视为未设置，继续向下回落。取到的值必须是绝对路径，否则返回错误。
// 错误信息会写明是哪一级来源给出了该相对路径。本函数不创建目录。
func ResolveDataDir(cfgDataDir string) (string, error) {
	value, source := DefaultDataDir, "DefaultDataDir"
	if cfgDataDir != "" {
		value = cfgDataDir
		source = "config data_dir"
	} else if env := os.Getenv(EnvDataDir); env != "" {
		value = env
		source = "environment variable " + EnvDataDir
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("data dir from %s is not an absolute path: %q", source, value)
	}
	return value, nil
}
