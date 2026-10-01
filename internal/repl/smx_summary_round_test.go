package repl

// SM1 §2.0 共用装置与 §2.4 端到端用例：真 SQLite（newRoundStreamService）、真 Runner、
// 假 CLI（本包 TestMain 扮演 claude：argv 带 --resume 时睡 700ms 再应答，正文 OK）。
// grok 的命令路径不存在，用来造 spawn_failed。
// 引用本刀的 Config.Summary 与 CO1 的 SetConcise、REPL 字段 now / concise、RoundEndEvent.Concise，
// 在两刀都落地之前编译不过是预期的红。

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// ─── §2.0 共用装置 ───────────────────────────────────────────────────

// smxCfg 是 §2.0 冻结的配置。claude 的 Command 是测试二进制自己；grok 指向一个不存在的路径。
func smxCfg(summary string) *config.Config {
	agent := func(display string, mode config.Mode) config.AgentConfig {
		return config.AgentConfig{
			DisplayName: display,
			Provider:    "claude",
			Model:       "claude-fable-5-1",
			Effort:      "high",
			Role:        "smx",
			Permissions: config.PermissionRead,
			Mode:        mode,
		}
	}
	bad := func(display string, mode config.Mode) config.AgentConfig {
		a := agent(display, mode)
		a.Provider = "grok"
		a.Model = "grok-4.7"
		a.Effort = ""
		return a
	}
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: os.Args[0]},
			"grok":   {Command: filepath.Join(os.TempDir(), "smx-no-such-dir", "smx-missing.exe")},
		},
		Agents: map[string]config.AgentConfig{
			"smx_a": agent("甲", config.ModeDiscussion),
			"smx_b": agent("", config.ModeDiscussion),
			"smx_j": agent("裁判 J", config.ModeJudge),
			"smx_s": agent("总结 S", config.ModeDiscussion),
			"smx_x": bad("坏 X", config.ModeDiscussion),
			"smx_y": bad("坏 Y", config.ModeDiscussion),
			"smx_z": bad("坏 Z", config.ModeDiscussion),
			"smx_k": bad("坏 K", config.ModeJudge),
		},
		Summary: config.SummaryConfig{Agent: summary},
	}
}

// smxClock 是带锁、计数的假时钟：第 1 次返回 base，之后每次返回 base+7250ms（总用时恒为 7.2s）。
type smxClock struct {
	mu sync.Mutex
	n  int
}

var smxClockBase = time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)

func (c *smxClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	if c.n == 1 {
		return smxClockBase
	}
	return smxClockBase.Add(7250 * time.Millisecond)
}

func (c *smxClock) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// smxRig 是一次 smxSetup 的全部装置。
type smxRig struct {
	ctx    context.Context
	svc    *conversation.Service
	rn     *turn.Runner
	r      *REPL
	buf    *rbaBuf
	clk    *smxClock
	convID string
}

// smxSetup 按 §2.0 的顺序搭装置（会话标题 smx 冻结，S2 的 prompt_bytes 依赖它）。
func smxSetup(t *testing.T, summary string, members []string, concise, slowA bool) *smxRig {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	cfg := smxCfg(summary)
	svc := newRoundStreamService(t)
	rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})
	conv, err := svc.Create(ctx, "smx")
	if err != nil {
		t.Fatalf("装置：建会话失败: %v", err)
	}
	for _, id := range members {
		if err := svc.Join(ctx, conv.ID, conversation.Participant{
			AgentID: id, Provider: cfg.Agents[id].Provider, JoinedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("装置：加入 %s 失败: %v", id, err)
		}
	}
	g := &smxRig{ctx: ctx, svc: svc, rn: rn, convID: conv.ID}
	g.post(t, "seed-msg")
	if slowA {
		if rec, err := rn.Ask(ctx, conv.ID, "smx_a"); err != nil {
			t.Fatalf("装置：慢 a 前置先问 smx_a 失败: %v；Record=%+v", err, rec)
		}
		g.post(t, "seed-msg-2")
	}
	g.buf = &rbaBuf{}
	g.r = New(cfg, svc, rn, nil, g.buf, RulesContext{})
	if err := g.r.Submit(ctx, "/use "+conv.ID); err != nil {
		t.Fatalf("装置：/use 失败: %v", err)
	}
	g.buf.Reset()
	if concise {
		g.r.SetConcise(true)
	}
	g.clk = &smxClock{}
	g.r.now = g.clk.now
	return g
}

// post 以用户身份发一条消息，返回落库后的消息。
func (g *smxRig) post(t *testing.T, text string) conversation.Message {
	t.Helper()
	m, err := g.svc.Post(g.ctx, conversation.Message{
		ConvID: g.convID, SenderID: conversation.ReservedSenderUser,
		Kind: conversation.KindUser, Text: text,
	})
	if err != nil {
		t.Fatalf("装置：发消息 %q 失败: %v", text, err)
	}
	return m
}

// submit 用装置 ctx 提交一条命令，要求返回 nil。
func (g *smxRig) submit(t *testing.T, cmd string) {
	t.Helper()
	if err := g.r.Submit(g.ctx, cmd); err != nil {
		t.Fatalf("%s 返回错误: %v", cmd, err)
	}
}

// leave 让 smx_s 退出会话（S7 left、S13、S14 的「离开」）。
func (g *smxRig) leave(t *testing.T) {
	t.Helper()
	if err := g.svc.Leave(g.ctx, g.convID, "smx_s", time.Now().UTC()); err != nil {
		t.Fatalf("装置：smx_s 退出失败: %v", err)
	}
}

// member 在 Participants(includeInactive=true) 里找 id。
func (g *smxRig) member(t *testing.T, id string) (conversation.Participant, bool) {
	t.Helper()
	ps, err := g.svc.Participants(g.ctx, g.convID, true)
	if err != nil {
		t.Fatalf("读成员表失败: %v", err)
	}
	for _, p := range ps {
		if p.AgentID == id {
			return p, true
		}
	}
	return conversation.Participant{}, false
}

// msgsFrom 取库里 SenderID 为 id 的全部消息。
func (g *smxRig) msgsFrom(t *testing.T, id string) []conversation.Message {
	t.Helper()
	all, err := g.svc.History(g.ctx, g.convID, 0, 10000)
	if err != nil {
		t.Fatalf("读历史失败: %v", err)
	}
	var out []conversation.Message
	for _, m := range all {
		if m.SenderID == id {
			out = append(out, m)
		}
	}
	return out
}

// requireNotMember：smx_s 不在成员表里（没 Join）。
func (g *smxRig) requireNotMember(t *testing.T, id string) {
	t.Helper()
	if p, ok := g.member(t, id); ok {
		t.Fatalf("%s 不应当在成员表里，实际在（Active=%v）", id, p.Active)
	}
}

// requireNoMsgs：库里没有 SenderID 为 id 的消息。
func (g *smxRig) requireNoMsgs(t *testing.T, id string) {
	t.Helper()
	if ms := g.msgsFrom(t, id); len(ms) != 0 {
		t.Fatalf("库里不应当有 %s 的消息，实际 %d 条: %+v", id, len(ms), ms)
	}
}

// requireLeftInactive：smx_s 在表里且未激活。
func (g *smxRig) requireLeftInactive(t *testing.T) {
	t.Helper()
	p, ok := g.member(t, "smx_s")
	if !ok || p.Active {
		t.Fatalf("smx_s 应当在成员表里且未激活，实际 在表=%v Active=%v", ok, p.Active)
	}
}

// requireSummaryMsg：smx_s 被 Join 且激活、库里恰好 1 条 smx_s 的消息且只给它自己。
func (g *smxRig) requireSummaryMsg(t *testing.T) conversation.Message {
	t.Helper()
	p, ok := g.member(t, "smx_s")
	if !ok || !p.Active {
		t.Fatalf("smx_s 应当在成员表里且激活，实际 在表=%v Active=%v", ok, p.Active)
	}
	ms := g.msgsFrom(t, "smx_s")
	if len(ms) != 1 {
		t.Fatalf("库里 smx_s 的消息应当恰好 1 条，实际 %d 条", len(ms))
	}
	if !reflect.DeepEqual(ms[0].Recipients, []string{"smx_s"}) {
		t.Fatalf("总结消息的 Recipients 应当是 [smx_s]，实际 %q", ms[0].Recipients)
	}
	return ms[0]
}

// seedStale 给 smx_b 种一条陈旧在飞记录（§2.0，TurnID 是固定字面量）。
func smxSeedStale(ctx context.Context, svc *conversation.Service, convID string) error {
	return svc.BeginTurn(ctx, conversation.InflightTurn{
		TurnID: "smx-stale-turn", ConvID: convID, AgentID: "smx_b", Provider: "claude",
		StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, Tier: "read",
	})
}

// smxHookWriter 嵌 rbaBuf，写入照常累积；第一次写入的内容以 prefix 开头时调一次 hook（锁外调）。
// matches 数以 prefix 开头的写入次数；hook 返回的错误记下，由测试 goroutine 事后当前提守卫检查
// （hook 可能跑在 turn 的 worker goroutine 上，那里不能 t.Fatalf）。
type smxHookWriter struct {
	rbaBuf
	prefix string
	hook   func() error

	hmu     sync.Mutex
	fired   bool
	matches int
	hookErr error
}

func (w *smxHookWriter) Write(p []byte) (int, error) {
	n, err := w.rbaBuf.Write(p)
	call := false
	w.hmu.Lock()
	if strings.HasPrefix(string(p), w.prefix) {
		w.matches++
		if !w.fired {
			w.fired = true
			call = true
		}
	}
	w.hmu.Unlock()
	if call && w.hook != nil {
		if herr := w.hook(); herr != nil {
			w.hmu.Lock()
			w.hookErr = herr
			w.hmu.Unlock()
		}
	}
	return n, err
}

func (w *smxHookWriter) state() (bool, int, error) {
	w.hmu.Lock()
	defer w.hmu.Unlock()
	return w.fired, w.matches, w.hookErr
}

// installHook 把 r.out 换成钩子写者（白盒，同包）。
func (g *smxRig) installHook(prefix string, hook func() error) *smxHookWriter {
	w := &smxHookWriter{prefix: prefix, hook: hook}
	g.r.out = w
	return w
}

// requireFired 是钩子的前提守卫：触发过、恰好匹配 want 次、hook 没出错。
func requireFired(t *testing.T, w *smxHookWriter, want int) {
	t.Helper()
	fired, n, err := w.state()
	if !fired || n != want || err != nil {
		t.Fatalf("前提不成立：钩子（前缀 %q）应当触发、恰好匹配 %d 次且没有出错，实际 触发=%v 匹配=%d 错误=%v；输出:\n%s", w.prefix, want, fired, n, err, w.String())
	}
}

// 冻结的规范化：成员头用时换成 T，预算行的命令行余量换成 H，wall_ms 换成 W。
var (
	smxHeadRe     = regexp.MustCompile(`（\d+\.\ds）`)
	smxHeadroomRe = regexp.MustCompile(`(?m)^(smx_[a-z]  未标定  未标定  预报\S+  不在飞  )-?\d+(  无样本)$`)
	smxWallRe     = regexp.MustCompile(`wall_ms=\d+`)
	smxCauseRe    = regexp.MustCompile(`(?m)^(/round smx_[xk]: ).+$`)
)

func smxNormalize(s string) string {
	s = smxHeadRe.ReplaceAllString(s, "（T）")
	s = smxHeadroomRe.ReplaceAllString(s, "${1}H${2}")
	return smxWallRe.ReplaceAllString(s, "wall_ms=W")
}

// smxLines 按 "\n" 切行并去掉末尾空串。
func smxLines(s string) []string {
	ls := strings.Split(s, "\n")
	if len(ls) > 0 && ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

func smxCount(lines []string, want string) int {
	n := 0
	for _, l := range lines {
		if l == want {
			n++
		}
	}
	return n
}

func smxIndex(s, prefix string) int {
	for i, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, prefix) {
			return i
		}
	}
	return -1
}

// smxRequireBBeforeA 是慢 a 的前提守卫：smx_b（ 那一行在 甲（ 那一行之前。
func smxRequireBBeforeA(t *testing.T, raw string) {
	t.Helper()
	ib, ia := smxIndex(raw, "smx_b（"), smxIndex(raw, "甲（")
	if ib < 0 || ia < 0 || ib >= ia {
		t.Fatalf("前提不成立：慢 a 应当让 smx_b 先回来（smx_b 行 %d，甲 行 %d）；原始输出:\n%s", ib, ia, raw)
	}
}

func smxEq(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s 整串不对\n实际: %q\n期望: %q", what, got, want)
	}
}

func smxClockIs(t *testing.T, g *smxRig, want int) {
	t.Helper()
	if n := g.clk.calls(); n != want {
		t.Fatalf("假时钟应当恰好被调用 %d 次，实际 %d 次", want, n)
	}
}

// §2.0 与 §3.2 的字面量（测试侧独立抄写）。
const (
	smxHint        = "总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）"
	smxSpawnLine   = "分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）"
	smxSpawn2Line  = "分不清进程是否起来 2（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）"
	smxRefuse      = "拒绝: smx_s 是总结成员（config.toml 的 [summary]），不在 /round 里发言；它的话只给你看，其余成员看不到。\n"
	smxInflight    = "这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行"
	smxNoReply     = "总的来看：没有总结（这条 /round 没有成员回复）"
	smxLeftLine    = "总的来看：没有总结（smx_s 已退出当前会话；要恢复总结用 /join smx_s）"
	smxNoteX       = "这些成员最后一次没有回复，总结看不到他们这一次的话：坏 X（没起来）"
	smxSumHeadOK   = "总的来看（总结 S 做总结）：OK"
	smxS1Want      = "本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n本轮问 1 个: smx_s\n" + smxSumHeadOK + "\n总用时 7.2s\n"
	smxS10Want     = "本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n总用时 7.2s\n"
	smxVerboseTail = "意外冷启动 smx_a"
)

// ─── S1 ──────────────────────────────────────────────────────────────

// TestSmxRoundSummaryConciseGolden 是 S1：简洁档两成员加总结的整串、自动 Join、私密、单独的 round_id。
//
// 【判别力】一条用例同时钉：屏幕格式与位置（总结在 CO1 的总用时之前；头的全角冒号、不留空格、显示名）、
// 自动 Join（连同 Join 字面量的 DisplayName / Provider：写成空串时红，§4 #51、#52）、库里的 Recipients、
// 单独一轮 round_id、以及真 Runner 上下一轮不带总结（私密性的端到端：Delivered 不含总结消息的 seq）。
// 对照：S3 no_summary（只差 flag）、S3 unconfigured（只差配置）。
func TestSmxRoundSummaryConciseGolden(t *testing.T) {
	g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, true, true)
	g.submit(t, "/round smx_a smx_b --go")
	raw := g.buf.String()
	smxRequireBBeforeA(t, raw)
	smxEq(t, "简洁档 /round 加总结", smxNormalize(raw), smxS1Want)
	smxClockIs(t, g, 2)

	p, ok := g.member(t, "smx_s")
	if !ok || !p.Active {
		t.Fatalf("smx_s 应当被自动 Join 且激活，实际 在表=%v Active=%v", ok, p.Active)
	}
	if p.DisplayName != "总结 S" || p.Provider != "claude" {
		t.Fatalf("自动 Join 应当用配置里的 display_name / provider，实际 DisplayName=%q Provider=%q", p.DisplayName, p.Provider)
	}
	sum := g.requireSummaryMsg(t)
	if sum.Text != "OK" {
		t.Fatalf("总结消息的正文应当是 OK，实际 %q", sum.Text)
	}
	su, err := g.svc.TurnUsage(g.ctx, g.convID, "smx_s", 10)
	if err != nil {
		t.Fatalf("读 smx_s 的 turn_usage 失败: %v", err)
	}
	bu, err := g.svc.TurnUsage(g.ctx, g.convID, "smx_b", 10)
	if err != nil {
		t.Fatalf("读 smx_b 的 turn_usage 失败: %v", err)
	}
	if len(su) != 1 || len(bu) != 1 {
		t.Fatalf("turn_usage 应当 smx_s、smx_b 各恰好 1 行，实际 %d / %d", len(su), len(bu))
	}
	if su[0].RoundID == "" || su[0].RoundID == bu[0].RoundID {
		t.Fatalf("总结应当有自己的 round_id（非空且不同于发言轮），实际 smx_s=%q smx_b=%q", su[0].RoundID, bu[0].RoundID)
	}

	after := g.post(t, "after")
	rec, err := g.rn.Ask(g.ctx, g.convID, "smx_b")
	if err != nil {
		t.Fatalf("总结之后再问 smx_b 失败: %v；Record=%+v", err, rec)
	}
	hasAfter, hasSum := false, false
	for _, s := range rec.Delivered {
		if s == after.Seq {
			hasAfter = true
		}
		if s == sum.Seq {
			hasSum = true
		}
	}
	if !hasAfter || hasSum {
		t.Fatalf("下一轮 smx_b 的 Delivered 应当含 after（seq %d）、不含总结（seq %d），实际 %v", after.Seq, sum.Seq, rec.Delivered)
	}
}

// ─── S2 ──────────────────────────────────────────────────────────────

// TestSmxRoundSummaryVerbose 是 S2：详细档下总结段照裁判轮的详细格式打，标签「总结轮」。
//
// 【判别力】lines[j:] 逐行：label 写错（「裁判轮」「总结」）时「总结轮结算」红；总结段不打预算表 / 扇出期间时红。
// 同时是详细档下各条缺席断言的正对照：同为详细档、同样配了 smx_s、同一条命令形状，这里总结照跑 ——
// 「总结轮结算」恰好一次、smx_s 被 Join 且激活、落了一条只给自己的消息；S3 single_verbose / no_summary_verbose、
// S7 left_verbose / all_fail_verbose、S10 canceled_verbose 的缺席断言因此能为假。
func TestSmxRoundSummaryVerbose(t *testing.T) {
	g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, false, true)
	g.submit(t, "/round smx_a smx_b --go")
	got := smxNormalize(g.buf.String())
	lines := smxLines(got)
	i := -1
	for k, l := range lines {
		if l == "发言轮结算" {
			i = k
		}
	}
	if i < 0 {
		t.Fatalf("前提不成立：没有整行「发言轮结算」；输出:\n%s", got)
	}
	j := -1
	for k := i; k < len(lines); k++ {
		if lines[k] == "成员  输入token  金额  冷启动  在飞  命令行余量  依据" {
			j = k
			break
		}
	}
	if j < 0 {
		t.Fatalf("发言轮结算之后没有总结段的预算表头；输出:\n%s", got)
	}
	want := []string{
		"成员  输入token  金额  冷启动  在飞  命令行余量  依据",
		"smx_s  未标定  未标定  预报冷  不在飞  H  无样本",
		"合计输入token 0（只含已标定的行）",
		"另有 1 名未标定",
		"扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。",
		"本轮问 1 个: smx_s",
		"outcome spoke  agent smx_s",
		"新开了一条 provider 会话 2a90d449-a348-497a-8812-e73e74d6898e",
		"seq 12  smx_s  claude/claude-fable-5-1/high",
		"OK",
		"用量 in=47751 cache_create=47749 cache_read=0 out=4 thinking=0",
		"金额 0.9551999999999999 USD",
		"冷暖 预报=冷 实测=冷  wall_ms=W  prompt_bytes=59  scanned_to=9",
		"总结轮结算",
		"确知起进程 1",
		"分不清进程是否起来 0",
		"用量 输入 47751  输出 4  cache_read 0  cache_create 47749  thinking 0",
		"金额 0.9551999999999999 USD",
		"冷启动 预报 1  实测 1",
	}
	if !reflect.DeepEqual(lines[j:], want) {
		t.Fatalf("详细档总结段逐行不对\n实际: %q\n期望: %q", lines[j:], want)
	}
	if strings.Contains(got, "总的来看") || strings.Contains(got, "总用时") {
		t.Fatalf("详细档不应当有「总的来看」或「总用时」；输出:\n%s", got)
	}
	if n := smxCount(lines, "总结轮结算"); n != 1 {
		t.Fatalf("整行「总结轮结算」应当恰好 1 次，实际 %d", n)
	}
	smxClockIs(t, g, 1)
	g.requireSummaryMsg(t)
}

// ─── S3 ──────────────────────────────────────────────────────────────

// TestSmxRoundSummaryTriggers 是 S3：什么时候总结（判据第 1、2、4 条与 --with-judge 的组合）。
//
// 【判别力】unconfigured / no_summary / single / preview / ask 与 S1 各只差一处（配置、flag、人数、--go、命令）；
// unconfigured_no_summary 杀「--no-summary 只在配了时才生效」；with_judge 钉「裁判算进人数」与「总结在裁判轮之后」。
// with_judge_no_summary 是 withJudge 与 noSummary 同时为真的唯一夹具，与 with_judge 只差 --no-summary：
// 杀「--no-summary 连裁判轮一起跳过」（裁判两行没了）、「带裁判时 --no-summary 失效」（多出总结两行、smx_s 被 Join）、
// 「--no-summary 早退挪到裁判块之前」（裁判两行没了）。summary 配成 smx_s 而不是空串：没配时「smx_s 不在表里」用不上。
// single_verbose / no_summary_verbose 与各自的简洁档原型只差档位，是判据第 2 条、第 1 条在详细档的唯一夹具：
// 判据只在简洁档生效时详细档多出整段「总结轮」、smx_s 被 Join，红（都是花钱方向）。
// 缺席的「没有总结轮结算」「smx_s 不在表里」的正对照是 S2。single_verbose 的末行与总结段结算块末行相同，所以整串相等。
func TestSmxRoundSummaryTriggers(t *testing.T) {
	const singleVerbose = "发言顺序按字典序，重复的 id 已去重，只问一次。\n" +
		"成员  输入token  金额  冷启动  在飞  命令行余量  依据\n" +
		"smx_a  未标定  未标定  预报冷  不在飞  H  无样本\n" +
		"合计输入token 0（只含已标定的行）\n" +
		"另有 1 名未标定\n" +
		"扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。\n" +
		"本轮问 1 个: smx_a\n" +
		"outcome spoke  agent smx_a\n" +
		"新开了一条 provider 会话 2a90d449-a348-497a-8812-e73e74d6898e\n" +
		"seq 4  smx_a  claude/claude-fable-5-1/high\n" +
		"OK\n" +
		"用量 in=47751 cache_create=47749 cache_read=0 out=4 thinking=0\n" +
		"金额 0.9551999999999999 USD\n" +
		"冷暖 预报=冷 实测=冷  wall_ms=W  prompt_bytes=19  scanned_to=1\n" +
		"发言轮结算\n" +
		"确知起进程 1\n" +
		"分不清进程是否起来 0\n" +
		"用量 输入 47751  输出 4  cache_read 0  cache_create 47749  thinking 0\n" +
		"金额 0.9551999999999999 USD\n" +
		"冷启动 预报 1  实测 1\n"
	const twoNoSummary = "本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n总用时 7.2s\n"
	cases := []struct {
		name     string
		summary  string
		members  []string
		concise  bool
		slowA    bool
		cmd      string
		want     string
		clock    int
		inTable  bool
		guardBoA bool
	}{
		{"unconfigured", "", []string{"smx_a", "smx_b"}, true, true, "/round smx_a smx_b --go",
			"本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n" + smxHint + "\n总用时 7.2s\n", 2, false, true},
		{"unconfigured_no_summary", "", []string{"smx_a", "smx_b"}, true, true, "/round smx_a smx_b --go --no-summary", twoNoSummary, 2, false, true},
		{"no_summary", "smx_s", []string{"smx_a", "smx_b"}, true, true, "/round smx_a smx_b --go --no-summary", twoNoSummary, 2, false, true},
		{"single", "smx_s", []string{"smx_a", "smx_b"}, true, false, "/round smx_a --go", "本轮问 1 个: smx_a\n甲（T）: OK\n总用时 7.2s\n", 2, false, false},
		{"preview", "smx_s", []string{"smx_a", "smx_b"}, true, false, "/round smx_a smx_b",
			"成员  输入token  金额  冷启动  在飞  命令行余量  依据\nsmx_a  未标定  未标定  预报冷  不在飞  H  无样本\nsmx_b  未标定  未标定  预报冷  不在飞  H  无样本\n合计输入token 0（只含已标定的行）\n另有 2 名未标定\n", 1, false, false},
		{"with_judge", "smx_s", []string{"smx_a", "smx_j"}, true, false, "/round smx_a smx_j --go --with-judge",
			"本轮问 1 个: smx_a\n甲（T）: OK\n本轮问 1 个: smx_j\n裁判 J（T）: OK\n本轮问 1 个: smx_s\n" + smxSumHeadOK + "\n总用时 7.2s\n", 2, true, false},
		{"ask", "smx_s", []string{"smx_a", "smx_b"}, true, false, "/ask smx_a", "正在问 smx_a ...\n甲（T）: OK\n", 0, false, false},
		{"with_judge_no_summary", "smx_s", []string{"smx_a", "smx_j"}, true, false, "/round smx_a smx_j --go --with-judge --no-summary",
			"本轮问 1 个: smx_a\n甲（T）: OK\n本轮问 1 个: smx_j\n裁判 J（T）: OK\n总用时 7.2s\n", 2, false, false},
		{"single_verbose", "smx_s", []string{"smx_a", "smx_b"}, false, false, "/round smx_a --go", singleVerbose, 1, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := smxSetup(t, tc.summary, tc.members, tc.concise, tc.slowA)
			g.submit(t, tc.cmd)
			raw := g.buf.String()
			if tc.guardBoA {
				smxRequireBBeforeA(t, raw)
			}
			smxEq(t, tc.cmd, smxNormalize(raw), tc.want)
			smxClockIs(t, g, tc.clock)
			if _, ok := g.member(t, "smx_s"); ok != tc.inTable {
				t.Fatalf("smx_s 在成员表里应当是 %v，实际 %v", tc.inTable, ok)
			}
		})
	}

	t.Run("no_summary_verbose", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, false, true)
		g.submit(t, "/round smx_a smx_b --go --no-summary")
		lines := smxLines(smxNormalize(g.buf.String()))
		i := -1
		for k, l := range lines {
			if l == "发言轮结算" {
				i = k
			}
		}
		if i < 0 {
			t.Fatalf("前提不成立：没有整行「发言轮结算」；输出:\n%s", g.buf.String())
		}
		want := []string{
			"发言轮结算",
			"确知起进程 2",
			"分不清进程是否起来 0",
			"用量 输入 95502  输出 8  cache_read 0  cache_create 95498  thinking 0",
			"金额 0.9551999999999999 USD",
			"冷启动 预报 1  实测 2",
			"意外冷启动 smx_a",
		}
		if !reflect.DeepEqual(lines[i:], want) {
			t.Fatalf("从最后一个「发言轮结算」起逐行不对\n实际: %q\n期望: %q", lines[i:], want)
		}
		if n := smxCount(lines, "总结轮结算"); n != 0 {
			t.Fatalf("--no-summary 下不应当有「总结轮结算」，实际 %d 次", n)
		}
		smxClockIs(t, g, 1)
		g.requireNotMember(t, "smx_s")
	})
}

// ─── S4 ──────────────────────────────────────────────────────────────

// TestSmxRoundSummaryVerboseUnconfigured 是 S4：详细档没配时一行都不多（提示行只在简洁档）。
//
// 【判别力】详细档也打提示行（if true）时最后一行变成提示行、整串含「总的来看」，红。对照是 S3 unconfigured（只差档位）。
func TestSmxRoundSummaryVerboseUnconfigured(t *testing.T) {
	g := smxSetup(t, "", []string{"smx_a", "smx_b"}, false, true)
	g.submit(t, "/round smx_a smx_b --go")
	got := smxNormalize(g.buf.String())
	lines := smxLines(got)
	if len(lines) == 0 || lines[len(lines)-1] != smxVerboseTail {
		t.Fatalf("最后一行应当是 %q；输出:\n%s", smxVerboseTail, got)
	}
	if strings.Contains(got, "总的来看") || strings.Contains(got, "总结轮") {
		t.Fatalf("详细档没配时不应当有「总的来看」或「总结轮」；输出:\n%s", got)
	}
}

// ─── S5 / S6 ─────────────────────────────────────────────────────────

// TestSmxRoundSummaryRounds2 是 S5：--rounds N 只在最后总结一次。
//
// 【判别力】每一轮都总结时「本轮问 1 个: smx_s」出现 2 次、行数变成 11，红；总结放在总用时之后时倒数两行互换，红。
// 第二轮两个成员都带 --resume，先后不定，所以不断言整串。
func TestSmxRoundSummaryRounds2(t *testing.T) {
	g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, true, false)
	g.submit(t, "/round smx_a smx_b --go --rounds 2")
	got := smxNormalize(g.buf.String())
	lines := smxLines(got)
	if len(lines) != 9 {
		t.Fatalf("应当恰好 9 行，实际 %d 行:\n%s", len(lines), got)
	}
	if n := smxCount(lines, "本轮问 2 个: smx_a, smx_b"); n != 2 {
		t.Fatalf("整行「本轮问 2 个: smx_a, smx_b」应当恰好 2 次，实际 %d:\n%s", n, got)
	}
	if n := smxCount(lines, "本轮问 1 个: smx_s"); n != 1 || lines[6] != "本轮问 1 个: smx_s" {
		t.Fatalf("整行「本轮问 1 个: smx_s」应当恰好 1 次且在倒数第 3 行:\n%s", got)
	}
	if lines[7] != smxSumHeadOK || lines[8] != "总用时 7.2s" {
		t.Fatalf("最后两行应当是总结头与总用时:\n%s", got)
	}
}

// TestSmxRoundSingleMemberTwoRounds 是 S6：同一个成员被问两次（两条记录）仍是「单成员」，不总结。
//
// 【判别力】人数若按记录条数算，两条记录够 2，会打出「没有总结（这条 /round 没有成员回复）」并多一行，红；
// smx_s 不在表里同时钉「没 Join」。
func TestSmxRoundSingleMemberTwoRounds(t *testing.T) {
	g := smxSetup(t, "smx_s", []string{"smx_a", "smx_x"}, true, false)
	g.submit(t, "/round smx_x --go --rounds 2")
	got := smxNormalize(g.buf.String())
	lines := smxLines(got)
	if len(lines) != 9 {
		t.Fatalf("应当恰好 9 行，实际 %d 行:\n%s", len(lines), got)
	}
	for _, k := range []int{0, 4} {
		if lines[k] != "本轮问 1 个: smx_x" {
			t.Fatalf("第 %d 行不对: %q\n%s", k, lines[k], got)
		}
		if lines[k+1] != "坏 X: 没有回复（没起来）" {
			t.Fatalf("第 %d 行不对: %q\n%s", k+1, lines[k+1], got)
		}
		if !strings.HasPrefix(lines[k+2], "/round smx_x: ") {
			t.Fatalf("前提不成立：第 %d 行应当以「/round smx_x: 」开头: %q", k+2, lines[k+2])
		}
		if lines[k+3] != smxSpawnLine {
			t.Fatalf("第 %d 行不对: %q\n%s", k+3, lines[k+3], got)
		}
	}
	if lines[8] != "总用时 7.2s" {
		t.Fatalf("第 8 行不对: %q\n%s", lines[8], got)
	}
	g.requireNotMember(t, "smx_s")
}

// ─── S7 ──────────────────────────────────────────────────────────────

// TestSmxRoundSummaryNotesAndFailures 是 S7：注明行、失败头、没人回复、已退出，以及它们在详细档。
//
// 【判别力】spawnfail_note / note_multi 钉注明行的显示名、结局词、顿号分隔、顺序、「只列没回复的」；
// all_fail 钉没人回复时不 Join、不起进程；summary_fails 钉失败头与原因行；left 钉已退出不拉回。
// 三个详细档子测试与各自的简洁档原型只差档位：已退出行、「没有成员回复」行、注明行只在简洁档打时红
// （left_verbose 红在末行：详细档不认已退出时总结照起、被拒成 post_rejected，末行变成结算块）；
// left_verbose 的「smx_s 仍未激活」「没有 smx_s 的消息」与 all_fail_verbose 的「smx_s 不在表里」守的是
// 「真拉回 / 真 Join」一类写法，正对照是 S2（那里总结照跑、smx_s 被 Join、落了消息）。
// note_refused 是注明行走「拒绝」结局与「显示名为空」的唯一夹具（只认 SpawnFailed、或不回落到 id 时红）；
// rounds2_last_record 是「前面说过话、最后一次没回复」的唯一夹具（取第一条记录时没有注明行，红）；
// note_order 钉跨段顺序（发言轮在前、裁判在后；反过来时顺序颠倒，红）。
func TestSmxRoundSummaryNotesAndFailures(t *testing.T) {
	t.Run("spawnfail_note", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_x"}, true, false)
		g.submit(t, "/round smx_a smx_x --go")
		got := smxNormalize(g.buf.String())
		lines := smxLines(got)
		if len(lines) != 9 {
			t.Fatalf("应当恰好 9 行，实际 %d 行:\n%s", len(lines), got)
		}
		if lines[0] != "本轮问 2 个: smx_a, smx_x" {
			t.Fatalf("第 0 行不对: %q", lines[0])
		}
		heads, fail := 0, -1
		for i := 1; i <= 3; i++ {
			if lines[i] == "甲（T）: OK" {
				heads++
			}
			if lines[i] == "坏 X: 没有回复（没起来）" {
				if fail >= 0 {
					t.Fatalf("失败行出现不止一次:\n%s", got)
				}
				fail = i
			}
		}
		if heads != 1 || fail < 0 || fail == 3 {
			t.Fatalf("第 1-3 行应当恰好一行「甲（T）: OK」、恰好一行失败行且不在第 3 行:\n%s", got)
		}
		if !strings.HasPrefix(lines[fail+1], "/round smx_x: ") {
			t.Fatalf("前提不成立：失败行之后应当紧跟以「/round smx_x: 」开头的原因行:\n%s", got)
		}
		want := []string{smxSpawnLine, smxNoteX, "本轮问 1 个: smx_s", smxSumHeadOK, "总用时 7.2s"}
		if !reflect.DeepEqual(lines[4:], want) {
			t.Fatalf("第 4-8 行不对\n实际: %q\n期望: %q", lines[4:], want)
		}
	})

	t.Run("note_multi", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_x", "smx_y"}, true, false)
		g.submit(t, "/round smx_a smx_x smx_y --go")
		got := smxNormalize(g.buf.String())
		lines := smxLines(got)
		if len(lines) < 4 {
			t.Fatalf("行数太少:\n%s", got)
		}
		if l := lines[len(lines)-4]; l != "这些成员最后一次没有回复，总结看不到他们这一次的话：坏 X（没起来）、坏 Y（没起来）" {
			t.Fatalf("倒数第 4 行应当是两人的注明行，实际 %q:\n%s", l, got)
		}
		if l := lines[len(lines)-2]; l != smxSumHeadOK {
			t.Fatalf("倒数第 2 行应当是总结头，实际 %q:\n%s", l, got)
		}
	})

	t.Run("all_fail", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_x", "smx_y"}, true, false)
		g.submit(t, "/round smx_x smx_y --go")
		got := smxNormalize(g.buf.String())
		lines := smxLines(got)
		if len(lines) != 8 || lines[5] != smxSpawn2Line || lines[6] != smxNoReply || lines[7] != "总用时 7.2s" {
			t.Fatalf("应当恰好 8 行、第 5-7 行依次是结算行、没有成员回复、总用时:\n%s", got)
		}
		g.requireNotMember(t, "smx_s")
	})

	t.Run("summary_fails", func(t *testing.T) {
		g := smxSetup(t, "smx_z", []string{"smx_a", "smx_b"}, true, true)
		g.submit(t, "/round smx_a smx_b --go")
		got := smxNormalize(g.buf.String())
		lines := smxLines(got)
		if len(lines) != 8 {
			t.Fatalf("应当恰好 8 行，实际 %d 行:\n%s", len(lines), got)
		}
		if !strings.HasPrefix(lines[5], "/round smx_z: ") {
			t.Fatalf("前提不成立：第 5 行应当以「/round smx_z: 」开头，实际 %q", lines[5])
		}
		want := []string{"本轮问 2 个: smx_a, smx_b", "smx_b（T）: OK", "甲（T）: OK", "本轮问 1 个: smx_z", "总的来看：没有总结（没起来）", lines[5], smxSpawnLine, "总用时 7.2s"}
		if !reflect.DeepEqual(lines, want) {
			t.Fatalf("逐行不对\n实际: %q\n期望: %q", lines, want)
		}
	})

	t.Run("left", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b", "smx_s"}, true, true)
		g.leave(t)
		g.submit(t, "/round smx_a smx_b --go")
		smxEq(t, "已退出", smxNormalize(g.buf.String()), "本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n"+smxLeftLine+"\n总用时 7.2s\n")
		g.requireLeftInactive(t)
		g.requireNoMsgs(t, "smx_s")
	})

	t.Run("left_verbose", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b", "smx_s"}, false, true)
		g.leave(t)
		g.submit(t, "/round smx_a smx_b --go")
		lines := smxLines(smxNormalize(g.buf.String()))
		if len(lines) == 0 || lines[len(lines)-1] != smxLeftLine {
			t.Fatalf("详细档最后一行应当是已退出行；输出:\n%s", g.buf.String())
		}
		g.requireLeftInactive(t)
		g.requireNoMsgs(t, "smx_s")
	})

	t.Run("all_fail_verbose", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_x", "smx_y"}, false, false)
		g.submit(t, "/round smx_x smx_y --go")
		lines := smxLines(smxNormalize(g.buf.String()))
		if len(lines) == 0 || lines[len(lines)-1] != smxNoReply {
			t.Fatalf("详细档最后一行应当是「没有成员回复」行；输出:\n%s", g.buf.String())
		}
		g.requireNotMember(t, "smx_s")
	})

	t.Run("note_verbose", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_x"}, false, false)
		g.submit(t, "/round smx_a smx_x --go")
		lines := smxLines(smxNormalize(g.buf.String()))
		if n := smxCount(lines, smxNoteX); n != 1 {
			t.Fatalf("详细档整行注明行应当恰好 1 次，实际 %d；输出:\n%s", n, g.buf.String())
		}
	})

	t.Run("note_refused", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, true, true)
		if err := smxSeedStale(g.ctx, g.svc, g.convID); err != nil {
			t.Fatalf("前置：写在飞记录失败: %v", err)
		}
		g.submit(t, "/round smx_a smx_b --go")
		want := "本轮问 2 个: smx_a, smx_b\n" +
			"smx_b: 没有回复（拒绝）\n" +
			"/round smx_b: " + smxInflight + "\n" +
			"甲（T）: OK\n" +
			"这些成员最后一次没有回复，总结看不到他们这一次的话：smx_b（拒绝）\n" +
			"本轮问 1 个: smx_s\n" +
			smxSumHeadOK + "\n" +
			"总用时 7.2s\n"
		smxEq(t, "注明行走「拒绝」", smxNormalize(g.buf.String()), want)
		smxClockIs(t, g, 2)
	})

	t.Run("rounds2_last_record", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, true, true)
		w := g.installHook("甲（", func() error { return smxSeedStale(context.Background(), g.svc, g.convID) })
		g.submit(t, "/round smx_a smx_b --go --rounds 2")
		if fired, _, err := w.state(); !fired || err != nil {
			t.Fatalf("前提不成立：钩子应当触发且种记录成功，实际 触发=%v 错误=%v；输出:\n%s", fired, err, w.String())
		}
		want := "本轮问 2 个: smx_a, smx_b\n" +
			"smx_b（T）: OK\n" +
			"甲（T）: OK\n" +
			"本轮问 2 个: smx_a, smx_b\n" +
			"smx_b: 没有回复（拒绝）\n" +
			"/round smx_b: " + smxInflight + "\n" +
			"甲（T）: OK\n" +
			"这些成员最后一次没有回复，总结看不到他们这一次的话：smx_b（拒绝）\n" +
			"本轮问 1 个: smx_s\n" +
			smxSumHeadOK + "\n" +
			"总用时 7.2s\n"
		smxEq(t, "最后一条记录", smxNormalize(w.String()), want)
		smxClockIs(t, g, 2)
	})

	t.Run("note_order", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_x", "smx_k"}, true, true)
		g.submit(t, "/round smx_a smx_x smx_k --go --with-judge")
		raw := g.buf.String()
		n := 0
		for _, l := range strings.Split(raw, "\n") {
			if strings.HasPrefix(l, "/round smx_x: ") || strings.HasPrefix(l, "/round smx_k: ") {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("前提不成立：原因行应当恰好 2 行，实际 %d；原始输出:\n%s", n, raw)
		}
		want := "本轮问 2 个: smx_a, smx_x\n" +
			"坏 X: 没有回复（没起来）\n" +
			"/round smx_x: <原因>\n" +
			"甲（T）: OK\n" +
			smxSpawnLine + "\n" +
			"本轮问 1 个: smx_k\n" +
			"坏 K: 没有回复（没起来）\n" +
			"/round smx_k: <原因>\n" +
			smxSpawnLine + "\n" +
			"这些成员最后一次没有回复，总结看不到他们这一次的话：坏 X（没起来）、坏 K（没起来）\n" +
			"本轮问 1 个: smx_s\n" +
			smxSumHeadOK + "\n" +
			"总用时 7.2s\n"
		smxEq(t, "跨段顺序", smxNormalize(smxCauseRe.ReplaceAllString(raw, "${1}<原因>")), want)
		smxClockIs(t, g, 2)
	})
}

// ─── S8 / S9 ─────────────────────────────────────────────────────────

// TestSmxRoundRefusesSummaryMember 是 S8：总结成员出现在 /round 名单里时整条命令拒绝。
//
// 【判别力】给名单、预览、mention 三条取名单的路都拒，简洁档与详细档各一个：拒绝只在简洁档生效时三个详细档红；
// 只在 --go 时生效时两个 preview 红；详细档 mention 推出来的名单不拒时 mention_verbose 红（那是花钱方向：
// 总结成员以发言者身份起进程，它的话别人看不到）。control_unconfigured 证明拒绝来自配置而不是别的
// （同一名单、没配时照常开跑）。
func TestSmxRoundRefusesSummaryMember(t *testing.T) {
	cases := []struct {
		name    string
		concise bool
		mention bool
		cmd     string
	}{
		{"named", true, false, "/round smx_a smx_s --go"},
		{"preview", true, false, "/round smx_a smx_s"},
		{"mention", true, true, "/round --go"},
		{"named_verbose", false, false, "/round smx_a smx_s --go"},
		{"preview_verbose", false, false, "/round smx_a smx_s"},
		{"mention_verbose", false, true, "/round --go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := smxSetup(t, "smx_s", []string{"smx_a", "smx_s"}, tc.concise, false)
			if tc.mention {
				g.submit(t, "看看 @smx_a @smx_s")
				g.buf.Reset()
			}
			g.submit(t, tc.cmd)
			smxEq(t, "拒绝", g.buf.String(), smxRefuse)
			smxClockIs(t, g, 1)
			g.requireNoMsgs(t, "smx_s")
		})
	}
	t.Run("control_unconfigured", func(t *testing.T) {
		g := smxSetup(t, "", []string{"smx_a", "smx_s"}, true, false)
		g.submit(t, "/round smx_a smx_s --go --no-summary")
		if got := g.buf.String(); !strings.HasPrefix(got, "本轮问 2 个: smx_a, smx_s\n") {
			t.Fatalf("没配时同一名单应当照常开跑，实际输出:\n%s", got)
		}
	})
}

// TestSmxAskSummaryMemberPrivate 是 S9：/ask 总结成员也只给用户看，且走成员头格式。
//
// 【判别力】私密按配置判、不按调用路径判：只在 /round 的总结段填 Recipients 时这里的 Recipients 为空，红；
// /ask 用了总结头时整串红。
func TestSmxAskSummaryMemberPrivate(t *testing.T) {
	g := smxSetup(t, "smx_s", []string{"smx_a", "smx_s"}, true, false)
	g.submit(t, "/ask smx_s 你怎么看")
	smxEq(t, "/ask 总结成员", smxNormalize(g.buf.String()), "正在问 smx_s ...\n总结 S（T）: OK\n")
	g.requireSummaryMsg(t)
}

// ─── S10 ─────────────────────────────────────────────────────────────

// TestSmxRoundCanceledNoSummary 是 S10：最后一个成员回来时取消，这一段照常收完，总结不跑（也不 Join）。
//
// 【判别力】concise 的对照是 S1（只差取消）：不看 ctx 时多出总结两行、smx_s 被 Join，红。
// canceled_verbose 是判据第 3 条在详细档的唯一夹具：判据只在简洁档生效时，详细档取消之后照走后面几条，
// 读成员表时 ctx 已取消，末行变成 round: list participants …: context canceled，红；对照是 S2（那里有总结轮结算、smx_s 被 Join）。
func TestSmxRoundCanceledNoSummary(t *testing.T) {
	t.Run("concise", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, true, true)
		ctx, cancel := context.WithCancel(g.ctx)
		defer cancel()
		w := g.installHook("甲（", func() error { cancel(); return nil })
		if err := g.r.Submit(ctx, "/round smx_a smx_b --go"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		requireFired(t, w, 1)
		smxEq(t, "取消", smxNormalize(w.String()), smxS10Want)
		g.requireNotMember(t, "smx_s")
	})
	t.Run("canceled_verbose", func(t *testing.T) {
		g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, false, true)
		ctx, cancel := context.WithCancel(g.ctx)
		defer cancel()
		w := g.installHook("outcome spoke  agent smx_a", func() error { cancel(); return nil })
		if err := g.r.Submit(ctx, "/round smx_a smx_b --go"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		requireFired(t, w, 1)
		lines := smxLines(smxNormalize(w.String()))
		if len(lines) == 0 || lines[len(lines)-1] != smxVerboseTail {
			t.Fatalf("详细档取消后最后一行应当是 %q；输出:\n%s", smxVerboseTail, w.String())
		}
		if n := smxCount(lines, "总结轮结算"); n != 0 {
			t.Fatalf("取消后不应当有「总结轮结算」，实际 %d 次", n)
		}
		g.requireNotMember(t, "smx_s")
	})
}

// ─── S11 ─────────────────────────────────────────────────────────────

// TestSmxRoundHelpLine 是 S11：/help 里 round 那一行的 summary 逐字。
//
// 【判别力】命令表 round 的 summary 改回 HEAD 原文或漏写 --no-summary 时红。
func TestSmxRoundHelpLine(t *testing.T) {
	var b strings.Builder
	r := New(smxCfg("smx_s"), nil, nil, nil, &b, RulesContext{})
	if err := r.Submit(context.Background(), "/help"); err != nil {
		t.Fatalf("/help 返回错误: %v", err)
	}
	const want = "按名单问一轮，--go 时发言者同时起进程；多个成员跑完自动总结（只给你看）: /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]"
	found := 0
	for _, l := range strings.Split(b.String(), "\n") {
		if strings.HasPrefix(l, "/round ") {
			found++
			if got := strings.TrimSpace(strings.TrimPrefix(l, "/round")); got != want {
				t.Fatalf("/help 里 /round 的 summary 不对\n实际: %q\n期望: %q", got, want)
			}
		}
	}
	if found != 1 {
		t.Fatalf("/help 里以「/round 」开头的行应当恰好 1 行，实际 %d:\n%s", found, b.String())
	}
}

// ─── S12 / S14 用的记录型观察者 ───────────────────────────────────────

// smxRecObs 带锁；RoundStart 记 RS 与事件，MemberDone 记 MD:Label:AgentID，RoundEnd 记 RE 与事件。
// onEnd 可选：RoundEnd 记下之后、放锁之后调它（S12 不设；S14 的取消用它）。
type smxRecObs struct {
	mu    sync.Mutex
	seq   []string
	rs    []RoundStartEvent
	re    []RoundEndEvent
	onEnd func()
}

func (o *smxRecObs) RoundStart(ev RoundStartEvent) {
	o.mu.Lock()
	o.seq = append(o.seq, "RS")
	o.rs = append(o.rs, ev)
	o.mu.Unlock()
}

func (o *smxRecObs) MemberDone(ev MemberDoneEvent) {
	o.mu.Lock()
	o.seq = append(o.seq, "MD:"+ev.Label+":"+ev.AgentID)
	o.mu.Unlock()
}

func (o *smxRecObs) RoundEnd(ev RoundEndEvent) {
	o.mu.Lock()
	o.seq = append(o.seq, "RE")
	o.re = append(o.re, ev)
	o.mu.Unlock()
	if o.onEnd != nil {
		o.onEnd()
	}
}

func (o *smxRecObs) snapshot() ([]string, []RoundStartEvent, []RoundEndEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.seq...), append([]RoundStartEvent(nil), o.rs...), append([]RoundEndEvent(nil), o.re...)
}

// TestSmxRoundSummaryObserverEvents 是 S12：TUI 看板靠的三种事件在总结段照发，label 冻结。
//
// 【判别力】总结段不发事件、label 写错（「裁判轮」「总结」）、K / N 不是 0、RoundEnd 不带 Concise 时各自红；
// 装了观察者时屏幕不打总用时（CO1），总结照打。它也是 S14 各缺席断言的正对照（同一个观察者、总结照跑）。
func TestSmxRoundSummaryObserverEvents(t *testing.T) {
	g := smxSetup(t, "smx_s", []string{"smx_a", "smx_b"}, true, true)
	obs := &smxRecObs{}
	g.r.SetRoundObserver(obs)
	g.submit(t, "/round smx_a smx_b --go")
	raw := g.buf.String()
	smxRequireBBeforeA(t, raw)
	smxEq(t, "装了观察者", smxNormalize(raw), strings.TrimSuffix(smxS1Want, "总用时 7.2s\n"))
	seq, rs, re := obs.snapshot()
	wantSeq := []string{"RS", "MD:发言轮:smx_b", "MD:发言轮:smx_a", "RE", "RS", "MD:总结轮:smx_s", "RE"}
	if !reflect.DeepEqual(seq, wantSeq) {
		t.Fatalf("事件序列不对\n实际: %q\n期望: %q", seq, wantSeq)
	}
	s := rs[1]
	if s.Label != "总结轮" || s.K != 0 || s.N != 0 || s.Members != 1 || !reflect.DeepEqual(s.Speakers, []string{"smx_s"}) {
		t.Fatalf("第二个 RoundStart 不对: %+v", s)
	}
	e := re[1]
	if e.Label != "总结轮" || !e.Concise || len(e.Results) != 1 || e.Results[0].Outcome != turn.OutcomeSpoke {
		t.Fatalf("第二个 RoundEnd 不对: %+v", e)
	}
}

// ─── S13 ─────────────────────────────────────────────────────────────

// TestSmxRoundSummaryJudgmentOrder 是 S13：§1.0 判据的顺序，每个子测试同时偏离两个条件。
//
// 【判别力】前三条钉「没配」（第 4 条）与第 2、3、5 条的先后：没配挪到人数之前 → single_unconfigured 多一行提示；
// 挪到取消之前 → canceled_unconfigured 多一行提示；没回复挪到没配之前 → all_fail_unconfigured 第 6 行变成「没有成员回复」。
// 后三条钉「已退出」（第 6 条）与第 2、3、5 条的先后：挪到人数之前 → left_single 多一行已退出行；
// 挪到没回复之前 → left_all_fail 第 6 行变成已退出行；挪到取消之前 → left_canceled 红（真 store 在 ctx 已取消时
// 读成员表报错，多出的是 round: list participants …: context canceled 那一行）。
// 「库里没有 smx_s 的消息」是缺席断言，正对照是 S1（同为简洁档、配了 smx_s、两人都回复、没取消，那里恰好 1 条）。
func TestSmxRoundSummaryJudgmentOrder(t *testing.T) {
	eight := func(sixth string) func(t *testing.T, got string) {
		return func(t *testing.T, got string) {
			t.Helper()
			lines := smxLines(got)
			if len(lines) != 8 || lines[5] != smxSpawn2Line || lines[6] != sixth || lines[7] != "总用时 7.2s" {
				t.Fatalf("应当恰好 8 行、第 5-7 行依次是结算行、%q、总用时:\n%s", sixth, got)
			}
		}
	}
	exact := func(want string) func(t *testing.T, got string) {
		return func(t *testing.T, got string) {
			t.Helper()
			smxEq(t, "判据顺序", got, want)
		}
	}
	const single = "本轮问 1 个: smx_a\n甲（T）: OK\n总用时 7.2s\n"
	cases := []struct {
		name    string
		summary string
		members []string
		slowA   bool
		leave   bool
		cancel  bool
		cmd     string
		check   func(t *testing.T, got string)
	}{
		{"single_unconfigured", "", []string{"smx_a", "smx_b"}, false, false, false, "/round smx_a --go", exact(single)},
		{"canceled_unconfigured", "", []string{"smx_a", "smx_b"}, true, false, true, "/round smx_a smx_b --go", exact(smxS10Want)},
		{"all_fail_unconfigured", "", []string{"smx_x", "smx_y"}, false, false, false, "/round smx_x smx_y --go", eight(smxHint)},
		{"left_single", "smx_s", []string{"smx_a", "smx_b", "smx_s"}, true, true, false, "/round smx_a --go", exact(single)},
		{"left_canceled", "smx_s", []string{"smx_a", "smx_b", "smx_s"}, true, true, true, "/round smx_a smx_b --go", exact(smxS10Want)},
		{"left_all_fail", "smx_s", []string{"smx_x", "smx_y", "smx_s"}, false, true, false, "/round smx_x smx_y --go", eight(smxNoReply)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := smxSetup(t, tc.summary, tc.members, true, tc.slowA)
			if tc.leave {
				g.leave(t)
			}
			ctx := g.ctx
			var w *smxHookWriter
			if tc.cancel {
				c, cancel := context.WithCancel(g.ctx)
				defer cancel()
				ctx = c
				w = g.installHook("甲（", func() error { cancel(); return nil })
			}
			if err := g.r.Submit(ctx, tc.cmd); err != nil {
				t.Fatalf("%s 返回错误: %v", tc.cmd, err)
			}
			out := g.buf.String()
			if w != nil {
				requireFired(t, w, 1)
				out = w.String()
			}
			tc.check(t, smxNormalize(out))
			smxClockIs(t, g, 2)
			g.requireNoMsgs(t, "smx_s")
		})
	}
}

// ─── S14 ─────────────────────────────────────────────────────────────

// TestSmxRoundSummaryObservedGates 是 S14：装了观察者（TUI 就是这样装的）时 §1.0 的判据照样生效。
//
// 【判别力】各子测试与 S12 只差一个条件（单成员、--no-summary、取消、已退出、名单含总结成员、没人回复、有人没回复）；
// S12 用同一个观察者证明总结段在这种装置下照跑（事件序列多出 RS, MD:总结轮:smx_s, RE、smx_s 被 Join、落了消息），
// 所以这里的缺席能为假。判据第 1、2、3、5、6 条、名单拒绝、注明行各加一个 && r.roundObs == nil 时，对应的子测试各自红。
// 装了观察者时 CO1 不打总用时，所以期望里都没有总用时。
func TestSmxRoundSummaryObservedGates(t *testing.T) {
	const two = "本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n"
	twoSeq := []string{"RS", "MD:发言轮:smx_b", "MD:发言轮:smx_a", "RE"}
	const (
		stNot = iota
		stLeft
		stSkip
	)
	cases := []struct {
		name    string
		members []string
		slowA   bool
		leave   bool
		cancel  bool
		cmd     string
		check   func(t *testing.T, got string)
		seq     []string
		state   int
		msgs    bool
	}{
		{"single_observed", []string{"smx_a", "smx_b"}, false, false, false, "/round smx_a --go",
			func(t *testing.T, got string) { smxEq(t, "单成员", got, "本轮问 1 个: smx_a\n甲（T）: OK\n") },
			[]string{"RS", "MD:发言轮:smx_a", "RE"}, stNot, false},
		{"no_summary_observed", []string{"smx_a", "smx_b"}, true, false, false, "/round smx_a smx_b --go --no-summary",
			func(t *testing.T, got string) { smxEq(t, "--no-summary", got, two) }, twoSeq, stNot, false},
		{"canceled_observed", []string{"smx_a", "smx_b"}, true, false, true, "/round smx_a smx_b --go",
			func(t *testing.T, got string) { smxEq(t, "取消", got, two) }, twoSeq, stNot, false},
		{"left_observed", []string{"smx_a", "smx_b", "smx_s"}, true, true, false, "/round smx_a smx_b --go",
			func(t *testing.T, got string) { smxEq(t, "已退出", got, two+smxLeftLine+"\n") }, twoSeq, stLeft, false},
		{"named_observed", []string{"smx_a", "smx_s"}, false, false, false, "/round smx_a smx_s --go",
			func(t *testing.T, got string) { smxEq(t, "名单拒绝", got, smxRefuse) }, nil, stSkip, false},
		{"all_fail_observed", []string{"smx_x", "smx_y"}, false, false, false, "/round smx_x smx_y --go",
			func(t *testing.T, got string) {
				lines := smxLines(got)
				if len(lines) != 7 || lines[5] != smxSpawn2Line || lines[6] != smxNoReply {
					t.Fatalf("应当恰好 7 行、第 5、6 行依次是结算行与「没有成员回复」:\n%s", got)
				}
			}, nil, stNot, false},
		{"note_observed", []string{"smx_a", "smx_x"}, false, false, false, "/round smx_a smx_x --go",
			func(t *testing.T, got string) {
				lines := smxLines(got)
				if n := smxCount(lines, smxNoteX); n != 1 || lines[len(lines)-1] != smxSumHeadOK {
					t.Fatalf("注明行应当恰好 1 次（实际 %d）、最后一行是总结头:\n%s", n, got)
				}
			}, nil, stSkip, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := smxSetup(t, "smx_s", tc.members, true, tc.slowA)
			if tc.leave {
				g.leave(t)
			}
			obs := &smxRecObs{}
			ctx := g.ctx
			if tc.cancel {
				c, cancel := context.WithCancel(g.ctx)
				defer cancel()
				ctx = c
				obs.onEnd = cancel
			}
			g.r.SetRoundObserver(obs)
			if err := g.r.Submit(ctx, tc.cmd); err != nil {
				t.Fatalf("%s 返回错误: %v", tc.cmd, err)
			}
			tc.check(t, smxNormalize(g.buf.String()))
			if tc.seq != nil {
				if seq, _, _ := obs.snapshot(); !reflect.DeepEqual(seq, tc.seq) {
					t.Fatalf("事件序列不对\n实际: %q\n期望: %q", seq, tc.seq)
				}
			}
			if !tc.msgs {
				g.requireNoMsgs(t, "smx_s")
			}
			switch tc.state {
			case stNot:
				g.requireNotMember(t, "smx_s")
			case stLeft:
				g.requireLeftInactive(t)
			}
		})
	}
}
