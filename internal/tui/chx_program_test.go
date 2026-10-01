package tui

// CH1 §2.2：真 tea.Program、真 REPL、真 Runner、假 CLI（本包 rba_fakecli_test.go 的 TestMain）。
// 装置 rbaNewTuiRig、rbbStartProg 与 spy 只读复用（rba_fakecli_test.go、rbb_order_test.go）。
//
// 背对背纪律：本文件由测试腿依据冻结契约 CH1 v1.7 与 HEAD dd0c15b 的源码写成，看不见实现腿的产出。
// 引用本刀新增的 (*repl.REPL).SetAutoReply，在 HEAD 上整包编译不过，是契约 §2 写明的预期红。

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	chxTuiHeadRe = regexp.MustCompile(`（\d+\.\ds）`)
	chxTuiFinRe  = regexp.MustCompile(`^/round 结束  \d+s  回来 2/2$`)
)

// chxTuiSeq 把日志从 P "> 第一行" 起（含）记成契约 §2.2 的序列：丢掉 TICK；P 记成 "P " + 正文（成员头用时换 （T），
// 收尾行换成 "/round 结束 FIN"）；RS 记成 "RS " + Label，同时断言它的 Speakers 是 [rba_a rba_b]；其余记 kind。
func chxTuiSeq(t *testing.T, log []rbbEntry) []string {
	t.Helper()
	start := -1
	for i, e := range log {
		if e.kind == "P" && e.text == "> 第一行" {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("前提不成立：日志里没有 P \"> 第一行\"；日志:\n%s", rbbLogDump(log))
	}
	var seq []string
	for _, e := range log[start:] {
		switch e.kind {
		case "TICK":
			continue
		case "P":
			text := chxTuiHeadRe.ReplaceAllString(e.text, "（T）")
			if chxTuiFinRe.MatchString(e.text) {
				text = "/round 结束 FIN"
			}
			seq = append(seq, "P "+text)
		case "RS":
			if !reflect.DeepEqual(e.rs.Speakers, []string{"rba_a", "rba_b"}) {
				t.Fatalf("RS 的 Speakers 应当是 [rba_a rba_b]，实际 %q；日志:\n%s", e.rs.Speakers, rbbLogDump(log))
			}
			seq = append(seq, "RS "+e.rs.Label)
		default:
			seq = append(seq, e.kind)
		}
	}
	return seq
}

// TestChxTuiMultilineBareAutoRound 是 T1：TUI 的一次多行裸文本提交在自动回复打开时问一轮。
//
// 【判别力】TUI 那条裸文本路径经 Submit 的 bare，多行正文整段是一条消息；manual 是同装置只差开关的缺席对照
// （只有两行回显与一个 DONE），它能为假：自动一轮一旦在开关关着时也起，序列里多出 RS / MD 等。
// Submit 不置 bare（M2）时 auto 只剩 manual 那三项，且等不到 FIN，红。auto 同时钉住：judge 照 PlanRound
// 进「本命令不问」；没配总结成员的简洁档照 SM1 打那句提示；收尾行仍写 /round 结束。杀 M2。
func TestChxTuiMultilineBareAutoRound(t *testing.T) {
	for _, tc := range []struct {
		name string
		auto bool
		want []string
	}{
		{"auto", true, []string{
			"P > 第一行", "P > 第二行", "P 裁判 rba_j 本命令不问，要用 --with-judge 才问。", "P 本轮问 2 个: rba_a, rba_b",
			"RS 发言轮", "P X（T）: OK", "MD", "P X（T）: OK", "MD", "RE",
			"P 总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）",
			"DONE", "P /round 结束 FIN", "FIN",
		}},
		{"manual", false, []string{"P > 第一行", "P > 第二行", "DONE"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := rbaNewTuiRig(t)
			g.r.SetConcise(true)
			g.r.SetAutoReply(tc.auto)
			pr := rbbStartProg(t, g)
			pr.useConv(t, g.convID)
			pr.submit("第一行\n第二行")
			if !pr.waitFor(20*time.Second, func(log []rbbEntry) bool { return rbbCount(log, "DONE") >= 2 }) {
				t.Fatalf("前提不成立：20 秒内没有等到这一次提交的 DONE；日志:\n%s", rbbLogDump(pr.spy.entries()))
			}
			if tc.auto {
				if !pr.waitFor(5*time.Second, func(log []rbbEntry) bool { return rbbCount(log, "FIN") >= 1 }) {
					t.Fatalf("5 秒内没有等到 FIN；日志:\n%s", rbbLogDump(pr.spy.entries()))
				}
			}
			got := chxTuiSeq(t, pr.spy.entries())
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("序列不对\n实际: %q\n期望: %q\n日志:\n%s", got, tc.want, rbbLogDump(pr.spy.entries()))
			}
		})
	}
}
