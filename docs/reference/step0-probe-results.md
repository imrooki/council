# Council Step 0 探针结果（2026-09-20）

机器：E 盘机（LONG）。版本：claude 2.1.278 / grok 1.0.13（经 CPA，跑在局域网里的另一台机器上）/ agy 1.1.26 / codex-cli 0.147.0。
方法：每家三/两轮极短往返。轮1 存一个数字，轮2 问它是什么（验证上下文），claude 轮3 让它加 1（验证上下文内运算）。
原始 JSON：本目录 `claude-t{1,2,3}.json`、`grok-t{1,2}.json`、`agy-t{1,2}.json`。

## 问题 1：resume 是否真的保持上下文

**三家全部通过。**

| | 机制 | 轮2 答案 | 轮3 |
|---|---|---|---|
| claude | `--resume <session_id>` | 4217 正确 | 4218 正确（上下文内运算） |
| grok | `--session-id <uuid>` 预指定 + `--resume <uuid>` | 8635 正确 | 未测 |
| agy | `--conversation <conversation_id>` | 5192 正确 | 未测 |

会话 id 语义有差别，对编排有影响：
- **grok 可以预先指定 UUID**（`--session-id`，必须是合法 UUID 且不得已存在）——编排方能在 spawn 前就决定 id，最省事
- **claude / agy 只能从轮1 的输出里取回** id（`session_id` / `conversation_id`）——编排方必须解析首轮输出才能继续

## 问题 2：NDJSON / JSON 输出结构

三家的 `--output-format json` 都给出干净可解析的单对象，字段各不相同但都够用：

- claude：`session_id` / `result` / `num_turns` / `duration_ms` / `total_cost_usd` / `usage{...}`
- grok：`text` / `stopReason` / `sessionId` / `requestId` / `thought` / `usage{...}` / `modelUsage{按模型分}`
- agy：`conversation_id` / `status` / `response` / `duration_seconds` / `num_turns` / `usage{...}`

**turn 边界是显式的**（`stopReason: end_turn` / `status: SUCCESS` / 进程退出），
不需要 idle 启发式，也不需要 VT 模拟器。这是对"JSON 层拥有对话"这一路线决定的直接验证。

grok 额外暴露 `thought` 字段（思维链摘要），agy 的 `usage` 里有 `thinking_tokens`。

**这引出一个计划里没有的设计决策：`thought` 要不要进 Canonical Conversation。**
它既可能是特性（Judge 能看到 Critic 的推理过程，而不只是结论），
也可能是泄漏（一个 agent 的私下斟酌被路由给了不该看到它的同伴，
而模型在"思考"时说的话往往比它最终output的更不设防）。
Context Router 必须对此显式表态，不能默认透传。

## 问题 3：token 与成本（本次唯一的意外）

### 3.1 固定开销：每个 agent 的入场费

回答一个 "OK" 所需的 input token：

| | 冷启动 input_tokens | 说明 |
|---|---|---|
| claude | **47,749** | 系统提示 + 工具定义 + 全局 CLAUDE.md |
| grok | 20,625 | 同上，且 `thought` 里明确出现了全局 CLAUDE.md 的规则内容 |
| agy | 18,180 | 三家最小 |

这是 Council 每个成员**仅仅是存在**就要付的钱，与它说多少话无关。
按成员构成 **18k–48k / 人**，5 人团队入场费 **90k–240k token**。

### 3.2 缓存决定一切（claude 是唯一能看到金额的）

| 轮次 | cache_create | cache_read | total_cost_usd | 本轮增量 |
|---|---|---|---|---|
| T1（冷启动） | 47,749 | 0 | $0.955 | $0.955 |
| T2（resume） | 19,363 | 28,436 | $1.350 | +$0.395 |
| T3（resume） | **95** | **47,752** | $1.364 | **+$0.014** |

**冷启动 $0.955；缓存全命中且几乎无新上下文时，边际成本 $0.014。
实际每轮落在两者之间，取决于该轮路由进多少新内容。**

注意 `total_cost_usd` 是会话**累计值**（0.955 → 1.350 → 1.364），上表的"本轮增量"是我算的差值。
不要把 $0.014 当成"每轮成本"去乘轮数 —— 那是 4 个输出 token、缓存全暖、零新上下文的最好情况。
Council 每轮都会路由新 delta 进去，`cache_creation` 不会停在 95。

T2 之所以还贵，是因为 T1 的对话内容追加后缓存前缀断在 28,436 处，后半段重建；
到 T3 前缀已稳定，几乎全命中。

**缓存档位：三轮全部是 `ephemeral_1h`（1 小时 TTL），`ephemeral_5m` 均为 0。**
这既解释了为什么隔了一两分钟再 resume 仍能命中，也是一条硬约束：
**一个 agent 闲置超过 1 小时，缓存过期，下次说话要重付冷启动价。**
一场跨越午休的讨论，会在恢复时把每个成员的入场费再付一遍。

grok 与 agy 是订阅制经 CPA/自有通道，**没有 cost 字段**，只能看 token：
- grok：T1 input 20,625 / cache_read 0 → T2 input **250** / cache_read **20,608**
- agy：T1 cache_read 0 → T2 cache_read **16,282**

三家都有服务端缓存，且 resume 都能命中。

## 对 Council 设计的四条结论

1. **持久会话 + 增量 delta 的设计被成本数据正面验证。** 如果每轮新建会话，claude 侧成本最多是暖缓存边际值的 68 倍。
   计划里"Canonical Conversation 不每轮全量重发"这条核心洞察，不只是优雅，是成本上的生死线。

2. **成本的主要变量不是对话长度，而是缓存能否命中。** 任何打断前缀稳定性的做法
   （重建会话以切换 model/effort、往历史中间插消息、并发写同一会话）都会把该 agent 打回接近冷启动的价格。
   Step 9 的"不能改 live session 就重建会话"是对的，但要知道它的标价：约 $0.4–1.0 一次（claude 侧）。

3. **每个 agent 都在为你的全局 CLAUDE.md 付费。** grok 的 `thought` 字段证实它读到了其中的规则。
   那份文件是为 Claude 主代理写的（advisor 规则、代码委派链、设备表……），
   对一个"力学专家 agent"全是噪声。按文件体积 17.7 KB 粗估约 6–8k token，
   占 grok/agy 固定开销的三分之一左右（**此项为估算，未做对照实验**）。
   Council 应当让成员跑在干净的规则环境里，这是个可直接落地的优化。

4. **agy 不慢。** 轮1 3.4 秒、轮2 24 秒。ask-agy 记录的"分钟级"是它做完整 agentic 任务时的表现，
   不是这种短往返。agy 作为 Council 的一等成员在响应时间上没有障碍。

## 未测项（诚实标注）

- codex 本轮**未测**（三家已足够验证路线，且 codex 的 `exec resume` 语义与另三家不同，留给 Step 3 设计时再测）
- grok / agy 的第三轮（上下文内运算）未测
- 全局 CLAUDE.md 的 token 占比未做对照实验，仅按体积估算
- 多 agent 并发时同一 provider 的会话隔离未测
- 长上下文（数十轮后）的缓存行为未测 —— 这是下一个该测的东西
- 同一 provider 开多个 session 时，缓存前缀是否共享 —— 直接决定"同 provider 多 agent"的成本能否摊薄，
  是 Step 6 设计真正需要的那个数，且很便宜就能测
