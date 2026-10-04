package tui

// 本文件覆盖冻结契约 X44 v1.2（E:\multi_model_cowork\x44_contract.md）§2.2 的 M1-M9：纯 Model，不起 Program。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 80a9385 的源码。
// 本文件与 mlx_program_test.go、mlx_run_test.go 只用 HEAD 已有的标识符，在 HEAD 上能编译（契约 §2.0 末条）；
// 三个文件共用的字面量（提示串、表头、j2）放在本文件。
//
// 提示字面量按契约 §3.1 在这里自己写一份，不引用生产常量：引用它的话「措辞被改」的变异杀不到。
// 不可见字符按码位写（契约 §2.0）。
//
// 断言分两档：前提守卫用 Fatalf；契约点名的断言用 Errorf，好让一次运行能看清是哪几句红。

import (
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
)

// mlxH 是被挡下时状态栏的那句话（契约 §3.1，UTF-8 80 字节，逐字节）。
const mlxH = "一次只能一条命令，逐条回车；整段当消息发，首行只写 /say"

// mlxHDR 是版本为 t 时、提示挂着的不忙表头。
const mlxHDR = "council t   " + mlxH

// mlxJ2 是 维护者 的第二块（契约 §2.2 的 j2）。
const mlxJ2 = "一句话\n/round rba_a rba_b --go"

// mlxK3 是 j2 去掉一个 / 的对照（契约 L1 K3）。
const mlxK3 = "一句话\nround rba_a rba_b --go"

const (
	mlxHdrRunning = "council t   正在运行  0s"
	mlxHdrIdle    = "council t   空闲"
)

// mlxTuiBOM 是 BOM（U+FEFF），按码位写。
var mlxTuiBOM = string(rune(0xFEFF))

// mlxNew 是契约 §2.2 的公共装置（冻结）。
func mlxNew(t *testing.T) (Model, *tui1bSubmitter, *tui1bPrinter) {
	t.Helper()
	sub := &tui1bSubmitter{}
	pr := &tui1bPrinter{}
	m := New(Options{Submit: sub, Out: tui1bNopDrainer{}, Version: "t", Println: pr.cmd, Now: func() time.Time { return rbaT0 }})
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	return m, sub, pr
}

func mlxHeader(m Model) string { return m.chrome()[0] }

func mlxPaste(t *testing.T, m Model, s string) Model {
	t.Helper()
	m, _ = tui1bStep(t, m, tea.PasteMsg{Content: s})
	return m
}

func mlxCtrlHome() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl} }

func mlxBackspace() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyBackspace} }

// mlxLeaves 把 Cmd 跑到底，返回叶子；同时断言没有撞见 BatchMsg（契约 §2.2「叶子」的定义）。
func mlxLeaves(t *testing.T, where string, cmd tea.Cmd) []string {
	t.Helper()
	tr := tui1bRun(t, cmd)
	if tr.sawBatch {
		t.Errorf("%s：展开 Cmd 时撞见了 tea.BatchMsg，契约要求 sawBatch == false；叶子 %q", where, tr.log)
	}
	return tr.log
}

func mlxWantHeader(t *testing.T, where string, m Model, want string) {
	t.Helper()
	if got := mlxHeader(m); got != want {
		t.Errorf("%s：表头 = %q，契约期望 %q", where, got, want)
	}
}

func mlxWantValue(t *testing.T, where string, m Model, want string) {
	t.Helper()
	if got := m.ta.Value(); got != want {
		t.Errorf("%s：输入框的值 = %q，契约期望 %q", where, got, want)
	}
}

func mlxWantNilCmd(t *testing.T, where string, cmd tea.Cmd) {
	t.Helper()
	if cmd != nil {
		t.Errorf("%s：返回的 Cmd 不是 nil，契约期望 nil", where)
	}
}

// TestMlxEnterBlocked：挡下（契约 §2.2 M1），每个子测试一个新 Model。
//
// 【判别力】门被删（Y1）→ Cmd 非 nil、跑 Cmd 之后 count 为 1、值被清空；只改 status 不返回（Y3）→ 表头是正在运行；
// 挡下时 Reset（Y4）→ 值红；挡下时回显（Y11）→ Cmd 非 nil、pr 有记录；照样起 submitCmd（Y12）→ Cmd 与 count 红；
// 置忙（Y9）→ 表头多 "  0s"；措辞改了（Y10）→ 表头红。middle 抓 Z1、Z11；trail 抓把去掉末尾换行的 text 写回（Z3）；
// 打 z 那一句抓门里把光标往回挪（Z12、Z13、Z14）。
// count 那一句必须先跑 Cmd 才能为假：Submit 只在 submitCmd 被执行时才会被调（v1.2，W1b 发现 1）。
// 对照：M2（同一装置，放行；ctl1/ctl2 与 maintainer2/maintainer1 只差一个 /，回车之后 count 是 1）。
// 前提值那一列同时钉住 sanitizer 的行为（\r→换行、tab→四空格、BOM 保留），升级 bubbles 后先红在前提守卫。
func TestMlxEnterBlocked(t *testing.T) {
	type setup func(t *testing.T, m Model) Model
	paste := func(s string) setup {
		return func(t *testing.T, m Model) Model { return mlxPaste(t, m, s) }
	}
	cases := []struct {
		name string
		put  setup
		pre  string
	}{
		{"maintainer1", paste("/new 真机复验\n/add opus-5.0,grok-4.7"), "/new 真机复验\n/add opus-5.0,grok-4.7"},
		{"maintainer2", paste(mlxJ2), mlxJ2},
		{"crlf", paste("a\r\n/c"), "a\n\n/c"},
		{"cr", paste("a\r/c"), "a\n/c"},
		{"tab", paste("a\n\t/c"), "a\n    /c"},
		{"line3", paste("a\nb\n/c"), "a\nb\n/c"},
		{"middle", paste("a\nb\n/c\nd"), "a\nb\n/c\nd"},
		{"trail", paste("a\n/c\n"), "a\n/c\n"},
		{"bom", paste("a\n" + mlxTuiBOM + "/c"), "a\n" + mlxTuiBOM + "/c"},
		{"keyboard", func(t *testing.T, m Model) Model {
			m = tui1bType(t, m, "a")
			m, _ = tui1bStep(t, m, tui1bCtrl('j'))
			return tui1bType(t, m, "  /b")
		}, "a\n  /b"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			m, sub, pr := mlxNew(t)
			m = c.put(t, m)
			if got := m.ta.Value(); got != c.pre {
				t.Fatalf("前提不成立：放进输入框之后值 = %q，契约的前提值 %q", got, c.pre)
			}
			if h := m.frameHeight(); h != 8 {
				t.Fatalf("前提不成立：回车前帧高 = %d，契约的前提 8", h)
			}

			m, cmd := tui1bStep(t, m, tui1bEnter())
			mlxWantNilCmd(t, "回车", cmd)
			tui1bRun(t, cmd)
			if n := sub.count(); n != 0 {
				t.Errorf("回车并跑完返回的 Cmd 之后 Submit 被调了 %d 次，契约期望 0", n)
			}
			if got := pr.snapshot(); len(got) != 0 {
				t.Errorf("挡下时有回显 %q，契约期望没有", got)
			}
			mlxWantHeader(t, "回车之后", m, mlxHDR)
			mlxWantValue(t, "回车之后", m, c.pre)
			if h := m.frameHeight(); h != 8 {
				t.Errorf("回车之后帧高 = %d，契约期望 8", h)
			}

			m = tui1bType(t, m, "z")
			mlxWantValue(t, "挡下之后打 z", m, c.pre+"z")
		})
	}
}

// TestMlxEnterPasses：放行（契约 §2.2 M2），每个子测试一个新 Model。
//
// 【判别力】ctl1/ctl2 与 M1 maintainer2/maintainer1 只差一个 /；leadblank、bomline 抓「整段不剥首」「用 TrimSpace 剥首」（X5、X6）；
// say 抓出路被删（X7）；ctl2 抓「连第 1 行一起判」（X12）；para 抓空行算命令行的两种写法（Z2、Z16）。
// 本组在 HEAD 上全绿（回显与 Submit 收到的文字与 HEAD 相同，D8）；它们能为假：门写宽了，叶子只剩空、count 为 0、表头是提示。
func TestMlxEnterPasses(t *testing.T) {
	cases := []struct {
		name   string
		paste  string
		sub    string
		leaves []string
	}{
		{"ctl1", mlxK3, mlxK3, []string{"print:> 一句话", "print:> round rba_a rba_b --go", "submitDone"}},
		{"ctl2", "/new 真机复验\nadd opus-5.0,grok-4.7", "/new 真机复验\nadd opus-5.0,grok-4.7",
			[]string{"print:> /new 真机复验", "print:> add opus-5.0,grok-4.7", "submitDone"}},
		{"leadblank", "\n/new x", "\n/new x", []string{"print:> ", "print:> /new x", "submitDone"}},
		{"bomline", mlxTuiBOM + "\n/new x", mlxTuiBOM + "\n/new x", []string{"print:> " + mlxTuiBOM, "print:> /new x", "submitDone"}},
		{"say", "/say\n/round x", "/say\n/round x", []string{"print:> /say", "print:> /round x", "submitDone"}},
		{"trail", "/new x\n", "/new x", []string{"print:> /new x", "submitDone"}},
		{"para", "a\r\n\r\nb", "a\n\n\n\nb", []string{"print:> a", "print:> ", "print:> ", "print:> ", "print:> b", "submitDone"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			m, sub, _ := mlxNew(t)
			m = mlxPaste(t, m, c.paste)
			m, cmd := tui1bStep(t, m, tui1bEnter())
			leaves := mlxLeaves(t, "回车", cmd)
			if !reflect.DeepEqual(leaves, c.leaves) {
				t.Errorf("回车之后的叶子 = %q，契约期望 %q", leaves, c.leaves)
			}
			if n := sub.count(); n != 1 {
				t.Errorf("Submit 被调了 %d 次，契约期望 1", n)
			}
			if got := sub.text(0); got != c.sub {
				t.Errorf("Submit 收到 %q，契约期望 %q", got, c.sub)
			}
			mlxWantHeader(t, "回车之后", m, mlxHdrRunning)
		})
	}
}

// TestMlxHintPersistsAndSayEscape：提示持续到哪里、出路走通（契约 §2.2 M3），一个 Model 逐步断言。
//
// 【判别力】打字、窗口尺寸、tick 清提示（Y13、Y14、Y15）→ 第 2、4、5 步红；第 5 步 tick 代次取当前值，
// 保证「清提示」写在 !busy 门之前时可观测。第 6 步幂等：挡下时照样起 submitCmd（Y12）→ Cmd 与 count 红，
// count 的对照是第 10 步（同一个 Model，真的提交）。Esc 清提示（Z7）→ 第 7 步；外层 default 清提示（Z15）→ 第 8 步；
// Ctrl-J 清提示（Z6、Z22）→ 第 9 步。第 9-10 步证明出路可用、且挡下时文字一个字节没丢。
// 【结构上恒真】第 8 步的值那一句：FocusMsg 只转给 textarea 的光标，不改值（契约 §2.2 M3 末段），只为表头那一句。
func TestMlxHintPersistsAndSayEscape(t *testing.T) {
	m, sub, _ := mlxNew(t)
	var cmd tea.Cmd

	// 第 1 步
	m = mlxPaste(t, m, mlxJ2)
	m, _ = tui1bStep(t, m, tui1bEnter())
	mlxWantHeader(t, "第 1 步", m, mlxHDR)
	mlxWantValue(t, "第 1 步", m, mlxJ2)

	// 第 2 步
	m = tui1bType(t, m, "z")
	mlxWantHeader(t, "第 2 步", m, mlxHDR)
	mlxWantValue(t, "第 2 步", m, mlxJ2+"z")

	// 第 3 步
	m, _ = tui1bStep(t, m, mlxBackspace())
	mlxWantHeader(t, "第 3 步", m, mlxHDR)
	mlxWantValue(t, "第 3 步", m, mlxJ2)

	// 第 4 步
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	mlxWantHeader(t, "第 4 步", m, mlxHDR)

	// 第 5 步
	m, cmd = tui1bStep(t, m, elapsedTickMsg{gen: m.tickGen, at: rbaT0.Add(3 * time.Second)})
	mlxWantNilCmd(t, "第 5 步", cmd)
	mlxWantHeader(t, "第 5 步", m, mlxHDR)

	// 第 6 步
	m, cmd = tui1bStep(t, m, tui1bEnter())
	mlxWantNilCmd(t, "第 6 步", cmd)
	mlxWantHeader(t, "第 6 步", m, mlxHDR)
	tui1bRun(t, cmd)
	if n := sub.count(); n != 0 {
		t.Errorf("第 6 步：再回车并跑完返回的 Cmd 之后 Submit 被调了 %d 次，契约期望 0", n)
	}
	mlxWantValue(t, "第 6 步", m, mlxJ2)

	// 第 7 步
	m, cmd = tui1bStep(t, m, tui1bEsc())
	mlxWantNilCmd(t, "第 7 步", cmd)
	mlxWantHeader(t, "第 7 步", m, mlxHDR)
	mlxWantValue(t, "第 7 步", m, mlxJ2)

	// 第 8 步（返回的 Cmd 不断言）
	m, _ = tui1bStep(t, m, tea.FocusMsg{})
	mlxWantHeader(t, "第 8 步", m, mlxHDR)
	mlxWantValue(t, "第 8 步", m, mlxJ2)

	// 第 9 步
	m, _ = tui1bStep(t, m, mlxCtrlHome())
	m = tui1bType(t, m, "/say")
	m, _ = tui1bStep(t, m, tui1bCtrl('j'))
	mlxWantHeader(t, "第 9 步", m, mlxHDR)
	mlxWantValue(t, "第 9 步", m, "/say\n"+mlxJ2)

	// 第 10 步
	m, cmd = tui1bStep(t, m, tui1bEnter())
	leaves := mlxLeaves(t, "第 10 步", cmd)
	want := []string{"print:> /say", "print:> 一句话", "print:> /round rba_a rba_b --go", "submitDone"}
	if !reflect.DeepEqual(leaves, want) {
		t.Errorf("第 10 步：叶子 = %q，契约期望 %q", leaves, want)
	}
	if got := sub.text(0); got != "/say\n"+mlxJ2 {
		t.Errorf("第 10 步：Submit 收到 %q，契约期望 %q", got, "/say\n"+mlxJ2)
	}
	mlxWantHeader(t, "第 10 步", m, mlxHdrRunning)
	mlxWantValue(t, "第 10 步", m, "")
}

// TestMlxPasteReplacesHint：粘贴改写提示（契约 §2.2 M4）。
//
// 【判别力】粘贴不改写提示（Z5）→ 表头红；挡下时 Reset（Y4）或门里挪光标（Z12-Z14）→ 值红。
// 前提守卫让 HEAD 上红在前提（HEAD 不挡，回车之后表头是正在运行）。
func TestMlxPasteReplacesHint(t *testing.T) {
	m, _, _ := mlxNew(t)
	m = mlxPaste(t, m, mlxJ2)
	m, _ = tui1bStep(t, m, tui1bEnter())
	if got := mlxHeader(m); got != mlxHDR {
		t.Fatalf("前提不成立：粘贴 j2 回车之后表头 = %q，应为 %q", got, mlxHDR)
	}
	m = mlxPaste(t, m, "z")
	mlxWantHeader(t, "再粘贴 z", m, "council t   粘贴了 1 个字符")
	mlxWantValue(t, "再粘贴 z", m, mlxJ2+"z")
}

// TestMlxBusyEnterUnchanged：忙时的门在前（契约 §2.2 M5）。
//
// 【判别力】门挪到 Enter 分支最前面（Y7）→ 忙时表头是提示而不是忙时那句。后半段证明忙时回车没吞掉文字、空闲后照样挡；
// 挡下时照样起 submitCmd（Y12）→ 最后一次回车的 Cmd 与 count（got 2 want 1）红；count 的对照是开头那次真的提交。
func TestMlxBusyEnterUnchanged(t *testing.T) {
	m, sub, _ := mlxNew(t)
	m = tui1bType(t, m, "x")
	m, cmd := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd)
	if n := sub.count(); n != 1 {
		t.Fatalf("前提不成立：提交 x 之后 Submit 被调 %d 次，应为 1", n)
	}

	m = mlxPaste(t, m, mlxJ2)
	m, cmd = tui1bStep(t, m, tui1bEnter())
	mlxWantNilCmd(t, "忙时回车", cmd)
	// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，忙时回车的提示跟着改。
	mlxWantHeader(t, "忙时回车", m, "council t   一轮正在跑，先等它回来或连按两次 Ctrl-C 取消  0s")
	mlxWantValue(t, "忙时回车", m, mlxJ2)

	m, _ = tui1bStep(t, m, submitDoneMsg{})
	mlxWantHeader(t, "submitDone 之后", m, mlxHdrIdle)

	m, cmd = tui1bStep(t, m, tui1bEnter())
	mlxWantNilCmd(t, "空闲后回车", cmd)
	tui1bRun(t, cmd)
	if n := sub.count(); n != 1 {
		t.Errorf("空闲后回车并跑完返回的 Cmd 之后 Submit 共被调 %d 次，契约期望 1", n)
	}
	mlxWantHeader(t, "空闲后回车", m, mlxHDR)
	mlxWantValue(t, "空闲后回车", m, mlxJ2)
}

// TestMlxFinalizeWindowUnchanged：收口窗口的门在前（契约 §2.2 M6）。
//
// 【判别力】门放在 busy 之后、finalizing 之前（Y8）或最前面（Y7）→ 窗口里的回车打出提示。
// 挡下时照样起 submitCmd（Y12）→ 最后一次回车的 Cmd 与 count 红（开头那次回车的 Cmd 丢掉没跑，所以期望 0）。
func TestMlxFinalizeWindowUnchanged(t *testing.T) {
	m, sub, _ := mlxNew(t)
	m = tui1bType(t, m, "x")
	m, _ = tui1bStep(t, m, tui1bEnter())
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m, _ = tui1bStep(t, m, roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 2, Speakers: []string{"a", "b"}}})
	m, cmd := tui1bStep(t, m, submitDoneMsg{})
	tr := tui1bRun(t, cmd)
	wantFin := []string{"print:/round 结束  0s  回来 0/2", "other:tui.boardFinalizeMsg"}
	if !reflect.DeepEqual(tr.log, wantFin) || tr.sawBatch {
		t.Fatalf("前提不成立：submitDone 的叶子 = %q（sawBatch=%v），应恰好为 %q", tr.log, tr.sawBatch, wantFin)
	}

	m = mlxPaste(t, m, mlxJ2)
	mlxWantHeader(t, "收口窗口里粘贴", m, "council t   粘贴了 27 个字符")
	if h := m.frameHeight(); h != 10 {
		t.Errorf("收口窗口里粘贴之后帧高 = %d，契约期望 10", h)
	}

	m, cmd = tui1bStep(t, m, tui1bEnter())
	mlxWantNilCmd(t, "收口窗口里回车", cmd)
	mlxWantHeader(t, "收口窗口里回车", m, "council t   粘贴了 27 个字符")
	mlxWantValue(t, "收口窗口里回车", m, mlxJ2)

	m, _ = tui1bStep(t, m, boardFinalizeMsg{})
	if h := m.frameHeight(); h != 8 {
		t.Errorf("收口之后帧高 = %d，契约期望 8", h)
	}

	m, cmd = tui1bStep(t, m, tui1bEnter())
	mlxWantNilCmd(t, "收口之后回车", cmd)
	tui1bRun(t, cmd)
	if n := sub.count(); n != 0 {
		t.Errorf("收口之后回车并跑完返回的 Cmd 之后 Submit 共被调 %d 次，契约期望 0", n)
	}
	mlxWantHeader(t, "收口之后回车", m, mlxHDR)
	mlxWantValue(t, "收口之后回车", m, mlxJ2)
}

// TestMlxIdleCtrlCQuitsAfterBlock：挡下之后空闲连按两次 Ctrl-C 照旧退出（契约 §2.2 M7）。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
//
// 【判别力】提示挂着时空闲第一次 Ctrl-C 只清提示、不武装（Z4）→ 表头回到空闲而不是「再按一次 Ctrl-C 退出」，第一次的断言红；第二次不再是退出，叶子里没有 quit，红。
// HEAD 上红在前提那一句（回车照样提交，表头是正在运行  0s）。
func TestMlxIdleCtrlCQuitsAfterBlock(t *testing.T) {
	m, _, _ := mlxNew(t)
	m = mlxPaste(t, m, mlxJ2)
	m, _ = tui1bStep(t, m, tui1bEnter())
	if got := mlxHeader(m); got != mlxHDR {
		t.Fatalf("前提不成立：粘贴 j2 回车之后表头 = %q，应为 %q", got, mlxHDR)
	}
	m, cmdArm := tui1bStep(t, m, tui1bCtrl('c'))
	mlxWantHeader(t, "空闲第一次 Ctrl-C", m, "council t   再按一次 Ctrl-C 退出")
	if cmdArm == nil || reflect.ValueOf(cmdArm).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
		t.Errorf("空闲第一次 Ctrl-C 应当只武装、不退出（cmd == nil：%v）", cmdArm == nil)
	}
	m, cmd := tui1bStep(t, m, tui1bCtrl('c'))
	mlxWantHeader(t, "空闲第二次 Ctrl-C", m, "council t   正在退出")
	tr := tui1bRun(t, cmd)
	if !tr.hasQuit() {
		t.Errorf("空闲 Ctrl-C 之后叶子里没有 quit：%q", tr.log)
	}
	if tr.sawBatch {
		t.Errorf("空闲 Ctrl-C 的 Cmd 展开时撞见了 tea.BatchMsg，契约期望 sawBatch == false；叶子 %q", tr.log)
	}
}

// TestMlxBlockKeepsCursor：挡下时光标不动（契约 §2.2 M8），两个子测试各一个新 Model。
//
// 【判别力】光标停在第 1 行第 1 个字之后，门里往任何一头挪光标都看得见：写回 text（Z3）与挪到整段末尾（Z10）→ 打 z 落在末尾；
// 挪到本行末尾（Z9）→ 落在第 1 行末；挪到开头（Z12）或行首（Z13、Z14）→ 落在最前。
// block 的缺席断言（Cmd 为 nil、count 为 0、值不变）以 control 为对照：同一装置、同样的按键，只差一个 /，真的提交。
func TestMlxBlockKeepsCursor(t *testing.T) {
	t.Run("block", func(t *testing.T) {
		m, sub, _ := mlxNew(t)
		m = mlxPaste(t, m, mlxJ2)
		m, _ = tui1bStep(t, m, mlxCtrlHome())
		m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
		if got := m.ta.Value(); got != mlxJ2 {
			t.Fatalf("前提不成立：值 = %q，应为 %q", got, mlxJ2)
		}
		m, cmd := tui1bStep(t, m, tui1bEnter())
		mlxWantNilCmd(t, "回车", cmd)
		tui1bRun(t, cmd)
		if n := sub.count(); n != 0 {
			t.Errorf("回车并跑完返回的 Cmd 之后 Submit 被调了 %d 次，契约期望 0", n)
		}
		mlxWantHeader(t, "回车之后", m, mlxHDR)
		mlxWantValue(t, "回车之后", m, mlxJ2)
		m = tui1bType(t, m, "z")
		mlxWantValue(t, "挡下之后打 z", m, "一z句话\n/round rba_a rba_b --go")
	})

	t.Run("control", func(t *testing.T) {
		m, sub, _ := mlxNew(t)
		m = mlxPaste(t, m, mlxK3)
		m, _ = tui1bStep(t, m, mlxCtrlHome())
		m, _ = tui1bStep(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
		if got := m.ta.Value(); got != mlxK3 {
			t.Fatalf("前提不成立：值 = %q，应为 %q", got, mlxK3)
		}
		m, cmd := tui1bStep(t, m, tui1bEnter())
		if cmd == nil {
			t.Fatalf("对照：回车返回的 Cmd 是 nil，契约期望非 nil（真的提交）")
		}
		leaves := mlxLeaves(t, "回车", cmd)
		want := []string{"print:> 一句话", "print:> round rba_a rba_b --go", "submitDone"}
		if !reflect.DeepEqual(leaves, want) {
			t.Errorf("回车之后的叶子 = %q，契约期望 %q", leaves, want)
		}
		if n := sub.count(); n != 1 {
			t.Errorf("Submit 被调了 %d 次，契约期望 1", n)
		}
		if got := sub.text(0); got != mlxK3 {
			t.Errorf("Submit 收到 %q，契约期望 %q", got, mlxK3)
		}
		mlxWantHeader(t, "回车之后", m, mlxHdrRunning)
		mlxWantValue(t, "回车之后", m, "")
	})
}

// TestMlxHintStaysAfterFix：删掉犯规的行之后提示仍挂着，回车照常提交（契约 §2.2 M9）。
//
// 【判别力】按键之后值不再犯规就清提示（Z17）→ 第 2 步表头红；同一写法放在 Esc（Z18）、窗口尺寸（Z19）、tick（Z20）、
// 外层 default（Z21）→ 第 3、4、5、6 步表头红。无条件清提示的 Y13、Y14、Y15、Z7、Z15 也在对应那一步红。
// 第 7 步是第 2-6 步「表头仍是提示」的对照：同一个 Model、同一个值，回车之后表头换成正在运行。
// 挡下时 Reset（Y4）、门里挪光标（Z12-Z14）→ 第 2 步的值红。
// 【结构上恒真】第 6 步的值那一句：FocusMsg 不改值（同 M3 第 8 步）。
func TestMlxHintStaysAfterFix(t *testing.T) {
	m, sub, _ := mlxNew(t)
	var cmd tea.Cmd

	// 第 1 步
	m = mlxPaste(t, m, "a\n/c")
	m, _ = tui1bStep(t, m, tui1bEnter())
	if got := mlxHeader(m); got != mlxHDR {
		t.Fatalf("前提不成立：粘贴 a\\n/c 回车之后表头 = %q，应为 %q", got, mlxHDR)
	}

	// 第 2 步
	m, _ = tui1bStep(t, m, mlxBackspace())
	m, _ = tui1bStep(t, m, mlxBackspace())
	mlxWantHeader(t, "第 2 步", m, mlxHDR)
	mlxWantValue(t, "第 2 步", m, "a\n")

	// 第 3 步
	m, cmd = tui1bStep(t, m, tui1bEsc())
	mlxWantNilCmd(t, "第 3 步", cmd)
	mlxWantHeader(t, "第 3 步", m, mlxHDR)
	mlxWantValue(t, "第 3 步", m, "a\n")

	// 第 4 步
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	mlxWantHeader(t, "第 4 步", m, mlxHDR)

	// 第 5 步
	m, cmd = tui1bStep(t, m, elapsedTickMsg{gen: m.tickGen, at: rbaT0.Add(3 * time.Second)})
	mlxWantNilCmd(t, "第 5 步", cmd)
	mlxWantHeader(t, "第 5 步", m, mlxHDR)

	// 第 6 步（返回的 Cmd 不断言）
	m, _ = tui1bStep(t, m, tea.FocusMsg{})
	mlxWantHeader(t, "第 6 步", m, mlxHDR)
	mlxWantValue(t, "第 6 步", m, "a\n")

	// 第 7 步
	m, cmd = tui1bStep(t, m, tui1bEnter())
	leaves := mlxLeaves(t, "第 7 步", cmd)
	want := []string{"print:> a", "submitDone"}
	if !reflect.DeepEqual(leaves, want) {
		t.Errorf("第 7 步：叶子 = %q，契约期望 %q", leaves, want)
	}
	if n := sub.count(); n != 1 {
		t.Errorf("第 7 步：Submit 被调了 %d 次，契约期望 1", n)
	}
	if got := sub.text(0); got != "a" {
		t.Errorf("第 7 步：Submit 收到 %q，契约期望 %q", got, "a")
	}
	mlxWantHeader(t, "第 7 步", m, mlxHdrRunning)
	mlxWantValue(t, "第 7 步", m, "")
}
