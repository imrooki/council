package headless

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// Options 是 `council ask` 的全部输入。它只由 flag 与配置决定，不读环境、
// 不碰文件：这样 options_test 可以直接喂一份手写的 Config，不必落盘。
type Options struct {
	Prompt       string
	Members      []string // 已按给出顺序
	Team         string
	Mode         string // "ask" | "round"
	Rounds       int
	Summary      string // "" 表示用配置；"none" 表示关掉
	BudgetUSD    float64
	Timeout      time.Duration
	Conversation string
	JSON         bool
	Verbose      bool

	// Workspace 是 -workspace 的仓库根。非空时成员在
	// <data_dir>\workspaces\<convID> 的隔离副本里跑，跑完把 diff 放回
	// Result.Workspace；不改原仓库、不自动 apply。空串是既有行为。
	Workspace string

	// ConfigPath 是 -config 的值。它不属于 ask 的语义，但 main 只能从这一层
	// 拿到它再去调 app.Run；app.Options 里同名字段就是从这里填的。
	// 空串的读法与全局 -config 相同：走环境变量与默认位置。
	ConfigPath string
}

// Deps 是 Run 需要的装配结果。它由 internal/app 在 open store 之后填，
// headless 自己不加载配置、不打开库、不构造 Runner。
type Deps struct {
	Cfg                 *config.Config
	Svc                 *conversation.Service
	Rn                  *turn.Runner
	DataDir, ProjectDir string
	Out, Err            io.Writer
	Verbose             bool

	// OnRecord 每记录到一条 turn 结果就调一次（在 Run 的主 goroutine 上）。
	// 它存在的理由：`council serve -rw` 要把每条结果实时推到浏览器，而 Result
	// 要等整趟跑完才拿得到。回调里不许阻塞——serve 只往内存里追加事件，
	// 绝不把网络读者接在轮次路径上。nil 时零开销，行为与没有这个字段时一致。
	OnRecord func(turn.Record)

	// OnStart 在会话确定之后（新建完或确认续接目标存在）、落问题之前调一次，
	// 参数是本次议会的会话 id。它让 serve 能在第一位成员开口之前就告诉浏览器
	// 「切到哪个会话」；只靠 OnRecord 要等一轮回来才知道 id，进度页会长时间
	// 没有目标。nil 时零开销；与 OnRecord 一样在主 goroutine 上同步调用。
	OnStart func(conversationID string)
}

// UsageError 是「命令行用错了」这一档的哨兵。退出码 2 只认它，
// 其余错误一律是执行失败（1）。用类型而不是字符串前缀，是因为
// 文案会改，退出码不能跟着改。
type UsageError struct {
	msg string
}

func (e *UsageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &UsageError{msg: fmt.Sprintf(format, args...)}
}

// ExitCode 把 Run / Parse 的返回值收敛成进程退出码。
// nil 与 partial 都是 0；用法错误 2；其余（含 budget_exceeded）1。
// main 不再自己判错误类型，映射只留这一处。
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ue *UsageError
	if errors.As(err, &ue) {
		return 2
	}
	return 1
}

// Parse 解析 `council ask` 的参数并校验。它是纯函数：不读环境、不碰文件、
// 不写任何 writer；flag 自己产生的报错也丢进 io.Discard，由调用方打印。
//
// cfg 允许为 nil：main 在 app.Run 之前只能拿到 flag，配置还没加载，
// 这一趟只做与配置无关的校验；cfg 非 nil 时（测试与 Run 内部）把成员、
// team、汇总成员一并校验。Run 会在拿到 cfg 后再调一次 Validate，
// 所以生产路径上的配置相关用法错误一个都不会漏。
func Parse(args []string, cfg *config.Config) (Options, error) {
	fs := flag.NewFlagSet("council ask", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	prompt := fs.String("p", "", "问题正文；也可以写成 -- 之后的位置参数")
	members := fs.String("members", "", "逗号分隔的 agent id，按给出顺序发言")
	team := fs.String("team", "", "团队 id；与 -members 互斥")
	mode := fs.String("mode", "ask", "ask＝各成员说一轮；round＝多轮，语义同 /round")
	rounds := fs.Int("rounds", 1, "仅 round 生效；ask 下必须是 1")
	summary := fs.String("summary", "", "汇总成员；none 显式关闭（round 下不允许关闭）")
	budget := fs.Float64("budget-usd", 5, "本次调用的硬上限，单位 USD，不得小于 0.5")
	timeout := fs.Duration("timeout", 10*time.Minute, "整体超时；到点取消全部在飞")
	conversation := fs.String("conversation", "", "续接已有会话；未知 id 退 1")
	workspace := fs.String("workspace", "", "在 git 仓库的隔离 worktree 副本里跑，跑完返回 diff")
	jsonOut := fs.Bool("json", false, "stdout 只输出一个 JSON 对象")
	verbose := fs.Bool("verbose", false, "逐成员进度写 stderr")
	configPath := fs.String("config", "", "配置文件路径，与全局 -config 含义相同")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Options{}, err
		}
		return Options{}, usagef("%v", err)
	}
	// 互斥按「flag 出现没出现」判，不按值判：-p "" 加位置参数、
	// -members "" 加 -team 都是两次表达，不能因为其中一边是空串就放过去。
	seen := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })

	positional := fs.Args()
	var promptText string
	switch {
	case seen["p"] && len(positional) > 0:
		return Options{}, usagef("-p 与 -- 后的位置参数只能给一个")
	case seen["p"]:
		promptText = *prompt
	case len(positional) > 0:
		// 位置参数不保证被 shell 合成一个；按空格拼回去，问题里的
		// 空白形状由 shell 的引号决定，本层不再加工。
		promptText = strings.Join(positional, " ")
	default:
		return Options{}, usagef("必须用 -p <prompt> 或 -- <prompt> 给出问题")
	}

	if seen["members"] && seen["team"] {
		return Options{}, usagef("-members 与 -team 不能同时给")
	}

	var memberList []string
	if raw := strings.TrimSpace(*members); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			id := strings.TrimSpace(part)
			if id == "" {
				continue
			}
			memberList = append(memberList, id)
		}
	}
	if seen["members"] && len(memberList) == 0 {
		return Options{}, usagef("-members 给了但名单是空的")
	}
	if seen["workspace"] && strings.TrimSpace(*workspace) == "" {
		return Options{}, usagef("-workspace 给了但目录是空的")
	}

	opts := Options{
		Prompt:       promptText,
		Members:      memberList,
		Team:         strings.TrimSpace(*team),
		Mode:         strings.ToLower(strings.TrimSpace(*mode)),
		Rounds:       *rounds,
		Summary:      strings.TrimSpace(*summary),
		BudgetUSD:    *budget,
		Timeout:      *timeout,
		Conversation: strings.TrimSpace(*conversation),
		Workspace:    strings.TrimSpace(*workspace),
		JSON:         *jsonOut,
		Verbose:      *verbose,
		ConfigPath:   *configPath,
	}
	if opts.Mode == "" {
		opts.Mode = "ask"
	}
	if err := Validate(opts, cfg); err != nil {
		return Options{}, err
	}
	return opts, nil
}

// Validate 是 Parse 的后半段，也可以单独调用。它在 cfg 为 nil 时跳过所有
// 跨配置的校验；Run 拿到真实 cfg 后会再调一次，因此直接构造 Options 的
// 调用方（例如测试）不会绕过任何一条。
func Validate(opts Options, cfg *config.Config) error {
	if strings.TrimSpace(opts.Prompt) == "" {
		return usagef("问题正文不能为空")
	}
	if len(opts.Members) > 0 && strings.TrimSpace(opts.Team) != "" {
		return usagef("-members 与 -team 不能同时给")
	}
	mode := opts.Mode
	if mode == "" {
		mode = "ask"
	}
	switch mode {
	case "ask":
		if opts.Rounds != 1 {
			return usagef("-mode ask 下 -rounds 必须是 1，实际 %d", opts.Rounds)
		}
	case "round":
		if opts.Rounds < 1 {
			return usagef("-mode round 下 -rounds 至少是 1，实际 %d", opts.Rounds)
		}
	default:
		return usagef("-mode 只认 ask 或 round，实际 %q", opts.Mode)
	}
	// NaN 与负数都过不了这一条；+Inf 能过 >=，但它等于没有上限，
	// 与 D5「没有不限值」相反，单独拒掉。
	if !(opts.BudgetUSD >= 0.5) || math.IsInf(opts.BudgetUSD, 0) {
		return usagef("-budget-usd 不得小于 0.5，实际 %v", opts.BudgetUSD)
	}
	if opts.Timeout <= 0 {
		return usagef("-timeout 必须大于 0，实际 %v", opts.Timeout)
	}

	if cfg == nil {
		return nil
	}
	resolved, err := resolveMembers(opts, cfg)
	if err != nil {
		return err
	}
	if allJudges(resolved, cfg.Agents) {
		return usagef("名单里的成员全是 judge 模式，没有可发言的人：%s", strings.Join(resolved, ", "))
	}
	if _, err := resolveSummary(opts, cfg); err != nil {
		return err
	}
	return nil
}

// allJudges 报告名单是不是清一色裁判。全裁判时 PlanRound 会把所有人
// 挪进 Judges，Speakers 为空；这不该等到起会话、落问题之后才用一个
// 空结果告诉用户，所以在 Validate 阶段就按用法错误拒绝。
func allJudges(members []string, agents map[string]config.AgentConfig) bool {
	if len(members) == 0 {
		return false
	}
	for _, id := range members {
		if agents[id].Mode != config.ModeJudge {
			return false
		}
	}
	return true
}

// resolveMembers 按 D2 定名单：-members 优先，其次 -team 的 members，
// 最后是配置里全部 agent 的 id（字典序）。重复的 id 只留第一次出现的那个，
// 顺序按给出顺序；每个 id 都必须在 cfg.Agents 里，否则是用法的错。
func resolveMembers(opts Options, cfg *config.Config) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("headless: 没有配置，无法解析成员")
	}
	var raw []string
	if len(opts.Members) > 0 {
		if strings.TrimSpace(opts.Team) != "" {
			return nil, usagef("-members 与 -team 不能同时给")
		}
		raw = opts.Members
	} else if teamID := strings.TrimSpace(opts.Team); teamID != "" {
		team, ok := cfg.Teams[teamID]
		if !ok {
			return nil, usagef("未知团队 %q：配置里没有这个 team", teamID)
		}
		raw = team.Members
	} else {
		raw = make([]string, 0, len(cfg.Agents))
		for id := range cfg.Agents {
			raw = append(raw, id)
		}
		sort.Strings(raw)
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, candidate := range raw {
		id := strings.TrimSpace(candidate)
		if id == "" {
			continue
		}
		if _, ok := cfg.Agents[id]; !ok {
			return nil, usagef("未知成员 %q：配置里没有这个 agent", id)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, usagef("成员名单为空")
	}
	return out, nil
}

// resolveSummary 决定这次要不要汇总、由谁汇总。只有 "none" 是关闭；
// 空串表示回到配置 [summary].agent。round 模式下关闭或没配都是用法错误
// （D4：多轮必须有人收口），ask 模式允许没有汇总。
func resolveSummary(opts Options, cfg *config.Config) (string, error) {
	mode := opts.Mode
	if mode == "" {
		mode = "ask"
	}
	raw := strings.TrimSpace(opts.Summary)
	if raw == "none" {
		if mode == "round" {
			return "", usagef("-mode round 必须有汇总成员，不能用 -summary none 关掉")
		}
		return "", nil
	}
	if raw == "" && cfg != nil {
		raw = strings.TrimSpace(cfg.Summary.Agent)
	}
	if raw == "" {
		if mode == "round" {
			return "", usagef("-mode round 没有汇总成员：在配置里写 [summary].agent，或用 -summary 指定")
		}
		return "", nil
	}
	if cfg != nil {
		ac, ok := cfg.Agents[raw]
		if !ok {
			return "", usagef("未知汇总成员 %q：配置里没有这个 agent", raw)
		}
		// 与 config.Validate 对 [summary].agent 的规矩同一条：汇总进程
		// 没有人当场点名，不给写盘能力。显式 -summary 也要守这条。
		if ac.Permissions != config.PermissionRead {
			return "", usagef("汇总成员 %q 必须是 read 权限，实际 %q", raw, ac.Permissions)
		}
	}
	return raw, nil
}

// Usage 是 -h 时打印的那段文字。flag 包自己的输出被 Parse 丢掉了，
// 帮助与用法错误都由 runAsk 打这一份，保证两处不会漂成两套。
func Usage() string {
	return `用法:
  council ask [flags] -- <prompt>
  council ask [flags] -p <prompt>

flags:
  -p <prompt>          问题正文；与 -- 后的位置参数二选一
  -members <ids>       逗号分隔的 agent id，按给出顺序发言
  -team <id>           团队 id；与 -members 互斥
  -mode ask|round      ask＝各成员说一轮（默认）；round＝多轮
  -rounds <n>          仅 round 生效，默认 1；ask 下必须是 1
  -summary <id|none>   汇总成员；默认取配置 [summary].agent，round 下不能用 none
  -budget-usd <n>      硬上限，默认 5，最小 0.5
  -timeout <duration>  整体超时，默认 10m
  -conversation <id>   续接已有会话
  -workspace <dir>     在 git 仓库的隔离 worktree 副本里跑，返回 diff
  -json                stdout 只输出一个 JSON 对象
  -verbose             逐成员进度写 stderr
  -config <path>       配置文件路径
`
}
