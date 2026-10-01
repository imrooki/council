package repl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// cmdRound 实现 /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]。
//
// 这段理由被改写过，每次留下的是当时为真的那条：
//
// 第一次（原文，11d §3.1 / 8c-2 冻结）：扇出留在 REPL 层，不进
// internal/turn：Runner.Ask 的「一次一轮」语义一个字不改，否则记账、
// 指纹、超时、锁都要在第二条路径上复制一份。
//
// 第二次（核注释漂移时，片 3b/3c 已落地之后）：internal/turn 现在确实
// 有了自己的并发扇出——片 3b 的 RunRound、片 3c 从中拆出的
// RunRoundWith（scheduler.go）——但当初担心的「第二条路径复制一份」
// 没有发生：它复用的还是 askUpToWith，记账、指纹、超时、锁都只有这一份
// 实现。真正没变的是本函数 cmdRound：它依旧是那个串行 for 循环，
// 一次一个 Ask，扇出依旧留在 REPL 层，没有调用 RunRound。
//
// 第三次（片 3d，cmdRound 切到 internal/turn 之后）：扇出现在真的走
// internal/turn。本函数只取一次 snap，BudgetWith 与 RunRoundPrepared
// 共用它，不再自己 for 循环调 Ask。记账、指纹、超时、锁仍只有
// askUpToWith 那一份，没有在 REPL 复制第二条路径。代价是循环里的
// 插入点没了：旧的「已跑 k/N」是在每一轮之前打的，这里不再打。
// 下面 Ctrl-C 四条是 2026-09-21 对当时那个串行循环的实测，原句保留，
// 这次没有重测。
//
// 第四次（多轮与裁判轮接回来之后）：第三次里有三句后来不成立。
// 有循环了，发言轮重新是 cmdRound 里的命令级 for，不是「不再自己
// for 循环调 Ask」。每轮各取一次快照，第一轮复用分类那次；第 2 到第 N
// 轮和裁判轮各自再取。「已跑 k/N」回来了，打在还没开始的那一轮之前。
// 这个 k 数的是命令级轮次不是成员级。Ctrl-C 进 ctx 时正在飞的那一轮
// 已经付过钱，由 runOneRound 收完，k 不含被腰斩的那一轮。记账、指纹、
// 超时、锁仍只有 askUpToWith 那一份。下面 Ctrl-C 四条仍是 2026-09-21
// 对当时那个串行循环的实测，原句保留，这次没有重测。
//
// 第五次（逐条输出接上之后）：runOneRound 改调 RunRoundPreparedStream，
// 逐条渲染从「全部回来之后的循环」挪进回调，谁回来先打谁。扇出、快照、
// 记账、本函数的命令级 for 都不变，变的只是那一段输出发生在什么时候。
//
// 默认只打印计划，不起任何进程；加 --go 才真跑。--go、--rounds、
// --with-judge、--no-summary 四个 flag 都可以出现在 fields 任意位置，剔掉后剩下的
// 才是 id。名单为空则拒绝，绝不默认全员（默认全员
// = 一次敲击起 N 个进程、付 N 次可能的入场费）。
//
// 第六次（#45，2026-09-27，Griffin）：推翻了「不引入配置键、不引入自动链」。
// [summary] 是配置键，自动总结是自动链。上限是「每条 /round 恰好多一次调用」
// （最多一个总结段，只在最后），有界、不循环。触发判据见 summary.go。
// 总结成员不许出现在名单里。--no-summary 跳过。
//
// 第七次（#47，2026-09-28，Griffin）：对话模式推翻了上面「绝不默认全员」，但只对裸文本。
// 自动回复打开时，裸文本落库之后由 autoReplyRound（autoreply.go）定名单：没点名就是全部活跃成员
// （去掉总结成员），再经 roundCmd 进本函数，名单总是显式给出。本函数自己的规则一条不变：
// 手敲的 /round 名单为空照旧拒绝，不默认全员。
//
// Ctrl-C 的真实行为（2026-09-21 实测）：
//   - 第一次 Ctrl-C 就腰斩当前轮，不是「让本轮跑完再停」；
//   - 停下并打出「已跑 k/N」之后 council 整个进程退出，不是回到 REPL 提示符；
//   - 一轮 N 个成员在飞时按 Ctrl-C，N 个子进程会同时收到信号（Windows 把
//     Ctrl-C 送给整个控制台进程组，实测子进程退出码 3221225786 =
//     STATUS_CONTROL_C_EXIT）。Council 不需要逐个去杀；反过来 Council 也
//     无从选择「只停循环、让当前轮跑完」。
//
// 总用时在返回时由 defer 打出。四个判据同时成立才打：返回的 err 为 nil
// （写屏幕失败时不打、不吞错误）、spawned（本条命令至少一段进入了扇出：
// runOneRound 返回 true、RunRoundPreparedStream 已调用；成员可能全被拒或起不来）、
// 简洁档、没有装观察者。装了观察者就是 TUI，总用时由 TUI 收尾行出，这里不打，两种模式都恰好一行。
// 用时走 FormatWall。详细档不打，今天的行式输出逐字节不变。
//
// 总结段在这里：最后一段（裁判轮，或没有裁判轮时的最后一个发言轮）之后、
// 本函数返回之前。defer 在返回时才打总用时，所以屏幕顺序是各段成员块、总结、总用时。
// 总结不另打一行总用时。总结调用若起了进程，它在本函数里，时间计入上面这条总用时。
// TUI 的收尾行在 Submit 返回之后才由 Model 打，自然在总结之后。
func cmdRound(ctx context.Context, r *REPL, args cmdArgs) (err error) {
	start := r.clock()
	spawned := false
	defer func() {
		if err != nil || !spawned || !r.concise || r.roundObs != nil {
			return
		}
		err = r.printf("总用时 %s\n", FormatWall(int(r.clock().Sub(start)/time.Millisecond)))
	}()
	goRun := false
	withJudge := false
	noSummary := false
	rounds := 1
	roundsSet := false
	given := make([]string, 0, len(args.fields))
	for i := 0; i < len(args.fields); i++ {
		f := args.fields[i]
		switch f {
		case "--go":
			goRun = true
			continue
		case "--with-judge":
			withJudge = true
			continue
		case "--no-summary":
			noSummary = true
			continue
		case "--rounds":
			// 值必须是下一个 fields 元素。写成 --rounds=3 或把 --rounds
			// 放在行尾，都当解析失败拒绝：默认 1 会把没写清的次数跑出去。
			if roundsSet || i+1 >= len(args.fields) {
				return r.printf("拒绝: --rounds 需要一个不小于 1 的整数。\n")
			}
			n, err := fmt.Sscanf(args.fields[i+1], "%d", &rounds)
			if err != nil || n != 1 || fmt.Sprintf("%d", rounds) != args.fields[i+1] {
				return r.printf("拒绝: --rounds 需要一个不小于 1 的整数。\n")
			}
			roundsSet = true
			i++
			continue
		}
		given = append(given, f)
	}
	if rounds < 1 {
		return r.printf("拒绝: --rounds 需要一个不小于 1 的整数。\n")
	}
	// N>1 而不带 --go 会把同一份计划印 N 遍，用户无法从屏幕上判断哪一次
	// 是要确认的那轮。拒绝发生在取名单之前，计划一行都不印。
	if rounds > 1 && !goRun {
		return r.printf("拒绝: --rounds %d 只在 --go 下可用。\n", rounds)
	}

	parts, err := r.svc.Participants(ctx, r.cur.ID, false)
	if err != nil {
		return r.reportErr("round", err)
	}
	active := make(map[string]string, len(parts))
	for _, p := range parts {
		id := p.AgentID
		active[strings.ToLower(id)] = id
	}

	var ids []string
	if len(given) > 0 {
		var unknown []string
		ids = make([]string, 0, len(given))
		for _, raw := range given {
			canon, ok := active[strings.ToLower(raw)]
			if !ok {
				unknown = append(unknown, raw)
				continue
			}
			ids = append(ids, canon)
		}
		if len(unknown) > 0 {
			return r.printf("拒绝: 不是当前活跃成员: %s\n", strings.Join(unknown, ", "))
		}
	} else {
		mentions, mErr := lastUserMentionIDs(ctx, r)
		if mErr != nil {
			return r.reportErr("round", mErr)
		}
		ids = make([]string, 0, len(mentions))
		for _, m := range mentions {
			if canon, ok := active[m]; ok {
				ids = append(ids, canon)
			}
		}
	}
	if len(ids) == 0 {
		return r.printf("拒绝: 名单为空。给了 id 才用给的；没给则取当前会话最后一条用户发言里的 mention 与当前活跃成员的交集。不会默认全员。\n")
	}

	// 分类只做一次。轮与轮之间要看见的是新 rules / 新 cfg，那是
	// TakeRoundSnap 的事；成员名单在这一条命令里不变。
	snap0, err := r.rn.TakeRoundSnap(ctx, r.cur.ID)
	if err != nil {
		return r.reportErr("round", err)
	}
	var agents map[string]config.AgentConfig
	sumID := ""
	if snap0.Cfg != nil {
		agents = snap0.Cfg.Agents
		sumID = snap0.Cfg.Summary.Agent
	}
	if sumID != "" {
		for _, id := range ids {
			if id == sumID {
				return r.printf("拒绝: %s 是总结成员（config.toml 的 [summary]），不在 /round 里发言；它的话只给你看，其余成员看不到。\n", id)
			}
		}
	}
	plan := turn.PlanRound(parts, agents, ids)
	// 分类跳过是整条命令的属性。放进下面的 for 会在 --rounds 3 时把
	// 同一份名单印三遍。no_delta 不在这里：它要等每一轮的预算才知道，
	// 依据列已经写了「无新内容，本轮跳过」。空串不印，避免没人被跳过时多一行空白。
	if text := renderSkipped(plan.Skipped); text != "" {
		if err := r.printf("%s", text); err != nil {
			return err
		}
	}
	// 裁判不在 Skipped 里，所以上面的 renderSkipped 碰不到他。
	// PlanRound 把 ModeJudge 归进 Judges：预算只为 Speakers 出行，
	// 没带 --with-judge 时裁判轮也不跑，三处都没有他。
	// 与分类跳过同一处、循环外。Judges 整条命令不变，放进 for
	// 会在 --rounds 3 时把同一行印三遍。带了 --with-judge 不印：
	// 那些人随后以裁判轮出现，这行会把即将要问的人说成不问。
	if !withJudge {
		if text := renderUnaskedJudges(plan.Judges); text != "" {
			if err := r.printf("%s", text); err != nil {
				return err
			}
		}
	}
	// 字典序和去重发生在 PlanRound，屏幕原先不解释，用户会对着键入顺序找人。
	// 与上面相邻，同一次、在循环外。不放进 renderBudget：那份输出被逐行断言。
	if !r.concise {
		if err := r.printf("发言顺序按字典序，重复的 id 已去重，只问一次。\n"); err != nil {
			return err
		}
	}

	// finished 只数 runOneRound 返回 true 的发言轮。提前 break 的那一轮
	// （没带 --go、argv 拒绝、没人可问）没有跑完。分子若仍写 rounds，
	// 取消文案会把没跑的轮说成已跑。
	// 代价：这种 break 之后若 ctx 已取消，屏幕是「已跑 0/N」或少一截。
	// 每一轮都返回 true 时，循环内那条的分子与原来的 k-1 相同。
	finished := 0
	var asked []turn.Record
	for k := 1; k <= rounds; k++ {
		// 只在两轮之间看 ctx。正在跑的那一轮已经起了进程，钱花了，
		// 由 runOneRound 收完；这里停的是还没开始的轮。
		if ctx.Err() != nil {
			return r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)
		}
		// 第一轮复用分类那次。分类与第一轮之间没有第二个时刻。
		snap := snap0
		if k > 1 {
			snap, err = r.rn.TakeRoundSnap(ctx, r.cur.ID)
			if err != nil {
				return r.reportErr("round", err)
			}
		}
		ran, err := runOneRound(ctx, r, plan, goRun, "发言轮", k, rounds, snap, writeAskRecord, &asked)
		if err != nil {
			return err
		}
		// false 只停发言轮，不停裁判轮。三种情形都是这样：
		// 没带 --go，用户要看的就是全部开销，含裁判轮的价；
		// argv 整轮拒绝，裁判是另一批成员，余量各算各的
		// （prompt 只会更长，下一轮发言同样超，不说明裁判超）；
		// 跳过之后没人可问，发言轮没有新内容，下一轮同样没有，
		// 裁判有自己的 delta。
		if !ran {
			break
		}
		spawned = true
		finished++
	}
	if withJudge && len(plan.Judges) > 0 {
		// 裁判只在发言轮循环停下之后跑一次。每个发言轮跟一次就是把一次 flag
		// 收成 N 倍入场费。裁判轮自己再取快照，看见的是发言轮写完之后的历史。
		if ctx.Err() != nil {
			return r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)
		}
		judgeSnap, err := r.rn.TakeRoundSnap(ctx, r.cur.ID)
		if err != nil {
			return r.reportErr("round", err)
		}
		// Plan 的三个切片都要非 nil。PlanRound 给出的就是非 nil 的空切片。
		judgePlan := turn.Plan{
			Speakers: plan.Judges,
			Judges:   []string{},
			Skipped:  []turn.SkipReason{},
		}
		judgeRan, err := runOneRound(ctx, r, judgePlan, goRun, "裁判轮", 0, 0, judgeSnap, writeAskRecord, &asked)
		if judgeRan {
			spawned = true
		}
		if err != nil {
			return err
		}
	}
	if noSummary {
		return nil
	}
	return runSummary(ctx, r, asked, sumID)
}

// runOneRound 跑计划里的一轮：先预报，--go 才起进程。
//
// 返回的 bool 表示发言轮循环还可以继续。false 是这一轮自己已经把结论
// 印完、后面的发言轮不该再跑：没带 --go、整轮 argv 拒绝、跳过之后没有人可问。
// 裁判轮不看这个 bool。这些都不是屏幕写失败，所以 error 为 nil。
// error 非 nil 只表示写屏幕失败或算预算失败，调用方原样返回。
//
// snap 由调用方传入，本函数不再拍。第一轮复用分类那次；其后每一轮各取一次。
// 与上一轮共用同一次快照会把两段历史报成同一份价。
//
// k、n 是命令级轮次：发言轮里 k 从 1 起、n 是这条命令的 --rounds；裁判轮与总结轮两个都传 0。
// 观察者非 nil 时本段发三种事件，屏幕文本与不观察时逐字节相同。
// 三个发射点都在对应文本已经写成功之后：
// RoundStart 在「本轮问 N 个」那一行返回 nil 之后、起进程之前；
// MemberDone 在该成员的明细（含领域错误那一行）写完之后；
// RoundEnd 在结算文本返回 nil 之后、return true 之前。
// 顺序是先有文本、再有事件：观察者看到的屏幕状态已经包含刚打出的那一段。
// RoundStart 带名单的新分配拷贝（不与 plan.Speakers 共享底层数组）；
// RoundEnd 带按名单顺序的本段结果（取自 recs，turn 保证它与 plan.Speakers 同序），供 TUI 无状态地拼回滚块。
// BudgetWith 出错、没带 --go、argv 整轮拒绝、没有人可问这四条早退一个事件都不发。
// 简洁档下，带 --go 的预算表、「扇出期间」、结算块不打。
// 分不清进程是否起来 N（只在 N>0）与 no_delta 跳过另起一行。
// RoundEnd 带 Concise，TUI 据此不打段终态块。
// 屏幕文本与不观察时逐字节相同仍成立：总用时在 cmdRound，不在本函数。
//
// render 是逐条渲染用的函数。发言轮、裁判轮传 writeAskRecord，总结段传
// writeSummaryRecord。格式仍只有一份，差别只在简洁档的两个头。
// asked 非 nil 时，在 writeErr 检查之后把这一段的 recs 追加进去。
// 写屏幕失败的那一段不追加，调用方随即返回错误、不会总结。总结段传 nil。
func runOneRound(ctx context.Context, r *REPL, plan turn.Plan, goRun bool, label string, k, n int, snap turn.RoundSnap, render func(*REPL, turn.Record) error, asked *[]turn.Record) (bool, error) {
	rows, err := r.rn.BudgetWith(ctx, r.cur.ID, plan, snap)
	if err != nil {
		return false, r.reportErr("round", err)
	}
	if !r.concise || !goRun {
		if err := r.printf("%s", renderBudget(rows)); err != nil {
			return false, err
		}
	}
	if !goRun {
		return false, nil
	}
	if msg := argvRefusal(rows); msg != "" {
		if err := r.printf("%s", msg); err != nil {
			return false, err
		}
		return false, nil
	}
	plan = skipNoDelta(plan, rows)
	if r.concise {
		var noDelta []turn.SkipReason
		for _, s := range plan.Skipped {
			if s.Kind == turn.SkipNoDelta {
				noDelta = append(noDelta, s)
			}
		}
		if text := renderSkipped(noDelta); text != "" {
			if err := r.printf("%s", text); err != nil {
				return false, err
			}
		}
	}
	// 空 Speakers 仍调 RunRoundPreparedStream 会落下 round_started 与
	// round_finished 两条系统消息，事实源里多出一轮什么都没问的轮。
	if len(plan.Speakers) == 0 {
		if err := r.printf("这一轮没有人可问。\n"); err != nil {
			return false, err
		}
		return false, nil
	}
	// maxParallel 传 0：契约 4.3 已裁，本步不加配置键，也不另编一个上限。
	// 0 在 RunRoundPreparedStream 里是不设信号量，这一轮发言者同时起进程。
	// 墙钟不再按人数累加，机器和额度的峰值按名单叠上去。
	// 扇出期间这条 REPL 不收下一条命令。上界按并发算，最慢的一个成员说了算，
	// 约 1 x 5 分钟。屏幕上的理由只写命令派发是串行的，不写 r.mu：
	// 契约 4.6 点名了这条，写锁会把已经拆掉的理由又说回去。
	// 代价：每个真正起进程的轮都印一次，含裁判轮；没到这一步的拒绝不印。
	if !r.concise {
		if err := r.printf("扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。\n"); err != nil {
			return false, err
		}
	}
	// 名单打在起进程之前。上面那句只说了「要等多久」，没说「谁在跑」：
	// 五分钟静默里用户至少该知道这一轮问了几个人、分别是谁。用的是
	// skipNoDelta 之后的 plan.Speakers，屏幕上的名单与真正起进程的那批
	// 因此是同一份。
	if err := r.printf("本轮问 %d 个: %s\n", len(plan.Speakers), strings.Join(plan.Speakers, ", ")); err != nil {
		return false, err
	}
	// 观察者只在这一处读一次。本段三种事件都经这个局部变量发，不再读字段。
	// 四条早退（预算失败、没带 --go、argv 整轮拒绝、没有人可问）在此之前已经 return。
	obs := r.roundObs
	if obs != nil {
		obs.RoundStart(RoundStartEvent{Label: label, K: k, N: n, Members: len(plan.Speakers), Speakers: append([]string(nil), plan.Speakers...)})
	}
	// 逐条渲染从「全部回来之后的循环」挪进回调：谁回来先打谁。最慢的
	// 那个成员不再替其余人压着输出。经 render 回调：发言轮与裁判轮传
	// writeAskRecord、总结段传 writeSummaryRecord，两者共用 writeRecord 这一份
	// 格式，不另写一份——两份格式会分叉，而分叉的那一天没有任何测试会红。
	//
	// 回调由 RunRoundWithStream 那一层串行化，同一时刻只有一个在跑，
	// 这里不必自己加锁；REPL 主 goroutine 此刻正阻塞在
	// RunRoundPreparedStream 里，也不会与回调争同一个输出流。
	// MemberDone 在回调末尾发，通常就在这把 progressMu 里；铸 id 失败或
	// round_started 写不进时 turn 走 notifyEach，回调改在本 goroutine 上同步
	// 逐个调用、不持锁。观察者只许做 O(1) 的轻活，不许回调 Runner 或 REPL，
	// 也不许等待任何要取这把锁的东西。
	//
	// writeErr 记下第一次写屏幕失败，其后的成员不再往一个已经写不动的
	// 流上打。闭包里读写它同样靠那一层的锁排好序，这里不另加同步。
	// 等这一轮全部回来再返回它，不在回调里中止：钱已经花了，提前中止
	// 只会连带丢掉结算。
	var writeErr error
	recs := r.rn.RunRoundPreparedStream(ctx, r.cur.ID, plan, 0, snap, func(rec turn.Record) {
		if writeErr != nil {
			return
		}
		if err := render(r, rec); err != nil {
			writeErr = err
			return
		}
		// 领域错误打出来就继续，不中止这一轮。旧的串行循环里第一个错
		// 就 return 是因为当时后面的人还没跑；现在这一轮是并发跑完的，
		// 中止只会吞掉后面几个人已经到手的结果。非 nil 的返回只留给
		// 写屏幕失败。
		if rec.Err != nil {
			if err := r.printf("/round %s: %s\n", rec.AgentID, askCause(rec.Err)); err != nil {
				writeErr = err
				return
			}
		}
		// 本次调用里任何一次写屏幕失败都不发。成功路径上这是闭包的最后一个动作。
		if obs != nil {
			obs.MemberDone(MemberDoneEvent{
				Label:         label,
				AgentID:       rec.AgentID,
				Outcome:       rec.Outcome,
				MaybeAdvanced: rec.MaybeAdvanced,
				Drifted:       rec.Drifted,
				Dropped:       rec.DroppedText != "",
				WallMS:        rec.WallMS,
				RoundID:       rec.RoundID,
			})
		}
	})
	if writeErr != nil {
		return false, writeErr
	}
	if asked != nil {
		*asked = append(*asked, recs...)
	}
	// NoDelta 的人随后被 skipNoDelta 挪走，不起进程。把他们算进预报，
	// 结算行会同时出现更大的「预报冷启动」和更小的「确知起进程」，中间没有解释。
	// 只数这一轮仍会起进程的人。代价：冷且无新内容的成员不进这个数。
	forecastCold := 0
	for _, row := range rows {
		if row.Cold && !row.NoDelta {
			forecastCold++
		}
	}
	var unexpected []string
	for _, rec := range recs {
		if rec.UnexpectedCold {
			unexpected = append(unexpected, rec.AgentID)
		}
	}
	settled := turn.Settle(recs)
	if r.concise {
		if settled.SpawnFailed > 0 {
			if err := r.printf("%s", spawnFailedLine(settled.SpawnFailed)); err != nil {
				return false, err
			}
		}
	} else {
		if err := r.printf("%s", renderSettlement(settled, forecastCold, unexpected, label)); err != nil {
			return false, err
		}
	}
	if obs != nil {
		results := make([]MemberDoneEvent, 0, len(recs))
		for _, rec := range recs {
			results = append(results, MemberDoneEvent{
				Label:         label,
				AgentID:       rec.AgentID,
				Outcome:       rec.Outcome,
				MaybeAdvanced: rec.MaybeAdvanced,
				Drifted:       rec.Drifted,
				Dropped:       rec.DroppedText != "",
				WallMS:        rec.WallMS,
				RoundID:       rec.RoundID,
			})
		}
		obs.RoundEnd(RoundEndEvent{Label: label, K: k, N: n, Results: results, Concise: r.concise})
	}
	return true, nil
}

// lastUserMentionIDs 取当前会话最后一条 KindUser 消息里的 mention。
// 从最大 seq 往回按窗口读，读到第一条 user 消息就停；一条都没有时返回空切片。
func lastUserMentionIDs(ctx context.Context, r *REPL) ([]string, error) {
	maxSeq, err := r.svc.MaxSeq(ctx, r.cur.ID)
	if err != nil {
		return nil, err
	}
	const window int64 = 64
	end := maxSeq
	for end > 0 {
		start := end - window
		if start < 0 {
			start = 0
		}
		msgs, err := r.svc.History(ctx, r.cur.ID, start, int(end-start))
		if err != nil {
			return nil, err
		}
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Kind == conversation.KindUser {
				return ParseMentions(msgs[i].Text), nil
			}
		}
		end = start
	}
	return []string{}, nil
}

// renderSkipped 把 PlanRound 判进 Skipped 的人印成屏幕文本。
//
// 每人一行，带 AgentID、Kind、Reason。空切片返回空串，调用方据此不印，
// 避免没人被跳过时多一行空白。no_delta 不会出现在这次调用里：打印点在
// 进轮次循环之前，而这一档要等预算才追加；依据列已经写过同一句话。
//
// 不再为 mode 补「若他是裁判」。裁判不在 Skipped 里，所以这里碰不到他。
// 那半句把分类跳过和没带 --with-judge 混在一起，对五值以外的 mode 是误导。
// 点名裁判在 renderUnaskedJudges。
// 代价：Reason 里通常已经带了 id，行上会再出现一次。
func renderSkipped(skipped []turn.SkipReason) string {
	if len(skipped) == 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range skipped {
		fmt.Fprintf(&b, "跳过 %s（%s）：%s", s.AgentID, s.Kind, s.Reason)
		b.WriteString("\n")
	}
	return b.String()
}

// renderUnaskedJudges 点名这条命令不会问的裁判。
//
// 裁判不在 Skipped 里，所以 renderSkipped 碰不到他。PlanRound 把
// ModeJudge 归进 Judges，不归进 Speakers，也不归进 Skipped。预算只为
// Speakers 出行。没带 --with-judge 时裁判轮不跑。三处都没有他，
// 屏幕就会一个字不提。这行只补展示，不改分类。
//
// 空切片返回空串，调用方据此不印。本函数不看 --with-judge：带了这个
// flag 时人随后以裁判轮出现，调用方不要印，免得把即将要问的人说成不问。
// 顺序与去重沿用传入切片，这里不再排一次。
func renderUnaskedJudges(judges []string) string {
	if len(judges) == 0 {
		return ""
	}
	return fmt.Sprintf("裁判 %s 本命令不问，要用 --with-judge 才问。\n", strings.Join(judges, "、"))
}

// renderBudget 把已经算好的预报行收成屏幕文本。
//
// 纯函数：不碰 REPL、不碰库、不起进程。internal/repl 的假 Store 让
// UsageSamples 返回 nil, nil，命令层用例里 Calibrated 恒为 false，
// 「已标定的行怎么印」在 cmdRound 里结构上走不到，只能从这里喂合成数据。
//
// 合计只加 InputTokens。金额不加：只有一部分 provider 报金额，为了凑
// 总额去编汇率，就是把没这个数写成一个数。Cold 是冷暖预报，不是价钱。
func renderBudget(rows []turn.BudgetRow) string {
	var b strings.Builder
	b.WriteString("成员  输入token  金额  冷启动  在飞  命令行余量  依据\n")
	sum := 0
	uncal := 0
	for _, row := range rows {
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s  %s  %s\n",
			row.AgentID,
			budgetTokens(row),
			budgetCost(row),
			budgetCold(row),
			budgetFlight(row),
			budgetHeadroom(row),
			budgetBasis(row),
		)
		if row.Calibrated && row.InputTokens != nil {
			sum += *row.InputTokens
		}
		if !row.Calibrated {
			uncal++
		}
	}
	if uncal > 0 {
		fmt.Fprintf(&b, "合计输入token %d（只含已标定的行）\n", sum)
	} else {
		fmt.Fprintf(&b, "合计输入token %d\n", sum)
	}
	// 0 是数过之后的「没有未标定」，不是量不出。全员已标定时这行是废话。
	// 只省这一行，合计行的两种写法不动。
	if uncal > 0 {
		fmt.Fprintf(&b, "另有 %d 名未标定\n", uncal)
	}
	return b.String()
}

func budgetTokens(row turn.BudgetRow) string {
	if !row.Calibrated || row.InputTokens == nil {
		return "未标定"
	}
	return fmt.Sprintf("%d", *row.InputTokens)
}

func budgetCost(row turn.BudgetRow) string {
	// 金额和 token 分开认。标定过但样本没报金额时，只这一格是未标定。
	if !row.Calibrated || row.CostUSD == nil {
		return "未标定"
	}
	return fmt.Sprintf("%g USD", *row.CostUSD)
}

func budgetCold(row turn.BudgetRow) string {
	if row.Cold {
		return "预报冷"
	}
	return "预报非冷"
}

func budgetFlight(row turn.BudgetRow) string {
	if row.InFlight {
		return "在飞"
	}
	return "不在飞"
}

func budgetHeadroom(row turn.BudgetRow) string {
	// nil 是量不出余量，不是 0。负数是真的超了，夹到 0 会把闸藏掉。
	if row.ArgvHeadroom == nil {
		return "余量未知"
	}
	return fmt.Sprintf("%d", *row.ArgvHeadroom)
}

func budgetBasis(row turn.BudgetRow) string {
	// NoDelta 不是「没取到样本」。token 格仍印未标定，只为了不编一个 0；
	// 依据列改印跳过，这行才不会和普通的标定失败长得一样。
	if row.NoDelta {
		return "无新内容，本轮跳过"
	}
	if row.Basis == "" {
		return "无样本"
	}
	return row.Basis
}

// renderSettlement 把 Settle 的结果收成屏幕文本。
//
// Have* 为 false 时那一列的零值是「没人报过」，印成 0 就是把不知道写成合计为零。
// SpawnFailed 单独成行：这一档分不清进程有没有起来，并进 Spawned 或省掉都会把不知道写成一个数。
// 实测冷启动只数冷暖确知的成员：判据是 turn.Record.MeasuredCold，Settle 据它收成 Cold 与 ColdUnknown。
// HaveCacheRead 为 false，或没有一个成员冷暖确知时，实测冷启动印未知；有人冷暖未知时数字带限定语（见 measuredCold）。
func renderSettlement(s turn.RoundSettlement, forecastCold int, unexpectedCold []string, label string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s结算\n", label)
	fmt.Fprintf(&b, "确知起进程 %d\n", s.Spawned)
	b.WriteString(spawnFailedLine(s.SpawnFailed))
	fmt.Fprintf(&b, "用量 输入 %s  输出 %s  cache_read %s  cache_create %s  thinking %s%s\n",
		settledTokens(s.HaveIn, s.InputTokens),
		settledTokens(s.HaveOut, s.OutputTokens),
		settledTokens(s.HaveCacheRead, s.CacheRead),
		settledTokens(s.HaveCacheCreate, s.CacheCreate),
		settledTokens(s.HaveThinking, s.ThinkingTokens),
		usageUnknownNote(s.UsageUnknown),
	)
	fmt.Fprintf(&b, "金额 %s\n", settledCost(s))
	fmt.Fprintf(&b, "冷启动 预报 %d  实测 %s\n", forecastCold, measuredCold(s))
	if len(unexpectedCold) > 0 {
		fmt.Fprintf(&b, "意外冷启动 %s\n", strings.Join(unexpectedCold, ", "))
	}
	return b.String()
}

// spawnFailedLine 是「分不清进程是否起来」那一行。
// 详细档的结算块与简洁档那一行共用它，格式只有一份。renderSettlement 的输出逐字节不变。
func spawnFailedLine(n int) string {
	line := fmt.Sprintf("分不清进程是否起来 %d", n)
	if n > 0 {
		line += "（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）"
	}
	return line + "\n"
}

func settledTokens(have bool, n int) string {
	if !have {
		return "未知"
	}
	return fmt.Sprintf("%d", n)
}

func usageUnknownNote(k int) string {
	if k <= 0 {
		return ""
	}
	return fmt.Sprintf("（%d 名成员用量未知，合计只含其余人）", k)
}

func settledCost(s turn.RoundSettlement) string {
	if !s.HaveCost {
		if s.CostUnreported > 0 {
			return fmt.Sprintf("未知（%d 名成员未报金额）", s.CostUnreported)
		}
		return "未知"
	}
	text := fmt.Sprintf("%g USD", s.CostUSD)
	if s.CostUnreported > 0 {
		text += fmt.Sprintf("（%d 名成员未报金额，合计只含其余人）", s.CostUnreported)
	}
	return text
}

// measuredCold 的 excluded 只取 ColdUnknown：按 Settle 的口径它已经含五个用量都没有数的成员
// （UsageUnknown 数的人全在其中），再加 UsageUnknown 就把同一个人数了两次。excluded 为 0 时不带限定语。
func measuredCold(s turn.RoundSettlement) string {
	excluded := s.ColdUnknown
	knownN := s.Spawned - excluded
	if !s.HaveCacheRead || knownN <= 0 {
		return "未知"
	}
	if excluded > 0 {
		return fmt.Sprintf("%d（%d 名成员冷暖未知，只数其余人）", s.Cold, excluded)
	}
	return fmt.Sprintf("%d", s.Cold)
}

// argvRefusal 在任一成员余量为负时拒绝整轮，返回空串表示可以跑。
//
// nil 不当成 0：量不出余量不是「已经超了」。只跳过超限的那几个、其余人照跑，
// 会花掉还能跑的那些人的钱，留下残缺的一轮；整轮不跑比这个便宜。
func argvRefusal(rows []turn.BudgetRow) string {
	var over []string
	for _, row := range rows {
		if row.ArgvHeadroom == nil || *row.ArgvHeadroom >= 0 {
			continue
		}
		over = append(over, fmt.Sprintf("%s 超了 %d 字节", row.AgentID, -*row.ArgvHeadroom))
	}
	if len(over) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"拒绝整轮: %s。命令行余量为负的人这一轮必然失败。若只把他们拿掉、其余人继续跑，其余人的钱照花，用户拿到残缺的一轮（五个人里两个必然失败、另外三个的钱照花，就是这个形状）；整轮不跑比部分跑更便宜。\n",
		strings.Join(over, "；"),
	)
}

// skipNoDelta 把预报判为没有新内容的发言者追加进 Skipped。
//
// PlanRound 产不出这一档（它看不到历史）。替换整个 Skipped 会丢掉
// not_in_config / not_on_roster / mode。留在 Speakers 里则会把明知
// 没有 delta 的人送进这一轮的 ask。
func skipNoDelta(plan turn.Plan, rows []turn.BudgetRow) turn.Plan {
	noDelta := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.NoDelta {
			noDelta[row.AgentID] = struct{}{}
		}
	}
	if len(noDelta) == 0 {
		return plan
	}
	kept := make([]string, 0, len(plan.Speakers))
	for _, id := range plan.Speakers {
		if _, hit := noDelta[id]; !hit {
			kept = append(kept, id)
			continue
		}
		plan.Skipped = append(plan.Skipped, turn.SkipReason{
			AgentID: id,
			Kind:    turn.SkipNoDelta,
			Reason:  fmt.Sprintf("%s 无新内容，本轮跳过", id),
		})
	}
	plan.Speakers = kept
	return plan
}
