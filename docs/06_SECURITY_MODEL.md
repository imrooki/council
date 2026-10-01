# 安全模型

短文档。Council 的安全面比 many-ai-cli 小得多，因为它不开 HTTP 服务、不暴露端口、
不做浏览器 UI，也不代管任何凭据。

## 1. 不做的事（继承自 D-2 的教训）

many-ai-cli 的审计结论里，风险几乎全部来自"它是个本地控制面"这一事实：
Hub token 等价于以当前用户身份任意执行代码、token 被放进 URL、
每个 CLI 子进程的环境里都有那个 token、`POST /api/spawn` 接受 `provider=shell`。

Council 没有这些，因为它没有 Hub。**不要在 MVP 里加回来。**
真要做 UI，UI 连的是 Council 的本地 SQLite 和进程，不是反过来让 UI 成为控制面。

同样明确：
- 不读取、不复制、不存储任何 provider 的认证凭据（各 CLI 自己管登录）
- 不把任何 token 放进命令行参数或 URL
- 不向 agent 的环境注入 Council 自身的控制凭据

## 2. 权限档到各 CLI 的翻译（不是直接映射）

**本节 2026-09-21 重写。** 原标题是「权限档直接映射到各 CLI 的原生 flag」，
那条前提被实测推翻，整张表跟着错了几格。留下这段是因为错的方式值得记住。

推翻它的事实：**有效权限 = 命令行 flag ∪ 用户的 `~/.claude/settings.json`**。
本机那份文件写着 `permissions.defaultMode = "bypassPermissions"`，`allow` 里有
`Write(*)` `Edit(*)` `Bash(*)`；claude 与 grok 都在读它（`grok inspect` 子命令直接列出来）。
于是原表 READ 那行「默认 + 不给写工具」在纸面上没错，跑出来**建得出文件** ——
READ 档实测挡不住写盘，修在 `6530839`。

教训一般化：**一格 flag 单独看没有意义**，要配上「这家能不能证明自己围得住」。
所以下表多两行，记的是实测行为而不是 flag 的字面承诺。

| Council 档 | claude | grok | agy |
|---|---|---|---|
| READ（只读讨论） | `--safe-mode --restricted --permission-mode default` | `--permission-mode default` + `--deny Write(*)` `--deny Edit(*)` `--deny Bash(*)` | **不发 `--mode`** |
| WORKSPACE（可改工作区） | `--safe-mode --restricted --permission-mode acceptEdits` | **拒绝，无边界可证** | `--mode accept-edits` + `--add-dir <workdir>` |
| FULL（危险） | **不提供**，见下 | **拒绝**，不给编排器 | `--dangerously-skip-permissions`（写在适配器里，上层未放行） |
| **写入边界实测** | **Yes**：`--restricted` 把文件工具围在工作目录内，目录内写得进、兄弟目录被拒 | **No**：`--deny` 是工具级的，路径模式静默不生效 | **Yes**：`--add-dir` 之外被拒，理由记在 `denied_actions` |
| **指令环境隔离实测** | **Yes**：`--safe-mode` | **No**：三招都试过，见 §4 | 见 §4 |

逐格的理由写在各适配器的 `*PermissionArgs` 注释里，原件在
`E:\Isolating_test\council\step8-perm-probe\STEP8-PERM-RESULTS.md`。三格需要单独说明：

- **agy 的 READ 不发 `--mode`**，不是「发一个只读值」。本家只有 `accept-edits`
  与 `plan` 两个值，**没有只读档**，而 `--mode plan` 下它 `status=SUCCESS` 却只回空串
  （未决 #17）。不发 flag 时落到默认档，默认档要求逐次授权、headless 下没人可问，
  于是写被拒。这条**验过一次**（建 PROOF.txt 失败，三处目录均无文件），
  但一次否定实验不等于沙箱，按「已验证一次、未穷尽」记。
- **claude 没有 FULL**，而且是结构性的：`--restricted` 明写 refuses bypassPermissions，
  READ 与 WORKSPACE 都建在它上面，这条路到不了 FULL。这不是偷懒，是好事。
- **grok 的 WORKSPACE 与 FULL 都拒绝**。它守不住边界这件事是实测的，
  而 `--tools` / `--deny` / `--sandbox` 对非法取值**静默接受、退出码 0**，
  所以在 grok 上「配了规则」永远不能当成「规则生效了」。

codex 尚无适配器，整列删掉，免得一张表同时描述已实现与没实现的东西。
它的档位映射在 `02` 的能力契约表里，等真写适配器时按同样的方式实测。

**默认 READ。** 一个 agent 要拿到 WORKSPACE，必须显式写出来，
**且它的 provider 必须能证明自己围得住**——证明不了的直接拒绝，不降档、不警告放行。

**FULL 档在 MVP 里不提供给编排器自动使用。** 这条是针对 many-ai-cli relay 的教训：
它的子会话默认全权限绕过（codex 用 `--sandbox danger-full-access --ask-for-approval never`），
那是为无人值守设计的，对 Council 不适用。

**2026-09-21 补：这条从姿态升级成了实测结论。** 原件
`E:\Isolating_test\council\oq-agy-denied-count\RESULTS.md`：给 agy 传
`--dangerously-skip-permissions`（就是本表里 FULL 档的映射，`agy.go:173`）
并同时传 `--add-dir <某目录>`，指令它往那个目录的**父目录**写两个文件 ——
**两个文件都真的落盘了**，内容就是探针写的 `LEAK1` / `LEAK2`，
而 `denied_actions` 这个键**整个缺席**（不是空数组，是没有）。
判据是文件系统，不是模型自述。

所以「FULL 档拒绝」现在有行为证据：**那个档映射到的 flag，实测会把文件
写到指定目录之外**。`turn.TranslatePermission` 硬拒 `full` 不是保守，是必需。

同一次探针还订正了本节与 `11c` 第 2 节共有的一处**归因错误**：
`--add-dir` 自己**不是**边界，它只划出「哪里算界内」；起作用的是权限模式。
这与本节第 2 节那张表被推翻过的那一次是同一类错误 ——
把边界归因给一个看起来像边界的 flag。

## 3. 审批缺口有多大：一个角色

这是 D-1 代价的量化。以 FSDT 讨论场景的五个角色为例：

| 角色 | 动作 | 需要人答审批 |
|---|---|---|
| Theory | 只读讨论 | 否 |
| Critic | 只读讨论 | 否 |
| Numerical Verifier | 跑数值，在隔离根 | 否（WORKSPACE 沙箱足够） |
| **Executor** | **改代码** | **是** |
| Final Reviewer | 只读综合 | 否 |

**只有 Executor。** 对这一个角色，MVP 的做法是：
跑在 WORKSPACE 档 + 一份 Council 自建的自包含 git 副本
（**不是** worktree —— grok 的 `--worktree` 在 headless `-p` 下明写不建 worktree，
原文见 `11d` 第 1.1 节；而 worktree 目录里的 `.git` 是个指向主仓库的**文件**，
agent 一 commit 就要写到 cwd 边界之外），
产出 diff，由人在合并前审阅。Council 不需要一个通用的实时审批 UI ——

**2026-09-21 实测补一条，它改变这个角色的能力边界**
（原件 `E:\Isolating_test\council\oq-task-git\RESULTS.md`）：

> **WORKSPACE 档的 agent 只能编辑，不能执行。**

A/B 探针，一次只改一个变量：加 `--restricted` 时 claude 的工具列表里
**没有 Bash、也没有 PowerShell**；不加时两个都在。
而 `--restricted` **正是**写入边界的来源（本文第 2 节那张表：
不加它，用户 `settings.json` 的 `bypassPermissions` 会生效，READ 档都写得出文件）。

**这两件事由同一个 flag 决定，二选一，Council 给不了「能写盘又能执行」。**

所以 Executor **跑不了测试、跑不了编译、跑不了 linter**。
「改代码」这个说法要按字面理解：它只改，不验证。
一份任务产出的 diff，在人合并之前**没有任何一层跑过它** ——
这不是流程上的省略，是能力上的边界，`/task` 的帮助文本要直说。

（只覆盖 claude。agy 在 WORKSPACE 下能不能执行命令**未测**，
它走 `--mode accept-edits --add-dir`，与 `--restricted` 不是一回事；
grok 不允许 WORKSPACE，不涉及。别默认三家一样。）
那是给"每个会话都交互且不可预测"的场景设计的，而 Council 的运行时**事先就知道谁要写盘**。

## 4. agent 的指令环境必须干净

**Step 0 实测发现**：grok 的 `thought` 字段里直接出现了用户全局 `~/.claude/CLAUDE.md` 的规则内容。
也就是说每个 agent 都在读那份为主代理写的文件 —— 里面有设备表、内网 IP、路径约定、工作流规则。

两个问题：一是成本（占固定开销的可观比例），二是**信息面**：
一个只该讨论力学的 agent 没有理由知道你的内网拓扑。

**要求**：Council 成员的指令环境由 Council 自己构造，
用各 provider 的 identity 机制（`--append-system-prompt` / `--system-prompt-override` /
`--rules` / `--agent`），而不是继承用户的全局规则文件。
具体隔离手段（改 HOME、改 provider 的配置目录环境变量、或用 provider 自带的 override flag）
按 provider 能力选，在 `Capabilities.Identity` 里声明。

### 4.1 这条要求有两半，2026-09-21 各走了一步

**后半截「不继承用户的」已做**（`6530839`）。实测结果按 provider 分家，**三家不对等**：

| | 切断继承 | 手段 | 后果 |
|---|---|---|---|
| claude | **能** | `--safe-mode` 关掉 CLAUDE.md 自动发现、skills、plugins、hooks、MCP，而**保留 auth、model、built-in tools 与 permissions** | Council 注入的身份块是**唯一**指令源 |
| grok | **不能** | `CLAUDE_CONFIG_DIR` 重定向无效（`grok inspect` 显示它照读原路径）；`--system-prompt-override` 也压不住（用户那条首行规则仍被照办） | Council 的规则是**叠加**在用户全局规则之上 |
| agy | **大概率**（未证明） | 未找到相关 flag，也不需要：它似乎本来就不读 `~/.claude` | 表现与 claude 相同，但证据是否定式的 |

**agy 这一格的证据强度，措辞要准**（2026-09-21 两轮）：

1. 探针轮：默认参数问一个无关问题，回答**没有**以用户那条首行规则开头；
   直接问它「系统提示或项目指令里有没有一条要求固定首句」，它明确否认。
   `agy --help` 里没有 `--safe-mode` 之类的开关，也没有 `grok inspect` 那样
   能列出已发现配置的子命令，所以这条路上没有第三次交叉验证可做。
2. 真机轮：一条真实的 Council 轮次里，它**照办了**经 Council 注入的两条规则，
   **同时没有照办**用户那条。这是个对照：它对同类指令是听的，独独那一条没出现。

**这仍然不是「它没读到」的证明**——模型完全可以读到而不遵从。
所以 `CleanInstructionEnv` 继续记 `SupportUnknown`，不要因为这两轮就翻成 Yes。
**真正没测的是另一件事**：agy 有自己的 `--project` / `--agent` /
`--disable-slash-commands`（原文提到 skill expansion）体系，
它会不会从那里带进指令，完全未测。别把这两件混为一谈。

`--safe-mode` 顺带解开了一个长期死结：原以为隔离规则文件必须改 `HOME`，
而改 `HOME` 会把登录态一起带走。`--safe-mode` 明写保留 auth，绕开了它。

**成本与安全在这里同向**，这是意外的强结果：同一个被拒的写请求，
`--disallowed-tools` $0.7730、`--restricted` $0.0485、`--safe-mode --restricted` $0.0140；
claude 的真机 `input_tokens` 从 58023 掉到 29413。切断继承不是为安全付钱，是**省钱**。

**前半截「Council 有自己的那份」是 Step 8 的事**，规格见 `11_STEP8_SPEC.md` 第 1 节。
那一节必须原样带上这张表：三家不对等不能在规格里假装一样，
用户在 claude 上写的规则是全部，在 grok 上写的规则只是增量。

## 5. 自动 agent-to-agent 循环的封顶

每一条自动调用链必须有：最大轮数、超时、取消、循环保护。
grok 有原生 `--max-turns` 可以叠加一层。

这不只是安全问题也是成本问题：一个失控的循环在 claude 侧每轮最高可达冷启动价。

## 6. ToS

Council 把多个订阅制 CLI 编成自动化系统，这件事本身带 ToS 风险，
与运行在哪一层无关（headless + resume 在厂商看来和交互式没有区别）。

已知先例：2026-02 Google 因第三方工具借 Gemini CLI 的 OAuth 访问后端而封号，
连带波及 Antigravity（Google 自家产品）。many-ai-cli 因此明确把 Gemini CLI 排除在外。
本机的 agy 走 `~/.gemini` 目录，落在这个风险区内。

**本项目的立场**：只用各 CLI 官方提供的 headless 接口，不借用、不代理、不中转任何厂商的 OAuth，
不为绕过速率限制而轮换账号。`--max-turns` 这类封顶让无人值守的用量可界定，
这是相对交互式使用唯一的、正向的差别。
