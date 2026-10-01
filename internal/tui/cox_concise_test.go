package tui

// CO1 §2.3：TUI 适配器在简洁档下不发段终态块（T1，纯函数），
// 以及真 tea.Program 上简洁档只有收尾行、没有第二行总用时（T2）。
// 引用本刀新增的 RoundEndEvent.Concise 与 (*repl.REPL).SetConcise，在 HEAD 上整包编译不过是预期的。

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
	"council/internal/turn"
)

// TestCoxRoundEndConciseSendsOnlyEndMsg 是 T1：简洁档的 RoundEnd 只发 roundEndMsg；详细档照发 3 行块。
//
// 【判别力】Results 非空（包装层要喂原返回值非空的输入：空 Results 本来就不出块，杀不到「忽略 Concise」）：
// 忽略 Concise（if true）时简洁档收到 4 条，红；把 roundEndMsg 也挪进 if 时简洁档收到 0 条，红。
// 对照是同一个 ev 改成 Concise=false：恰好 4 条，前 3 条是段终态块，证明这个 ev 确实会出块。
func TestCoxRoundEndConciseSendsOnlyEndMsg(t *testing.T) {
	ev := repl.RoundEndEvent{Label: "发言轮", K: 1, N: 1, Concise: true, Results: []repl.MemberDoneEvent{
		{Label: "发言轮", AgentID: "cox_a", Outcome: turn.OutcomeSpoke, WallMS: 6849, RoundID: "r-000000abcdef"},
		{Label: "发言轮", AgentID: "cox_b", Outcome: turn.OutcomeTimeout, MaybeAdvanced: true, WallMS: 300000, RoundID: "r-000000abcdef"},
	}}
	var got []tea.Msg
	newRoundObserver(func(msg tea.Msg) { got = append(got, msg) }).RoundEnd(ev)
	if len(got) != 1 {
		t.Fatalf("简洁档应当恰好收到 1 条消息，实际 %d 条: %#v", len(got), got)
	}
	if !reflect.DeepEqual(got[0], roundEndMsg{ev: ev}) {
		t.Fatalf("简洁档唯一的一条应当 DeepEqual roundEndMsg{ev: ev}，实际 %#v", got[0])
	}

	ev.Concise = false
	got = nil
	newRoundObserver(func(msg tea.Msg) { got = append(got, msg) }).RoundEnd(ev)
	if len(got) != 4 {
		t.Fatalf("详细档应当恰好收到 4 条消息，实际 %d 条: %#v", len(got), got)
	}
	want := []string{
		"发言轮 (abcdef)  回来 2/2",
		"  完成    cox_a  6.8s 子进程",
		"  超时    cox_b  5m00s 子进程  可能已吃下",
	}
	for i, w := range want {
		body, ok := rbbBody(got[i])
		if !ok {
			t.Fatalf("详细档第 %d 条应当是 printLine，实际 %T", i+1, got[i])
		}
		if body != w {
			t.Fatalf("详细档第 %d 行不对\n实际: %q\n期望: %q", i+1, body, w)
		}
	}
	if !reflect.DeepEqual(got[3], roundEndMsg{ev: ev}) {
		t.Fatalf("详细档第 4 条应当 DeepEqual roundEndMsg{ev: ev}，实际 %#v", got[3])
	}
}

var (
	coxFinRe     = regexp.MustCompile(`^/round 结束  \d+s  回来 2/2$`)
	coxHeadXRe   = regexp.MustCompile(`^X（\d+\.\ds）: OK$`)
	coxBlockBRe  = regexp.MustCompile(`^  完成    rba_b  \d+\.\ds 子进程$`)
	coxRoundText = "/round rba_a rba_b --go"
)

// TestCoxProgramConciseNoBlockOneTotal 是 T2：真 tea.Program、真 REPL、真 Runner、假 CLI。
//
// 【判别力】两个子测试只差档位。两者都断言：事件序列恰好 RS MD MD RE DONE FIN、窗口里没有以「总用时」开头的行
// （装了观察者时 REPL 不打总用时；去掉 r.roundObs != nil 判据时多一行，红）、最后一条 P 是收尾行。
// concise：P 恰好 5 条（本轮问、两个成员头、没配总结成员的提示行、收尾行），RE 前紧挨着的是 MD——适配器忽略 Concise 时中间夹着块，红；
// RoundEnd 不带 Concise 时同样出块，红。verbose：RE 前紧挨着的是段终态块的最后一行，证明详细档的块还在。
func TestCoxProgramConciseNoBlockOneTotal(t *testing.T) {
	for _, tc := range []struct {
		name    string
		concise bool
	}{{"concise", true}, {"verbose", false}} {
		t.Run(tc.name, func(t *testing.T) {
			g := rbaNewTuiRig(t)
			if tc.concise {
				g.r.SetConcise(true)
			}
			pr := rbbStartProg(t, g)
			pr.useConv(t, g.convID)
			pr.submit(coxRoundText)
			if !pr.waitFor(30*time.Second, rbbDoneThenFin(2)) {
				t.Fatalf("30 秒内没有等到 /round 的 DONE 与 FIN；日志:\n%s", rbbLogDump(pr.spy.entries()))
			}
			log := pr.spy.entries()
			w, ok := rbbWindow(log, coxRoundText)
			if !ok {
				t.Fatalf("取不到 /round 的窗口；日志:\n%s", rbbLogDump(log))
			}
			if kinds, _ := rbbEventKinds(w); kinds != "RS MD MD RE DONE FIN" {
				t.Fatalf("事件序列应当恰好 RS MD MD RE DONE FIN，实际 %q；窗口:\n%s", kinds, rbbLogDump(w))
			}
			var ps []string
			reIdx := -1
			for i, e := range w {
				if e.kind == "P" {
					ps = append(ps, e.text)
					if strings.HasPrefix(e.text, "总用时") {
						t.Fatalf("TUI 里不应当有以「总用时」开头的行，实际 %q；窗口:\n%s", e.text, rbbLogDump(w))
					}
				}
				if e.kind == "RE" && reIdx < 0 {
					reIdx = i
				}
			}
			if len(ps) == 0 || !coxFinRe.MatchString(ps[len(ps)-1]) {
				t.Fatalf("最后一条 P 应当是收尾行；窗口:\n%s", rbbLogDump(w))
			}
			if reIdx < 1 {
				t.Fatalf("窗口里 RE 之前没有条目；窗口:\n%s", rbbLogDump(w))
			}
			prev := w[reIdx-1]
			if tc.concise {
				if len(ps) != 5 {
					t.Fatalf("简洁档 P 应当恰好 5 条，实际 %d 条；窗口:\n%s", len(ps), rbbLogDump(w))
				}
				if ps[3] != "总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）" {
					t.Fatalf("第 4 条 P 应当是没配总结成员的提示行，实际 %q；窗口:\n%s", ps[3], rbbLogDump(w))
				}
				if ps[0] != "本轮问 2 个: rba_a, rba_b" {
					t.Fatalf("第 1 条 P 不对: %q", ps[0])
				}
				if !coxHeadXRe.MatchString(ps[1]) || !coxHeadXRe.MatchString(ps[2]) {
					t.Fatalf("第 2、3 条 P 应当是成员头，实际 %q / %q", ps[1], ps[2])
				}
				if prev.kind != "MD" {
					t.Fatalf("简洁档 RE 前紧挨着的应当是 MD（中间没有块），实际 %s；窗口:\n%s", prev.String(), rbbLogDump(w))
				}
			} else {
				if prev.kind != "P" || !coxBlockBRe.MatchString(prev.text) {
					t.Fatalf("详细档 RE 前紧挨着的应当是段终态块的最后一行，实际 %s；窗口:\n%s", prev.String(), rbbLogDump(w))
				}
			}
		})
	}
}
