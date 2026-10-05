package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/repl"
	"council/internal/store"
)

func rsmBanner(cfgPath, dataDir, dbPath, projectDir string, convs []conversation.Conversation) string {
	var buf bytes.Buffer
	writeBanner(&buf, "1.2.3", cfgPath, dataDir, dbPath, convs, projectDir, &config.Config{})
	return buf.String()
}

func TestRsmBannerProjectLines(t *testing.T) {
	const (
		cfgPath = `C:\cfg\config.toml`
		dataDir = `C:\data`
		dbPath  = `C:\data\council.db`
		dir     = `D:\proj\A`
		other   = `X:\别的项目`
		title   = "独特标题-不应出现在横幅"
	)
	tail := "已加载 0 个 agent，0 个 team\n输入 /help 查看可用命令，/quit 退出。\n\n"

	t.Run("空会话", func(t *testing.T) {
		got := rsmBanner(cfgPath, dataDir, dbPath, dir, nil)
		want := "council 1.2.3\n配置文件: " + cfgPath + "\n数据目录: " + dataDir + "\n库文件: " + dbPath +
			" (已有 0 个会话)\n项目目录: " + dir + "（本项目 0 个会话，/resume 列出并接上）\n" + tail
		if got != want {
			t.Errorf("横幅 = %q，期望 %q", got, want)
		}
		if strings.Contains(got, "另有") {
			t.Errorf("空会话不该有「另有」行；输出:\n%s", got)
		}
	})

	t.Run("本项目与未绑定", func(t *testing.T) {
		convs := []conversation.Conversation{
			{ID: "id-here", Title: title, Cwd: dir},
			{ID: "id-here-2", Title: title + "-2", Cwd: dir},
			{ID: "id-unbound", Title: title + "-un", Cwd: ""},
			{ID: "id-other", Title: title + "-ot", Cwd: other},
		}
		got := rsmBanner(cfgPath, dataDir, dbPath, dir, convs)
		want := "council 1.2.3\n配置文件: " + cfgPath + "\n数据目录: " + dataDir + "\n库文件: " + dbPath +
			" (已有 4 个会话)\n项目目录: " + dir + "（本项目 2 个会话，/resume 列出并接上）\n" +
			"另有 1 个未绑定项目的旧会话，/resume --all 查看\n" + tail
		if got != want {
			t.Errorf("横幅 = %q，期望 %q", got, want)
		}
		if strings.Contains(got, title) || strings.Contains(got, "id-here") {
			t.Errorf("横幅不该出现会话 id 或标题；输出:\n%s", got)
		}
	})

	t.Run("项目目录未知", func(t *testing.T) {
		convs := []conversation.Conversation{{ID: "id-unbound", Title: title, Cwd: ""}}
		got := rsmBanner(cfgPath, dataDir, dbPath, "", convs)
		want := "council 1.2.3\n配置文件: " + cfgPath + "\n数据目录: " + dataDir + "\n库文件: " + dbPath +
			" (已有 1 个会话)\n项目目录: 未知（/resume 将列出全部会话）\n" + tail
		if got != want {
			t.Errorf("横幅 = %q，期望 %q", got, want)
		}
		if strings.Contains(got, "另有") {
			t.Errorf("项目目录未知时不该有「另有」行；输出:\n%s", got)
		}
		if strings.Contains(got, title) {
			t.Errorf("横幅不该出现会话标题；输出:\n%s", got)
		}
	})

	t.Run("Windows 盘符大小写算同一项目", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("盘符大小写只在 Windows 上算同一项目")
		}
		lower := strings.ToLower(dir[:1]) + dir[1:]
		convs := []conversation.Conversation{{ID: "id-case", Title: title, Cwd: lower}}
		got := rsmBanner(cfgPath, dataDir, dbPath, dir, convs)
		want := "council 1.2.3\n配置文件: " + cfgPath + "\n数据目录: " + dataDir + "\n库文件: " + dbPath +
			" (已有 1 个会话)\n项目目录: " + dir + "（本项目 1 个会话，/resume 列出并接上）\n" + tail
		if got != want {
			t.Errorf("横幅 = %q，期望 %q", got, want)
		}
	})
}

func rsmRunLine(t *testing.T, cfgPath, input string) string {
	t.Helper()
	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Run 返回错误: %v\nErr:\n%s\nOut:\n%s", err, errOut, out)
	}
	return out
}

// rsmCountLine 数整行出现次数。行式 REPL 的提示符不带换行，
// 命令的第一行输出和 "council> " 粘在同一行，比对前先剥掉这一次前缀。
func rsmCountLine(out, line string) int {
	n := 0
	for _, got := range strings.Split(out, "\n") {
		got = strings.TrimRight(got, "\r")
		got = strings.TrimPrefix(got, "council> ")
		if got == line {
			n++
		}
	}
	return n
}

func TestRsmRunLineModeProjectDir(t *testing.T) {
	isolateAppEnv(t)
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	cfgPath := writeTestConfig(t, root, validConfigBody(dataDir))
	projA := filepath.Join(root, "projA")
	projB := filepath.Join(root, "projB")
	for _, d := range []string{projA, projB} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatalf("建项目目录 %s 失败: %v", d, err)
		}
	}
	wantA := repl.NormalizeProjectDir(projA)
	wantB := repl.NormalizeProjectDir(projB)
	if wantA == "" || wantB == "" {
		t.Fatalf("规范化后的项目目录是空的: A=%q B=%q", wantA, wantB)
	}

	t.Chdir(projA)
	out := rsmRunLine(t, cfgPath, "/new 第一场\n/resume\n/quit\n")
	lineA0 := "项目目录: " + wantA + "（本项目 0 个会话，/resume 列出并接上）"
	if rsmCountLine(out, lineA0) != 1 {
		t.Errorf("第一次横幅没有整行 %q；输出:\n%s", lineA0, out)
	}
	if strings.Contains(out, "另有") {
		t.Errorf("第一次不该有「另有」行；输出:\n%s", out)
	}
	resumeHead := "项目: " + wantA + "（本项目 1 个会话；/resume --all 看全部）"
	if rsmCountLine(out, resumeHead) != 1 {
		t.Errorf("第一次 /resume 没有整行 %q；输出:\n%s", resumeHead, out)
	}
	if !strings.Contains(out, "第一场") {
		t.Errorf("第一次输出里没有「第一场」；输出:\n%s", out)
	}

	out = rsmRunLine(t, cfgPath, "/resume\n/quit\n")
	if !strings.Contains(out, "已有 1 个会话") {
		t.Errorf("第二次横幅没有「已有 1 个会话」；输出:\n%s", out)
	}
	lineA1 := "项目目录: " + wantA + "（本项目 1 个会话，/resume 列出并接上）"
	if rsmCountLine(out, lineA1) != 1 {
		t.Errorf("第二次横幅没有整行 %q；输出:\n%s", lineA1, out)
	}
	if !strings.Contains(out, "第一场") {
		t.Errorf("第二次 /resume 没有列出「第一场」；输出:\n%s", out)
	}

	t.Chdir(projB)
	out = rsmRunLine(t, cfgPath, "/resume\n/resume --all\n/quit\n")
	lineB0 := "项目目录: " + wantB + "（本项目 0 个会话，/resume 列出并接上）"
	if rsmCountLine(out, lineB0) != 1 {
		t.Errorf("切到 B 之后横幅没有整行 %q；输出:\n%s", lineB0, out)
	}
	if strings.Contains(out, "另有") {
		t.Errorf("会话绑在 A，切到 B 不该有「另有」行；输出:\n%s", out)
	}
	emptyHere := "本项目还没有会话，用 /new <标题> 建一个（/resume --all 看其它项目的）"
	if rsmCountLine(out, emptyHere) != 1 {
		t.Errorf("默认 /resume 没有整行 %q；输出:\n%s", emptyHere, out)
	}
	allAt := strings.Index(out, "全部会话（")
	if allAt < 0 {
		t.Fatalf("输出里找不到 /resume --all 的表头「全部会话（」；输出:\n%s", out)
	}
	allOut := out[allAt:]
	if !strings.Contains(allOut, wantA) || !strings.Contains(allOut, "第一场") {
		t.Errorf("/resume --all 的输出里没有 %q 或「第一场」；后半:\n%s", wantA, allOut)
	}

	dataDir2 := filepath.Join(root, "data2")
	cfgRoot2 := filepath.Join(root, "cfg2")
	if err := os.Mkdir(cfgRoot2, 0o755); err != nil {
		t.Fatalf("建第二份配置目录失败: %v", err)
	}
	cfgPath2 := writeTestConfig(t, cfgRoot2, validConfigBody(dataDir2))
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir2)
	if err != nil {
		t.Fatalf("打开第二份库失败: %v", err)
	}
	if _, err := conversation.NewService(st).Create(ctx, "遗留会话甲"); err != nil {
		_ = st.Close()
		t.Fatalf("建未绑定会话失败: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关掉第二份库失败: %v", err)
	}

	t.Chdir(projA)
	out = rsmRunLine(t, cfgPath2, "/resume\n/resume --all\n/quit\n")
	unboundLine := "另有 1 个未绑定项目的旧会话，/resume --all 查看"
	if rsmCountLine(out, unboundLine) != 2 {
		t.Errorf("未绑定提示行出现 %d 次，期望恰好 2 次（横幅一次、默认 /resume 一次）；输出:\n%s", rsmCountLine(out, unboundLine), out)
	}
	allAt = strings.Index(out, "全部会话（")
	if allAt < 0 {
		t.Fatalf("输出里找不到 /resume --all 的表头「全部会话（」；输出:\n%s", out)
	}
	allOut = out[allAt:]
	if !strings.Contains(allOut, "(未绑定)") || !strings.Contains(allOut, "遗留会话甲") {
		t.Errorf("/resume --all 的输出里没有「(未绑定)」或「遗留会话甲」；后半:\n%s", allOut)
	}
}

func TestRsmRunTUIBranchSetsProjectDir(t *testing.T) {
	e := obxIsolate(t)
	root := e.root
	dataDir := filepath.Join(root, "appdata-rsm")
	cfgPath := writeTestConfig(t, root, validConfigBody(dataDir))
	proj := filepath.Join(root, "proj-tui")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatalf("建项目目录失败: %v", err)
	}
	t.Chdir(proj)

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
		_ = Run(ctx, Options{TUI: true, ConfigPath: cfgPath, InFile: inR, OutFile: outW, Err: outW})
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
	write("/resume", "/resume\r")
	if !flatHas(10*time.Second, "本项目1个会话") {
		t.Fatalf("/resume 之后 10 秒内没有看到「本项目1个会话」；flat %q", coxFlat(buf.String()))
	}
}
