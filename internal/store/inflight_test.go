// inflight_test.go 覆盖 Step 10 契约 v5 schema 之上、conversation.Tx 新增的
// 三个在飞记录方法（BeginTurn / SetInflightPhase / ClearInflight）与
// Store.AllInflight 的往返，UNIQUE(conv_id, agent_id) 在领域层是否也挡得住，
// 以及 provider_sessions.baseline_suspect 新列跟着整行写入/读回。
//
// 本文件落盘时 internal/conversation 的 InflightTurn / InflightPhase、
// Tx 新方法、Store.AllInflight、ProviderSession.BaselineSuspect 都还不存在，
// 编译不过是预期（背对背，实现方正在并行写这些符号）。
//
// 照 internal/store 既有测试的体例：真实 *Store、真实 WriteTx，
// tstStore / tstCtx / tstNewConv 等辅助函数直接复用 open_test.go 里已有的，
// tstBaseSession / tstSetProviderSession 复用 provider_session_test.go 的。
package store

import (
	"context"
	"testing"
	"time"

	"council/internal/conversation"
)

// tstFixedInflightTime 是本文件专用的固定时刻，带真实纳秒精度，与
// usage_test.go 的 tstFixedUsageTime、provider_session_test.go 的
// tstFixedSessionTime 分开取值——同一体例，互不掩盖对方的 bug。
var tstFixedInflightTime = time.Date(2026, 9, 21, 3, 17, 42, 543219876, time.UTC)

// tstBaseInflight 造一条 12 个字段两两可辨认的在飞记录，调用方按需覆盖。
// 「可辨认」是故意的：PlannedSessionID/ResumeSessionID、CursorSeq/ScannedTo、
// Tier/WorkDir 这几对容易被实现顺手写串的字段全部取不同值，防止两个恰好相等
// 的字段把「写串了列」这种坏实现掩盖过去。
func tstBaseInflight(convID, agentID, turnID string) conversation.InflightTurn {
	return conversation.InflightTurn{
		TurnID:           turnID,
		ConvID:           convID,
		AgentID:          agentID,
		Provider:         "claude",
		PlannedSessionID: "planned-" + turnID,
		ResumeSessionID:  "resume-" + turnID,
		CursorSeq:        7,
		ScannedTo:        12,
		StartedAt:        tstFixedInflightTime,
		Phase:            conversation.InflightPhase("begun"),
		Tier:             "tier-heavy",
		WorkDir:          "/work/" + turnID,
	}
}

// tstBeginTurn 在一个写事务里调 Tx.BeginTurn。
func tstBeginTurn(t *testing.T, ctx context.Context, s *Store, it conversation.InflightTurn) error {
	t.Helper()
	return s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.BeginTurn(ctx, it)
	})
}

// tstSetInflightPhase 在一个写事务里调 Tx.SetInflightPhase。
func tstSetInflightPhase(t *testing.T, ctx context.Context, s *Store, turnID string, phase conversation.InflightPhase) error {
	t.Helper()
	return s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetInflightPhase(ctx, turnID, phase)
	})
}

// tstClearInflight 在一个写事务里调 Tx.ClearInflight，把 existed 带出来。
func tstClearInflight(t *testing.T, ctx context.Context, s *Store, turnID string) (existed bool, err error) {
	t.Helper()
	err = s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var txErr error
		existed, txErr = tx.ClearInflight(ctx, turnID)
		return txErr
	})
	return existed, err
}

// tstAllInflight 读出全库的在飞记录，失败直接判此用例不通过。
func tstAllInflight(t *testing.T, ctx context.Context, s *Store) []conversation.InflightTurn {
	t.Helper()
	all, err := s.AllInflight(ctx)
	if err != nil {
		t.Fatalf("AllInflight 失败: %v", err)
	}
	return all
}

// tstFindInflight 从 AllInflight 的结果里按 turn_id 找一条，找不到直接判此用例不通过。
func tstFindInflight(t *testing.T, all []conversation.InflightTurn, turnID string) conversation.InflightTurn {
	t.Helper()
	for _, it := range all {
		if it.TurnID == turnID {
			return it
		}
	}
	t.Fatalf("AllInflight 结果里没有 turn_id=%q：%+v", turnID, all)
	return conversation.InflightTurn{}
}

// tstAssertInflightEqual 逐字段比较两条在飞记录，label 用于失败信息区分是
// 哪一轮比较。
func tstAssertInflightEqual(t *testing.T, got, want conversation.InflightTurn, label string) {
	t.Helper()
	if got.TurnID != want.TurnID {
		t.Errorf("%s: TurnID = %q，期望 %q", label, got.TurnID, want.TurnID)
	}
	if got.ConvID != want.ConvID {
		t.Errorf("%s: ConvID = %q，期望 %q", label, got.ConvID, want.ConvID)
	}
	if got.AgentID != want.AgentID {
		t.Errorf("%s: AgentID = %q，期望 %q", label, got.AgentID, want.AgentID)
	}
	if got.Provider != want.Provider {
		t.Errorf("%s: Provider = %q，期望 %q", label, got.Provider, want.Provider)
	}
	if got.PlannedSessionID != want.PlannedSessionID {
		t.Errorf("%s: PlannedSessionID = %q，期望 %q", label, got.PlannedSessionID, want.PlannedSessionID)
	}
	if got.ResumeSessionID != want.ResumeSessionID {
		t.Errorf("%s: ResumeSessionID = %q，期望 %q", label, got.ResumeSessionID, want.ResumeSessionID)
	}
	if got.CursorSeq != want.CursorSeq {
		t.Errorf("%s: CursorSeq = %d，期望 %d", label, got.CursorSeq, want.CursorSeq)
	}
	if got.ScannedTo != want.ScannedTo {
		t.Errorf("%s: ScannedTo = %d，期望 %d", label, got.ScannedTo, want.ScannedTo)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("%s: StartedAt = %v，期望 %v", label, got.StartedAt, want.StartedAt)
	}
	if got.Phase != want.Phase {
		t.Errorf("%s: Phase = %q，期望 %q", label, got.Phase, want.Phase)
	}
	if got.Tier != want.Tier {
		t.Errorf("%s: Tier = %q，期望 %q", label, got.Tier, want.Tier)
	}
	if got.WorkDir != want.WorkDir {
		t.Errorf("%s: WorkDir = %q，期望 %q", label, got.WorkDir, want.WorkDir)
	}
}

// TestBeginTurnAndAllInflightRoundTripAllTwelveFields 覆盖用例一：BeginTurn
// 写一条 12 个字段都填了可辨认值的记录，AllInflight 读回来逐字段比对
// （包括 StartedAt 的时刻与 CursorSeq / ScannedTo）。
// 挡的是「某个字段没写进去 / 写串了列」。
func TestBeginTurnAndAllInflightRoundTripAllTwelveFields(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "在飞记录往返")

	want := tstBaseInflight(conv.ID, "alpha", "turn-inflight-roundtrip-1")
	if err := tstBeginTurn(t, ctx, s, want); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	got := tstFindInflight(t, tstAllInflight(t, ctx, s), want.TurnID)
	tstAssertInflightEqual(t, got, want, "往返")
}

// TestSetInflightPhaseOnlyChangesPhase 覆盖用例二：SetInflightPhase 把一条
// 记录改成 spawned，再读回来断言 Phase 变了、其余 11 个字段没变。
// 挡的是「顺手把别的列也写了」。两次都从 AllInflight 读回（before/after）逐字段比，
// 不拿传给 BeginTurn 的原始结构体比——那样比不出「读回来的时候被悄悄改写」这种坏实现。
func TestSetInflightPhaseOnlyChangesPhase(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "只改 phase 不动其余列")

	orig := tstBaseInflight(conv.ID, "alpha", "turn-inflight-phase-1")
	if err := tstBeginTurn(t, ctx, s, orig); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}
	before := tstFindInflight(t, tstAllInflight(t, ctx, s), orig.TurnID)

	const newPhase = conversation.InflightPhase("spawned")
	if err := tstSetInflightPhase(t, ctx, s, orig.TurnID, newPhase); err != nil {
		t.Fatalf("SetInflightPhase 失败: %v", err)
	}

	after := tstFindInflight(t, tstAllInflight(t, ctx, s), orig.TurnID)
	if after.Phase != newPhase {
		t.Errorf("Phase = %q，期望 %q", after.Phase, newPhase)
	}
	want := before
	want.Phase = newPhase
	tstAssertInflightEqual(t, after, want, "改 phase 之后")
}

// TestClearInflightReportsExistedAndIsIdempotent 覆盖用例三：ClearInflight
// 删掉一条记录，断言 existed == true；再删一次，断言 existed == false 且不报错。
// 挡的是「用 error 表达不存在」与「第二次删也报 existed=true」这两种坏实现。
func TestClearInflightReportsExistedAndIsIdempotent(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "ClearInflight 幂等")

	it := tstBaseInflight(conv.ID, "alpha", "turn-inflight-clear-1")
	if err := tstBeginTurn(t, ctx, s, it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	existed, err := tstClearInflight(t, ctx, s, it.TurnID)
	if err != nil {
		t.Fatalf("第一次 ClearInflight 报错: %v（这一行本来存在，不该报错）", err)
	}
	if !existed {
		t.Error("第一次 ClearInflight existed = false，期望 true（这一行本来存在）")
	}

	existed2, err2 := tstClearInflight(t, ctx, s, it.TurnID)
	if err2 != nil {
		t.Errorf("第二次 ClearInflight 报错: %v，期望 nil——挡的是「用 error 表达不存在」", err2)
	}
	if existed2 {
		t.Error("第二次 ClearInflight existed = true，期望 false——挡的是「第二次删也报 existed=true」")
	}

	for _, row := range tstAllInflight(t, ctx, s) {
		if row.TurnID == it.TurnID {
			t.Errorf("ClearInflight 之后这一行还在: %+v", row)
		}
	}
}

// TestSetInflightPhaseOnMissingTurnIDErrors 覆盖用例四：SetInflightPhase 对
// 一个不存在的 turn_id 必须报错。挡的是「静默通过」——那会把一次真实的丢失盖掉。
func TestSetInflightPhaseOnMissingTurnIDErrors(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)

	err := tstSetInflightPhase(t, ctx, s, "turn-does-not-exist", conversation.InflightPhase("spawned"))
	if err == nil {
		t.Fatal("SetInflightPhase 对不存在的 turn_id 竟然成功了，期望报错")
	}
}

// TestAllInflightOrdersByStartedAtAscending 覆盖用例五：写三条 started_at
// 乱序的记录（插入顺序刻意不按时间），断言 AllInflight 返回的是按 started_at
// 升序。挡的是「按插入顺序 / rowid 返回」。
//
// 三条记录用同一个会话、三个不同的 agent_id：inflight_turns.conv_id 没有
// REFERENCES（schema.go v5DDL 注释），用同一个会话合法；(conv_id, agent_id)
// 三条两两不同，不会撞上用例七要测的 UNIQUE 约束。
func TestAllInflightOrdersByStartedAtAscending(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "AllInflight 按 started_at 升序")

	base := tstFixedInflightTime
	mid := tstBaseInflight(conv.ID, "agent-mid", "turn-inflight-order-mid")
	mid.StartedAt = base.Add(1 * time.Hour)
	late := tstBaseInflight(conv.ID, "agent-late", "turn-inflight-order-late")
	late.StartedAt = base.Add(2 * time.Hour)
	early := tstBaseInflight(conv.ID, "agent-early", "turn-inflight-order-early")
	early.StartedAt = base

	// 插入顺序故意不是时间顺序：先 mid、再 late、最后 early。
	if err := tstBeginTurn(t, ctx, s, mid); err != nil {
		t.Fatalf("BeginTurn(mid) 失败: %v", err)
	}
	if err := tstBeginTurn(t, ctx, s, late); err != nil {
		t.Fatalf("BeginTurn(late) 失败: %v", err)
	}
	if err := tstBeginTurn(t, ctx, s, early); err != nil {
		t.Fatalf("BeginTurn(early) 失败: %v", err)
	}

	var gotOrder []string
	for _, it := range tstAllInflight(t, ctx, s) {
		switch it.TurnID {
		case early.TurnID, mid.TurnID, late.TurnID:
			gotOrder = append(gotOrder, it.TurnID)
		}
	}
	want := []string{early.TurnID, mid.TurnID, late.TurnID}
	if len(gotOrder) != len(want) {
		t.Fatalf("AllInflight 里这三条各自出现的次数不对，实际看到 %v", gotOrder)
	}
	for i := range want {
		if gotOrder[i] != want[i] {
			t.Errorf("AllInflight 顺序 = %v，期望按 started_at 升序 %v（挡的是按插入顺序/rowid 返回）", gotOrder, want)
			break
		}
	}
}

// TestAllInflightEmptyIsEmptySliceNotNil 覆盖用例六：空表时 AllInflight 返回
// 空切片而不是 nil。两条断言分开写：只测 len==0 抓不住「返回了 nil」，
// 只测非 nil 抓不住「返回了非空结果」，两条都过才能排除这两类坏实现。
func TestAllInflightEmptyIsEmptySliceNotNil(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)

	all, err := s.AllInflight(ctx)
	if err != nil {
		t.Fatalf("AllInflight 失败: %v", err)
	}
	if all == nil {
		t.Error("AllInflight 空表时返回了 nil，期望非 nil 的空切片")
	}
	if len(all) != 0 {
		t.Errorf("AllInflight 空表时返回了 %d 条，期望 0 条", len(all))
	}
}

// TestBeginTurnRejectsSecondRowForSameConvAgent 覆盖用例七：UNIQUE(conv_id,
// agent_id) 在领域层（经 Tx.BeginTurn，不是拼 raw SQL）也挡得住。
// 同一个 (conv_id, agent_id)、不同 turn_id，第二次 BeginTurn 必须报错。
//
// 与 schema_v5_test.go 的 TestSchemaV5InflightTurnsUniqueConvIDAgentID 不是
// 重复用例：那条直接拼 INSERT 验证的是 DDL 里这条唯一索引本身存在；这条走
// Tx.BeginTurn，验证的是领域层没有把这条约束吞掉（例如实现里先 SELECT 存在
// 与否再决定 INSERT/UPDATE，UNIQUE 索引本身反而用不上）。
func TestBeginTurnRejectsSecondRowForSameConvAgent(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "UNIQUE(conv_id, agent_id) 领域层")

	first := tstBaseInflight(conv.ID, "alpha", "turn-inflight-unique-1")
	if err := tstBeginTurn(t, ctx, s, first); err != nil {
		t.Fatalf("第一次 BeginTurn 应当成功，实际: %v", err)
	}

	second := tstBaseInflight(conv.ID, "alpha", "turn-inflight-unique-2")
	if err := tstBeginTurn(t, ctx, s, second); err == nil {
		t.Error("同一个 (conv_id, agent_id) 用不同 turn_id 第二次 BeginTurn 竟然成功了，期望报错")
	}
}

// TestSetProviderSessionRoundTripsBaselineSuspect 覆盖用例八：
// SetProviderSession 写一条 BaselineSuspect: true 的行，读回来断言仍是 true；
// 再对同一个 (conv, agent) upsert 成 false，读回来是 false。
//
// 第二步是关键：另起一行写 false 挡不住「INSERT 列了 baseline_suspect，
// ON CONFLICT DO UPDATE 漏了它」这种坏实现——那种实现只在第一次 INSERT 时
// 把列写对，upsert 覆盖时会保留旧值，必须对同一个 (conv, agent) 再写一次
// 不同的值才挡得住。挡的是「新列没进整行写」与「读的时候恒当 false」。
func TestSetProviderSessionRoundTripsBaselineSuspect(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "BaselineSuspect 往返")

	trueSession := tstBaseSession(conv.ID, "alpha")
	trueSession.BaselineSuspect = true
	tstSetProviderSession(t, ctx, s, trueSession)

	got, ok, err := s.ProviderSession(ctx, conv.ID, "alpha")
	if err != nil || !ok {
		t.Fatalf("ProviderSession 返回 (%+v, %v, %v)", got, ok, err)
	}
	if !got.BaselineSuspect {
		t.Error("BaselineSuspect 读回 false，期望 true")
	}

	falseSession := tstBaseSession(conv.ID, "alpha")
	falseSession.BaselineSuspect = false
	tstSetProviderSession(t, ctx, s, falseSession)

	got2, ok2, err2 := s.ProviderSession(ctx, conv.ID, "alpha")
	if err2 != nil || !ok2 {
		t.Fatalf("第二次 ProviderSession 返回 (%+v, %v, %v)", got2, ok2, err2)
	}
	if got2.BaselineSuspect {
		t.Error("upsert 成 false 之后 BaselineSuspect 仍读回 true，期望 false（UPDATE 没覆盖这一列）")
	}
}
