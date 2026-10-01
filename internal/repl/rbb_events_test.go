package repl

// 本文件覆盖冻结契约 RB-B v1.1 §2.1 的 RB1-RB4：RoundStartEvent.Speakers 与 RoundEndEvent.Results
// 的内容、顺序、新分配，以及名单取 skipNoDelta 之后的那一份。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD a728cee 的源码。
// 装置一律 RB-A §2.0 的 repl 包装置（rbaNewRig、newREPL 之后清空缓冲、直接 r.Submit），只读复用。
//
// 本文件引用本刀新增的 Speakers / Results 两个字段，在 HEAD 上整包编译不过，这是契约 §2 写明的预期红。

import (
	"reflect"
	"regexp"
	"sync"
	"testing"

	"council/internal/conversation"
	"council/internal/turn"
)

// rbbSegIdx 按 RS 切段：返回每段 RS 与 RE 在 evs 里的下标。种类序列不是「RS (MD)* RE」的重复时返回 false。
func rbbSegIdx(evs []rbaEv) (rs, re []int, ok bool) {
	open := -1
	for i, e := range evs {
		switch e.kind {
		case "RS":
			if open >= 0 {
				return nil, nil, false
			}
			open = i
		case "MD":
			if open < 0 {
				return nil, nil, false
			}
		case "RE":
			if open < 0 {
				return nil, nil, false
			}
			rs = append(rs, open)
			re = append(re, i)
			open = -1
		}
	}
	return rs, re, open < 0
}

// rbbResultIDs 取 Results 的 AgentID 序列。
func rbbResultIDs(rs []MemberDoneEvent) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.AgentID
	}
	return out
}

// rbbCheckResultsMatchMD 断言某段 RE 的 Results[i] 与同段里 AgentID 相同的那个 MD 事件 DeepEqual，
// 且每段每人恰好一个 MD。evs[from+1:to] 是这一段的 MD。
func rbbCheckResultsMatchMD(t *testing.T, seg int, evs []rbaEv, from, to int) {
	t.Helper()
	mds := map[string]MemberDoneEvent{}
	for k := from + 1; k < to; k++ {
		md := evs[k].md
		if _, dup := mds[md.AgentID]; dup {
			t.Errorf("第 %d 段：AgentID %q 的 MD 不止一个", seg+1, md.AgentID)
		}
		mds[md.AgentID] = md
	}
	res := evs[to].re.Results
	if len(res) != len(mds) {
		t.Errorf("第 %d 段：Results 有 %d 项，同段 MD 有 %d 个", seg+1, len(res), len(mds))
	}
	for i, r := range res {
		md, ok := mds[r.AgentID]
		if !ok {
			t.Errorf("第 %d 段 Results[%d]（%q）在同段 MD 里找不到", seg+1, i, r.AgentID)
			continue
		}
		if !reflect.DeepEqual(r, md) {
			t.Errorf("第 %d 段 Results[%d] 应当与同段 %s 的 MD DeepEqual：\n  Results %+v\n  MD      %+v", seg+1, i, r.AgentID, r, md)
		}
	}
}

// ═══ RB1 ══════════════════════════════════════════════════════════════

// TestRbbRB1SpeakersAndResults：--rounds 2 --with-judge 三段的 Speakers、Results 与 MD 的一一对应。
//
// 【判别力】能为假，逐条：RS 不带 Speakers（#2）→ 第 1 条 DeepEqual 与 len==Members 红；
// RE 不带 Results（#4）→ 第 2 条红；Results 的 WallMS / RoundID / Label 写成零值（#6、#7、#8）→ 第 3 条 DeepEqual 红。
// 第 3 条能为假的前提是这四个字段在本命令的 MD 里都非零：先用前提守卫钉住（WallMS > 0、RoundID 非空、
// Label 非空、Outcome == Spoke），不成立按前提不成立 Fatalf，免得一个零值 MD 让 DeepEqual 对零值结果恒真。
// Outcome 写死 Spoke（#9）与三个布尔写死 false（#10-#12）在本命令里抓不到（本来就是 Spoke / false），
// 由 tui 包的 H2、H3 担保。对照：RB-A R3 同一命令只看旧字段。
func TestRbbRB1SpeakersAndResults(t *testing.T) {
	g := rbaNewRig(t)
	buf := &rbaBuf{}
	r := g.newREPL(t, buf, buf.Reset)
	rec := &rbaRec{}
	r.SetRoundObserver(rec)
	const cmd = "/round rba_a rba_b rba_j --go --rounds 2 --with-judge"
	if err := r.Submit(g.ctx, cmd); err != nil {
		t.Fatalf("Submit(%q) 返回错误: %v；输出:\n%s", cmd, err, buf.String())
	}
	evs := rec.events()
	const wantKinds = "RS MD MD RE RS MD MD RE RS MD RE"
	if got := rbaKinds(evs); got != wantKinds {
		t.Fatalf("前提不成立：事件种类序列应当恰好是 %q，实际 %q\n%s", wantKinds, got, rbaDump(evs))
	}
	for _, e := range evs {
		if e.kind != "MD" {
			continue
		}
		if e.md.WallMS <= 0 || e.md.RoundID == "" || e.md.Label == "" || e.md.Outcome != turn.OutcomeSpoke {
			t.Fatalf("前提不成立：本命令里每个 MD 的 WallMS、RoundID、Label 都应非零且 Outcome 为 Spoke，实际 %+v", e.md)
		}
	}
	rsAt, reAt, ok := rbbSegIdx(evs)
	if !ok || len(rsAt) != 3 {
		t.Fatalf("前提不成立：切不出 3 段\n%s", rbaDump(evs))
	}
	wantSp := [][]string{{"rba_a", "rba_b"}, {"rba_a", "rba_b"}, {"rba_j"}}
	for s := 0; s < 3; s++ {
		rs := evs[rsAt[s]].rs
		// 第 1 条。
		if !reflect.DeepEqual(rs.Speakers, wantSp[s]) {
			t.Errorf("第 %d 段 RS.Speakers 应当 DeepEqual %q，实际 %#v", s+1, wantSp[s], rs.Speakers)
		}
		if len(rs.Speakers) != rs.Members {
			t.Errorf("第 %d 段 len(Speakers) = %d 应当等于 Members = %d", s+1, len(rs.Speakers), rs.Members)
		}
		// 第 2 条。
		re := evs[reAt[s]].re
		if got := rbbResultIDs(re.Results); !reflect.DeepEqual(got, rs.Speakers) {
			t.Errorf("第 %d 段 RE.Results 的 AgentID 序列应当等于同段 Speakers %q，实际 %q", s+1, rs.Speakers, got)
		}
		if len(re.Results) != rs.Members {
			t.Errorf("第 %d 段 len(Results) = %d 应当等于 Members = %d", s+1, len(re.Results), rs.Members)
		}
		// 第 3 条。
		rbbCheckResultsMatchMD(t, s, evs, rsAt[s], reAt[s])
	}
}

// ═══ RB2 ══════════════════════════════════════════════════════════════

// TestRbbRB2ResultsFollowSpeakersNotArrival：rba_a 带 --resume（假 CLI 睡 700ms）、rba_b 首轮立即回来，
// 回来顺序与名单相反；Results 仍按名单顺序。
//
// 【判别力】能为假：在 onDone 里按回来顺序累积 Results（#5）→ Results 是 rba_b, rba_a，主断言红。
// 对照：前提那一句（回来顺序确实与名单相反），不成立按前提不成立 Fatalf——否则按回来顺序与按名单顺序
// 碰巧相同，主断言恒真。
func TestRbbRB2ResultsFollowSpeakersNotArrival(t *testing.T) {
	g := rbaNewRig(t)
	if _, err := g.rn.Ask(g.ctx, g.convID, "rba_a"); err != nil {
		t.Fatalf("前提不成立：/use 之前先问 rba_a 一次失败: %v", err)
	}
	if _, err := g.svc.Post(g.ctx, conversation.Message{
		ConvID: g.convID, SenderID: conversation.ReservedSenderUser,
		Kind: conversation.KindUser, Text: "seed-msg-2",
	}); err != nil {
		t.Fatalf("前提不成立：发 seed-msg-2 失败: %v", err)
	}
	buf := &rbaBuf{}
	r := g.newREPL(t, buf, buf.Reset)
	rec := &rbaRec{}
	r.SetRoundObserver(rec)
	const cmd = "/round rba_a rba_b --go"
	if err := r.Submit(g.ctx, cmd); err != nil {
		t.Fatalf("Submit(%q) 返回错误: %v；输出:\n%s", cmd, err, buf.String())
	}
	evs := rec.events()
	if got := rbaKinds(evs); got != "RS MD MD RE" {
		t.Fatalf("前提不成立：事件种类序列应当恰好是 %q，实际 %q\n%s", "RS MD MD RE", got, rbaDump(evs))
	}
	md1, md2 := evs[1].md, evs[2].md
	if md1.AgentID != "rba_b" || md2.AgentID != "rba_a" {
		t.Fatalf("前提不成立：回来顺序应当是 rba_b、rba_a（与名单相反），实际 %s、%s（wall %d、%d）",
			md1.AgentID, md2.AgentID, md1.WallMS, md2.WallMS)
	}
	res := evs[3].re.Results
	if got := rbbResultIDs(res); !reflect.DeepEqual(got, []string{"rba_a", "rba_b"}) {
		t.Fatalf("RE.Results 的 AgentID 应当依次是 rba_a、rba_b（名单顺序），实际 %q", got)
	}
	if !reflect.DeepEqual(res[0], md2) {
		t.Errorf("Results[0] 应当 DeepEqual 第 2 个 MD（rba_a）：\n  Results[0] %+v\n  MD        %+v", res[0], md2)
	}
	if !reflect.DeepEqual(res[1], md1) {
		t.Errorf("Results[1] 应当 DeepEqual 第 1 个 MD（rba_b）：\n  Results[1] %+v\n  MD        %+v", res[1], md1)
	}
}

// ═══ RB3 ══════════════════════════════════════════════════════════════

// rbbScribbler 是会改写名单的观察者：RoundStart 里先记下 Speakers 的拷贝，然后把 ev.Speakers[0] 改成 rbb_zz。
// MemberDone 在 worker goroutine 上调，所以加锁。
type rbbScribbler struct {
	mu     sync.Mutex
	copies [][]string
	mds    []MemberDoneEvent
	res    [][]MemberDoneEvent
}

var _ RoundObserver = (*rbbScribbler)(nil)

func (c *rbbScribbler) RoundStart(ev RoundStartEvent) {
	cp := append([]string(nil), ev.Speakers...)
	c.mu.Lock()
	c.copies = append(c.copies, cp)
	c.mu.Unlock()
	// 不带名单时没有第 0 个可改（变异 #2），拷贝已经记下，交给断言报红，不在这里 panic。
	if len(ev.Speakers) > 0 {
		ev.Speakers[0] = "rbb_zz"
	}
}

func (c *rbbScribbler) MemberDone(ev MemberDoneEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mds = append(c.mds, ev)
}

func (c *rbbScribbler) RoundEnd(ev RoundEndEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.res = append(c.res, ev.Results)
}

// TestRbbRB3SpeakersAreFreshCopy：观察者改写 ev.Speakers 不影响真正要问的人。
//
// 【判别力】能为假：`Speakers: plan.Speakers`（不拷贝，#1）→ 第 1 段问到 rbb_zz（判 refused），
// MD 的 AgentID / Outcome 与第 2 段 RS 的拷贝都红（原型实测第 2 段只剩 1 人）。
// 对照：同一形状的命令不改写时（RB1 前两段）名单都是 rba_a, rba_b。
func TestRbbRB3SpeakersAreFreshCopy(t *testing.T) {
	g := rbaNewRig(t)
	buf := &rbaBuf{}
	r := g.newREPL(t, buf, buf.Reset)
	rec := &rbbScribbler{}
	r.SetRoundObserver(rec)
	const cmd = "/round rba_a rba_b --go --rounds 2"
	if err := r.Submit(g.ctx, cmd); err != nil {
		t.Fatalf("Submit(%q) 返回错误: %v；输出:\n%s", cmd, err, buf.String())
	}
	rec.mu.Lock()
	copies, mds, res := rec.copies, rec.mds, rec.res
	rec.mu.Unlock()

	want := []string{"rba_a", "rba_b"}
	if len(copies) != 2 {
		t.Errorf("RS 应当恰好 2 个，实际 %d：%q", len(copies), copies)
	}
	for i, cp := range copies {
		if !reflect.DeepEqual(cp, want) {
			t.Errorf("第 %d 个 RS 进门时记下的名单拷贝应当是 %q，实际 %q", i+1, want, cp)
		}
	}
	if len(mds) != 4 {
		t.Errorf("MD 应当恰好 4 个，实际 %d", len(mds))
	}
	for i, md := range mds {
		if md.AgentID != "rba_a" && md.AgentID != "rba_b" {
			t.Errorf("第 %d 个 MD 的 AgentID 应当在 {rba_a, rba_b} 里，实际 %q", i+1, md.AgentID)
		}
		if md.Outcome != turn.OutcomeSpoke {
			t.Errorf("第 %d 个 MD（%s）的 Outcome 应当是 Spoke，实际 %v", i+1, md.AgentID, md.Outcome)
		}
	}
	if len(res) != 2 {
		t.Errorf("RE 应当恰好 2 个，实际 %d", len(res))
	}
	for i, rs := range res {
		if got := rbbResultIDs(rs); !reflect.DeepEqual(got, want) {
			t.Errorf("第 %d 个 RE 的 Results AgentID 应当是 %q，实际 %q", i+1, want, got)
		}
	}
}

// ═══ RB4 ══════════════════════════════════════════════════════════════

// TestRbbRB4SpeakersAfterSkipNoDelta：rba_b 被判无新内容而跳过，名单与结果都只有 rba_a。
//
// 【判别力】能为假：从预算行（rows）或 skipNoDelta 之前的 plan 取名单（#3）→ Speakers 多出 rba_b，红。
// 对照：RB1 没人被跳过时名单等于命令里的人。前提：预算表里确有 rba_b 被判无新内容那一行（正则守卫）。
func TestRbbRB4SpeakersAfterSkipNoDelta(t *testing.T) {
	g := rbaNewRig(t)
	if _, err := g.rn.Ask(g.ctx, g.convID, "rba_b"); err != nil {
		t.Fatalf("前提不成立：/use 之前先问 rba_b 一次失败: %v", err)
	}
	buf := &rbaBuf{}
	r := g.newREPL(t, buf, buf.Reset)
	rec := &rbaRec{}
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
		t.Fatalf("前提不成立：事件种类序列应当恰好是 %q，实际 %q\n%s", "RS MD RE", got, rbaDump(evs))
	}
	if got := evs[0].rs.Speakers; !reflect.DeepEqual(got, []string{"rba_a"}) {
		t.Errorf("RS.Speakers 应当 DeepEqual [rba_a]（skipNoDelta 之后），实际 %#v", got)
	}
	if got := rbbResultIDs(evs[2].re.Results); !reflect.DeepEqual(got, []string{"rba_a"}) {
		t.Errorf("RE.Results 应当只有 rba_a 一项，实际 %q", got)
	}
}
