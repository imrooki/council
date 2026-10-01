package repl

// fu1：MD1 / CO1 / SM1 独立审查余项在 repl 包的补测。只加用例，不动生产代码。
// 装置全部复用同包既有测试文件：CO1 的 coxNewRig / coxRoundREPL（cox_round_test.go）、
// MD1 的 mdxNewRig（mdx_addmodel_fixture_test.go）、SM1 的 smxSetup（smx_summary_round_test.go）。
// 期望值一律取自实跑；每条用例杀的变异写在它自己的注释里。

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
)

// ─── CO1 ─────────────────────────────────────────────────────────────

// TestFuxRoundNoDeltaVerboseGate 钉 no_delta 那一行的档位门在详细档那一侧：
// 同 R5 mixed 的装置与命令，详细档不打「跳过 cox_b（no_delta）」那一行（依据列已经写了同一句）。
//
// 【判别力】变异：round.go 里 no_delta 块的 if r.concise { 改成 if true {。
// 详细档随之多出这一行，verbose 子测试红。concise 子测试是同装置、只差档位的正对照
// （这一行恰好一次），证明缺席断言能为假。verbose 子测试另有两条在场守卫：
// 预算表里 cox_b 的依据列、「本轮问 1 个: cox_a」，防止命令提前失败时缺席断言空过。
func TestFuxRoundNoDeltaVerboseGate(t *testing.T) {
	const noDeltaLine = "跳过 cox_b（no_delta）：cox_b 无新内容，本轮跳过"
	run := func(t *testing.T, concise bool) []string {
		t.Helper()
		g := coxNewRig(t, []string{"cox_a", "cox_b", "cox_gone", "cox_j"}, nil)
		g.ask(t, "cox_b")
		r, buf, _ := coxRoundREPL(t, g, concise)
		if err := r.Submit(g.ctx, "/round cox_a cox_b cox_gone cox_j --go"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		return strings.Split(coxNormalize(buf.String()), "\n")
	}
	count := func(lines []string, prefix string) int {
		n := 0
		for _, l := range lines {
			if strings.HasPrefix(l, prefix) {
				n++
			}
		}
		return n
	}
	exact := func(lines []string, want string) int {
		n := 0
		for _, l := range lines {
			if l == want {
				n++
			}
		}
		return n
	}

	t.Run("verbose", func(t *testing.T) {
		lines := run(t, false)
		all := strings.Join(lines, "\n")
		if n := count(lines, "跳过 cox_b（no_delta）"); n != 0 {
			t.Fatalf("详细档不应当有以「跳过 cox_b（no_delta）」开头的行，实际 %d 行；全部输出:\n%s", n, all)
		}
		if n := exact(lines, "cox_b  未标定  未标定  预报非冷  不在飞  余量未知  无新内容，本轮跳过"); n != 1 {
			t.Fatalf("前提不成立：详细档预算表里 cox_b 那一整行（依据列「无新内容，本轮跳过」）应当恰好一次，实际 %d；全部输出:\n%s", n, all)
		}
		if n := exact(lines, "本轮问 1 个: cox_a"); n != 1 {
			t.Fatalf("前提不成立：整行「本轮问 1 个: cox_a」应当恰好一次，实际 %d；全部输出:\n%s", n, all)
		}
	})

	t.Run("concise_control", func(t *testing.T) {
		lines := run(t, true)
		if n := exact(lines, noDeltaLine); n != 1 {
			t.Fatalf("正对照：简洁档整行 %q 应当恰好一次，实际 %d；全部输出:\n%s", noDeltaLine, n, strings.Join(lines, "\n"))
		}
	})
}

// fuxTotalErr 是「总用时」那一行写失败时注入的错误。
var fuxTotalErr = errors.New("fux injected total-line write failure")

// fuxTotalFailWriter 带锁（成员块由 turn 的 worker goroutine 回调写入）；
// 写入内容含「总用时」时返回 fuxTotalErr、不写入，其余照常累积。
type fuxTotalFailWriter struct {
	mu   sync.Mutex
	b    strings.Builder
	hits int
}

func (w *fuxTotalFailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if strings.Contains(string(p), "总用时") {
		w.hits++
		return 0, fuxTotalErr
	}
	return w.b.Write(p)
}

func (w *fuxTotalFailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func (w *fuxTotalFailWriter) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.b.Reset()
	w.hits = 0
}

func (w *fuxTotalFailWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hits
}

// TestFuxRoundTotalLineWriteErrorPropagates 钉 cmdRound 的 defer：「总用时」那一行写失败，错误要从 Submit 传出去。
// 照 R10 的坏写者，但只在写「总用时」时失败；简洁档、起过进程、没装观察者，四个判据都成立，defer 确实去写。
//
// 【判别力】变异：round.go 的 defer 里吞掉 printf 的错误（err = r.printf(...) 改成 _ = r.printf(...)）。
// 这时 Submit 返回 nil，errors.Is 红。前提守卫：写「总用时」恰好被尝试一次、在它之前的整串逐字等于实跑值。
func TestFuxRoundTotalLineWriteErrorPropagates(t *testing.T) {
	g := coxNewRig(t, []string{"cox_a", "cox_b"}, nil)
	g.slowA(t)
	w := &fuxTotalFailWriter{}
	r, _ := g.newREPL(t, w, w.Reset, true)
	err := r.Submit(g.ctx, "/round cox_a cox_b --go")
	if n := w.count(); n != 1 {
		t.Fatalf("前提不成立：含「总用时」的写入应当恰好尝试 1 次，实际 %d 次；err=%v；输出:\n%s", n, err, w.String())
	}
	want := "本轮问 2 个: cox_a, cox_b\n" +
		"cox_b（T）: OK\n" +
		"甲（T）: OK\n" +
		"总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）\n"
	if got := coxNormalize(w.String()); got != want {
		t.Fatalf("前提不成立：总用时之前的整串不对\n实际: %q\n期望: %q", got, want)
	}
	if !errors.Is(err, fuxTotalErr) {
		t.Fatalf("Submit 返回的错误应当是注入的写失败，实际 %v", err)
	}
}

// ─── MD1 ─────────────────────────────────────────────────────────────

// TestFuxAddGuardLoadFailure 钉 /add +编号 写前守卫的读失败分支：config.Load 失败时打一行、什么都不动。
// 照 R7 的装置：/add 拍快照之后，往配置文件末尾追加 bogus = 1。模板最后一张表是 [teams.all]，
// 所以未知键读出来是 teams.all.bogus（以实跑为准）。
//
// 【判别力】变异：守卫里 config.Load 失败时不 return（打完那一行继续往下走）。
// 这时 onDisk 为 nil，下一句 onDisk.DataDir 解引用 panic，runCtx 接住判红（rejected 子测试）。
//
// control 子测试是缺席断言的同装置正对照（fu2 补）：同一个 setup，追加的字节是空串而不是 "bogus = 1\n"，
// 其余一步不差。这时 /add +14 照常写入：打加入两行、文件变、多一个备份、r.cfg 换了、sonnet-5-high 在场。
// 所以 rejected 子测试里「没备份、指针不变、没有新成员、内存里没有 sonnet-5-high」这几条缺席断言都能为假。
// 另一条变异（fu2 实跑）：守卫的整份比对改成 if true || ...，control 红（没有新备份文件，newBak 判红），
// rejected 不受影响（它在 config.Load 那一步就返回了）。
func TestFuxAddGuardLoadFailure(t *testing.T) {
	// setup 两支共用：/add 拍快照，往配置文件末尾追加 tail，再 /add +14。
	setup := func(t *testing.T, tail string) (g *mdxRig, edited []byte, cfg0 *config.Config, active0, baks0, lines []string) {
		t.Helper()
		g = mdxNewRig(t, mdxConfigTemplate)
		g.run("/add")
		edited = append(g.readFile(), []byte(tail)...)
		g.writeFile(edited)
		cfg0 = g.r.cfg
		active0 = g.activeIDs()
		baks0 = g.bakNames()
		lines = g.run("/add +14")
		return g, edited, cfg0, active0, baks0, lines
	}

	t.Run("rejected", func(t *testing.T) {
		g, edited, cfg0, active0, _, lines := setup(t, "bogus = 1\n")
		mdxWantLines(t, "/add +14（文件里多了未知键）", lines, []string{
			"读配置文件失败: unknown config keys in " + g.cfgPath + ": teams.all.bogus",
		})
		if got := g.readFile(); string(got) != string(edited) {
			t.Errorf("读失败之后配置文件变了")
		}
		if b := g.bakNames(); len(b) != 0 {
			t.Errorf("不应当有备份文件，实际 %q", b)
		}
		if g.r.cfg != cfg0 {
			t.Errorf("r.cfg 指针不应当变")
		}
		if got := g.activeIDs(); !reflect.DeepEqual(got, active0) {
			t.Errorf("在场成员 = %q，want %q（不应当有新成员）", got, active0)
		}
		if _, ok := g.r.cfg.Agents["sonnet-5-high"]; ok {
			t.Errorf("内存配置里不应当有 sonnet-5-high")
		}
	})

	t.Run("control", func(t *testing.T) {
		g, edited, cfg0, active0, baks0, lines := setup(t, "")
		if len(baks0) != 0 {
			t.Fatalf("前提不成立：/add +14 之前不应当有备份文件，实际 %q", baks0)
		}
		bak := g.newBak(baks0)
		mdxWantLines(t, "/add +14（正对照：没有追加未知键）", lines, g.joinedLines("sonnet-5-high", bak))
		if got := g.readFile(); string(got) == string(edited) {
			t.Errorf("正对照：写入之后配置文件应当变了")
		}
		if b := g.bakNames(); len(b) != 1 {
			t.Errorf("正对照：备份文件应当恰好 1 个，实际 %q", b)
		}
		if g.r.cfg == cfg0 {
			t.Errorf("正对照：r.cfg 指针应当换了")
		}
		if got, want := g.activeIDs(), append(append([]string(nil), active0...), "sonnet-5-high"); !reflect.DeepEqual(got, want) {
			t.Errorf("正对照：在场成员 = %q，want %q", got, want)
		}
		if _, ok := g.r.cfg.Agents["sonnet-5-high"]; !ok {
			t.Errorf("正对照：内存配置里应当有 sonnet-5-high")
		}
	})
}

// TestFuxAddGuardCoversSummary 钉守卫的整份比对也管 SM1 新增的 Config.Summary（MD1 契约 open item (s)）。
// 照 R7：/add、前缀（只改注释的一次 /add +14 照常加入）之后，在文件末尾追加一张 [summary] 表，
// 指向夹具里已配置、read 权限、role 非空的 opus-5.0；再 /add +15。
//
// 前提守卫：改后的文件能 LoadAndValidate，且加载结果与 r.cfg 只差 Summary 这一个字段
// （把 Summary 抹回去之后 DeepEqual 成立）。所以守卫拒绝只能是因为 Summary。
// 选 opus-5.0 而不是新加的 sonnet-5-high：后者 role 为空，Load 会给总结成员补默认 role，
// Agents 也跟着变，就不止差 Summary 了。
//
// 【判别力】变异：整份比对之前把 onDisk.Summary 设成 r.cfg.Summary。这时守卫放行，
// /add +15 写入成功（多一个备份、文件变、打的是加入两行），行、文件字节、备份数、指针、在场成员五条断言都红。
func TestFuxAddGuardCoversSummary(t *testing.T) {
	const reloadLine = "配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add +15"
	g := mdxNewRig(t, mdxConfigTemplate)
	g.run("/add")
	g.writeFile(append(g.readFile(), []byte("\n# edited\n")...))
	baks := g.bakNames()
	lines := g.run("/add +14")
	bak := g.newBak(baks)
	mdxWantLines(t, "前缀 /add +14（只改过注释）", lines, g.joinedLines("sonnet-5-high", bak))

	cfg1 := g.r.cfg
	edited := string(g.readFile()) + "\n[summary]\nagent = 'opus-5.0'\n"
	g.writeFile([]byte(edited))

	onDisk, err := config.LoadAndValidate(g.cfgPath)
	if err != nil {
		t.Fatalf("前提：加了 [summary] 的配置应当能加载并通过校验: %v", err)
	}
	if onDisk.Summary.Agent != "opus-5.0" {
		t.Fatalf("前提：加载出的 Summary.Agent = %q，want opus-5.0", onDisk.Summary.Agent)
	}
	onDisk.Summary = cfg1.Summary
	if !reflect.DeepEqual(onDisk, cfg1) {
		t.Fatalf("前提：抹掉 Summary 之后加载结果应当与 r.cfg 相等（只差 Summary 一个字段）")
	}

	mdxWantLines(t, "/add +15", g.run("/add +15"), []string{reloadLine})
	if got := string(g.readFile()); got != edited {
		t.Errorf("被拒之后配置文件变了")
	}
	if b := g.bakNames(); len(b) != 1 {
		t.Errorf("备份文件应当恰好 1 个（前缀那一次），实际 %q", b)
	}
	if g.r.cfg != cfg1 {
		t.Errorf("r.cfg 指针不应当变")
	}
	if got := g.activeIDs(); !reflect.DeepEqual(got, []string{"sonnet-5-high"}) {
		t.Errorf("在场成员 = %q，want [sonnet-5-high]", got)
	}
}

// ─── SM1 ─────────────────────────────────────────────────────────────

// fuxSeedStaleBoth 给 smx_a、smx_b 各种一条陈旧在飞记录（TurnID 各不相同）。
func fuxSeedStaleBoth(ctx context.Context, svc *conversation.Service, convID string) error {
	for _, id := range []string{"smx_a", "smx_b"} {
		if err := svc.BeginTurn(ctx, conversation.InflightTurn{
			TurnID: "fux-stale-" + id, ConvID: convID, AgentID: id, Provider: "claude",
			StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, Tier: "read",
		}); err != nil {
			return err
		}
	}
	return nil
}

// TestFuxSummaryRounds2AllRefusedLast 钉 replied 的取法：--rounds 2 时两个成员第 1 轮都回复、
// 第 2 轮都被在飞闸拒绝，总结照跑（总结看得到第 1 轮的话），注明行列出两人第 2 轮的「拒绝」。
// 照 S7 rounds2_last_record 的钩子写法：第 1 轮「甲（」那一行写出时两人都已回来，给两人都种陈旧在飞记录。
//
// 第 2 轮两人都在闸上被拒、都不起进程，回调先后不固定，所以第 2 轮那四行接受两种顺序，
// 两种都整串写死；其余每一行逐字。
//
// 【判别力】变异：summary.go 里 replied 的判据改成只看每个成员的最后一条记录（与注明行同一种取法）。
// 这时两人最后一条都没有回复，打的是「总的来看：没有总结（这条 /round 没有成员回复）」、不起总结，整串红。
func TestFuxSummaryRounds2AllRefusedLast(t *testing.T) {
	g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, true, true)
	w := g.installHook("甲（", func() error { return fuxSeedStaleBoth(context.Background(), g.svc, g.convID) })
	g.submit(t, "/round smx_a smx_b --go --rounds 2")
	if fired, _, err := w.state(); !fired || err != nil {
		t.Fatalf("前提不成立：钩子应当触发且种记录成功，实际 触发=%v 错误=%v；输出:\n%s", fired, err, w.String())
	}
	head := "本轮问 2 个: smx_a, smx_b\n" +
		"smx_b（T）: OK\n" +
		"甲（T）: OK\n" +
		"本轮问 2 个: smx_a, smx_b\n"
	refA := "甲: 没有回复（拒绝）\n/round smx_a: " + smxInflight + "\n"
	refB := "smx_b: 没有回复（拒绝）\n/round smx_b: " + smxInflight + "\n"
	tail := "这些成员最后一次没有回复，总结看不到他们这一次的话：甲（拒绝）、smx_b（拒绝）\n" +
		"本轮问 1 个: smx_s\n" +
		smxSumHeadOK + "\n" +
		"总用时 7.2s\n"
	wantAB := head + refA + refB + tail
	wantBA := head + refB + refA + tail
	got := smxNormalize(w.String())
	if got != wantAB && got != wantBA {
		t.Fatalf("整串不对\n实际: %q\n期望（两种顺序之一）:\n%q\n%q", got, wantAB, wantBA)
	}
	smxClockIs(t, g, 2)
	g.requireSummaryMsg(t)
}

// fuxSummaryWriteErr 是总结段写屏幕失败时注入的错误。
var fuxSummaryWriteErr = errors.New("fux injected summary write failure")

// fuxFailAfterWriter 带锁；在一次含 trigger 的写入（照常写入）之后，其后的每一次写入都返回
// fuxSummaryWriteErr、不写入。seen 数含 trigger 的写入次数，failed 数被拒的写入次数。
type fuxFailAfterWriter struct {
	mu      sync.Mutex
	b       strings.Builder
	trigger string
	armed   bool
	seen    int
	failed  int
}

func (w *fuxFailAfterWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.armed {
		w.failed++
		return 0, fuxSummaryWriteErr
	}
	if strings.Contains(string(p), w.trigger) {
		w.seen++
		w.armed = true
	}
	return w.b.Write(p)
}

func (w *fuxFailAfterWriter) state() (string, int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String(), w.seen, w.failed
}

// TestFuxSummaryWriteErrorPropagates 钉总结段写屏幕失败时错误从 Submit 传出去。
// 详细档：cmdRound 的 defer 在详细档不写总用时，变异下不会有第二次写失败把错误补回来。
// 坏写者在「本轮问 1 个: smx_s」那一行写出之后开始报错，第一处失败落在总结段的成员块上。
//
// 【判别力】变异：summary.go 里总结段 runOneRound 的错误被吞掉（return err 改成 return nil）。
// 这时 Submit 返回 nil，errors.Is 红。前提守卫：触发行恰好一次、之后确有写入被拒。
func TestFuxSummaryWriteErrorPropagates(t *testing.T) {
	g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, false, false)
	w := &fuxFailAfterWriter{trigger: "本轮问 1 个: smx_s"}
	g.r.out = w
	err := g.r.Submit(g.ctx, "/round smx_a smx_b --go")
	out, seen, failed := w.state()
	if seen != 1 || failed < 1 {
		t.Fatalf("前提不成立：触发行应当恰好写出 1 次、其后至少一次写入被拒，实际 %d / %d；err=%v；输出:\n%s", seen, failed, err, out)
	}
	if !strings.HasSuffix(out, "本轮问 1 个: smx_s\n") {
		t.Fatalf("前提不成立：写出的最后一行应当是触发行；输出:\n%s", out)
	}
	if !errors.Is(err, fuxSummaryWriteErr) {
		t.Fatalf("Submit 返回的错误应当是注入的写失败，实际 %v；输出:\n%s", err, out)
	}
}
