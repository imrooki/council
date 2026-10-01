package app

// step10a4_lost_turns_banner_test.go 覆盖 Step 10 A 片之四冻结契约第 6 节
// 验收清单第 8（横幅）、第 9（超过 10 条折叠）条。
//
// writeLostNotice 的第一行是冻结格式，直接单元测（不经 Run，控制 marked
// 与条数更精确）；「库干净时不出现那些字样」与「顺序在漂移横幅之前」两条
// 需要走完整的 Run()，用 seedInflightTurn 直接往库里塞一行在飞记录（全仓
// 没有任何造出「上次退出时崩在一轮中间」这个状态的命令，只能这样造），
// 照 drift_test.go 的 seedMismatchedProviderSession 同一个体例。
//
// 落盘时 turn.LostTurn / Runner.AuditInflight / Runner.SettleLostTurns /
// app.writeLostNotice 都还不存在，编译不过是预期。

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"
	"council/internal/turn"
)

func lostTurnFixture(convID, agentID, turnID string, phase conversation.InflightPhase, reason string) turn.LostTurn {
	return turn.LostTurn{
		Turn: conversation.InflightTurn{
			TurnID:    turnID,
			ConvID:    convID,
			AgentID:   agentID,
			StartedAt: time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC),
			Phase:     phase,
		},
		Reason: reason,
	}
}

// TestWriteLostNoticeFirstLineFormatAndMFromParameter 覆盖第 8 条的冻结
// 格式，以及「M 从参数传进来，不许在这一层用 phase 反推」：三条记录全是
// phase=spawned（不是 begun），marked 单独传 2——如果实现在这一层偷懒去数
// phase != begun 的条数，会数出 3 而不是我们传的 2，这里就会露出来。
func TestWriteLostNoticeFirstLineFormatAndMFromParameter(t *testing.T) {
	var buf strings.Builder
	lost := []turn.LostTurn{
		lostTurnFixture("conv-1", "alpha", "turn-1", conversation.PhaseSpawned, "起过进程，结局未知；这条 provider 会话点不出名字"),
		lostTurnFixture("conv-1", "bravo", "turn-2", conversation.PhaseSpawned, "起过进程，结局未知；这条 provider 会话点不出名字"),
		lostTurnFixture("conv-2", "charlie", "turn-3", conversation.PhaseSpawned, "起过进程，结局未知；这条 provider 会话点不出名字"),
	}
	writeLostNotice(&buf, lost, 2)

	out := buf.String()
	const wantFirstLine = "注意: 有 3 轮在上次退出时没记上账，其中 2 轮已把相关成员的成本基线标为可疑"
	if !strings.Contains(out, wantFirstLine) {
		t.Errorf("输出里应当有冻结格式的第一行 %q；完整输出：\n%s", wantFirstLine, out)
	}
}

// TestWriteLostNoticeZeroMarkedIsNotSpecialCased 覆盖第 8 条「M == 0 时
// 照常打印「其中 0 轮」，不做特判」。同样全用 phase=spawned（不是 begun），
// 挡的是同一类「在这一层用 phase 反推」的坏实现——若反推，这里会打出
// 「其中 2 轮」而不是我们要求的「其中 0 轮」。
func TestWriteLostNoticeZeroMarkedIsNotSpecialCased(t *testing.T) {
	var buf strings.Builder
	lost := []turn.LostTurn{
		lostTurnFixture("conv-1", "alpha", "turn-1", conversation.PhaseSpawned, "起过进程，结局未知；这条 provider 会话点不出名字"),
		lostTurnFixture("conv-1", "bravo", "turn-2", conversation.PhaseSpawned, "起过进程，结局未知；这条 provider 会话点不出名字"),
	}
	writeLostNotice(&buf, lost, 0)

	out := buf.String()
	const wantFirstLine = "注意: 有 2 轮在上次退出时没记上账，其中 0 轮已把相关成员的成本基线标为可疑"
	if !strings.Contains(out, wantFirstLine) {
		t.Errorf("输出里应当有 %q（不做特判）；完整输出：\n%s", wantFirstLine, out)
	}
}

// TestWriteLostNoticeFoldsBeyondCap 覆盖第 9 条：超过折叠上限时逐行点名
// 最多 turn.LostNoticeCap 条，其余折叠。折叠行本身的措辞契约没有冻结
// （「随便改，测试不许钉」），只断言可数的那部分：cap+5 条里恰好 cap 个
// 可辨认的 turnID 出现在输出里，其余不出现。引用 turn.LostNoticeCap 而不是
// 自己抄一份字面量 10——全仓只留一个出处正是返修轮把它导出的目的，
// 抄一份字面量会让「改了导出常量、测试还在拿旧数字比」这种漂移测不出来。
func TestWriteLostNoticeFoldsBeyondCap(t *testing.T) {
	var buf strings.Builder
	total := turn.LostNoticeCap + 5
	lost := make([]turn.LostTurn, 0, total)
	for i := 0; i < total; i++ {
		turnID := fmt.Sprintf("turn-fold-%02d", i)
		lost = append(lost, lostTurnFixture("conv-1", fmt.Sprintf("agent-%02d", i), turnID,
			conversation.PhaseSpawned, "起过进程，结局未知；这条 provider 会话点不出名字"))
	}
	writeLostNotice(&buf, lost, 0)
	out := buf.String()

	listed := 0
	for i := 0; i < total; i++ {
		if strings.Contains(out, fmt.Sprintf("turn-fold-%02d", i)) {
			listed++
		}
	}
	if listed != turn.LostNoticeCap {
		t.Errorf("输出里逐行点名的 turnID 有 %d 个，期望恰好 turn.LostNoticeCap(%d) 个（超过上限要折叠）；完整输出：\n%s",
			listed, turn.LostNoticeCap, out)
	}
}

// seedInflightTurn 直接打开 dataDir 下的库，写入一条在飞记录（模拟「上次
// 退出时进程崩在一轮中间」），withSession 为真时先建一条 provider 会话并
// 让在飞记录的 ResumeSessionID 指向它（这样 settle 之后能真的标上
// baseline_suspect）。照 drift_test.go 的 seedMismatchedProviderSession
// 同一个体例：全仓没有任何造出这个状态的命令，只能直接打开库写。
func seedInflightTurn(t *testing.T, dataDir, agentID string, withSession bool) (convID, turnID string) {
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
	conv, err := svc.Create(ctx, "启动在飞对账用例")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}

	const sessID = "seeded-lost-session"
	resumeID := ""
	if withSession {
		err = st.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
			return tx.SetProviderSession(ctx, conversation.ProviderSession{
				ConvID: conv.ID, AgentID: agentID, ProviderSessionID: sessID,
				CursorSeq: 1, LastTurnAt: time.Now().UTC(), IdentityFP: "fp-seeded",
			})
		})
		if err != nil {
			t.Fatalf("写入种子 provider 会话失败: %v", err)
		}
		resumeID = sessID
	}

	turnID = "seeded-inflight-turn"
	err = svc.BeginTurn(ctx, conversation.InflightTurn{
		TurnID: turnID, ConvID: conv.ID, AgentID: agentID, Provider: "claude",
		ResumeSessionID: resumeID,
		StartedAt:       time.Now().UTC(),
		Phase:           conversation.PhaseSpawned,
		Tier:            "read",
	})
	if err != nil {
		t.Fatalf("写入种子在飞记录失败: %v", err)
	}
	return conv.ID, turnID
}

// TestStartupLostTurnsAuditReportsAndOrdersBeforeDriftNotice 覆盖第 8 条的
// 正例：库里有一条在飞记录（对应一条真实的 provider 会话，settle 之后会
// 标上 baseline_suspect），启动横幅里必须出现冻结格式的第一行、N/M 都
// 正确；顺带验证契约 4.4 的顺序要求：种下的同一条 provider 会话的
// identity_fp 是固定占位值，必然触发漂移审计，于是同一次启动会打出两条
// 横幅——在飞对账的提示必须出现在漂移提示之前（AuditInflight ->
// SettleLostTurns -> writeLostNotice 排在 AuditDrift -> writeDriftNotice
// 之前）。
func TestStartupLostTurnsAuditReportsAndOrdersBeforeDriftNotice(t *testing.T) {
	isolateAppEnv(t)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := writeTestConfig(t, dir, validConfigBody(dataDir))

	if _, _, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader("")); err != nil {
		t.Fatalf("第一次 Run（建库）应当返回 nil，实际: %v", err)
	}

	const agentID = "theory"
	convID, turnID := seedInflightTurn(t, dataDir, agentID, true)

	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(""))
	if err != nil {
		t.Fatalf("第二次 Run 应当返回 nil，实际 %v；Err 输出:\n%s", err, errOut)
	}

	const wantLostLine = "注意: 有 1 轮在上次退出时没记上账，其中 1 轮已把相关成员的成本基线标为可疑"
	if !strings.Contains(out, wantLostLine) {
		t.Errorf("横幅里应当出现冻结格式的提醒行 %q；Out 完整输出:\n%s", wantLostLine, out)
	}
	if !strings.Contains(out, convID) || !strings.Contains(out, turnID) {
		t.Errorf("提醒应当点名涉及的会话 %q 与 turn %q；Out 完整输出:\n%s", convID, turnID, out)
	}

	const wantDriftLine = "注意: 有 1 条 provider 会话的身份已变"
	lostIdx := strings.Index(out, wantLostLine)
	driftIdx := strings.Index(out, wantDriftLine)
	if lostIdx == -1 || driftIdx == -1 {
		t.Fatalf("前置条件不成立：两条提醒都应当出现。lostIdx=%d driftIdx=%d；Out:\n%s", lostIdx, driftIdx, out)
	}
	if lostIdx >= driftIdx {
		t.Errorf("在飞对账提醒应当出现在漂移提醒之前（契约 4.4：settle 会写 baseline_suspect，"+
			"读排在写之前会让漂移横幅打出过期状态），实际 lostIdx=%d >= driftIdx=%d；Out:\n%s", lostIdx, driftIdx, out)
	}
}

// TestStartupLostTurnsAuditSilentWhenClean 覆盖第 8 条的反例：全新的库
// 没有任何在飞记录，横幅里不该出现「在飞」「没记上账」任何字样（契约 4.4
// 原话，照 TestStartupDriftAuditSilentWhenNoDrift 的体例）。
func TestStartupLostTurnsAuditSilentWhenClean(t *testing.T) {
	isolateAppEnv(t)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := writeTestConfig(t, dir, validConfigBody(dataDir))

	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；Err 输出:\n%s", err, errOut)
	}
	for _, forbidden := range []string{"没记上账", "在飞"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("全新的库不该有任何在飞记录，横幅里不该出现 %q；Out 完整输出:\n%s", forbidden, out)
		}
	}
}
