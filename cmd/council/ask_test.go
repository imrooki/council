package main

// A10 的分派证明：从 run([]string{"ask", ...}) 进，确认 ask 子命令在
// 全局 flag 解析之前被拦下，stdout 只有一个 JSON，stdin 不是终端也不会
// 掉进行式 REPL。这里必须测 run 而不是 app.Run：分派发生在 main 这一层。

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"
)

func TestMain(m *testing.M) {
	if os.Getenv("COUNCIL_FAKE_CLI") == "1" {
		runAskFakeProviderCLI()
		return
	}
	os.Exit(m.Run())
}

func runAskFakeProviderCLI() {
	_, _ = io.ReadAll(os.Stdin)
	fmt.Fprint(os.Stdout, `{"type":"result","subtype":"success","is_error":false,`+
		`"terminal_reason":"completed","api_error_status":null,"stop_reason":"end_turn",`+
		`"session_id":"fake-session","result":"四","total_cost_usd":0.01,`+
		`"usage":{"input_tokens":2,"cache_creation_input_tokens":10,`+
		`"cache_read_input_tokens":0,"output_tokens":4,`+
		`"output_tokens_details":{"thinking_tokens":0}}}`)
	os.Exit(0)
}

// runAskTestConfig 落一份最小配置，provider 指向测试二进制自己。
func runAskTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	body := fmt.Sprintf(`data_dir = '%s'

[providers.claude]
command = '%s'

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "分派测试成员"
permissions = "read"
mode = "discussion"
`, dataDir, os.Args[0])
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	return cfgPath
}

// captureRun 把进程标准流换成临时文件，跑一次 fn，再读回来。
func captureRun(t *testing.T, fn func() int) (stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	outFile, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatalf("建 stdout 临时文件失败: %v", err)
	}
	errFile, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatalf("建 stderr 临时文件失败: %v", err)
	}
	inFile, err := os.Create(filepath.Join(dir, "stdin"))
	if err != nil {
		t.Fatalf("建 stdin 临时文件失败: %v", err)
	}
	if _, err := inFile.WriteString("/help\n/quit\n"); err != nil {
		t.Fatalf("写 stdin 临时文件失败: %v", err)
	}
	if _, err := inFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("倒回 stdin 失败: %v", err)
	}

	savedOut, savedErr, savedIn := os.Stdout, os.Stderr, os.Stdin
	os.Stdout, os.Stderr, os.Stdin = outFile, errFile, inFile
	defer func() {
		os.Stdout, os.Stderr, os.Stdin = savedOut, savedErr, savedIn
		_ = outFile.Close()
		_ = errFile.Close()
		_ = inFile.Close()
	}()

	code = fn()
	if _, err := outFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("读 stdout 失败: %v", err)
	}
	if _, err := errFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("读 stderr 失败: %v", err)
	}
	outBytes, _ := io.ReadAll(outFile)
	errBytes, _ := io.ReadAll(errFile)
	return string(outBytes), string(errBytes), code
}

// TestAskDispatch_stdin不是终端 是 A10：run 识别 ask、stdin 不是终端，
// stdout 仍然只有一个 JSON。
func TestAskDispatch_stdin不是终端(t *testing.T) {
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	cfgPath := runAskTestConfig(t)

	stdout, stderr, code := captureRun(t, func() int {
		return run([]string{"ask", "-p", "1+1", "-members", "c1", "-json", "-config", cfgPath})
	})
	if code != exitOK {
		t.Fatalf("run(ask ...) = %d，期望 %d；stderr:\n%s", code, exitOK, stderr)
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	var res map[string]any
	if err := dec.Decode(&res); err != nil {
		t.Fatalf("stdout 不是 JSON: %v；完整输出:\n%s", err, stdout)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout 应当只有一个 JSON 对象，后面还有内容；完整输出:\n%s", stdout)
	}
	if res["status"] != "ok" {
		t.Errorf("status 应为 ok，实际 %v；stderr:\n%s", res["status"], stderr)
	}
	turns, _ := res["turns"].([]any)
	if len(turns) != 1 {
		t.Fatalf("turns 应恰好 1 条，实际 %v", res["turns"])
	}
	first, _ := turns[0].(map[string]any)
	if text, _ := first["text"].(string); text == "" {
		t.Errorf("turns[0].text 不应为空")
	}
	if strings.Contains(stdout, "配置文件:") || strings.Contains(stdout, "/help") {
		t.Errorf("stdout 不应有横幅或 stdin 内容，实际:\n%s", stdout)
	}
	if strings.Contains(stderr, "council>") {
		t.Errorf("不应出现行式 REPL 提示符，stderr:\n%s", stderr)
	}
}

// TestAskDispatch_help 确认 ask 的 -h 退 0 且用法写在 stderr。
func TestAskDispatch_help(t *testing.T) {
	stdout, stderr, code := captureRun(t, func() int {
		return run([]string{"ask", "-h"})
	})
	if code != exitOK {
		t.Fatalf("run(ask -h) = %d，期望 %d", code, exitOK)
	}
	if stdout != "" {
		t.Errorf("-h 的 stdout 应为空，实际:\n%s", stdout)
	}
	for _, want := range []string{"用法:", "-members", "-json"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("ask 用法里缺少 %q，实际:\n%s", want, stderr)
		}
	}
}

// TestRunStillRejectsPositional 对照：ask 之外的 `council foo` 行为不变。
func TestRunStillRejectsPositional(t *testing.T) {
	_, stderr, code := captureRun(t, func() int { return run([]string{"foo"}) })
	if code != exitUsage {
		t.Errorf("run(foo) = %d，期望 %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "不接受位置参数") {
		t.Errorf("stderr 应说明不接受位置参数，实际:\n%s", stderr)
	}
}
