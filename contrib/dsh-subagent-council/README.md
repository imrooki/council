# @council/dsh-subagent-council

把 Council 接成 dsh 的 subagent provider：每次委派启动一个 `council ask -json` 进程，把结果映射成 dsh 的外部 subagent 结果；再配一个 skill，教模型什么时候叫、怎么叫、怎么读。

**状态：代码与单测完成，未在 dsh 里跑过。** 契约 §5 的 B1–B4 需要真实 dsh 运行时（装插件、委派、取消、`[conv:]` 追问），本片没有执行，也没有伪造结论。部署前请按文末「真机验收」逐条跑。

## 目录

```
package.json            独立 npm 包；dsh.bundle.patch 指向 cordis.patch.yml
cordis.patch.yml        两个 provider 实例 + 两个委派工具实例
lib/index.js            Cordis 插件：Config、apply、provider.start（只做装配）
lib/run.js              spawn/超时/取消/结果解析；可注入 spawn 与 seam，供测试
lib/command.js          纯函数：argv 组装、[conv:] 解析、输出格式化、UTF-8 截断
lib/command.test.js     command.js 的 node --test
lib/index.test.js       run.js 的 node --test（目录入口聚合两个测试文件）
lib/package.json        仅用于让 Node 24+ 的 `node --test lib/` 找到测试文件（见「差异」）
SKILL.md                dsh skill（frontmatter + 三块正文）
README.md               本文件
```

运行时代码只依赖 `node:child_process`、`node:crypto`、`@deepseek-ai/schemastery` 与 `@deepseek-ai/dsh-subagent`。`run.js` 不 import dsh 包，只有 `index.js` 做 Cordis 装配；没有 Go 代码，不进 Go module 的编译范围。

## 组合

`cordis.patch.yml` 挂四个实例：

| id | 作用 |
|---|---|
| `subagent-council` | provider `council`，`workspace: false`，只读 |
| `subagent-council-work` | provider `council-work`，`workspace: true`，隔离副本 + diff |
| `tool-council` | 面向模型的 `council` 工具，指向 provider `council` |
| `tool-council-work` | 面向模型的 `council_work` 工具，指向 provider `council-work` |

把本目录变成 dsh 能解析到的 npm 包（本地 link 或发布），再把 patch 合并进 profile 的 Cordis patch 即可。**安装步骤本身没有在 dsh 里验证过。**

`tool-council` / `tool-council-work` 都写了 `maxDepth: provider-managed`。这不是装饰：`@deepseek-ai/dsh-tool-subagent` 默认用宿主数值 `maxDepth`，而外部 provider 没有 `depthLimit` 能力，缺这一项工具会在挂载时直接报错（见「差异」第 2 条）。

还有一条容易误解的运行时事实：`council` 的成员在 Council 的 READ 档工作目录（`<data_dir>\workdir\<agentID>`）里跑，那**不是当前仓库**；不同 provider 的沙箱也不一样（claude 的 `--restricted` 明确限制在 cwd，grok 的 `--cwd` 不是边界），所以不要假设成员能读到仓库。需要仓库内容时用 `council_work`，或把必要的片段写进任务。

## 配置

| 字段 | 默认 | 语义 |
|---|---|---|
| `providerName` | `council` | 注册到 `ctx.subagents` 的名字 |
| `councilPath` | `council` | `argv[0]`；Windows 建议写绝对路径或 `council.exe`。必须是真的可执行文件，不能是 `.cmd` / `.bat`（插件不经过 shell） |
| `members` | `[]` | 空表示不传 `-members`，由 Council 用配置里的全部 agent（按 id 字典序）；插件不暴露 `-team` |
| `summary` | `''` | 空串表示不传 `-summary`（不覆盖 Council 配置的 `[summary].agent`）；其余值原样传给 Council；`none` 仍接受，等价于空串的「不覆盖」 |
| `rounds` | `1` | `> 1` 时同时传 `-mode round`（否则 `ask` 会因 rounds≠1 退 2） |
| `budgetUsd` | `5` | 传 `-budget-usd`；schema 下限 0.5 |
| `workspace` | `false` | `true` 且有 cwd 时传 `-workspace <cwd>` |
| `timeoutMs` | `570000` | 插件侧超时（9.5 分钟），给 Council 自己的 `-timeout 10m` 留余量 |

注意 `summary` 默认是**空串**，语义是「不覆盖」而不是「命令行显式关闭汇总」。空串和 `none` 都不传 `-summary`，所以 Council 配置里存在的 `[summary].agent` 仍然会生效。要强制关闭汇总，需要改 Council 配置；插件层没有第二条路。旧配置里的 `none` 继续可用，但不再作为默认值。

`timeoutMs` 只控制插件侧的杀树；Council 自己的 `-timeout` 仍是默认 10m。把 `timeoutMs` 设得比 10m 大时，Council 会先按自己的超时退出，插件收到的是「非 0 退出 / JSON 里 status=error」，而不是超时诊断。

## 命令组装

```
<councilPath> ask -json -p <task> -budget-usd <n> -rounds <n>
  [-mode round]            rounds > 1
  [-members a,b]           members 非空
  [-summary <id>]          summary 不是空串 / none
  [-workspace <cwd>]       workspace && cwd 有值
  [-conversation <id>]     行首 [conv:conv_*] 解析成功
```

关键决策：

- **argv 数组直通 `spawn`，从不拼 shell 字符串。** 任务里的引号、换行、`--flag` 都留在同一个 argv 元素里；Go 的 `flag` 会把 `-p` 的下一个参数原样当值，哪怕它以 `-` 开头。单测覆盖了这一条。
- **cwd 的来源**：优先 `request.cwd`（dsh 的服务层已经把它解析成绝对路径），其次 `request.parent.session.header.cwd`。两者都没有就不传 `-workspace`，并在输出/诊断里写一条 `note`，避免 `council_work` 静默退化成只读。
- **`-workspace` 的目录就是传入的 cwd**，不额外 `git rev-parse`。Council 自己会校验仓库根，非根退 2，诊断会带回来。
- **超时杀进程树**：到 `timeoutMs` 后 Windows 用 `spawn('taskkill', ['/T','/F','/PID', pid])`，POSIX 用 `process.kill(-pid)`（子进程 `detached` 成进程组组长）。杀完等 `close`；10 秒还不退就强制结算并写诊断。结果 `stopReason: 'error'`。
- **取消**：`request.signal` 的 `abort` 和句柄 `dispose()` 都走同一套杀树逻辑；结果由 `settleRunResult` 收敛成 `stopReason: 'aborted'`，不会 reject。
- **JSON 解析失败**：`exit 0` 但 stdout 不是单个 JSON 对象 → `stopReason: 'error'`，diagnostic 带解析错误和 stdout（整体截 4 KiB）。`exit != 0` 但 stdout 恰好是完整 JSON（预算触线/部分结果）→ 仍然渲染 `output`，同时 `stopReason: 'error'`，diagnostic 带退出码和 stderr。不做「从日志里抠 JSON」的容错。
- **stdout 前置 BOM** 会被去掉；不做别的清洗。
- **不走 `ctx.subprocess`**：Council 需要调用方的完整环境（provider CLI 可能从环境读凭据）和本地 cwd/worktree；dsh 的 subprocess seam 会清洗形似凭据的环境变量，也可能由非本机的 provider 提供。契约要求 `env: process.env` 和本地进程树，故直接用 `node:child_process`，这是有意的选择，不是「读不到 API」的退路。

## 结果映射（契约 §4.3）

`output` 是一组 text block：

1. `status != ok` 时第一行 `[partial f/t]` / `[budget_exceeded]` / `[error]`。`f/t` 直接取 Council JSON 的 `failed` / `total`（按席位算，含真跑过的汇总成员）；老版 council 没有这两个字段时，退回按 turns 估算失败成员数/成员总数。
2. `## summary` + 汇总正文。
3. 每个成员一段：`### <agent_id> / <model> / $<四位小数>`，后面是该成员每一轮的前 20 行；多轮时加 `round N:`；失败轮加 `outcome: <...>`。
4. `council-work` 才有的 `### workspace diff (copy: ...)` + `files:` + diff。copy 路径和文件列表一起受 8 KiB 预算；文件太多时只列能放下的部分并写「还有 M 个」。diff 截到 8 KiB；Council 因超过 1 MiB 而返回 `diff_truncated` 时，改为显示 `diff_path`。
5. 最后一行 JSON 附录：

```json
{"conversation_id":"conv_...","cost_usd":0.0,"status":"ok","workspace":"C:\\...\\workspaces\\conv_..."}
```

`workspace` 是副本路径（没有就是空串）。追问时模型按 D9 把 `[conv:conv_*]` 放在任务最前面。

为了不撞 dsh 的 8 KiB 单字符串上限，summary 段、成员段、workspace 头部、diff 都按 UTF-8 字节截断并带可见标记；JSON 附录总是最后一块。

`cost_complete` 不在附录里（契约只给了四个键）。它只存在于 `council ask -json` 的原始输出；需要完整账时看 Council 侧结果。

## SKILL.md

`SKILL.md` 按 dsh skill 的 frontmatter 格式写，正文三块：

- **何时叫**：多方案权衡、跨模型分歧、高风险改动、单 agent 卡住两轮；单一事实查询不叫。
- **怎么叫**：`council` 只读且不保证能看到仓库 / `council_work` 走副本、能读能改；任务写清问题、约束、期望产出；`[conv:conv_*]` 追问。
- **怎么读**：先 summary，分歧看 turns，改动看 workspace.diff，status 非 ok 不当定论。

三条硬性规则在正文和末尾各写了一遍：每个委派 ≤ 5 USD、同一问题同一会话不重复叫、`council_work` 用于「要改文件」或「答案必须读仓库」，纯问答用 `council`。

## 测试

```powershell
node --test contrib/dsh-subagent-council/lib/
```

当前 42 个用例全绿：`command.test.js` 28 个，`index.test.js` 14 个。覆盖 `[conv:]` 行首/大小写/`conv_` 前缀/只剥第一个、workspace 开关与缺 cwd、members/summary 开关、rounds 触发 `-mode round`、引号/换行/`--flag` 不被拆参数、UTF-8 截断不劈码点且 marker 超预算时仍不越界、输出分段与 JSON 附录、500+ 文件头不超 8 KiB 且带「还有 M 个」、diff 只在 workspace 实例出现且截到 8 KiB、极端长文本下每个输出块仍 ≤ 8 KiB；`run.js` 侧覆盖 spawn ENOENT、超时/强制结算、abort、dispose 幂等、坏 JSON、空 stdout、BOM、exit≠0 带 JSON、4 KiB 诊断截断、exit 先于 taskkill、taskkill 非 0 fallback、pid 未就绪时的强制结算。

`package.json` 的 `scripts.test` 是同一个命令，也可在包目录里 `npm test`。

## 与任务书 / 契约的差异

1. **import 路径**：任务书让用 `@deepseek-ai/dsh-subagent/out-of-process`，但 0.2.1-rc.2 的 `exports` 没有这个子路径；`subprocessRunHandle` / `settleRunResult` / `NO_START_CAPABILITIES` 都由根入口 `@deepseek-ai/dsh-subagent` 重导出。代码从根入口 import。
2. **工具配置多了 `maxDepth: provider-managed`**：任务书 §3 的四条 config 没有这一项。实际 `dsh-tool-subagent` 在 provider 无 `depthLimit` 时，若 `maxDepth` 解析成数值（宿主默认 1）会拒绝挂载。不加这一项，patch 加载即失败。
3. **`rounds > 1` 时额外传 `-mode round`**：任务书 §5 的固定参数只列了 `-rounds`，但 Council 的 `ask` 在 rounds≠1 时退 2，只有 `-mode round` 才接受多轮。不传的话 `rounds` 配置是死的。
4. **多了一个 `lib/package.json`**：Node 24+ 的 `node --test <dir>/` 不再把目录展开成 `*.test.js`，而是把目录当模块入口解析；没有它，命令会去 import `lib/index.js` 并因缺 dsh 依赖失败。这个嵌套 package.json 只把 `main` 指向 `index.test.js`（该文件再 import `command.test.js`，所以两个套件都会跑），不改变包根 `exports -> ./lib/index.js`。
5. **片 2 状态**：任务书说 `workspace: true` 在片 2 落地前「可用但不通」。本仓库里 `internal/headless/workspace.go`、A7/A8/A9 测试已经存在，代码路径看起来已通；但本片没有在 dsh 里跑过 `council_work`，也没有跑真实 provider，所以仍按「未验收」对待。
6. **`-p` 的命令行长度**：任务文本作为一个 argv 元素传给 Windows `CreateProcess`，上限约 32K 字符。超长任务会启动失败（诊断里是 spawn 错误），不会自动改走 stdin。写任务时请给路径和摘要，不要把整份文件塞进去。
7. **`council_work` + `[conv:]` 追问的已知限制**：Council 的 worktree 副本路径固定为 `<data_dir>\workspaces\<convID>`，同一 conversation 再带 `-workspace` 跑会因「副本已存在」退 1；契约 §3.3 又要求不自动删副本。所以 worktree 模式的追问不要自行删除副本——把副本路径交给用户，或换一个新问题；`council` 的 `[conv:]` 追问不受影响。

## 真机验收（未执行，留给 维护者）

以下每一条都**没有跑过**：

| # | 步骤 | 期望 |
|---|---|---|
| B1 | 在 dsh 里用 `council` 委派一个简单问题 | 父级收到完成通知，`output` 含 summary 与 JSON 附录 |
| B2 | 委派后立刻取消 | 子进程树被杀；`Get-Process council` 为空 |
| B3 | 把 `councilPath` 指向不存在的路径 | 启动即失败，`stopReason: 'error'`，diagnostic 带 ENOENT |
| B4 | 同一 dsh 会话里用 `[conv:<id>]` 追问 | 第二次调用接到同一 conversation，`conversation_id` 不变 |

另外建议一并确认：`council_work` 传的是 git 仓库根、返回的 diff 不被 8 KiB 截断时完整、`partial` 结果不会让父级把失败成员当成共识。
