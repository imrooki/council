package repl

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// errRsmBoom 是故障装饰器故意返回的哨兵。
// 用独立错误，说明行里出现的文本才能和别的失败分开。
var errRsmBoom = errors.New("rsm boom")

// rsmLines 按换行切开，去掉末尾因结尾换行多出来的空串。
func rsmLines(out string) []string {
	lines := strings.Split(out, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// rsmCellSplit 按至少两个空格切 tabwriter 的一行。
// padding 是 2，单元格内部不能有连续空格，否则会多切出一格。
var rsmCellSplit = regexp.MustCompile(` {2,}`)

func rsmCells(line string) []string {
	return rsmCellSplit.Split(strings.TrimRight(line, " "), -1)
}

// rsmLocalTime 与 resumeTime 同一个格式。期望值用它算，不手写小时数。
func rsmLocalTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}

// rsmNewREPLOn 与 rsmNewREPL 同一套装配，但 svc 由调用方给。
// 读库失败用例要包一层故障装饰器，不能用 newTestService 造出来的那一份。
func rsmNewREPLOn(t *testing.T, svc *conversation.Service, projectDir string) (*REPL, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	r := New(newTestREPLConfig(), svc, nil, nil, &out, RulesContext{})
	if projectDir != "" {
		r.SetProjectDir(projectDir)
	}
	return r, &out
}

// rsmFlakyStore 让指定会话的某几项读失败，或让列会话整体失败。
// 内嵌 *fakeStore 提供其余 Store 方法；只覆盖要失败的几个。
type rsmFlakyStore struct {
	*fakeStore
	failParts    map[string]bool
	failMaxSeq   map[string]bool
	failMessages map[string]bool
	// emptyMessages 让 Messages 成功但返回空切片。MaxSeq 仍报有消息，
	// 末条读不到，LAST_ACTIVE 必须打 ?，不能当成「没有消息」。
	emptyMessages map[string]bool
	failList      bool
}

func (s *rsmFlakyStore) Conversations(ctx context.Context) ([]conversation.Conversation, error) {
	if s.failList {
		return nil, errRsmBoom
	}
	return s.fakeStore.Conversations(ctx)
}

func (s *rsmFlakyStore) Participants(ctx context.Context, convID string, includeInactive bool) ([]conversation.Participant, error) {
	if s.failParts[convID] {
		return nil, errRsmBoom
	}
	return s.fakeStore.Participants(ctx, convID, includeInactive)
}

func (s *rsmFlakyStore) MaxSeq(ctx context.Context, convID string) (int64, error) {
	if s.failMaxSeq[convID] {
		return 0, errRsmBoom
	}
	return s.fakeStore.MaxSeq(ctx, convID)
}

func (s *rsmFlakyStore) Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]conversation.Message, error) {
	if s.failMessages[convID] {
		return nil, errRsmBoom
	}
	if s.emptyMessages[convID] {
		return []conversation.Message{}, nil
	}
	return s.fakeStore.Messages(ctx, convID, afterSeq, limit)
}

// rsmFixture 是列表用例共用的五场会话。
// 最后活动：P1 +6h，P3 +4h，P2 +2h，OT +8h，UN +5h（无消息取创建时间）。
type rsmFixture struct {
	r          *REPL
	f          *fakeStore
	out        *bytes.Buffer
	projectDir string
	other      string
	p1, p2, p3 string
	ot, un     string
	t0         time.Time
}

func rsmListFixture(t *testing.T) *rsmFixture {
	t.Helper()
	fx := &rsmFixture{
		projectDir: rsmTempDir(t),
		other:      `X:\别的项目`,
		p1:         "p1-aaaaaaaaaaaa",
		p2:         "p2-bbbbbbbbbbbb",
		p3:         "p3-cccccccccccc",
		ot:         "ot-dddddddddddd",
		un:         "un-eeeeeeeeeeee",
		t0:         time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC),
	}
	fx.r, fx.f, fx.out = rsmNewREPL(t, fx.projectDir)
	plus := func(h int) time.Time { return fx.t0.Add(time.Duration(h) * time.Hour) }

	rsmSeed(t, fx.f, fx.p1, "项目一 会话", fx.projectDir, plus(0))
	rsmSeed(t, fx.f, fx.p2, "项目二", fx.projectDir, plus(2))
	rsmSeed(t, fx.f, fx.p3, "含\t制表符\n换行的标题", fx.projectDir, plus(3))
	rsmSeed(t, fx.f, fx.ot, "别的项目的会话", fx.other, plus(1))
	rsmSeed(t, fx.f, fx.un, "未绑定的旧会话", "", plus(5))

	m := fx.f.seedMessage(t, fx.p1, "user", conversation.KindUser, "文本")
	rsmSetMsgTime(fx.f, fx.p1, m.Seq, plus(1))
	m = fx.f.seedMessage(t, fx.p1, "user", conversation.KindUser, "文本")
	rsmSetMsgTime(fx.f, fx.p1, m.Seq, plus(6))

	m = fx.f.seedMessage(t, fx.p3, "user", conversation.KindUser, "文本")
	rsmSetMsgTime(fx.f, fx.p3, m.Seq, plus(4))

	m = fx.f.seedMessage(t, fx.ot, "user", conversation.KindUser, "文本")
	rsmSetMsgTime(fx.f, fx.ot, m.Seq, plus(2))
	m = fx.f.seedMessage(t, fx.ot, "user", conversation.KindUser, "文本")
	rsmSetMsgTime(fx.f, fx.ot, m.Seq, plus(3))
	m = fx.f.seedMessage(t, fx.ot, "user", conversation.KindUser, "文本")
	rsmSetMsgTime(fx.f, fx.ot, m.Seq, plus(8))

	fx.f.seedParticipant(t, fx.p1, "alpha")
	fx.f.seedParticipant(t, fx.p1, "beta")
	fx.f.seedParticipant(t, fx.p1, "gamma")
	if err := fx.f.DeactivateParticipant(context.Background(), fx.p1, "gamma", plus(7)); err != nil {
		t.Fatalf("让 gamma 退出失败: %v", err)
	}
	fx.f.seedParticipant(t, fx.p3, "alpha")
	fx.f.seedParticipant(t, fx.ot, "alpha")
	return fx
}

func rsmAssertCells(t *testing.T, line string, want []string) {
	t.Helper()
	got := rsmCells(line)
	if len(got) != len(want) {
		t.Errorf("行 %q 有 %d 格，期望 %d 格 %v", line, len(got), len(want), want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("行 %q 第 %d 格 = %q，期望 %q", line, i, got[i], want[i])
		}
	}
}

const rsmListHint = "敲 /resume <编号> 接上；编号按最近活动排序，每次重新计算"

func TestRsmListDefaultFiltersByProject(t *testing.T) {
	fx := rsmListFixture(t)
	rsmRun(t, fx.r, fx.out, "/use "+fx.p3)
	out := rsmRun(t, fx.r, fx.out, "/resume")
	lines := rsmLines(out)

	plus := func(h int) time.Time { return fx.t0.Add(time.Duration(h) * time.Hour) }
	want := [][]string{
		{"项目: " + fx.projectDir + "（本项目 3 个会话；/resume --all 看全部）"},
		{"NO", "CUR", "ID", "LAST_ACTIVE", "MEMBERS", "MSGS", "TITLE"},
		{"1", "-", fx.p1[:8], rsmLocalTime(plus(6)), "2", "2", "项目一 会话"},
		{"2", "*", fx.p3[:8], rsmLocalTime(plus(4)), "1", "1", "含 制表符 换行的标题"},
		{"3", "-", fx.p2[:8], rsmLocalTime(plus(2)), "0", "0", "项目二"},
		{"另有 1 个未绑定项目的旧会话，/resume --all 查看"},
		{rsmListHint},
	}
	// 表头和数据行按格比；说明行整行比。P3 是当前会话，CUR 是星号。
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
	if strings.Contains(out, fx.ot[:8]) || strings.Contains(out, fx.un[:8]) {
		t.Errorf("默认列表不该出现别的项目或未绑定会话的 id 前缀；输出:\n%s", out)
	}
}

func TestRsmListAll(t *testing.T) {
	fx := rsmListFixture(t)
	out := rsmRun(t, fx.r, fx.out, "/resume --all")
	lines := rsmLines(out)
	plus := func(h int) time.Time { return fx.t0.Add(time.Duration(h) * time.Hour) }
	want := [][]string{
		{"全部会话（5 个）"},
		{"NO", "CUR", "ID", "LAST_ACTIVE", "MEMBERS", "MSGS", "PROJECT", "TITLE"},
		{"1", "-", fx.ot[:8], rsmLocalTime(plus(8)), "1", "3", fx.other, "别的项目的会话"},
		{"2", "-", fx.p1[:8], rsmLocalTime(plus(6)), "2", "2", fx.projectDir, "项目一 会话"},
		{"3", "-", fx.un[:8], rsmLocalTime(plus(5)), "0", "0", "(未绑定)", "未绑定的旧会话"},
		{"4", "-", fx.p3[:8], rsmLocalTime(plus(4)), "1", "1", fx.projectDir, "含 制表符 换行的标题"},
		{"5", "-", fx.p2[:8], rsmLocalTime(plus(2)), "0", "0", fx.projectDir, "项目二"},
		{rsmListHint},
	}
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
	if strings.Contains(out, "另有") {
		t.Errorf("--all 不该提示未绑定会话；输出:\n%s", out)
	}
}

func TestRsmListOrderingTieBreaks(t *testing.T) {
	r, f, out := rsmNewREPL(t, "")
	t0 := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	plus := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }

	// 最后活动相同（都是 +4h），CreatedAt 晚的排前：late 在 early 前面。
	rsmSeed(t, f, "early-aaaa", "早创建", "", plus(0))
	m := f.seedMessage(t, "early-aaaa", "user", conversation.KindUser, "文本")
	rsmSetMsgTime(f, "early-aaaa", m.Seq, plus(4))
	rsmSeed(t, f, "late-bbbbb", "晚创建", "", plus(3))
	m = f.seedMessage(t, "late-bbbbb", "user", conversation.KindUser, "文本")
	rsmSetMsgTime(f, "late-bbbbb", m.Seq, plus(4))

	// 最后活动与 CreatedAt 都相同，id 升序：aaa 在 zzz 前面。
	// fake 按插入倒序返回，所以必须先塞 aaa 后塞 zzz，输入顺序才与期望顺序相反。
	// 先塞 zzz 的话，稳定排序不看 id 也会排成 aaa 在前。
	rsmSeed(t, f, "tie-aaa", "同刻前", "", plus(1))
	rsmSeed(t, f, "tie-zzz", "同刻后", "", plus(1))

	// 最近有消息的老会话排在更晚创建但没消息的会话之前。
	rsmSeed(t, f, "old-msg", "老但有消息", "", plus(0))
	m = f.seedMessage(t, "old-msg", "user", conversation.KindUser, "文本")
	rsmSetMsgTime(f, "old-msg", m.Seq, plus(9))
	rsmSeed(t, f, "new-quiet", "新但没消息", "", plus(5))

	got := rsmRun(t, r, out, "/resume --all")
	lines := rsmLines(got)
	var ids []string
	for _, line := range lines {
		cells := rsmCells(line)
		if len(cells) < 3 || cells[0] == "NO" {
			continue
		}
		ids = append(ids, cells[2])
	}
	full := []string{"old-msg", "new-quiet", "late-bbbbb", "early-aaaa", "tie-aaa", "tie-zzz"}
	want := make([]string, len(full))
	for i, id := range full {
		want[i] = id
		if len(want[i]) > 8 {
			want[i] = want[i][:8]
		}
	}
	if len(ids) != len(want) {
		t.Fatalf("数据行 ID = %v，期望 %v；输出:\n%s", ids, want, got)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("第 %d 个 ID = %q，期望 %q（整体 %v）", i+1, ids[i], want[i], ids)
		}
	}
}

func TestRsmListEmpty(t *testing.T) {
	const noneHere = "本项目还没有会话，用 /new <标题> 建一个（/resume --all 看其它项目的）\n"
	const noneAtAll = "还没有任何会话，用 /new <标题> 建一个\n"
	const unbound = "另有 1 个未绑定项目的旧会话，/resume --all 查看\n"
	t0 := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)

	t.Run("有项目目录且库是空的", func(t *testing.T) {
		r, _, out := rsmNewREPL(t, rsmTempDir(t))
		got := rsmRun(t, r, out, "/resume")
		if got != noneHere {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneHere)
		}
	})

	t.Run("只有别的项目和未绑定", func(t *testing.T) {
		r, f, out := rsmNewREPL(t, rsmTempDir(t))
		rsmSeed(t, f, "ot-only", "别的", `X:\别的项目`, t0)
		rsmSeed(t, f, "un-only", "未绑定", "", t0)
		got := rsmRun(t, r, out, "/resume")
		if got != noneHere+unbound {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneHere+unbound)
		}
	})

	t.Run("只有别的项目", func(t *testing.T) {
		r, f, out := rsmNewREPL(t, rsmTempDir(t))
		rsmSeed(t, f, "ot-only", "别的", `X:\别的项目`, t0)
		got := rsmRun(t, r, out, "/resume")
		if got != noneHere {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneHere)
		}
	})

	t.Run("全部且库是空的", func(t *testing.T) {
		r, _, out := rsmNewREPL(t, rsmTempDir(t))
		got := rsmRun(t, r, out, "/resume --all")
		if got != noneAtAll {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneAtAll)
		}
	})

	t.Run("没有项目目录且库是空的", func(t *testing.T) {
		r, _, out := rsmNewREPL(t, "")
		got := rsmRun(t, r, out, "/resume")
		if got != noneAtAll {
			t.Errorf("输出 = %q，期望恰好 %q", got, noneAtAll)
		}
	})
}

func TestRsmListWithoutProjectDir(t *testing.T) {
	r, f, out := rsmNewREPL(t, "")
	t0 := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	rsmSeed(t, f, "bound-xx", "有目录", `X:\p`, t0)
	rsmSeed(t, f, "unboundx", "没目录", "", t0.Add(time.Hour))

	got := rsmRun(t, r, out, "/resume")
	lines := rsmLines(got)
	if len(lines) < 2 {
		t.Fatalf("输出行数不足:\n%s", got)
	}
	if lines[0] != "没有项目目录，列出全部会话（2 个）" {
		t.Errorf("第 1 行 = %q，期望 %q", lines[0], "没有项目目录，列出全部会话（2 个）")
	}
	head := rsmCells(lines[1])
	if len(head) != 8 || head[6] != "PROJECT" {
		t.Errorf("表头 = %v，期望 8 格且含 PROJECT", head)
	}
	if strings.Contains(got, "另有") {
		t.Errorf("没有项目目录时不该提示未绑定；输出:\n%s", got)
	}

	all := rsmRun(t, r, out, "/resume --all")
	allLines := rsmLines(all)
	if len(allLines) == 0 || allLines[0] != "全部会话（2 个）" {
		t.Errorf("/resume --all 第 1 行 = %q，期望 %q；输出:\n%s", firstOrEmpty(allLines), "全部会话（2 个）", all)
	}
}

func firstOrEmpty(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func TestRsmListMatchIsCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("盘符与路径大小写折叠只在 Windows 上验")
	}
	projectDir := rsmTempDir(t)
	r, f, out := rsmNewREPL(t, projectDir)
	t0 := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	lowerDrive := strings.ToLower(projectDir[:1]) + projectDir[1:]
	rsmSeed(t, f, "drive-lo", "盘符小写", lowerDrive, t0)

	got := rsmRun(t, r, out, "/resume")
	if !strings.Contains(got, "本项目 1 个会话") {
		t.Errorf("盘符小写的同一路径应当算本项目；输出:\n%s", got)
	}
	if !strings.Contains(got, "drive-lo") {
		t.Errorf("列表里应当有 drive-lo；输出:\n%s", got)
	}

	upperAll := strings.ToUpper(projectDir)
	rsmSeed(t, f, "name-up", "目录名大写", upperAll, t0.Add(time.Hour))
	got = rsmRun(t, r, out, "/resume")
	if !strings.Contains(got, "本项目 2 个会话") {
		t.Errorf("整段大写的同一路径应当算本项目；输出:\n%s", got)
	}
	if !strings.Contains(got, "name-up") {
		t.Errorf("列表里应当有 name-up；输出:\n%s", got)
	}
}

func TestRsmListReadFailuresShowQuestionMark(t *testing.T) {
	projectDir := rsmTempDir(t)
	_, f, _ := rsmNewREPL(t, projectDir)
	t0 := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	plus := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	ids := []string{"fail-aaa", "fail-bbb", "fail-ccc"}
	for i, id := range ids {
		rsmSeed(t, f, id, "会话"+id, projectDir, plus(i))
		m := f.seedMessage(t, id, "user", conversation.KindUser, "文本")
		rsmSetMsgTime(f, id, m.Seq, plus(10+i))
		f.seedParticipant(t, id, "alpha")
	}
	flaky := &rsmFlakyStore{
		fakeStore:    f,
		failMaxSeq:   map[string]bool{ids[0]: true},
		failParts:    map[string]bool{ids[1]: true},
		failMessages: map[string]bool{ids[2]: true},
	}
	r, out := rsmNewREPLOn(t, conversation.NewService(flaky), projectDir)
	got := rsmRun(t, r, out, "/resume")
	lines := rsmLines(got)

	var data [][]string
	var notes []string
	for _, line := range lines {
		if strings.HasPrefix(line, "会话 ") {
			notes = append(notes, line)
			continue
		}
		cells := rsmCells(line)
		if len(cells) == 7 && (cells[0] == "1" || cells[0] == "2" || cells[0] == "3") {
			data = append(data, cells)
		}
	}
	if len(data) != 3 {
		t.Fatalf("数据行有 %d 行，期望 3 行；输出:\n%s", len(data), got)
	}
	// 排序：B 的末条 +11h，C 的创建时间 +2h，A 的创建时间 +0h。
	// C、A 的最后活动时间没能确定，LAST_ACTIVE 打 ?，排序仍用创建时间兜底。
	wantOrder := []string{ids[1][:8], ids[2][:8], ids[0][:8]}
	for i, id := range wantOrder {
		if data[i][2] != id {
			t.Errorf("第 %d 行 ID = %q，期望 %q", i+1, data[i][2], id)
		}
	}
	// 列：NO CUR ID LAST_ACTIVE MEMBERS MSGS TITLE
	if data[0][4] != "?" || data[0][5] != "1" || data[0][3] != rsmLocalTime(plus(11)) {
		t.Errorf("B 行 = %v，期望 MEMBERS=? MSGS=1 LAST_ACTIVE=%s", data[0], rsmLocalTime(plus(11)))
	}
	if data[1][4] != "1" || data[1][5] != "1" || data[1][3] != "?" {
		t.Errorf("C 行 = %v，期望 MEMBERS=1 MSGS=1 LAST_ACTIVE=?（取末条失败，不显示创建时间）", data[1])
	}
	if data[2][4] != "1" || data[2][5] != "?" || data[2][3] != "?" {
		t.Errorf("A 行 = %v，期望 MEMBERS=1 MSGS=? LAST_ACTIVE=?", data[2])
	}

	if len(notes) != 3 {
		t.Fatalf("说明行有 %d 行，期望 3 行；输出:\n%s", len(notes), got)
	}
	for _, id := range ids {
		prefix := "会话 " + id + " 读取失败（对应列显示 ?）: "
		found := false
		for _, note := range notes {
			if strings.HasPrefix(note, prefix) && strings.Contains(note, errRsmBoom.Error()) {
				found = true
			}
		}
		if !found {
			t.Errorf("没有以 %q 开头且含 %q 的说明行；说明行: %v", prefix, errRsmBoom.Error(), notes)
		}
	}
	if lines[len(lines)-1] != rsmListHint {
		t.Errorf("最后一行 = %q，期望 %q", lines[len(lines)-1], rsmListHint)
	}
}

func TestRsmListEmptyTailShowsQuestionMark(t *testing.T) {
	projectDir := rsmTempDir(t)
	_, f, _ := rsmNewREPL(t, projectDir)
	id := "empty-tail"
	rsmSeed(t, f, id, "末条读空", projectDir, time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC))
	f.seedMessage(t, id, "user", conversation.KindUser, "一条")
	f.seedParticipant(t, id, "alpha")
	flaky := &rsmFlakyStore{
		fakeStore:     f,
		emptyMessages: map[string]bool{id: true},
	}
	r, out := rsmNewREPLOn(t, conversation.NewService(flaky), projectDir)
	got := rsmRun(t, r, out, "/resume")
	lines := rsmLines(got)

	var data []string
	var notes []string
	for _, line := range lines {
		if strings.HasPrefix(line, "会话 ") {
			notes = append(notes, line)
			continue
		}
		cells := rsmCells(line)
		if len(cells) == 7 && cells[0] == "1" {
			data = cells
		}
	}
	if data == nil {
		t.Fatalf("没有数据行；输出:\n%s", got)
	}
	// 列：NO CUR ID LAST_ACTIVE MEMBERS MSGS TITLE
	if data[3] != "?" || data[4] != "1" || data[5] != "1" {
		t.Errorf("数据行 = %v，期望 LAST_ACTIVE=? MEMBERS=1 MSGS=1", data)
	}
	prefix := "会话 " + id + " 读取失败（对应列显示 ?）: "
	const detail = "MaxSeq 报告有 1 条消息，但读不到末条"
	if len(notes) != 1 || !strings.HasPrefix(notes[0], prefix) || !strings.Contains(notes[0], detail) {
		t.Errorf("说明行 = %v，期望恰好一条以 %q 开头且含 %q", notes, prefix, detail)
	}
	if lines[len(lines)-1] != rsmListHint {
		t.Errorf("最后一行 = %q，期望 %q", lines[len(lines)-1], rsmListHint)
	}
}

func TestRsmListStoreFailure(t *testing.T) {
	projectDir := rsmTempDir(t)
	_, f, _ := rsmNewREPL(t, projectDir)
	rsmSeed(t, f, "keep-cur", "当前", projectDir, time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC))
	flaky := &rsmFlakyStore{fakeStore: f}
	r, out := rsmNewREPLOn(t, conversation.NewService(flaky), projectDir)
	rsmRun(t, r, out, "/use keep-cur")
	if r.cur == nil || r.cur.ID != "keep-cur" {
		t.Fatalf("前提不成立：/use 之后 r.cur = %+v", r.cur)
	}
	before := r.cur.ID
	flaky.failList = true
	got := rsmRun(t, r, out, "/resume")
	if !strings.HasPrefix(got, "列出会话失败: ") {
		t.Errorf("输出 = %q，期望以 %q 开头", got, "列出会话失败: ")
	}
	if !strings.Contains(got, errRsmBoom.Error()) {
		t.Errorf("输出 %q 不含哨兵文本 %q", got, errRsmBoom.Error())
	}
	if r.cur == nil || r.cur.ID != before {
		t.Errorf("列会话失败后 r.cur = %+v，期望仍是 %s", r.cur, before)
	}
}

func TestRsmListIsPureRead(t *testing.T) {
	fx := rsmListFixture(t)
	msgsBefore := fx.f.totalMessages()
	convsBefore := len(fx.f.convs)
	type snap struct {
		cwd, title string
		members    int
	}
	before := map[string]snap{}
	for _, c := range fx.f.convs {
		before[c.ID] = snap{cwd: c.Cwd, title: c.Title, members: len(fx.f.parts[c.ID])}
	}
	rsmRun(t, fx.r, fx.out, "/resume")
	rsmRun(t, fx.r, fx.out, "/resume --all")
	if fx.f.totalMessages() != msgsBefore {
		t.Errorf("消息数从 %d 变成 %d，期望不变", msgsBefore, fx.f.totalMessages())
	}
	if len(fx.f.convs) != convsBefore {
		t.Errorf("会话数从 %d 变成 %d，期望不变", convsBefore, len(fx.f.convs))
	}
	for _, c := range fx.f.convs {
		s := before[c.ID]
		if c.Cwd != s.cwd || c.Title != s.title {
			t.Errorf("会话 %s 的 Cwd/Title 从 %q/%q 变成 %q/%q", c.ID, s.cwd, s.title, c.Cwd, c.Title)
		}
		if len(fx.f.parts[c.ID]) != s.members {
			t.Errorf("会话 %s 的成员数从 %d 变成 %d（含不活跃）", c.ID, s.members, len(fx.f.parts[c.ID]))
		}
	}
}

func TestRsmListNeedsNoCurrentConversation(t *testing.T) {
	fx := rsmListFixture(t)
	fx.r.cur = nil
	got := rsmRun(t, fx.r, fx.out, "/resume")
	if strings.Contains(got, msgNoConv) {
		t.Errorf("没有当前会话时不该答 %q；输出:\n%s", msgNoConv, got)
	}
	if !strings.Contains(got, "NO") {
		t.Errorf("输出里应当有表头 NO；输出:\n%s", got)
	}

	var out bytes.Buffer
	r := New(newTestREPLConfig(), nil, nil, nil, &out, RulesContext{})
	none := rsmRun(t, r, &out, "/resume")
	if none != "持久化未就绪，本命令不可用\n" {
		t.Errorf("svc 为 nil 时输出 = %q，期望 %q", none, "持久化未就绪，本命令不可用\n")
	}
}

func TestRsmListProjectCellIsOneLine(t *testing.T) {
	r, f, out := rsmNewREPL(t, "")
	rsmSeed(t, f, "oneline-proj", "t", "X:\\a\tb\nc", time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC))
	got := rsmRun(t, r, out, "/resume --all")
	lines := rsmLines(got)
	if len(lines) != 4 {
		t.Fatalf("输出有 %d 行，期望 4 行；输出:\n%s", len(lines), got)
	}
	if lines[0] != "全部会话（1 个）" {
		t.Errorf("第 1 行 = %q，期望 %q", lines[0], "全部会话（1 个）")
	}
	head := rsmCells(lines[1])
	if len(head) != 8 {
		t.Errorf("表头有 %d 格，期望 8 格；行 %q", len(head), lines[1])
	}
	cells := rsmCells(lines[2])
	if len(cells) != 8 {
		t.Fatalf("数据行有 %d 格，期望 8 格；行 %q", len(cells), lines[2])
	}
	const wantProj = `X:\a b c`
	if cells[6] != wantProj {
		t.Errorf("PROJECT 格 = %q，期望 %q（制表符和换行各换成一个空格）", cells[6], wantProj)
	}
}
