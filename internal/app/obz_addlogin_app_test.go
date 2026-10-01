package app

// obz_addlogin_app_test.go 是 OB3（00_DECISIONS.md #48：/add 的识别结果之后显示各家的登录状态）在 app 包的验收：
// Z10 行式分支、Z11 TUI 分支都装上了登录状态执行器（经真 app.Run 走一次 /add，假 codex 的 login status
// 那一支被调到、登录段出现）；Z12 首次运行的说法就是 repl.LoginText 那一个函数；Z13 首次运行里识别报错但
// stdout 有内容时照样判登录状态。
// 装置与夹具只读复用 obx_firstrun_app_test.go 的 §2.0（obxIsolate、obxRun、obxDrain、obxFake、obxTypeFake、obxCodexFake、
// obxFixture、obxIn、obxEnv、obxNotExist、obxSub、obxClaudeOut、obxGrokOut、obxCodexOut、obxCodexLoggedIn、obxHead、
// obxLoginHead、obxDone）与 cox_app_test.go 的 coxLockedBuf、coxWaitFor、coxFlat、coxESC；纪律同契约 19d §2 开头。
// Z10、Z11 用显式配置，不走首次运行；Z13 不给配置路径，走首次运行。

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/provider"
	"council/internal/repl"
)

// obzSection 是 Z10 的配置（claude、codex、grok）下 /add 打出的登录段（契约 §3）。
const obzSection = "各家的登录状态（本会话的快照，/add --refresh 重新检查）:\n" +
	"  claude  已登录（订阅：Claude Obx）\n" +
	"  codex   已登录\n" +
	"  grok    没登录；在终端里运行 grok login，登录之后 /add --refresh\n"

// obzCustomNote 是 obxCodexOut 里那一条 list 的 custom/ 记录带出的说明行（既有字面量）。
const obzCustomNote = "codex 目录里还有 1 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员\n"

// obzSetup 装 claude（已登录，订阅 Claude Obx）、grok（没登录）、codex（debug models 照常；login status 先写
// 标记文件 <fx>\status-ran 再打 Logged in using a subscription），写一份只有这三家 provider 的显式配置。
// 返回配置路径与标记文件路径。
func obzSetup(t *testing.T, e *obxEnv) (string, string) {
	t.Helper()
	obxTypeFake(t, e, "claude", obxClaudeOut)
	obxTypeFake(t, e, "grok", obxGrokOut)
	marker := filepath.Join(e.fx, "status-ran")
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, append([]string{`@echo x> "` + marker + `"`}, obxCodexLoggedIn...))
	explicit := filepath.Join(e.root, "explicit.toml")
	body := "data_dir = '" + filepath.Join(e.root, "data") + "'\n"
	for _, id := range []string{"claude", "codex", "grok"} {
		body += "\n" + config.ProviderBlockTOML(id, filepath.Join(e.bin, id+".cmd"))
	}
	if err := os.WriteFile(explicit, []byte(body), 0o644); err != nil {
		t.Fatalf("前提不成立：写显式配置失败：%v", err)
	}
	return explicit, marker
}

// TestObzAddLoginLineMode 是 Z10：行式分支。/add 的识别部分之后是登录段，codex 那一行来自假 codex 的
// login status（标记文件证明那一支被调到）；三家都在配置里、agy 没装，登录段之后没有提示。
// 标记在 Run 之前不存在（前提）。
// 【判别力】Q29（行式分支不装执行器：没有登录段、没有标记），另有 Q22、Q23。
func TestObzAddLoginLineMode(t *testing.T) {
	e := obxIsolate(t)
	explicit, marker := obzSetup(t, e)
	obxNotExist(t, marker)

	out, errOut, err := obxRun(t, context.Background(), Options{ConfigPath: explicit, In: obxIn("/new t\n/add\n/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s", err, errOut)
	}
	if errOut != "" {
		t.Errorf("显式路径不走首次运行，errOut 应为空，实际 %q", errOut)
	}
	wantTail := "EFFORTS 列是它支持的档\n" + obzCustomNote + obzSection + "council> 再见\n"
	if !strings.HasSuffix(out, wantTail) {
		t.Errorf("out 结尾不等\n实际 %q\n期望结尾 %q", out, wantTail)
	}
	if _, serr := os.Stat(marker); serr != nil {
		t.Errorf("假 codex 的 login status 应当被 /add 调到（标记文件 %s）：%v", marker, serr)
	}
}

// TestObzAddLoginTUI 是 Z11：TUI 分支。同 Z10 的装置，经 os.Pipe 走真 tui.Run，/new 之后 /add，
// 屏幕上（去掉 ANSI 与空格）出现登录段的段头与 codex 那一行，标记文件证明 login status 被调到。
// 【判别力】Q28（TUI 分支不装执行器）。
func TestObzAddLoginTUI(t *testing.T) {
	e := obxIsolate(t)
	explicit, marker := obzSetup(t, e)
	obxNotExist(t, marker)

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
		_ = Run(ctx, Options{TUI: true, ConfigPath: explicit, InFile: inR, OutFile: outW, Err: outW})
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
	write("/new", "/new t\r")
	if !flatHas(10*time.Second, "已建会话") {
		t.Fatalf("前提不成立：10 秒内没有看到「已建会话」；flat %q", coxFlat(buf.String()))
	}
	write("/add", "/add\r")
	head := strings.ReplaceAll("各家的登录状态（本会话的快照，/add --refresh 重新检查）:", " ", "")
	if !flatHas(10*time.Second, head) {
		t.Errorf("/add 之后 10 秒内没有看到登录段的段头 %q；flat %q", head, coxFlat(buf.String()))
	}
	if !flatHas(5*time.Second, "codex已登录") {
		t.Errorf("/add 之后没有看到登录段里 codex 那一行；flat %q", coxFlat(buf.String()))
	}
	if _, serr := os.Stat(marker); serr != nil {
		t.Errorf("假 codex 的 login status 应当被 /add 调到（标记文件 %s）：%v", marker, serr)
	}
}

// obzGrokExitOut 是 Z13 假 grok 打的输出：没登录那一句加一个模型。假 grok 打完它，往 stderr 打一行 obz-grok-exit，
// 再以退出码 1 结束。stderr 最后那一行由假 grok 自己打：隔离的 PATH 下 cmd 在跑批处理之前会往 stderr 打一行
// 找不到命令的报错（原型实测，没有查来源），不自己打一行，识别失败的说法就随机器而变。
const obzGrokExitOut = "You are not authenticated.\n\nAvailable models:\n  * grok-obz (default)\n"

// TestObzFirstRunGrokExitError 是 Z13：首次运行里识别报错（退出码非 0）但 stdout 有内容时，登录状态照样从
// stdout 判（与 /add 的 Z6 grok_exit_error 同一个判据）。只装 grok，它打出没登录那一句与模型列表、以 1 退出：
// 结果行是识别失败（exit code 1）而且没登录，之后列出 grok 的登录办法。识别失败那一段证明假 grok 确实以非 0
// 退出（前提）；「没登录」而不是「登录状态查不出」证明 stdout 被用上了。
// 【判别力】Q36（firstRunCheck 只在识别无错时才截 stdout，只有它杀得到），另有 Q23、Q30。
func TestObzFirstRunGrokExitError(t *testing.T) {
	e := obxIsolate(t)
	gfx := obxFixture(t, e, "grok.out", obzGrokExitOut)
	obxFake(t, e.bin, "grok", `@type "`+gfx+`"`, "@echo obz-grok-exit 1>&2", "@exit /b 1")

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 grok 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"+
		"  claude  没找到\n"+
		"  grok    <bin>\\grok.cmd  识别失败：model discovery command failed: exit code 1: obz-grok-exit；没登录\n"+
		"  agy     没找到\n  codex   没找到\n"+
		obxLoginHead+"  grok    grok login\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
}

// TestObzFirstRunLoginTextIsRepl 是 Z12：首次运行的说法就是 repl.LoginText 那一个函数（四档说法只有一份），
// 六种输入逐个相等是它的行为对照。
// 【判别力】Q31（首次运行另留一份说法）。
func TestObzFirstRunLoginTextIsRepl(t *testing.T) {
	if reflect.ValueOf(firstRunLoginText).Pointer() != reflect.ValueOf(repl.LoginText).Pointer() {
		t.Errorf("firstRunLoginText 应当就是 repl.LoginText，不是另一份")
	}
	for _, in := range []provider.LoginInfo{
		{State: provider.LoginYes, Subscription: "Claude Obz"},
		{State: provider.LoginYes},
		{State: provider.LoginAPIKey, KeySource: "apiKeyHelper"},
		{State: provider.LoginAPIKey},
		{State: provider.LoginNo},
		{},
	} {
		if got, want := firstRunLoginText(in), repl.LoginText(in); got != want {
			t.Errorf("firstRunLoginText(%+v) = %q，repl.LoginText 是 %q", in, got, want)
		}
	}
}
