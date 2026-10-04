package tui

// 本文件覆盖冻结契约 RB-A v1.2 §2.5 的 O1-O4：真 tea.Program、真 REPL、真 Runner、假 CLI。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3143bc8 的源码。
// 装置（rbaNewTuiRig、rbaStartProg、spy 日志）在 rba_fakecli_test.go。
// 日志里「P」指 printLine 正文；事件按 Update 的记录顺序。
//
// 这些用例的观察者是装置自己用 newRoundObserver(p.Send) 装的，不经过 run.go 的 F 那一行；
// F 那一行由 rba_wiring_test.go 的 W1 担保。

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
	"council/internal/turn"
)

// rbaColdLineRe 是 R3 第 4 条同一条冷暖正则，捕获 wall_ms。
var rbaColdLineRe = regexp.MustCompile(`^冷暖 预报=(冷|暖) 实测=(冷|暖|未知)  wall_ms=(\d+)  prompt_bytes=\d+  scanned_to=\d+$`)

// rbaEvents 取日志里非 P、非 TICK 的条目，并记下每条在原日志里的下标。
func rbaEvents(log []rbaEntry) ([]rbaEntry, []int) {
	var evs []rbaEntry
	var idx []int
	for i, e := range log {
		if e.kind == "P" || e.kind == "TICK" {
			continue
		}
		evs = append(evs, e)
		idx = append(idx, i)
	}
	return evs, idx
}

func rbaEntryKinds(evs []rbaEntry) string {
	ks := make([]string, len(evs))
	for i, e := range evs {
		ks[i] = e.kind
	}
	return strings.Join(ks, " ")
}

// rbaPBlock 取 log[from+1 : to) 里的 P 正文（两个事件之间的 printLine 块）。
func rbaPBlock(log []rbaEntry, from, to int) []string {
	var out []string
	for i := from + 1; i < to; i++ {
		if log[i].kind == "P" {
			out = append(out, log[i].text)
		}
	}
	return out
}

// rbaLastP 取 log[:to) 里最近一条 P 的正文。
func rbaLastP(log []rbaEntry, to int) (string, bool) {
	for i := to - 1; i >= 0; i-- {
		if log[i].kind == "P" {
			return log[i].text, true
		}
	}
	return "", false
}

func rbaLast6(s string) string {
	if len(s) > 6 {
		return s[len(s)-6:]
	}
	return s
}

// ═══ O1 ═══════════════════════════════════════════════════════════════

// TestRbaO1UpdateOrderRounds2WithJudge：--rounds 2 --with-judge 在真 Program 上的 Update 顺序、载荷与表头。
//
// 【判别力】这是「跨并发原语的顺序」那一条：MD 在 progressMu 里经真 p.Send 投递，明细行经 LineWriter
// 经同一个 p.Send 投递，两者进 Update 的先后只有在真 Program 上才是真的。能为假，逐条：
//   - MD 挪到 writeAskRecord 之前（#6）→ 第 3 条块为空或属于另一个成员，红；
//   - RS 挪到「本轮问」之前（#1）→ 第 2 条 RS 之前最近的 P 是「扇出期间……」，红；
//   - k/N 对调（#4）或裁判传 1,1（#5）→ 第 1 条载荷红；WallMS 写 0（#12）→ 第 3 条红；
//   - RoundID 写空（#13）、Label 写空（#14）→ 第 4 条红；RE 挪到结算之前（#15）→ 第 5 条红；
//   - 适配器 RoundEnd 发 roundStartMsg（#22）→ 第 1 条种类序列红；MemberDone 丢 ev（#23）→ 第 4 条红；
//   - 末 6 位写成前 6 位（#38）→ 第 6 条红。
//
// 对照：O2 同一命令连发两次 Ctrl-C。
func TestRbaO1UpdateOrderRounds2WithJudge(t *testing.T) {
	g := rbaNewTuiRig(t)
	pr := rbaStartProg(t, g)
	pr.useConv(t, g.convID)

	const cmd = "/round rba_a rba_b rba_j --go --rounds 2 --with-judge"
	pr.submit(cmd)
	pr.waitDone(t, 2, 30*time.Second)

	all := pr.spy.entries()
	log, ok := rbaAfterEcho(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有第 2 次 Enter 的回显 P；日志:\n%s", rbaLogDump(all))
	}
	evs, idx := rbaEvents(log)

	// 第 1 条。
	const wantKinds = "RS MD MD RE RS MD MD RE RS MD RE DONE"
	if got := rbaEntryKinds(evs); got != wantKinds {
		t.Fatalf("非 P、非 tick 条目序列应当恰好是 %q，实际 %q；日志:\n%s", wantKinds, got, rbaLogDump(log))
	}
	wantRS := []repl.RoundStartEvent{
		{Label: "发言轮", K: 1, N: 2, Members: 2},
		{Label: "发言轮", K: 2, N: 2, Members: 2},
		{Label: "裁判轮", K: 0, N: 0, Members: 1},
	}
	wantRE := []repl.RoundEndEvent{
		{Label: "发言轮", K: 1, N: 2},
		{Label: "发言轮", K: 2, N: 2},
		{Label: "裁判轮", K: 0, N: 0},
	}
	rsAt := []int{0, 4, 8}
	reAt := []int{3, 7, 10}
	for s := 0; s < 3; s++ {
		if got := evs[rsAt[s]].rs; rbaRS(got) != rbaRS(wantRS[s]) {
			t.Errorf("第 %d 段 RS 应当是 %+v，实际 %+v", s+1, wantRS[s], got)
		}
		if got := evs[reAt[s]].re; rbaRE(got) != rbaRE(wantRE[s]) {
			t.Errorf("第 %d 段 RE 应当是 %+v，实际 %+v", s+1, wantRE[s], got)
		}
	}

	// 第 2 条。
	wantAsk := []string{"本轮问 2 个: rba_a, rba_b", "本轮问 2 个: rba_a, rba_b", "本轮问 1 个: rba_j"}
	for s := 0; s < 3; s++ {
		got, ok := rbaLastP(log, idx[rsAt[s]])
		if !ok || got != wantAsk[s] {
			t.Errorf("第 %d 段 RS 之前最近一条 P 应当是 %q，实际 %q", s+1, wantAsk[s], got)
		}
	}

	// 第 3、4、6 条（MD）。
	segName := []string{"发言轮 1/2", "发言轮 2/2", "裁判轮"}
	segMembers := []int{2, 2, 1}
	wantAgents := []map[string]bool{
		{"rba_a": true, "rba_b": true},
		{"rba_a": true, "rba_b": true},
		{"rba_j": true},
	}
	gotAgents := []map[string]bool{{}, {}, {}}
	segRID := []string{"", "", ""}
	for s := 0; s < 3; s++ {
		j := 0
		for k := rsAt[s] + 1; k < reAt[s]; k++ {
			j++
			e := evs[k]
			md := e.md
			block := rbaPBlock(log, idx[k-1], idx[k])
			wantFirst := "outcome spoke  agent " + md.AgentID
			if len(block) == 0 || block[0] != wantFirst {
				t.Errorf("第 %d 段第 %d 个 MD(%s)：与上一个事件之间的 P 块第一条应当是 %q；块: %q", s+1, j, md.AgentID, wantFirst, block)
			}
			outcomes, colds := 0, 0
			for _, ln := range block {
				if strings.HasPrefix(ln, "outcome ") {
					outcomes++
				}
				if m := rbaColdLineRe.FindStringSubmatch(ln); m != nil {
					colds++
					if w, _ := strconv.Atoi(m[3]); w != md.WallMS {
						t.Errorf("第 %d 段 MD(%s)：冷暖行 wall_ms=%d 与 ev.WallMS=%d 不等", s+1, md.AgentID, w, md.WallMS)
					}
				}
			}
			if outcomes != 1 || colds != 1 {
				t.Errorf("第 %d 段 MD(%s)：P 块里 outcome 行应当恰好 1 条（实际 %d）、冷暖行恰好 1 条（实际 %d）；块: %q", s+1, md.AgentID, outcomes, colds, block)
			}
			// 第 4 条。
			if md.Label != wantRS[s].Label || md.Outcome != turn.OutcomeSpoke || md.MaybeAdvanced || md.Drifted || md.Dropped {
				t.Errorf("第 %d 段 MD：应当 Label=%s、Outcome=Spoke、三个布尔 false，实际 %+v", s+1, wantRS[s].Label, md)
			}
			if md.RoundID == "" {
				t.Errorf("第 %d 段 MD(%s)：RoundID 应当非空", s+1, md.AgentID)
			} else if segRID[s] == "" {
				segRID[s] = md.RoundID
			} else if segRID[s] != md.RoundID {
				t.Errorf("第 %d 段两个 MD 的 RoundID 不等：%q 与 %q", s+1, segRID[s], md.RoundID)
			}
			gotAgents[s][md.AgentID] = true
			// 第 6 条（MD 处的表头）。
			re := regexp.MustCompile(`^council t   正在运行  ` + regexp.QuoteMeta(segName[s]) +
				` \(` + regexp.QuoteMeta(rbaLast6(md.RoundID)) + `\)  回来 ` + strconv.Itoa(j) + `/` +
				strconv.Itoa(segMembers[s]) + `  \d+s$`)
			if !re.MatchString(e.header) {
				t.Errorf("第 %d 段第 %d 个 MD 处的表头应当匹配 %s，实际 %q", s+1, j, re, e.header)
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

	// 第 5 条。
	for s := 0; s < 3; s++ {
		block := rbaPBlock(log, idx[reAt[s]-1], idx[reAt[s]])
		want := wantRE[s].Label + "结算"
		if len(block) == 0 || block[0] != want {
			t.Errorf("第 %d 段 RE 与上一个 MD 之间的 P 块第一条应当是 %q；块: %q", s+1, want, block)
		}
	}

	// 第 6 条（RS 与 DONE 处的表头）。
	rsHeader := []*regexp.Regexp{
		regexp.MustCompile(`^council t   正在运行  发言轮 1/2  回来 0/2  \d+s$`),
		regexp.MustCompile(`^council t   正在运行  发言轮 2/2  回来 0/2  \d+s$`),
		regexp.MustCompile(`^council t   正在运行  裁判轮  回来 0/1  \d+s$`),
	}
	for s := 0; s < 3; s++ {
		if h := evs[rsAt[s]].header; !rsHeader[s].MatchString(h) {
			t.Errorf("第 %d 段 RS 处的表头应当匹配 %s，实际 %q", s+1, rsHeader[s], h)
		}
	}
	if h := evs[len(evs)-1].header; h != "council t   空闲" {
		t.Errorf("DONE 处的表头应当整串等于 %q，实际 %q", "council t   空闲", h)
	}
}

// ═══ O2 ═══════════════════════════════════════════════════════════════

// TestRbaO2CancelPath：第 1 段在飞时连按两次 Ctrl-C，事件与屏幕按取消路径走。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里连发两次。
//
// 【判别力】能为假，逐条：MD 挪到 askCause 那一行之前（#7）→ 第 3 条块的最后一条不是 "/round X: "，红；
// MD 只在 rec.Err == nil 时发（#8）→ 第 1 条序列少两个 MD，红；MaybeAdvanced 写死 false（#9）→ 第 2 条红；
// tick 或取消把「正在取消」盖掉（#29 一类）→ 第 5 条红。
// 对照：O1 同一命令不取消，走完三段。
func TestRbaO2CancelPath(t *testing.T) {
	t.Setenv("RBA_FAKE_SLEEP_MS", "3000")
	g := rbaNewTuiRig(t)
	pr := rbaStartProg(t, g)
	pr.useConv(t, g.convID)

	const cmd = "/round rba_a rba_b rba_j --go --rounds 2 --with-judge"
	pr.submit(cmd)
	if !pr.waitFor(10*time.Second, func(log []rbaEntry) bool { return rbaCount(log, "RS") >= 1 }) {
		t.Fatalf("前提不成立：10 秒内日志里没有出现第 1 个 RS；日志:\n%s", rbaLogDump(pr.spy.entries()))
	}
	time.Sleep(300 * time.Millisecond)
	pr.p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	pr.p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	pr.waitDone(t, 2, 15*time.Second)

	all := pr.spy.entries()
	log, ok := rbaAfterEcho(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有第 2 次 Enter 的回显 P；日志:\n%s", rbaLogDump(all))
	}
	evs, idx := rbaEvents(log)

	// 第 1 条。
	if got := rbaEntryKinds(evs); got != "RS MD MD RE DONE" {
		t.Fatalf("非 P、非 tick 条目序列应当恰好是 %q，实际 %q；日志:\n%s", "RS MD MD RE DONE", got, rbaLogDump(log))
	}
	if want := (repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 2}); rbaRS(evs[0].rs) != rbaRS(want) {
		t.Errorf("RS 应当是 %+v，实际 %+v", want, evs[0].rs)
	}
	if want := (repl.RoundEndEvent{Label: "发言轮", K: 1, N: 2}); rbaRE(evs[3].re) != rbaRE(want) {
		t.Errorf("RE 应当是 %+v，实际 %+v", want, evs[3].re)
	}

	mdHeader := regexp.MustCompile(`^council t   正在取消  发言轮 1/2 \([0-9a-f]{6}\)  回来 [12]/2  \d+s$`)
	for k := 1; k <= 2; k++ {
		md := evs[k].md
		// 第 2 条。
		if md.Outcome != turn.OutcomeCanceled || !md.MaybeAdvanced || md.Drifted || md.Dropped {
			t.Errorf("第 %d 个 MD 应当 Outcome=Canceled、MaybeAdvanced=true、Drifted/Dropped=false，实际 %+v", k, md)
		}
		// 第 3 条。
		block := rbaPBlock(log, idx[k-1], idx[k])
		if len(block) < 3 {
			t.Errorf("第 %d 个 MD(%s) 的 P 块至少应有 3 条（outcome、可能已吃下、askCause），实际 %q", k, md.AgentID, block)
		} else {
			if want := "outcome canceled  agent " + md.AgentID; block[0] != want {
				t.Errorf("第 %d 个 MD 的 P 块第一条应当是 %q，实际 %q", k, want, block[0])
			}
			if want := "provider 那边可能已经吃下这批内容"; block[1] != want {
				t.Errorf("第 %d 个 MD 的 P 块第二条应当是 %q，实际 %q", k, want, block[1])
			}
			if last := block[len(block)-1]; !strings.HasPrefix(last, "/round "+md.AgentID+": ") {
				t.Errorf("第 %d 个 MD 的 P 块最后一条应当以 %q 开头（askCause 那一行必须在 MD 之前），实际 %q", k, "/round "+md.AgentID+": ", last)
			}
		}
		// 第 5 条。
		if !mdHeader.MatchString(evs[k].header) {
			t.Errorf("第 %d 个 MD 处的表头应当匹配 %s，实际 %q", k, mdHeader, evs[k].header)
		}
	}

	// 第 4 条。
	tail := rbaPBlock(log, idx[3], idx[4])
	if len(tail) != 1 || tail[0] != "已跑 1/2 轮，其余取消" {
		t.Errorf("RE 与 DONE 之间应当恰好一条 P「已跑 1/2 轮，其余取消」，实际 %q", tail)
	}
	if h := evs[4].header; h != "council t   空闲" {
		t.Errorf("DONE 处的表头应当整串等于 %q，实际 %q", "council t   空闲", h)
	}
}

// ═══ O3 ═══════════════════════════════════════════════════════════════

// TestRbaO3aDriftedRealPath：第 2 轮假 CLI 换了会话 id，第 2 段两个 MD 的 Drifted 为 true。
//
// 【判别力】Drifted 写死 false（#10）→ 第 2 段红。对照：第 1 段同一成员 Drifted 为 false。
func TestRbaO3aDriftedRealPath(t *testing.T) {
	t.Setenv("RBA_FAKE_RESUME_SESSION", "11111111-2222-4333-8444-555555555555")
	g := rbaNewTuiRig(t)
	pr := rbaStartProg(t, g)
	pr.useConv(t, g.convID)

	const cmd = "/round rba_a rba_b --go --rounds 2"
	pr.submit(cmd)
	pr.waitDone(t, 2, 30*time.Second)

	all := pr.spy.entries()
	log, ok := rbaAfterEcho(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有第 2 次 Enter 的回显 P；日志:\n%s", rbaLogDump(all))
	}
	evs, idx := rbaEvents(log)
	if got := rbaEntryKinds(evs); got != "RS MD MD RE RS MD MD RE DONE" {
		t.Fatalf("前提不成立：序列应当是 %q，实际 %q；日志:\n%s", "RS MD MD RE RS MD MD RE DONE", got, rbaLogDump(log))
	}
	const driftLine = "送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了"
	for _, k := range []int{1, 2, 5, 6} {
		md := evs[k].md
		wantDrift := k >= 5
		if md.Outcome != turn.OutcomeSpoke {
			t.Errorf("MD(%d, %s) Outcome 应当是 Spoke，实际 %v", k, md.AgentID, md.Outcome)
		}
		if md.Drifted != wantDrift {
			t.Errorf("MD(%d, %s) Drifted 应当是 %v，实际 %v", k, md.AgentID, wantDrift, md.Drifted)
		}
		if wantDrift {
			n := 0
			for _, ln := range rbaPBlock(log, idx[k-1], idx[k]) {
				if ln == driftLine {
					n++
				}
			}
			if n != 1 {
				t.Errorf("第 2 段 MD(%s) 的 P 块里漂移说明行应当恰好 1 条，实际 %d", md.AgentID, n)
			}
		}
	}
}

// TestRbaO3bDroppedRealPath：rba_b 在飞时离开会话，它的 MD 是 PostRejected、Dropped 为 true。
//
// 【判别力】Dropped 写死 false（#11）→ rba_b 的 MD 红；MD 只在 rec.Err == nil 时发（#8）→ rba_b 的 MD 缺失，红。
// 对照：同轮 rba_a 是 Spoke、Dropped 为 false。
func TestRbaO3bDroppedRealPath(t *testing.T) {
	t.Setenv("RBA_FAKE_SLEEP_MS", "1500")
	g := rbaNewTuiRig(t)
	pr := rbaStartProg(t, g)
	pr.useConv(t, g.convID)

	const cmd = "/round rba_a rba_b --go"
	pr.submit(cmd)
	if !pr.waitFor(10*time.Second, func(log []rbaEntry) bool { return rbaCount(log, "RS") >= 1 }) {
		t.Fatalf("前提不成立：10 秒内日志里没有出现 RS；日志:\n%s", rbaLogDump(pr.spy.entries()))
	}
	if err := g.svc.Leave(context.Background(), g.convID, "rba_b", time.Now().UTC()); err != nil {
		t.Fatalf("前提不成立：rba_b 在飞时 Leave 失败: %v", err)
	}
	pr.waitDone(t, 2, 15*time.Second)

	all := pr.spy.entries()
	log, ok := rbaAfterEcho(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有第 2 次 Enter 的回显 P；日志:\n%s", rbaLogDump(all))
	}
	evs, idx := rbaEvents(log)
	if got := rbaEntryKinds(evs); got != "RS MD MD RE DONE" {
		t.Fatalf("序列应当是 %q，实际 %q；日志:\n%s", "RS MD MD RE DONE", got, rbaLogDump(log))
	}
	seen := map[string]bool{}
	for _, k := range []int{1, 2} {
		md := evs[k].md
		seen[md.AgentID] = true
		switch md.AgentID {
		case "rba_b":
			if md.Outcome != turn.OutcomePostRejected || !md.Dropped {
				t.Errorf("rba_b 的 MD 应当 Outcome=PostRejected、Dropped=true，实际 %+v", md)
			}
			n := 0
			for _, ln := range rbaPBlock(log, idx[k-1], idx[k]) {
				if ln == "正文写不进库（已经付过钱），整段如下:" {
					n++
				}
			}
			if n != 1 {
				t.Errorf("rba_b 的 P 块里「正文写不进库」那一行应当恰好 1 条，实际 %d", n)
			}
		case "rba_a":
			if md.Outcome != turn.OutcomeSpoke || md.Dropped {
				t.Errorf("rba_a 的 MD 应当 Outcome=Spoke、Dropped=false，实际 %+v", md)
			}
		default:
			t.Errorf("意外的 MD AgentID %q", md.AgentID)
		}
	}
	if !seen["rba_a"] || !seen["rba_b"] {
		t.Errorf("两个 MD 应当分别属于 rba_a 与 rba_b，实际 %v", seen)
	}
}

// ═══ O4 ═══════════════════════════════════════════════════════════════

// TestRbaO4TickOnlyWhileBusy：真 Program 上 tick 在忙时每秒走、续订第 2 拍到 2s；不忙之后至多一拍在途。
//
// 【判别力】从不装 tick（#32）→ 第 1 条没有任何 tick，红；续订代次错（#49）或间隔错（#50）→
// 没有表头为 2s 的那一拍，红（T3 是主杀手，这里是真 Program 上的辅助）；
// 不看 busy 续订（#27）→ 2.5 秒里至少 2 拍，第 2 条红；`n > 1` 写成 `n >= 1`（#39）→ 表头多出 1/1，第 1 条红。
func TestRbaO4TickOnlyWhileBusy(t *testing.T) {
	t.Setenv("RBA_FAKE_SLEEP_MS", "2800")
	g := rbaNewTuiRig(t)
	pr := rbaStartProg(t, g)
	pr.useConv(t, g.convID)
	time.Sleep(1500 * time.Millisecond)

	const cmd = "/round rba_a --go"
	pr.submit(cmd)
	pr.waitDone(t, 2, 15*time.Second)
	time.Sleep(2500 * time.Millisecond)

	all := pr.spy.entries()
	log, ok := rbaAfterEcho(all, cmd)
	if !ok {
		t.Fatalf("前提不成立：日志里没有第 2 次 Enter 的回显 P；日志:\n%s", rbaLogDump(all))
	}
	doneAt := -1
	for i, e := range log {
		if e.kind == "DONE" {
			doneAt = i
			break
		}
	}
	if doneAt < 0 {
		t.Fatalf("前提不成立：回显之后没有 submitDone；日志:\n%s", rbaLogDump(log))
	}

	// 第 1 条。
	anyRe := regexp.MustCompile(`^council t   正在运行  发言轮  回来 0/1  [1-9]s$`)
	sawAny, saw2s := false, false
	for _, e := range log[:doneAt] {
		if e.kind != "TICK" {
			continue
		}
		if anyRe.MatchString(e.header) {
			sawAny = true
		}
		if e.header == "council t   正在运行  发言轮  回来 0/1  2s" {
			saw2s = true
		}
	}
	if !sawAny {
		t.Errorf("submitDone 之前应当至少有一拍的表头匹配 %s；日志:\n%s", anyRe, rbaLogDump(log))
	}
	if !saw2s {
		t.Errorf("submitDone 之前应当有一拍的表头整串等于 %q（续订的第 2 拍）；日志:\n%s", "council t   正在运行  发言轮  回来 0/1  2s", rbaLogDump(log))
	}

	// 第 2 条。
	ticks := 0
	for _, e := range log[doneAt+1:] {
		if e.kind == "TICK" {
			ticks++
		}
		if e.header != "council t   空闲" {
			t.Errorf("submitDone 之后的表头应当都是 %q，实际 %q（%s）", "council t   空闲", e.header, e.kind)
		}
	}
	if ticks > 1 {
		t.Errorf("submitDone 之后的 tick 至多 1 条（在途的那一拍），实际 %d；日志:\n%s", ticks, rbaLogDump(log))
	}
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

func rbaRS(ev repl.RoundStartEvent) rbaRSKey { return rbaRSKey{ev.Label, ev.K, ev.N, ev.Members} }
func rbaRE(ev repl.RoundEndEvent) rbaREKey   { return rbaREKey{ev.Label, ev.K, ev.N} }
