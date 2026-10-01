package turn

// 本文件覆盖验收 4（AskUpTo 的上界钉住游标与投递范围）与验收 8
// （Ask 铸出的 RoundID 非空且逐轮不同）。这两条都不需要并发，用现成的
// 单线程 fakeSpawner（fixtures_test.go）就够，不用 gate_concurrency_test.go
// 里那个并发安全的 raceSpawner。
//
// 背对背纪律同 gate_concurrency_test.go：只依赖任务书冻结的形状
// （AskUpTo(ctx, convID, agentID, upTo, roundID)、Record.RoundID），
// 这些符号此刻还不存在，编译不过是预期状态。

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 验收 4：AskUpTo(upTo) 钉住 ScannedTo 与投递范围，不带上界的下一轮补上剩下的
// ---------------------------------------------------------------------------

func TestAskUpToCapsDeliveryAndUnboundedRoundSeesRest(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)

	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	// 用真实回来的 Seq 而不是硬编码字面量 7/10：newConv/joinAgent 会不会
	// 消耗 seq 是这个包的实现细节，不该让本用例的断言建立在「第一条
	// postUser 一定是 seq=1」这个假设上——用返回值本身就不依赖这个假设。
	seqs := make([]int64, 0, 10)
	for i := 1; i <= 10; i++ {
		m := postUser(t, svc, convID, fmt.Sprintf("msg-%02d", i))
		seqs = append(seqs, m.Seq)
	}
	upTo := seqs[6] // 第 7 条消息的 seq，对应验收原文的「upTo=7」

	fs.enqueueSuccess(minimalClaudeSuccess("SESSION-A", "resp-1"))
	rec1, err1 := r.AskUpTo(context.Background(), convID, testAgentClaude, upTo, "round-cap")
	if err1 != nil {
		t.Fatalf("AskUpTo(upTo=%d) 失败: %v", upTo, err1)
	}
	if rec1.ScannedTo != upTo {
		t.Fatalf("rec1.ScannedTo = %d，期望等于 upTo = %d——upTo 没有钉住游标", rec1.ScannedTo, upTo)
	}
	if rec1.RoundID != "round-cap" {
		t.Fatalf("rec1.RoundID = %q，期望原样等于调用时传入的 %q——"+
			"AskUpTo 必须把调用方传入的 roundID 落进 Record，不能自己另铸一个", rec1.RoundID, "round-cap")
	}
	if len(fs.calls) != 1 {
		t.Fatalf("期望恰好一次 Spawner 调用，got %d", len(fs.calls))
	}
	sentPrompt1 := string(fs.calls[0].cmd.Stdin)
	for i := 8; i <= 10; i++ {
		marker := fmt.Sprintf("msg-%02d", i)
		if strings.Contains(sentPrompt1, marker) {
			t.Fatalf("upTo=%d 的这一轮 prompt 里出现了 %q，本不该越过上界投递", upTo, marker)
		}
	}
	for i := 1; i <= 7; i++ {
		marker := fmt.Sprintf("msg-%02d", i)
		if !strings.Contains(sentPrompt1, marker) {
			t.Fatalf("upTo=%d 的这一轮 prompt 里缺了 %q，上界之内的内容不该被漏发", upTo, marker)
		}
	}

	// 不带上界再跑一轮：8-10 应该出现在这一轮的 delta 里。
	fs.enqueueSuccess(minimalClaudeSuccess("SESSION-A", "resp-2"))
	rec2, err2 := r.Ask(context.Background(), convID, testAgentClaude)
	if err2 != nil {
		t.Fatalf("Ask（不带上界）失败: %v", err2)
	}
	if len(rec2.Delivered) != 3 {
		t.Fatalf("rec2.Delivered = %v，期望恰好 3 条（seq 8-10）", rec2.Delivered)
	}
	wantDelivered := []int64{seqs[7], seqs[8], seqs[9]}
	for i, want := range wantDelivered {
		if rec2.Delivered[i] != want {
			t.Fatalf("rec2.Delivered[%d] = %d，期望 %d（完整序列 got=%v want=%v）",
				i, rec2.Delivered[i], want, rec2.Delivered, wantDelivered)
		}
	}
	if len(fs.calls) != 2 {
		t.Fatalf("期望累计两次 Spawner 调用，got %d", len(fs.calls))
	}
	sentPrompt2 := string(fs.calls[1].cmd.Stdin)
	for i := 8; i <= 10; i++ {
		marker := fmt.Sprintf("msg-%02d", i)
		if !strings.Contains(sentPrompt2, marker) {
			t.Fatalf("第二轮（不带上界）prompt 里缺了 %q", marker)
		}
	}
	for i := 1; i <= 7; i++ {
		marker := fmt.Sprintf("msg-%02d", i)
		if strings.Contains(sentPrompt2, marker) {
			t.Fatalf("第二轮（不带上界）prompt 里重复出现了已经在第一轮投递过的 %q"+
				"——增量路径应该只送 cursor 之后的新内容", marker)
		}
	}
}

// ---------------------------------------------------------------------------
// 验收 8（契约没列、补的一条）：Ask 铸的 roundID 必须非空，且两次 Ask
// 铸出的 RoundID 互不相同
// ---------------------------------------------------------------------------

func TestAskMintsNonEmptyDistinctRoundIDs(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)

	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	postUser(t, svc, convID, "round-1-msg")
	fs.enqueueSuccess(minimalClaudeSuccess("SESSION-A", "resp-1"))
	rec1, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("round1 Ask 失败: %v", err)
	}
	if rec1.RoundID == "" {
		t.Fatalf("Ask 铸的 RoundID 是空串——将来 turn_usage 加 round_id 列时，" +
			"这一行会退化成没有 round_id 的孤儿行，与「/ask 也是一轮之一」的裁法不符")
	}

	postUser(t, svc, convID, "round-2-msg")
	fs.enqueueSuccess(minimalClaudeSuccess("SESSION-A", "resp-2"))
	rec2, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("round2 Ask 失败: %v", err)
	}
	if rec2.RoundID == "" {
		t.Fatalf("round2 的 RoundID 也是空串")
	}
	if rec1.RoundID == rec2.RoundID {
		t.Fatalf("两轮 Ask 铸出了相同的 RoundID = %q，roundID 必须逐轮互不相同", rec1.RoundID)
	}
}
