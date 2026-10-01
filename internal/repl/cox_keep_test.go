package repl

// CO1 §2.2a（R11）：#45 列为简洁档照打的几类行——「已跑 k/N 轮，其余取消」、拒绝句、
// 持久化失败、argv 整轮拒绝、reportErr("round")——各自在简洁档下的夹具。
// 装置复用 cox_round_test.go（coxRoundCfg、coxNewRig、newREPL、假时钟、规范化、慢 a、记录型观察者）。

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"

	"council/internal/config"
)

// coxCancelWriter 是取消写者：带锁；把写入累积到内部缓冲；某次写入的内容含 sub 时，
// 写完之后调一次 cancel（只调第一次）并数触发次数。
type coxCancelWriter struct {
	mu     sync.Mutex
	b      strings.Builder
	sub    string
	cancel context.CancelFunc
	fired  int
}

func (w *coxCancelWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.b.Write(p)
	fire := false
	if strings.Contains(string(p), w.sub) {
		w.fired++
		fire = w.fired == 1
	}
	w.mu.Unlock()
	if fire && w.cancel != nil {
		w.cancel()
	}
	return n, err
}

func (w *coxCancelWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func (w *coxCancelWriter) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.b.Reset()
}

func (w *coxCancelWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fired
}

var coxArgvBytesRe = regexp.MustCompile(`超了 \d+ 字节`)

// coxOneLineWithPrefix 断言 s 以 prefix 开头、以换行结尾、只有一个换行。
func coxOneLineWithPrefix(t *testing.T, s, prefix string) {
	t.Helper()
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, "\n") || strings.Count(s, "\n") != 1 {
		t.Fatalf("输出应当是恰好一行、以 %q 开头，实际: %q", prefix, s)
	}
}

// TestCoxConciseKeepsLines 是 R11：简洁档照打的行，每一类至少一个打印点；
// 同一句话的多个打印点，取消走得到的逐个走到。
//
// 【判别力】每一类都是整行（persist_fail、round_report_err 只断言前缀，后面是存储层的错误链，不是本刀的字面量）：
// 给对应打印点加 if r.concise { return nil }（或 argv 处 return false, nil），那一行就没了，红（§4 #66-#75、#80）。
// 三处「--rounds 需要一个不小于 1 的整数」打同一句话，判别力在走到的是哪个调用点：每个子子测试只走一个，
// 另外两个在对方的变异下保持绿；canceled 与 canceled_judge 同理（#66 下 canceled_judge 绿、#67 下 canceled 绿）。
// canceled 与 canceled_line 只差有没有观察者，互为「总用时」有无的对照（canceled_judge 与 canceled_judge_line 同理）：
// defer 加 || ctx.Err() != nil（#77）时两条 _line 红、两条装观察者的绿；「已跑 k/N」只在装了观察者时打（#78、#79）
// 时各自那条 _line 红。round_report_err 在 :105 加门（#80）时红、persist_fail 绿。
// 缺席「总用时」的正对照：canceled_line（同一装置、同一命令，只差没装观察者，总用时在）。
func TestCoxConciseKeepsLines(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a", "cox_b"}, nil)
		g.slowA(t)
		r, buf, clk := coxRoundREPL(t, g, true)
		ctx2, cancel := context.WithCancel(g.ctx)
		defer cancel()
		r.SetRoundObserver(&coxRecObs{onEnd: cancel})
		if err := r.Submit(ctx2, "/round cox_a cox_b --go --rounds 2"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		raw := buf.String()
		coxRequireBBeforeA(t, raw)
		want := "本轮问 2 个: cox_a, cox_b\ncox_b（T）: OK\n甲（T）: OK\n已跑 1/2 轮，其余取消\n"
		if got := coxNormalize(raw); got != want {
			t.Fatalf("整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
		}
		if n := clk.calls(); n != 1 {
			t.Fatalf("装了观察者，假时钟应当恰好被调用 1 次，实际 %d 次", n)
		}
	})

	t.Run("canceled_judge", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a", "cox_j"}, nil)
		r, buf, clk := coxRoundREPL(t, g, true)
		ctx2, cancel := context.WithCancel(g.ctx)
		defer cancel()
		r.SetRoundObserver(&coxRecObs{onEnd: cancel})
		if err := r.Submit(ctx2, "/round cox_a cox_j --go --with-judge"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		want := "本轮问 1 个: cox_a\n甲（T）: OK\n已跑 1/1 轮，其余取消\n"
		if got := coxNormalize(buf.String()); got != want {
			t.Fatalf("整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
		}
		if n := clk.calls(); n != 1 {
			t.Fatalf("装了观察者，假时钟应当恰好被调用 1 次，实际 %d 次", n)
		}
	})

	t.Run("canceled_line", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a", "cox_b"}, nil)
		g.slowA(t)
		ctx2, cancel := context.WithCancel(g.ctx)
		defer cancel()
		w := &coxCancelWriter{sub: "甲（", cancel: cancel}
		r, clk := g.newREPL(t, w, w.Reset, true)
		if err := r.Submit(ctx2, "/round cox_a cox_b --go --rounds 2"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		if n := w.count(); n != 1 {
			t.Fatalf("前提不成立：取消写者应当恰好触发 1 次，实际 %d 次；输出:\n%s", n, w.String())
		}
		raw := w.String()
		coxRequireBBeforeA(t, raw)
		want := "本轮问 2 个: cox_a, cox_b\ncox_b（T）: OK\n甲（T）: OK\n已跑 1/2 轮，其余取消\n总用时 7.2s\n"
		if got := coxNormalize(raw); got != want {
			t.Fatalf("整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
		}
		if n := clk.calls(); n != 2 {
			t.Fatalf("假时钟应当恰好被调用 2 次，实际 %d 次", n)
		}
	})

	t.Run("canceled_judge_line", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a", "cox_j"}, nil)
		ctx2, cancel := context.WithCancel(g.ctx)
		defer cancel()
		w := &coxCancelWriter{sub: "甲（", cancel: cancel}
		r, clk := g.newREPL(t, w, w.Reset, true)
		if err := r.Submit(ctx2, "/round cox_a cox_j --go --with-judge"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		if n := w.count(); n != 1 {
			t.Fatalf("前提不成立：取消写者应当恰好触发 1 次，实际 %d 次；输出:\n%s", n, w.String())
		}
		want := "本轮问 1 个: cox_a\n甲（T）: OK\n已跑 1/1 轮，其余取消\n总用时 7.2s\n"
		if got := coxNormalize(w.String()); got != want {
			t.Fatalf("整串不对\n%s\n实际: %q\n期望: %q", rbaFirstDiff(got, want), got, want)
		}
		if n := clk.calls(); n != 2 {
			t.Fatalf("假时钟应当恰好被调用 2 次，实际 %d 次", n)
		}
	})

	t.Run("persist_fail", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a"}, nil)
		r, buf, _ := coxRoundREPL(t, g, true)
		ctx2, cancel := context.WithCancel(g.ctx)
		cancel()
		if err := r.Submit(ctx2, "/say hi"); err != nil {
			t.Fatalf("/say 返回错误: %v", err)
		}
		coxOneLineWithPrefix(t, buf.String(), "发言失败: ")
	})

	t.Run("round_report_err", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a"}, nil)
		r, buf, clk := coxRoundREPL(t, g, true)
		ctx2, cancel := context.WithCancel(g.ctx)
		cancel()
		if err := r.Submit(ctx2, "/round cox_a --go"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		coxOneLineWithPrefix(t, buf.String(), "round: ")
		if n := clk.calls(); n != 1 {
			t.Fatalf("假时钟应当恰好被调用 1 次，实际 %d 次", n)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		g := coxNewRig(t, []string{"cox_a"}, nil)
		r, buf, _ := coxRoundREPL(t, g, true)
		for _, c := range []struct {
			name string
			cmd  string
			want string
		}{
			{"rounds_no_value", "/round cox_a --go --rounds", "拒绝: --rounds 需要一个不小于 1 的整数。"},
			{"rounds_not_int", "/round cox_a --rounds abc --go", "拒绝: --rounds 需要一个不小于 1 的整数。"},
			{"rounds_zero", "/round --rounds 0 --go", "拒绝: --rounds 需要一个不小于 1 的整数。"},
			{"rounds_without_go", "/round cox_a --rounds 2", "拒绝: --rounds 2 只在 --go 下可用。"},
			{"unknown_member", "/round nobody --go", "拒绝: 不是当前活跃成员: nobody"},
			{"empty_roster", "/round --go", "拒绝: 名单为空。给了 id 才用给的；没给则取当前会话最后一条用户发言里的 mention 与当前活跃成员的交集。不会默认全员。"},
		} {
			t.Run(c.name, func(t *testing.T) {
				buf.Reset()
				clk := &coxClock{}
				r.now = clk.now
				if err := r.Submit(g.ctx, c.cmd); err != nil {
					t.Fatalf("%s 返回错误: %v", c.cmd, err)
				}
				if got := buf.String(); got != c.want+"\n" {
					t.Fatalf("%s 的输出不对\n实际: %q\n期望: %q", c.cmd, got, c.want+"\n")
				}
				if n := clk.calls(); n != 1 {
					t.Fatalf("假时钟应当恰好被调用 1 次，实际 %d 次", n)
				}
			})
		}
	})

	t.Run("argv_refusal", func(t *testing.T) {
		cfg := coxRoundCfgWith(func(agents map[string]config.AgentConfig) {
			a := agents["cox_a"]
			a.Model = strings.Repeat("m", 40000)
			agents["cox_a"] = a
		})
		g := coxNewRig(t, []string{"cox_a"}, cfg)
		r, buf, clk := coxRoundREPL(t, g, true)
		if err := r.Submit(g.ctx, "/round cox_a --go"); err != nil {
			t.Fatalf("/round 返回错误: %v", err)
		}
		want := "拒绝整轮: cox_a 超了 N 字节。命令行余量为负的人这一轮必然失败。若只把他们拿掉、其余人继续跑，其余人的钱照花，用户拿到残缺的一轮（五个人里两个必然失败、另外三个的钱照花，就是这个形状）；整轮不跑比部分跑更便宜。\n"
		if got := coxArgvBytesRe.ReplaceAllString(buf.String(), "超了 N 字节"); got != want {
			t.Fatalf("整串不对\n实际: %q\n期望: %q", got, want)
		}
		if n := clk.calls(); n != 1 {
			t.Fatalf("假时钟应当恰好被调用 1 次，实际 %d 次", n)
		}
	})
}
