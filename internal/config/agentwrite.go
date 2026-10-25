package config

// agentwrite.go 是设置页（GUI 路线第 6 片）写配置文件的实现。它只做一件事：
// 在**目标文件**里增 / 改 / 删 [agents.<id>] 这一段，其余字节一个不动，
// 绝不把合并后的配置整体落盘。
//
// 为什么要按段改而不是整体重写：合并结果是「项目层 + 全局层 + Claude 导入」
// 的并集。整体落盘会把全局层的成员复制进项目文件、把注释与单引号写法全部
// 抹掉，还会把 data_dir 这种只该出现在一层里的键写进错误的那一层。/add 早期
// 踩过这个坑（docs/11d 与 docs/00_DECISIONS.md #45），这里用「按行改段」
// 从结构上避免它：改动范围永远只有一行表头与它下面的键值行。
//
// 与 append.go 的分工：那里是 /add 的「只追加一段」，本文件是设置页的
// 「原地改 / 删 / 追加」。两者共用同一套备份命名规则（不同后缀）、同一套
// 临时文件 + Rename 的原子替换，以及同一个 config.Validate。

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrWriteTargetMissing 表示写入目标 layers[0].Path 不存在。设置接口把它
// 映射成 409：这不是「服务端坏了」，而是「你让我写的那份配置不在那」。
// -rw 模式不会走到这里（启动时要求配置文件存在），但文件可能在运行期被
// 删掉、挪走，或者测试直接构造了一个只有路径的 WriteDeps。
var ErrWriteTargetMissing = errors.New("settings: write target does not exist")

// ErrAgentSectionMissing 表示删除时目标文件里没有这个 agent 的段。
// 目标的「有」不等于「这一份里有」：继承自低优先级层的 agent 在目标文件
// 里没有段，删它必须去定义它的那一份，而本文件的写入目标只有 layers[0]。
var ErrAgentSectionMissing = errors.New("settings: agent section not found in write target")

// InvalidConfigError 标记「写出去的新内容没通过 LoadMerged / Validate」。
// 调用方据此回 400 而不是 500：这是用户输入（或其它层并发改坏）的问题，
// 不是服务端故障。出现这个错误时目标文件一个字节都没动，临时文件已清理。
type InvalidConfigError struct{ err error }

func (e *InvalidConfigError) Error() string { return e.err.Error() }

// Unwrap 保留底层错误链，便于调用方在需要时继续判断。
func (e *InvalidConfigError) Unwrap() error { return e.err }

// AgentWriteResult 是一次 agent 段写入的结果。
//
// Changed=false 表示算出来的新内容与原文逐字节相同，没有写文件：这时
// BackupPath 为空、Config 为 nil，调用方应当保留当前配置并把「没有改动」
// 如实告诉用户，而不是伪造一个备份路径。
type AgentWriteResult struct {
	// BackupPath 是写前原文的备份路径（<配置文件名>.bak-<YYYYMMDD-HHmmss>，
	// 同一秒内重名时追加 -2…-99）。
	BackupPath string
	// WriteTarget 是实际写入的文件（永远等于 layers[0].Path）。
	WriteTarget string
	// Changed 表示文件确实被替换过。
	Changed bool
	// Config 是用「新内容当最高优先层」重新合并、校验通过的配置。
	// 它不含 Claude 导入（那只活在 serve 的内存里），由调用方补回去。
	// Changed=false 时为 nil。
	Config *Config
}

// agentSectionKeys 是设置页可编辑的键，顺序固定。
//
// 固定顺序有两个作用：缺失的键按同一顺序插进段尾（同一份配置改两次不会
// 因为 map 遍历顺序不同而出现不同字节）；预览里的字段顺序与文件里的
// 书写顺序也一致。role / prompt / 其它未知键不在这个表里——它们不经过
// 设置页，也绝不因为一次保存被重写。
var agentSectionKeys = []string{"display_name", "provider", "model", "effort", "permissions", "mode"}

// WriteAgentSection 把 agent 段写进 layers[0]，目标文件里已有这一段时
// 原地改它，没有时在文件末尾追加一段。fallback 只在追加时使用：目标文件
// 里没有这一段意味着这个 agent 原先继承自低优先级层，整条覆盖会丢掉
// role / prompt，所以这两个字段从当前生效的那一份取。
//
// 校验、备份、替换的顺序：
//  1. 读原文，算新内容（纯字节处理）；
//  2. 新内容逐字节没变 → 不写、不备份，Changed=false；
//  3. 新内容写进同目录的临时文件；
//  4. 用临时文件替换 layers[0] 重新 LoadMerged（含 Validate）——不通过就
//     删临时文件并回 InvalidConfigError，目标文件一个字节没动、也不留备份；
//  5. 通过后备份原文；
//  6. Rename 覆盖目标文件（同一目录内，Windows 上会替换已存在的目标）。
func WriteAgentSection(layers []Layer, id string, agent, fallback AgentConfig) (AgentWriteResult, error) {
	return writeAgentFile(layers, func(orig []byte) ([]byte, bool, error) {
		next, found, err := upsertAgentSection(orig, id, agent, fallback)
		return next, found, err
	})
}

// RemoveAgentSection 从 layers[0] 删掉 [agents.<id>] 这一段。文件里本来
// 没有这一段时返回 ErrAgentSectionMissing（继承来的成员不能用删除目标层
// 的办法删掉）；新内容过不了 Validate（例如还有 team / summary 引用它）
// 时返回 InvalidConfigError，文件不变。
func RemoveAgentSection(layers []Layer, id string) (AgentWriteResult, error) {
	return writeAgentFile(layers, func(orig []byte) ([]byte, bool, error) {
		return removeAgentSection(orig, id)
	})
}

// AgentDefinedIn 返回定义了 agent id 的配置层路径（高优先级在前）。
// 删除被拒时调用方用它说清「这一段定义在别的文件里」；读不出来或解析
// 失败的层直接跳过——它已经不可能给出比删除失败更有用的信息。
func AgentDefinedIn(layers []Layer, id string) []string {
	var out []string
	for _, layer := range layers {
		if layer.Path == "" {
			continue
		}
		var cfg Config
		if _, err := decodeConfigFile(layer.Path, &cfg); err != nil {
			continue
		}
		if _, ok := cfg.Agents[id]; ok {
			out = append(out, layer.Path)
		}
	}
	return out
}

// writeAgentFile 是两种写入共用的外壳：读原文 -> mutate -> 临时文件 ->
// 合并校验 -> 备份 -> Rename。mutate 返回 found=false 表示「目标文件里
// 没有要动的东西」（目前只有删除会这样）。
func writeAgentFile(layers []Layer, mutate func([]byte) ([]byte, bool, error)) (AgentWriteResult, error) {
	if len(layers) == 0 || layers[0].Path == "" {
		return AgentWriteResult{}, errors.New("settings: 没有可写入的配置层")
	}
	target := layers[0].Path
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return AgentWriteResult{}, fmt.Errorf("%w: %s", ErrWriteTargetMissing, target)
		}
		return AgentWriteResult{}, fmt.Errorf("查看目标配置 %s: %w", target, err)
	}
	if info.IsDir() {
		return AgentWriteResult{}, fmt.Errorf("目标配置 %s 是一个目录", target)
	}
	orig, err := os.ReadFile(target)
	if err != nil {
		return AgentWriteResult{}, fmt.Errorf("读目标配置 %s: %w", target, err)
	}
	next, found, err := mutate(orig)
	if err != nil {
		return AgentWriteResult{}, err
	}
	if !found {
		return AgentWriteResult{}, fmt.Errorf("%w: %s", ErrAgentSectionMissing, target)
	}
	if bytes.Equal(next, orig) {
		return AgentWriteResult{WriteTarget: target}, nil
	}

	perm := info.Mode().Perm()
	tmpName, err := writeTempFile(filepath.Dir(target), filepath.Base(target)+".tmp-edit-*", next, perm)
	if err != nil {
		return AgentWriteResult{}, fmt.Errorf("写临时文件: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	// 用临时文件当最高优先层重新合并：解码、未知键检查、Validate 全部跑
	// 一遍，与下一次 serve 启动读到的结果一致。目标文件此时还没被替换。
	probe := make([]Layer, len(layers))
	copy(probe, layers)
	probe[0].Path = tmpName
	merged, mergeErr := LoadMerged(probe)
	if mergeErr != nil {
		// 校验错误里可能出现临时文件名（多层合并会给错误补「来自 <文件>」）。
		// 把它换回真实路径，用户才认得出该改哪一份；错误类型不再保留
		// ValidationErrors —— 调用方只需要 400 与一句能读的话。
		message := strings.ReplaceAll(mergeErr.Error(), tmpName, target)
		return AgentWriteResult{}, &InvalidConfigError{err: errors.New(message)}
	}

	backup, err := writeBackupWithSuffix(target, ".bak-", orig, time.Now())
	if err != nil {
		return AgentWriteResult{}, fmt.Errorf("备份目标配置: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		// 备份已经在了，目标文件没有被替换。把备份路径交回去：调用方至少
		// 能告诉用户原件在哪。
		return AgentWriteResult{BackupPath: backup, WriteTarget: target}, fmt.Errorf("替换目标配置: %w", err)
	}
	committed = true
	return AgentWriteResult{BackupPath: backup, WriteTarget: target, Changed: true, Config: merged.Config}, nil
}

// fileLine 是一行原文与它的行尾。
//
// 保留行尾（而不是统一切成 \n）是「其它内容一个字节都不动」的前提：
// Windows 上的配置文件常常是 CRLF，统一改写会让整份文件在 diff 里全变。
// eol 为空串表示这是文件最后一行且原文没有换行符。
type fileLine struct {
	text string
	eol  string
}

// splitFileLines 按 \n 切行，并把 \r 归到行尾（CRLF）。空内容返回 nil。
func splitFileLines(content string) []fileLine {
	if content == "" {
		return nil
	}
	out := make([]fileLine, 0, strings.Count(content, "\n")+1)
	for len(content) > 0 {
		i := strings.IndexByte(content, '\n')
		if i < 0 {
			out = append(out, fileLine{text: content})
			break
		}
		text := content[:i]
		eol := "\n"
		if strings.HasSuffix(text, "\r") {
			text = text[:len(text)-1]
			eol = "\r\n"
		}
		out = append(out, fileLine{text: text, eol: eol})
		content = content[i+1:]
	}
	return out
}

func joinFileLines(lines []fileLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.text)
		b.WriteString(line.eol)
	}
	return b.String()
}

// dominantEOL 返回这份文件主要用的行尾。新插入的行跟它保持一致；
// 混合行尾的文件按「出现过 CRLF 就用 CRLF」处理，不再制造第三种写法。
func dominantEOL(lines []fileLine) string {
	for _, line := range lines {
		if line.eol == "\r\n" {
			return "\r\n"
		}
	}
	return "\n"
}

func isBlankLine(line fileLine) bool { return strings.TrimSpace(line.text) == "" }

// parseTableHeader 解析一行 TOML 表头，返回点分后的键段。
//
// 不是表头返回 ok=false。数组表 [[...]] 也算表头（它同样是段的边界），
// 但调用方只用它判断「这一段到这里为止」。引号里的 ']' 不算闭合；
// 表名里的引号段只去引号、不解转义 —— agent id 受 idPattern 约束，
// 唯一需要引号的情形是含点号的 id，里面不会有转义字符。
func parseTableHeader(line string) (path []string, ok bool) {
	s := strings.TrimSpace(line)
	// Windows 上用手写/编辑器保存的 TOML 常带 UTF-8 BOM。toml.Decode 认它，
	// 但按行切出来之后第一行是 "\ufeff[agents.p1]"，不加这一步就永远匹配
	// 不上这个表头：结果是「目标文件里没有这一段」，于是又追加一段同 id 的
	// 表，下一次加载直接报 Key 'agents.p1' has already been defined
	// （2026-10-25 的 CDP 实测踩到）。BOM 只可能出现在文件第一行；这里只是
	// 匹配时剥掉，原文一个字节不改。
	s = strings.TrimPrefix(s, "\ufeff")
	if !strings.HasPrefix(s, "[") {
		return nil, false
	}
	array := strings.HasPrefix(s, "[[")
	open := 1
	if array {
		open = 2
	}
	closeSeq := "]"
	if array {
		closeSeq = "]]"
	}
	end := -1
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '[':
			// 裸的 '[' 不可能出现在合法的表名里，整行按「不是表头」处理。
			return nil, false
		case ']':
			if strings.HasPrefix(s[i:], closeSeq) {
				end = i
				i = len(s)
			}
		}
	}
	if end < 0 {
		return nil, false
	}
	rest := strings.TrimSpace(s[end+len(closeSeq):])
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return nil, false
	}
	return splitTablePath(s[open:end]), true
}

func splitTablePath(inner string) []string {
	var parts []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '.':
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	parts = append(parts, strings.TrimSpace(cur.String()))
	return parts
}

// findAgentSection 定位 [agents.<id>] 这一段：start 是表头行，end 是下一行
// 表头的下标（没有下一段时是 len(lines)）。ok=false 表示目标文件里没有这一段。
func findAgentSection(lines []fileLine, id string) (start, end int, ok bool) {
	start = -1
	for i, line := range lines {
		path, isHeader := parseTableHeader(line.text)
		if !isHeader {
			continue
		}
		if start < 0 {
			if len(path) == 2 && path[0] == "agents" && path[1] == id {
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

// isKeyLine 判断一行是不是 key = ... 的开头。key 后面必须紧跟 '=' 或空白，
// 否则 display_name_x = … 会被误认成 display_name。
func isKeyLine(text, key string) bool {
	s := strings.TrimSpace(text)
	if !strings.HasPrefix(s, key) {
		return false
	}
	rest := s[len(key):]
	if rest == "" {
		return false
	}
	if rest[0] != '=' && rest[0] != ' ' && rest[0] != '\t' {
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	return strings.HasPrefix(rest, "=")
}

// trailingComment 返回一行里引号外的 # 注释（含 #，已去首尾空白）。
//
// 只用于「替换单行语句」时保留用户写在行尾的说明：设置页改的是字段值，
// 不是注释。引号里的 # 是值的一部分，不算注释；双引号里的 \" 也不能
// 让后面的 # 被误判成注释开始。
func trailingComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '#':
			if i == 0 || line[i-1] == ' ' || line[i-1] == '\t' {
				return strings.TrimSpace(line[i:])
			}
		}
	}
	return ""
}

// statementEnd 返回一条键值语句的结束行（多行字符串会跨行）。limit 是
// 段尾（不含），用于防止一个没闭合的三引号把后面整份文件吞掉。
func statementEnd(lines []fileLine, i, limit int) int {
	eq := strings.IndexByte(lines[i].text, '=')
	if eq < 0 {
		return i
	}
	value := strings.TrimSpace(lines[i].text[eq+1:])
	for _, delim := range []string{`"""`, `'''`} {
		if !strings.HasPrefix(value, delim) {
			continue
		}
		if strings.Contains(value[len(delim):], delim) {
			return i
		}
		for j := i + 1; j < limit; j++ {
			if strings.Contains(lines[j].text, delim) {
				return j
			}
		}
		return limit - 1
	}
	return i
}

// agentFieldValues 把 AgentConfig 里设置页可编辑的字段取出来。
// 空 permissions / mode 补成 Load 的缺省值（read / discussion）：
// 写进文件里的值必须能独立通过 Validate，不能依赖读取时再补一次。
func agentFieldValues(agent AgentConfig) map[string]string {
	values := map[string]string{
		"display_name": agent.DisplayName,
		"provider":     agent.Provider,
		"model":        agent.Model,
		"effort":       agent.Effort,
		"permissions":  string(agent.Permissions),
		"mode":         string(agent.Mode),
	}
	if values["permissions"] == "" {
		values["permissions"] = string(PermissionRead)
	}
	if values["mode"] == "" {
		values["mode"] = string(ModeDiscussion)
	}
	return values
}

// upsertAgentSection 是改段 / 追加段的纯字节实现。返回的 found 表示目标
// 文件里有没有要动的东西：改段与追加都算 true，只有删除会返回 false。
func upsertAgentSection(content []byte, id string, agent, fallback AgentConfig) ([]byte, bool, error) {
	lines := splitFileLines(string(content))
	eol := dominantEOL(lines)
	values := agentFieldValues(agent)

	start, end, ok := findAgentSection(lines, id)
	if !ok {
		block := renderAgentSection(id, agent, fallback, eol)
		return []byte(appendAgentBlock(string(content), block, eol)), true, nil
	}

	body := make([]fileLine, 0, end-start-1+len(values))
	handled := make(map[string]bool, len(agentSectionKeys))
	for i := start + 1; i < end; i++ {
		key := ""
		for _, candidate := range agentSectionKeys {
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
		if key == "effort" && values["effort"] == "" {
			// 空的 effort 在 AgentBlockTOML 里就是「不写这一行」：
			// 写 effort = "" 也等价，但会在文件里留下一个没有意义的键。
			i = stmtEnd
			continue
		}
		want := key + " = " + tomlBasicString(values[key])
		if stmtEnd == i && strings.TrimSpace(lines[i].text) == want {
			// 已经是规范写法：原样保留这一行（连同它的缩进与注释）。
			body = append(body, lines[i])
		} else {
			replacement := fileLine{text: want, eol: lines[i].eol}
			if replacement.eol == "" {
				replacement.eol = eol
			}
			// 单行语句行尾的注释是用户写的说明，不属于被改的字段值：
			// 换掉值时把它接在后面。多行字符串的「行尾 #」在引号里，
			// trailingComment 会正确地返回空串。
			if stmtEnd == i {
				if comment := trailingComment(lines[i].text); comment != "" {
					replacement.text = want + "  " + comment
				}
			}
			body = append(body, replacement)
		}
		i = stmtEnd
	}

	var missing []fileLine
	for _, key := range agentSectionKeys {
		if handled[key] {
			continue
		}
		if key == "effort" && values["effort"] == "" {
			continue
		}
		missing = append(missing, fileLine{text: key + " = " + tomlBasicString(values[key]), eol: eol})
	}
	if len(missing) > 0 {
		at := len(body)
		for at > 0 && isBlankLine(body[at-1]) {
			at--
		}
		if at > 0 && body[at-1].eol == "" {
			// 段尾是文件最后一行且没有换行：先补行尾，否则插入的键会粘在
			// 上一行后面。
			body[at-1].eol = eol
		}
		if at == 0 && len(body) > 0 && body[0].eol == "" {
			body[0].eol = eol
		}
		merged := make([]fileLine, 0, len(body)+len(missing))
		merged = append(merged, body[:at]...)
		merged = append(merged, missing...)
		merged = append(merged, body[at:]...)
		body = merged
	}

	out := make([]fileLine, 0, start+1+len(body)+len(lines)-end)
	out = append(out, lines[:start+1]...)
	// 段头是文件最后一行且原文没有换行（空段 + EOF）时，后面要接新的键，
	// 先把段头补上换行；否则 join 之后会变成 [agents.x]display_name = ...
	if len(body) > 0 && out[len(out)-1].eol == "" {
		out[len(out)-1].eol = eol
	}
	out = append(out, body...)
	out = append(out, lines[end:]...)
	return []byte(joinFileLines(out)), true, nil
}

// renderAgentSection 渲染一个完整的新段。字段顺序与 agentSectionKeys 一致，
// role 总是写（与 /add 同体例），prompt 只在非空时写。
//
// id 含点号时表名加引号，与 AgentBlockTOML 的写法一致；id 受 idPattern
// 约束，其它字符不会出现，所以这里不需要再做转义。
func renderAgentSection(id string, agent, fallback AgentConfig, eol string) string {
	values := agentFieldValues(agent)
	role := agent.Role
	if role == "" {
		// 目标文件原来没有这一段：这个 agent 继承自低优先级层。整条覆盖会
		// 让继承来的 role / prompt 失效，所以从当前生效的那一份取。
		role = fallback.Role
	}
	prompt := agent.Prompt
	if prompt == "" {
		prompt = fallback.Prompt
	}

	var b strings.Builder
	if strings.Contains(id, ".") {
		b.WriteString("[agents.\"" + id + "\"]" + eol)
	} else {
		b.WriteString("[agents." + id + "]" + eol)
	}
	kv := func(key, value string) {
		b.WriteString(key + " = " + tomlBasicString(value) + eol)
	}
	kv("display_name", values["display_name"])
	kv("provider", values["provider"])
	kv("model", values["model"])
	if values["effort"] != "" {
		kv("effort", values["effort"])
	}
	kv("role", role)
	kv("permissions", values["permissions"])
	kv("mode", values["mode"])
	if prompt != "" {
		kv("prompt", prompt)
	}
	return b.String()
}

// appendAgentBlock 把一段接到文件末尾，分隔规则与 AppendBlock 一致：
// 原文为空直接接；以换行结尾则再补一个换行（空一行）；否则补两个。
func appendAgentBlock(content, block, eol string) string {
	if content == "" {
		return block
	}
	if strings.HasSuffix(content, "\n") {
		return content + eol + block
	}
	return content + eol + eol + block
}

// removeAgentSection 删掉 [agents.<id>] 整段。段尾的注释与空行不属于
// 这一段的下一个表头，保留下来（通常是下一段的说明文字）。
func removeAgentSection(content []byte, id string) ([]byte, bool, error) {
	lines := splitFileLines(string(content))
	eol := dominantEOL(lines)
	start, end, ok := findAgentSection(lines, id)
	if !ok {
		return nil, false, nil
	}

	// 从段尾往前退：空行与注释行留给下一段。
	stop := end
	for stop > start+1 {
		text := strings.TrimSpace(lines[stop-1].text)
		if text == "" || strings.HasPrefix(text, "#") {
			stop--
			continue
		}
		break
	}

	before := make([]fileLine, start, start+1)
	copy(before, lines[:start])
	if len(before) > 0 && before[len(before)-1].eol == "" {
		before[len(before)-1].eol = eol
	}
	for len(before) > 0 && isBlankLine(before[len(before)-1]) {
		before = before[:len(before)-1]
	}

	after := make([]fileLine, 0, len(lines)-stop)
	after = append(after, lines[stop:]...)
	for len(after) > 0 && isBlankLine(after[0]) {
		after = after[1:]
	}

	if len(before) > 0 && len(after) > 0 {
		before = append(before, fileLine{eol: eol})
	}
	out := append(before, after...)
	return []byte(joinFileLines(out)), true, nil
}
