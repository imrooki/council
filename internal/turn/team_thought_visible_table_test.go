// team_thought_visible_table_test.go 覆盖 docs/12_STEP9_CONTRACT.md 3.9 第 1
// 条里点到的 thoughtVisible 本身：它是纯函数，钉死输入到输出的映射，
// 不依赖 Runner、不依赖库、不依赖任何 I/O。
//
// 冻结形状：func thoughtVisible(team config.TeamConfig, to config.Mode) bool
// （任务书原话，包内私有，测试在 package turn 里够得到）。
//
// 覆盖缺口，写在这里免得漏项：contract 3.6 的判别器实际上是
// thoughtVisibleFor(teamID string, to config.Mode, cfg *config.Config)
// （未决 #25 之后是三参；第三参 cfg 是调用方持有的那一份轮内快照，本
// 函数不再读 r.cfg——轮中途再读一次会让同一轮里 Teams 与
// Agents/Providers 来自两份配置。在 conv.TeamID == "" 时直接短路成
// false），那个短路发生在 thoughtVisible 之外的包装层——thoughtVisible 的
// 签名只收 TeamConfig，天生不认识 "team 没绑" 这件事，所以本文件的表驱动
// 结构性地覆盖不到 conv.TeamID == "" 这一支。那一支目前的覆盖来自
// thought_policy_test.go 的 TestRouterProductionPathDoesNotLeakThoughtIntoOutgoingBytes
// （未绑 team 的会话 + 生产路径恒传 NewRouter(false)）——但那条用例的本意
// 是验证生产路径的默认值，不是专门为这一支写的，属于"顺带覆盖"，不是
// "有意覆盖"。见任务报告里的覆盖缺口一节。
package turn

import (
	"testing"

	"council/internal/config"
)

func TestThoughtVisibleTableDriven(t *testing.T) {
	tests := []struct {
		name string
		team config.TeamConfig
		to   config.Mode
		want bool
	}{
		{
			name: "空切片：谁都看不到",
			team: config.TeamConfig{ThoughtVisibleTo: []config.Mode{}},
			to:   config.ModeJudge,
			want: false,
		},
		{
			name: "nil 切片（TeamConfig 零值的默认状态）：谁都看不到",
			team: config.TeamConfig{},
			to:   config.ModeJudge,
			want: false,
		},
		{
			name: "恰好含该 mode：可见",
			team: config.TeamConfig{ThoughtVisibleTo: []config.Mode{config.ModeJudge}},
			to:   config.ModeJudge,
			want: true,
		},
		{
			name: "只含别的 mode，不含该 mode：不可见",
			team: config.TeamConfig{ThoughtVisibleTo: []config.Mode{config.ModeManager}},
			to:   config.ModeJudge,
			want: false,
		},
		{
			name: "含多个 mode，其中一个匹配：可见",
			team: config.TeamConfig{ThoughtVisibleTo: []config.Mode{
				config.ModeWorker, config.ModeJudge, config.ModeManager,
			}},
			to:   config.ModeJudge,
			want: true,
		},
		{
			name: "含多个 mode，全都不匹配：不可见",
			team: config.TeamConfig{ThoughtVisibleTo: []config.Mode{
				config.ModeWorker, config.ModeManager, config.ModeDiscussionAndWorker,
			}},
			to:   config.ModeJudge,
			want: false,
		},
		{
			// discussion 是五值枚举里最容易被枚举判断漏掉的一个（既有代码里
			// 常被当成"默认/无关紧要"的那一档），单独钉一条防止它被漏判。
			name: "收件人是 discussion，策略恰好只含 discussion：可见",
			team: config.TeamConfig{ThoughtVisibleTo: []config.Mode{config.ModeDiscussion}},
			to:   config.ModeDiscussion,
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := thoughtVisible(tc.team, tc.to)
			if got != tc.want {
				t.Errorf("thoughtVisible(%+v, %q) = %v，期望 %v", tc.team, tc.to, got, tc.want)
			}
		})
	}
}
