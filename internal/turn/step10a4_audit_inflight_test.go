package turn

// step10a4_audit_inflight_test.go 覆盖 Step 10 A 片之四冻结契约
// （E:\multi_model_cowork\step10a4_contract.md）第 4.3 节「OrphanID 与
// Reason 的分支（冻结措辞，两条腿都按这个）」那张表，以及 AuditInflight
// 的排序与只读性。
//
// AuditInflight 是纯读（契约：只调 r.svc.AllInflight，逐条算 Reason 与
// OrphanID，不写任何东西）。本文件用 newRecordingStore() 而不是真实
// *store.Store：这里不断言「库里某一行真的在不在」，只断言「给定一组
// InflightTurn，AuditInflight 算出来的 Reason/OrphanID 对不对」。契约给
// 测试腿的提醒点名的坑是「recordingStore.RecordUsage 是空桩，断言
// turn_usage 行会变成恒真的空断言」——AuditInflight 完全不碰 RecordUsage，
// 不落在那个坑里；本文件末尾另有一条只读性用例，直接断言 AuditInflight
// 期间 RecordUsage/AppendMessage/SetProviderSession 一次都没被调用过。
//
// 落盘时 turn.LostTurn / Runner.AuditInflight 都还不存在，编译不过是预期。

import (
	"context"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// newAuditInflightRunner 造一个用 recordingStore 撑起来的 Runner，
// 供本文件的用例直接往 rs.inflightTurns 里塞行（白盒，同包访问）。
func newAuditInflightRunner(t *testing.T) (r *Runner, rs *recordingStore, svc *conversation.Service, convID string) {
	t.Helper()
	rs = newRecordingStore()
	svc = conversation.NewService(rs)
	cfg := newTestConfig()
	convID = newConv(t, svc)
	r = newTestRunner(t, cfg, svc)
	return r, rs, svc, convID
}

// TestAuditInflightReasonAndOrphanIDPerPhase 逐条覆盖契约 4.3 节那张表的
// 前五行：begun / returned / spawned(resume非空) / spawned(仅planned非空) /
// spawned(两个id都空，点不出名字)。每一条都对 Reason 做逐字比较——契约
// 原话「冻结措辞，两条腿都按这个」，不是「大致像」。
func TestAuditInflightReasonAndOrphanIDPerPhase(t *testing.T) {
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		phase      conversation.InflightPhase
		resumeID   string
		plannedID  string
		wantOrphan string
		wantReason string
	}{
		{
			name:       "begun：进程没起",
			phase:      conversation.PhaseBegun,
			resumeID:   "should-be-ignored-resume",
			plannedID:  "should-be-ignored-planned",
			wantOrphan: "",
			wantReason: "进程没起，只是记录残留",
		},
		{
			name:       "returned：provider说过话正文没落库",
			phase:      conversation.PhaseReturned,
			resumeID:   "resume-returned-1",
			wantOrphan: "resume-returned-1",
			wantReason: "provider 确实说了话，正文没能落库",
		},
		{
			name:       "spawned且ResumeSessionID非空",
			phase:      conversation.PhaseSpawned,
			resumeID:   "resume-spawned-1",
			plannedID:  "should-be-ignored-planned",
			wantOrphan: "resume-spawned-1",
			wantReason: "起过进程，结局未知；provider 会话 resume-spawned-1 可能已经多了一轮",
		},
		{
			name:       "spawned且ResumeSessionID空PlannedSessionID非空",
			phase:      conversation.PhaseSpawned,
			plannedID:  "planned-spawned-1",
			wantOrphan: "planned-spawned-1",
			wantReason: "起过进程，结局未知；provider 侧可能有一条孤儿会话 planned-spawned-1",
		},
		{
			name:       "spawned且两个id都空点不出名字",
			phase:      conversation.PhaseSpawned,
			wantOrphan: "",
			wantReason: "起过进程，结局未知；这条 provider 会话点不出名字",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, rs, _, convID := newAuditInflightRunner(t)
			it := conversation.InflightTurn{
				TurnID:           "turn-" + tc.name,
				ConvID:           convID,
				AgentID:          "alpha",
				Provider:         "claude",
				PlannedSessionID: tc.plannedID,
				ResumeSessionID:  tc.resumeID,
				StartedAt:        base,
				Phase:            tc.phase,
				Tier:             "read",
			}
			rs.inflightTurns[it.TurnID] = it

			lost, err := r.AuditInflight(context.Background())
			if err != nil {
				t.Fatalf("AuditInflight 失败: %v", err)
			}
			if len(lost) != 1 {
				t.Fatalf("AuditInflight 返回 %d 条，期望 1", len(lost))
			}
			got := lost[0]
			if got.OrphanID != tc.wantOrphan {
				t.Errorf("OrphanID = %q，期望 %q", got.OrphanID, tc.wantOrphan)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("Reason = %q，期望 %q（冻结措辞）", got.Reason, tc.wantReason)
			}
			if got.Turn.TurnID != it.TurnID {
				t.Errorf("Turn.TurnID = %q，期望 %q（LostTurn 应当带回原始记录）", got.Turn.TurnID, it.TurnID)
			}
		})
	}
}

// TestAuditInflightIllegalPhase_FailsClosedAsSpawned 覆盖表的第六行：
// 三态之外的非法 phase 按「起过进程」fail-closed 处理。<值> 的具体展示
// 格式契约没有冻结（是 %q 还是 %v 没有明说），这里只用子串核对三个必须
// 出现的部分，不做整句的逐字比较。
func TestAuditInflightIllegalPhase_FailsClosedAsSpawned(t *testing.T) {
	r, rs, _, convID := newAuditInflightRunner(t)
	const badPhase = "quantum_superposition"
	it := conversation.InflightTurn{
		TurnID:    "turn-illegal-phase",
		ConvID:    convID,
		AgentID:   "alpha",
		Provider:  "claude",
		StartedAt: time.Now().UTC(),
		Phase:     conversation.InflightPhase(badPhase),
		Tier:      "read",
	}
	rs.inflightTurns[it.TurnID] = it

	lost, err := r.AuditInflight(context.Background())
	if err != nil {
		t.Fatalf("AuditInflight 失败: %v", err)
	}
	if len(lost) != 1 {
		t.Fatalf("AuditInflight 返回 %d 条，期望 1", len(lost))
	}
	got := lost[0]
	if got.OrphanID != "" {
		t.Errorf("OrphanID = %q，期望空串（非法 phase 没有约定取哪个 id）", got.OrphanID)
	}
	for _, want := range []string{"phase 值不合法", badPhase, "按起过进程处理"} {
		if !strings.Contains(got.Reason, want) {
			t.Errorf("Reason = %q，应当包含 %q", got.Reason, want)
		}
	}
}

// TestAuditInflightNonReadTier_AppendsWorkDirWarning 覆盖「Tier != read 时
// 追加工作目录警告」这一条，且用 begun 档证明这条追加与 phase 无关（连
// 「进程没起」这种最无害的 phase 也要追加，因为 WORKSPACE 及以上档位
// 真会写盘）。对照组用 read 档证明不追加。
func TestAuditInflightNonReadTier_AppendsWorkDirWarning(t *testing.T) {
	r, rs, _, convID := newAuditInflightRunner(t)
	const workDir = "/work/turn-workspace-1"
	it := conversation.InflightTurn{
		TurnID:    "turn-workspace-1",
		ConvID:    convID,
		AgentID:   "alpha",
		Provider:  "claude",
		StartedAt: time.Now().UTC(),
		Phase:     conversation.PhaseBegun,
		Tier:      "workspace",
		WorkDir:   workDir,
	}
	rs.inflightTurns[it.TurnID] = it

	lost, err := r.AuditInflight(context.Background())
	if err != nil {
		t.Fatalf("AuditInflight 失败: %v", err)
	}
	if len(lost) != 1 {
		t.Fatalf("AuditInflight 返回 %d 条，期望 1", len(lost))
	}
	want := "进程没起，只是记录残留；工作目录 " + workDir + " 可能有半截落盘的文件"
	if lost[0].Reason != want {
		t.Errorf("Reason = %q，期望 %q（非 read 档要追加工作目录警告，即使 phase 是 begun）", lost[0].Reason, want)
	}

	r2, rs2, _, convID2 := newAuditInflightRunner(t)
	it2 := it
	it2.TurnID = "turn-read-1"
	it2.ConvID = convID2
	it2.Tier = "read"
	rs2.inflightTurns[it2.TurnID] = it2
	lost2, err := r2.AuditInflight(context.Background())
	if err != nil {
		t.Fatalf("AuditInflight 失败: %v", err)
	}
	if len(lost2) != 1 {
		t.Fatalf("AuditInflight 返回 %d 条，期望 1", len(lost2))
	}
	if lost2[0].Reason != "进程没起，只是记录残留" {
		t.Errorf("read 档不该追加工作目录警告，实际 Reason = %q", lost2[0].Reason)
	}
}

// TestAuditInflightSortsByConvThenAgentAscending 覆盖「按 (ConvID, AgentID)
// 升序返回，输出要稳定可钉」。StartedAt 顺序故意与期望的 (ConvID, AgentID)
// 顺序相反（charlie 最早、alpha 最晚）：recordingStore.AllInflight 本身按
// started_at 升序回放，若 AuditInflight 直接照抄这个顺序、不自己再按
// (ConvID, AgentID) 排一次，这里会拿到 [charlie, bravo, alpha]，测不出
// 「没有自己排序」这种坏实现。
func TestAuditInflightSortsByConvThenAgentAscending(t *testing.T) {
	r, rs, _, convID := newAuditInflightRunner(t)
	base := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)

	rows := []conversation.InflightTurn{
		{TurnID: "turn-sort-charlie", ConvID: convID, AgentID: "charlie", Provider: "claude", StartedAt: base, Phase: conversation.PhaseBegun, Tier: "read"},
		{TurnID: "turn-sort-bravo", ConvID: convID, AgentID: "bravo", Provider: "claude", StartedAt: base.Add(time.Hour), Phase: conversation.PhaseBegun, Tier: "read"},
		{TurnID: "turn-sort-alpha", ConvID: convID, AgentID: "alpha", Provider: "claude", StartedAt: base.Add(2 * time.Hour), Phase: conversation.PhaseBegun, Tier: "read"},
	}
	for _, it := range rows {
		rs.inflightTurns[it.TurnID] = it
	}

	lost, err := r.AuditInflight(context.Background())
	if err != nil {
		t.Fatalf("AuditInflight 失败: %v", err)
	}
	if len(lost) != 3 {
		t.Fatalf("AuditInflight 返回 %d 条，期望 3", len(lost))
	}
	wantOrder := []string{"alpha", "bravo", "charlie"}
	for i, want := range wantOrder {
		if lost[i].Turn.AgentID != want {
			t.Errorf("第 %d 条 AgentID = %q，期望 %q（应当按 (ConvID, AgentID) 升序，不是按 started_at）", i, lost[i].Turn.AgentID, want)
		}
	}
}

// TestAuditInflightIsReadOnly 覆盖契约「只读，不写任何东西」。
// 断言 AuditInflight 前后 AllInflight 的条数不变，且期间没有发生任何
// AppendMessage / RecordUsage / SetProviderSession 调用。
func TestAuditInflightIsReadOnly(t *testing.T) {
	r, rs, svc, convID := newAuditInflightRunner(t)
	it := conversation.InflightTurn{
		TurnID: "turn-readonly-1", ConvID: convID, AgentID: "alpha", Provider: "claude",
		StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, ResumeSessionID: "resume-readonly-1", Tier: "read",
	}
	rs.inflightTurns[it.TurnID] = it

	beforeInflight, err := svc.AllInflight(context.Background())
	if err != nil {
		t.Fatalf("AllInflight 失败: %v", err)
	}
	beforeMsgCount := len(rs.appendMessageCalls)
	beforeUsageCount := len(rs.recordUsageCalls)
	beforeSessionCount := len(rs.setProviderSessionCalls)

	if _, err := r.AuditInflight(context.Background()); err != nil {
		t.Fatalf("AuditInflight 失败: %v", err)
	}

	afterInflight, err := svc.AllInflight(context.Background())
	if err != nil {
		t.Fatalf("AllInflight 失败: %v", err)
	}
	if len(afterInflight) != len(beforeInflight) {
		t.Errorf("AuditInflight 前后 AllInflight 条数变了：%d -> %d，期望不变（纯读）", len(beforeInflight), len(afterInflight))
	}
	if len(rs.appendMessageCalls) != beforeMsgCount {
		t.Error("AuditInflight 期间发生了 AppendMessage 调用，期望 0 次（纯读，不该发面包屑）")
	}
	if len(rs.recordUsageCalls) != beforeUsageCount {
		t.Error("AuditInflight 期间发生了 RecordUsage 调用，期望 0 次")
	}
	if len(rs.setProviderSessionCalls) != beforeSessionCount {
		t.Error("AuditInflight 期间发生了 SetProviderSession 调用，期望 0 次")
	}
}
