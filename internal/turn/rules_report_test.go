package turn

// rules_report_test.go 覆盖 11a 契约 5.1 需要的 Runner.RulesReport：
// 逐 provider 一行、按 provider id 升序、隔离位取自 Capabilities、
// 命令行余量随规则长度变化、以及两类"组不出来"不能让整条命令失败。

import (
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

// TestRulesReportListsRegisteredProvidersSorted 覆盖基本形状：newTestConfig
// 的三个 agent 各自挂在 claude/grok/agy 上，报告应当恰好三行、按 id 升序，
// 且 CleanInstrEnv 与各自 Capabilities() 里声明的值一致——这是"不要在
// internal/turn 里编一个能力位，只转发已有声明"这条要求的直接断言。
func TestRulesReportListsRegisteredProvidersSorted(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	rows := r.RulesReport()
	if len(rows) != 3 {
		t.Fatalf("期望 3 行（claude/grok/agy 各一行），实际 %d 行: %+v", len(rows), rows)
	}

	wantOrder := []string{"agy", "claude", "grok"}
	for i, want := range wantOrder {
		if rows[i].ProviderID != want {
			t.Errorf("第 %d 行 ProviderID = %q，期望 %q（应按字典序）", i, rows[i].ProviderID, want)
		}
	}

	wantClean := map[string]provider.Support{
		"claude": provider.SupportYes,
		"grok":   provider.SupportNo,
		"agy":    provider.SupportUnknown,
	}
	// wantCache 镜像上面 wantClean 的形状：CacheAcrossTurns 同样是本包
	// 转发、不加工的一个实测能力位，钉住"报告里的值与 Capabilities() 一致"
	// 这条与 CleanInstrEnv 是同一类断言。
	wantCache := map[string]provider.Support{
		"claude": provider.SupportYes,
		"grok":   provider.SupportYes,
		"agy":    provider.SupportNo,
	}
	for _, row := range rows {
		p, err := provider.Lookup(row.ProviderID)
		if err != nil {
			t.Fatalf("provider.Lookup(%q) 失败: %v", row.ProviderID, err)
		}
		if want := wantClean[row.ProviderID]; row.CleanInstrEnv != want {
			t.Errorf("provider %s 的 CleanInstrEnv = %v，期望 %v", row.ProviderID, row.CleanInstrEnv, want)
		}
		if row.CleanInstrEnv != p.Capabilities().CleanInstructionEnv {
			t.Errorf("provider %s 的 CleanInstrEnv 与 Capabilities() 报的不一致: %v vs %v",
				row.ProviderID, row.CleanInstrEnv, p.Capabilities().CleanInstructionEnv)
		}
		if want := wantCache[row.ProviderID]; row.CacheAcrossTurns != want {
			t.Errorf("provider %s 的 CacheAcrossTurns = %v，期望 %v", row.ProviderID, row.CacheAcrossTurns, want)
		}
		if row.CacheAcrossTurns != p.Capabilities().CacheAcrossTurns {
			t.Errorf("provider %s 的 CacheAcrossTurns 与 Capabilities() 报的不一致: %v vs %v",
				row.ProviderID, row.CacheAcrossTurns, p.Capabilities().CacheAcrossTurns)
		}
	}
}

// TestRulesReportHeadroomShrinksWithRulesSizeExceptClaude 是本任务最核心的
// 一条区分力断言：grok 与 agy 把身份块（含规则）直接放进 argv，规则变长
// 之后命令行余量必须相应变小；claude 走 --append-system-prompt-file（文件）
// 加 stdin，规则变长不该影响它的 argv 长度——如果实现把 Role 的长度也算
// 进了 claude 那一行的余量，这条会抓到。
func TestRulesReportHeadroomShrinksWithRulesSizeExceptClaude(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)

	small := newTestRunner(t, cfg, svc)
	rowsSmall := indexRulesReport(t, small.RulesReport())

	big := NewRunner(cfg, svc, small.workDir, RulesBlocks{Global: strings.Repeat("规则正文占位", 500)})
	rowsBig := indexRulesReport(t, big.RulesReport())

	for _, id := range []string{"grok", "agy"} {
		s, b := rowsSmall[id], rowsBig[id]
		if s.HeadroomNote != "" || b.HeadroomNote != "" {
			t.Fatalf("provider %s 探针不该组不出来: small=%+v big=%+v", id, s, b)
		}
		if !(b.HeadroomBytes < s.HeadroomBytes) {
			t.Errorf("provider %s 规则变长之后余量应当变小：规则短时 %d，规则长时 %d", id, s.HeadroomBytes, b.HeadroomBytes)
		}
	}

	claudeSmall, claudeBig := rowsSmall["claude"], rowsBig["claude"]
	if claudeSmall.HeadroomNote != "" || claudeBig.HeadroomNote != "" {
		t.Fatalf("claude 探针不该组不出来: small=%+v big=%+v", claudeSmall, claudeBig)
	}
	if claudeSmall.HeadroomBytes != claudeBig.HeadroomBytes {
		t.Errorf("claude 走文件加 stdin，规则变长不该影响它的命令行余量，实际从 %d 变成了 %d",
			claudeSmall.HeadroomBytes, claudeBig.HeadroomBytes)
	}
}

func indexRulesReport(t *testing.T, rows []ProviderRulesInfo) map[string]ProviderRulesInfo {
	t.Helper()
	out := make(map[string]ProviderRulesInfo, len(rows))
	for _, row := range rows {
		out[row.ProviderID] = row
	}
	return out
}

// TestRulesReportSkipsUnregisteredProvider 覆盖「配置里出现过、但
// provider.registry 查不到」的情况：不панic、也不在报告里出现一行全是
// 零值的记录——见 RulesReport 的函数注释，这是本函数在契约没覆盖到的
// 情形下做的选择。
func TestRulesReportSkipsUnregisteredProvider(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: "claude-fake"},
		},
		Agents: map[string]config.AgentConfig{
			testAgentClaude: {
				DisplayName: "Alice",
				Provider:    "claude",
				Model:       "claude-fable-5-1",
				Permissions: config.PermissionRead,
			},
			"ghost": {
				DisplayName: "Ghost",
				Provider:    "no-such-provider-xyz",
				Model:       "whatever",
				Permissions: config.PermissionRead,
			},
		},
	}
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	rows := r.RulesReport()
	if len(rows) != 1 || rows[0].ProviderID != "claude" {
		t.Fatalf("期望恰好 1 行（claude），未注册的 provider 应当被跳过，实际: %+v", rows)
	}
}

// TestRulesReportPermissionNotSupportedGivesNoteNotFailure 覆盖「探针组不出
// 命令行」的一种成因：代表 agent 的权限档不被编排器放行。用 full 而不是
// workspace——11c 第二片放行了 claude 的 workspace 档，只有 full（以及
// 拼错的档）仍然会在 TranslatePermission 那一层被拒。要求 HeadroomNote
// 非空、HeadroomBytes 不被使用，且不影响其它 provider 那一行能不能正常
// 报出来。
func TestRulesReportPermissionNotSupportedGivesNoteNotFailure(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: "claude-fake"},
			"grok":   {Command: "grok-fake"},
		},
		Agents: map[string]config.AgentConfig{
			testAgentClaude: {
				DisplayName: "Alice",
				Provider:    "claude",
				Model:       "claude-fable-5-1",
				Permissions: config.PermissionFull, // 编排器不提供（06 第 2 节）
			},
			testAgentGrok: {
				DisplayName: "Zulu",
				Provider:    "grok",
				Model:       "grok-4.6",
				Permissions: config.PermissionRead,
			},
		},
	}
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	rows := indexRulesReport(t, r.RulesReport())
	claudeRow, ok := rows["claude"]
	if !ok {
		t.Fatal("claude 那一行应当仍然出现在报告里")
	}
	if claudeRow.HeadroomNote == "" {
		t.Error("权限档不被放行时 HeadroomNote 应当非空，说明组不出探针命令行的原因")
	}
	grokRow, ok := rows["grok"]
	if !ok || grokRow.HeadroomNote != "" {
		t.Errorf("grok 那一行不该受 claude 探针失败的影响，实际: %+v (ok=%v)", grokRow, ok)
	}
}
