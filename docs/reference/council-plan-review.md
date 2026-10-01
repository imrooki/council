# Council 10 步计划评估（2026-09-20）

依据：many-ai-cli 源码（8 子系统审计 + 4 项定向探针，21 个 agent）、四家 CLI 的 --help 本机实测、本机环境实测。

## 0. 结论先行

计划的**产品判断**基本正确，**技术路线选错了层**。

Council 要的每一样能力，四家 CLI 都已在 headless/JSON 接口上原生提供。
many-ai-cli 是这些 CLI 的**交互式 TUI 刮屏层**，它把本已结构化的信息拆成像素再猜回来。
在它上面建 Council，等于主动选择用启发式去还原协议原语。

另有一条硬事实：**many-ai-cli 里已经有一个硬编码的 Council**（relay.go 5475 行），
计划 Step 5-9 描述的大半功能已经存在。计划读起来像不知道它存在。

## 1. 四家 CLI 的程序化接口（本机 --help 实测，四列都跑过）

| 能力 | claude | codex exec | grok | agy |
|---|---|---|---|---|
| 会话恢复 | --resume <id> / -c | resume <id> / --last | --resume <id/title> / -c / -s <uuid> | --conversation <id> / -c |
| 结构化流式输出 | --output-format stream-json | --json（JSONL 事件） | streaming-json / streaming-messages-json | --output-format stream-json |
| NDJSON 输入 | --input-format stream-json | stdin（单轮，`-` 或管道） | --prompt-json | --input-format stream-json |
| 结构化输出 schema | - | --output-schema <FILE> | --json-schema | --json-schema |
| 取最终回复 | - | -o --output-last-message <FILE> | - | - |
| 身份注入 | --append-system-prompt[-file] | 走 AGENTS.md | --system-prompt-override / --rules | --agent |
| effort | --effort <level> | -c model_reasoning_effort=… | --reasoning-effort | --effort low/medium/high |
| 权限档 | 有 | -s read-only/workspace-write/danger-full-access | --permission-mode（6 档） | --mode / --sandbox |
| 能力发现 | - | - | grok models | agy models / agy agents |
| 其他 | --fork-session, --bg | -C --cd, --add-dir, -p profile, --ephemeral | --max-turns, --agents JSON, --worktree, export, sessions | --add-dir, --new-project |

四家都成立，但不齐平：**claude / grok / agy 原生完整，codex 经 exec + AGENTS.md 达成**
（codex 没有 system-prompt flag，NDJSON 输入是单轮加 resume，不是同进程内交错多轮）。
agy 的 --input-format stream-json 按 help 描述是"每行跑一轮"，够做逐轮往返，
但**未实测**是否支持同一进程内真正的交互式 stdin。

即便如此，Council 计划 Step 3/6/7/9 要自建的 Provider Capability、Persistent Session、
Effort 选择、Max Turns、Worktree 隔离、结构化消息，在这一层基本都是现成的命令行 flag。

## 2. many-ai-cli 已有的 Council（探针实测）

- relay.go 5475 行：plan -> implement -> review -> fix 状态机，硬编码 3 角色
- internal/orchestrate/：conductor AI 调用的 CLI 子命令
- POST /api/sessions/:id/spawn-child
- board.md + child-<id>.md：文件式共享上下文，2 秒轮询
- handoff：跨 provider 续接
- agentChatMessage：已经是 provider-neutral 的消息模型

**作者自己的结论（relay.go:6-10）**：整个循环只靠两个信号推进——
子进程 progress 文件里 `## DONE <role>` 的**行数增长**，和 reviewer 的 `verdict:` 行。
不是 idle，不是 transcript，不是 VT。

这是作者建完同一个东西之后得出的教训：**确认必须在协议层，不能在 PTY 层。**
Council 计划 Step 7 设计的 delivery ack + cursor advancement 正是想在 PTY 层做这件事。

## 3. 三条硬阻塞（都经源码确证）

### 3.1 turn 完成事件：程序注入驱动的对话轮次不产生任何事件

- 全 Hub 只有一个状态机，provider 无关，唯一信号是 **PTY 静默 3 秒**（idle_state.go, server.go:47）
- 没有 thinking 状态；waiting 状态**在无浏览器时不可达**（approvalVisible 只由 UI 的 session_hint 写入，
  Go 侧 VT 检测器从不写它）——headless 消费者看到卡在审批上的 agent 只显示 running/standby
- 所有"一轮完成"路径都挂在一个 git baseline 上，而 captureGitTurnStart 只有一个调用者：
  来自 UI WebSocket 的真实按键（input_gate.go:126）。Hub 注入的文本走 injectText -> injectRaw -> submitInput，
  完全绕过它。因此 maybeCreateFallbackDoneSummary 和 handleCodexTaskCompletion **对被注入驱动的会话根本不触发**
- 且两者都要求 turn.Files > 0，所以**纯讨论轮次（Council 的主场景）即便走 UI 路径也什么都不产生**

结论：在 many-ai-cli 上，一个被程序驱动的 agent 说完一段话而没改文件，系统没有任何事件告诉你它说完了。
这正是 relay 退回到"让 AI 自己写 DONE 标记"的原因。

### 3.2 注入层给不了可靠投递

- injectText 返回 void；下层算出的 delivered bool 被 injectRaw 丢弃
- **没有 mid-turn 闸门**。CLI 正在生成时，粘贴体立即落进 composer，然后最多等 30 秒强制发 CR
- 确认是启发式："1500ms 内有新 PTY 输出"算确认。生成中的 CLI 一直在输出，**中途注入的确认必然假阳性**
- composer-ready 检测只覆盖 codex 和 claude；grok / copilot / cursor-agent / opencode / command-code / custom 什么都没有
- handleSessionInject 未做控制字节消毒：正文里的 `\x1b[201~` 能提前终止粘贴信封，其后内容变成实时按键
- 只有 relay 的**第一条** prompt 走验证投递路径（回显校验 + 重试）；后续 relayInject 是裸 injectText，无等待无校验无重试

### 3.3 编排 API 拒绝 custom provider —— agy 今天无法被程序化 spawn

- orchestrationProviders / validOrchestrationProvider（orchestration.go:3668）只认 7 个内置 id
- 所有多 agent 入口都过这道闸：handleSpawnChild、角色分配、relay 自动 spawn、spawn spec 校验
- 不一致之处：**人类可以从下拉框 spawn agy，编排 API 不行**
- 修复约 5 行（照 validSpawnProvider 的写法查 EffectiveCustomProviders），但这是 Council 第一件必须改的事

## 4. 其余硬冲突

- **CustomProvider 只有 4 个字段**：ID / Label / Command / ApprovalPatternSource。无 model、无 effort、无 capability。
  注释明说 custom provider 不接 built-in approval-hook wiring
- **isAIProvider 对 custom 返回 false** —— 这是 AI 功能总闸，custom provider 拿不到审批规则注入、
  chat 提取、handoff、usage 追踪、slash 命令
- **结构化 chat 提取只有 claude 和 codex**（agent_chat_handler.go:24）。
  grok 走另一套更弱的 handler，靠 10 分钟时间窗**近似匹配**会话目录。
  copilot / cursor-agent / opencode / command-code / custom 没有
- **effort 对所有 provider 都不可配**（wrapper flagset、spawn body 都没有该字段），
  尽管四家 CLI 本身都有 effort 入口
- **resolveSpawnModel 对 custom provider 硬置零 model**
- provider 字面量散在 **56 个非测试 Go 文件 + 20 个 TS 模块**，约 46 个 Go 决策点 + 20 个 web 决策点。
  计划规则 16 说"不为 Council 顺手大规模重构 many-ai-cli"，规则 7-9 要求通用 Provider Registry。
  这两条**互相冲突**，必须先裁决一条

## 5. 环境阻塞

- **本机没装 Go**（全盘搜过，PATH 与常见安装位置均无 go.exe）。repo 要求 go 1.25.13。
  Step 1 第 5 项"Build the unmodified project"直接做不了
- bun / node / git 都在
- fork imrooki/many-ai-cli 的 pushed_at 与 upstream 完全一致，是刚 fork、尚无自有提交的状态
- conda 在 D:\Install_path\Anaconda3，不在任何 PATH 上

## 6. 计划做对的地方

- **Provider != Agent** 正确，而且现有架构支持：同一 cwd 可跑多个 claude 会话，transcript 按 session id 区分
- **capability 要有显式 unsupported 状态、不要假装支持** 正确。agy 的 effort 烘焙在 model label 里
  （"Gemini 3.8 Flash (High)"）同时又另有 --effort flag，两者如何交互未测——正是这条规则的活例子
- **provider 不能改 live session 的 model/effort 时要重建会话** 正确。四家都有 resume，重建代价可控
- **max turns / timeout / cancellation / loop protection** 正确，relay 已有对应保险（maxRounds 默认 3、escalateAfter）
- **Canonical Conversation 不每轮全量重发** 正确，这是对的核心洞察
- **Judge 是 role=judge 的普通 Agent** 正确
- **保守默认权限** 正确，而且必要：relay 的子会话默认全权限绕过
  （codex 用 --sandbox danger-full-access --ask-for-approval never）

## 7. 计划缺失的

1. **额度与成本模型**。N 个持久会话各自累积路由进来的 delta，context 单调增长。
   5 个 agent 跑一轮讨论的 token 量没有估算。Council 的价值主张直接取决于这个数
2. **ToS 暴露**。无人值守的 agent-to-agent 循环跑在订阅制 CLI 上。
   upstream README 自己标了这条风险，并记了 Google 2026-02 因第三方工具借 Gemini CLI OAuth 而封号的先例
   （Antigravity 作为 Google 自家产品被连带波及）。agy 走 ~/.gemini 目录，正落在这个风险区
3. **council 与 Hub 的关系没定义**：council 是嵌一个 Hub，还是连已在跑的 Hub，还是完全不要 Hub
4. **per-agent 身份怎么进到已有自己 system prompt 的 CLI**。
   many-ai-cli 的 --append-system-prompt-file / --rules 路径今天只带固定的委派帮助文本，不带 role；
   relay 把 role 打进 PTY 的第一条消息。程序化接口这层才有干净答案
5. **哪个模型执行这些步骤**没说。若主代理是 Fable 系列，全局规则的代码委派链对每个实现步骤都适用
6. **agy 的 headless 权限行为**。agy 有 --mode（accept-edits/plan）和 --sandbox 权限档，
   但 ask-agy 的记录是 headless 下会卡在 y/N 提示，目前靠 --dangerously-skip-permissions 绕过。
   Council 若照搬这个绕法，权限系统对 agy 成员就形同虚设；正确做法是先测 --mode/--sandbox 在 headless 下是否够用

## 8. 反论：headless 路线的两个真实代价

**这份报告的主张要自己承受两条反驳。**

1. **审批谁来答。** many-ai-cli 存在的全部理由就是"有人来答审批"。
   Council 若搬到 headless 层，要么每个 worker 都跑在 bypass 模式
   （正是 relay 的默认，本报告 §6 刚说它不安全），要么得自建审批 UI——
   而那恰恰是 many-ai-cli 已经做好、Council 会重建的那块。
   所以 §9 的结论不是"显然该走 headless"，而是**headless 做 runtime，审批面借 many-ai-cli**
2. **"已经站在这一层"是打折的。** 你的 ask-codex / ask-grok / ask-agy 子代理确实在用程序化接口，
   但编排是 Claude 在做——路由、上下文裁剪、结论综合都发生在我的 context 里。
   那正是 Council 想要替换掉的东西。CLI 调用形态是现成的，**编排层不是**，别高估这个起点

## 9. 建议

**先答一个问题：Council 的价值在 runtime 还是在 UI？**

- 若在 **runtime**（多 agent 协作本身）：建在四家 CLI 的 headless/JSON 接口上。
  turn 边界、投递、身份、effort、能力发现全是协议原语，不需要 VT 模拟器、不需要 idle 启发式。
  但要正面解决 §8.1 的审批问题——这是这条路线唯一真正难的部分
- 若在 **UI**（要在手机上看/干预多 agent 讨论）：many-ai-cli 的 Hub 有独立价值。
  但注意"把 council 注册成 custom provider"只能拿到**一个终端面板**：
  isAIProvider 对 custom 返回 false，审批规则注入、chat 提取、handoff、usage 追踪全都拿不到
- 若坚持在 many-ai-cli 内部做：正确形态是**泛化 relay**，而不是在它旁边平行新建。
  第一步改 validOrchestrationProvider（~5 行），第二步把 relay 的硬编码 3 角色变成配置

**无论选哪条，Step 1 之前先装 Go。**

## 10. 这些提示词能不能直接贴

- **Step 1**：装完 Go 后可原样贴。它本来就要求区分"真实上游失败"和"环境失败"，没装 Go 会被正确报成环境失败
- **Step 2**：可贴，但**必须加一句**：
  "in particular internal/hub/relay*.go (5475 LOC), internal/hub/orchestration.go, and internal/orchestrate/ —
  characterize the existing multi-agent engine before proposing a new one"。
  不加这句，执行者很可能漏掉全仓最相关的部分
- **Step 3**：**不要在 Step 2 交出 relay 的完整画像之前跑**。
  否则会在不知道已有编排引擎的前提下做架构设计，这是整个计划最大的结构性盲点
- **Step 4 及之后**：等前三步的结论出来再定，因为 §9 的分层裁决会改写它们的前提
