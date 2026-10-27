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
