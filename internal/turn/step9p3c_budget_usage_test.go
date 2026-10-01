package turn

// 本文件是 Step 9 片 3c（docs/12_STEP9_CONTRACT.md 4.5/4.9 节）的测试，只测
// UsageSamples（Store 与 Service 各一份，签名相同）与 Runner.Budget 这两个
// 冻结符号。背对背写：实现方（grok-4.7 xhigh）与本文件在同一份冻结契约下
// 独立开工，互不读对方产出。落笔时 UsageSamples / BudgetRow / Budget 一个
// 符号都不存在，本文件编译不过是预期状态，不是错误。
//
// 冻结的形状（任务书原文 + 2026-09-22 主代理在 docs/12_STEP9_CONTRACT.md
// 4.5a 第二条追加的 NoDelta 字段，见下方与用例 8 的说明）：
//
//	UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]TurnUsage, error)
//
//	type BudgetRow struct {
//		AgentID, Provider string
//		Cold       bool
//		InFlight   bool
//		Calibrated bool
//		InputTokens *int
//		CostUSD     *float64
//		ArgvHeadroom *int
//		Basis        string
//		NoDelta      bool   // 4.5a 第二条新增：本轮 Select 之后没有可投递的新内容
//	}
//
//	func (r *Runner) Budget(ctx context.Context, convID string, plan Plan) ([]BudgetRow, error)
//
// 一处契约仍把裁决权交给了实现方，本文件的断言不钉死它：SupportUnknown
// 的取样规则——本文件只用 CacheAcrossTurns 已经实测钉死的两家：agy
// （SupportNo）与 claude（SupportYes），不用任何 SupportUnknown 的 provider。
// （原来另一条"Budget 与 RunRound 共用快照的落法"同样不钉死——本文件从不
// 在同一轮里既调 Budget 又调 RunRound，不触碰那条裁决，不需要专门写出来。）
//
// 造合成样本走 svc.RecordTurn(ctx, nil, u, nil)：msg 为 nil 时 RecordTurn
// 只要求 u.Seq 也是 nil（record_turn 的 (msg==nil)!=(u.Seq==nil) 校验），
// 不做会话存在性之外的任何校验，可以自由摆 InputTokens / PromptBytes /
// IdentityFP / Outcome / CreatedAt——这正是本文件需要的：用真实的
// conversation.Service.RecordTurn 入口造历史行，而不是绕开它直接拼 SQL
// （raw SQL 只在"查/改一列去核验"时用，见 p3cTurnUsageExists /
// p3cSetIdentityFP，仿 turn_usage_v4_columns_test.go 的体例）。

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"council/internal/conversation"
)

// ---------------------------------------------------------------------------
// 本文件专用的小建造函数与断言辅助，p3c 前缀（仿 planround_test.go 的 p3a
// 前缀），与别的片的测试专用 helper 分开。
// ---------------------------------------------------------------------------

func p3cIntPtr(v int) *int           { return &v }
func p3cFloatPtr(v float64) *float64 { return &v }

// p3cNewTurnID 铸一个新 v4 UUID 当合成样本的 turn_id。newV4UUID 是 record.go
// 里的包内私有函数，Ask/AskUpTo 铸 TurnID/RoundID 用的同一份算法，这里复用
// 而不是自己拼字符串，保持与生产路径行为一致。
func p3cNewTurnID(t *testing.T) string {
	t.Helper()
	id, err := newV4UUID()
	if err != nil {
		t.Fatalf("newV4UUID 失败: %v", err)
	}
	return id
}

// p3cInsertSample 用 Service.RecordTurn(msg=nil, session=nil) 造一行合成的
// turn_usage 历史。TurnID 为空时自动铸一个。u.Seq 强制清 nil：msg 为 nil 时
// RecordTurn 要求两者同步，这是本函数存在的意义之一——调用方不必每次都记得
// 清这一个字段。
func p3cInsertSample(t *testing.T, svc *conversation.Service, u conversation.TurnUsage) conversation.TurnUsage {
	t.Helper()
	if u.TurnID == "" {
		u.TurnID = p3cNewTurnID(t)
	}
	u.Seq = nil
	if _, err := svc.RecordTurn(context.Background(), nil, u, nil, false); err != nil {
		t.Fatalf("造合成样本失败 turn_id=%s: %v", u.TurnID, err)
	}
	return u
}

// p3cPlan 造一个只有 Speakers 的 Plan，Judges/Skipped 是空切片而不是 nil
// （Plan 自己的文档注释要求三个切片都不为 nil，见 plan.go）。
func p3cPlan(agentIDs ...string) Plan {
	speakers := append([]string{}, agentIDs...)
	return Plan{Speakers: speakers, Judges: []string{}, Skipped: []SkipReason{}}
}

// p3cBudgetSafe 调用 Runner.Budget，用 recover 把可能的 panic 转成一条清楚
// 的测试失败信息——这正是用例 7 第一个方向（"不崩"）要断言的东西：不能让
// 一次除零 panic 冒出一条裸的 runtime error 堆栈，测试要能明确指出"这就是
// 那条除零守卫失守了"。
func p3cBudgetSafe(t *testing.T, r *Runner, convID string, plan Plan) []BudgetRow {
	t.Helper()
	var rows []BudgetRow
	var err error
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("Runner.Budget 发生 panic——这正是用例 7 要挡住的那类崩溃"+
					"（典型根因：样本.input_tokens 与样本.prompt_bytes 做整数除法，"+
					"prompt_bytes==0 时 Go 的整数除法直接 panic，不像浮点除法那样退化成 Inf）: %v", p)
			}
		}()
		rows, err = r.Budget(context.Background(), convID, plan)
	}()
	if err != nil {
		t.Fatalf("Runner.Budget 返回错误: %v", err)
	}
	return rows
}

// p3cFindBudgetRow 从 Budget 的返回里按 AgentID 找一行，找不到直接 Fatal——
// 找不到本身就是前置条件不成立，不该让后面的断言在一个零值 BudgetRow 上
// 继续跑、把"没找到"误判成"找到了但字段不对"。
func p3cFindBudgetRow(t *testing.T, rows []BudgetRow, agentID string) BudgetRow {
	t.Helper()
	for _, row := range rows {
		if row.AgentID == agentID {
			return row
		}
	}
	t.Fatalf("Budget 返回的 %d 行里没有 agent %q 的记录，plan 与实现的行为对不上", len(rows), agentID)
	return BudgetRow{}
}

// p3cRawDB 开一条独立于 Service 的底层连接，仿 turn_usage_v4_columns_test.go
// 的 tstTurnUsageRoundAndIdentity：conversation.Store 接口没有给 turn_usage
// 配读方法，核验"turn_id 真的在库里"或者改一列，只能绕开 Service 走原始连接。
func p3cRawDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite",
		"file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
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

// p3cTurnUsageExists 去库里查一个 turn_id 是不是真的存在——用例 6(a) 与
// 任务书原话「Basis 要去库里查那个 turn_id 真存在，不是断言它非空」对应的
// 就是这个函数，不是子串或非空判断。
func p3cTurnUsageExists(t *testing.T, dbPath, turnID string) bool {
	t.Helper()
	db := p3cRawDB(t, dbPath)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM turn_usage WHERE turn_id = ?`, turnID).Scan(&count); err != nil {
		t.Fatalf("查 turn_usage(turn_id=%q) 是否存在失败: %v", turnID, err)
	}
	return count == 1
}

// p3cSetIdentityFP 直接改一行 turn_usage 的 identity_fp——用例 5 要求先证明
// "两条都取得到"，再"把其中一条的 identity_fp 改掉"，这是"改掉"那一步，
// RecordTurn 只会插入新行，没有更新入口，只能走原始连接。
func p3cSetIdentityFP(t *testing.T, dbPath, turnID, newFP string) {
	t.Helper()
	db := p3cRawDB(t, dbPath)
	res, err := db.Exec(`UPDATE turn_usage SET identity_fp = ? WHERE turn_id = ?`, newFP, turnID)
	if err != nil {
		t.Fatalf("改 turn_usage.identity_fp 失败 turn_id=%s: %v", turnID, err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		t.Fatalf("改 turn_usage.identity_fp 影响行数=%d，期望恰好 1（turn_id=%s）", n, turnID)
	}
}

// p3cMetaInt 从消息 Meta 里取一个数字字段。Meta 存的是 JSON，经
// encoding/json 解码进 map[string]any 之后数字一律是 float64，不是 int
// （store/read.go 的 scanMessage 走的是标准库的通用 json.Unmarshal）——
// 这里做一次归一，调用方不必关心这个细节。
func p3cMetaInt(m map[string]any, key string) (int, bool) {
	switch v := m[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------------
// 用例 5：UsageSamples 按 (agent_id, identity_fp) 取
// ---------------------------------------------------------------------------

// TestUsageSamplesTwoConversationsSameFingerprintThenFingerprintChanged 覆盖
// 契约 4.9 第 5 条的主干：造两条会话、同一个 agent、同一份指纹 -> 两条都
// 取得到；把其中一条的 identity_fp 改掉 -> 只剩一条。
//
// 为什么这条重要（任务书原话）：指纹变了意味着前缀作废、规模不可比。拿旧
// 指纹的样本预报新指纹的一轮，就是拿另一份前缀的数字冒充。
func TestUsageSamplesTwoConversationsSameFingerprintThenFingerprintChanged(t *testing.T) {
	svc, dbPath := newTestServiceWithPath(t)
	convA := newConv(t, svc)
	convB := newConv(t, svc)

	const agentID = "p3c-usage-agent"
	const fp = "p3c-fp-shared-v1"
	base := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

	rowA := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convA, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(500), PromptBytes: 1000,
		CreatedAt: base,
	})
	rowB := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convB, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(600), PromptBytes: 1200,
		CreatedAt: base.Add(time.Minute),
	})

	ctx := context.Background()

	// 第一段：两条会话、同一份指纹，都取得到。
	got, err := svc.UsageSamples(ctx, agentID, fp, 10)
	if err != nil {
		t.Fatalf("UsageSamples 失败: %v", err)
	}
	gotIDs := map[string]bool{}
	for _, u := range got {
		gotIDs[u.TurnID] = true
	}
	if len(got) != 2 || !gotIDs[rowA.TurnID] || !gotIDs[rowB.TurnID] {
		t.Fatalf("前置条件不成立：两条会话、同一个 agent、同一份指纹应当两条都取得到，"+
			"实际取到 %d 条：%v（want 含 %q 与 %q）", len(got), gotIDs, rowA.TurnID, rowB.TurnID)
	}

	// 第二段：把 rowA 的 identity_fp 改掉，只剩 rowB。
	p3cSetIdentityFP(t, dbPath, rowA.TurnID, "p3c-fp-changed-v2")

	got2, err := svc.UsageSamples(ctx, agentID, fp, 10)
	if err != nil {
		t.Fatalf("UsageSamples（改指纹之后）失败: %v", err)
	}
	if len(got2) != 1 {
		t.Fatalf("改掉 rowA 的 identity_fp 之后，按旧指纹查应当只剩 1 条，实际 %d 条：%+v", len(got2), got2)
	}
	if got2[0].TurnID != rowB.TurnID {
		t.Fatalf("改指纹之后剩下的应当是 rowB（%q），实际是 %q", rowB.TurnID, got2[0].TurnID)
	}
}

// TestUsageSamplesOutcomePostRejectedCountsNullInputTokensExcluded 钉住样本
// 口径（契约 4.5 / 1.4 第 4 条）："OutcomePostRejected 的行算样本（钱花了、
// 用量也记了，排除它就是往低估方向偏）；input_tokens 为 NULL 的不算。"
func TestUsageSamplesOutcomePostRejectedCountsNullInputTokensExcluded(t *testing.T) {
	svc, _ := newTestServiceWithPath(t)
	conv := newConv(t, svc)

	const agentID = "p3c-outcome-agent"
	const fp = "p3c-fp-outcome-v1"
	base := time.Date(2026, 9, 20, 11, 0, 0, 0, time.UTC)
	ctx := context.Background()

	// OutcomePostRejected：钱花了、用量记了（InputTokens 非 nil），必须算样本。
	rejected := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomePostRejected), InputTokens: p3cIntPtr(700), PromptBytes: 1400,
		CreatedAt: base,
	})
	// input_tokens 为 NULL：没解析到用量，不算样本，即使 Outcome 是 OutcomeSpoke。
	_ = p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: nil, PromptBytes: 1500,
		CreatedAt: base.Add(time.Minute),
	})

	got, err := svc.UsageSamples(ctx, agentID, fp, 10)
	if err != nil {
		t.Fatalf("UsageSamples 失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("应当恰好 1 条样本（OutcomePostRejected 且 input_tokens 非 NULL 的那条），"+
			"实际 %d 条：%+v", len(got), got)
	}
	if got[0].TurnID != rejected.TurnID {
		t.Fatalf("唯一的样本应当是 OutcomePostRejected 那条（%q），实际是 %q", rejected.TurnID, got[0].TurnID)
	}
	if got[0].InputTokens == nil || *got[0].InputTokens != 700 {
		t.Errorf("样本的 InputTokens 应当是 700，实际 %v", got[0].InputTokens)
	}
}

// ---------------------------------------------------------------------------
// 用例 6：Budget 分两段——只做 (b) 等于没验
// ---------------------------------------------------------------------------

// TestBudgetCalibratedRoundThreeUsesRealHistoricalTurnID 是用例 6 的重点
// (a)：对一条已经跑过至少两轮的会话预报第三轮。
//
// 用 agy（CacheAcrossTurns=SupportNo）而不是 claude/grok：SupportNo 的取样
// 规则最简单——"取最近一条成功轮的 input_tokens，不分冷暖"（契约 4.5），
// 不需要另外控制冷暖预报（ColdExpected 依赖 r.now()/lastTurnAt/CacheTTL），
// 减少一个与"除零守卫"无关的变量。
//
// 前两轮用 fixtures_test.go 现成的 agyT1JSON/agyT2JSON 真实探针原件（同一
// 条 conversation_id，第二轮 --resume 接上第一轮），走真实的 r.Ask 路径，
// 不是自己拼字段——turn_usage 里的 prompt_bytes 由 Runner 真实渲染算出，
// identity_fp 由 Ask 真实计算，不需要本文件去猜。
func TestBudgetCalibratedRoundThreeUsesRealHistoricalTurnID(t *testing.T) {
	cfg := newTestConfig()
	svc, dbPath := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	ctx := context.Background()

	postUser(t, svc, convID, "第一轮，正文不作数")
	fs.enqueueSuccess(agyT1JSON)
	rec1, err := r.Ask(ctx, convID, testAgentAgy)
	if err != nil || rec1.Outcome != OutcomeSpoke {
		t.Fatalf("第一轮 Ask 前置条件不成立: err=%v outcome=%v", err, rec1.Outcome)
	}

	postUser(t, svc, convID, "第二轮，正文不作数")
	fs.enqueueSuccess(agyT2JSON)
	rec2, err := r.Ask(ctx, convID, testAgentAgy)
	if err != nil || rec2.Outcome != OutcomeSpoke {
		t.Fatalf("第二轮 Ask 前置条件不成立: err=%v outcome=%v", err, rec2.Outcome)
	}

	// 第三轮预报之前先给点新内容，让本轮的 prompt_bytes 真的非零——
	// Budget 不实际起进程，只是让 Select/Render 跑一遍算出这一轮要送多少字节。
	postUser(t, svc, convID, "第三轮预报之前的新内容")

	spawnCallsBeforeBudget := len(fs.calls)
	plan := p3cPlan(testAgentAgy)
	rows := p3cBudgetSafe(t, r, convID, plan)
	if len(fs.calls) != spawnCallsBeforeBudget {
		t.Errorf("Budget 不该起任何进程（它是预报，不是执行），Spawner 调用次数从 %d 变成了 %d",
			spawnCallsBeforeBudget, len(fs.calls))
	}

	row := p3cFindBudgetRow(t, rows, testAgentAgy)
	if !row.Calibrated {
		t.Fatalf("已经跑过两轮真实成功的 Ask，第三轮预报应当 Calibrated=true，实际 false（row=%+v）", row)
	}
	if row.InputTokens == nil {
		t.Fatal("Calibrated=true 时 InputTokens 不该是 nil")
	}
	if row.CostUSD != nil {
		// agy 的 ReportsCost=SupportNo：它自己的 turn_usage 行 CostUSD 恒为
		// nil（Runner.Ask 那条既有规则），Budget 的预报不该替它编一个金额出来——
		// 契约 4.5："金额不能加……不为了凑总金额编汇率"，这条原则同样约束
		// 单行预报，不只是合计。一个非 nil 的 CostUSD 在这里就是一个编出来
		// 的数字，与用例 7 要挡住的那类失败同宗同源，只是长在 6(a) 而不是 7。
		t.Errorf("agy 不报金额（ReportsCost=SupportNo），预报的 CostUSD 应当是 nil，实际 %v", *row.CostUSD)
	}
	if row.InFlight {
		t.Error("本用例没有任何并发 in-flight 的轮次，InFlight 应当为 false")
	}

	// Basis 必须点名一个真实存在的 turn_id，不是断言它非空——去库里查。
	if row.Basis == "" {
		t.Fatal("Calibrated=true 时 Basis 不该是空串")
	}
	if !p3cTurnUsageExists(t, dbPath, row.Basis) {
		t.Fatalf("Basis=%q 在 turn_usage 表里查不到，Calibrated=true 却指向一个不存在的 turn_id", row.Basis)
	}
	// 不钉死"最近一条"具体指向哪一行（呼应文末歧义 3 与歧义 2：SupportNo
	// 的取样规则要不要求 Outcome==OutcomeSpoke、"最近"按什么排序，契约都
	// 没写清楚），只要求 Basis 落在这条会话真实存在过的两轮之一——6(a) 的
	// 重点是"指向一个真实存在、可核验的历史 turn_id"，不是钉死具体挑中
	// 哪一条。是否恰好是 rec2（较新的一轮）留给上面的 p3cTurnUsageExists
	// 与这里的集合归属检查共同核验，不做超出契约文本的强断言。
	if row.Basis != rec1.TurnID && row.Basis != rec2.TurnID {
		t.Errorf("Basis=%q，期望等于这条会话两轮真实成功轮之一（rec1=%q 或 rec2=%q）",
			row.Basis, rec1.TurnID, rec2.TurnID)
	}
}

// TestBudgetUncalibratedBrandNewTeamAllNilNotCalibrated 是用例 6 的 (b)：
// 全新 team（这个 agent 在这份指纹上一条样本都没有）跑一次预报。
//
// 只做这一条不够（任务书原话：「一个『永远返回未标定』的实现也能过」）——
// 本文件把它与上面的 (a) 放在同一个文件里，两条都在。
func TestBudgetUncalibratedBrandNewTeamAllNilNotCalibrated(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "全新会话，从没跑过任何一轮")

	r := newTestRunner(t, cfg, svc)

	plan := p3cPlan(testAgentClaude)
	rows := p3cBudgetSafe(t, r, convID, plan)

	row := p3cFindBudgetRow(t, rows, testAgentClaude)
	if row.Calibrated {
		t.Fatalf("全新会话、零样本，Calibrated 应当为 false，实际 true（row=%+v）", row)
	}
	if row.InputTokens != nil {
		t.Errorf("Calibrated=false 时 InputTokens 应当是 nil，实际 %v", *row.InputTokens)
	}
	if row.CostUSD != nil {
		t.Errorf("Calibrated=false 时 CostUSD 应当是 nil，实际 %v", *row.CostUSD)
	}
	// 11d 3.2 的裁决（Budget 直接继承）：SessionInfo 的 ok==false 必须算必冷。
	// 这条会话从没跑过，ok 必为 false，Cold 预报必为 true。
	if !row.Cold {
		t.Errorf("从没跑过的会话，Cold 预报应当为 true（ok==false 算必冷），实际 false")
	}
}

// ---------------------------------------------------------------------------
// 用例 7：除零守卫——本片最锋利的一条
// ---------------------------------------------------------------------------
//
// 背景（任务书原话）：turn_usage.prompt_bytes 是 NOT NULL DEFAULT 0，片 2a
// 之前的历史行全是 0。预报公式是
//
//	样本.input_tokens / 样本.prompt_bytes × 本轮.prompt_bytes
//
// 样本.prompt_bytes==0 就是一次除零。三个方向都要断言，不能只断言「不崩」：
//
//  1. 不崩（不 panic、不 divide by zero）——p3cBudgetSafe 的 recover 兜底。
//  2. 不给出 Inf / NaN，也不给出一个编出来的数字——最锋利的形式不是拿
//     math.IsInf/IsNaN 去筛一个已经产生的浮点数（那只抓得住"用浮点除法
//     算出 Inf 之后没有再检查"这一种坏法），而是直接断言"零可用样本时
//     InputTokens/CostUSD 必须是 nil"：一个把 prompt_bytes==0 特判成 1
//     的实现（用整数除法或者把分母钳到 >=1）不会崩、也不会产生字面意义
//     上的 Inf/NaN，但会算出一个真实存在、却是编出来的数字，比崩更糟
//     （任务书原话）。断言"必须是 nil"能同时抓住"浮点除零产生 Inf 后
//     直接透传"与"特判成 1 编数字"两类坏实现。用例 7-iii 额外对一个
//     报金额的 provider（claude）做字面的 math.IsInf/IsNaN 检查，把
//     两种判据都覆盖到，互相印证。
//  3. 若它是唯一的样本 -> Calibrated == false。
//
// 7-i／7-ii／7-iii 共用一条陷阱：如果"较新的那条零字节样本"因为某种别的
// 原因（Outcome 不对、cold_start 不匹配、identity_fp 对不上）本来就不会
// 进入取样候选池，那么 Budget 跳过它就什么都没证明——它压根没被考虑过。
// 对策（顾问复核指出的加固）：在调用 Budget 之前先调
// svc.UsageSamples(agentID, fp, N) 断言两条样本都在候选池里，证明两条
// 唯一的差异就是 prompt_bytes，Budget 跳过它一定是因为除零守卫，不是因为
// 别的资格筛选。

// TestBudgetZeroPromptBytesSampleSkippedWhenOnlySample 是 7-i：唯一的样本
// prompt_bytes==0。
func TestBudgetZeroPromptBytesSampleSkippedWhenOnlySample(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "本轮的新内容")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	fp := fingerprintFor(t, r, convID, testAgentAgy, cfg.Agents[testAgentAgy])
	sample := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convID, AgentID: testAgentAgy, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(12345), PromptBytes: 0,
		CreatedAt: time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC),
	})

	// 前置条件：这一条样本真的在候选池里（fp 与 agent 都对得上），
	// 不是因为查不到才 Calibrated=false。
	got, err := svc.UsageSamples(context.Background(), testAgentAgy, fp, 10)
	if err != nil {
		t.Fatalf("UsageSamples 失败: %v", err)
	}
	if len(got) != 1 || got[0].TurnID != sample.TurnID {
		t.Fatalf("前置条件不成立：唯一样本应当在候选池里，实际 UsageSamples 返回 %+v", got)
	}

	plan := p3cPlan(testAgentAgy)
	rows := p3cBudgetSafe(t, r, convID, plan) // 方向 1：不崩。
	if len(fs.calls) != 0 {
		t.Errorf("Budget 不该起任何进程，实际 Spawner 被调用了 %d 次", len(fs.calls))
	}

	row := p3cFindBudgetRow(t, rows, testAgentAgy)

	// 方向 3：唯一样本 prompt_bytes==0，跳完之后零可用样本 -> Calibrated=false。
	if row.Calibrated {
		t.Fatalf("唯一样本 prompt_bytes==0，跳过之后应当零可用样本、Calibrated=false，"+
			"实际 true（row=%+v）——这正是「把 0 特判成 1」那类坏实现的典型表现：不崩，"+
			"但会给出一个编出来的数字", row)
	}
	// 方向 2：不给出 Inf/NaN，也不给出编出来的数字——必须是 nil，不是"某个数"。
	if row.InputTokens != nil {
		t.Errorf("Calibrated=false 时 InputTokens 应当是 nil，实际 %v——这个非 nil 值"+
			"本身就是证据：实现没有正确跳过 prompt_bytes==0 的样本，用某种方式"+
			"（整数除法钳到 1、浮点除法产生 Inf 后未检查……）算出了一个不该存在的数字",
			*row.InputTokens)
	}
	if row.CostUSD != nil {
		t.Errorf("Calibrated=false 时 CostUSD 应当是 nil，实际 %v", *row.CostUSD)
	}
}

// TestBudgetZeroPromptBytesSampleSkippedFallsBackToOlderValidSample 是
// 7-ii：较新的样本 prompt_bytes==0，较旧的样本 prompt_bytes>0 且可用——
// 正确实现应当跳过较新的那条，退回用较旧的那条计算，Basis 指向旧的
// turn_id，不是新的。这是比 7-i 更强的判别：一个"看到最近一条就直接用，
// 不管 prompt_bytes 是不是 0"的实现，在 7-i 里恰好也会因为"唯一样本"
// 而被 7-i 抓到（不管它跳不跳都没有第二条可退），但在这里会被更精确地
// 抓住：它会算出一个基于错误样本（新的那条，input_tokens=99999，故意设
// 得很离谱）的数字，而不是基于正确样本（旧的那条，input_tokens=1000）
// 的数字。
//
// 顾问复核指出的两处加固：
//  1. 插入顺序按时间先后（先插旧的、再插新的），这样无论 Budget 是按
//     created_at 排序还是按插入/rowid 顺序判"最近一条"，两种读法都一致，
//     不会因为"最近一条"的定义不同而产生歧义——这个定义本身契约没写清楚，
//     见文末歧义清单。
//  2. Basis == olderTurnID 是本用例的主断言。数值上界只是次要的兜底：
//     一个"prompt_bytes==0 时就地把 input_tokens 原样透传（不缩放）"的
//     坏实现会给出 99999，仍然落在下面 10^6 的宽松上界之内，不会被数值
//     检查单独抓到——真正能抓住它的是 Basis 指向了错误的 turn_id。
func TestBudgetZeroPromptBytesSampleSkippedFallsBackToOlderValidSample(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "本轮的新内容")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	fp := fingerprintFor(t, r, convID, testAgentAgy, cfg.Agents[testAgentAgy])
	base := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)

	// 先插旧的、可用的。
	older := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convID, AgentID: testAgentAgy, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(1000), PromptBytes: 500,
		CreatedAt: base,
	})
	// 再插新的、prompt_bytes==0 的——若实现只看"最近一条"不做跳过，
	// input_tokens 故意设成 99999，用来在数值上也能看出"用错了样本"。
	newer := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convID, AgentID: testAgentAgy, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(99999), PromptBytes: 0,
		CreatedAt: base.Add(2 * time.Hour),
	})

	// 前置条件：两条样本都在候选池里，唯一的差异是 prompt_bytes。
	got, err := svc.UsageSamples(context.Background(), testAgentAgy, fp, 10)
	if err != nil {
		t.Fatalf("UsageSamples 失败: %v", err)
	}
	gotIDs := map[string]bool{}
	for _, u := range got {
		gotIDs[u.TurnID] = true
	}
	if len(got) != 2 || !gotIDs[older.TurnID] || !gotIDs[newer.TurnID] {
		t.Fatalf("前置条件不成立：两条样本都应当在候选池里，实际 UsageSamples 返回 %d 条：%+v",
			len(got), got)
	}

	plan := p3cPlan(testAgentAgy)
	rows := p3cBudgetSafe(t, r, convID, plan) // 方向 1：不崩。
	row := p3cFindBudgetRow(t, rows, testAgentAgy)

	if !row.Calibrated {
		t.Fatalf("较旧的样本 prompt_bytes>0 可用，跳过较新的零字节样本之后应当"+
			"Calibrated=true，实际 false（row=%+v）", row)
	}
	if row.Basis != older.TurnID {
		t.Fatalf("Basis=%q，期望等于较旧、可用样本的 turn_id=%q（较新的零字节样本"+
			"turn_id=%q 应当被跳过，不该被选中）", row.Basis, older.TurnID, newer.TurnID)
	}
	if row.InputTokens == nil {
		t.Fatal("Calibrated=true 时 InputTokens 不该是 nil")
	}
	// 方向 2 的数值兜底：结果应当是一个有限、非负、量级合理的数字，
	// 不该出现由 Inf/NaN 强转 int 产生的巨大或负数垃圾值。上界给得很宽松
	// （older 样本的缩放比是 1000/500=2 每字节，现实里一轮的 prompt 不会
	// 到 50 万字节），真正精确的判别力在上面 Basis 的比较上，这里只是
	// 兜底不让"看似合理"的坏结果蒙混过关。
	if *row.InputTokens < 0 || *row.InputTokens > 1_000_000 {
		t.Errorf("InputTokens=%d 超出合理量级（[0, 1000000]），"+
			"疑似来自未跳过的零字节样本或 Inf/NaN 强转 int 的垃圾值", *row.InputTokens)
	}
}

// TestBudgetZeroPromptBytesSampleDoesNotProduceInfOrNaNCostUSD 是 7-iii：
// 换一个真的报金额的 provider（claude，ReportsCost=SupportYes），对 CostUSD
// 这个字面意义上的 float64 字段做 math.IsInf/IsNaN 检查——7-i/7-ii 用的是
// agy，CostUSD 对 agy 恒为 nil（不报金额），没有一个真实的浮点数可供检查
// IEEE 754 的 Inf/NaN；这里补上这一半，两种判据互相印证。
//
// claude 的 CacheAcrossTurns=SupportYes，取样规则要按预报的冷暖分支
// （契约 4.5）。本用例从不建立真实的 provider_sessions 行（只插入合成
// turn_usage 样本），svc.ProviderSession 会返回 found=false——按 11d 3.2
// 的裁决（Budget 直接继承）"ok==false 必须算必冷"，本轮预报必为冷，
// 于是取样规则会去找 cold_start=1 的行，两条合成样本都设成 ColdStart:true
// 以匹配这一支；若实现走了别的判据分支（比如去找 cold_start=0 的行），
// 会因为找不到任何 cold_start=1 的样本而 Calibrated=false，本用例会在
// "应当 Calibrated=true"那一步直接失败，能明确指出前提没对上，不会
// 悄悄测错东西。
func TestBudgetZeroPromptBytesSampleDoesNotProduceInfOrNaNCostUSD(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "本轮的新内容")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	fp := fingerprintFor(t, r, convID, testAgentClaude, cfg.Agents[testAgentClaude])
	base := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)

	older := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convID, AgentID: testAgentClaude, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(1000), PromptBytes: 500,
		CostUSD: p3cFloatPtr(0.5), ColdStart: true,
		CreatedAt: base,
	})
	newer := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convID, AgentID: testAgentClaude, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(99999), PromptBytes: 0,
		CostUSD: p3cFloatPtr(123456.0), ColdStart: true,
		CreatedAt: base.Add(2 * time.Hour),
	})

	got, err := svc.UsageSamples(context.Background(), testAgentClaude, fp, 10)
	if err != nil {
		t.Fatalf("UsageSamples 失败: %v", err)
	}
	gotIDs := map[string]bool{}
	for _, u := range got {
		gotIDs[u.TurnID] = true
	}
	if len(got) != 2 || !gotIDs[older.TurnID] || !gotIDs[newer.TurnID] {
		t.Fatalf("前置条件不成立：两条样本都应当在候选池里，实际 UsageSamples 返回 %d 条：%+v",
			len(got), got)
	}

	plan := p3cPlan(testAgentClaude)
	rows := p3cBudgetSafe(t, r, convID, plan) // 方向 1：不崩。
	row := p3cFindBudgetRow(t, rows, testAgentClaude)

	if !row.Cold {
		t.Fatalf("前置条件不成立：这条会话从没建立过真实 provider 会话，" +
			"ok==false 应当预报必冷（Cold=true），实际 false——下面「应当" +
			"命中 cold_start=1 样本」的断言建立在这个前提上")
	}
	if !row.Calibrated {
		t.Fatalf("较旧的样本 prompt_bytes>0 可用，跳过较新的零字节样本之后应当"+
			"Calibrated=true，实际 false（row=%+v）——若这里失败，先确认是不是"+
			"预报冷暖分支与本用例的假设（cold_start=1）没对上", row)
	}
	if row.Basis != older.TurnID {
		t.Fatalf("Basis=%q，期望等于较旧、可用样本的 turn_id=%q", row.Basis, older.TurnID)
	}
	if row.CostUSD == nil {
		t.Fatal("Calibrated=true 且样本报了金额时 CostUSD 不该是 nil")
	}
	// 方向 2 的字面判据：真正对着一个 float64 做 IEEE 754 的 Inf/NaN 检查。
	if math.IsInf(*row.CostUSD, 0) {
		t.Errorf("CostUSD=%v 是 Inf——样本.prompt_bytes==0 的那一行没有被正确跳过，"+
			"除零产生了无穷大", *row.CostUSD)
	}
	if math.IsNaN(*row.CostUSD) {
		t.Errorf("CostUSD 是 NaN——除零产生了 NaN 且没有被拦下")
	}
	// 数值兜底，理由同 7-ii：older 样本的缩放比是 0.5/500=0.001 美元/字节，
	// 现实里一轮不会花到 1000 美元。
	if *row.CostUSD < 0 || *row.CostUSD > 1000 {
		t.Errorf("CostUSD=%v 超出合理量级（[0, 1000]），疑似来自未跳过的零字节样本", *row.CostUSD)
	}
}

// ---------------------------------------------------------------------------
// 用例 8：no_delta 这一档
// ---------------------------------------------------------------------------
//
// 本文件原先在这里发现一处契约洞：冻结的 Budget 签名 `func (r *Runner)
// Budget(ctx, convID string, plan Plan) ([]BudgetRow, error)` 按值接收
// Plan，结构上没有任何通道能把"这个成员本轮没有新内容"写回调用方的
// `Plan.Skipped`。主代理核实后确认这是真洞，裁决记在
// docs/12_STEP9_CONTRACT.md 4.5a 第二条：**`BudgetRow` 加一个 `NoDelta
// bool` 字段，`Budget` 签名不变（不接收 `*Plan`，不产生隐式副作用）；
// `Skipped` 的 no_delta 一档由片 3d 的 `/round` 命令据这个字段组装，
// 不是 Budget 自己组装的。**
//
// 于是用例 8 现在是两条测的不是一回事的用例，都在本文件里：
//
//  1. TestRunRoundSecondRoundNoDeltaMemberZeroSpawnsAndRefused（下面）：
//     "真跑了一轮、被 refuse"那一半。不需要 Budget——RunRound 自己的
//     文档注释已经写明"不判 no_delta……本函数会照常把那些成员送进
//     askUpToWith，由既有 ErrNoDelta 路径 refuse"，而那条 refuse 发生在
//     askUpToWith 调用 r.spawn 之前（runner.go 的既有代码，片 1/片 2
//     已冻结，本片未改）。连续两次 RunRound，第二次没有新内容的成员
//     Outcome 变成 OutcomeRefused、err 包着 ErrNoDelta、且 Spawner 调用
//     次数一次都没有增加。这条用例顺带在中间调一次 Budget，断言它此刻
//     报告的 NoDelta 也是 true——两层各自独立断言，互相印证。
//  2. TestBudgetReportsNoDeltaForMemberWithoutNewContent（下面）：
//     "起进程之前就判出来"那一半，从不调用 RunRound/Ask 第二次、从不
//     碰 Spawner。专门验证 Budget 本身能不能正确判出"这个成员本轮有没有
//     新内容"，且是真判别力（有新内容 -> NoDelta=false，没有 ->
//     NoDelta=true 两个方向都有断言，不是恒真）。
func TestRunRoundSecondRoundNoDeltaMemberZeroSpawnsAndRefused(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "第一轮的新内容")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	plan := p3cPlan(testAgentAgy)

	fs.enqueueSuccess(agyT1JSON)
	records1 := r.RunRound(context.Background(), convID, plan, 0)
	if len(records1) != 1 {
		t.Fatalf("第一轮 RunRound 返回 %d 条 Record，期望 1", len(records1))
	}
	if records1[0].Outcome != OutcomeSpoke {
		t.Fatalf("第一轮前置条件不成立：Outcome=%v，期望 OutcomeSpoke", records1[0].Outcome)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("第一轮前置条件不成立：Spawner 调用次数=%d，期望 1", len(fs.calls))
	}

	// 中间不 post 任何新消息：第二轮对这个成员而言没有新内容。
	// filterDelta 恒过滤掉 selfID 自己发的消息（router.go: skipSelf 恒 true），
	// 所以第一轮它自己的回复不会被它自己在第二轮当成"新内容"看到。

	// 按 /round 的真实顺序，RunRound 第二轮之前先调一次 Budget（4.5a 的
	// 裁决：这一步"起进程之前就判出来"）。用 Errorf 不用 Fatalf：这条与
	// 下面 RunRound 层的断言是两条独立证据，一条出错不该盖住另一条——
	// 这正是这条用例存在的意义（见上方长注释第 1 条）。
	rowsBeforeRound2 := p3cBudgetSafe(t, r, convID, plan)
	if row := p3cFindBudgetRow(t, rowsBeforeRound2, testAgentAgy); !row.NoDelta {
		t.Errorf("RunRound 第二轮之前调 Budget，这个成员自第一轮以来没有新内容，"+
			"NoDelta 应当为 true，实际 false（row=%+v）", row)
	}

	records2 := r.RunRound(context.Background(), convID, plan, 0)
	if len(records2) != 1 {
		t.Fatalf("第二轮 RunRound 返回 %d 条 Record，期望 1", len(records2))
	}
	rec2 := records2[0]
	if rec2.Outcome == OutcomeSpoke {
		t.Fatalf("第二轮没有新内容，Outcome 不该是 OutcomeSpoke，实际是（row=%+v）", rec2)
	}
	if rec2.Outcome != OutcomeRefused {
		t.Errorf("第二轮没有新内容，期望 Outcome=OutcomeRefused，实际 %v", rec2.Outcome)
	}
	if !errors.Is(rec2.Err, ErrNoDelta) {
		t.Errorf("第二轮没有新内容，期望 rec2.Err 包着 ErrNoDelta，实际 %v", rec2.Err)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("第二轮没有新内容，不该起任何新进程，Spawner 累计调用次数应当仍是 1，实际 %d", len(fs.calls))
	}

	// 结算行的 N 不含它：round_finished 面包屑的 spawned 应当是 0。
	// 这一条验的是既有片 3b 代码（roundFinishedMeta/roundSpawned，
	// runner.go 与 scheduler.go 里已冻结提交的部分）在这个场景下的既有
	// 行为，不是本片（3c）新增的任何东西——roundSpawned 对 OutcomeRefused
	// 恒返回 false，本条只是确认这一层既有逻辑与"no_delta"场景吻合，
	// 顺带把契约 4.9 第 8 条"结算行的 N 不含他们"这半句也钉上。
	msgs, err := svc.History(context.Background(), convID, 0, 10000)
	if err != nil {
		t.Fatalf("读历史失败: %v", err)
	}
	var lastFinished *conversation.Message
	for i := range msgs {
		m := &msgs[i]
		if m.Kind == conversation.KindSystem {
			if ev, _ := m.Meta[MetaEvent].(string); ev == "round_finished" {
				lastFinished = m
			}
		}
	}
	if lastFinished == nil {
		t.Fatal("库里没有找到任何 event=round_finished 的 KindSystem 消息")
	}
	if spawned, ok := p3cMetaInt(lastFinished.Meta, "spawned"); !ok {
		t.Errorf("round_finished 面包屑的 Meta[\"spawned\"] 不是数字: %#v", lastFinished.Meta["spawned"])
	} else if spawned != 0 {
		t.Errorf("第二轮结算行的 spawned=%d，期望 0（唯一的成员本轮没有新内容，不该被计入）", spawned)
	}
}

// TestBudgetReportsNoDeltaForMemberWithoutNewContent 是用例 8 的第二半：
// "起进程之前就判出来"。全程不调 RunRound、不调第二次 Ask、不碰 Spawner——
// 只验证 Budget 自己能不能正确判出"这个成员本轮有没有新内容"。
//
// 两个方向都断言，不是恒真的空用例：同一份"已经跑过一轮"的前置状态下，
// 中间不 post 新内容 -> NoDelta 应当是 true；中间 post 一条新内容 ->
// NoDelta 应当是 false。只写其中一个方向的话，一个把 NoDelta 恒写成
// true（或恒写成 false）的实现也能骗过去。
//
// 判据是什么（4.5a 第二条没有展开，这里说清楚，写给下一个读这条用例的人）：
// NoDelta 的判据应当与 askUpToWith 判 ErrNoDelta 的判据同一份输入——
// `len(Select(readAfter(cursor), ac, agentID, bootstrap)) == 0`，
// cursor 取自这个 (会话, 成员) 在 provider_sessions 里的 cursor_seq，
// 不是从 seq=0 读全量历史。这条边界值得说清楚：若 Budget 的实现偷懒，
// 直接拿整段历史（包含第一轮就存在的种子消息）去跑 Select，
// 即便这个成员早就在第一轮读过、回复过那条种子消息，Select 依然会
// 把它当成"新内容"选出来（Select 本身不认识游标，只认识调用方喂给它
// 的窗口）——NoDelta 就会被算成 false，即便真相是"没有新内容"，
// 本条用例的"没有新内容"那一半会在这种坏实现下失败，这正是它要抓的。
func TestBudgetReportsNoDeltaForMemberWithoutNewContent(t *testing.T) {
	cfg := newTestConfig()
	plan := p3cPlan(testAgentAgy)

	// setupAfterOneRound 建立"已经跑过一轮、provider 会话已建立、游标已
	// 推进"的前置状态——只有这样，第二轮对这个成员而言才可能真的是
	// "没有新内容"；一个从没问过的（bootstrap）成员只要会话里有过任何
	// 一条用户消息，selectBootstrap 就会把它选出来，不会出现 NoDelta。
	setupAfterOneRound := func(t *testing.T) (svc *conversation.Service, r *Runner, convID string) {
		t.Helper()
		svc = newTestService(t)
		convID = newConv(t, svc)
		joinAgent(t, svc, convID, testAgentAgy)
		postUser(t, svc, convID, "第一轮的种子内容")

		r = newTestRunner(t, cfg, svc)
		fs := &fakeSpawner{}
		r.spawn = fs.spawn
		fs.enqueueSuccess(agyT1JSON)
		rec, err := r.Ask(context.Background(), convID, testAgentAgy)
		if err != nil || rec.Outcome != OutcomeSpoke {
			t.Fatalf("前置条件不成立：第一轮 Ask 应当成功，实际 err=%v outcome=%v", err, rec.Outcome)
		}
		return svc, r, convID
	}

	t.Run("没有新内容_NoDelta应当为true", func(t *testing.T) {
		_, r, convID := setupAfterOneRound(t)
		// 中间不 post 任何新内容。

		rows := p3cBudgetSafe(t, r, convID, plan)
		row := p3cFindBudgetRow(t, rows, testAgentAgy)
		if !row.NoDelta {
			t.Errorf("这个成员自上一轮以来没有新内容，NoDelta 应当为 true，实际 false（row=%+v）", row)
		}
	})

	t.Run("有新内容_NoDelta应当为false", func(t *testing.T) {
		svc, r, convID := setupAfterOneRound(t)
		postUser(t, svc, convID, "第一轮之后新出现的内容")

		rows := p3cBudgetSafe(t, r, convID, plan)
		row := p3cFindBudgetRow(t, rows, testAgentAgy)
		if row.NoDelta {
			t.Errorf("这个成员上一轮之后出现了新内容，NoDelta 应当为 false，实际 true（row=%+v）", row)
		}
	})
}

// TestBudgetSameCreatedAtSamplesDeterministicBasisAcrossRepeatedCalls 覆盖
// docs/12_STEP9_CONTRACT.md 4.5a 第三条的裁决："最近一条"按
// `ORDER BY created_at DESC`，并以 `turn_id` 作第二排序键——理由是本地
// SQLite 的写是亚毫秒级的，两条样本完全可能拿到相同的 created_at 字符串，
// 此时顺序若靠 SQLite 的实现细节决定，同一份库跑两次 Budget 可能给出两个
// 不同的 Basis，这正是片 3a 测 map 迭代顺序那条病的同一个变种（确定性
// 不能靠运气）。
//
// 两条样本 created_at 完全相同，InputTokens 只相差 30%（1000 对 1300，
// 不是整数倍），是为了让"两次调用选中了不同的样本"这件事在数值上也
// 看得出来，不会被一次巧合的四舍五入掩盖——顾问复核指出：若两条样本的
// input_tokens 相差整数倍（比如 2 倍），一个有 bug 的实现即使读错了样本，
// 算出来的预报值也可能恰好等于正确值的整数倍，看起来"合理"，实际上
// 验证的是错误的东西。
//
// 同时断言 Basis 与 InputTokens 两者在重复调用间都不变——只验 Basis 不够：
// 一个"判定用哪一条的查询与实际用来算比例的查询是两条不同的、tiebreak
// 不一致的 SQL"的实现，可能 Basis 报得对、but 实际算比例时读到了另一条，
// 这种情况仅靠 Basis 相等看不出来。
func TestBudgetSameCreatedAtSamplesDeterministicBasisAcrossRepeatedCalls(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "本轮的新内容")

	r := newTestRunner(t, cfg, svc)
	fp := fingerprintFor(t, r, convID, testAgentAgy, cfg.Agents[testAgentAgy])
	same := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)

	a := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convID, AgentID: testAgentAgy, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(1000), PromptBytes: 500,
		CreatedAt: same,
	})
	b := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convID, AgentID: testAgentAgy, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(1300), PromptBytes: 500,
		CreatedAt: same,
	})

	plan := p3cPlan(testAgentAgy)
	var basisSeen string
	var inputTokensSeen int
	const trials = 10
	for i := 0; i < trials; i++ {
		rows := p3cBudgetSafe(t, r, convID, plan)
		row := p3cFindBudgetRow(t, rows, testAgentAgy)
		if !row.Calibrated {
			t.Fatalf("第 %d 次调用前置条件不成立：应当 Calibrated=true", i+1)
		}
		if row.Basis != a.TurnID && row.Basis != b.TurnID {
			t.Fatalf("第 %d 次调用 Basis=%q 既不是样本 a（%q）也不是样本 b（%q）",
				i+1, row.Basis, a.TurnID, b.TurnID)
		}
		if row.InputTokens == nil {
			t.Fatalf("第 %d 次调用前置条件不成立：Calibrated=true 时 InputTokens 不该是 nil", i+1)
		}
		if i == 0 {
			basisSeen = row.Basis
			inputTokensSeen = *row.InputTokens
			continue
		}
		if row.Basis != basisSeen {
			t.Fatalf("第 %d 次调用 Basis=%q，与第一次的 %q 不同——created_at 相同的两条样本，"+
				"选中哪一条必须是确定的（4.5a 裁决：turn_id 作第二排序键），不能在同一份库上"+
				"跑出两个不同结果", i+1, row.Basis, basisSeen)
		}
		if *row.InputTokens != inputTokensSeen {
			t.Fatalf("第 %d 次调用 InputTokens=%d，与第一次的 %d 不同——Basis 相等只说明"+
				"\"报的是哪条\"这件事确定，这里进一步确认\"用来算比例的确实是同一条\"",
				i+1, *row.InputTokens, inputTokensSeen)
		}
	}
}

// TestBudgetSupportNoBranchUsesOutcomePostRejectedSample 覆盖
// docs/12_STEP9_CONTRACT.md 4.5a 第四条的裁决：4.5 里 SupportNo 那一条
// 写"取最近一条**成功轮**的 input_tokens"是措辞不严，真正的判据是同节
// "样本口径"那句——input_tokens 非 NULL 即算样本，OutcomePostRejected
// 也算。三支（SupportNo / SupportYes 冷 / SupportYes 暖）统一。
//
// 订正（2026-09-23，X-35b）：上面那段裁决已被收窄。样本口径现在是
// 「Ingested() 且 input_tokens 非 NULL 且 prompt_bytes > 0」，三支仍然统一。
// 原因：失败轮的用量是货真价实的 0（provider.go:279-280 的失败探针），
// 却因为 runner.go:702 在失败返回前吸收了用量而是非 NULL，旧口径会拿它把预报缩成 0 token。
// 「成功轮」原来不是措辞不严，是本意。OutcomePostRejected 是 Ingested，仍然算样本，
// 所以本条用例的断言一个字不用改；它钉的「PostRejected 在 SupportNo 支也算」仍然成立。
//
// 本文件用例 5 的
// TestUsageSamplesOutcomePostRejectedCountsNullInputTokensExcluded 已经
// 钉死了 UsageSamples 这一层的口径，但那不能代替这一条：Budget 的
// SupportNo 分支完全可能在 UsageSamples 已经把 OutcomePostRejected 那行
// 交出来之后，自己又按 Outcome==OutcomeSpoke 二次过滤掉它——那样
// UsageSamples 层面的用例全绿，Budget 层面的口径却是错的，只有直接对着
// SupportNo（agy）这一支断言才能抓住。
//
// 造的唯一样本 Outcome=OutcomePostRejected、没有任何 OutcomeSpoke 的
// 样本可选：若 Budget 的 SupportNo 分支要求 Outcome==OutcomeSpoke，
// 这里会因为"零可用样本"而 Calibrated=false，与期望的 true 直接矛盾，
// 不会被巧合地蒙混过去。
func TestBudgetSupportNoBranchUsesOutcomePostRejectedSample(t *testing.T) {
	cfg := newTestConfig()
	svc, dbPath := newTestServiceWithPath(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "本轮的新内容")

	r := newTestRunner(t, cfg, svc)
	fp := fingerprintFor(t, r, convID, testAgentAgy, cfg.Agents[testAgentAgy])

	only := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: convID, AgentID: testAgentAgy, IdentityFP: fp,
		Outcome: int(OutcomePostRejected), InputTokens: p3cIntPtr(900), PromptBytes: 450,
		CreatedAt: time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC),
	})

	plan := p3cPlan(testAgentAgy)
	rows := p3cBudgetSafe(t, r, convID, plan)
	row := p3cFindBudgetRow(t, rows, testAgentAgy)

	if !row.Calibrated {
		t.Fatalf("唯一样本是 OutcomePostRejected 且 input_tokens 非 NULL，SupportNo 分支应当"+
			"仍然 Calibrated=true，实际 false（row=%+v）——若这里失败，疑似 Budget 的 SupportNo"+
			"分支自己另外要求 Outcome==OutcomeSpoke，与 4.5a 第四条裁决（以「样本口径」为准，"+
			"三支统一）不符", row)
	}
	if row.Basis != only.TurnID {
		t.Errorf("Basis=%q，期望等于唯一的那条 OutcomePostRejected 样本（%q）", row.Basis, only.TurnID)
	}
	if !p3cTurnUsageExists(t, dbPath, row.Basis) {
		t.Fatalf("Basis=%q 在 turn_usage 表里查不到", row.Basis)
	}
}

// ---------------------------------------------------------------------------
// 歧义清单（写进代码里，报告里逐条复述一遍，两处不冲突时以此处为准）。
// ---------------------------------------------------------------------------
//
// 下面六条最初都是本文件写用例时发现的契约空洞，报给主代理之后，主代理
// 核实源码、逐条裁决，写进了 docs/12_STEP9_CONTRACT.md 4.5a
// （2026-09-22）。四条（1/3/4/6）已经有了明确裁决，本文件已经据裁决
// 补上对应用例；一条（2）裁决之后本文件补了专门的确定性用例；
// 一条（5）仍然是空的，留在这里。**逐条列出裁决内容与对应用例，
// 不只是留一句"已解决"**——这样报告与代码两处不冲突时，任何一处都能
// 独立还原出完整的历史。
//
// 1.（原 headline）Budget 的冻结签名按值接收 Plan，结构上没有通道把
//    "该成员本轮无新内容"写回调用方的 Plan.Skipped。
//    **裁决（4.5a 第二条）**：`BudgetRow` 加一个 `NoDelta bool` 字段；
//    `Budget` 签名不变，不接收 `*Plan`，不产生隐式副作用；`Skipped` 的
//    no_delta 一档由片 3d 的 `/round` 命令据 `NoDelta` 组装，不是
//    Budget 自己组装的。
//    **对应用例**：TestBudgetReportsNoDeltaForMemberWithoutNewContent
//    （两个方向都测：有新内容 NoDelta=false，没有 NoDelta=true）；
//    TestRunRoundSecondRoundNoDeltaMemberZeroSpawnsAndRefused 里也补了
//    一条 NoDelta==true 的断言，与 RunRound 层的 ErrNoDelta 断言互相印证。
//
// 2. "最近一条"没有定义排序键：可能是 created_at，也可能是插入/rowid
//    顺序。
//    **裁决（4.5a 第三条）**：`ORDER BY created_at DESC`，并以 `turn_id`
//    作第二排序键——理由是本地 SQLite 写入是亚毫秒级的，同一轮扇出的
//    多条 turn_usage 可能拿到相同的 created_at 字符串，此时顺序不能靠
//    SQLite 的实现细节决定，否则同一份库跑两次 Budget 可能给出两个不同
//    的 Basis。
//    **对应用例**：
//    TestBudgetSameCreatedAtSamplesDeterministicBasisAcrossRepeatedCalls
//    ——两条 created_at 完全相同的样本，重复调用 10 次，Basis 与
//    InputTokens 都必须逐次相等。
//
// 3. 契约 4.5 SupportNo 那一条的措辞是"取最近一条**成功轮**的
//    input_tokens"，与同节"样本口径"那句"只要 input_tokens 非 NULL
//    就算样本（含 OutcomePostRejected）"字面不一致。
//    **裁决（4.5a 第四条）**：以"样本口径"为准，三支（SupportNo /
//    SupportYes 冷 / SupportYes 暖）统一——"成功轮"是措辞不严，不是
//    另立判据；排除 OutcomePostRejected 就是往低估方向偏。
//    **订正（2026-09-23，X-35b）**：样本口径已收窄为「Ingested() 且
//    input_tokens 非 NULL 且 prompt_bytes > 0」，三支仍统一；失败轮的用量是
//    真实的 0，旧口径会拿它把预报缩成 0 token。PostRejected 仍算（它是 Ingested），
//    「排除它往低估偏」这半句仍然成立。见 budget.go 的 pickUsageSample 注释。
//    **对应用例**：TestBudgetSupportNoBranchUsesOutcomePostRejectedSample
//    ——唯一样本是 OutcomePostRejected（没有任何 OutcomeSpoke 样本可选），
//    直接对着 SupportNo（agy）这一支断言 Calibrated=true，不能只靠
//    UsageSamples 层面的用例代替（Budget 完全可能在拿到样本之后自己
//    又用 Outcome==OutcomeSpoke 二次过滤）。
//
// 4. 一个"本轮无新内容"的成员，会不会出现在 Budget 返回的 []BudgetRow
//    里，还是被整个排除在返回值之外。
//    **裁决**：随歧义 1 一起解决——`NoDelta bool` 字段的存在本身回答了
//    这个问题：该成员**必然出现**在返回的 []BudgetRow 里，只是
//    `NoDelta=true`，不是被排除在外（字段是加在行上的，不是加一个
//    "是否出现"的开关）。见 TestBudgetReportsNoDeltaForMemberWithoutNewContent
//    ——两个子用例里 p3cFindBudgetRow 都能找到那一行，找不到直接 Fatal，
//    这本身就是对"必然出现"的断言。
//
// 5.（仍未解决）UsageSamples 的 limit 参数语义没有专门测试："最近 N 条"
//    里 N 到底按什么排序（呼应歧义 2 已裁的 created_at DESC + turn_id
//    次排序，limit 大概率沿用同一个排序取前 N 条，但契约没有把这一点
//    写成"UsageSamples 与 Budget 内部排序必须是同一份实现"）、以及
//    limit 小于候选总数时具体截断哪些，本文件全程只传了一个足够大的
//    limit（10），没有构造出候选数超过 limit 的场景去验证截断行为，
//    也没有验证 limit<=0 时的处置。
//
// 6.（原"在隔离桩里造实现时发现"）契约 4.5 那句"Budget 要算本轮的
//    prompt_bytes，那要把身份块渲染出来"容易被读成"prompt_bytes 含
//    身份块"。
//    **裁决（4.5a 第一条，最要紧的一条）**："本轮.prompt_bytes"就是
//    `len(router.Render(selected, conv.Title, bootstrap))`，不含身份块——
//    与落进 `turn_usage.prompt_bytes` 的那个量（`rec.PromptBytes =
//    len(prompt)`，身份块是另一个字段 `TurnRequest.Role`，从不计入
//    `len(prompt)`）逐字节同定义。Budget 渲染身份块只为 ArgvHeadroom，
//    不进缩放的乘数。
//
//    **裁决同时把这条公式自带的近似写明**（不许通过"把身份块算进
//    prompt_bytes"去修，那会破坏与 turn_usage 那一列的同定义）：
//    样本.input_tokens 是 provider 看到的全部输入（身份块 + delta）的
//    token 数，样本.prompt_bytes 只是 delta 的字节数——缩放时把固定的
//    身份块成本当成了按比例变化的，本轮 delta 比样本大很多时偏高、
//    小很多时偏低。这是已知近似，4.5 已经为此立了规矩：方向不作硬
//    断言，Basis 点名 turn_id，预报与实测并排印，方向只记录、不承诺。
//    **对应用例**：本文件的用例 6(a)/7-ii/7-iii 因此都只用宽松的数量级
//    上界断言，不断言 InputTokens/CostUSD 的精确值——这不是没测全，是
//    裁决本身就不许精确断言（钉精确值就是把一个已知近似钉成契约）。
//    隔离桩里验证判别力的"好"实现按裁决只用
//    `len(router.Render(...))`（不含身份块）计算本轮 prompt_bytes。
