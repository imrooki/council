package provider

import "testing"

// oby_login_test.go 是 OB2（00_DECISIONS.md #48 追加裁决：配了 API key 算已登录）在 provider 包的验收：
// K1 从识别输出判出已登录（API key）与 grok 的已登录，K2 从 codex login status 判出已登录（API key）。
// 拼 claude 那一行复用 obx_login_test.go 的 obxClaudeInit（只读复用）。期望值逐字取自契约。

// obyGrokCPAOut 是本机实测的 grok models 全部 stdout（grok 1.0.41，经 CPA 走 API key；191 字节，LF）。
const obyGrokCPAOut = "Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n  - grok-4.6\n  - grok-4.5\n  - grok-4.3\n  - grok-composer-2.5-fast\n  - grok-build\n"

// TestObyLoginFromDiscovery 是 K1。
// 【判别力】见契约 §4：grok_cpa_real、grok_cpa_padded 对 N1；grok_xai_api_key 对 N2；grok_deployment_key 对 N3；
// grok_logged_in 对 N4；claude_key_helper、claude_key_no_token_source 对 N5、N9；claude_key_none 对 N6、M94；
// claude_key_control_char 对 N7、M94；claude_sub_and_key 对 N8；grok_logged_in_empty、grok_model_key_empty_name 对 N10；
// grok_xai_api_key_tail 对 X1；grok_deployment_key_prefixed 对 X2；grok_not_authenticated_tail 对 X3；
// grok_cpa_padded 对 X11（grok_cpa_crlf 只多 CR，X11 下不红）。
// 期望查不出的各行是负对照：少句点、名字为空、句后多字、句前多字、不在首行、来源是 none 或含控制字符、类型不对；
// grok_weblogin_deprecated 是程序文本里另一句认证句（契约 §0 第 3 条），不列入判据。
func TestObyLoginFromDiscovery(t *testing.T) {
	key := func(src string) LoginInfo { return LoginInfo{State: LoginAPIKey, KeySource: src} }
	unk := LoginInfo{}
	for _, c := range []struct {
		name, id, stdout string
		want             LoginInfo
	}{
		{"claude_key_helper", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"apiKeyHelper","apiProvider":"firstParty"}`), key("apiKeyHelper")},
		{"claude_key_no_token_source", "claude", obxClaudeInit("council-models", `{"apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`), key("ANTHROPIC_API_KEY")},
		{"claude_key_none", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"none","apiProvider":"firstParty"}`), unk},
		{"claude_key_control_char", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"k\u0007","apiProvider":"firstParty"}`), unk},
		{"claude_key_not_string", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":7}`), unk},
		{"claude_sub_and_key", "claude", obxClaudeInit("council-models", `{"subscriptionType":"Claude Max","apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`), LoginInfo{State: LoginYes, Subscription: "Claude Max"}},
		{"grok_cpa_real", "grok", obyGrokCPAOut, key("")},
		{"grok_cpa_crlf", "grok", "Model 'grok-4.7' is using its own API key.\r\n\r\nAvailable models:\r\n", key("")},
		{"grok_cpa_padded", "grok", " Model 'grok-4.7' is using its own API key. \n\nAvailable models:\n", key("")},
		{"grok_model_key_empty_name", "grok", "Model '' is using its own API key.\n", unk},
		{"grok_model_key_no_period", "grok", "Model 'grok-4.7' is using its own API key\n", unk},
		{"grok_xai_api_key", "grok", "You are using XAI_API_KEY.\n\nDefault model: grok-oby\n\nAvailable models:\n  * grok-oby (default)\n", key("")},
		{"grok_xai_api_key_no_period", "grok", "You are using XAI_API_KEY\n", unk},
		{"grok_xai_api_key_tail", "grok", "You are using XAI_API_KEY. oby\n", unk},
		{"grok_deployment_key", "grok", "You are authenticated via deployment key.\n\nDefault model: grok-oby\n", key("")},
		{"grok_deployment_key_tail", "grok", "You are authenticated via deployment key. oby\n", unk},
		{"grok_deployment_key_prefixed", "grok", "oby You are authenticated via deployment key.\n", unk},
		{"grok_logged_in", "grok", "You are logged in with oby@example.invalid.\n\nDefault model: grok-oby\n", LoginInfo{State: LoginYes}},
		{"grok_logged_in_empty", "grok", "You are logged in with .\n", unk},
		{"grok_logged_in_no_period", "grok", "You are logged in with oby@example.invalid\n", unk},
		{"grok_key_not_first", "grok", "Default model: grok-oby\nYou are using XAI_API_KEY.\n", unk},
		{"grok_not_authenticated_tail", "grok", "You are not authenticated. oby\n", unk},
		{"grok_weblogin_deprecated", "grok", "You are using a deprecated authentication method (WebLogin).\n", unk},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := LoginFromDiscovery(c.id, []byte(c.stdout)); got != c.want {
				t.Errorf("LoginFromDiscovery(%s, %q) = %+v，期望 %+v", c.id, c.stdout, got, c.want)
			}
		})
	}
}

// TestObyParseLoginStatus 是 K2。
// 【判别力】codex_api_key_stderr 对 N12、N17；codex_api_key_exit1 对 N14；codex_bedrock 对 N13；
// codex_bedrock_tail 对 N16。codex_chatgpt_real、codex_access_token、codex_personal_access_token 钉住
// 「其余以 Logged in 开头的照旧是已登录」。
func TestObyParseLoginStatus(t *testing.T) {
	key := LoginInfo{State: LoginAPIKey}
	yes := LoginInfo{State: LoginYes}
	unk := LoginInfo{}
	for _, c := range []struct {
		name, id       string
		code           int
		stdout, stderr string
		want           LoginInfo
	}{
		{"codex_api_key_stderr", "codex", 0, "", "Logged in using an API key - sk-oby***\n", key},
		{"codex_api_key_exit1", "codex", 1, "", "Logged in using an API key - sk-oby***\n", unk},
		{"codex_bedrock", "codex", 0, "", "Logged in using Amazon Bedrock API key\n", key},
		{"codex_bedrock_tail", "codex", 0, "", "Logged in using Amazon Bedrock API key (oby)\n", yes},
		{"codex_chatgpt_real", "codex", 0, "", "Logged in using ChatGPT\n", yes},
		{"codex_access_token", "codex", 0, "", "Logged in using access token\n", yes},
		{"codex_personal_access_token", "codex", 0, "", "Logged in using personal access token\n", yes},
		{"claude_api_key_line", "claude", 0, "", "Logged in using an API key - sk-oby***\n", unk},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseLoginStatus(c.id, c.code, []byte(c.stdout), []byte(c.stderr)); got != c.want {
				t.Errorf("ParseLoginStatus(%s, %d, %q, %q) = %+v，期望 %+v", c.id, c.code, c.stdout, c.stderr, got, c.want)
			}
		})
	}
}
