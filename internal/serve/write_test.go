package serve

// write_test.go 覆盖 SRV4 §6 的写模式：锁冲突、405、400、fake provider 的
// SSE 端到端、客户端断开不取消议会、未知 job / TTL 清理。
//
// fake provider 的形状照 internal/app/smoke_test.go：测试二进制在
// COUNCIL_FAKE_CLI=1 时改行扮演 claude CLI，配置里的 command 写
// os.Args[0]。这样整条链（serve -> headless -> turn -> spawn -> 解析 ->
// 记账）都走真实实现，只有最外层的 claude 可执行文件是假的。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/headless"
	"council/internal/store"
	"council/internal/turn"
)

// TestMain 让同一个测试二进制在子进程里扮演 provider CLI，与
// internal/app/smoke_test.go 的 TestMain 同一套路。
func TestMain(m *testing.M) {
	if os.Getenv("COUNCIL_FAKE_CLI") == "1" {
		runServeFakeProviderCLI()
		return
	}
	os.Exit(m.Run())
}

func serveEnvInt(name string) int {
	n, _ := strconv.Atoi(os.Getenv(name))
	return n
}

// runServeFakeProviderCLI 打一份 claude 适配器认得的成功 JSON。
// COUNCIL_FAKE_SLEEP_ALL_MS 让每次调用先睡一会儿，供「观察者断开后
// 议会仍跑完」制造在飞窗口；正文与金额可由环境变量覆盖。
// COUNCIL_FAKE_ARGV_FILE 非空时把自己收到的 argv 追加成一行 JSON：
// 设置页热换之后真正发给 provider 的 --model / --resume 只有 argv 能证明，
// 配置与指纹都可能是内存里自洽而命令行没变的。
func runServeFakeProviderCLI() {
	// A3：先记 argv 再读 stdin。正文很长时读 stdin 会等到 EOF，而 argv
	// 与正文无关；先记下来，后面即使进程被取消也有证据。
	if path := os.Getenv("COUNCIL_FAKE_ARGV_FILE"); path != "" {
		if line, marshalErr := json.Marshal(os.Args[1:]); marshalErr == nil {
			if f, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); openErr == nil {
				_, _ = f.Write(append(line, '\n'))
				_ = f.Close()
			}
		}
	}
	_, _ = io.ReadAll(os.Stdin)
	if ms := serveEnvInt("COUNCIL_FAKE_SLEEP_ALL_MS"); ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	text := os.Getenv("COUNCIL_FAKE_TEXT")
	if text == "" {
		text = "OK"
	}
	cost := os.Getenv("COUNCIL_FAKE_COST")
	if cost == "" {
		cost = "0.01"
	}
	body, err := json.Marshal(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve fake CLI: %v\n", err)
		os.Exit(3)
	}
	// 字段与 internal/provider/claude.go 的 fail-closed 判据逐项对齐：
	// type=result、is_error=false、subtype=success、terminal_reason=completed、
	// stop_reason=end_turn、api_error_status=null。
	fmt.Fprintf(os.Stdout, `{"type":"result","subtype":"success","is_error":false,`+
		`"terminal_reason":"completed","api_error_status":null,"stop_reason":"end_turn",`+
		`"session_id":"fake-session","result":%s,"total_cost_usd":%s,`+
		`"usage":{"input_tokens":2,"cache_creation_input_tokens":10,`+
		`"cache_read_input_tokens":0,"output_tokens":4,`+
		`"output_tokens_details":{"thinking_tokens":0}}}`, body, cost)
	os.Exit(0)
}

// writeTestEnv 是写模式的 httptest 环境。不复用 newServeTestEnv：那个
// helper 只建只读 server，而这里需要同一份 store 上装配出来的 Service/Runner。
type writeTestEnv struct {
	store *store.Store
	srv   *Server
	http  *httptest.Server
}

// newWriteTestEnv 开一个真库，照 serve.runWritable 的顺序装配
// Service/Runner，再把写模式挂到 httptest 上。
func newWriteTestEnv(t *testing.T) *writeTestEnv {
	t.Helper()
	cfgPath, dataDir := writeAskConfig(t)

	cfg, err := config.LoadAndValidate(cfgPath)
	if err != nil {
		t.Fatalf("加载测试配置失败: %v", err)
	}
	rules, err := config.LoadRules(cfgPath, cfg.RulesFile)
	if err != nil {
		t.Fatalf("加载测试规则失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试库失败: %v", err)
		}
	})

	svc := conversation.NewService(st)
	svc.SetThoughtPolicy(cfg.StoreThought)
	rn := turn.NewRunner(cfg, svc, filepath.Join(dataDir, "workdir"), turn.RulesBlocks{
		Global: rules.GlobalText,
		Local:  rules.LocalText,
	})

	srv := NewServer(st, "test-version", modelsFromConfig(cfg))
	srv.EnableWrite(WriteDeps{
		Cfg:     cfg,
		Svc:     svc,
		Rn:      rn,
		DataDir: dataDir,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &writeTestEnv{store: st, srv: srv, http: ts}
}

// writeAskConfig 落一份能被 config.Validate 通过的最小配置：一个 claude
// 成员，command 指向测试二进制自己。
func writeAskConfig(t *testing.T) (cfgPath, dataDir string) {
	t.Helper()
	dir := t.TempDir()
	dataDir = filepath.Join(dir, "data")
	body := fmt.Sprintf(`data_dir = '%s'

[providers.claude]
command = '%s'

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "serve 写模式测试成员"
permissions = "read"
mode = "discussion"
`, dataDir, os.Args[0])
	cfgPath = filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	// 全局规则文件默认在用户主目录；测试不能读开发机上的那一份，
	// 否则同一个仓库在不同机器上会因规则内容不同而算出不同的身份指纹。
	t.Setenv(config.EnvGlobalRules, filepath.Join(dir, "no-global-rules.md"))
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")
	return cfgPath, dataDir
}

// postRaw 发一次原始 body 的 POST，返回状态码、Content-Type 与 body。
func postRaw(t *testing.T, url, contentType, body string) (int, string, []byte) {
	t.Helper()
	res, err := http.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s 失败: %v", url, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读 POST %s 响应失败: %v", url, err)
	}
	return res.StatusCode, res.Header.Get("Content-Type"), raw
}

// postAsk 发一次合法的 /api/ask，断言 202 与 job_id，返回 job id。
func postAsk(t *testing.T, env *writeTestEnv, payload map[string]any) string {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("编码 ask 请求失败: %v", err)
	}
	status, ctype, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", string(body))
	if status != http.StatusAccepted {
		t.Fatalf("POST /api/ask = %d，期望 202；Content-Type=%q body=%s", status, ctype, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("202 的 Content-Type = %q", ctype)
	}
	var reply struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("202 响应不是合法 JSON: %v；body=%s", err, raw)
	}
	if reply.JobID == "" {
		t.Fatalf("202 响应缺少 job_id: %s", raw)
	}
	return reply.JobID
}

// doAskRequest 带自定义头发一次 /api/ask，供 CSRF 闸的用例使用。
func doAskRequest(t *testing.T, url, contentType, origin, secFetchSite, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造 ask 请求失败: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if secFetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", secFetchSite)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s 失败: %v", url, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读 POST %s 响应失败: %v", url, err)
	}
	return res.StatusCode, raw
}

// waitJobStatus 等一个后台任务结束，返回它最终的 headless 状态。
// 202 用例必须等它，否则测试清库时任务还在写，会被误判成实现有问题。
func waitJobStatus(t *testing.T, env *writeTestEnv, id string) string {
	t.Helper()
	j, ok := env.srv.jobs.get(id)
	if !ok {
		t.Fatalf("任务 %s 不在表里", id)
	}
	select {
	case <-j.done:
	case <-time.After(15 * time.Second):
		t.Fatalf("任务 %s 15 秒内没有结束", id)
	}
	res, err := j.outcome()
	if err != nil {
		return "error: " + err.Error()
	}
	return res.Status
}

// sseFrame 是一帧 SSE。ID 从 1 起，与 handler 写入的 id 一致。
type sseFrame struct {
	ID    int
	Event string
	Data  json.RawMessage
}

// sseReader 从一个在飞的 SSE 响应里逐帧读，并保留原始行供测试打印
// 「真实报文」。用 ReadString 而不是 Scanner：SSE 帧之间没有长度上限，
// Scanner 的默认 token 上限会在一条长正文上误报错误。
type sseReader struct {
	resp *http.Response
	in   *bufio.Reader
	raw  bytes.Buffer
}

func newSSEReader(t *testing.T, ctx context.Context, url string) *sseReader {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("构造 SSE 请求失败: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("连接 SSE %s 失败: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("SSE %s = %d，期望 200；body=%s", url, res.StatusCode, raw)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		defer res.Body.Close()
		t.Fatalf("SSE 的 Content-Type = %q，期望 text/event-stream", got)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("SSE 的 Cache-Control = %q，期望 no-cache", got)
	}
	return &sseReader{resp: res, in: bufio.NewReader(res.Body)}
}

func (r *sseReader) close() {
	_ = r.resp.Body.Close()
}

func (r *sseReader) rawText() string {
	return r.raw.String()
}

// next 读一帧。空行是帧分隔符；单行注释（以 : 开头）直接跳过。
func (r *sseReader) next(t *testing.T) (sseFrame, bool) {
	t.Helper()
	var frame sseFrame
	for {
		line, err := r.in.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return sseFrame{}, false
			}
			t.Fatalf("读 SSE 失败: %v", err)
		}
		r.raw.WriteString(line)
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if frame.Event == "" {
				continue
			}
			return frame, true
		case strings.HasPrefix(line, ":"):
			continue
		case strings.HasPrefix(line, "id: "):
			n, err := strconv.Atoi(strings.TrimPrefix(line, "id: "))
			if err != nil {
				t.Fatalf("SSE id 不是整数: %q", line)
			}
			frame.ID = n
		case strings.HasPrefix(line, "event: "):
			frame.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			frame.Data = append(frame.Data, strings.TrimPrefix(line, "data: ")...)
		}
	}
}

// readUntil 一直读到指定事件（含）或流结束；超过上限直接失败，避免
// 服务端行为坏掉时测试挂死。
func (r *sseReader) readUntil(t *testing.T, name string) []sseFrame {
	t.Helper()
	var frames []sseFrame
	for i := 0; i < 100; i++ {
		frame, ok := r.next(t)
		if !ok {
			t.Fatalf("SSE 在收到 %q 之前就结束了；已收到 %d 帧", name, len(frames))
		}
		frames = append(frames, frame)
		if frame.Event == name {
			return frames
		}
	}
	t.Fatalf("SSE 超过 100 帧还没收到 %q", name)
	return nil
}

// TestRunWritableEndToEnd 从真正的 Run 入口走一次写模式：加载配置与规则、
// store.Open 拿锁、装配 Service/Runner、监听真实端口，再从 HTTP 上进一次
// ask。它补的是 newWriteTestEnv 绕过的那一段装配——handler 本身由后面的
// 用例逐个覆盖，这里只证明「serve -rw 真的能起来并且能跑」。
func TestRunWritableEndToEnd(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	cfgPath, _ := writeAskConfig(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newSyncBuffer()
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Options{
			Addr:       "127.0.0.1:0",
			ConfigPath: cfgPath,
			RW:         true,
			out:        out,
		}, "test-version")
	}()

	select {
	case <-out.wrote:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("10 秒内没有打印启动行")
	}
	started := out.String()
	idx := strings.Index(started, "http://")
	if idx < 0 {
		cancel()
		t.Fatalf("启动行里没有 URL: %q", started)
	}
	url := strings.TrimSpace(started[idx:])

	var health map[string]any
	getJSON(t, url+"/api/health", http.StatusOK, &health)
	if health["writable"] != true {
		t.Errorf("Run(-rw) 的 health.writable = %v，期望 true", health["writable"])
	}

	payload, err := json.Marshal(map[string]any{
		"prompt":  "从真入口跑一次",
		"members": []string{"c1"},
		"rounds":  1,
	})
	if err != nil {
		t.Fatalf("编码 ask 请求失败: %v", err)
	}
	status, ctype, raw := postRaw(t, url+"/api/ask", "application/json", string(payload))
	t.Logf("POST /api/ask -> %d %s %s", status, ctype, raw)
	if status != http.StatusAccepted {
		t.Fatalf("POST /api/ask = %d，期望 202；body=%s", status, raw)
	}
	var reply struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("202 响应不是合法 JSON: %v；body=%s", err, raw)
	}
	if reply.JobID == "" {
		t.Fatalf("202 响应缺少 job_id: %s", raw)
	}

	sseCtx, sseCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer sseCancel()
	reader := newSSEReader(t, sseCtx, url+"/api/jobs/"+reply.JobID+"/events")
	defer reader.close()
	frames := reader.readUntil(t, "done")
	if frames[len(frames)-1].Event != "done" {
		t.Fatalf("最后一帧是 %q，期望 done", frames[len(frames)-1].Event)
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("取消 ctx 之后 Run 返回错误: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("取消 ctx 之后 Run 超过 10 秒没有返回")
	}
}

// TestShutdownWaitsForRunningJob 是复审第 2 条：没有 SSE 读者时，
// HTTP Shutdown 会立刻返回；如果这时就关库放锁，后台议会会静默丢掉。
// 这里故意不订阅 SSE、收到 202 就触发关闭，然后等 Run 返回并直接读库：
// 只有 drain 等到 job 跑完，才能看到 c1 的回复。
func TestShutdownWaitsForRunningJob(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	// 让 provider 慢到足以跨过「Shutdown 已经返回、库还没关」的窗口。
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "1200")
	cfgPath, dataDir := writeAskConfig(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newSyncBuffer()
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Options{
			Addr:       "127.0.0.1:0",
			ConfigPath: cfgPath,
			RW:         true,
			out:        out,
		}, "test-version")
	}()

	select {
	case <-out.wrote:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("10 秒内没有打印启动行")
	}
	started := out.String()
	idx := strings.Index(started, "http://")
	if idx < 0 {
		cancel()
		t.Fatalf("启动行里没有 URL: %q", started)
	}
	url := strings.TrimSpace(started[idx:])

	payload, err := json.Marshal(map[string]any{
		"prompt":  "关闭时也要跑完",
		"members": []string{"c1"},
		"rounds":  1,
	})
	if err != nil {
		t.Fatalf("编码 ask 请求失败: %v", err)
	}
	status, _, raw := postRaw(t, url+"/api/ask", "application/json", string(payload))
	if status != http.StatusAccepted {
		t.Fatalf("POST /api/ask = %d，期望 202；body=%s", status, raw)
	}
	var reply struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil || reply.JobID == "" {
		t.Fatalf("202 响应不合法: err=%v body=%s", err, raw)
	}

	// 不等 SSE、不等 job，直接要求服务关闭：这正是旧实现丢任务的场景。
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("关闭时 Run 返回错误: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("关闭后 Run 超过 20 秒没有返回")
	}

	// Run 返回时 runWritable 的 defer 已经关库放锁。若 drain 没等 job，
	// 这一轮要么没建会话，要么写回复时报「database is closed」，这里读不到。
	st, err := store.OpenReadOnly(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Run 返回后只读打开库失败: %v", err)
	}
	defer func() { _ = st.Close() }()

	convs, err := st.Conversations(context.Background())
	if err != nil {
		t.Fatalf("读会话列表失败: %v", err)
	}
	replied := false
	for _, conv := range convs {
		msgs, err := st.Messages(context.Background(), conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("读会话 %s 的消息失败: %v", conv.ID, err)
		}
		for _, m := range msgs {
			if m.Kind == conversation.KindAgent && m.SenderID == "c1" && m.Text == "OK" {
				replied = true
			}
		}
	}
	if !replied {
		t.Error("关闭时后台议会没有跑完并落库：drain 没有在关库之前等到 job 结束")
	}
}

// TestDrainJobsTimeoutCancels 与 TestDrainJobsGraceExpires 把复审要求的
// 「10 秒等不到就 cancelJobs，再等 2 秒」两段都压到毫秒级验证：
// 前者证明取消能让任务及时收尾，后者证明宽限过后 drainJobs 真的返回错误。
func TestDrainJobsTimeoutCancels(t *testing.T) {
	env := newWriteTestEnv(t)
	// 生产 2 秒；这里压短，验证的是同一条等待路径。
	savedGrace := jobDrainGrace
	jobDrainGrace = 100 * time.Millisecond
	defer func() { jobDrainGrace = savedGrace }()

	env.srv.jobsWG.Add(1)
	observed := make(chan struct{})
	go func() {
		defer env.srv.jobsWG.Done()
		<-env.srv.jobCtx.Done()
		close(observed)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := env.srv.drainJobs(ctx); err != nil {
		t.Fatalf("取消后任务及时收尾，drainJobs 应返回 nil，实际 %v", err)
	}
	select {
	case <-observed:
	default:
		t.Fatal("drainJobs 返回时 jobCtx 尚未取消")
	}
}

func TestDrainJobsGraceExpires(t *testing.T) {
	env := newWriteTestEnv(t)
	savedGrace := jobDrainGrace
	jobDrainGrace = 40 * time.Millisecond
	defer func() { jobDrainGrace = savedGrace }()

	// 这个任务故意不看 jobCtx：模拟一个取消也停不下来的 provider 调用。
	release := make(chan struct{})
	env.srv.jobsWG.Add(1)
	go func() {
		defer env.srv.jobsWG.Done()
		<-release
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := env.srv.drainJobs(ctx); err == nil {
		t.Fatal("宽限期过完任务仍未结束，drainJobs 应返回错误")
	}
	close(release)
	env.srv.jobsWG.Wait()
}

// TestAskSSEEndToEnd 是任务书 §6.4：fake provider 跑一次完整 ask，
// SSE 依次收到 started、至少一条 turn、done，且 done.status 与
// `council ask` 同口径（直接来自 headless 写出的 Result）。
func TestAskSSEEndToEnd(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newWriteTestEnv(t)

	// health 必须告诉前端这是写模式，否则底栏不会出现。
	var health map[string]any
	getJSON(t, env.http.URL+"/api/health", http.StatusOK, &health)
	if health["writable"] != true {
		t.Fatalf("写模式的 health.writable = %v，期望 true", health["writable"])
	}

	jobID := postAsk(t, env, map[string]any{
		"prompt":     "1+1",
		"members":    []string{"c1"},
		"rounds":     1,
		"budget_usd": 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	reader := newSSEReader(t, ctx, env.http.URL+"/api/jobs/"+jobID+"/events")
	defer reader.close()

	frames := reader.readUntil(t, "done")
	t.Logf("SSE 原始报文：\n%s", reader.rawText())

	if len(frames) < 3 {
		t.Fatalf("SSE 只有 %d 帧，期望至少 started/turn/done", len(frames))
	}
	if frames[0].Event != "started" {
		t.Errorf("第一帧是 %q，期望 started", frames[0].Event)
	}
	last := frames[len(frames)-1]
	if last.Event != "done" {
		t.Fatalf("最后一帧是 %q，期望 done", last.Event)
	}
	for i, frame := range frames {
		if frame.ID != i+1 {
			t.Errorf("第 %d 帧的 id = %d，期望 %d（断线续传依赖单调递增）", i, frame.ID, i+1)
		}
	}

	var started startedEvent
	if err := json.Unmarshal(frames[0].Data, &started); err != nil {
		t.Fatalf("started.data 不是合法 JSON: %v；%s", err, frames[0].Data)
	}
	if started.ConversationID == "" {
		t.Errorf("started.conversation_id 为空")
	}

	turnCount := 0
	for _, frame := range frames {
		if frame.Event != "turn" {
			continue
		}
		turnCount++
		var turnEv turnEvent
		if err := json.Unmarshal(frame.Data, &turnEv); err != nil {
			t.Fatalf("turn.data 不是合法 JSON: %v；%s", err, frame.Data)
		}
		if turnEv.AgentID != "c1" || turnEv.Outcome != "ok" {
			t.Errorf("turn 事件 = %+v，期望 c1/ok", turnEv)
		}
		if turnEv.Text != "OK" {
			t.Errorf("turn.text = %q，期望假 provider 的 OK", turnEv.Text)
		}
		if turnEv.CostUSD < 0.0099 || turnEv.CostUSD > 0.0101 {
			t.Errorf("turn.cost_usd = %v，期望 0.01", turnEv.CostUSD)
		}
	}
	if turnCount == 0 {
		t.Fatal("SSE 一帧 turn 都没有")
	}

	var done doneEvent
	if err := json.Unmarshal(last.Data, &done); err != nil {
		t.Fatalf("done.data 不是合法 JSON: %v；%s", err, last.Data)
	}
	if done.Status != "ok" {
		t.Errorf("done.status = %q，期望 ok（与 council ask 的 Result.Status 同源）", done.Status)
	}
	if done.Failed != 0 || done.Total != 1 {
		t.Errorf("done.failed/total = %d/%d，期望 0/1", done.Failed, done.Total)
	}
	if done.CostUSD < 0.0099 || done.CostUSD > 0.0101 {
		t.Errorf("done.cost_usd = %v，期望 0.01", done.CostUSD)
	}
	if done.Summary != "" {
		t.Errorf("配置里没有汇总成员，done.summary 应为空，实际 %q", done.Summary)
	}

	// 库里必须真的落了问题与回复：SSE 只是观察面，事实源仍是 SQLite。
	msgs, err := env.store.Messages(context.Background(), started.ConversationID, 0, 0)
	if err != nil {
		t.Fatalf("读会话消息失败: %v", err)
	}
	var sawUser, sawAgent bool
	for _, m := range msgs {
		if m.Kind == conversation.KindUser && m.Text == "1+1" {
			sawUser = true
		}
		if m.Kind == conversation.KindAgent && m.SenderID == "c1" && m.Text == "OK" {
			sawAgent = true
		}
	}
	if !sawUser || !sawAgent {
		t.Errorf("库里应同时有 user 的问题与 c1 的回复，实际 sawUser=%v sawAgent=%v；消息=%+v",
			sawUser, sawAgent, msgs)
	}
}

// TestSSEDisconnectDoesNotCancelRun 是任务书 §6.5：收到 started 之后
// 立刻断开观察者，议会必须继续跑完、库里必须有回复。
func TestSSEDisconnectDoesNotCancelRun(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	// 让每个 provider 调用慢 800ms：观察者断开时议会一定还在飞。
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "800")
	env := newWriteTestEnv(t)

	jobID := postAsk(t, env, map[string]any{
		"prompt":  "断开也要跑完",
		"members": []string{"c1"},
		"rounds":  1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	reader := newSSEReader(t, ctx, env.http.URL+"/api/jobs/"+jobID+"/events")

	var convID string
	for {
		frame, ok := reader.next(t)
		if !ok {
			t.Fatalf("在 started 之前流就结束了")
		}
		if frame.Event != "started" {
			continue
		}
		var started startedEvent
		if err := json.Unmarshal(frame.Data, &started); err != nil {
			t.Fatalf("started.data 不是合法 JSON: %v", err)
		}
		convID = started.ConversationID
		break
	}
	// 客户端断开：只取消这一条 HTTP 请求，不碰任务本身。
	cancel()
	reader.close()

	j, ok := env.srv.jobs.get(jobID)
	if !ok {
		t.Fatalf("任务 %s 从表里消失了", jobID)
	}
	select {
	case <-j.done:
		t.Fatal("断开观察者之后议会立刻结束，无法证明它没有被取消")
	default:
	}

	select {
	case <-j.done:
	case <-time.After(15 * time.Second):
		t.Fatal("15 秒内议会没有跑完：客户端断开把任务一起取消了")
	}
	res, err := j.outcome()
	if err != nil {
		t.Fatalf("议会跑完但带着错误返回: %v", err)
	}
	if res.Status != "ok" {
		t.Errorf("客户端断开后的 status = %q，期望 ok", res.Status)
	}

	msgs, err := env.store.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("读会话消息失败: %v", err)
	}
	var replied bool
	for _, m := range msgs {
		if m.Kind == conversation.KindAgent && m.SenderID == "c1" && m.Text == "OK" {
			replied = true
		}
	}
	if !replied {
		t.Errorf("客户端断开后库里没有 c1 的回复：%+v", msgs)
	}
}

// TestReadOnlyWriteEndpointsReturn405 是任务书 §6.2：不带 -rw 时，
// 写端点必须是 405 + JSON 说明；health 的键集合也必须与 SRV1 一致。
func TestReadOnlyWriteEndpointsReturn405(t *testing.T) {
	env := newServeTestEnv(t, nil)

	status, ctype, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", `{"prompt":"x"}`)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式 POST /api/ask = %d，期望 405；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("405 的 Content-Type = %q", ctype)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("405 响应不是合法 JSON: %v；%s", err, raw)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, "-rw") {
		t.Errorf("405 的说明没有告诉用户用 -rw：%q", message)
	}

	// 写端点不接受 GET；只读模式下同样先说明是只读。
	res, err := http.Get(env.http.URL + "/api/ask")
	if err != nil {
		t.Fatalf("GET /api/ask 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("只读模式 GET /api/ask = %d，期望 405", res.StatusCode)
	}

	// 任务 SSE 也属于写模式的能力：只读模式下即使 GET 也回 405 + 说明，
	// 不能落进 /api/ 的兜底变成「没有这个接口」。
	jobRes, err := http.Get(env.http.URL + "/api/jobs/some-id/events")
	if err != nil {
		t.Fatalf("GET /api/jobs/... 失败: %v", err)
	}
	defer jobRes.Body.Close()
	if jobRes.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("只读模式 GET /api/jobs/... = %d，期望 405", jobRes.StatusCode)
	}
	var jobBody map[string]any
	if err := json.NewDecoder(jobRes.Body).Decode(&jobBody); err != nil {
		t.Fatalf("任务端点 405 不是合法 JSON: %v", err)
	}
	if msg, _ := jobBody["error"].(string); !strings.Contains(msg, "-rw") {
		t.Errorf("任务端点 405 的说明没有告诉用户用 -rw: %q", msg)
	}

	var health map[string]any
	getJSON(t, env.http.URL+"/api/health", http.StatusOK, &health)
	if _, exists := health["writable"]; exists {
		t.Errorf("只读模式的 health 不应出现 writable 键（SRV1 的响应必须逐字节不变）：%v", health)
	}
}

// TestAskRejectsBadJSONAndBudget 是任务书 §6.3：坏 JSON 与非法预算/轮次
// 都必须在建任务之前回 400，不能留下一个必然失败的后台 job。
func TestAskRejectsBadJSONAndBudget(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newWriteTestEnv(t)

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "坏 JSON", body: `{"prompt":`, want: "JSON"},
		{name: "预算低于 0.5", body: `{"prompt":"x","budget_usd":0.1}`, want: "0.5"},
		{name: "轮次为零", body: `{"prompt":"x","rounds":0}`, want: "rounds"},
		{name: "超时为零", body: `{"prompt":"x","timeout_seconds":0}`, want: "timeout_seconds"},
		{name: "空问题", body: `{"prompt":"   "}`, want: "问题"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, ctype, raw := postRaw(t, env.http.URL+"/api/ask", "application/json", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST /api/ask = %d，期望 400；body=%s", status, raw)
			}
			if ctype != "application/json; charset=utf-8" {
				t.Errorf("400 的 Content-Type = %q", ctype)
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("400 响应不是合法 JSON: %v；%s", err, raw)
			}
			message, _ := body["error"].(string)
			if !strings.Contains(message, tc.want) {
				t.Errorf("400 的 error = %q，期望包含 %q", message, tc.want)
			}
		})
	}

	env.srv.jobs.mu.Lock()
	jobCount := len(env.srv.jobs.jobs)
	env.srv.jobs.mu.Unlock()
	if jobCount != 0 {
		t.Errorf("被拒的请求建出了 %d 个任务，期望 0", jobCount)
	}
}

// TestAskWriteGuard 覆盖写端点的 CSRF 两道闸：
// 非 application/json（含无 Content-Type）→ 415；浏览器跨站 Origin /
// Sec-Fetch-Site → 403；同源与无 Origin 的本机调用 → 202。
// 这一条防的是「任何网页用简单请求打本机端口起议会、花额度」。
func TestAskWriteGuard(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newWriteTestEnv(t)
	askURL := env.http.URL + "/api/ask"
	validBody := `{"prompt":"守卫测试","members":["c1"],"rounds":1}`

	t.Run("text/plain 415", func(t *testing.T) {
		status, raw := doAskRequest(t, askURL, "text/plain", "", "", validBody)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("text/plain = %d，期望 415；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "application/json") {
			t.Errorf("415 的说明没有告诉用户要 application/json: %s", raw)
		}
	})

	t.Run("无 Content-Type 415", func(t *testing.T) {
		status, raw := doAskRequest(t, askURL, "", "", "", validBody)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("无 Content-Type = %d，期望 415；body=%s", status, raw)
		}
	})

	t.Run("跨站 Origin 403", func(t *testing.T) {
		status, raw := doAskRequest(t, askURL, "application/json", "https://evil.example", "", validBody)
		if status != http.StatusForbidden {
			t.Fatalf("evil Origin = %d，期望 403；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "跨站") {
			t.Errorf("403 的说明没有提到跨站: %s", raw)
		}
	})

	t.Run("跨站 Sec-Fetch-Site 403", func(t *testing.T) {
		status, raw := doAskRequest(t, askURL, "application/json", "", "cross-site", validBody)
		if status != http.StatusForbidden {
			t.Fatalf("cross-site = %d，期望 403；body=%s", status, raw)
		}
	})

	t.Run("同源 Origin 202", func(t *testing.T) {
		status, raw := doAskRequest(t, askURL, "application/json", env.http.URL, "", validBody)
		if status != http.StatusAccepted {
			t.Fatalf("同源 Origin = %d，期望 202；body=%s", status, raw)
		}
		var reply struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil || reply.JobID == "" {
			t.Fatalf("202 响应不合法: err=%v body=%s", err, raw)
		}
		if got := waitJobStatus(t, env, reply.JobID); got != "ok" {
			t.Errorf("同源任务 status = %q，期望 ok", got)
		}
	})

	t.Run("无 Origin 202", func(t *testing.T) {
		status, raw := doAskRequest(t, askURL, "application/json", "", "", validBody)
		if status != http.StatusAccepted {
			t.Fatalf("无 Origin = %d，期望 202；body=%s", status, raw)
		}
		var reply struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil || reply.JobID == "" {
			t.Fatalf("202 响应不合法: err=%v body=%s", err, raw)
		}
		if got := waitJobStatus(t, env, reply.JobID); got != "ok" {
			t.Errorf("无 Origin 任务 status = %q，期望 ok", got)
		}
	})
}

// TestRunAskJobRecoversPanic 钉住「回调/任务 panic 不能带崩整个 serve」。
// 用把 s.write 置空来在 runAskJob 最开头制造一次真实的 nil 解引用；
// recover 必须发出 error + done，并让 job 正常结束。
func TestRunAskJobRecoversPanic(t *testing.T) {
	env := newWriteTestEnv(t)
	j := newJob("panic-job")
	env.srv.jobs.add(j)
	env.srv.jobsWG.Add(1)

	saved := env.srv.write
	env.srv.write = nil
	defer func() { env.srv.write = saved }()
	env.srv.runAskJob(j, headless.Options{})

	select {
	case <-j.done:
	case <-time.After(2 * time.Second):
		t.Fatal("panic 之后任务没有结束")
	}
	res, err := j.outcome()
	if res.Status != "error" || err == nil {
		t.Errorf("panic 后 outcome = (%q, %v)，期望 error + 非 nil", res.Status, err)
	}

	var names []string
	for i := 0; ; i++ {
		ev, ok, _, _ := j.next(i)
		if !ok {
			break
		}
		names = append(names, ev.Name)
	}
	if strings.Join(names, ",") != "error,done" {
		t.Errorf("panic 后事件 = %v，期望 error,done", names)
	}
}

// TestJobUnknownIs404 与 TestJobCleanedUpAfterTTL 是任务书 §6.6：
// 未知 id → 404；任务结束后按可注入的 TTL 清理，不真等 60 秒。
func TestJobUnknownIs404(t *testing.T) {
	env := newWriteTestEnv(t)

	res, err := http.Get(env.http.URL + "/api/jobs/no-such-job/events")
	if err != nil {
		t.Fatalf("GET 未知 job 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("未知 job = %d，期望 404", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("404 的 Content-Type = %q，期望 JSON", got)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("404 响应不是合法 JSON: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("404 响应缺少 error 字段: %v", body)
	}
}

func TestJobCleanedUpAfterTTL(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newWriteTestEnv(t)
	// 可注入的 TTL：任务书允许的 60 秒在生产里由 defaultJobTTL 提供，
	// 测试把它压到 300ms，验证的是同一段清理代码。
	env.srv.jobTTL = 300 * time.Millisecond

	jobID := postAsk(t, env, map[string]any{
		"prompt":  "跑完就清掉",
		"members": []string{"c1"},
		"rounds":  1,
	})
	j, ok := env.srv.jobs.get(jobID)
	if !ok {
		t.Fatalf("刚创建的任务 %s 不在表里", jobID)
	}
	select {
	case <-j.done:
	case <-time.After(15 * time.Second):
		t.Fatal("15 秒内任务没有结束")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, exists := env.srv.jobs.get(jobID); !exists {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("任务结束 5 秒后仍在内存表里（TTL=300ms）")
		}
		time.Sleep(10 * time.Millisecond)
	}

	res, err := http.Get(env.http.URL + "/api/jobs/" + jobID + "/events")
	if err != nil {
		t.Fatalf("清理后 GET job 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("清理后 GET job = %d，期望 404", res.StatusCode)
	}
}
