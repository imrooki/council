# 消息协议与 Canonical Conversation

Canonical Conversation 是本地的完整事实源：谁在什么时候说了什么。
它不知道"品牌"，只知道 participant。

## 1. Participant 不等于 Provider

这是原计划里对的一条，必须在 schema 层面落实：

```
participant: mechanics-expert   provider: claude
participant: reviewer           provider: claude
participant: judge              provider: claude
```

三个独立 participant，共用一个 provider。
**消息的 sender 是 participant id，不是 provider 名。**
任何地方都不要用 "claude" / "grok" 当消息角色。

## 2. Message

```go
type Message struct {
    ID          string     // 稳定唯一 id
    ConvID      string
    Seq         int64      // 单调递增，会话内唯一，排序与游标的唯一依据
    SenderID    string     // participant id（"user" 是一个保留的 participant）

    // 这一次发言当时用的「脑子」。是消息的属性，不是 participant 的属性，理由见下。
    // SenderID 是保留 id "user" 时这三个字段必须为空（拦的是 SenderID，不是 Kind）。
    SenderProvider string
    SenderModel    string
    SenderEffort   string

    Recipients  []string   // 空 = 广播给活跃成员；非空 = 定向
    Kind        MessageKind
    Text        string
    Thought     string     // provider 返回的思维链摘要，默认存但默认不路由（见 04 §3）
    ParentID    string     // 回复/引用
    CreatedAt   time.Time
    Meta        map[string]any
}

type MessageKind string  // user | agent | system | task | task_result
```

`Seq` 由 Council 单调分配，不依赖时间戳（并发写时时间戳会撞）。

`MessageKind` 落地时定成了 `string` 而不是 `int`：council 是本地优先的工具，
用户会自己拿 SQLite 查看器开库，库里写 `"agent"` 比写 `2` 有用得多（`08` §D）。

### 脑子记在消息上，不记在 participant 上

`SenderProvider` / `SenderModel` / `SenderEffort` 看起来像 participant 的属性，其实不是。

D-4 与 `02` §4：**换 model 或 effort 必须重建会话**。所以同一个 agent 在一场对话里
可能换过脑子。若把这三样记在 `participants` 行上，换一次就会把它过去说过的每一句话
追溯性地重新标注成新脑子说的 —— Canonical Conversation 是事实源，事实源不能被这样改写。

记在消息上，每条消息各自带着它当时的脑子，换脑子只影响之后的消息。

**这三列是请求时声明的档位，不是实测生效的档位**（grok 的 effort 由它自己的 config 决定，
命令行参数会被静默丢弃 —— `02` §1.1 的 EffortPolicy「进程外配置」）。不要当 ground truth。

因换 model/effort 重建会话时，要追加一条 `system` 消息记录这次切换
（`04` §0 规则 3：重建是有标价的操作，要显式记录）。

## 3. 三家 provider 输出的归一化

Step 0 实测的三种输出形状，映射到上面的 `Message`。

**这张表曾经是错的**：它用简写代替真实键名（写 `input` 而实际是 `input_tokens`）、
说 claude 的完成信号是「进程退出」（实际有五个字段并存）、还漏了 claude 的 `modelUsage`。
2026-09-20 的 Step 6 设计合议拿七份原始探针 JSON 逐字重核过，下面是重写版。
**原始文件在 `E:\Isolating_test\council\step0-probe\`，有疑问一律以文件为准，不以本表为准。**

| Council 字段 | claude | grok | agy |
|---|---|---|---|
| Text | `result`（无尾换行） | `text`（无尾换行） | `response`（**带尾换行**，要 TrimRight） |
| Thought | 无（只有 token 数） | `thought` | 无（只有 token 数） |
| provider session | `session_id` | `sessionId` | `conversation_id` |
| 完成信号 | `type=result` + `subtype=success` + `is_error=false` + `terminal_reason=completed` + `stop_reason=end_turn`，**五个并存，缺一不可**（原因见陷阱 8） | `stopReason=end_turn`；失败时改走 `{"type":"error","message":...}` 这个**独立信封**，成功形状的字段一个都不出现 | `status="SUCCESS"`（大写）；失败时 `status="ERROR"` 且正文在 `error` 键里 |
| 单轮用量 | `usage{input_tokens, cache_creation_input_tokens, cache_read_input_tokens, output_tokens, output_tokens_details.thinking_tokens}` | `usage{input_tokens, cache_read_input_tokens, cache_creation_input_tokens, output_tokens, reasoning_tokens, total_tokens}` | `usage{input_tokens, output_tokens, thinking_tokens, cache_read_tokens, total_tokens}` |
| 金额 | `total_cost_usd`，**会话累计值** | 无 | 无 |
| `modelUsage` | 有，**会话累计值** | 有，**单轮值** | 无 |

### 八个必须知道的陷阱，每一条都能拿文件核

1. **claude 的 `usage` 是单轮、`modelUsage` 是累计；grok 的 `modelUsage` 却是单轮。**
   同名反义。证据：claude t3 的 `modelUsage.cacheReadInputTokens` = 76188 = 28436(t2) + 47752(t3)；
   grok t2 的 `modelUsage.inputTokens` = 250 = 该轮 `usage.input_tokens`。
   **单轮口径一律取 `usage`，`modelUsage` 只配用来读那个键名（即实测型号标签）。**

2. **agy 的 `input_tokens` 含已缓存部分，另两家不含。** 算术可验：
   agy t2 `total_tokens` 20482 = `input_tokens` 20348 + `output_tokens` 134，
   而 `cache_read_tokens` 16282 < 20348，说明 16282 已经包在 input 里；
   grok t2 `total_tokens` 21217 = 250 + 20608 + 359，三者相加才是总量。
   **三家直接混存同一列，数字会差一个数量级。** 这是 `turn_usage.input_tokens` 口径问题的根。

3. **缓存读的键名有三种拼法**：claude / grok 是 `cache_read_input_tokens`，
   agy 是 `cache_read_tokens`（**没有 `_input_`**）。照简写去取会静默得零值。

4. **思考 token 有三种拼法且都不在 `usage` 顶层同名位置**：
   claude 嵌两层 `usage.output_tokens_details.thinking_tokens`，
   grok 叫 `reasoning_tokens`，agy 叫 `thinking_tokens`。
   **三家实测都包含在 `output_tokens` 内**（grok 723 含 722，agy 55 含 54），
   任何视图不得把两者相加。claude 三轮 `thinking` 恒 0，包含关系 `[未验证]`，按同规则处理。

5. **agy 完全没有 `cache_creation` 这一类字段。** 不是 0，是没有。
   而 `turn_usage.cache_create` 是 `INTEGER NOT NULL DEFAULT 0`，
   存 0 会被读成「实测为 0」。这是 schema 层面的失真，见未决 #11 与 Step 6 的 J 节。
   **`ColdStart` 的判据只能是 `cache_read == 0`**，对三家一致；
   不能用 `cache_create > 0` 反推 —— grok 的 `cache_creation_input_tokens` 恒为 0，
   而它的 `cache_read` 明明从 0 涨到了 20608。

6. **claude 顶层有两个 UUID**：`session_id` 三轮不变（会话 id），
   `uuid` 每轮都变（这条 result 记录的 id）。抄错一个的后果是每轮冷启动、
   每次约 0.955 美元，而且**全程不报错**。

7. **失败不一定产出 JSON。** `--output-format json` 只约束成功路径。
   实测 `claude --resume <不存在的 id>` 的 **stdout 是 0 字节**，错误只在 stderr 纯文本里；
   而 grok 拿到一个本地不认识的会话 id 时连错都不报，它拉起交互式 OAuth 设备码流程
   无限阻塞（4 分钟只吃 0.6 秒 CPU）。所以解析层的第一道闸是「stdout 空吗」，
   而不是「JSON 里写着什么」；编排层必须把硬超时当主路径，不是保险。
   证据：`E:\Isolating_test\council\step6-probe\STEP6-PROBE-RESULTS.md`。

8. **失败时的 JSON 会拿成功字段骗你。** 三份失败原件已留档（2026-09-21，
   `step6-probe/{claude,grok,agy}-badmodel.out`，都是给一个不存在的 model 名触发，
   三家退出码都是 1）。逐条：
   - **claude 在一次失败的轮次里 `subtype` 仍然是 `"success"`。** 同一份输出里
     `is_error=true`、`terminal_reason="api_error"`、`api_error_status=404`（**整数**，
     不是对象也不是 null）、`stop_reason="stop_sequence"`（第四个取值）、`modelUsage={}`，
     而 `result` 装的是错误正文。**只看 `subtype` 会把 404 判成成功并把那段错误正文
     当成 agent 的回答写进只追加的事实源。** 上面那张表要求五个字段并存，就是为了它。
   - **grok 有错误信封**，此前写的「无任何错误字段」是错的，只是没触发过：
     `{"type":"error","message":"..."}`，成功形状的五个字段全是零值。
     不认这个信封就会把一条读得懂的错误判成「连结构都认不出」。
   - **agy 的失败正文在 `error` 键里**，`status="ERROR"`、`response` 与
     `conversation_id` 都是空串。不取那个键，这一轮就只剩一个 `"ERROR"` 字样。
   - **`usage` 全零可以是真实值。** 这三份里 claude 与 agy 的 `usage` 键都在、
     五项都是货真价实的 0，`total_cost_usd` 也是 0。所以**不许**用「全零」反推
     「这家没报用量」；「没报」唯一可靠的判据是键本身缺席（见陷阱 5 的 agy）。

   仍然零观测的是另一类：**provider 已经把 delta 吃进会话之后才失败**。
   上面三份都是请求发出**之前**被拒的（用量全零、金额为零即是证据），
   不能拿来当那一类的证据。解析失败分支继续 fail-closed 写：认不出来的就算失败。

**只有 claude 报金额**（`total_cost_usd`，且是**会话累计值**不是单轮）。
grok 与 agy 走订阅，只有 token。
所以 Council 的成本视图必须是「claude 侧有钱、其余侧只有 token」的混合体，
不要伪造一个统一的折算价；`turn_usage.cost_usd` 可空，对后两家一律写 `NULL` 而不是 0。

## 4. TurnUsage

每轮调用都记下来，这是 Council 唯一能做成本管理的抓手：

```go
type TurnUsage struct {
    AgentID        string
    Seq            int64     // 本轮产生的消息 seq
    InputTokens    int
    CacheCreate    int
    CacheRead      int
    OutputTokens   int
    ThinkingTokens int
    CostUSD        *float64  // 仅 claude 有；且注意它是会话累计值，存差值
    ColdStart      bool      // CacheRead == 0
    WallMS         int
}
```

`ColdStart` 单独标出来，因为它是成本的主要跳变点（D-4）。
一个会话里出现意料之外的 `ColdStart`，就是缓存前缀被打断的信号，应当告警。

## 5. 持久化

SQLite，单文件，**放数据目录下，不是配置目录**。
具体位置由 `config.ResolveDataDir()` 的返回值决定（D-9：`data_dir` > `COUNCIL_DATA_DIR` >
`E:\council-data`），Step 4 已经把这个解析实现好了，Step 5 直接用它的返回值当库的根，
不要另起一套路径逻辑，更不要写进 `~/.council/`。表：

- `conversations(id, title, created_at, ...)`
- `participants(conv_id, agent_id, display_name, provider, active, joined_at, left_at)`
- `messages(id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort,
  recipients, kind, text, thought, parent_id, created_at, meta)`
- `cursors(conv_id, agent_id, seq, pending, updated_at)`
- `turn_usage(conv_id, agent_id, seq, ...)`
- `provider_sessions(conv_id, agent_id, provider_session_id, last_turn_at, cold)`

必需操作：建会话、重开会话、加/停用 participant、列 participant、
加消息、按序读、读 seq 之后的、读写 cursor、重启后恢复。

**移除一个 agent 不得删除它的历史消息。** participant 表用 `active` + `left_at` 软删除。

**不要抄 many-ai-cli 的 sessionstore 实现** —— 它把审批问答未脱敏地写进 SQLite，
且保留期清理有一条失效链（会话日志默认关 → 目录不存在 → `ReadDir` 失败早返回 →
`PruneOlderThan` 永不执行，`maintenance.go:98-101`）。知道有这个坑，自己写干净的。

## 6. 保留与脱敏

- Council 存的是自己的对话记录，不存任何 provider 凭据
- `thought` 字段存但默认不路由（`04` §3）；若用户要求不存，提供开关
- 保留期清理的实现必须有独立测试，且不能像 many-ai-cli 那样嵌在另一个早返回之后
