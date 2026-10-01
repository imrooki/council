package app

// adn_addname_app_test.go 是 AD1（00_DECISIONS.md #49：/add 按名字加识别出的模型）在 app 包的验收，契约 §2.2：
// N20 行式分支、N21 TUI 分支都打开了开关（经真 app.Run，新会话里第一次就敲 /add gpt-obx-high，先识别、再写进
// 显式配置、加入）。开关关着时这一条打的是「配置里没有可加入的 gpt-obx-high……」，文件不变。
// 装置与夹具只读复用 obx_firstrun_app_test.go 的 §2.0（obxIsolate、obxRun、obxDrain、obxFixture、obxCodexFake、
// obxCodexOut、obxCodexLoggedIn、obxIn、obxEnv、obxReadFile）与 cox_app_test.go 的 coxLockedBuf、
// coxWaitFor、coxFlat、coxESC；纪律同契约 19d §2 开头。两条都用显式配置，不走首次运行。

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"council/internal/config"
)

// adnAppBlock 是 /add gpt-obx-high 写进配置的那一段（obxCodexOut 里 gpt-obx 只有 high 一档，display_name 缺席）。
const adnAppBlock = "\n# council /add 追加：codex 识别出的 gpt-obx（codex 目录）\n[agents.gpt-obx-high]\ndisplay_name = \"Gpt Obx\"\nprovider = \"codex\"\nmodel = \"gpt-obx\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"

// adnAppSetup 装假 codex（debug models 打 obxCodexOut，login status 打已登录），写一份只有 codex 一家的显式配置。
// 返回配置路径与它的原文。
func adnAppSetup(t *testing.T, e *obxEnv) (string, string) {
	t.Helper()
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, obxCodexLoggedIn)
	explicit := filepath.Join(e.root, "explicit.toml")
	body := "data_dir = '" + filepath.Join(e.root, "data") + "'\n\n" + config.ProviderBlockTOML("codex", filepath.Join(e.bin, "codex.cmd"))
	if err := os.WriteFile(explicit, []byte(body), 0o644); err != nil {
		t.Fatalf("前提不成立：写显式配置失败：%v", err)
	}
	return explicit, body
}

// adnAppBaks 是 dir 里 explicit.toml 的备份文件（config.AppendBlock 的命名）。
func adnAppBaks(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "explicit.toml.bak-add-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	return m
}

// adnAppFileAdded 断言显式配置等于原文加 adnAppBlock，恰好一个备份、字节等于原文。
func adnAppFileAdded(t *testing.T, e *obxEnv, explicit, orig string) {
	t.Helper()
	if got := obxReadFile(t, explicit); got != orig+adnAppBlock {
		t.Errorf("显式配置不等\n实际 %q\n期望 %q", got, orig+adnAppBlock)
	}
	baks := adnAppBaks(t, e.root)
	if len(baks) != 1 {
		t.Fatalf("应当恰好一个备份，实际 %q", baks)
	}
	if got := obxReadFile(t, baks[0]); got != orig {
		t.Errorf("备份 %s 应当是原文，实际 %q", baks[0], got)
	}
}

// TestAdnAppLineMode 是 N20：行式分支。新会话里第一次就敲 /add gpt-obx-high：先识别（正在识别模型: codex），
// 再追加、加入，out 的结尾整段相等（会话 id 与备份时间戳按形状比）；文件与备份按字节比。
// 【判别力】V45（行式分支不打开开关）。
func TestAdnAppLineMode(t *testing.T) {
	e := obxIsolate(t)
	explicit, orig := adnAppSetup(t, e)

	out, errOut, err := obxRun(t, context.Background(), Options{ConfigPath: explicit, In: obxIn("/new t\n/add gpt-obx-high\n/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s", err, errOut)
	}
	if errOut != "" {
		t.Errorf("显式路径不走首次运行，errOut 应为空，实际 %q", errOut)
	}
	q := regexp.QuoteMeta(explicit)
	tail := regexp.MustCompile(`council> 正在识别模型: codex\n已把 agent gpt-obx-high 追加进 ` + q + `（原文件备份在 ` + q + `\.bak-add-\d{8}-\d{6}）\ngpt-obx-high 已加入会话 [0-9a-f]{32}\ncouncil> 再见\n$`)
	if !tail.MatchString(out) {
		t.Errorf("out 结尾不对\n实际 %q\n期望形如 %q", out, tail.String())
	}
	adnAppFileAdded(t, e, explicit, orig)
}

// TestAdnAppTUI 是 N21：TUI 分支。同 N20 的装置，经 os.Pipe 走真 tui.Run，/new 之后 /add gpt-obx-high，
// 屏幕上（去掉 ANSI 与空格）出现「gpt-obx-high已加入会话」，文件与备份按字节比。
// 【判别力】V46（TUI 分支不打开开关）。
func TestAdnAppTUI(t *testing.T) {
	e := obxIsolate(t)
	explicit, orig := adnAppSetup(t, e)

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
	write("/add", "/add gpt-obx-high\r")
	if !flatHas(10*time.Second, "gpt-obx-high已加入会话") {
		t.Fatalf("/add gpt-obx-high 之后 10 秒内没有看到「gpt-obx-high已加入会话」；flat %q", coxFlat(buf.String()))
	}
	adnAppFileAdded(t, e, explicit, orig)
}
