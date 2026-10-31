package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// idPattern 是 provider / agent / team 三张表共用的 id 规则。
// internal/repl/mention.go 的 mentionIDPattern 必须是同一条，改这里时记得去改那边。
//
// 点号可以在中间（连续点号也行，a..b 合法），为的是 grok-4.7 这类带版本号的名字。
//
// 末字符类是 [a-z0-9_-] 而不是 [a-z0-9]：旧规则允许尾部的 - 与 _，mention.go 里
// 有一段既有裁决专门保护「真叫 gk- 的 agent」。这一刀只放开点号，不收紧别的。
//
// 不许以点号结尾，两个理由：
//   - agent id 会进文件系统路径（agentWorkDir 用 filepath.Join 拼它），Windows 会
//     静默剥掉名字尾部的点，id 与真实目录名就对不上；
//   - mention tokenizer 靠「尾部的点不可能属于合法 id」来剥句末的点，这条由本正则保证。
//
// 已知代价：id 含点号时，Validate 的错误 Path（例如 agents.grok-4.7.model）读不出
// 层级。Path 只给人看，生产代码不解析它，接受。
const idPattern = `^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$`

var idRegexp = regexp.MustCompile(idPattern)

// reservedAgentIDs 是不能拿来当 agent id 的名字：它们是 Canonical Conversation 里的
// **保留 sender**，不是 participant。配置里出现同名 agent 会与它们撞 ——
// 那个 agent 发的言会被当成保留 sender：跳过活跃成员校验，而且一律不许带脑子，
// 于是它的每一条发言在 /log 里都没有归属，而这正是用户要的那件事。
//
// 值必须与 conversation 包的 ReservedSenderUser / ReservedSenderSystem 一致。
// 这里写字面量而不是 import 那个包，是为了不让配置解析层依赖领域层。
// 代价是两份值要靠人保持同步：改那边的常量时记得回来改这里 ——
// 那边有用例钉着常量的取值，会先在那里红，不会悄悄漂走。
var reservedAgentIDs = map[string]bool{
	"user":   true, // conversation.ReservedSenderUser
	"system": true, // conversation.ReservedSenderSystem
}

// Validate 做规格 D 的全部校验，一次返回所有错误。无错时返回 nil。
// 返回的 error 实际类型是 ValidationErrors；长度为 0 时返回字面 nil，
// 避免空切片装进 error 接口后 err != nil。
func (c *Config) Validate() error {
	var errs ValidationErrors
	add := func(path, msg string) {
		errs = append(errs, ValidationError{Path: path, Msg: msg})
	}

	for _, id := range sortedIDs(c.Providers) {
		if !idRegexp.MatchString(id) {
			add("providers."+id, fmt.Sprintf("id %q does not match %s", id, idPattern))
		}
		pc := c.Providers[id]
		if pc.Command == "" {
			add("providers."+id+".command", "must not be empty")
		}
		if len(pc.Args) > MaxProviderArgs {
			add("providers."+id+".args", fmt.Sprintf("has %d entries, exceeds the %d-entry limit", len(pc.Args), MaxProviderArgs))
		}
		for i, arg := range pc.Args {
			if arg == "" {
				add(fmt.Sprintf("providers.%s.args[%d]", id, i), "must not be empty")
				continue
			}
			if strings.ContainsRune(arg, 0) {
				add(fmt.Sprintf("providers.%s.args[%d]", id, i), "must not contain NUL")
				continue
			}
			if problem := providerArgProblem(id, pc.Args, i); problem != "" {
				add(fmt.Sprintf("providers.%s.args[%d]", id, i), problem)
				continue
			}
			if len(arg) > MaxProviderEntryBytes {
				add(fmt.Sprintf("providers.%s.args[%d]", id, i), fmt.Sprintf(
					"is %d bytes, exceeds the %d-byte limit", len(arg), MaxProviderEntryBytes))
			}
		}
		if len(pc.Models) > MaxProviderModels {
			add("providers."+id+".models", fmt.Sprintf("has %d entries, exceeds the %d-entry limit", len(pc.Models), MaxProviderModels))
		}
		for i, model := range pc.Models {
			path := fmt.Sprintf("providers.%s.models[%d]", id, i)
			if strings.TrimSpace(model) == "" {
				add(path, "must not be empty")
				continue
			}
			if len(model) > MaxProviderEntryBytes {
				add(path, fmt.Sprintf("is %d bytes, exceeds the %d-byte limit", len(model), MaxProviderEntryBytes))
			}
		}
		if len(pc.Env) > MaxProviderEnv {
			add("providers."+id+".env", fmt.Sprintf("has %d entries, exceeds the %d-entry limit", len(pc.Env), MaxProviderEnv))
		}
		for i, entry := range pc.Env {
			path := fmt.Sprintf("providers.%s.env[%d]", id, i)
			if problem := EnvEntryProblem(entry); problem != "" {
				add(path, problem)
				continue
			}
			if len(entry) > MaxProviderEntryBytes {
				add(path, fmt.Sprintf("is %d bytes, exceeds the %d-byte limit", len(entry), MaxProviderEntryBytes))
			}
		}
	}

	for _, id := range sortedIDs(c.Agents) {
		agent := c.Agents[id]
		if !idRegexp.MatchString(id) {
			add("agents."+id, fmt.Sprintf("id %q does not match %s", id, idPattern))
		}
		if reservedAgentIDs[id] {
			add("agents."+id, fmt.Sprintf("id %q is reserved", id))
		}
		if _, ok := c.Providers[agent.Provider]; !ok {
			add("agents."+id+".provider", fmt.Sprintf("agent %q references undefined provider %q", id, agent.Provider))
		}
		if agent.Model == "" {
			add("agents."+id+".model", "must not be empty")
		}
		// 超限不截断：截断会让成员行为与 .claude/agents 里的文件内容悄悄
		// 不一致（MaxAgentPromptBytes 的注释）。错误路径带 agent id，
		// LoadMerged 合并多层时会再补上「来自哪一份配置文件」；直接调用
		// Validate 的调用方没有合并信息，所以 Msg 里不编一个文件路径出来。
		if len(agent.Prompt) > MaxAgentPromptBytes {
			add("agents."+id+".prompt", fmt.Sprintf(
				"prompt is %d bytes, exceeds the %d-byte limit; external system prompts are never truncated, shorten the agent definition",
				len(agent.Prompt), MaxAgentPromptBytes))
		}
		if !validPermission(agent.Permissions) {
			add("agents."+id+".permissions", fmt.Sprintf("invalid permissions %q (want read, workspace, or full)", agent.Permissions))
		}
		if !validMode(agent.Mode) {
			add("agents."+id+".mode", fmt.Sprintf("invalid mode %q (want discussion, worker, discussion_and_worker, judge, or manager)", agent.Mode))
		}
	}

	for _, id := range sortedIDs(c.Teams) {
		team := c.Teams[id]
		if !idRegexp.MatchString(id) {
			add("teams."+id, fmt.Sprintf("id %q does not match %s", id, idPattern))
		}
		// thought_visible_to 与 members 互相独立。成员表空时仍要校验
		// 可见性名单：否则一份「members 忘了填、mode 也写错」的配置
		// 只会报 members，把 mode 那条吞掉。
		//
		// 不在这里拦 store_thought==false 且名单非空。关掉存储不洗
		// 历史，历史行的 thought 照样会被 Router 渲染进打开了可见性
		// 的 Mode 的 prompt；拦掉会把「先存一段时间、现在不想再存、
		// 但仍要让裁判看得到已有的那些」这个合法配置拒掉。
		// 那一行提示印在 /rules，不在 Validate 里变错误。
		seenMode := make(map[Mode]struct{}, len(team.ThoughtVisibleTo))
		dupModeReported := make(map[Mode]struct{})
		for _, m := range team.ThoughtVisibleTo {
			if _, ok := seenMode[m]; ok {
				if _, reported := dupModeReported[m]; !reported {
					add("teams."+id+".thought_visible_to", fmt.Sprintf("duplicate mode %q", m))
					dupModeReported[m] = struct{}{}
				}
			} else {
				seenMode[m] = struct{}{}
			}
			if !validMode(m) {
				add("teams."+id+".thought_visible_to", fmt.Sprintf("invalid mode %q", m))
			}
		}
		if len(team.Members) == 0 {
			add("teams."+id+".members", "must not be empty")
			continue
		}
		seen := make(map[string]struct{}, len(team.Members))
		dupReported := make(map[string]struct{})
		for _, member := range team.Members {
			if _, ok := seen[member]; ok {
				if _, reported := dupReported[member]; !reported {
					add("teams."+id+".members", fmt.Sprintf("duplicate member %q", member))
					dupReported[member] = struct{}{}
				}
			} else {
				seen[member] = struct{}{}
			}
			if _, ok := c.Agents[member]; !ok {
				add("teams."+id+".members", fmt.Sprintf("undefined agent %q", member))
			}
		}
	}

	// 空串 = 没配，不报。大小写敏感，与 teams 的 undefined agent 同体例。
	// 只要求 read：总结是自动起的进程，没有人当场点名，不给写盘能力。
	// mode 不要求：总结路径不经 PlanRound，mode 在这条路上只影响 thought 可见性。
	if id := c.Summary.Agent; id != "" {
		agent, ok := c.Agents[id]
		if !ok {
			add("summary.agent", fmt.Sprintf("undefined agent %q", id))
		} else if agent.Permissions != PermissionRead {
			add("summary.agent", fmt.Sprintf("agent %q must have permissions = \"read\": the summary runs without being asked", id))
		}
	}

	// 空串按 auto 处理：直接构造的 Config 不会经过 applyDefaults，既有
	// 调用方（测试与内部装配）看不出这个新字段的存在是合理的。
	if !validLanguage(c.Language) {
		add("language", fmt.Sprintf("不支持的语言 %q（可选 zh / en / auto）", c.Language))
	}

	// 空串按 off 处理：直接构造的 Config 不经过 applyDefaults（同语言那条）。
	// 错误文案把合法取值列全，否则用户写 local_agent = "on" 只能靠猜。
	if !validLocalAgentLevel(c.LocalAgent) {
		add("local_agent", fmt.Sprintf("不支持的本机 agent 开关 %q（可选 off / observe / coordinate）", c.LocalAgent))
	}

	if c.DataDir != "" && !filepath.IsAbs(c.DataDir) {
		add("data_dir", fmt.Sprintf("must be an absolute path: %q", c.DataDir))
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// providerArgProblem 按 provider 的适配器维护一份「能改权限档」的 flag
// 黑名单（S5）。
//
// 为什么必须在配置层拒绝、而不是起进程前再过滤：args 是用户写进配置的
// 事实，如果启动时才偷偷丢掉，屏幕上看到的启动方式与实际跑的不是同一件
// 事；而且设置页会把它当成合法值继续保存。拒绝时错误路径能指到具体
// provider 的第几个参数。
//
// 只列各家的权限类 flag，不做「看起来像权限」的模糊匹配：普通参数
// （--verbose、-c model_reasoning_effort=high、--deny Write(*)）必须继续
// 可用，否则会逼用户绕过设置页。
//
// codex 的 -c/--config 是 key=value 形式，值可能与 flag 分开两个 argv
// 元素；只在 key 是 sandbox_mode 时拒绝。approval_policy 不拒绝：适配器
// 已经发 never，用户再写一次不会放大写盘边界。
func providerArgProblem(providerID string, args []string, i int) string {
	name, value, hasValue := splitProviderArg(args[i])
	name = strings.ToLower(name)
	reject := func() string {
		return fmt.Sprintf("permission-widening flag %q is not allowed in provider args: the adapter's own permission flags must stay authoritative", args[i])
	}

	switch strings.ToLower(providerID) {
	case "claude":
		switch name {
		case "--dangerously-skip-permissions", "--permission-mode", "--add-dir",
			"--allowedtools", "--allowed-tools", "--settings":
			return reject()
		}
	case "codex":
		switch name {
		case "--dangerously-bypass-approvals-and-sandbox", "--full-auto", "--yolo",
			"--sandbox", "-s", "--add-dir":
			return reject()
		}
		if name == "-c" || name == "--config" {
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
				hasValue = true
			}
			if hasValue {
				normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
				if strings.HasPrefix(normalized, "sandbox_mode=") {
					return reject()
				}
			}
		}
	case "grok":
		switch name {
		case "--dangerously-skip-permissions", "--permission-mode", "--allow",
			"--tools", "--sandbox", "--add-dir":
			return reject()
		}
	case "agy":
		switch name {
		case "--dangerously-skip-permissions", "--mode", "--add-dir", "--sandbox":
			return reject()
		}
	}
	return ""
}

// splitProviderArg 把 --flag=value 拆成名字与值；没有等号时 hasValue 为 false。
// 不用 strings.SplitN(arg, "=", 2) 直接切割：有些 flag 的值本身带等号
// （codex 的 sandbox_mode=... 就是），这里只切第一个。
func splitProviderArg(arg string) (name, value string, hasValue bool) {
	if i := strings.IndexByte(arg, '='); i >= 0 {
		return arg[:i], arg[i+1:], true
	}
	return arg, "", false
}

func validPermission(p Permission) bool {
	switch p {
	case PermissionRead, PermissionWorkspace, PermissionFull:
		return true
	default:
		return false
	}
}

func validMode(m Mode) bool {
	switch m {
	case ModeDiscussion, ModeWorker, ModeDiscussionAndWorker, ModeJudge, ModeManager:
		return true
	default:
		return false
	}
}

// validLanguage 接受空串（= auto，见 Validate 的注释）与三个合法取值。
// 大小写不敏感：手写配置里写 ZH 不该被拒绝，Load 会把它归一成小写。
func validLanguage(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", LanguageAuto, LanguageZh, LanguageEn:
		return true
	default:
		return false
	}
}

// validLocalAgentLevel 只接受空串、off、observe 与 coordinate，大小写不敏感
// （Load 会归一成小写；手写配置里写 OBSERVE 不该被拒绝）。空串接受的理由与
// validLanguage 相同：直接构造的 Config 没有经过 applyDefaults，语义上
// 空串就是 off。
//
// 为什么只认这三档：register（外部登记）与 collaborate（模型参与的协作）在
// v1 没有实现，写进合法枚举会让用户以为功能已经开了，而实际没有任何消息或
// 模型调用。coordinate 是实现了租约之后才放进来的（Plan §4.3 ③ 的取值表里
// 它的含义就是资源协调）。
func validLocalAgentLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", LocalAgentOff, LocalAgentObserve, LocalAgentCoordinate:
		return true
	default:
		return false
	}
}

func sortedIDs[V any](m map[string]V) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
