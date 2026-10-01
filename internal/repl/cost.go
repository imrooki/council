package repl

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"council/internal/conversation"
	"council/internal/turn"
)

// costRow 是一个成员在本次统计范围内的账。
//
// 屏幕上成栏的是只数适用行的 USAGE_UNKNOWN / COST_UNKNOWN / COLD_UNKNOWN：
// 适用行是 Outcome 不是 OutcomeRefused 的行。Refused 起进程之前就拒绝，
// 没有用量、没有金额、没有冷暖，它的 NULL 读作不适用，不进这三个计数。
// TokensUnknown / CostUnknown 两个字段保留、不再印：前者数「五列里至少一列 NULL」
// （含 Refused），grok 与 agy 每个成功轮的 cache_create 都是 NULL，印出来恒等于轮数；
// 后者把 Refused 的 NULL 金额也数进去。把「没报」折进合计，一张表会把没报的金额
// 显示成 0.00，比没有这张表更糟。
//
// 四个 token 列各自记「适用行里这一列非 NULL 的行数」，互相独立。
// 同一列在一部分适用行上有数、其余是 NULL 时，合计是部分和，屏幕不加标记，
// 旁边的 USAGE_UNKNOWN 只在五列全 NULL 时才加一。
type costRow struct {
	AgentID string

	Turns        int // 这个成员在本次统计范围内的全部轮数，含 lost / refused / 各类失败
	Spoke        int // Outcome 是 OutcomeSpoke 的轮数
	PostRejected int
	Lost         int // OutcomeLost，单独一栏
	Refused      int // OutcomeRefused，单独一栏
	OtherFailed  int // 其余既非 Ingested 也不是 lost / refused 的

	InputTokens    int64
	CacheRead      int64
	CacheCreate    int64
	OutputTokens   int64
	ThinkingTokens int64
	TokensUnknown  int // 至少有一个 token 列是 NULL 的轮数（含 Refused），不再印

	inReported     int // 适用行里 InputTokens 非 NULL 的行数
	readReported   int // 适用行里 CacheRead 非 NULL 的行数
	createReported int // 适用行里 CacheCreate 非 NULL 的行数
	outReported    int // 适用行里 OutputTokens 非 NULL 的行数

	CostUSD     float64
	CostKnown   int // CostUSD 非 nil 的轮数。Refused 的 CostUSD 恒为 NULL，所以它等于适用行里金额非 NULL 的行数
	CostUnknown int // CostUSD 为 nil 的轮数（含 Refused），不再印

	applicable               int // Outcome 不是 OutcomeRefused 的行数
	usageUnknown             int // 适用行里五个 token 列全是 NULL 的行数（含 ThinkingTokens）
	costUnreportedApplicable int // 适用行里 CostUSD 为 NULL 的行数；屏幕上的 COST_UNKNOWN
	coldKnown                int // 适用行里冷暖确知的行数：Ingested 且 CacheRead 非 NULL
	coldUnknown              int // 适用行里冷暖不确知的行数

	ColdStarts int // 只在「cold_start 有意义」的行上数，见 addTurn
}

// summarizeCost 是纯函数：不碰 IO，不排序输入，按 AgentID 升序返回。
//
// 空输入返回空切片不是 nil：调用方用 len==0 判断「还没有记过账」，
// nil 与空在这里没有第二种含义。
func summarizeCost(rows []conversation.TurnUsage) []costRow {
	byAgent := make(map[string]*costRow)
	order := make([]string, 0)
	for _, u := range rows {
		row := byAgent[u.AgentID]
		if row == nil {
			row = &costRow{AgentID: u.AgentID}
			byAgent[u.AgentID] = row
			order = append(order, u.AgentID)
		}
		row.addTurn(u)
	}
	sort.Strings(order)
	out := make([]costRow, 0, len(order))
	for _, id := range order {
		out = append(out, *byAgent[id])
	}
	return out
}

// addTurn 把一轮并进本行。四条裁决都在这里，缺一条就是把「不知道」写成一个数。
func (row *costRow) addTurn(u conversation.TurnUsage) {
	// 裁决 1：丢失轮计进 Turns，而且占 Lost 单独一栏。
	// 不单列、或者干脆不数它，用户会以为这条会话只跑了 N-1 轮，
	// 崩溃补上的那一行又一次从账上消失。
	row.Turns++
	switch turn.Outcome(u.Outcome) {
	case turn.OutcomeSpoke:
		row.Spoke++
	case turn.OutcomePostRejected:
		row.PostRejected++
	case turn.OutcomeLost:
		row.Lost++
	case turn.OutcomeRefused:
		row.Refused++
	default:
		// Ingested 只有 Spoke 与 PostRejected 两支，上面已经分走。
		// 落到这里的是超时、取消、起进程失败、解析失败，以及还没赋值的 0。
		// 不单开一栏：它们各自已经不是「没发生」，但本刀只要 lost / refused 分得开。
		row.OtherFailed++
	}

	// 裁决 2：OutcomeLost 的五个 token 与两个金额列全是 NULL，不进任何求和。
	// 它们本来就是 nil，下面的 nil 规则自动盖住，不必为 lost 再写一个「跳过求和」的分支。
	// 另写分支的人会以为 lost 的指针可能不是 nil，然后把某个零值加进去。

	// 裁决 3：任何 *int / *float64 为 nil 时，绝不当成 0 加进去。
	// 这正是 computeCostDelta 一路在防的那个谎：把「这家没报」写成「实测为零」，
	// 合计看起来很精确，其实是编的。nil 不进任何求和，只进各自的未知计数。
	// 部分已知仍进和：那几个非 nil 的数是真实测量值，不为「有缺口」另造一档把整行丢掉。
	// Refused 的 NULL 不进「报了的轮数」与屏幕上的未知计数：它没起进程，不适用。
	applicable := turn.Outcome(u.Outcome) != turn.OutcomeRefused
	if applicable {
		row.applicable++
	}
	unknown := false
	if u.InputTokens == nil {
		unknown = true
	} else {
		row.InputTokens += int64(*u.InputTokens)
		if applicable {
			row.inReported++
		}
	}
	if u.CacheRead == nil {
		unknown = true
	} else {
		row.CacheRead += int64(*u.CacheRead)
		if applicable {
			row.readReported++
		}
	}
	if u.CacheCreate == nil {
		unknown = true
	} else {
		row.CacheCreate += int64(*u.CacheCreate)
		if applicable {
			row.createReported++
		}
	}
	if u.OutputTokens == nil {
		unknown = true
	} else {
		row.OutputTokens += int64(*u.OutputTokens)
		if applicable {
			row.outReported++
		}
	}
	if u.ThinkingTokens == nil {
		unknown = true
	} else {
		row.ThinkingTokens += int64(*u.ThinkingTokens)
	}
	if unknown {
		row.TokensUnknown++
	}
	// 五个 token 列全 NULL 才算「这一轮一个用量数都没有」。只看 token，不看金额。
	if applicable && u.InputTokens == nil && u.CacheRead == nil && u.CacheCreate == nil && u.OutputTokens == nil && u.ThinkingTokens == nil {
		row.usageUnknown++
	}
	if u.CostUSD == nil {
		row.CostUnknown++
		if applicable {
			row.costUnreportedApplicable++
		}
	} else {
		row.CostKnown++
		row.CostUSD += *u.CostUSD
	}

	// 裁决 4：ColdStarts 只在 cold_start 有意义的行上数。
	// 判据是 Outcome：只有 Spoke 与 PostRejected（Ingested 为真）的行，这一列才是实测。
	// 其余行上那个 false 读作「不知道冷暖」，不是「暖」。
	// 列是 cold_start INTEGER NOT NULL DEFAULT 0：一轮根本没跑完，冷暖本来不知道，
	// 而列不可空，false 只能存成 0。这条纪律只能由读的这一侧承担。
	// 这是 00_DECISIONS.md 未决 #23 的裁决落点。
	// 只看 ColdStart、不看 Outcome，会把超时行上残留的 true 也数成一次冷启动。
	// 还要 CacheRead != nil：usage 对象缺失的成功轮，cold_start 是用没填的 0 算出来的
	// （CacheRead == 0 被写成冷），库侧对应的判据是 cache_read 非 NULL。
	// 适用行里冷暖不确知的（非 Ingested，或 Ingested 但 CacheRead 为 NULL）进 coldUnknown。
	// Refused 整行不适用，两个冷暖计数都不进。
	coldKnown := applicable && turn.Outcome(u.Outcome).Ingested() && u.CacheRead != nil
	if coldKnown {
		row.coldKnown++
		if u.ColdStart {
			row.ColdStarts++
		}
	} else if applicable {
		row.coldUnknown++
	}
}

// cmdCost 打当前会话的账：/cost 整条会话逐成员；/cost <agent-id> 只看一个成员；
// /cost --round <round-id> 只统计那一轮。
//
// needsSvc / needsConv 由命令表负责，这里不再判 svc 与当前会话。
func cmdCost(ctx context.Context, r *REPL, args cmdArgs) error {
	agentID, roundID, err := parseCostArgs(args.fields)
	if err != nil {
		return r.printf("%s\n", err.Error())
	}
	// limit < 0：与 UsageSamples 同一套，不限条数。账要的是范围内的全部行，
	// 先截断再聚合会把更早的轮悄悄丢掉。
	rows, err := r.svc.TurnUsage(ctx, r.cur.ID, agentID, -1)
	if err != nil {
		return r.reportErr("读账失败", err)
	}
	// nBefore 是过滤前的行数。空结果用它判，不用 unknown：
	// 行全属于别的 round、又没有空 round 行时，过滤后为空而 unknown 仍是 0。
	// 那时候打「本会话还没有记过账」，是把「这一轮没有」说成「会话没有」。
	nBefore := len(rows)
	unknown, unknownLost := 0, 0
	if roundID != "" {
		// 空 round 行不带 --round 时在表里，带了就整批消失，屏幕上一个字都不留。
		// 那是把「不知道属于哪一轮」写成「不属于这一轮」。
		// 空串也不许匹配本次的 round：匹配是把「不知道」写成「是」。
		// filterCostByRound 只认字符串相等，空串对不上 parseCostArgs 放行的非空 id。
		//
		// 空串的两个已知来源：崩溃后启动对账（lostUsage 只读得到 InflightTurn，
		// 那个结构体没有 RoundID，只能写 ""），以及 v4 加 round_id 列时的 DEFAULT ''。
		// 真正的修法在写侧：InflightTurn 带上 RoundID，BeginTurn 写入，对账带上它。
		// 本刀不做。老行的空串写侧也补不回来。
		// unknownLost 只数空 round 行里的 Lost。其余空 round 行从哪来读侧定不下来，
		// 只报这个实数，不写成「是 v4 之前的老账」。
		for _, u := range rows {
			if u.RoundID != "" {
				continue
			}
			unknown++
			if turn.Outcome(u.Outcome) == turn.OutcomeLost {
				unknownLost++
			}
		}
		// 过滤放在读回来之后，不另写一条 SQL。
		// 多一条 SQL 就多一处会与主路径分叉的过滤：agent 空串、升序、不丢空用量行
		// 都要在两条语句上各写一遍，写歪一条账就分叉。
		rows = filterCostByRound(rows, roundID)
	}
	summary := summarizeCost(rows)
	if len(summary) == 0 {
		if nBefore == 0 {
			return r.printf("本会话还没有记过账\n")
		}
		if err := r.printf("这一轮（round_id=%s）没有记账行\n", roundID); err != nil {
			return err
		}
		// 这句话之后还要看空 round 行。只剩这些行的会话配任何 --round，
		// 提前返回会让它们再一次一声不吭。过滤后没有数据行，也不建 tabwriter，
		// 一张只有表头的空表同样是谎。
		return printCostUnknownRound(r, unknown, unknownLost)
	}

	tw := r.newTabWriter()
	// 一格一个数都没有（适用行数 > 0，且这一列报了的轮数为 0）印 ?。
	// 部分轮次没报时合计照印数字。Refused 行不进任何屏幕上的未知计数：它没起进程。
	fmt.Fprintln(tw, "AGENT\tTURNS\tSPOKE\tLOST\tREFUSED\tIN\tCACHE_READ\tCACHE_CREATE\tOUT\tCOST_USD\tCOST_UNKNOWN\tCOLD\tUSAGE_UNKNOWN\tCOLD_UNKNOWN")
	for _, row := range summary {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\t%d\n",
			orPlaceholder(row.AgentID),
			row.Turns,
			row.Spoke,
			row.Lost,
			row.Refused,
			costCellCount(row.applicable, row.inReported, row.InputTokens),
			costCellCount(row.applicable, row.readReported, row.CacheRead),
			costCellCount(row.applicable, row.createReported, row.CacheCreate),
			costCellCount(row.applicable, row.outReported, row.OutputTokens),
			costCellUSD(row.applicable, row.CostKnown, row.CostUSD),
			row.costUnreportedApplicable,
			costCellCount(row.applicable, row.coldKnown, int64(row.ColdStarts)),
			row.usageUnknown,
			row.coldUnknown,
		)
	}
	// Flush 失败就停。表没写出去，再补一行会让失败看起来像已经说完。
	if err := tw.Flush(); err != nil {
		return err
	}
	// 追加行在 Flush 之后。tabwriter 与 r.printf 写的是同一个 r.out，
	// 攒到 Flush 才吐；写早了，这行会冒到表格上面。
	// 不带 --round，或空 round 行数为 0：表格就是全部输出。
	if roundID == "" || unknown == 0 {
		return nil
	}
	return printCostUnknownRound(r, unknown, unknownLost)
}

// printCostUnknownRound 在 --round 已经说出「这一轮的表 / 这一轮没有行」之后，
// 把没能归进任何一轮的行补在后面。unknown == 0 时不写：没有空 round 行就没有第二句。
// 括号里的数字只来自 unknownLost，两句格式串是冻结的。
func printCostUnknownRound(r *REPL, unknown, unknownLost int) error {
	if unknown == 0 {
		return nil
	}
	if unknownLost == 0 {
		return r.printf("未计入本轮：另有 %d 行 round_id 为空，不知道属于哪一轮；不带 --round 的 /cost 能看到它们\n", unknown)
	}
	return r.printf("未计入本轮：另有 %d 行 round_id 为空，不知道属于哪一轮（其中 %d 行是崩溃后启动对账补的账）；不带 --round 的 /cost 能看到它们\n", unknown, unknownLost)
}

// parseCostArgs 只认两种形状：零个词、一个 agent id，或恰好 --round 加一个 id。
// 不默默忽略多余的词：看错范围的账比报用法更糟。
func parseCostArgs(fields []string) (agentID, roundID string, err error) {
	switch len(fields) {
	case 0:
		return "", "", nil
	case 1:
		if fields[0] == "--round" {
			return "", "", fmt.Errorf("用法: /cost [--round <round-id> | <agent-id>]")
		}
		return fields[0], "", nil
	case 2:
		if fields[0] != "--round" || fields[1] == "" {
			return "", "", fmt.Errorf("用法: /cost [--round <round-id> | <agent-id>]")
		}
		return "", fields[1], nil
	default:
		return "", "", fmt.Errorf("用法: /cost [--round <round-id> | <agent-id>]")
	}
}

// filterCostByRound 留下 RoundID 恰好相等的行，保持原顺序。
// 原顺序已经是 created_at、turn_id 升序；summarizeCost 自己不排序输入，
// 这里再排一次只会让同一成员内的先后在调用方看来被改过。
func filterCostByRound(rows []conversation.TurnUsage, roundID string) []conversation.TurnUsage {
	out := make([]conversation.TurnUsage, 0)
	for _, u := range rows {
		if u.RoundID == roundID {
			out = append(out, u)
		}
	}
	return out
}

// costCellCount 在适用行里这一列一个数都没有时报 ?，否则印合计。
// applicable == 0（只有 Refused）时照印合计，全 0 是「被拒绝的轮什么都没花」。
func costCellCount(applicable, reported int, sum int64) string {
	if applicable > 0 && reported == 0 {
		return "?"
	}
	return fmt.Sprintf("%d", sum)
}

// costCellUSD 与 costCellCount 同一道门，印的是金额。
// known 是适用行里 CostUSD 非 NULL 的行数。Refused 的 CostUSD 恒为 NULL，
// 所以直接传 CostKnown 就是这个数。
func costCellUSD(applicable, known int, sum float64) string {
	if applicable > 0 && known == 0 {
		return "?"
	}
	return formatCostUSD(sum)
}

// formatCostUSD 把金额格式化到小数点后四位，尾随零去掉，但至少留两位。
// 只用于显示：内部累加仍是 float64 相加，这里不四舍五入成整数分。
// 单独放一个函数，是为了不在显示路径上引入第二个舍入策略。
func formatCostUSD(v float64) string {
	s := fmt.Sprintf("%.4f", v)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		for len(s) > i+3 && strings.HasSuffix(s, "0") {
			s = s[:len(s)-1]
		}
	}
	return s
}
