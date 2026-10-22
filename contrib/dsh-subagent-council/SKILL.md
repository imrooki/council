---
name: council
description: 'Use when a decision benefits from several independent models: multi-option tradeoffs, cross-model disagreement, high-risk changes (schema, permissions, concurrency, migrations), or when one agent has been stuck for two rounds. Not for a single factual lookup. Two tools exist: council (read-only) and council_work (edits an isolated git worktree copy and returns a diff).'
---

# Council：把问题交给多个模型

`council` 和 `council_work` 都会启动一个 Council 进程，让配置里的多个成员各自回答，可选地让一名汇总成员收口。它们**不继承当前对话**：任务必须自足。

## 1. 何时叫

叫：

- 需要在几个方案之间权衡（架构、取舍、迁移路径）。
- 预期不同模型会给出真实分歧，而分歧本身有信息量。
- 高风险改动：schema、权限、并发、数据迁移、对外契约。
- 单个 agent 已经卡住两轮，换一组视角比继续原地打转便宜。

不叫：

- 单一事实查询、读一个文件就能回答的问题——一个成员就能答，不值得起三个进程。
- 只需要执行、不需要判断的机械改动。
- 任务依赖当前对话里还没写下来的上下文；先把上下文写进任务文本，否则不要委派。

## 2. 怎么叫

选择工具：

- `council`：只读。成员在 Council 的 READ 档工作目录里跑，那**不是当前仓库**；不同 provider 的沙箱不同（claude 限制在 cwd，grok 的 `--cwd` 不是边界），**不要假设它们能读仓库**。问题所需的代码/数据要自己写进任务（贴相关片段，不要整份文件）。适合自足的问答题、方案比较。
- `council_work`：成员在 cwd 的 git worktree 隔离副本里跑，**能读真代码、也能改**，跑完返回 diff；**不会自动 apply，也不会自动删副本**。cwd 必须是 git 仓库根；当前目录不是仓库根时，用工具的 `cwd` 参数指到仓库根。`council_work` 用于「要改文件」或「答案必须读仓库」，纯问答用 `council`。

任务怎么写：

- 写清三件事：**问题**、**约束**（不能改什么、必须兼容什么）、**期望产出的形状**（结论 / 对比表 / 补丁思路 / diff）。
- `council`：把相关代码、报错、数据贴成紧凑片段；不要只给一个仓库路径，成员不保证读得到。
- `council_work`：给路径和符号名即可，不要把整份文件粘进任务；成员能在副本里自己读。
- 一次只问一个可收口的问题。不要把五个不相干的问题塞进一次委派。
- 追问时，把上一次输出末尾 JSON 附录里的 `conversation_id` 放到任务最前面：`[conv:conv_xxx] 接着问……`。前缀只认行首，大小写敏感；正文本身必须以 `[conv:` 开头时，在前面加一个空格或改写措辞。`council_work` 的追问会复用同一个 convID，而 worktree 副本不会自动删；副本已存在时 Council 会拒绝——**不要自行删除副本**；把副本路径交给用户，或换一个新问题。

预算与节奏：

- 每个委派 **≤ 5 USD**。默认预算就是 5；不要为了“多跑几轮”把它调高。
- **同一个问题在同一个会话里不重复叫议会**。第一次结果不理想时，先读 `turns[]` 找分歧，再决定是追问还是自己做。
- `council_work` 用于「要改文件」或「答案必须读仓库」，纯问答用 `council`。

## 3. 怎么读

按顺序：

1. 先看 `## summary`。汇总成员的意见是默认结论。
2. 有分歧时看各成员小节：`### <agent_id> / <model> / $<cost>`。每一轮只保留前 20 行；需要全文时看 `[conv:...]` 续接或让成员重述关键段。
3. 有改动时看 `### workspace diff (copy: ...)`。这是隔离副本里的 diff，不是已经应用到工作区的改动；要不要 apply 由你决定。diff 超过 8 KiB 会被截断，完整副本路径在标题里。
4. 看最后的 JSON 附录：`{"conversation_id":"...","cost_usd":...,"status":"...","workspace":"..."}`。追问时从这里取 id。

`status` 不是 `ok` 时：

- 首行 `[partial f/t]` 表示 t 个成员里有 f 个失败（与 Council 自己的 stderr 口径一致）。已有正文仍可参考，但不要把它当成全员共识。
- `[budget_exceeded]` / `[error]`：这次没有可靠结论，不要编造；向用户说明失败原因，或缩小问题后重试。
- 成本可能不完整：有的 provider 不报美元金额，`cost_usd` 只统计报了的那些轮。不要把 0 当成免费。

## 硬性规则

- 每个委派 **≤ 5 USD**。
- **同一问题在同一会话不重复叫议会**。
- `council_work` 用于「要改文件」或「答案必须读仓库」，纯问答用 `council`。
