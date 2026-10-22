// Package headless 实现 `council ask`：一次调用、一个结果、进程退出。
// 它不读 stdin、不画横幅、不进 REPL —— 这些事由 internal/app 与
// cmd/council 在外层决定；本包只认已经装配好的 Deps。
package headless

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/turn"
)

// status 是契约 §2.2 的四个取值。partial 退 0，其余两个失败态退 1。
const (
	statusOK             = "ok"
	statusPartial        = "partial"
	statusBudgetExceeded = "budget_exceeded"
	statusError          = "error"
)

// Result 是 -json 时 stdout 上的那个对象，也是文本模式的唯一数据来源。
// members / turns 永远是非 nil 切片：契约里它们是数组，不是 null。
type Result struct {
	Status         string     `json:"status"`
	ConversationID string     `json:"conversation_id"`
	Summary        string     `json:"summary"`
	Members        []string   `json:"members"`
	Turns          []TurnJSON `json:"turns"`
	CostUSD        float64    `json:"cost_usd"`
	// CostComplete：名单里每一位的 provider 都报金额才是 true。有一位
	// ReportsCost=SupportNo（grok / agy / codex），cost_usd 就只是
	// 「报过金额的那些轮」之和，不能当完整账看。为 false 不阻止执行，
	// 也不改 status：预算这一层只能对报金额的 provider 生效（D5）。
	CostComplete bool `json:"cost_complete"`
	// Workspace 只在 -workspace 时出现；nil 时 JSON 里没有这个键。
	Workspace *WorkspaceResult `json:"workspace,omitempty"`
	// Failed / Total 是给人复述这次成败的一组数，按「席位」算：发言名单
	// 加上真的跑过的汇总成员。汇总轮不进 turns（它的正文在 summary 里），
	// 所以调用方没法从 turns 数出正确的分母——插件先前就是因此把
	// 「全员成功、只有汇总失败」报成 0/N。这两个字段就是给那种调用方用的，
	// 与 stderr 的 `partial: f/t` 逐字同源。
	Failed int `json:"failed"`
	Total  int `json:"total"`
}

// WorkspaceResult 是 -workspace 的结果：副本路径、完整 diff、改动文件。
// 超过 1 MiB 时 Diff 为空、DiffTruncated 为 true、DiffPath 指向落到副本
// 旁边的全文，避免把一大段 diff 塞进 dsh 的事件上限。
type WorkspaceResult struct {
	Copy          string   `json:"copy"`
	Diff          string   `json:"diff"`
	Files         []string `json:"files"`
	DiffTruncated bool     `json:"diff_truncated,omitempty"`
	DiffPath      string   `json:"diff_path,omitempty"`
}

// TurnJSON 是一个成员一轮的结果。字段与契约 §2.2 一一对应；
// round 从 1 起，是命令级轮次，不是 turn.Record.RoundID。
type TurnJSON struct {
	AgentID    string    `json:"agent_id"`
	Model      string    `json:"model"`
	Effort     string    `json:"effort"`
	Round      int       `json:"round"`
	Outcome    string    `json:"outcome"`
	Text       string    `json:"text"`
	Usage      UsageJSON `json:"usage"`
	CostUSD    float64   `json:"cost_usd"`
	DurationMS int       `json:"duration_ms"`
}

// UsageJSON 是契约点名的四个键。没报的项写 0：契约的 JSON 形状没有
// null 这一档，而 -json 的读者也不该从「缺键」猜「未报」。
//
// 口径：in 是含缓存的完整 prompt 总量，cache_create / cache_read 是它的
// 细分，四者不是互斥桶。不要把四个数相加，相加会重复计数。
type UsageJSON struct {
	In          int `json:"in"`
	CacheCreate int `json:"cache_create"`
	CacheRead   int `json:"cache_read"`
	Out         int `json:"out"`
}

// Run 执行一次 ask。它只做五件事：定名单、建/接会话、入会并落问题、
// 跑轮次、汇总。所有给人看的东西走 Err；Out 在 -json 时只有一个对象。
//
// 返回 *UsageError 表示命令行用错（退出码 2）；其余非 nil 表示执行失败
// （退出码 1）。status=partial 不算失败：成员各自失败是业务结果，不是
// 这次调用崩了（D6）。
func Run(ctx context.Context, deps Deps, opts Options) error {
	out := deps.Out
	if out == nil {
		out = io.Discard
	}
	errOut := deps.Err
	if errOut == nil {
		errOut = io.Discard
	}
	if opts.Mode == "" {
		opts.Mode = "ask"
	}

	if deps.Cfg == nil {
		fmt.Fprintln(errOut, "council ask: 配置未加载，无法执行")
		return fmt.Errorf("headless: 配置未加载")
	}
	if err := Validate(opts, deps.Cfg); err != nil {
		fmt.Fprintf(errOut, "council ask: %v\n", err)
		return err
	}
	// app.Run 在 svc 为 nil 时不会构造 Runner。那一刻没有库也没有 provider
	// 调用，只能把话说明白退 1；不能在下面对 rn 解引用。
	if deps.Svc == nil || deps.Rn == nil {
		fmt.Fprintln(errOut, "council ask: 持久化或 provider 调用未就绪，无法执行")
		return fmt.Errorf("headless: 持久化或 provider 调用未就绪")
	}

	members, err := resolveMembers(opts, deps.Cfg)
	if err != nil {
		fmt.Fprintf(errOut, "council ask: %v\n", err)
		return err
	}
	sumID, err := resolveSummary(opts, deps.Cfg)
	if err != nil {
		fmt.Fprintf(errOut, "council ask: %v\n", err)
		return err
	}

	// 仓库根校验放在建会话之前：不是 git 仓库根是用法错误，不该先留下
	// 一条会话、一条 user 消息再退出。副本本身要等 convID 出来才能建。
	repoRoot := ""
	if opts.Workspace != "" {
		repoRoot, err = workspaceRepoRoot(ctx, opts.Workspace)
		if err != nil {
			fmt.Fprintf(errOut, "council ask: %v\n", err)
			return err
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	s := &askState{
		deps:      deps,
		opts:      opts,
		rn:        deps.Rn,
		out:       out,
		errOut:    errOut,
		verbose:   opts.Verbose || deps.Verbose,
		members:   members,
		summaryID: sumID,
		failed:    make(map[string]struct{}),
	}

	convID := opts.Conversation
	if convID == "" {
		// 标题取问题正文：/conv 列表里能一眼认出这次 ask 问的是什么。
		conv, createErr := deps.Svc.CreateIn(runCtx, strings.TrimSpace(opts.Prompt), deps.ProjectDir)
		if createErr != nil {
			s.execErr = fmt.Errorf("建会话失败: %w", createErr)
			return s.finish(statusError)
		}
		convID = conv.ID
	} else if _, convErr := deps.Svc.Conversation(runCtx, convID); convErr != nil {
		s.convID = convID
		s.execErr = fmt.Errorf("会话 %q 不存在或读不出来: %w", convID, convErr)
		return s.finish(statusError)
	}
	s.convID = convID
	// 会话已经确定：serve 在这里就能把浏览器切到新会话，不必等第一轮回来。
	// 放在落问题之前，是因为「这次问哪个会话」在 Post 之前就已经是不可变的
	// 事实了；万一后面的成员入会/落问题失败，也已经有一个真实存在的会话
	// 被创建出来，started 报它并不是谎报。
	if deps.OnStart != nil {
		deps.OnStart(convID)
	}

	if repoRoot != "" {
		copyDir, prepErr := prepareWorkspace(runCtx, repoRoot, deps.DataDir, convID)
		if prepErr != nil {
			s.execErr = fmt.Errorf("建 workspace 副本失败: %w", prepErr)
			return s.finish(statusError)
		}
		s.copyDir = copyDir
		// 固定工作目录只换成员 cwd，不换 store、不换会话。rules 从已经
		// 装配好的 Runner 上取，保证副本里的指纹与正常模式同一份规则。
		fixedRn, rnErr := turn.NewRunnerFixedWorkDir(deps.Cfg, deps.Svc, copyDir,
			filepath.Join(deps.DataDir, "workdir"), deps.Rn.Rules())
		if rnErr != nil {
			s.execErr = fmt.Errorf("构造固定工作目录 Runner 失败: %w", rnErr)
			return s.finish(statusError)
		}
		s.rn = fixedRn
		fmt.Fprintf(errOut, "council: -workspace 为本次调用新建 worktree 副本：%s（跑完返回 diff，不自动应用）\n", copyDir)
		s.auditWorkspaceDrift(runCtx)
	}

	active, err := deps.Svc.Participants(runCtx, convID, false)
	if err != nil {
		s.execErr = fmt.Errorf("列出成员失败: %w", err)
		return s.finish(statusError)
	}
	alreadyIn := make(map[string]struct{}, len(active))
	for _, p := range active {
		alreadyIn[p.AgentID] = struct{}{}
	}
	for _, id := range members {
		if _, ok := alreadyIn[id]; ok {
			continue
		}
		ac := deps.Cfg.Agents[id]
		part := conversation.Participant{
			ConvID:      convID,
			AgentID:     id,
			DisplayName: ac.DisplayName,
			Provider:    ac.Provider,
			Active:      true,
			JoinedAt:    time.Now().UTC(),
		}
		if joinErr := deps.Svc.Join(runCtx, convID, part); joinErr != nil {
			s.execErr = fmt.Errorf("成员 %s 入会失败: %w", id, joinErr)
			return s.finish(statusError)
		}
	}

	// 问题必须先落库再起进程：成员看到的历史里若没有这句话，
	// 这一轮就是花钱回答一个不存在的问题。
	if _, postErr := deps.Svc.Post(runCtx, conversation.Message{
		ConvID:   convID,
		SenderID: conversation.ReservedSenderUser,
		Kind:     conversation.KindUser,
		Text:     opts.Prompt,
	}); postErr != nil {
		s.execErr = fmt.Errorf("问题落库失败: %w", postErr)
		return s.finish(statusError)
	}

	// PlanRound 每轮重做：成员名单在一条命令里不变，但每一轮都要拿
	// 当前快照重新分类，与 /round 的做法一致。
	parts, err := deps.Svc.Participants(runCtx, convID, false)
	if err != nil {
		s.execErr = fmt.Errorf("列出成员失败: %w", err)
		return s.finish(statusError)
	}

	for k := 1; k <= opts.Rounds; k++ {
		if ctxErr := runCtx.Err(); ctxErr != nil {
			s.noteContext(ctxErr)
			break
		}
		snap, snapErr := s.rn.TakeRoundSnap(runCtx, convID)
		if snapErr != nil {
			s.execErr = fmt.Errorf("取轮次快照失败: %w", snapErr)
			break
		}
		plan := turn.PlanRound(parts, deps.Cfg.Agents, members)
		// 契约 §2.1 的 -members 是「按给出顺序发言」，而 PlanRound 为了
		// 输出稳定把 Speakers 排成字典序。分类与跳过仍由 PlanRound 决定，
		// 这里只把留下来的发言者按 members 的顺序摆回去。
		plan.Speakers = orderSpeakers(plan.Speakers, members)
		if k == 1 {
			// 被 PlanRound 判走的人不会出现在 turns 里。headless 没有 /round
			// 的屏幕，只剩这一行能解释「名单里有他、结果里没有」。
			for _, skipped := range plan.Skipped {
				fmt.Fprintf(errOut, "council: 跳过 %s：%s\n", skipped.AgentID, skipped.Reason)
			}
			// 裁判不在 Skipped 里，不补这行就会无声消失。council ask 没有
			// --with-judge，这一轮不跑裁判轮，所以不照 /round 提那个 flag。
			if len(plan.Judges) > 0 {
				fmt.Fprintf(errOut, "裁判 %s 本命令不问：council ask 不跑裁判轮。\n", strings.Join(plan.Judges, "、"))
			}
		}
		if len(plan.Speakers) == 0 {
			s.execErr = fmt.Errorf("名单里没有可发言的成员")
			break
		}
		if s.verbose {
			fmt.Fprintf(errOut, "council: 第 %d/%d 轮，问 %d 个成员\n", k, opts.Rounds, len(plan.Speakers))
		}
		recs := s.runPlan(runCtx, plan, snap, k)
		for _, rec := range recs {
			s.addTurn(rec, k)
			// 记账完成之后才通知：回调看到的一定是已经进了 turns / failed /
			// costUSD 的那条结果，serve 推给浏览器的费用与最终 done 同源。
			// 只在这一处调，汇总轮不经过 addTurn，也就不会在这里出现——
			// 汇总正文最终由 done.summary 一次交付。
			if deps.OnRecord != nil {
				deps.OnRecord(rec)
			}
		}
		// 每一条都在起进程之前被拒，说明问题不在成员，而在这套装配
		// （库、规则、身份指纹、权限闸）。这不是「有人失败」那一档的
		// partial，整轮根本没跑起来，按执行错误退 1。
		if allRefusedWithErr(recs) {
			s.execErr = fmt.Errorf("整轮没有跑起来: %w", recs[0].Err)
			break
		}
		if s.costUSD >= opts.BudgetUSD {
			s.budgetExceeded = true
			break
		}
	}
	// 循环最后一轮里取消的那一次不会在下一轮开头被看见。这里再收一次，
	// 让「第一次 Ctrl-C 仍输出 partial」对最后一轮也成立。
	if runCtx.Err() != nil && s.execErr == nil && !s.budgetExceeded {
		s.noteContext(runCtx.Err())
	}

	if s.execErr == nil && !s.budgetExceeded && !s.canceled && runCtx.Err() == nil {
		if sumErr := s.runSummary(runCtx); sumErr != nil {
			if runCtx.Err() != nil {
				// 总结轮在飞时 ctx 结束：发言轮已经付过钱、有结果，
				// 按契约 §2.4 与发言轮同权，不能因为总结没回来就整次报错。
				s.noteSummaryContext(runCtx.Err())
			} else {
				s.execErr = sumErr
			}
		}
	}
	// 总结轮也可能在 ctx 结束时直接返回一条失败记录（Runner 内部超时
	// 或外部取消），错误不一定从 runSummary 的返回值上来。这里再收一次，
	// 但只在真的尝试过总结时按 partial 处理；连总结都没开始就超时，
	// 仍按任务书 §4 的 error。
	if runCtx.Err() != nil && s.execErr == nil && !s.budgetExceeded && !s.canceled {
		if s.summaryAttempted {
			s.noteSummaryContext(runCtx.Err())
		} else {
			s.noteContext(runCtx.Err())
		}
	}
	// 总结也要花钱。它成功把钱加进 costUSD 之后必须重查一次上限，
	// 否则「发言轮未触线、总结把它推过线」会漏报成 ok。
	if s.execErr == nil && !s.budgetExceeded && s.costUSD >= opts.BudgetUSD {
		s.budgetExceeded = true
	}
	// diff 一定要收：成员已经改过副本，哪怕这一趟被取消、超时或触线，
	// 那些文件也真实存在。收集不跟着 runCtx 一起死（见 collectWorkspace）。
	if s.copyDir != "" {
		if wsErr := s.collectWorkspace(runCtx); wsErr != nil {
			// 收集失败只在既没有取消、也没有预算触线时升级成 error；
			// 那两档已经有自己的 status，不能被一句“diff 没收上”改成
			// error（文件还在副本里，用户照样能自己看）。
			if s.execErr == nil && !s.canceled && !s.budgetExceeded {
				s.execErr = wsErr
			} else {
				fmt.Fprintf(errOut, "council: %v\n", wsErr)
			}
		}
	}

	status := statusOK
	switch {
	case s.execErr != nil:
		status = statusError
	case s.canceled:
		// 「有 turn 记录」不等于「有回复」：取消时 Runner 仍会为每个在飞
		// 成员返回一条 OutcomeCanceled 记录，turns 因此非空。契约 §2.4 说的
		// 「一条回复都还没有就取消」只能按正文判。
		if !s.hasReply() {
			s.execErr = errors.New("已取消，尚无成员回复")
			status = statusError
		} else {
			status = statusPartial
		}
	case s.budgetExceeded:
		status = statusBudgetExceeded
	case len(s.failed) > 0 || s.summaryFailed:
		status = statusPartial
	}
	return s.finish(status)
}

// askState 收集一次 Run 的中间结果。字段都在 Run 的主 goroutine 上写，
// 成员回调由 turn 那一层串行化，所以这里不需要锁。
type askState struct {
	deps      Deps
	opts      Options
	out       io.Writer
	errOut    io.Writer
	verbose   bool
	convID    string
	members   []string
	summaryID string
	rn        *turn.Runner
	copyDir   string

	turns          []TurnJSON
	asked          []turn.Record
	costUSD        float64
	failed         map[string]struct{}
	budgetExceeded bool
	canceled       bool
	execErr        error
	summary        string
	// summaryAttempted 只在真的把汇总成员送进 RunRoundPrepared 之后为真。
	// summaryFailed 只在 round 模式、确实尝试过、且没拿到正文时为真：
	// 显式要求收口却没收到，算 partial；预算/取消跳过的总结不算。
	summaryAttempted bool
	summaryFailed    bool
	workspace        *WorkspaceResult
}

// noteContext 区分「到点超时」与「用户取消」。前者按 error 处理（任务书
// §4 的超时一条）；后者按契约 §2.4：已有结果时仍输出 partial。
func (s *askState) noteContext(err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		s.execErr = fmt.Errorf("超时（%s）: %w", s.opts.Timeout, err)
		return
	}
	s.canceled = true
}

// noteSummaryContext 是总结轮专用的 ctx 收尾。发言轮已经有结果时，
// 总结轮被取消或到点都不能把整次调用打成 error：契约 §2.4 说汇总轮
// 与发言轮同权，已拿到的结果仍按 partial 输出、退 0。一条 turn 都没有
// 时没有这个前提，退回 noteContext（取消→error）。
func (s *askState) noteSummaryContext(err error) {
	if len(s.turns) == 0 {
		s.noteContext(err)
		return
	}
	s.canceled = true
}

// runPlan 跑一轮。不 verbose 时用 RunRoundPrepared（与任务书给的调用一致）；
// verbose 时换成带回调的变体，谁回来先打谁，回调只写 stderr。
func (s *askState) runPlan(ctx context.Context, plan turn.Plan, snap turn.RoundSnap, round int) []turn.Record {
	// -workspace 下成员串行：两个成员同时写同一份副本会互相踩，diff 也
	// 混在一起说不清是谁改的。非 workspace 模式仍传 0（不限）。
	maxParallel := 0
	if s.opts.Workspace != "" {
		maxParallel = 1
	}
	if !s.verbose {
		return s.rn.RunRoundPrepared(ctx, s.convID, plan, maxParallel, snap)
	}
	label := "发言轮"
	if round == 0 {
		label = "总结轮"
	}
	return s.rn.RunRoundPreparedStream(ctx, s.convID, plan, maxParallel, snap, func(rec turn.Record) {
		fmt.Fprintf(s.errOut, "council: %s %s %s（%dms，%.4f USD）\n",
			label, rec.AgentID, outcomeName(rec.Outcome), rec.WallMS, costOf(rec))
	})
}

// orderSpeakers 把 PlanRound 排好序的发言者按调用方给出的成员顺序重排。
// 不在 order 里的发言者（理论上不会有）保持原相对顺序追加在末尾，
// 这样即使 PlanRound 以后加了新的来源，也不会有人被悄悄丢掉。
func orderSpeakers(speakers, order []string) []string {
	inPlan := make(map[string]struct{}, len(speakers))
	for _, id := range speakers {
		inPlan[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(speakers))
	out := make([]string, 0, len(speakers))
	for _, id := range order {
		if _, ok := inPlan[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range speakers {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// allRefusedWithErr 报告这一轮是不是整轮都没跑起来。判据是「每条都是
// 起进程之前被拒绝、带 Err、而且没有 TurnID」。
//
// TurnID 是这条判据的关键：RunRoundPrepared 在 mint round id / 写
// round_started 失败时预填的记录不带 TurnID（scheduler.go 的预填分支），
// 而成员进入执行之后才铸 TurnID，那时被拒（FULL 权限、prompt 过长等）
// 属于成员级失败，按 D6 仍是 partial，不能被提成 error。
func allRefusedWithErr(recs []turn.Record) bool {
	if len(recs) == 0 {
		return false
	}
	for _, rec := range recs {
		if rec.Outcome != turn.OutcomeRefused || rec.Err == nil || rec.TurnID != "" {
			return false
		}
	}
	return true
}

// hasReply 报告这一趟是否拿到过任何正文。取消时每条在飞成员都会留下一条
// OutcomeCanceled 记录，光看 len(turns) 会把「一条回复都没有」误判成有结果。
func (s *askState) hasReply() bool {
	for _, rec := range s.asked {
		if rec.Message != nil || rec.DroppedText != "" {
			return true
		}
	}
	return false
}

func (s *askState) addTurn(rec turn.Record, round int) {
	s.turns = append(s.turns, turnJSON(s.deps.Cfg, rec, round))
	s.asked = append(s.asked, rec)
	s.costUSD += costOf(rec)
	if rec.Outcome != turn.OutcomeSpoke {
		s.failed[rec.AgentID] = struct{}{}
	}
}

// runSummary 照 internal/repl/summary.go 的既有判据问一次汇总成员：
// 少于两个不同成员问过、没人真正回复、ctx 已经取消，都不总结。
// 已退出的汇总成员不拉回（/leave 是用户的明确意图）；没入过会的才 Join。
//
// 唯一与 REPL 不同的地方是去向：结果只进 Result.Summary，不写屏幕。
func (s *askState) runSummary(ctx context.Context) error {
	if s.summaryID == "" {
		return nil
	}
	order := make([]string, 0, len(s.asked))
	seen := make(map[string]struct{}, len(s.asked))
	replied := false
	for _, rec := range s.asked {
		if _, ok := seen[rec.AgentID]; !ok {
			seen[rec.AgentID] = struct{}{}
			order = append(order, rec.AgentID)
		}
		if rec.Message != nil {
			replied = true
		}
	}
	if len(order) < 2 || !replied {
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	parts, err := s.deps.Svc.Participants(ctx, s.convID, true)
	if err != nil {
		return fmt.Errorf("列出汇总成员失败: %w", err)
	}
	joined := false
	for _, p := range parts {
		if p.AgentID != s.summaryID {
			continue
		}
		if !p.Active {
			if s.verbose {
				fmt.Fprintf(s.errOut, "council: 汇总成员 %s 已退出当前会话，本次不汇总\n", s.summaryID)
			}
			return nil
		}
		joined = true
	}
	if !joined {
		ac := s.deps.Cfg.Agents[s.summaryID]
		if err := s.deps.Svc.Join(ctx, s.convID, conversation.Participant{
			ConvID:      s.convID,
			AgentID:     s.summaryID,
			DisplayName: ac.DisplayName,
			Provider:    ac.Provider,
			Active:      true,
			JoinedAt:    time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("汇总成员 %s 入会失败: %w", s.summaryID, err)
		}
	}
	snap, err := s.rn.TakeRoundSnap(ctx, s.convID)
	if err != nil {
		return fmt.Errorf("取总结轮快照失败: %w", err)
	}
	plan := turn.Plan{
		Speakers: []string{s.summaryID},
		Judges:   []string{},
		Skipped:  []turn.SkipReason{},
	}
	s.summaryAttempted = true
	recs := s.runPlan(ctx, plan, snap, 0)
	if len(recs) == 0 {
		return nil
	}
	rec := recs[0]
	s.costUSD += costOf(rec)
	if rec.Message != nil {
		s.summary = rec.Message.Text
	} else if rec.DroppedText != "" {
		s.summary = rec.DroppedText
	}
	if rec.Outcome != turn.OutcomeSpoke {
		fmt.Fprintf(s.errOut, "council: 汇总成员 %s 没有回复（%s）\n", s.summaryID, outcomeName(rec.Outcome))
	}
	// ask 的汇总可有可无，失败只说明；round 的汇总是 D4 的显式要求，
	// 真尝试过却一个字都没拿到，计入失败让 status 变 partial。被预算
	// 或取消跳过的总结走不到这里，不会误记。
	if s.summary == "" && ctx.Err() == nil && s.opts.Mode == "round" {
		s.summaryFailed = true
	}
	return nil
}

// finish 写结果、写必须的 stderr 行，并把退出码信息折进返回值。
// status 是唯一的分叉点：partial 退 0，budget_exceeded 与 error 退 1。
func (s *askState) finish(status string) error {
	members := s.members
	if members == nil {
		members = []string{}
	}
	turns := s.turns
	if turns == nil {
		turns = []TurnJSON{}
	}
	// 席位数先算出来：它同时喂 JSON 的 failed/total 与下面 partial 的
	// stderr 行，两处必须逐字同源，不能让调用方自己从 turns 里数。
	failedN := len(s.failed)
	total := len(s.members)
	if s.summaryFailed {
		failedN++
		// 汇总成员通常不在发言名单里，分母跟着加一；若它本来
		// 就在名单里，只加失败数，不重复数一个人。
		if !containsID(s.members, s.summaryID) {
			total++
		}
	}
	res := Result{
		Status:         status,
		ConversationID: s.convID,
		Summary:        s.summary,
		Members:        members,
		Turns:          turns,
		CostUSD:        s.costUSD,
		Workspace:      s.workspace,
		Failed:         failedN,
		Total:          total,
	}
	gap := costGap(s.deps.Cfg, members)
	// 汇总成员也是这一趟花的钱（runSummary 把它的金额加进了 cost_usd），
	// 它不报金额时 cost_complete 同样必须是 false，否则正是 D5 要防的
	// 「把没报当 0 元」。它已经在名单里时不重复计数。
	if s.summaryAttempted && s.summaryID != "" && !containsID(members, s.summaryID) {
		gap += costGap(s.deps.Cfg, []string{s.summaryID})
	}
	res.CostComplete = len(members) > 0 && gap == 0

	var writeErr error
	if s.opts.JSON {
		writeErr = json.NewEncoder(s.out).Encode(res)
	} else {
		writeErr = writeText(s.out, res, gap)
	}
	if writeErr != nil {
		fmt.Fprintf(s.errOut, "council ask: 写结果失败: %v\n", writeErr)
		return writeErr
	}

	switch status {
	case statusPartial:
		// D6 的冻结格式：有结论就算成功，但失败信息不能吞。
		fmt.Fprintf(s.errOut, "council: partial: %d/%d 成员失败\n", failedN, total)
		return nil
	case statusBudgetExceeded:
		fmt.Fprintf(s.errOut, "council: 预算触线：已花 %.4f USD，上限 %.2f USD\n", s.costUSD, s.opts.BudgetUSD)
		return fmt.Errorf("预算触线：已花 %.4f USD，上限 %.2f USD", s.costUSD, s.opts.BudgetUSD)
	case statusError:
		if s.execErr == nil {
			s.execErr = errors.New("执行出错")
		}
		fmt.Fprintf(s.errOut, "council ask: %v\n", s.execErr)
		return s.execErr
	}
	return nil
}

// writeText 是 -json 为假时的输出。它只写结果，不写横幅、不写提示符；
// 调用方（dsh、脚本、人）拿到的仍然是「一段可以整段读完的东西」。
func writeText(out io.Writer, res Result, costGap int) error {
	amount := fmt.Sprintf("%.4f", res.CostUSD)
	if costGap > 0 {
		amount += fmt.Sprintf("（不完全：%d 位成员不报金额）", costGap)
	}
	if _, err := fmt.Fprintf(out, "状态: %s\n会话: %s\n成员: %s\n金额: %s USD\n",
		res.Status, res.ConversationID, strings.Join(res.Members, ", "), amount); err != nil {
		return err
	}
	for _, t := range res.Turns {
		if _, err := fmt.Fprintf(out, "\n%s 第 %d 轮 %s（%s/%s，%dms，%.4f USD）\n",
			t.AgentID, t.Round, t.Outcome, t.Model, t.Effort, t.DurationMS, t.CostUSD); err != nil {
			return err
		}
		if t.Text == "" {
			if _, err := fmt.Fprintln(out, "（无正文）"); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintln(out, t.Text); err != nil {
			return err
		}
	}
	if res.Workspace != nil {
		files := strings.Join(res.Workspace.Files, ", ")
		if files == "" {
			files = "（无）"
		}
		if _, err := fmt.Fprintf(out, "\n工作区副本: %s\n改动文件: %s\n", res.Workspace.Copy, files); err != nil {
			return err
		}
		if res.Workspace.DiffTruncated {
			if _, err := fmt.Fprintf(out, "diff 超过 1 MiB，全文: %s\n", res.Workspace.DiffPath); err != nil {
				return err
			}
		} else if res.Workspace.Diff != "" {
			if _, err := fmt.Fprintf(out, "改动:\n%s\n", res.Workspace.Diff); err != nil {
				return err
			}
		}
	}
	if res.Summary != "" {
		if _, err := fmt.Fprintf(out, "\n汇总:\n%s\n", res.Summary); err != nil {
			return err
		}
	}
	return nil
}

func turnJSON(cfg *config.Config, rec turn.Record, round int) TurnJSON {
	model, effort := "", ""
	if cfg != nil {
		if ac, ok := cfg.Agents[rec.AgentID]; ok {
			model, effort = ac.Model, ac.Effort
		}
	}
	text := ""
	if rec.Message != nil {
		text = rec.Message.Text
	} else if rec.DroppedText != "" {
		// 付过钱、没写进库的正文不能因为 status=partial 就从结果里消失。
		text = rec.DroppedText
	}
	cacheCreate := 0
	if rec.Usage.CacheCreate != nil {
		cacheCreate = *rec.Usage.CacheCreate
	}
	return TurnJSON{
		AgentID: rec.AgentID,
		Model:   model,
		Effort:  effort,
		Round:   round,
		Outcome: outcomeName(rec.Outcome),
		Text:    text,
		Usage: UsageJSON{
			In:          rec.Usage.InputTokens,
			CacheCreate: cacheCreate,
			CacheRead:   rec.Usage.CacheRead,
			Out:         rec.Usage.OutputTokens,
		},
		CostUSD:    costOf(rec),
		DurationMS: rec.WallMS,
	}
}

// costGap 数名单里有多少位成员的 provider 不报金额。grok / agy / codex
// 都是 ReportsCost=SupportNo，它们的轮次 cost_usd 只能是 0，这个 0 是
// 「没有这个数」，不是「花了 0 元」。这个函数只做标注，不拒绝执行。
func costGap(cfg *config.Config, members []string) int {
	gap := 0
	for _, id := range members {
		if cfg == nil {
			gap++
			continue
		}
		ac, ok := cfg.Agents[id]
		if !ok {
			gap++
			continue
		}
		p, err := provider.Lookup(ac.Provider)
		if err != nil || p.Capabilities().ReportsCost != provider.SupportYes {
			gap++
		}
	}
	return gap
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// costOf 把可空的单轮金额折成结果里的数字。nil 读作「这一轮没报金额」，
// 结果里没有 null 这一档，只能写 0；总额因此是「报过金额的那些轮」之和。
func costOf(rec turn.Record) float64 {
	if rec.CostUSDDelta == nil {
		return 0
	}
	return *rec.CostUSDDelta
}

// outcomeName 是 JSON 里的机器词。屏幕上的中文词在 internal/repl，
// 这里不 import 它：headless 的 JSON 不应随屏幕文案改动。
func outcomeName(o turn.Outcome) string {
	switch o {
	case turn.OutcomeSpoke:
		return "ok"
	case turn.OutcomePostRejected:
		return "post_rejected"
	case turn.OutcomeProviderFailed:
		return "provider_failed"
	case turn.OutcomeUnparsable:
		return "unparsable"
	case turn.OutcomeTimeout:
		return "timeout"
	case turn.OutcomeSpawnFailed:
		return "spawn_failed"
	case turn.OutcomeRefused:
		return "refused"
	case turn.OutcomeCanceled:
		return "canceled"
	case turn.OutcomeLost:
		return "lost"
	default:
		return "unknown"
	}
}
