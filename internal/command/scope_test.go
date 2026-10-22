package command

import "testing"

// 本文件钉住三条与「入口过滤」有关的语义。它们都很容易在重构里被顺手改掉，
// 而改掉之后屏幕上看不出差别：只会在某天多出一个 TUI 专有命令时才暴露。

type testRun struct{ name string }

func TestInScope_空scope不是全部(t *testing.T) {
	var all Spec // Scopes 为空 = 三个入口都能用
	if !all.InScope(ScopeTUI) || !all.InScope(ScopeGUI) || !all.InScope(ScopeHeadless) {
		t.Fatalf("Scopes 为空的命令应当在三个入口都可见：%+v", all)
	}
	// 空串是零值，不是一个入口的名字。把它当全部会让 List("") 悄悄返回整张表，
	// 调用方以为自己按入口过滤过了，实际没有。
	if all.InScope("") {
		t.Errorf("空 scope 不是入口，InScope(\"\") 应为 false")
	}
	onlyTUI := Spec{Name: "x", Scopes: []Scope{ScopeTUI}}
	if onlyTUI.InScope(ScopeGUI) {
		t.Errorf("只声明 TUI 的命令不该在 GUI 可见")
	}
}

func TestList_空scope返回空(t *testing.T) {
	var r Registry[testRun]
	r.Register(Spec{Name: "a"}, testRun{name: "a"})
	if got := r.List(""); len(got) != 0 {
		t.Errorf("List(\"\") 应返回空，实际 %d 条", len(got))
	}
	if got := r.ListAll(); len(got) != 1 {
		t.Errorf("ListAll 应返回整张表，实际 %d 条", len(got))
	}
}

func TestListAll_有序且是副本(t *testing.T) {
	var r Registry[testRun]
	r.Register(Spec{Name: "b", Aliases: []string{"bb"}, Scopes: []Scope{ScopeTUI}}, testRun{name: "b"})
	r.Register(Spec{Name: "a"}, testRun{name: "a"})

	got := r.ListAll()
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("ListAll 应按名字排序，实际 %+v", got)
	}
	got[1].Name = "改坏了"
	got[1].Aliases[0] = "改坏了"
	again := r.ListAll()
	if again[1].Name != "b" || again[1].Aliases[0] != "bb" {
		t.Errorf("ListAll 必须返回副本，改它不该影响注册表：%+v", again)
	}
}

func TestRegister_大写名panic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("注册大写名字应当 panic：查找大小写敏感，/Help 与 /help 会落到两处")
		}
	}()
	var r Registry[testRun]
	r.Register(Spec{Name: "Help"}, testRun{name: "Help"})
}
