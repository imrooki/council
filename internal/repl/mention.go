package repl

import (
	"regexp"
	"strings"
)

// ParseMentions 按首次出现顺序返回去重后的小写 mention token。
//
// 本文件必须落在 internal/repl 而不是 internal/turn：internal/repl 导入
// internal/turn，反向导入是一个编译期的导入环。于是「agent 正文里的 @x
// 在路由上没有任何语义」由 Go 的导入图保证，不是靠一行注释、也不是靠 grep。
//
// tokenizer 三条写死：
//
//  1. 左边界：@ 的前一个字符必须是文本开头，或不属于 [A-Za-z0-9_@.-]。
//     这一条专挡 a@b.com 这类邮箱，以及 @@gk（第二个 @ 的左边还是 @）。
//  2. 最长匹配：@ 之后按 [A-Za-z0-9_.-] 取最长的一段，再剥掉尾部的 .
//     （只剥点，可以连续剥多个；剥完为空就不算 mention）。右侧不再加额外
//     边界条件——最长匹配已经把右边界定死，再要求右边是空白或标点白名单，
//     只会让句末的 @gk. 漏掉。
//  3. ASCII 小写折叠后必须匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$（与
//     internal/config/validate.go 的 idPattern 同一条），否则不算一个
//     mention。折叠是安全的：agent id 被这条正则限制成全小写，折叠在
//     id 空间上是单射，不可能把两个不同的 id 折成同一个。
//
// 尾部字符剥不剥，是「最长匹配」与「用户想说什么」会分叉的地方，要逐个裁。
//
// 原裁决是「不剥尾部的 - / _，最长匹配说了算」：@gk- 解析成 gk-，不是 gk。
// 理由是剥尾是一次静默的猜测，一个真叫 gk- 的 agent 从此就点不到名了；
// 而不剥的「猜错」不会静默——点名提示行会当场印「gk- 不是当前会话的成员」，
// 用户立刻看得见并改掉。这条仍然成立，不变。
//
// 点号不适用那条理由：id 正则禁止以点号结尾，所以尾部的点不可能是任何合法
// id 的一部分。剥它不是猜测，是由正则证明的。而不剥的代价是实打实的：ASCII
// 句点结束一句话太常见了。@grok-4.7. 若不剥，token 是 grok-4.7.，正则判不
// 合法，整个 mention 消失——用户以为点了名，其实没有，而 round.go 正拿它
// 决定一轮问谁。
//
// 判据只有一条：这个字符能不能是合法 id 的结尾。- 与 _ 能，所以不剥；
// . 不能，所以剥。
//
// 点号进字符类的代价：@gk.then 现在整段是一个 token gk.then，不再在 gk 停下。
// 它若不是成员，点名提示会当场印出来，/round 的名单解析会把它当成不在名单上
// 的 id 跳过。这是放开 id 里的点号的直接后果，接受。
//
// 期望行为：
//   - @gk2 解析出 gk2，gk 不在结果里（最长匹配，这是本函数存在的理由）
//   - @GK 命中 gk
//   - a@gk 与 a.@gk 不命中（左边界）
//   - (@gk) 与 @gk。命中（右侧不加边界条件；。是中文句号，不在 token 字符类里）
//   - @@gk 不命中
//   - @-gk 不命中（首字符不合 id 正则）
//   - @gk- 解析成 gk-；@gk_ 解析成 gk_（尾部的 - / _ 不剥）
//   - @grok-4.7 解析成 grok-4.7
//   - @grok-4.7. 与 @grok-4.7... 解析成 grok-4.7（尾部的点被剥）
//   - @gk. 解析成 gk（结果与点号进字符类之前相同，机制从「点不在字符类里」
//     变成「尾点被剥」）
//   - @gk-. 解析成 gk-（只剥点，剥到 - 就停）
//   - @gk.then 解析成 gk.then
//   - 重复 @gk @gk 只出一次，顺序按首次出现
func ParseMentions(text string) []string {
	seen := make(map[string]struct{})
	var out []string
	for i := 0; i < len(text); i++ {
		if text[i] != '@' {
			continue
		}
		if i > 0 && mentionLeftForbidden(text[i-1]) {
			continue
		}
		j := i + 1
		for j < len(text) && mentionTokenChar(text[j]) {
			j++
		}
		if j == i+1 {
			continue
		}
		tok := strings.ToLower(strings.TrimRight(text[i+1:j], "."))
		if tok == "" {
			continue
		}
		if !mentionIDRe.MatchString(tok) {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
		i = j - 1
	}
	return out
}

// mentionIDPattern 与 internal/config/validate.go 的 idPattern 同一条。
// 写在这里而不是 import config，是为了让 tokenizer 不依赖配置层；
// 两处必须保持同一条正则，改那边时记得回来。
const mentionIDPattern = `^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$`

var mentionIDRe = regexp.MustCompile(mentionIDPattern)

// mentionLeftForbidden 报告 c 贴在 @ 左边时，这个 @ 不能当 mention 的起点。
// @ 自身也在里面，所以 @@gk 的第二个 @ 过不了左边界。
func mentionLeftForbidden(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	case c == '_' || c == '@' || c == '.' || c == '-':
		return true
	}
	return false
}

// mentionTokenChar 是最长匹配吃进去的字符类 [A-Za-z0-9_.-]。
// . 必须在里面：id 允许中间有点号，不吃点的话 @grok-4.7 会停在 grok-4。
// 句末那个点由 ParseMentions 在匹配之后剥掉，不靠把它挡在字符类外面。
func mentionTokenChar(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	case c == '_' || c == '-' || c == '.':
		return true
	}
	return false
}
