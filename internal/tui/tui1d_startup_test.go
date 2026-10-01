package tui

// 本文件覆盖冻结契约「TUI-1d：TUI 启动死锁、输出可能乱序」
// （E:\multi_model_cowork\tui1d_startup_contract.md，v1.1）第 3 节的 A1-A5。
//
// 背对背纪律：本文件的全部断言由测试腿在看不见实现腿产出的情况下写成，
// 依据只有契约文本、HEAD 的 run.go / model.go / writer.go，以及 bubbletea v2.0.9 源码。
//
// ─── 三件写在前面的事 ───────────────────────────────────────────────
//
// 一、A1 / A2 为什么必须用真的 *tea.Program。
//
// 死锁的根因是 (*tea.Program).Send 在 Run 之前、在同一个 goroutine 上调用时
// 永远堵住（p.msgs 无缓冲，没人读）。tui1c 的假 program 的 Send 只记录、从不阻塞，
// 正是它把这个死锁藏了起来。所以本刀最要紧的一条只能用真程序测。
// 真程序无终端、无输入：WithInput(nil) 关掉输入，WithOutput 指向一个并发安全的缓冲。
//
// 二、ESC 字节不写成字面量。
//
// 源码里只出现 tui1bESC 这个常量（0x1b），转义序列都由它拼出来，
// 免得编辑工具把转义还原成真控制字节。
//
// 三、insertAbove 的字节形状（bubbletea v2.0.9 cursed_renderer.go:757 起）。
//
// 一次打印一行时写出的是 "\r" …… ESC[L <行内容> ESC[K "\r\n"（InsertLine(1) 不带数字）。
// 所以「经事件循环的 insertAbove 打出来的」在原始字节上的指纹是：行内容前面紧挨着 ESC[L。
// 直写 final 的行没有这个前缀。空串的 insertAbove 直接返回，什么都不写。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// tui1dESC 是 ESC 这一个字节组成的串。
var tui1dESC = string(rune(tui1bESC))

// tui1dIL 是 insertAbove 一次插一行时用的 InsertLine 序列 ESC[L。
var tui1dIL = tui1dESC + "[L"

// tui1dMarkRe 认 A2 的行标记 L0000 … L0499。
var tui1dMarkRe = regexp.MustCompile(`L[0-9]{4}`)

// ─── 真程序的装置 ────────────────────────────────────────────────────

// tui1dBuf 是并发安全的输出缓冲：渲染器、第 1 步的直写、测试的轮询三方同时摸它。
type tui1dBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *tui1dBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *tui1dBuf) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// tui1dNopSubmitter 是不做事的 Submitter。A1 / A2 不提交任何东西。
type tui1dNopSubmitter struct{}

func (tui1dNopSubmitter) Submit(context.Context, string) error { return nil }
func (tui1dNopSubmitter) IsQuit(error) bool                    { return false }

// tui1dRig 是一台跑着 runWith 的真程序。
type tui1dRig struct {
	lw   *LineWriter
	w    *tui1dBuf
	p    *tea.Program
	done chan error
}

// tui1dStart 按契约 A1 的装置起一台真程序：
// 先往 LineWriter 里写两行启动横幅（进 pending），再在另一个 goroutine 里跑 runWith。
//
// t.Cleanup(cancel) 是必须的：坏实现下 Send 堵在 Run 之前，只有取消根 ctx
// 才能让它走 ctx.Done() 那一支返回，runWith 的 goroutine 才能收尾，包才能跑完。
func tui1dStart(t *testing.T) *tui1dRig {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	lw := NewLineWriter(nil)
	tui1cWrite(t, lw, "横幅甲\n横幅乙\n")

	w := &tui1dBuf{}
	m := New(Options{Submit: tui1dNopSubmitter{}, Out: lw, Version: "t", Ctx: ctx})
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(w),
		tea.WithoutSignalHandler(),
	)

	done := make(chan error, 1)
	go func() { done <- runWith(p, lw, w) }()
	return &tui1dRig{lw: lw, w: w, p: p, done: done}
}

// tui1dWaitFor 在 d 之内轮询 w 的原始内容，cond 为真就返回 true；到点仍不成立返回 false。
func tui1dWaitFor(w *tui1dBuf, d time.Duration, cond func(raw string) bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond(w.String()) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// tui1dBannersShown：去 ANSI 之后依次出现「横幅甲」「横幅乙」。
func tui1dBannersShown(raw string) bool {
	s := tui1bStripANSI(raw)
	i := strings.Index(s, "横幅甲")
	return i >= 0 && strings.Contains(s[i:], "横幅乙")
}

// waitBanners 等启动横幅出现；5 秒内等不到就判启动死锁。
func (r *tui1dRig) waitBanners(t *testing.T) {
	t.Helper()
	if !tui1dWaitFor(r.w, 5*time.Second, tui1dBannersShown) {
		t.Fatalf("启动死锁：runWith 在 p.Run() 之前堵住 —— 5 秒内输出里没有依次出现 %q、%q。\n"+
			"启动横幅在 p.Run() 之前就在 pending 里；若 SetEmit 装的是 p.Send 包装，补吐会在同一个 goroutine 上\n"+
			"调 Send，而 p.msgs 无缓冲、事件循环还没起来，Send 永远堵住（契约 0.1）。\n"+
			"输出（去 ANSI）是 %q", "横幅甲", "横幅乙", tui1bStripANSI(r.w.String()))
	}
}

// quitAndWait 在另一个 goroutine 上调 p.Quit（它会堵到事件循环接走为止，不能在测试 goroutine 上同步调），
// 然后等 runWith 返回；5 秒内不返回判红。
func (r *tui1dRig) quitAndWait(t *testing.T) {
	t.Helper()
	go r.p.Quit()
	select {
	case err := <-r.done:
		t.Logf("runWith 返回 %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("启动死锁：runWith 在 p.Run() 之前堵住，或 Run 退不出来 —— go p.Quit() 之后 5 秒 runWith 没有返回。\n"+
			"输出（去 ANSI）是 %q", tui1bStripANSI(r.w.String()))
	}
}

// ═══ A1 ═══════════════════════════════════════════════════════════

// A1：真程序不死锁。本刀最要紧的一条。
//
// 【判别力】HEAD 的 runWith 第 1 步是 SetEmit(p.Send 包装)：pending 里的横幅在同一个
// goroutine 上补吐给 Send，Send 堵在无缓冲的 p.msgs 上，p.Run() 永远不会被调用。
// 那时输出里一个字都没有，第一个 5 秒截止判红。（rig 上在 HEAD 的 run.go / model.go
// 加一个 printLine 桩实测过，原始输出贴在测试腿报告里。）
// 断言能不能为假：能 —— 横幅只有在「有人把它们写到 w」时才出现，
// 死锁的实现下没有任何人写 w（Run 没跑，渲染器都没造出来）。
//
// 【判别不了的】启动行经事件循环补吐、只是不死锁的实现（契约 M4：go out.SetEmit(...)）
// 在这里是绿的：A1 只管有没有死锁。那条交给 A3 与 tui1c 的新验收 1。
func TestTui1dA1RealProgramDoesNotDeadlockAtStartup(t *testing.T) {
	r := tui1dStart(t)
	r.waitBanners(t)
	r.quitAndWait(t)
}

// ═══ A2 ═══════════════════════════════════════════════════════════

// A2：真程序上，运行期的行经事件循环的 insertAbove 按写入顺序打出来，空行保留。
//
// 【判别力，逐句】
//   - 哨兵 HELLO 必须带着 ESC[L 前缀出现：证明第 1b 步装上了、运行期的行走的是
//     insertAbove 而不是直写 final。删掉第 1b 步（M6）时 HELLO 被 Fprintln 直写、
//     没有前缀，5 秒截止判红。
//   - 500 个标记全在、严格递增：对「并发 Println」（M3，今天的写法）的判别是概率性的，
//     契约记的实测是 12%-20% 的运行出现逆序；确定性的那一半在 A3。
//   - 原始字节里每个标记前面紧挨着 ESC[L：同上，钉「走的是 insertAbove」。
//     先断言标记恰好 500 个，免得「每个都满足」在零个上空转。
//   - L0250 与 L0251 之间有一次插入的内容恰好是一个空格：printLine 不处理空串（M8）时，
//     tea.Println("") 进 insertAbove 直接返回，两者之间一次插入都没有，红。
//     对照组：同一条用例里 500 个非空行都带 ESC[L，说明「看不见空行」不是装置看不见插入。
func TestTui1dA2RealProgramKeepsOrderAndBlankLine(t *testing.T) {
	r := tui1dStart(t)
	r.waitBanners(t)

	// 哨兵：等它带着 ESC[L 出现，第 1b 步才算装上了。
	tui1cWrite(t, r.lw, "HELLO\n")
	if !tui1dWaitFor(r.w, 5*time.Second, func(raw string) bool {
		return strings.Contains(raw, tui1dIL+"HELLO")
	}) {
		raw := r.w.String()
		t.Fatalf("哨兵 HELLO 5 秒内没有带着 ESC[L 前缀出现在原始输出里。\n"+
			"运行期的行必须经 p.Send(printLine(line)) 进事件循环，由 insertAbove 打出（契约 1.1 第 1b 步）；\n"+
			"直写 final 的行没有这个前缀，会压坏活区。去 ANSI 后里面有没有 HELLO：%v。原始输出是 %q",
			strings.Contains(tui1bStripANSI(raw), "HELLO"), raw)
	}

	// 另一个 goroutine 连写 500 行，L0250 之后夹一行空行。
	// 这里不能用 tui1cWrite：它会在非测试 goroutine 上调 t.Fatalf。Write 恒返回 nil。
	go func() {
		for i := 0; i < 500; i++ {
			_, _ = r.lw.Write([]byte(fmt.Sprintf("L%04d\n", i)))
			if i == 250 {
				_, _ = r.lw.Write([]byte("\n"))
			}
		}
	}()

	if !tui1dWaitFor(r.w, 10*time.Second, func(raw string) bool {
		return strings.Contains(raw, "L0499")
	}) {
		t.Fatalf("10 秒内 L0499 没有出现在输出里。去 ANSI 后的输出尾部是 %q",
			tui1dTail(tui1bStripANSI(r.w.String()), 400))
	}

	raw := r.w.String()
	plain := tui1bStripANSI(raw)

	// 句一：去 ANSI 后 500 个标记全在、严格递增、都在「横幅乙」之后。
	marks := tui1dMarkRe.FindAllStringIndex(plain, -1)
	got := make([]string, 0, len(marks))
	for _, m := range marks {
		got = append(got, plain[m[0]:m[1]])
	}
	want := make([]string, 0, 500)
	for i := 0; i < 500; i++ {
		want = append(want, fmt.Sprintf("L%04d", i))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("去 ANSI 后的行标记不是严格递增的 L0000..L0499：共 %d 个，%s。\n"+
			"运行期的行必须按写入顺序到达；Model 在 Update 里返回 tea.Println Cmd 的写法\n"+
			"每个 Cmd 单开一个 goroutine，顺序没有保证（契约 0.2）",
			len(got), tui1dFirstDiff(got, want))
	}
	if b := strings.Index(plain, "横幅乙"); b < 0 || len(marks) == 0 || marks[0][0] < b {
		t.Errorf("行标记必须都在启动横幅「横幅乙」之后：横幅乙在 %d，第一个标记在 %v", b, tui1dFirstIdx(marks))
	}

	// 句二：原始字节里每个标记前面紧挨着 ESC[L。先钉住个数，免得零个时恒真。
	rawMarks := tui1dMarkRe.FindAllStringIndex(raw, -1)
	if len(rawMarks) != 500 {
		t.Errorf("原始字节里的行标记有 %d 个，应当恰好 500 个（每行只经 insertAbove 打一次）", len(rawMarks))
	}
	bad := 0
	firstBad := ""
	for _, m := range rawMarks {
		if m[0] < len(tui1dIL) || raw[m[0]-len(tui1dIL):m[0]] != tui1dIL {
			bad++
			if firstBad == "" {
				lo := max(0, m[0]-12)
				firstBad = fmt.Sprintf("%q", raw[lo:m[1]])
			}
		}
	}
	if bad != 0 {
		t.Errorf("原始字节里有 %d 个行标记前面没有紧挨着 ESC[L（第一个的上下文是 %s）：\n"+
			"它们不是经事件循环的 insertAbove 打出来的", bad, firstBad)
	}

	// 句三：L0250 与 L0251 之间有一次插入的内容恰好是一个空格。
	i250 := strings.Index(raw, "L0250")
	i251 := strings.Index(raw, "L0251")
	if i250 < 0 || i251 < 0 || i251 < i250 {
		t.Errorf("原始字节里 L0250 在 %d、L0251 在 %d，无法检查两者之间的空行", i250, i251)
	} else {
		between := raw[i250:i251]
		inserts := tui1dInsertBodies(between)
		found := false
		for _, s := range inserts {
			if s == " " {
				found = true
			}
		}
		if !found {
			t.Errorf("L0250 与 L0251 之间没有一次插入的内容恰好是一个空格：两者之间的插入是 %q，原始字节是 %q。\n"+
				"运行期空行必须经 printLine(\"\") 变成一个空格：bubbletea 的 insertAbove 对空串直接返回，\n"+
				"空行会从屏幕上消失（契约 1.1）", inserts, between)
		}
	}

	r.quitAndWait(t)
}

// tui1dInsertBodies 取出 s 里每个 ESC[L 之后、到下一个 ESC 之前的内容，即那一次插入的行内容。
func tui1dInsertBodies(s string) []string {
	var out []string
	for {
		i := strings.Index(s, tui1dIL)
		if i < 0 {
			return out
		}
		s = s[i+len(tui1dIL):]
		j := strings.Index(s, tui1dESC)
		if j < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:j])
		s = s[j:]
	}
}

func tui1dTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func tui1dFirstIdx(marks [][]int) any {
	if len(marks) == 0 {
		return "（没有标记）"
	}
	return marks[0][0]
}

// tui1dFirstDiff 描述两串标记第一处不同的位置，失败信息用。
func tui1dFirstDiff(got, want []string) string {
	n := min(len(got), len(want))
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			lo := max(0, i-3)
			hi := min(len(got), i+4)
			return fmt.Sprintf("第一处不同在第 %d 个：得到 %q 附近是 %v，应当是 %q", i, got[i], got[lo:hi], want[i])
		}
	}
	return fmt.Sprintf("前 %d 个一致，个数不同", n)
}

// ═══ A3 ═══════════════════════════════════════════════════════════

// A3 之一：runWith 的结构，用只记录的假 program（tui1cProgram），确定性。
//
// 【判别力，逐句】
//   - 进 Run() 那一刻 Send 一条都没收到：删掉第 1 步（M1，今天的死锁写法）时，
//     第 1b 步的 SetEmit 在 Run 之前同步把 pending 补吐给 Send，这里有东西，红。
//   - 进 Run() 那一刻 final 逐字节已经是启动行：M1 与 M4（go out.SetEmit(...)，
//     启动行改由事件循环补吐）下 final 都是空的，红。这一句对 M4 是确定性的，
//     与 goroutine 的排期无关：M4 下根本没有任何代码在 Run 之前写 final。
//     快照在 body 的第一行拍，不在 runWith 返回之后查 —— 事后第 3 步的 SetEmit(final)
//     会把 M4 残留在 pending 里的行补吐进 final，事后看不出区别。
//   - Send 依次收到的三条消息分别 DeepEqual 于 printLine("a"/"b"/"c")：
//     运行期出口发自定义消息（M3）时类型对不上，红；删掉第 1b 步（M6）时一条都收不到，红。
//     这一句的期望值来自被测的 printLine 本身，挡不住「printLine 改写内容」；
//     那一半由下面的钉子用例负责。
func TestTui1dA3RunWithStructure(t *testing.T) {
	lw := NewLineWriter(nil)
	tui1cWrite(t, lw, "启动甲\n启动乙\n")

	var final bytes.Buffer
	var finalAtEntry string
	p := &tui1cProgram{}
	p.body = func() {
		finalAtEntry = final.String()
		tui1cWrite(t, lw, "a\n")
		tui1cWrite(t, lw, "b\n")
		tui1cWrite(t, lw, "c\n")
	}

	if err := runWith(p, lw, &final); err != nil {
		t.Fatalf("runWith 返回了非 nil 错误 %v，假 program 的 Run 返回的是 nil", err)
	}
	if p.runCalls != 1 {
		t.Fatalf("Run() 被调了 %d 次，应当恰好 1 次", p.runCalls)
	}

	if len(p.atEntry) != 0 {
		t.Errorf("进 Run() 那一刻 Send 已经收到了 %v，应当一条都没有。\n"+
			"启动行必须在 Run 之前直写 final（契约 1.1 第 1 步）；在 Run 之前经 Send 交出去，\n"+
			"对真的 tea.Program 就是同一个 goroutine 上堵死（契约 0.1）", tui1cLines(p.atEntry))
	}
	if want := "启动甲\n启动乙\n"; finalAtEntry != want {
		t.Errorf("进 Run() 那一刻 final 里是 %q，应当逐字节是 %q。\n"+
			"启动行必须在 p.Run() 之前写到 final（契约 1.1 第 1 步），不能留到 Run 开始之后经事件循环补吐。\n"+
			"runWith 返回之后 final 是 %q，Send 共收到 %v", finalAtEntry, want, final.String(), tui1cLines(p.sent))
	}

	want := []tea.Msg{printLine("a"), printLine("b"), printLine("c")}
	if !reflect.DeepEqual(p.afterBody, want) {
		t.Errorf("Run() 正文写完 a、b、c 时 Send 依次收到的是 %v（类型 %v），应当分别等于 printLine(\"a\")、"+
			"printLine(\"b\")、printLine(\"c\")（类型 %T）。运行期出口必须是 p.Send(printLine(line))（契约 1.1 第 1b 步）",
			tui1cLines(p.afterBody), tui1dTypes(p.afterBody), printLine("a"))
	}
}

func tui1dTypes(msgs []tea.Msg) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, fmt.Sprintf("%T", m))
	}
	return out
}

// A3 之二：钉子。判据来自 tea.Println 原生结果，不来自被测的 printLine。
//
// 【判别力】每个输入卡一种「改写行内容」的坏实现：
//   - "  缩进"：吃掉行首空白（M7，strings.TrimLeft / TrimSpace）；
//   - "尾空格 "：吃掉行尾空白（TrimRight / TrimSpace）；
//   - "50%d"：换成 tea.Printf 把行当格式串（会变成 "50%!d(MISSING)"）；
//   - "a\tb"：把制表符展开或转义；
//   - ESC[31m红ESC[0m：把控制序列转义或剥掉。
//
// 空串那两句卡 M8（不处理空串）：printLine("") 必须等于 tea.Println(" ")()。
//
// 对照组：DeepEqual 对打印消息确实看内容 —— 两个内容不同的 tea.Println 结果不相等。
// 这句要是不成立（比如消息类型没有可比的字段），上面每一句都会恒真。
func TestTui1dA3PrintLinePinnedToTeaPrintln(t *testing.T) {
	if reflect.DeepEqual(tea.Println("  缩进")(), tea.Println("缩进")()) {
		t.Fatalf("前提没成立：tea.Println(\"  缩进\")() 与 tea.Println(\"缩进\")() DeepEqual，" +
			"下面的钉子全部恒真")
	}

	for _, s := range []string{
		"  缩进",
		"尾空格 ",
		"50%d",
		"a\tb",
		tui1dESC + "[31m红" + tui1dESC + "[0m",
	} {
		if got, want := printLine(s), tea.Println(s)(); !reflect.DeepEqual(got, want) {
			t.Errorf("printLine(%q) = %#v，应当 DeepEqual 于 tea.Println(%q)() = %#v。\n"+
				"除了空串换成一个空格，printLine 不许改写行内容（不 trim、不转义、不加前缀、不当格式串）（契约 1.1）",
				s, got, s, want)
		}
	}

	if got, want := printLine(""), tea.Println(" ")(); !reflect.DeepEqual(got, want) {
		t.Errorf("printLine(\"\") = %#v，应当 DeepEqual 于 tea.Println(\" \")() = %#v。\n"+
			"bubbletea 的 insertAbove 对空串直接返回，空行会从屏幕上消失；契约 1.1 要求印一个空格", got, want)
	}
	if reflect.DeepEqual(printLine(""), tea.Println("")()) {
		t.Errorf("printLine(\"\") DeepEqual 于 tea.Println(\"\")()：空串没有换成一个空格，运行期空行会被 insertAbove 丢掉")
	}
}

// ═══ A4 ═══════════════════════════════════════════════════════════

// A4：Run 直接出错（正文不写任何东西）时，启动行仍然在 final 里，错误原样返回。
//
// 【判别力】启动行在 final 里这一句：删掉第 1 步（M1）时启动行在 Run 之前经 Send
// 交给了假 program，第 3 步换回 final 时 pending 已空，final 是空的，红。
// 错误原样返回那一句与 tui1c 的验收 4 同一件事，这里比同一个值。
func TestTui1dA4RunErrorKeepsStartupLinesInFinal(t *testing.T) {
	lw := NewLineWriter(nil)
	tui1cWrite(t, lw, "启动横幅\n")

	sentinel := errors.New("Run 自己炸了")
	var final bytes.Buffer
	p := &tui1cProgram{err: sentinel}

	got := runWith(p, lw, &final)
	if got != sentinel {
		t.Errorf("runWith 返回的是 %v，应当原样是 Run 返回的那个错误 %v（契约 1.1 第 5 步不改）", got, sentinel)
	}
	if want := "启动横幅\n"; final.String() != want {
		t.Errorf("Run 出错时 final 里是 %q，应当是 %q。\n"+
			"第 1 步在 Run 之前已经把启动行写到 final，与 Run 成败无关（契约 3 A4）。\n"+
			"Send 收到的是 %v", final.String(), want, tui1cLines(p.sent))
	}
}

// ═══ A5 ═══════════════════════════════════════════════════════════

// A5：打印消息进 Model.Update 时落到 default 那一支，不改输入框、不提交。
//
// 【判别力弱，照实写】textarea 本来就不理会不认识的消息，今天的 Model 什么都不用做
// 这条就是绿的。它是回归钉，不是牙：只防将来有人为打印消息加一条专门的分支
// （契约 1.2 禁止）而顺手动了输入框。
// 输入框的值不按字段名读，走行为证明（与 tui1b 同一个手法）：Update 之后按 Enter，
// Submitter 收到的必须仍是「草稿」。
func TestTui1dA5PrintLineMsgLeavesInputBoxAlone(t *testing.T) {
	m, sub, _ := tui1bSized(t, nil)
	m = tui1bType(t, m, "草稿")

	m, cmd := tui1bStep(t, m, printLine("x"))
	tui1bRun(t, cmd)
	if n := sub.count(); n != 0 {
		t.Errorf("Update(printLine(\"x\")) 之后 Submit 被调了 %d 次（收到 %q），应当一次都不调", n, sub.text(0))
	}

	_, cmd2 := tui1bStep(t, m, tui1bEnter())
	tui1bRun(t, cmd2)
	if n := sub.count(); n != 1 {
		t.Fatalf("前提没成立：按 Enter 之后 Submit 被调了 %d 次，应当恰好 1 次", n)
	}
	if got := sub.text(0); got != "草稿" {
		t.Errorf("Update(printLine(\"x\")) 之后再按 Enter，Submit 收到的是 %q，应当仍是 %q —— "+
			"打印消息改动了输入框（契约 1.2：不许为它加专门的分支）", got, "草稿")
	}
}
