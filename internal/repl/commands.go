package repl

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"council/internal/command"
	"council/internal/conversation"
	"council/internal/turn"
)

// cmdArgs 是一条命令拿到的参数。
// fields 是命令名之后按空白切分的词，适合 /use、/leave 这类取单个 id 的命令；
// rest 是命令名之后的原始文本（只去首尾空白），/new 的标题与 /say 的正文必须读它 ——
// strings.Fields 会把正文里的连续空白压成一个，那是在改用户写下的内容。
// /add（/join）也切 rest 而不是用 fields：它的分隔符除了空白还有半角与全角逗号，
// fields 只按空白切，alpha,grok-4.7 会被当成一整段。
// bare 为 true 表示这段话是裸文本（不以 / 开头的一行、TUI 的一次提交、行式括号粘贴），
// 只有 dispatch、Submit、finishPaste 交给 sayCmd 时置上；/say <文本> 走斜杠命令那条路，永远是 false。
// 只有 cmdSay 读它（#47 对话模式）。
type cmdArgs struct {
	fields []string
	rest   string
	bare   bool
}

// errQuit 是 /quit 的哨兵错误。它不表示失败：Run 收到后打印告别行并返回 nil。
// 用错误而不是布尔返回值，是为了让命令处理函数保持同一个签名。
var errQuit = errors.New("repl: quit")

// emptyCell 是空字段在表格里的占位符。
// 直接留空会让后面的列贴到一起，看不出是哪一列空了。
const emptyCell = "-"

// currentCell 标记 /conv 里的当前会话。用 ASCII 星号而不是对勾类符号：
// 本机控制台默认 GBK，装饰性符号会乱码，而且规则明令禁止（08a 的符号禁令）。
const currentCell = "*"

const (
	// msgNoStore 是 svc 为 nil 时八个持久化命令的统一答复。
	msgNoStore = "持久化未就绪，本命令不可用"
	// msgNoConv 是没有当前会话时的提示。
	msgNoConv = "先 /new 或 /use 选一个会话"
	// msgNoRunner 是 rn 为 nil 时 /ask 与 /session 的统一答复。
	msgNoRunner = "provider 调用未就绪，本命令不可用"
)

// draftEndToken 与 draftCancelToken 是多行输入模式下**仅有的**两个被解释的行，
// 其余一律是正文。判定只看去掉首尾空白之后是否恰好等于这两个字面量。
//
// 终止符用 /end 而不是常见的单个句点，是因为这个模式的全部意义就是粘贴安全：
// 粘贴来的散文里出现一行孤立的句点完全可能，那会把消息拦腰截断，
// 而截断之后的剩余行会退回单行模式、被一条条当成新消息 —— 正是本模式要消灭的那个问题。
// 以 / 开头也让它与这个 REPL 其余的词汇一致，容易记。
const (
	draftEndToken    = "/end"
	draftCancelToken = "/cancel"
)

const (
	// msgDraftEnter 是进入多行输入时的说明。两个终止行必须写在里面 ——
	// 进得去出不来的模式比没有这个模式更糟。
	msgDraftEnter = "进入多行输入：整段粘贴或逐行敲；单独一行 /end 发送，单独一行 /cancel 放弃"
	// msgDraftEmpty 是草稿里没有任何正文时的答复，此时不发送。
	msgDraftEmpty = "草稿是空的，没有发送"
	// msgDraftCancelled 是用户主动放弃草稿时的答复。
	msgDraftCancelled = "已放弃这份草稿，未发送"
	// msgDraftDiscarded 是退出时还攒着草稿的答复（见 farewell）。
	msgDraftDiscarded = "多行草稿未发送，已丢弃"
	// msgDraftBadEncoding 是多行模式下**第一次**读到非法 UTF-8 时的答复。
	// 只打一次：一段编码错误的粘贴往往整段都是坏的，逐行报会把屏幕刷满。
	msgDraftBadEncoding = "这一行不是合法的 UTF-8，本份草稿已作废、不会发送；敲 /end 或 /cancel 退出多行输入"
	// msgDraftPoisoned 是作废的草稿被 /end 时的答复。
	msgDraftPoisoned = "草稿里有非法 UTF-8，整份已丢弃，未发送；请检查输入编码后重来"
	// msgNotUTF8 是单行模式下读到非法 UTF-8 时的答复。
	msgNotUTF8 = "这一行不是合法的 UTF-8，没有记录；请检查输入编码"
	// msgHasControl 是单行模式下读到控制字符时的答复。
	// 按 rune 判：码位 < 0x20 且不是 tab，或码位等于 0x7f。不用 unicode.IsControl，
	// 它会把 C1（U+0080-U+009F）也算进去，而这里有意放行 C1。
	msgHasControl = "这一行含有控制字符，没有记录，也没有当成命令执行"
	// msgPasteDiscarded 是退出时还在括号粘贴收集中的答复（见 farewell）。
	// 与草稿同一条纪律：没被用户确认过的半段不许写成永久记录。
	msgPasteDiscarded = "括号粘贴未完成，已丢弃"
	// msgPasteBadEncoding 是粘贴途中第一次读到非法 UTF-8 时的答复，只打一次。
	msgPasteBadEncoding = "粘贴内容含非法 UTF-8，整段作废"
)

// defaultLogLimit 是 /log 不带参数时打印的条数。
const defaultLogLimit = 20

// sayCmd 是 /say 的表项。它从表里单独提出来命名，是因为**裸文本发言复用它**：
// dispatch 把「非空、不以 / 开头的输入」原样交给这一项处理。
//
// 这样两条路径共用同一份前置条件（needsSvc / needsConv）与同一个处理函数，
// 不可能出现「/say 拦住了而裸文本没拦住」这种只在其中一条路径上暴露的洞。
// docs/08a_STEP5_API_CONTRACT.md 的「裸文本即发言」把这条列为硬性要求，
// 明确禁止为裸文本另写一份前置条件判断。
//
// 用包级变量而不是按名字查表：命令改名时编译器会直接报错，查表则会静默拿到
// 查不到时的零值表项，run 为 nil，到执行时才 panic。元数据（name / summary）
// 不在这里 —— 那是 Registry 的键，见 registry.go 里那一次 Register。
var sayCmd = cmdEntry{
	needsSvc:  true,
	needsConv: true,
	run:       cmdSay,
}

// roundCmd 是 /round 的表项。照 sayCmd 的先例单独命名，是因为对话模式（#47）的自动一轮复用它：
// 经 runCommand 走同一份前置条件（needsRunner 在这里拦住 rn 为 nil），再进 cmdRound 这一个处理函数，
// 屏幕与手敲同样名单的 /round <ids> --go 是同一份输出。
var roundCmd = cmdEntry{
	needsSvc:    true,
	needsConv:   true,
	needsRunner: true,
	run:         cmdRound,
}

// dispatch 处理一行输入：空行什么都不做，斜杠开头查命令表，其余一律当作发言。
// 未知命令、前置条件不满足都只打印提示，不退出、不返回错误。
//
// 最后那一档在 Step 4 时是打印「尚不支持对话」。Step 5 有了 Canonical Conversation，
// 改成直接以 user 身份发言（docs/08_STEP5_SPEC.md H 节「裸文本即发言」）。
// 代价是 Canonical Conversation 只追加不回收，手滑敲错的一行也会成为永久记录；
// 这一点已经当面告知用户并被接受。
func (r *REPL) dispatch(ctx context.Context, line string) error {
	payload, sawStart, sawEnd := splitPasteMarkers(line)

	// 编码闸门在最前面，草稿与粘贴两种模式共用。
	//
	// 非法 UTF-8 一旦落进 messages.text 就是永久的：SQLite 的 TEXT 不校验编码，
	// 而 Step 6 把历史拼成 JSON 送给 provider 时 json.Marshal 会把坏字节换成 U+FFFD
	// 且不报错 —— 于是库里存的、屏幕上显示的、agent 实际看到的三者永久不一致。
	// 宁可让用户重敲一行。本机用管道喂 stdin 时的 GBK 字节正是这条路。
	if !utf8.ValidString(line) {
		if r.inPaste || sawStart {
			return r.handlePasteInvalid(ctx, sawStart, sawEnd)
		}
		if r.inDraft {
			// 多行模式下有两条都不能走的路：只丢这一行会把一段粘贴悄悄截去一截，
			// 用户看不出少了什么；就此退出模式则会让粘贴剩下的行退回单行模式、
			// 一条条变成永久消息 —— 正是多行模式要消灭的那个问题。
			// 所以把草稿标成作废，留在模式里继续吃后续行，等用户显式退出。
			if !r.draftPoisoned {
				r.draftPoisoned = true
				return r.printf("%s\n", msgDraftBadEncoding)
			}
			return nil
		}
		return r.printf("%s\n", msgNotUTF8)
	}

	if r.inPaste || sawStart {
		return r.handlePaste(ctx, payload, sawStart, sawEnd)
	}

	if r.inDraft {
		return r.collectDraft(ctx, payload)
	}

	// 首尾同时剥掉空白与不可见字符。
	//
	// 只用 TrimSpace 是不够的：unicode.IsSpace 对 U+FEFF 与零宽字符全是 false，
	// 于是一个带 BOM 的 /join 过不了下面的 / 前缀判断，落进裸文本分支变成一条
	// 不可回收的发言，而屏幕上打的是「已记下 seq N」这种正常成功行，扫一眼发现不了。简洁档下这一行也不打，更看不出来。
	// 本机 PowerShell 5.1 用管道喂 stdin 时会在流首插 BOM，是已实证的高发场景。
	// 用 TrimFunc 而不是先 TrimSpace 再 TrimLeft，是为了处理两类字符交错的情形
	// （例如 BOM 后面还跟着空格）。
	text := strings.TrimFunc(payload, isTrimmable)
	if text == "" {
		return nil
	}
	if hasControlRune(text) {
		return r.printf("%s\n", msgHasControl)
	}
	if !strings.HasPrefix(text, "/") {
		// 交给 sayCmd 这个表项，而不是直接调 cmdSay：这样连前置条件一起复用，
		// 「持久化没起来」「还没选会话」两种情况下，裸文本与 /say 的答复完全一致。
		// rest 传整行原文；fields 按空白切分，与斜杠命令那条路的构造方式保持一致
		// （cmdSay 只读 rest，fields 在这里用不上，但两条路的 cmdArgs 形状应当一样）。
		// bare 置上：这是裸文本，自动回复打开时落库之后问一轮（#47）。
		return r.runCommand(ctx, sayCmd, cmdArgs{fields: strings.Fields(text), rest: text, bare: true})
	}

	fields := strings.Fields(text)
	name := strings.TrimPrefix(fields[0], "/")
	// Lookup 同时解析别名（/exit 与 /quit 是同一条命令）：别名表只在
	// 注册表里维护一份，dispatch 不知道、也不需要知道哪些名字是别名。
	_, cmd, ok := r.cmds.Lookup(name)
	if !ok {
		return r.printf("%s\n", msgUnknownCommand)
	}
	return r.runCommand(ctx, cmd, cmdArgs{fields: fields[1:], rest: restOf(text)})
}

// 这四个是 Windows 上从网页或文档粘贴、以及经管道喂 stdin 时真正会出现的不可见字符：
// 字节序标记与三个零宽字符。只收这四个，不穷举 Unicode 里所有不可见码位 ——
// 那会把正文里合法的格式字符也一起吃掉。
//
// 一律写成十六进制码位，不要改成带反斜杠 u 的转义形式：那种写法经过某些编辑与传输
// 路径时会被还原成真字符，而这几个字符恰好不可见，落进源码里既编译不过
// （Go 不允许文件中间出现 BOM），也违反本项目的符号禁令（U+200D 在禁用区间内）。
// 数字写法不可能被这样还原。这一处踩过一次，别改回去。
const (
	runeBOM  rune = 0xFEFF // 字节序标记
	runeZWSP rune = 0x200B // 零宽空格
	runeZWNJ rune = 0x200C // 零宽非连接符
	runeZWJ  rune = 0x200D // 零宽连接符
)

// hasControlRune 报告 text 里是否含有控制字符。
// 控制字符：码位 < 0x20 且不是 tab（0x09），或码位等于 0x7f。按 rune 判。
// 不用 unicode.IsControl：那会把 C1（U+0080-U+009F）算进去，而本定义有意放行 C1。
func hasControlRune(text string) bool {
	for _, c := range text {
		if c < 0x20 && c != '\t' || c == 0x7f {
			return true
		}
	}
	return false
}

// isTrimmable 报告一个字符是否该从输入行的首尾剥掉：空白，或上面那四个不可见字符。
func isTrimmable(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case runeBOM, runeZWSP, runeZWNJ, runeZWJ:
		return true
	}
	return false
}

// collectDraft 处理多行输入模式下的一行。
//
// 除两个终止行外，其余一律**原样**进草稿：不去首尾空白（缩进是正文的一部分），
// 不解释斜杠命令（以 / 开头的行就是以 / 开头的正文）。
// 这条是多行模式的全部意义所在 —— 粘贴进来的东西不该被再解释一遍。
func (r *REPL) collectDraft(ctx context.Context, line string) error {
	switch strings.TrimSpace(line) {
	case draftEndToken:
		poisoned := r.draftPoisoned
		lines := r.draft
		r.discardDraft()
		if poisoned {
			return r.printf("%s\n", msgDraftPoisoned)
		}
		text := strings.Join(lines, "\n")
		// 只有空白的草稿不发：那多半是误入模式又直接敲了 /end。
		if strings.TrimSpace(text) == "" {
			return r.printf("%s\n", msgDraftEmpty)
		}
		// 草稿发送算裸文本（#47）：自动回复打开时落库之后问一轮。
		return sayText(ctx, r, text, true)

	case draftCancelToken:
		r.discardDraft()
		return r.printf("%s\n", msgDraftCancelled)

	default:
		r.draft = append(r.draft, line)
		return nil
	}
}

// discardDraft 退出多行输入模式并丢掉草稿，连同作废标记一起清。
func (r *REPL) discardDraft() {
	r.inDraft = false
	r.draft = nil
	r.draftPoisoned = false
}

// postUserText 以保留 sender "user" 的身份把 text 追加成一条消息。
//
// 五条路共用一个本体 postUser：/say 带参数、裸文本、多行草稿发送经 sayText，
// /ask 带文本、/to 经本函数。共用的理由和 sayCmd 一样 —— 发出去的消息形状
// 必须一模一样，分多份写早晚会走失同步。recipients 为空（含未传）即广播，
// 四条既有发言路径都走这一支，形状不许分叉；/to 是 Recipients 列的第一个
// 生产写入方，把非空列表从这里传进去，而不是另写一份 Post。
//
// recipients 用变长参数，是为了让 postUserText(ctx, r, text) 继续表示广播；
// /to 把解析好的 id 列表展开传入。
//
// 三个脑子字段一律不填：人不是模型，没有脑子可记。
// 简洁档下成功落库不打「已记下 seq N」、不打「点名了」；两条警告（不是成员、定向收窄）照打。
//
// 本函数是 postUser 的薄包装（auto 恒为 false）：/ask 带文本与 /to 的输出与 #47 之前逐字节相同。
func postUserText(ctx context.Context, r *REPL, text string, recipients ...string) error {
	_, err := postUser(ctx, r, text, false, recipients)
	return err
}

// sayText 是 cmdSay 与多行草稿发送共用的落库入口（#47 对话模式）。
// bare 为 true 表示这段话是裸文本；它与 r.autoReply 同时为真才是「自动」：
// 落库成功之后紧接着经 autoReplyRound 问一轮。Post 失败（posted 为 false）时一个进程都不起：
// 成员回复的必须是用户刚说的那句话，而那句话没进库。
// 其余情形（/say <文本>、自动回复关着）与 postUserText 逐字节相同。
func sayText(ctx context.Context, r *REPL, text string, bare bool) error {
	auto := bare && r.autoReply
	posted, err := postUser(ctx, r, text, auto, nil)
	if err != nil || !posted || !auto {
		return err
	}
	return autoReplyRound(ctx, r, text)
}

// postUser 是 postUserText 的本体。返回的 bool 表示这条消息落了库：
// Post 失败时打「发言失败: ...」、返回 false 与 nil（reportErr 打印成功时返回 nil，
// 所以只看 error 分不出有没有落库）。error 非 nil 只表示写屏幕失败。
//
// auto 为 true 表示随后要自动问一轮：这时不打「点名了 X（用 /ask X 让他说话）」——
// 那一轮马上就问，「用 /ask 让他说话」是假话。「不是当前会话的成员」照打。
func postUser(ctx context.Context, r *REPL, text string, auto bool, recipients []string) (bool, error) {
	posted, err := r.svc.Post(ctx, conversation.Message{
		ConvID:     r.cur.ID,
		SenderID:   conversation.ReservedSenderUser,
		Kind:       conversation.KindUser,
		Text:       text,
		Recipients: recipients,
	})
	if err != nil {
		return false, r.reportErr("发言失败", err)
	}
	if !r.concise {
		if err := r.printf("已记下 seq %d\n", posted.Seq); err != nil {
			return true, err
		}
	}
	if len(recipients) > 0 {
		// 收窄不可逆：一条消息在某个 agent 的流里被判为不路由，游标照样
		// 往前推，它永远不会再送给它。这句必须在落库成功后当场说出来，
		// 否则用户会以为只是「先给这几个人看」。
		if err := r.printf("只有 %s 会看到这条；其余成员永远不会 —— 游标只前进\n", strings.Join(recipients, ", ")); err != nil {
			return true, err
		}
	}

	// 点名提示是 best-effort：它在 REPL 层，别的写入方绕得开，不进契约、
	// 不阻塞、不影响落库结果。解析或查成员失败都只是不打这一行，
	// 绝不能让已经成功的 Post 变成一次「发言失败」。
	//
	// 这一处判定与 Router 的判定不是一个谓词的两份拷贝，不能「统一」：
	// 这里的输入是 participants 表（给人看的，回答「你点的这个名字在不在场」）；
	// Router 的输入只有消息本身（纯函数、不读 participants，回答「这条消息
	// 该不该进这个 agent 的包」）。两者回答的是不同的问题。
	tokens := ParseMentions(text)
	if len(tokens) == 0 {
		return true, nil
	}
	ps, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return true, nil
	}
	known := make(map[string]struct{}, len(ps))
	for _, p := range ps {
		known[p.AgentID] = struct{}{}
	}
	for _, tok := range tokens {
		if _, ok := known[tok]; ok {
			if !r.concise && !auto {
				if err := r.printf("点名了 %s（用 /ask %s 让他说话）\n", tok, tok); err != nil {
					return true, err
				}
			}
			continue
		}
		if err := r.printf("%s 不是当前会话的成员\n", tok); err != nil {
			return true, err
		}
	}
	return true, nil
}

// cmdTo 定向发言：落一条 KindUser、SenderID=user、Recipients 非空的消息。
//
// 用法: /to <id>[,<id>...] <文本>
//
// 必须走 postUserText 而不是另写一份 Post：四条既有发言路径（/say、裸文本、
// 多行草稿、/ask 带文本）继续走广播，/to 只是给同一个构造点多传一个
// Recipients。形状一分叉，Canonical Conversation 只追加不回收，就永远分叉。
//
// 两道闸，缺一不可：
//
//  1. Service.Post 校验「存在」（含已退出的）。未知 id 让 Post 响亮失败，
//     错误里点名那个 id。
//  2. 本函数在 REPL 层校验「有活人看得见」：Recipients 与当前活跃成员
//     的交集为空时拒绝、不落库，提示「这条消息不会有任何在场成员看到」。
//
// 第 2 道的理由：Service.Recipients 的既有口径明写「含已退出的成员：定向
// 发给一个退了的 agent 仍是合法的解析结果」，那条口径立于没有 Router 收窄
// 的年代。加上收窄之后，一条只发给已退出成员的消息会落库、对全部活跃成员
// 都过不了可见性判据、而游标照样往前推——永久对谁都不可见，且没有任何一层
// 会说话。
//
// 第 2 道放在 REPL 而不是 Service，因为它判的是「人的意图」不是数据不变量，
// 而且「活跃」这个属性会被 /leave 改变（那正是未决 #13 的 TOCTOU 窗口），
// Service 那一层不该收一条自带竞态的判据。
//
// 未知 id 与「全是已退出」要分开处理：未知的交给 Post 点名；全认识但没
// 一个在场的才打第 2 道的提示。否则 /to ghost x 会被「没有在场成员」盖住，
// 用户看不到 ghost 根本不存在。
func cmdTo(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) == 0 {
		return r.printf("用法: /to <id>[,<id>...] <文本>\n")
	}
	text := restOf(args.rest)
	if text == "" {
		return r.printf("用法: /to <id>[,<id>...] <文本>\n")
	}

	rawIDs := strings.Split(args.fields[0], ",")
	ids := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, raw := range rawIDs {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			return r.printf("用法: /to <id>[,<id>...] <文本>\n")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	ps, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return r.reportErr("列出成员失败", err)
	}
	byID := make(map[string]conversation.Participant, len(ps))
	for _, p := range ps {
		byID[p.AgentID] = p
	}
	missing := false
	anyActive := false
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			missing = true
			continue
		}
		if p.Active {
			anyActive = true
		}
	}
	if !missing && !anyActive {
		return r.printf("这条消息不会有任何在场成员看到\n")
	}

	return postUserText(ctx, r, text, ids...)
}

// runCommand 检查一条命令的前置条件，通过了才执行它。
//
// 前置条件只在这一个地方判：msgNoStore、msgNoConv、msgNoRunner 各自只有一个出处，
// 加新命令时改命令表即可，裸文本那条路也自动跟着一致 —— 那正是它必须存在的理由。
func (r *REPL) runCommand(ctx context.Context, cmd cmdEntry, args cmdArgs) error {
	// 顺序是有讲究的：持久化没起来时，连「有没有当前会话」都无从谈起；
	// 没选会话时，provider 调用未就绪这句话还轮不到说。
	// 所以 needsSvc -> needsConv -> needsRunner，先答 msgNoStore，再答 msgNoConv，
	// 最后才答 msgNoRunner。
	if (cmd.needsSvc || cmd.needsConv || cmd.needsRunner) && r.svc == nil {
		return r.printf("%s\n", msgNoStore)
	}
	if cmd.needsConv && r.cur == nil {
		return r.printf("%s\n", msgNoConv)
	}
	if cmd.needsRunner && r.rn == nil {
		return r.printf("%s\n", msgNoRunner)
	}
	return cmd.run(ctx, r, args)
}

// restOf 返回命令名之后的原始文本，只去掉首尾空白。
// 命令名本身以 / 开头且不含空白，所以第一个空白字符就是分界点。
func restOf(text string) string {
	i := strings.IndexFunc(text, unicode.IsSpace)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(text[i:])
}

// cmdHelp 从命令表生成帮助，按命令名字典序输出。
//
// 格式在 WriteCommandTable 里，与 council commands（不带 -json）共用同一份定义：
// 帮助不再是一份手写的命令清单，加命令时不可能漏改。
func cmdHelp(_ context.Context, r *REPL, _ cmdArgs) error {
	return WriteCommandTable(r.out, r.cmds.List(command.ScopeTUI))
}

// cmdQuit 返回哨兵错误 errQuit，由 Run 翻译成正常退出。
func cmdQuit(_ context.Context, _ *REPL, _ cmdArgs) error {
	return errQuit
}

// cmdProviders 列出 provider 的 id 与 command。
func cmdProviders(_ context.Context, r *REPL, _ cmdArgs) error {
	if len(r.cfg.Providers) == 0 {
		return r.printf("没有配置任何 provider\n")
	}

	tw := r.newTabWriter()
	fmt.Fprintln(tw, "ID\tCOMMAND")
	for _, id := range sortedKeys(r.cfg.Providers) {
		fmt.Fprintf(tw, "%s\t%s\n", id, r.cfg.Providers[id].Command)
	}
	return tw.Flush()
}

// cmdAgents 列出 agent 的 id、provider、model、effort、permissions、mode。
//
// 不显示 display_name：tabwriter 按字节数算列宽，而 display_name 通常是中文
// （一个汉字 3 字节、终端里占 2 格），混进表格会把后面所有列都撑歪。
// /agents 是给人核对配置用的，id 已经足够定位，所以取对齐、舍 display_name。
func cmdAgents(_ context.Context, r *REPL, _ cmdArgs) error {
	if len(r.cfg.Agents) == 0 {
		return r.printf("没有配置任何 agent\n")
	}

	tw := r.newTabWriter()
	fmt.Fprintln(tw, "ID\tPROVIDER\tMODEL\tEFFORT\tPERMISSIONS\tMODE")
	for _, id := range sortedKeys(r.cfg.Agents) {
		a := r.cfg.Agents[id]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			id,
			orPlaceholder(a.Provider),
			orPlaceholder(a.Model),
			orPlaceholder(a.Effort),
			orPlaceholder(string(a.Permissions)),
			orPlaceholder(string(a.Mode)),
		)
	}
	return tw.Flush()
}

// cmdTeams 列出 team 的 id、成员数与成员 id 列表，末尾多一列标出
// 当前会话绑的是哪一个。既有三列一个都不删。
//
// 成员列表保持配置里的书写顺序（TOML 数组有序），不重排 —— 顺序本身是用户的意图，
// 而且同一份配置每次输出都一样，测试照样稳定。
//
// CURRENT 列回库读 Conversation，不读 r.cur.TeamID。r.cur 是缓存副本，
// BindTeam 只写库，副本里的 TeamID 不会自己变。/team use 绑完后会用
// 读回的整份替换 r.cur，所以「刚绑完再 /teams」这条路径上缓存是新的；
// 但 /teams 是给人看的只读命令，它的全部价值就是告诉用户现在到底绑
// 的是哪个。读缓存等于把这一列的正确性绑死在「每条写 TeamID 的路径
// 都记得刷新 r.cur」上 —— 今天写路径只有 /team use 一条，漏一次就是
// 一次静默说谎，而且 10a §1.1 的 cursors 表已经为「两处各维护一份
// 真相」付过一次账。回库读把正确性放回库。一次库读发生在一条交互
// 命令上，代价可以忽略。
//
// r.cur == nil 时这一列印占位符，并且不调 Conversation：/teams 没有
// needsConv，没选会话也该能看有哪些 team。标记复用 /conv 的 currentCell
// （ASCII 星号），未绑或没选会话都是 emptyCell；id 已经在 ID 列，不抄第二遍。
func cmdTeams(ctx context.Context, r *REPL, _ cmdArgs) error {
	if len(r.cfg.Teams) == 0 {
		return r.printf("没有配置任何 team\n")
	}

	boundID := ""
	if r.cur != nil {
		c, err := r.svc.Conversation(ctx, r.cur.ID)
		if err != nil {
			return r.reportErr("读当前会话失败", err)
		}
		boundID = c.TeamID
	}

	tw := r.newTabWriter()
	fmt.Fprintln(tw, "ID\tMEMBERS\tMEMBER_IDS\tCURRENT")
	for _, id := range sortedKeys(r.cfg.Teams) {
		t := r.cfg.Teams[id]
		current := emptyCell
		if boundID != "" && id == boundID {
			current = currentCell
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n",
			id,
			len(t.Members),
			orPlaceholder(strings.Join(t.Members, ", ")),
			current,
		)
	}
	return tw.Flush()
}

// cmdTeam 是 `/team use <teamID>` 的入口。今天只有这一个子命令。
// 参数不对就印一行用法，不报错、不退出（体例同 /session rebuild）。
func cmdTeam(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) != 2 || args.fields[0] != "use" {
		return r.printf("用法: /team use <team-id>\n")
	}
	return cmdTeamUse(ctx, r, args.fields[1])
}

// cmdTeamUse 绑定当前会话到 teamID，并把该 team 里尚未加入的成员一次性 Join 进来。
//
// 「尚未加入」必须按全量 participants 判，不能只看活跃的：AddParticipant
// 对同一个 (ConvID, AgentID) 是重新激活不是插第二行，按 includeInactive=false
// 会把已经 /leave 的人判成尚未加入、Join 一次、静默拉回。那正是不做的事，
// 而且每拉回一个就是一次入场费（实测 0.4 到 1.0 美元）。
//
// 判据：先 Participants(ctx, convID, includeInactive=true) 拉全量。
// 全量是活跃的超集，查一次就同时覆盖「活跃表」和「含退出的表」。
// active 的跳过；LeftAt 非空的也跳过，并逐个点名
// 「X 已退出，未自动拉回，要加回用 /join」。
// 只有两张表里都查不到的才 Join。点名这件事进契约：静默跳过等于
// 让用户以为整个 team 都进来了。
//
// 例外是总结成员（[summary] agent，#47，CL1）：配置校验允许它出现在 team 的 members 里，
// 两张表里都查不到它时也不 Join，同样点名一行（「不随 team 加入」），多个成员回复之后由
// runSummary 自己加入。在场、已退出的总结成员照上面两条走，与普通成员相同。
//
// BindTeam 只写库，r.cur 不会跟着变。绑成功之后回库读 Conversation
// 整份替换 r.cur，不要只改 r.cur.TeamID —— 那是在两个地方各维护一份
// 真相。
//
// 这一步为什么还留着：/teams 的 CURRENT 列（cmdTeams）读的是回库的
// Conversation，不读 r.cur.TeamID——那条读法本身就不依赖"谁写了
// TeamID 都记得刷新缓存"，所以理论上不刷新 r.cur 也不会让 /teams
// 出错。这里仍然刷新，理由换了一条：r.cur 整体（尤其 r.cur.ID）在
// 别处到处被读，留一个已知过期的字段（TeamID）在里面是另一种技术
// 债——刷新是为了不让 r.cur 变成一份明知有部分字段错误的快照，不是
// 为了 /teams 的正确性。
//
// 改这段注释时对 team_use_test.go 做过三个变异：把「尚未加入」的判据
// （上面注释里 Participants includeInactive=true 那一段）改成只看
// 活跃成员（includeInactive=false）——TestTeamUseDoesNotSilentlyRejoinLeftMember
// （用例 2）红，三条断言同时红，说明用例对这条判据是有牙的；把
// cmdTeams 改成直接读 r.cur.TeamID 缓存——用例照样绿；反过来去掉这里
// 的 r.cur 刷新、只留 cmdTeams 的回库读——用例也照样绿。也就是说
// "回库读"与"刷新 r.cur"这两条机制目前是冗余的：当前用例分不出哪
// 一条在承重，而且现查 r.cur.TeamID 今天没有任何读者。两条都保留是
// 本轮的裁决，不是因为哪条测试逼着必须留——下一个人要删其中一条时，
// 先知道删了不会有用例变红，得另外找理由（或另外补一条能分辨的用例）
// 再删。
//
// 重读一次的代价是一次库读，发生在一条交互命令上，可以忽略。
func cmdTeamUse(ctx context.Context, r *REPL, teamID string) error {
	t, ok := r.cfg.Teams[teamID]
	if !ok {
		return r.printf("配置里没有 team %s，用 /teams 看看有哪些\n", teamID)
	}

	if err := r.svc.BindTeam(ctx, r.cur.ID, teamID); err != nil {
		return r.reportErr("绑定 team 失败", err)
	}

	fresh, err := r.svc.Conversation(ctx, r.cur.ID)
	if err != nil {
		return r.reportErr("读会话失败", err)
	}
	r.cur = &fresh

	if err := r.printf("已绑定 team %s\n", teamID); err != nil {
		return err
	}

	ps, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return r.reportErr("列出成员失败", err)
	}
	existing := make(map[string]conversation.Participant, len(ps))
	for _, p := range ps {
		existing[p.AgentID] = p
	}

	for _, agentID := range t.Members {
		p, found := existing[agentID]
		if found {
			if p.Active {
				continue
			}
			if p.LeftAt != nil {
				if err := r.printf("%s 已退出，未自动拉回，要加回用 /join\n", agentID); err != nil {
					return err
				}
			}
			continue
		}

		// 总结成员（[summary] agent）不当普通成员加（#47，CL1）：它在 team 的 members 里、本会话里
		// 又还不是成员时不 Join，打一行说明；多个成员回复之后由 runSummary 自己 Join。已是成员的
		// （在场、已退出）走上面那一支，与它是不是总结成员无关。
		if sumID := r.cfg.Summary.Agent; sumID != "" && agentID == sumID {
			if err := r.printf("%s 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结\n", agentID); err != nil {
				return err
			}
			continue
		}

		ac, ok := r.cfg.Agents[agentID]
		if !ok {
			return r.printf("配置里没有 agent %s，用 /agents 看看有哪些\n", agentID)
		}
		part := conversation.Participant{
			ConvID:      r.cur.ID,
			AgentID:     agentID,
			DisplayName: ac.DisplayName,
			Provider:    ac.Provider,
			Active:      true,
			JoinedAt:    time.Now().UTC(),
		}
		if err := r.svc.Join(ctx, r.cur.ID, part); err != nil {
			return r.reportErr("加入失败", err)
		}
		if err := r.printf("%s 已加入会话 %s\n", agentID, r.cur.ID); err != nil {
			return err
		}
	}
	return nil
}

// cmdNew 建一个会话并把它切成当前会话。
// 标题取命令名之后的全部原始文本，允许含空格。
// 建会话时盖上当前项目目录，r.projectDir 为空时不盖章（与从前一样）。
// /use 不盖章：接回旧会话不改变它的项目归属。
func cmdNew(ctx context.Context, r *REPL, args cmdArgs) error {
	title := args.rest
	if title == "" {
		return r.printf("用法: /new <标题>\n")
	}

	conv, err := r.svc.CreateIn(ctx, title, r.projectDir)
	if err != nil {
		return r.reportErr("建会话失败", err)
	}
	r.cur = &conv
	return r.printf("已建会话 %s，标题: %s\n", conv.ID, oneLine(conv.Title))
}

// cmdConv 列出全部会话，首列标出当前那个。
// 顺序由 store 决定（规格 F：按 created_at 倒序），这里不重排 ——
// 这不是 map 遍历，顺序本来就是稳定的。
func cmdConv(ctx context.Context, r *REPL, _ cmdArgs) error {
	convs, err := r.svc.Conversations(ctx)
	if err != nil {
		return r.reportErr("列出会话失败", err)
	}
	if len(convs) == 0 {
		return r.printf("还没有任何会话，用 /new <标题> 建一个\n")
	}

	tw := r.newTabWriter()
	fmt.Fprintln(tw, "CUR\tID\tCREATED_AT\tTITLE")
	for _, c := range convs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			r.currentMark(c.ID),
			c.ID,
			formatTime(c.CreatedAt),
			orPlaceholder(oneLine(c.Title)),
		)
	}
	return tw.Flush()
}

// cmdUse 切换当前会话。参数是完整 id，或者 id 的一段前缀 ——
// 前缀是给人用的便利，id 那么长，每次敲全不现实。
// 前缀匹配到多个就报错并列出候选，一个都没匹配上同样报错，不静默留在原会话。
//
// 完整 id 先于前缀判定：id 的长度与字符集不在契约里（08a），
// 短 id 之间完全可能出现「一个是另一个的前缀」，那时精确匹配必须赢。
func cmdUse(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) != 1 {
		return r.printf("用法: /use <id 或 id 前缀>\n")
	}
	prefix := args.fields[0]

	convs, err := r.svc.Conversations(ctx)
	if err != nil {
		return r.reportErr("列出会话失败", err)
	}

	var matched []conversation.Conversation
	for _, c := range convs {
		if c.ID == prefix {
			matched = []conversation.Conversation{c}
			break
		}
		if strings.HasPrefix(c.ID, prefix) {
			matched = append(matched, c)
		}
	}

	switch len(matched) {
	case 0:
		return r.printf("没有 id 以 %s 开头的会话，用 /conv 看看有哪些\n", prefix)
	case 1:
		c := matched[0]
		r.cur = &c
		return r.printf("当前会话: %s，标题: %s\n", c.ID, oneLine(c.Title))
	default:
		if err := r.printf("id 前缀 %s 匹配到 %d 个会话，请给出更长的前缀:\n", prefix, len(matched)); err != nil {
			return err
		}
		// 候选按 id 排序，输出与 store 的返回顺序无关，测试才稳定。
		sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
		tw := r.newTabWriter()
		fmt.Fprintln(tw, "ID\tCREATED_AT\tTITLE")
		for _, c := range matched {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", c.ID, formatTime(c.CreatedAt), orPlaceholder(oneLine(c.Title)))
		}
		return tw.Flush()
	}
}

// fullwidthComma 是全角逗号的码位。写成数字常量而不是字面字符：本仓库踩过
// 编辑链路把转义还原成真字符的坑，数字常量不受影响。中文输入法下敲逗号
// 默认出的就是它，/add 不认它的话，alpha，grok-4.7 会被当成一整段。
const fullwidthComma rune = 0xFF0C

// cmdAdd 是 /add 与 /join 共用的 handler：把配置里的 agent 加进当前会话。
// 两个名字指向同一个函数，而不是两份实现，免得其中一份改了另一份没跟上。
//
// 只加配置里已定义的 agent（识别出的模型先被追加进配置文件，再按配置加入）：
// participant 的 display_name 与 provider 要从配置抄过去，
// 配置里没有这个 id 就没有这两样东西可抄，加进去也是一行残缺的成员。
//
// 第一个参数以 + 开头时交给 cmdAddDiscovered（addmodel.go，识别出的模型加入，#45）。
// --refresh 丢掉识别快照，其余与不带参数相同。不带参数时先打配置里的候选
// （候选为空时的说法见 printAddCandidates），再打识别出的模型，最后对本机装了、
// 配置里还没有的工具各打一行提示（cliadvice.go，#48）。
//
// 开关打开时（SetAddByName，app 装配时打开），带参数的 /add 先交给 cmdAddByName
// （addname.go，AD1）：每一段都在配置里命中、也没有 --id / --effort 时它不插手，
// 下面照旧。有段在配置里对不上时到识别快照里按名字找；两个词的写法与带 --id / --effort
// 的写法里，名字在配置里只是前缀命中也会去快照里找。找到之后多半是写进配置并加入；没给
// 强度、也没给 --id 的单个名字，若配置里已有同一个模型的成员，就只是加入那个成员，或者
// 拒绝、列出，都不写配置（addname_existing.go 的 addNameExisting）。
//
// 不带参数时列出可加入的候选（带编号）；其余参数按全名 / 编号 / 前缀加入，
// 可一次加多个。候选是「配置里的全部 id，去掉当前在场的成员与总结成员」，已退出的
// 普通成员在候选里：Join 对已有的行是 upsert，会把那一行重新激活。已退出的总结成员
// 不在候选里，/add <它的全名> 由 resolveAddSegment 第 0 步放行。
//
// 成员名单只读一次（includeInactive=true），在场与否看 Active。
func cmdAdd(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) > 0 && strings.HasPrefix(args.fields[0], "+") {
		return cmdAddDiscovered(ctx, r, args.fields)
	}
	refresh := false
	if len(args.fields) > 0 && args.fields[0] == "--refresh" {
		if len(args.fields) != 1 {
			return r.printf("%s\n", msgAddModelUsage)
		}
		refresh = true
		r.modelSnap = nil
	}

	participants, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return r.reportErr("列出成员失败", err)
	}
	active := make(map[string]bool, len(participants))
	member := make(map[string]bool, len(participants))
	for _, p := range participants {
		member[p.AgentID] = true
		if p.Active {
			active[p.AgentID] = true
		}
	}

	// 编号不存状态：每次都对当次重新算出的候选列表取下标。代价是两次命令之间
	// 有人加入、或配置被重载，同一个编号指向的 agent 会变 —— 所以每加成功
	// 一个都回显全名，用户当场看得见加的是谁。
	// 总结成员（[summary] agent）不进候选（#47）：它不当普通成员加，多个成员回复之后由
	// runSummary 自己 Join。候选只管列表与编号；显式点它的名字由 resolveAddSegment 拦。
	sumID := r.cfg.Summary.Agent
	var candidates []string
	for _, id := range sortedKeys(r.cfg.Agents) {
		if !active[id] && id != sumID {
			candidates = append(candidates, id)
		}
	}

	if len(args.fields) == 0 || refresh {
		if err := printAddCandidates(r, candidates, active, member); err != nil {
			return err
		}
		if err := printDiscoveredModels(ctx, r); err != nil {
			return err
		}
		// 识别被取消时不再打提示（#48）：那一次只该看到「识别被取消」。
		if ctx.Err() != nil {
			return nil
		}
		return printUnconfiguredCLIs(r)
	}

	segs := splitAddArgs(args.rest)
	if r.addByName {
		if done, err := cmdAddByName(ctx, r, args.fields, segs, candidates, active, member); done {
			return err
		}
	}
	if len(segs) == 0 {
		return r.printf("用法: /add 列出可加入的；/add <全名|编号|前缀>[,...] 一次加多个\n")
	}

	// 全或无：先把所有段解析完，任何一段失败就一个都不加，并把每一段的错误
	// 按出现顺序都打出来。部分加入会留下一个用户没要求的中间状态；而加入本身
	// 不花钱，改掉错的那段重敲一次，代价近乎为零。只报第一个错误的话，用户
	// 改完一段再敲，才发现下一段也错，要来回好几趟。
	var ids []string
	seen := make(map[string]struct{})
	var problems []string
	for _, seg := range segs {
		id, problem := resolveAddSegment(r, seg, candidates, active, member)
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		// 多段解析到同一个 id（例如 /add 1,<候选 1 的全名>）只加一次、只回显一次。
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(problems) > 0 {
		for _, msg := range problems {
			if err := r.printf("%s", msg); err != nil {
				return err
			}
		}
		return nil
	}

	// 全或无只管到解析这一层。存储层失败发生在逐个写入的途中，前面已经成功的
	// 不回滚：不改 internal/conversation 就做不到在一个事务里写多个成员。
	//
	// 陷阱：reportErr 打印成功时返回 nil，它不是「出错了」的信号。写成
	// if err := r.reportErr(...); err != nil { return err } 不会停下，
	// 会接着把后面的 id 也加进去。这里必须无条件 return。
	for _, id := range ids {
		ac := r.cfg.Agents[id]
		p := conversation.Participant{
			ConvID:      r.cur.ID,
			AgentID:     id,
			DisplayName: ac.DisplayName,
			Provider:    ac.Provider,
			Active:      true,
			JoinedAt:    time.Now().UTC(),
		}
		if err := r.svc.Join(ctx, r.cur.ID, p); err != nil {
			return r.reportErr("加入失败", err)
		}
		if err := r.printf("%s 已加入会话 %s\n", id, r.cur.ID); err != nil {
			return err
		}
	}
	return nil
}

// printAddCandidates 打不带参数的 /add：候选表，或者候选为空时四种说法之一（OB1），
// 按顺序取第一个成立的：总结成员在本会话里加入过又退出了（runSummary 不会再把它拉回来，
// 要恢复得 /join；这一种与其余成员在不在场无关，所以排最前）；配置里除总结成员外一个
// agent 都没有（首次运行生成的配置就是这样）；总结成员没加入过、其余都在场；都在场。
// 前三种以前都打「都已经在当前会话里了」，与事实不符。active、member 与 cmdAdd 的同名表
// 相同：在场的、含已退出在内的全部成员。
//
// display_name 放最后一列：tabwriter 按 rune 计宽，而中文在终端里一个字占两格，
// 放在中间列会把后面的列撑歪；放最后一列就没有后面的列可歪。
func printAddCandidates(r *REPL, candidates []string, active, member map[string]bool) error {
	if len(candidates) == 0 {
		sumID := r.cfg.Summary.Agent
		others := 0
		for id := range r.cfg.Agents {
			if id != sumID {
				others++
			}
		}
		switch {
		case sumID != "" && member[sumID] && !active[sumID]:
			return r.printf("总结成员 %s 已退出当前会话，要恢复总结用 /join %s；除它之外配置里没有可以 /add 的成员\n", sumID, sumID)
		case others == 0:
			return r.printf("配置里还没有可以 /add 的成员\n")
		case sumID != "" && !active[sumID]:
			return r.printf("除总结成员 %s 外，配置里的 agent 都已经在当前会话里了；总结成员不用 /add：多个成员回复之后它会自己加入做总结\n", sumID)
		default:
			return r.printf("配置里的 agent 都已经在当前会话里了\n")
		}
	}
	tw := r.newTabWriter()
	fmt.Fprintln(tw, "NO\tAGENT\tPROVIDER\tDISPLAY_NAME")
	for i, id := range candidates {
		ac := r.cfg.Agents[id]
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n",
			i+1,
			id,
			orPlaceholder(ac.Provider),
			orPlaceholder(oneLine(ac.DisplayName)),
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return r.printf("用 /add <编号|全名|前缀> 加入，多个用空格或逗号隔开，例如 /add 1,2\n")
}

// ideographicSpace 是全角空格的码位，写成数字常量的理由同 fullwidthComma。中文输入法
// 全角状态下敲空格出的就是它；args.fields（strings.Fields）本来就在它上面切，splitAddArgs
// 也切，两处对齐（AD1）。这一条不看 SetAddByName 的开关。
const ideographicSpace rune = 0x3000

// splitAddArgs 把 /add 的参数按 ASCII 空白、全角空格、半角逗号、全角逗号切开，丢掉空段。
// 按 rune 判而不是按字节：全角逗号是多字节的，按字节切会把它劈成乱码。
func splitAddArgs(s string) []string {
	return strings.FieldsFunc(s, func(c rune) bool {
		switch c {
		case ' ', '\t', '\n', '\r', '\v', '\f', ideographicSpace, ',', fullwidthComma:
			return true
		}
		return false
	})
}

// resolveAddSegment 把 /add 的一段解析成一个要加入的 id。成功时 problem 为空；
// 失败时 problem 是要打给用户的一整行（含换行），id 为空。
//
// 顺序是写死的，先命中的算数：全名精确 > 编号 > 前缀。配置里的第 0-3 步在
// resolveAddConfig，都没命中时的说法在 addSegmentNotFound：按名字加识别出的模型
// （AD1，addname.go）要在这两者之间插一步，到识别快照里找。
func resolveAddSegment(r *REPL, seg string, candidates []string, active, member map[string]bool) (id, problem string) {
	if id, problem := resolveAddConfig(r, seg, candidates, active, member); id != "" || problem != "" {
		return id, problem
	}
	return "", addSegmentNotFound(r, seg, active, member)
}

// resolveAddConfig 是 resolveAddSegment 在配置里的第 0-3 步。某一步命中时返回 id
// （加入）或 problem（拒绝的那一行，含换行），恰好一个非空。两个都是空串表示没命中，
// 只有一种情形：第 3 步的前缀在候选里一个都没找到。总结成员、只是成员不在配置里、
// 全是数字（含超出范围）、前缀歧义，都算命中。
func resolveAddConfig(r *REPL, seg string, candidates []string, active, member map[string]bool) (id, problem string) {
	// 0. 总结成员（#47）排在全名之前拦：它在配置里，不拦的话第 1 步就放行了。
	// 唯一放行的是「它是这个会话里已退出的成员」：那是 runSummary 那句「要恢复总结用 /join <id>」
	// 指给用户的路，Join 把那一行重新激活。没加入过、或正在场，一律拒绝。
	// 正在场的（OB1 v1.2）说它已经在了：它已经加入，再说「它会自己加入」不准。
	if sumID := r.cfg.Summary.Agent; sumID != "" && seg == sumID && !(member[seg] && !active[seg]) {
		if active[seg] {
			return "", fmt.Sprintf("%s 是总结成员（config.toml 的 [summary]），已经在当前会话里了\n", seg)
		}
		return "", fmt.Sprintf("%s 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结\n", seg)
	}
	// 1. 全名精确命中排在最前，而且集合要含成员 id（含已退出的）。全名是用户
	// 最明确的意图；W1 与 W1b 各抓到一次「全名被另一个以它为前缀的候选偷走」：
	// opus-5-0 在场、opus-5-0-max 是候选时，只在候选里判就会把 /add opus-5-0
	// 解析成 max；一个已从配置删掉、但仍是成员的 opus-5-0 也会被前缀解析成
	// 另一个候选，静默加入一个用户没点名的 agent。
	//
	// 在配置里的 id 要么是候选、要么是在场成员，两种都去 Join：在场成员照旧
	// 调 Join，是保留加前缀之前 /join 的行为 —— upsert 会按当下配置刷新
	// display_name、provider 与 joined_at 的快照，/who 的 JOINED_PROVIDER 列靠它。
	if _, ok := r.cfg.Agents[seg]; ok {
		return seg, ""
	}
	// 只是成员、不在配置里：没有 display_name / provider 可抄，不能 Join。
	// 这一段直接失败，绝不落进下面的前缀解析。
	if member[seg] {
		return "", fmt.Sprintf("配置里没有 agent %s，用 /agents 看看有哪些\n", seg)
	}

	// 2. 全是 ASCII 数字就当编号。正则允许全数字的 agent id（例如就叫 7），
	// 它只要在上一步能精确命中就按名字加入；编号只在没有精确命中时才生效。
	// Atoi 溢出（很长的数字串）与越界同样处理，都报「超出范围」。
	if isASCIIDigits(seg) {
		n, err := strconv.Atoi(seg)
		if err != nil || n < 1 || n > len(candidates) {
			if len(candidates) == 0 {
				return "", fmt.Sprintf("编号 %s 超出范围（没有可加入的）\n", seg)
			}
			return "", fmt.Sprintf("编号 %s 超出范围（1-%d）\n", seg, len(candidates))
		}
		return candidates[n-1], ""
	}

	// 3. 前缀只在候选里找。代价：opus-5-0 在场、opus-5-0-max 是候选时，
	// /add opus-5 会加入 max。与 /ask 在「成员 ∪ 配置」上判歧义不同，理由是
	// 加入不花钱、成功时回显全名、/leave 能撤回；/ask 挑错一个就是一次入场费。
	resolved, err := resolveAgentRef(seg, candidates)
	if err == nil {
		return resolved, ""
	}
	var refErr *agentRefError
	if errors.As(err, &refErr) && len(refErr.Matches) >= 2 {
		return "", fmt.Sprintf("%s 有歧义，可能是：%s\n", seg, strings.Join(refErr.Matches, ", "))
	}
	return "", ""
}

// addSegmentNotFound 是一段在配置里没命中（resolveAddConfig 两个返回值都是空串）、
// 开关打开时在识别快照里也没对上时的说法，一整行，含换行。
//
// 没找到时区分「以它开头的都已经在场」与「根本没有」：前者用户多半是
// 想加一个已经在的人，告诉他不用加了，比说「配置里没有」准确。
// 不在场的总结成员也以它开头时（OB1），先说总结成员：它不是候选，
// 说「都已经在当前会话里了」是假话。加入过又退出的，runSummary 不会再把它
// 拉回来，照 runSummary 那句指 /join，不说「它会自己加入」。
//
// 开关打开时（AD1）最后一句换成同时指出两条路的那一句：不带参数的 /add 列出两边，
// 识别表里的模型可以直接 /add 它的 ID。前两句不变，而且排在识别快照之后
// （addname.go 先在快照里找，对不上才到这里）。
func addSegmentNotFound(r *REPL, seg string, active, member map[string]bool) string {
	if sumID := r.cfg.Summary.Agent; sumID != "" && !active[sumID] && strings.HasPrefix(sumID, seg) {
		if member[sumID] {
			return fmt.Sprintf("%s 开头的除总结成员 %s 外没有可加入的；%s 已退出当前会话，要恢复总结用 /join %s\n", seg, sumID, sumID, sumID)
		}
		return fmt.Sprintf("%s 开头的除总结成员 %s 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结\n", seg, sumID)
	}
	for aid := range active {
		if strings.HasPrefix(aid, seg) {
			return fmt.Sprintf("%s 开头的都已经在当前会话里了\n", seg)
		}
	}
	if r.addByName {
		return fmt.Sprintf(msgAddNameNotFound, seg)
	}
	return fmt.Sprintf("配置里没有可加入的 %s，不带参数的 /add 能列出可加的\n", seg)
}

// isASCIIDigits 报告 s 非空且每个字节都是 ASCII 数字。不用 unicode.IsDigit：
// 全角数字、阿拉伯-印度数字都算 digit，strconv.Atoi 却不认，会把一个
// 看上去是编号的段报成「超出范围」。
func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// cmdLeave 让一个 agent 退出当前会话。退出是软删除，它说过的话仍在库里（规格 G）。
//
// 这里故意不查配置：一个 agent 从配置里被删掉之后，仍然可能是某个旧会话的活跃成员，
// 那时更需要能把它退掉。id 在不在这个会话里，由 Service 去判断并回 ErrParticipantNotFound。
func cmdLeave(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) != 1 {
		return r.printf("用法: /leave <agent-id>\n")
	}
	agentID := args.fields[0]

	if err := r.svc.Leave(ctx, r.cur.ID, agentID, time.Now().UTC()); err != nil {
		return r.reportErr("退出失败", err)
	}
	return r.printf("%s 已退出会话 %s\n", agentID, r.cur.ID)
}

// cmdWho 列出当前会话的成员，含已退出的：退出是软删除，看不到它们就看不出「谁曾经在」。
// 保留 sender "user" 不是 participant，所以不会出现在这张表里（规格 G 第 3 条）。
//
// CONFIG_BRAIN 一列取自**配置**，不是取自库：它答的是「这个 agent 现在配置成什么脑子」。
// 这和 /log 的 BRAIN 列不是一回事 —— 那一列是每条消息当时记下的脑子。
// 两者本来就会不一致（改一次配置，这里立刻变，/log 里的历史一条都不动），
// 所以列名刻意不同；把它们看成同一件事正是 docs/08_STEP5_SPEC.md H 节要消灭的陷阱。
//
// 同一张表里还并排着两个时点不同的 provider，所以第二列叫 JOINED_PROVIDER 而不是
// PROVIDER：它取自 participants 行，是 /join 当时从配置抄下来的快照，
// 只有重新 /join 才会刷新；CONFIG_BRAIN 的第一段取自配置、是此刻的值。
// 两列都不标时点的话，这张表自己就成了它本该消灭的那个陷阱。
func cmdWho(ctx context.Context, r *REPL, _ cmdArgs) error {
	ps, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return r.reportErr("列出成员失败", err)
	}
	if len(ps) == 0 {
		return r.printf("会话 %s 还没有成员，用 /join <agent-id> 加一个\n", r.cur.ID)
	}

	// 返回顺序不在契约里，这里按 agent id 排一遍，输出才稳定。
	sort.Slice(ps, func(i, j int) bool { return ps[i].AgentID < ps[j].AgentID })

	tw := r.newTabWriter()
	fmt.Fprintln(tw, "AGENT\tJOINED_PROVIDER\tACTIVE\tJOINED_AT\tLEFT_AT\tCONFIG_BRAIN")
	for _, p := range ps {
		left := emptyCell
		if p.LeftAt != nil {
			left = formatTime(*p.LeftAt)
		}
		// 配置里已经没有这个 agent 时，取到的是零值 AgentConfig，brainCell 给出占位符。
		// 不报错：它仍然是这个会话的成员，仍然要显示（理由同 cmdLeave 的注释）。
		ac := r.cfg.Agents[p.AgentID]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			p.AgentID,
			orPlaceholder(p.Provider),
			boolCell(p.Active),
			formatTime(p.JoinedAt),
			left,
			brainCell(ac.Provider, ac.Model, ac.Effort),
		)
	}
	return tw.Flush()
}

// cmdSay 以保留 sender "user" 的身份往当前会话追加一条消息。
//
// 它同时是裸文本发言的处理函数（见 dispatch 与 sayCmd）：用户直接敲一行文本走的是这里，
// 敲 /say <文本> 走的也是这里。/say 在裸文本能发言之后保留下来，是因为它还兼作转义出口 ——
// 要发一句以 / 开头的字面量就写 /say /foo，发出去的正文是 /foo 而不是被当成命令。
//
// 不带参数时它还有第三个用途：**进入多行输入模式**。读循环是按行切的，
// 粘贴一段三行的需求会变成三条各自独立的永久消息，而 Canonical Conversation
// 只追加不回收。多行模式把一次粘贴收成一条消息，见 collectDraft。
//
// Recipients 留空表示广播给当前活跃成员（规格 G 第 4 条），
// 收件人是读的时候算的，消息行里存的仍是空数组。
// ID、Seq、CreatedAt 由 store 在事务里填，这里不传。
func cmdSay(ctx context.Context, r *REPL, args cmdArgs) error {
	text := args.rest
	if text == "" {
		// /say 不带参数 = 进入多行输入模式。
		// 裸文本那条路的 rest 永远非空（dispatch 已经把空行挡在外面），
		// 所以能走到这里的一定是用户显式敲了一个光秃秃的 /say。
		// 进模式前的两个前置条件（持久化就绪、已选会话）由 runCommand 查过了，
		// 而 draft 收集期间没有任何命令能改动它们，所以发送时不必重查。
		r.inDraft = true
		r.draft = nil
		return r.printf("%s\n", msgDraftEnter)
	}
	// args.bare 分开两条路：裸文本在自动回复打开时落库之后问一轮，/say <文本> 永远只记下（#47）。
	return sayText(ctx, r, text, args.bare)
}

// cmdLog 按 seq 升序打印当前会话最近的 n 条消息，n 默认 20。
//
// BRAIN 一列是**这条消息当时记录的**脑子，直接取自消息行，不查配置。
// 这是用户要的那件事：不同模型、不同思考强度说的话要能分得出来。
// 它和 /who 的 CONFIG_BRAIN 不是一回事 —— 那一列是配置现在的样子。
// 换 model 或 effort 要重建会话（D-4），所以同一个 agent 的历史消息里
// 完全可能出现不同的 BRAIN，那不是 bug，正是这一列存在的意义。
// user 消息这一列是占位符：人没有脑子可记。
//
// 不打印 thought：默认存但默认不路由（04 节 3），存了不等于要往人脸上糊。
// 要看 thought 是另一条路（将来的导出或审计命令），不是日常的 /log。
// 打的是「thought 被丢了」这个标记，不是 thought 本身。
//
// TEXT 是最后一格，末尾按固定顺序追加三种标记：event、thought_dropped、
// denied_actions（logMetaMarks）。标记接在 orPlaceholder 的结果之后（正文或
// 占位符 - 之后都一样），不进 orPlaceholder 的参数；没有标记时这一格
// 与从前逐字节相同。不加列：tabwriter 按 rune 数算宽，可能含中文的字段必须
// 放最后一列，后面不能再有列被它撑歪；新列放在 TEXT 之后会被中文撑歪，放在
// TEXT 之前会让没有标记的行也多一列占位，并改变表头。denied_actions 不印
// 条数、不印 action 名：屏幕上那一次提示已经把这个长度钉成下界而不是清点，
// 这里再印一个数会被当成清点。
func cmdLog(ctx context.Context, r *REPL, args cmdArgs) error {
	limit := defaultLogLimit
	switch len(args.fields) {
	case 0:
	case 1:
		n, err := strconv.Atoi(args.fields[0])
		if err != nil || n <= 0 {
			return r.printf("条数要是正整数，用法: /log [n]\n")
		}
		limit = n
	default:
		return r.printf("用法: /log [n]\n")
	}

	// 「最近 n 条」= 先问最大 seq，再从 max-n 之后读 n 条。
	// 读法本身是游标读（04 节 1），只是这里的游标是临时算出来的，不落库。
	maxSeq, err := r.svc.MaxSeq(ctx, r.cur.ID)
	if err != nil {
		return r.reportErr("读会话进度失败", err)
	}
	after := maxSeq - int64(limit)
	if after < 0 {
		after = 0
	}

	msgs, err := r.svc.History(ctx, r.cur.ID, after, limit)
	if err != nil {
		return r.reportErr("读消息失败", err)
	}
	if len(msgs) == 0 {
		return r.printf("会话 %s 还没有消息，用 /say <文本> 说点什么\n", r.cur.ID)
	}

	tw := r.newTabWriter()
	fmt.Fprintln(tw, "SEQ\tSENDER\tKIND\tTO\tBRAIN\tTEXT")
	for _, m := range msgs {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n",
			m.Seq,
			m.SenderID,
			orPlaceholder(string(m.Kind)),
			orPlaceholder(strings.Join(m.Recipients, ", ")),
			brainCell(m.SenderProvider, m.SenderModel, m.SenderEffort),
			orPlaceholder(oneLine(m.Text))+logMetaMarks(m.Meta),
		)
	}
	return tw.Flush()
}

// logMetaMarks 把消息 Meta 里的三种标记拼成一段尾巴，按 event、
// thought_dropped、denied_actions 的固定顺序，每命中一件前面加一个 ASCII 空格。
// 都不命中返回空串。不看消息的 Kind。
//
// 同一个键在两种读法下形状不同，所以 denied_actions 用 reflect 看是不是
// 非空切片，而不是对 []any 做类型断言。真库经 JSON 读回：event 是 string，
// thought_dropped 是 bool，denied_actions 是 []any。repl 的假 store 不经
// JSON，denied_actions 原样是 []provider.DeniedAction。只认其中一种，
// 另一种上标记会静默消失。
//
// meta 为 nil 时返回空串（对 nil map 取下标合法）。今天没有任何写入路径
// 会把 nil 传进来：真库把 nil 编成空对象，读回空串、null 一律是空 map；
// 假 store 在追加时把 nil 换成空 map。
func logMetaMarks(meta map[string]any) string {
	var b strings.Builder
	if ev, ok := meta[turn.MetaEvent].(string); ok {
		ev = oneLine(ev)
		if ev != "" {
			b.WriteString(" [event=")
			b.WriteString(ev)
			b.WriteString("]")
		}
	}
	if dropped, ok := meta[conversation.MetaThoughtDropped].(bool); ok && dropped {
		b.WriteString(" [thought_dropped]")
	}
	v := reflect.ValueOf(meta[turn.MetaDeniedActions])
	if v.Kind() == reflect.Slice && v.Len() > 0 {
		b.WriteString(" [denied_actions]")
	}
	return b.String()
}

// currentMark 返回 /conv 首列的标记：当前会话是星号，其余是占位符。
func (r *REPL) currentMark(convID string) string {
	if r.cur != nil && r.cur.ID == convID {
		return currentCell
	}
	return emptyCell
}

// sortedKeys 返回 map 的键的字典序切片。
// map 遍历顺序随机，任何要输出的遍历都必须先过这里，否则测试会随机失败。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// orPlaceholder 把空字符串换成占位符，保证表格每一列都有内容。
func orPlaceholder(s string) string {
	if s == "" {
		return emptyCell
	}
	return s
}

// boolCell 把布尔值渲染成表格里的一列。用 yes / no 而不是 true / false，
// 是因为这张表是给人看的，不是给解析器看的。
func boolCell(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// brainCell 把 provider / model / effort 渲染成表格里的一格，形如 claude/opus/high。
// /log 用它渲染消息里记的脑子，/who 用它渲染配置里当前的脑子。
//
// 三段全空时整格是占位符 —— user 消息本来就没有脑子，配置里已经删掉的 agent 也没有。
// 只缺其中一段时只有那一段是占位符（例如 claude/opus/-）：有的 provider 根本没有
// effort 这个概念（docs/02_AGENT_PROVIDER_ARCHITECTURE.md 第 1.1 节），
// 那是「不适用」，不是「数据丢了」，两者要能分开看。
//
// 用斜杠连成一格而不是拆三列：这三样总是一起看的，拆开会把 /log 撑到五列以上，
// 而且中间留空格容易让人误以为是三列。
// 这一列按字节算宽度，配置里的 provider / model / effort 都是 ASCII id，
// 真往里写中文会把后面的列撑歪 —— 和 /agents 不显示 display_name 是同一个原因。
func brainCell(provider, model, effort string) string {
	if provider == "" && model == "" && effort == "" {
		return emptyCell
	}
	return orPlaceholder(provider) + "/" + orPlaceholder(model) + "/" + orPlaceholder(effort)
}

// formatTime 按 UTC 的 RFC3339 渲染时间。库里存的是 RFC3339Nano，
// 纳秒对人没有意义，显示时截到秒；时区强制 UTC，和存储保持一致。
func formatTime(t time.Time) string {
	if t.IsZero() {
		return emptyCell
	}
	return t.UTC().Format(time.RFC3339)
}

// oneLineReplacer 把 tabwriter 会当成分隔符的字符统统换成空格。
//
// 五个都要管，少一个就漏：text/tabwriter 的 Write 里那一行是
// case '\t', '\v', '\n', '\f' —— \t 与 \v 终结当前单元格，
// \n 与 \f 终结当前行，而 \f 还会强制 Flush、终结整个对齐段落。
// 漏掉 \v 的后果是正文被静默吞掉一个字符、显示的与库里存的不是同一个字符串；
// 漏掉 \f 的后果是一条消息被拆成两行、后半句落在 SEQ 列的位置上，
// 且 \f 前后两截表格各算各的列宽，整张表列不齐。
// strings.TrimSpace 挡不住它们：它只削首尾，正文中间的照单全收。
//
// 这道关卡从 Step 5 起才真正吃重：裸文本即发言之后，用户粘贴来的任意字符
// （例如从 PDF 复制来的分页符 U+000C）都会经 messages.text 走到这里。
var oneLineReplacer = strings.NewReplacer(
	"\r\n", " ",
	"\r", " ",
	"\n", " ",
	"\t", " ",
	"\v", " ",
	"\f", " ",
)

// oneLine 把一段可能多行的文本压成一行，供表格的最后一列使用。
func oneLine(s string) string {
	return strings.TrimSpace(oneLineReplacer.Replace(s))
}
