package serve

// srv12_followup_test.go 覆盖 GUI 路线第 1 片「在已有会话里继续追问」：
//   - POST /api/ask 的 conversation 非空时续接同一个 convID，空串仍新建；
//   - 未知 conversation → 400，不能先收 202 再在 SSE 上看错；
//   - 只读模式行为不变：写接口 405；
//   - 新建与续接两条路径的 OnStart 都把正确的 convID 交给 serve（体现在
//     started 事件上）；
//   - 前端的目标行 / 提交载荷 / 续接后的增量刷新有静态锚点，innerHTML
//     仍由 srv11 的全局用例守着 0 处。
//
// 真浏览器行为不在这里：CDP 真跑一次的验收见交付报告。

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/headless"
)

// followupWaitJob 等任务结束，并取回 OnStart 发出的 convID 与最终 Result。
// started 事件就是 headless.Deps.OnStart 的产物（write.go 的 runAskJob 把
// 它直接映射成 emit("started", ...)），所以它等于「OnStart 收到了什么」。
func followupWaitJob(t *testing.T, env *writeTestEnv, jobID string) (string, headless.Result) {
	t.Helper()
	j, ok := env.srv.jobs.get(jobID)
	if !ok {
		t.Fatalf("任务 %s 不在表里", jobID)
	}
	select {
	case <-j.done:
	case <-time.After(15 * time.Second):
		t.Fatalf("任务 %s 15 秒内没有结束", jobID)
	}
	res, err := j.outcome()
	if err != nil {
		t.Fatalf("任务 %s 失败: %v", jobID, err)
	}

	startedID := ""
	for i := 0; ; i++ {
		ev, ok, _, _ := j.next(i)
		if !ok {
			break
		}
		if ev.Name != "started" {
			continue
		}
		started, ok := ev.Data.(startedEvent)
		if !ok {
			t.Fatalf("started 事件的载荷类型 = %T，期望 startedEvent", ev.Data)
		}
		startedID = started.ConversationID
		break
	}
	if startedID == "" {
		t.Fatalf("任务 %s 没有带 convID 的 started 事件：OnStart 没有被调用或参数为空", jobID)
	}
	return startedID, res
}

// followupSeed 先用一次普通 ask 建一个真实存在的会话，作为续接目标。
func followupSeed(t *testing.T, env *writeTestEnv, prompt string) (string, string) {
	t.Helper()
	jobID := postAsk(t, env, map[string]any{
		"prompt":  prompt,
		"members": []string{"c1"},
		"rounds":  1,
	})
	convID, res := followupWaitJob(t, env, jobID)
	if res.Status != "ok" {
		t.Fatalf("建种子会话的 status = %q，期望 ok", res.Status)
	}
	if res.ConversationID != convID {
		t.Fatalf("result.conversation_id = %q，started = %q，两者应当相同", res.ConversationID, convID)
	}
	return jobID, convID
}

// TestAskConversationContinuesExistingConversation 钉住核心行为：
// conversation 非空时第二轮落在同一个 convID 上，问题与回复都在那里，
// 标题不被改写，会话总数不增加。
func TestAskConversationContinuesExistingConversation(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newWriteTestEnv(t)
	_, convID := followupSeed(t, env, "第一问")

	ctx := context.Background()
	before, err := env.store.Messages(ctx, convID, 0, 0)
	if err != nil {
		t.Fatalf("读第一轮消息失败: %v", err)
	}
	convBefore, err := env.store.Conversation(ctx, convID)
	if err != nil {
		t.Fatalf("读第一轮会话失败: %v", err)
	}
	if convBefore.Title != "第一问" {
		t.Fatalf("新建会话的标题 = %q，期望取问题正文「第一问」", convBefore.Title)
	}

	secondJob := postAsk(t, env, map[string]any{
		"prompt":       "追问",
		"members":      []string{"c1"},
		"rounds":       1,
		"conversation": convID,
	})
	secondConvID, res := followupWaitJob(t, env, secondJob)
	if secondConvID != convID {
		t.Errorf("续接时 OnStart 的 convID = %q，期望 %q", secondConvID, convID)
	}
	if res.ConversationID != convID {
		t.Errorf("续接时 result.conversation_id = %q，期望 %q", res.ConversationID, convID)
	}
	if res.Status != "ok" {
		t.Errorf("续接的一轮 status = %q，期望 ok", res.Status)
	}

	after, err := env.store.Messages(ctx, convID, 0, 0)
	if err != nil {
		t.Fatalf("读续接后的消息失败: %v", err)
	}
	if len(after) <= len(before) {
		t.Fatalf("续接后消息数 = %d，第一轮 = %d；续接应当追加消息而不是另开一份", len(after), len(before))
	}
	for _, m := range after {
		if m.ConvID != convID {
			t.Errorf("消息 seq=%d 落在会话 %q，期望全部落在 %q", m.Seq, m.ConvID, convID)
		}
	}

	prompts := make([]string, 0, 2)
	replies := 0
	for _, m := range after {
		if m.Kind == conversation.KindUser {
			prompts = append(prompts, m.Text)
		}
		if m.Kind == conversation.KindAgent && m.SenderID == "c1" && m.Text == "OK" {
			replies++
		}
	}
	joined := strings.Join(prompts, "\n")
	for _, want := range []string{"第一问", "追问"} {
		if !strings.Contains(joined, want) {
			t.Errorf("同一个会话里找不到问题 %q；实际问题: %q", want, joined)
		}
	}
	if replies < 2 {
		t.Errorf("同一个会话里 c1 的回复 = %d 条，期望两轮各一条", replies)
	}

	convAfter, err := env.store.Conversation(ctx, convID)
	if err != nil {
		t.Fatalf("读续接后的会话失败: %v", err)
	}
	if convAfter.Title != convBefore.Title {
		t.Errorf("续接改写了标题：%q -> %q；标题逻辑只应在新建时生效", convBefore.Title, convAfter.Title)
	}

	convs, err := env.store.Conversations(ctx)
	if err != nil {
		t.Fatalf("读会话列表失败: %v", err)
	}
	if len(convs) != 1 {
		t.Fatalf("续接后会话数 = %d，期望 1（不能另开一个新会话）", len(convs))
	}
	if convs[0].ID != convID {
		t.Errorf("唯一的会话是 %q，期望 %q", convs[0].ID, convID)
	}
}

// TestAskOnStartConversationIDNewAndContinue 单独钉 OnStart 的两条路径：
// 新建时它必须报新建出来的 id，续接时必须报请求里给的那个 id。started
// 事件是 OnStart 的唯一映射点（runAskJob 里 emit("started", ...)）。
func TestAskOnStartConversationIDNewAndContinue(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newWriteTestEnv(t)

	newJob := postAsk(t, env, map[string]any{
		"prompt":  "新建路径",
		"members": []string{"c1"},
		"rounds":  1,
	})
	newConvID, newRes := followupWaitJob(t, env, newJob)
	if newConvID != newRes.ConversationID {
		t.Errorf("新建时 OnStart = %q，Result.ConversationID = %q", newConvID, newRes.ConversationID)
	}

	continueJob := postAsk(t, env, map[string]any{
		"prompt":       "续接路径",
		"members":      []string{"c1"},
		"rounds":       1,
		"conversation": newConvID,
	})
	continueConvID, continueRes := followupWaitJob(t, env, continueJob)
	if continueConvID != newConvID {
		t.Errorf("续接时 OnStart = %q，期望请求里的 %q", continueConvID, newConvID)
	}
	if continueRes.ConversationID != newConvID {
		t.Errorf("续接时 Result.ConversationID = %q，期望 %q", continueRes.ConversationID, newConvID)
	}
}

// TestAskConversationUnknownIs400 钉未知 id 的请求阶段拒绝：400 + 说清
// id 与出路，且不留下任何任务或新会话。
func TestAskConversationUnknownIs400(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newWriteTestEnv(t)

	const missing = "no-such-conversation"
	body, err := json.Marshal(map[string]any{
		"prompt":       "续接不存在的会话",
		"members":      []string{"c1"},
		"rounds":       1,
		"conversation": missing,
	})
	if err != nil {
		t.Fatalf("编码请求失败: %v", err)
	}
	status, ctype, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
	if status != http.StatusBadRequest {
		t.Fatalf("未知 conversation = %d，期望 400；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("400 的 Content-Type = %q", ctype)
	}
	var reply struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("400 响应不是合法 JSON: %v；%s", err, raw)
	}
	for _, want := range []string{"会话", "不存在", missing, "新建"} {
		if !strings.Contains(reply.Error, want) {
			t.Errorf("400 的 error = %q，期望包含 %q", reply.Error, want)
		}
	}

	env.srv.jobs.mu.Lock()
	jobCount := len(env.srv.jobs.jobs)
	env.srv.jobs.mu.Unlock()
	if jobCount != 0 {
		t.Errorf("被拒的请求建出了 %d 个任务，期望 0", jobCount)
	}
	convs, err := env.store.Conversations(context.Background())
	if err != nil {
		t.Fatalf("读会话列表失败: %v", err)
	}
	if len(convs) != 0 {
		t.Errorf("被拒的请求留下了 %d 个会话，期望 0", len(convs))
	}
}

// TestAskConversationReadOnly405 钉只读模式不变：带 conversation 的
// POST /api/ask 仍然先撞 s.write == nil，回 405 + 用 -rw 的说明。
func TestAskConversationReadOnly405(t *testing.T) {
	env := newServeTestEnv(t, nil)
	body := `{"prompt":"x","conversation":"whatever"}`
	status, ctype, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", body)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式带 conversation 的 POST = %d，期望 405；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("405 的 Content-Type = %q", ctype)
	}
	var reply struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("405 响应不是合法 JSON: %v；%s", err, raw)
	}
	if !strings.Contains(reply.Error, "-rw") {
		t.Errorf("405 的说明没有告诉用户用 -rw: %q", reply.Error)
	}
}

// TestIndexFollowUpTargetMarkup 静态钉前端锚点：目标行在输入行上方、
// 两种文案都存在、提交时按 state.currentId 决定是否带 conversation、
// 续接分支不重置 lastSeq。真实 DOM 行为由 CDP 验收覆盖。
func TestIndexFollowUpTargetMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		`id="ask-target"`,
		`id="ask-target-text"`,
		`id="ask-target-new"`,
		`t('ui.composer.continueConversation', '继续「{title}」'`,
		`t('ui.composer.newConversation', '新建会话')`,
		`const targetId = state.currentId`,
		`if (targetId) payload.conversation = targetId`,
		`const alreadyShowing = state.currentId === data.conversation_id`,
		`selectConversation(data.conversation_id)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}

	target := strings.Index(html, `id="ask-target"`)
	askRow := strings.Index(html, `<div class="ask-row">`)
	if target < 0 || askRow < 0 {
		t.Fatalf("找不到目标行或输入行：target=%d askRow=%d", target, askRow)
	}
	if target > askRow {
		t.Errorf("目标行（offset %d）必须在输入行（offset %d）之前", target, askRow)
	}
}
