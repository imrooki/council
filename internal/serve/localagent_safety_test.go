package serve

// localagent_safety_test.go 是 S-05（Local Agent 新增模型调用为 0）在 HTTP 层的
// 机器可读证据。
//
// 与 internal/localagent 的 safety_test.go 同一体例：只扫本片新增的那个
// 生产文件，出现「会起进程 / 会读文件 / 会跑模型」的符号就红。整包扫描会
// 被无关代码里的 os.Open 撞红，所以范围只圈 localagent.go 与它的写入端点。
//
// 为什么这条值得单独存在：这一片的诱惑是「顺手在接口里跑一次模型推理」
// （比如让 /api/local-agents 解释一下这些进程）。HUD 上写「新增模型调用：0」
// 是这个模块对用户的核心承诺，它必须有一处能失败的地方。

import (
	"os"
	"strings"
	"testing"
)

func TestLocalAgentEndpointNeverCallsAModel(t *testing.T) {
	forbidden := []string{
		"headless.Run",  // 唯一会起模型进程的出口
		"turn.NewRunner",
		"turn.Runner",
		".Ask(",         // Runner 的回合入口
		"exec.Command",  // 起子进程
		"os.Getenv",     // 环境变量
		"os.ReadFile(",  // 读文件
		"os.Open(",
		"os.WriteFile",
	}

	body, err := os.ReadFile("localagent.go")
	if err != nil {
		t.Fatalf("读 localagent.go 失败: %v", err)
	}
	text := string(body)
	for _, token := range forbidden {
		if strings.Contains(text, token) {
			t.Errorf("internal/serve/localagent.go 里出现了 %q；本机 agent 接口只许读事实，不许起进程或跑模型（S-05 / S-07）", token)
		}
	}

	// 正对照：这个文件确实在做它该做的事。少了它，上面那串断言在
	// 「文件被清空」或「改名」时也会绿。
	for _, want := range []string{
		"localagent.Observe(",
		"localagent.MergeManaged(",
		"s.store.GrantedLeases(",
		"localagent.ResourceOverlaps(",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("internal/serve/localagent.go 里缺少 %q：本片的核心路径不见了", want)
		}
	}
}
