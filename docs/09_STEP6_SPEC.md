# Step 6 规格：Provider 适配器与一轮调用

Step 5 交付的是一个有本地事实源、但只有人能说话的 REPL。Step 6 让 agent 真的开口：
把三家 CLI（claude / grok / agy）的 headless 调用与 JSON 输出归一化成 `Provider` 适配器
（纯 flag 翻译 + 纯函数解析），再用一个 `turn.Runner` 把「挑批次 -> 渲染 -> 起进程 -> 计时 ->
解析 -> 算差值 -> 落库 -> 推进会话状态」串成一条没有分支的直线，REPL 加一条
`/ask <agent-id> [文本]` 就能让一个成员说一轮话。

**本步只往库里写一样东西：带脑子的 `KindAgent` 消息，外加会话与失败事件的 `KindSystem`
面包屑。** provider 会话 id、游标、cost 累计基线三样同生共死地活在 Runner 的进程内存里；
`turn_usage` 与 `provider_sessions` 的内容组装成 `turn.Record` 一次性交给 Step 7，
本步不落它们的库。**不碰 `internal/store` 的任何一行，不改 schema 的任何一个字**
（`08a` 冻结时就把 `Open` 装上了形状指纹校验，改了字段会在 `Open` 时就地炸掉）。

**明确不做**：不做 Context Router 的路由规则（本步的挑块规则是「游标之后的全部」这一条直线，
接口按 Router 的形状留好）；不做 bootstrap 包；不支持热切 model/effort；不做并发扇出
（一把全局锁，整轮持有）；不注册 codex 适配器（Step 0 零观测，`exec resume` 语义与另三家不同，
不照文档猜）；不改 HOME 去隔离指令环境（会连登录态一起隔离掉，`CleanInstructionEnv` 三家全声明
`Unknown`）。

**本步的难点不是拼命令行，是三件事**：让 D-6 的会话 id 分裂不蔓延出适配器边界；让 D-4 的
缓存前缀稳定性有一条能被测试钉住的路径而不只是一句叮嘱；以及承认**「provider 侧已经发生了、
本地没记上」这第三种结局的存在**。前两件在设计评审阶段的几个方案里都做到了，第三件是本规格
相对它们唯一的结构性增补，在 A 节与 G 节各出现一次。

## A. 前置：Step 6.0 探针

**这三项已经跑过一轮（2026-09-20），原始输出与完整记录在
`E:\Isolating_test\council\step6-probe\STEP6-PROBE-RESULTS.md`。**
下表是原计划；每项的实际结论紧跟在表后。**结果推翻了本规格起草时的一个共同前提**，
见「探针推翻了什么」一节。

写代码之前先补三项探针，都很便宜，且每一项都决定后面某个类型的取值或某条规则的置信度：

| # | 探针 | 决定什么 |
|---|---|---|
| 1 | **故意触发一次 claude 的 `is_error=true`**（例如给一个不存在的 model 名）并把原始输出留档 | G 节 `OutcomeProviderFailed` 的判据现在完全是从成功样本的字段名反推出来的（七份探针全是成功轮，见 `05` §3 第 7 条陷阱）。没有这一项，那条判据只是推测，标 `[未验证]`。这是三项里**唯一被列为前置条件而非可选项**的一项：K 节测试矩阵把它列在「provider 自报失败」这组用例的最前面，没有它那组用例全部标 `[未验证]` |
| 2 | 三家在 `-p` 模式下**读不读 stdin** | 决定 `PromptTransport` 是否可能是 `PromptTransportStdin`，进而决定 J 节「prompt 传输」那条能不能在 Step 7 就动手，还是要等一次新探针 |
| 3 | grok 对一个**已存在**的 `--session-id` 与一个**不存在**的 `--resume` 各自怎么反应；claude `--resume` 一个不存在的 id 是报错还是另起新会话 | 决定 G 节里 `ErrTurnFailed` 与 `Drifted` 的具体判据；也是 D 节「grok 原样回显预指定 uuid」这条前提在本机第一次被直接验证的机会 |

三项探针不改变本规格的结构，只影响几个具体判据的置信度标记。已知结论标注在对应小节；
没有探针支持的判据统一标 `[未验证]`，不假装是定论。

### 探针推翻了什么

**结构没被推翻**（G 节那张九行结局表里，`OutcomeTimeout` 与 `OutcomeUnparsable` 本来就在），
但有一个**四版设计与本规格初稿共有的前提是错的**：

> **失败不一定产出 JSON。** `--output-format json` 只约束成功路径。

实测：`claude --resume <不存在的 id> --output-format json` 的 **stdout 是 0 字节**，
错误只在 stderr 里，纯文本：`No conversation found with session ID: ...`。
所以 `ParseTurn` 面对的常态失败是「stdout 空 + 非零退出 + stderr 有文本」，
而不是「JSON 里写着 is_error」。G 节第 6 行的「stdout 前若干字节」要按可能为空来实现。

**另一条更严重的**，见下面 D 节与 G 节都引用的那条纪律：

> **grok 对未知会话 id 不报错，它拉起交互式 OAuth 然后无限阻塞。**

实测 stderr：`Session "..." not found locally, restoring conversation from remote...` ->
`To sign in, open this URL in your browser: https://accounts.x.ai/oauth2/device?...` ->
`Waiting for authorization...`。进程 4 分钟只吃 0.6 秒 CPU，在等 I/O，手工 kill 才结束。
这条路**绕开 CPA 直连 x.ai**。对无人值守的编排器，这是最坏的一种失败：
占着槽位永不返回，而且没有任何错误可捕获。

由此得到两条**硬性**要求（不是建议）：

1. **每一次 spawn 都必须带硬超时**，没有「它自己会报错」这条退路。
2. **未经输出确认的会话 id 绝不能拿去 `--resume`。** D 节那条「id 未被确认就一律作废」
   的纪律，理由从「洁癖」升级成「防死锁」。

### 三项探针的实际结论

| # | 状态 | 结论 |
|---|---|---|
| 1 | **已闭合（2026-09-21）** | 按上一轮自己写下的办法做的：给一个不存在的 model 名。三家全部产出 JSON，退出码都是 1，claude 的 `total_cost_usd` 是 **0**，预计的费用没有发生。原件三份留档在 `E:\Isolating_test\council\step6-probe\{claude,grok,agy}-badmodel.out`，结论见下面「探针 1 闭合后得到了什么」 |
| 2 | **部分闭合** | agy 的报错文本自证支持 stdin：`Prompts are read only from -p/--print, -i/--prompt-interactive, or stdin`。claude 与 grok 的 stdin 行为**未测**，`[未验证]` |
| 3 | **已闭合** | 见上面「探针推翻了什么」。另：grok 经 CPA 的正常调用输出形状与 Step 0 完全一致；`total_tokens 20336 = input 19969 + output 239 + cache_read 128`，再次证实 grok 的 `input_tokens` 不含缓存命中 |

### 探针 1 闭合后得到了什么

三份失败原件（2026-09-21，`step6-probe/{claude,grok,agy}-badmodel.out`，触发方式都是
给一个不存在的 model 名，三家退出码都是 1）。

**最值钱的一条，claude 在一次失败的轮次里 `subtype` 仍然是 `"success"`。** 完整取值：

```
is_error: true        subtype: "success"       type: "result"
terminal_reason: "api_error"                   api_error_status: 404
stop_reason: "stop_sequence"                   modelUsage: {}
total_cost_usd: 0     usage: 五项全为真实的 0
result: "There's an issue with the selected model (...). It may not exist or you may not
         have access to it. Run --model to pick a different model."
```

谁把 `subtype == "success"` 当成功信号，谁就把判定读反了，并且会把 `result` 里那段错误
正文当成 agent 的回答写进只追加的事实源。`ParseTurn` 现在的成功闸是五项合取，这份样本
被其中**四道独立拦下**（`is_error` / `terminal_reason` / `api_error_status` / `stop_reason`）。
这条合取此前的理由是「不猜」，现在是实测。另外 `stop_reason` 的第四个取值
`"stop_sequence"` 也是这次才见到的，`api_error_status` 是**整数**不是对象也不是 null
（用 `json.RawMessage` 接住是对的，换成具体类型会让整份输出掉进 `ErrUnparseableOutput`，
失去「失败」与「不可解析」的区分）。

**grok 有错误信封**，此前写在代码注释里的「grok 连一个错误字段都没有」是错的，只是没触发过：

```
{"type":"error","message":"Couldn't set model '...': Invalid params: \"unknown model id\". ..."}
```

成功形状的五个字段（`stopReason` / `sessionId` / `text` / `usage` / `modelUsage`）全是零值，
所以旧的 shaped 判据会把它判成「连结构都认不出」，退化成 `ErrUnparseableOutput`，
把一条读得懂的错误信息丢掉。已按此新增 `type` / `message` 两个字段，归 `ErrTurnFailed`。

**agy 的失败正文在一个此前没取的 `error` 键里**，`status` 变成 `"ERROR"`，
`response` 是空串，`conversation_id` 是空串，`usage` 五项是真实的 0。
不取那个键，这一轮就只剩一个 `"ERROR"` 字样，诊断信息要靠重跑才拿得回来。

**一条被证伪的启发式**：此前代码里有一个「`usage` 四项全为 0 就记进 `Unreported`」的
兜底，理由是「全零多半说明这家没报」。三份原件里 claude 与 agy 的 `usage` 键都在、
数字都是货真价实的 0（那一轮确实什么都没消耗）。留着它，每一次失败轮都会谎称用量
没被上报。已删除；「没报」唯一可靠的判据是键本身缺席。

**这份样本没有回答的问题**：它是在请求真正发出去**之前**被服务端以 404 拒掉的
（`usage` 全零、`total_cost_usd` 为 0 即是证据），所以它**不能**用来判断
「provider 已经把 delta 吃进会话之后才失败」那一类。G 节「第 5-7 行不推游标」
那条决定所依赖的正是后一类，**它仍然 `[未验证]`**，需要另一次探针（让调用真的发生、
再在中途失败）才能闭合。不要拿这次的成功当那条的证据。

### 第一次真机调用（2026-09-21）：四个只有真跑才抓得到的缺陷

实现全部落地、350 余个单测全绿之后，做了第一次真实 provider 调用。
配置与全部输出留档在 `E:\Isolating_test\council\step6-smoke\`。
**四个缺陷全部在第一次真跑时暴露，没有一个能被静态审查、单元测试或契约核对抓到**
—— 因为它们都是三家 CLI 的真实行为与文档之间的差，而那个差不在任何一份文档里。

| # | 缺陷 | 表象 | 为什么单测抓不到 |
|---|---|---|---|
| 1 | grok 的正文必须紧跟 `-p` | `a value is required for '--single <PROMPT>'`，退出码 2，stdout 零字节 | 单测只能断言我们发了什么，不能断言对方怎么解析 |
| 2 | agy 的正文也必须紧跟 `-p` | `-p took "--output-format" as its prompt, so the intended prompt was left as an argument and ignored`，退出码 2 | 它的 `--help` 把 `-p` 印成「Short alias for --print」、不带值占位符，看上去是 bool |
| 3 | `.cmd` 垫片在 argv 的换行处截断命令行 | claude 报 `Input must be provided either through stdin or as a prompt argument` | 需要同时具备「.cmd 垫片」与「多行 argv 元素」两个条件，而两者都在进程边界之外 |
| 4 | agy 在 `--mode plan` 下正文为空 | `status=SUCCESS`、`out=724 thinking=624`、`response=""` | 需要真的消耗 token 才看得见 |

前两条已修（正文绑到 `-p` 上）。第三条见未决 #16，处置是 claude 改走
`--append-system-prompt-file` 加 stdin，并在 `Spawner` 里加一道 fail-closed 的闸。
第四条见未决 #17，留给 Step 7 或 8。

**三家 CLI 的正文怎么递，互不相同**，这正是 `Provider` 这层抽象该吸收的差异：

| provider | 正文 | 身份块 |
|---|---|---|
| claude | stdin（原为末尾位置参数，因 `.cmd` 垫片改掉） | `--append-system-prompt-file <文件>` |
| grok | `-p <正文>`，绑在 flag 上 | `--system-prompt-override <文本>`（原生 exe，多行安全） |
| agy | `-p <正文>`，绑在 flag 上 | 拼进首轮正文最前面 |

### 第三种结局在真机上自己发生了

本规格相对四版天真设计**唯一的结构性增补**是第三种结局：provider 侧已经发生了、
钱花了、它的会话已经多了一轮，但我们本地没记上。它最难验证，因为要制造一个
「provider 成功、`Post` 失败」的窗口。真机冒烟时它自己撞上来了：

```
/leave gk
/ask gk 还在吗
-> outcome post_rejected
   正文写不进库（已经付过钱），整段如下:
   收到
   用量 in=19849 cache_create=0 cache_read=19584 out=399 thinking=398
   scanned_to=5
```

逐条对上了：grok 真的跑了、真的消耗了 token、真的回答了；`Post` 被写事务内的
`RequireActiveParticipant` 拒掉；那段付过钱的正文**整段打到屏幕上**（库里记不上它，
屏幕是唯一还拿得回它的地方）；追加了一条 `KindSystem` 面包屑；
**游标照推**，因为 `Ingested()` 对 `OutcomePostRejected` 为真 ——
对面已经吃下这批 delta 了，重发只会重复烧钱。

顺带证实了 Step 5 那个 TOCTOU 修复真的拦得住：活跃成员校验在与追加消息同一个
`WriteTx` 里，`/leave` 已提交之后那条 `Post` 必然被拒，而不是靠事务外先查一次。

同一轮还验了三条不花钱的拒绝路径：未知 agent（`ErrAgentUnknown`，起进程之前就拒）、
没有新内容（`ErrNoDelta`）、`/session` 在没有活跃成员时给提示而不是打一张空表。

### 真机跑通之后闭合的三条 `[未验证]`

1. **grok 原样回显预指定的 uuid。** D 节那条前提原先标「本机没有直接验过，
   等 A 节探针 3 或专门的一次调用补上」。现在验了：送出去的 `PlannedSessionID`
   与回来的 `TurnResult.ProviderSession` 相同，`Record.Drifted` 为假。
2. **`--resume` 那条路通。** 第二轮 `in=19849`、`cache_read=19584`、
   delta 只有 36 字节 —— `skipSelf` 正确跳过了这个 agent 自己上一轮的发言，
   只送了那条新的用户消息；游标从 1 推到 4。
3. **缓存前缀跨会话共享**，见未决 #1。这条顺带说明 `ColdExpected` 偏保守。

### 一条计划外的发现：agy 的版本已经漂移

本机 agy 实测 **1.2.7**，而 `02` §1.1 的能力表记的是 **1.1.26** —— Step 0 那两份 agy 探针
是旧版产出的。**本轮实测 1.2.7 的输出形状与 1.1.26 逐项一致**（六个顶层键、`response`
仍带尾换行、`usage` 仍是那五个子键），所以适配器不必改；但 `02` 的版本号要更正，
而且这件事本身证明「CLI 版本会在两次会话之间变掉」不是假想风险。
`Capabilities` 里带 CLI 版本、并在版本变化时视为前缀可能已变，是有实据的要求。

## B. 包结构

两个新包，加上对四个既有包的增量改动。依赖方向严格单向，编译期挡得住循环：

```
repl -> turn -> provider -> (标准库)
          |
          +-> conversation -> (标准库)
          |
          +-> config

store -> conversation          （Step 5 已有，本步一行不动）
provider 不认识 config，不认识 conversation，不认识 SQL，也不起进程
```

`provider` 刻意不导入 `conversation`：适配器吐的是 Text / Thought / Usage 三样原始材料，
拼成 `conversation.Message` 是 `turn` 的事。这样适配器的单元测试只需要几份 JSON，
不用拖着领域类型跑。

### internal/provider（契约层 + 三家适配器）

| 文件 | 职责 |
|---|---|
| `provider.go` | `Provider` 接口、`Register` / `Lookup` / `IDs`、八个哨兵错误 |
| `capabilities.go` | `Capabilities` 与六个枚举 + `Support` + `UsageFormula`，零值全是 Unknown |
| `turn.go` | `TurnRequest` / `Command` / `DroppedOption` / `Usage` / `TurnResult` / `Completion` / `CostBasis` |
| `claude.go` | `-p --output-format json --model <m> [--effort <e>] --append-system-prompt <身份块> --permission-mode default [--resume <id>] <prompt>`；解析见 09a |
| `grok.go` | 空会话发 `--session-id <本地现铸 uuid>`，非空发 `--resume <id>`；`--system-prompt-override <身份块>`、`--permission-mode default`；**不发任何 effort flag**（E 节）。多一个 `uuid.go`：`crypto/rand` 取 16 字节、设版本位与变体位、按 8-4-4-4-12 格式化 —— **不引 `github.com/google/uuid`**，那是 `modernc.org/sqlite` 的传递依赖，不是我们的 API |
| `agy.go` | `-p --output-format json --model <m> --mode plan --add-dir <workdir> [--conversation <id>]`；`Identity` 是 `IdentityPromptPreamble`（`--agent` 收的是预设名不是任意文本），空会话时把身份块拼在首轮正文最前面；`response` 必须 `TrimRight("\n")`。**本行 2026-09-21 订正过两处，见下面「agy 的 model 与 effort」** |
| `testdata/` | 七份探针 JSON 原样拷进仓库。测试不得引用 `E:\` 绝对路径 —— D 盘机上没有那个盘，测试要能在两台机器上跑 |
| `claude_test.go` / `grok_test.go` / `agy_test.go` / `registry_test.go` | 见 K 节 |

### agy 的 model 与 effort（2026-09-21 订正，本节原来是错的）

上表 `agy.go` 那一行原先写的是 `-p --output-format json --mode plan [--effort <e>] [--conversation <id>]`
—— **漏了 `--model`，而且多了一个不该发的 `--effort`**。实现是照着这一行写的，所以这是
规格缺陷，不是实现走样。改这两处的理由不对称，分开说。

**必须发 `--model`。** 三重后果，指向同一个方向：

1. `AgentConfig.Model` 是被 `config.Validate` 校验过的**必填**字段，却对这一家静默失效。
   静默丢弃正是本项目一路在防的那类坑（`02` §1.1 grok 的 effort 就是这么栽的）。
2. agy 的 `ReportsObservedModel` 是 `SupportNo`：它的输出里没有 `modelUsage`，
   **我们没有任何办法事后知道它实际跑了哪个模型**。不发这个 flag，
   `messages.sender_model` 记下的那个名字就既无从强制、也无从核对 ——
   成了写进只追加事实源里的一句纯粹的编造。别家至少还能用 `observed_model` 对账，
   这一家一旦不发就彻底失去抓手，所以这一条对它比对另两家更要紧。
3. agy 把推理强度**编进模型标签**：它自己的模型目录里 `Gemini 3.8 Flash` 的
   High / Medium / Low 是三个并列的模型名（原件见
   `E:\Isolating_test\council\step6-probe\agy-badmodel.err`）。不发标签等于强度也没发，
   而 `IdentityFingerprint` 那边已经按配置值算过指纹了 —— 指纹声称的身份从未被送出去。

**不发 `--effort`，改记进 `Command.Dropped`。** `--effort` 这个 flag 确实存在
（失败输出里 agy 把 `--effort ""` 原样回显过），但既然强度由模型标签决定，
两者如何交互本机**未实测**。明知可能无效还发，是在给自己伪造一份「强度已按配置生效」
的证据 —— 与 grok 那一格同一类错误，方向相反：那边是发了会被能力门吞掉，
这边是发了也盖不过标签。

**连带的一处收益**：`Capabilities.Effort` 因此从 `EffortPerTurnFlag` 改成
`EffortBakedIntoModelLabel`（这个枚举值本来就是为这种情况留的）。于是
`effortFlagEmitted` 对 agy 恒为空串，指纹的行为自动变对：改 `model` 触发身份漂移
（对 agy 而言强度就在里面，该漂移），改 `effort` 不触发（它本来就没送出去，
前缀没变，重建纯属白烧一次入场费）。不需要为 agy 写任何特例分支。

**不写 `codex.go`。** 配置里写了 codex 的 agent 在启动横幅上被点名「没有适配器（Step 0 未实测）」，
`/ask` 时返回包了 `ErrUnknownProvider` 的错误并列出已登记的 id。

### internal/turn（编排层）

| 文件 | 职责 |
|---|---|
| `runner.go` | `Runner` / `Spawner` / `Ask` / `SessionInfo`，全部流程在这里 |
| `spawn.go` | `Spawner` 的生产实现：`exec.CommandContext` + stdout/stderr 分流 + 输出封顶 + **stdin 接 null device** + 超时后杀进程树 |
| `killtree_windows.go` / `killtree_other.go` | `taskkill /T /F /PID` 兜底，尽力而为并记进 `Record.MaybeAdvanced`（都是包内私有实现，不进 09a 的契约面） |
| `session.go` | `sessionKey` / `sessionState` / `CacheTTL` / `IdentityFingerprint` |
| `render.go` | `RenderDelta`，纯函数，过滤规则集中在这一处，golden 文件锁住 |
| `record.go` | `Record` / `Outcome` / `Outcome.Ingested` / Meta 键常量 |
| `permission.go` | `config.Permission` -> `provider.PermissionTier`；Step 6 只放行 read，另两档在起进程之前拒绝（`06` §2：FULL 不给编排器自动使用） |
| `runner_test.go` / `render_test.go` / `cost_test.go` / `session_test.go` | 见 K 节 |

### 既有包的改动

- `internal/conversation/types.go` + `service.go`：加 `ReservedSenderSystem` 与
  `ErrMissingSenderBrain`；`Post` 加两条校验（08a 的 MUST 与未决 #14）。**签名不变。**
- `internal/config/validate.go`：把 `"system"` 与 `"user"` 一同列为保留 agent id。
  additive，旧配置照常加载。
- `internal/config/load.go`：加载时若某 agent 的 provider 的 `EffortPolicy` 是
  `EffortProcessExternalConfig` 而 `Effort` 非空，往 `Err` 记一行警告：
  「这个 provider 的 effort 不可按次切换，配置里写的值不会生效」。**不是错误，不拦启动**。
- `internal/repl/commands.go` + `repl.go`：`command` 加 `needsRunner`（和 `needsSvc` /
  `needsConv` 一样只在命令表里写一次，由 `runCommand` 统一判）；新增 `/ask <agent-id> [文本]`
  （带文本时先走 `postUserText` —— 与 `/say`、裸文本共用同一个函数，人说的话永远以同一形状落库 ——
  再让 agent 跑一轮）与 `/session`（打印当前会话每个成员的 provider 会话 id、`last_turn_at`、
  下一轮是否冷启动）。
- `internal/app/app.go`：为每个已配置的 agent 建 `<data_dir>/workdir/<agent-id>/`
  （启动时 `MkdirAll`，一次性建完，Ask 期间不再现建目录），再建
  `turn.NewRunner(cfg, svc, <data_dir>/workdir)` 传进 `repl.New`。

**一处对 08a 冻结签名的修改，按 08a 自己的规矩「退回来说，不要自行改」**：`repl.New` 要多一个
`rn *turn.Runner` 参数。这不是在本文件里悄悄改掉 08a，而是作为一条明文的修订请求列在 09a 里，
等编排者认可再动那份契约。`rn` 为 nil 时 `/ask` 与 `/session` 一律回一句「provider 调用未就绪」，
**不得 panic**，与现有 `svc == nil` 的兜底同风格。

### 文档

新增本文件与 `docs/09a_STEP6_API_CONTRACT.md`，沿用 08 / 08a 的分工。

`docs/05_MESSAGE_PROTOCOL.md` §3 那张归一化表在写作本文件时读到的已经是修订版
（五字段完成信号、原始探针逐字重核过），**不需要 Step 6 再补这个修正** —— 早期设计草稿里
提到要顺带改它，那处是针对旧版 05 写的，写这份规格时已经过时。

## C. Provider 契约层怎么设计

`Provider` 接口只有两个方法：

```go
ID() string
Capabilities() Capabilities
BuildTurn(req TurnRequest) (Command, error)
ParseTurn(stdout []byte) (TurnResult, error)
```

`BuildTurn` 是一层 flag 适配器：把一次 Council 调用翻译成这个 CLI 的命令行。
`ParseTurn` **只吃 stdout，是纯函数**：同样的字节必须给出同样的 `TurnResult`。
它刻意不收 `exitCode` 与 `stderr`——收了适配器就会忍不住拿它们做兜底。这条在 A 节探针 1
闭合后反而更站得住：那份 claude 失败样本的退出码是 1，可它同一份输出里 `subtype` 写着
`"success"`；退出码与 JSON 各说各话时，让纯函数去调和两者只会调和出一个编出来的结论。
退出码由 Runner 判，`ParseTurn` 只对字节负责。

它同样算不出 cost 差值、冷启动判定与挂钟耗时——那些要么需要上一轮的状态，要么需要时钟，
都在 `turn.Record` 上由 Runner 填（F 节）。这是对 `02` §1.2 伪码「`ParseTurn` 返回
`TurnResult`」的一处有意偏离，理由即此。

实现必须**无状态、可被同 provider 的多个 agent 并发复用**（`04` §0 规则 2：每个 agent
一条独立的流）：任何需要从 `BuildTurn` 带到 `ParseTurn` 的东西一律走 `Command` /
`TurnResult` 这两个值传递，绝不放进接收者——放进去就是两个 agent 共享状态，表现为串会话，
而且只在并发时出现。

### Capabilities 必须声明，不许猜

`02` §1.2 定死的约束：`Capabilities` 每一项都要能表达 `Unsupported` / `Unknown`，
绝不用一个默认值假装支持。六个枚举字段（`SessionID` / `Effort` / `Identity` / `Prompt` /
`ReportsCost` / `ReportsCacheCreate` / `ReportsObservedModel` / `CleanInstructionEnv`）
零值全是 `Unknown`，漏填一项的后果是「未实测」而不是「支持」。

新增一个字段 `UsageFormula`（ruling，见下）：每个 provider 的 `input_tokens` 归一化公式
不再只是写在 `Usage` 类型的 Go 注释里，而是提到 `Capabilities` 上，一处定义、跟着
`Capabilities()` 的实现走。理由见 F 节——三家的公式不是三个数量级噪声，是三条不同算法，
喂进同一列之前必须先知道各自怎么算的。

配置要求了这个 CLI 给不了的东西，`BuildTurn` 返回包了对应哨兵的错误，**绝不静默降级**：
降级成更低的档只是变慢，降级成更高的档是安全事故，而两者在代码里长得一样。
权限档不在 `Capabilities.Permissions` 里 -> `ErrUnsupportedTier`；`req.WorkDir` 为空 ->
`ErrNoWorkDir`；`Transport` 是 `Argv` 且 prompt 超 `MaxPromptBytes` -> `ErrPromptTooLong`；
`Capabilities` 里该项仍是 `Unknown` 而这一轮需要它 -> `ErrCapabilityUnknown`（不猜）。

## D. 会话 id：两个判定点，一层一个

**做法：把「首轮 vs 后续轮」压成两个具名的判定点，一层一个，其余任何地方都不许出现这个概念。**

### 判定点一，在适配器里

唯一的载体是 `TurnRequest.ProviderSession` 这个字符串是不是空：

| provider | 空（新建） | 非空（恢复） |
|---|---|---|
| grok（Preassignable） | 现场铸一个 v4 UUID 拼进 `--session-id`，同时写进 `Command.PlannedSessionID` | `--resume <id>` |
| claude（ReturnedOnFirstTurn） | 不带会话 flag | `--resume <id>` |
| agy（ReturnedOnFirstTurn） | 不带会话 flag，**并且**把身份块拼进首轮正文最前面 | `--conversation <id>` |

agy 那一格顺带证明这个判定点是通用的、不只管 session flag：它没有可用的文本身份 flag
（`--agent` 收的是预设名），所以身份注入挂在同一个判定上。

### 裁决：grok 的 uuid 在 BuildTurn 内部铸，不向上冒泡

设计评审阶段出现过一个更谨慎的版本，给编排层显式建了一个三态 `SessionOrigin`：
**无会话 / 已预指定但尚未被输出确认 / 已确认**。这三态描述的是**世界**可能处于的三种状态之一——
grok 预铸的 uuid 在 spawn 之后、输出返回之前，确实已经写进了子进程的命令行参数，
子进程这一刻可能正在用它建会话文件，这是真实存在过的一个中间态。

但采纳的做法（下称 mvp 版）不建这个模，理由不是「那个中间态不存在」，而是
**编排层永远没有手段去观测它**：`Command.PlannedSessionID` 只活在这一次 `BuildTurn` 调用的
返回值里，一旦这次 spawn 返回（无论成功还是失败），要么进了 `ParseTurn` 被输出确认，
要么整条 `Command` 连同 `PlannedSessionID` 一起被下一轮 `Ask` 覆盖丢弃（配套规则 1：
只有成功返回的 id 才落地）。给一个在两次 `Ask` 调用之间永远跨不过去、编排层永远读不到的状态
建一个类型，不是更精确，是在维护一个没人读的字段。**能被消除的歧义就不要去建模它。**

`req.ProviderSession` 是不是空串，对三家统一等于「空 = 新建」，`02` §1.2 那句注释
「session 为空表示开新会话」不再对 grok 撒谎——这正是能拿掉三态模型的原因：判定点一
在 grok 那个中间态存在期间也完全不需要改变判据，它仍然是「空」，因为下一次 `Ask` 发生时，
这次 `Command` 早已经决出结果。

这条成立的前提是**grok 会把预指定的 uuid 原样回显**：设计评审阶段独立核实过这一点，
所以「一律以输出里的 id 为准」对三家逐字相同。本机自己的 Step 0 探针没有专门测过
「预指定 `--session-id` 之后 grok 是否原样回显」这一条（探针测的是 grok 自生会话 id 之后
`--resume` 的路径），A 节探针 3（grok 对已存在 `--session-id` 的反应）与这条前提相关但不完全
等价，**这一条本身仍标 `[未验证]`，等 A 节探针 3 或专门的一次调用补上**。

**`Command.PlannedSessionID` 只在它真正被读取的场合才计入契约，不多加字段**：与
`TurnResult.ProviderSession` 做漂移比对（配套规则 3），以及失败时在诊断信息里点名这个可能的
孤儿会话。除此之外不因为「将来可能有用」而添加任何诊断字段——没人读的字段是维护负担，
不是保险。

### 判定点二，在 Runner 里

只有一句话：`st, ok := r.sess[key]; fresh := !ok`。冷启动的四件事全从这一句流出来——
游标归 0、cost 基线归 0、渲染时不跳过自己说过的话、发一条 `session_opening` 面包屑。

`skipSelf = (req.ProviderSession != "")` **不是第三个判定**，它由判定点一的同一个谓词导出。
一条全新的 provider 会话里什么都没有，包括这个 agent 自己过去的发言，所以新会话那一轮必须
重放自己；resume 那一轮则必须跳过（那些话已经在 CLI 那边的会话里了，再送一遍既费钱又会让
模型看见自己的回声）。

### 它如何不蔓延，逐层说

- **编排层**：主流程是一条直线，读不到「第几轮」：`req.ProviderSession = st.providerSession`
  -> `BuildTurn` -> spawn -> `ParseTurn` -> `st.providerSession = res.ProviderSession`。
  最后那一句是无条件的，因为**三家的 `ParseTurn` 都必须把 id 填进
  `TurnResult.ProviderSession`**（grok 填的是它自己铸的、被 CLI 原样回显的那个）。
- **持久化层**：`Record.ProviderSession` 永远是一个非空 id，Step 7 的 `provider_sessions`
  upsert 对两类 provider 是同一条语句，不分插入与更新。
- **错误处理层**：只有一条规则——**失败就什么都不写**：不写 session、不推游标、不推基线。
  grok 那个已铸的 `PlannedSessionID` 在失败时一律作废，下一轮重铸一个新的。这一条让
  「grok 失败」与「claude 失败」在代码里长得一模一样。
- `Capabilities.SessionID` 仍然如实声明 Preassignable / ReturnedOnFirstTurn，但**编排层
  一次都不读它**。它是给 Step 9 排并发与预建会话用的元数据，留在那里只是因为不声明就等于撒谎。
  有一条 conformance 测试扫 `internal/turn` 的源文件，断言 `SessionIDPolicy` 这个标识符
  在编排层一次都不出现——规则只靠人自觉，第三次改动时就会失守。

### 配套五条

1. **只有成功返回的 id 才落地。** grok 首轮失败时预先铸的 uuid 直接丢弃，下次重铸；
   grok 要求 uuid 不得已存在，而我们**从不复用、从不派生**，所以不会撞。这同时排除了
   「从 agent id 确定性派生一个固定 uuid」那条路——那会让 `/new` 开的第二场会话要么撞 id、
   要么直接继承第一场的全部上下文，两场讨论串台而 Canonical Conversation 里没有任何一条
   消息解释得了。这条与 `04` §6「失败不推进游标」是同一条规则的两半。
2. **spawn 从没跑起来时更是什么都不写。** grok 不在 PATH、CPA 在 Mac 上睡着、进程立刻非零
   退出——这些都走同一条「失败就什么都不写」的路，不会留下一个 provider 侧从未存在过的会话 id
   让下一轮永远 `--resume` 它。
3. **漂移检测。** Runner 把回传的 id 与送出去的 id（`req.ProviderSession`，或首轮的
   `cmd.PlannedSessionID`）比一遍，不等就是 `Record.Drifted = true`：CLI 悄悄开了新会话，
   前缀已经断了，钱已经花了。**这是本设计里唯一能在事后抓到静默重建的钩子，两行字符串比较。**
   命中时除了告警，还必须**把 cost 基线归零**——不然下一条差值是负数（F 节）。
4. **身份指纹守住会话生命周期。** `IdentityFingerprint(role, model, 实际发出的 effort flag,
   权限档)` 在建会话时存进 `sessionState`，每轮比对。不一致就 `ErrIdentityDrift` 并点名
   哪一项变了，**不闷头重建**——Step 6 不支持热切脑子（`02` §4：四家都不支持在已有会话里改
   model 或 effort），重建是一次 0.4 到 1.0 美元的有标价操作，得由人来按（Step 8 做那个开关）。
   指纹取「实际发出的 flag」而不是 `AgentConfig.Effort`，是为了让 `EffortProcessExternalConfig`
   的 provider 这一位恒为空串：否则用户改一个反正会被静默丢弃的值，就触发一次什么都没变的、
   20k 入场费的空重建。指纹**绝不含 ObservedModel**，否则 provider 换个型号别名就白烧一次重建。
5. **A 节探针 3** 决定 grok 对已存在 `--session-id`、claude 对不存在 `--resume` 的具体反应，
   三次调用测完，它们决定 G 节里 `ErrTurnFailed` 与 `Drifted` 的具体判据。

## E. Effort 记录：请求值与实测值分开

**选 (a)：把 `AgentConfig.Effort` 原样抄进 `messages.sender_effort`，三家一视同仁。**

### 两条理由，第一条是决定性的

1. **08a 是冻结契约，它已经把这件事裁了。** `08a_STEP5_API_CONTRACT.md` 明写「Step 6 的
   适配器只能把 `AgentConfig.Effort` 抄进这一列。**不要把这列当 ground truth。**」而 08a
   开篇写明「实现者不得改动本文件里的签名；觉得签名有问题，退回来说，不要自行改」。选 (b) 或 (c)
   都是实现者自行改冻结契约——要改得先改文档，那不是 Step 6 的活。
2. **(b)（provider 没有 effort 概念就留空）会让这一列更糊涂而不是更诚实。** 空串现在同时
   表示「这家 provider 没有 effort 这个概念」与「有但适配器漏填了」。给 grok 留空等于往这个
   重载里塞第三种含义「有这个概念但我们不敢报」。三义比两义差，而且
   `WHERE sender_effort = 'high'` 这类查询会静默漏掉全部 grok 行。agy 更糟：它的 effort 同时
   以 model label 后缀和 `--effort` flag 两种形式存在、交互未实测（`02` §1.2 的 Unknown），
   留空同样说不出「有但不确定哪个赢」。

### 放弃 (c)（读 `~/.grok/config.toml` 拿实测值）的理由，三条，第一条是硬的

1. **读那个文件就是读凭据。** 本机 `~/.grok/config.toml` 的 `[model."grok-4.6"]` 段里，
   `api_key` 与 `reasoning_effort` 就在相邻两行，另有若干 `[model.*]` 段各自带 `api_key`，
   `[endpoints]` 段还写着内网地址。`06` §1 写死「不读取、不复制、不存储任何 provider 的
   认证凭据」。TOML 解析是整文件加载，密钥会进 Council 的进程内存，一条带原文的解析错误就能
   把它打进日志——而 Canonical Conversation 只追加不回收，删不掉，用户导出 `/log`、把库发给
   别人排查时一并带走。为了一个展示用的档位字符串去破这条，不值。
2. **就算读了也不是 ground truth，只是换了个地方猜。** 那个文件里的回落顺序（默认档位 ->
   具体型号段）要做对，Council 得实现 grok 的整条回落顺序并在每次 grok 升级后重新验证。
   更要命的是 `02` §1.1 的实测结论：真正做取舍的是本机 CPA 的能力门，配置文件只是
   **送进门之前**的请求值。号称「唯一为真」的选项，实际只是把一个可能错的来源换成了另一个。
3. **三家的输出里一个 effort 字段都没有。** 七份 JSON 全文检索 `effort`，零命中。
   也就是说**没有任何选项能让这一列成为实测值**，(a)(b)(c) 之外不存在第四条路。

**门没关死。** `EffortProcessExternalConfig` 这个枚举值本身就是给它留的位置。将来真要做，
形态必须是「只读、只取 effort、默认关闭、由用户在 config 里显式开、解析失败静默回落到 (a)
并记一条 warning」，而且要先有一次实测证明 CPA 不再压档，并先解决「那个文件里有凭据」这一条。
在 `06` §1 改口之前这条路是封的（见 L 节 #15）。

### 三条零成本的补救，都不改 schema

1. **`EffortPolicy` 是 `EffortProcessExternalConfig` 的 provider，适配器根本不发 effort
   flag。** 明知会被能力门静默丢弃还发，是在给自己伪造证据，而且会让命令行、诊断输出和
   三个月后的排查都在撒同一个谎。不发，并把 `Meta["effort_flag_omitted"] = true` 与这一项
   写进 `Command.Dropped`。**这同时是缓存前缀的要求**：flag 每轮必须逐字节一致，而一个
   「发了也没用」的 flag 是最容易被将来某个人顺手删掉的那一个。
2. **`Meta["effort_policy"]` 与 `Meta["effort_source"]`。** 前者写 `per_turn_flag` /
   `process_external_config` / `unsupported` / `unknown`，后者恒为 `requested`。于是
   `sender_effort = "high"` 这一格什么时候可信、什么时候只是个愿望，库里自带解释。
3. **同一条规则应用到 `sender_model`，但方向相反。** `sender_model` 写**请求值**，实测标签
   （如 `claude-fable-5-1`、`grok-4.6-build`）进 `Meta["observed_model"]` 并配
   `Meta["model_source"]`。`modelUsage` 的键数不等于 1 时（claude 的 modelUsage 是累计的，
   一旦某轮用过第二个型号，此后每轮都是多键）`observed_model` 留空、`model_source` 记
   `ambiguous`——宁可标不确定，也不挑一个键假装知道。

### 一条加载时的警告

配置加载时，若某 agent 的 provider 的 `EffortPolicy` 是 `EffortProcessExternalConfig` 而
`AgentConfig.Effort` 非空，往 `Err` 记一行：「这个 provider 的 effort 不可按次切换，
配置里写的值不会生效」。**不是错误，不拦启动。** 没有这一行，用户改一次 grok 的 effort、
重问一遍同一个问题，会发现回答质量与 `reasoning_tokens` 毫无差别，而库里清清楚楚记着一次
high 一次 low，于是得出「effort 对 grok 没影响」这个结论——Canonical Conversation 从事实源
变成伪证，而且只追加、纠正不了。

### 未决 #11 往前推一格

既然三家输出里都没有 effort，NULL 也好空串也好都不可能从输出里填出来，这个未决**不需要再等
新证据**，只等一次 schema 改动的窗口。裁决与改法进 L 节。

## F. Usage 归一化与 cost 差值

### 三家的 InputTokens 公式不是同一个数量级，是同一件事的三种算法

D-5 的入场费表（claude 47,749 / grok 20,625 / agy 18,180）之所以三家可比，是因为它们都在
**冷启动、`cache_read=0`** 时测的。暖轮时「本轮完整 prompt」对三家不是同一个字段：

```
claude: input_tokens + cache_creation_input_tokens + cache_read_input_tokens
        (t1 = 2+47749+0 = 47751, t2 = 2+19363+28436 = 47801)
grok:   input_tokens + cache_read_input_tokens        —— 它的 input 不含缓存命中
        (t1 = 20625+0 = 20625, t2 = 250+20608 = 20858；实测 total 21217 = 250+20608+359 可验算)
agy:    input_tokens 原样                             —— 它的 input 本来就含 cache_read
        (t1 = 18180, t2 = 20348；实测 total 20482 = 20348+134 可验算，且 16282 < 20348)
```

`turn_usage.input_tokens` 存的是「本轮完整 prompt」，理由是它是 Council 唯一能做成本管理的
抓手，而 D-5 那张入场费表记的就是这个量。**但三个公式喂一列**，所以：

- 列存「完整 prompt」；公式放进 `Capabilities.UsageFormula`（C 节新增字段），一处定义，
  每个适配器在自己的 `Capabilities()` 里写出人类可读的公式字符串，不再只是散落在 `Usage`
  类型的 Go 注释里
- 三家的原始 usage 对象整个进 `messages.meta["usage_raw"]`，将来改口径可回算
- **`turn_usage.input_tokens` 是按 provider 各自公式算出的「本轮完整 prompt」，公式在
  `Capabilities.UsageFormula` 里；跨 provider 比较前先看 `Meta["usage_raw"]`。** 这句话
  逐字写进 09a，是 Step 7 及以后任何做成本视图的人第一件要读的东西。

### ThinkingTokens 已经包含在 OutputTokens 里，任何视图不许相加

grok 723 含 722（`reasoning_tokens`），agy 55 含 54（`thinking_tokens`）均实测；
claude 三轮 thinking 恒为 0，包含关系本身未能被这三轮数据证实（0 加 0 看不出谁包含谁），
**按同规则处理，不因为观测不到就假设它是例外**。这条规则不是新引入的——它与
`05` §3 第 4 条陷阱是同一件事，本节把它重申一遍是因为它极容易被下一个只看 `Usage` 类型
定义、没看这两份文档的人漏掉。

### Unreported：诚实地说「没报」

`Usage.Unreported` 列出哪些 0 是「provider 没报这个数」而不是「实测为 0」。两处实测：
agy 的 usage 里完全没有 `cache_creation` 这一项；grok 的 `cache_creation_input_tokens`
在 t1 与 t2 都是 0，而 t2 明明读了 20608 的缓存。两处都必须进这张表，否则成本视图会显示
这两家从不建缓存。`turn_usage.cache_create` 是 `NOT NULL DEFAULT 0`，Step 6 唯一能说实话
的地方是 `messages.meta["usage_unreported"]`，数据不丢，只是查不动（改法见 L 节 #3）。

### cost 差值：Runner 算，在 ParseTurn 返回的那一刻算

**适配器算不了**：它只看得见这一轮的 stdout，`total_cost_usd` 是会话累计值
（0.9552 -> 1.349789 -> 1.363847），上一轮的累计值不在这段字节里，硬塞一个 prev 参数
就是把跨轮状态塞进纯函数。**Step 7 在写事务里更算不了**：`turn_usage` 的主键是
`(conv_id, agent_id, seq)`，表里没有 `provider_session_id` 这一列（L 节 #2 要补的窟窿），
`SUM(turn_usage.cost_usd)` 必然横跨会话重建的边界，算出来是个负数。
**只有 Runner 同时看得见两样东西**：上一轮的累计值基线，和「这一轮是不是新会话」。

五条规则，**任何一条都不许猜一个数**：

1. `CostBasis == CostBasisNone`（grok / agy 根本不报金额）-> `CostUSDDelta = nil`，
   **不是 0**。全局成本视图是「claude 侧有钱、其余侧只有 token」的混合体，不折算。
2. 这条会话是本进程开的第一轮 -> 基线记 0，差值即累计值。t1 支持这条：
   `cache_read = 0`、累计 0.9552 就是首轮实价，重建冷启动那 0.955 美元被如实记下，
   不需要 `max(0, delta)` 那种把最贵事件抹成零的补丁。
3. `st.baselineSuspect` 为真（上一轮超时或解析不了，那一轮可能真的计了费而我们没记上）
   -> `CostUSDDelta = nil` 并记 `Meta["cost_baseline_suspect"] = true`。
4. `Record.Drifted` 为真（回传 id 与送出去的不同）-> 基线先归零再算，本轮差值即累计值。
   不做这一步，差值就是 `0.955 - 1.364 = -0.409` 这种负数。
5. 本轮累计值 < 基线（provider 侧把会话重置了，我们没察觉）-> `CostUSDDelta = nil` 并记
   原因。**永不写负数。**

差值算完立刻推进基线。推进的时机是 **`ParseTurn` 成功的那一刻，不是 `Post` 成功的那一刻**——
钱在前者就已经花掉了。`Usage.CostUSDTotal` 的原值始终随 `Record` 带出来，写进
`Meta["cost_cumulative_usd"]`，Step 7 加了列之后可以直接入表并回头对账历史差值。

### 其余字段

- `Cold = (Usage.CacheRead == 0)`（`05` §4）。`Usage.Unreported` 含 `"cache_read"` 时它
  没有意义，别拿它告警。
- `ColdExpected` 是 spawn **之前**的预报（新会话，或距 `LastTurnAt` 超 `CacheTTL`），
  好让 REPL 能在发出去之前告诉用户这一轮大概要花多少。
- `UnexpectedCold = !ColdExpected && Cold`，是 `05` §4「意料之外的 ColdStart 应当告警」的
  一行实现。
- **`ColdExpected` 与 `Fresh` 刻意是两个字段**：TTL 过期只让下一轮贵，会话本身照样 resume
  得上（G 节有独立一条讲这件事，这里先point一下：两者混为一谈会导致方向相反的两种昂贵错误）。
- `WallMS` 由 Runner 自己 `time.Since` 计时，**明确不取 JSON 里的任何时长字段**：三家的
  时长字段口径不同（claude 三个、agy 一个浮点秒、grok 没有），用它们的数字等于让三家的账
  不可比。

### 两条边界规则

- `ScannedTo` 取**本轮扫描到的最大 seq**，在 spawn 之前定下、在自己的回复落库之前定下。
  不是 `Delivered` 的最大值（末尾被过滤掉的行会把游标钉在原地），也不是 `MaxSeq(conv)`
  （飞行中别人拿到的 seq 会落在游标之后，那条发言永久漏投）。
- 过滤完一条都不剩时（连着敲两次 `/ask X`），不起进程，回 `ErrNoDelta`，**游标照常推到
  `ScannedTo`**——空 prompt 送给 `claude -p` 多半直接报错，而那次冷启动的钱已经付了。

### 为什么 Step 6 不写 cursors 表

`Tx.SetCursor` 已经存在，顺手把 cursors 也写了看起来白赚。**不做，理由是正确性不是工期**：
Step 6 的 provider session 只活在进程内存里，而游标一旦落库就活过重启。重启之后 agent
拿到的是一条全空的新会话，游标却声称它已经看过 seq 1..N——这个 agent 会带着「我已经读过了」
的账本走进一条什么都没有的会话，而库里没有任何一层看得出不对。所以
`providerSession` / `cursor` / `costTotal` / `lastTurnAt` / `identityFP` 放在同一个
`sessionState` 里，同一个生命周期，同生同灭，desync 这一类错误在 Step 6 根本不存在
（Step 7 接手时的义务见 L 节 #7）。

### 面包屑写但不读回

`meta["provider_session_id"]` 与 `meta["cost_cumulative_usd"]` 无条件写进每条 agent
消息与每条面包屑上，但 Step 6 **不读回**。

**不读回的理由不是「两个事实源」**——那个理由站不住：meta 从写下的那一刻起就已经是第二份
记录了，写不写回读都改变不了这一点。真正的理由是：**读回需要一次有效性校验，而 Step 6
没有任何手段做这个校验**。一个过期的 provider 会话 id 会被 CLI 静默 resume 成一条全新的
会话（前缀已经断了，模型已经失忆），本地唯一能探测到这件事的钩子是「送出去的 id 与回传的 id
不一致」这条漂移断言（D 节配套规则 3）——而它开火的时刻，冷启动的钱已经付过了。也就是说，
读回一个 meta 里的旧 id 去 `--resume`，最坏情况下等于**主动买一次每 agent 约 0.955 美元
的探针**，去验证一个我们本可以不问的问题。

写是免费的、无争议的，数据不丢；读回要等 Step 7 的 `provider_sessions` 表真正被写入之后——
那张表已有的 `cold` 与 `last_turn_at` 两列（Step 5 建表时就留好了形状）能在 resume 之前
先做一次「这条会话看起来还新鲜吗」的校验，读回才安全。**代价要说清**：在那之前，council
一重启，每个 agent 的下一轮都是冷启动——claude 侧实测 47,749 token / 0.955 美元一个人，
五人团队一次重启接近 5 美元，这恰恰是 `04` §0 要防的那件事。这不是疏忽，是「不碰 store」
这条铁律的直接后果，但后果就是后果，不因为有理由就变轻（详见 L 节 #8）。

## G. 结局分类：第三种结局

本节回答三个互不相同的问题，**判据也各不相同**——这是 Step 6 相对天真实现最重要的一处结构
增补。天真实现只有「成功」和「本轮失败」两种结局，没有第三种叫**「provider 侧已经发生了，
但我们没记上」**。

| # | 判据 | Outcome | 落正文 | `Ingested()` 推游标 | 推会话状态与基线 | 面包屑 | 屏幕 |
|---|---|---|---|---|---|---|---|
| 1 | 五信号全对 + 有会话 id | `OutcomeSpoke` | 是 | **是** | 是 | 仅 fresh 时 `session_opening` | 正文 + 脑子 + seq + 用量 + 冷暖 + 金额 + 耗时 |
| 2 | 同上，但 `stop_reason=max_tokens` 或撞 `--max-turns` | `OutcomeSpoke`，`Completion=Truncated` | 是 | **是** | 是 | 同上 | 同上 + 响亮地标「本轮被截断」 |
| 3 | 同上，但正文是空串 | `OutcomeSpoke`，`Completion=Empty` | 是 | **是** | 是 | 同上 | 同上 + 响亮地标「本轮为空」 |
| 4 | 1-3 任一成立，但 `Service.Post` 返回错误（典型：飞行中 `/leave`） | `OutcomePostRejected` | **否** | **是** | **是** | `post_rejected` | 整条正文打到屏幕上 + 错误原因 |
| 5 | provider 自报失败（判据见下，`[未验证]`） | `OutcomeProviderFailed` | 否 | 否 | 否，置 `baselineSuspect` | `turn_failed` | provider id + 退出码 + `StopRaw` + stderr 头若干行 |
| 6 | JSON 认不出 / 成功形状但没有会话 id | `OutcomeUnparsable` | 否 | 否 | 否，置 `baselineSuspect` | `turn_failed` | 同上 + stdout 前若干字节 |
| 7 | ctx 超时或被取消，进程被杀 | `OutcomeTimeout`，`MaybeAdvanced=true` | 否 | 否 | 否，置 `baselineSuspect` | `turn_failed` | 同上 + 「provider 侧可能已经吃下这批内容」 |
| 8 | 进程压根没起来 | `OutcomeSpawnFailed` | 否 | 否 | 否 | `turn_failed` | 可执行文件路径 + 系统错误 |
| 9 | 起进程**之前**就拒（权限档 / prompt 超长 / 身份指纹漂移 / 没有 WorkDir / 批次会改写历史 / 没有新内容） | `OutcomeRefused` | 否 | 仅 `ErrNoDelta` 时推 | 否 | 无 | Council 自己的具名错误 + 怎么办 |

「屏幕」列里第 5-9 行提到的退出码 / `stderr` 头 / 可执行文件路径这些诊断细节，`turn.Record`
**没有专门的字段承载**——它们被折进 `Record.Err` 这个错误值本身的格式化消息里（例如
`fmt.Errorf("provider %s exited %d: %s: %w", agentID, exitCode, head, provider.ErrTurnFailed)`），
REPL 只需要 `%v` 打印 `Err` 就够了，与现有 `reportErr` 的风格一致，不需要在 `Record`
上再加一组诊断字段。

### 两条被探针改写的实现细节

**一、第 6 行的「stdout 前若干字节」要按可能为空实现。**
实测 `claude --resume <不存在的 id> --output-format json` 的 stdout 是**0 字节**，
全部信息在 stderr 的纯文本里。`--output-format json` 只约束成功路径，
常态失败长成「stdout 空 + 非零退出 + stderr 有文本」。
诊断消息里 stdout 那一段经常什么都没有，**stderr 那一段才是唯一线索**，
格式化时不要把它当可选项。

**二、第 7 行对 grok 有一条特有的、更凶的触发路径。**
grok 拿到一个它本地不认识的会话 id 时**不报错**：它去远端恢复，
于是拉起交互式 OAuth 设备码流程，打印一个 URL 和验证码，然后
`Waiting for authorization...` —— 无限等下去（实测 4 分钟只吃 0.6 秒 CPU）。
这条路还**绕开 CPA 直连 x.ai**。

含义有三层，都要落进实现：

1. 对 grok 而言，「会话 id 失效」**永远**表现为第 7 行（超时），**从不**表现为第 5 行
   （provider 自报失败）。指望它报错的代码路径是死代码。
2. 超时不是兜底，是**主路径**。`Spawner` 的 deadline 对 grok 是功能性必需，不是保险。
3. D 节那条「未经输出确认的 id 一律作废、绝不拿去 resume」的纪律，
   理由从「少一个类型、少一处歧义」升级成「防死锁」。
   一个从面包屑里读回来的陈旧 id，在 grok 上的代价不是一次冷启动，
   而是一个挂死的进程 —— 这也是 F 节「面包屑写但不读回」那条裁决的第三个理由，
   起草时还不知道。

### 哨兵错误一览

调用方一律用 `errors.Is` 判断，实现必须用 `%w` 包装，不要让调用方匹配错误字符串。
完整签名在 `09a`，这里点名它们各自对应什么状况、以及给用户的下一步是什么：

| 哨兵 | 状况 | 用户的下一步 |
|---|---|---|
| `provider.ErrUnknownProvider` | agent 在配置里，但它写的 provider **没有注册适配器**（典型：codex） | 换一个 provider，或等后续步骤补适配器 |
| `turn.ErrAgentUnknown` | **配置里根本没有这个 agent id** | `/agents` 看有哪些 |
| `turn.ErrTierNotInStep6` | 该 agent 配了 `workspace` 或 `full` 档 | Step 6 只支持 `read`，改配置或等后续步骤 |
| `turn.ErrNoDelta` | 游标之后没有新内容可送 | 先说点什么再 `/ask` |
| `turn.ErrIdentityDrift` | 身份或脑子在会话开着的时候被改了 | 这是有标价的重建，Step 6 拒绝，由人决定 |
| `turn.ErrPrefixRewrite` | 这一批会改写已经送出去的历史 | 编排层的 bug，不该发生 |
| `provider.ErrPromptTooLong` / `ErrNoWorkDir` / `ErrUnsupportedTier` | 起进程**之前**就能判定的拒绝 | 各自具名，见 `09a` |
| `provider.ErrTurnFailed` / `ErrUnparseableOutput` / `ErrNoSessionID` | 起进程**之后**的三类失败 | 见上面那张结局表 |
| `conversation.ErrMissingSenderBrain` | 适配器漏填了 `SenderProvider` 或 `SenderModel` | 这是适配器的 bug，响亮地失败 |

**前两个必须分开**，它们给用户的下一步完全不同：一个是「这个名字不存在」，
一个是「名字存在但这家 CLI 还接不上」。合成一个的话，用户会去改 agent id，
而问题其实在 provider 那一栏。

`09a` 里另有几个纯契约层的符号（`IdentityPolicy`、`EffortPolicy.String()`、
`TranslatePermission`），它们是实现细节、不对用户暴露，本规格不点名，以 `09a` 为准。

### claude 的成功判据是五个字段，不是退出码

`is_error == false && subtype == "success" && terminal_reason == "completed" &&
api_error_status == null && stop_reason ∈ {"end_turn","max_tokens"}`。

API 过载时进程 exit 0、JSON 完全合法，而 `result` 字段里装着一串错误文本。只判退出码 +
可解析，就会让一条 `"API Error: ..."` 带着像模像样的 `sender_provider` / `sender_model`
落进只追加的事实源；下一轮 Router 把它当成这个成员的观点发给别人，另一个 agent 认真回应
「看来出现了 API 错误」，Judge 把它写进结论，而 `total_cost_usd` 照涨、cursor 照推进、
全链路无一处报错。

grok：`stopReason == "end_turn"`（或 `"max_tokens"` 类截断信号，待 A 节探针 3 确认）。
agy：`status == "SUCCESS"`（大写）。

**三家失败时的输出形状已实测**（2026-09-21，A 节「探针 1 闭合后得到了什么」）。
第 5 行的判据不再是从成功样本反推的，改标 `[已验证]`，但只覆盖「请求发出前被拒」这一类；
「provider 已吃下 delta 之后才失败」那一类仍然零观测。`ParseTurn` 继续一律 fail-closed：
不是已知的成功形状，就是失败——这条纪律恰恰被那份样本救了一次，它的 `subtype` 是
`"success"`，任何「不是已知失败就当成功」的写法都会把 404 的错误正文当回答收下。

### 为什么第 4 行推游标而第 5-7 行不推

`Ingested()` 的语义是「**有证据表明 provider 已经把这批 delta 吃进它的会话**」，不是
「本轮成功」。这是对 `04` §6「成功返回才推进」的一处有意收窄：

- **第 2、3、4 行：证据确凿，两位评委都同意。** 截断正文与空正文都是**成功**的调用，delta
  确实已经被 provider 吃下去了；模型已经读过这批内容了，重发一遍等于往那条只追加的流里塞
  重复内容，既烧缓存前缀，又造成一次看不见的污染。重发的代价高于留一条截断或空的正文。
- **第 4 行尤其要说清**：provider 吃下了、钱花了、会话已经多了一轮，只是本地记不上。
  `Post` 被拒**不得重试**——重试每次都能拿到回答、每次都写不进去，只是重复烧钱。
  会话状态照常推进（否则下一轮重发同一批 delta，与 CLI 侧的历史永久错位）。
  **正文绝不无声蒸发**：整条进 `Record.DroppedText`、打到屏幕上，并追加一条
  `post_rejected` 面包屑。
- **第 5 行：没有证据，而且方向未知，这是本设计里最危险的一条判据。** 我们不知道 CLI 在
  API 调用失败之前有没有把用户那一轮写进会话文件。判「是」而错了，delta 在只追加的流里
  **永久丢失**；判「否」而错了，代价是一次**看得见的**重复。**选「否」（重发）**——但这是
  在**零观测**（七份探针全是成功轮）的情况下立的判据，是本步骤置信度最低的一条决定。
  **`[未验证]`，标为 A 节探针 1 是前置条件而非可选项，是本节唯一的强制性探针义务**。
- **第 7 行：同样没有证据，而且很可能已经发生了。** Windows 上被 `exec.CommandContext`
  杀掉的往往只是 `claude.cmd` / `grok.cmd` 那层垫片，真正干活的 `node.exe` 是它的子进程，
  会活着把这一轮做完、写进 CLI 自己的会话文件。缓解分两层：`Spawner` 用
  `taskkill /T /F /PID` 杀进程树（尽力而为）；并且置 `MaybeAdvanced = true`，进 Meta
  与屏幕。仍判 `Ingested()==false` 重发，理由同第 5 行——丢 delta 比重复更糟——但重复是
  **有记录的**，不是「agent 说如前所述而事实源里查无此事」。

### 一条独立的规则：TTL 过期不是失败，也不是重建

距 `LastTurnAt` 超过 `CacheTTL`（1 小时）时，Runner 在 spawn **之前**置
`ColdExpected = true` 并在屏幕上说一句「这一轮会按冷启动计价」。**不改会话 id、不重建、
什么都不改。**

缓存过期的是服务端的前缀缓存，**不是 CLI 那边的会话文件**——会话照样 `--resume` 得上，
只是要重付入场费。**`CacheTTL` 绝不进入「是否重建会话」的判据，只作为下一轮的价格预报。**
把两件事混为一谈会导致一个方向的昂贵错误（用 0.955 美元换来一个失忆的成员，严格地比什么都
不做更糟，用户看到的现象是「午休回来大家都不记得上午讨论了什么」）；反方向的天真做法同样
有害（为保住缓存每 55 分钟发一轮空转 prompt，既烧钱又往只追加的事实源里塞垃圾）。**两条都
不做。**

### 验收标尺只用 T3，T2 不当模型

实现完之后跑一遍 claude 的三轮，看 `usage.cache_creation_input_tokens`。**标尺是第三轮：
它必须是 O(95) 而不是 O(47749)。** 若第三轮的 cache_creation 逼近 47749，说明某个 flag
或身份块每轮都在改前缀，当场回退。这条验收把 D-4 从「我们相信前缀是稳的」变成
「我们量过，标尺在这」，而且不花一分钱买新数据，直接拿 Step 0 已经付过的账当资产。

**T2 = 19,363 不当因果模型**：探针的 T2 在 `input_tokens=2`、没有任何身份 flag 的纯
`--resume` 下仍然建了 19,363 的缓存，这个数现有的任何解释都圆不上，它是一次**未经解释的
首次 resume 跳变**，记作观测，不记作模型。要把它写进文档当因果，得先补一轮探针——本步
没有做这件事，所以这份规格里 T2 只作为一个已知但不解释的数字出现，不作为任何判据的依据。
标尺是 T3 的 95，不是 T2。

### 重试的边界

**Step 6 没有任何自动重试循环。** `/ask` 是人手动敲的，失败就是失败，屏幕上说清是哪一类，
下一次敲同一条命令会重发同一批（因为游标没推）。失败不推游标 + messages 只追加，所以同一个
seq 区间第二次渲染必然得到同一串字节——`04` §6「重发必须是同一批内容、同样的顺序」是
结构性成立的，不靠自觉。

## H. Runner 主流程

`Ask(ctx, convID, agentID) (Record, error)` 让 `agentID` 在 `convID` 里说一轮话，
返回这一轮的完整记账。**即使返回 error，Record 也有意义**：`Outcome`、`Usage`、
`ProviderSession`、`MaybeAdvanced` 都已填好，调用方据此决定屏幕上说什么、要不要重发。

顺序固定，没有任何按 provider 或按「第几轮」的分支：

1. 查配置与适配器；翻译权限档（Step 6 只放行 read）；算身份指纹，与本会话存的不一致就
   `ErrIdentityDrift` 并点名哪一项变了。
2. `st, ok := r.sess[key]; fresh := !ok`——冷启动的四件事全从这一句流出来。
3. 读 `seq > st.cursor` 的消息，定下 `ScannedTo`，过滤、渲染成 prompt、量字节。前置断言：
   批次里最小的 seq 必须严格大于 `st.cursor`，否则 `ErrPrefixRewrite`，**不起进程**
   （`04` §0 规则 1）。一条都不剩时不起进程，回 `ErrNoDelta`，**游标照常推到 `ScannedTo`**。
4. fresh 时先发 `session_opening` 面包屑（在起进程**之前**：进程一起，冷启动的钱就花了，
   哪怕这一轮后来失败——所以「已尝试建会话」才是要记的事实）。
5. `BuildTurn` -> spawn（带超时，ctx 一路传下去，Ctrl+C 能杀掉子进程树）。
6. 判结局（G 节那张表）：不落正文的结局一律不推游标、不保存会话 id、不推基线（并置
   `baselineSuspect`），发一条 `turn_failed` 面包屑。
7. 成功 -> 比对回传 id 与送出去的 id（含 `Command.PlannedSessionID`）：不等即
   `Drifted`，**基线归零**。然后算差值、判冷暖、`Post` 一条 `KindAgent` 消息、推进会话状态。
8. `Post` 被拒 -> `Outcome` 置 `OutcomePostRejected`：**会话状态照常推进**（provider 确实
   吃下了这批 delta），正文进 `Record.DroppedText` 并打到屏幕上，追加一条 `post_rejected`
   面包屑，**不重试**。

`RenderDelta` 是这条流程里唯一的纯函数边界（05 之前段落已描述其过滤规则与 `skipSelf`
的推导方式，见 D 节「判定点二」）。整条 `Ask` 由一把全局锁（`r.mu`）从头到尾持有，于是
`04` §5「目标 agent 正在跑时不并发调用同一会话」自动成立——两个进程不可能同时 `--resume`
同一个 id 把缓存前缀打叉。代价是不同 agent 也被串行化，Step 9 要扇出时换成按 `sessionKey`
的锁（见 L 节 #17）。

## I. REPL 与既有包的改动

### conversation 包：system 消息与「按 kind 立规矩」

`ReservedSenderSystem` 裁定未决 #14 的后一半：它与 `ReservedSenderUser` 同类——不是
participant，不需要 join，不出现在成员列表里，三个脑子列必须为空，`Post` 对它跳过
`RequireActiveParticipant`。为什么不拿「那个 agent 的 id」当 sender：一条「你的会话开了」
不是那个 agent 说的话，是 Council 替它记的账；挂在它名下，`/log` 里会显示成它说的，
Step 7 的 Router 还会把它当成那个成员的观点转给别人。更要命的是：非保留 id 要过事务内的
`RequireActiveParticipant`，而这类事件恰恰常常发生在刚 `/leave` 或正被重配的成员身上，
那样 08a 明令必须留下的那条记录一条都留不下。实测已核过 `internal/store/schema.go` 的
`CHECK` 只约束 `'user'`，`sender_id` 也没有外键，`kind` 的 `CHECK` 含 `'system'`——
所以 `sender_id='system'` 带三个空脑子列完全合法，**本步不需要改 schema**。配套把
`"system"` 也加进 `internal/config/validate.go` 的保留 agent id。

`ErrMissingSenderBrain` 落实 08a 的 MUST：`KindAgent` 消息必须带 `SenderProvider` 与
`SenderModel`（`SenderEffort` 仍可空）。它要挡的洞是：Step 5 两层校验拦的都是
`sender_id`，反方向一条都没有，适配器漏填三列照样落库，而渲染侧把三列全空渲染成「不适用」
的占位符——漏填于是落在一个代码自己认定为合法的显示上，不报错、测试全绿，而 Canonical
Conversation 只追加，这批消息的归属此后无法回填。这是**按 `Kind` 立的第一条规矩**，
与现有那条按 `SenderID` 的并存，`Service.Post` 的签名不变。

### repl.New：一处明文的修订请求

`repl.New` 目前的冻结签名是 `func New(cfg *config.Config, svc *conversation.Service,
in io.Reader, out io.Writer) *REPL`。Step 6 需要在这个签名里加一个 `rn *turn.Runner`
参数，因为 `/ask` 与 `/session` 要经它调用编排层。按 08a 自己写下的规矩「实现者不得改动
本文件里的签名；觉得签名有问题，退回来说，不要自行改」，这条改动**不是**在 09a 里悄悄覆盖
08a，而是作为一条独立、可被单独否决的修订请求列出（见 09a）。`rn` 为 nil 时两条新命令一律
回一句「provider 调用未就绪」，不得 panic，与现有 `svc == nil` 的兜底同风格。

### `/ask <agent-id> [文本]`

带文本时先走 `postUserText`（与 `/say`、裸文本共用同一个函数，人说的话永远以同一形状
落库），再调 `rn.Ask(ctx, r.cur.ID, agentID)`；不带文本时直接调 `Ask`（对已有的、还没
读过的历史发言）。返回的 `Record` 按 G 节的表打印：正文 / 脑子 / seq / 用量 / 冷暖 / 金额 /
耗时，或对应的失败诊断。`Record.System != nil` 时（典型是 fresh 那一轮）额外打一行提示
「新开了一条 provider 会话」。

### `/session`

列出当前会话每个 participant 的 provider 会话状态。这需要 Runner 暴露一个只读查询，
Step 6 的骨架里没有给出这个方法的确切签名，本文件据行为要求把它定成：

```go
type SessionInfo struct {
    ProviderSession string
    LastTurnAt      time.Time
    ColdExpected    bool
}

func (r *Runner) SessionInfo(convID, agentID string) (SessionInfo, bool)
```

`ok` 为 `false` 表示这个 agent 在这条会话里还没开过口（下一轮会是 `Fresh`）。这是一个
纯读方法，除了拿锁读一眼 map 之外不做任何状态变更（见 09a）。

## J. 生命周期变化

`app.Run` 的顺序在 Step 5 的基础上多两步：打开 store、建 service 之后，**为每个已配置的
agent 建 `<data_dir>/workdir/<agent-id>/`**（`os.MkdirAll`，在启动时一次性建完，
`Ask` 期间不再现建目录——TurnRequest.WorkDir 必须非空，且用配置加载时已知的 agent 集合
一次建齐，比每轮现建更可预测、更容易测），再用同一个 `dataDir/workdir` 基目录构造
`turn.NewRunner(cfg, svc, workDir)`，传进 `repl.New`。目录建不出来时按现有「致命错误、
写 `Err`、不进 REPL」的风格处理，与 `store.Open` 失败同一套。

`Runner.Ask` 内部把某个 agent 的 `TurnRequest.WorkDir` 算成
`filepath.Join(r.workDir, agentID)`——因为 `workDir` 这个 Council 自己的干净目录已经在
启动时对所有已配置 agent 建好，`Ask` 不需要、也不应该现场再判断目录存不存在。

## K. 测试矩阵

`go test ./...` 全绿。适配器测试一律吃仓库内 `internal/provider/testdata/` 的七份 JSON
副本，**不得引用 `E:\` 绝对路径**（D 盘机上没有那个盘）。Runner 测试一律用假 `Spawner`
回放那七份 JSON，**不真去调任何 CLI**。

| 组 | 用例 |
|---|---|
| **前置条件** | ~~A 节探针 1 必须先跑完~~ **已满足**（2026-09-21）。三份失败 fixture 就位：`step6-probe/{claude,grok,agy}-badmodel.out`。本组用例必须用这三份真实字节，不得自造失败 JSON。其中 claude 那份必须单独有一条用例钉住「只看 `subtype` 会把它判成成功」这个反例 |
| **解析：claude** | 三份 fixture 各自断言硬数字：t1 `Text="OK"` / `ProviderSession` 不变的会话 id / `InputTokens=47751` / `CacheCreate=47749` / `CacheRead=0` / `CostUSDTotal≈0.9552` / `CostBasis=CumulativeSession`；t2 `InputTokens=47801` / `CacheCreate=19363` / `CacheRead=28436` / `CostUSDTotal≈1.349789`；t3 `CacheCreate=95` / `CacheRead=47752`。`ThinkingTokens` 取自 `usage.output_tokens_details.thinking_tokens`（三轮恒 0） |
| **解析：grok** | t1 `Text="OK"` / `InputTokens=20625` / `CacheRead=0` / `OutputTokens=723` / `ThinkingTokens=722`（取自 `reasoning_tokens`）/ `CostBasis=CostBasisNone` / `CostUSDTotal=nil`；t2 `InputTokens=20858`（=250+20608）/ `CacheRead=20608`；`Thought` 非空且原样带回 |
| **解析：agy** | t1 `Text="OK"`（已 `TrimRight`，不是 `"OK\n"`）/ `InputTokens=18180`（原样，不加 cache_read）/ `CacheRead=0`（取自 `cache_read_tokens` 这个键）/ `ThinkingTokens=54` / **`CacheCreate == nil`** / `Unreported` 含 `"cache_create"`；t2 `InputTokens=20348` / `CacheRead=16282` / `Text="5192"` |
| **累计值对照（钉死两个坑）** | claude t2 `modelUsage[...].cacheCreationInputTokens == 67112 == 47749+19363`、t3 `== 67207 == 67112+95`、`cacheReadInputTokens == 76188 == 28436+47752`——断言这些数**不等于**解析出的 `Usage`；反向断言 grok 的 `modelUsage[...].inputTokens` **等于** `usage.input_tokens`（t1 20625、t2 250）；agy 的 `num_turns` 是累计（1 -> 2）而 claude / grok 恒为 1 |
| **会话 id 的兄弟字段** | claude 三份的 `session_id` 完全相同而 `uuid` 三份各异；grok 两份的 `sessionId` 相同而 `requestId` 各异——断言解析出的 `ProviderSession` 跨轮恒等，且**不等于**任何一份的 `uuid` / `requestId` |
| **解析：负路径** | 成功形状但删掉会话 id -> `ErrNoSessionID`；`is_error=true` / `subtype!="success"` / `api_error_status` 非 null 任一成立 -> `ErrTurnFailed` 且 `Text` **不**出现在返回值里（用 A 节探针 1 的真实样本，不是编造的 fixture）；完全不是 JSON -> `ErrUnparseableOutput`；成功形状但多出若干未知键 -> 必须被忽略、正常解析；`stop_reason="max_tokens"` -> `Completion=Truncated` 且仍是成功；正文空串 -> `Completion=Empty` 且仍是成功；成功形状但全部 token 为 0 -> 记 `Unreported` 并告警 |
| **BuildTurn** | grok：空 session -> argv 含 `--session-id` 且 `PlannedSessionID` 非空且是合法 v4 UUID；非空 -> 含 `--resume` 且 `PlannedSessionID` 为空。claude / agy：空 session -> argv **不含**任何会话 flag 且 `PlannedSessionID` 为空。三家：`WorkDir` 为空 -> `ErrNoWorkDir`；`PermissionWorkspace` / `PermissionFull` -> `ErrUnsupportedTier`；grok 的 argv 里一次都不出现 `--effort`，且 `Dropped` 含这一项 |
| **BuildTurn 的字节稳定性** | 同一个 `TurnRequest` 调两次，`Command.Args` 逐元素相等；两轮 resume 的 argv 除会话 id 那一项外逐字节相等 |
| **registry / conformance** | `Register` 重复登记 panic；`Lookup` 未知 id 返回 `ErrUnknownProvider` 且信息里列出候选；`IDs` 字典序稳定；**codex 不在已登记列表里**；对每个已登记 provider 断言 `Capabilities` 的每个枚举字段都不是零值；扫 `internal/turn` 全部源文件断言 `SessionIDPolicy` 一次都不出现 |
| **渲染** | `RenderDelta` 同一批消息两次调用逐字节相等（golden 文件）；跳过 `KindSystem`；`skipSelf` 两种取值各自对；首发失败后追加两条新消息再重发，送出去的字节与首发逐字节相同；扫源码断言 `render.go` 里不出现 `time.Now` |
| **游标** | `ScannedTo` 取本轮扫描到的最大 seq，不是 `Delivered` 的最大值，也不是 `MaxSeq(conv)`（构造 A 飞行中、B 拿到 seq 12、A 的回复拿到 seq 13，断言 A 的游标停在 12 之前）；`ErrNoDelta` 时游标照常推到 `ScannedTo`；批次最小 seq <= 游标 -> `ErrPrefixRewrite` 且没有调用过 Spawner |
| **cost 差值** | t1/t2/t3 依次喂进同一条会话，差值分别约 0.9552 / 0.394589 / 0.014058（浮点比较用容差）；grok / agy 的差值是 `nil` 不是 0；`Drifted` 时基线归零、下一轮差值为正而非负；`baselineSuspect` 时差值为 nil；累计值回退时差值为 nil 且绝不为负、也绝不为 0 |
| **会话状态** | 首轮 `Fresh=true`；spawn 失败 / 解析失败 / provider 自报失败后，内存里没有这条会话的 id，下一轮仍是 `Fresh`；grok 连续两次首轮失败，两次的 `PlannedSessionID` 不相同；回传 id 与送出去的不同 -> `Drifted=true`；`(convA, X)` 与 `(convB, X)` 是两条互不相干的会话 |
| **身份指纹** | 会话建立后改 `model` -> `ErrIdentityDrift`；改 `role` -> 同样；改 `AgentConfig.Effort` 时，`EffortPerTurnFlag` 的 provider 漂移而 `EffortProcessExternalConfig` 的不漂移；`ObservedModel` 变化不触发漂移 |
| **TTL** | `lastTurnAt` 推到 61 分钟前 -> `ColdExpected=true`，但 `ProviderSession` 不变、`Fresh=false`、argv 里仍是 `--resume`；`ColdExpected=false` 而实测 `CacheRead=0` -> `UnexpectedCold=true` |
| **结局分类** | G 节那张表逐行一个用例，各自断言四件事：落不落正文、推不推游标、推不推会话状态、发哪一条面包屑。重点两行：`Post` 被拒时游标与会话状态照常推进、`DroppedText` 非空；超时时 `MaybeAdvanced=true` 且不推游标 |
| **system 消息** | `ReservedSenderSystem` 带三个空脑子列能落库（不改 schema）；带脑子时 `Post` 返回 `ErrInvalidSender`；对一个已 `/leave` 的成员发面包屑仍然成功；`Meta["event"]` 与 `Meta["about_agent"]` 都在 |
| **发言归属** | `KindAgent` 缺 `SenderProvider` 或 `SenderModel` -> `Post` 返回 `ErrMissingSenderBrain`；`SenderEffort` 为空合法；grok 的消息 `sender_effort` 记的是请求值且 `Meta["effort_policy"]="process_external_config"`、`Meta["effort_flag_omitted"]=true`；`sender_model` 记请求值而 `Meta["observed_model"]` 记实测标签；agy 不写 `observed_model` 这个键；`modelUsage` 多于一个键时 `Meta["model_source"]="ambiguous"` |
| **子进程契约** | `Command.Dir` 非空且不等于进程当前工作目录；`Command.Env` 为 nil；扫 `spawn.go` 源码断言不出现 `os.Stdin`；stdout 与 stderr 分别回来、不合并 |
| **prompt 上限** | 构造一个超 `MaxPromptBytes` 的批次 -> `ErrPromptTooLong`，且没有调用过 Spawner；`ProviderConfig.Command` 是带空格的路径时，`Command.Path` 是整串、`Args[0]` 不是半截路径 |
| **repl** | `/ask` 不带文本、带文本两条路径各一个输出断言；`rn == nil` 时回「provider 调用未就绪」而不 panic；`/ask` 带文本时人说的那句话走的是与 `/say`、裸文本同一个函数；没有当前会话时的提示；`/session` 的输出含会话 id、`last_turn_at`、下一轮冷暖 |
| **config** | `id == "system"` 的 agent 被校验拒绝；`EffortProcessExternalConfig` 的 provider 配了非空 effort 时 `Err` 里有一行警告且加载仍然成功 |

共 20 组（含前置条件那一组）。

### 一条不花钱的人工验收（不进 `go test`，但必须跑一次）

见 G 节「验收标尺只用 T3，T2 不当模型」。跑一遍 claude 的三轮，`cache_creation_input_tokens`
的第三轮必须是 O(95)；逼近 47749 就当场回退。T2 = 19,363 只作为已知但未解释的观测记录，
不作为任何自动化断言的依据。

## L. 不在 Step 6 做

下面每一条要么**改 schema**、要么**超出「让一个 agent 开口」**，全部留给 Step 7 及以后。

### 一、必须改 schema 才能做的

1. **`provider_sessions` 加一列 `cost_total_usd REAL`。** claude 的 `total_cost_usd`
   是会话累计值，算差值要有上一轮的基线。Step 6 的基线在内存里，跟会话同生共死所以自洽；
   Step 7 一旦把 `provider_sessions` 持久化，基线也必须跟着持久化，否则重启后第一次差值
   会把整个会话的累计额当成单轮成本记上去。**这是最想要的那一列**。不加这一列的替代方案
   （`基线 = SUM(turn_usage.cost_usd) WHERE ... AND seq > 锚点seq`，锚点取最近一条
   `session_opening` system 消息的 seq）能用，但把一次列读做成了两次查询加一条约定，
   而且失败轮不写行会让 SUM 少算一笔。
2. **`turn_usage` 加一列 `provider_session_id TEXT`。** cost 的累计作用域是 provider
   会话，不是 `(conv_id, agent_id)`。没有这一列，「这一轮属于哪个会话」在 SQL 里不可查，
   第 1 条的锚点方案是唯一出路；有了它，`SUM(...) WHERE provider_session_id = ?` 一句话。
   它与第 1 条是一对，要一起加。
3. **`turn_usage` 的用量列要能表达「没报」。** 现在七个数值列全是
   `INTEGER NOT NULL DEFAULT 0`。agy 完全没有 `cache_creation` 这一项、grok 的
   `cache_creation_input_tokens` 在 t1 与 t2 都是 0 却明明读了 20608 的缓存——存 0 会被
   读成「这一轮没往缓存里写东西」，与 `05` §3「可空表示这家 provider 根本没有这个数，
   不是 0」直接冲突。Step 6 的兜底是写进 `messages.meta["usage_unreported"]`，数据不丢，
   只是查不动。
4. **未决 #11（三个脑子列用空串还是 NULL）。** Step 6 给出了它等的那个证据：三家的输出里
   一个 effort 字段都没有，所以 `sender_effort` 永远只可能是请求值，NULL 与空串都填不出
   实测值来。**本步的裁决是维持 `TEXT NOT NULL DEFAULT ''` 不动**：「没有 effort 概念」
   与「适配器漏填了」的区分不靠列的可空性表达，靠 `meta.effort_policy` 与
   `meta.effort_source` 这两个机器可读的键，同时 `ErrMissingSenderBrain` 从源头堵住
   provider/model 的漏填。这条未决可以关掉，理由要写进 `00_DECISIONS.md`。
5. **`turn_usage` 需要一个不依赖 messages 的挂靠点。** 主键是 `(conv_id, agent_id, seq)`，
   seq 只有 messages 产得出，所以一轮花了钱但没产生发言时无处可记。Step 6 的对策是给失败轮
   造一条 `turn_failed` 的 system 消息当挂靠点——能用，但它把记账需求塞进了对话记录。
   干净的做法是主键换成自己的 id、seq 改可空，那是 schema 改动。同类的还有
   `stop_reason` / `completion` / `provider_turn_id` / `identity_fingerprint` /
   `high_water_seq` / `rebuild_count`，现在全只能进 meta。这六条其实是同一条的碎片：
   **要不要建一张 `session_events` 表**，让 Canonical Conversation 不再兼任事件日志。

### 二、Step 7 本来就要做的（Step 6 只把料备齐）

6. **`Tx` 加 `RecordUsage` 与 `SetProviderSession`**，把 `turn.Record` 的四组值写进
   messages + cursors + turn_usage + provider_sessions 同一个事务。Step 6 交出的
   `Record` 已经一次拿齐，不用回头问 provider。绕过 `Service` 直插时 sender 那组校验没有
   对应哨兵，这个新入口得先想清楚错误怎么分类。
7. **游标持久化，且必须与 provider session 在同一个事务里提交或一起作废。** Step 6 特意
   不写 cursors，理由见 F 节末（游标落库、会话在内存，会造出带着已读账本走进一条空会话的
   agent）。Step 7 把两者一起持久化时，这条不变量要写进代码而不是注释：
   `providerSession` / `cursor` / `costTotal` / `lastTurnAt` / `identityFP` 必须一起
   诞生、一起推进、一起作废，读回来时缺一即全部作废重来。
8. **面包屑读回。** Step 6 写 meta 但不读回，理由与代价见 F 节末尾专段——读回需要一次
   有效性校验，而 Step 6 没有手段做这个校验；每个过期 id 每个 agent 约 0.955 美元的探针
   成本是不做的直接代价。Step 7 有了 `provider_sessions` 表的 `cold` 与 `last_turn_at`
   之后，读回才安全。
9. **真正的 Context Router。** `04` §2 的确定性过滤、§3 `thought` 的路由开关（Step 6
   无条件存、无条件不路由）、跨 agent 的结论摘要。Step 6 的挑块规则是「游标之后的全部消息」
   这一条直线，接口按 Router 的形状留好。
10. **Bootstrap 包（`04` §4），这一条必须单独点名。** Step 6 对一个新 `/join` 的成员、
    对每次重启的成员，做法都是**把整场对话重新渲染一遍全量重放**——这**直接违反**
    `04` §4「不要把整个对话倒给它」。Step 6 的 prompt 传输全是 argv、Windows 命令行上限
    约 32767 字符，一场贴过几段需求的讨论重启后，那个成员会以 `ErrPromptTooLong` 失败，
    **在 Step 7 把 stdin / `--prompt-file` 做出来之前就再也开不了口**。不是「慢一点」
    「贵一点」，是坏的。这是 Step 6 最难看的一处，写在这里而不是藏起来。
11. **prompt 传输**：stdin 或 `--prompt-file`，解掉 argv 的 32767 上限。
    `Capabilities.Prompt` 已经把这个维度声明出来了，改的时候只动适配器。前提是 A 节
    探针 2 先测出三家在 `-p` 模式下读不读 stdin。

### 三、更靠后的

12. **codex 适配器**：Step 0 零观测，`exec resume` 的语义与另三家不同。不照文档猜，
    等一次像 Step 0 那样的探针。
13. **指令环境隔离（D-5 / `06` §4）**：`CleanInstructionEnv` 三家全是 `SupportUnknown`。
    每个 agent 照旧读着用户为主代理写的全局 `CLAUDE.md`，这既是每轮都在付的 token，也是
    `06` 点名的信息面问题。改 HOME 或各 CLI 的配置目录环境变量能挡住规则文件，但三家的
    登录态都在同一棵目录下，改了之后全部 agent 同时变成未登录。**必须先做隔离测试，
    不该夹在 Step 6 里顺手做。** Step 6 拿到的便宜那一半（固定一个干净的 cwd）已经做了。
14. **model / effort 热切与随之的会话重建**：Step 6 不支持切，身份指纹不符时是拒绝而不是
    重建，所以 08a 要求的那条「切脑子时追加一条 system 消息」在 Step 6 触发不了。但消息的
    形状（`ReservedSenderSystem` + `Meta["event"]` + `Meta["about_agent"]`）已经在这一步
    定死，Step 8 接上即可。
15. **岔路二的 (c)（读 `~/.grok/config.toml`）**：形态必须是「只读、只取 effort、默认关闭、
    用户显式开、解析失败静默回落到 (a) 并记 warning」，且要先有一次实测证明 CPA 不再压档。
    **前提是先解决「那个文件里有凭据」这一条**——在 `06` §1 改口之前，这条路是封的。
16. **WORKSPACE / FULL 权限档与 git worktree（`06` §3）**：Step 6 只放行 read，另两档在
    起进程之前就拒绝。Step 8 做任务执行时一起做。
17. **并发扇出**：现在是一把全局锁，不同 agent 也被串行化。Step 9 要让五个人同时说话时
    换成按 `sessionKey` 的锁。**已于 Step 9 片 1 落地**（`sessionGate` + `gatesMu`,
    `SessionInfo` / `Rebuild` 改 `TryLock`）。

    **订正（2026-09-22，片 1 落地时）**：本条原文的后半句
    「那时 `Capabilities.SessionID` 的 `Preassignable` 才真正有用（grok 可以先发 id
    再排队）」**撤回**。按 key 的闸下「排队」就是锁等待 —— 第二个调用方阻塞在
    `g.mu.Lock()` 上，此时它连 `BuildTurn` 都还没调，没有任何需要预先知道会话 id 的
    动作。`Preassignable` 解决的是另一件事：**首轮之前就想知道 id**（例如把 id 写进
    面包屑再起进程）。那件事与并发无关，拆锁前后都一样。
    留着这句话，下一个人会去做一件不需要做的事。
18. **未决 #1（同 provider 多 session 是否共享缓存前缀）**：Step 6 第一次真的会先后跑
    两个 claude agent，那时顺手就能量到这个数，但量它不是 Step 6 的交付项。
19. **未决 #8（同一个 `data_dir` 上的第二个 council 进程）与 `-race` 实测**（本机无
    gcc）：Step 6 的并发全部是单进程内的 goroutine，与 D-10 的实测边界一致。
20. **一条 NDJSON 诊断 journal**（把每轮的 `Record` 落进 `<data_dir>/logs/`）。Step 6
    不做：`Record` 直接返回给 REPL、打到屏幕上，已经满足「付过钱的正文不许无声蒸发」与
    `04` §7 的可观测性。要做成可事后查询的东西是一个独立的小工程，且与第 6 条功能重叠，
    应当等那条落地之后再判断还缺什么。
