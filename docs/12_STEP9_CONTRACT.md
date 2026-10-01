# Step 9 冻结契约：Team、Scheduler、Judge

日期：2026-09-21 · 机器：E 盘机 · 核对基线：`10332ec`

本文件是实现方与测试方**共同的唯一权威**。改本文件要留记录。

> **订正（2026-09-29，CH1 `4d3dc0b`）**：本文与 `round.go` 头注释里的「名单为空则拒绝，**绝不默认全员**」，**对裸文本已被 #47 推翻**：对话模式下，直接打字、没点名就问当前会话全部活跃成员（去掉总结成员），见 `19a_CH1_CONVERSATION_MODE_CONTRACT.md`。`/round` 自身不变：手敲的 `/round` 名单为空照旧拒绝。

本步分三片，**顺序写死**：片 1（拆锁）-> 片 2（Team）-> 片 3（Scheduler）。
设计提案说片 1 与片 2「谁先腾出手谁先走」，那在文本上不成立 ——
两片都要重写 `Ask` 的同一段（片 1 改开头的锁与规则快照，片 2 改中段的
`NewRouter(false)` 与早期拒绝），谁后走谁要在一个被大改过的 `Ask` 上重做。
这里定死顺序，不留给「谁先腾出手」。

**片 2 是唯一 `isMinimalUseful` 的一片**，但它排在片 1 之后。

---

## 0. 本步改动的包

| 片 | 包 | 改什么 |
|---|---|---|
| 1 | `internal/turn` | 新文件 `gate.go`；`runner.go` 的 `r.mu` 换成按 sessionKey 的闸；`AskUpTo`；`SessionInfo.InFlight`；`ErrTurnInFlight`；`rulesMu` |
| 1 | `internal/repl` | `/session` 的 NEXT 列多一个 `busy` 取值 |
| 1 | docs | `10a` §5、`09` L 节 #17 两处订正 |
| 2 | `internal/config` | `TeamConfig.ThoughtVisibleTo`；`validate.go` 的 teams 循环加校验 |
| 2 | `internal/store` | schema **v4**：`conversations.team_id`、`turn_usage.identity_fp`、`turn_usage.round_id` |
| 2 | `internal/conversation` | `Conversation.TeamID`；`Tx.SetConversationTeam`；`Service.BindTeam` |
| 2 | `internal/turn` | `thoughtVisible` 纯函数；`ErrTeamUnknown`；`identity_fp` / `round_id` 穿进 `makeTurnUsage` |
| 2 | `internal/repl` | `/team use` |
| 3 | `internal/turn` | 新文件 `scheduler.go` / `budget.go`；`RunRound` |
| 3 | `internal/conversation` + `internal/store` | `UsageSamples` 读路径（**turn_usage 的第一条读路径**） |
| 3 | `internal/repl` | `/round` **升级**（不是新建，见 1.1）、`/judge` |

---

## 1. 先说五条核查，它们改掉了设计提案里的前置

设计合议与对抗复核都跑在 `1aa23f7` 上，**那之前 Step 8 的第一片与第二片都还没落地**。
所以提案与复核里有一批前置已经过期。下面五条是我在 `10332ec` 这棵树上逐条重开核过的。

### 1.1 `/round` 已经在 `11d` 8c-2 里冻结了，本步是**升级**不是新建

`docs/11d_STEP8C_CONTRACT.md` 第 3 节已经把 `/round` 冻结成：
REPL 层、**串行**扇出、默认只打印计划、`--go` 才真跑、名单取「最后一条
`KindUser` 消息里的 mention ∩ 活跃成员」、空名单拒绝。

Step 9 片 3 的提案又写了一遍 `/round`，形状不同（并发、`--yes`、带文本、Budget）。
**这不是两条命令，是同一条命令的两个阶段。** 契约必须逐条说清哪些性质保留、哪些改：

| 性质 | `11d` 8c-2（串行版） | Step 9 片 3（并发版） |
|---|---|---|
| 两步形状 | 默认打印计划，`--go` 才跑 | **保留**，计划里多一张 Budget 表 |
| 执行方式 | 串行 | **改成并发**，全员共用一个 `upTo` |
| 确认 flag | `--go` | **仍是 `--go`**，不引入 `--yes` |
| 名单 | 最后一条 `KindUser` 的 mention ∩ 活跃成员 | **保留**为默认；另加 `/round <id...>` 显式点名与 `/round all` |
| 空名单 | 拒绝，一个进程不起 | **保留** |
| 冷暖预报 | `SessionInfo` 的 `ok == false` 必须算**必冷** | **保留**（这条是 8c-2 的裁决，Budget 直接继承） |
| 取消 | `OutcomeCanceled`，Ctrl-C 腰斩当前轮 | **保留**，但腰斩的是 N 轮（见 6.4） |
| REPL 不响应的时长 | 最长 **N × 5 分钟** | **并发之后降到约 1 × 5 分钟**，提示文本要跟着改 |
| 带文本 | 不带 | **仍然不带**（裁决见 4.6） |

**`--yes` 这个名字不引入。** 一条命令只能有一个「我确认要花钱」的 flag，
两个名字会让 `11d` 与本文各自的用例和帮助文本长期不一致。

### 1.2 `Selector` 会打破 `11d` 1.2 立的那道导入图闸

提案给 `PlanRound` 设计了三态 `Selector`，其中 `SelectMentioned{Text string}`
要在 `internal/turn/scheduler.go` 里解析 mention。

**这与 `11d` 1.2 直接冲突**：那一条把 `ParseMentions` 放进 `internal/repl`，
理由是 `internal/repl` 导入 `internal/turn`，反向导入是一个**编译期的导入环** ——
「agent 正文里的 `@x` 在路由上没有任何语义」这件事因此由 Go 的导入图保证。
`scheduler.go` 一旦要解析 mention，那道闸就得拆掉。

**裁决：`Selector` 收窄成一个已经解析好的 id 列表。**

```go
// requested 为 nil 表示「全员」；非 nil 表示调用方已经点好名。
// 本函数不解析任何文本 —— 「用户点了谁的名」是一个 REPL 的问题
// （输入是人打的字），「给定 Mode 与参与状态谁真的说话」才是本包的问题
// （输入是 participants 与 config）。两者回答的不是同一个问题，
// 这与 11d 第 2.7 节区分提示行与 Router 的谓词是同一条理由。
func PlanRound(members []conversation.Participant,
	agents map[string]config.AgentConfig, requested []string) Plan
```

REPL 侧用 `ParseMentions` 把 `@` 名单解析出来，交进 `requested`。
这样既保住那道闸，又让 `PlanRound` 变成一个**不碰文本**的纯函数，更好测。

### 1.3 gate 的生命周期没定，照直译会把它存在的唯一理由打掉（阻塞）

`runner.go:146-148` 今天：

```go
key := sessionKey{convID: convID, agentID: agentID}
_, memExisted := r.sess[key]
delete(r.sess, key)
```

把 map 的值换成 `*sessionGate` 之后，若 `Rebuild` 仍然 `delete` 这个 key：

1. G1 调 `gate(key)` 拿到 gate A，还没 `Lock`；
2. `Rebuild` 取 `gatesMu`，删掉 key；
3. G2 调 `gate(key)`，创建 gate B。

此后 G1 锁 A、G2 锁 B，**两条 goroutine 同时在同一个 `(conv, agent)` 上跑** ——
正是「同一条 provider 会话绝不并发 `--resume`」这条不变量被拆掉的那一刻，
而且没有任何一层会报错，只表现为下一轮贵几十倍（D-4）。

**裁决，写成本片的显式不变量并进 `gate.go` 的注释**：

> **gate 一经创建，永不从 map 里移除。**

`Rebuild` 改成：`TryLock` 拿到该 key 的 gate 之后，**在持锁状态下把 `g.st = nil`**
（不是删 map 条目），`memExisted` 由 `g.st != nil` 回答。

**gates map 只增不减的内存账要写出来**：每个 `(conv, agent)` 一个指针加一个
mutex 加一个指针，24 字节量级，会话数 × 成员数级别，可接受 —— 但要写出来，
不许当成零成本。

### 1.4 Budget 的「只许高估不许低估」不可满足，而它的验收是空跑的（阻塞）

**(a) 取样规则结构上就偏低。** `capabilities.go:99` 明写
「`turn_usage.input_tokens` 存的是**本轮完整 prompt**」，而 delta 是累积的：
同一条会话里 `input_tokens` 单调上涨（未决 #17 的 agy `16195 -> 32722` 就是提案
自己引的证据）。拿「最近一条同类的行」去预报下一轮，**取的必然是一个比实测小的数**。
扇出把这件事放大 N 倍：一轮之后每个成员的下一批 delta 里多出 **N-1** 条别人的发言，
不是 1 条。

**(b) 验收在 0 行上成立。** 提案的验收是「三家各一个 agent 组一个 team、`/round` 一次」。
那是一个**全新**的 team：三个成员在 `(agent_id, identity_fp)` 上一条样本都没有，
全部印「未标定」、不进合计，于是「预报低于实测的行数必须为 0」在 **0 行**上成立。
这正是本项目反复栽过的那一类（缺了反方向，一次巧合把验收骗过去）。

**裁决**（见 4.5 的完整形状）：

1. **放弃把「只许高估」当硬验收**。改成可核的口径：`Basis` 必须点名取样的那个
   `turn_id`，屏幕上**预报与事后实测并排印**，方向**记录下来**而不是断言下来。
2. **预报按规模缩放，不照抄样本**。`Select` + `Render` 是纯函数、在起进程之前
   就跑得完，所以**这一轮的 `prompt_bytes` 在预报时就知道**。基数用
   `样本.input_tokens / 样本.prompt_bytes × 本轮.prompt_bytes`。
   **`prompt_bytes` 是 `NOT NULL DEFAULT 0`（`schema.go:237`），所以样本里
   `prompt_bytes == 0` 的行不能用来缩放，必须跳过** —— 否则是一次除零。
   这一条提案与复核都没说。
3. **验收分两段，缺一不可**：第一段对一条**已经跑过至少两轮**的会话预报第三轮
   （那才有样本）；第二段才是全新 team 验「未标定」那条路径。
4. **失败轮算不算样本，开工前裁死**：`OutcomePostRejected` 是**钱花了、用量也记了**
   （`record.go` 那段注释写得很清楚），排除它就是往禁止的方向偏。
   口径定成 **「只要 `input_tokens` 非 NULL 就算样本」**。

### 1.5 入场费在本仓库里有两个数字，提案抓错了一个

提案的 evidence 写「`docs/README.md:33-35` —— claude 冷启动今天实测
29537 token / $0.118394」。**README 那里写的是另一个数**：

> 冷启动 47,749 token / $0.955；暖缓存零新上下文边际 $0.014；缓存 TTL 1 小时。

29537 / $0.118394 这个数在 `11_STEP8_SPEC.md` §1.2.1（也在 HANDOFF 里），
是**一条真实轮次**在加了 `--safe-mode --restricted` 之后、用一份很小的夹具量的
（`in=29537 cache_create=29535`）。README 的 47,749 是 Step 0 探针、旧 flag 组合。

**两个都是真的，量的不是同一件事。** 所以：

- **本仓库里没有一个可以直接引用的「入场费常数」**，这恰恰是本步
  「报价只能来自这台机器上这个身份自己跑过的行」那条裁决的**最强证据** ——
  提案在引一个常数的同时裁决不许用常数，自相矛盾。
- README 那三行要加一句限定，写明它是**哪一组 flag 下**量的，
  以及 `11` §1.2.1 有一个在当前 flag 组合下的、更小夹具的数。
  **本步片 1 落地时顺手改 README**，因为下一个人（包括模型）会照抄它当常数 ——
  提案已经照抄错了一次。

---

## 2. 片 1：拆锁 + AskUpTo 快照

`isMinimalUseful: false`。没有任何用户可见的新功能，但它是片 3 的前置。
全在 `internal/turn`（加 `internal/repl` 一处显示、两处文档订正）。

### 2.1 先把「为什么现在做」说诚实

提案给片 1 三个交付物（`SessionInfo.InFlight`、`Rebuild` 的 `ErrTurnInFlight`、
rules 的竞态）写的理由是「一次 `/ask` 在飞时 `/session` 会被挡住」「一轮跑到一半被
`/rules reload --rebuild` 换掉」。**这两个场景在今天的 REPL 下一个都发生不了**：
命令派发是单 goroutine 串行的（`11d` 1.3 核过，`runner.go:71-74` 的注释自己也这么写）。

**代码是对的、是前瞻的，理由是编的。** 契约里的理由写成这一条：

> 这三处的锁形状在单线程 REPL 下观察不到。本片做它们，是因为 gate 拆开之后
> **结构上**允许并发调用，而结构允许的事情早晚会被某个调用方用上（异步 round
> 是已记的独立一片）。用例**直接并发调这三个方法**来锁住行为，
> 不假装它今天会在屏幕上发生。

`10a` §5 的订正文案照此写，不许写成「扇出之后天天撞上」。

### 2.2 `sessionGate`

```go
// sessionGate 是一条 (会话, 成员) 的串行闸，整轮 Ask 期间持有。
//
// st 只在持有 mu 时读写 —— 把 sessionState 塞进 gate 而不是留在外面的 map 里，
// 是为了让「读写 sessionState 必须持有该 key 的锁」由结构保证，不靠注释。
//
// **不变量：gate 一经创建，永不从 gates map 里移除**（理由见 12 第 1.3 节）。
type sessionGate struct {
	mu sync.Mutex
	st *sessionState
}
```

`Runner` 上 `mu sync.Mutex` + `sess map[sessionKey]*sessionState`
换成 `gatesMu sync.Mutex` 守 `gates map[sessionKey]*sessionGate`，
`gatesMu` 的临界区**只有一次 map 读写**。
`func (r *Runner) gate(key sessionKey) *sessionGate` 做 get-or-create。

**既有断言要跟着改语义，不是机械替换**（提案把这件事写成「机械替换、不改断言」，
那是错的）：

- `runner_rules_contract_test.go` 今天断言「map 里那条没了」-> 改成断言 `gate.st == nil`；
- `runner_reject_session_test.go` 今天断言「map 里有这条」= 有内存会话。
  gate 变成 get-or-create 之后**这条断言会恒真**（空跑）,
  必须改成断言 `st != nil` **且** `providerSession` 非空 ——
  否则就是一条静默失效的用例。

**处数不要照抄任何一份提案**：设计说「6 个文件约 8 处」，复核说「6 个文件 11 处」，
我在 `10332ec` 上数到的是 **7 个文件 13 处**（`grep -rn '\.sess\[' internal/turn/*_test.go`）。
这个数一直在动，**开工时现数，不引用本文的数字**。

**gate 的闸挡不住什么，要在注释里写出来**：
gate 按 `(conv, agent)` 拆，保住的是「同一条 provider 会话不并发 resume」。
但有两样东西是**按 agent 共享**的：READ 档的工作目录不带 convID
（`agentWorkDir`，`11c` §4 的有意裁决），以及 claude 每轮往那个目录里写的
`council-role.txt`。同一个 agent 在两条会话里并发跑，两条 goroutine 会同时
写同一个路径。今天 REPL 只有一条当前会话所以不可达，**本片不修**，
但不许让下一个人误以为闸已经挡住了。

### 2.3 `AskUpTo`

```go
func (r *Runner) AskUpTo(ctx context.Context, convID, agentID string, upTo int64, roundID string) (Record, error)
// Ask(ctx, convID, agentID) 自己铸一个 roundID，再调 AskUpTo(..., 0, 那个 id)。
// upTo = 0 表示不设上界。
```

**`roundID` 必须是参数，不能让 `AskUpTo` 自己铸。** 3.3 给 `turn_usage` 加了
`round_id` 列，6.3 裁的是「每一轮都有 `round_id`，`/ask` 是一轮之一」 ——
那就必须有人在**扇出之前**铸一个 id 并发给全部 N 个成员，
否则五个成员会各铸各的，`/cost` 的按轮聚合（`13` 7.1）拿到五个不同的 id。

- `RunRound` 铸一个，传给每个 `AskUpTo`；
- `Ask`（`/ask` 那条路）自己铸一个，于是那一列**永远有值**,
  不会退化成「只有 `/round` 的行有、`/ask` 的行是空串」的半空列。

铸法照既有的 `TurnID`（同一个工具函数），**不许用时钟或随机源以外的东西
去推导它**，也不许把它塞进任何进 prompt 的地方 —— 它是记账用的，不进前缀。

实现只在 `readAfter` 拉回来之后**按 upTo 在内存里截断**，
不改 `conversation.Store.Messages` 的签名。

`09a` 冻结了 `Ask` 的三参数签名，不改它；新增方法**同样是契约面动作**，
照未决 #9(a) 给 `Service.SetThoughtPolicy` 立的规矩在 `09a` 留订正记录。

**`upTo` 保证的是什么，话要说准**（提案把 `04` §6 当成立论基础，读者会以为
跨轮重发的逐字节相同被解决了，那不成立）：

> `upTo` 保证的是**一轮内的确定性**：同一批 delta 不依赖谁先答完。
> 它**不保证**跨轮重发的逐字节相同 —— 一个 `MaybeAdvanced` 的成员在下一轮重发时，
> `upTo` 变了，那一批里会多出这一轮另外 N-1 个人的发言。
> 后者在只追加的事实源上本来就做不到；`04` §6 在 Council 的实现里落成的是
> 「不改写已投递部分」（`04` §0 规则 1），不是「重发字节全等」。

**用例要钉前者**：同一份夹具跑两次 `RunRound`，假 Spawner 用**不同的返回延迟顺序**，
断言每个成员两次拿到的 prompt 逐字节相同。

### 2.4 `SessionInfo` 改不阻塞

`SessionInfo` 加 `InFlight bool`；实现改用 `gate.mu.TryLock()`,
拿不到就从库读上一次提交的状态并置 `InFlight = true`。
`/session` 的 NEXT 列多一个取值 `busy`（排版不在契约面，`10a` §4）。

**`SessionInfo` 今天在「从没开过口」时返回 `SessionInfo{}, false`**
（`runner.go:539-567`），这条语义不变，`11d` 3.2 的「`ok == false` 算必冷」照旧成立。
`InFlight` 只在 `ok == true` 那一支有意义。

契约面新增 `SessionInfo.InFlight`，要登记。

### 2.5 `Rebuild` 在飞时拒绝

新增哨兵 `ErrTurnInFlight`。`Rebuild` 用 `TryLock`，拿不到就返回包了它的错误。

理由不只是「阻塞 5 分钟不可接受」：**即便等到了也白做** ——
在飞那一轮结束时 `SetProviderSession` 是 upsert，会把刚删的行又写回来，
用户看到「已丢弃」而什么都没丢。拒绝是 fail-closed 的那一边。

验收判据是**读回来的那一行**，不是屏幕输出。

### 2.6 `r.rules` 加读写锁，快照粒度是**轮**不是**次**

提案只说「`Ask`/`AskUpTo` 一进来取一次 snapshot；`SetRules` 取写锁」。**漏了两个读点**，
而漏掉的那两个正是 `/rules reload` 走的路：

- `runner.go:111` `func (r *Runner) Rules() RulesBlocks`（`/rules` 显示用）；
- `drift.go:34` `AuditDrift` 里 `r.auditDriftAgainst(ctx, r.rules)` —— **裸读**。

更关键的是 `drift.go` 的注释说明了 `/rules reload`（不带 `--rebuild`）的实现方式是
「靠 repl 那边**临时调公开的 `SetRules` 换上去、调 `AuditDrift`、再换回来**」 ——
这是一个会把 `r.rules` 来回写两次的序列，任何与它并发的 `Ask` 都可能快照到那份临时规则。

**`rulesMu` 要覆盖全部五个点**：`SetRules`（写锁）、`Rules()`（读锁）、
`AuditDrift` 取快照（读锁）、`Ask`/`AskUpTo` 取快照（读锁）、
`rules_report.go` 的 `probeHeadroom`（读锁）。

**订正（2026-09-22，片 1 开工前 + 落地）**：本段原文写的是「全部四个点」，
漏了第五个 —— `rules_report.go:138` 的 `probeHeadroom` 里
`renderIdentity(agentID, ac, r.rules)` 同样是**裸读**，而它由 `/rules` 命令经
`RulesReport()` 调到，正是与 `SetRules` 来回写那个序列同一条命令路径上的读。
主代理开工前现查源码核出这一处，实现方（grok）独立报了同一处，两边对上。

**一条已知的、本片没修的窄缝**：`RulesReport()` 按 (provider, tier) 逐行调
`probeHeadroom`，每行**各取一次**读锁 —— 快照粒度是「次」不是「一次 `/rules` 输出」。
与 `SetRules` 并发时，同一张表的两行可能算自两份不同的规则文本，
`Rules()` 显示的那一份也可能与表里的行不一致。
它只影响**给人看的显示**，不进指纹、不影响 argv、不花钱，所以片 1 不扩范围；
记在 `00_DECISIONS.md` 里，与 `r.cfg` 那条缺口一起处理。

**快照粒度提到轮**：`RunRound` 进来取一次 rules 快照，与 `upTo` 一起传给每个 `AskUpTo`。
理由与 `upTo` 完全相同 —— 同一轮里两个成员用了两份规则，一个的指纹会与另一个对不上，
而漂移只会在**下一轮**才被发现。

**顺带订正 `drift.go` 那条注释**：`11d` 4.4 已经要求把它的理由改写成
「REPL 单 goroutine 串行派发」。片 1 把它再改一次 —— 拆锁之后那条理由也不成立了，
新理由是「`r.sess` 已不在 `AuditDrift` 的读集里，`r.rules` 由 `rulesMu` 守」。
**两次改同一段注释是有意的**：它每次都必须说当时为真的那条理由。

### 2.7 两处文档订正

- `10a` §5 改成「已按 Step 9 片 1 拆掉，做法是按 sessionKey 的闸 + `SessionInfo` 用 `TryLock`」；
- `09` L 节 #17 里**撤回**「那时 `Capabilities.SessionID` 的 `Preassignable` 才真正有用
  （grok 可以先发 id 再排队）」这句 —— 按 key 的闸下排队就是锁等待，
  预铸 session id 解决的是「首轮之前就想知道 id」，与并发无关。
  留着它下一个人会去做一件不需要做的事。
- **顺带改 README 那三行的限定**（见 1.5）。

### 2.8 片 1 的验收

1. 两个**不同** agent 的 `Ask` 用假 Spawner 各睡 200 ms 并发跑，总墙钟 < 300 ms
   （今天必然 > 400 ms）。
2. **同一个** agent 的两次 `Ask` 并发跑：两次进入/退出 Spawner 的时间区间**不相交**,
   **且第二次的 `req.ProviderSession` 等于第一次返回的 id**。
   只验区间不相交不够 —— 那在「两次都走 bootstrap 各开一条新会话」时也成立,
   而那正是要防的那种花两次钱。
   **不用 `-race`**：本机无 gcc（未决 #13 已记），这条锁住的是行为。
3. **gate 不被删**：`Rebuild` 之后并发调 `Ask`，断言两次拿到的是同一个 gate 指针
   （或等价地：`Rebuild` 与 `Ask` 并发跑 100 次，两个 goroutine 的 Spawner 区间从不相交）。
   这是 1.3 那条不变量的直接断言。
4. `AskUpTo(upTo=7)`：post 到 seq=10，断言送出去的 prompt 不含 seq 8-10 且
   `rec.ScannedTo == 7`；随后不带上界再跑一轮，8-10 出现在这一轮的 delta 里。
5. 一轮在飞时调 `SessionInfo`：50 ms 内返回且 `InFlight == true`。
6. 一轮在飞时调 `Rebuild`：`errors.Is(err, ErrTurnInFlight)` 为真，
   **且库里那一行原值还在**。
7. 一轮在飞时调 `SetRules`：断言该轮用于计算 `identity_fp` 的规则摘要
   与真正送进 argv 的身份块来自**同一份**规则文本，且下一轮才用上新规则。
8. `go build` / `vet` / `test -count=1 ./...` 全绿；启动漂移审计 0 条。

---

## 3. 片 2：Team 接进会话 + thought 路由 + Judge 可见性

`isMinimalUseful: true`。

**裁决先写清楚**：**Team = 配置层的名册 + 策略容器**；会话侧只增加一个 `team_id` 指针，
**成员仍然只由 `participants` 表说了算**。

否决「Team 在会话里成为一等实体、成员归属改由 team 表达」：那会造出第二个成员事实源，
本项目已经为「两个事实源」付过一次账（`10a` §1.1 的 cursors 表）。
也否决「Team 纯粹是配置宏、会话不记 team_id」：那样策略在运行时无处可查,
一个 agent 同属两个 team 时只能靠猜。

### 3.1 `TeamConfig.ThoughtVisibleTo`

```go
type TeamConfig struct {
	DisplayName      string   `toml:"display_name"`
	Members          []string `toml:"members"`
	ThoughtVisibleTo []Mode   `toml:"thought_visible_to"` // 空 = 谁都看不到，典型值 ["judge"]
}
```

用 `[]Mode` 而不是裸 bool：`04` §3 要表达的本来就是「哪些人看得到」，
一个全局 bool 表达不了「只有裁判看得到」这个典型用法，而那正是文档举的例子。

**`NewRouter(routeThought bool)` 的签名一个字不动**（`10a` 冻结）：
Runner 每轮按 `(conv 绑定的 team 策略, 这个收件人的 Mode)` 算出一个 bool 传进去。

**判别器只有 `Mode`。** 未决 #7 已裁：`Role` 进身份块、进指纹，
拿它分支等于让「把某人改成裁判」变成一次 `ErrIdentityDrift` 加一次入场费。
本机现状核过：`Mode` 今天在非测试代码里只有三个读点
（`load.go:71-74` 填默认与归一、`validate.go:63` 校验、`commands.go:375` 显示），
**一个行为分支都没有** —— 片 3 的 scheduler 是全仓第一处。

### 3.2 校验：两条 fail-closed，**第三条降级成提示**

- **(a)** `thought_visible_to` 的每个值必须是五值枚举之一，
  否则 `add("teams.<id>.thought_visible_to", invalid mode %q)`。
- **(b)** 同一个 team 里重复的 mode 只报一次（与 members 的去重同体例）。
- **(c)** 提案要求「`len(ThoughtVisibleTo) > 0` 且根级 `store_thought == false` 时报错」，
  文案是「`store_thought = false` 时 thought 列恒为空串，这个开关永远路由不到任何东西」。
  **这条理由是编的，而且会挡掉一个合法配置 —— 降级。**

  `config/schema.go` 的注释明写：关掉之后**已经落库的 thought 不回溯**
  （不洗历史，事实源只追加）；这一轮起新写的行，`Thought` 列照写空串。
  也就是说 `store_thought=false` 只影响**此后**的行，
  **历史行里的 thought 照样会被 Router 渲染进 judge 的 prompt**（`router.go` 那条
  分支只看 `m.Thought != ""`）。

  于是「先存了一段时间、现在不想再存了、但仍然要让裁判看得到已有的那些」
  是一个**真实且合法**的配置，这条 fail-closed 会把它拒掉;
  而文案里那句「恒为空串」在事实上是假的。

  **裁决：不拦。** 改成在 `/rules` 或启动横幅印一行：
  「`store_thought = false`：此后新增的轮次不再产生 thought；
  `thought_visible_to` 只对历史行生效。」
  **文案里不许出现「恒为空串」。**

### 3.3 schema v4

```sql
ALTER TABLE conversations ADD COLUMN team_id     TEXT NOT NULL DEFAULT '';
ALTER TABLE turn_usage    ADD COLUMN identity_fp TEXT NOT NULL DEFAULT '';
ALTER TABLE turn_usage    ADD COLUMN round_id    TEXT NOT NULL DEFAULT '';
```

`team_id` 用 `ALTER` 而不是 `DROP+CREATE`：主键没变，与 `10a` §1.1 那次重建不是同一情形，
而且 `conversations` 里有真数据。

**`round_id` 是本契约新加的，提案里没有**，理由见 6.3。

**形状声明有一个已经被注释警告过的坑**：`store/schema.go:330` 是
`var v3Shape = v2Shape` —— 两个变量指向**同一张 map**，`:327` 的注释明写
「这一版没有改列」。若 v4 照着写成 `v4Shape := v3Shape` 再赋值改键,
**会连 `v2Shape` / `v3Shape` 一起改掉**，那两版的形状校验从此对不上历史库。

**`v4Shape` 必须走 `buildV2Shape` 那条路（`make` + `range` 拷贝），
不许赋值别名**，并在注释里点名 `v3Shape` 是别名、不能照抄它的写法。

**验收**：造一个 v3 的旧库跑迁移，`Open` 成功且不报 `ErrSchemaMismatch`
（未决 #10 加的形状校验会逼着形状声明同时改）；
迁移之后**一条既有 provider 会话都不重建**（正面断言 `identity_fp` 没被碰过）。

### 3.4 会话绑定 team

```go
func (s *Service) BindTeam(ctx context.Context, convID, teamID string) error
// Tx 加 SetConversationTeam(ctx, convID, teamID string) error
```

`Service.Create` 的签名一个字不动（`08a` 冻结）。
新增方法既保住冻结签名，又让「已经开着的会话中途绑一个 team」成为可能
（改绑只影响此后的 delta，不改前缀，所以是安全的）。

绑定要留一条 system 面包屑（`ReservedSenderSystem` + `Meta["event"] = "team_bound"`，
形状 `09` L#14 已定死）：它改变了后续所有轮次的可见性规则,
事实源里必须看得出来它是什么时候变的。

**`Tx` 加方法会让假实现编译不过**。
提案在片 2 一个字没说这件事，只在片 3 为 `Store` 新增方法时说了。
**这是好事，是结构在逼人同时改** —— 但要写进 cost，不能让实现方以为是意外。

**订正（2026-09-22，片 2a 落地时）**：本段原文写的是「5 个假实现」并列出
`service_test.go`、`toctou_test.go`、`repl/conversation_test.go`、
`turn/recording_store_test.go`、`turn/runner_cursor_test.go`。
实测**要直接改的只有 4 个** —— `toctou_test.go` 的 `callSpy` 是
`struct { *fakeStore; calls []string }`，**内嵌同包的 `fakeStore`,
新方法自动继承**，一个字都不用改。
（实现方 grok 独立预测时也报了 5，跟着契约错的同一处；测试方现查源码核出是 4。
两边不一致正是背对背要暴露的东西。）
下一次给 `Tx` 或 `Store` 加方法时，**先查一遍哪些假实现是内嵌的**，
再报数字，不要照抄这一段的清单。

### 3.4a 片 2a 落地记录（2026-09-22）

片 2 按 8c 的先例再拆三块，**2a 是存储层**，已落地：schema v4 三列 +
`v4Shape` 深拷贝 + `Conversation.TeamID` + `Tx.SetConversationTeam` +
`Service.BindTeam` + `team_bound` 面包屑 + `TurnUsage.IdentityFP` / `RoundID`
+ `Record.IdentityFP`。2b（校验 + `thoughtVisible` + `ErrTeamUnknown`）与
2c（`/team use`、`/teams`）另记。

**契约面新增，逐条登记**（照未决 #9(a) 立的规矩：新增方法同样是契约面动作）：

| 符号 | 位置 |
|---|---|
| `Conversation.TeamID string` | `internal/conversation/types.go` |
| `TurnUsage.IdentityFP string` / `TurnUsage.RoundID string` | 同上 |
| `Tx.SetConversationTeam(ctx, convID, teamID string) error` | `internal/conversation/store.go` |
| `Service.BindTeam(ctx, convID, teamID string) error` | `internal/conversation/service.go` |
| `ErrEmptyTeamID` | `internal/conversation/store.go` |
| `Record.IdentityFP string` | `internal/turn/record.go` |
| `MetaEvent` / `EventTeamBound` / `MetaTeamID` | `internal/conversation/types.go` |

**三条本片做出的裁决，写下来免得下一个人重推**：

1. **空串 `teamID` 拒绝，不当解绑**，且拒绝落在 `Tx.SetConversationTeam`
   而不只是 `Service.BindTeam`。理由：空串在 `conversations.team_id` 上读作
   「没绑」，拿它当解绑会让「从未绑过」和「绑过又解开」在库里长得一样;
   而事件名只有 `team_bound`，解绑没有对应的面包屑可写，事实源里会留下
   一次**无记录的可见性规则变化**。空串是列语义，闸要落在写这一列的最后一关,
   否则任何直接走 `WriteTx` 的路径都能绕过去。
   **代价**：解绑做不了，要做必须另开方法 + 另开事件。

2. **`conversation` 包自己再声明一次 `MetaEvent = "event"`**。既有的那份在
   `internal/turn/record.go:141`，但 `conversation` **不能导入 `turn`**
   （`turn` 依赖 `conversation`，反向导入是编译环）。两边的值已核过逐字节相同。
   **代价**：两份常量会各自漂移，加新事件时要同时看两处。

3. **`BindTeam` 不查 `cfg.Teams`**，未知 team 的拒绝归 `Ask` 的
   `ErrTeamUnknown`（3.6）。`conversation` 包本来就不认识 `config`。
   **这一条实现方与测试方在互不可见的情况下独立给出了同一个答案**,
   是本片背对背最有价值的一次对上。

**两条本片没覆盖、留给后面的**：

- **指纹算出来之后才拒绝的那一轮**（例如 `ErrIdentityDrift`），
  `turn_usage.identity_fp` 该带那个算出来的指纹，还是仍读作空？
  实现是**带上**（`rec.IdentityFP = fp` 紧跟在 `fp` 算出来之后），
  这个选择是对的（那一轮确实算到了指纹），但**没有用例钉它**。
- **对同一条会话连续 `BindTeam` 两次**，是每次都写一条 `team_bound`,
  还是只在值真的变了时写？实现是每次都写，**没有用例钉它**。
  每次都写是可辩护的（每一次绑定都是一个事件），但要有人明确裁一下。

### 3.5 `/team use`

```
/team use <teamID>
```

绑定当前会话，并把该 team 里**尚未加入**的成员一次性 Join 进来（复用 `Service.Join`）。

**「尚未加入」的判据要写死，不能想当然**：`store.go:61-62` 的 `AddParticipant`
注释明写「同一个 `(ConvID, AgentID)` 之前退出过的话，这是**重新激活**，不是插第二行」。
所以若按 `Participants(ctx, convID, false)` 判，**已 `/leave` 的人会被判成「尚未加入」、
被 Join 一次、被静默拉回** —— 正好是裁决说不做的那件事，而且每拉回一个就是一次入场费。

**判据**：

> 先 `Participants(ctx, convID, includeInactive=**true**)` 拉全量。
> active 的跳过；`left_at` 非空的**也跳过**，并在屏幕上逐个点名
> 「X 已退出，未自动拉回，要加回用 `/join`」。
> 只有两张表里都查不到的才 Join。

`/teams` 保留不动，输出多标一列「当前会话绑的是哪个」。

### 3.6 `routeThought` 由计算得出，team 查不到时拒绝这一轮

```go
func thoughtVisible(team config.TeamConfig, to config.Mode) bool
// Ask 里：router := NewRouter(r.thoughtVisibleFor(conv.TeamID, ac.Mode))
```

- `conv.TeamID == ""` -> `false`；
- `conv.TeamID` 非空但 `cfg.Teams` 里查不到 -> 新哨兵 `ErrTeamUnknown`，
  **refuse 这一轮，起进程之前，一分钱不花**。

否决「查不到就按 false」：那会让「我明明打开了裁判的可见性」与「我什么都没配」
在屏幕上长得一模一样，是本项目定义的那种最危险的静默降级。

这条拒绝走的是既有体例（`runner.go` 里 agent 不存在 / provider 没适配器 /
权限档不放行 / WorkDir 为空都在起进程之前拒），
且未决 #20 已落地 —— 早期拒绝传下去的 `session` 为 nil 现在读作「不碰这一行」,
所以**不会把已建立的 provider 会话删掉**。

**`/config reload` 的交叉影响**（`11d` 4.3 的根级键表没覆盖 teams）：
`thought_visible_to` 改动**允许、不触发漂移**（它不进指纹，见 3.8），
`--apply` 之后下一轮生效。把 teams 这一档补进 `11d` 4.3 的表。
但 team 被**删掉**时，已绑定它的会话下一轮会撞 `ErrTeamUnknown` ——
`/config reload` 的报告里要单列这一类：「N 条会话绑定的 team 将消失」。

**订正补齐（2026-09-22，片 2b 开工后由测试方点出的规格空洞，主代理裁）**：

本节原文只说「refuse 这一轮，起进程之前，一分钱不花」，**没说插在哪一步**。
那不是措辞问题：片 2a 已经把 `turn_usage.identity_fp` 的空串立成了有语义的区分
（**空串 = 这一轮没算到指纹**，见 `Record.IdentityFP` 的注释与
`TestAskRefusedBeforeFingerprintLeavesIdentityFPEmpty`）。
于是插在指纹**之前**还是**之后**，会让同一条拒绝在库里留下两种不同的记录。

**裁决：插在 `conv` 取到之后、`NewRouter` 之前，也就是指纹算出来之后。
那一行的 `identity_fp` 非空。**

理由有两层：

1. **结构上本来就只能这样**。这条检查要读 `conv.TeamID`，而 `conv` 是在
   `readAfter` 之后才取的（`runner.go` 里 `r.svc.Conversation` 那一行），
   而 `fp` 早在会话状态解析之前就算完了。要想更早拒绝，得把取 `conv` 整个提前，
   那是另一次重排，本片不做。
2. **非空才是诚实的**。这一轮**确实**算到了指纹 —— 它是在「身份已经定下、
   但这条会话绑的 team 查不到」这一步倒下的。把它记成空串等于说
   「这一轮没算到指纹」，那是假的。

**代价**：`identity_fp` 非空的行里，从此混有「算到了指纹但根本没起进程」的轮次。
读这一列的人要同时看 `outcome` —— 与未决 #23 对 `cold_start` 的处置同一条纪律。

**这条要有用例钉**：`ErrTeamUnknown` 那一轮的 `turn_usage.identity_fp` 断言非空。
它顺带补上了片 2a 记下的那条缺口（「指纹算出来之后才拒绝的那一轮该不该带指纹」,
见 3.4a 末尾），那一条从此有了一个具体的、可断言的场景。

**另外三条测试方点出、本片明确不做的**（记下来免得下一个人以为是漏项）：

- **`Ask` 不校验 `agentID` 是不是 `conv.TeamID` 那个 team 的成员**。本节只问
  「这个 team 存在不存在」。成员资格由 `participants` 表说了算（3 节开头的裁决：
  **成员仍然只由 `participants` 表说了算**），team 只是策略容器。
  加成员校验会造出第二个成员事实源，正是本步否决过的那件事。
- **3.2(c) 的「降级成提示」是屏幕文案，不在 `Validate()` 这一层**，
  所以它没有、也不该有 `ValidationErrors` 的用例。
- **`/config reload` 对「绑定的 team 被删」的横幅**（本节结尾那句）不在片 2b,
  它是 `/config reload` 命令的行为，留给后面。

### 3.7 双向用例：thought 到底有没有进送出去的字节

两个用例共用一份夹具（一条带非空 `Thought` 的历史消息）：

- **(A)** 策略不含该收件人的 Mode -> 断言那段 Thought 文本
  **不出现在 argv 任一元素、也不出现在 stdin**；
- **(B)** 策略含该 Mode -> 断言它**出现**。

**缺后半就等于没验** —— 一次「这轮本来就没有 thought」的巧合能把验收整个骗过去，
而今天没有任何一条用例断言生产路径传的是 `false`（把 `NewRouter(false)` 改成
`NewRouter(true)` 全套用例照样绿）。

**判据是送出去的字节**，不是退出码、不是屏幕。
把 `thoughtVisible` 写死成恒 true 或恒 false 时，必有一个变红。

### 3.8 明写：改 `thought_visible_to` 不触发漂移

用例：同一个 agent，只改 team 的 `thought_visible_to`，断言
`IdentityFingerprint` **逐字节不变**、下一轮不走 Fresh、`provider_sessions` 那一行不动。

理由：thought 只进 Render 出来的正文，**不进身份块、不进前缀** ——
它让此后的 delta 变大，不改已送出去的前缀，按 `04` §0 规则 1 是合法的追加。
反过来也要说清楚：**它不回溯**，已经投递过的消息不会被补上 thought，
因为那就是改写前缀。

写成用例而不是注释，是因为下一个人很可能出于「保险」把它塞进指纹，
那一塞就是每改一次开关全队重付入场费（5 人量级 90k-240k token）。

### 3.9 片 2 的验收

1. 三条校验各一个用例（(a)(b) 断言 `ValidationErrors` 里有对应 Path；
   **(c) 断言的是「不报错」** —— 见 3.2 的降级）。
2. v3 旧库跑迁移：`Open` 成功、不报 `ErrSchemaMismatch`、`identity_fp` 没被碰过。
3. **`v4Shape` 不是别名**：改 `v4Shape` 之后断言 `v2Shape` 与 `v3Shape` 不变。
   这条直接钉 3.3 那个坑。
4. `BindTeam` 之后 `Conversation(ctx, id)` 读回的 `TeamID` 是新值，
   且会话里多了一条 `event=team_bound` 的 system 消息；重启进程再读，值还在。
5. `/team use`：对一个已有两名成员的会话用一个三人 team，`/who` 显示**三人**;
   **先 `/leave` 一个再 `/team use` 同一个 team，那个人仍然是 inactive，
   且屏幕点名了他**（3.5 的直接断言）。
6. 把配置里的 team 删掉再 `/ask`：`Outcome == OutcomeRefused`、
   `errors.Is(err, ErrTeamUnknown)` 为真、`wall_ms == 0`、
   **且 `provider_sessions` 那一行原值还在**。
7. 3.7 的双向用例两个都在。
8. 3.8 的指纹不变用例。

---

## 4. 片 3：Scheduler、`/round` 升级、Budget、Judge

`isMinimalUseful: false`。依赖片 1 与片 2。

**删掉提案里的「第 0 个 commit：修 @mention 的 token 边界」** ——
那件事已经在 `docs/11d_STEP8C_CONTRACT.md` 第 2 节（8c-1）冻结,
而且 `11d` 1.2 把结论推得更远（Router 根本不解析 mention）。
本片不得重做它。复核针对那个 commit 的验收批评因此一并作废。

### 4.1 `Plan` 与 `PlanRound`

```go
type Plan struct {
	Speakers []string
	Judges   []string
	Skipped  []SkipReason
}

// requested 为 nil 表示全员。本函数不解析任何文本（理由见 1.2）。
func PlanRound(members []conversation.Participant,
	agents map[string]config.AgentConfig, requested []string) Plan
```

- `Speakers` 与 `Judges` 按字典序，**互不相交**。
- `ModeJudge`：不进 `Speakers`。
- **`ModeManager`：本步与 `ModeDiscussion` 完全同待遇**，
  在代码注释与文档里明写这是有意的剩余项。
  `02` §2 只说了它与 judge 判别器同型，没说它做什么；
  编一个就是发明没设计过的语义，不写清楚则读代码的人会以为漏实现了。

纯函数，与 Router 同纪律：不看时钟、不看随机源、不依赖 map 迭代顺序。

**`Skipped` 的理由集合必须包含 `no_delta`**（复核 missing 6）：
连着敲两次 `/round`，第二次绝大多数成员会走 `runner.go:349` 那条 `ErrNoDelta`
早期拒绝 —— 不起进程、不花钱、`Outcome == OutcomeRefused`、游标照推。
`Select` 是纯函数、起进程之前就跑得出来，所以**这一条在 Budget 阶段就能判出来**。
这些成员在 Budget 表里单独一档「本轮无新内容，不唤醒」,
**结算行的 N 只数真正起了进程的**。不做的话用户会看到
「唤醒 5 人、总 token 0」，然后不知道是没花钱还是记账坏了。

### 4.2 轮次模型的裁决

**先把成本这条轴说清楚**：轮流与全体并发**在入场费上完全等价** ——
两者都唤醒全部 N 人，而每个 agent 的入场费是固定成本、与它说多少话无关（D-5）。
所以「轮流 vs 并发」是**延迟与串话**的选择，不是成本选择;
**唯一能省钱的是不唤醒某人**。

**裁决：默认全体并发一轮，全员共用同一个 `upTo` 快照。**
串行是 `requested` 依次跑的退化情形，不做专门的 `--serial`。

**并且必须写进文档的一条**：按 mention 省下的入场费,
**可能被 1 小时缓存 TTL 过期后重付的入场费吃掉** ——
让一个成员沉默 90 分钟再发言，比让它每 40 分钟说一句要贵（`04` 原文）。
所以不把「按 mention」做成省钱模式，账要连起来算。

```go
func (r *Runner) RunRound(ctx context.Context, convID string, plan Plan, maxParallel int) []Record
```

进来先取一次 `upTo := MaxSeq(convID)` **与一次 rules 快照**（2.6），
每个成员各起一个 goroutine 调 `AskUpTo`，全部返回后才返回。
`maxParallel <= 0` 表示不限。**一个成员失败不取消别人** ——
失败是既有设计里的正常业务结局，一人超时把另外四人已经花过的钱作废才是真正的损失。

### 4.3 `maxParallel`：**不加配置键**

提案的 ruling 说「`maxParallel` 这个键留好（Team 配置上，0 = 不限）」,
但三片的交付物里**没有任何一条**给 `TeamConfig` 加这个键、给 validate 加校验、
给 `/round` 接这个值。那就是一个**谁也不读的键** ——
正是本项目（与 `11d` 两次）反对的死键。

**裁决：本步不加这个配置键。** `RunRound` 的 `maxParallel` 只作为函数参数,
REPL 恒传 0，把「将来加在 Team 配置上」记进剩余项。

同时把探针的定位改写清楚：它测的是**「N 个进程能不能同时拉起来」**,
不是「并发上限是多少」，标题不要叫「上限标定」 ——
提案自己也承认那个探针（三家都填不存在的 model 名）
「不标定真实推理时的内存占用」。

### 4.4 argv 硬上限：**扇出最直接的代价，提案一个字没提**

`grok.go` 与 `agy.go` 都是 `MaxPromptBytes: windowsArgvPromptLimit`，正文走 argv;
只有 claude 改走 stdin。`11` §2 的标题就叫「规模即成本，而且在两家上是硬上限」,
未决 #15 / #19 的 `ErrCommandLineTooLong` 就是为此立的。

**本步在两个方向同时把 prompt 撑大**：

1. 并发一轮之后每个成员的下一批 delta 里是 **N-1** 条别人的发言，不是 1 条；
2. `thought_visible_to` 打开之后每条消息还要附上 thought 正文。

两件事叠起来，grok 与 agy 的成员会在某一轮开始**每一轮都被起进程之前拒绝**,
而那是一个「从此再也说不了话、除非 rebuild 重付入场费」的死局。

**裁决**：

- **Budget 表加一列「argv 余量」**，复用 `/rules` 已有的同类余量探针
  （`11c` §6 已按 `(provider, tier)` 分组）。
- **`/round` 在起进程之前，对任何会超限的成员响亮拒绝整轮**,
  而不是让它们一个个 refuse。理由：一轮里五个人有两个必然失败,
  另外三个的钱照花，用户拿到一个残缺的轮次 —— fail-closed 是整轮不跑。

### 4.5 Budget

```go
type BudgetRow struct {
	AgentID, Provider string
	Cold       bool    // 预报；ok == false 时必为 true（11d 3.2）
	InFlight   bool    // 复用 SessionInfo 的 TryLock 路径
	Calibrated bool
	InputTokens *int
	CostUSD     *float64
	ArgvHeadroom *int  // 4.4
	Basis        string // 取样的那个 turn_id，让人能自己去库里核
}

func (r *Runner) Budget(ctx context.Context, convID string, plan Plan) ([]BudgetRow, error)
```

**取样规则**（`UsageSamples(agentID, identityFP, limit)`，按 `CacheAcrossTurns` 分三条）：

- `SupportNo`（agy）：取最近一条成功轮的 `input_tokens`，**不分冷暖**
  （未决 #17 实测跨轮无缓存收益）；
- `SupportYes` 且预报冷：取最近一条 `cold_start = 1` 的行；
- `SupportYes` 且预报暖：取最近一条 `cold_start = 0` 的行。

**样本口径**：只要 `input_tokens` 非 NULL 就算样本 —— 包括
`OutcomePostRejected`（钱花了、用量也记了，排除它就是往低估方向偏）。

**预报按规模缩放，不照抄样本**（1.4）：

```
预报 = 样本.input_tokens / 样本.prompt_bytes × 本轮.prompt_bytes
```

**样本 `prompt_bytes == 0` 的行不能用来缩放，必须跳过**（`NOT NULL DEFAULT 0`,
历史行都是 0，直接用就是除零）。跳完之后一条可用样本都没有 -> `Calibrated = false`。

**`Calibrated = false` 时**：`InputTokens` 与 `CostUSD` 都是 nil，那一行印「未标定」、
**不进合计**，合计旁边响亮写「另有 k 名成员未标定，本数字是下限」。

**合计能不能加**：token **能**加 —— `capabilities.go:99` 明写三家的
`turn_usage.input_tokens` 都归一到「本轮完整 prompt」这同一个量
（只是各自的计算公式不同）。**金额不能**加 —— 只有 claude 报金额,
grok 与 agy 只报 token，**不为了凑总金额编汇率**（与 `writeAskMeter` 今天的口径一致,
也与 `05` §3「可空表示这家根本没有这个数，不是 0」同一条纪律）。

**方向不作硬断言**（1.4）：`Basis` 点名 `turn_id`，屏幕上预报与事后实测并排印,
方向**记录下来**。

**Budget 与 `RunRound` 必须用同一份 rules 快照。** Budget 要算本轮的
`prompt_bytes`，那要把身份块渲染出来，而身份块含两段规则正文（`11a`）。
若 Budget 在 T1 取规则、`RunRound` 在 T2 再取一次，预报与实际送出去的
就不是同一段前缀，而 Budget 的全部意义在于预报那一段的规模。

今天这件事不会出错，因为 REPL 的命令派发是串行的（`11d` 1.3）——
**但本片 2.1 明说片 1 的全部意义是「结构上允许并发」**,
所以不许把这条正确性挂在那个串行上。

**落法二选一，写死其中一条**：把 Budget 收进 `RunRound` 的开头（同一次快照）,
或者让 `/round` 取一次快照后**同时**传给 Budget 与 `RunRound`。
`upTo` 同理：Budget 判 `no_delta`（4.1）用的必须是 `RunRound` 将要用的那个 `upTo`。

**`identity_fp` 的空窗期要正面裁掉**（复核 missing 7）：本列只有片 2 之后产生的行有值,
历史行是空串。**片 3 上线后每个成员的第一轮必然印「未标定」，这是对的** ——
不要为了让表好看去回填一个猜出来的指纹（回填就是编数据）。
`/round` 的输出里把这句话印给用户看一次。

### 4.5a 片 3c 开工后补齐的四处空洞（2026-09-22，测试方点出，主代理裁）

背对背的测试方在写用例时点出四处契约没说清的地方。**四条都是真的空洞**，逐条裁：

#### 一、`本轮.prompt_bytes` 与 `样本.prompt_bytes` 必须是**同一个量**（最要紧的一条）

4.5 那句「Budget 要算本轮的 `prompt_bytes`，那要把身份块渲染出来」**容易被读成
「prompt_bytes 含身份块」，那是错的**。现查源码：

- `runner.go` 里 `rec.PromptBytes = len(prompt)`，而
  `prompt = router.Render(selected, conv.Title, bootstrap)` —— **只有渲染出来的 delta**；
- 身份块是 `TurnRequest.Role`，**另一个字段**，不进 `prompt`;
- `probeHeadroom` 渲染身份块是为了量 **argv 命令行总长**（那里 `Prompt` 传空串）。

**裁决：`本轮.prompt_bytes` 就是 `len(Render(...))`，不含身份块 ——
与落进 `turn_usage.prompt_bytes` 的那个量逐字节同定义。**
Budget 渲染身份块只为 `ArgvHeadroom`，不进缩放的乘数。

照字面读成「含身份块」的后果：缩放比的**分母**（样本，不含身份）与**乘数**
（本轮，含身份）是两个不同的量，预报**系统性偏高**，而且偏多少取决于规则文件多长 ——
没有任何一层会报错。

**顺带把这条公式自带的近似写明**（免得下一个人「修」错方向）：
`样本.input_tokens` 是 provider 看到的**全部**输入（身份块 + delta）的 token 数，
而 `样本.prompt_bytes` 只是 delta 的字节数。所以这个比值是
「(身份+delta) 的 token / delta 的字节」—— 缩放时**把固定的身份块成本当成了
按比例变化的**。本轮 delta 比样本大很多时偏高、小很多时偏低。

**这是已知近似，不是缺陷，不许通过「把身份块算进 prompt_bytes」去修** ——
那会破坏与 `turn_usage` 那一列的同定义，代价更大。
4.5 已经为此立了规矩：**方向不作硬断言**，`Basis` 点名 `turn_id`，
屏幕上预报与事后实测**并排印**，方向只记录、不承诺。

#### 二、`no_delta` 怎么回到 `Plan.Skipped`

`Budget(ctx, convID string, plan Plan)` 收的是 **`Plan` 值**，
**没有任何回写通道** —— 4.9 第 8 条那句「该成员进了 `Skipped` 的 `no_delta` 一档」
按现在这个签名做不到。

**裁决：`BudgetRow` 加一个 `NoDelta bool`，`Skipped` 由 `/round`（片 3d）据它组。**
不改 `Budget` 的签名（4.5 已登记），也不让 Budget 去改调用方的 `Plan` ——
一个返回 `[]BudgetRow` 的函数顺手改入参，是本项目反对的那种隐式副作用。

片 3d 落地时：`/round` 拿到 `[]BudgetRow`，对 `NoDelta == true` 的成员
追加一条 `SkipReason{Kind: SkipNoDelta}`，**结算行的 N 只数真正起了进程的**。

#### 三、「最近一条」的排序键

4.5 反复说「取**最近一条**」，但没说按什么排。

**裁决：`ORDER BY created_at DESC`，并以 `turn_id` 作第二排序键。**
只按 `created_at` 排不够：本地 SQLite 的写是亚毫秒级的，同一轮扇出的五条
`turn_usage` 完全可能拿到同一个 `created_at` 字符串，那时顺序就交给了
SQLite 的实现细节 —— 而「预报用了哪一条样本」必须是确定的，否则同一份库
跑两次 `Budget` 会给出两个不同的 `Basis`。

#### 四、`SupportNo` 那一支的样本口径

4.5 的 `SupportNo` 条目写「取最近一条**成功轮**的 `input_tokens`」，
而同节的「样本口径」写的是「只要 `input_tokens` 非 NULL 就算样本，
**包括 `OutcomePostRejected`**」。两句话不一致。

**裁决：以「样本口径」那句为准，三支统一** —— `input_tokens` 非 NULL 即算样本。
`SupportNo` 那句里的「成功轮」是措辞不严，不是另立一条判据。
排除 `OutcomePostRejected` 就是往**低估**方向偏（那一轮钱花了、用量也记了）。

### 4.6 `/round` 的升级形状

```
/round [id...] [--go] [--with-judge] [--rounds N]
```

**`/round` 不收文本。** 提案让它带一段文本并「先落库、再打 Budget 表、
一个进程都不起」，那意味着「看了价格决定不发」**做不到** ——
事实源只追加，那条消息已经在库里，此后每个成员的 delta 都会带上它。

`11d` 8c-2 定的名单规则本来就是「**最后一条 `KindUser` 消息**里的 mention」,
也就是说用户先用既有路径（裸文本 / `/say` / 多行草稿）把话说出去，再敲 `/round`。
**保持这一条，不给 `/round` 加文本参数。** 好处三条：

1. 「反悔」这件事回到既有的多行草稿状态里解决，不新增 pending 状态机；
2. `upTo` 与那条 user 消息的先后**自动正确** —— 话先落库，
   `RunRound` 取的 `upTo` 必然 `>= 那条 seq`（复核 missing 2 担心的顺序问题消失）;
3. `/round` 的语义变干净：它只回答「让谁就现有的历史说一轮」。

**仍要一条用例断言**：五个成员的 prompt 里都含用户刚说的那句话。

`--rounds N`：默认 1，**`N > 1` 只在 `--go` 下可用**。
默认 1 不是一个标定值，是最保守值；不给次数就不循环,
对应 `06` §5 从 relay 抄的「硬封顶」那条。

**REPL 在扇出期间阻塞**，`--go` 之前印的时长从 `11d` 3.3 的
「最长 N × 5 分钟」改成「**约 1 × 5 分钟**（并发，最慢的那个成员说了算）」。
理由仍然写**串行的命令派发**，不写 `r.mu`（`11d` 1.3）。

### 4.6a 片 3d 的落地契约（2026-09-22 冻结，实现方与测试方据此背对背开工）

4.6 定的是**形状**，本节定的是**接法**。片 3a/3b/3c 已经把零件全造好了，
3d 基本上是接线 + 渲染，不是重新设计。

#### 4.6a.1 开工前先加一个函数（未决 #30 已裁，选 (b)）

```go
func (r *Runner) RunRoundPrepared(ctx context.Context, convID string, plan Plan, maxParallel int, snap RoundSnap) []Record
```

它铸 `roundID`、分配并预填 `out`、再转给既有的 `RunRoundWith`。
`RunRound` 随之退化成 `TakeRoundSnap` + `RunRoundPrepared` 的薄包装。

**为什么不让 `/round` 直接调 `RunRoundWith`**：调用方得同时知道四件包内约定 ——
`out` 长度等于 `len(plan.Speakers)`、每行预填 `ConvID` 与 `AgentID` 且顺序与
`Speakers` 一致、**每行默认 `Outcome: OutcomeRefused`**、出错时逐行塞 `.Err` 再整体返回。
让 `internal/repl` 复制这四条，等于把「一个从没跑过的 Record 默认是 refused」
这条不变量放进两个包。铸 id 的入口因此是**减少**到一处，不是增加。

#### 4.6a.2 `/round` 的执行顺序（固定八步，不许调换）

1. 解析 flag：`--go`、`--with-judge`、`--rounds N`。剔掉之后剩下的才是 id。
   `--rounds N` 的 `N > 1` **只在 `--go` 下可用**，否则拒绝（不印计划也不跑）。默认 1。
2. 取名单：给了 id 就用给的；没给则取最后一条 `KindUser` 里的 mention
   与当前活跃成员的交集。**名单为空一律拒绝，绝不默认全员。**这一段逻辑照搬现有
   `cmdRound`，一个字不改。
3. **`snap, err := r.rn.TakeRoundSnap(ctx, convID)`** —— **这一轮**取这一次。
4. `plan := turn.PlanRound(members, agents, ids)`。
5. `rows, err := r.rn.BudgetWith(ctx, convID, plan, snap)` —— 与第 3 步同一份 snap。
6. 印 Budget 表（见 4.6a.3）。**没有 `--go` 就到此为止、返回。**
7. **argv 整轮拒绝闸**（见 4.6a.4）。命中就在这里停，**一个进程都不起**。
8. `--go`：把 `NoDelta` 的成员从 `plan.Speakers` 挪进 `plan.Skipped`
   （`Kind: turn.SkipNoDelta`），再
   `r.rn.RunRoundPrepared(ctx, convID, plan, 0, snap)`。

**第 6 步在第 7 步之前，是有意的**（4.4 只说「起进程之前」拒绝，没说「印表之前」）：
拒绝的理由就写在表里的 `argv 余量` 那一列。先拒绝再不印表，用户看到
「成员 X 超了 N 字节」却看不到那一列，等于把解释藏起来了。

**`maxParallel` 恒传 0**，见 4.3 的裁决（本步不加这个配置键，
留好一个谁也不读的键正是本项目反对的死键）。实现方不要自己编一个常数。

**第 3 步的不变量是「一轮一次」，不是「一条命令一次」。**（2026-09-22 订正：
冻结时写的是「整条命令只取这一次」，与 4.6a.6 的「每一轮都重新 TakeRoundSnap」
和 4.6a.7 的「judges 走第二次 TakeRoundSnap」直接打架，验收 2b 与 9 也因此
一个要 `MaxSeq == 1`、一个要 `== 3`。量词写错了，以本段为准。）

**真正的不变量**：一轮取一次快照，**这一轮的预报与执行必须共用它** ——
`BudgetWith(snap)` 与 `RunRoundPrepared(snap)` 之间的 `SetRules` /
`SetConfig --apply` 撬不开这条缝。分开调公开的 `Budget` 与 `RunRound`
会各拍一次，那正是 3c 导出 `*With` 三件套要避免的。

**「撬不开」的范围是一轮之内，不跨轮。** 轮与轮之间用户改了配置，
那本来就是两轮（4.6a.6 已这么定），新一轮理当看见新配置。

于是 `MaxSeq` 的调用次数是：**发言轮数 + （带 `--with-judge` 则再 +1）**。
验收 2b（不带 `--rounds` 不带 `--with-judge`）仍是 1；验收 9（`--rounds 3`）是 3。

#### 4.6a.2a 渲染必须是纯函数（2026-09-22 补裁，现查假 Store 之后）

**`internal/repl` 里三个包内纯函数，`cmdRound` 只负责调用它们：**
（2026-09-22 订正：冻结时写的是「导出两个」，但签名给的是小写不导出的，
第一刀也按小写落了；第二刀又要加第三个印 `Skipped`。以本段为准 ——
**包内、三个**。不导出是对的，它们只服务本包的 `/round`。）

```go
func renderBudget(rows []turn.BudgetRow) string
func renderSkipped(skipped []turn.SkipReason) string        // 第二刀新增
func renderSettlement(s turn.RoundSettlement, forecastCold int, unexpectedCold []string, label string) string
```

`renderSettlement` 的 `label` 是第二刀加的第四个参数（`发言轮` / `裁判轮`），
见 4.6a.7。第一刀的调用点传 `发言轮` 即可。

不碰 `REPL`、不碰库、不起进程，收已经算好的数据、吐字符串。

**为什么必须这样，不是风格偏好**：`internal/repl` 的 `fakeStore.UsageSamples`
返回 `nil, nil`（接口新增方法时补的桩）。于是任何 REPL 层的 `/round` 用例里
**人人 `Calibrated == false`** —— 「已标定行怎么印」「金额 nil 但 token 有值怎么印」
这两条渲染路径在 REPL 层**结构上走不到**。

而 4.6a.8 的验收 7（`cache_read` 未知与 `cold=0` 同时出现时不许裸印 0）
恰恰需要构造一组特定的 `RoundSettlement`。纯函数可以直接喂合成数据：
`Calibrated=false` 的行、`CostUSD=nil` 而 `Calibrated=true` 的行、
`ArgvHeadroom=nil` 的行、`HaveCacheRead=false` 而 `Cold=0` 的结算，
一个个直接造出来。

**这不是「顺便也做成纯函数」，是「不这样做，本片最要紧的两条验收就没法测」。**

#### 4.6a.3 Budget 表的渲染规则

**每一条都是为了不把「不知道」印成一个确定的数。**

- `BudgetRow.Calibrated == false`：那一行的 token 与金额印 **`未标定`**，不印 0。
  表尾另起一行「另有 k 名未标定」，k 是这样的成员数。
  **`k == 0` 时整行省略**（2026-09-22 补：冻结时没说，第一刀无条件印，
  于是全员已标定时会印出「另有 0 名未标定」。那个 0 是已知计数、不是「不知道」，
  不违反本片的纪律，但是一行废话）。
- `CostUSD == nil` 而 `Calibrated == true`：金额那格印 `未标定`（样本没报金额），
  **token 格照印** —— 两者是独立的两件事，不许因为没金额就把 token 也藏起来。
- **合计只加 `InputTokens`，不加金额。** 只有 claude 报金额，为凑总金额编汇率
  就是把「没这个数」写成一个数（4.5 已定，这条同样约束单行，不只约束合计）。
- `NoDelta == true` 的成员**必须出现在表上**，单独一档（如 `无新内容，本轮跳过`），
  不从表里消失。
- `Basis` 点名那个 `turn_id`，不要只印「已标定」。
- `ArgvHeadroom` 为负数照印负数，不夹到 0。

#### 4.6a.4 argv 整轮拒绝（4.4 的落地）

第 7 步（2026-09-22 订正：冻结时这里写的是「第 6 步」，
而 4.6a.2 把印表与拒绝的先后换过，闸在印表之后。以第 7 步为准）：`rows` 里任何一行 `ArgvHeadroom != nil && *ArgvHeadroom < 0`，
**拒绝整轮**，点名是哪几个成员、各自超了多少字节，并印一句可操作的出路。

**fail-closed 的理由写进屏幕文案**：一轮里五个人有两个必然失败，
另外三个的钱照花，用户拿到一个残缺的轮次。整轮不跑比部分跑更便宜。

`ArgvHeadroom == nil`（组不出命令行、量不出余量）**不算超限**，
但要在那一行印 `余量未知`，不要印成一个数，也不要当成 0 去触发拒绝。

#### 4.6a.5 结算行的渲染（本片最容易把「不知道」印错的地方）

`RunRoundPrepared` 已经把 `round_finished` 面包屑写进事实源了，
屏幕这一行是**另印**的，不要去读回那条面包屑再印。

**但「口径一致」不许靠在 `internal/repl` 里再实现一遍聚合来保证。**
认 `Unreported`、`spawn_failed` 分档、`usage_unknown` 计数这三件事是
`internal/turn` 的知识，抄一份到 repl，两边任何一处改了另一处都不会跟着改 ——
与未决 #30 裁掉的是同一个缺陷。

**裁决：`internal/turn` 导出一个纯函数**

```go
type RoundSettlement struct {
    Spawned, SpawnFailed, UsageUnknown, CostUnreported, Cold, ColdUnknown int  // ColdUnknown：X-36ab（2026-09-23）加
    InputTokens, OutputTokens, CacheRead, CacheCreate, ThinkingTokens int
    // Have* 为 false 表示「一个成员都没报过这一列」-> 屏幕印「未知」，不印 0
    HaveIn, HaveOut, HaveCacheRead, HaveCacheCreate, HaveThinking, HaveCost bool
    CostUSD float64
}

func Settle(recs []Record) RoundSettlement
```

`roundFinishedMeta` 改成调它再序列化成 Meta；`/round` 拿着
`RunRoundPrepared` 返回的 `[]Record` 也调它，然后只负责渲染。
**聚合只有一份实现，两个消费者。**屏幕与事实源口径一致因此是结构性的，
不是靠两边各自小心。

渲染时照下面这几条：

- `spawned` 是**确知起过进程**的人数。`spawn_failed` 是**分不清进程有没有起来**的那一档
  （不在 PATH / 服务睡着 / 起来了立刻非零退出，三者无法区分），**必须单独印**，
  不许并进 `spawned`，也不许省略。
- `usage_unknown > 0` 时，四个 token 合计旁边**必须带一句限定**
  （如 `（k 名成员用量未知，合计只含其余人）`；X-38e 之前的措辞是「未解析用量」）。
  **X-38e 订正（2026-09-24）**：`usage_unknown` 数的是 spawned 里「五个用量一个数都没有」的成员（`Record.ReportedUsage()` 五个全 nil），
  不只是没解析到用量的——usage 对象缺失的成功轮、grok 的错误信封轮也在内。
  不带这句，一个部分和会被读成全体的和。
- **某个用量指标全员未报时，那个键在 Meta 里根本不存在。屏幕上印 `未知`，不印 0。**
- **这一条单列出来，因为它最容易错**：`cache_read` 未知与 `cold=0` 可以同时出现，
  而那个 `0` 的意思是**「没人够格被数」**，不是「没有冷启动」。
  两者同时出现时，`cold` 那一格必须印 `未知` 或带限定语，**不许裸印 0**。
- 预报冷启动数 vs 实测冷启动数并排印；`UnexpectedCold` 为真的成员**点名**。
- **X-36ab 补（2026-09-23）**：冷启动只数冷暖是实测值的成员（`Record.MeasuredCold` 的 `known` 为真：Ingested、用量已解析、cache_read 已报）。
  其余计入 `spawned` 的成员数进 `ColdUnknown` / Meta 的 `cold_unknown`（含五个用量都没有数的，与 `usage_unknown` 可以重叠；`usage_unknown` 数的人全在 `cold_unknown` 里）。
  屏幕：有人冷暖未知时印 `实测 N（k 名成员冷暖未知，只数其余人）`，没有一个人冷暖确知时印 `未知`。

#### 4.6a.5a 第二刀要补的六条（2026-09-22，第一刀落地后由实现方自报，主代理核实）

第一刀（`cmdRound` 切到 `RunRoundPrepared`）落地时，实现方列出了它没做、
而任务书也没点名的副作用。逐条记在这里，**第二刀一并处理**，不要丢。

1. **被跳过的成员会从预算表里无声消失。** 表只印 `plan.Speakers`；
   `PlanRound` 判成 `not_in_config` / `not_on_roster` / `mode` 的人只进
   `plan.Skipped`，屏幕一个字不提。**用户点名了一个人，他既不在表上、
   也不会被问、还没有解释。**这是本条里唯一的用户可见缺口。
   **第二刀必须把 `plan.Skipped` 印出来**，每人带上 `Kind` 与 `Reason`。
   `mode` 那一档要说清楚出路。

   **2026-09-22 订正**：原文写「点名一个 `ModeJudge` 的成员就会走到这里」——
   **这句是错的**。`PlanRound` 把 `ModeJudge` 收进 `plan.Judges`，
   `SkipMode` 只发给五值枚举**之外**的 mode。
   点名的裁判不在 `Skipped` 里，`renderSkipped` 碰不到他。
   那半个缺口由新增的验收 13a 单独处理。

2. **`--rounds` 与 `--with-judge` 现在会掉进 id 名单**，多半被
   「不是当前活跃成员」拒绝。第二刀实现它们时自然消失，但在那之前
   用户敲这两个 flag 会拿到一句驴唇不对马嘴的错。

3. **`forecastCold` 把随后挪进 `Skipped` 的 `NoDelta` 成员也数了进去。**
   于是「预报冷启动数」可以大于这一轮真正起的进程数，
   即使一个意外冷启动都没有。要么改成只数留在 `Speakers` 里的，
   要么在屏幕上说清楚这个数的口径。

4. **发言者被挪空之后仍然调用 `RunRoundPrepared`**（全员 `NoDelta`，
   或点名的全是裁判）。空 `Speakers` 仍会写
   `round_started` / `round_finished` 两条系统消息 ——
   事实源里于是有一轮「什么都没发生的轮」。裁一下要不要提前返回。

5. **`PlanRound` 把发言者排成字典序并去重。** 表和扇出不再是键入顺序
   或 mention 顺序；同一个 id 写两次，旧的串行循环会问两次，现在问一次。
   这是 `PlanRound` 的既定行为、不是缺陷，但屏幕没有解释顺序为什么变了。

6. **「扇出期间 REPL 不接受任何命令、最长 N x 5 分钟」那句没了。**
   命令派发仍是同步的，但这一层不再告诉用户，也不再给墙钟上界。
   而且 `maxParallel` 为 0 时全部发言者**同时**起进程，
   旧的「N x 5 分钟」本来就不再成立（4.6 已改成「约 1 x 5 分钟」）。
   第二刀要按 4.6 的措辞把这句加回去。

#### 4.6a.6 `--rounds N`

`N > 1` 时把第 3 到第 8 步整体重复 N 次，**每一轮都重新 `TakeRoundSnap`**
（轮与轮之间用户可能改了配置，那是两轮，不是一轮）。
每轮之间检查 `ctx.Err()`，被腰斩就印「已跑 k/N 轮，其余取消」。
**不引入任何配置键，不引入自动链。**上限永远是人按的那一次。

#### 4.6a.7 `--with-judge`

`Speakers` 全部返回之后，judges 走**第二次 `TakeRoundSnap` + 第二次
`RunRoundPrepared`**，于是它们拿到自己的 `round_id`，事实源里是第二对
`round_started` / `round_finished` 面包屑。
judge 看的是本轮结论，所以不能与 Speakers 共用同一个 snap ——
**这正是 judge 不进 Speakers 的实际原因，不只是「角色不同」。**
不给 judge 特殊模板、不解析 verdict 语法、不自动触发、不循环。

**2026-09-22 订正三处，都是冻结时留下的洞：**

**(1) 不是「重新取一次 `MaxSeq`」，是重新 `TakeRoundSnap`。** 冻结时这两句并列写着，
但它们不是一回事：`TakeRoundSnap` 除了 `MaxSeq` 还会 `snapshotRulesConfig()`
重拍 rules 与 cfg。也就是说一条 `/round --with-judge` 命令中途的
`SetRules` / `SetConfig --apply` **会被裁判轮看见**。

**这是对的，不是缺陷。** 裁判轮按本项目对「一轮」的定义（一份快照、一个 `upTo`、
一组成员）**就是另一轮**，与 4.6a.6 的轮间口径一致：轮与轮之间改了配置，
新一轮理当看见新配置。4.6a.2 那句「撬不开这条缝」的范围是**一轮之内的
预报与执行之间**，不跨轮 —— 那一节已同步订正。

**(2) 裁判轮就是一轮普通的轮，只是 `Speakers` 换成 `plan.Judges`。**

冻结时只说了「再跑 `plan.Judges`」，漏了一件事：`BudgetWith` 只为
`plan.Speakers` 出行（`budget.go` 明写「Judges 本片不预报」），于是照字面实现，
**裁判轮既没有预算表、也没有 argv 余量检查** —— 一个 argv 超限的 judge
会在第二轮直接失败，而 argv 闸的存在意义正是挡这个。

**裁决：构造 `judgePlan := turn.Plan{Speakers: plan.Judges}`，
把 4.6a.2 的第 3 到第 8 步对它原样再跑一遍。** 预算表、argv 整轮拒绝、
面包屑、结算行于是全部自动适用，不为裁判轮开任何特例。
它的预算表与结算行都带 `裁判轮` 标签。

**(3) `--rounds N --with-judge` 时，裁判只在最后一轮之后跑一次，不是每轮一次。**

冻结时没说。理由是钱：用户敲的是**一个** flag，每轮一次 = N 倍入场费，
那不是他要的。judge 在最后一轮之后取的快照，`upTo` 已经涵盖前 N 轮的全部结论。

理由是本项目对「一轮」的定义：**一份快照、一个 `upTo`、一组成员**。
judges 的 `upTo` 按 4.7 必须是重取的，那它就已经不是同一轮了。
硬凑成同一个 `round_id`，等于让 `turn_usage.round_id` 这一列
再也分不出「谁在发言、谁在裁决」——而分得出来是有用的。

**代价（写进屏幕文案，不藏）**：用户敲了**一条** `/round --with-judge`，
事实源里出现**两轮**。所以屏幕上的两条结算行必须**各自带标签**
（如 `发言轮结算` / `裁判轮结算`），不许并成一行、也不许只印后一条。

#### 4.6a.8 片 3d 的验收

1. 不带 `--go`：印出 Budget 表，**一个进程都不起**（断言 Spawner 调用数不变）。
2a. **（turn 包，直接调 API）**预报与执行共用同一份快照：
   `snap := TakeRoundSnap` -> `BudgetWith(snap)` -> **`SetRules(新规则)`** ->
   `RunRoundPrepared(snap)`，断言这一轮送出去的身份块仍是**预报时那一份**。
   这是 API 契约本身，直接可测。
2b. **（repl 包，结构性）**`cmdRound` 整条命令只取一次快照。
   判据：**给显式 id**（这样 `lastUserMentionIDs` 不走）跑一次 `/round <id> --go`，
   假 Store 上的 `MaxSeq` 调用数**恰好 1**。
   已现查过：`internal/turn` 里 `MaxSeq` 只有 `TakeRoundSnap` 一个调用点，
   `readAfter` 走的是 `History`，`askUpToWith` 不调。
   错接法（先 `Budget()` 再 `RunRound()`，各拍一次）会是 **2**，一眼分得出。

   **不要把「中间插 SetRules」写成 repl 层的用例** —— `cmdRound` 是一个函数，
   测试塞不进它的第 5 步与第 8 步之间。那条由 2a 在 API 层担保。
3. `NoDelta` 的成员出现在 Budget 表上、且进了 `Plan.Skipped`、且**一个进程都没为它起**。
4. argv 超限：造一个 `ArgvHeadroom < 0` 的成员，断言**整轮零 spawn** 且点名了它。
5. `ArgvHeadroom == nil` 不触发整轮拒绝，且那一行印的是「余量未知」而不是数字。
6. **全员未报某个用量指标时，屏幕那一格不是 `0`。**（对着字符串断言，不是对着 Meta）
7. **`cache_read` 未知且 `cold` 计数为 0 时，屏幕不许裸印 `cold=0`。**
   这一条独立成用例，因为它是本片最容易错的一处。
8. `spawn_failed` 单独出现在屏幕上，不并进 `spawned`。
9. `--rounds 3`（给显式 id）跑三轮，断言：三个**互不相同**的 `round_id`；
   且 `MaxSeq` 调用数**恰好 3**（每轮重取一次快照，见 4.6a.6）。
   「第二轮之前改配置会被第二轮看到」这半条**不在 repl 层测** ——
   同 2b 的理由，快照新鲜度由 2a 在 API 层担保。
10. `--rounds 2` 不带 `--go` -> 拒绝。
11. `--with-judge`：judge 的 prompt 里含本轮 speakers 刚说的话
    （证明它的 `upTo` 是重取的，不是共用的）。
    **并断言事实源里有两对** `round_started` / `round_finished`，
    **且两对的 `round_id` 不同**（4.6a.7 的裁决）。
    屏幕上的两条结算行各自带标签。
    **`MaxSeq` 调用数恰好 2**（发言轮一次、裁判轮一次）。

11a. **裁判轮也有预算表，也过 argv 闸**（4.6a.7 订正 (2)）。
    判据：给一个 `ArgvHeadroom < 0` 的 judge，断言**裁判轮零 spawn 且点名了它**，
    而发言轮照常跑完 —— 两轮各自独立过闸，不是一损俱损。

11b. `--rounds 2 --with-judge`：**裁判只跑一次**，事实源里是 **3 对**面包屑
    （两个发言轮 + 一个裁判轮），`MaxSeq` 调用数恰好 3（4.6a.7 订正 (3)）。

13. **`plan.Skipped` 印在屏幕上**，每人带 `Kind` 与 `Reason`（4.6a.5a 第 1 条）。
    判据：造三档 `Kind`（`not_in_config` / `not_on_roster` / `mode`）各一个，
    断言三个 id 都出现、三行互不相同。

13a. **点名了裁判却没带 `--with-judge` 时，屏幕要点名他们。**

    （2026-09-22 补裁。原验收 13 把这件事与上一条混成了一条，
    实现方在落地 `renderSkipped` 时发现**它关不上这个缺口**并报了回来。）

    **`ModeJudge` 不进 `plan.Skipped`** —— `PlanRound` 把裁判收进 `plan.Judges`，
    `SkipMode` 只发给五值枚举**之外**的 mode（`plan.go` 的常量注释就写着
    「ModeJudge 不走这里，走 Judges」）。于是点名一个裁判、不带 `--with-judge` 时，
    他**既不在预算表上（`BudgetWith` 只为 Speakers 出行）、也不会被问、
    也不在 `Skipped` 里** —— 三处都碰不到，屏幕一个字不提。

    这正是 4.6a.5a 第 1 条要解决的那个「无声消失」，
    而 `renderSkipped` 只覆盖了它的一半。

    **裁决**：`cmdRound` 在 `PlanRound` 之后，当 `len(plan.Judges) > 0`
    **且没带 `--with-judge`** 时，印一行点名他们，并说明要用 `--with-judge` 才问。
    带了 `--with-judge` 时不印（他们会以裁判轮的形式出现在屏幕上）。

    **不改 `PlanRound`** —— 它把裁判归进 `Judges` 是对的，缺口在展示层不在分类层。

    判据：点名一个 `ModeJudge` 成员、不带 `--with-judge`，
    断言屏幕上**出现他的 AgentID** 且出现与 `--with-judge` 相关的提示；
    对照组：带了 `--with-judge` 时**不出现**这一行（而是出现裁判轮的预算表）。

14. **`forecastCold` 的口径**（4.6a.5a 第 3 条）：造一个 `Cold && NoDelta` 的成员，
    断言屏幕上的「预报冷启动数」要么不含他、要么那一行写明了口径。
    不许出现「预报冷启动 2、确知起进程 1」却一个字不解释。

15. **`Speakers` 被 `skipNoDelta` 挪空时提前返回**（4.6a.5a 第 4 条）：
    全员 `NoDelta` 时断言**一对面包屑都不写**，屏幕说清楚这一轮没人可问。
    事实源里不该出现「什么都没发生的轮」。

16. **顺序说明**（4.6a.5a 第 5 条）：同一个 id 给两次，断言只问一次，
    且屏幕上有一句说明顺序按字典序、重复已去重。

17. **阻塞提示**（4.6a.5a 第 6 条）：`--go` 起进程之前印出
    「扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）」。
    理由写串行的命令派发，不写 `r.mu`。断言这句在起进程之前出现。

18. **`另有 k 名未标定` 在 `k == 0` 时整行省略**（4.6a.3 补），
    断言全员已标定时屏幕上不出现这句。
12. 用户先说一句话再敲 `/round`：五个成员的 prompt 里都含那句话（4.6 已要求）。

**验收 2、7 是本片的两条主断言**，写用例时优先保证这两条有判别力：
2 挡住「两处各拍一次快照」，7 挡住「把不知道印成 0」。

### 4.7 Judge 的触发只有显式一条

`/round --with-judge`：`Speakers` 全部返回之后，**重新取一次 `MaxSeq` 作为 judge 的 `upTo`**,
再跑 `plan.Judges`；或单独的 `/judge` 只唤醒裁判。

judge 要看的是**本轮结论**，所以它的 `upTo` 必须是这一轮结束之后的,
不能与 Speakers 共用同一个快照 —— **这正是它不进 Speakers 的实际原因**,
不只是「角色不同」。

**不做**：不给 judge 特殊 prompt 模板、**不解析 verdict 语法**、不自动触发、不循环。
`04` §5 提到 relay 靠 `verdict:` 行推进，但那是 relay 自己的硬编码格式;
现在发明一套 verdict 语法就是编一个没验证过的契约,
而且一旦有代码按它分支，模型输出格式漂一次就静默停摆。
裁决先当普通 `KindAgent` 消息存着。

### 4.8 结算行与轮次面包屑

**结算行**：本轮唤醒 N 人（**只数真正起了进程的**，4.1）、总 in/out token、
报金额的那部分合计 USD、**不报金额的成员数**、实际冷启动数 vs 预报冷启动数、
`UnexpectedCold` 为真的成员点名。

**轮次要在事实源里留痕**（复核 missing 3，本契约的处置见 6.3）：
`RunRound` 开头与结尾各追加一条 `KindSystem` 面包屑
（`ReservedSenderSystem` + `Meta["event"] = "round_started"` / `"round_finished"`，
形状 `09` L#14 已定死），带上 `round_id`、`upTo`、成员名单、结算数字。
（X-36ab 之后 `round_finished` 另有无条件键 `cold_unknown`、文本末尾 ` cold_unknown=N`。**老行没有这个键，读作「早于 X-36ab、没记」，不是 0**。）
（X-38e `9729e94` 之前写的 `round_finished` 行，`usage_unknown` 只数 `UsageParsed` 为假的成员，**是下界**；键名没变，新老行只能按提交时间区分。）
**屏幕不是事实源，重启就没了。**

### 4.9 片 3 的验收

1. `PlanRound`：`mode=judge` 的成员在 `requested == nil` 下不出现在 `Speakers`、
   出现在 `Judges`；同一份输入跑两次名单逐字节相同；`Skipped` 每条都有可读理由。
2. 改 mode 不触发重建：把某成员从 `discussion` 改成 `judge`，
   `Speakers` 少了他、`Judges` 多了他，而 `identity_fp` **一个字没变**。
3. `RunRound` 真并发：假 Spawner 各睡 200 ms 跑 5 人，总墙钟 < 400 ms;
   期间往会话里 post 一条新消息，断言**这一轮 5 个人谁都没看到它、下一轮 5 个人都看到了**。
4. **一轮内确定性**（2.3）：同一份夹具跑两次 `RunRound`，假 Spawner 用不同的返回延迟顺序,
   断言每个成员两次拿到的 prompt 逐字节相同。
5. `UsageSamples`：造两条会话、同一个 agent、同一份指纹，两条都取得到;
   把其中一条的 `identity_fp` 改掉，就只剩一条。
6. **Budget 分两段验**（1.4）：
   (a) 对一条**已经跑过至少两轮**的会话预报第三轮，断言 `Basis` 指到一个真实
   `turn_id`、`Calibrated == true`、预报与实测并排印得出来;
   (b) 全新 team 跑一次，断言全部印「未标定」、不进合计、合计旁写明「k 名未标定」。
   **只做 (b) 等于没验。**
7. **除零守卫**：造一条 `prompt_bytes == 0` 的样本，断言它被跳过而不是崩或给出 Inf。
8. `no_delta`：连敲两次 `/round`，第二次断言那些成员进了 `Skipped` 的 `no_delta` 一档、
   **一个进程都没起**，且结算行的 N 不含他们。
9. **argv 超限整轮拒绝**（4.4）：造一个会让 grok 成员超限的历史,
   断言 `/round --go` **一个进程都没起**，错误文本点名是哪个成员、超了多少。
10. `--with-judge`：judge 那一轮的 prompt 里**含本轮其他成员刚产生的发言**
    （判据是送出去的字节）；不带 `--with-judge` 时 judge 一个进程都不起。
11. 结算行：三家各一个 agent 跑一轮，USD 合计**只含 claude 那一份**,
    且明写另外两名成员的 provider 不报金额。
12. **面包屑**：一轮结束后库里有 `round_started` 与 `round_finished` 两条 system 消息,
    且这一轮每条 `turn_usage` 的 `round_id` 都等于面包屑里那个。

---

## 5. 被否决的设计提案条目（逐条记，带理由）

| 提案的裁法 | 本契约的裁法 | 理由 |
|---|---|---|
| 片 3 第 0 个 commit 修 @mention token 边界 | **删掉，本步不做** | 已在 `11d` 8c-1 冻结，且 `11d` 1.2 把结论推得更远（Router 根本不解析 mention） |
| `Selector` 三态，`SelectMentioned{Text}` 在 `internal/turn` 解析文本 | 收窄成 `requested []string`，REPL 解析 | 否则打破 `11d` 1.2 的导入图闸（1.2） |
| `/round` 是本步新建的命令，用 `--yes` | 是 `11d` 8c-2 那条命令的**升级**，仍用 `--go` | 一条命令只能有一个确认 flag（1.1） |
| `/round [文本]`，先落库再打表 | **不收文本** | 「看了价格决定不发」做不到（事实源只追加）；不收文本则 `upTo` 顺序自动正确（4.6） |
| `Rebuild` 里继续 `delete(r.sess, key)` | **gate 永不从 map 移除**，改成持锁置 `g.st = nil` | 否则两条 goroutine 会同时跑同一个 `(conv, agent)`（1.3） |
| 「直接读 `r.sess[key]` 的那几处属于机械替换、不改断言」 | **要改断言**，其中一条会变成恒真的空跑 | `runner_reject_session_test.go` 那条（2.2） |
| `store_thought == false` 时打开 `thought_visible_to` 直接报错 | **不拦**，降成一行提示 | `store_thought=false` 不回溯历史，历史行的 thought 照样渲染进 judge 的 prompt；「恒为空串」是假的（3.2） |
| Budget 硬性「只许高估不许低估」，验收断言方向 | 方向**记录**不断言；预报按 `prompt_bytes` 缩放 | 取样规则结构上就偏低，且提案的验收在 0 行上成立（1.4） |
| Budget 验收只跑一个全新 team | **分两段**，先验有样本那一段 | 全新 team 全部「未标定」，断言恒真（1.4） |
| 失败轮不算样本 | **只要 `input_tokens` 非 NULL 就算** | `OutcomePostRejected` 钱花了、用量也记了，排除它往低估偏（1.4） |
| `maxParallel` 作为 Team 配置键「留好」 | **不加这个键**，只做函数参数 | 三片没有任何交付物读它，那就是死键（4.3） |
| 引 README 的「29537 / $0.118394」当入场费 | **README 那里是 47,749 / $0.955**；两个数量的不是同一件事 | 恰好证明「不许用常数」那条裁决（1.5） |
| 片 1 的理由「一次 /ask 在飞时 /session 被挡住」 | 理由改成「结构上允许并发，用例直接并发调」 | 单线程 REPL 下那两个场景不可达（2.1） |
| `/team use` 按 `includeInactive=false` 判「尚未加入」 | 按 `true` 拉全量，`left_at` 非空的跳过并点名 | 否则已 `/leave` 的人被静默拉回，每人一次入场费（3.5） |
| v4 形状声明照 v3 的写法 | 必须 `make` + `range` 拷贝 | `v3Shape = v2Shape` 是别名，照抄会改掉 v2/v3 的历史形状（3.3） |
| 复核：`r.mu` 让 `/session` 被挡 | 挡住的是**串行命令派发** | 与 `11d` 1.3 同一条订正 |
| 提案未提 argv 硬上限 | Budget 加一列余量，超限**整轮拒绝** | 扇出与 thought 同时撑大 prompt，会造成「从此说不了话」的死局（4.4） |
| 提案未提轮次面包屑 | `round_id` 列 + 两条 system 面包屑 | 屏幕不是事实源（4.8 / 6.3） |

---

## 6. 已知缺口与本契约新增的处置

### 6.1 `Mode` 从本步起第一次变成承重墙

未决 #7 的裁决（判别器是 `Mode` 不是 `Role`）此前是一条零行为改动的措辞订正;
片 3 的 scheduler 是**全仓第一处按 `Mode` 的行为分支**。
这件事要在 `00_DECISIONS.md` 未决 #7 那条下面记一笔。

### 6.2 gate 挡不住「同一个 agent 在两条会话里并发跑」

见 2.2 末尾。**本片不修**，只在 `gate.go` 的注释里写清楚边界。

### 6.3 `round_id`：本契约新加，提案里没有

复核指出「N 个成员并发 Post，seq 顺序由完成时刻决定，库里没有任何东西标记
这五条属于同一轮」。`11d` §7 已经把这条记成已知缺口。

**片 2 正好在做 v4 迁移，加一列比以后再迁便宜**，所以本契约把它加进去：

- `turn_usage.round_id TEXT NOT NULL DEFAULT ''`;
- **每一轮都有 `round_id`，`/ask` 是一轮之一**（不是让这一列对 `/ask` 恒空 ——
  那就又是一个半空的列）；
- 历史行保持空串，与 `identity_fp` 的空窗期同待遇（4.5），**不回填**。

面包屑与列**两样都要**：列让 `turn_usage` 能精确聚合，
面包屑让人在 `/log` 里读得出来这一轮是什么时候、对谁、送到哪个 `upTo`。

### 6.4 Ctrl-C 把「第三种结局」乘了 N，本地那一半已有处置，provider 那一半没有

`killtree_windows.go` 自己写着「尽力而为：taskkill 失败不把 Spawn 改判成功」。
一轮 5 个成员在飞时 Ctrl-C：本地会留 5 条 `turn_failed` 面包屑、5 个 `MaybeAdvanced`
（这一半 `11d` 3.5 的 `OutcomeCanceled` 已经处置），
但 **provider 那边可能有若干个没被杀干净的 CLI 进程仍在跑、仍会把那批 delta 吃进各自的会话**。
下一轮这 5 个人全部 `baselineSuspect`、成本差值全为 nil ——
也就是 **Budget 的样本池同时少 5 条**。

**本片不修**（要修得改 killtree 的语义，那是独立一片），但要：

- 在探针里加一条判据：**Ctrl-C 之后 10 秒，检查有没有遗留的对应 CLI 进程**；
- 把「一次取消会让整轮的成本记账失真」写进 `00_DECISIONS.md` 的未决区。

### 6.5 本步不做的

- **异步 round**（REPL 在扇出期间仍然阻塞）—— 独立一片。
- **`manager` 的行为** —— 与 `discussion` 同待遇，有意的剩余项。
- **verdict 语法** —— 不发明。
- **`maxParallel` 配置键** —— 等探针量出来再说（4.3）。
- **`/round` 之外的自动触发** —— `06` §5 的封顶纪律，与 `11d` 3.7 一致。

---

## 7. 探针（两条是前置，不是顺带）

| # | 测什么 | 何时 | 成本 | 判据 |
|---|---|---|---|---|
| P1 | 同一条 provider 会话真的没有被并发 resume | **片 1 的前置** | 零 | 两个 Spawner 区间不相交，**且第二次的 `req.ProviderSession` 等于第一次返回的 id** |
| P2 | thought 到底有没有进送出去的字节 | **片 2 的前置** | 零 | 两个方向各一次，判据是 argv 与 stdin 的字节 |
| P3 | N 个进程能不能同时拉起来（**不是「上限标定」**） | 片 3 开工前 | 零（三家都填不存在的 model 名） | 出现 spawn 失败且 stderr 指向资源类原因才算撞到上限；全程没有就明写「N <= 8 未见上限，未穷尽」 |
| P4 | Budget 预报的方向 | 片 3 收尾 | claude 一次冷启动量级 + 另两家订阅额度 | **分两段**（1.4 第 3 条）；方向记录不断言 |
| P5 | Ctrl-C 之后有没有遗留的 CLI 进程 | 片 3 收尾 | 零（搭在 P4 那一轮上） | 10 秒后进程表 |

P1 与 P2 **零成本且是前置**，在它们回来之前不许把片 1 / 片 2 的代码写死。
探针结果落 `E:\Isolating_test\council\step9-*\`。

---

## 8. 落地顺序与契约面登记

| 片 | 前置 | 契约面新增（**都要登记**） |
|---|---|---|
| 1 | P1 | `AskUpTo`（`09a`）、`SessionInfo.InFlight`、`ErrTurnInFlight` |
| 2 | 片 1；P2 | `Conversation.TeamID`、`Service.BindTeam`、`Tx.SetConversationTeam`、`TurnUsage.IdentityFP`、`TurnUsage.RoundID`、`ErrTeamUnknown` |
| 3 | 片 1、片 2；P3 | `Service.UsageSamples`、`Store.UsageSamples`、`Runner.RunRound`、`Runner.Budget` |

登记规格照未决 #9(a) 给 `Service.SetThoughtPolicy` 立的那条：
**新增方法同样是契约面动作，别当成零契约改动。**

**每一片单独一个 commit**，message 里如实记哪几条验收通过、哪几条没跑、为什么。

---

## 9. 符号禁令

代码文件里禁用装饰性符号：emoji（U+1F300–U+1FAFF）、dingbats（U+2700–U+27BF）、
杂项符号（U+2600–U+26FF、U+2B00–U+2BFF）、带圈数字（U+2460–U+24FF）、
U+FE0F、U+200D。注释、字符串字面量、变量名都算。
放行中文、希腊字母、数学算符、普通箭头 U+2190–U+21FF。
