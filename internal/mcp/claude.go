package mcp

import (
	"fmt"
	"sort"
	"strings"

	"council/internal/config"
)

// ClaudeImport 是一次「把 .claude/agents 并进本次运行」的结果。
//
// 与 `serve -rw` 的 importClaudeAgents 是同一套语义（只读、只在内存里、
// 配置优先、缺 [providers.claude] 时整体跳过、超长 prompt 跳过不截断），
// 但放在这里而不是 import internal/serve：`council mcp` 不该为了读一个
// 目录把整个 Web 服务包拖进依赖图。两边真正共享的是 config.ScanClaudeAgents
// 与 config.MaxAgentPromptBytes，那两条才是会漂的东西。
type ClaudeImport struct {
	// Merged 是真正并进 cfg.Agents 的成员（id -> 定义）。council_models
	// 靠它区分 source=claude 与 source=config：合并之后单看 cfg.Agents
	// 已经分不出来源了。
	Merged map[string]config.AgentConfig
	// WithoutProvider 是扫到了、但因为配置缺 [providers.claude] 而没有
	// 合并的成员。council_models 用它在末尾补一句「为什么看不到它们」。
	WithoutProvider map[string]config.AgentConfig
	// Notices 只在 -verbose 时打印（跳过坏文件、同名冲突、成功合并了几条）。
	Notices []string
	// Warnings 不藏在 -verbose 后面：它们都指向一个可执行的配置动作，
	// 或者解释「为什么工具里少了一批成员」。
	Warnings []string
}

// ImportClaudeAgents 扫描 cwd 下的 Claude agents，并在 provider 存在时把
// 它们合并进 cfg.Agents。
//
// 调用点必须在 turn.NewRunner 之前：Runner 持有 cfg 指针，之后原地改
// Agents 会破坏它「交给 Runner 之后配置不再变」的不变量（internal/turn/
// runner.go 的 SetConfig 注释）。internal/app 的 MCP 分支就是照这个顺序
// 装配的。
//
// 合并后不再跑 config.Validate：Claude 的 model 允许为空，而配置校验要求
// 非空。这个限制对导入成员不适用；但「每轮都会进系统提示」的 prompt 上限
// 仍然有效，所以在下面单独挡。
func ImportClaudeAgents(cfg *config.Config, cwd string) ClaudeImport {
	imported := ClaudeImport{
		Merged:          make(map[string]config.AgentConfig),
		WithoutProvider: make(map[string]config.AgentConfig),
	}
	if cfg == nil {
		return imported
	}

	scanned := config.ScanClaudeAgents(cwd, cfg.Agents)
	imported.Notices = append(imported.Notices, scanned.Notices...)
	if len(scanned.Agents) == 0 {
		return imported
	}

	ids := sortedAgentIDs(scanned.Agents)
	if _, ok := cfg.Providers["claude"]; !ok {
		imported.WithoutProvider = scanned.Agents
		imported.Warnings = append(imported.Warnings, fmt.Sprintf(
			"发现 %d 个 Claude agent（%s），但配置里没有 [providers.claude]，已整体跳过："+
				"它们不会出现在 council_models，也不能被 council_ask / council_round 选中。"+
				"要启用请在配置里补上 [providers.claude] 后重启",
			len(ids), strings.Join(ids, ", ")))
		return imported
	}

	if cfg.Agents == nil {
		cfg.Agents = make(map[string]config.AgentConfig, len(scanned.Agents))
	}
	mergedIDs := make([]string, 0, len(scanned.Agents))
	for _, id := range ids {
		if _, exists := cfg.Agents[id]; exists {
			// ScanClaudeAgents 已经用 reserved 挡过同名，这里再挡一次：
			// 任何一次覆盖都会让「配置优先」变成取决于遍历顺序的偶然结果。
			continue
		}
		if n := len(scanned.Agents[id].Prompt); n > config.MaxAgentPromptBytes {
			imported.Warnings = append(imported.Warnings, fmt.Sprintf(
				"跳过 claude agent %q：系统提示 %d 字节，超过 %d 上限",
				id, n, config.MaxAgentPromptBytes))
			continue
		}
		cfg.Agents[id] = scanned.Agents[id]
		imported.Merged[id] = scanned.Agents[id]
		mergedIDs = append(mergedIDs, id)
	}
	if len(mergedIDs) > 0 {
		imported.Notices = append(imported.Notices, fmt.Sprintf(
			"已把 %d 个 Claude agent（%s）合并进本次运行的成员表（仅内存，不写配置文件）",
			len(mergedIDs), strings.Join(mergedIDs, ", ")))
	}
	return imported
}

func sortedAgentIDs(agents map[string]config.AgentConfig) []string {
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
