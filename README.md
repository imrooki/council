# Council

本机多模型议会：把同一个问题同时交给你已经登录的几个命令行模型，各自发言、可选多轮与汇总，会话与费用记在本地 SQLite 里。

Council 不替你登录任何服务，只调用你机器上已装好、已登录的 `claude` / `codex` / `grok` / `agy`。

## 三种用法

| 入口 | 命令 | 适合 |
|---|---|---|
| 终端 | `council` | 日常问答、多轮议会、复制结果 |
| 看板 | `council serve -rw` | 图形界面：会话、成员、工作区、导出 |
| 工具 | `council mcp` | 挂进 Claude Code 等宿主，让它自己调用 |

还有 `council ask`（一次性、输出 JSON、可脚本化）和 Electron 桌面版。

## 快速开始

```powershell
# 编译（需要 Go 1.25+）
go build -o council.exe ./cmd/council

# 终端界面
./council.exe

# 看板：只读 / 可跑议会
./council.exe serve
./council.exe serve -rw -app
```

第一次运行会在 `~/.council/config.toml` 生成一份带注释的配置，按里面的说明填 `[providers.*]` 与 `[agents.*]` 即可。

最小配置：

```toml
[providers.claude]
command = 'claude'

[agents.reviewer]
display_name = "Reviewer"
provider = "claude"
model = "opus"
role = "挑毛病，不夸人"
permissions = "read"
```

## 议会怎么跑

```
你 ──▶ 成员 A、B、C（并行，各自一个 CLI 进程）
        │
        ├─ 第 2 轮：各自看到上一轮的全部发言
        └─ 汇总成员：只给你看的一段结论
```

- **成员**由「哪家工具 + 哪个模型 + 思考强度 + 角色 + 权限」组合；
- **权限**：`read` 只读、`workspace` 可改文件、`full` 完全放开；
- **预算**：每一轮有硬上限，超了立刻停；
- **账**：每轮的 token 与费用都落库，按成员、按轮次可查。

## MCP

```powershell
claude mcp add council -- C:\path\to\council.exe mcp
```

挂上之后宿主可以使用三个工具：`council_ask`（单轮）、`council_round`（多轮 + 汇总）、`council_models`（列出可用成员）。协议是 stdio 上的 JSON-RPC，不依赖任何 SDK，日志全部走 stderr。

## 本机 agent（可选，默认关闭）

看板与 CLI 能顺带看一眼**本机正在跑哪些 agent CLI**，并在需要时登记一条资源租约。它是可选模块，配置里那一个键控制一切：

```toml
local_agent = "off"          # 默认值：一次进程都不枚举、不起后台任务、不接受租约
# local_agent = "observe"    # 只读观察
# local_agent = "coordinate" # 观察 + 允许申请资源租约
```

| 命令 | 作用 |
|---|---|
| `council who` | 只读列出三件事：在飞的成员、本机 claude/codex/grok/agy 进程、仍然有效的租约。不拿锁，TUI 或看板开着时也能跑 |
| `council who -server <url>` | 同一份视图，但事实来自**正在跑**的看板快照：多出子进程 PID 与「这一轮对应哪个进程」的合并结果（只有它拿得到），例如 `http://127.0.0.1:8765` |
| `council claim <目录\|tcp:端口>` | 申请一条租约（`-mode read\|write`、`-ttl 5m`）。要 `coordinate` |
| `council claim -server <url> …` | 同上，但走**已经开着**的看板（它握着 `council.lock`）；地址就是 `serve` 启动时打印的那一行，例如 `http://127.0.0.1:8765` |
| `council release -lease <id>` | 交还租约（也可 `-server`）。不需要 `coordinate`：关小开关不该把授权锁死 |

设置页第 6 类「本机 agent」也能改层级、看实例与租约、点按钮申请/交还。

三条边界，界面上也这么写：

- **默认关闭时零行为**：不枚举进程、不起后台循环、不读库；`council who` 只打印一句「未开启」。
- **租约是提醒，不是文件锁**：它拦不住任何进程写文件，只在页面上和 `council who` 里说清「这个目录有人占着」。
- **不新增任何模型调用**：三档都是纯本地事实。`agy`（Google Antigravity CLI）处于条款灰区，只观察它的进程，不自动拉起、也不参与任何自动审查路径。

一个 `data_dir` 同时只允许一个写进程（看板、TUI、MCP 都算）。所以看板开着时，`council claim` 会明确拒绝并告诉你去停哪个进程 —— 想继续用命令行，加 `-server` 让它走那个看板；两条路拿到的租约、退出码、文案完全一样。

## 桌面版

`desktop/` 是 Electron 外壳：关窗口进托盘，托盘里可彻底退出。

```powershell
cd desktop
npm install
npm run dist        # 产出 dist/Council-0.1.0-portable.exe
```

## 当前状态

- 只在 Windows 上实测过；Linux/macOS 有代码分支但没人跑过。
- 自动测试全绿（13 个 Go 包 + Node 侧测试），关键约束都有回归用例：数据库迁移、身份指纹、并发换配置、写配置只动一段。
- 许可证：MIT。
- 未做代码签名，Windows SmartScreen 会拦一下。

## 文档

决策记录、每片的取舍与验收报告放在私有仓库，不随代码公开。代码里的 `#45`、`未决 #20` 这类编号指的是那些记录。

构建与发布：

```powershell
./scripts/build.ps1     # 版本号 = VERSION + git 提交短哈希[.dirty]
```
