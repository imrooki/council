package repl

// SM1 §2.3：直接调 writeAskRecord / writeSummaryRecord，不起进程。
// 配置用 smx_summary_round_test.go 的 smxCfg（§2.0 冻结）。
// 引用本刀新增的 writeSummaryRecord 与 CO1 的 SetConcise，在两刀都落地之前编译不过是预期的红。

import (
	"strings"
	"testing"

	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/turn"
)

// smxRecordREPL 每次一个新 REPL，输出到新的 strings.Builder。
func smxRecordREPL(concise bool) (*REPL, *strings.Builder) {
	var b strings.Builder
	r := New(smxCfg("smx_s"), nil, nil, nil, &b, RulesContext{})
	r.SetConcise(concise)
	return r, &b
}

func smxFloat(v float64) *float64 { return &v }

// smxRecordCase 是 §2.3 的六个 Record 与它们在 K1 下的整串期望。
type smxRecordCase struct {
	name    string
	rec     turn.Record
	concise string
}

func smxRecordCases() []smxRecordCase {
	return []smxRecordCase{
		{
			name: "spoke_rich",
			rec: turn.Record{
				AgentID:         "smx_s",
				Outcome:         turn.OutcomeSpoke,
				WallMS:          6849,
				Fresh:           true,
				ProviderSession: "sess-x",
				UsageParsed:     true,
				CostUSDDelta:    smxFloat(0.5),
				ColdExpected:    true,
				Message: &conversation.Message{
					Seq:            5,
					SenderID:       "smx_s",
					SenderProvider: "claude",
					SenderModel:    "m",
					SenderEffort:   "high",
					Text:           "第一行\n第二行\n",
				},
			},
			concise: "总的来看（总结 S 做总结）：第一行\n第二行\n",
		},
		{
			name:    "spoke_empty",
			rec:     turn.Record{AgentID: "smx_s", Outcome: turn.OutcomeSpoke, WallMS: 99, Message: &conversation.Message{Text: ""}},
			concise: "总的来看（总结 S 做总结）：\n本轮为空\n",
		},
		{
			name: "spoke_truncated",
			rec: turn.Record{AgentID: "smx_s", Outcome: turn.OutcomeSpoke, WallMS: 60000, Message: &conversation.Message{
				Text: "半句", Meta: map[string]any{turn.MetaCompletion: "truncated"},
			}},
			concise: "总的来看（总结 S 做总结）：半句\n本轮被截断\n",
		},
		{
			name:    "timeout_maybe",
			rec:     turn.Record{AgentID: "smx_s", Outcome: turn.OutcomeTimeout, WallMS: 300000, MaybeAdvanced: true, ProviderSession: "sess-y"},
			concise: "总的来看：没有总结（超时）\nprovider 那边可能已经吃下这批内容\n",
		},
		{
			name: "post_rejected",
			rec: turn.Record{
				AgentID:         "smx_b",
				Outcome:         turn.OutcomePostRejected,
				WallMS:          1234,
				Drifted:         true,
				DroppedText:     "付过钱的\n两行",
				DeniedActions:   []provider.DeniedAction{{Action: "write_file", DisplayName: "WriteToFile"}},
				UsageParsed:     true,
				ProviderSession: "sess-z",
			},
			concise: "总的来看：没有总结（未入库）\n" +
				"送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了\n" +
				"provider 报了 1 个被拒的工具调用（可能不止这些）：write_file(WriteToFile)。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败\n" +
				"正文写不进库（已经付过钱），整段如下:\n" +
				"付过钱的\n两行\n",
		},
		{
			name:    "summary_fallback_id",
			rec:     turn.Record{AgentID: "smx_b", Outcome: turn.OutcomeSpoke, WallMS: 100, Message: &conversation.Message{Text: "B"}},
			concise: "总的来看（smx_b 做总结）：B\n",
		},
	}
}

// TestSmxSummaryRecordConcise 是 K1：简洁档 writeSummaryRecord 的两个「头」与照打的行。
//
// 【判别力】三个 Spoke 子测试钉成功头：全角冒号写成 ASCII、冒号后多一个空格、头里用 id 而不是显示名，都红；
// timeout_maybe / post_rejected 钉失败头 + 照打行（可能已吃下、漂移、被拒工具调用、整段正文），失败头若带用时
// 会多出（5m00s）/（1.2s），红；summary_fallback_id 是总结头里「显示名为空时回落到 id」的唯一夹具：
// 头绕过 displayName、直接读配置里的显示名时印成「总的来看（ 做总结）：B」，红。
// 第二段把 9 个非 Spoke 结局全走一遍（WallMS 非零），失败头用 outcomeLabel 或漏一个词时红。
// 对照是 K2（同一组 Record 走 writeAskRecord）。
func TestSmxSummaryRecordConcise(t *testing.T) {
	for _, tc := range smxRecordCases() {
		t.Run(tc.name, func(t *testing.T) {
			r, b := smxRecordREPL(true)
			if err := writeSummaryRecord(r, tc.rec); err != nil {
				t.Fatalf("writeSummaryRecord 返回错误: %v", err)
			}
			if got := b.String(); got != tc.concise {
				t.Fatalf("简洁档总结块不对\n实际: %q\n期望: %q", got, tc.concise)
			}
		})
	}
	words := []struct {
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
	for _, w := range words {
		t.Run("outcome_"+w.word, func(t *testing.T) {
			r, b := smxRecordREPL(true)
			if err := writeSummaryRecord(r, turn.Record{AgentID: "smx_s", Outcome: w.o, WallMS: 1234}); err != nil {
				t.Fatalf("writeSummaryRecord 返回错误: %v", err)
			}
			want := "总的来看：没有总结（" + w.word + "）\n"
			if w.o == turn.OutcomePostRejected {
				want += "正文写不进库（已经付过钱），整段如下:\n"
			}
			if got := b.String(); got != want {
				t.Fatalf("结局 %d 的失败头不对\n实际: %q\n期望: %q", w.o, got, want)
			}
		})
	}
}

// smxCutHead 按 §2.3 K2 的切法去掉「头」。
func smxCutHead(t *testing.T, out string, rec turn.Record, sep string) string {
	t.Helper()
	if rec.Message != nil && rec.Message.Text != "" {
		i := strings.Index(out, sep)
		if i < 0 {
			t.Fatalf("前提不成立：输出里找不到 %q：%q", sep, out)
		}
		return out[i+len(sep):]
	}
	i := strings.Index(out, "\n")
	if i < 0 {
		t.Fatalf("前提不成立：输出里没有换行：%q", out)
	}
	return out[i:]
}

// TestSmxSummaryRecordParity 是 K2：格式只有一份。
//
// 【判别力】(a) 详细档 writeSummaryRecord 与 writeAskRecord 逐字节相等：详细档若看了 summary（例如总结段不打 outcome 行
// 或换了标题），红；(b) 简洁档两者去掉「头」之后逐字节相等：把照打的任何一行（可能已吃下、漂移、被拒工具调用、
// 整段正文、本轮为空 / 被截断）只放进其中一条路，红。spoke_rich 的 (a) 喂的是原值非空的全套（会话行、seq、用量、金额、冷暖都有料）。
// (b) 能为假的前提是切法本身不吞内容：两边切下的都是各自的头，K1 已钉住头的整串。
func TestSmxSummaryRecordParity(t *testing.T) {
	for _, tc := range smxRecordCases() {
		t.Run(tc.name, func(t *testing.T) {
			ra, ba := smxRecordREPL(false)
			rs, bs := smxRecordREPL(false)
			if err := writeAskRecord(ra, tc.rec); err != nil {
				t.Fatalf("详细档 writeAskRecord 返回错误: %v", err)
			}
			if err := writeSummaryRecord(rs, tc.rec); err != nil {
				t.Fatalf("详细档 writeSummaryRecord 返回错误: %v", err)
			}
			if ba.String() != bs.String() {
				t.Fatalf("(a) 详细档两者应当逐字节相等\nwriteSummaryRecord: %q\nwriteAskRecord:     %q", bs.String(), ba.String())
			}
			if ba.String() == "" {
				t.Fatalf("前提不成立：详细档输出为空")
			}

			ca, cba := smxRecordREPL(true)
			cs, cbs := smxRecordREPL(true)
			if err := writeAskRecord(ca, tc.rec); err != nil {
				t.Fatalf("简洁档 writeAskRecord 返回错误: %v", err)
			}
			if err := writeSummaryRecord(cs, tc.rec); err != nil {
				t.Fatalf("简洁档 writeSummaryRecord 返回错误: %v", err)
			}
			askBody := smxCutHead(t, cba.String(), tc.rec, ": ")
			sumBody := smxCutHead(t, cbs.String(), tc.rec, "：")
			if askBody != sumBody {
				t.Fatalf("(b) 简洁档去掉头之后应当逐字节相等\nwriteSummaryRecord: %q\nwriteAskRecord:     %q", sumBody, askBody)
			}
		})
	}
}
