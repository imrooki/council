package turn

// x39f_driftkind_test.go 覆盖 X-39f 契约 2.1：DriftRow.Kind 的六类判定、
// 常量取值与 AskRefused 的语义，以及第 1 类「被拒、不花钱、行还在」这件
// 事本身（T3），用来担保 AskRefused 对第 1 类说的是真话。
//
// 本文件引用了新 API（DriftKind、DriftRow.Kind、AskRefused），在 7343f0c
// 上编译不过，这是预期的，不算「HEAD 红」。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// x39fReasonPrefixProvider 等三个前缀是第 2、3、4 类 Reason 今天的取值
// 开头（drift.go 里 fmt.Sprintf 的格式串去掉 %v），Reason 文本冻结不改。
const (
	x39fReasonAgentGone      = "配置里已无此 agent"
	x39fReasonPrefixProvider = "provider 不可用，算不出指纹: "
	x39fReasonPrefixPerm     = "权限档不放行，算不出指纹: "
	x39fReasonPrefixFP       = "算不出指纹: "
	x39fReasonUnparsable     = "指纹格式不可解"
)

// x39fKindCase 是 T1 的一个子用例：setup 在给定的配置与库上造出恰好一行
// 某一类漂移，并返回审计要用的 Runner。
type x39fKindCase struct {
	name        string
	wantKind    DriftKind
	wantRefused bool
	setup       func(t *testing.T) *Runner
	checkReason func(t *testing.T, reason string)
}

// x39fReasonEquals 返回一个断言 Reason 全等于 want 的检查函数。
func x39fReasonEquals(want string) func(t *testing.T, reason string) {
	return func(t *testing.T, reason string) {
		t.Helper()
		if reason != want {
			t.Errorf("Reason = %q，期望全等于 %q", reason, want)
		}
	}
}

// x39fReasonHasPrefix 返回一个断言 Reason 以 prefix 开头的检查函数。
func x39fReasonHasPrefix(prefix string) func(t *testing.T, reason string) {
	return func(t *testing.T, reason string) {
		t.Helper()
		if !strings.HasPrefix(reason, prefix) {
			t.Errorf("Reason = %q，期望以 %q 开头", reason, prefix)
		}
	}
}

// x39fSeedAnyFP 给 (convID, agentID) 种一行任意（解不开的）指纹。
func x39fSeedAnyFP(t *testing.T, svc *conversation.Service, convID, agentID string) {
	t.Helper()
	seedProviderSession(t, svc, convID, agentID, conversation.ProviderSession{
		ProviderSessionID: "sess-x39f",
		IdentityFP:        "x39f 任意的坏编码指纹",
	})
}

// TestX39fDriftKindSixBranches 是 T1：六类各种一行，断言恰好 1 行、Kind
// 等于对应常量、AskRefused 前四类为真后两类为假，Reason 与今天的取值一致。
func TestX39fDriftKindSixBranches(t *testing.T) {
	cases := []x39fKindCase{
		{
			name:        "第1类_配置里已无此agent",
			wantKind:    DriftAgentGone,
			wantRefused: true,
			setup: func(t *testing.T) *Runner {
				cfg := newTestConfig()
				svc := newTestService(t)
				convID := newConv(t, svc)
				x39fSeedAnyFP(t, svc, convID, testAgentClaude)
				delete(cfg.Agents, testAgentClaude)
				return newTestRunner(t, cfg, svc)
			},
			checkReason: x39fReasonEquals(x39fReasonAgentGone),
		},
		{
			name:        "第2类_provider不可用",
			wantKind:    DriftProviderUnavailable,
			wantRefused: true,
			setup: func(t *testing.T) *Runner {
				cfg := newTestConfig()
				svc := newTestService(t)
				convID := newConv(t, svc)
				x39fSeedAnyFP(t, svc, convID, testAgentClaude)
				ac := cfg.Agents[testAgentClaude]
				ac.Provider = "no-such-provider-xyz"
				cfg.Agents[testAgentClaude] = ac
				return newTestRunner(t, cfg, svc)
			},
			checkReason: x39fReasonHasPrefix(x39fReasonPrefixProvider),
		},
		{
			name:        "第3类_权限档不放行",
			wantKind:    DriftPermissionRefused,
			wantRefused: true,
			setup: func(t *testing.T) *Runner {
				cfg := newTestConfig()
				svc := newTestService(t)
				convID := newConv(t, svc)
				x39fSeedAnyFP(t, svc, convID, testAgentClaude)
				ac := cfg.Agents[testAgentClaude]
				ac.Permissions = config.PermissionFull
				cfg.Agents[testAgentClaude] = ac
				return newTestRunner(t, cfg, svc)
			},
			checkReason: x39fReasonHasPrefix(x39fReasonPrefixPerm),
		},
		{
			name:        "第4类_算不出指纹",
			wantKind:    DriftFingerprintUnavailable,
			wantRefused: true,
			setup: func(t *testing.T) *Runner {
				cfg := newTestConfig()
				svc := newTestService(t)
				convID := newConv(t, svc)
				x39fSeedAnyFP(t, svc, convID, testAgentAgy)
				ac := cfg.Agents[testAgentAgy]
				ac.Role = strings.Repeat("R", 40000)
				cfg.Agents[testAgentAgy] = ac
				return newTestRunner(t, cfg, svc)
			},
			checkReason: x39fReasonHasPrefix(x39fReasonPrefixFP),
		},
		{
			name:        "第5类_指纹格式不可解",
			wantKind:    DriftFingerprintUnparsable,
			wantRefused: false,
			setup: func(t *testing.T) *Runner {
				cfg := newTestConfig()
				svc := newTestService(t)
				convID := newConv(t, svc)
				x39fSeedAnyFP(t, svc, convID, testAgentClaude)
				return newTestRunner(t, cfg, svc)
			},
			checkReason: x39fReasonEquals(x39fReasonUnparsable),
		},
		{
			name:        "第6类_逐字段差异",
			wantKind:    DriftFieldsChanged,
			wantRefused: false,
			setup: func(t *testing.T) *Runner {
				// 照 drift_test.go 的 TestAuditDriftDetectsRulesChange：旧指纹用空
				// 规则算，Runner 手里换成非空的全局规则；workDir 必须共用一条。
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
				workDir := t.TempDir()
				oldFP, err := identityFPFor(p, testAgentClaude, ac, workDir, convID, tier, RulesBlocks{})
				if err != nil {
					t.Fatalf("identityFPFor 失败: %v", err)
				}
				seedProviderSession(t, svc, convID, testAgentClaude, conversation.ProviderSession{
					ProviderSessionID: "sess-x39f",
					IdentityFP:        oldFP,
				})
				return NewRunner(cfg, svc, workDir, RulesBlocks{Global: "x39f 新加的全局规则"})
			},
			checkReason: func(t *testing.T, reason string) {
				t.Helper()
				if reason == "" {
					t.Errorf("第 6 类的 Reason 不该为空")
				}
				if reason == x39fReasonAgentGone || reason == x39fReasonUnparsable ||
					strings.HasPrefix(reason, x39fReasonPrefixProvider) ||
					strings.HasPrefix(reason, x39fReasonPrefixPerm) ||
					strings.HasPrefix(reason, x39fReasonPrefixFP) {
					t.Errorf("第 6 类的 Reason = %q，不该是前五类的任何一种", reason)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.setup(t)
			rows, err := r.AuditDrift(context.Background())
			if err != nil {
				t.Fatalf("AuditDrift 失败: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("期望恰好 1 行，实际 %d 行: %+v", len(rows), rows)
			}
			row := rows[0]
			if row.Kind != tc.wantKind {
				t.Errorf("Kind = %d，期望 %d（Reason=%q）", int(row.Kind), int(tc.wantKind), row.Reason)
			}
			if got := row.AskRefused(); got != tc.wantRefused {
				t.Errorf("AskRefused() = %v，期望 %v（Kind=%d，Reason=%q）", got, tc.wantRefused, int(row.Kind), row.Reason)
			}
			tc.checkReason(t, row.Reason)
		})
	}
}

// TestX39fDriftKindConstantsAndZeroValue 是 T2：六个常量恰好是 1..6、
// DriftKindUnset 是 0；零值行与越界值行的 AskRefused 都为假。
func TestX39fDriftKindConstantsAndZeroValue(t *testing.T) {
	want := []struct {
		name string
		got  DriftKind
		val  int
	}{
		{"DriftKindUnset", DriftKindUnset, 0},
		{"DriftAgentGone", DriftAgentGone, 1},
		{"DriftProviderUnavailable", DriftProviderUnavailable, 2},
		{"DriftPermissionRefused", DriftPermissionRefused, 3},
		{"DriftFingerprintUnavailable", DriftFingerprintUnavailable, 4},
		{"DriftFingerprintUnparsable", DriftFingerprintUnparsable, 5},
		{"DriftFieldsChanged", DriftFieldsChanged, 6},
	}
	for _, w := range want {
		if int(w.got) != w.val {
			t.Errorf("%s = %d，期望 %d", w.name, int(w.got), w.val)
		}
	}

	if (DriftRow{}).AskRefused() {
		t.Errorf("零值 DriftRow{}（Kind 未填）的 AskRefused() 应为 false")
	}
	if (DriftRow{Kind: 7}).AskRefused() {
		t.Errorf("DriftRow{Kind: 7}（不属于任何一类）的 AskRefused() 应为 false")
	}

	// 逐值的真值表，不经 AuditDrift：与 T1 互为印证，T1 查的是分支填值，
	// 这里只查 AskRefused 本身。
	for k := 1; k <= 6; k++ {
		wantRefused := k <= 4
		if got := (DriftRow{Kind: DriftKind(k)}).AskRefused(); got != wantRefused {
			t.Errorf("DriftRow{Kind: %d}.AskRefused() = %v，期望 %v", k, got, wantRefused)
		}
	}
}

// TestX39fAgentGoneAskRefusedWithoutSpawnAndRowKept 是 T3：已删 agent 的
// 真实后果。Ask 在起进程之前被拒（ErrAgentUnknown、Spawner 零次调用）、
// 库里那一行不动；AuditDrift 仍报 1 行第 1 类；Rebuild 能删掉它。
func TestX39fAgentGoneAskRefusedWithoutSpawnAndRowKept(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "x39f：删 agent 之前的一句话")
	x39fSeedAnyFP(t, svc, convID, testAgentClaude)
	delete(cfg.Agents, testAgentClaude)

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	ctx := context.Background()

	rec, err := r.Ask(ctx, convID, testAgentClaude)
	if rec.Outcome != OutcomeRefused {
		t.Errorf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if !errors.Is(err, ErrAgentUnknown) {
		t.Errorf("errors.Is(err, ErrAgentUnknown) 应为真，实际 err=%v", err)
	}
	if len(fs.calls) != 0 {
		t.Errorf("Spawner 被调用了 %d 次，期望 0 次（被拒类不花钱）", len(fs.calls))
	}

	if _, found, psErr := svc.ProviderSession(ctx, convID, testAgentClaude); psErr != nil || !found {
		t.Fatalf("被拒的 Ask 不该碰 provider_sessions，那一行应当还在；found=%v err=%v", found, psErr)
	}

	rows, err := r.AuditDrift(ctx)
	if err != nil {
		t.Fatalf("AuditDrift 失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Kind != DriftAgentGone {
		t.Fatalf("Ask 被拒之后 AuditDrift 应当仍报 1 行第 1 类，实际: %+v", rows)
	}

	existed, err := r.Rebuild(ctx, convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Rebuild 失败: %v", err)
	}
	if !existed {
		t.Errorf("Rebuild 应当报 existed=true（那一行还在）")
	}

	rows, err = r.AuditDrift(ctx)
	if err != nil {
		t.Fatalf("Rebuild 之后 AuditDrift 失败: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("Rebuild 之后 AuditDrift 应当为 0 行，实际: %+v", rows)
	}
}
