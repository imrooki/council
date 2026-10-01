// 本文件覆盖 docs/12_STEP9_CONTRACT.md 3.9 第 4 条里点到、6.3 节展开的那半句：
// turn_usage 的两个新列（round_id、identity_fp）不能是"加了列但没人写"的
// 半空列——列有 NOT NULL DEFAULT ”，没人写的时候它是空串、查得出来、
// 不报错，一路静默，这正是这组用例要拆穿的事。
//
// 正面用例跑一轮真正成功的 Ask，断言那一行两列都非空；负面用例专门造一轮
// "在指纹算出来之前就失败"的轮次（agent 不在配置里），断言 identity_fp
// 是空串——空串在这里必须读作"这一轮没算到指纹"，不是"指纹是空的"，
// 与未决 #22 给七个用量列立的那条纪律（NULL 与 0 不能混成一回事）是同一类
// 区分，只是这里的可空性由 v4DDL 的 NOT NULL DEFAULT ” 定死，本片不能像
// 未决 #22 那样把列改成可空——区分只能靠"这一行代表的轮次到底有没有走到
// 指纹计算那一步"这层业务语义本身去读，列本身不会说话。
package turn

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"
)

// newTestServiceWithPath 与 fixtures_test.go 的 newTestService 功能相同
// （真实 SQLite store 包一层 conversation.Service），额外把库文件路径带出来。
// conversation.Store 接口没有给 turn_usage 表配 Go 层的读方法（store.go 原话：
// "Tx 存在的理由是事务边界，不是为了给每张表配一组 CRUD"），本文件的用例
// 要直接查 round_id / identity_fp 两列，只能绕开 Service 走原始连接，
// 因此需要库文件路径——newTestService 不返回它，这里另写一份而不是改
// fixtures_test.go，避免与实现方（正在独立写同一片实现）在共用文件上撞车。
func newTestServiceWithPath(t *testing.T) (*conversation.Service, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "council-data")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatalf("store.Open(%q) 失败: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试 store 失败: %v", err)
		}
	})
	return conversation.NewService(st), st.Path()
}

// tstTurnUsageRoundAndIdentity 用测试自己的连接直接查 turn_usage 的
// round_id 与 identity_fp 两列。用测试自己的连接而不是 Service：
// Service 没有给 turn_usage 配读方法，这是本文件存在的理由之一。
func tstTurnUsageRoundAndIdentity(t *testing.T, dbPath, turnID string) (roundID, identityFP string) {
	t.Helper()
	// busy_timeout 与 internal/store/open_test.go 的 tstRawDB 一致：库仍然
	// 开着（Store 那份写句柄没关），另开一条连接在 WAL 下短暂 SQLITE_BUSY
	// 是正常的，没有这个 pragma 会把一次瞬时争用误判成用例本身的红。
	db, err := sql.Open("sqlite",
		"file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("打开底层连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭底层连接失败: %v", err)
		}
	}()
	err = db.QueryRow(
		`SELECT round_id, identity_fp FROM turn_usage WHERE turn_id = ?`, turnID,
	).Scan(&roundID, &identityFP)
	if err != nil {
		t.Fatalf("读 turn_usage(turn_id=%q) 的 round_id/identity_fp 失败: %v", turnID, err)
	}
	return roundID, identityFP
}

// TestAskWritesNonEmptyRoundIDAndIdentityFP 是正面的那一半：一轮正常成功的
// Ask 之后，turn_usage 那一行的 round_id 与 identity_fp 都必须非空，
// 且 round_id 要与 Record.RoundID 一致——不是随便写了个非空值就算过。
func TestAskWritesNonEmptyRoundIDAndIdentityFP(t *testing.T) {
	cfg := newTestConfig()
	svc, dbPath := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "一轮正常的 Ask")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if rec.Outcome != OutcomeSpoke {
		t.Fatalf("Outcome = %v，期望 OutcomeSpoke（前置条件不成立，下面的断言无意义）", rec.Outcome)
	}

	roundID, identityFP := tstTurnUsageRoundAndIdentity(t, dbPath, rec.TurnID)
	if roundID == "" {
		t.Error("turn_usage.round_id 是空串，期望非空——加了列却没人写，是一列半空列")
	}
	if roundID != rec.RoundID {
		t.Errorf("turn_usage.round_id = %q，期望与 Record.RoundID 一致：%q", roundID, rec.RoundID)
	}
	if identityFP == "" {
		t.Error("turn_usage.identity_fp 是空串，期望非空——这一轮真的算出了指纹，不该记成没算到")
	}
}

// TestAskRefusedBeforeFingerprintLeavesIdentityFPEmpty 是负面的那一半：
// 一轮在指纹算出来之前就被拒绝的轮次（agent 根本不在配置里，runner.go
// 在 identityFPFor 之前就 return 了），turn_usage 那一行必须存在
// （TurnID 在 AskUpTo 一开始就铸好，persistTurn 只在 TurnID 为空时跳过），
// 且 round_id 仍然非空（它与指纹计算无关），但 identity_fp 必须是空串——
// 这是"没算到"，不是"算出来是空的"。
func TestAskRefusedBeforeFingerprintLeavesIdentityFPEmpty(t *testing.T) {
	cfg := newTestConfig()
	svc, dbPath := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	// 故意不 join、也不在配置里放这个 agent 名：Ask 应当在算指纹之前就
	// 因为"agent 不在配置里"（ErrAgentUnknown）拒绝，一分钱不花。

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{} // 不 enqueue 任何结果：这一轮不该调用 Spawner。
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, "agent-not-in-config")
	if err == nil {
		t.Fatal("agent 不在配置里，Ask 应当报错")
	}
	if rec.Outcome != OutcomeRefused {
		t.Fatalf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if len(fs.calls) != 0 {
		t.Fatalf("这一轮不该调用 Spawner，实际调用了 %d 次（起进程之前就该失败）", len(fs.calls))
	}
	if rec.TurnID == "" {
		t.Fatal("TurnID 为空，前置条件不成立：persistTurn 靠它判断要不要落库")
	}

	roundID, identityFP := tstTurnUsageRoundAndIdentity(t, dbPath, rec.TurnID)
	if roundID == "" {
		t.Error("turn_usage.round_id 是空串，期望非空——round_id 在 AskUpTo 一开始就铸好了，不该受指纹计算成败影响")
	}
	if identityFP != "" {
		t.Errorf("identity_fp = %q，期望空串（这一轮在指纹算出来之前就被拒绝了，读作\"没算到\"，不是\"算出来是空的\"）", identityFP)
	}
}
