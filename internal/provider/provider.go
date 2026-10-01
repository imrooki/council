package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Package provider 是各家 CLI 的契约层与 flag 适配器。
// 不导入 internal/conversation、internal/store、internal/config、database/sql，
// 不起进程、不读文件、不读环境变量。起进程是 turn.Runner 的事。

// Provider 是一层 flag 适配器（02 第 1 节）：把一次 Council 调用翻译成这个 CLI 的
// headless 命令行，再把它的 JSON 输出翻译回 Council 的消息与用量。
//
// 它不认识 Canonical Conversation，不认识 SQLite，也不起进程 —— 起进程是
// turn.Runner 的事，这样每个适配器都能在不 spawn 任何东西的情况下被单测。
// 实现必须无状态、可被同 provider 的多个 agent 并发复用（04 第 0 节规则 2：
// 每个 agent 一条独立的流）：任何需要从 BuildTurn 带到 ParseTurn 的东西一律走
// Command / TurnResult 这两个值传递，绝不放进接收者。
type Provider interface {
	ID() string
	Capabilities() Capabilities

	// BuildTurn 把一次调用翻译成命令行。
	//
	// req.ProviderSession 是不是空串，是「这一轮要不要新建会话」的唯一信号，
	// 也是整个适配器层唯一一处「首轮与后续轮」的区别（09_STEP6_SPEC.md D 节）。
	// 除会话 flag 外的每一个 flag 每轮都原样再发一遍，值只由 TurnRequest 决定。
	// 不做「首轮发身份、后续不发」那种优化 —— 那恰恰是在改缓存前缀。
	//
	// 配置要求了这个 CLI 给不了的东西，返回包了对应哨兵的错误，绝不静默降级：
	//   权限档不在 Capabilities.Permissions 里      -> ErrUnsupportedTier
	//   WORKSPACE/FULL 且 WriteBoundary != Yes       -> ErrUnsupportedTier（文本点名「围不住」，11c 3.2）
	//   req.WorkDir 为空                            -> ErrNoWorkDir
	//   Transport 是 Argv 且 prompt 超 MaxPromptBytes -> ErrPromptTooLong
	//   Capabilities 里该项仍是 Unknown 而这一轮需要它 -> ErrCapabilityUnknown（不猜）
	BuildTurn(req TurnRequest) (Command, error)

	// ParseTurn 只吃 stdout，是纯函数：同样的字节必须给出同样的 TurnResult。
	//
	// 它刻意不收 exitCode 与 stderr：收了适配器就会忍不住拿它们做兜底，而三家
	// 失败时的输出形状零观测（09_STEP6_SPEC.md A 节探针 1 之前），那种兜底只能是
	// 编造的。退出码由 Runner 判。
	//
	// 三种返回：
	//   已知的成功形状            -> (TurnResult, nil)，Completion 区分 Complete / Truncated / Empty
	//   provider 自报失败        -> 包了 ErrTurnFailed 的错误，StopRaw 与 Usage 仍尽量填上
	//                               （钱花了，账要记；但绝不把错误正文当成回答返回）
	//   连结构都认不出           -> 包了 ErrUnparseableOutput 的错误
	// 成功形状但会话 id 为空     -> 包了 ErrNoSessionID 的错误：静默把空串当 id 存进去，
	//                               下一轮就是一次冷启动，这种错误必须响亮。
	ParseTurn(stdout []byte) (TurnResult, error)
}

var registry = map[string]Provider{}

// Register 在各适配器文件的 init() 里登记自己。删掉一个适配器文件就等于把它下线，
// 不用改第二处。重复登记直接 panic：那是编译期就该发现的编码错误，不该留到运行时。
func Register(p Provider) {
	id := p.ID()
	if _, exists := registry[id]; exists {
		panic("provider: already registered: " + id)
	}
	registry[id] = p
}

// Lookup 按 provider id 取适配器。
//
// 没有适配器是一种正常状态，不是配置错误：配置里写了一个本包没有登记的 provider 时，
// council 照常启动、横幅上点名，只是那个 agent 开不了口。找不到时返回包了
// ErrUnknownProvider 的错误，信息里列出已登记的 id。
func Lookup(id string) (Provider, error) {
	p, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (registered: %s): %w", id, strings.Join(IDs(), ", "), ErrUnknownProvider)
	}
	return p, nil
}

// IDs 按字典序返回已登记的 provider id。给启动横幅与报错列候选用，顺序必须稳定。
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

var (
	ErrUnknownProvider   = errors.New("provider not registered")
	ErrUnsupportedTier   = errors.New("permission tier not supported by this provider")
	ErrNoWorkDir         = errors.New("turn request carries no work dir")
	ErrPromptTooLong     = errors.New("prompt exceeds this provider transport limit")
	ErrTurnFailed        = errors.New("provider reported a failed turn")
	ErrUnparseableOutput = errors.New("provider output is not a known success shape")
	ErrNoSessionID       = errors.New("provider output carries no session id")
	ErrCapabilityUnknown = errors.New("provider capability is unknown, refusing to guess")
	// ErrUnsupportedEffort：配置要求了一个这家 provider 不认的 effort 值。
	// 只有值被静默接受、必须由适配器自己校验的 provider 才返回它（今天只有
	// codex）。它与 ErrCapabilityUnknown 不同：那个说能力位是 Unknown，这个
	// 说能力位已知、值不在本家的取值里。
	ErrUnsupportedEffort = errors.New("effort value not supported by this provider")
)

// windowsArgvPromptLimit 是 Windows CreateProcess 命令行上限的字节近似。
//
// 它不是一道严密的闸，别把它当成。OS 上限约束的是整条命令行，而契约把
// MaxPromptBytes 定义成只量 prompt（09a：Transport 是 Argv 且 prompt 超
// MaxPromptBytes -> ErrPromptTooLong）。于是 prompt 刚好卡在限下、再加上几百
// 字节的 flag 与身份块之后，整条命令行仍可能越界 —— 那时失败来自 OS，不是
// 本层的哨兵，报错形状也不受本包控制。
// 这里不自行减一个凭空想出来的余量：编一个数会把「未实测」伪装成「已标定」。
// 记为未决 #15，等 Step 7 用真实批次量出 flag 段的实际字节数再收紧。
// 0 在契约里表示不限，Argv 传输不许填 0 假装没有上限。
const windowsArgvPromptLimit = 32767

func validateRequest(caps Capabilities, req TurnRequest) error {
	if req.WorkDir == "" {
		return fmt.Errorf("%w", ErrNoWorkDir)
	}
	// 写入边界闸（11c 契约第 3.2 节）：WORKSPACE 与 FULL 两档要求这家的写盘
	// 边界经过实测证实，caps.WriteBoundary 零值 SupportUnknown 同样 fail-closed。
	//
	// 这道闸必须排在下面的 tierAllowed 之前，顺序不是随意的：grok 的
	// Permissions 里根本不列 workspace（它证明不了边界，3.1 节明写），
	// 若让 tierAllowed 先判，grok 请求 workspace 会先在那里落空，报出的是
	// 「本家没有这个档」的泛泛措辞——但那不是真相，真相是「围不住」，与
	// claude 结构性地没有 FULL 是两种不同的「不行」，错误文本不能混同
	// （3.2 节原话）。放在这里先判，grok 与 claude/agy 之间那点「声不声明
	// 这一档」的差异就不会污染报错文本该说哪一种「不行」。
	//
	// 复用 ErrUnsupportedTier 这一个哨兵，不新开一个：两种「不行」的差别
	// 在错误文本，不在哨兵——调用方一律 errors.Is(err, ErrUnsupportedTier)
	// 就该认出「这一档要不到」，不需要认出「为什么要不到」。
	if (req.Permission == PermissionWorkspace || req.Permission == PermissionFull) && caps.WriteBoundary != SupportYes {
		return fmt.Errorf("permission tier %q: no verified write boundary: %w", req.Permission, ErrUnsupportedTier)
	}
	if !tierAllowed(caps.Permissions, req.Permission) {
		return fmt.Errorf("permission tier %q: %w", req.Permission, ErrUnsupportedTier)
	}
	// Unknown 的能力这一轮真用到了就不能猜。MaxTurnsFlag 是目前 TurnRequest
	// 里唯一会走到这一支的项：claude / agy 是 SupportNo（丢弃，不是猜），
	// grok 是 SupportYes。没有一家是 Unknown，这条是给漏填能力位的。
	if req.MaxTurns > 0 && caps.MaxTurnsFlag == SupportUnknown {
		return fmt.Errorf("max_turns: %w", ErrCapabilityUnknown)
	}
	// effort 同理，而且这一条不是摆设：契约（09a BuildTurn 注释）把「该项仍是
	// Unknown 而这一轮需要它」整类归到 ErrCapabilityUnknown，agy.go 顶部的注释
	// 也明写它是靠这条闸才不得不把 Effort 声明成 EffortPerTurnFlag。没有这条闸，
	// 那段注释描述的就是一个不存在的保护。
	// 两个方向都不能猜：猜「发 flag」会给自己伪造一份没生效的证据（grok 经 CPA
	// 正是这样被静默吞掉、退出码仍是 0）；猜「不发」则让一个本来支持 per-turn
	// effort 的 CLI 永远跑在默认档，而屏幕上显示的是请求档。
	if req.Effort != "" && caps.Effort == EffortUnknown {
		return fmt.Errorf("effort: %w", ErrCapabilityUnknown)
	}
	if caps.Prompt == PromptTransportUnknown {
		return fmt.Errorf("prompt transport: %w", ErrCapabilityUnknown)
	}
	return nil
}

func tierAllowed(have []PermissionTier, want PermissionTier) bool {
	for _, t := range have {
		if t == want {
			return true
		}
	}
	return false
}

func checkArgvPrompt(caps Capabilities, prompt string) error {
	if caps.Prompt != PromptTransportArgv {
		return nil
	}
	if caps.MaxPromptBytes > 0 && len(prompt) > caps.MaxPromptBytes {
		return fmt.Errorf("prompt is %d bytes, limit %d: %w", len(prompt), caps.MaxPromptBytes, ErrPromptTooLong)
	}
	return nil
}

func dropMaxTurns(caps Capabilities, req TurnRequest) []DroppedOption {
	if req.MaxTurns <= 0 {
		return nil
	}
	if caps.MaxTurnsFlag != SupportNo {
		return nil
	}
	return []DroppedOption{{
		Option: "max_turns",
		Value:  strconv.Itoa(req.MaxTurns),
		Reason: "this provider has no max-turns flag",
	}}
}

func emptyStdout(stdout []byte) bool {
	return len(bytes.TrimSpace(stdout)) == 0
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// absentOrJSONNull 把「这个键根本不在」与「这个键是 null」读成同一件事。
//
// 为什么要单独一个函数：json.RawMessage 在键缺失时留在 nil，而 isJSONNull(nil)
// 是 false。于是 !isJSONNull(缺失) 为真，一道写成「api_error_status 必须是 null」
// 的成功闸会把缺了这个键的输出判成失败。后果是最难查的那一种：claude 正常回答、
// 退出码 0、钱照花，而我们把它归类成失败并丢掉正文 —— 屏幕上那个 agent 只是
// 没说话，没有任何一层会喊。
//
// 本机三份 claude 原件（step0-probe/claude-t1..t3）里这个键每一份都在、且都是
// 字面 null，所以这道闸在已观测的路径上是对的，这个函数是给「将来某版省掉一个
// null 键」准备的，不是在修一个已复现的 bug。之所以仍然改：缺失读作 null 是
// JSON 这一侧的通用约定（encoding/json 对指针字段就是这么做的），两种读法里
// 只有这一种的失败模式不是静默丢正文。
func absentOrJSONNull(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw)
}

// observedFromModelUsage 只读 modelUsage 的键名。
//
// 禁令（05 第 3 节陷阱 1）：单轮 token 一律取 usage，不许取 modelUsage。
// claude 的 modelUsage 是会话累计（t3 cacheReadInputTokens=76188=28436+47752），
// grok 的 modelUsage 却是单轮（t2 inputTokens=250=该轮 usage.input_tokens）。
// 同名反义。这里只数键、在恰好一个键时把它当 ObservedModel。
func observedFromModelUsage(raw json.RawMessage) (string, int) {
	s := bytes.TrimSpace(raw)
	if len(s) == 0 || isJSONNull(s) {
		return "", 0
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(s, &m); err != nil {
		return "", 0
	}
	n := len(m)
	if n != 1 {
		return "", n
	}
	for k := range m {
		return k, 1
	}
	return "", n
}

// rawTopLevelField 从整段 stdout 里摘出一个顶层键的原始字节。
//
// 为什么再解析一遍而不是给结构体加字段：同一个 json tag 不能出现两次。
// 把 Usage 改成 json.RawMessage 再二次解析也行，但那会连带改掉错误语义 ——
// 现在 usage 形状不对会让整份输出落进 ErrUnparseableOutput（fail-closed），
// 改完就会悄悄变成「这家没报用量」。多解析一遍是每次 CLI 调用一次的代价，
// 而那次调用本身要花几秒到几十秒，这点开销买回的是不动既有判据。
//
// 键缺席、为 null、或整段不是 JSON 时返回 nil。
func rawTopLevelField(stdout []byte, key string) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(stdout, &m); err != nil {
		return nil
	}
	v, ok := m[key]
	if !ok || absentOrJSONNull(v) {
		return nil
	}
	return v
}

func completionFrom(text, stop string) Completion {
	if text == "" {
		return CompletionEmpty
	}
	if stop == "max_tokens" {
		return CompletionTruncated
	}
	return CompletionComplete
}

// 这里曾经有一个 noteAllZeroUsage：usage 四项全为 0 就把它们记进 Unreported，
// 理由是「全零多半说明这家没报」。2026-09-20 的失败探针证伪了它 ——
// claude 与 agy 的失败输出里 usage 键都在、五项都是货真价实的 0
// （那一轮确实什么都没消耗）。留着它，每一次失败轮都会谎称用量没被上报。
// 「没报」唯一可靠的判据是键本身缺席，也就是各适配器里 u == nil 那一支，
// 它已经单独处理了。少一个启发式，Unreported 这张表才敢被当成事实读。

func appendUnreported(have []string, names ...string) []string {
	seen := make(map[string]bool, len(have)+len(names))
	for _, n := range have {
		seen[n] = true
	}
	for _, n := range names {
		if seen[n] {
			continue
		}
		have = append(have, n)
		seen[n] = true
	}
	return have
}

func intPtr(v int) *int { return &v }

func floatPtr(v float64) *float64 { return &v }
