// team_thought_visible_to_test.go 覆盖 docs/12_STEP9_CONTRACT.md 3.2 的三条
// thought_visible_to 校验：(a)(b) 是两条 fail-closed，(c) 是唯一一条「断言
// 不报错」的用例——3.2 的裁决原话是「这条理由是编的，而且会挡掉一个合法
// 配置，降级」，(c) 存在的全部意义就是钉住这个降级不被下一个人重新拉回
// fail-closed。
//
// 复用 validate_test.go 里已经有的 tstValidConfig / tstEditTeam，不新起一份
// 平行的合法配置建造函数——两份配置分叉之后，负例改的是"哪一版"会变得
// 含糊，且本文件与 validate_test.go 同在 package config 里，直接互相看得见。
//
// 本片测试方背对背写，本文件此刻编译不过是预期状态：TeamConfig 还没有
// ThoughtVisibleTo 字段（冻结形状见任务书，实现方 grok 正在独立落地）。

package config

import "testing"

// TestValidateTeamThoughtVisibleToRejectsInvalidMode 覆盖 (a)：
// thought_visible_to 里放一个不属于五值枚举的字符串，断言 ValidationErrors
// 里有且只有一条 Path 精确等于 teams.<id>.thought_visible_to 的错误。
//
// 用精确 Path 匹配而不是 strings.Contains 整条错误信息：子串匹配对这个
// 场景尤其危险——teams.project.members 与 teams.project.thought_visible_to 共享
// 前缀 "teams.project."，一条写得太松的 Contains 断言，在实现把错误记错到
// members 那一路时仍然可能读作"通过"。
func TestValidateTeamThoughtVisibleToRejectsInvalidMode(t *testing.T) {
	cfg := tstValidConfig()
	tstEditTeam(cfg, "project", func(tm *TeamConfig) {
		tm.ThoughtVisibleTo = []Mode{Mode("referee")}
	})

	err := cfg.Validate()
	if err == nil {
		t.Fatalf("期望 Validate 报错，实际返回 nil。配置为: %+v", cfg)
	}
	errs, ok := err.(ValidationErrors)
	if !ok {
		t.Fatalf("err 的动态类型不是 ValidationErrors，实际: %T", err)
	}
	wantPath := "teams.project.thought_visible_to"
	found := 0
	for _, e := range errs {
		if e.Path == wantPath {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("Path 精确等于 %q 的错误条数 = %d，期望恰好 1 条。全部错误: %+v", wantPath, found, errs)
	}
}

// TestValidateTeamThoughtVisibleToDedupsRepeatedMode 覆盖 (b)：同一个 team
// 里重复写同一个合法 mode，断言只报一条，不是两条——体例与既有 members
// 去重的用例一致（validate_test.go 的 "D9 team 的 member 重复"）。
//
// 用的是合法枚举值 ModeJudge：这条用例只想触发"去重"这一条分支。若换成
// 非法值，会与 (a) 的 fail-closed 撞在一起，红了也分不清是哪条规则坏了——
// (a) 与 (b) 的输入必须互不相交，这条用例本身就是这个原则的产物。
func TestValidateTeamThoughtVisibleToDedupsRepeatedMode(t *testing.T) {
	cfg := tstValidConfig()
	tstEditTeam(cfg, "project", func(tm *TeamConfig) {
		tm.ThoughtVisibleTo = []Mode{ModeJudge, ModeJudge}
	})

	err := cfg.Validate()
	if err == nil {
		t.Fatalf("期望 Validate 报错，实际返回 nil。配置为: %+v", cfg)
	}
	errs, ok := err.(ValidationErrors)
	if !ok {
		t.Fatalf("err 的动态类型不是 ValidationErrors，实际: %T", err)
	}
	wantPath := "teams.project.thought_visible_to"
	found := 0
	for _, e := range errs {
		if e.Path == wantPath {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("重复的合法 mode 只应报一条错误，实际 Path=%q 的条数 = %d。全部错误: %+v", wantPath, found, errs)
	}
}

// TestValidateAllowsThoughtVisibleToWithStoreThoughtOff 覆盖 (c)——本片
// 唯一断言"不报错"的用例，也是三条里最容易被写反的一条。
//
// 3.2 的裁决：提案原想让"根级 store_thought=false 且某 team 的
// thought_visible_to 非空"直接报错，理由是"store_thought=false 时 thought
// 列恒为空串，这个开关永远路由不到任何东西"——但 schema.go 的注释与
// router.go 的实现都证明这句话是假的：store_thought=false 只影响此后新写
// 的行，历史行里已经落库的 thought 照样会被 Router 渲染进 judge 的 prompt
// （router.go 的那条分支只看 m.Thought != ""，不看当前的 store_thought）。
//
// 于是"先存了一段时间、现在不想再存了、但仍要让裁判看得到已经存下的那些"
// 是一个真实且合法的配置，fail-closed 会把它挡在门外。这条用例存在的
// 全部意义就是钉住"不挡"这个裁决——把它错写成 fail-closed，这条用例会
// 变红，那正是它的价值所在。
//
// 没有覆盖的半句（写在这里免得漏项）：3.2 要求把"文案里不许出现『恒为
// 空串』"落在 /rules 或启动横幅的提示文案里，那是屏幕输出，Validate() 这一层
// 判断不到，本文件没有、也不该为它写用例——见任务报告里的覆盖缺口一节。
func TestValidateAllowsThoughtVisibleToWithStoreThoughtOff(t *testing.T) {
	cfg := tstValidConfig()
	cfg.StoreThought = false
	tstEditTeam(cfg, "project", func(tm *TeamConfig) {
		tm.ThoughtVisibleTo = []Mode{ModeJudge}
	})

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("store_thought=false 加上某 team 的 thought_visible_to 非空，"+
			"3.2 的裁决是不拦（历史 thought 仍会被路由，降级成提示而不是报错），"+
			"实际 Validate 报错: %v", err)
	}
}
