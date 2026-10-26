package turn

// agent_reset.go 是设置页热换身份字段时「清内存那一半」的实现。
//
// 为什么光删库不够：Runner 的每条 (会话, 成员) 闸上挂着一份 sessionState
// 内存快照，Ask 只要看到 g.st 非 nil 就拿它和新算出来的指纹比，不符直接
// ErrIdentityDrift 拒绝（runner.go 的「进程内已有的那条不符」分支），
// 根本不会回头读库。也就是说：只删 provider_sessions 而不清这份快照，
// 下一轮仍然会被拒绝，用户看到的是「改了配置之后这个成员突然不能说话了」。
// /session rebuild 正是靠「删行 + 把 g.st 置 nil」两件事一起做才有效的。
//
// 与 Rebuild 的差别只有范围：Rebuild 按 (会话, 成员) 一次清一条，这里按
// 成员清掉**所有会话**——身份指纹是所有会话共用的，任何一条旧快照留下来
// 都会在那一场上撞漂移。持锁方式照抄 Rebuild：TryLock 拿不到就 fail-closed
// 返回 ErrTurnInFlight，什么都不清。

import (
	"fmt"
	"sort"
)

// ForgetAgentSessions 丢掉 agentID 在进程内每一条会话上的 provider 会话
// 快照，返回清掉的非空快照数（不等于删掉的库行数：进程重启之后内存是空的，
// 库里却还有；反过来一轮失败之后内存记了状态但从未落库）。
//
// 只清内存，不碰数据库。删库由调用方（internal/serve 的 settingsHotReload）
// 在同一个 reloadMu 临界区里经 store.ResetProviderSessions 完成：那里已经
// 确认过没有轮次在跑，本进程也不存在「清完内存、删库之前插进一轮」的窗口
// ——handleAsk 把 job 放进任务表时抢的是同一把 reloadMu。
//
// 为什么这里用 TryLock 而不是等：等一把被在飞一轮持有的闸意味着阻塞整次
// 设置保存；而且等到了也白做——在飞那一轮结束时会 SetProviderSession 把
// 行写回来。拒绝是 fail-closed 的那一边，调用方把它翻译成「有轮次在运行，
// 等它结束再保存」。
//
// agentID 为空时返回 (0, nil)：没有任何闸可能匹配，这不是错误。这里不
// trim 也不校验 id 格式——它必须是配置里那个原样的 key，猜一个规范化形式
// 只会让「清了个寂寞」变得更难发现。
func (r *Runner) ForgetAgentSessions(agentID string) (int, error) {
	r.gatesMu.Lock()
	keys := make([]sessionKey, 0, len(r.gates))
	for key := range r.gates {
		if key.agentID == agentID {
			keys = append(keys, key)
		}
	}
	// 排序只为让「哪一把闸先被锁」确定下来：同一个成员在两条会话上并发
	// 调本方法时不会以相反的加锁顺序互相干扰（TryLock 本身不阻塞，这里
	// 是给将来的排查留一份可复现的顺序）。
	sort.Slice(keys, func(i, j int) bool { return keys[i].convID < keys[j].convID })
	gates := make([]*sessionGate, 0, len(keys))
	for _, key := range keys {
		gates = append(gates, r.gates[key])
	}
	r.gatesMu.Unlock()

	locked := make([]*sessionGate, 0, len(gates))
	release := func() {
		for _, g := range locked {
			g.mu.Unlock()
		}
	}
	for _, g := range gates {
		if !g.mu.TryLock() {
			release()
			return 0, fmt.Errorf("forget agent sessions %s: %w", agentID, ErrTurnInFlight)
		}
		locked = append(locked, g)
	}
	defer release()

	cleared := 0
	for _, g := range locked {
		if g.st == nil {
			continue
		}
		g.st = nil
		cleared++
	}
	return cleared, nil
}
