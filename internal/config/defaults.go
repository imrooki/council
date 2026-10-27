package config

// Default 返回一份能通过 Validate 的最小配置。
// DataDir 留空，由 ResolveDataDir 按环境变量与 DefaultDataDir 回落。
func Default() *Config {
	return &Config{
		DataDir: "",
		// StoreThought 的自然零值是 false，与设计默认值相反（见 schema.go 的
		// 注释），Default() 不经过 Load 的预置逻辑，必须在这里显式写 true。
		StoreThought: true,
		Language:     LanguageAuto,
		// 本机 agent 观察默认关闭（S-01）：Home 里也把这句话写死，不靠
		// 「空串恰好等于 off」这条隐含约定。
		LocalAgent: LocalAgentOff,
		Providers: map[string]ProviderConfig{
			"claude": {Command: "claude"},
		},
		Agents: map[string]AgentConfig{
			"theory": {
				DisplayName: "Theory",
				Provider:    "claude",
				Model:       "opus",
				Effort:      "high",
				Role:        "只做一阶剪切变形理论的推导与校核",
				Permissions: PermissionRead,
				Mode:        ModeDiscussion,
			},
			"critic": {
				DisplayName: "Critic",
				Provider:    "claude",
				Model:       "opus",
				Role:        "质疑推导中的假设与遗漏",
				Permissions: PermissionRead,
				Mode:        ModeDiscussion,
			},
		},
		Teams: map[string]TeamConfig{
			"project": {
				DisplayName: "<project> 讨论组",
				Members:     []string{"theory", "critic"},
			},
		},
	}
}

// DefaultTOML 返回配置文件开头的说明注释。#48 起全是注释行，没有任何表，
// 单独加载是一份零 provider、零成员的合法配置。首次运行生成的配置以它开头
// （FirstRunTOML），后面接找到的 [providers.*]。
func DefaultTOML() string {
	return `# Council 配置文件。表键即 id，须唯一。
# 手改之后在 council 里敲 /config reload --apply 生效（data_dir 与 rules_file 只能重启生效）。
#
# id 须匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$：可以含点号，但不许以点号开头或结尾；
# id 含点号时表名要加引号，例如 [agents."grok-4.7"]（TOML 把不加引号的点号当成层级）。
# user 与 system 是保留名，不能当 agent id。
#
# data_dir: 数据目录（本地库与成员的工作目录）。填写则必须是绝对路径。
# 不写时按环境变量 COUNCIL_DATA_DIR，再按内置默认 ` + DefaultDataDir + `；目录不存在时 council 启动会建它。
# data_dir = 'D:\council-data'
#
# rules_file: 局部规则文件，相对本文件所在目录解析。
# 全局规则固定在 ~/.council/COUNCIL.md，不在这里写。
# rules_file = 'COUNCIL.md'
#
# store_thought: 是否把 provider 的 thought/思维链摘要存进消息记录。默认 true。
# store_thought = true
#
# language: 界面语言，zh / en / auto。默认 auto：依次看环境变量 COUNCIL_LANG、
# 本键、系统区域；都不是英文/中文时回落到中文。
# language = 'auto'
#
# local_agent: 本机 agent 观察（可选模块）。off（默认）或 observe。
# off 时一次进程都不枚举、不起后台任务；observe 之后 council who 与 MCP 的
# council_who 才会只读地列出本机 claude / codex / grok / agy 进程。
# local_agent = 'off'
#
# [providers.<id>]: council 去哪找这家工具。id 是 claude、grok、agy、codex 之一，
# command 写可执行文件的绝对路径，或 PATH 上的名字。
#
# 成员不用手写：在 council 里敲 /new <标题> 开一个会话，敲 /add 看识别出的模型，
# 再用 /add +编号 挑成员，council 会把那一段 [agents.<id>] 追加到本文件末尾。
# 手写的话照这个格式：
# [agents.sonnet-5-high]
# display_name = 'Sonnet 5'
# provider     = 'claude'
# model        = 'claude-sonnet-5'
# effort       = 'high'
# permissions  = 'read'
# mode         = 'discussion'
# model 与 effort 由对应的工具解释，council 不做枚举校验；effort 可以不写。
# permissions: read / workspace / full，缺省 read。
# 注意：full 档能写进配置并通过校验，不等于编排器可以自动使用它，见 docs/06_SECURITY_MODEL.md 第 2 节。
# mode: discussion / worker / discussion_and_worker / judge / manager，缺省 discussion。
#
# 总结成员：多个成员回复之后，由 [summary] 指定的成员出一段总结，只给你看。
# 先用 /add 加一个成员（它的 permissions 必须是 read），再在本文件末尾加两行：
# [summary]
# agent = '<那个成员的 id>'
#
# 分组：[teams.<id>] 的 members 列出成员 id，/team use <id> 把他们一次加进当前会话。
# [teams.review]
# members = ['sonnet-5-high', 'grok-4.7']
`
}
