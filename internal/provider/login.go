package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

// login.go 是首次运行（00_DECISIONS.md #48，OB1 v1.1；OB2 加了「已登录（API key）」）显示的
// 登录状态。与本包其余文件同一条约定：不起进程、不读文件、不读环境变量。claude 与 grok 的
// 信号取自识别那一次本来就拿到的 stdout；codex 的识别输出里没有信号，另跑一次
// codex login status，由调用方注入的 LoginStatusExec 执行、把退出码与两路输出交回来判；
// agy 查不出。
//
// 证据等级（claude 2.1.283、grok 1.0.41、codex-cli 0.147.0）：
//   - 本机实测过的：claude 的 subscriptionType 与没登录的形状；grok 的
//     You are not authenticated. 与 Model '<模型>' is using its own API key.（本机经 CPA 的 grok
//     实测是这一行）；codex 的 Logged in using ChatGPT 与 Not logged in。
//   - 只在程序文本里见过、没实跑的：claude 的 apiKeySource；grok 的
//     You are using XAI_API_KEY.、You are authenticated via deployment key.、
//     You are logged in with <内容>.；codex 的 Logged in using an API key - <内容>、
//     Logged in using Amazon Bedrock API key。
//
// 只有正面信号才算已登录或已登录（API key），只有负面标记才算没登录，其余（字段缺失、
// 形状不认识、CLI 版本不同、识别超时）一律查不出。

// LoginState 是四档登录状态。零值是查不出。
type LoginState int

const (
	// LoginUnknown 是查不出：没有正面信号，也没有负面标记。
	LoginUnknown LoginState = iota
	// LoginYes 是已登录：有正面信号。
	LoginYes
	// LoginNo 是没登录：有负面标记。
	LoginNo
	// LoginAPIKey 是已登录（API key）：有「用 API key 一类的 key 认证」的正面信号（OB2）。
	LoginAPIKey
)

// LoginInfo 是一家的登录状态。Subscription 只有 claude 已登录时非空：initialize 回应里
// account.subscriptionType 的原值（本机实测是 Claude Max），不改写、不猜。KeySource 只有
// claude 已登录（API key）时非空：account.apiKeySource 的原值（例如 ANTHROPIC_API_KEY、
// /login managed key），同样不改写。
type LoginInfo struct {
	State        LoginState
	Subscription string
	KeySource    string
}

// LoginStatusExec 是跑登录状态命令的执行器。生产实现是 turn.RunLoginStatus。
// 进程自己以非 0 退出不算错误，退出码照实交回；起不来或被 ctx 杀掉时 err 非 nil。
type LoginStatusExec func(ctx context.Context, cmd DiscoveryCommand) (exitCode int, stdout, stderr []byte, err error)

// grok models 的 stdout 首行是一组互斥的认证状态句（grok 1.0.41 的程序文本里挨在一起，
// 见 OB2 契约 §0）。不含变量的三句整句相等；含变量的两句按前缀与后缀认，中间不能为空。
const (
	// grokNotAuthenticated 是没登录（实测）。
	grokNotAuthenticated = "You are not authenticated."
	// grokXAIAPIKey 按字面说的是在用 XAI_API_KEY（程序文本，未实跑）。
	grokXAIAPIKey = "You are using XAI_API_KEY."
	// grokDeploymentKey 按字面说的是用 deployment key 认证（程序文本，未实跑）。
	grokDeploymentKey = "You are authenticated via deployment key."
	// grokModelKeyPrefix、grokModelKeySuffix 夹着模型名：那个模型配了自己的 API key
	// （实测，本机经 CPA 的 grok 就是这一行）。
	grokModelKeyPrefix = "Model '"
	grokModelKeySuffix = "' is using its own API key."
	// grokLoggedInPrefix、grokLoggedInSuffix 夹着一段内容（程序文本里是占位符）：按字面说的是
	// 登录过（程序文本，未实跑）。
	grokLoggedInPrefix = "You are logged in with "
	grokLoggedInSuffix = "."
)

// codex login status 的 API key 登录（程序文本，未实跑）。第一句在程序文本里后面接
// 「 - 」与一个占位符，按前缀认，那一段不上屏；第二句整句相等。
const (
	codexAPIKeyPrefix  = "Logged in using an API key"
	codexBedrockAPIKey = "Logged in using Amazon Bedrock API key"
)

// LoginFromDiscovery 从识别那一次的 stdout 判登录状态。claude 看 initialize 回应的 account，
// grok 看 stdout 首行；其余返回查不出。stdout 为 nil（例如识别超时）也是查不出。
func LoginFromDiscovery(providerID string, stdout []byte) LoginInfo {
	switch providerID {
	case "claude":
		return claudeLogin(stdout)
	case "grok":
		return grokLogin(stdout)
	default:
		return LoginInfo{}
	}
}

// claudeAccountLine 只读 initialize 回应里 response.response.account 的三个键。
type claudeAccountLine struct {
	Response *struct {
		Response *struct {
			Account *struct {
				TokenSource      string `json:"tokenSource"`
				SubscriptionType string `json:"subscriptionType"`
				APIKeySource     string `json:"apiKeySource"`
			} `json:"account"`
		} `json:"response"`
	} `json:"response"`
}

// claudeLogin 取第一份 request_id 为 council-models 的 control_response（与 parseClaudeDiscovery
// 认的是同一行），按顺序取第一个成立的：
//  1. subscriptionType 非空、不含控制字符、而且 tokenSource 不是 none：已登录，订阅记原值。
//  2. apiKeySource 非空、不是 none、不含控制字符：已登录（API key），来源记原值。
//  3. tokenSource 是 none、subscriptionType 与 apiKeySource 都为空：没登录。
//  4. 其余查不出。
//
// 实测：登录态下 account 有 email、organization、subscriptionType、apiProvider，没有
// tokenSource；CLAUDE_CONFIG_DIR 指空目录时只有 tokenSource（none）与 apiProvider。
// apiKeySource 取自 claude 2.1.283 的程序文本，未实跑：拼 account 的函数只在有 key 时写它，
// 值是 key 的来源（ANTHROPIC_API_KEY、apiKeyHelper、/login managed key 等）；同一份程序里
// system/init 消息的 apiKeySource 枚举把 none 说成没有用 key，所以 none 不算。订阅排在前面：
// 订阅与 key 同时在时显示订阅，已登录那一档的判据不变。
func claudeLogin(stdout []byte) LoginInfo {
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		if _, ok := claudeDiscoveryResponse(line); !ok {
			continue
		}
		var v claudeAccountLine
		if err := json.Unmarshal(bytes.TrimSpace(line), &v); err != nil || v.Response == nil || v.Response.Response == nil || v.Response.Response.Account == nil {
			return LoginInfo{}
		}
		a := v.Response.Response.Account
		switch {
		case a.SubscriptionType != "" && !hasControlChar(a.SubscriptionType) && a.TokenSource != "none":
			return LoginInfo{State: LoginYes, Subscription: a.SubscriptionType}
		case a.APIKeySource != "" && a.APIKeySource != "none" && !hasControlChar(a.APIKeySource):
			return LoginInfo{State: LoginAPIKey, KeySource: a.APIKeySource}
		case a.TokenSource == "none" && a.SubscriptionType == "" && a.APIKeySource == "":
			return LoginInfo{State: LoginNo}
		default:
			return LoginInfo{}
		}
	}
	return LoginInfo{}
}

// grokLogin 只看 stdout 首行（去掉首尾空白）：You are not authenticated. 是没登录；
// You are using XAI_API_KEY.、You are authenticated via deployment key.、
// Model '<模型>' is using its own API key. 是已登录（API key）；You are logged in with <内容>.
// 是已登录；其余查不出。
func grokLogin(stdout []byte) LoginInfo {
	first, _, _ := strings.Cut(string(stdout), "\n")
	first = strings.TrimSpace(first)
	switch {
	case first == grokNotAuthenticated:
		return LoginInfo{State: LoginNo}
	case first == grokXAIAPIKey, first == grokDeploymentKey, wrapped(first, grokModelKeyPrefix, grokModelKeySuffix):
		return LoginInfo{State: LoginAPIKey}
	case wrapped(first, grokLoggedInPrefix, grokLoggedInSuffix):
		return LoginInfo{State: LoginYes}
	default:
		return LoginInfo{}
	}
}

// wrapped 报告 s 是否以 prefix 开头、以 suffix 结尾，而且两者之间至少还有一个字节。
func wrapped(s, prefix, suffix string) bool {
	return len(s) > len(prefix)+len(suffix) && strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix)
}

// LoginStatusCommandFor 给出要单独跑一次的登录状态命令。今天只有 codex：codex login status，
// stdin 为空。其余 ok 为假：claude、grok 用识别的输出判，agy 没有零花费的办法。
func LoginStatusCommandFor(providerID, path string) (DiscoveryCommand, bool) {
	if providerID == "codex" {
		return DiscoveryCommand{Path: path, Args: []string{"login", "status"}}, true
	}
	return DiscoveryCommand{}, false
}

// ParseLoginStatus 判登录状态命令的结果。codex：逐行（去掉首尾空白）看，取第一行认得出的。
// 退出码 0、而且以 Logged in using an API key 开头或恰好是 Logged in using Amazon Bedrock API key，
// 是已登录（API key）；退出码 0、而且以 Logged in 开头的其余各行，是已登录；退出码非 0、
// 而且恰好是 Not logged in，是没登录；其余查不出。实测两种都打在 stderr：
// Logged in using ChatGPT（退出码 0）、Not logged in（退出码 1）；API key 那两句是程序文本，
// 未实跑。其余 provider 一律查不出。
func ParseLoginStatus(providerID string, exitCode int, stdout, stderr []byte) LoginInfo {
	if providerID != "codex" {
		return LoginInfo{}
	}
	lines := strings.Split(string(stdout)+"\n"+string(stderr), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if exitCode == 0 && (strings.HasPrefix(l, codexAPIKeyPrefix) || l == codexBedrockAPIKey) {
			return LoginInfo{State: LoginAPIKey}
		}
		if exitCode == 0 && strings.HasPrefix(l, "Logged in") {
			return LoginInfo{State: LoginYes}
		}
		if exitCode != 0 && l == "Not logged in" {
			return LoginInfo{State: LoginNo}
		}
	}
	return LoginInfo{}
}
