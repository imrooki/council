package turn

// 本文件是 Step 10 A 之五（契约 s10a5 v1.1 §2.1）的 turn 包验收：
// 别人的在飞记录在起进程之前就拒（E1），以及 SessionInfo.BaselineSuspect（E2）。
// 闸与库的交互全部跑在真 SQLite 上（newRealTurnStore），不用 recordingStore：
// 后者不强制 (conv_id, agent_id) 唯一索引，测不出「第二道」。

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"

	"modernc.org/sqlite"
)

// s10aStaleTurnID 是契约 §2.0「别人的在飞记录」的 turn_id。
const s10aStaleTurnID = "s10a-stale-turn"

// s10aForeignInflight 按契约 §2.0 造一条「别人的在飞记录」，phase 由调用方给。
func s10aForeignInflight(convID, agentID string, phase conversation.InflightPhase) conversation.InflightTurn {
	return conversation.InflightTurn{
		TurnID:           s10aStaleTurnID,
		ConvID:           convID,
		AgentID:          agentID,
		Provider:         "claude",
		PlannedSessionID: "s10a-planned",
		ResumeSessionID:  "",
		CursorSeq:        0,
		ScannedTo:        1,
		StartedAt:        time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Phase:            phase,
		Tier:             "read",
		WorkDir:          "s10a-wd",
	}
}

// s10aSeedForeign 经 Service.BeginTurn 写入别人的在飞记录，失败即停。
func s10aSeedForeign(t *testing.T, svc *conversation.Service, it conversation.InflightTurn) {
	t.Helper()
	if err := svc.BeginTurn(context.Background(), it); err != nil {
		t.Fatalf("前提不成立：写入别人的在飞记录失败: %v", err)
	}
}

// s10aAssertInflightFields 逐一比对 12 个字段，StartedAt 用 Equal。
func s10aAssertInflightFields(t *testing.T, got, want conversation.InflightTurn) {
	t.Helper()
	if got.TurnID != want.TurnID || got.ConvID != want.ConvID || got.AgentID != want.AgentID ||
		got.Provider != want.Provider || got.PlannedSessionID != want.PlannedSessionID ||
		got.ResumeSessionID != want.ResumeSessionID || got.CursorSeq != want.CursorSeq ||
		got.ScannedTo != want.ScannedTo || !got.StartedAt.Equal(want.StartedAt) ||
		got.Phase != want.Phase || got.Tier != want.Tier || got.WorkDir != want.WorkDir {
		t.Errorf("在飞记录与写入的那条不一致：\n  got =%+v\n  want=%+v", got, want)
	}
}

// s10aUsageRow 查 turn_usage 里某个 turn_id 的全部行，返回 (outcome, identity_fp) 对。
type s10aUsageRow struct {
	outcome    int
	identityFP string
}

func s10aUsageRows(t *testing.T, rs realTurnStore, turnID string) []s10aUsageRow {
	t.Helper()
	db := rs.rawDB(t)
	rows, err := db.Query(`SELECT outcome, identity_fp FROM turn_usage WHERE turn_id = ?`, turnID)
	if err != nil {
		t.Fatalf("查询 turn_usage 失败: %v", err)
	}
	defer rows.Close()
	var out []s10aUsageRow
	for rows.Next() {
		var r s10aUsageRow
		if err := rows.Scan(&r.outcome, &r.identityFP); err != nil {
			t.Fatalf("扫描 turn_usage 失败: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 turn_usage 失败: %v", err)
	}
	return out
}

// s10aBaselineSuspectInDB 读 provider_sessions 那一行的 baseline_suspect 列。
func s10aBaselineSuspectInDB(t *testing.T, rs realTurnStore, convID, agentID string) int {
	t.Helper()
	db := rs.rawDB(t)
	var v int
	if err := db.QueryRow(`SELECT baseline_suspect FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`, convID, agentID).Scan(&v); err != nil {
		t.Fatalf("查询 provider_sessions.baseline_suspect 失败: %v", err)
	}
	return v
}

// TestS10aAskRefusedByForeignInflight 是 G1：三个场景 × 三个 phase。
//
// 【判别力】
//   - 断言 1、2 在「检查整段失效」（§4 #1）下仍绿：真库下会落到 BeginTurn 撞唯一索引，
//     store 译出同一个哨兵，结局仍是 refused、不起进程（HEAD 上 store 不译，断言 1 才红）。
//     #1 下变红的是 bootstrap_* 的 3/6/7、resume_* 的 6、fpmismatch_* 的 3/4/6：
//     这几条才是区分「第一道」与「第二道」的判据。
//   - 断言 3（History 条数不变）在「检查挪到 BeginTurn 正前方」（§4 #2）下，
//     bootstrap_* 从 1 变 2（多出 session_opening），fpmismatch_* 从 4 变 6；
//     resume_* 不发面包屑，这一条对它恒等，不是它的杀手。
//   - 断言 4 只在 fpmismatch_* 能为假：那一支会 DropProviderSession；其余场景恒真。
//   - 断言 6 的 identity_fp == "" 在 #2 下三个场景都红（指纹已算出）；
//     outcome == 7 与「恰好 1 行」在 #5（refuse 传 true）下红：RecordTurn 报
//     ErrInflightMissing 整笔回滚，本轮那一行不见了（0 行）。
//   - 断言 7（仅 bootstrap_*）在 #20（挪到 MkdirAll 之后）下红，其余断言那时全绿。
//   - phase 参数化防 #19：闸按 phase 过滤时，只有被放过的那个 phase 的子测试红。
//   - 对照是 G1c：不写外来行时同一前置下照常说话。
func TestS10aAskRefusedByForeignInflight(t *testing.T) {
	phases := []conversation.InflightPhase{conversation.PhaseBegun, conversation.PhaseSpawned, conversation.PhaseReturned}
	scenarios := []string{"bootstrap", "resume", "fpmismatch"}
	for _, sc := range scenarios {
		for _, ph := range phases {
			sc, ph := sc, ph
			t.Run(sc+"_"+string(ph), func(t *testing.T) {
				ctx := context.Background()
				rs := newRealTurnStore(t)
				convID := newConv(t, rs.svc)
				joinAgent(t, rs.svc, convID, testAgentClaude)
				postUser(t, rs.svc, convID, "hello")
				cfg := newTestConfig()
				r := newTestRunner(t, cfg, rs.svc)
				fs := &fakeSpawner{}
				r.spawn = fs.spawn

				if sc == "resume" || sc == "fpmismatch" {
					fs.enqueueSuccess(claudeT1JSON)
					first, err := r.Ask(ctx, convID, testAgentClaude)
					if err != nil || first.Outcome != OutcomeSpoke {
						t.Fatalf("前提不成立：首轮应当成功，err=%v outcome=%v", err, first.Outcome)
					}
					postUser(t, rs.svc, convID, "second")
				}
				if sc == "fpmismatch" {
					cfg2 := newTestConfig()
					ac := cfg2.Agents[testAgentClaude]
					ac.Model = "claude-other"
					cfg2.Agents[testAgentClaude] = ac
					r = newTestRunnerSameWorkDir(r, cfg2, rs.svc)
					r.spawn = fs.spawn
				}
				foreign := s10aForeignInflight(convID, testAgentClaude, ph)
				s10aSeedForeign(t, rs.svc, foreign)

				psBefore, foundBefore, err := rs.svc.ProviderSession(ctx, convID, testAgentClaude)
				if err != nil {
					t.Fatalf("读 ProviderSession 失败: %v", err)
				}
				histBefore, err := rs.svc.History(ctx, convID, 0, 100)
				if err != nil {
					t.Fatalf("读 History 失败: %v", err)
				}
				callsBefore := len(fs.calls)

				rec, askErr := r.Ask(ctx, convID, testAgentClaude)

				// 断言 1
				if rec.Outcome != OutcomeRefused {
					t.Errorf("断言 1：Outcome = %v，期望 OutcomeRefused", rec.Outcome)
				}
				if !errors.Is(askErr, conversation.ErrInflightConflict) {
					t.Errorf("断言 1：错误链里没有 ErrInflightConflict: %v", askErr)
				}
				// 断言 2
				if got := len(fs.calls); got != callsBefore {
					t.Errorf("断言 2：Spawner 调用次数 %d -> %d，期望不变", callsBefore, got)
				}
				// 断言 3
				histAfter, err := rs.svc.History(ctx, convID, 0, 100)
				if err != nil {
					t.Fatalf("读 History 失败: %v", err)
				}
				if len(histAfter) != len(histBefore) {
					t.Errorf("断言 3：History 条数 %d -> %d，期望不变（不许有 session_opening / session_rebuilt）", len(histBefore), len(histAfter))
				}
				// 断言 4
				psAfter, foundAfter, err := rs.svc.ProviderSession(ctx, convID, testAgentClaude)
				if err != nil {
					t.Fatalf("读 ProviderSession 失败: %v", err)
				}
				if foundAfter != foundBefore || psAfter != psBefore {
					t.Errorf("断言 4：ProviderSession 变了：\n  before found=%v %+v\n  after  found=%v %+v", foundBefore, psBefore, foundAfter, psAfter)
				}
				// 断言 5
				got, ok, err := rs.svc.InflightFor(ctx, convID, testAgentClaude)
				if err != nil || !ok {
					t.Errorf("断言 5：InflightFor ok=%v err=%v，期望那一行仍在", ok, err)
				} else {
					s10aAssertInflightFields(t, got, foreign)
				}
				// 断言 6
				rows := s10aUsageRows(t, rs, rec.TurnID)
				want := []s10aUsageRow{{outcome: int(OutcomeRefused), identityFP: ""}}
				if !reflect.DeepEqual(rows, want) {
					t.Errorf("断言 6：turn_usage[turn_id=%s] = %+v，期望 %+v", rec.TurnID, rows, want)
				}
				// 断言 7
				if sc == "bootstrap" {
					_, statErr := os.Stat(r.workDir)
					if !errors.Is(statErr, os.ErrNotExist) {
						t.Errorf("断言 7：拒绝之后 workDir %s 应当不存在，Stat 返回 %v", r.workDir, statErr)
					}
				}
			})
		}
	}
}

// TestS10aAskControlWithoutForeignInflightSpeaks 是 G1c：同 bootstrap 前置但不写外来行。
//
// 【判别力】它是 G1 的对照：只差「库里有没有那一行」。恒拒的实现（例如判据写成
// !priorFound）下它红；它本身不担保拒绝，只担保 G1 的红来自那一行。
func TestS10aAskControlWithoutForeignInflightSpeaks(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	convID := newConv(t, rs.svc)
	joinAgent(t, rs.svc, convID, testAgentClaude)
	postUser(t, rs.svc, convID, "hello")
	cfg := newTestConfig()
	r := newTestRunner(t, cfg, rs.svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	fs.enqueueSuccess(claudeT1JSON)

	rec, err := r.Ask(ctx, convID, testAgentClaude)
	if err != nil {
		t.Errorf("err = %v，期望 nil", err)
	}
	if rec.Outcome != OutcomeSpoke {
		t.Errorf("Outcome = %v，期望 OutcomeSpoke", rec.Outcome)
	}
	if len(fs.calls) != 1 {
		t.Errorf("Spawner 调用 %d 次，期望 1", len(fs.calls))
	}
	if _, ok, err := rs.svc.InflightFor(ctx, convID, testAgentClaude); err != nil || ok {
		t.Errorf("成功一轮之后 InflightFor ok=%v err=%v，期望没有、无错", ok, err)
	}
}

// TestS10aForeignInflightCheckedBeforeAgentLookup 是 G1e：配置里删掉 alice，库里留着外来行。
//
// 【判别力】检查挪到 cfg.Agents 查找之后（§4 #21）或 MkdirAll 之后（§4 #20）时，
// 先命中的是「配置里没有这个 agent」，错误变成 ErrAgentUnknown，第 2、3 条红。
// 对照是 G1 的 bootstrap_spawned（只差配置里有没有 alice）。
func TestS10aForeignInflightCheckedBeforeAgentLookup(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	convID := newConv(t, rs.svc)
	joinAgent(t, rs.svc, convID, testAgentClaude)
	postUser(t, rs.svc, convID, "hello")
	s10aSeedForeign(t, rs.svc, s10aForeignInflight(convID, testAgentClaude, conversation.PhaseSpawned))
	cfg := newTestConfig()
	delete(cfg.Agents, testAgentClaude)
	r := newTestRunner(t, cfg, rs.svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	rec, err := r.Ask(ctx, convID, testAgentClaude)
	if rec.Outcome != OutcomeRefused {
		t.Errorf("断言 1：Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if !errors.Is(err, conversation.ErrInflightConflict) {
		t.Errorf("断言 2：错误链里没有 ErrInflightConflict: %v", err)
	}
	if errors.Is(err, ErrAgentUnknown) {
		t.Errorf("断言 3：错误链里不该有 ErrAgentUnknown（检查应早于配置查找）: %v", err)
	}
	if len(fs.calls) != 0 {
		t.Errorf("断言 4：Spawner 调用 %d 次，期望 0", len(fs.calls))
	}
}

// TestS10aAskUpToAndRunRoundAreGated 是 G2：AskUpTo 与 RunRound 两条入口同样被拒。
//
// 【判别力】(i) 在 E1 失效（§4 #1）时，真库下落到第二道，结局与哨兵都还在，
// 所以 (i) 担保的是「这条入口也经同一个哨兵」；(ii) 里 nova 那一行是对照：
// 一人被拒不取消别人，nova 照常 spoke、Spawner 恰好一次。
func TestS10aAskUpToAndRunRoundAreGated(t *testing.T) {
	t.Run("askupto", func(t *testing.T) {
		ctx := context.Background()
		rs := newRealTurnStore(t)
		convID := newConv(t, rs.svc)
		joinAgent(t, rs.svc, convID, testAgentClaude)
		postUser(t, rs.svc, convID, "hello")
		s10aSeedForeign(t, rs.svc, s10aForeignInflight(convID, testAgentClaude, conversation.PhaseSpawned))
		r := newTestRunner(t, newTestConfig(), rs.svc)
		fs := &fakeSpawner{}
		r.spawn = fs.spawn

		rec, err := r.AskUpTo(ctx, convID, testAgentClaude, 0, "s10a-round-id")
		if rec.Outcome != OutcomeRefused {
			t.Errorf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
		}
		if !errors.Is(err, conversation.ErrInflightConflict) {
			t.Errorf("错误链里没有 ErrInflightConflict: %v", err)
		}
		if len(fs.calls) != 0 {
			t.Errorf("Spawner 调用 %d 次，期望 0", len(fs.calls))
		}
	})
	t.Run("runround", func(t *testing.T) {
		ctx := context.Background()
		rs := newRealTurnStore(t)
		convID := newConv(t, rs.svc)
		joinAgent(t, rs.svc, convID, testAgentClaude)
		joinAgent(t, rs.svc, convID, testAgentAgy)
		postUser(t, rs.svc, convID, "hello")
		s10aSeedForeign(t, rs.svc, s10aForeignInflight(convID, testAgentClaude, conversation.PhaseSpawned))
		r := newTestRunner(t, newTestConfig(), rs.svc)
		fs := &fakeSpawner{}
		r.spawn = fs.spawn
		fs.enqueueSuccess(agyT1JSON)

		recs := r.RunRound(ctx, convID, Plan{Speakers: []string{testAgentClaude, testAgentAgy}}, 0)
		if len(recs) != 2 {
			t.Fatalf("len(recs) = %d，期望 2", len(recs))
		}
		if recs[0].AgentID != testAgentClaude || recs[0].Outcome != OutcomeRefused || !errors.Is(recs[0].Err, conversation.ErrInflightConflict) {
			t.Errorf("recs[0] = agent %q outcome %v err %v，期望 alice / refused / ErrInflightConflict", recs[0].AgentID, recs[0].Outcome, recs[0].Err)
		}
		if recs[1].AgentID != testAgentAgy || recs[1].Outcome != OutcomeSpoke {
			t.Errorf("recs[1] = agent %q outcome %v err %v，期望 nova / spoke", recs[1].AgentID, recs[1].Outcome, recs[1].Err)
		}
		if len(fs.calls) != 1 {
			t.Errorf("Spawner 调用 %d 次，期望 1（只有 nova）", len(fs.calls))
		}
	})
}

// s10aRaceStore 模拟「检查之后、BeginTurn 之前，这一行出现了」：InflightFor 先读真实现，
// 第一次被调用时再插一行 s10a-race-turn，然后返回第一步读到的真实结果（读的那一刻确实没有）。
type s10aRaceStore struct {
	*store.Store
	fired bool
}

func (s *s10aRaceStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	it, ok, err := s.Store.InflightFor(ctx, convID, agentID)
	if !s.fired {
		s.fired = true
		werr := s.Store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
			return tx.BeginTurn(ctx, conversation.InflightTurn{
				TurnID:    "s10a-race-turn",
				ConvID:    convID,
				AgentID:   agentID,
				Provider:  "claude",
				StartedAt: time.Now().UTC(),
				Phase:     conversation.PhaseBegun,
				Tier:      "read",
			})
		})
		if werr != nil {
			panic("s10aRaceStore: 插入竞争行失败: " + werr.Error())
		}
	}
	return it, ok, err
}

// TestS10aUniqueCollisionCarriesSameSentinel 是 G3：第二道（唯一索引）给出同一个哨兵。
//
// 【判别力】前提守卫证明真的走到了 BeginTurn 撞 2067；store 不译哨兵（§4 #6）时
// errors.Is 那一条红；译的时候丢掉驱动错误（§4 #8）时前提守卫红。
func TestS10aUniqueCollisionCarriesSameSentinel(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	race := &s10aRaceStore{Store: rs.st}
	svc := conversation.NewService(race)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "hello")
	r := newTestRunner(t, newTestConfig(), svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	rec, err := r.Ask(ctx, convID, testAgentClaude)
	var se *sqlite.Error
	if !(errors.As(err, &se) && se.Code() == 2067) {
		t.Fatalf("前提不成立：错误链里没有 Code()==2067 的 *sqlite.Error（没走到第二道，或驱动错误被丢了）: %v", err)
	}
	if rec.Outcome != OutcomeRefused {
		t.Errorf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if !errors.Is(err, conversation.ErrInflightConflict) {
		t.Errorf("错误链里没有 ErrInflightConflict（store 没译哨兵）: %v", err)
	}
	if len(fs.calls) != 0 {
		t.Errorf("Spawner 调用 %d 次，期望 0", len(fs.calls))
	}
}

// s10aErrInjected 是 G4 注入的读失败。
var s10aErrInjected = errors.New("s10a injected read failure")

// s10aFailingReadStore 的 InflightFor 恒返回 s10aErrInjected，其余透传真库。
type s10aFailingReadStore struct {
	*store.Store
}

func (s s10aFailingReadStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	return conversation.InflightTurn{}, false, s10aErrInjected
}

// TestS10aInflightReadErrorRefuses 是 G4：读不出来是「不知道」，不是「没有」。
//
// 【判别力】读失败那一支失效（§4 #3）时，这一轮会照常成功（已 enqueueSuccess），
// 前三条断言红，第四条那时 err 为 nil、恒绿；第四条 !errors.Is(ErrInflightConflict)
// 防的是另一种写法：读失败也包成冲突哨兵（那样屏幕会把「不知道」说成「有别人的记录」）。
// 第五条与 G1 断言 6 同形：这一支的 refuse 把 began 写成字面量 true（审查变异 P1）时，
// RecordTurn 带 expectInflight=true 去清一行不存在的在飞记录，报 ErrInflightMissing 并
// 整笔回滚，turn_usage 里本轮 0 行而红；前四条那时全绿（结局、哨兵、Spawner 都不受影响）。
// 读失败那一支失效（§4 #3）时本轮 spoke，outcome 与 identity_fp 都不对，同样红。
// 对照是 G1 断言 6：同一张表、同一形状，那边拒绝来自「查到了别人的行」。
func TestS10aInflightReadErrorRefuses(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	svc := conversation.NewService(s10aFailingReadStore{Store: rs.st})
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "hello")
	r := newTestRunner(t, newTestConfig(), svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	fs.enqueueSuccess(claudeT1JSON)

	rec, err := r.Ask(ctx, convID, testAgentClaude)
	if rec.Outcome != OutcomeRefused {
		t.Errorf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if !errors.Is(err, s10aErrInjected) {
		t.Errorf("错误链里没有注入的读失败: %v", err)
	}
	if errors.Is(err, conversation.ErrInflightConflict) {
		t.Errorf("读失败不该被说成 ErrInflightConflict: %v", err)
	}
	if len(fs.calls) != 0 {
		t.Errorf("Spawner 调用 %d 次，期望 0", len(fs.calls))
	}
	// 第五条：被拒的这一轮照常记一行 refused 的 turn_usage（经真库的原始句柄查）。
	rows := s10aUsageRows(t, rs, rec.TurnID)
	want := []s10aUsageRow{{outcome: int(OutcomeRefused), identityFP: ""}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("turn_usage[turn_id=%s] = %+v，期望 %+v（恰好 1 行、refused、identity_fp 为空）", rec.TurnID, rows, want)
	}
}

// TestS10aSessionInfoBaselineSuspect 是 G5：SessionInfo 三个 ok 分支各有真、假两个值。
//
// 【判别力】内存分支恒 false（§4 #12）时第 2、6 步红；拿到闸读库分支恒 false（#13）
// 时第 3 步红；拿不到闸分支恒 false（#14）时第 4 步红；三个分支恒 true 时第 1、5 步
// 及第 6 步前半红。第 6 步把内存与库设成相反的值，证明内存分支读的是内存。
func TestS10aSessionInfoBaselineSuspect(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	svc := rs.svc
	cfg := newTestConfig()
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "hello")
	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	key := sessionKey{convID: convID, agentID: testAgentClaude}

	// 第 1 步：成功一轮，内存分支为假。
	fs.enqueueSuccess(claudeT1JSON)
	if rec, err := r.Ask(ctx, convID, testAgentClaude); err != nil || rec.Outcome != OutcomeSpoke {
		t.Fatalf("前提不成立：首轮应当成功，err=%v outcome=%v", err, rec.Outcome)
	}
	if info, ok := r.SessionInfo(ctx, convID, testAgentClaude); !ok || info.BaselineSuspect {
		t.Errorf("第 1 步：ok=%v BaselineSuspect=%v，期望 ok 且为假", ok, info.BaselineSuspect)
	}

	// 第 2 步：超时一轮，内存分支为真。
	postUser(t, svc, convID, "second")
	fs.enqueue(spawnResult{killed: true, err: context.DeadlineExceeded})
	rec2, _ := r.Ask(ctx, convID, testAgentClaude)
	if rec2.Outcome != OutcomeTimeout {
		t.Fatalf("前提不成立：第二轮 Outcome = %v，期望 OutcomeTimeout", rec2.Outcome)
	}
	if v := s10aBaselineSuspectInDB(t, rs, convID, testAgentClaude); v != 1 {
		t.Fatalf("前提不成立：库里 baseline_suspect = %d，期望 1", v)
	}
	if info, ok := r.SessionInfo(ctx, convID, testAgentClaude); !ok || info.InFlight || !info.BaselineSuspect {
		t.Errorf("第 2 步：ok=%v InFlight=%v BaselineSuspect=%v，期望 ok、不在飞、为真", ok, info.InFlight, info.BaselineSuspect)
	}

	// 第 3 步：新 Runner，拿到闸、读库分支为真。
	r2 := newTestRunnerSameWorkDir(r, cfg, svc)
	if info, ok := r2.SessionInfo(ctx, convID, testAgentClaude); !ok || info.InFlight || !info.BaselineSuspect {
		t.Errorf("第 3 步：ok=%v InFlight=%v BaselineSuspect=%v，期望 ok、不在飞、为真", ok, info.InFlight, info.BaselineSuspect)
	}

	// 第 4 步：占住闸，拿不到闸分支为真。
	g := r2.gate(key)
	g.mu.Lock()
	info4, ok4 := r2.SessionInfo(ctx, convID, testAgentClaude)
	g.mu.Unlock()
	if !ok4 || !info4.InFlight || !info4.BaselineSuspect {
		t.Errorf("第 4 步：ok=%v InFlight=%v BaselineSuspect=%v，期望 ok、在飞、为真", ok4, info4.InFlight, info4.BaselineSuspect)
	}

	// 第 5 步：对照会话，库里为 0，两个读库分支都为假。
	conv2 := newConv(t, svc)
	joinAgent(t, svc, conv2, testAgentClaude)
	postUser(t, svc, conv2, "hello")
	fs.enqueueSuccess(claudeT1JSON)
	if rec, err := r.Ask(ctx, conv2, testAgentClaude); err != nil || rec.Outcome != OutcomeSpoke {
		t.Fatalf("前提不成立：对照会话首轮应当成功，err=%v outcome=%v", err, rec.Outcome)
	}
	if v := s10aBaselineSuspectInDB(t, rs, conv2, testAgentClaude); v != 0 {
		t.Fatalf("前提不成立：对照会话库里 baseline_suspect = %d，期望 0", v)
	}
	key2 := sessionKey{convID: conv2, agentID: testAgentClaude}
	r3 := newTestRunnerSameWorkDir(r, cfg, svc)
	if info, ok := r3.SessionInfo(ctx, conv2, testAgentClaude); !ok || info.InFlight || info.BaselineSuspect {
		t.Errorf("第 5 步（拿到闸）：ok=%v InFlight=%v BaselineSuspect=%v，期望 ok、不在飞、为假", ok, info.InFlight, info.BaselineSuspect)
	}
	g2 := r3.gate(key2)
	g2.mu.Lock()
	info5, ok5 := r3.SessionInfo(ctx, conv2, testAgentClaude)
	g2.mu.Unlock()
	if !ok5 || !info5.InFlight || info5.BaselineSuspect {
		t.Errorf("第 5 步（拿不到闸）：ok=%v InFlight=%v BaselineSuspect=%v，期望 ok、在飞、为假", ok5, info5.InFlight, info5.BaselineSuspect)
	}

	// 第 6 步：白盒，内存与库相反。
	r4 := newTestRunnerSameWorkDir(r, cfg, svc)
	r4.gate(key).st = &sessionState{providerSession: "s10a-mem", baselineSuspect: false}
	r4.gate(key2).st = &sessionState{providerSession: "s10a-mem", baselineSuspect: true}
	if info, ok := r4.SessionInfo(ctx, convID, testAgentClaude); !ok || info.BaselineSuspect {
		t.Errorf("第 6 步（内存假、库真）：ok=%v BaselineSuspect=%v，期望 ok 且为假", ok, info.BaselineSuspect)
	}
	if info, ok := r4.SessionInfo(ctx, conv2, testAgentClaude); !ok || !info.BaselineSuspect {
		t.Errorf("第 6 步（内存真、库假）：ok=%v BaselineSuspect=%v，期望 ok 且为真", ok, info.BaselineSuspect)
	}
}

// s10aCtxKey 是 G7 塞进调用方 ctx 的测试专用 key，包外与生产代码都拿不到它。
type s10aCtxKey struct{}

// s10aCtxMarker 是 G7 塞进去的值。
const s10aCtxMarker = "s10a-caller-ctx"

// s10aProbeStore 在 InflightFor 被调用的那一刻做两件观察，其余方法透传真库：
//   - 对本 (conv, agent) 的闸做一次 TryLock：拿到了说明检查跑在闸外，立刻 Unlock
//     还回去（不还的话 askUpToWith 随后的 g.mu.Lock() 会把用例挂死，而不是让它红）；
//   - 记下收到的 ctx 里 s10aCtxKey 的值。
//
// gate 由用例在 Ask 之前经 r.gate(key) 取好塞进来：gate 一经创建永不从 map 里移除
// （sessionGate 的不变量），同一个 key 在 Ask 里拿到的是同一个指针；探针自己不调
// r.gate，免得在 InflightFor 里碰 gatesMu。整个 Ask 在用例 goroutine 里同步执行，
// 这几个字段没有并发读写。Go 的 Mutex 不可重入，同一 goroutine 对已被持有的锁
// TryLock 必然返回 false，不靠任何时序。
type s10aProbeStore struct {
	*store.Store
	gate        *sessionGate
	calls       int
	sawGateFree bool
	ctxVal      any
}

func (s *s10aProbeStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	s.calls++
	s.ctxVal = ctx.Value(s10aCtxKey{})
	if s.gate.mu.TryLock() {
		s.gate.mu.Unlock()
		s.sawGateFree = true
	}
	return s.Store.InflightFor(ctx, convID, agentID)
}

// s10aProbeAsk 用 s10aProbeStore 装一个 Runner，带着塞了 s10aCtxKey 的调用方 ctx 跑一轮 Ask。
// 返回探针；前提守卫：InflightFor 恰好被调一次、本轮照常 spoke（没有外来行，检查放行）。
func s10aProbeAsk(t *testing.T) *s10aProbeStore {
	t.Helper()
	rs := newRealTurnStore(t)
	probe := &s10aProbeStore{Store: rs.st}
	svc := conversation.NewService(probe)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "hello")
	r := newTestRunner(t, newTestConfig(), svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	fs.enqueueSuccess(claudeT1JSON)
	probe.gate = r.gate(sessionKey{convID: convID, agentID: testAgentClaude})

	ctx := context.WithValue(context.Background(), s10aCtxKey{}, s10aCtxMarker)
	rec, err := r.Ask(ctx, convID, testAgentClaude)
	if probe.calls != 1 {
		t.Fatalf("前提不成立：一轮 Ask 里 InflightFor 被调了 %d 次，期望恰好 1 次", probe.calls)
	}
	if err != nil || rec.Outcome != OutcomeSpoke {
		t.Fatalf("前提不成立：没有外来行时本轮应当 spoke，err=%v outcome=%v", err, rec.Outcome)
	}
	return probe
}

// TestS10aInflightCheckRunsInsideGate 是 G6：起进程之前的在飞检查跑在本 (conv, agent) 的闸之内。
//
// 【判别力】契约 §4 原列为「只在审查时看」。检查整段挪到 g.mu.Lock() 之前（审查变异 P2）
// 时，探针的 TryLock 能拿到闸，sawGateFree 为真而红；好实现下闸正被本轮持有，TryLock
// 恒失败。能为假：P2 下确定性为假，不靠睡眠、不靠并发。对照是 G7（同一个探针、同一轮，
// 只看 ctx），以及前提守卫「InflightFor 恰好一次」——检查整段被删时守卫先红，
// 不会让本条因为探针根本没跑而空绿。
func TestS10aInflightCheckRunsInsideGate(t *testing.T) {
	probe := s10aProbeAsk(t)
	if probe.sawGateFree {
		t.Errorf("InflightFor 被调用时本 (conv, agent) 的闸是空闲的：检查跑在闸外，同 key 的两次并发调用会都看到「没有」")
	}
}

// TestS10aInflightCheckUsesCallerCtx 是 G7：在飞检查用的是调用方的 ctx。
//
// 【判别力】契约 §4 原列为「只在审查时看」。检查里的 ctx 换成 context.Background()
// （审查变异 P3）时，探针收到的 ctx 里拿不到 s10aCtxKey，ctxVal 为 nil 而红；好实现下
// 恒等于塞进去的 s10aCtxMarker。能为假：P3 下确定性为假。对照是 G6（同一个探针、同一轮，
// 只看闸）；前提守卫同 G6。
func TestS10aInflightCheckUsesCallerCtx(t *testing.T) {
	probe := s10aProbeAsk(t)
	if got, ok := probe.ctxVal.(string); !ok || got != s10aCtxMarker {
		t.Errorf("InflightFor 收到的 ctx 里 s10aCtxKey = %#v，期望 %q（检查没用调用方的 ctx）", probe.ctxVal, s10aCtxMarker)
	}
}
