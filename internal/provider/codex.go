package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// codexProvider 适配 codex-cli 0.147.0 的 exec --json（JSONL 事件流）。
//
// 两套命令行形状（CX1 契约 §3.1、§3.2）：空会话是 exec ...，续会话是
// exec resume <thread_id> ...。其余参数两套逐项相同，每轮原样再发
// （provider.go 里「除会话 flag 外每一个 flag 每轮都原样再发」那条纪律）。
//
// 正文走 stdin，命令行末尾显式给 -：本机的 codex 是 npm 装出来的 codex.cmd
// 垫片，多行 delta 不能进 argv（spawn.go 对 .cmd / .bat 的换行闸）。两条
// help 都自述 - 读 stdin，未实测（debug prompt-input 不读 stdin，零花费
// 测不到）。真机复验见 CX1 契约 §6。
//
// 身份走首轮正文前缀，与 agy 同一套（IdentityPromptPreamble：只有空会话
// 那一支拼，resume 不再拼，身份已经在那条 thread 里）。不用
// -c developer_instructions=：身份与规则是任意多行文本，要进 argv、过 .cmd
// 垫片（cmd.exe 再解析一遍 %*），而且它经 TOML 解析，解析失败按原样字符串；
// Go 起进程时的真实行为没测。不用 cwd 下的 AGENTS.md：codex 对项目文档有
// 字节上限（project_doc_max_bytes），规则文件大时身份会被静默截短 [上限的
// 具体值与截断行为本机未实测]，而且它只在会话开头注入。
//
// 不发 -C（cwd 由 Command.Dir 给；exec resume 没有 -C）。不发 -o（stdout
// 已经带着正文）。不发 --ephemeral（不落会话文件，下一轮 resume 找不到）。
// 不发 --ignore-user-config（它不加载 $CODEX_HOME/config.toml，Griffin 的
// 模型路由与 model_instructions_file 都在那份文件里；debug prompt-input
// 不收这个参数，后果零花费测不到）。
type codexProvider struct{}

func init() {
	Register(codexProvider{})
}

func (codexProvider) ID() string { return "codex" }

// codexEffortLevels 的取值来自 codex debug models 全部 13 个模型
// supported_reasoning_levels 的并集（2026-09-28）。
//
// 为什么要自己校验：-c model_reasoning_effort=<非法值> 被 codex 静默接受、
// 退出码 0（debug prompt-input 实测），写错一个字母就是「屏幕上是请求档、
// 实际跑默认档」。只比并集、不按模型查：本包不起进程、不读目录。某个模型
// 不支持的档（例如 gpt-6-luna 没有 ultra）照样放行，那一轮的真实行为未实测。
// 大小写敏感（LOW 拒绝）。
//
// 识别那一层（discovery_parse.go 的 parseCodexDiscovery）按模型收窄：/add +编号
// 只给该模型自己 supported_reasoning_levels 里、同时在本清单里的档。手写进配置的
// 成员不经识别，仍只按本清单校验。
var codexEffortLevels = []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

func (codexProvider) Capabilities() Capabilities {
	return Capabilities{
		// SessionID：thread_id 只能从输出的 thread.started 取（没有预指定
		// id 的参数）；续会话时不变（真机两轮同一个 id）。
		SessionID: SessionIDReturnedOnFirstTurn,
		// Effort：-c model_reasoning_effort=<值> 每轮真的放上命令行（真机两轮都带着）。
		Effort: EffortPerTurnFlag,
		// Identity：首轮正文前缀，见类型注释。
		Identity: IdentityPromptPreamble,
		// Prompt / MaxPromptBytes：stdin，没有 argv 上限。turn.argvHeadroom
		// 于是只量固定的参数段（CX1 契约 §2.2 T3）。
		Prompt:         PromptTransportStdin,
		MaxPromptBytes: 0,
		// Permissions：只有 read。workspace 不给，理由见 codexPermissionArgs。
		Permissions: []PermissionTier{PermissionRead},
		// UsageFormula：input_tokens [推断：它已含 cached_input_tokens，第二轮
		// 16000 <= 32396]。但 turn.completed 的 usage 很可能是 thread 累计值
		// [推断]，所以 Council 一个 token 列都不记，见 codexUsage。
		UsageFormula: "input_tokens",

		// OutputSchema：help 列了 --output-schema <FILE>。本步 TurnRequest 没有这一项，不发。
		OutputSchema: SupportYes,
		// MaxTurnsFlag：help 里没有。
		MaxTurnsFlag: SupportNo,
		// ReportsCost：输出里没有金额字段。
		ReportsCost: SupportNo,
		// ReportsCacheCreate：cache_write_input_tokens 这个键在，但两轮都是 0
		// （第二轮 cached 已是 16000），与 grok 的陷阱 7 同形。反正不记。
		ReportsCacheCreate: SupportYes,
		// ReportsObservedModel：输出里没有型号字段。
		ReportsObservedModel: SupportNo,
		// CleanInstructionEnv = No：$CODEX_HOME 下的全局 AGENTS.md（3681 字节）
		// 去不掉（project_doc_max_bytes=0 无效；改 CODEX_HOME 会把登录态一起
		// 带走），与 grok 同一档。能去掉的（skills 说明、插件推荐）已经去掉，
		// 见 BuildTurn 里的隔离参数 [插件推荐那一项是推断：没有做加与不加的对照]。
		//
		// 同样去不掉的还有 $CODEX_HOME/config.toml 里配的 MCP 服务器：没有
		// 整类关闭的参数（features list 的每一项 --disable 都不改 mcp list
		// 的状态，零花费实测）。按名字的 -c mcp_servers.<名>.enabled=false
		// 在名字不存在时让 codex 起不来，名字又来自用户自己会改的配置，不能
		// 冻进 argv。所以每个 codex 成员都带着用户配的全部 MCP 服务器。它们
		// 的工具在不在模型的工具清单里，零花费看不到 [推断：在]。与 grok 的
		// MCP 残留风险同一类（grok.go read 档那段注释）。Griffin 2026-09-28
		// 接受这一风险（00_DECISIONS #46），Council 不关、不改他的配置。
		CleanInstructionEnv: SupportNo,
		// CacheAcrossTurns = Yes：同一 thread 第二轮 cached_input_tokens 从 0
		// 到 16000（一次观测。不论 usage 是累计还是本轮，第二轮都命中了缓存）。
		CacheAcrossTurns: SupportYes,
		// WriteBoundary = Unknown：workspace-write 作为写入边界在 exec / exec resume
		// 上没测过（codex sandbox 上的零花费结果见 codexPermissionArgs 注释）。零值
		// Unknown 让 validateRequest 在 codexPermissionArgs 之前就拒掉
		// workspace / full（文本点名「no verified write boundary」）。
		//
		// read 档自己也有一个缺口，写在这里：MCP 工具的调用不经 shell，
		// sandbox_mode=read-only 管不管得住它们零花费测不到 [推断：多半管不住]。
		// read 档「写不了盘」即使在真机写盘探针之后，也只对 shell 成立，MCP
		// 那一路是残留风险（CX1 契约 §0 第 8 条、§7）。
		WriteBoundary: SupportUnknown,
		// ModelDiscovery = Subcommand：codex debug models（零花费；codex-cli
		// 0.147.0 上 2026-09-28 实测约 0.2 秒、stdout 是一行约 581 KB 的 JSON，
		// 在 maxSpawnOutputBytes 之内）。命令与解析在 discovery.go 的
		// DiscoveryCommandFor 与 discovery_parse.go 的 parseCodexDiscovery。
		// 它列的是 codex 的模型目录（是否联网、是否反映订阅都没有验证），
		// 来源标注是「codex 目录」，不是「订阅」。
		ModelDiscovery: ModelDiscoverySubcommand,
	}
}

// BuildTurn 的顺序是 validateRequest，然后权限档，然后强度。只有空会话且
// Role 非空才拼身份（身份块 + 一个换行 + 原始 Prompt，不 Trim、不改 Prompt
// 的任何字节）。Stdin 在生产路径上永不为空（Runner 在没有 delta 时以
// ErrNoDelta 拒绝、不起进程；算指纹用的探针正文是非空的 prefixProbePrompt）。
// 空 Stdin 会被 spawnProcess 接成空设备、codex 读到一个空 prompt。别为「省
// 一次拷贝」改这条路。
//
// 隔离参数：
//
//   - --ignore-rules：不加载用户与项目的 execpolicy .rules。持久化的「允许」
//     规则可能让命令不经批准执行 [推断]，read 档不该被它放宽。help 自述，
//     未实测（debug prompt-input 不收它）。
//   - --disable plugins --disable apps：真机两轮都带着，去掉插件推荐块
//     [推断：没有做加与不加的对照]。
//   - -c skills.include_instructions=false：debug prompt-input 实测去掉整块
//     skills 说明（9080 字节，列的是用户的私人 skill 清单）。在真 exec 上未实测。
//
// --disable 的名字必须是 codex 认识的 feature：不认识的名字让 codex 起不来
// （Unknown feature flag，零花费实测），标 removed 的照收。将来某版把 plugins
// 或 apps 整个删掉，每一轮都会失败（codex 报错退出、stdout 为空。在 mcp list
// 与 debug prompt-input 上实测过，exec 上同样 [推断]）。不改写成
// -c features.<名>=false：那一形对不认识的名字静默接受，写错一个字母就是静默
// 不生效（与强度值同一类坑，见 codexEffortLevels）。
//
// 做不到的：全局 AGENTS.md 与用户配的 MCP 服务器（见 Capabilities 的
// CleanInstructionEnv）。
func (p codexProvider) BuildTurn(req TurnRequest) (Command, error) {
	caps := p.Capabilities()
	if err := validateRequest(caps, req); err != nil {
		return Command{}, err
	}
	perm, err := codexPermissionArgs(req.Permission)
	if err != nil {
		return Command{}, err
	}
	if req.Effort != "" && !codexEffortAllowed(req.Effort) {
		return Command{}, fmt.Errorf("effort %q is not a codex reasoning level (want one of %s): %w", req.Effort, strings.Join(codexEffortLevels, ", "), ErrUnsupportedEffort)
	}

	prompt := req.Prompt
	if req.ProviderSession == "" && req.Role != "" {
		prompt = req.Role + "\n" + req.Prompt
	}

	args := make([]string, 0, 24)
	args = append(args, "exec")
	if req.ProviderSession != "" {
		args = append(args, "resume", req.ProviderSession)
	}
	args = append(args, "--json", "--skip-git-repo-check")
	args = append(args, perm...)
	args = append(args, "-m", req.Model)
	if req.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+req.Effort)
	}
	args = append(args, "--ignore-rules")
	args = append(args, "--disable", "plugins", "--disable", "apps")
	args = append(args, "-c", "skills.include_instructions=false")
	args = append(args, "-")

	return Command{
		Args:      args,
		Dir:       req.WorkDir,
		Env:       nil,
		Stdin:     []byte(prompt),
		Transport: PromptTransportStdin,
		Dropped:   dropMaxTurns(caps, req),
	}, nil
}

func codexEffortAllowed(e string) bool {
	for _, v := range codexEffortLevels {
		if v == e {
			return true
		}
	}
	return false
}

// codexPermissionArgs 把 read 译成 -c sandbox_mode=read-only 加
// -c approval_policy=never。两套形状都用 -c 而不是 -s：exec resume 没有 -s，
// 用同一种写法两套 argv 只差 resume <id>，前缀摘要（按空会话形状算）就能
// 代表续会话的参数。
//
// -c sandbox_mode= 被配置加载认（debug prompt-input 实测：换成 workspace-write
// / danger-full-access 时 permissions 说明跟着变，非法值 rc 1）。在真 exec
// 与 exec resume 上是否生效未实测。approval_policy=never：headless 没人可批，
// debug prompt-input 实测它让 permissions 说明变成「commands will be rejected」。
// 它在真 exec 与 exec resume 上是否被接受、是否生效未实测（两次真机调用都没带它）。
// exec 下默认是否本来就是 never 没核对，显式发与 claude「显式发
// --permission-mode default」同一条理由。
//
// Windows 上的 read-only，零花费能看到的（CX1 契约 §0 第 4 条 s07-s10，
// 2026-09-28）：codex sandbox（受限令牌沙箱）在 -P :read-only 下写盘被拒
// （Access is denied.，rc 1），-P :workspace 下写得出来。codex sandbox 不带
// -P 时读 -c sandbox_mode=：danger-full-access 写得出（正对照），read-only
// 与不给都拒写，workspace-write 在不带 -C 的这一形里也拒写 cwd（原因未测）。
// 所以在 codex sandbox 这条路上，-c sandbox_mode=read-only 选中的是拒写的
// 沙箱。-c sandbox_mode=read-only 盖得过用户 config.toml 里的
// default_permissions（debug prompt-input 的渲染）。config.toml 里的旧
// profile 键已不被支持。
//
// 看不到的：exec 与 exec resume 走不走同一套沙箱，未实测。read 档「写不了盘」
// 在真机写盘探针（CX1 契约 §6 第 10-15 步）之前只是声明，不是实测结论。
// MCP 工具那一路见 Capabilities 的 WriteBoundary。
//
// workspace 不给：workspace-write 作为写入边界在 exec / exec resume 上没测过
// （codex sandbox 上的零花费结果见本注释「Windows 上的 read-only」那一段）。
// default 分支正常走不到（validateRequest 先拦），与 grok / claude 同一体例的
// 防御写法。
func codexPermissionArgs(t PermissionTier) ([]string, error) {
	switch t {
	case PermissionRead:
		return []string{"-c", "sandbox_mode=read-only", "-c", "approval_policy=never"}, nil
	default:
		return nil, fmt.Errorf("permission tier %q: codex adapter offers read only: %w", t, ErrUnsupportedTier)
	}
}

type codexEvent struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     *codexItem      `json:"item"`
	Usage    json.RawMessage `json:"usage"`
	Error    *codexError     `json:"error"`
	Message  string          `json:"message"`
}

type codexItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexError struct {
	Message string `json:"message"`
}

// ParseTurn 逐行解析 JSONL：按 \n 切，每行 TrimSpace（吃 CRLF 与空行）。任何
// 一行不是 JSON 就整份 ErrUnparseableOutput（--json 的 stdout 只有事件行）。
//
// 事件：thread.started 取 thread_id（两条不同 id 判认不出）。item.completed
// 只收 item.type == "agent_message"，别的条目类型（reasoning、命令执行）不进
// 正文。turn.completed 恰好一条才算成功，多于一条认不出。turn.failed 与 error
// 两种失败事件的名字与形状是推断（参照已观测到的成功事件命名），本机零观测。
//
// 判定顺序：先看 turn.completed 的条数，多于一条先判认不出，不看有没有
// turn.failed。turn.completed 不多于一条（零条或一条）时，有 turn.failed 一律
// 判失败（哪怕后面还有那一条 turn.completed）。没有 turn.completed 时有 error
// 算失败，什么失败事件都没有就认不出（零观测，不编一个结局）。有
// turn.completed 时 error 事件不改结局（它可能是中途重连的提示 [推断]）。
//
// 多条 agent_message：正文取最后一条（与 codex 自己 -o 写「最后一条消息」
// 同一个定义），前面的按 \n\n 连成 Thought（存、默认不路由，不丢），没有一条
// 时正文为空（CompletionEmpty）。codex 没有截断信号，Completion 只有
// Complete / Empty 两种。
//
// ParseTurn 在 turn.failed / error 两种失败结局上（ErrTurnFailed）也带回
// thread_id 与 UsageRaw（钱可能花了）；成功形状但 thread_id 为空时
// （ErrNoSessionID）带回 UsageRaw；认不出的形状（ErrUnparseableOutput）返回
// 零值，两样都没有。UsageRaw 只来自那一条 turn.completed（它的 usage 键缺席
// 或为 null 时也为空），没有 turn.completed 的失败轮它为空。这两样里 runner
// 今天只吸收 thread_id（runner.go 的 absorbParse 不碰 UsageRaw），失败轮的
// UsageRaw 被丢弃，只有成功轮经 agentMeta 进 meta.usage_raw。
// 非零退出码不进判定：spawnProcess 把 *exec.ExitError 收成 err == nil，Runner
// 只在进程没起来时判 spawn_failed，结局全由 stdout 决定。与另三家一致，本
// 适配器不另起一套（provider.go 的约定：ParseTurn 不收退出码）。
func (codexProvider) ParseTurn(stdout []byte) (TurnResult, error) {
	if emptyStdout(stdout) {
		return TurnResult{}, fmt.Errorf("codex: stdout is empty: %w", ErrUnparseableOutput)
	}
	var (
		threadID  string
		known     bool
		messages  []string
		completed int
		usageRaw  json.RawMessage
		failed    bool
		failMsg   string
		errored   bool
		errMsg    string
	)
	for i, line := range bytes.Split(stdout, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var ev codexEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return TurnResult{}, fmt.Errorf("codex: stdout line %d is not JSON: %w", i+1, ErrUnparseableOutput)
		}
		switch ev.Type {
		case "thread.started":
			known = true
			if threadID != "" && ev.ThreadID != threadID {
				return TurnResult{}, fmt.Errorf("codex: two thread.started events with different thread_id (%q, %q): %w", threadID, ev.ThreadID, ErrUnparseableOutput)
			}
			threadID = ev.ThreadID
		case "turn.started", "item.started", "item.updated":
			known = true
		case "item.completed":
			known = true
			if ev.Item != nil && ev.Item.Type == "agent_message" {
				messages = append(messages, ev.Item.Text)
			}
		case "turn.completed":
			known = true
			completed++
			if !absentOrJSONNull(ev.Usage) {
				usageRaw = append(json.RawMessage(nil), ev.Usage...)
			}
		case "turn.failed":
			known = true
			if !failed {
				failed = true
				if ev.Error != nil {
					failMsg = ev.Error.Message
				}
			}
		case "error":
			known = true
			if !errored {
				errored = true
				errMsg = ev.Message
			}
		}
	}
	if !known {
		return TurnResult{}, fmt.Errorf("codex: stdout has no known codex event: %w", ErrUnparseableOutput)
	}
	if completed > 1 {
		return TurnResult{}, fmt.Errorf("codex: %d turn.completed events, expected one: %w", completed, ErrUnparseableOutput)
	}

	base := TurnResult{
		ProviderSession: threadID,
		Usage:           codexUsage(),
		UsageRaw:        usageRaw,
	}
	if failed {
		base.StopRaw = "turn.failed"
		base.Completion = CompletionUnknown
		return base, fmt.Errorf("codex: turn failed (message=%q): %w", failMsg, ErrTurnFailed)
	}
	if completed == 0 {
		if errored {
			base.StopRaw = "error"
			base.Completion = CompletionUnknown
			return base, fmt.Errorf("codex: turn failed (error=%q): %w", errMsg, ErrTurnFailed)
		}
		return TurnResult{}, fmt.Errorf("codex: stdout has no turn.completed event: %w", ErrUnparseableOutput)
	}

	base.StopRaw = "turn.completed"
	if n := len(messages); n > 0 {
		base.Text = messages[n-1]
		base.Thought = strings.Join(messages[:n-1], "\n\n")
	}
	base.Completion = completionFrom(base.Text, "")
	if threadID == "" {
		return base, fmt.Errorf("codex: success shape carries empty thread_id: %w", ErrNoSessionID)
	}
	return base, nil
}

// codexUsage 把五项全部标未报，CostBasisNone，金额与 CacheCreate 为 nil。
//
// 理由：usage 很可能是 thread 累计 [推断，两个数据点分不开两种假设]。把累计
// 值当本轮记，假设对时每轮多记一整段历史，写进只追加的事实源改不掉。标未报
// 时假设对错都只是「不知道」。ParseTurn 在一部分失败结局上也带回 thread_id
// 或 UsageRaw（哪几种、各带回什么见 ParseTurn 的注释；UsageRaw 只来自那一条
// turn.completed，没有 turn.completed 的失败轮它为空），但 runner 今天只吸收
// thread_id，失败轮的 UsageRaw 被丢弃，只有成功轮的原始对象进
// messages.meta["usage_raw"]，将来可以回算。不许把 usage 里的任何数字填进
// Usage 的 token 字段。
func codexUsage() Usage {
	return Usage{
		CostBasis:    CostBasisNone,
		CostUSDTotal: nil,
		CacheCreate:  nil,
		Unreported:   []string{"input_tokens", "cache_create", "cache_read", "output_tokens", "thinking_tokens"},
	}
}
