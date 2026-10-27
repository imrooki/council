//go:build windows

package localagent

// detect_windows_test.go 只做一件事：真实调用一次 Toolhelp32 + 取启动时间，
// 证明这条系统调用路径在本机可用、当前进程能被枚举到。
//
// 它不断言「本机没有 agent」或「有几个 agent」：开发机与 CI 上跑着什么是
// 不可控的，那样的断言只会变成随机红。识别逻辑的判别力在 rules_test.go。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnumerateSeesCurrentProcess(t *testing.T) {
	processes, err := Enumerate()
	if err != nil {
		t.Fatalf("枚举本机进程失败: %v", err)
	}
	if len(processes) == 0 {
		t.Fatal("进程快照是空的；至少应当包含当前测试进程")
	}

	want := os.Getpid()
	var self *Process
	for i := range processes {
		if processes[i].PID == want {
			self = &processes[i]
			break
		}
	}
	if self == nil {
		t.Fatalf("快照里没有当前进程 pid=%d（共 %d 条）", want, len(processes))
	}
	if self.ExeName == "" {
		t.Errorf("当前进程的 ExeName 是空的（pid=%d）", want)
	}
	if self.ExePath == "" {
		t.Errorf("当前进程的 ExePath 是空的（pid=%d）；映像路径是 exact/probable 判定的依据", want)
	}
	if base := filepath.Base(self.ExePath); !strings.EqualFold(base, self.ExeName) {
		t.Errorf("ExePath 的基名 %q 与 ExeName %q 不一致", base, self.ExeName)
	}
	if self.StartedAt.IsZero() {
		t.Errorf("当前进程的启动时间拿不到（pid=%d）；自己的进程应当能 OpenProcess 成功", want)
	}
	if len(self.ExeName) >= 260 {
		t.Errorf("ExeName 长度 %d，超出 MAX_PATH：%q", len(self.ExeName), self.ExeName)
	}
}
