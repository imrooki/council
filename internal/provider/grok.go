package provider

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// grokProvider 适配 grok 1.0.13（经本机 CPA）的 headless JSON 输出。
//
// 空会话发 --session-id <本地现铸 uuid>，非空发 --resume <id>。
// 身份用 --system-prompt-override，每轮都发。
// 不发任何 effort flag：经 CPA 会被能力门静默丢弃，退出码仍是 0（02 第 1.1 节）。
//
// 本机实测：grok 拿到本地不认识的会话 id 时不报错，去远端恢复，拉起交互式
// OAuth 设备码流程然后无限等待授权（4 分钟只吃 0.6 秒 CPU）。契约的
// Capabilities 没有对应字段；上层必须把超时当主路径，不能指望它报错。
type grokProvider struct{}

func init() {
	Register(grokProvider{})
}

func (grokProvider) ID() string { return "grok" }

func (grokProvider) Capabilities() Capabilities {
	return Capabilities{
		SessionID:      SessionIDPreassignable,
		Effort:         EffortProcessExternalConfig,
		Identity:       IdentitySystemPromptOverride,
		Prompt:         PromptTransportArgv, // -p 把正文放进 argv 是实测路径；stdin 是否可读未测，本步不用 stdin
		MaxPromptBytes: windowsArgvPromptLimit,
		Permissions:    []PermissionTier{PermissionRead},
		UsageFormula:   "input_tokens + cache_read_input_tokens",

		OutputSchema:         SupportYes, // 02 第 1.1 节：--json-schema。本步 TurnRequest 没有这项，不发
		MaxTurnsFlag:         SupportYes, // --max-turns
		ReportsCost:          SupportNo,  // 一个金额字段都没有，必须是「没有这个数」，不是 0
		ReportsCacheCreate:   SupportYes, // 键在，但实测恒为 0，即使 cache_read 从 0 涨到 20608（陷阱 7）
		ReportsObservedModel: SupportYes, // 键是 grok-4.6-build，与请求的 grok-4.6 不是同一个字符串
		// CleanInstructionEnv 从 Unknown 降到 **No**（2026-09-21 实测）。
		// 这一位不是「还没测」，是**测过了、做不到**：
		// grok inspect 直接列出它读用户的 ~/.claude/Claude.md（15016 字节）
		// 与 ~/.claude/settings.json，外加 51 个用户 skill。试过两条路都无效 ——
		// 重定向 CLAUDE_CONFIG_DIR 之后 inspect 显示它照样读原路径；
		// --system-prompt-override 只是覆盖系统提示，继承来的规则照样生效
		// （实测回答里那条「第一句必须是 收到，Griffin。」仍然照办）。
		// 代价是每轮都固定多付这一整份指令环境，且用户全局文件里的设备表、
		// 内网 IP、路径约定对每个 grok 成员都可见。
		//
		// 别引用 inspect 报的那个 token 数：它写着 ~3754 tokens，而
		// 15016 / 4 = 3754 正好整除 —— 那是 bytes/4 这个启发式，不是真分词
		// （2026-09-21 核对）。真正实测过的量级在 claude 那边：加上
		// --safe-mode --restricted 之后 input_tokens 从 58023 掉到 29413。
		CleanInstructionEnv: SupportNo,
		// CacheAcrossTurns = SupportYes：Step 0 探针里同一条会话 resume，
		// T1 input=20625 cache_read=0，T2 input=250 cache_read=20608
		// （docs/reference/step0-probe-results.md）；00_DECISIONS.md 未决 #1
		// 另有跨会话印证（两条互不相干的会话、身份块与首条正文逐字节相同时，
		// 第二次 cache_read=19584 而第一次是 0）；Step 8 真机冒烟
		// （E:\Isolating_test\council\step8a-smoke\RESULTS.md）同一轮次里
		// 也是 cache_read=128、cache_create=0。三处互相印证。
		CacheAcrossTurns: SupportYes,
		// WriteBoundary = SupportNo（11c 契约第 2 节表）：--deny 是工具级
		// 黑名单，路径模式（Write(../**)）实测静默不生效，同一轮里它把
		// 兄弟目录的文件照样写出来了；--sandbox 的取值集合查不出来
		// （非法 profile 名被静默接受）。
		WriteBoundary: SupportNo,
		// ModelDiscovery = Subcommand：grok models 只回显本机 grok 配置里
		// 手写的模型，不反映订阅。真实清单要带 CPA 密钥问中转；#45 裁
		// 只列本地配置、不碰密钥。
		ModelDiscovery: ModelDiscoverySubcommand,
	}
}

func (p grokProvider) BuildTurn(req TurnRequest) (Command, error) {
	caps := p.Capabilities()
	if err := validateRequest(caps, req); err != nil {
		return Command{}, err
	}
	if err := checkArgvPrompt(caps, req.Prompt); err != nil {
		return Command{}, err
	}

	perm, err := grokPermissionArgs(req.Permission)
	if err != nil {
		return Command{}, err
	}

	args := make([]string, 0, 20)
	// prompt 必须紧跟在 -p 后面，因为 grok 的 -p 是 --single <PROMPT> 的短名，
	// 正文是这个 flag 的**值**，不是末尾的位置参数。三家在这一点上不一样：
	// claude 是 claude [options] [prompt]（位置参数，放最后），
	// agy 的 -p 是 --print 的 bool 别名（正文同样走位置参数），
	// 只有 grok 把正文绑在 flag 上。
	// 放错的后果不是静默降级而是硬失败，但失败得很不像话：clap 会说
	// a value is required for --single <PROMPT> but none was supplied，
	// 退出码 2、stdout 零字节 —— 看起来像「CLI 坏了」而不是「我们参数排错了」。
	// 2026-09-21 第一次真机冒烟就撞在这里，是本项目第一次真正调用 provider。
	args = append(args, "-p", req.Prompt)
	args = append(args, "--output-format", "json")
	args = append(args, "--model", req.Model)
	args = append(args, "--system-prompt-override", req.Role)
	args = append(args, perm...)
	args = append(args, "--cwd", req.WorkDir)
	if req.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(req.MaxTurns))
	}

	var planned string
	if req.ProviderSession == "" {
		id, err := newV4UUID()
		if err != nil {
			return Command{}, fmt.Errorf("grok: mint session id: %w", err)
		}
		planned = id
		args = append(args, "--session-id", id)
	} else {
		args = append(args, "--resume", req.ProviderSession)
	}
	// 正文已经在最前面跟着 -p 发过了，这里不再追加位置参数。

	// effort 只在这一轮真的要了才记进 Dropped。
	//
	// 为什么有条件（2026-09-21 统一）：Dropped 的语义是「你要的东西没能放上
	// 命令行」。没要就没什么好丢的。无条件塞一条 Value 为空串的记录，
	// 会让同一个 Option=="effort" 在库里有两种意思 —— agy 那条读作
	// 「你要了，我们丢了」，本家这条读作「政策说明」。两者一旦都落进
	// messages.meta["dropped"]，Step 7 的成本视图分不开它们。
	// 「这家的 effort 根本不走命令行」由 Meta["effort_policy"] 承担，
	// 那个键 Runner 无条件写，不需要 Dropped 再说一遍。
	var dropped []DroppedOption
	if req.Effort != "" {
		dropped = append(dropped, DroppedOption{
			Option: "effort",
			Value:  req.Effort,
			Reason: "command-line --effort / --reasoning-effort is silently dropped by CPA (exit 0); real effort lives in process-external config, so this adapter never emits the flag",
		})
	}

	return Command{
		Args:             args,
		Dir:              req.WorkDir,
		Env:              nil,
		Stdin:            nil,
		PlannedSessionID: planned,
		Transport:        PromptTransportArgv,
		Dropped:          dropped,
	}, nil
}

func grokPermissionArgs(t PermissionTier) ([]string, error) {
	switch t {
	case PermissionRead:
		// 06 第 2 节写的 READ 档是 --permission-mode default **加 --deny**。
		// 本函数此前省掉了 --deny，理由写的是「没有一份实测清单，瞎填工具名
		// 是在发明沙箱」。那个判断是错的：2026-09-21 实测，只发 default 时
		// grok 照样建出了文件。**省掉一道规格写明要有的闸，比填错它危险**，
		// 正确的做法是去把取值查出来。
		//
		// 根因与 claude 那格相同：grok inspect 直接显示它读用户的
		// ~/.claude/settings.json 作为权限来源，而那份文件这台机器上写着
		// defaultMode = "bypassPermissions"。命令行 --deny 压得过配置文件的 allow，
		// 实测有效：加上这三条之后 grok 连试写入、命令行、替换三种工具全被拦，
		// 而且它拒绝谎报成功 ——「I am not reporting DONE because the file
		// was not created」。
		//
		// **残余风险，明写在这里**：这三条是工具级的黑名单，不是白名单。
		// grok 会加载 MCP 服务器，而 MCP 工具不在这三条的覆盖内。
		// 白名单（--allow / --tools）本该更 fail-closed，但实测 grok 对
		// **非法工具名静默接受**（--tools "NoSuchToolXYZ" 不报错照常回答），
		// 所以写错一个名字不会有人告诉你，白名单在这种语义下反而更危险。
		return []string{
			"--permission-mode", "default",
			"--deny", "Write(*)",
			"--deny", "Edit(*)",
			"--deny", "Bash(*)",
		}, nil
	case PermissionWorkspace:
		// **本家给不出 WORKSPACE 档要的边界，所以拒绝，不静默降级。**
		//
		// WORKSPACE 的语义是「可改工作区」，前提是存在一个「工作区之外改不了」
		// 的边界。实测 grok 的 deny 规则只在**工具级**生效：Write(*) 全禁管用，
		// 而 Write(../**) 这种路径级规则**不管用** —— 同一轮里它把兄弟目录的
		// 文件照样写出来了。--cwd 只设工作目录，不是边界。
		// 于是本家在这一档上只有「全有或全无」。
		//
		// --sandbox <PROFILE> 可能是出路，但它的取值集合查不出来：
		// help 不列，而**非法 profile 名被静默接受**。按「未实测」处理。
		// 对照：claude 的 --restricted 与 agy 的 --add-dir 都实测能围住，
		// 所以这不是「三家都做不到」，是本家做不到。
		//
		// 11c 第二片之后这一支正常情况下走不到：Capabilities().WriteBoundary
		// 是 SupportNo，provider.validateRequest 会在调用本函数之前就把
		// workspace 请求拦下。留着这个分支是同一种防御性写法——claude/agy
		// 的 PermissionFull 分支同样是 validateRequest 已经先拦过的死代码，
		// 这里保持同一体例，而不是假设上游永远不会绕过 validateRequest 来调。
		return nil, fmt.Errorf("permission tier %q: grok has no verified write boundary (path-scoped deny rules do not take effect): %w", t, ErrUnsupportedTier)
	case PermissionFull:
		// FULL 在 06 第 2 节明写「MVP 里不提供给编排器自动使用」。
		// 此前这里返回 bypassPermissions，虽然被 Capabilities.Permissions
		// 挡在更前面而到不了，但一个「照着规格禁止的事去翻译」的分支
		// 早晚会被某次重构放出来。改成明确拒绝。
		return nil, fmt.Errorf("permission tier %q: not offered to the orchestrator (06 section 2): %w", t, ErrUnsupportedTier)
	default:
		return nil, fmt.Errorf("permission tier %q: %w", t, ErrUnsupportedTier)
	}
}

type grokOutput struct {
	Text       string          `json:"text"`
	StopReason string          `json:"stopReason"`
	SessionID  string          `json:"sessionId"`
	RequestID  string          `json:"requestId"`
	Thought    string          `json:"thought"`
	Usage      *grokUsage      `json:"usage"`
	ModelUsage json.RawMessage `json:"modelUsage"`
	NumTurns   json.RawMessage `json:"num_turns"`

	// Type / Message 是 grok 的错误信封。2026-09-20 本机实测抓到
	// （step6-probe/grok-badmodel.out，传一个不存在的 model 名触发）：
	// 整个 stdout 就是 {"type":"error","message":"Couldn't set model ..."}，
	// 退出码 1，没有 sessionId、没有 stopReason、没有 usage。
	// 这两个字段与成功形状的字段完全不相交，所以同一个结构体装得下两种形状。
	Type    string `json:"type"`
	Message string `json:"message"`
}

type grokUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	ReasoningTokens          int `json:"reasoning_tokens"`
	TotalTokens              int `json:"total_tokens"`
}

func (grokProvider) ParseTurn(stdout []byte) (TurnResult, error) {
	if emptyStdout(stdout) {
		return TurnResult{}, fmt.Errorf("grok: stdout is empty: %w", ErrUnparseableOutput)
	}
	var raw grokOutput
	if err := json.Unmarshal(stdout, &raw); err != nil {
		return TurnResult{}, fmt.Errorf("grok: stdout is not JSON: %w", ErrUnparseableOutput)
	}

	usage := grokUsageToUsage(raw.Usage)
	observed, nkeys := observedFromModelUsage(raw.ModelUsage)
	base := TurnResult{
		Thought:         raw.Thought,
		ProviderSession: raw.SessionID,
		ObservedModel:   observed,
		ModelKeys:       nkeys,
		StopRaw:         raw.StopReason,
		Usage:           usage,
		UsageRaw:        rawTopLevelField(stdout, "usage"),
	}

	if !grokSuccess(raw) {
		if !grokShaped(raw) {
			return TurnResult{}, fmt.Errorf("grok: stdout is not a known success shape: %w", ErrUnparseableOutput)
		}
		base.Text = ""
		base.Completion = CompletionUnknown
		if raw.Type == "error" {
			// 已识别的失败形状，所以归 ErrTurnFailed 而不是 ErrUnparseableOutput：
			// 后者的意思是「连结构都认不出、只能重跑才能重新分类」，用在这里会把
			// 一条读得懂的错误信息丢掉。StopRaw 记 "error" 而不是空串，
			// 是为了让上层看得出这一轮走的是信封这条路，不是「没有 stopReason」。
			base.StopRaw = "error"
			return base, fmt.Errorf("grok: turn failed (type=error, message=%q): %w", raw.Message, ErrTurnFailed)
		}
		// 走到这里说明形状认得出、但不是错误信封、stopReason 也不在完成词里。
		// 这一支至今零观测，不编 schema，把原始判别串原样带回就够了。
		return base, fmt.Errorf("grok: turn failed (stopReason=%q): %w", raw.StopReason, ErrTurnFailed)
	}

	base.Text = raw.Text
	base.Completion = completionFrom(base.Text, raw.StopReason)
	if raw.SessionID == "" {
		return base, fmt.Errorf("grok: success shape carries empty sessionId (requestId=%q is not the session): %w", raw.RequestID, ErrNoSessionID)
	}
	return base, nil
}

func grokSuccess(v grokOutput) bool {
	// 已知成功信号只有 stopReason。max_tokens 作为截断成功，尚未被失败探针打过，
	// 但与 claude 同一组完成词，按成功截断而不是未知失败。
	return v.StopReason == "end_turn" || v.StopReason == "max_tokens"
}

func grokShaped(v grokOutput) bool {
	// v.Type != "" 这一项是错误信封那条路的入场券：那份输出里成功形状的五个字段
	// 全是零值，少了这一项就会被判成「连结构都认不出」，退化成 ErrUnparseableOutput。
	return v.Type != "" || v.StopReason != "" || v.SessionID != "" || v.Text != "" || v.Usage != nil || len(v.ModelUsage) > 0
}

func grokUsageToUsage(u *grokUsage) Usage {
	out := Usage{CostBasis: CostBasisNone}
	// 金额字段没有。CostUSDTotal 必须是 nil，不是 0（陷阱 11）。
	out.CostUSDTotal = nil
	if u == nil {
		out.Unreported = appendUnreported(out.Unreported, "cache_create", "input_tokens", "cache_read", "output_tokens", "thinking_tokens")
		return out
	}
	// grok 的 input_tokens 不含已缓存部分（陷阱 2）：本轮 total 20336 =
	// input 19969 + output 239 + cache_read 128。完整 prompt = input + cache_read。
	// 不要去加 cache_creation：它恒为 0，加了也只是给自己看一个假的「没建缓存」。
	out.InputTokens = u.InputTokens + u.CacheReadInputTokens
	out.CacheCreate = intPtr(u.CacheCreationInputTokens)
	out.CacheRead = u.CacheReadInputTokens
	out.OutputTokens = u.OutputTokens
	// 思考 token 叫 reasoning_tokens，在 usage 顶层，不叫 thinking_tokens（陷阱 4）。
	// output 239 含 reasoning 238，不许与 OutputTokens 相加（陷阱 5）。
	out.ThinkingTokens = u.ReasoningTokens
	// cache_creation_input_tokens 恒为 0，即使缓存命中过（陷阱 7）。
	// 判断冷启动只能用 cache_read == 0。这个 0 必须进 Unreported，
	// 否则成本视图会显示 grok 从不建缓存。
	out.Unreported = appendUnreported(out.Unreported, "cache_create")
	return out
}
