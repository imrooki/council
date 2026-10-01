package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// agyProvider 适配 agy 1.2.7 的 headless JSON 输出（与 1.1.26 形状实测一致）。
//
// 命令行：-p --output-format json --mode plan [--effort <e>] [--conversation <id>]
// 身份：--agent 收的是预设名不是任意文本，所以 Identity 是 PromptPreamble ——
// 空会话时把身份块拼在首轮正文最前面，resume 不再拼（身份已经在那条会话里）。
//
// effort 归 EffortBakedIntoModelLabel：本家的模型目录把强度枚举成并列的模型名
// 未实测（02 第 1.2 节）。本步仍发 --effort（EffortPerTurnFlag），因为规格 B 节
// 写明 [--effort <e>] 会放上命令行；不把该项留在零值 Unknown，否则非空 Effort
// 会被 ErrCapabilityUnknown 整段拒绝。交互本身仍然未知，注释留在这里。
type agyProvider struct{}

func init() {
	Register(agyProvider{})
}

func (agyProvider) ID() string { return "agy" }

func (agyProvider) Capabilities() Capabilities {
	return Capabilities{
		SessionID:      SessionIDReturnedOnFirstTurn,
		Effort:         EffortBakedIntoModelLabel,
		Identity:       IdentityPromptPreamble,
		Prompt:         PromptTransportArgv, // -p 把正文放进 argv。报错文本自证也读 stdin，本步不用那条路
		MaxPromptBytes: windowsArgvPromptLimit,
		Permissions:    []PermissionTier{PermissionRead, PermissionWorkspace},
		UsageFormula:   "input_tokens",

		OutputSchema:         SupportYes, // 02 第 1.1 节：--json-schema。本步 TurnRequest 没有这项，不发
		MaxTurnsFlag:         SupportNo,
		ReportsCost:          SupportNo,
		ReportsCacheCreate:   SupportNo,      // usage 里根本没有这一项，不是 0（陷阱 6）
		ReportsObservedModel: SupportNo,      // 没有 modelUsage
		CleanInstructionEnv:  SupportUnknown, // 没实测过隔离全局规则文件；改 HOME 会把登录态一起带走
		// CacheAcrossTurns = SupportNo：00_DECISIONS.md 未决 #17 真机观察，
		// 同一条会话续到第二轮，input_tokens 从 16195 涨到 32722 而
		// cache_read 仍是 0；Step 8 真机冒烟
		// （E:\Isolating_test\council\step8a-smoke\RESULTS.md）同一轮次里
		// 同样是 cache_read=0（in=16316）。没有跨轮缓存收益，按轮数乘着付。
		CacheAcrossTurns: SupportNo,
		// WriteBoundary = SupportYes（11c 契约第 2 节表）：--add-dir 之外被拒，
		// 被拒的调用记在 denied_actions 字段（只有 action 与 display_name
		// 两个键，人类可读的拒绝理由在 stderr，不在这个字段里）。
		//
		// 2026-09-21 真机实测证明「Council 不解析 denied_actions」不是缺口，
		// 是阻塞（11c 契约第 7 节，原件 E:\Isolating_test\council\oq-agy-ws\
		// RESULTS.md，本次复核原件 E:\Isolating_test\council\oq-agy-ws\rerun\
		// raw.json）。三层机制：
		//   1. agy 在 WORKSPACE 下确实能写界内（单独做一件界内的事时正常
		//      建文件并回 DONE）；
		//   2. 越界写被自动拒绝（headless 没人可问），边界是真的，证据在
		//      denied_actions 字段里，不在正文里；
		//   3. 一次自动拒绝会把整轮的正文清空，而 status 仍是 SUCCESS——
		//      界内已经做完的事照样做了，只是模型的汇报没了。
		// 不解析这个字段，Council 对这一轮只能报「本轮为空」，与「这一轮真的
		// 没话说」在字节上完全一样（未决 #17 记的同形陷阱），一个 WORKSPACE
		// 成员到底写没写文件，用户从屏幕上完全看不出来。现在 ParseTurn 解析
		// 它、归一化进 TurnResult.DeniedActions，见下面 agyDeniedAction 与
		// provider.DeniedAction 的注释。
		WriteBoundary: SupportYes,
		// ModelDiscovery = Subcommand：agy --output-format json models。
		// 按账号从服务器拉，反映订阅；强度编在 label 里。
		ModelDiscovery: ModelDiscoverySubcommand,
	}
}

func (p agyProvider) BuildTurn(req TurnRequest) (Command, error) {
	caps := p.Capabilities()
	if err := validateRequest(caps, req); err != nil {
		return Command{}, err
	}

	prompt := req.Prompt
	if req.ProviderSession == "" && req.Role != "" {
		// 身份块 + 一个换行 + 原始 Prompt。不 Trim、不改 Prompt 的任何一个字节。
		// 只有空会话这一支拼接；resume 再拼一次会把身份塞进 delta，污染只追加的流。
		prompt = req.Role + "\n" + req.Prompt
	}
	if err := checkArgvPrompt(caps, prompt); err != nil {
		return Command{}, err
	}

	perm, err := agyPermissionArgs(req.Permission)
	if err != nil {
		return Command{}, err
	}

	args := make([]string, 0, 16)
	// 正文必须紧跟 -p。它的 --help 把 -p 印成「Short alias for --print」、
	// 后面不带值占位符，看着像个 bool，实际要值 —— 2026-09-21 真机调用时它
	// 自己把话说明白了：-p took "--output-format" as its prompt, so the
	// intended prompt was left as an argument and ignored。
	// 注意那一轮退出码是 2、stdout 零字节，正文被「忽略」而不是报错，
	// 要不是它顺手打了这句 stderr，表现就只是一次莫名其妙的空输出。
	// 三家在这里不一致：grok 与 agy 的正文绑在 flag 上，claude 是末尾位置参数。
	args = append(args, "-p", prompt)
	args = append(args, "--output-format", "json")
	// --model 每轮都发，不带任何条件。规格 09 第 165 行原本漏了这个 flag，
	// 那是规格的缺陷不是实现的走样（2026-09-21 补）。漏掉它有三重后果：
	//  1. AgentConfig.Model 是被校验过的必填项，却对这一家静默失效 ——
	//     静默丢弃正是本包一直在防的那类坑，grok 的 effort 就这么栽过。
	//  2. 本家 ReportsObservedModel 是 SupportNo：输出里没有 modelUsage，
	//     我们没有任何办法事后知道它实际跑了哪个模型。不发这个 flag，
	//     messages.sender_model 记下的名字就既无从强制、也无从核对，
	//     成了写进只追加事实源里的一句纯粹的编造。
	//  3. 本家把推理强度编进模型标签（看它自己的目录：Gemini 3.8 Flash 的
	//     High / Medium / Low 是三个并列的模型名），不发标签等于强度也没发，
	//     而 IdentityFingerprint 那边已经按配置值算过指纹了。
	args = append(args, "--model", req.Model)
	args = append(args, perm...)
	args = append(args, "--add-dir", req.WorkDir)
	if req.ProviderSession != "" {
		args = append(args, "--conversation", req.ProviderSession)
	}
	// 正文已经在最前面跟着 -p 发过了，这里不再追加位置参数。

	// effort 不发上命令行，改记进 Dropped。本家的 --effort 这个 flag 确实存在
	// （失败输出里它把 --effort "" 原样回显过），但推理强度真正由模型标签决定，
	// 两者如何交互本机未实测。明知可能无效还发，是在给自己伪造一份
	// 「强度已按配置生效」的证据 —— 与 grok 那一格同一类错误，方向相反：
	// 那边是发了会被能力门吞掉，这边是发了也盖不过标签。
	// 用户要调强度就改 model 字段，不是改 effort。
	dropped := dropMaxTurns(caps, req)
	if req.Effort != "" {
		dropped = append(dropped, DroppedOption{
			Option: "effort",
			Value:  req.Effort,
			Reason: "agy encodes reasoning strength in the model label, not in a separate flag; put it in the model field instead",
		})
	}
	return Command{
		Args:      args,
		Dir:       req.WorkDir,
		Env:       nil,
		Stdin:     nil,
		Transport: PromptTransportArgv,
		Dropped:   dropped,
	}, nil
}

func agyPermissionArgs(t PermissionTier) ([]string, error) {
	switch t {
	case PermissionRead:
		// READ 档**不发 --mode**，这是 2026-09-21 单变量隔离实测之后改的。
		//
		// 本家只有两个模式值：accept-edits 与 plan，没有只读档。原来 READ 翻译成
		// --mode plan，看起来最保守，实际它连话都不说：plan 模式下 response 恒为
		// 空串而 status 仍是 SUCCESS，token 照烧（实测一次纯对话烧掉 576 个 output
		// token 却只回一个空串；换成文本输出格式时 stdout 干脆是 0 字节）。
		// 只改这一个变量、其余全部不动时它就正常回答（21 个 output token 回 ACK），
		// 所以病因是 plan 模式本身，不是别的。原件在
		// E:\Isolating_test\council\agy-mode-probe\。
		//
		// 不发 flag 去碰默认，与 claude 那格「显式发 --permission-mode default」的
		// 做法相反，是因为本家根本没有一个可发的只读值 —— 那边是「有默认值可显式
		// 写出来」，这边是「没有这个档」。写一个不存在的值会被参数解析直接拒。
		//
		// 只读这条性质是直接验过的，不是推的：让它「在工作目录里建一个 PROOF.txt
		// 并写入 WRITTEN」，结果 response 为空、工作目录 / cwd / 项目根三处
		// 都没有任何文件被写出（同一轮里纯文本提问是能正常回答的，
		// 所以不是它整体罢工）。默认档要求逐次授权，而 headless 下没人可问。
		// **残余风险**：一次否定实验证明不了「写盘绝无可能」。这条按
		// 「已验证一次、未穷尽」记，比原来的「未验证」强，但别当成沙箱。
		return nil, nil
	case PermissionWorkspace:
		return []string{"--mode", "accept-edits"}, nil
	case PermissionFull:
		return []string{"--dangerously-skip-permissions"}, nil
	default:
		return nil, fmt.Errorf("permission tier %q: %w", t, ErrUnsupportedTier)
	}
}

type agyOutput struct {
	ConversationID string          `json:"conversation_id"`
	Status         string          `json:"status"`
	Response       string          `json:"response"`
	DurationSec    json.RawMessage `json:"duration_seconds"`
	NumTurns       json.RawMessage `json:"num_turns"`
	Usage          *agyUsage       `json:"usage"`

	// Error 只在失败时非空。2026-09-20 本机实测（step6-probe/agy-badmodel.out）：
	// status 变成 "ERROR"，response 是空串，错误正文全在这个键里，
	// 而 conversation_id 是空串、usage 五项全是真实的 0。
	// 不取它这一轮就只剩一个 "ERROR" 字样，诊断信息要靠重跑才拿得回来。
	Error string `json:"error"`

	// DeniedActions 对应顶层键 denied_actions：越界写盘被 headless 模式自动
	// 拒绝时非空，本机实测两次都是数组（哪怕只有一个元素）：
	//   [{"action":"write_file","display_name":"WriteToFile"}]
	// 逐字节来自 2026-09-21 复核原件
	// E:\Isolating_test\council\oq-agy-ws\rerun\raw.json。
	//
	// 注意：Griffin 在任务书里贴的那份实测输出经 PowerShell 管道打印后看起来
	// 像单个对象而不是数组——那是 PowerShell 对单元素数组的显示折叠（管道/
	// ConvertFrom-Json 都会把 `[X]` 显示成 `X`），不是 agy 真实吐出的字节。
	// Council 这里直接读子进程 stdout 的原始字节，不经过任何 PowerShell 管道，
	// 这个折叠不会发生。unmarshal 成 []agyDeniedAction 而不是写一个「数组或
	// 单对象都能吃」的兼容解析：真实形状变了就该在这里 unmarshal 失败、
	// fail-closed 成 ErrUnparseableOutput，而不是悄悄兼容一个从未在真实
	// 子进程输出里出现过的形状——那是在猜，不是在解析已实测的东西。
	DeniedActions []agyDeniedAction `json:"denied_actions"`
}

// agyDeniedAction 是 denied_actions 数组元素的形状，两个键都是字符串，
// 逐字节实测于上面 agyOutput.DeniedActions 的注释。
type agyDeniedAction struct {
	Action      string `json:"action"`
	DisplayName string `json:"display_name"`
}

type agyUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ThinkingTokens  int `json:"thinking_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
	TotalTokens     int `json:"total_tokens"`
}

func (agyProvider) ParseTurn(stdout []byte) (TurnResult, error) {
	if emptyStdout(stdout) {
		return TurnResult{}, fmt.Errorf("agy: stdout is empty: %w", ErrUnparseableOutput)
	}
	var raw agyOutput
	if err := json.Unmarshal(stdout, &raw); err != nil {
		return TurnResult{}, fmt.Errorf("agy: stdout is not JSON: %w", ErrUnparseableOutput)
	}

	usage := agyUsageToUsage(raw.Usage)
	base := TurnResult{
		ProviderSession:  raw.ConversationID,
		ObservedModel:    "",
		ModelKeys:        0,
		StopRaw:          raw.Status,
		Usage:            usage,
		UsageRaw:         rawTopLevelField(stdout, "usage"),
		DeniedActions:    agyDeniedActionsToDenied(raw.DeniedActions),
		DeniedActionsRaw: rawTopLevelField(stdout, "denied_actions"),
	}

	if !agySuccess(raw) {
		if !agyShaped(raw) {
			return TurnResult{}, fmt.Errorf("agy: stdout is not a known success shape: %w", ErrUnparseableOutput)
		}
		base.Text = ""
		base.Completion = CompletionUnknown
		return base, fmt.Errorf("agy: turn failed (status=%q error=%q): %w", raw.Status, raw.Error, ErrTurnFailed)
	}

	// 正文带尾换行："OK\n" / "5192\n"。claude 的 result 与 grok 的 text 都不带。
	// 不 TrimRight 的话库里每条 agy 发言都多一个换行，事实源只追加、洗不掉（陷阱 9）。
	base.Text = strings.TrimRight(raw.Response, "\n")
	base.Thought = ""
	base.Completion = completionFrom(base.Text, "")
	if raw.ConversationID == "" {
		return base, fmt.Errorf("agy: success shape carries empty conversation_id: %w", ErrNoSessionID)
	}
	return base, nil
}

func agySuccess(v agyOutput) bool {
	// 已知成功信号是 status == "SUCCESS"（全大写）。其它取值零观测，不猜。
	return v.Status == "SUCCESS"
}

func agyShaped(v agyOutput) bool {
	return v.Status != "" || v.ConversationID != "" || v.Response != "" || v.Usage != nil || v.Error != ""
}

// agyDeniedActionsToDenied 把 agy 的原始形状归一化成 provider.DeniedAction。
// nil 输入（denied_actions 键缺席或为 null）原样透传成 nil，不返回一个空切片
// 冒充——那会让「这一轮没有任何调用被拒」与「这个键根本没解出来」在下游长成
// 同一个东西，和 CacheCreate 那处「nil 不是 0」的纪律是同一类问题。
func agyDeniedActionsToDenied(in []agyDeniedAction) []DeniedAction {
	if in == nil {
		return nil
	}
	out := make([]DeniedAction, 0, len(in))
	for _, d := range in {
		out = append(out, DeniedAction{Action: d.Action, DisplayName: d.DisplayName})
	}
	return out
}

func agyUsageToUsage(u *agyUsage) Usage {
	out := Usage{CostBasis: CostBasisNone}
	out.CostUSDTotal = nil
	out.CacheCreate = nil
	out.Unreported = appendUnreported(out.Unreported, "cache_create")
	if u == nil {
		out.Unreported = appendUnreported(out.Unreported, "input_tokens", "cache_read", "output_tokens", "thinking_tokens")
		return out
	}
	// agy 的 input_tokens 已经含缓存命中（陷阱 2）：t2 total 20482 = input 20348 +
	// output 134，且 cache_read_tokens 16282 < 20348。完整 prompt = input 原样，
	// 再加 cache_read 会把已缓存部分数两遍。
	out.InputTokens = u.InputTokens
	// 缓存读的键是 cache_read_tokens，没有 _input_（陷阱 3）。照简写去取会静默得零。
	out.CacheRead = u.CacheReadTokens
	out.OutputTokens = u.OutputTokens
	// 思考 token 在 usage 顶层叫 thinking_tokens（陷阱 4）。
	// output 120 含 thinking 119，不许与 OutputTokens 相加（陷阱 5）。
	out.ThinkingTokens = u.ThinkingTokens
	return out
}
