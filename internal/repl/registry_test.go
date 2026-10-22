package repl

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"council/internal/command"
)

// helpLines 跑一次 /help，返回不含表头与末尾空行的各行。
func helpLines(t *testing.T) []string {
	t.Helper()
	var buf bytes.Buffer
	r := New(nil, nil, nil, nil, &buf, RulesContext{})
	if err := r.dispatch(context.Background(), "/help"); err != nil {
		t.Fatalf("/help 返回 %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	// 表头是 ASCII 两列；列宽由 tabwriter 按最长的命令名补，这里不钉具体空白。
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "COMMAND") || !strings.HasSuffix(lines[0], "SUMMARY") {
		t.Fatalf("/help 的首行不是 COMMAND / SUMMARY 表头，实际:\n%s", buf.String())
	}
	return lines[1:]
}

// TestHelpRenderedFromRegistry 钉住「/help 由注册表生成」：每一条 List(ScopeTUI)
// 恰好占一行，顺序与 List 相同，说明逐字等于 Spec.Summary。
//
// 【判别力】另写一份手写帮助（或漏登记一条命令）时条数不等；顺序不再按 Name
// 字典序时第 i 条对不上；说明被改过一个字时后缀断言红。
func TestHelpRenderedFromRegistry(t *testing.T) {
	var buf bytes.Buffer
	r := New(nil, nil, nil, nil, &buf, RulesContext{})
	specs := r.cmds.List(command.ScopeTUI)
	lines := helpLines(t)

	if len(lines) != len(specs) {
		t.Fatalf("/help 有 %d 行命令，注册表有 %d 条；完整输出:\n%s", len(lines), len(specs), buf.String())
	}
	for i, spec := range specs {
		// 说明是最后一列，后面没有别的列，所以整行以它结尾。
		if !strings.HasPrefix(lines[i], "/"+spec.Name) {
			t.Errorf("第 %d 行 = %q，期望以 /%s 开头（顺序应当与 List 相同）", i+1, lines[i], spec.Name)
			continue
		}
		if !strings.HasSuffix(lines[i], spec.Summary) {
			t.Errorf("/%s 那一行的说明与 Spec.Summary 不同\n行: %q\nSpec: %q", spec.Name, lines[i], spec.Summary)
		}
	}
}

// TestHelpDoesNotListAliases 钉住 /exit 只是别名：帮助里命令仍是 25 条，
// 没有多出 /exit 这一行（别名解析到 /quit 的同一个 Spec）。
func TestHelpDoesNotListAliases(t *testing.T) {
	lines := helpLines(t)
	for _, line := range lines {
		if strings.HasPrefix(line, "/exit") {
			t.Fatalf("/help 里出现了别名行 %q：别名不该单独占一行", line)
		}
	}
	var quit int
	for _, line := range lines {
		if strings.HasPrefix(line, "/quit") {
			quit++
		}
	}
	if quit != 1 {
		t.Fatalf("/help 里 /quit 有 %d 行，期望 1 行", quit)
	}
}

// TestExitAliasDispatchesToQuit 钉住 /exit 与 /quit 是同一条命令：dispatch（行式）
// 与 Submit（TUI）都必须返回 errQuit，而不是「未知命令」。
func TestExitAliasDispatchesToQuit(t *testing.T) {
	for _, path := range []string{"dispatch", "submit"} {
		t.Run(path, func(t *testing.T) {
			var buf bytes.Buffer
			r := New(nil, nil, nil, nil, &buf, RulesContext{})
			var err error
			if path == "dispatch" {
				err = r.dispatch(context.Background(), "/exit")
			} else {
				err = r.Submit(context.Background(), "/exit")
			}
			if !IsQuit(err) {
				t.Fatalf("/exit 返回 %v，期望 errQuit（/quit 的哨兵）", err)
			}
			if buf.Len() != 0 {
				t.Errorf("/exit 不该打印任何东西，实际 %q", buf.String())
			}
		})
	}
}

// TestUnknownCommandUnchanged 是 /exit 之外的那条：大小写写错仍是未知命令，
// 答复与引入别名之前逐字节相同。
func TestUnknownCommandUnchanged(t *testing.T) {
	var buf bytes.Buffer
	r := New(nil, nil, nil, nil, &buf, RulesContext{})
	if err := r.dispatch(context.Background(), "/EXIT"); err != nil {
		t.Fatalf("/EXIT 返回 %v，期望 nil", err)
	}
	if got, want := buf.String(), msgUnknownCommand+"\n"; got != want {
		t.Errorf("/EXIT 的输出 = %q，期望 %q", got, want)
	}
}

// TestRegistryEntriesHaveHandlers 钉住每条登记的命令都带处理函数：
// 漏写 run 的注册项能通过 Register（它只查名字），但一敲就空指针。
func TestRegistryEntriesHaveHandlers(t *testing.T) {
	reg := newCommandRegistry()
	for _, spec := range reg.List(command.ScopeTUI) {
		_, entry, ok := reg.Lookup(spec.Name)
		if !ok {
			t.Fatalf("%s 在 List 里却查不到", spec.Name)
		}
		if entry.run == nil {
			t.Errorf("%s 没有处理函数", spec.Name)
		}
		if spec.Summary == "" {
			t.Errorf("%s 的 Summary 是空的：/help 会少一列说明", spec.Name)
		}
	}
}

// TestCommandSpecsSortedAndResolvable 钉住 council commands -json 的那份表：
// 按名字字典序、无重名、别名都能解析回它所属的命令。
func TestCommandSpecsSortedAndResolvable(t *testing.T) {
	specs := CommandSpecs()
	if len(specs) == 0 {
		t.Fatal("CommandSpecs 是空的")
	}
	reg := newCommandRegistry()
	seen := make(map[string]bool, len(specs))
	quitAliased := false
	for i, spec := range specs {
		if i > 0 && specs[i-1].Name >= spec.Name {
			t.Errorf("List 没有按名字字典序：%q 在 %q 之前", specs[i-1].Name, spec.Name)
		}
		if seen[spec.Name] {
			t.Errorf("命令 %q 重复出现", spec.Name)
		}
		if spec.Name == "exit" {
			t.Error("/exit 是 /quit 的别名，不该作为独立命令出现")
		}
		seen[spec.Name] = true
		for _, alias := range spec.Aliases {
			got, _, ok := reg.Lookup(alias)
			if !ok || got.Name != spec.Name {
				t.Errorf("%s 的别名 %q 解析到 %q（ok=%v），期望 %q", spec.Name, alias, got.Name, ok, spec.Name)
			}
			if spec.Name == "quit" && alias == "exit" {
				quitAliased = true
			}
		}
	}
	if !quitAliased {
		t.Error("quit 没有登记 exit 别名（SRV2 的唯一新增别名）")
	}
}
