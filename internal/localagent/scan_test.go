package localagent

// scan_test.go 钉住本模块最重要的一条硬约束：local_agent 关着的时候，
// 一次进程枚举都不许发生（S-01）。证明方式是注入一个会 panic 的
// Enumerator —— off 路径如果调用它，测试直接崩，而不是悄悄放过。

import (
	"errors"
	"testing"
)

// TestScanOffNeverCallsEnumerator 是 off 路径的判别性用例。
// 枚举函数一被调用就 count 加一并 panic；断言 count 仍为 0，
// 说明 Scan 在参数检查那一步就返回了。
func TestScanOffNeverCallsEnumerator(t *testing.T) {
	calls := 0
	exploding := func() ([]Process, error) {
		calls++
		panic("local_agent off 时不许枚举进程（S-01）")
	}

	levels := []Level{
		LevelOff,
		"",
		"register",
		"coordinate",
		"collaborate",
		LevelOf(""),
		LevelOf("off"),
		LevelOf("on"),
	}
	for _, level := range levels {
		agents, err := Scan(level, exploding)
		if err != nil {
			t.Errorf("Scan(%q) 返回错误: %v", level, err)
		}
		if len(agents) != 0 {
			t.Errorf("Scan(%q) 返回了 %d 个 agent，期望 0 个", level, len(agents))
		}
	}
	if calls != 0 {
		t.Fatalf("off 路径调用了 %d 次枚举函数，期望 0 次", calls)
	}
}

// TestScanObserveCallsEnumerator 是正对照：observe 时必须调用一次，
// 并把规则表的结果原样交出来。没有这一条，上一条在「Scan 永远不调用
// 枚举」的坏实现下也会绿。
func TestScanObserveCallsEnumerator(t *testing.T) {
	calls := 0
	enum := func() ([]Process, error) {
		calls++
		return []Process{
			{PID: 200, PPID: 100, ExeName: "node.exe"},
			{PID: 300, PPID: 200, ExeName: "claude.exe"},
			{PID: 301, PPID: 1, ExeName: "python.exe"},
		}, nil
	}

	agents, err := Scan(LevelObserve, enum)
	if err != nil {
		t.Fatalf("Scan(observe) 返回错误: %v", err)
	}
	if calls != 1 {
		t.Errorf("observe 调用了 %d 次枚举函数，期望 1 次", calls)
	}
	if len(agents) != 1 {
		t.Fatalf("observe 识别出 %d 个 agent，期望 1 个：%+v", len(agents), agents)
	}
	got := agents[0]
	if got.Kind != KindClaude || got.PID != 300 || got.PPID != 200 || got.Source != SourceObserved {
		t.Errorf("识别结果 = %+v，期望 claude/pid=300/ppid=200/observed", got)
	}
}

// TestScanNilEnumerator fails closed：
//   - off + nil：不枚举，返回空表、无错误；
//   - observe + nil：返回 ErrUnsupported，不 panic。
func TestScanNilEnumerator(t *testing.T) {
	agents, err := Scan(LevelOff, nil)
	if err != nil || len(agents) != 0 {
		t.Errorf("Scan(off, nil) = (%v, %v)，期望空表与 nil 错误", agents, err)
	}

	agents, err = Scan(LevelObserve, nil)
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("Scan(observe, nil) 的错误 = %v，期望 ErrUnsupported", err)
	}
	if len(agents) != 0 {
		t.Errorf("Scan(observe, nil) 返回了 %d 个 agent，期望 0 个", len(agents))
	}
}

// TestScanPropagatesEnumeratorError 钉住错误不被吞成空表：调用方要靠
// 这个错误把「枚举失败」与「本机没有 agent」分开显示。
func TestScanPropagatesEnumeratorError(t *testing.T) {
	boom := errors.New("boom")
	agents, err := Scan(LevelObserve, func() ([]Process, error) {
		return nil, boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("Scan 的错误 = %v，期望 %v", err, boom)
	}
	if len(agents) != 0 {
		t.Errorf("出错时返回了 %d 个 agent，期望 0 个", len(agents))
	}
}

// TestLevelOf 钉住「只认 observe，其余一律 off」：register / coordinate /
// collaborate 在 L1 没有实现，不能被当成 observe 打开采集面。
func TestLevelOf(t *testing.T) {
	tests := []struct {
		raw  string
		want Level
	}{
		{raw: "", want: LevelOff},
		{raw: "off", want: LevelOff},
		{raw: "on", want: LevelOff},
		{raw: "register", want: LevelOff},
		{raw: "coordinate", want: LevelOff},
		{raw: "collaborate", want: LevelOff},
		{raw: "observe", want: LevelObserve},
		{raw: " OBSERVE ", want: LevelObserve},
	}
	for _, tc := range tests {
		if got := LevelOf(tc.raw); got != tc.want {
			t.Errorf("LevelOf(%q) = %q，期望 %q", tc.raw, got, tc.want)
		}
	}
}
