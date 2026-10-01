package turn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/provider"
)

// obx_loginstatus_exec_test.go 是 OB1 v1.1（契约 §2.5）在 turn 包的验收：RunLoginStatus
// 照实交回退出码与两路输出，被 ctx 杀掉时返回包着 ctx 错误的 err。假 CLI 是只用 cmd
// 内建命令的 .cmd；PATH 不改（CL1 起超时杀树只起系统目录里的 taskkill.exe，与 PATH 无关）。

// obxLoopLimit 是 killed 子测试等待循环的计数上限，与 app 包 §2.0 的同名常量同值：保证循环一定会停。
const obxLoopLimit = "60000"

// obxLoopLines 是一个会自己停的等待循环：cond 为真时继续等，计数到 obxLoopLimit 就以 0 退出。
func obxLoopLines(cond string, then ...string) []string {
	lines := []string{"@echo off", "set n=0", ":obxloop", "set /a n+=1", "if %n% geq " + obxLoopLimit + " exit /b 0", "if " + cond + " goto obxloop"}
	return append(lines, then...)
}

// obxStatusFake 在一个临时目录里写 obx-status.cmd（各行以 \r\n 连接、末尾再加 \r\n），返回路径。
func obxStatusFake(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "obx-status.cmd")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0o644); err != nil {
		t.Fatalf("写假 CLI: %v", err)
	}
	return p
}

// TestObxRunLoginStatus 是 L4。
// 【判别力】M91（非 0 退出当错误）红 exit1_stderr、stdout_and_args；M92（丢 stderr）红 exit0_stderr、
// exit1_stderr；M93（%w 换 %v）红 killed。exit0_stderr 是 RunLoginStatus 存在的理由：RunDiscovery
// 在退出码 0 时不交回 stderr。
func TestObxRunLoginStatus(t *testing.T) {
	args := []string{"login", "status"}
	for _, c := range []struct {
		name           string
		lines          []string
		code           int
		stdout, stderr string
	}{
		{"exit0_stderr", []string{"@echo Logged in using a subscription 1>&2", "@exit /b 0"}, 0, "", "Logged in using a subscription \r\n"},
		{"exit1_stderr", []string{"@echo Not logged in 1>&2", "@exit /b 1"}, 1, "", "Not logged in \r\n"},
		{"stdout_and_args", []string{"@echo %1 %2", "@exit /b 7"}, 7, "login status\r\n", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := obxStatusFake(t, c.lines...)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			code, so, se, err := RunLoginStatus(ctx, provider.DiscoveryCommand{Path: p, Args: args})
			if err != nil {
				t.Fatalf("err 应当是 nil，实际 %v", err)
			}
			if code != c.code {
				t.Errorf("退出码应当是 %d，实际 %d", c.code, code)
			}
			if string(so) != c.stdout {
				t.Errorf("stdout 应当是 %q，实际 %q", c.stdout, so)
			}
			if string(se) != c.stderr {
				t.Errorf("stderr 应当是 %q，实际 %q", c.stderr, se)
			}
		})
	}
	t.Run("killed", func(t *testing.T) {
		dir := t.TempDir()
		on := filepath.Join(dir, "loop.on")
		if err := os.WriteFile(on, []byte("x"), 0o644); err != nil {
			t.Fatalf("写 loop.on: %v", err)
		}
		p := obxStatusFake(t, obxLoopLines(`exist "`+on+`"`)...)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		_, _, _, err := RunLoginStatus(ctx, provider.DiscoveryCommand{Path: p, Args: args})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("被 ctx 杀掉时 err 应当满足 errors.Is(err, context.DeadlineExceeded)，实际 %v", err)
		}
	})
	t.Run("empty_path", func(t *testing.T) {
		_, _, _, err := RunLoginStatus(context.Background(), provider.DiscoveryCommand{})
		if err == nil {
			t.Errorf("Path 为空时 err 应当非 nil")
		}
	})
}
