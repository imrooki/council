package turn

// step10a4_settle_lost_turns_test.go 覆盖 Step 10 A 片之四冻结契约第 6 节
// 验收清单第 1（面包屑测辞不同那半）、2、3、4、6、7 条，测的是
// Runner.AuditInflight + Runner.SettleLostTurns 这条完整链路——构造
// msg/u/session 的业务逻辑（哪个 phase 传不传 nil、整行拷贝只改
// BaselineSuspect、marked 只数真的标上的）全在 recover.go 里，
// Service.SettleLostTurn 自己的持久化行为已经在
// internal/store/step10a4_settle_lost_turn_test.go 测过。
//
// 用真实 *store.Store（newRealTurnStore，复用 step10a3_fixtures_test.go
// 已有的工装）：turn_usage 的列内容与 provider_sessions.cursor_seq 有没有
// 被动过，只有真库才能忠实回答。
//
// 落盘时 Runner.AuditInflight / Runner.SettleLostTurns 都还不存在，
// 编译不过是预期。

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// turnUsageA4Row 是从 turn_usage 直接 Scan 出来、本文件专用的一行。
// 不复用 internal/store 的 tstUsageRow：那是另一个包的未导出类型，
// 跨包访问不到，只能在这里另起一份同类型的扫描器（与
// realTurnStore.turnUsageRowCount 同一个理由，见 step10a3_fixtures_test.go
// 文件头注释第 2 点）。
type turnUsageA4Row struct {
	outcome           int64
	seq               sql.NullInt64
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
}

func (r realTurnStore) scanTurnUsageA4(t *testing.T, turnID string) turnUsageA4Row {
	t.Helper()
	db := r.rawDB(t)
	var row turnUsageA4Row
	err := db.QueryRow(`
SELECT outcome, seq, input_tokens, cache_create, cache_read, output_tokens, thinking_tokens,
       cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes, delivered_lo, delivered_hi
FROM turn_usage WHERE turn_id = ?`, turnID).Scan(
		&row.outcome, &row.seq, &row.inputTokens, &row.cacheCreate, &row.cacheRead,
		&row.outputTokens, &row.thinkingTokens, &row.costUSD, &row.costCumulativeUSD,
		&row.coldStart, &row.wallMS, &row.promptBytes, &row.deliveredLo, &row.deliveredHi,
	)
	if err != nil {
		t.Fatalf("读 turn_usage(turn_id=%q) 失败: %v", turnID, err)
	}
	return row
}

// TestSettleLostTurnsMarkedCountExcludesBootstrapAndBegun 覆盖验收第 3 条，
// 本片最容易写成空断言的一条：造三条在飞记录——begun（不该标）、bootstrap
// 崩溃（phase != begun，但没有 provider_sessions 行可标）、正常 spawned
// （phase != begun 且有行可标）。phase != begun 的一共有两条，但 marked
// 必须是 1：若坏实现按「phase != begun 的条数」反推 marked，这里会拿到 2。
func TestSettleLostTurnsMarkedCountExcludesBootstrapAndBegun(t *testing.T) {
	rts := newRealTurnStore(t)
	cfg := newTestConfig()
	convID := newConv(t, rts.svc)
	r := newTestRunner(t, cfg, rts.svc)

	base := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)

	beginIT := conversation.InflightTurn{
		TurnID: "turn-marked-begun", ConvID: convID, AgentID: "alpha", Provider: "claude",
		StartedAt: base, Phase: conversation.PhaseBegun, Tier: "read",
	}
	bootstrapIT := conversation.InflightTurn{
		TurnID: "turn-marked-bootstrap", ConvID: convID, AgentID: "bravo", Provider: "grok",
		PlannedSessionID: "planned-bootstrap-1",
		StartedAt:        base.Add(time.Minute), Phase: conversation.PhaseSpawned, Tier: "read",
	}

	existingSession := conversation.ProviderSession{
		ConvID: convID, AgentID: "charlie", ProviderSessionID: "sess-charlie-1",
		CursorSeq: 5, HaveCostTotal: true, CostTotalUSD: 0.5,
		LastTurnAt: base, IdentityFP: "fp-charlie",
	}
	if err := rts.st.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, existingSession)
	}); err != nil {
		t.Fatalf("预置 provider 会话失败: %v", err)
	}
	normalIT := conversation.InflightTurn{
		TurnID: "turn-marked-normal", ConvID: convID, AgentID: "charlie", Provider: "claude",
		ResumeSessionID: existingSession.ProviderSessionID,
		StartedAt:       base.Add(2 * time.Minute), Phase: conversation.PhaseSpawned, Tier: "read",
	}

	for _, it := range []conversation.InflightTurn{beginIT, bootstrapIT, normalIT} {
		if err := rts.svc.BeginTurn(context.Background(), it); err != nil {
			t.Fatalf("BeginTurn(%s) 失败: %v", it.TurnID, err)
		}
	}

	lost, err := r.AuditInflight(context.Background())
	if err != nil {
		t.Fatalf("AuditInflight 失败: %v", err)
	}
	if len(lost) != 3 {
		t.Fatalf("AuditInflight 返回 %d 条，期望 3（前置条件不成立）", len(lost))
	}

	marked, errs := r.SettleLostTurns(context.Background(), lost)
	if len(errs) != 0 {
		t.Fatalf("SettleLostTurns 报错: %v", errs)
	}
	if marked != 1 {
		t.Errorf("marked = %d，期望 1（只有 charlie 那条真的标上了；bootstrap 那条没有行可标，"+
			"若实现按 phase != begun 反推会错误地数成 2）", marked)
	}

	if n := rts.turnUsageRowCount(t, beginIT.TurnID); n != 0 {
		t.Errorf("begin 那条 turn_usage 有 %d 行，期望 0", n)
	}
	if n := rts.turnUsageRowCount(t, bootstrapIT.TurnID); n != 1 {
		t.Errorf("bootstrap 那条 turn_usage 有 %d 行，期望 1", n)
	}
	if _, ok, err := rts.svc.ProviderSession(context.Background(), convID, "bravo"); err != nil || ok {
		t.Errorf("bravo 不该有 provider_sessions 行（不许新造一个只填几个字段的），实际 ok=%v err=%v", ok, err)
	}
	if n := rts.turnUsageRowCount(t, normalIT.TurnID); n != 1 {
		t.Errorf("normal 那条 turn_usage 有 %d 行，期望 1", n)
	}
	got, ok, err := rts.svc.ProviderSession(context.Background(), convID, "charlie")
	if err != nil || !ok {
		t.Fatalf("ProviderSession(charlie) 返回 (%+v, %v, %v)", got, ok, err)
	}
	if !got.BaselineSuspect {
		t.Error("charlie 的 baseline_suspect 应当是 true")
	}

	all, err := rts.svc.AllInflight(context.Background())
	if err != nil {
		t.Fatalf("AllInflight 失败: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("settle 之后 AllInflight 还有 %d 条，期望 0: %+v", len(all), all)
	}
}

// TestSettleLostTurnsLostRowColumnsAreNullOrZeroNotGuessed 覆盖验收第 6 条：
// Lost 行的三个 NOT NULL 列必须是 0（读作「不知道」不是「零」），七个可空
// 列（seq、五个用量列、delivered_lo/hi）与两个 cost 列必须是 NULL。
func TestSettleLostTurnsLostRowColumnsAreNullOrZeroNotGuessed(t *testing.T) {
	rts := newRealTurnStore(t)
	cfg := newTestConfig()
	convID := newConv(t, rts.svc)
	r := newTestRunner(t, cfg, rts.svc)

	base := time.Date(2026, 9, 22, 9, 30, 0, 0, time.UTC)
	session := conversation.ProviderSession{
		ConvID: convID, AgentID: "alpha", ProviderSessionID: "sess-columns-1",
		CursorSeq: 3, HaveCostTotal: true, CostTotalUSD: 1.23,
		LastTurnAt: base, IdentityFP: "fp-columns",
	}
	if err := rts.st.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, session)
	}); err != nil {
		t.Fatalf("预置 provider 会话失败: %v", err)
	}
	it := conversation.InflightTurn{
		TurnID: "turn-columns-1", ConvID: convID, AgentID: "alpha", Provider: "claude",
		ResumeSessionID: session.ProviderSessionID,
		StartedAt:       base.Add(time.Minute), Phase: conversation.PhaseSpawned, Tier: "read",
	}
	if err := rts.svc.BeginTurn(context.Background(), it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	lost, err := r.AuditInflight(context.Background())
	if err != nil || len(lost) != 1 {
		t.Fatalf("AuditInflight 返回 (%v, %v)，期望 1 条无错误", lost, err)
	}
	marked, errs := r.SettleLostTurns(context.Background(), lost)
	if len(errs) != 0 || marked != 1 {
		t.Fatalf("SettleLostTurns 返回 (marked=%d, errs=%v)，期望 (1, [])", marked, errs)
	}

	row := rts.scanTurnUsageA4(t, it.TurnID)
	if row.outcome != 9 {
		t.Errorf("outcome = %d，期望 9（OutcomeLost）", row.outcome)
	}
	nullChecks := map[string]sql.NullInt64{
		"seq": row.seq, "input_tokens": row.inputTokens, "cache_create": row.cacheCreate,
		"cache_read": row.cacheRead, "output_tokens": row.outputTokens,
		"thinking_tokens": row.thinkingTokens, "delivered_lo": row.deliveredLo, "delivered_hi": row.deliveredHi,
	}
	for name, col := range nullChecks {
		if col.Valid {
			t.Errorf("%s 应当是 NULL（Lost 行「不知道」不是「零」），实际 %d", name, col.Int64)
		}
	}
	floatChecks := map[string]sql.NullFloat64{"cost_usd": row.costUSD, "cost_cumulative_usd": row.costCumulativeUSD}
	for name, col := range floatChecks {
		if col.Valid {
			t.Errorf("%s 应当是 NULL，实际 %v", name, col.Float64)
		}
	}
	if row.coldStart != 0 {
		t.Errorf("cold_start = %d，期望 0（三个 0 是硬要求，不可空列只能读作「不知道」）", row.coldStart)
	}
	if row.wallMS != 0 {
		t.Errorf("wall_ms = %d，期望 0", row.wallMS)
	}
	if row.promptBytes != 0 {
		t.Errorf("prompt_bytes = %d，期望 0", row.promptBytes)
	}
}

// TestSettleLostTurnsPreservesCursorAndCopiesFullRow 覆盖验收第 7 条：
// 游标没被推，且整行拷贝、只改 BaselineSuspect。三个数字（会话本来的
// cursor_seq、这条在飞记录的 CursorSeq、ScannedTo）两两不同，挡的是
// 「把 ProviderSession.CursorSeq 写成了 Turn.CursorSeq 或 Turn.ScannedTo」
// 这种张冠李戴；随后逐字段核对其余列没有被动过，挡的是「只填几个字段」
// 那种坏实现。
func TestSettleLostTurnsPreservesCursorAndCopiesFullRow(t *testing.T) {
	rts := newRealTurnStore(t)
	cfg := newTestConfig()
	convID := newConv(t, rts.svc)
	r := newTestRunner(t, cfg, rts.svc)

	base := time.Date(2026, 9, 22, 9, 45, 0, 0, time.UTC)
	const (
		sessionCursorSeq = int64(77)
		turnCursorSeq    = int64(5)
		turnScannedTo    = int64(12)
	)
	session := conversation.ProviderSession{
		ConvID: convID, AgentID: "alpha", ProviderSessionID: "sess-cursor-1",
		CursorSeq: sessionCursorSeq, HaveCostTotal: true, CostTotalUSD: 9.87,
		LastTurnAt: base, IdentityFP: "fp-cursor-untouched", Spoken: true,
	}
	if err := rts.st.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, session)
	}); err != nil {
		t.Fatalf("预置 provider 会话失败: %v", err)
	}
	it := conversation.InflightTurn{
		TurnID: "turn-cursor-1", ConvID: convID, AgentID: "alpha", Provider: "claude",
		ResumeSessionID: session.ProviderSessionID,
		CursorSeq:       turnCursorSeq,
		ScannedTo:       turnScannedTo,
		StartedAt:       base.Add(time.Minute), Phase: conversation.PhaseSpawned, Tier: "read",
	}
	if err := rts.svc.BeginTurn(context.Background(), it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	lost, err := r.AuditInflight(context.Background())
	if err != nil || len(lost) != 1 {
		t.Fatalf("AuditInflight 返回 (%v, %v)", lost, err)
	}
	marked, errs := r.SettleLostTurns(context.Background(), lost)
	if len(errs) != 0 || marked != 1 {
		t.Fatalf("SettleLostTurns 返回 (marked=%d, errs=%v)", marked, errs)
	}

	got, ok, err := rts.svc.ProviderSession(context.Background(), convID, "alpha")
	if err != nil || !ok {
		t.Fatalf("ProviderSession 返回 (%+v, %v, %v)", got, ok, err)
	}
	if got.CursorSeq != sessionCursorSeq {
		t.Errorf("CursorSeq = %d，期望 %d（对账不写 cursor_seq，游标不许被推）", got.CursorSeq, sessionCursorSeq)
	}
	if got.ProviderSessionID != session.ProviderSessionID {
		t.Errorf("ProviderSessionID = %q，期望 %q（应当整行拷贝）", got.ProviderSessionID, session.ProviderSessionID)
	}
	if got.CostTotalUSD != session.CostTotalUSD || got.HaveCostTotal != session.HaveCostTotal {
		t.Errorf("CostTotalUSD/HaveCostTotal = (%v,%v)，期望 (%v,%v)（应当整行拷贝，不是只填几个字段）",
			got.CostTotalUSD, got.HaveCostTotal, session.CostTotalUSD, session.HaveCostTotal)
	}
	if !got.LastTurnAt.Equal(session.LastTurnAt) {
		t.Errorf("LastTurnAt = %v，期望 %v", got.LastTurnAt, session.LastTurnAt)
	}
	if got.IdentityFP != session.IdentityFP {
		t.Errorf("IdentityFP = %q，期望 %q（不该被 settle 动到）", got.IdentityFP, session.IdentityFP)
	}
	if got.Spoken != session.Spoken {
		t.Errorf("Spoken = %v，期望 %v", got.Spoken, session.Spoken)
	}
	if !got.BaselineSuspect {
		t.Error("BaselineSuspect 应当是 true：这是本方法唯一该改的那一列")
	}
}

// TestSettleLostTurnsOrphanMessageNamesNothingWhenBothIDsEmpty 覆盖验收
// 第 4 条落到持久化里的样子：两个 id 都空时（claude 与 agy 的 bootstrap
// 崩溃两个 id 都是空，契约 4.3 原话），面包屑正文要老实说「点不出名字」，
// 且 Meta 里不许出现值为空字符串的键（不许印一个空的 id 占位糊过去）。
func TestSettleLostTurnsOrphanMessageNamesNothingWhenBothIDsEmpty(t *testing.T) {
	rts := newRealTurnStore(t)
	cfg := newTestConfig()
	convID := newConv(t, rts.svc)
	r := newTestRunner(t, cfg, rts.svc)

	it := conversation.InflightTurn{
		TurnID: "turn-orphan-nameless-1", ConvID: convID, AgentID: "alpha", Provider: "agy",
		StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, Tier: "read",
	}
	if err := rts.svc.BeginTurn(context.Background(), it); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	lost, err := r.AuditInflight(context.Background())
	if err != nil || len(lost) != 1 {
		t.Fatalf("AuditInflight 返回 (%v, %v)", lost, err)
	}
	if lost[0].OrphanID != "" {
		t.Fatalf("前置条件不成立：OrphanID = %q，期望空串", lost[0].OrphanID)
	}

	marked, errs := r.SettleLostTurns(context.Background(), lost)
	if len(errs) != 0 {
		t.Fatalf("SettleLostTurns 报错: %v", errs)
	}
	if marked != 0 {
		t.Errorf("marked = %d，期望 0（bootstrap 崩溃没有 provider_sessions 行可标）", marked)
	}

	msgs, err := rts.svc.History(context.Background(), convID, 0, 10)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("消息条数 = %d，期望 1", len(msgs))
	}
	m := msgs[0]
	if !strings.Contains(m.Text, "点不出名字") {
		t.Errorf("消息正文 = %q，应当含「点不出名字」", m.Text)
	}
	if m.SenderID != conversation.ReservedSenderSystem {
		t.Errorf("SenderID = %q，期望 %q", m.SenderID, conversation.ReservedSenderSystem)
	}
	if !m.SenderBrainEmpty() {
		t.Error("system 面包屑的三个脑子列必须留空，保留 sender 带脑子会被 ErrInvalidSender 拒掉")
	}
	// 用字面量 "turn_lost" 而不是包里的 eventTurnLost 常量比对：这条断言要
	// 挡的正是「那个常量的字面值被打错」这种坏实现，拿它自己的符号去比对
	// 会让这类错误自己跟自己一致、测不出来（与 record.go 里 Outcome: 9 用
	// 字面量而不是 turn.OutcomeLost 是同一条理由）。
	if got := m.Meta[MetaEvent]; got != "turn_lost" {
		t.Errorf("Meta[%q] = %v，期望 %q", MetaEvent, got, "turn_lost")
	}
	if got := m.Meta[MetaAboutAgent]; got != "alpha" {
		t.Errorf("Meta[%q] = %v，期望 %q", MetaAboutAgent, got, "alpha")
	}
	for k, v := range m.Meta {
		if v == nil {
			t.Errorf("Meta[%q] 是 nil——孤儿点不出名字时不许印一个空的 id 占位，"+
				"该键要么不写、要么有值，不能写一个 null 充数", k)
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			t.Errorf("Meta[%q] 是空字符串——孤儿点不出名字时不许印一个空的 id 占位，"+
				"该键要么不写、要么有值，不能写一个空串充数", k)
		}
	}
}

// TestSettleLostTurnsBreadcrumbWordingDiffersBetweenSpawnedAndReturned 覆盖
// 验收第 1 条明写的「returned -> 同上但面包屑措辞不同」：同一条 provider
// 会话上先后造一条 spawned 与一条 returned 的在飞记录，settle 之后两条
// system 面包屑正文必须不同，且各自带着 4.3 表里对应 phase 的冻结措辞。
func TestSettleLostTurnsBreadcrumbWordingDiffersBetweenSpawnedAndReturned(t *testing.T) {
	rts := newRealTurnStore(t)
	cfg := newTestConfig()
	convID := newConv(t, rts.svc)
	r := newTestRunner(t, cfg, rts.svc)

	session := conversation.ProviderSession{
		ConvID: convID, AgentID: "alpha", ProviderSessionID: "sess-wording-1",
		CursorSeq: 1, LastTurnAt: time.Now().UTC(), IdentityFP: "fp-wording",
	}
	if err := rts.st.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, session)
	}); err != nil {
		t.Fatalf("预置 provider 会话失败: %v", err)
	}

	spawnedIT := conversation.InflightTurn{
		TurnID: "turn-wording-spawned", ConvID: convID, AgentID: "alpha", Provider: "claude",
		ResumeSessionID: session.ProviderSessionID, StartedAt: time.Now().UTC(),
		Phase: conversation.PhaseSpawned, Tier: "read",
	}
	if err := rts.svc.BeginTurn(context.Background(), spawnedIT); err != nil {
		t.Fatalf("BeginTurn(spawned) 失败: %v", err)
	}
	lostSpawned, err := r.AuditInflight(context.Background())
	if err != nil || len(lostSpawned) != 1 {
		t.Fatalf("AuditInflight(spawned) 返回 (%v, %v)", lostSpawned, err)
	}
	if _, errs := r.SettleLostTurns(context.Background(), lostSpawned); len(errs) != 0 {
		t.Fatalf("SettleLostTurns(spawned) 报错: %v", errs)
	}

	returnedIT := conversation.InflightTurn{
		TurnID: "turn-wording-returned", ConvID: convID, AgentID: "alpha", Provider: "claude",
		ResumeSessionID: session.ProviderSessionID, StartedAt: time.Now().UTC(),
		Phase: conversation.PhaseReturned, Tier: "read",
	}
	if err := rts.svc.BeginTurn(context.Background(), returnedIT); err != nil {
		t.Fatalf("BeginTurn(returned) 失败: %v", err)
	}
	lostReturned, err := r.AuditInflight(context.Background())
	if err != nil || len(lostReturned) != 1 {
		t.Fatalf("AuditInflight(returned) 返回 (%v, %v)", lostReturned, err)
	}
	if _, errs := r.SettleLostTurns(context.Background(), lostReturned); len(errs) != 0 {
		t.Fatalf("SettleLostTurns(returned) 报错: %v", errs)
	}

	msgs, err := rts.svc.History(context.Background(), convID, 0, 10)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("消息条数 = %d，期望 2", len(msgs))
	}
	if msgs[0].Text == msgs[1].Text {
		t.Fatalf("spawned 与 returned 两条面包屑正文相同: %q，期望不同（验收第 1 条明写「面包屑测辞不同」）", msgs[0].Text)
	}
	if !strings.Contains(msgs[1].Text, "没能落库") {
		t.Errorf("returned 那条面包屑正文 = %q，应当含冻结措辞「没能落库」", msgs[1].Text)
	}
}

// TestSettleLostTurnsOneFailureDoesNotBlockOthers 覆盖契约 4.3 节「逐条调
// Service.SettleLostTurn，每条一笔事务，一条失败不拖累其余」。手工构造
// []LostTurn（不经 AuditInflight——AuditInflight 只会原样回显库里已有的
// turn_id，不可能产出空串，要造出这种坏输入只能直接构造切片），坏的排在
// 好的前面：挡的是「遇到第一个错误就提前 return」这种坏实现，那样排在
// 后面的好记录会漏处理。
func TestSettleLostTurnsOneFailureDoesNotBlockOthers(t *testing.T) {
	rts := newRealTurnStore(t)
	cfg := newTestConfig()
	convID := newConv(t, rts.svc)
	r := newTestRunner(t, cfg, rts.svc)

	goodIT := conversation.InflightTurn{
		TurnID: "turn-good-after-bad", ConvID: convID, AgentID: "alpha", Provider: "claude",
		StartedAt: time.Now().UTC(), Phase: conversation.PhaseBegun, Tier: "read",
	}
	if err := rts.svc.BeginTurn(context.Background(), goodIT); err != nil {
		t.Fatalf("BeginTurn 失败: %v", err)
	}

	bad := LostTurn{
		Turn:   conversation.InflightTurn{TurnID: "", ConvID: convID, AgentID: "ghost", Phase: conversation.PhaseBegun},
		Reason: "构造出来的坏输入",
	}
	good := LostTurn{Turn: goodIT, Reason: "进程没起，只是记录残留"}

	marked, errs := r.SettleLostTurns(context.Background(), []LostTurn{bad, good})
	if len(errs) != 1 {
		t.Fatalf("errs 长度 = %d，期望 1（只有 bad 那条失败）: %v", len(errs), errs)
	}
	if !errors.Is(errs[0], conversation.ErrMissingTurnID) {
		t.Errorf("errs[0] 应当包住 ErrMissingTurnID，实际: %v", errs[0])
	}
	if marked != 0 {
		t.Errorf("marked = %d，期望 0（两条都不该被标：bad 从未成功，good 是 begun 档）", marked)
	}

	all, err := rts.svc.AllInflight(context.Background())
	if err != nil {
		t.Fatalf("AllInflight 失败: %v", err)
	}
	for _, row := range all {
		if row.TurnID == goodIT.TurnID {
			t.Errorf("good 那条在飞记录还在: %+v，期望已被处理（一条失败不该拖累其余的处理）", row)
		}
	}
}
