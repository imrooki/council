package repl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"council/internal/command"
	"council/internal/conversation"
	"council/internal/i18n"
)

// commandFunc 是一条命令的处理函数。参数形状与执行顺序由 runCommand 统一安排：
// 前置条件先判，通过之后才调它。
type commandFunc func(ctx context.Context, r *REPL, args cmdArgs) error

// cmdEntry 是登记在元数据后面的那一半：前置条件与处理函数。
//
// 前置条件不进 command.Spec：它回答的是「这条命令此刻能不能跑」（持久化起没起来、
// 选没选会话、provider 调用就绪没有），而 GUI 面板与 dsh 工具只列命令、不判这些。
// 但它必须和元数据在同一次 Register 里写，否则「加命令时漏改一处」的老问题会换个地方
// 复发：说明与别名进了注册表，前置条件还留在另一张表里。
//
// 名字与说明不在这里 —— 那是 Registry 的键，只写一份。
type cmdEntry struct {
	needsSvc    bool // 需要 conversation.Service
	needsConv   bool // 需要当前会话（蕴含 needsSvc）
	needsRunner bool // 需要 *turn.Runner；nil 表示 provider 调用未就绪
	run         commandFunc
}

// registry 是 repl 这一侧的注册表实例化：元数据用 command.Spec，负载是表项。
// 处理函数的形状是本包的私事，command 包不需要认识 *REPL。
type registry = command.Registry[cmdEntry]

// newCommandRegistry 构造命令表。
//
// 这里是命令的唯一出处：/help、行式 REPL 与 TUI 的命令分发、council commands -json
// 读的都是同一份表。加一条命令就是在下面加一次 Register，元数据、别名与前置条件
// 写在同一个地方，不会走失同步。
//
// 表按 REPL 构造一次，不写成包级变量：注册表构造完就是只读快照，每个 REPL 一份，
// 测试之间不会共享状态；构造一次的开销可以忽略。
func newCommandRegistry() *registry {
	reg := &registry{}

	// 不需要持久化的这几条排在最前：svc 为 nil 时它们照常可用（见 New 的注释）。
	reg.Register(command.Spec{Name: "help", Summary: "列出所有命令"}, cmdEntry{run: cmdHelp})
	// /exit 是 /quit 的别名（Claude Code 的习惯叫法），语义完全相同。
	// /clear 指向 /new 被明确否决：清上下文与建新会话是两件事，别名会让人按错的语义用。
	reg.Register(command.Spec{Name: "quit", Aliases: []string{"exit"}, Summary: "退出 council"}, cmdEntry{run: cmdQuit})
	reg.Register(command.Spec{Name: "providers", Summary: "列出已配置的 provider"}, cmdEntry{run: cmdProviders})
	reg.Register(command.Spec{Name: "agents", Summary: "列出已配置的 agent"}, cmdEntry{run: cmdAgents})
	reg.Register(command.Spec{Name: "teams", Summary: "列出已配置的 team"}, cmdEntry{run: cmdTeams})
	// /skills 只读扫描本地 .claude/skills 的元数据，不碰持久化与 provider，
	// 所以放在这一组里：svc 为 nil 时照常可用。它不执行 skill、不注入正文，
	// 类型与扫描口径直接复用 internal/serve 那一套（config.Skill）。
	reg.Register(command.Spec{Name: "skills", Summary: "列出 .claude/skills 扫到的 skill（只读，不执行、不注入）"}, cmdEntry{run: cmdSkills})
	reg.Register(command.Spec{Name: "team", Args: "use <team-id>", Summary: "绑定当前会话到一个 team，并把尚未加入的成员（总结成员除外）一次性加进来: /team use <team-id>"}, cmdEntry{needsSvc: true, needsConv: true, run: cmdTeam})

	reg.Register(command.Spec{Name: "new", Args: "<标题>", Summary: "建一个会话并切过去: /new <标题>"}, cmdEntry{needsSvc: true, run: cmdNew})
	// /copy 的正文由 TUI 那一侧取（见 CopyRecent）：剪贴板是终端的事，行式入口只有一个
	// io.Writer，写不了它。参数形状、解析与「哪些消息算可复制的正文」仍然只有这一份。
	// 标 ScopeTUI 是因为它能生效的入口只有 TUI（行式入口落一句说明）；GUI 面板与
	// dsh 工具拿到的命令目录里因此不会出现一条它们执行不了复制的东西。
	reg.Register(command.Spec{Name: "copy", Args: "[n]", Scopes: []command.Scope{command.ScopeTUI}, Summary: "复制最近 n 条成员或你的发言到剪贴板: /copy [n]（不带参数是 1 条）"}, cmdEntry{run: cmdCopy})
	reg.Register(command.Spec{Name: "conv", Summary: "列出全部会话，标出当前那个"}, cmdEntry{needsSvc: true, run: cmdConv})
	reg.Register(command.Spec{Name: "use", Args: "<id 或 id 前缀>", Summary: "切换当前会话: /use <id 或 id 前缀>"}, cmdEntry{needsSvc: true, run: cmdUse})
	reg.Register(command.Spec{Name: "resume", Args: "[--all] [--archived] [<编号> | <id 前缀>]", Summary: "列出本机会话并接上: /resume 列出当前项目的会话；/resume --all 列出全部；/resume --archived 列出本项目的归档会话（标题后标 [已归档]）；/resume --all --archived 列出全部归档会话；/resume [--all] [--archived] <编号> 按列表编号接上；/resume [--archived] <id 前缀> 按 id 接上（同 /use，并回显概况）"}, cmdEntry{needsSvc: true, run: cmdResume})
	reg.Register(command.Spec{Name: "add", Args: "<全名|编号|前缀>[,...] | --refresh", Summary: "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别"}, cmdEntry{needsSvc: true, needsConv: true, run: cmdAdd})
	reg.Register(command.Spec{Name: "join", Args: "同 /add", Summary: "同 /add"}, cmdEntry{needsSvc: true, needsConv: true, run: cmdAdd})
	reg.Register(command.Spec{Name: "leave", Args: "<agent-id>", Summary: "让 agent 退出当前会话: /leave <agent-id>"}, cmdEntry{needsSvc: true, needsConv: true, run: cmdLeave})
	reg.Register(command.Spec{Name: "who", Summary: "列出当前会话的成员"}, cmdEntry{needsSvc: true, needsConv: true, run: cmdWho})
	// say 与 round 的表项单独命名，是因为裸文本、粘贴与自动一轮复用的正是它们，
	// 见 commands.go 里 sayCmd / roundCmd 的注释。
	reg.Register(command.Spec{Name: "say", Args: "[<文本>]", Summary: "只记下、不让任何人回复: /say <文本>；/say /foo 发字面量 /foo；/say 不带参数进多行输入（/end 发送时与直接打字相同）"}, sayCmd)
	reg.Register(command.Spec{Name: "to", Args: "<id>[,<id>...] <文本>", Summary: "定向发言，只有列出的成员会看到: /to <id>[,<id>...] <文本>"}, cmdEntry{needsSvc: true, needsConv: true, run: cmdTo})
	reg.Register(command.Spec{Name: "log", Args: "[n]", Summary: "按 seq 升序打印当前会话最近的消息: /log [n]"}, cmdEntry{needsSvc: true, needsConv: true, run: cmdLog})
	reg.Register(command.Spec{Name: "cost", Args: "[<agent-id>] | --round <round-id>", Summary: "按成员汇总当前会话的账: /cost [<agent-id>]；/cost --round <round-id> 只看那一轮"}, cmdEntry{needsSvc: true, needsConv: true, run: cmdCost})
	reg.Register(command.Spec{Name: "ask", Args: "<agent-id> [文本]", Summary: "让一个成员说一轮: /ask <agent-id> [文本]"}, cmdEntry{needsSvc: true, needsConv: true, needsRunner: true, run: cmdAsk})
	reg.Register(command.Spec{Name: "round", Args: "[id...] [--go] [--rounds N] [--with-judge] [--no-summary]", Summary: "按名单问一轮，--go 时发言者同时起进程；多个成员跑完自动总结（只给你看）: /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]"}, roundCmd)
	reg.Register(command.Spec{Name: "session", Args: "[rebuild <agent-id>]", Summary: "列出当前会话每个成员的 provider 会话；/session rebuild <agent-id> 先结清这个成员留在库里的陈旧在飞记录，再丢弃并重建"}, cmdEntry{needsSvc: true, needsConv: true, needsRunner: true, run: cmdSession})
	reg.Register(command.Spec{Name: "rules", Args: "[reload [--rebuild]]", Summary: "显示规则文件与逐 provider 的指令环境；/rules reload [--rebuild] 重载规则文件"}, cmdEntry{needsRunner: true, run: cmdRules})
	reg.Register(command.Spec{Name: "config", Args: "[reload [--apply]]", Summary: "显示当前配置文件路径与 agent / provider / team 计数；/config reload [--apply] 重载配置文件"}, cmdEntry{run: cmdConfig})
	reg.Register(command.Spec{Name: "verbose", Args: "[on|off]", Summary: "切换输出档位: /verbose 在简洁与详细之间切换；/verbose on 详细；/verbose off 简洁"}, cmdEntry{run: cmdVerbose})
	reg.Register(command.Spec{Name: "auto", Args: "[on|off]", Summary: "切换直接打字要不要让成员回复: /auto 切换；/auto on 自动回复（点名了谁就只问谁，没点名问全部成员）；/auto off 只记下"}, cmdEntry{run: cmdAuto})

	return reg
}

// ─── /copy ──────────────────────────────────────────────────────────────
//
// /copy 是唯一一条结果不在屏幕上的命令：它要的是终端剪贴板。剪贴板只有 TUI 有
// （tea.SetClipboard 是一条往终端写 OSC52 的 Cmd），而 REPL 的输出只有 io.Writer，
// 命令处理函数拿不到 Program。所以分工是：
//   - 这里：命令元数据、参数解析、从当前会话取最近 n 条可复制的正文（CopyRecent）；
//   - TUI：在提交之前截下这条命令，取到正文之后自己写剪贴板、自己改状态栏。
//
// 行式入口（cmdCopy）没有剪贴板，只回一句说明，不假装复制成功。

const (
	// CopyUsage 是 /copy 参数非法时的提示行。与其余命令的「用法: …」同一体例。
	CopyUsage = "用法: /copy [n]"
	// MsgCopyNoContent 是没有任何可复制正文时的答复。
	MsgCopyNoContent = "没有可复制的消息"
	// MsgCopyNotInTUI 是行式入口敲 /copy 的答复。剪贴板只在终端 UI 那一侧存在。
	MsgCopyNotInTUI = "行式入口没有剪贴板，/copy 只在 TUI 里可用"
)

var (
	// ErrCopyUsage 是 /copy 参数非法（负数、0、非数字、多给一个词）的哨兵。
	// TUI 把它当成命令参数错误打到回滚区 —— 与其余命令的用法提示同一个位置。
	ErrCopyUsage = errors.New(CopyUsage)
	// ErrCopyEmpty 是「没有可复制的消息」的哨兵。TUI 把它放状态栏：那是正常结果，
	// 不是命令用错了，不该混在一行行的输出里。
	ErrCopyEmpty = errors.New(MsgCopyNoContent)
)

// copyPage 是往回翻页时一次覆盖的 seq 范围宽度。它只影响读几次库：
// 一段里全是系统面包屑时窗口翻倍，正常情况下一条命令一两次查询就够。
const copyPage = 50

// cmdCopy 是 /copy 在行式入口的处理函数。
//
// 它不去取正文：行式路径的出口只有一个 io.Writer，写不了剪贴板；把正文糊在屏幕上
// 又不是「复制」。所以只说明这件事，与 /say 空参在两条入口上的分叉同一个体例
// （见 seam.go 里 msgDraftNotInTUI 那段注释）。
func cmdCopy(_ context.Context, r *REPL, _ cmdArgs) error {
	return r.printf("%s\n", r.t("tui.copy.notInTUI", MsgCopyNotInTUI))
}

// CopyRecent 取最近 n 条可复制的会话消息正文，供 TUI 的 /copy 直接调用。
//
// args 是命令名之后的原始参数：空表示 1 条，其余必须是正整数。
// 成功时返回按时间正序、两条之间空一行的正文（调用方写剪贴板、按 rune 计数）；
// 参数非法返回 ErrCopyUsage，没有可复制的正文返回 ErrCopyEmpty，
// 其余失败返回一句可以直接显示给用户的中文错误。
//
// 前两道前置条件与 runCommand 里 needsSvc / needsConv 的答复逐字相同：
// TUI 的 /copy 不进 runCommand（它不交给 Submit，见 Model.beginSubmit），
// 所以这里必须自己判一遍，否则 svc 或 cur 为 nil 时会在下面空指针。
func (r *REPL) CopyRecent(ctx context.Context, args string) (string, error) {
	if r.svc == nil {
		return "", errors.New(r.t("tui.repl.noStore", msgNoStore))
	}
	if r.cur == nil {
		return "", errors.New(r.t("tui.repl.noConv", msgNoConv))
	}
	n, err := parseCopyCount(args)
	if err != nil {
		return "", err
	}
	msgs, err := r.recentCopyable(ctx, r.cur.ID, n)
	if err != nil {
		return "", err
	}
	if len(msgs) == 0 {
		return "", ErrCopyEmpty
	}
	parts := make([]string, len(msgs))
	for i, m := range msgs {
		parts[i] = m.Text
	}
	return strings.Join(parts, "\n\n"), nil
}

// parseCopyCount 解析 /copy 的条数。缺省是 1；负数、0、非数字、多给一个词都是用法错误。
// 用 Atoi 而不是 ParseInt：条数没有有意义的上限，溢出时 Atoi 自己就会报错。
func parseCopyCount(args string) (int, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return 1, nil
	}
	// 只接受一个词：/copy 3 4 与 /copy 3x 都是「用法不对」。悄悄取第一个词
	// 会把用户写错的东西当成另一条命令执行，正是命令参数错误要拦的那类误读。
	if len(strings.Fields(args)) != 1 {
		return 0, ErrCopyUsage
	}
	n, err := strconv.Atoi(args)
	if err != nil || n <= 0 {
		return 0, ErrCopyUsage
	}
	return n, nil
}

// recentCopyable 从最新往回找够 want 条可复制的消息，按时间正序返回。
//
// 为什么要翻页：History 只按 seq 正序读（conversation.Service.History），
// 「最近 n 条」得从 maxSeq 往回落。一次读一整段 seq 范围（limit 恰好等于范围宽度），
// 段里全是系统面包屑时把窗口翻倍再往前找：既不会漏读，也不会为了最后一条消息
// 把整场会话读进内存。
func (r *REPL) recentCopyable(ctx context.Context, convID string, want int) ([]conversation.Message, error) {
	maxSeq, err := r.svc.MaxSeq(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("%s%w", r.t("tui.err.readProgressPrefix", "读会话进度失败: "), err)
	}
	picked := make([]conversation.Message, 0, min(want, copyPage)) // 从新到旧，最后再反过来
	after, window := maxSeq, int64(copyPage)
	for after > 0 && len(picked) < want {
		from := max(0, after-window)
		msgs, err := r.svc.History(ctx, convID, from, int(after-from))
		if err != nil {
			return nil, fmt.Errorf("%s%w", r.t("tui.err.readMessagesPrefix", "读消息失败: "), err)
		}
		for i := len(msgs) - 1; i >= 0 && len(picked) < want; i-- {
			if copyableMessage(msgs[i]) {
				picked = append(picked, msgs[i])
			}
		}
		after = from
		window *= 2
	}
	for i, j := 0, len(picked)-1; i < j; i, j = i+1, j-1 {
		picked[i], picked[j] = picked[j], picked[i]
	}
	return picked, nil
}

// copyableMessage 报告一条消息算不算「可复制的正文」：成员说的话与用户说的话。
// system 是 Council 自己的面包屑（会话建立、一轮失败这类事件），task / task_result
// 是工具行，它们都不进剪贴板 —— 粘出去的那一段应当只有人读得懂的正文。
// 正文为空的消息也跳过，否则两条正文之间会凭空空出两行。
func copyableMessage(m conversation.Message) bool {
	if m.Kind != conversation.KindUser && m.Kind != conversation.KindAgent {
		return false
	}
	return strings.TrimSpace(m.Text) != ""
}

// CommandSpecs 返回命令目录的全部条目，按名字字典序。
//
// 它不建 REPL、不读配置、不碰库：council commands -json 与将来的 GUI 面板
// 只需要这一份静态元数据，不该为列一遍命令付出启动一个 REPL 的代价。
// 这里**不按入口过滤**：JSON 的消费者要的是命令目录本身。入口可见集用
// TUISpecs（/help 与纯文本输出）或 List(ScopeGUI)（将来的面板）自己取。
func CommandSpecs() []command.Spec {
	return newCommandRegistry().ListAll()
}

// TUISpecs 返回 /help 该列的命令，按名字字典序。
//
// 单独一个函数，是为了让「council commands 的纯文本输出与 /help 相同」这件事
// 在类型上成立，而不是靠今天所有命令都恰好三个入口通用。将来出现 TUI 专有命令时，
// 纯文本输出走这里才不会和 /help 分叉。
func TUISpecs() []command.Spec {
	return newCommandRegistry().List(command.ScopeTUI)
}

// WriteCommandTable 按 /help 的列格式把 specs 写成给人看的表。
//
// /help 与 council commands（不带 -json）共用它：表头、列宽与顺序只有这一份定义，
// 两处的输出因此逐字节相同，不会出现「命令行上看到的和 /help 里看到的不是同一张表」。
// 列宽由 newTabWriter 决定，与其余列表命令一致。
func WriteCommandTable(w io.Writer, specs []command.Spec) error {
	tw := newTabWriter(w)
	// 表头用 ASCII，和其余几张表保持一致；说明文字是最后一列，中文不影响对齐。
	fmt.Fprintln(tw, "COMMAND\tSUMMARY")
	for _, spec := range specs {
		fmt.Fprintf(tw, "/%s\t%s\n", spec.Name, spec.Summary)
	}
	return tw.Flush()
}

// WriteCommandTableLang 与 WriteCommandTable 同一张表，只是先把摘要按语言
// 替换掉：key 与 /help 共用 tui.cmd.<name>.summary，fallback 是注册表里的
// 中文摘要，所以中文（或缺条）时输出与 WriteCommandTable 逐字节相同。
//
// council commands 的纯文本输出走它；-json 不走，元数据契约里的 summary
// 始终是注册表原值。
func WriteCommandTableLang(w io.Writer, specs []command.Spec, lang i18n.Lang) error {
	localized := make([]command.Spec, len(specs))
	copy(localized, specs)
	for i := range localized {
		localized[i].Summary = i18n.TF(lang, "tui.cmd."+localized[i].Name+".summary", localized[i].Summary)
	}
	return WriteCommandTable(w, localized)
}
