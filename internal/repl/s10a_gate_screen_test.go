package repl

// 本文件是 Step 10 A 之五（契约 s10a5 v1.1 §2.3）的 repl 包验收：
// 别人的在飞记录被拒时屏幕上的那句话（/ask 与 /round），以及 /session 的 BASELINE 列。
// 只用 HEAD 已有的标识符：本刀新增的哨兵与字段一个都不引用，屏幕与库是唯一判据。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/store"
	"council/internal/turn"
)

// s10aRefusalSentence 逐字复制契约 §3.1。
const s10aRefusalSentence = "这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行"

// s10aForeignInflight 按契约 §2.0 造一条「别人的在飞记录」（repl 用例一律 spawned）。
func s10aForeignInflight(convID, agentID string) conversation.InflightTurn {
	return conversation.InflightTurn{
		TurnID:           "s10a-stale-turn",
		ConvID:           convID,
		AgentID:          agentID,
		Provider:         "claude",
		PlannedSessionID: "s10a-planned",
		ResumeSessionID:  "",
		CursorSeq:        0,
		ScannedTo:        1,
		StartedAt:        time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Phase:            conversation.PhaseSpawned,
		Tier:             "read",
		WorkDir:          "s10a-wd",
	}
}

// s10aSeedInFlightConv 种一个会话、ag_wait 这个成员与一条种子消息；foreign 为真时再写入别人的在飞记录。
func s10aSeedInFlightConv(t *testing.T, svc *conversation.Service, f *fakeStore, convID string, foreign bool) {
	t.Helper()
	f.seedConversation(convID, "s10a")
	f.seedParticipant(t, convID, inFlightAgentID)
	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "seed-msg")
	if foreign {
		if err := svc.BeginTurn(context.Background(), s10aForeignInflight(convID, inFlightAgentID)); err != nil {
			t.Fatalf("前提不成立：写入别人的在飞记录失败: %v", err)
		}
	}
}

// s10aHasAdjacentLines 判断 lines 里是否存在下标 i，使第 i 行整行等于 a、第 i+1 行整行等于 b。
func s10aHasAdjacentLines(lines []string, a, b string) bool {
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] == a && lines[i+1] == b {
			return true
		}
	}
	return false
}

// s10aHasLine 判断 lines 里是否有整行等于 want 的一行。
func s10aHasLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

// TestS10aAskCauseSameSentenceForBothProducers 是 R1：两个产出方、同一句话。
//
// 【判别力】产出方一：store 不译哨兵（§4 #6）时 askCause 落进 default，印出裸 SQLite
// 文本而红；产出方二：E1 失效（§4 #1）时 repl 的 fakeStore 不强制唯一索引，Ask 会起假
// CLI 并成功，e2 为 nil 而 Fatalf；askCause 新分支失效（§4 #9）时两个产出方都红。
// 对照 askCause(errors.New("s10a-plain")) 证明 default 分支原样返回、不是恒返回那句话。
func TestS10aAskCauseSameSentenceForBothProducers(t *testing.T) {
	ctx := context.Background()

	// 产出方一：真库，BeginTurn 撞唯一索引。
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s10a-data"))
	if err != nil {
		t.Fatalf("store.Open 失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭 store 失败: %v", err)
		}
	})
	realSvc := conversation.NewService(st)
	conv, err := realSvc.Create(ctx, "s10a")
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	mk := func(turnID string) conversation.InflightTurn {
		return conversation.InflightTurn{TurnID: turnID, ConvID: conv.ID, AgentID: "s10a-g", Provider: "claude", Phase: conversation.PhaseBegun}
	}
	if err := realSvc.BeginTurn(ctx, mk("s10a-t1")); err != nil {
		t.Fatalf("前提不成立：第一次 BeginTurn 失败: %v", err)
	}
	e1 := realSvc.BeginTurn(ctx, mk("s10a-t2"))
	if e1 == nil {
		t.Fatalf("前提不成立：同一 (conv, agent) 第二次 BeginTurn 应当报错")
	}
	if got := askCause(e1); got != s10aRefusalSentence {
		t.Errorf("产出方一：askCause = %q，期望 §3.1 那句话", got)
	}

	// 产出方二：起进程之前的检查。
	svc, f := newTestService(t)
	rn := newInFlightRunner(t, svc)
	const convID = "conv_s10a_r1_producer2_000000000000"
	s10aSeedInFlightConv(t, svc, f, convID, true)
	_, e2 := rn.Ask(ctx, convID, inFlightAgentID)
	if e2 == nil {
		t.Fatalf("产出方二：Ask 返回 nil 错误，期望被拒（起进程之前的检查没有生效）")
	}
	if got := askCause(e2); got != s10aRefusalSentence {
		t.Errorf("产出方二：askCause = %q，期望 §3.1 那句话", got)
	}

	// 对照：认不出哨兵时原样返回。
	if got := askCause(errors.New("s10a-plain")); got != "s10a-plain" {
		t.Errorf("对照：askCause(s10a-plain) = %q，期望原样", got)
	}
}

// TestS10aAskScreenForeignInflight 是 R2：/ask 屏幕上的那两行。
//
// 【判别力】foreign：E1 失效（§4 #1）时 fakeStore 不强制唯一索引，假 CLI 真的起了、
// 印 outcome spoke，refused 那一行不存在而红，f.msgs 也会多出面包屑与回复；askCause
// 新分支失效（§4 #9）时 refused 之后那一行是错误原文而红。control 是对照：同一命令、
// 不写外来行时照常 spoke，且屏幕上没有那句话（防恒印）。
// 「没有任何一行以 用量 / 金额 / 冷暖 开头」是契约写明的前缀判据：refused 不打用量行。
func TestS10aAskScreenForeignInflight(t *testing.T) {
	t.Run("foreign", func(t *testing.T) {
		svc, f := newTestService(t)
		rn := newInFlightRunner(t, svc)
		const convID = "conv_s10a_r2_foreign_00000000000000"
		s10aSeedInFlightConv(t, svc, f, convID, true)

		out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+inFlightAgentID+"\n")
		if err != nil {
			t.Fatalf("Run 失败: %v；输出:\n%s", err, out)
		}
		lines := strings.Split(out, "\n")
		if !s10aHasAdjacentLines(lines, "outcome refused  agent "+inFlightAgentID, s10aRefusalSentence) {
			t.Errorf("没有找到整行 %q 紧跟整行 §3.1 那句话；完整输出:\n%s", "outcome refused  agent "+inFlightAgentID, out)
		}
		for _, l := range lines {
			if strings.HasPrefix(l, "用量") || strings.HasPrefix(l, "金额") || strings.HasPrefix(l, "冷暖") {
				t.Errorf("被拒的一轮不该打用量 / 金额 / 冷暖行，实际有 %q；完整输出:\n%s", l, out)
			}
		}
		f.mu.Lock()
		n := len(f.msgs[convID])
		f.mu.Unlock()
		if n != 1 {
			t.Errorf("messages 条数 = %d，期望 1（只有种子消息，没有 session_opening、没有回复）", n)
		}
	})
	t.Run("control", func(t *testing.T) {
		svc, f := newTestService(t)
		rn := newInFlightRunner(t, svc)
		const convID = "conv_s10a_r2_control_00000000000000"
		s10aSeedInFlightConv(t, svc, f, convID, false)

		out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+inFlightAgentID+"\n")
		if err != nil {
			t.Fatalf("Run 失败: %v；输出:\n%s", err, out)
		}
		lines := strings.Split(out, "\n")
		if !s10aHasLine(lines, "outcome spoke  agent "+inFlightAgentID) {
			t.Errorf("没有整行 %q；完整输出:\n%s", "outcome spoke  agent "+inFlightAgentID, out)
		}
		if s10aHasLine(lines, s10aRefusalSentence) {
			t.Errorf("没有外来行时屏幕上不该出现 §3.1 那句话；完整输出:\n%s", out)
		}
	})
}

// TestS10aRoundScreenForeignInflight 是 R3：/round 下被拒成员的两行，另一成员照常。
//
// 【判别力】E1 失效（§4 #1）时 s10a_a 也会起假 CLI 并 spoke，refused 那一行不存在而红；
// askCause 新分支失效（§4 #9）时第 i+1 行是 "/round s10a_a: " 加错误原文而红。
// s10a_b 那一行是对照：一人被拒不取消别人。不断言两人先后（完成顺序不定）。
func TestS10aRoundScreenForeignInflight(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	agent := func(display string) config.AgentConfig {
		return config.AgentConfig{
			DisplayName: display,
			Provider:    "claude",
			Model:       "claude-fable-5-1",
			Effort:      "high",
			Role:        "s10a",
			Permissions: config.PermissionRead,
			Mode:        config.ModeDiscussion,
		}
	}
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{"claude": {Command: os.Args[0]}},
		Agents: map[string]config.AgentConfig{
			"s10a_a": agent("A"),
			"s10a_b": agent("B"),
		},
	}
	svc, f := newTestService(t)
	rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})

	const convID = "conv_s10a_r3_round_0000000000000000"
	f.seedConversation(convID, "s10a")
	f.seedParticipant(t, convID, "s10a_a")
	f.seedParticipant(t, convID, "s10a_b")
	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "seed-msg")
	if err := svc.BeginTurn(context.Background(), s10aForeignInflight(convID, "s10a_a")); err != nil {
		t.Fatalf("前提不成立：写入别人的在飞记录失败: %v", err)
	}

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round s10a_a s10a_b --go\n")
	if err != nil {
		t.Fatalf("Run 失败: %v；输出:\n%s", err, out)
	}
	lines := strings.Split(out, "\n")
	if !s10aHasAdjacentLines(lines, "outcome refused  agent s10a_a", "/round s10a_a: "+s10aRefusalSentence) {
		t.Errorf("没有找到整行 %q 紧跟整行 %q；完整输出:\n%s", "outcome refused  agent s10a_a", "/round s10a_a: "+s10aRefusalSentence, out)
	}
	if !s10aHasLine(lines, "outcome spoke  agent s10a_b") {
		t.Errorf("没有整行 %q（一人被拒不该取消别人）；完整输出:\n%s", "outcome spoke  agent s10a_b", out)
	}
}

// s10aFieldsFrom 把 line 按空白切开，从第一个等于 first 的字段起截到末尾。
func s10aFieldsFrom(line, first string) []string {
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == first {
			return fields[i:]
		}
	}
	return nil
}

// TestS10aSessionBaselineColumn 是 R4：/session 的 BASELINE 列。
//
// 【判别力】ag_alpha 与 ag_mid 只差 BaselineSuspect 一处：恒印 ok（§4 #15）时 ag_alpha
// 红；fresh 行仍是四格（#16）时 ag_zeta 红；表头没加（#17）时表头红；新列放在 NEXT 之后
// （#18）时表头与各行都红。ag_wait 是「在飞但基线不可疑」：InFlight 时一律印 suspect
// （#23）只让它这一格红，它与 ag_alpha 互为对照。s10a_wsus 是「在飞且基线可疑」：
// BASELINE 格只在不在飞时才看 BaselineSuspect（审查变异 P5：条件写成
// info.BaselineSuspect && !info.InFlight）时，只有它这一格从 suspect 变 ok 而红；
// 它与 ag_wait 只差库里 baseline_suspect 一处，互为对照，与 ag_alpha 只差在不在飞。
// 时序沿用 TestSessionNextColumnBusyDuringInFlightThenWarm 的 700ms 睡眠 / 350ms
// 探测窗口：两个在飞成员的第二轮在同一次 inFlightProbeDelay 之前一起起，探测落在
// 两段睡眠的共同窗口里，不另起时序。
func TestS10aSessionBaselineColumn(t *testing.T) {
	ctx := context.Background()
	svc, f := newTestService(t)
	// 与 newInFlightRunner 同一套做法，只是配置里多一个与 ag_wait 同形的成员。
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	const suspectBusyID = "s10a_wsus"
	inCfg := newInFlightConfig()
	inCfg.Agents[suspectBusyID] = inCfg.Agents[inFlightAgentID]
	rn := turn.NewRunner(inCfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})

	const convID = "conv_s10a_r4_session_000000000000000"
	f.seedConversation(convID, "s10a")
	for _, id := range []string{"ag_alpha", "ag_mid", "ag_zeta", inFlightAgentID, suspectBusyID} {
		f.seedParticipant(t, convID, id)
	}
	lt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f.seedProviderSession(conversation.ProviderSession{
		ConvID: convID, AgentID: "ag_alpha", ProviderSessionID: "sess-alpha", CursorSeq: 1,
		LastTurnAt: lt, IdentityFP: "fp", Spoken: true, BaselineSuspect: true,
	})
	f.seedProviderSession(conversation.ProviderSession{
		ConvID: convID, AgentID: "ag_mid", ProviderSessionID: "sess-mid", CursorSeq: 1,
		LastTurnAt: lt, IdentityFP: "fp", Spoken: true, BaselineSuspect: false,
	})

	// ag_wait：先成功一轮，再让第二轮在飞。
	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "seed-msg")
	first, err := rn.Ask(ctx, convID, inFlightAgentID)
	if err != nil || first.Outcome != turn.OutcomeSpoke {
		t.Fatalf("前提不成立：ag_wait 首轮应当成功，err=%v outcome=%v", err, first.Outcome)
	}
	ps, found, err := svc.ProviderSession(ctx, convID, inFlightAgentID)
	if err != nil || !found || ps.BaselineSuspect {
		t.Fatalf("前提不成立：ag_wait 首轮之后 found=%v err=%v BaselineSuspect=%v，期望有行、基线不可疑", found, err, ps.BaselineSuspect)
	}
	// s10a_wsus：同样先成功一轮（Runner 内存里 g.st 有了、指纹对得上，第二轮会 --resume
	// 进 700ms 睡眠），再把库里那一行改成基线可疑。内存里仍是 false，不影响第二轮起进程；
	// 在飞时 SessionInfo 拿不到闸，读的正是库里这一行。
	sfirst, err := rn.Ask(ctx, convID, suspectBusyID)
	if err != nil || sfirst.Outcome != turn.OutcomeSpoke {
		t.Fatalf("前提不成立：%s 首轮应当成功，err=%v outcome=%v", suspectBusyID, err, sfirst.Outcome)
	}
	sps, found, err := svc.ProviderSession(ctx, convID, suspectBusyID)
	if err != nil || !found {
		t.Fatalf("前提不成立：%s 首轮之后 found=%v err=%v，期望有行", suspectBusyID, found, err)
	}
	sps.BaselineSuspect = true
	f.seedProviderSession(sps)
	if got, found, err := svc.ProviderSession(ctx, convID, suspectBusyID); err != nil || !found || !got.BaselineSuspect {
		t.Fatalf("前提不成立：%s 改完之后 found=%v err=%v BaselineSuspect=%v，期望有行、基线可疑", suspectBusyID, found, err, got.BaselineSuspect)
	}
	f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "second-msg")

	var wg sync.WaitGroup
	var secondErr, suspectSecondErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, secondErr = rn.Ask(context.Background(), convID, inFlightAgentID)
	}()
	go func() {
		defer wg.Done()
		_, suspectSecondErr = rn.Ask(context.Background(), convID, suspectBusyID)
	}()
	t.Cleanup(wg.Wait)

	time.Sleep(inFlightProbeDelay)
	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/session\n")
	wg.Wait()
	if err != nil {
		t.Fatalf("Run 失败: %v；输出:\n%s", err, out)
	}
	if secondErr != nil {
		t.Fatalf("前提不成立：ag_wait 第二轮失败: %v", secondErr)
	}
	if suspectSecondErr != nil {
		t.Fatalf("前提不成立：%s 第二轮失败: %v", suspectBusyID, suspectSecondErr)
	}

	header := s10aFieldsFrom(lineContaining(t, out, "PROVIDER_SESSION"), "AGENT")
	if want := []string{"AGENT", "PROVIDER_SESSION", "LAST_TURN_AT", "BASELINE", "NEXT"}; !reflect.DeepEqual(header, want) {
		t.Errorf("表头 = %q，期望 %q；完整输出:\n%s", header, want, out)
	}
	cases := []struct {
		agent string
		want  []string
	}{
		{"ag_alpha", []string{"ag_alpha", "sess-alpha", "2026-01-02T03:04:05Z", "suspect", "cold"}},
		{"ag_mid", []string{"ag_mid", "sess-mid", "2026-01-02T03:04:05Z", "ok", "cold"}},
		{"ag_zeta", []string{"ag_zeta", "-", "-", "-", "fresh"}},
		{inFlightAgentID, []string{inFlightAgentID, "2a90d449-a348-497a-8812-e73e74d6898e", ps.LastTurnAt.UTC().Format(time.RFC3339), "ok", "busy"}},
		{suspectBusyID, []string{suspectBusyID, sps.ProviderSessionID, sps.LastTurnAt.UTC().Format(time.RFC3339), "suspect", "busy"}},
	}
	for _, c := range cases {
		if got := strings.Fields(lineContaining(t, out, c.agent)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s 那一行 = %q，期望 %q；完整输出:\n%s", c.agent, got, c.want, out)
		}
	}
}
