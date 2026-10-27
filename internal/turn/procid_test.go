package turn

// procid_test.go 覆盖 Plan §4.3 ⑨ 的 PID 钩子：生产 Spawner 真的把 PID 交出来、
// Ask 把钩子挂在它自己那次 spawn 的 ctx 上、零值（不设 OnProcessStart）时
// 一个回调都不发生。

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"council/internal/provider"
)

// TestSpawnProcessReportsPID 走真实进程：spawnProcess 必须把子进程 PID 交给
// 挂在 ctx 上的观察者，而且只交一次。
//
// 用测试二进制自己当子进程（-test.run=^$ 让它立刻退出）：不需要外部依赖，
// 也不会真的跑一个模型 CLI。
func TestSpawnProcessReportsPID(t *testing.T) {
	var pids []int
	ctx := WithProcessIDObserver(context.Background(), func(pid int) {
		pids = append(pids, pid)
	})

	_, _, exitCode, killed, err := spawnProcess(ctx, provider.Command{
		Path: os.Args[0],
		Args: []string{"-test.run=^$"},
		Dir:  filepath.Dir(os.Args[0]),
	})
	if err != nil {
		t.Fatalf("spawnProcess 失败: %v", err)
	}
	if killed {
		t.Fatal("子进程不该被杀")
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d，期望 0", exitCode)
	}
	if len(pids) != 1 {
		t.Fatalf("PID 回调被调 %d 次，期望 1 次：%v", len(pids), pids)
	}
	if pids[0] <= 0 {
		t.Errorf("报告的 PID = %d，期望正数", pids[0])
	}
}

// TestSpawnProcessWithoutObserverIsSilent 是上一条的对照：不挂观察者时
// 一切照旧（零值不改行为）。
func TestSpawnProcessWithoutObserverIsSilent(t *testing.T) {
	_, _, exitCode, _, err := spawnProcess(context.Background(), provider.Command{
		Path: os.Args[0],
		Args: []string{"-test.run=^$"},
		Dir:  filepath.Dir(os.Args[0]),
	})
	if err != nil || exitCode != 0 {
		t.Fatalf("没有观察者时 spawnProcess = exit %d, err %v", exitCode, err)
	}
}

// TestRunnerWiresProcessObserverIntoContext 钉住 Runner 那一侧：Ask 必须把
// 「回调到 OnProcessStart」挂进它交给 Spawner 的那一个 ctx，并且带上正确的
// 会话与成员。
func TestRunnerWiresProcessObserverIntoContext(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "钩子测试")

	fs := &fakeSpawner{}
	fs.enqueueSuccess(minimalClaudeSuccess("SESSION-PID", "resp-pid"))
	r.spawn = fs.spawn

	var (
		gotConv  string
		gotAgent string
		gotPID   int
		calls    int
	)
	r.OnProcessStart = func(convID, agentID string, pid int) {
		calls++
		gotConv, gotAgent, gotPID = convID, agentID, pid
	}

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("Spawner 调用 %d 次，期望 1 次", len(fs.calls))
	}
	notify := processIDObserverFrom(fs.calls[0].ctx)
	if notify == nil {
		t.Fatal("Ask 没有把 PID 钩子挂进 spawn 的 ctx：OnProcessStart 永远不会被调用")
	}
	notify(4242)
	if calls != 1 || gotConv != convID || gotAgent != testAgentClaude || gotPID != 4242 {
		t.Errorf("OnProcessStart = (%q, %q, %d)，期望 (%q, %q, 4242)",
			gotConv, gotAgent, gotPID, convID, testAgentClaude)
	}

	// 父 ctx 不该被污染：钩子只属于那一次 spawn。
	if processIDObserverFrom(context.Background()) != nil {
		t.Error("PID 钩子漏进了别的 ctx")
	}
}

// TestRunnerWithoutObserverLeavesContextClean 是上一条的对照：不设回调时
// 交给 Spawner 的 ctx 里没有钩子（也就没有任何额外开销或副作用）。
func TestRunnerWithoutObserverLeavesContextClean(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "没有钩子")

	fs := &fakeSpawner{}
	fs.enqueueSuccess(minimalClaudeSuccess("SESSION-NOPID", "resp-nopid"))
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("Spawner 调用 %d 次，期望 1 次", len(fs.calls))
	}
	if processIDObserverFrom(fs.calls[0].ctx) != nil {
		t.Error("没设 OnProcessStart 时 ctx 里不该有 PID 钩子")
	}
}
