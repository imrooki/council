package config

// skills.go 只读扫描 Claude Code 的 skills，给 /api/options 展示用。
//
// 这里不执行 skill、不把正文拼进任何 prompt、不参与 cfg.Agents，也不写文件。
// 官方文档明确说 skill 的正文只在被调用时才进上下文，启动阶段能看到的只是
// name / description；Council 照这个边界只读元数据。
//
// 官方规则（2026-10 核实，https://code.claude.com/docs/en/skills.md）：
//   - 每个 skill 是一个目录，入口固定为目录下的 SKILL.md；scripts/ 等支持
//     文件与 SKILL.md 同级。.claude/skills/ 下的散装 .md 不是 skill。
//   - 位置：个人级 ~/.claude/skills/<name>/SKILL.md；项目级
//     <dir>/.claude/skills/<name>/SKILL.md。企业级在 managed settings 目录
//     里，Council 没有这个概念，不扫。
//   - 项目级从启动目录出发，沿每一级父目录读到自己所属仓库的根；启动目录
//     之下的 .claude/skills 在启动时不加载（Claude Code 要等真正读写那个
//     子目录里的文件时才加载），所以这里也不往下递归。
//   - 同名优先级：企业 > 个人 > 项目。也就是说用户级 skill 会盖住项目里的
//     同名项（文档原话 "personal over project"）。这个方向和 agents 相反，
//     所以扫描顺序不能共用同一段代码。
//   - 同一层里目录名就是唯一键，不会重复；跨层重名由「先扫到者胜」实现。
//   - 保留名：synced（任何大小写）与 anthropic-skills / anthropic-skills:*
//     官方在任何位置都不加载，这里同样跳过。
//   - frontmatter 只有文件第一行是 --- 时才读，字段全部可选。name 缺省取
//     目录名；description 缺省取正文第一行非空内容，也可以写成 | / > 块标量。
//     官方还说 YAML 解析失败时 skill 仍加载、只是字段全空——本文件借用
//     agents 的标量辅助函数，解析不出来的行等价于忽略，效果一致。
//   - 名字里带控制字符、零宽字符、尖括号等的 skill 官方不加载；这里不做这套
//     字符审查：本片只展示，误杀一个正常名字比漏掉一条的代价更大，而且
//     Claude Code 自己仍会在那边拒绝它。

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Skill 是一条只读导入的 Claude Code skill 元数据。
//
// ID 取磁盘上的目录名：官方把「同名 skill」定义为目录/文件名相同，目录名
// 同时也是 name 缺省时的命令名；Name 取 frontmatter 的 name，缺省等于目录名。
// 两者分开存，是因为 name 可以覆盖命令名，而目录仍是这台机器上的身份。
type Skill struct {
	ID          string
	Name        string
	Description string
	// Path 是 SKILL.md 的路径，只用于诊断与测试；不进 /api/options：
	// 把开发机的绝对路径发给前端没有任何用处。
	Path string
}

// ScanClaudeSkills 扫描 cwd 能看到的 Claude Code skills，按官方优先级从高到低
// 返回：用户级在前，项目级由近到远在后。同名（目录名相同）只保留第一条。
//
// 纯函数：只读磁盘，不写文件、不改全局状态。目录不存在或读不出来一律当作
// 0 个——没有 ~/.claude/skills 是常态，一个读不了的目录不该让 /api/options
// 失败；任务书给的签名就是 []Skill，没有 Notices 出口，所以扫描本身不产生提示。
func ScanClaudeSkills(cwd string) []Skill {
	out := make([]Skill, 0)
	// seen 跨层生效：高优先级的目录名一旦读成功，更低层的同名目录直接忽略。
	// 只在成功读出 SKILL.md 后才标记，否则一个没有 SKILL.md 的高优先级目录
	// 会把低优先级里真正的 skill 挡掉。
	seen := make(map[string]bool)

	for _, dir := range claudeSkillDirs(cwd) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		// os.ReadDir 按文件名排序，所以同一层里的结果稳定；跨层顺序由
		// claudeSkillDirs 的优先级决定。
		for _, entry := range entries {
			id := entry.Name()
			if claudeSkillReserved(id) || seen[id] {
				continue
			}
			skillPath := filepath.Join(dir, id, "SKILL.md")
			if !claudeSkillDir(filepath.Join(dir, id), entry) {
				continue
			}
			spec, ok := readClaudeSkill(skillPath, id)
			if !ok {
				// 没有 SKILL.md 的目录不是 skill（也可能只是别的工具放在
				// 这里的目录）。不标记 seen：更低优先级的同名目录仍可定义它。
				continue
			}
			seen[id] = true
			out = append(out, spec)
		}
	}
	return out
}

// claudeSkillDirs 按官方优先级返回要扫描的目录，优先级从高到低。
//
// 用户级排在最前（优先级最高），项目链由近到远排在其后：官方 skills 文档
// 写明同名时 "personal over project"，与 agents 的「项目覆盖用户」相反。
// 不要为了和 claudeAgentDirs 对称把两者合成一个 helper——合了就得在每个
// 调用点各自翻转一次顺序，反而更容易把优先级抄反。
//
// 项目链复用 projectDirs（cwd -> 仓库根），与 .council 配置、Claude agents
// 同一条「仓库边界」判断：没有 .git 就只认 cwd，不一路走到盘符根。
func claudeSkillDirs(cwd string) []string {
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
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".claude", "skills"))
	}
	for _, dir := range projectDirs(cwd) {
		add(filepath.Join(dir, ".claude", "skills"))
	}
	return dirs
}

// claudeSkillReserved 判断一个目录名是不是官方保留、永远不加载的名字。
//
// synced 是 claude.ai 同步目录（文档要求任何大小写都不能用）；anthropic-skills
// 及其冒号前缀是同步 skill 的保留命名空间。按不区分大小写判断：目录名在这里
// 只用于展示，多跳过一个保留名的变体比把同步缓存当成用户 skill 更安全。
func claudeSkillReserved(id string) bool {
	lower := strings.ToLower(id)
	return lower == "synced" ||
		lower == "anthropic-skills" ||
		strings.HasPrefix(lower, "anthropic-skills:")
}

// claudeSkillDir 判断 .claude/skills 下的一个条目是不是 skill 目录。
//
// 目录本身可以是符号链接：官方允许 <skill-name> 指向磁盘别处的目录，并按
// 目标里的 SKILL.md 加载。DirEntry.IsDir 对符号链接返回 false，所以这里要
// 再看一次目标；链接坏了就当作不是 skill。
func claudeSkillDir(path string, entry fs.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&fs.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// readClaudeSkill 读一个 SKILL.md 并解析出元数据。ok=false 表示这个目录不是
// 可用的 skill（文件不存在、读不了）：调用方静默跳过，不报错。
func readClaudeSkill(path, dirName string) (Skill, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, false
	}
	name, description := parseClaudeSkillMarkdown(data)
	if name == "" {
		name = dirName
	}
	return Skill{ID: dirName, Name: name, Description: description, Path: path}, true
}

// parseClaudeSkillMarkdown 只取 name 与 description：其它 frontmatter 字段
// （allowed-tools、context、paths……）与 Council 无关，读了也没有消费者。
//
// 解析范围与 agents 相同——顶层标量、单双引号、行尾注释——但这里多两条：
//   - 只有顶格的 key 才算字段。缩进行是嵌套映射（metadata 之类）或块标量的
//     内容，不能拿来覆盖 name / description；
//   - 支持 description 的块标量（| 字面、> 折叠）。官方示例虽然是单行，
//     但 YAML 合法写法里多行说明很常见，原样展示 ">" 是错的。
//
// 不支持锚点、流程映射、多文档；超出范围时读到什么算什么，与 agents 一致。
func parseClaudeSkillMarkdown(data []byte) (name, description string) {
	text := strings.TrimPrefix(string(data), utf8BOM)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	body := lines
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		if end := claudeSkillFrontmatterEnd(lines); end > 0 {
			values := parseClaudeSkillFrontmatter(lines[1:end])
			name = strings.TrimSpace(values["name"])
			description = strings.TrimSpace(values["description"])
			body = lines[end+1:]
		}
	}
	if description == "" {
		// 官方缺省：description 取正文第一行非空内容。没有正文就是空串，
		// /api/options 里保留这条 skill（名字仍然可展示）。
		description = claudeSkillFirstLine(body)
	}
	return name, description
}

// parseClaudeSkillFrontmatter 解析 skill frontmatter 的顶层字段。
func parseClaudeSkillFrontmatter(lines []string) map[string]string {
	values := make(map[string]string)
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		// 顶格才可能是顶层 key；空行、注释、缩进行都跳过。
		if raw == "" || raw[0] == ' ' || raw[0] == '\t' {
			continue
		}
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := splitClaudeYAMLKey(line)
		if !ok {
			continue
		}
		value = stripClaudeYAMLComment(value)
		if !claudeYAMLBlockScalar(value) {
			values[key] = unquoteClaudeYAML(value)
			continue
		}
		block, next := claudeYAMLBlock(lines, i+1, strings.HasPrefix(value, ">"))
		values[key] = block
		i = next - 1
	}
	return values
}

// claudeYAMLBlockScalar 判断一个值是否块标量标记（|、>，允许带 +/- 与缩进
// 指示数字）。值以引号开头时是普通字符串，不会走到这里。
func claudeYAMLBlockScalar(value string) bool {
	if value == "" || (value[0] != '|' && value[0] != '>') {
		return false
	}
	for _, c := range value[1:] {
		if c != '+' && c != '-' && (c < '1' || c > '9') && c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

// claudeYAMLBlock 从 start 行开始收集块标量正文，返回折叠后的文本与下一行
// 行号（第一个顶格行）。
//
// folded 为真（>）时把各行用空格连起来——YAML 折叠标量的语义就是换行变空格；
// 为假（|）时保留换行。公共缩进按非空行的最小缩进去掉，尾部空行丢弃：
// 这里只用于展示 description，chomping 指示符不影响最终文本。
func claudeYAMLBlock(lines []string, start int, folded bool) (string, int) {
	end := start
	for end < len(lines) {
		raw := lines[end]
		if raw != "" && raw[0] != ' ' && raw[0] != '\t' {
			break
		}
		end++
	}
	block := lines[start:end]
	for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
		block = block[:len(block)-1]
	}

	indent := -1
	for _, raw := range block {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		n := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	parts := make([]string, 0, len(block))
	for _, raw := range block {
		switch {
		case strings.TrimSpace(raw) == "":
			parts = append(parts, "")
		case indent >= 0 && len(raw) >= indent:
			parts = append(parts, raw[indent:])
		default:
			parts = append(parts, strings.TrimSpace(raw))
		}
	}

	if folded {
		return strings.Join(strings.Fields(strings.Join(parts, " ")), " "), end
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), end
}

// claudeSkillFrontmatterEnd 返回结束标记 --- 的行号；没有则 -1。
//
// 只从第二行开始找：第一行必须是 --- 才算 frontmatter（官方规则），
// 否则整个文件都是正文。
func claudeSkillFrontmatterEnd(lines []string) int {
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i
		}
	}
	return -1
}

// claudeSkillFirstLine 返回第一行非空内容，去掉首尾空白。官方把这一行当作
// 缺省 description，值原样展示，不剥 Markdown 标记（# 标题也照留）。
func claudeSkillFirstLine(lines []string) string {
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
