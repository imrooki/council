package config

// teamwrite.go 与 summarywrite.go 的服务对象是设置页那两处「只读展示」：
// teams 与 [summary] agent。写法与 agent / provider 段一致——只改目标这一段，
// 其它内容逐字节保留，写前校验、写前备份。

import (
	"errors"
	"strings"
)

// ErrTeamSectionMissing / ErrSummarySectionMissing 与 ErrAgentSectionMissing
// 同义：目标文件里没有这一段，删不掉（它定义在低优先级层）。
var (
	ErrTeamSectionMissing    = errors.New("settings: team section not found in write target")
	ErrSummarySectionMissing = errors.New("settings: summary section not found in write target")
)

// teamSectionKeys 是设置页会改的键。mode / thought_visible_to 这类更细的
// 字段不在这里：它们在文件里原样保留，不会被一次保存顺手改写。
var teamSectionKeys = []string{"display_name", "members"}

// WriteTeamSection 把 [teams.<id>] 写进 layers[0]。
func WriteTeamSection(layers []Layer, id string, team TeamConfig) (AgentWriteResult, error) {
	return writeSettingsFile(layers, ErrTeamSectionMissing, func(orig []byte) ([]byte, bool, error) {
		return upsertTeamSection(orig, id, team)
	})
}

// RemoveTeamSection 从 layers[0] 删掉 [teams.<id>]。被 team 引用的成员不受
// 影响——team 只是成员的一个名单，删名单不等于删成员；反过来，Validate 会
// 检查名单里的 id 都存在。
func RemoveTeamSection(layers []Layer, id string) (AgentWriteResult, error) {
	return writeSettingsFile(layers, ErrTeamSectionMissing, func(orig []byte) ([]byte, bool, error) {
		return removeTableSection(orig, "teams", id)
	})
}

// WriteSummaryAgent 设置 [summary] agent。id 为空表示关掉汇总：整张表删掉，
// 而不是留下 agent = ''。
func WriteSummaryAgent(layers []Layer, agentID string) (AgentWriteResult, error) {
	return writeSettingsFile(layers, ErrSummarySectionMissing, func(orig []byte) ([]byte, bool, error) {
		if strings.TrimSpace(agentID) == "" {
			return removeTableSection(orig, "summary", "")
		}
		return upsertSummarySection(orig, agentID)
	})
}

// WriteDataDir 改顶层 data_dir。与语言键同一条路（setTopLevelKey），
// 下次启动生效。
func WriteDataDir(layers []Layer, dir string) (AgentWriteResult, error) {
	return writeSettingsFile(layers, ErrWriteTargetMissing, func(orig []byte) ([]byte, bool, error) {
		return setTopLevelKey(orig, "data_dir", dir)
	})
}

func upsertTeamSection(content []byte, id string, team TeamConfig) ([]byte, bool, error) {
	lines := splitFileLines(string(content))
	eol := dominantEOL(lines)
	start, end, ok := findTableSection(lines, "teams", id)
	values := map[string]string{
		"display_name": tomlCommandString(team.DisplayName),
		"members":      tomlStringArray(team.Members),
	}
	if !ok {
		table := "teams." + id
		if strings.Contains(id, ".") {
			table = `teams."` + id + `"`
		}
		var b strings.Builder
		b.WriteString("[" + table + "]" + eol)
		for _, key := range teamSectionKeys {
			b.WriteString(key + " = " + values[key] + eol)
		}
		return []byte(appendAgentBlock(string(content), b.String(), eol)), true, nil
	}
	return []byte(joinFileLines(replaceSectionKeys(lines, start, end, teamSectionKeys, values, eol))), true, nil
}

func upsertSummarySection(content []byte, agentID string) ([]byte, bool, error) {
	lines := splitFileLines(string(content))
	eol := dominantEOL(lines)
	start, end, ok := findTableSection(lines, "summary", "")
	values := map[string]string{"agent": tomlCommandString(agentID)}
	if !ok {
		return []byte(appendAgentBlock(string(content), "[summary]"+eol+"agent = "+values["agent"]+eol, eol)), true, nil
	}
	return []byte(joinFileLines(replaceSectionKeys(lines, start, end, []string{"agent"}, values, eol))), true, nil
}

// findTableSection 找 [table.<id>]；id 为空时找 [table] 本身。
func findTableSection(lines []fileLine, table, id string) (start, end int, ok bool) {
	start = -1
	for i, line := range lines {
		path, isHeader := parseTableHeader(line.text)
		if !isHeader {
			continue
		}
		if start < 0 {
			match := false
			if id == "" {
				match = len(path) == 1 && path[0] == table
			} else {
				match = len(path) == 2 && path[0] == table && path[1] == id
			}
			if match {
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

// replaceSectionKeys 是「只改认识的键、其余行原样保留」的公共实现：
// 命中的键换成新值（保留行尾注释），没命中的键插到段尾，空值删掉。
func replaceSectionKeys(lines []fileLine, start, end int, keys []string, values map[string]string, eol string) []fileLine {
	body := make([]fileLine, 0, end-start+len(keys))
	handled := map[string]bool{}
	for i := start + 1; i < end; i++ {
		key := ""
		for _, candidate := range keys {
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
		want := key + " = " + values[key]
		replacement := fileLine{text: want, eol: lines[i].eol}
		if replacement.eol == "" {
			replacement.eol = eol
		}
		if stmtEnd == i {
			if comment := trailingComment(lines[i].text); comment != "" {
				replacement.text = want + "  " + comment
			}
		}
		body = append(body, replacement)
		i = stmtEnd
	}
	var missing []fileLine
	for _, key := range keys {
		if handled[key] {
			continue
		}
		missing = append(missing, fileLine{text: key + " = " + values[key], eol: eol})
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
	return out
}

// removeTableSection 删掉 [table.<id>]（id 为空时删 [table]）。删完若整段
// 只剩表头与空行，也一并删掉，免得留下一个空壳。
func removeTableSection(content []byte, table, id string) ([]byte, bool, error) {
	lines := splitFileLines(string(content))
	start, end, ok := findTableSection(lines, table, id)
	if !ok {
		return nil, false, nil
	}
	// 段里还有别的键（例如 summary 以外的字段）时不能整段删，只删我们认识的。
	if id == "" {
		keep := make([]fileLine, 0, end-start)
		keep = append(keep, lines[start])
		removed := false
		for i := start + 1; i < end; i++ {
			if isKeyLine(lines[i].text, "agent") {
				removed = true
				i = statementEnd(lines, i, end)
				continue
			}
			keep = append(keep, lines[i])
		}
		if !removed {
			return nil, false, nil
		}
		// 只剩表头与空行 → 整段删掉。
		onlyHeader := true
		for _, line := range keep[1:] {
			if !isBlankLine(line) {
				onlyHeader = false
				break
			}
		}
		if onlyHeader {
			out := append([]fileLine{}, lines[:start]...)
			out = append(out, lines[end:]...)
			return []byte(joinFileLines(out)), true, nil
		}
		out := append([]fileLine{}, lines[:start]...)
		out = append(out, keep...)
		out = append(out, lines[end:]...)
		return []byte(joinFileLines(out)), true, nil
	}
	out := make([]fileLine, 0, len(lines)-(end-start))
	out = append(out, lines[:start]...)
	out = append(out, lines[end:]...)
	return []byte(joinFileLines(out)), true, nil
}
