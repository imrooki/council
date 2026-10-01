package app

// CH1 §2.3：真 app.Run、真 store、真 config.Load、假 CLI（本包 smoke_test.go 的 TestMain）。
// 不用 runAppForTest（本刀落地后它固定 NoAuto: true）。
//
// 背对背纪律：本文件由测试腿依据冻结契约 CH1 v1.7 与 HEAD dd0c15b 的源码写成，看不见实现腿的产出。
// 引用本刀新增的 Options.NoAuto，在 HEAD 上整包编译不过，是契约 §2 写明的预期红。

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// chxAppCfgBody 是 §2.3 冻结的配置文本（照 smx_summary_app_test.go 的体例）。
func chxAppCfgBody(dataDir string) string {
	return fmt.Sprintf(`data_dir = %s

[providers.claude]
command = %s

[agents.c1]
display_name = "成员一"
provider = "claude"
model = "opus"
effort = "high"
role = "chx"
permissions = "read"
mode = "discussion"

[agents.c2]
display_name = "成员二"
provider = "claude"
model = "opus"
effort = "high"
role = "chx"
permissions = "read"
mode = "discussion"

[agents.sm]
display_name = "总结"
provider = "claude"
model = "opus"
effort = "high"
permissions = "read"
mode = "discussion"

[summary]
agent = "sm"
`, tomlLiteral(dataDir), tomlLiteral(os.Args[0]))
}

// chxAppSetup：隔离环境、假 CLI、写配置，返回配置路径。
func chxAppSetup(t *testing.T) string {
	t.Helper()
	isolateAppEnv(t)
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	dir := t.TempDir()
	return writeTestConfig(t, dir, chxAppCfgBody(filepath.Join(dir, "data")))
}

var chxAppTotalRe = regexp.MustCompile(`^总用时 \d+\.\ds$`)

// TestChxAppDefaultAutoLine 是 A1：行式分支下 app 默认是对话模式，NoAuto 关得掉。
//
// 【判别力】两个子测试只差 NoAuto。行式分支不调 SetAutoReply（M17）时 default 的尾部只剩「再见」，红；
// 写反（M19a）时两个都红；sayText 不看开关（M6，auto := bare）时 no_auto 多出一轮，红。
// no_auto 的尾部是缺席断言，正对照就是 default。第 2、3 行排序后比（两个成员并发回来，先后不固定）。杀 M6、M17、M19a。
func TestChxAppDefaultAutoLine(t *testing.T) {
	const script = "/new t\n/join c1\n/join c2\n你好\n/quit\n"
	run := func(t *testing.T, noAuto bool) []string {
		t.Helper()
		cfgPath := chxAppSetup(t)
		var out, errOut bytes.Buffer
		if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, NoAuto: noAuto}); err != nil {
			t.Fatalf("app.Run 返回 %v；Out:\n%s\nErr:\n%s", err, out.String(), errOut.String())
		}
		tail := smxAppTail(out.String())
		for i, l := range tail {
			if chxAppTotalRe.MatchString(l) {
				tail[i] = "总用时 T"
			}
		}
		return tail
	}
	t.Run("default", func(t *testing.T) {
		tail := run(t, false)
		if len(tail) != 8 {
			t.Fatalf("尾部应当恰好 8 行，实际 %d 行: %q", len(tail), tail)
		}
		mid := []string{tail[1], tail[2]}
		sort.Strings(mid)
		tail[1], tail[2] = mid[0], mid[1]
		want := []string{"本轮问 2 个: c1, c2", "成员一（T）: OK", "成员二（T）: OK", "本轮问 1 个: sm", "总的来看（总结 做总结）：OK", "总用时 T", "再见", ""}
		if strings.Join(tail, "\n") != strings.Join(want, "\n") {
			t.Fatalf("默认（对话模式）的尾部不对\n实际: %q\n期望: %q", tail, want)
		}
	})
	t.Run("no_auto", func(t *testing.T) {
		tail := run(t, true)
		want := []string{"再见", ""}
		if strings.Join(tail, "\n") != strings.Join(want, "\n") || len(tail) != len(want) {
			t.Fatalf("NoAuto 的尾部不对\n实际: %q\n期望: %q", tail, want)
		}
	})
}

var chxConvRe = regexp.MustCompile(`已建会话 (\S+)，标题: t`)

// chxRunTui 是 A2 的装置：第 0 段行式建会话（不设 NoAuto，没有裸文本），第 1 段 TUI 模式 Run 经 os.Pipe 进同一个会话，
// 敲「你好」，等这一次提交回到空闲，再敲 /who 作为缺席一侧的同步点。返回第 1 段 flat 里从第一个「>你好」起的后半段。
// 同步点照 coxRunTui（cox_app_test.go）。
func chxRunTui(t *testing.T, noAuto bool) string {
	t.Helper()
	cfgPath := chxAppSetup(t)

	var out0, err0 bytes.Buffer
	if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader("/new t\n/join c1\n/quit\n"), Out: &out0, Err: &err0}); err != nil {
		t.Fatalf("前提不成立：第 0 段 Run 返回 %v；Out:\n%s\nErr:\n%s", err, out0.String(), err0.String())
	}
	m := chxConvRe.FindStringSubmatch(out0.String())
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
		runDone <- Run(ctx, Options{ConfigPath: cfgPath, TUI: true, InFile: inR, OutFile: outW, Err: errOut, NoAuto: noAuto})
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
	write("你好", "你好\r")
	if !coxWaitFor(buf, 15*time.Second, func(raw string) bool {
		f := coxFlat(raw)
		i := strings.Index(f, ">你好")
		return i >= 0 && strings.Contains(f[i:], "council空闲")
	}) {
		t.Fatalf("前提不成立：15 秒内没有看到「>你好」之后回到空闲；Err:\n%s\nflat %q", errOut.String(), coxFlat(buf.String()))
	}
	write("/who", "/who\r")
	if !flatHas(5*time.Second, "AGENTJOINED_PROVIDER") {
		t.Fatalf("前提不成立：5 秒内没有看到 /who 的表头；Err:\n%s\nflat %q", errOut.String(), coxFlat(buf.String()))
	}
	teardown()
	flat := coxFlat(buf.String())
	i := strings.Index(flat, ">你好")
	if i < 0 {
		t.Fatalf("前提不成立：flat 里没有「>你好」；flat %q", flat)
	}
	return flat[i:]
}

var (
	chxTuiHeadRe  = regexp.MustCompile(`成员一（\d+\.\ds）:OK`)
	chxTuiFinalRe = regexp.MustCompile(`/round结束\d+s回来1/1`)
)

// TestChxAppDefaultAutoTui 是 A2：TUI 分支（经真 tui.Run）下 app 默认是对话模式，NoAuto 关得掉。
//
// 【判别力】no_auto 的三条缺席断言与 default 同装置、只差 NoAuto；/who 是缺席一侧的同步点（命令串行，/who 的表头
// 出现时「你好」那次提交一定已经结束，所以缺席不是没等够）。TUI 分支不调 SetAutoReply（M18）或 Submit 不置 bare（M2）
// 时 default 红；写死 SetAutoReply(true)（M19b）时 no_auto 红。杀 M2、M18、M19b。
// 【为什么是子串】断言的是终端字节流，帧与帧之间夹着光标移动与重绘，整串相等做不到；剥 ANSI、删空格之后找的每一句
// 都是一整行的全部内容（本轮问、成员头连正文、收尾行）。
func TestChxAppDefaultAutoTui(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		after := chxRunTui(t, false)
		if !strings.Contains(after, "本轮问1个:c1") {
			t.Fatalf("默认（对话模式）应当有「本轮问1个:c1」；flat 后半段 %q", after)
		}
		if !chxTuiHeadRe.MatchString(after) {
			t.Fatalf("默认（对话模式）应当有成员头「成员一（N.Ns）:OK」；flat 后半段 %q", after)
		}
		if !chxTuiFinalRe.MatchString(after) {
			t.Fatalf("默认（对话模式）应当有收尾行「/round 结束 Ns 回来 1/1」；flat 后半段 %q", after)
		}
	})
	t.Run("no_auto", func(t *testing.T) {
		after := chxRunTui(t, true)
		if strings.Contains(after, "本轮问") {
			t.Fatalf("NoAuto 时不应当有「本轮问」；flat 后半段 %q", after)
		}
		if chxTuiHeadRe.MatchString(after) {
			t.Fatalf("NoAuto 时不应当有成员头「成员一（N.Ns）:OK」；flat 后半段 %q", after)
		}
		if strings.Contains(after, "/round结束") {
			t.Fatalf("NoAuto 时不应当有「/round结束」；flat 后半段 %q", after)
		}
	})
}
