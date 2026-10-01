package repl

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

// MentionCandidate 是 TUI 输入框 @ 补全列表里的一行（00_DECISIONS #47 补充，AT1）。
// ID 是成员的 agent id（全小写，与 ParseMentions 解析出的 token 同一空间）；
// Name 是配置里的 display_name 压成一行、去掉控制字符之后的样子，配置里没有这个 id 或 display_name 为空时是空串。
type MentionCandidate struct {
	ID   string
	Name string
}

// MentionCandidates 返回当前会话的活跃成员，去掉 [summary] 总结成员，按 ID 字节序升序。
// 只读：不打印、不改 REPL 的任何字段、不写库。
// 没有当前会话、或持久化没起来时返回 nil, nil。
//
// 调用纪律与 SetRoundObserver 相同：只许在没有 Submit 在跑的时候调用。
// TUI 在执行 Submit 的那个 goroutine 上、Submit 返回之后调用它（internal/tui 的 submitCmd），
// 与命令串行，所以不加锁。
func (r *REPL) MentionCandidates(ctx context.Context) ([]MentionCandidate, error) {
	if r.svc == nil || r.cur == nil {
		return nil, nil
	}
	parts, err := r.svc.Participants(ctx, r.cur.ID, false)
	if err != nil {
		return nil, err
	}
	sumID := r.cfg.Summary.Agent
	out := make([]MentionCandidate, 0, len(parts))
	for _, p := range parts {
		if sumID != "" && p.AgentID == sumID {
			continue
		}
		name := ""
		if ac, ok := r.cfg.Agents[p.AgentID]; ok {
			name = mentionName(ac.DisplayName)
		}
		out = append(out, MentionCandidate{ID: p.AgentID, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// mentionName 把 display_name 变成补全列表里能放进一行的样子：控制字符换成空格，再去掉首尾空白。
// 配置层不校验 display_name，换行或 ESC 进了活区会把帧高算错。
func mentionName(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
}

// MentionBoundaryOK 报告 prev 贴在 @ 左边时，这个 @ 能不能当 mention 的起点。
// 与 ParseMentions 的左边界是同一条规则（mentionLeftForbidden）；非 ASCII 字符不挡，
// 因为 ParseMentions 按字节看时，多字节字符的最后一个字节不在禁止集合里。
func MentionBoundaryOK(prev rune) bool {
	return prev >= 0x80 || !mentionLeftForbidden(byte(prev))
}

// MentionTokenRune 报告 r 是不是 ParseMentions 最长匹配吃进去的字符（mentionTokenChar，ASCII 的 [A-Za-z0-9_.-]）。
func MentionTokenRune(r rune) bool {
	return r < 0x80 && mentionTokenChar(byte(r))
}
