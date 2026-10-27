package localagent

// registry.go 记住「这一轮的子进程 PID 是多少」，供合并使用（Plan §4.3 ⑨）。
//
// 三个边界：
//   - **只在内存里**：观察到的进程不落库（S-09），PID 属于同一类事实。
//     进程退出，这张表就没了；跨进程的 council who 永远读不到它 ——
//     这正是把钩子做成进程内回调的直接后果，写进 README 也不能算意外。
//   - **只记住、不判断**：这里不做「这轮还在不在飞」的判断，那是调用方
//     拿 inflight_turns 对出来的。表里留着一条已经结束的轮无害：调用方只按
//     当前在飞的轮去查，查不到就不会合并。
//   - **有上限**：条目按 (会话, 成员) 覆盖写，并且超过 ttl 的会被清掉，
//     不会随会话数无限长。

import (
	"sync"
	"time"
)

// pidEntryTTL 是一条 PID 记录的最长寿命。
//
// 一轮的墙钟上限是几分钟（Runner 的 spawn 超时），一小时足够覆盖任何
// 真实轮次，又让长期开着的看板不会攒下几千条。清理只在观察时顺手做。
const pidEntryTTL = time.Hour

// pidKey 是「哪一轮」的最小标识：同一个 (会话, 成员) 同时只会有一轮在飞
// （Runner 的会话闸保证），所以不需要 turn_id 做键。
type pidKey struct {
	convID  string
	agentID string
}

type pidEntry struct {
	pid int
	at  time.Time
}

// PIDRegistry 是并发安全的「(会话, 成员) → 子进程 PID」表。
//
// 零值不可用：用 NewPIDRegistry。为什么不是 map 直接用：观察来自 Runner 的
// worker goroutine，读取来自 HTTP handler，两边不在一个 goroutine 里。
type PIDRegistry struct {
	mu      sync.Mutex
	entries map[pidKey]pidEntry
	// now 可换（测试里用固定时钟验证 ttl 清理）；生产是 time.Now。
	now func() time.Time
}

// NewPIDRegistry 造一张空表。
func NewPIDRegistry() *PIDRegistry {
	return &PIDRegistry{
		entries: make(map[pidKey]pidEntry, 8),
		now:     time.Now,
	}
}

// Observe 记下某个成员这一轮的子进程 PID。pid <= 0 或任一 id 为空时忽略：
// 那不是一条事实。
//
// 顺手清掉过期条目（最多每条记录一次全扫，表很小）。
func (r *PIDRegistry) Observe(convID, agentID string, pid int) {
	if r == nil || pid <= 0 || convID == "" || agentID == "" {
		return
	}
	now := r.clock()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(now)
	r.entries[pidKey{convID: convID, agentID: agentID}] = pidEntry{pid: pid, at: now}
}

// Lookup 返回某个成员最近一次记下的 PID。
//
// ok 为 false 表示没记到（钩子没接、这一轮在别的进程里跑、或已过期）。
func (r *PIDRegistry) Lookup(convID, agentID string) (int, bool) {
	if r == nil {
		return 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[pidKey{convID: convID, agentID: agentID}]
	if !ok {
		return 0, false
	}
	if r.clock().Sub(entry.at) > pidEntryTTL {
		return 0, false
	}
	return entry.pid, true
}

// Len 是条目数，只给测试与诊断用。
func (r *PIDRegistry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// clock 返回当前时间。r.now 为 nil 时（零值 Registry）退回 time.Now，
// 这样即使有人直接声明零值也不会在时钟上 panic。
func (r *PIDRegistry) clock() time.Time {
	if r == nil || r.now == nil {
		return time.Now()
	}
	return r.now()
}

func (r *PIDRegistry) pruneLocked(now time.Time) {
	for key, entry := range r.entries {
		if now.Sub(entry.at) > pidEntryTTL {
			delete(r.entries, key)
		}
	}
}
