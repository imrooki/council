package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ClaudeAgents 是一次 Claude Code agents 扫描的结果。
//
// Agents 的 key 是 frontmatter 里的 name，不是文件名 —— 官方文档写明 name
// 才是身份，文件名不参与（docs/24 §「Claude 那边的形状」）。Notices 只给
// Verbose 档：跳过某个文件、同名冲突、目录读不出来都在这里，扫描本身不报错。
//
// 结果只进运行时内存，不写回任何配置，也不改动 .claude/ 里的文件。
type ClaudeAgents struct {
	Agents  map[string]AgentConfig
	Notices []string
}

// ScanClaudeAgents 按官方文档的优先级扫描 Claude Code 的 agent 定义：
//
//	项目层：cwd -> ... -> 仓库根，每一层的 .claude/agents/**/*.md
//	用户层：~/.claude/agents/**/*.md（最后扫，优先级最低）
//
// 扫描顺序就是优先级顺序（高到低），同一个 name 第一次出现的那条胜出，
// 所以「离 cwd 越近的覆盖越远的」不需要额外的覆盖逻辑，只需要先扫近的。
// 两个目录都递归；不在没有 .claude/ 时报错，静默当作 0 个。
//
// reserved 是配置里已有的 agent：同名时配置优先，Claude 的那条跳过并记一条
// 提示（任务书 §3 的映射表）。传 nil 表示没有配置 agent。
func ScanClaudeAgents(cwd string, reserved map[string]AgentConfig) ClaudeAgents {
	out := ClaudeAgents{Agents: make(map[string]AgentConfig)}
	// seen 跨层生效：一层定义过的 name，后面更低的层直接忽略。
	seen := make(map[string]bool)
	// reservedSeen 让「与配置同名」的提示只出一条：同一个冲突可能出现在
	// 项目层、用户层、甚至同一层的多个文件里，逐条打印只会刷屏。
	reservedSeen := make(map[string]bool)

	for _, dir := range claudeAgentDirs(cwd) {
		files, err := claudeAgentFiles(dir)
		if err != nil {
			// 读不出来的目录不该让整个扫描失败：用户主目录权限异常时，
			// 项目里那几个 agent 仍然值得显示出来。
			out.Notices = append(out.Notices,
				fmt.Sprintf("读取 Claude agents 目录 %s 失败，已跳过：%v", dir, err))
			continue
		}

		// layerSeen 只在同一个目录层内生效，用来实现「同一层内重名按文件
		// 路径字典序取第一个」；files 已经排过序，所以第一次见到就是胜者。
		layerSeen := make(map[string]string)
		for _, path := range files {
			spec, reason := readClaudeAgentFile(path)
			if reason != "" {
				out.Notices = append(out.Notices,
					fmt.Sprintf("跳过 Claude agent 文件 %s：%s", path, reason))
				continue
			}
			if _, ok := reserved[spec.Name]; ok {
				// 也写进 seen：同一个冲突在多份文件里出现时只提示一次。
				seen[spec.Name] = true
				if !reservedSeen[spec.Name] {
					reservedSeen[spec.Name] = true
					out.Notices = append(out.Notices,
						fmt.Sprintf("Claude agent %q（%s）与配置里的 agent 同名，配置优先，已跳过", spec.Name, path))
				}
				continue
			}
			if prev, ok := layerSeen[spec.Name]; ok {
				out.Notices = append(out.Notices, fmt.Sprintf(
					"Claude agent %q 在 %s 里重复定义，按文件路径字典序保留 %s，跳过 %s",
					spec.Name, dir, prev, path))
				continue
			}
			if seen[spec.Name] {
				// 高优先级目录已经定义过这个名字：这是正常的覆盖，不提示。
				// 这个判断必须放在 layerSeen 之后：同一层里的第二个同名文件
				// 会先命中 layerSeen，才会留下「同层重名」的提示；放到前面
				// 的话第二份会被当成「低优先级层」，提示永远不会出现。
				continue
			}
			layerSeen[spec.Name] = path
			seen[spec.Name] = true
			out.Agents[spec.Name] = spec.toAgentConfig()
		}
	}
	return out
}

// claudeAgentDirs 返回要扫描的目录，优先级从高到低。
//
// 用户层排在项目层后面，是因为项目里的同名定义要盖住用户级的那份；
// 先扫项目层、后扫用户层，加上「先见者胜」，正好得到这个优先级。
func claudeAgentDirs(cwd string) []string {
	dirs := make([]string, 0, 4)
	seen := make(map[string]bool)
	add := func(path string) {
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		dirs = append(dirs, clean)
	}
	for _, dir := range projectDirs(cwd) {
		add(filepath.Join(dir, ".claude", "agents"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".claude", "agents"))
	}
	return dirs
}

// claudeAgentFiles 递归收集 dir 下的 *.md，按路径字典序返回。
//
// 目录不存在返回空表、不报错：没有 .claude/ 是常态（docs/24「不在没有
// .claude/ 时报错，静默当作 0 个」）。其它错误原样交给调用方记提示。
func claudeAgentFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var files []string
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// WalkDir 在单个目录内是字典序，但整个递归结果不保证全局有序。
	// 「同一层重名取路径字典序第一个」要求的是全局序，所以这里显式排一次。
	sort.Strings(files)
	return files, nil
}

// claudeAgentSpec 是从一个 Claude agent 文件里读出来的、本片需要的字段。
type claudeAgentSpec struct {
	Name        string
	Description string
	Model       string
	Tools       []string
	// Prompt 是 frontmatter 之后的正文，也就是 Claude 那边的系统提示。
	// 本片只把它存进 AgentConfig.Prompt，不注入任何 prompt（下一片才用）。
	Prompt string
}

// toAgentConfig 按任务书 §3 的映射表把 Claude 定义翻成 Council 的 agent。
func (spec claudeAgentSpec) toAgentConfig() AgentConfig {
	permission := PermissionRead
	for _, tool := range spec.Tools {
		if claudeToolWritesFiles(tool) {
			permission = PermissionWorkspace
			break
		}
	}
	return AgentConfig{
		// Claude 的 name 同时是身份与唯一的人话标签（它没有单独的
		// display_name 字段），所以两个位置都用它。
		DisplayName: spec.Name,
		Provider:    "claude",
		Model:       spec.Model,
		Role:        spec.Description,
		Permissions: permission,
		Mode:        ModeDiscussion,
		Prompt:      spec.Prompt,
	}
}

// claudeToolWritesFiles 判断一个 tool 名会不会写盘。
//
// 只认任务书点名的 Write / Edit / Bash 三个，大小写不敏感，并剥掉
// Bash(git commit:*) 这类参数（Claude 的 tools 字段允许带限定范围）。
// 不做「包含 write 子串」这种判断：Read 与 Write 只差一个字母，
// 放宽到子串会把只读成员误升成 workspace。
func claudeToolWritesFiles(tool string) bool {
	name := strings.TrimSpace(tool)
	if i := strings.IndexAny(name, "(:"); i >= 0 {
		name = strings.TrimSpace(name[:i])
	}
	switch strings.ToLower(name) {
	case "write", "edit", "bash":
		return true
	default:
		return false
	}
}

// readClaudeAgentFile 读一个文件并解析。reason 为空表示成功；非空是给人看的
// 跳过原因（只进 Verbose 提示，不是错误）。
func readClaudeAgentFile(path string) (claudeAgentSpec, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return claudeAgentSpec{}, "读取失败: " + err.Error()
	}
	return parseClaudeAgentMarkdown(data)
}

// parseClaudeAgentMarkdown 解析 YAML frontmatter + 正文。
//
// 这里刻意不引入 YAML 依赖，只认 Claude agents 实际用到的那个子集：
//   - frontmatter 必须是文件开头第一行的 --- 到下一行 --- 之间；
//   - 只认顶层 key: value；不支持嵌套映射、块标量（| / >）、锚点；
//   - tools 支持三种写法：逗号分隔的字符串、行内列表 [Read, Edit]、
//     以及紧跟本行的一组 "- 项" 缩进列表；
//   - 值可以带引号（单/双），只去掉首尾引号，不解转义；未加引号的值支持
//     行尾注释（# 前面要有空白）。
//
// 超出这个子集时按「读到什么算什么」处理：未知键忽略；块标量（description: >）
// 不支持，会把标记本身当成值。官方 agents 的 frontmatter 就是这些简单标量，
// 所以不做完整 YAML 解析器。
func parseClaudeAgentMarkdown(data []byte) (claudeAgentSpec, string) {
	text := strings.TrimPrefix(string(data), utf8BOM)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return claudeAgentSpec{}, "文件开头没有 YAML frontmatter（第一行应是 ---）"
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return claudeAgentSpec{}, "frontmatter 没有结束的 ---"
	}

	fm := parseClaudeFrontmatter(lines[1:end])
	spec := claudeAgentSpec{
		Name:        strings.TrimSpace(fm.values["name"]),
		Description: strings.TrimSpace(fm.values["description"]),
		Model:       strings.TrimSpace(fm.values["model"]),
		Tools:       fm.tools,
		Prompt:      strings.TrimSpace(strings.Join(lines[end+1:], "\n")),
	}
	if spec.Name == "" {
		return spec, "缺少必填字段 name"
	}
	if spec.Description == "" {
		return spec, "缺少必填字段 description"
	}
	return spec, ""
}

// claudeFrontmatter 是解析出来的顶层字段。没有单独建模 PermissionMode：
// Council 这一片用不到它，多存一个没有任何消费者的字段只会让人以为它生效。
type claudeFrontmatter struct {
	values map[string]string
	tools  []string
}

func parseClaudeFrontmatter(lines []string) claudeFrontmatter {
	fm := claudeFrontmatter{values: make(map[string]string)}
	// 正在读的块列表：只可能是 tools。碰到下一个顶层 key 时收尾。
	listKey := ""
	var listItems []string
	flush := func() {
		if listKey == "tools" {
			fm.tools = listItems
		}
		listKey = ""
		listItems = nil
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if listKey != "" && strings.HasPrefix(line, "- ") {
			listItems = append(listItems, unquoteClaudeYAML(strings.TrimSpace(line[2:])))
			continue
		}
		flush()

		key, value, ok := splitClaudeYAMLKey(line)
		if !ok {
			continue
		}
		value = stripClaudeYAMLComment(value)
		switch key {
		case "tools":
			switch {
			case strings.HasPrefix(value, "["):
				fm.tools = splitClaudeTools(value)
			case value == "":
				// 值在后续 "- 项" 行里。
				listKey = "tools"
			default:
				fm.tools = splitClaudeTools(value)
			}
		default:
			fm.values[key] = unquoteClaudeYAML(value)
		}
	}
	flush()
	return fm
}

// splitClaudeYAMLKey 按第一个冒号切 key / value。description 里可以有冒号，
// 所以只切第一个；key 必须非空。
func splitClaudeYAMLKey(line string) (key, value string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:i])
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(line[i+1:]), true
}

// splitClaudeTools 解析 tools 的两种单行写法：逗号分隔字符串与 [a, b]。
// 值可能带括号（Bash(git commit:*)），但括号里没有逗号，按逗号切不会切坏。
//
// 先对整段值去一次引号，是为了 tools: "Read, Edit" 这种「整段是一个字符串」
// 的写法；若是 [..] 列表，整段不是成对引号，这次去引号不会生效，再逐项去。
func splitClaudeTools(value string) []string {
	value = unquoteClaudeYAML(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	parts := strings.Split(value, ",")
	tools := make([]string, 0, len(parts))
	for _, part := range parts {
		part = unquoteClaudeYAML(strings.TrimSpace(part))
		if part != "" {
			tools = append(tools, part)
		}
	}
	return tools
}

// unquoteClaudeYAML 去掉首尾成对的单/双引号。YAML 的转义序列不在支持子集里，
// 原样保留；单引号里的 ” 是 YAML 唯一的转义，顺手还原。
func unquoteClaudeYAML(value string) string {
	if len(value) < 2 {
		return value
	}
	switch {
	case value[0] == '"' && value[len(value)-1] == '"':
		return value[1 : len(value)-1]
	case value[0] == '\'' && value[len(value)-1] == '\'':
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	default:
		return value
	}
}

// stripClaudeYAMLComment 去掉未加引号的值后面的行尾注释。
//
// 判据与 YAML 一致：# 只有在行首或前面是空白时才开始注释。所以
// "description: C# 开发者" 里的 # 不会被误切。引号里的 # 也不切。
func stripClaudeYAMLComment(value string) string {
	var quote byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '#':
			if i == 0 || value[i-1] == ' ' || value[i-1] == '\t' {
				return strings.TrimSpace(value[:i])
			}
		}
	}
	return strings.TrimSpace(value)
}
