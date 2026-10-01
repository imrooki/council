package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// 本文件落地 02 第 1.2 节的能力位 ModelDiscovery，以及第 1.3 节「能力发现」的第一级（子命令）。
//
// 本包仍然不起进程、不读文件、不读环境变量（provider.go 的约定不变）。识别要跑的外部命令一律经调用方注入的 DiscoveryExec 执行：生产实现是 turn.RunDiscovery，测试传入夹具字节。
//
// 识别输出怎么解析在 discovery_parse.go。本文件不引用那个文件里的任何标识符，所以只落本文件时 go build ./internal/provider 能过。
//
// ModelDiscovery 是 02 第 1.2 节设计过的能力位：这家 CLI 能不能、以什么方式列出它能用的模型。零值 Unknown，与其它能力位同一条纪律（漏填等于未实测，不是支持）。
type ModelDiscovery int

const (
	// ModelDiscoveryUnknown 是零值：这一位还没实测，不能当成能跑。
	ModelDiscoveryUnknown ModelDiscovery = iota
	// ModelDiscoverySubcommand 是子命令列出模型。agy 跑 agy --output-format json models，grok 跑 grok models，codex 跑 codex debug models。
	ModelDiscoverySubcommand
	// ModelDiscoveryControlHandshake 是 claude 的方式：没有列表子命令，走 SDK 控制协议的 initialize 握手。
	ModelDiscoveryControlHandshake
	// ModelDiscoveryUnsupported 是已经看过、确认这家列不出模型。与 Unknown 不同：Unknown 是还没测。
	ModelDiscoveryUnsupported
)

// Available 报告这一位是不是一种能跑的识别方式。只认 Subcommand 与 ControlHandshake；Unknown 与 Unsupported 都是假。
func (d ModelDiscovery) Available() bool {
	return d == ModelDiscoverySubcommand || d == ModelDiscoveryControlHandshake
}

// ModelSource 是识别结果每一行的来源标注。四种非零取值的来由：claude 的行，以及 agy 清单里 label 以 "Gemini " 开头的行，是订阅；grok 的行只反映本机 grok 配置里手写的模型，不反映订阅；agy 清单里的 Claude 行与 GPT-OSS 行走的是 agy 的额度（#45 要求显示并标注）；codex 的行来自 codex debug models 列出的模型目录，它反不反映订阅没有验证，所以不标订阅。判「Gemini 行」的依据是 label 以 Gemini 加一个空格开头，判据本身在 discovery_parse.go。
type ModelSource int

const (
	ModelSourceUnknown ModelSource = iota
	// ModelSourceSubscription 是订阅里的模型。屏幕标注「订阅」。
	ModelSourceSubscription
	// ModelSourceLocalConfig 是本机配置里手写的模型。屏幕标注「本地配置」。grok 用这一种。
	ModelSourceLocalConfig
	// ModelSourceAgyQuota 是经 agy 额度调用的模型。屏幕标注「走 agy 额度」。
	ModelSourceAgyQuota
	// ModelSourceCodexCatalog 是 codex debug models 列出的模型目录。屏幕标注「codex 目录」。
	ModelSourceCodexCatalog
)

// Label 是屏幕上的来源标注。四种非零取值分别是「订阅」「本地配置」「走 agy 额度」「codex 目录」；零值与任何未列出的取值都是「-」。
func (s ModelSource) Label() string {
	switch s {
	case ModelSourceSubscription:
		return "订阅"
	case ModelSourceLocalConfig:
		return "本地配置"
	case ModelSourceAgyQuota:
		return "走 agy 额度"
	case ModelSourceCodexCatalog:
		return "codex 目录"
	default:
		return "-"
	}
}

// DiscoveredModel 是识别出的一行。
//
// Model 是要写进配置 model 字段的值。Alias 是同一模型的另一种写法（claude 的 value、agy 的 id），只用来判「已在配置里」，不写进配置。Efforts 只有 claude 与 codex 有；claude 的 JSON 里这个键缺席时是 nil，codex 一档都没有时是 nil。
//
// Custom 只有 codex 的行可能为真：slug 以 custom/ 开头，是用户在自己的 codex 配置里自定义的模型（Griffin 的是经 CPA 转发的 grok 与 gemini），列出来就能经第二条路加出一个其实是别家模型的 codex 成员。repl 不给这种行编号、不显示，只数一共几行。
type DiscoveredModel struct {
	Provider string
	Model    string
	Alias    string
	Name     string
	Efforts  []string
	Source   ModelSource
	Custom   bool
}

// DefaultEffort 是加入时不指定强度用的档。清单里有 high 就用 high（#45 默认 high）；没有 high 就取该模型的第一档；一个档都没有（例如 haiku）返回空串。
func (m DiscoveredModel) DefaultEffort() string {
	if len(m.Efforts) == 0 {
		return ""
	}
	for _, e := range m.Efforts {
		if e == "high" {
			return "high"
		}
	}
	return m.Efforts[0]
}

// SuggestedID 是建议的 agent id，与 #45「模型-版本-强度」的写法对齐。它不保证合法、也不保证不重名，加入时由 config.NewAgentIDProblem 再判。
//
// claude：去掉 claude- 前缀，去掉末尾恰好 8 位的日期段，相邻的纯数字段用点号连成版本号（4-5 变成 4.5），方括号尾巴（[1m]）变成 -1m，再接 -<强度>。
// agy：把 label 规整成 id（小写，a-z、0-9 与点号之外的连续字符换成一个 -，去掉首尾的 - 与 .）。规整后若以另一家已登记 provider 的 id 加 - 开头（今天是 claude- 或 grok-），就把那个前缀换成 agy-。这是 #45「只在两家重名时加来源前缀」：Claude Opus 4.6 (Thinking) 得到 agy-opus-4.6-thinking。
// grok：把名字按同一套规则规整成 id。
// codex：把 slug 按同一套规则规整成 id；与 agy 同一条撞名规则，规整后若以另一家已登记 provider 的 id 加 - 开头，就把那个前缀换成 codex-；再接 -<强度>（强度为空时不接）。gpt-6-sol 以 high 加入得到 gpt-6-sol-high。
func (m DiscoveredModel) SuggestedID(effort string) string {
	switch m.Provider {
	case "claude":
		id := claudeIDBase(m.Model)
		if effort != "" {
			id += "-" + sanitizeModelID(effort)
		}
		return id
	case "agy":
		id := sanitizeModelID(m.Model)
		for _, other := range IDs() {
			if other == "agy" {
				continue
			}
			if strings.HasPrefix(id, other+"-") {
				return "agy-" + strings.TrimPrefix(id, other+"-")
			}
		}
		return id
	case "codex":
		id := sanitizeModelID(m.Model)
		for _, other := range IDs() {
			if other == "codex" {
				continue
			}
			if strings.HasPrefix(id, other+"-") {
				id = "codex-" + strings.TrimPrefix(id, other+"-")
				break
			}
		}
		if effort != "" {
			id += "-" + sanitizeModelID(effort)
		}
		return id
	default:
		return sanitizeModelID(m.Model)
	}
}

// SuggestedDisplayName 是写进配置的 display_name。display_name 进身份块、也进指纹（internal/turn/render.go 的订正），所以加入时就要定好，事后改要付一次入场费。
//
// claude：由 id 的模型段转成词首大写的名字；强度不是 high、也不空时，在后面加一个空格和强度（与现有的 Opus 5.0、Opus 5.0 max 写法一致）。agy：Agy 加一个空格再加 label。grok：由规整后的名字转成词首大写。codex：取目录里的 display_name（例如 GPT-6-Sol），为空时由规整后的 slug 转成词首大写；强度的写法同 claude。
func (m DiscoveredModel) SuggestedDisplayName(effort string) string {
	switch m.Provider {
	case "claude":
		name := prettyModelName(claudeIDBase(m.Model))
		if effort != "" && effort != "high" {
			name += " " + effort
		}
		return name
	case "agy":
		return "Agy " + m.Name
	case "codex":
		name := m.Name
		if name == "" {
			name = prettyModelName(sanitizeModelID(m.Model))
		}
		if effort != "" && effort != "high" {
			name += " " + effort
		}
		return name
	default:
		return prettyModelName(sanitizeModelID(m.Model))
	}
}

// sanitizeModelID 把一段名字收成 id 片段：先转小写，a-z、0-9 与点号原样保留，其余字符的连续一段收成一个 -，再去掉首尾的 - 与 .。
func sanitizeModelID(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	sep := false
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' {
			if sep && b.Len() > 0 {
				b.WriteByte('-')
			}
			sep = false
			b.WriteRune(c)
			continue
		}
		sep = true
	}
	return strings.Trim(b.String(), "-.")
}

// claudeIDBase 从 resolvedModel 取出建议 id 的模型段，不含强度。规则见 SuggestedID。
func claudeIDBase(model string) string {
	s := strings.TrimPrefix(strings.ToLower(model), "claude-")
	tail := ""
	if i := strings.IndexByte(s, '['); i >= 0 {
		tail = s[i:]
		s = s[:i]
	}
	parts := strings.Split(s, "-")
	if len(parts) > 1 && len(parts[len(parts)-1]) == 8 && allASCIIDigits(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}
	var out []string
	prevDigit := false
	for _, p := range parts {
		digit := allASCIIDigits(p)
		if digit && prevDigit {
			out[len(out)-1] += "." + p
		} else {
			out = append(out, p)
		}
		prevDigit = digit
	}
	base := sanitizeModelID(strings.Join(out, "-"))
	if t := sanitizeModelID(tail); t != "" {
		base += "-" + t
	}
	return base
}

// allASCIIDigits 报告 s 的每个字节是不是 ASCII 数字。空串不算。
func allASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// prettyModelName 把 id 片段按 - 切开，每段词首转大写，再用空格接起来。空段丢掉。
func prettyModelName(s string) string {
	parts := strings.Split(s, "-")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if p[0] >= 'a' && p[0] <= 'z' {
			p = string(p[0]-'a'+'A') + p[1:]
		}
		out = append(out, p)
	}
	return strings.Join(out, " ")
}

// DiscoveryCommand 是一次识别要跑的外部命令。
//
// CloseStdinAfter 为 nil 时，执行器写完 Stdin 立刻关 stdin。非 nil 时，执行器逐行读 stdout，第一次有一行让它返回 true 才关 stdin。claude 的握手必须收到匹配的回应才关：实测关 stdin 之后约 0.165 秒正常退出。提前关 stdin 的后果没对真 claude 实测；T1 的假 CLI 按「收不到回应就退出」建模。
type DiscoveryCommand struct {
	Path            string
	Args            []string
	Stdin           []byte
	CloseStdinAfter func(line []byte) bool
}

// DiscoveryExec 是可注入的执行器。生产实现是 turn.RunDiscovery，测试传入夹具字节。本包自己不起进程。
type DiscoveryExec func(ctx context.Context, cmd DiscoveryCommand) ([]byte, error)

// 五个哨兵。DiscoverModels（在 discovery_parse.go）把执行器与解析的结果映射到它们：
//
//   - ErrDiscoveryTimeout：执行器的错误链里有 context.DeadlineExceeded。
//   - ErrDiscoveryFailed：其它执行器错误。执行器的错误文本用 %v 接在后面，不用 %w。
//   - ErrDiscoveryUnparseable：输出认不出已知形状。
//   - ErrDiscoveryEmpty：形状对，但一行可选的模型都没有。多半是没登录。
//   - ErrDiscoveryUnsupported：这家 provider 没有识别方式。
var (
	ErrDiscoveryTimeout     = errors.New("model discovery timed out")
	ErrDiscoveryFailed      = errors.New("model discovery command failed")
	ErrDiscoveryUnparseable = errors.New("model discovery output is not a known shape")
	ErrDiscoveryEmpty       = errors.New("model discovery listed no models")
	ErrDiscoveryUnsupported = errors.New("provider has no model discovery")
)

// claudeDiscoveryRequestID 是 initialize 请求的 request_id，固定为 council-models。只认回应里 response.request_id 与它相等的那一行。
const claudeDiscoveryRequestID = "council-models"

// claudeInitializeRequest 是写进 claude stdin 的那一行，末尾带换行。flag 与这一行都照 #45 原文。
const claudeInitializeRequest = `{"type":"control_request","request_id":"council-models","request":{"subtype":"initialize"}}` + "\n"

// DiscoveryCommandFor 给出 providerID 这一家的识别命令。
//
// claude 的 flag 是 -p、--safe-mode、--input-format stream-json、--output-format stream-json、--verbose、--no-session-persistence、--strict-mcp-config。--safe-mode 关掉用户的 CLAUDE.md、skills、hooks、MCP，只留登录态。stdin 是上面那一行 initialize。CloseStdinAfter 只认 type 为 control_response 且 response.request_id 匹配的那一行；system 帧和别的 request 的回应一律跳过。
// agy 跑 --output-format json models，写完空 stdin 就关。grok 跑 models，同样写完就关。codex 跑 debug models，同样写完就关。
// 没有识别方式的 provider 返回包了 ErrDiscoveryUnsupported 的错误。
func DiscoveryCommandFor(providerID, path string) (DiscoveryCommand, error) {
	switch providerID {
	case "claude":
		return DiscoveryCommand{
			Path:            path,
			Args:            []string{"-p", "--safe-mode", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config"},
			Stdin:           []byte(claudeInitializeRequest),
			CloseStdinAfter: isClaudeDiscoveryResponse,
		}, nil
	case "agy":
		return DiscoveryCommand{Path: path, Args: []string{"--output-format", "json", "models"}}, nil
	case "grok":
		return DiscoveryCommand{Path: path, Args: []string{"models"}}, nil
	case "codex":
		return DiscoveryCommand{Path: path, Args: []string{"debug", "models"}}, nil
	default:
		return DiscoveryCommand{}, fmt.Errorf("provider %q: %w", providerID, ErrDiscoveryUnsupported)
	}
}

// claudeDiscoveryLine 是 claude stdout 里一行的形状。只读 type，以及 response 里的 subtype、request_id 和 response.models。
type claudeDiscoveryLine struct {
	Type     string `json:"type"`
	Response *struct {
		Subtype   string `json:"subtype"`
		RequestID string `json:"request_id"`
		Response  *struct {
			Models []claudeModelOption `json:"models"`
		} `json:"response"`
	} `json:"response"`
}

// claudeModelOption 是 models 数组里的一行。value 是别名，resolvedModel 是要写进配置的模型名，supportedEffortLevels 缺席时是 nil，disabled 为真的行不给选。
type claudeModelOption struct {
	Value                 string   `json:"value"`
	ResolvedModel         string   `json:"resolvedModel"`
	DisplayName           string   `json:"displayName"`
	SupportedEffortLevels []string `json:"supportedEffortLevels"`
	Disabled              bool     `json:"disabled"`
}

// claudeDiscoveryResponse 判断这一行是不是我们这次 initialize 的回应。JSON 解不开，或者 type 不是 control_response，或者 response 为空，或者 response.request_id 不是 council-models，一律当不是。别的行（system 帧、别的 request 的回应）由此跳过。
func claudeDiscoveryResponse(line []byte) (claudeDiscoveryLine, bool) {
	var v claudeDiscoveryLine
	if err := json.Unmarshal(bytes.TrimSpace(line), &v); err != nil {
		return claudeDiscoveryLine{}, false
	}
	if v.Type != "control_response" || v.Response == nil || v.Response.RequestID != claudeDiscoveryRequestID {
		return claudeDiscoveryLine{}, false
	}
	return v, true
}

// isClaudeDiscoveryResponse 是 claude 的 CloseStdinAfter：第一次读到匹配的 control_response 才关 stdin。
func isClaudeDiscoveryResponse(line []byte) bool {
	_, ok := claudeDiscoveryResponse(line)
	return ok
}
