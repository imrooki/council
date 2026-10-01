package main

// CH1 §2.4（F1）：-no-auto 启动参数确实登记了，默认值与说明文字逐字。照 cox_flag_test.go 的体例：
// run 先建 FlagSet、Parse、处理 -version / -h，之后才注册信号、碰进程的其余状态，所以下面三次调用都没有副作用。
// 本文件不引用任何新 Go 标识符，在 HEAD 上能编译，但 -no-auto 未登记，是运行期的红（第一条断言）。
//
// 背对背纪律：本文件由测试腿依据冻结契约 CH1 v1.7 与 HEAD dd0c15b 的源码写成，看不见实现腿的产出。

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// chxNoAutoUsage 是 -no-auto 在 flag.PrintDefaults 格式下的全部两行（名字一行、说明一行），由脚本从契约 §2.4 逐字生成。
const chxNoAutoUsage = "  -no-auto\n    \t直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）\n"

// TestChxNoAutoFlagRegistered 是 F1。
//
// 【判别力】第一条：flag 改名（M20）时 -no-auto 成了未定义的 flag，Parse 失败，返回 exitUsage，红；HEAD 上没有这个 flag，
// 同样在这一条红。第二条是对照：同一个 run、同样带 -version，一个确实没登记的 flag 必须走 exitUsage，证明第一条能为假。
// 第三条钉默认值与说明文字：默认改成 true（M20b）时 PrintDefaults 在说明后面补「 (default true)」，这一段不再以
// 「切换）\n」结尾，红；默认 false 时 PrintDefaults 不印「(default false)」，所以忠实实现上它为真。
// 【为什么是子串】用法块里还有别的 flag，整块逐字会被别的刀改红；这一段是 -no-auto 的全部两行，不是行的一截。
// 【判别不了的】NoAuto: *noAuto 是否真的传进 app.Options，只在审查时看（契约 §4）。杀 M20、M20b。
func TestChxNoAutoFlagRegistered(t *testing.T) {
	if got := run([]string{"-no-auto", "-version"}); got != exitOK {
		t.Fatalf("run(-no-auto -version) = %d，期望 exitOK(%d)：-no-auto 没有登记", got, exitOK)
	}
	if got := run([]string{"-chx-no-such-flag", "-version"}); got != exitUsage {
		t.Fatalf("对照：run(-chx-no-such-flag -version) = %d，期望 exitUsage(%d)", got, exitUsage)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提不成立：os.Pipe() 失败: %v", err)
	}
	var captured bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(&captured, r)
	}()
	saved := os.Stderr
	os.Stderr = w
	code := run([]string{"-h"})
	os.Stderr = saved
	_ = w.Close()
	<-readDone
	_ = r.Close()

	if code != exitOK {
		t.Fatalf("run(-h) = %d，期望 exitOK(%d)", code, exitOK)
	}
	if !strings.Contains(captured.String(), chxNoAutoUsage) {
		t.Fatalf("用法里没有逐字的 -no-auto 那两行\n用法: %q\n期望含: %q", captured.String(), chxNoAutoUsage)
	}
}
