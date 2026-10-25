package serve

// gui7_regression_test.go 钉住三个已确认小 bug 的回归：
//
//  1. /api/options.agents[].runnable 反映「该 id 是否在 Runner 启动时拿到
//     的 RunCfg 里」，/api/ask 选中设置页新增成员时给出「重启后可用」的
//     根因，而不是泛泛的「未知成员」；
//  2. 已软删的会话再 rename 必须 404，且库里的标题一个字节都不变；
//  3. 汇总下拉只列 read 权限成员、没有 read 成员时禁用应用——前端行为由
//     真实 CDP 验收覆盖，这里只钉 index.html 里的结构锚点。
//
// 第 1 条用「换掉展示配置的指针、保留 RunCfg」的最小装配模拟设置页保存，
// 不重跑写配置文件那一整套；那套流程已经由 gui6_settings_test.go 覆盖。

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"

	_ "modernc.org/sqlite"
)

// gui7InstallDisplayAgent 只改展示配置（WriteDeps.Cfg），RunCfg 保持启动
// 时那一份——这正是设置页保存之后进程里的状态。
func gui7InstallDisplayAgent(t *testing.T, env *writeTestEnv, id string, ac config.AgentConfig) {
	t.Helper()
	run := env.srv.runCfg()
	if run == nil {
		t.Fatal("测试装配缺少 RunCfg")
	}
	display := *run
	display.Agents = make(map[string]config.AgentConfig, len(run.Agents)+1)
	for existingID, existing := range run.Agents {
		display.Agents[existingID] = existing
	}
	display.Agents[id] = ac

	env.srv.cfgMu.Lock()
	env.srv.write.Cfg = &display
	env.srv.cfgMu.Unlock()
}

// gui7OptionByID 从 /api/options 里取一条 agent 的原始 JSON 与结构体。
func gui7OptionByID(t *testing.T, env *writeTestEnv, id string) (agentOption, map[string]json.RawMessage) {
	t.Helper()
	var opts optionsResponse
	getJSON(t, env.http.URL+"/api/options", http.StatusOK, &opts)
	for _, agent := range opts.Agents {
		if agent.ID == id {
			var raw struct {
				Agents []map[string]json.RawMessage `json:"agents"`
			}
			// 原始 JSON 单独取一次：runnable 必须真的出现在线上响应里，
			// 不能只是结构体里有个零值字段。
			res, err := http.Get(env.http.URL + "/api/options")
			if err != nil {
				t.Fatalf("GET /api/options 失败: %v", err)
			}
			defer res.Body.Close()
			if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
				t.Fatalf("解码 /api/options 原始 JSON 失败: %v", err)
			}
			for _, entry := range raw.Agents {
				var entryID string
				if err := json.Unmarshal(entry["id"], &entryID); err == nil && entryID == id {
					return agent, entry
				}
			}
			t.Fatalf("/api/options 原始 JSON 里找不到 %q", id)
		}
	}
	t.Fatalf("/api/options 里找不到 agent %q: %+v", id, opts.Agents)
	return agentOption{}, nil
}

// TestGui7OptionsRunnableAndSettingsAddedHint 是 Bug 1 的回归：
//   - 启动配置里的 c1 → runnable=true；
//   - 只有展示配置里有的 p9 → runnable=false，但仍在列表里；
//   - /api/ask 选 p9（或把它当汇总成员）时 400 文案点名「设置页新增 /
//     重启 council serve 后可用」，不能再说「未知成员」。
func TestGui7OptionsRunnableAndSettingsAddedHint(t *testing.T) {
	env := newWriteTestEnv(t)

	c1, c1Raw := gui7OptionByID(t, env, "c1")
	if !c1.Runnable {
		t.Errorf("启动配置里的 c1 runnable = false，期望 true")
	}
	if raw, ok := c1Raw["runnable"]; !ok || string(raw) != "true" {
		t.Errorf("c1 的原始 JSON runnable = %s (存在=%v)，期望 true", raw, ok)
	}

	gui7InstallDisplayAgent(t, env, "p9", config.AgentConfig{
		DisplayName: "Project Nine",
		Provider:    "claude",
		Model:       "sonnet",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	})

	p9, p9Raw := gui7OptionByID(t, env, "p9")
	if p9.Runnable {
		t.Errorf("只在展示配置里的 p9 runnable = true，期望 false")
	}
	if raw, ok := p9Raw["runnable"]; !ok || string(raw) != "false" {
		t.Errorf("p9 的原始 JSON runnable = %s (存在=%v)，期望 false", raw, ok)
	}
	if p9.Source != agentSourceConfig {
		t.Errorf("p9 source = %q，期望 %q", p9.Source, agentSourceConfig)
	}

	t.Run("选成普通成员", func(t *testing.T) {
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json",
			`{"prompt":"hi","members":["p9"]}`)
		if status != http.StatusBadRequest {
			t.Fatalf("选 p9 的 /api/ask = %d，期望 400；body=%s", status, raw)
		}
		var reply struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("400 不是合法 JSON: %v；%s", err, raw)
		}
		for _, want := range []string{"p9", "该成员是设置页新增的", "重启 council serve 后可用"} {
			if !strings.Contains(reply.Error, want) {
				t.Errorf("400 的说明 = %q，期望包含 %q", reply.Error, want)
			}
		}
		if strings.Contains(reply.Error, "未知成员") {
			t.Errorf("400 的说明 = %q，不该再回泛泛的「未知成员」", reply.Error)
		}
	})

	t.Run("选成汇总成员", func(t *testing.T) {
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json",
			`{"prompt":"hi","members":["c1"],"summary":"p9"}`)
		if status != http.StatusBadRequest {
			t.Fatalf("汇总用 p9 的 /api/ask = %d，期望 400；body=%s", status, raw)
		}
		var reply struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("400 不是合法 JSON: %v；%s", err, raw)
		}
		for _, want := range []string{"p9", "汇总成员", "该成员是设置页新增的", "重启 council serve 后可用"} {
			if !strings.Contains(reply.Error, want) {
				t.Errorf("400 的说明 = %q，期望包含 %q", reply.Error, want)
			}
		}
	})

	t.Run("拼错的 id 仍按未知成员报", func(t *testing.T) {
		status, _, raw := postRaw(t, env.http.URL+"/api/ask", "application/json",
			`{"prompt":"hi","members":["typo"]}`)
		if status != http.StatusBadRequest {
			t.Fatalf("选 typo 的 /api/ask = %d，期望 400；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "未知成员") {
			t.Errorf("拼错 id 的 400 = %s，期望保留「未知成员」", raw)
		}
		if strings.Contains(string(raw), "设置页新增") {
			t.Errorf("拼错 id 的 400 = %s，不该说成设置页新增", raw)
		}
	})
}

// TestGui7RenameSoftDeletedConversationNotFound 是 Bug 2 的回归：软删之后
// 再 rename 必须 404，并且直接查库确认标题仍是删除前的值。
func TestGui7RenameSoftDeletedConversationNotFound(t *testing.T) {
	env := newWriteTestEnv(t)
	const original = "删除前的标题"
	conv := gui3aSeedConversation(t, env, original)

	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/delete",
		"application/json", `{}`)
	if status != http.StatusOK {
		t.Fatalf("先软删会话 = %d，期望 200；body=%s", status, raw)
	}

	status, _, raw = postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/rename",
		"application/json", `{"title":"删除后偷偷改掉"}`)
	if status != http.StatusNotFound {
		t.Fatalf("软删后 rename = %d，期望 404；body=%s", status, raw)
	}
	var reply struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("404 不是合法 JSON: %v；%s", err, raw)
	}
	if !strings.Contains(reply.Error, "会话不存在") || !strings.Contains(reply.Error, conv.ID) {
		t.Errorf("404 的说明 = %q，期望点名会话不存在与 id %q", reply.Error, conv.ID)
	}

	// 直接查库：store.Conversation 已经把已删读成不存在，看不到标题。
	dbPath := filepath.Join(env.srv.write.DataDir, "council.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		t.Fatalf("只读打开测试库失败: %v", err)
	}
	defer db.Close()
	var (
		title     string
		deletedAt sql.NullString
	)
	if err := db.QueryRow(
		`SELECT title, deleted_at FROM conversations WHERE id = ?`, conv.ID,
	).Scan(&title, &deletedAt); err != nil {
		t.Fatalf("直接查会话行失败: %v", err)
	}
	if title != original {
		t.Errorf("库里的标题 = %q，期望仍是删除前的 %q", title, original)
	}
	if !deletedAt.Valid {
		t.Errorf("deleted_at = NULL，期望会话仍处于软删状态")
	}
}

// TestGui7IndexSummaryReadPermissionAnchors 钉住 Bug 3 的前端结构锚点。
// 真正的过滤、禁用与提示由 CDP 在真实 Chromium 里点一次（见交付报告）；
// 这里只防止以后改前端时把这些关键片段删掉而不自知。
func TestGui7IndexSummaryReadPermissionAnchors(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		`function unrunnableHint()`,
		`t('ui.members.unrunnableHint', '设置页新增，重启 council serve 后可用')`,
		`agent.permission === SUMMARY_PERMISSION`,
		`function summaryNoneRead()`,
		`t('ui.composer.summaryNoRead', '（没有 read 权限的成员可用）')`,
		`empty.textContent = summaryNoneRead()`,
		`paramApply.disabled = candidates.length === 0`,
		`summaryProblemMessage(values.summary)`,
		`summaryProblemMessage(summary)`,
		`if (summaryMessage) return summaryMessage`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
}
