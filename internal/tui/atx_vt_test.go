package tui

// 本文件覆盖 AT1 冻结契约（E:\multi_model_cowork\at1_contract.md，v1.1 + W1b 修订）§2.5 的 V1：VT 回放守卫。
// 真 tea.Program 的输出写进 rbbRecorder，跑完按序喂给 rbbNewVT 的模拟器，读回滚区加屏幕，量残影。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3d134ee 的源码。
// 本文件引用本刀新增的 mentionSource、submitDoneMsg，与本包其余 atx 文件同包编译，在 HEAD 上编译不过（契约 §2.0 的预期红）。
//
// V1 守的是「列表开着时钉住真光标」：不钉光标时收起或筛短列表会留下缩掉的那几行（契约 §0 第 3 条）。
// 放上方（A4）V1 杀不到，放法由 M1 的「View 第 9 行起」整列守。
// 升级 bubbletea 或 ultraviolet 时重跑（与 RB-B 守卫同一条）。

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
)

// atxVTSub 是 V1 的假 Submitter：Submit 用 p.Send(printLine("ATXEND")) 打一行、返回 nil；
// MentionCandidates 返回 atxMembers 的拷贝。p 在 NewProgram 之后赋值。
type atxVTSub struct {
	p *tea.Program
}

var _ mentionSource = (*atxVTSub)(nil)

func (s *atxVTSub) Submit(ctx context.Context, text string) error {
	s.p.Send(printLine("ATXEND"))
	return nil
}

func (s *atxVTSub) IsQuit(error) bool { return false }

func (s *atxVTSub) MentionCandidates(ctx context.Context) ([]repl.MentionCandidate, error) {
	return atxCands(atxMembers), nil
}

// atxVTSpy 同 P1 的 spy 形状（委托、断回 Model、返回自己），只数 submitDoneMsg。
type atxVTSpy struct {
	m     Model
	mu    sync.Mutex
	doneN int
}

func (s *atxVTSpy) Init() tea.Cmd  { return s.m.Init() }
func (s *atxVTSpy) View() tea.View { return s.m.View() }

func (s *atxVTSpy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := s.m.Update(msg)
	if nm, ok := next.(Model); ok {
		s.m = nm
	}
	if _, ok := msg.(submitDoneMsg); ok {
		s.mu.Lock()
		s.doneN++
		s.mu.Unlock()
	}
	return s, cmd
}

func (s *atxVTSpy) dones() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doneN
}

// atxVTScene 是 V1 的一个子测试：键序列、结束时的值、是不是退出场景。
type atxVTScene struct {
	name  string
	keys  []tea.KeyPressMsg
	value string
	exit  bool
}

// atxVTReading 是一次跑、回放之后的读数。
type atxVTReading struct {
	residue int
	heads   int
	unknown []string
	drawn   bool
	value   string
	clean   bool
	dirty   []string
	all     []string
}

// atxVTOnce 按 §2.5「一次跑」起一台真 Program 跑一次，再按「回放」读数。
func atxVTOnce(t *testing.T, sc atxVTScene) atxVTReading {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := &rbbRecorder{}
	sub := &atxVTSub{}
	m := New(Options{Submit: sub, Out: tui1bNopDrainer{}, Version: "t", Ctx: ctx})
	spy := &atxVTSpy{m: m}
	p := tea.NewProgram(spy,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(rec),
		tea.WithoutSignalHandler(),
		tea.WithWindowSize(80, 24),
	)
	sub.p = p
	runDone := make(chan error, 1)
	go func() {
		_, err := p.Run()
		runDone <- err
	}()
	returned := false
	var runErr error
	defer func() {
		if returned {
			return
		}
		cancel()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：cancel 之后 5 秒 Program.Run 仍没有返回")
		}
	}()

	if !rbbWaitUntil(5*time.Second, func() bool { return rec.count() >= 1 }) {
		t.Fatalf("前提不成立：5 秒内录制器一块都没有收到")
	}
	p.Send(tea.PasteMsg{Content: "/use x"})
	p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !rbbWaitUntil(5*time.Second, func() bool { return spy.dones() >= 1 }) {
		t.Fatalf("前提不成立：5 秒内没有等到 /use x 的 submitDone")
	}
	time.Sleep(100 * time.Millisecond)
	for _, k := range sc.keys {
		n := rec.count()
		p.Send(k)
		// 等不到不算错（契约 §2.5）；慢机器上几个键可能合进一次 flush，见契约 §7 (h)。
		rbbWaitUntil(150*time.Millisecond, func() bool { return rec.count() > n })
	}
	time.Sleep(150 * time.Millisecond)

	if sc.exit {
		select {
		case runErr = <-runDone:
			returned = true
		case <-time.After(5 * time.Second):
			t.Fatalf("前提不成立：退出场景 5 秒内 Program.Run 没有自己返回")
		}
	} else {
		cancel()
		select {
		case runErr = <-runDone:
			returned = true
		case <-time.After(5 * time.Second):
			t.Fatalf("cancel 之后 5 秒 Program.Run 没有返回")
		}
	}
	chunks := rec.snapshot()
	var rd atxVTReading
	vt := rbbNewVT(80, 24)
	for i := 1; i <= 30; i++ {
		vt.feed([]byte(fmt.Sprintf("S%02d\r\n", i)))
	}
	for _, c := range chunks {
		vt.feed(c)
		if bytes.Contains(c, []byte("@alpha")) {
			rd.drawn = true
		}
	}
	if sc.exit {
		vt.feed([]byte(exitClearSeq(spy.m, runErr)))
	}
	rd.value = spy.m.ta.Value()
	rd.unknown = vt.unknown

	all := vt.all()
	rd.all = all
	s, h := -1, -1
	for i, ln := range all {
		if ln == "ATXEND" {
			s = i
		}
		if strings.HasPrefix(ln, "council t   ") {
			h = i
			rd.heads++
		}
	}
	rd.residue = h - s - 1
	if s < 0 || h < 0 {
		rd.residue = -1
	}
	rd.clean = true
	for _, ln := range all {
		if strings.HasPrefix(ln, "council t") || strings.HasPrefix(ln, "─") || strings.Contains(ln, "@alpha") {
			rd.clean = false
			rd.dirty = append(rd.dirty, ln)
		}
	}
	return rd
}

// atxTail 取末 n 行，失败信息用。
func atxTail(ls []string, n int) []string {
	if len(ls) > n {
		return ls[len(ls)-n:]
	}
	return ls
}

// TestAtxVTNoResidue 是 V1：七个子测试，每个跑 5 次。
//
// 【判别力】A1（不钉光标）→ 七个子测试全红（契约 §0 第 3 条的探针在同样的键序列上读到残影 6/6/7/11/6/6、exit 0/5 干净）。
// 「画过」前提（原始块里有 @alpha）防止列表根本没出现时残影为 0 的空绿；「值」前提防止键序列没按预期生效
// （A17、A28、A29、A37、A38 会让个别子测试的值前提红）。
// 残影 0 能为假：不钉光标时就是缩掉的行数（上面的探针读数）；表头行数 1 与 unknown 0 是同一次回放的旁证。
// 退出场景的「干净」能为假：不钉光标时 5 次都不干净（契约 §0 表「退出」一行）。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里连发两次。
// exit 场景第一个 Ctrl-C 只收起列表，后面两次才是空闲退出。
// A2（Y 不加 chrome）、A4（放上方）V1 杀不到：光标仍钉在输入框及以上，公式为 0；它们由 M1 守。
func TestAtxVTNoResidue(t *testing.T) {
	k := tui1bKey
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	bs := tea.KeyPressMsg{Code: tea.KeyBackspace}
	cj := tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
	cc := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

	scenes := []atxVTScene{
		{name: "open_esc", keys: []tea.KeyPressMsg{k('@'), esc}, value: "@"},
		{name: "down_tab", keys: []tea.KeyPressMsg{k('@'), down, tab}, value: "@b1 "},
		{name: "narrow", keys: []tea.KeyPressMsg{k('@'), k('b'), k('r'), k('x'), bs, esc}, value: "@br"},
		{name: "backspace", keys: []tea.KeyPressMsg{k('@'), k('a'), bs, bs}, value: ""},
		{name: "multiline", keys: []tea.KeyPressMsg{k('x'), cj, k('y'), cj, k('z'), cj, k('@'), down, esc}, value: "x\ny\nz\n@"},
		{name: "enter_accept", keys: []tea.KeyPressMsg{k('@'), k('b'), enter}, value: "@b1 "},
		{name: "exit", keys: []tea.KeyPressMsg{k('@'), cc, cc, cc}, exit: true},
	}
	for _, sc := range scenes {
		t.Run(sc.name, func(t *testing.T) {
			for run := 1; run <= 5; run++ {
				rd := atxVTOnce(t, sc)
				t.Logf("%s 第 %d 次：残影 %d，表头行 %d，unknown %d，画过 %v，值 %q，干净 %v",
					sc.name, run, rd.residue, rd.heads, len(rd.unknown), rd.drawn, rd.value, rd.clean)
				if !rd.drawn {
					t.Fatalf("%s 第 %d 次：前提不成立：原始块里没有含 @alpha 的块（列表根本没画出来）", sc.name, run)
				}
				if !sc.exit && rd.value != sc.value {
					t.Fatalf("%s 第 %d 次：前提不成立：结束时的值应当是 %q，实际 %q", sc.name, run, sc.value, rd.value)
				}
				if sc.exit {
					if !rd.clean {
						t.Errorf("%s 第 %d 次：退出后屏幕与回滚区里不应当有表头、横线或候选行，实际有 %q", sc.name, run, rd.dirty)
					}
				} else {
					if rd.residue != 0 {
						t.Errorf("%s 第 %d 次：残影应当是 0 行，实际 %d", sc.name, run, rd.residue)
					}
					if rd.heads != 1 {
						t.Errorf("%s 第 %d 次：以「council t   」开头的行应当恰好 1 行，实际 %d", sc.name, run, rd.heads)
					}
					if len(rd.unknown) != 0 {
						t.Errorf("%s 第 %d 次：模拟器未知控制序列应当是 0 条，实际 %d 条：%q", sc.name, run, len(rd.unknown), rd.unknown)
					}
				}
				if t.Failed() {
					t.Logf("%s 第 %d 次回放后的末 40 行:\n%s", sc.name, run, strings.Join(atxTail(rd.all, 40), "\n"))
					return
				}
			}
		})
	}
}
