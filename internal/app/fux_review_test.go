package app

// fu1：CO1 独立审查余项在 app 包的补测。只加用例，不动生产代码。
// 装置复用 cox_app_test.go 的 coxAppSetup / coxRunApp / coxAppTail（真 app.Run、真 store、假 CLI）。

import (
	"bytes"
	"strings"
	"testing"
)

// TestFuxAppVerboseMentionHint 钉「点名了」那一行的档位门在详细档那一侧：详细档照打。
// 照 A1 的装置与脚本（裸文本 @c1 点名一个在场成员），只差 Options.Verbose。
//
// 【判别力】变异：commands.go 的「点名了」门改成恒不打（if !r.concise { 改成 if false {）。
// 这时 verbose 子测试里这一整行消失，红。default 子测试是同装置、只差档位的对照：
// 简洁档没有这一行（A1 default 的尾部整串也钉着这一点）；它同时证明在场断言不是装置写死的。
func TestFuxAppVerboseMentionHint(t *testing.T) {
	const script = "/new t\n/join c1\n裸文本 @c1\n/round c1 --go\n/ask c1 再问\n/quit\n"
	const hint = "点名了 c1（用 /ask c1 让他说话）"
	tail := func(t *testing.T, verbose bool) []string {
		t.Helper()
		cfgPath := coxAppSetup(t)
		var out, errOut bytes.Buffer
		if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: verbose, NoAuto: true}); err != nil {
			t.Fatalf("app.Run 返回 %v；Out:\n%s\nErr:\n%s", err, out.String(), errOut.String())
		}
		got := coxAppTail(out.String())
		if len(got) == 0 {
			t.Fatalf("前提不成立：尾部为空（找不到「c1 已加入会话 」那一行）；Out:\n%s", out.String())
		}
		return got
	}
	count := func(lines []string, want string) int {
		n := 0
		for _, l := range lines {
			if l == want {
				n++
			}
		}
		return n
	}

	t.Run("verbose", func(t *testing.T) {
		got := tail(t, true)
		if n := count(got, hint); n != 1 {
			t.Fatalf("详细档的尾部应当恰好一次整行 %q，实际 %d；尾部: %q", hint, n, got)
		}
		if n := count(got, "outcome spoke  agent c1"); n < 1 {
			t.Fatalf("前提不成立：详细档的尾部应当含整行「outcome spoke  agent c1」；尾部: %q", got)
		}
	})

	t.Run("default", func(t *testing.T) {
		got := tail(t, false)
		if n := count(got, hint); n != 0 {
			t.Fatalf("简洁档的尾部不应当有 %q，实际 %d 次；尾部: %q", hint, n, got)
		}
	})
}
