# Step 7 冻结契约

> 与 `10_STEP7_SPEC.md` 配套。**实现者不得改动本文件里的签名**；
> 觉得签名有问题，退回来说，不要自行改。
> 这条规矩在 Step 5 与 Step 6 各救过一次（两次都是契约自己有洞，
> 退回来改契约比实现者自行发挥要便宜得多）。

## 0. 本步改动的包

| 包 | 改什么 |
|---|---|
| `internal/store` | schema v2 迁移 + 形状声明；`Tx` 的两个新方法的实现 |
| `internal/conversation` | 两个新类型、`Tx` 接口加两个方法、`Store` 接口加一个读方法、`Service` 加一个写入口 |
| `internal/turn` | `Record` 加 `TurnID`；Runner 改为从库读回、往库写；Router |
| `internal/repl` | `/session` 显示 seq 区间（排版不在契约面） |

## 1. schema v2

### 1.1 DDL

v1 的 `turn_usage` 与 `provider_sessions` 两张表**整表重建**（Step 5 建表时明写
「本步只建表，不写方法」，所以线上不存在任何一行数据，重建是安全的；
迁移里要先 `DROP TABLE` 再 `CREATE`，不要写 `ALTER`，因为主键变了）。

```sql
CREATE TABLE turn_usage (
    turn_id             TEXT PRIMARY KEY,
    conv_id             TEXT NOT NULL REFERENCES conversations(id),
    agent_id            TEXT NOT NULL,
    provider_session_id TEXT NOT NULL DEFAULT '',
    seq                 INTEGER,
    outcome             INTEGER NOT NULL,
    input_tokens        INTEGER,
    cache_create        INTEGER,
    cache_read          INTEGER,
    output_tokens       INTEGER,
    thinking_tokens     INTEGER,
    cost_usd            REAL,
    cost_cumulative_usd REAL,
    cold_start          INTEGER NOT NULL DEFAULT 0,
    wall_ms             INTEGER NOT NULL DEFAULT 0,
    prompt_bytes        INTEGER NOT NULL DEFAULT 0,
    delivered_lo        INTEGER,
    delivered_hi        INTEGER,
    created_at          TEXT NOT NULL
);

CREATE TABLE provider_sessions (
    conv_id             TEXT NOT NULL REFERENCES conversations(id),
    agent_id            TEXT NOT NULL,
    provider_session_id TEXT NOT NULL,
    cursor_seq          INTEGER NOT NULL,
    cost_total_usd      REAL,
    have_cost_total     INTEGER NOT NULL DEFAULT 0,
    last_turn_at        TEXT NOT NULL,
    identity_fp         TEXT NOT NULL,
    spoken              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (conv_id, agent_id)
);

CREATE INDEX idx_turn_usage_session ON turn_usage(provider_session_id);
```

逐条理由，都写进 `schema.go` 的注释：

- **`turn_id TEXT PRIMARY KEY`**：一轮有了不依赖任何其它表的身份。
  v1 的主键是 `(conv_id, agent_id, seq)`，而 `seq` 只有 messages 产得出，
  于是失败轮要靠一条 `turn_failed` 的 system 消息当锚点 —— 那是把记账需求
  塞进对话记录，哪天有人决定某类失败不发面包屑，账就静默少一笔。
- **`seq INTEGER`（可空）**：成功轮指向它产生的那条消息，失败轮为 NULL。
- **`outcome INTEGER NOT NULL`**：存 `turn.Outcome` 的整数值。
  **不存字符串**：枚举的字面量不在契约面（`09a` 明写），存字符串等于把一个
  可以改的措辞焊进历史行。读出来要翻译时用 Go 那边的 `String()`。
- **七个用量列改可空**：NULL 读作「这家没报这个数」，0 读作「实测为 0」。
  Step 6 用真实字节证明了三种情况在 v1 里长得一样：agy 的 usage 里根本没有
  `cache_creation`；grok 的 `cache_creation_input_tokens` 恒为 0 而
  `cache_read` 明明从 0 涨到 20608；而三份**失败**原件里 claude 与 agy 的
  五项是**货真价实的 0**。
- **`cost_cumulative_usd REAL`**：provider 报的累计原值，与算好的 `cost_usd`
  分开存。有了它，将来对账可以不依赖差值算法的正确性。
- **`provider_session_id`**：cost 的累计作用域是 provider 会话，不是
  `(conv_id, agent_id)`。没有这一列，「这一轮属于哪个会话」在 SQL 里不可查。
- **`delivered_lo` / `delivered_hi`**：本轮真正渲染进 prompt 的 seq 区间，
  对应 `04` §7「Router 必须能回答上一轮送了什么给谁」。
  **存区间而不是列表**，因为第 2 节那条裁决保证了它一定是一段连续区间；
  存成 JSON 列表就等于默许将来有人把它变成非连续的。空批次两列都是 NULL。
- **`provider_sessions` 把游标搬了进来（`cursor_seq`）**：五样东西同生共死。
  `cursors` 表**保留不动**（Step 5 已有，`Cursors(convID)` 还在用），
  但**本步不再往它写 agent 的会话游标** —— 两处都写就有了两个事实源。
  这一条要写进 `schema.go` 的注释，否则下一个人一定会去写 `cursors`。
- **`last_turn_at TEXT NOT NULL`** 与 **`identity_fp TEXT NOT NULL`**：
  v1 的 `last_turn_at` 可空，v2 改成必填 —— 一条存在的会话必然有上一轮的时刻，
  可空只会让「读回时缺一样」这个判断多一种说不清的中间态。

### 1.2 形状声明

`Open` 在迁移成功后逐表比对 `pragma_table_info`（Step 5 加的），
所以**迁移必须自带 v2 的形状声明**（`migration.shape`），
否则迁移成功后立刻以 `store.ErrSchemaMismatch` 失败。
`TestSchemaShapeCoversEveryTable` 会堵住「加了表却忘了声明形状」。

v2 的形状里，`turn_usage` 与 `provider_sessions` 两张表按上面的 DDL 逐列列出；
其余表沿用 v1 的形状，**不要复制粘贴一份新的** —— 用 v1 的 map 做基底再覆盖两张表，
免得改其它表时漏改一处。

## 2. internal/conversation

### 2.1 两个新类型

```go
// TurnUsage 是一轮的记账，与 turn_usage 表一一对应。
//
// 七个可空的用量字段用 *int / *float64：nil 表示「这家 provider 没报这个数」，
// 指向 0 表示「实测为 0」。这两件事在 Step 6 的真实字节里都出现过，
// 用 0 冒充 nil 会让成本视图显示 agy 与 grok 从不建缓存。
//
// 2026-09-21 订正（未决 #22 落地）：nil 现在有**第二种**读法 ——
// 「这一轮从没解析过 provider 的输出，所以我们不知道」。被信号打死、
// 进程没起来、起进程之前就拒绝、stdout 连形状都认不出，这四条路径都是它。
// 两种读法在库里长得一样，要分开只能看同一行的 outcome。
// 读的那一侧（Step 10 D 的 /cost）两种都不许当 0 加进去，判据不变。
type TurnUsage struct {
	TurnID            string // 调用方铸的 v4 UUID，一轮一个
	ConvID            string
	AgentID           string
	ProviderSessionID string
	Seq               *int64 // 成功轮指向它产生的消息；失败轮 nil
	Outcome           int    // turn.Outcome 的整数值

	InputTokens    *int
	CacheCreate    *int
	CacheRead      *int
	OutputTokens   *int
	ThinkingTokens *int

	CostUSD           *float64 // 算好的单轮差值。nil 有三种读法，见 09a Record.CostUSDDelta
	CostCumulativeUSD *float64 // provider 报的累计原值，原样

	ColdStart   bool
	WallMS      int
	PromptBytes int
	DeliveredLo *int64 // 本轮渲染进 prompt 的 seq 区间；空批次两个都是 nil
	DeliveredHi *int64
	CreatedAt   time.Time
}

// ProviderSession 是一条 provider 会话在库里的全部状态。
//
// 这个结构体**必须整个读、整个写**，不给任何一个字段单独的 setter。
// 五样东西（ProviderSessionID / CursorSeq / CostTotalUSD / LastTurnAt /
// IdentityFP）同生共死：读回时缺任何一样，五样全部作废，该 agent 按 Fresh 处理。
// 理由见 10_STEP7_SPEC.md 第 4 节 —— 其中最要命的是成本基线：
// 重建意味着 provider 那边是一条新会话、累计计数器从零开始，
// 若只作废会话 id 而留着旧基线，下一轮的差值是负数，computeCostDelta 返回 nil，
// 那一笔真实花掉的钱就静默从账上消失了。
type ProviderSession struct {
	ConvID            string
	AgentID           string
	ProviderSessionID string
	CursorSeq         int64
	CostTotalUSD      float64
	HaveCostTotal     bool // false 表示还没见过任何金额（grok / agy 恒为 false）
	LastTurnAt        time.Time
	IdentityFP        string
	Spoken            bool
}
```

### 2.2 Tx 加两个方法

```go
type Tx interface {
	// ... Step 5 与 Step 6 的方法不变 ...

	// RecordUsage 插入一行 turn_usage。TurnID 冲突时返回错误，不 upsert：
	// 同一个 turn_id 写两次说明调用方把一轮记了两遍，那是 bug 不是重试。
	RecordUsage(ctx context.Context, u TurnUsage) error

	// SetProviderSession 按 (ConvID, AgentID) upsert 一条 provider 会话的全部状态。
	// 收全字段结构体，不提供部分更新的入口。
	SetProviderSession(ctx context.Context, s ProviderSession) error

	// DropProviderSession 删掉一条 provider 会话。作废走这里，
	// 不要用「写一个空的 ProviderSession」来表达作废 —— 那会留下一行
	// provider_session_id 为空串的记录，与「还没开过会话」在库里长得一样。
	DropProviderSession(ctx context.Context, convID, agentID string) error
}
```

### 2.3 Store 加一个读方法

```go
type Store interface {
	// ... 原有方法不变 ...

	// ProviderSession 读回一条 provider 会话。第二个返回值为 false 表示
	// 这个 agent 在这条会话里还没开过口（或上一次的状态已被作废）。
	ProviderSession(ctx context.Context, convID, agentID string) (ProviderSession, bool, error)
}
```

**2026-09-27 登记（Step 10 A 之五，`8c00ec9`）**：`Store` 与 `Service` 各加一个定点读
`InflightFor(ctx, convID, agentID string) (InflightTurn, bool, error)`（走 v5 的唯一索引；读不到返回 false、读失败返回 err），
`conversation` 包加哨兵 `ErrInflightConflict`（「这个 (conv, agent) 有一条不属于本轮的在飞记录」，两个来源：`askUpToWith` 的显式检查、`BeginTurn` 撞 UNIQUE 时的翻译）。
Step 10 A 片此前给 `Store` 加的在飞记录方法（`BeginTurn`、`SetInflightPhase`、`AllInflight` 等）没有在本表重抄，以 `13_STEP10_CONTRACT.md` §3 与源码为准；A 之五的逐字签名见 `13a_S10_A5_CONTRACT.md` §1.1。

### 2.4 Service 加一个写入口

```go
// RecordTurn 把一轮的全部结果写进库：追加消息（若有）、记用量、更新会话状态，
// **四件事同一个 WriteTx**，任一步失败整轮回滚。
//
// 这是本步唯一允许写 turn_usage 与 provider_sessions 的入口。
// 不要在别处直接开 WriteTx 调那两个 Tx 方法 —— 那样「五样同生共死」
// 就退化成一条靠人遵守的约定。
//
// msg 为 nil 表示这一轮没有产生发言（失败轮），此时 TurnUsage.Seq 必须是 nil。
// session 为 nil 表示这一轮不碰 provider_sessions 这一行——不写，也不删。
//
// 返回落库后的消息（msg 为 nil 时返回 nil）。
func (s *Service) RecordTurn(ctx context.Context, msg *Message, u TurnUsage, session *ProviderSession) (*Message, error)
```

**订正（2026-09-21，未决 #20，语义变更）**：本节原文写的是「session 为 nil
表示这一轮之后会话作废，此时内部走 `DropProviderSession`」。那个语义被坐实为
一个真实 bug：调用方（`runner.go`）里起进程之前就拒绝的那几支——agent 不存在、
provider 没适配器、权限档不放行、`WorkDir` 为空、指纹探针出错——传下去的
`session` 本来就是 `nil`，这只是「这一轮压根没碰过会话状态」，与「这一轮决定
作废会话」毫无关系。旧实现把这两件事共用同一个 `nil`，于是这些早期拒绝会把一条
老早建好、这一轮根本没读过的 provider 会话静默删掉：只改一个不相关的配置字段
（例如把 `permissions` 从 `read` 改成 `workspace`）就会把已经花过钱的会话抹掉，
改回来要重付一次入场费，而且没有任何一层会说（真机 repro 见 `00_DECISIONS.md`
未决 #20）。

现在 `session == nil` 只表示「不碰这一行」：内部实现是「非 nil 才
`SetProviderSession`，nil 什么都不做」，不再调用 `DropProviderSession`。真正的
作废改成调用方在做出那个决定的那一刻显式调 `Service.DropProviderSession`——
即 `internal/turn/runner.go` 里从库读回、指纹不符那一支（§3.2 另有一处措辞
同步订正）。`ErrIdentityDrift` 那一支（进程内已有记录指纹不符）不受影响：
它传的 `session` 带着非空 `ProviderSessionID`，走的是 `SetProviderSession`
原值回写，从未经过这条 `nil` 分支。

**校验（在事务内做，与 `Post` 同一套理由）**：

- `msg != nil` 时走与 `Post` 完全相同的校验路径（包括 `RequireActiveParticipant`
  与脑子列校验）。**不要为了少写几行就绕过 `Post` 的校验**：Step 6 的真机冒烟
  证明了那条路径真的会拒（`/leave` 之后那一轮），而被拒正是 `OutcomePostRejected`
  这个结局存在的理由。
- `msg != nil` 且 `u.Seq == nil`（或反之）-> 返回包了 `ErrTurnSeqMismatch` 的错误。
- `u.TurnID == ""` -> 包 `ErrMissingTurnID`。
- **三个入参带的 `(会话, agent)` 必须一致** -> 不一致包 `ErrTurnIdentityMismatch`。
  这一条是 2026-09-21 补的，由写测试的执行者指出：`msg` 用 `ConvID + SenderID`、
  `u` 用 `ConvID + AgentID`、`session` 用 `ConvID + AgentID`，三处互不相干。
  调用方一处笔误（比如把上一轮的 agent 留在 `u.AgentID` 里），消息会记在甲名下、
  用量与会话状态记在乙名下 —— **四张表各自都写成合法行，外键也拦不住**，
  事后只能靠人眼比对才看得出来。`RecordTurn` 存在的理由就是让「五样同生共死」
  由结构保证而不是靠人记住，少了这道比对它就退回成一条约定。

  两处细节，实现必须照办：
  1. 这道比对放在「与 `Post` 逐条相同」的那组校验**之后**，让更具体的错误先出 ——
     `SenderID` 为空该报 `ErrParticipantNotFound`，不该被这条笼统的比对盖住。
  2. `SenderID` 只在**非保留 sender** 时才与 `u.AgentID` 比。保留 sender
     （`user` / `system`）不是 participant，一条 system 面包屑的 `SenderID`
     本来就不等于这一轮的 agent，拿它去比是把两种身份混成一种。`ConvID` 无此例外。

新增三个哨兵，与现有的放一起：

```go
var (
	ErrTurnSeqMismatch      = errors.New("turn usage seq does not match the message")
	ErrMissingTurnID        = errors.New("turn usage carries no turn id")
	ErrTurnIdentityMismatch = errors.New("turn record parts disagree on conversation or agent")
)
```

## 3. internal/turn

### 3.1 Record 加一个字段

```go
type Record struct {
	// TurnID 是这一轮的身份，Ask 一进来就铸（v4 UUID），无论结局如何都有值。
	// turn_usage 的主键是它，于是记账不再依赖 messages 产出的 seq，
	// 失败轮也记得上。
	TurnID string

	// ... 其余字段不变 ...
}
```

### 3.2 Runner 从库读回

`NewRunner` 的签名不变。内存里的 `sess map[sessionKey]*sessionState` **保留**
（它仍是本进程内的快照），但：

- **读**：`Ask` 发现 `sess` 里没有这一条时，先从 `svc` 读回。
  **`ok == false` 与「读回报错」是两种处置，不是一种**（本行 2026-09-21 订正，
  原写「读回失败或 ok == false 时按 Fresh 处理」，把两者混成了一条）：
  - `ok == false` 表示**确定没有**这条会话 -> 按 Fresh 走，新建。
  - 读回返回 error 表示**不知道** -> **拒绝这一轮**（`OutcomeRefused`），不起进程。
    把「不知道」当成「没有」，后果是开一条新的 provider 会话、付一次入场费
    （claude 实测约 0.1 到 0.95 美元一次，取决于前缀大小），而原来那条若还在，
    就成了没人认领的孤儿会话。库读不出来时，Council 的事实源本身已经不可信，
    这时候继续花钱说话比停下来更糟。fail-closed。
- **指纹不符的处置有两个分支，不许合并**：
  - 进程内已有的那条（`sess` 里有）不符 -> **拒绝**，`ErrIdentityDrift`。
    那说明用户在会话开着的时候改了配置，静默重建会白烧一次入场费且他不知情。
  - 从库读回的那条不符 -> **作废重建**（丢弃五样，按 Fresh 走）。
    两次运行之间改配置本来就允许，此时没有「正在开着的会话」可言。
- **写**：每一轮结束时经 `Service.RecordTurn` 一次写完。
  `Ingested()` 为假的结局传 `session = 当前会话`（会话还在，只是这一轮没成）。

  **作废那一支的措辞订正（2026-09-21）**：原写「身份漂移作废那一支传
  `session = nil`」，与代码路径对不上。实际是这样，两条路都达成了「旧行没了」：
  - 从库读回的指纹不符 -> 当作没读到，按 Fresh 重建。**这一轮若成功**，
    `SetProviderSession` 是 upsert，新会话直接覆盖同一个 `(conv, agent)` 的旧行，
    根本不需要先删。
  - **这一轮若失败**，且它是 bootstrap（会话从未建立），才传 `session = nil`，
    走 `DropProviderSession` 把旧行删掉 —— 否则库里会留着一条已被判定
    身份不符、却没有任何进程认领的会话。

  换句话说 `session = nil` 的判据是「这一轮之后不该有任何会话行」，
  不是「发生了身份漂移」。前者能从 `Outcome` 与 `bootstrap` 推出来，
  后者是它的一个成因而已。

  **再订正（2026-09-21，未决 #20，这段上面整段一起作废）**：`session = nil`
  在 §2.4 已经改义为「不碰这一行」，`RecordTurn` 不再对 nil 做任何
  `DropProviderSession`。于是上面这段「这一轮若失败……才传 `session = nil`，
  走 `DropProviderSession` 把旧行删掉」不再成立——传 nil 现在什么都不删。

  真正的删行挪到了**读回的那一刻**，不再等到本轮结束：`Ask` 发现从库读回的
  指纹不符时，在 `postRebuildNotice` 发出面包屑之后、`st = &sessionState{}`
  重建之前，立刻显式调一次 `Service.DropProviderSession`。这样不管这一轮
  最终成功还是失败都对——成功时 `SetProviderSession` 的 upsert 本来就会覆盖
  （早删晚删结果一样），失败时旧行已经在读回那一刻被删掉，不会因为
  `session = nil` 现在不再触发删除而留下一条孤儿会话。

### 3.3 Router

```go
// Router 决定一批消息里哪些送给这个收件人，以及渲染成什么。
//
// **必须是纯函数。** 函数体里不得含 time.Now、随机源、游标数字、重试次数、
// 本轮序号，也不得依赖 map 迭代顺序。同一批消息 + 同一个收件人配置，
// 必须永远渲染出同一串字节。
//
// 理由是 D-4 的实测量级：打断缓存前缀时**没有任何一层会报错** ——
// agent 照常回答、退出码 0、JSON 合法、游标照推，只是每轮贵几十倍
// （冷启动 47749 token / 0.955 美元 vs 暖缓存边际 0.014 美元；
// Step 6 真机又量了一次，claude 两轮 0.1031 vs 0.0119）。
//
// 有一条 conformance 测试扫本接口实现所在的文件，断言不出现 time.Now 与 rand。
type Router interface {
	// Select 从 ms 里挑出要送给 to 的消息，按 seq 升序返回。
	//
	// bootstrap 为真时走 bootstrap 规则（新会话的首轮），否则走增量规则。
	// 两者是同一个判定点的两面：bootstrap == (ProviderSession == "")，
	// 调用方不得再引入第二个「是不是首轮」的判断。
	Select(ms []conversation.Message, to config.AgentConfig, selfID string, bootstrap bool) []conversation.Message

	// Render 把挑出来的消息渲染成正文。与 Select 分开，是为了让
	// Delivered 与真正送出去的字节来自同一次挑选（Step 6 的教训：
	// 两处各过滤一遍，游标按一处推、重发却换了另一处的正文）。
	Render(selected []conversation.Message, title string, bootstrap bool) string
}
```

`RouteThought bool` 作为 Router 实现的构造参数，**Step 7 恒传 false**，
Step 9 接 Team 配置。理由留在代码里：探针里 grok 的 `thought` 直接复述了
用户全局规则文件的内容，路由给另一个 agent 等于让后者读到前者的私下斟酌，
而那段内容里可能有路径、IP、内部约定。

**bootstrap 的挑选规则**（`10_STEP7_SPEC.md` §6.3，逐字实现）：

1. 会话标题（由 `Render` 的 `title` 参数带进来，不占一条消息）；
2. 全部 `KindUser` 消息；
3. 每个其他活跃成员在 `[0, ScannedTo]` **闭区间内的最后一条** `KindAgent` 消息。

窗口必须写死成入参 `ms` 的全量，**调用方有义务把 `[0, ScannedTo]` 的全量交进来，
不能只交某一页**。这一条要写进 `Select` 的注释：`readAfter` 是分页拉的
（`historyPage = 512`），「本页的最后一条」与「全量的最后一条」不是一回事。

## 4. 不属于契约面

实现可以自由决定，**测试不得对它们做字面断言**：

- `/session` 与 `/ask` 输出的措辞、列名列序、seq 区间的显示格式
- 错误信息的具体中英文措辞（但必须能被 `errors.Is` 认出哨兵）
- `turn_id` 的具体生成方式（只要求是 v4 UUID 且一轮一个）
- Router 实现的内部结构、文件名
- 迁移里 `DROP` 与 `CREATE` 的具体语句顺序

### 3.4 SessionInfo 的签名要改（2026-09-21，真机冒烟抓到）

`09a` 冻结的是：

```go
func (r *Runner) SessionInfo(convID, agentID string) (SessionInfo, bool)
```

**改为**：

```go
func (r *Runner) SessionInfo(ctx context.Context, convID, agentID string) (SessionInfo, bool)
```

理由：Step 6 的实现只看进程内存的快照，这在 Step 7 之后会**说谎**。
真机冒烟里，重启之后第一次 `/session` 显示那个 agent 是 fresh、会话 id 是占位符，
而那条会话明明躺在 `provider_sessions` 里 —— 紧接着的 `/ask` 就用回了它。
「还没开过口」与「本进程还没读回来」是两件事，混成一件会让 `/session`
在最需要它的时刻（刚重启、想知道下一轮冷不冷）给出相反的答案。

所以它要能回库读一次，而回库读需要 `ctx`。

**读回不产生任何副作用**：不写进 `r.sess`，也不做指纹校验。指纹校验归 `Ask` ——
那里指纹不符要作废重建，而 `/session` 只是看一眼。代价是 `/session` 显示的会话
可能在下一次 `Ask` 时因指纹不符被作废；那是如实反映库里的状态，不是错。

读失败时照实返回「没有」，**不把错误往上抛**：`/session` 是一条给人看的命令，
一行读失败不该让整张表打不出来。

## 5. 一个已知的锁形状问题（已于 Step 9 片 1 拆掉）

**原文（Step 7 时为真）**：`Runner.Ask` 在**整轮调用期间**持有 `r.mu`
（最长 5 分钟的硬超时），而 `SessionInfo` 取的是同一把锁。
**`SessionInfo` 可能阻塞在一次在飞的 `Ask` 之后**，它不是一个随时可查的只读探针。

**订正（2026-09-22，Step 9 片 1 落地）**：已经拆掉。做法是
**按 `sessionKey` 的闸**（`sessionGate{mu, st}`，`gatesMu` 只护 map 的
get-or-create，临界区里不做 IO）；`SessionInfo` 与 `Rebuild` 改用 `TryLock`——
`SessionInfo` 拿不到闸就从库读上一次提交的状态并置 `InFlight = true`，
`Rebuild` 拿不到就返回 `ErrTurnInFlight` 且什么都不做。

**理由不是「扇出之后天天撞上」。** 这三处的锁形状在单线程 REPL 下今天一次都
观察不到，把它写成「会经常发生」是编的。真实理由是：**闸拆开之后结构上允许
并发调用这三个方法，而结构允许的事情早晚会被某个调用方用上**——所以用例直接
并发调它们来把行为锁住，不假装它今天会在屏幕上发生。

`Rebuild` 拒绝在飞那一轮，理由也不只是「阻塞 5 分钟不可接受」：**即便等到了也白做**——
在飞那一轮结束时 `SetProviderSession` 是 upsert，会把刚删的行又写回来，
用户看到「已丢弃」而其实什么都没丢。拒绝是 fail-closed 的那一边。

**这一片没有拆掉的**：`r.cfg`。`SetConfig`（`/config reload --apply`）仍然裸写，
而拆闸之后并发 `Ask` 会裸读 `r.cfg.Agents[agentID]`。这是一条**已知缺口**，
单独记在 `00_DECISIONS.md`，不在片 1 的范围内。

## 6. 符号禁令（写代码时必须遵守）

代码文件里（含注释、字符串字面量、打印输出、标识符）禁用装饰性符号：
Emoji U+1F300-U+1FAFF、Dingbats U+2700-U+27BF、杂项符号 U+2600-U+26FF、
杂项符号与箭头 U+2B00-U+2BFF、带圈字母数字 U+2460-U+24FF、U+FE0F、U+200D。
放行：希腊字母、数学算符、度数正负乘除微符号、上下标、普通箭头、中文、ASCII 制表符。

**源码里不要出现任何不可见字符。** 要表示这类码位一律用十六进制数值
（例如 `const runeBOM rune = 0xFEFF`），不要用带反斜杠 u 的转义写法 ——
那种写法在传输中会被还原成真字符，Go 不允许文件中间出现 BOM，直接编译不过。
本项目已经踩过一次。
