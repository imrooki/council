package repl

// 审查员对 UX-2 修订版补丁找出的两处「承重但零覆盖」的地方，各补一条回归钉。
// 这两条不是背对背写的：是看过实现（ask.go 的 universe、commands.go 的 cmdAdd）
// 之后、按审查员的变异 MA / MB 反推出来的。它们钉住的是契约 6.2 与 6.3 的行为，
// 不是实现的写法。

import (
	"context"
	"fmt"
	"testing"

	"council/internal/conversation"
)

// /ask 的全集必须含成员 id（契约 6.2 第 2 步）。已从配置删掉、仍是成员的 opus-5-0
// 敲全名时，若全集只有配置键，它不在全集里，就会走前缀解析，被唯一以它为前缀的
// 配置 agent opus-5-0-max 偷走 —— 起进程、花一次入场费。在场与已退出两种状态都要
// 挡住：已退出时正确行为是 X-1 的「已退出」那句，被偷走时问的是在场的 max。
func TestUx2rBackfillAskGoneMemberFullNameNotStolen(t *testing.T) {
	for _, state := range []string{"active", "left"} {
		t.Run(state, func(t *testing.T) {
			fx := ux2rNewFix(t, "opus-5-0-max")
			// opus-5-0 不在配置里，provider 只能手给；它的值与本用例无关。
			if state == "active" {
				fx.seedActive(t, "opus-5-0", "pv_gone")
			} else {
				fx.seedLeft(t, "opus-5-0", "pv_gone")
			}
			fx.seedActive(t, "opus-5-0-max", "")
			// 只记 max 的基线：在场那一档里正确实现会合法地以 opus-5-0 调 Ask，
			// 它自己的 turn_usage 会涨，不能断言它不变。
			before := fx.usages(t, "opus-5-0-max")

			out := fx.run(t, "/ask opus-5-0\n")

			fx.requireUsageUnchanged(t, before, out)
			// 整行相等天然带词边界：「正在问 opus-5-0 ...」不会等于这一行。
			if n := ux2rCountLine(out, fmt.Sprintf(ux2rFmtAsking, "opus-5-0-max")); n != 0 {
				t.Errorf("不该出现「正在问 opus-5-0-max」，实际 %d 次；完整输出:\n%s", n, out)
			}
			if state == "left" {
				ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAskLeft, "opus-5-0", "opus-5-0"))
			} else {
				ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAsking, "opus-5-0"))
			}
		})
	}
}

// /join、/add 在场成员时刷新快照（契约 4.5、6.3、验收 23），display_name 也在
// 快照里。TestUx2rJoinActiveRefreshesSnapshot 只看了 provider；这里把旧行的
// display_name 设成与配置不同的值，要求命令之后变成配置里的 DisplayName。
//
// 注意 fakeStore.AddParticipant 只在新值非空时才覆盖 display_name（真 store 的
// upsert 是无条件覆盖）。所以「Participant 的 DisplayName 填空串」这个坏实现在
// fake 上留下旧值、在真 store 上写成空串，两边都不等于配置值，这条都会红。
func TestUx2rBackfillJoinRefreshesDisplayName(t *testing.T) {
	for _, cmd := range []string{"/join", "/add"} {
		t.Run(cmd[1:], func(t *testing.T) {
			fx := ux2rNewFix(t, "alpha", "beta")
			const oldName = "DN_old_snapshot"
			want := fx.cfg.Agents["alpha"].DisplayName
			if want == oldName || want == "" {
				t.Fatalf("夹具前提不成立：配置里的 display_name 是 %q", want)
			}
			// 不走 seedActive：它把 display_name 写死成配置值，旧值就无从区分了。
			err := fx.st.fakeStore.AddParticipant(context.Background(), conversation.Participant{
				ConvID:      ux2rConvID,
				AgentID:     "alpha",
				DisplayName: oldName,
				Provider:    ux2rProvider,
				Active:      true,
				JoinedAt:    replTestNow,
			})
			if err != nil {
				t.Fatalf("塞入旧快照失败: %v", err)
			}

			out := fx.run(t, cmd+" alpha\n")

			ux2rRequireLine(t, out, ux2rJoined("alpha"))
			p, ok := fx.participant(t, "alpha")
			if !ok || !p.Active {
				t.Fatalf("alpha 应当仍在场，实际 %+v（存在=%v）", p, ok)
			}
			if p.DisplayName != want {
				t.Errorf("参与者行的 display_name 应当刷新成配置里的 %q，实际 %q；完整输出:\n%s", want, p.DisplayName, out)
			}
		})
	}
}
