package tui

// 本文件覆盖冻结契约 RB-B v1.1 §2.3 的 B1、B2：纯 Model 单测，不起 Program。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD a728cee 的源码。
//
// 公共装置（§2.3）：假 Submitter / 假 Println（tui1bPrinter）/ 假 Drainer，时钟读 rig 里的 now，
// 先喂一条装置尺寸的 WindowSizeMsg。「提交」= 敲 x 再 Enter，丢掉 Cmd 不跑；「消费」= 再喂一条同尺寸的
// WindowSizeMsg，丢掉 Cmd（把 RB-A E5 的待装 tick 吃掉）。「看板行」= chrome()[2:]；帧高每次都同时断言
// 等于 View 的行数（看板行必须真的在 View 里、且每行单行）。「叶子」= tui1bRun 的 log，并断言没见过 Batch。
// 期望值是 §3.6 两张表的逐字节常量，逐行整行比较。
//
// 本文件引用本刀新增的 Speakers / Results 字段与 boardFinalizeMsg，在 HEAD 上整包编译不过，
// 这是契约 §2 写明的预期红。

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"council/internal/repl"
	"council/internal/turn"
)

// rbbModelRig 是 §2.3 的公共装置。now 由用例改，Model 的时钟每次调用时读它。
type rbbModelRig struct {
	t    *testing.T
	m    Model
	now  time.Time
	sub  *tui1bSubmitter
	pr   *tui1bPrinter
	w, h int
}

func rbbNewModelRig(t *testing.T, w, h int) *rbbModelRig {
	t.Helper()
	r := &rbbModelRig{t: t, now: rbaT0, sub: &tui1bSubmitter{}, pr: &tui1bPrinter{}, w: w, h: h}
	r.m = New(Options{
		Submit:  r.sub,
		Out:     tui1bNopDrainer{},
		Version: "t",
		Println: r.pr.cmd,
		Now:     func() time.Time { return r.now },
	})
	r.m, _ = tui1bStep(t, r.m, tea.WindowSizeMsg{Width: w, Height: h})
	return r
}

// submitConsume 做一次「提交」再「消费」。
func (r *rbbModelRig) submitConsume() {
	r.t.Helper()
	r.m = tui1bType(r.t, r.m, "x")
	r.m, _ = tui1bStep(r.t, r.m, tui1bEnter())
	r.m, _ = tui1bStep(r.t, r.m, tea.WindowSizeMsg{Width: r.w, Height: r.h})
}

// step 喂一条消息，Model 存回 rig，返回 Cmd。
func (r *rbbModelRig) step(msg tea.Msg) tea.Cmd {
	r.t.Helper()
	var cmd tea.Cmd
	r.m, cmd = tui1bStep(r.t, r.m, msg)
	return cmd
}

// ─── 断言小工具 ────────────────────────────────────────────────────────

func rbbBoardOf(m Model) []string {
	return append([]string(nil), m.chrome()[2:]...)
}

func rbbWantBoard(t *testing.T, where string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s：看板行应当恰好 %d 行 %q，实际 %d 行 %q", where, len(want), want, len(got), got)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s：看板第 %d 行应当整行等于\n  %q\n实际\n  %q", where, i+1, want[i], got[i])
		}
	}
}

// rbbWantFrame 断言帧高，并断言帧高等于 View 的行数。
func rbbWantFrame(t *testing.T, where string, m Model, want int) {
	t.Helper()
	fh := m.frameHeight()
	if fh != want {
		t.Errorf("%s：frameHeight() 应当是 %d，实际 %d", where, want, fh)
	}
	if n := strings.Count(m.View().Content, "\n") + 1; n != fh {
		t.Errorf("%s：frameHeight() = %d 与 View 的行数 %d 不等（看板行没进 View，或某行折成了多行）", where, fh, n)
	}
}

func rbbWantHead(t *testing.T, where string, m Model, want string) {
	t.Helper()
	if got := m.chrome()[0]; got != want {
		t.Errorf("%s：表头应当整串等于\n  %q\n实际\n  %q", where, want, got)
	}
}

func rbbWantCursorNil(t *testing.T, where string, m Model) {
	t.Helper()
	if c := m.View().Cursor; c != nil {
		t.Errorf("%s：光标应当是 nil（不钉），实际 %+v", where, *c)
	}
}

func rbbWantCursor00(t *testing.T, where string, m Model) {
	t.Helper()
	c := m.View().Cursor
	if c == nil {
		t.Errorf("%s：光标应当钉在 (0,0)，实际 nil", where)
		return
	}
	if c.X != 0 || c.Y != 0 {
		t.Errorf("%s：光标应当钉在 (0,0)，实际 (%d,%d)", where, c.X, c.Y)
	}
}

func rbbWantNilCmd(t *testing.T, where string, cmd tea.Cmd) {
	t.Helper()
	if cmd != nil {
		tr := tui1bRun(t, cmd)
		t.Errorf("%s：Cmd 应当是 nil，实际非 nil，展开后叶子 %q", where, tr.log)
	}
}

// rbbWantLeaves 把 Cmd 展开到底，断言叶子恰好是 want、且途中没见过 tea.BatchMsg。
func rbbWantLeaves(t *testing.T, where string, cmd tea.Cmd, want ...string) {
	t.Helper()
	if cmd == nil {
		t.Errorf("%s：Cmd 应当非 nil，叶子应当恰好是 %q，实际 Cmd 为 nil", where, want)
		return
	}
	tr := tui1bRun(t, cmd)
	if tr.sawBatch {
		t.Errorf("%s：展开途中见到了 tea.BatchMsg（必须 tea.Sequence，不许 tea.Batch）；叶子 %q", where, tr.log)
	}
	if len(tr.log) != len(want) {
		t.Errorf("%s：叶子应当恰好是 %q，实际 %q", where, want, tr.log)
		return
	}
	for i := range want {
		if tr.log[i] != want[i] {
			t.Errorf("%s：叶子应当恰好是 %q，实际 %q", where, want, tr.log)
			return
		}
	}
}

func rbbTick(s int) elapsedTickMsg {
	return elapsedTickMsg{gen: 1, at: rbaT0.Add(time.Duration(s) * time.Second)}
}

const rbbFinLeaf = "other:tui.boardFinalizeMsg"

// ═══ B1 ═══════════════════════════════════════════════════════════════

// TestRbbB1MainSequence：§3.6 B1 主步骤表，逐步喂消息，每步断言表中列出的全部项。
//
// 【判别力】逐步（变异编号见契约 §4）：
//   - B1-1：看板行数取 Members（#27 与本步同值，杀不到，见 B2-6）；看板只进 View 不进 chrome（#62）→ 帧高 ≠ View 行数，红。
//   - B1-2：在跑行从提交起点算（#37）→ 12s ≠ 7s，红。
//   - B1-3：窗口外的 finalize 也清（#61）→ mx 的帧高 8，红。
//   - B1-4：按回来次序改第 k 行（#38）→ 第 2 行被改，红；原地改切片（#39）→ saved 的看板变了，红。
//   - B1-6：roundEndMsg 分支返回 tea.Println（#15）→ RE 的 Cmd 非 nil，红（RE 带一项 Results）。
//   - B1-7：新段追加到旧行后面（#40）→ 行数与内容红；每段重设 floor（#29 同类）在这里不缩，杀不到，见 B2-4；
//     新段复用旧行的底层数组（#67，`m.rows[:0]` 再 append）→ saved2 的第 1 行变成 rbb_j，红（2026-09-25 补，#41 (i)）。
//   - B1-9：收尾行时间用最近一拍（#42）→ 1m05s ≠ 2m05s；计数只算当前段（#43）→ 1/1 ≠ 4/4；Batch（#44）→ sawBatch
//     且倒序；直接 tea.Println（#45）→ 叶子是 other:tea.printLineMessage；不钉光标（#46、#66）或钉在 (0,8)（#47）→ 光标红；
//     不把 status 改成空闲（#53）→ 表头红；不打收尾行（#50）或收尾行为空串（#51）→ 叶子红。
//   - B1-10：窗口里不挡 Enter（#57）→ Cmd 非 nil，红。B1-11：不挡 Ctrl-C（#56）→ Cmd 是到期 tick、不含 quit，表头变成武装提示，红。
//   - B1-12：D4 不清 floor（#58）→ 帧高 11；不清 pinCursor（#59）→ 光标 (0,0)，红。
//   - B1-14：D4 不清 finalizing（#60）→ Enter 被挡，Cmd 为 nil，红。
//
// 对照：B2-6 的前一半（不带名单的 RB-A 式事件走 RB-A 路径）。
func TestRbbB1MainSequence(t *testing.T) {
	r := rbbNewModelRig(t, 100, 24)
	r.submitConsume()

	// B1-1
	r.now = rbaT0.Add(5 * time.Second)
	cmd := r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3,
		Speakers: []string{"rbb_a", "rbb_long_name", "克劳德.甲"}}})
	rbbWantNilCmd(t, "B1-1", cmd)
	rbbWantFrame(t, "B1-1", r.m, 11)
	rbbWantCursorNil(t, "B1-1", r.m)
	rbbWantHead(t, "B1-1", r.m, "council t   正在运行  发言轮 1/2  回来 0/3  0s")
	rbbWantBoard(t, "B1-1", rbbBoardOf(r.m),
		"  在跑    rbb_a          0s",
		"  在跑    rbb_long_name  0s",
		"  在跑    克劳德.甲      0s")

	// B1-2
	r.step(rbbTick(12))
	rbbWantHead(t, "B1-2", r.m, "council t   正在运行  发言轮 1/2  回来 0/3  12s")
	b12 := []string{
		"  在跑    rbb_a          7s",
		"  在跑    rbb_long_name  7s",
		"  在跑    克劳德.甲      7s",
	}
	rbbWantBoard(t, "B1-2", rbbBoardOf(r.m), b12...)

	// B1-3：对 m 喂 FIN 得到 mx，丢掉 mx，继续用 m。
	saved := r.m
	mx, cmd := tui1bStep(t, r.m, boardFinalizeMsg{})
	rbbWantNilCmd(t, "B1-3", cmd)
	rbbWantFrame(t, "B1-3", mx, 11)
	rbbWantBoard(t, "B1-3", rbbBoardOf(mx), b12...)

	// B1-4
	r.step(memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rbb_a", Outcome: turn.OutcomeSpoke, WallMS: 18234, RoundID: rbaRID}})
	r.step(memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "克劳德.甲", Outcome: turn.OutcomeTimeout, MaybeAdvanced: true, WallMS: 300000, RoundID: rbaRID}})
	rbbWantHead(t, "B1-4", r.m, "council t   正在运行  发言轮 1/2 (688558)  回来 2/3  12s")
	rbbWantBoard(t, "B1-4", rbbBoardOf(r.m),
		"  完成    rbb_a          18.2s 子进程",
		"  在跑    rbb_long_name  7s",
		"  超时    克劳德.甲      5m00s 子进程  可能已吃下")
	rbbWantBoard(t, "B1-4 saved（B1-2 时存下的 Model 值，COW）", rbbBoardOf(saved), b12...)

	// B1-5
	r.step(rbbTick(65))
	rbbWantHead(t, "B1-5", r.m, "council t   正在运行  发言轮 1/2 (688558)  回来 2/3  1m05s")
	rbbWantBoard(t, "B1-5", rbbBoardOf(r.m),
		"  完成    rbb_a          18.2s 子进程",
		"  在跑    rbb_long_name  1m00s",
		"  超时    克劳德.甲      5m00s 子进程  可能已吃下")

	// B1-6
	r.step(memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rbb_long_name", Outcome: turn.OutcomePostRejected, Dropped: true, Drifted: true, WallMS: 999, RoundID: rbaRID}})
	cmd = r.step(roundEndMsg{ev: repl.RoundEndEvent{Label: "发言轮", K: 1, N: 2, Results: []repl.MemberDoneEvent{
		{Label: "发言轮", AgentID: "rbb_a", Outcome: turn.OutcomeSpoke, WallMS: 1000, RoundID: rbaRID},
	}}})
	rbbWantNilCmd(t, "B1-6 RE", cmd)
	rbbWantHead(t, "B1-6", r.m, "council t   正在运行  发言轮 1/2 (688558)  回来 3/3  1m05s")
	rbbWantBoard(t, "B1-6", rbbBoardOf(r.m),
		"  完成    rbb_a          18.2s 子进程",
		"  未入库  rbb_long_name  0.9s 子进程  正文见上方  (前缀已断)",
		"  超时    克劳德.甲      5m00s 子进程  可能已吃下")
	rbbWantFrame(t, "B1-6", r.m, 11)

	// B1-7
	saved2 := r.m
	r.now = rbaT0.Add(70 * time.Second)
	r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "裁判轮", Members: 1, Speakers: []string{"rbb_j"}}})
	rbbWantHead(t, "B1-7", r.m, "council t   正在运行  裁判轮  回来 0/1  1m05s")
	rbbWantBoard(t, "B1-7", rbbBoardOf(r.m), "  在跑    rbb_j  0s", "", "")
	rbbWantFrame(t, "B1-7", r.m, 11)
	rbbWantBoard(t, "B1-7 saved2（B1-6 之后存下的 Model 值，COW）", rbbBoardOf(saved2),
		"  完成    rbb_a          18.2s 子进程",
		"  未入库  rbb_long_name  0.9s 子进程  正文见上方  (前缀已断)",
		"  超时    克劳德.甲      5m00s 子进程  可能已吃下")

	// B1-8
	r.step(memberDoneMsg{ev: repl.MemberDoneEvent{Label: "裁判轮", AgentID: "rbb_j", Outcome: turn.OutcomeRefused, WallMS: 0, RoundID: rbaRID2}})
	rbbWantHead(t, "B1-8", r.m, "council t   正在运行  裁判轮 (aa08ae)  回来 1/1  1m05s")
	rbbWantBoard(t, "B1-8", rbbBoardOf(r.m), "  拒绝    rbb_j  -", "", "")

	// B1-9
	r.now = rbaT0.Add(125 * time.Second)
	cmd = r.step(submitDoneMsg{})
	rbbWantHead(t, "B1-9", r.m, "council t   空闲")
	rbbWantBoard(t, "B1-9", rbbBoardOf(r.m), "  拒绝    rbb_j  -", "", "")
	rbbWantFrame(t, "B1-9", r.m, 11)
	rbbWantCursor00(t, "B1-9", r.m)
	rbbWantLeaves(t, "B1-9", cmd, "print:/round 结束  2m05s  回来 4/4", rbbFinLeaf)

	// B1-10
	r.m = tui1bType(t, r.m, "y")
	cmd = r.step(tui1bEnter())
	rbbWantNilCmd(t, "B1-10", cmd)
	rbbWantHead(t, "B1-10", r.m, "council t   空闲")
	rbbWantFrame(t, "B1-10", r.m, 11)

	// B1-11
	cmd = r.step(tui1bCtrl('c'))
	rbbWantNilCmd(t, "B1-11", cmd)
	rbbWantHead(t, "B1-11", r.m, "council t   空闲")
	rbbWantCursor00(t, "B1-11", r.m)

	// B1-12
	cmd = r.step(boardFinalizeMsg{})
	rbbWantNilCmd(t, "B1-12", cmd)
	rbbWantFrame(t, "B1-12", r.m, 8)
	if n := len(r.m.chrome()); n != 2 {
		t.Errorf("B1-12：len(chrome()) 应当是 2，实际 %d", n)
	}
	rbbWantCursorNil(t, "B1-12", r.m)
	rbbWantHead(t, "B1-12", r.m, "council t   空闲")

	// B1-13
	cmd = r.step(boardFinalizeMsg{})
	rbbWantNilCmd(t, "B1-13", cmd)
	rbbWantFrame(t, "B1-13", r.m, 8)

	// B1-14
	cmd = r.step(tui1bEnter())
	rbbWantLeaves(t, "B1-14", cmd, "print:> y", "submitDone")
	if n := r.sub.count(); n != 1 {
		t.Errorf("B1-14：跑完 Cmd 之后 Submit 应当恰好被调 1 次，实际 %d", n)
	}
	rbbWantHead(t, "B1-14", r.m, "council t   正在运行  0s")
	rbbWantFrame(t, "B1-14", r.m, 8)
}

// ═══ B2 ═══════════════════════════════════════════════════════════════

// TestRbbB2Boundaries：§3.6 B2 边界表，每条另起一个 Model。
//
// 【判别力】逐条：
//   - B2-1（上限 3、4 人）：不设上限（#30）或上限多 1（#31）→ 出 4 行，帧高 12，红；floor == 0 时也钉光标（#48）→ 光标红；
//     floor == 0 时不打收尾行（#49）→ 叶子红。对照 B2-2：只差人数。
//   - B2-2（上限 3、3 人）：上限少 1（#32）→ 3 行不出，帧高 8，红。
//   - B2-3（终端高未知）：用 height（兜底 24）而不是 termHeight（#33）→ 出 3 行，红。对照 B2-2：只差终端高是否已知。
//   - B2-4：每段重设 floor（#29）→ 第 3 个 RS 后帧高 9；窗口变矮时按新上限夹 floor（#34）→ 帧高变小；
//     放不下时照样显示名单前 floor 行（#35）→ 看板不是三行空串；放不下照样长高（#36）→ 帧高 13；
//     err 支钉光标或走收尾行（#54）→ 光标 / 叶子红；err 支清看板（#55）→ 帧高 8、exitClearSeq 变成上移 7 行，红。
//   - B2-5：收尾行动词看 status 文字（#41）→ 此时 status 是「一轮正在跑…」，印成「结束」，红；
//     忙时 Ctrl-C 不置 canceling（#65）→ 同样印成「结束」，红。
//   - B2-6：看板行数取 Members（#27）或 runSeen 在不带名单的 RS 上也置位（#28）或无论 runSeen 都走新路径（#52）→
//     第一次 submitDone 的 Cmd 非 nil，红；beginSubmit 不清 canceling（#63）→ 第二次印成「已取消」；
//     不清两个累计数（#64）→ 「回来 2/1」，红。
//   - B2-7：活区不截断（#25）→ 宽度超过 30；用 lipgloss Width 截断（#26）→ 折成多行，帧高 ≠ View 行数，红。
func TestRbbB2Boundaries(t *testing.T) {
	t.Run("B2-1_降级", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 12)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 4, Speakers: []string{"a", "b", "c", "d"}}})
		rbbWantFrame(t, "B2-1 RS 之后", r.m, 8)
		if n := len(r.m.chrome()); n != 2 {
			t.Errorf("B2-1 RS 之后：len(chrome()) 应当是 2，实际 %d", n)
		}
		cmd := r.step(submitDoneMsg{})
		rbbWantCursorNil(t, "B2-1 submitDone 之后", r.m)
		rbbWantFrame(t, "B2-1 submitDone 之后", r.m, 8)
		rbbWantLeaves(t, "B2-1 submitDone 之后", cmd, "print:/round 结束  0s  回来 0/4", rbbFinLeaf)
	})

	t.Run("B2-2_上限边界", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 12)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 3, Speakers: []string{"a", "b", "c"}}})
		rbbWantFrame(t, "B2-2", r.m, 11)
		rbbWantBoard(t, "B2-2", rbbBoardOf(r.m), "  在跑    a  0s", "  在跑    b  0s", "  在跑    c  0s")
	})

	t.Run("B2-3_终端高未知", func(t *testing.T) {
		r := rbbNewModelRig(t, 0, 0)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 3, Speakers: []string{"a", "b", "c"}}})
		rbbWantFrame(t, "B2-3", r.m, 8)
		if n := len(r.m.chrome()); n != 2 {
			t.Errorf("B2-3：len(chrome()) 应当是 2，实际 %d", n)
		}
	})

	t.Run("B2-4_只长不缩与出错路径", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 24)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 3, Members: 2, Speakers: []string{"a", "b"}}})
		rbbWantFrame(t, "B2-4 第 1 个 RS", r.m, 10)
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 2, N: 3, Members: 3, Speakers: []string{"a", "b", "c"}}})
		rbbWantFrame(t, "B2-4 第 2 个 RS", r.m, 11)
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 3, N: 3, Members: 1, Speakers: []string{"a"}}})
		rbbWantFrame(t, "B2-4 第 3 个 RS", r.m, 11)
		rbbWantBoard(t, "B2-4 第 3 个 RS", rbbBoardOf(r.m), "  在跑    a  0s", "", "")
		r.step(tea.WindowSizeMsg{Width: 100, Height: 10})
		rbbWantFrame(t, "B2-4 窗口变矮", r.m, 11)
		rbbWantBoard(t, "B2-4 窗口变矮", rbbBoardOf(r.m), "  在跑    a  0s", "", "")
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "裁判轮", Members: 5, Speakers: []string{"j1", "j2", "j3", "j4", "j5"}}})
		rbbWantFrame(t, "B2-4 裁判 RS", r.m, 11)
		rbbWantBoard(t, "B2-4 裁判 RS", rbbBoardOf(r.m), "", "", "")
		rbbWantHead(t, "B2-4 裁判 RS", r.m, "council t   正在运行  裁判轮  回来 0/5  0s")
		cmd := r.step(submitDoneMsg{err: errors.New("坏了")})
		rbbWantHead(t, "B2-4 出错", r.m, "council t   发生错误，正在退出")
		rbbWantFrame(t, "B2-4 出错", r.m, 11)
		rbbWantCursorNil(t, "B2-4 出错", r.m)
		rbbWantLeaves(t, "B2-4 出错", cmd, "print:坏了", "quit")
		// min(11, 10) = 10：从帧末行上移 9 行。
		if got, want := exitClearSeq(r.m, nil), tui1dESC+"[9A\r"+tui1dESC+"[J"; got != want {
			t.Errorf("B2-4 出错：exitClearSeq 应当是 %q，实际 %q", want, got)
		}
	})

	t.Run("B2-5_取消与标志", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 24)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 4, Speakers: []string{"a", "b", "c", "d"}}})
		r.step(memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "a", Outcome: turn.OutcomeCanceled, MaybeAdvanced: true, WallMS: 300}})
		// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
		r.step(tui1bCtrl('c'))
		r.step(tui1bCtrl('c'))
		r.m = tui1bType(t, r.m, "z")
		r.step(tui1bEnter())
		r.step(memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "b", Outcome: turn.OutcomeSpawnFailed, WallMS: 5}})
		r.now = rbaT0.Add(12 * time.Second)
		rbbWantHead(t, "B2-5 submitDone 之前", r.m, "council t   一轮正在跑，先等它回来或连按两次 Ctrl-C 取消  发言轮 1/2  回来 2/4  0s")
		rbbWantFrame(t, "B2-5 submitDone 之前", r.m, 12)
		rbbWantBoard(t, "B2-5 submitDone 之前", rbbBoardOf(r.m),
			"  取消    a  0.3s 子进程  可能已吃下",
			"  没起来  b  ?",
			"  在跑    c  0s",
			"  在跑    d  0s")
		cmd := r.step(submitDoneMsg{})
		rbbWantLeaves(t, "B2-5 submitDone 之后", cmd, "print:/round 已取消  12s  回来 2/4", rbbFinLeaf)
	})

	t.Run("B2-6_每次提交重置", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 24)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}})
		r.step(memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "q", RoundID: rbaRID}})
		// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
		r.step(tui1bCtrl('c'))
		r.step(tui1bCtrl('c'))
		if !r.m.canceling {
			t.Fatalf("B2-6 前提：连按两次 Ctrl-C 之后 canceling 应当为真，实际为假")
		}
		cmd := r.step(submitDoneMsg{})
		rbbWantNilCmd(t, "B2-6 第一次 submitDone", cmd)
		rbbWantCursorNil(t, "B2-6 第一次 submitDone", r.m)
		rbbWantFrame(t, "B2-6 第一次 submitDone", r.m, 8)
		rbbWantHead(t, "B2-6 第一次 submitDone", r.m, "council t   空闲")

		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 1, Speakers: []string{"a"}}})
		r.step(memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "a", Outcome: turn.OutcomeSpoke, WallMS: 1000}})
		cmd = r.step(submitDoneMsg{})
		rbbWantLeaves(t, "B2-6 第二次 submitDone", cmd, "print:/round 结束  0s  回来 1/1", rbbFinLeaf)
		rbbWantFrame(t, "B2-6 第二次 submitDone", r.m, 9)
		rbbWantBoard(t, "B2-6 第二次 submitDone", rbbBoardOf(r.m), "  完成    a  1.0s 子进程")
		rbbWantCursor00(t, "B2-6 第二次 submitDone", r.m)
	})

	t.Run("B2-7_截断", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 24)
		r.submitConsume()
		r.step(tea.WindowSizeMsg{Width: 30, Height: 24})
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 2, Speakers: []string{"克劳德.甲乙丙丁戊己", "b"}}})
		r.step(memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "克劳德.甲乙丙丁戊己", Outcome: turn.OutcomePostRejected, Dropped: true, Drifted: true, WallMS: 61000}})
		rbbWantFrame(t, "B2-7", r.m, 10)
		board := rbbBoardOf(r.m)
		rbbWantBoard(t, "B2-7", board, "  未入库  克劳德.甲乙丙丁戊己 ", "  在跑    b"+strings.Repeat(" ", 19))
		for i, ln := range board {
			if w := ansi.StringWidth(ln); w != 30 {
				t.Errorf("B2-7：看板第 %d 行的显示宽度应当是 30，实际 %d（%q）", i+1, w, ln)
			}
		}
	})
}
