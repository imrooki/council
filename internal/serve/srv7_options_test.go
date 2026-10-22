package serve

// srv7_options_test.go 覆盖 SRV7 §3/§5.4 在 HTTP 一层的落点：
// /api/options 的 agents 里每条都带 source，Claude 来源的值是 "claude"，
// 配置来源的值是 "config"，同名时配置优先、不出现重复条目。

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"council/internal/config"
)

// TestSrv7OptionsAgentsCarrySource 钉住 /api/options 的合并与 source 字段。
func TestSrv7OptionsAgentsCarrySource(t *testing.T) {
	env := newWriteTestEnv(t) // 配置里有一个 agent c1
	env.srv.write.ClaudeAgents = map[string]config.AgentConfig{
		// 与配置同名：必须被配置盖住，响应里只有一条 c1（source=config）。
		"c1": {
			DisplayName: "Claude C1",
			Provider:    "claude",
			Model:       "sonnet",
			Permissions: config.PermissionRead,
			Mode:        config.ModeDiscussion,
			Prompt:      "这段提示本片不使用",
		},
		"critic": {
			DisplayName: "Critic",
			Provider:    "claude",
			Model:       "sonnet",
			Effort:      "high",
			Role:        "质疑假设",
			Permissions: config.PermissionRead,
			Mode:        config.ModeDiscussion,
			Prompt:      "你是批评者。",
		},
	}

	// 先拿原始 body：source 必须真的出现在 JSON 里，而不只是结构体字段。
	res, err := http.Get(env.http.URL + "/api/options")
	if err != nil {
		t.Fatalf("GET /api/options 失败: %v", err)
	}
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Fatalf("读 /api/options 响应失败: %v", err)
	}
	t.Logf("GET /api/options -> %s", body)

	var got optionsResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("options 响应不是合法 JSON: %v\n%s", err, body)
	}

	if len(got.Agents) != 2 {
		t.Fatalf("agents 应当恰好 2 条（c1 配置覆盖 + critic 来自 Claude），实际 %+v", got.Agents)
	}
	if got.Agents[0].ID != "c1" || got.Agents[1].ID != "critic" {
		t.Fatalf("agents 顺序 = [%s %s]，期望按 id 字典序 [c1 critic]",
			got.Agents[0].ID, got.Agents[1].ID)
	}

	c1 := got.Agents[0]
	if c1.Source != agentSourceConfig {
		t.Errorf("配置里的 c1 source = %q，期望 %q", c1.Source, agentSourceConfig)
	}
	if c1.DisplayName != "C1" {
		t.Errorf("同名时应当保留配置里的定义，display_name = %q，期望 C1", c1.DisplayName)
	}

	critic := got.Agents[1]
	if critic.Source != agentSourceClaude {
		t.Errorf("Claude 导入的 critic source = %q，期望 %q", critic.Source, agentSourceClaude)
	}
	if critic.Provider != "claude" || critic.Model != "sonnet" || critic.Effort != "high" ||
		critic.Permission != "read" || critic.Mode != "discussion" || !critic.Active {
		t.Errorf("critic 的字段不完整或口径不对: %+v", critic)
	}

	// 原始 JSON：每条 agent 都必须有 source 键，值分别是 config / claude。
	var raw struct {
		Agents []map[string]json.RawMessage `json:"agents"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("按原始 JSON 再解一次失败: %v", err)
	}
	if len(raw.Agents) != 2 {
		t.Fatalf("原始 JSON 里 agents 有 %d 条，期望 2", len(raw.Agents))
	}
	wantSources := []string{agentSourceConfig, agentSourceClaude}
	for i, entry := range raw.Agents {
		got, ok := entry["source"]
		if !ok {
			t.Errorf("第 %d 条 agent 的原始 JSON 缺少 source 键: %v", i, entry)
			continue
		}
		var source string
		if err := json.Unmarshal(got, &source); err != nil {
			t.Errorf("第 %d 条 agent 的 source 不是字符串: %s", i, got)
			continue
		}
		if source != wantSources[i] {
			t.Errorf("第 %d 条 agent 的 source = %q，期望 %q", i, source, wantSources[i])
		}
	}
}
