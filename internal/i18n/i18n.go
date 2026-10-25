// Package i18n 是中英文支持的机制层：语言解析、消息目录与查表。
//
// 本片只落地机制与 Web 界面文案；REPL / TUI / CLI 的文案是后续片。
// 目录里的 key 目前全部以 "ui." 开头，/api/i18n 只交出这一段：服务端
// 错误信息之类不属于界面文案的 key 将来会放在别的前缀下，不会跟着
// 一起进浏览器。
package i18n

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Lang 是具体语言。它只取 zh / en 两个值；"auto" 是配置与 flag 里的
// 占位值，由 Resolve 解析成具体语言，不会出现在 Lang 里。
type Lang string

const (
	LangZh Lang = "zh"
	LangEn Lang = "en"
)

// Default 是解析不到任何有效输入时的语言。
//
// 选中文而不是英文：Council 的作者与现有界面都是中文，首次运行的老用户
// 不应因为系统里恰好有个 LANG=en_US 就整页变成英文。想要英文的人显式
// 选一次，之后配置里会记住。
const Default = LangZh

// EnvLang 是覆盖配置语言的进程环境变量。优先级低于命令行 flag、高于
// 配置文件：临时跑一次 `set COUNCIL_LANG=en && council serve` 比改配置快。
const EnvLang = "COUNCIL_LANG"

// Auto 是配置与 flag 共用的「跟随下一级」值。用字符串常量而不是 Lang，
// 因为它不是一种具体语言，不能混进 Resolve 的返回值。
const Auto = "auto"

// Resolve 按优先级求本次进程界面使用的语言：
//
//	flagValue > 环境变量 COUNCIL_LANG > configValue > 系统区域 > 中文
//
// 每一级都按同一套规则解释：
//   - 空串或 "auto"：这一级没有决定，继续看下一级；
//   - "zh" / "en"（大小写不敏感）：返回它；
//   - 其它值：非法输入，按默认语言处理，不再往下看。
//
// 最后一级系统区域只读 LC_ALL / LC_MESSAGES / LANG 这三个标准环境变量，
// 不引入平台 API、不加依赖：Windows 上它们通常为空，于是回到中文；
// Unix / Git Bash 下 LANG=en_US.UTF-8 会被认成英文。C / POSIX 视为
// 「没有区域信息」，继续看下一个变量。
func Resolve(flagValue, configValue string) Lang {
	for _, value := range []string{flagValue, os.Getenv(EnvLang), configValue} {
		switch normalize(value) {
		case "":
			continue
		case Auto:
			continue
		case string(LangZh):
			return LangZh
		case string(LangEn):
			return LangEn
		default:
			return Default
		}
	}
	return systemLang()
}

// normalize 把输入折成小写并去掉首尾空白。TOML 与 PowerShell 都容易带
// 进空格或大写，这里统一一次，调用方不必各自 Trim。
func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// systemLang 读标准区域环境变量。LC_ALL 覆盖 LC_MESSAGES，后者覆盖 LANG。
func systemLang() Lang {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		raw := strings.TrimSpace(os.Getenv(name))
		if raw == "" {
			continue
		}
		base := localeBase(raw)
		switch base {
		case "en":
			return LangEn
		case "zh":
			return LangZh
		case "", "c", "posix":
			// C / POSIX 不携带语言偏好，继续看下一个变量。
			continue
		default:
			// 明确给了别的语言（fr_FR 等）：Council 没有那套文案，
			// 按默认处理，不再被后面的 LANG 覆盖。
			return Default
		}
	}
	return Default
}

// localeBase 把 en_US.UTF-8 / zh-CN / en_GB@modifier 折成主语言标签。
func localeBase(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.IndexAny(value, ":@"); i >= 0 {
		value = value[:i]
	}
	if i := strings.IndexByte(value, '.'); i >= 0 {
		value = value[:i]
	}
	if i := strings.IndexAny(value, "_-"); i >= 0 {
		value = value[:i]
	}
	return value
}

// Catalog 是一份「key -> 文案」的消息目录。
type Catalog map[string]string

// T 查一条文案并做可选格式化。args 为空时原样返回；缺失 key 时返回 key
// 本身，绝不 panic —— 一条漏翻的文案不该让页面或进程崩掉。
//
// 传给 args 的格式化串沿用 fmt 的动词（Go 侧）；Web 目录里的文案用的是
// {name} 占位符，由前端 t() 自己替换，Go 侧不会拿它们去 Sprintf。
func T(lang Lang, key string, args ...any) string {
	text, ok := catalogFor(lang)[key]
	if !ok {
		return key
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

// Keys 返回中英两份目录的 key 并集（已排序）。
//
// 它存在的唯一理由是让测试能断言两份目录的 key 集合完全一致：只返回中文
// 目录的 key 时，英文多出来的一条永远测不到。生产代码不需要它。
func Keys() []string {
	seen := make(map[string]struct{}, len(zh)+len(en))
	for key := range zh {
		seen[key] = struct{}{}
	}
	for key := range en {
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// UIMessages 返回某一语言里所有界面文案的副本。只交出 "ui." 前缀的 key：
// /api/i18n 是浏览器拿到的完整表，服务端错误文案不能混进去。
func UIMessages(lang Lang) map[string]string {
	source := catalogFor(lang)
	out := make(map[string]string, len(source))
	for key, text := range source {
		if strings.HasPrefix(key, "ui.") {
			out[key] = text
		}
	}
	return out
}

func catalogFor(lang Lang) Catalog {
	if lang == LangEn {
		return en
	}
	return zh
}
