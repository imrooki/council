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
  council_who          只读列出本机 agent 进程（需要 config 里 local_agent = "observe" 或 "coordinate"；不花额度）
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

	// ---- council init（首次运行 / 桌面版设置向导） ----
	"cli.init.flag.config":           "配置文件路径（默认 ~/.council/config.toml）；与全局 -config 含义相同",
	"cli.init.flag.lang":             "输出语言：zh / en / auto（默认 auto）",
	"cli.init.flag.json":             "按 JSON 输出检测结果（给桌面向导与脚本用），不写任何文件",
	"cli.init.flag.apply":            "从文件读 JSON 并写配置（用 - 表示 stdin）；配置文件已存在时拒绝",
	"cli.init.error.positional":      "council init 不接受位置参数，多出来的是: %v\n",
	"cli.init.error.layers":          "无法确定配置文件位置: %v\n",
	"cli.init.error.write":           "写输出失败: %v\n",
	"cli.init.error.exists":          "配置文件已存在，不覆盖：%s（要改它请用 council serve -rw 的设置页，或自己编辑）",
	"cli.init.error.read":            "读输入失败: %v",
	"cli.init.error.json":            "输入不是合法 JSON: %v",
	"cli.init.error.unknownProvider": "没有检测到这个 provider（或它没装）：%s",
	"cli.init.error.duplicateAgent":  "成员 id 重复：%s",
	"cli.init.error.noProvider":      "至少要选一个 provider",
	"cli.init.error.invalid":         "生成的配置没有通过校验: %v",
	"cli.init.error.writeFile":       "写不进配置文件 %s: %v",
	"cli.init.head":                  "本机 CLI 检测（不会写任何文件）：\n",
	"cli.init.notFound":              "没找到",
	"cli.init.probeFailed":           "识别失败：%v",
	"cli.init.modelCount":            "识别到 %d 个模型",
	"cli.init.configPath":            "配置文件：%s（%s）",
	"cli.init.configExists":          "已存在，不会被覆盖",
	"cli.init.noneFound":             "一个都没找到。至少装一个（claude / codex / grok / agy）并在终端里登录，再回来。\n",
	"cli.init.written":               "已生成配置文件：%s\n  provider：%s\n  成员：%s\n接下来：council serve -rw 打开看板，或直接 council 进终端界面。\n",
	"cli.init.usage": `用法: council init [flags]

检测本机装了哪几家 CLI（claude / codex / grok / agy）、能不能列出模型、登录状态如何；
默认只打印，不写任何文件。桌面版的首次运行向导就是靠它拿到这份结果的。

  council init --json              给程序看的检测结果（不写文件）
  council init --apply <file|->    按 JSON 写配置（配置文件已存在时拒绝）

退出码: 0 成功；1 环境/写盘失败；2 用法错误。

flags:
  -json                按 JSON 输出检测结果
  -apply <file|->      读入勾选结果并写配置；- 表示 stdin
  -config <path>       配置文件路径，默认 ~/.council/config.toml
  -lang zh|en|auto     输出语言，默认 auto
`,

	// ---- council who ----
	"cli.who.flag.json":           "按 JSON 输出（local_agent / managed / observed / leases / notice）",
	"cli.who.flag.config":         "配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml",
	"cli.who.flag.lang":           "输出语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）",
	"cli.who.error.positional":    "council who 不接受位置参数，多出来的是: %v\n",
	"cli.who.configBroken":        "council who: 配置文件有问题: %v\n",
	"cli.who.off":                 `本机 agent 未开启（config 里 local_agent = "observe" 开启观察，"coordinate" 同时开启资源租约）`,
	"cli.who.headManaged":         "Council 管理的成员（%d）",
	"cli.who.managedRow":          "  会话 %s / 成员 %s / turn %s / phase %s / 起于 %s",
	"cli.who.managedEmpty":        "  （没有在飞的成员）",
	"cli.who.managedUnavailable":  "  （库不可用：%v；只显示本机进程部分）",
	"cli.who.headObserved":        "本机其他 agent 进程（%d）",
	"cli.who.observedRow":         "  %s  pid=%d ppid=%d exe=%s source=%s certainty=%s 起于 %s%s",
	"cli.who.observedEmpty":       "  （没有发现本机 agent 进程）",
	"cli.who.observedUnavailable": "  （进程枚举不可用：%v）",
	"cli.who.unknown":             "未知",
	"cli.who.headLeases":          "本机租约（%d）",
	"cli.who.leaseRow":            "  资源 %s / 持有者 %s/%s / 模式 %s / 代数 %d / 到期 %s / lease %s",
	"cli.who.leasesEmpty":         "  （没有有效租约）",
	"cli.who.leasesUnavailable":   "  （租约读不出来：%v）",
	"cli.who.note":                "本命令看的是本机进程，不是会话成员；不读取命令行、环境变量或其它 CLI 的配置；三段不做 PID 合并（.cmd 垫片拿到的是 cmd.exe 的 PID）；租约是提醒，不是文件锁。",
	"cli.who.noteServer":          "这份快照由看板进程给出：它把「这一轮起的子进程」与进程表里的那一行按 PID（含 .cmd 垫片）对过；不读取命令行、环境变量或其它 CLI 的配置；租约是提醒，不是文件锁。",
	"cli.who.servedFrom":          "（以下事实来自看板 %s 的快照：PID 与合并结果只有它拿得到）",
	"cli.who.observedMerged":      " merged=%s（turn %s）",
	"cli.who.flag.server":         "改问正在跑的看板要一份快照（例如 http://127.0.0.1:8765）：只有它拿得到 PID 与合并结果",
	"cli.who.serverFailed":        "council who: 问不到 %s 的快照：%v（地址就是看板启动时打印的那一行）\n",
	"cli.who.usage": `用法: council who [flags]

只读列出三段本机事实，不拿 council.lock，可以在 TUI 或 serve -rw 开着的时候跑：
  1. Council 管理的成员：库里正在飞的 turn
  2. 本机其他 agent 进程：claude / codex / grok / agy（需要 config 里 local_agent = "observe" 或 "coordinate"）
  3. 本机租约：仍然有效的 council claim（租约要 schema v10；旧库这一段只说读不出来）
本命令看的是本机进程，不是会话成员（那是在 council 里敲 /who）。
certainty=exact 只给落在已知 CLI 安装目录里的进程；路径拿不到或不在安装目录里的标 probable，
桌面客户端（Claude Desktop 等）的同名 claude.exe 不报。
不读取命令行、环境变量或其它 CLI 的配置；local_agent = "off"（默认）时一次进程都不枚举。
看板（serve -rw）正握着 council.lock 时，它的快照比跨进程读库多两样东西：子进程 PID 与
「这一轮对应哪个进程」的合并结果 —— 加 -server 就向它要那一份（只读 GET）。

flags:
  -server <url>        问正在跑的看板要快照，例如 http://127.0.0.1:8765（多出 PID 与合并结果）
  -json                按 JSON 输出（local_agent / managed / observed / leases / notice）
  -config <path>       配置文件路径，与全局 -config 含义相同
  -lang zh|en|auto     输出语言，默认 auto
`,
	// ---- council claim / release（L2 资源租约） ----
	"cli.claim.flag.config": "配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml",
	"cli.claim.flag.lang":   "输出语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）",
	"cli.claim.flag.json":   "按 JSON 输出（granted / resource / mode / lease / holder / notice）",
	"cli.claim.flag.mode":   "租约模式：write（独占，默认）或 read（共享）",
	"cli.claim.flag.ttl":    "租约有效期，例如 5m / 1h（默认 2m，最短 5s，最长 24h）",
	"cli.claim.flag.owner":  "持有者名字（默认 cli@主机名）；它只是展示与交还时的匹配条件，不参与鉴权",
	"cli.claim.flag.server": "改走已经持有 council.lock 的看板，例如 http://127.0.0.1:8765；不给就自己开库",
	"cli.claim.busyServer":  "资源被占用，没有拿到租约：\n  %s\n",

	"cli.claim.error.positional": "council claim 需要恰好一个资源参数（目录路径，或 tcp:<host>:<port>），多出来的是: %v\n",
	"cli.claim.notEnabled":       `本机 agent 协调未开启（config 里 local_agent = "coordinate" 可开启）：本命令不写任何租约。`,
	"cli.claim.failed":           "council claim: %v\n",
	"cli.claim.granted": `已获得租约
  资源    %s
  模式    %s
  持有者  %s/%s
  lease   %s
  代数    %d
  到期    %s（%s 之后）
交还：council release %s。租约只写库，不扫描进程、不调用任何模型。
`,
	"cli.claim.busy": `资源被占用，没有拿到租约
  资源    %s
  持有者  %s/%s（%s 租约）
  lease   %s
  到期    %s
先等它到期，或请持有者 council release %s 之后再申请。租约是自愿遵守的提醒，不会阻止任何进程写文件。
`,
	"cli.claim.usage": `用法: council claim <目录|tcp:端口> [flags]

在 council.db（schema v10）里申请一条资源租约。租约是**自愿遵守的提醒**，
不是文件锁：它不阻止任何进程写文件，只是让下一个 agent 知道这个资源有人占着。
本命令零模型调用、零进程扫描，只读写两张表。

开启方式：config 里 local_agent = "coordinate"（设置页第 6 类也能改）。
写库要拿 council.lock，所以 serve -rw / TUI / council mcp 正在跑时会明确拒绝；
那种情况下加 -server 走那个看板，它本来就握着锁。

退出码: 0 拿到租约；3 被拒（被占用 / 层级未开启 / 没有这条租约）；1 出错；2 用法错误。

flags:
  -mode read|write     租约模式，默认 write（独占）；read 可并存
  -ttl <duration>      有效期，例如 5m / 1h（默认 2m，最短 5s，最长 24h）
  -owner <name>        持有者名字，默认 cli@主机名（只是展示与交还匹配，不参与鉴权）
  -server <url>        走已经持有 council.lock 的看板，例如 http://127.0.0.1:8765
  -json                按 JSON 输出（granted / resource / mode / lease / holder / notice）
  -config <path>       配置文件路径，与全局 -config 含义相同（与 -server 互斥）
  -lang zh|en|auto     输出语言，默认 auto
`,

	"cli.release.flag.config":   "配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml",
	"cli.release.flag.lang":     "输出语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）",
	"cli.release.flag.json":     "按 JSON 输出（released / resource / notice）",
	"cli.release.flag.lease":    "要交还的 lease_id（council claim 的输出里有）",
	"cli.release.flag.resource": "按资源交还自己在这个资源上的全部租约（需要 -owner 匹配，默认 cli@主机名）",
	"cli.release.flag.owner":    "持有者名字，与 claim 时给的一致（默认 cli@主机名）；只释放名字完全相同的租约",
	"cli.release.flag.server":   "改走已经持有 council.lock 的看板，例如 http://127.0.0.1:8765；不给就自己开库",
	"cli.release.nothingServer": "看板那边没有可交还的租约（可能已经交还过，或 owner 不匹配）。",

	"cli.release.error.positional": "council release 不接受位置参数，用 -lease 或 -resource 指定目标；多出来的是: %v\n",
	"cli.release.error.target":     "council release 需要一个目标：-lease <lease_id> 或 -resource <目录|tcp:端口>\n",
	"cli.release.error.both":       "council release 只接受一个目标：-lease 与 -resource 不能同时给\n",
	"cli.release.failed":           "council release: %v\n",
	"cli.release.notFound":         "没有这条租约：%s（它可能已经被删掉，或 lease_id 抄错了）",
	"cli.release.already":          "租约 %s 的状态是 %s，不需要再交还（没有改动，也没有写事件）。",
	"cli.release.nothing":          "这个资源上没有 %s 的租约可交还（只释放 owner_id 完全相同的租约，没有 force）。",
	"cli.release.released":         "已交还 %d 条租约\n",
	"cli.release.usage": `用法: council release -lease <lease_id>
       council release -resource <目录|tcp:端口> [-owner <name>]

交还自己申请的租约（state 改成 released 并留一条事件）。交还**不需要**
local_agent = coordinate：关小开关不该让已经发出去的授权无法收回。

只释放 owner_id 完全相同的租约，没有 force 参数 —— 释放别人的租约不在本模块
的能力里。写库同样要拿 council.lock；看板正握着它时加 -server。

退出码: 0 交还成功（或本来就已交还）；3 没找到可交还的租约；1 出错；2 用法错误。

flags:
  -lease <id>          要交还的 lease_id（council claim 的输出里有）
  -resource <dir|port> 按资源交还自己在这个资源上的全部租约
  -owner <name>        持有者名字，默认 cli@主机名
  -server <url>        走已经持有 council.lock 的看板，例如 http://127.0.0.1:8765
  -json                按 JSON 输出（released / resource / notice）
  -config <path>       配置文件路径，与全局 -config 含义相同（与 -server 互斥）
  -lang zh|en|auto     输出语言，默认 auto
`,

	// 两个子命令共用的两条环境失败文案。
	"cli.lease.configBroken":           "%s: 配置文件有问题: %v\n",
	"cli.lease.openFailed":             "%s: 打不开本地库（数据目录 %s）: %v\n",
	"cli.lease.serverFailed":           "%s: 连不上 %s：%v（地址就是看板启动时打印的那一行）\n",
	"cli.lease.serverBadReply":         "%s: 看板返回了看不懂的内容: %s\n",
	"cli.lease.error.serverWithConfig": "-server 与 -config 不能同时给：走看板时配置由那个进程负责\n",
	"cli.lease.locked": `%s: 数据目录正被另一个 council 进程占用（council.lock）。
租约与主库在同一个 council.db 里，写它必须先拿到那把锁；本命令不会绕过锁另开一个库。
  数据目录  %s
先停下 serve -rw（含桌面壳自己拉起的那个）、TUI 或 council mcp，再重跑本命令。
`,

	// ---- 命令细则输出（中文值 = 调用处的 fallback 原文） ----

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
  council_who          list local agent processes read-only (requires local_agent = "observe" or "coordinate"; costs nothing)
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

	// ---- council init (first run / desktop setup wizard) ----
	"cli.init.flag.config":           "Config file path (default ~/.council/config.toml); same meaning as the global -config",
	"cli.init.flag.lang":             "Output language: zh / en / auto (auto by default)",
	"cli.init.flag.json":             "Print the detection result as JSON (for the desktop wizard and scripts); writes no file",
	"cli.init.flag.apply":            "Read JSON from a file and write the config (use - for stdin); refuses when the config already exists",
	"cli.init.error.positional":      "council init takes no positional arguments; extra: %v\n",
	"cli.init.error.layers":          "cannot resolve the config file location: %v\n",
	"cli.init.error.write":           "cannot write the output: %v\n",
	"cli.init.error.exists":          "the config file already exists, refusing to overwrite: %s (edit it in the settings page of council serve -rw, or by hand)",
	"cli.init.error.read":            "cannot read the input: %v",
	"cli.init.error.json":            "the input is not valid JSON: %v",
	"cli.init.error.unknownProvider": "that provider was not detected (or is not installed): %s",
	"cli.init.error.duplicateAgent":  "duplicate agent id: %s",
	"cli.init.error.noProvider":      "pick at least one provider",
	"cli.init.error.invalid":         "the generated config did not validate: %v",
	"cli.init.error.writeFile":       "cannot write the config file %s: %v",
	"cli.init.head":                  "Local CLI detection (writes no file):\n",
	"cli.init.notFound":              "not found",
	"cli.init.probeFailed":           "detection failed: %v",
	"cli.init.modelCount":            "%d models detected",
	"cli.init.configPath":            "config file: %s (%s)",
	"cli.init.configExists":          "already exists, will not be overwritten",
	"cli.init.noneFound":             "Nothing found. Install at least one (claude / codex / grok / agy), log in from a terminal, then come back.\n",
	"cli.init.written":               "wrote the config file: %s\n  providers: %s\n  agents: %s\nNext: council serve -rw for the dashboard, or just council for the terminal UI.\n",
	"cli.init.usage": `Usage: council init [flags]

Detect which local CLIs are installed (claude / codex / grok / agy), whether they can list models,
and their login state. By default it only prints; it writes no file. The desktop first-run wizard
uses --json to get exactly this result.

  council init --json              machine-readable result (writes no file)
  council init --apply <file|->    write the config from JSON; refuses when the config already exists

Exit codes: 0 ok; 1 environment/write failure; 2 usage error.

flags:
  -json                print the detection result as JSON
  -apply <file|->      read the selection and write the config; - means stdin
  -config <path>       config file path, default ~/.council/config.toml
  -lang zh|en|auto     output language, auto by default
`,

	// ---- council who ----
	"cli.who.flag.json":           "Print JSON (local_agent / managed / observed / notice)",
	"cli.who.flag.config":         "Config file path; takes precedence over COUNCIL_CONFIG and the default ~/.council/config.toml",
	"cli.who.flag.lang":           "Output language: zh / en / auto (auto, the default, checks COUNCIL_LANG, then language in the config, then the system locale)",
	"cli.who.error.positional":    "council who does not accept positional arguments; got: %v\n",
	"cli.who.configBroken":        "council who: the config file has problems: %v\n",
	"cli.who.off":                 `local agent is off (set local_agent = "observe" to observe, or "coordinate" to also allow resource leases)`,
	"cli.who.headManaged":         "Members managed by Council (%d)",
	"cli.who.managedRow":          "  conversation %s / member %s / turn %s / phase %s / started %s",
	"cli.who.managedEmpty":        "  (no in-flight turns)",
	"cli.who.managedUnavailable":  "  (database unavailable: %v; showing only the local-process section)",
	"cli.who.headObserved":        "Other local agent processes (%d)",
	"cli.who.observedRow":         "  %s  pid=%d ppid=%d exe=%s source=%s certainty=%s started %s%s",
	"cli.who.observedEmpty":       "  (no local agent processes found)",
	"cli.who.observedUnavailable": "  (process enumeration unavailable: %v)",
	"cli.who.unknown":             "unknown",
	"cli.who.headLeases":          "Local leases (%d)",
	"cli.who.leaseRow":            "  resource %s / holder %s/%s / mode %s / generation %d / expires %s / lease %s",
	"cli.who.leasesEmpty":         "  (no valid leases)",
	"cli.who.leasesUnavailable":   "  (leases could not be read: %v)",
	"cli.who.note":                "This command looks at local processes, not conversation members; it reads no command lines, environment variables or other CLIs' config files; the three sections are not joined by PID (.cmd shims report cmd.exe's PID); a lease is a reminder, not a file lock.",
	"cli.who.noteServer":          "This snapshot comes from the dashboard process: it matched the child process each turn started against the process table by PID (including .cmd shims); it reads no command lines, environment variables or other CLIs' config files; a lease is a reminder, not a file lock.",
	"cli.who.servedFrom":          "(the facts below come from the dashboard at %s: it is the only one that has the PIDs and the merge result)",
	"cli.who.observedMerged":      " merged=%s (turn %s)",
	"cli.who.flag.server":         "Ask the running dashboard for a snapshot (e.g. http://127.0.0.1:8765); only it has the PIDs and the merge result",
	"cli.who.serverFailed":        "council who: cannot get a snapshot from %s: %v (the address is the line the dashboard prints at startup)\n",
	"cli.who.usage": `Usage: council who [flags]

Print three read-only local sections without taking council.lock, so it works while the TUI or serve -rw is running:
  1. Members managed by Council: in-flight turns from the database
  2. Other local agent processes: claude / codex / grok / agy (requires local_agent = "observe" or "coordinate" in the config)
  3. Local leases: still-valid council claim records (leases need schema v10; on an older database this section only says it could not be read)
This command looks at local processes, not conversation members (that is /who inside council).
certainty=exact is only for processes under a known CLI install directory; missing or unknown paths are probable,
and same-named claude.exe of desktop clients (Claude Desktop etc.) is not reported.
It reads no command lines, environment variables or other CLIs' config files; with local_agent = "off" (the default) it enumerates nothing.
While the dashboard (serve -rw) holds council.lock, its snapshot knows two things reading the database from
outside cannot: child PIDs, and which process belongs to which turn. Add -server to ask it (read-only GET).

flags:
  -server <url>        ask the running dashboard for a snapshot, e.g. http://127.0.0.1:8765 (adds PIDs and the merge result)
  -json                print JSON (local_agent / managed / observed / leases / notice)
  -config <path>       config file path, same meaning as the global -config
  -lang zh|en|auto     output language, default auto
`,
	// ---- council claim / release (L2 resource leases) ----
	"cli.claim.flag.config": "Config file path; takes precedence over COUNCIL_CONFIG and the default ~/.council/config.toml",
	"cli.claim.flag.lang":   "Output language: zh / en / auto (auto, the default, checks COUNCIL_LANG, then language in the config, then the system locale)",
	"cli.claim.flag.json":   "Print JSON (granted / resource / mode / lease / holder / notice)",
	"cli.claim.flag.mode":   "Lease mode: write (exclusive, the default) or read (shared)",
	"cli.claim.flag.ttl":    "Lease lifetime, e.g. 5m / 1h (default 2m, minimum 5s, maximum 24h)",
	"cli.claim.flag.owner":  "Owner name (default cli@hostname); used for display and for matching on release, never for authorization",
	"cli.claim.flag.server": "Post to the dashboard that already holds council.lock, e.g. http://127.0.0.1:8765; omit to open the database yourself",
	"cli.claim.busyServer":  "The resource is taken; no lease was granted:\n  %s\n",

	"cli.claim.error.positional": "council claim needs exactly one resource argument (a directory path, or tcp:<host>:<port>); got: %v\n",
	"cli.claim.notEnabled":       `Local agent coordination is off (set local_agent = "coordinate" in the config to enable it): this command writes no lease.`,
	"cli.claim.failed":           "council claim: %v\n",
	"cli.claim.granted": `Lease granted
  resource   %s
  mode       %s
  owner      %s/%s
  lease      %s
  generation %d
  expires    %s (in %s)
Release it with: council release %s. A lease only touches the database: no process scan, no model call.
`,
	"cli.claim.busy": `The resource is taken; no lease was granted
  resource   %s
  holder     %s/%s (%s lease)
  lease      %s
  expires    %s
Wait for it to expire, or ask the holder to run council release %s and try again. Leases are voluntary reminders; they never stop a process from writing files.
`,
	"cli.claim.usage": `Usage: council claim <dir|tcp:port> [flags]

Take a resource lease in council.db (schema v10). A lease is a voluntary reminder,
not a file lock: it never stops a process from writing, it only tells the next agent
that someone holds this resource. This command makes no model call and scans no
process; it only reads and writes two tables.

Enabling it: local_agent = "coordinate" in the config (the 6th settings page works too).
Writing needs council.lock, so it refuses clearly while serve -rw / the TUI / council mcp is running;
in that case add -server to go through that dashboard, which already holds the lock.

Exit codes: 0 lease granted; 3 denied (taken / level off / lease not found); 1 error; 2 usage error.

flags:
  -mode read|write     lease mode, write (exclusive) by default; read can be shared
  -ttl <duration>      lifetime, e.g. 5m / 1h (default 2m, minimum 5s, maximum 24h)
  -owner <name>        owner name, default cli@hostname (display and release matching only, never authorization)
  -server <url>        go through the dashboard that already holds council.lock, e.g. http://127.0.0.1:8765
  -json                print JSON (granted / resource / mode / lease / holder / notice)
  -config <path>       config file path, same meaning as the global -config (mutually exclusive with -server)
  -lang zh|en|auto     output language, default auto
`,

	"cli.release.flag.config":   "Config file path; takes precedence over COUNCIL_CONFIG and the default ~/.council/config.toml",
	"cli.release.flag.lang":     "Output language: zh / en / auto (auto, the default, checks COUNCIL_LANG, then language in the config, then the system locale)",
	"cli.release.flag.json":     "Print JSON (released / resource / notice)",
	"cli.release.flag.lease":    "The lease_id to release (council claim prints it)",
	"cli.release.flag.resource": "Release all of your own leases on this resource (matched by -owner, default cli@hostname)",
	"cli.release.flag.owner":    "Owner name, the same one used with claim (default cli@hostname); only leases with exactly this name are released",
	"cli.release.flag.server":   "Post to the dashboard that already holds council.lock, e.g. http://127.0.0.1:8765; omit to open the database yourself",
	"cli.release.nothingServer": "The dashboard had no lease to release (already released, or the owner does not match).",

	"cli.release.error.positional": "council release does not accept positional arguments; use -lease or -resource; got: %v\n",
	"cli.release.error.target":     "council release needs a target: -lease <lease_id> or -resource <dir|tcp:port>\n",
	"cli.release.error.both":       "council release accepts a single target: -lease and -resource cannot be combined\n",
	"cli.release.failed":           "council release: %v\n",
	"cli.release.notFound":         "No such lease: %s (it may have been deleted, or the lease_id is mistyped)",
	"cli.release.already":          "Lease %s is already %s; nothing to release (file untouched, no event written).",
	"cli.release.nothing":          "No lease of %s to release on this resource (only leases with an exactly matching owner_id are released; there is no force).",
	"cli.release.released":         "Released %d lease(s)\n",
	"cli.release.usage": `Usage: council release -lease <lease_id>
       council release -resource <dir|tcp:port> [-owner <name>]

Return a lease you took (state becomes released and an event is recorded). Releasing
does **not** require local_agent = coordinate: turning the setting down must not
strand an authorization you already handed out.

Only leases with an exactly matching owner_id are released; there is no force
parameter, because releasing someone else's lease is outside this module. Writing
also needs council.lock; add -server while the dashboard holds it.

Exit codes: 0 released (or already released); 3 nothing matchable to release; 1 error; 2 usage error.

flags:
  -lease <id>          the lease_id to release (council claim prints it)
  -resource <dir|port> release all of your own leases on this resource
  -owner <name>        owner name, default cli@hostname
  -server <url>        go through the dashboard that already holds council.lock, e.g. http://127.0.0.1:8765
  -json                print JSON (released / resource / notice)
  -config <path>       config file path, same meaning as the global -config (mutually exclusive with -server)
  -lang zh|en|auto     output language, default auto
`,

	// Shared by both subcommands when the environment is the problem.
	"cli.lease.configBroken":           "%s: the config file has problems: %v\n",
	"cli.lease.openFailed":             "%s: cannot open the local database (data dir %s): %v\n",
	"cli.lease.serverFailed":           "%s: cannot reach %s: %v (the address is the line the dashboard prints at startup)\n",
	"cli.lease.serverBadReply":         "%s: the dashboard returned something unreadable: %s\n",
	"cli.lease.error.serverWithConfig": "-server and -config cannot be combined: when you go through the dashboard, that process owns the config\n",
	"cli.lease.locked": `%s: the data dir is held by another council process (council.lock).
Leases live in the same council.db, so writing them needs that lock; this command never opens a second database to bypass it.
  data dir  %s
Stop serve -rw (including the one the desktop shell starts), the TUI or council mcp, then run the command again.
`,
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
