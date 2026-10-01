# 15 - TUI 规格到 Council 的映射

2026-09-23。上游是 Griffin 交来的那份 TUI 规格（27 节，原文 2026-09-25 归档在 `reference/griffin-tui-spec.md`）。本文件做三件事：
把规格的每一节落到本仓库已有的东西上、点名真缺口、给拆刀顺序。

规格原文抬头写的是 "Many-AI CLI"。**本文件把它当设计简报读，不当作业对象**：
many-ai-cli 已裁为只读参考（`00_DECISIONS.md`），本次改的是 Council。

规格 §24 末句授权「当前架构使某部分规格不合适时，智能适配并在总结里说明理由」。
第 3 节全部是照这条走的。

---

## 1. 已经有了，一个字不用动

| 规格要求 | Council 里的东西 | 坐标 |
|---|---|---|
| §3 动态 Agent Registry，不许硬编码模型名 | `provider.Register` / `Lookup` / `IDs`，各适配器在 `init()` 自己登记 | `internal/provider/provider.go:59-92` |
| §4 Provider / Model / Agent 三者分离 | `ProviderConfig{Command}` / `AgentConfig.Model` / `AgentConfig` 整体 | `internal/config/schema.go:77-90` |
| §25.5 不许围绕 Claude/Grok/Codex 硬编码 | 编排层只认 `provider` 字符串，查不到就是 `ErrUnknownProvider`，不是 panic | `provider.go:88` |
| §12 Presets | `[teams]` + `/team use` | `schema.go:93`，`repl/commands.go:130` |
| §22 selected agents | 会话成员（`Participant.Active`），`/join` `/leave` `/who` | `repl/commands.go:135-137` |
| §13 结构化取消 | ctx 一路传到 `spawnProcess`，超时/取消走 `killProcessTree`，落 `OutcomeCanceled` | `turn/spawn.go`，`turn/record.go:46` |
| §13 一个慢模型不得挡住快模型 | `RunRoundWithStream` 每成员一个 goroutine，`onDone` 谁回来先报谁 | `turn/scheduler.go:196-260` |
| §13 一个失败不毁掉整轮 | 明写的设计：一个成员失败不取消别人，不用 errgroup | `scheduler.go:33-37` |
| §23 配置持久化、会话历史 | SQLite 事实源，只追加 | `internal/store/` |
| §16 斜杠命令系统 | 命令表 + `dispatch`，16 条命令 | `repl/commands.go:124-147` |
| §10 Judge **执行** | `--with-judge`：发言轮跑完后另取快照、另起一轮，`Speakers: plan.Judges` | `repl/round.go:217-236` |

**Judge 不是缺口。** 我最初按 `scheduler.go:39` 那句「不跑 plan.Judges」以为它没接上，
那句注释说的是 `RunRoundWith` 这一层不跑，接在上面一层的 `/round`。缺的只是选择器 UI。

---

## 2. 一个命名陷阱，任务书必须带上

规格的 **Role**（planner / architect / reviewer / judge ...）= Council 的 **`Mode`**
（`discussion` / `worker` / `discussion_and_worker` / `judge` / `manager`，`schema.go:31-37`）。

Council 自己的 **`Role`** 是另一样东西：塞进身份块的 system prompt 正文（`schema.go:85`）。

两者都叫 role，语义正交。不写清楚，实现方会造第二个 Role 类型出来。

附带一条已裁的事实，决定了 Judge 能不能热切：
**`Mode` 不进身份指纹，`Role` 进。** 见 `schema.go:104-112` 的裁决注释 ——
把某人改成裁判是免费的，改 `role` 是一次 `ErrIdentityDrift` 加一次入场费。
所以规格 §26 Scenario C（Judge 从 Claude 换成 Grok 不需要重编译）**今天就成立**，
路径是改配置 `mode` 再 `/config reload --apply`。

---

## 3. 规格里 Council 做不到的，以及为什么

### 3.1 流式（§13）-- 做不到，不是没做

`internal/turn/spawn.go:155-181`：

```go
var stdoutBuf, stderrBuf bytes.Buffer
cmd.Stdout = &capWriter{buf: &stdoutBuf, cap: maxSpawnOutputBytes}
// ...
cmd.Start()
waitErr <- cmd.Wait()
```

三家 CLI 一律 headless 一次性调用（`--output-format json`），**进程退出之后才解析**。
provider 层没有 token 流，也没有中途事件。

于是规格 §13 列的九个状态里，`thinking` / `streaming` / `tool-running` 三个
**在 Council 的数据里不存在**。板上画一个观测不到的状态就是编。

Council 真正能分辨的：

| 规格状态 | Council 有没有 | 来源 |
|---|---|---|
| queued | 有，但今天永不出现 | `maxParallel` 恒传字面量 0，没有信号量 |
| starting | 有 | `InflightPhase = begun`（BeginTurn 之后、spawn 之前） |
| thinking | **没有** | provider 不给 |
| streaming | **没有** | 同上 |
| tool-running | **没有** | 同上 |
| waiting | **没有** | 同上 |
| completed | 有 | `OutcomeSpoke` |
| failed | 有，而且分六档（实时能到的） | `ProviderFailed` / `Unparsable` / `Timeout` / `SpawnFailed` / `Refused` / `PostRejected`。（2026-09-24 订正：原来还列了 `Lost`，它只在**启动对账**时由 `recover.go` 补出来，永远不经过 `RoundProgress` 回调，板子上不会出现） |
| cancelled | 有 | `OutcomeCanceled` |

**裁决：第一版板子只画 running（带活计时器）到终态。**
`begun` / `spawned` 两态今天只落库、不经回调回传（`RoundProgress` 只在完成时触发），
要实时区分得加 `onStart` 钩子 —— 推后。
`queued` 不画：`maxParallel==0` 时它结构上不可能出现。

**Council 比规格的参照物多出来的那一档要画。** 规格假设失败就是失败；
Council 有三种「花了钱但没拿到东西」：`PostRejected`（付过钱、正文写不进去）、
`MaybeAdvanced`（provider 侧可能已推进、我方不确定）、`Drifted`（身份漂移）。
参照物丢一行是少看一眼日志，Council 丢一行是丢一笔账。这三档各有独立标记，**行不自动清**。

**2026-09-24 订正（TUI 第 2 刀调查查出，两份报告独立得出）**：
- `Drifted` **不是「身份漂移」，也不属于「没拿到东西」**。它是解析成功路径上的一个布尔（`runner.go` 在拿到正文之后才赋值，结局只可能是 Spoke 或 PostRejected）：
  正文已到手、钱已付，只是 provider 会话的前缀断了，下一轮更贵。真正的身份漂移是 `ErrIdentityDrift`，判 Refused、不花钱。板子上它只标一个提示，不和 PostRejected、MaybeAdvanced 同级。
- 行状态是「结局 × 标记」两个维度：结局（running / spoke / timeout / canceled / refused / spawn_failed / provider_failed / unparsable / post_rejected）× 标记（drifted、maybe_advanced）。
  MaybeAdvanced 只随 Timeout / Canceled 出现；Drifted 可以与 Spoke 或 PostRejected 同时出现。
- 「行不自动清」在内联模式下定义为：**终态行打进回滚区就算**（回滚区本来不会消失）。不在活区里钉住（钉住会让活区常驻变高，与看板的高度策略冲突）。
  PostRejected 的 DroppedText、MaybeAdvanced 与 Drifted 的说明行照旧由 `writeAskRecord` 整段打进回滚区，看板上的标记只是索引，不能替代它们。

**2026-09-24 进展**：表头总览形态已实现（RB-A，`69fed6c`）：忙时表头带已用时间，跑 /round 时再带「发言轮 k/N (round_id 末 6 位)  回来 x/y」。
它同时是第 3 刀（成员行进活区）在终端太矮、看板行数超限时的降级形态。数据经 `repl.RoundObserver` 三种事件进 TUI，行式路径不装。落地记录见 `00_DECISIONS.md` #41。

**2026-09-25 进展**：成员行已进活区（RB-B，`572467c`）：每个成员一行，按「结局 × 标记」两维显示（状态词 + 可能已吃下 / 正文见上方 / (前缀已断)），终态块在每段结束时进回滚区，整条 /round 结束打一行运行小结。
表头总览仍在，终端太矮时看板降级为 0 行、只剩它。第一版不用字形（第 4 刀再定）。契约原文见 `15b_RUN_BOARD_RBB_CONTRACT.md`。

### 3.2 工具活动可视化（§14）-- 第一版不做

同一条根因：拿到的是终稿 JSON，没有工具事件流。
三家的 JSON 里有没有可复原的工具轨迹，**没查过，记 [未验证]**。
查证之前不画那棵树，画出来只能是假的。

### 3.3 补丁 / 受控写盘阶段（§15）-- 第一版不做

Council 没有自己的 patch 系统：agent 在自己的 workdir 里按权限档直接干活
（`internal/turn/permission.go`，`provider` 层的 `WriteBoundary` 闸）。
规格描述的 proposed patches 到 judge 到 one controlled writer 是另一个产品形态。
规格 §15 说 preserve any existing patch approval workflow if one already exists ——
**不存在，所以没有要保的**。

### 3.4 协作模式（§11）-- 三个已有，四个推后

已有：`direct`（`/ask`）、`ask-all`（`/round --go`）、`judge`（`/round --go --with-judge`）。
推后：`sequential`、`review`、`debate`、`parallel`（`parallel` 与 `ask-all` 在 Council 里是同一件事）。
规格 §11 自己写了「不要一次把每个策略都过度工程化」。

接口留法：模式是一个值，选出一个策略；新策略是纯追加，不改 TUI。

---

## 4. 真正要新写的

| # | 东西 | 规格节 | 依赖 |
|---|---|---|---|
| 1 | TUI 外壳（bubbletea v2 内联） | §5 §6 §19 §20 | 第 0 刀的缝 |
| 2 | Run board（每成员一行活计时器） | §7 | UX-1 的 `RoundProgress`（已落地） |
| 3 | Agent Selector | §9 | UX-2 行式部分已落地（`2923bf2`：`/add`、前缀、id 许点号）；选择器 UI 未做 |
| 4 | Judge Selector | §10 | 后端已有，只差 UI + 一条裁决 |
| 5 | 模式选择器 | §11 | 无 |
| 6 | 命令面板 | §17 | 命令表已有 `summary` 字段，直接喂 |
| 7 | Compare view | §8 | `Record.Message` 已有全文 |
| 8 | 展开/折叠单个成员结果 | §7 | 同上 |

---

## 5. 需要 Griffin 裁的四条

1. **内联还是全屏（alt-screen）。**
   规格 §6 画的是顶栏 + 底栏的全屏框。调研推荐内联（`tea.Println` 把定稿行推进终端
   原生回滚区，保住滚轮、选中、复制）。规格自己写 conceptual layout, adapt intelligently，
   且它点名的参照物 Claude Code / Codex CLI 都是内联的。
   **我按内联走，头部信息做成活区顶部一行。** 要全屏就说。

2. **Judge 选择器改的是配置还是会话。**
   今天 judge 身份来自配置 `mode='judge'`，`PlanRound` 读的是配置 map。
   选择器如果只改本会话，需要新的每会话状态；如果改配置，就是替用户写 `config.toml` 再 reload。
   **倾向：本会话覆盖，不动用户的配置文件。** 但这是新状态，要单独一刀。

3. **一轮进行中输入框：禁用还是排队。**
   今天语义是「扇出期间 REPL 不接受任何命令」（`round.go:281`）。
   参照物都排队，但它们只有一个 agent；Council 排队要先裁 `r.cur` / `r.svc` 的
   并发归属，而且裸文本即发言不可回收。
   **倾向：第一版禁 Enter**，TUI 只负责把「为什么不能输入」从一句提示变成看得见的状态。

4. **Ctrl-C 的新语义。**
   改成「第一次取消本轮回到提示符、第二次退出」是参照物的肌肉记忆。
   **但以探针 P1 通过为前提**。

---

## 6. 两个探针，Griffin 亲手在真控制台跑

子代理的 stdio 被重定向，拿不到 CONIN$，这两条不能靠读文档定。

**P1（直接关系到钱，raw 模式一行代码落地前必须过）**

最小 bubbletea v2 程序起 N 个子进程，按 Ctrl-C，查子进程退出码。

- `3221225786` = 控制台仍在广播 CTRL_C_EVENT
- `1` = 只有 `killProcessTree`（`spawn.go:190-196`）杀掉了它
- **还活着 = 孤儿进程继续烧 token**

不处理的后果：用户按 Ctrl-C 以为停了，五个 provider CLI 继续跑到 5 分钟硬超时，
继续花钱，而且这笔钱没有任何记录能对上。
`round.go:59-63` 那条 2026-09-21 实测是在熟模式下做的，代码路径不同，不能搬。

**P2**

同一程序在 conhost 与 Windows Terminal 各跑一次：

- 粘多行文本，看 `PasteMsg.Content` 是否完整
- `tea.Println` 在回滚区会不会闪、错位
- **状态符号渲染宽度**：下一节那四个是 East Asian Ambiguous 宽度，
  conhost 下占一格还是两格会决定整张表对不对齐。对不齐就退回纯 ASCII 标记。

P2 失败则「对话记录用原生回滚区」这个最大卖点打折，改用 bubbles 的 viewport。

---

## 7. 符号纪律（任务书必须原样带上）

规格的 mockup 里那个对勾是 U+2713，落在 Dingbats U+2700-27BF，**CLAUDE.md 禁令区内**。
任务书里出现它，实现方会照抄进源码。

替代一组，全部在放行范围内：

| 状态 | 符号 | 码位 | 为什么可以 |
|---|---|---|---|
| 完成 | 实心圆 | U+25CF | Geometric Shapes，不在禁令表 |
| 进行中 | 半填圆 | U+25D0 | 同上 |
| 排队 | 空心圆 | U+25CB | 同上（今天用不到） |
| 失败 | 乘号 | U+00D7 | CLAUDE.md 放行清单明写 |

Box Drawing（U+2500-257F）不在禁令表。
但禁令的**理由**是 GBK 控制台渲染，所以宽度归 P2 探针一起测。

---

## 8. 拆刀顺序

每一刀独立落地、独立验收，不许是「重构一半」。

- **第 0 刀 - 冻缝 + 护栏**。零 UI。今天没有任何用例卡在
  `dispatch` / `r.out io.Writer` / 行式非交互路径这三条缝上，重构时被顺手拆掉
  不会有一条变红 —— 126 个顶层用例会先因为编译不过被一起改掉，回归网就这么没了。
  全案最便宜的一刀，先做。
- **第 1 刀 - TUI 外壳**。bubbletea v2 内联，isatty 分流（非 TTY 走今天的 `Run`，
  既有用例一字不改），composer，定稿行推回滚区，busy 期间禁 Enter。
- **第 2 刀 - Run board**。压 UX-1 的 `RoundProgress`。
- **第 3 刀 - Agent Selector**。UX-2 契约（`/add` + 前缀匹配 + id 允许点号 +
  `/ask` 非成员烧钱修复）内容全部仍然有效，UI 换成选择器。
  **（2026-09-23）行式部分已落地**：`/ask` 非成员烧钱修复是 X-1（`eb1e464`），`/add` + 前缀 + 点号是 `2923bf2`。
  剩下的只有选择器 UI 本身，它调的就是 `/add` 的解析。
  **`/ask` 那个烧钱 bug 与 TUI 无关，无论哪条路都要落。**
- **第 4 刀 - Judge / 模式选择器 + 命令面板**。
- **第 5 刀 - Compare view、展开折叠、响应式打磨**。

---

## 9. 依赖的实测事实

| 事实 | 值 | 来源 |
|---|---|---|
| `charm.land/bubbletea/v2` 版本 | v2.0.9 | 本机 modcache |
| 它要求的 Go 版本 | `go 1.25.0` | 该版 go.mod:5 |
| council 的 Go 版本 | `go 1.25.0` | 本仓 go.mod:3 |
| **要不要动 go.mod** | **不要** | 上面两行一字不差 |
| 配套模块 | `bubbles/v2 v2.2.1`、`lipgloss/v2 v2.0.6` | 本机 modcache |
| 模块路径 | `charm.land/...`，**不是** `github.com/charmbracelet/...` | 按老路径 go get 直接报错 |
| 二进制增量 | **+3.50 MiB**（默认）/ +2.54 MiB（`-ldflags "-s -w"`） | 侦察腿真构建对照，见下 |
| go.mod 新增 | **18 行 require**（3 直接 + 15 indirect），go.sum 52 -> 94 行 | 拷贝沙箱里 `go mod tidy` 实测 |
| `-race` | **跑不了**：`-race requires cgo`，gcc 不在本机 | 本会话两次实测 |

**订正一处我抄错的数字**：调研腿报的是 +2.30 MiB，我照抄进了本文件第一版。
侦察腿用「同样的 ctx + 信号 + isatty 骨架、只差 TUI」做对照物真构建，量出的是
**+3,673,600 字节（3.50 MiB）**。两个数不是同一个对照基准，以后者为准。

**伪版本那条的疑问解开了**：`ultraviolet` 被解析成 `v0.0.0-20260811164956-...`
而不是 bubbletea 自己 go.mod 里钉的 `20260703014108-...`，是因为
**lipgloss v2.0.6 的 go.mod 要求更高的版本**，MVS 取高。不是漂移，是正常解析。
`GOPROXY=off go mod tidy` 逐字节复现同一结果，所以这个列表是确定性的。
但它仍是未打 tag 的伪版本，整个 Windows 输入输出传输层压在它上面，
API 稳定性 **[未验证]**，记进决策。

## 10. bubbletea v2 的四个坑（侦察实测，任务书必须原样带）

这四条每一条都**能编译通过**，失败在运行时或静默，靠读文档发现不了。

1. **`tea.WithAltScreen()` 在 v2 里根本不存在。** 改成 `View` 结构体上的
   `AltScreen` 字段。照 v1 的记忆写，整刀作废。
2. **`Model.View()` 返回 `tea.View` 结构体，`textarea.View()` 返回 `string`。**
   中间必须垫一个 `tea.NewView(...)`。
3. **textarea 的 `Focus()` / `Reset()` / `InsertRune()` / `SetWidth()` 都是指针方法。**
   在值接收者的 `Init()` 里写 `m.ta.Focus()` **编译得过**，但 focus 落在副本上直接丢，
   textarea 永远 blurred、`Update` 第一行就 return。
   表现是「输入框完全没反应」，编译器一声不吭。
4. **`tea.KeyMsg` 是接口，`tea.KeyPressMsg` 才是具体类型。**
   type switch 里把 `case tea.KeyMsg` 写在 `case tea.KeyPressMsg` 前面，
   Go **不报 unreachable**，后面那个 case 永远进不去。

附带两条同类：`tea.Batch` 里塞多条 `tea.Println` **顺序不保证**（要顺序用 `tea.Sequence`）；
**Enter 默认被 textarea 的 `InsertNewline` 绑走**，不先 `KeyMap.InsertNewline.Unbind()` 抢不回来。

## 11. 非 TTY 下 bubbletea 的表现（侦察实测，决定 isatty 分流怎么写）

- **`Program.Run()` 不失败、不报错、正常返回。** 不能拿它当 isatty 判据。
- **stdin EOF 不会让它退出**：`./probe.exe < /dev/null` 配 8 秒 timeout -> exit 124。
  `< NUL` 且没有超时机制 = 僵尸进程。
- 非 TTY 下只送一条 `WindowSizeMsg{0, 0}`，**此后再也没有第二条**。
- **renderer 宽度为 0 时活区一个字节都不写**，而 `tea.Println` 照常工作。
  在 model 层给 width 兜底**解决不了**，renderer 的宽度还是 0。
- **`tea.WithoutRenderer()` 不是退路**：实测输出 0 字节，`tea.Println` 被静默吞掉。

**结论：isatty 分流必须在 `NewProgram` 之前做，走完全不同的代码路径。**
判据用 `repl.IsTerminal`（第 0 刀已落地）。

## 12. 内联模式的一条硬约束

~~退出时活区被 `\r\x1b[J` 抹掉，最后一帧 `View()` 不会留在屏幕上。~~ **这句是错的**（2026-09-24 订正）：
bubbletea v2.0.9 正常退出时把最终帧再画一次，`close()` 只擦帧末行，上面各行留在屏幕上（真机复验看到的残影，#39(e)）。
X-39e（`4edced9`）起，正常退出（空闲 Ctrl-C、`/quit`、出错退出）由 `runWith` 第 2b 步补一段清屏擦掉；被杀的路（SIGTERM、关窗）不擦，残影照旧。
所以最后一帧的内容**不能指望**留下来给用户看，这条约束照旧成立：
**任何要让用户退出后还看得见的东西，必须在 `tea.Quit` 之前经 `tea.Println` 送出去。**
告别行、结算行、错误原因都归这一条管。

## 13. 中文光标定位的已知风险（侦察实测 + 一条未实测的推论）

textarea 内部**三套宽度实现混用**：行宽/换行用 `rivo/uniseg`，
**光标列偏移用 `mattn/go-runewidth`**（`textarea.go:748`），终端渲染用 `x/ansi`。

本机实测：纯 CJK（中 文 全角括号）三套一致等于 2，**光标定位对得上**。
放行符号那批（希腊字母、度、正负、破折号、箭头、偏导）三套一致等于 1，也对得上。

**但这个一致性有条件**：`go-runewidth` 的 `IsEastAsian()` 本机是 `false`，
而它是 **locale 相关的**（`RUNEWIDTH_EASTASIAN` 环境变量 / `LC_*`）。
（2026-09-24 订正：在 Windows 上判据不是 `LC_*`：`WT_SESSION` 非空（Windows Terminal 里）返回 false；
否则看 `GetConsoleOutputCP` 是否属于 932/936/949/950。TUI 第 2 刀调查实测：从 Bash 起、代码页 936 的进程得 **true**，
所以在 conhost + 中文代码页下，● ◐ × 省略号这类模糊宽度字符 runewidth 按 2 格算、ansi/lipgloss 按 1 格算，textarea 的光标会漂。）
一旦翻成 true，`RuneWidth('α')` 变 2 而 uniseg/ansi 还是 1，
`textarea.go:748` 的光标会**每遇到一个 ambiguous 宽度字符右漂一列**。
这个失效模式 **[未实测]**，是从三套实现的分歧推出来的。
**真终端里的中文光标定位也没验证过**（子代理跑不了交互），只有宽度函数级的一致性证据。

## 14. TUI 之后会变味的注释 —— 逐条核实，调研腿有一条给错了

调研腿列了一批「TUI 之后会指向空气的注释」，建议同刀订正。本会话逐条核实，
**其中两条它判错了，而且方向危险**。

### 14.1 调研腿判错的第一条：`scheduler.go` 的 `progressMu`

它说「TUI 路径下 `progressMu` 确实冗余，但别删，免得将来有人把 out 换回
`io.Writer` 撞上行穿插 bug」。

**冗余这个判断是错的。** `LineWriter` 保证的是**一次 `Write` 的字节不被撕开**，
它不把多次 `Write` 攒成一组。而 REPL 渲染一条 Record 要连打六次 `printf`
（结局、会话、正文、用量、金额、冷暖各一次）。两个 goroutine 各打六次，
`LineWriter` 之后仍然会在**行粒度**上互相穿插 —— 一条记录读到一半冒出另一个人的数字。

**`progressMu` 在 TUI 之后不但不冗余，它是唯一把一条 Record 的六行拢在一起的东西。**

真正过期的只有那段括号里的机制描述（`scheduler.go:188-190`）：
「internal/app 把 os.Stdout 原样交给 repl.New，REPL.out 是 io.Writer，
printf 直接 WriteString，中间没有 bufio」。TUI 之后交进去的是 `LineWriter`。
**订正机制描述，结论一个字不改，锁一定不许删。**

### 14.2 调研腿判错的第二条：三处点名 `readLines` 的注释

它把 `runner.go:46-47`、`spawn.go:88-89`、`conformance_test.go:56` 归进
「会指向空气」。**不对。** 这三处说的是同一件事：

> 子进程的 stdin 必须接 null device，绝不接编排器自己的输入句柄：
> repl.go 的 `readLines` goroutine 正阻塞在那同一个句柄上，抢起来胜负随时序漂移。

TUI 之后，TTY 路径上阻塞在 stdin 的**不再是 `readLines`，而是 bubbletea 的输入读取器**。
**危险本身一个字没变，变的只是那个读取者的名字。**

而且**危险变大了**：bubbletea 把控制台切进 raw 模式，子进程若偷走 stdin 的字节，
坏掉的不再只是 REPL 少读一行，而是按键流被污染 —— 用户敲的东西会乱。

**所以这三处要改的是符号名，不是删掉。** 把「repl.go 的 readLines goroutine」
改成「编排器自己的输入读取器（行式路径是 `repl.go` 的 `readLines`，
TUI 路径是 bubbletea 的输入读取器）」。

实际命中五处不是三处：上面三处，加 `repl.go:190`（`readLines` 自己的 doc，
不动）与 `fairness_test.go:28`（行式路径的用例，不动）。

### 14.3 真的会指向空气的

| 位置 | 现在说什么 | TUI 之后 |
|---|---|---|
| `round.go:56-63` | 2026-09-21 实测：第一次 Ctrl-C 腰斩本轮、**council 整个进程退出**；Windows 把 Ctrl-C 送给整个控制台进程组，子进程退出码 3221225786 | **两句都可能不成立**。第一句：TUI 下 Ctrl-C 是按键，第一次取消回提示符、不退出。第二句：**归 P1 探针裁**，raw 模式下控制台还广不广播没人知道 |
| `killtree_windows.go:12-14` | 「不设 `CREATE_NEW_PROCESS_GROUP` —— 那会让 Ctrl+C 不再传到垫片」 | 这句的**前提**是「Ctrl+C 会传到垫片」。TUI 下 Ctrl-C 根本不产生信号，前提消失。**同样归 P1 裁**：若 P1 报「危险」，那这个不设进程组的理由已经不成立，反而该重新考虑设它 |
| `round.go:277,281` | 「扇出期间 REPL 不接受任何命令」 | **语义仍然成立**（busy 期间 Enter 被禁），但机制从「dispatch 是同步的」变成「Enter 被闸住」。那句提示文本在 TUI 下应当换成看得见的状态，不是一行字 |
| `repl.go:154/164/173` | 三处补换行，理由是「光标停在提示符后面且没有回显换行」 | 仅 TUI 路径过期；行式路径保留。提示符在 TUI 下归活区管 |
| `main.go:83-85` | 「已知缺口：连按两次 Ctrl+C 走 os.Exit，控制台模式还原不了。本片不改」 | **TUI 这一刀必须改**。今天漏的只是括号粘贴模式（化妆品级）；raw 之后漏的是用户终端回不来 —— 无回显、光标乱、必须 `reset`。**2026-09-23 补**：TUI 模式早已不注册硬退 goroutine；X-39b（`80143b8`）之后一行模式也不再改控制台模式，这段缺口连同 `EnablePaste` 一起删了 |

### 14.4 一条纪律

上面每一条都是**先读了当前代码原文再判的**，不是照抄调研腿的清单。
两条判错正好说明这一步不能省：一条会让我删掉唯一拢住六行输出的那把锁，
另一条会让我删掉一段仍然成立、而且危险变大了的警告。

## 15. 一层我写进契约、但实际不存在的保护（实测证伪）

第 1 刀之三的契约原本写着：`runWith` 在 `p.Run()` 返回之后换出口时，
`SetEmit` 会把「事件循环死得太早、没送出去」的行补吐到兜底出口。

**这句话是错的，而且不只是测不出来 —— 它在生产上也不成立。**

照 `internal/tui/writer.go` 的 `ingest` 逐行走：

```go
	lines = w.splitLocked()
	fn = w.emit
	if fn == nil {                                  // <- 只有这一支才填 pending
		w.pending = append(w.pending, lines...)
		lines = nil
	}
```

装上非 nil 出口之后，行**直接交给出口**，不进 `pending`。出口调 `(*tea.Program).Send`，
事件循环已死时 `Send` 是空操作，行就此消失，`LineWriter` 手里不再有它。
换出口时 `swapEmit` 发现 `pending` 是空的，没有任何可补吐的。

测试腿对着**按契约五步写的参照实现**实测，失败信息里这一句是最硬的证据：

> `Send` 收到的是 `[丢不得]` —— 行确实经出口走了，只是没人接。

**根因在设计不在测试**：`(*tea.Program).Send` 不返回任何东西、也不给 ctx，
包装层无从知道一次投递落空了。要补上得给它加一个投递回执，而 bubbletea 不提供。

### 裁决

**承认这层保护不存在，把残余窗口写明，不假装它被护住了。**

`Run` 返回之后换出口这一步仍然要做，但它真正买到的只有一件事：
**`Run` 之后 REPL 的输出直接落屏幕，不再投进一个已经死掉的事件循环。**

### 残余窗口

事件循环停掉、而 `p.Run()` 还没返回的那一瞬，此刻写进来的行会经 `Send` 落空而丢失。
窗口是微秒到毫秒级。`Run` 返回之后的写入由换出口那一步接住，不在窗口内。

**今天没有办法把它关掉**，所以只记录，不声称已处理。
真要关，得让 `program` 接口带一个投递回执或 ctx 把手，让出口在投递不出去时
把行退回 `LineWriter`。记为剩余项。

### 为什么这条值得单独记一节

它是本次三刀里唯一一处**我写下的保护被实测证伪**的地方。
前面几处都是「契约写得不够」（验收不可满足、缺字段、标题与条件矛盾），
这一处是「契约声称的机制不存在」—— 落地之后会变成一句写在注释里、
下一个人会信的谎。

## 16. 契约把一个死锁写成了验收（TUI-1d，2026-09-23，`da88d20`）

第 1 刀之三的契约验收 1 要求「进 `Run()` 那一刻，启动横幅已经经 `Send` 交出去」。
测试用的假 program 的 `Send` 从不阻塞，所以全绿。真 `(*tea.Program).Send` 往无缓冲的 `p.msgs` 发，
要等事件循环消费；事件循环要等 `p.Run()` 被调用才起来，而补吐就在调 `p.Run()` 的那个 goroutine 上、在它之前。
Griffin 第一次在真终端里敲 `council`：屏幕一片空白，CPU 时间不动，Ctrl-C 能退出（根 ctx 取消后 `Send` 从 `ctx.Done` 那一支返回）。

同源的两处一起修了：
- 运行期每行在 `Update` 里各返回一个 `tea.Println` Cmd，bubbletea 给每个 Cmd 单开 goroutine，行的到达顺序没有保证（旧路径上 500 行里量到 12%-20% 的运行乱序）；
- bubbletea 的 `insertAbove` 对空串直接返回，分段空行从屏幕上消失。

现在的顺序（`internal/tui/run.go` 的 `runWith`）：
1. 启动行在 `p.Run()` 之前**直接写 final**（生产上是 stdout），不经事件循环；
2. 运行期出口换成 `p.Send(printLine(line))`：`printLine` 当场执行 `tea.Println`，把 bubbletea 内部的打印消息送进事件循环，
   事件循环在 `Update` 之前按到达顺序处理；空串换成一个空格。

**测试装置的教训**：跨并发原语的顺序至少要有一条用**真依赖**跑。`tui1d_startup_test.go` 的 A1、A2 用无终端的真 `tea.Program`
（`WithInput(nil)`、输出写 buffer）；A1 先在旧代码上跑红（5 秒内输出为空）才算数。
假件比真依赖宽松时，全绿证明的是假件，不是代码。

§15 的残余窗口不受影响：它说的是运行期出口在事件循环死后落空，与启动顺序无关。
