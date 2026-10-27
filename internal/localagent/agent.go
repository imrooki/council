// Package localagent 观察本机正在运行的 agent 进程，是 Local Agent 模块
// 的第一阶段（L1「看得见」）。
//
// 它默认关闭。配置里的 local_agent 不是 observe 时，Scan 一次枚举都不做，
// 调用方拿到空列表（S-01：关着的时候零扫描、零后台、零行为变化）。开启后
// 它也只读一份最小事实：可执行文件名、PID、父 PID、启动时间（若系统给得出）。
// 它不读完整命令行、环境变量、窗口标题、剪贴板或其它 CLI 的配置文件
// （S-03 / S-09），也不启动、结束、挂起或注入任何进程（S-07）。
//
// 为什么单独成包：CLI（council who）与 MCP（council_who）必须用同一份事实
// 与同一张规则表。放进 internal/turn 会让「跑一轮」知道机器上有几个编辑器；
// 放进 internal/serve 会让 CLI 依赖 Web 层。
package localagent

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// ErrUnsupported 表示当前平台没有进程枚举实现（非 Windows）。
//
// 为什么不是悄悄返回空列表：空列表会被读成「本机确实没有 agent 进程」，
// 而事实是「这个平台看不了」。调用方要把两者分开显示，否则用户会以为
// 观察功能已经开着、只是机器很干净。
var ErrUnsupported = errors.New("local agent observation is not supported on this platform")

// Source 说明一条 agent 记录的事实来源。
type Source string

const (
	// SourceManaged 表示这个进程由 Council 自己的调度器启动，能与
	// inflight_turns / 未来的 PID 钩子对上号。L1 不产生这种记录：
	// Council 现在只把子进程 PID 留在 spawnProcess 的局部变量里，没有
	// 向上返回（Plan v3 §4.3 ⑨）；PID 钩子是 L2 的事。
	//
	// 而且**不能靠 PID 直接对**：cmd.Start() 起 .cmd 垫片时拿到的是
	// cmd.exe 的 PID，真正干活的进程是它的子进程。L1 因此不做
	// managed ↔ observed 的合并 —— 把观察到的进程标成 managed 是误报，
	// 那是 L2 有了进程树/启动时间窗口之后才能做的事。
	SourceManaged Source = "managed"
	// SourceObserved 表示这是本机进程枚举看到的进程。Council 不知道它在
	// 思考什么、用哪个模型、负责哪个任务 —— 只有 exe 名、路径与进程关系。
	SourceObserved Source = "observed"
)

// Certainty 说明一条 observed 记录的确定性。它回答的是「凭什么说这个
// 进程是那个 agent」，不是「这个进程存在吗」。
type Certainty string

const (
	// CertaintyExact 表示映像的完整路径落在已知的 CLI 安装位置里
	// （npm / node_modules / .local/bin / .cargo/bin 等）。同名程序可能有
	// 多个（Claude Desktop 也装了 claude.exe），只有路径证据才配 exact。
	CertaintyExact Certainty = "exact"
	// CertaintyProbable 表示只有映像名或父进程关系这类间接证据：
	// 映像路径拿不到（受保护进程），或者路径不在已知安装位置里。
	// 宁可标 probable，也不要把 IDE / 桌面端里的同名程序报成确认的 agent。
	CertaintyProbable Certainty = "probable"
)

// Kind 是本机 agent 的类型，取值就是四家 CLI 的名字。
type Kind string

const (
	KindClaude Kind = "claude"
	KindCodex  Kind = "codex"
	KindGrok   Kind = "grok"
	KindAgy    Kind = "agy"
	// KindUnknown 是「规则命中了，但结论是认不出」。它不会出现在
	// Identify / Scan 的结果里：L1 只列能确认的四家 CLI，把 node.exe /
	// python.exe 这类通用宿主也塞进结果只会制造噪声与误判。
	KindUnknown Kind = "unknown"
)

// Process 是一行原始进程事实，也是规则表的输入。
//
// 字段刻意只有这五个：映像基名、映像完整路径、PID、父 PID、启动时间。
// 命令行、用户名、窗口标题、环境块一律不在里面：那些字段一旦进了结构体，
// 就会有人拿它们做识别，而 S-09 禁止采集（命令行里常带 token 与私密路径）。
// Toolhelp32 本来也拿不到完整命令行。
//
// 为什么要映像完整路径：同名的 exe 不止一个。Claude Desktop 也装了
// claude.exe，codex.exe 也可能来自 IDE / app-server；只看基名会把它们
// 全报成 CLI agent。路径只用来判断「装在哪个目录」，不读它指向的文件。
type Process struct {
	PID     int
	PPID    int
	ExeName string
	// ExePath 是映像的完整路径（QueryFullProcessImageName 给出）。
	// 打不开进程句柄时是空串 —— 空串会让规则退到 probable，不会猜。
	ExePath string
	// StartedAt 是进程创建时间；打不开进程句柄（受保护、跨会话）时是
	// time.Time 的零值。不要伪造一个时间。
	StartedAt time.Time
}

// Agent 是一个被识别出来的本机 agent 进程。
//
// 为什么 Source 与 Certainty 分开：Source 回答「这条记录从哪来」
// （Council 自己调度 / 本机观察），Certainty 回答「观察到的这个进程有
// 多确定是那家 CLI」。同一种 Kind 可以同时有 Council 自己起的实例与本机
// 观察到的实例，混成一个枚举以后就没法做去重（Plan v3 §4.3 ⑨ 的合并规则）。
type Agent struct {
	Kind    Kind
	PID     int
	PPID    int
	ExeName string
	// ExePath 是规则判定所依据的映像完整路径；拿不到时是空串。
	ExePath string
	// StartedAt 是进程创建时间；零值表示系统没给（不是「1970 年」）。
	StartedAt time.Time
	Source    Source
	// Certainty 只对 SourceObserved 有意义：exact 表示路径命中了已知
	// CLI 安装位置，probable 表示只有映像名 / 父进程证据。
	Certainty Certainty
}

// Enumerator 返回一次本机进程快照。返回值只允许上面 Process 的四个字段。
//
// 单独定义成函数类型是为了让「关着的时候不枚举」可以被测试直接证明：
// 测试注入一个会 panic 的实现，断言 off 路径没有调用它。
type Enumerator func() ([]Process, error)

// Level 是 local_agent 开关的两种取值。L1 只有关与观察两档。
type Level string

const (
	// LevelOff 是默认值：不枚举、不扫描、不留后台痕迹。
	LevelOff Level = "off"
	// LevelObserve 表示用户显式开启只读观察。
	LevelObserve Level = "observe"
)

// LevelOf 把配置里的字符串折成 Level。
//
// 只认 observe（大小写、首尾空白不敏感），其它一律 off：Load 已经做过一次
// 归一与校验，这里再 fail closed 一次，防止直接构造 Config 的调用方把未来
// 某个层级名（register / coordinate / …）带进来 —— 那些层级在 v1 没有实现，
// 当成 observe 会静默扩大采集面。
func LevelOf(raw string) Level {
	if strings.EqualFold(strings.TrimSpace(raw), string(LevelObserve)) {
		return LevelObserve
	}
	return LevelOff
}

// Enumerate 返回一次本机进程快照（只读）。这是生产代码唯一的进程枚举
// 调用点；Scan 在关着的时候不会走到它。
func Enumerate() ([]Process, error) {
	return enumerate()
}

// Scan 是唯一的扫描入口：council who 与 MCP 的 council_who 都走它。
// 「off 时不枚举」只需要在这一处保证，不会因为两个调用方各写一遍而漏掉。
//
// level 不是 LevelObserve 时直接返回空列表，连 enum 都不碰：即便调用方传
// 进来一个会 panic 的枚举函数，off 路径也不会调用它（S-01，测试直接钉住）。
//
// enum 为 nil 时返回 ErrUnsupported 而不是 panic：调用方漏注入时应当拿到
// 一条可展示的错误，而不是让整个 council who 崩掉。
func Scan(level Level, enum Enumerator) ([]Agent, error) {
	if level != LevelObserve {
		return []Agent{}, nil
	}
	if enum == nil {
		return []Agent{}, ErrUnsupported
	}
	processes, err := enum()
	if err != nil {
		return []Agent{}, err
	}
	return Identify(processes), nil
}

// baseName 把可执行文件名折成小写基名。
//
// 同时处理 / 与 \：Toolhelp32 只给基名，但测试与将来的调用方可能传完整
// 路径；只认一种分隔符会让规则在另一个平台上静默不命中。
func baseName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return strings.ToLower(name)
}

// sortAgents 让输出稳定：先按类型，再按 PID。两个 codex 谁先谁后不应该
// 随进程枚举顺序漂移，否则 council who 的输出无法按行断言。
func sortAgents(agents []Agent) {
	sort.SliceStable(agents, func(i, j int) bool {
		if agents[i].Kind != agents[j].Kind {
			return agents[i].Kind < agents[j].Kind
		}
		return agents[i].PID < agents[j].PID
	})
}
