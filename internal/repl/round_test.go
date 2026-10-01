package repl

// 本文件是 internal/repl 里 /round 命令层的第一组测试。round_render_test.go
// 那七条只喂合成数据直接调 renderBudget / renderSettlement 两个纯渲染函数，
// 一条都不经过 cmdRound；本文件走真正的 cmdRound 分派，验证 --rounds N、
// --with-judge、空 Speakers 这三块命令层行为。
//
// 背对背：写这份文件时实现方正在并行改 internal/repl/round.go（加
// --rounds 与 --with-judge），本文件没有读那份进行中的改动来猜断言——
// 断言全部来自冻结契约本身，以及现查到的既有代码（PlanRound、
// budgetOne、RunRoundPrepared/RunRoundWith 等片 3b/3c 已冻结提交的部分）。
// 大多数用例现在会红：--rounds / --with-judge 这两个 flag 还没被
// cmdRound 认得，会被当成不认识的 agent id 处理，走「不是当前活跃
// 成员」那条拒绝分支。红在哪里、为什么红，写在最终报告里。
//
// 观测手段：不注入假 Spawner（跨包设不了 turn.Runner 的不导出字段
// spawn），改用事实源——RunRoundPrepared/RunRoundWith 先写 round_started
// 面包屑、再起进程（scheduler.go），于是「起没起过这一轮」与「起没起
// 进程」是两件独立可观测的事，查库里的 KindSystem 面包屑即可，不需要
// 真的起得起来子进程。

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// ============================================================
// 计数装饰器与通用夹具
// ============================================================

// countingMaxSeqStore 包一层 *fakeStore，只在 MaxSeq 上计数。
//
// internal/turn/budget.go 的 TakeRoundSnap 调 r.svc.MaxSeq，这是
// internal/turn 包里 Store.MaxSeq 唯一的调用点（2026-09-22 现查：
// grep -rn "MaxSeq(" internal --include=*.go | grep -v _test 只有八处，
// 定义两处 + internal/repl/ask.go 两处（/ask 带文本时落库前后各问一次，
// 与 /round 无关）+ internal/repl/commands.go 一处（cmdLog 用）+
// internal/repl/round.go 一处（lastUserMentionIDs，只在不给显式 id 时
// 才会调）+ internal/turn/budget.go 这一处。本文件的用例只要（a）全部
// 给显式 id、（b）不碰 /ask 与 /log，就能保证计数器只统计
// TakeRoundSnap 的调用次数，口径干净。
//
// 计数的作用域：只在经 conversation.Service.MaxSeq 发起的调用上计数。
// fakeStore.WriteTx 把内层 *fakeStore 自己（不是这层包装）当 Tx 传给
// fn——但 MaxSeq 根本不在 conversation.Tx 接口里，不存在「经 Tx 路径
// 绕过计数器」这回事，本文件全程都是经 Service.MaxSeq 读，不需要
// 担心统计口径分裂。
type countingMaxSeqStore struct {
	*fakeStore
	maxSeqCalls int
}

var _ conversation.Store = (*countingMaxSeqStore)(nil)

func (c *countingMaxSeqStore) MaxSeq(ctx context.Context, convID string) (int64, error) {
	c.maxSeqCalls++
	return c.fakeStore.MaxSeq(ctx, convID)
}

// newCountingRoundFixture 造一套跑 /round 用的最小夹具：计数装饰过的
// fakeStore、挂在它上面的 conversation.Service、一个真正的 *turn.Runner
// （复用 newTestREPLConfig 的三个 agent：ag_alpha/ag_mid 是发言者
// （Mode 分别是 ModeDiscussion / ModeWorker），ag_zeta 是 config.ModeJudge），
// 外加一个已建好、id 已知、三人都已入座的会话。
//
// 三个 agent 的 provider（pv_alpha/pv_mid/pv_zeta）都不在
// internal/provider 的全局 registry 里（registry 只登记 claude/grok/agy，
// 见 internal/provider/{claude,grok,agy}.go 的 init 调 Register），budgetOne
// 里的 provider.Lookup 会失败、提前返回一个 NoDelta 恒为 false 的空行；
// 真正 Ask 时会在 provider.Lookup 那一步被拒绝，不起真进程
// （ask_test.go newTestRunner 的注释已经论证过这一点，同一套配置这里
// 直接复用）。round_started/round_finished 面包屑在 RunRoundWith 里是
// 起进程之前写的，refuse 不影响这半句已经发生的事实——「起没起过这一轮」
// 与「起没起进程」因此是两件可以分开观测的事，这正是本文件全程要用的
// 观测手段。
func newCountingRoundFixture(t *testing.T) (*conversation.Service, *countingMaxSeqStore, *turn.Runner, string) {
	t.Helper()
	inner := newFakeStore()
	cs := &countingMaxSeqStore{fakeStore: inner}
	svc := conversation.NewService(cs)
	rn := turn.NewRunner(newTestREPLConfig(), svc, t.TempDir(), turn.RulesBlocks{})

	const convID = "conv_round_fixture0000000000000000"
	inner.seedConversation(convID, "round 命令层用例")
	inner.seedParticipant(t, convID, "ag_alpha")
	inner.seedParticipant(t, convID, "ag_mid")
	inner.seedParticipant(t, convID, "ag_zeta")
	return svc, cs, rn, convID
}

// roundBreadcrumbs 从 svc 里读出全部 event=round_started / round_finished
// 的 KindSystem 消息。查法照抄 internal/turn/step9p3c_budget_usage_test.go
// 里查面包屑的办法（遍历 History、按 Kind==KindSystem 和
// Meta[MetaEvent] 过滤），limit 给得足够大盖住整段历史。
func roundBreadcrumbs(t *testing.T, svc *conversation.Service, convID string) (started, finished []conversation.Message) {
	t.Helper()
	msgs, err := svc.History(context.Background(), convID, 0, 1000000)
	if err != nil {
		t.Fatalf("读历史失败: %v", err)
	}
	for _, m := range msgs {
		if m.Kind != conversation.KindSystem {
			continue
		}
		ev, _ := m.Meta[turn.MetaEvent].(string)
		switch ev {
		case "round_started":
			started = append(started, m)
		case "round_finished":
			finished = append(finished, m)
		}
	}
	return started, finished
}

// breadcrumbRoundID 取一条面包屑的 Meta[round_id]。用 fmt.Sprint 而不是
// 断言成 string：fakeStore 原样存 map[string]any，实际存的就是 string，
// 但不必在这里对它的动态类型打赌。
func breadcrumbRoundID(t *testing.T, m conversation.Message) string {
	t.Helper()
	v, ok := m.Meta[turn.MetaRoundID]
	if !ok {
		t.Fatalf("面包屑缺 Meta[round_id]: %+v", m)
	}
	return fmt.Sprint(v)
}

// assertBreadcrumbPairs 断言 started 与 finished 各恰好 n 条、started 的
// round_id 互不相同、finished 的每个 round_id 都能在 started 的集合里
// 配上对——「N 对」不只是两个数字凑巧相等，是真的配对上的 N 对。
// 返回 started 的 round_id 集合，供调用方需要时复用。
func assertBreadcrumbPairs(t *testing.T, started, finished []conversation.Message, n int, label string) map[string]bool {
	t.Helper()
	if len(started) != n || len(finished) != n {
		t.Fatalf("%s: 应当各有 %d 条 round_started/round_finished 面包屑，实际 started=%d finished=%d",
			label, n, len(started), len(finished))
	}
	startedIDs := make(map[string]bool, n)
	for _, m := range started {
		id := breadcrumbRoundID(t, m)
		if startedIDs[id] {
			t.Errorf("%s: round_started 的 round_id=%q 重复出现", label, id)
		}
		startedIDs[id] = true
	}
	if len(startedIDs) != n {
		t.Errorf("%s: %d 条 round_started 面包屑应当有 %d 个互不相同的 round_id，实际只有 %d 个不同值",
			label, n, n, len(startedIDs))
	}
	for _, m := range finished {
		id := breadcrumbRoundID(t, m)
		if !startedIDs[id] {
			t.Errorf("%s: round_finished 的 round_id=%q 在 round_started 集合里找不到配对", label, id)
		}
	}
	return startedIDs
}

// ============================================================
// 1. --go 闸：不带 --go 不起任何一轮
// ============================================================

// TestRoundNoGoStartsNothing（验收 1）挡的坏实现：默认（不带 --go）也
// 真的起了进程 / 写了 round_started。同一个夹具先跑一次不带 --go 的，
// 断言库里没有任何 round_started；再原地跑一次带 --go 的（对照组），
// 断言确实出现了 round_started——证明「上一步没有面包屑」不是因为这套
// 夹具本身写不出面包屑（比如 provider 不在 registry 里就直接整体失败），
// 而是「不带 --go 真的什么都没起」。两半都在同一个 svc/store 上跑，
// 断言都发生在 Run 已经从 done channel 返回之后，happens-before 由 channel 建立；
// fakeStore 自 2026-09-25 起另有自己的锁（见 conversation_test.go），管的是 worker 之间。
func TestRoundNoGoStartsNothing(t *testing.T) {
	svc, _, rn, convID := newCountingRoundFixture(t)

	outNoGo, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha\n")
	if err != nil {
		t.Fatalf("不带 --go 时 Run 应当返回 nil，实际 %v；完整输出:\n%s", err, outNoGo)
	}
	startedBefore, _ := roundBreadcrumbs(t, svc, convID)
	if len(startedBefore) != 0 {
		t.Fatalf("不带 --go 不该写下任何 round_started 面包屑，实际 %d 条；完整输出:\n%s",
			len(startedBefore), outNoGo)
	}

	outGo, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha --go\n")
	if err != nil {
		t.Fatalf("带 --go 时 Run 应当返回 nil，实际 %v；完整输出:\n%s", err, outGo)
	}
	startedAfter, _ := roundBreadcrumbs(t, svc, convID)
	if len(startedAfter) == 0 {
		t.Fatalf("对照组：同一夹具带 --go 应当至少写下一条 round_started 面包屑，"+
			"实际 0 条，说明这套夹具本身就观测不出「起没起过这一轮」；完整输出:\n%s", outGo)
	}
}

// ============================================================
// 2. 单轮只取一次快照
// ============================================================

// TestRoundSingleRoundTakesExactlyOneSnapshot（验收 2b）挡的坏实现：
// cmdRound 分开调公开的 Budget(ctx, convID, plan)（内部自己 TakeRoundSnap
// 一次）与 RunRound(ctx, convID, plan, 0)（内部又 TakeRoundSnap 一次），
// 而不是 TakeRoundSnap 一次、把同一份 snap 传给 BudgetWith 与
// RunRoundPrepared 共用。错接法下 Store.MaxSeq 会被调 2 次
// （internal/turn/budget.go 的 TakeRoundSnap 是 MaxSeq 唯一调用点，
// 见本文件顶部 countingMaxSeqStore 的长注释），正确接法只调 1 次。
//
// 给显式 id 很重要：不给的话 round.go 里的 lastUserMentionIDs 会自己
// 先调一次 r.svc.MaxSeq（在决定 ids 之前，找「最后一条用户发言」），
// 那次调用与 TakeRoundSnap 那次来源不同、挡的坏实现也不同，混在一起
// 会把「给隐式 id 的额外读」和「错接法 Budget+RunRound 各拍一次」
// 记成同一个数字，分不清红的是哪一种。
func TestRoundSingleRoundTakesExactlyOneSnapshot(t *testing.T) {
	svc, cs, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	if cs.maxSeqCalls != 1 {
		t.Fatalf("Store.MaxSeq 调用次数=%d，期望恰好 1。"+
			"TakeRoundSnap 应当只取一次快照，BudgetWith 与 RunRoundPrepared 共用它；"+
			"错接法（分别调公开的 Budget() 与 RunRound()，各自 TakeRoundSnap 一次）"+
			"会是 2。完整输出:\n%s", cs.maxSeqCalls, out)
	}
}

// ============================================================
// 3. --rounds 3：跑三轮，round_id 各不相同
// ============================================================

// TestRoundsThreeRunsThreeRoundsWithDistinctIDs（验收 9）挡的坏实现：
// --rounds N 只取一次快照、只发一对面包屑，或者虽然跑了 N 次但共用同
// 一个 round_id。三重断言分别挡三种退化：面包屑对数不对、round_id 不
// distinct、MaxSeq 没有跟着重新取（契约「每一轮都重新取快照」）。
func TestRoundsThreeRunsThreeRoundsWithDistinctIDs(t *testing.T) {
	svc, cs, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha --rounds 3 --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	started, finished := roundBreadcrumbs(t, svc, convID)
	assertBreadcrumbPairs(t, started, finished, 3, "--rounds 3")

	if cs.maxSeqCalls != 3 {
		t.Errorf("Store.MaxSeq 调用次数=%d，期望恰好 3："+
			"契约要求「每一轮都重新取快照」，3 轮应当各自 TakeRoundSnap 一次，"+
			"不是共用一份快照跑三次（那样会是 1）；完整输出:\n%s", cs.maxSeqCalls, out)
	}
}

// ============================================================
// 4. --rounds > 1 不带 --go 必须被拒绝
// ============================================================

// TestRoundsWithoutGoIsRefused（验收 10）挡的坏实现：N > 1 时即使不带
// --go 也照样印出预报表（把「计划」当成了不需要 --go 闸的东西），或者
// 虽然拒绝了但已经偷偷取过快照 / 印过预算表。
//
// "拒绝"是这个代码库里已有的统一措辞（round.go 里「不是当前活跃成员」
// 「名单为空」「命令行余量为负」三条现有拒绝分支都以它开头），用它判定
// 断言不锁具体话术。"未标定"是契约 4.6a.3 钦定的占位词：本文件的假
// Store 让 UsageSamples 恒返回 nil, nil，行都是 Calibrated=false
// （round_render_test.go 顶部注释、ask_test.go newTestRunner 注释都点过
// 这件事），renderBudget 只要真被调用过，输出里就必然含"未标定"；
// 不含它就说明预算表没有被打印出来。
func TestRoundsWithoutGoIsRefused(t *testing.T) {
	svc, cs, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha --rounds 2\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	if !strings.Contains(out, "拒绝") {
		t.Fatalf("--rounds > 1 不带 --go 应当被拒绝，屏幕上应出现“拒绝”字样，实际输出:\n%s", out)
	}
	if strings.Contains(out, "未标定") {
		t.Fatalf("--rounds > 1 不带 --go 不该印预算表（既不印计划也不跑），"+
			"实际输出里出现了预算表才会有的“未标定”:\n%s", out)
	}

	started, _ := roundBreadcrumbs(t, svc, convID)
	if len(started) != 0 {
		t.Fatalf("--rounds > 1 不带 --go 不该起任何一轮，实际有 %d 条 round_started 面包屑；完整输出:\n%s",
			len(started), out)
	}
	if cs.maxSeqCalls != 0 {
		t.Errorf("--rounds > 1 不带 --go 应当在拿到 N 与 --go 状态之后立刻拒绝、"+
			"连 TakeRoundSnap 都不做，Store.MaxSeq 调用次数应为 0，实际 %d；完整输出:\n%s",
			cs.maxSeqCalls, out)
	}
}

// ============================================================
// 5 / 6. --with-judge
// ============================================================

// TestWithJudgeRunsOneExtraRoundWithOwnID（验收 11）挡的坏实现：
// --with-judge 根本没有另起一轮（裁判混进发言轮，或者干脆被忽略）。
//
// 显式给 "ag_alpha ag_zeta"：ag_zeta 在 newTestREPLConfig 里 Mode 是
// config.ModeJudge，PlanRound（internal/turn/plan.go，现查确认）按
// candidates 的 Mode 把它分进 Judges、ag_alpha 分进 Speakers，不需要
// 另配一套 config。ag_zeta 的 Permissions 是 config.PermissionFull，
// TranslatePermission 对 PermissionFull 返回 ErrTierNotOffered
// （internal/turn/permission.go 现查确认），budgetOne 里这一步出错时
// 直接 return row, nil（不返回 error，NoDelta 保持零值 false），
// 所以 ag_zeta 不会被 skipNoDelta 误判成"没有新内容"而被踢出裁判轮——
// 它会正常走到 RunRoundPrepared，写下自己的 round_started/round_finished。
//
// 断言库里有 2 对面包屑、round_id 不同（发言轮与裁判轮各自的
// round_id）、MaxSeq 恰好调 2 次（TakeRoundSnap 各拍一次）。
//
// 结算行的断言方式（2026-09-22 改）：原先这里有个 settlementMarker
// 辅助函数，现场调一次 renderSettlement(turn.Settle(nil), 0, nil, "发言轮")，
// 拿样例输出的第一行当"这是一条结算行"的稳定特征，再用 strings.Count
// 在屏幕输出里数它出现几次。这个办法的前提是"第一行与调用方传的参数
// 无关"——这个前提已经不成立：round.go 现查确认 renderSettlement 把
// label 参数直接印在第一行（`fmt.Fprintf(&b, "%s结算\n", label)`），
// 生产代码（round.go 166/198 行）分别传 "发言轮" 与 "裁判轮" 两个不同的
// label。取样例时传的是 "发言轮"，marker 因此永远等于"发言轮结算"，在
// --with-judge 场景下只能匹配到发言轮那一段，数出 1 次而不是期望的
// 2 次——不是实现漏渲染了裁判轮的结算行（两条都完整印了，见任务里的
// 真实屏幕输出），是这套取样手段已经不能代表"结算行"这个类别了，只能
// 代表"发言轮的结算行"。旧的 settlementMarker/splitOnMarker 两个辅助函数
// 除了这条用例没有别的调用方（2026-09-22 现查 grep 整个 internal/repl），
// 随这次改动一并删除。
//
// 新断言改成直接按标签逐个点名，不再依赖第一行是什么：
// (1) 屏幕上必须恰好出现 1 次"发言轮结算"；
// (2) 屏幕上必须恰好出现 1 次"裁判轮结算"；
// (3) "发言轮结算"必须出现在"裁判轮结算"之前（先发言后裁判，是
// docs/12_STEP9_CONTRACT.md §4.6a.7 的裁决）。
// (1)(2) 合起来等价于旧断言"总共出现 2 次"，但逐个点名还额外挡住了旧
// 断言数不出的退化——比如"发言轮结算印了两次、裁判轮结算一次都没印"，
// 总数同样是 2，旧断言会放过，新断言会在 (2) 处炸；(3) 挡的是"两条标签
// 都印了但顺序反了"。
func TestWithJudgeRunsOneExtraRoundWithOwnID(t *testing.T) {
	svc, cs, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha ag_zeta --with-judge --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	started, finished := roundBreadcrumbs(t, svc, convID)
	assertBreadcrumbPairs(t, started, finished, 2, "--with-judge")

	if cs.maxSeqCalls != 2 {
		t.Errorf("Store.MaxSeq 调用次数=%d，期望恰好 2（发言轮、裁判轮各自 TakeRoundSnap 一次）；完整输出:\n%s",
			cs.maxSeqCalls, out)
	}

	const speakLabel = "发言轮结算"
	const judgeLabel = "裁判轮结算"
	if n := strings.Count(out, speakLabel); n != 1 {
		t.Fatalf("屏幕上应当恰好出现 1 次 %q，实际 %d 次；完整输出:\n%s", speakLabel, n, out)
	}
	if n := strings.Count(out, judgeLabel); n != 1 {
		t.Fatalf("屏幕上应当恰好出现 1 次 %q，实际 %d 次；完整输出:\n%s", judgeLabel, n, out)
	}
	speakIdx := strings.Index(out, speakLabel)
	judgeIdx := strings.Index(out, judgeLabel)
	if speakIdx >= judgeIdx {
		t.Fatalf("%q 应当出现在 %q 之前（先发言后裁判，契约 §4.6a.7 的裁决），"+
			"实际前者位于第 %d 字节、后者位于第 %d 字节；完整输出:\n%s",
			speakLabel, judgeLabel, speakIdx, judgeIdx, out)
	}
}

// TestWithJudgeRunsOnlyOnceAcrossMultipleRounds（验收 11b）挡的坏实现：
// 「每个发言轮后面跟一次裁判」——那种实现在 --rounds 2 --with-judge 下
// 会产生 2（发言）+2（裁判）=4 对面包屑；正确实现是全部发言轮跑完之后
// 只单独跑一次裁判轮：2+1=3 对。
func TestWithJudgeRunsOnlyOnceAcrossMultipleRounds(t *testing.T) {
	svc, cs, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha ag_zeta --rounds 2 --with-judge --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	started, finished := roundBreadcrumbs(t, svc, convID)
	assertBreadcrumbPairs(t, started, finished, 3, "--rounds 2 --with-judge")

	if cs.maxSeqCalls != 3 {
		t.Errorf("Store.MaxSeq 调用次数=%d，期望恰好 3（2 个发言轮 + 1 个裁判轮各自 TakeRoundSnap 一次，"+
			"不是每个发言轮后面都跟一次裁判）；完整输出:\n%s", cs.maxSeqCalls, out)
	}
}

// ============================================================
// 7. 全员 NoDelta：整轮跳过，事实源里不该出现"什么都没发生的轮"
// ============================================================

// noDeltaAgentID 是本组用例专用的成员：provider 必须是 internal/provider
// 全局 registry 里真正登记过的 id（claude/grok/agy 三选一），BudgetWith
// 才会走过 provider.Lookup 那一步、继续往下算到"这个成员本轮有没有新
// 内容"。本文件其它用例用的 pv_alpha/pv_mid/pv_zeta 在 registry 里查不到，
// budgetOne 会在 provider.Lookup 那一步就提前 return 一个 NoDelta 恒为
// false 的空行（internal/turn/budget.go 现查确认），没法用来构造"全员
// NoDelta"这个场景——这正是为什么本组用例需要单独一份配置，而不是复用
// newCountingRoundFixture。
const noDeltaAgentID = "ag_probe"

// newNoDeltaConfig 造一份只含 noDeltaAgentID 一个成员的配置，provider 用
// 真实登记过的 "agy"，Command 指向 t.TempDir() 下一个必然不存在的绝对
// 路径。
//
// 安全考虑（2026-09-22 现查）：本机确实装了 agy/grok/claude 三家真实
// CLI，如果 Command 给一个裸名字（比如 "agy"），一旦本组用例的场景没
// 构造成功、代码真的走到了 spawn 那一步，会经 PATH 解析到本机真实安装
// 的 agy CLI、实际起一个子进程——这是绝对要避免的副作用。给一个
// t.TempDir() 下的绝对路径，即使真的走到 spawn，也只会在起子进程那一步
// 干净地报"文件不存在"，不会命中任何真实可执行文件。
//
// budgetOne 判定 NoDelta 的分支（"if len(selected) == 0 { row.NoDelta =
// true ... }"）在 argvHeadroom（会调 exec.LookPath）之前就返回了，所以
// NoDelta==true 的路径本身根本不会碰这个 Command；只有控制组（有新内容、
// NoDelta==false）会经过 argvHeadroom 与之后的真实 spawn 尝试，两处都已
// 用不存在的绝对路径挡住。
func newNoDeltaConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Providers: make(map[string]config.ProviderConfig),
		Agents:    make(map[string]config.AgentConfig),
		Teams:     make(map[string]config.TeamConfig),
	}
	missing := filepath.Join(t.TempDir(), "missing-agy-binary.exe")
	cfg.Providers["agy"] = config.ProviderConfig{Command: missing}
	cfg.Agents[noDeltaAgentID] = config.AgentConfig{
		DisplayName: "Probe",
		Provider:    "agy",
		Model:       "probe-model",
		Effort:      "",
		Role:        "probe role",
		Permissions: config.PermissionRead,
		Mode:        config.ModeWorker,
	}
	return cfg
}

// newNoDeltaFixture 用 newNoDeltaConfig 建一个专属的 *turn.Runner。
// cmdRound 只经 snap.Cfg（来自 r.rn 自己在构造时捕获的 cfg）读 agents
// 配置，不读 REPL.New 的 cfg 参数——runREPLRunner 内部固定传
// newTestREPLConfig()，与这里传给 turn.NewRunner 的自定义 cfg 互不干扰，
// 不需要、也没办法通过 runREPLRunner 换掉 REPL 自己的 cfg。
func newNoDeltaFixture(t *testing.T) (*conversation.Service, *countingMaxSeqStore, *turn.Runner, string) {
	t.Helper()
	inner := newFakeStore()
	cs := &countingMaxSeqStore{fakeStore: inner}
	svc := conversation.NewService(cs)
	cfg := newNoDeltaConfig(t)
	rn := turn.NewRunner(cfg, svc, t.TempDir(), turn.RulesBlocks{})

	const convID = "conv_nodelta0000000000000000000000"
	inner.seedConversation(convID, "全员 NoDelta 用例")
	inner.seedParticipant(t, convID, noDeltaAgentID)
	return svc, cs, rn, convID
}

// TestAllNoDeltaSkipsRoundEntirely（验收 15）挡的坏实现：本轮全员
// NoDelta 被挪进 Skipped 之后，plan.Speakers 空了，仍然无条件调了
// RunRoundPrepared。2026-09-22 现查 internal/turn/scheduler.go 的
// RunRoundWith：len(plan.Speakers)==0 时它仍然会铸一个 roundID、写下
// 一条 speakers="" 的 round_started、wg.Wait() 立刻返回、再写一条
// round_finished——也就是说，今天这套未打补丁的 cmdRound 会为"全员
// NoDelta"的一轮真的写下一对"什么都没发生"的面包屑。这正是本条用例要
// 挡的坏实现，也是为什么这条用例现在应当是红的：不是因为构造得不对，
// 是因为这个保护今天确实还没加。
func TestAllNoDeltaSkipsRoundEntirely(t *testing.T) {
	t.Run("全员NoDelta时整轮跳过", func(t *testing.T) {
		svc, cs, rn, convID := newNoDeltaFixture(t)

		// 不 post 任何消息：noDeltaAgentID 从没被问过（bootstrap==true），
		// 但 selectBootstrap 只从入参的历史消息切片里选
		// （internal/turn/router.go 现查确认，纯函数，不额外读任何隐藏
		// 状态），历史是空切片，选出来的东西也必然是空切片，
		// budgetOne 因此会判它 NoDelta=true。这是本机不需要真的先跑完
		// 一轮就能构造出 NoDelta=true 的最简办法。
		out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round "+noDeltaAgentID+" --go\n")
		if err != nil {
			t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
		}

		started, _ := roundBreadcrumbs(t, svc, convID)
		if len(started) != 0 {
			t.Fatalf("全员 NoDelta 时不该有任何 round_started 面包屑（事实源里不该出现"+
				"\"什么都没发生的轮\"），实际 %d 条；完整输出:\n%s", len(started), out)
		}
		if cs.maxSeqCalls != 1 {
			t.Errorf("Store.MaxSeq 调用次数=%d，期望恰好 1：TakeRoundSnap 在判定出"+
				"\"全员 NoDelta\"之前已经拍过一次快照，这次快照仍然要发生，"+
				"只是发生之后不该再调 RunRoundPrepared；完整输出:\n%s", cs.maxSeqCalls, out)
		}
		// 契约原句「印一句说清没人可问就返回」——具体措辞不在冻结契约的
		// 断言面内，这里只用几个语义上大概率会出现的候选词做一个软断言
		// （t.Errorf 不是 t.Fatalf，猜错候选词不该盖住上面两条硬断言）。
		// 如果实现选的措辞不含这几个词，这一条会误报，请以上面两条结构性
		// 断言（无面包屑、MaxSeq==1）为准；这一点已经写进最终报告。
		if !strings.Contains(out, "没人") && !strings.Contains(out, "无人") &&
			!strings.Contains(out, "没有人") && !strings.Contains(out, "无新内容") {
			t.Errorf("全员 NoDelta 时屏幕上应当说清楚没人可问，"+
				"实际输出里找不到候选词（没人/无人/没有人/无新内容）之一，"+
				"实际输出:\n%s", out)
		}
	})

	t.Run("对照组_有新内容时同一夹具会写下面包屑", func(t *testing.T) {
		svc, _, rn, convID := newNoDeltaFixture(t)

		// 先 /say 塞一条用户消息：noDeltaAgentID 的 selectBootstrap 能选到
		// 至少一条 KindUser 消息，NoDelta 不再成立，证明"上面那半条测不出
		// 东西的可能性"（比如这套夹具本身就走不到 RunRoundPrepared，
		// 不管有没有 NoDelta 都不会写面包屑）被排除。真起子进程时
		// Command 指向的是一个不存在的绝对路径，会在 spawn 那一步干净地
		// 失败，但 round_started 在 spawn 之前已经写下
		// （RunRoundWith 的先后顺序，本文件顶部与 newNoDeltaConfig 的
		// 注释都已现查确认），不影响这条对照组要证明的事。
		out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/say 有新内容\n/round "+noDeltaAgentID+" --go\n")
		if err != nil {
			t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
		}
		started, _ := roundBreadcrumbs(t, svc, convID)
		if len(started) == 0 {
			t.Fatalf("对照组：有新内容时应当至少写下一条 round_started 面包屑，"+
				"实际 0 条，说明这套夹具本身观测不出\"起没起过这一轮\"；完整输出:\n%s", out)
		}
	})
}

// ============================================================
// 本文件以下是第二组测试（片 3d 第二刀开工前订正契约那六项新增行为）。
// 背对背：写这一段时实现方正在并行改 internal/repl/round.go 落地这六项，
// 本段没有读那份进行中的改动来猜断言——断言全部来自冻结契约本身、以及
// 现查到的既有代码（internal/turn/plan.go 的 PlanRound/SkipReason、
// internal/turn/budget.go 的 budgetOne/forecastSession、
// internal/turn/router.go 的 selectBootstrap/recipientOK、
// internal/turn/scheduler.go 的 MetaSpeakers、internal/repl/commands.go
// 的 /to 定向发言）。写这段时 renderSkipped 这个符号还不存在，本段大多数
// 用例当时预期会编译不过或跑红；实测时实现方已经并行落地了这六项里的
// 五项（renderSkipped、forecastCold 口径、顺序说明、阻塞提示、k==0 省略
// 「另有」行），只有"点名 ModeJudge 成员但不带 --with-judge 时经
// renderSkipped 交代清楚"这一个子场景还没接上（TestRoundWithJudge...
// 那两条用例因此仍是红的）。哪几条红、为什么红，写在最终报告里。
// ============================================================

// ============================================================
// A. renderSkipped 纯函数用例
// ============================================================

// TestRenderSkippedThreeKindsProduceDistinguishableLines 挡的坏实现：
// renderSkipped 只印 AgentID + Reason，压根不看 Kind——三档 Reason 如果
// 由调用方分别写成不同的话，这种坏实现也能造出"三行不同"的假象，测不出
// 问题。
//
// 判别力设计：三条 SkipReason 故意给同一个 Reason 字符串，只让 AgentID
// 与 Kind 不同；渲染完之后把每一行里那个 AgentID 子串抠掉再比较——这时
// 唯一还可能让三行不同的就只剩 Kind 有没有被渲染出来了。如果实现方确实
// 按 Kind 分档（哪怕只是原样印出 Kind 的字面量 not_in_config /
// not_on_roster / mode），抠掉 AgentID 之后三行仍然两两不同，用例通过；
// 如果 Kind 被无视，抠掉 AgentID 之后三行会逐字节相同，用例失败。
func TestRenderSkippedThreeKindsProduceDistinguishableLines(t *testing.T) {
	const sameReason = "占位理由，三档故意给成同一句"
	skipped := []turn.SkipReason{
		{AgentID: "ag-notcfg", Kind: turn.SkipNotInConfig, Reason: sameReason},
		{AgentID: "ag-notroster", Kind: turn.SkipNotOnRoster, Reason: sameReason},
		{AgentID: "ag-judge", Kind: turn.SkipMode, Reason: sameReason},
	}

	out := renderSkipped(skipped)

	ids := []string{"ag-notcfg", "ag-notroster", "ag-judge"}
	for _, id := range ids {
		if !strings.Contains(out, id) {
			t.Fatalf("renderSkipped 输出里应当出现 %s，实际输出：\n%s", id, out)
		}
	}

	lineNotCfg := strings.ReplaceAll(agentRowLines(out, "ag-notcfg"), "ag-notcfg", "")
	lineNotRoster := strings.ReplaceAll(agentRowLines(out, "ag-notroster"), "ag-notroster", "")
	lineJudge := strings.ReplaceAll(agentRowLines(out, "ag-judge"), "ag-judge", "")

	if lineNotCfg == "" || lineNotRoster == "" || lineJudge == "" {
		t.Fatalf("三档各自的行不应为空，实际：notcfg=%q notroster=%q judge=%q", lineNotCfg, lineNotRoster, lineJudge)
	}
	if lineNotCfg == lineNotRoster || lineNotCfg == lineJudge || lineNotRoster == lineJudge {
		t.Fatalf("三档 Kind 抠掉各自 AgentID 之后应当两两不同（挡住\"只印 AgentID+Reason、不看 Kind\"的坏实现），"+
			"实际有相同的：\nnotcfg=%q\nnotroster=%q\njudge=%q\n完整输出：\n%s",
			lineNotCfg, lineNotRoster, lineJudge, out)
	}

	// mode 那一档要不要额外断言"提到裁判/--with-judge"：这里的 Reason 是
	// 本用例自己传进去的字面量，就算断言含"占位理由"里的字也测不出实现方
	// 有没有把 --with-judge 讲清楚——那是循环论证，纯函数层面写不出有判别
	// 力的断言。真正端到端检验这件事的是下面 B 组的命令层用例
	// TestRoundWithJudgeIDWithoutFlagIsSkippedWithModeNote，那里 Reason 是
	// 实现方自己写的，不是我预先塞好的词。这一点已经写进最终报告。
}

// TestRenderSkippedEmptyReturnsEmptyString 挡的坏实现：空切片被印成表头
// 或者"没有人被跳过"这类占位句，而不是契约要求的空串。
func TestRenderSkippedEmptyReturnsEmptyString(t *testing.T) {
	if out := renderSkipped(nil); out != "" {
		t.Fatalf("renderSkipped(nil) 应当返回空串，实际 %q", out)
	}
	if out := renderSkipped([]turn.SkipReason{}); out != "" {
		t.Fatalf("renderSkipped([]turn.SkipReason{}) 应当返回空串，实际 %q", out)
	}
}

// ============================================================
// B. 点名裁判但不带 --with-judge（验收 13）
// ============================================================

// TestRoundWithJudgeIDWithoutFlagIsSkippedWithModeNote 挡的坏实现：点名
// 了一个 ModeJudge 成员、没带 --with-judge，他既不在预算表上、也不被
// 问、屏幕一个字不提（今天就是这样）。
//
// 用 "/round ag_alpha ag_zeta"（不带 --with-judge、不带 --go）：ag_alpha
// 是普通发言者（Mode=ModeDiscussion），同一份 requested 里带着它，
// PlanRound 分出的 Speakers 非空，不会撞上"名单为空"那条既有拒绝分支，
// 能把断言落在"这条路径正常走到 renderSkipped"上，而不是落在一个不确定
// 会不会先被拒绝掉的分支里。不带 --go：契约 §4.6 只说阻塞提示在起进程
// 之前，没说预算表/跳过说明必须等 --go 才印；不带 --go 更能确定走的是
// "预览"这一段，不会因为 --go 触发的真实扇出把断言复杂化。
func TestRoundWithJudgeIDWithoutFlagIsSkippedWithModeNote(t *testing.T) {
	svc, _, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha ag_zeta\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	// 对照组：ag_alpha 是普通发言者，应当在输出里可见——证明"出现"这件事
	// 本身在这套夹具/这条命令下是可观测的，不是因为整段输出都是空的或者
	// 被吞掉了。
	if !strings.Contains(out, "ag_alpha") {
		t.Fatalf("对照组失败：普通发言者 ag_alpha 应当出现在输出里，实际输出:\n%s", out)
	}

	// 硬断言：点名的裁判 ag_zeta 必须在屏幕上出现。
	if !strings.Contains(out, "ag_zeta") {
		t.Fatalf("坏实现挡不住：点名的裁判 ag_zeta 应当在屏幕上出现（renderSkipped 那一行），"+
			"实际输出里找不到 ag_zeta，说明它被悄悄吞掉了；完整输出:\n%s", out)
	}

	// 软断言：ag_zeta 那一行应当说清楚是因为 mode（没带 --with-judge）。
	// 候选词覆盖几种实现方可能选的措辞，限定在 ag_zeta 自己那一行，避免
	// 命中输出里别处无关的文字；命中哪个不在契约断言面内。
	zetaLine := agentRowLines(out, "ag_zeta")
	modeWords := []string{"裁判", "judge", "Judge", "--with-judge"}
	hit := false
	for _, w := range modeWords {
		if strings.Contains(zetaLine, w) {
			hit = true
			break
		}
	}
	if !hit {
		t.Errorf("软断言：ag_zeta 那一行应当出现与\"裁判 / --with-judge\"相关的说明，"+
			"候选词 %v 一个都没命中，可能是实现方选了别的措辞，以上面两条硬断言为准；ag_zeta 那一行:\n%s",
			modeWords, zetaLine)
	}
}

// TestRoundWithJudgeSkipNotePrintedOncePerCommand 挡的坏实现：ag_zeta 的
// 跳过说明随 --rounds 增多被重复打印（renderSkipped 契约要求"在 cmdRound
// 里 PlanRound 之后只印一次，不是每轮印"）。
//
// 残留风险（写进报告）：这里用 ag_zeta 在整段输出里的出现总次数当代理，
// 如果实现方在别处（比如某种汇总文字）又额外点了一次 ag_zeta 的名，会把
// 这条用例的次数顶到 >1 造成误报；没有更精确的观测手段（面包屑只记实际
// 扇出的 Speakers，不记被跳过的 Judges），只能用软断言。
func TestRoundWithJudgeSkipNotePrintedOncePerCommand(t *testing.T) {
	svc, _, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha ag_zeta --rounds 2 --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	n := strings.Count(out, "ag_zeta")
	if n == 0 {
		t.Fatalf("ag_zeta 应当至少出现一次，实际 0 次；完整输出:\n%s", out)
	}
	if n != 1 {
		t.Errorf("软断言：ag_zeta 的跳过说明应当只印一次（不随 --rounds 增多重复打印），本用例 --rounds 2，"+
			"实际 ag_zeta 在屏幕上出现了 %d 次；完整输出:\n%s", n, out)
	}
}

// ============================================================
// C. forecastCold 口径：只数留在这一轮里的（验收 14）
// ============================================================

// coldRunAgentID / coldSkipAgentID 是本组用例专用的两个成员，provider 用
// 真实登记过的 "agy"（否则 budgetOne 会在 provider.Lookup 那一步提前
// 返回、NoDelta 恒为 false，构造不出 NoDelta=true 的场景，道理与
// noDeltaAgentID 那组注释相同）。
const (
	coldRunAgentID  = "ag_cold_run"
	coldSkipAgentID = "ag_cold_skip"
)

// newColdMixConfig 造一份含两个成员的配置，Command 同样指向 t.TempDir()
// 下必然不存在的绝对路径，安全考虑与 newNoDeltaConfig 相同：不给裸名字，
// 避免真的解析到本机安装的 agy CLI。
func newColdMixConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Providers: make(map[string]config.ProviderConfig),
		Agents:    make(map[string]config.AgentConfig),
		Teams:     make(map[string]config.TeamConfig),
	}
	missing := filepath.Join(t.TempDir(), "missing-agy-binary-coldmix.exe")
	cfg.Providers["agy"] = config.ProviderConfig{Command: missing}
	mk := func(display string) config.AgentConfig {
		return config.AgentConfig{
			DisplayName: display,
			Provider:    "agy",
			Model:       "probe-model",
			Effort:      "",
			Role:        "probe role",
			Permissions: config.PermissionRead,
			Mode:        config.ModeWorker,
		}
	}
	cfg.Agents[coldRunAgentID] = mk("ColdRun")
	cfg.Agents[coldSkipAgentID] = mk("ColdSkip")
	return cfg
}

// newColdMixFixture 造一个只含 coldRunAgentID / coldSkipAgentID 两个成员
// 的专属 *turn.Runner，用法与 newNoDeltaFixture 相同。
func newColdMixFixture(t *testing.T) (*conversation.Service, *countingMaxSeqStore, *turn.Runner, string) {
	t.Helper()
	inner := newFakeStore()
	cs := &countingMaxSeqStore{fakeStore: inner}
	svc := conversation.NewService(cs)
	cfg := newColdMixConfig(t)
	rn := turn.NewRunner(cfg, svc, t.TempDir(), turn.RulesBlocks{})

	const convID = "conv_coldmix00000000000000000000000"
	inner.seedConversation(convID, "forecastCold 口径用例")
	inner.seedParticipant(t, convID, coldRunAgentID)
	inner.seedParticipant(t, convID, coldSkipAgentID)
	return svc, cs, rn, convID
}

// forecastColdPattern 抓结算行里"冷启动 预报 <数字>"这一段。格式字符串由
// 任务书直接给出（"冷启动 预报 %d  实测 %s"），这里不写死具体空格数，用
// \s* 容错空格数量上的出入，只锁"冷启动"与"预报"这两个词加中间那个数字
// 的相对顺序。
var forecastColdPattern = regexp.MustCompile(`冷启动\s*预报\s*([0-9]+)`)

// TestForecastColdOnlyCountsMembersStayingInRound（验收 14）是本组主
// 断言。挡的坏实现：结算行的"预报冷启动"把随后被挪去 Skipped、根本不起
// 进程的 NoDelta 成员也数了进去。
//
// 场景 A（本用例真正要测的）：/to 定向只给 coldRunAgentID 发一条消息，
// coldSkipAgentID 从没被问过也没收到广播，selectBootstrap 对它选出空
// 切片（recipientOK 排掉了它，internal/turn/router.go 现查确认），
// budgetOne 因此判它 NoDelta=true；且它是第一次开口，forecastSession 的
// "没找到会话"分支恒给 cold=true（internal/turn/budget.go 现查确认），
// 所以它同时满足 Cold=true 且 NoDelta=true——正是契约要求 forecastCold
// 不该数进去的那种成员。coldRunAgentID 收到了定向消息，NoDelta=false，
// 留在本轮里；它也是第一次开口，同样 Cold=true。
//
// 正确实现：forecastCold 只数 Cold && !NoDelta 的，本场景 = 1。
// 坏实现（现状）：不看 NoDelta，两个 Cold=true 的成员都数，= 2。
//
// 场景 B（自证对照组）：把 /to 换成广播的 /say，两个成员都收到消息、都
// NoDelta=false、都留在本轮、都 Cold=true，这时 forecastCold 应当 = 2。
// 对照组证明两件事：(1) 这套夹具真能数出 2 个冷启动成员，不是夹具本身
// 只会数 1；(2) 场景 A 里的 /to 确实把 coldSkipAgentID 隔离掉了，场景 A
// 的 1 不是巧合。
func TestForecastColdOnlyCountsMembersStayingInRound(t *testing.T) {
	svcA, _, rnA, convIDA := newColdMixFixture(t)
	outA, err := runREPLRunner(t, svcA, rnA,
		"/use "+convIDA+"\n/to "+coldRunAgentID+" 有新内容\n/round "+coldRunAgentID+" "+coldSkipAgentID+" --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, outA)
	}
	// 锚到「发言轮结算」再在其后找「冷启动 预报」：不锚定的话，若预算表
	// 里也有"冷启动"这个列名/"预报"这个字眼，FindStringSubmatch 可能在
	// 整段输出里命中错误的位置，不是结算行那个数字。
	idxA := strings.Index(outA, "发言轮结算")
	if idxA < 0 {
		t.Fatalf("找不到结算行锚点「发言轮结算」，无法定位「冷启动 预报」；实际输出:\n%s", outA)
	}
	mA := forecastColdPattern.FindStringSubmatch(outA[idxA:])
	if mA == nil {
		t.Fatalf("结算行里找不到「冷启动 预报 <数字>」，实际输出:\n%s", outA)
	}
	if mA[1] != "1" {
		t.Fatalf("forecastCold 应当只数 Cold && !NoDelta 的成员：本场景只有 %s 满足（%s 因 NoDelta "+
			"被挪出本轮），期望预报冷启动=1，实际=%s；完整输出:\n%s", coldRunAgentID, coldSkipAgentID, mA[1], outA)
	}

	svcB, _, rnB, convIDB := newColdMixFixture(t)
	outB, err := runREPLRunner(t, svcB, rnB,
		"/use "+convIDB+"\n/say 有新内容\n/round "+coldRunAgentID+" "+coldSkipAgentID+" --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, outB)
	}
	idxB := strings.Index(outB, "发言轮结算")
	if idxB < 0 {
		t.Fatalf("对照组：找不到结算行锚点「发言轮结算」，无法定位「冷启动 预报」；实际输出:\n%s", outB)
	}
	mB := forecastColdPattern.FindStringSubmatch(outB[idxB:])
	if mB == nil {
		t.Fatalf("对照组：结算行里找不到「冷启动 预报 <数字>」，实际输出:\n%s", outB)
	}
	if mB[1] != "2" {
		t.Fatalf("对照组：广播消息后两个成员都应留在本轮且都 Cold=true，期望预报冷启动=2，实际=%s；"+
			"说明这套夹具本身数不出 2，场景 A 的 1 可能只是巧合，不能当成 forecastCold 已经按 NoDelta "+
			"过滤的证据；完整输出:\n%s", mB[1], outB)
	}
}

// ============================================================
// D. 重复 id 只问一次 + 顺序说明（验收 16）
// ============================================================

// TestRoundDuplicateIDSpeakersDedupedInBreadcrumb 挡的坏实现：cmdRound
// 没有真的把 PlanRound 去重之后的 Speakers 交给 RunRoundWith，比如按
// 用户敲的原始 argv 长度另开了一份名单，让同一个 id 在实际扇出的成员表
// 里出现两次。
//
// 观测手段：不猜屏幕排版，直接查 round_started 面包屑的
// Meta[turn.MetaSpeakers]（internal/turn/scheduler.go 现查：类型是
// []string，注释写的是"本轮实际扇出的成员名单"，写入前就是
// append([]string(nil), plan.Speakers...)，是最贴近"这一轮到底扇给谁"的
// 事实源，不经过任何屏幕排版）。fakeStore.AppendMessage 不做序列化，
// Meta 原样存进内存（conversation_test.go 现查确认），类型断言成
// []string 是安全的。
//
// PlanRound 本身已经去重（internal/turn/plan.go 的 seenCand），所以这条
// 用例的"只出现一次"在今天大概率已经是绿的——它是一条结构性回归闸，不是
// 这次要新加的六项之一；真正新的是下面
// TestRoundOrderNotePrintedOnceNotPerRound 里的顺序说明。
func TestRoundDuplicateIDSpeakersDedupedInBreadcrumb(t *testing.T) {
	svc, _, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha ag_alpha --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	started, finished := roundBreadcrumbs(t, svc, convID)
	assertBreadcrumbPairs(t, started, finished, 1, "重复 id 去重")

	speakersRaw, ok := started[0].Meta[turn.MetaSpeakers]
	if !ok {
		t.Fatalf("round_started 面包屑缺 Meta[%q]: %+v", turn.MetaSpeakers, started[0])
	}
	// 不对 Meta[speakers] 的动态类型打赌（与本文件 breadcrumbRoundID 用
	// fmt.Sprint 处理 Meta[round_id] 是同一条纪律）：postSystem 经
	// Service.Post 把 map[string]any 原样交给 tx.AppendMessage，本文件的
	// fakeStore 不做序列化，今天应当仍是 []string；但不必在这里断言具体
	// 动态类型，用 fmt.Sprint 转成文字再数子串出现次数，不管它是 []string
	// 还是别的切片类型都数得对。
	count := strings.Count(fmt.Sprint(speakersRaw), "ag_alpha")
	if count != 1 {
		t.Fatalf("round_started 面包屑里 Meta[speakers]=%v，ag_alpha 应当恰好出现 1 次（用户敲了两遍，"+
			"PlanRound 去重之后应当只留一个），实际 %d 次；完整输出:\n%s", speakersRaw, count, out)
	}
}

// TestRoundOrderNotePrintedOnceNotPerRound（验收 16）挡的坏实现：发言
// 顺序（字典序 + 去重）的说明要么根本没印，要么在 --rounds > 1 时被
// PlanRound 每轮调用一次就跟着印一次，变成"每轮印"而不是契约要求的
// "在 cmdRound 里只印一次"。
//
// 软断言：不知道实现方选的具体措辞，候选词覆盖"字典序""去重""顺序"几种
// 可能，命中哪个就用哪个继续判次数；一个都没命中时只报错不中止
// （t.Errorf），避免猜错候选词盖住其它信息。
func TestRoundOrderNotePrintedOnceNotPerRound(t *testing.T) {
	svc, _, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha ag_alpha --rounds 2 --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	orderWords := []string{"字典序", "去重", "顺序"}
	hit := ""
	for _, w := range orderWords {
		if strings.Contains(out, w) {
			hit = w
			break
		}
	}
	if hit == "" {
		t.Errorf("软断言：屏幕上应当有一句关于发言顺序（字典序 / 去重）的说明，候选词 %v 一个都没命中，"+
			"可能是实现方选了别的措辞；实际输出:\n%s", orderWords, out)
		return
	}
	if n := strings.Count(out, hit); n != 1 {
		t.Errorf("契约要求顺序说明「在 cmdRound 里只印一次」（不是每轮印），命中候选词 %q 出现了 %d 次"+
			"（本用例 --rounds 2，如果这个数是 2 就说明被每轮打印了一遍）；实际输出:\n%s", hit, n, out)
	}
}

// ============================================================
// E. --go 起进程之前的阻塞提示（验收 17）
// ============================================================

// TestRoundPrintsBlockingHintBeforeSpawning 挡的坏实现：阻塞提示没印，
// 或者印在了结算之后（用户看到结果才被告知"扇出期间会阻塞"，为时已晚，
// 提示的意义就没了）。
//
// 候选词覆盖契约 §4.6 原句"REPL 在扇出期间阻塞，约 1 x 5 分钟（并发，
// 最慢的那个成员说了算）"里的几个关键词，具体措辞不锁，命中一个即可；
// 位置断言才是本用例真正的判别力——只断言"出现了"挡不住"印在结算之后"
// 这种坏实现，必须与"发言轮结算"这个既有锚点（TestWithJudgeRuns...
// 那组用例已经确认过的稳定标签）比较字节位置。
func TestRoundPrintsBlockingHintBeforeSpawning(t *testing.T) {
	svc, _, rn, convID := newCountingRoundFixture(t)

	out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/round ag_alpha --go\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；完整输出:\n%s", err, out)
	}

	hintWords := []string{"扇出", "阻塞", "5 分钟", "5分钟", "不接受"}
	hintIdx := -1
	var hintWord string
	for _, w := range hintWords {
		idx := strings.Index(out, w)
		if idx < 0 {
			continue
		}
		if hintIdx == -1 || idx < hintIdx {
			hintIdx = idx
			hintWord = w
		}
	}
	if hintIdx < 0 {
		t.Fatalf("软断言：屏幕上应当有一句起进程前的阻塞提示，候选词 %v 一个都没命中，可能是实现方选了"+
			"别的措辞；实际输出:\n%s", hintWords, out)
	}

	settleIdx := strings.Index(out, "发言轮结算")
	if settleIdx < 0 {
		t.Fatalf("找不到结算行锚点「发言轮结算」，无法判定阻塞提示的先后位置；实际输出:\n%s", out)
	}
	if hintIdx >= settleIdx {
		t.Fatalf("阻塞提示应当出现在第一条结算行之前（起进程之前打招呼，不是打完才说），命中候选词 %q "+
			"位于第 %d 字节，结算行「发言轮结算」位于第 %d 字节；完整输出:\n%s",
			hintWord, hintIdx, settleIdx, out)
	}
}
