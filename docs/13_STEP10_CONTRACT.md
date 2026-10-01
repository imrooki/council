# Step 10 冻结契约：崩溃恢复、粘贴、测试缺口、MVP 判据

日期：2026-09-21 · 机器：E 盘机 · 核对基线：`10332ec`

本文件是实现方与测试方**共同的唯一权威**。改本文件要留记录。

本步分四片：**A**（崩溃恢复）、**B**（粘贴，未决 #12）、**C**（测试缺口）、**D**（MVP 判据）。
A 是唯一 `isMinimalUseful` 的一片。B **完全卡在一次人工探针上**，见 1.3。

---

## 0. 本步改动的包

| 片 | 包 | 改什么 |
|---|---|---|
| A | `internal/store` | schema **v5**（见 1.1）：新表 `inflight_turns`、`provider_sessions` 加 `baseline_suspect` |
| A | `internal/conversation` | `InflightTurn` 类型；`ProviderSession.BaselineSuspect`；`Store`/`Tx` 各加方法；`RecordTurn` 多一件事与一个参数 |
| A | `internal/turn` | 新文件 `recover.go`；`Ask` 里三处 phase 写入；`SessionInfo.BaselineSuspect`；`OutcomeLost` |
| A | `internal/app` + `internal/repl` | 启动横幅；`/session` 多一列 |
| B | `E:\Isolating_test\council\oq12-paste\` | **先修探针**（不是代码） |
| B | `internal/repl` + `cmd/council` | 按探针结果二选一，见 5 |
| C | `internal/app` / `internal/conversation` | 两个新测试文件 |
| D | `internal/store` / `internal/conversation` / `internal/repl` | `/cost`（turn_usage 的**第一个生产读点**）；`/log` 加标记列 |
| D | `docs` | `14_MVP_CHECKLIST.md`；`00_DECISIONS.md` 加 D-11 |

---

## 1. 先说四条核查，它们改掉了设计提案里的前置

设计合议与对抗复核都跑在 `1aa23f7` 上。Step 8 的第一片与第二片此后都已落地
（`10332ec`），而 `12_STEP9_CONTRACT.md` 也已冻结 —— 提案里一批前置因此过期或冲突。

### 1.1 schema 版本号撞车：本步是 **v5**，不是 v4

提案把 `inflight_turns` + `baseline_suspect` 写成 **v4**。
但 `12_STEP9_CONTRACT.md` 第 3.3 节已经把 **v4** 定给 Step 9 片 2
（`conversations.team_id` + `turn_usage.identity_fp` + `turn_usage.round_id`）。

**裁决：按路线图顺序，Step 9 片 2 = v4，本步 A 片 = v5。**

若将来决定把 A 片提前到 Step 9 之前落地（它技术上不依赖 Step 9，见 3.1），
两个号对调，**两份契约的形状声明段各改一行**，不许两边都写 v4。

**两份契约里同一个坑要写两遍，这是有意的**：`store/schema.go:330` 是
`var v3Shape = v2Shape` —— **裸别名**，两个变量指向同一张 map。
而 `:261` 的 `buildV2Shape` 是**深拷贝** v1Shape 再覆盖。
**v5 必须照 `buildV2Shape` 的体例深拷贝 v4Shape，不许照 v3Shape 的别名写法** ——
照别名写会把 v2/v3/v4 的历史形状一起改掉，而 `validateSchema` 与
`TestSchemaShapeCoversEveryTable` 都**只看最后一版**，这种踩踏不会被任何用例抓住。

### 1.2 `ErrInflightMissing` 的判别式编译不过（阻塞）

提案写「判别式是 `u.Outcome != int(turn.OutcomeRefused)`」，位置在
`internal/conversation/service.go` 的 `RecordTurn`。

**`internal/turn` 导入 `internal/conversation`**（`record.go:8`、`render.go:7`、
`router.go:8`，以及 `Runner.svc *conversation.Service`），**反向导入本机核过是零** ——
`conversation/types.go` 把 `Outcome` 写成裸 `int` 并注明「`turn.Outcome` 的整数值」,
正是为了躲开这个依赖。反着写就是一个导入环。

**裁决：`RecordTurn` 多收一个 `expectInflight bool`，由 turn 侧算好传进来。**

```go
// expectInflight 说的是「这一轮应当有一条在飞记录等着被清掉」。
//
// 只有 turn 知道答案：它知道 BeginTurn 跑没跑过（起进程之前的那几支拒绝
// 根本没跑到 BeginTurn）。conversation 不认识 Outcome 的语义 ——
// types.go 把 Outcome 写成裸 int 就是为了不认识它，这里不许反悔。
//
// expectInflight 为真而 ClearInflight 报 existed == false 时：
// **整轮事务回滚**，返回包了 ErrInflightMissing 的错误。
// 那只有一种可能 —— BeginTurn 被绕过了，是结构性 bug，
// 正是 10a §2.4「由结构保证而不是靠人记住」要防的东西。
func (s *Service) RecordTurn(ctx context.Context, msg *Message, u *TurnUsage,
	session *ProviderSession, expectInflight bool) (*Message, error)
```

否决把这个标志塞进 `TurnUsage` 结构体：那是一个**控制标志**，不是一条要落库的数据,
混进数据结构里迟早有人把它写进表。

### 1.3 B 片引的那个探针**从来没有跑过**

提案写「三组跑出来的 false 分不开『Windows 做不到』与『我们没申请』」,
又写「A 组是今天 council 实际跑在什么模式下的**唯一测量**」。

**`E:\Isolating_test\council\oq12-paste\` 下只有 `README.txt` / `go.mod` /
`main.go` / `oq12-paste.exe` —— 没有 `RESULTS.md`。** 同一个隔离根下的
`oq8-lock`、`oq8-multiproc`、`oq-cwd-prefix`、`step8a-smoke` 都有。
`00_DECISIONS.md` 未决 #12 今天仍写着「**需要实测，探针已设计**」。

**不存在「跑出来的 false」，也不存在任何「测量」。**
探针缺 mode 2004 这一条是真的（`grep -c "2004" main.go` 得 0），
但把一次没做的测量说成做过了，正是这整轮复核要拦的东西。

**裁决：B 片的第一个交付物是探针本身，第二个交付物是一次人工运行的结果文件。
在 `E:\Isolating_test\council\oq12-paste\RESULTS.md` 存在之前，B 片不动一行生产代码。**
这一条是硬前置，不是流程建议。

### 1.3.1 A 组已跑（2026-09-21，用户在真终端里跑的）

原件 `E:\Isolating_test\council\oq12-paste\RESULTS.md`。

```
console mode: original=0x01F7 requested=0x01F7 actual=0x01F7
  LINE_INPUT=true  ECHO_INPUT=true  VT_INPUT=false

chunk 1: 31 2b 31 3d ef bc 9f 0d 0a   "1+1=？<CR><LF>"
chunk 2: 32 2b 32 0d 0a               "2+2<CR><LF>"
chunk 3: 33 2b 33 0d 0a               "3+3<CR><LF>"
```

**坐实了三件**：

1. 没有 `ESC[200~`，一个字节都没有；
2. 多行粘贴被**逐行交付**，每行一个 read（`LINE_INPUT=true` 是熟模式,
   控制台的行规程把那一整段按行切开）；
3. **粘进来的三行与手敲的三行在字节上完全一样** —— 这就是 #12 的本体。
   Council 的 REPL 没有任何证据能区分「一段三行的需求」与「三条命令」,
   于是它落成三条永久消息，而 Canonical Conversation 只追加不回收。

**它不能读作「Windows 做不到」**：A 组从来没申请过 2004。

### 1.3.2 探针已修（2026-09-21）

**旧探针的 A / B / C 三组一个都不申请 2004**，所以跑 B 与 C 同样得不到
可解读的结果 —— 那会白费用户一次真终端的时间。
括号粘贴不是「开了 `VT_INPUT` 就自动有」的东西，它要程序**显式申请**：
往输出写 `ESC[?2004h`。

`main.go` 已补三组（`A2` / `C2` / `B2`），并补上申请前必须先给**输出**句柄开
`ENABLE_VIRTUAL_TERMINAL_PROCESSING` 这一步（没有它 conhost 会把那串转义
字面打在屏幕上而不是解释它，于是这一组的结果会被读成「申请了也没有」）。
退出前写 `ESC[?2004l`，并把两个句柄的模式**原样**写回 ——
原样，不是写回一个看起来合理的常量：用户的 `QUICK_EDIT` / `INSERT` 设置是他自己的
（A 组量到的 `0x01F7` 就含这两位）。

新探针在申请失败时**响亮拒绝**（退出码 4）而不是继续跑,
理由与本项目一贯的 fail-closed 相同：一次读不出来的 false 比没有结果更坏。

**还要跑的只剩一组是关键的：`C2`。** 它看得到标记就走 B-C（最便宜的一条路）,
只有 `B2` 看得到就走 B-B，全看不到也走 B-B。

### 1.3.5 **C2 已跑出结果：拿得到标记，走 B-C**（2026-09-21）

原件 `E:\Isolating_test\council\oq12-paste\RESULTS.md`。第四次运行,
标记出现在**原始字节**里：

```
hex : 1b 5b 32 30 30 7e ... 1b 5b 32 30 31 7e 0d 0a
       ESC  [  2  0  0  ~     ESC  [  2  0  1  ~

bracketed-paste START (ESC[200~) seen: true
bracketed-paste END   (ESC[201~) seen: true
```

四个前置条件印在同一份输出里、全部成立：
`asked-2004=true`、`VT_PROCESSING=true`、`VT_INPUT=true`、
**`LINE_INPUT=true`（熟模式）**。

**裁决：B 片走 B-C 分支。** Council **不必**切 raw 模式，
也就**不必**自己写最小行编辑器或引 readline —— 整条修法里最贵的那一笔不用付。
5.3 的 B-B 分支作废（保留在本文里只作被否决记录）。

**两处限定，不许略掉**：

1. **这一次覆盖的不是 Windows Terminal**：探针自报
   `not Windows Terminal (no WT_SESSION / TERM_PROGRAM / TERM)`。
   非 WT 的终端拿得到，WT 大概率也拿得到 —— **大概率不是实测**，
   WT 按 `[未验证]` 记。这不影响 B-C 的落地形状（见 1.3.4：
   `enablePaste` 本来就按 `ok bool` 设计，天然承载「这个终端不支持」）。
2. **多行粘贴怎么分块没量过**：这一次粘的是**一行**，所以两个标记落在同一个 chunk 里。
   熟模式下行规程很可能仍按换行切开，于是 `ESC[200~` 跟着第 1 行、
   `ESC[201~` 跟着第 N 行，中间几行两个标记都没有 ——
   5.2 的 `splitPasteMarkers(line string) (text string, start, end bool)`
   **正是按这个形状设计的**（逐行、各自回答 start/end），所以契约不用改。
   **但这是推断不是实测**：落地 B-C 的第一件事就是拿同一个探针粘一段三行的文本钉死它。
   钉出来若是「整段一个 chunk」，`splitPasteMarkers` 的签名要从逐行改成收整块。

   **2026-09-22 已关闭：实测是逐行交付，签名不改。**
   Griffin 在真终端跑 C2 组，四行粘贴的分块形状是：
   `chunk1 = ESC[200~ + 第1行 + CRLF`、中间两行各自一个 chunk 不带标记、
   `chunk4 = 第4行 + ESC[201~ + 之后手敲的内容 + CRLF`。
   **额外量到一件本节没问的**：END 标记与最后一行正文同在一个 chunk，
   而且后面还能跟着手敲的字符 —— 分割器必须能处理「标记出现在一行中间」。
   现查 `paste.go` 的实现正好覆盖（`strings.Contains` 全行找、
   `ReplaceAll` 只挖标记保留前后正文），**落地代码写在测量之前，但与实测逐条吻合**。
   raw 模式（B2）的分块形状不同（标记单独成 chunk），但那次粘的是单行，
   「raw 下多行是不是整块」按 [未验证] 记 —— 我们走 B-C，不适用。
   完整记录见 `00_DECISIONS.md` 未决 #12。

### 1.3.6 计时判据量错了东西，探针已订正

那一份 verdict 里**自相矛盾**：既印「DOES deliver markers」
又印「consistent with TYPING, NOT a paste」。

病因：计时判据量的是 **chunk 之间**的间隔，而人「粘一段、回车、再粘一段、回车」时
chunk 之间当然是几秒。**它回答的是「这几行是不是一次突发送进来的」,
不是「这是不是一次粘贴」。**

**标记本身是直接证据**：终端只在真粘贴时才发 `ESC[200~`。
探针已改成**标记优先**：看得到标记时计时判据让位、只作信息;
看不到标记时才靠它把「粘了但没标记」与「根本没粘」分开。

记在这里是因为它是本轮第四个同形的错误 ——
**一个测量装置量的不是它自称在量的那件事**。前三个是
「有没有申请 2004」「人粘了还是敲了」「跑在哪个终端下」。

### 1.3.3 C2 跑过一次，前置全部成立，但那个 false 不能采信

C2 的第一次运行（2026-09-21）里**前置条件这次全部成立**，这是相对 A 组的进步：
`VT_INPUT` 被接受（`requested` 与 `actual` 都是 `0x03F7`，没被静默拒掉）、
stdout 的 `VT_PROCESSING` 为真（所以 `ESC[?2004h` 是被**解释**了不是被打出来）、
申请确实发出去了。**而 `ESC[200~` 没有出现。**

**但输出里回显与 chunk 打印是交错的**：`1` / dump / `2` / dump / 空行 / dump / `3` / dump。
熟模式下控制台收到一整段粘贴会**先把四行一起回显**，之后程序才读到第一行;
交错出现是「一行一行敲进去」的样子。A 组的输出有同样的形态。

**如果那几行是敲的，这个探针什么都没证明** —— 它要问的是真粘贴送进来什么字节。
这正是本项目一路在防的那一类：一个看起来是结论的东西，
其实前提没被控制住（与 1.3 那条「没申请过 2004 的 false 不可解读」同源，
只是这次没被控制的变量换成了「人到底粘了还是敲了」）。

**处置：探针再改一次，让它自己拿证据，不靠回显排版去反推、
也不靠人自述「我粘了」。** 每个 chunk 记一次单调时钟：
真粘贴的各行之间是亚毫秒到几毫秒，手敲之间是几百毫秒到几秒。
间隔落在 50ms 与 300ms 之间时程序**不猜**，印 `AMBIGUOUS`;
没粘贴过的 false 印成 **`INCONCLUSIVE`** 而不是一个结论。

**50ms / 300ms 这两个数不是标定过的阈值，是量级判据**，
探针的注释里明写了这一点 —— 它们分开的是「亚毫秒」与「几百毫秒」两个量级,
不是一条精确的线，落在中间就拒绝下结论。

**C2 要重跑，并且必须用剪贴板粘（Ctrl+V 或右键），不能敲。**

### 1.3.4 第三个没被控制住的变量：**跑在哪个终端模拟器下**

前两次 C2 都跑在 **Claude 桌面应用自带的终端面板**里，不是 Windows Terminal
（核过：那条 shell 的父进程链是 `powershell -> cmd -> claude -> claude -> explorer`，
且 `WT_SESSION` / `TERM_PROGRAM` / `TERM` 全部未设置）。

**加括号粘贴标记的正是终端模拟器这一层** —— conhost、Windows Terminal、
VS Code 的终端、应用自带的面板，各自实现各自的。
所以一份没有写明模拟器的结果**说不清它覆盖了什么**。

这是这条探针上第三个没被控制住的变量，前两个是
「有没有申请 2004」（1.3）与「人到底粘了还是敲了」（1.3.3）。
三个都是同一个形状：**一个看起来是结论的 false，其实前提没被控制住。**

**处置：探针自报终端**（`describeTerminal()`，判据是 `WT_SESSION` 等环境变量），
并在 verdict 里印出来。**每一份结果都必须写明它覆盖哪个模拟器。**

**这条同时改变了 B 片的裁决口径**：不能再问「Windows 能不能做括号粘贴」,
只能问「**用户实际跑 Council 的那个终端**能不能」。于是：

- `C2` 至少要在 **Windows Terminal** 与 **应用自带面板**两处各跑一次;
- 两处结果不同时，B-C 分支的落地条件写成
  「`enablePaste` 拿不到标记就退回今天的行为」——
  它本来就是按 `ok bool` 设计的，天然能承载「这个终端不支持」这件事,
  **不需要为此改形状**；
- `00_DECISIONS.md` 未决 #12 的记法里要带上终端名，不许只写一个 true/false。

### 1.4 `Tx.SetBaselineSuspect` 会破坏一条白纸黑字的不变量

`conversation/types.go:161-165` 原文：

> `ProviderSession` 是一条 provider 会话在库里的全部状态。
> **这个结构体必须整个读、整个写，不给任何一个字段单独的 setter。**
> 五样东西（`ProviderSessionID` / `CursorSeq` / `CostTotalUSD` / `LastTurnAt` /
> `IdentityFP`）同生共死：读回时缺任何一样，五样全部作废，该 agent 按 Fresh 处理。

提案要加 `Tx.SetBaselineSuspect(convID, agentID, v)` —— 一个单字段 setter。
提案的 conflicts 列了三处契约冲突，**恰恰漏了这一处，而它是唯一破坏不变量的那处**。

**裁决，两条一起**：

1. **不加单字段 setter。** `SettleLostTurns` 在自己的 `WriteTx` 里
   **整行读回 -> 改标志 -> 整行写回**，走与 `RecordTurn` 同形的路径。
   `types.go:163` 一个字不改。
2. `BaselineSuspect` 加进 `ProviderSession`，但**明写它不是「第六样」** ——
   它是一个**关于那五样的标记**。「五样同生共死」那条读回规则不变,
   `types.go` 的注释要补一句说清楚这个区分，否则下一个人会把它算进去、
   然后为「读回时缺了它就整条作废」写一段没必要的代码。

`10a` §2.4「**这是本步唯一允许写 `turn_usage` 与 `provider_sessions` 的入口**」
**确实要改**，改成两个入口，并写明豁免的结构性理由：

> `SettleLostTurns` **只在启动时、进 REPL 之前跑一次**（见 3.5），
> 所以它与 `RecordTurn` 在结构上不可能并发。
> 这是它能成为第二个写入口的全部理由；哪天它变成可以中途调用的东西，
> 这条豁免同时作废。

---

## 2. A 片要解决的那个洞，先说准

今天「council 进程崩在一轮中间」是**第三种结局家族里唯一没有记账的成员**：

| 结局 | `turn_usage` | `provider_sessions` |
|---|---|---|
| 失败轮（超时 / 解析不了 / CLI 自报失败） | **有**（`persistTurn` 只要 TurnID 非空就写） | 标 `baselineSuspect`（**只在内存里**） |
| `OutcomePostRejected` | 有 | 有 |
| **进程崩了** | **一行都没有** | 什么都没有 |

**还有一个独立的既有 bug**：`sessionState.baselineSuspect` 只活在内存里,
`conversation.ProviderSession` 的九个字段里没有它，而 `cost_total_usd` 是**持久的**。
于是一次超时之后重启：**基线可疑这件事没了、基线本身还在**,
`computeCostDelta` 会拿一个已知落后的基线去算差值并且不报告。
超时对 grok 是主路径不是兜底，所以这不是边角情形。

---

## 3. A 片：崩溃恢复

`isMinimalUseful: true`。

### 3.1 依赖与顺序

**提案的 dependsOn 盯错了分支**（它写「排在 #9(a) 那一路之后」，而 #9(a) 就是当时的 HEAD）。
现在的正确写法：

> **前置：`12_STEP9_CONTRACT.md` 片 2 的 schema v4 已落地并提交，
> 且 `go build ./...` 与 `go test -count=1 ./...` 全绿。**

**插入点一律写锚点，不写行号**（提案引的 4 组行号在 Step 8 第二片落地时全漂了）：
`BeginTurn` 插在 `cmd.Dir` 回填**之后**、`spawnCtx, cancel := context.WithTimeout(ctx, timeout)`
这一行**之前**。

A 片技术上**不依赖 Step 9 的任何代码**（只依赖它的 schema 版本号）。
若要把它提前，见 1.1 的版本号对调规则。

### 3.2 schema v5

```sql
CREATE TABLE inflight_turns (
    turn_id            TEXT PRIMARY KEY,
    conv_id            TEXT NOT NULL,
    agent_id           TEXT NOT NULL,
    provider           TEXT NOT NULL,
    planned_session_id TEXT NOT NULL DEFAULT '',
    resume_session_id  TEXT NOT NULL DEFAULT '',
    cursor_seq         INTEGER NOT NULL,
    scanned_to         INTEGER NOT NULL,
    started_at         TEXT NOT NULL,
    phase              TEXT NOT NULL DEFAULT 'begun',   -- 见 3.3
    tier               TEXT NOT NULL DEFAULT '',        -- 见 3.4
    work_dir           TEXT NOT NULL DEFAULT ''         -- 见 3.4
);
ALTER TABLE provider_sessions ADD COLUMN baseline_suspect INTEGER NOT NULL DEFAULT 0;
```

`ADD COLUMN` 追加在末尾，`v5Shape` 的列序照此声明。
**`v5Shape` 必须深拷贝 v4Shape**（1.1）。

**验收**：造一个 v4 库、里面一条 `provider_sessions` 行，用 v5 打开：
那一行原有的值**逐字节不变**、`baseline_suspect` 读出 0、`inflight_turns` 存在且为空；
并且对同一份配置算出的 `IdentityFingerprint` 与迁移前**相等**
（照 `11a` §8 第 2 条「升级不花钱」的体例）。

### 3.3 `phase`：三态，不是一个 bool（提案里没有）

复核指出提案的落点留了一条**假阳性**：`BeginTurn` 之后任何一条 refuse / failClosed 路径上
`persistTurn` 的 `RecordTurn` 失败，在飞记录就留下来了，
下次启动会把一轮**一个进程都没起过**的轮报成丢失、标 `baseline_suspect`、
并写一条说「provider 侧可能已经发生」的面包屑。

复核建议加一个 `spawned` 布尔。**本契约用三态 `phase`**，因为还有第三种状态
（复核的 missing 1 单独提的那个）没地方放：

| `phase` | 何时写 | 对账时读作 |
|---|---|---|
| `begun` | `BeginTurn` 时（自开一个 `WriteTx`） | **进程没起，记录残留**。只删行、只记一行诊断，**不标 `baseline_suspect`、不发面包屑** |
| `spawned` | **`r.spawn` 调用之前立刻**（自开一个 `WriteTx`）—— 原文写的是「返回之后」，2026-09-22 订正，见 3.3a | **起过进程、结局未知**。标 `baseline_suspect`、发面包屑 |
| `returned` | `absorbParse` 成功之后立刻（自开一个 `WriteTx`） | **provider 确实说了话，正文没落库**。面包屑措辞与 `spawned` **不同**，见 3.6 |

`returned` 这一档对应的是 `runner.go` 里 `RecordTurn` 与它的补救**两次都失败**那条路：
那一轮我们**确知** provider 说过话、正文已经打到屏幕上了（`rec.DroppedText`），
面包屑若还写「provider 侧可能已经发生」就是把一件已知的事说成不知道。

每次 phase 更新各自开一个 `WriteTx`，与 `BeginTurn` 同一条理由：
**这一行的全部价值就是「进程在两个事务之间死掉时它还留得下」。**
本地 SQLite 写是亚毫秒级的。

**顺带改掉提案理由段里的一处事实错误**：它写「起进程之前就拒绝的那几支
（权限档不放行、`ErrNoDelta`、指纹探针出错、**命令行超长**）本来就没落过在飞记录」。
**命令行超长是落过的** —— `ErrCommandLineTooLong` 与 `ErrShimNewlineArg` 都在
`spawnProcess` 里返回，也就是在 `r.spawn` 里，而 `BeginTurn` 插在它之前。
眼下无害（两者 `Outcome=Refused`，照样被清掉），但理由段里那一项要删掉或改对。

### 3.3a `spawned` 的落点订正：写在 `r.spawn` **之前**（2026-09-22）

**3.3 的表格与 3.15 第 3 条原本写「`r.spawn` 返回之后立刻写 `spawned`」。
这条与本节的验收 14 / 15 正面冲突，落地之前订正。**

现查（`internal/turn/spawn.go:97`、`:150`）：`spawnProcess` 在**同一次调用**里
`exec.Command` -> `Start` -> `Wait`，也就是说 `r.spawn` 是「起进程**并等它跑完**」。
于是「返回之后才写 `spawned`」意味着：

- `begun` 覆盖的是 **[BeginTurn 提交] 到 [子进程跑完]** 这整段 ——
  硬超时是 5 分钟，这是崩溃窗口里**最长、最可能中招的那一段**；
- 而 3.3 的表格把 `begun` 读作「**进程没起**，不标 `baseline_suspect`、不发面包屑」。

**后果是把主用例判反**：验收 14（子进程还在跑时 `taskkill /F` council，
重启后断言 `baseline_suspect == 1`）与验收 15（bootstrap 轮同样杀）
**按原文必然失败**，而且失败的方向是**静默少报** —— 崩溃之后第一笔钱会被
自信地算出来，正是第 2 节要修的那个洞。
`spawned` 与 `returned` 之间剩下的差别只有一次解析的微秒级窗口。

**裁决：`SetInflightPhase(spawned)` 写在 `r.spawn` 调用之前**，
落点锚在 `timeout` 算完**之后**、
`spawnCtx, cancel := context.WithTimeout(ctx, timeout)` 这一行**之前**。
（初稿写的是「`spawnCtx` 之后」，同样满足「早于 `r.spawn`」，但那样这一条提前
`return` 必须记得先调 `cancel()`，否则泄漏一个 context。
放在 `spawnCtx` 之前就根本没有这个问题，而两处对崩溃窗口完全等价。）用**调用方的 ctx**（与 `BeginTurn` 同理，
3.12：spawn 之前取消就该连记录也不写）。这一次写失败就 **refuse，不起进程** ——
没有留得下的见证就不要去花钱。

**不改 `Spawner` 类型**去传一个「进程已起」回调：那个字段测试要换，
改签名会把成本摊到每一个换 Spawner 的用例上。

**这个落点的代价，如实写在这里**：`ErrCommandLineTooLong` 与 `ErrShimNewlineArg`
是在 `spawnProcess` 里、`exec.Command` **之前**返回的，此刻 phase 已经是 `spawned`
而一个进程都没起。正常流程下它们走 `refuse` -> `persistTurn` -> `ClearInflight`，
行被删掉，看不出来；**只有当那次清理的 `RecordTurn` 也失败**（双重故障）时，
才会漏下一条 `spawned` 的假阳性，下次启动多标一次 `baseline_suspect`、
多发一条措辞偏严的面包屑。

3.3 设计三态正是为了躲这个假阳性。**但它躲的是一次双重故障，代价是每一次真崩溃
都判成「没花钱」。这笔交换要反过来做**：宁可在双重故障时多说一次「可能花了钱」，
不可在单次崩溃时少说一次。

**剩余项**（不在本片做）：把那两条 argv 预检从 `spawnProcess` 里提出来，
在 `spawned` 写入**之前**由 Runner 先跑一遍，假阳性就没了。

**订正之后 `begun` 还剩什么**：只剩两种 ——
(a) 崩在 `BeginTurn` 提交与 `SetInflightPhase(spawned)` 之间的那个微秒窗口；
(b) `SetInflightPhase(spawned)` 本身失败、随后的清理也失败。
**两种都确实没有把命令交给 OS，按 3.3 原文「只删行、只记一行诊断」处置仍然正确**，
只是罕见。罕见不是死键：DDL 造得出这个值，对账必须认得它。

**仍然写两次（`BeginTurn` 记 `begun`，再 `SetInflightPhase` 记 `spawned`），
不合并成 `BeginTurn(Phase: spawned)`**：合并会让 `PhaseBegun` 变成一个
谁都写不出的值，而 `types.go` 与 `Valid()` 已经把它当合法值冻住了。

### 3.4 `tier` 与 `work_dir`：WORKSPACE 落地之后必须记

Step 8 第二片（`11c`）已经放行 WORKSPACE：`agentWorkDir` 给 WORKSPACE 及以上建
真正可写的目录、每轮起进程前 `MkdirAll`。
**从那一片起，崩在一轮中间的成员可能已经往盘上写了一半。**

`inflight_turns` 记下档位与工作目录，`turn_lost` 面包屑在 `tier != read` 时点名那个目录,
措辞写成「**可能有半截落盘的文件**」。

**什么都不删、什么都不回滚** —— 这与「对账只碰本地状态与只追加的事实源」不冲突,
缺的只是把已知的事实记下来。

### 3.5 `AuditInflight` 与 `SettleLostTurns`

```go
type LostTurn struct {
	Turn     conversation.InflightTurn
	Reason   string
	OrphanID string // 见 3.6：只有 grok 的 fresh 轮填得上
}
```

**两个方法都只在启动时、进 REPL 之前跑，不导出给 REPL。**

提案让 `AuditInflight` 成为一个可随时调的公开方法并照抄 `drift.go` 的
「不取 `r.mu`」理由。**那条理由对它不成立**：`AuditDrift` 能无锁读是因为
`provider_sessions` 在**一轮结束时**才写；`inflight_turns` 是**一轮开始时**写的。
一次飞行中的 `/ask` 期间做无锁读，会读到那一轮**自己**的在飞记录,
把一条正在跑的轮报成 `LostTurn` —— 提案自己写过「假阳性会让这个审计被人忽略，等于没有」。

**裁决：结构性地只在启动时跑**（`app.Run` 在进 REPL 之前调一次）,
**并删掉「否则一次启动审计会被一轮飞行中的 `/ask` 挡 5 分钟」那句理由** ——
那句话只有在它会中途跑时才成立，而中途跑正是它出错的时候。

这条同时是 1.4 那个豁免的结构性依据：启动时跑，就不可能与 `RecordTurn` 并发。
**Step 9 片 1 把 `r.mu` 换成按 key 的闸之后，这条裁决仍然成立且更重要** ——
那时根本没有一把全局锁可取。

`SettleLostTurns` 逐条、每条一个 `WriteTx`，一个事务里做四件：

1. **往 `turn_usage` 补一行**（见 3.6）；
2. 追加 `KindSystem` 面包屑（`MetaEvent = "turn_lost"`）;
3. **整行读回 `provider_sessions` -> 置 `baseline_suspect=1` -> 整行写回**（1.4）;
4. 删在飞记录。

四件不同生共死的话，会出现「面包屑写了但记录还在」这种下次启动重复报的状态。

### 3.5a `SettleLostTurn` 的校验清单为什么与 `Post` / `RecordTurn` 不一样（2026-09-22）

落地之后有两条腿先后问到同一件事，所以把裁决写下来，免得下一个人再推一遍。

`SettleLostTurn` 是**第三个会追加消息的公开入口**。它**有**这几道、
**故意没有**那几道，判据只有一条：

> **这一道，底下有没有人管？**
> 底下没人管的，必须在这一层补；底下有两层管的，不在事务外重复。

**必须有（底下没人管）**：

| 校验 | 为什么底下管不住 |
|---|---|
| 保留 sender 不许带脑子 | `messages` 的 CHECK 现查是 `CHECK (sender_id <> 'user' OR ...)`，**只约束 `'user'`，管不到 `'system'`**；`Tx.AppendMessage` 也不查 |
| `SenderID == ""` | `AppendMessage` 不查 sender；`messages.sender_id` 是 `TEXT NOT NULL`，**空串过得了这一列** |
| `applyThoughtPolicy` | 这不是校验，是用户在配置里表达的存储开关。`SetThoughtPolicy` 的文档注释**逐个点名入口**，多一条路不接就让那句注释当场变假 |

**故意没有（底下有两层管）**：

- **`Kind.Valid()`**。`Tx.AppendMessage` 第一行就查（`internal/store/tx.go:97`，
  返回 `ErrInvalidKind`），`messages.kind` 上还有
  `CHECK (kind IN ('user','agent','system','task','task_result'))`（`schema.go:70`）。
  非法 kind 落不了库、事务回滚、在飞记录还在，下次启动照样能重来。
  在事务外再查一次只省掉一次写事务的开销，**换不到任何正确性**。
  **这与脑子那道的区别就是全部理由：那边两层都不管，这边两层都管。**

**故意没有（与本方法的设计冲突）**：

- **事务外先查会话存在**。会话不存在是 **3.9 降级路径的入口**，不是失败。
  提前查到就当失败返回，3.9 整条就没了。
- **事务内 `RequireActiveParticipant`**。正当 sender 是保留的 `system`，
  `Post` 对它本来就跳过。这类事件常常恰好落在刚退出或正在被重配的成员身上，
  过活跃成员校验，**该留的面包屑一条都留不下**。
- **`(msg == nil) != (u.Seq == nil)`**。本方法存在的理由就是这条在 `RecordTurn`
  上成立、在丢失轮上不成立：面包屑非 nil，而 `seq` 必须是 nil。
- **非保留 sender 的 `SenderID` 与 `u.AgentID` 比对**。面包屑的 sender 是 `system`，
  本来就不等于这一轮的 agent。

**下一个想「对齐三个入口的检查表」的人，请先用上面那条判据过一遍**：
清单对齐是形式，「底下有没有人管」才是内容。

### 3.6 崩在 bootstrap 轮的钱，必须记得上

**这是提案最大的一个落空**：它在交付物里写「`provider_sessions` 行不存在时
（bootstrap 轮崩掉）**只删在飞记录、不报错**」。

而 bootstrap 轮**正是最贵的那一轮**（D-5：一人 18k-48k token）。
照提案的处置，那笔钱在库里**根本不存在**：没有 `provider_sessions` 行可标、
没有 `turn_usage` 行（`persistTurn` 从没跑到）、于是 D 片的 `/cost` 读不到它。
片头宣称的「不再是一次静默的成本谎言」恰恰在最该兑现的地方落空。

**裁决：`SettleLostTurns` 无论 `provider_sessions` 行在不在，
只要 `phase != 'begun'`，都往 `turn_usage` 补一行**：

- 新结局 **`OutcomeLost = 9`** —— **钉死数字，不写「加在末尾」**。
  `11d` 第 3.5 节也要加一个 `OutcomeCanceled`，而本契约 1.1 允许 A 片提前落地;
  落地顺序一反两个值就对调，**而 `turn_usage.outcome` 存的是整数**。
  两个值在 `09a` 里钉成字面量：`OutcomeCanceled = 8`、`OutcomeLost = 9`
  （今天的块是 `Unknown=0 ... Refused=7`）。
- **全部 token / cost 列为 NULL**；
- `conv` / `agent` / `provider` / `started_at` 取自 `inflight_turns`。

**目标表的 NOT NULL 约束要逐列点清楚，不能只说「该有的都有」**
（`turn_usage` 的 v2 DDL 本机核过）：

| 列 | Lost 行怎么填 |
|---|---|
| `turn_id` (PK) | `inflight_turns.turn_id` |
| `conv_id` NOT NULL **REFERENCES conversations(id)** | **有外键** —— 见 3.9，会话没了就插不进去 |
| `agent_id` NOT NULL | 取自在飞记录 |
| `provider_session_id` NOT NULL DEFAULT `''` | `resume_session_id`；bootstrap 轮是空串 |
| `seq` 可空 | **NULL**（没有产生消息） |
| `outcome` NOT NULL | `OutcomeLost` |
| 七个 token 列、`cost_usd`、`cost_cumulative_usd` | **全部可空 -> 全填 NULL** |
| `cold_start` NOT NULL DEFAULT 0 | 只能填 0，**但 0 在这一行不读作「暖」而读作「不知道」** —— `/cost` 必须把 Lost 行排除在冷启动计数之外（7.1 已裁） |
| `wall_ms` NOT NULL DEFAULT 0 | 同上，0 是「不知道」 |
| `prompt_bytes` NOT NULL DEFAULT 0 | 0。**这一条顺带让 `12` 4.5 的取样自动跳过 Lost 行**（那里规定 `prompt_bytes == 0` 的样本不能用来缩放），两份契约在这里是一致的，落地时不要「顺手修好」它 |
| `delivered_lo` / `delivered_hi` 可空 | **NULL** —— 在飞记录有 `cursor_seq` 与 `scanned_to`，但那是**打算送**的范围，不是**确实送到**的，不许拿它冒充 |
| `created_at` NOT NULL | `inflight_turns.started_at` |

**三个 NOT NULL DEFAULT 0 的列（`cold_start` / `wall_ms` / `prompt_bytes`）
在 Lost 行上全都是「不知道」而不是「零」**，这与 `05` §3
「可空表示这家 provider 根本没有这个数，不是 0」是同一条纪律的反面情形 ——
这里列不可空，所以纪律只能靠**读的那一侧**（`/cost`、Budget 取样）承担。
两处都要写进注释。

这样 `/cost` 会把它计进「金额不知道」那一格，MVP 判据第 8 条
「『不知道』不被当成 0」才真的覆盖到崩溃。

**孤儿 id 的话要说得有边界**。提案写「孤儿会话的 id 是**我们本来就知道**的」 ——
**那只对 grok 的 fresh 轮成立**：`provider/build_test.go` 断言 resume 轮
`PlannedSessionID` 必须是空串，另两条断言 claude 与 agy **根本不预铸**。
**claude / agy 的 bootstrap 崩溃，两个 id 都是空，那才是真正叫不出名字的孤儿。**

### 3.6.1 这件事已经在真实操作里发生过一次（2026-09-21）

原件 `E:\Isolating_test\council\oq-ctrlc\RESULTS.md`。跑 needsProbe #6
（Ctrl-C）时自然产生了一条：grok 预铸了
`1068abab-73ce-4af5-9d14-65e06e319a08` 并真的跑了 11.4 秒，那边**很可能真有
一条会话**。而本地：

- `provider_sessions` **一行都没有**；
- `turn_failed` 那条 system 消息的 `meta` 只有
  `{"about_agent":"gk","event":"turn_failed"}` —— **没有 `planned_session_id`**。

那个 id 只出现在 `Record` 的错误文本里，也就是**只活在当次终端的滚动历史里**。
关掉窗口就没了；重启之后 `/log` 只剩「agent gk 这轮调用失败（outcome 5）」。

**这不是一个想出来的场景，是按一次 Ctrl-C 就会得到的东西。**
它直接坐实本片 `inflight_turns` 存 `planned_session_id` 的设计 ——
**今天没有第二个地方存得住它**，而它是崩溃之后唯一能点名那条孤儿会话的东西。

**顺带给本片加一条验收**（并进 3.15 的真机那一组）：
按 Ctrl-C 制造一条在飞记录，重启之后**面包屑里必须带得出那个 id**。
今天这条必红 —— 那正是本片要修的。

面包屑在 `OrphanID == ""` 时必须**老实说这条 provider 会话点不出名字**,
不许印一个空串糊过去。

### 3.7 两条纪律，写成用例而不是注释

- **绝不把 `planned_session_id` 拿去 resume**：grok 对未知 id 会拉起交互式 OAuth
  无限等待，代价不是一次冷启动而是一个挂死的进程。
- **绝不因为「provider 可能已经吃下了」就推游标**：丢 delta 是永久的、
  在只追加的流里纠正不了，**重投是可见的**。

对账**不写 `cursor_seq`**；`LostTurn.OrphanID` 只进面包屑与屏幕,
**不进任何 `TurnRequest`**。

**收口判据**：对账之后紧接着跑一轮假 CLI，断言
`rec.Fresh == true`（没有拿孤儿 id 去 resume）、
`Delivered` 里含崩溃那一轮送过的那些 seq（游标没被偷推）、
`rec.CostUSDDelta == nil` 且 Meta 里有 `cost_baseline_suspect`。
**崩溃之后第一笔钱必须报「不知道」，不许猜一个数。**

### 3.7a `Record.BaselineSuspect` 问的是进轮时的状态，不是出轮时的（2026-09-22）

写 3.7 收口判据与 3.15 验收的人都要先看这一条。A 片之三的测试腿在这里读错过一次，
写出了一条**挡住自己真判据**的前置断言，所以把时序钉在这里。

`runner.go` 里这一行发生在**进这一轮的早期**（与 `bootstrap` / `ColdExpected` 同一批）：

```go
rec.BaselineSuspect = st.baselineSuspect
```

而 `failClosed` 把 `st.baselineSuspect = true` 是在**这一轮跑完、判定失败之后**才做的，
**不回头改已经赋过值的 `rec`**。`record.go` 的字段注释本来就点破了：

> `BaselineSuspect bool // 上一轮的结局是未知的，本轮差值一律 nil 并记 Meta`

**「上一轮」**——问的是「进这一轮时，上一轮留下的状态」。

**所以：一轮超时，它自己那个 `Record.BaselineSuspect` 是 `false`。**
变成 `true` 的是**下一轮**。要断言「基线被标可疑了」，两个判据可选，
**都不要去问失败那一轮自己的 Record**：

1. 直接查库：`SELECT baseline_suspect FROM provider_sessions WHERE ...` 应当是 1；
2. 再跑一轮（或换一个新 Runner 模拟重启），问**那一轮**的 `rec.BaselineSuspect`。

3.7 的收口判据原文写的是「对账之后**紧接着跑一轮**假 CLI」，口径本来就是对的，
本节只是把「为什么不能问失败那一轮自己」说明白。

**同名不同时态是这个坑的根源**：`turn.Record.BaselineSuspect`（进轮快照，只读）
与 `turn.sessionState.baselineSuspect`（活状态，会被 `failClosed` 置位、
被 `commitSession` 清零）同名，而 `conversation.ProviderSession.BaselineSuspect`
是它落库的那一份。三个同名的东西，时态各不相同。

### 3.8 `SettleLostTurns` 失败必须响亮，但不拦着进 REPL

提案让它照 `11a` §6「审计失败只往 Err 记一行、不阻止进 REPL」处置。
**那条先例说的是一个纯读的审计**；`SettleLostTurns` 是**写**,
而且写的正是成本正确性的闸门：settle 失败 -> `baseline_suspect` 留在 0 ->
下一次 `/ask` 从库里建 `sessionState` -> `computeCostDelta` 看见 false ->
**自信地打印一个错数**。

**裁决：分开处置，且不采用「拦住进 REPL」。**

- `AuditInflight`（纯读）失败：照 `11a` §6 只记一行 Err。
- `SettleLostTurns`（写）失败：**受影响的 `(conv, agent)` 的下一轮 `/ask` 直接拒绝**,
  直到对账成功。不拦整个 REPL —— 用户连 `/log` 都进不去才是更糟的处置。

**实现上不需要新状态**：settle 失败时那条在飞记录**没被删掉**,
所以 `Ask` 在起进程之前查一次「这个 `(conv, agent)` 有没有一条不属于本轮的在飞记录」,
有就 refuse。这是结构性的，不靠一个内存标志。

**这条检查的位置要写死**：**在拿到该 key 的 gate 之后、`BeginTurn` 之前**
（`12_STEP9_CONTRACT.md` 片 1 把 `r.mu` 换成按 sessionKey 的闸之后尤其要紧）。
放在闸外的话，同一个 `(conv, agent)` 的两次并发 `AskUpTo` 会**都**看到
「没有别人的在飞记录」，然后**都**调 `BeginTurn` ——
而 `inflight_turns` 的主键是 `turn_id` 不是 `(conv_id, agent_id)`,
**两行都插得进去**，一个成员就有了两条在飞记录。

**并且加一条 `UNIQUE(conv_id, agent_id)`**：有 gate 时它是第二道保险,
**而在 Step 9 落地之前**（本契约 1.1 允许 A 片提前）**它是唯一的那道**。
违反时的处置是让 `BeginTurn` 响亮失败 -> `Ask` refuse,
与 3.8 这条拒绝是同一个结局，用户看到的是同一句话。

**2026-09-27 落地订正（A 之五，`8c00ec9`；契约原文归档在 `13a_S10_A5_CONTRACT.md`）**：
- **位置比本节写的更早**：检查放在取到闸之后、**任何副作用之前**（`began := false` 之后、`cfg == nil` 判断之前），不是「紧挨 `BeginTurn`」。
  实测撞唯一索引之前已经有副作用：bootstrap 轮先发了一条 `session_opening`；指纹不符那一支先发 `session_rebuilt` 再 `DropProviderSession`。
  对 spawned / returned 行，删掉的正是对账要标可疑的那一行；begun 行的对账（`recover.go`）本来只删在飞行、不标 `provider_sessions`，但假面包屑与删行照样发生。
- **UNIQUE 索引 v5 早已建好**（`schema.go` 的 `idx_inflight_turns_conv_agent`），不需要迁移。
- **「同一句话」由结构保证**：哨兵 `conversation.ErrInflightConflict` 有两个来源——这道显式检查，以及 `store/tx.go` 的 `BeginTurn` 撞 UNIQUE（modernc 扩展码 2067）时的翻译；`askCause` 只有一个分支。
  那句话（Griffin 2026-09-27 裁）：「这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行」。
- **读在飞记录失败也拒绝**（「不知道」不是「没有」）。
- **「闸空闲时还留着的行只可能是没对上账的」要多算一种来源**：`BeginTurn` 之后、起进程之前的一次取消。`refuse` 用的是调用方 ctx，被取消时清理写不进，这一行会留到重启对账，被拒那一轮的 `turn_usage` 也会丢。这是 A 之五之前就有的行为，本刀不修，记在 `00_DECISIONS.md` #43。
- **解开的路只有重启对账**：`/session rebuild` 只删 `provider_sessions`、不碰 `inflight_turns`，解不开；拒绝那句话因此不提它。对账持续失败时 REPL 里没有入口，只能手改库——#43 的剩余项。

### 3.9 在飞记录指向的会话已经不存在时（提案里没有）

`drift.go` 对同构的情形有明确处置（「配置里已经没有这个 agent 了，但那条会话还在库里占着位」,
也要报）。inflight 这边提案一个字没有。

**问题很具体**：追加 `KindSystem` 面包屑需要一个活着的 `conv_id`；
会话被删掉的话 `AppendMessage` 会失败，**整个 settle 事务回滚**,
于是这一行永远卡在那里、每次启动都报，而且按 3.8 那个成员从此 `/ask` 不了。

**而且不止面包屑**：`turn_usage.conv_id` 上有
`REFERENCES conversations(id)`（v2 DDL 核过），所以会话没了那条 `turn_usage`
**也插不进去** —— 3.6 说的「照样补一行」在这一支不成立。
提案与复核都没走到这一步。

**裁决**：会话不存在时，**四件降成两件**：

1. 删在飞记录；
2. 在启动的 Err 行里点名：
   「该会话已不存在（`<conv_id>`），这一轮的账**补不上**（外键），
   只知道它在 `<started_at>` 起过、`phase=<phase>`。」

面包屑与 `turn_usage` 都跳过 —— **没有事实源可追加是事实，不是错误**;
`provider_sessions` 那一行同样已被级联清掉或本就不存在。

**这是一次真实的账目损失，契约里要直说，不许假装补上了。**
它的前提（用户删掉了一条有在飞记录的会话）本身很罕见,

**触发点的订正（2026-09-22，A 片之四实现腿指出）**：
本节与 A 片之四的任务书都把 `ErrConversationNotFound` 的来源写成
`Tx.RecordUsage` 内部的 `requireConversation`（外键是第二道）。**那个点名是错的**：
`Tx.AppendMessage` 同样先调 `requireConversation`（`internal/store/tx.go` 现查），
而非 `begun` 路径上 `msg != nil`、`AppendMessage` 排在 `RecordUsage` 之前，
所以事务实际上在 **`AppendMessage`** 就返回了这个哨兵，`RecordUsage` 根本走不到。

**行为不受影响**（`errors.Is` 照样命中，降级路径一样走），错的只是「谁报的错」。
记在这里是因为：哪天有人把 `AppendMessage` 的那句 `requireConversation` 拿掉、
或者把面包屑改成在 `RecordUsage` 之后追加，这条降级路径的触发点就真的变了，
而按错的点名去查会找不到原因。**判定一律用 `errors.Is`，不要依赖是哪一步报的。**

但「罕见」不是「可以静默」。

### 3.9a 3.9 那条支路今天**走不到**，但仍要写（2026-09-22 现查）

3.9 处置的是「在飞记录指向的会话已经不存在」。**现查：Council 里没有任何一条
删会话的路径** —— 全仓 `grep -rn "DELETE FROM conversations|DeleteConversation|DropConversation"`
**零命中**，REPL 也没有 `/delete` 这类命令。`participants` 是软删（`active=0`），
`/leave` 不动 `conversations` 那一行。

**所以这条支路今天只能由「有人手工改了 SQLite 文件」产生。**

**仍然要实现**，理由不变（3.9 自己写了：「罕见」不是「可以静默」），
而且成本很低。但**验收的写法要跟着改**：

- **不许**写成「跑 `/delete` 再重启」—— 没有那个命令，那条用例写不出来，
  硬写只会变成一条测不到东西的空用例。
- **要**写成：直接造库 —— 先写一条在飞记录，再用裸 SQL 删掉 `conversations` 那一行，
  然后跑 `SettleLostTurns`，断言「在飞记录被删掉 / 没有面包屑 / 没有 `turn_usage` 行 /
  Err 行点名」。

**两条结构性事实，落地时要用到**：

1. `inflight_turns.conv_id` **没有** `REFERENCES conversations(id)`
   （v5 DDL 现查，只有 `TEXT NOT NULL`），所以会话行没了**不会**连带清掉在飞记录 ——
   3.9 那个「行永远卡在那里」的场景确实成立。
2. `turn_usage.conv_id` **有**外键（v2 DDL），`Tx.BeginTurn` 另外还手动调
   `requireConversation`。所以「照样补一行 `turn_usage`」在这一支**真的插不进去**，
   3.6 的通则在这里确实有例外，3.9 说得对。

**哪天加了删会话的命令，这条注记作废**，那时 3.9 从「手工造库才到得了」
变成「用户敲一条命令就到得了」，验收要连带升级成真机探针。

### 3.10 在飞记录的上限与重复崩溃（提案里没有）

连着崩三次就是三行，每次启动全报一遍。加上 3.8（settle 失败时行会留着）,
这件事更要紧。

**裁决**：横幅**最多逐行点名 10 条，其余折叠成一个计数**。
固定首行的格式仍然可钉（见 3.11）。

### 3.11 横幅与 `/session`

横幅第一行**固定格式、可钉**：

```
注意: 有 <N> 轮在上次退出时没记上账，已把相关成员的成本基线标为可疑
```

之后逐行点名 会话 / 成员 / turn id / 起始时刻 / `phase` / 孤儿 id（或「点不出名字」）/
工作目录（`tier != read` 时），这部分按 `11a` §7「随便改，测试不许钉」处理。

`SessionInfo` 加 `BaselineSuspect bool`；`/session` 多一列 ——
否则用户看到一个「金额：不知道」时无从知道为什么。

**库是干净的时候整个输出里不出现「在飞」「没记上账」任何字样**
（照 `TestStartupDriftAuditSilentWhenNoDrift` 的体例，不许制造噪声）。

**2026-09-27 落地（A 之五，`8c00ec9`）**：`SessionInfo.BaselineSuspect` 已加（内存分支读 `sessionState`，另两个分支读库）；
`/session` 在 NEXT 之前插一列 **`BASELINE`**，格值 `suspect` / `ok`，没有会话的行印 `-`（Griffin 2026-09-27 裁，与既有英文列名同体例）。

### 3.11a 横幅固定首行会在全 `begun` 时说谎，改成两个数（2026-09-22）

3.11 把首行钉成：

```
注意: 有 <N> 轮在上次退出时没记上账，已把相关成员的成本基线标为可疑
```

**后半句对 `phase == 'begun'` 的行不成立**：3.3 规定 `begun` 只删行、只记一行诊断，
**不标 `baseline_suspect`、不发面包屑**。一次启动若捞到的全是 `begun`，
这行字就是在报告一件根本没发生的事 —— 而横幅存在的理由正是「把静默的事情变响亮」，
它自己先说一句假话是最坏的开头。

**裁决：首行报两个数，第二个数是真正被标可疑的轮数。**

```
注意: 有 <N> 轮在上次退出时没记上账，其中 <M> 轮已把相关成员的成本基线标为可疑
```

- `N` 是本次对账处理掉的在飞记录总数（三档 phase 全算，含 3.9 那种账补不上的）；
- `M` 是**真的把某一行 `provider_sessions.baseline_suspect` 置成 1 的轮数** ——
  不是「`phase != begun` 的轮数」。两者不等：bootstrap 轮崩溃（3.6）
  `phase` 是 `spawned` 但**根本没有 `provider_sessions` 行可标**，
  它补得出 `turn_usage`、标不出可疑，**不能计进 `M`**。
  3.9 那一支同理，`M` 也不加。
- `M` 由 `SettleLostTurns` 逐条如实回报（见该方法的返回值），
  **不许在横幅这一层用 phase 反推**。反推就会在 bootstrap 那一支多报一次。

**仍然可钉**：这一行的格式照旧冻结，测试可以钉死，只是多一个占位。
`M == 0` 时照常打印「其中 0 轮」，不做特判 —— 特判会多出一条分支，
而那条分支恰好只在最容易被忽略的情形下生效。

**3.10 的折叠规则不变**：逐行点名最多 10 条，其余折叠成一个计数。
`N` 是总数，不是被点名的条数。

**库干净时整个输出不出现这些字样**（3.11 原文，一个字不改）：
`N == 0` 时连首行都不打印。

### 3.11b 对账排在漂移审计之前：顺序对，理由错（2026-09-22 订正）

A 片之四的任务书 4.4 写「`AuditInflight` -> `SettleLostTurns` -> `writeLostNotice`
排在 `AuditDrift` 之前，**因为漂移审计读 `provider_sessions`，而 settle 会往那张表写
`baseline_suspect`；读排在写之前，漂移横幅会打出过期状态**」。

**后半句是假的。** 现查 `drift.go` 的 `auditDriftAgainst`：它从
`ProviderSession` 上只读三个字段 —— `ConvID`、`AgentID`、`IdentityFP`。
**`BaselineSuspect` 它一个字都不读。** 而 settle 是整行读回、只改 `BaselineSuspect`、
整行写回，`IdentityFP` 逐字节不变。

**所以两者调换顺序，漂移横幅的内容逐字节相同**，不存在什么「过期状态」。
A 片之四的测试腿独立发现了同一件事，并且如实写进了它的报告
（它的顺序用例断言的是两行在输出里的**位置**，不是内容，
并注明「a swapped call order would produce byte-identical banner content」）。

**顺序仍然按原样落地，但理由换成这两条**：

1. **给人看的次序**：先说「上次退出时发生了什么」，再说「下次 `/ask` 会发生什么」。
   丢账那条更急，而且是既成事实；漂移那条是预告。倒过来读着别扭。
2. **前向依赖，写下来是为了不被随手调换**：`AuditDrift` 今天不读
   `baseline_suspect`，**哪天它读了**（例如漂移行想顺带标出「这条的基线还可疑」），
   顺序就从「好看」变成「正确性」。现在把依赖方向写明，那一天就不必重新推一遍。

**代码注释里不许再写「否则漂移横幅会打出过期状态」** —— 那是一句
听起来对、现查不成立的理由，和它要防的 bug 一样隐蔽：
**理由写错了不会让任何测试变红。**

**这条同时是对测试腿那条用例的背书**：它只能断言位置、不能断言内容，
那不是用例弱，是**这件事今天确实只有位置这一个可观测面**。
如实写下限制，比编一个内容判据强。

### 3.12 取消路径要写出来（提案一句没提）

`runner.go` 把 `context.Canceled` 与 `DeadlineExceeded` 一起接住走 `failClosed`,
而 `failClosed` 用 `context.Background()` 落库，**所以在飞记录会被清掉**。
设计里一句没提，读的人无从知道这是想过的还是碰巧的。

**写进契约**：

- `BeginTurn` 传的是**调用方的 ctx**。在 spawn 之前这是对的 ——
  取消就该连在飞记录也不写。**不许「顺手」把它改成 `Background()`。**
- 清理路径（`failClosed` / `persistTurn`）用 `context.Background()`,
  理由与它已有的注释相同。
- **`11d` 3.4 把成功路径的落库也改成 `Background()` 之后**,
  「provider 返回了但 ctx 已死」那条路不再丢在飞记录。

  **但这是一条前向引用，落地时要先核它成立没有。** 本契约 1.1 允许 A 片
  在 `11d` 8c-2 之前落地；那种顺序下**这句话在落地当时是假的** ——
  成功路径仍用调用方 ctx，一次 Ctrl-C 会让在飞记录留下来、
  下次启动把一轮 provider 确实说过话的轮报成 `spawned`（而不是 `returned`）。

  **处置**：A 片的实现里，`phase = 'returned'` 那次写入**也必须用
  `context.Background()`** —— 它不依赖 `11d`，A 片自己就能把这条补上。
  于是无论两步谁先落地，`returned` 这个事实都记得住。
  **`11d` 8c-2 因此是 A 片的软依赖，不是硬依赖**，本节把这件事写明白。

### 3.13 `r.svc == nil`

`SessionInfo` 显式判了 `if r.svc == nil`，说明这个状态是预期内的（测试会这么构造）。
`persistTurn` 今天没判，算既有隐患；但 A 片在**每一轮的主路径**上新加了一个
不设防的 `r.svc` 调用点，把边角隐患挪到了热路径。

**裁决**：`BeginTurn` 与两处 phase 更新前各补一句判，照 `SessionInfo` 的既有体例。
把 nil svc 改成构造期错误是更好的修法，但那会红掉一批测试，**本片不做**,
记进剩余项。

### 3.13a `r.svc == nil` 的判不加：它的前提不成立（2026-09-22 订正）

3.13 的论证是：「A 片在**每一轮的主路径**上新加了一个不设防的 `r.svc` 调用点，
把边角隐患挪到了热路径」，据此要求 `BeginTurn` 与两处 phase 更新前各补一句 nil 判。

**现查，那个前提是假的。**`askUpToWith` 在到达 `BeginTurn` 的锚点之前，
**已经无保护地解引用过 `r.svc` 至少两次**：

- `runner.go:469` -> `r.readAfter(ctx, convID, st.cursor)` -> `r.svc.History(...)`，
  **每一轮都走**，没有任何 nil 判；
- `runner.go:424` -> `r.svc.ProviderSession(...)`（进程内还没有快照时走，即每对
  `(conv, agent)` 的首轮）；
- `runner.go:505` -> `r.svc.Conversation(...)`，同样每轮都走。

三处都在 `BeginTurn` 的锚点（`cmd.Dir` 回填之后）**上游**。
也就是说 `r.svc == nil` 的 `Runner` 走 `Ask`，在到达 `BeginTurn` 之前就已经 panic 了。
`SessionInfo` 能判是因为它是**另一条入口**，不经过这段。

**裁决：本片不加那三句 nil 判。**加了就是三个**永远走不到**的分支 ——
它们看起来在防一件事，而那件事在那个位置不可能为真；更糟的是它们会让读代码的人
以为 `Ask` 这条路径对 nil svc 是安全的，而上游三处告诉你并不是。
项目的「不交付死键」与「写断言前先问它能不能为假」是同一条纪律的两面。

**3.13 自己给的更好修法仍然成立，仍然不在本片做**：把 nil svc 改成
**构造期错误**（`NewRunner` 拒绝 nil svc），那会一次性关掉全部三处既有隐患，
代价是红掉一批用例。记进剩余项。

**如果将来把 `BeginTurn` 挪到 `readAfter` 上游**（今天不可能 —— `PlannedSessionID`
要等 `BuildTurn`），这条订正同时作废，那时 nil 判才第一次有意义。

### 3.14 `OutcomePostRejected` 第二次 `RecordTurn` 也失败（提案里没有）

第一次 `RecordTurn` 失败会整轮回滚（连同 `ClearInflight`），
之后用 `msg=nil` 再调一次。**若第二次也失败**，在飞记录留下来,
下次启动把它报成丢失 —— 可这一轮我们**确知** provider 说过话。

这条由 3.3 的 `phase = 'returned'` 承担：面包屑措辞写成
「**provider 确实说了话，正文没能落库**」，不是「provider 侧可能已经发生」。

### 3.15 A 片的验收

**单测**

1. v4 库跑 v5 迁移：原有值逐字节不变、`baseline_suspect` 读出 0、
   `inflight_turns` 空、`IdentityFingerprint` 与迁移前相等。
2. **`v5Shape` 不是别名**：改 `v5Shape` 之后断言 `v2Shape` / `v3Shape` / `v4Shape` 不变。
3. **调用序列**（判据是序列，不是最终状态 —— 两种实现下最终状态一样）：
   用 `recordingStore` 断言一轮成功时是
   `BeginTurn(自开 tx) -> phase=spawned -> spawn -> phase=returned ->
   WriteTx 打开 -> AppendMessage -> RecordUsage -> SetProviderSession -> ClearInflight -> 提交`。
4. **`BeginTurn` 严格早于 spawn**；另造一个 `BeginTurn` 报错的假 Service,
   断言 `Outcome == OutcomeRefused` 且**假 Spawner 一次都没被调用**
   （判据是「进程没起」，不是错误文本）。
5. `ErrInflightMissing`：造一个「不调 `BeginTurn` 直接 `RecordTurn`、
   `expectInflight=true`」的调用，断言 `errors.Is` 认得出且**库里没有新增 `turn_usage` 行**；
   对照组 `expectInflight=false` 断言正常提交。
6. **三档 `phase` 各一条 settle 用例**：
   `begun` -> 只删行、**不标 `baseline_suspect`、不发面包屑**；
   `spawned` -> 标、发、补 `turn_usage`（`OutcomeLost`、token/cost 全 NULL）；
   `returned` -> 同上但面包屑措辞**不同**（3.14）。
7. **bootstrap 轮崩溃**（`provider_sessions` 行不存在）：
   断言**照样补出一条 `OutcomeLost` 的 `turn_usage` 行**（3.6 的直接断言）,
   且不报错。
8. **孤儿点不出名字**：`OrphanID == ""` 时面包屑里有「点不出名字」那句。
9. **会话已不存在**：断言在飞记录被删掉、`turn_usage` 补上了、
   没有面包屑、Err 行点名（3.9）。
10. **settle 失败则该成员 `/ask` 被拒**：留一条在飞记录不删，
    断言下一轮 `Ask` 在起进程之前 refuse（3.8）。
11. **3.7 的收口判据**（Fresh、游标没推、`CostUSDDelta == nil`）。
12. 横幅：塞一条在飞记录再跑 `app.Run`，固定首行出现且 N 正确；
    **库干净时整个输出不出现那些字样**。
13. **超过 10 条时折叠**（3.10）。

**真机**（**两条，不是一条** —— 提案那一条与它自己的设计矛盾）

14. **探针 2a（resume 轮崩溃）**：先成功跑一轮 `/ask`，再敲第二次 `/ask`,
    在子进程还在跑时 `taskkill /F`（**不加 `/T`**）。重启后断言
    `baseline_suspect == 1`、游标没推、下一轮金额报「不知道」。
15. **探针 2b（bootstrap 轮崩溃）**：新会话**第一次** `/ask` 就杀。
    断言的是 **`turn_usage` 那条 `OutcomeLost` 行与面包屑**,
    **不是** `baseline_suspect`（那一行根本不存在 —— 提案的探针在这里必然看到一个假失败）。
16. **claude 的 bootstrap 崩溃**（提案的探针只用 grok，唯一填得上
    `planned_session_id` 的一家）：断言面包屑老实说「点不出名字」。
    **不做这一条就把 3.6 那一格标 `[未验证]`**，不许默认它成立。

探针结果落 `E:\Isolating_test\council\step10-crash\RESULTS.md`。

### 3.16 入场费：零 —— 但这句话有边界

送给 provider 的字节一个都没变：`turn_lost` 面包屑是 `KindSystem`,
而 `KindSystem` 被 Router 挡在 prompt 之外，
`runner.go` 还明写「下次再扫一遍被过滤掉的 `KindSystem` 是免费的」。
schema v5 与 `baseline_suspect` 都不进 `IdentityFingerprint`
（指纹取自 role / model / effort / tier / 两段规则 / prefixArgs，与 `provider_sessions` 的列无关）。

**A / C / D 三片零入场费成立，B 片改的是本地读键盘、同样零。**

**但这句话不是整个 Step 10 的通行证**：Step 9 片 2 的 v4 同样零入场费,
而 `11c` 的 `agentWorkDir` 按会话分目录**是要付**的（`oq-cwd-prefix` 量过）。
提交说明里把这两笔分开写，否则下一个人会把账算错边。

---

## 4. 契约面改动清单（动手之前要点头）

| 改动 | 落在哪 | 处置 |
|---|---|---|
| `RecordTurn` 从「四件事」变「五件事」，并加 `expectInflight` 参数 | `10a` §2.4 | 留订正记录，**不许埋在代码注释里** |
| `10a` §2.4「唯一允许写 `provider_sessions` 的入口」变两个 | `10a` §2.4 | 写明启动时跑这条结构性豁免（1.4） |
| `ProviderSession.BaselineSuspect` | `10a` 字段表 + `types.go` | 明写它**不是「第六样」**（1.4） |
| 新哨兵 `ErrInflightMissing` | `10a` | 登记 |
| 新结局 `OutcomeLost` | `09a` | **钉死 `OutcomeLost Outcome = 9`**（2026-09-22 订正，见 4a 第 5 条） |
| `SessionInfo.BaselineSuspect` | `09a` | 与 Step 9 的 `SessionInfo.InFlight` 一批登记 |
| `Service.TurnUsage`（D 片） | `10a` | `TurnUsage` 类型已冻，这里只加读法 |
| `09a` 的 event 取值注释过期 | `09a` + `record.go` | 见下 |

**event 取值注释要补的是 `09a` 里那两处**（`09a` 的 `Meta["event"]` 说明段
与 `MetaEvent` 注释），两处今天都只列 `session_opening` / `turn_failed` /
`post_rejected`，缺 `session_rebuilt` / `round_started` / `round_finished` / `turn_lost`。
这次连 `turn_lost` 一起补齐。

**2026-09-22 订正**：本节原本还指控 `internal/turn/record.go` 的 `MetaEvent` 注释
「同样漏了 `session_rebuilt`」并引坐标 `record.go:116`。**现查不成立** ——
那条注释今天在 `record.go:151`，正文里已经含 `session_rebuilt`、`round_started`、
`round_finished`。片 3b 落地时补上的。所以**只有 `09a` 的两处要改**，
`record.go` 那处只需在本片补 `turn_lost`。详见 4a 第 8 条。

### 4a 主代理的点头记录（2026-09-22，A 片开工前）

§9 写着「第 4 节那张契约面改动清单要先点头再动手，照未决 #18 的先例」。
逐条过一遍，结论如下。**九条里七条照原样通过，两条改了。**

| # | 改动 | 裁决 |
|---|---|---|
| 1 | `RecordTurn` 四件事变五件事 + `expectInflight` | 通过。参数由 turn 侧算好传进来，**不塞进 `TurnUsage` 结构体** —— 它是控制标志，不是要落库的数据 |
| 2 | `10a` §2.4「唯一入口」变两个 | 通过。豁免理由必须写成**结构性**的：`SettleLostTurns` 只在启动时、进 REPL 之前跑一次，与 `RecordTurn` 结构上不可能并发。**哪天它能中途调用，豁免同时作废** —— 这句要写进 `10a` |
| 3 | `ProviderSession.BaselineSuspect` | 通过。`types.go` 里要明写它**不是「第六样」**：它是关于那五样的**标记**，「五样同生共死」的读回规则一个字不改 |
| 4 | 新哨兵 `ErrInflightMissing` | 通过 |
| 5 | 新结局 `OutcomeLost` | **改了。**原措辞「加在 const 末尾，已有 iota 不许动」与 §3.6「**钉死数字，不写『加在末尾』**」正面冲突。以 §3.6 为准：`OutcomeLost Outcome = 9`。依据是 `record.go` 的 const 块注释早已写明「9 留给 `OutcomeLost`，不许占用」，`11d` §5 也已写死 `= 9` |
| 6 | `SessionInfo.BaselineSuspect` | 通过。与片 3 的 `SessionInfo.InFlight` 一批登记进 `09a` |
| 7 | `Service.TurnUsage`（D 片） | 通过。`TurnUsage` 类型已冻，只加读法 |
| 8 | `09a` 的 event 取值注释过期 | **改了。**原文还指控 `internal/turn/record.go` 同样漏了 `session_rebuilt`，引坐标 `record.go:116`。**现查不成立**：那条注释今天在 `record.go:151`，已含 `session_rebuilt` / `round_started` / `round_finished`（片 3b 补的）。所以只有 `09a` 两处要改；`record.go` 那处本片只需补 `turn_lost` |
| 9 | 连 `turn_lost` 一起补齐 | 通过 |

**不许做的（§8 已否决，这里再钉一次）**：不加 `Tx.SetBaselineSuspect` 单字段 setter
（`types.go` 那条「不给任何一个字段单独的 setter」一个字不改，走整行读回改写回）；
不在 `conversation` 里读 `turn.OutcomeRefused`（反向导入是编译环）；
不用一个 `spawned` 布尔代替三态 `phase`；
bootstrap 轮崩溃**照样补一条 `OutcomeLost` 的 `turn_usage`**（那正是最贵的一轮）。

### 4b B 片那几节的口径已经过期（2026-09-22 订正）

§1.3 的标题与正文（「B 片引的那个探针**从来没有跑过**」「`oq12-paste\` 下
没有 `RESULTS.md`」「不存在任何测量」）是**契约初稿的快照**，
被其后追加的 §1.3.1 到 §1.3.6 推翻了，但原文一字未删。
同一口径还留在 §5 的「硬前置」、§8 表的最后一行、§9 落地顺序表的 B 行，共四处。

**现查（2026-09-22）**：

- `E:\Isolating_test\council\oq12-paste\RESULTS.md` **存在**，8548 字节，
  mtime 2026-09-21 21:53，正文含「已跑：A 组」与
  「结论：熟模式下拿得到括号粘贴标记，走 B-C 分支」。
- `docs/00_DECISIONS.md` 的未决 #12 今天写的是
  「**已实测，已裁：走括号粘贴识别（B-C）**（2026-09-21）」，
  而不是 §1.3 说的「需要实测，探针已设计」。
- **B 片的生产代码已经落地**：`03dcef4`（`cmd/council/main.go` 与
  `internal/repl` 五个文件，248 行）、`10124a9`（`paste_test.go` 318 行）。

**所以那四处「硬前置未满足」的口径全部作废**，原文按本项目体例保留
（观点被推翻时写明当时是怎么想的，不抹掉）。§5.3 的 B-B 分支同样作废，
保留为被否决记录（§1.3.5 已这么裁，但 §0 的包表还写着「二选一」，一并作废）。

**B 片真正还欠的两件**（与「硬前置」无关，是落地之后的验证）：

1. **多行分块形状没实测。**`RESULTS.md` 里粘的是一行；
   「整段一个 chunk 还是逐行交付」会决定 `splitPasteMarkers` 的签名形状。
2. **Windows Terminal 下没拿真粘贴跑过。**§1.3.4 要求 WT 与应用自带面板
   两处各跑一次，而 §1.3.5 的裁决只凭一处非 WT 的结果。WT 仍是 [未验证]。

两件都**必须由人在真终端里跑**（探针在 stdin 被重定向时退出码 3，
`README.txt` 与 `RESULTS.md` 抬头都写明了）。台子已经搭好：
`oq12-paste.exe` 存在且是修好之后编的（`main.go` 里 `2004` 出现 24 次，
六组 A/B/C/A2/C2/B2 都在）。跑法见该目录的 `README.txt`。

**这两件不阻塞 A / C / D 片。**

---

## 5. B 片：粘贴（未决 #12）

**硬前置**：`E:\Isolating_test\council\oq12-paste\RESULTS.md` 存在（1.3）。
**在它存在之前，本片不动一行生产代码。**

### 5.1 先修探针

现在这份探针从头到尾没有写过 `ESC[?2004h`，也没给 stdout 开
`ENABLE_VIRTUAL_TERMINAL_PROCESSING`。「有没有申请 mode 2004」是一个**没有被控制的变量**,
于是照现在这份跑出来的结果**不可解读**：分不开「Windows 控制台做不到」与「我们从来没申请过」,
而这两种读法通向的处置一个是最便宜的一条路、一个是最贵的一条路。

**不动 A 组的定义**（它是今天 council 实际跑在什么模式下的唯一对照），补三组：

| 组 | 设置 |
|---|---|
| A2 | 只申请 2004，输入句柄一个字不动 |
| C2 | 2004 + `ENABLE_VIRTUAL_TERMINAL_INPUT` + **保留行缓冲** |
| B2 | 2004 + raw |

写 2004 之前**必须先给 stdout 开 `ENABLE_VIRTUAL_TERMINAL_PROCESSING`**,
否则 conhost 会把那串字面打出来；退出前写 `ESC[?2004l` 并把两个句柄的模式**原样放回**。

**判据只有一条，是收到的字节**：粘一段三行以上文本之后,
读到的第一段里**有没有 `ESC[200~`**。不是退出码、不是「看起来正常」。

分支选择：**C2 看得到 -> 走 B-C；只有 B2 看得到 -> 走 B-B；六组全看不到 -> 走 B-B。**

### 5.2 B-C 分支（仅当 C2 组看得到标记）

- `internal/repl/paste.go`：`func splitPasteMarkers(line string) (text string, start, end bool)`,
  **纯字符串处理，不碰系统调用**，所以能被单测钉死。
- `internal/repl/console_windows.go` + `console_other.go`（build tag 拆分,
  照 `store/lock_windows.go` 与 `lock_other.go` 的先例）:
  `func enablePaste(in, out *os.File) (restore func(), ok bool)`。
  `GetConsoleMode` 失败（stdin 被重定向）时返回 `ok=false` 与一个空 `restore` ——
  **这就是 isatty 判断本身**，不要把 `mattn/go-isatty` 从 indirect 提成 direct。
- 调用点放 `cmd/council/main.go` **而不是 `app.Run`**：那两个包的注释都承诺过
  不需要 TTY，而 `enablePaste` 要的是具体的 `*os.File`，`opts.In` 只是 `io.Reader`。

**零新依赖**（`golang.org/x/sys` 已是直接依赖，需要的常量全在）。

**两个边角，提案没写，必须写**：

1. **非 Windows 恒关**：`console_other.go` 只能恒返回 `ok=false`,
   于是在括号粘贴本来最好使的平台上这个功能永远关着
   （Unix 终端靠 `ESC[?2004h` 就能开，不需要任何 ioctl）。
   本项目是 Windows 优先，这是**设计后果，不是「未验证」** ——
   注释里要写明「本平台恒返回 `ok=false`，括号粘贴不生效」,
   别让人以为只是没验。
2. **`inPaste` 与 `inDraft` 必须是两个标志**：见到 END 标记就当 `/end` 的话,
   用户本来已经在 `/say` 草稿里、中途粘一段，END 会把他的草稿**提前发出去**。
   `dispatch` 里记一个 `inPaste`，**END 只结束 `inPaste` 那一层**;
   复用既有草稿机器时不要共用同一个布尔。

**验收**：把 `"\x1b[200~第一行\n第二行\n\x1b[201~\n"` 喂进 `New(...).Run`,
断言库里**只多了一条** user 消息、正文等于两行原文含换行；
再喂一段正文中间含一行 `/end` 字面量的，断言**仍然只有一条**消息
（括号粘贴期间 `draftEndToken` 不生效，这正是这条修法相对显式模式多出来的正确性）。
**验不了的老实说**：真终端里真按 Ctrl+V 会不会送这对标记，单测碰不到,
落地后要人拿同一个探针对着 council 本身再跑一次。

### 5.3 B-B 分支

横幅加一行「要粘贴多行，先敲 `/say` 再粘」；
`00_DECISIONS.md` 未决 #12 从「待实测」改成「已实测、已裁、留一条已知限制」。

**这条分支下 #12 的严重度不会从 high 降下来，规格里照直写,
不许用「已缓解」糊过去** —— 要不要付一个行编辑器的价，是用户的决定。

否决写行编辑器：退格、方向键、Home/End、历史、宽字符光标是一个独立项目;
引一个 readline 与 D-8 已经量出来的依赖代价正面冲突。
时间窗启发式按 D-1「不做 idle 启发式」早已排除。

---

## 6. C 片：三条具名缺口，不是覆盖率

**不追覆盖率数字**：这个项目已经有背对背写的契约测试、conformance 源码扫描、golden,
缺的是别的东西。

### 6.1 端到端烟测（C1）

**今天没有任何一条用例证明这七个包接得起来。**
`NewRunner` 全仓只在 `app.go` 的生产代码里出现一次;
`internal/app` 的测试里**零命中**（提案说「唯一命中是 `drift_test.go:78` 的一个字符串
字面量」—— 复核核出那一行是 `/ask` 的中文提示，既不匹配 `NewRunner` 也不匹配 `.Ask(`；
结论比提案写的**更强**，但那条「唯一命中」是编的）。

接不起来这件事**真机上发生过**：`08b9e5f` 改了三个签名、漏改四处调用点,
被另一个包的编译失败掩盖住提交出去了。

**形状**：标准 helper-process 套路，**不需要 `go build`、不需要找 `go.exe`**：
`TestMain` 看到环境变量 `COUNCIL_FAKE_CLI=1` 就扮演一个 provider CLI
（读 argv 与 stdin，往 stdout 打一份成功 JSON，退出 0），否则跑正常测试;
配置里 provider 的 command 写 `os.Args[0]`。
测试二进制是 `.exe` 不是 `.cmd`，不会撞 `ErrShimNewlineArg`。

**那份 JSON 要从既有的成功样本原样搬字节，不要手写** ——
`ParseTurn` 是 fail-closed 的五字段判据，手写的形状正是这条用例会烂掉的地方。

stdin 脚本：`/new t`、`/join c1`、一行裸文本、`/ask c1`、`/log`、`/quit`。

**验收全是行为判据**（直接查库）：`messages` 里 user 与 agent 各一条且 agent 那条
三个脑子列非空；`turn_usage` 恰好一行且 `outcome == int(turn.OutcomeSpoke)`;
`provider_sessions` 一行且 `cursor_seq` 等于**本轮 `readAfter` 扫到的最大 seq**
（本用例的时序下就是那条 user 消息的 seq）；`inflight_turns` 为空。

**订正记录（2026-09-22，C1 用例落地时）**：上一句原文是
「`cursor_seq` 等于 agent 那条消息之前的最大 seq」。按字面
（`MAX(seq) WHERE seq < agentMsg.seq`）算出来是 **2** —— 把本轮那条
`session_opening` 面包屑也算了进去 —— 但真机实测是 **1**。
复核 `internal/turn/record.go:85` 与 `runner.go:318-322 / 360 / 663`
之后确认**实现是对的、契约措辞是错的**：`rec.ScannedTo`
（「本轮扫描到的最大 seq，spawn 之前定下」）在 `readAfter` 之后就已经算死,
而 `session_opening` 面包屑要等到之后的 `bootstrap` 分支才被追加进 `messages`——
追加面包屑这个动作发生在「这一轮扫到哪里」被定下**之后**，
所以面包屑自己的 seq 不计入这一轮的游标；`commitSession` 落库的
`cursor_seq` 就是这个 `ScannedTo`。
订正的是措辞，不是行为：**没有改任何一行实现**。

### 6.2 崩溃对账端到端（C2）

先跑一次正常轮建起库与会话，再**直接往库里塞一条 `inflight_turns`**
（模拟崩在 spawn 与 RecordTurn 之间，`phase='spawned'`），再跑第二次 `app.Run`。

**验收**：第二次启动的横幅点名 conv / agent / turn_id；跑完 `inflight_turns` 空、
`provider_sessions.baseline_suspect == 1`、`messages` 里多一条 `event=turn_lost` 的面包屑
且 Meta 里带得住 `planned_session_id`；紧接着的 `/ask` 那一轮
`Record.CostUSDDelta == nil`、Meta 有 `cost_baseline_suspect`、
且 `Delivered` 里含崩溃那轮送过的 seq（游标没被偷推）。

`/log` 能看见那条面包屑 —— **证据是 `cmdLog` 的函数体与 `read.go` 里那条
`WHERE conv_id = ? AND seq > ? ORDER BY seq ASC` 的 SQL（确实没有 kind 谓词）**,
不是「`grep -n KindSystem commands.go` 零命中」。
常量名不出现证明不了没有过滤 —— 过滤可以写成字符串字面量、也可以在 store 那一层。
结论碰巧是对的，但用一个缺席的常量当证据，是这一整轮复核之所以存在的那个毛病的轻症。

**2026-09-27 订正（C2 落地时，探针在 HEAD `92dfc89` 上实测）**：本节有两处与今天的代码对不上，是契约过期，不是生产代码缺东西：
- 「Meta 里带得住 `planned_session_id`」：面包屑的键是 `orphan_session_id`（`record.go` 的 `MetaOrphanSession`），取值 resume id 优先、planned id 兜底。
  而本节场景要求 `baseline_suspect == 1`，这只在 resume 轮成立（有 `provider_sessions` 行可标），resume 轮的 planned id 恒为空。两件事结构上不能同时成立。
  用例因此拆成两个形状：resume 轮（claude）断言孤儿 id == resume id 且基线标可疑；bootstrap 轮（grok fresh）断言孤儿 id == planned id、不标可疑、补一条 `OutcomeLost`。
- 「第二次 `app.Run` 跑完 `baseline_suspect == 1`」与「紧接着的 `/ask`」在同一次 Run 里观察不到：`/ask` 成功时 `commitSession` 把可疑标记清零并写回。用例拆成三次 Run（正常一轮 → 只对账 → `/use`、`/ask`、`/log`），可疑标记在第三次 Run 里只能经 `sessionStateFrom` 从库里读回，正好验了跨重启那座桥。
- 落地文件 `internal/app/s10c2_crash_e2e_test.go`（三条用例：主用例、不崩溃的对照、grok bootstrap 形状）。探针 7 个变异全杀；主代理在主仓库上复做「`sessionStateFrom` 不抄 `baselineSuspect`」，主用例变红。
  `/log` 那条只证明面包屑那一行在；它带不带 `turn_lost` 标记要等 D 之二。A 之五落地后可加强：`/session` 的可疑基线列、「对账失败、行留着」时 `/ask` 被拒且不起进程。

### 6.3 未决 #14 关闭时承诺的两条负向用例（C3）

**2026-09-22 订正：本节已经做完了，下面的「今天它们不存在」是过期快照。**
`internal/conversation/reserved_sender_test.go` 现查**存在**，里面两条
（`TestPostRejectsReservedSystemSenderWithBrain` 与
`TestRecordTurnRejectsReservedSystemSenderWithBrain`）与本节要求的一字不差，
落地 commit 是 `1a5d46e`。**那一刀只订正了 6.1（C1），漏了本节** ——
与未决 #22「修了但没销」是同一天、同一种病。
原文按本项目体例保留（观点被推翻时写明当时是怎么想的，不抹掉）。

`00_DECISIONS.md` #14 把这两条写成关闭条件。今天它们**不存在**：
`grep -rn "ReservedSenderSystem" --include=*_test.go internal/conversation/` **零命中**。

新文件 `internal/conversation/reserved_sender_test.go`。
两个用例各构造 `{SenderID: ReservedSenderSystem, Kind: KindSystem, 三个脑子列带满}`,
分别送 `Service.Post` 与 `Service.RecordTurn`，断言两处都返回可被 `errors.Is` 认出的
`ErrInvalidSender`。

**变异法的坐标要现找，不要照抄 #14 原文**：#14 里那两个坐标今天已经过期
（提案照抄了，复核核出它们一处是 `CreateConversation` 里的一行 `fmt.Errorf`、
一处是注释，「照它写的变异法一步都做不下去」）。
真正的保留 sender 判据在 `Post` 与 `RecordTurn` 里各一处，`isReservedSender` 是它们共用的谓词 ——
**动手时现 grep `isReservedSender` 的调用点**。

### 6.4 `-race`：容器探针，带明确的读法限制（C4）

本机跑不起来是已核实的（`CGO_ENABLED=0`、无 gcc）。Docker 在 PATH 上。

```
docker run --rm -v <repo>:/src -w /src golang:1.27 go test -race ./...
```

**只写进 MVP 清单，不产生代码。**

**判据是行为**：要么点名文件与行报出竞态，要么全绿。
**全绿只能读作「在 linux/amd64、走 `lock_other.go` 与 `killtree_other.go` 那一支的
构建下没测到竞态」，不能读作「Windows 上那个二进制没有竞态」** —— 那是另一个二进制。

附带收益：那两个 `[未验证]` 的非 Windows 分支第一次被真的执行。

**Step 9 片 1 拆锁之后这条的价值会大涨** —— 那时才第一次有真并发。
本片先把它写成可重复的一条命令。

---

## 7. D 片：MVP 判据表与两个读回口

### 7.1 `/cost`：`turn_usage` 的**第一个生产读点**

**不是「想要个报表」**：D-4 与 D-5 把成本立成一等约束，
而承载全部证据的那张表今天**一行都读不回来** ——
本机核过 `grep -rn "FROM turn_usage" --include=*.go internal/` 在非测试代码里**零命中**。
一个把「每轮多少钱」当首要约束的系统不能只会写不会读;
MVP 判据第 8 条没有它就核不了。

```go
func (s *Service) TurnUsage(ctx context.Context, convID, agentID string, limit int) ([]TurnUsage, error)
// agentID 为空表示整条会话，按 created_at 升序。
```

`/cost` 打逐成员的：轮数、in / cache_read / cache_create / out 之和、金额之和、
冷启动轮数、各 outcome 的计数。

**加上 `OutcomeLost` 之后的三条裁决**（提案没有）：

1. **丢失轮计进「轮数」**，而且**单独一栏** —— 否则又是一次把「不知道」藏起来。
2. 它的 token 列全是 NULL，**不进任何求和**。
3. 「冷启动轮数」不含它（不知道它是冷是暖）。

**验收的要害**：造若干行 `turn_usage`（`CostUSD` 非 nil / nil / `outcome=refused` /
`outcome=lost`），断言金额合计等于第一行那个值、「金额不知道」计数正确、
refused 与 lost 各自单独计数。
**`nil` 绝不能被当成 0 加进去** —— 那正是 `computeCostDelta` 一路在防的那个谎,
用例必须直接钉它。

**Step 9 加了 `turn_usage.round_id`（`12` 6.3）** —— `/cost` 要能按 round 聚合,
否则 `/round` 的结算行在重启之后就核不了了。

**X-38bd 订正（2026-09-23，`772ce1d`，见 `00_DECISIONS.md` #38(b)(d)）**：
- 裁决 3 的「不知道冷暖」在库里不止 Lost 一种：Ingested 但 `cache_read` 为 NULL 的成功轮（usage 对象缺失），
  它的 `cold_start` 是拿没填的 0 算出来的。冷启动只数「Ingested 且 `cache_read` 非 NULL」的行。
- 「nil 不当 0 加」只做到求和这一半还不够：某一列在这个成员的适用行（非 Refused）里一个数都没报时，合计印 `?`，不印 `0`。
  部分轮次没报时照印部分和，由新增的 `USAGE_UNKNOWN`（五个 token 列全 NULL 的适用行数）限定；冷暖不确知的适用行进新增的 `COLD_UNKNOWN`。
- 屏幕上的「金额不知道」计数（`COST_UNKNOWN`）只数适用行。Refused 没起进程，不算「不知道」。`CostUnknown` 字段本身仍数 Refused，只是不再印。

### 7.2 `/log` 的标记列

`00_DECISIONS.md` #9(a) 第 8 条自己记着：`thought_dropped` 今天**没有任何读者**,
所以「不许静默丢」只兑现了机器可读那一半。`turn_lost` 面包屑同理 —— 写了没人看等于没写。
**`denied_actions` 也是同一类**（`7ab093e` 落地时进了 meta，`/log` 还看不见）。

system 消息那一行末尾打 `Meta["event"]`；任何一条消息的 Meta 里有
`thought_dropped == true` / `denied_actions` 非空时各打一个标记。措辞不进契约。

**验收要两个方向**：关掉 `store_thought` 跑一轮假 CLI（那一轮的 thought 非空）,
`/log` 看得出这条消息的 thought 被丢了；**打开开关同一位置不出现那个标记**。
缺了后半，一次「这轮本来就没 thought」的巧合会把判据骗过去。

**2026-09-27 落地订正（D 之二，`a2e3d76`；契约原文归档在 `13b_S10_D2_CONTRACT.md`）**，对本节有两处有意的偏离：
- **措辞冻结了**，推翻「措辞不进契约」：标记是这一刀唯一的可观测面，不冻结就只能断言「两行不同」，分不出打的是哪种标记。
  Griffin 2026-09-27 裁用英文键名，与库里的 Meta 键一致：` [event=<值>]`、` [thought_dropped]`、` [denied_actions]`，顺序固定，接在 TEXT 格末尾，不加列；没有标记的行与从前逐字节相同。
- **event 标记不限 system 消息**，是本节的超集：今天写 `event` 的只有三处 system 面包屑（`postSystem`、`BindTeam`、`recover.go` 的 `lostNoticeMessage`），生产上等价，少一个分支。
- `denied_actions` 用 reflect 判任意非空切片（真库读回是 `[]any`，repl 的假 store 是 `[]provider.DeniedAction`），不认 map、不印条数。正文为空时三种标记照打（生产上 grok 正文为空、thought 非空、`store_thought=false` 时真会出现）。
- 端到端用例走真 `app.Run` 与 TOML 里的 `store_thought`，顺带担保了 #9(a) 记的那处装配（`app.go` 的 `SetThoughtPolicy`）。

### 7.3 MVP 判据表

**2026-09-22 现查，动手前必读**：本契约两处引用「**MVP 判据第 8 条**」
（3.6 的 Lost 行段、7.1 的 `/cost` 段），而**全仓根本没有一份编号的 MVP 判据清单**。
`docs/README.md` 明写 `07_MVP_ROADMAP`「未写」，`docs/` 下也没有任何 MVP 文件。

**那两处是指向一份还不存在的文件的前向引用。**

**处置**：`14_MVP_CHECKLIST.md` 这一刀**才第一次建立这个编号**。所以要么

- 排编号时**有意让「金额『不知道』不被当成 0」落在第 8 条**，让那两处引用成立；
- 要么排完之后**回头把 3.6 与 7.1 那两处的编号改对**。

**二选一，不许两处都不做** —— 那会留下两句永远对不上的引用，
而下一个人会去找一份不存在的清单。


新文件 `docs/14_MVP_CHECKLIST.md`（README 路线图 Step 10 那一行指过去）。

**规矩**：每一条只许挂三种标记之一 —— `[用例]<用例名>`、
`[真机]<RESULTS.md 绝对路径>`、`[人跑一次]`。**不许有第四种，尤其不许有「显然成立」。**

**提案自己违反了这条规矩两次，复核核出来了，本契约先改掉**：

- 第 1 条「正文落库、游标推进、`/session` 报冷暖」挂在 `step8a-smoke\RESULTS.md` 上 ——
  **那份文件验的是三家对规则文件的服从**（标题就是「Step 8 第一片 真机验收结果」）,
  没有任何一处证明那三件。
- 第 2 条挂的是「`README:48`」 —— **那是路线图表格里的一句自述，不是 RESULTS.md**;
  `step7-smoke` 目录下只有 `out*` / `in*` / `err*` / `run*.cmd`，**没有 RESULTS.md**。

**处置**：这两条要么**补一份真的 RESULTS.md**（`step7-smoke` 的原始 `out_*.txt` 还在,
补写一份把游标与冷暖的证据钉上去成本很低），要么把标记降成 `[人跑一次]`。
**不许拿路线图的自述当真机证据** —— 那正是这张表要消灭的东西。

表里**必须有一条诚实声明**：

> **崩溃之后接受一次可见的重复投递，绝不接受一次静默的丢失。**

另立一张「MVP+（Step 9 在范围内时）」：一个 team 三个成员各说一轮、
Judge 出一次结论、全程成本可核 —— 按 `12_STEP9_CONTRACT.md` 的验收指过去。

### 7.4 D-11：不做 TUI，带推翻条件

写进 `00_DECISIONS.md`。**这是一条决策不是一次拖延**，要能被后来的人按条件推翻,
而不是每次接手重新推一遍。

- **否的是什么**：bubbletea 一类全屏 TUI 框架。
- **理由**：D-1（运行时的层决定 UI 的可能性）+ D-8 量出来的依赖代价
  （1 个变 11 个、二进制 3 MB 变 10 MB）。
- **代替方案**：#12 的粘贴修法 + `/cost` + `/log`，都在现有 REPL 皮肤里。
- **推翻条件**：真的需要同时盯五个成员的**流式**输出，
  或需要在一屏里做成员间的并排比对 —— 那时 TUI 才是在解决一个 REPL 解决不了的问题,
  而不是换个好看的壳。**Step 9 的扇出正好是最可能触发它的那件事**,
  所以这条要在 `/round` 真跑起来之后复看一次。

**可核的只有一条**：README 路线图 Step 10 那一行的措辞要同步改，不能还写着「TUI」。

---

## 8. 被否决的设计提案条目

| 提案的裁法 | 本契约的裁法 | 理由 |
|---|---|---|
| schema v4 | **v5** | v4 已被 `12` 片 2 占用（1.1） |
| `ErrInflightMissing` 判别式在 `conversation` 里读 `turn.OutcomeRefused` | `RecordTurn` 收 `expectInflight bool` | 反向导入是编译环（1.2） |
| `Tx.SetBaselineSuspect` 单字段 setter | **整行读回改写回** | `types.go:163` 明禁单字段 setter（1.4） |
| `AuditInflight` 公开、不取锁，理由「否则被 `/ask` 挡 5 分钟」 | **结构性只在启动时跑**，删掉那句理由 | `inflight_turns` 是轮**开始**时写的，无锁读会把在跑的轮报成丢失（3.5） |
| `SettleLostTurns` 失败照 `11a` §6 只记一行 | **受影响成员的 `/ask` 直接拒绝**，但不拦 REPL | `11a` §6 说的是纯读审计；这是写，且写的是成本闸门（3.8） |
| bootstrap 轮崩溃「只删在飞记录、不报错」 | **照样补一条 `OutcomeLost` 的 `turn_usage`** | 那正是最贵的一轮，照提案那笔钱在库里根本不存在（3.6） |
| 「孤儿会话的 id 是我们本来就知道的」 | 只对 **grok 的 fresh 轮**成立，claude/agy 点不出名字 | `build_test.go` 的三条断言（3.6） |
| 一个 `spawned` 布尔 | **三态 `phase`** | 还有「provider 返回了但正文没落库」这第三态（3.3 / 3.14） |
| 理由段把「命令行超长」列为没落过在飞记录的一支 | **删掉或改对** | 它在 `r.spawn` 里返回，而 `BeginTurn` 在它之前（3.3） |
| 崩溃探针一条（用 grok，第一次 `/ask` 就杀） | **拆成 2a / 2b，另加 claude 一条** | 提案的探针断言 `baseline_suspect`，而 bootstrap 轮那一行根本不存在 —— 必然看到假失败（3.15） |
| 引「探针跑出来的 false」「唯一测量」 | **那个探针从来没跑过** | `oq12-paste\` 下没有 `RESULTS.md`（1.3） |
| 「`internal/app` 里唯一命中是 `drift_test.go:78` 的字符串字面量」 | **零命中** | 那一行是 `/ask` 的中文提示，两个模式都不匹配（6.1） |
| `/log` 不过滤 kind 的证据是「`grep KindSystem` 零命中」 | 证据换成 `cmdLog` 的函数体与那条 SQL | 常量缺席证明不了没有过滤（6.2） |
| #14 变异法照抄原文坐标 | **动手时现 grep `isReservedSender` 的调用点** | 那两个坐标今天已经过期（6.3） |
| MVP 表第 1、2 条挂 `step8a-smoke` / `README:48` | 补真 RESULTS.md 或降成 `[人跑一次]` | 违反它自己刚立的规矩（7.3） |
| `record.go` 的 `MetaEvent` 注释「是对的」 | **它同样漏了 `session_rebuilt`** | 三处一次对齐（第 4 节） |
| 「凡是对 `ProviderSession` 做 `DeepEqual` 的用例要补字段」 | **全仓没有任何 `DeepEqual` 作用在它上面**，这笔成本是虚的 | 具名字段字面量加一个字段不用改 |

---

## 9. 落地顺序

| 片 | 前置 | 备注 |
|---|---|---|
| A | Step 9 片 2（schema v4）已落地且全绿 | 唯一 `isMinimalUseful`；技术上不依赖 Step 9 的代码，只依赖版本号（1.1） |
| B | **`oq12-paste\RESULTS.md` 存在**（人工跑） | 硬前置，探针没结果之前一行代码不写（1.3） |
| C | C1/C3/C4 可与 A 并行；**C2 依赖 A** | |
| D | A（面包屑要在 `/log` 里看得见；成本判据要 `baseline_suspect`） | `/cost` 要能按 `round_id` 聚合（Step 9 加的列） |

**每一片单独一个 commit**，message 里如实记哪几条验收通过、哪几条没跑、为什么。

**第 4 节那张契约面改动清单要先点头再动手**，照未决 #18 的先例。

---

## 10. 符号禁令

代码文件里禁用装饰性符号：emoji（U+1F300–U+1FAFF）、dingbats（U+2700–U+27BF）、
杂项符号（U+2600–U+26FF、U+2B00–U+2BFF）、带圈数字（U+2460–U+24FF）、
U+FE0F、U+200D。注释、字符串字面量、变量名都算。
放行中文、希腊字母、数学算符、普通箭头 U+2190–U+21FF。
