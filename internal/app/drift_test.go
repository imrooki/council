package app

// drift_test.go 覆盖 11a 契约 6 节：启动漂移审计。0 条时什么都不印，
// 有漂移时打冻结格式的提醒（外加逐条明细），审计本身失败时只记一行
// 到 Err、不阻止进 REPL。X-39f 之后提醒按后果分两句（会重建付费 / 会被拒不花钱），
// 本文件的种子都是会重建的那类（解不开的指纹），两句的分类由 x39f_driftnotice_test.go 覆盖。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"
)

// seedMismatchedProviderSession 直接打开 dataDir 下的库，写入一条
// identity_fp 与当前配置/规则算出来的值必然不同的 provider 会话
// （随便一个非空字符串就行——AuditDrift 判定漂移只看指纹对不对得上，
// 不关心具体是哪一种"对不上"），并返回它的会话 id 供断言用。
func seedMismatchedProviderSession(t *testing.T, dataDir, agentID string) (convID string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭 store 失败: %v", err)
		}
	}()

	svc := conversation.NewService(st)
	conv, err := svc.Create(ctx, "启动漂移审计用例")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}

	err = st.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, conversation.ProviderSession{
			ConvID:            conv.ID,
			AgentID:           agentID,
			ProviderSessionID: "seeded-mismatched-session",
			CursorSeq:         0,
			LastTurnAt:        time.Now().UTC(),
			IdentityFP:        "启动前手工塞入的、必然对不上当前配置的指纹",
		})
	})
	if err != nil {
		t.Fatalf("写入种子 provider 会话失败: %v", err)
	}
	return conv.ID
}

// TestStartupDriftAuditReportsMismatchedSessions 覆盖 N 条这一支：
// 先跑一次 Run 建库，再直接往库里塞一条身份对不上的 provider 会话，
// 第二次 Run 的横幅里必须出现冻结格式的那一行，且点名涉及的会话与成员。
func TestStartupDriftAuditReportsMismatchedSessions(t *testing.T) {
	isolateAppEnv(t)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := writeTestConfig(t, dir, validConfigBody(dataDir))

	if _, _, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader("")); err != nil {
		t.Fatalf("第一次 Run（建库）应当返回 nil，实际: %v", err)
	}

	const agentID = "theory" // validConfigBody 里配置的唯一 agent
	convID := seedMismatchedProviderSession(t, dataDir, agentID)

	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(""))
	if err != nil {
		t.Fatalf("第二次 Run 应当返回 nil，实际 %v；Err 输出:\n%s", err, errOut)
	}
	const wantLine = "注意: 有 1 条 provider 会话的身份已变，下次 /ask 会各重建一次（每条一次入场费）"
	if !strings.Contains(out, wantLine) {
		t.Errorf("横幅里应当出现冻结格式的提醒行 %q；Out 完整输出:\n%s", wantLine, out)
	}
	if !strings.Contains(out, convID) || !strings.Contains(out, agentID) {
		t.Errorf("提醒应当点名涉及的会话 %q 与成员 %q；Out 完整输出:\n%s", convID, agentID, out)
	}
}

// TestStartupDriftAuditSilentWhenNoDrift 覆盖 0 条这一支：一次全新的库
// 没有任何 provider 会话，横幅里不该出现漂移相关的任何字样，不能制造噪声。
func TestStartupDriftAuditSilentWhenNoDrift(t *testing.T) {
	isolateAppEnv(t)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := writeTestConfig(t, dir, validConfigBody(dataDir))

	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；Err 输出:\n%s", err, errOut)
	}
	for _, forbidden := range []string{"身份已变", "注意:"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("全新的库不该有任何 provider 会话，横幅里不该出现 %q；Out 完整输出:\n%s", forbidden, out)
		}
	}
}

// 「审计本身失败（读库出错）只往 Err 记一行、不阻止进 REPL」这一支
// 没有在本文件里单独构造：要让 rn.AuditDrift 内部的
// Service.AllProviderSessions 报错，同时又不影响前面 store.Open /
// config.LoadRules 都成功，需要在 store 已经打开、迁移已经跑完之后，
// 只让这一条 SQL 查询本身失败——用真实的 SQLite 文件做不到这种精确的
// 故障注入，伪造需要绕开 internal/app 只认 *turn.Runner（内部持有具体的
// *conversation.Service，不是接口）这一层具体类型，为了测这一条分支去改
// 生产代码的类型不划算。这条分支的正确性由两处间接证据支撑：
//  1. internal/turn/drift_test.go 已经在更低一层单独验证过
//     Runner.AuditDrift 在 Service.AllProviderSessions 报错时会原样把
//     错误包起来返回（TestAuditDriftPropagatesStoreError）；
//  2. app.go 里 `if rows, auditErr := rn.AuditDrift(ctx); auditErr != nil`
//     这一条分支只做 fmt.Fprintf(errOut, ...)，结构上不可能触发
//     return——对照本文件其余「配置/规则读不出来就 return 错误」的用例，
//     这一条在源码里明显是另一种形状（没有 return 语句），走读即可确认。
