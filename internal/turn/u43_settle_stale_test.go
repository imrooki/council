package turn

// u43_settle_stale_test.go 是 U43A 契约 §2.1 的验收：Runner.SettleStaleInflight 在持有
// (会话, 成员) 的闸时，按启动对账的同一条路（settleOne(classifyLost(行))）结清一条陈旧的
// 在飞记录。全部跑在真 SQLite 上（newRealTurnStore），不起真 provider（fakeSpawner 或本文件
// 的阻塞 Spawner）。并发类用例一律靠 channel 与不可重入的 Mutex 做确定性判定，不睡眠。
// 不新写 TestMain。

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/store"
)

// u43PhaseSet 是契约 §2.0 的 phase 全集：三个合法值加一个 schema 放得进来的非法值。
var u43PhaseSet = []conversation.InflightPhase{
	conversation.PhaseBegun,
	conversation.PhaseSpawned,
	conversation.PhaseReturned,
	conversation.InflightPhase("u43-bogus"),
}

// u43ErrInjectedRead 是 T7 注入的读失败。
var u43ErrInjectedRead = errors.New("u43 injected read failure")

// u43Stale 按契约 §2.0 组一条陈旧的在飞记录（provider claude，tier read，work_dir u43-wd）。
func u43Stale(convID, agentID, turnID, planned, resume string, scannedTo int64, phase conversation.InflightPhase) conversation.InflightTurn {
	return conversation.InflightTurn{
		TurnID:           turnID,
		ConvID:           convID,
		AgentID:          agentID,
		Provider:         "claude",
		PlannedSessionID: planned,
		ResumeSessionID:  resume,
		CursorSeq:        1,
		ScannedTo:        scannedTo,
		StartedAt:        time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Phase:            phase,
		Tier:             "read",
		WorkDir:          "u43-wd",
	}
}

// u43Seed 写一条陈旧行：合法 phase 经 svc.BeginTurn，非法 phase 经裸 SQL（契约 §2.0 冻结的语句）。
func u43Seed(t *testing.T, rs realTurnStore, svc *conversation.Service, it conversation.InflightTurn) {
	t.Helper()
	if it.Phase.Valid() {
		if err := svc.BeginTurn(context.Background(), it); err != nil {
			t.Fatalf("前提不成立：BeginTurn 写陈旧行 %s 失败: %v", it.TurnID, err)
		}
		return
	}
	_, err := rs.rawDB(t).Exec(`INSERT INTO inflight_turns (turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id, cursor_seq, scanned_to, started_at, phase, tier, work_dir) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.TurnID, it.ConvID, it.AgentID, it.Provider, it.PlannedSessionID, it.ResumeSessionID, it.CursorSeq, it.ScannedTo,
		it.StartedAt.UTC().Format(time.RFC3339Nano), string(it.Phase), it.Tier, it.WorkDir)
	if err != nil {
		t.Fatalf("前提不成立：裸 SQL 写非法 phase 的陈旧行 %s 失败: %v", it.TurnID, err)
	}
}

// u43OpenConv 是「开口过的会话」：建会话、alice 入会、发 hello、alice 成功说一轮、再发 second。
func u43OpenConv(t *testing.T, svc *conversation.Service, r *Runner, fs *fakeSpawner) string {
	t.Helper()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "hello")
	fs.enqueueSuccess(claudeT1JSON)
	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil || rec.Outcome != OutcomeSpoke {
		t.Fatalf("前提不成立：开口那一轮应当 spoke，err=%v outcome=%v", err, rec.Outcome)
	}
	postUser(t, svc, convID, "second")
	return convID
}

// u43Setup 是共同前置：真库、默认配置、Runner（spawn 换成 fakeSpawner）。
func u43Setup(t *testing.T) (realTurnStore, *Runner, *fakeSpawner) {
	t.Helper()
	rs := newRealTurnStore(t)
	r := newTestRunner(t, newTestConfig(), rs.svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	return rs, r, fs
}

// u43UsageRow 是 turn_usage 一行里除 turn_id、conv_id 之外的 19 列。
type u43UsageRow struct {
	agentID           string
	providerSessionID string
	seq               sql.NullInt64
	outcome           int64
	inputTokens       sql.NullInt64
	cacheCreate       sql.NullInt64
	cacheRead         sql.NullInt64
	outputTokens      sql.NullInt64
	thinkingTokens    sql.NullInt64
	costUSD           sql.NullFloat64
	costCumulativeUSD sql.NullFloat64
	coldStart         int64
	wallMS            int64
	promptBytes       int64
	deliveredLo       sql.NullInt64
	deliveredHi       sql.NullInt64
	createdAt         string
	identityFP        string
	roundID           string
}

// u43UsageRows 读某个 turn_id 在 turn_usage 里的全部行（19 列）。
func u43UsageRows(t *testing.T, rs realTurnStore, turnID string) []u43UsageRow {
	t.Helper()
	rows, err := rs.rawDB(t).Query(`SELECT agent_id, provider_session_id, seq, outcome, input_tokens, cache_create, cache_read, output_tokens, thinking_tokens, cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes, delivered_lo, delivered_hi, created_at, identity_fp, round_id FROM turn_usage WHERE turn_id = ?`, turnID)
	if err != nil {
		t.Fatalf("查询 turn_usage 失败: %v", err)
	}
	defer rows.Close()
	var out []u43UsageRow
	for rows.Next() {
		var u u43UsageRow
		if err := rows.Scan(&u.agentID, &u.providerSessionID, &u.seq, &u.outcome, &u.inputTokens, &u.cacheCreate, &u.cacheRead,
			&u.outputTokens, &u.thinkingTokens, &u.costUSD, &u.costCumulativeUSD, &u.coldStart, &u.wallMS, &u.promptBytes,
			&u.deliveredLo, &u.deliveredHi, &u.createdAt, &u.identityFP, &u.roundID); err != nil {
			t.Fatalf("扫描 turn_usage 失败: %v", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 turn_usage 失败: %v", err)
	}
	return out
}

// u43Count 跑一句 SELECT COUNT(*)。
func u43Count(t *testing.T, rs realTurnStore, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := rs.rawDB(t).QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("查库失败 %q: %v", q, err)
	}
	return n
}

// u43History 读一条会话的全部消息。
func u43History(t *testing.T, svc *conversation.Service, convID string) []conversation.Message {
	t.Helper()
	h, err := svc.History(context.Background(), convID, 0, 1000)
	if err != nil {
		t.Fatalf("读 History 失败: %v", err)
	}
	return h
}

// u43Suspect 读 provider_sessions.baseline_suspect。
func u43Suspect(t *testing.T, rs realTurnStore, convID, agentID string) int64 {
	t.Helper()
	return u43Count(t, rs, `SELECT baseline_suspect FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`, convID, agentID)
}

// TestU43SettleStaleMatchesStartupPath 是 T1：本方法结清的行与真启动对账结清的行逐列相同。
//
// 【判别力】本刀「同一条路」的核心担保：B 是真启动对账（AuditInflight + SettleLostTurns）写出来的，
// A 是本方法写出来的，turn_usage 19 列整体比、面包屑比、baseline_suspect 比。M4（另写一份：裸删）
// 时 spawned / returned / u43-bogus 的 A 侧 turn_usage 行数 0 != 1、面包屑少一条、不标可疑；
// M21（非法 phase 当 begun）时 u43-bogus 红；M10 / M10c（顺手结清同成员别的会话）时 B 的行被本方法
// 先结清，启动对账那一侧 len(lost) == 0，红在前提守卫。begun 子测试是对照：两边都 0 行、0 条面包屑、
// 不标——这一支的「两边相等」是 0 对 0，只担保「begun 不补账」。绝对值那几条（outcome 9、NULL 三列、
// provider_session_id）防的是两边一起错。
func TestU43SettleStaleMatchesStartupPath(t *testing.T) {
	for _, phase := range u43PhaseSet {
		phase := phase
		t.Run(string(phase), func(t *testing.T) {
			ctx := context.Background()
			rs, r, fs := u43Setup(t)
			cfg := newTestConfig()
			convA := u43OpenConv(t, rs.svc, r, fs)
			convB := u43OpenConv(t, rs.svc, r, fs)
			psA, foundA, err := rs.svc.ProviderSession(ctx, convA, testAgentClaude)
			if err != nil || !foundA {
				t.Fatalf("前提不成立：A 的 provider 会话 found=%v err=%v", foundA, err)
			}
			u43Seed(t, rs, rs.svc, u43Stale(convA, testAgentClaude, "u43-stale-a", "u43-planned", psA.ProviderSessionID, 2, phase))
			u43Seed(t, rs, rs.svc, u43Stale(convB, testAgentClaude, "u43-stale-b", "u43-planned", psA.ProviderSessionID, 2, phase))
			histA0 := len(u43History(t, rs.svc, convA))
			histB0 := len(u43History(t, rs.svc, convB))

			got, err := r.SettleStaleInflight(ctx, convA, testAgentClaude)
			if err != nil {
				t.Fatalf("SettleStaleInflight err = %v，期望 nil", err)
			}

			// 启动对账那一侧作对照。
			r2 := newTestRunnerSameWorkDir(r, cfg, rs.svc)
			lost, err := r2.AuditInflight(ctx)
			if err != nil || len(lost) != 1 || lost[0].Turn.TurnID != "u43-stale-b" {
				t.Fatalf("前提不成立：启动对账应当只看到 u43-stale-b，err=%v lost=%+v", err, lost)
			}
			marked, errs := r2.SettleLostTurns(ctx, lost)
			if len(errs) != 0 {
				t.Fatalf("前提不成立：启动对账结清失败: %v", errs)
			}
			wantMarked := 0
			if phase != conversation.PhaseBegun {
				wantMarked = 1
			}
			if marked != wantMarked {
				t.Fatalf("前提不成立：启动对账 marked = %d，期望 %d", marked, wantMarked)
			}

			// 返回值。
			if !got.Found || got.Lost.Turn.TurnID != "u43-stale-a" || got.Lost.Reason != lost[0].Reason {
				t.Errorf("返回值：Found=%v TurnID=%q Reason=%q，期望 true / u43-stale-a / %q", got.Found, got.Lost.Turn.TurnID, got.Lost.Reason, lost[0].Reason)
			}
			wantSettle := conversation.LostSettlement{Existed: true, SuspectMarked: phase != conversation.PhaseBegun}
			if got.Settlement != wantSettle {
				t.Errorf("返回值：Settlement = %+v，期望 %+v", got.Settlement, wantSettle)
			}

			// turn_usage 逐列。
			wantN := 1
			if phase == conversation.PhaseBegun {
				wantN = 0
			}
			rowsA := u43UsageRows(t, rs, "u43-stale-a")
			rowsB := u43UsageRows(t, rs, "u43-stale-b")
			if len(rowsA) != wantN || len(rowsB) != wantN {
				t.Errorf("turn_usage 行数 A=%d B=%d，期望都是 %d", len(rowsA), len(rowsB), wantN)
			}
			if !reflect.DeepEqual(rowsA, rowsB) {
				t.Errorf("turn_usage 两边不同：\n  A=%+v\n  B=%+v", rowsA, rowsB)
			}
			if wantN == 1 && len(rowsA) == 1 {
				a := rowsA[0]
				if a.outcome != 9 || a.seq.Valid || a.inputTokens.Valid || a.costUSD.Valid || a.providerSessionID != psA.ProviderSessionID {
					t.Errorf("A 那一行绝对值不对：%+v，期望 outcome 9、seq / input_tokens / cost_usd 为 NULL、provider_session_id %q", a, psA.ProviderSessionID)
				}
			}

			// 面包屑。
			histA := u43History(t, rs.svc, convA)
			histB := u43History(t, rs.svc, convB)
			if len(histA) != histA0+wantN || len(histB) != histB0+wantN {
				t.Errorf("History 条数 A %d -> %d，B %d -> %d，期望各多 %d", histA0, len(histA), histB0, len(histB), wantN)
			}
			if wantN == 1 && len(histA) > 0 && len(histB) > 0 {
				la, lb := histA[len(histA)-1], histB[len(histB)-1]
				if la.Kind != lb.Kind || la.SenderID != lb.SenderID || la.Text != lb.Text || !reflect.DeepEqual(la.Meta, lb.Meta) {
					t.Errorf("面包屑两边不同：\n  A=%+v\n  B=%+v", la, lb)
				}
				if la.Meta[MetaEvent] != eventTurnLost {
					t.Errorf("A 最后一条 Meta[%s] = %v，期望 %q", MetaEvent, la.Meta[MetaEvent], eventTurnLost)
				}
			}

			// baseline_suspect。
			wantSuspect := int64(wantMarked)
			if a, b := u43Suspect(t, rs, convA, testAgentClaude), u43Suspect(t, rs, convB, testAgentClaude); a != wantSuspect || b != wantSuspect {
				t.Errorf("baseline_suspect A=%d B=%d，期望都是 %d", a, b, wantSuspect)
			}

			// 在飞记录都没了。
			if _, ok, err := rs.svc.InflightFor(ctx, convA, testAgentClaude); err != nil || ok {
				t.Errorf("A 的 InflightFor ok=%v err=%v，期望没有、无错", ok, err)
			}
			if _, ok, err := rs.svc.InflightFor(ctx, convB, testAgentClaude); err != nil || ok {
				t.Errorf("B 的 InflightFor ok=%v err=%v，期望没有、无错", ok, err)
			}
		})
	}
}

// TestU43SettleStaleOnlyTouchesExactKey 是 T2：只动这一 (会话, 成员) 的行。
//
// 【判别力】M10（顺手结清全表）、M10b（结清同会话别的成员）、M10c（结清同成员别的会话）各自让
// 对应那一行的断言红：other-agent / other-conv 的在飞记录消失、turn_usage 多一行、A 或 B 的面包屑
// 多一条、(B, alice) 被标可疑。(A, alice) 那几条是正对照：证明本方法确实结清了目标行。
func TestU43SettleStaleOnlyTouchesExactKey(t *testing.T) {
	ctx := context.Background()
	rs, r, fs := u43Setup(t)
	convA := u43OpenConv(t, rs.svc, r, fs)
	convB := u43OpenConv(t, rs.svc, r, fs)
	joinAgent(t, rs.svc, convA, testAgentAgy)
	target := u43Stale(convA, testAgentClaude, "u43-target", "u43-planned", "", 2, conversation.PhaseSpawned)
	otherAgent := u43Stale(convA, testAgentAgy, "u43-other-agent", "u43-planned", "", 2, conversation.PhaseSpawned)
	otherConv := u43Stale(convB, testAgentClaude, "u43-other-conv", "u43-planned", "", 2, conversation.PhaseSpawned)
	for _, it := range []conversation.InflightTurn{target, otherAgent, otherConv} {
		u43Seed(t, rs, rs.svc, it)
	}
	histA0 := len(u43History(t, rs.svc, convA))
	histB0 := len(u43History(t, rs.svc, convB))

	got, err := r.SettleStaleInflight(ctx, convA, testAgentClaude)
	if err != nil || !got.Found || got.Lost.Turn.TurnID != "u43-target" {
		t.Fatalf("SettleStaleInflight err=%v Found=%v TurnID=%q，期望 nil / true / u43-target", err, got.Found, got.Lost.Turn.TurnID)
	}

	for _, want := range []conversation.InflightTurn{otherAgent, otherConv} {
		it, ok, err := rs.svc.InflightFor(ctx, want.ConvID, want.AgentID)
		if err != nil || !ok {
			t.Errorf("%s 的 InflightFor ok=%v err=%v，期望仍在", want.TurnID, ok, err)
			continue
		}
		if it.TurnID != want.TurnID || it.Phase != want.Phase || !it.StartedAt.Equal(want.StartedAt) {
			t.Errorf("%s 那一行变了：%+v", want.TurnID, it)
		}
	}
	for id, wantN := range map[string]int64{"u43-other-agent": 0, "u43-other-conv": 0, "u43-target": 1} {
		if n := rs.turnUsageRowCount(t, id); n != wantN {
			t.Errorf("turn_usage[%s] 行数 = %d，期望 %d", id, n, wantN)
		}
	}
	if n := len(u43History(t, rs.svc, convB)); n != histB0 {
		t.Errorf("B 的 History 条数 %d -> %d，期望不变", histB0, n)
	}
	histA := u43History(t, rs.svc, convA)
	if len(histA) != histA0+1 {
		t.Errorf("A 的 History 条数 %d -> %d，期望恰好 +1", histA0, len(histA))
	} else if about := histA[len(histA)-1].Meta[MetaAboutAgent]; about != testAgentClaude {
		t.Errorf("A 最后一条 Meta[%s] = %v，期望 %q", MetaAboutAgent, about, testAgentClaude)
	}
	if v := u43Suspect(t, rs, convB, testAgentClaude); v != 0 {
		t.Errorf("(B, alice) baseline_suspect = %d，期望 0", v)
	}
	if v := u43Suspect(t, rs, convA, testAgentClaude); v != 1 {
		t.Errorf("(A, alice) baseline_suspect = %d，期望 1", v)
	}
}

// TestU43SettleStaleRefusesWhileGateHeld 是 T3（白盒）：闸被占着时拒绝，什么都不碰。
//
// 【判别力】M1（不取闸）下结清照做，err 为 nil、行被删、turn_usage 多一行而红。对照（同一用例里
// 放闸后再调一次）证明被拒不是因为这一行本身不可结清。M2（Lock() 阻塞等待）下本用例在同一
// goroutine 上死锁，由 go test -timeout 杀。X2 / X2c 杀不到本条：闸被测试占着，TryLock 先失败，
// 那一行走不到；读没有副作用。
func TestU43SettleStaleRefusesWhileGateHeld(t *testing.T) {
	ctx := context.Background()
	rs, r, fs := u43Setup(t)
	convID := u43OpenConv(t, rs.svc, r, fs)
	u43Seed(t, rs, rs.svc, u43Stale(convID, testAgentClaude, "u43-stale", "u43-planned", "", 2, conversation.PhaseSpawned))
	hist0 := len(u43History(t, rs.svc, convID))

	g := r.gate(sessionKey{convID: convID, agentID: testAgentClaude})
	g.mu.Lock()
	got, err := r.SettleStaleInflight(ctx, convID, testAgentClaude)
	g.mu.Unlock()

	if !errors.Is(err, ErrTurnInFlight) {
		t.Errorf("err = %v，期望包了 ErrTurnInFlight", err)
	}
	if got != (StaleSettlement{}) {
		t.Errorf("返回值 = %+v，期望零值", got)
	}
	if _, ok, err := rs.svc.InflightFor(ctx, convID, testAgentClaude); err != nil || !ok {
		t.Errorf("InflightFor ok=%v err=%v，期望那一行仍在", ok, err)
	}
	if n := rs.turnUsageRowCount(t, "u43-stale"); n != 0 {
		t.Errorf("turn_usage[u43-stale] 行数 = %d，期望 0", n)
	}
	if n := len(u43History(t, rs.svc, convID)); n != hist0 {
		t.Errorf("History 条数 %d -> %d，期望不变", hist0, n)
	}

	// 对照：放闸之后同一行可以结清。
	got2, err := r.SettleStaleInflight(ctx, convID, testAgentClaude)
	if err != nil || !got2.Found {
		t.Errorf("对照：放闸之后 err=%v Found=%v，期望 nil / true", err, got2.Found)
	}
}

// u43BlockingSpawner 是 T4 的阻塞 Spawner：被调用时 close(started)，等 release，再回放 claudeT1JSON。
type u43BlockingSpawner struct {
	started chan struct{}
	release chan struct{}
}

func (b *u43BlockingSpawner) spawn(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
	close(b.started)
	<-b.release
	return []byte(claudeT1JSON), nil, 0, false, nil
}

// u43AskResult 是 T4 里在飞那一轮 Ask 的结果。
type u43AskResult struct {
	rec Record
	err error
}

// TestU43SettleStaleRefusesDuringLiveTurn 是 T4：真的一轮在飞时拒绝，不把那一轮自己的行当陈旧行结清。
//
// 【判别力】不变量 (a) 的可观测灾难。M1（不取闸）下本方法读到在飞那一轮自己的 spawned 行并把它
// 结清成 lost（Found、err 为 nil，turn_usage 以 live 为 turn_id 写下 outcome 9），那一轮随后
// RecordUsage 撞主键、Ask 返回 OutcomePostRejected：四条断言全红。X2 / X2c 杀不到本条：闸被在飞
// 那一轮占着，TryLock 先失败；X2 由 T4b 担保。M2（Lock() 等待）下与在飞那一轮互等而死锁，由
// -timeout 杀。时序全靠 channel（started / release），不睡眠。
func TestU43SettleStaleRefusesDuringLiveTurn(t *testing.T) {
	ctx := context.Background()
	rs, r, fs := u43Setup(t)
	convID := u43OpenConv(t, rs.svc, r, fs)

	b := &u43BlockingSpawner{started: make(chan struct{}), release: make(chan struct{})}
	r.spawn = b.spawn
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(b.release) }) }
	t.Cleanup(release)

	done := make(chan u43AskResult, 1)
	go func() {
		rec, err := r.Ask(ctx, convID, testAgentClaude)
		done <- u43AskResult{rec: rec, err: err}
	}()
	<-b.started

	liveRow, ok, err := rs.svc.InflightFor(ctx, convID, testAgentClaude)
	if err != nil || !ok || liveRow.Phase != conversation.PhaseSpawned {
		release()
		<-done
		t.Fatalf("前提不成立：在飞那一轮的行 ok=%v err=%v phase=%q，期望 spawned", ok, err, liveRow.Phase)
	}
	live := liveRow.TurnID

	got, settleErr := r.SettleStaleInflight(ctx, convID, testAgentClaude)
	release()
	res := <-done

	if !errors.Is(settleErr, ErrTurnInFlight) || got.Found {
		t.Errorf("SettleStaleInflight err=%v Found=%v，期望包了 ErrTurnInFlight、Found 为假", settleErr, got.Found)
	}
	if res.err != nil || res.rec.Outcome != OutcomeSpoke {
		t.Errorf("在飞那一轮 err=%v outcome=%v，期望 nil / OutcomeSpoke", res.err, res.rec.Outcome)
	}
	if n := u43Count(t, rs, `SELECT COUNT(*) FROM turn_usage WHERE turn_id = ? AND outcome = 1`, live); n != 1 {
		t.Errorf("turn_usage[%s] outcome=1 行数 = %d，期望 1", live, n)
	}
	if n := u43Count(t, rs, `SELECT COUNT(*) FROM turn_usage WHERE turn_id = ?`, live); n != 1 {
		t.Errorf("turn_usage[%s] 行数 = %d，期望恰好 1", live, n)
	}
	if n := u43Count(t, rs, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 9`); n != 0 {
		t.Errorf("outcome=9 行数 = %d，期望 0", n)
	}
}

// u43InflightProbe 是 T4b 的一次性探针：只重写 InflightFor。武装时先解除武装、计数，再对 gate
// 做 TryLock，拿到就立刻还回去并记 sawGateFree；无论武装与否都透传真库。gate 由用例在自己的
// goroutine 里经 r.gate 取好塞进来，探针自己不调 r.gate。整段调用同步执行，字段没有并发读写。
type u43InflightProbe struct {
	*store.Store
	gate        *sessionGate
	armed       bool
	calls       int
	sawGateFree bool
}

func (s *u43InflightProbe) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	if s.armed {
		s.armed = false
		s.calls++
		if s.gate.mu.TryLock() {
			s.gate.mu.Unlock()
			s.sawGateFree = true
		}
	}
	return s.Store.InflightFor(ctx, convID, agentID)
}

// TestU43SettleStaleHoldsGateAcrossRead 是 T4b（白盒，确定性）：读在飞记录时闸由本方法持有。
//
// 【判别力】Go 的 Mutex 不可重入，整段在用例 goroutine 里同步执行：好实现下读的那一刻闸在本方法
// 手里，探针的 TryLock 必然失败。X2（拿到闸立刻放）、X2c（读挪到 TryLock 之前）、M1（不取闸）下
// 探针拿得到闸，sawGateFree 为真而红。正对照（同一个探针、同一个 key，只差「在不在本方法里读」）
// 证明探针真能看见空闲的闸，不会因为装错而空绿。前提守卫「恰好 1 次」只挡得住 0 次（探针一次性，
// 计数最多是 1）：挡的是「读被删掉、探针没跑」的空绿。X2 下的可观测灾难没有用例，只由本条从结构上担保。
func TestU43SettleStaleHoldsGateAcrossRead(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	probe := &u43InflightProbe{Store: rs.st}
	svc := conversation.NewService(probe)
	cfg := newTestConfig()
	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	convID := u43OpenConv(t, svc, r, fs)

	probe.gate = r.gate(sessionKey{convID: convID, agentID: testAgentClaude})
	probe.armed = true
	got, err := r.SettleStaleInflight(ctx, convID, testAgentClaude)
	if probe.calls != 1 {
		t.Fatalf("前提不成立：武装之后 InflightFor 被调了 %d 次，期望恰好 1 次", probe.calls)
	}
	if err != nil || got.Found {
		t.Fatalf("前提不成立：没有在飞记录时 err=%v Found=%v，期望 nil / false", err, got.Found)
	}
	if probe.sawGateFree {
		t.Errorf("读在飞记录时本 (会话, 成员) 的闸是空闲的：闸没有跨过读")
	}

	// 正对照：闸外直接读，探针必须看见空闲的闸。
	probe.armed = true
	if _, _, err := svc.InflightFor(ctx, convID, testAgentClaude); err != nil {
		t.Fatalf("前提不成立：正对照的 InflightFor err = %v", err)
	}
	if !(probe.calls == 2 && probe.sawGateFree) {
		t.Errorf("正对照：calls=%d sawGateFree=%v，期望 2 / true（探针在闸外应当拿得到闸）", probe.calls, probe.sawGateFree)
	}
}

// u43SessionProbe 是 T4c 的一次性探针：只重写 ProviderSession，体例与 u43InflightProbe 相同。
type u43SessionProbe struct {
	*store.Store
	gate        *sessionGate
	armed       bool
	calls       int
	sawGateFree bool
}

func (s *u43SessionProbe) ProviderSession(ctx context.Context, convID, agentID string) (conversation.ProviderSession, bool, error) {
	if s.armed {
		s.armed = false
		s.calls++
		if s.gate.mu.TryLock() {
			s.gate.mu.Unlock()
			s.sawGateFree = true
		}
	}
	return s.Store.ProviderSession(ctx, convID, agentID)
}

// TestU43SettleStaleHoldsGateAcrossSettle 是 T4c（白盒，确定性；主代理裁定采纳，契约 §7.2 第 6 条）：
// settleOne 在事务之外读回 ProviderSession 的那一刻，闸仍由本方法持有。
//
// 【判别力】X2b（读完之后、settleOne 之前放闸）下这次读落在闸外，sawGateFree 为真而红，只有本条
// 杀得到；X2、M1 下同样红。X2c、X2d、M2 下绿（X2d 只能逐字核对）。武装之后 ProviderSession 只被
// settleOne 调这一次，前提守卫「恰好 1 次」挡住「读被删掉、探针没跑」的空绿，只抓得到 0 次。
// 正对照同 T4b：闸外直接读，探针必须看见空闲的闸。
func TestU43SettleStaleHoldsGateAcrossSettle(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	probe := &u43SessionProbe{Store: rs.st}
	svc := conversation.NewService(probe)
	cfg := newTestConfig()
	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	convID := u43OpenConv(t, svc, r, fs)
	if err := svc.BeginTurn(ctx, u43Stale(convID, testAgentClaude, "u43-stale", "u43-planned", "", 2, conversation.PhaseSpawned)); err != nil {
		t.Fatalf("前提不成立：BeginTurn 写陈旧行失败: %v", err)
	}

	probe.gate = r.gate(sessionKey{convID: convID, agentID: testAgentClaude})
	probe.armed = true
	got, err := r.SettleStaleInflight(ctx, convID, testAgentClaude)
	if probe.calls != 1 {
		t.Fatalf("前提不成立：武装之后 ProviderSession 被调了 %d 次，期望恰好 1 次", probe.calls)
	}
	if err != nil || !got.Found {
		t.Fatalf("前提不成立：结清应当成功，err=%v Found=%v", err, got.Found)
	}
	if probe.sawGateFree {
		t.Errorf("settleOne 读回 ProviderSession 时本 (会话, 成员) 的闸是空闲的：闸没有跨过结清")
	}

	// 正对照：闸外直接读，探针必须看见空闲的闸。
	probe.armed = true
	if _, _, err := svc.ProviderSession(ctx, convID, testAgentClaude); err != nil {
		t.Fatalf("前提不成立：正对照的 ProviderSession err = %v", err)
	}
	if !(probe.calls == 2 && probe.sawGateFree) {
		t.Errorf("正对照：calls=%d sawGateFree=%v，期望 2 / true（探针在闸外应当拿得到闸）", probe.calls, probe.sawGateFree)
	}
}

// TestU43SettleStaleClearsMemorySnapshot 是 T5（白盒）：结清成功后内存快照置空，下一轮从库里读回。
//
// 【判别力】M5（不置 nil）下两个子测试的第一条都红；spawned 另因 SessionInfo 走内存分支读到旧的
// false 而红——那就是「库里已标可疑、下一轮却自信地算出金额」。begun 是对照：库里没标，读库也是 false。
func TestU43SettleStaleClearsMemorySnapshot(t *testing.T) {
	for _, phase := range []conversation.InflightPhase{conversation.PhaseSpawned, conversation.PhaseBegun} {
		phase := phase
		t.Run(string(phase), func(t *testing.T) {
			ctx := context.Background()
			rs, r, fs := u43Setup(t)
			convID := u43OpenConv(t, rs.svc, r, fs)
			key := sessionKey{convID: convID, agentID: testAgentClaude}
			if st := r.gate(key).st; st == nil || st.baselineSuspect {
				t.Fatalf("前提不成立：开口之后内存快照 = %+v，期望非空且不可疑", st)
			}
			u43Seed(t, rs, rs.svc, u43Stale(convID, testAgentClaude, "u43-stale", "u43-planned", "", 2, phase))

			got, err := r.SettleStaleInflight(ctx, convID, testAgentClaude)
			if err != nil || !got.Found {
				t.Fatalf("前提不成立：结清应当成功，err=%v Found=%v", err, got.Found)
			}
			if st := r.gate(key).st; st != nil {
				t.Errorf("结清之后内存快照 = %+v，期望 nil", st)
			}
			info, ok := r.SessionInfo(ctx, convID, testAgentClaude)
			if !ok {
				t.Fatalf("SessionInfo ok = false，期望 true")
			}
			if want := phase == conversation.PhaseSpawned; info.BaselineSuspect != want {
				t.Errorf("SessionInfo.BaselineSuspect = %v，期望 %v", info.BaselineSuspect, want)
			}
		})
	}
}

// TestU43SettleStaleNoRowIsNoop 是 T6：没有在飞记录时什么都不做，内存快照原样。
//
// 【判别力】M7（没查到也清内存快照）下指针比对红。对照是 T5（只差「有没有那一行」）。
func TestU43SettleStaleNoRowIsNoop(t *testing.T) {
	ctx := context.Background()
	rs, r, fs := u43Setup(t)
	convID := u43OpenConv(t, rs.svc, r, fs)
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	st0 := r.gate(key).st
	if st0 == nil {
		t.Fatalf("前提不成立：开口之后内存快照为 nil")
	}
	hist0 := len(u43History(t, rs.svc, convID))

	got, err := r.SettleStaleInflight(ctx, convID, testAgentClaude)
	if err != nil || got != (StaleSettlement{}) {
		t.Errorf("err=%v got=%+v，期望 nil / 零值", err, got)
	}
	if st := r.gate(key).st; st != st0 {
		t.Errorf("内存快照指针变了：%p -> %p", st0, st)
	}
	if n := len(u43History(t, rs.svc, convID)); n != hist0 {
		t.Errorf("History 条数 %d -> %d，期望不变", hist0, n)
	}
}

// u43FailReadStore 只重写 InflightFor，恒返回 u43ErrInjectedRead，其余透传真库。
type u43FailReadStore struct {
	*store.Store
}

func (s u43FailReadStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	return conversation.InflightTurn{}, false, u43ErrInjectedRead
}

// TestU43SettleStaleReadErrorTouchesNothing 是 T7：读在飞记录失败时返回错误，什么都不做。
//
// 【判别力】M3（读失败当「没有」）下 err 为 nil 而红。「读不出来」是「不知道」，不是「没有」。
func TestU43SettleStaleReadErrorTouchesNothing(t *testing.T) {
	ctx := context.Background()
	rs, r, fs := u43Setup(t)
	cfg := newTestConfig()
	convID := u43OpenConv(t, rs.svc, r, fs)
	u43Seed(t, rs, rs.svc, u43Stale(convID, testAgentClaude, "u43-stale", "u43-planned", "", 2, conversation.PhaseSpawned))

	bad := newTestRunnerSameWorkDir(r, cfg, conversation.NewService(u43FailReadStore{Store: rs.st}))
	got, err := bad.SettleStaleInflight(ctx, convID, testAgentClaude)
	if !errors.Is(err, u43ErrInjectedRead) {
		t.Errorf("err = %v，期望包了注入的读失败", err)
	}
	if got != (StaleSettlement{}) {
		t.Errorf("返回值 = %+v，期望零值", got)
	}
	if _, ok, err := rs.svc.InflightFor(ctx, convID, testAgentClaude); err != nil || !ok {
		t.Errorf("InflightFor ok=%v err=%v，期望那一行仍在", ok, err)
	}
	if n := rs.turnUsageRowCount(t, "u43-stale"); n != 0 {
		t.Errorf("turn_usage[u43-stale] 行数 = %d，期望 0", n)
	}
}

// TestU43SettleStaleWriteFailureFailsClosed 是 T8：结清写失败（turn_usage 主键冲突）时 fail closed。
//
// 【判别力】M6（结清之前就清内存）红在 st0 指针比对；M8（失败时 Found 为假）、M9（失败被吞成成功）
// 红在前两条；M4（裸删）不写 turn_usage、不撞主键，结清成功而红。最后一段证明 fail closed 之后成员
// 照样被 A 之五的闸锁住（不起进程）。
func TestU43SettleStaleWriteFailureFailsClosed(t *testing.T) {
	ctx := context.Background()
	rs, r, fs := u43Setup(t)
	convID := u43OpenConv(t, rs.svc, r, fs)
	stale := u43Stale(convID, testAgentClaude, "u43-stale", "u43-planned", "", 2, conversation.PhaseSpawned)
	if _, err := rs.svc.RecordTurn(ctx, nil, conversation.TurnUsage{TurnID: stale.TurnID, ConvID: convID, AgentID: testAgentClaude, Outcome: 7, CreatedAt: time.Now().UTC()}, nil, false); err != nil {
		t.Fatalf("前提不成立：主键冲突夹具的 RecordTurn 失败: %v", err)
	}
	u43Seed(t, rs, rs.svc, stale)
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	st0 := r.gate(key).st
	ps0, found0, err := rs.svc.ProviderSession(ctx, convID, testAgentClaude)
	if err != nil || !found0 {
		t.Fatalf("前提不成立：开口之后读不到 provider 会话 found=%v err=%v", found0, err)
	}
	hist0 := len(u43History(t, rs.svc, convID))

	got, err := r.SettleStaleInflight(ctx, convID, testAgentClaude)
	if err == nil || !got.Found || got.Lost.Turn.TurnID != "u43-stale" || got.Settlement != (conversation.LostSettlement{}) {
		t.Errorf("err=%v Found=%v TurnID=%q Settlement=%+v，期望非 nil / true / u43-stale / 零值", err, got.Found, got.Lost.Turn.TurnID, got.Settlement)
	}
	wantErr := "settle stale inflight " + convID + "/alice: settle lost turn u43-stale: settle lost turn u43-stale: record usage u43-stale: constraint failed: UNIQUE constraint failed: turn_usage.turn_id (1555)"
	if err == nil || err.Error() != wantErr {
		t.Errorf("err.Error() =\n  %v\n期望\n  %s", err, wantErr)
	}
	if _, ok, err := rs.svc.InflightFor(ctx, convID, testAgentClaude); err != nil || !ok {
		t.Errorf("InflightFor ok=%v err=%v，期望那一行仍在", ok, err)
	}
	if n := u43Count(t, rs, `SELECT COUNT(*) FROM turn_usage WHERE turn_id = 'u43-stale'`); n != 1 {
		t.Errorf("turn_usage[u43-stale] 行数 = %d，期望恰好 1", n)
	}
	if n := u43Count(t, rs, `SELECT COUNT(*) FROM turn_usage WHERE turn_id = 'u43-stale' AND outcome = 7`); n != 1 {
		t.Errorf("turn_usage[u43-stale] outcome=7 行数 = %d，期望 1（种下的那行）", n)
	}
	if n := len(u43History(t, rs.svc, convID)); n != hist0 {
		t.Errorf("History 条数 %d -> %d，期望不变", hist0, n)
	}
	if ps1, found1, err := rs.svc.ProviderSession(ctx, convID, testAgentClaude); err != nil || !found1 || ps1 != ps0 {
		t.Errorf("provider 会话变了：found=%v err=%v\n  before=%+v\n  after =%+v", found1, err, ps0, ps1)
	}
	if st := r.gate(key).st; st != st0 {
		t.Errorf("内存快照指针变了：%p -> %p，期望不动", st0, st)
	}

	// fail closed 之后成员照样被锁。
	fs.enqueueSuccess(claudeT1JSON)
	calls0 := len(fs.calls)
	rec, err := r.Ask(ctx, convID, testAgentClaude)
	if !errors.Is(err, conversation.ErrInflightConflict) || rec.Outcome != OutcomeRefused {
		t.Errorf("结清失败之后的 Ask err=%v outcome=%v，期望 ErrInflightConflict / OutcomeRefused", err, rec.Outcome)
	}
	if n := len(fs.calls); n != calls0 {
		t.Errorf("Spawner 调用 %d -> %d，期望不变（不起进程）", calls0, n)
	}
}
