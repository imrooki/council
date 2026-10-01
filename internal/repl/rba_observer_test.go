package repl

// 本文件覆盖冻结契约 RB-A v1.2 §2.2 的 R2-R6：装了观察者之后屏幕文本不变、
// 三种事件的顺序与载荷、Members 的口径、早退与写屏幕失败时不发事件。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3143bc8 的源码。
// 装置（rbaNewRig、rbaBuf、rbaNormalize、rbaGolden）在 rba_golden_test.go。
//
// 本文件引用本刀新增的 RoundObserver / SetRoundObserver / 三种事件类型，在 HEAD 上整包编译不过，
// 这是契约 §2 写明的预期红。

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"council/internal/conversation"
	"council/internal/turn"
)

// ─── 记录型观察者 ─────────────────────────────────────────────────────

// rbaEv 是记录下来的一个事件。snap 是事件进门那一刻输出缓冲的整串内容。
type rbaEv struct {
	kind string // "RS" / "MD" / "RE"
	rs   RoundStartEvent
	md   MemberDoneEvent
	re   RoundEndEvent
	snap string
}

// rbaRec 是记录型观察者。MemberDone 在 worker goroutine 上调，所以加锁。
// snap 非 nil 时每个事件进门时调它把缓冲整串存下（R3、R4 的快照）。
type rbaRec struct {
	mu   sync.Mutex
	snap func() string
	evs  []rbaEv
}

var _ RoundObserver = (*rbaRec)(nil)

func (c *rbaRec) add(e rbaEv) {
	if c.snap != nil {
		e.snap = c.snap()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evs = append(c.evs, e)
}

func (c *rbaRec) RoundStart(ev RoundStartEvent) { c.add(rbaEv{kind: "RS", rs: ev}) }
func (c *rbaRec) MemberDone(ev MemberDoneEvent) { c.add(rbaEv{kind: "MD", md: ev}) }
func (c *rbaRec) RoundEnd(ev RoundEndEvent)     { c.add(rbaEv{kind: "RE", re: ev}) }

func (c *rbaRec) events() []rbaEv {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]rbaEv(nil), c.evs...)
}

// rbaKinds 把事件列表收成 "RS MD RE" 这样的种类序列。
func rbaKinds(evs []rbaEv) string {
	ks := make([]string, len(evs))
	for i, e := range evs {
		ks[i] = e.kind
	}
	return strings.Join(ks, " ")
}

// rbaDump 把事件列表印成可读形式，失败信息用。
func rbaDump(evs []rbaEv) string {
	var b strings.Builder
	for i, e := range evs {
		switch e.kind {
		case "RS":
			fmt.Fprintf(&b, "%d RS %+v\n", i, e.rs)
		case "MD":
			fmt.Fprintf(&b, "%d MD %+v\n", i, e.md)
		case "RE":
			fmt.Fprintf(&b, "%d RE %+v\n", i, e.re)
		}
	}
	return b.String()
}

// rbaCheckMD 比 MD 的 Label、AgentID、Outcome == Spoke、三个布尔为 false（R2 的比较口径）。
func rbaCheckMD(t *testing.T, where string, md MemberDoneEvent, label, agent string) {
	t.Helper()
	if md.Label != label || md.AgentID != agent || md.Outcome != turn.OutcomeSpoke ||
		md.MaybeAdvanced || md.Drifted || md.Dropped {
		t.Errorf("%s：MD 应当是 {Label:%s AgentID:%s Outcome:Spoke 三个布尔 false}，实际 %+v", where, label, agent, md)
	}
}

// rbaLines 把一段以换行结尾的文本切成行，丢掉末尾换行造成的那个空串。
func rbaLines(s string) []string {
	if s == "" {
		return nil
	}
	ls := strings.Split(s, "\n")
	if ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// rbaLastNonEmpty 取一段文本按 \n 切行后最后一个非空行。
func rbaLastNonEmpty(s string) string {
	ls := strings.Split(s, "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if ls[i] != "" {
			return ls[i]
		}
	}
	return ""
}

// rbaColdRe 是 R3 第 4 条的冷暖行正则，捕获 wall_ms。
var rbaColdRe = regexp.MustCompile(`^冷暖 预报=(冷|暖) 实测=(冷|暖|未知)  wall_ms=(\d+)  prompt_bytes=\d+  scanned_to=\d+$`)

// ─── 失败写者（R6） ──────────────────────────────────────────────────

// rbaFailWriter 是会按内容写失败的输出。failOn 非空且本次写入的内容含它时不写入、计数 +1、返回错误；
// onWrite 非 nil 时每次写入先调它（在判失败之前，在锁外调，免得它回头取同一把锁）。
type rbaFailWriter struct {
	mu      sync.Mutex
	b       strings.Builder
	failOn  string
	onWrite func(s string)
	fails   int
}

var rbaErrWrite = errors.New("rba 失败写者: 按约定写失败")

func (w *rbaFailWriter) Write(p []byte) (int, error) {
	s := string(p)
	w.mu.Lock()
	hook := w.onWrite
	w.mu.Unlock()
	if hook != nil {
		hook(s)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failOn != "" && strings.Contains(s, w.failOn) {
		w.fails++
		return 0, rbaErrWrite
	}
	return w.b.WriteString(s)
}

func (w *rbaFailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func (w *rbaFailWriter) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.b.Reset()
}

func (w *rbaFailWriter) arm(failOn string, onWrite func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failOn = failOn
	w.onWrite = onWrite
}

func (w *rbaFailWriter) failCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fails
}

// ═══ R2 ═══════════════════════════════════════════════════════════════

// TestRbaR2ObserverKeepsGoldenAndEmitsSix：装了观察者，屏幕文本仍与 golden 整串相等，
// 且事件恰好是 [RS{发言轮,1,1,1}, MD{rba_a}, RE{发言轮,1,1}, RS{裁判轮,0,0,1}, MD{rba_j}, RE{裁判轮,0,0}]。
//
// 【判别力】能为假：观察者被调用时顺手 printf 一行（变异 #18）→ 与 golden 不等；
// 只在观察者为 nil 时才不打印的写法也被抓（R1 装不上它，这里装上了）。
// 裁判轮传 1,1（变异 #5）→ 第 4 个事件不等于 {裁判轮,0,0,1}。
// 对照：R1 同一装置、同一命令，只差没装观察者。
func TestRbaR2ObserverKeepsGoldenAndEmitsSix(t *testing.T) {
	g := rbaNewRig(t)
	buf := &rbaBuf{}
	r := g.newREPL(t, buf, buf.Reset)
	rec := &rbaRec{}
	r.SetRoundObserver(rec)
	if err := r.Submit(g.ctx, rbaR1Command); err != nil {
		t.Fatalf("Submit(%q) 返回错误: %v；输出:\n%s", rbaR1Command, err, buf.String())
	}
	rbaCheckGolden(t, buf.String())

	evs := rec.events()
	if got := rbaKinds(evs); got != "RS MD RE RS MD RE" {
		t.Fatalf("事件种类序列应当恰好是 %q，实际 %q\n%s", "RS MD RE RS MD RE", got, rbaDump(evs))
	}
	if want := (RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 1}); rbaRS(evs[0].rs) != rbaRS(want) {
		t.Errorf("第 1 个事件 RS 应当是 %+v，实际 %+v", want, evs[0].rs)
	}
	rbaCheckMD(t, "第 2 个事件", evs[1].md, "发言轮", "rba_a")
	if want := (RoundEndEvent{Label: "发言轮", K: 1, N: 1}); rbaRE(evs[2].re) != rbaRE(want) {
		t.Errorf("第 3 个事件 RE 应当是 %+v，实际 %+v", want, evs[2].re)
	}
	if want := (RoundStartEvent{Label: "裁判轮", K: 0, N: 0, Members: 1}); rbaRS(evs[3].rs) != rbaRS(want) {
		t.Errorf("第 4 个事件 RS 应当是 %+v，实际 %+v", want, evs[3].rs)
	}
	rbaCheckMD(t, "第 5 个事件", evs[4].md, "裁判轮", "rba_j")
	if want := (RoundEndEvent{Label: "裁判轮", K: 0, N: 0}); rbaRE(evs[5].re) != rbaRE(want) {
		t.Errorf("第 6 个事件 RE 应当是 %+v，实际 %+v", want, evs[5].re)
	}
}

// ═══ R3 ═══════════════════════════════════════════════════════════════

// TestRbaR3OrderAndSnapshots：--rounds 2 --with-judge 下三段事件的顺序、载荷，
// 以及每个事件进门时屏幕已经打到哪里。
//
// 【判别力】能为假，逐条：
//   - MD 挪到 writeAskRecord 之前（变异 #6）→ 第 4 条的新增块为空（或属于上一个成员），红；
//   - RE 挪到结算之前（#15）→ 第 6 条新增块第一行不是「X结算」，红；
//   - RS 挪到「本轮问」之前（#1）→ 第 3 条快照最后一行是「扇出期间……」，红；
//   - k/N 传反（#4）或裁判传 1,1（#5）→ 第 2 条整值不等，红；
//   - WallMS 写 0（#12）→ 第 4 条冷暖行捕获的 wall_ms 不等于 0（真墙钟至少几毫秒；见 weak 说明），红；
//   - RoundID 写空（#13）→ 第 5 条「非空」红；Label 写空（#14）→ 第 5 条 Label 红。
//
// 对照：同一命令里三段各自是对照，第 2 段与第 1 段只差 k。
func TestRbaR3OrderAndSnapshots(t *testing.T) {
	g := rbaNewRig(t)
	buf := &rbaBuf{}
	r := g.newREPL(t, buf, buf.Reset)
	rec := &rbaRec{snap: buf.String}
	r.SetRoundObserver(rec)
	const cmd = "/round rba_a rba_b rba_j --go --rounds 2 --with-judge"
	if err := r.Submit(g.ctx, cmd); err != nil {
		t.Fatalf("Submit(%q) 返回错误: %v；输出:\n%s", cmd, err, buf.String())
	}
	evs := rec.events()

	// 第 1 条：种类序列。
	const wantKinds = "RS MD MD RE RS MD MD RE RS MD RE"
	if got := rbaKinds(evs); got != wantKinds {
		t.Fatalf("事件种类序列应当恰好是 %q，实际 %q\n%s\n输出:\n%s", wantKinds, got, rbaDump(evs), buf.String())
	}

	// 第 2 条：RS / RE 整值。
	wantRS := []RoundStartEvent{
		{Label: "发言轮", K: 1, N: 2, Members: 2},
		{Label: "发言轮", K: 2, N: 2, Members: 2},
		{Label: "裁判轮", K: 0, N: 0, Members: 1},
	}
	wantRE := []RoundEndEvent{
		{Label: "发言轮", K: 1, N: 2},
		{Label: "发言轮", K: 2, N: 2},
		{Label: "裁判轮", K: 0, N: 0},
	}
	rsIdx := []int{0, 4, 8}
	reIdx := []int{3, 7, 10}
	for s := 0; s < 3; s++ {
		if got := evs[rsIdx[s]].rs; rbaRS(got) != rbaRS(wantRS[s]) {
			t.Errorf("第 %d 段 RS 应当是 %+v，实际 %+v", s+1, wantRS[s], got)
		}
		if got := evs[reIdx[s]].re; rbaRE(got) != rbaRE(wantRE[s]) {
			t.Errorf("第 %d 段 RE 应当是 %+v，实际 %+v", s+1, wantRE[s], got)
		}
	}

	// 第 3 条：RS 快照的最后一个非空行。
	wantAsk := []string{"本轮问 2 个: rba_a, rba_b", "本轮问 2 个: rba_a, rba_b", "本轮问 1 个: rba_j"}
	for s := 0; s < 3; s++ {
		if got := rbaLastNonEmpty(evs[rsIdx[s]].snap); got != wantAsk[s] {
			t.Errorf("第 %d 段 RS 进门时屏幕最后一个非空行应当是 %q，实际 %q", s+1, wantAsk[s], got)
		}
	}

	// 第 4、5、6 条。
	segOf := func(i int) int {
		switch {
		case i < 4:
			return 0
		case i < 8:
			return 1
		}
		return 2
	}
	wantAgents := []map[string]bool{
		{"rba_a": true, "rba_b": true},
		{"rba_a": true, "rba_b": true},
		{"rba_j": true},
	}
	gotAgents := []map[string]bool{{}, {}, {}}
	segRID := []string{"", "", ""}
	for i, e := range evs {
		if i == 0 {
			continue
		}
		prev := evs[i-1].snap
		if !strings.HasPrefix(e.snap, prev) {
			t.Fatalf("前提不成立：第 %d 个事件的快照不以上一个事件的快照开头（缓冲只增不改的前提破了）", i+1)
		}
		block := rbaLines(e.snap[len(prev):])
		seg := segOf(i)
		switch e.kind {
		case "MD":
			md := e.md
			// 第 4 条。
			wantFirst := "outcome spoke  agent " + md.AgentID
			if len(block) == 0 || block[0] != wantFirst {
				first := "<空块>"
				if len(block) > 0 {
					first = block[0]
				}
				t.Errorf("第 %d 个事件 MD(%s)：相对上一个事件新增块的第一行应当是 %q，实际 %q\n块: %q", i+1, md.AgentID, wantFirst, first, block)
			}
			outcomes, colds := 0, 0
			for _, ln := range block {
				if strings.HasPrefix(ln, "outcome ") {
					outcomes++
				}
				if m := rbaColdRe.FindStringSubmatch(ln); m != nil {
					colds++
					if w, _ := strconv.Atoi(m[3]); w != md.WallMS {
						t.Errorf("第 %d 个事件 MD(%s)：冷暖行的 wall_ms=%d 与 ev.WallMS=%d 不等", i+1, md.AgentID, w, md.WallMS)
					}
				}
			}
			if outcomes != 1 {
				t.Errorf("第 %d 个事件 MD(%s)：新增块里以 outcome 开头的行应当恰好 1 行，实际 %d；块: %q", i+1, md.AgentID, outcomes, block)
			}
			if colds != 1 {
				t.Errorf("第 %d 个事件 MD(%s)：新增块里冷暖行应当恰好 1 行，实际 %d；块: %q", i+1, md.AgentID, colds, block)
			}
			// 第 5 条。
			if md.Label != wantRS[seg].Label {
				t.Errorf("第 %d 个事件 MD(%s)：Label 应当是本段的 %q，实际 %q", i+1, md.AgentID, wantRS[seg].Label, md.Label)
			}
			if md.Outcome != turn.OutcomeSpoke || md.MaybeAdvanced || md.Drifted || md.Dropped {
				t.Errorf("第 %d 个事件 MD：应当 Outcome=Spoke 且三个布尔为 false，实际 %+v", i+1, md)
			}
			if md.RoundID == "" {
				t.Errorf("第 %d 个事件 MD(%s)：RoundID 应当非空", i+1, md.AgentID)
			} else if segRID[seg] == "" {
				segRID[seg] = md.RoundID
			} else if segRID[seg] != md.RoundID {
				t.Errorf("第 %d 段两个 MD 的 RoundID 不等：%q 与 %q", seg+1, segRID[seg], md.RoundID)
			}
			gotAgents[seg][md.AgentID] = true
		case "RE":
			// 第 6 条。
			wantFirst := e.re.Label + "结算"
			if len(block) == 0 || block[0] != wantFirst {
				t.Errorf("第 %d 个事件 RE：相对上一个 MD 新增块的第一行应当是 %q；块: %q", i+1, wantFirst, block)
			}
			for _, ln := range block {
				if strings.HasPrefix(ln, "成员  ") {
					t.Errorf("第 %d 个事件 RE：新增块里不该有下一段的预算表行，实际有 %q", i+1, ln)
				}
			}
		}
	}
	for s := 0; s < 3; s++ {
		if len(gotAgents[s]) != len(wantAgents[s]) {
			t.Errorf("第 %d 段 MD 的 AgentID 集合应当是 %v，实际 %v", s+1, wantAgents[s], gotAgents[s])
			continue
		}
		for id := range wantAgents[s] {
			if !gotAgents[s][id] {
				t.Errorf("第 %d 段 MD 的 AgentID 集合应当是 %v，实际 %v", s+1, wantAgents[s], gotAgents[s])
				break
			}
		}
	}
	if segRID[0] != "" && (segRID[0] == segRID[1] || segRID[0] == segRID[2] || segRID[1] == segRID[2]) {
		t.Errorf("三段的 RoundID 应当两两不等，实际 %q", segRID)
	}
}

// ═══ R4 ═══════════════════════════════════════════════════════════════

// TestRbaR4MembersAfterSkipNoDelta：Members 取 skipNoDelta 之后的名单。
//
// 【判别力】能为假：rba_b 被判无新内容而跳过，预算表有两行、真正起进程的只有一人；
// `Members: len(rows)`（变异 #3）→ RS 是 {发言轮,1,1,2}，红。
// 对照：R3 同一字段在没人被跳过时等于名单长度。
func TestRbaR4MembersAfterSkipNoDelta(t *testing.T) {
	g := rbaNewRig(t)
	if _, err := g.rn.Ask(g.ctx, g.convID, "rba_b"); err != nil {
		t.Fatalf("前提不成立：/use 之前先问 rba_b 一次失败: %v", err)
	}
	buf := &rbaBuf{}
	r := g.newREPL(t, buf, buf.Reset)
	rec := &rbaRec{snap: buf.String}
	r.SetRoundObserver(rec)
	const cmd = "/round rba_a rba_b --go"
	if err := r.Submit(g.ctx, cmd); err != nil {
		t.Fatalf("Submit(%q) 返回错误: %v；输出:\n%s", cmd, err, buf.String())
	}
	skipRe := regexp.MustCompile(`(?m)^rba_b  .*  无新内容，本轮跳过$`)
	if !skipRe.MatchString(buf.String()) {
		t.Fatalf("前提不成立：预算表里没有 rba_b 被判无新内容的那一行；输出:\n%s", buf.String())
	}
	evs := rec.events()
	if got := rbaKinds(evs); got != "RS MD RE" {
		t.Fatalf("事件种类序列应当恰好是 %q，实际 %q\n%s", "RS MD RE", got, rbaDump(evs))
	}
	if want := (RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 1}); rbaRS(evs[0].rs) != rbaRS(want) {
		t.Errorf("RS 应当是 %+v（Members 数的是 skipNoDelta 之后的人），实际 %+v", want, evs[0].rs)
	}
	rbaCheckMD(t, "MD", evs[1].md, "发言轮", "rba_a")
	if want := (RoundEndEvent{Label: "发言轮", K: 1, N: 1}); rbaRE(evs[2].re) != rbaRE(want) {
		t.Errorf("RE 应当是 %+v，实际 %+v", want, evs[2].re)
	}
	if got := rbaLastNonEmpty(evs[0].snap); got != "本轮问 1 个: rba_a" {
		t.Errorf("RS 进门时屏幕最后一个非空行应当是 %q，实际 %q", "本轮问 1 个: rba_a", got)
	}
}

// ═══ R5 ═══════════════════════════════════════════════════════════════

// TestRbaR5EarlyExitEmitsNothing：没带 --go 的早退路径上一个事件都不发，屏幕与不装观察者时相同。
//
// 【判别力】能为假：RS 挪到 `if !goRun` 之前（变异 #2）→ 两段各发一个 RS，红；
// RE 放进 defer、每条 return 都发（#16）→ 两段各发一个 RE，红。
// 第二部分（与不装观察者的装置比屏幕）抓的是「装了观察者就多打东西」。
// 对照：同一命令加 --go（R2）会发满六个事件。
func TestRbaR5EarlyExitEmitsNothing(t *testing.T) {
	const cmd = "/round rba_a rba_j --with-judge"

	g1 := rbaNewRig(t)
	buf1 := &rbaBuf{}
	r1 := g1.newREPL(t, buf1, buf1.Reset)
	rec := &rbaRec{}
	r1.SetRoundObserver(rec)
	if err := r1.Submit(g1.ctx, cmd); err != nil {
		t.Fatalf("装了观察者：Submit(%q) 返回错误: %v", cmd, err)
	}
	if !strings.Contains(buf1.String(), "成员  输入token") {
		t.Fatalf("前提不成立：没带 --go 时也应当打预算表；输出:\n%s", buf1.String())
	}
	if evs := rec.events(); len(evs) != 0 {
		t.Errorf("没带 --go 的早退路径上不该发任何事件，实际 %q\n%s", rbaKinds(evs), rbaDump(evs))
	}

	g2 := rbaNewRig(t)
	buf2 := &rbaBuf{}
	r2 := g2.newREPL(t, buf2, buf2.Reset)
	if err := r2.Submit(g2.ctx, cmd); err != nil {
		t.Fatalf("不装观察者：Submit(%q) 返回错误: %v", cmd, err)
	}
	a, b := rbaNormalize(buf1.String()), rbaNormalize(buf2.String())
	if a != b {
		t.Errorf("装与不装观察者，规范化后的屏幕应当整串相等。%s", rbaFirstDiff(a, b))
	}
}

// ═══ R6 ═══════════════════════════════════════════════════════════════

// rbaR6Run 搭一个输出是失败写者的全新装置，按 prep 在 /use 之前做准备，
// 装记录型观察者，再按 arm 设好失败条件，Submit 被测命令。
// 返回事件、写者与 Submit 的返回值。Submit 返回非 nil 的写错误是预期（契约 R6 前提段）。
func rbaR6Run(t *testing.T, prep func(g *rbaRig), arm func(w *rbaFailWriter, cancel context.CancelFunc), cmd string) ([]rbaEv, *rbaFailWriter, error) {
	t.Helper()
	g := rbaNewRig(t)
	if prep != nil {
		prep(g)
	}
	w := &rbaFailWriter{}
	r := g.newREPL(t, w, w.Reset)
	rec := &rbaRec{}
	r.SetRoundObserver(rec)
	cmdCtx, cancel := context.WithCancel(g.ctx)
	defer cancel()
	arm(w, cancel)
	err := r.Submit(cmdCtx, cmd)
	return rec.events(), w, err
}

// TestRbaR6WriteFailureEmitsNoEvent：本次调用里任何一次写屏幕失败时，MD / RE 不发。
//
// 【判别力】能为假，逐条：
//   - R6a：writeAskRecord 失败那一支 return 之前照发 MD（变异 #46）→ 变成 `RS MD`，红；
//   - R6b：结算写失败的早退上照发 RE（#48）→ 变成 `RS MD RE`，红；
//   - R6c：`/round` 那一行写失败后不 return、回到 HEAD 写法（#47）→ 落到末尾照发 MD，变成 `RS MD`，红。
//
// 对照：R4 的形状，写者不失败时同一装置跑 /round rba_a ... --go 是完整的 `RS MD RE`。
// 三条都先核前提（失败写者恰好失败 1 次；R6c 另核失败落在 /round 那一行上），前提不成立按 Fatalf 报。
// Submit 的返回值断言为非 nil：写失败冒泡是 HEAD 既有行为，这一句在忠实实现上恒真，只是回归钉。
func TestRbaR6WriteFailureEmitsNoEvent(t *testing.T) {
	wantRS := RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 1}

	t.Run("R6a_outcome行写失败", func(t *testing.T) {
		evs, w, err := rbaR6Run(t, nil, func(w *rbaFailWriter, _ context.CancelFunc) {
			w.arm("outcome ", nil)
		}, "/round rba_a --go")
		if n := w.failCount(); n != 1 {
			t.Fatalf("前提不成立：失败写者应当恰好失败 1 次，实际 %d；输出:\n%s", n, w.String())
		}
		if err == nil {
			t.Errorf("写屏幕失败时 Submit 应当返回非 nil 的写错误，实际 nil")
		}
		if got := rbaKinds(evs); got != "RS" {
			t.Fatalf("writeAskRecord 写失败时只应有 RS，实际 %q\n%s", got, rbaDump(evs))
		}
		if rbaRS(evs[0].rs) != rbaRS(wantRS) {
			t.Errorf("RS 应当是 %+v，实际 %+v", wantRS, evs[0].rs)
		}
	})

	t.Run("R6b_结算写失败", func(t *testing.T) {
		evs, w, err := rbaR6Run(t, nil, func(w *rbaFailWriter, _ context.CancelFunc) {
			w.arm("结算", nil)
		}, "/round rba_a --go")
		if n := w.failCount(); n != 1 {
			t.Fatalf("前提不成立：失败写者应当恰好失败 1 次，实际 %d；输出:\n%s", n, w.String())
		}
		if err == nil {
			t.Errorf("写屏幕失败时 Submit 应当返回非 nil 的写错误，实际 nil")
		}
		if got := rbaKinds(evs); got != "RS MD" {
			t.Fatalf("结算写失败时应当恰好是 RS MD，实际 %q\n%s", got, rbaDump(evs))
		}
		if rbaRS(evs[0].rs) != rbaRS(wantRS) {
			t.Errorf("RS 应当是 %+v，实际 %+v", wantRS, evs[0].rs)
		}
		if evs[1].md.AgentID != "rba_a" {
			t.Errorf("MD 的 AgentID 应当是 rba_a，实际 %q", evs[1].md.AgentID)
		}
	})

	t.Run("R6c_round行写失败", func(t *testing.T) {
		prep := func(g *rbaRig) {
			if _, err := g.rn.Ask(g.ctx, g.convID, "rba_a"); err != nil {
				t.Fatalf("前提不成立：/use 之前先问 rba_a 一次失败: %v", err)
			}
			if _, err := g.svc.Post(g.ctx, conversation.Message{
				ConvID: g.convID, SenderID: conversation.ReservedSenderUser,
				Kind: conversation.KindUser, Text: "seed-msg-2",
			}); err != nil {
				t.Fatalf("前提不成立：发 seed-msg-2 失败: %v", err)
			}
		}
		evs, w, err := rbaR6Run(t, prep, func(w *rbaFailWriter, cancel context.CancelFunc) {
			w.arm("/round rba_a: ", func(s string) {
				if strings.Contains(s, "本轮问") {
					cancel()
				}
			})
		}, "/round rba_a --go")
		out := w.String()
		if n := w.failCount(); n != 1 {
			t.Fatalf("前提不成立：失败写者应当恰好失败 1 次，实际 %d；输出:\n%s", n, out)
		}
		lines := rbaLines(out)
		hasAsk, outcomes := false, 0
		for _, ln := range lines {
			if ln == "本轮问 1 个: rba_a" {
				hasAsk = true
			}
			if strings.HasPrefix(ln, "outcome ") {
				outcomes++
			}
		}
		if !hasAsk || outcomes != 1 {
			t.Fatalf("前提不成立：缓冲里应当有整行「本轮问 1 个: rba_a」且恰好一行以 outcome 开头（失败落在 /round 那一行），"+
				"实际 本轮问=%v outcome 行=%d；输出:\n%s", hasAsk, outcomes, out)
		}
		if err == nil {
			t.Errorf("写屏幕失败时 Submit 应当返回非 nil 的写错误，实际 nil")
		}
		if got := rbaKinds(evs); got != "RS" {
			t.Fatalf("/round 那一行写失败时只应有 RS，实际 %q\n%s\n输出:\n%s", got, rbaDump(evs), out)
		}
		if rbaRS(evs[0].rs) != rbaRS(wantRS) {
			t.Errorf("RS 应当是 %+v，实际 %+v", wantRS, evs[0].rs)
		}
	})
}

// rbaRSKey / rbaREKey 是 RB-A 冻结的那几个字段。RB-B 给两种事件追加了切片字段，
// 整值不再可比较（编译错误）；这里只比 RB-A 冻结的字段，判别力与改动前相同。
// 追加字段由 RB-B 的 rbb 用例另行断言。
type rbaRSKey struct {
	Label         string
	K, N, Members int
}

type rbaREKey struct {
	Label string
	K, N  int
}

func rbaRS(ev RoundStartEvent) rbaRSKey { return rbaRSKey{ev.Label, ev.K, ev.N, ev.Members} }
func rbaRE(ev RoundEndEvent) rbaREKey   { return rbaREKey{ev.Label, ev.K, ev.N} }
