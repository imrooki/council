package main

// serve_stdin_test.go 钉住 S3 的 Go 侧：桌面版把 stdin 当生命周期管道，
// 关掉写端之后 `council serve` 必须走优雅退出，而不是等下一次 Ctrl-C 或
// 被 TerminateProcess 硬杀。这里起真实的测试二进制子进程，从 stdout 读到
// 监听地址后再关 stdin，断言退出码 0。

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/store"
)

const (
	serveStdinChildGate   = "COUNCIL_TEST_SERVE_STDIN_CHILD"
	serveStdinChildAddr   = "COUNCIL_TEST_SERVE_STDIN_ADDR"
	serveStdinChildConfig = "COUNCIL_TEST_SERVE_STDIN_CONFIG"
)

// TestServeStdinEOFChild 是子进程入口，不是正常用例。
func TestServeStdinEOFChild(t *testing.T) {
	if os.Getenv(serveStdinChildGate) != "1" {
		t.Skip("子进程专用：由 serve stdin EOF 用例用 -test.run 拉起")
	}
	args := []string{
		"serve",
		"-addr", os.Getenv(serveStdinChildAddr),
		"-config", os.Getenv(serveStdinChildConfig),
	}
	// run 会阻塞到 stdin EOF；这里不返回。
	os.Exit(run(args))
}

func TestServeExitsWhenStdinCloses(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	// 只读 serve 要求库已经存在；先建一个空库再关掉。
	st, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("准备测试库失败: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关闭测试库失败: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("data_dir = '"+dataDir+"'\n"), 0o644); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("取测试二进制路径失败: %v", err)
	}
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("造 stdin 管道失败: %v", err)
	}
	defer stdinW.Close()

	cmd := exec.Command(exe, "-test.run", "^TestServeStdinEOFChild$")
	cmd.Stdin = stdinR
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("接 stdout 失败: %v", err)
	}
	stderrFile, err := os.Create(filepath.Join(dir, "child.stderr.log"))
	if err != nil {
		t.Fatalf("建 stderr 文件失败: %v", err)
	}
	cmd.Stderr = stderrFile
	cmd.Env = serveStdinChildEnv(t, dir, cfgPath)

	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 serve 子进程失败: %v", err)
	}
	// 父进程关掉读端，只留子进程手里的那份。
	_ = stdinR.Close()
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		_ = stderrFile.Close()
	})

	urlCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "council serve: http://") {
				select {
				case urlCh <- strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "council serve: ")):
				default:
				}
			}
		}
	}()

	select {
	case <-urlCh:
	case <-time.After(15 * time.Second):
		stderr, _ := os.ReadFile(filepath.Join(dir, "child.stderr.log"))
		t.Fatalf("15 秒内没有读到启动行；stderr:\n%s", stderr)
	}

	// 关掉写端 = 桌面版退出。
	if err := stdinW.Close(); err != nil {
		t.Fatalf("关 stdin 写端失败: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			stderr, _ := os.ReadFile(filepath.Join(dir, "child.stderr.log"))
			t.Fatalf("stdin EOF 之后 serve 退出码非 0: %v；stderr:\n%s", err, stderr)
		}
	case <-time.After(15 * time.Second):
		stderr, _ := os.ReadFile(filepath.Join(dir, "child.stderr.log"))
		t.Fatalf("stdin EOF 之后 15 秒 serve 还没退出；stderr:\n%s", stderr)
	}
}

func serveStdinChildEnv(t *testing.T, base, cfgPath string) []string {
	t.Helper()
	drop := map[string]bool{
		"COUNCIL_CONFIG":          true,
		"COUNCIL_DATA_DIR":        true,
		"COUNCIL_GLOBAL_RULES":    true,
		"HOME":                    true,
		"USERPROFILE":             true,
		serveStdinChildGate:       true,
		serveStdinChildAddr:       true,
		serveStdinChildConfig:     true,
		serveExitOnStdinEOFEnv:    true,
		"COUNCIL_LANG":            true,
	}
	env := make([]string, 0, len(os.Environ())+8)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || drop[strings.ToUpper(key)] {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		serveStdinChildGate+"=1",
		serveStdinChildAddr+"=127.0.0.1:0",
		serveStdinChildConfig+"="+cfgPath,
		serveExitOnStdinEOFEnv+"=1",
		"COUNCIL_GLOBAL_RULES="+filepath.Join(base, "no-global-rules.md"),
		"HOME="+filepath.Join(base, "home"),
		"USERPROFILE="+filepath.Join(base, "home"),
	)
	return env
}
