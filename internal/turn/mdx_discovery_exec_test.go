package turn

// MD1 契约 §2.2：识别执行器 RunDiscovery。测试二进制自己扮演识别 CLI：
// 环境变量 MDX_FAKE_DISCOVERY 非空时 TestMain 不跑用例，按取值模拟一种 CLI。
// 本文件的 TestMain 是 turn 包唯一的一个（契约相交表：只有 MD1 在本包加 TestMain）。

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"council/internal/provider"
)

// mdxInitializeLine 是契约 §3.4 的 initialize 请求行（92 字节，含末尾换行）。
const mdxInitializeLine = `{"type":"control_request","request_id":"council-models","request":{"subtype":"initialize"}}` + "\n"

// mdxSystemFrame 是假 CLI 在 handshake 模式下打的第一行。
const mdxSystemFrame = `{"type":"system","subtype":"commands_changed"}` + "\n"

// mdxControlResponseFormat 是假 CLI 回的 control_response，%q 处填它读到的第一行。
const mdxControlResponseFormat = "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"sonnet\",\"resolvedModel\":\"claude-sonnet-5\",\"displayName\":\"Sonnet\"}]}},\"mdx_first\":%q}\n"

func TestMain(m *testing.M) {
	mode := os.Getenv("MDX_FAKE_DISCOVERY")
	if mode == "" {
		os.Exit(m.Run())
	}
	mdxFakeDiscoveryCLI(mode)
}

// mdxFakeDiscoveryCLI 按契约 §2.2 冻结的六种模式扮演识别 CLI，永不返回。
func mdxFakeDiscoveryCLI(mode string) {
	switch mode {
	case "handshake":
		// 四种执行器行为各走各的出口：写完就关 stdin 走 exit 7；stdout 来第一行
		// 就关走 exit 8；永不关等满超时；收到匹配的回应才关走 exit 0。
		br := bufio.NewReader(os.Stdin)
		first, _ := br.ReadString('\n')
		eof := make(chan struct{})
		go func() {
			_, _ = io.ReadAll(br)
			close(eof)
		}()
		select {
		case <-eof:
			_, _ = os.Stdout.WriteString("mdx-stdin-closed-before-response\n")
			os.Exit(7)
		case <-time.After(300 * time.Millisecond):
		}
		_, _ = os.Stdout.WriteString(mdxSystemFrame)
		select {
		case <-eof:
			_, _ = os.Stdout.WriteString("mdx-stdin-closed-after-system-frame\n")
			os.Exit(8)
		case <-time.After(300 * time.Millisecond):
		}
		_, _ = os.Stdout.WriteString(fmt.Sprintf(mdxControlResponseFormat, first))
		<-eof
		_, _ = os.Stdout.WriteString("mdx-after-eof\n")
		os.Exit(0)
	case "readall":
		b, _ := io.ReadAll(os.Stdin)
		_, _ = os.Stdout.WriteString(fmt.Sprintf("mdx-readall stdin=%d args=%q\n", len(b), os.Args[1:]))
		os.Exit(0)
	case "fail":
		_, _ = os.Stderr.WriteString("mdx first\nmdx last line\n\n")
		_, _ = os.Stdout.WriteString("mdx partial stdout\n")
		os.Exit(3)
	case "failsilent":
		os.Exit(4)
	case "hang":
		time.Sleep(60 * time.Second)
		os.Exit(0)
	case "cwd":
		wd, _ := os.Getwd()
		_, _ = os.Stdout.WriteString(wd)
		os.Exit(0)
	default:
		_, _ = os.Stderr.WriteString("mdx: unknown MDX_FAKE_DISCOVERY mode " + mode + "\n")
		os.Exit(2)
	}
}

// TestMdxRunDiscoveryClaudeHandshake 覆盖 T1：claude 的握手要收到匹配的
// control_response 才关 stdin。
//
// 【判别力】执行器写完就关 stdin（M24）或 claude 的 CloseStdinAfter 为 nil
// （M20）时假 CLI 走 exit 7；stdout 来第一行就关 stdin、不看是不是匹配的回应
// （M76）时假 CLI 走 exit 8；永不关 stdin 时等满 10 s 超时。这三种写法下
// err 都非 nil、out 也不等。mdx_first 是假 CLI 读到的第一行，逐字等于 §3.4。
func TestMdxRunDiscoveryClaudeHandshake(t *testing.T) {
	if len(mdxInitializeLine) != 92 {
		t.Fatalf("initialize 请求行应为 92 字节，实际 %d", len(mdxInitializeLine))
	}
	t.Setenv("MDX_FAKE_DISCOVERY", "handshake")
	cmd, err := provider.DiscoveryCommandFor("claude", os.Args[0])
	if err != nil {
		t.Fatalf("DiscoveryCommandFor(claude): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := RunDiscovery(ctx, cmd)
	if err != nil {
		t.Errorf("RunDiscovery: err = %v，期望 nil；out = %q", err, out)
	}
	want := mdxSystemFrame + fmt.Sprintf(mdxControlResponseFormat, mdxInitializeLine) + "mdx-after-eof\n"
	if string(out) != want {
		t.Errorf("stdout 不符\n实际: %q\n期望: %q", out, want)
	}
	models, perr := provider.ParseDiscovery("claude", out)
	if perr != nil || len(models) != 1 || models[0].Model != "claude-sonnet-5" {
		t.Errorf("ParseDiscovery(claude, out) = %#v, %v；期望 1 行 claude-sonnet-5", models, perr)
	}
}

// TestMdxRunDiscoveryClosesStdinImmediately 覆盖 T2：CloseStdinAfter 为 nil 时
// 写完 Stdin 立刻关。
//
// 【判别力】不关 stdin（M23）时 readall 模式读不到 EOF，10 s 超时报错。
// 第二次给了 3 字节 stdin，stdin=3 钉住「先写完再关」。
func TestMdxRunDiscoveryClosesStdinImmediately(t *testing.T) {
	t.Setenv("MDX_FAKE_DISCOVERY", "readall")
	cmd, err := provider.DiscoveryCommandFor("agy", os.Args[0])
	if err != nil {
		t.Fatalf("DiscoveryCommandFor(agy): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := RunDiscovery(ctx, cmd)
	if err != nil {
		t.Errorf("agy: err = %v，期望 nil", err)
	}
	if want := "mdx-readall stdin=0 args=[\"--output-format\" \"json\" \"models\"]\n"; string(out) != want {
		t.Errorf("agy: stdout = %q，期望 %q", out, want)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	out, err = RunDiscovery(ctx2, provider.DiscoveryCommand{Path: os.Args[0], Args: []string{"x"}, Stdin: []byte("abc")})
	if err != nil {
		t.Errorf("abc: err = %v，期望 nil", err)
	}
	if want := "mdx-readall stdin=3 args=[\"x\"]\n"; string(out) != want {
		t.Errorf("abc: stdout = %q，期望 %q", out, want)
	}
}

// TestMdxRunDiscoveryNonZeroExit 覆盖 T3：退出码非 0 时错误文本带 stderr 最后
// 一个非空行，stdout 照样返回。
//
// 【判别力】不看退出码（M26）时 err 为 nil；不收 stderr（M27）时错误文本少了
// ": mdx last line"。stderr 末尾有空行，取「最后一行」而不是「最后一个非空行」
// 的写法在这里得到空串、文本不等。
func TestMdxRunDiscoveryNonZeroExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Setenv("MDX_FAKE_DISCOVERY", "fail")
	out, err := RunDiscovery(ctx, provider.DiscoveryCommand{Path: os.Args[0]})
	if err == nil || err.Error() != "exit code 3: mdx last line" {
		t.Errorf("fail: err = %v，期望 %q", err, "exit code 3: mdx last line")
	}
	if string(out) != "mdx partial stdout\n" {
		t.Errorf("fail: stdout = %q，期望 %q", out, "mdx partial stdout\n")
	}

	t.Setenv("MDX_FAKE_DISCOVERY", "failsilent")
	out, err = RunDiscovery(ctx, provider.DiscoveryCommand{Path: os.Args[0]})
	if err == nil || err.Error() != "exit code 4" {
		t.Errorf("failsilent: err = %v，期望 %q", err, "exit code 4")
	}
	if len(out) != 0 {
		t.Errorf("failsilent: stdout = %q，期望空", out)
	}
}

// TestMdxRunDiscoveryTimeoutKillsTree 覆盖 T4：超时杀进程树，立刻返回。
//
// 【判别力】不杀进程（M25）时要等满 discoveryKillGrace（5 s）才返回，耗时
// 断言红。错误要用 %w 包住 ctx.Err()：errors.Is 与整句文本都钉着。
func TestMdxRunDiscoveryTimeoutKillsTree(t *testing.T) {
	t.Setenv("MDX_FAKE_DISCOVERY", "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	out, err := RunDiscovery(ctx, provider.DiscoveryCommand{Path: os.Args[0]})
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v，期望包住 context.DeadlineExceeded", err)
	}
	if err == nil || err.Error() != "model discovery killed: context deadline exceeded" {
		t.Errorf("err = %v，期望 %q", err, "model discovery killed: context deadline exceeded")
	}
	if out != nil {
		t.Errorf("out = %q，期望 nil", out)
	}
	if elapsed >= 4*time.Second {
		t.Errorf("耗时 %v，期望 < 4s（超时应当立刻杀进程）", elapsed)
	}
}

// TestMdxRunDiscoveryWorkDirIsTemp 覆盖 T5：工作目录是 os.TempDir()。
//
// 【判别力】不设 Dir（M28）时子进程继承测试的工作目录（包目录），与
// os.TempDir() 不等。
func TestMdxRunDiscoveryWorkDirIsTemp(t *testing.T) {
	t.Setenv("MDX_FAKE_DISCOVERY", "cwd")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := RunDiscovery(ctx, provider.DiscoveryCommand{Path: os.Args[0]})
	if err != nil {
		t.Fatalf("err = %v，期望 nil", err)
	}
	if got, want := filepath.Clean(string(out)), filepath.Clean(os.TempDir()); got != want {
		t.Errorf("子进程工作目录 = %q，期望 %q", got, want)
	}
}

// TestMdxRunDiscoveryStartErrors 覆盖 T6：路径为空与可执行文件不存在。
//
// 【判别力】第一条逐字比文本；第二条只断言 err 非 nil（文本随系统，契约不比），
// 结构上只能抓「吞掉启动错误」这一种写法。
func TestMdxRunDiscoveryStartErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := RunDiscovery(ctx, provider.DiscoveryCommand{})
	if err == nil || err.Error() != "discovery command path is empty" {
		t.Errorf("空路径: err = %v，期望 %q", err, "discovery command path is empty")
	}
	missing := filepath.Join(t.TempDir(), "mdx-no-such.exe")
	if _, err := RunDiscovery(ctx, provider.DiscoveryCommand{Path: missing}); err == nil {
		t.Errorf("不存在的 %s: err 为 nil，期望非 nil", missing)
	}
}
