package i18n

// messages_cli.go 是命令行入口的两张消息目录（cli.*）。
//
// 覆盖范围：全局 flag 说明、各子命令的用法块、参数错误，以及进程退出前
// 打到 stderr 的启动错误。命令名、flag 名与默认值一个字都不翻——翻译的是
// 说明与提示，不是接口。
//
// 与 tui.* 同一条纪律：调用处用 TF，fallback 传中文原文，所以中文默认
// 输出与改造前逐字节相同。
var zhCLI = Catalog{
	// ---- 全局 flag（council -h） ----
	"cli.flag.config":  "配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml",
	"cli.flag.lang":    "界面语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）",
	"cli.flag.version": "打印版本号后退出，不加载配置",
	"cli.flag.verbose": "详细输出：每轮的用量、金额、冷暖、provider 会话、预算表与结算全部显示（默认简洁；进去之后也可以用 /verbose 切换）",
	"cli.flag.noTUI":   "不用 TUI，走行式 REPL（TUI 在你的终端上出问题时的退路）",
	"cli.flag.noAuto":  "直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）",

	"cli.error.positional": "council 不接受位置参数，多出来的是: %v\n",

	// ---- council commands ----
	"cli.commands.flag.json":        "按 JSON 打印全部命令的元数据（name / aliases / args / summary / detail / scopes），给 GUI 面板与 dsh 工具用",
	"cli.commands.error.positional": "council commands 不接受位置参数，多出来的是: %v\n",

	// ---- 启动与退出（internal/app） ----
	"cli.startup.configPathFailed":      "无法确定配置文件位置: %v\n",
	"cli.startup.configPathHint":        "可以用 --config <path> 直接指定配置文件，或设置环境变量 %s 指向它。\n",
	"cli.startup.rulesFailed":           "规则文件读不出来: %v\n",
	"cli.startup.rulesHint":             "全局规则找的是 %s（可用环境变量 %s 改），局部规则由配置里的 rules_file 指定。\n",
	"cli.startup.rulesMissing":          "文件不存在是正常的（等于没有规则）；这里报错说明它存在却读不了。\n",
	"cli.startup.dataDirInvalid":        "数据目录无效: %v\n",
	"cli.startup.dataDirHint":           "data_dir 必须是绝对路径，可以写在配置文件的 data_dir 里，或设置环境变量 %s。\n",
	"cli.startup.storeFailed":           "无法打开本地库: %v\n",
	"cli.startup.storeHint":             "数据目录是 %s，可以在配置文件的 data_dir 里改，或设置环境变量 %s。\n",
	"cli.startup.storeCloseFailed":      "关闭本地库失败: %v\n",
	"cli.startup.conversationsFailed":   "读取会话列表失败: %v\n",
	"cli.startup.conversationsHint":     "库文件是 %s。\n",
	"cli.startup.inflightFailed":        "启动在飞对账失败: %v\n",
	"cli.startup.settleFailed":          "启动在飞对账补账失败: %v\n",
	"cli.startup.driftFailed":           "启动漂移审计失败: %v\n",
	"cli.startup.replFailed":            "REPL 异常结束: %v\n",
	"cli.startup.configMissing":         "找不到配置文件: %s\n",
	"cli.startup.configMissingExplicit": "这个路径是你用 --config 或环境变量 %s 指定的，请确认它没写错。\n",
	"cli.startup.configMissingHint":     "可以用 --config <path> 指定别处的配置文件，或设置环境变量 %s 指向它。\n",
	"cli.startup.configBroken":          "配置文件有问题: %s\n",

	// 在飞对账与漂移提醒（横幅的一部分）
	"cli.notice.lostHead":         "注意: 有 %d 轮在上次退出时没记上账，其中 %d 轮已把相关成员的成本基线标为可疑\n",
	"cli.notice.lostRow":          "  会话 %s / 成员 %s / turn %s / 起于 %s / phase %s: %s\n",
	"cli.notice.lostFolded":       "  其余 %d 条已折叠\n",
	"cli.notice.driftHeadRebuild": "注意: 有 %d 条 provider 会话的身份已变，下次 /ask 会各重建一次（每条一次入场费）\n",
	"cli.notice.driftHeadRefused": "注意: 有 %d 条 provider 会话按当前配置 /ask 会被拒，不花钱；其中原因是「配置里已无此 agent」的，先 /use 那个会话再敲 /session rebuild <成员> 即可清掉\n",
	"cli.notice.driftRow":         "  会话 %s / 成员 %s: %s\n",

	// ---- council ask：flag 说明 ----
	"cli.ask.flag.prompt":       "问题正文；也可以写成 -- 之后的位置参数",
	"cli.ask.flag.members":      "逗号分隔的 agent id，按给出顺序发言",
	"cli.ask.flag.team":         "团队 id；与 -members 互斥",
	"cli.ask.flag.mode":         "ask＝各成员说一轮；round＝多轮，语义同 /round",
	"cli.ask.flag.rounds":       "仅 round 生效；ask 下必须是 1",
	"cli.ask.flag.summary":      "汇总成员；none 显式关闭（round 下不允许关闭）",
	"cli.ask.flag.budget":       "本次调用的硬上限，单位 USD，不得小于 0.5",
	"cli.ask.flag.timeout":      "整体超时；到点取消全部在飞",
	"cli.ask.flag.conversation": "续接已有会话；未知 id 退 1",
	"cli.ask.flag.workspace":    "在 git 仓库的隔离 worktree 副本里跑，跑完返回 diff",
	"cli.ask.flag.json":         "stdout 只输出一个 JSON 对象",
	"cli.ask.flag.verbose":      "逐成员进度写 stderr",
	"cli.ask.flag.config":       "配置文件路径，与全局 -config 含义相同",
	"cli.ask.flag.lang":         "输出语言：zh / en / auto",

	// ---- council ask：用法块 ----
	"cli.ask.usage": `用法:
  council ask [flags] -- <prompt>
  council ask [flags] -p <prompt>

flags:
  -p <prompt>          问题正文；与 -- 后的位置参数二选一
  -members <ids>       逗号分隔的 agent id，按给出顺序发言
  -team <id>           团队 id；与 -members 互斥
  -mode ask|round      ask＝各成员说一轮（默认）；round＝多轮
  -rounds <n>          仅 round 生效，默认 1；ask 下必须是 1
  -summary <id|none>   汇总成员；默认取配置 [summary].agent，round 下不能用 none
  -budget-usd <n>      硬上限，默认 5，最小 0.5
  -timeout <duration>  整体超时，默认 10m
  -conversation <id>   续接已有会话
  -workspace <dir>     在 git 仓库的隔离 worktree 副本里跑，返回 diff
  -json                stdout 只输出一个 JSON 对象
  -verbose             逐成员进度写 stderr
  -config <path>       配置文件路径
  -lang zh|en|auto     输出语言，默认 auto
`,

	// ---- council ask：参数错误 ----
	"cli.ask.err.pAndPositional":    "-p 与 -- 后的位置参数只能给一个",
	"cli.ask.err.promptRequired":    "必须用 -p <prompt> 或 -- <prompt> 给出问题",
	"cli.ask.err.membersAndTeam":    "-members 与 -team 不能同时给",
	"cli.ask.err.membersEmpty":      "-members 给了但名单是空的",
	"cli.ask.err.workspaceEmpty":    "-workspace 给了但目录是空的",
	"cli.ask.err.promptBlank":       "问题正文不能为空",
	"cli.ask.err.askRounds":         "-mode ask 下 -rounds 必须是 1，实际 %d",
	"cli.ask.err.roundRounds":       "-mode round 下 -rounds 至少是 1，实际 %d",
	"cli.ask.err.modeInvalid":       "-mode 只认 ask 或 round，实际 %q",
	"cli.ask.err.budgetTooSmall":    "-budget-usd 不得小于 0.5，实际 %v",
	"cli.ask.err.timeout":           "-timeout 必须大于 0，实际 %v",
	"cli.ask.err.allJudges":         "名单里的成员全是 judge 模式，没有可发言的人：%s",
	"cli.ask.err.unknownTeam":       "未知团队 %q：配置里没有这个 team",
	"cli.ask.err.unknownMember":     "未知成员 %q：配置里没有这个 agent",
	"cli.ask.err.membersBlank":      "成员名单为空",
	"cli.ask.err.roundNoSummary":    "-mode round 必须有汇总成员，不能用 -summary none 关掉",
	"cli.ask.err.roundNoSummaryCfg": "-mode round 没有汇总成员：在配置里写 [summary].agent，或用 -summary 指定",
	"cli.ask.err.unknownSummary":    "未知汇总成员 %q：配置里没有这个 agent",
	"cli.ask.err.summaryReadOnly":   "汇总成员 %q 必须是 read 权限，实际 %q",

	// ---- council ask：执行期提示（stderr） ----
	"cli.ask.err.configMissing": "council ask: 配置未加载，无法执行",
	"cli.ask.err.notReady":      "council ask: 持久化或 provider 调用未就绪，无法执行",
	"cli.ask.workspaceCreated":  "council: -workspace 为本次调用新建 worktree 副本：%s（跑完返回 diff，不自动应用）\n",
	"cli.ask.skipped":           "council: 跳过 %s：%s\n",
	"cli.ask.judgesIgnored":     "裁判 %s 本命令不问：council ask 不跑裁判轮。\n",
	"cli.ask.roundOf":           "council: 第 %d/%d 轮，问 %d 个成员\n",
	"cli.ask.summaryAgentLeft":  "council: 汇总成员 %s 已退出当前会话，本次不汇总\n",
	"cli.ask.summaryNoReply":    "council: 汇总成员 %s 没有回复（%s）\n",
	"cli.ask.writeFailed":       "council ask: 写结果失败: %v\n",
	"cli.ask.partial":           "council: partial: %d/%d 成员失败\n",
	"cli.ask.budgetExceeded":    "council: 预算触线：已花 %.4f USD，上限 %.2f USD\n",

	// ---- council ask：文本模式（-json 为假） ----
	"cli.ask.text.statusLine": "状态: %s\n会话: %s\n成员: %s\n金额: %s USD\n",
	"cli.ask.text.incomplete": "（不完全：%d 位成员不报金额）",
	"cli.ask.text.turnHead":   "\n%s 第 %d 轮 %s（%s/%s，%dms，%.4f USD）\n",
	"cli.ask.text.noText":     "（无正文）",
	"cli.ask.text.workspace":  "\n工作区副本: %s\n改动文件: %s\n",
	"cli.ask.text.noFiles":    "（无）",
	"cli.ask.text.diffTrunc":  "diff 超过 1 MiB，全文: %s\n",
	"cli.ask.text.diff":       "改动:\n%s\n",
	"cli.ask.text.summary":    "\n汇总:\n%s\n",

	// ---- council serve：flag 说明 ----
	"cli.serve.flag.addr":    "监听地址，必须是回环地址（127.0.0.0/8、::1 或 localhost）",
	"cli.serve.flag.open":    "启动后调用系统浏览器打开看板；打不开只提示，不影响服务",
	"cli.serve.flag.app":     "用 Edge/Chrome 的应用窗口打开看板，没有地址栏与标签页（仅 Windows；找不到浏览器或启动失败时退回 -open 的行为）",
	"cli.serve.flag.config":  "配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml",
	"cli.serve.flag.lang":    "界面语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）",
	"cli.serve.flag.verbose": "每个请求打一行到 stderr",
	"cli.serve.flag.rw":      "写模式：拿 council.lock，网页可以跑议会（会花你的额度；此时 TUI 不能再开）",

	// ---- council mcp：flag 说明 ----
	"cli.mcp.flag.config":  "配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml",
	"cli.mcp.flag.lang":    "工具输出语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）",
	"cli.mcp.flag.verbose": "每个请求与每次工具调用都打一行到 stderr",

	// ---- council mcp：用法块与参数错误 ----
	"cli.mcp.err.positional": "council mcp 不接受位置参数，多出来的是: %v",
	"cli.mcp.usage": `用法:
  council mcp [flags]

在 stdin / stdout 上跑一个 MCP 服务器：JSON-RPC 2.0，一行一条消息。
stdout 只有协议消息，所有日志走 stderr；stdin 读到底、跑完在飞的工具后退出。
同一个 data_dir 只能有一个写进程，别在 TUI 或 serve -rw 开着时再开它。

flags:
  -config <path>       配置文件路径，与全局 -config 含义相同
  -lang zh|en|auto     工具输出语言，默认 auto
  -verbose             每个请求与每次工具调用都写一行 stderr

工具:
  council_ask          问议会一轮（可指定成员、预算、工作目录、续接会话）
  council_models       列出可用成员（配置 + .claude/agents 导入）
  council_round        多轮议会 + 汇总（1..5 轮）
`,

	// ---- council serve：用法块 ----
	"cli.serve.usage": `用法: council serve [flags]

默认只读看板：在浏览器里看本机的会话、消息、每轮用量与费用。
不写库、不跑迁移、不抢 council.lock，TUI 开着时也能读同一个库。
-rw 写模式：拿 council.lock、可迁移，网页里可以跑一次议会并花你的额度。
拿不到锁时直接退出；写模式开着时不要再开 TUI。
只允许绑定回环地址；Ctrl-C 优雅关闭。
-app（仅 Windows）：用 Edge 或 Chrome 的 --app 窗口打开看板，没有地址栏、没有标签页。
  窗口默认 1440x900，profile 在 <data_dir>\ui-profile，不碰你日常浏览器的会话与扩展。
  找不到 Edge/Chrome 或启动失败时打印一行说明，退回 -open 的行为；与 -open 同给时以 -app 为准。
  窗口是独立的浏览器进程：serve 退出之后它仍然开着，要关请自己关那个窗口。

Flags:
`,

	// ---- council serve：参数错误 ----
	"cli.serve.err.positional":      "不接受位置参数，多出来的是: %v",
	"cli.serve.err.addrInvalid":     "地址 %q 不合法，应写成 127.0.0.1:8765: %v",
	"cli.serve.err.addrNoHost":      "地址 %q 没有指定主机；必须写回环地址（例如 127.0.0.1:8765）",
	"cli.serve.err.addrNoPort":      "地址 %q 没有指定端口",
	"cli.serve.err.addrBadPort":     "地址 %q 的端口 %q 不合法，必须是 0 到 65535 的整数",
	"cli.serve.err.addrNotLoopback": "只允许绑定回环地址（127.0.0.0/8、::1、localhost），%q 不是回环地址",
	// ---- 命令细则输出（本片；中文值 = 调用处的 fallback 原文） ----

	"cli.add.comment":            "council /add 追加：%s 识别出的 %s（%s）",
	"cli.add.backupPrefix":       "备份: ",
	"cli.add.id.exists":          "配置里已经有 agent %s",
	"cli.add.id.folds":           "id %s 与已有的 agent %s 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录",
	"cli.add.id.hint":            "；用 --id <新 id> 换一个",
	"cli.add.id.invalid":         "id %s 不合法（要匹配 %s）",
	"cli.add.id.reserved":        "id %s 是保留名",
	"cli.add.id.windowsDevice":   "id %s 是 Windows 保留设备名，不能当工作目录名",
	"cli.add.readOriginalPrefix": "读原文件: ",
	"cli.add.writePrefix":        "写入: ",
}

var enCLI = Catalog{
	"cli.flag.config":  "Config file path; takes precedence over COUNCIL_CONFIG and the default ~/.council/config.toml",
	"cli.flag.lang":    "UI language: zh / en / auto (auto, the default, checks COUNCIL_LANG, then language in the config, then the system locale)",
	"cli.flag.version": "Print the version and exit without loading the config",
	"cli.flag.verbose": "Verbose output: per-turn usage, cost, warmth, provider session, budget table and settlement (concise by default; /verbose toggles it inside)",
	"cli.flag.noTUI":   "Skip the TUI and use the line-mode REPL (the fallback when the TUI misbehaves in your terminal)",
	"cli.flag.noAuto":  "Typed text is only recorded, members do not reply (the default asks the current conversation's members; /auto toggles it inside)",

	"cli.error.positional": "council does not accept positional arguments; got: %v\n",

	"cli.commands.flag.json":        "Print all command metadata as JSON (name / aliases / args / summary / detail / scopes) for the GUI panel and dsh tools",
	"cli.commands.error.positional": "council commands does not accept positional arguments; got: %v\n",

	"cli.startup.configPathFailed":      "Cannot determine the config file location: %v\n",
	"cli.startup.configPathHint":        "Pass --config <path>, or set the %s environment variable to point at it.\n",
	"cli.startup.rulesFailed":           "Cannot read the rules file: %v\n",
	"cli.startup.rulesHint":             "The global rules file is %s (override with %s); the local one comes from rules_file in the config.\n",
	"cli.startup.rulesMissing":          "A missing file is normal (it means no rules); this error means it exists but cannot be read.\n",
	"cli.startup.dataDirInvalid":        "Invalid data directory: %v\n",
	"cli.startup.dataDirHint":           "data_dir must be an absolute path; set it in the config or via the %s environment variable.\n",
	"cli.startup.storeFailed":           "Cannot open the local database: %v\n",
	"cli.startup.storeHint":             "The data directory is %s; change data_dir in the config or set the %s environment variable.\n",
	"cli.startup.storeCloseFailed":      "Failed to close the local database: %v\n",
	"cli.startup.conversationsFailed":   "Failed to read the conversation list: %v\n",
	"cli.startup.conversationsHint":     "The database file is %s.\n",
	"cli.startup.inflightFailed":        "Startup in-flight reconciliation failed: %v\n",
	"cli.startup.settleFailed":          "Startup in-flight settlement failed: %v\n",
	"cli.startup.driftFailed":           "Startup drift audit failed: %v\n",
	"cli.startup.replFailed":            "REPL ended unexpectedly: %v\n",
	"cli.startup.configMissing":         "Config file not found: %s\n",
	"cli.startup.configMissingExplicit": "This path came from --config or the %s environment variable; please check it.\n",
	"cli.startup.configMissingHint":     "Pass --config <path> to point at another config, or set the %s environment variable.\n",
	"cli.startup.configBroken":          "The config file has problems: %s\n",

	"cli.notice.lostHead":         "Note: %d turns from the last exit were never recorded; %d of them marked the related members' cost baseline as suspect\n",
	"cli.notice.lostRow":          "  conversation %s / member %s / turn %s / started %s / phase %s: %s\n",
	"cli.notice.lostFolded":       "  %d more folded\n",
	"cli.notice.driftHeadRebuild": "Note: %d provider sessions changed identity; the next /ask rebuilds each of them (one entry fee each)\n",
	"cli.notice.driftHeadRefused": "Note: %d provider sessions would be refused by /ask under the current config, at no cost; for the ones whose agent is gone from the config, /use that conversation and run /session rebuild <member> to clear them\n",
	"cli.notice.driftRow":         "  conversation %s / member %s: %s\n",

	"cli.ask.flag.prompt":       "Prompt text; alternatively pass it as the positional argument after --",
	"cli.ask.flag.members":      "Comma-separated agent ids, speaking in the given order",
	"cli.ask.flag.team":         "Team id; mutually exclusive with -members",
	"cli.ask.flag.mode":         "ask = each member speaks one turn; round = multiple rounds, same semantics as /round",
	"cli.ask.flag.rounds":       "Only for round; must be 1 under ask",
	"cli.ask.flag.summary":      "Summary member; none disables it (not allowed under round)",
	"cli.ask.flag.budget":       "Hard cap for this call in USD; at least 0.5",
	"cli.ask.flag.timeout":      "Overall timeout; cancels everything in flight when reached",
	"cli.ask.flag.conversation": "Resume an existing conversation; unknown ids exit 1",
	"cli.ask.flag.workspace":    "Run in an isolated git worktree copy and return the diff",
	"cli.ask.flag.json":         "Write exactly one JSON object to stdout",
	"cli.ask.flag.verbose":      "Per-member progress on stderr",
	"cli.ask.flag.config":       "Config file path; same meaning as the global -config",
	"cli.ask.flag.lang":         "Output language: zh / en / auto",

	"cli.ask.usage": `Usage:
  council ask [flags] -- <prompt>
  council ask [flags] -p <prompt>

flags:
  -p <prompt>          prompt text; use either this or the positional argument after --
  -members <ids>       comma-separated agent ids, speaking in the given order
  -team <id>           team id; mutually exclusive with -members
  -mode ask|round      ask = each member speaks one turn (default); round = multiple rounds
  -rounds <n>          only for round, default 1; must be 1 under ask
  -summary <id|none>   summary member; defaults to [summary].agent, none not allowed under round
  -budget-usd <n>      hard cap, default 5, minimum 0.5
  -timeout <duration>  overall timeout, default 10m
  -conversation <id>   resume an existing conversation
  -workspace <dir>     run in an isolated git worktree copy and return the diff
  -json                write exactly one JSON object to stdout
  -verbose             per-member progress on stderr
  -config <path>       config file path
  -lang zh|en|auto     output language, default auto
`,

	"cli.ask.err.pAndPositional":    "give the prompt either with -p or as the positional argument after --, not both",
	"cli.ask.err.promptRequired":    "a prompt is required: use -p <prompt> or -- <prompt>",
	"cli.ask.err.membersAndTeam":    "-members and -team cannot be given together",
	"cli.ask.err.membersEmpty":      "-members was given but the list is empty",
	"cli.ask.err.workspaceEmpty":    "-workspace was given but the directory is empty",
	"cli.ask.err.promptBlank":       "the prompt must not be empty",
	"cli.ask.err.askRounds":         "under -mode ask, -rounds must be 1, got %d",
	"cli.ask.err.roundRounds":       "under -mode round, -rounds must be at least 1, got %d",
	"cli.ask.err.modeInvalid":       "-mode only accepts ask or round, got %q",
	"cli.ask.err.budgetTooSmall":    "-budget-usd must be at least 0.5, got %v",
	"cli.ask.err.timeout":           "-timeout must be greater than 0, got %v",
	"cli.ask.err.allJudges":         "every member in the list has judge mode; nobody can speak: %s",
	"cli.ask.err.unknownTeam":       "unknown team %q: no such team in the config",
	"cli.ask.err.unknownMember":     "unknown member %q: no such agent in the config",
	"cli.ask.err.membersBlank":      "the member list is empty",
	"cli.ask.err.roundNoSummary":    "under -mode round a summary member is required; -summary none cannot be used",
	"cli.ask.err.roundNoSummaryCfg": "under -mode round there is no summary member: set [summary].agent in the config or pass -summary",
	"cli.ask.err.unknownSummary":    "unknown summary member %q: no such agent in the config",
	"cli.ask.err.summaryReadOnly":   "summary member %q must have read permission, got %q",

	"cli.ask.err.configMissing": "council ask: config not loaded; cannot run",
	"cli.ask.err.notReady":      "council ask: storage or provider calls are not ready; cannot run",
	"cli.ask.workspaceCreated":  "council: -workspace created a worktree copy for this call: %s (the diff is returned afterwards, never applied automatically)\n",
	"cli.ask.skipped":           "council: skipping %s: %s\n",
	"cli.ask.judgesIgnored":     "judge %s is not asked by this command: council ask does not run judge rounds.\n",
	"cli.ask.roundOf":           "council: round %d/%d, asking %d members\n",
	"cli.ask.summaryAgentLeft":  "council: summary member %s has left this conversation; no summary this time\n",
	"cli.ask.summaryNoReply":    "council: summary member %s did not reply (%s)\n",
	"cli.ask.writeFailed":       "council ask: failed to write the result: %v\n",
	"cli.ask.partial":           "council: partial: %d/%d members failed\n",
	"cli.ask.budgetExceeded":    "council: budget exceeded: spent %.4f USD, cap %.2f USD\n",

	"cli.ask.text.statusLine": "status: %s\nconversation: %s\nmembers: %s\ncost: %s USD\n",
	"cli.ask.text.incomplete": " (incomplete: %d members do not report cost)",
	"cli.ask.text.turnHead":   "\n%s round %d %s (%s/%s, %dms, %.4f USD)\n",
	"cli.ask.text.noText":     "(no text)",
	"cli.ask.text.workspace":  "\nworkspace copy: %s\nchanged files: %s\n",
	"cli.ask.text.noFiles":    "(none)",
	"cli.ask.text.diffTrunc":  "diff exceeds 1 MiB; full text: %s\n",
	"cli.ask.text.diff":       "changes:\n%s\n",
	"cli.ask.text.summary":    "\nsummary:\n%s\n",

	"cli.serve.flag.addr":    "Listen address; must be a loopback address (127.0.0.0/8, ::1 or localhost)",
	"cli.serve.flag.open":    "Open the dashboard in the system browser after start; a failure only prints a note",
	"cli.serve.flag.app":     "Open the dashboard in an Edge/Chrome app window without address bar or tabs (Windows only; falls back to -open when the browser is missing or fails)",
	"cli.serve.flag.config":  "Config file path; takes precedence over COUNCIL_CONFIG and the default ~/.council/config.toml",
	"cli.serve.flag.lang":    "UI language: zh / en / auto (auto, the default, checks COUNCIL_LANG, then language in the config, then the system locale)",
	"cli.serve.flag.verbose": "Print one line per request to stderr",
	"cli.serve.flag.rw":      "Write mode: take council.lock and let the web UI run councils (spends your quota; the TUI cannot be open at the same time)",

	"cli.mcp.flag.config":  "Config file path; takes precedence over COUNCIL_CONFIG and the default ~/.council/config.toml",
	"cli.mcp.flag.lang":    "Language of tool output: zh / en / auto (auto checks COUNCIL_LANG, then language in the config, then the system locale)",
	"cli.mcp.flag.verbose": "Print one line per request and per tool call to stderr",

	"cli.mcp.err.positional": "council mcp does not accept positional arguments; got: %v",
	"cli.mcp.usage": `Usage:
  council mcp [flags]

Run an MCP server over stdin/stdout: JSON-RPC 2.0, one message per line.
stdout carries protocol messages only; all logs go to stderr. It exits after stdin ends and in-flight tools finish.
Only one process may write a data_dir: do not start it while the TUI or serve -rw is running.

flags:
  -config <path>       config file path, same meaning as the global -config
  -lang zh|en|auto     language of tool output, default auto
  -verbose             print one line per request and per tool call to stderr

tools:
  council_ask          ask the council once (members, budget, cwd, continue a conversation)
  council_models       list available members (config + .claude/agents imports)
  council_round        multi-round debate + summary (1..5 rounds)
`,

	"cli.serve.usage": `Usage: council serve [flags]

Read-only dashboard by default: browse local conversations, messages and per-round usage/cost.
It does not write the database, run migrations, or take council.lock, so it can read while the TUI is open.
-rw write mode: takes council.lock, may migrate, and lets the web UI run a council (spending your quota).
It exits when the lock is unavailable; do not open the TUI while write mode is running.
Only loopback addresses can be bound; Ctrl-C shuts down gracefully.
-app (Windows only): open the dashboard in an Edge or Chrome --app window, without address bar or tabs.
  The window defaults to 1440x900 and its profile lives in <data_dir>\ui-profile, untouched by your daily browser profile.
  When Edge/Chrome is missing or fails to start, a note is printed and -open behavior applies; -app wins when both are given.
  The window is a separate browser process: it stays open after serve exits, close it yourself.

Flags:
`,

	"cli.serve.err.positional":      "does not accept positional arguments; got: %v",
	"cli.serve.err.addrInvalid":     "invalid address %q; write it as 127.0.0.1:8765: %v",
	"cli.serve.err.addrNoHost":      "address %q has no host; a loopback address is required (for example 127.0.0.1:8765)",
	"cli.serve.err.addrNoPort":      "address %q has no port",
	"cli.serve.err.addrBadPort":     "address %q has an invalid port %q; it must be an integer from 0 to 65535",
	"cli.serve.err.addrNotLoopback": "only loopback addresses are allowed (127.0.0.0/8, ::1, localhost); %q is not one",
	// ---- command-detail output (this slice) ----

	"cli.add.comment":            "council /add appended: %s model %s (%s)",
	"cli.add.backupPrefix":       "backup: ",
	"cli.add.id.exists":          "the config already has agent %s",
	"cli.add.id.folds":           "id %s and the existing agent %s differ only by dots, underscores or hyphens; claude would put both working directories under the same project directory",
	"cli.add.id.hint":            "; use --id <new id> to pick another",
	"cli.add.id.invalid":         "id %s is invalid (must match %s)",
	"cli.add.id.reserved":        "id %s is a reserved name",
	"cli.add.id.windowsDevice":   "id %s is a Windows reserved device name and cannot be a working directory name",
	"cli.add.readOriginalPrefix": "read original: ",
	"cli.add.writePrefix":        "write: ",
}
