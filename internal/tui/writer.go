// Package tui 是 Council 的终端界面层。
//
// LineWriter 是 REPL 输出漏斗上的行缓冲。全部输出都写进注入的 io.Writer，
// 但渲染一条记录要连打好几次 printf：一行可能由多次 Write 拼成，一次 Write
// 也可能含多行。内联模式里定稿的行要整行推进终端原生回滚区（tea.Println 的
// 单位是完整的行）。把每次 Write 原样交出去，屏幕上会看到一行被劈成好几段。
// 这一层攒到换行才吐，吐出的是不含结尾换行的完整行。
//
// 三件写错就难复现的事也在这里：
//   - 并发写。调用方未必串行，这一层自己保证一次 Write 的字节不被别的 Write 撕开。
//   - 退出时没有换行的残段。它还在缓冲里、没交给任何出口；不在退出时 Flush 出去
//     （quitCmd 交给事件循环打印，Run 返回后 runWith 第 4 步交给 final），进程一退就永久丢失。
//   - Program 还没起来就有人写。启动横幅写在 repl.New(...).Run(ctx) 之前。
//
// 本文件只用标准库，不 import bubbletea。和终端的耦合是一个 EmitFunc 函数值。
package tui

import (
	"bytes"
	"sync"
)

// EmitFunc 把一行完整的、不含结尾换行的文本交出去。
// 实现方通常是 (*tea.Program).Send 的一层包装。
//
// 本包不 import bubbletea：这一层要能在没有终端的情况下单独测，
// 而耦合成类型就必须把整个 TUI 拖进测试。
type EmitFunc func(line string)

// LineWriter 是 REPL 的 io.Writer 到「一次一整行」的适配器。
// 只切行，不截断、不量宽度、不加前缀。
//
// 必须按指针使用。复制该值会把 mu 一起复制。
type LineWriter struct {
	mu sync.Mutex

	// emit 为 nil 时完整行进 pending，残段留在 buf，都不调用出口。
	emit EmitFunc

	// buf 是还没见过 '\n' 的残段。
	buf []byte

	// pending 是 emit 为 nil 期间已经见过换行的完整行，顺序与写入顺序一致。
	pending []string
}

// NewLineWriter 造一个 LineWriter。emit 为 nil 时写进来的东西只攒不吐，
// 直到 SetEmit 装上真正的出口。
func NewLineWriter(emit EmitFunc) *LineWriter {
	return &LineWriter{emit: emit}
}

// SetEmit 换掉出口。换上非 nil 的出口时，把此前攒下的完整行按原顺序补吐。
// 残段留在 buf 里，等后续 Write 或 Flush。
//
// 启动横幅写在 repl.New(...).Run(ctx) 之前，那时 Program 还没造出来，出口仍是
// nil。这些已经见过换行的行记在 pending 里，在这里补吐。丢掉它们，启动时
// 这几条提示就没有了。
func (w *LineWriter) SetEmit(emit EmitFunc) {
	lines := w.swapEmit(emit)
	emitLines(emit, lines)
}

// Write 实现 io.Writer。
// 返回值恒为 (len(p), nil)，是本次写入的长度，不是累计缓冲长度。
// 这一层不制造 IO 错误：调用方会把写输出失败当成致命错误终止 REPL，
// 屏幕上的一次投递不该杀掉会话。
func (w *LineWriter) Write(p []byte) (int, error) {
	lines, fn := w.ingest(p)
	emitLines(fn, lines)
	return len(p), nil
}

// Flush 把缓冲里剩下的、不以换行结尾的残段作为一行吐出去。
// 缓冲为空时什么都不做。
//
// 出口仍是 nil 时不调用 emit，也不丢：残段改记入 pending，缓冲同样清空，
// 等 SetEmit 装上出口再补吐。这样连续两次 Flush 不会造出两份残段。
//
// 残段以 '\r' 结尾时不剥。剥除规则只在切行处生效（见 splitLocked）。
// 这是一处有意的不对称，裁决理由：这条路径今天通过产品走不到 —— REPL 不产生
// 以 '\r' 结尾且不带换行的输出 —— 而让 Flush 也剥，就要引入第二条剥除规则，
// 两条规则迟早会分叉。将来若真出现这种输出，改的是这里，不是 splitLocked。
func (w *LineWriter) Flush() {
	line, fn, ok := w.detachResidual()
	if !ok {
		return
	}
	emitLines(fn, []string{line})
}

// swapEmit 换出口，并在出口非 nil 时取走 pending。调用期间持有 mu，返回后已放开。
func (w *LineWriter) swapEmit(emit EmitFunc) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.emit = emit
	if emit == nil || len(w.pending) == 0 {
		return nil
	}
	lines := w.pending
	w.pending = nil
	return lines
}

// ingest 把 p 接上残段并切出已经见过换行的行。调用期间持有 mu，返回后已放开。
// 一次 Write 的拼接和切行在同一把锁里完成，别的 goroutine 不能把字节插进这一次的中间。
func (w *LineWriter) ingest(p []byte) (lines []string, fn EmitFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	lines = w.splitLocked()
	fn = w.emit
	if fn == nil {
		w.pending = append(w.pending, lines...)
		lines = nil
	}
	return lines, fn
}

// detachResidual 取走残段。buf 为空时 ok 为 false。
// 出口为 nil 时把残段移入 pending 并让 ok 为 false，调用方不发射。
// 调用期间持有 mu，返回后已放开。
func (w *LineWriter) detachResidual() (line string, fn EmitFunc, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return "", nil, false
	}
	// 残段还没被 '\n' 切开，不适用「\r\n 里的 \r 要剥」那条，字节原样成为一行。
	line = string(w.buf)
	w.buf = nil
	fn = w.emit
	if fn == nil {
		w.pending = append(w.pending, line)
		return "", nil, false
	}
	return line, fn, true
}

// splitLocked 把 buf 里以 '\n' 结束的部分切成行。调用方必须持有 mu。
// 切行只认 '\n'。紧挨在这个 '\n' 前面的一个 '\r' 剥掉，行中间的 '\r' 保留。
func (w *LineWriter) splitLocked() []string {
	var lines []string
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		lines = append(lines, cutTrailingCR(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) == 0 {
		w.buf = nil
	}
	return lines
}

// cutTrailingCR 去掉切行后行尾的那一个 '\r'。line 不含 '\n'。
func cutTrailingCR(line []byte) string {
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	return string(line)
}

// emitLines 逐行调用 fn。全文件只有这里会调用出口。
// 调用方必须已经放开 LineWriter.mu。这里故意不接收 *LineWriter，避免再摸到锁。
//
// 不能在持锁时调用出口。死锁有两条路，都来自 emit 的实现 (*tea.Program).Send：
//
// Send 的本体是
//
//	select {
//	case <-p.ctx.Done():
//	case p.msgs <- msg:
//	}
//
// p.msgs 无缓冲。Run 之前 Send 会一直堵到事件循环开始消费，在同一个 goroutine 上这么调会永远堵住 —— TUI-1d 修掉的正是这个死锁；Run 结束之后 ctx
// 已 Done，Send 是空操作。事件循环正停在 Update 里时没人来取 p.msgs，同步 Send
// 自己就会堵死。
//
// 路一，跨 goroutine。Write 拿着 mu 调用 Send，Send 堵在 p.msgs 上。事件循环要
// 把这条消息取走才能继续，而它继续下去之前可能还在等另一个 Write 的结果（输出
// 不写完，这一帧就组织不起来）。那个 Write 要 mu。mu 在已经堵死的 Send 手里。
// 四个参与者绕一圈：持锁的 Write 等 Send，Send 等事件循环，事件循环等另一个
// Write，另一个 Write 等 mu。
//
// 路二，同 goroutine 重入。sync.Mutex 不可重入。出口回调里再调 Write 时，外层
// Write 如果还拿着 mu，内层 Lock 永远等不到。验收就是这条：emit 里再
// Write([]byte("re\n"))，坏实现会挂住，好实现必须返回。
//
// 所以切行、换出口、取残段都在各自的持锁函数里做完；那些函数用 defer 放锁，
// 返回之后才进入 emitLines。不能把 emitLines 挪进持锁函数，也不能在那些函数里
// defer 放锁的同时又在 return 之前调用它：defer 要等函数返回才执行，出口会落回
// 持锁区间。出口自己 panic 时，锁也已经在进 emitLines 之前放开，不会把 mu 留在
// 锁住的状态。
func emitLines(fn EmitFunc, lines []string) {
	if fn == nil || len(lines) == 0 {
		return
	}
	for _, line := range lines {
		fn(line)
	}
}
