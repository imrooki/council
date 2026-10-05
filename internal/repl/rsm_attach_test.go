package repl

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// rsmListIDs 从 /resume 列表输出里取出数据行的 ID 列（前 8 个字符）。
// 表头和说明行没有 NO 这种数字开头，跳过。
func rsmListIDs(t *testing.T, out string) []string {
	t.Helper()
	var ids []string
	for _, line := range rsmLines(out) {
		cells := rsmCells(line)
		if len(cells) < 3 || cells[0] == "NO" {
			continue
		}
		if cells[0] == "" || cells[0][0] < '0' || cells[0][0] > '9' {
			continue
		}
		ids = append(ids, cells[2])
	}
	return ids
}

func rsmAssertLines(t *testing.T, out string, want []string) {
	t.Helper()
	got := rsmLines(out)
	if len(got) != len(want) {
		t.Fatalf("输出有 %d 行，期望 %d 行\n实际:\n%s\n期望:\n%s", len(got), len(want), out, strings.Join(want, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 行 = %q，期望 %q", i+1, got[i], want[i])
		}
	}
}

func rsmCurID(r *REPL) string {
	if r.cur == nil {
		return ""
	}
	return r.cur.ID
}

func rsmCurCwd(r *REPL) string {
	if r.cur == nil {
		return "<nil>"
	}
	return r.cur.Cwd
}

func TestRsmSelectByNumber(t *testing.T) {
	fx := rsmListFixture(t)

	out := rsmRun(t, fx.r, fx.out, "/resume 2")
	if rsmCurID(fx.r) != fx.p3 {
		t.Errorf("/resume 2 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p3)
	}
	if rsmCurCwd(fx.r) != fx.projectDir {
		t.Errorf("/resume 2 之后 Cwd = %q，期望 %q", rsmCurCwd(fx.r), fx.projectDir)
	}
	lines := rsmLines(out)
	wantFirst := "已接上会话 p3-cccccccccccc，标题: 含 制表符 换行的标题"
	if len(lines) == 0 || lines[0] != wantFirst {
		t.Errorf("第 1 行 = %q，期望 %q", firstOrEmpty(lines), wantFirst)
	}

	rsmRun(t, fx.r, fx.out, "/resume 1")
	if rsmCurID(fx.r) != fx.p1 {
		t.Errorf("/resume 1 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p1)
	}
	rsmRun(t, fx.r, fx.out, "/resume 3")
	if rsmCurID(fx.r) != fx.p2 {
		t.Errorf("/resume 3 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p2)
	}
	rsmRun(t, fx.r, fx.out, "/resume 02")
	if rsmCurID(fx.r) != fx.p3 {
		t.Errorf("/resume 02 之后 r.cur.ID = %q，期望与 /resume 2 相同的 %q", rsmCurID(fx.r), fx.p3)
	}

	rsmRun(t, fx.r, fx.out, "/resume --all 2")
	if rsmCurID(fx.r) != fx.p1 {
		t.Errorf("/resume --all 2 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p1)
	}
	rsmRun(t, fx.r, fx.out, "/resume 2 --all")
	if rsmCurID(fx.r) != fx.p1 {
		t.Errorf("/resume 2 --all 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p1)
	}
	rsmRun(t, fx.r, fx.out, "/resume --all 1")
	if rsmCurID(fx.r) != fx.ot {
		t.Errorf("/resume --all 1 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.ot)
	}
	unOut := rsmRun(t, fx.r, fx.out, "/resume --all 3")
	if rsmCurID(fx.r) != fx.un {
		t.Errorf("/resume --all 3 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.un)
	}
	if !strings.Contains(unOut, "项目: (未绑定)") {
		t.Errorf("接上未绑定会话的输出里没有 %q；输出:\n%s", "项目: (未绑定)", unOut)
	}

	rsmAssertNumbersMatchList(t, fx, false)
	rsmAssertNumbersMatchList(t, fx, true)
}

// rsmSharedListStore 每次 Conversations 都返回同一份切片。
// fake 默认每次新建切片，r.cur 即使指向 store 返回的切片元素也看不出来。
// 让 store 每次返回同一份可改的切片，接上之后改这份切片里的元素，
// r.cur 的内容就必须不跟着变。
type rsmSharedListStore struct {
	*fakeStore
	shared []conversation.Conversation
}

func (s *rsmSharedListStore) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	return s.shared, nil
}

func TestRsmAttachCurIsCopyOfStoreElement(t *testing.T) {
	fx := rsmListFixture(t)
	shared, err := fx.f.Conversations(context.Background())
	if err != nil {
		t.Fatalf("取会话列表失败: %v", err)
	}
	r, out := rsmNewREPLOn(t, conversation.NewService(&rsmSharedListStore{fakeStore: fx.f, shared: shared}), fx.projectDir)

	// 这条测试能抓到 r.cur 指向 store 返回的切片元素。
	// r.cur 指向 loadResumeRows 内部局部切片的元素则观察不到，
	// 那一点只靠 attachResume 按值收参数、取自己形参的地址。
	check := func(line string) {
		t.Helper()
		rsmRun(t, r, out, line)
		if r.cur == nil {
			t.Fatalf("%s 之后 r.cur 是 nil", line)
		}
		id, title, cwd := r.cur.ID, r.cur.Title, r.cur.Cwd
		idx := -1
		for i := range shared {
			if shared[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Fatalf("%s 接上的 %q 不在 shared 里", line, id)
		}
		oldTitle, oldCwd := shared[idx].Title, shared[idx].Cwd
		shared[idx].Title = "被改过"
		shared[idx].Cwd = "被改过"
		if r.cur.Title != title || r.cur.Cwd != cwd {
			t.Errorf("%s 之后改 shared 里的元素，r.cur 跟着变了：Title=%q Cwd=%q，接上时是 %q / %q",
				line, r.cur.Title, r.cur.Cwd, title, cwd)
		}
		shared[idx].Title = oldTitle
		shared[idx].Cwd = oldCwd
	}
	check("/resume 1")
	check("/resume --all 1")
	check("/resume " + fx.p2)
	check("/resume " + fx.p2[:4])
}

// rsmAssertNumbersMatchList 先取列表的 ID 列，再按编号接上，
// 两边的前 8 个字符必须对得上。编号每次重新计算，列表和选择必须是同一个口径。
func rsmAssertNumbersMatchList(t *testing.T, fx *rsmFixture, all bool) {
	t.Helper()
	line := "/resume"
	if all {
		line = "/resume --all"
	}
	ids := rsmListIDs(t, rsmRun(t, fx.r, fx.out, line))
	if len(ids) == 0 {
		t.Fatalf("%s 没有数据行", line)
	}
	for n := 1; n <= len(ids); n++ {
		rsmRun(t, fx.r, fx.out, line+" "+strconv.Itoa(n))
		got := rsmCurID(fx.r)
		if len(got) > 8 {
			got = got[:8]
		}
		if got != ids[n-1] {
			t.Errorf("%s %d 接上的 id 前缀 = %q，列表第 %d 行 ID = %q", line, n, got, n, ids[n-1])
		}
	}
}

func TestRsmSelectNumberOutOfRange(t *testing.T) {
	fx := rsmListFixture(t)
	rsmRun(t, fx.r, fx.out, "/use "+fx.p2)
	if rsmCurID(fx.r) != fx.p2 {
		t.Fatalf("前提不成立：/use 之后 r.cur = %q", rsmCurID(fx.r))
	}

	cases := []struct {
		line string
		want string
	}{
		{"/resume 0", "编号 0 超出范围（1..3），先敲 /resume 看列表\n"},
		{"/resume 4", "编号 4 超出范围（1..3），先敲 /resume 看列表\n"},
		{"/resume 999", "编号 999 超出范围（1..3），先敲 /resume 看列表\n"},
		{"/resume --all 6", "编号 6 超出范围（1..5），先敲 /resume 看列表\n"},
	}
	for _, tc := range cases {
		got := rsmRun(t, fx.r, fx.out, tc.line)
		if got != tc.want {
			t.Errorf("%s 输出 = %q，期望恰好 %q", tc.line, got, tc.want)
		}
		if rsmCurID(fx.r) != fx.p2 {
			t.Errorf("%s 之后 r.cur.ID = %q，期望仍是 %q", tc.line, rsmCurID(fx.r), fx.p2)
		}
	}

	fx.r.cur = nil
	got := rsmRun(t, fx.r, fx.out, "/resume 0")
	if got != "编号 0 超出范围（1..3），先敲 /resume 看列表\n" {
		t.Errorf("r.cur 为 nil 时 /resume 0 输出 = %q", got)
	}
	if fx.r.cur != nil {
		t.Errorf("越界之后 r.cur = %+v，期望仍是 nil", fx.r.cur)
	}

	rsmSeed(t, fx.f, "9-prefix-conv", "数字开头", fx.other, fx.t0)
	got = rsmRun(t, fx.r, fx.out, "/resume 9")
	want := "编号 9 超出范围（1..3），先敲 /resume 看列表\n"
	if got != want {
		t.Errorf("/resume 9 输出 = %q，期望恰好 %q（不该回落成前缀去接 9-prefix-conv）", got, want)
	}
	if fx.r.cur != nil {
		t.Errorf("/resume 9 之后 r.cur = %+v，期望仍是 nil", fx.r.cur)
	}

	empty, _, emptyOut := rsmNewREPL(t, rsmTempDir(t))
	got = rsmRun(t, empty, emptyOut, "/resume 1")
	want = "编号 1 超出范围（当前没有可选的会话），先敲 /resume 看列表\n"
	if got != want {
		t.Errorf("空列表时 /resume 1 输出 = %q，期望恰好 %q", got, want)
	}
}

func TestRsmSelectByID(t *testing.T) {
	fx := rsmListFixture(t)

	rsmRun(t, fx.r, fx.out, "/resume p2-bbbbbbbbbbbb")
	if rsmCurID(fx.r) != fx.p2 {
		t.Errorf("完整 id 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p2)
	}
	rsmRun(t, fx.r, fx.out, "/resume p3-c")
	if rsmCurID(fx.r) != fx.p3 {
		t.Errorf("前缀 p3-c 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p3)
	}
	rsmRun(t, fx.r, fx.out, "/resume ot-")
	if rsmCurID(fx.r) != fx.ot {
		t.Errorf("默认模式下 /resume ot- 之后 r.cur.ID = %q，期望 %q（前缀不受项目过滤）", rsmCurID(fx.r), fx.ot)
	}
	unOut := rsmRun(t, fx.r, fx.out, "/resume un-e")
	if rsmCurID(fx.r) != fx.un {
		t.Errorf("/resume un-e 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.un)
	}
	if !strings.Contains(unOut, "项目: (未绑定)") {
		t.Errorf("接上未绑定会话的输出里没有 %q；输出:\n%s", "项目: (未绑定)", unOut)
	}
	rsmRun(t, fx.r, fx.out, "/resume --all p2")
	if rsmCurID(fx.r) != fx.p2 {
		t.Errorf("/resume --all p2 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p2)
	}

	rsmSeed(t, fx.f, "abc", "短", fx.projectDir, fx.t0)
	rsmSeed(t, fx.f, "abcd", "更长", fx.projectDir, fx.t0)
	rsmRun(t, fx.r, fx.out, "/resume abc")
	if rsmCurID(fx.r) != "abc" {
		t.Errorf("/resume abc 之后 r.cur.ID = %q，期望 abc（完整 id 优先于前缀）", rsmCurID(fx.r))
	}

	rsmSeed(t, fx.f, "1234-numeric", "纯数字前缀", fx.projectDir, fx.t0)
	rsmRun(t, fx.r, fx.out, "/resume 1234")
	if rsmCurID(fx.r) != "1234-numeric" {
		t.Errorf("/resume 1234 之后 r.cur.ID = %q，期望 1234-numeric（四位纯数字当前缀）", rsmCurID(fx.r))
	}
	none := rsmRun(t, fx.r, fx.out, "/resume 5678")
	wantNone := "没有 id 以 5678 开头的会话，用 /resume --all 看看有哪些\n"
	if none != wantNone {
		t.Errorf("/resume 5678 输出 = %q，期望恰好 %q", none, wantNone)
	}
	if rsmCurID(fx.r) != "1234-numeric" {
		t.Errorf("零匹配之后 r.cur.ID = %q，期望仍是 1234-numeric", rsmCurID(fx.r))
	}

	rsmSeed(t, fx.f, "dup-2222", "二号", fx.projectDir, fx.t0)
	rsmSeed(t, fx.f, "dup-1111", "一号", fx.projectDir, fx.t0)
	before := rsmCurID(fx.r)
	dup := rsmRun(t, fx.r, fx.out, "/resume dup")
	dupLines := rsmLines(dup)
	if len(dupLines) < 1 || dupLines[0] != "id 前缀 dup 匹配到 2 个会话，请给出更长的前缀:" {
		t.Errorf("多匹配第 1 行 = %q", firstOrEmpty(dupLines))
	}
	if len(dupLines) < 2 {
		t.Fatalf("多匹配输出行数不足:\n%s", dup)
	}
	rsmAssertCells(t, dupLines[1], []string{"ID", "TITLE"})
	if len(dupLines) != 4 {
		t.Fatalf("多匹配输出有 %d 行，期望 4 行:\n%s", len(dupLines), dup)
	}
	rsmAssertCells(t, dupLines[2], []string{"dup-1111", "一号"})
	rsmAssertCells(t, dupLines[3], []string{"dup-2222", "二号"})
	if strings.Contains(dup, "已接上") {
		t.Errorf("多匹配不该接上；输出:\n%s", dup)
	}
	if rsmCurID(fx.r) != before {
		t.Errorf("多匹配之后 r.cur.ID = %q，期望仍是 %q", rsmCurID(fx.r), before)
	}

	zzz := rsmRun(t, fx.r, fx.out, "/resume zzz")
	wantZZZ := "没有 id 以 zzz 开头的会话，用 /resume --all 看看有哪些\n"
	if zzz != wantZZZ {
		t.Errorf("/resume zzz 输出 = %q，期望恰好 %q", zzz, wantZZZ)
	}
	if rsmCurID(fx.r) != before {
		t.Errorf("零匹配之后 r.cur.ID = %q，期望仍是 %q", rsmCurID(fx.r), before)
	}
}

func TestRsmBadArgs(t *testing.T) {
	fx := rsmListFixture(t)
	rsmRun(t, fx.r, fx.out, "/use "+fx.p1)
	const usage = "用法: /resume [--all] [<编号|id 前缀>]\n"
	for _, line := range []string{"/resume --bogus", "/resume a b", "/resume --all a b", "/resume --all --bogus"} {
		got := rsmRun(t, fx.r, fx.out, line)
		if got != usage {
			t.Errorf("%s 输出 = %q，期望恰好 %q", line, got, usage)
		}
		if rsmCurID(fx.r) != fx.p1 {
			t.Errorf("%s 之后 r.cur.ID = %q，期望仍是 %q", line, rsmCurID(fx.r), fx.p1)
		}
	}
}

func TestRsmAttachSummary(t *testing.T) {
	projectDir := rsmTempDir(t)
	r, f, out := rsmNewREPL(t, projectDir)
	const id = "sum-full-id-0000"
	rsmSeed(t, f, id, "完整概况\t含制表符", projectDir, time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC))
	f.seedParticipant(t, id, "bob")
	f.seedParticipant(t, id, "alice")
	f.seedParticipant(t, id, "carol")
	if err := f.DeactivateParticipant(context.Background(), id, "carol", time.Now()); err != nil {
		t.Fatalf("让 carol 退出失败: %v", err)
	}
	msgs := []struct {
		sender string
		kind   conversation.MessageKind
		text   string
	}{
		{"user", conversation.KindUser, "老消息一"},
		{"alice", conversation.KindAgent, "老消息二"},
		{"system", conversation.KindSystem, "会话绑定 team t1"},
		{"alice", conversation.KindTask, "任务派发"},
		{"alice", conversation.KindTaskResult, "任务结果"},
		{"user", conversation.KindUser, "line one\nline two\tTabbed"},
		{"bob", conversation.KindAgent, strings.Repeat("字", 150)},
		{"user", conversation.KindUser, strings.Repeat("中", 99) + "AB"},
	}
	for _, m := range msgs {
		f.seedMessage(t, id, m.sender, m.kind, m.text)
	}
	got := rsmRun(t, r, out, "/resume "+id)
	want := []string{
		"已接上会话 sum-full-id-0000，标题: 完整概况 含制表符",
		"项目: " + projectDir,
		"成员 (2): alice, bob",
		"共 8 条消息；最近 6 条:",
		"  [3] system (system): 会话绑定 team t1",
		"  [4] alice (task): 任务派发",
		"  [5] alice (task_result): 任务结果",
		"  [6] user: line one line two Tabbed",
		"  [7] bob: " + strings.Repeat("字", 100) + "...",
		"  [8] user: " + strings.Repeat("中", 99) + "A...",
	}
	rsmAssertLines(t, got, want)
	if rsmCurID(r) != id {
		t.Errorf("r.cur.ID = %q，期望 %q", rsmCurID(r), id)
	}

	t.Run("恰好 100 个字符不截断", func(t *testing.T) {
		const shortID = "sum-exact-100"
		rsmSeed(t, f, shortID, "恰好", projectDir, time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC))
		f.seedMessage(t, shortID, "user", conversation.KindUser, strings.Repeat("字", 100))
		got := rsmRun(t, r, out, "/resume "+shortID)
		body := "  [1] user: " + strings.Repeat("字", 100)
		found := false
		for _, line := range rsmLines(got) {
			if line == body {
				found = true
			}
			if strings.Contains(line, "...") {
				t.Errorf("恰好 100 个字不该出现省略号，行 = %q", line)
			}
		}
		if !found {
			t.Errorf("没有行 %q；输出:\n%s", body, got)
		}
	})

	t.Run("消息条数", func(t *testing.T) {
		for _, n := range []int{3, 6, 7} {
			cid := "sum-n-" + strconv.Itoa(n)
			rsmSeed(t, f, cid, "条数", projectDir, time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC))
			for i := 1; i <= n; i++ {
				f.seedMessage(t, cid, "user", conversation.KindUser, "m"+strconv.Itoa(i))
			}
			got := rsmRun(t, r, out, "/resume "+cid)
			lines := rsmLines(got)
			shown := n
			if shown > 6 {
				shown = 6
			}
			head := "共 " + strconv.Itoa(n) + " 条消息；最近 " + strconv.Itoa(shown) + " 条:"
			if !rsmContainsLine(lines, head) {
				t.Errorf("%d 条时没有行 %q；输出:\n%s", n, head, got)
			}
			start := 1
			if n > 6 {
				start = n - 5
			}
			for seq := start; seq <= n; seq++ {
				row := "  [" + strconv.Itoa(seq) + "] user: m" + strconv.Itoa(seq)
				if !rsmContainsLine(lines, row) {
					t.Errorf("%d 条时没有行 %q", n, row)
				}
			}
		}
	})

	t.Run("空会话", func(t *testing.T) {
		const emptyID = "sum-empty"
		rsmSeed(t, f, emptyID, "空", "", time.Date(2026, 9, 20, 11, 0, 0, 0, time.UTC))
		got := rsmRun(t, r, out, "/resume "+emptyID)
		rsmAssertLines(t, got, []string{
			"已接上会话 sum-empty，标题: 空",
			"项目: (未绑定)",
			"成员 (0): 还没有，用 /add 或 /team use 加入",
			"还没有消息",
		})
	})

	t.Run("空正文", func(t *testing.T) {
		const blankID = "sum-blank"
		rsmSeed(t, f, blankID, "空正文", projectDir, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))
		f.seedMessage(t, blankID, "user", conversation.KindUser, "")
		got := rsmRun(t, r, out, "/resume "+blankID)
		if !rsmContainsLine(rsmLines(got), "  [1] user: -") {
			t.Errorf("空正文没有回显成占位符；输出:\n%s", got)
		}
	})
}

func rsmContainsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func TestRsmAttachWorksWithoutCurrentConversation(t *testing.T) {
	fx := rsmListFixture(t)
	fx.r.cur = nil
	before := map[string]int{}
	for id, msgs := range fx.f.msgs {
		before[id] = len(msgs)
	}
	totalBefore := fx.f.totalMessages()

	got := rsmRun(t, fx.r, fx.out, "/resume 1")
	if strings.Contains(got, msgNoConv) {
		t.Errorf("没有当前会话时不该答 %q；输出:\n%s", msgNoConv, got)
	}
	if rsmCurID(fx.r) != fx.p1 {
		t.Errorf("/resume 1 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.p1)
	}
	rsmRun(t, fx.r, fx.out, "/resume --all 1")
	if rsmCurID(fx.r) != fx.ot {
		t.Errorf("/resume --all 1 之后 r.cur.ID = %q，期望 %q", rsmCurID(fx.r), fx.ot)
	}
	rsmRun(t, fx.r, fx.out, "/say 你好")
	ot := fx.f.msgs[fx.ot]
	if len(ot) == 0 || ot[len(ot)-1].Text != "你好" {
		t.Errorf("接上后 /say 没有写进 %s；消息 = %+v", fx.ot, ot)
	}
	if fx.f.totalMessages() != totalBefore+1 {
		t.Errorf("消息总数 = %d，期望 %d", fx.f.totalMessages(), totalBefore+1)
	}
	for id, n := range before {
		gotN := len(fx.f.msgs[id])
		want := n
		if id == fx.ot {
			want = n + 1
		}
		if gotN != want {
			t.Errorf("会话 %s 的消息数 = %d，期望 %d", id, gotN, want)
		}
	}
}

func TestRsmAttachIsPureRead(t *testing.T) {
	fx := rsmListFixture(t)
	type snap struct {
		cwd, title string
		members    int
	}
	beforeConv := map[string]snap{}
	for _, c := range fx.f.convs {
		beforeConv[c.ID] = snap{cwd: c.Cwd, title: c.Title, members: len(fx.f.parts[c.ID])}
	}
	msgs := fx.f.totalMessages()
	nconv := len(fx.f.convs)
	ninf := len(fx.f.inflight)
	nsess := len(fx.f.providerSessions)

	for _, line := range []string{"/resume 2", "/resume --all 1", "/resume p2-bbbbbbbbbbbb"} {
		rsmRun(t, fx.r, fx.out, line)
	}
	if fx.f.totalMessages() != msgs {
		t.Errorf("消息数从 %d 变成 %d", msgs, fx.f.totalMessages())
	}
	if len(fx.f.convs) != nconv {
		t.Errorf("会话数从 %d 变成 %d", nconv, len(fx.f.convs))
	}
	if len(fx.f.inflight) != ninf {
		t.Errorf("inflight 从 %d 变成 %d", ninf, len(fx.f.inflight))
	}
	if len(fx.f.providerSessions) != nsess {
		t.Errorf("providerSessions 从 %d 变成 %d", nsess, len(fx.f.providerSessions))
	}
	for _, c := range fx.f.convs {
		s := beforeConv[c.ID]
		if c.Cwd != s.cwd || c.Title != s.title {
			t.Errorf("会话 %s 的 Cwd/Title 从 %q/%q 变成 %q/%q", c.ID, s.cwd, s.title, c.Cwd, c.Title)
		}
		if len(fx.f.parts[c.ID]) != s.members {
			t.Errorf("会话 %s 的成员数从 %d 变成 %d", c.ID, s.members, len(fx.f.parts[c.ID]))
		}
	}
}

func TestRsmAttachReadFailureKeepsSwitch(t *testing.T) {
	projectDir := rsmTempDir(t)
	_, f, _ := rsmNewREPL(t, projectDir)
	const id = "fail-attach"
	rsmSeed(t, f, id, "会失败", projectDir, time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC))
	f.seedMessage(t, id, "user", conversation.KindUser, "一条")
	f.seedMessage(t, id, "alpha", conversation.KindAgent, "两条")
	f.seedParticipant(t, id, "alpha")
	flaky := &rsmFlakyStore{fakeStore: f, failParts: map[string]bool{id: true}}
	r, out := rsmNewREPLOn(t, conversation.NewService(flaky), projectDir)

	got := rsmRun(t, r, out, "/resume "+id)
	if !strings.Contains(got, "成员 (?): 读取失败: ") || !strings.Contains(got, errRsmBoom.Error()) {
		t.Errorf("成员失败时输出不含失败说明；输出:\n%s", got)
	}
	if !strings.Contains(got, "共 2 条消息；最近 2 条:") {
		t.Errorf("成员失败时仍应打出消息头；输出:\n%s", got)
	}
	if !rsmContainsLine(rsmLines(got), "  [1] user: 一条") || !rsmContainsLine(rsmLines(got), "  [2] alpha: 两条") {
		t.Errorf("成员失败时两条消息行缺失；输出:\n%s", got)
	}
	if rsmCurID(r) != id {
		t.Errorf("成员失败后 r.cur.ID = %q，期望 %q", rsmCurID(r), id)
	}

	flaky.failParts = nil
	flaky.failMaxSeq = map[string]bool{id: true}
	got = rsmRun(t, r, out, "/resume "+id)
	if !strings.Contains(got, "消息: 读取失败: ") || !strings.Contains(got, errRsmBoom.Error()) {
		t.Errorf("MaxSeq 失败时输出不含消息失败说明；输出:\n%s", got)
	}
	if !strings.Contains(got, "成员 (1): alpha") {
		t.Errorf("MaxSeq 失败时成员行应正常；输出:\n%s", got)
	}
	if rsmCurID(r) != id {
		t.Errorf("MaxSeq 失败后 r.cur.ID = %q，期望 %q", rsmCurID(r), id)
	}

	flaky.failMaxSeq = nil
	flaky.failMessages = map[string]bool{id: true}
	got = rsmRun(t, r, out, "/resume "+id)
	if !strings.Contains(got, "消息: 读取失败: ") || !strings.Contains(got, errRsmBoom.Error()) {
		t.Errorf("Messages 失败时输出不含消息失败说明；输出:\n%s", got)
	}
	if rsmCurID(r) != id {
		t.Errorf("Messages 失败后 r.cur.ID = %q，期望 %q", rsmCurID(r), id)
	}
}

func TestRsmNewThenResume(t *testing.T) {
	d1 := rsmTempDir(t)
	r1, f, out1 := rsmNewREPL(t, d1)
	rsmRun(t, r1, out1, "/new 甲")
	rsmRun(t, r1, out1, "/new 乙")
	if r1.cur == nil {
		t.Fatal("/new 乙 之后 r.cur 是 nil")
	}
	listed := rsmRun(t, r1, out1, "/resume")
	lines := rsmLines(listed)
	if len(lines) == 0 || !strings.Contains(lines[0], "本项目 2 个会话") {
		t.Errorf("第 1 行 = %q，期望含 %q", firstOrEmpty(lines), "本项目 2 个会话")
	}
	var sawJia, sawYi, starOnYi bool
	for _, line := range lines {
		cells := rsmCells(line)
		if len(cells) < 7 || cells[0] == "NO" {
			continue
		}
		title := cells[len(cells)-1]
		if title == "甲" {
			sawJia = true
		}
		if title == "乙" {
			sawYi = true
			starOnYi = cells[1] == "*"
		}
	}
	if !sawJia || !sawYi {
		t.Errorf("列表里甲=%v 乙=%v，期望都在；输出:\n%s", sawJia, sawYi, listed)
	}
	if !starOnYi {
		t.Errorf("星号不在乙那一行；输出:\n%s", listed)
	}

	d2 := rsmTempDir(t)
	r2, out2 := rsmNewREPLOn(t, conversation.NewService(f), d2)
	empty := rsmRun(t, r2, out2, "/resume")
	wantEmpty := "本项目还没有会话，用 /new <标题> 建一个（/resume --all 看其它项目的）\n"
	if empty != wantEmpty {
		t.Errorf("另一个项目的 /resume 输出 = %q，期望恰好 %q", empty, wantEmpty)
	}
	all := rsmRun(t, r2, out2, "/resume --all")
	allLines := rsmLines(all)
	projHits := 0
	for _, line := range allLines {
		cells := rsmCells(line)
		if len(cells) < 8 || cells[0] == "NO" {
			continue
		}
		if cells[6] == d1 {
			projHits++
		}
	}
	if projHits != 2 {
		t.Errorf("PROJECT 列等于 d1 的行有 %d 行，期望 2 行；输出:\n%s", projHits, all)
	}

	rsmRun(t, r2, out2, "/resume 1 --all")
	if r2.cur == nil {
		t.Fatal("接上之后 r2.cur 是 nil")
	}
	stored, err := f.Conversation(context.Background(), r2.cur.ID)
	if err != nil {
		t.Fatalf("读回接上的会话失败: %v", err)
	}
	if stored.Cwd != d1 {
		t.Errorf("接上后库里的 Cwd = %q，期望仍是 %q", stored.Cwd, d1)
	}
	if r2.cur.Cwd != d1 {
		t.Errorf("接上后 r2.cur.Cwd = %q，期望 %q", r2.cur.Cwd, d1)
	}
	if stored.Title != "甲" && stored.Title != "乙" {
		t.Errorf("接上的会话标题 = %q，期望是甲或乙", stored.Title)
	}
}
