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

第一次运行会在 `~/.council/config.toml` 生成一份带注释的配置，按里面的说明填 `[providers.*]` 与 `[agents.*]` 即可。设置页的「提供方」也能直接增删改（写入前会显示目标路径与新旧对比，并自动备份原文）；provider 除了 `command`，还可以带 `args`（追加参数）与 `env`（覆盖环境变量）。

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
| `council init` | 检测本机装了哪些 CLI（`--json` 给程序看），`--apply` 按勾选写配置。桌面版的首次运行向导走的就是它 |
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

`desktop/` 是 Electron 外壳。窗口里显示的是看板页面，但外壳本身不是「省掉开浏览器」那层薄包装 —— 它负责的是这个多进程系统的启动、收尾与升级：

- **第一次打开就能用**：还没有配置文件时，窗口里先出现设置向导 —— 检测本机装了哪几家 CLI（claude / codex / grok / agy）、列出模型与登录状态，勾选并起名后写进 `~/.council/config.toml`，然后才启动工作台。没有终端、没有报错对话框。
- **先连后起**：启动时探测 `http://127.0.0.1:8765/api/health`，已经有看板在跑就直接挂上去；没有再拉起 `council serve -rw`。一个 `data_dir` 只允许一个写进程，第二次双击不该撞 `council.lock`。
- **端口不写死**：8765 被占用时传 `-addr 127.0.0.1:0` 让系统分配，再从子进程输出的 `council serve: <url>` 里读回真实地址。用户看不到端口。
- **只关自己拉起的那个**：退出时只结束由它 spawn 的 `serve`（attach 到别人的进程就绝不去杀）。Windows 上先 `kill()`，2 秒不退再 `taskkill /T` 连子进程树。
- **自动更新**：启动后静默检查、后台下载，下载完问一次「立即重启」；安装前先停掉自己拉起的 `serve`。托盘「诊断 → 检查更新…」可手动检查。
- **关窗口不退服务**：窗口隐藏进托盘，托盘里可以「打开 Council」「关于 Council」「诊断」「彻底退出」。
- **记得窗口**：位置与大小存在用户数据目录，拔掉外接屏后不会被恢复到屏幕外。
- **深链 `council://conversation/<id>`**：安装了桌面版之后，这个地址会打开（或聚焦）窗口并切到那个会话；工作台认 `?conversation=<id>`，浏览器里同样能用。
- **diagnostics**：托盘「诊断 → 打开诊断日志」，记录 attach/spawn 决策、端口、健康探测、更新事件；起不来时对话框只说该做什么，细节都在这里。

它仍然只有网页做界面（窗口内没有原生控件）。

```powershell
cd desktop
npm install
npm test            # 深链解析的单元测试（node --test）
npm run dist        # 产出 dist/Council-Setup-<版本>.exe 与 portable
```

### 发版

桌面版自带自动更新（`electron-updater` 读 release 里的 `latest.yml`）。发一版要做三件事，少一件用户就收不到更新：

```powershell
# 1. 改 desktop/package.json 与仓库根 VERSION 的版本号，构建一次（版本串带 git 哈希）
powershell -NoProfile -File scripts\build.ps1

# 2. 打安装包：electron-builder 会一并产出 dist\latest.yml
cd desktop
$env:COUNCIL_EXE = "$env:USERPROFILE\bin\council.exe"; npm run dist

# 3. 建 release，把 latest.yml 一起传上去（它里面的 sha512 与那个安装包一一对应）
gh release create v<版本> dist\Council-Setup-<版本>.exe dist\Council-<版本>-portable.exe dist\latest.yml
```

已经装着的旧版会在启动后静默检查、后台下载，然后问一次「立即重启」。

**代码签名**：没有证书时 electron-builder 会跳过签名（构建日志里有一行 `signing is skipped`），安装与自动更新都能用，代价是 Windows SmartScreen 首次运行会拦一次。拿到证书后，在打安装包时设置环境变量即可，不需要改代码：

```powershell
$env:CSC_LINK = 'C:\path\to\cert.pfx'      # 或 base64
$env:CSC_KEY_PASSWORD = '<密码>'
npm run dist
```

签过名的版本会自动在 `latest.yml` 里带上发布者名，`electron-updater` 随即开始校验更新包的签名 —— 这一步不需要额外配置。

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
