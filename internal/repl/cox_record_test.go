package repl

// CO1 §2.1 的第二个文件：K1、K3、K4、K5，简洁档下直接调 writeAskRecord，不起进程。
// 夹具与构造辅助来自 cox_record_verbose_test.go；本文件引用本刀新增的
// SetConcise、FormatWall、FormatElapsed、OutcomeWord，在 HEAD 上编译不过是预期的。

import (
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// coxNewConciseRecordREPL 在构造辅助之后切到简洁档。
func coxNewConciseRecordREPL() (*REPL, interface{ String() string }) {
	r, b := coxNewRecordREPL()
	r.SetConcise(true)
	return r, b
}

// TestCoxConciseRecordBlocks 是 K1：简洁档，8 个 Record 各一个子测试，整串相等。
//
// 【判别力】spoke_rich 喂的是「原值非空」的全套：会话行、用量、金额、冷暖、seq 都有料，
// 漏掉任何一个门就多出一行；spoke_empty 钉「空正文不留尾随空格、提示在头之后」；
// spoke_truncated 钉「提示在正文之后」与 60000 这个边界（<= 60000 会印 60.0s）；
// timeout / post_rejected 钉照打的四类行与失败行不带用时（WallMS 非零，带了会印（5m00s）/（1.2s））；
// 两个 fallback 钉「空 display_name」与「配置里没有」两种回落到 id。
// 对照是 K2（同一组 Record 的详细档，只差档位）。
func TestCoxConciseRecordBlocks(t *testing.T) {
	for _, tc := range coxRecordCases() {
		t.Run(tc.name, func(t *testing.T) {
			r, b := coxNewConciseRecordREPL()
			if err := writeAskRecord(r, tc.rec); err != nil {
				t.Fatalf("writeAskRecord 返回错误: %v", err)
			}
			if got := b.String(); got != tc.concise {
				t.Fatalf("简洁档输出不对\n实际: %q\n期望: %q", got, tc.concise)
			}
		})
	}
}

// TestCoxConciseFailureWordsAllOutcomes 是 K3：9 个非 Spoke 结局各喂一个 Message 为 nil 的 Record。
//
// 【判别力】结局值全集都走一遍（夹具要覆盖变异所在的取值全集）：失败行用英文 outcomeLabel、
// 某个词写错、或 OutcomeWord 漏一个 case 都红。WallMS 为 1234 非零，失败行若带用时会印（1.2s）。
// PostRejected 那一条另钉「DroppedText 为空时标题行照打」这一 HEAD 既有行为。
// 第二段直接断言 repl.OutcomeWord 对 10 个值（含 Spoke 的「完成」）逐一相等，
// 它是 TUI 看板与简洁档失败行共用的那一份。
func TestCoxConciseFailureWordsAllOutcomes(t *testing.T) {
	cases := []struct {
		o    turn.Outcome
		word string
	}{
		{turn.OutcomeUnknown, "未知"},
		{turn.OutcomePostRejected, "未入库"},
		{turn.OutcomeProviderFailed, "报错"},
		{turn.OutcomeUnparsable, "认不出"},
		{turn.OutcomeTimeout, "超时"},
		{turn.OutcomeSpawnFailed, "没起来"},
		{turn.OutcomeRefused, "拒绝"},
		{turn.OutcomeCanceled, "取消"},
		{turn.OutcomeLost, "丢失"},
	}
	for _, c := range cases {
		t.Run(c.word, func(t *testing.T) {
			r, b := coxNewConciseRecordREPL()
			if err := writeAskRecord(r, turn.Record{AgentID: "cox_a", Outcome: c.o, WallMS: 1234}); err != nil {
				t.Fatalf("writeAskRecord 返回错误: %v", err)
			}
			want := "甲 A: 没有回复（" + c.word + "）\n"
			if c.o == turn.OutcomePostRejected {
				want = "甲 A: 没有回复（未入库）\n正文写不进库（已经付过钱），整段如下:\n"
			}
			if got := b.String(); got != want {
				t.Fatalf("结局 %d 的失败行不对\n实际: %q\n期望: %q", c.o, got, want)
			}
		})
	}
	words := append(cases, struct {
		o    turn.Outcome
		word string
	}{turn.OutcomeSpoke, "完成"})
	for _, c := range words {
		if got := OutcomeWord(c.o); got != c.word {
			t.Errorf("OutcomeWord(%d) = %q，期望 %q", c.o, got, c.word)
		}
	}
}

// TestCoxFormatWall 是 K4：FormatWall 的边界表，并且每个值经简洁档成员头再走一遍；另测 FormatElapsed。
//
// 【判别力】%.1f 四舍五入会把 59999 印成 60.0s、99 印成 0.1s；<= 60000 会把 60000 印成 60.0s；
// 负数不夹紧时 -1500 印 -1.-5s（取 -1500 而不是 -5：Go 向零截断下 -5 不夹紧也印 0.0s，杀不到）。
// 成员头那一遍证明 writeAskRecord 确实用的是 FormatWall（改用 FormatElapsed 时 6849 印 6s）。
// FormatElapsed 的四个值分别落在负数、不足一分钟（59999ms 向零截断为 59s）、不足一小时、一小时以上四档。
func TestCoxFormatWall(t *testing.T) {
	walls := []struct {
		ms   int
		want string
	}{
		{-1500, "0.0s"},
		{0, "0.0s"},
		{99, "0.0s"},
		{100, "0.1s"},
		{999, "0.9s"},
		{1000, "1.0s"},
		{6849, "6.8s"},
		{59999, "59.9s"},
		{60000, "1m00s"},
		{65432, "1m05s"},
		{3600000, "1h00m"},
	}
	for _, w := range walls {
		if got := FormatWall(w.ms); got != w.want {
			t.Errorf("FormatWall(%d) = %q，期望 %q", w.ms, got, w.want)
		}
		r, b := coxNewConciseRecordREPL()
		rec := turn.Record{AgentID: "cox_a", Outcome: turn.OutcomeSpoke, WallMS: w.ms, Message: &conversation.Message{Text: "x"}}
		if err := writeAskRecord(r, rec); err != nil {
			t.Fatalf("writeAskRecord 返回错误: %v", err)
		}
		if got, want := b.String(), "甲 A（"+w.want+"）: x\n"; got != want {
			t.Errorf("WallMS=%d 的成员头不对\n实际: %q\n期望: %q", w.ms, got, want)
		}
	}
	elapsed := []struct {
		d    time.Duration
		want string
	}{
		{-1 * time.Second, "0s"},
		{59999 * time.Millisecond, "59s"},
		{3599 * time.Second, "59m59s"},
		{7322 * time.Second, "2h02m"},
	}
	for _, e := range elapsed {
		if got := FormatElapsed(e.d); got != e.want {
			t.Errorf("FormatElapsed(%v) = %q，期望 %q", e.d, got, e.want)
		}
	}
}

// TestCoxDisplayNameFollowsCurrentConfig 是 K5：同一个 Record 连写三次，
// 中间先改 map 里的 DisplayName、再整个换掉 r.cfg 指针。
//
// 【判别力】在 SetConcise / New 时缓存名字，或持有旧 *config.Config，第 2、3 行红。
// 第三步模拟 /config reload --apply 与 MD 那一刀的换指针。第一行是正对照（原配置下确实印「甲 A」）。
func TestCoxDisplayNameFollowsCurrentConfig(t *testing.T) {
	r, b := coxNewConciseRecordREPL()
	rec := turn.Record{AgentID: "cox_a", Outcome: turn.OutcomeSpoke, WallMS: 1000, Message: &conversation.Message{Text: "x"}}
	if err := writeAskRecord(r, rec); err != nil {
		t.Fatalf("第一次 writeAskRecord 返回错误: %v", err)
	}
	ac := r.cfg.Agents["cox_a"]
	ac.DisplayName = "乙"
	r.cfg.Agents["cox_a"] = ac
	if err := writeAskRecord(r, rec); err != nil {
		t.Fatalf("第二次 writeAskRecord 返回错误: %v", err)
	}
	r.cfg = &config.Config{Agents: map[string]config.AgentConfig{"cox_a": {DisplayName: "丙"}}}
	if err := writeAskRecord(r, rec); err != nil {
		t.Fatalf("第三次 writeAskRecord 返回错误: %v", err)
	}
	want := "甲 A（1.0s）: x\n乙（1.0s）: x\n丙（1.0s）: x\n"
	if got := b.String(); got != want {
		t.Fatalf("显示名没有跟着当前配置走\n实际: %q\n期望: %q", got, want)
	}
}
