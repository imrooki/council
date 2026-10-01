package repl

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	msgAutoUsage = "用法: /auto 在自动回复与只记下之间切换；/auto on 直接打字就让成员回复；/auto off 直接打字只记下"
	// msgAutoOn 与 msgAutoOnNoSummary 按配置里有没有 [summary] 选一句（v1.1）：
	// 没配总结成员时多个成员回复之后打的是 SM1 的「没有总结（没配总结成员…）」，说「自动总结」是假话。
	msgAutoOn          = "已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁），多个成员回复之后自动总结；/auto off 关掉"
	msgAutoOnNoSummary = "已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁）；没配总结成员，多个成员回复之后不做总结；/auto off 关掉"
	msgAutoOff         = "已关闭自动回复：直接打字只记下，不问任何人；要成员说话用 /ask 或 /round；/auto on 打开"
	// msgAutoNoneMentioned 是「这条消息点了名，但点到的没有一个是能回复的成员」时的一行。
	// 成因有三种（点的不是成员、点的已退出、点的是总结成员），一句话覆盖，不分因；
	// 点的不是成员时 postUser 已经先打了「<tok> 不是当前会话的成员」。
	// 贴进来的代码注解（@Override、@property）、npm i @types/node、@all 也解析得出 token、落到这一支，
	// 这些 @ 删不掉，所以末尾给与 msgAutoStrayAt 逐字相同的出路（v1.3）：消息已经记下了，/round 显式给 id 就让成员回复。
	// 用户可见的提示里不写尖括号占位符（v1.5）：照字面敲出来的尖括号不是合法的 id。
	msgAutoNoneMentioned = "点名的人都不是当前会话里能回复的成员，这条只记下了、没有问任何人；不点名就会问全部成员；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复"
	// msgAutoNoMembers 是「没点名，会话里也没有能回复的成员」时的一行：没有活跃成员，或只剩总结成员。
	msgAutoNoMembers = "当前会话还没有能回复的成员，这条只记下了；用 /add 加成员之后，直接打字就会有人回复"
	// msgAutoStrayAt 是「有 @ 冲着认识的成员去、却一个点名都没认出来」时的一行（v1.1；v1.5 按成员判）：
	// 谁都不问，不落到「没点名问全部成员」。全角 ＠ 不看后面是什么，不是点名的全角 ＠ 也会落到这一支，
	// 所以这一行同时给出路（v1.2）：消息已经记下了，/round 显式给 id 就让成员回复。
	// 不写尖括号占位符（v1.5）：照字面敲 @ 加尖括号加 id，按 hasStrayAt 仍是没认出的点名，用户会再看到这一行。
	msgAutoStrayAt = "有 @ 没认出点的是谁，这条只记下了、没有问任何人；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复；TUI 里敲 @ 会列出成员的 id 与显示名"
	// msgAutoStrayAtPartly 是「认出了一部分点名、还有冲着成员去的 @ 没认出来」时的一行（v1.1）：
	// 照问认出的那些（与「点名里夹着不在场的人照问其余」一致），这一行告诉用户有一处没算进去。
	msgAutoStrayAtPartly = "还有 @ 没认出点的是谁，没算进点名；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；TUI 里敲 @ 会列出成员的 id 与显示名"
)

// 对话模式（00_DECISIONS #47，2026-09-28，维护者）。
//
// 裸文本（不以 / 开头的一行、TUI 的一次提交、行式括号粘贴、多行草稿 /end）落库之后，
// 自动回复打开时紧接着问一轮，等价于手敲 /round <ids> --go：一轮、不带裁判、多个成员回复时照 SM1 自动总结。
// /say <文本>、/to、/ask <id> 文本、/round 一律不变。
//
// SetAutoReply 设定对话模式：on 为 true 是自动回复，false 是只记下。
// 与 SetConcise 同一条纪律，只许在没有命令在跑时调用
// （app 在 Run / tui.Run 之前调；/auto 本身是一条命令，命令是串行派发的）。
func (r *REPL) SetAutoReply(on bool) {
	r.autoReply = on
}

// cmdAuto 切换或设定对话模式。写法照 cmdVerbose：不带参数是切换；on / off 是设定
// （只认小写、只认恰好一个参数）；别的一律打用法、不改。无前置条件。
// 打开时按配置里有没有 [summary] 选 msgAutoOn 或 msgAutoOnNoSummary（r.cfg 经 New 兜底恒非 nil）。
func cmdAuto(_ context.Context, r *REPL, args cmdArgs) error {
	switch {
	case len(args.fields) == 0:
		r.autoReply = !r.autoReply
	case len(args.fields) == 1 && args.fields[0] == "on":
		r.autoReply = true
	case len(args.fields) == 1 && args.fields[0] == "off":
		r.autoReply = false
	default:
		return r.printf("%s\n", msgAutoUsage)
	}
	if r.autoReply {
		if r.cfg.Summary.Agent == "" {
			return r.printf("%s\n", msgAutoOnNoSummary)
		}
		return r.printf("%s\n", msgAutoOn)
	}
	return r.printf("%s\n", msgAutoOff)
}

// autoReplyRound 在一条裸文本落库之后定名单，再经 roundCmd 问一轮。
//
// 名单只按这条消息算，永远传显式 id，不走 /round 不带 id 的那一支（lastUserMentionIDs）：
// 那一支对「点了名但交集为空」打 /round 自己的「名单为空」拒绝，对点了总结成员的整条拒绝，都不是这里要的。
//   - 候选 = 当前会话的活跃成员（Participants includeInactive=false）去掉总结成员（r.cfg.Summary.Agent）。
//     总结成员出现在 /round 名单里会让整条拒绝；它第一次总结之后已经是活跃成员，所以第二句话起一定要剔。
//   - 这条消息里有 mention（ParseMentions）：名单 = mention 与候选的交集，按 mention 出现的顺序；
//     交集为空时打 msgAutoNoneMentioned，谁都不问。
//   - 没有 mention：名单 = 全部候选；候选为空时打 msgAutoNoMembers，谁都不问。
//   - v1.1：有冲着认识的成员去、却没认出来的 @（hasStrayAt，v1.5 按成员判）时，没有 mention 就打
//     msgAutoStrayAt、谁都不问（想点名却没认出来，不能当成没点名去问全部成员）；有 mention 就先打
//     msgAutoStrayAtPartly，再照上面有 mention 的规则走。
//
// 名单的顺序不影响屏幕：PlanRound 按字典序排、去重，「本轮问 N 个」与手敲同样名单的 /round 相同。
// judge 模式的成员照 PlanRound 的既有规则进 Judges，不带 --with-judge，所以照打「裁判 X 本命令不问」。
//
// 走 r.runCommand(ctx, roundCmd, ...) 而不是直接调 cmdRound：rn 为 nil 时由 needsRunner 答
// msgNoRunner（这条消息已经记下了），不空指针。闸、A 之五在飞记录、指纹、BeginTurn、记账、
// 总结、总用时、观察者全在 cmdRound 那一条路上，这里一个都不另写。
//
// 代价：每条裸文本起 N 个进程，多个成员回复时再多一次总结调用，没有预览。取消照 /round 的既有行为。
func autoReplyRound(ctx context.Context, r *REPL, text string) error {
	parts, err := r.svc.Participants(ctx, r.cur.ID, false)
	if err != nil {
		return r.reportErr("round", err)
	}
	sumID := r.cfg.Summary.Agent
	// hasStrayAt 的匹配集合（v1.5）：配置里全部 agent 的 id，并上当前会话活跃成员的 id
	// （配置里有、会话里没加入的算；会话里有、配置里已经没有的也算，#45 的改名）。
	// 显示名取配置里的 display_name，TrimSpace 之后为空的不要。
	known := make(map[string]bool, len(r.cfg.Agents)+len(parts))
	names := make([]string, 0, len(r.cfg.Agents))
	for id, a := range r.cfg.Agents {
		known[id] = true
		if name := strings.TrimSpace(a.DisplayName); name != "" {
			names = append(names, name)
		}
	}
	byLower := make(map[string]string, len(parts))
	all := make([]string, 0, len(parts))
	for _, p := range parts {
		known[p.AgentID] = true
		if p.AgentID == sumID {
			continue
		}
		byLower[strings.ToLower(p.AgentID)] = p.AgentID
		all = append(all, p.AgentID)
	}
	ids := all
	stray := hasStrayAt(text, known, names)
	if mentions := ParseMentions(text); len(mentions) > 0 {
		if stray {
			if err := r.printf("%s\n", msgAutoStrayAtPartly); err != nil {
				return err
			}
		}
		ids = make([]string, 0, len(mentions))
		for _, m := range mentions {
			if canon, ok := byLower[m]; ok {
				ids = append(ids, canon)
			}
		}
		if len(ids) == 0 {
			return r.printf("%s\n", msgAutoNoneMentioned)
		}
	} else if stray {
		return r.printf("%s\n", msgAutoStrayAt)
	}
	if len(ids) == 0 {
		return r.printf("%s\n", msgAutoNoMembers)
	}
	fields := append(append([]string(nil), ids...), "--go")
	return r.runCommand(ctx, roundCmd, cmdArgs{fields: fields, rest: strings.Join(fields, " ")})
}

// hasStrayAt 报告 text 里有没有「冲着一个认识的成员去、却没被 ParseMentions 认成 mention」的 @
// （v1.1，只给 autoReplyRound 用；v1.5 按成员判）。要判的是这个 @ 是不是冲着成员去的：是，就不能把这句
// 当成没点名去问全员（花钱）；不是（numpy 的 A @ B、$@、here-string 收尾的 '@、价格 @ 5 元、@所有人、@{），
// 就是没点名，按 维护者 的原话「如果不 @ 成员，默认是所有加入的都回复」问全员。
//
// 只看 ParseMentions 允许 mention 起头的位置：文本开头，或前一个字节不在 mentionLeftForbidden 里
// （a@b.com、a＠b.com 这类邮箱不看）。
// 全角 ＠（U+FF20）到这里就算，后面是什么都一样：ParseMentions 只认半角，它永远构不成 mention，
// 中文输入法下敲出来的 ＠ 多半是想点名。不许「为了一致」把全角也改成按成员判。
// 半角 @ 的下一个字节是 ASCII 字母或数字时，ParseMentions 已经把它认成 mention（mentionIDRe 只要求
// 首字符是字母数字，剥掉尾点之后结尾也一定合），点的不是成员时走 msgAutoNoneMentioned，这里跳过。
// 其余的半角 @ 按成员判：从下一个字符起，一个一个跳过 @ 与成员之间的分隔符（atSkip：空格、tab、
// 标点这类 ASCII 字节，以及全角括号、引号、冒号、全角空格、NBSP、零宽字符这类非 ASCII 的分隔符；
// v1.6 起非 ASCII 的也跳，中文输入法下 @（smx_b）与 @(smx_b) 是同一个意图）。遇到 CR / LF、字母、
// 汉字、数字就停；停在 CR / LF 上时下面两条都不会命中：行末的 @ 与下一行开头的词不算一回事。
// 两条命中一条就算：
//   - 跳过途中的每一个位置（含 @ 紧后面、还没跳的时候），剩下的文本以 names 里的一个显示名开头
//     （v1.7 起不分大小写，用 strings.EqualFold 比：@ 坏 x 也冲着显示名「坏 X」，与 id 那一支折小写一致；
//     先按显示名的字节长度切出一段，剩下的不够长就不比，切片不会越界；切口落在多字节字符中间时，
//     这一段的尾巴不是合法 UTF-8，EqualFold 按 U+FFFD 比，对不上；显示名自己含 U+FFFD 时才可能对上，
//     那也只往谁都不问那一侧偏）；每个位置都看，是因为显示名自己也可能以括号、引号开头；
//   - 跳过之后按 mentionTokenChar 取最长的一段，剥掉尾部的点、转小写，恰好是 known 里的一个 id。
//
// 一个 @ 不算，就接着看后面的 @：认成 mention 的与不冲着成员的都不定胜负
// （@smx_b @甲 算，价格 @ 5 元 @甲 也算）。
// known 与 names 由 autoReplyRound 在读成员表之后给：配置里全部 agent 的 id 并上当前会话活跃成员的 id，
// 配置里的显示名。@所有人 不冲着任何成员，所以问全员；这里不许加「@所有人 / @all 就是问全体」的语义。
// 只读 mention.go 的两个辅助，不改 ParseMentions；mention.go 的 tokenizer 改了，这里要跟着改。
func hasStrayAt(text string, known map[string]bool, names []string) bool {
	for i := 0; i < len(text); i++ {
		full := strings.HasPrefix(text[i:], string(rune(0xFF20)))
		if text[i] != '@' && !full {
			continue
		}
		if i > 0 && mentionLeftForbidden(text[i-1]) {
			continue
		}
		if full {
			return true
		}
		j := i + 1
		if j < len(text) && asciiAlnum(text[j]) {
			continue
		}
		for {
			for _, name := range names {
				if len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {
					return true
				}
			}
			n := atSkip(text[j:])
			if n == 0 {
				break
			}
			j += n
		}
		k := j
		for k < len(text) && mentionTokenChar(text[k]) {
			k++
		}
		if known[strings.ToLower(strings.TrimRight(text[j:k], "."))] {
			return true
		}
	}
	return false
}

// atSkip 报告 s 开头的那一个字符是不是 @ 与成员之间可以跳过的分隔符（v1.6，只给 hasStrayAt 用）：
// 是，返回它的字节数；不是，返回 0。ASCII 里除了字母、数字、CR、LF 都跳（空格、tab、
// < ( " 反引号 * - _ . @ 等）；非 ASCII 只跳空白（Zs）、标点（P）、符号（S）、组合记号（M）、
// 格式字符（Cf）。字母与汉字不跳，要留给显示名与 id 去比；U+2028、U+2029、U+0085 这几个换行
// 不在这几类里，与 CR / LF 一样停下。
func atSkip(s string) int {
	if s == "" || asciiAlnum(s[0]) || s[0] == '\n' || s[0] == '\r' {
		return 0
	}
	if s[0] < 0x80 {
		return 1
	}
	r, size := utf8.DecodeRuneInString(s)
	if unicode.In(r, unicode.Zs, unicode.P, unicode.S, unicode.M, unicode.Cf) {
		return size
	}
	return 0
}

// asciiAlnum 报告 c 是不是 ASCII 字母或数字（v1.5，只给 hasStrayAt 与 atSkip 用）。
func asciiAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
