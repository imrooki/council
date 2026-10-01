# Step 7 规格：持久化一轮、Context Router、Bootstrap

> 前置：Step 6 已完成（`09` / `09a`），三家 CLI 真机跑通。
> 本步的义务清单由 `09` 的 L 节列出，本文件逐条落实并补上 Step 6 之后新发现的约束。

## 0. 这一步在解决什么

Step 6 让一个 agent 能开口，代价是**三样东西只活在进程内存里**：provider 会话 id、
游标、cost 基线。重启即全丢。同时它的挑块规则是一条直线「游标之后的全部消息」，
对一个新 `/join` 的成员就是**把整场对话重放一遍** —— 这直接违反 `04` §4。

本步做三件事，按依赖顺序：

1. **把一轮记进库**：messages + cursors + turn_usage + provider_sessions 四张表一个事务。
   `turn.Record` 已经一次拿齐所有值，不用回头问 provider。
2. **持久化会话状态**，并守住「五样东西同生共死」这条不变量。
3. **真正的 Context Router**：按收件人挑内容，以及新成员的 bootstrap 包。

第 3 件最危险，因为它与 Step 6 用命换来的那条纪律正面冲突。

## 1. 首要约束：Router 必须是纯函数

Step 6 的 `RenderDelta` 是纯函数，函数体里不得含 `time.Now`、游标数字、重试次数、
本轮序号，也不得依赖 map 迭代顺序。Step 7 的 Router **继承这条，一个字都不放松**。

理由是 D-4 那个实测量级：打断缓存前缀时**没有任何一层会报错** —— agent 照常回答、
退出码 0、JSON 合法、游标照推，只是每轮贵几十倍（冷启动 47749 token / 0.955 美元，
暖缓存边际 0.014 美元）。Step 6 真机又量了一次，claude 两轮 0.1031 vs 0.0119。

于是 Router 的签名里**不许出现时钟、随机源、或任何「第几次」**。
一批消息 + 一个收件人配置，必须永远渲染出同一串字节。
失败重发时送出去的东西逐字节相同，这是结构保证，不是靠小心。

**推论（必须写进代码而不是注释）**：Router 里任何「最近 N 条」「距今多久」
「已经发过几次」的表述都是违规的。要表达「近期」只能用 seq 区间，不能用时间。

## 2. 裁决：被过滤掉的消息不再回头

Step 6 把游标推到 `ScannedTo`（本轮扫到的最大 seq），**包括被过滤掉的那些**。
Step 7 的 Router 会过滤得多得多，于是这条裁决要显式立：

> **一条消息在某个 agent 的流里被判为「不路由」，就永远不会再送给它。**

替代方案是给每个 agent 维护一个「已投递集合」而不是一个游标，
那样将来某条老消息变得相关时还能补送。**不采纳**，三条理由：

1. 补送一条老消息本身不打断前缀（那仍是追加），但它让「这个 agent 看到的世界」
   不再等于「一段连续的 seq 区间」，可观测性（`04` §7 要求能回答「上一轮送了什么给谁」）
   从一个区间退化成一个集合，`/session` 与调试输出全部要改形状。
2. 「某条老消息变得相关了」这个判断本身需要时钟或轮次，直接违反第 1 节。
3. 一个游标一个整数，一个集合是无界增长的状态，而这个状态要进同一个写事务。

**代价要说清楚**：Router 的过滤规则改了之后，老 agent 不会追溯性地补上它以前
漏掉的内容。新规则只对新消息生效。这是本步接受的、明写的损失。

## 3. 四张表一个事务

`Tx` 加两个方法（`08a` 早已预告），不是重构：

```go
RecordUsage(ctx context.Context, u TurnUsage) error
SetProviderSession(ctx context.Context, s ProviderSession) error
```

一轮的写入顺序固定：追加消息（拿到 seq）-> 记用量 -> 推游标 -> 更新会话。
四步同一个 `WriteTx`，任一步失败整轮回滚。

### 3.1 turn_usage 必须有自己的身份

现在的主键是 `(conv_id, agent_id, seq)`，而 `seq` 只有 messages 产得出。
Step 6 的对策是给失败轮造一条 `turn_failed` 的 system 消息当挂靠点 —— 能用，
但它把**记账需求塞进了对话记录**：哪天有人决定某类失败不再发面包屑，
账就静默少一笔，而且少得看不出来。

**裁决：`turn_usage` 主键换成自己的 `turn_id TEXT`，`seq` 改可空。**

`turn_id` 由 Runner 在一轮开始时铸（v4 UUID，复用 `provider` 包已有的铸法），
放进 `turn.Record`。于是「这一轮」有了一个不依赖任何其它表的身份，
成功轮的 `seq` 指向它产生的那条消息，失败轮 `seq` 为 NULL。
面包屑照发（用户要在 `/log` 里看见失败），但账不再依赖它。

### 3.2 cost 基线必须持久化

`provider_sessions` 加一列 `cost_total_usd REAL`（可空）。

claude 的 `total_cost_usd` 是**会话累计值**，算差值要有上一轮的基线。
Step 6 的基线在内存里，跟会话同生共死所以自洽。一旦持久化会话而不持久化基线，
**重启后第一次差值会把整个会话的累计额当成单轮成本记上去** ——
而且它是个看起来完全合理的数字，没有任何一层会报错。

同时 `turn_usage` 加一列 `provider_session_id TEXT`：cost 的累计作用域是 provider
会话，不是 `(conv_id, agent_id)`。没有它，「这一轮属于哪个会话」在 SQL 里不可查。
这两列是一对，一起加。

### 3.3 用量列要能表达「没报」

现在七个数值列全是 `INTEGER NOT NULL DEFAULT 0`。Step 6 用真实字节证明了这不够：

- agy 的 usage 里**根本没有** `cache_creation` 这一项；
- grok 的 `cache_creation_input_tokens` 恒为 0，而它的 `cache_read` 明明从 0 涨到 20608；
- 而三份**失败**原件里 claude 与 agy 的 usage 五项是**货真价实的 0**。

三种情况在库里现在长得一模一样。改成可空之后，NULL 读作「这家没报这个数」，
0 读作「实测为 0」。Step 6 的兜底 `meta["usage_unreported"]` 保留但降级为交叉校验。

### 3.4 schema v2 与形状指纹

Step 5 给 `Open` 加了形状指纹校验（`migration.shape`，逐表比对 `pragma_table_info`
的 name / type / notnull 与顺序）。本步的迁移必须**自带 v2 的形状声明**，
否则 `Open` 会在迁移成功后立刻以 `ErrSchemaMismatch` 失败。这是好事：
它保证「加了列却忘了声明形状」当场红，而不是留到第一次读写。

`TestSchemaShapeCoversEveryTable` 会堵住「加了表却忘了声明形状」这个二阶漏洞。

## 4. 五样东西同生共死

`providerSession` / `cursor` / `costTotal` / `lastTurnAt` / `identityFP`
必须**一起诞生、一起推进、一起作废**。

Step 6 特意不落游标，理由是：游标落了库而会话在内存，重启后会造出
「带着已读账本走进一条空会话」的 agent，而库里没有任何一层看得出不对。
Step 7 把它们一起持久化，这条不变量从「不写就不会错」变成**必须在代码里强制**：

> 读回时缺任何一样，**五样全部作废**，该 agent 下一轮按 Fresh 处理。

实现上不给这五样各自的 setter，只给一个 `SetProviderSession(s ProviderSession)`
收一个全字段的结构体，读回时也一次读全。**不许出现只更新其中一样的入口。**

`identityFP` 读回后要与当前配置重算的指纹比对，不符则作废（不是拒绝）——
与 Step 6 不同：Step 6 在进程内发现漂移是**拒绝**（因为那说明用户在会话开着的时候
改了配置，静默重建会白烧一次入场费且用户不知情）；重启后发现不符是**作废重建**
（配置本来就允许在两次运行之间改，此时没有「正在开着的会话」可言）。
这两种处置不同，且不能合并成一条，要在代码里是两个分支、各带注释。

**「作废」一律指五样全废，指纹这条也不例外。** 单独点出来是因为这里有一个
只丢钱、不报错的漏：重建意味着 provider 那边开的是一条**新会话**，
而 claude 的 `total_cost_usd` 是**按会话累计**的，新会话的计数器从零开始。
若只作废了会话 id 而把 `costTotal` 留下当基线，下一轮的
`本轮累计 - 旧基线` 会是**负数**，`computeCostDelta` 遇到负数返回 nil ——
于是那一轮的成本被静默记成「不知道」。它不崩、不报错、不产生错数字，
只是让一笔真实花掉的钱从账上消失。所以基线必须跟着一起作废，
让下一轮以 0 为基线，那时「本轮累计」本身就是这一轮的成本。

## 5. 面包屑读回

Step 6 把 `provider_session_id` 写进 `messages.meta` 但**不读回**，理由是读回需要
一次有效性校验，而 Step 6 没有手段做这个校验；每个过期 id 每个 agent 约 0.955 美元的
探针成本是不做的直接代价。

Step 7 有了 `provider_sessions` 表的 `last_turn_at` 之后，读回才安全：

- 读回的 id 必须来自 `provider_sessions` 表，**不是**从 `messages.meta` 里刮；
  meta 里那两个键（`provider_session_id` / `cost_cumulative_usd`）按 `09a` 的注释
  「Step 7 退休」处理 —— 停止写入，已有历史行保留不动。
- 读回后**不做探针验证**。Step 6 的实测给了理由：grok 拿到一个它本地不认识的会话 id
  时**不报错**，它拉起交互式 OAuth 设备码流程然后无限阻塞（4 分钟只吃 0.6 秒 CPU）。
  所以「先探一下这个 id 还在不在」这个动作本身就是那个事故。
  取而代之的是**硬超时 + 失败即作废**：用读回的 id 直接发下一轮，
  超时或 `ErrTurnFailed` 就把五样一起作废、下一轮 Fresh。

## 6. Context Router

### 6.1 MVP 的规则是确定性的

`04` §2 的清单落成代码时，**每一条都要能只看 `conversation.Message` 判出来**，
不许引入 embedding、不许调模型、不许看时钟：

| 应当路由 | Step 7 的判据 |
|---|---|
| 直接 @ 了这个 agent | `Text` 里有 `@<agent-id>`（Step 8 做 @mention 时这条才真正有用） |
| 用户指令 | `Kind == KindUser` |
| 其他 agent 的结论 | `Kind == KindAgent` 且发言者是本会话活跃成员 |

| 不应当路由 | Step 7 的判据 |
|---|---|
| 内部事件与 provider 诊断 | `Kind == KindSystem`（Step 6 已经这么做） |
| 这个 agent 自己说的话 | `SenderID == 自己` 且不是首轮（Step 6 已经这么做） |
| `thought` | 永远不进 delta 正文（见 6.2） |

**Step 7 的 Router 与 Step 6 的过滤差别不大，这是有意的。** 真正的收益在 Step 8
（@mention、任务指派）和 Step 9（Team 分组）之后才出现，现在就引入复杂规则，
等于在没有 @mention 的世界里猜哪些消息「与当前任务相关」。
本步的任务是**把接口做成 Router 的形状**、把纯函数性钉死，规则本身保持简单。

### 6.2 thought 的路由开关

`04` §3 立场是「默认存、默认不路由」，要让某个 role 看到得在 Team 配置里显式打开。
Team 配置是 Step 9 的东西，所以**本步维持无条件不路由**，但要把开关的位置留好：
Router 收一个 `RouteThought bool`，Step 7 恒传 false，Step 9 接上配置。

理由要留在代码里：探针里 grok 的 `thought` 直接复述了用户全局规则文件的内容，
把它路由给另一个 agent 等于让后者读到前者的私下斟酌，而且那段内容里可能有
路径、IP、内部约定（`06` 点名的信息面问题）。

### 6.3 Bootstrap 包

`04` §4：新加入的 agent 没有历史会话，**不要把整个对话倒给它**。

Step 6 的做法正是倒给它 —— 对一个新 `/join` 的成员、对每次重启的成员，
都把整场对话重新渲染一遍全量重放。Step 6 的规格自己把这条写成「最难看的一处」。

**裁决：Bootstrap 包是对既有消息的一次确定性挑选，不是一段生成的摘要。**

不采纳「让模型生成一段摘要当 bootstrap」，理由是第 1 节：摘要不是纯函数，
重发时产出不同的字节，直接打断前缀；要让它稳定就得把摘要本身持久化成一条消息，
那等于往 Canonical Conversation 里写一条没人说过的话。

MVP 的挑选规则（同样只看消息本身）：

1. 会话标题（`conversations.title`）—— 对应 `04` §4 的「原始/当前任务」；
2. 全部 `KindUser` 消息；
3. 每个其他活跃成员在 `[0, ScannedTo]` 这个**闭区间内的最后一条** `KindAgent` 消息
   —— 对应「已形成的结论，不是全过程」。

按 seq 升序渲染，去重。规则简单到可以一眼看出它是纯函数。

**第 3 条的窗口必须写进规则本身，不能只说「最后一条」。** 「最后」是相对于
扫描窗口的：`readAfter` 是分页拉的（`historyPage = 512`），若某个成员的最后一条
落在更早的一页里，「本页的最后一条」与「全量扫描的最后一条」会是两条不同的消息。
纯函数性本身拦不住这个 —— 两次调用只要窗口相同就仍然逐字节相同，
但窗口本身会随分页边界漂移，于是同一个 bootstrap 在不同时刻挑出不同的消息。
所以规则里写死 `[0, ScannedTo]`，并要求 Router 拿到的是**这个区间的全量**
而不是某一页。

**bootstrap 与普通 delta 的关系**：bootstrap 是首轮那一批的内容，
之后游标照常推进，后续轮是普通 delta。所以「首轮 vs 后续轮」这个判定点仍然只有一个
（`ProviderSession` 是不是空串），Step 6 的那条纪律不变。

**规模上限**：claude 已经走 stdin，没有 argv 上限；grok 与 agy 仍在 argv 上
（未决 #15）。所以 bootstrap 要能报出自己的字节数，超过该 provider 的
`MaxPromptBytes` 时返回 `ErrPromptTooLong` 而不是截断 —— 截断一个 bootstrap 包
等于让那个 agent 带着残缺的世界观开始，而它自己不知道。

## 7. 可观测性

`04` §7 要求 Router 能回答「上一轮到底送了什么给谁」。Step 6 的 `Record` 已经有
`Delivered []int64` 与 `PromptBytes`，Step 7 把它们落进库（`turn_usage` 的新列或
`session_events`），并让 `/session` 能显示每个成员的 seq 区间。

**一个已知的锁形状问题，本步只记录不改**：`Runner.Ask` 在**整轮调用期间**持有
`r.mu`（最长 5 分钟的硬超时），而 `SessionInfo` 取的是同一把锁。于是一次 `/ask`
在飞时 `/session` 会被挡住。Step 6 的 REPL 是单线程读循环，观察不到；
本步让 `/session` 显示 seq 区间也不改变这一点，因为数据来自库而不是内存。
真正需要拆锁是 Step 9 扇出的时候（`09` L 节 #17，换成按 `sessionKey` 的锁）。
在那之前，契约里要明写「`SessionInfo` 可能阻塞在一次在飞的 `Ask` 之后」，
免得有人把它当成一个随时可查的只读探针。

**订正（2026-09-22）**：上面这段是 Step 7 当时为真的记录，**已于 Step 9 片 1 兑现**。
`r.mu` 换成了按 `sessionKey` 的闸（`sessionGate`），`SessionInfo` 与 `Rebuild` 改用
`TryLock`：前者拿不到闸就从库读上一次提交的状态并置 `InFlight = true`（`/session`
的 NEXT 列印 `busy`），后者拿不到就返回 `ErrTurnInFlight` 且什么都不做。
做法与完整理由见 `10a` §5 与 `12` §2.2-2.6，不在本文件重复。

**不得在调试输出里包含任何凭据。** 注意 Step 0 的发现：agent 会读到用户全局
`CLAUDE.md` 的内容，而那份文件里可能有路径、IP、内部约定。

## 8. 不在 Step 7 做

1. **agy 在 `--mode plan` 下正文为空**（未决 #17）。它是一次真实调用暴露的，
   `in=16999 out=724 thinking=624` 而 `response` 是空串。要么给 READ 档换一个 agy 模式，
   要么确认 plan 模式下正文该从别的键取。**在查清之前 agy 只算「通路已打通」，
   不算「可用成员」。** 留给 Step 8（那时要做任务执行与权限，正好一起定模式映射）。
2. **grok 与 agy 的 prompt 传输**（未决 #15）。claude 已经走 stdin，另两家还在 argv。
   grok 有 `--prompt-file`，agy 未查。做之前先量 flag 段的真实字节数。
3. **@mention 的真正语义**（Step 8）。本步只把 `@<agent-id>` 当一条路由判据。
4. **Team、Judge、Scheduler**（Step 9）。`RouteThought` 的开关位置留好，不接配置。
5. **codex 适配器**（Step 0 零观测，等一次探针）。
6. **指令环境隔离**（D-5 / `06` §4，`CleanInstructionEnv` 三家全是 `SupportUnknown`）。
   改 HOME 能挡住规则文件，但三家的登录态都在同一棵目录下，改了之后全部 agent
   同时变成未登录。**必须先做隔离测试**，不该夹在本步里顺手做。

## 9. 验收

| 组 | 用例 |
|---|---|
| **三表一事务** | 一轮成功：messages / turn_usage / provider_sessions 各多一行且互相指得上；任一步失败全部回滚，三张表都没动。**`cursors` 表不该多出任何一行** —— 游标已搬进 `provider_sessions.cursor_seq`，两处都写就有了两个事实源（本行原写「四表」，是第 3 节那条裁决之前的措辞，2026-09-21 订正） |
| **身份一致** | `msg` / `u` / `session` 三个入参带的 `(会话, agent)` 对不上时，**事务开始之前**就拒，报 `ErrTurnIdentityMismatch`；保留 sender（user / system）不参与 agent 那一项的比对 |
| **失败轮记账** | 失败轮 `turn_usage` 有行、`seq` 为 NULL、`provider_session_id` 指得上；把面包屑那条 system 消息删掉不影响这一行（证明账不依赖对话记录） |
| **同生共死** | 手工把 `cursors` 的行留下、`provider_sessions` 的删掉，重启后该 agent 必须按 Fresh 处理且**不**带着旧游标；反向同理 |
| **cost 基线** | 跑两轮后重启，第三轮的差值不等于会话累计值；`provider_sessions.cost_total_usd` 与最后一轮的累计值相等 |
| **用量可空** | agy 的 `cache_create` 在库里是 NULL 不是 0；失败轮 claude 的五项是 0 不是 NULL |
| **Router 纯函数** | 同一批消息、同一收件人、调两次逐字节相同；源码扫描 Router 所在文件不出现 `time.Now` / `rand` |
| **过滤不回头** | 一条被判为不路由的消息，之后任何一轮都不再出现在该 agent 的 delta 里 |
| **bootstrap** | 新 `/join` 的成员首轮 delta **不等于**全量历史；含会话标题、含全部 user 消息、每个其他成员只含最后一条 agent 消息；规则连调两次逐字节相同 |
| **bootstrap 超限** | 构造一个超 `MaxPromptBytes` 的 bootstrap -> `ErrPromptTooLong`，且没有调用过 Spawner，**绝不截断** |
| **thought 不路由** | 任何一轮的 delta 里都不出现任何消息的 `thought` 内容 |
| **schema v2** | 旧 v1 库打开时迁移到 v2 并通过形状指纹；`TestSchemaShapeCoversEveryTable` 覆盖新表 |
