package localagent

// registry_test.go 钉住 PID 表的三个边界：只记事实、过期清掉、并发安全。

import (
	"sync"
	"testing"
	"time"
)

// TestPIDRegistryObserveLookup 覆盖最基本的写读，以及「垃圾输入不写」。
func TestPIDRegistryObserveLookup(t *testing.T) {
	r := NewPIDRegistry()
	r.Observe("c1", "a1", 1234)
	if pid, ok := r.Lookup("c1", "a1"); !ok || pid != 1234 {
		t.Errorf("Lookup(c1,a1) = %d, %v；期望 1234, true", pid, ok)
	}
	// 同一 (会话, 成员) 覆盖写：一轮结束下一轮会用新 PID。
	r.Observe("c1", "a1", 5678)
	if pid, _ := r.Lookup("c1", "a1"); pid != 5678 {
		t.Errorf("覆盖写之后 Lookup = %d，期望 5678", pid)
	}
	// 没记过的组合。
	if _, ok := r.Lookup("c2", "a1"); ok {
		t.Error("没记过的组合不该命中")
	}
	// 垃圾输入：不写。
	for _, bad := range []struct {
		conv, agent string
		pid         int
	}{
		{"c1", "a1", 0},
		{"c1", "a1", -5},
		{"", "a1", 99},
		{"c1", "", 99},
	} {
		before := r.Len()
		r.Observe(bad.conv, bad.agent, bad.pid)
		if r.Len() != before {
			t.Errorf("垃圾输入 %+v 被记了下来", bad)
		}
	}
}

// TestPIDRegistryExpires 覆盖 ttl：超过 pidEntryTTL 的条目既读不到、也会在
// 下一次 Observe 时被清掉。
func TestPIDRegistryExpires(t *testing.T) {
	r := NewPIDRegistry()
	clock := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return clock }

	r.Observe("c1", "a1", 4321)
	if _, ok := r.Lookup("c1", "a1"); !ok {
		t.Fatal("刚写进去就查不到")
	}

	clock = clock.Add(pidEntryTTL / 2)
	if _, ok := r.Lookup("c1", "a1"); !ok {
		t.Error("半个 ttl 之内应当还在")
	}

	clock = clock.Add(pidEntryTTL)
	if _, ok := r.Lookup("c1", "a1"); ok {
		t.Error("超过 ttl 之后不该再命中")
	}
	// 下一次 Observe 顺手清掉旧条目（不是无限增长）。
	r.Observe("c2", "a2", 1111)
	if r.Len() != 1 {
		t.Errorf("清理之后条目数 = %d，期望 1（只留新写的那条）", r.Len())
	}
}

// TestPIDRegistryConcurrent 用 race detector 能看的方式并发写读。
// 没有 -race 时它至少能证明不会 panic 或死锁。
func TestPIDRegistryConcurrent(t *testing.T) {
	r := NewPIDRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			r.Observe("c1", "a1", 1000+i)
		}(i)
		go func() {
			defer wg.Done()
			_, _ = r.Lookup("c1", "a1")
			_ = r.Len()
		}()
	}
	wg.Wait()
	if r.Len() != 1 {
		t.Errorf("条目数 = %d，期望 1（同一个 (会话, 成员) 只占一行）", r.Len())
	}
}

// TestPIDRegistryNilSafe 钉住零值/ nil 接收者不会 panic：看板没接钩子时
// 这两个方法仍可能被调到。
func TestPIDRegistryNilSafe(t *testing.T) {
	var r *PIDRegistry
	r.Observe("c1", "a1", 1)
	if _, ok := r.Lookup("c1", "a1"); ok {
		t.Error("nil 表不该命中")
	}
	if got := r.Len(); got != 0 {
		t.Errorf("nil 表的 Len = %d", got)
	}
}
