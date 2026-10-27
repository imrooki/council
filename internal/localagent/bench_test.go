package localagent

// bench_test.go 对应 Plan §12 的性能行：「200 个未知进程 → 单次扫描 < 100 ms，
// 不阻塞 HTTP handler」。
//
// 这里量的是**纯 CPU 的识别**（Identify），不含进程枚举的系统调用：
// Toolhelp32 那一段由 Windows 决定，本机实测在几十毫秒量级，把它算进这条
// 断言里会让用例随机器负载漂。枚举本身在端点里是同步执行的一次快照，
// 超时由 localAgentScanTimeout 兜住。

import (
	"fmt"
	"testing"
	"time"
)

// tstManyProcesses 造 n 条「认不出的通用宿主」进程：它们走的是规则表里
// 最长的一条路径（每个都要过一遍规则），比全是 claude.exe 更接近真实负载。
func tstManyProcesses(n int) []Process {
	processes := make([]Process, 0, n)
	for i := 0; i < n; i++ {
		processes = append(processes, Process{
			PID:     10000 + i,
			PPID:    1,
			ExeName: "node.exe",
			ExePath: fmt.Sprintf(`C:\Program Files\nodejs\node.exe`),
		})
	}
	// 掺几个真正的 CLI，让 Identify 的返回路径也走一遍。
	processes = append(processes,
		Process{PID: 1, ExeName: "claude.exe", ExePath: `C:\Users\u\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\bin\claude.exe`},
		Process{PID: 2, ExeName: "codex.exe", ExePath: `C:\Users\u\AppData\Local\Programs\OpenAI\Codex\bin\codex.exe`},
		Process{PID: 3, ExeName: "grok.exe", ExePath: `C:\Users\u\.grok\bin\grok.exe`},
	)
	return processes
}

// TestIdentify200ProcessesIsFast 是 §12 的那条验收：200 条进程的识别必须在
// 100 ms 内完成。这个上界对着实测余量极大（本机在微秒级），所以它不会成为
// 一条 flaky 用例；真正的价值是在有人往规则表里塞正则或读写盘时立刻红。
func TestIdentify200ProcessesIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("性能用例在 -short 下跳过")
	}
	processes := tstManyProcesses(200)

	start := time.Now()
	agents := Identify(processes)
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Fatalf("识别 200 条进程用了 %s，超过 §12 的 100 ms 上限", elapsed)
	}
	// 只有那三条 CLI 会被确认；node.exe 一律不确认（L1 的规则）。
	if len(agents) != 3 {
		t.Errorf("识别出 %d 条，期望 3 条（node.exe 不算）", len(agents))
	}
	t.Logf("识别 200 条进程耗时 %s", elapsed)
}

// BenchmarkIdentify200Processes 给后续改动一个基线数字。
func BenchmarkIdentify200Processes(b *testing.B) {
	processes := tstManyProcesses(200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Identify(processes)
	}
}

// BenchmarkObserve200Processes 把「枚举 + 识别」整条按需扫描路径也量一遍：
// 枚举用一个固定快照替代（真枚举要碰 Windows API，基准里不该混进系统调用）。
func BenchmarkObserve200Processes(b *testing.B) {
	processes := tstManyProcesses(200)
	enum := func() ([]Process, error) { return processes, nil }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obs, err := Observe(LevelObserve, enum)
		if err != nil {
			b.Fatalf("Observe: %v", err)
		}
		if len(obs.Agents) != 3 {
			b.Fatalf("agents = %d", len(obs.Agents))
		}
	}
}
