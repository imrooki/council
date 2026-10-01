// 本文件将来落到 internal/turn/runner_rules_test.go。
//
// 覆盖 11a_STEP8A_API_CONTRACT.md 3.3、3.4、3.5 节：NewRunner 加参、
// SetRules、Rebuild（验收表第 6 条）、AuditDrift（验收表第 7 条）。
// 只照契约写，不看 internal/turn/render.go、session.go、spawn.go 的实现；
// runner.go 不在禁读名单内，但 Rebuild / AuditDrift 的具体实现体本身
// 没有被读过（任务书禁止的是"看实现再写测试"，不是禁止读这个文件——
// 本文件用到的 Runner 结构体字段 r.rules、r.gates（拆闸之后 gate(key).st
// 取代了旧版按会话直接存 sessionState 的那张 map）、sessionKey、sessionState
// 的存在与用法，全部学自既有的、任务书明确允许读的 *_test.go 文件
// 【fixtures_test.go、runner_session_test.go】）。
//
// 本文件里凡是需要"构造一个已知的旧指纹再看 AuditDrift 判不判得出漂移"的
// 用例，一律把 Runner 的 RulesBlocks 留空（RulesBlocks{}）。理由：契约 3.3
// 只钉死了 Ask 里怎么算指纹（renderIdentity 全文喂给 IdentityFingerprint），
// 没有对 AuditDrift 内部怎么算"当前指纹"给出同等精确的描述；而当两段规则
// 都是空串时，"身份块是否包含规则渲染"这件事对最终字符串没有任何影响
// （规则为空时 renderIdentity 根本不产出规则那两段），所以不管 AuditDrift
// 内部具体怎么实现，只要它对同一份 (agentID, ac, 空规则) 算出确定性的指纹，
// 这里用公开的 IdentityFingerprint(renderIdentity(...), ...) 算出来的"当前值"
// 就必然与它一致。这样写用例不必对 AuditDrift 的内部实现做任何假设。
package turn

import (
	"context"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/store"
)

// ---------------------------------------------------------------------------
// 共用建造函数
// ---------------------------------------------------------------------------

// newRulesTestServiceWithStore 与 fixtures_test.go 的 newTestService 同构，
// 但额外交回 *store.Store 本身——本文件的用例需要绕过 Service 直接向
// provider_sessions 写入指定的 identity_fp（模拟"库里已经有一条旧状态"），
// Service 接口没有开放这样的写法，也不该开放。
func newRulesTestServiceWithStore(t *testing.T) (*conversation.Service, *store.Store) {
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
	return conversation.NewService(st), st
}

// newRulesTestRunner 直接调新的四参数 NewRunner，不经 fixtures_test.go 的
// newTestRunner——那个辅助函数在契约落地后签名会跟着变，本文件不依赖它，
// 免得两边的改动互相牵连。
func newRulesTestRunner(t *testing.T, cfg *config.Config, svc *conversation.Service, rules RulesBlocks) *Runner {
	t.Helper()
	workDir := filepath.Join(t.TempDir(), "workdir")
	return NewRunner(cfg, svc, workDir, rules)
}

// tstSeedProviderSession 直接向 provider_sessions 写一行，绕过 Ask，模拟"库里
// 已经有一条历史状态"。
func tstSeedProviderSession(t *testing.T, st *store.Store, convID, agentID, fp string) {
	t.Helper()
	err := st.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, conversation.ProviderSession{
			ConvID:            convID,
			AgentID:           agentID,
			ProviderSessionID: "sess-" + agentID,
			CursorSeq:         0,
			HaveCostTotal:     false,
			LastTurnAt:        time.Now().UTC(),
			IdentityFP:        fp,
			Spoken:            true,
		})
	})
	if err != nil {
		t.Fatalf("tstSeedProviderSession(%s/%s) 失败: %v", convID, agentID, err)
	}
}

// tstCurrentFingerprint 直接调 identityFPFor——与 Ask/AuditDrift 唯一的指纹
// 计算入口同一份代码，不再自己重复拼 IdentityFingerprint 的调用——为
// (agentID, ac) 在给定 rules 下算出"现在应该是什么"的指纹。
//
// convID 必须与调用方后续要比对的那一行 provider_sessions 属于同一条会话：
// WORKSPACE 档的工作目录含 convID（agentWorkDir），传错会让算出来的指纹
// 与那一行的真实指纹对不上。本文件的用例全是 read 档，传什么都不影响结果，
// 但签名要求原样传，避免将来加 workspace 档用例时静默踩坑。
//
// 必须传 r：指纹第七段（PrefixArgsDigest）依赖 WorkDir，这里若另起一个
// t.TempDir() 会与 r.AuditDrift 内部用 r.workDir 算出来的指纹对不上，
// 把两个测试临时目录的差异误报成一次「命令行」漂移。
func tstCurrentFingerprint(t *testing.T, r *Runner, convID, agentID string, ac config.AgentConfig, rules RulesBlocks) string {
	t.Helper()
	p, err := provider.Lookup(ac.Provider)
	if err != nil {
		t.Fatalf("provider.Lookup(%q) 失败: %v", ac.Provider, err)
	}
	tier, err := TranslatePermission(ac.Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission(%q) 失败: %v", ac.Permissions, err)
	}
	fp, err := identityFPFor(p, agentID, ac, r.workDir, convID, tier, rules)
	if err != nil {
		t.Fatalf("identityFPFor(%q) 失败: %v", agentID, err)
	}
	return fp
}

// ---------------------------------------------------------------------------
// NewRunner / SetRules（契约 3.3）
// ---------------------------------------------------------------------------

// TestNewRunnerStoresRulesAndSetRulesReplaces 覆盖 NewRunner 加的第四个参数、
// 以及 SetRules"换掉 Runner 手里的规则"。r.rules 这个字段名是契约 3.3
// 正文里直接点出来的（"Ask 里那一行改成 ... r.rules")，本文件与 session.go
// 同包，直接读写这个字段是白盒测试，不是猜实现。
func TestNewRunnerStoresRulesAndSetRulesReplaces(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := newRulesTestServiceWithStore(t)
	initial := RulesBlocks{Global: "初始全局规则"}
	r := newRulesTestRunner(t, cfg, svc, initial)

	if r.rules != initial {
		t.Fatalf("NewRunner 之后 r.rules = %+v，期望 %+v", r.rules, initial)
	}

	updated := RulesBlocks{Global: "新的全局规则", Local: "新的局部规则"}
	r.SetRules(updated)
	if r.rules != updated {
		t.Fatalf("SetRules 之后 r.rules = %+v，期望 %+v", r.rules, updated)
	}
}

// ---------------------------------------------------------------------------
// Rebuild（契约 3.4，验收表第 6 条）
// ---------------------------------------------------------------------------

// TestRebuildDropsRowNotJustFingerprint 覆盖验收表第 6 条核心断言："五样必须
// 全丢...做法是删掉 provider_sessions 里那一整行，不是只清指纹"。
// 判别力在于：若实现只清了 identity_fp 而留着行，Service.ProviderSession
// 读回的 ok 仍然会是 true（只是字段变空）——这与"整行删掉"在 ok 这个返回值上
// 是唯二能分辨的地方，contract 原文也是拿这一点做论证依据。
func TestRebuildDropsRowNotJustFingerprint(t *testing.T) {
	cfg := newTestConfig()
	svc, st := newRulesTestServiceWithStore(t)
	convID := newConv(t, svc)

	tstSeedProviderSession(t, st, convID, testAgentClaude, "6:role-a;7:model-a;4:high;4:read;0:;0:;")

	r := newRulesTestRunner(t, cfg, svc, RulesBlocks{})
	key := sessionKey{convID: convID, agentID: testAgentClaude}
	// 本用例单 goroutine，预置时没有并发，不取 g.mu。
	r.gate(key).st = &sessionState{providerSession: "in-memory-stale-session"}

	existed, err := r.Rebuild(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Rebuild 返回错误: %v", err)
	}
	if !existed {
		t.Error("existed 应为 true：Rebuild 之前确实有一条记录")
	}

	// gate 一经创建永不从 r.gates 里移除（不变量见 sessionGate 的注释），
	// 所以「这个 key 还在不在 map 里」这条老断言在拆闸之后会恒真——
	// r.gate(key) 本身就是 get-or-create，无论 Rebuild 做没做事，这个 key
	// 必然还在。真正该问的是这条 gate 的 st 有没有被清空（置 nil）。
	// 本文件已经用 st 这个名字指 *store.Store（newRulesTestServiceWithStore
	// 的第二个返回值），这里不再另起一个 st 变量，直接内联判断避免遮蔽。
	if r.gate(key).st != nil {
		t.Error("Rebuild 之后这条 (会话, 成员) 的 gate.st 应当被清掉（置 nil），gate 本身不删")
	}

	_, ok, err := svc.ProviderSession(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("ProviderSession 返回错误: %v", err)
	}
	if ok {
		t.Error("Rebuild 之后 ProviderSession 的 ok 仍为 true：说明只清了指纹没有删行，" +
			"这与库里从没写过这一行长得一样，违反契约「删掉一整行」的硬性要求")
	}
}

// TestRebuildMissingRowIsNotError 覆盖："existed 报告本来有没有这条会话。
// 不存在不是错误：用户对一个还没说过话的成员敲 rebuild，回一句「本来就没有」
// 就够了。"
func TestRebuildMissingRowIsNotError(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := newRulesTestServiceWithStore(t)
	convID := newConv(t, svc)
	r := newRulesTestRunner(t, cfg, svc, RulesBlocks{})

	existed, err := r.Rebuild(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Rebuild 对一个从没写过的会话不该报错，实际: %v", err)
	}
	if existed {
		t.Error("existed 应为 false：这个 agent 从没在这条会话里开过口")
	}
}

// ---------------------------------------------------------------------------
// AuditDrift（契约 3.5，验收表第 7 条）
// ---------------------------------------------------------------------------

// TestAuditDriftZeroRows 覆盖"0 条"：库里没有任何 provider_sessions 行时，
// 返回空结果、无错误。
func TestAuditDriftZeroRows(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := newRulesTestServiceWithStore(t)
	r := newRulesTestRunner(t, cfg, svc, RulesBlocks{})

	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 返回错误: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("AuditDrift 返回 %d 条，期望 0 条: %+v", len(rows), rows)
	}
}

// TestAuditDriftNRowsSortedAndExcludesMatching 覆盖"N 条"与排序：跨两个会话
// 各造几条 provider_sessions，一部分指纹与"现在应该是什么"不符（漂移），
// 一部分完全相符（不该出现在结果里）。会话 id 是 store 生成的随机十六进制，
// 不能硬编码，所以期望顺序在本用例内用 sort.Slice 现算，而不是写死字面量。
func TestAuditDriftNRowsSortedAndExcludesMatching(t *testing.T) {
	cfg := newTestConfig()
	svc, st := newRulesTestServiceWithStore(t)
	r := newRulesTestRunner(t, cfg, svc, RulesBlocks{})

	convA := newConv(t, svc)
	convB := newConv(t, svc)

	// 漂移：指纹是随便铸的、与真实配置无关的字符串，无论 AuditDrift 内部
	// 具体怎么算"现在的指纹"，都不可能凑巧与这个值相等。
	bogusFP := IdentityFingerprint("SEEDED-STALE-ROLE-绝不会与任何真实配置巧合相等", "stale-model", "high", provider.PermissionRead, RulesBlocks{}, "")
	tstSeedProviderSession(t, st, convA, testAgentClaude, bogusFP)
	tstSeedProviderSession(t, st, convA, testAgentAgy, bogusFP)
	tstSeedProviderSession(t, st, convB, testAgentClaude, bogusFP)

	// 不漂移（负例）：指纹恰好等于"现在应该是什么"，必须完全不出现在结果里。
	matchingFP := tstCurrentFingerprint(t, r, convB, testAgentGrok, cfg.Agents[testAgentGrok], RulesBlocks{})
	tstSeedProviderSession(t, st, convB, testAgentGrok, matchingFP)

	beforeRows, err := st.AllProviderSessions(context.Background())
	if err != nil {
		t.Fatalf("AllProviderSessions（调用前）失败: %v", err)
	}

	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 返回错误: %v", err)
	}

	type pair struct{ convID, agentID string }
	wantPairs := []pair{
		{convA, testAgentClaude},
		{convA, testAgentAgy},
		{convB, testAgentClaude},
	}
	sort.Slice(wantPairs, func(i, j int) bool {
		if wantPairs[i].convID != wantPairs[j].convID {
			return wantPairs[i].convID < wantPairs[j].convID
		}
		return wantPairs[i].agentID < wantPairs[j].agentID
	})

	if len(rows) != len(wantPairs) {
		t.Fatalf("AuditDrift 返回 %d 条，期望 %d 条: %+v", len(rows), len(wantPairs), rows)
	}
	for i, w := range wantPairs {
		if rows[i].ConvID != w.convID || rows[i].AgentID != w.agentID {
			t.Errorf("第 %d 条 = (%q,%q)，期望 (%q,%q)（按 ConvID,AgentID 升序）",
				i, rows[i].ConvID, rows[i].AgentID, w.convID, w.agentID)
		}
		// 第 7 节措辞不属于契约面，但至少要能与"指纹格式不可解"/
		// "配置里已无此 agent"这两条特殊取值区分开——它们是契约给出的
		// 字面量（3.5 节），这里的行是"指纹解得开但不相符"这一支。
		if rows[i].Reason == "" {
			t.Errorf("第 %d 条 Reason 为空串", i)
		}
		if rows[i].Reason == "指纹格式不可解" || rows[i].Reason == "配置里已无此 agent" {
			t.Errorf("第 %d 条 Reason = %q，这一支应当是「指纹解得开」分支，不该落到另外两种特殊取值", i, rows[i].Reason)
		}
	}
	for _, row := range rows {
		if row.ConvID == convB && row.AgentID == testAgentGrok {
			t.Errorf("负例 (convB, %s) 不该出现在漂移结果里，它的指纹本来就与现在相符", testAgentGrok)
		}
	}

	// 纯读：AuditDrift 调用前后库里的行不变。
	afterRows, err := st.AllProviderSessions(context.Background())
	if err != nil {
		t.Fatalf("AllProviderSessions（调用后）失败: %v", err)
	}
	if len(afterRows) != len(beforeRows) {
		t.Fatalf("AuditDrift 调用前后 provider_sessions 行数变了：调用前 %d 行，调用后 %d 行",
			len(beforeRows), len(afterRows))
	}
	for i := range beforeRows {
		if beforeRows[i] != afterRows[i] {
			t.Errorf("第 %d 行在 AuditDrift 调用前后不同，期望纯读不改任何东西\n之前: %+v\n之后: %+v",
				i, beforeRows[i], afterRows[i])
		}
	}
}

// TestAuditDriftUnparseableFingerprintFromContract 覆盖"指纹解不开"：迁移之后理论上不该
// 再出现的四段旧指纹（或任何不合语法的字符串），Reason 必须恰好是契约给出的
// 字面量"指纹格式不可解"。
func TestAuditDriftUnparseableFingerprintFromContract(t *testing.T) {
	cfg := newTestConfig()
	svc, st := newRulesTestServiceWithStore(t)
	r := newRulesTestRunner(t, cfg, svc, RulesBlocks{})

	convID := newConv(t, svc)
	tstSeedProviderSession(t, st, convID, testAgentClaude, "6:role-a;7:model-a;4:high;4:read;") // 四段，缺规则两段

	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 返回错误: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("AuditDrift 返回 %d 条，期望 1 条: %+v", len(rows), rows)
	}
	if rows[0].ConvID != convID || rows[0].AgentID != testAgentClaude {
		t.Fatalf("返回的行是 (%q,%q)，期望 (%q,%q)", rows[0].ConvID, rows[0].AgentID, convID, testAgentClaude)
	}
	if rows[0].Reason != "指纹格式不可解" {
		t.Errorf("Reason = %q，期望恰好是契约给出的字面量 %q", rows[0].Reason, "指纹格式不可解")
	}
}

// TestAuditDriftAgentRemovedFromConfigFromContract 覆盖"配置里已经没有这个 agent"：
// 库里还留着这一行（用户删了 agent 又加回来时它会复活），Reason 必须恰好是
// 契约给出的字面量"配置里已无此 agent"。
func TestAuditDriftAgentRemovedFromConfigFromContract(t *testing.T) {
	cfg := newTestConfig()
	svc, st := newRulesTestServiceWithStore(t)
	convID := newConv(t, svc)

	const ghostAgent = "ghost-agent-already-removed"
	wellFormedFP := IdentityFingerprint("role", "model", "high", provider.PermissionRead, RulesBlocks{}, "")
	tstSeedProviderSession(t, st, convID, ghostAgent, wellFormedFP)

	if _, ok := cfg.Agents[ghostAgent]; ok {
		t.Fatalf("测试前提有误：newTestConfig() 里不该有 %q 这个 agent", ghostAgent)
	}

	r := newRulesTestRunner(t, cfg, svc, RulesBlocks{})
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 返回错误: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("AuditDrift 返回 %d 条，期望 1 条: %+v", len(rows), rows)
	}
	if rows[0].ConvID != convID || rows[0].AgentID != ghostAgent {
		t.Fatalf("返回的行是 (%q,%q)，期望 (%q,%q)", rows[0].ConvID, rows[0].AgentID, convID, ghostAgent)
	}
	if rows[0].Reason != "配置里已无此 agent" {
		t.Errorf("Reason = %q，期望恰好是契约给出的字面量 %q", rows[0].Reason, "配置里已无此 agent")
	}
}
