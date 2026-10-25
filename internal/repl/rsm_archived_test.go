package repl

// 本文件锁住 /resume --archived（docs/29_FIXES_PLAN.md A1）：
//   - 归档视图只列归档会话，每条标题后标 [已归档]；不带 --archived 时行为不变；
//   - --all --archived 跨项目，单独 --archived 只列本项目的归档会话；
//   - 选中归档会话之后照常接上，/say 落进同一条会话；
//   - 用法与帮助文本里都有 --archived。
//
// 两个视图互斥这件事由 fakeStore 的两张表承担（见 conversation_test.go 里
// Conversations / ArchivedConversations 的注释）；真 store 那一半由
// internal/store 自己的用例覆盖，本文件不重复造那份夹具。

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// rsmSeedArchived 塞一个已归档的会话。归档位在 rsmSeed 之后单独设：
// 既有用例的种子会话因此仍然是活跃的，本文件的归档位也不会漏到别处。
func rsmSeedArchived(t *testing.T, f *fakeStore, id, title, cwd string, created time.Time) conversation.Conversation {
	t.Helper()
	rsmSeed(t, f, id, title, cwd, created)
	f.mu.Lock()
	defer f.mu.Unlock()
	i, ok := f.convIdx[id]
	if !ok {
		t.Fatalf("seedConversation 之后 convIdx 里没有 %q", id)
	}
	f.convs[i].Archived = true
	return f.convs[i]
}

// rsmAssertRows 逐行比对：单格的行整行比文字，多格的行按 tabwriter 的列切分比。
// 与 rsm_list_test.go 里那几段内联循环同一套断言方式，抽出来是因为归档视图
// 有好几张表要逐行比。
func rsmAssertRows(t *testing.T, out string, want [][]string) {
	t.Helper()
	lines := rsmLines(out)
	if len(lines) != len(want) {
		t.Fatalf("输出有 %d 行，期望 %d 行:\n%s", len(lines), len(want), out)
	}
	for i, row := range want {
		if len(row) == 1 {
			if lines[i] != row[0] {
				t.Errorf("第 %d 行 = %q，期望 %q", i+1, lines[i], row[0])
			}
			continue
		}
		rsmAssertCells(t, lines[i], row)
	}
}

// rsmArchivedHint 是归档视图列表最后那句提示。它必须带 --archived：
// 编号是按归档表算的，照抄 /resume <编号> 会看到另一张表。
const rsmArchivedHint = "敲 /resume --archived <编号> 接上；编号按最近活动排序，每次重新计算"

// rsmArchivedFixture 是归档视图共用的六场会话：
// 本项目活跃 p1、别的项目活跃 p2；本项目归档 a1/a2、别的项目归档 a3、
// 未绑定项目的归档 a4。归档会话都没有消息，最后活动时间等于创建时间，
// 排序因此只按创建时间倒序。
type rsmArchivedFixture struct {
	r          *REPL
	f          *fakeStore
	out        *bytes.Buffer
	projectDir string
	other      string
	p1, p2     string
	a1, a2     string
	a3, a4     string
	t0         time.Time
}

func rsmNewArchivedFixture(t *testing.T) *rsmArchivedFixture {
	t.Helper()
	fx := &rsmArchivedFixture{
		projectDir: rsmTempDir(t),
		other:      `X:\别的项目`,
		p1:         "live-one-aaaa",
		p2:         "live-two-bbbb",
		a1:         "arch-one-1111",
		a2:         "arch-two-2222",
		a3:         "arch-three-3333",
		a4:         "arch-four-4444",
		t0:         time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC),
	}
	fx.r, fx.f, fx.out = rsmNewREPL(t, fx.projectDir)
	plus := func(h int) time.Time { return fx.t0.Add(time.Duration(h) * time.Hour) }

	rsmSeed(t, fx.f, fx.p1, "活跃一", fx.projectDir, plus(0))
	rsmSeed(t, fx.f, fx.p2, "活跃二", fx.other, plus(1))
	rsmSeedArchived(t, fx.f, fx.a1, "归档一", fx.projectDir, plus(2))
	rsmSeedArchived(t, fx.f, fx.a2, "归档二", fx.projectDir, plus(3))
	rsmSeedArchived(t, fx.f, fx.a3, "归档三", fx.other, plus(4))
	rsmSeedArchived(t, fx.f, fx.a4, "归档四", "", plus(5))
	return fx
}

// TestRsmArchivedListOnlyArchived 是主路径：--archived 只列本项目的归档会话、
// 每条标题后带标记；同一份数据不带 --archived 时一个归档会话都不出现（回归）。
func TestRsmArchivedListOnlyArchived(t *testing.T) {
	fx := rsmNewArchivedFixture(t)

	// 回归：默认列表仍然是「本项目的活跃会话」，归档的一个都不出现。
	def := rsmRun(t, fx.r, fx.out, "/resume")
	defHead := "项目: " + fx.projectDir + "（本项目 1 个会话；/resume --all 看全部）"
	if lines := rsmLines(def); len(lines) == 0 || lines[0] != defHead {
		t.Errorf("默认列表第 1 行 = %q，期望 %q；输出:\n%s", firstOrEmpty(rsmLines(def)), defHead, def)
	}
	if ids := rsmListIDs(t, def); len(ids) != 1 || ids[0] != fx.p1[:8] {
		t.Errorf("默认列表数据行 = %v，期望恰好 [%s]；输出:\n%s", ids, fx.p1[:8], def)
	}
	if strings.Contains(def, "已归档") {
		t.Errorf("默认列表里出现了归档标记；输出:\n%s", def)
	}

	// --all 同理：只多出别的项目的活跃会话，归档仍然不在。
	all := rsmRun(t, fx.r, fx.out, "/resume --all")
	if lines := rsmLines(all); len(lines) == 0 || lines[0] != "全部会话（2 个）" {
		t.Errorf("/resume --all 第 1 行 = %q，期望 %q；输出:\n%s", firstOrEmpty(rsmLines(all)), "全部会话（2 个）", all)
	}
	if ids := rsmListIDs(t, all); len(ids) != 2 {
		t.Errorf("/resume --all 数据行数 = %d，期望 2；输出:\n%s", len(ids), all)
	}

	// 归档视图：只有本项目的两个归档会话，顺序按创建时间倒序，各带标记；
	// 未绑定的归档会话被项目过滤藏起来，进提示行。
	plus := func(h int) time.Time { return fx.t0.Add(time.Duration(h) * time.Hour) }
	got := rsmRun(t, fx.r, fx.out, "/resume --archived")
	want := [][]string{
		{"项目: " + fx.projectDir + "（本项目 2 个归档会话；/resume --all --archived 看全部）"},
		{"NO", "CUR", "ID", "LAST_ACTIVE", "MEMBERS", "MSGS", "TITLE"},
		{"1", "-", fx.a2[:8], rsmLocalTime(plus(3)), "0", "0", "归档二 [已归档]"},
		{"2", "-", fx.a1[:8], rsmLocalTime(plus(2)), "0", "0", "归档一 [已归档]"},
		{"另有 1 个未绑定项目的归档会话，/resume --all --archived 查看"},
		{rsmArchivedHint},
	}
	rsmAssertRows(t, got, want)

	for _, id := range []string{fx.p1, fx.p2, fx.a3} {
		if strings.Contains(got, id[:8]) {
			t.Errorf("本项目的归档视图里出现了不属于它的会话 %s；输出:\n%s", id, got)
		}
	}
}

// TestRsmArchivedListAll 锁 --all --archived：跨项目的归档视图，带 PROJECT 列，
// 不含活跃会话，也不打未绑定提示；两个 flag 的书写顺序不影响结果。
func TestRsmArchivedListAll(t *testing.T) {
	fx := rsmNewArchivedFixture(t)
	plus := func(h int) time.Time { return fx.t0.Add(time.Duration(h) * time.Hour) }

	got := rsmRun(t, fx.r, fx.out, "/resume --all --archived")
	want := [][]string{
		{"全部归档会话（4 个）"},
		{"NO", "CUR", "ID", "LAST_ACTIVE", "MEMBERS", "MSGS", "PROJECT", "TITLE"},
		{"1", "-", fx.a4[:8], rsmLocalTime(plus(5)), "0", "0", "(未绑定)", "归档四 [已归档]"},
		{"2", "-", fx.a3[:8], rsmLocalTime(plus(4)), "0", "0", fx.other, "归档三 [已归档]"},
		{"3", "-", fx.a2[:8], rsmLocalTime(plus(3)), "0", "0", fx.projectDir, "归档二 [已归档]"},
		{"4", "-", fx.a1[:8], rsmLocalTime(plus(2)), "0", "0", fx.projectDir, "归档一 [已归档]"},
		{rsmArchivedHint},
	}
	rsmAssertRows(t, got, want)
	if strings.Contains(got, "另有") {
		t.Errorf("--all --archived 不该提示未绑定归档会话（它们已经列在表里）；输出:\n%s", got)
	}
	for _, id := range []string{fx.p1, fx.p2} {
		if strings.Contains(got, id[:8]) {
			t.Errorf("归档视图里出现了活跃会话 %s；输出:\n%s", id, got)
		}
	}

	reversed := rsmRun(t, fx.r, fx.out, "/resume --archived --all")
	if reversed != got {
		t.Errorf("--archived --all 与 --all --archived 输出不同；前者:\n%s\n后者:\n%s", got, reversed)
	}

	// 同一批数据不带 --archived 时仍然是活跃表（回归的第二半）。
	plain := rsmRun(t, fx.r, fx.out, "/resume --all")
	if lines := rsmLines(plain); len(lines) == 0 || lines[0] != "全部会话（2 个）" {
		t.Errorf("不带 --archived 的 --all 第 1 行 = %q，期望 %q；输出:\n%s", firstOrEmpty(rsmLines(plain)), "全部会话（2 个）", plain)
	}
}

// TestRsmArchivedAttachThenSay 是最硬的一条：选中归档会话之后它与普通会话
// 完全一样 —— 概况照常回显，r.cur 是整份 Conversation（含归档位），
// 随后的 /say 落进同一条会话，/log 与 /who 读的也是它。
func TestRsmArchivedAttachThenSay(t *testing.T) {
	projectDir := rsmTempDir(t)
	r, f, out := rsmNewREPL(t, projectDir)
	const id = "archived-full-0001"
	const otherID = "live-other-0001"
	t0 := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	rsmSeedArchived(t, f, id, "归档会话", projectDir, t0)
	rsmSeed(t, f, otherID, "活跃会话", projectDir, t0.Add(time.Hour))
	f.seedParticipant(t, id, "alpha")
	f.seedMessage(t, id, "user", conversation.KindUser, "归档里的旧消息")

	attach := rsmRun(t, r, out, "/resume --archived 1")
	rsmAssertLines(t, attach, []string{
		"已接上会话 " + id + "，标题: 归档会话",
		"项目: " + projectDir,
		"成员 (1): alpha",
		"共 1 条消息；最近 1 条:",
		"  [1] user: 归档里的旧消息",
	})
	if rsmCurID(r) != id {
		t.Fatalf("/resume --archived 1 之后 r.cur.ID = %q，期望 %q", rsmCurID(r), id)
	}
	// 会话状态与普通接上逐字段相同：同一份 Conversation 整份设进 r.cur，
	// 没有为归档会话另开一套状态（Archived 也只是它自带的一个字段）。
	if r.cur == nil || !r.cur.Archived || r.cur.Title != "归档会话" || r.cur.Cwd != projectDir {
		t.Errorf("接上之后 r.cur = %+v，期望整份归档会话（Archived=true, Title=归档会话, Cwd=%s）", r.cur, projectDir)
	}

	// /say 走同一个 r.cur：落进归档会话，seq 接着往下走。
	rsmRun(t, r, out, "/say 接上之后说的")
	msgs := f.msgs[id]
	if len(msgs) != 2 {
		t.Fatalf("归档会话里有 %d 条消息，期望 2 条：%+v", len(msgs), msgs)
	}
	if msgs[1].Text != "接上之后说的" || msgs[1].Seq != 2 || msgs[1].SenderID != conversation.ReservedSenderUser {
		t.Errorf("接上后 /say 落下的消息 = %+v，期望 user 说的「接上之后说的」，seq 2", msgs[1])
	}
	if got := f.totalMessages(); got != 2 {
		t.Errorf("消息总数 = %d，期望 2（只该写进归档会话）", got)
	}
	if got := len(f.msgs[otherID]); got != 0 {
		t.Errorf("活跃会话里有 %d 条消息，期望 0（/say 不该写到别的会话）", got)
	}

	// /log 与 /who 照常可用：needsConv 判的是 r.cur，归档会话与普通会话没有区别。
	logOut := rsmRun(t, r, out, "/log")
	for _, want := range []string{"归档里的旧消息", "接上之后说的"} {
		if !strings.Contains(logOut, want) {
			t.Errorf("/log 的输出里缺少 %q；完整输出:\n%s", want, logOut)
		}
	}
	if who := rsmRun(t, r, out, "/who"); !strings.Contains(who, "alpha") {
		t.Errorf("/who 的输出里缺少成员 alpha；完整输出:\n%s", who)
	}

	// 编号按归档表算：越界提示指向 --archived，否则用户会去看另一张表。
	oob := rsmRun(t, r, out, "/resume --archived 2")
	const wantOOB = "编号 2 超出范围（1..1），先敲 /resume --archived 看列表\n"
	if oob != wantOOB {
		t.Errorf("/resume --archived 2 输出 = %q，期望恰好 %q", oob, wantOOB)
	}
	if rsmCurID(r) != id {
		t.Errorf("越界之后 r.cur.ID = %q，期望仍是 %q", rsmCurID(r), id)
	}

	// id 前缀按视图找：不带 --archived 时不命中归档会话（那是在活跃表里找），
	// 带上之后与列表是同一张表，接上的就是它。
	rsmRun(t, r, out, "/resume "+otherID)
	if rsmCurID(r) != otherID {
		t.Fatalf("切到活跃会话失败：r.cur.ID = %q", rsmCurID(r))
	}
	miss := rsmRun(t, r, out, "/resume archived-")
	const wantMiss = "没有 id 以 archived- 开头的会话，用 /resume --all 看看有哪些\n"
	if miss != wantMiss {
		t.Errorf("/resume archived- 输出 = %q，期望恰好 %q", miss, wantMiss)
	}
	if rsmCurID(r) != otherID {
		t.Errorf("零匹配之后 r.cur.ID = %q，期望仍是 %q", rsmCurID(r), otherID)
	}
	hit := rsmRun(t, r, out, "/resume --archived archived-")
	if !strings.HasPrefix(hit, "已接上会话 "+id+"，标题: 归档会话") {
		t.Errorf("/resume --archived archived- 没有接上归档会话；输出:\n%s", hit)
	}
	if rsmCurID(r) != id {
		t.Errorf("前缀接上之后 r.cur.ID = %q，期望 %q", rsmCurID(r), id)
	}
}

// TestRsmArchivedListEmpty 锁归档视图的三种「没有」说法：本项目没有、
// 全局没有（--all 与没有项目目录时同一句）。归档视图不能沿用活跃视图的
// 「用 /new 建一个」：新建的会话是活跃的，不会出现在这张表里。
func TestRsmArchivedListEmpty(t *testing.T) {
	const noneHere = "本项目还没有归档会话，用 /resume --all --archived 看其它项目的\n"
	const noneAtAll = "还没有任何归档会话（在网页里归档的会话会出现在这里）\n"
	const unbound = "另有 1 个未绑定项目的归档会话，/resume --all --archived 查看\n"
	t0 := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)

	t.Run("有项目目录且没有归档会话", func(t *testing.T) {
		projectDir := rsmTempDir(t)
		r, f, out := rsmNewREPL(t, projectDir)
		rsmSeed(t, f, "live-only", "活跃", projectDir, t0)
		if got := rsmRun(t, r, out, "/resume --archived"); got != noneHere {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneHere)
		}
	})

	t.Run("只有别的项目和未绑定的归档会话", func(t *testing.T) {
		projectDir := rsmTempDir(t)
		r, f, out := rsmNewREPL(t, projectDir)
		rsmSeedArchived(t, f, "arch-other", "别的", `X:\别的项目`, t0)
		rsmSeedArchived(t, f, "arch-unbound", "未绑定", "", t0)
		if got := rsmRun(t, r, out, "/resume --archived"); got != noneHere+unbound {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneHere+unbound)
		}
	})

	t.Run("全部且库是空的", func(t *testing.T) {
		r, _, out := rsmNewREPL(t, rsmTempDir(t))
		if got := rsmRun(t, r, out, "/resume --all --archived"); got != noneAtAll {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneAtAll)
		}
	})

	t.Run("没有项目目录", func(t *testing.T) {
		r, _, out := rsmNewREPL(t, "")
		if got := rsmRun(t, r, out, "/resume --archived"); got != noneAtAll {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneAtAll)
		}
	})
}

// TestRsmArchivedListStoreFailure 锁归档列表读库失败：只打一行、不接上、
// 不动当前会话；默认列表是另一条读路径，不受影响。
func TestRsmArchivedListStoreFailure(t *testing.T) {
	projectDir := rsmTempDir(t)
	_, f, _ := rsmNewREPL(t, projectDir)
	rsmSeed(t, f, "keep-cur", "当前", projectDir, time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC))
	rsmSeedArchived(t, f, "arch-one", "归档", projectDir, time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC))
	flaky := &rsmFlakyStore{fakeStore: f}
	r, out := rsmNewREPLOn(t, conversation.NewService(flaky), projectDir)
	rsmRun(t, r, out, "/use keep-cur")
	if rsmCurID(r) != "keep-cur" {
		t.Fatalf("前提不成立：/use 之后 r.cur.ID = %q", rsmCurID(r))
	}

	flaky.failArchivedList = true
	got := rsmRun(t, r, out, "/resume --archived")
	if !strings.HasPrefix(got, "列出会话失败: ") {
		t.Errorf("输出 = %q，期望以 %q 开头", got, "列出会话失败: ")
	}
	if !strings.Contains(got, errRsmBoom.Error()) {
		t.Errorf("输出 %q 不含哨兵文本 %q", got, errRsmBoom.Error())
	}
	if rsmCurID(r) != "keep-cur" {
		t.Errorf("归档列表读失败后 r.cur.ID = %q，期望仍是 %q", rsmCurID(r), "keep-cur")
	}

	// 两张表是两条读路径：归档表失败不该连累默认列表。
	plain := rsmRun(t, r, out, "/resume")
	if strings.Contains(plain, "列出会话失败") {
		t.Errorf("归档表失败时默认列表也失败了；输出:\n%s", plain)
	}
	if !strings.Contains(plain, "keep-cur") {
		t.Errorf("默认列表里应当仍有 keep-cur；输出:\n%s", plain)
	}
}

// TestRsmArchivedUsageMentionsFlag 锁用法说明：usage 常量、注册表的 Args 与
// Summary、/help 那一条都必须写上 --archived，否则用户不知道有这个入口。
func TestRsmArchivedUsageMentionsFlag(t *testing.T) {
	const wantUsage = "用法: /resume [--all] [--archived] [<编号|id 前缀>]"
	if resumeUsage != wantUsage {
		t.Errorf("resumeUsage = %q，期望 %q", resumeUsage, wantUsage)
	}

	spec, cmd, ok := newCommandRegistry().Lookup("resume")
	if !ok {
		t.Fatal("命令表里没有 resume")
	}
	if !strings.Contains(spec.Args, "--archived") {
		t.Errorf("Args = %q，期望含 --archived", spec.Args)
	}
	if !strings.Contains(spec.Summary, "--archived") {
		t.Errorf("Summary = %q，期望含 --archived", spec.Summary)
	}
	// 「标题后标 [已归档]」是这次给用户看的唯一可见约定，帮助里也要说出来。
	if !strings.Contains(spec.Summary, resumeArchivedMark) {
		t.Errorf("Summary = %q，期望含标记 %q", spec.Summary, resumeArchivedMark)
	}
	if cmd.run == nil {
		t.Error("resume.run 是 nil，期望指向 cmdResume")
	}

	fx := rsmNewArchivedFixture(t)
	bad := rsmRun(t, fx.r, fx.out, "/resume --archived --not-a-flag")
	if bad != resumeUsage+"\n" {
		t.Errorf("参数非法时输出 = %q，期望恰好 %q", bad, resumeUsage+"\n")
	}

	help := rsmRun(t, fx.r, fx.out, "/help")
	var line string
	for _, l := range strings.Split(help, "\n") {
		if strings.HasPrefix(l, "/resume") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("/help 里没有 /resume 这一行；输出:\n%s", help)
	}
	if !strings.Contains(line, "--archived") {
		t.Errorf("/help 里 /resume 这一行 = %q，期望含 --archived", line)
	}
}
