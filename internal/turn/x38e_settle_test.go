package turn

// 本文件是 X-38e 冻结契约第 2 节 A0-A4 的验收：/round 结算的 UsageUnknown
// 改成「ReportedUsage 五个指针全 nil」，与 makeTurnUsage 写库五列全 NULL、
// /cost 的 USAGE_UNKNOWN 同一个逐行谓词；人群仍是 Spawned 那道门。
//
// 背对背：本文件只按契约写，不依赖实现腿的任何新符号。每一条「新行为」断言
// 都配一条只差一处的对照（A1 对 A1c、A1s；A3 的 K1 对 K1c、K2），证明断言
// 不是结构上恒真。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
)

// x38eAsk 是 A0 的回放帮手：每次调用新建一套 recordingStore + Runner +
// fakeSpawner，回放一条 spawnResult，返回 Record 与写库的那一行 TurnUsage。
// Ask 的 error 不看（失败轮本来就返回 error），只看 Record。
func x38eAsk(t *testing.T, agentID string, sr spawnResult) (Record, conversation.TurnUsage) {
	t.Helper()
	rs := newRecordingStore()
	svc := conversation.NewService(rs)
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, agentID)
	postUser(t, svc, convID, "问题")
	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueue(sr)
	r.spawn = fs.spawn
	rec, _ := r.Ask(context.Background(), convID, agentID)
	if len(rs.recordUsageCalls) != 1 {
		t.Fatalf("x38eAsk(%s): RecordUsage 调用 %d 次，期望恰好 1 次", agentID, len(rs.recordUsageCalls))
	}
	return rec, rs.recordUsageCalls[0]
}

// x38eCols 把 TurnUsage 的五个可空用量列按 in / cc / cr / out / th 的顺序摊开。
func x38eCols(u conversation.TurnUsage) [5]*int {
	return [5]*int{u.InputTokens, u.CacheCreate, u.CacheRead, u.OutputTokens, u.ThinkingTokens}
}

// x38eColLabels 只用于失败消息。
var x38eColLabels = [5]string{"InputTokens", "CacheCreate", "CacheRead", "OutputTokens", "ThinkingTokens"}

// x38ePtrStr 把 *int 写成可读的形状，失败消息用。
func x38ePtrStr(p *int) string {
	if p == nil {
		return "NULL"
	}
	return fmt.Sprintf("&%d", *p)
}

// x38eAssertAllNull 是 A1 / A1s 的前提：写库五列全 NULL。不满足就 Fatal。
func x38eAssertAllNull(t *testing.T, u conversation.TurnUsage) {
	t.Helper()
	for i, p := range x38eCols(u) {
		if p != nil {
			t.Fatalf("前提不成立：TurnUsage.%s = %s，期望 NULL（五列全 NULL）", x38eColLabels[i], x38ePtrStr(p))
		}
	}
}

// x38eSpawnFailed 照 usage_unknown_test.go:190 的 SpawnFailed 输入。
func x38eSpawnFailed() spawnResult {
	return spawnResult{err: errors.New("exec: \"claude-fake\": executable file not found in %PATH%")}
}

// ---------------------------------------------------------------------------
// A1. 真回放：五个全未报的成员，UsageUnknown == 1（HEAD 红）
// ---------------------------------------------------------------------------

func TestX38eAskReplayCountsFiveUnreported(t *testing.T) {
	cases := []struct {
		name    string
		agent   string
		sr      func(t *testing.T) spawnResult
		outcome Outcome
	}{
		{"grok_envelope", testAgentGrok, func(t *testing.T) spawnResult {
			return spawnResult{stdout: []byte(grokBadModelOut), exitCode: 1}
		}, OutcomeProviderFailed},
		{"grok_no_usage", testAgentGrok, func(t *testing.T) spawnResult {
			return spawnResult{stdout: []byte(mustReplaceOnce(t, grokT1JSON, `"usage":`, `"x38e_no_usage":`)), exitCode: 0}
		}, OutcomeSpoke},
		{"claude_no_usage", testAgentClaude, func(t *testing.T) spawnResult {
			return spawnResult{stdout: []byte(mustReplaceOnce(t, claudeT1JSON, `"usage":`, `"x38e_no_usage":`)), exitCode: 0}
		}, OutcomeSpoke},
		{"agy_no_usage", testAgentAgy, func(t *testing.T) spawnResult {
			return spawnResult{stdout: []byte(mustReplaceOnce(t, agyT1JSON, `"usage":`, `"x38e_no_usage":`)), exitCode: 0}
		}, OutcomeSpoke},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, u := x38eAsk(t, c.agent, c.sr(t))
			if rec.Outcome != c.outcome {
				t.Fatalf("前提不成立：Outcome = %d，期望 %d", rec.Outcome, c.outcome)
			}
			if !rec.UsageParsed {
				t.Fatalf("前提不成立：UsageParsed = false，期望 true（这一档正是旧判据漏数的人）")
			}
			x38eAssertAllNull(t, u)

			s := Settle([]Record{rec})
			if s.Spawned != 1 {
				t.Errorf("Spawned = %d，期望 1", s.Spawned)
			}
			if s.SpawnFailed != 0 {
				t.Errorf("SpawnFailed = %d，期望 0", s.SpawnFailed)
			}
			if s.UsageUnknown != 1 {
				t.Errorf("UsageUnknown = %d，期望 1（库里五列全 NULL，/cost 数它为 USAGE_UNKNOWN）", s.UsageUnknown)
			}
			if s.ColdUnknown != 1 {
				t.Errorf("ColdUnknown = %d，期望 1（冷暖判定必须先于用量未知那一支）", s.ColdUnknown)
			}
			if s.HaveIn {
				t.Errorf("HaveIn = true，期望 false（五个全未报的成员一个数都不该加进合计）")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A1c. 真回放对照：部分未报与真 0 不算（HEAD 绿）
// ---------------------------------------------------------------------------

func TestX38eAskReplayControlsNotCounted(t *testing.T) {
	p := x38aIntPtr
	cases := []struct {
		name        string
		agent       string
		sr          spawnResult
		cols        [5]*int
		coldUnknown int
	}{
		{"grok_normal", testAgentGrok, spawnResult{stdout: []byte(grokT1JSON), exitCode: 0},
			[5]*int{p(20625), nil, p(0), p(723), p(722)}, 0},
		{"agy_normal", testAgentAgy, spawnResult{stdout: []byte(agyT1JSON), exitCode: 0},
			[5]*int{p(18180), nil, p(0), p(55), p(54)}, 0},
		{"agy_badmodel", testAgentAgy, spawnResult{stdout: []byte(agyBadModelOut), exitCode: 1},
			[5]*int{p(0), nil, p(0), p(0), p(0)}, 1},
		{"claude_normal", testAgentClaude, spawnResult{stdout: []byte(claudeT1JSON), exitCode: 0},
			[5]*int{p(47751), p(47749), p(0), p(4), p(0)}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, u := x38eAsk(t, c.agent, c.sr)
			got := x38eCols(u)
			for i := range got {
				g, w := got[i], c.cols[i]
				if (g == nil) != (w == nil) || (g != nil && *g != *w) {
					t.Fatalf("前提不成立：TurnUsage.%s = %s，期望 %s", x38eColLabels[i], x38ePtrStr(g), x38ePtrStr(w))
				}
			}
			s := Settle([]Record{rec})
			if s.Spawned != 1 {
				t.Errorf("Spawned = %d，期望 1", s.Spawned)
			}
			if s.UsageUnknown != 0 {
				t.Errorf("UsageUnknown = %d，期望 0（至少一列有数，不是用量未知）", s.UsageUnknown)
			}
			if s.ColdUnknown != c.coldUnknown {
				t.Errorf("ColdUnknown = %d，期望 %d", s.ColdUnknown, c.coldUnknown)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A1s. 真回放：SpawnFailed 五列全 NULL 也不算（HEAD 绿）
// ---------------------------------------------------------------------------

func TestX38eAskReplaySpawnFailedNotCounted(t *testing.T) {
	rec, u := x38eAsk(t, testAgentClaude, x38eSpawnFailed())
	if rec.Outcome != OutcomeSpawnFailed {
		t.Fatalf("前提不成立：Outcome = %d，期望 OutcomeSpawnFailed(%d)", rec.Outcome, OutcomeSpawnFailed)
	}
	if rec.UsageParsed {
		t.Fatalf("前提不成立：UsageParsed = true，期望 false")
	}
	x38eAssertAllNull(t, u)

	s := Settle([]Record{rec})
	if s.Spawned != 0 {
		t.Errorf("Spawned = %d，期望 0", s.Spawned)
	}
	if s.SpawnFailed != 1 {
		t.Errorf("SpawnFailed = %d，期望 1", s.SpawnFailed)
	}
	if s.UsageUnknown != 0 {
		t.Errorf("UsageUnknown = %d，期望 0（排除 SpawnFailed 的是人群门，不是逐行谓词）", s.UsageUnknown)
	}
	if s.ColdUnknown != 0 {
		t.Errorf("ColdUnknown = %d，期望 0", s.ColdUnknown)
	}
}

// ---------------------------------------------------------------------------
// A2. 混合轮：round_finished 整串冻结（HEAD 红）
// ---------------------------------------------------------------------------

func TestX38eRoundFinishedMixedRound(t *testing.T) {
	grokNormal, _ := x38eAsk(t, testAgentGrok, spawnResult{stdout: []byte(grokT1JSON), exitCode: 0})
	grokNoUsage, _ := x38eAsk(t, testAgentGrok, spawnResult{stdout: []byte(mustReplaceOnce(t, grokT1JSON, `"usage":`, `"x38e_no_usage":`)), exitCode: 0})
	grokEnvelope, _ := x38eAsk(t, testAgentGrok, spawnResult{stdout: []byte(grokBadModelOut), exitCode: 1})
	agyBad, _ := x38eAsk(t, testAgentAgy, spawnResult{stdout: []byte(agyBadModelOut), exitCode: 1})
	claudeSF, _ := x38eAsk(t, testAgentClaude, x38eSpawnFailed())
	recs := []Record{grokNormal, grokNoUsage, grokEnvelope, agyBad, claudeSF}

	extra, text := roundFinishedMeta("rid-x38e-mixed", recs)

	// 断言顺序照契约：rlMetaInt 取不到键就 Fatal，键名被改也会红。
	if got := rlMetaInt(t, extra, "usage_unknown"); got != 2 {
		t.Errorf("Meta usage_unknown = %d，期望 2（grok 去 usage 与 grok 错误信封）", got)
	}
	if got := rlMetaInt(t, extra, "cold_unknown"); got != 3 {
		t.Errorf("Meta cold_unknown = %d，期望 3", got)
	}
	if got := rlMetaInt(t, extra, "spawned"); got != 4 {
		t.Errorf("Meta spawned = %d，期望 4", got)
	}
	if got := rlMetaInt(t, extra, "spawn_failed"); got != 1 {
		t.Errorf("Meta spawn_failed = %d，期望 1", got)
	}
	if got := rlMetaInt(t, extra, "input_tokens"); got != 20625 {
		t.Errorf("Meta input_tokens = %d，期望 20625", got)
	}

	const want = "一轮结束 round_id=rid-x38e-mixed spawned=4 spawn_failed=1 usage_unknown=2 in=20625 out=723 cache_read=0 cache_create=? thinking=722 cost_usd=? cost_unreported=4 cold=1 cold_unknown=3"
	if text != want {
		t.Errorf("round_finished 文本不符\n实际: %s\n期望: %s", text, want)
	}
}

// ---------------------------------------------------------------------------
// A3 / A4 共用：网格记录与判据
// ---------------------------------------------------------------------------

// x38eGridRec 造一条网格记录：mask 第 i 位为 1 表示 x38aNames[i] 进 Unreported。
func x38eGridRec(oc Outcome, parsed bool, mask int, cc *int) Record {
	var unrep []string
	for i := 0; i < 5; i++ {
		if mask&(1<<i) != 0 {
			unrep = append(unrep, x38aNames[i])
		}
	}
	return Record{
		Outcome:     oc,
		UsageParsed: parsed,
		Usage: provider.Usage{
			InputTokens:    11,
			CacheCreate:    cc,
			CacheRead:      33,
			OutputTokens:   44,
			ThinkingTokens: 55,
			Unreported:     unrep,
		},
	}
}

// x38eCCShapes 是 CacheCreate 的三种形状；每次调用铸新指针，行与行之间不共享。
func x38eCCShapes() []*int {
	return []*int{nil, x38aIntPtr(0), x38aIntPtr(22)}
}

func x38eCCStr(cc *int) string { return x38ePtrStr(cc) }

// x38ePop 是人群门的显式 switch，不调 roundSpawned。
func x38ePop(oc Outcome) bool {
	switch oc {
	case OutcomeRefused, OutcomeUnknown, OutcomeSpawnFailed:
		return false
	default:
		return true
	}
}

// x38eAllNull 用写库那一侧（makeTurnUsage）当判据，不用 ReportedUsage / Settle。
func x38eAllNull(rec Record) bool {
	u := makeTurnUsage(&rec, nil, time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	return u.InputTokens == nil && u.CacheCreate == nil && u.CacheRead == nil && u.OutputTokens == nil && u.ThinkingTokens == nil
}

func x38eWant(rec Record) int {
	if x38ePop(rec.Outcome) && x38eAllNull(rec) {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// A3. 与写库结构绑定的扫描（HEAD 红）
// ---------------------------------------------------------------------------

func TestX38eSettleUsageUnknownMatchesStoreSweep(t *testing.T) {
	// 点名行：循环外各单独构造一次，杀手行不会被网格改动悄悄删掉。
	named := []struct {
		name    string
		rec     Record
		want    int
		allNull bool // 只对 K2 断言
		checkAN bool
	}{
		{"K1 Spoke/parsed/mask29/cc=nil", x38eGridRec(OutcomeSpoke, true, 29, nil), 1, false, false},
		{"K1c Spoke/parsed/mask29/cc=&0", x38eGridRec(OutcomeSpoke, true, 29, x38aIntPtr(0)), 0, false, false},
		{"K2 SpawnFailed/unparsed/mask0/cc=nil", x38eGridRec(OutcomeSpawnFailed, false, 0, nil), 0, true, true},
		{"K3 Spoke/unparsed/mask0/cc=&22", x38eGridRec(OutcomeSpoke, false, 0, x38aIntPtr(22)), 1, false, false},
	}
	for _, k := range named {
		if k.checkAN && x38eAllNull(k.rec) != k.allNull {
			t.Fatalf("%s：前提不成立，makeTurnUsage 五列全 NULL = %v，期望 %v", k.name, x38eAllNull(k.rec), k.allNull)
		}
		if w := x38eWant(k.rec); w != k.want {
			t.Fatalf("%s：夹具与判据不一致，判据给 %d，点名期望 %d", k.name, w, k.want)
		}
		if got := Settle([]Record{k.rec}).UsageUnknown; got != k.want {
			t.Errorf("%s：UsageUnknown = %d，期望 %d", k.name, got, k.want)
		}
	}

	n := 0
	var wantParsed, wantUnparsed, zeroInPop, nullOutPop int
	bad := 0
	for _, oc := range x36abAllOutcomes {
		for _, parsed := range []bool{false, true} {
			for mask := 0; mask < 32; mask++ {
				for _, cc := range x38eCCShapes() {
					n++
					rec := x38eGridRec(oc, parsed, mask, cc)
					pop := x38ePop(oc)
					allNull := x38eAllNull(rec)
					want := 0
					if pop && allNull {
						want = 1
					}
					switch {
					case want == 1 && parsed:
						wantParsed++
					case want == 1 && !parsed:
						wantUnparsed++
					}
					if want == 0 && pop {
						zeroInPop++
					}
					if allNull && !pop {
						nullOutPop++
					}
					if got := Settle([]Record{rec}).UsageUnknown; got != want {
						bad++
						t.Errorf("结局=%s parsed=%v Unreported=%v CacheCreate=%s：UsageUnknown = %d，期望 %d",
							x36abOutcomeName[oc], parsed, rec.Usage.Unreported, x38eCCStr(cc), got, want)
					}
				}
			}
		}
	}
	if n != 1920 {
		t.Fatalf("网格组数 = %d，期望 1920", n)
	}
	if bad > 0 {
		t.Logf("A3 不符组数 %d / %d", bad, n)
	}
	if wantParsed < 1 {
		t.Errorf("防退化：want==1 且已解析的组数 = %d，期望 ≥ 1", wantParsed)
	}
	if wantUnparsed < 1 {
		t.Errorf("防退化：want==1 且未解析的组数 = %d，期望 ≥ 1", wantUnparsed)
	}
	if zeroInPop < 1 {
		t.Errorf("防退化：want==0 且在人群内的组数 = %d，期望 ≥ 1", zeroInPop)
	}
	if nullOutPop < 1 {
		t.Errorf("防退化：五列全 NULL 且不在人群内的组数 = %d，期望 ≥ 1", nullOutPop)
	}
}

// ---------------------------------------------------------------------------
// A4. 不变量扫描：UsageUnknown ≤ ColdUnknown，含五个全未报（HEAD 红）
// ---------------------------------------------------------------------------

func TestX38eUsageUnknownWithinColdUnknownSweep(t *testing.T) {
	n := 0
	parsedCounted := 0
	for _, oc := range x36abAllOutcomes {
		for _, parsed := range []bool{false, true} {
			for mask := 0; mask < 32; mask++ {
				for _, cc := range x38eCCShapes() {
					for _, cold := range []bool{false, true} {
						n++
						rec := x38eGridRec(oc, parsed, mask, cc)
						rec.Cold = cold
						s := Settle([]Record{rec})
						_, mk := rec.MeasuredCold()
						tag := func() string {
							return fmt.Sprintf("结局=%s parsed=%v Unreported=[%s] CacheCreate=%s Cold=%v",
								x36abOutcomeName[oc], parsed, strings.Join(rec.Usage.Unreported, ","), x38eCCStr(cc), cold)
						}
						if s.UsageUnknown > s.Spawned {
							t.Errorf("%s：UsageUnknown=%d > Spawned=%d", tag(), s.UsageUnknown, s.Spawned)
						}
						if s.Spawned == 1 {
							if s.UsageUnknown > s.ColdUnknown {
								t.Errorf("%s：UsageUnknown=%d > ColdUnknown=%d", tag(), s.UsageUnknown, s.ColdUnknown)
							}
							k := 0
							if mk {
								k = 1
							}
							if s.ColdUnknown+k != s.Spawned {
								t.Errorf("%s：ColdUnknown=%d + 冷暖确知=%d != Spawned=%d", tag(), s.ColdUnknown, k, s.Spawned)
							}
						}
						if s.UsageUnknown == 1 && parsed {
							parsedCounted++
						}
					}
				}
			}
		}
	}
	if n != 3840 {
		t.Fatalf("网格组数 = %d，期望 3840", n)
	}
	if parsedCounted < 1 {
		t.Errorf("防退化：UsageUnknown==1 且已解析的组数 = %d，期望 ≥ 1（新计入的那类人必须被扫到）", parsedCounted)
	}
}
