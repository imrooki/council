package repl

// u43_rebuild_settle_test.go 是 U43A 契约 §2.2 的验收：/session rebuild 被 A 之五
// 的闸拒绝之后，经与启动对账同一条路结清陈旧的在飞记录，再丢 provider 会话。
//
// 全部用真 SQLite（store.Open 到 t.TempDir() 之下）、真 turn.Runner、假 CLI
// （session_inflight_test.go 的 TestMain：测试二进制自己扮 claude）。本文件只用
// HEAD 已有的标识符，所以能单独放进 HEAD 的拷贝里跑，看它在修之前红在哪里。
// 不新写 TestMain，不用 repl 包的 fakeStore。
//
// 屏幕的切行规则（契约 §2 通用纪律，冻结）：strings.Split(out, "\n")，每一行反复
// 剥掉前缀 "council> " 直到没有；「整段等于 [X, Y]」一律指
// ["当前会话: <convID>，标题: u43", X, Y, "", "再见", ""]。

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"
	"council/internal/turn"
)

// 契约 §3.1：本刀新增的五个格式串，逐字节。
const (
	u43FmtSettled      = "已结清 %s 的一轮在飞记录（turn %s，phase %s），对它 /ask 不会再因为这条记录被拒\n"
	u43LineLost        = "那一轮记为丢失：可能花了钱，用量与金额都不知道（与重启时启动对账的记法相同）\n"
	u43LineBegunNoCost = "那一轮没起过进程、没花钱，只删掉了这条记录\n"
	u43FmtSettleFail   = "结清 %s 的在飞记录（turn %s）失败，什么都没改，provider 会话也没丢；对它 /ask 仍会被拒: %v\n"
	u43FmtReadFail     = "读不出 %s 的在飞记录，什么都没改，provider 会话也没丢: %v\n"
)

// 契约 §3.2：本刀复用、要比对的既有文本。
const (
	u43FmtBusy       = "%s 此刻正有一轮在跑，什么都没丢；等这一轮跑完再敲一次\n"
	u43LineNoSession = "本来就没有开着的会话，下一轮照常是首轮\n"
	u43FmtDropped    = "已丢弃 %s 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢），下一轮按首轮计价\n"
	u43FmtNotMember  = "%s 不在当前会话的活跃成员里，用 /who 看看有哪些\n"
	u43LineRefusal   = "这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行"
	u43FmtOutcome    = "outcome %s  agent %s"
	u43FmtAsking     = "正在问 %s ..."
)

// u43StaleTurnID 是本文件陈旧行一律用的 turn id（契约 §2.2 共同前置）。
const u43StaleTurnID = "u43-stale-turn"

// u43FakeSessionID 是假 CLI 回放的 claude 会话 id（fakeClaudeSuccessJSON 里的 session_id）。
const u43FakeSessionID = "2a90d449-a348-497a-8812-e73e74d6898e"

// u43ConvTitle 是会话标题，冻结为 "u43"。
const u43ConvTitle = "u43"

// u43PhaseSet 是契约 §2.0 的 phase 全集：三个合法值加一个 schema 放得进来的非法值。
var u43PhaseSet = []conversation.InflightPhase{
	conversation.PhaseBegun,
	conversation.PhaseSpawned,
	conversation.PhaseReturned,
	conversation.InflightPhase("u43-bogus"),
}

// u43ErrInjectedRead 是 R4 注入的读失败。
var u43ErrInjectedRead = errors.New("u43 injected read failure")

// u43FailReadStore 只重写 InflightFor，恒返回 u43ErrInjectedRead，其余透传真库。
type u43FailReadStore struct {
	*store.Store
}

func (s u43FailReadStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	return conversation.InflightTurn{}, false, u43ErrInjectedRead
}

// u43Env 是一份真库装置：store、Service、会话 id、查库用的另一条连接。
type u43Env struct {
	st   *store.Store
	svc  *conversation.Service
	conv string
	db   *sql.DB
}

// u43NewEnv 开真库、建会话（标题 u43）；join 为真时把 ag_wait 加成成员。
func u43NewEnv(t *testing.T, join bool) u43Env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "u43-data"))
	if err != nil {
		t.Fatalf("前提不成立：store.Open 失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试 store 失败: %v", err)
		}
	})
	svc := conversation.NewService(st)
	conv, err := svc.Create(ctx, u43ConvTitle)
	if err != nil {
		t.Fatalf("前提不成立：建会话失败: %v", err)
	}
	if join {
		if err := svc.Join(ctx, conv.ID, conversation.Participant{AgentID: inFlightAgentID, Provider: "claude", JoinedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("前提不成立：Join 失败: %v", err)
		}
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(st.Path())+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("前提不成立：打开查库连接失败: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭查库连接失败: %v", err)
		}
	})
	return u43Env{st: st, svc: svc, conv: conv.ID, db: db}
}

// u43Post 以保留 sender user 发一条用户消息。
func u43Post(t *testing.T, e u43Env, text string) {
	t.Helper()
	if _, err := e.svc.Post(context.Background(), conversation.Message{
		ConvID:   e.conv,
		SenderID: conversation.ReservedSenderUser,
		Kind:     conversation.KindUser,
		Text:     text,
	}); err != nil {
		t.Fatalf("前提不成立：Post(%q) 失败: %v", text, err)
	}
}

// u43OpenSession 是「开口过」：发 seed-msg，再让 ag_wait 成功说一轮（假 CLI 首轮立刻回）。
func u43OpenSession(t *testing.T, e u43Env, rn *turn.Runner) {
	t.Helper()
	u43Post(t, e, "seed-msg")
	rec, err := rn.Ask(context.Background(), e.conv, inFlightAgentID)
	if err != nil || rec.Outcome != turn.OutcomeSpoke {
		t.Fatalf("前提不成立：开口那一轮应当 spoke，err=%v outcome=%v", err, rec.Outcome)
	}
}

// u43StaleRow 按契约 §2.0 组一条陈旧的在飞记录（repl 侧：PlannedSessionID 为空、ScannedTo 为 1）。
func u43StaleRow(conv string, phase conversation.InflightPhase, resume string) conversation.InflightTurn {
	return conversation.InflightTurn{
		TurnID:           u43StaleTurnID,
		ConvID:           conv,
		AgentID:          inFlightAgentID,
		Provider:         "claude",
		PlannedSessionID: "",
		ResumeSessionID:  resume,
		CursorSeq:        1,
		ScannedTo:        1,
		StartedAt:        time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Phase:            phase,
		Tier:             "read",
		WorkDir:          "u43-wd",
	}
}

// u43SeedStale 写一条陈旧行：合法 phase 走 BeginTurn，非法 phase 走裸 SQL（契约 §2.0 冻结的语句）。
func u43SeedStale(t *testing.T, e u43Env, it conversation.InflightTurn) {
	t.Helper()
	if it.Phase.Valid() {
		if err := e.svc.BeginTurn(context.Background(), it); err != nil {
			t.Fatalf("前提不成立：BeginTurn 写陈旧行失败: %v", err)
		}
		return
	}
	_, err := e.db.Exec(`INSERT INTO inflight_turns (turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id, cursor_seq, scanned_to, started_at, phase, tier, work_dir) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.TurnID, it.ConvID, it.AgentID, it.Provider, it.PlannedSessionID, it.ResumeSessionID, it.CursorSeq, it.ScannedTo,
		it.StartedAt.UTC().Format(time.RFC3339Nano), string(it.Phase), it.Tier, it.WorkDir)
	if err != nil {
		t.Fatalf("前提不成立：裸 SQL 写非法 phase 的陈旧行失败: %v", err)
	}
}

// u43SeedPKConflict 是主键冲突夹具：先记一行同 turn_id 的 turn_usage（outcome 7），再 BeginTurn。
// 顺序冻结：反过来 RecordTurn 会清掉在飞记录。
func u43SeedPKConflict(t *testing.T, e u43Env, it conversation.InflightTurn) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.svc.RecordTurn(ctx, nil, conversation.TurnUsage{TurnID: it.TurnID, ConvID: it.ConvID, AgentID: it.AgentID, Outcome: 7, CreatedAt: time.Now().UTC()}, nil, false); err != nil {
		t.Fatalf("前提不成立：主键冲突夹具的 RecordTurn 失败: %v", err)
	}
	if err := e.svc.BeginTurn(ctx, it); err != nil {
		t.Fatalf("前提不成立：主键冲突夹具的 BeginTurn 失败: %v", err)
	}
}

// u43Count 跑一句 SELECT COUNT(*)。
func u43Count(t *testing.T, e u43Env, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("查库失败 %q: %v", q, err)
	}
	return n
}

// u43Lines 按冻结的切行规则切屏幕输出。
func u43Lines(out string) []string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		for strings.HasPrefix(l, "council> ") {
			l = strings.TrimPrefix(l, "council> ")
		}
		lines[i] = l
	}
	return lines
}

// u43Line 把一个带换行的格式串填好，去掉末尾换行，成为切行后的一行。
func u43Line(format string, args ...any) string {
	return strings.TrimSuffix(fmt.Sprintf(format, args...), "\n")
}

// u43Frame 是「整段等于 [body...]」的完整期望。
func u43Frame(conv string, body ...string) []string {
	out := []string{"当前会话: " + conv + "，标题: " + u43ConvTitle}
	out = append(out, body...)
	return append(out, "", "再见", "")
}

// u43AssertFrame 断言整段输出按行切片整体相等。
func u43AssertFrame(t *testing.T, out string, want []string) {
	t.Helper()
	if got := u43Lines(out); !reflect.DeepEqual(got, want) {
		t.Errorf("屏幕整段不符：\n  got =%q\n  want=%q", got, want)
	}
}

// u43Run 走 runREPLRunner（详细档），REPL 返回错误即停。
func u43Run(t *testing.T, e u43Env, rn *turn.Runner, input string) string {
	t.Helper()
	out, err := runREPLRunner(t, e.svc, rn, input)
	if err != nil {
		t.Fatalf("REPL.Run 返回错误: %v；输出:\n%s", err, out)
	}
	return out
}

// TestU43RebuildSettlesStaleThenAskSpawns 是 R1：种陈旧行 -> /ask 被拒 -> /session rebuild 结清
// -> 库里多一行与启动对账同形的 lost -> 下一次 /ask 起进程。
//
// 【判别力】第 2 条是主判据：HEAD 上 /session rebuild 只打「已丢弃……」，四个子测试都红在这里；
// M11（先 Rebuild）下第三行变「本来就没有开着的会话」；M14 / M16 下结清行缺失或第二行换错；
// M15 只让 begun 的第二行变成 L2；M20 只让 u43-bogus 的第二行变成 L3；M21 让 u43-bogus 被当
// begun 处置（第二行 L3、第 5、6 条变 0）；M4（裸删）让 spawned / returned / u43-bogus 的第 5、6
// 条从 1 变 0。第 1 条在 HEAD 上也成立，它是前提（先被拒）而不是判别点。第 3、7 条证明解开之后
// 真起了进程：结清了而闸仍拒时 spoke 行不会出现、outcome=1 只有 1 行。begun 与其余三个互为对照
// （第二行与第 5、6 条）。
func TestU43RebuildSettlesStaleThenAskSpawns(t *testing.T) {
	for _, phase := range u43PhaseSet {
		phase := phase
		t.Run(string(phase), func(t *testing.T) {
			e := u43NewEnv(t, true)
			rn := newInFlightRunner(t, e.svc)
			u43OpenSession(t, e, rn)
			u43Post(t, e, "second-msg")
			u43SeedStale(t, e, u43StaleRow(e.conv, phase, u43FakeSessionID))

			out := u43Run(t, e, rn, "/use "+e.conv+"\n/ask "+inFlightAgentID+"\n/session rebuild "+inFlightAgentID+"\n/ask "+inFlightAgentID+"\n")
			lines := u43Lines(out)

			// 第 1 条：先被 A 之五的闸拒绝。
			refusedAt := -1
			for i := 0; i+1 < len(lines); i++ {
				if lines[i] == u43Line(u43FmtOutcome, "refused", inFlightAgentID) && lines[i+1] == u43LineRefusal {
					refusedAt = i
					break
				}
			}
			if refusedAt < 0 {
				t.Fatalf("第 1 条：没有找到「outcome refused」紧跟拒绝那句的两行；整段:\n%q", lines)
			}

			// 第 2 条：之后出现结清两行紧跟「已丢弃……」。
			second := u43Line(u43LineLost)
			if phase == conversation.PhaseBegun {
				second = u43Line(u43LineBegunNoCost)
			}
			want := []string{
				u43Line(u43FmtSettled, inFlightAgentID, u43StaleTurnID, string(phase)),
				second,
				u43Line(u43FmtDropped, inFlightAgentID),
			}
			settledAt := -1
			for j := refusedAt + 1; j+3 <= len(lines); j++ {
				if reflect.DeepEqual(lines[j:j+3], want) {
					settledAt = j
					break
				}
			}
			if settledAt < 0 {
				t.Errorf("第 2 条：拒绝之后没有找到这三行 %q；整段:\n%q", want, lines)
			}

			// 第 3 条：解开之后真起了进程。
			spokeFound := false
			if settledAt >= 0 {
				for _, l := range lines[settledAt:] {
					if l == u43Line(u43FmtOutcome, "spoke", inFlightAgentID) {
						spokeFound = true
						break
					}
				}
			}
			if !spokeFound {
				t.Errorf("第 3 条：结清之后没有整行 %q；整段:\n%q", u43Line(u43FmtOutcome, "spoke", inFlightAgentID), lines)
			}

			ctx := context.Background()
			// 第 4 条：在飞记录没了。
			if _, ok, err := e.svc.InflightFor(ctx, e.conv, inFlightAgentID); err != nil || ok {
				t.Errorf("第 4 条：InflightFor ok=%v err=%v，期望没有、无错", ok, err)
			}

			wantLost := int64(1)
			if phase == conversation.PhaseBegun {
				wantLost = 0
			}
			// 第 5 条：与启动对账同形的 lost 行（绝对值）。
			if n := u43Count(t, e, `SELECT COUNT(*) FROM turn_usage WHERE turn_id = 'u43-stale-turn' AND outcome = 9 AND seq IS NULL AND input_tokens IS NULL AND cost_usd IS NULL AND provider_session_id = '2a90d449-a348-497a-8812-e73e74d6898e' AND created_at = '2026-09-01T00:00:00Z'`); n != wantLost {
				t.Errorf("第 5 条：lost 行数 = %d，期望 %d", n, wantLost)
			}
			// 第 6 条：turn_lost 面包屑。
			if n := u43Count(t, e, `SELECT COUNT(*) FROM messages WHERE conv_id = ? AND kind = 'system' AND json_extract(meta, '$.event') = 'turn_lost'`, e.conv); n != wantLost {
				t.Errorf("第 6 条：turn_lost 面包屑条数 = %d，期望 %d", n, wantLost)
			}
			// 第 7 条：spoke 行恰好两行（首轮 + 解开之后那一轮）。
			if n := u43Count(t, e, `SELECT COUNT(*) FROM turn_usage WHERE agent_id = 'ag_wait' AND outcome = 1`); n != 2 {
				t.Errorf("第 7 条：outcome=1 行数 = %d，期望 2", n)
			}
		})
	}
}

// TestU43RebuildRejectedWhileTurnInFlight 是 R2：一轮真的在飞时 /session rebuild 拒绝，什么都不碰。
//
// 【判别力】不变量 (a) 在 REPL 层的担保。M1（不取闸）下本方法结清正在跑那一轮自己的行，那一轮
// 随后撞 turn_usage 主键失败，红在前提守卫「第二轮 err == nil」，整段也多出结清两行；M2（Lock()
// 等待）下等那一轮跑完、查不到行、接着 Rebuild 丢会话：整段变「已丢弃……」、found == false；
// M13（去掉闸被占那一支）下落到读失败那一句。HEAD 上绿（HEAD 的 Rebuild 同样拒绝），是回归守卫。
// 时序沿用既有 700ms / 350ms（inFlightSleep / inFlightProbeDelay），这是本文件唯一靠睡眠的用例，
// 契约冻结的写法。
func TestU43RebuildRejectedWhileTurnInFlight(t *testing.T) {
	e := u43NewEnv(t, true)
	rn := newInFlightRunner(t, e.svc)
	u43OpenSession(t, e, rn)
	u43Post(t, e, "second-msg")

	var wg sync.WaitGroup
	var secondErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, secondErr = rn.Ask(context.Background(), e.conv, inFlightAgentID)
	}()
	t.Cleanup(wg.Wait)

	time.Sleep(inFlightProbeDelay)
	out := u43Run(t, e, rn, "/use "+e.conv+"\n/session rebuild "+inFlightAgentID+"\n")
	wg.Wait()
	if secondErr != nil {
		t.Fatalf("前提不成立：在飞那一轮应当成功，err=%v；/session rebuild 的输出:\n%s", secondErr, out)
	}

	u43AssertFrame(t, out, u43Frame(e.conv, u43Line(u43FmtBusy, inFlightAgentID)))
	if n := u43Count(t, e, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 9`); n != 0 {
		t.Errorf("outcome=9 行数 = %d，期望 0", n)
	}
	if n := u43Count(t, e, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 1`); n != 2 {
		t.Errorf("outcome=1 行数 = %d，期望 2", n)
	}
	if n := u43Count(t, e, `SELECT COUNT(*) FROM inflight_turns`); n != 0 {
		t.Errorf("inflight_turns 行数 = %d，期望 0", n)
	}
	if _, found, err := e.svc.ProviderSession(context.Background(), e.conv, inFlightAgentID); err != nil || !found {
		t.Errorf("ProviderSession found=%v err=%v，期望还在", found, err)
	}
}

// TestU43RebuildSettleFailureFailsClosed 是 R3：结清写失败（主键冲突）时 fail closed。
//
// 【判别力】M9（失败吞成成功）、M12（失败了还往下走）下整段多出「已结清」「已丢弃」、provider
// 会话被删；M11（先 Rebuild 再结清）下 provider 会话被删；M8、M17 下打成读失败那一句；M4（裸删）
// 下 begun 式的裸删不写 turn_usage、不撞主键，结清成功，整段变样。HEAD 上红：HEAD 打「已丢弃」
// 并删掉 provider 会话，接着的 /ask 照样被拒。
func TestU43RebuildSettleFailureFailsClosed(t *testing.T) {
	e := u43NewEnv(t, true)
	rn := newInFlightRunner(t, e.svc)
	u43OpenSession(t, e, rn)
	u43Post(t, e, "second-msg")
	u43SeedPKConflict(t, e, u43StaleRow(e.conv, conversation.PhaseSpawned, u43FakeSessionID))
	ctx := context.Background()
	ps0, found0, err := e.svc.ProviderSession(ctx, e.conv, inFlightAgentID)
	if err != nil || !found0 {
		t.Fatalf("前提不成立：开口之后读不到 provider 会话 found=%v err=%v", found0, err)
	}

	out := u43Run(t, e, rn, "/use "+e.conv+"\n/session rebuild "+inFlightAgentID+"\n/ask "+inFlightAgentID+"\n")

	settleErr := "settle stale inflight " + e.conv + "/ag_wait: settle lost turn u43-stale-turn: settle lost turn u43-stale-turn: record usage u43-stale-turn: constraint failed: UNIQUE constraint failed: turn_usage.turn_id (1555)"
	u43AssertFrame(t, out, u43Frame(e.conv,
		u43Line(u43FmtSettleFail, inFlightAgentID, u43StaleTurnID, settleErr),
		u43Line(u43FmtAsking, inFlightAgentID),
		u43Line(u43FmtOutcome, "refused", inFlightAgentID),
		u43LineRefusal,
	))
	ps1, found1, err := e.svc.ProviderSession(ctx, e.conv, inFlightAgentID)
	if err != nil || !found1 || ps1 != ps0 {
		t.Errorf("provider 会话变了：found=%v err=%v\n  before=%+v\n  after =%+v", found1, err, ps0, ps1)
	}
	if _, ok, err := e.svc.InflightFor(ctx, e.conv, inFlightAgentID); err != nil || !ok {
		t.Errorf("InflightFor ok=%v err=%v，期望那一行仍在", ok, err)
	}
}

// TestU43RebuildReadFailureTouchesNothing 是 R4：读在飞记录失败时什么都不改。
//
// 【判别力】M3（读失败当「没有」）下往下 Rebuild 丢会话：整段变「已丢弃……」、found == false；
// M12 同样往下走；M17 下打成结清失败那一句。HEAD 上红（HEAD 不读在飞记录，照样丢会话）。
func TestU43RebuildReadFailureTouchesNothing(t *testing.T) {
	e := u43NewEnv(t, true)
	rn := newInFlightRunner(t, e.svc)
	u43OpenSession(t, e, rn)
	ctx := context.Background()
	ps0, found0, err := e.svc.ProviderSession(ctx, e.conv, inFlightAgentID)
	if err != nil || !found0 {
		t.Fatalf("前提不成立：开口之后读不到 provider 会话 found=%v err=%v", found0, err)
	}

	badSvc := conversation.NewService(u43FailReadStore{Store: e.st})
	badRn := newInFlightRunner(t, badSvc)
	out, err := runREPLRunner(t, badSvc, badRn, "/use "+e.conv+"\n/session rebuild "+inFlightAgentID+"\n")
	if err != nil {
		t.Fatalf("REPL.Run 返回错误: %v；输出:\n%s", err, out)
	}

	u43AssertFrame(t, out, u43Frame(e.conv,
		u43Line(u43FmtReadFail, inFlightAgentID, "settle stale inflight "+e.conv+"/ag_wait: read: u43 injected read failure"),
	))
	ps1, found1, err := e.svc.ProviderSession(ctx, e.conv, inFlightAgentID)
	if err != nil || !found1 || ps1 != ps0 {
		t.Errorf("provider 会话变了：found=%v err=%v\n  before=%+v\n  after =%+v", found1, err, ps0, ps1)
	}
}

// TestU43RebuildWithoutStaleRowUnchanged 是 R5：没有在飞记录时与本刀之前逐字节相同。
//
// 【判别力】正面担保「没有行时一个字不多打」。M11（先 Rebuild 再结清）下第二次 Rebuild 打
// 「本来就没有开着的会话」而红。R1 是它只差「有没有那一行」的对照。HEAD 上绿。
func TestU43RebuildWithoutStaleRowUnchanged(t *testing.T) {
	e := u43NewEnv(t, true)
	rn := newInFlightRunner(t, e.svc)
	u43OpenSession(t, e, rn)
	before := u43Count(t, e, `SELECT COUNT(*) FROM messages`)

	out := u43Run(t, e, rn, "/use "+e.conv+"\n/session rebuild "+inFlightAgentID+"\n")

	u43AssertFrame(t, out, u43Frame(e.conv, u43Line(u43FmtDropped, inFlightAgentID)))
	if after := u43Count(t, e, `SELECT COUNT(*) FROM messages`); after != before {
		t.Errorf("messages 行数 %d -> %d，期望不变", before, after)
	}
}

// TestU43RebuildNonMemberDoesNotSettle 是 R6：不是成员就在成员闸拒绝，不结清任何东西。
//
// 【判别力】M18（结清挪到成员闸之前）下这一行被结清、整段多出两行、InflightFor 变没有。
// R1 是对照（只差是不是成员）。HEAD 上绿。
func TestU43RebuildNonMemberDoesNotSettle(t *testing.T) {
	e := u43NewEnv(t, false)
	u43SeedStale(t, e, u43StaleRow(e.conv, conversation.PhaseSpawned, ""))
	rn := newInFlightRunner(t, e.svc)

	out := u43Run(t, e, rn, "/use "+e.conv+"\n/session rebuild "+inFlightAgentID+"\n")

	u43AssertFrame(t, out, u43Frame(e.conv, u43Line(u43FmtNotMember, inFlightAgentID)))
	if _, ok, err := e.svc.InflightFor(context.Background(), e.conv, inFlightAgentID); err != nil || !ok {
		t.Errorf("InflightFor ok=%v err=%v，期望那一行仍在", ok, err)
	}
}

// TestU43RebuildSettlesBootstrapCrash 是 R7：库里没有 provider_sessions（崩在 bootstrap 轮）也照结清。
//
// 【判别力】第三行取的是既有「本来就没有开着的会话」那一句（不变量 (d) 的「下一轮会怎样」）；
// turn_usage 那条证明没有 provider_sessions 行可标时钱照样记得上（M4 裸删下为 0）。M14 / M16
// 下结清行缺失或第二行换错。HEAD 上红（HEAD 只打「本来就没有」，之后被拒）。
func TestU43RebuildSettlesBootstrapCrash(t *testing.T) {
	e := u43NewEnv(t, true)
	u43Post(t, e, "seed-msg")
	u43SeedStale(t, e, u43StaleRow(e.conv, conversation.PhaseSpawned, ""))
	rn := newInFlightRunner(t, e.svc)

	out := u43Run(t, e, rn, "/use "+e.conv+"\n/session rebuild "+inFlightAgentID+"\n/ask "+inFlightAgentID+"\n")
	lines := u43Lines(out)

	want := []string{
		u43Line(u43FmtSettled, inFlightAgentID, u43StaleTurnID, string(conversation.PhaseSpawned)),
		u43Line(u43LineLost),
		u43Line(u43LineNoSession),
	}
	at := -1
	for j := 0; j+3 <= len(lines); j++ {
		if reflect.DeepEqual(lines[j:j+3], want) {
			at = j
			break
		}
	}
	if at < 0 {
		t.Errorf("没有找到这三行 %q；整段:\n%q", want, lines)
	}
	spokeFound := false
	if at >= 0 {
		for _, l := range lines[at:] {
			if l == u43Line(u43FmtOutcome, "spoke", inFlightAgentID) {
				spokeFound = true
				break
			}
		}
	}
	if !spokeFound {
		t.Errorf("结清之后没有整行 %q；整段:\n%q", u43Line(u43FmtOutcome, "spoke", inFlightAgentID), lines)
	}
	if n := u43Count(t, e, `SELECT COUNT(*) FROM turn_usage WHERE turn_id = 'u43-stale-turn' AND outcome = 9 AND provider_session_id = ''`); n != 1 {
		t.Errorf("lost 行数 = %d，期望 1", n)
	}
}

// TestU43RebuildSettleLinesSameInConcise 是 R8：简洁档（真机默认）下结清那几行照打，逐字相同。
//
// 【判别力】R1-R7 走 runREPLRunner，是详细档；M19（简洁档不打结清那几行）只有本用例杀得到。
// HEAD 上红（没有结清两行）。
func TestU43RebuildSettleLinesSameInConcise(t *testing.T) {
	e := u43NewEnv(t, true)
	rn := newInFlightRunner(t, e.svc)
	u43OpenSession(t, e, rn)
	u43SeedStale(t, e, u43StaleRow(e.conv, conversation.PhaseSpawned, u43FakeSessionID))

	var buf bytes.Buffer
	r := New(newTestREPLConfig(), e.svc, rn, strings.NewReader("/use "+e.conv+"\n/session rebuild "+inFlightAgentID+"\n"), &buf, RulesContext{})
	r.SetConcise(true)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("REPL.Run 返回错误: %v；输出:\n%s", err, buf.String())
	}

	u43AssertFrame(t, buf.String(), u43Frame(e.conv,
		u43Line(u43FmtSettled, inFlightAgentID, u43StaleTurnID, string(conversation.PhaseSpawned)),
		u43Line(u43LineLost),
		u43Line(u43FmtDropped, inFlightAgentID),
	))
}
