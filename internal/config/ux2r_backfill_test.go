package config

// 审查员对 UX-2 修订版补丁找出的一处「承重但零覆盖」的地方：TOML 引号提示按
// （表, id）去重、同 id 先按 id 再按表名排序。这条不是背对背写的，是看过
// load.go 的 dottedIDHints 之后、按审查员的变异 MH2 反推出来的。
//
// 为什么按（表, id）而不是只按 id 去重：冻结的提示句里嵌着表名，同一个 id 在
// agents 与 teams 下各写错一次时，只按 id 去重会丢掉其中一张表的真提示。

import (
	"strings"
	"testing"
)

// 同一个 id x.y 同时出现在 [agents.x.y] 与 [teams.x.y] 下，各带已知字段 ->
// 恰好两条提示，agents 那条在 teams 那条前面。
//
// 两种书写顺序都测：md.Undecoded() 按文件出现顺序给键，两个元素时 sort.Slice
// 走插入排序、是稳定的，所以去掉表名那层比较后，输出次序就是文件次序。只写
// 「agents 在前」一种顺序的话，那个坏实现照样绿；teams 在前那一档才挡得住它。
func TestUx2rBackfillTOMLHintSameIDTwoTables(t *testing.T) {
	agents := []string{`[agents.x.y]`, `provider = "pv_alpha"`, `model = "mod-ux2r"`, ``}
	teams := []string{`[teams.x.y]`, `members = ["foo"]`, `display_name = "T"`, ``}
	orders := []struct {
		name  string
		lines []string
	}{
		{"agents-first", append(append(ux2rWith(), agents...), teams...)},
		{"teams-first", append(append(ux2rWith(), teams...), agents...)},
	}
	wantAgents := ux2rHint("agents", "x.y")
	wantTeams := ux2rHint("teams", "x.y")
	for _, o := range orders {
		t.Run(o.name, func(t *testing.T) {
			_, msg := ux2rLoadErr(t, o.lines...)
			got := ux2rHintLines(msg)
			if len(got) != 2 {
				t.Fatalf("应当恰好两条提示（agents 与 teams 各一），实际 %d 条: %q；错误:\n%s", len(got), got, msg)
			}
			if got[0] != wantAgents || got[1] != wantTeams {
				t.Errorf("提示次序应当是 agents 在前、teams 在后\n期望: %q\n实际: %q；错误:\n%s",
					[]string{wantAgents, wantTeams}, got, msg)
			}
			if n := strings.Count(msg, `[teams."x.y"]`); n != 1 {
				t.Errorf("%s 应当恰好出现一次，实际 %d 次", `[teams."x.y"]`, n)
			}
		})
	}
}
