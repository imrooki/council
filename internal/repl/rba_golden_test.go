package repl

// 本文件覆盖冻结契约 RB-A v1.2（E:\multi_model_cowork\rb_a_contract.md）§2.1 的 R1，
// 并定义 §2.0 的 repl 包共用装置，rba_observer_test.go 共用它们。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3143bc8 的源码。
//
// 本文件**不引用本刀新增的任何标识符**（RoundObserver、SetRoundObserver 等一概不碰），
// 所以在 HEAD 上单独放进包里就能编译、而且必须绿：R1 的 golden 是 HEAD 代码的真输出。
// 记录型观察者与失败写者这两样要引用新标识符或只给 R2-R6 用，放在 rba_observer_test.go。
//
// 只读复用的既有辅助：newRoundStreamService（step10ux1_round_stream_test.go）与
// 本包 TestMain 的假 CLI（session_inflight_test.go，COUNCIL_FAKE_CLI=1 时扮演 claude；
// argv 带 --resume 时睡 700ms 再应答）。别的既有辅助（syncBuffer、runREPLInBackground 等）
// 不在契约的放行清单里，一概不用。

import (
	"context"
	"fmt"
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

// rbaBuf 是自带互斥锁的输出缓冲。/round 的逐条输出在 turn 的 worker goroutine 上写，
// 测试 goroutine（与观察者的快照）同时读，所以必须加锁。
type rbaBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *rbaBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *rbaBuf) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func (w *rbaBuf) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.b.Reset()
}

// rbaAgentIDs 是装置里三个 agent 的 id，也是 Join 的顺序（§2.0 冻结）。
var rbaAgentIDs = []string{"rba_a", "rba_b", "rba_j"}

// rbaConfig 造 §2.0 的配置：provider 是登记过的 claude，Command 是测试二进制自己；
// rba_a、rba_b 是讨论模式，rba_j 是裁判模式。
func rbaConfig() *config.Config {
	agent := func(mode config.Mode) config.AgentConfig {
		return config.AgentConfig{
			DisplayName: "X",
			Provider:    "claude",
			Model:       "claude-fable-5-1",
			Effort:      "high",
			Role:        "rba",
			Permissions: config.PermissionRead,
			Mode:        mode,
		}
	}
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: os.Args[0]},
		},
		Agents: map[string]config.AgentConfig{
			"rba_a": agent(config.ModeDiscussion),
			"rba_b": agent(config.ModeDiscussion),
			"rba_j": agent(config.ModeJudge),
		},
	}
}

// rbaRig 是 §2.0 装置里 REPL 之前的那一半：配置、库、Runner、一条带种子消息的会话。
// 拆成两半是因为 R4、R6c 要在 /use 之前对同一个 Runner 先 Ask 一次。
type rbaRig struct {
	ctx    context.Context
	cfg    *config.Config
	svc    *conversation.Service
	rn     *turn.Runner
	convID string
}

// rbaNewRig 按 §2.0 的顺序搭装置：Setenv、库、Runner、建会话、按 rba_a/rba_b/rba_j 加入、发种子消息。
func rbaNewRig(t *testing.T) *rbaRig {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	cfg := rbaConfig()
	svc := newRoundStreamService(t)
	rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})
	conv, err := svc.Create(ctx, "rba")
	if err != nil {
		t.Fatalf("装置：建会话失败: %v", err)
	}
	for _, id := range rbaAgentIDs {
		if err := svc.Join(ctx, conv.ID, conversation.Participant{
			AgentID: id, Provider: "claude", JoinedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("装置：加入 %s 失败: %v", id, err)
		}
	}
	if _, err := svc.Post(ctx, conversation.Message{
		ConvID: conv.ID, SenderID: conversation.ReservedSenderUser,
		Kind: conversation.KindUser, Text: "seed-msg",
	}); err != nil {
		t.Fatalf("装置：发种子消息失败: %v", err)
	}
	return &rbaRig{ctx: ctx, cfg: cfg, svc: svc, rn: rn, convID: conv.ID}
}

// newREPL 造 REPL、Submit 一次 /use、再清空缓冲。reset 由调用方给（两种缓冲各有各的清法）。
func (g *rbaRig) newREPL(t *testing.T, out io.Writer, reset func()) *REPL {
	t.Helper()
	r := New(g.cfg, g.svc, g.rn, nil, out, RulesContext{})
	if err := r.Submit(g.ctx, "/use "+g.convID); err != nil {
		t.Fatalf("装置：/use 失败: %v", err)
	}
	reset()
	return r
}

// rbaWallRe 与 rbaHeadroomRe 是 §3.4 的两条规范化规则，按这个顺序套用。
var (
	rbaWallRe     = regexp.MustCompile(`wall_ms=\d+`)
	rbaHeadroomRe = regexp.MustCompile(`(?m)^(rba_[a-z]  未标定  未标定  预报\S+  不在飞  )-?\d+(  无样本)$`)
)

// rbaNormalize 按 §3.4 规范化整个缓冲：墙钟换成 W，命令行余量换成 H。
func rbaNormalize(s string) string {
	s = rbaWallRe.ReplaceAllString(s, "wall_ms=W")
	return rbaHeadroomRe.ReplaceAllString(s, "${1}H${2}")
}

// rbaFirstDiff 给出两段文本第一处不同的行，失败信息用。
func rbaFirstDiff(got, want string) string {
	g := strings.Split(got, "\n")
	w := strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		} else {
			gl = "<缺>"
		}
		if i < len(w) {
			wl = w[i]
		} else {
			wl = "<缺>"
		}
		if gl != wl {
			return fmt.Sprintf("第 %d 行\n实际: %q\n期望: %q", i+1, gl, wl)
		}
	}
	return "逐行相同（差在末尾换行？）"
}

// rbaGolden 是 §3.4 的 golden，逐字节照抄（1806 字节，39 行）。
const rbaGolden = `发言顺序按字典序，重复的 id 已去重，只问一次。
成员  输入token  金额  冷启动  在飞  命令行余量  依据
rba_a  未标定  未标定  预报冷  不在飞  H  无样本
合计输入token 0（只含已标定的行）
另有 1 名未标定
扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。
本轮问 1 个: rba_a
outcome spoke  agent rba_a
新开了一条 provider 会话 2a90d449-a348-497a-8812-e73e74d6898e
seq 4  rba_a  claude/claude-fable-5-1/high
OK
用量 in=47751 cache_create=47749 cache_read=0 out=4 thinking=0
金额 0.9551999999999999 USD
冷暖 预报=冷 实测=冷  wall_ms=W  prompt_bytes=19  scanned_to=1
发言轮结算
确知起进程 1
分不清进程是否起来 0
用量 输入 47751  输出 4  cache_read 0  cache_create 47749  thinking 0
金额 0.9551999999999999 USD
冷启动 预报 1  实测 1
成员  输入token  金额  冷启动  在飞  命令行余量  依据
rba_j  未标定  未标定  预报冷  不在飞  H  无样本
合计输入token 0（只含已标定的行）
另有 1 名未标定
扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。
本轮问 1 个: rba_j
outcome spoke  agent rba_j
新开了一条 provider 会话 2a90d449-a348-497a-8812-e73e74d6898e
seq 8  rba_j  claude/claude-fable-5-1/high
OK
用量 in=47751 cache_create=47749 cache_read=0 out=4 thinking=0
金额 0.9551999999999999 USD
冷暖 预报=冷 实测=冷  wall_ms=W  prompt_bytes=30  scanned_to=5
裁判轮结算
确知起进程 1
分不清进程是否起来 0
用量 输入 47751  输出 4  cache_read 0  cache_create 47749  thinking 0
金额 0.9551999999999999 USD
冷启动 预报 1  实测 1
`

// rbaR1Command 是 R1 / R2 的被测命令。
const rbaR1Command = "/round rba_a rba_j --go --with-judge"

// rbaCheckGolden 把缓冲规范化后与 golden 整串比较。
func rbaCheckGolden(t *testing.T, raw string) {
	t.Helper()
	got := rbaNormalize(raw)
	if got != rbaGolden {
		t.Errorf("规范化后的输出与 §3.4 golden 不相等。%s\n完整规范化输出:\n%s", rbaFirstDiff(got, rbaGolden), got)
	}
}

// TestRbaR1NilObserverGolden：不装观察者，/round 的行式输出规范化后与 golden 整串相等。
//
// 【判别力】能为假：golden 逐字节钉住了从「发言顺序」到裁判轮结算的 39 行；发射点附近多打一行
// （变异 #18 的同类，在 nil 观察者时也打）、或 nil 观察者被解引用（变异 #17，panic）都红。
// 规范化只抹掉墙钟与命令行余量两个随环境变的数，别的一个字节都不放过。
// 对照：R2 同一装置、同一命令，只多装一个观察者。
// 本条在 HEAD 上必须绿（它证明 golden 是 HEAD 的真输出，不是写出来的期望）。
func TestRbaR1NilObserverGolden(t *testing.T) {
	g := rbaNewRig(t)
	buf := &rbaBuf{}
	r := g.newREPL(t, buf, buf.Reset)
	if err := r.Submit(g.ctx, rbaR1Command); err != nil {
		t.Fatalf("Submit(%q) 返回错误: %v；输出:\n%s", rbaR1Command, err, buf.String())
	}
	rbaCheckGolden(t, buf.String())
}
