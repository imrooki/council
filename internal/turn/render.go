package turn

import (
	"strings"

	"council/internal/config"
	"council/internal/conversation"
)

// RenderDelta 把一批 canonical 消息渲染成这一轮的正文。
//
// 纯函数，这是 Step 6 守 04 第 0 节规则 1 的结构保证：函数体里不得含
// time.Now、不得含游标数字、不得含重试次数、不得含本轮序号、不得依赖 map
// 迭代顺序。messages 表只追加、行永不改写，所以同一个 seq 区间第二次渲染
// 必然得到同一串字节 —— 这是结构性的，不是靠自觉。用 golden 文件锁住。
//
// 过滤规则集中在这一个函数里：跳过 KindSystem（04 第 2 节把内部事件与
// provider 诊断列进「不应当路由的」），skipSelf 为真时跳过这个 agent 自己
// 说过的话。
//
// skipSelf 不是第二个判定：它由 (ProviderSession != "") 导出，与「是不是新
// 会话」同一个谓词。一条全新的 provider 会话里什么都没有，包括这个 agent
// 自己过去的发言，所以新会话那一轮必须重放自己；resume 那一轮则必须跳过。
func RenderDelta(ms []conversation.Message, to config.AgentConfig, selfID string, skipSelf bool) string {
	// Step 6 的挑块是「游标之后的全部」一条直线（09 H 节），不按收件人特化。
	// to 留在冻结签名里，是给 Step 7 的 Router 换实现时不必改契约。
	// 这里故意不把 to.DisplayName / to.Role 写进 delta 正文。
	//
	// 订正（2026-09-21）：本段原来的理由写的是「DisplayName 不在
	// IdentityFingerprint 里」，那句是错的 —— renderIdentity 把发言者自己的
	// DisplayName 拼进了身份块，而身份块整段进指纹，所以改显示名**确实会**
	// 触发 ErrIdentityDrift，测试已钉住这个行为。
	// 那个行为本身是对的：显示名进了身份 flag，改了前缀就真的断了，
	// 拒绝比静默重建诚实。错的只是这条注释的措辞。
	//
	// 不写进 delta 正文的真实理由是另一条：delta 的内容必须只由 messages 决定。
	// 把收件人的配置字段混进正文，同一段历史就会因为「谁在读」而渲染出不同的
	// 字节，重发时对不上，而且
	// 打断时退出码 0、JSON 合法、游标照推，只是每轮贵几十倍（D-4 实测
	// 冷启动 0.955 美元 vs 暖缓存 0.014 美元）。
	_ = to

	visible := filterDelta(ms, selfID, skipSelf)
	if len(visible) == 0 {
		return ""
	}
	parts := make([]string, 0, len(visible))
	for _, m := range visible {
		// thought 默认存但不路由（04 第 3 节）。写进 prompt 等于让别的
		// agent 读到这段私下斟酌；探针里 grok 的 thought 还复述过用户全局规则。
		parts = append(parts, m.SenderID+": "+m.Text)
	}
	return strings.Join(parts, "\n\n")
}

// filterDelta 是 RenderDelta 与 Runner 填 Delivered 共用的过滤。
// 两处必须是同一个函数：否则「渲染进 prompt 的 seq」和「真正送出去的字节」
// 会对不上，游标按 Delivered 推、重发却换了一段正文，正好打断缓存前缀。
//
// 可见性只走 recipientOK：它是 bootstrap（selectBootstrap）与增量
// （本函数）两条路径唯一的可见性判据。这里再手写一份 Recipients
// 判断，就是 Step 6「两处各过滤一遍」的复刻。
//
// KindTask / KindTaskResult 在增量路径不另开槽——非 system、过
// recipientOK、再按 skipSelf 跳过自己，即过。按成员数封顶只属于
// bootstrap 包；增量轮按窗口里实际新增的消息走。
// RunRound 的 round_started / round_finished 依赖这里丢掉 KindSystem：
// 那两条面包屑因此绝不进 prompt。这是结构保证，不是碰巧。改掉
// 这条 continue 等于改轮次边界的可见性，不是只改内部事件的显示。
func filterDelta(ms []conversation.Message, selfID string, skipSelf bool) []conversation.Message {
	out := make([]conversation.Message, 0, len(ms))
	for _, m := range ms {
		if m.Kind == conversation.KindSystem {
			continue
		}
		if !recipientOK(m, selfID) {
			continue
		}
		if skipSelf && m.SenderID == selfID {
			continue
		}
		out = append(out, m)
	}
	return out
}

// RulesBlocks 是要渲染进身份块的两段规则正文，已由 config.LoadRules 归一化。
// 用独立类型而不是两个 string 参数：这两段总是一起传，拆开传迟早有人传反。
type RulesBlocks struct {
	Global string
	Local  string
}

// renderIdentityCore 是**不含规则**的那半块身份文本，也就是本函数改动之前
// renderIdentity 的全部内容，一个字节都没动。
//
// 它单独存在只有一个理由：**指纹的 role 段必须用这一块，不能用带规则的整块**。
// IdentityFingerprint 把 role 段按全文存进 identity_fp 那一列，而规则另有
// 两段摘要。若把带规则的整块喂进去，同一份规则正文会以全文再进一次那一列 ——
// 摘要就白设了（库照样被撑大），而且规则一改，identityDriftError 会先把
// 几 KB 的旧规则原文糊在屏幕上，再打那两行本来是给人看的摘要。
//
// 这个矛盾是实现方在照契约写代码时发现并指出来的，契约原文没有区分这两块。
//
// Provider 写进身份块是有意的：IdentityFingerprint 的冻结签名不含 provider id，
// 用户把同一个 agent 从 claude 改成 grok 时，若不改身份文本，编排层会拿着
// claude 的会话 id 去 grok --resume。把 provider 收进这块文本，指纹就会变，
// 下一轮以 ErrIdentityDrift 拒绝，而不是静默串台。
func renderIdentityCore(agentID string, ac config.AgentConfig) string {
	name := ac.DisplayName
	if name == "" {
		name = agentID
	}
	var b strings.Builder
	b.WriteString("You are ")
	b.WriteString(name)
	b.WriteString(" (id=")
	b.WriteString(agentID)
	b.WriteString(").\n")
	b.WriteString("Provider: ")
	b.WriteString(ac.Provider)
	if ac.Role != "" {
		b.WriteString("\n")
		b.WriteString(ac.Role)
	}
	return b.String()
}

// renderIdentity 把规则两段与核心身份拼成真正送进 TurnRequest.Role 的整块。
// 适配器再把它放上 --append-system-prompt-file / --system-prompt-override，
// 或（agy）拼进首轮正文最前面。本函数是纯函数：只读 AgentConfig 与
// RulesBlocks，不含时钟、不含会话 id、不含第几轮 —— 同一份配置与同一份
// 规则两次调用逐字节相同。
//
// 规则段排在核心身份前面，且共享（global）在前、专有（local）在后。
// 实测发现这些 CLI 的前缀缓存是按内容命中、不按会话命中的，所以同一家
// provider 的多个 agent 共享的前缀越长越省。排序本身零成本，不排是白扔。
//
// 两段各自带一行 ASCII 标签。grok 那一家切不断用户全局规则的继承，
// 出问题时唯一的窗口是让成员自述它看到了什么；没有标签它没法说清
// 那条指令来自哪个文件。标签两行加起来二十几字节，值。
//
// **两段都为空时输出与本改动之前逐字节相同**，这条由 golden 用例钉住。
// 它加上 schema v3 的纯追加迁移，共同保证不用规则文件的人升级之后
// 一条会话都不重建。
func renderIdentity(agentID string, ac config.AgentConfig, rules RulesBlocks) string {
	var b strings.Builder
	if rules.Global != "" {
		b.WriteString("# Council rules (global)\n")
		b.WriteString(rules.Global)
		b.WriteString("\n\n")
	}
	if rules.Local != "" {
		b.WriteString("# Council rules (project)\n")
		b.WriteString(rules.Local)
		b.WriteString("\n\n")
	}
	b.WriteString(renderIdentityCore(agentID, ac))
	return b.String()
}
