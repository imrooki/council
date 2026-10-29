package config

// langwrite.go 是设置页「语言」那一行的写盘实现。它与 agentwrite.go 共用
// 同一条路径：只写 layers[0]，先算新内容、逐字节比较、临时文件 + 合并校验、
// 备份、Rename；目标文件不存在、写后校验失败的错误类型与文案也完全一致。
//
// 与 agent 段不同的是，language 是顶层键，在 TOML 里必须写在所有表之前。
// 所以这里不能像 agent 段那样追加到文件末尾（那会把它写进最后一个
// [agents.x] 表里），而是：已有顶层 language 就原地替换；没有就插在第一个
// 表头之前。顶层键、注释与行尾的字节保持不变。
//
// local_agent 走的是同一个 setTopLevelKey（见 localagentwrite.go）：两个键
// 在 TOML 里的位置规则一模一样，复制一份只会让「插在哪一行」出现两种实现。

import (
	"errors"
	"fmt"
	"strings"
)

// LanguageValue 把界面语言归一成写进配置文件的字面值。空串按 auto：
// 与 Load 的默认值同一条规则，避免文件里出现 language = "" 这种既非
// 默认又过不了 Validate 的写法。
func LanguageValue(language string) string {
	value := strings.ToLower(strings.TrimSpace(language))
	if value == "" {
		return LanguageAuto
	}
	return value
}

// WriteLanguage 把 language 写进 layers[0] 并返回与 agent 写盘相同的结果
// 形状（AgentWriteResult 是设置页写盘唯一的结果类型，复用它而不是再造
// 一个只差名字的结构）。
func WriteLanguage(layers []Layer, language string) (AgentWriteResult, error) {
	value := LanguageValue(language)
	return writeSettingsFile(layers, errors.New("settings: 语言键不在写目标里"), func(orig []byte) ([]byte, bool, error) {
		return setTopLevelKey(orig, "language", value)
	})
}

// setTopLevelKey 是纯字节实现：只碰 key 这一行，其余行原样保留。
//
// key 只允许简单标识符。当前的调用方传的都是包内常量，这道检查是给以后
// 加键的人留的护栏：key 会被拼进 `key = "value"`，拼进一个带引号、换行或
// 等号的字符串就等于让调用方往配置文件里注入任意字节。
func setTopLevelKey(content []byte, key, value string) ([]byte, bool, error) {
	if !validTopLevelKeyName(key) {
		return nil, false, fmt.Errorf("顶层键 %q 不是合法标识符", key)
	}
	lines := splitFileLines(string(content))
	eol := dominantEOL(lines)

	firstHeader := -1
	existing := -1
	for i, line := range lines {
		if _, isHeader := parseTableHeader(line.text); isHeader {
			firstHeader = i
			break
		}
		if isKeyLine(line.text, key) {
			existing = i
			break
		}
	}

	if existing >= 0 {
		limit := len(lines)
		if firstHeader >= 0 {
			limit = firstHeader
		}
		end := statementEnd(lines, existing, limit)
		replacement := fileLine{text: key + ` = ` + tomlBasicString(value), eol: lines[existing].eol}
		if replacement.eol == "" {
			replacement.eol = eol
		}
		if end == existing {
			if comment := trailingComment(lines[existing].text); comment != "" {
				replacement.text += "  " + comment
			}
		}
		out := make([]fileLine, 0, len(lines)-(end-existing))
		out = append(out, lines[:existing]...)
		out = append(out, replacement)
		out = append(out, lines[end+1:]...)
		return []byte(joinFileLines(out)), true, nil
	}

	keyLine := fileLine{text: key + ` = ` + tomlBasicString(value), eol: eol}
	if firstHeader >= 0 {
		out := make([]fileLine, 0, len(lines)+2)
		out = append(out, lines[:firstHeader]...)
		// 文件以表头开头时没有前文，直接插在最前面。
		if len(out) > 0 && out[len(out)-1].eol == "" {
			out[len(out)-1].eol = eol
		}
		out = append(out, keyLine)
		// 顶层键与表头之间空一行。即使原文表头前已经有一个空行也照样
		// 加：那个空行现在位于 language 之前，键与表头仍需要分隔。
		out = append(out, fileLine{eol: eol})
		out = append(out, lines[firstHeader:]...)
		return []byte(joinFileLines(out)), true, nil
	}

	// 没有表：顶层键可以安全地追加到末尾。原文最后一个字节是不是换行
	// 决定新行前面要不要补一个行尾，以及写完要不要补回换行。
	text := string(content)
	if text == "" {
		keyLine.eol = "\n"
		return []byte(joinFileLines([]fileLine{keyLine})), true, nil
	}
	if !strings.HasSuffix(text, "\n") {
		keyLine.eol = ""
		return []byte(text + eol + keyLine.text), true, nil
	}
	return []byte(text + keyLine.text + eol), true, nil
}

// validTopLevelKeyName 只放行 TOML 的裸键写法（ASCII 字母、数字、连字符、
// 下划线，且不以数字开头）。本项目自己要写的两个键都是这种形状。
func validTopLevelKeyName(key string) bool {
	if key == "" {
		return false
	}
	if key[0] >= '0' && key[0] <= '9' {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}
