package mcp_test

// 本文件用 internal/app 的正式装配路径驱动 `council mcp`：临时配置 + 临时
// data_dir + 真 SQLite，provider 由测试二进制自己扮演（与 internal/headless
// 的 e2e 同一套装置）。这样测到的是「宿主真正会拉起的那个进程」的行为，
// 而不是一个手搭的假 server。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"council/internal/app"
	"council/internal/config"
	"council/internal/i18n"
	"council/internal/mcp"
	"council/internal/store"
)

func TestMain(m *testing.M) {
	if os.Getenv("COUNCIL_FAKE_CLI") == "1" {
		runFakeProviderCLI()
		return
	}
	os.Exit(m.Run())
}

// runFakeProviderCLI 扮演 claude：读掉 stdin，可选地睡一会儿（取消用例要
// 一个「确实在飞」的子进程），然后吐一行 claude 形状的 JSON。
func runFakeProviderCLI() {
	_, _ = io.ReadAll(os.Stdin)
	fakeCallNumber()
	if ms := envInt("COUNCIL_FAKE_SLEEP_ALL_MS"); ms > 0 {
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
		fmt.Fprintf(os.Stderr, "fake CLI: %v\n", err)
		os.Exit(3)
	}
	fmt.Fprintf(os.Stdout, `{"type":"result","subtype":"success","is_error":false,`+
		`"terminal_reason":"completed","api_error_status":null,"stop_reason":"end_turn",`+
		`"session_id":"fake-session","result":%s,"total_cost_usd":%s,`+
		`"usage":{"input_tokens":2,"cache_creation_input_tokens":10,`+
		`"cache_read_input_tokens":0,"output_tokens":4,`+
		`"output_tokens_details":{"thinking_tokens":0}}}`, body, cost)
	os.Exit(0)
}

func envInt(name string) int {
	n, _ := strconv.Atoi(os.Getenv(name))
	return n
}

// fakeCallNumber 往计数文件里追加一个字节，让测试能等「provider 子进程真的
// 起来了」再发取消通知。
func fakeCallNumber() {
	path := os.Getenv("COUNCIL_FAKE_CALLS")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write([]byte("x"))
	_ = f.Close()
}

const testTimeout = 60 * time.Second

// writeMCPConfig 落一份能通过 config.Validate 的配置。provider 命令指向
// 测试二进制自己；withSummary 决定 [summary].agent 是否存在。
func writeMCPConfig(t *testing.T, withSummary bool) (cfgPath, dataDir string) {
	t.Helper()
	dir := t.TempDir()
	dataDir = filepath.Join(dir, "data")
	summary := ""
	if withSummary {
		summary = "[summary]\nagent = \"s1\"\n"
	}
	body := fmt.Sprintf(`data_dir = '%s'

[providers.claude]
command = '%s'

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "MCP 测试成员一"
permissions = "read"
mode = "discussion"

[agents.c2]
display_name = "C2"
provider = "claude"
model = "sonnet"
effort = "high"
role = "MCP 测试成员二"
permissions = "read"
mode = "discussion"

[agents.s1]
display_name = "S1"
provider = "claude"
model = "opus"
effort = "high"
role = "MCP 测试汇总人"
permissions = "read"
mode = "discussion"

%s`, dataDir, os.Args[0], summary)
	cfgPath = filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	return cfgPath, dataDir
}

// rpcLine 拼一行请求。params 是原始 JSON 串。
func rpcLine(id int, method, params string) string {
	if params == "" {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q}`, id, method)
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params)
}

func toolCallLine(id int, name, args string) string {
	return rpcLine(id, "tools/call", fmt.Sprintf(`{"name":%q,"arguments":%s}`, name, args))
}

// isolateEnv 是本组用例共用的进程环境隔离：把用户主目录指到临时目录，
// 免得真机上 ~/.claude/agents 的内容混进成员表（ScanClaudeAgents 会扫它）。
func isolateEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
}

// runServer 走 app.Run 的 MCP 分支，喂完 stdin 等它自然退出。
func runServer(t *testing.T, cfgPath string, in io.Reader) (stdout, stderr string, err error) {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")
	isolateEnv(t)

	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, app.Options{
			ConfigPath: cfgPath,
			Version:    "test-version",
			In:         in,
			Out:        &out,
			Err:        &errOut,
			MCP:        &mcp.Options{Lang: i18n.LangZh},
			Lang:       i18n.LangZh,
		})
	}()

	select {
	case err = <-done:
		return out.String(), errOut.String(), err
	case <-time.After(testTimeout):
		t.Fatalf("council mcp 在 %v 内没有退出", testTimeout)
		return "", "", nil
	}
}

// runServerWithLines 把若干行拼成 stdin 一次喂完，再关闭（strings.Reader
// 读到底就是 EOF）。
func runServerWithLines(t *testing.T, cfgPath string, lines ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runServer(t, cfgPath, strings.NewReader(strings.Join(lines, "\n")+"\n"))
}

type testResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type testToolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

func parseResponses(t *testing.T, stdout string) []testResponse {
	t.Helper()
	var responses []testResponse
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			t.Fatalf("stdout 里有非 JSON 行: %q\n完整 stdout:\n%s", line, stdout)
		}
		var resp testResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("解析响应失败: %v（行: %s）", err, line)
		}
		responses = append(responses, resp)
	}
	return responses
}

func responseByID(t *testing.T, responses []testResponse, id int) testResponse {
	t.Helper()
	want := fmt.Sprintf("%d", id)
	for _, resp := range responses {
		if string(resp.ID) == want {
			return resp
		}
	}
	t.Fatalf("没有找到 id=%d 的响应；实际响应: %s", id, dumpIDs(responses))
	return testResponse{}
}

func dumpIDs(responses []testResponse) string {
	ids := make([]string, len(responses))
	for i, resp := range responses {
		ids[i] = string(resp.ID)
	}
	return strings.Join(ids, ", ")
}

func decodeToolResult(t *testing.T, resp testResponse) testToolResult {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("期望 tools/call 返回 result，实际是 JSON-RPC error: %+v", resp.Error)
	}
	var result testToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("解 tools/call result 失败: %v（原始: %s）", err, resp.Result)
	}
	return result
}

func onlyText(t *testing.T, result testToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("content 应当只有一段文本，实际 %d 段", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Fatalf("content[0].type = %q，期望 text", result.Content[0].Type)
	}
	return result.Content[0].Text
}

func TestProtocol_初始化(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	stdout, stderr, err := runServerWithLines(t, cfgPath, rpcLine(1, "initialize",
		`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}`))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 1 {
		t.Fatalf("initialize 应当只回一行，实际 %d 行", len(responses))
	}
	resp := responses[0]
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q，期望 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != "1" {
		t.Errorf("id = %s，期望 1", resp.ID)
	}

	var result struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("解 initialize result 失败: %v", err)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocolVersion = %q", result.ProtocolVersion)
	}
	if _, ok := result.Capabilities["tools"]; !ok {
		t.Errorf("capabilities 缺少 tools: %s", resp.Result)
	}
	if result.ServerInfo.Name != "council" {
		t.Errorf("serverInfo.name = %q", result.ServerInfo.Name)
	}
	if result.ServerInfo.Version != "test-version" {
		t.Errorf("serverInfo.version = %q，期望 test-version", result.ServerInfo.Version)
	}
	if !strings.Contains(stderr, "stdio 服务器就绪") {
		t.Errorf("stderr 里应有启动日志，实际:\n%s", stderr)
	}
}

func TestProtocol_工具列表(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	stdout, stderr, err := runServerWithLines(t, cfgPath, rpcLine(2, "tools/list", ""))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 1 {
		t.Fatalf("tools/list 应当只回一行，实际 %d 行", len(responses))
	}

	var result struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(responses[0].Result, &result); err != nil {
		t.Fatalf("解 tools/list result 失败: %v", err)
	}
	if len(result.Tools) != 4 {
		t.Fatalf("应当有 4 个工具，实际 %d", len(result.Tools))
	}
	byName := make(map[string]map[string]any, len(result.Tools))
	for _, tool := range result.Tools {
		if tool.Description == "" {
			t.Errorf("工具 %s 缺少 description", tool.Name)
		}
		byName[tool.Name] = tool.InputSchema
	}
	for _, name := range []string{"council_ask", "council_models", "council_round", "council_who"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("tools/list 缺少 %s（实际: %v）", name, keysOf(byName))
		}
	}

	askSchema := byName["council_ask"]
	askProps := schemaProperties(t, askSchema)
	if required := schemaRequired(t, askSchema); len(required) != 1 || required[0] != "prompt" {
		t.Errorf("council_ask required = %v，期望 [prompt]", required)
	}
	if got := askProps["budget_usd"].(map[string]any)["minimum"]; got != 0.5 {
		t.Errorf("council_ask budget_usd.minimum = %v，期望 0.5", got)
	}
	if _, ok := askProps["conversation"]; !ok {
		t.Errorf("council_ask 缺少 conversation 参数")
	}
	if _, ok := askProps["cwd"]; !ok {
		t.Errorf("council_ask 缺少 cwd 参数")
	}
	langEnum, _ := askProps["lang"].(map[string]any)["enum"].([]any)
	if len(langEnum) != 2 || langEnum[0] != "zh" || langEnum[1] != "en" {
		t.Errorf("council_ask lang.enum = %v，期望 [zh en]", langEnum)
	}

	roundProps := schemaProperties(t, byName["council_round"])
	roundsProp := roundProps["rounds"].(map[string]any)
	if roundsProp["minimum"] != 1.0 {
		t.Errorf("council_round rounds.minimum = %v，期望 1", roundsProp["minimum"])
	}
	if roundsProp["maximum"] != 5.0 {
		t.Errorf("council_round rounds.maximum = %v，期望 5", roundsProp["maximum"])
	}

	modelsSchema := byName["council_models"]
	if props := schemaProperties(t, modelsSchema); len(props) != 0 {
		t.Errorf("council_models 不应有参数，实际 %v", keysOf(anyMap(props)))
	}

	whoSchema := byName["council_who"]
	if props := schemaProperties(t, whoSchema); len(props) != 0 {
		t.Errorf("council_who 不应有参数，实际 %v", keysOf(anyMap(props)))
	}
	if whoSchema["additionalProperties"] != false {
		t.Errorf("council_who 的 additionalProperties = %v，期望 false", whoSchema["additionalProperties"])
	}
}

func schemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema 缺少 properties: %v", schema)
	}
	return props
}

func schemaRequired(t *testing.T, schema map[string]any) []string {
	t.Helper()
	raw, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema 缺少 required: %v", schema)
	}
	out := make([]string, len(raw))
	for i, value := range raw {
		out[i] = value.(string)
	}
	return out
}

func anyMap(m map[string]any) map[string]any { return m }

func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func TestProtocol_ping(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	stdout, _, err := runServerWithLines(t, cfgPath, rpcLine(9, "ping", ""))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v", err)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 1 {
		t.Fatalf("ping 应当只回一行，实际 %d 行", len(responses))
	}
	if string(responses[0].Result) != "{}" {
		t.Errorf("ping result = %s，期望 {}", responses[0].Result)
	}
}

func TestProtocol_未知方法(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	stdout, stderr, err := runServerWithLines(t, cfgPath, rpcLine(7, "tools/does_not_exist", "{}"))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 1 || responses[0].Error == nil {
		t.Fatalf("未知方法应当回一条 JSON-RPC error，实际: %s", stdout)
	}
	if responses[0].Error.Code != -32601 {
		t.Errorf("错误码 = %d，期望 -32601", responses[0].Error.Code)
	}
	if string(responses[0].ID) != "7" {
		t.Errorf("错误响应的 id = %s，期望 7", responses[0].ID)
	}
}

func TestProtocol_坏JSON(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	stdout, _, err := runServerWithLines(t, cfgPath, `{"jsonrpc":"2.0","id":3,"method":`)
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v", err)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 1 || responses[0].Error == nil {
		t.Fatalf("坏 JSON 应当回一条 JSON-RPC error，实际: %s", stdout)
	}
	if responses[0].Error.Code != -32700 {
		t.Errorf("错误码 = %d，期望 -32700", responses[0].Error.Code)
	}
	if string(responses[0].ID) != "null" {
		t.Errorf("解析失败时 id 应为 null，实际 %s", responses[0].ID)
	}
}

func TestProtocol_通知不回应(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	stdout, _, err := runServerWithLines(t, cfgPath,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		rpcLine(2, "ping", ""),
		`{"jsonrpc":"2.0","method":"notifications/whatever"}`,
	)
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v", err)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 1 {
		t.Fatalf("通知不应回包，实际 %d 行: %s", len(responses), stdout)
	}
	if string(responses[0].ID) != "2" {
		t.Errorf("唯一的响应应当属于 ping(id=2)，实际 id=%s", responses[0].ID)
	}
}

func TestProtocol_非法参数(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	stdout, stderr, err := runServerWithLines(t, cfgPath,
		rpcLine(1, "tools/call", `{}`),
		toolCallLine(2, "no_such_tool", `{}`),
		toolCallLine(3, "council_models", `[]`),
	)
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 3 {
		t.Fatalf("应当有 3 条错误响应，实际 %d 行: %s", len(responses), stdout)
	}
	for _, resp := range responses {
		if resp.Error == nil {
			t.Errorf("id=%s 应当是 JSON-RPC error，实际 result=%s", resp.ID, resp.Result)
			continue
		}
		if resp.Error.Code != -32602 {
			t.Errorf("id=%s 的错误码 = %d，期望 -32602", resp.ID, resp.Error.Code)
		}
	}
}

// TestStdoutClean 是「stdout 只能有协议消息」那条要求的端到端证据：一次
// 会话里跑初始化、工具列表、models、真的跑一次议会、ping、非法参数与坏
// JSON，然后逐行断言只有合法 JSON-RPC。横幅、进度、claude 调用日志都不许
// 出现。
func TestStdoutClean(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	t.Setenv("COUNCIL_FAKE_TEXT", "clean stdout answer")

	lines := []string{
		rpcLine(1, "initialize", `{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}`),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		rpcLine(2, "tools/list", ""),
		toolCallLine(3, "council_models", `{}`),
		toolCallLine(4, "council_ask", `{"prompt":"stdout 干净吗","members":["c1"],"summary":"none","budget_usd":5}`),
		rpcLine(5, "ping", ""),
		rpcLine(6, "tools/call", `{}`),
		`{"jsonrpc":"2.0","id":7,"method":`,
		rpcLine(8, "nope/nope", "{}"),
	}
	stdout, stderr, err := runServerWithLines(t, cfgPath, lines...)
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}

	responses := parseResponses(t, stdout)
	if len(responses) != 8 {
		t.Fatalf("应当有 8 条响应（通知不回包），实际 %d 行:\n%s", len(responses), stdout)
	}
	for _, resp := range responses {
		if resp.JSONRPC != "2.0" {
			t.Errorf("有一行的 jsonrpc 不是 2.0: %+v", resp)
		}
		if resp.Error == nil && len(resp.Result) == 0 {
			t.Errorf("有一行既没有 result 也没有 error: %+v", resp)
		}
	}

	for _, forbidden := range []string{"配置文件:", "数据目录:", "库文件:", "已加载", "输入 /help", "council mcp:"} {
		if strings.Contains(stdout, forbidden) {
			t.Errorf("stdout 里混进了非协议内容 %q:\n%s", forbidden, stdout)
		}
	}
	if !strings.Contains(stderr, "council mcp:") {
		t.Errorf("日志应当全部在 stderr，实际 stderr:\n%s", stderr)
	}

	// 真的跑过一次议会：证明工具执行期间 provider 的输出也没有漏进 stdout。
	askText := onlyText(t, decodeToolResult(t, responseByID(t, responses, 4)))
	if !strings.Contains(askText, "clean stdout answer") {
		t.Errorf("council_ask 的正文里应有假 provider 的回答，实际:\n%s", askText)
	}
}

// TestCouncilAsk_文本与落库 覆盖要求里的第 3 条：假 provider 的正文出现在
// 返回文本里，并且问题与回答真的写进了库。
func TestCouncilAsk_文本与落库(t *testing.T) {
	cfgPath, dataDir := writeMCPConfig(t, false)
	t.Setenv("COUNCIL_FAKE_TEXT", "pong from fake")

	stdout, stderr, err := runServerWithLines(t, cfgPath,
		toolCallLine(1, "council_ask", `{"prompt":"1+1 等于几","members":["c1"],"summary":"none","budget_usd":5}`))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	responses := parseResponses(t, stdout)
	result := decodeToolResult(t, responseByID(t, responses, 1))
	if result.IsError {
		t.Fatalf("council_ask 不应失败，实际文本:\n%s", onlyText(t, result))
	}
	text := onlyText(t, result)
	for _, want := range []string{"成员 c1（C1）", "pong from fake", "费用: $0.0100"} {
		if !strings.Contains(text, want) {
			t.Errorf("返回文本缺少 %q，实际:\n%s", want, text)
		}
	}
	if strings.Contains(text, "不完整") {
		t.Errorf("claude 报金额，费用行不该标不完整:\n%s", text)
	}

	// 库已经关掉（app.Run 返回时 defer 已经 Close），只读打开来核对事实。
	ctx := context.Background()
	st, err := store.OpenReadOnly(ctx, dataDir)
	if err != nil {
		t.Fatalf("只读打开库失败: %v", err)
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			t.Errorf("关闭只读库失败: %v", cerr)
		}
	}()

	convs, err := st.Conversations(ctx)
	if err != nil {
		t.Fatalf("读会话失败: %v", err)
	}
	var convID string
	for _, conv := range convs {
		if conv.Title == "1+1 等于几" {
			convID = conv.ID
			break
		}
	}
	if convID == "" {
		t.Fatalf("库里没有标题为问题的会话；实际会话: %+v", convs)
	}

	msgs, err := st.Messages(ctx, convID, 0, 100)
	if err != nil {
		t.Fatalf("读消息失败: %v", err)
	}
	var sawUser, sawAnswer bool
	for _, msg := range msgs {
		switch {
		case msg.SenderID == "user" && msg.Text == "1+1 等于几":
			sawUser = true
		case msg.SenderID == "c1" && strings.Contains(msg.Text, "pong from fake"):
			sawAnswer = true
		}
	}
	if !sawUser {
		t.Errorf("库里没有落下的用户问题；实际消息: %+v", msgs)
	}
	if !sawAnswer {
		t.Errorf("库里没有 c1 的回答；实际消息: %+v", msgs)
	}
}

// TestCouncilAsk_预算触线是isError 覆盖「工具失败用 isError」这条：headless
// 在预算触线时仍然写出结果，MCP 要把正文保留下来、把 isError 置真，并在
// 文本里说清触线原因。
func TestCouncilAsk_预算触线是isError(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	t.Setenv("COUNCIL_FAKE_TEXT", "costly answer")
	t.Setenv("COUNCIL_FAKE_COST", "1.00")

	stdout, stderr, err := runServerWithLines(t, cfgPath,
		toolCallLine(1, "council_ask", `{"prompt":"花钱","members":["c1","c2"],"summary":"none","budget_usd":0.5}`))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	result := decodeToolResult(t, responseByID(t, parseResponses(t, stdout), 1))
	if !result.IsError {
		t.Fatalf("预算触线应当是 isError，实际文本:\n%s", onlyText(t, result))
	}
	text := onlyText(t, result)
	for _, want := range []string{"budget_exceeded", "预算", "费用: $"} {
		if !strings.Contains(text, want) {
			t.Errorf("预算触线文本缺少 %q，实际:\n%s", want, text)
		}
	}
}

// TestCouncilAsk_续接与英文输出 覆盖 conversation 与 lang 两个参数：第二次
// 调用接在第一场的会话后面，输出换成英文形状。
func TestCouncilAsk_续接与英文输出(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	t.Setenv("COUNCIL_FAKE_TEXT", "continued answer")

	stdout, stderr, err := runServerWithLines(t, cfgPath,
		toolCallLine(1, "council_ask", `{"prompt":"first question","members":["c1"],"summary":"none","budget_usd":5}`))
	if err != nil {
		t.Fatalf("第一次 app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	first := onlyText(t, decodeToolResult(t, responseByID(t, parseResponses(t, stdout), 1)))
	convID := conversationIDFromText(first)
	if convID == "" {
		t.Fatalf("第一次返回没有「会话:」行:\n%s", first)
	}

	stdout, stderr, err = runServerWithLines(t, cfgPath,
		toolCallLine(2, "council_ask",
			fmt.Sprintf(`{"prompt":"second question","members":["c1"],"conversation":%q,"summary":"none","budget_usd":5,"lang":"en"}`, convID)))
	if err != nil {
		t.Fatalf("第二次 app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	second := onlyText(t, decodeToolResult(t, responseByID(t, parseResponses(t, stdout), 2)))
	for _, want := range []string{"conversation: " + convID, "member c1 (C1):", "Cost: $"} {
		if !strings.Contains(second, want) {
			t.Errorf("英文续接输出缺少 %q，实际:\n%s", want, second)
		}
	}
}

func conversationIDFromText(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "会话: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "会话: "))
		}
	}
	return ""
}

// TestCouncilModels_配置与Claude导入 覆盖第 4 条：配置成员带 source=config，
// .claude/agents 里扫到的成员带 source=claude。
func TestCouncilModels_配置与Claude导入(t *testing.T) {
	projectDir := t.TempDir()
	agentsDir := filepath.Join(projectDir, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("建 .claude/agents 失败: %v", err)
	}
	helper := `---
name: helper
description: MCP 测试导入成员
model: sonnet
tools: Read
---
你是一个从 Claude Code 导入的助手。
`
	if err := os.WriteFile(filepath.Join(agentsDir, "helper.md"), []byte(helper), 0o644); err != nil {
		t.Fatalf("写 helper.md 失败: %v", err)
	}
	// 从项目目录启动：ScanClaudeAgents 按 cwd 向上找 .claude/agents。
	// 用户主目录由 runServer 统一隔离到临时目录，真机上的 agents 不参与。
	t.Chdir(projectDir)

	cfgPath, _ := writeMCPConfig(t, false)
	stdout, stderr, err := runServerWithLines(t, cfgPath, toolCallLine(1, "council_models", `{}`))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	text := onlyText(t, decodeToolResult(t, responseByID(t, parseResponses(t, stdout), 1)))
	for _, want := range []string{
		"id=c1", `display_name="C1"`, "provider=claude", "model=opus", "permissions=read", "source=config",
		"id=helper", "model=sonnet", "source=claude",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("成员列表缺少 %q，实际:\n%s", want, text)
		}
	}
}

// Test参数校验 覆盖第 5 条：未知成员、预算 0.1、轮数 0 都是 isError，而且
// 文案点名了问题。
func Test参数校验(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, true)
	stdout, stderr, err := runServerWithLines(t, cfgPath,
		toolCallLine(1, "council_ask", `{"prompt":"hi","members":["nope"]}`),
		toolCallLine(2, "council_ask", `{"prompt":"hi","budget_usd":0.1}`),
		toolCallLine(3, "council_round", `{"prompt":"hi","rounds":0}`),
		toolCallLine(4, "council_ask", `{"prompt":"   "}`),
		toolCallLine(5, "council_ask", `{"prompt":"hi","lang":"fr"}`),
		toolCallLine(6, "council_ask", `{"prompt":"hi","cwd":"relative\\dir"}`),
	)
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 6 {
		t.Fatalf("应当有 6 条 tools/call 响应，实际 %d: %s", len(responses), stdout)
	}

	cases := []struct {
		id   int
		want string
	}{
		{1, "未知成员"},
		{1, `"nope"`},
		{2, "0.5"},
		{3, "1..5"},
		{4, "prompt"},
		{5, "lang"},
		{6, "cwd"},
	}
	for _, tc := range cases {
		result := decodeToolResult(t, responseByID(t, responses, tc.id))
		if !result.IsError {
			t.Errorf("id=%d 应当是 isError，实际:\n%s", tc.id, onlyText(t, result))
			continue
		}
		if text := onlyText(t, result); !strings.Contains(text, tc.want) {
			t.Errorf("id=%d 的说明缺少 %q，实际:\n%s", tc.id, tc.want, text)
		}
	}
}

// TestCouncilRound_多轮与汇总 确保第三个工具真的走 round 模式，返回汇总、
// 各轮与费用行。用两个成员：单成员的「第 2 轮」没有新 delta，成员会被
// Runner 拒绝（ErrNoDelta），那是既有语义，不是这个工具的失败。
func TestCouncilRound_多轮与汇总(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, true)
	t.Setenv("COUNCIL_FAKE_TEXT", "round answer")

	stdout, stderr, err := runServerWithLines(t, cfgPath,
		toolCallLine(1, "council_round", `{"prompt":"辩论一下","members":["c1","c2"],"rounds":2,"summary":"s1","budget_usd":5}`))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	result := decodeToolResult(t, responseByID(t, parseResponses(t, stdout), 1))
	if result.IsError {
		t.Fatalf("council_round 不应失败，实际文本:\n%s", onlyText(t, result))
	}
	text := onlyText(t, result)
	for _, want := range []string{"## 汇总", "## 第 1 轮", "## 第 2 轮", "round answer", "费用: $"} {
		if !strings.Contains(text, want) {
			t.Errorf("轮次结果缺少 %q，实际:\n%s", want, text)
		}
	}
	if strings.Contains(text, "partial") || strings.Contains(text, "refused") {
		t.Errorf("两成员两轮不应有失败，实际:\n%s", text)
	}
	if strings.Index(text, "## 汇总") > strings.Index(text, "## 第 1 轮") {
		t.Errorf("汇总应当排在各轮之前，实际:\n%s", text)
	}
}

// Test取消通知中断在飞工具 覆盖「不阻塞 stdin 循环」：议会还在飞时发
// notifications/cancelled，进程应在假 provider 睡满 8 秒之前返回，并且不再
// 给被取消的请求回包。
func Test取消通知中断在飞工具(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)
	callsFile := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "8000")
	t.Setenv("COUNCIL_FAKE_CALLS", callsFile)
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")
	isolateEnv(t)

	pr, pw := io.Pipe()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, app.Options{
			ConfigPath: cfgPath,
			Version:    "test-version",
			In:         pr,
			Out:        &out,
			Err:        &errOut,
			MCP:        &mcp.Options{Lang: i18n.LangZh},
			Lang:       i18n.LangZh,
		})
	}()

	writeLine := func(line string) {
		t.Helper()
		if _, err := fmt.Fprintln(pw, line); err != nil {
			t.Fatalf("写 stdin 失败: %v", err)
		}
	}
	writeLine(rpcLine(1, "initialize", `{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}`))
	writeLine(toolCallLine(2, "council_ask", `{"prompt":"慢问题","members":["c1"],"summary":"none","budget_usd":5}`))

	waitForFile(t, callsFile, 10*time.Second)
	start := time.Now()
	writeLine(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":2,"reason":"用户不要了"}}`)
	_ = pw.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, errOut.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("取消之后 15 秒仍未退出\nstderr:\n%s", errOut.String())
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("取消没有真正中断在飞工具：从发通知到退出用了 %v（假 provider 要睡 8s）", elapsed)
	}

	stdout := out.String()
	responses := parseResponses(t, stdout)
	if len(responses) != 1 {
		t.Fatalf("被取消的请求不应回包，实际 stdout:\n%s", stdout)
	}
	if string(responses[0].ID) != "1" {
		t.Errorf("唯一响应应当属于 initialize，实际 id=%s", responses[0].ID)
	}
	if !strings.Contains(errOut.String(), "取消请求") {
		t.Errorf("stderr 里应记录取消，实际:\n%s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "不再回响应") {
		t.Errorf("stderr 里应说明不发送响应，实际:\n%s", errOut.String())
	}
}

func waitForFile(t *testing.T, path string, limit time.Duration) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等不到 provider 启动（计数文件 %s 没有内容）", path)
}

// TestCouncilWho_走正式装配的默认关闭 覆盖第四个工具在真实 app.Run 装配下
// 的默认行为：配置里没有 local_agent，council_who 只回一句「未开启」，
// 不枚举进程、不打开任何额外东西。识别逻辑与 observe 输出由同包的 who_test.go
// 用注入的假枚举器钉住（那里才能确定性断言行内容）。
func TestCouncilWho_默认关闭只回说明(t *testing.T) {
	cfgPath, _ := writeMCPConfig(t, false)

	stdout, stderr, err := runServerWithLines(t, cfgPath, toolCallLine(1, "council_who", `{}`))
	if err != nil {
		t.Fatalf("app.Run 返回错误: %v\nstderr:\n%s", err, stderr)
	}
	responses := parseResponses(t, stdout)
	if len(responses) != 1 {
		t.Fatalf("应当有 1 条响应，实际 %d: %s", len(responses), stdout)
	}
	result := decodeToolResult(t, responseByID(t, responses, 1))
	if result.IsError {
		t.Fatalf("默认关闭不是错误，实际 isError: %s", onlyText(t, result))
	}
	text := onlyText(t, result)
	if !strings.Contains(text, "未开启") {
		t.Errorf("默认关闭时应当说明未开启，实际:\n%s", text)
	}
	if strings.Contains(text, "pid=") {
		t.Errorf("默认关闭时不该出现进程行，实际:\n%s", text)
	}
}
