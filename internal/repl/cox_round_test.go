package repl

// CO1 §2.2：/round、/ask、/say、/to、/verbose 的端到端用例。
// 真 SQLite（newRoundStreamService）、真 Runner、假 CLI（本包 TestMain 扮演 claude：
// argv 带 --resume 时睡 700ms 再应答，正文 OK）。grok 的命令路径不存在，用来造 spawn_failed。
// 引用本刀新增的 SetConcise、REPL 字段 now / concise、RoundEndEvent.Concise，在 HEAD 上编译不过是预期的。

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// coxRoundCfg 是 §2.2 冻结的配置。claude 的 Command 是测试二进制自己；
// grok 的 Command 指向一个不存在的路径，起进程必然失败（不起任何真 CLI）。
func coxRoundCfg() *config.Config {
	agent := func(display string, mode config.Mode) config.AgentConfig {
		return config.AgentConfig{
			DisplayName: display,
			Provider:    "claude",
			Model:       "claude-fable-5-1",
			Effort:      "high",
			Role:        "cox",
			Permissions: config.PermissionRead,
			Mode:        mode,
		}
	}
	x := agent("坏 X", config.ModeDiscussion)
	x.Provider = "grok"
	x.Model = "grok-4.7"
	x.Effort = ""
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: os.Args[0]},
			"grok":   {Command: filepath.Join(os.TempDir(), "cox-no-such-dir", "cox-missing.exe")},
		},
		Agents: map[string]config.AgentConfig{
			"cox_a": agent("甲", config.ModeDiscussion),
			"cox_b": agent("", config.ModeDiscussion),
			"cox_j": agent("裁判 J", config.ModeJudge),
			"cox_x": x,
		},
	}
}

// coxRoundCfgWith 返回 coxRoundCfg 的一份拷贝，Agents 是新 map，edit 可以改它。
func coxRoundCfgWith(edit func(agents map[string]config.AgentConfig)) *config.Config {
	cfg := coxRoundCfg()
	agents := make(map[string]config.AgentConfig, len(cfg.Agents)+1)
	for k, v := range cfg.Agents {
		agents[k] = v
	}
	edit(agents)
	cfg.Agents = agents
	return cfg
}

// coxRig 是 REPL 之前的那一半装置：配置、库、Runner、一条带种子消息的会话。
type coxRig struct {
	ctx    context.Context
	cfg    *config.Config
	svc    *conversation.Service
	rn     *turn.Runner
	convID string
}

// coxNewRig 按 §2.2 的顺序搭装置。cfg 为 nil 时用 coxRoundCfg()。
// Join 的 Provider 取配置里的 provider（cox_x、cox_y 是 grok），配置里没有的（cox_gone）按 claude。
func coxNewRig(t *testing.T, members []string, cfg *config.Config) *coxRig {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	if cfg == nil {
		cfg = coxRoundCfg()
	}
	svc := newRoundStreamService(t)
	rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})
	conv, err := svc.Create(ctx, "cox")
	if err != nil {
		t.Fatalf("装置：建会话失败: %v", err)
	}
	for _, id := range members {
		prov := "claude"
		if ac, ok := cfg.Agents[id]; ok && ac.Provider != "" {
			prov = ac.Provider
		}
		if err := svc.Join(ctx, conv.ID, conversation.Participant{
			AgentID: id, Provider: prov, JoinedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("装置：加入 %s 失败: %v", id, err)
		}
	}
	g := &coxRig{ctx: ctx, cfg: cfg, svc: svc, rn: rn, convID: conv.ID}
	g.post(t, "seed-msg")
	return g
}

// post 以用户身份发一条消息。
func (g *coxRig) post(t *testing.T, text string) {
	t.Helper()
	if _, err := g.svc.Post(g.ctx, conversation.Message{
		ConvID: g.convID, SenderID: conversation.ReservedSenderUser,
		Kind: conversation.KindUser, Text: text,
	}); err != nil {
		t.Fatalf("装置：发消息 %q 失败: %v", text, err)
	}
}

// ask 在 REPL 之外先问一次 id（前置）。
func (g *coxRig) ask(t *testing.T, id string) {
	t.Helper()
	if rec, err := g.rn.Ask(g.ctx, g.convID, id); err != nil {
		t.Fatalf("前置：先问 %s 失败: %v；Record=%+v", id, err, rec)
	}
}

// slowA 是「慢 a」前置：cox_a 首轮先问一次，再发 seed-msg-2。
// 之后 /round 里 cox_a 带 --resume、假 CLI 睡 700ms，首轮的成员立即回来，完成顺序固定。
func (g *coxRig) slowA(t *testing.T) {
	t.Helper()
	g.ask(t, "cox_a")
	g.post(t, "seed-msg-2")
}

// coxClock 是带锁、计数的假时钟：第 1 次返回 base，之后每次返回 base+7250ms。
type coxClock struct {
	mu sync.Mutex
	n  int
}

var coxClockBase = time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)

func (c *coxClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	if c.n == 1 {
		return coxClockBase
	}
	return coxClockBase.Add(7250 * time.Millisecond)
}

func (c *coxClock) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// newREPL 造 REPL、/use、清空缓冲、按需切简洁档、注入假时钟。
func (g *coxRig) newREPL(t *testing.T, out io.Writer, reset func(), concise bool) (*REPL, *coxClock) {
	t.Helper()
	r := New(g.cfg, g.svc, g.rn, nil, out, RulesContext{})
	if err := r.Submit(g.ctx, "/use "+g.convID); err != nil {
		t.Fatalf("装置：/use 失败: %v", err)
	}
	reset()
	if concise {
		r.SetConcise(true)
	}
	clk := &coxClock{}
	r.now = clk.now
	return r, clk
}

// coxRoundREPL 是最常用的组合：rbaBuf 当输出。
func coxRoundREPL(t *testing.T, g *coxRig, concise bool) (*REPL, *rbaBuf, *coxClock) {
	t.Helper()
	buf := &rbaBuf{}
	r, clk := g.newREPL(t, buf, buf.Reset, concise)
	return r, buf, clk
}

// 冻结的规范化：成员头用时换成 T，预算行的命令行余量换成 H。
var (
	coxHeadRe     = regexp.MustCompile(`（\d+\.\ds）`)
	coxHeadroomRe = regexp.MustCompile(`(?m)^(cox_[a-z]  未标定  未标定  预报\S+  不在飞  )-?\d+(  无样本)$`)
)

func coxNormalize(s string) string {
	s = coxHeadRe.ReplaceAllString(s, "（T）")
	return coxHeadroomRe.ReplaceAllString(s, "${1}H${2}")
}

// coxNormalizeCause 先守卫以 prefix 开头的行恰好 want 行，再把这些行的原因换成 <原因>。
// 只用于 R5 round2（/round cox_x: ，2 行）与 judge_spawn_failed（/round cox_y: ，1 行）。
func coxNormalizeCause(t *testing.T, raw, prefix string, want int) string {
	t.Helper()
	n := 0
	for _, l := range strings.Split(raw, "\n") {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	if n != want {
		t.Fatalf("前提不成立：以 %q 开头的行应当恰好 %d 行，实际 %d 行；原始输出:\n%s", prefix, want, n, raw)
	}
	re := regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(prefix) + `).+$`)
	return re.ReplaceAllString(raw, "${1}<原因>")
}

// coxLineIndex 返回第一行以 prefix 开头的行号，没有返回 -1。
func coxLineIndex(s, prefix string) int {
	for i, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, prefix) {
			return i
		}
	}
	return -1
}

// coxRequireBBeforeA 是慢 a 前置的前提守卫：cox_b（ 那一行在 甲（ 那一行之前。
func coxRequireBBeforeA(t *testing.T, raw string) {
	t.Helper()
	ib, ia := coxLineIndex(raw, "cox_b（"), coxLineIndex(raw, "甲（")
	if ib < 0 || ia < 0 || ib >= ia {
		t.Fatalf("前提不成立：慢 a 应当让 cox_b 先回来（cox_b 行 %d，甲 行 %d）；原始输出:\n%s", ib, ia, raw)
	}
}

// coxR1Want 是 R1 的整串期望（规范化后）。
const coxR1Want = "本轮问 2 个: cox_a, cox_b\n" +
	"cox_b（T）: OK\n" +
	"甲（T）: OK\n" +
	"本轮问 1 个: cox_j\n" +
	"裁判 J（T）: OK\n" +
	"总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）\n" +
	"总用时 7.2s\n"

// TestCoxRoundConciseGolden 是 R1：简洁档下一条 /round --go --with-judge 的整串输出。
//
// 【判别力】这一串同时钉住：发言顺序、预算表、扇出期间、outcome / 会话 / seq / 用量 / 金额 / 冷暖、
// 结算（含「分不清进程是否起来 0」与「意外冷启动」——慢 a 那一轮预报暖实测冷，详细档会打）全部不打；
// 显示名与 id 回落；总用时恰好一行且用 FormatWall（FormatElapsed 会印 7s；用真时钟会印一个大数）；
// 时钟恰好 2 次（入口一次、defer 一次）。对照是 R2（只差档位）。
func TestCoxRoundConciseGolden(t *testing.T) {
	g := coxNewRig(t, []string{"cox_a", "cox_b", "cox_j"}, nil)
	g.slowA(t)
	r, buf, clk := coxRoundREPL(t, g, true)
	if err := r.Submit(g.ctx, "/round cox_a cox_b cox_j --go --with-judge"); err != nil {
		t.Fatalf("/round 返回错误: %v", err)
	}
	raw := buf.String()
	coxRequireBBeforeA(t, raw)
	if got := coxNormalize(raw); got != coxR1Want {
		t.Fatalf("简洁档 /round 整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, coxR1Want), got, coxR1Want)
	}
	if n := clk.calls(); n != 2 {
		t.Fatalf("假时钟应当恰好被调用 2 次，实际 %d 次", n)
	}
}

// TestCoxRoundVerboseControl 是 R2：同 R1 装置与命令，不调 SetConcise。
//
// 【判别力】只差档位一处。第一行是「发言顺序」、两段结算标题各一次，证明详细档的输出还在
// （这几条在简洁档变异下会被藏掉）；「没有总用时」在忠实实现的详细档上结构上为真，
// 只在总用时的 r.concise 判据被拿掉时才红（RB-A 的 TestRbaR1NilObserverGolden 也会红）。
func TestCoxRoundVerboseControl(t *testing.T) {
	g := coxNewRig(t, []string{"cox_a", "cox_b", "cox_j"}, nil)
	g.slowA(t)
	r, buf, _ := coxRoundREPL(t, g, false)
	if err := r.Submit(g.ctx, "/round cox_a cox_b cox_j --go --with-judge"); err != nil {
		t.Fatalf("/round 返回错误: %v", err)
	}
	out := buf.String()
	lines := strings.Split(out, "\n")
	if lines[0] != "发言顺序按字典序，重复的 id 已去重，只问一次。" {
		t.Fatalf("详细档第一行应当是「发言顺序」那句，实际 %q；全部输出:\n%s", lines[0], out)
	}
	speak, judge := 0, 0
	for _, l := range lines {
		if strings.HasPrefix(l, "总用时") {
			t.Fatalf("详细档不应当打总用时，实际有一行 %q；全部输出:\n%s", l, out)
		}
		if l == "发言轮结算" {
			speak++
		}
		if l == "裁判轮结算" {
			judge++
		}
	}
	if speak != 1 || judge != 1 {
		t.Fatalf("整行「发言轮结算」「裁判轮结算」应当各恰好一次，实际 %d / %d；全部输出:\n%s", speak, judge, out)
	}
}

// coxRecObs 是记录型观察者：带锁；RoundStart / MemberDone 空实现，RoundEnd 追加事件。
// onEnd 可选：追加之后、放锁之后调它（R3 不设；R11 canceled 用它取消 ctx）。
type coxRecObs struct {
	mu    sync.Mutex
	ends  []RoundEndEvent
	onEnd func()
}

func (o *coxRecObs) RoundStart(RoundStartEvent) {}
func (o *coxRecObs) MemberDone(MemberDoneEvent) {}
func (o *coxRecObs) RoundEnd(ev RoundEndEvent) {
	o.mu.Lock()
	o.ends = append(o.ends, ev)
	o.mu.Unlock()
	if o.onEnd != nil {
		o.onEnd()
	}
}

func (o *coxRecObs) events() []RoundEndEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]RoundEndEvent(nil), o.ends...)
}

// TestCoxRoundObserverSuppressesTotal 是 R3：装了观察者（TUI）时 REPL 不打总用时，RoundEnd 带档位。
//
// 【判别力】concise 子测试整串等于 R1 去掉最后一行：去掉 r.roundObs != nil 判据时多一行总用时，红；
// 两个子测试的 RoundEnd 恰好 2 个且 Concise 随档位：RoundEnd 字面量漏赋 Concise 时 concise 子测试的事件是 false，红。
// verbose 子测试的「Concise 都是 false」在忠实实现上是零值、结构上为真，它只是 concise 子测试的对照。
// 对照是 R1（只差有没有观察者）。
func TestCoxRoundObserverSuppressesTotal(t *testing.T) {
	for _, tc := range []struct {
		name    string
		concise bool
	}{{"concise", true}, {"verbose", false}} {
		t.Run(tc.name, func(t *testing.T) {
			g := coxNewRig(t, []string{"cox_a", "cox_b", "cox_j"}, nil)
			g.slowA(t)
			r, buf, _ := coxRoundREPL(t, g, tc.concise)
			obs := &coxRecObs{}
			r.SetRoundObserver(obs)
			if err := r.Submit(g.ctx, "/round cox_a cox_b cox_j --go --with-judge"); err != nil {
				t.Fatalf("/round 返回错误: %v", err)
			}
			if tc.concise {
				raw := buf.String()
				coxRequireBBeforeA(t, raw)
				want := strings.TrimSuffix(coxR1Want, "总用时 7.2s\n")
				if got := coxNormalize(raw); got != want {
					t.Fatalf("装了观察者的简洁档整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
				}
			}
			evs := obs.events()
			if len(evs) != 2 {
				t.Fatalf("RoundEnd 应当恰好 2 个，实际 %d 个", len(evs))
			}
			for i, ev := range evs {
				if ev.Concise != tc.concise {
					t.Fatalf("第 %d 个 RoundEnd 的 Concise = %v，期望 %v", i, ev.Concise, tc.concise)
				}
			}
		})
	}
}

// TestCoxRoundPreviewConcise 是 R4：简洁档下的纯预览（不带 --go）。
//
// 【判别力】plain：纯预览两档都印预算表（!r.concise || !goRun 写成 !r.concise 时这里空）；
// 没起进程不打总用时（spawned 判据被拿掉时多一行）；「发言顺序」不打。
// with_judge：预览下裁判轮也走到 runOneRound、印完预算表返回 false，judgeRan 为假；
// E4 的门写成 judgeRan || true（#63）或 judgeRan || !goRun（#64）时多出「总用时 7.2s」、时钟 2 次，红。
// judgeRan || goRun（#65）这里杀不到（goRun 为假），它的杀手是 R5 judge_nodelta，两条合起来才覆盖那道门的两个方向。
// 缺席的那几行的正对照：R1（同样 --with-judge、裁判真起了进程，总用时在）与 R2（详细档，发言顺序在）。
func TestCoxRoundPreviewConcise(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a", "cox_b"}, nil)
		r, buf, _ := coxRoundREPL(t, g, true)
		if err := r.Submit(g.ctx, "/round cox_a cox_b"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		want := "成员  输入token  金额  冷启动  在飞  命令行余量  依据\n" +
			"cox_a  未标定  未标定  预报冷  不在飞  H  无样本\n" +
			"cox_b  未标定  未标定  预报冷  不在飞  H  无样本\n" +
			"合计输入token 0（只含已标定的行）\n" +
			"另有 2 名未标定\n"
		if got := coxNormalize(buf.String()); got != want {
			t.Fatalf("简洁档预览整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
		}
	})
	t.Run("with_judge", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a", "cox_j"}, nil)
		r, buf, clk := coxRoundREPL(t, g, true)
		if err := r.Submit(g.ctx, "/round cox_a cox_j --with-judge"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		want := "成员  输入token  金额  冷启动  在飞  命令行余量  依据\n" +
			"cox_a  未标定  未标定  预报冷  不在飞  H  无样本\n" +
			"合计输入token 0（只含已标定的行）\n" +
			"另有 1 名未标定\n" +
			"成员  输入token  金额  冷启动  在飞  命令行余量  依据\n" +
			"cox_j  未标定  未标定  预报冷  不在飞  H  无样本\n" +
			"合计输入token 0（只含已标定的行）\n" +
			"另有 1 名未标定\n"
		if got := coxNormalize(buf.String()); got != want {
			t.Fatalf("简洁档带裁判的预览整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
		}
		if n := clk.calls(); n != 1 {
			t.Fatalf("预览没起进程，假时钟应当恰好被调用 1 次，实际 %d 次", n)
		}
	})
}

// TestCoxRoundNoDeltaConcise 是 R5：简洁档下 no_delta、只有裁判起进程、第 2 轮、裁判 no_delta、裁判起不来。
//
// 【判别力】mixed：no_delta 行只打新追加的那一档（打全量 plan.Skipped 时 cox_gone 出现两次）、
// 分类跳过与「谁没被问」两档照打、发言者起了进程才记 spawned；only：没人起进程就没有总用时、
// 时钟只在入口调一次、「这一轮没有人可问。」照打；judge_only：只有裁判轮起进程也要记 spawned。三者互为对照。
// round2：no_delta 行与「分不清进程是否起来 1（…）」在第 2 个发言轮照打——门写成 k <= 1（#59、#61）时红；
// 对照是 mixed（第 1 轮的 no_delta 行）与 R6（第 1 轮的「分不清」那一行）。
// judge_nodelta：no_delta 行在裁判轮照打（门写成只在发言轮，#60，红）；它还是 --go 下「裁判轮没起进程」的
// 唯一夹具，E4 的门写成 judgeRan || goRun（#65）或 judgeRan || true（#63）时多出总用时、时钟 2 次，红；对照是 judge_only。
// judge_spawn_failed（主代理 2026-09-28 裁定采纳）：裁判轮里的失败行、/round <id>: 原因行、
// 简洁档唯一照打的结算行都在；#62（「分不清」只在发言轮打）与 #83（原因行只在发言轮打）下红，对照是 R6（同样的失败在发言轮）。
func TestCoxRoundNoDeltaConcise(t *testing.T) {
	const spawnLine = "分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）\n"
	cases := []struct {
		name    string
		members []string
		cfg     func() *config.Config
		pre     func(t *testing.T, g *coxRig)
		cmd     string
		cause   string // 非空时先守卫并规范化原因行
		causeN  int
		want    string
		clock   int
	}{
		{
			name:    "mixed",
			members: []string{"cox_a", "cox_b", "cox_gone", "cox_j"},
			pre:     func(t *testing.T, g *coxRig) { g.ask(t, "cox_b") },
			cmd:     "/round cox_a cox_b cox_gone cox_j --go",
			want:    "跳过 cox_gone（not_in_config）：cox_gone 不在配置里\n裁判 cox_j 本命令不问，要用 --with-judge 才问。\n跳过 cox_b（no_delta）：cox_b 无新内容，本轮跳过\n本轮问 1 个: cox_a\n甲（T）: OK\n总用时 7.2s\n",
			clock:   2,
		},
		{
			name:    "only",
			members: []string{"cox_b"},
			pre:     func(t *testing.T, g *coxRig) { g.ask(t, "cox_b") },
			cmd:     "/round cox_b --go",
			want:    "跳过 cox_b（no_delta）：cox_b 无新内容，本轮跳过\n这一轮没有人可问。\n",
			clock:   1,
		},
		{
			name:    "judge_only",
			members: []string{"cox_b", "cox_j"},
			pre:     func(t *testing.T, g *coxRig) { g.ask(t, "cox_b") },
			cmd:     "/round cox_b cox_j --go --with-judge",
			want:    "跳过 cox_b（no_delta）：cox_b 无新内容，本轮跳过\n这一轮没有人可问。\n本轮问 1 个: cox_j\n裁判 J（T）: OK\n总用时 7.2s\n",
			clock:   2,
		},
		{
			name:    "round2",
			members: []string{"cox_a", "cox_x"},
			pre:     func(t *testing.T, g *coxRig) { g.slowA(t) },
			cmd:     "/round cox_a cox_x --go --rounds 2",
			cause:   "/round cox_x: ",
			causeN:  2,
			want: "本轮问 2 个: cox_a, cox_x\n坏 X: 没有回复（没起来）\n/round cox_x: <原因>\n甲（T）: OK\n" + spawnLine +
				"跳过 cox_a（no_delta）：cox_a 无新内容，本轮跳过\n本轮问 1 个: cox_x\n坏 X: 没有回复（没起来）\n/round cox_x: <原因>\n" + spawnLine +
				"总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）\n" +
				"总用时 7.2s\n",
			clock: 2,
		},
		{
			name:    "judge_nodelta",
			members: []string{"cox_j"},
			pre:     func(t *testing.T, g *coxRig) { g.ask(t, "cox_j") },
			cmd:     "/round cox_j --go --with-judge",
			want:    "这一轮没有人可问。\n跳过 cox_j（no_delta）：cox_j 无新内容，本轮跳过\n这一轮没有人可问。\n",
			clock:   1,
		},
		{
			name:    "judge_spawn_failed",
			members: []string{"cox_a", "cox_y"},
			cfg: func() *config.Config {
				return coxRoundCfgWith(func(agents map[string]config.AgentConfig) {
					y := agents["cox_x"]
					y.DisplayName = "坏裁判"
					y.Mode = config.ModeJudge
					agents["cox_y"] = y
				})
			},
			cmd:    "/round cox_a cox_y --go --with-judge",
			cause:  "/round cox_y: ",
			causeN: 1,
			want:   "本轮问 1 个: cox_a\n甲（T）: OK\n本轮问 1 个: cox_y\n坏裁判: 没有回复（没起来）\n/round cox_y: <原因>\n" + spawnLine + "总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）\n" + "总用时 7.2s\n",
			clock:  2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg *config.Config
			if tc.cfg != nil {
				cfg = tc.cfg()
			}
			g := coxNewRig(t, tc.members, cfg)
			if tc.pre != nil {
				tc.pre(t, g)
			}
			r, buf, clk := coxRoundREPL(t, g, true)
			if err := r.Submit(g.ctx, tc.cmd); err != nil {
				t.Fatalf("%s 返回错误: %v", tc.cmd, err)
			}
			raw := buf.String()
			if tc.cause != "" {
				raw = coxNormalizeCause(t, raw, tc.cause, tc.causeN)
			}
			if got := coxNormalize(raw); got != tc.want {
				t.Fatalf("整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, tc.want), got, tc.want)
			}
			if n := clk.calls(); n != tc.clock {
				t.Fatalf("假时钟应当恰好被调用 %d 次，实际 %d 次", tc.clock, n)
			}
		})
	}
}

// TestCoxRoundSpawnFailedConcise 是 R6：一个成员起不来（grok 路径不存在）时的简洁档输出。
//
// 【判别力】spawn_failed 这一档：失败行、/round <id>: 原因行、结算里唯一照打的那一行（N>0）都在，
// 而结算的其余行不在（整体恰好 7 行）；把那一行改成另一份字面量（比如只印「分不清进程是否起来 1」）时第 4 行红；
// 判据写成 > 1 时那一行没了，红。两个成员的先后不断言（没有慢 a，先后不固定）。
func TestCoxRoundSpawnFailedConcise(t *testing.T) {
	g := coxNewRig(t, []string{"cox_a", "cox_x"}, nil)
	r, buf, _ := coxRoundREPL(t, g, true)
	if err := r.Submit(g.ctx, "/round cox_a cox_x --go"); err != nil {
		t.Fatalf("/round 返回错误: %v", err)
	}
	got := coxNormalize(buf.String())
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 7 {
		t.Fatalf("应当恰好 7 行，实际 %d 行:\n%s", len(lines), got)
	}
	if lines[0] != "本轮问 2 个: cox_a, cox_x" {
		t.Fatalf("第 0 行不对: %q", lines[0])
	}
	if lines[4] != "分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）" {
		t.Fatalf("第 4 行不对: %q；全部:\n%s", lines[4], got)
	}
	if lines[5] != "总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）" {
		t.Fatalf("第 5 行不对: %q；全部:\n%s", lines[5], got)
	}
	if lines[6] != "总用时 7.2s" {
		t.Fatalf("第 6 行不对: %q；全部:\n%s", lines[6], got)
	}
	fail := -1
	for i := 1; i <= 3; i++ {
		if lines[i] == "坏 X: 没有回复（没起来）" {
			if fail >= 0 {
				t.Fatalf("失败行出现不止一次:\n%s", got)
			}
			fail = i
		}
	}
	if fail < 0 {
		t.Fatalf("第 1-3 行里没有整行「坏 X: 没有回复（没起来）」:\n%s", got)
	}
	if fail+1 > 3 || !strings.HasPrefix(lines[fail+1], "/round cox_x: ") {
		t.Fatalf("前提不成立：失败行之后应当紧跟以「/round cox_x: 」开头的原因行:\n%s", got)
	}
	heads := 0
	for _, l := range lines {
		if l == "甲（T）: OK" {
			heads++
		}
	}
	if heads != 1 {
		t.Fatalf("整行「甲（T）: OK」应当恰好一次，实际 %d 次:\n%s", heads, got)
	}
}

// TestCoxAskSayConcise 是 R7：简洁档下 /ask 带文本、/say、/to。
//
// 【判别力】「已记下 seq N」（三次落库）与「点名了 cox_b」不打；「不是当前会话的成员」「只有…会看到」
// 「正在问」照打；/ask 没有总用时、时钟 0 次（/ask 不经 cmdRound）。任何一处门加错或漏加，整串多一行或少一行。
func TestCoxAskSayConcise(t *testing.T) {
	g := coxNewRig(t, []string{"cox_a", "cox_b"}, nil)
	r, buf, clk := coxRoundREPL(t, g, true)
	for _, cmd := range []string{"/ask cox_a 你好 @cox_b @nobody", "/say hi", "/to cox_a 只给甲"} {
		if err := r.Submit(g.ctx, cmd); err != nil {
			t.Fatalf("%s 返回错误: %v", cmd, err)
		}
	}
	want := "nobody 不是当前会话的成员\n" +
		"正在问 cox_a ...\n" +
		"甲（T）: OK\n" +
		"只有 cox_a 会看到这条；其余成员永远不会 —— 游标只前进\n"
	if got := coxNormalize(buf.String()); got != want {
		t.Fatalf("整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
	}
	if n := clk.calls(); n != 0 {
		t.Fatalf("/ask /say /to 不应当调用时钟，实际 %d 次", n)
	}
}

// TestCoxAskRefusedConcise 是 R8：成员有一条没记账的在飞记录，/ask 被拒。
//
// 【判别力】/ask 的原因行（askCause）在简洁档照打；失败行不带用时（拒绝的轮 WallMS 为 0，带了会印（0.0s））。
// 缓冲不规范化，整串逐字。
func TestCoxAskRefusedConcise(t *testing.T) {
	g := coxNewRig(t, []string{"cox_a"}, nil)
	if err := g.svc.BeginTurn(g.ctx, conversation.InflightTurn{
		TurnID: "cox-stale-turn", ConvID: g.convID, AgentID: "cox_a", Provider: "claude",
		StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, Tier: "read",
	}); err != nil {
		t.Fatalf("前置：写在飞记录失败: %v", err)
	}
	r, buf, _ := coxRoundREPL(t, g, true)
	if err := r.Submit(g.ctx, "/ask cox_a"); err != nil {
		t.Fatalf("/ask 返回错误: %v", err)
	}
	want := "正在问 cox_a ...\n" +
		"甲: 没有回复（拒绝）\n" +
		"这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行\n"
	if got := buf.String(); got != want {
		t.Fatalf("整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
	}
}

// §3.3 与 §3.4 的冻结字面量，逐字。
const (
	coxMsgConcise = "已切到简洁输出：只显示成员的回复、用时，以及关系到钱或内容丢失的提示；/verbose on 切到详细"
	coxMsgVerbose = "已切到详细输出：用量、金额、冷暖、会话 id、预算表与结算全部显示；/verbose off 回到简洁"
	coxMsgUsage   = "用法: /verbose 在简洁与详细之间切换；/verbose on 切到详细；/verbose off 切到简洁"
	coxHelpVerb   = "切换输出档位: /verbose 在简洁与详细之间切换；/verbose on 详细；/verbose off 简洁"
)

// TestCoxVerboseCommand 是 R9：/verbose 的切换、设定、用法，以及档位确实被后续命令读到、命令表登记。
//
// 【判别力】步 1、2 钉切换（不带参数设成详细而不是切换时步 1 的档位红）；步 3-6 钉 on / off 是设定、
// 重复设定不翻转；步 7、8、10 钉用法且不改档位（接受多余参数、大小写不敏感时红）；
// 之后 /ask 在简洁档印成员头、切回详细后第 2 行是 outcome 行，证明档位被后续命令读到；
// /help 那一行钉命令表登记与 summary 逐字（命令改名或漏登记时红）。
func TestCoxVerboseCommand(t *testing.T) {
	g := coxNewRig(t, []string{"cox_a"}, nil)
	r, buf, _ := coxRoundREPL(t, g, false)
	steps := []struct {
		cmd     string
		want    string
		concise bool
	}{
		{"/verbose", coxMsgConcise, true},
		{"/verbose", coxMsgVerbose, false},
		{"/verbose off", coxMsgConcise, true},
		{"/verbose off", coxMsgConcise, true},
		{"/verbose on", coxMsgVerbose, false},
		{"/verbose on", coxMsgVerbose, false},
		{"/verbose bad", coxMsgUsage, false},
		{"/verbose on off", coxMsgUsage, false},
		{"/verbose off", coxMsgConcise, true},
		{"/verbose ON", coxMsgUsage, true},
	}
	for i, s := range steps {
		buf.Reset()
		if err := r.Submit(g.ctx, s.cmd); err != nil {
			t.Fatalf("第 %d 步 %s 返回错误: %v", i+1, s.cmd, err)
		}
		if got := buf.String(); got != s.want+"\n" {
			t.Fatalf("第 %d 步 %s 的输出不对\n实际: %q\n期望: %q", i+1, s.cmd, got, s.want+"\n")
		}
		if r.concise != s.concise {
			t.Fatalf("第 %d 步 %s 之后 r.concise = %v，期望 %v", i+1, s.cmd, r.concise, s.concise)
		}
	}

	buf.Reset()
	if err := r.Submit(g.ctx, "/ask cox_a"); err != nil {
		t.Fatalf("简洁档 /ask 返回错误: %v", err)
	}
	if got, want := coxNormalize(buf.String()), "正在问 cox_a ...\n甲（T）: OK\n"; got != want {
		t.Fatalf("简洁档 /ask 整串不对\n实际: %q\n期望: %q", got, want)
	}

	if err := r.Submit(g.ctx, "/verbose on"); err != nil {
		t.Fatalf("/verbose on 返回错误: %v", err)
	}
	g.post(t, "more")
	buf.Reset()
	if err := r.Submit(g.ctx, "/ask cox_a"); err != nil {
		t.Fatalf("详细档 /ask 返回错误: %v", err)
	}
	lines := strings.Split(buf.String(), "\n")
	if len(lines) < 2 || lines[1] != "outcome spoke  agent cox_a" {
		t.Fatalf("详细档 /ask 的第 2 行应当是 outcome 行；全部输出:\n%s", buf.String())
	}

	buf.Reset()
	if err := r.Submit(g.ctx, "/help"); err != nil {
		t.Fatalf("/help 返回错误: %v", err)
	}
	found := false
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(l, "/verbose ") {
			found = true
			if got := strings.TrimSpace(strings.TrimPrefix(l, "/verbose")); got != coxHelpVerb {
				t.Fatalf("/help 里 /verbose 的 summary 不对\n实际: %q\n期望: %q", got, coxHelpVerb)
			}
		}
	}
	if !found {
		t.Fatalf("/help 里没有以「/verbose 」开头的行:\n%s", buf.String())
	}
}

// coxWriteErr 是 R10 失败写者注入的错误。
var coxWriteErr = errors.New("cox injected write failure")

// coxFailWriter 带锁；把写入累积到内部缓冲；含「本轮问 」的写入计数，第 2 次返回 coxWriteErr、不写入。
type coxFailWriter struct {
	mu   sync.Mutex
	b    strings.Builder
	hits int
}

func (w *coxFailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if strings.Contains(string(p), "本轮问 ") {
		w.hits++
		if w.hits == 2 {
			return 0, coxWriteErr
		}
	}
	return w.b.Write(p)
}

func (w *coxFailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func (w *coxFailWriter) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.b.Reset()
	w.hits = 0
}

func (w *coxFailWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hits
}

// TestCoxRoundWriteErrorKeepsError 是 R10：第 1 段起过进程之后，第 2 段写屏幕失败。
//
// 【判别力】spawned 已为真、简洁、行式，四个判据里只剩 err != nil 挡着总用时；
// defer 若不看 err，会在失败之后再打一行总用时并把返回的错误换成 nil——errors.Is 与「不含总用时」都红。
// 计数恰好 2 是前提守卫（证明失败确实发生在第 2 段）。
func TestCoxRoundWriteErrorKeepsError(t *testing.T) {
	g := coxNewRig(t, []string{"cox_a", "cox_b"}, nil)
	w := &coxFailWriter{}
	r, _ := g.newREPL(t, w, w.Reset, true)
	err := r.Submit(g.ctx, "/round cox_a cox_b --go --rounds 2")
	if n := w.count(); n != 2 {
		t.Fatalf("前提不成立：含「本轮问 」的写入应当恰好 2 次，实际 %d 次；err=%v；输出:\n%s", n, err, w.String())
	}
	if !errors.Is(err, coxWriteErr) {
		t.Fatalf("返回的错误应当是注入的写失败，实际 %v；输出:\n%s", err, w.String())
	}
	if strings.Contains(w.String(), "总用时") {
		t.Fatalf("写失败之后不应当再打总用时；输出:\n%s", w.String())
	}
}
