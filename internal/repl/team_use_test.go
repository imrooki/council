package repl

// team_use_test.go 是 Step 9 片 2c 的测试，背对背写：/team 这个命令此刻在
// HEAD（761f21a）不存在，实现方（grok）正在独立写同一片。两边从同一份
// 冻结契约（docs/12_STEP9_CONTRACT.md 3.5、3.9 第 5 条，以及主代理下发的
// 任务书）出发，互相看不到对方的产出。
//
// 只加本文件，不改任何实现文件；也不碰 internal/repl/conversation_test.go
// 的公共 fakeStore（硬约束 2）——记账用的 joinTrackingStore 是本文件自己的
// 类型，靠内嵌 *fakeStore 复用它已有的全部读写方法，只覆写两个需要拦截
// 或补全的点，见下面 joinTrackingStore 的注释。
//
// 关于「现在应该编译不过」：任务书说 /team 不存在时编译错误应当只有
// 「缺符号」一类。实测下来这个假设对本文件不成立——/team use 是纯字符串
// 命令分派（和 /join、/leave 等既有命令一样，走 REPL 文本输入，不是直接
// 调用某个新导出符号），所以本文件在 HEAD 上编译通过，只是运行时全红
// （/team 走 msgUnknownCommand 那一支）。这是背对背这一片的真实情况，
// 不是本文件写错了，报告里会附上 HEAD 上的红测证据。

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
)

// joinCall 记一次 Tx.AddParticipant 调用收到的 (会话, agent) 参数。
type joinCall struct {
	convID  string
	agentID string
}

// joinTrackingStore 包一层 fakeStore，记账每一次 AddParticipant 调用，
// 供用例断言"有没有为某个人发生过一次 Join"（任务书用例 2 的负向断言、
// 用例 4 的"没有任何人被 Join"）。
//
// SetConversationTeam 不再需要在这里覆写：conversation_test.go 里的公共
// fakeStore 现在自己会把 TeamID 回写进 f.convs（协调方裁决 #2 之后修的,
// 这是"记账不等于生效"那条教训第二次留下的补丁，见那边的方法注释），
// 本类型只靠内嵌直接继承这个行为，不重复一遍。
//
// WriteTx 必须覆写：fakeStore.WriteTx 原样把内嵌的 *fakeStore 自己当 Tx
// 传给 fn（"return fn(ctx, f)"），若不覆写，fn 里的 tx.AddParticipant 调的
// 是内嵌 fakeStore 自己的方法，绕开下面这个覆写版本，记账形同虚设。
type joinTrackingStore struct {
	*fakeStore
	joinCalls []joinCall
}

func newJoinTrackingStore() *joinTrackingStore {
	return &joinTrackingStore{fakeStore: newFakeStore()}
}

var (
	_ conversation.Store = (*joinTrackingStore)(nil)
	_ conversation.Tx    = (*joinTrackingStore)(nil)
)

func (s *joinTrackingStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return fn(ctx, s)
}

func (s *joinTrackingStore) AddParticipant(ctx context.Context, p conversation.Participant) error {
	s.joinCalls = append(s.joinCalls, joinCall{convID: p.ConvID, agentID: p.AgentID})
	return s.fakeStore.AddParticipant(ctx, p)
}

// joinCountFor 数 convID/agentID 这一对在本次记账里发生过几次 Join。
func (s *joinTrackingStore) joinCountFor(convID, agentID string) int {
	n := 0
	for _, c := range s.joinCalls {
		if c.convID == convID && c.agentID == agentID {
			n++
		}
	}
	return n
}

// newTeamTrioConfig 在既有的 newTestREPLConfig（repl_test.go）基础上加一个
// 三人 team：既有三张 team（tm_zeta 1 人、tm_mid 2 人、tm_alpha 2 人）里
// 没有一个三人的，本片用例 1 需要"team 比当前会话成员多一个人"的情形。
// 三个成员就是 newTestREPLConfig 已经配置好的全部三个 agent，不需要另造
// agent 配置。不改 repl_test.go：只是调用它、在返回值上追加一条。
func newTeamTrioConfig() *config.Config {
	cfg := newTestREPLConfig()
	cfg.Teams["tm_trio"] = config.TeamConfig{
		DisplayName: "Team Trio",
		Members:     []string{"ag_alpha", "ag_mid", "ag_zeta"},
	}
	return cfg
}

// runTeamREPL 起一个 REPL 跑到结束，带看门狗，svc 由调用方给的 store 现场构造。
// 与 conversation_test.go 的 runREPLSvc 同一个形状，唯一区别是 cfg 由调用方传——
// 本文件的用例需要 newTeamTrioConfig 这份带三人 team 的配置，不是包级默认的
// newTestREPLConfig。
func runTeamREPL(t *testing.T, cfg *config.Config, store conversation.Store, input string) (string, error) {
	t.Helper()

	svc := conversation.NewService(store)
	var out bytes.Buffer
	r := New(cfg, svc, nil, strings.NewReader(input), &out, RulesContext{})

	done := make(chan error, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				done <- fmt.Errorf("REPL.Run panic: %v", rec)
			}
		}()
		done <- r.Run(context.Background())
	}()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(testREPLTimeout):
		t.Fatalf("REPL.Run 在 %v 内没有返回", testREPLTimeout)
		return "", nil
	}
}

// teamsRowFields 把一次（或多次）/teams 输出按行拆开，对每个给定的 team id
// 找到恰好含有该 id 作为一个独立字段（strings.Fields 切出的一个 token）的
// 那一行，记下这一行切出的全部字段。
//
// 两层防护，都是本项目反复栽过的坑：
//
//  1. 不用"整段输出 Contains team id"判定"当前列显示哪个 id"——team id
//     本来就会出现在 ID 列里，那样的断言对这张表恒真（任务书原话）。这里
//     改成先定位到那一行、再按字段比较，且字段"精确等于"而不是子串；
//     四个 team id（tm_zeta/tm_mid/tm_alpha/tm_trio）互不为前缀也互不
//     包含，可以放心精确匹配。
//  2. 只在"ID\tMEMBERS\tMEMBER_IDS"这行表头（cmdTeams 现有的、契约明写
//     "/teams 保留不动"的表头）**之后**的行里找，且只认最后一次出现的
//     表头：/team use 的绑定确认行如果本身也带上了 team id（例如
//     "已绑定 team tm_trio"），它出现在表头之前，不会被这个函数当成
//     表格的一行误收。
func teamsRowFields(out string, teamIDs []string) map[string][]string {
	lines := strings.Split(out, "\n")

	headerAt := -1
	for i, line := range lines {
		for _, f := range strings.Fields(line) {
			if f == "MEMBER_IDS" {
				headerAt = i
			}
		}
	}

	rows := make(map[string][]string, len(teamIDs))
	if headerAt < 0 {
		return rows
	}
	for _, line := range lines[headerAt+1:] {
		fields := strings.Fields(line)
		for _, want := range teamIDs {
			for _, f := range fields {
				if f == want {
					rows[want] = fields
					break
				}
			}
		}
	}
	return rows
}

// equalFields 逐个比较两个字段切片是否完全相同（长度与每个元素）。
func equalFields(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------
// 用例 1：三人 team 用在两人会话上。
// ---------------------------------------------------------------------

// TestTeamUseJoinsOnlyMissingMembers 覆盖任务书用例 1：会话里已有两名成员
// （直接塞进假实现，绕开 /join 的回显——见下面对用例 1 的第二个断言），
// team 配三个人，/team use 之后 /who 显示三人，且只有真正缺的那个人
// 被 Join 了一次，另两个人一次都不该被 Join。
//
// /who 的输出取自"同一个 store 上第二次单独跑的 REPL"，只敲 /use 与
// /who，不掺 /team use 自己的确认行——否则若 /team use 的确认行里本来
// 就会回显"ag_zeta 已加入"之类的话，"/who 的输出里有没有 ag_zeta"这条
// 断言就会被 /team use 自己的输出捎带满足，测不出 /who 是不是真的读到了
// 三人（顾问评审指出的点）。
func TestTeamUseJoinsOnlyMissingMembers(t *testing.T) {
	cfg := newTeamTrioConfig()
	store := newJoinTrackingStore()
	ctx := context.Background()

	const convID = "convtrio01"
	store.fakeStore.seedConversation(convID, "三人 team 用在两人会话上")
	if err := store.fakeStore.AddParticipant(ctx, conversation.Participant{
		ConvID: convID, AgentID: "ag_alpha", Provider: "fake", JoinedAt: replTestNow, Active: true,
	}); err != nil {
		t.Fatalf("测试夹具：塞入 ag_alpha 失败: %v", err)
	}
	if err := store.fakeStore.AddParticipant(ctx, conversation.Participant{
		ConvID: convID, AgentID: "ag_mid", Provider: "fake", JoinedAt: replTestNow, Active: true,
	}); err != nil {
		t.Fatalf("测试夹具：塞入 ag_mid 失败: %v", err)
	}

	out1, err := runTeamREPL(t, cfg, store, "/use "+convID+"\n/team use tm_trio\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out1)
	}

	parts, err := store.Participants(ctx, convID, true)
	if err != nil {
		t.Fatalf("读假实现里的 participant 失败: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("/team use 之后应当有 3 个 participant，实际 %d 个: %+v；/team use 输出:\n%s", len(parts), parts, out1)
	}
	byID := make(map[string]conversation.Participant, len(parts))
	for _, p := range parts {
		byID[p.AgentID] = p
	}
	zeta, ok := byID["ag_zeta"]
	if !ok {
		t.Fatalf("team 里缺的第三人 ag_zeta 没有被加进来，实际成员: %+v", parts)
	}
	if !zeta.Active {
		t.Errorf("ag_zeta 应当是活跃成员，实际: %+v", zeta)
	}

	// Join 记账：zeta 是真缺的那个，应当且只应当被 Join 一次；
	// alpha/mid 已经在会话里（本用例直接塞进去的，没经过 Join 记账那条路），
	// /team use 不应当再为他们调用一次 Join。
	if n := store.joinCountFor(convID, "ag_zeta"); n != 1 {
		t.Errorf("ag_zeta 是尚未加入的那个人，/team use 应当且只应当 Join 它一次，实际 %d 次", n)
	}
	if n := store.joinCountFor(convID, "ag_alpha"); n != 0 {
		t.Errorf("ag_alpha 已经在会话里，/team use 不应当为它调用 Join，实际 %d 次", n)
	}
	if n := store.joinCountFor(convID, "ag_mid"); n != 0 {
		t.Errorf("ag_mid 已经在会话里，/team use 不应当为它调用 Join，实际 %d 次", n)
	}

	// /who 单独一次跑，不掺 /team use 自己的输出。
	out2, err := runTeamREPL(t, cfg, store, "/use "+convID+"\n/who\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out2)
	}
	for _, want := range []string{"ag_alpha", "ag_mid", "ag_zeta"} {
		if !strings.Contains(out2, want) {
			t.Errorf("/who 的输出里缺少成员 %q；完整输出:\n%s", want, out2)
		}
	}
}

// ---------------------------------------------------------------------
// 用例 2（本片要害）：退出过的人不被静默拉回。
// ---------------------------------------------------------------------

// TestTeamUseDoesNotSilentlyRejoinLeftMember 覆盖任务书用例 2。
//
// 场景 A：三人都已经在 participants 表里，但 ag_mid 是 inactive（曾经
// /leave 过）。/team use 之后必须三件事同时成立：
//  1. ag_mid 仍然 inactive（不能被静默拉回）；
//  2. 屏幕点名了他；
//  3. 没有任何人发生 Join（三人两张表——active 表与全量表——都查得到，
//     没有一个人满足"两张表都查不到"这个门槛，contract 3.5 的判据）。
//
// 只验场景 A 的"点名"这一半不够：若实现在绑定确认行里无条件回显
// "team tm_trio: ag_alpha, ag_mid, ag_zeta"这类话，场景 A 的输出会
// "碰巧"含有 ag_mid，但那不是任务书要的"点名"，是团队成员列表的常规
// 回显。所以另起一个场景 B——同一个 team，三人都活跃、没有人退出过——
// 在一个干净的 store 上跑同样的 /team use，拿两个场景的输出做差分。
//
// 差分不能比较整段输出逐字是否相同：两段输出各自带着 /use convA、
// /use convB 的回显行，convID 不一样，两段输出结构上永远不可能逐字
// 相同，这样比没有分辨力。真正要比的是 cmdTeamUse 里 p.LeftAt != nil
// 那一支实际打印的整句公告——"ag_mid 已退出，未自动拉回，要加回用
// /join"——只应当出现在场景 A（ag_mid 真的退出过、未拉回），不应当
// 出现在场景 B（三人都活跃，没人会走到这一支）。这句话只在 outA
// 出现、不在 outB 出现，才说明"点名"是按 ag_mid 的真实状态触发的，
// 不是团队成员列表的常规回显恰好带出来的。
func TestTeamUseDoesNotSilentlyRejoinLeftMember(t *testing.T) {
	cfg := newTeamTrioConfig()
	ctx := context.Background()

	// 场景 A：ag_mid 曾经退出。
	storeA := newJoinTrackingStore()
	const convA = "convleaveA1"
	storeA.fakeStore.seedConversation(convA, "退出过的人不被静默拉回")
	for _, agent := range []string{"ag_alpha", "ag_mid", "ag_zeta"} {
		if err := storeA.fakeStore.AddParticipant(ctx, conversation.Participant{
			ConvID: convA, AgentID: agent, Provider: "fake", JoinedAt: replTestNow, Active: true,
		}); err != nil {
			t.Fatalf("测试夹具：塞入 %s 失败: %v", agent, err)
		}
	}
	leftAt := replTestNow.Add(time.Hour)
	if err := storeA.fakeStore.DeactivateParticipant(ctx, convA, "ag_mid", leftAt); err != nil {
		t.Fatalf("测试夹具：/leave ag_mid 失败: %v", err)
	}

	outA, err := runTeamREPL(t, cfg, storeA, "/use "+convA+"\n/team use tm_trio\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, outA)
	}

	// 断言 1：ag_mid 仍然 inactive。
	partsA, err := storeA.Participants(ctx, convA, true)
	if err != nil {
		t.Fatalf("读假实现里的 participant 失败: %v", err)
	}
	var midRow *conversation.Participant
	for i := range partsA {
		if partsA[i].AgentID == "ag_mid" {
			midRow = &partsA[i]
		}
	}
	if midRow == nil {
		t.Fatalf("ag_mid 的 participant 行不见了，实际: %+v", partsA)
	}
	if midRow.Active {
		t.Errorf("ag_mid 应当仍是 inactive（不能被静默拉回），实际 Active=true")
	}
	if midRow.LeftAt == nil {
		t.Errorf("ag_mid 的 LeftAt 应当仍然非空，实际被清空了")
	}

	// 断言 3：没有任何人发生 Join——三人两张表都查得到（含 inactive 的
	// ag_mid），contract 3.5 的判据下一个都不该被 Join。
	if n := len(storeA.joinCalls); n != 0 {
		t.Errorf("三人都已经在 participants 表里（含 inactive），/team use 不应当发生任何 Join，实际发生了 %d 次: %+v",
			n, storeA.joinCalls)
	}

	// 断言 2（点名）：ag_mid 的 id 出现在输出里。
	if !strings.Contains(outA, "ag_mid") {
		t.Fatalf("/team use 应当在屏幕上点名 ag_mid（退出过、未自动拉回），实际输出:\n%s", outA)
	}

	// 场景 B：同一个 team，三人都活跃，没有人退出过。
	storeB := newJoinTrackingStore()
	const convB = "convactiveB1"
	storeB.fakeStore.seedConversation(convB, "三人都在场的对照场景")
	for _, agent := range []string{"ag_alpha", "ag_mid", "ag_zeta"} {
		if err := storeB.fakeStore.AddParticipant(ctx, conversation.Participant{
			ConvID: convB, AgentID: agent, Provider: "fake", JoinedAt: replTestNow, Active: true,
		}); err != nil {
			t.Fatalf("测试夹具：塞入 %s 失败: %v", agent, err)
		}
	}

	outB, err := runTeamREPL(t, cfg, storeB, "/use "+convB+"\n/team use tm_trio\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, outB)
	}
	if n := len(storeB.joinCalls); n != 0 {
		t.Errorf("对照场景三人本来就都活跃，/team use 也不应当发生任何 Join，实际发生了 %d 次: %+v",
			n, storeB.joinCalls)
	}

	// 断言 4（真判据，替掉上面那条死断言）：cmdTeamUse 里 p.LeftAt != nil
	// 那一支实际打印的整句公告——"ag_mid 已退出，未自动拉回，要加回用
	// /join"（commands.go cmdTeamUse，`r.printf("%s 已退出，未自动拉回，
	// 要加回用 /join\n", agentID)`）——必须只出现在场景 A，不能出现在
	// 场景 B。
	//
	// 原来这里写的是 outA == outB 的整段输出比对：场景 A 和场景 B 用的
	// 是不同的 convID（convA / convB），而两段输出都是 "/use <convID>\n
	// /team use tm_trio\n" 跑出来的整屏文本——/use 的回显行本身就带着
	// convID，两段输出结构上永远不可能逐字相同。于是 outA == outB
	// 结构上永远不成立，这条 t.Errorf 永远不会触发，是一条死断言。
	// 撑腰的只剩上面「断言 2（点名）」那条弱断言（outA 里含子串 "ag_mid" 就算过），
	// 而"绑定确认行之后无条件回显团队成员名单"这种坏实现，只要名单里
	// 带 ag_mid，一样能让那条弱断言通过——那正是这条差分本来要挡住的
	// 东西，可惜它结构上挡不住。
	//
	// 换成下面这对断言才是真正能分辨两种实现的判据：
	//   - 坏实现 A（绑定确认行之后无条件回显团队成员名单，例如打印
	//     "team tm_trio: ag_alpha, ag_mid, ag_zeta"，不再单独判断谁退出
	//     过）-> ag_mid 这个子串在 outA 里仍然会出现，「断言 2（点名）」那条弱断言
	//     被蒙混过关；但下面第一条断言要求的是那句完整公告，名单回显
	//     没有这句话的文本，outA 里找不到它 -> 转红，抓住这个坏实现；
	//   - 坏实现 B（把这句公告的打印从 `p.LeftAt != nil` 的判断里挪出来，
	//     对每一个已存在的成员都打印同一句状态行，不管他到底退出过没有）
	//     -> 场景 B 三人都活跃，也会各自打印这句公告，outB 里就会出现
	//     "ag_mid 已退出，未自动拉回，要加回用 /join" -> 下面第二条断言
	//     （outB 不能含它）转红，抓住这个坏实现；
	//   - 好实现 -> 这句公告只在真的判定"找得到这个人、他不是 Active、
	//     且 LeftAt 非空"时才打印，场景 A 里只有 ag_mid 满足，场景 B
	//     里没有人满足 -> outA 含、outB 不含，两条断言都过。
	midLeftAnnounce := fmt.Sprintf("%s 已退出，未自动拉回，要加回用 /join\n", "ag_mid")
	if !strings.Contains(outA, midLeftAnnounce) {
		t.Errorf("场景 A 应当打印 ag_mid 的退出未拉回公告 %q，实际输出:\n%s", midLeftAnnounce, outA)
	}
	if strings.Contains(outB, midLeftAnnounce) {
		t.Errorf("场景 B 三人都活跃、没有人退出过，/team use 不应当打印 ag_mid 的退出未拉回公告，"+
			"实际却出现了 %q，说明这句公告要么是无条件打印的（比如绑定确认行之后无条件回显团队"+
			"成员名单），要么是对每个已存在的成员都打印同一句状态行，不是只在真的检测到"+
			"「找得到此人、非 Active、LeftAt 非空」时才打印；场景 B 输出:\n%s", midLeftAnnounce, outB)
	}
}

// ---------------------------------------------------------------------
// 用例 3：绑完立刻 /teams，当前列要对。
// ---------------------------------------------------------------------

// TestTeamsCurrentColumnUpdatesAfterTeamUse 覆盖任务书用例 3，钉的是
// r.cur 这份内存副本会不会跟着 BindTeam 写库同步更新（措辞不进契约，
// "当前列变没变"进契约）。
//
// 用前后两次独立跑的 REPL 做差分，不猜"当前"标记具体长什么样（星号、
// 某个词、还是别的都可能）：
//   - "绑前"：/new 之后直接 /teams（这次没有 /team use）；
//   - "绑后"：/new -> /team use tm_trio -> 紧接着同一次运行里 /teams。
//
// 两次的配置完全一样，四个 team 的 MEMBERS / MEMBER_IDS 内容也完全一样，
// 所以"绑前"与"绑后"里同一个 team 的行，字段级别唯一可能变化的就是"当前"
// 那一列。tm_trio 是被绑的那个，它的行必须变；另外三个从没被绑过，行
// 必须原样不变——否则要么"当前列"根本没起作用，要么是整张表都在跟着
// 别的什么东西变，那也测不出 r.cur 缓存这个具体问题。
func TestTeamsCurrentColumnUpdatesAfterTeamUse(t *testing.T) {
	cfg := newTeamTrioConfig()
	teamIDs := []string{"tm_zeta", "tm_mid", "tm_alpha", "tm_trio"}

	beforeStore := newJoinTrackingStore()
	before, err := runTeamREPL(t, cfg, beforeStore, "/new 绑定前先看 teams\n/teams\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, before)
	}
	beforeRows := teamsRowFields(before, teamIDs)
	for _, id := range teamIDs {
		if _, ok := beforeRows[id]; !ok {
			t.Fatalf("绑定前 /teams 的输出里找不到 team %q 的行；完整输出:\n%s", id, before)
		}
	}

	afterStore := newJoinTrackingStore()
	after, err := runTeamREPL(t, cfg, afterStore, "/new 绑定前先看 teams\n/team use tm_trio\n/teams\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, after)
	}
	afterRows := teamsRowFields(after, teamIDs)
	for _, id := range teamIDs {
		if _, ok := afterRows[id]; !ok {
			t.Fatalf("绑定后 /teams 的输出里找不到 team %q 的行；完整输出:\n%s", id, after)
		}
	}

	if equalFields(beforeRows["tm_trio"], afterRows["tm_trio"]) {
		t.Errorf("/team use tm_trio 之后紧接着 /teams，tm_trio 这一行的当前列应当变化，实际逐字未变\n"+
			"绑前: %v\n绑后: %v\n绑后完整输出:\n%s", beforeRows["tm_trio"], afterRows["tm_trio"], after)
	}
	for _, id := range []string{"tm_zeta", "tm_mid", "tm_alpha"} {
		if !equalFields(beforeRows[id], afterRows[id]) {
			t.Errorf("没有被绑定的 team %q 这一行不应当变化，实际变了\n绑前: %v\n绑后: %v\n绑后完整输出:\n%s",
				id, beforeRows[id], afterRows[id], after)
		}
	}
}

// ---------------------------------------------------------------------
// 用例 4：team id 不存在。
// ---------------------------------------------------------------------

// TestTeamUseUnknownTeamIDRefusesWithoutSideEffects 覆盖任务书用例 4：
// /team use 一个配置里没有的 id，要打一行人话、不发生绑定、不发生任何 Join。
//
// "打了一行人话"不看措辞，但也不能只看"输出有没有变多"——/team 一旦注册
// 进命令表，不管实现对不对，多半都会打点什么，"变多"这条单独存在时
// 对任何"打印点什么"的实现都恒真，测不出内容对不对。加一条更有信息量的：
// 断言 tm_missing 这个坏 id 本身出现在输出里。这不是在猜具体措辞——
// 本项目里"配置里查不到这个 id"的既有分支全都回显了那个 id
// （cmdJoin: "配置里没有 agent %s"；cmdUse: "没有 id 以 %s 开头的会话"），
// 是这份代码库自己一贯的写法，不是本文件凭空加的假设。
//
// "变多"这条留作次要断言：baseline 用 TestStepFiveCommandsWithoutService
// 同款手法——baseline 跑 "/new X\n\n"（多一个空行，空行不产生任何输出），
// 实际跑 "/new X\n/team use tm_missing\n"，两边的提示符次数、/new 那一行
// 输出完全相同，唯一可能不同的是第二条命令自己打的东西。若直接拿
// "/new X\n"（不带空行）当 baseline，两边天然就差一个提示符
// "council> "，长度比较对任何实现都恒真——这是评审过程中发现的一个坑，
// 已经改掉。
func TestTeamUseUnknownTeamIDRefusesWithoutSideEffects(t *testing.T) {
	cfg := newTeamTrioConfig()
	const newLine = "/new team 用例四\n"

	baselineStore := newJoinTrackingStore()
	baseline, err := runTeamREPL(t, cfg, baselineStore, newLine+"\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, baseline)
	}

	store := newJoinTrackingStore()
	out, err := runTeamREPL(t, cfg, store, newLine+"/team use tm_missing\n")
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}
	if !strings.Contains(out, "tm_missing") {
		t.Errorf("/team use 一个配置里没有的 team id，提示里应当点名 %q（本项目"+
			"既有的「配置里查不到这个 id」分支都这么做），实际输出:\n%s", "tm_missing", out)
	}
	if len(out) <= len(baseline) {
		t.Errorf("/team use 一个配置里没有的 team id 应当打印一行提示，实际输出没有比只 /new 时更多；"+
			"baseline（%d 字节）:\n%s\n实际（%d 字节）:\n%s", len(baseline), baseline, len(out), out)
	}

	const convID = "conv001"
	ctx := context.Background()
	conv, err := store.Conversation(ctx, convID)
	if err != nil {
		t.Fatalf("读假实现里的会话失败: %v", err)
	}
	if conv.TeamID != "" {
		t.Errorf("team id 不存在时不应当发生绑定，实际 TeamID=%q", conv.TeamID)
	}
	if n := len(store.setConversationTeamCalls); n != 0 {
		t.Errorf("team id 不存在时不应当调用 SetConversationTeam，实际调用了 %d 次: %+v", n, store.setConversationTeamCalls)
	}
	if n := len(store.joinCalls); n != 0 {
		t.Errorf("team id 不存在时不应当有任何人被 Join，实际发生了 %d 次: %+v", n, store.joinCalls)
	}
}

// TestTeamUseWithoutCurrentConversation 锁住冻结形状里"/team 需要 needsSvc
// 与 needsConv"这一条：没有当前会话时敲 /team use，应当走既有的 msgNoConv
// 兜底，不发生绑定、不发生 Join。五条用例之外的加项，成本很低，顺手锁上。
func TestTeamUseWithoutCurrentConversation(t *testing.T) {
	cfg := newTeamTrioConfig()
	store := newJoinTrackingStore()

	out, err := runTeamREPL(t, cfg, store, "/team use tm_trio\n")
	if err != nil {
		t.Fatalf("没有当前会话时 Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}
	if !strings.Contains(out, msgNoConv) {
		t.Errorf("没有当前会话时 /team use 应当走既有的 %q 兜底，实际输出:\n%s", msgNoConv, out)
	}
	if n := len(store.setConversationTeamCalls); n != 0 {
		t.Errorf("没有当前会话时不应当调用 SetConversationTeam，实际调用了 %d 次", n)
	}
	if n := len(store.joinCalls); n != 0 {
		t.Errorf("没有当前会话时不应当有任何人被 Join，实际发生了 %d 次", n)
	}
}

// ---------------------------------------------------------------------
// 用例 5：没选会话时的 /teams。
// ---------------------------------------------------------------------

// TestTeamsWithoutCurrentConversationListsAllTeams 覆盖任务书用例 5：
// 不 /new 也不 /use，直接敲 /teams，应当照常列出全部 team、不报错、不
// panic、也不要求当前会话（冻结契约明写"这一点不许改"）。
//
// "当前列是占位符"这半句没有在本用例里钉死：冻结契约只说"多标一列"，
// 没定字面符号是什么，本文件也刻意不猜（teamsRowFields 的注释里解释了
// 为什么"整段 Contains"对这张表是恒真陷阱）。TestTeamsCurrentColumnUpdatesAfterTeamUse
// 已经用前后差分的办法钉住了"绑定确实会改变那一列"这件事本身；这条用例
// 只锁"没有当前会话时不 panic、不报 needsConv 的提示、全部 team 照常
// 列出、每个 team 的成员 id 都在输出里"，算是这半句里能不靠猜测就验到
// 的部分。差的那一半（"没有当前会话时，四行的当前列彼此一致、且与
// TestTeamsCurrentColumnUpdatesAfterTeamUse 的绑前输出一致"）留在报告的
// 验证等级里如实说明，不用一个自己都不确定对不对的断言去凑数。
func TestTeamsWithoutCurrentConversationListsAllTeams(t *testing.T) {
	cfg := newTeamTrioConfig()
	store := newJoinTrackingStore()

	out, err := runTeamREPL(t, cfg, store, "/teams\n")
	if err != nil {
		t.Fatalf("没有当前会话时 /teams 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}
	if strings.Contains(out, msgNoConv) {
		t.Errorf("/teams 不应当要求当前会话，实际打印了提示 %q；完整输出:\n%s", msgNoConv, out)
	}

	teamIDs := []string{"tm_zeta", "tm_mid", "tm_alpha", "tm_trio"}
	rows := teamsRowFields(out, teamIDs)
	for _, id := range teamIDs {
		if _, ok := rows[id]; !ok {
			t.Errorf("没有当前会话时 /teams 的输出里缺少 team %q；完整输出:\n%s", id, out)
		}
	}

	// 成员列表的完整性：每个 team 配置的成员 id 至少出现一次。这里用整段
	// 输出做 Contains 是安全的——冒的不是"当前列"那个恒真的险：agent id
	// （ag_*）和 team id（tm_*）互不为子串，不会撞上任务书点名的那个坑。
	for id, tc := range cfg.Teams {
		for _, member := range tc.Members {
			if !strings.Contains(out, member) {
				t.Errorf("team %q 的成员 %q 没有出现在 /teams 的输出里；完整输出:\n%s", id, member, out)
			}
		}
	}
}
