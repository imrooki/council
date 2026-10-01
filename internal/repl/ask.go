package repl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"council/internal/conversation"
	"council/internal/turn"
)

// cmdAsk 让当前会话里的一个 agent 说一轮话。
//
// 用法: /ask <agent-id> [文本]
//
// 不给 agent-id 只打用法、返回 nil，和 /use 一样：缺参数不是 REPL 故障，
// 用户看一眼就能补上。带文本时先经 postUserText 落一条 user 消息，再调 Ask。
// 必须复用 postUserText，不许另写一份 Post：/say、裸文本、多行草稿、/ask 带文本
// 四条路发出去的消息形状必须一模一样（ConvID / SenderID / Kind / 空脑子列），
// 分两份写早晚会走失同步，Canonical Conversation 只追加不回收，形状一旦分叉
// 就永远分叉。
//
// 正文取 restOf(args.rest)，不是 strings.Fields 再 Join。args.rest 是命令名之后
// 的原始文本，第一个词是 agent-id，restOf 从第一个空白切开并只削首尾空白，
// 正文中间的连续空白原样保留。Fields 会把它们压成一个，等于改用户写下的内容。
//
// postUserText 失败就不能接着调 Ask。它把 Post 的领域失败打成一行提示后返回
// nil（免得打死 REPL），返回非 nil 只可能是写屏幕失败。两种都不能往下走：
// 写屏幕都失败了更没必要起进程；领域失败则用户那句话没落库，这一轮送给
// agent 的 delta 里就没有它，等于花钱回答一个用户以为没发出去的问题。
// 所以落库前后各问一次 MaxSeq，没涨就停。REPL 是单线程读循环，没有别人
// 在这中间 Post。
//
// Ask 返回 error 时 Record 仍然要打：Outcome / Usage / ProviderSession /
// MaybeAdvanced 都已填好，调用方据此决定屏幕上说什么。只打一行错误会把
// 付过钱的用量、被拒的正文、超时是否可能已经吃进 CLI 会话这些要点一起丢掉。
// 一轮调用失败是正常业务结果，打出来即可，cmdAsk 返回 error 只用于写屏幕
// 失败这类 I/O 错误。
//
// agent-id 可以是全名，也可以是一个唯一前缀（/ask gr 问 grok-4.7）；解析在
// 成员闸之前做，理由见函数体里的注释。
func cmdAsk(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) == 0 {
		return r.printf("用法: /ask <agent-id> [文本]\n")
	}
	ref := args.fields[0]
	text := restOf(args.rest)

	// 成员闸必须在 postUserText、「正在问」和 Ask 之前。非成员没有 provider
	// 会话时会被当成 Fresh，selectBootstrap 会拿全量历史起进程；钱花掉后，
	// RecordTurn 才会因 RequireActiveParticipant 拒绝，落成 PostRejected，
	// 也就是「付过钱、正文写不进去」。带文本时若先 postUserText，给不在场
	// agent 的话还会永久留在只追加的 Canonical Conversation 里。
	//
	// 闸放在 REPL 层，与 /round 先用 PlanRound 按成员名单过滤保持一致。
	// 不放进 Runner.Ask，是为了避免 /round 已读过名单后，每个成员再各读一次
	// Participants。代价是 Runner.Ask 本身仍然没有这道闸，绕过 REPL 直接
	// 调用它的调用方仍可能先花钱、再在落库时被拒。
	//
	// includeInactive 必须传 true，才能把已经 /leave 的成员与从未加入过的
	// agent 分开；传 false 会把「退出过」误报成「还没加入」。
	participants, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return r.reportErr("列出成员失败", err)
	}

	// 先把引用解析成全名，再过闸。两件事：前缀解析只决定「问谁」，下面的闸
	// 决定「能不能问」，所以有了前缀解析，闸仍然需要。
	//
	// 全集是「成员（含已退出）∪ 配置」，先在全集上判精确，命中就不做前缀。
	// 不这样会怎样（W1 用真实配置实测）：配置里有 opus-5-0 与 opus-5-0-max、
	// 只有后者在场，/ask opus-5-0 若先在成员里做前缀，会问成 opus-5-0-max，
	// 起进程、花一次入场费。先判精确也保证了敲全名时的行为与加前缀之前逐字节相同。
	// 全集里「成员」那一半同样不能少（W1b）：一个已从配置删掉、但仍是成员的 id
	// （configcmd.go 的 goneButStillMembers 专门报告这种状态）若不在全集里，敲它的
	// 全名会落进前缀解析，被以它为前缀的配置 agent 偷走 —— 配置只剩 opus-5-0-max 时，
	// /ask opus-5-0 会问成 max、花钱。
	//
	// 前缀也在同一个全集上判歧义，不只在成员里判：/ask 花钱，而用户多半不记得
	// 谁在场。只在成员里判的话，opus-5-0-max 在场时 /ask opus-5 会直接问它，
	// 哪怕用户想的是还没加入的 opus-5-0。
	// 代价：按 <模型>-<强度> 的命名约定，/ask op、/ask fa 这类前缀今后总是报
	// 歧义，只有唯一的家族前缀（/ask gr）能省事。这是有意的 —— 宁可让用户多敲
	// 几个字，也不替他挑一个要花钱的 agent。
	memberIDs := make([]string, 0, len(participants))
	for _, p := range participants {
		memberIDs = append(memberIDs, p.AgentID)
	}
	universe := unionIDs(memberIDs, sortedKeys(r.cfg.Agents))
	agentID := ref
	exact := false
	for _, id := range universe {
		if id == ref {
			exact = true
			break
		}
	}
	if !exact {
		resolved, rerr := resolveAgentRef(ref, universe)
		if rerr != nil {
			var refErr *agentRefError
			if errors.As(rerr, &refErr) && len(refErr.Matches) >= 2 {
				return r.printf("%s 有歧义，可能是：%s\n", ref, strings.Join(refErr.Matches, ", "))
			}
			return r.printf("配置里没有 %s 这个 agent，用 /agents 看看有哪些\n", ref)
		}
		agentID = resolved
	}

	found := false
	for _, p := range participants {
		if p.AgentID != agentID {
			continue
		}
		found = true
		if !p.Active {
			return r.printf("%s 已退出当前会话，用 /join %s 重新加入\n", agentID, agentID)
		}
		break
	}
	if !found {
		if _, ok := r.cfg.Agents[agentID]; ok {
			return r.printf("%s 在配置里但还没加入当前会话，用 /join %s 加入\n", agentID, agentID)
		}
		return r.printf("配置里没有 %s 这个 agent，用 /agents 看看有哪些\n", agentID)
	}

	if text != "" {
		before, err := r.svc.MaxSeq(ctx, r.cur.ID)
		if err != nil {
			return r.reportErr("读会话进度失败", err)
		}
		if err := postUserText(ctx, r, text); err != nil {
			return err
		}
		after, err := r.svc.MaxSeq(ctx, r.cur.ID)
		if err != nil {
			return r.reportErr("读会话进度失败", err)
		}
		if after <= before {
			return nil
		}
	}

	// 这一行必须打在 Ask 之前，晚一步就等于没有：Ask 里那一次 spawn 的
	// 硬超时是 5 分钟，实测一轮 wall_ms=11646 全程零输出，用户的原话是
	// 「我问了，我也不知道它有没有反应」。行里要带 agent id，否则
	// /round 那边分不清是谁。
	//
	// 取舍：Ask 仍有一批「起进程之前就拒绝」的路径（agent 已从配置删掉但仍是
	// 会话成员、权限档不放行、没有 delta、铸 round id 失败），那些情形下这一行
	// 会先打出来、紧接着是一行拒绝。这是接受的，不要为了避免它把这一行挪进
	// Ask —— 挪进去就得让 turn 包知道怎么往屏幕打字，而那个包不持有任何输出流
	// （它的「响亮」一直是指写进只追加的事实源）。
	//
	// 「agent 不在配置里」这一档：上面那道成员闸只拦住**从没加入过**的 agent。
	// 闸只看成员名单、不看配置，所以一个**已从配置删掉、仍是会话成员**的 agent
	// 照样走到这里，先打「正在问 X」，再由 Ask 在起进程之前以「配置里没有」拒绝
	// （不花钱）。2026-09-23 真机实测过：会话里遗留的成员 op，/ask op 正是这样。
	// X-1 落地时这里曾写成「这一档不再会出现」，那句话只对从没加入过的 agent 成立，
	// 已订正。绕过 REPL 直接调 Runner.Ask 的调用方两种都会命中。
	if err := r.printf("正在问 %s ...\n", agentID); err != nil {
		return err
	}
	rec, askErr := r.rn.Ask(ctx, r.cur.ID, agentID)
	if err := writeAskRecord(r, rec); err != nil {
		return err
	}
	if askErr != nil {
		return r.printf("%s\n", askCause(askErr))
	}
	return nil
}

// cmdSession 列出当前会话每个活跃成员的 provider 会话状态；带
// `rebuild <agentID>` 子命令时改成丢弃并重建那个 agent 的 provider 会话
// （11a 契约 5.3）。不带参数的行为一个字不改，仍是下面的列表。
//
// 只问活跃成员（Participants 的 includeInactive=false）：已 /leave 的人
// 不再会被 /ask，他们的 CLI 会话对「下一轮冷还是暖」没有用。
// ok 为 false 表示这个 agent 还没开过口、下一轮 Ask 会是 Fresh ——
// 要明确显示成 fresh，不要显示成空会话 id 了事，空 id 看起来像「有会话
// 但 id 丢了」，和「根本还没起过进程」不是一回事。
//
// 排版与 /who、/log 相同：newTabWriter、ASCII 表头、可能变长的字段放
// 后面；返回顺序不在契约里，按 agent id 排一遍，输出才稳定。
// BASELINE 一列回答「下一轮的金额会不会报『不知道』」，它不回答「下一轮会不会被拒」。
func cmdSession(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) > 0 {
		return cmdSessionRebuild(ctx, r, args.fields)
	}
	ps, err := r.svc.Participants(ctx, r.cur.ID, false)
	if err != nil {
		return r.reportErr("列出成员失败", err)
	}
	if len(ps) == 0 {
		return r.printf("会话 %s 没有活跃成员，用 /join <agent-id> 加一个\n", r.cur.ID)
	}

	sort.Slice(ps, func(i, j int) bool { return ps[i].AgentID < ps[j].AgentID })

	tw := r.newTabWriter()
	fmt.Fprintln(tw, "AGENT\tPROVIDER_SESSION\tLAST_TURN_AT\tBASELINE\tNEXT")
	for _, p := range ps {
		info, ok := r.rn.SessionInfo(ctx, r.cur.ID, p.AgentID)
		if !ok {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.AgentID, emptyCell, emptyCell, emptyCell, "fresh")
			continue
		}
		// InFlight 优先于 cold / warm：在飞时 ProviderSession 与 LastTurnAt
		// 都是从库里读回的**上一次提交**的值，ColdExpected 是拿那个旧
		// LastTurnAt 算出来的。把它印成 cold 或 warm 等于用上一轮的时刻
		// 预报下一轮的冷暖，而这一轮此刻正在跑、跑完就会把 LastTurnAt
		// 推到现在。印 busy 是照实说「这一列现在答不了」，不是多一个
		// 装饰状态。
		//
		// 闸拆开之后结构上允许并发调用；片 3b 的 RunRound、片 3c 的
		// RunRoundWith 已经在 internal/turn/scheduler.go 落地，是真的
		// 并发起进程，不再只是结构上允许，InFlight 因此有了会触发它的
		// 调用方（session_inflight_test.go / gate_concurrency_test.go
		// 从第二条 goroutine 观测到 busy）。/round 这条 REPL 命令本身
		// 仍是串行 Ask，尚未接上 RunRoundWith（片 3d），所以今天的
		// 交互式 REPL 里还看不到这一列变 busy；这个字段先加上消费方，
		// 免得片 3d 落地那天它还是个死键。
		next := "warm"
		if info.InFlight {
			next = "busy"
		} else if info.ColdExpected {
			next = "cold"
		}
		baseline := "ok"
		if info.BaselineSuspect {
			baseline = "suspect"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			p.AgentID,
			orPlaceholder(info.ProviderSession),
			formatTime(info.LastTurnAt),
			baseline,
			next,
		)
	}
	return tw.Flush()
}

// cmdSessionRebuild 是 `/session rebuild <agentID>` 的实现。
//
// 两条放行路径，id 一律字符串全等、不做前缀解析、不做大小写折叠：
// 一是 agentID 在当前会话的活跃成员里；二是它不是成员，但本会话的漂移
// 名单里有同一 id 且 Kind 恰好是 DriftAgentGone（配置里已无此 agent 的悬空行）。
// 放宽只认第 1 类：第 2、3、4 类的非成员往往是一时写错配置，改回去还能 resume，
// 不在这里删行。拼错 id、前缀、别的会话里的悬空行，仍然报错、不动任何东西。
// 审计失败时不放行也不重建：分不出类别就既不能放宽闸，也选不出不说假话的文案。
//
// 放行之后、Rebuild 之前，先经 Runner.SettleStaleInflight 结清这个成员在当前会话里
// 陈旧的在飞记录（#43(a)）：被 A 之五的闸拒绝之后，这是不重启就能解开的那条路。
// 结清走启动对账的同一个函数，只动这一 (会话, 成员)；/rules reload --rebuild
// 不经这里，不会替用户结清任何一行。
func cmdSessionRebuild(ctx context.Context, r *REPL, fields []string) error {
	if len(fields) != 2 || fields[0] != "rebuild" {
		return r.printf("用法: /session 或 /session rebuild <agent-id>\n")
	}
	agentID := fields[1]

	active, err := r.svc.Participants(ctx, r.cur.ID, false)
	if err != nil {
		return r.reportErr("列出成员失败", err)
	}
	found := false
	for _, p := range active {
		if p.AgentID == agentID {
			found = true
			break
		}
	}

	drift, auditErr := r.rn.AuditDrift(ctx)
	if auditErr != nil {
		return r.reportErr("身份漂移审计失败", auditErr)
	}
	var row turn.DriftRow
	rowFound := false
	for _, d := range drift {
		if d.ConvID == r.cur.ID && d.AgentID == agentID {
			row = d
			rowFound = true
			break
		}
	}
	if !(found || (rowFound && row.Kind == turn.DriftAgentGone)) {
		return r.printf("%s 不在当前会话的活跃成员里，用 /who 看看有哪些\n", agentID)
	}

	// 先结清陈旧的在飞记录，再丢 provider 会话，顺序不能反。一是 fail closed：
	// 结清失败或读不出来就停，不调 Rebuild，什么都不改、provider 会话也不丢，
	// 这个成员照样被 /ask 拒绝，用户看到的是原因。二是结清会整行读回
	// provider_sessions 标可疑，两次取闸之间 Rebuild 若被拒，这个标记留得住。
	// 没有在飞记录时一个字不打，往下与本刀之前逐字节相同。
	stale, err := r.rn.SettleStaleInflight(ctx, r.cur.ID, agentID)
	if err != nil {
		if errors.Is(err, turn.ErrTurnInFlight) {
			return r.printf("%s 此刻正有一轮在跑，什么都没丢；等这一轮跑完再敲一次\n", agentID)
		}
		if stale.Found {
			return r.printf("结清 %s 的在飞记录（turn %s）失败，什么都没改，provider 会话也没丢；对它 /ask 仍会被拒: %v\n", agentID, stale.Lost.Turn.TurnID, err)
		}
		return r.printf("读不出 %s 的在飞记录，什么都没改，provider 会话也没丢: %v\n", agentID, err)
	}
	if stale.Found {
		if err := r.printf("已结清 %s 的一轮在飞记录（turn %s，phase %s），对它 /ask 不会再因为这条记录被拒\n", agentID, stale.Lost.Turn.TurnID, stale.Lost.Turn.Phase); err != nil {
			return err
		}
		consequence := "那一轮记为丢失：可能花了钱，用量与金额都不知道（与重启时启动对账的记法相同）\n"
		if stale.Lost.Turn.Phase == conversation.PhaseBegun {
			consequence = "那一轮没起过进程、没花钱，只删掉了这条记录\n"
		}
		if err := r.write(consequence); err != nil {
			return err
		}
	}

	existed, err := r.rn.Rebuild(ctx, r.cur.ID, agentID)
	if err != nil {
		// 拿不到闸就拒绝，不等。即便等到了也白做：在飞那一轮结束时
		// SetProviderSession 是 upsert，会把刚删的行又写回来，用户看到
		// 「已丢弃」而其实什么都没丢。拒绝是 fail-closed 的那一边。
		// Rebuild 拿不到闸时什么都不做，尤其没有先删库再发现拿不到。
		if errors.Is(err, turn.ErrTurnInFlight) {
			return r.printf("%s 此刻正有一轮在跑，什么都没丢；等这一轮跑完再敲一次\n", agentID)
		}
		return r.reportErr("重建失败", err)
	}
	if !existed {
		return r.printf("本来就没有开着的会话，下一轮照常是首轮\n")
	}
	if rowFound && row.AskRefused() {
		return r.printf("已清掉 %s 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢）；按当前配置对它 /ask 会被拒，不花钱\n", agentID)
	}
	return r.printf("已丢弃 %s 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢），下一轮按首轮计价\n", agentID)
}

// writeAskRecord 渲染发言轮与裁判轮的一条 Record，也是 /ask 的渲染：/ask 问总结成员
// 同样走它，简洁档打的是成员头，不是总结头。summary 恒为 false。
func writeAskRecord(r *REPL, rec turn.Record) error {
	return writeRecord(r, rec, false)
}

// writeSummaryRecord 渲染总结段的一条 Record。summary 为 true，只改简洁档的两个头。
func writeSummaryRecord(r *REPL, rec turn.Record) error {
	return writeRecord(r, rec, true)
}

// writeRecord 把一轮 Record 的要点打到屏幕上。
//
// 即使 Ask 返回了 error 也要走这里。OutcomePostRejected 的 DroppedText
// 是付过钱、写不进库的那段正文，必须整段打出来，不许 oneLine、不许截断、
// 不许无声蒸发 —— Canonical Conversation 记不上它，屏幕是唯一还拿得回
// 这段文字的地方。MaybeAdvanced 为真时（超时被杀）要明确告诉用户
// provider 那边可能已经吃下这批内容：Windows 上杀掉的往往只是 .cmd 垫片，
// 真正干活的 node.exe 会活着把这一轮写进 CLI 自己的会话文件。
//
// 格式只有一份：简洁档是这个函数里的分支，不是另一个函数。
// 简洁档的成员块：Message 非 nil（这一轮的发言已落库）时打「名（用时）: 正文」，
// 正文为空则冒号后直接换行；本轮为空 / 本轮被截断在正文之后另起一行。
// Message 为 nil 时打「名: 没有回复（结局词）」，不带用时：起进程之前被拒的轮 WallMS 是 0，印成（0.0s）是假话。
// 判据是 Message 非 nil，不是 Outcome，以免 Message 为 nil 时解引用。
// 简洁档照打关系到钱或内容丢失的几类（#45）：MaybeAdvanced、漂移、被拒工具调用、PostRejected 的整段正文。
// 简洁档不打：outcome 行、两种 provider 会话行、seq 行、writeAskMeter 的全部行。
//
// summary 只改简洁档的两个头：#45 规定的「总的来看（<名> 做总结）：」与
// 「总的来看：没有总结（<结局>）」。其余照打的行（可能已吃下、漂移、被拒工具调用、
// 正文写不进库整段）与成员块同一份代码。详细档不看 summary：总结段照裁判轮的详细格式打。
// 用一个参数而不是另写一个函数：round.go 写过，两份格式会分叉，而分叉的那一天没有任何测试会红。
// 总结头用全角冒号、不带用时：#45 的原话是「总的来看（Fable 5.1 做总结）：…」，原话里没有用时。
func writeRecord(r *REPL, rec turn.Record, summary bool) error {
	if r.concise {
		name := r.displayName(rec.AgentID)
		if rec.Message != nil {
			head := name + "（" + FormatWall(rec.WallMS) + "）:"
			sep := " "
			if summary {
				head = "总的来看（" + name + " 做总结）："
				sep = ""
			}
			if rec.Message.Text == "" {
				if err := r.write(head + "\n"); err != nil {
					return err
				}
			} else {
				if err := r.write(head + sep); err != nil {
					return err
				}
				if err := writeFullText(r, rec.Message.Text); err != nil {
					return err
				}
			}
			if note := completionNote(rec); note != "" {
				if err := r.printf("%s\n", note); err != nil {
					return err
				}
			}
		} else {
			line := name + ": 没有回复（" + OutcomeWord(rec.Outcome) + "）\n"
			if summary {
				line = "总的来看：没有总结（" + OutcomeWord(rec.Outcome) + "）\n"
			}
			if err := r.write(line); err != nil {
				return err
			}
		}
	} else if err := r.printf("outcome %s  agent %s\n", outcomeLabel(rec.Outcome), rec.AgentID); err != nil {
		return err
	}
	if rec.MaybeAdvanced {
		if err := r.printf("provider 那边可能已经吃下这批内容\n"); err != nil {
			return err
		}
	}
	if !r.concise {
		if rec.Fresh {
			if err := r.printf("新开了一条 provider 会话 %s\n", orPlaceholder(rec.ProviderSession)); err != nil {
				return err
			}
		} else if rec.ProviderSession != "" {
			if err := r.printf("provider 会话 %s\n", rec.ProviderSession); err != nil {
				return err
			}
		}
	}
	if rec.Drifted {
		if err := r.printf("送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了\n"); err != nil {
			return err
		}
	}
	// 放在 rec.Message 判断之外：turn_failed / post_rejected 这两条路径也可能
	// 带着非空的 DeniedActions（absorbParse 先于失败判定运行），不能只在
	// 「说成功了话」这一支才把这件事说出来。
	if note := deniedActionsNote(rec); note != "" {
		if err := r.printf("%s\n", note); err != nil {
			return err
		}
	}
	if rec.Message != nil && !r.concise {
		m := rec.Message
		if err := r.printf("seq %d  %s  %s\n", m.Seq, m.SenderID, brainCell(m.SenderProvider, m.SenderModel, m.SenderEffort)); err != nil {
			return err
		}
		if note := completionNote(rec); note != "" {
			if err := r.printf("%s\n", note); err != nil {
				return err
			}
		}
		if err := writeFullText(r, m.Text); err != nil {
			return err
		}
	}
	if rec.Outcome == turn.OutcomePostRejected {
		if err := r.printf("正文写不进库（已经付过钱），整段如下:\n"); err != nil {
			return err
		}
		if err := writeFullText(r, rec.DroppedText); err != nil {
			return err
		}
	}
	return writeAskMeter(r, rec)
}

// writeAskMeter 打印用量、金额、冷暖、耗时。
// OutcomeRefused / OutcomeUnknown 是起进程之前就停的，没有用量可报，
// 打「金额未报」会让人以为花了钱但 provider 没给数字。其余结局都可能
// 已经花过钱：解析到的用量照打，零值也照打；库里为 NULL 的格子（provider 标了未报，
// 或 CacheCreate 本来就是 nil）印 ?，判据是 Record.ReportedUsage。没解析到用量（UsageParsed 为假）
// 就说未知，不把没填的零印成实测为零。这一行的门控只看 UsageParsed，
// 不看结局、不看数字是不是零。金额为 nil 时绝不猜一个数。
// 冷暖的「实测」只经 rec.MeasuredCold() 判：Record.Cold 只在成功路径上赋值，
// 其余结局上是没赋过的零值，cache_read 标了未报时也没有含义，这些轮印未知。
// 意外冷启动行用同一次判定的 known 门控，两行的判据写在同一处。
// 简洁档一行都不打（用量、金额、冷暖、wall_ms、意外冷启动；金额整类不显示，#45）；子进程用时改由成员头带出。
func writeAskMeter(r *REPL, rec turn.Record) error {
	if r.concise {
		return nil
	}
	switch rec.Outcome {
	case turn.OutcomeRefused, turn.OutcomeUnknown:
		return nil
	}

	if rec.UsageParsed {
		ru := rec.ReportedUsage()
		cell := func(v *int) string {
			if v == nil {
				return "?"
			}
			return fmt.Sprintf("%d", *v)
		}
		if err := r.printf("用量 in=%s cache_create=%s cache_read=%s out=%s thinking=%s\n",
			cell(ru.Input),
			cell(ru.CacheCreate),
			cell(ru.CacheRead),
			cell(ru.Output),
			cell(ru.Thinking),
		); err != nil {
			return err
		}
	} else {
		if err := r.printf("用量 未知（没解析到用量）\n"); err != nil {
			return err
		}
	}

	switch {
	case rec.CostUSDDelta != nil:
		if err := r.printf("金额 %g USD\n", *rec.CostUSDDelta); err != nil {
			return err
		}
	case rec.BaselineSuspect:
		if err := r.printf("金额未计（上一轮结局未知，基线可疑）\n"); err != nil {
			return err
		}
	default:
		if err := r.printf("金额未报\n"); err != nil {
			return err
		}
	}

	forecast := "暖"
	if rec.ColdExpected {
		forecast = "冷"
	}
	cold, known := rec.MeasuredCold()
	actual := "未知"
	if known {
		actual = "暖"
		if cold {
			actual = "冷"
		}
	}
	if err := r.printf("冷暖 预报=%s 实测=%s  wall_ms=%d  prompt_bytes=%d  scanned_to=%d\n",
		forecast, actual, rec.WallMS, rec.PromptBytes, rec.ScannedTo); err != nil {
		return err
	}
	if rec.UnexpectedCold && known {
		if err := r.printf("意外冷启动：预报是暖的，实测 cache_read=0\n"); err != nil {
			return err
		}
	}
	return nil
}

// writeFullText 把一段正文原样写到屏幕上。
// 不用 oneLine：那是表格最后一列才需要的压缩，这里要的是付过钱的原文字，
// 换行、缩进都是正文的一部分。不用 printf(s) 直接当格式串：用户或模型
// 写下的百分号不是占位符。
func writeFullText(r *REPL, s string) error {
	if s == "" {
		return nil
	}
	if err := r.write(s); err != nil {
		return err
	}
	if strings.HasSuffix(s, "\n") {
		return nil
	}
	return r.write("\n")
}

// deniedActionsNote 把 Record.DeniedActions 翻成给人看的一句话。
//
// 只有 agy 今天会填这个字段（provider.DeniedAction 的注释）。空正文配上这
// 一行，「本轮为空」就从「不明原因」变成「有 N 个工具调用被拒，分别是谁」——
// 这正是本片要修的那个洞（docs/11c_STEP8B_WORKSPACE_CONTRACT.md 第 7 节）。
// 刻意不说「这一轮失败了」：agy 越界写被拒的同时，界内那件事很可能已经
// 做完，只是模型的汇报被清空，把它说成失败是另一种谎言（同样来自那一节）。
func deniedActionsNote(rec turn.Record) string {
	if len(rec.DeniedActions) == 0 {
		return ""
	}
	names := make([]string, 0, len(rec.DeniedActions))
	for _, d := range rec.DeniedActions {
		names = append(names, fmt.Sprintf("%s(%s)", d.Action, d.DisplayName))
	}
	// 「provider 报了」这四个字不是客气话，是实测出来的限定：一条要求做
	// 1 件界内 + 2 件越界的指令跑完之后，两个越界文件一个都没落盘，而
	// denied_actions 只有 1 条（原件 oq-agy-denied-count/RESULTS.md，
	// 11c 第 7.1 节）。这个长度是**下界**，不是被拒次数的清点 ——
	// 可能是 agy 首次被拒就停下这一轮，也可能是它按 action 类型去重，
	// 本次探针分辨不出来。所以措辞不许写成「一共有 N 个」。
	return fmt.Sprintf(
		"provider 报了 %d 个被拒的工具调用（可能不止这些）：%s。界内的事可能已经做完，"+
			"只是这一轮的正文被清空了，不代表整轮失败",
		len(rec.DeniedActions), strings.Join(names, "、"),
	)
}

func completionNote(rec turn.Record) string {
	if rec.Message == nil {
		return ""
	}
	if rec.Message.Text == "" {
		return "本轮为空"
	}
	if rec.Message.Meta == nil {
		return ""
	}
	v, _ := rec.Message.Meta[turn.MetaCompletion].(string)
	if v == "truncated" {
		return "本轮被截断"
	}
	if v == "empty" {
		return "本轮为空"
	}
	return ""
}

func outcomeLabel(o turn.Outcome) string {
	switch o {
	case turn.OutcomeSpoke:
		return "spoke"
	case turn.OutcomePostRejected:
		return "post_rejected"
	case turn.OutcomeProviderFailed:
		return "provider_failed"
	case turn.OutcomeUnparsable:
		return "unparsable"
	case turn.OutcomeTimeout:
		return "timeout"
	case turn.OutcomeCanceled:
		return "canceled"
	case turn.OutcomeSpawnFailed:
		return "spawn_failed"
	case turn.OutcomeRefused:
		return "refused"
	default:
		return "unknown"
	}
}

// askCause 把 Ask 的错误翻成给人看的一句。
// 哨兵用 errors.Is 认，不匹配 Error() 字符串。ErrNoDelta 是「这个 agent
// 没有新内容可读」，不是一条刺眼的内部错误；其余哨兵同样给出状况而不是
// 堆栈。认得的哨兵还有 conversation.ErrInflightConflict。认不出哨兵时
// 原样打 Err —— 第 5-9 类结局的退出码 / stderr 头 /
// 可执行文件路径已经折进这个错误值的格式化消息里。
func askCause(err error) string {
	switch {
	// 两个产出方共用一个哨兵：起进程之前的检查，以及 BeginTurn 撞
	// (conv_id, agent_id) 唯一索引。/ask 与 /round 都经这一个函数，所以打的
	// 是同一句话。这句话只说重启（A 之五裁定的原句，本刀不改）；
	// /session rebuild <成员> 同样能解开：它先按启动对账的同一条路结清陈旧的
	// 在飞记录，再丢掉 provider 会话，下一轮付一次入场费。
	case errors.Is(err, conversation.ErrInflightConflict):
		return "这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行"
	case errors.Is(err, turn.ErrNoDelta):
		return "这个 agent 没有新内容可读"
	case errors.Is(err, turn.ErrAgentUnknown):
		return "配置里没有这个 agent，用 /agents 看看有哪些"
	case errors.Is(err, turn.ErrIdentityDrift):
		return "身份或脑子在这条 provider 会话开着的时候被改了；Step 6 不静默重建"
	case errors.Is(err, turn.ErrPrefixRewrite):
		return "这一批会改写已经送出去的历史"
	case errors.Is(err, turn.ErrTierNotOffered):
		return "full 权限档不提供给编排器自动使用（06 第 2 节）；拼错或未知的档同样被拒绝"
	default:
		return err.Error()
	}
}
