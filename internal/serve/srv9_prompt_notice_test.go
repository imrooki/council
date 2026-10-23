package serve

// SRV9：`serve -rw` 启动时的外部系统提示可见性提示。
//
// 任务书 §4 要求：有成员带 prompt 时打一行，数量与成员名正确；0 个时
// 一个字节都不打印。这条不受 -verbose 控制，所以测的是输出本身。

import (
	"bytes"
	"strings"
	"testing"

	"council/internal/config"
)

func TestClaudePromptStartupNotice(t *testing.T) {
	t.Run("数量与成员名", func(t *testing.T) {
		cfg := &config.Config{Agents: map[string]config.AgentConfig{
			// 故意乱序放，断言输出按 id 排序：同一份配置每次启动打印同一行。
			"reviewer": {Provider: "claude", Prompt: "你是 reviewer。"},
			"critic":   {Provider: "claude", Prompt: "你是 critic。"},
			"plain":    {Provider: "claude"},
			// 非 claude 成员带 prompt 不会被注入：不能把它算进来，
			// 否则启动提示会报一个实际不存在的注入面。
			"grok-one": {Provider: "grok", Prompt: "这段不会被任何适配器消费"},
		}}

		var out bytes.Buffer
		printClaudePromptNotice(&out, cfg)

		want := "council serve: 2 个成员带外部系统提示（critic, reviewer），来自 .claude/agents\n"
		if out.String() != want {
			t.Errorf("启动提示不是期望的那一行：\n实际: %q\n期望: %q", out.String(), want)
		}
	})

	t.Run("0 个成员时一个字都不打印", func(t *testing.T) {
		cfg := &config.Config{Agents: map[string]config.AgentConfig{
			"plain": {Provider: "claude"},
		}}

		var out bytes.Buffer
		printClaudePromptNotice(&out, cfg)
		if got := out.String(); got != "" {
			t.Errorf("没有成员带 prompt 时不该打印任何东西，实际: %q", got)
		}
	})

	t.Run("nil 配置不 panic 也不打印", func(t *testing.T) {
		var out bytes.Buffer
		printClaudePromptNotice(&out, nil)
		if got := out.String(); got != "" {
			t.Errorf("nil 配置时不该打印任何东西，实际: %q", got)
		}
	})

	// 只含空白的 prompt 与空串不是一回事：它会被原样注入（BuildTurn 只看
	// `!= ""`），所以也必须算进提示里，否则屏幕上会少一个成员。
	t.Run("空白 prompt 也算带提示", func(t *testing.T) {
		cfg := &config.Config{Agents: map[string]config.AgentConfig{
			"spaced": {Provider: "claude", Prompt: "   "},
		}}
		if note := claudePromptNotice(cfg); !strings.Contains(note, "spaced") {
			t.Errorf("只含空白的 prompt 应当算带提示，实际 %q", note)
		}
	})
}
