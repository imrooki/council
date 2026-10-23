package turn

// SRV9 的端到端补充：Ask 真的把 AgentConfig.Prompt 带到了每一轮的 Command 里。
//
// provider 层的用例只证明 BuildTurn 会拼，指纹用例只证明 identityFPFor 会算；
// 两处都对着 ac.Prompt，若 Ask 忘了把字段填进 TurnRequest，两条用例照样全绿，
// 而真实请求会静默地不带外部提示——没有任何一层会报错（指纹比较的是两次
// 「算出来的」指纹，不是「算出来的」与「真正发出去的」命令行）。这条用例
// 走完整的 Ask 两次，直接看 Spawner 收到的 Command。

import (
	"context"
	"strings"
	"testing"
)

func TestAskInjectsClaudeSystemPromptEveryTurn(t *testing.T) {
	const prompt = "你是 reviewer。\n\n只读，不要改文件。"

	cfg := newTestConfig()
	ac := cfg.Agents[testAgentClaude]
	ac.Prompt = prompt
	cfg.Agents[testAgentClaude] = ac

	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn

	postUser(t, svc, convID, "第一轮的正文")
	fs.enqueueSuccess(claudeT1JSON)
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第一轮 Ask 失败: %v", err)
	}

	postUser(t, svc, convID, "第二轮的正文")
	fs.enqueueSuccess(claudeT2JSON)
	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("第二轮 Ask 失败: %v", err)
	}

	if len(fs.calls) != 2 {
		t.Fatalf("期望两轮各起一次进程，实际 %d 次", len(fs.calls))
	}
	var firstPath string
	for i, call := range fs.calls {
		idx := -1
		for n, arg := range call.cmd.Args {
			if arg == "--append-system-prompt-file" {
				idx = n
				break
			}
		}
		if idx < 0 || idx+1 >= len(call.cmd.Args) {
			t.Fatalf("第 %d 轮的 argv 里没有 --append-system-prompt-file <file>: %v", i+1, call.cmd.Args)
		}
		path := call.cmd.Args[idx+1]
		body, ok := call.cmd.Files[path]
		if !ok {
			t.Fatalf("第 %d 轮 argv 指向的 %q 不在 Files 里: %v", i+1, path, call.cmd.Files)
		}
		if !strings.Contains(string(body), prompt) {
			t.Errorf("第 %d 轮的身份文件里没有外部系统提示：%q", i+1, string(body))
		}
		if !strings.Contains(string(body), "Provider: claude") {
			t.Errorf("第 %d 轮的身份文件里没有身份块：%q", i+1, string(body))
		}
		if firstPath == "" {
			firstPath = path
		} else if path != firstPath {
			t.Errorf("两轮用了不同的身份文件：%q vs %q", firstPath, path)
		}
	}

	// 两轮都成功只说明指纹在轮次之间稳定；它证明不了「发出去的东西」与
	// 「算指纹用的探针」一致。上面逐个检查 Files 才是那条证明。
	for _, call := range fs.calls {
		if len(call.cmd.Stdin) == 0 {
			t.Error("正文没有走 stdin")
		}
	}
}
