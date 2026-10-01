package turn

// SM1 §2.2：总结成员的回复只给用户看 —— 送进其余成员子进程的 stdin 里没有它。
// 用既有 fakeSpawner，不起进程；不新增 TestMain，也不依赖本包 MD1 的那个 TestMain。
// 只引用本刀新增的 Config.Summary，不依赖 CO1；在没有本刀的 HEAD 上编译不过是预期的红。

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestSmxSummaryMessageUserOnly 是 P1：configured（Summary.Agent = sm）与 control（没配）两个子测试逐步相同。
//
// 【判别力】增量（alice 续会话）与 bootstrap（late 首次）两条路都覆盖，且断言的是真正送进子进程的 stdin 字节；
// control 证明同一装置下总结文本本来会进 prompt（configured 里「没有它」这条断言因此能为假）；
// 步骤 2 的 Recipients 钉「按配置给总结成员填 [自己]」（写成 nil、["user"] 或条件取反时红）；
// 步骤 3 的 Recipients 长度 0 钉「只给总结成员填，别人不填」（条件取反时 alice 的消息被填，红）。
func TestSmxSummaryMessageUserOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		summary    string
		srecRcpt   []string
		aDelivered []int64
		aStdin     string
		lDelivered []int64
		lStdin     string
	}{
		{
			name:       "configured",
			summary:    "sm",
			srecRcpt:   []string{"sm"},
			aDelivered: []int64{6},
			aStdin:     "user: q2",
			lDelivered: []int64{1, 6, 7},
			lStdin:     "turn 包测试会话\n\nuser: q1\n\nuser: q2\n\nalice: OK",
		},
		{
			name:       "control",
			summary:    "",
			srecRcpt:   nil,
			aDelivered: []int64{5, 6},
			aStdin:     "sm: smx-summary-text\n\nuser: q2",
			lDelivered: []int64{1, 5, 6, 7},
			lStdin:     "turn 包测试会话\n\nuser: q1\n\nsm: smx-summary-text\n\nuser: q2\n\nalice: OK",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := newTestConfig()
			cfg.Agents["sm"] = cfg.Agents[testAgentClaude]
			cfg.Agents["late"] = cfg.Agents[testAgentClaude]
			cfg.Summary.Agent = tc.summary
			svc := newTestService(t)
			conv := newConv(t, svc)
			joinAgent(t, svc, conv, "alice")
			joinAgent(t, svc, conv, "sm")
			joinAgent(t, svc, conv, "late")
			r := newTestRunner(t, cfg, svc)
			fs := &fakeSpawner{}
			r.spawn = fs.spawn
			lastStdin := func() string {
				t.Helper()
				if len(fs.calls) == 0 {
					t.Fatalf("前提不成立：fakeSpawner 一次都没被调用")
				}
				return string(fs.calls[len(fs.calls)-1].cmd.Stdin)
			}

			// 步骤 1：alice 首轮。
			postUser(t, svc, conv, "q1")
			fs.enqueueSuccess(claudeT1JSON)
			if rec, err := r.Ask(ctx, conv, "alice"); err != nil {
				t.Fatalf("前提不成立：步骤 1 alice 失败: %v；Record=%+v", err, rec)
			}

			// 步骤 2：总结成员说一句。
			fs.enqueueSuccess(strings.Replace(claudeT1JSON, "\"result\":\"OK\"", "\"result\":\"smx-summary-text\"", 1))
			srec, err := r.Ask(ctx, conv, "sm")
			if err != nil || srec.Message == nil {
				t.Fatalf("前提不成立：步骤 2 sm 失败: %v；Record=%+v", err, srec)
			}
			if srec.Message.Seq != 5 {
				t.Fatalf("前提不成立：总结消息的 seq 应当是 5，实际 %d", srec.Message.Seq)
			}
			if tc.srecRcpt == nil {
				if len(srec.Message.Recipients) != 0 {
					t.Fatalf("没配时总结成员的消息不应当带 Recipients，实际 %q", srec.Message.Recipients)
				}
			} else if !reflect.DeepEqual(srec.Message.Recipients, tc.srecRcpt) {
				t.Fatalf("总结消息的 Recipients 应当是 %q，实际 %q", tc.srecRcpt, srec.Message.Recipients)
			}

			// 步骤 3：alice 续会话（增量路径）。
			postUser(t, svc, conv, "q2")
			fs.enqueueSuccess(claudeT1JSON)
			arec, err := r.Ask(ctx, conv, "alice")
			if err != nil || arec.Message == nil {
				t.Fatalf("前提不成立：步骤 3 alice 失败: %v；Record=%+v", err, arec)
			}
			if !reflect.DeepEqual(arec.Delivered, tc.aDelivered) {
				t.Fatalf("步骤 3 alice 的 Delivered 应当是 %v，实际 %v", tc.aDelivered, arec.Delivered)
			}
			if got := lastStdin(); got != tc.aStdin {
				t.Fatalf("步骤 3 送进 alice 子进程的 stdin 不对\n实际: %q\n期望: %q", got, tc.aStdin)
			}
			if len(arec.Message.Recipients) != 0 {
				t.Fatalf("alice 的消息不应当带 Recipients，实际 %q", arec.Message.Recipients)
			}

			// 步骤 4：late 首次开口（bootstrap 路径）。
			fs.enqueueSuccess(claudeT1JSON)
			lrec, err := r.Ask(ctx, conv, "late")
			if err != nil {
				t.Fatalf("前提不成立：步骤 4 late 失败: %v；Record=%+v", err, lrec)
			}
			if !reflect.DeepEqual(lrec.Delivered, tc.lDelivered) {
				t.Fatalf("步骤 4 late 的 Delivered 应当是 %v，实际 %v", tc.lDelivered, lrec.Delivered)
			}
			if got := lastStdin(); got != tc.lStdin {
				t.Fatalf("步骤 4 送进 late 子进程的 stdin 不对\n实际: %q\n期望: %q", got, tc.lStdin)
			}
		})
	}
}
