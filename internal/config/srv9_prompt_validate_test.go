package config

// SRV9：AgentConfig.Prompt 的 8 KiB 上限。
//
// 两件事分开验：
//   - 上限按**字节**算，且恰好 8192 放行、8193 拒绝（不静默截断）；
//   - 合并配置时错误要能说清它来自哪一层文件——合并之后光看
//     agents.<id>.prompt 已经推不出该改哪一份。

import (
	"strings"
	"testing"
)

func TestValidateRejectsOversizedAgentPrompt(t *testing.T) {
	t.Run("按字节计量且边界放行", func(t *testing.T) {
		cfg := tstValidConfig()
		// 2730 * 3 + 2 = 8192 字节：runes 远少于 8192，能钉住「按字节不按字符」。
		exact := strings.Repeat("提", 2730) + "ab"
		if len(exact) != MaxAgentPromptBytes {
			t.Fatalf("用例构造错误：len(exact) = %d，期望 %d", len(exact), MaxAgentPromptBytes)
		}
		tstEditAgent(cfg, "theory", func(a *AgentConfig) { a.Prompt = exact })
		if err := cfg.Validate(); err != nil {
			t.Fatalf("恰好 %d 字节的 prompt 应当通过校验，实际报错: %v", MaxAgentPromptBytes, err)
		}

		// 2731 个三字节字符 = 8193 字节，但只有 2731 个字符：若实现按 rune
		// 数校验，这一条会误放行。
		tstEditAgent(cfg, "theory", func(a *AgentConfig) { a.Prompt = strings.Repeat("提", 2731) })
		err := cfg.Validate()
		tstRequireErrContains(t, err, "agents.theory.prompt", "8192")
	})

	t.Run("合并配置时点名来源文件", func(t *testing.T) {
		globalPath := tstWriteTOML(t,
			`[providers.claude]`,
			`command = "claude"`,
			``,
			`[agents.theory]`,
			`display_name = "Theory"`,
			`provider     = "claude"`,
			`model        = "opus"`,
			`prompt       = "`+strings.Repeat("x", MaxAgentPromptBytes+1)+`"`,
		)
		projectPath := tstWriteTOML(t,
			`[agents.critic]`,
			`display_name = "Critic"`,
			`provider     = "claude"`,
			`model        = "opus"`,
		)

		_, err := LoadMerged([]Layer{
			{Path: projectPath, Project: true},
			{Path: globalPath},
		})
		// 超限的 theory 只写在全局层，来源必须是那一份路径；只报 agent id
		// 的话，用户会去项目配置里找一个根本不存在的键。
		tstRequireErrContains(t, err, "agents.theory.prompt", "8192", globalPath)
	})
}
