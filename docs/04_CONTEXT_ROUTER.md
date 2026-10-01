# Context Router

Canonical Conversation 是本地完整事实源，但**不是**每轮发给模型的 prompt。
Router 决定：每个 agent 该看到什么、什么时候看到、看到之后游标推进到哪。

## 0. Router 的首要职责不是"选内容"，是"别弄坏缓存"

**这是 Step 0 实测带来的、原计划里没有的第一约束。**

实测（claude，`STEP0-RESULTS.md`）：冷启动 47,749 token 建缓存、$0.955；
前缀稳定后边际 $0.014。两者相差最多 68 倍。
决定成本的不是对话有多长，而是**这个 agent 的上下文前缀是否保持追加式增长**。

由此得出三条硬规则：

1. **只追加，不改写。** 送给某 agent 的内容只能加在它已见内容之后。
   任何"重排历史""插入补充说明到中间""修正之前送错的消息"的做法都会打断前缀，
   把它打回接近冷启动。要修正，就在末尾追加一条修正消息。
2. **每个 agent 的输入序列是独立的一条流。** 两个 agent 即使来自同一 provider，
   也各有各的 ProviderSession 和各自的前缀。不要试图共享。
   （同 provider 多 session 能否共享前缀缓存是未决问题，见 `00_DECISIONS.md`。）
3. **重建会话是一次有标价的操作。** 切 model/effort、会话损坏、缓存超 1 小时 TTL 过期，
   都会让下一轮回到冷启动价。Router 应当把它当作显式事件记录下来，而不是悄悄发生。

**1 小时 TTL 的调度含义**：`AgentSession.LastTurnAt` 超过 1 小时的成员，
下一轮必然是冷启动。编排器在排轮次时应当知道这件事 ——
让一个成员在讨论里沉默 90 分钟再发言，比让它每 40 分钟说一句要贵。

## 1. 数据结构

```go
type AgentCursor struct {
    AgentID   string
    Seq       int64      // 已成功投递并被处理的最大 seq
    Pending   []int64    // 已发出、尚未确认的 seq
    UpdatedAt time.Time
}
```

Canonical Conversation 里每条消息有单调递增的 `Seq`（见 `05_MESSAGE_PROTOCOL.md`）。
Router 对每个 agent 维护一个 cursor。选下一个发言者时，取
`messages where seq > cursor.Seq`，再按下面的规则过滤。

## 2. 确定性路由规则（MVP 不用 embedding）

**应当路由的**：

- 与当前任务相关的用户指令
- 直接 @ 了这个 agent 的消息
- 这个 agent 被指派的任务及其上下文
- 发给这个 agent 的任务结果
- 其他 agent 在本任务上已形成的结论（不是全过程）
- Judge / Manager 的指令

**不应当路由的**：

- 原始 PTY 噪声（本架构里根本不存在，因为不刮屏）
- 内部 UI 事件、provider 诊断
- 重复消息
- 与本任务无关的历史讨论
- 完整执行日志（除非明确请求）

## 3. `thought` 字段：一个必须显式表态的决定

**Step 0 发现**：grok 的 JSON 输出带 `thought` 字段（思维链摘要），
agy 的 usage 里有 `thinking_tokens`，claude 的 usage 里有 `thinking_tokens`。

这不是一个可以默认处理的字段：

- **当作特性**：Judge 看到 Critic 的推理过程而不只是结论，合议质量更高
- **当作泄漏**：模型在"思考"时说的话比最终输出更不设防
  （本次探针里 grok 的 `thought` 就直接复述了用户全局规则的内容），
  把它路由给另一个 agent 等于让后者读到前者的私下斟酌

**本设计的立场**：`thought` **默认存入 Canonical Conversation 但默认不路由**。
存是为了可审计、可回放；不路由是因为默认应当保守。
要让某个 **mode**（典型是 `mode == judge`）看到，必须在 Team 配置里显式打开。

**两处订正（2026-09-21）**：

1. 原文写的是「某个 **role**」。判别器已裁定只有 `Mode`，`Role` 是注入身份块的
   自由文本（未决 #7、`02` §2 的订正）。措辞改掉，免得下一个人按 `Role` 去实现。
2. 这个开关**归 Step 9，不归 Step 8 第三片**（未决 #9）：它长在 Team 配置上，
   而 Team 接进会话是 Step 9 的事（`10a` 冻结契约已写明）。
   今天 Teams 只被 `/teams` 列出来看，没接进任何会话。

**「存」这一半今天是活的**（`messages.thought` 列、`AppendMessage` 无条件写），
**「可审计」这一半还是承诺**：`/log` 不打印 thought，导出 / 审计命令还不存在。
真做那个命令时要回头看一眼未决 #9 裁的那个存储开关的默认值。

## 4. 新成员的 bootstrap

新加入的 agent 没有历史会话。**不要把整个对话倒给它**（那是一次昂贵的冷启动，
且大部分内容与它的角色无关）。

Bootstrap 包 = 原始/当前任务 + 共享工作摘要（若有）+ 相关的近期结论 + 被显式引用的消息。

这同样适用于「切 model/effort 导致会话重建」的场景 —— 对 Router 而言两者是同一件事。

## 5. 投递与确认：抄 relay 的教训

many-ai-cli 的作者建过同一个东西，结论写在 `internal/hub/relay.go:6-10`：
整个循环只靠两个信号推进 —— 子进程 progress 文件里 `## DONE <role>` 的**行数增长**，
和 reviewer 的 `verdict:` 行。不是 idle，不是 transcript，不是 VT。

**确认必须在协议层，不能在传输层。**

在 Council 的架构里这件事反而简单了：headless 调用是同步的，
进程退出 + `stopReason: end_turn` / `status: SUCCESS` 就是确认本身。
不需要 relay 那套文件标记，因为我们不隔着 PTY。

**但 relay 的四个模式仍然值得抄**：

| 模式 | 出处 | 抄什么 |
|---|---|---|
| 注入前快照计数 | `relay.go:884-900`、`relay.go:387-409` | 发指令前先记下基线，只有基线之后产生的完成信号才算数。Council 里对应"cursor 只在本轮成功返回后推进" |
| 入队到 idle | `orchestration_notify.go:60-170` | 有序 FIFO + 溢出记账 + 严格门控。Council 里对应"目标 agent 正在跑时，新消息入队而不是并发调用同一会话" |
| 硬封顶 | relay 的 `maxRounds`（默认 3）、`escalateAfter` | 自动 agent-to-agent 循环必须有轮数上限、超时、取消。grok 还有原生 `--max-turns` 可叠加 |
| registry 形状 | `internal/subscription/adapter.go:85-108` | interface + `init()` 里 `Register` + `AdapterFor`，是个现成可抄的形状，不必自己发明 |

## 6. 游标推进规则

- 一轮调用**成功返回**（进程 exit 0 且 JSON 可解析且 `status` 表示成功）→ 推进 cursor
- 调用失败、超时、JSON 不可解析 → **不推进**，该批消息留在 `Pending`，下次重发
- 重发必须是同一批内容、同样的顺序（否则打断前缀，见 §0 规则 1）
- 进程崩溃后重启：从持久化的 cursor 恢复，`Pending` 重发

## 7. 可观测性

Router 必须能回答「上一轮到底送了什么给谁」。
调试输出里包含每个 agent 每轮的：seq 区间、字节数、是否触发重建、本轮 usage。

**不得在调试输出里包含任何凭据。**
注意 Step 0 的发现：agent 会读到用户全局 `CLAUDE.md` 的内容，
而那份文件里可能有路径、IP、内部约定 —— 这也是 D-5 要求「成员跑在干净指令环境」的原因之一。
