package serve

// models_test.go 覆盖 GUI 第 8 片的两个端点：GET /api/models（识别本机模型）
// 与 POST /api/conversations/{id}/members/from-model（按识别结果加成员）。
//
// 识别本身（起进程、解析各家输出）由 internal/provider 自己的用例覆盖；这里
// 换掉 discoverProviderModels 这一层，只钉三件事：菜单拿到的形状对不对、
// 只读模式回 405、加成员真的写了配置、换了 Runner 的配置并加入会话。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

// stubModelDiscovery 把「一家 provider 的识别」整个换掉。
func stubModelDiscovery(t *testing.T, rows map[string][]provider.DiscoveredModel, errs map[string]error) {
	t.Helper()
	old := discoverProviderModels
	discoverProviderModels = func(ctx context.Context, providerID, path string, exec provider.DiscoveryExec) ([]provider.DiscoveredModel, error) {
		if err := errs[providerID]; err != nil {
			return nil, err
		}
		return rows[providerID], nil
	}
	t.Cleanup(func() { discoverProviderModels = old })
}

// always405 是只读模式下的期望状态码。
func TestModelsReadOnly405(t *testing.T) {
	env := newWriteTestEnv(t)
	ro := NewServer(env.store, "test-version", nil)

	rec := httptest.NewRecorder()
	ro.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式 GET /api/models = %d，期望 405；body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "-rw") {
		t.Errorf("405 的说明里没有提到 -rw：%s", rec.Body.String())
	}
}

// TestModelsListsDiscovered 钉住 /api/models 的响应形状：一行一个模型，
// 默认强度取该模型自己的档，建议 id 非空；一家识别失败时那家单独带 error，
// 不拖垮整张表。
func TestModelsListsDiscovered(t *testing.T) {
	env := newWriteTestEnv(t)
	stubModelDiscovery(t, map[string][]provider.DiscoveredModel{
		"claude": {{
			Provider: "claude",
			Model:    "claude-opus-5-5",
			Name:     "Opus 5.5",
			Efforts:  []string{"low", "high"},
		}},
	}, nil)

	res, err := http.Get(env.http.URL + "/api/models")
	if err != nil {
		t.Fatalf("GET /api/models 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/models = %d，期望 200", res.StatusCode)
	}
	var body struct {
		Models []struct {
			Provider      string   `json:"provider"`
			Model         string   `json:"model"`
			Efforts       []string `json:"efforts"`
			DefaultEffort string   `json:"default_effort"`
			SuggestedID   string   `json:"suggested_id"`
			InConfig      bool     `json:"in_config"`
		} `json:"models"`
		Providers []struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if len(body.Models) != 1 {
		t.Fatalf("models 行数 = %d，期望 1：%+v", len(body.Models), body.Models)
	}
	got := body.Models[0]
	if got.Provider != "claude" || got.Model != "claude-opus-5-5" {
		t.Errorf("模型行 = %+v", got)
	}
	if got.DefaultEffort != "high" {
		t.Errorf("default_effort = %q，期望 high（有 high 就取 high）", got.DefaultEffort)
	}
	if got.SuggestedID == "" {
		t.Error("suggested_id 为空：前端没有可写的 agent id")
	}
	if got.InConfig {
		t.Error("in_config = true，但这个模型还没写进配置")
	}
	if len(body.Providers) != 1 || body.Providers[0].ID != "claude" {
		t.Errorf("providers = %+v，期望只有 claude 一家", body.Providers)
	}
}

// TestAddMemberFromModel 钉住写配置 + 热换 Runner + 加入会话这条链：
// 加完之后配置里要有这一段、Runner 的配置要认得它、会话成员表里要有它。
func TestAddMemberFromModel(t *testing.T) {
	env := newWriteTestEnv(t)
	stubModelDiscovery(t, map[string][]provider.DiscoveredModel{
		"claude": {{
			Provider: "claude",
			Model:    "claude-opus-5-5",
			Name:     "Opus 5.5",
			Efforts:  []string{"low", "high"},
		}},
	}, nil)

	// 设置页与 from-model 都写「最高优先层」那一份文件；测试环境里它就是
	// data_dir 旁边那份 config.toml。
	cfgPath := filepath.Join(filepath.Dir(env.srv.write.DataDir), "config.toml")
	env.srv.write.Layers = []config.Layer{{Path: cfgPath}}
	env.srv.write.ConfigPath = cfgPath

	conv := gui4SeedConversation(t, env, "from-model")

	payload := `{"provider":"claude","model":"claude-opus-5-5","effort":"high"}`
	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/members/from-model",
		"application/json", payload)
	if status != http.StatusOK {
		t.Fatalf("from-model = %d，期望 200；body=%s", status, raw)
	}

	var resp struct {
		ConversationID string `json:"conversation_id"`
		Members        []struct {
			AgentID string `json:"agent_id"`
			Active  bool   `json:"active"`
		} `json:"members"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("响应不是合法 JSON: %v；body=%s", err, raw)
	}
	if resp.ConversationID != conv.ID {
		t.Fatalf("conversation_id = %q，期望 %q", resp.ConversationID, conv.ID)
	}
	added := ""
	for _, m := range resp.Members {
		if m.Active && strings.Contains(m.AgentID, "opus") {
			added = m.AgentID
		}
	}
	if added == "" {
		t.Fatalf("成员表里没有刚加的成员：%s", raw)
	}

	written, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("读配置文件失败: %v", err)
	}
	if !strings.Contains(string(written), "claude-opus-5-5") {
		t.Errorf("配置文件里没有新成员：\n%s", written)
	}

	// 热换：不重启也得认得这个成员，否则菜单里加进来的人根本没法发言。
	if _, ok := env.srv.runCfg().Agents[added]; !ok {
		t.Errorf("Runner 的配置里没有 %s：新成员要重启才能用", added)
	}
}
