package repl

// fu2：X44 独立审查余项在 repl 包的补测（HasLaterCommandLine 的取值）。只加用例，不动生产代码。
// 期望值一律取自实跑；每条用例杀的变异写在它自己的注释里。

import "testing"

// fuyLaterCase 是 HasLaterCommandLine 的一行取值。
type fuyLaterCase struct {
	id   string
	in   string
	want bool
}

// fuyCheckLater 逐行比较，不等时点名那一行。
func fuyCheckLater(t *testing.T, cases []fuyLaterCase) {
	t.Helper()
	for _, c := range cases {
		if got := HasLaterCommandLine(c.in); got != c.want {
			t.Errorf("%s：HasLaterCommandLine(%q) = %v，期望 %v", c.id, c.in, got, c.want)
		}
	}
}

// TestFuyAskToOpenNoExemption 钉 /ask、/to 不开出路：只有第 1 行恰好是 /say 才豁免。
// 第 1 行是 /ask（单独一行或带参数）、/to 带参数时，后面一行以 / 开头照样挡。
//
// 【判别力】变异：latercmd.go 的豁免判据额外放过第 1 行第一个词是 /ask 的情形
// （strings.TrimSpace(lines[0]) == "/say" 之外再或上 strings.HasPrefix(strings.TrimSpace(lines[0]), "/ask")），
// ask_bare、ask_arg 两行红；另一条变异只放过 /to，to_arg 一行红。
// say_control 与 ask_bare 同装置、只差第 1 行的命令名，证明同一形状下 false 这个结局确实存在。
// 这两条变异下全仓 go test ./... 只有本用例红（fu2 实跑），本用例是它们唯一的杀手。
func TestFuyAskToOpenNoExemption(t *testing.T) {
	fuyCheckLater(t, []fuyLaterCase{
		{"ask_bare", "/ask\nx\n/round a --go", true},
		{"ask_arg", "/ask a\n/round", true},
		{"to_arg", "/to a\n/b", true},
		{"say_control", "/say\nx\n/round a --go", false},
	})
}

// TestFuyOtherPunctuationNotCommand 钉别的标点开头的行不挡：D8 长文粘贴（Markdown 的标题、列表、引用）与改动前相同。
// 每一个标点行都配一条同装置、只把那个标点换成 / 的正对照（期望 true），证明 false 不是因为这个形状根本判不出 true。
//
// 【判别力】变异：行判定把 "#" 也当命令前缀
// （strings.HasPrefix(x, "/") 之外再或上 strings.HasPrefix(x, "#")），hash 一行红。
// dash、quote、star 三行不被这条变异杀，它们钉的是同一条规则在别的标点上的取值。
// 这条变异下全仓 go test ./... 只有本用例红（fu2 实跑）。
func TestFuyOtherPunctuationNotCommand(t *testing.T) {
	fuyCheckLater(t, []fuyLaterCase{
		{"hash", "a\n# 标题\nb", false},
		{"dash", "a\n- 列表\nb", false},
		{"quote", "a\n> 引用\nb", false},
		{"star", "a\n* 列表\nb", false},
		{"hash_control", "a\n/ 标题\nb", true},
		{"dash_control", "a\n/ 列表\nb", true},
		{"quote_control", "a\n/ 引用\nb", true},
		{"star_control", "a\n/ 列表\nb", true},
	})
}
