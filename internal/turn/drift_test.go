package turn

// drift_test.go 覆盖 11a 契约 3.5 的 AuditDrift：0 行、N 行、指纹解不开、
// agent 已从配置里删掉，外加 provider 查不到 / 权限档不放行这两种任务书
// 要求实现方自行判断处置的情况，以及返回顺序。

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// TestAuditDriftNoSessionsReturnsEmpty 覆盖 0 行：库里一条 provider 会话
// 都没有时，返回空切片而不是错误。
func TestAuditDriftNoSessionsReturnsEmpty(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 失败: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("库里没有任何 provider 会话时应当返回 0 行，实际 %d 行: %+v", len(rows), rows)
	}
}

// TestAuditDriftNoChangeReturnsEmpty 是反例：种子指纹与 Runner 手里的规则
// 用同一份规则算出来，即使规则本身非空，也不该报出漂移——AuditDrift
// 比的是「变没变」，不是「规则是不是空的」。
func TestAuditDriftNoChangeReturnsEmpty(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)

	ac := cfg.Agents[testAgentClaude]
	p, err := provider.Lookup(ac.Provider)
	if err != nil {
		t.Fatalf("provider.Lookup 失败: %v", err)
	}
	tier, err := TranslatePermission(ac.Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission 失败: %v", err)
	}
	rules := RulesBlocks{Global: "同一份全局规则", Local: "同一份局部规则"}
	// workDir 必须与下面 AuditDrift 用的 Runner 共用同一条：指纹第七段
	// （PrefixArgsDigest）依赖 WorkDir，两个各自现铸的 t.TempDir() 会让
	// AuditDrift recompute 出一个不同的「命令行」段，把「规则没变」误报成
	// 「命令行变了」——那不是本用例想验的东西。
	workDir := t.TempDir()
	fp, err := identityFPFor(p, testAgentClaude, ac, workDir, convID, tier, rules)
	if err != nil {
		t.Fatalf("identityFPFor 失败: %v", err)
	}
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "sess-1",
		IdentityFP:        fp,
	})

	r := NewRunner(cfg, svc, workDir, rules)
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 失败: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("规则与建立会话时完全相同，不该报出漂移，实际: %+v", rows)
	}
}

// TestAuditDriftDetectsRulesChange 覆盖最主要的场景：Runner 手里的规则与
// 建立会话时不同，报出恰好一行，且 Reason 点名「全局规则」——这是
// identityDriftFields 与 identityDriftError 共用同一套逐项措辞的直接证据。
func TestAuditDriftDetectsRulesChange(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)

	ac := cfg.Agents[testAgentClaude]
	p, err := provider.Lookup(ac.Provider)
	if err != nil {
		t.Fatalf("provider.Lookup 失败: %v", err)
	}
	tier, err := TranslatePermission(ac.Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission 失败: %v", err)
	}
	// workDir 与下面 Runner 共用一条，理由同 TestAuditDriftNoChangeReturnsEmpty：
	// 指纹第七段依赖 WorkDir，两个独立的 t.TempDir() 会把「命令行」也算进
	// 这次漂移，污染「本用例只改了全局规则」这个前提。
	workDir := t.TempDir()
	oldFP, err := identityFPFor(p, testAgentClaude, ac, workDir, convID, tier, RulesBlocks{})
	if err != nil {
		t.Fatalf("identityFPFor 失败: %v", err)
	}
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "sess-1",
		IdentityFP:        oldFP,
	})

	r := NewRunner(cfg, svc, workDir, RulesBlocks{Global: "新加的全局规则"})
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("期望恰好 1 行漂移，实际 %d 行: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.ConvID != convID || row.AgentID != testAgentClaude {
		t.Errorf("漂移行是 %+v，期望 ConvID=%s AgentID=%s", row, convID, testAgentClaude)
	}
	if !strings.Contains(row.Reason, "全局规则") {
		t.Errorf("Reason = %q，应当点名全局规则", row.Reason)
	}
	if strings.Contains(row.Reason, "role") {
		t.Errorf("Reason = %q，role 没有变，不该被提到", row.Reason)
	}
}

// TestAuditDriftUnparseableFingerprint 覆盖「指纹解不开」：库里这一行的
// identity_fp 本身就不是合法编码（理论上 schema v3 迁移之后不该再有这种行，
// 但要 fail-closed 地报出来，不能悄悄当成没有漂移放过）。
func TestAuditDriftUnparseableFingerprint(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "sess-1",
		IdentityFP:        "这不是合法编码",
	})

	r := newTestRunner(t, cfg, svc)
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Reason != "指纹格式不可解" {
		t.Fatalf("期望恰好 1 行 Reason=\"指纹格式不可解\"，实际: %+v", rows)
	}
}

// TestAuditDriftAgentRemovedFromConfig 覆盖「配置里已无此 agent」：那条会话
// 还在库里占着位，用户删了 agent 又加回来时它会复活，所以这一条也要报。
func TestAuditDriftAgentRemovedFromConfig(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "sess-1",
		IdentityFP:        "whatever-fp",
	})

	delete(cfg.Agents, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Reason != "配置里已无此 agent" {
		t.Fatalf("期望恰好 1 行 Reason=\"配置里已无此 agent\"，实际: %+v", rows)
	}
}

// TestAuditDriftProviderLookupFailure 覆盖任务书要求实现方自行判断的情况
// 之一：配置里 agent 还在，但它的 provider 名此刻查不到（写错了，或者
// 那个适配器被下线了）。不跳过、不让整批审计失败，报出「算不出」本身。
func TestAuditDriftProviderLookupFailure(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "sess-1",
		IdentityFP:        "whatever-fp",
	})

	ac := cfg.Agents[testAgentClaude]
	ac.Provider = "no-such-provider-xyz"
	cfg.Agents[testAgentClaude] = ac

	r := newTestRunner(t, cfg, svc)
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 不该因为一行算不出来就整体失败: %v", err)
	}
	if len(rows) != 1 || !strings.Contains(rows[0].Reason, "provider 不可用") {
		t.Fatalf("期望恰好 1 行提示 provider 不可用，实际: %+v", rows)
	}
}

// TestAuditDriftPermissionTranslateFailure 覆盖另一种「算不出」：配置里
// agent 还在、provider 也查得到，但当前权限档不被放行。用 full 而不是
// workspace 触发——11c 第二片放行了 workspace，只有 full（以及拼错的档）
// 仍然在 TranslatePermission 那一层被拒。同样不该让整批审计失败。
func TestAuditDriftPermissionTranslateFailure(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
		ProviderSessionID: "sess-1",
		IdentityFP:        "whatever-fp",
	})

	ac := cfg.Agents[testAgentClaude]
	ac.Permissions = config.PermissionFull
	cfg.Agents[testAgentClaude] = ac

	r := newTestRunner(t, cfg, svc)
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 不该因为一行算不出来就整体失败: %v", err)
	}
	if len(rows) != 1 || !strings.Contains(rows[0].Reason, "权限档不放行") {
		t.Fatalf("期望恰好 1 行提示权限档不放行，实际: %+v", rows)
	}
}

// TestAuditDriftPrefixDigestFailure 覆盖未决 #18 identityFPFor 新增的失败出口：
// 配置里 agent 还在、provider 也查得到、权限档也放行，但探针本身组不出命令行
// （agy 把 Role 拼进探针正文最前面，一段真正超大的 Role 会让探针撞上
// ErrPromptTooLong）。这一支必须落成"算不出指纹: ..."这一行，不让整批审计
// 失败——与 provider 不可用、权限档不放行同体例。
func TestAuditDriftPrefixDigestFailure(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	seedProviderSession(t, svc, convID, testAgentAgy, conversation.ProviderSession{
		ProviderSessionID: "sess-1",
		IdentityFP:        "whatever-fp",
	})

	ac := cfg.Agents[testAgentAgy]
	ac.Role = strings.Repeat("R", 40000)
	cfg.Agents[testAgentAgy] = ac

	r := newTestRunner(t, cfg, svc)
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 不该因为一行算不出来就整体失败: %v", err)
	}
	if len(rows) != 1 || !strings.Contains(rows[0].Reason, "算不出指纹") {
		t.Fatalf("期望恰好 1 行提示算不出指纹，实际: %+v", rows)
	}
}

// erroringAllSessionsStore 是一个只让 AllProviderSessions 报错的最小
// conversation.Store 实现，其余方法都不会被 AuditDrift 走到——它只调
// Service.AllProviderSessions 这一条读路径，不碰 Conversation /
// Participants 之类。存在的唯一理由是给 TestAuditDriftPropagatesStoreError
// 用，验证 AuditDrift 不吞掉库层面的错误。
type erroringAllSessionsStore struct{}

var errAllSessionsAlwaysFails = errors.New("erroringAllSessionsStore: AllProviderSessions always fails, by design")

func (erroringAllSessionsStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return errors.New("erroringAllSessionsStore: WriteTx should not be reached by this test")
}
func (erroringAllSessionsStore) Conversation(ctx context.Context, id string) (conversation.Conversation, error) {
	return conversation.Conversation{}, conversation.ErrConversationNotFound
}
func (erroringAllSessionsStore) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	return nil, nil
}

// ArchivedConversations 只为满足接口而存在，理由同上：AuditDrift 不读会话列表。
func (erroringAllSessionsStore) ArchivedConversations(ctx context.Context) ([]conversation.Conversation, error) {
	return nil, nil
}
func (erroringAllSessionsStore) Participants(ctx context.Context, convID string, includeInactive bool) ([]conversation.Participant, error) {
	return nil, nil
}
func (erroringAllSessionsStore) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]conversation.Message, error) {
	return nil, nil
}
func (erroringAllSessionsStore) MaxSeq(ctx context.Context, convID string) (int64, error) {
	return 0, nil
}
func (erroringAllSessionsStore) Cursor(ctx context.Context, convID, agentID string) (conversation.Cursor, error) {
	return conversation.Cursor{}, conversation.ErrNoCursor
}
func (erroringAllSessionsStore) Cursors(ctx context.Context, convID string) ([]conversation.Cursor, error) {
	return nil, nil
}
func (erroringAllSessionsStore) ProviderSession(ctx context.Context, convID, agentID string) (conversation.ProviderSession, bool, error) {
	return conversation.ProviderSession{}, false, nil
}
func (erroringAllSessionsStore) AllProviderSessions(ctx context.Context) ([]conversation.ProviderSession, error) {
	return nil, errAllSessionsAlwaysFails
}

// InflightFor 为 Store.InflightFor 补的方法。本假实现 AllInflight 恒答空，这里恒答没有。
func (erroringAllSessionsStore) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error) {
	return conversation.InflightTurn{}, false, nil
}

// AllInflight 不属于本假实现的既定失败面：它存在的唯一理由是让
// Service.AllProviderSessions 这一条读路径必然出错，其余方法一律
// 答「没有/空」而不是报错，AllInflight 同样如此。
func (erroringAllSessionsStore) AllInflight(ctx context.Context) ([]conversation.InflightTurn, error) {
	return nil, nil
}

// UsageSamples 只为满足接口而存在：erroringAllSessionsStore 的既定失败面
// 只在 AllProviderSessions 一个方法上，本文件的用例不走预报路径。
func (erroringAllSessionsStore) UsageSamples(ctx context.Context, agentID, identityFP string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

// TurnUsages 是 Step 10 D 片之一给 Store 加的读方法（/cost 用），只为满足
// 接口而存在，与上面 UsageSamples 同一个理由：erroringAllSessionsStore 的
// 既定失败面只在 AllProviderSessions 一个方法上，本文件的用例（AuditDrift
// 只读 Service.AllProviderSessions 这一条路径）不会走到 turn_usage 读路径。
//
// 失效条件：这个类型哪天被复用于一条会调用 Service.TurnUsage 的用例，
// 要么给它另开一个失败面（同上面 errAllSessionsAlwaysFails 那种专用哨兵
// 错误），要么换成有状态的假实现，不能让这个恒答"没有"的空桩被当成
// "读到了一个干净的空会话"。
func (erroringAllSessionsStore) TurnUsages(ctx context.Context, convID, agentID string, limit int) ([]conversation.TurnUsage, error) {
	return nil, nil
}

// TestAuditDriftPropagatesStoreError 覆盖「审计本身失败（读库出错）」这
// 一支：AuditDrift 不该吞掉 Service.AllProviderSessions 的错误、也不该
// 把它悄悄当成「0 行」处理——调用方（/rules 与 app.go 的启动审计）都要
// 能分清「没有漂移」与「读不出来，不知道有没有漂移」，这是两件不同的事。
func TestAuditDriftPropagatesStoreError(t *testing.T) {
	cfg := newTestConfig()
	svc := conversation.NewService(erroringAllSessionsStore{})
	r := NewRunner(cfg, svc, t.TempDir(), RulesBlocks{})

	rows, err := r.AuditDrift(context.Background())
	if !errors.Is(err, errAllSessionsAlwaysFails) {
		t.Fatalf("errors.Is(err, errAllSessionsAlwaysFails) 应当为真，实际: %v", err)
	}
	if rows != nil {
		t.Errorf("读库失败时应当返回 nil 行，实际: %+v", rows)
	}
}

// TestAuditDriftSortedByConvThenAgent 覆盖返回顺序：按 (ConvID, AgentID)
// 升序，不依赖 Service.AllProviderSessions 恰好已经排好序这件事——
// conv id 是随机生成的十六进制串，这里自己算一遍期望顺序再比对，
// 而不是假设创建顺序就是字典序。
func TestAuditDriftSortedByConvThenAgent(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)

	convX := newConv(t, svc)
	convY := newConv(t, svc)

	type pair struct{ conv, agent string }
	pairs := []pair{
		{convY, testAgentClaude},
		{convX, testAgentGrok},
		{convX, testAgentClaude},
	}
	for _, p := range pairs {
		seedProviderSession(t, svc, p.conv, p.agent, conversation.ProviderSession{
			ProviderSessionID: "sess",
			// 解不开的指纹恒报漂移，且与 convID/agentID 无关，
			// 这样每一行是否被报出来不依赖除排序之外的任何逻辑。
			IdentityFP: "恒漂移的坏编码",
		})
	}

	r := newTestRunner(t, cfg, svc)
	rows, err := r.AuditDrift(context.Background())
	if err != nil {
		t.Fatalf("AuditDrift 失败: %v", err)
	}
	if len(rows) != len(pairs) {
		t.Fatalf("期望 %d 行，实际 %d 行: %+v", len(pairs), len(rows), rows)
	}

	want := make([]pair, len(pairs))
	copy(want, pairs)
	sort.Slice(want, func(i, j int) bool {
		if want[i].conv != want[j].conv {
			return want[i].conv < want[j].conv
		}
		return want[i].agent < want[j].agent
	})
	for i, w := range want {
		if rows[i].ConvID != w.conv || rows[i].AgentID != w.agent {
			t.Errorf("第 %d 行 = (ConvID=%s, AgentID=%s)，期望 (%s, %s)", i, rows[i].ConvID, rows[i].AgentID, w.conv, w.agent)
		}
	}
}
