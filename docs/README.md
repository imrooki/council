# Council 设计文档

本地优先、CLI 优先的多模型协作运行时。
Agent = Provider + Model + Effort + Role + Permissions + RuntimeMode，由用户自由组合。

## 先读这个

| 文档 | 内容 |
|---|---|
| [00_DECISIONS.md](00_DECISIONS.md) | **入口**。十条决策（D-1 到 D-10，其中 D-10 已被实测推翻并订正）、明确不做的事、**42 条**未决问题（现查 2026-09-25：20 条已加删除线关闭，剩下的多数是**已裁但有意保留**的记录，不是待办；#31 排在 #24 与 #25 之间）。关闭理由与实测出处都留在原条目里。**这个数字会过期，改条目时顺手改这里**。接手前先读，不要重新推导 |
| [02_AGENT_PROVIDER_ARCHITECTURE.md](02_AGENT_PROVIDER_ARCHITECTURE.md) | Provider 是 flag 适配器；四家 CLI 的能力契约表；Agent 定义；AgentSession 为什么不是进程 |
| [04_CONTEXT_ROUTER.md](04_CONTEXT_ROUTER.md) | 路由规则；**缓存前缀稳定性是第一约束**；`thought` 字段的处置；从 relay 抄的四个模式 |
| [05_MESSAGE_PROTOCOL.md](05_MESSAGE_PROTOCOL.md) | Canonical Conversation schema；三家输出的归一化；用量记账；SQLite 表 |
| [06_SECURITY_MODEL.md](06_SECURITY_MODEL.md) | 权限档到各 CLI 的翻译（**不是**直接映射，那条前提已被实测推翻）；每家的写入边界与指令环境隔离都按实测记；审批缺口只有一个角色；ToS |

未写：`01_PRODUCT_VISION`（原始计划本身就是愿景）、`03_GROUP_CHAT_ARCHITECTURE`（并入 04 与 05）、
`07_MVP_ROADMAP`（见下表）。

`reference/` 放证据来源，不是设计文档，但上面每份设计的依据都在里面：
many-ai-cli 审计报告、原 10 步计划的评估、Step 0 探针结果、Step 1 基线验证。

接手请先读 [`../.handoff/HANDOFF.md`](../.handoff/HANDOFF.md) 的最新一条。

## 实证基础

本设计不是纸面推演，依据两批实测：

- **many-ai-cli 源码审计**（8 子系统 + 4 项定向探针，21 个 agent）——
  确定了「不在 PTY 层建运行时」这个决定，以及该从 `relay.go` 抄哪四样
- **Step 0 探针**（2026-09-20，E 盘机）—— claude / grok / agy 三家 headless 会话恢复与成本实测。
  原始数据：`E:\Isolating_test\council\step0-probe\STEP0-RESULTS.md`

关键数字（claude 侧，其余两家订阅制只有 token）：
冷启动 47,749 token / $0.955；暖缓存零新上下文边际 $0.014；缓存 TTL 1 小时。
每 agent 固定入场费 18k–48k token。

**这三行是 Step 0 探针在当时的 flag 组合下量的，不是一个可以直接引用的常数。**
换一组 flag 就是另一个数：`11_STEP8_SPEC.md` §1.2.1 记了一条真实轮次，
加了 `--safe-mode --restricted` 之后 claude 侧是 `in=29537 cache_create=29535 $0.118394`
（夹具很小，与这里的 47,749 量的不是同一件事）。
**本仓库里没有一个可引用的「入场费常数」** —— 要报价一律按
`12_STEP9_CONTRACT.md` 第 4.5 节，从这台机器上这个身份自己跑过的
`turn_usage` 行取样。Step 9 的设计合议已经照抄这里的数字错过一次。

## 路线图（原 10 步计划的修订版）

| 步骤 | 状态 |
|---|---|
| Step 0 成本与往返探针 | **已完成** |
| Step 1 基线 | **已完成**（many-ai-cli 作为**参考材料**的基线，非开发基线。`E:\Isolating_test\many-ai-cli\baseline\BASELINE.md`） |
| Step 2 从 relay/orchestrate 提取该抄什么 | **大半完成**，结论在 `04` §5 |
| Step 3 架构设计 | **本目录** |
| Step 4 council CLI 与配置系统 | **已完成**（`4034abf` + `f5a4a9a`，规格见 `07` / `07a`） |
| Step 5 Canonical Conversation 与动态 participant | **已完成**（`739348f` 基线 + `79e1579` + `486f5f3`，规格见 `08` / `08a`） |
| Step 6 Provider 适配器与持久会话 | **主体已完成**（2026-09-21）。三家 CLI 全部真机跑通：`/ask` 起进程、解析、落库、推游标，`/session` 报冷暖。`internal/provider` 与 `internal/turn` 两个包 + REPL 两条命令 + app 接线，规格见 `09` / `09a`。第三种结局（provider 吃下了、本地记不上）在真机上验过。**三家都是可用成员**（agy 的空正文问题已定位并修复，见未决 #17） |
| Step 7 Context Router 与增量上下文 | **主体已完成**（2026-09-21）。schema v2（`turn_usage` 换 `turn_id` 主键、用量列可空、游标搬进 `provider_sessions`）、`Service.RecordTurn` 三表一事务、Runner 从库读回、`Router` 与 bootstrap 包。规格见 `10` / `10a`。**真机验过重启**：第二个进程 `/ask` 直接用回库里那条 provider 会话，游标与冷暖都读回了。**剩余**：Router 的规则仍然简单（真正的收益要等 Step 8 的 @mention）、`/session` 还没显示 seq 区间、prompt 传输 grok 与 agy 仍在 argv（未决 #15） |
| Step 8 动态 agent、@mention、任务执行、权限 | **第一片、第二片已完成**（2026-09-21）。**第一片**（Council 自己的全局 / 局部规则 md 文件）规格见 `11` 第 1 节、冻结契约 `11a`，四轮真机验收。**第二片**（WORKSPACE 档按实测的写入边界放行）契约 `11c`，`445b809` + `7ab093e`，真机验收 4/4（agy 那条靠解析 `denied_actions` 才过，见 `11c` 第 7 节）。**第三片**（定向语义、有界扇出、`/config reload`、任务执行）冻结契约见 **`11d`**：定向语义、有界扇出、`/config reload` 已在后续各步落地；`/task` 任务执行（8c-4，#24）2026-09-29 Griffin 定**暂缓**。`06` 第 2 节那张权限表已按实测重写两次（`7c69602`、`10332ec`） |
| Step 9 Team、Scheduler、Judge | **已完成**（2026-09-22）。契约 **`12_STEP9_CONTRACT.md`**。八刀全部落地：片 1 按 `sessionKey` 拆锁 + `AskUpTo`（`31e3ea7`）/ 片 2a schema v4 + team 绑定（`063d60f`）/ 片 2b thought 可见性（`761f21a`）/ 片 2c `/team use`（`36456b4`）/ 片 3a `PlanRound` 纯函数（`85137c6`）/ 片 3b `RunRound` 真并发（`dba2e0f`）/ 片 3c Budget 预报与 `UsageSamples`（`f589daf`）/ 片 3d `RunRoundPrepared`、`--rounds`、`--with-judge`（`40d350f`、`0a084ca`、`9b7824d`、`e2218d3`）。**Judge 是真的会跑的**（`repl/round.go:217-236`：发言轮跑完后另取快照、`Speakers: plan.Judges` 另起一轮），不是只分类 |
| Step 10 崩溃恢复、粘贴、测试缺口、MVP | **一多半已落地**，契约 **`13_STEP10_CONTRACT.md`**。**D-11「不做 TUI」已被推翻**（2026-09-22 用户裁定），TUI 在做，规格映射见 **`15_TUI_SPEC_MAPPING.md`**。<br>**A 崩溃恢复**：之一 schema v5（`204e615`）、之二 `expectInflight`（`90bad1c`）、之三 三条 phase 与 `began` 穿线（`92db175`）、之四 启动对账与横幅（`9e85bea`）已落；之五（§3.8 闸 + `/session` 的 BASELINE 列，`8c00ec9`）已落，剩余项见 `00_DECISIONS.md` #43。<br>**B 粘贴**：`03dcef4` + `10124a9` 已落；**Windows Terminal 下的人工探针仍 [未验证]**（实测跑过的是 conhost/PowerShell）。<br>**C 测试缺口**：C1 端到端烟测、C3 保留 sender 负向用例已落（`1a5d46e`）；**C2（崩溃对账端到端）已落**（2026-09-27，§6.2 的两处措辞按实测订正）；**C4（`-race`）本机跑不了** —— `-race requires cgo`，gcc 不在本机，三次实测；**2026-09-25 第一次在云端 Linux 容器里跑了**：生产代码没报出竞态，读法与剩下的上游竞态见 `00_DECISIONS.md` #42。<br>**D**：之一 `/cost`（`e18f02b`）已落；之二（`/log` 标记，`a2e3d76`）已落；**之三（`14_MVP_CHECKLIST.md`）草案已写（2026-09-30），待 Griffin 圈定 MVP 范围并照着实操**。<br>**UX-1**（一轮进行中谁回来先报谁，`b34bbe4`）不在原契约里，是用真机撞出来的需求 |
| 新功能（2026-09-27 立项，#44 / #45 / #46） | **已落地，真机未复验**（2026-09-28）：简洁输出 + 每个模型的用时（CO1，`1c62dbb`，契约 **`16b`**）；模型识别 + `/add +编号` 追加写配置（MD1，`e9afff9`，**`16a`**）；多成员 `/round` 后自动总结、只给用户看（SM1，`a877d44`，**`16c`**）；TUI 整块粘贴多行命令的防护（X44，`e85d1bc`，**`17a`**）；codex provider 适配器（CX1，`c0e6a27`，**`17b`**，MCP 风险 Griffin 已接受）；codex 模型接进 `/add` 识别 + `/help`（CXH，`db8c41a`，**`18a`**）；`/session rebuild` 结清陈旧在飞记录、被拒后不用重启（U43A，`17b3236`，**`18b`**，#43(a)）。裁决与落地记录在 `00_DECISIONS.md` #43、#44、#45、#46 |
| 对话模式与 @ 补全（2026-09-28 立项，#47） | **已落地，真机未复验**（2026-09-29）：输入框敲 `@` 弹出成员补全（AT1，`4202b6c`，契约 **`19b`**）与它的跟进（AT1-FU，`ee247c6`，**`19c`**）；直接打字就让成员回复（CH1，`4d3dc0b`，**`19a`**）。用法见下一节，落地记录在 `00_DECISIONS.md` #47 |
| 首次运行自动配置（2026-09-29 立项，#48） | **已落地，真机未复验**（2026-09-30）：第一次运行检测本机的 claude / grok / agy / codex、报登录状态、生成配置（OB1，`3317940`，契约 **`19d`**）；配了 API key 的（含经 CPA 的 grok）算已登录（OB2，`70025db`，**`19e`**）；`/add` 在识别结果之后也显示各家的登录状态（OB3，`ef76eef`，**`19f`**）。见「第一次运行」一节，落地记录在 `00_DECISIONS.md` #48 |
| `/add` 按名字加识别出的模型（2026-09-30 立项，#49） | **已落地，真机未复验**（2026-10-01）：识别出的订阅模型直接 `/add <名字>` 加入，不必先写进配置（AD1，`d4ad85f`，契约 **`19g`**）。见「加成员」一节，落地记录在 `00_DECISIONS.md` #49 |
| 一批小修补（2026-10-01，#50） | **已落地，真机未复验**：`/help` 补上按名字加；`/team use` 不再把总结成员当普通成员加入；结束卡住的进程只用系统目录里的 taskkill，不再按 PATH 找；几处测试与注释（CL1，`5c4840a`，契约 **`19h`**）。落地记录在 `00_DECISIONS.md` #50 |

## 第一次运行（#48，2026-09-30 起）

- 装好 council、在终端里登录好 claude / grok / agy / codex 里的至少一家之后，直接运行 `council`。默认位置 `~/.council/config.toml` 不存在时，council 会：
  - 找本机装了哪几家工具：先查 PATH，再查 `%APPDATA%\npm`、`%USERPROFILE%\.grok\bin`、`%LOCALAPPDATA%\agy\bin`；
  - 让每家列一次模型（不提问、不花钱），每家一行「识别到 N 个模型；<登录状态>」。登录状态是下面几种之一：
    - 已登录：claude 用订阅登录时带订阅类型，例如「已登录（订阅：Claude Max）」；
    - 已登录（API key）：配了 API key 的，包括经 CPA 的 grok；claude 还带 key 的来源，例如「已登录（API key：ANTHROPIC_API_KEY）」；
    - 没登录；
    - 登录状态查不出；
  - 生成配置：只写 `data_dir = ~/.council/data` 与找到的 `[providers.*]`，不写成员；然后照常启动。
- 「识别到模型」不等于登录了：没登录的 claude、grok、codex 也会列出一部分模型。没登录或查不出的那几家，council 会列出登录办法：在终端里分别运行 `claude auth login`、`grok login`、`codex login`，agy 运行后按提示登录。已登录与已登录（API key）的不列。
- 「已登录（API key）」说的是配了 key，不检查 key 能不能用：例如 CPA 所在的机器睡着了，这里照样显示已登录（API key），能不能用以第一次提问为准。
- 一家都没找到时不写文件，只告诉你去哪装。用 `--config` 或环境变量 `COUNCIL_CONFIG` 指定了路径却找不到时，照旧报错、不生成。
- 已有配置的：以后又装了新工具，`/add` 的识别表后面会提示「本机装了 X，配置里还没有」，并给出要追加的两行。追加之后敲 `/config reload --apply` 与 `/add --refresh`。council 不会自动改你的配置。
- 以后随时看登录状态（OB3 起）：不带参数的 `/add` 在识别结果之后、上面那句提示之前，多一段「各家的登录状态（本会话的快照，/add --refresh 重新检查）:」。
  - 配置里写了的每一家一行，说法与第一次运行相同。
  - 只有「没登录」的那一家在行末带「在终端里运行 <登录办法>，登录之后 /add --refresh」；查不出的不带（agy 永远查不出）。
  - 这一段和识别出的模型一起存在本会话的快照里，登录或登出之后敲 `/add --refresh` 才重新检查。
  - 每次重新检查只多跑一次 `codex login status`，不花钱。

## 加成员（#45、#49）

- 配置里已经写好的成员：`/add <全名|编号|前缀>`，多个用空格或逗号隔开，只加入会话，不改配置。
- 识别出来、配置里还没有的模型（各家订阅里的），有两种加法，效果相同：
  - 先敲不带参数的 `/add` 看识别表，再 `/add +编号`，要换强度加 `--effort <强度>`；
  - 直接敲名字（AD1 起）：
    - `/add gpt-6.1-sol-xhigh`：识别表 ID 列里的名字，最后一段是强度，换成别的档也行；
    - `/add gpt-6.1-sol xhigh`：模型名加强度，中间的空格可以是全角空格；
    - `/add gpt-6.1-sol --effort xhigh`。
    名字不区分大小写。本会话还没识别过时，会先自动识别一次（打「正在识别模型」，几秒到十几秒，不花钱）。
- 两种加法都是先备份 config.toml、再把这个成员追加进去、再加入当前会话。屏幕上会打出备份文件的路径。新成员的权限是只读、讨论。
- 只敲模型名、不写强度时，不会给会话里已有的模型再加一个重复的成员：
  - 配置里还没有这个模型：按默认档 high 新建；
  - 这个模型已经有成员在会话里：不加，告诉你是哪个；要另加一档强度，就把强度写上；
  - 配置里有这个模型、但成员不在会话里：只有一个就把它加回来，有几个就列出来让你挑。这几种都不写配置。
- 强度敲错（例如 `xigh`）时，会列出这个模型支持的档，不写配置。一个名字对上好几个模型时，列出候选，不猜。识别出的模型一次加一个。
- 撤回：`/leave <id>` 只让它退出这个会话，配置里那一段还在。要连配置一起撤回，`/quit` 之后用加入时打出来的备份文件覆盖回 config.toml。
- `/team use <team-id>` 把 team 里还没加入的成员一次性加进来，但总结成员（config.toml 的 `[summary]`）除外：它不随 team 加入，多个成员回复之后会自己加入做总结（CL1 起）。

## 对话模式怎么用（#47，2026-09-29 起默认打开）

- **直接打字**（不以 `/` 开头）：先记下，再自动让当前会话的成员回复。
  - 没点名就问全部活跃成员，总结成员除外；消息里 `@` 了谁就只问谁；多个成员回复之后自动总结，总结只给你看。
  - **代价**：每句话是 N 次调用（N = 这次被问的人数），N ≥ 2 时再加一次总结，没有预览。成员多时随手一句就按全员付费。
- **只想记下、不让任何人回复**：用 `/say 文本`，或 `/auto off` 把这个会话的自动回复关掉（`/auto on` 打开，`/auto` 切换）；启动时加 `-no-auto` 则整个进程默认只记下。
- **点名**：写半角 `@` 紧跟成员 id（`/who` 第一列），不带尖括号，不是显示名，`@` 前面空一格或在行首，例如 `@grok-4.7 你怎么看`。两个点名之间要空一格：`@a@b` 里第二个认不出。TUI 里敲 `@` 会列出成员的 id 与显示名，上下键选，Tab 或回车补全。
- **冲着成员却没写对的 @，这句谁都不问**，打一行说明，改好写法重发，或照说明接着敲 `/round`。例如：
  - `@` 后面隔着空格、tab 或标点跟着成员 id：`@ grok-4.7`、`@(grok-4.7)`、`@**grok-4.7**`，中文输入法下的 `@（grok-4.7）` 也一样；
  - 跟着成员的显示名，大小写不同也算：显示名是 `Opus` 时写 `@ opus`；
  - 全角 `＠`。
- **不冲着任何成员的 @ 当成没点名，问全部成员（花钱）**：
  - 贴进来的 numpy / torch `A @ B`、`$@`、PowerShell 的 `@{` 与 here-string 收尾、SQL 的 `@@IDENTITY`；
  - `价格 @ 5 元`、句末单独一个 `@`；
  - `@` 与 id 之间隔着换行；
  - `@所有人`：它只是不冲着任何成员，不是「问全体」的语义。
- **解析得出名字、但不是会话成员的**：`@all`、`@staticmethod`、PowerShell 的 `@params`、`@Override`，算「点的不是成员」，谁都不问、打两行说明，出路同样是 `/round`。
- **`@` 左边紧贴 ASCII 字母、数字或 `_ @ . -` 时不算点名，照问全员**：例如 `hi@grok-4.7`、`hi＠grok-4.7`。邮箱 `a@b.com` 也因此不算点名。

Step 4 起的所有步骤，删掉原计划里全部 PTY / Hub / many-ai-cli custom-provider 的耦合。

## 动手写代码之前

确认由谁执行。若主代理运行在 Claude Fable 系列上，全局规则的代码委派链对每个代码步骤生效
（首选 grok，回退 Opus 5 子代理，Fable 只做规格、审查、落盘、验收）。这会影响 Step 4 的组织方式。
