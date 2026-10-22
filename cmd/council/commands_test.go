package main

// SRV2：council commands 的进程边界。这里钉命令行契约——-json 的字段名、
// 条数与顺序，以及未知 flag 的退出码；元数据本身（别名、撞名 panic）由
// internal/command 与 internal/repl 的测试负责。

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"council/internal/repl"
)

// commandsRun 跑一次 run(args) 并截下 stdout。照 chx_flag_test.go 的体例：
// run 在 commands 这条路上不注册信号、不碰进程的其余状态，只写 stdout。
func commandsRun(t *testing.T, args []string) (string, int) {
	t.Helper()
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提不成立：os.Pipe() 失败: %v", err)
	}
	var captured bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(&captured, readEnd)
	}()
	saved := os.Stdout
	os.Stdout = writeEnd
	code := run(args)
	os.Stdout = saved
	_ = writeEnd.Close()
	<-readDone
	_ = readEnd.Close()
	return captured.String(), code
}

// TestCommandsJSON 是 §4 的机器可读输出：字段名、条数、顺序与别名。
//
// 【判别力】少一个字段或改名（例如 scopes 写成 scope）时逐字段断言红；
// 把别名也当成一条命令打印时条数比注册表多；List 不再排序时第一条不是 add；
// 空别名/空范围写成 null 时 [] 的断言红（GUI 得为两种形状各写一条路）。
func TestCommandsJSON(t *testing.T) {
	out, code := commandsRun(t, []string{"commands", "-json"})
	if code != exitOK {
		t.Fatalf("council commands -json 退出码 = %d，期望 %d；stderr 见上", code, exitOK)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("输出不是 JSON 数组: %v\n输出:\n%s", err, out)
	}
	specs := repl.CommandSpecs()
	if len(rows) != len(specs) {
		t.Fatalf("JSON 有 %d 条，注册表有 %d 条", len(rows), len(specs))
	}

	wantKeys := []string{"name", "aliases", "args", "summary", "detail", "scopes"}
	for i, row := range rows {
		if len(row) != len(wantKeys) {
			t.Errorf("第 %d 条有 %d 个字段，期望 %d 个：%v", i+1, len(row), len(wantKeys), row)
		}
		for _, key := range wantKeys {
			if _, ok := row[key]; !ok {
				t.Errorf("第 %d 条少了字段 %q：%v", i+1, key, row)
			}
		}
		if row["name"] != specs[i].Name {
			t.Errorf("第 %d 条的 name = %v，期望 %q（顺序应当与 CommandSpecs 相同）", i+1, row["name"], specs[i].Name)
		}
		if summary, _ := row["summary"].(string); summary == "" {
			t.Errorf("%v 的 summary 是空的", row["name"])
		}
		// 空别名与空范围必须是 [] 而不是 null。
		if row["aliases"] == nil {
			t.Errorf("%v 的 aliases 是 null，期望 []", row["name"])
		}
		if row["scopes"] == nil {
			t.Errorf("%v 的 scopes 是 null，期望 []", row["name"])
		}
	}

	// 名字按字典序（与 List 同一个判据）。
	for i := 1; i < len(rows); i++ {
		prev, _ := rows[i-1]["name"].(string)
		cur, _ := rows[i]["name"].(string)
		if prev >= cur {
			t.Fatalf("JSON 没有按名字字典序：%q 在 %q 之前", prev, cur)
		}
	}

	// /exit 只以别名出现：既没有一条叫 exit 的命令，quit 的 aliases 里有它。
	quitFound := false
	for _, row := range rows {
		name, _ := row["name"].(string)
		if name == "exit" {
			t.Errorf("JSON 里有一条 name=exit 的命令：/exit 只是 /quit 的别名")
		}
		if name != "quit" {
			continue
		}
		quitFound = true
		aliases, _ := row["aliases"].([]any)
		hasExit := false
		for _, alias := range aliases {
			if alias == "exit" {
				hasExit = true
			}
		}
		if !hasExit {
			t.Errorf("quit 的 aliases 里没有 exit：%v", aliases)
		}
	}
	if !quitFound {
		t.Fatal("JSON 里没有 quit")
	}
}

// TestCommandsPlainMatchesSharedFormat 钉住不带 -json 的输出就是 /help 的列格式：
// 同一份 WriteCommandTable、同一份元数据，两处不可能长得不一样。
func TestCommandsPlainMatchesSharedFormat(t *testing.T) {
	out, code := commandsRun(t, []string{"commands"})
	if code != exitOK {
		t.Fatalf("council commands 退出码 = %d，期望 %d", code, exitOK)
	}
	if !strings.HasPrefix(out, "COMMAND") {
		t.Fatalf("输出没有以 COMMAND 表头开头:\n%s", out)
	}

	var want bytes.Buffer
	if err := repl.WriteCommandTable(&want, repl.CommandSpecs()); err != nil {
		t.Fatalf("WriteCommandTable 返回 %v", err)
	}
	if out != want.String() {
		t.Errorf("council commands 的输出与 WriteCommandTable(CommandSpecs()) 不同\n实际:\n%s\n期望:\n%s", out, want.String())
	}
	if n := strings.Count(out, "\n"); n != len(repl.CommandSpecs())+1 {
		t.Errorf("输出有 %d 行，期望 %d 行（表头 + 每条命令一行）", n, len(repl.CommandSpecs())+1)
	}
}

// TestCommandsUsageErrors 钉住退出码：未知 flag 与多余的位置参数都退 2（不加载配置）。
func TestCommandsUsageErrors(t *testing.T) {
	if _, code := commandsRun(t, []string{"commands", "-srv2-no-such-flag"}); code != exitUsage {
		t.Errorf("未知 flag 的退出码 = %d，期望 exitUsage(%d)", code, exitUsage)
	}
	if _, code := commandsRun(t, []string{"commands", "extra"}); code != exitUsage {
		t.Errorf("多余位置参数的退出码 = %d，期望 exitUsage(%d)", code, exitUsage)
	}
}
