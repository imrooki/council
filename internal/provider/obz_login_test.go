package provider

import "testing"

// obz_login_test.go 是 OB3（契约 §2.1）补 OB2 审查剩下的三行负对照（契约 19e 文件头）：grok 首行的三句
// 句前多了字时都是查不出。每行配一个只差句前 "obz " 的正对照（同一张表、同一个断言）。

// TestObzGrokPrefixedLines 是 K3。
// 【判别力】grok_model_key_prefixed 对 Q1（本地转发服务 那一句放宽成只看后缀加长度）；grok_xai_api_key_prefixed 对
// Q2（XAI_API_KEY 那一句整句相等放宽成 HasSuffix）；grok_not_authenticated_prefixed 对 Q3（没登录那一句
// 整句相等放宽成 HasSuffix）。三行不带前缀的是正对照：夹具与判据都对时它们不是查不出。
func TestObzGrokPrefixedLines(t *testing.T) {
	for _, c := range []struct {
		name, stdout string
		want         LoginInfo
	}{
		{"grok_model_key", "Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n", LoginInfo{State: LoginAPIKey}},
		{"grok_model_key_prefixed", "obz Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n", LoginInfo{}},
		{"grok_xai_api_key", "You are using XAI_API_KEY.\n", LoginInfo{State: LoginAPIKey}},
		{"grok_xai_api_key_prefixed", "obz You are using XAI_API_KEY.\n", LoginInfo{}},
		{"grok_not_authenticated", "You are not authenticated.\n", LoginInfo{State: LoginNo}},
		{"grok_not_authenticated_prefixed", "obz You are not authenticated.\n", LoginInfo{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := LoginFromDiscovery("grok", []byte(c.stdout)); got != c.want {
				t.Errorf("LoginFromDiscovery(grok, %q) = %+v，期望 %+v", c.stdout, got, c.want)
			}
		})
	}
}
