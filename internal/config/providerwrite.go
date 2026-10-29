package config

// providerwrite.go 让设置页能改 providers —— 与 agentwrite.go 同一套纪律：
// 只写 layers[0]、纯字节编辑保住其余段与注释、先用临时文件合并校验、
// 通过之后才备份并覆盖。区别只有一个：provider 段里只有一个键 command。

import (
	"errors"
	"strings"
)

// ErrProviderSectionMissing 表示目标文件里没有 [providers.<id>] 这一段。
// 与 ErrAgentSectionMissing 同一个用途：删除时用来区分「这一段不在我能写
// 的那份文件里」和「写盘失败」。
var ErrProviderSectionMissing = errors.New("settings: provider section not found in write target")

// providerSectionKeys 是 provider 段里设置页会改的键。顺序固定，理由同
// agentSectionKeys：同一份配置改两次不该出现不同字节。
var providerSectionKeys = []string{"command", "args", "env", "models"}

// WriteProviderSection 把 [providers.<id>] 写进 layers[0]：已有这一段就
// 原地改 command，没有就在文件末尾追加一段。
//
// 校验、备份、替换的顺序与 WriteAgentSection 完全一致（见那里的注释）：
// 新内容过不了 LoadMerged + Validate 时，目标文件一个字节都不会动。
func WriteProviderSection(layers []Layer, id string, provider ProviderConfig) (AgentWriteResult, error) {
	return writeSettingsFile(layers, ErrProviderSectionMissing, func(orig []byte) ([]byte, bool, error) {
		return upsertProviderSection(orig, id, provider)
	})
}

// RemoveProviderSection 从 layers[0] 删掉 [providers.<id>] 这一段。
//
// 文件里本来没有这一段（provider 继承自低优先级层）时返回
// ErrProviderSectionMissing；删掉之后若还有 agent 引用它，Validate 会拒绝，
// 返回 InvalidConfigError，文件不变 —— 调用方应当先把引用列给用户看。
func RemoveProviderSection(layers []Layer, id string) (AgentWriteResult, error) {
	return writeSettingsFile(layers, ErrProviderSectionMissing, func(orig []byte) ([]byte, bool, error) {
		return removeProviderSection(orig, id)
	})
}

// ProviderDefinedIn 返回定义了 provider id 的配置层路径（高优先级在前）。
// 与 AgentDefinedIn 同义：删除被拒时用来告诉用户该去改哪份文件。
func ProviderDefinedIn(layers []Layer, id string) []string {
	var out []string
	for _, layer := range layers {
		if layer.Path == "" {
			continue
		}
		var cfg Config
		if _, err := decodeConfigFile(layer.Path, &cfg); err != nil {
			continue
		}
		if _, ok := cfg.Providers[id]; ok {
			out = append(out, layer.Path)
		}
	}
	return out
}

// EnvEntryProblem 返回一条 env 条目的问题（空串表示合法）。
//
// 形状固定是 NAME=VALUE：NAME 只能是字母、数字、下划线，且不以数字开头。
// 这条规则不是为了好看——Windows 的 CreateProcess 拿到一条没有 '=' 的条目
// 会把它当成「继承这个变量名」，与用户想表达的「设一个值」完全不同，而
// 这种差异在进程里看不出来，只会体现成「设了却没生效」。
func EnvEntryProblem(entry string) string {
	if entry == "" {
		return "must not be empty"
	}
	eq := strings.IndexByte(entry, '=')
	if eq <= 0 {
		return "must look like NAME=VALUE"
	}
	name := entry[:eq]
	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (i > 0 && c >= '0' && c <= '9')
		if !ok {
			return "variable name may only contain letters, digits and _, and must not start with a digit"
		}
	}
	if strings.ContainsRune(entry, 0) {
		return "must not contain NUL"
	}
	return ""
}

// ProviderIDProblem 返回 provider id 不合规的原因（空串表示没问题）。
//
// 与 agent id 用同一条 idPattern：它要成为 TOML 表名 [providers.<id>]，
// 也是 [agents.x].provider 里的取值。这里不做 agent 那套「Windows 设备名 /
// 保留名」检查——provider id 不会变成目录名，那些约束在它身上没有意义。
func ProviderIDProblem(id string) string {
	if id == "" {
		return "provider id 不能为空"
	}
	if !idRegexp.MatchString(id) {
		return "provider id 只能用 a-z、0-9、_ . -，且不能以 . 或 - 结尾"
	}
	return ""
}

// findProviderSection 找 [providers.<id>] 段：返回段头行号与段尾（下一段
// 表头之前，或文件末尾）。
func findProviderSection(lines []fileLine, id string) (start, end int, ok bool) {
	start = -1
	for i, line := range lines {
		path, isHeader := parseTableHeader(line.text)
		if !isHeader {
			continue
		}
		if start < 0 {
			if len(path) == 2 && path[0] == "providers" && path[1] == id {
				start = i
			}
			continue
		}
		return start, i, true
	}
	if start < 0 {
		return 0, 0, false
	}
	return start, len(lines), true
}

// upsertProviderSection 是改段 / 追加段的纯字节实现。
func upsertProviderSection(content []byte, id string, provider ProviderConfig) ([]byte, bool, error) {
	lines := splitFileLines(string(content))
	eol := dominantEOL(lines)

	start, end, ok := findProviderSection(lines, id)
	if !ok {
		block := renderProviderSection(id, provider, eol)
		return []byte(appendAgentBlock(string(content), block, eol)), true, nil
	}

	body := make([]fileLine, 0, end-start+2)
	handled := map[string]bool{}
	for i := start + 1; i < end; i++ {
		key := ""
		for _, candidate := range providerSectionKeys {
			if isKeyLine(lines[i].text, candidate) {
				key = candidate
				break
			}
		}
		if key == "" {
			body = append(body, lines[i])
			continue
		}
		stmtEnd := statementEnd(lines, i, end)
		handled[key] = true
		value, keep := providerKeyValue(provider, key)
		if !keep {
			// 空数组不写进文件：args = [] 与「没有这一行」等价，留在文件里
			// 只会让用户以为设了什么。
			i = stmtEnd
			continue
		}
		want := key + " = " + value
		replacement := fileLine{text: want, eol: lines[i].eol}
		if replacement.eol == "" {
			replacement.eol = eol
		}
		// 行尾注释是用户写的说明，换值时接回去（与 agent 段同一处理）。
		if stmtEnd == i {
			if comment := trailingComment(lines[i].text); comment != "" {
				replacement.text = want + "  " + comment
			}
		}
		body = append(body, replacement)
		i = stmtEnd
	}

	var missing []fileLine
	for _, key := range providerSectionKeys {
		if handled[key] {
			continue
		}
		value, keep := providerKeyValue(provider, key)
		if !keep {
			continue
		}
		missing = append(missing, fileLine{text: key + " = " + value, eol: eol})
	}
	if len(missing) > 0 {
		at := len(body)
		for at > 0 && isBlankLine(body[at-1]) {
			at--
		}
		if at > 0 && body[at-1].eol == "" {
			body[at-1].eol = eol
		}
		merged := make([]fileLine, 0, len(body)+len(missing))
		merged = append(merged, body[:at]...)
		merged = append(merged, missing...)
		merged = append(merged, body[at:]...)
		body = merged
	}

	out := make([]fileLine, 0, start+1+len(body)+len(lines)-end)
	out = append(out, lines[:start+1]...)
	if len(body) > 0 && out[len(out)-1].eol == "" {
		out[len(out)-1].eol = eol
	}
	out = append(out, body...)
	out = append(out, lines[end:]...)
	return []byte(joinFileLines(out)), true, nil
}

// providerKeyValue 把 ProviderConfig 里的一个键渲染成 TOML 值文本。
// keep=false 表示这个键不该出现在文件里（空 args / 空 env）。
func providerKeyValue(provider ProviderConfig, key string) (value string, keep bool) {
	switch key {
	case "command":
		return tomlCommandString(provider.Command), true
	case "args":
		if len(provider.Args) == 0 {
			return "", false
		}
		return tomlStringArray(provider.Args), true
	case "env":
		if len(provider.Env) == 0 {
			return "", false
		}
		return tomlStringArray(provider.Env), true
	case "models":
		if len(provider.Models) == 0 {
			return "", false
		}
		return tomlStringArray(provider.Models), true
	}
	return "", false
}

// tomlStringArray 渲染 ["a", "b"]。元素用 tomlCommandString 的同一套规则：
// 能单引号就单引号（Windows 路径里的反斜杠不用转义），否则退回基本字符串。
func tomlStringArray(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, tomlCommandString(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// removeProviderSection 删掉整段。缩进写在段头之前的注释行不归这一段管，
// 原样保留。
func removeProviderSection(content []byte, id string) ([]byte, bool, error) {
	lines := splitFileLines(string(content))
	start, end, ok := findProviderSection(lines, id)
	if !ok {
		return nil, false, nil
	}
	out := make([]fileLine, 0, len(lines)-(end-start))
	out = append(out, lines[:start]...)
	out = append(out, lines[end:]...)
	return []byte(joinFileLines(out)), true, nil
}

// renderProviderSection 渲染一个完整的新段。id 含点号时表名加引号，与
// ProviderBlockTOML 的写法一致。
func renderProviderSection(id string, provider ProviderConfig, eol string) string {
	table := "providers." + id
	if strings.Contains(id, ".") {
		table = `providers."` + id + `"`
	}
	var b strings.Builder
	b.WriteString("[" + table + "]" + eol)
	for _, key := range providerSectionKeys {
		value, keep := providerKeyValue(provider, key)
		if !keep {
			continue
		}
		b.WriteString(key + " = " + value + eol)
	}
	return b.String()
}
