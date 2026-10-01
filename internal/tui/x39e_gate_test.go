package tui

// 本文件覆盖冻结契约「X-39e：TUI 退出后在 Run 返回之后清掉留在屏幕上的最后一帧」
// （E:\multi_model_cowork\x39e_contract.md，v1.2）第 2.2 节的 A7、A8，
// 以及全部四份 x39e 测试文件共用的帮手 x39eClear 与假 program x39eProgram。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，
// 依据只有契约文本、HEAD 7343f0c 的 run.go / model.go 与既有测试帮手。
//
// 为什么 x39eClear 放在这里：本文件（跨平台）与 rig（只在 Windows 编译）都引用它；
// 放进 _windows_test.go 的话，非 Windows 上本文件编译不过。frame 文件引用的 x39eSized 同理放在这里。
//
// 断言「不写」一律用 final 整串相等，不用子串断言（契约 2 节）。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// x39eClear 构造契约第 3 节冻结的清屏序列，逐字节。
// h >= 2 时从帧末行上移 h-1 行、回行首、擦到屏尾；h == 1 时不写 CUU（参数 0 会被当成 1）。
func x39eClear(h int) string {
	if h >= 2 {
		return tui1dESC + "[" + strconv.Itoa(h-1) + "A\r" + tui1dESC + "[J"
	}
	return "\r" + tui1dESC + "[J"
}

// x39eProgram 是注入给 runWith 的假 program。
//
// 不能复用 tui1cProgram：它的 Run 恒返回 nil model，门 (1) 在它上面恒挡，
// 清屏的正面路径一条都走不到。
// Send 什么都不做；Run 先跑可选的 body，再原样返回用例摆好的 (m, err)。
type x39eProgram struct {
	m    tea.Model
	err  error
	body func()
}

func (p *x39eProgram) Send(tea.Msg) {}

func (p *x39eProgram) Run() (tea.Model, error) {
	if p.body != nil {
		p.body()
	}
	return p.m, p.err
}

// x39eSized 造一个收过一条 WindowSizeMsg{w,h} 的 Model（契约 2.0 的 M(W,H)）。
func x39eSized(t *testing.T, w, h int) Model {
	t.Helper()
	m, _, _ := tui1bNew(t, nil)
	m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// ═══ A7 ═══════════════════════════════════════════════════════════

// A7：四道门与高度的表驱动用例，假 program，final 整串相等。
//
// 【判别力，逐行】每一行的「杀谁」照契约 2.2 的表；成对的对照写在这里：
//   - nil 与 killed_ctx / killed_interrupted / early / panic 只差 err 一处：
//     前者要写 x39eClear(8)，后者一律空串。放行 Killed 或删掉 err 门的实现在后者上红；
//     删掉整个清屏的实现在前者上红。
//   - nil 与 zero / w0_h24 / unsized / resized_to_0 只差 Model 收到的尺寸：
//     用兜底 height（24）判门、只判 Height>0、门 (3) 恒真、不以最近一条为准，各红一行。
//   - h5 / h2 / h1 与 nil 只差终端高：去掉 min 时帧高 8 盖过终端高，三行都红；
//     h1 另钉「h==1 不写 ESC[0A」。
//   - nil_model 是回归钉：类型门被去掉时 m 是零值 Model，termHeight 为 0，门 (3) 兜住，
//     这一行没有独立杀伤，契约照实写明。
//
// 能不能为假：期望值里有非空串（nil、h5、h2、h1），HEAD 上这四行必红；
// 其余九行期望空串，HEAD 上绿，它们靠变异证明有牙（见交付报告）。
// 每行另断言 runWith 原样返回 err（第 5 步不变）。
func TestX39eGates(t *testing.T) {
	type row struct {
		name  string
		model func(t *testing.T) tea.Model
		err   error
		want  string
	}
	sized := func(w, h int) func(t *testing.T) tea.Model {
		return func(t *testing.T) tea.Model { return x39eSized(t, w, h) }
	}
	rows := []row{
		{name: "nil", model: sized(80, 24), err: nil, want: x39eClear(8)},
		{name: "killed_ctx", model: sized(80, 24),
			err: fmt.Errorf("%w: %w", tea.ErrProgramKilled, context.Canceled), want: ""},
		{name: "killed_interrupted", model: sized(80, 24),
			err: fmt.Errorf("%w: %w", tea.ErrProgramKilled, tea.ErrInterrupted), want: ""},
		{name: "early", model: sized(80, 24),
			err: errors.New("bubbletea: error opening TTY"), want: ""},
		{name: "panic", model: sized(80, 24),
			err: fmt.Errorf("%w: %w", tea.ErrProgramKilled, tea.ErrProgramPanic), want: ""},
		{name: "nil_model", model: func(*testing.T) tea.Model { return nil }, err: nil, want: ""},
		{name: "h5", model: sized(80, 5), err: nil, want: x39eClear(5)},
		{name: "h2", model: sized(80, 2), err: nil, want: x39eClear(2)},
		{name: "h1", model: sized(80, 1), err: nil, want: "\r" + tui1dESC + "[J"},
		{name: "zero", model: sized(0, 0), err: nil, want: ""},
		{name: "w0_h24", model: sized(0, 24), err: nil, want: ""},
		{name: "unsized", model: func(t *testing.T) tea.Model {
			m, _, _ := tui1bNew(t, nil)
			return m
		}, err: nil, want: ""},
		{name: "resized_to_0", model: func(t *testing.T) tea.Model {
			m := x39eSized(t, 80, 24)
			m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 0, Height: 0})
			return m
		}, err: nil, want: ""},
	}

	// 前提：x39eClear 自己逐字节等于契约 3.1 / 3.2 给的原始字节，
	// 否则下面的「整串相等」拿错的期望值比对，红绿都不作数。
	if got, want := []byte(x39eClear(8)), []byte{0x1b, 0x5b, 0x37, 0x41, 0x0d, 0x1b, 0x5b, 0x4a}; !bytes.Equal(got, want) {
		t.Fatalf("前提没成立：x39eClear(8) 的字节是 % X，契约 3.1 冻结的是 % X", got, want)
	}
	if got, want := []byte(x39eClear(1)), []byte{0x0d, 0x1b, 0x5b, 0x4a}; !bytes.Equal(got, want) {
		t.Fatalf("前提没成立：x39eClear(1) 的字节是 % X，契约 3.2 冻结的是 % X", got, want)
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			lw := NewLineWriter(nil)
			var final bytes.Buffer
			p := &x39eProgram{m: tc.model(t), err: tc.err}

			gotErr := runWith(p, lw, &final)
			if gotErr != tc.err {
				t.Errorf("runWith 返回 %v，应当原样是 Run 返回的 %v（契约 1.1 第 5 步不变）", gotErr, tc.err)
			}
			if got := final.String(); got != tc.want {
				t.Errorf("行 %s：final 整串是 %q，应当是 %q。\n"+
					"四道门（本包 Model、err == nil、termHeight > 0、final 支持 VT）全部满足才写清屏，\n"+
					"H 取 min(帧高, 真终端高)，h == 1 时只写回行首加擦到屏尾（契约 1.1 R1、3 节）",
					tc.name, got, tc.want)
			}
		})
	}
}

// ═══ A8 ═══════════════════════════════════════════════════════════

// A8：清屏排在第 4 步 Flush 之前，残段落在清屏之后（也就是原帧的顶行）。
//
// 【判别力】清屏挪到 Flush 之后时 final 是「残段\n」加清屏，整串比对红；
// 删掉清屏时 final 是「残段\n」，同样红。
// 对照组：TestTui1cAcc2ResidualGoesToFinalAfterRun，nil model，final 只有残段一行，保持绿；
// 两者只差 Run 返回的 model。
func TestX39eResidualAfterClear(t *testing.T) {
	lw := NewLineWriter(nil)
	var final bytes.Buffer
	p := &x39eProgram{m: x39eSized(t, 80, 24)}
	p.body = func() { tui1cWrite(t, lw, "残段") }

	if err := runWith(p, lw, &final); err != nil {
		t.Fatalf("runWith 返回了非 nil 错误 %v，假 program 的 Run 返回的是 nil", err)
	}
	if got, want := final.String(), x39eClear(8)+"残段\n"; got != want {
		t.Errorf("final 整串是 %q，应当是 %q。\n"+
			"第 2b 步的清屏必须在第 3 步换出口、第 4 步 Flush 之前写出，\n"+
			"残段才从原帧的顶行开始，而不是落在旧帧下面再被擦掉（契约 1.1 R2）", got, want)
	}
}
