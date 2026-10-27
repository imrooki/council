package localagent

// merge.go 把「Council 自己知道的那一轮」与「本机进程表里的一条」对成同一条
// 实例（Plan §4.3 ⑨ 的合并规则）。
//
// 为什么需要它：managed 行有身份（turn_id / conv_id / agent_id）但没有 PID，
// observed 行有 PID 但没有身份。两段并列着画，用户看到的是「我的会话在跑」
// 和「机器上有个 claude.exe」两条互不相干的事实。⑨ 的 PID 钩子把它们接上：
// 同一轮的子进程 PID 一旦知道，就能指认「这一条进程就是那一轮」。
//
// 三条防御，缺一条就会把别人的进程算成自己的：
//  1. 只合并 **PID 相同**的行；PID 不同一律不合并。
//  2. `.cmd` / `.bat` 垫片的 PID 是 cmd.exe 的，不是真正干活的进程（本机实测：
//     probe.exe → cmd.exe → claude.exe）。所以垫片那条要找它**唯一**的 agent
//     子进程；找不到唯一一个就不合并，保留 managed 的 turn_id 就好。
//  3. 两条都拿得到启动时间时，相差超过 mergeWindow 就不合并 —— PID 复用之后
//     新进程的创建时间不会等于旧轮次的开始时间。拿不到启动时间时（受保护
//     进程）不拿它当否决条件，因为「不知道」不等于「不匹配」。

import (
	"time"
)

// mergeWindow 是「这一轮的开始」与「这个进程的创建时间」之间允许的偏差。
//
// 取两分钟：spawn 到 exec 之间只隔一次 CreateProcess（毫秒级），但
// inflight_turns.started_at 是 Council 记账时写的，与进程创建时间之间
// 有命令行组装、落盘、鉴权等开销；两分钟足够宽，又远小于 Linux 上默认的
// pid_max 回绕周期。
const mergeWindow = 2 * time.Minute

// Observation 是一次扫描的两个半边：原始进程快照与规则表结论。
//
// 为什么要把 Processes 也交出来：垫片判定要看父进程的子进程列表，只看
// Agents（已识别的 CLI）不知道 cmd.exe 有几个孩子。
type Observation struct {
	Processes []Process
	Agents    []Agent
}

// ManagedRef 是一条仍在飞的轮，带上「同一进程里记到的子进程 PID」。
//
// PID 为 0 表示没记到（钩子没接、或这一轮在别的进程里跑的）——那种轮次
// 照旧单独列在 managed 段，不参与合并。
//
// WorkDir 与 Phase 是给界面看的透传字段，不参与合并判断：合并只认 PID、
// 垫片子进程与启动时间三件事。
type ManagedRef struct {
	TurnID    string
	ConvID    string
	AgentID   string
	Provider  string
	PID       int
	StartedAt time.Time
	WorkDir   string
	Phase     string
}

// MergeResult 是合并之后的两段。
//
// Unmatched 是没能在进程表里找到对应进程的那些轮（照旧单独列）；
// Agents 是进程表，其中对上的那些已经被升级成 managed 并带上身份。
// 一条轮只会出现在其中一处，不会两边都画 —— 那正是这次合并的目的。
type MergeResult struct {
	Unmatched []ManagedRef
	Agents    []Agent
}

// MergeManaged 按 PID（必要时穿过垫片）把两段事实对上。
//
// 纯函数：不读环境、不看时钟，输入输出都是值，所以合并规则可以被表驱动
// 用例逐条钉住（见 merge_test.go）。
func MergeManaged(refs []ManagedRef, obs Observation) MergeResult {
	agents := make([]Agent, len(obs.Agents))
	copy(agents, obs.Agents)

	indexOfPID := make(map[int]int, len(agents))
	for i, agent := range agents {
		indexOfPID[agent.PID] = i
	}
	agentChildren := make(map[int][]int, len(agents))
	for _, agent := range agents {
		agentChildren[agent.PPID] = append(agentChildren[agent.PPID], agent.PID)
	}
	exeByPID := make(map[int]string, len(obs.Processes))
	for _, process := range obs.Processes {
		exeByPID[process.PID] = process.ExeName
	}

	used := make(map[int]bool, len(agents))
	unmatched := make([]ManagedRef, 0, len(refs))
	for _, ref := range refs {
		index, evidence := matchManaged(ref, indexOfPID, agentChildren, exeByPID, agents, used)
		if index < 0 {
			unmatched = append(unmatched, ref)
			continue
		}
		if startedApart(agents[index], ref) {
			// PID 撞上了但时间差得远：那是 PID 复用，不是同一轮。
			unmatched = append(unmatched, ref)
			continue
		}
		used[index] = true
		agent := &agents[index]
		agent.Source = SourceManaged
		agent.ConversationID = ref.ConvID
		agent.AgentID = ref.AgentID
		agent.TurnID = ref.TurnID
		agent.MergeEvidence = evidence
	}
	return MergeResult{Unmatched: unmatched, Agents: agents}
}

// matchManaged 回答「这条轮对应哪一条 agent」，返回下标与合并依据。
// 返回 -1 表示对不上，调用方把它留在 managed 段。
func matchManaged(ref ManagedRef, indexOfPID map[int]int, agentChildren map[int][]int,
	exeByPID map[int]string, agents []Agent, used map[int]bool) (int, string) {
	if ref.PID <= 0 {
		return -1, ""
	}
	// 直配：钩子拿到的就是真正的 CLI 进程（grok / agy / codex 这类原生 exe）。
	if index, ok := indexOfPID[ref.PID]; ok {
		if used[index] {
			return -1, ""
		}
		return index, "pid"
	}
	// 垫片：钩子拿到的是 cmd.exe，真正干活的是它唯一的那个 agent 子进程。
	children := agentChildren[ref.PID]
	if len(children) != 1 {
		return -1, ""
	}
	index, ok := indexOfPID[children[0]]
	if !ok || used[index] {
		return -1, ""
	}
	shim := exeByPID[ref.PID]
	if shim == "" {
		shim = "shim"
	}
	return index, "shim=" + shim
}

// startedApart 判断「这一轮的开始」与「这个进程的创建时间」差得太远。
// 任一侧拿不到时间就返回 false（不拿「不知道」当否决）。
func startedApart(agent Agent, ref ManagedRef) bool {
	if agent.StartedAt.IsZero() || ref.StartedAt.IsZero() {
		return false
	}
	diff := agent.StartedAt.Sub(ref.StartedAt)
	if diff < 0 {
		diff = -diff
	}
	return diff > mergeWindow
}

// Observe 是 Scan 的完整版：连原始进程快照一起交出来，供合并使用。
//
// 层级不允许扫描时返回零值，一次枚举都不做（与 Scan 同一条 fail-closed）。
// enum 为 nil 时返回 ErrUnsupported：调用方漏注入时应当拿到一条可展示的
// 错误，而不是一个「本机没有进程」的空表。
func Observe(level Level, enum Enumerator) (Observation, error) {
	if !level.Scans() {
		return Observation{Processes: []Process{}, Agents: []Agent{}}, nil
	}
	if enum == nil {
		return Observation{Processes: []Process{}, Agents: []Agent{}}, ErrUnsupported
	}
	processes, err := enum()
	if err != nil {
		return Observation{Processes: []Process{}, Agents: []Agent{}}, err
	}
	return Observation{Processes: processes, Agents: Identify(processes)}, nil
}
