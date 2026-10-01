package repl

// 验收 31（mention 这一侧）与 32-34：id 允许点号、mention tokenizer 剥尾点（契约第 5、6.4 节）。

import (
	"regexp"
	"testing"
)

// ux2rFrozenIDPattern 是契约第 5 节冻结的那一条，两处一模一样。
const ux2rFrozenIDPattern = `^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$`

// ux2rIDCases 是验收 31 的正反例。gk-、gk_、a..b、a.- 必须合法：
// 第 5 节的正则末字符类是 [a-z0-9_-]，旧规则允许的尾部 - _ 一个不收紧。
var ux2rIDCases = []struct {
	id   string
	want bool
}{
	{"grok-4.7", true},
	{"grok-4.7.", false},
	{".grok", false},
	{"gk-", true},
	{"gk_", true},
	{"a..b", true},
	{"a.-", true},
	{"a", true},
	{"7", true},
	{"a.", false},
	{".", false},
	{"-gk", false},
	{"Grok-4.7", false},
}

// 验收 31（mention 侧）：mention.go 的正则常量就是冻结的那一条，且按它判正反例。
func TestUx2rMentionIDPattern(t *testing.T) {
	if mentionIDPattern != ux2rFrozenIDPattern {
		t.Fatalf("mentionIDPattern 应当是 %q，实际 %q", ux2rFrozenIDPattern, mentionIDPattern)
	}
	re := regexp.MustCompile(mentionIDPattern)
	for _, c := range ux2rIDCases {
		if got := re.MatchString(c.id); got != c.want {
			t.Errorf("mentionIDPattern 对 %q 判 %v，期望 %v", c.id, got, c.want)
		}
		// 经 ParseMentions 走一遍：合法 id 必须整体解析出来，这证明 tokenizer 真的在用新正则，
		// 而且字符类里有点号。非法的 .grok 与 -gk 不许出任何 token。
		if c.id == "Grok-4.7" {
			continue
		}
		got := ParseMentions("@" + c.id + " ")
		switch {
		case c.want:
			assertMentionSlice(t, "@"+c.id+" ", got, []string{c.id})
		case c.id == ".grok" || c.id == "-gk" || c.id == ".":
			assertMentionSlice(t, "@"+c.id+" ", got, nil)
		}
	}
}

// 验收 32-34 与第 6.4 节「更新期望行为清单」里列出的其余几条。
func TestUx2rParseMentionsDots(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		// 验收 32：句末的 ASCII 句点被剥掉，mention 不消失。
		{"点号id", "@grok-4.7", []string{"grok-4.7"}},
		{"点号id句末一个点", "@grok-4.7.", []string{"grok-4.7"}},
		{"点号id句末三个点", "@grok-4.7...", []string{"grok-4.7"}},
		{"点号id句中", "请 @grok-4.7. 看一下", []string{"grok-4.7"}},
		{"大写折叠", "@GROK-4.7.", []string{"grok-4.7"}},
		// 验收 33：- 与 _ 能是合法 id 的结尾，不剥。把它们也剥了的实现会在这里红。
		{"尾部连字符不剥", "@gk-", []string{"gk-"}},
		{"尾部下划线不剥", "@gk_", []string{"gk_"}},
		{"尾部连字符后跟点只剥点", "@gk-.", []string{"gk-"}},
		{"尾部下划线后跟多个点只剥点", "@gk_..", []string{"gk_"}},
		// 验收 34：点号进了字符类，@gk.then 整段是一个 token。
		{"点号在中间整段是一个token", "@gk.then", []string{"gk.then"}},
		{"连续点号在中间", "@a..b", []string{"a..b"}},
		// 结果与今天相同、机制换成剥尾点。
		{"gk句点", "@gk.", []string{"gk"}},
		{"左边界仍挡点号", "a.@gk", nil},
		{"剥完为空不算", "@...", nil},
		{"只剩点号开头不合法", "@.gk", nil},
		{"去重按首次出现", "@grok-4.7. 然后 @grok-4.7 再 @gk-", []string{"grok-4.7", "gk-"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertMentionSlice(t, tc.in, ParseMentions(tc.in), tc.want)
		})
	}
}
