// step10a4_settle_lost_turn_test.go 覆盖 Step 10 A 片之四冻结契约
// （E:\multi_model_cowork\step10a4_contract.md）第 6 节验收清单里
// 「哪些行真的存在、列里是什么值」这一类断言：第 1（begun/populated 两支）、
// 2、5、6 条，外加一条契约没写但静默出错风险很高的补充用例（没有在飞记录
// 时 SettleLostTurn 不该报错）。
//
// Service.SettleLostTurn 本身是一个薄封装：把调用方给的 msg/u/session
// 原样收进同一笔事务。构造 msg/u/session 的业务逻辑（三个 0、五个 nil、
// 整行拷贝只改 BaselineSuspect、哪个 phase 传不传 nil）在
// internal/turn/recover.go 里，那部分的判别力见
// internal/turn/step10a4_settle_lost_turns_test.go——本文件只测
// SettleLostTurn 自己对「给定这些参数，库里最终长什么样」这件事忠不忠实。
//
// 必须用真实 *store.Store：conversation.NewService(假实现) 的假实现不做
// 真实的事务回滚（本包 inflight_recordturn_test.go 文件头同一条理由），
// 「会话已不存在」那一支尤其依赖真实的 requireConversation 与整笔事务回滚。
//
// 落盘时 conversation.Service.SettleLostTurn / conversation.LostSettlement
// 都还不存在，编译不过是预期（背对背，实现腿正在并行写这些符号）。
package store

import (
	"database/sql"
	"testing"

	"council/internal/conversation"
)

// TestSettleLostTurnAllNilOnlyDeletesInflightRow 覆盖验收第 1 条的 begun 支：
// msg/u/session 三者全 nil 时只删在飞记录这一行，不标 baseline_suspect
// （没有 session 可标）、不发面包屑（没有 msg）、不补 turn_usage（没有 u）。
func TestSettleLostTurnAllNilOnlyDeletesInflightRow(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "settle：三者全 nil 只删行")

	const turnID = "turn-a4-begun-1"
	it := tstBaseInflight(conv.ID, "alpha", turnID)
	it.Phase = conversation.InflightPhase("begun")
	if err := tstBeginTurn(t, ctx, s, it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	settlement, err := svc.SettleLostTurn(ctx, turnID, nil, nil, nil)
	if err != nil {
		t.Fatalf("SettleLostTurn 应当成功，实际: %v", err)
	}
	if !settlement.Existed {
		t.Error("Existed 应当为 true（这一行本来存在）")
	}
	if settlement.SuspectMarked {
		t.Error("三者全 nil 时不该标 SuspectMarked（没有 session 可标）")
	}
	if settlement.ConvMissing {
		t.Error("会话本来就在，ConvMissing 应当是 false")
	}

	for _, row := range tstAllInflight(t, ctx, s) {
		if row.TurnID == turnID {
			t.Errorf("在飞记录还在: %+v，期望已被删掉", row)
		}
	}
	if n := tstMessageCount(t, ctx, s, conv.ID); n != 0 {
		t.Errorf("messages 有 %d 行，期望 0 行（三者全 nil 不该发面包屑）", n)
	}
	raw := tstRawDB(t, s.Path())
	if n := tstUsageRowCount(t, raw, turnID); n != 0 {
		t.Errorf("turn_usage 有 %d 行，期望 0 行（三者全 nil 不该补账）", n)
	}
}

// TestSettleLostTurnNoInflightRow_NotAnError 覆盖契约没有明写、但静默出错
// 风险很高的一条：ClearInflight 报 existed=false 不是错误（契约 4.2 第 5
// 点），这里直接对一个从未 BeginTurn 过的 turnID 调 SettleLostTurn。
// 挡的是照抄 RecordTurn 的 expectInflight 语义（那边 existed=false 在
// expectInflight=true 时是结构性 bug，会整笔回滚）——对账这一侧只意味着
// 「别人已经处理过了」，不该报错。
func TestSettleLostTurnNoInflightRow_NotAnError(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)

	settlement, err := svc.SettleLostTurn(ctx, "turn-a4-never-existed", nil, nil, nil)
	if err != nil {
		t.Fatalf("ClearInflight existed=false 不是错误，SettleLostTurn 不该报错，实际: %v", err)
	}
	if settlement.Existed {
		t.Error("Existed 应当是 false：这一行从来没存在过")
	}
	if settlement.SuspectMarked || settlement.ConvMissing {
		t.Errorf("SuspectMarked/ConvMissing 都应当是 false，实际 %+v", settlement)
	}
}

// TestSettleLostTurnPopulatedArgs_MarksRecordsAndBreadcrumb 覆盖验收第 1 条
// 的「populated」支（对应 spawned/returned 两档的共同形状）与第 6 条
// （Lost 行的三个 0 与七个 nil）：msg/u/session 都非 nil 时，一笔事务里
// 追加面包屑、补 turn_usage（outcome=9，用量与 cost 列全 NULL，三个
// NOT NULL 列全 0）、并把 baseline_suspect 置 1。
func TestSettleLostTurnPopulatedArgs_MarksRecordsAndBreadcrumb(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "settle：spawned/returned 形状")

	const agentID = "alpha"
	const turnID = "turn-a4-spawned-1"

	session := tstBaseSession(conv.ID, agentID)
	session.BaselineSuspect = false
	tstSetProviderSession(t, ctx, s, session)

	it := tstBaseInflight(conv.ID, agentID, turnID)
	it.Phase = conversation.InflightPhase("spawned")
	if err := tstBeginTurn(t, ctx, s, it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	msg := &conversation.Message{
		ConvID:   conv.ID,
		SenderID: conversation.ReservedSenderSystem,
		Kind:     conversation.KindSystem,
		Text:     "agent alpha 本轮在上次退出时丢失（settle 测试构造）",
		Meta:     map[string]any{"event": "turn_lost", "about_agent": agentID},
	}
	u := &conversation.TurnUsage{
		TurnID:            turnID,
		ConvID:            conv.ID,
		AgentID:           agentID,
		ProviderSessionID: session.ProviderSessionID,
		Seq:               nil,
		Outcome:           9,
		ColdStart:         false,
		WallMS:            0,
		PromptBytes:       0,
		CreatedAt:         it.StartedAt,
	}
	sessionCopy := session
	sessionCopy.BaselineSuspect = true

	settlement, err := svc.SettleLostTurn(ctx, turnID, msg, u, &sessionCopy)
	if err != nil {
		t.Fatalf("SettleLostTurn 应当成功，实际: %v", err)
	}
	if !settlement.Existed {
		t.Error("Existed 应当是 true")
	}
	if !settlement.SuspectMarked {
		t.Error("session 非 nil 且提交成功，SuspectMarked 应当是 true")
	}
	if settlement.ConvMissing {
		t.Error("会话存在，ConvMissing 应当是 false")
	}

	for _, row := range tstAllInflight(t, ctx, s) {
		if row.TurnID == turnID {
			t.Errorf("在飞记录还在: %+v，期望已被删掉", row)
		}
	}
	if n := tstMessageCount(t, ctx, s, conv.ID); n != 1 {
		t.Fatalf("messages 有 %d 行，期望 1 行（面包屑）", n)
	}

	raw := tstRawDB(t, s.Path())
	row := tstScanUsageRow(t, raw, turnID)
	if row.outcome != 9 {
		t.Errorf("outcome = %d，期望 9（OutcomeLost）", row.outcome)
	}
	if row.seq.Valid {
		t.Errorf("seq 应当是 NULL，实际 %d", row.seq.Int64)
	}
	nullInts := map[string]sql.NullInt64{
		"input_tokens": row.inputTokens, "cache_create": row.cacheCreate,
		"cache_read": row.cacheRead, "output_tokens": row.outputTokens,
		"thinking_tokens": row.thinkingTokens,
		"delivered_lo":    row.deliveredLo, "delivered_hi": row.deliveredHi,
	}
	for name, col := range nullInts {
		if col.Valid {
			t.Errorf("%s 应当是 NULL，实际 %d", name, col.Int64)
		}
	}
	nullFloats := map[string]sql.NullFloat64{
		"cost_usd": row.costUSD, "cost_cumulative_usd": row.costCumulativeUSD,
	}
	for name, col := range nullFloats {
		if col.Valid {
			t.Errorf("%s 应当是 NULL，实际 %v", name, col.Float64)
		}
	}
	if row.coldStart != 0 {
		t.Errorf("cold_start = %d，期望 0", row.coldStart)
	}
	if row.wallMS != 0 {
		t.Errorf("wall_ms = %d，期望 0", row.wallMS)
	}
	if row.promptBytes != 0 {
		t.Errorf("prompt_bytes = %d，期望 0", row.promptBytes)
	}

	got, ok, err := svc.ProviderSession(ctx, conv.ID, agentID)
	if err != nil || !ok {
		t.Fatalf("ProviderSession 返回 (%+v, %v, %v)", got, ok, err)
	}
	if !got.BaselineSuspect {
		t.Error("provider_sessions.baseline_suspect 应当是 1")
	}
}

// TestSettleLostTurnBootstrapCrash_RecordsUsageWithoutSessionRow 覆盖验收
// 第 2 条：bootstrap 轮崩溃（这个 (conv, agent) 从未建立过 provider_sessions
// 行）时，session 参数传 nil，SettleLostTurn 依然要补出一条 turn_usage 行、
// 依然不报错，且不许为了标 SuspectMarked 顺手新造一行只填几个字段的
// provider_sessions（那个坑契约原话点过名）。
func TestSettleLostTurnBootstrapCrash_RecordsUsageWithoutSessionRow(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "settle：bootstrap 轮崩溃，没有 provider_sessions 行")

	const agentID = "alpha"
	const turnID = "turn-a4-bootstrap-1"

	it := tstBaseInflight(conv.ID, agentID, turnID)
	it.Phase = conversation.InflightPhase("spawned")
	it.ResumeSessionID = ""
	if err := tstBeginTurn(t, ctx, s, it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	msg := &conversation.Message{
		ConvID:   conv.ID,
		SenderID: conversation.ReservedSenderSystem,
		Kind:     conversation.KindSystem,
		Text:     "agent alpha 的 bootstrap 轮在上次退出时丢失",
	}
	u := &conversation.TurnUsage{
		TurnID: turnID, ConvID: conv.ID, AgentID: agentID, Outcome: 9, CreatedAt: it.StartedAt,
	}

	settlement, err := svc.SettleLostTurn(ctx, turnID, msg, u, nil)
	if err != nil {
		t.Fatalf("SettleLostTurn 应当成功（哪怕没有 provider_sessions 行），实际: %v", err)
	}
	if settlement.SuspectMarked {
		t.Error("session 传 nil，SuspectMarked 应当是 false（没有行可标）")
	}
	if settlement.ConvMissing {
		t.Error("会话存在，ConvMissing 应当是 false")
	}

	raw := tstRawDB(t, s.Path())
	if n := tstUsageRowCount(t, raw, turnID); n != 1 {
		t.Errorf("turn_usage 有 %d 行，期望 1 行——bootstrap 轮正是最贵的一轮，"+
			"不补的话这笔钱在库里根本不存在", n)
	}
	if n := tstMessageCount(t, ctx, s, conv.ID); n != 1 {
		t.Errorf("messages 有 %d 行，期望 1 行（面包屑仍要发）", n)
	}
	if _, ok, err := svc.ProviderSession(ctx, conv.ID, agentID); err != nil || ok {
		t.Errorf("provider_sessions 应当仍然没有这一行（session=nil 不该新造一行），实际 ok=%v err=%v", ok, err)
	}
}

// TestSettleLostTurnConversationMissing_DegradesWithoutError 覆盖验收第 5
// 条：直接裸 SQL 删掉 conversations 那一行（全仓没有任何删会话的命令），
// SettleLostTurn 必须降级——不报错、在飞记录仍被清掉、但面包屑与 turn_usage
// 都补不上（第一笔事务因 ErrConversationNotFound 整体回滚）。
//
// 故意传一个非 nil 且 BaselineSuspect=true 的 session：挡的是「SuspectMarked
// 在开事务之前就按 session != nil 算好」这种坏实现——正确实现里那一步
// 从未真正提交，SuspectMarked 必须是 false。
func TestSettleLostTurnConversationMissing_DegradesWithoutError(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	conv := tstNewConv(t, ctx, s, "settle：会话已不存在")

	const agentID = "alpha"
	const turnID = "turn-a4-convmissing-1"

	it := tstBaseInflight(conv.ID, agentID, turnID)
	it.Phase = conversation.InflightPhase("spawned")
	if err := tstBeginTurn(t, ctx, s, it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	raw := tstRawDB(t, s.Path())
	if _, err := raw.Exec(`DELETE FROM conversations WHERE id = ?`, conv.ID); err != nil {
		t.Fatalf("裸 SQL 删会话失败: %v", err)
	}

	msg := &conversation.Message{
		ConvID:   conv.ID,
		SenderID: conversation.ReservedSenderSystem,
		Kind:     conversation.KindSystem,
		Text:     "会话已不存在，这笔账补不上",
	}
	u := &conversation.TurnUsage{
		TurnID: turnID, ConvID: conv.ID, AgentID: agentID, Outcome: 9, CreatedAt: it.StartedAt,
	}
	session := &conversation.ProviderSession{
		ConvID: conv.ID, AgentID: agentID, ProviderSessionID: "sess-should-not-be-written",
		BaselineSuspect: true, LastTurnAt: it.StartedAt,
	}

	settlement, err := svc.SettleLostTurn(ctx, turnID, msg, u, session)
	if err != nil {
		t.Fatalf("会话已不存在这一支不该报错，实际: %v", err)
	}
	if !settlement.ConvMissing {
		t.Error("ConvMissing 应当是 true")
	}
	if settlement.SuspectMarked {
		t.Error("SuspectMarked 应当是 false：事务整体回滚，session 那一步从未真正提交" +
			"（挡的是 SuspectMarked 在开事务之前就按 session != nil 算好的坏实现）")
	}

	for _, row := range tstAllInflight(t, ctx, s) {
		if row.TurnID == turnID {
			t.Errorf("在飞记录还在: %+v，期望已被删掉（降级事务仍要清它）", row)
		}
	}
	if n := tstScanInt(t, raw, `SELECT COUNT(*) FROM messages WHERE conv_id = ?`, conv.ID); n != 0 {
		t.Errorf("messages 有 %d 行，期望 0 行（会话没了，面包屑也补不上）", n)
	}
	if n := tstUsageRowCount(t, raw, turnID); n != 0 {
		t.Errorf("turn_usage 有 %d 行，期望 0 行（第一笔事务整体回滚）", n)
	}
	if n := tstScanInt(t, raw, `SELECT COUNT(*) FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`, conv.ID, agentID); n != 0 {
		t.Errorf("provider_sessions 有 %d 行，期望 0 行（同一笔事务，同样回滚）", n)
	}
}
