package repl

// mention_test.go 覆盖 docs/11d_STEP8C_CONTRACT.md 第 2.1 节与第 2.8 节第 1 条：
// ParseMentions 的 tokenizer 契约——左边界、最长匹配、ASCII 小写折叠后必须
// 匹配 internal/config/validate.go 的 idPattern（本文件写成时是 ^[a-z0-9][a-z0-9_-]*$；2026-09-23 UX-2 起放开了中间的点号，见 validate.go），
// 以及 2.1 节的裁决：尾部的 "-" / "_" 不剥，最长匹配说了算。
//
// 背对背纪律：本文件的全部断言只来自 11d 契约文字，不读 mention.go 的
// 实现——写这份测试的时刻它要么不存在、要么是半成品。ParseMentions 此刻
// 大概率还不存在，本文件编译不过是预期状态，等实现落地后才会绿。

import "testing"

// TestParseMentionsGolden 是 2.8 第 1 条要求的表驱动 golden，逐行对应契约
// 2.1 节与 2.8 第 1 条列出的每一种边界情形。
func TestParseMentionsGolden(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			// 这一行是本块存在的理由（2.8 第 1 条原话）：@gk2 必须整体解析成
			// gk2，gk 不能作为独立 token 出现在结果里。旧的
			// strings.Contains(text, "@"+selfID) 会把 "@gk2" 误判成命中
			// "@gk"，这正是 ParseMentions 的最长匹配规则（2.1 节第 2 条）
			// 要堵上的洞。
			name: "最长匹配挡住前缀截断_gk2不能被认成gk",
			in:   "@gk2 你怎么看",
			want: []string{"gk2"},
		},
		{
			name: "大写折叠命中gk",
			in:   "@GK 你来说说",
			want: []string{"gk"},
		},
		{
			name: "邮箱形状不命中_左边界挡住",
			in:   "联系 a@gk 试试",
			want: nil,
		},
		{
			name: "圆括号包裹命中_右侧不加边界条件",
			in:   "(@gk)",
			want: []string{"gk"},
		},
		{
			name: "中文句号收尾命中_右侧不加边界条件",
			in:   "@gk。",
			want: []string{"gk"},
		},
		{
			name: "双@不命中_第二个@的左边界字符是@本身",
			in:   "@@gk",
			want: nil,
		},
		{
			name: "首字符是连字符不合idPattern_整个token不命中",
			in:   "@-gk",
			want: nil,
		},
		{
			// 2.1 节裁决原文：不剥尾部的 "-" / "_"，最长匹配说了算。
			// "@gk-" 解析成 "gk-"，不是 "gk"。
			name: "尾部连字符不剥尾_解析成gk-而不是gk",
			in:   "@gk-",
			want: []string{"gk-"},
		},
		{
			name: "尾部下划线不剥尾_解析成gk_而不是gk",
			in:   "@gk_",
			want: []string{"gk_"},
		},
		{
			name: "重复只出一次_顺序按首次出现",
			in:   "@gk 你好 @bob 再问一次 @gk",
			want: []string{"gk", "bob"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseMentions(tc.in)
			assertMentionSlice(t, tc.in, got, tc.want)
		})
	}
}

// TestParseMentionsGK2NeverYieldsGK 是 2.8 第 1 条“这一行是本块存在的理由”
// 的独立断言：不依赖上面整表比较的措辞，单独钉住“解析 @gk2 的结果集合里
// 不能出现 gk”这件事，即使将来有人改动表驱动用例的写法，这一条也不能丢。
func TestParseMentionsGK2NeverYieldsGK(t *testing.T) {
	const in = "@gk2 你怎么看"
	got := ParseMentions(in)
	for _, tok := range got {
		if tok == "gk" {
			t.Fatalf("ParseMentions(%q) = %v，其中出现了 %q——@gk2 的最长匹配必须整体是 "+
				"gk2，gk 不能作为独立 token 出现（11d 契约 2.1 节 / 2.8 第 1 条）", in, got, "gk")
		}
	}
	if len(got) != 1 || got[0] != "gk2" {
		t.Fatalf("ParseMentions(%q) = %v，期望恰好 [\"gk2\"]", in, got)
	}
}

// assertMentionSlice 比较 ParseMentions 的结果与期望值，逐个下标比较，
// 顺序也要求相同（2.1 节：按首次出现顺序返回去重后的 token）。
func assertMentionSlice(t *testing.T, in string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ParseMentions(%q) = %v，期望 %v", in, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseMentions(%q) = %v，期望 %v（第 %d 个不同）", in, got, want, i)
		}
	}
}
