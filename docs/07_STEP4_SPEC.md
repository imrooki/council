# Step 4 规格：council CLI 骨架 + 配置系统

> **订正（2026-09-30，OB1 `3317940`，#48）**：本文「council 不替用户创建配置文件 / 不创建目录」在**默认位置、文件不存在**时已被推翻：首次运行会检测本机工具、生成 `~/.council/config.toml`（见 `19d_OB1_FIRST_RUN_CONTRACT.md`）。用 `--config` 或 COUNCIL_CONFIG 指定的路径找不到时照旧报错。`DefaultTOML()` 从 #48 起是全注释的中性说明，不再是 FSDT 示例；`Default()` 没改。

范围锁死：**不实现多 agent 对话，不实现任何 provider 调用，不建 SQLite。**
Step 4 只交付一个能启动、能读配置、能把配置里的东西列出来、能干净退出的壳。

前提（已决，见 `00_DECISIONS.md`）：D-8 Go；D-9 配置在 C、数据在 E。
Go 模块代理已实测可达（2026-09-20，`proxy.golang.org` 1 秒返回版本列表），依赖可以用。

## A. 包结构

```
council/
  go.mod                       module council；go 1.25
  cmd/council/main.go          入口：全局 flag、建 root context、装信号、调 app.Run、决定退出码
  internal/config/
    schema.go                  Config / ProviderConfig / AgentConfig / TeamConfig + toml tag
    paths.go                   ResolveConfigPath / ResolveDataDir 的优先级
    load.go                    Load(path) (*Config, error)，拒绝未知键
    validate.go                交叉校验（下面 D）
    defaults.go                Default() 最小可用配置 + WriteDefault 落盘用的模板文本
  internal/app/
    app.go                     Run(ctx, Options) error：解析路径 -> 加载 -> 校验 -> 起 REPL -> 等结束
  internal/repl/
    repl.go                    循环：读一行、分派、写结果
    commands.go                命令表与各命令实现
```

不建 `pkg/`。不建 `internal/provider/`、`internal/store/` 的空壳目录 —— Step 6 / Step 5 再建，空目录是噪声。

## B. 配置文件形态

TOML。三张表都用 TOML 的 map 语法，**表键即 id**，天然唯一，不用再写 `id` 字段。

```toml
data_dir = "E:\\council-data"

[providers.claude]
command = "claude"

[providers.grok]
command = "C:\\Users\\25142\\.grok\\bin\\grok.exe"

[agents.theory]
display_name = "Theory"
provider     = "claude"
model        = "opus"
effort       = "high"
role         = "只做一阶剪切变形理论的推导与校核"
permissions  = "read"
mode         = "discussion"

[agents.critic]
display_name = "Critic"
provider     = "claude"
model        = "opus"
role         = "质疑推导中的假设与遗漏"
permissions  = "read"
mode         = "discussion"

[teams.fsdt]
display_name = "FSDT 讨论组"
members      = ["theory", "critic"]
```

（`[agents.critic]` 不是凑数：本节的示例同时是 `DefaultTOML()` 的蓝本，
而 §D-9 要求 team 成员必须是已定义的 agent。少写一个，模板就过不了自己的校验。）

`model` / `effort` 是**不透明字符串**，Step 4 不做任何枚举校验（`02` §2）。
校验归 Provider 的 `Capabilities`，那是 Step 6 的事。

## C. 路径优先级（D-9 的落地）

| 项 | 优先级，高到低 |
|---|---|
| 配置文件 | `--config <path>` flag > 环境变量 `COUNCIL_CONFIG` > `~/.council/config.toml` |
| 数据目录 | `config.toml` 的 `data_dir` > 环境变量 `COUNCIL_DATA_DIR` > 默认 `E:\council-data` |

两条都必须有独立测试逐级验证，包括「高优先级为空字符串时不应当吃掉低优先级」这种边界。
`data_dir` 必须是绝对路径，相对路径直接报错 —— REPL 的工作目录会变，相对路径没有稳定含义。

**空串与不存在是两回事**：空字符串表示「这一级没设置」，继续向下回落；
非空但指向的文件不存在，**必须报错，不得回落到下一级**。
用户明确写了 `--config D:\x.toml` 却因为打错字悄悄用了默认配置，是最难查的一类问题。

**Step 4 不创建这两个目录**，只解析与校验。测试一律 `t.TempDir()` + `t.Setenv`。
真实目录留到第一次真跑时再建。

### 「绝对路径」在 Windows 上到底算什么（实测）

有人担心 `\council-data` 这种缺盘符的路径会通过 `filepath.IsAbs`，
从而让数据落到「当前盘根下」。本机实测否定了这个担心
（go1.27.0 windows/amd64，脚本在 `E:\Isolating_test\council\go-path-probe\main.go`）：

| 输入 | `IsAbs` | `VolumeName` 长度 |
|---|---|---|
| `E:\council-data` | true | 2 |
| `E:\council-data\` | true | 2 |
| `E:/council-data` | true | 2 |
| `\council-data` | **false** | 0 |
| `/council-data` | **false** | 0 |
| `E:council-data`（盘符相对） | **false** | 2 |
| `council-data` / `.\...` / `..\...` | false | 0 |
| `\\server\share\council-data` | true | 14 |
| `\\?\E:\council-data` | true | 6 |

结论：Go 在 Windows 上要求有卷名才算绝对路径，`filepath.IsAbs` 单独一条就够用，
不需要额外写盘符检查。UNC 与 `\\?\` 前缀判为绝对，这是可接受的。

## D. 校验规则（每条都要有一个负例测试）

1. **未知键一律报错**，不静默忽略。用 `toml.MetaData.Undecoded()`，错误信息里列出所有未知键的完整路径。
   理由：配置里写错 `efort` 而被静默当成空值，会在 Step 6 变成一个查不出来的行为差异。

   **`Undecoded()` 只管「键不认识」，不管「键认识但类型不对」。**
   `members = "theory"`（该是数组）、`permissions = 1`（该是字符串）这类走的是
   TOML 解码器本身的类型错误，两条路都要有测试，别以为查了未知键就覆盖了。
2. `providers.<id>.command` 非空。Step 4 不检查该命令是否真的存在于 PATH（那是 Step 6 的能力探测）。
3. `agents.<id>.provider` 必须是已定义的 provider id，报错要指出是哪个 agent 引用了哪个不存在的 provider。
4. `agents.<id>.model` 非空。`effort` 可空（`02` §1.1：grok 的 effort 根本不能按次传）。
5. `permissions` ∈ {`read`, `workspace`, `full`}，**缺省为 `read`**（`06` §2）。
6. `mode` ∈ {`discussion`, `worker`, `discussion_and_worker`, `judge`, `manager`}，缺省 `discussion`。

   **5 与 6 共用同一套规范化规则，不许一个折大小写另一个不折**：
   键省略或写成空串 → 填缺省值；写了值 → 统一 `strings.ToLower` 后再判枚举。
   规范化在 `Load` 里做完，`Validate` 只判是否属于合法集合，不再碰大小写。
   否则 `/agents` 显示的档位与 Step 6 实际读到的会不一致。

   **`full` 能写进配置并通过校验，不等于编排器可以自动使用它。**
   `06` §2 写明 FULL 在 MVP 不提供给编排器自动使用，且 claude 侧根本没有对应的原生 flag。
   Step 4 的职责是让配置能表达这个档位，拒绝它是 Step 6 编排层的事，不是 schema 的事。
   这句话必须同时出现在 `DefaultTOML()` 的注释里，否则用户会以为写上就能用。
7. id 命名：`^[a-z0-9][a-z0-9_-]*$`。理由是 id 要当 @mention 用（`02` §2）。
8. **`user` 是保留 id**，不得用作 agent id（`05` §1：user 是保留 participant）。
9. `teams.<id>.members` 非空、每个必须是已定义 agent、不得重复。
10. 校验失败要**一次报全部错误**，不是遇到第一个就返回。改配置的人应当一次看到所有问题。

## E. 生命周期

**`signal.NotifyContext` 一个人实现不了两次 Ctrl+C。** 本机读过
`C:\Program Files\Go\src\os\signal\signal.go`：它内部的 goroutine 收到第一个信号就 `cancel` 并退出，
而 `stop()` 不会被自动调用，注册着的那个 cap=1 的 channel 从此没有读者。
所以只用它的话，**第二次 Ctrl+C 会被那个无人读的 channel 吞掉，进程既不退出也不响应** ——
比"回到系统默认终止行为"更坏。

可用的形状有两种，都必须写在 `main` 里：

其一，完全自己管，少一个活动部件：

```go
sigCh := make(chan os.Signal, 2)
signal.Notify(sigCh, os.Interrupt)
ctx, cancel := context.WithCancel(context.Background())
go func() {
    <-sigCh            // 第一次：请求优雅退出
    cancel()
    <-sigCh            // 第二次：不等了
    os.Exit(130)
}()
```

其二（**当前实现采用**），留着 `NotifyContext`，另注册一个 channel 数信号。
成立的前提是 `os/signal` 是**广播**语义：同一个信号会送给每一个注册过的 channel，
所以第二个 channel 不会把第一次信号从 `NotifyContext` 手上抢走 —— 两边各收一份，
`NotifyContext` 取消 ctx 走优雅退出，另一边把第一次数掉、读到第二次才硬退。
注册顺序必须是先 `NotifyContext` 后 `Notify`，见 `cmd/council/main.go`。

- 第一次 Ctrl+C：取消 context，REPL 收到后打印一行告别，`app.Run` 返回 nil，退出码 0。
- 第二次 Ctrl+C：`os.Exit(130)`。
- **`os.Exit` 只许出现在 `cmd/council`**。`internal/` 下任何包都不得调用它。
  理由不是洁癖：Step 6 要把同一个 `ctx` 传给 `exec.CommandContext` 去起各家 CLI 的子进程，
  进程内 `os.Exit` 会跳过所有 defer，留下一地没被杀掉的 claude / codex / grok / agy 进程。
  生命周期契约是「`Run` 返回，由 `main` 决定退出码」。
- stdin 关闭（Ctrl+Z 回车 / 管道结束）等同 `/quit`。
- **REPL 的阻塞读不能挡住 context 取消**：读 stdin 放独立 goroutine，行送 channel，
  主循环 `select` 于 channel 与 `ctx.Done()`。进程退出时那个 goroutine 会随进程消失，
  这是已知且接受的（Windows 上没有可移植的方式打断阻塞中的 console read）。
- **`select` 的公平性坑**：一行输入与 `ctx.Done()` 同时就绪时 `select` 随机挑一个，
  可能在 Ctrl+C 之后还执行一条命令。主循环取到一行之后、真正执行之前，
  必须再非阻塞地检查一次 `ctx.Err()`，非 nil 就直接走退出路径。
- 配置加载失败：打印全部错误到 stderr，退出码 1，**不进 REPL**。

## F. REPL 命令（只读，全部不触发任何外部进程）

| 命令 | 行为 |
|---|---|
| `/help` | 列出本表 |
| `/quit` | 退出，码 0 |
| `/providers` | 表格：id、command |
| `/agents` | 表格：id、provider、model、effort、permissions、mode |
| `/teams` | 表格：id、成员数、成员 id 列表 |

- 未知的 `/xxx`：打印「未知命令，试 /help」，**不退出、不返回非零**。
- 非 `/` 开头的输入：打印「Step 4 尚不支持对话，见 docs/07_STEP4_SPEC.md」。
  **[已作废，Step 5 起改了]** 有了 Canonical Conversation 之后，非 `/` 开头的输入
  直接以 `user` 身份发言，等同 `/say <原文>`。本条连同 `msgNotChat` 常量已删除，
  现行规则见 `08_STEP5_SPEC.md` §H「裸文本即发言」。
  本行保留是为了记录 Step 4 当时的行为，不要照它改回去。
- 空行：什么都不做。
- 命令分派写成表驱动（`map[string]func(...)`），**输出写到注入的 `io.Writer` 而不是直接 `fmt.Println`**，
  这样测试不需要 TTY。

## G. 测试矩阵

`go test ./...`（全路径 `C:\Program Files\Go\bin\go.exe`）必须全绿。至少覆盖：

| 组 | 用例 |
|---|---|
| paths | 三级优先级各一个正例；`--config` 覆盖环境变量；环境变量为空串时回落默认；相对 `data_dir` 报错 |
| load | 合法文件；未知键报错且错误里有键名；**类型错误**（`members` 给字符串、`permissions` 给整数）报错；TOML 语法错误的错误信息含行号；文件不存在的错误信息含路径；缺省填充；大小写规范化；三个 map 缺席时返回非 nil 的空 map |
| validate | D 的 1-9 每条至少一个负例；一个「三处同时错」的用例验证 §D-10 的批量报错 |
| defaults | `Default()` 的产物能通过 `Validate()` |
| repl | 五个命令各一个输出断言；未知命令；非命令输入；空行；`/quit` 让循环结束 |
| lifecycle | context 取消后 `Run` 在有限时间内返回（用 `context.WithCancel` + 超时断言，不用真信号） |

## H. 不在 Step 4 做

- 不连 provider、不起子进程、不做能力探测
- 不建 SQLite、不写任何持久化
- 不做 TUI（bubbletea 之类），只要 `bufio` 行读
- 不实现 `@mention`、不实现消息路由
- 不创建 `~/.council/` 与 `E:\council-data\` 真实目录
