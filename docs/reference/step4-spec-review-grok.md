# grok 对 Step 4 规格的审查（2026-09-20）

模型 grok-4.6，经本机 CLIProxyAPI 中转，`reasoning_effort = high`。
审查对象是 `docs/07_STEP4_SPEC.md` 的 A–H 八节（审查发起时 `07a_STEP4_API_CONTRACT.md` 尚未写出，
grok 没有把它当作权威输入，这解释了下面若干"含糊"其实已被契约文件定死）。

## 编排者的两条更正，读报告前先看

**一、报告末尾那段「越界扫描 mandatory scan report」是误判，不要采信。**
它把 `go.mod`、`go.sum`、`docs/07a_STEP4_API_CONTRACT.md`、`internal/repl/repl.go`
四个文件判成 grok 违规写盘。实际来源：前三个是编排者（Claude Opus 5）自己写的，
第四个是并行跑的 Workflow 子代理写的。grok 这一轮全程只读，没有碰过磁盘。
`.git` 被触碰是编排者执行 `git init` 与首次提交留下的。
教训是：越界扫描必须以「调用外部模型之前的文件清单」为基线做差集，
不能拿「当前未跟踪文件」当证据 —— 并行有其它执行者在写盘时，后者必然误判。

**二、被实测推翻的一条。** 报告的 Top 3 风险第 3 条称
`\council-data` 与 `/council-data` 会通过 `filepath.IsAbs`，从而落到当前盘根下。
本机实测（go1.27.0 windows/amd64，脚本 `E:\Isolating_test\council\go-path-probe\main.go`）：
两者 `IsAbs` 均为 **false**，`E:council-data` 也是 false。
Go 在 Windows 上要求有卷名才算绝对路径。该风险不成立，结论表已并入 `07_STEP4_SPEC.md` C 节。

## 被采纳并已改进文档的意见

| grok 的意见 | 处置 |
|---|---|
| E 节用 `signal.NotifyContext` 实现不出「两次 Ctrl+C」，规格内部不自洽 | **成立且更严重**。读 `C:\Program Files\Go\src\os\signal\signal.go` 确认：它的 goroutine 收完第一个信号就退出且不自动 `stop()`，第二次信号被无人读的 cap=1 channel 吞掉，进程既不退也不响应。已重写 E 节，给出两种可用形状，并说明当前实现靠 `os/signal` 的广播语义成立 |
| `os.Exit` 若出现在 `internal/` 会在 Step 6 跳过子进程清理 | 成立。E 节已写死「`os.Exit` 只许出现在 `cmd/council`」及理由 |
| D.5 折大小写而 D.6 不折，规格内部不一致；空串语义未定义 | 成立。已合并成一条共用规范化规则：省略或空串填缺省，有值则统一小写后判枚举，规范化在 `Load` 完成 |
| `full` 被写成普通合法值，与 `06` §2「FULL 不提供给编排器自动使用」没有接口 | 成立。已写明「能通过校验 ≠ 编排器可自动使用」，并要求这句话出现在 `DefaultTOML()` 注释里 |
| B 节示例 `members = ["theory", "critic"]` 但只定义了 `[agents.theory]`，过不了自己的 D-9 | 成立，真文档缺陷。已补 `[agents.critic]` 并注明该示例同时是 `DefaultTOML()` 的蓝本 |
| `Undecoded()` 管不到「键认识但类型不对」 | 成立，测试矩阵确有缺口。已补 `internal/config/load_types_test.go`，覆盖 `members` 给字符串、`permissions` 给整数、`data_dir` 给数组 |
| 「高优先级非空但目标不存在」必须报错、不得回落 | 成立。已写进 C 节，并补了对应测试 |
| A 节写 `go 1.25` 与 D-8 的 Go 1.27 不一致 | 成立但无害。已在 D-8 注明 `go` 指令是语言版本下限、toolchain 是编译器，两者本就不必相等 |
| `00` 未决 #6 仍写「模块代理未测」，与 `07` 开头「已实测可达」矛盾 | 成立。未决 #6 已标记为已决并填入实测数字 |
| `05` §5 说 SQLite 放配置目录，与 D-9 的数据目录冲突，Step 5 作者会照 `05` 放错地方 | 成立。已改 `05` §5，指向 `config.ResolveDataDir()` 的返回值 |
| `02` 把 judge 同时放进 `RuntimeMode` 和「`Role == "judge"`」两处 | 成立，但不是 Step 4 能独断的。已记为 `00` 未决问题 #7，留给 Step 9 之前裁定 |
| `select` 在「一行输入」与 `ctx.Done()` 同时就绪时随机，可能在 Ctrl+C 后多执行一条命令 | 成立。已写进 E 节：取到一行后、执行之前再查一次 `ctx.Err()` |

## 未采纳的意见

- **`user` 保留 id 应同时禁止用于 provider 与 team**。`05` §1 的保留语义只针对 participant，
  provider 不是 participant。保持只禁 agent id。
- **`command` 应 trim 后判非空**。可以加，但 Step 4 不检查命令是否可执行，
  trim 与否不改变任何行为，留到 Step 6 做能力探测时一并处理。
- **provider id 与 agent id 撞名应当禁止**。两套命名空间，协议上不冲突，
  只是可读性建议。不在 Step 4 加这条限制。

## grok 原始报告（未删改）

以下为 grok 返回的四段报告正文，除上面两条更正外未作修改。
其中「越界扫描」一节已被证伪，保留是为了记录该失效模式。

> 第一段 结论
>
> 这份规格不能原样开工。配置形态、只读 REPL、校验 1–4/7–10、以及把 SQLite / provider / TUI 推迟，
> 方向与 00 / 02 / 05 / 06 大体相容，B 节「表键即 id」在 participant 语义下也成立。
> 真正挡住写码的是两处会让实现者做出互不相容选择的含糊：E 节用 signal.NotifyContext
> 无法实现它自己写下的「两次 Ctrl+C」；D.5/D.6 的缺省值没有规定「缺省 / 空串 / 大小写」
> 如何落成内存里的值，且 full 被写成合法配置，与 06 对 FULL 的 MVP 禁令没有接口。
> 这两处不先改规格，Step 4 的测试会把错误语义锁死，Step 5/6 再改是行为变更而不是加功能。

后续三段（证据、Top 3 风险、不确定点）的逐条处置见上表。
完整原文保留在本次会话记录中；本文件的上表已把每一条映射到具体的文档改动或驳回理由。
