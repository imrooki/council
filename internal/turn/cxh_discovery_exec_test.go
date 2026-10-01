package turn

// CXH 契约 §2.2：codex 的识别命令经 Council 的真执行器 RunDiscovery 落到子进程
// argv。测试二进制自己扮演 CLI（MD1 的 TestMain 与 MDX_FAKE_DISCOVERY=readall，
// 见 mdx_discovery_exec_test.go）；本文件不新增 TestMain，不起真 codex。

import (
	"context"
	"os"
	"testing"
	"time"

	"council/internal/provider"
)

// TestCxhRunDiscoveryCodexCommand 覆盖 T1。
//
// 【判别力】这是 codex 的命令经真执行器落到子进程 argv 的唯一一条：少了 debug
// （C2）时 args 不等；codex 一支删掉（C5）时 DiscoveryCommandFor 报错；Path 写死
// 成 cxh-nosuch-codex（C4）时 RunDiscovery 报 executable file not found、stdout
// 空；给了非空 stdin 的写法 stdin= 不是 0。
func TestCxhRunDiscoveryCodexCommand(t *testing.T) {
	t.Setenv("MDX_FAKE_DISCOVERY", "readall")
	cmd, err := provider.DiscoveryCommandFor("codex", os.Args[0])
	if err != nil {
		t.Fatalf("DiscoveryCommandFor(codex): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := RunDiscovery(ctx, cmd)
	if err != nil {
		t.Errorf("RunDiscovery(codex): err = %v，期望 nil", err)
	}
	if want := "mdx-readall stdin=0 args=[\"debug\" \"models\"]\n"; string(out) != want {
		t.Errorf("codex: stdout = %q，期望 %q", out, want)
	}
}
