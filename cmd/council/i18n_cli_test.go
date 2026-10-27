package main

// i18n_cli_test.go 钉住命令行入口的中英分叉：
//
//   - `council -h` 在 -lang en 下没有汉字，默认（中文）下有；
//   - `council ask -h` 与 `council serve -h` 同样；
//   - 参数错误按 -lang 取语言（位置参数那一条）。
//
// 这些路径都不加载配置、不碰库：run 在 -h / 位置参数处就返回了。

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"unicode"

	"council/internal/i18n"
)

// captureStderr 跑一次 run，截下 stderr。与 chx_flag_test.go 同一手法：
// run 在这几条路上只写 stderr，不注册信号。
func captureStderr(t *testing.T, args []string) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提不成立：os.Pipe() 失败: %v", err)
	}
	var captured bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&captured, r)
	}()
	saved := os.Stderr
	os.Stderr = w
	code := run(args)
	os.Stderr = saved
	_ = w.Close()
	<-done
	_ = r.Close()
	return captured.String(), code
}

func cliHasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// cleanLangEnv 清掉三个区域变量与 COUNCIL_LANG，避免开发机的 LANG 影响
// 「默认中文」这一条。run 在显式 -lang zh/en 时会 os.Setenv(COUNCIL_LANG)，
// t.Setenv 负责还原。
func cleanLangEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	t.Setenv(i18n.EnvLang, "")
}

func TestCLIHelpEnglishHasNoChinese(t *testing.T) {
	cleanLangEnv(t)
	zh, code := captureStderr(t, []string{"-h"})
	if code != exitOK {
		t.Fatalf("council -h 退出码 = %d，期望 %d", code, exitOK)
	}
	if !cliHasHan(zh) {
		t.Fatalf("默认（中文）-h 里没有汉字，用例失去判别力：\n%s", zh)
	}

	en, code := captureStderr(t, []string{"-lang", "en", "-h"})
	if code != exitOK {
		t.Fatalf("council -lang en -h 退出码 = %d，期望 %d", code, exitOK)
	}
	if cliHasHan(en) {
		t.Fatalf("-lang en 的 -h 里出现汉字:\n%s", en)
	}
	for _, want := range []string{"-no-tui", "-verbose", "Config file path"} {
		if !strings.Contains(en, want) {
			t.Errorf("-lang en 的 -h 缺少 %q:\n%s", want, en)
		}
	}
}

func TestCLISubcommandUsageEnglish(t *testing.T) {
	cleanLangEnv(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "ask", args: []string{"ask", "-lang", "en", "-h"}, want: "Usage:"},
		{name: "serve", args: []string{"serve", "-lang", "en", "-h"}, want: "Usage: council serve"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := captureStderr(t, tc.args)
			if code != exitOK {
				t.Fatalf("%v 退出码 = %d，期望 %d", tc.args, code, exitOK)
			}
			if cliHasHan(out) {
				t.Fatalf("%v 的用法里出现汉字:\n%s", tc.args, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("用法里缺少 %q:\n%s", tc.want, out)
			}
		})
	}
}

func TestCLIPositionalErrorFollowsLang(t *testing.T) {
	cleanLangEnv(t)
	zh, code := captureStderr(t, []string{"extra"})
	if code != exitUsage {
		t.Fatalf("council extra 退出码 = %d，期望 %d", code, exitUsage)
	}
	if !strings.Contains(zh, "不接受位置参数") {
		t.Fatalf("中文位置参数错误没了:\n%s", zh)
	}

	en, code := captureStderr(t, []string{"-lang", "en", "extra"})
	if code != exitUsage {
		t.Fatalf("council -lang en extra 退出码 = %d，期望 %d", code, exitUsage)
	}
	if cliHasHan(en) {
		t.Fatalf("-lang en 的位置参数错误里出现汉字:\n%s", en)
	}
	if !strings.Contains(en, "does not accept positional arguments") {
		t.Fatalf("英文位置参数错误里没有对应文案:\n%s", en)
	}
}
