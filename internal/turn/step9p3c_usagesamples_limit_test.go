package turn

// 本文件补 UsageSamples 的 limit 参数语义——上一条 commit 的披露原话："limit
// 参数语义一条用例都没有"：既有的 step9p3c_budget_usage_test.go 全程只传
// limit=10，从没构造过"候选数超过 limit"的截断场景，也没验过 limit<=0。
// 本文件只关这一个缺口，不为 step9p3c_budget_usage_test.go 已经盯住的
// (agent_id, identity_fp) 分组、OutcomePostRejected 算样本、input_tokens
// 为 NULL 不算这几条另开专门用例——那几条契约由每条用例的噪声行、以及
// 用例 4 的 fixture（rZero / rRejected）顺带覆盖，不是没测到。
//
// 唯一权威是 internal/conversation/store.go 上 UsageSamples 方法的接口
// 注释，不是读实现之后反推断言：
//
//	limit > 0 最多取这么多条。limit == 0 是"要零条"，返回空切片。
//	limit < 0 不限。空结果是空切片不是 nil。
//	created_at 倒序、turn_id 倒序作第二键。
//
// 造数据全程复用 step9p3c_budget_usage_test.go 已有的 p3cIntPtr /
// p3cInsertSample / newTestServiceWithPath / newConv，不另起一套；那个
// 文件里的辅助函数够用（p3cInsertSample 接收完整的 TurnUsage，CreatedAt
// 与 TurnID 都能自由摆），不需要在本文件另加本地辅助函数。

import (
	"context"
	"testing"
	"time"

	"council/internal/conversation"
)

// TestUsageSamplesLimitTruncatesToNewestByCreatedAt 防住"limit>0 时截断
// 留下的不是最新的那几条"这一类坏实现——包括"按插入顺序（rowid）截"、
// "按 created_at 升序截"这两种最容易冒充"按 created_at 降序截"的错法。
//
// 5 条候选 created_at 偏移 +2、+0、+4、+1、+3 分钟，插入顺序就按这个次序
// （既非升序也非降序）：如果实现偷懒用 rowid 当排序键，rowid 降序的前两名
// 会是偏移 +3、+1 那两条（对应插入顺序里最后两个），与真正按 created_at
// 降序应得的 +4、+3 两条在第一位就不一样；rowid 升序前两名是 +2、+0，
// 同样不一样。三种"坏实现"的输出互不相同，本用例的插入顺序能把它们都
// 分辨出来。
func TestUsageSamplesLimitTruncatesToNewestByCreatedAt(t *testing.T) {
	svc, _ := newTestServiceWithPath(t)
	conv := newConv(t, svc)

	const agentID = "p3c-lim-agent-1"
	const fp = "p3c-lim-fp-1"
	base := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)

	mk := func(offsetMin int) conversation.TurnUsage {
		return conversation.TurnUsage{
			ConvID: conv, AgentID: agentID, IdentityFP: fp,
			Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(100), PromptBytes: 200,
			CreatedAt: base.Add(time.Duration(offsetMin) * time.Minute),
		}
	}

	p3cInsertSample(t, svc, mk(2))
	p3cInsertSample(t, svc, mk(0))
	row4 := p3cInsertSample(t, svc, mk(4))
	p3cInsertSample(t, svc, mk(1))
	row3 := p3cInsertSample(t, svc, mk(3))

	// 噪声行：同 agent、另一份指纹，input_tokens 非 NULL（不靠 NULL 规则被
	// 排除），created_at 是全集里最新的一条——指纹过滤若失效，它会顶到
	// got[0]，下面的位置断言立刻能抓到，不会被"len 对了但凑巧"蒙混过去。
	noise := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: "p3c-lim-fp-1-noise",
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(999), PromptBytes: 999,
		CreatedAt: base.Add(10 * time.Minute),
	})

	ctx := context.Background()
	got, err := svc.UsageSamples(ctx, agentID, fp, 2)
	if err != nil {
		t.Fatalf("UsageSamples 失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit=2、候选 5 条，应当恰好截断成 2 条，实际 %d 条：%+v", len(got), got)
	}
	// 逐个 turn_id 点名，不只是 len(got)==2：防住"截断了但截错了"。
	if got[0].TurnID != row4.TurnID {
		t.Errorf("got[0] 应当是 created_at 最新的那条 row4（%q），实际 %q", row4.TurnID, got[0].TurnID)
	}
	if got[1].TurnID != row3.TurnID {
		t.Errorf("got[1] 应当是 created_at 次新的那条 row3（%q），实际 %q", row3.TurnID, got[1].TurnID)
	}
	for _, u := range got {
		if u.TurnID == noise.TurnID {
			t.Errorf("噪声行（另一份指纹）不该出现在结果里，它的 created_at 是全集最新的，"+
				"出现说明指纹过滤失效: %+v", u)
		}
	}
}

// TestUsageSamplesLimitTieBreakByTurnIDDescAlsoTruncates 证明第二排序键
// （created_at 相同时按 turn_id 倒序）不只影响"排序"，还影响"截断时留下
// 谁"——一个只在"输出顺序"上用 turn_id、但截断阶段仍按插入顺序/rowid
// 挑选的实现，会在这条用例上露馅。
//
// 3 条候选 created_at 完全相同，turn_id 是可比较的字符串
// p3c-lim-tie-a < p3c-lim-tie-b < p3c-lim-tie-c。插入顺序是 b、c、a——
// 最大的 c 不是最后插入的，防住"插入顺序恰好等于 turn_id 顺序"这种巧合。
// 真按 turn_id 降序取前 2 应当是 c、b；若实现按 rowid 降序取前 2，会是
// a（最后插入，rowid 最大）、c，与真答案在第一位就不一样。
func TestUsageSamplesLimitTieBreakByTurnIDDescAlsoTruncates(t *testing.T) {
	svc, _ := newTestServiceWithPath(t)
	conv := newConv(t, svc)

	const agentID = "p3c-lim-agent-2"
	const fp = "p3c-lim-fp-2"
	same := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	mk := func(turnID string) conversation.TurnUsage {
		return conversation.TurnUsage{
			TurnID: turnID,
			ConvID: conv, AgentID: agentID, IdentityFP: fp,
			Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(150), PromptBytes: 300,
			CreatedAt: same,
		}
	}

	rowB := p3cInsertSample(t, svc, mk("p3c-lim-tie-b"))
	rowC := p3cInsertSample(t, svc, mk("p3c-lim-tie-c"))
	p3cInsertSample(t, svc, mk("p3c-lim-tie-a"))

	// 噪声行：另一个 agent_id（这条用另一个维度覆盖"指纹进键"要求的另一半——
	// (agent_id, identity_fp) 两个字段都要进键，不是只测过 identity_fp）。
	// turn_id 故意取字母序最大的 "z"，created_at 与三条真候选相同：
	// agent_id 过滤若失效，它会在 turn_id 倒序里排到 got[0]。
	noise := p3cInsertSample(t, svc, conversation.TurnUsage{
		TurnID: "p3c-lim-tie-z",
		ConvID: conv, AgentID: "p3c-lim-agent-2-noise", IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(150), PromptBytes: 300,
		CreatedAt: same,
	})

	ctx := context.Background()
	got, err := svc.UsageSamples(ctx, agentID, fp, 2)
	if err != nil {
		t.Fatalf("UsageSamples 失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit=2、候选 3 条 created_at 相同，应当截断成 2 条，实际 %d 条：%+v", len(got), got)
	}
	if got[0].TurnID != rowC.TurnID {
		t.Errorf("created_at 相同时应当按 turn_id 倒序，got[0] 应当是 %q，实际 %q", rowC.TurnID, got[0].TurnID)
	}
	if got[1].TurnID != rowB.TurnID {
		t.Errorf("got[1] 应当是 turn_id 第二大的那条 %q，实际 %q——"+
			"这条专门证明第二排序键不只影响顺序，还影响截断留下谁", rowB.TurnID, got[1].TurnID)
	}
	for _, u := range got {
		if u.TurnID == noise.TurnID {
			t.Errorf("噪声行（另一个 agent_id，turn_id 字母序最大）不该出现在结果里，"+
				"出现说明 agent_id 过滤失效: %+v", u)
		}
	}
}

// TestUsageSamplesLimitZeroReturnsEmptyNotNil 钉住 limit==0 的两条要求：
// 返回长度为 0，且返回值是空切片不是 nil。候选必须真实存在——没有候选时
// 谁都能返回空，测不出实现是不是真的读了 limit==0 这个分支。
func TestUsageSamplesLimitZeroReturnsEmptyNotNil(t *testing.T) {
	svc, _ := newTestServiceWithPath(t)
	conv := newConv(t, svc)

	const agentID = "p3c-lim-agent-3"
	const fp = "p3c-lim-fp-3"
	base := time.Date(2026, 9, 21, 11, 0, 0, 0, time.UTC)

	p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(100), PromptBytes: 200,
		CreatedAt: base,
	})
	p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(100), PromptBytes: 200,
		CreatedAt: base.Add(time.Minute),
	})
	// 噪声行照样造一条，即便 limit==0 时结果恒为空、"不在结果里"这条断言
	// 天然满足——保持四条用例都覆盖"指纹/agent 进键"这条要求的一致性，
	// 也顺带证明噪声行的存在不会让 limit==0 分支出错或 panic。
	noise := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: "p3c-lim-fp-3-noise",
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(100), PromptBytes: 200,
		CreatedAt: base.Add(2 * time.Minute),
	})

	ctx := context.Background()
	got, err := svc.UsageSamples(ctx, agentID, fp, 0)
	if err != nil {
		t.Fatalf("UsageSamples(limit=0) 应当不报错，实际: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("limit=0 应当返回 0 条，实际 %d 条：%+v", len(got), got)
	}
	// 单独断言非 nil，与上面的长度检查分开：契约原文"空结果是空切片不是
	// nil"。一个写成 `var out []TurnUsage; ...; return out, nil` 的实现在
	// limit==0 分支直接早退时会在这里返回 nil——len(got)==0 对 nil 切片
	// 和空切片（[]TurnUsage{}）都成立，唯有单独判 got == nil 才能把两者
	// 分开，调用方对 nil 与空切片可能有不同处理（例如序列化成 null 而不
	// 是 []）。
	if got == nil {
		t.Error("UsageSamples(limit=0) 返回了 nil，契约要求空切片")
	}
	for _, u := range got {
		if u.TurnID == noise.TurnID {
			t.Errorf("噪声行不该出现在结果里: %+v", u)
		}
	}
}

// TestUsageSamplesLimitNegativeUnlimitedVsPositiveCap 钉住 limit<0"不限"，
// 并用同一份候选集合、同一次调用后再传 limit=3 做差分：防住"limit<0 与
// limit>0 共用同一条『不限』路径"的坏实现（例如把 limit<0 误判成一个开关，
// 从此忽略所有 limit，而不是只在 limit<0 时不限）。
//
// 4 条候选顺带钉住两条样本口径（不必另开用例）：rZero 的 prompt_bytes==0
// 仍要返回；rRejected 的 Outcome 是 OutcomePostRejected 仍要算样本。
// created_at 偏移 +2、+4、+1、+3 分钟，插入顺序按这个次序（非升非降）：
// rowid 降序取前 3 会是 rRejected、rZero、rNewest，与真答案
// rNewest、rRejected、rMid 不一样，同样能分辨出"按 rowid 排"的坏实现。
func TestUsageSamplesLimitNegativeUnlimitedVsPositiveCap(t *testing.T) {
	svc, _ := newTestServiceWithPath(t)
	conv := newConv(t, svc)

	const agentID = "p3c-lim-agent-4"
	const fp = "p3c-lim-fp-4"
	base := time.Date(2026, 9, 21, 13, 0, 0, 0, time.UTC)

	rMid := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(600), PromptBytes: 1200,
		CreatedAt: base.Add(2 * time.Minute),
	})
	rNewest := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(500), PromptBytes: 1000,
		CreatedAt: base.Add(4 * time.Minute),
	})
	rZero := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(100), PromptBytes: 0,
		CreatedAt: base.Add(1 * time.Minute),
	})
	rRejected := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: fp,
		Outcome: int(OutcomePostRejected), InputTokens: p3cIntPtr(400), PromptBytes: 800,
		CreatedAt: base.Add(3 * time.Minute),
	})

	// 噪声行：同 agent、另一份指纹，created_at 是全集最新——指纹过滤若
	// 失效，它会顶到 gotAll[0]。
	noise := p3cInsertSample(t, svc, conversation.TurnUsage{
		ConvID: conv, AgentID: agentID, IdentityFP: "p3c-lim-fp-4-noise",
		Outcome: int(OutcomeSpoke), InputTokens: p3cIntPtr(999), PromptBytes: 999,
		CreatedAt: base.Add(10 * time.Minute),
	})

	ctx := context.Background()

	// limit<0：不限，4 条全回来，且仍按 created_at 倒序完整排列。
	gotAll, err := svc.UsageSamples(ctx, agentID, fp, -1)
	if err != nil {
		t.Fatalf("UsageSamples(limit=-1) 失败: %v", err)
	}
	if len(gotAll) != 4 {
		t.Fatalf("limit<0 应当不限、候选 4 条全回来，实际 %d 条：%+v", len(gotAll), gotAll)
	}
	wantOrder := []conversation.TurnUsage{rNewest, rRejected, rMid, rZero}
	for i, want := range wantOrder {
		if gotAll[i].TurnID != want.TurnID {
			t.Errorf("limit=-1 第 %d 位应当是 %q，实际 %q（完整顺序应为 created_at 倒序）",
				i, want.TurnID, gotAll[i].TurnID)
		}
	}
	for _, u := range gotAll {
		if u.TurnID == noise.TurnID {
			t.Errorf("噪声行（另一份指纹，created_at 最新）不该出现在 limit=-1 的结果里: %+v", u)
		}
		if u.TurnID == rZero.TurnID && u.PromptBytes != 0 {
			t.Errorf("rZero 的 prompt_bytes 应当原样是 0，实际 %d——prompt_bytes==0 的行"+
				"仍是样本，不该被悄悄丢弃或改写", u.PromptBytes)
		}
	}

	// 差分：同一份候选，limit=3 应当只回 3 条。
	gotCapped, err := svc.UsageSamples(ctx, agentID, fp, 3)
	if err != nil {
		t.Fatalf("UsageSamples(limit=3) 失败: %v", err)
	}
	if len(gotCapped) != 3 {
		t.Fatalf("limit=3 应当恰好截断成 3 条，实际 %d 条：%+v——"+
			"若这里仍是 4 条，说明 limit<0 与 limit>0 共用了同一条『不限』路径",
			len(gotCapped), gotCapped)
	}
	wantCapped := []conversation.TurnUsage{rNewest, rRejected, rMid}
	for i, want := range wantCapped {
		if gotCapped[i].TurnID != want.TurnID {
			t.Errorf("limit=3 第 %d 位应当是 %q，实际 %q", i, want.TurnID, gotCapped[i].TurnID)
		}
	}
}
