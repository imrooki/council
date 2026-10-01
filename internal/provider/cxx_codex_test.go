package provider_test

// CX1 测试腿：codex 适配器的纯函数验收（契约 E:\multi_model_cowork\cx1_contract.md §2.1）。
//
// 本文件只调 provider 包的导出标识符，不起进程、不读文件。夹具是两份真机原件
// （中转根 codexprobe_exec1.jsonl / codexprobe_exec2.jsonl，2026-09-28 主代理两次付费
// 调用的 stdout）逐字节抄成的常量，行尾是原件里的 CRLF；turn.failed 与 error 两种
// 失败形状没有真机原件，P6 / P7 里凡用到它们的输入都是合成的：失败形状是推断，
// 本机零观测。

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"council/internal/provider"
)

// cxxExec1CRLF 逐字节等于 codexprobe_exec1.jsonl（347 字节，SHA-256 E888CBCE…）：
// 新会话一轮，正文 PONG。行尾 CRLF 原样保留。
const cxxExec1CRLF = "{\"type\":\"thread.started\",\"thread_id\":\"01a0e373-ac6c-7083-87d7-62536800ada0\"}\r\n" +
	"{\"type\":\"turn.started\"}\r\n" +
	"{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"agent_message\",\"text\":\"PONG\"}}\r\n" +
	"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":16184,\"cached_input_tokens\":0,\"cache_write_input_tokens\":0,\"output_tokens\":6,\"reasoning_output_tokens\":0}}\r\n"

// cxxExec2CRLF 逐字节等于 codexprobe_exec2.jsonl（352 字节，SHA-256 08B20446…）：
// 同一 thread 续会话一轮，正文 KIWI。
const cxxExec2CRLF = "{\"type\":\"thread.started\",\"thread_id\":\"01a0e373-ac6c-7083-87d7-62536800ada0\"}\r\n" +
	"{\"type\":\"turn.started\"}\r\n" +
	"{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"agent_message\",\"text\":\"KIWI\"}}\r\n" +
	"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":32396,\"cached_input_tokens\":16000,\"cache_write_input_tokens\":0,\"output_tokens\":12,\"reasoning_output_tokens\":0}}\r\n"

// 两份原件里 usage 子对象的原文（UsageRaw 的期望值）。
const (
	cxxUsage1   = `{"input_tokens":16184,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":6,"reasoning_output_tokens":0}`
	cxxUsage2   = `{"input_tokens":32396,"cached_input_tokens":16000,"cache_write_input_tokens":0,"output_tokens":12,"reasoning_output_tokens":0}`
	cxxThreadID = "01a0e373-ac6c-7083-87d7-62536800ada0"
)

// cxxUnreported 是契约 §3.6 冻结的未报清单与顺序。
var cxxUnreported = []string{"input_tokens", "cache_create", "cache_read", "output_tokens", "thinking_tokens"}

// cxxArgs 按契约 §2 通用纪律生成期望 argv（§3.1 / §3.2），model 固定 gpt-6-sol。
func cxxArgs(session, effort string) []string {
	a := []string{"exec"}
	if session != "" {
		a = append(a, "resume", session)
	}
	a = append(a, "--json", "--skip-git-repo-check", "-c", "sandbox_mode=read-only", "-c", "approval_policy=never", "-m", "gpt-6-sol")
	if effort != "" {
		a = append(a, "-c", "model_reasoning_effort="+effort)
	}
	a = append(a, "--ignore-rules", "--disable", "plugins", "--disable", "apps", "-c", "skills.include_instructions=false", "-")
	return a
}

func cxxCodex(t *testing.T) provider.Provider {
	t.Helper()
	p, err := provider.Lookup("codex")
	if err != nil {
		t.Fatalf("Lookup(%q): %v", "codex", err)
	}
	return p
}

func cxxReq() provider.TurnRequest {
	return provider.TurnRequest{
		AgentID:     "sol",
		DisplayName: "Sol",
		Model:       "gpt-6-sol",
		Effort:      "low",
		Role:        "R1\nR2",
		Permission:  provider.PermissionRead,
		Prompt:      "P1\n\nP2",
		WorkDir:     `C:\cxx\wd`,
	}
}

// cxxOK 造一个成功形状的期望 TurnResult（契约 §2.1 共用）。
func cxxOK(text, thought, session, usage string) provider.TurnResult {
	r := provider.TurnResult{
		Text:            text,
		Thought:         thought,
		ProviderSession: session,
		StopRaw:         "turn.completed",
		Usage:           provider.Usage{CostBasis: provider.CostBasisNone, Unreported: cxxUnreported},
		Completion:      provider.CompletionComplete,
	}
	if text == "" {
		r.Completion = provider.CompletionEmpty
	}
	if usage != "" {
		r.UsageRaw = json.RawMessage(usage)
	}
	return r
}

// cxxFail 造一个失败形状的期望 TurnResult（契约 §2.1 P6 的 fail 记号）：
// 会话 id 固定 T1，Completion 留零值 CompletionUnknown。
func cxxFail(stop, usage string) provider.TurnResult {
	r := provider.TurnResult{
		ProviderSession: "T1",
		StopRaw:         stop,
		Usage:           provider.Usage{CostBasis: provider.CostBasisNone, Unreported: cxxUnreported},
	}
	if usage != "" {
		r.UsageRaw = json.RawMessage(usage)
	}
	return r
}

// P6 / P7 的输入记号（合成，未实测）。
const (
	cxxTh   = "{\"type\":\"thread.started\",\"thread_id\":\"T1\"}\n"
	cxxDone = "{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1}}\n"
	cxxU1   = `{"input_tokens":1}`
)

func cxxMsg(id, typ, text string) string {
	return "{\"type\":\"item.completed\",\"item\":{\"id\":\"" + id + "\",\"type\":\"" + typ + "\",\"text\":\"" + text + "\"}}\n"
}

// TestCxxCapabilities 覆盖 P1：整个 Capabilities 与 §3.6 的字面量 DeepEqual。
//
// 【判别力】任一字段改一档都不等（X20-X27）。ModelDiscovery 由 CXH 改成
// ModelDiscoverySubcommand，want 显式写出；codex.go 退回 Unknown 时这一项不等。
func TestCxxCapabilities(t *testing.T) {
	want := provider.Capabilities{
		SessionID:            provider.SessionIDReturnedOnFirstTurn,
		Effort:               provider.EffortPerTurnFlag,
		Identity:             provider.IdentityPromptPreamble,
		Prompt:               provider.PromptTransportStdin,
		MaxPromptBytes:       0,
		Permissions:          []provider.PermissionTier{provider.PermissionRead},
		UsageFormula:         "input_tokens",
		OutputSchema:         provider.SupportYes,
		MaxTurnsFlag:         provider.SupportNo,
		ReportsCost:          provider.SupportNo,
		ReportsCacheCreate:   provider.SupportYes,
		ReportsObservedModel: provider.SupportNo,
		CleanInstructionEnv:  provider.SupportNo,
		CacheAcrossTurns:     provider.SupportYes,
		WriteBoundary:        provider.SupportUnknown,
		ModelDiscovery:       provider.ModelDiscoverySubcommand,
	}
	if got := cxxCodex(t).Capabilities(); !reflect.DeepEqual(got, want) {
		t.Errorf("Capabilities() =\n%+v\n期望\n%+v", got, want)
	}
}

// TestCxxBuildTurnShapes 覆盖 P2：六种请求形状下整个 Command 的 DeepEqual。
//
// 【判别力】resume 与 fresh 只差会话 id：身份只在空会话拼（X06），续会话要带
// resume <id>（X16）；fresh_no_role 与 fresh 只差 Role：空 Role 不拼、不留前导换行
// （X07）；身份与正文之间恰好一个换行（X08）；正文不进 argv、argv 末尾是 "-"
// （X09、X14、X56）；argv 其余每一项（X10-X13、X15、X17、X18、X55、X57）；
// max_turns 与 fresh 只差 MaxTurns：Dropped 必须带一项（X19）。Path / Env / Files /
// PlannedSessionID 必须是零值，由整值比较钉住。
func TestCxxBuildTurnShapes(t *testing.T) {
	type tc struct {
		name    string
		mut     func(*provider.TurnRequest)
		args    []string
		stdin   string
		dropped []provider.DroppedOption
	}
	cases := []tc{
		{"fresh", func(r *provider.TurnRequest) {}, cxxArgs("", "low"), "R1\nR2\nP1\n\nP2", nil},
		{"resume", func(r *provider.TurnRequest) { r.ProviderSession = cxxThreadID }, cxxArgs(cxxThreadID, "low"), "P1\n\nP2", nil},
		{"fresh_no_role", func(r *provider.TurnRequest) { r.Role = "" }, cxxArgs("", "low"), "P1\n\nP2", nil},
		{"fresh_no_effort", func(r *provider.TurnRequest) { r.Effort = "" }, cxxArgs("", ""), "R1\nR2\nP1\n\nP2", nil},
		{"resume_no_effort", func(r *provider.TurnRequest) { r.Effort = ""; r.ProviderSession = cxxThreadID }, cxxArgs(cxxThreadID, ""), "P1\n\nP2", nil},
		{"max_turns", func(r *provider.TurnRequest) { r.MaxTurns = 3 }, cxxArgs("", "low"), "R1\nR2\nP1\n\nP2",
			[]provider.DroppedOption{{Option: "max_turns", Value: "3", Reason: "this provider has no max-turns flag"}}},
	}
	p := cxxCodex(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := cxxReq()
			c.mut(&req)
			got, err := p.BuildTurn(req)
			if err != nil {
				t.Fatalf("BuildTurn 报错: %v", err)
			}
			want := provider.Command{
				Args:      c.args,
				Dir:       `C:\cxx\wd`,
				Stdin:     []byte(c.stdin),
				Transport: provider.PromptTransportStdin,
				Dropped:   c.dropped,
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Command =\n%#v\n期望\n%#v\n（Stdin 实际 %q，期望 %q）", got, want, got.Stdin, want.Stdin)
			}
		})
	}
}

// TestCxxBuildTurnEffort 覆盖 P3：强度取值的白名单与报错文本。
//
// 【判别力】合法值表两端 minimal、ultra 各自单独杀一条变异（X03、X02）；LOW 杀
// 大小写不敏感（X04）；首尾空格两条防 Trim；去掉校验（X01）时非法值那一段红；
// 哨兵换成别的（X05）时 errors.Is 红；新哨兵文本改掉（X52）时整串红。对照是
// 合法值那一段（同一装置、只差取值）。
func TestCxxBuildTurnEffort(t *testing.T) {
	p := cxxCodex(t)
	for _, e := range []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"} {
		t.Run("ok_"+e, func(t *testing.T) {
			req := cxxReq()
			req.Effort = e
			got, err := p.BuildTurn(req)
			if err != nil {
				t.Fatalf("合法强度 %q 被拒: %v", e, err)
			}
			if want := cxxArgs("", e); !reflect.DeepEqual(got.Args, want) {
				t.Errorf("Args = %q，期望 %q", got.Args, want)
			}
		})
	}
	bad := []struct{ name, e string }{
		{"bad_bogus", "bogus"},
		{"bad_upper_LOW", "LOW"},
		{"bad_none", "none"},
		{"bad_leading_space", " high"},
		{"bad_trailing_space", "high "},
		{"bad_effort_golden", "effort-golden"},
	}
	for _, b := range bad {
		e := b.e
		t.Run(b.name, func(t *testing.T) {
			req := cxxReq()
			req.Effort = e
			got, err := p.BuildTurn(req)
			if !errors.Is(err, provider.ErrUnsupportedEffort) {
				t.Fatalf("强度 %q：err = %v，期望包着 ErrUnsupportedEffort", e, err)
			}
			if !reflect.DeepEqual(got, provider.Command{}) {
				t.Errorf("强度 %q 被拒时 Command 应为零值，实际 %#v", e, got)
			}
		})
	}
	t.Run("bogus_text", func(t *testing.T) {
		req := cxxReq()
		req.Effort = "bogus"
		_, err := p.BuildTurn(req)
		const want = `effort "bogus" is not a codex reasoning level (want one of minimal, low, medium, high, xhigh, max, ultra): effort value not supported by this provider`
		if err == nil || err.Error() != want {
			t.Errorf("err = %v\n期望 %s", err, want)
		}
	})
}

// TestCxxBuildTurnTiers 覆盖 P4：只给 read 档，workspace / full / 空档与空 WorkDir 都在
// 起进程之前拒绝。
//
// 【判别力】workspace、full 两条的文本来自 validateRequest 的写入边界闸，证明拒绝
// 发生在 codexPermissionArgs 之前（X53 去掉 validateRequest 后文本变成
// 「codex adapter offers read only」）；WriteBoundary 声明成 Yes（X23）时 workspace
// 这条文本变成空档那一形（不再有 no verified write boundary）。对照是 P2 的 fresh
// （同一请求只差 Permission，成功）。
func TestCxxBuildTurnTiers(t *testing.T) {
	p := cxxCodex(t)
	cases := []struct {
		name string
		tier provider.PermissionTier
		want string
	}{
		{"workspace", provider.PermissionWorkspace, "permission tier \"workspace\": no verified write boundary: permission tier not supported by this provider"},
		{"full", provider.PermissionFull, "permission tier \"full\": no verified write boundary: permission tier not supported by this provider"},
		{"empty", "", "permission tier \"\": permission tier not supported by this provider"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := cxxReq()
			req.Permission = c.tier
			got, err := p.BuildTurn(req)
			if !errors.Is(err, provider.ErrUnsupportedTier) {
				t.Fatalf("err = %v，期望包着 ErrUnsupportedTier", err)
			}
			if err.Error() != c.want {
				t.Errorf("err.Error() = %q\n期望 %q", err.Error(), c.want)
			}
			if !reflect.DeepEqual(got, provider.Command{}) {
				t.Errorf("被拒时 Command 应为零值，实际 %#v", got)
			}
		})
	}
	t.Run("no_workdir", func(t *testing.T) {
		req := cxxReq()
		req.WorkDir = ""
		got, err := p.BuildTurn(req)
		if !errors.Is(err, provider.ErrNoWorkDir) {
			t.Fatalf("err = %v，期望包着 ErrNoWorkDir", err)
		}
		if !reflect.DeepEqual(got, provider.Command{}) {
			t.Errorf("被拒时 Command 应为零值，实际 %#v", got)
		}
	})
}

// TestCxxParseFixtures 覆盖 P5：两份真机原件（以及去掉 \r 的 LF 版）的整值解析。
//
// 【判别力】usage 里任何一个数被填进 Usage、或未报清单少一项（X39、X40）都不等；
// UsageRaw 取成整行而不是 usage 子对象（X41）不等；StopRaw 丢掉（X47）不等。
// 它的对照是 turn 包 T1 里同一装置下的 claude 成员（token 列非 NULL）。
func TestCxxParseFixtures(t *testing.T) {
	p := cxxCodex(t)
	cases := []struct {
		name string
		in   string
		want provider.TurnResult
	}{
		{"exec1_crlf", cxxExec1CRLF, cxxOK("PONG", "", cxxThreadID, cxxUsage1)},
		{"exec1_lf", strings.ReplaceAll(cxxExec1CRLF, "\r", ""), cxxOK("PONG", "", cxxThreadID, cxxUsage1)},
		{"exec2_crlf", cxxExec2CRLF, cxxOK("KIWI", "", cxxThreadID, cxxUsage2)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := p.ParseTurn([]byte(c.in))
			if err != nil {
				t.Fatalf("ParseTurn 报错: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("TurnResult =\n%#v\n期望\n%#v\n（UsageRaw 实际 %s）", got, c.want, got.UsageRaw)
			}
		})
	}
}

// TestCxxParseShapes 覆盖 P6：21 个合成输入（合成，未实测）的哨兵与整个 TurnResult。
//
// 【判别力】成对的对照（每对只差一处）：
//   - multi_msg 与 same_thread_twice：三条 vs 一条 agent_message，杀取第一条（X28）、
//     Thought 丢掉（X29）、全部拼接（X30）、收了 reasoning（X31）；
//   - reasoning_only 与 zero_msg：只差一条非 agent_message 条目（X31）；
//   - error_then_completed 与 error_only：只差有没有 turn.completed（X36）；
//   - failed_and_completed 与 turn_failed：只差后面有没有 turn.completed（X35）；前者
//     也是「失败路径带回 UsageRaw」唯一的非空输入（X50）；两者都杀失败时丢会话 id（X49）；
//   - null_usage 与 zero_msg：只差 usage 是不是 null（X42）；
//   - no_completed 与 error_only：只差有没有 error 事件（X37）；
//   - blank_line_inside 与 same_thread_twice：只差中间只有 \r 的空行（X32）；
//   - item_without_item 与 zero_msg：只差一条不带 item 的 item.completed（X59 空指针）；
//   - second_thread_empty 与 two_threads：只差第二个 id 是空串还是 T2（X60）；
//   - turn_failed_no_error 与 turn_failed：只差 turn.failed 带不带 error 对象（X61 空指针）。
//
// 另：two_completed 杀 X33，two_threads 杀 X34，nonjson_line 杀 X44，no_thread 杀 X43，
// zero_msg 等四条杀 X46。
func TestCxxParseShapes(t *testing.T) {
	p := cxxCodex(t)
	zero := provider.TurnResult{}
	cases := []struct {
		name string
		in   string
		sent error
		want provider.TurnResult
	}{
		{"multi_msg", cxxTh + cxxMsg("item_0", "agent_message", "A") + cxxMsg("item_1", "reasoning", "R") + cxxMsg("item_2", "agent_message", "B") + cxxMsg("item_3", "agent_message", "C") + cxxDone,
			nil, cxxOK("C", "A\n\nB", "T1", cxxU1)},
		{"zero_msg", cxxTh + cxxDone, nil, cxxOK("", "", "T1", cxxU1)},
		{"reasoning_only", cxxTh + cxxMsg("item_0", "reasoning", "R") + cxxDone, nil, cxxOK("", "", "T1", cxxU1)},
		{"error_then_completed", cxxTh + "{\"type\":\"error\",\"message\":\"retry\"}\n" + cxxMsg("item_0", "agent_message", "OK") + cxxDone,
			nil, cxxOK("OK", "", "T1", cxxU1)},
		{"null_usage", cxxTh + "{\"type\":\"turn.completed\",\"usage\":null}\n", nil, cxxOK("", "", "T1", "")},
		{"no_usage", cxxTh + "{\"type\":\"turn.completed\"}\n", nil, cxxOK("", "", "T1", "")},
		{"blank_line_inside", cxxTh + "\r\n" + cxxMsg("item_0", "agent_message", "X") + " \r\n" + cxxDone, nil, cxxOK("X", "", "T1", cxxU1)},
		{"same_thread_twice", cxxTh + cxxTh + cxxMsg("item_0", "agent_message", "X") + cxxDone, nil, cxxOK("X", "", "T1", cxxU1)},
		{"turn_failed", cxxTh + "{\"type\":\"turn.started\"}\n" + "{\"type\":\"turn.failed\",\"error\":{\"message\":\"boom\"}}\n",
			provider.ErrTurnFailed, cxxFail("turn.failed", "")},
		{"failed_and_completed", cxxTh + cxxMsg("item_0", "agent_message", "OK") + "{\"type\":\"turn.failed\",\"error\":{\"message\":\"late\"}}\n" + cxxDone,
			provider.ErrTurnFailed, cxxFail("turn.failed", cxxU1)},
		{"error_only", cxxTh + "{\"type\":\"error\",\"message\":\"stream err\"}\n", provider.ErrTurnFailed, cxxFail("error", "")},
		{"no_completed", cxxTh + "{\"type\":\"turn.started\"}\n", provider.ErrUnparseableOutput, zero},
		{"two_completed", cxxTh + cxxDone + cxxDone, provider.ErrUnparseableOutput, zero},
		{"two_threads", cxxTh + "{\"type\":\"thread.started\",\"thread_id\":\"T2\"}\n" + cxxDone, provider.ErrUnparseableOutput, zero},
		{"nonjson_line", cxxTh + "Reading prompt from stdin...\n" + cxxDone, provider.ErrUnparseableOutput, zero},
		{"unknown_only", "{\"type\":\"something.else\"}\n", provider.ErrUnparseableOutput, zero},
		{"empty", " \r\n\n", provider.ErrUnparseableOutput, zero},
		{"no_thread", "{\"type\":\"turn.started\"}\n" + cxxMsg("item_0", "agent_message", "X") + cxxDone, provider.ErrNoSessionID, cxxOK("X", "", "", cxxU1)},
		{"item_without_item", cxxTh + "{\"type\":\"item.completed\"}\n" + cxxDone, nil, cxxOK("", "", "T1", cxxU1)},
		{"second_thread_empty", cxxTh + "{\"type\":\"thread.started\",\"thread_id\":\"\"}\n" + cxxDone, provider.ErrUnparseableOutput, zero},
		{"turn_failed_no_error", cxxTh + "{\"type\":\"turn.started\"}\n" + "{\"type\":\"turn.failed\"}\n", provider.ErrTurnFailed, cxxFail("turn.failed", "")},
	}
	if len(cases) != 21 {
		t.Fatalf("P6 应有 21 行，实际 %d", len(cases))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := p.ParseTurn([]byte(c.in))
			if c.sent == nil {
				if err != nil {
					t.Fatalf("err = %v，期望 nil", err)
				}
			} else if !errors.Is(err, c.sent) {
				t.Fatalf("err = %v，期望包着 %v", err, c.sent)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("TurnResult =\n%#v\n期望\n%#v\n（UsageRaw 实际 %s）", got, c.want, got.UsageRaw)
			}
		})
	}
}

// TestCxxParseErrorTexts 覆盖 P7：十二个输入的 err.Error() 整串（§3.5 第 3-11 行与
// v1.1 三行）。这些文本经 diagnose 进 Record.Err、经 askCause 上屏，是用户可见字面量。
//
// 【判别力】「认不出任何事件」与「没有 turn.completed」同一个哨兵，只有文本分得开
// （no_known_event 一行是 X45 唯一的杀手）；turn_failed_no_error 与 turn_failed_boom
// 只差 error 对象在不在（X61 的第二个杀手）；two_failed、two_errors 钉「两条失败事件
// 时第一条胜出」（X62、X63），失败信息不进 TurnResult，只有整串文本杀得到，它们的
// 对照是 turn_failed_boom、error_stream（只有一条失败事件）。
func TestCxxParseErrorTexts(t *testing.T) {
	p := cxxCodex(t)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"stdout_empty", "", "codex: stdout is empty: provider output is not a known success shape"},
		{"line2_not_json", cxxTh + "oops\n", "codex: stdout line 2 is not JSON: provider output is not a known success shape"},
		{"no_known_event", "{\"type\":\"x\"}\n", "codex: stdout has no known codex event: provider output is not a known success shape"},
		{"two_completed", cxxTh + cxxDone + cxxDone, "codex: 2 turn.completed events, expected one: provider output is not a known success shape"},
		{"two_threads", cxxTh + "{\"type\":\"thread.started\",\"thread_id\":\"T2\"}\n", "codex: two thread.started events with different thread_id (\"T1\", \"T2\"): provider output is not a known success shape"},
		{"turn_failed_boom", cxxTh + "{\"type\":\"turn.failed\",\"error\":{\"message\":\"boom\"}}\n", "codex: turn failed (message=\"boom\"): provider reported a failed turn"},
		{"error_stream", cxxTh + "{\"type\":\"error\",\"message\":\"stream err\"}\n", "codex: turn failed (error=\"stream err\"): provider reported a failed turn"},
		{"no_completed", cxxTh, "codex: stdout has no turn.completed event: provider output is not a known success shape"},
		{"empty_thread", cxxDone, "codex: success shape carries empty thread_id: provider output carries no session id"},
		{"turn_failed_no_error", cxxTh + "{\"type\":\"turn.failed\"}\n", "codex: turn failed (message=\"\"): provider reported a failed turn"},
		{"two_failed", cxxTh + "{\"type\":\"turn.failed\",\"error\":{\"message\":\"first\"}}\n" + "{\"type\":\"turn.failed\",\"error\":{\"message\":\"second\"}}\n",
			"codex: turn failed (message=\"first\"): provider reported a failed turn"},
		{"two_errors", cxxTh + "{\"type\":\"error\",\"message\":\"first\"}\n" + "{\"type\":\"error\",\"message\":\"second\"}\n",
			"codex: turn failed (error=\"first\"): provider reported a failed turn"},
	}
	if len(cases) != 12 {
		t.Fatalf("P7 应有 12 行，实际 %d", len(cases))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := p.ParseTurn([]byte(c.in))
			if err == nil {
				t.Fatalf("err = nil，期望 %q", c.want)
			}
			if err.Error() != c.want {
				t.Errorf("err.Error() = %q\n期望 %q", err.Error(), c.want)
			}
		})
	}
}
