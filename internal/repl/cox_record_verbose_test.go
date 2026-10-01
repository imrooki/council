package repl

// CO1 §2.1 的第一个文件：公共配置、REPL 构造辅助、8 个 Record 的夹具与 K2。
// 本文件只用 HEAD 已有的标识符（不调 SetConcise、不读 concise 字段），
// 所以它能单独放进 HEAD 的拷贝里编译运行，证明 K2 的期望就是今天的输出。

import (
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/turn"
)

// coxRecordCfg 是 §2.1 的公共配置：cox_a 有显示名，cox_b 显示名为空；
// cox_gone 故意不在配置里。
func coxRecordCfg() *config.Config {
	return &config.Config{Agents: map[string]config.AgentConfig{
		"cox_a": {DisplayName: "甲 A", Provider: "claude", Model: "m", Effort: "high"},
		"cox_b": {DisplayName: "", Provider: "claude", Model: "m", Effort: "high"},
	}}
}

// coxNewRecordREPL 造一个只写内存缓冲的 REPL。不带档位参数：
// 得到的是零值档位（详细）；要简洁档的调用方自己在构造之后调 SetConcise(true)。
func coxNewRecordREPL() (*REPL, *strings.Builder) {
	var b strings.Builder
	r := New(coxRecordCfg(), nil, nil, nil, &b, RulesContext{})
	return r, &b
}

// coxFloat 返回指向 v 的指针。
func coxFloat(v float64) *float64 { return &v }

// coxRecordCase 是一个 Record 夹具：名字即子测试名，concise / verbose 是两档的整串期望。
type coxRecordCase struct {
	name    string
	rec     turn.Record
	concise string
	verbose string
}

// coxRecordCases 是 §2.1 的 8 个共用 Record，K1、K2 共用。
// 期望值逐字取自契约 §2.1 的 K1 / K2 两张表。
func coxRecordCases() []coxRecordCase {
	return []coxRecordCase{
		{
			name: "spoke_rich",
			rec: turn.Record{
				AgentID:         "cox_a",
				Outcome:         turn.OutcomeSpoke,
				WallMS:          6849,
				Fresh:           true,
				ProviderSession: "sess-x",
				UsageParsed:     true,
				CostUSDDelta:    coxFloat(0.5),
				ColdExpected:    true,
				Message: &conversation.Message{
					Seq:            5,
					SenderID:       "cox_a",
					SenderProvider: "claude",
					SenderModel:    "m",
					SenderEffort:   "high",
					Text:           "第一行\n第二行\n",
				},
			},
			concise: "甲 A（6.8s）: 第一行\n第二行\n",
			verbose: "outcome spoke  agent cox_a\n新开了一条 provider 会话 sess-x\nseq 5  cox_a  claude/m/high\n第一行\n第二行\n用量 in=0 cache_create=? cache_read=0 out=0 thinking=0\n金额 0.5 USD\n冷暖 预报=冷 实测=暖  wall_ms=6849  prompt_bytes=0  scanned_to=0\n",
		},
		{
			name: "spoke_no_newline",
			rec: turn.Record{
				AgentID: "cox_a",
				Outcome: turn.OutcomeSpoke,
				WallMS:  0,
				Message: &conversation.Message{Text: "OK"},
			},
			concise: "甲 A（0.0s）: OK\n",
			verbose: "outcome spoke  agent cox_a\nseq 0    -\nOK\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=0  prompt_bytes=0  scanned_to=0\n",
		},
		{
			name: "spoke_empty",
			rec: turn.Record{
				AgentID: "cox_a",
				Outcome: turn.OutcomeSpoke,
				WallMS:  99,
				Message: &conversation.Message{Text: ""},
			},
			concise: "甲 A（0.0s）:\n本轮为空\n",
			verbose: "outcome spoke  agent cox_a\nseq 0    -\n本轮为空\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=99  prompt_bytes=0  scanned_to=0\n",
		},
		{
			name: "spoke_truncated",
			rec: turn.Record{
				AgentID: "cox_a",
				Outcome: turn.OutcomeSpoke,
				WallMS:  60000,
				Message: &conversation.Message{
					Text: "半句",
					Meta: map[string]any{turn.MetaCompletion: "truncated"},
				},
			},
			concise: "甲 A（1m00s）: 半句\n本轮被截断\n",
			verbose: "outcome spoke  agent cox_a\nseq 0    -\n本轮被截断\n半句\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=60000  prompt_bytes=0  scanned_to=0\n",
		},
		{
			name: "timeout_maybe_advanced",
			rec: turn.Record{
				AgentID:         "cox_a",
				Outcome:         turn.OutcomeTimeout,
				WallMS:          300000,
				MaybeAdvanced:   true,
				ProviderSession: "sess-y",
			},
			concise: "甲 A: 没有回复（超时）\nprovider 那边可能已经吃下这批内容\n",
			verbose: "outcome timeout  agent cox_a\nprovider 那边可能已经吃下这批内容\nprovider 会话 sess-y\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=300000  prompt_bytes=0  scanned_to=0\n",
		},
		{
			name: "post_rejected_drift_denied",
			rec: turn.Record{
				AgentID:         "cox_b",
				Outcome:         turn.OutcomePostRejected,
				WallMS:          1234,
				Drifted:         true,
				DroppedText:     "付过钱的\n两行",
				DeniedActions:   []provider.DeniedAction{{Action: "write_file", DisplayName: "WriteToFile"}},
				UsageParsed:     true,
				ProviderSession: "sess-z",
			},
			concise: "cox_b: 没有回复（未入库）\n送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了\nprovider 报了 1 个被拒的工具调用（可能不止这些）：write_file(WriteToFile)。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败\n正文写不进库（已经付过钱），整段如下:\n付过钱的\n两行\n",
			verbose: "outcome post_rejected  agent cox_b\nprovider 会话 sess-z\n送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了\nprovider 报了 1 个被拒的工具调用（可能不止这些）：write_file(WriteToFile)。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败\n正文写不进库（已经付过钱），整段如下:\n付过钱的\n两行\n用量 in=0 cache_create=? cache_read=0 out=0 thinking=0\n金额未报\n冷暖 预报=暖 实测=暖  wall_ms=1234  prompt_bytes=0  scanned_to=0\n",
		},
		{
			name: "refused_not_in_config",
			rec: turn.Record{
				AgentID: "cox_gone",
				Outcome: turn.OutcomeRefused,
			},
			concise: "cox_gone: 没有回复（拒绝）\n",
			verbose: "outcome refused  agent cox_gone\n",
		},
		{
			name: "spoke_fallback_id",
			rec: turn.Record{
				AgentID: "cox_b",
				Outcome: turn.OutcomeSpoke,
				WallMS:  100,
				Message: &conversation.Message{Text: "B"},
			},
			concise: "cox_b（0.1s）: B\n",
			verbose: "outcome spoke  agent cox_b\nseq 0    -\nB\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=100  prompt_bytes=0  scanned_to=0\n",
		},
	}
}

// TestCoxVerboseRecordUnchanged 是 K2：详细档（不调 SetConcise）下同一组 Record 的整串输出。
//
// 【判别力】「详细档逐字节不变」的唯一逐字节担保：期望就是 HEAD 的 writeAskRecord 输出
// （本文件单独放进 HEAD 拷贝跑过，8 个子测试全绿）。每个 Record 都喂了会让某一类行出现的料
// （会话行、seq、用量、金额、冷暖、漂移、被拒工具调用、PostRejected 整段），详细档漏打任何一行都红。
// 把门写反（if !r.concise 与 if r.concise 互换）时本用例与 K1 同时红；对照是 K1（同一组 Record 的简洁档）。
func TestCoxVerboseRecordUnchanged(t *testing.T) {
	for _, tc := range coxRecordCases() {
		t.Run(tc.name, func(t *testing.T) {
			r, b := coxNewRecordREPL()
			if err := writeAskRecord(r, tc.rec); err != nil {
				t.Fatalf("writeAskRecord 返回错误: %v", err)
			}
			if got := b.String(); got != tc.verbose {
				t.Fatalf("详细档输出与 HEAD 不同\n实际: %q\n期望: %q", got, tc.verbose)
			}
		})
	}
}
