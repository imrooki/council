// inflight_recordturn_test.go 覆盖 Step 10 契约 §3.15 单测 5 的主断言：
// Service.RecordTurn 新增的 expectInflight 参数，以及 expectInflight=true
// 但没有在飞记录时必须返回 ErrInflightMissing 并整轮回滚。
//
// 本文件放在 internal/store，不是任务书原先建议的 internal/conversation，
// 是本文件落盘时刻意做的判断，理由有三条：
//
//  1. internal/conversation 那份 fakeStore 明确不做回滚——service_test.go
//     里 fakeStore.WriteTx 的注释原话是「这里不做回滚：事务语义属于
//     internal/store 的测试面，本包只关心 Service 有没有走 WriteTx」，
//     RecordUsage 也是 `return nil` 的空桩，从不记录写过什么。放在
//     internal/conversation 的话，本组用例最关键的断言——「turn_usage 没有
//     新增行」——对任何实现都恒真，是一条测不出问题的空断言。
//  2. 更硬的一条：RecordTurn 内部要调 tx.ClearInflight，conversation.Tx
//     接口必然因此新增这个方法；fakeStore 若不给它补一个桩，整个
//     internal/conversation 测试二进制都编不过，而补桩要去改
//     service_test.go——任务书禁令 1 不许改既有 _test.go 文件。实现方
//     落地时大概率也要给 fakeStore 补同一个桩，两边都补就成了重复定义、
//     互相打架。
//  3. internal/store/record_turn_test.go 的文件头已经把「真实的事务原子性
//     只有真库能验，调用顺序层面的结构性验证才归 internal/conversation」
//     写成了本仓库现成的体例，本文件照抄同一个理由、同一套写法：真实
//     *store.Store、真实 conversation.NewService(s)、真实 WriteTx。
//
// 落盘时 conversation.ErrInflightMissing 与 RecordTurn 的第五个参数
// expectInflight 都还不存在，编译不过是预期。
package store

import (
	"errors"
	"testing"

	"council/internal/conversation"
)

// TestRecordTurnExpectInflightMissingRollsBackWholeTurn 覆盖用例九，本组的
// 主断言：不调 BeginTurn，直接 RecordTurn(expectInflight: true)。
//
// 断言分两层：errors.Is 认出 ErrInflightMissing 是第一层；「turn_usage 没有
// 新增行、整个事务回滚了」是第二层，也是关键的那层——只断言错误类型的话，
// 一个「报了错但照样提交」的坏实现也能让第一层过。
func TestRecordTurnExpectInflightMissingRollsBackWholeTurn(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "expectInflight=true 但没有 BeginTurn")
	tstJoinAlpha(t, ctx, s, conv.ID)

	msg := tstAgentMsg(conv.ID, "alpha", "没有在飞记录却要求必须有")
	u := tstBaseUsage(conv.ID, "alpha", "turn-inflight-missing-1")

	_, err := svc.RecordTurn(ctx, &msg, u, nil, true)
	if !errors.Is(err, conversation.ErrInflightMissing) {
		t.Fatalf("errors.Is(err, ErrInflightMissing) 应当为真，实际: %v", err)
	}

	if n := tstMessageCount(t, ctx, s, conv.ID); n != 0 {
		t.Errorf("messages 有 %d 行，期望 0 行（整轮回滚）", n)
	}
	raw := tstRawDB(t, s.Path())
	if n := tstUsageRowCount(t, raw, u.TurnID); n != 0 {
		t.Errorf("turn_usage 有 %d 行，期望 0 行——这是本用例的关键：只断言错误类型"+
			"挡不住「报了错但照样提交」这种坏实现", n)
	}
	if _, ok, err := s.ProviderSession(ctx, conv.ID, "alpha"); err != nil || ok {
		t.Errorf("provider_sessions 应当没有行，实际 ok=%v err=%v", ok, err)
	}
}

// TestRecordTurnExpectInflightFalseCommitsRegardlessOfInflightRow 覆盖用例
// 十：对照组。同样不调 BeginTurn，但 expectInflight=false，断言正常提交。
//
// 没有这条对照，上一条「回滚」的断言测不出「无论 expectInflight 是什么都回滚」
// 这种更粗暴的坏实现——例如把 ClearInflight 的 existed=false 恒当失败处理，
// 不管调用方是不是真的要求过必须存在。
func TestRecordTurnExpectInflightFalseCommitsRegardlessOfInflightRow(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "expectInflight=false 且没有 BeginTurn")
	tstJoinAlpha(t, ctx, s, conv.ID)

	msg := tstAgentMsg(conv.ID, "alpha", "没有在飞记录，也不要求必须有")
	u := tstBaseUsage(conv.ID, "alpha", "turn-inflight-missing-2")

	stored, err := svc.RecordTurn(ctx, &msg, u, nil, false)
	if err != nil {
		t.Fatalf("RecordTurn 应当成功，实际: %v", err)
	}
	if stored == nil {
		t.Fatal("成功路径返回的消息是 nil")
	}

	if n := tstMessageCount(t, ctx, s, conv.ID); n != 1 {
		t.Errorf("messages 有 %d 行，期望 1 行（正常提交）", n)
	}
	raw := tstRawDB(t, s.Path())
	if n := tstUsageRowCount(t, raw, u.TurnID); n != 1 {
		t.Errorf("turn_usage 有 %d 行，期望 1 行（正常提交）", n)
	}
}

// TestRecordTurnExpectInflightTrueClearsRowOnSuccess 覆盖用例十一，正路：
// 先 BeginTurn 再 RecordTurn(expectInflight: true)，断言正常提交，
// 且在飞记录被 ClearInflight 删掉了（AllInflight 里找不到这个 turn_id）。
func TestRecordTurnExpectInflightTrueClearsRowOnSuccess(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "先 BeginTurn 再 RecordTurn(expectInflight=true)")
	tstJoinAlpha(t, ctx, s, conv.ID)

	const turnID = "turn-inflight-missing-3"
	// InflightTurn.TurnID 必须与 TurnUsage.TurnID 是同一个字符串：RecordTurn
	// 内部按 u.TurnID 调 ClearInflight，两个不一致的话这条用例会以
	// ErrInflightMissing 失败，看着像实现错，其实是这里没对齐。
	it := tstBaseInflight(conv.ID, "alpha", turnID)
	if err := tstBeginTurn(t, ctx, s, it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	msg := tstAgentMsg(conv.ID, "alpha", "有在飞记录，正路")
	u := tstBaseUsage(conv.ID, "alpha", turnID)

	stored, err := svc.RecordTurn(ctx, &msg, u, nil, true)
	if err != nil {
		t.Fatalf("RecordTurn 应当成功，实际: %v", err)
	}
	if stored == nil {
		t.Fatal("成功路径返回的消息是 nil")
	}

	if n := tstMessageCount(t, ctx, s, conv.ID); n != 1 {
		t.Errorf("messages 有 %d 行，期望 1 行", n)
	}

	for _, row := range tstAllInflight(t, ctx, s) {
		if row.TurnID == turnID {
			t.Errorf("RecordTurn 成功之后在飞记录还在: %+v，期望已被 ClearInflight 删掉", row)
		}
	}
}
