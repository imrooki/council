// 本文件是 TUI 第 1 刀之一（输出漏斗 / 行缓冲 writer）的背对背测试。
// 覆盖的是冻结契约 E:\multi_model_cowork\tui1a_writer_contract.md 第 5 节
// 的全部 12 条验收，不多不少。测试只依据契约文本编写，不读、不依赖同目录下
// writer.go 的源码——实现腿正在并行写它，两条腿互相看不见。
package tui

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// deadlockDetectTimeout 是验收 11（a/b/c）用来判定"疑似死锁"的等待上限。
//
// 选值考虑：好实现下，emit 是在解锁之后才被调用的，emit 回调里再入 w.Write
// 拿到的是一把当下空闲的锁，整条再入链路是纯 CPU 操作，正常应在微秒级完成，
// 2 秒相对真实耗时有几个数量级的余量，不会因为测试机负载、调度抖动而偶发超时；
// 坏实现（持锁调用 emit）下，再入的 Lock() 会永久阻塞该 goroutine，2 秒后
// 必然命中超时分支——一旦命中，唯一合理的解释就是死锁，不是"跑得慢"。
const deadlockDetectTimeout = 2 * time.Second

// ===== 验收 1 =====
// Write([]byte("abc\n")) -> emit 恰好一次，收到 "abc"（不含换行）。
func TestAcceptance1_SingleLineWithNewline(t *testing.T) {
	var got []string
	w := NewLineWriter(func(line string) {
		got = append(got, line)
	})

	n, err := w.Write([]byte("abc\n"))
	if err != nil {
		t.Fatalf("Write 返回了非 nil 的 error: %v", err)
	}
	if n != len("abc\n") {
		t.Fatalf("Write 返回的 n 应为 %d，实际 %d", len("abc\n"), n)
	}
	if len(got) != 1 {
		t.Fatalf("emit 应被调用恰好一次，实际 %d 次：%v", len(got), got)
	}
	if got[0] != "abc" {
		t.Fatalf("emit 收到的内容应为 %q（不含换行），实际 %q", "abc", got[0])
	}
}

// ===== 验收 2 =====
// Write([]byte("a\nb\nc\n")) -> emit 三次，依次 "a" "b" "c"。
func TestAcceptance2_MultipleLinesInOneWrite(t *testing.T) {
	var got []string
	w := NewLineWriter(func(line string) {
		got = append(got, line)
	})

	if _, err := w.Write([]byte("a\nb\nc\n")); err != nil {
		t.Fatalf("Write 返回了非 nil 的 error: %v", err)
	}

	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("emit 应被调用 %d 次，实际 %d 次：%v", len(want), len(got), got)
	}
	for i, wantLine := range want {
		if got[i] != wantLine {
			t.Fatalf("第 %d 次 emit 应收到 %q，实际 %q（完整序列 %v）", i, wantLine, got[i], got)
		}
	}
}

// ===== 验收 3 =====
// Write([]byte("ab")) 之后 emit 零次；再 Write([]byte("c\n")) -> emit 一次，收到 "abc"。
func TestAcceptance3_PartialThenComplete(t *testing.T) {
	var got []string
	w := NewLineWriter(func(line string) {
		got = append(got, line)
	})

	n1, err1 := w.Write([]byte("ab"))
	if err1 != nil {
		t.Fatalf("第一次 Write 返回了非 nil 的 error: %v", err1)
	}
	if n1 != 2 {
		t.Fatalf("第一次 Write 返回的 n 应为 2，实际 %d", n1)
	}
	if len(got) != 0 {
		t.Fatalf("不含换行时不应触发 emit，实际已调用 %d 次：%v", len(got), got)
	}

	n2, err2 := w.Write([]byte("c\n"))
	if err2 != nil {
		t.Fatalf("第二次 Write 返回了非 nil 的 error: %v", err2)
	}
	if n2 != 2 {
		t.Fatalf("第二次 Write 返回的 n 应为 2（即该次 p 的长度，不是累计缓冲长度），实际 %d", n2)
	}
	if len(got) != 1 {
		t.Fatalf("接上换行后应触发 emit 恰好一次，实际 %d 次：%v", len(got), got)
	}
	if got[0] != "abc" {
		t.Fatalf("emit 收到的内容应为跨两次 Write 拼接后的 %q，实际 %q", "abc", got[0])
	}
}

// ===== 验收 4 =====
// Write([]byte("\n")) -> emit 一次，收到 ""（空行要吐，不许吞）。
func TestAcceptance4_EmptyLineIsEmitted(t *testing.T) {
	var got []string
	w := NewLineWriter(func(line string) {
		got = append(got, line)
	})

	if _, err := w.Write([]byte("\n")); err != nil {
		t.Fatalf("Write 返回了非 nil 的 error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("emit 应被调用恰好一次，实际 %d 次：%v", len(got), got)
	}
	if got[0] != "" {
		t.Fatalf("emit 收到的空行应为空字符串，实际 %q", got[0])
	}
}

// ===== 验收 5 =====
// 行尾 \r\n 的 \r 要在吐出去之前剥掉；行中间的 \r 不动。
func TestAcceptance5_CarriageReturnHandling(t *testing.T) {
	t.Run("行尾\\r被剥掉", func(t *testing.T) {
		var got []string
		w := NewLineWriter(func(line string) {
			got = append(got, line)
		})
		if _, err := w.Write([]byte("a\r\n")); err != nil {
			t.Fatalf("Write 返回了非 nil 的 error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("emit 应被调用恰好一次，实际 %d 次：%v", len(got), got)
		}
		if got[0] != "a" {
			t.Fatalf("行尾的 \\r 应被剥掉，emit 应收到 %q，实际 %q", "a", got[0])
		}
	})

	t.Run("行中间\\r不动", func(t *testing.T) {
		var got []string
		w := NewLineWriter(func(line string) {
			got = append(got, line)
		})
		if _, err := w.Write([]byte("a\rb\n")); err != nil {
			t.Fatalf("Write 返回了非 nil 的 error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("emit 应被调用恰好一次，实际 %d 次：%v", len(got), got)
		}
		if got[0] != "a\rb" {
			t.Fatalf("行中间的 \\r 不应被动，emit 应收到 %q，实际 %q", "a\rb", got[0])
		}
	})
}

// ===== 验收 6 =====
// Write 的返回值永远是 (len(p), nil)。
//
// 契约原文这句不太通顺："即使 emit 为 nil、即使 emit 自己 panic 之外的任何情况"。
// 按上下文理解为：emit 自身 panic 的情况不在保证范围内（未定义行为，本测试不覆盖），
// 除此之外（包括 emit 为 nil）都必须是 (len(p), nil)。这个理解记在报告第 7 条。
func TestAcceptance6_WriteReturnValueAlwaysFullLength(t *testing.T) {
	t.Run("emit为nil的空写入", func(t *testing.T) {
		w := NewLineWriter(nil)
		n, err := w.Write([]byte{})
		if err != nil {
			t.Fatalf("空写入应返回 nil error，实际 %v", err)
		}
		if n != 0 {
			t.Fatalf("空写入应返回 n=0，实际 %d", n)
		}
	})

	t.Run("正常单行", func(t *testing.T) {
		w := NewLineWriter(func(string) {})
		p := []byte("abc\n")
		n, err := w.Write(p)
		if err != nil {
			t.Fatalf("应返回 nil error，实际 %v", err)
		}
		if n != len(p) {
			t.Fatalf("应返回 n=%d，实际 %d", len(p), n)
		}
	})

	t.Run("多行一次写入", func(t *testing.T) {
		w := NewLineWriter(func(string) {})
		p := []byte("x\ny\nz\n")
		n, err := w.Write(p)
		if err != nil {
			t.Fatalf("应返回 nil error，实际 %v", err)
		}
		if n != len(p) {
			t.Fatalf("应返回 n=%d，实际 %d", len(p), n)
		}
	})

	t.Run("emit为nil的多行写入不panic不丢", func(t *testing.T) {
		w := NewLineWriter(nil)
		p := []byte("x\ny\n")
		n, err := w.Write(p)
		if err != nil {
			t.Fatalf("应返回 nil error，实际 %v", err)
		}
		if n != len(p) {
			t.Fatalf("应返回 n=%d，实际 %d", len(p), n)
		}
	})

	t.Run("连续两次Write各自返回本次长度而非累计缓冲长度", func(t *testing.T) {
		w := NewLineWriter(func(string) {})
		n1, err1 := w.Write([]byte("ab"))
		if err1 != nil {
			t.Fatalf("第一次 Write 应返回 nil error，实际 %v", err1)
		}
		if n1 != 2 {
			t.Fatalf("第一次 Write 应返回 n=2，实际 %d", n1)
		}
		n2, err2 := w.Write([]byte("c\n"))
		if err2 != nil {
			t.Fatalf("第二次 Write 应返回 nil error，实际 %v", err2)
		}
		if n2 != 2 {
			t.Fatalf("第二次 Write 应返回该次 p 的长度 2，不是累计缓冲长度 3，实际 %d", n2)
		}
	})
}

// ===== 验收 7 =====
// Write([]byte("残段")) 之后 Flush() -> emit 一次，收到 "残段"；
// 再 Flush() -> emit 不再被调用。
func TestAcceptance7_FlushEmitsResidualOnce(t *testing.T) {
	var got []string
	w := NewLineWriter(func(line string) {
		got = append(got, line)
	})

	if _, err := w.Write([]byte("残段")); err != nil {
		t.Fatalf("Write 返回了非 nil 的 error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("不含换行的残段不应触发 emit，实际已调用 %d 次：%v", len(got), got)
	}

	w.Flush()
	if len(got) != 1 {
		t.Fatalf("Flush 后应恰好吐一次，实际 %d 次：%v", len(got), got)
	}
	if got[0] != "残段" {
		t.Fatalf("Flush 吐出的内容应为 %q，实际 %q", "残段", got[0])
	}

	w.Flush()
	if len(got) != 1 {
		t.Fatalf("缓冲已空时再 Flush 不应再触发 emit，实际累计 %d 次：%v", len(got), got)
	}
}

// ===== 验收 8 =====
// 缓冲为空时 Flush() -> emit 零次（不许吐一个空行）。
func TestAcceptance8_FlushOnEmptyBufferEmitsNothing(t *testing.T) {
	var got []string
	w := NewLineWriter(func(line string) {
		got = append(got, line)
	})

	w.Flush()
	if len(got) != 0 {
		t.Fatalf("从未写入时 Flush 不应触发 emit，实际 %d 次：%v", len(got), got)
	}
}

// ===== 验收 9 =====
// emit 为 nil 时 Write([]byte("x\ny\n")) 不 panic、不丢；
// 随后 SetEmit(f) -> f 被调用两次，依次 "x" "y"。
func TestAcceptance9_NilEmitBuffersThenBackfillsCompleteLines(t *testing.T) {
	w := NewLineWriter(nil)

	n, err := w.Write([]byte("x\ny\n"))
	if err != nil {
		t.Fatalf("emit 为 nil 时 Write 返回了非 nil 的 error: %v", err)
	}
	if n != len("x\ny\n") {
		t.Fatalf("emit 为 nil 时 Write 返回的 n 应为 %d，实际 %d", len("x\ny\n"), n)
	}

	var got []string
	w.SetEmit(func(line string) {
		got = append(got, line)
	})

	want := []string{"x", "y"}
	if len(got) != len(want) {
		t.Fatalf("SetEmit 装上出口后应补吐 %d 行，实际 %d 行：%v", len(want), len(got), got)
	}
	for i, wantLine := range want {
		if got[i] != wantLine {
			t.Fatalf("补吐第 %d 行应为 %q，实际 %q（完整序列 %v）", i, wantLine, got[i], got)
		}
	}
}

// ===== 验收 10 =====
// emit 为 nil 时 Write([]byte("x\n残段"))，随后 SetEmit(f) -> f 被调用一次（"x"）；
// 再 Flush() -> f 再被调用一次（"残段"）。
func TestAcceptance10_NilEmitPartialLineBackfillThenFlush(t *testing.T) {
	w := NewLineWriter(nil)

	if _, err := w.Write([]byte("x\n残段")); err != nil {
		t.Fatalf("emit 为 nil 时 Write 返回了非 nil 的 error: %v", err)
	}

	var got []string
	w.SetEmit(func(line string) {
		got = append(got, line)
	})

	if len(got) != 1 {
		t.Fatalf("SetEmit 装上出口后应只补吐已见过换行的完整行（1 行），实际 %d 行：%v", len(got), got)
	}
	if got[0] != "x" {
		t.Fatalf("补吐的一行应为 %q，实际 %q", "x", got[0])
	}

	w.Flush()
	if len(got) != 2 {
		t.Fatalf("Flush 后应再吐一行残段，累计应为 2 行，实际 %d 行：%v", len(got), got)
	}
	if got[1] != "残段" {
		t.Fatalf("Flush 吐出的残段应为 %q，实际 %q", "残段", got[1])
	}
}

// ===== 验收 11（a：从 Write 触发的 emit 再入 Write） =====
// 这是本刀唯一能抓到"持锁调用 emit 导致死锁"这个真实 bug 的用例之一（另两条见
// 11b、11c，覆盖契约第 6 条点名的 Write、Flush、SetEmit 三条路径，理由见报告第 4 条）。
//
// 坏实现（Write 内部持锁调用 emit）下，emit 回调里再调一次 w.Write 会尝试对同一把
// sync.Mutex 再次 Lock，Go 的 sync.Mutex 不可重入，会永久阻塞该 goroutine；
// 好实现（解锁后再调用 emit）下，这次再入拿到的是一把当下空闲的锁，整条链路是纯
// CPU 操作，正常几微秒内完成。
//
// 用"另起 goroutine 跑触发链路 + 主测试 goroutine 用 select/time.After 判超时"
// 把"卡死"翻译成一条干净的 FAIL，而不是让 go test 挂到外部 -timeout 才被杀：
// 超时分支里调用的是 t.Fatal（判红），不是 t.Log 之后放过。deadlockDetectTimeout
// 的取值理由见其定义处的注释。
func TestAcceptance11a_EmitNotUnderLock_ReenterFromWrite(t *testing.T) {
	var w *LineWriter
	var mu sync.Mutex
	var got []string
	reentered := make(chan struct{})

	emit := func(line string) {
		mu.Lock()
		got = append(got, line)
		mu.Unlock()
		if line == "outer" {
			// 验收 11 / 契约第 6 条的核心动作：在 emit 回调里再调一次 w.Write。
			// 好实现下这里不会阻塞；坏实现（持锁调用 emit）下这里会永久卡住。
			w.Write([]byte("re\n"))
			close(reentered)
		}
	}
	w = NewLineWriter(emit)

	done := make(chan struct{})
	go func() {
		w.Write([]byte("outer\n"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(deadlockDetectTimeout):
		t.Fatal("Write 未在超时内返回：emit 回调里再入 Write 疑似死锁（怀疑 Write 持锁调用了 emit）")
	}

	select {
	case <-reentered:
	case <-time.After(deadlockDetectTimeout):
		t.Fatal("emit 回调里的再入 Write 未完成：疑似死锁")
	}

	// 以下是链路是否按预期顺序走通的辅助核对，判别力弱于上面的超时检测
	// （见报告第 4 条：真正抓 bug 的是超时分支，这里只是确认没有静默吞掉）。
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("emit 应总共被调用两次（outer 与 re），实际 %d 次：%v", len(got), got)
	}
	if got[0] != "outer" || got[1] != "re" {
		t.Fatalf("emit 收到的内容应依次为 [\"outer\" \"re\"]，实际 %v", got)
	}
}

// ===== 验收 11（b：从 Flush 触发的 emit 再入 Write） =====
// 验证 Flush 吐残段时同样不持锁调用 emit。超时判定的理由同 11a，不重复。
func TestAcceptance11b_EmitNotUnderLock_ReenterFromFlush(t *testing.T) {
	var w *LineWriter
	var mu sync.Mutex
	var got []string
	reentered := make(chan struct{})

	emit := func(line string) {
		mu.Lock()
		got = append(got, line)
		mu.Unlock()
		if line == "resid" {
			w.Write([]byte("re\n"))
			close(reentered)
		}
	}
	w = NewLineWriter(emit)

	if _, err := w.Write([]byte("resid")); err != nil {
		t.Fatalf("准备残段时 Write 返回了非 nil 的 error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		w.Flush()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(deadlockDetectTimeout):
		t.Fatal("Flush 未在超时内返回：emit 回调里再入 Write 疑似死锁（怀疑 Flush 持锁调用了 emit）")
	}

	select {
	case <-reentered:
	case <-time.After(deadlockDetectTimeout):
		t.Fatal("emit 回调（由 Flush 触发）里的再入 Write 未完成：疑似死锁")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("emit 应总共被调用两次（resid 与 re），实际 %d 次：%v", len(got), got)
	}
	if got[0] != "resid" || got[1] != "re" {
		t.Fatalf("emit 收到的内容应依次为 [\"resid\" \"re\"]，实际 %v", got)
	}
}

// ===== 验收 11（c：从 SetEmit 补吐触发的 emit 再入 Write） =====
// 验证 SetEmit 装上非 nil 出口、补吐已缓冲完整行时同样不持锁调用 emit。
// 超时判定的理由同 11a，不重复。
func TestAcceptance11c_EmitNotUnderLock_ReenterFromSetEmitBackfill(t *testing.T) {
	w := NewLineWriter(nil)
	if _, err := w.Write([]byte("x\n")); err != nil {
		t.Fatalf("准备待补吐的完整行时 Write 返回了非 nil 的 error: %v", err)
	}

	var mu sync.Mutex
	var got []string
	reentered := make(chan struct{})

	emit := func(line string) {
		mu.Lock()
		got = append(got, line)
		mu.Unlock()
		if line == "x" {
			w.Write([]byte("re\n"))
			close(reentered)
		}
	}

	done := make(chan struct{})
	go func() {
		w.SetEmit(emit)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(deadlockDetectTimeout):
		t.Fatal("SetEmit 未在超时内返回：补吐时 emit 回调里再入 Write 疑似死锁（怀疑 SetEmit 持锁调用了 emit）")
	}

	select {
	case <-reentered:
	case <-time.After(deadlockDetectTimeout):
		t.Fatal("emit 回调（由 SetEmit 补吐触发）里的再入 Write 未完成：疑似死锁")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("emit 应总共被调用两次（x 与 re），实际 %d 次：%v", len(got), got)
	}
	if got[0] != "x" || got[1] != "re" {
		t.Fatalf("emit 收到的内容应依次为 [\"x\" \"re\"]，实际 %v", got)
	}
}

// ===== 验收 12 =====
// N 个 goroutine 各自 Write 自己的完整行（"g<id>-line<n>\n"），全部返回后 emit
// 收到的行数应恰好等于总行数，且每一行必须完整、不能被别的 goroutine 的内容
// 穿插进去。
//
// 只数行数不够：一个把两个 goroutine 的内容拼错的实现，行数可能照样对。判别力
// 来自"每一行自带可自校验的结构"——emit 收到的每个字符串都要能严格解析回
// (id, n)，并且把 (id, n) 反着按同样格式拼回去必须逐字节等于原串（见 parseGLine，
// 避免 fmt.Sscanf 对带垃圾尾巴或格式变形的坏行也"部分匹配"成功）。任何一行
// 解析失败，或者同一个 (id, n) 出现了不止一次（意味着内容被复制或错位拼接），判红。
//
// 没有刻意构造"两个 goroutine 各自只写半行、指望它们拼出一个新行"这类变体——
// 那种情况下不同 goroutine 的半行落进同一个共享缓冲区、按到达顺序拼接，是任何
// 符合契约的实现都会产生的合法结果（缓冲区不认得"这半行是谁的"），拿它当穿插
// 证据会把正确实现也判红。本测试能测的只有"每个 goroutine 一次 Write 一整行"
// 这个形状，这是故意的，不是遗漏。
//
// 本机 -race 跑不了（-race requires cgo，本机没有 gcc，已用 go test -race 实测
// 报错两次，见报告第 6 条），所以这里不能指望 -race 兜底，判别力必须自己造：
// 用 start 屏障让所有 goroutine 尽量在同一时刻开始抢锁，把 5 轮独立的 LineWriter
// 实例跑一遍进一步放大命中窗口。这仍然是概率性的探测手段，不是形式化证明——
// 调度器恰好没有暴露出的 race 仍然可能漏网，这一点在报告里也会写明。
func TestAcceptance12_ConcurrentWritesEachLineIntact(t *testing.T) {
	const rounds = 5
	const numGoroutines = 20
	const linesPerGoroutine = 50
	const totalLines = numGoroutines * linesPerGoroutine

	for round := 0; round < rounds; round++ {
		var mu sync.Mutex
		received := make([]string, 0, totalLines)
		emit := func(line string) {
			mu.Lock()
			received = append(received, line)
			mu.Unlock()
		}
		w := NewLineWriter(emit)

		start := make(chan struct{})
		var wg sync.WaitGroup
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				<-start
				for n := 0; n < linesPerGoroutine; n++ {
					line := fmt.Sprintf("g%d-line%d\n", id, n)
					if _, err := w.Write([]byte(line)); err != nil {
						t.Errorf("round %d: Write 返回了非 nil 的 error: %v", round, err)
					}
				}
			}(g)
		}
		close(start)
		wg.Wait()

		mu.Lock()
		got := append([]string(nil), received...)
		mu.Unlock()

		if len(got) != totalLines {
			t.Fatalf("round %d: emit 收到的行数应为 %d，实际 %d", round, totalLines, len(got))
		}

		seen := make(map[[2]int]bool, totalLines)
		for _, line := range got {
			id, n, ok := parseGLine(line)
			if !ok {
				t.Fatalf("round %d: 收到一行不满足 g<id>-line<n> 的严格结构，怀疑被别的 goroutine 的内容穿插破坏：%q", round, line)
			}
			key := [2]int{id, n}
			if seen[key] {
				t.Fatalf("round %d: 行 %q 对应的 (id,n) 重复出现，怀疑内容被复制或错位拼接", round, line)
			}
			seen[key] = true
		}
		if len(seen) != totalLines {
			t.Fatalf("round %d: 去重后唯一行数 %d 应等于总行数 %d", round, len(seen), totalLines)
		}
	}
}

// parseGLine 把形如 "g<id>-line<n>" 的一行严格解析成 (id, n)。
// 严格的含义：把解析出的 (id, n) 反过来按同样的格式重新拼出字符串，必须逐字节
// 等于原始输入，否则判定为不匹配（防止 Sscanf 对带垃圾尾巴或格式变形的坏行也
// "部分匹配"成功，放过被穿插破坏的行）。
func parseGLine(line string) (id int, n int, ok bool) {
	if _, err := fmt.Sscanf(line, "g%d-line%d", &id, &n); err != nil {
		return 0, 0, false
	}
	rebuilt := fmt.Sprintf("g%d-line%d", id, n)
	if rebuilt != line {
		return 0, 0, false
	}
	return id, n, true
}
