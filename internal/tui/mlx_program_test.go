package tui

// 本文件覆盖冻结契约 X44 v1.2（E:\multi_model_cowork\x44_contract.md）§2.3 的 P1：
// 真 tea.Program、真 REPL、真 Runner、假 CLI（TestMain 在 rba_fakecli_test.go，本文件不加）。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 80a9385 的源码。
// 只用 HEAD 已有的标识符，在 HEAD 上能编译（契约 §2.0 末条）。
//
// spy 照 RB-A rbaSpy 的形状另写一份（契约 §2.3：rbaSpy 不记按键，不复用）。

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// mlxCountSub 包住 replAdapter：Submit 先在锁里记下收到的文字、再委托；IsQuit 委托。
type mlxCountSub struct {
	inner replAdapter
	mu    sync.Mutex
	texts []string
}

func (s *mlxCountSub) Submit(ctx context.Context, text string) error {
	s.mu.Lock()
	s.texts = append(s.texts, text)
	s.mu.Unlock()
	return s.inner.Submit(ctx, text)
}

func (s *mlxCountSub) IsQuit(err error) bool { return s.inner.IsQuit(err) }

func (s *mlxCountSub) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

// mlxEntry 是 spy 记下的一条 Update。kind 取 KEY、PASTE、DONE、RS、P。
// header 与 value 是这条 Update 之后的 chrome()[0] 与 ta.Value()。
type mlxEntry struct {
	kind   string
	text   string
	header string
	value  string
}

func (e mlxEntry) String() string {
	return fmt.Sprintf("%s %q | %s | %q", e.kind, e.text, e.header, e.value)
}

// mlxSpy：Init、View 委托；Update 先委托、把返回值断回 Model 存回、再按消息记一条日志、返回 spy 自己。
type mlxSpy struct {
	m   Model
	mu  sync.Mutex
	log []mlxEntry
}

func (s *mlxSpy) Init() tea.Cmd  { return s.m.Init() }
func (s *mlxSpy) View() tea.View { return s.m.View() }

func (s *mlxSpy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := s.m.Update(msg)
	if nm, ok := next.(Model); ok {
		s.m = nm
	}
	var e mlxEntry
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		e = mlxEntry{kind: "KEY", text: v.String()}
	case tea.PasteMsg:
		e = mlxEntry{kind: "PASTE", text: v.Content}
	case submitDoneMsg:
		e = mlxEntry{kind: "DONE"}
		if v.err != nil {
			e.text = v.err.Error()
		}
	case roundStartMsg:
		e = mlxEntry{kind: "RS", text: v.ev.Label}
	default:
		if fmt.Sprintf("%T", msg) != "tea.printLineMessage" {
			return s, cmd
		}
		e = mlxEntry{kind: "P", text: reflect.ValueOf(msg).FieldByName("messageBody").String()}
	}
	e.header = s.m.chrome()[0]
	e.value = s.m.ta.Value()
	s.mu.Lock()
	s.log = append(s.log, e)
	s.mu.Unlock()
	return s, cmd
}

func (s *mlxSpy) entries() []mlxEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]mlxEntry(nil), s.log...)
}

// mlxProg 是一台跑着的真 Program 与它的装置。
type mlxProg struct {
	g   *rbaTuiRig
	p   *tea.Program
	spy *mlxSpy
	sub *mlxCountSub
}

// mlxStartProg 照契约 §2.3 的装置（冻结）起真 Program，并先提交 /use、等第 1 条 DONE（前提）。
func mlxStartProg(t *testing.T) *mlxProg {
	t.Helper()
	g := rbaNewTuiRig(t)
	sub := &mlxCountSub{inner: replAdapter{r: g.r}}
	ctx, cancel := context.WithCancel(context.Background())
	m := New(Options{Submit: sub, Out: g.lw, Version: "t", Ctx: ctx})
	spy := &mlxSpy{m: m}
	p := tea.NewProgram(spy,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithoutSignalHandler(),
		tea.WithWindowSize(100, 24),
	)
	g.r.SetRoundObserver(newRoundObserver(p.Send))
	g.lw.SetEmit(func(line string) { p.Send(printLine(line)) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = p.Run()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：cancel 之后 5 秒 Program.Run 仍没有返回")
		}
	})
	pr := &mlxProg{g: g, p: p, spy: spy, sub: sub}
	pr.submit("/use " + g.convID)
	pr.waitCount(t, "DONE", "", 1, 10*time.Second)
	return pr
}

// submit = 粘贴再按回车（契约 §2.3「提交 s」）。
func (pr *mlxProg) submit(s string) {
	pr.p.Send(tea.PasteMsg{Content: s})
	pr.p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// mlxCount 数日志里 kind（且 text 等于 text，text 为空时不看）的条数。
func mlxCount(log []mlxEntry, kind, text string) int {
	n := 0
	for _, e := range log {
		if e.kind == kind && (text == "" || e.text == text) {
			n++
		}
	}
	return n
}

// waitCount 等到日志里有 n 条指定条目；等不到就 Fatalf（前提，不是断言对象）。
func (pr *mlxProg) waitCount(t *testing.T, kind, text string, n int, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		if mlxCount(pr.spy.entries(), kind, text) >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("前提不成立：%v 之内没有等到第 %d 条 %s %q；日志:\n%s", d, n, kind, text, mlxDump(pr.spy.entries()))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func mlxDump(log []mlxEntry) string {
	s := ""
	for i, e := range log {
		s += fmt.Sprintf("%3d %s\n", i, e.String())
	}
	return s
}

// mlxNthIndex 返回第 n 条（从 1 起）指定条目的下标；没有时返回 -1。
func mlxNthIndex(log []mlxEntry, kind, text string, n int) int {
	k := 0
	for i, e := range log {
		if e.kind == kind && (text == "" || e.text == text) {
			k++
			if k == n {
				return i
			}
		}
	}
	return -1
}

// mlxPsAfter 返回下标 from 之后的 P 正文：to 为 -1 时到下一条 DONE 为止；否则到下标 to 为止（都不含）。
func mlxPsAfter(log []mlxEntry, from, to int) []string {
	out := []string{}
	for i := from + 1; i < len(log); i++ {
		if to >= 0 && i >= to {
			break
		}
		if to < 0 && log[i].kind == "DONE" {
			break
		}
		if log[i].kind == "P" {
			out = append(out, log[i].text)
		}
	}
	return out
}

// mlxCheckTail 是 reject 与 control 共有的第 4、5 条。
func (pr *mlxProg) mlxCheckTail(t *testing.T, log []mlxEntry, seq2 string) {
	t.Helper()
	// 第 4 条
	if n := mlxCount(log, "RS", ""); n != 0 {
		t.Errorf("第 4 条：日志里有 %d 条 RS，契约期望 0；日志:\n%s", n, mlxDump(log))
	}
	if n := mlxCount(log, "DONE", ""); n != 2 {
		t.Errorf("第 4 条：日志里有 %d 条 DONE，契约期望恰好 2；日志:\n%s", n, mlxDump(log))
	}
	if len(log) == 0 {
		t.Errorf("第 4 条：日志是空的")
	} else {
		last := log[len(log)-1]
		if last.kind != "DONE" {
			t.Errorf("第 4 条：最后一条日志是 %s，契约期望 DONE；日志:\n%s", last.String(), mlxDump(log))
		} else if last.header != mlxHdrIdle {
			t.Errorf("第 4 条：最后一条 DONE 的表头 = %q，契约期望 %q", last.header, mlxHdrIdle)
		}
	}
	// 第 5 条
	msgs, err := pr.g.svc.History(context.Background(), pr.g.convID, 0, 100)
	if err != nil {
		t.Fatalf("第 5 条：读 History 失败：%v", err)
	}
	if len(msgs) != 2 {
		texts := make([]string, 0, len(msgs))
		for _, m := range msgs {
			texts = append(texts, m.Text)
		}
		t.Errorf("第 5 条：会话里有 %d 条消息 %q，契约期望恰好 2 条", len(msgs), texts)
		return
	}
	if msgs[0].Seq != 1 || msgs[0].Text != "seed-msg" {
		t.Errorf("第 5 条：第 1 条消息 seq=%d text=%q，契约期望 seq 1 %q", msgs[0].Seq, msgs[0].Text, "seed-msg")
	}
	if msgs[1].Seq != 2 || msgs[1].Text != seq2 {
		t.Errorf("第 5 条：第 2 条消息 seq=%d text=%q，契约期望 seq 2 %q", msgs[1].Seq, msgs[1].Text, seq2)
	}
}

// TestMlxProgramBlocksLaterCommand：真 Program 上挡下、走出路（契约 §2.3 P1），两个子测试各一套装置，只差一个 /。
//
// 【判别力】这是跨并发原语的那一条：回车经真事件循环、Submit 在 Cmd goroutine 上、REPL 的行经 p.Send 回来。
// reject 第 2 条是缺席断言，对照是 control 第 3 条（同一装置，只差一个 /，回车之后立刻有回显与「已记下」）。
// 门被删（Y1）或挡下时照样起 submitCmd（Y12）→ 第 1 条多出一条、第 5 条多出一条消息；挡下时回显（Y11）→ 第 2 条红；
// 出路被删（X7）→ 出路那次回车又被挡，等不到第 2 条 DONE，前提不成立而红。
func TestMlxProgramBlocksLaterCommand(t *testing.T) {
	t.Run("reject", func(t *testing.T) {
		pr := mlxStartProg(t)
		pr.submit(mlxJ2)
		pr.waitCount(t, "KEY", "enter", 2, 5*time.Second)
		log := pr.spy.entries()
		i2 := mlxNthIndex(log, "KEY", "enter", 2)
		if log[i2].header != mlxHDR {
			t.Errorf("第 2 条 KEY enter 之后表头 = %q，契约期望 %q", log[i2].header, mlxHDR)
		}
		if log[i2].value != mlxJ2 {
			t.Errorf("第 2 条 KEY enter 之后值 = %q，契约期望 %q", log[i2].value, mlxJ2)
		}

		pr.p.Send(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl})
		for _, r := range "/say" {
			pr.p.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		pr.p.Send(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
		pr.p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		pr.waitCount(t, "DONE", "", 2, 10*time.Second)
		time.Sleep(300 * time.Millisecond)
		log = pr.spy.entries()

		// 第 1 条
		wantSub := []string{"/use " + pr.g.convID, "/say\n" + mlxJ2}
		if got := pr.sub.got(); !reflect.DeepEqual(got, wantSub) {
			t.Errorf("第 1 条：Submitter 收到 %q，契约期望 %q", got, wantSub)
		}
		// 第 2 条
		i2 = mlxNthIndex(log, "KEY", "enter", 2)
		i3 := mlxNthIndex(log, "KEY", "enter", 3)
		if i3 < 0 {
			t.Errorf("第 2 条：日志里没有第 3 条 KEY enter；日志:\n%s", mlxDump(log))
		} else if ps := mlxPsAfter(log, i2, i3); len(ps) != 0 {
			t.Errorf("第 2 条：第 2、3 条 KEY enter 之间有 P %q，契约期望没有", ps)
		}
		// 第 3 条
		wantP := []string{"> /say", "> 一句话", "> /round rba_a rba_b --go", "已记下 seq 2"}
		if i3 >= 0 {
			if ps := mlxPsAfter(log, i3, -1); !reflect.DeepEqual(ps, wantP) {
				t.Errorf("第 3 条：第 3 条 KEY enter 之后、下一条 DONE 之前的 P = %q，契约期望 %q", ps, wantP)
			}
		}
		pr.mlxCheckTail(t, log, mlxJ2)
	})

	t.Run("control", func(t *testing.T) {
		pr := mlxStartProg(t)
		pr.submit(mlxK3)
		pr.waitCount(t, "DONE", "", 2, 10*time.Second)
		time.Sleep(300 * time.Millisecond)
		log := pr.spy.entries()

		// 第 1 条
		wantSub := []string{"/use " + pr.g.convID, mlxK3}
		if got := pr.sub.got(); !reflect.DeepEqual(got, wantSub) {
			t.Errorf("第 1 条：Submitter 收到 %q，契约期望 %q", got, wantSub)
		}
		// 第 3 条
		i2 := mlxNthIndex(log, "KEY", "enter", 2)
		wantP := []string{"> 一句话", "> round rba_a rba_b --go", "已记下 seq 2"}
		if i2 < 0 {
			t.Errorf("第 3 条：日志里没有第 2 条 KEY enter；日志:\n%s", mlxDump(log))
		} else if ps := mlxPsAfter(log, i2, -1); !reflect.DeepEqual(ps, wantP) {
			t.Errorf("第 3 条：第 2 条 KEY enter 之后、下一条 DONE 之前的 P = %q，契约期望 %q", ps, wantP)
		}
		pr.mlxCheckTail(t, log, mlxK3)
	})
}
