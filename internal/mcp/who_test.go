package mcp

// who_test.go 是 council_who 的包内用例。它直接构造 server 并替换
// enumerate 接缝，所以能确定性地断言输出行：进程集合来自测试，不来自
// 这台机器上恰好在跑什么。
//
// 外部用例（mcp_test.go）负责证明真实 app.Run 装配下默认关闭只回一句说明；
// 这里负责 off 不调用枚举、observe 的文本形状与错误路径。

import (
	"errors"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/i18n"
	"council/internal/localagent"
)

func whoTestServer(level string, lang i18n.Lang) *server {
	return &server{
		deps: Deps{Cfg: &config.Config{LocalAgent: level}},
		lang: lang,
	}
}

// TestToolWhoOffNeverEnumerates 是 off 路径的判别性用例：枚举函数一被调用
// 就 Fatalf。没有这一条，实现里多写一句 localagent.Scan 也测不出来。
func TestToolWhoOffNeverEnumerates(t *testing.T) {
	for _, level := range []string{"", "off", "register", "collaborate"} {
		t.Run("level="+level, func(t *testing.T) {
			old := enumerate
			t.Cleanup(func() { enumerate = old })
			enumerate = func() ([]localagent.Process, error) {
				t.Fatalf("local_agent=%q 时不该枚举进程（S-01）", level)
				return nil, nil
			}

			result := whoTestServer(level, i18n.LangZh).toolWho()
			if result.IsError {
				t.Fatalf("off 不是错误，实际 isError: %s", result.Content[0].Text)
			}
			if len(result.Content) != 1 {
				t.Fatalf("content 应当只有一段，实际 %d 段", len(result.Content))
			}
			if result.Content[0].Type != "text" {
				t.Errorf("content[0].type = %q，期望 text", result.Content[0].Type)
			}
			if !strings.Contains(result.Content[0].Text, "未开启") {
				t.Errorf("off 的说明里没有「未开启」:\n%s", result.Content[0].Text)
			}
		})
	}
}

// TestToolWhoNilConfigIsOff 钉住「配置没加载 = 没有授权 = off」，
// 而不是默认打开扫描。
func TestToolWhoNilConfigIsOff(t *testing.T) {
	old := enumerate
	t.Cleanup(func() { enumerate = old })
	enumerate = func() ([]localagent.Process, error) {
		t.Fatal("配置未加载时不该枚举进程")
		return nil, nil
	}

	s := &server{lang: i18n.LangZh}
	result := s.toolWho()
	if result.IsError {
		t.Fatalf("off 不是错误，实际: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "未开启") {
		t.Errorf("说明里没有「未开启」:\n%s", result.Content[0].Text)
	}
}

// TestToolWhoObserveReturnsText 钉住 observe 的文本形状：一个 text block、
// 表头带实例数、每行是 kind/pid/ppid/exe/started_at/source、末行固定脚注。
func TestToolWhoObserveReturnsText(t *testing.T) {
	old := enumerate
	t.Cleanup(func() { enumerate = old })
	calls := 0
	started := time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)
	enumerate = func() ([]localagent.Process, error) {
		calls++
		return []localagent.Process{
			{PID: 10, PPID: 1, ExeName: "claude.exe", StartedAt: started},
			{PID: 20, PPID: 10, ExeName: "node.exe"},
			{PID: 30, PPID: 1, ExeName: "codex.exe"},
		}, nil
	}

	result := whoTestServer(config.LocalAgentObserve, i18n.LangZh).toolWho()
	if result.IsError {
		t.Fatalf("observe 不该失败: %s", result.Content[0].Text)
	}
	if calls != 1 {
		t.Errorf("observe 调用了 %d 次枚举，期望 1 次", calls)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("content 形状不对: %+v", result.Content)
	}

	text := result.Content[0].Text
	want := []string{
		"本机 agent 进程（2）:",
		"- kind=claude pid=10 ppid=1 exe=claude.exe started_at=2026-10-27T09:30:00Z source=observed certainty=probable",
		"- kind=codex pid=30 ppid=1 exe=codex.exe started_at=unknown source=observed certainty=probable",
		"本工具不新增任何模型调用。",
	}
	for _, line := range want {
		if !strings.Contains(text, line) {
			t.Errorf("返回文本缺少 %q，实际:\n%s", line, text)
		}
	}
	if strings.Contains(text, "node.exe") {
		t.Errorf("通用宿主 node.exe 不该出现在结果里:\n%s", text)
	}
	if !strings.HasSuffix(text, "本工具不新增任何模型调用。") {
		t.Errorf("末行应当固定是「本工具不新增任何模型调用。」:\n%s", text)
	}
}

// TestToolWhoEmptyListIsNotAnError：本机确实没有 agent 时也要回一段文本，
// 不是 isError；这一行与枚举失败必须能分开。
func TestToolWhoEmptyListIsNotAnError(t *testing.T) {
	old := enumerate
	t.Cleanup(func() { enumerate = old })
	enumerate = func() ([]localagent.Process, error) {
		return []localagent.Process{{PID: 1, ExeName: "python.exe"}}, nil
	}

	result := whoTestServer(config.LocalAgentObserve, i18n.LangZh).toolWho()
	if result.IsError {
		t.Fatalf("空列表不是错误: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "本机 agent 进程（0）:") || !strings.Contains(text, "没有发现本机 agent 进程") {
		t.Errorf("空列表的输出不对:\n%s", text)
	}
}

// TestToolWhoEnumerationError：枚举失败按 isError 返回，并点名原因。
func TestToolWhoEnumerationError(t *testing.T) {
	old := enumerate
	t.Cleanup(func() { enumerate = old })
	boom := errors.New("boom")
	enumerate = func() ([]localagent.Process, error) { return nil, boom }

	result := whoTestServer(config.LocalAgentObserve, i18n.LangZh).toolWho()
	if !result.IsError {
		t.Fatalf("枚举失败应当是 isError，实际:\n%s", result.Content[0].Text)
	}
	if text := result.Content[0].Text; !strings.Contains(text, "进程枚举不可用") || !strings.Contains(text, "boom") {
		t.Errorf("错误说明缺少原因:\n%s", text)
	}
}

// TestToolWhoEnglish 钉住英文目录真的被用上（避免漏翻时静默回中文）。
func TestToolWhoEnglish(t *testing.T) {
	old := enumerate
	t.Cleanup(func() { enumerate = old })
	enumerate = func() ([]localagent.Process, error) {
		return []localagent.Process{{PID: 10, PPID: 1, ExeName: "claude.exe"}}, nil
	}

	text := whoTestServer(config.LocalAgentObserve, i18n.LangEn).toolWho().Content[0].Text
	for _, want := range []string{"local agent processes (1):", "kind=claude", "This tool adds no model calls."} {
		if !strings.Contains(text, want) {
			t.Errorf("英文输出缺少 %q，实际:\n%s", want, text)
		}
	}
}
