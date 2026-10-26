package main

// lang.go 是交互入口的 -lang 落点。本片 REPL / TUI 的文案不翻译，但
// 命令行的解析机制要先就位：
//
//   - flag 与环境变量在 i18n.Resolve 里已经能决定，只有 flag / 环境变量
//     都没给出具体语言时才需要读配置里的 language；
//   - 求出的最终语言写回 COUNCIL_LANG，后续片（以及任何读取这个变量的
//     子进程）拿到的是同一次解析结果，而不是各自再排一遍优先级。
//
// 读配置是只读、失败即忽略的：配置缺失（首次运行）或写坏由 app.Run
// 照常报错；语言的解析不该抢先失败，否则一个拼错的配置会变成一句
// 与配置无关的启动错误。

import (
	"os"
	"strings"

	"council/internal/config"
	"council/internal/i18n"
)

// prescanFlag 在 flag.Parse 之前取一个 flag 的值。
//
// 存在的理由：-lang 决定的是「flag 说明本身用哪种语言写」，而 -h 在
// fs.Parse 里就返回 ErrHelp，那时已经来不及按解析结果重建 FlagSet。
// 所以先按命令行上写的值求一次语言，用来构造 FlagSet；解析完成后
// applyLanguageFlag 会按正式结果再写一次 COUNCIL_LANG，两者通常相同。
//
// 只认 -name value 与 -name=value 两种写法（单个或两个连字符），遇到
// "--" 或第一个非 flag 参数就停：那之后是位置参数，不是 flag。
func prescanFlag(args []string, name string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return ""
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return ""
		}
		trimmed := strings.TrimLeft(arg, "-")
		if trimmed == name {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if value, ok := strings.CutPrefix(trimmed, name+"="); ok {
			return value
		}
	}
	return ""
}

// explicitLang 判断一个值是不是明确的 zh / en。auto、空串与乱写都不算。
func explicitLang(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(i18n.LangZh), string(i18n.LangEn):
		return true
	default:
		return false
	}
}

// cliLanguage 按 i18n.Resolve 的四级顺序求语言。只在前面几级都没有明确
// 取值时才读配置文件：一个显式的 -lang en 不该为了一个用不到的值去碰磁盘。
func cliLanguage(flagValue, configPath string) i18n.Lang {
	configValue := ""
	if !explicitLang(flagValue) && !explicitLang(os.Getenv(i18n.EnvLang)) {
		configValue = readConfigLanguage(configPath)
	}
	return i18n.Resolve(flagValue, configValue)
}

// readConfigLanguage 只读地取合并配置里的 language。任何一步失败都返回
// 空串（= 没有配置值），让 Resolve 继续看系统区域。
func readConfigLanguage(configPath string) string {
	layers, err := config.ResolveConfigLayers(configPath)
	if err != nil || len(layers) == 0 {
		return ""
	}
	if _, err := os.Stat(layers[0].Path); err != nil {
		return ""
	}
	merged, err := config.LoadMerged(layers)
	if err != nil || merged.Config == nil {
		return ""
	}
	return merged.Config.Language
}

// applyLanguageFlag 把解析结果固定进进程环境。返回最终语言是给测试用的；
// 生产调用方不需要它。
func applyLanguageFlag(flagValue, configPath string) i18n.Lang {
	lang := cliLanguage(flagValue, configPath)
	// 设置失败只可能发生在变量名非法，而 EnvLang 是常量。忽略返回值，
	// 不让一个纯粹的环境变量写入失败挡住启动。
	_ = os.Setenv(i18n.EnvLang, string(lang))
	return lang
}
