package provider

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// claudeProvider 适配 claude 2.1.278 的 headless JSON 输出。
//
// 命令行形态（09 B 节，2026-09-21 本机实测）：
//
//	-p --output-format json --model <m> [--effort <e>]
//	--append-system-prompt-file <workdir>/council-role.txt
//	--permission-mode default [--resume <id>]
//	正文走 stdin，不放进 argv。
//
// 身份 flag 每轮都发，不做「首轮才发」：那会改缓存前缀（D-4）。
// 即使 Role 是空串也照发：flag 在不在是缓存前缀的一部分，只是载体从
// --append-system-prompt 换成了 --append-system-prompt-file。
//
// 为什么走文件和 stdin，而不是把身份块和正文放进 argv：Windows 上
// claude 的可执行文件是 npm 装出来的 .cmd 垫片。Go 的 exec 起 .cmd 时，
// 命令行由 cmd.exe 再解析一遍，argv 元素里的换行会被当成命令分隔符，
// 整条命令行在第一个换行处截断。身份块天然多行（renderIdentity 产出
// 「You are X (id=y).」换行「Provider: z」换行「role」）。截断后若剩下
// 的命令行仍然合法，claude 会带着被截短的系统提示跑完、退出码 0、
// JSON 合法、游标照推 —— 身份错了、缓存前缀错了，没有任何一层会报错。
// 对照实验（step6-smoke）：--append-system-prompt-file 加 stdin 参数
// 解析通过；单行 --append-system-prompt 加位置参数正文 exit 0。
type claudeProvider struct{}

func init() {
	Register(claudeProvider{})
}

func (claudeProvider) ID() string { return "claude" }

func (claudeProvider) Capabilities() Capabilities {
	return Capabilities{
		SessionID:      SessionIDReturnedOnFirstTurn,
		Effort:         EffortPerTurnFlag,
		Identity:       IdentitySystemPromptAppend,
		Prompt:         PromptTransportStdin, // 2026-09-21 实测：-p 模式下 stdin 可读；正文走 stdin，不放 argv（.cmd 垫片会把 argv 里的换行当成命令分隔符）
		MaxPromptBytes: 0,                    // stdin 没有 argv 上限
		Permissions:    []PermissionTier{PermissionRead, PermissionWorkspace},
		UsageFormula:   "input_tokens + cache_creation_input_tokens + cache_read_input_tokens",

		OutputSchema:         SupportNo,  // 02 第 1.1 节能力表为破折号，本机没见过对应 flag
		MaxTurnsFlag:         SupportNo,  // 无 --max-turns
		ReportsCost:          SupportYes, // 只有 claude 报 total_cost_usd，且是会话累计值
		ReportsCacheCreate:   SupportYes,
		ReportsObservedModel: SupportYes, // modelUsage 的键，例如 claude-fable-5-1
		// CleanInstructionEnv 从 Unknown 升到 Yes（2026-09-21 实测）。
		// 手段不是改 HOME —— 那条路会把登录态一起带走，是 Step 6 L 节 #13
		// 记着的死结 —— 而是 --safe-mode：它关掉 CLAUDE.md 自动发现、skills、
		// plugins、hooks、MCP，而明写保留 auth、model、built-in tools 与 permissions。
		// 验证方法：身份块里放一个可检测的标记，同时看用户全局 CLAUDE.md 的
		// 首行规则（「回复第一句必须是 收到，Griffin。」）还在不在。
		// 结果是标记照常出现、那条规则的痕迹消失 —— 身份注入没被一起关掉，
		// 继承来的规则被关掉了，正是 06 第 4 节要的。
		CleanInstructionEnv: SupportYes,
		// CacheAcrossTurns = SupportYes：D-4 的 Step 0 探针（三轮 resume），
		// T1 cache_create=47749 cache_read=0，T3 cache_create=95 cache_read=47752。
		CacheAcrossTurns: SupportYes,
		// WriteBoundary = SupportYes（11c 契约第 2 节表）：--restricted 明写
		// confines file tools to the working directories；实测目录内写得进、
		// 兄弟目录 ..\outside 被拒，它自述 file tools are restricted to the
		// current working directory；拒绝记进输出的 permission_denials 字段。
		WriteBoundary: SupportYes,
		// ModelDiscovery = ControlHandshake：claude 没有列出模型的子命令（#45）。
		// 识别走 SDK 控制协议的 initialize 握手：写入 initialize，
		// 等到匹配的 control_response 再关 stdin。取值与依据见 discovery.go。
		ModelDiscovery: ModelDiscoveryControlHandshake,
	}
}

func (p claudeProvider) BuildTurn(req TurnRequest) (Command, error) {
	caps := p.Capabilities()
	if err := validateRequest(caps, req); err != nil {
		return Command{}, err
	}
	if err := checkArgvPrompt(caps, req.Prompt); err != nil {
		return Command{}, err
	}

	perm, err := claudePermissionArgs(req.Permission)
	if err != nil {
		return Command{}, err
	}

	rolePath := filepath.Join(req.WorkDir, "council-role.txt")

	args := make([]string, 0, 16)
	args = append(args, "-p", "--output-format", "json")
	args = append(args, "--model", req.Model)
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}
	// 每轮都发，包括 Role 为空串：flag 在不在是缓存前缀的一部分。
	// 载体是 --append-system-prompt-file 而不是 --append-system-prompt：
	// 身份块天然多行，不能放进 .cmd 垫片的 argv。
	args = append(args, "--append-system-prompt-file", rolePath)
	args = append(args, perm...)
	if req.ProviderSession != "" {
		args = append(args, "--resume", req.ProviderSession)
	}

	return Command{
		Args:      args,
		Dir:       req.WorkDir,
		Env:       nil,
		Stdin:     []byte(req.Prompt),
		Files:     map[string][]byte{rolePath: []byte(req.Role)},
		Transport: PromptTransportStdin,
		Dropped:   dropMaxTurns(caps, req),
	}, nil
}

func claudePermissionArgs(t PermissionTier) ([]string, error) {
	switch t {
	case PermissionRead:
		// READ = 只读讨论。三个 flag 缺一不可，2026-09-21 逐条实测过。
		//
		// --permission-mode default 单独一个**挡不住写盘**。本机实测：
		// 用与本函数此前完全相同的参数让 claude 建一个文件，它建出来了。
		// 根因是它会读用户的 ~/.claude/settings.json，而那份文件这台机器上写着
		// permissions.defaultMode = "bypassPermissions" 且 allow 里有 Write(*)、
		// Edit(*)、Bash(*)。所以有效权限是「命令行 flag 与用户配置文件的合集」，
		// 而 06 第 2 节「权限档直接映射到各 CLI 的原生 flag」这条前提是不成立的。
		//
		// --restricted 是那道真闸：它明写「ignores user, project and local
		// settings files」，并且「confines the file tools to the working
		// directories」、「refuses bypassPermissions」。加上它之后同一个写请求
		// 被拒，而且拒绝记进了输出的 permission_denials 字段。
		//
		// --safe-mode 关掉 CLAUDE.md 自动发现、skills、plugins、hooks、MCP，
		// 而**保留 auth、model、built-in tools 与 permissions**。它同时解决三件事：
		// 一是 06 第 4 节要求的指令环境隔离 —— 不加它时 grok 与 claude 都在读
		// 用户为主代理写的 ~/.claude/CLAUDE.md（15016 字节，里面有设备表与内网 IP；
		// grok inspect 把它报成 ~3754 tokens，但 15016/4 正好整除，那是 bytes/4
		// 的启发式不是真分词，别当实测引用 —— 真实测的是下面那两个 token 数），
		// 加了之后实测那份规则的痕迹消失，而 --append-system-prompt-file 注入的
		// 身份块照常生效；二是省钱，同一个被拒的写请求 cache_create 从 22357
		// 掉到 3956，金额从 0.0485 掉到 0.0140 美元；三是 Step 6 L 节 #13 记着的
		// 那个死结（改 HOME 隔离规则文件会把登录态一起带走）被它绕开了。
		//
		// 三个都显式发，不省略去「碰默认」——省略与显式在缓存前缀上不是同一个字节。
		return []string{"--safe-mode", "--restricted", "--permission-mode", "default"}, nil
	case PermissionWorkspace:
		// WORKSPACE 沿用同样的 --safe-mode 与 --restricted，只把权限模式换成
		// acceptEdits。实测这一组合给得出真边界：目录内写得进去，
		// 兄弟目录 ..\\outside 被拒，claude 自己的回答是
		// 「file tools are restricted to the current working directory」。
		// 注意 --restricted 同时「refuses bypassPermissions」，所以这条路
		// 天然到不了 FULL 档 —— 那正是我们要的。
		// 本档在 11c 第二片已经放行（turn.TranslatePermission 不再硬拒
		// workspace，provider.validateRequest 的写入边界闸也认这一家）。
		return []string{"--safe-mode", "--restricted", "--permission-mode", "acceptEdits"}, nil
	default:
		// FULL 无对应。Capabilities.Permissions 没列出的档走到这里也拒绝，不静默降档。
		return nil, fmt.Errorf("permission tier %q: %w", t, ErrUnsupportedTier)
	}
}

type claudeOutput struct {
	Type           string          `json:"type"`
	Subtype        string          `json:"subtype"`
	IsError        *bool           `json:"is_error"`
	APIErrorStatus json.RawMessage `json:"api_error_status"`
	TerminalReason string          `json:"terminal_reason"`
	StopReason     string          `json:"stop_reason"`
	SessionID      string          `json:"session_id"`
	UUID           string          `json:"uuid"`
	Result         string          `json:"result"`
	TotalCostUSD   *float64        `json:"total_cost_usd"`
	Usage          *claudeUsage    `json:"usage"`
	ModelUsage     json.RawMessage `json:"modelUsage"`
	NumTurns       json.RawMessage `json:"num_turns"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	OutputTokensDetails      struct {
		ThinkingTokens int `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

func (claudeProvider) ParseTurn(stdout []byte) (TurnResult, error) {
	// fail-closed：失败不一定产出 JSON。实测 claude --resume <不存在的 id>
	// --output-format json 的 stdout 是 0 字节，错误只在 stderr 纯文本里。
	if emptyStdout(stdout) {
		return TurnResult{}, fmt.Errorf("claude: stdout is empty: %w", ErrUnparseableOutput)
	}
	var raw claudeOutput
	if err := json.Unmarshal(stdout, &raw); err != nil {
		return TurnResult{}, fmt.Errorf("claude: stdout is not JSON: %w", ErrUnparseableOutput)
	}

	usage := claudeUsageToUsage(raw.Usage, raw.TotalCostUSD)
	observed, nkeys := observedFromModelUsage(raw.ModelUsage)
	base := TurnResult{
		ProviderSession: raw.SessionID,
		ObservedModel:   observed,
		ModelKeys:       nkeys,
		StopRaw:         raw.StopReason,
		Usage:           usage,
		UsageRaw:        rawTopLevelField(stdout, "usage"),
	}

	if !claudeSuccess(raw) {
		if !claudeShaped(raw) {
			return TurnResult{}, fmt.Errorf("claude: stdout is not a known success shape: %w", ErrUnparseableOutput)
		}
		// 自报失败：钱可能已经花了，Usage / StopRaw / session_id 尽量带回。
		// 绝不把 result 里的错误正文当成回答（陷阱 12）。2026-09-20 实测的那份
		// 样本里 result 装的是「There is an issue with the selected model ...」，
		// 而 type / subtype / session_id / usage 一应俱全，只判形状会把它当回答。
		//
		// 诊断信息里不能只报 subtype：它在失败时仍然是 "success"。带上真正会变的
		// terminal_reason 与 api_error_status，否则读日志的人会以为这一轮成功了。
		apiStatus := "<absent>"
		if len(raw.APIErrorStatus) > 0 {
			apiStatus = string(raw.APIErrorStatus)
		}
		base.Text = ""
		base.Completion = CompletionUnknown
		return base, fmt.Errorf("claude: turn failed (terminal_reason=%q api_error_status=%s stop_reason=%q subtype=%q): %w", raw.TerminalReason, apiStatus, raw.StopReason, raw.Subtype, ErrTurnFailed)
	}

	base.Text = raw.Result
	base.Thought = ""
	base.Completion = completionFrom(base.Text, raw.StopReason)
	if raw.SessionID == "" {
		// 顶层有两个长得像 UUID 的键：session_id 三轮不变，uuid 每轮都变。
		// 抄成 uuid 的后果是每轮冷启动、每次约 0.955 美元，而且全程不报错。
		// 这里取的是 session_id；它为空就不能静默当 id 存进去。
		return base, fmt.Errorf("claude: success shape carries empty session_id (uuid=%q is the record id, not the session): %w", raw.UUID, ErrNoSessionID)
	}
	return base, nil
}

func claudeSuccess(v claudeOutput) bool {
	// 已知成功形状是这些字段并存，不是退出码。缺一项就不是已知成功。
	//
	// 这个合取不是保守起见，是被实测打过的：2026-09-20 抓到的失败样本
	// （step6-probe/claude-badmodel.out，传一个不存在的 model 名触发）里，
	// subtype 仍然是 "success"。只看 subtype 会把一次 404 判成成功，
	// 把错误正文当回答存进只追加的事实源。那份样本被下面四道独立拦下：
	// is_error=true、terminal_reason="api_error"、api_error_status=404、
	// stop_reason="stop_sequence"（第四个取值，之前三轮成功样本里没见过）。
	// 留着 subtype 这一项不是因为它有用，是因为删掉它等于宣称我们知道它没用。
	if v.Type != "result" {
		return false
	}
	if v.IsError == nil || *v.IsError {
		return false
	}
	if v.Subtype != "success" {
		return false
	}
	if v.TerminalReason != "completed" {
		return false
	}
	if !absentOrJSONNull(v.APIErrorStatus) {
		return false
	}
	if v.StopReason != "end_turn" && v.StopReason != "max_tokens" {
		return false
	}
	return true
}

func claudeShaped(v claudeOutput) bool {
	return v.Type != "" || v.Subtype != "" || v.SessionID != "" || v.StopReason != "" || v.IsError != nil || v.TerminalReason != ""
}

func claudeUsageToUsage(u *claudeUsage, cost *float64) Usage {
	out := Usage{CostBasis: CostBasisCumulativeSession}
	if cost != nil {
		out.CostUSDTotal = floatPtr(*cost)
	}
	if u == nil {
		out.Unreported = appendUnreported(out.Unreported, "input_tokens", "cache_create", "cache_read", "output_tokens", "thinking_tokens")
		return out
	}
	// 单轮口径取 usage，不是 modelUsage（陷阱 1）。
	// claude 的 input_tokens 不含缓存；完整 prompt = 三项相加
	// （t1: 2+47749+0=47751；t2: 2+19363+28436=47801）。
	out.InputTokens = u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	out.CacheCreate = intPtr(u.CacheCreationInputTokens)
	out.CacheRead = u.CacheReadInputTokens
	out.OutputTokens = u.OutputTokens
	// 思考 token 嵌两层 usage.output_tokens_details.thinking_tokens（陷阱 4）。
	// 三轮实测恒 0，包含关系未能证实，按「已含在 OutputTokens 内」处理，不许相加（陷阱 5）。
	out.ThinkingTokens = u.OutputTokensDetails.ThinkingTokens
	return out
}
