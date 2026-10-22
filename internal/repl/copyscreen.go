package repl

// 本文件是 TUI-2（docs/21f_TUI2_COPY_MODE_TASK.md §6）在 repl 这一侧的唯一入口：
// 复制模式是一块临时整屏，正文要在进屏之前一次取齐，由 TUI 自己排版。
//
// 与 /copy（registry.go 的 CopyRecent）的分工照旧：哪些消息算正文（copyableMessage）、
// 怎么从最新往回翻页（recentCopyable）都只有那一份实现；这里只把「最近 n 条」
// 换成「最近 CopyScreenMax 条」，并补上屏幕上要显示的成员名。
// 剪贴板与整屏渲染在 internal/tui 那侧 —— repl 的输出出口只有一个 io.Writer，
// 拿不到 Program，写不了 OSC52，也管不了备用屏。
//
// 不改 internal/turn、internal/store：复制模式要的只是读，Service 现有的
// MaxSeq + History 已经够用，多一条接口只会让两处都要维护。

import (
	"context"
	"errors"

	"council/internal/conversation"
)

// CopyScreenMax 是复制模式一次最多装载的消息条数。
//
// 取全量不能无上限：History 会把整场会话读进内存，而复制模式一屏只看得见
// 二十来行，两千条已经是几十屏。超出时只保留最近这么多条，TUI 在顶栏说明
// 「只加载了最近 CopyScreenMax 条」，不假装整场都在。
const CopyScreenMax = 2000

// CopyLine 是复制模式里的一条发言，不是一行：Text 里可以带换行，由 TUI 拆成多行。
// Who 是屏幕上的显示名（repl.displayName 的规则：配置里的 display_name，缺了用 id）；
// User 为 true 表示用户自己说的，TUI 排成 [你]。
type CopyLine struct {
	Who  string
	User bool
	Text string
}

// CopyScreen 取当前会话最近 CopyScreenMax 条可复制的正文，按时间正序。
//
// truncated 为 true 表示更早还有可复制的正文、被上限截掉了。
// 没有当前会话与没有可复制正文都返回 ErrCopyEmpty：复制模式对这两件事的反应
// 完全相同（状态栏一句「没有可复制的消息」，不进整屏），任务书 §3 就是这么分的。
// 持久化没起来（svc 为 nil）仍返回 msgNoStore：那不是「没内容」，是真出了故障，
// TUI 会把它打到回滚区，不装成一句正常结果。
func (r *REPL) CopyScreen(ctx context.Context) (lines []CopyLine, truncated bool, err error) {
	if r.svc == nil {
		return nil, false, errors.New(msgNoStore)
	}
	if r.cur == nil {
		return nil, false, ErrCopyEmpty
	}
	msgs, truncated, err := r.copyScreenMessages(ctx, r.cur.ID, CopyScreenMax)
	if err != nil {
		return nil, false, err
	}
	if len(msgs) == 0 {
		return nil, false, ErrCopyEmpty
	}
	lines = make([]CopyLine, len(msgs))
	for i, m := range msgs {
		line := CopyLine{Text: m.Text}
		if m.Kind == conversation.KindUser || m.SenderID == conversation.ReservedSenderUser {
			line.User = true
		} else {
			line.Who = r.displayName(m.SenderID)
		}
		lines[i] = line
	}
	return lines, truncated, nil
}

// copyScreenMessages 从最新往回取 limit 条可复制的正文，按时间正序返回；
// truncated 表示更早还有。多要一条是为了分辨「刚好 limit 条」与「不止 limit 条」，
// 多出来的那条随后丢掉 —— 翻页仍走 recentCopyable，不另写一套。
//
// limit 由调用方给，CopyScreen 传 CopyScreenMax；测试用小值钉上限，
// 不必为了 2001 条往临时库里插两千行。
func (r *REPL) copyScreenMessages(ctx context.Context, convID string, limit int) ([]conversation.Message, bool, error) {
	if limit < 1 {
		return nil, false, nil
	}
	msgs, err := r.recentCopyable(ctx, convID, limit+1)
	if err != nil {
		return nil, false, err
	}
	if len(msgs) > limit {
		return msgs[len(msgs)-limit:], true, nil
	}
	return msgs, false, nil
}
