package repl

// P2：中文输入法打出的全角斜杠也要当命令，不能静默落成一条发言。

import "testing"

func TestNormalizeCommandSlash(t *testing.T) {
	cases := map[string]string{
		"／use 123": "/use 123",
		"/use 123":  "/use 123",
		"a／b":      "a／b",
		"":          "",
	}
	for in, want := range cases {
		if got := NormalizeCommandSlash(in); got != want {
			t.Errorf("NormalizeCommandSlash(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestHasLaterCommandLineFullWidthSlash(t *testing.T) {
	if !HasLaterCommandLine("/say 第一行\n／use 123") {
		t.Fatal("全角斜杠的第二行没有被识别成命令")
	}
	if HasLaterCommandLine("／say 第一行\n普通第二行") {
		t.Fatal("全角 /say 没有豁免多行命令检查")
	}
}
