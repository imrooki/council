package tui

// 本文件覆盖冻结契约 RB-B v1.1 §2.2 的 K1-K7：适配器 RoundEnd 发出的终态块。纯函数，不起 Program。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD a728cee 的源码。
// printLine 消息的正文用 RB-A 的取法（%T 为 tea.printLineMessage，反射取 messageBody），
// 不调生产的 printLine 造期望值。期望值是 §3.5 的逐字节常量，逐行整行比较。
//
// 本文件引用本刀新增的 Results 字段，在 HEAD 上整包编译不过，这是契约 §2 写明的预期红。

import (
	"fmt"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
	"council/internal/turn"
)

// rbbObserve 调一次 RoundEnd，收下 send 收到的全部消息。
func rbbObserve(ev repl.RoundEndEvent) []tea.Msg {
	var got []tea.Msg
	o := newRoundObserver(func(msg tea.Msg) { got = append(got, msg) })
	o.RoundEnd(ev)
	return got
}

// rbbBody 认出 printLine 消息并取正文。
func rbbBody(msg tea.Msg) (string, bool) {
	if fmt.Sprintf("%T", msg) != "tea.printLineMessage" {
		return "", false
	}
	return reflect.ValueOf(msg).FieldByName("messageBody").String(), true
}

// rbbCheckBlock 断言 got 恰好是 len(want) 条 printLine（正文逐条整行等于 want）加最后一条 roundEndMsg{ev}。
func rbbCheckBlock(t *testing.T, name string, ev repl.RoundEndEvent, got []tea.Msg, want []string) {
	t.Helper()
	if len(got) != len(want)+1 {
		t.Errorf("%s：send 应当恰好收到 %d 条消息（%d 行块 + roundEndMsg），实际 %d 条: %#v", name, len(want)+1, len(want), len(got), got)
	}
	for i, w := range want {
		if i >= len(got) {
			break
		}
		body, ok := rbbBody(got[i])
		if !ok {
			t.Errorf("%s：第 %d 条应当是 printLine，实际 %T", name, i+1, got[i])
			continue
		}
		if body != w {
			t.Errorf("%s：第 %d 行正文应当整行等于\n  %q\n实际\n  %q", name, i+1, w, body)
		}
	}
	if len(got) == len(want)+1 {
		if !reflect.DeepEqual(got[len(want)], roundEndMsg{ev: ev}) {
			t.Errorf("%s：最后一条应当 DeepEqual roundEndMsg{ev: ev}，实际 %#v", name, got[len(want)])
		}
	}
}

// §3.5 的逐字节期望。每行都没有行尾空格。
var (
	rbbK1Want = []string{
		"发言轮 1/2 (688558)  回来 3/3",
		"  完成    rbb_a          18.2s 子进程",
		"  未入库  rbb_long_name  0.9s 子进程  正文见上方  (前缀已断)",
		"  超时    克劳德.甲      5m00s 子进程  可能已吃下",
	}
	rbbK3Want = []string{
		"发言轮  回来 10/10",
		"  未知    o0  -",
		"  完成    o1  1.2s 子进程",
		"  未入库  o2  1.2s 子进程",
		"  报错    o3  1.2s 子进程",
		"  认不出  o4  1.2s 子进程",
		"  超时    o5  1.2s 子进程",
		"  没起来  o6  ?",
		"  拒绝    o7  -",
		"  取消    o8  1.2s 子进程",
		"  丢失    o9  -",
	}
	rbbK4Want = []string{
		"发言轮  回来 11/11",
		"  完成    wa  0.0s 子进程",
		"  完成    wb  0.0s 子进程",
		"  完成    wc  0.0s 子进程",
		"  完成    wd  0.1s 子进程",
		"  完成    we  0.9s 子进程",
		"  完成    wf  1.0s 子进程",
		"  完成    wg  18.2s 子进程",
		"  完成    wh  59.9s 子进程",
		"  完成    wi  1m00s 子进程",
		"  完成    wj  1m05s 子进程",
		"  完成    wk  1h00m 子进程",
	}
	rbbK5Want = []string{
		"发言轮  回来 3/3",
		"  完成    克劳德  1.0s 子进程",
		"  完成    ab      1.0s 子进程",
		"  未入库  m1      1.0s 子进程  可能已吃下  正文见上方  (前缀已断)",
	}
)

// TestRbbK1BlockLinesOrderThenRoundEnd：块的行、顺序、最后一条是 roundEndMsg。
//
// 【判别力】能为假：先发 roundEndMsg 再发块（#13）→ 第 1 条不是 printLine，红；标题 back: 0（#16）或
// 不带 roundID → 第 1 行红；终态行用「在跑」→ 红；标记顺序变（#23）→ 第 3 行红；
// id 按字节补齐（#24）→ 「克劳德.甲」一行（13 字节、宽 9）少补 4 格，红。
// 对照：K7（Results 为空只发 1 条）。
func TestRbbK1BlockLinesOrderThenRoundEnd(t *testing.T) {
	ev := repl.RoundEndEvent{Label: "发言轮", K: 1, N: 2, Results: []repl.MemberDoneEvent{
		{Label: "发言轮", AgentID: "rbb_a", Outcome: turn.OutcomeSpoke, WallMS: 18234, RoundID: rbaRID},
		{Label: "发言轮", AgentID: "rbb_long_name", Outcome: turn.OutcomePostRejected, Dropped: true, Drifted: true, WallMS: 999, RoundID: rbaRID},
		{Label: "发言轮", AgentID: "克劳德.甲", Outcome: turn.OutcomeTimeout, MaybeAdvanced: true, WallMS: 300000, RoundID: rbaRID},
	}}
	rbbCheckBlock(t, "K1", ev, rbbObserve(ev), rbbK1Want)
}

// TestRbbK2JudgeSpawnFailedEmptyRoundID：裁判轮、SpawnFailed、空 RoundID。
//
// 【判别力】能为假：SpawnFailed 的时间格印成 - 或子进程时长（#19）→ 第 2 行红；
// 裁判轮 n=0 时照样拼 k/n、或空 RoundID 也拼括号 → 第 1 行红。对照：K1 的标题带 k/n 与括号。
func TestRbbK2JudgeSpawnFailedEmptyRoundID(t *testing.T) {
	ev := repl.RoundEndEvent{Label: "裁判轮", Results: []repl.MemberDoneEvent{
		{Label: "裁判轮", AgentID: "rbb_j", Outcome: turn.OutcomeSpawnFailed},
	}}
	rbbCheckBlock(t, "K2", ev, rbbObserve(ev), []string{"裁判轮  回来 1/1", "  没起来  rbb_j  ?"})
}

// TestRbbK3OutcomeWordTable：状态词与时间格全表。
//
// 【判别力】能为假：任意一个 Outcome 的词写错（#18）或时间格错（#19：SpawnFailed 印 -、Refused 印 ?）→
// 对应那一行红；状态词按字节数补齐（#24）→ 两字词（6 字节、宽 4）后少两个空格，除「未入库」「认不出」「没起来」
// 外全部红。三字词行与两字词行互为对照（三字词宽 6，补齐恰好为 0）。
func TestRbbK3OutcomeWordTable(t *testing.T) {
	outs := []turn.Outcome{
		turn.OutcomeUnknown, turn.OutcomeSpoke, turn.OutcomePostRejected, turn.OutcomeProviderFailed,
		turn.OutcomeUnparsable, turn.OutcomeTimeout, turn.OutcomeSpawnFailed, turn.OutcomeRefused,
		turn.OutcomeCanceled, turn.OutcomeLost,
	}
	res := make([]repl.MemberDoneEvent, len(outs))
	for i, o := range outs {
		res[i] = repl.MemberDoneEvent{AgentID: fmt.Sprintf("o%d", i), Outcome: o, WallMS: 1234}
	}
	ev := repl.RoundEndEvent{Label: "发言轮", K: 1, N: 1, Results: res}
	rbbCheckBlock(t, "K3", ev, rbbObserve(ev), rbbK3Want)
}

// TestRbbK4WallFormatBoundaries：子进程时长的格式边界。
//
// 【判别力】能为假：%.1f 四舍五入（#20）→ 59999 印 60.0s、99 印 0.1s、999 印 1.0s，红；
// `< 60000` 写成 `<= 60000`（#21）→ 60000 印 60.0s，红；不夹负数（#22）→ -1500 印 -1.-5s，红
// （-1500 而不是 -5：Go 向零截断，-5 不夹也印 0.0s，杀不到）。相邻两行（59999 与 60000、99 与 100）互为对照。
func TestRbbK4WallFormatBoundaries(t *testing.T) {
	walls := []int{-1500, 0, 99, 100, 999, 1000, 18234, 59999, 60000, 65432, 3600000}
	res := make([]repl.MemberDoneEvent, len(walls))
	for i, w := range walls {
		res[i] = repl.MemberDoneEvent{AgentID: "w" + string(rune('a'+i)), Outcome: turn.OutcomeSpoke, WallMS: w}
	}
	ev := repl.RoundEndEvent{Label: "发言轮", K: 1, N: 1, Results: res}
	rbbCheckBlock(t, "K4", ev, rbbObserve(ev), rbbK4Want)
}

// TestRbbK5IDPadByWidthAndMarkOrder：id 按显示宽度补齐，三个标记的顺序。
//
// 【判别力】能为假：id 列宽按字节数（「克劳德」9 字节、宽 6，#24）→ 后两行多 3 个空格，红；
// 标记顺序变（#23）→ 最后一行红。对照：第 1 行（最宽的 id 本身不补齐）。
func TestRbbK5IDPadByWidthAndMarkOrder(t *testing.T) {
	ev := repl.RoundEndEvent{Label: "发言轮", K: 1, N: 1, Results: []repl.MemberDoneEvent{
		{AgentID: "克劳德", Outcome: turn.OutcomeSpoke, WallMS: 1000},
		{AgentID: "ab", Outcome: turn.OutcomeSpoke, WallMS: 1000},
		{AgentID: "m1", Outcome: turn.OutcomePostRejected, MaybeAdvanced: true, Dropped: true, Drifted: true, WallMS: 1000},
	}}
	rbbCheckBlock(t, "K5", ev, rbbObserve(ev), rbbK5Want)
}

// TestRbbK6TitleFirstNonEmptyRoundID：段标题取第一个非空 RoundID。
//
// 【判别力】能为假：只看 Results[0].RoundID（#17）→ 标题没有括号，红。对照：K1（第一项就非空）。
func TestRbbK6TitleFirstNonEmptyRoundID(t *testing.T) {
	ev := repl.RoundEndEvent{Label: "发言轮", K: 1, N: 1, Results: []repl.MemberDoneEvent{
		{AgentID: "x", Outcome: turn.OutcomeSpoke, WallMS: 1000, RoundID: ""},
		{AgentID: "y", Outcome: turn.OutcomeSpoke, WallMS: 1000, RoundID: rbaRID},
	}}
	got := rbbObserve(ev)
	if len(got) != 4 {
		t.Fatalf("send 应当恰好收到 4 条（标题 + 两行 + roundEndMsg），实际 %d", len(got))
	}
	body, ok := rbbBody(got[0])
	if !ok {
		t.Fatalf("第 1 条应当是 printLine，实际 %T", got[0])
	}
	if want := "发言轮 (688558)  回来 2/2"; body != want {
		t.Errorf("段标题应当整行等于 %q，实际 %q", want, body)
	}
}

// TestRbbK7EmptyResultsOnlyRoundEnd：Results 为空只发一条 roundEndMsg。
//
// 【判别力】能为假：Results 为空时也打段标题（#14）→ 2 条，红。对照：K2（一项 Results 发 3 条）。
func TestRbbK7EmptyResultsOnlyRoundEnd(t *testing.T) {
	ev := repl.RoundEndEvent{Label: "发言轮", K: 2, N: 3}
	got := rbbObserve(ev)
	if len(got) != 1 {
		t.Fatalf("Results 为空时 send 应当恰好收到 1 条，实际 %d 条: %#v", len(got), got)
	}
	if !reflect.DeepEqual(got[0], roundEndMsg{ev: ev}) {
		t.Errorf("唯一一条应当 DeepEqual roundEndMsg{ev: ev}，实际 %#v", got[0])
	}
}
