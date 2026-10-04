package tui

// 本文件覆盖冻结契约 RB-A v1.3 §2.4 的 T1-T8 与 T4b（T4b 是落地后补的）：纯 Model 单测，不起 Program。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3143bc8 的源码。
//
// 公共装置（§2.4）：Model 用假 Submitter / 假 Drainer / 假 Println、时钟恒为 rbaT0，先喂一条
// WindowSizeMsg{100,24}。「提交」= 敲一个可见字符 + Enter（丢掉 Cmd，不跑）。
// 「消费」= 每次提交之后再喂一条 WindowSizeMsg{100,24}，把 E5 的待装 tick 吃掉（丢掉它的 Cmd）。
// 表头一律取 m.chrome()[0] 整串比较。

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
	"council/internal/turn"
)

// rbaT0 是一个固定时刻，公共装置的假钟恒返回它。
var rbaT0 = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

// rbaRID 与 rbaRID2 是 §3.3 的两个 roundID。
const (
	rbaRID  = "467afaff-4b9d-4694-8a4e-66f322688558"
	rbaRID2 = "d4515aea-24a4-4b12-b95e-cdd3a3aa08ae"
)

// rbaNewModel 造公共装置的 Model 并喂一条 WindowSizeMsg{100,24}。now 为 nil 时时钟恒为 rbaT0。
func rbaNewModel(t *testing.T, now func() time.Time) Model {
	t.Helper()
	if now == nil {
		now = func() time.Time { return rbaT0 }
	}
	m := New(Options{
		Submit:  &tui1bSubmitter{},
		Out:     tui1bNopDrainer{},
		Version: "t",
		Println: (&tui1bPrinter{}).cmd,
		Now:     now,
	})
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	return m
}

// rbaSubmit 敲一个可见字符再按 Enter，丢掉 Cmd 不跑。
func rbaSubmit(t *testing.T, m Model) Model {
	t.Helper()
	m = tui1bType(t, m, "x")
	m, _ = tui1bStep(t, m, tui1bEnter())
	return m
}

// rbaConsume 喂一条 WindowSizeMsg{100,24}，把待装 tick 吃掉，丢掉 Cmd。
func rbaConsume(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	return m
}

// rbaWantHeader 断言表头整串相等。
func rbaWantHeader(t *testing.T, m Model, step, want string) {
	t.Helper()
	if got := m.chrome()[0]; got != want {
		t.Errorf("%s：表头应当整串等于\n  %q\n实际\n  %q", step, want, got)
	}
}

// ═══ T1 ═══════════════════════════════════════════════════════════════

// TestRbaT1FormatElapsedTable：formatElapsed 逐条整串等于 §3.2 的表。
//
// 【判别力】每个边界值都是某条变异的唯一杀手：`< 60` 写成 `<= 60`（#24）→ 60s 印成 60s 而不是 1m00s；
// `%02d` 写成 `%d`（#25）→ 65s 印成 1m5s、3720s 印成 1h2m；小时格带秒 → 3659s 不等于 1h00m；
// 不夹负数（#26）→ -1s 印成 -1s。相邻两行（59.999s 与 60s、3599s 与 3600s）互为对照。
func TestRbaT1FormatElapsedTable(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{-1 * time.Second, "0s"},
		{0, "0s"},
		{999 * time.Millisecond, "0s"},
		{1 * time.Second, "1s"},
		{59*time.Second + 999*time.Millisecond, "59s"},
		{60 * time.Second, "1m00s"},
		{65 * time.Second, "1m05s"},
		{3599 * time.Second, "59m59s"},
		{3600 * time.Second, "1h00m"},
		{3659 * time.Second, "1h00m"},
		{3720 * time.Second, "1h02m"},
		{36000 * time.Second, "10h00m"},
		{100*time.Hour + 59*time.Minute + 59*time.Second, "100h59m"},
	}
	for _, c := range cases {
		if got := formatElapsed(c.in); got != c.want {
			t.Errorf("formatElapsed(%v) = %q，应当是 %q", c.in, got, c.want)
		}
	}
}

// ═══ T2 ═══════════════════════════════════════════════════════════════

// TestRbaT2HeaderSequence：按 §3.3 的步骤表逐步喂消息，每步之后表头整串等于表中那一行。
//
// 【判别力】能为假，逐项：状态文字被 tick 覆盖（#29）→ H11c、H12b 红；不忙时仍拼已用时间（#40）→ H1、H13a 红；
// roundID 每次覆盖（#37）→ H6 红；取前 6 位（#38）→ H5 红；`n > 1` 写成 `n >= 1`（#39）→ H9 红；
// 分隔写成一个空格 → 全部带段的行红；已用时长按拍累加（#30）→ H3 一拍只到 1s 而不是 12s，红；
// beginSubmit 不 tickGen++（#34）→ 代次 1 的拍被丢，H3 仍是 0s，红；两处都不清 Run 状态（#35）→ H15 红。
// H13b 在忠实实现上是空断言（不忙时既不显示段、段也活不到下一次提交），只作回归钉，见 weak 清单。
// 对照：H3 与 H11c 只差状态文字；H10a 与 H10b 只差 roundID 是否为空。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。H11b 是第二次，H12 的提示是新文案。
func TestRbaT2HeaderSequence(t *testing.T) {
	m := rbaNewModel(t, nil)
	rbaWantHeader(t, m, "H1", "council t   空闲")

	m = rbaConsume(t, rbaSubmit(t, m))
	rbaWantHeader(t, m, "H2", "council t   正在运行  0s")

	step := func(name string, msg tea.Msg, want string) {
		t.Helper()
		m, _ = tui1bStep(t, m, msg)
		rbaWantHeader(t, m, name, want)
	}

	step("H3", elapsedTickMsg{gen: 1, at: rbaT0.Add(12 * time.Second)}, "council t   正在运行  12s")
	step("H4", roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}},
		"council t   正在运行  发言轮 1/2  回来 0/3  12s")
	step("H5", memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rba_a", RoundID: rbaRID}},
		"council t   正在运行  发言轮 1/2 (688558)  回来 1/3  12s")
	step("H6", memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rba_b", RoundID: rbaRID2}},
		"council t   正在运行  发言轮 1/2 (688558)  回来 2/3  12s")
	step("H7", roundEndMsg{ev: repl.RoundEndEvent{Label: "发言轮", K: 1, N: 2}},
		"council t   正在运行  发言轮 1/2 (688558)  回来 2/3  12s")
	step("H8", roundStartMsg{ev: repl.RoundStartEvent{Label: "裁判轮", K: 0, N: 0, Members: 1}},
		"council t   正在运行  裁判轮  回来 0/1  12s")
	step("H9", roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 2}},
		"council t   正在运行  发言轮  回来 0/2  12s")
	step("H10a", memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "rba_a", RoundID: ""}},
		"council t   正在运行  发言轮  回来 1/2  12s")
	step("H10b", memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "rba_b", RoundID: "abc"}},
		"council t   正在运行  发言轮 (abc)  回来 2/2  12s")

	m, _ = tui1bStep(t, m, roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}})
	step("H11a", memberDoneMsg{ev: repl.MemberDoneEvent{RoundID: rbaRID}},
		"council t   正在运行  发言轮 1/2 (688558)  回来 1/3  12s")
	step("H11a2", tui1bCtrl('c'), "council t   再按一次 Ctrl-C 取消本轮  发言轮 1/2 (688558)  回来 1/3  12s")
	step("H11b", tui1bCtrl('c'), "council t   正在取消  发言轮 1/2 (688558)  回来 1/3  12s")
	step("H11c", elapsedTickMsg{gen: 1, at: rbaT0.Add(65 * time.Second)},
		"council t   正在取消  发言轮 1/2 (688558)  回来 1/3  1m05s")

	m = tui1bType(t, m, "y")
	step("H12a", tui1bEnter(),
		"council t   一轮正在跑，先等它回来或连按两次 Ctrl-C 取消  发言轮 1/2 (688558)  回来 1/3  1m05s")
	step("H12b", elapsedTickMsg{gen: 1, at: rbaT0.Add(3720 * time.Second)},
		"council t   一轮正在跑，先等它回来或连按两次 Ctrl-C 取消  发言轮 1/2 (688558)  回来 1/3  1h02m")

	step("H13a", submitDoneMsg{}, "council t   空闲")
	m, _ = tui1bStep(t, m, roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}})
	step("H13b", memberDoneMsg{ev: repl.MemberDoneEvent{RoundID: rbaRID}}, "council t   空闲")

	m = rbaConsume(t, rbaSubmit(t, m))
	step("H14a", submitDoneMsg{err: errors.New("坏了")}, "council t   发生错误，正在退出")
	step("H14b", elapsedTickMsg{gen: 2, at: rbaT0.Add(99 * time.Second)}, "council t   发生错误，正在退出")

	// H15 另起一个 Model。
	m2 := rbaNewModel(t, nil)
	m2 = rbaConsume(t, rbaSubmit(t, m2))
	m2, _ = tui1bStep(t, m2, roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}})
	m2, _ = tui1bStep(t, m2, memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rba_a", RoundID: rbaRID}})
	m2, _ = tui1bStep(t, m2, submitDoneMsg{})
	m2 = rbaConsume(t, rbaSubmit(t, m2))
	rbaWantHeader(t, m2, "H15", "council t   正在运行  0s")
}

// ═══ T3 ═══════════════════════════════════════════════════════════════

// TestRbaT3GenerationAndBusy：旧代次的拍丢掉；当前代次的拍更新时长并续订一拍（间隔 1 秒、同代次）；
// 不忙之后的拍什么都不做。
//
// 【判别力】能为假，逐句：去掉 gen 检查（#28）→ 第一句表头变成 1m05s、Cmd 非 nil，红；
// 续订那一拍代次写成 tickGen+1（#49）→ 收到的 gen 是 3，红；续订间隔写成 1 分钟（#50）→ 1.5 秒超时，红；
// 不看 busy 就续订（#27）→ 最后一句 Cmd 非 nil，红。
// 对照：前两次喂的消息只差 gen 一处。
func TestRbaT3GenerationAndBusy(t *testing.T) {
	m := rbaNewModel(t, nil)
	m = rbaSubmit(t, m) // 代次 1
	m, _ = tui1bStep(t, m, submitDoneMsg{})
	m = rbaConsume(t, rbaSubmit(t, m)) // 代次 2

	var cmd tea.Cmd
	m, cmd = tui1bStep(t, m, elapsedTickMsg{gen: 1, at: rbaT0.Add(65 * time.Second)})
	rbaWantHeader(t, m, "旧代次的拍", "council t   正在运行  0s")
	if cmd != nil {
		t.Errorf("旧代次（gen 1）的拍应当返回 nil Cmd，实际非 nil")
	}

	m, cmd = tui1bStep(t, m, elapsedTickMsg{gen: 2, at: rbaT0.Add(65 * time.Second)})
	rbaWantHeader(t, m, "当前代次的拍", "council t   正在运行  1m05s")
	if cmd == nil {
		t.Fatalf("当前代次（gen 2）且忙时应当续订下一拍，实际 Cmd 为 nil")
	}

	// 跑续订的那条 Cmd：在新 goroutine 里调，测试 goroutine 最多等 1500ms。
	// 那个 goroutine 里不调 t 的任何方法。
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case got := <-ch:
		tick, ok := got.(elapsedTickMsg)
		if !ok {
			t.Fatalf("续订的 Cmd 跑出来应当是 elapsedTickMsg，实际 %T", got)
		}
		if tick.gen != 2 {
			t.Errorf("续订那一拍的代次应当是 2，实际 %d", tick.gen)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatalf("续订的 Cmd 在 1.5 秒内没有产出消息：续订间隔不是 1 秒")
	}

	m, _ = tui1bStep(t, m, submitDoneMsg{})
	m, cmd = tui1bStep(t, m, elapsedTickMsg{gen: 2, at: rbaT0.Add(70 * time.Second)})
	rbaWantHeader(t, m, "不忙之后的拍", "council t   空闲")
	if cmd != nil {
		t.Errorf("不忙时的拍应当返回 nil Cmd（只在 busy 时续订），实际非 nil")
	}
}

// ═══ T4 ═══════════════════════════════════════════════════════════════

// TestRbaT4TickInstalledOnceOnNextUpdate：提交后的下一条 Update 才装 tick，只装一次。
//
// 【判别力】从不装（#32）→ Cmd1 为 nil，红；每条 Update 都装（#33）→ Cmd2 非 nil，红。
// 两条消息完全相同，只差先后（对照）。在 Enter 那一次就装由既有 Acc6 抓（#31 本条不一定杀，见契约 §4）。
func TestRbaT4TickInstalledOnceOnNextUpdate(t *testing.T) {
	m := rbaNewModel(t, nil)
	m = rbaSubmit(t, m)

	m, cmd1 := tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	if cmd1 == nil {
		t.Fatalf("提交后的第一条 Update 应当带回 tick，实际 Cmd 为 nil")
	}
	tr := tui1bRun(t, cmd1)
	n := 0
	for _, msg := range tr.msgs {
		if tick, ok := msg.(elapsedTickMsg); ok {
			n++
			if tick.gen != 1 {
				t.Errorf("装上的 tick 代次应当是 1，实际 %d", tick.gen)
			}
		}
	}
	if n != 1 {
		t.Errorf("Cmd1 展开后应当恰好有 1 条 elapsedTickMsg，实际 %d；叶子 %v", n, tr.log)
	}

	_, cmd2 := tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	if cmd2 != nil {
		t.Errorf("提交后的第二条 Update 不应再装 tick，实际 Cmd 非 nil")
	}
}

// ═══ T4b ══════════════════════════════════════════════════════════════

// TestRbaT4bTickKeepsOriginalCmd：提交后的第一条 Update 装 tick 时，这条消息自己的 Cmd 不能丢。
// 被测消息是敲一个可见字符 'z'：textarea 光标移动后捎回一条闪烁 Cmd（跑出来是 cursor.BlinkMsg）。
//
// 前提：另起一个 Model，消费待装之后喂同一条消息，返回的 Cmd 必须非 nil；否则这条消息本来就不产 Cmd，
// 测不到丢弃，按前提不成立 Fatalf。另起而不是从被测 Model 复制：光标的闪烁上下文是指针，
// 复制出来的两份共用它，后调的 Blink 会取消先前那条。
//
// 【判别力】能为假：E5 装 tick 时丢掉原 Cmd、只返回 tick（#51）→ 展开后只剩 1 条 elapsedTickMsg，
// 其余叶子为 0，红。T4 用的 WindowSizeMsg 本来就返回 nil Cmd，丢不丢都一样，抓不到这一条。
// 从不装（#32）→ elapsedTickMsg 为 0 条，也红（T4 已抓，这里顺带）。
// 对照：前提那一步，同一条消息，只差待装是否已被消费。
func TestRbaT4bTickKeepsOriginalCmd(t *testing.T) {
	pre := rbaConsume(t, rbaSubmit(t, rbaNewModel(t, nil)))
	if _, cmd := tui1bStep(t, pre, tui1bKey('z')); cmd == nil {
		t.Fatalf("前提不成立：消费待装之后敲 'z' 返回的 Cmd 为 nil，这条消息本来就不产 Cmd，测不到丢弃")
	}

	m := rbaSubmit(t, rbaNewModel(t, nil))
	_, cmd1 := tui1bStep(t, m, tui1bKey('z'))
	if cmd1 == nil {
		t.Fatalf("提交后的第一条 Update 应当带回 tick 与原 Cmd，实际 Cmd 为 nil")
	}
	tr := tui1bRun(t, cmd1)
	ticks, others := 0, 0
	for _, msg := range tr.msgs {
		if tick, ok := msg.(elapsedTickMsg); ok {
			ticks++
			if tick.gen != 1 {
				t.Errorf("装上的 tick 代次应当是 1，实际 %d", tick.gen)
			}
			continue
		}
		others++
	}
	if ticks != 1 {
		t.Errorf("Cmd1 展开后应当恰好有 1 条 elapsedTickMsg，实际 %d；叶子 %v", ticks, tr.log)
	}
	if others < 1 {
		t.Errorf("Cmd1 展开后应当至少还有 1 片不是 elapsedTickMsg 的叶子（原 Cmd 的产出），实际 %d；叶子 %v", others, tr.log)
	}
}

// ═══ T5 ═══════════════════════════════════════════════════════════════

// TestRbaT5FrameHeightUnchanged：忙、有段、有 roundID 时帧高仍是 8，窄窗口下也不折行。
//
// 【判别力】段信息用换行拼进表头（#41）→ View 行数变 9，红。frameHeight 那一句在这种写法下
// 仍是 8（chrome 还是两个元素），只在把段做成 chrome 的第三个元素时才变，属回归钉。
// 对照：X-39e 的 A9 在空闲状态下钉同一个 8（它不拼段，杀不到 #41）。
func TestRbaT5FrameHeightUnchanged(t *testing.T) {
	m := rbaNewModel(t, nil)
	m = rbaConsume(t, rbaSubmit(t, m))
	m, _ = tui1bStep(t, m, elapsedTickMsg{gen: 1, at: rbaT0.Add(12 * time.Second)})
	m, _ = tui1bStep(t, m, roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}})
	m, _ = tui1bStep(t, m, memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rba_a", RoundID: rbaRID}})
	// 前提：确实在 H5 状态。
	if got := m.chrome()[0]; got != "council t   正在运行  发言轮 1/2 (688558)  回来 1/3  12s" {
		t.Fatalf("前提不成立：没有到达 H5 状态，表头是 %q", got)
	}

	check := func(where string, m Model) {
		t.Helper()
		if h := m.frameHeight(); h != 8 {
			t.Errorf("%s：frameHeight() 应当是 8，实际 %d", where, h)
		}
		if n := strings.Count(m.View().Content, "\n") + 1; n != 8 {
			t.Errorf("%s：View 的行数应当是 8，实际 %d", where, n)
		}
	}
	check("宽 100", m)
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 20, Height: 24})
	check("宽 20", m)
}

// ═══ T6 ═══════════════════════════════════════════════════════════════

// TestRbaT6AdapterMapsVerbatim：适配器三个方法各发恰好 1 条消息，与 {ev: ev} 逐字相等。
//
// 【判别力】方法之间串线（#22）、丢字段（#23）、发指针 → DeepEqual 红。每个字段都用非零值，
// 丢任何一个字段都看得见。
func TestRbaT6AdapterMapsVerbatim(t *testing.T) {
	var got []tea.Msg
	o := newRoundObserver(func(msg tea.Msg) { got = append(got, msg) })

	rs := repl.RoundStartEvent{Label: "发言轮", K: 2, N: 3, Members: 4}
	md := repl.MemberDoneEvent{
		Label: "裁判轮", AgentID: "rba_x", Outcome: turn.OutcomePostRejected,
		MaybeAdvanced: true, Drifted: true, Dropped: true, WallMS: 1234, RoundID: "rid-xyz",
	}
	re := repl.RoundEndEvent{Label: "发言轮", K: 2, N: 3}

	cases := []struct {
		name string
		call func()
		want tea.Msg
	}{
		{"RoundStart", func() { o.RoundStart(rs) }, roundStartMsg{ev: rs}},
		{"MemberDone", func() { o.MemberDone(md) }, memberDoneMsg{ev: md}},
		{"RoundEnd", func() { o.RoundEnd(re) }, roundEndMsg{ev: re}},
	}
	for _, c := range cases {
		got = nil
		c.call()
		if len(got) != 1 {
			t.Errorf("%s：send 应当恰好收到 1 条消息，实际 %d 条: %#v", c.name, len(got), got)
			continue
		}
		if !reflect.DeepEqual(got[0], c.want) {
			t.Errorf("%s：send 收到 %#v，应当是 %#v", c.name, got[0], c.want)
		}
	}
}

// ═══ T7 ═══════════════════════════════════════════════════════════════

// TestRbaT7NewMessagesReturnNilCmd：消费待装之后，三种新消息返回的 Cmd 都是 nil。
//
// 【判别力】分支里返回 tea.Println（#42）→ Cmd 非 nil，红。
// 在忠实实现上这三条是结构上的「分支返回 nil」，判别力只针对 #42 这一类。
func TestRbaT7NewMessagesReturnNilCmd(t *testing.T) {
	m := rbaNewModel(t, nil)
	m = rbaConsume(t, rbaSubmit(t, m))
	msgs := []struct {
		name string
		msg  tea.Msg
	}{
		{"roundStartMsg", roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 2}}},
		{"memberDoneMsg", memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rba_a", RoundID: rbaRID}}},
		{"roundEndMsg", roundEndMsg{ev: repl.RoundEndEvent{Label: "发言轮", K: 1, N: 2}}},
	}
	for _, c := range msgs {
		var cmd tea.Cmd
		m, cmd = tui1bStep(t, m, c.msg)
		if cmd != nil {
			t.Errorf("%s 应当返回 nil Cmd（不许由 Update 返回 tea.Println），实际非 nil", c.name)
		}
	}
}

// ═══ T8 ═══════════════════════════════════════════════════════════════

// TestRbaT8RestartClockEachSubmit：每次提交都重取起点、已用时长清零。
//
// 【判别力】beginSubmit 不清零已用时长（#44）→ 第 2 步显示 45s，红；
// 起点只在第一次提交时取（#45）→ 第 3 步显示 1m45s，红。
// T 系列别的用例时钟恒为 rbaT0，抓不到这两条；第 2、3 步只差已用时长的来源（对照）。
func TestRbaT8RestartClockEachSubmit(t *testing.T) {
	now := rbaT0
	m := rbaNewModel(t, func() time.Time { return now })

	m = rbaConsume(t, rbaSubmit(t, m)) // 代次 1
	m, _ = tui1bStep(t, m, elapsedTickMsg{gen: 1, at: rbaT0.Add(45 * time.Second)})
	rbaWantHeader(t, m, "第 1 步", "council t   正在运行  45s")

	m, _ = tui1bStep(t, m, submitDoneMsg{})
	now = rbaT0.Add(100 * time.Second)
	m = rbaConsume(t, rbaSubmit(t, m)) // 代次 2
	rbaWantHeader(t, m, "第 2 步", "council t   正在运行  0s")

	m, _ = tui1bStep(t, m, elapsedTickMsg{gen: 2, at: rbaT0.Add(105 * time.Second)})
	rbaWantHeader(t, m, "第 3 步", "council t   正在运行  5s")
}
