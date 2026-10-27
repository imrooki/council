package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"council/internal/headless"
	"council/internal/i18n"
	"council/internal/localagent"
)

// 与 `council ask` 同口径的默认值：预算下限 0.5、默认 5 USD、整体超时 10m。
// 改这里之前先看 internal/headless/options.go 的 Validate：两处必须一起动，
// 否则 MCP 会在自己这一层放行一个 CLI 会拒绝的值。
const (
	minBudgetUSD     = 0.5
	defaultBudgetUSD = 5.0
	defaultRounds    = 2
	maxRounds        = 5
	toolTimeout      = 10 * time.Minute

	// roundTurnRunes 是 council_round 里每人每轮正文的上限（按 rune 计）。
	// 多轮 × 多人的全文很容易上万字，MCP 的结果最终要进宿主的上下文；
	// 这里只截断展示，库里的消息仍是全文。council_ask 不截断：那一次调用
	// 的正文就是它要交付的东西。
	roundTurnRunes = 1200
)

// toolDefinition 是 tools/list 里的一项。输入用 JSON Schema 手写：三个工具
// 的参数都不复杂，为它们引一套 schema 生成器不值得。
type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// toolResult 是 tools/call 的 result。失败也是这个形状：
// isError=true 加上一段说明文本，不是 JSON-RPC error。
type toolResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textResult(text string) toolResult {
	return toolResult{Content: []contentBlock{{Type: "text", Text: text}}}
}

func errorResult(text string) toolResult {
	return toolResult{Content: []contentBlock{{Type: "text", Text: text}}, IsError: true}
}

func toolDefinitions() []toolDefinition {
	return []toolDefinition{
		{
			Name: "council_ask",
			Description: "Ask the locally configured Council members a question. " +
				"Each member answers once; an optional summary agent then writes one synthesis. " +
				"Returns one text block: each member's reply, then a cost line (e.g. $0.0123, marked incomplete when a provider does not report cost). " +
				"Costs real quota.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "The question or task. Required and must not be blank.",
					},
					"members": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Agent ids to ask, in speaking order. Omit or pass [] to use every member defined in the config (members imported from .claude/agents are only used when named explicitly).",
					},
					"rounds": map[string]any{
						"type":        "integer",
						"minimum":     1,
						"maximum":     1,
						"description": "council_ask is single-round; only 1 is accepted. Use council_round for multiple rounds.",
					},
					"summary": map[string]any{
						"type":        "string",
						"description": "Summary agent id, or \"none\" to disable. Omit to use [summary].agent from the config.",
					},
					"budget_usd": map[string]any{
						"type":        "number",
						"minimum":     minBudgetUSD,
						"description": "Hard cost cap in USD. Must be at least 0.5; default 5.",
					},
					"cwd": map[string]any{
						"type":        "string",
						"description": "Existing absolute directory all members run in. Omit for the default per-agent workdirs.",
					},
					"conversation": map[string]any{
						"type":        "string",
						"description": "Existing conversation id to continue. Omit to create a new conversation.",
					},
					"lang": map[string]any{
						"type":        "string",
						"enum":        []string{"zh", "en"},
						"description": "Language of the returned text. Defaults to the server's language.",
					},
				},
				"required":             []string{"prompt"},
				"additionalProperties": false,
			},
		},
		{
			Name: "council_models",
			Description: "List the Council members available to this server: " +
				"id, display name, provider, model, permissions and source (config or .claude/agents). " +
				"Also reports Claude agents that exist but were not imported because [providers.claude] is missing. " +
				"Takes no arguments and costs nothing.",
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			Name: "council_round",
			Description: "Run a multi-round Council debate (1-5 rounds) and a summary. " +
				"Returns one text block: the summary, each round's member replies (long replies are excerpted), then a cost line. " +
				"A summary member must exist in the config or be passed explicitly. Costs real quota and takes longer than council_ask.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "The question or task. Required and must not be blank.",
					},
					"members": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Agent ids to ask, in speaking order. Omit or pass [] to use every member defined in the config.",
					},
					"rounds": map[string]any{
						"type":        "integer",
						"minimum":     1,
						"maximum":     maxRounds,
						"default":     defaultRounds,
						"description": "Number of debate rounds, 1-5. Default 2.",
					},
					"summary": map[string]any{
						"type":        "string",
						"description": "Summary agent id (read-only). Omit to use [summary].agent from the config. \"none\" is rejected for multi-round debates.",
					},
					"budget_usd": map[string]any{
						"type":        "number",
						"minimum":     minBudgetUSD,
						"description": "Hard cost cap in USD. Must be at least 0.5; default 5.",
					},
				},
				"required":             []string{"prompt"},
				"additionalProperties": false,
			},
		},
		{
			Name: "council_who",
			Description: "List the agent processes visible on this machine (claude / codex / grok / agy), read-only. " +
				"Requires local_agent = \"observe\" in the config; when observation is off it explains that and enumerates nothing. " +
				"Returns one text block with kind, pid, ppid, exe name, start time, source and certainty " +
				"(exact only when the image path is under a known CLI install directory; probable otherwise). " +
				"It reads no command lines, environment variables or other CLIs' config files, and never calls a model.",
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
	}
}

func knownTool(name string) bool {
	switch name {
	case "council_ask", "council_models", "council_round", "council_who":
		return true
	default:
		return false
	}
}

func (s *server) callTool(ctx context.Context, name string, raw json.RawMessage) toolResult {
	switch name {
	case "council_ask":
		var args askArgs
		if err := decodeToolArgs(raw, &args); err != nil {
			return errorResult(fmt.Sprintf(messagesFor(s.lang).errArgs, err))
		}
		return s.toolAsk(ctx, args)
	case "council_round":
		var args roundArgs
		if err := decodeToolArgs(raw, &args); err != nil {
			return errorResult(fmt.Sprintf(messagesFor(s.lang).errArgs, err))
		}
		return s.toolRound(ctx, args)
	case "council_models":
		return s.toolModels()
	case "council_who":
		return s.toolWho()
	default:
		// handleToolCall 已经挡过未知工具名，这里只是不让未来新增的分支漏掉。
		return errorResult(fmt.Sprintf("unknown tool %q", name))
	}
}

// decodeToolArgs 把 arguments 解进目标结构体。缺省、null 与 {} 都按「没有
// 参数」处理；字段类型不对（例如 prompt 给了数字）是工具层错误，按 isError
// 返回，不升级成 JSON-RPC error —— MCP 的 error 留给协议本身。
func decodeToolArgs(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, target)
}

type askArgs struct {
	Prompt       string   `json:"prompt"`
	Members      []string `json:"members"`
	Rounds       *int     `json:"rounds"`
	Summary      string   `json:"summary"`
	BudgetUSD    *float64 `json:"budget_usd"`
	Cwd          string   `json:"cwd"`
	Conversation string   `json:"conversation"`
	Lang         string   `json:"lang"`
}

type roundArgs struct {
	Prompt    string   `json:"prompt"`
	Members   []string `json:"members"`
	Rounds    *int     `json:"rounds"`
	Summary   string   `json:"summary"`
	BudgetUSD *float64 `json:"budget_usd"`
}

func (s *server) toolAsk(ctx context.Context, args askArgs) toolResult {
	lang, errText := s.resolveLang(args.Lang)
	if errText != "" {
		return errorResult(errText)
	}
	t := messagesFor(lang)
	opts, errText := s.buildAskOptions(args, lang, t)
	if errText != "" {
		return errorResult(errText)
	}
	return s.runAndFormat(ctx, opts, lang, t, "council_ask", false)
}

func (s *server) toolRound(ctx context.Context, args roundArgs) toolResult {
	t := messagesFor(s.lang)
	opts, errText := s.buildRoundOptions(args, s.lang, t)
	if errText != "" {
		return errorResult(errText)
	}
	return s.runAndFormat(ctx, opts, s.lang, t, "council_round", true)
}

func (s *server) resolveLang(raw string) (i18n.Lang, string) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return s.lang, ""
	case "zh":
		return i18n.LangZh, ""
	case "en":
		return i18n.LangEn, ""
	default:
		return s.lang, fmt.Sprintf(messagesFor(s.lang).errLang, raw)
	}
}

func (s *server) buildAskOptions(args askArgs, lang i18n.Lang, t *messages) (headless.Options, string) {
	if s.deps.Cfg == nil {
		return headless.Options{}, t.errNoConfig
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return headless.Options{}, t.errPrompt
	}
	if args.Rounds != nil && *args.Rounds != 1 {
		return headless.Options{}, fmt.Sprintf(t.errRoundsAsk, *args.Rounds)
	}

	opts := headless.Options{
		Prompt:       args.Prompt,
		Mode:         "ask",
		Rounds:       1,
		Summary:      strings.TrimSpace(args.Summary),
		BudgetUSD:    defaultBudgetUSD,
		Timeout:      toolTimeout,
		Conversation: strings.TrimSpace(args.Conversation),
		Cwd:          strings.TrimSpace(args.Cwd),
		JSON:         true,
		Lang:         lang,
	}
	if args.BudgetUSD != nil {
		if !(*args.BudgetUSD >= minBudgetUSD) {
			return headless.Options{}, fmt.Sprintf(t.errBudget, *args.BudgetUSD)
		}
		opts.BudgetUSD = *args.BudgetUSD
	}

	members, errText := s.resolveMembers(args.Members, t)
	if errText != "" {
		return headless.Options{}, errText
	}
	opts.Members = members

	if opts.Cwd != "" {
		if !filepath.IsAbs(opts.Cwd) {
			return headless.Options{}, fmt.Sprintf(t.errCwdAbs, opts.Cwd)
		}
		info, err := os.Stat(opts.Cwd)
		if err != nil || !info.IsDir() {
			return headless.Options{}, fmt.Sprintf(t.errCwdDir, opts.Cwd)
		}
	}

	// 最后交给 headless 的同一套校验：未知汇总成员、全裁判、round 缺汇总、
	// 预算下限这些规则只写在一处（internal/headless/options.go），MCP 不再
	// 复制第二份。这里先调一次只是为了让错误在起任何进程之前出现。
	if err := headless.Validate(opts, s.deps.Cfg); err != nil {
		return headless.Options{}, err.Error()
	}
	return opts, ""
}

func (s *server) buildRoundOptions(args roundArgs, lang i18n.Lang, t *messages) (headless.Options, string) {
	if s.deps.Cfg == nil {
		return headless.Options{}, t.errNoConfig
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return headless.Options{}, t.errPrompt
	}

	rounds := defaultRounds
	if args.Rounds != nil {
		rounds = *args.Rounds
	}
	if rounds < 1 || rounds > maxRounds {
		return headless.Options{}, fmt.Sprintf(t.errRoundsRange, rounds)
	}

	opts := headless.Options{
		Prompt:    args.Prompt,
		Mode:      "round",
		Rounds:    rounds,
		Summary:   strings.TrimSpace(args.Summary),
		BudgetUSD: defaultBudgetUSD,
		Timeout:   toolTimeout,
		JSON:      true,
		Lang:      lang,
	}
	if args.BudgetUSD != nil {
		if !(*args.BudgetUSD >= minBudgetUSD) {
			return headless.Options{}, fmt.Sprintf(t.errBudget, *args.BudgetUSD)
		}
		opts.BudgetUSD = *args.BudgetUSD
	}

	members, errText := s.resolveMembers(args.Members, t)
	if errText != "" {
		return headless.Options{}, errText
	}
	opts.Members = members

	if err := headless.Validate(opts, s.deps.Cfg); err != nil {
		return headless.Options{}, err.Error()
	}
	return opts, ""
}

// resolveMembers 定名单。没给（或给了空数组）时用配置里的成员；显式给了
// 就必须每一个都在成员表里，并点名所有不认识的 id。
//
// 「配置里的成员」不含从 .claude/agents 导入的那几位：`council ask` 在
// CLI 路径上根本看不到它们，MCP 的默认名单若是把导入成员也算进去，同一个
// 问题在两条入口上会问到不同的人。要问导入成员，显式写 members。
func (s *server) resolveMembers(raw []string, t *messages) ([]string, string) {
	members := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, candidate := range raw {
		id := strings.TrimSpace(candidate)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		members = append(members, id)
	}
	if len(raw) > 0 && len(members) == 0 {
		return nil, t.errMembersEmpty
	}
	if len(members) == 0 {
		return s.defaultMembers(t)
	}

	var unknown []string
	for _, id := range members {
		if _, ok := s.deps.Cfg.Agents[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Sprintf(t.errUnknownMember,
			quoteIDs(unknown, t.listSep), strings.Join(s.availableMemberIDs(), t.listSep))
	}
	return members, ""
}

func (s *server) defaultMembers(t *messages) ([]string, string) {
	ids := make([]string, 0, len(s.deps.Cfg.Agents))
	for id := range s.deps.Cfg.Agents {
		if _, imported := s.deps.Claude.Merged[id]; imported {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return nil, t.errNoMembers
	}
	return ids, ""
}

func (s *server) availableMemberIDs() []string {
	return sortedAgentIDs(s.deps.Cfg.Agents)
}

func (s *server) toolModels() toolResult {
	t := messagesFor(s.lang)
	if s.deps.Cfg == nil {
		return errorResult(t.noConfig)
	}

	ids := sortedAgentIDs(s.deps.Cfg.Agents)
	var b strings.Builder
	fmt.Fprintf(&b, t.modelsHead+"\n", len(ids))
	if len(ids) == 0 {
		b.WriteString(t.modelsEmpty + "\n")
	}
	for _, id := range ids {
		agent := s.deps.Cfg.Agents[id]
		model := agent.Model
		if strings.TrimSpace(model) == "" {
			model = t.modelNoModel
		}
		source := "config"
		if _, imported := s.deps.Claude.Merged[id]; imported {
			source = "claude"
		}
		fmt.Fprintf(&b, t.modelRow+"\n",
			id, agent.DisplayName, agent.Provider, model, string(agent.Permissions), source)
	}
	if n := len(s.deps.Claude.WithoutProvider); n > 0 {
		skipped := sortedAgentIDs(s.deps.Claude.WithoutProvider)
		fmt.Fprintf(&b, t.importedSkipped+"\n", n, quoteIDs(skipped, t.listSep))
	}
	return textResult(strings.TrimRight(b.String(), "\n"))
}

// enumerate 是 council_who 的进程枚举接缝。生产值就是
// localagent.Enumerate（Windows 上走 Toolhelp32），只有测试会换掉它。
//
// 为什么要接缝：off 分支必须能被证明「一次枚举都没有发生」。注入一个会
// panic 的实现比断言输出文本更硬 —— 调用方只要碰了枚举函数，测试立刻崩。
var enumerate = localagent.Enumerate

// toolWho 是 council_who 的实现：只读、无参数、不新增任何模型调用。
//
// off 时返回一句说明并**不枚举**：localagent.Scan 在 LevelOff 那一侧就
// 返回了，连函数指针都不调（S-01）。开启后输出与 toolModels 同构：
// 一行表头 + 每实例一行 + 一行固定脚注，整体是一个 text block。
func (s *server) toolWho() toolResult {
	t := messagesFor(s.lang)

	// 配置没加载时按 off 处理：没有配置就没有用户授权，不能默认打开扫描。
	level := localagent.LevelOff
	if s.deps.Cfg != nil {
		level = localagent.LevelOf(s.deps.Cfg.LocalAgent)
	}
	if level != localagent.LevelObserve {
		return textResult(t.whoOff)
	}

	agents, err := localagent.Scan(level, enumerate)
	if err != nil {
		return errorResult(fmt.Sprintf(t.whoUnavailable, err))
	}

	var b strings.Builder
	fmt.Fprintf(&b, t.whoHead+"\n", len(agents))
	if len(agents) == 0 {
		b.WriteString(t.whoEmpty + "\n")
	}
	for _, agent := range agents {
		started := "unknown"
		if !agent.StartedAt.IsZero() {
			started = agent.StartedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(&b, t.whoRow+"\n", agent.Kind, agent.PID, agent.PPID, agent.ExeName, started, agent.Source, agent.Certainty)
	}
	b.WriteString(t.whoFooter)
	return textResult(strings.TrimRight(b.String(), "\n"))
}

// runAndFormat 是三个工具共用的执行路径：把 headless 的 JSON 结果接进内存，
// 再按要交付的形状排成文本。JSON 必须开着：这里要按成员、按轮次重组，
// 不能把 headless 的文本模式原样交给宿主。
func (s *server) runAndFormat(ctx context.Context, opts headless.Options, lang i18n.Lang, t *messages, toolName string, round bool) toolResult {
	if s.deps.Cfg == nil {
		return errorResult(t.errNoConfig)
	}
	if s.deps.Svc == nil || s.deps.Rn == nil {
		return errorResult(t.errNotReady)
	}

	var buf bytes.Buffer
	runErr := headless.Run(ctx, headless.Deps{
		Cfg:        s.deps.Cfg,
		Svc:        s.deps.Svc,
		Rn:         s.deps.Rn,
		DataDir:    s.deps.DataDir,
		ProjectDir: s.deps.ProjectDir,
		Out:        &buf,
		Err:        s.err,
		Verbose:    s.deps.Verbose,
	}, opts)
	if runErr != nil {
		s.logf(fmt.Sprintf("%s 失败: %v", toolName, runErr))
	}

	var res headless.Result
	decoded := buf.Len() > 0 && json.Unmarshal(buf.Bytes(), &res) == nil
	if !decoded {
		if runErr != nil {
			return errorResult(fmt.Sprintf(t.errRunFailed, toolName, runErr))
		}
		return errorResult(fmt.Sprintf(t.errNoResult, toolName))
	}

	s.logf(fmt.Sprintf("%s 完成: 状态=%s 会话=%s 花费=%.4f 不完整=%v",
		toolName, res.Status, res.ConversationID, res.CostUSD, !res.CostComplete))
	return toolResult{
		Content: []contentBlock{{Type: "text", Text: s.formatResult(res, runErr, t, round)}},
		IsError: runErr != nil,
	}
}

// formatResult 把一次议会排成交付文本。形状是稳定的（测试按行断言其中
// 的标题、正文与费用行）：
//
//	会话: …
//	状态: …（仅非 ok / 有失败时）
//	错误: …（仅 runErr 非空时）
//	成员 <id>（<显示名>）:
//	<正文>
//	…
//	费用: $0.0000（不完整）
//
// round 时把成员段换成「汇总 + 第 N 轮」。
func (s *server) formatResult(res headless.Result, runErr error, t *messages, round bool) string {
	var b strings.Builder
	if res.ConversationID != "" {
		fmt.Fprintf(&b, t.conversation+"\n", res.ConversationID)
	}
	switch {
	case res.Status == "partial":
		fmt.Fprintf(&b, t.statusPartial+"\n", res.Failed, res.Total)
	case res.Status != "" && res.Status != "ok":
		fmt.Fprintf(&b, t.status+"\n", res.Status)
	}
	if runErr != nil {
		fmt.Fprintf(&b, t.errorLine+"\n", runErr.Error())
	}

	if round {
		s.formatRoundBody(&b, res, t)
	} else {
		s.formatAskBody(&b, res, t)
	}
	s.formatCost(&b, res, t)
	return strings.TrimRight(b.String(), "\n")
}

func (s *server) formatAskBody(b *strings.Builder, res headless.Result, t *messages) {
	for _, turn := range res.Turns {
		s.writeTurn(b, t, turn, 0)
	}
	if summary := strings.TrimRight(strings.TrimSpace(res.Summary), "\n"); summary != "" {
		fmt.Fprintf(b, "%s\n%s\n\n", t.summaryHead, summary)
	}
}

func (s *server) formatRoundBody(b *strings.Builder, res headless.Result, t *messages) {
	fmt.Fprintf(b, "%s\n", t.summaryHead)
	if summary := strings.TrimRight(strings.TrimSpace(res.Summary), "\n"); summary != "" {
		fmt.Fprintf(b, "%s\n\n", summary)
	} else {
		fmt.Fprintf(b, "%s\n\n", t.summaryEmpty)
	}

	currentRound := 0
	for _, turn := range res.Turns {
		if turn.Round != currentRound {
			currentRound = turn.Round
			fmt.Fprintf(b, t.roundHead+"\n", currentRound)
		}
		s.writeTurn(b, t, turn, roundTurnRunes)
	}
}

func (s *server) writeTurn(b *strings.Builder, t *messages, turn headless.TurnJSON, maxRunes int) {
	name := ""
	if s.deps.Cfg != nil {
		name = strings.TrimSpace(s.deps.Cfg.Agents[turn.AgentID].DisplayName)
	}
	if name != "" && name != turn.AgentID {
		fmt.Fprintf(b, t.memberHead+"\n", turn.AgentID, name)
	} else {
		fmt.Fprintf(b, t.memberHeadSame+"\n", turn.AgentID)
	}

	text := strings.TrimRight(turn.Text, "\n")
	if maxRunes > 0 {
		text = truncateRunes(text, maxRunes, t.truncated)
	}
	if strings.TrimSpace(text) == "" {
		// 没有正文的成员也必须出现：安静消失比一条「失败」更难排查。
		fmt.Fprintf(b, t.noText+"\n", turn.Outcome)
	} else {
		b.WriteString(text)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// formatCost 永远写最后一行。不完整只跟「provider 报不报金额」有关：
// 有成员失败时它的 0 元是真实的 0（没跑），不是「没报」。
func (s *server) formatCost(b *strings.Builder, res headless.Result, t *messages) {
	line := fmt.Sprintf(t.costLine, res.CostUSD)
	hasContent := len(res.Turns) > 0 || strings.TrimSpace(res.Summary) != ""
	if !res.CostComplete && hasContent {
		line += t.incomplete
	}
	b.WriteString(line)
	b.WriteString("\n")
}

func truncateRunes(text string, max int, suffix string) string {
	if max <= 0 || utf8.RuneCountInString(text) <= max {
		return text
	}
	return string([]rune(text)[:max]) + suffix
}

func quoteIDs(ids []string, sep string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	return strings.Join(quoted, sep)
}
