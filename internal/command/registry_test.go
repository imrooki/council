package command

import "testing"

// mustPanic 断言 fn 会 panic。注册表的错误都是编程错误，panic 是它的契约。
func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s 没有 panic", what)
		}
	}()
	fn()
}

func TestRegistryLookupNameAndAlias(t *testing.T) {
	var reg Registry[string]
	reg.Register(Spec{Name: "round", Aliases: []string{"r"}, Summary: "问一轮"}, "round-handler")
	reg.Register(Spec{Name: "quit", Aliases: []string{"exit"}, Summary: "退出"}, "quit-handler")

	// 别名解析在 Lookup 里：拿到的是同一条命令的 Spec 与同一个负载。
	spec, h, ok := reg.Lookup("r")
	if !ok {
		t.Fatal("Lookup(\"r\") 没找到 round 的别名")
	}
	if spec.Name != "round" || spec.Summary != "问一轮" {
		t.Errorf("别名解析到 %+v，期望 round 的元数据", spec)
	}
	if h != "round-handler" {
		t.Errorf("别名解析到的负载 = %q，期望 round-handler", h)
	}

	// 名字仍然直接可查。
	if _, h, ok := reg.Lookup("round"); !ok || h != "round-handler" {
		t.Errorf("Lookup(\"round\") = (%q, %v)，期望 (round-handler, true)", h, ok)
	}

	// 大小写敏感：/Round 是未知命令，不是 round。
	if _, _, ok := reg.Lookup("Round"); ok {
		t.Error("Lookup(\"Round\") 命中了，期望大小写敏感")
	}
	if _, _, ok := reg.Lookup("EXIT"); ok {
		t.Error("Lookup(\"EXIT\") 命中了，期望大小写敏感")
	}
	if _, _, ok := reg.Lookup("nosuch"); ok {
		t.Error("Lookup(\"nosuch\") 命中了")
	}
}

func TestRegistryPanicsOnDuplicateKeys(t *testing.T) {
	cases := []struct {
		name   string
		first  Spec
		second Spec
	}{
		{"重名", Spec{Name: "add"}, Spec{Name: "add"}},
		{"后一条的别名撞前一条的名字", Spec{Name: "add"}, Spec{Name: "join", Aliases: []string{"add"}}},
		{"后一条的名字撞前一条的别名", Spec{Name: "add", Aliases: []string{"a"}}, Spec{Name: "a"}},
		{"两条的别名相同", Spec{Name: "add", Aliases: []string{"a"}}, Spec{Name: "join", Aliases: []string{"a"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reg Registry[int]
			reg.Register(tc.first, 1)
			mustPanic(t, "撞名注册", func() { reg.Register(tc.second, 2) })
		})
	}
}

func TestRegistryPanicsOnMalformedSpec(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
	}{
		{"空名字", Spec{}},
		{"空别名", Spec{Name: "add", Aliases: []string{""}}},
		{"名字带斜杠", Spec{Name: "/add"}},
		{"别名带斜杠", Spec{Name: "add", Aliases: []string{"/a"}}},
		{"名字含空白", Spec{Name: "two words"}},
		{"名字与自己的别名相同", Spec{Name: "add", Aliases: []string{"add"}}},
		{"同一条命令里别名重复", Spec{Name: "add", Aliases: []string{"a", "a"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reg Registry[int]
			mustPanic(t, tc.name, func() { reg.Register(tc.spec, 1) })
		})
	}
}

func TestRegistryListSortedByScope(t *testing.T) {
	var reg Registry[int]
	// 故意乱序注册：List 的顺序只能来自 Name 排序，不能来自注册顺序。
	reg.Register(Spec{Name: "zzz"}, 1)
	reg.Register(Spec{Name: "aaa", Scopes: []Scope{ScopeGUI}}, 2)
	reg.Register(Spec{Name: "mmm", Aliases: []string{"x"}}, 3)

	tui := reg.List(ScopeTUI)
	// aaa 只登记在 GUI，TUI 这个范围里不该出现；其余两条按字典序。
	if len(tui) != 2 || tui[0].Name != "mmm" || tui[1].Name != "zzz" {
		t.Fatalf("List(ScopeTUI) = %+v，期望 [mmm zzz]", tui)
	}

	gui := reg.List(ScopeGUI)
	if len(gui) != 3 || gui[0].Name != "aaa" || gui[1].Name != "mmm" || gui[2].Name != "zzz" {
		t.Fatalf("List(ScopeGUI) = %+v，期望 [aaa mmm zzz]", gui)
	}

	// 顺序稳定：同一个 scope 连取两次必须一样（map 迭代顺序不能用）。
	again := reg.List(ScopeGUI)
	for i := range gui {
		if gui[i].Name != again[i].Name {
			t.Fatalf("两次 List(ScopeGUI) 顺序不同：%v 对 %v", gui, again)
		}
	}
}

func TestRegistryReturnsCopies(t *testing.T) {
	var reg Registry[int]
	reg.Register(Spec{Name: "help", Aliases: []string{"h"}, Summary: "原始说明"}, 1)

	// 空切片的约定：JSON 里是 [] 而不是 null，消费者不必先判空。
	spec, _, ok := reg.Lookup("help")
	if !ok {
		t.Fatal("Lookup(\"help\") 没找到")
	}
	if spec.Aliases == nil || spec.Scopes == nil {
		t.Fatalf("空别名/空范围应当是空切片，实际 aliases=%v scopes=%v", spec.Aliases, spec.Scopes)
	}

	// 调用方改返回值，不能改到注册表里那份。
	spec.Aliases[0] = "changed"
	spec.Scopes = append(spec.Scopes, ScopeGUI)
	spec.Summary = "被改过的说明"

	fresh, _, _ := reg.Lookup("help")
	if fresh.Summary != "原始说明" || fresh.Aliases[0] != "h" || len(fresh.Scopes) != 0 {
		t.Errorf("改了 Lookup 的返回值之后注册表跟着变了：%+v", fresh)
	}
	if _, _, ok := reg.Lookup("h"); !ok {
		t.Error("别名 h 查不到了：返回值与注册表共享了底层数组")
	}

	// List 的每一项同样是副本。
	list := reg.List(ScopeTUI)
	list[0].Summary = "又被改过"
	if again := reg.List(ScopeTUI); again[0].Summary != "原始说明" {
		t.Errorf("改了 List 的返回值之后注册表跟着变了：%+v", again)
	}
}

func TestSpecInScope(t *testing.T) {
	all := Spec{Name: "help"}
	for _, scope := range []Scope{ScopeTUI, ScopeGUI, ScopeHeadless} {
		if !all.InScope(scope) {
			t.Errorf("Scopes 为空的命令应当属于 %s", scope)
		}
	}
	tuiOnly := Spec{Name: "help", Scopes: []Scope{ScopeTUI}}
	if tuiOnly.InScope(ScopeGUI) {
		t.Error("只登记在 TUI 的命令不该属于 GUI")
	}
	if !tuiOnly.InScope(ScopeTUI) {
		t.Error("只登记在 TUI 的命令应当属于 TUI")
	}
}
