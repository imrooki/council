// jobs_current_test.go 钉 10-26 这一轮的「页面重开之后把进度面板接回来」：
//
//   - GET /api/jobs/current 在没有轮次时回 {"job":null}；
//   - 有轮次时回与 POST /api/ask 202 同一组字段（job_id / conversation_id /
//     started_at / mode / rounds / members / summary / budget_usd / permission）；
//   - 轮次结束后立刻回 null（不等 TTL 清理）；
//   - SSE 在没有 Last-Event-ID 时从头重放，带了就从该 id 之后续传——这两条
//     是前端敢在 reload 之后直接重挂、而不是从零猜进度的前提。
//
// 浏览器侧的重挂行为由 CDP 验收覆盖（刷新页面后有在跑的轮次时进度面板
// 不是空的）；这里钉的是重挂赖以成立的服务端契约。
package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// jobsCurrentBody 是 /api/jobs/current 的原始响应。Job 用 RawMessage 是为了
// 分辨「显式 null」与「缺少字段」——两者对前端不是一回事。
type jobsCurrentBody struct {
	Job json.RawMessage `json:"job"`
}

func (b jobsCurrentBody) isNull() bool {
	raw := strings.TrimSpace(string(b.Job))
	return raw == "" || raw == "null"
}

// createAskConversation 在写模式测试库里建一场会话，供续接用例使用。
func createAskConversation(t *testing.T, env *writeTestEnv) conversation.Conversation {
	t.Helper()
	ctx := context.Background()
	var conv conversation.Conversation
	if err := env.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, "重挂进度的会话")
		return err
	}); err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	return conv
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// newSSEReaderWithLastEventID 与 newSSEReader 相同，只多带一个
// Last-Event-ID 头：浏览器自动重连时就是靠它续传，而不是从头重放。
func newSSEReaderWithLastEventID(t *testing.T, ctx context.Context, url, lastID string) *sseReader {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("构造 SSE 请求失败: %v", err)
	}
	req.Header.Set("Last-Event-ID", lastID)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("连接 SSE %s 失败: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("SSE %s = %d，期望 200；body=%s", url, res.StatusCode, raw)
	}
	return &sseReader{resp: res, in: bufio.NewReader(res.Body)}
}

// waitJobEventCount 等 job 的事件表至少攒到 n 帧。SSE 重放用例靠它制造
// 「订阅之前已经有历史」这个真实场景（页面 reload 时正是这样）。
func waitJobEventCount(t *testing.T, j *job, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		j.mu.Lock()
		got := len(j.events)
		j.mu.Unlock()
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("10 秒内只等到 %d 帧事件，期望至少 %d 帧", got, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestJobsCurrentReadOnly405 与 /events、/cancel 同一个门：只读模式回 405
// 并说明原因，不是落进 /api/ 兜底的 404。
func TestJobsCurrentReadOnly405(t *testing.T) {
	env := newServeTestEnv(t, nil)

	res, err := http.Get(env.http.URL + "/api/jobs/current")
	if err != nil {
		t.Fatalf("GET /api/jobs/current 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式 GET /api/jobs/current = %d，期望 405", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("405 的 Content-Type = %q，期望 JSON", got)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("405 响应不是合法 JSON: %v", err)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "-rw") {
		t.Errorf("405 的说明没有告诉用户用 -rw：%q", msg)
	}
}

// TestJobsCurrentNullWhenIdle 是「没有在跑的轮次」这一档：显式 null。
func TestJobsCurrentNullWhenIdle(t *testing.T) {
	env := newWriteTestEnv(t)

	var body jobsCurrentBody
	getJSON(t, env.http.URL+"/api/jobs/current", http.StatusOK, &body)
	if !body.isNull() {
		t.Fatalf("空闲时 /api/jobs/current 的 job = %s，期望 null", body.Job)
	}
}

// TestJobsCurrentReportsRunningJob 覆盖续接场景下的完整字段：
// 202 与 /current 必须是同一组事实，轮次结束后 /current 立刻变回 null。
func TestJobsCurrentReportsRunningJob(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	// 一次 provider 调用慢 1.2 秒：202 回来之后到轮次结束之间有一段稳定的
	// 观察窗口，「在跑」这件事才验得准。
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "1200")
	env := newWriteTestEnv(t)
	conv := createAskConversation(t, env)

	status, ctype, raw := postRaw(t, env.http.URL+"/api/ask", "application/json",
		fmt.Sprintf(`{"prompt":"接着上一轮","members":["c1"],"rounds":1,`+
			`"conversation":%q,"budget_usd":2.5,"permission":"read"}`, conv.ID))
	if status != http.StatusAccepted {
		t.Fatalf("POST /api/ask = %d，期望 202；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("202 的 Content-Type = %q", ctype)
	}
	var accepted askResponse
	if err := json.Unmarshal(raw, &accepted); err != nil {
		t.Fatalf("202 响应不是合法 JSON: %v；body=%s", err, raw)
	}
	if accepted.JobID == "" {
		t.Fatalf("202 响应缺少 job_id: %s", raw)
	}
	if accepted.ConversationID != conv.ID {
		t.Errorf("202 的 conversation_id = %q，期望续接的 %q", accepted.ConversationID, conv.ID)
	}
	if accepted.Status != "running" {
		t.Errorf("202 的 status = %q，期望 running", accepted.Status)
	}
	if accepted.StartedAt == "" {
		t.Error("202 缺少 started_at")
	}

	var body jobsCurrentBody
	getJSON(t, env.http.URL+"/api/jobs/current", http.StatusOK, &body)
	if body.isNull() {
		t.Fatal("轮次在跑时 /api/jobs/current 的 job 是 null")
	}
	var job askResponse
	if err := json.Unmarshal(body.Job, &job); err != nil {
		t.Fatalf("job 不是合法 JSON: %v；body=%s", err, body.Job)
	}
	if job.JobID != accepted.JobID {
		t.Errorf("/api/jobs/current 的 job_id = %q，期望与 202 同为 %q", job.JobID, accepted.JobID)
	}
	if job.ConversationID != conv.ID {
		t.Errorf("/api/jobs/current 的 conversation_id = %q，期望 %q", job.ConversationID, conv.ID)
	}
	if job.Mode != "ask" || job.Rounds != 1 {
		t.Errorf("/api/jobs/current 的 mode/rounds = %q/%d，期望 ask/1", job.Mode, job.Rounds)
	}
	if len(job.Members) != 1 || job.Members[0] != "c1" {
		t.Errorf("/api/jobs/current 的 members = %v，期望 [c1]", job.Members)
	}
	if job.BudgetUSD != 2.5 {
		t.Errorf("/api/jobs/current 的 budget_usd = %v，期望 2.5", job.BudgetUSD)
	}
	if job.Permission != "read" {
		t.Errorf("/api/jobs/current 的 permission = %q，期望 read", job.Permission)
	}
	startedAt, err := time.Parse(time.RFC3339, job.StartedAt)
	if err != nil {
		t.Fatalf("started_at = %q 不是 RFC3339: %v", job.StartedAt, err)
	}
	if d := time.Since(startedAt); d < 0 || d > time.Minute {
		t.Errorf("started_at 看起来不是刚刚：%v（now-start=%v）", job.StartedAt, d)
	}

	// 等轮次结束：/current 必须立刻变回 null，而不是等 60 秒 TTL 把它从
	// 表里清掉——前端重挂时不能接上一个已经结束的任务。
	if got := waitJobStatus(t, env, accepted.JobID); got != "ok" {
		t.Fatalf("轮次最终状态 = %q，期望 ok", got)
	}
	var after jobsCurrentBody
	getJSON(t, env.http.URL+"/api/jobs/current", http.StatusOK, &after)
	if !after.isNull() {
		t.Errorf("轮次结束后 /api/jobs/current 的 job = %s，期望 null", after.Job)
	}
}

// TestJobsCurrentFillsNewConversationIDOnStart 覆盖新建会话：202 那一刻还
// 不知道 id，OnStart 之后 /current 必须能给出同一个 id，前端重挂才不会
// 落到一个空的「新建会话」面板上。
func TestJobsCurrentFillsNewConversationIDOnStart(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "800")
	env := newWriteTestEnv(t)

	jobID := postAsk(t, env, map[string]any{
		"prompt":  "新建会话也会被重挂",
		"members": []string{"c1"},
		"rounds":  1,
	})

	deadline := time.Now().Add(10 * time.Second)
	var convID string
	for convID == "" {
		var body jobsCurrentBody
		getJSON(t, env.http.URL+"/api/jobs/current", http.StatusOK, &body)
		if body.isNull() {
			t.Fatalf("任务 %s 还在跑，/api/jobs/current 却回了 null", jobID)
		}
		var job askResponse
		if err := json.Unmarshal(body.Job, &job); err != nil {
			t.Fatalf("job 不是合法 JSON: %v；body=%s", err, body.Job)
		}
		convID = job.ConversationID
		if convID != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("10 秒内 /api/jobs/current 一直没有给出新建会话的 conversation_id")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := waitJobStatus(t, env, jobID); got != "ok" {
		t.Fatalf("轮次最终状态 = %q，期望 ok", got)
	}
	// 轮次结束后库里确实有这场会话，说明 /current 给的不是一个临时值。
	if _, err := env.store.Conversation(context.Background(), convID); err != nil {
		t.Errorf("conversation_id %q 在库里读不到: %v", convID, err)
	}
}

// TestJobEventsReplayFromStartForReattach 是重挂的核心：订阅发生在事件已经
// 产生之后（页面 reload 的真实时序），服务端仍然从第 1 帧开始重放，前端
// 因此能把整块进度面板按原顺序重建。
func TestJobEventsReplayFromStartForReattach(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "900")
	env := newWriteTestEnv(t)

	jobID := postAsk(t, env, map[string]any{
		"prompt":  "重挂要从头放",
		"members": []string{"c1"},
		"rounds":  1,
	})
	j, ok := env.srv.jobs.get(jobID)
	if !ok {
		t.Fatalf("任务 %s 不在表里", jobID)
	}
	// 等到订阅之前就已经有三帧历史（started + turn_started + 至少一条
	// turn）：这时新建的 EventSource 没有 Last-Event-ID，必须从头放。
	waitJobEventCount(t, j, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := newSSEReader(t, ctx, env.http.URL+"/api/jobs/"+jobID+"/events")
	defer reader.close()

	first, ok := reader.next(t)
	if !ok {
		t.Fatal("重挂之后 SSE 立刻结束")
	}
	if first.ID != 1 || first.Event != "started" {
		t.Fatalf("重挂的第一帧 = id %d / %s，期望 id 1 / started（从头重放）", first.ID, first.Event)
	}
	frames := reader.readUntil(t, "done")
	// 第一帧已经单独断言过，这里把它一起装进事件名列表再比对整条时间线。
	names := make([]string, 0, len(frames)+1)
	names = append(names, first.Event)
	for _, frame := range frames {
		names = append(names, frame.Event)
	}
	for _, want := range []string{"started", "turn_started", "turn", "done"} {
		if !containsString(names, want) {
			t.Errorf("重放里缺少 %q 事件；实际 %v", want, names)
		}
	}
}

// TestJobEventsResumeFromLastEventID 是同一套历史的另一半：浏览器断线
// 自动重连会带 Last-Event-ID，服务端从那之后续传，已经显示过的帧不重复。
func TestJobEventsResumeFromLastEventID(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "900")
	env := newWriteTestEnv(t)

	jobID := postAsk(t, env, map[string]any{
		"prompt":  "断线续传",
		"members": []string{"c1"},
		"rounds":  1,
	})
	j, ok := env.srv.jobs.get(jobID)
	if !ok {
		t.Fatalf("任务 %s 不在表里", jobID)
	}
	waitJobEventCount(t, j, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := newSSEReaderWithLastEventID(t, ctx,
		env.http.URL+"/api/jobs/"+jobID+"/events", "2")
	defer reader.close()

	next, ok := reader.next(t)
	if !ok {
		t.Fatal("带 Last-Event-ID 的 SSE 立刻结束")
	}
	if next.ID != 3 {
		t.Fatalf("Last-Event-ID: 2 之后的第一帧 id = %d，期望 3", next.ID)
	}
}
