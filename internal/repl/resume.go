package repl

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"council/internal/conversation"
	"council/internal/i18n"
)

const (
	// resumeTailLimit 是接上后回显最近几条消息。再多会把屏幕刷走，
	// 完整历史仍走 /log。
	resumeTailLimit = 6
	// resumeTextLimit 是回显的每条消息正文最多多少个字符（rune）。
	// 按 rune 截，避免把一个汉字切成半个。
	resumeTextLimit = 100
)

// resumeUsage 是参数不对时的用法行。
// --archived 排在 --all 之后：两个 flag 管的都是「看哪一批」，
// 连在一起写，用户才看得出 --all --archived 是跨项目的归档视图。
const resumeUsage = "用法: /resume [--all] [--archived] [<编号|id 前缀>]"

// resumeArchivedMark 是归档视图里打在每条标题后面的标记。
// 用 ASCII 方括号配中文，与 [thought_dropped] 那类标记同一个体例；
// 标题在最后一列，标记不会把后面的列撑歪。
const resumeArchivedMark = " [已归档]"

// NormalizeProjectDir 把启动目录收成可比较的绝对路径。
// 同一个目录有大小写、末尾分隔符、相对路径、符号链接多种写法，
// 会话的归属判断和 /resume 的过滤都靠字符串比较，不规范化会把同一个项目判成两个。
// 纯函数：不创建目录、不读目录内容。
//
// 空串返回空串。Abs 失败（Unix 上是取不到当前目录，Windows 上是路径串本身非法）
// 也返回空串：宁可不绑，也不存相对路径或半截路径。
// EvalSymlinks 失败（目录不存在、没有权限）就继续用 Abs 的结果。
// Clean 去掉末尾分隔符，根目录除外（C:\ 和 / 不变）。
// Windows 上把盘符改成大写，d:\a 与 D:\a 才是同一个目录。
func NormalizeProjectDir(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	abs = filepath.Clean(abs)
	if runtime.GOOS == "windows" && len(abs) >= 2 && abs[1] == ':' {
		if abs[0] >= 'a' && abs[0] <= 'z' {
			abs = string(abs[0]-'a'+'A') + abs[1:]
		}
	}
	return abs
}

// sameProjectDir 判断两个已经规范化过的项目目录是不是同一个。
// 两者都非空才算；Windows 下大小写不敏感，其它平台按字节比较。
// 这里不再做规范化，调用方传的都应是 NormalizeProjectDir 的结果。
func sameProjectDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// SameProjectDir 与 /resume 的过滤是同一个口径，横幅数字才和 /resume 列出的一致。
func SameProjectDir(a, b string) bool {
	return sameProjectDir(a, b)
}

// SetProjectDir 把启动目录收成规范化绝对路径，存进 r.projectDir。
// 与 SetConcise、SetAutoReply 同一条纪律，只许在 Run 或第一次 Submit 之前调。
// 传入空串、或规范化失败，结果都是空串：/new 不盖章，/resume 列出全部。
func (r *REPL) SetProjectDir(dir string) {
	r.projectDir = NormalizeProjectDir(dir)
}

// resumeRow 是 /resume 列表里的一行。
// members、msgs 用 -1 作哨兵：那一列读失败，打印时换成 ?，不让整个命令失败。
// lastKnown 是布尔：真表示 lastActive 是真实的最后活动时间，可以打到 LAST_ACTIVE。
// lastKnown 为假时 lastActive 只是排序键，取会话的 CreatedAt 兜底，绝不是显示值。
type resumeRow struct {
	conv       conversation.Conversation
	members    int
	msgs       int64
	lastActive time.Time
	lastKnown  bool
	readErrs   []error
}

// resumeList 是一次按项目过滤之后的列表。filtered 与 rows、hiddenUnbound
// 来自同一次计算：表头说明和表列都读它，调用方不要再算一遍是否过滤，
// 否则说明行和列会对不上。
type resumeList struct {
	rows          []resumeRow
	hiddenUnbound int
	filtered      bool
}

// loadResumeRows 取出 /resume 要显示或按编号挑选的会话。
// 无参数列表和按编号选择必须走这同一个函数，两处编号才一致。
//
// archived 决定读哪张表：真读 ArchivedConversations，假读 Conversations。
// 两张表都由 store 分好（过滤条件在 internal/store/read.go 的 SQL 里），
// 这里不再按 c.Archived 自己分一次 —— 同一个条件写两处，早晚会有一处改漏。
//
// filtered 只在没带 --all 且当前有项目目录时为真：只保留 cwd 与 r.projectDir
// 相同的会话。被滤掉的会话里 cwd 为空的个数记入 hiddenUnbound，
// 用来提示还有没绑项目的旧会话。--all，或取不到启动目录时，不过滤。
// 归档视图与默认视图共用这一条过滤：单独 --archived 是「本项目的归档会话」，
// --all --archived 是「全部归档会话」。
//
// 排序在这里做，不改 store：store 的 created_at 倒序是 /conv 与 /use 依赖的契约，
// /resume 要按最后活动排。lastActive 倒序，相同则 CreatedAt 倒序，再相同则 id 升序。
// 最后活动时间没能从库里确定时，lastActive 仍是创建时间，只作排序兜底，
// LAST_ACTIVE 那一格打 ?，不要把这个兜底时间印出去。
func (r *REPL) loadResumeRows(ctx context.Context, all, archived bool) (resumeList, error) {
	convs, err := r.resumeConvs(ctx, archived)
	if err != nil {
		return resumeList{}, err
	}
	filter := !all && r.projectDir != ""
	var hiddenUnbound int
	kept := make([]conversation.Conversation, 0, len(convs))
	for _, c := range convs {
		if filter && !sameProjectDir(c.Cwd, r.projectDir) {
			if c.Cwd == "" {
				hiddenUnbound++
			}
			continue
		}
		kept = append(kept, c)
	}

	rows := make([]resumeRow, 0, len(kept))
	for _, c := range kept {
		row := resumeRow{conv: c, lastActive: c.CreatedAt}
		parts, perr := r.svc.Participants(ctx, c.ID, false)
		if perr != nil {
			row.members = -1
			row.readErrs = append(row.readErrs, perr)
		} else {
			row.members = len(parts)
		}
		maxSeq, merr := r.svc.MaxSeq(ctx, c.ID)
		if merr != nil {
			// 连有没有消息都不知道，最后活动时间不确定。
			row.msgs = -1
			row.readErrs = append(row.readErrs, merr)
		} else {
			row.msgs = maxSeq
			if maxSeq == 0 {
				row.lastKnown = true
			} else {
				tail, herr := r.svc.History(ctx, c.ID, maxSeq-1, 1)
				if herr != nil {
					row.readErrs = append(row.readErrs, herr)
				} else if len(tail) == 0 {
					// MaxSeq 说有消息，History 却一条也没给：库状态对不上，
					// 这一格必须是 ?，表后也要有一句能对上的说明。
					row.readErrs = append(row.readErrs, fmt.Errorf("%s", r.t("tui.resume.tailMissing", "MaxSeq 报告有 %d 条消息，但读不到末条", maxSeq)))
				} else {
					row.lastActive = tail[len(tail)-1].CreatedAt
					row.lastKnown = true
				}
			}
		}
		rows = append(rows, row)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if !a.lastActive.Equal(b.lastActive) {
			return a.lastActive.After(b.lastActive)
		}
		if !a.conv.CreatedAt.Equal(b.conv.CreatedAt) {
			return a.conv.CreatedAt.After(b.conv.CreatedAt)
		}
		return a.conv.ID < b.conv.ID
	})
	return resumeList{rows: rows, hiddenUnbound: hiddenUnbound, filtered: filter}, nil
}

// resumeConvs 按视图取会话列表：归档视图读 ArchivedConversations，默认视图读
// Conversations。两条都是「全库、未删除」的口径，区别只在归档位。
//
// 抽成一个函数，是因为列表与 id 前缀两条路都要它：谁读哪张表只在这里决定一次，
// 不会出现列表是归档的、前缀却在活跃表里找那种对不上的行为。
func (r *REPL) resumeConvs(ctx context.Context, archived bool) ([]conversation.Conversation, error) {
	if archived {
		return r.svc.ArchivedConversations(ctx)
	}
	return r.svc.Conversations(ctx)
}

// resumeListCommand 是提示里那句「先敲 … 看列表」用的命令名。
// 归档视图必须带 --archived：不带的话用户看到的是另一张表，编号对不上，
// 指向错的命令比不指更糟。
func resumeListCommand(archived bool) string {
	if archived {
		return "/resume --archived"
	}
	return "/resume"
}

// cmdResume 列出本机会话，或按编号 / id 前缀接上。
// needsConv 是假：没有当前会话时也要能列和接，否则它自己会答「先 /new 或 /use」。
//
// id 前缀不受项目过滤影响，同 /use：用户给的是完整身份线索，限制范围只会让他困惑。
// 编号每次重新计算：列表会随新消息变序，编号不是稳定句柄。
// 接上是纯读：不触发任何成员调用，不改成员，不写库。
//
// --archived 只换列表来源（归档表而不是默认表），别的都照旧：项目过滤、
// 编号、回显概况与接上走的是同一套函数，归档会话接上后与普通会话没有区别。
func cmdResume(ctx context.Context, r *REPL, args cmdArgs) error {
	all, archived, token, ok := parseResumeArgs(args.fields)
	if !ok {
		return r.printf("%s\n", r.t("tui.resume.usage", resumeUsage))
	}
	if token == "" {
		return r.printResumeList(ctx, all, archived)
	}
	if isResumeIndex(token) {
		return r.resumeByIndex(ctx, all, archived, token)
	}
	return r.resumeByPrefix(ctx, token, archived)
}

// parseResumeArgs 从 fields 里拆出 --all、--archived 和至多一个选择 token。
// 两个 flag 都出现几次都行，位置不限。其余以 -- 开头的 token 是未知 flag。
//
// --archived 与 --all 正交，四种组合都有意义：
// 什么都不带 = 本项目的活跃会话；--all = 全部活跃会话；
// --archived = 本项目的归档会话；--all --archived = 全部归档会话。
// 不带 --archived 时行为与加这个 flag 之前逐字节相同。
func parseResumeArgs(fields []string) (all, archived bool, token string, ok bool) {
	ok = true
	for _, f := range fields {
		switch f {
		case "--all":
			all = true
			continue
		case "--archived":
			archived = true
			continue
		}
		if strings.HasPrefix(f, "--") {
			return false, false, "", false
		}
		if token != "" {
			return false, false, "", false
		}
		token = f
	}
	return all, archived, token, ok
}

// isResumeIndex 判定 token 是不是列表编号：全是 ASCII 十进制数字，且长度不超过 3。
// 更长的纯数字当 id 前缀，避免把一长串 id 误当成编号。
func isResumeIndex(token string) bool {
	if token == "" || len(token) > 3 {
		return false
	}
	for i := 0; i < len(token); i++ {
		if token[i] < '0' || token[i] > '9' {
			return false
		}
	}
	return true
}

// printResumeList 打无参数 /resume 的整段输出。archived 为真时打的是归档视图：
// 列表来自 ArchivedConversations，每条标题后加 resumeArchivedMark。
//
// 表头说明行必须在建 tabwriter 之前或 Flush 之后写。tabwriter 在 Flush
// 之前不输出，说明行若写在两者之间，会跑到表的后面。
// 空列表时不打表头说明和表，只答一句；过滤藏起了未绑定的旧会话时，在这句之后再补一行提示。
//
// PROJECT 列排在 TITLE 之前：TITLE 要留在最后一列，标题可能含中文。
// 路径含中文时 tabwriter 按 rune 算宽，TITLE 一列会略有错位；PROJECT 只在
// --all 或没有项目目录时才有，这点错位是有意接受的。
// 表后各行的顺序（读库失败说明、未绑定提示、编号提示）也是有意的：
// 先说哪几行的数字不可信，再说还有没列出来的旧会话，最后才是怎么接上。
//
// 归档视图与默认视图共用同一条项目过滤、同一张表的列与同一个排序，只有
// 说明、空列表答复、未绑定提示与末尾的编号提示换词：这些提示里都带着命令，
// 不带 --archived 的话用户照着敲会看到另一张表，编号也就对不上。
func (r *REPL) printResumeList(ctx context.Context, all, archived bool) error {
	list, err := r.loadResumeRows(ctx, all, archived)
	if err != nil {
		return r.reportErr(r.t("tui.err.listConvs", "列出会话失败"), err)
	}
	rows, hiddenUnbound, filter := list.rows, list.hiddenUnbound, list.filtered
	n := len(rows)
	if n == 0 {
		var line string
		switch {
		case archived && filter:
			line = r.t("tui.resume.emptyProjectArch", "本项目还没有归档会话，用 /resume --all --archived 看其它项目的")
		case archived:
			line = r.t("tui.resume.emptyArchived", "还没有任何归档会话（在网页里归档的会话会出现在这里）")
		case filter:
			line = r.t("tui.resume.emptyProject", "本项目还没有会话，用 /new <标题> 建一个（/resume --all 看其它项目的）")
		default:
			line = r.t("tui.resume.emptyAll", "还没有任何会话，用 /new <标题> 建一个")
		}
		if err := r.printf("%s\n", line); err != nil {
			return err
		}
		if filter && hiddenUnbound > 0 {
			return r.printf("%s\n", resumeUnboundHint(r.lang, hiddenUnbound, archived))
		}
		return nil
	}

	var head string
	switch {
	case archived && filter:
		head = r.t("tui.resume.headProjectArch", "项目: %s（本项目 %d 个归档会话；/resume --all --archived 看全部）", r.projectDir, n)
	case archived && all:
		head = r.t("tui.resume.headAllArch", "全部归档会话（%d 个）", n)
	case archived:
		head = r.t("tui.resume.headNoProjArch", "没有项目目录，列出全部归档会话（%d 个）", n)
	case filter:
		head = r.t("tui.resume.headProject", "项目: %s（本项目 %d 个会话；/resume --all 看全部）", r.projectDir, n)
	case all:
		head = r.t("tui.resume.headAll", "全部会话（%d 个）", n)
	default:
		head = r.t("tui.resume.headNoProject", "没有项目目录，列出全部会话（%d 个）", n)
	}
	if err := r.printf("%s\n", head); err != nil {
		return err
	}

	tw := r.newTabWriter()
	if filter {
		fmt.Fprintln(tw, "NO\tCUR\tID\tLAST_ACTIVE\tMEMBERS\tMSGS\tTITLE")
	} else {
		fmt.Fprintln(tw, "NO\tCUR\tID\tLAST_ACTIVE\tMEMBERS\tMSGS\tPROJECT\tTITLE")
	}
	for i, row := range rows {
		c := row.conv
		id := c.ID
		if len(id) > 8 {
			id = id[:8]
		}
		cells := []string{
			strconv.Itoa(i + 1),
			r.currentMark(c.ID),
			id,
			resumeLastActive(row),
			resumeCount(row.members),
			resumeCount64(row.msgs),
		}
		if !filter {
			// 与 TITLE 同一道关卡：这一格进 tabwriter，路径里的制表符或换行会把列拆乱。
			// oneLine 之后仍是空串才写 (未绑定)，不要把「有路径」和「没绑」写成同一格。
			proj := oneLine(c.Cwd)
			if proj == "" {
				proj = r.t("tui.resume.unboundCell", "(未绑定)")
			}
			cells = append(cells, proj)
		}
		cells = append(cells, resumeTitleCell(r.lang, c.Title, archived))
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	for _, row := range rows {
		if len(row.readErrs) == 0 {
			continue
		}
		parts := make([]string, len(row.readErrs))
		for i, e := range row.readErrs {
			parts[i] = e.Error()
		}
		if err := r.printf("%s\n", r.t("tui.resume.readFailed", "会话 %s 读取失败（对应列显示 ?）: %s", row.conv.ID, strings.Join(parts, "; "))); err != nil {
			return err
		}
	}
	if filter && hiddenUnbound > 0 {
		if err := r.printf("%s\n", resumeUnboundHint(r.lang, hiddenUnbound, archived)); err != nil {
			return err
		}
	}
	return r.printf("%s\n", resumeNumberHint(r.lang, archived))
}

// resumeTitleCell 是 TITLE 那一格：压成一行、空标题换占位符，归档视图再在末尾加标记。
//
// 标记按视图打，不按 c.Archived 打：归档列表里的每一行都来自 ArchivedConversations，
// 视图本身就是「这些是归档的」这件事的唯一出处。再去看每行的布尔，等于把同一个
// 判定写两遍；两处一旦不一致，同一张表里就会出现有标记、没标记混着的行。
func resumeTitleCell(lang i18n.Lang, title string, archived bool) string {
	cell := orPlaceholder(oneLine(title))
	if archived {
		return cell + i18n.TF(lang, "tui.resume.mark", resumeArchivedMark)
	}
	return cell
}

// resumeUnboundHint 是「另有 N 个未绑定项目的…」那行提示。
// 归档视图指向 --all --archived：照着不带 --archived 的命令敲，看到的是另一张表。
func resumeUnboundHint(lang i18n.Lang, n int, archived bool) string {
	if archived {
		return i18n.TF(lang, "tui.resume.unboundArchived", "另有 %d 个未绑定项目的归档会话，/resume --all --archived 查看", n)
	}
	return i18n.TF(lang, "tui.resume.unbound", "另有 %d 个未绑定项目的旧会话，/resume --all 查看", n)
}

// resumeNumberHint 是列表最后那句「怎么接上」。编号是按当次这张表算的，
// 所以提示里的命令必须带上 --archived，照抄才能接上同一张表里的编号。
func resumeNumberHint(lang i18n.Lang, archived bool) string {
	return i18n.TF(lang, "tui.resume.numberHint", "敲 %s <编号> 接上；编号按最近活动排序，每次重新计算", resumeListCommand(archived))
}

// resumeLastActive 只在 lastKnown 时打时间。未知打一个 ASCII 问号：
// 排序用的创建时间不是显示值，印出去会和表后「对应列显示 ?」对不上。
func resumeLastActive(row resumeRow) string {
	if !row.lastKnown {
		return "?"
	}
	return resumeTime(row.lastActive)
}

// resumeTime 用本地时区、截到分钟。零值打成占位符，避免印出 0001-01-01。
func resumeTime(t time.Time) string {
	if t.IsZero() {
		return emptyCell
	}
	return t.Local().Format("2006-01-02 15:04")
}

// resumeCount 把读失败的哨兵 -1 打成 ?，其余按十进制。不要把 -1 印成数字。
func resumeCount(n int) string {
	if n < 0 {
		return "?"
	}
	return strconv.Itoa(n)
}

// resumeCount64 与 resumeCount 同一条哨兵：msgs 为 -1 时这一列是 ?，不是负数。
func resumeCount64(n int64) string {
	if n < 0 {
		return "?"
	}
	return strconv.FormatInt(n, 10)
}

// resumeByIndex 按编号接上。编号取自与无参数列表同一个 loadResumeRows
// 口径（连 --all / --archived 都一起传下去），越界不回落成 id 前缀：
// 输错一位编号不该悄悄接上别的会话。
//
// 越界提示里的命令按视图给：归档视图下若还提示「先敲 /resume 看列表」，
// 用户看到的是活跃会话那张表，编号只会更对不上。
func (r *REPL) resumeByIndex(ctx context.Context, all, archived bool, token string) error {
	n, err := strconv.Atoi(token)
	if err != nil {
		return r.printf("%s\n", r.t("tui.resume.usage", resumeUsage))
	}
	list, err := r.loadResumeRows(ctx, all, archived)
	if err != nil {
		return r.reportErr(r.t("tui.err.listConvs", "列出会话失败"), err)
	}
	rows := list.rows
	cmd := resumeListCommand(archived)
	if n < 1 || n > len(rows) {
		if len(rows) == 0 {
			return r.printf("%s", r.t("tui.resume.indexOutOfRangeEmpty", "编号 %d 超出范围（当前没有可选的会话），先敲 %s 看列表\n", n, cmd))
		}
		return r.printf("%s", r.t("tui.resume.indexOutOfRange", "编号 %d 超出范围（1..%d），先敲 %s 看列表\n", n, len(rows), cmd))
	}
	return r.attachResume(ctx, rows[n-1].conv)
}

// resumeByPrefix 在全部会话里匹配，不走项目过滤：用户给的是身份线索，
// 收窄到本项目会让别的目录里那个会话永远接不上。
// 完整 id 先于前缀，避免短 id 被更长的前缀撞成多匹配。
// 多匹配只列候选、不切换 r.cur。
//
// archived 只决定在哪张表里找：带 --archived 时找归档表，否则找默认表。
// 两张表互斥，所以不带 --archived 时归档会话的 id 前缀不会命中 —— 这是
// 有意的：要接归档会话就带上 --archived，与列表是同一张表。
// 多匹配的候选表不加归档标记：这张表只有一种来源，没有「混着两种会话」的歧义。
func (r *REPL) resumeByPrefix(ctx context.Context, prefix string, archived bool) error {
	convs, err := r.resumeConvs(ctx, archived)
	if err != nil {
		return r.reportErr(r.t("tui.err.listConvs", "列出会话失败"), err)
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
		if archived {
			return r.printf("%s", r.t("tui.resume.noPrefixArchived", "没有 id 以 %s 开头的归档会话，用 /resume --all --archived 看看有哪些\n", prefix))
		}
		return r.printf("%s", r.t("tui.resume.noPrefix", "没有 id 以 %s 开头的会话，用 /resume --all 看看有哪些\n", prefix))
	case 1:
		return r.attachResume(ctx, matched[0])
	default:
		if err := r.printf("%s", r.t("tui.resume.ambiguous", "id 前缀 %s 匹配到 %d 个会话，请给出更长的前缀:\n", prefix, len(matched))); err != nil {
			return err
		}
		sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
		tw := r.newTabWriter()
		fmt.Fprintln(tw, "ID\tTITLE")
		for _, c := range matched {
			fmt.Fprintf(tw, "%s\t%s\n", c.ID, orPlaceholder(oneLine(c.Title)))
		}
		return tw.Flush()
	}
}

// attachResume 把 c 设成当前会话并回显概况。c 是按值传入的拷贝，
// r.cur 指向这份新变量，不指向调用方切片里的元素。
// 成员或消息读失败只打一行，不回滚已经切换的 r.cur，也不让命令返回错误。
func (r *REPL) attachResume(ctx context.Context, c conversation.Conversation) error {
	r.cur = &c
	if err := r.printf("%s\n", r.t("tui.resume.attached", "已接上会话 %s，标题: %s", c.ID, oneLine(c.Title))); err != nil {
		return err
	}
	proj := c.Cwd
	if proj == "" {
		proj = r.t("tui.resume.unboundCell", "(未绑定)")
	}
	if err := r.printf("%s\n", r.t("tui.resume.project", "项目: %s", proj)); err != nil {
		return err
	}
	if err := r.printResumeMembers(ctx, c.ID); err != nil {
		return err
	}
	return r.printResumeTail(ctx, c.ID)
}

// printResumeMembers 只列仍在场的成员。读失败只打一行：
// 切换已经发生在回显之前，这里返回的是 printf 的结果，不是读库错误，
// 命令不算失败，也不回滚 r.cur。
func (r *REPL) printResumeMembers(ctx context.Context, id string) error {
	parts, err := r.svc.Participants(ctx, id, false)
	if err != nil {
		return r.printf("%s", r.t("tui.resume.membersReadFailed", "成员 (?): 读取失败: %v\n", err))
	}
	if len(parts) == 0 {
		return r.printf("%s", r.t("tui.resume.membersEmpty", "成员 (0): 还没有，用 /add 或 /team use 加入\n"))
	}
	ids := make([]string, len(parts))
	for i, p := range parts {
		ids[i] = p.AgentID
	}
	sort.Strings(ids)
	return r.printf("%s", r.t("tui.resume.members", "成员 (%d): %s\n", len(ids), strings.Join(ids, ", ")))
}

// printResumeTail 回显最近几条消息。MaxSeq 或 History 读失败同样只打一行，
// 不让命令返回错误，也不回滚已经切换的 r.cur：回显是附带信息，接上本身已经完成。
func (r *REPL) printResumeTail(ctx context.Context, id string) error {
	maxSeq, err := r.svc.MaxSeq(ctx, id)
	if err != nil {
		return r.printf("%s", r.t("tui.resume.messagesReadFailed", "消息: 读取失败: %v\n", err))
	}
	if maxSeq == 0 {
		return r.printf("%s", r.t("tui.resume.noMessages", "还没有消息\n"))
	}
	after := maxSeq - resumeTailLimit
	if after < 0 {
		after = 0
	}
	msgs, err := r.svc.History(ctx, id, after, resumeTailLimit)
	if err != nil {
		return r.printf("%s", r.t("tui.resume.messagesReadFailed", "消息: 读取失败: %v\n", err))
	}
	if err := r.printf("%s", r.t("tui.resume.recentMessages", "共 %d 条消息；最近 %d 条:\n", maxSeq, len(msgs))); err != nil {
		return err
	}
	for _, m := range msgs {
		if err := r.printf("  [%d] %s: %s\n", m.Seq, resumeSender(m), truncateRunes(orPlaceholder(oneLine(m.Text)), resumeTextLimit)); err != nil {
			return err
		}
	}
	return nil
}

// resumeSender 用 SenderID。user 与 agent 之外的 kind（system、task、task_result）
// 在发送者后面加空格和圆括号包着的 kind，屏幕上才分得清这不是成员说的。
func resumeSender(m conversation.Message) string {
	if m.Kind == conversation.KindUser || m.Kind == conversation.KindAgent {
		return m.SenderID
	}
	return m.SenderID + " (" + string(m.Kind) + ")"
}

// truncateRunes 按 rune 截到 max 个字符，超出的部分换成末尾三个 ASCII 点。
// 按字节截会把一个汉字切成半个，终端上显示成乱码。
func truncateRunes(s string, max int) string {
	if max < 0 {
		max = 0
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= max {
			break
		}
		b.WriteRune(r)
		n++
	}
	b.WriteString("...")
	return b.String()
}
