package serve

// app_usage_test.go 是 `-app` 在 flag 层与用法文本上的证明：flag 必须被
// Parse 认得（不能落到「未知 flag」），用法里必须写清窗口会活过 serve 退出
// 这件与直觉相反的事（需求要求写进用法）。

import (
	"strings"
	"testing"
)

// TestParseAppFlag 钉住 -app 的解析与默认值。同时给 -open 不算用法错误：
// 互斥的口径是「-app 优先」，由 openDashboard 打一行说明，不是退 2。
func TestParseAppFlag(t *testing.T) {
	opts, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil) 失败: %v", err)
	}
	if opts.App {
		t.Error("默认 -app 应当为 false")
	}

	opts, err = Parse([]string{"-app"})
	if err != nil {
		t.Fatalf("Parse(-app) 失败: %v", err)
	}
	if !opts.App || opts.Open {
		t.Errorf("Parse(-app) = %+v，期望 App=true、Open=false", opts)
	}

	opts, err = Parse([]string{"-app", "-open"})
	if err != nil {
		t.Fatalf("Parse(-app -open) 失败: %v", err)
	}
	if !opts.App || !opts.Open {
		t.Errorf("Parse(-app -open) = %+v，两个 flag 都应当为 true（互斥在打开时处理）", opts)
	}
}

// TestUsageDocumentsAppWindow 钉住用法文本里的三件事：窗口模式本身、
// 独立 profile 的位置、以及窗口是独立进程（serve 退出后仍然开着）。
func TestUsageDocumentsAppWindow(t *testing.T) {
	usage := Usage()
	for _, want := range []string{
		"-app",
		"Edge",
		"Chrome",
		"没有地址栏",
		"ui-profile",
		"1440x900",
		"serve 退出之后",
		"仍然开着",
	} {
		if !strings.Contains(usage, want) {
			t.Errorf("Usage() 里缺少 %q:\n%s", want, usage)
		}
	}
}
