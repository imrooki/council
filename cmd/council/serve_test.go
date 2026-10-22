package main

// SRV1 §3 的分派证明：从 run([]string{"serve", ...}) 进，确认 serve 在
// 全局 flag 解析之前被拦下，私有 flag 由 internal/serve 这一套解析。
// 这些用例都走不到监听：回环校验与 -h 都在 net.Listen 之前返回，
// 所以不会在测试里留下一个真实端口。

import (
	"strings"
	"testing"
)

func TestServeDispatchNonLoopbackIsUsageError(t *testing.T) {
	_, stderr, code := captureRun(t, func() int {
		return run([]string{"serve", "-addr", "0.0.0.0:8765"})
	})
	if code != exitUsage {
		t.Fatalf("run(serve -addr 0.0.0.0:8765) = %d，期望 %d；stderr:\n%s", code, exitUsage, stderr)
	}
	if !strings.Contains(stderr, "回环") {
		t.Errorf("stderr 应当说明只允许回环地址，实际:\n%s", stderr)
	}
	// 若 serve 没被提前拦下，这一行会变成全局 FlagSet 的「不接受位置参数」。
	if strings.Contains(stderr, "不接受位置参数") {
		t.Errorf("serve 没有在全局 flag 解析之前被拦下，stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "用法: council serve") {
		t.Errorf("退 2 时应当打印 serve 的用法，实际:\n%s", stderr)
	}
}

func TestServeDispatchHelp(t *testing.T) {
	_, stderr, code := captureRun(t, func() int {
		return run([]string{"serve", "-h"})
	})
	if code != exitOK {
		t.Fatalf("run(serve -h) = %d，期望 %d", code, exitOK)
	}
	for _, want := range []string{"用法: council serve", "-addr", "-open", "-config", "-verbose"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("serve 用法里缺少 %q，实际:\n%s", want, stderr)
		}
	}
}

func TestServeDispatchRejectsUnknownFlagAndPositional(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "未知 flag", args: []string{"serve", "-nope"}},
		{name: "位置参数", args: []string{"serve", "extra"}},
		{name: "非法端口", args: []string{"serve", "-addr", "127.0.0.1:abc"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, code := captureRun(t, func() int { return run(tc.args) })
			if code != exitUsage {
				t.Fatalf("run(%v) = %d，期望 %d；stderr:\n%s", tc.args, code, exitUsage, stderr)
			}
			if !strings.Contains(stderr, "用法: council serve") {
				t.Errorf("stderr 应当打印 serve 的用法，实际:\n%s", stderr)
			}
		})
	}
}
