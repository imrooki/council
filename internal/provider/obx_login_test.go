package provider

import (
	"reflect"
	"testing"
)

// obx_login_test.go 是 OB1 v1.1（首次运行的登录状态，契约 §2.4）在 provider 包的验收：
// 从识别输出判登录状态（L1）、判 codex login status 的结果（L2）、登录状态命令（L3）。
// 期望值与表逐字取自契约。

// obxClaudeInit 拼一行 initialize 回应，account 那一段原样嵌进去。
func obxClaudeInit(requestID, account string) string {
	return `{"type":"control_response","response":{"subtype":"success","request_id":"` + requestID + `","response":{"models":[{"value":"opus","resolvedModel":"claude-obx"}],"account":` + account + `}}}` + "\n"
}

// TestObxLoginFromDiscovery 是 L1。
// 【判别力】claude_none_and_sub 对 M80、M82；claude_sub_control_char 对 M81；claude_real_shape、
// claude_other_request_only、claude_other_then_ours 对 M83；grok_marker_not_first 对 M84；
// grok_api_key_line 对 OB2 的 N1；claude_api_key_env、claude_console_login 对 OB2 的 N5（claude_isolated_shape
// 是它们的正对照，只差有没有 apiKeySource）。OB2 起这三行的期望是已登录（API key）。
func TestObxLoginFromDiscovery(t *testing.T) {
	yes := func(sub string) LoginInfo { return LoginInfo{State: LoginYes, Subscription: sub} }
	no := LoginInfo{State: LoginNo}
	unk := LoginInfo{}
	for _, c := range []struct {
		name, id, stdout string
		want             LoginInfo
	}{
		{"claude_real_shape", "claude", `{"type":"system","subtype":"init"}` + "\n" + obxClaudeInit("council-models", `{"email":"obx@example.invalid","organization":"obx","subscriptionType":"Subscription","apiProvider":"firstParty"}`), yes("Subscription")},
		{"claude_isolated_shape", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiProvider":"firstParty"}`), no},
		{"claude_no_account", "claude", `{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[]}}}` + "\n", unk},
		{"claude_account_empty", "claude", obxClaudeInit("council-models", `{"apiProvider":"firstParty"}`), unk},
		{"claude_none_and_sub", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","subscriptionType":"max"}`), unk},
		{"claude_sub_control_char", "claude", obxClaudeInit("council-models", `{"subscriptionType":"max\u0007"}`), unk},
		{"claude_sub_not_string", "claude", obxClaudeInit("council-models", `{"subscriptionType":7}`), unk},
		{"claude_other_request_only", "claude", obxClaudeInit("obx-other", `{"subscriptionType":"max"}`), unk},
		{"claude_other_then_ours", "claude", obxClaudeInit("obx-other", `{"subscriptionType":"max"}`) + obxClaudeInit("council-models", `{"tokenSource":"none"}`), no},
		{"claude_api_key_env", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`), LoginInfo{State: LoginAPIKey, KeySource: "ANTHROPIC_API_KEY"}},
		{"claude_console_login", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"/login managed key","organization":"obx","email":"obx@example.invalid","apiProvider":"firstParty"}`), LoginInfo{State: LoginAPIKey, KeySource: "/login managed key"}},
		{"claude_nil", "claude", "", unk},
		{"grok_not_authenticated", "grok", "You are not authenticated.\n\nDefault model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n  - grok-4.5\n", no},
		{"grok_not_authenticated_crlf", "grok", "You are not authenticated.\r\nAvailable models:\r\n", no},
		{"grok_api_key_line", "grok", "Model 'grok-4.7' is using its own API key.\n\nAvailable models:\n  * grok-4.7 (default)\n", LoginInfo{State: LoginAPIKey}},
		{"grok_marker_not_first", "grok", "Available models:\nYou are not authenticated.\n", unk},
		{"grok_empty", "grok", "", unk},
		{"agy", "agy", `{"status":"SUCCESS"}`, unk},
		{"codex", "codex", `{"models":[]}`, unk},
	} {
		t.Run(c.name, func(t *testing.T) {
			var in []byte
			if c.stdout != "" {
				in = []byte(c.stdout)
			}
			if got := LoginFromDiscovery(c.id, in); got != c.want {
				t.Errorf("LoginFromDiscovery(%s, %q) = %+v，期望 %+v", c.id, c.stdout, got, c.want)
			}
		})
	}
}

// TestObxParseLoginStatus 是 L2。
// 【判别力】codex_logged_in_but_exit1 对 M86；codex_not_logged_in_but_exit0 对 M87；
// codex_not_logged_in_padded 对 M88；claude 对 M90。
func TestObxParseLoginStatus(t *testing.T) {
	yes := LoginInfo{State: LoginYes}
	no := LoginInfo{State: LoginNo}
	unk := LoginInfo{}
	for _, c := range []struct {
		name, id       string
		code           int
		stdout, stderr string
		want           LoginInfo
	}{
		{"codex_real_logged_in", "codex", 0, "", "Logged in using a subscription\n", yes},
		{"codex_logged_in_stdout_crlf", "codex", 0, "Logged in using an API key - sk-obx***\r\n", "", LoginInfo{State: LoginAPIKey}},
		{"codex_real_not_logged_in", "codex", 1, "", "Not logged in\n", no},
		{"codex_not_logged_in_padded", "codex", 1, "", "  Not logged in  \r\n", no},
		{"codex_logged_in_but_exit1", "codex", 1, "", "Logged in using a subscription\n", unk},
		{"codex_not_logged_in_but_exit0", "codex", 0, "", "Not logged in\n", unk},
		{"codex_other_error", "codex", 3, "", "obx-boom\n", unk},
		{"codex_exit0_silent", "codex", 0, "", "", unk},
		{"claude", "claude", 0, "", "Logged in using a subscription\n", unk},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseLoginStatus(c.id, c.code, []byte(c.stdout), []byte(c.stderr)); got != c.want {
				t.Errorf("ParseLoginStatus(%s, %d, %q, %q) = %+v，期望 %+v", c.id, c.code, c.stdout, c.stderr, got, c.want)
			}
		})
	}
}

// TestObxLoginStatusCommandFor 是 L3：只有 codex 有登录状态命令。
// 【判别力】M89。
func TestObxLoginStatusCommandFor(t *testing.T) {
	cmd, ok := LoginStatusCommandFor("codex", `C:\x\codex.cmd`)
	want := DiscoveryCommand{Path: `C:\x\codex.cmd`, Args: []string{"login", "status"}}
	if !ok || !reflect.DeepEqual(cmd, want) {
		t.Errorf("LoginStatusCommandFor(codex) = (%+v, %v)，期望 (%+v, true)", cmd, ok, want)
	}
	for _, id := range []string{"claude", "grok", "agy", "obx"} {
		cmd, ok := LoginStatusCommandFor(id, `C:\x\`+id+`.cmd`)
		if ok || !reflect.DeepEqual(cmd, DiscoveryCommand{}) {
			t.Errorf("LoginStatusCommandFor(%s) = (%+v, %v)，期望零值与 false", id, cmd, ok)
		}
	}
}
