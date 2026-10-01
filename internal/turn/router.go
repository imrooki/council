package turn

import (
	"slices"
	"sort"
	"strings"

	"council/internal/config"
	"council/internal/conversation"
)

// Router 决定一批消息里哪些送给这个收件人，以及渲染成什么。
//
// 必须是纯函数。函数体里不得含时钟、随机源、游标数字、重试次数、
// 本轮序号，也不得依赖 map 迭代顺序。同一批消息 + 同一个收件人配置，
// 必须永远渲染出同一串字节。要表达「近期」只能用 seq 区间，不能用
// 墙钟；任何「距今多久」「已经发过几次」的写法都会让重发对不上前缀。
//
// 理由是 D-4 的实测量级：打断缓存前缀时没有任何一层会报错 ——
// agent 照常回答、退出码 0、JSON 合法、游标照推，只是每轮贵几十倍
// （冷启动 47749 token / 0.955 美元 vs 暖缓存边际 0.014 美元；
// Step 6 真机又量了一次，claude 两轮 0.1031 vs 0.0119）。
// 失败重发时送出去的东西逐字节相同，这是结构保证，不是靠小心。
type Router interface {
	// Select 从 ms 里挑出要送给 to 的消息，按 seq 升序返回。
	//
	// bootstrap 为真时走 bootstrap 规则（新会话的首轮），否则走增量规则。
	// 两者是同一个判定点的两面：bootstrap == (ProviderSession == "")，
	// 调用方不得再引入第二个「是不是首轮」的判断。
	//
	// 窗口必须写死成入参 ms 的全量。调用方有义务把 [0, ScannedTo]
	// 的全量交进来，不能只交某一页：readAfter 是分页拉的（historyPage
	// 为 512），「本页的最后一条」与「全量的最后一条」不是一回事。
	// 纯函数性本身拦不住窗口漂移 —— 两次调用只要窗口相同就仍然
	// 逐字节相同，但窗口随分页边界漂就会挑出不同的消息。
	Select(ms []conversation.Message, to config.AgentConfig, selfID string, bootstrap bool) []conversation.Message

	// Render 把挑出来的消息渲染成正文。与 Select 分开，是为了让
	// Delivered 与真正送出去的字节来自同一次挑选（Step 6 的教训：
	// 两处各过滤一遍，游标按一处推、重发却换了另一处的正文）。
	Render(selected []conversation.Message, title string, bootstrap bool) string
}

// defaultRouter 是 Step 7 的默认实现。
//
// routeThought 是 thought 的路由开关。04 第 3 节立场是默认存、默认
// 不路由；要让某个 **mode** 看到得在 Team 配置里显式打开（原文这里写的是
// role，已按未决 #7 的裁决订正：判别器只有 Mode，Role 是注入身份块的自由
// 文本，拿它当判别器等于让「把某人改成裁判」变成一次会话重建）。
// Team 配置已经接上：Runner 按 (team 的 thought_visible_to, 收件人 Mode)
// 算出这个 bool。NewRouter 的签名一个字不动（10a 冻结）。
//
// 恒传 false 的理由：探针里 grok 的 thought 直接复述了用户全局规则
// 文件的内容，路由给另一个 agent 等于让后者读到前者的私下斟酌，
// 而那段内容里可能有路径、IP、内部约定（06 点名的信息面问题）。
type defaultRouter struct {
	routeThought bool
}

// NewRouter 构造默认 Router。routeThought 为 false 时 thought 永远
// 不进正文。Runner 按 team 策略传入，签名一个字不动（10a 冻结）。
func NewRouter(routeThought bool) Router {
	return &defaultRouter{routeThought: routeThought}
}

// Select 见 Router 接口注释。增量路径复用 filterDelta，不另写一份
// 过滤：否则「渲染进 prompt 的 seq」和「真正送出去的字节」会对不上。
// 可见性同样只走 recipientOK，bootstrap 与增量共用；两处各写一份
// Recipients 判断就是 Step 6 那个教训的复刻。
func (r *defaultRouter) Select(ms []conversation.Message, to config.AgentConfig, selfID string, bootstrap bool) []conversation.Message {
	// to 留在冻结签名里给收件人特化，本步的规则只看消息本身。
	// 把收件人配置混进挑选结果，同一段历史就会因为「谁在读」而
	// 挑出不同的 seq，重发对不上。
	_ = to
	if !bootstrap {
		return filterDelta(ms, selfID, true)
	}
	return selectBootstrap(ms, selfID)
}

// recipientOK 是 bootstrap 与增量两条路径唯一的可见性判据。
// 两处各写一份就是 Step 6 那个教训的复刻：游标按一处推、重发
// 却换了另一处的正文。
//
// Recipients 为空 = 广播；非空 = 只发给名单里的 agent id。
//
// 这道收窄是不可逆的：一条消息在某个 agent 的流里被判为不路由，
// 游标照样推到 ScannedTo，它永远不会再送给它。
func recipientOK(m conversation.Message, selfID string) bool {
	if len(m.Recipients) == 0 {
		return true
	}
	return slices.Contains(m.Recipients, selfID)
}

// selectBootstrap 按 10_STEP7_SPEC.md 6.3 从 ms 里挑 bootstrap 包：
//
//  1. 会话标题由 Render 的 title 参数带进来，不占一条消息；
//  2. 全部 KindUser 与 KindTask 消息（KindTask 与 KindUser 同档：
//     它就是一条有结构的用户发言，SenderID 是保留 id "user"。
//     不走「每成员最后一条」——漏收会让该 agent 首轮要么挑不到
//     任何消息触发 ErrNoDelta，要么进程起了、钱花了，prompt 里
//     却没有任务正文）；
//  3. 每个其他成员在入参 ms 全量（即调用方交来的 [0, ScannedTo]
//     闭区间）内的最后一条发言（KindAgent 或 KindTaskResult）。
//     两者共用一个槽：这条规则的全部价值在于 bootstrap 包按成员数
//     封顶，给 KindTaskResult 开第二个槽就是把上界翻倍。
//
// 曾经有过「正文包含 @<selfID> 就加投」的分支。删掉它是一次有意
// 的行为变更，不是修 bug，也不是把子串匹配换成 token 边界。
// 那个分支扫的是入参 ms 的全量（[0, ScannedTo] 闭区间）。成员之间
// 习惯用 @ 互相称呼的长会话里，某个 agent 的 bootstrap 包会把
// 历史上每一条提到过它的消息全收进来，没有任何上界；旁边「每个
// 其他成员最后一条」按成员数封顶，这一条不封顶。bootstrap 包就是
// 入场费本身（claude 47749 / grok 20625 / agy 18180 token），
// 成本不可接受。循环保护由别处的导入图承担，不是删它的理由。
//
// 窗口写死成 ms 的全量，是因为「最后一条」是相对于扫描窗口的。
// readAfter 一页 512 条，若某个成员的最后一条落在更早的一页里，
// 「本页的最后一条」与「全量的最后一条」会是两条不同的消息，
// 同一个 bootstrap 在不同时刻就会挑出不同的包。
//
// 签名里没有成员表，本函数又必须是纯函数，不能去读 Participants。
// 「其他活跃成员」因此落成「在这个窗口里发过 KindAgent 或
// KindTaskResult 的、且不是自己的那些 sender」——只看消息本身
// 能判出来的那一部分。不可逆的可见性收窄见 recipientOK：游标
// 照样推到 ScannedTo，被判为不路由的消息永远不会再送给它。
func selectBootstrap(ms []conversation.Message, selfID string) []conversation.Message {
	seen := make(map[int64]struct{}, len(ms))
	out := make([]conversation.Message, 0, len(ms))
	add := func(m conversation.Message) {
		if _, ok := seen[m.Seq]; ok {
			return
		}
		seen[m.Seq] = struct{}{}
		out = append(out, m)
	}

	lastBySender := make(map[string]conversation.Message)
	for _, m := range ms {
		if !recipientOK(m, selfID) {
			continue
		}
		if m.Kind == conversation.KindSystem {
			// 与 filterDelta 同一条结构保证：KindSystem 不进 prompt。
			// 首轮不走 filterDelta，RunRound 的轮次面包屑同样依赖这里。
			continue
		}
		if m.Kind == conversation.KindUser || m.Kind == conversation.KindTask {
			add(m)
		}
		if (m.Kind == conversation.KindAgent || m.Kind == conversation.KindTaskResult) && m.SenderID != selfID {
			// ms 由 History 按 seq 升序交出；后写覆盖先写，留下窗口内最后一条。
			lastBySender[m.SenderID] = m
		}
	}

	// 遍历 map 求「每个成员最后一条」时必须先把 key 排序。
	// 不排序就会依赖 map 迭代顺序，两次调用的中间态不同；
	// 出口虽然还会按 seq 再排一次，但纯函数的论证不应建立在
	// 「凑巧最后 sort 了」上。
	senders := make([]string, 0, len(lastBySender))
	for id := range lastBySender {
		senders = append(senders, id)
	}
	sort.Strings(senders)
	for _, id := range senders {
		add(lastBySender[id])
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// Render 把 Select 的结果收成正文。bootstrap 时把会话标题放在最前，
// 它对应 04 4 的「原始/当前任务」，不是一条消息，不占 seq。
// thought 默认不进正文；只有构造时 routeThought 为真才附上。
func (r *defaultRouter) Render(selected []conversation.Message, title string, bootstrap bool) string {
	n := len(selected)
	if bootstrap && title != "" {
		n++
	}
	if n == 0 {
		return ""
	}
	parts := make([]string, 0, n)
	if bootstrap && title != "" {
		parts = append(parts, title)
	}
	for _, m := range selected {
		line := m.SenderID + ": " + m.Text
		if r.routeThought && m.Thought != "" {
			line += "\n" + m.Thought
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, "\n\n")
}
