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
var providerSectionKeys = []string{"command"}

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
	want := "command = " + tomlCommandString(provider.Command)

	start, end, ok := findProviderSection(lines, id)
	if !ok {
		block := renderProviderSection(id, provider, eol)
		return []byte(appendAgentBlock(string(content), block, eol)), true, nil
	}

	body := make([]fileLine, 0, end-start+2)
	handled := false
	for i := start + 1; i < end; i++ {
		if !handled && isKeyLine(lines[i].text, "command") {
			stmtEnd := statementEnd(lines, i, end)
			handled = true
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
			continue
		}
		body = append(body, lines[i])
	}
	if !handled {
		at := len(body)
		for at > 0 && isBlankLine(body[at-1]) {
			at--
		}
		if at > 0 && body[at-1].eol == "" {
			body[at-1].eol = eol
		}
		merged := make([]fileLine, 0, len(body)+1)
		merged = append(merged, body[:at]...)
		merged = append(merged, fileLine{text: want, eol: eol})
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
		value := provider.Command
		if key == "command" {
			value = provider.Command
		}
		b.WriteString(key + " = " + tomlCommandString(value) + eol)
	}
	return b.String()
}
