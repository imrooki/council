package localagent

// safety_test.go 是数据最小化（S-09）的机器可读证据。
//
// 规则表与枚举器都只允许碰「exe 名 / PID / PPID / 启动时间」四个字段，
// 但「代码里没有出现读命令行 / 读环境变量 / 读文件 / 起子进程的调用」
// 这件事靠人 review 容易漏。这里逐条扫描本包的生产源码，发现禁用符号
// 就红。只在测试里读文件；生产代码一次文件都不读。

import (
	"os"
	"strings"
	"testing"
)

// TestProductionSourcesStayReadOnly 的禁用清单里**没有**
// QueryFullProcessImageName：映像完整路径是 exact/probable 判定的必需
// 证据（Claude Desktop 也装了 claude.exe），它只说明程序装在哪个目录，
// 不是命令行、也不读文件内容。其余采集面仍然一条都不许出现。
func TestProductionSourcesStayReadOnly(t *testing.T) {
	forbidden := []string{
		"os.Getenv",         // 环境变量（S-09）
		"os.ReadFile",       // 任何第三方配置文件（S-02 / S-03）
		"os.Open(",          // 同上
		"os.OpenFile",       // 不写任何文件
		"exec.Command",      // 不起子进程（S-07）
		"GetCommandLine",    // 完整命令行（S-09）
		"CommandLineToArgv", // 同上
		"ReadProcessMemory", // 进程内存（S-09）
		"GetWindowText",     // 窗口标题（S-09）
		"clipboard",         // 剪贴板（S-09）
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读包目录失败: %v", err)
	}
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", name, err)
		}
		text := string(body)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Errorf("%s 里出现了禁用调用 %q；本包只允许枚举 exe 名 / PID / PPID / 启动时间", name, token)
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("一个生产源文件都没扫到；这条用例失去了判别力")
	}
}
