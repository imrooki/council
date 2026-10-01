package app

// CO1 §2.4（A1）：真 app.Run、真 store、假 CLI。app 的默认档是简洁，Verbose: true 回到今天的详细输出；
// 行式与 TUI 两条分支各两个子测试。引用本刀新增的 Options.Verbose，在 HEAD 上编译不过是预期的。
// 不用 runAppForTest（本刀落地后它固定 Verbose: true）。

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// coxAppCfgBody 是 §2.4 冻结的配置文本（照 smoke_test.go 的体例）。
func coxAppCfgBody(dataDir string) string {
	return fmt.Sprintf(`data_dir = %s

[providers.claude]
command = %s

[agents.c1]
display_name = "成员一"
provider = "claude"
model = "opus"
effort = "high"
role = "cox"
permissions = "read"
mode = "discussion"
`, tomlLiteral(dataDir), tomlLiteral(os.Args[0]))
}

// coxAppSetup 做两类子测试共用的前置：隔离环境、假 CLI、写配置。返回配置路径。
func coxAppSetup(t *testing.T) string {
	t.Helper()
	isolateAppEnv(t)
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	dir := t.TempDir()
	return writeTestConfig(t, dir, coxAppCfgBody(filepath.Join(dir, "data")))
}

// coxRunApp 直接调 Run，带一个看门狗（Run 不返回就判失败，不把测试挂死）。
func coxRunApp(t *testing.T, opts Options) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), opts) }()
	select {
	case err := <-done:
		return err
	case <-time.After(60 * time.Second):
		t.Fatalf("app.Run 在 60 秒内没有返回")
		return nil
	}
}

var (
	coxAppHeadRe  = regexp.MustCompile(`（\d+\.\ds）`)
	coxAppTotalRe = regexp.MustCompile(`^总用时 \d+\.\ds$`)
)

// coxAppTail 取「尾部」：按行切，每行反复去掉行首的 council> ，从以「c1 已加入会话 」开头的那一行之后开始收集，
// 每行做成员头规范化，整行是总用时的换成「总用时 T」。
func coxAppTail(out string) []string {
	var tail []string
	started := false
	for _, l := range strings.Split(out, "\n") {
		for strings.HasPrefix(l, "council> ") {
			l = strings.TrimPrefix(l, "council> ")
		}
		if !started {
			if strings.HasPrefix(l, "c1 已加入会话 ") {
				started = true
			}
			continue
		}
		l = coxAppHeadRe.ReplaceAllString(l, "（T）")
		if coxAppTotalRe.MatchString(l) {
			l = "总用时 T"
		}
		tail = append(tail, l)
	}
	return tail
}

// coxLockedBuf 是带锁的缓冲：TUI 的 stdout 管道由读 goroutine 写，测试 goroutine 轮询读。
type coxLockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *coxLockedBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *coxLockedBuf) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// coxFlat 是 TUI 子测试冻结的比对形状：剥掉 ANSI 转义，再删掉全部 ASCII 空格。
func coxFlat(raw string) string {
	return strings.ReplaceAll(ansi.Strip(raw), " ", "")
}

// coxWaitFor 每 10ms 轮询一次，直到 cond 对缓冲内容成立或超时。
func coxWaitFor(buf *coxLockedBuf, d time.Duration, cond func(raw string) bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond(buf.String()) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const coxESC = "\x1b"

var (
	coxConvRe     = regexp.MustCompile(`已建会话 (\S+)，标题: t`)
	coxTuiHeadRe  = regexp.MustCompile(`成员一（\d+\.\ds）:OK`)
	coxTuiFinalRe = regexp.MustCompile(`/round结束\d+s回来1/1`)
)

// coxRunTui 是 tui / tui_verbose 共用的装置，参数只有 Verbose；返回第 1 段 stdout 管道的 flat。
// 第 0 段行式 Run 建会话（不带 Verbose），返回之后才开第 1 段（store 的排他锁要等 Close）；
// 第 1 段 TUI 模式 Run 经 os.Pipe 进同一个会话跑 /round c1 --go，每一步一个可观测同步点。
func coxRunTui(t *testing.T, verbose bool) string {
	t.Helper()
	cfgPath := coxAppSetup(t)

	var out0, err0 bytes.Buffer
	if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader("/new t\n/join c1\nhi\n/quit\n"), Out: &out0, Err: &err0, NoAuto: true}); err != nil {
		t.Fatalf("前提不成立：第 0 段 Run 返回 %v；Out:\n%s\nErr:\n%s", err, out0.String(), err0.String())
	}
	m := coxConvRe.FindStringSubmatch(out0.String())
	if m == nil {
		t.Fatalf("前提不成立：第 0 段输出里取不到会话 id；Out:\n%s", out0.String())
	}
	convID := m[1]

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

	errOut := &coxLockedBuf{}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Options{ConfigPath: cfgPath, TUI: true, InFile: inR, OutFile: outW, Err: errOut, Verbose: verbose, NoAuto: true})
	}()

	tornDown := false
	teardown := func() {
		if tornDown {
			return
		}
		tornDown = true
		cancel()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：cancel() 之后 5 秒 Run 仍没有返回")
		}
		_ = inW.Close()
		_ = inR.Close()
		_ = outW.Close()
		select {
		case <-readDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：关掉 outW 之后 5 秒读 goroutine 仍没有结束")
		}
		_ = outR.Close()
	}
	t.Cleanup(teardown)

	flatHas := func(d time.Duration, want string) bool {
		return coxWaitFor(buf, d, func(raw string) bool { return strings.Contains(coxFlat(raw), want) })
	}
	write := func(what, s string) {
		t.Helper()
		if _, err := inW.Write([]byte(s)); err != nil {
			t.Fatalf("前提不成立：往输入管道写 %s 失败：%v；Err:\n%s", what, err, errOut.String())
		}
	}

	if !coxWaitFor(buf, 5*time.Second, func(raw string) bool { return strings.Contains(raw, coxESC+"[J") }) {
		t.Fatalf("前提不成立：5 秒内 stdout 管道里没有出现 ESC[J；Err:\n%s\n内容 %q", errOut.String(), buf.String())
	}
	write("窗口尺寸报告", coxESC+"[8;24;100t")
	if !flatHas(5*time.Second, "council空闲") {
		t.Fatalf("前提不成立：5 秒内没有看到空闲表头；Err:\n%s\nflat %q", errOut.String(), coxFlat(buf.String()))
	}
	write("/use", "/use "+convID+"\r")
	if !flatHas(5*time.Second, "当前会话:") {
		t.Fatalf("前提不成立：5 秒内没有看到「当前会话:」；Err:\n%s\nflat %q", errOut.String(), coxFlat(buf.String()))
	}
	write("/round", "/round c1 --go\r")
	if !flatHas(15*time.Second, "/round结束") {
		t.Fatalf("前提不成立：15 秒内没有看到收尾行「/round 结束」；Err:\n%s\nflat %q", errOut.String(), coxFlat(buf.String()))
	}
	teardown()
	return coxFlat(buf.String())
}

// TestCoxAppDefaultConcise 是 A1：app.Run 的默认档是简洁，Verbose: true 回到详细；行式与 TUI 两条分支。
//
// 【判别力】default / verbose 只差 Options.Verbose：行式分支不调 SetConcise（#36）时 default 印的是详细档，
// 尾部整体不等，红；写反（#37）两个都红。default 的尾部整串还钉住裸文本在简洁档不打「已记下」「点名了」
// （两个 council> 连在一起）、/round 后恰好一行总用时、/ask 没有总用时。verbose 的「不含总用时 T」是 default 的对照。
// tui / tui_verbose 装置完全相同、只差 Options.Verbose，互为对方缺席断言的正对照：
// TUI 分支不调 SetConcise（#56）时 tui 红；写反（#57）两个都红；写死 SetConcise(true)（#58）时 tui_verbose 红。
// 【为什么是子串】tui / tui_verbose 断言的是终端字节流，帧与帧之间夹着光标移动与重绘，整串相等做不到；
// 剥 ANSI、删空格之后找的每一句都是一整行的全部内容（成员头连正文、outcome 行、结算标题、收尾行）。
func TestCoxAppDefaultConcise(t *testing.T) {
	const script = "/new t\n/join c1\n裸文本 @c1\n/round c1 --go\n/ask c1 再问\n/quit\n"
	lineMode := func(t *testing.T, verbose bool) []string {
		cfgPath := coxAppSetup(t)
		var out, errOut bytes.Buffer
		if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: verbose, NoAuto: true}); err != nil {
			t.Fatalf("app.Run 返回 %v；Out:\n%s\nErr:\n%s", err, out.String(), errOut.String())
		}
		return coxAppTail(out.String())
	}

	t.Run("default", func(t *testing.T) {
		got := lineMode(t, false)
		want := []string{"本轮问 1 个: c1", "成员一（T）: OK", "总用时 T", "正在问 c1 ...", "成员一（T）: OK", "再见", ""}
		if strings.Join(got, "\n") != strings.Join(want, "\n") || len(got) != len(want) {
			t.Fatalf("默认档（简洁）的尾部不对\n实际: %q\n期望: %q", got, want)
		}
	})

	t.Run("verbose", func(t *testing.T) {
		got := lineMode(t, true)
		hasOutcome := false
		for _, l := range got {
			if l == "outcome spoke  agent c1" {
				hasOutcome = true
			}
			if l == "总用时 T" {
				t.Fatalf("详细档不应当有总用时；尾部: %q", got)
			}
		}
		if !hasOutcome {
			t.Fatalf("详细档的尾部应当含整行「outcome spoke  agent c1」；尾部: %q", got)
		}
	})

	t.Run("tui", func(t *testing.T) {
		flat := coxRunTui(t, false)
		if !coxTuiHeadRe.MatchString(flat) {
			t.Fatalf("简洁档 TUI 应当有成员头「成员一（N.Ns）:OK」；flat %q", flat)
		}
		if strings.Contains(flat, "outcomespokeagentc1") || strings.Contains(flat, "发言轮结算") {
			t.Fatalf("简洁档 TUI 不应当有 outcome 行或结算块；flat %q", flat)
		}
		if !coxTuiFinalRe.MatchString(flat) {
			t.Fatalf("TUI 应当有收尾行「/round 结束 Ns 回来 1/1」；flat %q", flat)
		}
	})

	t.Run("tui_verbose", func(t *testing.T) {
		flat := coxRunTui(t, true)
		if !strings.Contains(flat, "outcomespokeagentc1") || !strings.Contains(flat, "发言轮结算") {
			t.Fatalf("详细档 TUI 应当有 outcome 行与结算块；flat %q", flat)
		}
		if coxTuiHeadRe.MatchString(flat) {
			t.Fatalf("详细档 TUI 不应当有简洁档的成员头；flat %q", flat)
		}
		if !coxTuiFinalRe.MatchString(flat) {
			t.Fatalf("TUI 应当有收尾行「/round 结束 Ns 回来 1/1」；flat %q", flat)
		}
	})
}
