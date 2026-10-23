package turn

// SRV9：外部系统提示（AgentConfig.Prompt）必须计入身份指纹。
//
// 判据有三条：
//   - 同一份配置连算两次必须相同（指纹是确定性的）；
//   - Prompt 变 → 指纹变（改了 .claude/agents/x.md 就是换了一副脑子）；
//   - Prompt 为空 → 指纹与改动前的算法逐字节相同（既有会话一条都不许重建）。

import (
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

func TestClaudeSystemPromptChangesIdentityFingerprint(t *testing.T) {
	p, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("provider.Lookup(claude) 失败: %v", err)
	}

	cfg := newTestConfig()
	ac := cfg.Agents[testAgentClaude]
	const convID = "srv9-conv"
	baseDir := t.TempDir()

	fpOf := func(agent config.AgentConfig) string {
		t.Helper()
		fp, err := identityFPFor(p, testAgentClaude, agent, baseDir, convID, provider.PermissionRead, RulesBlocks{})
		if err != nil {
			t.Fatalf("identityFPFor 失败: %v", err)
		}
		return fp
	}

	emptyFP := fpOf(ac)
	if again := fpOf(ac); again != emptyFP {
		t.Errorf("同一份配置两次算出的指纹不同: %q vs %q", emptyFP, again)
	}

	withPromptA := ac
	withPromptA.Prompt = "你是 code reviewer。\n\n先读文件，再给结论。"
	fpA := fpOf(withPromptA)
	if fpA == emptyFP {
		t.Errorf("加了外部系统提示，指纹却没变: %q", fpA)
	}
	if again := fpOf(withPromptA); again != fpA {
		t.Errorf("同一份带提示的配置两次算出的指纹不同: %q vs %q", fpA, again)
	}

	withPromptB := withPromptA
	withPromptB.Prompt += "\n多一行"
	if fpB := fpOf(withPromptB); fpB == fpA {
		t.Errorf("外部系统提示改了一个字节，指纹却没变: %q", fpB)
	}

	// 第三段：空 Prompt 的指纹必须仍然等于「改动前的算法」。这里不复用
	// identityFPFor，而是按本片改动之前的函数体重算一遍：探针请求不带
	// SystemPrompt。若哪天有人把 prompt 的哈希无条件塞进某一段，这条会红——
	// 那意味着所有没用 prompt 的成员升级后都重建一次会话。
	probe := provider.TurnRequest{
		AgentID:     testAgentClaude,
		DisplayName: ac.DisplayName,
		Model:       ac.Model,
		Effort:      ac.Effort,
		Role:        renderIdentity(testAgentClaude, ac, RulesBlocks{}),
		Permission:  provider.PermissionRead,
		WorkDir:     agentWorkDir(baseDir, convID, testAgentClaude, provider.PermissionRead),
	}
	digest, err := PrefixArgsDigest(p, probe)
	if err != nil {
		t.Fatalf("PrefixArgsDigest 失败: %v", err)
	}
	wantLegacyFP := IdentityFingerprint(
		renderIdentityCore(testAgentClaude, ac),
		ac.Model,
		effortFlagEmitted(p.Capabilities().Effort, ac.Effort),
		provider.PermissionRead,
		RulesBlocks{},
		digest,
	)
	if emptyFP != wantLegacyFP {
		t.Errorf("空 Prompt 的指纹与改动前的算法不同：\n实际: %q\n改动前: %q", emptyFP, wantLegacyFP)
	}
}
