package config

// localagentwrite.go 是设置页第 6 类「本机 agent」那一行层级下拉的写盘实现。
//
// 它复用 langwrite.go 的 setTopLevelKey 与 agentwrite.go 的 writeAgentFile：
// 只写 layers[0]、写前备份、写后按「临时文件当最高层」重新合并 + Validate。
// 为什么不为这个键另写一套：local_agent 与 language 在 TOML 里的位置规则
// 完全一样（都要写在所有表之前），差别只有键名与取值。
//
// 取值范围在 Validate（validLocalAgentLevel）里守，这里只做归一：
// 小写 + 去空白 + 空串按 off。写盘的最后一关仍然是 writeAgentFile 里的
// LoadMerged —— 它会把非法取值变成 400 且目标文件一个字节不动。

import (
	"strings"
)

// LocalAgentValue 把界面上的选择归一成写进配置文件的字面值。
//
// 空串按 off：与 Load 的默认值同一条规则，避免文件里出现 local_agent = ""
// 这种「既不是默认值、语义上又等于 off」的写法。
func LocalAgentValue(level string) string {
	value := strings.ToLower(strings.TrimSpace(level))
	if value == "" {
		return LocalAgentOff
	}
	return value
}

// WriteLocalAgent 把 local_agent 写进 layers[0]，返回与其它设置写盘相同的
// 结果形状。
func WriteLocalAgent(layers []Layer, level string) (AgentWriteResult, error) {
	value := LocalAgentValue(level)
	return writeAgentFile(layers, func(orig []byte) ([]byte, bool, error) {
		return setTopLevelKey(orig, "local_agent", value)
	})
}
