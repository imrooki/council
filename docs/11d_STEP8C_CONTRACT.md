# Step 8 第三片 冻结契约：定向语义、有界扇出、配置重载、任务执行

日期：2026-09-21 · 机器：E 盘机 · 核对基线：`7ab093e`（第二片已落地并验收 4/4）

本文的全部 `file:line` 都是在 `7ab093e` 这棵树上逐条重开核过的。
`internal/turn/runner.go` 在 `7ab093e` 里有改动，但全部落在 `:768` 之后，
本文引用的行（`:405` / `:411` / `:470-480` / `:487` / `:498` / `:509` /
`:539-567` / `:617` / `:624`）一行都没移。

配套 `11_STEP8_SPEC.md` 第 4 节。前置 **第二片已落地**（`445b809`），
所以 `agentWorkDir(base, convID, agentID, tier)` 与 `caps.WriteBoundary`
都是现成的，8c-4 直接建在上面。

本文件是实现方与测试方**共同的唯一权威**。改本文件要留记录。

本片分四块，**顺序写死**：8c-1 -> 8c-2 -> 8c-3 -> 8c-4。
8c-1 是唯一 `isMinimalUseful` 的一块，其余三块各自依赖它。

---

## 0. 本片改动的包

| 块 | 包 | 改什么 |
|---|---|---|
| 8c-1 | `internal/repl` | 新文件 `mention.go`（**解析器落在这里，不在 `internal/turn`**，理由见 1.2）；`/to` 命令；`/log` 加 TO 列 |
| 8c-1 | `internal/turn` | `router.go` 删掉 mention 加投、加 `recipientOK`；`render.go` 的 `filterDelta` 加同一句；KindTask / KindTaskResult 在两条路径上各归其位 |
| 8c-1 | `internal/conversation` | `Service.Post` 加 Recipients 存在性校验 |
| 8c-2 | `internal/repl` | 新文件 `round.go` |
| 8c-2 | `internal/turn` | 成功路径落库改 `context.Background()`；新增 `OutcomeCanceled` |
| 8c-3 | `internal/repl` | 新文件 `configcmd.go` |
| 8c-3 | `internal/turn` | `Runner.SetConfig`；`drift.go` 那段注释**必须改写**（理由见 4.4） |
| 8c-4 | `internal/task`（新包） | `workspace.go`（`Prepare`）、`collect.go`（`Collect`） |
| 8c-4 | `internal/repl` | 新文件 `task.go` |
| 8c-4 | `internal/conversation` | `Post` 的脑子三列判据扩到 `KindTaskResult` |

---

## 1. 先说四条核查，它们改掉了设计提案里的前置

设计合议产出四块提案，对抗复核给的结论是「三块都改完可冻结」，
并点出 20 条核不实的证据。下面四条是我**逐条重开文件核过**的，
它们改变的是形状，不只是措辞。

### 1.1 claude 与 agy 一个轮数闸都没有（本次零成本实测）

原件 `E:\Isolating_test\council\oq-maxturns\RESULTS.md`。

| provider | `--max-turns` | 出处 |
|---|---|---|
| claude | **没有** | `claude --help` 全文 21695 字符，`turn` 只出现在 `--bg` 与 hooks 的描述里，没有任何上限项 |
| agy | **没有** | help 全文无 `max-turns` / `turns` / `iteration` / `rounds`；最接近的 `--print-timeout` 默认 `0s`，即一直等 |
| grok | **有** | `--max-turns <N>  Maximum number of agent turns` |

**两个被 `11c` 放行 WORKSPACE 的 provider，一个轮数闸都没有；
唯一有轮数闸的那家，恰恰是不允许写盘的那家。**

后果写进 8c-4 的注释，不许含糊成「有超时保护」：一轮任务里 agent 内部的
agentic 循环，Council 手上只有 `defaultAskTimeout`（墙钟 5 分钟）这一道闸。

顺带核实了另一条此前记 `[待核]` 的：grok `--worktree` 的 help 原文写着
**Headless (`-p`) does not create a worktree from this flag**。
`06` 第 3 节「跑在 WORKSPACE 档 + 独立 git worktree（grok 有原生 `--worktree`）」
是空的 —— Council 全程走 headless `-p`。8c-4 落地时改写那半句。

### 1.2 mention 加投在 Router 里其实是死代码

这条推翻了设计提案对 8c-1 的整个定性。

提案的 ruling #2 裁「`@id` 只加投、不收窄，**今天已经是这样**，只是匹配方式错了」，
ruling #4 裁「`@` **只在 `KindUser` 上解析**」。把这两条放在一起看 ——

`selectBootstrap`（`router.go:108-124`）的分支顺序是：

```go
if m.Kind == conversation.KindSystem { continue }      // :109
if m.Kind == conversation.KindUser   { add(m) }        // :112-114   无条件收
if mention != "@" && strings.Contains(m.Text, mention) { add(m) }   // :117   加投
if m.Kind == KindAgent && m.SenderID != selfID { lastBySender[...] = m }  // :120
```

`add` 按 `Seq` 去重（`:98-104`）。**全部 `KindUser` 已经在 `:112` 无条件收进去了**，
所以 `:117` 那次 `add` 对 `KindUser` 恒为一次 no-op。
「只在 `KindUser` 上解析」+「`KindUser` 本来就全收」= **这个分支什么都不做**。

也就是说：按提案自己的两条裁决，`router.go:117` 应当整段删掉，
而不是换一个更好的匹配函数。复核只说提案「把一次功能删除说成了匹配方式修正」，
没往下推这一步 —— 推到底，结论是 Router 根本不需要 mention 解析器。

**裁决：`internal/turn` 不解析 mention，一个字都不解析。**
`ParseMentions` 落在 `internal/repl`。

**为什么这个落点是结构性的**：`internal/repl` 导入 `internal/turn`，
所以 `internal/turn` 反过来导入 `internal/repl` 是一个**编译期的导入环**。
于是「agent 正文里的 `@x` 在路由上没有任何语义」这件事由 Go 的导入图保证，
不是靠一行注释、也不是靠 grep。ruling #4 想要的那道「比计数器靠前的闸」，
这是它能取到的最强形式。

Step 9 的 Scheduler 若要拿 mention 决定谁说话，必须**先把解析器搬出 repl**，
那是一次有 diff、有 review 的动作 —— 正好符合 ruling #1 的「封顶必须与触发同时到位」。

**顺带删掉一个死键**：提案要交付 `MentionsAgent(text, agentID) bool`。
Router 不用它之后，它零调用方。本片**不交付它**，只交付 `ParseMentions`。
（`11_STEP8_SPEC.md:50-53` 立过这个先例：不交付死键。）

**这一条还带来一个必须正面承认的行为变更**，见 2.2。

### 1.3 `r.mu` 持锁 × N 那笔账算错了，约束是 REPL 的串行派发

复核在 missing 里写「`/round --go` 串行扇出 5 个成员，最坏情况下 `/session`
被 `r.mu` 挡 25 分钟」。这笔账的机制是错的。

`runner.go:71-74` 那段注释自己写着：

> 代价是不同 agent 也被串行化。SessionInfo 取同一把锁，一次 /ask 在飞时
> /session 会被挡住最多 5 分钟（defaultAskTimeout）。**Step 6 的 repl 是
> 单线程读循环，观察不到这件事**；Step 9 扇出时要换成按 sessionKey 的锁。

我核过 `internal/repl`：只有 `readLines` 一条独立 goroutine，它只做阻塞读、
把行塞进 channel；**命令派发在主循环那一条 goroutine 上串行执行**。
所以 `/round --go` 在跑时，用户敲的 `/session` 根本轮不到被派发 —— 挡住它的是
串行的命令循环，不是 `r.mu`。

**用户看到的现象一样**（REPL 最长 N × 5 分钟没反应），**但原因不同**，
契约里必须写对的那一个。按 `r.mu` 写会让下一个人以为「换成按 sessionKey 拆锁
就解决了」，而那样改完 REPL 照样不响应。

`/round` 必须在 `--go` 之前把这句话印出来（见 3.1）。
Step 9 真要让 `/session` 在扇出期间可用，要动的是命令派发，不只是锁。

### 1.4 `11_STEP8_SPEC.md` 第 4 节那条冲突已经关闭，不要再改一次

提案的 conflicts 第 3 条要求「落地时把 `11` 第 4 节 thought 路由那一条改掉」。
我重开了 `11_STEP8_SPEC.md:478-486`，原文已经是：

> **thought 路由开关**（未决 #9(b)）。**这一条原文写错了地方**：它不属于
> 本步 @mention 那一片。开关长在 Team 配置上，而 Team 接进会话是 Step 9 ……
> 两件事混在一起排活会让第三片按错的前提干活。

它说的正是提案想让它说的话。按提案的处置去改，是把一段已经订正好的文字再改一次。
**本片对 `11` 第 4 节那一条不动。**

真正陈旧的是**行号**：`11:457` 与 `11:483` 都引 `router.go:115`，
现在实际是 `:106`（拼 `"@"+selfID`）与 `:117`（`strings.Contains`）；
`00_DECISIONS.md` 未决 #2 引的 `runner.go:289` 现在是 `:320`。
这两处 8c-1 落地时顺手订正。

---

## 2. 8c-1 定向语义与 mention token 边界

`isMinimalUseful: true`。其余三块都依赖它。

### 2.1 `ParseMentions`

**落点**：新文件 `internal/repl/mention.go`（理由见 1.2）。

```go
// ParseMentions 按首次出现顺序返回去重后的小写 mention token。
func ParseMentions(text string) []string
```

tokenizer 契约，三条写死：

1. **左边界**：`@` 的前一个字符必须是文本开头，或不属于 `[A-Za-z0-9_@.-]`。
   这一条专挡 `a@b.com` 这类邮箱，以及 `@@gk`。
2. **最长匹配**：`@` 之后按 `[A-Za-z0-9_-]` 取最长的一段。
   右侧**不再加额外边界条件** —— 最长匹配已经把右边界定死，
   再要求右边是空白或标点白名单，只会让句末的 `@gk.` 漏掉。
3. **ASCII 小写折叠后必须匹配 `internal/config/validate.go:10` 的
   `^[a-z0-9][a-z0-9_-]*$`**，否则不算一个 mention。
   折叠是安全的：agent id 被 `idPattern` 限制成全小写，
   折叠在 id 空间上是单射，不可能把两个不同的 id 折成同一个。

**`-` 与 `_` 这两个字符要单独裁一次**，它们是「最长匹配」与「用户想说什么」
唯一会分叉的地方：`.` 不在 token 字符类里，所以 `@gk.` 自然停在 `gk`；
而 `-` 和 `_` **既在 token 字符类里、又被 `idPattern` 的尾部允许**，
于是 `@gk-` 的最长匹配是 `gk-`，而 `gk-` 本身是一个合法 id。

**裁决：不剥尾部的 `-` / `_`，最长匹配说了算。** `@gk-` 解析成 `gk-`，不是 `gk`。

理由两条：
(a) 剥尾是一次**静默的猜测**，而一个真叫 `gk-` 的 agent 从此就点不到名了 ——
`idPattern` 确实允许这种 id；
(b) 这次「猜错」不会静默：2.7 的点名提示行会当场印
「`gk-` 不是当前会话的成员」，用户立刻看得见并改掉。
**让错误可见**比让程序替用户猜要好，这是本项目一贯的姿态。

golden 表必须把 `@gk-` 与 `@gk_` 各钉一行（见 2.8），
否则实现方与测试方会各自猜一个，而且可能猜得不一样。

**不交付 `MentionsAgent`**（见 1.2）。

### 2.2 Router：删掉 mention 加投，加 `recipientOK`

**这一项是一次行为变更，不是修 bug。契约正面写明它，实现方不许在注释里
自称「只是把子串匹配换成 token 边界」。**

`11_STEP8_SPEC.md:464-467` 对那个活 bug 的定性是「**只多投不少投**，
后果是 `gk` 的 delta 里多出不该看的消息、入场费多付一点」。
本片做的是把 `router.go:106` 与 `:115-119` **整段删掉**，
那是一次**少投方向**的语义收窄：一条 `KindAgent` 正文里含 `@gk` 的消息，
改动前会进 `gk` 的 bootstrap，改动后不会。

**为什么这是对的，理由不是循环保护，是成本**（循环保护那一层由 1.2 的导入图承担）：

`:117` 扫的是入参 `ms` 的**全量**，即 `[0, ScannedTo]` 闭区间。
一个成员之间习惯用 `@` 互相称呼的长会话里，`gk` 的 bootstrap 包会把
**历史上每一条提到过 `@gk` 的消息**全收进来，**没有任何上界**。
旁边 `:120` 那条「每个其他成员最后一条」按成员数封顶，这一条不封顶。
而 bootstrap 包就是入场费本身（D-5：claude 47749 / grok 20625 / agy 18180 token）。

新增包内谓词：

```go
// recipientOK 是 bootstrap 与增量两条路径**唯一**的可见性判据。
//
// 两条路径各写一份过滤，就是 Step 6 那个教训的复刻（router.go:37-39）：
// 「渲染进 prompt 的 seq」与「真正送出去的字节」会对不上。
//
// 空 Recipients = 广播（05 第 2 节），非空 = 定向。
// 注意这道收窄是**不可逆**的：一条消息在某个 agent 的流里被判为不路由，
// 游标照样推到 ScannedTo，它永远不会再送给它（router.go:93-94）。
// 这就是 /to 必须打那行提示的原因。
func recipientOK(m conversation.Message, selfID string) bool
```

- `len(m.Recipients) == 0` -> `true`
- 否则 `slices.Contains(m.Recipients, selfID)`

`filterDelta` 的**签名不变**，只在循环体里加一句 `if !recipientOK(m, selfID) { continue }`；
`selectBootstrap` 的循环体开头加同一句。

### 2.3 `KindTask` / `KindTaskResult` 在两条路径上各归其位

**这是复核点出的唯一一条阻塞级问题，提前到 8c-1 一并裁掉。**

`selectBootstrap` 今天的四条分支 —— 跳 `KindSystem` / 收全部 `KindUser` /
mention 加投 / 每个其他成员最后一条 `KindAgent` —— **`KindTask` 一条都不命中**。
而 8c-4 派的正是 `Kind=KindTask` 的消息。后果分两支：

- 若这是该 agent 在本会话的首轮，`len(selected)==0` -> `ErrNoDelta` -> refuse，
  一个进程都不起，任务永远不会开始；
- 若恰好另有别的 `KindUser` 消息，进程会起、钱会花，而 **prompt 里没有任务正文**。

而「第一次给刚提档的成员派任务」这条最主流的路径，结构上就是 bootstrap 轮：
`11c` 第 5 节明写把一个成员从 READ 提到 WORKSPACE 会改变工作目录、
改变前缀摘要、触发一次重建，下一轮必然 bootstrap。

**裁决**：

| Kind | `selectBootstrap` | `filterDelta`（增量） |
|---|---|---|
| `KindTask` | **无条件收**，与 `KindUser` 同一个分支 | 照旧（非 system、非自己即过）+ `recipientOK` |
| `KindTaskResult` | 并进「每个其他成员最后一条**发言**」那个槽位，与 `KindAgent` 共用一个槽 | 同上 |

`KindTask` 与 `KindUser` 同档，因为它就是一条有结构的用户发言（`SenderID="user"`）。
`KindTaskResult` 与 `KindAgent` **共用一个槽**而不是各占一个：
那条规则的全部价值在于 bootstrap 包的大小**按成员数封顶**，
给它开第二个槽就是把上界翻倍。规则改写成「每个其他成员在窗口内最后一条发言
（`KindAgent` 或 `KindTaskResult`）」，语义仍然可解释、仍然封顶。

### 2.4 `KindTask` 是广播，不是定向 —— 推翻提案 8c-4 的裁法

提案把 `KindTask` 定向给一个人，理由是「投了就是给每个成员的前缀里塞一段
与它无关的内容」。**这条理由与提案自己的 ruling #2 相抵**，而 ruling #2 才是对的：

> 每人只看见 @ 自己的消息，那是 N 个 1:1 私聊，不是合议。

一条在本会话里派出去的任务，不是「与本任务无关的历史讨论」（`04` 第 2 节），
它是此刻最相关的那件事；`04:52` 还把「其他 agent 在本任务上已形成的结论」
列进应当路由的，而任务结论的前提就是任务本身。

复核还指出一个更难看的后果，它坐实了这条裁决：**任务轮的回答本来就是广播。**
`Runner.Ask` 构造落库消息时**不设 `Recipients`**（`runner.go:470-480`：
`ConvID` / `SenderID` / 脑子三列 / `Kind: KindAgent` / `Text` / `Thought` / `Meta`，
没有 `Recipients`）。所以按提案的形状，别人**看得见答案、看不见题**。

**裁决：`KindTask` 与 `KindTaskResult` 都是广播（`Recipients` 留空）。**

这条裁决还顺手消掉了复核的 correction 里那个代价最大的选项 ——
它建议「给 `Runner.Ask` 增加本轮落库消息的 Recipients 这一条（契约新增，
要在 `09a`/`11a` 登记）」。广播之后**不需要动 `Runner.Ask` 的冻结签名**。

于是本片 `messages.recipients` 的生产写入方只有一个：`/to`。
这要老实记下来 —— ruling #5 的论证里把 `/task` 算作第二个写入方，
现在它不是了。留这一列的理由退回到两条：
删列要走一次**改表形状**的 schema v4（迁移框架今天只做过「形状不变、只改数据」
那一档，见 `11` 第 1.5 节），且要拆掉 `Service.Recipients` 这个 `08a` 冻结面上的方法；
而 Step 9 的 Judge / Manager 定向指令已经排进路线图。

### 2.5 `/to` 命令

```
/to <id>[,<id>...] <文本>
```

落一条 `Kind=KindUser`、`SenderID="user"`、`Recipients=[...]` 的消息。
四条既有发言路径（裸文本、`/say`、多行草稿、`/ask` 带文本）继续走广播，
形状不分叉 —— `/to` 只是给同一个构造点多传一个 `Recipients`。

**两道闸，缺一不可**：

1. **`Service.Post` 校验「存在」**（见 2.6）。
2. **`/to` 在 REPL 层校验「有活人看得见」**：
   `Recipients ∩ 当前活跃成员` 为空时**拒绝**，不落库，
   提示「这条消息不会有任何在场成员看到」。

第 2 道是复核 missing 里点出的缺口，必须补：
`Service.Recipients` 的既有口径明写「含已退出的成员：定向发给一个退了的 agent
仍是合法的解析结果」（`service.go:380-381`），那条口径立于**没有 Router 收窄**的年代。
加上 2.2 的收窄之后，一条只发给已退出成员的消息会落库、对全部活跃成员都过不了
`recipientOK`、而游标照样推到 `ScannedTo` —— **永久对谁都不可见，且没有任何一层会说话**。

第 2 道放在 REPL 而不是 `Service`，因为它判的是「人的意图」不是数据不变量，
而且「活跃」这个属性会被 `/leave` 改变（未决 #13 的 TOCTOU 窗口就在这里），
`Service` 那一层不该收一条自带竞态的判据。

落库成功后**必须打一行不可逆提示**：
「只有 gk2 会看到这条；其余成员永远不会 —— 游标只前进」。
这句不是修辞，是 `router.go:93-94` 的直接后果。

### 2.6 `Service.Post` 的 Recipients 存在性校验

落点：`internal/conversation/service.go` 的 `Service.Post`，
在 `Kind` / `SenderID` / 脑子三列那几条校验之后，**事务外**。

```go
if len(m.Recipients) > 0 {
    if _, err := s.Recipients(ctx, m); err != nil { ... }
}
```

直接复用已经存在、已有 5 条用例的 `Service.Recipients`，**零契约新增**。

**必须写进注释的论证**（否则下一个人会照未决 #13 的先例把它搬进事务）：
#13 拦的是「活跃」，那个属性会被 `/leave` 改变，所以有 TOCTOU 窗口；
这里校验的是「存在」，而 `Leave` 只置 `active=false` 并记 `LeftAt`，
participants 行从不删除 —— **存在性单调不减，这个口径下窗口不存在**。
搬进事务要解冻 `Tx` 接口，换来零表达力。

### 2.7 `/log` 的 TO 列与点名提示行

`/log` 加一列 TO：`[]` 时印既有占位符，非空时印逗号分隔的 id。
定向消息若在 `/log` 里看不出来，用户会以为大家都看到了。

点名提示行：`postUserText` 成功之后打一行
「点名了 gk2（用 `/ask gk2` 让他说话）」，不在成员里的 token 印
「gk9 不是当前会话的成员」。

**明记为 best-effort，不进契约、不阻塞、不影响落库结果。**
它在 REPL 层，别的写入方绕得开。

**这一处判定与 Router 的判定不是一个谓词的两份拷贝，不许将来有人去「统一」它们**：
这一处的输入是 participants 表（给人看的，回答「你点的这个名字在不在场」），
Router 的输入只有消息本身（纯函数、不读 participants，见 `router.go:90-92`，
回答「这条消息该不该进这个 agent 的包」）。两者回答的是不同的问题。

### 2.8 8c-1 的验收

**单测**

1. `ParseMentions` 表驱动 golden，至少钉住：
   - `@gk2` 解析出 `gk2`，且 `gk` 不在结果里（**这一行是本块存在的理由，必写**）
   - `@GK` 命中 `gk`
   - `a@gk` 不命中（左边界）
   - `(@gk)` 与 `@gk。` 命中（右侧不加边界条件）
   - `@@gk` 不命中
   - `@-gk` 不命中（首字符不合 `idPattern`）
   - **`@gk-` 解析成 `gk-`，`gk` 不在结果里**（2.1 的裁决：不剥尾）
   - **`@gk_` 解析成 `gk_`，`gk` 不在结果里**（同上）
   - 重复 `@gk @gk` 只出一次，顺序按首次出现
2. **导入图闸**：`internal/turn` 不得导入 `internal/repl`（本来就是编译期保证），
   并加一条用例断言 `internal/turn` 的源码里不出现 `ParseMentions`。
   这条用例是 1.2 那道结构闸的看门人。
3. **行为变更的正面断言**：一条 `KindAgent` 正文含 `@gk` 的消息，
   **不在** `gk` 的 bootstrap 结果里。改动前它在。
   用例名里要有 `BehaviorChange`，不许写成 `FixesBug`。
4. **逐字节 golden 的前提要缩窄并写明理由**：
   造一批 `Recipients` 全空**且正文不含任何 `@token`** 的既有消息，
   Router 的挑选结果与本片改动之前逐字节相同。
   fixture 里为什么不能有 `@token`，注释要说 —— 因为第 3 条那个行为变更是有意的。
5. `recipientOK`：造一条 `Recipients=["gk2"]` 的消息，
   `gk` 的 bootstrap 与增量两条路径都挑不到，`gk2` 两条路径都挑得到。
6. **`KindTask` 进 bootstrap**：造一条 `Kind=KindTask` 的消息（广播），
   断言**每个**成员的 bootstrap 结果都含该 seq，且 `Render` 出的正文含任务描述。
   再造一条 `Recipients=["gk"]` 的 `KindTask`，断言只有 `gk` 挑得到 ——
   定向能力仍在，只是 8c-4 不用它。
7. **`KindTaskResult` 与 `KindAgent` 共用一个槽**：
   同一个 sender 先发 `KindAgent` 再发 `KindTaskResult`，
   断言别人的 bootstrap 里**只出现后者**，不是两条都在。
8. `Post` 的 Recipients 校验：`Recipients=["ghost"]` 被拒、不落库
   （前后 `MaxSeq` 不变），错误里点名 `ghost` 且可被 `errors.Is(ErrParticipantNotFound)` 认出。

**真机**（判据是行为，不是退出码、不是模型自述）

9. `/to gk2 只给你看` 之后：`/ask gk`（无其它新消息时）得 `ErrNoDelta`；
   `/ask gk2` 的 `Delivered` 含该 seq；屏幕在 Post 当时就打了那条不可逆提示。
10. `/to <一个已 /leave 的成员> x` 被 REPL 拒绝、不落库，提示说「没有在场成员会看到」。
11. `/log` 那一行的 TO 列显示 `gk2`；`/say 问问 @gk2` 之后屏幕出现点名提示行，
    且消息照常广播（`/ask gk` 的 `Delivered` 含该 seq）。
12. **既有会话零漂移**：本块不动 `IdentityFingerprint` 的任何输入，
    落地后启动漂移审计应当报 0 条。

---

## 3. 8c-2 人工触发的有界扇出

依赖 8c-1（名单来自 `ParseMentions`）。

### 3.1 `/round` 命令

**落点**：`internal/repl/round.go`，**不进 `internal/turn`**。
扇出留在 REPL 层，`Runner.Ask` 的「一次一轮」语义一个字不改 ——
否则记账、指纹、超时、`r.mu` 都要在第二条路径上复制一份。

```
/round [id...] [--go]
```

- **名单**：不给 id 时取「当前会话**最后一条** `KindUser` 消息里的 mention
  ∩ 当前活跃成员」。名单为空则**拒绝并说明**，**绝不默认全员**
  （默认全员 = 一次敲击起 N 个进程、付 N 次可能的入场费）。
- **默认只打印计划，不起任何进程。** 加 `--go` 才真跑，串行。
- 逐轮之前检查 `ctx.Err()`，非 nil 就停下并报「已跑 k/N 轮，其余取消」。

这个两步形状照抄 `/rules reload [--rebuild]` 的先例
（`11` 1.7.1：一条命令可能一次烧掉几个入场费，那种事必须是用户按下去的）。

**订正一条提案里的说法**：提案写「`/rules reload [--rebuild]` 的两步形状
已经落地并真机验过」。`.handoff/HANDOFF.md:1028-1032` 的原文是
真机验过「只报告」那一支，**紧接着写着「仍未真机验的只剩 `--rebuild` 那一支」**。
要花钱的那一半恰恰是没真机验的那一半，「零发明」的底气比提案写的薄一层。

### 3.2 冷启动预报：`ok == false` 必须算冷

**提案这一条会把最贵的一档报成暖。**

`Runner.SessionInfo`（`runner.go:539-567`）在 agent 于本会话从没开过口时
（内存没有、库里也读不到）返回 `SessionInfo{}, false` —— `.ColdExpected`
是零值 `false`。而「从没开过口」正是下一轮必然 bootstrap、必然付整笔入场费的那一档。
同一函数 `:565` 的 `coldExpectedAt(now, st.lastTurnAt, false)` 第三个参数
（`fresh`）也恒为 `false`，它从设计上就不表达 bootstrap。

于是一支全新组队的 `/round gk cl` 会打出「将起 2 轮，其中 **0** 轮预报冷启动」，
而正确答案是 2。用户据这一行决定按不按 `--go`。

**裁决**：`/round` 必须把 `ok == false` 当成**必冷**，只有 `ok == true`
才读 `.ColdExpected`。

**这一行的措辞也要限定**：`ColdExpected` 已实测**不能预测** `Cold`
（未决 #1 那次「预报=冷、实测=暖」）。它偏保守，是**价格上界，不是估价**。
屏幕上印「预报」，不许印一个美元数。

### 3.3 REPL 在扇出期间不响应（按 1.3 的正确机制写）

`--go` 之前必须印一行：
「扇出期间 REPL 不接受任何命令，最长 N × 5 分钟（每轮的 `defaultAskTimeout`）。」

**理由写命令派发是串行的，不要写 `r.mu`**（见 1.3）。
`AuditDrift` 不取 `r.mu`（`drift.go:24-28`）这件事在这里不构成例外 ——
`/rules` 与 `/config reload` 同样轮不到被派发。

### 3.4 前置修复一：成功路径落库改 `context.Background()`

**这条是复核 missing 里最要紧的一条，本片必须修，它不只影响 `/round`。**

`Runner.Ask` 的**成功路径**用调用方 ctx 落库（`runner.go:487`
`r.svc.RecordTurn(ctx, &msg, u, session)`），失败后的两次补救同样用它
（`:498` `postSystem`、`:509` 第二次 `RecordTurn`）。
而 `failClosed` 已经改用 `context.Background()`（`:617`、`:624`），
注释写明理由：

> 超时那一支调用方的 ctx 已经死了。用它发面包屑会连这条 turn_failed 一起丢掉。

于是：进程成功返回、钱花了、provider 那边真多了一轮，此刻 ctx 若已取消
（`main.go:62` 的 `signal.NotifyContext`，一次 Ctrl-C 就够），
`RecordTurn` 失败 -> `OutcomePostRejected` -> 补救的两次也用同一个死 ctx ->
一起失败 -> **provider 会话 id 一个字没落库**。
这正是未决 #8 逐条查过的那种孤儿会话：它在对面真实存在、钱花了，
本地再没有任何一行指向它，没有任何命令列得出来。

`/round --go` 把这个暴露面乘以 N。

**裁决**：provider 已经成功返回之后的全部落库（`:487` 的 `RecordTurn`、
`:498` 的 `postSystem`、`:509` 的第二次 `RecordTurn`）一律改用 `context.Background()`，
与 `:614-616` 同一条理由，注释指过去。
本地 SQLite 写是亚毫秒级的，拿不到取消能力不构成风险。

### 3.5 前置修复二：新增 `OutcomeCanceled`

`runner.go:411` 把三件事并成一个分支：

```go
if killed || errors.Is(spawnErr, context.DeadlineExceeded) || errors.Is(spawnErr, context.Canceled) {
```

-> `failClosed(..., OutcomeTimeout, maybeAdvanced=true, ...)`。

于是**用户按 Ctrl-C 与一次真实的 5 分钟超时在库里逐字节同形**。
账上永远分不清「我打断的」与「它跑不完」。

**裁决**：新增 `OutcomeCanceled`，判别式是**父 ctx**：

```
ctx.Err() != nil   -> OutcomeCanceled   （取消来自上层，即信号）
否则                -> OutcomeTimeout    （只有 spawnCtx 到点，即墙钟）
```

`spawnCtx` 是 `context.WithTimeout(ctx, timeout)`（`:405`），所以这个判别式是精确的。
两者**都保持 `maybeAdvanced=true`** —— 进程跑过、结局未知，暴露面完全一样，
变的只是标签，让记账分得开。

**契约面改动**：`Outcome` 的常量加在 `const` 块末尾，已有取值的 iota 位置不许动
（库里已有行按**数字**存）。

**但「末尾」不够，必须钉死数字。** `13_STEP10_CONTRACT.md` 第 3.6 节也要加一个
`OutcomeLost`，也写「加在末尾」。两个 const 块的追加者互不知道对方,
而 `13` 第 1.1 节明确允许 Step 10 的 A 片提前落地 —— 落地顺序一反，
两个值就对调，**而 `turn_usage.outcome` 存的是整数**。
两台机器按不同顺序迁移出来的库会对同一个 `8` 有不同的理解。

**裁决：现在就在 `09a` 把两个值钉成字面量**，两份契约都引那个数字、不引「末尾」：

```
OutcomeCanceled Outcome = 8   // 本契约 3.5
OutcomeLost     Outcome = 9   // 13 第 3.6 节
```

（今天的块是 `Unknown=0 ... Refused=7`，本机核过。）
这与 schema 版本号撞车（`13` 1.1）是同一类问题 —— 两个并行的追加者，
一个共享的整数空间。
`internal/repl/ask.go` 的结局渲染要加一行。
这条要在 `09a` 留订正记录，与未决 #9(a) 给 `Service.SetThoughtPolicy` 的登记同规格。

### 3.6 Ctrl-C 会腰斩当前轮，契约明写，不假装

**needsProbe #6 已跑（2026-09-21），原件
`E:\Isolating_test\council\oq-ctrlc\RESULTS.md`。** 本节原来的三步里
**第 3 步的措辞是错的**，下面按实测改写。

做完 3.4 与 3.5 之后，`/round --go` 期间按一次 Ctrl-C 的真实行为是：

1. `main.go:62` 的 `NotifyContext` 取消 ctx；
2. 在飞的那一轮的 `spawnCtx` 跟着死 -> `OutcomeCanceled`、
   `maybeAdvanced=true`、面包屑经 `Background()` 落库（3.4）。
   **实测 `wall_ms=11426`，那一轮确实被腰斩**，不是「让它跑完」；
3. `/round` 在 return 之前打出「已跑 k/N 轮，其余取消」；
4. **然后 council 整个退出。**

**第 4 步是本节原文漏掉的，而它改变这条命令的用法。** 实测屏幕最后是
`council>` 换行 `再见`，然后回到 shell 提示符 —— `Run` 的循环发现 ctx 死了就
打告别行、返回、进程结束。原文写「停下，报『已跑 k/N 轮』」读起来像是停下之后
REPL 还能接着用，**不能**。那句话仍然打得出来，但打完用户就回到 shell 了。
帮助文本与那句提示要把这一点说清楚，别让人以为按 Ctrl-C 只是「取消这次扇出」。

**另一条实测补充：子进程是被控制台的信号直接打死的，不是只被我们的 ctx 杀掉。**
grok 的退出码是 `3221225786` = `0xC000013A` = `STATUS_CONTROL_C_EXIT`，
说明 **Windows 把 Ctrl-C 送给了整个控制台进程组**。

对扇出的两个后果，都要写进 `/round` 的注释：

- 一轮 N 个成员在飞时按 Ctrl-C，**N 个子进程会同时收到信号**，
  Council 不需要逐个去杀；
- 反过来，**Council 也无从选择「只停循环、让当前轮跑完」** ——
  那不是它能控制的层。`12` 第 1.1 节那张表里「取消」一行照此理解。

**契约不假装「让循环在本轮结束后停下」。** 第一次 Ctrl-C 就是腰斩。
`main.go:65-77` 的第二次 Ctrl-C 是 `os.Exit(130)` 硬退。

**「已跑 k/N 轮」这句里，被腰斩的那一轮算「已跑」**（钱花了），
但屏幕必须在它后面补一句它的 outcome 是 `OutcomeCanceled`。
不许只印一个数字。

**验收判据是那一行的 `outcome`，不是 `turn_usage` 的行数** ——
提案写的「断言 turn_usage 只多了一行」在「第一轮跑完、第二轮没起」与
「第一轮被腰斩、第二轮没起」两种结局下都成立，测不出它要测的那一种。

**needsProbe #6 已完成**（2026-09-21，grok 经 CPA，接近零成本），
原件 `E:\Isolating_test\council\oq-ctrlc\RESULTS.md`。
它是 8c-2 的前置而不是「顺便验一下」，因为本仓库今天**没有任何一条用例
覆盖过 REPL 的中断路径**。

**实测坐实了 3.5 的立论**：`turn_usage.outcome = 5`，也就是 `OutcomeTimeout` ——
一次用户 Ctrl-C 与一次真实的 5 分钟墙钟超时**在库里是同一个整数**。
此前这条是从代码推的（`runner.go` 把 `killed || DeadlineExceeded || Canceled`
并成一个分支），现在有了行为证据。

**探针还量出两件本节原本没有的东西**：第 4 步（council 整个退出）与
子进程收到 `STATUS_CONTROL_C_EXIT`，都已写进上面。
另有两件不归本片、已分别转走：孤儿会话 id 不进事实源（归 Step 10 A，
见 `13` 第 3.6 节）、失败轮把「不知道」的用量写成了 0（新未决）。

### 3.7 自动链封顶：只写规格，不交付键

`06` 第 5 节要求每条自动调用链有最大轮数、超时、取消、循环保护。
**本片不引入任何自动链**，所以四件按现有结构兑现，**不加任何配置键**：

| 要求 | 本片的落点 |
|---|---|
| 最大轮数 | 「人按一次 `/round --go` 跑一遍名单」。将来的自动链入口必须收一个**显式**上限，**没有显式上限一律 refuse** —— 不给默认值，「默认 3」是从 relay 借来的数，本项目一个常数都没标定过 |
| 超时 | 既有的 `defaultAskTimeout = 5 * time.Minute`（`runner.go:23`，出处与理由写在 `:17-22`） |
| 取消 | `/round` 逐轮 `ctx.Err()` + 3.5 的 `OutcomeCanceled` |
| 循环保护 | **`internal/turn` 不解析 mention，由导入图保证**（1.2） |

验收（文档判据）：`grep -rn "max_auto_rounds\|MaxAutoRounds" internal/` **无命中**，
确认没有交付死键（`11:50-53` 的先例）。

### 3.8 8c-2 的验收

1. 上一条 user 消息没有点名时 `/round` 拒绝，且**一个进程都没起**（库里无新 `turn_usage` 行）。
2. **全新组队**（两个成员都从没开过口）`/round gk cl` 不带 `--go`：
   打印「将起 2 轮，其中 **2** 轮预报冷启动」。这条是 3.2 的直接断言，必写。
3. 一个暖成员 + 一个新成员：预报 1 冷。
4. `--go` 时两条 Record 依次打出。
5. **取消**：`--go` 跑第一轮时中断，断言那一行的 `outcome` 是 `OutcomeCanceled`
   （不是 `OutcomeTimeout`、不是行数），且第二轮没起，且**库里仍有那条
   `provider_sessions`**（3.4 的直接断言）。
6. `grep -rn "max_auto_rounds\|MaxAutoRounds" internal/` 无命中。

---

## 4. 8c-3 动态 agent = `/config reload [--apply]`

依赖 8c-1（不强，但排在它之后，避免同一批文件两次改）。

**裁决先写清楚**：**不做「运行时凭空新增一个 agent」这条独立路径。**
新增 agent 的路径是「改 `config.toml` -> `/config reload --apply` -> `/join`」。

否决 `/agent add <id> --provider ... --model ...` 的理由：
它要新增一套运行时校验（与 `Config.Validate` 重复）、要决定写不写回 `config.toml`
（写回就是让程序改用户手写的 toml，会丢注释与格式；不写回就是第二事实源，重启即消失），
换来的便利只是省下一次编辑文件。

### 4.1 `/config` 与 `/config reload [--apply]`

落点：`internal/repl/configcmd.go`。

- `/config`：打印当前配置文件绝对路径与 agent / provider / team 计数。
- `/config reload`：重读同一个路径、跑 `Config.Validate()`、
  用**新**配置对 `provider_sessions` 全表算一遍指纹，报四类：
  会漂移的成员、配置里新增的 agent、配置里已消失但仍是会话成员的 agent、脑子三列有变化的 agent。
  **默认只报，不换任何状态。**
- `--apply`：才真的换。

**订正（2026-09-21，开工前核出）**：本节原文写「给 `drift.go` 的
`auditDriftAgainst` 加一个收 `*config.Config` 的兄弟函数」。
**那会交付一个没人调用的私有函数** —— `internal/repl` 够不到包内私有的东西，
而 `AuditDrift(ctx)` 是 `11a` 3.5 冻结的无参签名。这正是本契约驳过三次的死键。

**照 `/rules reload` 的既有先例办，不加任何新函数**：
`internal/repl/rules.go` 的注释已经把同一个抉择记录在案 ——
「`auditDriftAgainst` 是包内私有的，repl 够不到，**而且够不到也没关系**」，
它改用已经在契约里的公开方法 `SetRules` 临时换上去、调一次 `AuditDrift`、
再用 `defer` 换回来（用 `defer` 而不是在每个 return 前手写，
「那种写法只要漏掉一处 return，预览就会把探测用的规则悄悄留在 Runner 里」）。

`/config reload` 用**同一个形状**：`SetConfig(新)` -> `AuditDrift` ->
`defer SetConfig(旧)`。于是本块的 `internal/turn` 改动只剩 `SetConfig`
与 4.4 那段注释，一个新函数都不加。

**四类报告里有两类 `AuditDrift` 答不上来**，要 REPL 自己比对新旧两份
`cfg.Agents` 算出来：「配置里新增的 agent」（它还没有 `provider_sessions` 行）
与「配置里已消失但仍是会话成员的 agent」。这两类不进 `internal/turn`，
它们问的是配置差异，不是会话身份。

**账要算准**：指纹是 per-agent 的（`session.go:98-106` 七段全部是 per-agent 的输入），
`identityFPFor` 逐 agent 计算，所以改一个 agent 的字段只漂它自己。
**全队一起漂只发生在改规则文件时**，那条路走 `/rules reload`，与本块无关。
任务书里「改一个字就可能让全队重建」这个前提对配置热重载**不成立**，
照搬它会得出「热重载太贵、不做」这个错误结论。

### 4.2 `--apply` 必须换掉**全部**持有者

**提案这一条留了一个静默的脑裂，而它自己的验收恰好测不出来。**

`internal/app/app.go` 把**同一个 `*config.Config` 指针**分别交给
`NewRunner`（`:139`）与 `repl.New`（`:159`）—— 开工前在 `10332ec` 上重核过，
原文写的 `:146` / `:166` 是漂掉的旧行号，但**事实成立**：全仓只有这两个持有者。
只换 Runner 手里那份的话：
`/agents`、`/providers`、`/teams`、`/join`、`/who` 全部继续显示旧配置，
而 `/ask` 已经按新配置跑。

提案的验收写「把 `data_dir` 改掉之后 `/config reload` 拒绝，
且 `/agents` 仍列出旧配置（断言状态一个字没换）」—— 这条**恒真、测不出东西**：
`/agents` 读的是 REPL 自己那份指针，reload 成功与失败，结果一模一样。

**裁决**：`--apply` 在同一次命令里换掉两处 —— `Runner.SetConfig(cfg)` 与
REPL 自己的 `r.cfg`。两处都在 REPL 那条 goroutine 上，是一个不可分的步骤。
**不允许只换一处并把它登记成已知缺陷。**

### 4.3 根级键变化的 fail-closed 处置

| 键 | 处置 |
|---|---|
| `data_dir` | **整个 reload 拒绝**，一个字都不换 |
| `rules_file` | **整个 reload 拒绝**，提示「规则正文改动请用 `/rules reload`」 |
| `store_thought` | 允许；`--apply` 时连带调一次 `svc.SetThoughtPolicy`（与 `app.go` 装配点同一个调用） |
| `providers.*.command` | 允许，但**必须响亮告警**（见下） |

`data_dir` 在启动时已经决定了打开哪个库、在哪个路径上持有 `council.lock`
（未决 #8 已落地的那把锁）。reload 换掉它等于让进程内两份状态指向两个库，
而锁还在旧的那个上。

`providers.*.command` 那条告警的理由，是一个**指纹补不到的洞**：
`session.go` 的 `PrefixArgsDigest` / `normalizedPrefixArgs` 只摘 `cmd.Args`，
而 `Command.Path` 是 `runner.go:388-391` 在 `BuildTurn` **之后**从 `cfg.Providers`
回填的，**从来不进摘要**。把 grok 的 exe 换成另一个版本、或把 claude 从 `.cmd`
换成别的垫片，前缀完全可能真的变了而指纹一个字不动。
告警文本要建议 `/session rebuild`。

这是未决 #18 同类的一个洞，**本片只给告警，不修**：
修它要动 `PrefixArgsDigest` 的定义域，那是 `11b` 的冻结面。
按 `[代码证实、未经真机]` 记进 `00_DECISIONS.md` 的未决区。

### 4.4 `drift.go` 那段注释**必须改写**，它是不取锁的唯一依据

`drift.go:24-28` 今天写着 `AuditDrift` 不取 `r.mu` 的理由：

> 它们要读的 r.rules 和 r.cfg 在本方法运行期间根本不会被 Ask 改写
> （改 r.rules 只有 /rules reload --rebuild 会做，**改 r.cfg 本步没有任何命令会做**）。

`/config reload --apply` 让最后那半句**当场失效**，而它是那段论证的支柱。

**裁决**：改写那段注释，换成真正的理由 —— **REPL 的命令派发是单 goroutine 串行的**
（1.3 核过），`AuditDrift`、`Ask`、`SetRules`、`SetConfig` 全部从那一条 goroutine 调，
它们之间不可能并发。`runner.go:103-104` 给 `SetRules` 写的就是这个理由
（「Runner 的全部方法本来就只从 REPL 那一条 goroutine 调用」），
两处措辞要对齐。

`SetConfig` 因此**与 `SetRules` 同待遇：不加锁**，注释写明同一条理由。
提案自己在同一个 deliverable 里既说「`--apply` 才**在 `r.mu` 下**换掉 `r.cfg`」
又说「与 `SetRules` 同待遇（**不加锁**）」—— 两句不能同时成立，这里定成后者。

`SetConfig` 是契约面新增，要在 `09a` 登记。

### 4.5 配置非法时的处置

`Validate()` 返回 `ValidationErrors` 时逐条打印（它已按 `Path` 字典序排好），
**旧配置原样生效，不部分应用**。

reload 是唯一一条让一份非法配置进到运行中进程的路。
启动时的处置是「写 Err、不进 REPL」，运行中不能照搬（不能把用户踢出去）。

### 4.6 8c-3 的验收

**判据一律是 `/ask` 的 outcome 与库里 `provider_sessions` 那一行在不在，
不是屏幕上那句话。**

### 4.6.0 一个漏掉的前提：真机验收**必须在进程运行期间改配置文件**

本节原文只写了判据，没写**怎么让差异出现**。落地时撞上了，补在这里。

`/config reload` 比的是「**进程内存里**的配置」与「**此刻磁盘上**的文件」。
所以：

- **启动前改文件是没用的** —— 进程一上来就把改后的版本加载进内存，
  reload 一个差异都看不到，六条验收会全部「通过」而什么都没测到；
- 第 4 条（改 `data_dir`）更糟：进程会直接**开到另一个空库上**，
  `/use <会话 id>` 当场失败，连前置都摆不出来。

**可行的做法**（原件 `E:\Isolating_test\council\step8c3-acceptance\drive.ps1`）：
每一条先跑一轮真实的 `/ask`（实测 4 到 26 秒）撑开一个时间窗，
后台在 t=2s 把新配置写进去，窗口关掉之后才轮到 `/config reload`。

**这个做法能被接受，靠的是 misfire 响亮**：时间窗没赶上的话，
`/config reload` 会打出「没有成员会因此漂移 / 新增 0 / 消失 0 / 脑子三列 0」
四行全零 —— 一眼看得出来，**不会伪装成一次通过**。
一个时间依赖的验收只有在「失败可见」时才允许用，这条要一并写进脚本注释。

**另外两条踩过的坑，一并记下**：

1. `/use` 收的是**会话 id 或 id 前缀**，不收标题。id 从种子那一轮的
   stdout 里取（`已建会话 <id>`），不要去查库 —— 本机 PATH 上的
   `python.exe` 是 Windows Store 的存根，跑起来退 49，不是真解释器。
2. 驱动脚本若含中文注释，**必须存成 UTF-8 带 BOM**。
   这与本项目「跨进程文件一律无 BOM」那条规则**方向相反**：
   那条管的是数据文件，而 `.ps1` 是 PS 5.1 自己当源码读的，
   没有 BOM 它按系统 ANSI（本机 GBK）解码，中文注释被解成乱码、
   乱码里凑出引号或花括号把语法拆坏，**报错还指在完全正常的行上**。

1. 改某 agent 的 `model`：`/config reload` 报「1 个成员会漂移」且**行为不变**
   （随后 `/ask` 仍按旧配置成功、不报 drift）。
2. `--apply` 之后同一个 `/ask` 被 `ErrIdentityDrift` 拒绝；
   `/session rebuild <id>` 之后下一轮走 Fresh 并成功。
3. **脑裂断言（4.2 的直接验收）**：在 `config.toml` 里**新增**一个 agent，
   `--apply` 之后 `/agents` **列得出它**。这条会红当且仅当只换了 Runner 那一份。
4. 把 `data_dir` 改掉：`/config reload` 拒绝，且**随后 `/ask` 仍能正常跑完一轮**
   （断言状态一个字没换 —— 用行为，不用 `/agents` 的输出）。
5. 把某 provider 的 `command` 改成另一个路径、`--apply`：
   屏幕出现那条告警，**且** `AuditDrift` 报 0 条漂移。
   这两件同时成立才证明告警确实在补一个指纹补不到的洞。
6. 把某 agent 的 `provider` 改成不存在的名字：`/config reload` 逐条打印校验错误，
   随后 `/ask` 仍按旧配置正常跑完一轮。

---

## 5. 8c-4 任务执行：派任务、拿回 diff、人审合并

依赖第二片（`11c`）与 8c-1（`KindTask` 的 bootstrap 归属，2.3）。

**这一块引入本项目第一个 provider CLI 之外的外部可执行文件依赖：`git`。**
本机已核：`D:\Install_path\Git\cmd\git.exe`，`git version 2.52.0.windows.1`。

### 5.1 任务副本由 Council 自建，建在 agent **现有**工作目录之下

落点：新包 `internal/task`，`workspace.go`。

```go
// dest = filepath.Join(agentWorkDir(base, convID, agentID, tier), "task-"+taskID)
func Prepare(ctx context.Context, repo, baseRef, dest string) (Workspace, error)
```

实现用 `git clone --no-hardlinks <repo> <dest>` + `git -C <dest> checkout <baseRef>`。

**不用 `git worktree add`**：worktree 目录里的 `.git` 是一个指向主仓库
`.git/worktrees/...` 的**文件**，agent 一 commit 就要写到 cwd 边界之外，
而两家能围住的 provider 的边界正好锚在 cwd 上
（claude 的 `--restricted`、agy 的 `--add-dir`）。
那会让「边界生效」与「任务能完成」变成互斥。clone 自包含、零跨界，是 fail-closed 的那一边。
`--no-hardlinks` 连硬链接都不用，防 agent 跑 `gc` 伤到主仓库 objects。
代价是完整拷贝，大仓库慢 —— 写进注释。

**为什么建在既有工作目录之下而不是给任务开一个新 cwd**：
`11c` 第 1 节实测（`oq-cwd-prefix`）其余参数逐字节相同、只换 `--cwd`，
`cache_read` 从 19072 掉回 128。**工作目录进模型可见的前缀。**
新 cwd = 每派一个任务付一次完整入场费，**并且**因为 `11c` 第 5 节把 workDir
收进前缀摘要而触发一次 `ErrIdentityDrift`。两笔账都是白付的。

（入场费的数字一律引 D-5：claude 47749 / grok 20625 / agy 18180 token。
19072 那个数是 `11c` 第 1 节 cwd 探针里那一次的 `cache_read`，
量的是那次探针的前缀，**不是入场费**。提案把两个数串了台，这里分开。）

**`Prepare` 对已存在的 `dest` 必须拒绝，不许覆盖。** 同一个 taskID 再派一次、
clone 中途失败留下半个目录，都走这条。

### 5.2 脏工作树：默认拒绝，`--allow-dirty` 才放行

**这条是复核 missing 里点出的，提案完全没讨论。**

`git clone` 拿的是 **HEAD**，不是用户的工作树。
用户有未提交改动时（本项目自己的常态），agent 拿到的是上一次 commit 的状态，
而用户说的是「在我的仓库上改这个」。产出的 diff 基于一个用户没在看的版本，
合并时冲突或静默覆盖，**而没有任何一层会说** —— 正是本项目一路在防的那类。

**裁决**：

- `/task` 在 clone 之前跑 `git -C <repo> status --porcelain`，
  **非空即拒绝**，提示「主仓库有未提交改动，clone 只会拿到 HEAD；
  先提交，或加 `--allow-dirty` 明确接受」。
- `--allow-dirty` 放行，但**仍然响亮告警**。
- **无论走哪一支，实际用的 base ref 的 commit sha 都要写进
  `KindTask` 消息的 `Meta` 与 `/task show` 的输出** —— 那是事后唯一能对得上的锚点。

默认拒绝而不是只告警，理由是这个失败模式静默且昂贵，而覆盖它只要一个 flag。

### 5.3 任务派发：一条**广播**的 `KindTask`，复用 `Runner.Ask`

```
/task <agentID> --repo <绝对路径> [--base <ref>] [--allow-dirty] <描述>
```

1. 先 `task.Prepare`；
2. 再 Post 一条 `Kind=KindTask`、`SenderID="user"`、**`Recipients` 留空（广播，见 2.4）**、
   `Meta={task_id, repo, base_sha, workspace, conv_id, agent_id}` 的消息，
   正文 = 用户描述 + 一行「你的工作副本在 `./task-<id>/`」；
3. 最后调既有的 `Runner.Ask(ctx, convID, agentID)`。

**不新增第二条起进程的路径** —— 否则记账、指纹、超时、`r.mu` 串行全要复制一份。

准备失败（`git` 不在 PATH、repo 不是仓库、目录建不出来、dest 已存在、
工作树脏且没给 `--allow-dirty`）一律在 **Post 之前**返回，**一个进程都不起**。

`messages.kind` 的 CHECK（`store/schema.go:68`）本来就含 `'task'` 与 `'task_result'`，
**零迁移**。

**轮数闸这件事要写进 `internal/task` 的注释**（见 1.1）：
claude 与 agy 都没有 `--max-turns`，一轮任务里 agent 内部的 agentic 循环，
Council 手上只有墙钟这一道闸。不许写成「有超时保护」。

### 5.4 结果回收：diff 落盘，消息只放统计

落点：`internal/task/collect.go` + `internal/repl/task.go`。

轮次结束后在副本里跑 `git -C <dest> add -A` 与 `git -C <dest> diff --cached`，
diff 写 `<dataDir>/tasks/<taskID>/result.diff`，
再 Post 一条 `Kind=KindTaskResult`、`SenderID=<agentID>`、**带满脑子三列**、
**`Recipients` 留空（广播）**、正文是「改了 N 个文件、+A/-B 行、文件清单、diff 路径」的消息。

diff 可以是几百 KB。整段塞进 Canonical Conversation 会被路由进别人的 prompt、
进 argv（grok 与 agy 的正文还在 argv 上，未决 #15 只关掉了整数量级那一段），
而且只追加不回收。统计进消息、正文进文件，是唯一能同时满足
「可审计」与「不撑爆前缀」的形状。

**连带改一条按 kind 立的规矩**：`Service.Post` 里
`if m.Kind == KindAgent && (SenderProvider == "" || SenderModel == "")`
那条判据扩成 `KindAgent || KindTaskResult`。
今天 `KindTaskResult` **不受** `ErrMissingSenderBrain` 约束，
一条不知道是哪个脑子干的活的 diff，正是用户要的那件事的反面
（未决 #14 那类「按 kind 立的规矩」的同一个缺口）。
这是契约面改动，要在 `08a` 留订正记录。

### 5.4a 收集路径加固（2026-09-22 裁，两个探针支撑）

**问题**：5.4 让 Council 在副本里跑 `git add -A` 与 `git diff --cached`。
git 的 filter / textconv 驱动由 `.gitattributes`（声明）+ config（命令）两半组成，
**两半都是模型用 Edit / Write 就能写出来的普通文件**，而 Council 不受限制。
实测两条命令各自都执行了模型给的命令，**一条错误都没报**
（`oq-task-git-filter\RESULTS.md`，问一 VULNERABLE）。

**裁决：收集时把 git 目录放到工作区之外，并关掉全局与系统配置。**

```
git --git-dir=<dataDir>/tasks/<taskID>/gitdir     --work-tree=<workspace>     add -A
git --git-dir=<dataDir>/tasks/<taskID>/gitdir     --work-tree=<workspace>     diff --cached
```

收集进程的环境必须带：

```
GIT_CONFIG_NOSYSTEM=1
GIT_CONFIG_GLOBAL=<一个空文件的绝对路径>
```

**承重的是一条结构性事实，不是枚举**：**filter 的命令必须来自 config，
而 config 归我们。** 工作树里的 `.gitattributes` 单独存在时指向一个不存在的驱动,
git 直接忽略。

#### 为什么选这条，不选「收集前校验 `.git` 未被动」

第二个探针（`oq-task-git-hardening\RESULTS.md`，两次独立运行一致）测了三条假设:

| 用例 | `git add -A` | `git diff --cached` | |
|---|---|---|---|
| 台子自检：根 `.gitattributes` + config | **HIT** | **HIT** | 复现漏洞 |
| `.gitattributes` 指向**未定义**驱动，无 config | NO | NO | 真惰性：rc=0、不报错、内容 `cmp` 一致 |
| 外部干净 `--git-dir` + 被投毒的工作树 | NO | NO | 本裁决的直接证据 |
| 排除 `.git` 拷树 + 全新 `git init` | NO | NO | 修法 2 同样成立 |
| **只有 `sub/.gitattributes`（子目录）** | **HIT** | **HIT** | **新执行面** |
| **只有 `.git/info/attributes`** | **HIT** | **HIT** | **新执行面** |

后两行是否决「收集前校验」的直接理由：按直觉实现成「检查根 `.gitattributes`
+ `.git/config`」，**这两条都绕得过去**。而且**前一个探针找到 2 个面、
这个探针又找到 2 个** —— 它证明的不是「只剩这两个」，
而是**枚举这件事本身不可靠**。校验是白名单式安全，本项目的 fail-closed 纪律反对它。

**修法 3 相对修法 2（干净 clone）的结构性优势**：工作区里**根本没有 `.git`**,
于是 `.git/info/attributes` 与 `.git/hooks/` 这两个面从「要检查」变成「写不出来」。
修法 2 是绕着被投毒的 `.git` 拷，修法 3 是从不创建它。附带好处是不用拷整棵树。

#### 全局 / 系统 gitconfig 是一条独立的缺口，双向验过

它**不是攻击面**（agent 写不到 `~/.gitconfig`），但若**用户自己的**全局配置里
本来就有 filter 定义，「干净的 git-dir」并不干净：agent 只需在工作树里写一个
指向那个名字的 `.gitattributes`。

| | `add -A` | `diff --cached` |
|---|---|---|
| 修法 3，**不加**两个环境变量 | **HIT** | **HIT** |
| 修法 3 + `GIT_CONFIG_NOSYSTEM=1` + `GIT_CONFIG_GLOBAL=<空>` | NO | NO |

**两个环境变量不是可选项。**

#### 连带改动

- **工作区创建**：clone 时把 git 目录分出去（`--separate-git-dir`，或 clone 后
  把 `.git` 移走）。要保证工作树**与普通 checkout 逐字节相同，只是少一个 `.git`**。
- **`/task drop`**：`gitdir` 与工作区**两处都删**，仍按 5.5 的规矩 ——
  逐文件删、路径必须在 `<dataDir>` 之下，`result.diff` 不删。
- **agy 的连带**：`11d` 5.6 记着 agy 在 WORKSPACE 下能不能执行命令**未测**。
  修法 3 下工作区没有 `.git`，**若 agy 能跑 git，它的 git 命令会响亮失败而不是
  静默做坏事** —— 这是 fail-closed 的那一边，不是缺陷。

#### 仍未验证（落地时不要写成已验证）

- 系统级配置在 Windows 上的实际位置与 `GIT_CONFIG_NOSYSTEM` 的覆盖面**没单独测**
  （只测了 `GIT_CONFIG_GLOBAL` 那一半有效）。
- `.gitmodules`、符号链接、`core.attributesFile` 指进工作树这几类**没测**。
  **没测不等于安全** —— 但修法 3 不依赖「我们想全了」，这几类若真能执行,
  命令仍须来自 config，而 config 归我们。
- `--separate-git-dir` 产生的工作树里会留一个 `.git` **文件**（不是目录）指向真 gitdir。
  那个文件是否可写、写坏了会怎样，**没测**。落地时要么测它，要么改用「clone 后移走」。

### 5.5 任务的事实源与生命周期

**`KindTask` 消息的 `Meta` 是唯一权威**，不另开表、不另落盘索引：

```
{task_id, repo, base_sha, workspace（绝对路径）, conv_id, agent_id}
```

- `/task list`：扫 `KindTask` 消息，从 `Meta` 读。
- `/task show <taskID>`：打印统计与 diff 的绝对路径。
- `/task drop <taskID>`：删副本目录，**逐文件删、路径必须在 `<dataDir>` 之下**，
  `result.diff` **不删**（那是产物，不是中间物）。

这样做的理由：`messages` 是只追加的事实源，副本目录按 `11c` 第 4 节天然绑在
`(convID, agentID)` 上，而 diff 绑在 taskID 上 —— 两处分居，必须有一个反向索引，
而 `Meta` 是唯一不引入第二事实源的落点。

**没有 `/task apply`。** 帮助文本里明写「合并请在你自己的 git 里做」。
`06` 第 3 节写的是「产出 diff，由人在合并前审阅」；本项目连 FULL 档都不给编排器用，
自动 apply 会让 council 变成一个能改用户主仓库的东西。这是裁决，不是省事。

### 5.6 needsProbe #3 已跑：问题问错了，而答案改写了本片的前提

**2026-09-21 实测**，原件 `E:\Isolating_test\council\oq-task-git\RESULTS.md`。

本节原文问的是「`--restricted` 对 `.git` 里 index / objects 的写入算不算普通
文件写」。**那个问题在 Council 的 WORKSPACE 档下不成立** ——
按契约跑下来，agent 的回答是：

> (1) 改 `./task-x/README.md` —— **成功**（用 Edit 工具）
> (2) `git -C task-x add -A` —— **Not run.** I have no tool available in this
> session capable of executing shell/git commands.

**它根本跑不了 git。**

#### 真正量到的：`--restricted` 同时拿掉 Bash 与 PowerShell

直接调 claude 做 A/B，一次只改一个变量（两组都有 `--safe-mode` 与
`--permission-mode acceptEdits`）：

| 组 | flag | Bash / PowerShell |
|---|---|---|
| A | **不加** `--restricted` | **都在** |
| B | 加 `--restricted` | **都没有** |

#### 写入边界与执行能力是同一个 flag，二选一

`11c` 第 2 节实测过：**不加 `--restricted` 就没有写入边界**
（用户 `settings.json` 的 `bypassPermissions` 会生效，READ 档都写得出文件）。

> **要写入边界就必须加 `--restricted`；加了它 agent 就不能执行任何命令。
> Council 给不了「能写盘又能执行」。** 这不是可以调参绕开的取舍，是 CLI 的结构。

#### 对本片的两个后果

**一、clone 方案成立，`[未验证]` 可以摘掉 —— 但理由不是契约原本设想的那条。**
5.4 的收集路径是 **Council** 跑 `git -C <dest> add -A` 与 `diff --cached`，
**不是 agent 跑**。所以 agent 只要能用 Edit / Write 在副本里改文件就够了，
而那一点已经验过。

> **撤回（2026-09-22）**：本段原文接着写的是「Council 是普通进程，**不受限制**」，
> 并把它当成这条路径的**安全理由**。**那句话作为安全理由必须撤回 ——
> 不受限制恰恰是它危险的地方。**
>
> 实测（`E:\Isolating_test\council\oq-task-git-filter\RESULTS.md`）：
> git 支持在 `.gitattributes` 里声明 filter / diff 驱动、由 config 给出命令，
> 而 `git add` 与 `git diff` 会**替调用者执行那条命令**。两者都是**普通文本文件**，
> 模型只用 Edit / Write 就写得出来 —— 而 `--restricted` 拦的是命令，不是文件。
> 整套写入边界拦得住 agent 自己执行命令，**拦不住它让 Council 替它执行**。
>
> 加固方案见下面 5.4a。**在 5.4a 落地之前，8c-4 不许落地。**

**二、WORKSPACE 档的 agent 只能编辑，不能验证 —— 这一条必须写进 `/task` 的
帮助文本与 `06` 第 3 节。**

- 跑不了测试、跑不了编译、跑不了 linter；
- 「派一个任务、拿回 diff」意味着 agent **盲改**：它写出来的东西，
  在人合并之前没有任何一层跑过；
- `06` 第 3 节把 Executor 描述成「改代码」的角色，读起来像是能自己迭代到通过。
  **不能。** 落地 8c-4 时改写那半句。

**两件仍按 `[未验证]` 记**：agy 在 WORKSPACE 下能不能执行命令没测
（它走 `--mode accept-edits --add-dir`，与 `--restricted` 不是一回事），
别默认三家一样；以及 `--restricted` 会忽略 settings 文件因而换掉默认模型
（本项目不受影响，Council 每轮显式发 `--model`，但记一笔）。

**needsProbe #3 的原始处方已作废，留在下面只为留痕，不要照它跑**：

> ~~在 `<dataDir>/workdir/<convID>/<agentID>/task-x/` 里 clone 一个小仓库，
> 让一个 WORKSPACE 档的 claude agent 在里面改一个文件并 `git add` / `git commit`。
> 判据是行为：`git -C <副本> log -1` 看不看得到那条 commit。~~

**它问不出答案**：那个 agent 根本没有能执行 git 的工具（本节开头就是这个发现）。
2026-09-21 的多模型复核（Codex gpt-6-astra）点出这段处方与本节结论自相矛盾，
核对属实。探针原件仍在 `E:\Isolating_test\council\oq-task-git\`。

#### needsProbe #3b（新的硬前置，方向对了）：agent 能不能**用文件工具**改 `.git`

原处方问的是「agent 能不能**跑 git**」，问错了。要问的是
**「agent 能不能用 Edit / Write 写副本里的 `.git/config` 与 `.gitattributes`」** ——
那是普通文件写，不需要任何命令执行能力，而 `--restricted` 拦的是命令不是文件。

**为什么它是硬前置**：本契约 5.4 让 **Council** 去跑 `git add -A` 与 `diff --cached`，
并把「Council 是普通进程、不受限制」当成安全理由。方向反了 ——
**不受限制正是它危险的地方**。git 支持 `.gitattributes` 里声明 filter 驱动、
由 `.git/config` 给出对应的命令，而 `git add` 会**替调用者执行那条命令**。
于是：模型写两个普通文本文件 -> Council 自己的收集步骤成为执行入口，
绕过整套写入边界。这条由 Codex 在复核里提出，本机**尚未复现**，
按 `[未验证、需探针]` 记，但**不许在证明收集路径安全之前落地 8c-4**。

判据是行为：让 WORKSPACE 档的 agent 在副本里建一个 `.gitattributes` 与
改 `.git/config`（两件都用文件工具，不用命令），看写不写得进去；
写得进去就再看 Council 侧一条 `git add -A` 会不会触发那个 filter。
若触发，收集路径必须改成在**隔离的 git 配置**下跑
（例如显式清空 `core.attributesFile`、`core.hooksPath`、filter/diff 外部命令），
或者在收集前校验 `.git` 与 `.gitattributes` 未被改动。

若 agent 连 clone 内部的 `.git` 都写不进去，这条风险自动消解，
但**那仍然要测出来才算数**，不许推定。

### 5.7 8c-4 的验收

**单测**

1. `Prepare` 对已存在的 `dest` 返回错误、不动那个目录。
2. `git` 不在 PATH 时 `Prepare` 响亮失败，错误文本点名 `git`。
3. 构造一条 `SenderProvider` 为空的 `KindTaskResult` 去 `Post`，
   必须拿到 `ErrMissingSenderBrain`。
4. 工作树脏且未给 `--allow-dirty` 时拒绝，**库里没有新消息、没有新 `turn_usage` 行**。

**真机**（判据是行为）

5. 给一个 WORKSPACE 档的 claude agent 派「在 README 末尾加一行 `TASK-OK`」：
   轮次结束后副本里 README 确实多了那一行。
6. `<agentWorkDir>/task-<id>/.git` 是一个**目录**（不是文件）；
   `git -C <主仓库> worktree list` 只有主工作树一行。
7. **成本判据（本块最关键的一条）**：任务轮与该 agent 上一轮讨论的
   `identity_fp` **第七段相同** —— 派任务没有触发重建。
8. 把同一个任务派给 grok：在**起进程之前**被拒，
   错误文本说的是「证明不了边界」（`11c` 3.2 的措辞）。
9. `repo` 路径不是 git 仓库时拒绝，且 `wall_ms=0`、库里没有新的 `turn_usage` 行。
10. `result.diff` 非空且只含预期的那一处改动；
    **主仓库 `git status --porcelain` 输出为空**（一个字节没动）。
11. 那条 `task_result` 消息的 BRAIN 列在 `/log` 里显示得出 provider/model。
12. **`KindTask` 是广播（2.4 的直接断言）**：派完任务后 `/ask` 另一个成员，
    它的 `Delivered` **含**那条 task 消息 —— 别人既看得见题，也看得见答案。
13. `/task show` 打得出 diff 的绝对路径且该文件可读；
    `/task drop` 之后副本目录不存在而 `result.diff` 仍在。
14. `grep -rn "apply" internal/repl/task.go` 不出现任何执行 `git apply` / `git am` 的代码路径。
15. **记录 `wall_ms`**（needsProbe #4）：搭在第 5 条那一轮上，零额外成本。
    看它逼不逼近 300000。`defaultAskTimeout` 的 5 分钟是按讨论轮定的
    （`runner.go:17-22`），任务轮量级可能不同。
    **在量到之前不许改那个常数**，改了就是编一个没标定过的数。

    **2026-09-21 订正**：本条原文写「任务轮是编辑-运行-再编辑」——
    **错的**，5.6 实测那个 agent 跑不了任何命令，任务轮是**纯编辑**，
    没有「运行」那一段。原措辞会让人以为 wall_ms 里含着跑测试的时间。
    这一条同样由 Codex 在复核里核出（与上面 5.6 那处是同一批残留）。

---

## 6. 被否决的设计提案条目（逐条记，带理由）

写在这里是为了不被重新提出来。

| 提案的裁法 | 本契约的裁法 | 理由 |
|---|---|---|
| `@mention` 解析器放 `internal/turn/mention.go`，Router 用它做加投 | 放 `internal/repl/mention.go`，**Router 一个字都不解析** | 按提案自己的 ruling #2 + #4 推到底，Router 那个加投分支是死代码（1.2） |
| 交付 `MentionsAgent(text, agentID) bool` | **不交付** | Router 不用它之后零调用方，是个死键 |
| 8c-1 是「把活 bug 修掉」，逐字节 golden 证明 no-op | 是一次**行为变更**，golden 的前提缩到「fixture 不含 `@token`」 | 两者不能同时成立；正面断言那个变更（2.8 第 3 条） |
| `KindTask` 定向给一个人 | **广播** | 与 ruling #2「不是 N 个 1:1 私聊」相抵；且答案本来就是广播，定向题目会让别人看得见答案看不见题（2.4） |
| 给 `Runner.Ask` 增加 Recipients 参数（复核的 correction） | **不动 `Runner.Ask` 的冻结签名** | `KindTask` 广播之后这个需求消失 |
| `/round` 读 `SessionInfo(...).ColdExpected` | `ok == false` 必须算**必冷** | 从没开过口的成员返回 `SessionInfo{}, false`，零值是 `false`，正好把最贵的一档报成暖（3.2） |
| 取消的验收「断言 `turn_usage` 只多了一行」 | 断言那一行的 **outcome** | 行数在两种结局下都成立，测不出要测的那一种（3.6） |
| Ctrl-C「让循环在本轮结束后停下」 | **明写第一次 Ctrl-C 就腰斩当前轮** | `main.go:62` + `runner.go:405`，行为如此；假装不如写清楚（3.6） |
| `SetConfig` 在 `r.mu` 下 / 又说不加锁 | **不加锁**，与 `SetRules` 同待遇，并改写 `drift.go` 的理由 | 提案两句自相矛盾；真实理由是 REPL 单 goroutine 串行派发（1.3 / 4.4） |
| `--apply` 只换 Runner 手里那份 | **两处一起换** | `app.go:146/166` 是同一个指针，只换一处 = 静默脑裂（4.2） |
| 验收「`/agents` 仍列出旧配置」 | 换成「新增 agent 之后 `/agents` 列得出它」 | 原验收恒真，测不出东西（4.2） |
| 复核：`/round --go` 让 `/session` 被 `r.mu` 挡 25 分钟 | 挡住它的是**串行的命令派发**，不是 `r.mu` | `runner.go:71-74` 自己写着「Step 6 的 repl 是单线程读循环，观察不到这件事」（1.3） |
| 提案：落地时改 `11` 第 4 节 thought 路由那一条 | **不动** | 那段已经订正过，说的正是提案想让它说的话（1.4） |
| 入场费记成「grok 约 19k」 | 一律引 D-5 的三个数 | 19k 是 `11c` cwd 探针那次的 `cache_read`，不是入场费（5.1） |

---

## 7. 已知缺口与未测项

本片**不修**，但必须写进注释或 `00_DECISIONS.md` 的未决区：

1. **`cfg.Providers[x].Command` 不进身份指纹**（4.3）。
   `PrefixArgsDigest` 只摘 `cmd.Args`，`Command.Path` 是 `BuildTurn` 之后回填的。
   换 exe 前缀可能真变而指纹不动。本片只给告警。
   按 `[代码证实、未经真机]` 记。修它要动 `11b` 的冻结面。
2. **`RenderDelta`（`render.go:24`）零个生产调用方**，只剩测试在用；
   生产路径走 `Router.Select` + `Router.Render`。8c-1 改 `filterDelta` 时它会跟着变。
   本片不删它，但注释要说一句，免得下一个人以为自己看错了。
3. **`inflight_turns` 没有上限、没有清理**（复核在 Step 10 那块点出的，与本片无关但同源）。
   留给 Step 10。
4. **轮次在事实源里不留痕**：`turn_usage` 有 `turn_id` 没有 `round_id`，
   `/round` 扇出的 N 轮在库里没有任何东西标记它们属于同一次。
   本片不加（加它要动 schema），但 `/round` 的实现里不许假装有。
   记进未决区。
5. **needsProbe #2（grok `--max-turns` 命中时 Council 归成哪个 Outcome）本片不跑**：
   grok 不允许 WORKSPACE，它不是任务执行的路径；
   这条留给 Step 9 真要用原生封顶时再跑。

---

## 8. 落地顺序与每块的完成判据

| 块 | 前置 | 完成判据 |
|---|---|---|
| 8c-1 | 无 | 2.8 全部通过；`go build` / `vet` / `test -count=1 ./...` 全绿；启动漂移审计 0 条 |
| 8c-2 | 8c-1；**needsProbe #6 已跑** | 3.8 全部通过 |
| 8c-3 | 8c-1 | 4.6 全部通过 |
| 8c-4 | `11c`；8c-1；**needsProbe #3 已跑且结论是「写得进」** | 5.7 全部通过 |

**每一块单独一个 commit。** commit message 里如实记哪几条验收通过、
哪几条没跑、为什么 —— 与第二片同规格（`445b809` 里 agy 那条的写法）。

**探针先于实现**：needsProbe #3 与 #6 的结论会改变 8c-4 与 8c-2 的形状，
在它们回来之前不许把那两块的代码写死。

---

## 9. 符号禁令

代码文件里禁用装饰性符号：emoji（U+1F300–U+1FAFF）、dingbats（U+2700–U+27BF）、
杂项符号（U+2600–U+26FF、U+2B00–U+2BFF）、带圈数字（U+2460–U+24FF）、
U+FE0F、U+200D。注释、字符串字面量、变量名都算。
放行中文、希腊字母、数学算符、普通箭头 U+2190–U+21FF。
