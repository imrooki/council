package tui

// 本文件覆盖冻结契约 RB-B v1.1 §2.5 的 G1-G4：VT 回放守卫（探针 R10）。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本、探针结论
// （rb_o2_probe_result.json 的 measurements 与 contract_requirements R10）与 HEAD a728cee 的源码。
//
// 做法：真 tea.Program 的输出写进一个按 Write 分块的录制器；跑完之后把块按序喂给测试代码里重写的
// VT 模拟器（语义照契约 §2.5 冻结），读屏幕与回滚区，得出残影行数；另用「缩高那一块喂进去之前的光标行」
// 按公式 max(0, relY-7) 算一次，两种读数互相核对。模拟器对未知控制序列计数，每次跑都断言为 0。
// 源码里不写 ESC 字面量，一律用 tui1dESC 拼、或按字节值 0x1b 比较。
//
// 落地后补（2026-09-25）：契约 §2.5 只写了 Windows 上 \n 的语义。非 Windows 上 bubbletea 让渲染器
// 假定终端把 \n 映射成 \r\n，模拟器按 rbbRendererMapsNL 照样解释；不补的话 G1-G4 在 Linux 上读出
// 10 行、1 行的假残影。Windows 上的行为一字未变。
//
// 本文件引用本刀新增的 Speakers / Results 字段与 boardFinalizeMsg，在 HEAD 上整包编译不过，
// 这是契约 §2 写明的预期红。

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"council/internal/repl"
	"council/internal/turn"
)

// ─── VT 模拟器 ─────────────────────────────────────────────────────────

// rbbCell 是一格。w == 0 是空格子；w == 1 / 2 是字符的首格与它的宽度；w == -1 是宽字符的续格。
type rbbCell struct {
	r rune
	w int
}

// rbbVT 是宽 w、高 h 的格子屏，外加回滚区与光标。语义见契约 §2.5，逐条照写。
type rbbVT struct {
	w, h    int
	grid    [][]rbbCell
	x, y    int
	wrap    bool // 待折行
	sx, sy  int  // ESC 7 存下的光标
	back    []string
	last    rune // 上一个可打印字符（REP 用）
	unknown []string
	pend    []byte // 块尾没收完的序列或 UTF-8，留给下一块
	mapNL   bool   // \n 是否连带回车，见 rbbRendererMapsNL
}

// rbbRendererMapsNL 照 bubbletea v2.0.9 `tea.go:1084` 的判据
// `mapNl := runtime.GOOS != "windows" && p.ttyInput == nil`。本文件的 Program 用 tea.WithInput(nil)，
// ttyInput 恒为 nil，只剩平台这一条。mapNl 为真时渲染器假定终端的 ONLCR 把 \n 映射成 \r\n，
// 换行处只写 \n、不写 \r，模拟器得照样解释；否则非 Windows 上读出来的「残影」是模拟器误读。
// Windows 上为假，\n 只下移不回车，与契约 §2.5 冻结的语义逐字相同。
var rbbRendererMapsNL = runtime.GOOS != "windows"

func rbbNewVT(w, h int) *rbbVT {
	v := &rbbVT{w: w, h: h, grid: make([][]rbbCell, h), mapNL: rbbRendererMapsNL}
	for i := range v.grid {
		v.grid[i] = make([]rbbCell, w)
	}
	return v
}

func (v *rbbVT) blank() []rbbCell { return make([]rbbCell, v.w) }

func (v *rbbVT) rowText(row []rbbCell) string {
	var b strings.Builder
	for _, c := range row {
		switch {
		case c.w < 0:
		case c.w == 0:
			b.WriteByte(' ')
		default:
			b.WriteRune(c.r)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// screen 是屏幕各行的文本；all 是回滚区加屏幕各行。
func (v *rbbVT) screen() []string {
	out := make([]string, v.h)
	for i, row := range v.grid {
		out[i] = v.rowText(row)
	}
	return out
}

func (v *rbbVT) all() []string {
	return append(append([]string(nil), v.back...), v.screen()...)
}

func (v *rbbVT) scrollUp(n int) {
	for i := 0; i < n; i++ {
		v.back = append(v.back, v.rowText(v.grid[0]))
		copy(v.grid, v.grid[1:])
		v.grid[v.h-1] = v.blank()
	}
}

func (v *rbbVT) scrollDown(n int) {
	for i := 0; i < n; i++ {
		copy(v.grid[1:], v.grid[:v.h-1])
		v.grid[0] = v.blank()
	}
}

// lf 只下移不回车；在最底行时整屏上卷一行，顶行文本进回滚区。
func (v *rbbVT) lf() {
	v.wrap = false
	if v.y == v.h-1 {
		v.scrollUp(1)
		return
	}
	v.y++
}

// clearCell 清一格；踩到宽字符的一半时把另一半也清掉。
func (v *rbbVT) clearCell(y, x int) {
	if x < 0 || x >= v.w || y < 0 || y >= v.h {
		return
	}
	row := v.grid[y]
	switch c := row[x]; {
	case c.w < 0 && x > 0:
		row[x-1] = rbbCell{}
	case c.w == 2 && x+1 < v.w:
		row[x+1] = rbbCell{}
	}
	row[x] = rbbCell{}
}

func (v *rbbVT) print(r rune) {
	wd := ansi.StringWidth(string(r))
	if wd <= 0 {
		return
	}
	if v.wrap {
		v.x = 0
		v.lf()
	}
	if wd == 2 && v.x == v.w-1 {
		v.x = 0
		v.lf()
	}
	v.clearCell(v.y, v.x)
	if wd == 2 {
		v.clearCell(v.y, v.x+1)
	}
	v.grid[v.y][v.x] = rbbCell{r: r, w: wd}
	if wd == 2 {
		v.grid[v.y][v.x+1] = rbbCell{w: -1}
	}
	v.last = r
	if nx := v.x + wd; nx > v.w-1 {
		v.x = v.w - 1
		v.wrap = true
	} else {
		v.x = nx
	}
}

func (v *rbbVT) bad(format string, a ...any) {
	v.unknown = append(v.unknown, fmt.Sprintf(format, a...))
}

func (v *rbbVT) c0(b byte) {
	switch b {
	case '\r':
		v.x = 0
		v.wrap = false
	case '\n':
		if v.mapNL {
			v.x = 0
		}
		v.lf()
	case '\b':
		if v.x > 0 {
			v.x--
		}
		v.wrap = false
	case '\t':
		v.x = min(v.w-1, (v.x/8+1)*8)
		v.wrap = false
	}
	// 其余 C0 控制字符忽略。
}

// feed 喂一块字节。块尾没收完的序列或 UTF-8 留到下一块。
func (v *rbbVT) feed(p []byte) {
	buf := append(v.pend, p...)
	v.pend = nil
	i := 0
	for i < len(buf) {
		b := buf[i]
		switch {
		case b == 0x1b:
			n, ok := v.esc(buf[i:])
			if !ok {
				v.pend = append([]byte(nil), buf[i:]...)
				return
			}
			i += n
		case b < 0x20 || b == 0x7f:
			v.c0(b)
			i++
		default:
			if !utf8.FullRune(buf[i:]) {
				v.pend = append([]byte(nil), buf[i:]...)
				return
			}
			r, size := utf8.DecodeRune(buf[i:])
			if r == utf8.RuneError && size <= 1 {
				v.bad("坏的 UTF-8 字节 %#x", b)
				i++
				continue
			}
			v.print(r)
			i += size
		}
	}
}

// esc 处理以 ESC 开头的一段；返回吃掉的字节数，序列不完整时 ok 为假。
func (v *rbbVT) esc(s []byte) (int, bool) {
	if len(s) < 2 {
		return 0, false
	}
	switch s[1] {
	case '[':
		return v.csi(s)
	case ']', 'P', '_', '^':
		for j := 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1, true
			}
			if s[j] == 0x1b {
				if j+1 >= len(s) {
					return 0, false
				}
				if s[j+1] == '\\' {
					return j + 2, true
				}
			}
		}
		return 0, false
	case 'M':
		v.wrap = false
		if v.y == 0 {
			v.scrollDown(1)
		} else {
			v.y--
		}
	case '7':
		v.sx, v.sy = v.x, v.y
	case '8':
		v.x, v.y = v.sx, v.sy
		v.wrap = false
	case '=', '>':
	default:
		v.bad("未知 ESC 序列 %q", string(s[:2]))
	}
	return 2, true
}

// csi 解析 ESC [ 参数 中间字节 终止字节。
func (v *rbbVT) csi(s []byte) (int, bool) {
	j := 2
	for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
		j++
	}
	pEnd := j
	for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
		j++
	}
	if j >= len(s) {
		return 0, false
	}
	final := s[j]
	params := string(s[2:pEnd])
	inter := string(s[pEnd:j])
	if final < 0x40 || final > 0x7e {
		v.bad("坏的 CSI %q", string(s[:j+1]))
		return j + 1, true
	}
	if inter != "" || strings.HasPrefix(params, "?") || strings.HasPrefix(params, ">") || strings.HasPrefix(params, "=") {
		return j + 1, true
	}
	switch final {
	case 'm', 't', 'n', 'c', 'r', 'u', 's':
		return j + 1, true
	}
	var ps []int
	if params != "" {
		for _, f := range strings.Split(params, ";") {
			if f == "" {
				ps = append(ps, 0)
				continue
			}
			n, err := strconv.Atoi(f)
			if err != nil {
				v.bad("CSI 参数不认识 %q", string(s[:j+1]))
				return j + 1, true
			}
			ps = append(ps, n)
		}
	}
	v.doCSI(final, ps, string(s[:j+1]))
	return j + 1, true
}

func (v *rbbVT) doCSI(final byte, ps []int, raw string) {
	arg := func(i, def int) int {
		if i < len(ps) && ps[i] > 0 {
			return ps[i]
		}
		return def
	}
	mode := 0
	if len(ps) > 0 {
		mode = ps[0]
	}
	clampY := func(y int) int { return max(0, min(v.h-1, y)) }
	clampX := func(x int) int { return max(0, min(v.w-1, x)) }
	switch final {
	case 'A':
		v.y = clampY(v.y - arg(0, 1))
		v.wrap = false
	case 'B':
		v.y = clampY(v.y + arg(0, 1))
		v.wrap = false
	case 'C':
		v.x = clampX(v.x + arg(0, 1))
		v.wrap = false
	case 'D':
		v.x = clampX(v.x - arg(0, 1))
		v.wrap = false
	case 'E':
		v.y = clampY(v.y + arg(0, 1))
		v.x = 0
		v.wrap = false
	case 'F':
		v.y = clampY(v.y - arg(0, 1))
		v.x = 0
		v.wrap = false
	case 'G':
		v.x = clampX(arg(0, 1) - 1)
		v.wrap = false
	case 'H', 'f':
		v.y = clampY(arg(0, 1) - 1)
		v.x = clampX(arg(1, 1) - 1)
		v.wrap = false
	case 'd':
		v.y = clampY(arg(0, 1) - 1)
		v.wrap = false
	case 'J':
		switch mode {
		case 0:
			for x := v.x; x < v.w; x++ {
				v.clearCell(v.y, x)
			}
			for y := v.y + 1; y < v.h; y++ {
				v.grid[y] = v.blank()
			}
		case 1:
			for y := 0; y < v.y; y++ {
				v.grid[y] = v.blank()
			}
			for x := 0; x <= v.x; x++ {
				v.clearCell(v.y, x)
			}
		case 2:
			for y := 0; y < v.h; y++ {
				v.grid[y] = v.blank()
			}
		default:
			v.bad("不支持的 ED 模式 %q", raw)
		}
	case 'K':
		switch mode {
		case 0:
			for x := v.x; x < v.w; x++ {
				v.clearCell(v.y, x)
			}
		case 1:
			for x := 0; x <= v.x; x++ {
				v.clearCell(v.y, x)
			}
		case 2:
			v.grid[v.y] = v.blank()
		default:
			v.bad("不支持的 EL 模式 %q", raw)
		}
	case 'L':
		for i := 0; i < arg(0, 1); i++ {
			copy(v.grid[v.y+1:], v.grid[v.y:v.h-1])
			v.grid[v.y] = v.blank()
		}
		v.x = 0
		v.wrap = false
	case 'M':
		for i := 0; i < arg(0, 1); i++ {
			copy(v.grid[v.y:], v.grid[v.y+1:])
			v.grid[v.h-1] = v.blank()
		}
		v.x = 0
		v.wrap = false
	case 'P':
		n := min(arg(0, 1), v.w-v.x)
		row := v.grid[v.y]
		copy(row[v.x:], row[v.x+n:])
		for x := v.w - n; x < v.w; x++ {
			row[x] = rbbCell{}
		}
	case '@':
		n := min(arg(0, 1), v.w-v.x)
		row := v.grid[v.y]
		copy(row[v.x+n:], row[v.x:v.w-n])
		for x := v.x; x < v.x+n; x++ {
			row[x] = rbbCell{}
		}
	case 'X':
		for x := v.x; x < min(v.w, v.x+arg(0, 1)); x++ {
			v.clearCell(v.y, x)
		}
	case 'S':
		v.scrollUp(arg(0, 1))
	case 'T':
		v.scrollDown(arg(0, 1))
	case 'b':
		if v.last != 0 {
			for i := 0; i < arg(0, 1); i++ {
				v.print(v.last)
			}
		}
	default:
		v.bad("未知 CSI %q", raw)
	}
}

// TestRbbVTSelfCheck：模拟器自检（本文件装置的前提，不对应契约条目）。
// 【判别力】对宽字符折行、LF 只下移不回车、mapNL 时 LF 连带回车、IL、ED、CUU、忽略私有 CSI、未知 CSI 计数各给一个已知答案；
// 模拟器写错时 G 系列的读数不可信，这里先红。两种 LF 语义在每个平台上都各测一次，不跟 rbbRendererMapsNL 走。
func TestRbbVTSelfCheck(t *testing.T) {
	v := rbbNewVT(6, 3)
	v.mapNL = false
	e := tui1dESC
	v.feed([]byte("ab\ncd\r\n"))
	if got := v.screen(); got[0] != "ab" || got[1] != "  cd" || v.y != 2 || v.x != 0 {
		t.Errorf("LF 只下移不回车：屏幕 %q 光标 (%d,%d)", got, v.x, v.y)
	}
	v = rbbNewVT(6, 3)
	v.mapNL = true
	v.feed([]byte("ab\ncd"))
	if got := v.screen(); got[0] != "ab" || got[1] != "cd" || v.y != 1 || v.x != 2 {
		t.Errorf("mapNL 时 LF 连带回车：屏幕 %q 光标 (%d,%d)", got, v.x, v.y)
	}
	v = rbbNewVT(6, 3)
	v.feed([]byte("abcde克x"))
	if got := v.screen(); got[0] != "abcde" || got[1] != "克x" {
		t.Errorf("宽字符在 x == W-1 时先折行：屏幕 %q", got)
	}
	v = rbbNewVT(4, 2)
	v.feed([]byte("L1\r\nL2\r\nL3"))
	if len(v.back) != 1 || v.back[0] != "L1" {
		t.Errorf("底行 LF 上卷、顶行进回滚区：回滚区 %q", v.back)
	}
	v.feed([]byte(e + "[A" + e + "[L" + "N"))
	if got := v.screen(); got[0] != "N" || got[1] != "L2" {
		t.Errorf("CUU + IL：屏幕 %q", got)
	}
	v.feed([]byte(e + "[?25l" + e + "[1 q" + e + "]0;t" + string(rune(7)) + e + "[0m"))
	if len(v.unknown) != 0 {
		t.Errorf("私有 CSI、带中间字节的 CSI、OSC、SGR 都应当忽略，实际记错 %q", v.unknown)
	}
	v.feed([]byte(e + "[5Z"))
	if len(v.unknown) != 1 {
		t.Errorf("未知 CSI 终止字节应当记 1 条错误，实际 %q", v.unknown)
	}
	v = rbbNewVT(4, 3)
	v.feed([]byte("aa\r\nbb\r\ncc" + e + "[2;1H" + e + "[J"))
	if got := v.screen(); got[0] != "aa" || got[1] != "" || got[2] != "" {
		t.Errorf("CUP + ED 0：屏幕 %q", got)
	}
}

// ─── 录制器、假 Submitter、spy ─────────────────────────────────────────

// rbbRecorder 是带锁的 io.Writer，每次 Write 存成一块。
type rbbRecorder struct {
	mu     sync.Mutex
	chunks [][]byte
}

func (r *rbbRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chunks = append(r.chunks, append([]byte(nil), p...))
	return len(p), nil
}

func (r *rbbRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.chunks)
}

func (r *rbbRecorder) snapshot() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]byte(nil), r.chunks...)
}

// rbbVTSub 是 §2.5 的假 Submitter。p 在 NewProgram 之后赋值。
type rbbVTSub struct {
	p    *tea.Program
	n    int
	gap  time.Duration
	fail bool

	mu  sync.Mutex
	seq int
}

// line 用 p.Send(printLine) 打一行带全局编号的行。
func (s *rbbVTSub) line(text string) {
	s.mu.Lock()
	s.seq++
	l := fmt.Sprintf("L%04d %s", s.seq, text)
	s.mu.Unlock()
	s.p.Send(printLine(l))
}

func (s *rbbVTSub) Submit(ctx context.Context, text string) error {
	ids := make([]string, s.n)
	for i := range ids {
		ids[i] = "rbb_" + string(rune('a'+i))
	}
	s.line("本轮问")
	obs := newRoundObserver(s.p.Send)
	obs.RoundStart(repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: s.n, Speakers: append([]string(nil), ids...)})
	walls := make([]int, s.n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			ms := 30 + rand.IntN(220)
			time.Sleep(time.Duration(ms) * time.Millisecond)
			mu.Lock()
			defer mu.Unlock()
			walls[i] = ms
			s.line("outcome spoke  agent " + id)
			obs.MemberDone(repl.MemberDoneEvent{Label: "发言轮", AgentID: id, Outcome: turn.OutcomeSpoke, WallMS: ms, RoundID: rbaRID})
		}(i, id)
	}
	wg.Wait()
	s.line("发言轮结算")
	res := make([]repl.MemberDoneEvent, s.n)
	for i, id := range ids {
		res[i] = repl.MemberDoneEvent{Label: "发言轮", AgentID: id, Outcome: turn.OutcomeSpoke, WallMS: walls[i], RoundID: rbaRID}
	}
	obs.RoundEnd(repl.RoundEndEvent{Label: "发言轮", K: 1, N: 1, Results: res})
	time.Sleep(s.gap)
	if s.fail {
		return fmt.Errorf("坏了")
	}
	return nil
}

func (s *rbbVTSub) IsQuit(error) bool { return false }

// rbbVTSpy 包住 Model，记下 submitDone / finalize 进 Update 时录制器的块数（finalize 那个在委托之前记）。
type rbbVTSpy struct {
	m   Model
	rec *rbbRecorder

	mu       sync.Mutex
	doneMark int
	finMark  int
	seenRS   bool
	seenFIN  bool
}

func (s *rbbVTSpy) Init() tea.Cmd  { return s.m.Init() }
func (s *rbbVTSpy) View() tea.View { return s.m.View() }

func (s *rbbVTSpy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case submitDoneMsg:
		n := s.rec.count()
		s.mu.Lock()
		s.doneMark = n
		s.mu.Unlock()
	case boardFinalizeMsg:
		n := s.rec.count()
		s.mu.Lock()
		s.finMark = n
		s.mu.Unlock()
	}
	next, cmd := s.m.Update(msg)
	if nm, ok := next.(Model); ok {
		s.m = nm
	}
	switch msg.(type) {
	case roundStartMsg:
		s.mu.Lock()
		s.seenRS = true
		s.mu.Unlock()
	case boardFinalizeMsg:
		s.mu.Lock()
		s.seenFIN = true
		s.mu.Unlock()
	}
	return s, cmd
}

func (s *rbbVTSpy) flags() (rs, fin bool, finMark int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seenRS, s.seenFIN, s.finMark
}

// ─── 一次跑与回放 ───────────────────────────────────────────────────────

type rbbVTScene struct {
	n      int
	typing bool
	gap    time.Duration
	widen  bool
	fail   bool
}

type rbbVTRun struct {
	chunks  [][]byte
	end     int
	finMark int
	model   Model
	runErr  error
}

// rbbWiden 是只在测试里用的拉宽函数：嵌套的 Sequence 由 execSequenceMsg 同步展开，
// 于是 insertAbove 与 finalize 之间隔开 40ms，不改生产代码。
func rbbWiden(d time.Duration) func(string) tea.Cmd {
	return func(line string) tea.Cmd {
		return tea.Sequence(tea.Println(line), func() tea.Msg { time.Sleep(d); return nil })
	}
}

func rbbWaitUntil(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// rbbVTOnce 按 §2.5「一次跑」起一台真 Program 跑一次，返回录下的块。
func rbbVTOnce(t *testing.T, sc rbbVTScene) rbbVTRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := &rbbRecorder{}
	sub := &rbbVTSub{n: sc.n, gap: sc.gap, fail: sc.fail}
	var pl func(string) tea.Cmd
	if sc.widen {
		pl = rbbWiden(40 * time.Millisecond)
	}
	m := New(Options{Submit: sub, Out: tui1bNopDrainer{}, Version: "t", Ctx: ctx, Println: pl})
	spy := &rbbVTSpy{m: m, rec: rec}
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
	// 无论从哪条路离开，都先叫停打字、取消 ctx、等 Run 返回，不留 goroutine。
	stop := make(chan struct{})
	var stopOnce sync.Once
	typingDone := make(chan struct{})
	halt := func() { stopOnce.Do(func() { close(stop) }) }
	defer func() {
		halt()
		<-typingDone
		cancel()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：cancel 之后 5 秒 Program.Run 仍没有返回")
		}
	}()

	if !rbbWaitUntil(5*time.Second, func() bool { return rec.count() >= 1 }) {
		close(typingDone)
		t.Fatalf("前提不成立：5 秒内录制器一块都没有收到")
	}
	p.Send(tea.PasteMsg{Content: "/round x"})
	p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	if sc.typing {
		go func() {
			defer close(typingDone)
			for {
				if rs, _, _ := spy.flags(); rs {
					break
				}
				select {
				case <-stop:
					return
				case <-time.After(2 * time.Millisecond):
				}
			}
			for k := 0; k < 3; k++ {
				p.Send(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
			}
			tk := time.NewTicker(7 * time.Millisecond)
			defer tk.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tk.C:
					p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
				}
			}
		}()
	} else {
		close(typingDone)
	}

	var run rbbVTRun
	if sc.fail {
		select {
		case err := <-runDone:
			run.runErr = err
			runDone <- err // 留给收尾的 defer 读
		case <-time.After(10 * time.Second):
			t.Fatalf("前提不成立：出错退出的场景 10 秒内 Run 没有自己返回")
		}
		run.chunks = rec.snapshot()
		run.end = len(run.chunks)
		run.model = spy.m
		_, _, run.finMark = spy.flags()
		return run
	}

	if !rbbWaitUntil(10*time.Second, func() bool { _, fin, _ := spy.flags(); return fin }) {
		t.Fatalf("前提不成立：10 秒内没有见到 boardFinalizeMsg")
	}
	time.Sleep(250 * time.Millisecond)
	halt()
	<-typingDone
	time.Sleep(100 * time.Millisecond)
	run.end = rec.count()
	cancel()
	select {
	case err := <-runDone:
		runDone <- err
	case <-time.After(5 * time.Second):
		t.Fatalf("cancel 之后 5 秒 Program.Run 没有返回")
	}
	run.chunks = rec.snapshot()
	_, _, run.finMark = spy.flags()
	return run
}

var rbbLMarkRe = regexp.MustCompile(`^L(\d{4})`)

// rbbVTReading 是一次回放的两种读数。
type rbbVTReading struct {
	screenRes  int
	formulaRes int
	formulaOK  bool
	summaryOK  bool // 「/round 」开头的行恰好 1 行且整行匹配
	problems   []string
	observable bool // G1/G2 前提：收尾行 insertAbove 那一块与 finMark 之间还有别的块
	hasSummary bool // 原始块里有收尾行
	all        []string
}

// rbbReplay 回放一次跑，按 §2.5 读屏幕与公式。
func rbbReplay(run rbbVTRun, n int) rbbVTReading {
	var rd rbbVTReading
	vt := rbbNewVT(80, 24)
	for i := 1; i <= 30; i++ {
		vt.feed([]byte(fmt.Sprintf("S%02d\r\n", i)))
	}
	ed := []byte(tui1dESC + "[J")
	shrink := -1
	for i := run.finMark; i < run.end; i++ {
		if bytes.Contains(run.chunks[i], ed) {
			shrink = i
			break
		}
	}
	for i := 0; i < run.end; i++ {
		if i == shrink {
			hdr := -1
			for y, ln := range vt.screen() {
				if strings.HasPrefix(ln, "council t   ") {
					hdr = y
				}
			}
			if hdr >= 0 {
				rd.formulaRes = max(0, vt.y-hdr-7)
				rd.formulaOK = true
			}
		}
		vt.feed(run.chunks[i])
	}
	if !rd.formulaOK {
		rd.problems = append(rd.problems, fmt.Sprintf("前提不成立：找不到缩高块（finMark=%d 之后含 ESC[J 的块）或其之前屏幕上没有表头", run.finMark))
	}

	// 原始块里的收尾行位置（G1/G2 前提）。
	sumIdx := -1
	for i := 0; i < run.end; i++ {
		if bytes.Contains(run.chunks[i], []byte("/round 结束")) {
			sumIdx = i
			break
		}
	}
	rd.hasSummary = sumIdx >= 0
	rd.observable = sumIdx >= 0 && run.finMark-sumIdx >= 2

	all := vt.all()
	rd.all = all
	sumRe := regexp.MustCompile(fmt.Sprintf(`^/round 结束  \d+s  回来 %d/%d$`, n, n))
	s, sCount, h, hCount := -1, 0, -1, 0
	for i, ln := range all {
		if strings.HasPrefix(ln, "/round ") {
			sCount++
			s = i
		}
		if strings.HasPrefix(ln, "council t   ") {
			hCount++
			h = i
		}
	}
	rd.summaryOK = sCount == 1 && sumRe.MatchString(all[s])
	if !rd.summaryOK {
		rd.problems = append(rd.problems, fmt.Sprintf("以「/round 」开头的行应当恰好 1 行且整行匹配 %s，实际 %d 行", sumRe, sCount))
	}
	if hCount != 1 {
		rd.problems = append(rd.problems, fmt.Sprintf("以「council t   」开头的行应当恰好 1 行，实际 %d 行", hCount))
	}
	// 屏幕残影 = h - s - 1。表头不止 1 行时（旧帧顶留在屏幕上）上面已经记了错；
	// 这里 h 取最后一个表头行，照样算出行数，失败信息里看得到残影几行。
	if s >= 0 && h >= 0 {
		if h <= s {
			rd.problems = append(rd.problems, fmt.Sprintf("表头（第 %d 行）应当在收尾行（第 %d 行）之下", h, s))
		}
		rd.screenRes = h - s - 1
	} else {
		rd.screenRes = -1
	}
	want := 1
	for _, ln := range all {
		if m := rbbLMarkRe.FindStringSubmatch(ln); m != nil {
			if got, _ := strconv.Atoi(m[1]); got != want {
				rd.problems = append(rd.problems, fmt.Sprintf("L 编号应当从 1 起连续，第 %d 个是 L%04d", want, got))
				break
			}
			want++
		}
	}
	if want-1 != n+2 {
		// 1 行本轮问 + n 行 outcome + 1 行结算。
		rd.problems = append(rd.problems, fmt.Sprintf("L 行应当恰好 %d 行，实际 %d 行", n+2, want-1))
	}
	if len(vt.unknown) != 0 {
		rd.problems = append(rd.problems, fmt.Sprintf("模拟器错误（未知控制序列）%d 条: %q", len(vt.unknown), vt.unknown))
	}
	return rd
}

// rbbRunScene 跑一个场景 times 次，逐次断言；needObservable 时在逐次主断言之后统计窗口可观测次数。
func rbbRunScene(t *testing.T, name string, sc rbbVTScene, times int, needObservable bool) {
	t.Helper()
	withSummary, observable := 0, 0
	for k := 0; k < times; k++ {
		run := rbbVTOnce(t, sc)
		rd := rbbReplay(run, sc.n)
		t.Logf("%s 第 %d 次：屏幕残影 %d，公式残影 %d（公式可读=%v），收尾行在原始块里=%v，窗口可观测=%v，块数 %d，finMark %d",
			name, k+1, rd.screenRes, rd.formulaRes, rd.formulaOK, rd.hasSummary, rd.observable, run.end, run.finMark)
		for _, p := range rd.problems {
			t.Errorf("%s 第 %d 次：%s", name, k+1, p)
		}
		if rd.screenRes != 0 {
			t.Errorf("%s 第 %d 次：屏幕残影应当是 0 行，实际 %d", name, k+1, rd.screenRes)
		}
		if rd.formulaOK && rd.formulaRes != 0 {
			t.Errorf("%s 第 %d 次：公式残影应当是 0 行，实际 %d", name, k+1, rd.formulaRes)
		}
		if t.Failed() && k == 0 {
			t.Logf("%s 第 1 次回放后的全部行（末 40 行）:\n%s", name, strings.Join(rbbTailLines(rd.all, 40), "\n"))
		}
		if rd.hasSummary {
			withSummary++
			if rd.observable {
				observable++
			}
		}
	}
	if needObservable && observable < 8 {
		t.Fatalf("%s 前提不成立：收尾行存在的 %d 次里，收尾行 insertAbove 与 finalize 之间还有 flush 的只有 %d 次（< 8），"+
			"「去掉光标钉」在这个场景里观测不到", name, withSummary, observable)
	}
}

func rbbTailLines(ls []string, n int) []string {
	if len(ls) > n {
		return ls[len(ls)-n:]
	}
	return ls
}

// ═══ G1-G3 ════════════════════════════════════════════════════════════

// TestRbbG1TypingThroughShrink：4 人、缩高前后一直打字、gap 50ms、拉宽 40ms，跑 10 次。
//
// 【判别力】去掉光标钉（#46、#66）→ 拉宽状态下残影 2（原型 10/10）；去掉收尾行（#50）或收尾行为空串（#51）→
// 「收尾行恰好 1 行」红。前提（窗口可观测 ≥ 8/10）放在逐次主断言之后，与光标钉无关。对照：G3（自然时序）。
func TestRbbG1TypingThroughShrink(t *testing.T) {
	rbbRunScene(t, "G1", rbbVTScene{n: 4, typing: true, gap: 50 * time.Millisecond, widen: true}, 10, true)
}

// TestRbbG2EightRowsWidened：8 人、不打字、gap 50ms、拉宽 40ms，跑 10 次。
//
// 【判别力】同 G1；8 行看板时每次 flush 都重画带宽字符的看板行，光标停在最后一个宽字符行上，不钉光标必残影。
func TestRbbG2EightRowsWidened(t *testing.T) {
	rbbRunScene(t, "G2", rbbVTScene{n: 8, gap: 50 * time.Millisecond, widen: true}, 10, true)
}

// TestRbbG3EightRowsNatural：8 人、不打字、gap 50ms、不拉宽，跑 10 次。
//
// 【判别力】去掉收尾行（#50）→ 没有收尾行、公式残影 2；收尾行为空串（#51）→ 收尾行缺失。
// 去掉光标钉在自然时序下抓不到（窗口里几乎没有 flush），这正是 G1/G2 要拉宽的原因。
func TestRbbG3EightRowsNatural(t *testing.T) {
	rbbRunScene(t, "G3", rbbVTScene{n: 8, gap: 50 * time.Millisecond}, 10, false)
}

// ═══ G4 ═══════════════════════════════════════════════════════════════

// TestRbbG4ErrorExitClearsBoard：8 人、Submit 在 RoundEnd 与 gap 之后返回错误；出错退出时看板在场。
//
// 【判别力】err 支清看板（#55）→ 帧高 8，seq 变成上移 7 行，整串不等，且屏幕留下看板行；
// 看板行只在 View、不在 chrome（#62）→ frameHeight 只数 8 行，seq 不等、屏幕留表头。跑 5 次。
// Linux 上实测（2026-09-25）：#55 只由 seq 的整串比较杀掉，回放后的屏幕是干净的（渲染器在那次 Update
// 里自己把帧缩了），上面「屏幕留下看板行」在 Linux 上不成立；Windows 上没复核。#62 三条断言都红，与上面一致。
// 对照：X-39e A3（没有看板时同一出错路径清 8 行）。
func TestRbbG4ErrorExitClearsBoard(t *testing.T) {
	want := tui1dESC + "[15A\r" + tui1dESC + "[J"
	for k := 0; k < 5; k++ {
		run := rbbVTOnce(t, rbbVTScene{n: 8, gap: 50 * time.Millisecond, fail: true})
		if run.runErr != nil {
			t.Fatalf("G4 第 %d 次：前提不成立：出错退出走 QuitMsg，Run 应当返回 nil，实际 %v", k+1, run.runErr)
		}
		seq := exitClearSeq(run.model, run.runErr)
		if seq != want {
			t.Errorf("G4 第 %d 次：exitClearSeq 应当整串等于 %q（帧高 16 = 2 + 8 + 6），实际 %q", k+1, want, seq)
		}
		vt := rbbNewVT(80, 24)
		for i := 1; i <= 30; i++ {
			vt.feed([]byte(fmt.Sprintf("S%02d\r\n", i)))
		}
		for _, c := range run.chunks {
			vt.feed(c)
		}
		vt.feed([]byte(seq))
		all := vt.all()
		bad, errAt, errCount := 0, -1, 0
		for i, ln := range all {
			if strings.HasPrefix(ln, "council t") || strings.HasPrefix(ln, "─") {
				bad++
			}
			if ln == "坏了" {
				errCount++
				errAt = i
			}
		}
		if bad != 0 {
			t.Errorf("G4 第 %d 次：清屏之后全部行里不应当有以「council t」或「─」开头的行，实际 %d 行", k+1, bad)
		}
		if errCount != 1 {
			t.Errorf("G4 第 %d 次：「坏了」应当恰好 1 行，实际 %d 行", k+1, errCount)
		} else {
			for i := errAt + 1; i < len(all); i++ {
				if all[i] != "" {
					t.Errorf("G4 第 %d 次：「坏了」之下的屏幕行应当全是空行，第 %d 行是 %q", k+1, i, all[i])
					break
				}
			}
		}
		if len(vt.unknown) != 0 {
			t.Errorf("G4 第 %d 次：模拟器错误（未知控制序列）%q", k+1, vt.unknown)
		}
		if t.Failed() {
			t.Logf("G4 第 %d 次回放后的全部行（末 30 行）:\n%s", k+1, strings.Join(rbbTailLines(all, 30), "\n"))
			return
		}
	}
}
