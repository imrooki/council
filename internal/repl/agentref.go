package repl

import (
	"fmt"
	"sort"
	"strings"
)

// agentRefError 是引用解析失败。Matches 为空表示没找到，
// 长度 >= 2 表示有歧义（已按字典序排序）。
//
// 用一个带字段的类型而不是两个哨兵错误：调用方要把全部候选原样打给用户，
// 哨兵错误装不下候选名单，调用方就只能再扫一遍候选、自己重算一次。
type agentRefError struct {
	Ref     string
	Matches []string
}

// Error 返回给人看的一句话。措辞不冻结，但歧义时必须带上全部候选：
// 只说「有歧义」而不列候选，用户得自己去猜要多敲几个字。
//
// Ref 用 %s 原样拼进去而不是 %q：%q 会转义反斜杠与引号，拼出来的就不再是
// 用户敲下的那几个字。空引用单独给一句，否则屏幕上是两个空格夹着空气。
func (e *agentRefError) Error() string {
	if e.Ref == "" {
		return "agent 引用为空"
	}
	if len(e.Matches) == 0 {
		return fmt.Sprintf("没有与 %s 匹配的 agent", e.Ref)
	}
	return fmt.Sprintf("%s 有歧义，可能是：%s", e.Ref, strings.Join(e.Matches, ", "))
}

// resolveAgentRef 在 candidates 里把一个引用解析成唯一 id。
// 优先级：完全相同 > 唯一前缀 > 报错。本函数不读任何状态，不修改 candidates，
// 对 candidates 的顺序与重复不做假设。
//
// 它只在 candidates 这个集合里判「完全相同」。调用方要保证 candidates 就是
// 「用户可能指的那个全集」（见 6.2、6.3），否则全名会被当成前缀。
//
// 「完全相同」必须排在前缀之前：候选同时有 gk 与 gk2 时，gk 若只走前缀，
// 它同时是 gk 与 gk2 的前缀，就永远是歧义，一个真叫 gk 的 agent 从此点不到名。
//
// 绝不猜：有歧义就报错并列出全部候选，不挑「最短的」或「字典序第一个」。
// 挑错一个 agent 的代价是一次入场费，而且用户不会立刻发现 ——
// 屏幕上照样有人回答，只是答的不是他想问的那个。
func resolveAgentRef(ref string, candidates []string) (string, error) {
	// 空串是所有 id 的前缀。不单独拦下，它会在候选多于一个时报成「全部歧义」，
	// 在候选恰好一个时静默选中那一个 —— 两种都不是用户说出来的意思。
	if ref == "" {
		return "", &agentRefError{Ref: ""}
	}
	for _, c := range candidates {
		if c == ref {
			return c, nil
		}
	}

	// 去重用 map：candidates 可能由调用方拼接多个来源而来，同一个 id 出现两次
	// 不该让一个唯一前缀变成「歧义」。
	seen := make(map[string]struct{})
	var matches []string
	for _, c := range candidates {
		if !strings.HasPrefix(c, ref) {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		matches = append(matches, c)
	}

	switch len(matches) {
	case 0:
		return "", &agentRefError{Ref: ref}
	case 1:
		return matches[0], nil
	default:
		// 排序让输出与候选的来源顺序（map 遍历、store 返回顺序）无关，
		// 同一状态下同一条命令每次打出来的一样。
		sort.Strings(matches)
		return "", &agentRefError{Ref: ref, Matches: matches}
	}
}

// unionIDs 把几组 id 并成一个去重后的切片，按字典序排好。
// 排序不是为了解析（resolveAgentRef 不依赖顺序），是为了让同一状态下
// 的候选列表稳定，便于排查。
func unionIDs(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, g := range groups {
		for _, id := range g {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
