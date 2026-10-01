# Provider 与 Agent 架构

核心命题：**Provider 不是 Agent。**
Provider 是一个执行后端（某个 CLI 的 headless 接口）；
Agent 是运行时从配置造出来的实例 = Provider + Model + Effort + Role + Permissions + RuntimeMode。
同一个 Provider 可以造出任意多个互不相干的 Agent。

## 1. Provider 就是一层 flag 适配器

选定 D-1 之后，Provider 的实现量小得多：它不包装 TUI，只负责
「把一次 Council 调用翻译成这个 CLI 的 headless 命令行，再把它的 JSON 输出翻译回 Council 的消息」。

### 1.1 四家的契约（本机实测，2026-09-20）

**版本会漂。** 这张表初版记的 agy 是 1.1.26，同日再查已经是 1.2.7 ——
Step 0 留下的两份 agy 探针 JSON 是旧版产出的。
2026-09-20 的 Step 6.0 探针实测 1.2.7 的输出形状与 1.1.26 逐项一致，所以适配器不必改；
但这件事本身说明「CLI 版本在两次会话之间变掉」不是假想风险。
**读这张表之前先自查版本**，对不上就以实测为准，并回来更正这里。

| 能力 | claude 2.1.278 | codex-cli 0.147.0 | grok 1.0.13（经 CPA） | agy 1.2.7 |
|---|---|---|---|---|
| 单轮 | `-p/--print` | `exec` | `-p/--single` | `-p/--print` |
| 恢复 | `--resume <id>` | `exec resume <id>` / `--last` | `--resume <id>` | `--conversation <id>` |
| 预指定 id | 否 | 否 | **`--session-id <uuid>`** | 否 |
| JSON 输出 | `--output-format json` | `--json` | `--output-format json` | `--output-format json` |
| 流式 NDJSON | `stream-json` | `--json` | `streaming-json` / `streaming-messages-json` | `stream-json` |
| 输出 schema | — | `--output-schema <FILE>` | `--json-schema` | `--json-schema` |
| 身份注入 | `--append-system-prompt[-file]` | 走 AGENTS.md | `--system-prompt-override` / `--rules` | `--agent` |
| effort | `--effort <level>` | `-c model_reasoning_effort=` | **经 CPA 被静默丢弃** | `--effort low/med/high` |
| 权限档 | 有 | `-s read-only/workspace-write/danger-full-access` | `--permission-mode`（6 档） | `--mode` / `--sandbox` |
| 工作目录 | — | `-C/--cd` | `--cwd` | `--add-dir` |
| 轮数封顶 | — | — | `--max-turns` | — |
| 能力发现 | — | — | `grok models` | `agy models` / `agy agents` |
| 跨轮缓存 | **有**（D-4 Step 0 探针：T1 `cache_create=47749 cache_read=0`，T3 `cache_create=95 cache_read=47752`） | 未实测（无适配器） | **有**（Step 0 同一条会话 resume `cache_read` 0→20608；未决 #1 跨会话印证；Step 8 冒烟 `cache_read=128`） | **无**（未决 #17：同一条会话第二轮 `input_tokens` 16195→32722 而 `cache_read` 仍是 0；Step 8 冒烟同样 `cache_read=0`） |

**grok 的 effort 是个坑**：命令行 `--effort` / `--reasoning-effort` 经本机的 CLIProxyAPI 中转会被
能力门静默丢弃，退出码仍是 0。真正生效的是 `~/.grok/config.toml` 里
`[model."grok-4.6"]` 段的 `reasoning_effort`（当前 high）。
所以 grok 的 EffortPolicy 必须声明为「进程外配置，不可按次切换」。

### 1.2 Provider 接口（设计，非代码）

```go
type Provider interface {
    ID() string
    Capabilities() Capabilities

    // BuildTurn 把一次调用翻译成具体的命令行。
    // session 为空表示开新会话；非空表示恢复。
    BuildTurn(req TurnRequest) (Command, error)

    // ParseTurn 把该 CLI 的 JSON 输出翻译成 Council 的消息 + 用量。
    ParseTurn(stdout []byte) (TurnResult, error)
}

type Capabilities struct {
    SessionID     SessionIDPolicy   // Preassignable | ReturnedOnFirstTurn
    Effort        EffortPolicy      // PerTurnFlag | ProcessExternalConfig | BakedIntoModelLabel | Unsupported
    Identity      IdentityPolicy    // SystemPromptFlag | RulesFile | AgentPreset | InstructionFileOnly
    Permissions   []PermissionTier  // 该 CLI 原生支持的档位
    OutputSchema  bool
    MaxTurnsFlag  bool
    ModelDiscovery ModelDiscovery   // Subcommand | ConfiguredList | Passthrough | Unknown
}
```

**关键约束**：`Capabilities` 里每一项都要能表达 **Unsupported / Unknown**，
绝不用一个默认值假装支持。agy 的 effort 同时以 model label 后缀
（`Gemini 3.8 Flash (High)`）和独立 `--effort` flag 两种形式存在，二者如何交互未实测 ——
这正是必须有 `Unknown` 这个状态的理由。

### 1.3 能力发现的三级回退

1. **子命令**（grok / agy 有 `models`）—— 运行时查，最准
2. **配置清单** —— 用户在 Council 配置里列出可用 model
3. **透传** —— 不认识就原样传给 CLI，让它自己报错

不伪造第四种「猜一个」。

## 2. Agent

```go
type AgentDefinition struct {
    ID          string   // 稳定标识，@mention 用这个
    DisplayName string
    Provider    string
    Model       string   // 由 Provider 解释，Council 核心不认识具体型号
    Effort      string   // 同上；Provider 声明不支持时拒绝该配置并说明原因
    Role        string   // 自由文本，注入 identity
    Permissions PermissionTier
    Mode        RuntimeMode
}
```

`Model` 和 `Effort` 是**不透明字符串**，Council 核心不做枚举校验 ——
校验交给对应 Provider 的 `Capabilities`。这条直接来自原计划的规则 9 和 10，是对的。

`RuntimeMode`（discussion / worker / discussion_and_worker / judge / manager）
是**行为配置，不是类型层级**。Judge 是 `Mode == judge` 的普通 Agent，
provider / model / effort 全部由用户选。

**订正（2026-09-21，未决 #7 裁决）。** 这里原本还有一句
「Judge 就是 `Role == "judge"` 的普通 Agent」，与上面那行 `Mode RuntimeMode`
并存了很久，于是同一份文档里有两个判别器。**已删掉那半句，判别器只有 `Mode`。**

**为什么留 `Mode` 删 `Role`，不是反过来**：

- `Role` 是**自由文本，注入身份块**（本节字段表自己就是这么写的）。
  它进 `renderIdentityCore`、进缓存前缀、进 `IdentityFingerprint` ——
  拿它当判别器，等于让「把某人改成裁判」变成一次 `ErrIdentityDrift`、
  一次会话重建、一次入场费。
- 实测：`AgentConfig.Role` 在 `internal/turn` **之外零处被读**
  （只有字段声明与两处配置赋值，值是整句中文，不是枚举串）；
  turn 包内唯一读点是把它原样拼进身份文本。**今天没有任何一处按 `Role` 分支。**
- `Mode` 反过来：全仓非测试读点三处（枚举校验、缺省与小写规范化、`/agents` 打印），
  **三处都不是按 `Mode` 分支的行为**，在 `internal/turn` / `internal/provider` /
  `internal/app` / `cmd` 四个目录零命中。留它零代价。

`manager` 与 judge 同型，一并按 `Mode` 处置。
`config.Mode` 的五值枚举**一个字不动**（`07a` 冻结过，且有四处测试钉子），
`validate.go` 也不加新校验 —— 本次订正是**零行为改动**。

## 3. AgentSession：不是进程，是游标

**这是 Step 0 带来的形状变化。**

```go
type AgentSession struct {
    AgentID        string
    ProviderSession string    // CLI 那边的 session_id / conversation_id
    Cursor         int64      // Canonical Conversation 里已投递到的 seq
    LastTurnAt     time.Time  // 用于判断 1h 缓存是否已过期
    Cold           bool       // 尚未建立 provider session
}
```

一轮调用 = 「按 `ProviderSession` 恢复 → 送入这一轮的 delta → 读 JSON → 推进 Cursor → 进程退出」。
没有需要维护的长活进程，也就没有僵尸进程、没有 PTY 生命周期、没有重连。

**`LastTurnAt` 是一等字段**，因为缓存 TTL 是 1 小时（D-4）。
超过之后这个 agent 的下一轮会付冷启动价，调度器应当知道这件事，
必要时在 UI 上提示，或在编排时优先唤醒快要过期的成员。

## 4. 切换 model / effort

四家都**不支持在一个已有会话里改 model 或 effort**。
做法只有一个：**建新会话，用 Context Router 做 bootstrap**（见 `04`）。

不要假装可以热切。要明确告诉用户这会发生什么：
该 agent 回到冷启动，约 $0.4–1.0（claude 侧实测量级），上下文靠 bootstrap 重建而非继承。

## 5. 关于 many-ai-cli 的 custom provider

**不走那条路。** 它的 `CustomProvider` 只有 4 个字段（ID / Label / Command / ApprovalPatternSource），
没有 model、没有 effort、没有 capability；`isAIProvider` 对 custom 返回 false，
意味着拿不到 chat 提取、审批规则、handoff、usage 追踪；
而 `validOrchestrationProvider`（orchestration.go:3668）根本拒绝 custom provider 被编排 API spawn。

Council 自己的 Provider 抽象从第一天就比它完整，没有理由去迁就它。
