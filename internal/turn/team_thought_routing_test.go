// team_thought_routing_test.go 覆盖 docs/12_STEP9_CONTRACT.md 3.7 与 3.9 第 7
// 条：thought 到底有没有进送出去的字节，只由「team 的 thought_visible_to
// 是否包含收件人的 Mode」决定。两个方向共用同一份夹具——一条已经落库、
// 带非空 Thought 的历史消息，Thought 正文里嵌一个独一无二的哨兵串。
//
// 判据是送出去的字节（argv / stdin / Files），不是退出码、不是屏幕，理由
// 见 thought_policy_test.go 顶部注释与 12_STEP9_CONTRACT.md 3.7：把
// thoughtVisible 写死成恒 true 或恒 false 时，(A) 与 (B) 两支里必有一支
// 变红——这是这对用例存在的全部意义，缺一半就是"这轮本来就没有 thought"
// 的巧合能把验收整个骗过去。
//
// 覆盖范围：两条送出通路都覆盖了——claude（stdin）与 agy（argv），各自
// 都有 (A)(B) 两支，一共 4 个 case。最初一版草稿只覆盖了 claude/stdin，
// coordinator 追加要求补 argv 那半边；这里判断"夹具是否便宜"：
// seedThoughtBearingMessage 直接经 svc.Post 插入历史消息，不依赖收件人是
// 谁；agyT1JSON 这份原件已经在 fixtures_test.go 里现成躺着；newTestConfig
// 里 testAgentAgy 也已经配好 provider/permissions。补一条 agy 分支不需要
// 新造任何前提，是廉价的，所以直接补，不是硬凑。
//
// 覆盖范围的诚实说明：这条用例走的是 bootstrap 路径（收件人在这条会话里
// 还没开过口）。Select 分 bootstrap / 增量两支，但 Render——真正决定
// thought 是否附进正文的那一步——是同一份代码，两支共用（router.go 的
// routeThought 分支只在 Render 里，与 bootstrap 还是增量无关）。所以本文件
// 没有再补一条"增量路径"的双胞胎用例，不是漏项，是同一段渲染逻辑不需要
// 测两次；但没有另一条用例直接证明这句推理，算是可核查性上的一个小缺口，
// 写进任务报告。
package turn

import (
	"context"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
)

// teamThoughtSentinel 是本文件专用的哨兵串。thought_policy_test.go 里已有
// 一个包级常量 thoughtNeedle（同包，不能重名），两份哨兵各自独立、互不
// 依赖：即使那个文件将来被删，本文件也不受影响。
const teamThoughtSentinel = "TEAM-THOUGHT-VISIBLE-SENTINEL-8f3a2b91"

// seedThoughtBearingMessage 直接经 Service.Post 插入一条 KindAgent 历史
// 消息，Thought 字段带 teamThoughtSentinel，Text 字段故意不提这串文本——
// 避免"thought 被路由"与"正文本来就有这句话"两件事分不清。
//
// 不走一轮真实的 Ask + 假 Spawner 来产生这条历史消息：本用例只关心
// Router.Render 之后 thought 是否出现在送出去的字节里，不需要再验一遍
// AskUpTo 八步里与本用例无关的环节，直接插入能让用例更短、更对焦。这个
// 做法与生产写入路径（真跑一轮 Ask，见 thought_policy_test.go）是两种
// 都站得住的选择，契约没有二选一，见任务报告里的歧义一节。
func seedThoughtBearingMessage(t *testing.T, svc *conversation.Service, convID, senderID string, ac config.AgentConfig) {
	t.Helper()
	_, err := svc.Post(context.Background(), conversation.Message{
		ConvID:         convID,
		SenderID:       senderID,
		SenderProvider: ac.Provider,
		SenderModel:    ac.Model,
		Kind:           conversation.KindAgent,
		Text:           "已核对，没有问题。",
		Thought:        teamThoughtSentinel,
	})
	if err != nil {
		t.Fatalf("插入带 Thought 的历史消息失败: %v", err)
	}
}

// TestTeamThoughtVisibilityGatesOutgoingBytes 是 (A)(B) 两支、两条送出通路
// 的合体：同一份夹具，收件人分别是 claude（走 stdin）与 agy（走 argv），
// 每个收件人各自覆盖策略含/不含它的 Mode。
func TestTeamThoughtVisibilityGatesOutgoingBytes(t *testing.T) {
	cases := []struct {
		name        string
		recipient   string
		fixture     string
		visibleTo   []config.Mode
		wantPresent bool
	}{
		{
			name:        "claude(stdin) A：策略不含收件人 Mode，哨兵串不出现",
			recipient:   testAgentClaude,
			fixture:     claudeT1JSON,
			visibleTo:   []config.Mode{config.ModeManager},
			wantPresent: false,
		},
		{
			name:        "claude(stdin) B：策略含收件人 Mode，哨兵串出现",
			recipient:   testAgentClaude,
			fixture:     claudeT1JSON,
			visibleTo:   []config.Mode{config.ModeJudge},
			wantPresent: true,
		},
		{
			name:        "agy(argv) A：策略不含收件人 Mode，哨兵串不出现",
			recipient:   testAgentAgy,
			fixture:     agyT1JSON,
			visibleTo:   []config.Mode{config.ModeManager},
			wantPresent: false,
		},
		{
			name:        "agy(argv) B：策略含收件人 Mode，哨兵串出现",
			recipient:   testAgentAgy,
			fixture:     agyT1JSON,
			visibleTo:   []config.Mode{config.ModeJudge},
			wantPresent: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig()
			// 收件人的 Mode 在这份用例里固定为 judge：(A) 的策略里没有
			// judge，(B) 的策略里正好只有 judge，两支各改一处，其余字段
			// （provider/model/permissions）原封不动。
			recipient := cfg.Agents[tc.recipient]
			recipient.Mode = config.ModeJudge
			cfg.Agents[tc.recipient] = recipient

			cfg.Teams = map[string]config.TeamConfig{
				"project": {
					DisplayName:      "<project> 讨论组",
					Members:          []string{testAgentGrok, tc.recipient},
					ThoughtVisibleTo: tc.visibleTo,
				},
			}

			svc := newTestService(t)
			convID := newConv(t, svc)
			joinAgent(t, svc, convID, testAgentGrok)
			joinAgent(t, svc, convID, tc.recipient)
			if err := svc.BindTeam(context.Background(), convID, "project"); err != nil {
				t.Fatalf("BindTeam 失败: %v", err)
			}
			postUser(t, svc, convID, "请二位核对这条记录")
			seedThoughtBearingMessage(t, svc, convID, testAgentGrok, cfg.Agents[testAgentGrok])

			r := newTestRunner(t, cfg, svc)
			fs := &fakeSpawner{}
			fs.enqueueSuccess(tc.fixture)
			r.spawn = fs.spawn

			if _, err := r.Ask(context.Background(), convID, tc.recipient); err != nil {
				t.Fatalf("%s 的 Ask 失败: %v", tc.recipient, err)
			}
			if len(fs.calls) != 1 {
				t.Fatalf("期望恰好一次 spawn 调用，实际 %d 次", len(fs.calls))
			}
			cmd := fs.calls[0].cmd

			presentInArgs := false
			for i, a := range cmd.Args {
				if strings.Contains(a, teamThoughtSentinel) {
					presentInArgs = true
					t.Logf("哨兵串出现在 argv[%d]: %q", i, a)
				}
			}
			presentInStdin := strings.Contains(string(cmd.Stdin), teamThoughtSentinel)
			presentInFiles := false
			for path, content := range cmd.Files {
				if strings.Contains(string(content), teamThoughtSentinel) {
					presentInFiles = true
					t.Logf("哨兵串出现在落盘文件 %s", path)
				}
			}
			gotPresent := presentInArgs || presentInStdin || presentInFiles

			if gotPresent != tc.wantPresent {
				t.Errorf("哨兵串是否出现在送出去的字节里 = %v，期望 %v（argv 命中=%v, stdin 命中=%v, files 命中=%v）",
					gotPresent, tc.wantPresent, presentInArgs, presentInStdin, presentInFiles)
			}
		})
	}
}
