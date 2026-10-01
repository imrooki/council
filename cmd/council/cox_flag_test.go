package main

// CO1 §2.5（L1）：-verbose 启动参数确实登记了，默认值与说明文字逐字。
// run 先建 FlagSet、Parse、处理 -version / -h，之后才注册信号、碰进程的其余状态，
// 所以下面三次调用都没有副作用。本文件不引用任何新 Go 标识符，在 HEAD 上能编译，
// 但 -verbose 未登记，是运行期的红（第一条断言）。

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// coxVerboseUsage 是 -verbose 在 flag.PrintDefaults 格式下的全部两行（名字一行、说明一行），逐字取自 §2.5。
const coxVerboseUsage = "  -verbose\n    \t详细输出：每轮的用量、金额、冷暖、provider 会话、预算表与结算全部显示（默认简洁；进去之后也可以用 /verbose 切换）\n"

// TestCoxVerboseFlagRegistered 是 L1。
//
// 【判别力】第一条：flag 改名（#76）时 -verbose 成了未定义的 flag，Parse 失败，返回 exitUsage，红；
// HEAD 上没有这个 flag，同样在这一条红。第二条是对照：同一个 run、同样带 -version，一个确实没登记的 flag
// 必须走 exitUsage，证明第一条能为假。第三条钉默认值与说明文字：默认改成 true（#81）时 PrintDefaults
// 在说明后面补「 (default true)」，这一段不再以「切换）\n」结尾，红；说明文字改一个字（#82）红。
// 默认 false 时 PrintDefaults 不印「(default false)」，所以忠实实现上它为真。
// 【为什么是子串】用法块里还有别的 flag（-config、-no-tui、-version，别的刀也可能加），整块逐字会被别的刀改红；
// 这一段是 -verbose 的全部两行，不是行的一截。
// 【判别不了的】Verbose: *verbose 是否真的传进 app.Options：run 要走到 app.Run 就得过了 -version、-h 两处返回，
// 之后硬编码进程标准流、注册进程级信号，只在审查时看。
func TestCoxVerboseFlagRegistered(t *testing.T) {
	if got := run([]string{"-verbose", "-version"}); got != exitOK {
		t.Fatalf("run(-verbose -version) = %d，期望 exitOK(%d)：-verbose 没有登记", got, exitOK)
	}
	if got := run([]string{"-cox-no-such-flag", "-version"}); got != exitUsage {
		t.Fatalf("对照：run(-cox-no-such-flag -version) = %d，期望 exitUsage(%d)", got, exitUsage)
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
	if !strings.Contains(captured.String(), coxVerboseUsage) {
		t.Fatalf("用法里没有逐字的 -verbose 那两行\n用法: %q\n期望含: %q", captured.String(), coxVerboseUsage)
	}
}
