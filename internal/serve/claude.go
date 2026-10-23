package serve

// claude.go 把 `serve -rw` 启动时扫到的 Claude Code agents 在运行时并进
// cfg.Agents。它只改内存里的那一份 Config，不写任何配置文件，也不改
// .claude/ 里的文件：Claude 那边的定义仍是唯一事实源，下次启动重新扫。
//
// 合并而不是另建一张「可执行成员表」，是因为 headless.Validate /
// resolveMembers / turn.Runner 全部从 cfg.Agents 读名单（见
// internal/headless/options.go 与 internal/turn/runner.go）。在这里并进去，
// 校验、成员解析、执行三处自动一致，不必给 headless 加第二条查表路径——
// 两条路径迟早会漂成两套「谁能被选中」的答案。

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"council/internal/config"
)

// claudeImport 是一次 Claude agents 导入的结果。
type claudeImport struct {
	// agents 是真正合并进 cfg.Agents 的成员。没有 [providers.claude] 时为空：
	// 那种情况下它们既不该出现在 /api/options，也不该被执行。
	agents map[string]config.AgentConfig
	// merged 是 agents 的 id 集合，给 /api/options 区分来源用（source=claude）。
	// 合并后这些 id 也在 Cfg.Agents 里，单看那张表已经分不出哪些是配置、
	// 哪些是导入的。
	merged map[string]struct{}
	// withoutProvider 是扫到了、但因为配置缺 [providers.claude] 而没合并的
	// 成员。留着它是为了让 /api/ask 被选中时能说清根因（缺 provider），
	// 而不是笼统地回一句「未知成员」。
	withoutProvider map[string]config.AgentConfig
	// notices 是扫描与合并的详细提示，只给 Verbose 档打印：跳过坏文件、
	// 同名冲突、成功合并了几条都属于「发生了什么」，不是必须让用户处理的事。
	notices []string
	// warning 是不依赖 Verbose 的提示。目前只有「缺 [providers.claude]，
	// 已整体跳过」这一种：它指向一个可执行的配置动作，藏在 -verbose 后面
	// 用户只会看到工作台里莫名其妙少了一批成员。
	warning string
}

// importClaudeAgents 扫描 cwd 下的 Claude agents，并在 provider 存在时把
// 它们合并进 cfg.Agents。
//
// 调用点必须在构造 turn.Runner 之前：Runner 持有 cfg 指针，并明确要求
// 被指向的 Config 在交给它之后不再原地修改（internal/turn/runner.go 的
// SetConfig 注释把这条写成不变量）。启动阶段先合并完再交给 Runner，
// 就不存在「轮次读到一半成员表变了」的窗口。
//
// 合并后不再跑 config.Validate：Claude 的 model 允许为空（任务书 §1 的映射
// 表），而 config.Validate 要求配置 agent 的 model 非空。这张表只进运行时、
// 不落盘，不能拿配置文件的校验规则去拒掉一个合法的 Claude 定义。
func importClaudeAgents(cfg *config.Config, cwd string) claudeImport {
	if cfg == nil {
		// 写模式一定会加载配置；这里只是不让一个装配疏漏变成 nil 解引用。
		return claudeImport{}
	}

	scanned := config.ScanClaudeAgents(cwd, cfg.Agents)
	imported := claudeImport{notices: scanned.Notices}
	if len(scanned.Agents) == 0 {
		return imported
	}

	if _, ok := cfg.Providers["claude"]; !ok {
		// 没有 provider 就不能合并：能合并进来的是「成员」，不是「能跑的
		// 进程」。headless 校验会放行一条永远找不到 provider 的成员，
		// 用户要到花钱那一步才看到失败；任务书要求这里整体跳过。
		imported.withoutProvider = scanned.Agents
		ids := sortedAgentIDs(scanned.Agents)
		imported.warning = fmt.Sprintf(
			"发现 %d 个 Claude agent（%s），但配置里没有 [providers.claude]，已整体跳过：它们不会出现在工作台，也不能被 /api/ask 选中。要启用请在配置里补上 [providers.claude] 后重启",
			len(ids), strings.Join(ids, ", "))
		return imported
	}

	if cfg.Agents == nil {
		// Load / LoadMerged 都会把表初始化好；直接构造 Config 的测试可能没做。
		cfg.Agents = make(map[string]config.AgentConfig, len(scanned.Agents))
	}
	imported.agents = make(map[string]config.AgentConfig, len(scanned.Agents))
	imported.merged = make(map[string]struct{}, len(scanned.Agents))
	mergedIDs := make([]string, 0, len(scanned.Agents))
	for _, id := range sortedAgentIDs(scanned.Agents) {
		if _, exists := cfg.Agents[id]; exists {
			// ScanClaudeAgents 已经用 reserved 挡过同名，这里再挡一次：
			// 合并的是同一张内存表，任何一条覆盖都会让「配置优先」变成
			// 取决于遍历顺序的偶然结果。
			continue
		}
		cfg.Agents[id] = scanned.Agents[id]
		imported.agents[id] = scanned.Agents[id]
		imported.merged[id] = struct{}{}
		mergedIDs = append(mergedIDs, id)
	}
	if len(mergedIDs) > 0 {
		imported.notices = append(imported.notices, fmt.Sprintf(
			"已把 %d 个 Claude agent（%s）合并进本次运行的成员表（仅内存，不写配置文件）",
			len(mergedIDs), strings.Join(mergedIDs, ", ")))
	}
	return imported
}

// claudePromptNotice 是 `serve -rw` 启动时那行外部系统提示的可见性提示
// （任务书 §4）。0 个成员时返回空串，调用方据此不打印——这是安全可见性，
// 不是日志，所以不受 -verbose 控制。
//
// 数的是运行时 cfg.Agents 里 provider == "claude" 的成员：导入的 Claude
// 成员在 importClaudeAgents 里已经并进去了，而没合并的成员（同名冲突、缺
// provider）不会被执行，也就没有提示被注入。手写配置的 claude 成员同样会被
// 注入，所以也数进来；非 claude 成员带 prompt 不会被任何适配器消费，
// 把它们列进「外部系统提示」只会制造一条吓人的假警报。
// 成员 id 排序后输出，同一份配置每次启动打印同一行。
//
// 「来自 .claude/agents」是任务书要求的那一句措辞，也是 Prompt 的主要来源
// （Claude 导入；手写配置的 prompt 走的是同一个注入路径，出处在这里分不出来）。
// 若将来要按来源拆成两句话，这句要跟着改；现在改它只会让任务书要求的那一行消失。
func claudePromptNotice(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	ids := make([]string, 0, len(cfg.Agents))
	for id, ac := range cfg.Agents {
		if ac.Provider == "claude" && ac.Prompt != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return fmt.Sprintf("%d 个成员带外部系统提示（%s），来自 .claude/agents",
		len(ids), strings.Join(ids, ", "))
}

// printClaudePromptNotice 把 claudePromptNotice 打到启动日志上；没有成员
// 带提示时一个字节都不写。单独拆出来是为了让「0 不打印」这条可以在
// 不启动 HTTP 服务器的前提下被用例钉住。
func printClaudePromptNotice(w io.Writer, cfg *config.Config) {
	note := claudePromptNotice(cfg)
	if note == "" {
		return
	}
	fmt.Fprintf(w, "council serve: %s\n", note)
}

// claudeProviderHint 给 headless.Validate 的「未知成员」错误补一句根因。
//
// headless 的用法错误没有 exported 的类别字段（msg 是私有的），所以这里只
// 在该错误确实是「未知成员」、且被点名的 id 恰好在一个因缺 provider 而未
// 导入的 Claude agent 里时替换文案；其它用法错误原样返回。不这么做的话，
// 用户看到的是「配置里没有这个 agent」，而真正要改的是 [providers.claude]。
func (s *Server) claudeProviderHint(err error, members []string) error {
	if err == nil || s.write == nil || len(s.write.ClaudeAgentsWithoutProvider) == 0 {
		return err
	}
	message := err.Error()
	if !strings.Contains(message, "未知成员") {
		return err
	}

	names := make([]string, 0, len(members))
	for _, raw := range members {
		id := strings.TrimSpace(raw)
		if _, ok := s.write.ClaudeAgentsWithoutProvider[id]; !ok {
			continue
		}
		// 只补被这条错误真正点名的那几个：同一份请求里可能既有拼错的 id，
		// 也有确实存在、只是缺 provider 的 Claude 成员。
		if !strings.Contains(message, fmt.Sprintf("%q", id)) {
			continue
		}
		names = append(names, id)
	}
	if len(names) == 0 {
		return err
	}
	sort.Strings(names)

	return fmt.Errorf("%w；%s 在 .claude/agents 里存在，但配置缺少 [providers.claude]，Claude agents 已整体跳过。补上 [providers.claude] 后重启即可选中",
		err, quoteJoined(names))
}

// quoteJoined 把 id 排成稳定的、带引号的列表。错误文案里有多个成员时，
// 顺序不能跟着请求里的出现顺序漂：同一份请求每次得到的提示应当一样。
func quoteJoined(ids []string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	return strings.Join(quoted, "、")
}
