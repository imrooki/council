package app

// obx_firstrun_app_test.go 是 OB1（00_DECISIONS.md #48，首次运行自动配置）在 app 包的验收：
// 契约 §2.0 的共用装置与夹具、§2.3 的 A0-A15。
//
// 纪律（契约 §2 开头）：
//   - 屏幕输出与文件字节整段或整行 == 比较；只有 A5、A13 里操作系统的错误文本不比。
//   - 会走到默认配置位置、或会检测本机工具的用例，第一件事是 obxGuardRealHome（经 obxIsolate），
//     然后才 t.Setenv；COUNCIL_DATA_DIR 必须在 isolateAppEnv 之后设（顺序反了会回落到内置默认的数据目录）。
//   - 另起 goroutine 调 Run 的一律经 obxRun / obxRunTee（A11 直接用 obxDrain），t.Fatalf 只在测试 goroutine 里调。
//   - 假 CLI 一律是临时 bin 目录里只用 cmd 内建命令的 .cmd，不经 TestMain，不调用任何真 CLI。

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/repl"
)

// ---------------------------------------------------------------------------
// §2.0 共用装置
// ---------------------------------------------------------------------------

// obxHomeStat 是 obxStatHome 的结果：可以用 == 比较。只来自 os.Stat，不打开文件、不读内容。
type obxHomeStat struct {
	dirExists  bool  // <home>\.council 是目录
	fileExists bool  // <home>\.council\config.toml 存在
	size       int64 // config.toml 的 Size()
	modTime    int64 // config.toml 的 ModTime()，纳秒
}

// obxStatHome 只 os.Stat <home>\.council 与 <home>\.council\config.toml。
func obxStatHome(home string) obxHomeStat {
	var s obxHomeStat
	if fi, err := os.Stat(filepath.Join(home, ".council")); err == nil && fi.IsDir() {
		s.dirExists = true
	}
	if fi, err := os.Stat(filepath.Join(home, ".council", "config.toml")); err == nil {
		s.fileExists = true
		s.size = fi.Size()
		s.modTime = fi.ModTime().UnixNano()
	}
	return s
}

// obxGuardRealHome 在任何 t.Setenv 之前取 os.UserHomeDir()（此刻是真用户目录），记下 obxStatHome；
// t.Cleanup 里再取一次，不相等就报错。它是最后一道：挡住写真目录的是环境隔离与 COUNCIL_DATA_DIR 的顺序。
// 真配置已存在的机器上它只看得出「被改了」，看不出「被建了」；正对照是 A0。
func obxGuardRealHome(t *testing.T) {
	t.Helper()
	real, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("前提不成立：取不到用户主目录：%v", err)
	}
	before := obxStatHome(real)
	t.Cleanup(func() {
		if after := obxStatHome(real); after != before {
			t.Errorf("用户主目录 %s 下的 .council 被动过：之前 %+v，之后 %+v", real, before, after)
		}
	})
}

// obxEnv 是 obxIsolate 建出来的隔离环境。
type obxEnv struct {
	root    string // t.TempDir()
	home    string // <root>\home，USERPROFILE 与 HOME
	appdata string // <root>\appdata，APPDATA
	local   string // <root>\local，LOCALAPPDATA
	bin     string // <root>\bin，PATH 只有它
	fx      string // <root>\fx，夹具
	cfgPath string // <home>\.council\config.toml
}

// obxIsolate 先守真用户目录；建 home、appdata、local、bin、fx 五个目录（data 不预建）；
// isolateAppEnv；四个 profile 变量与 PATH 指到隔离目录；最后才设 COUNCIL_DATA_DIR 为 <root>\data。
func obxIsolate(t *testing.T) *obxEnv {
	t.Helper()
	obxGuardRealHome(t)
	root := t.TempDir()
	e := &obxEnv{
		root:    root,
		home:    filepath.Join(root, "home"),
		appdata: filepath.Join(root, "appdata"),
		local:   filepath.Join(root, "local"),
		bin:     filepath.Join(root, "bin"),
		fx:      filepath.Join(root, "fx"),
	}
	for _, d := range []string{e.home, e.appdata, e.local, e.bin, e.fx} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("前提不成立：建目录 %s 失败：%v", d, err)
		}
	}
	isolateAppEnv(t)
	t.Setenv("USERPROFILE", e.home)
	t.Setenv("HOME", e.home)
	t.Setenv("APPDATA", e.appdata)
	t.Setenv("LOCALAPPDATA", e.local)
	t.Setenv("PATH", e.bin)
	// 必须在 isolateAppEnv 之后：它把 COUNCIL_DATA_DIR 清成空串。
	t.Setenv(config.EnvDataDir, filepath.Join(root, "data"))
	e.cfgPath = filepath.Join(e.home, ".council", "config.toml")
	return e
}

// obxFake 写 <dir>\<id>.cmd：各行以 \r\n 连接，末尾再加 \r\n。返回路径。
func obxFake(t *testing.T, dir, id string, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, id+".cmd")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0o644); err != nil {
		t.Fatalf("前提不成立：写假 CLI %s 失败：%v", p, err)
	}
	return p
}

// obxFixture 写 <fx>\<name>，返回路径。
func obxFixture(t *testing.T, e *obxEnv, name, content string) string {
	t.Helper()
	p := filepath.Join(e.fx, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("前提不成立：写夹具 %s 失败：%v", p, err)
	}
	return p
}

// obxTypeFake 写夹具 <fx>\<id>.out，再写只有一行 @type "<夹具绝对路径>" 的假 CLI。
func obxTypeFake(t *testing.T, e *obxEnv, id, content string) string {
	t.Helper()
	fx := obxFixture(t, e, id+".out", content)
	return obxFake(t, e.bin, id, `@type "`+fx+`"`)
}

// obxEntries 是 os.ReadDir 的名字列表。
func obxEntries(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录 %s 失败：%v", dir, err)
	}
	names := make([]string, 0, len(ents))
	for _, d := range ents {
		names = append(names, d.Name())
	}
	return names
}

// obxSub 只把 <home>、<bin> 换成真路径；<标题>、<成员 id>、<path> 这类原样保留。
func obxSub(e *obxEnv, s string) string {
	s = strings.ReplaceAll(s, "<home>", e.home)
	return strings.ReplaceAll(s, "<bin>", e.bin)
}

// obxGenData 是生成的配置里写的 data_dir：<home>\.council\data（v1.2）。
func obxGenData(e *obxEnv) string {
	return filepath.Join(e.home, ".council", "data")
}

// obxWantFile 对 claude、grok、agy、codex 依次造 DetectedProvider，ids 里有的 Path 为 <bin>\<id>.cmd，
// 其余为空，返回 config.FirstRunTOML(obxGenData(e), 那四项)。
// 注意它是自引用的：只钉「写进文件的正是 FirstRunTOML(检测结果)」；FirstRunTOML 自己的字节由
// config 包的 C3 用字面量钉，data_dir 那一行由 A1 的 LoadAndValidate 断言钉。
func obxWantFile(e *obxEnv, ids ...string) string {
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}
	var found []config.DetectedProvider
	for _, id := range []string{"claude", "grok", "agy", "codex"} {
		d := config.DetectedProvider{ID: id}
		if have[id] {
			d.Path = filepath.Join(e.bin, id+".cmd")
		}
		found = append(found, d)
	}
	return config.FirstRunTOML(obxGenData(e), found)
}

// obxProjectLine 与 app.Run 同一口径：测试进程的工作目录就是启动目录，规范化后再写进横幅。
// 这些用例的库里没有会话，所以本项目是 0，没有「另有」那一行。
func obxProjectLine() string {
	if wd, err := os.Getwd(); err == nil {
		return "项目目录: " + repl.NormalizeProjectDir(wd) + "（本项目 0 个会话，/resume 列出并接上）\n"
	}
	return "项目目录: 未知（/resume 将列出全部会话）\n"
}

// obxBanner 是行式分支只敲 /quit 时 out 的全文（数据目录是生成的配置里写的那一个）。
func obxBanner(e *obxEnv) string {
	gen := obxGenData(e)
	return "配置文件: " + e.cfgPath + "\n数据目录: " + gen + "\n库文件: " + filepath.Join(gen, "council.db") +
		" (已有 0 个会话)\n" + obxProjectLine() + "已加载 0 个 agent，0 个 team\n输入 /help 查看可用命令，/quit 退出。\n\ncouncil> 再见\n"
}

// obxReadFile 读整个文件；读不出来就 Fatal。
func obxReadFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", p, err)
	}
	return string(b)
}

// obxNotExist 断言 p 不存在（os.Stat 的错误是 NotExist）。
func obxNotExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("%s 不该存在，Stat 结果: %v", p, err)
	}
}

// obxSetTimeout 把 firstRunTimeout 改成 d，t.Cleanup 还原。必须在 obxRun 之前调：
// 这样还原排在 obxDrain 之后，Run 返回之前一直看到的是 d。
func obxSetTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := firstRunTimeout
	firstRunTimeout = d
	t.Cleanup(func() { firstRunTimeout = old })
}

// obxIn 是行式 Run 的输入。
func obxIn(s string) io.Reader { return strings.NewReader(s) }

// ---------------------------------------------------------------------------
// §2.0 收尾（v1.1，F0；逐字）
// ---------------------------------------------------------------------------

// obxRunTimeout 是 obxRun 的看门狗；obxDrainTimeout 是收尾时 cancel 之后等 Run 返回的上限。
const (
	obxRunTimeout   = 30 * time.Second
	obxDrainTimeout = 10 * time.Second
)

// obxDrain 是 obxRun 与 A11 共用的收尾：先 cancel()，再最多等 obxDrainTimeout 让 Run 返回；
// 等不到就 os.Exit 结束整个测试进程。不用 panic：testing 在 cleanup panic 之后照样跑完
// 其余 cleanup（还原环境变量、删临时目录）才退出，还没返回的 Run 正好在这个窗口里读到
// 还原后的环境（v1.1 探针实测）；os.Exit 之后什么都不再执行。
func obxDrain(cancel context.CancelFunc, finished <-chan struct{}, who string) {
	cancel()
	select {
	case <-finished:
	case <-time.After(obxDrainTimeout):
		fmt.Fprintf(os.Stderr, "%s：cancel() 之后 %v Run 仍没有返回；为了不让它在环境变量还原之后接着写盘，直接结束测试进程\n", who, obxDrainTimeout)
		os.Exit(3)
	}
}

// obxRun 把 Out、Err 换成两个 strings.Builder，另起 goroutine 调 Run，返回 (out, errOut, err)。
// 交给 Run 的 ctx 由 ctx 派生、可取消；本函数在内部注册一个 t.Cleanup 调 obxDrain。它比
// obxIsolate 晚注册，按后进先出先于环境变量还原与删临时目录执行，用例体里任何 Fatal 路径
// 都盖得到。看门狗只在测试 goroutine 里 select，不在别的 goroutine 里调 t.Fatalf。
func obxRun(t *testing.T, ctx context.Context, opts Options) (string, string, error) {
	t.Helper()
	return obxRunTee(t, ctx, opts, nil)
}

// obxRunTee 同 obxRun；tee 非 nil 时 Run 的错误出口同时写 tee（A12 用它看「正在检查」何时写出）。
func obxRunTee(t *testing.T, ctx context.Context, opts Options, tee io.Writer) (string, string, error) {
	t.Helper()
	var out, errOut strings.Builder
	opts.Out = &out
	opts.Err = &errOut
	if tee != nil {
		opts.Err = io.MultiWriter(&errOut, tee)
	}
	rctx, cancel := context.WithCancel(ctx)
	finished := make(chan struct{})
	var runErr error
	go func() {
		defer close(finished)
		runErr = Run(rctx, opts)
	}()
	t.Cleanup(func() { obxDrain(cancel, finished, t.Name()) })
	select {
	case <-finished:
		return out.String(), errOut.String(), runErr
	case <-time.After(obxRunTimeout):
		t.Fatalf("Run %v 没有返回", obxRunTimeout)
		return "", "", nil
	}
}

// ---------------------------------------------------------------------------
// §2.0 等待循环、假 codex、夹具、期望块（逐字）
// ---------------------------------------------------------------------------

// obxLoopLimit 是 A8、A9、A12 假 claude 等待循环的计数上限：本机单独实测 cmd 内建的这种循环
// 每秒约 3400 次（60000 次约 18 s），go test 里 M33 下 A8 实测 21.25 s 才数到上限；给 1 s、3 s 的超时留足余量，
// 又保证循环一定会停。
const obxLoopLimit = "60000"

// obxLoopLines 是一个会自己停的等待循环：cond 为真时继续等，计数到 obxLoopLimit 就以 0 退出。
func obxLoopLines(cond string, then ...string) []string {
	lines := []string{"@echo off", "set n=0", ":obxloop", "set /a n+=1", "if %n% geq " + obxLoopLimit + " exit /b 0", "if " + cond + " goto obxloop"}
	return append(lines, then...)
}

// obxCodexFake 写假 codex：第一个参数是 login 时（codex login status）跑 status 那几行，
// 否则（codex debug models）跑 models 那几行、以退出码 0 结束。
func obxCodexFake(t *testing.T, e *obxEnv, models, status []string) string {
	t.Helper()
	lines := []string{`@if "%1"=="login" goto obxlogin`}
	lines = append(lines, models...)
	lines = append(lines, "@exit /b 0", ":obxlogin")
	lines = append(lines, status...)
	return obxFake(t, e.bin, "codex", lines...)
}

const (
	obxClaudeOut        = `{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[{"value":"default","resolvedModel":"claude-obx-1"},{"value":"obx-a","resolvedModel":"claude-obx-a","supportedEffortLevels":["low","high"]},{"value":"obx-b","resolvedModel":"claude-obx-b"}],"account":{"apiProvider":"firstParty","subscriptionType":"Claude Obx"}}}}` + "\n"
	obxClaudeNoLoginOut = `{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[{"value":"default","resolvedModel":"claude-obx-1"},{"value":"obx-a","resolvedModel":"claude-obx-a","supportedEffortLevels":["low","high"]},{"value":"obx-b","resolvedModel":"claude-obx-b"}],"account":{"tokenSource":"none","apiProvider":"firstParty"}}}}` + "\n"
	obxGrokOut          = "You are not authenticated.\n\nDefault model: grok-obx-1\n\nAvailable models:\n  * grok-obx-1 (default)\n  - grok-obx-2\n  - grok-obx-3\n"
	obxGrokKeyOut       = "Model 'grok-obx-1' is using its own API key.\n\nDefault model: grok-obx-1\n\nAvailable models:\n  * grok-obx-1 (default)\n"
	obxCodexOut         = `{"models":[{"slug":"gpt-obx","visibility":"list","supported_reasoning_levels":[{"effort":"high"}]},{"slug":"custom/obx","visibility":"list"},{"slug":"obx-hidden","visibility":"hide"}]}` + "\n"
	obxAgyOut           = `{"status":"SUCCESS","command":{"name":"models","data":{"models":[{"id":"a","label":"Gemini Obx"}]}}}` + "\n"
)

var (
	obxCodexLoggedIn = []string{"@echo Logged in using ChatGPT 1>&2", "@exit /b 0"}
	obxCodexNotIn    = []string{"@echo Not logged in 1>&2", "@exit /b 1"}
)

// obxThreeFakes 装 claude（已登录，订阅 Claude Obx）、grok（没登录）、codex（已登录），agy 不装。
func obxThreeFakes(t *testing.T, e *obxEnv) {
	t.Helper()
	obxTypeFake(t, e, "claude", obxClaudeOut)
	obxTypeFake(t, e, "grok", obxGrokOut)
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, obxCodexLoggedIn)
}

const obxHead = `第一次运行：还没有配置文件 <home>\.council\config.toml，council 来生成一份。
`

const obxChecking = "正在检查 claude、grok、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"

const obxRowsOK = `  claude  <bin>\claude.cmd  识别到 2 个模型；已登录（订阅：Claude Obx）
  grok    <bin>\grok.cmd  识别到 3 个模型；没登录
  agy     没找到
  codex   <bin>\codex.cmd  识别到 1 个模型；已登录
能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：
  grok    grok login
`

const obxDone = `已生成配置文件: <home>\.council\config.toml（只写了找到的工具，还没有成员）
上手三步：
  1. /new <标题>  开一个会话
  2. /add  看识别出的模型，再用 /add +编号 挑成员
  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个

`

const obxWantNone = `第一次运行：还没有配置文件 <home>\.council\config.toml，council 来生成一份。
  claude  没找到
  grok    没找到
  agy     没找到
  codex   没找到
一个都没找到，没有生成配置文件。至少装一个、在终端里登录好，再运行 council：
  claude  npm install -g @anthropic-ai/claude-code，装好后运行 claude auth login
  grok    照 grok 官方的说明安装，装好后运行 grok login
  agy     照 agy 官方的说明安装，装好后运行 agy，按提示登录
  codex   npm install -g @openai/codex，装好后运行 codex login
council 找的是 PATH，以及这几处默认安装位置：%APPDATA%\npm、%USERPROFILE%\.grok\bin、%LOCALAPPDATA%\agy\bin
`

const obxLoginHead = "能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：\n"

// obxStep3Auto / obxStep3NoAuto 是上手三步第 3 步的两句（§3.2），A2 用来从 obxDone 换出 NoAuto 的期望。
const (
	obxStep3Auto   = "  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个\n"
	obxStep3NoAuto = "  3. 直接打字只记下（你用了 -no-auto）；/auto on 之后，直接打字会让会话里的成员回复\n"
)

// obxMarkWriter 一看到「正在检查」就建标记文件（A12）。
type obxMarkWriter struct {
	mu   sync.Mutex
	path string
	buf  strings.Builder
	done bool
}

func (w *obxMarkWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if !w.done && strings.Contains(w.buf.String(), "正在检查") {
		w.done = true
		if err := os.WriteFile(w.path, []byte("x"), 0o644); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// ---------------------------------------------------------------------------
// §2.3 用例
// ---------------------------------------------------------------------------

// TestObxGuardSeesCreation 是 A0：守卫的正对照。对一个临时 home，建 .council\config.toml 前后两次
// obxStatHome 必须不等，而且之后 dirExists、fileExists 都为真。
// 【判别力】obxStatHome 若不看这两样（例如永远返回零值），前后相等，红。
func TestObxGuardSeesCreation(t *testing.T) {
	home := t.TempDir()
	before := obxStatHome(home)
	if err := os.MkdirAll(filepath.Join(home, ".council"), 0o755); err != nil {
		t.Fatalf("前提不成立：建 .council 失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".council", "config.toml"), []byte("# obx\n"), 0o644); err != nil {
		t.Fatalf("前提不成立：写 config.toml 失败：%v", err)
	}
	after := obxStatHome(home)
	if before == after {
		t.Errorf("建了 .council\\config.toml 之后 obxStatHome 没变：%+v", after)
	}
	if !after.dirExists || !after.fileExists {
		t.Errorf("建了之后 dirExists、fileExists 都应为真：%+v", after)
	}
}

// TestObxFirstRunEndToEnd 是 A1（E2 在同一个用例里）：真 app.Run、行式、端到端。
// 【判别力】三档里的「已登录（带订阅）」「没登录」「已登录（不带订阅）」都在这一条里：M56、M57、M58、M89、M92；
// M101、M103（DataDir 断言与横幅）。E2 的「errOut 为空」以第一次的非空 errOut 为正对照。
func TestObxFirstRunEndToEnd(t *testing.T) {
	e := obxIsolate(t)
	obxThreeFakes(t, e)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s", err, errOut)
	}
	if want := obxSub(e, obxHead+obxChecking+obxRowsOK+obxDone); errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	file := obxReadFile(t, e.cfgPath)
	if want := obxWantFile(e, "claude", "grok", "codex"); file != want {
		t.Errorf("配置文件字节不等\n实际 %q\n期望 %q", file, want)
	}
	if cfg, lerr := config.LoadAndValidate(e.cfgPath); lerr != nil {
		t.Errorf("生成的配置 LoadAndValidate 失败：%v", lerr)
	} else if cfg.DataDir != obxGenData(e) {
		t.Errorf("生成的配置里 DataDir = %q，期望 %q", cfg.DataDir, obxGenData(e))
	}
	if want := obxBanner(e); out != want {
		t.Errorf("out 不等\n实际 %q\n期望 %q", out, want)
	}

	// E2：同一环境再跑一次，不再触发首次运行。
	out2, errOut2, err2 := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err2 != nil {
		t.Fatalf("E2：第二次 Run 返回 %v；errOut:\n%s", err2, errOut2)
	}
	if errOut2 != "" {
		t.Errorf("E2：第二次 errOut 应为空，实际 %q", errOut2)
	}
	if want := obxBanner(e); out2 != want {
		t.Errorf("E2：第二次 out 不等\n实际 %q\n期望 %q", out2, want)
	}
	if file2 := obxReadFile(t, e.cfgPath); file2 != file {
		t.Errorf("E2：第二次之后配置文件变了\n之前 %q\n之后 %q", file, file2)
	}
}

// TestObxFirstRunNoAuto 是 A2：同 A1 但 NoAuto；只差第 3 步那一句。与 A1 互为对照。
// 【判别力】M30（noAuto 那一支永远不走）。
func TestObxFirstRunNoAuto(t *testing.T) {
	if !strings.Contains(obxDone, obxStep3Auto) {
		t.Fatalf("前提不成立：obxDone 里没有第 3 步的对话模式那一句，换不出 NoAuto 的期望")
	}
	e := obxIsolate(t)
	obxThreeFakes(t, e)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n"), NoAuto: true})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s", err, errOut)
	}
	done := strings.Replace(obxDone, obxStep3Auto, obxStep3NoAuto, 1)
	if want := obxSub(e, obxHead+obxChecking+obxRowsOK+done); errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if file, want := obxReadFile(t, e.cfgPath), obxWantFile(e, "claude", "grok", "codex"); file != want {
		t.Errorf("配置文件字节不等\n实际 %q\n期望 %q", file, want)
	}
	if want := obxBanner(e); out != want {
		t.Errorf("out 不等\n实际 %q\n期望 %q", out, want)
	}
}

// TestObxFirstRunNoneFound 是 A3：一个假 CLI 都不装。
// 【判别力】M31（一家都没找到时返回 nil）；不建 .council 以 A1 为正对照。
func TestObxFirstRunNoneFound(t *testing.T) {
	e := obxIsolate(t)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err == nil {
		t.Errorf("一家都没找到时 Run 应当返回错误")
	}
	if want := obxSub(e, obxWantNone); errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if out != "" {
		t.Errorf("out 应为空，实际 %q", out)
	}
	obxNotExist(t, filepath.Join(e.home, ".council"))
}

// TestObxFirstRunExplicitMissing 是 A4：本机「装了」三家、默认位置也没有文件，只是显式指定了一个不存在的路径。
// 与 HEAD 逐字节相同地报错，不进首次运行。A1 是它的正对照（只差显式指定）。
// 【判别力】M48（env）、M49（flag）、M50（两个都红）。
func TestObxFirstRunExplicitMissing(t *testing.T) {
	for _, mode := range []string{"flag", "env"} {
		t.Run(mode, func(t *testing.T) {
			e := obxIsolate(t)
			obxThreeFakes(t, e)
			missing := filepath.Join(e.root, "typo.toml")
			opts := Options{In: obxIn("/quit\n")}
			if mode == "flag" {
				opts.ConfigPath = missing
			} else {
				t.Setenv(config.EnvConfigPath, missing)
			}

			out, errOut, err := obxRun(t, context.Background(), opts)
			if err == nil {
				t.Errorf("显式路径找不到时 Run 应当返回错误")
			}
			want := "找不到配置文件: " + missing + "\n这个路径是你用 --config 或环境变量 COUNCIL_CONFIG 指定的，请确认它没写错。\n"
			if errOut != want {
				t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
			}
			if out != "" {
				t.Errorf("out 应为空，实际 %q", out)
			}
			obxNotExist(t, missing)
			obxNotExist(t, filepath.Join(e.home, ".council"))
		})
	}
}

// TestObxFirstRunWriteFails 是 A5：<home>\.council 是一个普通文件，建不了目录。
// 【判别力】M43（不打本来要写的内容）、M44（返回 nil）。
func TestObxFirstRunWriteFails(t *testing.T) {
	e := obxIsolate(t)
	obxThreeFakes(t, e)
	blocker := filepath.Join(e.home, ".council")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("前提不成立：写挡路的文件失败：%v", err)
	}

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err == nil {
		t.Errorf("写不进配置文件时 Run 应当返回错误")
	}
	prefix := obxSub(e, obxHead+obxChecking+obxRowsOK) + "写不进配置文件 " + e.cfgPath + ": "
	if !strings.HasPrefix(errOut, prefix) {
		t.Fatalf("errOut 开头不等\n实际 %q\n期望开头 %q", errOut, prefix)
	}
	rest := errOut[len(prefix):]
	nl := strings.Index(rest, "\n")
	if nl <= 0 {
		t.Fatalf("「写不进配置文件」那一行之后应当是非空的操作系统错误文本加换行，实际 %q", rest)
	}
	wantTail := "下面是本来要写的内容，请自己存成 " + e.cfgPath + "（UTF-8），再运行 council：\n\n" +
		strings.TrimRight(obxWantFile(e, "claude", "grok", "codex"), "\n") + "\n"
	if tail := rest[nl+1:]; tail != wantTail {
		t.Errorf("错误文本那一行之后不等\n实际 %q\n期望 %q", tail, wantTail)
	}
	if out != "" {
		t.Errorf("out 应为空，实际 %q", out)
	}
	if got := obxReadFile(t, blocker); got != "x" {
		t.Errorf("挡路的文件内容被改了：%q", got)
	}
}

// TestObxFirstRunRace 是 A6（撞车）：假 claude 在识别期间把准备好的文件拷到默认位置。
// 找到的全都已登录，登录说明与登录办法都不打（A1 里那两段的缺席对照）。
// 【判别力】M41、M42、M24（临时文件没删）、M74（总打登录说明）。
func TestObxFirstRunRace(t *testing.T) {
	e := obxIsolate(t)
	body := "[providers.claude]\ncommand = 'obx-prepared'\n\n[agents.p1]\nprovider = 'claude'\nmodel = 'm'\n"
	prepared := obxFixture(t, e, "prepared.toml", body)
	cfx := obxFixture(t, e, "claude.out", obxClaudeOut)
	dir := filepath.Join(e.home, ".council")
	obxFake(t, e.bin, "claude",
		`@if not exist "`+dir+`" mkdir "`+dir+`"`,
		`@copy /y "`+prepared+`" "`+e.cfgPath+`" >nul`,
		`@type "`+cfx+`"`)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("撞车时 Run 应当照常加载那份文件，实际返回 %v；errOut:\n%s", err, errOut)
	}
	want := obxSub(e, obxHead+"正在检查 claude 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"+
		"  claude  <bin>\\claude.cmd  识别到 2 个模型；已登录（订阅：Claude Obx）\n"+
		"  grok    没找到\n  agy     没找到\n  codex   没找到\n"+
		"配置文件在检查期间已经被建好了，改用它: <home>\\.council\\config.toml\n\n")
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if file := obxReadFile(t, e.cfgPath); file != body {
		t.Errorf("配置文件应仍是准备好的那份\n实际 %q\n期望 %q", file, body)
	}
	if !strings.Contains(out, "已加载 1 个 agent，0 个 team\n") {
		t.Errorf("out 应含「已加载 1 个 agent，0 个 team」（加载的是准备好的那份），实际 %q", out)
	}
	if got := obxEntries(t, dir); !reflect.DeepEqual(got, []string{"config.toml"}) {
		t.Errorf(".council 里应只剩 config.toml，实际 %q", got)
	}
}

// TestObxFirstRunCanceled 是 A7：传一个已经取消的 ctx。
// 【判别力】M36（不查取消）、M52（取消当错误返回）；不建文件以 A1 为正对照。
func TestObxFirstRunCanceled(t *testing.T) {
	e := obxIsolate(t)
	obxThreeFakes(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out, errOut, err := obxRun(t, ctx, Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Errorf("被取消时 Run 应当返回 nil，实际 %v", err)
	}
	if want := obxSub(e, obxHead+obxChecking+"识别被取消，没有生成配置文件；下次运行 council 会重新检查\n"); errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if out != "" {
		t.Errorf("out 应为空，实际 %q", out)
	}
	obxNotExist(t, e.cfgPath)
	obxNotExist(t, filepath.Join(e.home, ".council"))
}

// TestObxFirstRunFailures 是 A8：失败分类、查不出、失败的也写进配置。claude 超时（最后完成）、
// grok 一个模型都没列出来、codex 以 3 退出（debug models 与 login status 都走这两行）。
// 【判别力】M33、M37、M39、M40、M66、X2（claude 最后超时，一定最后完成）。
func TestObxFirstRunFailures(t *testing.T) {
	e := obxIsolate(t)
	obxSetTimeout(t, 1*time.Second)
	loopOn := obxFixture(t, e, "loop.on", "x")
	obxFake(t, e.bin, "claude", obxLoopLines(`exist "`+loopOn+`"`)...)
	obxTypeFake(t, e, "grok", "Available models:\n")
	obxFake(t, e.bin, "codex", "@echo obx-boom 1>&2", "@exit /b 3")

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 claude、grok、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 1s）……\n"+
		"  claude  <bin>\\claude.cmd  识别失败：超时（1s 内没有回来）；登录状态查不出\n"+
		"  grok    <bin>\\grok.cmd  识别失败：一个模型都没列出来（没登录？）；登录状态查不出\n"+
		"  agy     没找到\n"+
		"  codex   <bin>\\codex.cmd  识别失败：model discovery command failed: exit code 3: obx-boom；登录状态查不出\n"+
		obxLoginHead+"  claude  claude auth login\n  grok    grok login\n  codex   codex login\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if file, wantFile := obxReadFile(t, e.cfgPath), obxWantFile(e, "claude", "grok", "codex"); file != wantFile {
		t.Errorf("识别失败的也应写进配置；配置文件字节不等\n实际 %q\n期望 %q", file, wantFile)
	}
}

// TestObxFirstRunParallel 是 A9（并行）：claude 要等 codex 起来（写标记）才回答；串行（claude 在前）时
// claude 5 s 超时。codex 是没登录这一档。
// 【判别力】M34（串行）；M73、M88、M91（codex 没登录这一档）。
func TestObxFirstRunParallel(t *testing.T) {
	e := obxIsolate(t)
	obxSetTimeout(t, 5*time.Second)
	marker := filepath.Join(e.fx, "codex-started")
	cfx := obxFixture(t, e, "claude.out", obxClaudeOut)
	obxFake(t, e.bin, "claude", obxLoopLines(`not exist "`+marker+`"`, `type "`+cfx+`"`)...)
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@echo x> "` + marker + `"`, `@type "` + xfx + `"`}, obxCodexNotIn)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 claude、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 5s）……\n"+
		"  claude  <bin>\\claude.cmd  识别到 2 个模型；已登录（订阅：Claude Obx）\n"+
		"  grok    没找到\n  agy     没找到\n"+
		"  codex   <bin>\\codex.cmd  识别到 1 个模型；没登录\n"+
		obxLoginHead+"  codex   codex login\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
}

// TestObxAddHintLineMode 是 A10：行式分支装了检测器。显式配置（不走首次运行）里有 claude，
// configured 再加 grok；/add 之后对本机装了、配置里没有的每一家打提示。两个子测试只差 grok 在不在配置里，
// 互为「grok 提示出现 / 不出现」的对照；claude 在配置里、agy 没装，都不提示。
// 【判别力】M53（行式分支不装检测器）、M60、M61、M63、M2、M12。
func TestObxAddHintLineMode(t *testing.T) {
	for _, mode := range []string{"hint", "configured"} {
		t.Run(mode, func(t *testing.T) {
			e := obxIsolate(t)
			obxThreeFakes(t, e)
			explicit := filepath.Join(e.root, "explicit.toml")
			body := "data_dir = '" + filepath.Join(e.root, "data") + "'\n\n" + config.ProviderBlockTOML("claude", filepath.Join(e.bin, "claude.cmd"))
			if mode == "configured" {
				body += "\n" + config.ProviderBlockTOML("grok", filepath.Join(e.bin, "grok.cmd"))
			}
			if err := os.WriteFile(explicit, []byte(body), 0o644); err != nil {
				t.Fatalf("前提不成立：写显式配置失败：%v", err)
			}

			out, errOut, err := obxRun(t, context.Background(), Options{ConfigPath: explicit, In: obxIn("/new t\n/add\n/quit\n")})
			if err != nil {
				t.Fatalf("Run 返回 %v；errOut:\n%s", err, errOut)
			}
			if errOut != "" {
				t.Errorf("显式路径不走首次运行，errOut 应为空，实际 %q", errOut)
			}
			hint := func(id string) string {
				p := filepath.Join(e.bin, id+".cmd")
				return "本机装了 " + id + "（" + p + "），配置里还没有：要用它，把下面两行追加到 " + explicit +
					" 末尾，再敲 /config reload --apply 与 /add --refresh\n[providers." + id + "]\ncommand = '" + p + "'\n"
			}
			// OB3 起识别表之后、提示之前是各家的登录状态（配置里写了的几家，字典序；OB3 契约 T2）。
			wantTail := "EFFORTS 列是它支持的档\n各家的登录状态（本会话的快照，/add --refresh 重新检查）:\n  claude  已登录（订阅：Claude Obx）\n"
			if mode == "hint" {
				wantTail += hint("grok")
			} else {
				wantTail += "  grok    没登录；在终端里运行 grok login，登录之后 /add --refresh\n"
			}
			wantTail += hint("codex") + "council> 再见\n"
			if !strings.HasSuffix(out, wantTail) {
				t.Errorf("out 结尾不等\n实际 %q\n期望结尾 %q", out, wantTail)
			}
		})
	}
}

// TestObxFirstRunTUI 是 A11：TUI 模式下首次运行的输出在横幅与第一帧之前（错误出口与 stdout 是同一根管子），
// 以及 TUI 分支装了检测器（首次运行之后才装上的 agy 在 /add 之后得到提示）。
// 【判别力】M54（TUI 分支不装检测器）；M47 之类把首次运行写进 out 的，顺序变，红。
func TestObxFirstRunTUI(t *testing.T) {
	e := obxIsolate(t)
	obxThreeFakes(t, e)

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提不成立：os.Pipe() 造输入管道失败：%v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = inW.Close()
		_ = inR.Close()
		t.Fatalf("前提不成立：os.Pipe() 造输出管道失败：%v", err)
	}
	buf := &coxLockedBuf{}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(buf, outR)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = Run(ctx, Options{TUI: true, InFile: inR, OutFile: outW, Err: outW})
	}()
	t.Cleanup(func() {
		obxDrain(cancel, finished, t.Name())
		_ = inW.Close()
		_ = inR.Close()
		_ = outW.Close()
		select {
		case <-readDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：关掉 outW 之后 5 秒读 goroutine 仍没有结束")
		}
		_ = outR.Close()
	})

	flatHas := func(d time.Duration, want string) bool {
		return coxWaitFor(buf, d, func(raw string) bool { return strings.Contains(coxFlat(raw), want) })
	}
	write := func(what, s string) {
		t.Helper()
		if _, err := inW.Write([]byte(s)); err != nil {
			t.Fatalf("前提不成立：往输入管道写 %s 失败：%v", what, err)
		}
	}

	if !coxWaitFor(buf, 10*time.Second, func(raw string) bool { return strings.Contains(raw, coxESC+"[J") }) {
		t.Fatalf("前提不成立：10 秒内 stdout 管道里没有出现 ESC[J；内容 %q", buf.String())
	}
	write("窗口尺寸报告", coxESC+"[8;24;100t")
	if !flatHas(5*time.Second, "council空闲") {
		t.Fatalf("前提不成立：5 秒内没有看到空闲表头；flat %q", coxFlat(buf.String()))
	}
	wantHead := obxSub(e, obxHead+obxChecking+obxRowsOK+obxDone) + "配置文件: " + e.cfgPath + "\n"
	if raw := buf.String(); !strings.HasPrefix(raw, wantHead) {
		n := len(wantHead) + 200
		if n > len(raw) {
			n = len(raw)
		}
		t.Errorf("管道的原始字节应以首次运行的全部输出加横幅第一行开头\n实际开头 %q\n期望开头 %q", raw[:n], wantHead)
	}

	// 首次运行之后才装上的 agy：/add 现检测，应当提示。
	obxTypeFake(t, e, "agy", obxAgyOut)
	write("/new", "/new t\r")
	if !flatHas(10*time.Second, "已建会话") {
		t.Fatalf("前提不成立：10 秒内没有看到「已建会话」；flat %q", coxFlat(buf.String()))
	}
	write("/add", "/add\r")
	want := strings.ReplaceAll("本机装了 agy（"+filepath.Join(e.bin, "agy.cmd")+"），配置里还没有", " ", "")
	if !flatHas(10*time.Second, want) {
		t.Errorf("/add 之后 10 秒内没有看到 agy 的提示 %q；flat %q", want, coxFlat(buf.String()))
	}
}

// TestObxFirstRunCheckingFirst 是 A12：「正在检查」必须在起 goroutine 之前写出。假 claude 等 errOut 里
// 出现「正在检查」（obxMarkWriter 建标记）才回答。找到的全都已登录，登录说明不打。
// 【判别力】X3（「正在检查」挪到识别之后打：claude 等不到标记，3 s 超时）、M74。
func TestObxFirstRunCheckingFirst(t *testing.T) {
	e := obxIsolate(t)
	obxSetTimeout(t, 3*time.Second)
	marker := filepath.Join(e.fx, "checking-seen")
	cfx := obxFixture(t, e, "claude.out", obxClaudeOut)
	obxFake(t, e.bin, "claude", obxLoopLines(`not exist "`+marker+`"`, `type "`+cfx+`"`)...)

	out, errOut, err := obxRunTee(t, context.Background(), Options{In: obxIn("/quit\n")}, &obxMarkWriter{path: marker})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 claude 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 3s）……\n"+
		"  claude  <bin>\\claude.cmd  识别到 2 个模型；已登录（订阅：Claude Obx）\n"+
		"  grok    没找到\n  agy     没找到\n  codex   没找到\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
}

// TestObxFirstRunBadHome 是 A13：USERPROFILE 是 <root>\a<b，Stat 的错误不是 NotExist，不进首次运行，
// 交给 LoadAndValidate 照常报。A1 是「不含第一次运行」的正对照（只差 home 的名字合不合法）。
// 【判别力】X5（Stat 的任何错误都进首次运行）。
func TestObxFirstRunBadHome(t *testing.T) {
	e := obxIsolate(t)
	obxThreeFakes(t, e)
	bad := filepath.Join(e.root, "a<b")
	t.Setenv("USERPROFILE", bad)
	t.Setenv("HOME", bad)
	before := obxEntries(t, e.root)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err == nil {
		t.Errorf("home 的名字不合法时 Run 应当返回错误")
	}
	prefix := "配置文件有问题: " + filepath.Join(bad, ".council", "config.toml") + "\n"
	if !strings.HasPrefix(errOut, prefix) {
		t.Errorf("errOut 开头不等\n实际 %q\n期望开头 %q", errOut, prefix)
	}
	if strings.Contains(errOut, "第一次运行") {
		t.Errorf("Stat 的其它错误不该进首次运行，errOut %q", errOut)
	}
	if out != "" {
		t.Errorf("out 应为空，实际 %q", out)
	}
	if after := obxEntries(t, e.root); !reflect.DeepEqual(after, before) {
		t.Errorf("<root> 下的条目变了\n之前 %q\n之后 %q", before, after)
	}
}

// TestObxFirstRunRelativeHome 是 A14：USERPROFILE 是相对路径，默认路径不是绝对路径，不进首次运行
// （否则会在启动目录下建文件）；走 reportConfigError 非 explicit 的 NotExist 分支，只打两行说明。
// 【判别力】M27（去掉 IsAbs：在启动目录下建出 relhome\.council\config.toml）、M55（这一支把模板加回来）。
func TestObxFirstRunRelativeHome(t *testing.T) {
	e := obxIsolate(t)
	obxThreeFakes(t, e)
	cwd := filepath.Join(e.root, "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("前提不成立：建 cwd 失败：%v", err)
	}
	t.Chdir(cwd)
	t.Setenv("USERPROFILE", "relhome")
	t.Setenv("HOME", "relhome")

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err == nil {
		t.Errorf("默认路径不是绝对路径、文件不存在时 Run 应当返回错误")
	}
	want := "找不到配置文件: relhome\\.council\\config.toml\n可以用 --config <path> 指定别处的配置文件，或设置环境变量 COUNCIL_CONFIG 指向它。\n"
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if out != "" {
		t.Errorf("out 应为空，实际 %q", out)
	}
	obxNotExist(t, filepath.Join(cwd, "relhome"))
}

// TestObxFirstRunLoginStates 是 A15：四家都装，claude 没登录、grok 登录态那一行（OB2 起是已登录（API key），
// 不列登录办法）、agy 找到时的查不出与兜底句、codex 已登录（不列登录办法）。
// 【判别力】OB2 的 N1、N20、N21（grok 那一行与登录办法）、M39、M89、M92、M56。
func TestObxFirstRunLoginStates(t *testing.T) {
	e := obxIsolate(t)
	obxTypeFake(t, e, "claude", obxClaudeNoLoginOut)
	obxTypeFake(t, e, "grok", obxGrokKeyOut)
	obxTypeFake(t, e, "agy", obxAgyOut)
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, obxCodexLoggedIn)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 claude、grok、agy、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"+
		"  claude  <bin>\\claude.cmd  识别到 2 个模型；没登录\n"+
		"  grok    <bin>\\grok.cmd  识别到 1 个模型；已登录（API key）\n"+
		"  agy     <bin>\\agy.cmd  识别到 1 个模型；登录状态查不出\n"+
		"  codex   <bin>\\codex.cmd  识别到 1 个模型；已登录\n"+
		obxLoginHead+"  claude  claude auth login\n  agy     agy（按提示登录）\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if file, wantFile := obxReadFile(t, e.cfgPath), obxWantFile(e, "claude", "grok", "agy", "codex"); file != wantFile {
		t.Errorf("配置文件字节不等\n实际 %q\n期望 %q", file, wantFile)
	}
}
