package turn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// defaultAskTimeout 是每一次 spawn 的硬超时。
// 这不是保险，是主路径：grok 拿到本地不认识的会话 id 时不报错，去远端恢复，
// 拉起交互式 OAuth 设备码流程然后无限等待授权（实测 4 分钟只吃 0.6 秒 CPU，
// 还绕开 本地转发服务 直连 x.ai）。没有超时，那个进程会永远占着槽位，而且没有任何
// 错误可捕获。5 分钟给得起一轮高 effort 的真思考，又不会让挂死的设备码
// 流程把编排器卡住一个下午。
const defaultAskTimeout = 5 * time.Minute

const (
	historyPage           = 512
	diagStderrBytes       = 2048
	diagStdoutBytes       = 512
	eventOpening          = "session_opening"
	eventTurnFailed       = "turn_failed"
	eventPostRejected     = "post_rejected"
	eventSessionRebuilt   = "session_rebuilt"
	eventTurnLost         = "turn_lost"
	eventRoundStarted     = "round_started"
	eventRoundFinished    = "round_finished"
	effortSourceRequested = "requested"
	modelSourceRequested  = "requested"
	modelSourceObserved   = "observed"
	modelSourceAmbiguous  = "ambiguous"
)

// Spawner 起一个进程并把 stdout / stderr 分开收回来。
//
// 三条硬约束：
//  1. 不合并 stderr：合并会让 JSON 解析吃到诊断文本。
//  2. stdin 必须接 null device，绝不接编排器自己的输入句柄：repl.go 的
//     readLines goroutine 正阻塞在同一个句柄上，抢起来胜负随时序漂移。
//  3. 超时后杀进程树：Windows 上 exec.CommandContext 杀掉的往往只是 .cmd 垫片，
//     真正干活的 node.exe 是它的子进程。实现用 taskkill /T /F /PID 兜底，
//     尽力而为，并把 killed 回给 Runner 去置 Record.MaybeAdvanced。
//
// 抽成字段是为了测试能塞一个回放探针 JSON 的假实现，不必真去调 CLI。
type Spawner func(ctx context.Context, c provider.Command) (stdout, stderr []byte, exitCode int, killed bool, err error)

// sessionGate 是一条 (会话, 成员) 的串行闸，整轮 Ask 期间持有。
//
// st 只在持有 mu 时读写 —— 把 sessionState 塞进 gate 而不是留在外面的 map 里，
// 是为了让「读写 sessionState 必须持有该 key 的锁」由结构保证，不靠注释。
//
// 不变量：gate 一经创建，永不从 gates map 里移除。删了就有一个窗口，
// 两条 goroutine 各自 get-or-create 出两个不同的 gate，闸等于没有。
// Rebuild 要「清掉内存会话」时把 g.st 置 nil，不是删 gate。
//
// 闸按 (conv, agent) 拆，保住的是「同一条 provider 会话不并发 resume」。
// 挡不住两样按 agent 共享的东西：READ 档的工作目录不带 convID
// （agentWorkDir 的有意裁决），以及 claude 每轮往那个目录写的
// council-role.txt。同一个 agent 在两条会话里并发跑，两条 goroutine
// 会同时写同一个路径。今天 REPL 只有一条当前会话所以不可达，本片不修，
// 但不许让下一个人以为闸已经挡住了。
type sessionGate struct {
	mu sync.Mutex
	st *sessionState
}

// Runner 是编排层：挑批次、渲染、翻译、起进程、计时、解析、算差值、
// 落库、推进会话状态。零值不可用，必须经 NewRunner 构造。
//
// 内存里的会话快照仍是本进程内的。Ask 发现这条 gate 的 st 为空时，
// 先从 Service.ProviderSession 读回；每一轮结束经 Service.RecordTurn
// 一次写完 turn_usage 与 provider_sessions。不写 cursors 表：游标已经
// 搬进 provider_sessions.cursor_seq，两处都写就有了两个事实源。
type Runner struct {
	// cfgMu 守 r.cfg。/config reload（--apply 留下新配置；预览路径
	// 临时换上去给 AuditDrift 看一眼再换回来）会经 SetConfig 换指针。
	// 不锁的话并发 Ask 可能快照到那份临时配置，同一轮里 argv 与指纹
	// 还会各用各的。取锁点：SetConfig（写）、Ask/AskUpTo/RunRound/Budget
	// 入场快照（读）、AuditDrift 取快照（读）、RulesReport 取快照（读）。
	//
	// 全局锁顺序：g.mu 先于 rulesMu，g.mu 先于 cfgMu。快照在入场、
	// 拿闸之前取，拷完即放，与 AuditDrift / RunRound 同一位置；
	// 不再持有 g.mu 时嵌套这两把读锁。任何持有 cfgMu 的路径都不得
	// 再取 g.mu。cfgMu 与 rulesMu 互不嵌套持有。
	cfgMu   sync.RWMutex
	cfg     *config.Config
	svc     *conversation.Service
	spawn   Spawner
	now     func() time.Time
	workDir string // Council 自己的干净目录，data_dir 下，绝不是 council 的启动目录
	timeout time.Duration

	// gatesMu 只护 gates 这张 map 的 get-or-create。临界区里只做 map
	// 读写，拿完立刻放，绝不在持有它时做 IO，也绝不在持有它时去取
	// g.mu —— 否则 Ask 整轮持有 g.mu 再回头碰 map，就和旧的全局锁
	// 一样把所有 key 卡住了。
	//
	// 不变量：gate 一经创建，永不从 gates 里移除。删了就有一个窗口，
	// 两条 goroutine 各自 get-or-create 出两个不同的 gate，闸等于没有。
	// Rebuild 要「清掉内存会话」时把 g.st 置 nil，不是删 gate。
	gatesMu sync.Mutex
	gates   map[sessionKey]*sessionGate

	// rulesMu 守 r.rules。/rules reload（不带 --rebuild）会经 SetRules
	// 把临时规则换上去、AuditDrift、再换回来；不锁的话并发 Ask 可能
	// 快照到那份临时规则。五个取锁点：SetRules（写）、Rules（读）、
	// AuditDrift 取快照（读）、Ask/AskUpTo/RunRound/Budget 入场快照（读）、
	// RulesReport 取快照（读）。
	rulesMu sync.RWMutex
	// rules 是 Council 自己的两级规则文件正文，启动时随配置读一次。
	// 它进身份块、进指纹，所以换掉它等于让所有成员的前缀作废 ——
	// 只有 /rules reload --rebuild 会经 SetRules 永久换，且换之前会把代价说清楚。
	rules RulesBlocks
}

// NewRunner 用给定的配置、持久化入口与工作目录基路径构造 Runner。
// workDir 是所有 agent 共用的基目录（<data_dir>/workdir），Ask 内部按
// agentWorkDir(workDir, convID, agentID, tier) 算出每个 agent 这一轮的
// TurnRequest.WorkDir（READ 忽略 convID，WORKSPACE 及以上带上它）。
func NewRunner(cfg *config.Config, svc *conversation.Service, workDir string, rules RulesBlocks) *Runner {
	return &Runner{
		cfg:     cfg,
		svc:     svc,
		spawn:   spawnProcess,
		now:     time.Now,
		workDir: workDir,
		timeout: defaultAskTimeout,
		gates:   make(map[sessionKey]*sessionGate),
		rules:   rules,
	}
}

// gate 按 key 取出这条 (会话, 成员) 的闸，没有就建一把。
//
// gatesMu 的临界区里只有 map 的查找，没有就再写一次；创建路径是一次
// 读取加一次写入，命中路径只有一次读取。绝不在持有 gatesMu 时做 IO，
// 也绝不在持有它时去取 g.mu。调用方拿到返回值之后自己决定何时 Lock /
// TryLock 那把闸。
func (r *Runner) gate(key sessionKey) *sessionGate {
	r.gatesMu.Lock()
	g := r.gates[key]
	if g == nil {
		g = &sessionGate{}
		r.gates[key] = g
	}
	r.gatesMu.Unlock()
	return g
}

// SetConfig 换掉 Runner 手里的配置。由 /config reload 调用：
// --apply 时留下新配置；预览时只是临时换上去给 AuditDrift 看一眼再换回来。
//
// 这段理由被改写过三次，每次都必须说当时为真的那条：
//
// 第一次（原文）：「不加锁：Runner 的全部方法本来就只从 REPL 那一条
// goroutine 调用。哪天真有第二条 goroutine，要加的锁不止这一处。」
//
// 第二次（片 1，按 key 拆闸）：第一条过期。结构上已经允许并发 Ask，
// 而 Ask 会读 r.cfg.Agents[agentID]。本片不加锁，这是一条已知缺口，
// 要动 SetConfig 的冻结说明才能补，留给独立一片。
//
// 第三次（本片）：补上 cfgMu。取写锁，整体换掉指针，立刻放。不锁的话
// 并发 Ask 可能把 /config reload 预览的临时配置快照进身份块和指纹，
// 同一轮里 argv 与指纹还会各用各的。
//
// 不变量：必须整体换掉指针，绝不原地修改被指向的那个 Config。拷指针
// 能保住轮内一致，只在这个前提下成立——AskUpTo 入场拷走的是指针，
// 若有人拿到 r.cfg 之后往 r.cfg.Agents 里塞一条，快照底下的数据还会
// 变，轮粒度就是假的，而且不会有任何一层报错。
//
// 调用方传进来的那个 *config.Config，SetConfig 之后也不该再被调用方
// 改——它已经被别的 goroutine 持有了。今天 /config reload 每次都 Load
// 一份全新的，这条自然成立；写在这里是为了防下一个人「优化」成复用
// 同一份。
func (r *Runner) SetConfig(cfg *config.Config) {
	r.cfgMu.Lock()
	r.cfg = cfg
	r.cfgMu.Unlock()
}

// SetRules 换掉 Runner 手里的规则。永久换由 /rules reload --rebuild 调用；
// 不带 --rebuild 的预览也会临时调本方法换上去、AuditDrift、再换回来。
//
// 取 rulesMu 写锁。不锁的话，并发 Ask 可能把那份临时规则快照进身份块
// 和指纹，同一轮里 argv 与指纹还会各用各的。
func (r *Runner) SetRules(rules RulesBlocks) {
	r.rulesMu.Lock()
	r.rules = rules
	r.rulesMu.Unlock()
}

// Rules 交出当前生效的规则，给 /rules 显示用。
// 返回值是副本（RulesBlocks 是两个 string 的值类型），调用方改不动内部状态。
// 取 rulesMu 读锁：/rules 同时调 Rules 与 RulesReport，两边必须看见同一份。
func (r *Runner) Rules() RulesBlocks {
	r.rulesMu.RLock()
	defer r.rulesMu.RUnlock()
	return r.rules
}

// Rebuild 丢弃 (convID, agentID) 的 provider 会话，让下一轮走 Fresh。
//
// 五样必须全丢：会话 id / 游标 / 成本基线 / lastTurnAt / 指纹。做法是删掉
// provider_sessions 里那一整行（经 Service.DropProviderSession），不是只
// 清指纹——10_STEP7_SPEC.md 第 4 节那条原样适用：重建意味着 provider 那边
// 计数器从零开始，只作废会话 id 而留着旧成本基线，下一轮差值是负数，
// computeCostDelta 返回 nil，那一笔真实花掉的钱就静默从账上消失了。
//
// 同时把这条 gate 的 st 置 nil（gate 本身不删，见 sessionGate 的不变量），
// 否则下一轮 Ask 还会读到旧的（in-memory 分支会先比指纹，指纹当然还是
// 旧的那份，不会重新去读库）。
//
// 取 g.mu 用 TryLock，拿不到就返回包了 ErrTurnInFlight 的错误，且什么
// 都不做——尤其不许先 DropProviderSession 再发现拿不到闸。拒绝不只是
// 因为「阻塞 5 分钟不可接受」：即便等到了也白做，在飞那一轮结束时
// SetProviderSession 是 upsert，会把刚删的行又写回来，用户看到「已丢弃」
// 而什么都没丢。拒绝是 fail-closed 的那一边。
//
// existed 报告本来有没有这条会话（数据库里的行，或者内存里的快照，
// 两者任一存在都算「本来有」）。不存在不是错误：对一个还没说过话的成员
// 敲 rebuild，回一句「本来就没有」就够了。
//
// 不检查 agentID 是不是仍在配置里：这个方法存在的意义之一就是清掉
// 已经从配置里删掉的 agent 留下的悬空行（AuditDrift 的「配置里已无此
// agent」那一支正是为它准备的），要求它必须先在配置里会让最需要清理的
// 那一行反而清不掉。
func (r *Runner) Rebuild(ctx context.Context, convID, agentID string) (existed bool, err error) {
	g := r.gate(sessionKey{convID: convID, agentID: agentID})
	if !g.mu.TryLock() {
		return false, fmt.Errorf("rebuild %s/%s: %w", convID, agentID, ErrTurnInFlight)
	}
	defer g.mu.Unlock()

	dbExisted, err := r.svc.DropProviderSession(ctx, convID, agentID)
	if err != nil {
		return false, fmt.Errorf("rebuild %s/%s: %w", convID, agentID, err)
	}

	memExisted := g.st != nil
	g.st = nil

	// 两处都可能各自「有」：数据库那一行与内存快照本来就是分开维护的
	// 两份状态（进程重启之后内存是空的，库里却还有；或者反过来，一轮
	// 失败之后内存记了 baselineSuspect 但从未真正落过库）。existed 只要
	// 有一处「曾经有」就算有，这样用户看到的是「有没有东西被丢掉」，
	// 不是「数据库那一份巧合地在不在」。
	return dbExisted || memExisted, nil
}

// Ask 让 agentID 在 convID 里说一轮话，返回这一轮的完整记账。
// 三参数签名是冻结契约，一个字不改。自己铸一个 roundID，取一次
// 规则/配置快照，再调 askUpToWith(..., 0, 那个 id)。upTo = 0 表示不设上界。
//
// 即使返回 error，Record 也有意义：Outcome、Usage、ProviderSession、
// MaybeAdvanced 都已填好，调用方据此决定屏幕上说什么、要不要重发。
func (r *Runner) Ask(ctx context.Context, convID, agentID string) (Record, error) {
	roundID, mintErr := newV4UUID()
	if mintErr != nil {
		rec := Record{ConvID: convID, AgentID: agentID, Outcome: OutcomeRefused}
		rec.Err = fmt.Errorf("mint round id: %w", mintErr)
		return rec, rec.Err
	}
	rules, cfg := r.snapshotRulesConfig()
	return r.askUpToWith(ctx, convID, agentID, 0, roundID, rules, cfg)
}

// AskUpTo 是带 seq 上界的公开入口。upTo = 0 表示不设上界。
// 签名已在 09a 登记，一个字不改。
//
// roundID 必须由调用方铸好传入，本方法自己不铸：turn_usage.round_id
// 的裁法是「每一轮都有 round_id，/ask 是一轮之一」。必须有人在扇出
// 之前铸一个 id 发给全部 N 个成员，否则 N 个成员各铸各的，按轮聚合
// 会拿到 N 个不同的 id。Ask 自己铸一个，于是那一列永远有值，不会
// 退化成半空列。
//
// roundID 只记账：绝不进 prompt、绝不进身份块、绝不进指纹。写入
// turn_usage.round_id（片 2 已加列）。
//
// 一进来取一次规则/配置快照再交给 askUpToWith。RunRound 不走本方法，
// 它自己取一次快照后对每个成员调 askUpToWith——否则五个 goroutine
// 各自进本方法就是五次快照，轮粒度是假的。
//
// 代价：同一 (conv, agent) 上排队等闸的调用方，本轮用的是它入队那一刻
// 的规则与配置快照。期间发生的 SetRules / SetConfig --apply 对这一轮
// 不生效，turn_usage.identity_fp 落库为旧值，且不报 drift（旧规则算出
// 的指纹与 g.st 里已经存的那个——同样是旧规则算出来的——相符，
// askUpToWith 里的指纹比对因此悄悄放行）。这个窗口今天不可达：
// PlanRound 对 Speakers 去重，一轮之内每个 (conv, agent) 只出现一次，
// 两个 goroutine 不会争同一把闸。
//
// 快照拍在拿闸之前，与 AuditDrift / RunRound 同一位置（见 cfgMu 字段的
// 注释）：RunRound 一份快照要交给 N 把闸，本来就拍不到任何一把闸里面；
// Ask / AskUpTo 跟着放在同一位置，三条入口对「哪一刻的规则/配置进这一轮」
// 只有一种答案。上面这份代价是这个选择的直接后果。
//
// 顺序固定，没有任何按 provider 或按「第几轮」的分支，完整八步见
// 09_STEP6_SPEC.md H 节。
func (r *Runner) AskUpTo(ctx context.Context, convID, agentID string, upTo int64, roundID string) (Record, error) {
	rules, cfg := r.snapshotRulesConfig()
	return r.askUpToWith(ctx, convID, agentID, upTo, roundID, rules, cfg)
}

// snapshotRulesConfig 拷走当前规则与配置指针，立刻放锁。
// 不在持有 rulesMu / cfgMu 时做 IO，也不取任何 g.mu。
// 配置必须整体换指针（见 SetConfig）；这里拷的是指针，底下的 map
// 若被原地改，轮粒度就是假的。
func (r *Runner) snapshotRulesConfig() (RulesBlocks, *config.Config) {
	r.rulesMu.RLock()
	rules := r.rules
	r.rulesMu.RUnlock()
	r.cfgMu.RLock()
	cfg := r.cfg
	r.cfgMu.RUnlock()
	return rules, cfg
}

// askUpToWith 是 Ask / AskUpTo / RunRound 的共用实现。
// rules 与 cfg 必须由调用方已经快照好传入，本函数不再读 r.rules / r.cfg。
func (r *Runner) askUpToWith(ctx context.Context, convID, agentID string, upTo int64, roundID string, rules RulesBlocks, cfg *config.Config) (Record, error) {
	rec := Record{ConvID: convID, AgentID: agentID, RoundID: roundID}
	id, mintErr := newV4UUID()
	if mintErr != nil {
		rec.Outcome = OutcomeRefused
		rec.Err = fmt.Errorf("mint turn id: %w", mintErr)
		return rec, rec.Err
	}
	rec.TurnID = id

	key := sessionKey{convID: convID, agentID: agentID}
	g := r.gate(key)
	g.mu.Lock()
	defer g.mu.Unlock()

	var st *sessionState
	// began 只在落点 A 的 BeginTurn 成功返回之后置 true，此后永不置回。
	// 所有 refuse / failClosed 以及三处 RecordTurn 的 expectInflight 都传它，
	// 不在调用点写字面量。refuse 有十几个调用点，其中只有一个在 BeginTurn
	// 之后；让每个调用点自己声明 true/false，写错的方向不对称：该传 false
	// 却传了 true，RecordTurn 看到 expectInflight 而 ClearInflight 报
	// existed=false，返回 ErrInflightMissing 并把整笔回滚，一轮被拒绝的、
	// 本该记账的轮就从账上消失了。传一个派生变量，新加的调用点自动拿到正确的值。
	began := false

	// 起进程之前先看库里有没有别人的在飞记录。查到的那一行一定不属于本轮：
	// 本轮的 turn_id 刚铸好，自己的 BeginTurn 还没跑，库里不可能有本轮的行。
	// 生产里每个进程（每个 data_dir）只有一个 Runner（app.go 装配处），闸在
	// Runner 上，所以同一 (conv, agent) 在本进程里的每一轮都在同一把 g.mu 下
	// 写入，并在释放闸之前经 RecordTurn 清掉（failClosed 与成功路径用
	// Background 落库；BeginTurn 之后仍走 refuse 的几支——SetInflightPhase
	// 失败、命令行超长或 shim 参数带换行——用的是调用方 ctx）。闸空闲时还留着
	// 的行只可能是没对上账的：上次崩溃、settle 失败、本进程里一次清理也失败
	// 的双重故障，或 BeginTurn 之后、起进程之前的一次取消（refuse 拿着已取消
	// 的 ctx，清理那笔写不进，这一行会留到重启对账）。不同 key 的行
	// (conv_id, agent_id) 不同，定点读看不到。另一个 council 进程开不了
	// 同一个库（store.ErrDataDirLocked）。SettleLostTurns 只在启动时、进
	// REPL 之前跑。所以不需要内存里的 turn id 清单，「查库里有没有这一行」
	// 就是结构性的判据，实现上不需要新状态。
	//
	// 检查放在闸之内：放在闸外，同一 key 的两次并发调用会都看到「没有」然后
	// 都去 BeginTurn。唯一索引是第二道。
	//
	// 放在任何副作用之前：原来撞唯一索引之前已经发了 session_opening，指纹
	// 不符时已经删了 settle 要标的那一行。后者只对 spawned / returned 行成立；
	// begun 行只是提前删了一行对账不会去标的记录。
	//
	// 怎么解开：重启 council，启动对账成功就删掉这一行；或者 /session rebuild
	// <成员>，它经 SettleStaleInflight 按启动对账的同一条路结清这一行（拿到这一
	// key 的闸之后才读，所以结清的只可能是陈旧行），再丢掉 provider 会话。
	// 结清本身失败（例如 turn_usage 主键冲突）时两条路都解不开。
	//
	// 被拒的这一轮照常记一行 refused 的 turn_usage，identity_fp 与
	// provider_session_id 是空串，与「配置里没有这个 agent」那几支早拒绝
	// 同一种形状。
	prior, priorFound, priorErr := r.svc.InflightFor(ctx, convID, agentID)
	if priorErr != nil {
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("read inflight %s/%s: %w", convID, agentID, priorErr))
	}
	if priorFound && prior.TurnID != rec.TurnID {
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("agent %q: unsettled turn %s (phase %s): %w", agentID, prior.TurnID, prior.Phase, conversation.ErrInflightConflict))
	}

	if cfg == nil || cfg.Agents == nil {
		// 配置都没有，连「这个名字在不在」都无从判。不这样做会在下面的
		// map 读上 panic，或者拿着空 agent 去 Lookup，错误落在错误的哨兵上。
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("agent %q: %w", agentID, ErrAgentUnknown))
	}
	ac, ok := cfg.Agents[agentID]
	if !ok {
		// 配置里根本没有这个 id。与「有 agent 但 provider 没适配器」必须分开：
		// 合成一个的话，用户会去改 agent id，而问题其实在 provider 那一栏。
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("agent %q: %w", agentID, ErrAgentUnknown))
	}
	p, err := provider.Lookup(ac.Provider)
	if err != nil {
		// 不另造 ErrNoAdapter：registry 是「有没有适配器」的唯一权威，
		// Lookup 已经返回 provider.ErrUnknownProvider。再造一个哨兵会让
		// 调用方必须同时认两个（09a 按 09 裁定删掉了那个重复哨兵）。
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("agent %q provider %q: %w", agentID, ac.Provider, err))
	}
	caps := p.Capabilities()

	tier, err := TranslatePermission(ac.Permissions)
	if err != nil {
		// full、或拼错的档在起进程之前拒绝（06 第 2 节：FULL 不给编排器
		// 自动使用）。workspace 已在 11c 第二片放行，不再落进这一支。
		// 不这样做会把明令不提供的档送进 CLI，失败要响亮，而且要在花钱
		// 之前失败。
		return r.refuse(ctx, &rec, st, began, err)
	}
	if r.workDir == "" {
		// 空基目录拼出来的路径是相对的，子进程会继承启动目录。
		// 三家 CLI 会读到 cwd 下的项目级指令文件，入场费与行为随启动目录漂移，
		// 拿到写盘权限的成员还会把文件写进那个仓库。起进程之前拒绝。
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("agent %q: %w", agentID, provider.ErrNoWorkDir))
	}
	dir := agentWorkDir(r.workDir, convID, agentID, tier)

	// 每轮起进程之前 MkdirAll，取代 app.go 里那个「启动时一次建齐」的旧循环
	// （11c 契约第 4 节）：那个循环只知道 agentID，不知道 convID，有了会话
	// 维度之后 WORKSPACE 那一档的目录建不全。READ 那一档的目录同样挪到这里
	// 建，理由是它与 WORKSPACE 共用同一处「起进程之前」的拒绝路径，不必再
	// 维护第二套「谁在哪建目录」的规则。建不出来按 refuse 处理，不起进程。
	if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("agent %q: mkdir %s: %w", agentID, dir, mkdirErr))
	}

	// 送出去的是带规则的整块，进指纹 role 段的是不带规则的核心 —— 两者
	// 分开算的理由见 renderIdentityCore 的注释。identityFPFor 与 AuditDrift
	// 共用同一份计算，理由见它自己的注释：两处但凡有一处漏改，审计就会
	// 报出一堆不存在的漂移，或者反过来漏报真实的漂移。
	identity := renderIdentity(agentID, ac, rules)
	fp, fpErr := identityFPFor(p, agentID, ac, r.workDir, convID, tier, rules)
	if fpErr != nil {
		// 探针组不出命令行（权限档不放行、agy 的规则文件把探针正文顶过
		// MaxPromptBytes……）：与其它起进程之前的拒绝走同一条路，不吞。
		return r.refuse(ctx, &rec, st, began, fpErr)
	}
	// 算出来立刻赋上：在它之后发生的任何一条拒绝路径上这一列也有值。
	rec.IdentityFP = fp

	st = g.st
	if st != nil {
		if st.identityFP != fp {
			// 进程内已有的那条不符：用户在会话开着的时候改了配置。
			// 静默重建会白烧一次入场费（0.4 到 1.0 美元）且他不知情，
			// 所以拒绝，不闷头开新会话。不拒绝的话会拿着旧会话 id 去
			// --resume，前缀已按旧身份建好，模型继续用旧脑子说话，
			// 库里却写着新 model / effort。
			return r.refuse(ctx, &rec, st, began, identityDriftError(st.identityFP, fp))
		}
	} else {
		// 读回之后立刻比指纹：相符才放进 g.st，不符就当作没读到。
		// 这样后面看到的 st 一定是「本进程内早先建立的」，
		// 两种处置才分得开 —— 若先放进 g.st 再比，两条分支会塌成
		// 同一种「闸里有一条指纹不对的记录」。
		//
		// 从库读回的那条不符：两次运行之间改配置本来就允许，此时
		// 没有「正在开着的会话」可言，作废重建（丢弃五样，按 Fresh
		// 走），不报错。作废必须五样全废：重建意味着 provider 那边
		// 是一条新会话、累计计数器从零开始，若只作废会话 id 而留着
		// 旧基线，下一轮的差值是负数，computeCostDelta 返回 nil，
		// 那一笔真实花掉的钱就静默从账上消失了。
		loaded, found, loadErr := r.svc.ProviderSession(ctx, convID, agentID)
		if loadErr != nil {
			// 读不出来是「不知道」，不是「没有」：st 仍是 nil，下面
			// persistTurn 传给 RecordTurn 的 session 也会是 nil。未决 #20
			// 之前这里会被旧语义误读成「作废」而删掉一条可能还在、只是
			// 这次读失败的会话；现在 nil 读作「不碰」，这一支顺带被修对。
			return r.refuse(ctx, &rec, nil, began, loadErr)
		}
		if found && loaded.IdentityFP == fp {
			st = sessionStateFrom(loaded)
			g.st = st
		} else {
			if found {
				// 库里确实有过一条状态，指纹却不符：这一刻旧指纹还在手上，
				// 是唯一能点名「到底哪一项变了」的时机——过了这一刻它就被
				// 下面 st = &sessionState{} 覆盖没了。未决 #18 点名的正是这件事：
				// 这条路径今天静默作废重建，除了改 model / effort 之外几乎
				// 没什么会触发它，于是没人注意到它其实一直在发生。
				if noticeErr := r.postRebuildNotice(ctx, convID, agentID, loaded.IdentityFP, fp); noticeErr != nil {
					// 面包屑都写不进去就不要去花钱，与 session_opening 那支同理。
					return r.refuse(ctx, &rec, nil, began, fmt.Errorf("session_rebuilt: %w", noticeErr))
				}
				// 未决 #20 裁决：这一刻才是「决定作废」真正发生的地方，必须在这里
				// 显式删行，不能再指望本轮末尾 RecordTurn 的 session=nil 顺带删掉——
				// 那个含义已经改成「不碰这一行」了。不显式删的话，这一轮若随后失败
				// （还没起进程），bootstrap 的 st.providerSession 仍是空串，
				// persistTurn 传给 RecordTurn 的 session 会是 nil，新语义下什么都
				// 不会发生，库里就会留着一条指纹已经不符、却没有任何进程认领的行——
				// 正是本条要防的那类孤儿状态。
				if _, dropErr := r.svc.DropProviderSession(ctx, convID, agentID); dropErr != nil {
					return r.refuse(ctx, &rec, nil, began, fmt.Errorf("session_rebuilt drop: %w", dropErr))
				}
			}
			st = &sessionState{}
		}
	}

	// 「要不要新建会话 / 是不是首轮」只有这一个出处：ProviderSession
	// 是不是空串。Select / Render 的 bootstrap、session_opening、
	// ErrNoDelta 是否推游标、失败时 session 传不传，全部用它。
	bootstrap := st.providerSession == ""
	rec.Fresh = bootstrap
	rec.BaselineSuspect = st.baselineSuspect
	rec.ColdExpected = coldExpectedAt(r.now(), st.lastTurnAt, bootstrap)

	scanned, err := r.readAfter(ctx, convID, st.cursor)
	if err != nil {
		return r.refuse(ctx, &rec, st, began, err)
	}
	// upTo 保证的是一轮内的确定性：同一批 delta 不依赖谁先答完。
	// 它不保证跨轮重发的逐字节相同 —— 一个 MaybeAdvanced 的成员在下一轮
	// 重发时 upTo 变了，那一批里会多出这一轮另外 N-1 个人的发言。
	// 后者在只追加的事实源上本来就做不到。
	//
	// 截断只发生在内存里，不改 conversation.Store.Messages 的签名。
	// upTo = 0 表示不设上界。ScannedTo 跟着截断后的批次走，等于留下
	// 那些消息里的最大 seq；一批被滤空时退回游标。
	if upTo > 0 {
		kept := scanned[:0]
		for _, m := range scanned {
			if m.Seq <= upTo {
				kept = append(kept, m)
			}
		}
		scanned = kept
	}
	rec.ScannedTo = st.cursor
	for _, m := range scanned {
		if m.Seq > rec.ScannedTo {
			rec.ScannedTo = m.Seq
		}
	}
	if len(scanned) > 0 && scanned[0].Seq <= st.cursor {
		// History 契约是 seq 严格大于 afterSeq。落到这里说明游标与事实源
		// 已经错位：再送一遍会改写已投递的前缀。缓存前缀会断，而打断前缀时
		// 没有任何一层会报错（agent 照常回答、退出码 0、JSON 合法、游标照推），
		// 只是每轮贵几十倍：冷启动 47749 token / 0.955 美元，暖缓存边际 0.014 美元。
		// 不起进程，钱还没花。
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("min seq %d <= cursor %d: %w", scanned[0].Seq, st.cursor, ErrPrefixRewrite))
	}

	conv, err := r.svc.Conversation(ctx, convID)
	if err != nil {
		return r.refuse(ctx, &rec, st, began, err)
	}

	routeThought, visErr := r.thoughtVisibleFor(conv.TeamID, ac.Mode, cfg)
	if visErr != nil {
		// 起进程之前拒绝，session_opening 还没发。st 在指纹相符时已经
		// 从库读回，persistTurn 原值回写；st 仍是 bootstrap 空会话或 nil
		// 时 snapshotProviderSession 交出 nil，未决 #20 读作「不碰这一行」，
		// 不会把已建立的 provider 会话删掉。
		return r.refuse(ctx, &rec, st, began, visErr)
	}
	router := NewRouter(routeThought)
	selected := router.Select(scanned, ac, agentID, bootstrap)
	prompt := router.Render(selected, conv.Title, bootstrap)
	rec.Delivered = deliveredSeqs(selected)
	rec.PromptBytes = len(prompt)

	if len(selected) == 0 {
		// 连着敲两次 /ask X：不起进程。空 prompt 送给 claude -p 多半直接报错，
		// 而那次冷启动的钱已经付了。游标照常推到 ScannedTo。
		//
		// 只在已经有会话记录时推：bootstrap 时还没有 provider 会话，若为了记游标
		// 插一条空 sessionState，下一轮 bootstrap 会变成 false，session_opening
		// 发不出来，但 ProviderSession 仍是空串，BuildTurn 还是会开一条新 CLI
		// 会话。没有会话就没有游标可推，下次再扫一遍被过滤掉的 KindSystem 是免费的。
		if !bootstrap {
			st.cursor = rec.ScannedTo
		}
		return r.refuse(ctx, &rec, st, began, ErrNoDelta)
	}

	if bootstrap {
		// 时机不能改：契约要求这条面包屑在起进程之前发。进程一起，冷启动的
		// 钱就花了，哪怕这一轮后来失败。措辞必须承受「下一条可能是
		// turn_failed」——只追加、永不改写的事实源里不能留下一条「已开启」
		// 而那条会话根本没开成。
		sys, postErr := r.postSystem(ctx, convID, agentID, eventOpening, openingText(agentID, st.spoken), nil)
		if postErr != nil {
			// 面包屑都写不进去就不要去花钱。不拒绝的话，spawn 失败后事实源
			// 里连「尝试过」都没有，排查只能靠屏幕上已经滚走的那一行。
			return r.refuse(ctx, &rec, st, began, fmt.Errorf("session_opening: %w", postErr))
		}
		rec.System = sys
	}

	req := provider.TurnRequest{
		AgentID:         agentID,
		DisplayName:     ac.DisplayName,
		Model:           ac.Model,
		Effort:          ac.Effort,
		Role:            identity,
		Permission:      tier,
		ProviderSession: st.providerSession,
		Prompt:          prompt,
		WorkDir:         dir,
		// MaxTurns 恒为 0：Step 6 不设 --max-turns（06 第 5 节）。写出来而不是
		// 靠结构体零值，是为了让读代码的人知道这是故意留给下一步的，不是漏填。
		// 真去设上限会让 Completion=Truncated 的成功轮混进「我们没截过」的假设。
		MaxTurns: 0,
	}
	cmd, err := p.BuildTurn(req)
	if err != nil {
		// prompt 超长、档位不支持、能力项仍是 Unknown：都是起进程之前就能判定
		// 的拒绝。不拦截会把注定失败的命令行送进 CLI，冷启动的钱白付。
		return r.refuse(ctx, &rec, st, began, err)
	}
	if cmd.Path == "" && cfg.Providers != nil {
		// 适配器不一定认识配置里的可执行文件路径。空 Path 起进程会变成
		// 「找不到命令」，错误落在 spawn 而不是配置。
		cmd.Path = cfg.Providers[ac.Provider].Command
	}
	if cmd.Dir == "" {
		// 回填的是这个 agent 自己的工作目录，不是 provider 名拼出来的目录。
		// 两个 claude agent 必须各有各的 cwd；按 provider 回填会让他们共享
		// 指令文件与落盘位置。BuildTurn 本应从 req.WorkDir 抄进 Command.Dir，
		// 这里再拦一次是因为测试会换 Spawner，生产路径不能把空 Dir 放过去。
		cmd.Dir = dir
	}

	// 落点必须在 BuildTurn 之后、起进程之前。再早拿不到 cmd.PlannedSessionID：
	// 它由 p.BuildTurn 现铸，不是调用方事先知道的。崩在 bootstrap 轮时那个
	// id 是唯一能点名 provider 那边那条孤儿会话的东西，漏记就再也对不上。
	//
	// ctx 用调用方的，不用 Background()。此刻还没花钱，spawn 之前取消就该
	// 连在飞记录也不写。清理路径用 Background() 是另一回事，那时钱已经花了。
	//
	// WorkDir 取 cmd.Dir，不取上面那个 dir。子进程真正跑在 cmd.Dir 里；
	// 崩溃后「可能有半截落盘的文件」的是那个目录。两者在回填之后通常相同，
	// 但适配器自己填了 Dir 时 dir 是我们算的、cmd.Dir 才是它跑的。
	if err := r.svc.BeginTurn(ctx, conversation.InflightTurn{
		TurnID:           rec.TurnID,
		ConvID:           convID,
		AgentID:          agentID,
		Provider:         ac.Provider,
		PlannedSessionID: cmd.PlannedSessionID,
		ResumeSessionID:  req.ProviderSession,
		CursorSeq:        st.cursor,
		ScannedTo:        rec.ScannedTo,
		StartedAt:        r.now(),
		Phase:            conversation.PhaseBegun,
		Tier:             string(tier),
		WorkDir:          cmd.Dir,
	}); err != nil {
		// 失败绝不能继续往下走去起进程：库里没有这一行，进程却已经起来，
		// 正是本刀要补的那个洞。此刻 began 仍是 false，refuse 里的
		// expectInflight 因此是 false，不会因为没有在飞记录而把用量回滚掉。
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("begin turn: %w", err))
	}
	began = true

	timeout := r.timeout
	if timeout <= 0 {
		// 零或负数不能表示「无限等」：对 grok 那就是把主路径的硬超时关掉。
		timeout = defaultAskTimeout
	}

	// r.spawn 不是「把进程起起来」，是「起起来并等它跑完」：spawnProcess
	// 在同一次调用里 exec.Command、Start、Wait。spawned 必须写在调用它之前。
	// 写在返回之后，整个子进程生命期（硬超时 5 分钟）都留在 begun 那一档，
	// 而对账把 begun 读作「进程没起、没花钱」——崩在这一段里会少说一次钱。
	//
	// 代价是 ErrCommandLineTooLong / ErrShimNewlineArg。这两道闸在
	// spawnProcess 里、exec.Command 之前返回，此刻 phase 已经是 spawned，
	// 一个进程都没起。正常流程它们走 refuse，清理把行删掉；只有清理也失败
	// 的双重故障才漏得下一条假阳性。这笔交换的方向是有意的：宁可双重故障时
	// 多说一次「可能花了钱」，不可单次崩溃时少说一次。
	//
	// ctx 仍用调用方的：还没进 spawn，取消就该连这一档也不写，并且不起进程。
	// 落点在 spawnCtx 之前，没有 cancel() 要管。
	if err := r.svc.SetInflightPhase(ctx, rec.TurnID, conversation.PhaseSpawned); err != nil {
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("set inflight phase spawned: %w", err))
	}

	spawnCtx, cancel := context.WithTimeout(ctx, timeout)
	start := time.Now()
	stdout, stderr, exitCode, killed, spawnErr := r.spawn(spawnCtx, cmd)
	rec.WallMS = int(time.Since(start) / time.Millisecond)
	cancel()

	if killed || errors.Is(spawnErr, context.DeadlineExceeded) || errors.Is(spawnErr, context.Canceled) {
		// 失败就什么都不写：不写 session、不推游标、不推基线。grok 首轮预铸的
		// PlannedSessionID 在这里一律作废，下一轮重铸。未经输出确认的 id 拿去
		// --resume，grok 会挂死在 OAuth 上。
		//
		// 判别式精确的原因：spawnCtx 来自 context.WithTimeout(ctx, timeout)，
		// 上面的 cancel() 取消的是 spawnCtx 不是父 ctx，所以此刻读 ctx.Err()
		// 仍然准。父 ctx 非 nil 说明取消来自上层（Ctrl-C，main.go 的
		// NotifyContext），标 OutcomeCanceled；否则只有 spawnCtx 到点，即墙钟，
		// 标 OutcomeTimeout。两支 maybeAdvanced 都是 true：进程已经跑过，
		// 结局未知，变的只是标签，让记账分得开。
		out := OutcomeTimeout
		if ctx.Err() != nil {
			out = OutcomeCanceled
		}
		return r.failClosed(ctx, &rec, st, began, bootstrap, out, true, diagnose(agentID, cmd, exitCode, stderr, stdout, spawnErr, true))
	}
	if errors.Is(spawnErr, ErrCommandLineTooLong) || errors.Is(spawnErr, ErrShimNewlineArg) {
		// 未决 #19 与 #21：这两道闸命中时都是一个进程都没起（都在 spawnProcess
		// 里，比 exec.Command 更早），性质是「起进程之前拒绝」，与
		// TranslatePermission 不放行、prompt 超长同类，不是「起了进程但失败」。
		// 归到 spawn_failed 会让这个计数被拿去反推「起过多少次进程」时多算。
		// #19 当初只订正了 ErrCommandLineTooLong 一个（契约第 5 节只点了它），
		// #21 指出 ErrShimNewlineArg 是同一个分类问题、同一个改法、同一个理由。
		//
		// wall_ms 显式清零：time.Since(start) 量到的那一点点耗时只是这两道闸
		// 自己的校验开销，不是等一个真实子进程的墙钟，混进 spawn_failed /
		// timeout 的墙钟统计里会污染那两档的含义。
		rec.WallMS = 0
		return r.refuse(ctx, &rec, st, began, spawnErr)
	}
	if spawnErr != nil {
		// 进程压根没起来（不在 PATH、本地转发服务 睡着、立刻非零退出）。更没有会话 id
		// 可留：留下一个 provider 侧从未存在过的 id，下一轮会永远 --resume 它。
		return r.failClosed(ctx, &rec, st, began, bootstrap, OutcomeSpawnFailed, false, diagnose(agentID, cmd, exitCode, stderr, stdout, spawnErr, false))
	}

	res, parseErr := p.ParseTurn(stdout)
	if parseErr != nil {
		outcome := OutcomeUnparsable
		if errors.Is(parseErr, provider.ErrTurnFailed) {
			outcome = OutcomeProviderFailed
		}
		// ErrTurnFailed 等路径用量与会话 id 仍然有效（钱花了），先吸收再 fail-closed。
		// 不吸收的话付过钱的数字只活在这段即将丢弃的 stdout 里。
		// ErrUnparseableOutput 对应零值 TurnResult，吸收会把「不知道」写成 &0，跳过。
		if !errors.Is(parseErr, provider.ErrUnparseableOutput) {
			r.absorbParse(&rec, res)
		}
		return r.failClosed(ctx, &rec, st, began, bootstrap, outcome, false, diagnose(agentID, cmd, exitCode, stderr, stdout, parseErr, false))
	}
	if res.ProviderSession == "" {
		// 适配器本应返回 ErrNoSessionID。静默把空串当 id 存进去，下一轮就是
		// 一次冷启动；对 grok 更糟 —— 拿空/陈旧 id 去 resume 会挂死在 OAuth 上。
		r.absorbParse(&rec, res)
		return r.failClosed(ctx, &rec, st, began, bootstrap, OutcomeUnparsable, false, diagnose(agentID, cmd, exitCode, stderr, stdout, provider.ErrNoSessionID, false))
	}

	sentID := req.ProviderSession
	if sentID == "" {
		sentID = cmd.PlannedSessionID
	}
	rec.Drifted = sentID != "" && res.ProviderSession != sentID
	r.absorbParse(&rec, res)

	// 必须用 Background()。成功路径此刻钱已经花了、provider 那边真多了一轮。
	// 父 ctx 若已被 Ctrl-C 取消，用它写会失败，于是一轮我们确知 provider
	// 说过话的轮，崩溃之后会被报成 spawned（「provider 侧可能已经发生」），
	// 把一件已知的事说成不知道。解析失败与空会话 id 那两支不写 returned：
	// 它们随后走 failClosed，phase 留在 spawned 才是对的。
	//
	// 写失败吞掉，不改变本轮结局，也不写进 rec.Err。写不进去的唯一后果是：
	// 若紧接着的 RecordTurn 也没跑完就崩了，下次启动的面包屑措辞偏严
	// （仍说 spawned，而其实已经 returned）。为一句措辞让一轮已经花过钱、
	// 已经拿到正文的成功轮失败，是拿一次真实的损失去换一句措辞。
	_ = r.svc.SetInflightPhase(context.Background(), rec.TurnID, conversation.PhaseReturned)

	rec.ProviderSession = res.ProviderSession
	rec.CostUSDDelta = computeCostDelta(st, rec.Drifted, rec.Usage)
	// Cold 取 CacheRead == 0（05 第 4 节），只有走到这里的轮才赋值：失败的各条路在上面
	// 经 failClosed 返回，Cold 留在零值。所以读冷暖要经 Record.MeasuredCold，不直接读它。
	// UnexpectedCold 排除 Unreported 含 cache_read 的情况：那一档的 0 表示
	// 「没报」不是「没命中」，拿它告警是在对「不知道」按「冷启动」处理。
	rec.Cold = rec.Usage.CacheRead == 0
	rec.UnexpectedCold = !rec.ColdExpected && rec.Cold && !unreportedHas(rec.Usage.Unreported, "cache_read")

	meta := r.agentMeta(caps, cmd, rec, res, ac.Effort)
	msg := conversation.Message{
		ConvID:         convID,
		SenderID:       agentID,
		SenderProvider: ac.Provider,
		SenderModel:    ac.Model,
		SenderEffort:   ac.Effort,
		Kind:           conversation.KindAgent,
		Text:           res.Text,
		Thought:        res.Thought,
		Meta:           meta,
	}
	// #45「总结只给 维护者 看」。可见性只有 recipientOK（router.go）这一条：
	// 名单只有它自己，其余成员 bootstrap 与增量两条路都挑不到，它自己又被
	// self 规则排除，所以没有任何成员的 prompt 会带上这条。/log 不过滤，
	// 照样看得到（TO 列是它自己）。填 [自己] 而不是 ["user"]：user 是保留
	// sender、不是参与者，Service.Post 的收件人校验会拒它；RecordTurn 今天
	// 不校验收件人，将来补上时 [自己] 仍然合法。按配置判、不按调用路径判，
	// 所以 /ask <总结成员> 也只给用户看。cfg 是这一轮入场时的快照，与指纹同一份。
	if cfg.Summary.Agent != "" && agentID == cfg.Summary.Agent {
		msg.Recipients = []string{agentID}
	}
	r.commitSession(g, st, &rec, fp, res)
	session := snapshotProviderSession(convID, agentID, st)

	rec.Outcome = OutcomeSpoke
	seqHold := int64(0)
	u := makeTurnUsage(&rec, &seqHold, r.now())
	// 成功返回之后落库一律用 context.Background()：provider 已经跑完、钱花了、
	// 对面真多了一轮。此刻父 ctx 若已被 Ctrl-C 取消，用它落库会全部失败，变成
	// OutcomePostRejected，补救的两次（下面 postSystem 面包屑、以及 RecordTurn
	// 只记账不落正文那条）再用同一个死 ctx，一起失败，provider 会话 id 一个字
	// 没落库，留下一条孤儿会话。/round --go 把这个暴露面乘以 N。本地 SQLite
	// 写是亚毫秒级的，拿不到取消能力不构成风险。failClosed 内部已经用
	// context.Background()，成功路径与它对齐。起进程之前的 opening 那条
	// postSystem 仍用调用方 ctx，那时还没花钱。
	//
	// expectInflight 回答的是「这一轮应当有一条在飞记录等着被清掉」。只有
	// turn 侧知道答案：conversation 把 Outcome 存成裸 int，就是为了不认识它。
	// began 在 BeginTurn 成功之后才是 true；此前拒绝的轮传 false，ClearInflight
	// 删不到也照常提交，turn_usage 不会因为没有在飞记录而整笔回滚。
	stored, postErr := r.svc.RecordTurn(context.Background(), &msg, u, session, began)
	if postErr != nil {
		// provider 吃下了、钱花了、会话已经多了一轮，只是本地记不上。
		// 不重试：重试每次都能拿到回答、每次都写不进去，只是重复烧钱。
		// 会话状态照常推进，否则下一轮重发同一批 delta，与 CLI 侧的历史永久错位。
		rec.Outcome = OutcomePostRejected
		rec.DroppedText = res.Text
		rec.Err = fmt.Errorf("post agent message: %w", postErr)
		rec.Message = nil
		extra := map[string]any{MetaTextDropped: true}
		r.attachSessionMeta(extra, rec)
		// 落库用 Background，理由见上面 RecordTurn(&msg, ...) 那条。
		sys, sysErr := r.postSystem(context.Background(), convID, agentID, eventPostRejected, postRejectedText(agentID), extra)
		if sysErr == nil {
			rec.System = sys
		}
		// 第一次 RecordTurn 把 msg 和用量放进同一个 WriteTx。AppendMessage
		// 被拒（典型：飞行中 /leave，事务内 RequireActiveParticipant 拒掉）
		// 时整轮回滚，库里不会留下 turn_usage 行。钱已经花了，账仍要记 ——
		// 所以改用 msg=nil 再调一次，只写用量与当前会话，不再尝试追加正文。
		// 不在 RecordTurn 内部偷偷拆成「消息回滚、用量提交」，那会让
		// 「任一步失败整轮回滚」变成有例外的约定；两次调用是调用方义务。
		u = makeTurnUsage(&rec, nil, r.now())
		// 落库用 Background，理由见上面 RecordTurn(&msg, ...) 那条。
		// expectInflight 仍传 began：第一次 RecordTurn 整笔回滚，在飞那一行
		// 还在，补救这次同样要把它清掉。传 false 会让成功提交之后行还留着，
		// 下次启动把一轮已经落了账的轮再报成在飞。
		if _, recErr := r.svc.RecordTurn(context.Background(), nil, u, session, began); recErr != nil && rec.Err == nil {
			rec.Err = recErr
		}
		return rec, rec.Err
	}

	rec.Outcome = OutcomeSpoke
	rec.Message = stored
	rec.Err = nil
	return rec, nil
}

// SessionInfo 返回 (convID, agentID) 这条 provider 会话的当前状态，供 /session 用。
// ok 为 false 表示这个 agent 在这条会话里**从来**没开过口 —— 下一轮 Ask 会是 Fresh。
// 这条语义一个字不改：/round 的冷启动预报依赖它，ok == false 算必冷。
//
// 它先看进程内存的快照，没有再回库读一次（Step 7 加的）。
// 只看内存是 Step 6 的做法，在 Step 7 之后会说谎：2026-09-21 的真机冒烟里，
// 重启之后第一次 /session 显示这个 agent 是 fresh、会话 id 是占位符，
// 而那条会话明明躺在 provider_sessions 里，紧接着的 /ask 就用回了它。
// 「还没开过口」与「本进程还没读回来」是两件事，混成一件会让 /session
// 在最需要它的时刻（刚重启、想知道下一轮冷不冷）给出相反的答案。
//
// 读回不改变任何状态：不写进 g.st，也不做指纹校验。指纹校验归 Ask ——
// 那里指纹不符要作废重建，而 /session 只是看一眼，不该产生副作用。
// 代价是 /session 显示的会话可能在下一次 Ask 时因指纹不符被作废；
// 那是如实反映库里的状态，不是错。
//
// 取闸用 TryLock：拿到就照旧读内存（没有再回库），读完 Unlock。
// 拿不到说明这条 (会话, 成员) 正有一轮在飞，不阻塞，改从库读上一次
// 提交的状态并置 InFlight = true。InFlight 只在 ok == true 那一支有意义。
// 从没开过口（库里没行）仍然返回 SessionInfo{}, false。
// BaselineSuspect 的来源随分支而定：拿到闸且内存里已有状态时读
// g.st.baselineSuspect；其余 ok == true 的两支（拿到闸但内存为空、拿不到闸）
// 都读库里那一行的 BaselineSuspect。
func (r *Runner) SessionInfo(ctx context.Context, convID, agentID string) (SessionInfo, bool) {
	g := r.gate(sessionKey{convID: convID, agentID: agentID})
	if g.mu.TryLock() {
		defer g.mu.Unlock()
		if g.st != nil {
			now := time.Now()
			if r.now != nil {
				now = r.now()
			}
			return SessionInfo{
				ProviderSession: g.st.providerSession,
				LastTurnAt:      g.st.lastTurnAt,
				ColdExpected:    coldExpectedAt(now, g.st.lastTurnAt, false),
				InFlight:        false,
				BaselineSuspect: g.st.baselineSuspect,
			}, true
		}
		if r.svc == nil {
			return SessionInfo{}, false
		}
		loaded, found, err := r.svc.ProviderSession(ctx, convID, agentID)
		if err != nil || !found {
			// 读不出来就照实说「没有」。这里不把 err 往上抛：/session 是
			// 一条给人看的命令，一行读失败不该让整张表打不出来。
			return SessionInfo{}, false
		}
		now := time.Now()
		if r.now != nil {
			now = r.now()
		}
		return SessionInfo{
			ProviderSession: loaded.ProviderSessionID,
			LastTurnAt:      loaded.LastTurnAt,
			ColdExpected:    coldExpectedAt(now, loaded.LastTurnAt, false),
			InFlight:        false,
			BaselineSuspect: loaded.BaselineSuspect,
		}, true
	}

	if r.svc == nil {
		return SessionInfo{}, false
	}
	loaded, found, err := r.svc.ProviderSession(ctx, convID, agentID)
	if err != nil || !found {
		return SessionInfo{}, false
	}
	now := time.Now()
	if r.now != nil {
		now = r.now()
	}
	return SessionInfo{
		ProviderSession: loaded.ProviderSessionID,
		LastTurnAt:      loaded.LastTurnAt,
		ColdExpected:    coldExpectedAt(now, loaded.LastTurnAt, false),
		InFlight:        true,
		BaselineSuspect: loaded.BaselineSuspect,
	}, true
}

// SessionInfo 是 /session 命令需要的只读快照。
type SessionInfo struct {
	ProviderSession string
	LastTurnAt      time.Time
	// ColdExpected 是查询这一刻的即时判断：距 LastTurnAt 是否已超过 CacheTTL。
	// 与 Record.ColdExpected 同一个判据，但这里是随时可查，不依赖真的起一轮调用。
	ColdExpected bool
	// InFlight 表示这条 (会话, 成员) 此刻正有一轮在飞：闸拿不到，
	// 下面的状态是从库里读回的**上一次提交**的值，不是此刻内存里的值。
	// 只在返回值 ok == true 那一支有意义。
	InFlight bool
	// BaselineSuspect 是下一轮进场时会看到的基线状态。true 表示下一轮的
	// 金额差值一律 nil（writeAskMeter 那句「金额未计」）。与
	// Record.BaselineSuspect（进轮快照）、sessionState.baselineSuspect
	// （活状态）、ProviderSession.BaselineSuspect（落库的那份）四个同名，
	// 时态不同。它不看 inflight_turns，那件事由起进程之前的拒绝负责。
	BaselineSuspect bool
}

var (
	ErrNoDelta       = errors.New("nothing new to send to this agent")
	ErrAgentUnknown  = errors.New("agent is not in the config")
	ErrIdentityDrift = errors.New("identity or brain changed since this session was opened")
	ErrPrefixRewrite = errors.New("batch would rewrite history already delivered to this agent")
	// ErrTierNotOffered 改名自 ErrTierNotInStep6（11c 契约第 3.3 节，
	// docs/09a_STEP6_API_CONTRACT.md 留了订正记录）：旧名字里的「Step 6」
	// 已经不准——workspace 在 11c 第二片放行了，这个哨兵现在只挡 full
	// 与拼错的档。这是编排层的政策（06 第 2 节：FULL 档不给编排器自动
	// 使用），不是 provider 能力问题，所以留在 TranslatePermission 这一层，
	// 与 provider.validateRequest 里那道写入边界闸并存——两道闸拦的不是
	// 同一件事：这一道问「编排器要不要开放这个档」，那一道问「这家的
	// provider 能不能证明自己围得住」。
	ErrTierNotOffered = errors.New("this permission tier is not offered to the orchestrator")
	ErrShimNewlineArg = errors.New("cmd/bat shim argument contains a newline")
	// ErrTurnInFlight：这条 (会话, 成员) 正有一轮在飞，Rebuild 拒绝。
	// 理由见 Rebuild 的注释：等到了也白做，SetProviderSession 会把刚
	// 删的行 upsert 回来。拒绝是 fail-closed 的那一边。
	ErrTurnInFlight = errors.New("a turn is in flight for this conversation and agent")
	// ErrTeamUnknown：会话绑了一个配置里查不到的 team。拒绝这一轮，
	// 起进程之前，一分钱不花。否决「查不到就按 false」：那会让「我明明
	// 打开了裁判的可见性」与「我什么都没配」在屏幕上长得一模一样，
	// 是本项目定义的那种最危险的静默降级。
	ErrTeamUnknown = errors.New("team is not in the config")
)

// thoughtVisible 判定这个 Mode 能不能在 Render 正文里看到 Thought。
// 判别器只有 Mode。未决 #7 已裁：Role 进身份块、进指纹，拿它分支
// 等于让「把某人改成裁判」变成一次 ErrIdentityDrift 加一次入场费。
//
// 空切片与 nil 都返回 false，不要把它们区分开。
func thoughtVisible(team config.TeamConfig, to config.Mode) bool {
	for _, m := range team.ThoughtVisibleTo {
		if m == to {
			return true
		}
	}
	return false
}

// thoughtVisibleFor 按会话绑定的 team 算出这一轮 NewRouter 要的 bool。
//
// teamID 为空：没绑 team = 谁都看不到，返回 false，不是错误。
// teamID 非空但 cfg.Teams 里查不到：返回 ErrTeamUnknown，调用方必须
// refuse 这一轮、起进程之前。否决「查不到就按 false」。
//
// cfg 是调用方持有的那一份轮内快照，本函数不再读 r.cfg。同一轮里
// Teams 必须与 Agents / Providers 来自同一份指针，否则 thought 可见性
// 与身份块会对不上。
func (r *Runner) thoughtVisibleFor(teamID string, to config.Mode, cfg *config.Config) (bool, error) {
	if teamID == "" {
		return false, nil
	}
	if cfg == nil || cfg.Teams == nil {
		return false, fmt.Errorf("team %q: %w", teamID, ErrTeamUnknown)
	}
	team, ok := cfg.Teams[teamID]
	if !ok {
		return false, fmt.Errorf("team %q: %w", teamID, ErrTeamUnknown)
	}
	return thoughtVisible(team, to), nil
}

func (r *Runner) refuse(ctx context.Context, rec *Record, st *sessionState, began bool, err error) (Record, error) {
	rec.Outcome = OutcomeRefused
	rec.Err = err
	r.persistTurn(ctx, rec, st, began)
	return *rec, rec.Err
}

func (r *Runner) failClosed(ctx context.Context, rec *Record, st *sessionState, began bool, fresh bool, outcome Outcome, maybeAdvanced bool, err error) (Record, error) {
	rec.Outcome = outcome
	rec.MaybeAdvanced = maybeAdvanced
	rec.Err = err
	rec.Message = nil
	if !fresh {
		// 进程跑过、结局未知：基线可能已经落后。fresh 时还没有会话记录可标，
		// 下一轮仍是 Fresh，不需要、也不该为此插一条空会话。
		st.baselineSuspect = true
	}
	extra := map[string]any{}
	r.attachSessionMeta(extra, *rec)
	// 超时那一支调用方的 ctx 已经死了。用它发面包屑会连这条 turn_failed
	// 一起丢掉，而 09 G 节要求失败必须在只追加的事实源里留下痕迹。
	// context.Background() 与调用方取消解耦，只为写下这条记录。
	sys, sysErr := r.postSystem(context.Background(), rec.ConvID, rec.AgentID, eventTurnFailed, turnFailedText(rec.AgentID, outcome), extra)
	if sysErr == nil {
		rec.System = sys
	}
	// 失败轮也要记账，且不再依赖 turn_failed 那条 system 消息当锚点。
	// 超时路径的 ctx 已死，落库同样走 Background。
	_ = ctx
	r.persistTurn(context.Background(), rec, st, began)
	return *rec, rec.Err
}

// commitSession 把本轮成功解析到的会话状态写进这条闸的 st。
// 调用方必须已经持有 g.mu：AskUpTo 整轮 defer Unlock，本函数只从那里
// 进来，所以写 g.st 时闸是拿着的。Fresh 路径上 st 此前只活在局部变量
// 里，这里才第一次挂到闸上；in-memory 路径上 st 本来就是 g.st，再赋一次
// 同一指针。
func (r *Runner) commitSession(g *sessionGate, st *sessionState, rec *Record, fp string, res provider.TurnResult) {
	st.providerSession = res.ProviderSession
	st.cursor = rec.ScannedTo
	st.lastTurnAt = r.now()
	st.identityFP = fp
	st.spoken = true
	st.baselineSuspect = false
	// drift 的语义是「CLI 悄悄开了新会话，前缀断了，对面的累计计数器从零开始」。
	// 必须先丢掉旧基线，再无条件用本轮累计值当新基线。不要先写 0 并把
	// haveCostTotal 置 true、再被本轮累计覆盖：那是两条互相踩的赋值，
	// CostUSDTotal 为 nil 时会留下「我们知道从零开始」的假基线，其实我们
	// 什么都不知道。本函数只改内存快照；库由调用方经 RecordTurn 写，
	// 不在这里碰 cursors 表（游标已经在 provider_sessions.cursor_seq）。
	if rec.Drifted {
		st.costTotal = 0
		st.haveCostTotal = false
	}
	if res.Usage.CostUSDTotal != nil {
		st.costTotal = *res.Usage.CostUSDTotal
		st.haveCostTotal = true
	}
	rec.LastTurnAt = st.lastTurnAt
	rec.ProviderSession = st.providerSession
	g.st = st
}

// persistTurn 把这一轮的用量与当前会话状态写进库。会话还在就传当前会话；
// st 为 nil 或 st.providerSession 为空串时传 nil。
//
// 未决 #20 之后 RecordTurn 的 session=nil 读作「这一轮不碰 provider_sessions
// 这一行」，不再是「删行」：真正的作废（指纹不符重建那一支）已经在 Ask 里
// 读回的那一刻显式调 Service.DropProviderSession 完成了，这里传 nil 只是
// 如实反映「这一轮没有一条要写的会话状态」，不会把一条早于本轮就存在、
// 本轮根本没碰过的行误删——起进程之前就拒绝的那几支（st 从未被赋值）
// 正是这条要保护的场景。
func (r *Runner) persistTurn(ctx context.Context, rec *Record, st *sessionState, began bool) {
	if rec.TurnID == "" {
		return
	}
	var seq *int64
	if rec.Message != nil {
		s := rec.Message.Seq
		seq = &s
	}
	u := makeTurnUsage(rec, seq, r.now())
	if u.ProviderSessionID == "" && st != nil {
		u.ProviderSessionID = st.providerSession
	}
	session := snapshotProviderSession(rec.ConvID, rec.AgentID, st)
	// expectInflight 回答的是「这一轮应当有一条在飞记录等着被清掉」，
	// 只有 turn 侧知道答案（conversation 把 Outcome 存成裸 int 就是为了
	// 不认识它）。began 由 askUpToWith 在 BeginTurn 成功之后置位，原样传入；
	// 这里再猜一次就会和调用点分叉。
	if _, err := r.svc.RecordTurn(ctx, rec.Message, u, session, began); err != nil && rec.Err == nil {
		rec.Err = err
	}
}

// sessionStateFrom 把库里的一整条 ProviderSession 填进内存快照。
// 五样同生共死，不给单独字段的入口；spoken 也一并读回，决定面包屑措辞。
func sessionStateFrom(ps conversation.ProviderSession) *sessionState {
	return &sessionState{
		providerSession: ps.ProviderSessionID,
		cursor:          ps.CursorSeq,
		costTotal:       ps.CostTotalUSD,
		haveCostTotal:   ps.HaveCostTotal,
		// 不抄这一列，重启之后 st.baselineSuspect 回到 false，而库里那一列
		// 还是 1。computeCostDelta 会拿一个已知落后的基线去算差值并且不报告。
		// DDL、Tx.SetProviderSession、scanProviderSession 都已经带着这一列；
		// 前三处齐了而桥断在这里，那个 bug 一行没修。
		baselineSuspect: ps.BaselineSuspect,
		lastTurnAt:      ps.LastTurnAt,
		identityFP:      ps.IdentityFP,
		spoken:          ps.Spoken,
	}
}

// snapshotProviderSession 把内存快照收成 RecordTurn 要的整包。
// providerSession 为空串表示会话从未建立（或本轮按 Fresh 走），传 nil
// 让 RecordTurn 跳过 provider_sessions 这一行——不写，也不删（未决 #20）。
//
// 若这一行本来就不存在（真正的 bootstrap），跳过与旧语义的「删」结果相同；
// 若这一行其实存在、只是 st 还是初始的 nil（起进程之前就拒绝的那几支，
// 从未读过库），跳过才是对的——删掉就是 #20 记录的那个 bug：一次与会话
// 无关的拒绝把已经花过钱的会话静默抹掉。
func snapshotProviderSession(convID, agentID string, st *sessionState) *conversation.ProviderSession {
	if st == nil || st.providerSession == "" {
		return nil
	}
	return &conversation.ProviderSession{
		ConvID:            convID,
		AgentID:           agentID,
		ProviderSessionID: st.providerSession,
		CursorSeq:         st.cursor,
		CostTotalUSD:      st.costTotal,
		HaveCostTotal:     st.haveCostTotal,
		LastTurnAt:        st.lastTurnAt,
		IdentityFP:        st.identityFP,
		Spoken:            st.spoken,
		// 写侧不带这一列，failClosed 在内存里置的 baselineSuspect 过不了
		// RecordTurn。重启后 sessionStateFrom 读回来的仍是 false，与读侧
		// 漏抄是同一个 bug 的两个方向：一次超时之后基线可疑这件事没了、
		// 基线本身还在。DDL、Tx.SetProviderSession、scanProviderSession
		// 已经齐了，断在这两个桥接点上就整段白做。
		BaselineSuspect: st.baselineSuspect,
	}
}

// makeTurnUsage 从 Record 填 TurnUsage。七个可空用量按「这家报没报」
// 决定 nil 还是指针：Unreported 里列出的一律填 nil，其余填指针。
// CacheCreate 本来就是 *int，未列入 Unreported 时原样透传（含 nil）。
// 未解析过（UsageParsed 为 false）时四个 token 列与 CacheCreate 一律 nil：
// 没见过 usage JSON，「不知道」不能写成实测为零（未决 #22）。
//
// 屏幕上的对应物是 Record.ReportedUsage（/ask 的用量行据它印 ? 或数字）；两者对五个可空用量列
// 逐列一致，由 x38a_reported_test.go 拿本函数当判据扫全部组合钉住，改这里的规则要一起改那边。
func makeTurnUsage(rec *Record, seq *int64, createdAt time.Time) conversation.TurnUsage {
	u := conversation.TurnUsage{
		TurnID:            rec.TurnID,
		ConvID:            rec.ConvID,
		AgentID:           rec.AgentID,
		ProviderSessionID: rec.ProviderSession,
		Seq:               seq,
		Outcome:           int(rec.Outcome),
		CacheCreate:       rec.Usage.CacheCreate,
		CostUSD:           rec.CostUSDDelta,
		CostCumulativeUSD: rec.Usage.CostUSDTotal,
		ColdStart:         rec.Cold,
		WallMS:            rec.WallMS,
		PromptBytes:       rec.PromptBytes,
		CreatedAt:         createdAt,
		IdentityFP:        rec.IdentityFP,
		RoundID:           rec.RoundID,
	}
	if !rec.UsageParsed {
		// 门放在结构体字面量之后、reportedInt 之前：CostUSD / CostCumulativeUSD
		// 保持 Record 原样；四个 token 列绝不能进 reportedInt，否则 0 会变成 &0。
		u.CacheCreate = nil
		u.InputTokens = nil
		u.CacheRead = nil
		u.OutputTokens = nil
		u.ThinkingTokens = nil
	} else {
		if unreportedHas(rec.Usage.Unreported, "cache_create") {
			u.CacheCreate = nil
		}
		u.InputTokens = reportedInt(rec.Usage.Unreported, "input_tokens", rec.Usage.InputTokens)
		u.CacheRead = reportedInt(rec.Usage.Unreported, "cache_read", rec.Usage.CacheRead)
		u.OutputTokens = reportedInt(rec.Usage.Unreported, "output_tokens", rec.Usage.OutputTokens)
		u.ThinkingTokens = reportedInt(rec.Usage.Unreported, "thinking_tokens", rec.Usage.ThinkingTokens)
	}
	if n := len(rec.Delivered); n > 0 {
		lo := rec.Delivered[0]
		hi := rec.Delivered[n-1]
		u.DeliveredLo = &lo
		u.DeliveredHi = &hi
	}
	return u
}

func reportedInt(unreported []string, name string, v int) *int {
	if unreportedHas(unreported, name) {
		return nil
	}
	x := v
	return &x
}

func (r *Runner) absorbParse(rec *Record, res provider.TurnResult) {
	// 本函数被调用即「解析过」。UsageParsed 只在这里置位，其它路径保持零值 false。
	rec.UsageParsed = true
	rec.Usage = copyUsage(res.Usage)
	if res.ProviderSession != "" {
		rec.ProviderSession = res.ProviderSession
	}
	// nil 输入 append 零个元素仍是 nil（Go 的 append 语义），
	// 「这家没报」与「报了但是空」的区分保持原样，不必另写一个判空分支。
	rec.DeniedActions = append([]provider.DeniedAction(nil), res.DeniedActions...)
}

func (r *Runner) readAfter(ctx context.Context, convID string, afterSeq int64) ([]conversation.Message, error) {
	var all []conversation.Message
	cur := afterSeq
	for {
		ms, err := r.svc.History(ctx, convID, cur, historyPage)
		if err != nil {
			return nil, err
		}
		if len(ms) == 0 {
			break
		}
		all = append(all, ms...)
		next := ms[len(ms)-1].Seq
		if next <= cur {
			// fail-closed：seq 没有前进，再读只会原地打转。不拦的话 History
			// 一页 512 条会变成死循环，这条 (会话, 成员) 的闸一直拿着，
			// 下一轮同一把 key 的 Ask 进不来。SessionInfo 用 TryLock，
			// 不会跟着挂死，但会报 InFlight。
			return nil, fmt.Errorf("conversation history seq did not advance (last %d)", next)
		}
		cur = next
		if len(ms) < historyPage {
			break
		}
	}
	if all == nil {
		all = []conversation.Message{}
	}
	return all, nil
}

func (r *Runner) postSystem(ctx context.Context, convID, agentID, event, text string, extra map[string]any) (*conversation.Message, error) {
	meta := map[string]any{
		MetaEvent: event,
	}
	// 空 agentID 表示这件事不关于某一个成员（round_started /
	// round_finished）。写下 about_agent="" 读回的人会以为缺了 id。
	// team_bound 的先例是根本不写这个键，这里对齐。
	if agentID != "" {
		meta[MetaAboutAgent] = agentID
	}
	for k, v := range extra {
		meta[k] = v
	}
	stored, err := r.svc.Post(ctx, conversation.Message{
		ConvID:   convID,
		SenderID: conversation.ReservedSenderSystem,
		Kind:     conversation.KindSystem,
		Text:     text,
		Meta:     meta,
	})
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

// postRebuildNotice 把「库里那条 provider 会话因为指纹不符被作废重建」这件事
// 说响亮（未决 #18 第 4 点）：旧指纹只有这一刻还在手上，过了这一刻它就被
// 调用方的 st = &sessionState{} 覆盖没了，identityDriftFields 现成可用，
// 能点名到底哪一项变了；不响亮地说出来的话，这条路径与真冷启动之后
// 没有任何一层能分辨「这轮为什么这么贵」。
//
// 走的是与 session_opening / turn_failed / post_rejected 相同的机制——
// 追加一条 KindSystem 面包屑：本包不持有任何输出流，「响亮」在这个项目里
// 从来就是指写进只追加的事实源，不是直接往屏幕打字（repl 包才认输出流）。
func (r *Runner) postRebuildNotice(ctx context.Context, convID, agentID, oldFP, newFP string) error {
	_, err := r.postSystem(ctx, convID, agentID, eventSessionRebuilt, rebuildNoticeText(agentID, oldFP, newFP), nil)
	return err
}

// rebuildNoticeText 拼作废重建的面包屑正文。能点名具体哪一项变了就点名
// （复用 identityDriftFields，与 identityDriftError、AuditDrift 同一份逐项
// 措辞，不另写一份走失同步）；旧指纹解不开（库里存的是升级前的六段格式，
// 或任何坏编码）就说「指纹格式已升级」，不去猜四个碰巧能用的空字段。
func rebuildNoticeText(agentID, oldFP, newFP string) string {
	reason := "指纹格式已升级"
	if parts, ok := identityDriftFields(oldFP, newFP); ok {
		if len(parts) > 0 {
			reason = strings.Join(parts, "; ")
		} else {
			// 两个指纹字符串不相等、但逐字段比较没有差异：packFingerprint 是
			// 确定性编码，理论上不该发生（drift.go 同一处防线的注释）。
			// fail-closed 地给一句能看的兜底，而不是打印一个空字符串。
			reason = "指纹不符（逐字段比较未发现差异，字符串本身不同）"
		}
	}
	return fmt.Sprintf("作废重建 %s 的 provider 会话：%s（下一轮按首轮计价）", agentID, reason)
}

func (r *Runner) agentMeta(caps provider.Capabilities, cmd provider.Command, rec Record, res provider.TurnResult, effort string) map[string]any {
	meta := map[string]any{
		MetaEffortPolicy: caps.Effort.String(),
		MetaEffortSource: effortSourceRequested,
		// 记的是这一轮命令行有没有真的带上 effort flag，不是 Capabilities
		// 上的策略。claude 虽是 EffortPerTurnFlag，但 effort 空串时 BuildTurn
		// 不会发 --effort；只看策略会把「没发」报成 false。
		MetaEffortFlagOmitted: effortFlagEmitted(caps.Effort, effort) == "",
		MetaModelSource:       modelSourceOf(caps, res),
		MetaCompletion:        completionMeta(res.Completion),
		MetaDelivered:         rec.Delivered,
		MetaPromptBytes:       rec.PromptBytes,
		// usage_raw 存 provider 的原始字节，不是归一化后的 Usage。
		// 归一化结果回算不出原值：三家的 InputTokens 由三条不同公式算出，
		// 公式一改，只剩结果就没法追溯。09 F 节与 05 第 3 节承诺的是原始对象。
		// UsageRaw 为 nil（usage 键缺席）时下面不写这个键，不写 null 占位。
		MetaCostBaselineSuspect: rec.BaselineSuspect,
	}
	if rec.ProviderSession != "" {
		meta[MetaProviderSession] = rec.ProviderSession
	}
	if len(res.UsageRaw) > 0 {
		meta[MetaUsageRaw] = res.UsageRaw
	}
	if res.StopRaw != "" {
		meta[MetaStopRaw] = res.StopRaw
	}
	if len(res.DeniedActions) > 0 {
		// 写进只追加的事实源，不只是屏幕上说一句：重启之后 /log 也要能看到
		// 「这一轮有过被拒的工具调用」，不能只活在当次终端的滚动历史里。
		meta[MetaDeniedActions] = append([]provider.DeniedAction(nil), res.DeniedActions...)
	}
	if len(res.DeniedActionsRaw) > 0 {
		meta[MetaDeniedActionsRaw] = res.DeniedActionsRaw
	}
	if len(res.Usage.Unreported) > 0 {
		meta[MetaUsageUnreported] = append([]string(nil), res.Usage.Unreported...)
	}
	if res.Usage.CostUSDTotal != nil {
		meta[MetaCostCumulativeUSD] = *res.Usage.CostUSDTotal
	}
	if len(cmd.Dropped) > 0 {
		meta[MetaDropped] = append([]provider.DroppedOption(nil), cmd.Dropped...)
	}
	if caps.ReportsObservedModel != provider.SupportNo && res.ObservedModel != "" {
		meta[MetaObservedModel] = res.ObservedModel
	}
	return meta
}

func (r *Runner) attachSessionMeta(meta map[string]any, rec Record) {
	if rec.ProviderSession != "" {
		meta[MetaProviderSession] = rec.ProviderSession
	}
	if rec.Usage.CostUSDTotal != nil {
		meta[MetaCostCumulativeUSD] = *rec.Usage.CostUSDTotal
	}
	if rec.BaselineSuspect {
		meta[MetaCostBaselineSuspect] = true
	}
	if len(rec.DeniedActions) > 0 {
		// turn_failed / post_rejected 的面包屑同样要带上：这两条路径都可能
		// 是先解析成功、后面才在别的判据上失败（absorbParse 先于
		// failClosed/post-rejected 分支运行），denied_actions 这条事实
		// 不该只挂在 OutcomeSpoke 一条路径上。
		meta[MetaDeniedActions] = append([]provider.DeniedAction(nil), rec.DeniedActions...)
	}
}

func modelSourceOf(caps provider.Capabilities, res provider.TurnResult) string {
	// 0 个键与多个键是两件不同的事。
	// ModelKeys == 0 是「这家根本不报实测型号」（agy 没有 modelUsage；claude
	// 失败时 modelUsage 是 {}），不是有歧义，应当走 requested。
	// ModelKeys > 1 才是 ambiguous：claude 的 modelUsage 是累计的，一旦某轮
	// 用过第二个型号，此后每轮都是多键，宁可标不确定，也不挑一个键假装知道。
	if caps.ReportsObservedModel == provider.SupportNo {
		return modelSourceRequested
	}
	if res.ModelKeys > 1 {
		return modelSourceAmbiguous
	}
	if res.ObservedModel != "" {
		return modelSourceObserved
	}
	return modelSourceRequested
}

func completionMeta(c provider.Completion) string {
	switch c {
	case provider.CompletionComplete:
		return "complete"
	case provider.CompletionTruncated:
		return "truncated"
	case provider.CompletionEmpty:
		return "empty"
	default:
		return "unknown"
	}
}

func effortFlagEmitted(policy provider.EffortPolicy, effort string) string {
	// 指纹和 MetaEffortFlagOmitted 都看「实际放上命令行的那个值」，
	// 不是 AgentConfig.Effort。EffortProcessExternalConfig 这一位恒为空串，
	// 用户改一个反正会被静默丢弃的值，不会触发一次 20k 入场费的空重建。
	if policy == provider.EffortPerTurnFlag {
		return effort
	}
	return ""
}

func computeCostDelta(st *sessionState, drifted bool, u provider.Usage) *float64 {
	if u.CostBasis == provider.CostBasisNone || u.CostUSDTotal == nil {
		// 不知道之一：这家根本不报金额（grok / agy）。nil 不是 0，
		// 全局成本视图是「claude 侧有钱、其余侧只有 token」的混合体，不折算。
		return nil
	}
	if st.baselineSuspect {
		// 不知道之二：上一轮超时或解析不了，那一轮可能真的计了费而我们没记上。
		// 猜一个差值会让成本视图从此少一笔或多一笔，而且只追加、纠正不了。
		return nil
	}
	if u.CostBasis == provider.CostBasisPerTurn {
		d := *u.CostUSDTotal
		return &d
	}
	total := *u.CostUSDTotal
	baseline := 0.0
	if !drifted && st.haveCostTotal {
		baseline = st.costTotal
	}
	if total < baseline {
		// 不知道之三：本轮累计值小于基线，provider 侧把会话重置了而我们没察觉。
		// 永不写负数，也永不 max(0, delta)：那会把这次重置抹成一笔零成本，
		// 成本视图从此撒谎。
		return nil
	}
	d := total - baseline
	return &d
}

func copyUsage(u provider.Usage) provider.Usage {
	if u.Unreported != nil {
		u.Unreported = append([]string(nil), u.Unreported...)
	}
	if u.CacheCreate != nil {
		v := *u.CacheCreate
		u.CacheCreate = &v
	}
	if u.CostUSDTotal != nil {
		v := *u.CostUSDTotal
		u.CostUSDTotal = &v
	}
	return u
}

func deliveredSeqs(ms []conversation.Message) []int64 {
	out := make([]int64, 0, len(ms))
	seen := make(map[int64]struct{}, len(ms))
	for _, m := range ms {
		if _, ok := seen[m.Seq]; ok {
			continue
		}
		seen[m.Seq] = struct{}{}
		out = append(out, m.Seq)
	}
	return out
}

func unreportedHas(list []string, name string) bool {
	for _, s := range list {
		if s == name {
			return true
		}
	}
	return false
}

// identityDriftFields 逐项比较两个指纹，返回点名哪些字段变了的短语列表
// （例如 `role "a" -> "b"`）。ok 为 false 表示至少一个指纹解不开 ——
// 调用方（identityDriftError 与 AuditDrift）都必须退回各自的「整体不符」
// 说法，不许把解不开的指纹拆成四个碰巧能用的空串，那会让一次身份漂移
// 被显示成「什么都没变」。
//
// identityDriftError 与 AuditDrift 共用这一个函数：前者要点名单条会话
// 在起进程之前为什么被拒，后者要给一整表会话逐行点名，两处的逐项措辞
// 必须是同一份，不能各写一份走失同步。
//
// 遍历一张 (标签, 旧值, 新值) 的表，不再逐字段写 if（契约第 3.3 节）：
// 加第八段是往表里加一行，不是在三个地方（这里、identityFPFor、
// parseFingerprint）各改一处、还要记得同步措辞。
// 命令行段的标签是「命令行」——用户认得的是"我发出去的那条命令行变了"，
// 不是"PrefixArgsDigest 变了"这种实现细节。
func identityDriftFields(oldFP, newFP string) (parts []string, ok bool) {
	oldParts, oldOK := parseFingerprint(oldFP)
	newParts, newOK := parseFingerprint(newFP)
	if !oldOK || !newOK {
		return nil, false
	}
	// 措辞按用户认得的东西说：他改的是一个 md 文件，不是一个叫 RulesGlobal
	// 的字段；两段规则存的是摘要（字节数 / 哈希前 12 位），命令行段同理，
	// 所以这两类打出来的都是短的，不是原文。
	fields := []struct {
		label string
		old   string
		new   string
	}{
		{"role", oldParts.Role, newParts.Role},
		{"model", oldParts.Model, newParts.Model},
		{"effort", oldParts.Effort, newParts.Effort},
		{"permission", oldParts.Tier, newParts.Tier},
		{"全局规则", oldParts.RulesGlobal, newParts.RulesGlobal},
		{"局部规则", oldParts.RulesLocal, newParts.RulesLocal},
		{"命令行", oldParts.PrefixArgs, newParts.PrefixArgs},
	}
	for _, f := range fields {
		if f.old != f.new {
			parts = append(parts, fmt.Sprintf("%s %q -> %q", f.label, f.old, f.new))
		}
	}
	return parts, true
}

func identityDriftError(oldFP, newFP string) error {
	parts, ok := identityDriftFields(oldFP, newFP)
	if !ok || len(parts) == 0 {
		return fmt.Errorf("fingerprint mismatch: %w", ErrIdentityDrift)
	}
	return fmt.Errorf("%s: %w", strings.Join(parts, "; "), ErrIdentityDrift)
}

func diagnose(agentID string, cmd provider.Command, exitCode int, stderr, stdout []byte, cause error, timeout bool) error {
	if cause == nil {
		cause = errors.New("provider turn failed")
	}
	// stderr 不是可选项：claude --resume 一个不存在的 id 时 stdout 是 0 字节，
	// 全部线索在 stderr 的纯文本里。stdout 那段经常什么都没有。
	msg := fmt.Sprintf("provider %s path %s exit %d stderr %q stdout %q",
		agentID, cmd.Path, exitCode, headBytes(stderr, diagStderrBytes), headBytes(stdout, diagStdoutBytes))
	if cmd.PlannedSessionID != "" {
		msg += fmt.Sprintf(" planned_session %s", cmd.PlannedSessionID)
	}
	if timeout {
		msg += " provider side may have ingested this batch"
	}
	return fmt.Errorf("%s: %w", msg, cause)
}

func headBytes(b []byte, n int) string {
	if n > len(b) {
		n = len(b)
	}
	return string(b[:n])
}

func openingText(agentID string, spoken bool) string {
	// 这条面包屑在起进程之前发（契约，时机不能改），所以措辞必须是尚未完成。
	// spawn 随后失败时，只追加的事实源里下一条就是 turn_failed；写成「已开启」
	// 会留下一条与世界不符、又改不掉的记录。
	if spoken {
		return fmt.Sprintf("正在重新开启 agent %s 的 provider 会话", agentID)
	}
	return fmt.Sprintf("正在为 agent %s 开启一条新的 provider 会话", agentID)
}

func turnFailedText(agentID string, outcome Outcome) string {
	return fmt.Sprintf("agent %s 本轮调用失败（outcome %d）", agentID, int(outcome))
}

func postRejectedText(agentID string) string {
	return fmt.Sprintf("agent %s 的本轮回答未写入对话（provider 已吃下本批内容）", agentID)
}
