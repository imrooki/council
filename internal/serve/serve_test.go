package serve

// serve_test.go 覆盖 SRV1 §6 的 HTTP 侧：四个端点、200/400/404、字段名与顺序，
// 以及 §3 的 flag 解析与回环地址校验。全部用例走 httptest，不监听真实端口、
// 不打开浏览器、不启动任何 provider。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"
)

type serveTestEnv struct {
	store *store.Store
	http  *httptest.Server
}

// newServeTestEnv 在临时目录建一个真库，并把它挂到一个 httptest 服务上。
// handler 只读，写路径仅用于测试造数；看板本身不会调用 WriteTx。
func newServeTestEnv(t *testing.T, models map[string]string) *serveTestEnv {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatalf("建测试库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试库失败: %v", err)
		}
	})

	srv := NewServer(st, "test-version", models)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &serveTestEnv{store: st, http: ts}
}

func (e *serveTestEnv) ctx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func (e *serveTestEnv) createConversation(t *testing.T, title string) conversation.Conversation {
	t.Helper()
	ctx := e.ctx(t)
	var conv conversation.Conversation
	err := e.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, title)
		return err
	})
	if err != nil {
		t.Fatalf("建会话 %q 失败: %v", title, err)
	}
	return conv
}

func (e *serveTestEnv) join(t *testing.T, convID, agentID, displayName, provider string) {
	t.Helper()
	ctx := e.ctx(t)
	err := e.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.AddParticipant(ctx, conversation.Participant{
			ConvID:      convID,
			AgentID:     agentID,
			DisplayName: displayName,
			Provider:    provider,
			Active:      true,
		})
	})
	if err != nil {
		t.Fatalf("加入成员 %q 失败: %v", agentID, err)
	}
}

func (e *serveTestEnv) leave(t *testing.T, convID, agentID string) {
	t.Helper()
	ctx := e.ctx(t)
	err := e.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.DeactivateParticipant(ctx, convID, agentID, time.Now().UTC())
	})
	if err != nil {
		t.Fatalf("退出成员 %q 失败: %v", agentID, err)
	}
}

func (e *serveTestEnv) message(t *testing.T, convID, senderID, text string) conversation.Message {
	t.Helper()
	ctx := e.ctx(t)
	var msg conversation.Message
	err := e.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var err error
		msg, err = tx.AppendMessage(ctx, conversation.Message{
			ConvID:   convID,
			SenderID: senderID,
			Kind:     conversation.KindUser,
			Text:     text,
		})
		return err
	})
	if err != nil {
		t.Fatalf("追加消息 %q 失败: %v", text, err)
	}
	return msg
}

func (e *serveTestEnv) usage(t *testing.T, u conversation.TurnUsage) {
	t.Helper()
	ctx := e.ctx(t)
	err := e.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.RecordUsage(ctx, u)
	})
	if err != nil {
		t.Fatalf("记录用量 %q 失败: %v", u.TurnID, err)
	}
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func getJSON(t *testing.T, url string, wantStatus int, out any) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s 失败: %v", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != wantStatus {
		var body bytes.Buffer
		_, _ = body.ReadFrom(res.Body)
		t.Fatalf("GET %s = %d，期望 %d；body: %s", url, res.StatusCode, wantStatus, body.String())
	}
	if got := res.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("GET %s 的 Content-Type = %q，期望 application/json; charset=utf-8", url, got)
	}
	if out != nil {
		dec := json.NewDecoder(res.Body)
		if err := dec.Decode(out); err != nil {
			t.Fatalf("GET %s 的响应不是合法 JSON: %v", url, err)
		}
	}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// 数量小，直接插入排序，避免为一个测试引入 sort 的额外导入。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func requireKeys(t *testing.T, what string, obj map[string]any, want ...string) {
	t.Helper()
	got := sortedKeys(obj)
	if len(got) != len(want) {
		t.Fatalf("%s 的字段 = %v，期望 %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s 的字段 = %v，期望 %v", what, got, want)
		}
	}
}

func TestServerHealth(t *testing.T) {
	env := newServeTestEnv(t, nil)
	var got map[string]any
	getJSON(t, env.http.URL+"/api/health", http.StatusOK, &got)
	requireKeys(t, "health", got, "db", "ok", "schema_expected_version", "schema_lag", "schema_version", "version")
	if got["ok"] != true {
		t.Errorf("health.ok = %v，期望 true", got["ok"])
	}
	if got["version"] != "test-version" {
		t.Errorf("health.version = %v，期望 test-version", got["version"])
	}
	if got["db"] != env.store.Path() {
		t.Errorf("health.db = %v，期望 %q", got["db"], env.store.Path())
	}
	if got["schema_version"] != float64(env.store.SchemaVersion()) {
		t.Errorf("health.schema_version = %v，期望 %d", got["schema_version"], env.store.SchemaVersion())
	}
	if got["schema_expected_version"] != float64(env.store.ExpectedSchemaVersion()) {
		t.Errorf("health.schema_expected_version = %v，期望 %d",
			got["schema_expected_version"], env.store.ExpectedSchemaVersion())
	}
	if got["schema_lag"] != false {
		t.Errorf("最新库的 health.schema_lag = %v，期望 false", got["schema_lag"])
	}
}

func TestServerConversationsOrderAndShape(t *testing.T) {
	env := newServeTestEnv(t, nil)
	first := env.createConversation(t, "先建的会话")
	env.join(t, first.ID, "alpha", "Alpha", "claude")
	env.message(t, first.ID, conversation.ReservedSenderUser, "第一句")

	// 两个会话的 created_at 与消息时间分开：把第一条消息追加到先建的会话，
	// 它应当因为「最后活动」更晚而排到前面，而不是靠创建时间。
	time.Sleep(20 * time.Millisecond)
	second := env.createConversation(t, "后建的会话")
	env.message(t, second.ID, conversation.ReservedSenderUser, "第二句")
	time.Sleep(20 * time.Millisecond)
	env.message(t, first.ID, conversation.ReservedSenderUser, "先建的会话后来又说了一句")

	var list []map[string]any
	getJSON(t, env.http.URL+"/api/conversations", http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("会话数 = %d，期望 2", len(list))
	}
	if list[0]["id"] != first.ID || list[1]["id"] != second.ID {
		t.Fatalf("会话顺序 = [%v, %v]，期望按最后活动倒序 [%v, %v]",
			list[0]["id"], list[1]["id"], first.ID, second.ID)
	}
	for i, row := range list {
		requireKeys(t, "conversation", row,
			"archived", "created_at", "cwd", "id", "last_activity", "last_seq", "members",
			"message_count", "team_id", "title", "workspace_id")
		if row["archived"] != false {
			t.Errorf("未归档会话的 archived = %v，期望 false", row["archived"])
		}
		if i == 0 {
			if row["members"] != float64(1) {
				t.Errorf("第一条会话的 members = %v，期望 1（只数活跃成员）", row["members"])
			}
			if row["message_count"] != float64(2) {
				t.Errorf("第一条会话的 message_count = %v，期望 2", row["message_count"])
			}
			if row["last_seq"] != float64(2) {
				t.Errorf("第一条会话的 last_seq = %v，期望 2", row["last_seq"])
			}
			if row["title"] != "先建的会话" {
				t.Errorf("第一条会话的 title = %v", row["title"])
			}
		}
	}
	if list[0]["last_activity"] == list[1]["last_activity"] {
		t.Errorf("两条会话的 last_activity 不应相同: %v", list[0]["last_activity"])
	}
}

func TestServerConversationsEmptyIsArray(t *testing.T) {
	env := newServeTestEnv(t, nil)
	var list []map[string]any
	getJSON(t, env.http.URL+"/api/conversations", http.StatusOK, &list)
	if list == nil {
		t.Fatal("空库的会话列表应当是 []，实际解出 nil")
	}
	if len(list) != 0 {
		t.Fatalf("空库返回了 %d 条会话", len(list))
	}
}

func TestServerConversationDetail(t *testing.T) {
	env := newServeTestEnv(t, nil)
	conv := env.createConversation(t, "详情")
	env.join(t, conv.ID, "beta", "Beta", "grok")
	env.join(t, conv.ID, "alpha", "Alpha", "claude")
	env.leave(t, conv.ID, "beta")

	var got map[string]any
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID, http.StatusOK, &got)
	requireKeys(t, "conversation detail", got,
		"archived", "created_at", "cwd", "id", "last_activity", "last_seq", "members",
		"message_count", "team_id", "title", "workspace_id")
	if got["archived"] != false {
		t.Errorf("未归档会话详情的 archived = %v，期望 false", got["archived"])
	}
	members, ok := got["members"].([]any)
	if !ok {
		t.Fatalf("members = %T，期望数组", got["members"])
	}
	if len(members) != 2 {
		t.Fatalf("members 有 %d 项，期望 2（含已退出的一位）", len(members))
	}
	first, _ := members[0].(map[string]any)
	second, _ := members[1].(map[string]any)
	// model 是 GUI4 新增的字段：participants 表里没有它，详情按启动时
	// 的配置补。这里 newServeTestEnv 传的 models 是 nil，所以值是空串，
	// 但字段必须存在，前端才能稳定地按同一形状渲染。
	requireKeys(t, "member", first, "active", "agent_id", "display_name", "model", "provider")
	requireKeys(t, "member", second, "active", "agent_id", "display_name", "model", "provider")
	if first["agent_id"] != "alpha" || first["active"] != true {
		t.Errorf("members[0] = %v，期望 alpha/true", first)
	}
	if second["agent_id"] != "beta" || second["active"] != false {
		t.Errorf("members[1] = %v，期望 beta/false", second)
	}

	var errBody map[string]any
	getJSON(t, env.http.URL+"/api/conversations/no-such-id", http.StatusNotFound, &errBody)
	requireKeys(t, "404", errBody, "error")
}

func TestServerMessages(t *testing.T) {
	env := newServeTestEnv(t, nil)
	conv := env.createConversation(t, "消息")
	// 前三条用 user，后两条用带脑子的 agent，顺带验证 sender_id / kind 原样透出。
	env.message(t, conv.ID, conversation.ReservedSenderUser, "一")
	env.message(t, conv.ID, conversation.ReservedSenderUser, "二")
	env.message(t, conv.ID, conversation.ReservedSenderUser, "三")
	env.message(t, conv.ID, conversation.ReservedSenderUser, "四")

	var page map[string]any
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/messages?after=2&limit=2",
		http.StatusOK, &page)
	requireKeys(t, "messages", page, "last_seq", "messages")
	list, _ := page["messages"].([]any)
	if len(list) != 2 {
		t.Fatalf("messages 有 %d 条，期望 2（limit 截断）", len(list))
	}
	if got := page["last_seq"]; got != float64(4) {
		t.Errorf("last_seq = %v，期望 4（这一页最后一条）", got)
	}
	for i, raw := range list {
		row, _ := raw.(map[string]any)
		requireKeys(t, "message", row, "created_at", "kind", "sender_id", "seq", "text")
		if row["seq"] != float64(i+3) {
			t.Errorf("messages[%d].seq = %v，期望 %d", i, row["seq"], i+3)
		}
		if row["sender_id"] != conversation.ReservedSenderUser {
			t.Errorf("messages[%d].sender_id = %v", i, row["sender_id"])
		}
		if row["kind"] != string(conversation.KindUser) {
			t.Errorf("messages[%d].kind = %v", i, row["kind"])
		}
	}

	// 默认 limit 200，五条以内一次读完；after 超过最大 seq 回显 after。
	env.message(t, conv.ID, conversation.ReservedSenderUser, "五")
	var all map[string]any
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/messages", http.StatusOK, &all)
	if got := all["last_seq"]; got != float64(5) {
		t.Errorf("不传参数的 last_seq = %v，期望 5", got)
	}
	var empty map[string]any
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/messages?after=99", http.StatusOK, &empty)
	msgs, _ := empty["messages"].([]any)
	if len(msgs) != 0 {
		t.Errorf("after=99 读回 %d 条，期望 0", len(msgs))
	}
	if got := empty["last_seq"]; got != float64(99) {
		t.Errorf("空页的 last_seq = %v，期望回显 after=99", got)
	}

	for _, query := range []string{
		"after=-1",
		"after=abc",
		"after=",
		"limit=0",
		"limit=1001",
		"limit=abc",
		"limit=",
	} {
		var errBody map[string]any
		getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/messages?"+query,
			http.StatusBadRequest, &errBody)
		if _, ok := errBody["error"]; !ok {
			t.Errorf("非法查询 %q 的 400 响应缺少 error 字段: %v", query, errBody)
		}
	}

	var errBody map[string]any
	getJSON(t, env.http.URL+"/api/conversations/no-such-id/messages",
		http.StatusNotFound, &errBody)
	if _, ok := errBody["error"]; !ok {
		t.Errorf("未知会话的 404 响应缺少 error 字段: %v", errBody)
	}
}

func TestServerUsage(t *testing.T) {
	models := map[string]string{"alpha": "opus", "beta": "grok-4"}
	env := newServeTestEnv(t, models)
	conv := env.createConversation(t, "账")
	env.join(t, conv.ID, "beta", "Beta", "grok")
	env.join(t, conv.ID, "alpha", "Alpha", "claude")

	env.usage(t, conversation.TurnUsage{
		TurnID:       "turn-1",
		ConvID:       conv.ID,
		AgentID:      "alpha",
		Outcome:      1,
		InputTokens:  intPtr(100),
		CacheRead:    intPtr(10),
		OutputTokens: intPtr(20),
		CostUSD:      floatPtr(0.01),
	})
	env.usage(t, conversation.TurnUsage{
		TurnID:       "turn-2",
		ConvID:       conv.ID,
		AgentID:      "alpha",
		Outcome:      1,
		InputTokens:  intPtr(50),
		CacheCreate:  intPtr(5),
		CacheRead:    intPtr(0),
		OutputTokens: intPtr(5),
		CostUSD:      floatPtr(0.02),
	})
	// grok 不报金额：cost_usd 为 nil，不能当 0 加进合计。
	env.usage(t, conversation.TurnUsage{
		TurnID:       "turn-3",
		ConvID:       conv.ID,
		AgentID:      "beta",
		Outcome:      1,
		InputTokens:  intPtr(7),
		OutputTokens: intPtr(3),
	})

	var got map[string]any
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/usage", http.StatusOK, &got)
	requireKeys(t, "usage", got, "complete", "cost_usd", "per_agent")
	if got["complete"] != false {
		t.Errorf("有 grok 成员时 complete = %v，期望 false", got["complete"])
	}
	if cost, _ := got["cost_usd"].(float64); math.Abs(cost-0.03) > 1e-9 {
		t.Errorf("cost_usd = %v，期望 0.03", got["cost_usd"])
	}

	rows, _ := got["per_agent"].([]any)
	if len(rows) != 2 {
		t.Fatalf("per_agent 有 %d 行，期望 2", len(rows))
	}
	alpha, _ := rows[0].(map[string]any)
	beta, _ := rows[1].(map[string]any)
	requireKeys(t, "agent usage", alpha,
		"agent_id", "cache_create", "cache_read", "cost_reported", "cost_usd", "in", "model", "out", "provider", "turns")
	if alpha["cost_reported"] != true {
		t.Errorf("claude 会报金额，cost_reported 应为 true，实际 %v", alpha["cost_reported"])
	}
	if beta["cost_reported"] != false {
		t.Errorf("grok 不报金额，cost_reported 应为 false，实际 %v", beta["cost_reported"])
	}
	if alpha["agent_id"] != "alpha" || alpha["provider"] != "claude" || alpha["model"] != "opus" {
		t.Errorf("alpha 行 = %v", alpha)
	}
	if alpha["turns"] != float64(2) ||
		alpha["in"] != float64(150) ||
		alpha["cache_create"] != float64(5) ||
		alpha["cache_read"] != float64(10) ||
		alpha["out"] != float64(25) {
		t.Errorf("alpha 的用量 = %v，期望 turns=2 in=150 cache_create=5 cache_read=10 out=25", alpha)
	}
	if cost, _ := alpha["cost_usd"].(float64); math.Abs(cost-0.03) > 1e-9 {
		t.Errorf("alpha.cost_usd = %v，期望 0.03", alpha["cost_usd"])
	}
	if beta["agent_id"] != "beta" || beta["turns"] != float64(1) || beta["in"] != float64(7) {
		t.Errorf("beta 行 = %v", beta)
	}
	if cost, _ := beta["cost_usd"].(float64); cost != 0 {
		t.Errorf("beta.cost_usd = %v，期望 0（没报金额）", beta["cost_usd"])
	}

	// 全是 claude 且报过金额时 complete 为 true。
	gammaConv := env.createConversation(t, "全是 claude")
	env.join(t, gammaConv.ID, "alpha", "Alpha", "claude")
	env.usage(t, conversation.TurnUsage{
		TurnID:      "turn-4",
		ConvID:      gammaConv.ID,
		AgentID:     "alpha",
		Outcome:     1,
		InputTokens: intPtr(1),
		CostUSD:     floatPtr(0.5),
	})
	var complete map[string]any
	getJSON(t, env.http.URL+"/api/conversations/"+gammaConv.ID+"/usage", http.StatusOK, &complete)
	if complete["complete"] != true {
		t.Errorf("全是 claude 时 complete = %v，期望 true", complete["complete"])
	}
	if cost, _ := complete["cost_usd"].(float64); math.Abs(cost-0.5) > 1e-9 {
		t.Errorf("cost_usd = %v，期望 0.5", complete["cost_usd"])
	}

	var errBody map[string]any
	getJSON(t, env.http.URL+"/api/conversations/no-such-id/usage",
		http.StatusNotFound, &errBody)
	if _, ok := errBody["error"]; !ok {
		t.Errorf("未知会话的 404 响应缺少 error 字段: %v", errBody)
	}
}

func TestServerIndexAndEmbed(t *testing.T) {
	if len(indexHTML) == 0 {
		t.Fatal("内嵌的 index.html 是空的")
	}
	if !strings.Contains(string(indexHTML), `id="app"`) {
		t.Fatal(`内嵌的 index.html 里没有 id="app"，embed 路径多半写错了`)
	}
	if !strings.Contains(string(indexHTML), "还没有会话，先在终端里跑一次 council") {
		t.Error("index.html 里缺少空库提示语")
	}

	env := newServeTestEnv(t, nil)
	res, err := http.Get(env.http.URL + "/")
	if err != nil {
		t.Fatalf("GET / 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d，期望 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("GET / 的 Content-Type = %q", got)
	}
	var body bytes.Buffer
	if _, err := body.ReadFrom(res.Body); err != nil {
		t.Fatalf("读 / 的响应失败: %v", err)
	}
	if !strings.Contains(body.String(), `id="app"`) {
		t.Error("GET / 返回的页面里没有 id=\"app\"")
	}
}

func TestServerUnknownAPIIsJSON404(t *testing.T) {
	env := newServeTestEnv(t, nil)
	var got map[string]any
	getJSON(t, env.http.URL+"/api/nope", http.StatusNotFound, &got)
	requireKeys(t, "404", got, "error")
}

func TestServerMethodNotAllowed(t *testing.T) {
	env := newServeTestEnv(t, nil)
	res, err := http.Post(env.http.URL+"/api/health", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST /api/health 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/health = %d，期望 405", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("405 的 Content-Type = %q", got)
	}
	if got := res.Header.Get("Allow"); got != "GET" {
		t.Errorf("405 的 Allow = %q，期望 GET", got)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("405 响应不是合法 JSON: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("405 响应缺少 error 字段: %v", body)
	}
}

func TestServerVerboseLogsOneLinePerRequest(t *testing.T) {
	env := newServeTestEnv(t, nil)
	var log bytes.Buffer
	// Verbose 与 Log 必须在 Handler() 之前设好：Handler 在构造时就把
	// 是否包日志中间件这件事定下来了。
	srv := NewServer(env.store, "test-version", nil)
	srv.Verbose = true
	srv.Log = &log
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	getJSON(t, ts.URL+"/api/health", http.StatusOK, nil)
	line := log.String()
	if !strings.Contains(line, "GET /api/health 200") {
		t.Errorf("verbose 日志里没有这一行请求: %q", line)
	}
	if got := strings.Count(strings.TrimSpace(line), "\n"); got > 0 {
		t.Errorf("一次请求应当只写一行，实际:\n%s", line)
	}
}

func TestParse(t *testing.T) {
	t.Run("默认值", func(t *testing.T) {
		opts, err := Parse(nil)
		if err != nil {
			t.Fatalf("Parse(nil) 失败: %v", err)
		}
		if opts.Addr != defaultAddr {
			t.Errorf("默认 -addr = %q，期望 %q", opts.Addr, defaultAddr)
		}
		if opts.Open || opts.Verbose || opts.ConfigPath != "" {
			t.Errorf("默认值不对: %+v", opts)
		}
	})

	t.Run("回环地址都接受", func(t *testing.T) {
		for _, addr := range []string{
			"127.0.0.1:8765",
			"127.9.9.9:1",
			"localhost:0",
			"[::1]:0",
		} {
			if _, err := Parse([]string{"-addr", addr}); err != nil {
				t.Errorf("Parse(-addr %q) 期望成功，实际: %v", addr, err)
			}
		}
	})

	t.Run("非回环地址退 2 语义", func(t *testing.T) {
		for _, addr := range []string{
			"0.0.0.0:8765",
			":8765",
			"192.168.1.2:8765",
			"[::]:8765",
			"example.com:80",
			"127.0.0.1",
			"127.0.0.1:abc",
			"127.0.0.1:70000",
		} {
			_, err := Parse([]string{"-addr", addr})
			if err == nil {
				t.Errorf("Parse(-addr %q) 期望报错，实际成功", addr)
				continue
			}
			if !strings.Contains(err.Error(), "回环") && !strings.Contains(err.Error(), "端口") &&
				!strings.Contains(err.Error(), "不合法") {
				t.Errorf("Parse(-addr %q) 的错误没有说明原因: %v", addr, err)
			}
		}
	})

	t.Run("私有 flag 全部登记", func(t *testing.T) {
		opts, err := Parse([]string{"-open", "-verbose", "-config", "x.toml", "-addr", "localhost:1234"})
		if err != nil {
			t.Fatalf("Parse 失败: %v", err)
		}
		if !opts.Open || !opts.Verbose || opts.ConfigPath != "x.toml" || opts.Addr != "localhost:1234" {
			t.Errorf("解析结果不对: %+v", opts)
		}
	})

	t.Run("-h 返回 flag.ErrHelp", func(t *testing.T) {
		_, err := Parse([]string{"-h"})
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("Parse(-h) = %v，期望 flag.ErrHelp", err)
		}
	})

	t.Run("未知 flag 与位置参数报错", func(t *testing.T) {
		if _, err := Parse([]string{"-nope"}); err == nil {
			t.Error("未知 flag 期望报错")
		}
		if _, err := Parse([]string{"extra"}); err == nil {
			t.Error("位置参数期望报错")
		}
	})
}

// 让 Usage 的输出也接受一次最小检查：改了 flag 名而没改用法时至少破一条。
func TestUsageListsFlags(t *testing.T) {
	usage := Usage()
	for _, want := range []string{"用法: council serve", "-addr", "-open", "-config", "-verbose", defaultAddr} {
		if !strings.Contains(usage, want) {
			t.Errorf("Usage() 里缺少 %q:\n%s", want, usage)
		}
	}
}

// TestResolveDataDirFromConfig 钉住 -config 与全局同义：配置里的 data_dir
// 与 [agents.*].model 会被带出来；显式指了不存在的配置文件则报错，不回落。
func TestResolveDataDirFromConfig(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := filepath.Join(dir, "config.toml")
	body := fmt.Sprintf(`data_dir = '%s'

[providers.claude]
command = 'c'

[agents.alpha]
display_name = "Alpha"
provider = "claude"
model = "opus"
permissions = "read"
mode = "discussion"
`, dataDir)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}

	gotDir, models, _, err := resolveDataDir(cfgPath)
	if err != nil {
		t.Fatalf("resolveDataDir 失败: %v", err)
	}
	if gotDir != dataDir {
		t.Errorf("data_dir = %q，期望 %q", gotDir, dataDir)
	}
	if models["alpha"] != "opus" {
		t.Errorf("agents.alpha.model = %q，期望 opus（map=%v）", models["alpha"], models)
	}

	if _, _, _, err := resolveDataDir(filepath.Join(dir, "missing.toml")); err == nil {
		t.Error("显式 -config 指向不存在的文件时期望报错")
	}
}

// syncBuffer 是进程边界测试用的 stdout：Run 在 goroutine 里写，测试在
// 主 goroutine 里等「第一行已经写出来」再取消 ctx，避免靠 sleep 猜启动时刻。
type syncBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	once  sync.Once
	wrote chan struct{}
}

func newSyncBuffer() *syncBuffer {
	return &syncBuffer{wrote: make(chan struct{})}
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	n, err := b.buf.Write(p)
	b.mu.Unlock()
	b.once.Do(func() { close(b.wrote) })
	return n, err
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestRunStartupLineAndShutdown 真的起一次 Run：用 127.0.0.1:0 让系统分端口，
// 打印启动行之后取消 ctx，断言 Run 走 Shutdown 并返回 nil。
// 不只测 Handler：任务书 §3 的「启动打印一行」与 Ctrl-C 优雅关闭都在这条路上。
func TestRunStartupLineAndShutdown(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	st, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("准备测试库失败: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("准备测试库失败: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("data_dir = '"+dataDir+"'\n"), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newSyncBuffer()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Addr: "127.0.0.1:0", ConfigPath: cfgPath, out: out}, "test-version")
	}()

	select {
	case <-out.wrote:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("5 秒内没有打印启动行")
	}
	if got := out.String(); !strings.Contains(got, "council serve: http://127.0.0.1:") {
		t.Errorf("启动行不对: %q", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("取消 ctx 之后 Run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取消 ctx 之后 Run 超过 5 秒没有返回，Shutdown 上限失效")
	}
}
