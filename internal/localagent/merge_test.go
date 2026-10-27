package localagent

// merge_test.go 钉住 Plan §4.3 ⑨ 的合并规则。每一条都是「怎样才会认错人」
// 的反例：宁可让一条轮留在 managed 段，也不能把别人的进程算成自己的。

import (
	"testing"
	"time"
)

// tstProcess 造一条原始进程事实（垫片判定要看 Processes）。
func tstProcess(pid, ppid int, exe string) Process {
	return Process{PID: pid, PPID: ppid, ExeName: exe}
}

// tstAgent 造一条已识别的 CLI 进程。
func tstAgent(pid, ppid int, kind Kind, started time.Time) Agent {
	return Agent{
		Kind:      kind,
		PID:       pid,
		PPID:      ppid,
		ExeName:   string(kind) + ".exe",
		StartedAt: started,
		Source:    SourceObserved,
		Certainty: CertaintyProbable,
	}
}

var tstMergeStart = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

// TestMergeManagedBySamePID 是最常见的一支：钩子拿到的就是 CLI 进程本身
// （grok / agy / codex 这类原生 exe），PID 直配，证据是 "pid"。
func TestMergeManagedBySamePID(t *testing.T) {
	obs := Observation{
		Processes: []Process{tstProcess(1000, 1, "grok.exe")},
		Agents:    []Agent{tstAgent(1000, 1, KindGrok, tstMergeStart)},
	}
	ref := ManagedRef{TurnID: "t1", ConvID: "c1", AgentID: "a1", Provider: "grok", PID: 1000, StartedAt: tstMergeStart}

	got := MergeManaged([]ManagedRef{ref}, obs)
	if len(got.Unmatched) != 0 {
		t.Fatalf("不该有未匹配的轮: %+v", got.Unmatched)
	}
	if len(got.Agents) != 1 {
		t.Fatalf("agents = %d 条，期望 1 条", len(got.Agents))
	}
	agent := got.Agents[0]
	if agent.Source != SourceManaged || agent.TurnID != "t1" || agent.ConversationID != "c1" || agent.AgentID != "a1" {
		t.Errorf("合并结果 = %+v，期望升级成 managed 并带上身份", agent)
	}
	if agent.MergeEvidence != "pid" {
		t.Errorf("MergeEvidence = %q，期望 pid", agent.MergeEvidence)
	}
	// 规则表原来的判定不该被抹掉：Kind / Certainty 还是进程事实。
	if agent.Kind != KindGrok || agent.Certainty != CertaintyProbable {
		t.Errorf("合并抹掉了进程事实: %+v", agent)
	}
}

// TestMergeManagedThroughCmdShim 是本机实测的那条路径：钩子拿到 cmd.exe 的
// PID，真正干活的是它唯一的 claude.exe 子进程，证据记 shim=cmd.exe。
func TestMergeManagedThroughCmdShim(t *testing.T) {
	obs := Observation{
		Processes: []Process{
			tstProcess(2000, 1, "cmd.exe"),
			tstProcess(2001, 2000, "claude.exe"),
		},
		Agents: []Agent{tstAgent(2001, 2000, KindClaude, tstMergeStart)},
	}
	ref := ManagedRef{TurnID: "t2", ConvID: "c2", AgentID: "a2", Provider: "claude", PID: 2000, StartedAt: tstMergeStart}

	got := MergeManaged([]ManagedRef{ref}, obs)
	if len(got.Unmatched) != 0 {
		t.Fatalf("垫片应当对上唯一子进程: %+v", got.Unmatched)
	}
	agent := got.Agents[0]
	if agent.Source != SourceManaged || agent.TurnID != "t2" || agent.PID != 2001 {
		t.Errorf("合并结果 = %+v，期望把 2001 认成 t2", agent)
	}
	if agent.MergeEvidence != "shim=cmd.exe" {
		t.Errorf("MergeEvidence = %q，期望 shim=cmd.exe", agent.MergeEvidence)
	}
}

// TestMergeManagedRejectsAmbiguousShim 是上一条的反例：垫片下有两个 agent
// 子进程时分不清，不合并（保留 managed 的 turn_id 就好）。
func TestMergeManagedRejectsAmbiguousShim(t *testing.T) {
	obs := Observation{
		Processes: []Process{
			tstProcess(3000, 1, "cmd.exe"),
			tstProcess(3001, 3000, "claude.exe"),
			tstProcess(3002, 3000, "codex.exe"),
		},
		Agents: []Agent{
			tstAgent(3001, 3000, KindClaude, tstMergeStart),
			tstAgent(3002, 3000, KindCodex, tstMergeStart),
		},
	}
	ref := ManagedRef{TurnID: "t3", ConvID: "c3", AgentID: "a3", Provider: "claude", PID: 3000, StartedAt: tstMergeStart}

	got := MergeManaged([]ManagedRef{ref}, obs)
	if len(got.Unmatched) != 1 {
		t.Fatalf("两个子进程时分不清，应当不合并: %+v", got)
	}
	for _, agent := range got.Agents {
		if agent.Source != SourceObserved {
			t.Errorf("不该把 %+v 升级成 managed", agent)
		}
	}
}

// TestMergeManagedRejectsPIDReuse 钉住 PID 复用防御：PID 撞上了但进程创建
// 时间差了半小时，那是别的进程，不合并。
func TestMergeManagedRejectsPIDReuse(t *testing.T) {
	obs := Observation{
		Processes: []Process{tstProcess(4000, 1, "claude.exe")},
		Agents:    []Agent{tstAgent(4000, 1, KindClaude, tstMergeStart.Add(30*time.Minute))},
	}
	ref := ManagedRef{TurnID: "t4", ConvID: "c4", AgentID: "a4", Provider: "claude", PID: 4000, StartedAt: tstMergeStart}

	got := MergeManaged([]ManagedRef{ref}, obs)
	if len(got.Unmatched) != 1 {
		t.Fatalf("PID 复用不该合并: %+v", got)
	}
	if got.Agents[0].Source != SourceObserved {
		t.Errorf("PID 复用时被升级成了 managed: %+v", got.Agents[0])
	}
}

// TestMergeManagedKeepsUnknownTime 是上一条的对照：拿不到进程创建时间时
// 「不知道」不是否决条件（受保护进程很常见）。
func TestMergeManagedKeepsUnknownTime(t *testing.T) {
	obs := Observation{
		Processes: []Process{tstProcess(5000, 1, "agy.exe")},
		Agents:    []Agent{tstAgent(5000, 1, KindAgy, time.Time{})},
	}
	ref := ManagedRef{TurnID: "t5", ConvID: "c5", AgentID: "a5", Provider: "agy", PID: 5000, StartedAt: tstMergeStart}

	got := MergeManaged([]ManagedRef{ref}, obs)
	if len(got.Unmatched) != 0 || got.Agents[0].Source != SourceManaged {
		t.Errorf("拿不到启动时间时仍应合并: %+v", got)
	}
}

// TestMergeManagedLeavesUnrelatedAlone 覆盖三种「对不上」：没 PID、PID 不存在、
// 同一 PID 被两条轮抢。三种都必须留在 managed 段，不碰进程表。
func TestMergeManagedLeavesUnrelatedAlone(t *testing.T) {
	obs := Observation{
		Processes: []Process{tstProcess(6000, 1, "codex.exe")},
		Agents:    []Agent{tstAgent(6000, 1, KindCodex, tstMergeStart)},
	}
	refs := []ManagedRef{
		{TurnID: "no-pid", ConvID: "c6", AgentID: "a6", Provider: "codex", PID: 0, StartedAt: tstMergeStart},
		{TurnID: "gone", ConvID: "c7", AgentID: "a7", Provider: "codex", PID: 6999, StartedAt: tstMergeStart},
		{TurnID: "first", ConvID: "c8", AgentID: "a8", Provider: "codex", PID: 6000, StartedAt: tstMergeStart},
		{TurnID: "second", ConvID: "c9", AgentID: "a9", Provider: "codex", PID: 6000, StartedAt: tstMergeStart},
	}

	got := MergeManaged(refs, obs)
	if len(got.Unmatched) != 3 {
		t.Fatalf("应当有 3 条留在 managed 段，实际 %+v", got.Unmatched)
	}
	matched := map[string]bool{}
	for _, ref := range got.Unmatched {
		matched[ref.TurnID] = true
	}
	for _, want := range []string{"no-pid", "gone", "second"} {
		if !matched[want] {
			t.Errorf("%q 应当留在 managed 段（一条进程只能属于一条轮）", want)
		}
	}
	if got.Agents[0].TurnID != "first" {
		t.Errorf("抢到进程的应当是 first，实际 %q", got.Agents[0].TurnID)
	}
	// 合并只改被对上的那一条：进程表长度与顺序都不变。
	if len(got.Agents) != len(obs.Agents) {
		t.Errorf("合并改变了进程表长度: %d → %d", len(obs.Agents), len(got.Agents))
	}
}

// TestMergeManagedDoesNotMutateInput 钉住纯函数语义：输入切片不被改写
// （调用方可能同时留着自己那份快照）。
func TestMergeManagedDoesNotMutateInput(t *testing.T) {
	original := tstAgent(7000, 1, KindClaude, tstMergeStart)
	obs := Observation{
		Processes: []Process{tstProcess(7000, 1, "claude.exe")},
		Agents:    []Agent{original},
	}
	ref := ManagedRef{TurnID: "t7", ConvID: "c7", AgentID: "a7", Provider: "claude", PID: 7000, StartedAt: tstMergeStart}
	_ = MergeManaged([]ManagedRef{ref}, obs)
	if obs.Agents[0].Source != SourceObserved || obs.Agents[0].TurnID != "" {
		t.Errorf("MergeManaged 改写了入参: %+v", obs.Agents[0])
	}
}

// TestObserveRespectsLevel 钉住 Observe 与 Scan 同一条 fail-closed：
// off 时一次枚举都不做，返回空的两段；nil 枚举器返回 ErrUnsupported。
func TestObserveRespectsLevel(t *testing.T) {
	exploding := func() ([]Process, error) {
		t.Fatal("off 时不许枚举进程（S-01）")
		return nil, nil
	}
	obs, err := Observe(LevelOff, exploding)
	if err != nil || len(obs.Processes) != 0 || len(obs.Agents) != 0 {
		t.Errorf("Observe(off) = %+v, %v；期望空两段与 nil 错误", obs, err)
	}

	if _, err := Observe(LevelObserve, nil); err != ErrUnsupported {
		t.Errorf("Observe(observe, nil) 的错误 = %v，期望 ErrUnsupported", err)
	}
}
