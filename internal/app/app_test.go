package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/config"
)

// testAppTimeout 是单个 Run 允许的最长阻塞时间。超过即判定没有按契约返回。
const testAppTimeout = 3 * time.Second

// testBlockingReader 在测试结束前既不产出数据也不返回 EOF。
// 配置出错的用例用它当 In：实现若错误地进了 REPL，Run 会挂住并被看门狗判失败。
type testBlockingReader struct {
	release <-chan struct{}
}

func (r testBlockingReader) Read(p []byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

// newTestBlockingReader 返回一个永久阻塞的 reader，测试结束时自动解除阻塞，避免 goroutine 泄漏。
func newTestBlockingReader(t *testing.T) io.Reader {
	t.Helper()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	return testBlockingReader{release: release}
}

// isolateAppEnv 把两个环境变量清成空串（规格 C：空串视为未设置），
// 免得跑测试的机器上真有 COUNCIL_CONFIG / COUNCIL_DATA_DIR 泄进用例。
// 注意用了 t.Setenv 的测试不能 t.Parallel。
func isolateAppEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, "")
}

// tomlLiteral 把路径写成 TOML 字面字符串。Windows 路径里的反斜杠在普通字符串里会被当转义。
func tomlLiteral(s string) string {
	return "'" + s + "'"
}

// validConfigBody 生成一份能通过全部校验的配置文本。dataDir 为空时整个 data_dir 键都不写。
func validConfigBody(dataDir string) string {
	var b strings.Builder
	if dataDir != "" {
		fmt.Fprintf(&b, "data_dir = %s\n\n", tomlLiteral(dataDir))
	}
	b.WriteString(`[providers.claude]
command = "claude"

[agents.theory]
display_name = "Theory"
provider = "claude"
model = "opus"
effort = "high"
role = "只做一阶剪切变形理论的推导与校核"
permissions = "read"
mode = "discussion"

[teams.project]
display_name = "<project> 讨论组"
members = ["theory"]
`)
	return b.String()
}

// writeTestConfig 把 body 写进 dir 下的 config.toml，返回完整路径。
func writeTestConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写测试配置失败：%v", err)
	}
	return path
}

// runAppForTest 跑一次 Run，带看门狗：Run 不返回就直接判失败，不会把测试挂死。
func runAppForTest(t *testing.T, ctx context.Context, configPath string, in io.Reader) (string, string, error) {
	t.Helper()

	var out, errOut bytes.Buffer
	opts := Options{
		ConfigPath: configPath,
		In:         in,
		Out:        &out,
		Err:        &errOut,
		Verbose:    true,
		NoAuto:     true,
	}

	done := make(chan error, 1)
	go func() { done <- Run(ctx, opts) }()

	select {
	case err := <-done:
		return out.String(), errOut.String(), err
	case <-time.After(testAppTimeout):
		t.Fatalf("app.Run 在 %v 内没有返回", testAppTimeout)
		return "", "", nil
	}
}

// TestRunValidConfig 正常路径：合法配置、In 立刻 EOF，Run 返回 nil，
// 横幅里同时出现配置路径与解析出来的数据目录。
//
// 数据目录那条断言在 Step 5 翻了个面：Step 4 明确不创建任何目录，
// Step 5 起 store.Open 负责建（08 §D 与 08a 都写明这是一处有意改变）。
func TestRunValidConfig(t *testing.T) {
	isolateAppEnv(t)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := writeTestConfig(t, dir, validConfigBody(dataDir))

	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(""))
	if err != nil {
		t.Fatalf("合法配置下 Run 应当返回 nil，实际返回 %v；Err 输出：\n%s", err, errOut)
	}
	for _, want := range []string{cfgPath, dataDir} {
		if !strings.Contains(out, want) {
			t.Errorf("横幅里缺少 %q；Out 完整输出：\n%s", want, out)
		}
	}

	info, statErr := os.Stat(dataDir)
	if statErr != nil {
		t.Fatalf("Step 5 起应当创建数据目录 %s，实际: %v", dataDir, statErr)
	}
	if !info.IsDir() {
		t.Fatalf("%s 应当是一个目录", dataDir)
	}
	dbPath := filepath.Join(dataDir, testDBFileName)
	if _, statErr := os.Stat(dbPath); statErr != nil {
		t.Errorf("库文件 %s 应当在 Run 之后存在，实际: %v", dbPath, statErr)
	}
	if !containsPath(out, dbPath) {
		t.Errorf("横幅里应当含库文件路径 %q；Out 完整输出：\n%s", dbPath, out)
	}
}

// TestRunDataDirResolution 覆盖数据目录的两级来源：配置里的 data_dir 优先于环境变量。
func TestRunDataDirResolution(t *testing.T) {
	cases := []struct {
		name          string
		useCfgDataDir bool
		useEnvDataDir bool
		wantFromCfg   bool
	}{
		{name: "配置里的绝对路径优先", useCfgDataDir: true, useEnvDataDir: true, wantFromCfg: true},
		{name: "配置不给时用环境变量", useCfgDataDir: false, useEnvDataDir: true, wantFromCfg: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgDataDir := filepath.Join(dir, "data-from-config")
			envDataDir := filepath.Join(dir, "data-from-env")

			t.Setenv(config.EnvConfigPath, "")
			if tc.useEnvDataDir {
				t.Setenv(config.EnvDataDir, envDataDir)
			} else {
				t.Setenv(config.EnvDataDir, "")
			}

			body := validConfigBody("")
			if tc.useCfgDataDir {
				body = validConfigBody(cfgDataDir)
			}
			cfgPath := writeTestConfig(t, dir, body)

			out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(""))
			if err != nil {
				t.Fatalf("Run 应当返回 nil，实际返回 %v；Err 输出：\n%s", err, errOut)
			}

			want, other := envDataDir, cfgDataDir
			if tc.wantFromCfg {
				want, other = cfgDataDir, envDataDir
			}
			if !strings.Contains(out, want) {
				t.Errorf("横幅里缺少数据目录 %q；Out 完整输出：\n%s", want, out)
			}
			if tc.useCfgDataDir && tc.useEnvDataDir && strings.Contains(out, other) {
				t.Errorf("横幅里出现了优先级更低的数据目录 %q；Out 完整输出：\n%s", other, out)
			}
		})
	}
}

// TestRunConfigFailures 覆盖配置出错的两条路径：文件不存在、交叉校验不过。
// 两例都要求 Run 返回错误、错误细节写进 Err、并且不进 REPL。
// 「不进 REPL」的证据是 In 给了一个永不结束的 reader：真进了 REPL，Run 就会挂住被看门狗判失败。
func TestRunConfigFailures(t *testing.T) {
	// danglingProviderBody 引用了不存在的 provider，命中规格 D-3。
	// 仍然写上 data_dir，指向临时目录：实现若在校验失败之前就解析了数据目录，
	// 也不会落到默认的真实盘符路径上。
	danglingProviderBody := func(dataDir string) string {
		return fmt.Sprintf(`data_dir = %s

[providers.claude]
command = "claude"

[agents.ghost]
display_name = "Ghost"
provider = "no_such_provider"
model = "opus"
`, tomlLiteral(dataDir))
	}

	cases := []struct {
		name         string
		makeBody     func(dataDir string) string
		writeFile    bool
		wantErrParts func(cfgPath string) []string
	}{
		{
			name:      "配置文件不存在",
			writeFile: false,
			wantErrParts: func(cfgPath string) []string {
				return []string{cfgPath}
			},
		},
		{
			name:      "agent 引用了不存在的 provider",
			makeBody:  danglingProviderBody,
			writeFile: true,
			wantErrParts: func(cfgPath string) []string {
				return []string{"agents.", "ghost", "no_such_provider"}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// 环境变量也指向临时目录，保证无论实现按什么顺序解析，都不会碰到默认的真实盘符路径。
			t.Setenv(config.EnvConfigPath, "")
			t.Setenv(config.EnvDataDir, filepath.Join(dir, "data"))

			var cfgPath string
			if tc.writeFile {
				cfgPath = writeTestConfig(t, dir, tc.makeBody(filepath.Join(dir, "data")))
			} else {
				cfgPath = filepath.Join(dir, "no_such_config.toml")
			}

			_, errOut, err := runAppForTest(t, context.Background(), cfgPath, newTestBlockingReader(t))
			if err == nil {
				t.Fatalf("配置有问题时 Run 应当返回错误，实际返回 nil；Err 输出：\n%s", errOut)
			}
			for _, want := range tc.wantErrParts(cfgPath) {
				if !strings.Contains(errOut, want) {
					t.Errorf("Err 输出里缺少 %q；完整输出：\n%s", want, errOut)
				}
			}
		})
	}
}

// TestRunReturnsOnContextCancel 断言 ctx 取消后 Run 在有限时间内返回。
// In 是永不产出也永不结束的 reader，所以 Run 只可能因 ctx 取消而返回。
// 取消后的返回值契约没有规定（nil 还是 ctx.Err()），这里只断言返回，不断言值。
func TestRunReturnsOnContextCancel(t *testing.T) {
	isolateAppEnv(t)

	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, validConfigBody(filepath.Join(dir, "data")))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go cancel()

	_, _, err := runAppForTest(t, ctx, cfgPath, newTestBlockingReader(t))
	t.Logf("ctx 取消后 Run 返回：%v", err)
}
