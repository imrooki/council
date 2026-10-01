package repl

import "strings"

// HasLaterCommandLine 报告一次多行提交里，第 2 行起有没有哪一行单独提交时会被 Submit 当成斜杠命令。
// TUI 在回车时用它挡住「整块粘贴几条命令、按一次回车」（00_DECISIONS #44）：
// Submit 只把整段的第一个词当命令，后面几行全成了它的参数或正文，而回显照样逐行印成「> /...」。
//
// 行号与判定都按 Submit 的看法算：
//   - 先像 Submit 那样把整段首尾的空白与四个不可见字符剥掉（isTrimmable），再按 "\n" 切行；
//     剥掉之后的第一行才是第 1 行，前导空行不算。
//   - 一行剥掉行首的空白与四个不可见字符之后以 "/" 开头，就算命令行。行首缩进不改变判定：
//     Submit 收到 "  /new x" 同样当命令执行。
//
// 例外：第 1 行去掉行尾空白后恰好是 "/say" 时返回 false。那时 Submit 的第一个词就是 /say，
// 它把 /say 之后的内容整体去掉首尾空白、中间一字不改，当成一条消息发出（commands.go 的 restOf），
// 这是用户明说的「整段是消息」。
// 行尾只去空白（unicode.IsSpace），不去四个不可见字符：Submit 按空白切词，"/say" 后面紧跟一个
// 零宽字符时第一个词不是 /say，Submit 会答「未知命令」，那种输入不豁免。
// 第 1 行是 "/say 某些字" 时也不豁免。
func HasLaterCommandLine(text string) bool {
	lines := strings.Split(strings.TrimFunc(text, isTrimmable), "\n")
	if len(lines) < 2 {
		return false
	}
	if strings.TrimSpace(lines[0]) == "/say" {
		return false
	}
	for _, line := range lines[1:] {
		if strings.HasPrefix(strings.TrimLeftFunc(line, isTrimmable), "/") {
			return true
		}
	}
	return false
}
