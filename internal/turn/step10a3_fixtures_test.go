package turn

// 本文件是 Step 10 A 片之三（在飞记录写侧）专用的测试工装，供本目录下
// step10a3_*_test.go 的用例共用，不与 fixtures_test.go / recording_store_test.go
// 重复——那两个文件已经把"造 Runner / 记账"这件事做完了，本文件只加两种新能力：
//
//  1. 故障注入：让 BeginTurn / SetInflightPhase / ClearInflight 里的某一个方法
//     按需报错或撒谎，其余方法原样透传给内嵌的 recordingStore。三个包装类型
//     都是同一个体例：外层 Store 只重写 WriteTx，交出去的 Tx 只重写那一个
//     要作怪的方法，其余方法靠内嵌 *recordingStore 的方法集自动满足接口。
//     之所以能这样做：Service.BeginTurn / Service.SetInflightPhase 各自的
//     WriteTx 只做一件事（契约 §4.1），RecordTurn 的 WriteTx 里除
//     ClearInflight 外的步骤仍然要走真实实现，两者都只需要"整体透传、
//     单点作怪"，不需要重新实现整套记账。
//
//  2. 真实 *store.Store：recordingStore.WriteTx 只是把 fn 原地调用一次，
//     不做真实的提交/回滚（fn 已经直接改了内存状态，返回错误也不撤销）——
//     这在验证"expectInflight 传错时数据真的会不会回滚"这类断言时不够用，
//     必须换成货真价实的 SQLite 事务才能拆穿。newRealTurnStore 照
//     fixtures_test.go 的 newTestService 造，多留一个 *store.Store 的引用，
//     好在需要时直接查 turn_usage / provider_sessions 这类
//     conversation.Store 接口没有配读方法的表（recording_store_test.go
//     文件头那条注释同样的道理）。

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"

	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// 真库：只在需要真实提交/回滚语义时才用。
// ---------------------------------------------------------------------------

// realTurnStore 打包一个真实 *store.Store 与包在它上面的 *conversation.Service。
type realTurnStore struct {
	svc *conversation.Service
	st  *store.Store
}

// newRealTurnStore 在 t.TempDir() 下开一个真实的 SQLite store。
// 与 fixtures_test.go 的 newTestService 逻辑相同，多返回一个 *store.Store，
// 让调用方能在需要时直接查库文件。
func newRealTurnStore(t *testing.T) realTurnStore {
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
	return realTurnStore{svc: conversation.NewService(st), st: st}
}

// rawDB 在真库文件上另开一条测试专用连接，只用来直接查表——
// 照 internal/store 自己测试的体例（provider_session_test.go 的 tstRawDB）。
func (r realTurnStore) rawDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.ToSlash(r.st.Path()) + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("打开底层连接失败: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭底层连接失败: %v", err)
		}
	})
	return db
}

// turnUsageRowCount 数 turn_usage 里某个 turn_id 的行数。
// conversation.Store 接口没有给 turn_usage 配读方法（recording_store_test.go
// 文件头那条注释同样的道理），只能绕到底层连接直接查。
func (r realTurnStore) turnUsageRowCount(t *testing.T, turnID string) int64 {
	t.Helper()
	db := r.rawDB(t)
	var n int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM turn_usage WHERE turn_id = ?`, turnID).Scan(&n); err != nil {
		t.Fatalf("查询 turn_usage 失败: %v", err)
	}
	return n
}

// ---------------------------------------------------------------------------
// 故障注入一：BeginTurn 恒定报错。
// ---------------------------------------------------------------------------

// beginTurnFailingStore 包一层 recordingStore：WriteTx 交出去的 Tx 把
// BeginTurn 换成恒定报错的实现，其余方法（CreateConversation /
// AddParticipant / Post 等要用到的那些）全部透传给内嵌的 recordingStore。
// Service.BeginTurn 那一笔 WriteTx 只调用 tx.BeginTurn 一个方法，其余方法
// 在这条特定路径上根本不会被调用，但类型仍要满足 conversation.Tx 的
// 全部方法签名，嵌入是最省代码、也最不容易漏掉哪个方法的办法。
type beginTurnFailingStore struct {
	*recordingStore
	err error
}

func (s *beginTurnFailingStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return fn(ctx, beginTurnFailingTx{recordingStore: s.recordingStore, err: s.err})
}

type beginTurnFailingTx struct {
	*recordingStore
	err error
}

func (t beginTurnFailingTx) BeginTurn(ctx context.Context, it conversation.InflightTurn) error {
	return t.err
}

// ---------------------------------------------------------------------------
// 故障注入二：SetInflightPhase 恒定报错（不管传的是哪个 phase）。
// ---------------------------------------------------------------------------

// setInflightPhaseFailingStore 与 beginTurnFailingStore 同一个体例，换成
// SetInflightPhase 恒定报错。BeginTurn 本身透传给 recordingStore 的真实
// 实现——落点 B 的场景要求 BeginTurn 先成功（此刻 began 已经是 true），
// 只有随后 phase=spawned 这一笔写失败。
type setInflightPhaseFailingStore struct {
	*recordingStore
	err error
}

func (s *setInflightPhaseFailingStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return fn(ctx, setInflightPhaseFailingTx{recordingStore: s.recordingStore, err: s.err})
}

type setInflightPhaseFailingTx struct {
	*recordingStore
	err error
}

func (t setInflightPhaseFailingTx) SetInflightPhase(ctx context.Context, turnID string, phase conversation.InflightPhase) error {
	return t.err
}

// ---------------------------------------------------------------------------
// 故障注入三：ClearInflight 恒定报告 existed=false（不管这一行实际在不在）。
// ---------------------------------------------------------------------------

// clearInflightNeverExistsStore 模拟"RecordTurn 调用 ClearInflight 的那一刻，
// 这一行已经不在了"这种极端时序（不追问是什么导致的——本刀不关心怎么会
// 发生，只关心 expectInflight 传对了没有：existed=false 时，expectInflight
// 为 true 必须让整笔 RecordTurn 报错，为 false 则照常放过）。
// 仍然记账（clearInflightCalls 照常追加，供断言核对确实被调用过），
// 只是 existed 恒定为 false，与 recordingStore 真实的 ClearInflight 只差这一点。
type clearInflightNeverExistsStore struct {
	*recordingStore
}

func (s *clearInflightNeverExistsStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return fn(ctx, clearInflightNeverExistsTx{recordingStore: s.recordingStore})
}

type clearInflightNeverExistsTx struct {
	*recordingStore
}

func (t clearInflightNeverExistsTx) ClearInflight(ctx context.Context, turnID string) (bool, error) {
	t.recordingStore.clearInflightCalls = append(t.recordingStore.clearInflightCalls, turnID)
	return false, nil
}
