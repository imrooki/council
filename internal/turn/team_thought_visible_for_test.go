// team_thought_visible_for_test.go 补 docs/12_STEP9_CONTRACT.md 3.6 的覆盖
// 缺口（本片测试方在背对背报告里自己点出的歧义 #3，coordinator 授权补）：
// grep -rn "thoughtVisibleFor" --include=*_test.go internal/ 在补之前只命中
// 一行注释，没有任何用例直接调它——端到端的
// TestAskRefusesWhenBoundTeamRemovedFromConfig 只覆盖了"team 非空但查不到"
// 这一支，"conv.TeamID == \"\" -> false，不是错误"那一支只是被
// TestRouterProductionPathDoesNotLeakThoughtIntoOutgoingBytes（未绑 team 的
// 会话——thoughtVisibleFor("") 算出 routeThought == false，NewRouter 拿到
// 的是这个动态值，不是字面量 false）顺带盖到，不是专门为它写的。
//
// 本文件直接打 thoughtVisibleFor（包内私有方法），不再经 Ask 的八步流程：
// 判别函数本身的返回值坏在哪一层，与"这一轮该不该 refuse、钱花没花、库里
// 那一行动没动"是两件事，各自独立测。
//
// 未决 #25（r.cfg 补并发保护、轮粒度快照）落地后，thoughtVisibleFor 的
// 签名从两参改成三参，末尾加了 cfg *config.Config：函数不再读 r.cfg，
// 只认调用方传进来的这份轮内快照（AskUpTo 经 snapshotRulesConfig 拷一次——
// 「持 cfgMu 读锁拷一次」的实际动作在 snapshotRulesConfig 里面，整轮
// 用同一份）。本文件三条用例都要跟着传这第三个参数，第三条
// （TestThoughtVisibleForDegenerateConfigTreatsNonEmptyTeamIDAsUnknown）
// 的语义因此发生了实质变化，改动理由写在它自己的注释里，不在这里重复。
package turn

import (
	"errors"
	"testing"

	"council/internal/config"
)

// TestThoughtVisibleForEmptyTeamIDReturnsFalseNoError 覆盖 3.6 第一支：
// conv.TeamID == "" -> false，且这不是一个错误。
//
// 两件都要断言，缺一件都是空用例：只断言 false 的话，一个"空 teamID 也报
// ErrTeamUnknown（顺带把 visible 置 false 作为返回值的另一半）"的坏实现
// 照样能让 "visible == false" 这一半通过；只断言 err == nil 的话，一个
// "空 teamID 返回 true"的坏实现（没绑 team 却读成"谁都看得到"，是比不拦
// 更危险的静默泄漏）也能蒙混过关。
//
// 第三个实参传 cfg（与建 r 用的同一份、Teams 里有 "fsdt"）：这条用例测的
// 是 teamID == "" 的短路，不是 cfg 的内容，传一份看得出内容的正常 cfg，
// 免得读者疑心"是不是因为 cfg 本身退化才返回 false"。注意这不能证明短路
// 发生在读 cfg 之前——它只说明"cfg 正常时结果对"，谁先谁后由函数实现的
// 分支顺序决定，本文件不对此单独立一条用例。
func TestThoughtVisibleForEmptyTeamIDReturnsFalseNoError(t *testing.T) {
	cfg := newTestConfig()
	cfg.Teams = map[string]config.TeamConfig{
		"fsdt": {DisplayName: "FSDT 讨论组", Members: []string{testAgentClaude}},
	}
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	visible, err := r.thoughtVisibleFor("", config.ModeJudge, cfg)
	if err != nil {
		t.Fatalf("teamID 为空（没绑 team）不该报错，实际: %v", err)
	}
	if visible {
		t.Error("teamID 为空（没绑 team）应当读作\"谁都看不到\"，返回 false，实际 true")
	}
}

// TestThoughtVisibleForUnknownTeamIDReturnsErrTeamUnknown 覆盖 3.6 第二支：
// teamID 非空、cfg.Teams 里查不到 -> errors.Is(err, ErrTeamUnknown) 为真。
//
// 与端到端的 TestAskRefusesWhenBoundTeamRemovedFromConfig 不重复：那条验的
// 是"这一轮该不该 refuse、Spawner 有没有被调、provider_sessions 那一行
// 动没动"，这条验的是判别函数本身的返回值——两处任一处坏了，靠哪条用例
// 变红能立刻分清坏在"判别函数算错了"还是"Ask 拿到正确判定后处置错了"。
//
// 第三个实参传 cfg（与建 r 用的同一份指针）：这条用例不测"传进去的 cfg
// 和 r.cfg 是不是两份不同的东西"，就用同一份，最贴近 askUpToWith 里
// r.thoughtVisibleFor(conv.TeamID, ac.Mode, cfg) 的真实调用形状——那里的
// cfg 也是拷自 r.cfg 的同一个指针。r.cfg 与参数 cfg 是否真的解耦，由
// 第三条用例专门去分辨。
func TestThoughtVisibleForUnknownTeamIDReturnsErrTeamUnknown(t *testing.T) {
	cfg := newTestConfig()
	cfg.Teams = map[string]config.TeamConfig{
		"fsdt": {DisplayName: "FSDT 讨论组", Members: []string{testAgentClaude}},
	}
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	visible, err := r.thoughtVisibleFor("no-such-team", config.ModeJudge, cfg)
	if !errors.Is(err, ErrTeamUnknown) {
		t.Fatalf("errors.Is(err, ErrTeamUnknown) 应当为真，实际 err=%v, visible=%v", err, visible)
	}
}

// TestThoughtVisibleForDegenerateConfigTreatsNonEmptyTeamIDAsUnknown 覆盖
// coordinator 点名的第三支：非空 teamID 撞上退化的 cfg 参数时该怎么办。
//
// 未决 #25 之前，thoughtVisibleFor 自己读 r.cfg，这支测的是"r.cfg 退化
// 时怎么办"，属于 Runner 的状态。未决 #25 把 r.cfg 从函数体里拿掉，签名
// 多了第三个参数 cfg（调用方持有的轮内快照），函数只认这个参数、不再碰
// r.cfg。这支现在测的是"传进来的这份快照退化时怎么办"——是函数自己对
// 输入的承诺，跟 r.cfg 是不是正常没有关系，这两件事不是一回事，下面故意
// 把它们造成两份不同的 cfg 来分清楚（细节见 r 与 cases 之间的注释）。
//
// 按 3.6 的语义推一遍两种退化：
//
//   - cfg.Teams 为 nil（cfg 本身非 nil，只是配置里从没写过 teams 这张表，
//     不是"team 都删光了"，是"这个键从来没出现过"）：对 nil map 取值
//     team, ok := cfg.Teams[id] 在 Go 里是安全的，team 是零值、ok 恒为
//     false——天然落进"查不到"这一支，本来就该报 ErrTeamUnknown，不需要
//     为它单独加判断。落地的实现里确实有一句 cfg == nil || cfg.Teams ==
//     nil 的显式判断，但推一遍就知道 cfg.Teams == nil 这半句删掉结果
//     不变：它是防御性的对称写法，不是这里真的存在一个语义空洞——本文件
//     照样把它留成一支独立用例，因为"结果不变"是推理结论，不是不测的
//     理由，实现里万一真把这句删掉、又不小心引入别的分支，还是要有一条
//     用例能接住。
//   - cfg 本身是 nil：这一种不一样，对 nil 指针取字段 cfg.Teams 会真的
//     panic，必须显式挡在前面。生产路径今天走不到这个输入——askUpToWith
//     在把 cfg 传给 thoughtVisibleFor 之前，早就已经用同一份快照指针
//     做过 cfg == nil || cfg.Agents == nil 的门（拿到 g.mu 之后、查
//     cfg.Agents[agentID] 之前那一句；AskUpTo 本身只剩两行，取一次
//     规则/配置快照就转手交给 askUpToWith，这道门不在 AskUpTo 里），
//     能走到 thoughtVisibleFor 调用点的 cfg 必是通过了那道门的同一个
//     指针，不可能是 nil。但 cfg.Teams == nil（cfg 本身非 nil）不受那道门管，
//     一份"配了 agents、没配 teams"的真配置照样能带着非空 teamID 走到
//     这里——所以这两支退化不是同一等级：一支是函数自身对输入的承诺
//     （生产触发不到，写进用例名和注释里，不要让人误读成"这是一条真机
//     会发生的路径"），另一支是生产路径真能撞见的配置形态。
//
// 两个子用例的结论相同：非空 teamID 在 cfg 退化的两种情形下都应该读作
// ErrTeamUnknown，不是 false（那会把"没配置"悄悄等同于"配置了但谁都看不
// 到"，两件事应该长得不一样），也不该 panic。
func TestThoughtVisibleForDegenerateConfigTreatsNonEmptyTeamIDAsUnknown(t *testing.T) {
	svc := newTestService(t)

	// r 用一份包含 "fsdt" team 的正常配置 wellFormed 构造，故意与下面传给
	// thoughtVisibleFor 的退化 cfg 参数不是同一份：
	//   - 如果实现退回未决 #25 之前的老路（自己读 r.cfg，不看参数），
	//     wellFormed.Teams 里的 "fsdt" 查得到，两支都会错误地返回
	//     (thoughtVisible 的真实结果, nil)，不是 ErrTeamUnknown。
	//   - 如果实现是"参数非 nil 时用参数、参数为 nil 时退回 r.cfg"这种
	//     半吊子改法，cfg 本身是 nil 那一支会读到 wellFormed 里的 "fsdt"，
	//     同样错误地返回 nil error——这一支专门堵这条退路，只传退化 cfg、
	//     不传能兜底成功的东西，实现没有第二个信息源可用。
	// 只有实现真的只认参数 cfg、对退化输入不做任何回退时，两支才会如愿
	// 落进 ErrTeamUnknown。
	wellFormed := newTestConfig()
	wellFormed.Teams = map[string]config.TeamConfig{
		"fsdt": {DisplayName: "FSDT 讨论组", Members: []string{testAgentClaude}},
	}
	r := newTestRunner(t, wellFormed, svc)

	cases := []struct {
		name string
		cfg  *config.Config
	}{
		{
			name: "cfg 非 nil 但 Teams 是 nil",
			cfg:  &config.Config{},
		},
		{
			name: "cfg 参数本身是 nil",
			cfg:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			visible, err := r.thoughtVisibleFor("fsdt", config.ModeJudge, tc.cfg)
			if !errors.Is(err, ErrTeamUnknown) {
				t.Fatalf("errors.Is(err, ErrTeamUnknown) 应当为真，实际 err=%v, visible=%v", err, visible)
			}
		})
	}
}
