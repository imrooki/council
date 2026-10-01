package repl

// CH1 §2.1：对话模式的端到端用例（真 SQLite、真 Runner、假 CLI；本包 session_inflight_test.go 的 TestMain
// 扮演 claude：argv 带 --resume 时睡 700ms 再应答，正文 OK）。装置只读复用 smx_summary_round_test.go 的 smxSetup 等辅助。
//
// 背对背纪律：本文件由测试腿依据冻结契约 CH1 v1.7 与 HEAD dd0c15b 的源码写成，看不见实现腿的产出。
// 期望值与字面量逐字取自契约 §2.1 与 §3（测试侧独立抄写，不引用生产代码的 msgAuto* 常量）。
//
// 引用本刀新增的 (*REPL).SetAutoReply、REPL 字段 autoReply 与函数 hasStrayAt，在 HEAD 上整包编译不过，
// 是契约 §2 写明的预期红。

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
)

// §3.1、§3.2、§3.3 的冻结字面量（测试侧独立抄写，逐字节）。
const (
	chxAutoOn          = "已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁），多个成员回复之后自动总结；/auto off 关掉"
	chxAutoOnNoSummary = "已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁）；没配总结成员，多个成员回复之后不做总结；/auto off 关掉"
	chxAutoOff         = "已关闭自动回复：直接打字只记下，不问任何人；要成员说话用 /ask 或 /round；/auto on 打开"
	chxAutoUsage       = "用法: /auto 在自动回复与只记下之间切换；/auto on 直接打字就让成员回复；/auto off 直接打字只记下"

	chxNoneMentioned = "点名的人都不是当前会话里能回复的成员，这条只记下了、没有问任何人；不点名就会问全部成员；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复"
	chxNoMembers     = "当前会话还没有能回复的成员，这条只记下了；用 /add 加成员之后，直接打字就会有人回复"
	chxStrayAt       = "有 @ 没认出点的是谁，这条只记下了、没有问任何人；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复；TUI 里敲 @ 会列出成员的 id 与显示名"
	chxStrayAtPartly = "还有 @ 没认出点的是谁，没算进点名；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；TUI 里敲 @ 会列出成员的 id 与显示名"

	chxRejFmt = "%s 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结"
)

// §2.1 的记号：NONE / EMPTY / STRAY / PARTLY 是 §3.2 四句各加换行；ONE_A、ONE_B、JUDGE 照 C3 表头。
const (
	chxNone   = chxNoneMentioned + "\n"
	chxEmpty  = chxNoMembers + "\n"
	chxStray  = chxStrayAt + "\n"
	chxPartly = chxStrayAtPartly + "\n"
	chxOneA   = "本轮问 1 个: smx_a\n甲（T）: OK\n总用时 7.2s\n"
	chxOneB   = "本轮问 1 个: smx_b\nsmx_b（T）: OK\n总用时 7.2s\n"
	chxJudge  = "裁判 smx_j 本命令不问，要用 --with-judge 才问。\n"
)

// errChxWrite 是写屏幕失败的哨兵（契约 C11 冻结的名字）。
var errChxWrite = errors.New("chx: 写屏幕失败")

// chxAuto 搭 smxSetup 的装置并打开自动回复。
func chxAuto(t *testing.T, members []string, concise, slowA bool) *smxRig {
	t.Helper()
	g := smxSetup(t, "smx_s", members, concise, slowA)
	g.r.SetAutoReply(true)
	return g
}

// chxUserTexts 取库里 SenderID 为 user 的全部消息正文，按 seq 顺序。
func chxUserTexts(t *testing.T, g *smxRig) []string {
	t.Helper()
	var out []string
	for _, m := range g.msgsFrom(t, conversation.ReservedSenderUser) {
		out = append(out, m.Text)
	}
	return out
}

// chxRequireUsers 断言 user 消息正文恰好是 want（消息照常记下；也是「没有多记、没有少记」）。
func chxRequireUsers(t *testing.T, g *smxRig, want ...string) {
	t.Helper()
	got := chxUserTexts(t, g)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") || len(got) != len(want) {
		t.Fatalf("库里 user 消息应当恰好是 %q，实际 %q", want, got)
	}
}

// chxRequireNoMemberMsgs：smx_a、smx_b、smx_s、smx_j 都没有消息（这一句谁都没问，总结也没做）。
func chxRequireNoMemberMsgs(t *testing.T, g *smxRig) {
	t.Helper()
	for _, id := range []string{"smx_a", "smx_b", "smx_s", "smx_j"} {
		g.requireNoMsgs(t, id)
	}
}

// chxGuardSlowA 是慢 a 的前提守卫：两人的成员头都在时，用 smxRequireBBeforeA 要求 smx_b 先回来。
// 成员头缺了（这一轮根本没问，或只问了一个人）时不在这里判，交给随后的整串断言报「整串不对」，
// 免得一个真实的红被记成「前提不成立」。两行都在、顺序反了，才是慢 a 这一前提本身没成立。
func chxGuardSlowA(t *testing.T, raw string) {
	t.Helper()
	if strings.Contains(raw, "smx_b（") && strings.Contains(raw, "甲（") {
		smxRequireBBeforeA(t, raw)
	}
}

// chxOneSuffix 数以 suffix 结尾的行。
func chxOneSuffix(lines []string, suffix string) int {
	n := 0
	for _, l := range lines {
		if strings.HasSuffix(l, suffix) {
			n++
		}
	}
	return n
}

// ─── C1 ─────────────────────────────────────────────────────────────

// TestChxAutoEqualsManualRound 是 C1：自动一轮的屏幕与手敲 /round <ids> --go 逐字节相同。
//
// 【判别力】auto 与 manual 第二步是同一个期望 smxS1Want，这就是「与手敲同样名单的 /round --go 一致」的逐字节担保；
// manual 第一步是自动那一侧的缺席对照（同装置、只差开关），它能为假：自动一轮一旦在开关关着时也起，缓冲非空、时钟非 0。
// auto 能为假：Submit 不置 bare（M2）时输出为空；名单不带 --go（M14）时只打预算表不起进程；
// 没有 mention 也走「有 mention」那一支（M26）时交集为空打 NONE；postUser 没有 token 时报没落库（M29）时一个进程都不起。
// 杀 M2、M14、M26、M29。
func TestChxAutoEqualsManualRound(t *testing.T) {
	t.Run("auto", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a", "smx_b"}, true, true)
		g.submit(t, "你好")
		raw := g.buf.String()
		chxGuardSlowA(t, raw)
		smxEq(t, "自动一轮的输出", smxNormalize(raw), smxS1Want)
		smxClockIs(t, g, 2)
		g.requireSummaryMsg(t)
		chxRequireUsers(t, g, "seed-msg", "seed-msg-2", "你好")
	})
	t.Run("manual", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, true, true)
		g.submit(t, "你好")
		smxEq(t, "只记下时的输出", g.buf.String(), "")
		smxClockIs(t, g, 0)
		g.requireNoMsgs(t, "smx_s")
		g.buf.Reset()
		g.submit(t, "/round smx_a smx_b --go")
		raw := g.buf.String()
		chxGuardSlowA(t, raw)
		smxEq(t, "手敲 /round 的输出", smxNormalize(raw), smxS1Want)
		chxRequireUsers(t, g, "seed-msg", "seed-msg-2", "你好")
	})
}

// ─── C2 ─────────────────────────────────────────────────────────────

// TestChxAutoSecondMessage 是 C2：总结成员第一次总结之后已是活跃成员，第二句话起必须剔掉。
//
// 【判别力】§0 第 4 条那条真机第二句的路径；不剔总结成员（M10）时两个子测试都变成「拒绝: smx_s 是总结成员……」，
// second 的 6 行断言与 summary_member_from_start 的整串都红。对照是 C1 auto（总结成员尚未加入时剔不剔都一样，
// 所以 C1 杀不了 M10）。second 的第 2、3 行排序后比：第二句两人都带 --resume、都睡 700ms，先后不固定。
func TestChxAutoSecondMessage(t *testing.T) {
	t.Run("second", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a", "smx_b"}, true, true)
		g.submit(t, "你好")
		raw := g.buf.String()
		chxGuardSlowA(t, raw)
		smxEq(t, "第一句的输出", smxNormalize(raw), smxS1Want)
		if p, ok := g.member(t, "smx_s"); !ok || !p.Active {
			t.Fatalf("前提不成立：第一句之后 smx_s 应当在成员表里且激活，实际 在表=%v Active=%v", ok, p.Active)
		}
		g.buf.Reset()
		g.submit(t, "再说一句")
		got := smxLines(smxNormalize(g.buf.String()))
		if len(got) != 6 {
			t.Fatalf("第二句的输出应当恰好 6 行，实际 %d 行: %q", len(got), got)
		}
		mid := []string{got[1], got[2]}
		sort.Strings(mid)
		got[1], got[2] = mid[0], mid[1]
		want := []string{"本轮问 2 个: smx_a, smx_b", "smx_b（T）: OK", "甲（T）: OK", "本轮问 1 个: smx_s", "总的来看（总结 S 做总结）：OK", "总用时 0.0s"}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("第二句的输出（第 2、3 行排序后）不对\n实际: %q\n期望: %q", got, want)
		}
		if ms := g.msgsFrom(t, "smx_s"); len(ms) != 2 {
			t.Fatalf("库里 smx_s 的消息应当恰好 2 条，实际 %d 条", len(ms))
		}
	})
	t.Run("summary_member_from_start", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a", "smx_b", "smx_s"}, true, true)
		g.submit(t, "你好")
		raw := g.buf.String()
		chxGuardSlowA(t, raw)
		smxEq(t, "输出", smxNormalize(raw), smxS1Want)
	})
}

// ─── C3 ─────────────────────────────────────────────────────────────

// chxRosterRow 是 C3 表的一行（由脚本从契约 §2.1 C3 表逐字生成）。
type chxRosterRow struct {
	name    string
	members []string
	slowA   bool
	leaveB  bool
	input   string
	want    string
	clock   int
}

// chxRosterRows 是 C3 的全部 76 行，顺序与契约相同。
var chxRosterRows = []chxRosterRow{
	{"subset", []string{"smx_a", "smx_b"}, false, false, "@smx_b 看一下", chxOneB, 2},
	{"upper", []string{"smx_a", "smx_b"}, false, false, "@SMX_B 看一下", chxOneB, 2},
	{"order", []string{"smx_a", "smx_b"}, true, false, "@smx_b @smx_a 看一下", smxS1Want, 2},
	{"all", []string{"smx_a", "smx_b"}, true, false, "看一下", smxS1Want, 2},
	{"not_member", []string{"smx_a", "smx_b"}, false, false, "@ghost 看一下", "ghost 不是当前会话的成员\n" + chxNone, 0},
	{"left", []string{"smx_a", "smx_b"}, false, true, "@smx_b 看一下", chxNone, 0},
	{"left_all", []string{"smx_a", "smx_b"}, false, true, "看一下", chxOneA, 2},
	{"left_mixed", []string{"smx_a", "smx_b"}, false, true, "@smx_b @smx_a 看一下", chxOneA, 2},
	{"summary_mentioned", []string{"smx_a", "smx_s"}, false, false, "@smx_s 看一下", chxNone, 0},
	{"mixed", []string{"smx_a", "smx_b"}, false, false, "@ghost @smx_a 看一下", "ghost 不是当前会话的成员\n" + chxOneA, 2},
	{"no_members", nil, false, false, "看一下", chxEmpty, 0},
	{"only_summary", []string{"smx_s"}, false, false, "看一下", chxEmpty, 0},
	{"only_summary_mentioned", []string{"smx_s"}, false, false, "@smx_s 看一下", chxNone, 0},
	{"summary_plus_one", []string{"smx_a", "smx_s"}, false, false, "看一下", chxOneA, 2},
	{"judge", []string{"smx_a", "smx_j"}, false, false, "看一下", chxJudge + chxOneA, 2},
	{"judge_only", []string{"smx_a", "smx_j"}, false, false, "@smx_j 看一下", chxJudge + "这一轮没有人可问。\n", 1},
	{"summary_and_member", []string{"smx_a", "smx_s"}, false, false, "@smx_s @smx_a 看一下", chxOneA, 2},
	{"stray_display", []string{"smx_a", "smx_b"}, false, false, "@甲 看一下", chxStray, 0},
	{"stray_fullwidth", []string{"smx_a", "smx_b"}, false, false, string(rune(0xFF20)) + "smx_b 看一下", chxStray, 0},
	{"stray_space", []string{"smx_a", "smx_b"}, false, false, "@ smx_b 看一下", chxStray, 0},
	{"stray_bad_id", []string{"smx_a", "smx_b"}, false, false, "@-smx_b 看一下", chxStray, 0},
	{"stray_no_members", nil, false, false, "@甲 看一下", chxStray, 0},
	{"email", []string{"smx_a", "smx_b"}, true, false, "发到 a@b.com 就行", smxS1Want, 2},
	{"email_fullwidth", []string{"smx_a", "smx_b"}, true, false, "发到 a" + string(rune(0xFF20)) + "b.com 就行", smxS1Want, 2},
	{"trailing_dot", []string{"smx_a", "smx_b"}, false, false, "看一下 @smx_b.", chxOneB, 2},
	{"glued_at", []string{"smx_a", "smx_b"}, true, false, "hi@smx_b 看一下", smxS1Want, 2},
	{"partly", []string{"smx_a", "smx_b"}, false, false, "@甲 @smx_b 看一下", chxPartly + chxOneB, 2},
	{"partly_none", []string{"smx_a", "smx_b"}, false, false, "@甲 @ghost 看一下", "ghost 不是当前会话的成员\n" + chxPartly + chxNone, 0},
	{"mid_space_display", []string{"smx_a", "smx_b"}, false, false, "看一下 @甲", chxStray, 0},
	{"mid_space_fullwidth", []string{"smx_a", "smx_b"}, false, false, "看一下 " + string(rune(0xFF20)) + "smx_b", chxStray, 0},
	{"mid_cjk_display", []string{"smx_a", "smx_b"}, false, false, "请@甲看一下", chxStray, 0},
	{"line2_display", []string{"smx_a", "smx_b"}, false, false, "第一行\n@甲 看一下", chxStray, 0},
	{"line2_fullwidth", []string{"smx_a", "smx_b"}, false, false, "第一行\n" + string(rune(0xFF20)) + "smx_b 看一下", chxStray, 0},
	{"line2_mention", []string{"smx_a", "smx_b"}, false, false, "第一行\n@smx_b 看一下", chxOneB, 2},
	{"stray_after_mention", []string{"smx_a", "smx_b"}, false, false, "@smx_b @甲 看一下", chxPartly + chxOneB, 2},
	{"glued_left_cjk_tail", []string{"smx_a", "smx_b"}, true, false, "发到 x@甲 就行", smxS1Want, 2},
	{"ps_hashtable", []string{"smx_a", "smx_b"}, true, false, "$h = @{ Name = 'x' } 这行为什么报错", smxS1Want, 2},
	{"sql_identity", []string{"smx_a", "smx_b"}, true, false, "SELECT @@IDENTITY 返回空", smxS1Want, 2},
	{"at_everyone_cn", []string{"smx_a", "smx_b"}, true, false, "@所有人 看一下", smxS1Want, 2},
	{"glued_fullwidth", []string{"smx_a", "smx_b"}, true, false, "hi" + string(rune(0xFF20)) + "smx_b 看一下", smxS1Want, 2},
	{"double_at", []string{"smx_a", "smx_b"}, false, false, "@@smx_b 看一下", chxStray, 0},
	{"backtick_at", []string{"smx_a", "smx_b"}, true, false, "`@` 这个符号怎么打", smxS1Want, 2},
	{"ps_array", []string{"smx_a", "smx_b"}, true, false, "$a = @() 然后 $a += 1", smxS1Want, 2},
	{"stray_eot", []string{"smx_a", "smx_b"}, true, false, "你看 @", smxS1Want, 2},
	{"stray_newline", []string{"smx_a", "smx_b"}, true, false, "第一行 @\n第二行", smxS1Want, 2},
	{"stray_crlf", []string{"smx_a", "smx_b"}, true, false, "第一行 @\r\n第二行", smxS1Want, 2},
	{"stray_tab", []string{"smx_a", "smx_b"}, false, false, "@\tsmx_b 看一下", chxStray, 0},
	{"stray_vtab", []string{"smx_a", "smx_b"}, false, false, "@\vsmx_b 看一下", chxStray, 0},
	{"stray_formfeed", []string{"smx_a", "smx_b"}, false, false, "@\fsmx_b 看一下", chxStray, 0},
	{"price_at", []string{"smx_a", "smx_b"}, true, false, "价格 @ 5 元", smxS1Want, 2},
	{"angle_template", []string{"smx_a", "smx_b"}, false, false, "@<smx_b> 看一下", chxStray, 0},
	{"partly_ascii", []string{"smx_a", "smx_b"}, false, false, "@smx_b @(smx_a) 看一下", chxPartly + chxOneB, 2},
	{"numpy_matmul", []string{"smx_a", "smx_b"}, true, false, "y = A @ x + b", smxS1Want, 2},
	{"bash_dollar_at", []string{"smx_a", "smx_b"}, true, false, "echo $@", smxS1Want, 2},
	{"ps_herestring_close", []string{"smx_a", "smx_b"}, true, false, "$s = @'\nhello\n'@", smxS1Want, 2},
	{"newline_cut", []string{"smx_a", "smx_b"}, true, false, "@\nsmx_b 看一下", smxS1Want, 2},
	{"cr_cut", []string{"smx_a", "smx_b"}, true, false, "@\rsmx_b 看一下", smxS1Want, 2},
	{"paren_id", []string{"smx_a", "smx_b"}, false, false, "@(smx_b) 看一下", chxStray, 0},
	{"backtick_id", []string{"smx_a", "smx_b"}, false, false, "@`smx_b` 看一下", chxStray, 0},
	{"bold_id", []string{"smx_a", "smx_b"}, false, false, "@**smx_b** 看一下", chxStray, 0},
	{"space_display", []string{"smx_a", "smx_b"}, false, false, "@ 甲 看一下", chxStray, 0},
	{"config_only", []string{"smx_a", "smx_b"}, false, false, "@ smx_x 看一下", chxStray, 0},
	{"session_only", []string{"smx_a", "smx_b", "smx_q"}, false, false, "@ smx_q 看一下", chxStray, 0},
	{"space_dot", []string{"smx_a", "smx_b"}, false, false, "看一下 @ smx_b.", chxStray, 0},
	{"space_upper", []string{"smx_a", "smx_b"}, false, false, "@ SMX_B 看一下", chxStray, 0},
	{"fullwidth_everyone", []string{"smx_a", "smx_b"}, false, false, string(rune(0xFF20)) + "所有人 看一下", chxStray, 0},
	{"skip_then_display", []string{"smx_a", "smx_b"}, false, false, "价格 @ 5 元 @甲 看一下", chxStray, 0},
	{"fw_paren_id", []string{"smx_a", "smx_b"}, false, false, "@（smx_b）看一下", chxStray, 0},
	{"ideo_space_id", []string{"smx_a", "smx_b"}, false, false, "@" + string(rune(0x3000)) + "smx_b 看一下", chxStray, 0},
	{"zwsp_id", []string{"smx_a", "smx_b"}, false, false, "@" + string(rune(0x200B)) + "smx_b 看一下", chxStray, 0},
	{"fw_angle_id", []string{"smx_a", "smx_b"}, false, false, "@＜smx_b＞ 看一下", chxStray, 0},
	{"emoji_vs_id", []string{"smx_a", "smx_b"}, false, false, "@" + string(rune(0x2764)) + string(rune(0xFE0F)) + "smx_b 看一下", chxStray, 0},
	{"ideo_space_display", []string{"smx_a", "smx_b"}, false, false, "@" + string(rune(0x3000)) + "甲 看一下", chxStray, 0},
	{"partly_fullwidth", []string{"smx_a", "smx_b"}, false, false, "@smx_b @（smx_a）看一下", chxPartly + chxOneB, 2},
	{"ls_cut", []string{"smx_a", "smx_b"}, true, false, "@" + string(rune(0x2028)) + "smx_b 看一下", smxS1Want, 2},
	{"display_case", []string{"smx_a", "smx_b"}, false, false, "@ 坏 x 看一下", chxStray, 0},
}

// TestChxAutoRoster 是 C3：名单全集（表驱动）。
//
// 【判别力】每行断言整串、时钟次数、最后一条 user 消息 == 输入；时钟为 0 的行另断言四个成员都没有消息。
// 「有 mention 但交集为空」三种成因（not_member、left、summary_mentioned，外加 only_summary_mentioned）与
// 「没有 mention」（all、left_all、summary_plus_one）、「没有可问的成员」（no_members、only_summary）两边都有；
// left 与 left_all 只差有没有 mention；only_summary 与 only_summary_mentioned 只差 mention（M13、M13b）；
// judge_only 钉 judge 照 PlanRound 进 cmdRound、时钟 1 次。杀 M10、M11、M12、M13、M13b、M26、M30、M32。
// 没认出的 @（STRAY / PARTLY 各行）只在 hasStrayAt 生效时成立（M36），正对照是同装置写成 @smx_b 的 subset 与没有 @ 的 all；
// 各行按契约 C3 表后说明分别杀 M37-M39、M43、M44、M50-M55、M53b、M65-M87 的点名子测试。
// slowA 的行都期望 smxS1Want，先用 smxRequireBBeforeA 做前提守卫（两人先后固定）。
// 缺席一侧（时钟 0、四个成员都没有消息）的正对照是同表时钟 2 的行（同装置、只差输入）。
func TestChxAutoRoster(t *testing.T) {
	if len(chxRosterRows) != 76 {
		t.Fatalf("C3 表应当有 76 行，实际 %d 行", len(chxRosterRows))
	}
	for _, tc := range chxRosterRows {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := chxAuto(t, tc.members, true, tc.slowA)
			if tc.leaveB {
				if err := g.svc.Leave(g.ctx, g.convID, "smx_b", time.Now().UTC()); err != nil {
					t.Fatalf("前提不成立：smx_b 退出失败: %v", err)
				}
			}
			g.submit(t, tc.input)
			raw := g.buf.String()
			if tc.slowA {
				chxGuardSlowA(t, raw)
			}
			smxEq(t, "输出", smxNormalize(raw), tc.want)
			smxClockIs(t, g, tc.clock)
			users := chxUserTexts(t, g)
			if len(users) == 0 || users[len(users)-1] != tc.input {
				t.Fatalf("库里最后一条 user 消息应当是输入 %q，实际 %q", tc.input, users)
			}
			if tc.clock == 0 {
				chxRequireNoMemberMsgs(t, g)
			}
		})
	}
}

// ─── C4 ─────────────────────────────────────────────────────────────

// TestChxAutoOnlyBareText 是 C4：只有裸文本触发自动一轮。
//
// 【判别力】bare 是其余五行的正对照（同装置、只差命令或开关）；bare_manual 在 auto := bare（M6）下红；
// say 在 cmdSay 不看 args.bare（M5）下红；round_empty 钉「/round 名单为空照旧拒绝」（最后一条 user 消息是 seed-msg，
// 没有 mention），时钟 1 次是 cmdRound 的 start。to、ask 钉 /to、/ask <id> 文本 之后没有自动一轮。杀 M5、M6。
func TestChxAutoOnlyBareText(t *testing.T) {
	cases := []struct {
		name  string
		auto  bool
		input string
		want  string
		clock int
	}{
		{"bare", true, "你好", chxOneA, 2},
		{"bare_manual", false, "你好", "", 0},
		{"say", true, "/say 你好", "", 0},
		{"to", true, "/to smx_a 你好", "只有 smx_a 会看到这条；其余成员永远不会 —— 游标只前进\n", 0},
		{"ask", true, "/ask smx_a 你好", "正在问 smx_a ...\n甲（T）: OK\n", 0},
		{"round_empty", true, "/round --go", "拒绝: 名单为空。给了 id 才用给的；没给则取当前会话最后一条用户发言里的 mention 与当前活跃成员的交集。不会默认全员。\n", 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := smxSetup(t, "smx_s", []string{"smx_a"}, true, false)
			g.r.SetAutoReply(tc.auto)
			g.submit(t, tc.input)
			smxEq(t, "输出", smxNormalize(g.buf.String()), tc.want)
			smxClockIs(t, g, tc.clock)
		})
	}
}

// ─── C5 ─────────────────────────────────────────────────────────────

// TestChxAutoVerboseMentionLine 是 C5：详细档「点名了」门的两侧，以及「先落库、再问一轮」。
//
// 【判别力】auto 与 manual 只差开关：自动时不打「点名了」（M8 下 auto 红），手动时照打（M9 下 manual 红）；
// auto_say 与 auto 只差命令（/say 走 sayText 的 bare=false 一侧，M5 下红）；auto_to 是 postUserText 包装层那一侧，
// 喂的是会打出「点名了」的输入，包装层把 auto 传成 true（M27）时这一行消失，红。auto 的「恰好 1 行以
// "  scanned_to=2" 结尾」是先落库再问一轮的直接断言：自动一轮挪到 postUser 之前（M31）时这一行是 scanned_to=1、
// 「已记下」变成 seq 6 且排到最后，红。auto_not_member 钉 NONE 的整串（含 §3.2 第 1 句的出路）。
// 「没有任何一行以 点名了 开头」是缺席断言，同装置的正对照是 manual。杀 M8、M9、M27、M5、M31。
func TestChxAutoVerboseMentionLine(t *testing.T) {
	t.Run("auto", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a"}, false, false)
		g.submit(t, "@smx_a 你好")
		lines := smxLines(g.buf.String())
		if len(lines) < 2 || lines[0] != "已记下 seq 2" || lines[1] != "发言顺序按字典序，重复的 id 已去重，只问一次。" {
			t.Fatalf("前两行应当是「已记下 seq 2」与「发言顺序按字典序……」，实际输出:\n%s", g.buf.String())
		}
		for _, l := range lines {
			if strings.HasPrefix(l, "点名了") {
				t.Fatalf("自动回复时不应当打「点名了」，实际有一行 %q；输出:\n%s", l, g.buf.String())
			}
		}
		if n := smxCount(lines, "本轮问 1 个: smx_a"); n != 1 {
			t.Fatalf("整行「本轮问 1 个: smx_a」应当恰好 1 次，实际 %d 次；输出:\n%s", n, g.buf.String())
		}
		if n := smxCount(lines, "outcome spoke  agent smx_a"); n != 1 {
			t.Fatalf("整行「outcome spoke  agent smx_a」应当恰好 1 次，实际 %d 次；输出:\n%s", n, g.buf.String())
		}
		if n := chxOneSuffix(lines, "  scanned_to=2"); n != 1 {
			t.Fatalf("应当恰好 1 行以「  scanned_to=2」结尾，实际 %d 行；输出:\n%s", n, g.buf.String())
		}
	})
	t.Run("auto_not_member", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a"}, false, false)
		g.submit(t, "@ghost 你好")
		smxEq(t, "输出", g.buf.String(), "已记下 seq 2\nghost 不是当前会话的成员\n"+chxNone)
	})
	const manualWant = "已记下 seq 2\n点名了 smx_a（用 /ask smx_a 让他说话）\n"
	t.Run("manual", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a"}, false, false)
		g.submit(t, "@smx_a 你好")
		smxEq(t, "输出", g.buf.String(), manualWant)
	})
	t.Run("auto_say", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a"}, false, false)
		g.submit(t, "/say @smx_a 你好")
		smxEq(t, "输出", g.buf.String(), manualWant)
	})
	t.Run("auto_to", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a"}, false, false)
		g.submit(t, "/to smx_a @smx_a 你好")
		smxEq(t, "输出", g.buf.String(), "已记下 seq 2\n只有 smx_a 会看到这条；其余成员永远不会 —— 游标只前进\n点名了 smx_a（用 /ask smx_a 让他说话）\n")
	})
}

// ─── C6 ─────────────────────────────────────────────────────────────

// TestChxAutoLineModePaths 是 C6：行式 Run 的三条裸文本路径（单行、多行草稿 /end、括号粘贴）与 /say。
//
// 【判别力】三条路径各自自动 / 手动两侧只差开关；say_auto 与 single_auto 只差「/say 」。
// single_auto 在 dispatch 不置 bare（M1）下红，draft_auto 在草稿发送走 sayText(…, false)（M4）下红，
// paste_auto 在 finishPaste 不置 bare（M3）下红，say_auto 在 M5 下红，*_manual 在 M6 下红。
// 提示符的个数与位置也在整串里：ROUND 夹在哪两个 council> 之间，钉它是哪一行输入引出来的。杀 M1、M3、M4、M5、M6。
func TestChxAutoLineModePaths(t *testing.T) {
	const (
		round = chxOneA
		enter = "进入多行输入：整段粘贴或逐行敲；单独一行 /end 发送，单独一行 /cancel 放弃\n"
		d3    = "   ...>    ...>    ...> "
	)
	draft := "/say\n第一行\n第二行\n/end\n"
	paste := "\x1b[200~第一行\n第二行\x1b[201~\n"
	cases := []struct {
		name   string
		auto   bool
		script string
		want   string
		body   string
		clock  int
	}{
		{"single_auto", true, "你好\n", "council> " + round + "council> \n再见\n", "你好", 2},
		{"single_manual", false, "你好\n", "council> council> \n再见\n", "你好", 0},
		{"draft_auto", true, draft, "council> " + enter + d3 + round + "council> \n再见\n", "第一行\n第二行", 2},
		{"draft_manual", false, draft, "council> " + enter + d3 + "council> \n再见\n", "第一行\n第二行", 0},
		{"paste_auto", true, paste, "council> council> " + round + "council> \n再见\n", "第一行\n第二行", 2},
		{"paste_manual", false, paste, "council> council> council> \n再见\n", "第一行\n第二行", 0},
		{"say_auto", true, "/say 你好\n", "council> council> \n再见\n", "你好", 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := smxSetup(t, "smx_s", []string{"smx_a"}, true, false)
			g.r.SetAutoReply(tc.auto)
			g.r.in = strings.NewReader(tc.script)
			if err := g.r.Run(g.ctx); err != nil {
				t.Fatalf("Run 返回 %v；输出:\n%s", err, g.buf.String())
			}
			smxEq(t, "整个 Run 的输出", smxNormalize(g.buf.String()), tc.want)
			smxClockIs(t, g, tc.clock)
			chxRequireUsers(t, g, "seed-msg", tc.body)
		})
	}
}

// ─── C7 ─────────────────────────────────────────────────────────────

// TestChxAutoPostFailNoRound 是 C7：Post 失败时一个进程都不起。
//
// 【判别力】Post 失败时 postUser 返回 (false, nil)；sayText 不看 posted（M7）时，取消的 ctx 让 autoReplyRound
// 的读成员表失败，多打一行「round: …」，「恰好 1 行」为假。live 是同装置、只差 ctx 的正对照。杀 M7。
func TestChxAutoPostFailNoRound(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a"}, true, false)
		ctx2, cancel := context.WithCancel(g.ctx)
		cancel()
		if err := g.r.Submit(ctx2, "你好"); err != nil {
			t.Fatalf("Submit 返回 %v，期望 nil", err)
		}
		lines := smxLines(g.buf.String())
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "发言失败: ") {
			t.Fatalf("输出应当恰好 1 行且以「发言失败: 」开头，实际:\n%s", g.buf.String())
		}
		smxClockIs(t, g, 0)
		chxRequireUsers(t, g, "seed-msg")
	})
	t.Run("live", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a"}, true, false)
		g.submit(t, "你好")
		smxEq(t, "输出", smxNormalize(g.buf.String()), chxOneA)
	})
}

// ─── C8 ─────────────────────────────────────────────────────────────

// TestChxAutoNoRunner 是 C8：rn 为 nil 时由 needsRunner 答 msgNoRunner，消息照常记下，不空指针。
//
// 【判别力】自动一轮经 runCommand(roundCmd) 才有 needsRunner 这道闸；直接调 cmdRound（M15）或 roundCmd 不要
// Runner（M16）时空指针 panic，整个测试二进制红。manual 是只差开关的对照（没有自动一轮就没有这一行）。杀 M15、M16。
func TestChxAutoNoRunner(t *testing.T) {
	for _, tc := range []struct {
		name string
		auto bool
		want string
	}{
		{"auto", true, "provider 调用未就绪，本命令不可用\n"},
		{"manual", false, ""},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := smxSetup(t, "smx_s", []string{"smx_a"}, true, false)
			b := &rbaBuf{}
			r := New(smxCfg("smx_s"), g.svc, nil, nil, b, RulesContext{})
			r.SetConcise(true)
			r.SetAutoReply(tc.auto)
			if err := r.Submit(g.ctx, "/use "+g.convID); err != nil {
				t.Fatalf("前提不成立：/use 返回 %v", err)
			}
			b.Reset()
			if err := r.Submit(g.ctx, "你好"); err != nil {
				t.Fatalf("Submit 返回 %v，期望 nil", err)
			}
			smxEq(t, "输出", b.String(), tc.want)
			chxRequireUsers(t, g, "seed-msg", "你好")
		})
	}
}

// ─── C9 ─────────────────────────────────────────────────────────────

// chxHelpLines 是 C9 的 /help 四行（由脚本从契约逐字生成；命令名之后的空白是 tabwriter 补到第 12 列的）。
var chxHelpLines = []string{
	"/auto       切换直接打字要不要让成员回复: /auto 切换；/auto on 自动回复（点名了谁就只问谁，没点名问全部成员）；/auto off 只记下",
	"/say        只记下、不让任何人回复: /say <文本>；/say /foo 发字面量 /foo；/say 不带参数进多行输入（/end 发送时与直接打字相同）",
	"/session    列出当前会话每个成员的 provider 会话；/session rebuild <agent-id> 先结清这个成员留在库里的陈旧在飞记录，再丢弃并重建",
	"/round      按名单问一轮，--go 时发言者同时起进程；多个成员跑完自动总结（只给你看）: /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]",
}

// TestChxAutoCommand 是 C9：/auto 的切换、设定、用法，/help 的三行新说法与 /round 那一行不变，打开句按配置选。
//
// 【判别力】第 4、7 步钉 on / off 是设定不是切换（M21 下第 2 步红，M21b 下第 6 步红）；第 5、8、9 步钉用法不改状态
// （第 5 步在 true 上、第 8、9 步在 false 上，两侧都有）；/round 那一行钉 roundCmd 的 summary 与 HEAD 逐字相同。
// summary 与 no_summary 只差配置里的 [summary]：无条件打第 1 句（M49）时 no_summary 与 steps 红，
// 无条件打第 2 句（M49b）时 summary 红。杀 M21、M21b、M49、M49b。
func TestChxAutoCommand(t *testing.T) {
	const (
		on    = chxAutoOnNoSummary + "\n"
		off   = chxAutoOff + "\n"
		usage = chxAutoUsage + "\n"
	)
	t.Run("steps", func(t *testing.T) {
		ctx := context.Background()
		b := &rbaBuf{}
		r := New(nil, nil, nil, nil, b, RulesContext{})
		steps := []struct {
			cmd   string
			want  string
			after bool
		}{
			{"/auto", on, true},
			{"/auto", off, false},
			{"/auto on", on, true},
			{"/auto on", on, true},
			{"/auto x", usage, true},
			{"/auto off", off, false},
			{"/auto off", off, false},
			{"/auto on off", usage, false},
			{"/auto ON", usage, false},
		}
		for i, s := range steps {
			b.Reset()
			if err := r.Submit(ctx, s.cmd); err != nil {
				t.Fatalf("第 %d 步 %s 返回 %v", i+1, s.cmd, err)
			}
			if got := b.String(); got != s.want {
				t.Fatalf("第 %d 步 %s 的输出不对\n实际: %q\n期望: %q", i+1, s.cmd, got, s.want)
			}
			if r.autoReply != s.after {
				t.Fatalf("第 %d 步 %s 之后 autoReply 应当是 %v，实际 %v", i+1, s.cmd, s.after, r.autoReply)
			}
		}
		b.Reset()
		if err := r.Submit(ctx, "/help"); err != nil {
			t.Fatalf("/help 返回 %v", err)
		}
		lines := smxLines(b.String())
		for _, want := range chxHelpLines {
			if n := smxCount(lines, want); n != 1 {
				t.Fatalf("/help 里整行 %q 应当恰好 1 次，实际 %d 次；/help:\n%s", want, n, b.String())
			}
		}
	})
	for _, tc := range []struct {
		name string
		sum  string
		on   string
	}{
		{"summary", "smx_s", chxAutoOn + "\n"},
		{"no_summary", "", chxAutoOnNoSummary + "\n"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			b := &rbaBuf{}
			r := New(smxCfg(tc.sum), nil, nil, nil, b, RulesContext{})
			for i, s := range []struct{ cmd, want string }{{"/auto on", tc.on}, {"/auto off", off}, {"/auto", tc.on}} {
				b.Reset()
				if err := r.Submit(ctx, s.cmd); err != nil {
					t.Fatalf("第 %d 步 %s 返回 %v", i+1, s.cmd, err)
				}
				if got := b.String(); got != s.want {
					t.Fatalf("第 %d 步 %s 的输出不对\n实际: %q\n期望: %q", i+1, s.cmd, got, s.want)
				}
			}
		})
	}
}

// ─── C10 ────────────────────────────────────────────────────────────

// C10 的两张候选表与 TAIL（由脚本从契约逐字生成）。
const (
	chxWithSum = "NO  AGENT  PROVIDER  DISPLAY_NAME\n" +
		"1   smx_a  claude    甲\n" +
		"2   smx_b  claude    -\n" +
		"3   smx_j  claude    裁判 J\n" +
		"4   smx_k  grok      坏 K\n" +
		"5   smx_x  grok      坏 X\n" +
		"6   smx_y  grok      坏 Y\n" +
		"7   smx_z  grok      坏 Z\n"
	chxNoSum = "NO  AGENT  PROVIDER  DISPLAY_NAME\n" +
		"1   smx_a  claude    甲\n" +
		"2   smx_b  claude    -\n" +
		"3   smx_j  claude    裁判 J\n" +
		"4   smx_k  grok      坏 K\n" +
		"5   smx_s  claude    总结 S\n" +
		"6   smx_x  grok      坏 X\n" +
		"7   smx_y  grok      坏 Y\n" +
		"8   smx_z  grok      坏 Z\n"
	chxAddTail = "用 /add <编号|全名|前缀> 加入，多个用空格或逗号隔开，例如 /add 1,2\n"
)

// chxAddRig 是 C10 的装置：真库、没有 Runner、cfg.Providers 置空（不识别模型、不打识别表）。
type chxAddRig struct {
	ctx    context.Context
	svc    *conversation.Service
	r      *REPL
	b      *rbaBuf
	convID string
}

func chxNewAddRig(t *testing.T, sum string) *chxAddRig {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	svc := newRoundStreamService(t)
	conv, err := svc.Create(ctx, "chx")
	if err != nil {
		t.Fatalf("装置：建会话失败: %v", err)
	}
	cfg := smxCfg(sum)
	cfg.Providers = nil
	b := &rbaBuf{}
	r := New(cfg, svc, nil, nil, b, RulesContext{})
	if err := r.Submit(ctx, "/use "+conv.ID); err != nil {
		t.Fatalf("装置：/use 失败: %v", err)
	}
	b.Reset()
	return &chxAddRig{ctx: ctx, svc: svc, r: r, b: b, convID: conv.ID}
}

// run 清空缓冲、提交一条命令，返回把会话 id 换成 <C> 之后的输出。
func (a *chxAddRig) run(t *testing.T, cmd string) string {
	t.Helper()
	a.b.Reset()
	if err := a.r.Submit(a.ctx, cmd); err != nil {
		t.Fatalf("%s 返回 %v", cmd, err)
	}
	return strings.ReplaceAll(a.b.String(), a.convID, "<C>")
}

// active 返回活跃成员 id。
func (a *chxAddRig) active(t *testing.T) []string {
	t.Helper()
	ps, err := a.svc.Participants(a.ctx, a.convID, false)
	if err != nil {
		t.Fatalf("读成员表失败: %v", err)
	}
	var ids []string
	for _, p := range ps {
		ids = append(ids, p.AgentID)
	}
	return ids
}

func (a *chxAddRig) join(t *testing.T, id string) {
	t.Helper()
	if err := a.svc.Join(a.ctx, a.convID, conversation.Participant{AgentID: id, Provider: "claude", Active: true, JoinedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("装置：加入 %s 失败: %v", id, err)
	}
}

// TestChxAddHidesSummary 是 C10：/add 的候选藏掉总结成员，显式 /add 总结成员拒绝，已退出的放行。
//
// 【判别力】list / number 两侧只差配置（M22 下 sum="smx_s" 一侧表里多出 smx_s、编号 5 变成 smx_s，红）；
// explicit 三条经同一个 resolveAddSegment（M23 下三条都加入了，红），sum="" 是只差配置的正对照；
// active、left_rejoin、explicit 覆盖放行条件 member && !active 的全部三种取值（没加入过、在场、已退出）：
// M24（不放行已退出）下 left_rejoin 红，M25（放行在场）下 active 红。杀 M22、M23、M24、M25。
func TestChxAddHidesSummary(t *testing.T) {
	rej := fmt.Sprintf(chxRejFmt, "smx_s") + "\n"
	t.Run("list", func(t *testing.T) {
		smxEq(t, "sum=smx_s 的 /add", chxNewAddRig(t, "smx_s").run(t, "/add"), chxWithSum+chxAddTail)
		smxEq(t, "sum=\"\" 的 /add", chxNewAddRig(t, "").run(t, "/add"), chxNoSum+chxAddTail)
	})
	t.Run("number", func(t *testing.T) {
		smxEq(t, "sum=smx_s 的 /add 5", chxNewAddRig(t, "smx_s").run(t, "/add 5"), "smx_x 已加入会话 <C>\n")
		smxEq(t, "sum=\"\" 的 /add 5", chxNewAddRig(t, "").run(t, "/add 5"), "smx_s 已加入会话 <C>\n")
	})
	t.Run("explicit", func(t *testing.T) {
		for _, cmd := range []string{"/add smx_s", "/join smx_s", "/add smx_a,smx_s"} {
			a := chxNewAddRig(t, "smx_s")
			smxEq(t, "sum=smx_s 的 "+cmd, a.run(t, cmd), rej)
			if ids := a.active(t); len(ids) != 0 {
				t.Fatalf("%s 被拒之后活跃成员应当为空（全或无），实际 %q", cmd, ids)
			}
			c := chxNewAddRig(t, "")
			if got := c.run(t, cmd); !strings.Contains(got, "smx_s 已加入会话 <C>\n") {
				t.Fatalf("对照：sum=\"\" 时 %s 应当加入 smx_s，实际 %q", cmd, got)
			}
		}
	})
	t.Run("active", func(t *testing.T) {
		a := chxNewAddRig(t, "smx_s")
		a.join(t, "smx_s")
		smxEq(t, "在场时的 /add smx_s", a.run(t, "/add smx_s"), "smx_s 是总结成员（config.toml 的 [summary]），已经在当前会话里了\n")
	})
	t.Run("left_rejoin", func(t *testing.T) {
		a := chxNewAddRig(t, "smx_s")
		a.join(t, "smx_s")
		if err := a.svc.Leave(a.ctx, a.convID, "smx_s", time.Now().UTC()); err != nil {
			t.Fatalf("装置：smx_s 退出失败: %v", err)
		}
		if ids := a.active(t); len(ids) != 0 {
			t.Fatalf("前提不成立：退出之后活跃成员应当为空，实际 %q", ids)
		}
		smxEq(t, "已退出时的 /join smx_s", a.run(t, "/join smx_s"), "smx_s 已加入会话 <C>\n")
		if ids := a.active(t); len(ids) != 1 || ids[0] != "smx_s" {
			t.Fatalf("/join 之后活跃成员应当恰好是 [smx_s]，实际 %q", ids)
		}
		smxEq(t, "重新加入之后的 /add", a.run(t, "/add"), chxWithSum+chxAddTail)
	})
}

// ─── C11 ────────────────────────────────────────────────────────────

// chxFailWriter 每次 Write 都返回 (0, errChxWrite)。
type chxFailWriter struct{}

func (chxFailWriter) Write([]byte) (int, error) { return 0, errChxWrite }

// TestChxAutoWriteFailure 是 C11：写屏幕失败时错误原样并进返回值，且 Post 之后的写失败不起进程。
//
// 【判别力】详细档下 postUser 在「已记下 seq N」处写失败、返回 (true, err)：bare 钉 sayText 必须先看 err
// （M7b 下带着失败去问一轮，时钟被调用、成员有消息，红）；to 钉包装层把 err 原样交出（M28 下返回 nil，红）。
// 简洁档落库不写屏幕，写失败发生在自动一轮里面：bare_concise 走到 cmdRound，start 调一次时钟、随后「本轮问 1 个」写失败，
// 时钟恰好 1 次，这一条同时是前提守卫；它杀 M33、M34（包装层吞错误，返回 nil）。
// no_members_concise、summary_mentioned_concise、stray_concise 分别杀 M47、M48、M46；partly_concise 杀 M45
// （吞掉之后照问一轮，时钟变成 1）。bare_concise 与 bare 只差档位，互为对照。
func TestChxAutoWriteFailure(t *testing.T) {
	cases := []struct {
		name    string
		members []string
		concise bool
		input   string
		body    string
		clock   int
	}{
		{"bare", []string{"smx_a"}, false, "你好", "你好", 0},
		{"say", []string{"smx_a"}, false, "/say 你好", "你好", 0},
		{"to", []string{"smx_a"}, false, "/to smx_a 你好", "你好", 0},
		{"bare_concise", []string{"smx_a"}, true, "你好", "你好", 1},
		{"no_members_concise", nil, true, "你好", "你好", 0},
		{"summary_mentioned_concise", []string{"smx_a", "smx_s"}, true, "@smx_s 你好", "@smx_s 你好", 0},
		{"stray_concise", []string{"smx_a"}, true, "@甲 你好", "@甲 你好", 0},
		{"partly_concise", []string{"smx_a"}, true, "@甲 @smx_a 你好", "@甲 @smx_a 你好", 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := smxSetup(t, "smx_s", tc.members, tc.concise, false)
			g.r.SetAutoReply(true)
			g.r.out = chxFailWriter{}
			err := g.r.Submit(g.ctx, tc.input)
			if !errors.Is(err, errChxWrite) {
				t.Fatalf("Submit 应当返回 errChxWrite，实际 %v", err)
			}
			smxClockIs(t, g, tc.clock)
			chxRequireNoMemberMsgs(t, g)
			chxRequireUsers(t, g, "seed-msg", tc.body)
		})
	}
}

// ─── C12 ────────────────────────────────────────────────────────────

// chxCancelOnceWriter 第一次写入照常收下（记下内容），内容以「已记下 seq 」开头时在写入里调 cancel；
// 之后每次写都返回 (0, errChxWrite)。
type chxCancelOnceWriter struct {
	mu     sync.Mutex
	n      int
	first  string
	cancel context.CancelFunc
}

func (w *chxCancelOnceWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.n++
	if w.n > 1 {
		w.mu.Unlock()
		return 0, errChxWrite
	}
	w.first = string(p)
	w.mu.Unlock()
	if strings.HasPrefix(string(p), "已记下 seq ") {
		w.cancel()
	}
	return len(p), nil
}

func (w *chxCancelOnceWriter) firstWrite() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.first
}

var chxRoundCanceledRe = regexp.MustCompile(`^round: .*: context canceled$`)

// TestChxAutoParticipantsFailAfterPost 是 C12：Post 已成功、随后读成员表失败。
//
// 【判别力】钩子在「已记下 seq 」那次写入里取消 ctx，这时 Post 已成功；autoReplyRound 的 Participants 必然失败，
// 走 reportErr("round", err)。那一支静默返回 nil（M35）时两个子测试都只剩第 1 行，红；把 parts 置空（M35b）时
// 第 2 行变成 EMPTY（plain）或 NONE（mention），红；postUser 读成员表那一支返回 false（M35c）时 mention 只剩第 1 行，
// plain 照绿（它不走那一支），所以两个子测试缺一不可。write_fail 与 plain 只差写者：reportErr 的写屏幕错误写成
// 「_ = …; return nil」（M35d）时 Submit 返回 nil，红。mention 的正对照是 C5 auto（同装置、同输入、不取消）。
// 杀 M35、M35b、M35c、M35d。
func TestChxAutoParticipantsFailAfterPost(t *testing.T) {
	for _, tc := range []struct{ name, input string }{{"plain", "你好"}, {"mention", "@smx_a 你好"}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := chxAuto(t, []string{"smx_a"}, false, false)
			ctx, cancel := context.WithCancel(g.ctx)
			defer cancel()
			w := g.installHook("已记下 seq ", func() error { cancel(); return nil })
			if err := g.r.Submit(ctx, tc.input); err != nil {
				t.Fatalf("Submit 返回 %v，期望 nil", err)
			}
			requireFired(t, w, 1)
			lines := smxLines(w.String())
			if len(lines) != 2 || lines[0] != "已记下 seq 2" || !chxRoundCanceledRe.MatchString(lines[1]) {
				t.Fatalf("输出应当恰好两行：「已记下 seq 2」与「round: …: context canceled」，实际:\n%s", w.String())
			}
			smxClockIs(t, g, 0)
			chxRequireNoMemberMsgs(t, g)
			chxRequireUsers(t, g, "seed-msg", tc.input)
		})
	}
	t.Run("write_fail", func(t *testing.T) {
		g := chxAuto(t, []string{"smx_a"}, false, false)
		ctx, cancel := context.WithCancel(g.ctx)
		defer cancel()
		w := &chxCancelOnceWriter{cancel: cancel}
		g.r.out = w
		err := g.r.Submit(ctx, "你好")
		if got := w.firstWrite(); got != "已记下 seq 2\n" {
			t.Fatalf("前提不成立：第一次写入应当是「已记下 seq 2」，实际 %q", got)
		}
		if !errors.Is(err, errChxWrite) {
			t.Fatalf("Submit 应当返回 errChxWrite，实际 %v", err)
		}
		smxClockIs(t, g, 0)
		chxRequireNoMemberMsgs(t, g)
		chxRequireUsers(t, g, "seed-msg", "你好")
	})
}

// ─── C13 ────────────────────────────────────────────────────────────

// TestChxAutoStrayThenRound 是 C13：§3.2 第 3 句与第 1 句给的出路（接着敲 /round <id> --go）实测，成员读到的正是这一条。
//
// 【判别力】第 1 步是详细档下「先落库、再判名单」的直接断言：自动一轮挪到 postUser 之前（M31）时两个子测试的整串
// 都变成说明句在前、「已记下 seq 2」在后，红。stray 的全角 ＠ 在第 10 字节：只看第 0 字节（M50）、全角只认第 0 字节（M54）、
// 不认全角（M37）、全角也按成员判（M70）下这一步不打 STRAY、照问 smx_a，红；M36、M38 下同样红。
// none 在 M12、M13、M38 下红（第 1 步不打第 1 句）。第 2 步「恰好 1 行以 "  scanned_to=2" 结尾」：这条消息若没进库，
// 库里就没有 seq 2，这一行不会出现。杀 M31、M36、M37、M38、M50、M54、M70（stray），M12、M13、M31、M38（none）。
func TestChxAutoStrayThenRound(t *testing.T) {
	for _, tc := range []struct{ name, in, w1 string }{
		{"stray", "看一下 " + string(rune(0xFF20)) + "所有人", "已记下 seq 2\n" + chxStray},
		{"none", "npm i @types/node 报错", "已记下 seq 2\ntypes 不是当前会话的成员\n" + chxNone},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := chxAuto(t, []string{"smx_a"}, false, false)
			g.submit(t, tc.in)
			smxEq(t, "第 1 步的输出", g.buf.String(), tc.w1)
			smxClockIs(t, g, 0)
			g.requireNoMsgs(t, "smx_a")
			g.buf.Reset()
			g.submit(t, "/round smx_a --go")
			lines := smxLines(g.buf.String())
			if n := chxOneSuffix(lines, "  scanned_to=2"); n != 1 {
				t.Fatalf("第 2 步应当恰好 1 行以「  scanned_to=2」结尾，实际 %d 行；输出:\n%s", n, g.buf.String())
			}
			if n := smxCount(lines, "本轮问 1 个: smx_a"); n != 1 {
				t.Fatalf("第 2 步整行「本轮问 1 个: smx_a」应当恰好 1 次，实际 %d 次；输出:\n%s", n, g.buf.String())
			}
			if n := smxCount(lines, "outcome spoke  agent smx_a"); n != 1 {
				t.Fatalf("第 2 步整行「outcome spoke  agent smx_a」应当恰好 1 次，实际 %d 次；输出:\n%s", n, g.buf.String())
			}
			if ms := g.msgsFrom(t, "smx_a"); len(ms) != 1 {
				t.Fatalf("smx_a 的消息应当恰好 1 条，实际 %d 条", len(ms))
			}
			chxRequireUsers(t, g, "seed-msg", tc.in)
		})
	}
}

// ─── C14 ────────────────────────────────────────────────────────────

// chxNamesRows 是 C14 的全部 9 行（由脚本从契约 §2.1 C14 表逐字生成）。
var chxNamesRows = []struct {
	name  string
	names []string
	input string
	hit   bool
}{
	{"lead_fw_bracket", []string{"【测试】乙"}, "@【测试】乙 看一下", true},
	{"lead_fw_bracket_space", []string{"【测试】乙"}, "@ 【测试】乙 看一下", true},
	{"lead_ascii_paren", []string{"(bot) X"}, "@(bot) X 看一下", true},
	{"lead_ascii_paren_space", []string{"(bot) X"}, "@ (bot) X 看一下", true},
	{"lead_negative", []string{"(bot) X"}, "@(bot) 看一下", false},
	{"letters_stop", []string{"甲"}, "@所有人，smx_b 也看一下", false},
	{"letters_stop_control", []string{"甲"}, "@，smx_b 也看一下", true},
	{"display_case_hit", []string{"甲", "裁判 J", "总结 S", "坏 X", "坏 Y", "坏 Z", "坏 K"}, "@ 坏 x 看一下", true},
	{"display_case_miss", []string{"甲", "裁判 J", "总结 S", "坏 Y", "坏 Z", "坏 K"}, "@ 坏 x 看一下", false},
}

// TestChxHasStrayAtNames 是 C14：白盒直接调 hasStrayAt，钉「显示名在跳过途中每个位置都比」与「字母、汉字不跳」。
//
// 【判别力】lead_* 四行的显示名以全角方括号或 ASCII 括号开头：显示名只在跳完之后比（M84）或去掉显示名那一支（M66）时
// 四行都变成 false，红。lead_negative 是同一组 names 的反面，M65（跳过之后取得出任何一段就算）下变成 true，红。
// letters_stop 在 M74（非 ASCII 的字母、汉字也跳）下变成 true，红；letters_stop_control 是只差「所有人」三个字的正对照，
// M75、M77、M79 下红。display_case_hit 在 M85（逐字节比）、M66 下红；display_case_miss 是缺席断言，§4 没有让它变红的
// 变异（契约写明），它的正对照是只差一个名字的 display_case_hit。
func TestChxHasStrayAtNames(t *testing.T) {
	if len(chxNamesRows) != 9 {
		t.Fatalf("C14 表应当有 9 行，实际 %d 行", len(chxNamesRows))
	}
	known := map[string]bool{"smx_a": true, "smx_b": true}
	for _, tc := range chxNamesRows {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := hasStrayAt(tc.input, known, tc.names); got != tc.hit {
				t.Fatalf("hasStrayAt(%q, known, %q) = %v，期望 %v", tc.input, tc.names, got, tc.hit)
			}
		})
	}
}
