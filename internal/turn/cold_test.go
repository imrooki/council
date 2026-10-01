package turn

// 本文件覆盖 K 节「TTL」组与「冷暖」相关的 Runner 级行为：ColdExpected 的
// spawn 前预报、TTL 过期不改变会话本身、以及 Cold 的判据是
// Usage.CacheRead==0 而不是 cache_create>0（用 grok 的真实字节演示这条区分，
// 09 F 节点名的那个反例本身就是 grok：cache_creation_input_tokens 恒为 0，
// 而它的 cache_read 明明从 0 涨到 20608）。

import (
	"context"
	"testing"
	"time"
)

// TestColdExpectedTrueOnFreshSession 覆盖首轮：新会话恒为 ColdExpected=true。
func TestColdExpectedTrueOnFreshSession(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if !rec.ColdExpected {
		t.Error("首轮 ColdExpected 应当为真")
	}
}

// TestColdExpectedAfterTTLDoesNotRebuildSession 覆盖 09 G 节「TTL 过期不是
// 失败，也不是重建」：用可控时钟把 lastTurnAt 推到超过 CacheTTL（1 小时）
// 之后，下一轮 ColdExpected=true，但 ProviderSession 不变、Fresh=false，
// 而且真正发给 Spawner 的命令行里仍然是 --resume（不是重新起一条会话）。
func TestColdExpectedAfterTTLDoesNotRebuildSession(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	clock := newFakeClock(time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC))
	r.now = clock.now
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec1, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	firstSession := rec1.ProviderSession
	if firstSession == "" {
		t.Fatal("前置条件不成立：首轮应当拿到一个非空的 ProviderSession")
	}

	// 把时钟拨到 61 分钟之后，超过 CacheTTL。
	clock.advance(61 * time.Minute)
	postUser(t, svc, convID, "第二轮，距上一轮已过 TTL")
	fs.enqueueSuccess(claudeT2JSON)

	rec2, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}
	if !rec2.ColdExpected {
		t.Error("距上一轮已过 1 小时，第二轮 ColdExpected 应当为真")
	}
	if rec2.Fresh {
		t.Error("TTL 过期不是会话过期：第二轮 Fresh 应当为假")
	}
	if rec2.ProviderSession != firstSession {
		t.Errorf("TTL 过期不该换会话 id：第二轮 ProviderSession=%q，期望仍是 %q", rec2.ProviderSession, firstSession)
	}

	if len(fs.calls) != 2 {
		t.Fatalf("期望 Spawner 被调用 2 次，实际 %d 次", len(fs.calls))
	}
	args := fs.calls[1].cmd.Args
	foundResume := false
	for i, a := range args {
		if a == "--resume" && i+1 < len(args) && args[i+1] == firstSession {
			foundResume = true
			break
		}
	}
	if !foundResume {
		t.Errorf("第二轮命令行里没有找到 --resume %s，实际 args=%v（TTL 过期只让这一轮贵，不改变 --resume 的事实）", firstSession, args)
	}
}

// TestColdJudgedByCacheReadNotCacheCreate 覆盖 05 第 3 节陷阱 5 与 09 F 节：
// Cold 的判据只能是 Usage.CacheRead==0，不能用 cache_create>0 反推——
// grok 是这条规则的实测反例本身：cache_creation_input_tokens 在 t1 与 t2
// 都是 0，而 t2 的 cache_read_input_tokens 明明从 0 涨到了 20608。
// 用真实的 grok-t1（CacheRead=0）与 grok-t2（CacheRead=20608）两份字节
// 直接验证 Cold 的真值随 CacheRead 变化，而不是随 CacheCreate 变化
// （两轮的 CacheCreate 归一化之后都是 0，若判据认错了字段，t2 也会被
// 错误地判成冷启动）。
func TestColdJudgedByCacheReadNotCacheCreate(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)
	postUser(t, svc, convID, "第一轮")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(grokT1JSON)
	r.spawn = fs.spawn

	rec1, err := r.Ask(context.Background(), convID, testAgentGrok)
	if err != nil {
		t.Fatalf("第一轮失败: %v", err)
	}
	if !rec1.Cold {
		t.Error("grok 首轮 CacheRead=0，Cold 应当为真")
	}
	if rec1.Usage.CacheCreate == nil || *rec1.Usage.CacheCreate != 0 {
		t.Fatalf("前置条件不成立：grok 首轮 CacheCreate 应当是 0，实际 %+v", rec1.Usage.CacheCreate)
	}

	postUser(t, svc, convID, "第二轮")
	fs.enqueueSuccess(grokT2JSON)
	rec2, err := r.Ask(context.Background(), convID, testAgentGrok)
	if err != nil {
		t.Fatalf("第二轮失败: %v", err)
	}
	if rec2.Usage.CacheCreate == nil || *rec2.Usage.CacheCreate != 0 {
		t.Fatalf("前置条件不成立：grok 第二轮 CacheCreate 也应当仍是 0（陷阱 7），实际 %+v", rec2.Usage.CacheCreate)
	}
	if rec2.Usage.CacheRead != 20608 {
		t.Fatalf("前置条件不成立：grok 第二轮 CacheRead 应当是 20608，实际 %d", rec2.Usage.CacheRead)
	}
	if rec2.Cold {
		t.Error("grok 第二轮 CacheRead=20608（非零），Cold 应当为假；如果这里为真，说明判据被误接成了 CacheCreate>0 那一支")
	}
}
