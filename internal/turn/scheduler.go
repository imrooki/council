package turn

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// RunRound 让 plan.Speakers 在 convID 里并发说一轮话。
//
// 进来取一次 MaxSeq 与一次规则/配置快照，再交给 RunRoundPrepared
// 铸一个 roundID 发给全部成员。五个 goroutine 共用这三样。N 个
// 成员各铸各的 roundID，按轮聚合会拿到 N 个不同的 id；各取各的
// 快照，同一轮里两个成员的指纹会对不上，而漂移只会在下一轮才被发现。
//
// 顺序与拆出 RunRoundPrepared 之前相反：那时是先铸 id，再取快照。
// 现在 snap 是 RunRoundPrepared 的参数，本函数只能先 TakeRoundSnap。
// 两者同时失败时，胜出的错误从「mint round id」变成 MaxSeq 那条；
// 铸 id 失败的路径也会先做一次 MaxSeq 的库读。这是有意的。本地
// SQLite 读是亚毫秒级的，一次白读可以忽略；而 RunRoundPrepared
// 要能被 /round 单独拿着已经预报过的那份 snap 调用，它就不能
// 自己再拍一次。
//
// maxParallel <= 0 表示不限；> 0 时用缓冲 channel 当信号量。
// 它只是本函数的参数。契约 4.3：不加 TeamConfig 键，REPL 恒传 0。
// 给配置加一个谁也不读的键，正是本项目反对的死键。
//
// 一个成员失败不取消别人。失败是既有设计里的正常业务结局，
// 一人超时把另外四人已经花过的钱作废才是真正的损失。
// 不用 errgroup 那种「第一个错就 cancel」的形状。父 ctx 取消
// （Ctrl-C）仍传给每一个 Ask，那是调用方要停整轮，与「一人
// 业务失败」不是同一件事。
//
// 返回顺序跟 plan.Speakers（已是字典序），不按完成顺序。
// 按完成顺序返回会让调用方的输出随机。
//
// 本函数不做的事（不是漏了）：
//   - 不跑 plan.Judges。--with-judge 是片 3d；judge 要重新取一次
//     MaxSeq 当它的 upTo，它要看的是本轮结论，不能与 Speakers
//     共用这个快照。
//   - 不判 no_delta。Budget 在行上标 NoDelta；Skipped 由片 3d 的
//     /round 据它组，再把改过的 plan 交给本函数。直接收到的 plan
//     仍照常送进 askUpToWith，由既有 ErrNoDelta 路径 refuse。
//   - 不做 argv 超限的整轮拒绝（片 3c/3d）。
//   - 不印任何东西到屏幕。本包不持有输出流，屏幕归 repl。
//
// plan.Skipped 原样带过，本函数不读、不改。
func (r *Runner) RunRound(ctx context.Context, convID string, plan Plan, maxParallel int) []Record {
	snap, snapErr := r.TakeRoundSnap(ctx, convID)
	if snapErr != nil {
		// 预填已经搬进 RunRoundPrepared。这一支发生在调用它之前，
		// 只能在这里自己做出同样形状的切片：长度、顺序、默认结局
		// 都是本包的约定，不能把一个半填的 out 漏给调用方。
		out := make([]Record, len(plan.Speakers))
		for i, id := range plan.Speakers {
			out[i] = Record{ConvID: convID, AgentID: id, Outcome: OutcomeRefused, Err: snapErr}
		}
		return out
	}
	return r.RunRoundPrepared(ctx, convID, plan, maxParallel, snap)
}

// RoundProgress 是一个成员刚回来时的回调。nil 表示不要进度。
//
// 为什么这件事值得一个钩子：一轮 N 个成员并发起进程，每个的硬超时是
// defaultAskTimeout（5 分钟）。等全部回来才一次性打印，最坏情况就是
// 五分钟一个字都不动，用户分不清是在跑、卡死了、还是自己敲错了。
// 有了这个钩子，调用方可以谁回来先打谁。
//
// 触发它的纪律（串行、传副本、nil 合法）写在 RunRoundWithStream 里，
// 那是唯一触发它的地方。
type RoundProgress func(rec Record)

// RunRoundPrepared 用调用方已经取好的 snap 做完开轮前的准备，再交给
// RunRoundWith。片 3d 的 /round 必须「取一次快照，用它预报，再用同一份
// 执行」：预报与执行之间若再拍一次，SetRules 或 SetConfig --apply
// 会让报出去的价与真正送出去的前缀不是同一段。
//
// 不把 /round 直接接到 RunRoundWith。out 的长度、与 Speakers 对齐的
// 预填、默认结局 OutcomeRefused、出错时逐行塞 Err 再整体返回，这四条
// 是本包自己的知识。漏给 internal/repl 之后，两边任何一处改了，另一处
// 都不会跟着改。铸 roundID 也只留在这里，RunRound 不再自己铸，入口是
// 少了一处而不是多了一处。
//
// 本方法退化成 RunRoundPreparedStream 传 nil 的薄包装。签名一个字不改：
// 它与 RunRound / RunRoundWith 在 Step 9 片 3 已冻结，现有调用点与用例
// 都按它们写——加一个变体是纯追加，改签名是波及全仓。
func (r *Runner) RunRoundPrepared(ctx context.Context, convID string, plan Plan, maxParallel int, snap RoundSnap) []Record {
	return r.RunRoundPreparedStream(ctx, convID, plan, maxParallel, snap, nil)
}

// RunRoundPreparedStream 是 RunRoundPrepared 多收一个进度回调的变体。
// 除了把 onDone 透给 RunRoundWithStream，行为与 RunRoundPrepared 逐段相同。
func (r *Runner) RunRoundPreparedStream(ctx context.Context, convID string, plan Plan, maxParallel int, snap RoundSnap, onDone RoundProgress) []Record {
	n := len(plan.Speakers)
	out := make([]Record, n)
	for i, id := range plan.Speakers {
		out[i] = Record{ConvID: convID, AgentID: id, Outcome: OutcomeRefused}
	}

	roundID, mintErr := newV4UUID()
	if mintErr != nil {
		for i := range out {
			out[i].Err = fmt.Errorf("mint round id: %w", mintErr)
		}
		// 这一支不进并发段，回调一次都不会被触发。必须在这里补一遍：
		// 调用方已经把「逐条渲染」整个搬进了回调，不补的话这一轮用户
		// 只剩一行结算，N 个成员各自因为什么没跑成一个字都没有——而
		// 这恰好是最需要解释的一轮。补了之后「每个成员恰好一次」对
		// 全部返回路径都成立，不只对跑到并发段的那一条。
		notifyEach(out, onDone)
		return out
	}
	for i := range out {
		out[i].RoundID = roundID
	}
	return r.RunRoundWithStream(ctx, convID, plan, maxParallel, roundID, out, snap, onDone)
}

// notifyEach 把已经填好结局的 out 逐条交给回调，用在「还没进并发段就
// 整轮失败」的两条路径上（铸 id 失败、round_started 写不进事实源）。
//
// 不上锁：这两处都还在调用方自己的 goroutine 上，此刻一个 worker 都
// 没起来，串行是结构上的，不是靠锁换来的。
func notifyEach(out []Record, onDone RoundProgress) {
	if onDone == nil {
		return
	}
	for i := range out {
		onDone(out[i])
	}
}

// RunRoundWith 用调用方已经铸好的 roundID、预填好的 out、取好的 snap
// 跑完一轮。铸 id 与预填在 RunRoundPrepared，不在这里：铸 id 失败
// 不该为了复用本函数再进一轮 ask。
//
// 本函数不再自己 snapshotRulesConfig、不再自己 MaxSeq。面包屑、信号量、
// askUpToWith、round_finished 与抽出来之前逐段相同。
//
// 与 RunRoundPrepared 同理，本方法退化成 RunRoundWithStream 传 nil 的
// 薄包装，签名一个字不改。
func (r *Runner) RunRoundWith(ctx context.Context, convID string, plan Plan, maxParallel int, roundID string, out []Record, snap RoundSnap) []Record {
	return r.RunRoundWithStream(ctx, convID, plan, maxParallel, roundID, out, snap, nil)
}

// RunRoundWithStream 是 RunRoundWith 多收一个进度回调的变体，也是唯一
// 触发 onDone 的地方。onDone 为 nil 时一次都不调，nil 是正常值不是错误。
func (r *Runner) RunRoundWithStream(ctx context.Context, convID string, plan Plan, maxParallel int, roundID string, out []Record, snap RoundSnap, onDone RoundProgress) []Record {
	rules, cfg, upTo := snap.Rules, snap.Cfg, snap.UpTo

	// round_started 在 MaxSeq 之后发。upTo 是「决定开这一轮时库里
	// 已经有的最大 seq」；面包屑记录这个决定，它自己不是决定的一部分。
	// 先发再 MaxSeq，upTo 会含这条面包屑，除非再减 1——那种算术
	// 过一个版本就会错。
	//
	// 它不进任何人的 prompt：这是结构保证，不是碰巧。生产路径是
	// Router.Select，增量走 filterDelta、首轮走 selectBootstrap，
	// 两条都丢掉 KindSystem。seq > upTo，本轮截断扫不到它；下一轮
	// 扫到再丢掉，与 ErrNoDelta 那条「再扫一遍被过滤掉的 KindSystem
	// 是免费的」同一类。
	speakers := append([]string(nil), plan.Speakers...)
	if speakers == nil {
		speakers = []string{}
	}
	startText := fmt.Sprintf("一轮开始 round_id=%s upTo=%d speakers=%s", roundID, upTo, strings.Join(speakers, ", "))
	if _, startErr := r.postSystem(ctx, convID, "", eventRoundStarted, startText, map[string]any{
		MetaRoundID:  roundID,
		MetaUpTo:     upTo,
		MetaSpeakers: speakers,
	}); startErr != nil {
		// 面包屑都写不进去就不要去花钱。与 session_opening 那支同理。
		for i := range out {
			out[i].Err = fmt.Errorf("round_started: %w", startErr)
		}
		// 与铸 id 失败那一支同一条理由：这里也不进并发段，不补一遍
		// 用户就只看得到结算，看不到这一轮为什么一个人都没问。
		notifyEach(out, onDone)
		return out
	}

	var sem chan struct{}
	if maxParallel > 0 {
		sem = make(chan struct{}, maxParallel)
	}

	// 回调的串行化做在这一层，不推给调用方。调用方是 REPL，它往
	// os.Stdout 打字（internal/app 把 os.Stdout 原样交给 repl.New，
	// REPL.out 是 io.Writer，printf 直接 WriteString，中间没有 bufio）。
	// 而它渲染一条 Record 要连打好几次（结局、会话、正文、用量、金额、
	// 冷暖各一次），每次是一次独立的 Write。让 N 个 goroutine 直接写
	// 同一个 io.Writer，两个成员的行就会互相穿插：一条记录读到一半
	// 冒出另一个人的数字。而这种穿插只在「两家几乎同时回来」时出现，
	// 最难复现、最像偶发。锁在这里，调用方就不需要知道回调是并发触发
	// 的；锁在调用方，每个将来的调用方都要重新想一遍。
	//
	// 锁是本次调用局部的，不挂到 Runner 结构体上：挂上去会让两轮并发的
	// /round 互相阻塞对方的打印，而它们本来无关。今天的 REPL 是单线程
	// 读循环、同时只有一轮在跑，所以这句防的是任何将来的并发调用方，
	// 不是今天就会发生的事。
	var progressMu sync.Mutex

	var wg sync.WaitGroup
	for i, id := range plan.Speakers {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}
			rec, _ := r.askUpToWith(ctx, convID, id, upTo, roundID, rules, cfg)
			out[i] = rec
			if onDone == nil {
				return
			}
			// 传 rec 这个局部值，不传 &out[i]：out 还在被别的 goroutine
			// 写，把指向它的指针交出去就是一条数据竞争。
			// 代价：Record 是浅拷贝。Message、System、CostUSDDelta、
			// Usage.CacheCreate 是指针，Delivered、DeniedActions、
			// Usage.Unreported 是切片，拷贝之后仍指向同一份底层数据。
			// 回调改它们指向的东西，返回的 []Record 里那一条会跟着变。
			// 回调只许读。
			//
			// 另一项代价，写下来免得下一个人当成 bug：上面 sem 的释放是
			// defer 的，排在这个回调之后，所以一个慢回调会一直占着并发槽。
			// 今天的回调只是打印，可以忽略；但回调不许做 IO 等待之外的
			// 重活，更不许在里面再去调 Runner 的方法（会自我阻塞）。
			progressMu.Lock()
			onDone(rec)
			progressMu.Unlock()
		}()
	}
	wg.Wait()

	// 结算写在 round_finished 里，不留给片 3d：本函数手里有 []Record，
	// 算得出来。片 3d 只是把它印到屏幕上。分两次写同一条面包屑，
	// 第二次就要改第一次写下的行——事实源只追加不改写。
	//
	// 落库用 Background：成员可能已经把调用方 ctx 取消了（Ctrl-C），
	// 用死 ctx 会连结算一起丢掉。与 failClosed / 成功路径 RecordTurn
	// 同一条理由。本地 SQLite 写是亚毫秒级的。
	finExtra, finText := roundFinishedMeta(roundID, out)
	_, _ = r.postSystem(context.Background(), convID, "", eventRoundFinished, finText, finExtra)

	return out
}

// roundSpawned 报告这一条 Record 是否确知起过进程。
// OutcomeRefused / OutcomeUnknown 是起进程之前就停的（含 no_delta、
// 身份漂移、argv 闸命中后被改判 refuse 的那支）。
// OutcomeSpawnFailed 不算起过。runner 里通向这个结果的一支把三件不同的事
// 收成同一档：不在 PATH、本地转发服务 睡着、立刻非零退出。三者里只有最后一种起过
// 进程，Record 上没有字段能拆开（没有 exitCode；这条路径上的 WallMS 也
// 不可靠）。分不清就不当「起过」，否则 spawned 会把不知道写成一个确定的数。
// 这一档由 roundFinishedMeta 单列成 spawn_failed。
// 其余 Outcome 仍走 default，算确知起过。
func roundSpawned(rec Record) bool {
	switch rec.Outcome {
	case OutcomeRefused, OutcomeUnknown, OutcomeSpawnFailed:
		return false
	default:
		return true
	}
}

// RoundSettlement 是一轮 []Record 的结算。
// Have* 为 false 表示「一个成员都没报过这一列」：调用方各自决定怎么表达
// （Meta 省略整键，屏幕印未知、不印 0），数值字段在这种情况下是零值，没有含义。
// ColdUnknown 是冷暖这一列没覆盖的人数：计入 Spawned、但冷暖不是实测值的成员，
// 含五个用量都没有数的。它与 UsageUnknown（token 合计那几列没覆盖的人数）可以重叠，
// UsageUnknown 数的人全在 ColdUnknown 里；与 CostUnreported 同一种写法。
// Cold 只数冷暖确知且为冷的成员。
type RoundSettlement struct {
	Spawned, SpawnFailed, UsageUnknown, CostUnreported, Cold, ColdUnknown int
	InputTokens, OutputTokens, CacheRead, CacheCreate, ThinkingTokens     int

	HaveIn, HaveOut, HaveCacheRead, HaveCacheCreate, HaveThinking, HaveCost bool
	CostUSD                                                                 float64
}

// Settle 从本轮 []Record 收结算数字。纯函数：不碰库、不发面包屑、不读 Runner。
//
// 片 3d 的 /round 要在屏幕上印同一份结算数字。
// 让 internal/repl 自己再实现一遍这套聚合的话，
// 认 Unreported、spawn_failed 分档、usage_unknown 计数这三件事
// 就同时住在两个包里 —— 任何一处改了、另一处都不会跟着改。
// 抽出来之后聚合只有一份实现，两个消费者
// （roundFinishedMeta 序列化进事实源，/round 渲染到屏幕）。
// 屏幕与事实源口径一致因此是结构性的，不是靠两边各自小心。
//
// Have* 那组布尔是把「一个成员都没报过这一列」这件事带给渲染方的通道：
// Meta 那边表达成「键整个不出现」，屏幕那边表达成「印未知、不印 0」，
// 两种表达同一件事，都从这一个结构体来。
//
// Spawned 只数 roundSpawned 为真的成员。OutcomeSpawnFailed 另计 SpawnFailed，
// 不混进 Spawned：这一档分不清进程到底有没有起来。
// CostUnreported 用同一道门，只数确知起过、但 CostUSDDelta 为 nil 的成员
// （grok / agy 不报金额，或基线可疑、累计回退）。没起来的，以及分不清有没有
// 起来的，都不是「起了进程却没报金额」。
// UsageUnknown 数的是已经算进 Spawned、但五个用量一个数都没有的成员：
// 判据是 rec.ReportedUsage() 的五个指针全为 nil，与 makeTurnUsage 写库五列全 NULL、
// /cost 的 USAGE_UNKNOWN 是同一个逐行谓词。没解析到用量（UsageParsed 为 false）、
// usage 对象缺失的成功轮、grok 的错误信封轮都在这一档。
// 人群仍是 Spawned 这道门，不照 /cost 的「非 Refused」：SpawnFailed 另计，不进这里。
// 用量合计是「报过这一列的那些行」的和，不是全体成员的和；不把跳过的行数
// 留下，部分和会被读成全体。
// 五个用量各自认 Unreported。成员把某指标标成未报时，这一行不加进该指标，
// 值是 0 也不加：那个 0 是没填，不是实测为零。grok 的 cache_create（非 nil
// 的 0，同时标了未报）是这条路；五个指标全未报的成员在上面用量未知那一支就已跳过。
// 某个指标一个报的都没有，对应的 Have* 为 false，数值留在零值上，不拿它冒充合计是零。
// 这与 CostUSD 只在至少一名成员报了金额时才算报过（HaveCost）是同一条纪律。
// cache_create 两道都要过：标了未报的跳过；指针是 nil 的也跳过，nil 同样没有数。
// 冷暖这一列的判据只来自 rec.MeasuredCold()：每个计入 Spawned 的成员判一次，
// 冷暖确知且为冷的进 Cold，冷暖不确知的进 ColdUnknown（含五个用量都没有数的），
// 确知为暖的两个都不加，所以 ColdUnknown 加冷暖确知的人数恒等于 Spawned。
// 这一判定放在用量未知那一支之前、且不跳过后面的累加：冷暖未知的成员
// 报了的用量照样进五个指标的合计。
func Settle(recs []Record) RoundSettlement {
	var s RoundSettlement
	for _, rec := range recs {
		if rec.Outcome == OutcomeSpawnFailed {
			s.SpawnFailed++
			continue
		}
		if !roundSpawned(rec) {
			continue
		}
		s.Spawned++
		if rec.CostUSDDelta == nil {
			s.CostUnreported++
		} else {
			s.CostUSD += *rec.CostUSDDelta
			s.HaveCost = true
		}
		if cold, known := rec.MeasuredCold(); !known {
			s.ColdUnknown++
		} else if cold {
			s.Cold++
		}
		if ru := rec.ReportedUsage(); ru.Input == nil && ru.CacheCreate == nil && ru.CacheRead == nil && ru.Output == nil && ru.Thinking == nil {
			s.UsageUnknown++
			continue
		}
		if !unreportedHas(rec.Usage.Unreported, "input_tokens") {
			s.InputTokens += rec.Usage.InputTokens
			s.HaveIn = true
		}
		if !unreportedHas(rec.Usage.Unreported, "output_tokens") {
			s.OutputTokens += rec.Usage.OutputTokens
			s.HaveOut = true
		}
		if !unreportedHas(rec.Usage.Unreported, "cache_read") {
			s.CacheRead += rec.Usage.CacheRead
			s.HaveCacheRead = true
		}
		if !unreportedHas(rec.Usage.Unreported, "thinking_tokens") {
			s.ThinkingTokens += rec.Usage.ThinkingTokens
			s.HaveThinking = true
		}
		if !unreportedHas(rec.Usage.Unreported, "cache_create") && rec.Usage.CacheCreate != nil {
			s.CacheCreate += *rec.Usage.CacheCreate
			s.HaveCacheCreate = true
		}
	}
	return s
}

// roundFinishedMeta 把 Settle 的结果序列化成事实源的一条面包屑。
// 无条件落的键：round_id、spawned、spawn_failed、usage_unknown、cost_unreported、cold、cold_unknown。
// input_tokens、output_tokens、cache_read、cache_create、thinking_tokens、cost_usd
// 只在对应 Have* 为真时落键；为假时整个键省略。
// 文本里这些省略的指标印成 ?，不印 0：印成 0 就又把不知道写成了合计为零。
// cost_usd 用 %g，其余用量用十进制整数。
func roundFinishedMeta(roundID string, recs []Record) (map[string]any, string) {
	s := Settle(recs)
	extra := map[string]any{
		MetaRoundID:       roundID,
		"spawned":         s.Spawned,
		"spawn_failed":    s.SpawnFailed,
		"usage_unknown":   s.UsageUnknown,
		"cost_unreported": s.CostUnreported,
		"cold":            s.Cold,
		"cold_unknown":    s.ColdUnknown,
	}
	if s.HaveIn {
		extra["input_tokens"] = s.InputTokens
	}
	if s.HaveOut {
		extra["output_tokens"] = s.OutputTokens
	}
	if s.HaveCacheRead {
		extra["cache_read"] = s.CacheRead
	}
	if s.HaveCacheCreate {
		extra["cache_create"] = s.CacheCreate
	}
	if s.HaveThinking {
		extra["thinking_tokens"] = s.ThinkingTokens
	}
	// 省略的指标在文本里必须是 ?。印成 0 就又把不知道写成了合计为零。
	shown := func(have bool, n int) string {
		if !have {
			return "?"
		}
		return fmt.Sprintf("%d", n)
	}
	costText := "?"
	if s.HaveCost {
		extra["cost_usd"] = s.CostUSD
		costText = fmt.Sprintf("%g", s.CostUSD)
	}
	text := fmt.Sprintf("一轮结束 round_id=%s spawned=%d spawn_failed=%d usage_unknown=%d in=%s out=%s cache_read=%s cache_create=%s thinking=%s cost_usd=%s cost_unreported=%d cold=%d cold_unknown=%d",
		roundID, s.Spawned, s.SpawnFailed, s.UsageUnknown,
		shown(s.HaveIn, s.InputTokens), shown(s.HaveOut, s.OutputTokens), shown(s.HaveCacheRead, s.CacheRead),
		shown(s.HaveCacheCreate, s.CacheCreate), shown(s.HaveThinking, s.ThinkingTokens),
		costText, s.CostUnreported, s.Cold, s.ColdUnknown)
	return extra, text
}
