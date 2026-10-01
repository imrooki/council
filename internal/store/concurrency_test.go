// concurrency_test.go 是 D-10 的回归测试，不是一个普通的压力测试。
//
// 这个用例红了，基本可以断定不是「并发写得不好」，而是有人动了 SQLite 的打开方式：
// DSN 里的 _pragma、_txlock=immediate、或者写句柄的 SetMaxOpenConns(1) 少了一条。
// 三条各挡一个坑，缺任何一条都会大面积 database is locked，或者慢两个数量级。
// 动手改之前先读 00_DECISIONS.md 的 D-10 与 docs/reference/sqlite-probe-results.md，
// 那里有四轮探针共 1280 次并发写的实测数据，不要重新推导，也不要「优化」掉其中任何一条。
//
// 负载与探针一致：8 个 goroutine 各写 40 条，共 320 条。
// 断言：一次失败都没有、落库正好 320 行、seq 从 1 到 320 连续、无重复、无空洞。
package store

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
)

const (
	// tstConcurrentWriters 与 tstWritesPerGoroutine 抄探针的负载参数，改之前先看 D-10。
	tstConcurrentWriters  = 8
	tstWritesPerGoroutine = 40
	tstConcurrentTotal    = tstConcurrentWriters * tstWritesPerGoroutine
)

// tstConcurrencyHint 是并发用例失败时附带的指路信息。
const tstConcurrencyHint = "这是 D-10 的回归用例：先看 DSN 的 _pragma / _txlock=immediate / 写句柄 SetMaxOpenConns(1)，" +
	"证据在 00_DECISIONS.md D-10 与 docs/reference/sqlite-probe-results.md"

func TestConcurrentAppendMessage(t *testing.T) {
	s, _ := tstStore(t)

	// 整批写共用一个带超时的上下文：写句柄是单连接，真出现事务泄漏或死锁时，
	// 这里会变成一条超时失败，而不是让 go test 挂到整体超时。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conv := tstNewConv(t, ctx, s, "并发追加")

	type writeErr struct {
		writer int
		index  int
		err    error
	}

	var (
		mu     sync.Mutex
		failed []writeErr
		seqs   = make([]int64, 0, tstConcurrentTotal)
		ids    = make([]string, 0, tstConcurrentTotal)
		wg     sync.WaitGroup
	)

	for w := 0; w < tstConcurrentWriters; w++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for i := 0; i < tstWritesPerGoroutine; i++ {
				// goroutine 里不碰 t：出错一律收进切片，等 Wait 之后统一断言。
				var got conversation.Message
				err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
					var err error
					got, err = tx.AppendMessage(ctx, tstUserMsg(conv.ID, "并发写入"))
					return err
				})
				mu.Lock()
				if err != nil {
					failed = append(failed, writeErr{writer: writer, index: i, err: err})
				} else {
					seqs = append(seqs, got.Seq)
					ids = append(ids, got.ID)
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()

	t.Run("320 次并发写全部成功", func(t *testing.T) {
		if len(failed) == 0 {
			return
		}
		locked := 0
		for _, f := range failed {
			if strings.Contains(strings.ToLower(f.err.Error()), "database is locked") {
				locked++
			}
		}
		t.Errorf("%d/%d 次并发写失败，其中 %d 次是 database is locked；首个错误（writer %d 第 %d 次）: %v",
			len(failed), tstConcurrentTotal, locked, failed[0].writer, failed[0].index, failed[0].err)
		if locked > 0 {
			t.Errorf("出现了 database is locked，%s", tstConcurrencyHint)
		}
	})

	t.Run("返回的 seq 无重复", func(t *testing.T) {
		seen := make(map[int64]int, len(seqs))
		for _, seq := range seqs {
			seen[seq]++
		}
		for seq, n := range seen {
			if n > 1 {
				t.Errorf("seq %d 被分配了 %d 次；UNIQUE(conv_id, seq) 本该拦住它，%s",
					seq, n, tstConcurrencyHint)
			}
		}
	})

	t.Run("返回的消息 id 无重复", func(t *testing.T) {
		seen := make(map[string]int, len(ids))
		for _, id := range ids {
			seen[id]++
		}
		for id, n := range seen {
			if n > 1 {
				t.Errorf("消息 id %q 出现了 %d 次，id 生成在并发下不唯一", id, n)
			}
		}
	})

	t.Run("落库正好 320 行且 seq 从 1 到 320 连续", func(t *testing.T) {
		if len(failed) > 0 {
			// 有写失败时行数与 seq 必然对不上，这里再报一遍只会把注意力引到错的地方。
			t.Skipf("有 %d 次写失败，先看上一条用例", len(failed))
		}
		got, err := s.Messages(ctx, conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("读消息失败: %v", err)
		}
		if len(got) != tstConcurrentTotal {
			t.Fatalf("落库 %d 行，期望 %d 行，%s", len(got), tstConcurrentTotal, tstConcurrencyHint)
		}
		for i, m := range got {
			want := int64(i + 1)
			if m.Seq != want {
				t.Fatalf("第 %d 行的 seq = %d，期望 %d（seq 必须从 1 起连续、无重复、无空洞），%s",
					i, m.Seq, want, tstConcurrencyHint)
			}
		}
	})

	t.Run("MaxSeq 等于 320", func(t *testing.T) {
		if len(failed) > 0 {
			t.Skipf("有 %d 次写失败，先看上一条用例", len(failed))
		}
		got, err := s.MaxSeq(ctx, conv.ID)
		if err != nil {
			t.Fatalf("MaxSeq 返回错误: %v", err)
		}
		if got != tstConcurrentTotal {
			t.Errorf("MaxSeq = %d，期望 %d", got, tstConcurrentTotal)
		}
	})
}
