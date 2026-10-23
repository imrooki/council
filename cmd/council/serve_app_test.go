package main

// `-app` 的分派证明：它必须在 serve 的私有 flag 里登记，不能落到全局
// FlagSet 变成「未知 flag」。仍然沿用 serve_test.go 的办法——用非回环
// 地址让 serve 在 net.Listen 之前退 2，测试里不会留下真实端口、更不会
// 拉起浏览器。

import (
	"strings"
	"testing"
)

func TestServeDispatchAcceptsAppFlag(t *testing.T) {
	_, stderr, code := captureRun(t, func() int {
		return run([]string{"serve", "-app", "-addr", "0.0.0.0:8765"})
	})
	if code != exitUsage {
		t.Fatalf("run(serve -app -addr 0.0.0.0:8765) = %d，期望 %d；stderr:\n%s", code, exitUsage, stderr)
	}
	// 走到回环校验才说明 -app 被 serve.Parse 认下；未知 flag 会在那之前就退出。
	if !strings.Contains(stderr, "回环") {
		t.Errorf("-app 没有作为 serve 的私有 flag 被接受，stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, "not defined") {
		t.Errorf("stderr 出现了未知 flag 的报错:\n%s", stderr)
	}
}

func TestServeHelpMentionsAppFlag(t *testing.T) {
	_, stderr, code := captureRun(t, func() int {
		return run([]string{"serve", "-h"})
	})
	if code != exitOK {
		t.Fatalf("run(serve -h) = %d，期望 %d", code, exitOK)
	}
	for _, want := range []string{"-app", "Edge", "Chrome"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("serve 用法里缺少 %q，实际:\n%s", want, stderr)
		}
	}
}
