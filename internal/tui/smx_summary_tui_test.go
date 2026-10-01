package tui

// SM1 §2.5：真 tea.Program、真 REPL、真 Runner、假 CLI（本包 rba_fakecli_test.go 的 TestMain）。
// 引用本刀的 config.SummaryConfig / Config.Summary 与 CO1 的 (*repl.REPL).SetConcise，
// 在两刀都落地之前编译不过是预期的红。

import (
	"context"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/repl"
	"council/internal/store"
	"council/internal/turn"
)

// smxNewTuiRig 照 rbaNewTuiRig 的步骤搭一份装置，差别只有：加 smx_s（同 rba_a、显示名「总结 S」）与 [summary]；
// 只 Join rba_a、rba_b（不 Join rba_j、smx_s）；会话标题 smx；种子消息 seed-msg。
func smxNewTuiRig(t *testing.T, summary string) *rbaTuiRig {
	t.Helper()
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "council-data"))
	if err != nil {
		t.Fatalf("装置：store.Open 失败: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("装置：关闭 store 失败: %v", err)
		}
	})
	svc := conversation.NewService(st)
	cfg := rbaTuiConfig()
	s := cfg.Agents["rba_a"]
	s.DisplayName = "总结 S"
	cfg.Agents["smx_s"] = s
	cfg.Summary = config.SummaryConfig{Agent: summary}
	rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})

	conv, err := svc.Create(ctx, "smx")
	if err != nil {
		t.Fatalf("装置：建会话失败: %v", err)
	}
	for _, id := range []string{"rba_a", "rba_b"} {
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

	lw := NewLineWriter(nil)
	r := repl.New(cfg, svc, rn, nil, lw, repl.RulesContext{})
	return &rbaTuiRig{svc: svc, rn: rn, r: r, lw: lw, convID: conv.ID}
}

const (
	smxTuiCmd  = "/round rba_a rba_b --go"
	smxTuiHint = "总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）"
)

var (
	smxTuiHeadRe   = regexp.MustCompile(`^X（\d+\.\ds）: OK$`)
	smxTuiFin3Re   = regexp.MustCompile(`^/round 结束  \d+s  回来 3/3$`)
	smxTuiFin2Re   = regexp.MustCompile(`^/round 结束  \d+s  回来 2/2$`)
	smxTuiBlockRe  = regexp.MustCompile(`^总结轮 \([0-9a-f]{6}\)  回来 1/1$`)
	smxTuiMemberRe = regexp.MustCompile(`^  完成    smx_s  \d+\.\ds 子进程$`)
)

// TestSmxProgramSummarySegment 是 T1：看板上的第三种段、收尾行的人数、详细档段终态块、没配时的提示行。
//
// 【判别力】concise / verbose / concise_unconfigured 两两只差一处：总结段不发事件时 kinds 少一段，红；
// 收尾行的人数不把总结段算进去时 3/3 变 2/2，红；详细档的段终态块用错标签时 verbose 红；
// 简洁档不打提示行时 concise_unconfigured 的 P 少一条，红。verbose_unconfigured 的「没有提示行」是缺席断言，
// 正对照是 concise_unconfigured（同一装置、同一窗口、同一 kinds 串，只差档位）；它杀「装了观察者时详细档也打提示行」。
func TestSmxProgramSummarySegment(t *testing.T) {
	cases := []struct {
		name    string
		summary string
		concise bool
	}{
		{"concise", "smx_s", true},
		{"verbose", "smx_s", false},
		{"concise_unconfigured", "", true},
		{"verbose_unconfigured", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := smxNewTuiRig(t, tc.summary)
			if tc.concise {
				g.r.SetConcise(true)
			}
			pr := rbbStartProg(t, g)
			pr.useConv(t, g.convID)
			pr.submit(smxTuiCmd)
			if !pr.waitFor(30*time.Second, rbbDoneThenFin(2)) {
				t.Fatalf("前提不成立：30 秒内没有等到 /round 的 DONE 与 FIN；日志:\n%s", rbbLogDump(pr.spy.entries()))
			}
			w, ok := rbbWindow(pr.spy.entries(), smxTuiCmd)
			if !ok {
				t.Fatalf("前提不成立：取不到 /round 的窗口；日志:\n%s", rbbLogDump(pr.spy.entries()))
			}
			kinds, idx := rbbEventKinds(w)
			var ps []string
			for _, e := range w {
				if e.kind == "P" {
					ps = append(ps, e.text)
				}
			}
			switch tc.name {
			case "concise":
				if kinds != "RS MD MD RE RS MD RE DONE FIN" {
					t.Fatalf("事件序列不对: %q；窗口:\n%s", kinds, rbbLogDump(w))
				}
				if len(ps) != 6 || ps[0] != "本轮问 2 个: rba_a, rba_b" || !smxTuiHeadRe.MatchString(ps[1]) || !smxTuiHeadRe.MatchString(ps[2]) ||
					ps[3] != "本轮问 1 个: smx_s" || ps[4] != "总的来看（总结 S 做总结）：OK" || !smxTuiFin3Re.MatchString(ps[5]) {
					t.Fatalf("P 应当恰好 6 条且逐条匹配，实际 %q；窗口:\n%s", ps, rbbLogDump(w))
				}
				rs := w[idx[4]].rs
				if rs.Label != "总结轮" || len(rs.Speakers) != 1 || rs.Speakers[0] != "smx_s" {
					t.Fatalf("第二个 RS 应当 Label 总结轮、Speakers [smx_s]，实际 %+v", rs)
				}
			case "verbose":
				if kinds != "RS MD MD RE RS MD RE DONE FIN" {
					t.Fatalf("事件序列不对: %q；窗口:\n%s", kinds, rbbLogDump(w))
				}
				hit := -1
				for i, p := range ps {
					if smxTuiBlockRe.MatchString(p) {
						if hit >= 0 {
							t.Fatalf("段终态块标题出现不止一次；窗口:\n%s", rbbLogDump(w))
						}
						hit = i
					}
				}
				if hit < 0 || hit+1 >= len(ps) || !smxTuiMemberRe.MatchString(ps[hit+1]) {
					t.Fatalf("应当恰好一条「总结轮 (xxxxxx)  回来 1/1」且紧随成员行；P: %q", ps)
				}
				if !smxTuiFin3Re.MatchString(ps[len(ps)-1]) {
					t.Fatalf("最后一条 P 应当是 3/3 的收尾行，实际 %q", ps[len(ps)-1])
				}
			case "concise_unconfigured":
				if kinds != "RS MD MD RE DONE FIN" {
					t.Fatalf("事件序列不对: %q；窗口:\n%s", kinds, rbbLogDump(w))
				}
				if len(ps) != 5 || ps[3] != smxTuiHint || !smxTuiFin2Re.MatchString(ps[4]) {
					t.Fatalf("P 应当恰好 5 条、第 4 条是提示行、最后一条是 2/2 的收尾行，实际 %q；窗口:\n%s", ps, rbbLogDump(w))
				}
			case "verbose_unconfigured":
				if kinds != "RS MD MD RE DONE FIN" {
					t.Fatalf("事件序列不对: %q；窗口:\n%s", kinds, rbbLogDump(w))
				}
				for _, p := range ps {
					if p == smxTuiHint {
						t.Fatalf("详细档不应当有提示行；窗口:\n%s", rbbLogDump(w))
					}
				}
				if len(ps) == 0 || !smxTuiFin2Re.MatchString(ps[len(ps)-1]) {
					t.Fatalf("最后一条 P 应当是 2/2 的收尾行；P: %q", ps)
				}
			}
		})
	}
}
