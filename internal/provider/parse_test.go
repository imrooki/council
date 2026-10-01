package provider_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"council/internal/provider"
)

// approxEqual 给 CostUSDTotal 这类浮点值一点容差。09_STEP6_SPEC.md K 节自己在
// "cost 差值"那一组写"浮点比较用容差"，本文件对所有金额比较沿用同一个尺度，
// 即便这里比的是 provider 报的原值而不是 Runner 算出来的差值。
func approxEqual(a, b, eps float64) bool {
	return math.Abs(a-b) < eps
}

func wantCostUSDTotal(t *testing.T, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("CostUSDTotal = nil, want %v", want)
	}
	if !approxEqual(*got, want, 1e-6) {
		t.Fatalf("CostUSDTotal = %v, want %v", *got, want)
	}
}

func wantCacheCreate(t *testing.T, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("CacheCreate = nil, want %d (05 §3 陷阱 5/6: 0 与没报是两回事，claude/grok 这一项是 SupportYes/SupportYes 的报告值)", want)
	}
	if *got != want {
		t.Fatalf("CacheCreate = %d, want %d", *got, want)
	}
}

func wantCacheCreateNil(t *testing.T, got *int) {
	t.Helper()
	if got != nil {
		t.Fatalf("CacheCreate = %d, want nil (agy 的 usage 里根本没有 cache_creation 这一项，05 §3 陷阱 5：不许用 0 冒充)", *got)
	}
}

// ---- claude ----

func TestParseTurn_ClaudeSuccessFixtures(t *testing.T) {
	p, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("Lookup(claude): %v", err)
	}

	// 每一行的数字都是 09_STEP6_SPEC.md K 节"解析：claude"那一行钉死的硬数字，
	// 可以直接拿 claude-t{1,2,3}.json 的算术核对：
	// InputTokens = input_tokens + cache_creation_input_tokens + cache_read_input_tokens
	// （F 节公式；claude 是三项相加，与 grok/agy 都不同）。
	cases := []struct {
		name         string
		stdout       string
		wantText     string
		wantInput    int
		wantCacheCr  int
		wantCacheRd  int
		wantOutput   int
		wantThinking int
		wantCost     float64
		wantSession  string
		wantNotUUID  string // session_id 不应该等于这个（顶层 uuid，每轮各异）
		wantStopRaw  string
	}{
		{
			name: "t1_cold_start", stdout: claudeT1JSON, wantText: "OK",
			wantInput: 2 + 47749 + 0, wantCacheCr: 47749, wantCacheRd: 0,
			wantOutput: 4, wantThinking: 0, wantCost: 0.9551999999999999,
			wantSession: "2a90d449-a348-497a-8812-e73e74d6898e",
			wantNotUUID: "78205525-edf7-485a-b1d4-c9cbccd75462",
			wantStopRaw: "end_turn",
		},
		{
			name: "t2_resume", stdout: claudeT2JSON, wantText: "4217",
			wantInput: 2 + 19363 + 28436, wantCacheCr: 19363, wantCacheRd: 28436,
			wantOutput: 4, wantThinking: 0, wantCost: 1.349789,
			wantSession: "2a90d449-a348-497a-8812-e73e74d6898e",
			wantNotUUID: "e63d159c-00b7-42f8-a437-4a570a2daa18",
			wantStopRaw: "end_turn",
		},
		{
			name: "t3_resume", stdout: claudeT3JSON, wantText: "4218",
			wantInput: 2 + 95 + 47752, wantCacheCr: 95, wantCacheRd: 47752,
			wantOutput: 4, wantThinking: 0, wantCost: 1.3638469999999998,
			wantSession: "2a90d449-a348-497a-8812-e73e74d6898e",
			wantNotUUID: "6fe7529a-ae67-4627-8ca8-21bae84259d1",
			wantStopRaw: "end_turn",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := p.ParseTurn([]byte(c.stdout))
			if err != nil {
				t.Fatalf("ParseTurn: unexpected error %v", err)
			}
			if res.Text != c.wantText {
				t.Errorf("Text = %q, want %q", res.Text, c.wantText)
			}
			if res.Usage.InputTokens != c.wantInput {
				t.Errorf("InputTokens = %d, want %d (陷阱 1: 单轮口径必须取 usage 不是 modelUsage)", res.Usage.InputTokens, c.wantInput)
			}
			wantCacheCreate(t, res.Usage.CacheCreate, c.wantCacheCr)
			if res.Usage.CacheRead != c.wantCacheRd {
				t.Errorf("CacheRead = %d, want %d", res.Usage.CacheRead, c.wantCacheRd)
			}
			if res.Usage.OutputTokens != c.wantOutput {
				t.Errorf("OutputTokens = %d, want %d", res.Usage.OutputTokens, c.wantOutput)
			}
			if res.Usage.ThinkingTokens != c.wantThinking {
				t.Errorf("ThinkingTokens = %d, want %d (取自 usage.output_tokens_details.thinking_tokens，陷阱 4)", res.Usage.ThinkingTokens, c.wantThinking)
			}
			wantCostUSDTotal(t, res.Usage.CostUSDTotal, c.wantCost)
			if res.Usage.CostBasis != provider.CostBasisCumulativeSession {
				t.Errorf("CostBasis = %v, want CostBasisCumulativeSession", res.Usage.CostBasis)
			}
			if res.ProviderSession != c.wantSession {
				t.Errorf("ProviderSession = %q, want %q (陷阱 6: 必须取 session_id 不是 uuid)", res.ProviderSession, c.wantSession)
			}
			if res.ProviderSession == c.wantNotUUID {
				t.Errorf("ProviderSession 取成了顶层 uuid（每轮都变的那个记录 id），不是 session_id（三轮不变的会话 id）")
			}
			if res.StopRaw != c.wantStopRaw {
				t.Errorf("StopRaw = %q, want %q", res.StopRaw, c.wantStopRaw)
			}
			if res.Completion != provider.CompletionComplete {
				t.Errorf("Completion = %v, want CompletionComplete", res.Completion)
			}
			if res.ObservedModel != "claude-fable-5-1" {
				t.Errorf("ObservedModel = %q, want %q (modelUsage 恰好一个键)", res.ObservedModel, "claude-fable-5-1")
			}
			if res.ModelKeys != 1 {
				t.Errorf("ModelKeys = %d, want 1", res.ModelKeys)
			}
		})
	}
}

// TestParseTurn_ClaudeSessionAcrossTurnsIsStable 断言三轮的 session_id 完全相同
// （同一条 --resume 链），这是 D 节整个会话 id 设计的地基：ParseTurn 必须每轮都
// 把同一个字符串填进 ProviderSession，Runner 才能拿它去和送出去的 id 做漂移比对。
func TestParseTurn_ClaudeSessionAcrossTurnsIsStable(t *testing.T) {
	p, _ := provider.Lookup("claude")
	r1, _ := p.ParseTurn([]byte(claudeT1JSON))
	r2, _ := p.ParseTurn([]byte(claudeT2JSON))
	r3, _ := p.ParseTurn([]byte(claudeT3JSON))
	if r1.ProviderSession != r2.ProviderSession || r2.ProviderSession != r3.ProviderSession {
		t.Fatalf("session id drifted across turns: %q / %q / %q", r1.ProviderSession, r2.ProviderSession, r3.ProviderSession)
	}
}

// TestParseTurn_ClaudeModelUsageIsCumulative_UsageIsPerTurn 用三轮真实数字互相
// 印证陷阱 1："claude 的 usage 是单轮、modelUsage 是累计"。09_STEP6_SPEC.md K 节
// "累计值对照"那一行点名的三个数字（67112 / 67207 / 76188）逐字来自
// claude-t2.json / claude-t3.json 的 modelUsage 段，这里不重新解析 modelUsage
// （那是实现内部的 json.RawMessage，不在契约面），而是反过来验证：把 ParseTurn
// 各轮解析出的 Usage.CacheCreate / CacheRead 累加，必须等于探针文件里那几个
// 已知的 modelUsage 累计数——如果 ParseTurn 哪天不小心从 modelUsage 取数
// （而不是 usage），这几个数会立刻对不上。
func TestParseTurn_ClaudeModelUsageIsCumulative_UsageIsPerTurn(t *testing.T) {
	p, _ := provider.Lookup("claude")
	r1, err := p.ParseTurn([]byte(claudeT1JSON))
	if err != nil {
		t.Fatalf("t1: %v", err)
	}
	r2, err := p.ParseTurn([]byte(claudeT2JSON))
	if err != nil {
		t.Fatalf("t2: %v", err)
	}
	r3, err := p.ParseTurn([]byte(claudeT3JSON))
	if err != nil {
		t.Fatalf("t3: %v", err)
	}

	cc1, cc2, cc3 := *r1.Usage.CacheCreate, *r2.Usage.CacheCreate, *r3.Usage.CacheCreate
	if got, want := cc1+cc2, 67112; got != want {
		t.Errorf("cache_creation cumulative t1+t2 = %d, want %d (claude-t2.json modelUsage.cacheCreationInputTokens)", got, want)
	}
	if got, want := cc1+cc2+cc3, 67207; got != want {
		t.Errorf("cache_creation cumulative t1+t2+t3 = %d, want %d (claude-t3.json modelUsage.cacheCreationInputTokens)", got, want)
	}
	if got, want := r2.Usage.CacheRead+r3.Usage.CacheRead, 76188; got != want {
		t.Errorf("cache_read cumulative t2+t3 = %d, want %d (claude-t3.json modelUsage.cacheReadInputTokens)", got, want)
	}
	// 反过来钉死：单轮值绝不能等于累计值（否则说明取错了字段）。
	if cc2 == 67112 {
		t.Errorf("t2 的 CacheCreate 等于累计值 67112，说明取自 modelUsage 而不是 usage")
	}
}

// TestParseTurn_ClaudeMissingSessionID 用真实 t1 原件派生：只清空 session_id
// 这一个字段，其余字节不动。09a：成功形状但会话 id 为空 -> ErrNoSessionID，
// 且 Usage 仍然填好了（不能因为要报错就把已经解析出来的用量也扔掉，那部分钱
// 已经花了，账仍要记）。
func TestParseTurn_ClaudeMissingSessionID(t *testing.T) {
	stdout := mustReplaceOnce(t, claudeT1JSON,
		`"session_id":"2a90d449-a348-497a-8812-e73e74d6898e"`,
		`"session_id":""`)
	p, _ := provider.Lookup("claude")
	res, err := p.ParseTurn([]byte(stdout))
	if !errors.Is(err, provider.ErrNoSessionID) {
		t.Fatalf("err = %v, want ErrNoSessionID", err)
	}
	if res.Usage.InputTokens != 47751 {
		t.Errorf("Usage.InputTokens = %d, want 47751 (Usage 必须仍然填好)", res.Usage.InputTokens)
	}
}

// TestParseTurn_ClaudeProviderReportedFailure 用 2026-09-20 补测的真实失败样本
// （claude-badmodel.out，故意传一个不存在的 model 名，退出码 1）。这份原件是
// claude 失败判据里最危险的反例：subtype 单独看是 "success"，五字段合取里另外
// 四项都不过关。测试分两半：
//  1. 证明"陷阱"确实存在——只看 subtype 这一个字段会把它误判成功；
//  2. 证明 ParseTurn 没有掉进这个陷阱——五字段合取正确地判它失败。
func TestParseTurn_ClaudeProviderReportedFailure(t *testing.T) {
	var probe struct {
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal([]byte(claudeBadModelJSON), &probe); err != nil {
		t.Fatalf("decode probe subtype: %v", err)
	}
	if probe.Subtype != "success" {
		t.Fatalf("fixture 前提不成立: subtype = %q, want %q (这条用例就是要测“subtype 单独看是 success”这件事)", probe.Subtype, "success")
	}

	p, _ := provider.Lookup("claude")
	res, err := p.ParseTurn([]byte(claudeBadModelJSON))
	if !errors.Is(err, provider.ErrTurnFailed) {
		t.Fatalf("err = %v, want ErrTurnFailed (只看 subtype==\"success\" 会把这份失败原件误判成功；09_STEP6_SPEC.md G 节的判据是五个字段合取，这份原件里 is_error/terminal_reason/api_error_status/stop_reason 四项独立地拦下了它)", err)
	}
	if res.Text != "" {
		t.Errorf("Text = %q, want empty (绝不能把 result 里的错误正文当成回答返回)", res.Text)
	}
	if res.StopRaw != "stop_sequence" {
		t.Errorf("StopRaw = %q, want %q", res.StopRaw, "stop_sequence")
	}
	if res.ModelKeys != 0 {
		t.Errorf("ModelKeys = %d, want 0 (modelUsage 是 {}，0 个键)", res.ModelKeys)
	}
	wantCostUSDTotal(t, res.Usage.CostUSDTotal, 0)
	// usage 这个键在 JSON 里是存在的，只是五个数值字段全是真实的 0（这次调用
	// 确实没跑到任何 token）。05 §3 陷阱 5 的规则是"provider 没报"才叫
	// Unreported，键存在、值是 0 属于实测值，不该进这张表。
	// commit 498ac77（"fix(step6): 按三家 CLI 的真实失败输出修正解析，关闭
	// Step 6.0 探针 1"）删掉了旧的 noteAllZeroUsage 启发式，判据改成只看 usage
	// 键在不在——这条用例验证的正是那次修正。
	for _, name := range []string{"input_tokens", "cache_read", "output_tokens", "thinking_tokens"} {
		if containsSubstring(res.Usage.Unreported, name) {
			t.Errorf("Unreported 含 %q，但 usage 键本身存在、值是真实的 0，不应该标成没报 (claude-badmodel.out 实测)", name)
		}
	}
}

// ---- grok ----

func TestParseTurn_GrokSuccessFixtures(t *testing.T) {
	p, err := provider.Lookup("grok")
	if err != nil {
		t.Fatalf("Lookup(grok): %v", err)
	}

	cases := []struct {
		name         string
		stdout       string
		wantText     string
		wantInput    int
		wantCacheRd  int
		wantOutput   int
		wantThinking int
		wantSession  string
		wantNotReq   string // 不应该等于 requestId
	}{
		{
			name: "t1_cold_start", stdout: grokT1JSON, wantText: "OK",
			wantInput: 20625 + 0, wantCacheRd: 0, wantOutput: 723, wantThinking: 722,
			wantSession: "c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31",
			wantNotReq:  "9582f1ad-3946-401e-8d0c-9315b94c86fe",
		},
		{
			name: "t2_resume", stdout: grokT2JSON, wantText: "8635",
			wantInput: 250 + 20608, wantCacheRd: 20608, wantOutput: 359, wantThinking: 357,
			wantSession: "c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31",
			wantNotReq:  "0cd23532-5783-408f-83bf-6c0cd67c1601",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := p.ParseTurn([]byte(c.stdout))
			if err != nil {
				t.Fatalf("ParseTurn: unexpected error %v", err)
			}
			if res.Text != c.wantText {
				t.Errorf("Text = %q, want %q", res.Text, c.wantText)
			}
			if res.Usage.InputTokens != c.wantInput {
				t.Errorf("InputTokens = %d, want %d (陷阱 2: grok 完整 prompt = input_tokens + cache_read_input_tokens)", res.Usage.InputTokens, c.wantInput)
			}
			// grok 的 cache_creation_input_tokens 键在两轮里都是 0，即使 t2
			// 明明命中了 20608 的缓存——这个 0 必须进 Unreported（05 §3 陷阱 7），
			// 但 CacheCreate 本身仍然是一个指向 0 的指针，不是 nil（键是在的）。
			wantCacheCreate(t, res.Usage.CacheCreate, 0)
			if !containsSubstring(res.Usage.Unreported, "cache_creat") {
				t.Errorf("Unreported 不含 cache_create 相关项，want 含有（grok 的 cache_creation_input_tokens 恒为 0 但缓存确实命中过）")
			}
			if res.Usage.CacheRead != c.wantCacheRd {
				t.Errorf("CacheRead = %d, want %d (键名是 cache_read_input_tokens)", res.Usage.CacheRead, c.wantCacheRd)
			}
			if res.Usage.OutputTokens != c.wantOutput {
				t.Errorf("OutputTokens = %d, want %d", res.Usage.OutputTokens, c.wantOutput)
			}
			if res.Usage.ThinkingTokens != c.wantThinking {
				t.Errorf("ThinkingTokens = %d, want %d (取自顶层 reasoning_tokens，陷阱 4，键名与 claude/agy 都不同)", res.Usage.ThinkingTokens, c.wantThinking)
			}
			if res.Usage.CostBasis != provider.CostBasisNone {
				t.Errorf("CostBasis = %v, want CostBasisNone", res.Usage.CostBasis)
			}
			if res.Usage.CostUSDTotal != nil {
				t.Errorf("CostUSDTotal = %v, want nil (grok 订阅制，没有金额字段)", *res.Usage.CostUSDTotal)
			}
			if res.ProviderSession != c.wantSession {
				t.Errorf("ProviderSession = %q, want %q", res.ProviderSession, c.wantSession)
			}
			if res.ProviderSession == c.wantNotReq {
				t.Errorf("ProviderSession 取成了 requestId，不是 sessionId")
			}
			if res.Thought == "" {
				t.Errorf("Thought 为空，want 非空且原样带回（grok 是唯一暴露 thought 字段的一家）")
			}
			if res.ObservedModel != "grok-4.6-build" {
				t.Errorf("ObservedModel = %q, want %q", res.ObservedModel, "grok-4.6-build")
			}
			if res.ModelKeys != 1 {
				t.Errorf("ModelKeys = %d, want 1", res.ModelKeys)
			}
			if res.Completion != provider.CompletionComplete {
				t.Errorf("Completion = %v, want CompletionComplete", res.Completion)
			}
		})
	}
}

// TestParseTurn_GrokTextIsNotTrimmed 用真实 t1 派生一份 text 带尾换行的 fixture，
// 证明 TrimRight 只是 agy 的行为（陷阱 9），不该被误套到 grok 头上——grok 的
// text 字段本身没有尾换行，但这条用例不该只靠"原件恰好没有"来通过，构造一份
// 有尾换行的输入更能钉住这一条。
func TestParseTurn_GrokTextIsNotTrimmed(t *testing.T) {
	stdout := mustReplaceOnce(t, grokT1JSON, `"text": "OK",`, "\"text\": \"OK\\n\",")
	p, _ := provider.Lookup("grok")
	res, err := p.ParseTurn([]byte(stdout))
	if err != nil {
		t.Fatalf("ParseTurn: %v", err)
	}
	if res.Text != "OK\n" {
		t.Errorf("Text = %q, want %q (grok 的 text 不做 TrimRight)", res.Text, "OK\n")
	}
}

// TestParseTurn_GrokMissingSessionID 见 claude 那条同名用例的注释，判据一致，
// 字段名不同：grok 是 sessionId。
func TestParseTurn_GrokMissingSessionID(t *testing.T) {
	stdout := mustReplaceOnce(t, grokT1JSON,
		`"sessionId": "c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31",`,
		`"sessionId": "",`)
	p, _ := provider.Lookup("grok")
	res, err := p.ParseTurn([]byte(stdout))
	if !errors.Is(err, provider.ErrNoSessionID) {
		t.Fatalf("err = %v, want ErrNoSessionID", err)
	}
	if res.Usage.InputTokens != 20625 {
		t.Errorf("Usage.InputTokens = %d, want 20625 (Usage 必须仍然填好)", res.Usage.InputTokens)
	}
}

// TestParseTurn_GrokProviderReportedFailure 用 2026-09-20 补测的真实失败样本
// （grok-badmodel.out，退出码 1）。这份原件的形状和 Step 0 七份原件、乃至
// claude/agy 的失败样本都不一样：没有 stopReason / sessionId / usage 中的
// 任何一个，只有一对 {"type":"error","message":"..."}。
//
// commit 498ac77 给 grokOutput 加了 type/message 两个 json tag，并让
// grokShaped 把 v.Type != "" 也算作"认得出的结构"，type=="error" 时归
// ErrTurnFailed（不是 ErrUnparseableOutput）且 StopRaw 记 "error"——这条用例
// 验证的正是那次修正；grok.go 旧版注释"grok 连一个错误字段都没有"已经跟着
// 这次修正一起删掉了。
func TestParseTurn_GrokProviderReportedFailure(t *testing.T) {
	p, _ := provider.Lookup("grok")
	res, err := p.ParseTurn([]byte(grokBadModelJSON))
	if !errors.Is(err, provider.ErrTurnFailed) {
		t.Fatalf("err = %v, want ErrTurnFailed (type==\"error\" 是可识别的失败判据，不是未知结构；见 grok-badmodel.out)", err)
	}
	if res.Text != "" {
		t.Errorf("Text = %q, want empty", res.Text)
	}
	if res.StopRaw != "error" {
		t.Errorf("StopRaw = %q, want %q (type==\"error\" 这条失败信封路径应该让上层看得出走的是信封，不是空 stopReason)", res.StopRaw, "error")
	}
	// usage 这个键在这份原件里根本不存在（不是键存在值为 0），五个数值字段
	// 全部应该标进 Unreported。
	for _, name := range []string{"cache_create", "input_tokens", "cache_read", "output_tokens", "thinking_tokens"} {
		if !containsSubstring(res.Usage.Unreported, name) {
			t.Errorf("Unreported 不含 %q，want 含有（这份原件根本没有 usage 键）", name)
		}
	}
}

// ---- agy ----

func TestParseTurn_AgySuccessFixtures(t *testing.T) {
	p, err := provider.Lookup("agy")
	if err != nil {
		t.Fatalf("Lookup(agy): %v", err)
	}

	cases := []struct {
		name         string
		stdout       string
		wantText     string
		wantInput    int
		wantCacheRd  int
		wantOutput   int
		wantThinking int
		wantSession  string
	}{
		{
			name: "t1_cold_start", stdout: agyT1JSON, wantText: "OK",
			wantInput: 18180, wantCacheRd: 0, wantOutput: 55, wantThinking: 54,
			wantSession: "e484a528-68c6-4f30-ac0f-aa69c1a06ede",
		},
		{
			name: "t2_resume", stdout: agyT2JSON, wantText: "5192",
			wantInput: 20348, wantCacheRd: 16282, wantOutput: 134, wantThinking: 129,
			wantSession: "e484a528-68c6-4f30-ac0f-aa69c1a06ede",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := p.ParseTurn([]byte(c.stdout))
			if err != nil {
				t.Fatalf("ParseTurn: unexpected error %v", err)
			}
			// response 字段带尾换行 "OK\n" / "5192\n"，必须被 TrimRight 掉（陷阱 9）。
			if res.Text != c.wantText {
				t.Errorf("Text = %q, want %q (response 字段应已 TrimRight)", res.Text, c.wantText)
			}
			if res.Usage.InputTokens != c.wantInput {
				t.Errorf("InputTokens = %d, want %d (陷阱 2: agy 的 input_tokens 原样，已经含缓存命中，不能再加 cache_read)", res.Usage.InputTokens, c.wantInput)
			}
			wantCacheCreateNil(t, res.Usage.CacheCreate)
			if !containsSubstring(res.Usage.Unreported, "cache_creat") {
				t.Errorf("Unreported 不含 cache_create 相关项，want 含有（agy 的 usage 里根本没有这一项）")
			}
			if res.Usage.CacheRead != c.wantCacheRd {
				t.Errorf("CacheRead = %d, want %d (键名是 cache_read_tokens，没有 _input_，陷阱 3)", res.Usage.CacheRead, c.wantCacheRd)
			}
			if res.Usage.OutputTokens != c.wantOutput {
				t.Errorf("OutputTokens = %d, want %d", res.Usage.OutputTokens, c.wantOutput)
			}
			if res.Usage.ThinkingTokens != c.wantThinking {
				t.Errorf("ThinkingTokens = %d, want %d (顶层 thinking_tokens，陷阱 4)", res.Usage.ThinkingTokens, c.wantThinking)
			}
			if res.Usage.CostBasis != provider.CostBasisNone {
				t.Errorf("CostBasis = %v, want CostBasisNone", res.Usage.CostBasis)
			}
			if res.ProviderSession != c.wantSession {
				t.Errorf("ProviderSession = %q, want %q", res.ProviderSession, c.wantSession)
			}
			if res.ObservedModel != "" {
				t.Errorf("ObservedModel = %q, want empty (agy 没有 modelUsage)", res.ObservedModel)
			}
			if res.ModelKeys != 0 {
				t.Errorf("ModelKeys = %d, want 0", res.ModelKeys)
			}
			if res.Completion != provider.CompletionComplete {
				t.Errorf("Completion = %v, want CompletionComplete", res.Completion)
			}
		})
	}
}

func TestParseTurn_AgyMissingSessionID(t *testing.T) {
	stdout := mustReplaceOnce(t, agyT1JSON,
		`"conversation_id":"e484a528-68c6-4f30-ac0f-aa69c1a06ede"`,
		`"conversation_id":""`)
	p, _ := provider.Lookup("agy")
	res, err := p.ParseTurn([]byte(stdout))
	if !errors.Is(err, provider.ErrNoSessionID) {
		t.Fatalf("err = %v, want ErrNoSessionID", err)
	}
	if res.Usage.InputTokens != 18180 {
		t.Errorf("Usage.InputTokens = %d, want 18180 (Usage 必须仍然填好)", res.Usage.InputTokens)
	}
}

// TestParseTurn_AgyProviderReportedFailure 用 2026-09-20 补测的真实失败样本
// （agy-badmodel.out，退出码 1）。status="ERROR"（非 "SUCCESS"）本身已经是
// agy 现有 agySuccess/agyShaped 逻辑能正确识别的失败信号；commit 498ac77 又
// 给 agyOutput 加了 Error 字段并把它并进错误串。error 字段的具体文本不在
// 契约面（09a："错误信息的具体中英文措辞...不在"），本用例不对它做字面断言。
func TestParseTurn_AgyProviderReportedFailure(t *testing.T) {
	p, _ := provider.Lookup("agy")
	res, err := p.ParseTurn([]byte(agyBadModelJSON))
	if !errors.Is(err, provider.ErrTurnFailed) {
		t.Fatalf("err = %v, want ErrTurnFailed", err)
	}
	if res.Text != "" {
		t.Errorf("Text = %q, want empty", res.Text)
	}
	if res.StopRaw != "ERROR" {
		t.Errorf("StopRaw = %q, want %q", res.StopRaw, "ERROR")
	}
	// usage 键本身存在、五个数值字段是真实的 0（这次调用没有跑到任何 token），
	// 不该被标进 Unreported——cache_create 例外，agy 无条件没有这一项。
	// 见 claude 那条同名用例的注释：commit 498ac77 删掉了 noteAllZeroUsage，
	// 这条用例验证的正是那次修正。
	for _, name := range []string{"input_tokens", "cache_read", "output_tokens", "thinking_tokens"} {
		if containsSubstring(res.Usage.Unreported, name) {
			t.Errorf("Unreported 含 %q，但 usage 键本身存在、值是真实的 0 (agy-badmodel.out 实测)", name)
		}
	}
	if !containsSubstring(res.Usage.Unreported, "cache_creat") {
		t.Errorf("Unreported 不含 cache_create 相关项，want 含有（agy 无条件没有这一项，与 usage 键是否存在无关）")
	}
}

// TestParseTurn_AgyDeniedActions 覆盖 11c 契约第 7 节记的三层机制里能被
// ParseTurn 单独钉住的那一半：denied_actions 非空、status 仍是 SUCCESS、
// response 被清空，这不是失败（err 必须是 nil，Outcome 由调用方仍然走
// OutcomeSpoke 那条路），而是一条要被响亮报出来的附加事实。
func TestParseTurn_AgyDeniedActions(t *testing.T) {
	p, _ := provider.Lookup("agy")
	res, err := p.ParseTurn([]byte(agyDeniedActionJSON))
	if err != nil {
		t.Fatalf("ParseTurn: unexpected error %v (denied_actions 非空不是失败信封)", err)
	}
	if res.Text != "" {
		t.Errorf("Text = %q, want empty（越界拒绝会把整轮正文清空，这是实测行为）", res.Text)
	}
	if res.Completion != provider.CompletionEmpty {
		t.Errorf("Completion = %v, want CompletionEmpty", res.Completion)
	}
	if len(res.DeniedActions) != 1 {
		t.Fatalf("DeniedActions 长度 = %d, want 1: %+v", len(res.DeniedActions), res.DeniedActions)
	}
	want := provider.DeniedAction{Action: "write_file", DisplayName: "WriteToFile"}
	if res.DeniedActions[0] != want {
		t.Errorf("DeniedActions[0] = %+v, want %+v", res.DeniedActions[0], want)
	}
	if len(res.DeniedActionsRaw) == 0 {
		t.Error("DeniedActionsRaw 为空，want 保留 denied_actions 键的原始字节")
	}
	if !bytes.Contains(res.DeniedActionsRaw, []byte("write_file")) {
		t.Errorf("DeniedActionsRaw = %s，应当原样含 denied_actions 子对象的字节", res.DeniedActionsRaw)
	}
}

// TestParseTurn_AgySuccessFixturesHaveNoDeniedActions 是上一条的反例：
// 两份既有的成功原件（agy-t1 / agy-t2）里 denied_actions 键根本不存在，
// DeniedActions 必须是 nil，不是一个空切片——不然下游没法区分「这一轮
// 没有任何调用被拒」与「这个字段被解析出了一个空数组」。
func TestParseTurn_AgySuccessFixturesHaveNoDeniedActions(t *testing.T) {
	p, _ := provider.Lookup("agy")
	for _, stdout := range []string{agyT1JSON, agyT2JSON} {
		res, err := p.ParseTurn([]byte(stdout))
		if err != nil {
			t.Fatalf("ParseTurn: unexpected error %v", err)
		}
		if res.DeniedActions != nil {
			t.Errorf("DeniedActions = %+v, want nil（denied_actions 键在这份原件里根本不存在）", res.DeniedActions)
		}
		if res.DeniedActionsRaw != nil {
			t.Errorf("DeniedActionsRaw = %s, want nil", res.DeniedActionsRaw)
		}
	}
}

// ---- 三家共用：未知形状、纯函数性质 ----

// TestParseTurn_UnknownShape 断言四种"认不出"的输入都落进 ErrUnparseableOutput：
// 空 stdout、非 JSON、合法但空的 {}、合法 JSON 但字段与已知形状无关。09a 明写
// ParseTurn "连结构都认不出 -> 包了 ErrUnparseableOutput 的错误"；05 §3 陷阱 13
// 与 09_STEP6_SPEC.md C 节都强调三家失败形状零观测，不许凭空猜，所以这里只测
// "认不出"这一支，不去猜三家未观测到的失败 schema（那一支单独用真实样本测，
// 见 TestParseTurn_*ProviderReportedFailure）。
func TestParseTurn_UnknownShape(t *testing.T) {
	stdouts := map[string]string{
		"empty":           "",
		"whitespace_only": "   \n\t  ",
		"not_json":        "not json {",
		"empty_object":    "{}",
		"unrelated_shape": `{"hello":"world"}`,
	}
	for _, id := range []string{"claude", "grok", "agy"} {
		p, err := provider.Lookup(id)
		if err != nil {
			t.Fatalf("Lookup(%s): %v", id, err)
		}
		for name, stdout := range stdouts {
			t.Run(id+"/"+name, func(t *testing.T) {
				_, err := p.ParseTurn([]byte(stdout))
				if !errors.Is(err, provider.ErrUnparseableOutput) {
					t.Fatalf("err = %v, want ErrUnparseableOutput", err)
				}
			})
		}
	}
}

// TestParseTurn_IsPureFunction 断言 ParseTurn 是纯函数：同一段字节连调两次，
// TurnResult 必须逐字段相等（reflect.DeepEqual），错误也必须同样地为 nil 或
// 同样地包着同一个哨兵。09a 原文："ParseTurn 只吃 stdout，是纯函数：同样的
// 字节必须给出同样的 TurnResult"。三种输入各测一遍：成功、失败、认不出。
func TestParseTurn_IsPureFunction(t *testing.T) {
	type call struct {
		id     string
		stdout string
	}
	calls := []call{
		{"claude", claudeT1JSON},
		{"claude", claudeBadModelJSON},
		{"claude", "{}"},
		{"grok", grokT1JSON},
		{"grok", grokBadModelJSON},
		{"agy", agyT1JSON},
		{"agy", agyBadModelJSON},
	}
	for _, c := range calls {
		t.Run(c.id+"/"+c.stdout[:minInt(20, len(c.stdout))], func(t *testing.T) {
			p, _ := provider.Lookup(c.id)
			r1, err1 := p.ParseTurn([]byte(c.stdout))
			r2, err2 := p.ParseTurn([]byte(c.stdout))
			if !reflect.DeepEqual(r1, r2) {
				t.Fatalf("ParseTurn is not pure: r1=%+v r2=%+v", r1, r2)
			}
			if (err1 == nil) != (err2 == nil) {
				t.Fatalf("ParseTurn error-ness differs across calls: err1=%v err2=%v", err1, err2)
			}
			if err1 != nil {
				// 两次都必须包着同一个哨兵，即使 fmt.Errorf 每次生成的字符串
				// 值相等但不是同一个 error 值也没关系——errors.Is 沿 %w 链走。
				sentinels := []error{
					provider.ErrUnparseableOutput, provider.ErrTurnFailed,
					provider.ErrNoSessionID,
				}
				for _, s := range sentinels {
					if errors.Is(err1, s) != errors.Is(err2, s) {
						t.Fatalf("errors.Is(err, %v) differs across calls", s)
					}
				}
			}
		})
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---- TurnResult.UsageRaw (commit 6534c72) ----
//
// UsageRaw 存在的全部意义是"三家的 InputTokens 由三条不同公式算出，公式一旦
// 要改，光有归一化结果回算不出原值"（turn.go 字段注释；09 F 节 / 05 §3 承诺
// "原始 usage 对象整个进 messages.meta[usage_raw]，将来改口径可回算"）。下面
// 几条测试分别钉住：字节忠实、真的可回算、以及"没有这个键"与"这个键的值是
// 真实的 0"两种情况不能被混成同一个 nil。

// rawUsageSubObject 独立于被测代码，从一份 fixture 里摘出 "usage" 这个顶层键
// 的原始字节。用标准库自己解一遍，而不是手抄一份期望字符串——手抄错了、
// 测试还是绿的，这条防的就是这个（coordinator 原话）。
func rawUsageSubObject(t *testing.T, stdout string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &m); err != nil {
		t.Fatalf("decode fixture to extract usage: %v", err)
	}
	return m["usage"]
}

// withTopLevelField 从一份真实 fixture 派生出"某个顶层键被删掉，或被替换成
// 给定值"的变体，用 JSON 往返而不是 mustReplaceOnce 那种字符串手术：usage
// 子对象自己还嵌了好几层花括号，手抄一段来精确匹配容易抄错一个字符而不自知
// （mustReplaceOnce 至少会在 0 次命中时炸掉，但抄错成"另一段同样存在于原文
// 里的字节"这种情况它拦不住）。map[string]json.RawMessage 只把要动的那个键
// 摘出来处理，其余每个键的字节都是 json.RawMessage 自己的 MarshalJSON 原样
// 吐回去，不会被重新格式化，等价于对原文做了一次精确的外科手术。
func withTopLevelField(t *testing.T, stdout string, key string, value json.RawMessage, remove bool) []byte {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &m); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if remove {
		delete(m, key)
	} else {
		m[key] = value
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("re-encode fixture: %v", err)
	}
	return out
}

// TestParseTurn_UsageRawMatchesFixtureBytes 覆盖七份成功原件，逐字节比对
// UsageRaw 与该原件里 "usage" 子对象的原文。
func TestParseTurn_UsageRawMatchesFixtureBytes(t *testing.T) {
	cases := []struct {
		id     string
		stdout string
	}{
		{"claude", claudeT1JSON}, {"claude", claudeT2JSON}, {"claude", claudeT3JSON},
		{"grok", grokT1JSON}, {"grok", grokT2JSON},
		{"agy", agyT1JSON}, {"agy", agyT2JSON},
	}
	for _, c := range cases {
		t.Run(c.id+"/"+c.stdout[:minInt(20, len(c.stdout))], func(t *testing.T) {
			p, _ := provider.Lookup(c.id)
			res, err := p.ParseTurn([]byte(c.stdout))
			if err != nil {
				t.Fatalf("ParseTurn: %v", err)
			}
			want := rawUsageSubObject(t, c.stdout)
			if !bytes.Equal(res.UsageRaw, want) {
				t.Fatalf("UsageRaw = %s, want %s (逐字节等于 usage 子对象原文)", res.UsageRaw, want)
			}
		})
	}
}

// TestParseTurn_UsageRawIsRecomputableIntoInputTokens 是 UsageRaw 存在的全部
// 意义所在，前一条只是形式：把 UsageRaw 反解回该家的原始字段，按
// Capabilities.UsageFormula 描述的公式重新算一遍，必须等于 ParseTurn 已经
// 算好的 Usage.InputTokens——claude 三项相加、grok 两项相加、agy 原样
// （F 节公式，与 claude/grok/agySuccessFixtures 那几条用例给硬数字用的是
// 同一套公式，这里是反过来从原始字节验证）。
func TestParseTurn_UsageRawIsRecomputableIntoInputTokens(t *testing.T) {
	t.Run("claude_three_terms", func(t *testing.T) {
		p, _ := provider.Lookup("claude")
		for _, stdout := range []string{claudeT1JSON, claudeT2JSON, claudeT3JSON} {
			res, err := p.ParseTurn([]byte(stdout))
			if err != nil {
				t.Fatalf("ParseTurn: %v", err)
			}
			var raw struct {
				InputTokens              int `json:"input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			}
			if err := json.Unmarshal(res.UsageRaw, &raw); err != nil {
				t.Fatalf("decode UsageRaw: %v", err)
			}
			if got := raw.InputTokens + raw.CacheCreationInputTokens + raw.CacheReadInputTokens; got != res.Usage.InputTokens {
				t.Errorf("recomputed from UsageRaw = %d, want Usage.InputTokens = %d", got, res.Usage.InputTokens)
			}
		}
	})
	t.Run("grok_two_terms", func(t *testing.T) {
		p, _ := provider.Lookup("grok")
		for _, stdout := range []string{grokT1JSON, grokT2JSON} {
			res, err := p.ParseTurn([]byte(stdout))
			if err != nil {
				t.Fatalf("ParseTurn: %v", err)
			}
			var raw struct {
				InputTokens          int `json:"input_tokens"`
				CacheReadInputTokens int `json:"cache_read_input_tokens"`
			}
			if err := json.Unmarshal(res.UsageRaw, &raw); err != nil {
				t.Fatalf("decode UsageRaw: %v", err)
			}
			if got := raw.InputTokens + raw.CacheReadInputTokens; got != res.Usage.InputTokens {
				t.Errorf("recomputed from UsageRaw = %d, want Usage.InputTokens = %d", got, res.Usage.InputTokens)
			}
		}
	})
	t.Run("agy_as_is", func(t *testing.T) {
		p, _ := provider.Lookup("agy")
		for _, stdout := range []string{agyT1JSON, agyT2JSON} {
			res, err := p.ParseTurn([]byte(stdout))
			if err != nil {
				t.Fatalf("ParseTurn: %v", err)
			}
			var raw struct {
				InputTokens int `json:"input_tokens"`
			}
			if err := json.Unmarshal(res.UsageRaw, &raw); err != nil {
				t.Fatalf("decode UsageRaw: %v", err)
			}
			if raw.InputTokens != res.Usage.InputTokens {
				t.Errorf("recomputed from UsageRaw = %d, want Usage.InputTokens = %d", raw.InputTokens, res.Usage.InputTokens)
			}
		}
	})
}

// TestParseTurn_UsageRawNilWhenUsageKeyAbsentOrNull 分两种"没有"：usage 键
// 整个不存在、以及 usage 字面是 null。两种都必须是 nil，不是 []byte("{}")
// 冒充——那会让"没有 usage"与"usage 是空对象"在库里长成同一个东西。
func TestParseTurn_UsageRawNilWhenUsageKeyAbsentOrNull(t *testing.T) {
	bases := map[string]string{"claude": claudeT1JSON, "grok": grokT1JSON, "agy": agyT1JSON}
	for _, id := range []string{"claude", "grok", "agy"} {
		p, _ := provider.Lookup(id)
		base := bases[id]

		t.Run(id+"/key_absent", func(t *testing.T) {
			stdout := withTopLevelField(t, base, "usage", nil, true)
			res, err := p.ParseTurn(stdout)
			// 去掉 usage 键不影响三家任何一家的成功判据（没有一家把 usage 键
			// 本身的存在与否当成完成信号），这里顺带确认这一点，不只测 UsageRaw。
			if err != nil {
				t.Fatalf("ParseTurn: unexpected error %v after only removing the usage key", err)
			}
			if res.UsageRaw != nil {
				t.Fatalf("UsageRaw = %s, want nil when the usage key is entirely absent", res.UsageRaw)
			}
		})

		t.Run(id+"/key_is_literal_null", func(t *testing.T) {
			stdout := withTopLevelField(t, base, "usage", json.RawMessage("null"), false)
			res, err := p.ParseTurn(stdout)
			if err != nil {
				t.Fatalf("ParseTurn: unexpected error %v after only nulling the usage key", err)
			}
			if res.UsageRaw != nil {
				t.Fatalf("UsageRaw = %s, want nil when the usage key is literal null", res.UsageRaw)
			}
		})
	}
}

// TestParseTurn_UsageRawOnFailureFixtures 用三份真实失败样本把"全零是真实值"
// 与"根本没有这个键"分开钉住：claude / agy 的失败输出里 usage 键都在、五项
// 都是真实的 0（UsageRaw 非 nil，且逐字节等于那个真实的零值对象）；grok 的
// 错误信封里压根没有 usage 这个键（UsageRaw 必须是 nil）。
func TestParseTurn_UsageRawOnFailureFixtures(t *testing.T) {
	t.Run("claude_badmodel_usage_key_present_all_zero", func(t *testing.T) {
		p, _ := provider.Lookup("claude")
		res, err := p.ParseTurn([]byte(claudeBadModelJSON))
		if !errors.Is(err, provider.ErrTurnFailed) {
			t.Fatalf("err = %v, want ErrTurnFailed", err)
		}
		if res.UsageRaw == nil {
			t.Fatalf("UsageRaw = nil, want non-nil (usage 键在这份原件里存在，五项都是真实的 0)")
		}
		if want := rawUsageSubObject(t, claudeBadModelJSON); !bytes.Equal(res.UsageRaw, want) {
			t.Fatalf("UsageRaw = %s, want %s", res.UsageRaw, want)
		}
	})

	t.Run("agy_badmodel_usage_key_present_all_zero", func(t *testing.T) {
		p, _ := provider.Lookup("agy")
		res, err := p.ParseTurn([]byte(agyBadModelJSON))
		if !errors.Is(err, provider.ErrTurnFailed) {
			t.Fatalf("err = %v, want ErrTurnFailed", err)
		}
		if res.UsageRaw == nil {
			t.Fatalf("UsageRaw = nil, want non-nil (usage 键在这份原件里存在，五项都是真实的 0)")
		}
		if want := rawUsageSubObject(t, agyBadModelJSON); !bytes.Equal(res.UsageRaw, want) {
			t.Fatalf("UsageRaw = %s, want %s", res.UsageRaw, want)
		}
	})

	t.Run("grok_badmodel_no_usage_key", func(t *testing.T) {
		p, _ := provider.Lookup("grok")
		res, err := p.ParseTurn([]byte(grokBadModelJSON))
		if !errors.Is(err, provider.ErrTurnFailed) {
			t.Fatalf("err = %v, want ErrTurnFailed", err)
		}
		if res.UsageRaw != nil {
			t.Fatalf("UsageRaw = %s, want nil (这份错误信封里根本没有 usage 键)", res.UsageRaw)
		}
	})
}

// 纯函数性：TestParseTurn_IsPureFunction（本文件前半段）已经对整个 TurnResult
// 做 reflect.DeepEqual，UsageRaw（json.RawMessage，本质是 []byte）落地成为
// TurnResult 的字段后自动被那条比较覆盖，不需要另开一条用例。
