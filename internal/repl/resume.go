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
)

const (
	// resumeTailLimit 是接上后回显最近几条消息。再多会把屏幕刷走，
	// 完整历史仍走 /log。
	resumeTailLimit = 6
	// resumeTextLimit 是回显的每条消息正文最多多少个字符（rune）。
	// 按 rune 截，避免把一个汉字切成半个。
	resumeTextLimit = 100
)

const resumeUsage = "用法: /resume [--all] [<编号|id 前缀>]"

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
// filtered 只在没带 --all 且当前有项目目录时为真：只保留 cwd 与 r.projectDir
// 相同的会话。被滤掉的会话里 cwd 为空的个数记入 hiddenUnbound，
// 用来提示还有没绑项目的旧会话。--all，或取不到启动目录时，不过滤。
//
// 排序在这里做，不改 store：store 的 created_at 倒序是 /conv 与 /use 依赖的契约，
// /resume 要按最后活动排。lastActive 倒序，相同则 CreatedAt 倒序，再相同则 id 升序。
// 最后活动时间没能从库里确定时，lastActive 仍是创建时间，只作排序兜底，
// LAST_ACTIVE 那一格打 ?，不要把这个兜底时间印出去。
func (r *REPL) loadResumeRows(ctx context.Context, all bool) (resumeList, error) {
	convs, err := r.svc.Conversations(ctx)
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
					row.readErrs = append(row.readErrs, fmt.Errorf("MaxSeq 报告有 %d 条消息，但读不到末条", maxSeq))
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

// cmdResume 列出本机会话，或按编号 / id 前缀接上。
// needsConv 是假：没有当前会话时也要能列和接，否则它自己会答「先 /new 或 /use」。
//
// id 前缀不受项目过滤影响，同 /use：用户给的是完整身份线索，限制范围只会让他困惑。
// 编号每次重新计算：列表会随新消息变序，编号不是稳定句柄。
// 接上是纯读：不触发任何成员调用，不改成员，不写库。
func cmdResume(ctx context.Context, r *REPL, args cmdArgs) error {
	all, token, ok := parseResumeArgs(args.fields)
	if !ok {
		return r.printf("%s\n", resumeUsage)
	}
	if token == "" {
		return r.printResumeList(ctx, all)
	}
	if isResumeIndex(token) {
		return r.resumeByIndex(ctx, all, token)
	}
	return r.resumeByPrefix(ctx, token)
}

// parseResumeArgs 从 fields 里拆出 --all 和至多一个选择 token。
// --all 出现几次都行，位置不限。其余以 -- 开头的 token 是未知 flag。
func parseResumeArgs(fields []string) (all bool, token string, ok bool) {
	ok = true
	for _, f := range fields {
		if f == "--all" {
			all = true
			continue
		}
		if strings.HasPrefix(f, "--") {
			return false, "", false
		}
		if token != "" {
			return false, "", false
		}
		token = f
	}
	return all, token, ok
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

// printResumeList 打无参数 /resume 的整段输出。
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
func (r *REPL) printResumeList(ctx context.Context, all bool) error {
	list, err := r.loadResumeRows(ctx, all)
	if err != nil {
		return r.reportErr("列出会话失败", err)
	}
	rows, hiddenUnbound, filter := list.rows, list.hiddenUnbound, list.filtered
	n := len(rows)
	if n == 0 {
		var line string
		if filter {
			line = "本项目还没有会话，用 /new <标题> 建一个（/resume --all 看其它项目的）"
		} else {
			line = "还没有任何会话，用 /new <标题> 建一个"
		}
		if err := r.printf("%s\n", line); err != nil {
			return err
		}
		if filter && hiddenUnbound > 0 {
			return r.printf("另有 %d 个未绑定项目的旧会话，/resume --all 查看\n", hiddenUnbound)
		}
		return nil
	}

	var head string
	switch {
	case filter:
		head = fmt.Sprintf("项目: %s（本项目 %d 个会话；/resume --all 看全部）", r.projectDir, n)
	case all:
		head = fmt.Sprintf("全部会话（%d 个）", n)
	default:
		head = fmt.Sprintf("没有项目目录，列出全部会话（%d 个）", n)
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
				proj = "(未绑定)"
			}
			cells = append(cells, proj)
		}
		cells = append(cells, orPlaceholder(oneLine(c.Title)))
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
		if err := r.printf("会话 %s 读取失败（对应列显示 ?）: %s\n", row.conv.ID, strings.Join(parts, "; ")); err != nil {
			return err
		}
	}
	if filter && hiddenUnbound > 0 {
		if err := r.printf("另有 %d 个未绑定项目的旧会话，/resume --all 查看\n", hiddenUnbound); err != nil {
			return err
		}
	}
	return r.printf("敲 /resume <编号> 接上；编号按最近活动排序，每次重新计算\n")
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
// 口径，越界不回落成 id 前缀：输错一位编号不该悄悄接上别的会话。
func (r *REPL) resumeByIndex(ctx context.Context, all bool, token string) error {
	n, err := strconv.Atoi(token)
	if err != nil {
		return r.printf("%s\n", resumeUsage)
	}
	list, err := r.loadResumeRows(ctx, all)
	if err != nil {
		return r.reportErr("列出会话失败", err)
	}
	rows := list.rows
	if n < 1 || n > len(rows) {
		if len(rows) == 0 {
			return r.printf("编号 %d 超出范围（当前没有可选的会话），先敲 /resume 看列表\n", n)
		}
		return r.printf("编号 %d 超出范围（1..%d），先敲 /resume 看列表\n", n, len(rows))
	}
	return r.attachResume(ctx, rows[n-1].conv)
}

// resumeByPrefix 在全部会话里匹配，不走项目过滤：用户给的是身份线索，
// 收窄到本项目会让别的目录里那个会话永远接不上。
// 完整 id 先于前缀，避免短 id 被更长的前缀撞成多匹配。
// 多匹配只列候选、不切换 r.cur。
func (r *REPL) resumeByPrefix(ctx context.Context, prefix string) error {
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
		return r.printf("没有 id 以 %s 开头的会话，用 /resume --all 看看有哪些\n", prefix)
	case 1:
		return r.attachResume(ctx, matched[0])
	default:
		if err := r.printf("id 前缀 %s 匹配到 %d 个会话，请给出更长的前缀:\n", prefix, len(matched)); err != nil {
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
	if err := r.printf("已接上会话 %s，标题: %s\n", c.ID, oneLine(c.Title)); err != nil {
		return err
	}
	proj := c.Cwd
	if proj == "" {
		proj = "(未绑定)"
	}
	if err := r.printf("项目: %s\n", proj); err != nil {
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
		return r.printf("成员 (?): 读取失败: %v\n", err)
	}
	if len(parts) == 0 {
		return r.printf("成员 (0): 还没有，用 /add 或 /team use 加入\n")
	}
	ids := make([]string, len(parts))
	for i, p := range parts {
		ids[i] = p.AgentID
	}
	sort.Strings(ids)
	return r.printf("成员 (%d): %s\n", len(ids), strings.Join(ids, ", "))
}

// printResumeTail 回显最近几条消息。MaxSeq 或 History 读失败同样只打一行，
// 不让命令返回错误，也不回滚已经切换的 r.cur：回显是附带信息，接上本身已经完成。
func (r *REPL) printResumeTail(ctx context.Context, id string) error {
	maxSeq, err := r.svc.MaxSeq(ctx, id)
	if err != nil {
		return r.printf("消息: 读取失败: %v\n", err)
	}
	if maxSeq == 0 {
		return r.printf("还没有消息\n")
	}
	after := maxSeq - resumeTailLimit
	if after < 0 {
		after = 0
	}
	msgs, err := r.svc.History(ctx, id, after, resumeTailLimit)
	if err != nil {
		return r.printf("消息: 读取失败: %v\n", err)
	}
	if err := r.printf("共 %d 条消息；最近 %d 条:\n", maxSeq, len(msgs)); err != nil {
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
