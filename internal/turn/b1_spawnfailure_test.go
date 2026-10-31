package turn

// B1：spawn 失败里「进程根本没起来」的那一档不能把成本基线标成可疑。
// 那一轮一分钱都没花；标成可疑会让下一轮真实金额被 computeCostDelta 丢掉。

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"council/internal/provider"
)

func TestB1SpawnFailureBeforeStartDoesNotSuspectBaseline(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "task-b1")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	fs.enqueue(spawnResult{err: fmt.Errorf("%w: binary not found", ErrProcessNotStarted)})
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮应当成功: %v", err)
	}
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	if st := r.gate(key).st; st == nil {
		t.Fatal("前提不成立：第一轮成功后没有会话状态")
	}
	// 第二轮要有新内容可送，否则会先在「没有新东西」那一支被拒，测不到 spawn。
	postUser(t, svc, convID, "task-b1-2")

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err == nil {
		t.Fatal("第二轮应当因 spawn 失败而报错")
	}
	if rec.Outcome != OutcomeSpawnFailed {
		t.Errorf("outcome = %v（err=%v），期望 OutcomeSpawnFailed", rec.Outcome, rec.Err)
	}
	if st := r.gate(key).st; st.baselineSuspect {
		t.Error("进程从未 Start 的失败把 baselineSuspect 置成了 true；下一轮真实金额会被丢掉（B1）")
	}
}

func TestB1SpawnProcessMarksStartFailureAsNotStarted(t *testing.T) {
	dir := t.TempDir()
	_, _, _, _, err := spawnProcess(context.Background(), provider.Command{
		Path: filepath.Join(dir, "definitely-missing-provider"),
		Dir:  dir,
	})
	if err == nil {
		t.Fatal("不存在的可执行文件应当让 spawnProcess 报错")
	}
	if !errors.Is(err, ErrProcessNotStarted) {
		t.Fatalf("Start 失败应包成 ErrProcessNotStarted，实际: %v", err)
	}
}
