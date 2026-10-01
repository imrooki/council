package tui

// 本文件覆盖冻结契约 RB-B v1.1 §2.4 的 H1、H2、H3a、H3b：真 tea.Program、真 REPL、真 Runner、假 CLI。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD a728cee 的源码。
// 装置 rbaNewTuiRig 与 TestMain 假 CLI 只读复用（rba_fakecli_test.go）；spy 是本文件自己的一份：
// 照 rbaSpy 的形状，多记 FIN（boardFinalizeMsg）、每条同时记下 Update 之后 chrome() 整个切片的拷贝与 frameHeight()。
// 日志里「P」指 printLine 正文。起 Program 的选项与顺序逐字照 RB-A §2.0 的真 Program 装置。
//
// 本文件引用本刀新增的 Speakers / Results 字段与 boardFinalizeMsg，在 HEAD 上整包编译不过，
// 这是契约 §2 写明的预期红。

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
	"council/internal/turn"
)

// rbbEntry 是 rbb spy 记下的一条 Update。
type rbbEntry struct {
	kind   string // P / RS / MD / RE / DONE / FIN / TICK
	text   string
	rs     repl.RoundStartEvent
	md     repl.MemberDoneEvent
	re     repl.RoundEndEvent
	err    error
	chrome []string
	fh     int
}

func (e rbbEntry) String() string {
	tail := fmt.Sprintf(" | fh=%d chrome=%q", e.fh, e.chrome)
	switch e.kind {
	case "P":
		return fmt.Sprintf("P %q", e.text) + tail
	case "RS":
		return fmt.Sprintf("RS %+v", e.rs) + tail
	case "MD":
		return fmt.Sprintf("MD %+v", e.md) + tail
	case "RE":
		return fmt.Sprintf("RE %+v", e.re) + tail
	case "DONE":
		return fmt.Sprintf("DONE err=%v", e.err) + tail
	}
	return e.kind + tail
}

// rbbSpy 包住 Model：Init、View 委托；Update 先委托、断回 Model 存回，再按消息记一条日志，返回 spy 自己。
type rbbSpy struct {
	m   Model
	mu  sync.Mutex
	log []rbbEntry
}

func (s *rbbSpy) Init() tea.Cmd  { return s.m.Init() }
func (s *rbbSpy) View() tea.View { return s.m.View() }

func (s *rbbSpy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := s.m.Update(msg)
	if nm, ok := next.(Model); ok {
		s.m = nm
	}
	var e rbbEntry
	switch v := msg.(type) {
	case roundStartMsg:
		e = rbbEntry{kind: "RS", rs: v.ev}
	case memberDoneMsg:
		e = rbbEntry{kind: "MD", md: v.ev}
	case roundEndMsg:
		e = rbbEntry{kind: "RE", re: v.ev}
	case submitDoneMsg:
		e = rbbEntry{kind: "DONE", err: v.err}
	case boardFinalizeMsg:
		e = rbbEntry{kind: "FIN"}
	case elapsedTickMsg:
		e = rbbEntry{kind: "TICK"}
	default:
		body, ok := rbbBody(msg)
		if !ok {
			return s, cmd
		}
		e = rbbEntry{kind: "P", text: body}
	}
	e.chrome = append([]string(nil), s.m.chrome()...)
	e.fh = s.m.frameHeight()
	s.mu.Lock()
	s.log = append(s.log, e)
	s.mu.Unlock()
	return s, cmd
}

func (s *rbbSpy) entries() []rbbEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]rbbEntry(nil), s.log...)
}

// rbbProg 是一台跑着的真 Program。
type rbbProg struct {
	p   *tea.Program
	spy *rbbSpy
}

// rbbStartProg 照 RB-A §2.0 的顺序起真 Program：先装观察者，再 SetEmit，再 go p.Run()。收尾 cancel 并等 Run 返回（5 秒）。
func rbbStartProg(t *testing.T, g *rbaTuiRig) *rbbProg {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	m := New(Options{Submit: replAdapter{r: g.r}, Out: g.lw, Version: "t", Ctx: ctx})
	spy := &rbbSpy{m: m}
	p := tea.NewProgram(spy,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutSignalHandler(),
		tea.WithWindowSize(100, 24),
	)
	g.r.SetRoundObserver(newRoundObserver(p.Send))
	g.lw.SetEmit(func(line string) { p.Send(printLine(line)) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = p.Run()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：cancel 之后 5 秒 Program.Run 仍没有返回")
		}
	})
	return &rbbProg{p: p, spy: spy}
}

func (pr *rbbProg) submit(text string) {
	pr.p.Send(tea.PasteMsg{Content: text})
	pr.p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func (pr *rbbProg) waitFor(d time.Duration, cond func(log []rbbEntry) bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond(pr.spy.entries()) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func rbbCount(log []rbbEntry, kind string) int {
	n := 0
	for _, e := range log {
		if e.kind == kind {
			n++
		}
	}
	return n
}

func rbbLogDump(log []rbbEntry) string {
	var b strings.Builder
	for i, e := range log {
		fmt.Fprintf(&b, "%3d %s\n", i, e.String())
	}
	return b.String()
}

// rbbDoneThenFin：日志里有第 n 个 DONE，且它之后有 FIN。
func rbbDoneThenFin(n int) func(log []rbbEntry) bool {
	return func(log []rbbEntry) bool {
		k := 0
		for i, e := range log {
			if e.kind == "DONE" {
				k++
				if k == n {
					return rbbCount(log[i+1:], "FIN") >= 1
				}
			}
		}
		return false
	}
}

// useConv 提交 /use 并等到第 1 个 DONE（/use 不出看板，没有 FIN）。
func (pr *rbbProg) useConv(t *testing.T, convID string) {
	t.Helper()
	pr.submit("/use " + convID)
	if !pr.waitFor(10*time.Second, func(log []rbbEntry) bool { return rbbCount(log, "DONE") >= 1 }) {
		t.Fatalf("前提不成立：10 秒内没有等到 /use 的 DONE；日志:\n%s", rbbLogDump(pr.spy.entries()))
	}
}

// rbbWindow 取回显 P（"> "+cmd）之后、到其后第一个 FIN 为止（含）的日志。
func rbbWindow(log []rbbEntry, cmd string) ([]rbbEntry, bool) {
	start := -1
	for i, e := range log {
		if e.kind == "P" && e.text == "> "+cmd {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	for i := start; i < len(log); i++ {
		if log[i].kind == "FIN" {
			return log[start : i+1], true
		}
	}
	return nil, false
}

// rbbEventKinds 取非 P、非 TICK 条目的种类序列与它们的下标。
func rbbEventKinds(log []rbbEntry) (string, []int) {
	var ks []string
	var idx []int
	for i, e := range log {
		if e.kind == "P" || e.kind == "TICK" {
			continue
		}
		ks = append(ks, e.kind)
		idx = append(idx, i)
	}
	return strings.Join(ks, " "), idx
}

// rbbWall 是 §2.4 的时长格：假 CLI 远小于 60 秒。
func rbbWall(ms int) string {
	return fmt.Sprintf("%d.%ds", ms/1000, (ms%1000)/100)
}

// rbbSeg 是日志里的一段：RS、它的 MD、RE 的下标。
type rbbSeg struct {
	rs  int
	mds []int
	re  int
}

// rbbSegments 按 RS / RE 切段。
func rbbSegments(log []rbbEntry) []rbbSeg {
	var segs []rbbSeg
	cur := -1
	for i, e := range log {
		switch e.kind {
		case "RS":
			segs = append(segs, rbbSeg{rs: i, re: -1})
			cur = len(segs) - 1
		case "MD":
			if cur >= 0 {
				segs[cur].mds = append(segs[cur].mds, i)
			}
		case "RE":
			if cur >= 0 {
				segs[cur].re = i
				cur = -1
			}
		}
	}
	return segs
}

// rbbMDByID 取一段里按 AgentID 索引的 MD。
func rbbMDByID(log []rbbEntry, sg rbbSeg) map[string]repl.MemberDoneEvent {
	out := map[string]repl.MemberDoneEvent{}
	for _, i := range sg.mds {
		out[log[i].md.AgentID] = log[i].md
	}
	return out
}

// rbbCheckResults 断言某段 RE 的 Results 与同段 MD 按 AgentID 一一对应且 DeepEqual，
// 且 Results 的 AgentID 序列等于同段 RS 的 Speakers。
func rbbCheckResults(t *testing.T, s int, log []rbbEntry, sg rbbSeg) {
	t.Helper()
	re := log[sg.re].re
	mds := rbbMDByID(log, sg)
	if len(re.Results) != len(sg.mds) || len(mds) != len(sg.mds) {
		t.Errorf("第 %d 段：Results %d 项、MD %d 个（去重后 %d），应当一一对应", s+1, len(re.Results), len(sg.mds), len(mds))
	}
	ids := make([]string, len(re.Results))
	for i, r := range re.Results {
		ids[i] = r.AgentID
		md, ok := mds[r.AgentID]
		if !ok {
			t.Errorf("第 %d 段 Results[%d]（%q）在同段 MD 里找不到", s+1, i, r.AgentID)
			continue
		}
		if !reflect.DeepEqual(r, md) {
			t.Errorf("第 %d 段 Results[%d] 应当与同段 %s 的 MD DeepEqual：\n  Results %+v\n  MD      %+v", s+1, i, r.AgentID, r, md)
		}
	}
	if sp := log[sg.rs].rs.Speakers; !reflect.DeepEqual(ids, sp) {
		t.Errorf("第 %d 段：Results 的 AgentID 序列 %q 应当等于同段 RS 的 Speakers %q", s+1, ids, sp)
	}
}

// rbbBlockBefore 取 RE（下标 re）之前紧挨着的 n 条 P（跳过夹在中间的 TICK），按日志顺序；
// 以及块之前那一条 P。碰到别的非 P 条目就停，返回 ok=false。
func rbbBlockBefore(log []rbbEntry, re, n int) (block []string, before string, ok bool) {
	i := re - 1
	for ; i >= 0 && len(block) < n; i-- {
		switch log[i].kind {
		case "TICK":
			continue
		case "P":
			block = append([]string{log[i].text}, block...)
		default:
			return block, "", false
		}
	}
	if len(block) < n {
		return block, "", false
	}
	for ; i >= 0; i-- {
		if log[i].kind == "P" {
			return block, log[i].text, true
		}
		if log[i].kind != "TICK" {
			break
		}
	}
	return block, "", false
}

// rbbCheckSegBlock 断言某段的块：第 1 条等于 title，其后按 order 每人一行等于 rowOf(那人的 MD)。
// 块之前那条 P 以「冷启动 预报 」或「意外冷启动 」开头（结算的最后一行，前提守卫，子串）。
func rbbCheckSegBlock(t *testing.T, s int, log []rbbEntry, sg rbbSeg, title string, order []string, rowOf func(md repl.MemberDoneEvent) string) {
	t.Helper()
	block, before, ok := rbbBlockBefore(log, sg.re, 1+len(order))
	if !ok {
		t.Errorf("第 %d 段：RE 之前紧挨着的不是 %d 条 P（块缺失或被别的事件隔开）；取到 %q", s+1, 1+len(order), block)
		return
	}
	if !strings.HasPrefix(before, "冷启动 预报 ") && !strings.HasPrefix(before, "意外冷启动 ") {
		t.Errorf("第 %d 段：块之前那条 P 应当是结算的最后一行（以「冷启动 预报 」或「意外冷启动 」开头），实际 %q", s+1, before)
	}
	if block[0] != title {
		t.Errorf("第 %d 段：块第 1 条应当整行等于 %q，实际 %q", s+1, title, block[0])
	}
	mds := rbbMDByID(log, sg)
	for i, id := range order {
		md, ok := mds[id]
		if !ok {
			t.Errorf("第 %d 段：没有 %s 的 MD", s+1, id)
			continue
		}
		if want := rowOf(md); block[1+i] != want {
			t.Errorf("第 %d 段：块第 %d 条（%s）应当整行等于\n  %q\n实际\n  %q", s+1, 2+i, id, want, block[1+i])
		}
	}
}

// rbbSegRID6 取一段 MD 里第一个非空的 RoundID 的末 6 位。
func rbbSegRID6(log []rbbEntry, sg rbbSeg) string {
	for _, i := range sg.mds {
		if rid := log[i].md.RoundID; rid != "" {
			return rbaLast6(rid)
		}
	}
	return ""
}

const rbbBudgetHead = "成员  输入token  金额  冷启动  在飞  命令行余量  依据"

// ═══ H1 ═══════════════════════════════════════════════════════════════

// TestRbbH1ThreeSegmentsBlocksSummaryResubmit：三段、块的位置、收尾行、finalize 后可再提交。
//
// 【判别力】这是跨并发原语的顺序那一条：块的 Send 与 LineWriter 的 Send 在同一个 goroutine、同一个 p.msgs，
// 收尾行经 Cmd。能为假：先发 RE 再发块（#13）→ RE 之前的不是块，第 3 条红；块改由 roundEndMsg 分支返回
// tea.Println（#15）→ 块跑到 RE 之后，第 3 条红；RE 不带 Results（#4）→ 没有块，第 3 条红；
// Results 的 WallMS / RoundID 写零值（#6、#7）→ 第 2 条 DeepEqual 红，且第 3 条的期望从 MD 算、块从 Results 拼，也红；
// 收尾行用 Batch 或提前发 → 第 5 条红；D4 不清 finalizing（#60）→ 第 7 条等不到 DONE，红；
// RS 不带 Speakers（#2）→ 永远不出看板、不 finalize，等 FIN 30 秒后前提不成立。
// 对照：RB-A O1 同一命令只看事件与表头。
func TestRbbH1ThreeSegmentsBlocksSummaryResubmit(t *testing.T) {
	g := rbaNewTuiRig(t)
	pr := rbbStartProg(t, g)
	pr.useConv(t, g.convID)

	const cmd = "/round rba_a rba_b rba_j --go --rounds 2 --with-judge"
	pr.submit(cmd)
	if !pr.waitFor(30*time.Second, rbbDoneThenFin(2)) {
		t.Fatalf("前提不成立：30 秒内没有等到第 2 个 DONE 与其后的 FIN；日志:\n%s", rbbLogDump(pr.spy.entries()))
	}
	all := pr.spy.entries()
	log, ok := rbbWindow(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有该命令的回显 P 或其后没有 FIN；日志:\n%s", rbbLogDump(all))
	}

	// 第 1 条。
	const wantKinds = "RS MD MD RE RS MD MD RE RS MD RE DONE FIN"
	kinds, idx := rbbEventKinds(log)
	if kinds != wantKinds {
		t.Fatalf("非 P、非 TICK 条目应当恰好是 %q，实际 %q；日志:\n%s", wantKinds, kinds, rbbLogDump(log))
	}
	doneAt, finAt := idx[len(idx)-2], idx[len(idx)-1]
	segs := rbbSegments(log)
	if len(segs) != 3 {
		t.Fatalf("前提不成立：应当切出 3 段，实际 %d", len(segs))
	}

	// 第 2、3 条。
	titles := []string{"发言轮 1/2 (%s)  回来 2/2", "发言轮 2/2 (%s)  回来 2/2", "裁判轮 (%s)  回来 1/1"}
	orders := [][]string{{"rba_a", "rba_b"}, {"rba_a", "rba_b"}, {"rba_j"}}
	spokeRow := func(md repl.MemberDoneEvent) string {
		return "  完成    " + md.AgentID + "  " + rbbWall(md.WallMS) + " 子进程"
	}
	for s, sg := range segs {
		rbbCheckResults(t, s, log, sg)
		rbbCheckSegBlock(t, s, log, sg, fmt.Sprintf(titles[s], rbbSegRID6(log, sg)), orders[s], spokeRow)
	}

	// 第 4 条。
	for s := 0; s < 2; s++ {
		next := ""
		for i := segs[s].re + 1; i < len(log); i++ {
			if log[i].kind == "P" {
				next = log[i].text
				break
			}
		}
		if next != rbbBudgetHead {
			t.Errorf("第 %d 个 RE 之后的第一条 P 应当整行等于下一段的预算表头 %q，实际 %q", s+1, rbbBudgetHead, next)
		}
	}
	for i := segs[2].re + 1; i < len(log); i++ {
		if log[i].kind == "TICK" {
			continue
		}
		if log[i].kind != "DONE" {
			t.Errorf("第 3 个 RE 之后下一条非 TICK 条目应当是 DONE，实际 %s", log[i].String())
		}
		break
	}

	// 第 5 条。
	var between []string
	for i := doneAt + 1; i < finAt; i++ {
		if log[i].kind == "P" {
			between = append(between, log[i].text)
		}
	}
	sumRe := regexp.MustCompile(`^/round 结束  \d+s  回来 5/5$`)
	if len(between) != 1 || !sumRe.MatchString(between[0]) {
		t.Errorf("DONE 与 FIN 之间应当恰好一条 P 且整行匹配 %s，实际 %q", sumRe, between)
	}
	if h := log[doneAt].chrome[0]; h != "council t   空闲" {
		t.Errorf("DONE 处的表头应当整串等于 %q，实际 %q", "council t   空闲", h)
	}

	// 第 6 条。
	for _, i := range idx[:len(idx)-1] {
		if log[i].fh != 10 {
			t.Errorf("%s 条目的帧高应当是 10（2 + floor 2 + 6），实际 %d", log[i].kind, log[i].fh)
		}
	}
	if log[finAt].fh != 8 {
		t.Errorf("FIN 条目的帧高应当是 8，实际 %d", log[finAt].fh)
	}
	runRe := regexp.MustCompile(`^  在跑    rba_[abj]  \d+s$`)
	for s, sg := range segs {
		sp := log[sg.rs].rs.Speakers
		for _, row := range log[sg.rs].chrome[2:] {
			if row != "" && !runRe.MatchString(row) {
				t.Errorf("第 %d 段 RS 条目的看板行应当匹配 %s 或为空串，实际 %q", s+1, runRe, row)
			}
		}
		for _, i := range sg.mds {
			md := log[i].md
			ch := log[i].chrome
			row := -1
			for k, id := range sp {
				if id == md.AgentID {
					row = k
				}
			}
			if row < 0 || 2+row >= len(ch) {
				t.Errorf("第 %d 段 MD(%s)：在名单 %q 里找不到它，或 chrome 太短 %q", s+1, md.AgentID, sp, ch)
				continue
			}
			if want := spokeRow(md); ch[2+row] != want {
				t.Errorf("第 %d 段 MD(%s) 条目的看板行应当整行等于 %q，实际 %q", s+1, md.AgentID, want, ch[2+row])
			}
			if s == 2 {
				if len(ch) != 4 || ch[3] != "" {
					t.Errorf("裁判段 MD 条目的 chrome 应当恰好 4 项、第 4 项是空串，实际 %q", ch)
				}
			}
		}
	}

	// 第 7 条。
	use := "/use " + g.convID
	pr.submit(use)
	if !pr.waitFor(10*time.Second, func(l []rbbEntry) bool { return rbbCount(l, "DONE") >= 3 }) {
		t.Fatalf("FIN 之后提交 %q：10 秒内没有出现第 3 个 DONE（finalize 之后 Enter 仍被挡？）；日志:\n%s", use, rbbLogDump(pr.spy.entries()))
	}
	all = pr.spy.entries()
	fins, sawEcho := 0, false
	for _, e := range all {
		if e.kind == "FIN" {
			fins++
		}
		if fins >= 1 && e.kind == "P" && e.text == "> "+use {
			sawEcho = true
		}
		if e.kind == "DONE" && fins >= 1 {
			break
		}
	}
	if !sawEcho {
		t.Errorf("第 3 个 DONE 之前应当有 FIN 之后的回显 P %q；日志:\n%s", "> "+use, rbbLogDump(all))
	}
}

// ═══ H2 ═══════════════════════════════════════════════════════════════

// TestRbbH2CancelPath：第 1 段在飞时 Ctrl-C。
//
// 【判别力】能为假：Results 的 Outcome 写死 Spoke（#9）或 MaybeAdvanced 写死 false（#10）→ DeepEqual 与块行红
// （RB1 抓不到这两个：那条命令里它们本来就是 Spoke / false）；忙时 Ctrl-C 不置 canceling（#65）→ 收尾行印「结束」，红。
// 收尾行用 status 文字而不是标志（#41）这条不抓（此时 status 恰是「正在取消」），由 B2-5 抓。
// 对照：H1 同一命令不取消。
func TestRbbH2CancelPath(t *testing.T) {
	t.Setenv("RBA_FAKE_SLEEP_MS", "3000")
	g := rbaNewTuiRig(t)
	pr := rbbStartProg(t, g)
	pr.useConv(t, g.convID)

	const cmd = "/round rba_a rba_b rba_j --go --rounds 2 --with-judge"
	pr.submit(cmd)
	if !pr.waitFor(10*time.Second, func(l []rbbEntry) bool { return rbbCount(l, "RS") >= 1 }) {
		t.Fatalf("前提不成立：10 秒内没有出现第 1 个 RS；日志:\n%s", rbbLogDump(pr.spy.entries()))
	}
	time.Sleep(300 * time.Millisecond)
	pr.p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !pr.waitFor(15*time.Second, rbbDoneThenFin(2)) {
		t.Fatalf("前提不成立：15 秒内没有等到第 2 个 DONE 与其后的 FIN；日志:\n%s", rbbLogDump(pr.spy.entries()))
	}
	all := pr.spy.entries()
	log, ok := rbbWindow(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有该命令的回显 P 或其后没有 FIN；日志:\n%s", rbbLogDump(all))
	}
	kinds, idx := rbbEventKinds(log)
	if kinds != "RS MD MD RE DONE FIN" {
		t.Fatalf("非 P、非 TICK 条目应当恰好是 %q，实际 %q；日志:\n%s", "RS MD MD RE DONE FIN", kinds, rbbLogDump(log))
	}
	segs := rbbSegments(log)
	sg := segs[0]
	for _, i := range sg.mds {
		if md := log[i].md; md.Outcome != turn.OutcomeCanceled || !md.MaybeAdvanced {
			t.Fatalf("前提不成立：MD 应当 Outcome=Canceled、MaybeAdvanced=true，实际 %+v", md)
		}
	}
	rbbCheckResults(t, 0, log, sg)
	rbbCheckSegBlock(t, 0, log, sg, fmt.Sprintf("发言轮 1/2 (%s)  回来 2/2", rbbSegRID6(log, sg)), []string{"rba_a", "rba_b"},
		func(md repl.MemberDoneEvent) string {
			return "  取消    " + md.AgentID + "  " + rbbWall(md.WallMS) + " 子进程  可能已吃下"
		})

	reAt, doneAt, finAt := idx[3], idx[4], idx[5]
	var tail, sum []string
	for i := reAt + 1; i < doneAt; i++ {
		if log[i].kind == "P" {
			tail = append(tail, log[i].text)
		}
	}
	for i := doneAt + 1; i < finAt; i++ {
		if log[i].kind == "P" {
			sum = append(sum, log[i].text)
		}
	}
	if len(tail) != 1 || tail[0] != "已跑 1/2 轮，其余取消" {
		t.Errorf("RE 与 DONE 之间应当恰好一条 P「已跑 1/2 轮，其余取消」，实际 %q", tail)
	}
	sumRe := regexp.MustCompile(`^/round 已取消  \d+s  回来 2/2$`)
	if len(sum) != 1 || !sumRe.MatchString(sum[0]) {
		t.Errorf("DONE 与 FIN 之间应当恰好一条 P 且整行匹配 %s，实际 %q", sumRe, sum)
	}
}

// ═══ H3 ═══════════════════════════════════════════════════════════════

// TestRbbH3aDriftedBlockMark：第 2 轮假 CLI 换了会话 id，第 2 段块的成员行带「(前缀已断)」。
//
// 【判别力】能为假：Results 的 Drifted 写死 false（#11）→ 第 2 段块行少标记、DeepEqual 红。
// 对照：第 1 段同一成员不带标记。
func TestRbbH3aDriftedBlockMark(t *testing.T) {
	t.Setenv("RBA_FAKE_RESUME_SESSION", "11111111-2222-4333-8444-555555555555")
	g := rbaNewTuiRig(t)
	pr := rbbStartProg(t, g)
	pr.useConv(t, g.convID)

	const cmd = "/round rba_a rba_b --go --rounds 2"
	pr.submit(cmd)
	if !pr.waitFor(30*time.Second, rbbDoneThenFin(2)) {
		t.Fatalf("前提不成立：30 秒内没有等到第 2 个 DONE 与其后的 FIN；日志:\n%s", rbbLogDump(pr.spy.entries()))
	}
	all := pr.spy.entries()
	log, ok := rbbWindow(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有该命令的回显 P 或其后没有 FIN；日志:\n%s", rbbLogDump(all))
	}
	if kinds, _ := rbbEventKinds(log); kinds != "RS MD MD RE RS MD MD RE DONE FIN" {
		t.Fatalf("前提不成立：序列应当是 %q，实际 %q；日志:\n%s", "RS MD MD RE RS MD MD RE DONE FIN", kinds, rbbLogDump(log))
	}
	segs := rbbSegments(log)
	for s, sg := range segs {
		for _, i := range sg.mds {
			if md := log[i].md; md.Drifted != (s == 1) || md.Outcome != turn.OutcomeSpoke {
				t.Fatalf("前提不成立：第 %d 段 MD 应当 Outcome=Spoke、Drifted=%v，实际 %+v", s+1, s == 1, md)
			}
		}
		rbbCheckResults(t, s, log, sg)
		mark := ""
		if s == 1 {
			mark = "  (前缀已断)"
		}
		rbbCheckSegBlock(t, s, log, sg, fmt.Sprintf("发言轮 %d/2 (%s)  回来 2/2", s+1, rbbSegRID6(log, sg)), []string{"rba_a", "rba_b"},
			func(md repl.MemberDoneEvent) string {
				return "  完成    " + md.AgentID + "  " + rbbWall(md.WallMS) + " 子进程" + mark
			})
	}
}

// TestRbbH3bDroppedBlockMark：rba_b 在飞时离开会话，它的块行是「未入库 … 正文见上方」。
//
// 【判别力】能为假：Results 的 Dropped 写死 false（#12）→ rba_b 的块行少标记、DeepEqual 红；
// Outcome 写死 Spoke（#9）→ 状态词红。对照：同段 rba_a 是「完成」、不带标记。
func TestRbbH3bDroppedBlockMark(t *testing.T) {
	t.Setenv("RBA_FAKE_SLEEP_MS", "1500")
	g := rbaNewTuiRig(t)
	pr := rbbStartProg(t, g)
	pr.useConv(t, g.convID)

	const cmd = "/round rba_a rba_b --go"
	pr.submit(cmd)
	if !pr.waitFor(10*time.Second, func(l []rbbEntry) bool { return rbbCount(l, "RS") >= 1 }) {
		t.Fatalf("前提不成立：10 秒内没有出现 RS；日志:\n%s", rbbLogDump(pr.spy.entries()))
	}
	if err := g.svc.Leave(context.Background(), g.convID, "rba_b", time.Now().UTC()); err != nil {
		t.Fatalf("前提不成立：rba_b 在飞时 Leave 失败: %v", err)
	}
	if !pr.waitFor(15*time.Second, rbbDoneThenFin(2)) {
		t.Fatalf("前提不成立：15 秒内没有等到第 2 个 DONE 与其后的 FIN；日志:\n%s", rbbLogDump(pr.spy.entries()))
	}
	all := pr.spy.entries()
	log, ok := rbbWindow(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有该命令的回显 P 或其后没有 FIN；日志:\n%s", rbbLogDump(all))
	}
	if kinds, _ := rbbEventKinds(log); kinds != "RS MD MD RE DONE FIN" {
		t.Fatalf("前提不成立：序列应当是 %q，实际 %q；日志:\n%s", "RS MD MD RE DONE FIN", kinds, rbbLogDump(log))
	}
	sg := rbbSegments(log)[0]
	mds := rbbMDByID(log, sg)
	if b := mds["rba_b"]; b.Outcome != turn.OutcomePostRejected || !b.Dropped {
		t.Fatalf("前提不成立：rba_b 的 MD 应当 Outcome=PostRejected、Dropped=true，实际 %+v", b)
	}
	rbbCheckResults(t, 0, log, sg)
	rbbCheckSegBlock(t, 0, log, sg, fmt.Sprintf("发言轮 (%s)  回来 2/2", rbbSegRID6(log, sg)), []string{"rba_a", "rba_b"},
		func(md repl.MemberDoneEvent) string {
			if md.AgentID == "rba_b" {
				return "  未入库  rba_b  " + rbbWall(md.WallMS) + " 子进程  正文见上方"
			}
			return "  完成    rba_a  " + rbbWall(md.WallMS) + " 子进程"
		})
}
