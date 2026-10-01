# CO1 冻结契约：简洁输出档位 + 每个模型的用时（v1.1，含 W1b 修订，2026-09-28）

> **落地后订正（2026-09-28，主代理；CO1 已于 `1c62dbb` 落地）**：
> - 「总用时只在 /round 真起过进程时打」（§0 第 8 条、§1.1 E2、§7.1）措辞不准：`spawned` 来自 runOneRound 的返回值，意思是**至少一段进入了扇出**（RunRoundPreparedStream 已调用），成员仍可能全被拒或起不来、没有任何进程，这时照打总用时（与 TUI 收尾行的 runSeen 门一致）。代码与冻结的 E3、E4 相符，错的是这几句话；落地代码的注释由 fu1 订正。
> - §4「判定为等价」第 2 条（`start := r.clock()` 挪到 TakeRoundSnap 之前）**已不等价**：v1.1 的 R11 在拒绝路径与 Participants 失败路径上数时钟，那些路径在取快照之前返回，集成员实测被 R11 refusals 与 round_report_err 杀掉。
> - 独立审查余项（代码都没错）：详细档那一侧的 no_delta 门、「点名了」门、「总用时」写失败都没有用例，由 fu1 补。

> **主代理裁定（2026-09-28 05:30，先读）**：§7.4 两条都采纳，见 §7.4 开头那一段，它覆盖契约别处与之冲突的说法。要点：R5 多一个子测试 `judge_spawn_failed`（#62、#83 由它杀）；§1.1 E4、E6 的局部变量名冻结为 `judgeRan`、`noDelta`、`settled`。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `28845e0`（代码与 `ab53c0b` 逐字节相同，之后只有文档提交，本文行号仍以 `ab53c0b` 为准；生产代码最后一次改动在 `a2e3d76`，之后只有测试与文档）。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`。本机没有 gcc，不能 `-race`。
- 来源：`docs/00_DECISIONS.md` #45 第二节「简洁输出与自动总结」的第 1-4 条（`:2121-2126`：Griffin 要什么、主代理定的档位与格式只有一份、简洁模式照样上屏的清单、用时）。第 5-6 条（`:2127-2129`，自动总结 SM）**不在本刀**，本刀只给它留位置（§1.1 E7）。`:2130`（codex 接入）无关。
- 方向（Griffin 已裁，不许改）：REPL 加输出档位，默认简洁；`-verbose` 启动参数或 `/verbose` 切回今天的详细输出；零值是详细；格式只有一份；成员名用配置里的 `display_name`（空则 id），只改屏幕、不改 prompt；回复仍按完成先后上屏；金额整类不显示；成员名后带子进程用时，最后一行总用时。
- 本契约的全部期望值（屏幕逐字节、TUI 真 Program 上的日志、app.Run 的输出）都已在隔离拷贝 `E:\Isolating_test\council\co1_draft\` 里用一份原型实现（§1.1 全部生产改动 + §1.2 授权的那一处既有测试改动 + §2 全部用例）实跑得出：原型下 `gofmt -l` 为空、`go vet ./internal/repl ./internal/tui ./internal/app ./cmd/council` 无报告、全量 `go test -count=1 ./...` 八包全绿；§4 的变异逐条在原型上跑过（锚点恰好一次、从字节副本还原）。原型不交付，已删除。
- v1.1 的新增与改动（见下节）在另一份原型 `E:\Isolating_test\council\co1_v11\` 上实跑得出：`git archive 28845e0` 解出，§1.1 A-I 与 §1.2 那一处逐字落盘（**§1.1 一个字没改**），§2 的 v1 用例照写。落 §1.1 之前先在纯 HEAD 上单放 `cox_record_verbose_test.go` 跑 K2，8 个子测试全绿；落完之后 v1 的六个文件全绿；再加 v1.1 的用例。原型下 `gofmt -l` 为空、`go vet ./internal/repl ./internal/tui ./internal/app ./cmd/council` 无报告、全量 `go test -count=1 ./...` 九包全绿（v1.1 起 `cmd/council` 有了测试文件）；v1.1 新增用例 `-count=10` 零偶发。§4 新增的变异 #56-#76 与改写后的 #29 逐条在原型上跑过（锚点恰好一次、从字节副本还原并比对字节相等），点名杀手实测变红，点名「不该红」的对照实测保持绿。原型不交付，已删除。
- W1b 修订（见下节末尾的「W1b 修订」小节）在第三份原型 `E:\Isolating_test\council\co1_fix\` 上实跑得出：`git archive 28845e0` 解出；§1.1 A-I 与 §1.2 那一处用锚点脚本落盘（A、D1、E2、E4、E6 的 no_delta 块与结算块、E8、G1、F1 那一行、I 那一行、C 那一行直接从本文件截取，其余按条文写；每个锚点先核恰好一次；**§1.1 一个字没改**）；`gofmt -l` 为空、`go vet ./internal/repl ./internal/tui ./internal/app ./cmd/council` 无报告、这四个包 `go test -count=1` 全绿。忠实性先验：v1.1 已冻结的 R1、R5 五个子测试、R6、R11 `canceled` / `canceled_judge` / `persist_fail` 照本文重写，整串逐字全绿，之后才取新期望。新增的 R11 `canceled_line`、`canceled_judge_line`、`round_report_err`、L1 第三条与 §7.4 待裁的 `judge_spawn_failed` 各 `-count=15` 零偶发；§4 新增的 #77-#83 与重跑的 #62、#76 逐条实跑（锚点恰好一次、从字节副本还原并比对字节相等），点名杀手实测变红、点名对照实测保持绿。原型不交付，已删除。

## v1.1 变更记录（W1 的 F0-F6 与主代理裁决，2026-09-28）

W1：审查员照 v1 忠实实现，§2 全部期望第一次就过，55 条变异全被点名杀手杀掉；自拟变异里 4 条 major（夹具走不到变异所在的分支）、3 条 minor。证伪者逐条复核 F0-F3，全部成立，并指出其中几条修法的漏洞；主代理裁决全部接受、凡证伪者更正处按更正做。**§1.1 冻结的生产代码一个字没改**（四条 major 全是测试侧的缺口），所以本节没有「§1.1 改动」一项。

- **F0（major，TUI 分支的 `SetConcise` 能测）** → §2.4 A1 加 `tui`、`tui_verbose` 两个子测试（两段式：行式 `Run` 建会话，TUI 模式 `Run` 经 `os.Pipe` 进同一个会话跑 `/round c1 --go`；每一步一个可观测同步点，`/use` 的回显已核实两档都打；`In` / `Out` 留 nil、`Err` 进 `Fatalf`；原型 `-count=10` 零偶发）。「`tui.Run` 要真终端、测不到」的错话三处一起改：§2.4 A1 的【判别力】、§4「只在审查时看」第 1 条（删去）、§6 第 5 步之 6 的括号。§4 新增 #56（不调，X1）、#57（写反，X2）、#58（写死 `true`）。
- **F1（major，no_delta 行只测了第一轮）** → §2.2 R5 加 `round2`（`cox_a, cox_x`，慢 a 前置，`--rounds 2`）与 `judge_nodelta`（只有 `cox_j`，先问 j，`--go --with-judge`），整串逐字、时钟 2 / 1 次；R5 的表加「前置」一列。慢 a 固定了第 1 轮的完成顺序，所以 `round2` 能整串相等，V1 要的「从上一个匹配处往后找、no_delta 行恰好一次、第 2 轮失败行之后还有『分不清』那一行」都被整串蕴含。通用纪律加一条只用于 `round2` 的规范化（原因行）。§4 新增 #59（X7）、#60（X12）、#61（「分不清」那一行只在第 1 轮）。**裁决里「『分不清』那一行只在发言轮打，杀手是 round2」做不到**：`round2` 两段都是发言轮，这条变异（#62）在原型上实测全部 cox 用例都绿；裁判轮的这道门要一个走 grok 的裁判才走得到，按裁决本刀不造，#62 列进「没有用例能抓」并登记 §7 open items。
- **F2（major，`spawned` 没看 `judgeRan`）** → §2.2 R4 拆成 `plain`（v1 原样）与 `with_judge`（预览 `/round cox_a cox_j --with-judge`，两张预算表整串逐字；没有「发言顺序」、没有「裁判 … 本命令不问」、没有「总用时」；时钟恰好 1 次）。§4 新增 #63（X13，`judgeRan || true`）、#64（X8，`judgeRan || !goRun`）、#65（`judgeRan || goRun`，杀手是 R5 `judge_nodelta`，R4 与 R5 两处都写明这个依赖）。
- **F3（major，必须照打的行没有简洁档夹具）** → 新增 §2.2a `internal/repl/cox_keep_test.go`（R11 `TestCoxConciseKeepsLines`）：`canceled`（观察者在 RoundEnd 取消 ctx，`--rounds 2`，整串逐字，慢 a 固定顺序）、`canceled_judge`（同一句的第二个打印点 `round.go:223`；裁决只点了一处，按「同类的还有没有别处」补上）、`persist_fail`（前缀 `发言失败: ` 且恰好一行）、`refusals`（六个打印点 `:82`、`:86`、`:95`、`:100`、`:126`、`:141` 各一个子子测试，整行）、`argv_refusal`（4 万字符的 Model，整行、字节数规范化成 N）。§4 新增 #66-#75（X5 两处、X11、六个拒绝点各一条、X6），杀手写到子测试名。§4「只在审查时看」原第 3 条删去；通用纪律加 `argv_refusal` 的规范化。记录型观察者多一个可选回调（R3 不设，行为不变）。
- **F4（minor，#29 照字面写编译不过）** → §4 #29 写全替换前后的文本，杀手按 v1.1 原型实测补上 A1 `default`。§4 开头说明变异行的写法（`\t`、`\n`、表格里的 `\|`），v1.1 新增的每一行都写全替换前后的文本。
- **F5（minor，`-verbose` 是否登记能单测）** → 新增 §2.5 `cmd/council/cox_flag_test.go`（L1 `TestCoxVerboseFlagRegistered`：`run(-verbose -version)` 为 `exitOK`，另加一条未登记 flag 走 `exitUsage` 的对照）。§4 新增 #76（flag 改名）。「只在审查时看」第 2 条收窄到 `Verbose: *verbose` 这一处传递。
- **F6（minor，范围写「每条命令一行总用时」，契约只在部分命令后打）** → 主代理裁：`总用时` 只在 `/round` 真起过进程时打；预览、没人可问、被拒都不打；`/ask` 靠成员头里的用时；与 TUI 收尾行的门一致。写进 §0 第 8 条与 §7 open items，标「主代理定，#45 原话是每条命令一行」。代码与用例不变（R4 `plain` / `with_judge`、R5 `only` / `judge_nodelta`、R7、R11 各子测试已经钉着这些情形不打）。
- **待 Griffin 四条**（裁决原文）→ 登记 §7.3，本刀照 v1 做，不改。
- 其余连带的改动：标题与仓库行改成 v1.1、HEAD `28845e0`；通用纪律的文件数五 → 七（W1b 修订更正：原写「六 → 八」，v1 本来就多数了一个）、允许引用的辅助补 app 与 cmd/council 两条、「HEAD 上的状态」补两个新文件；§4 开头补 v1.1 的实跑说明；§6 第 2 步文件数与包列表、第 3 步变异范围、第 4 步 #45 的取舍清单；新增 §7。

**v1.1 新增或改动的用例一览**：A1 `tui`、A1 `tui_verbose`（新）；R4 拆成 `plain`（原 R4）与 `with_judge`（新）；R5 `round2`、`judge_nodelta`（新，R5 的表加「前置」一列）；R11 `TestCoxConciseKeepsLines` 整个文件（新：`canceled`、`canceled_judge`、`persist_fail`、`refusals` 的六个子子测试、`argv_refusal`）；L1 `TestCoxVerboseFlagRegistered`（新文件）。**v1.1 新增或改写的变异**：#29（改写）、#56-#76（#62 没有杀手）。W1b 修订再加：R11 `canceled_line`、`canceled_judge_line`、`round_report_err`（新子测试）；L1 第三条断言（`-h` 的用法）；变异 #77-#83（#83 与 #62 一样没有杀手，补不补待主代理，§7.4）。

**给 SM1 修订人的清单**（裁决要求；W1b 修订按 SM1 §1.1 的判据顺序改写）：SM1 §1.1 在 `cmdRound` 走完发言轮与（若有）裁判轮、没有提前 `return` 之后按顺序判：第 1 条，带了 `--no-summary` 就一行都不打；第 2 条，本条命令真正送进 `RunRoundPreparedStream` 的成员（发言轮各段与裁判轮，按 `AgentID` 去重）少于 2 个就一行都不打；第 3 条，取消了就一行都不打；第 4 条，没配总结成员就在简洁档打一行提示（详细档不打）。「这些成员里没有一个落了库的发言」是第 5 条，排在提示行**之后**，所以**有没有人回复不影响提示行**（v1.1 原稿的判据多写了「有人回复」，W1b 指出，已删）。CO1 的夹具都没有 `[summary]`，于是「简洁档 + 走完没提前 return + 没带 `--no-summary` + 送进去去重 ≥2 个 + 没取消」的 `/round` 输出会在 `总用时` 之前多一行提示。v1 里受影响的 R1、R3 `concise`、R6、T2 `concise` **保持 v1 的形状**（R3 仍从 R1 的常量派生，没有重构）。v1.1 与 W1b 修订新增的用例逐条过这几条判据：
- R5 `round2`：简洁；第 1 轮送进 `cox_a`、`cox_x` 两个（`cox_x` 没起来，但送进去了）；走完、没取消 → **中，SM1 落地后这一条的整串会多一行**（位置在最后一行 `总用时 7.2s` 之前）。
- R11 `canceled`：简洁、送进两个，但**取消了**（观察者在第 1 段 RoundEnd 取消 ctx，`round.go:192` 提前 return，末行是「已跑 1/2 轮，其余取消」），且装了观察者 → 不中；SM1 若决定取消之后也总结，这一条要重看。
- R11 `canceled_judge`：只送进一个（`cox_a`），且取消 → 不中。
- R11 `canceled_line`（W1b 修订）：送进两个，但取消（`:192` 提前 return）→ 不中，`总用时 7.2s` 之前不多行；SM1 若决定取消之后也总结，这一条同样要重看。R11 `canceled_judge_line`（W1b 修订）：只送进一个，且取消 → 不中。
- R11 `round_report_err`（W1b 修订）：`round.go:105` 提前 return，一个都没送 → 不中。
- A1 `tui`：简洁，只送进一个（`c1`）→ 不中。A1 `tui_verbose`：详细 → 不中。
- R4 `with_judge`（预览）、R5 `judge_nodelta`（没人可问）、R11 `refusals`（六条拒绝）、R11 `argv_refusal`（整轮拒绝）：一个都没送进去 → 不中。R11 `persist_fail`（`/say`）、L1（`-version`、`-h`）：不是 `/round` → 不中。
- §7.4 第 1 条待裁的 `judge_spawn_failed`（W1b 修订备好，**没有**进 §2）：若采纳，送进 `cox_a`（发言轮）与 `cox_y`（裁判轮，没起来）两个，走完、没取消 → **中**，提示行在 `总用时 7.2s` 之前。

### W1b 修订（第二轮审查的 3 条 major、4 条 minor，2026-09-28）

W1b：照 v1.1 忠实实现，§2 全部期望第一次就过；#29 与 #56-#76 逐条实跑，点名杀手都红、点名对照都绿；自拟变异里 3 条 major（新夹具只走到变异所在的一个取值，或收窄之后留出空档）、4 条 minor。本修订逐条处理如下。**§1.1 冻结的生产代码一个字没改**（修订前后 §1.1、§1.2 两段逐字节比对相同），所以仍然没有「§1.1 改动」一节。凡是要推翻裁决书或动 §1.1 文字的，取保守读法：证据在原型上跑齐，登记 §7.4「待主代理」，不进 §2 与 §4 的杀手列。

- **major 1（行式取消路径没有夹具）** → 照改。§2.2a R11 加 `canceled_line`：与 `canceled` 同一装置、同一前置、同一命令，只差不装观察者，取消改由带锁的取消写者在 `甲（` 写出之后触发。按「同类的还有没有别处」再加 `canceled_judge_line`（`:223` 那一处的行式镜像）。两条都是整串逐字、时钟 2 次，末行 `总用时 7.2s`。起过进程之后被取消照打总用时，是 §1.1 E2 四个判据本来的推论，不是新行为；它与 TUI 取消时的收尾行 `/round 已取消  …` 对应。§0 第 8 条与 §7.1 各补半句写明。§4 新增 #77（defer 加 `|| ctx.Err() != nil`）、#78（`:192` 只在装了观察者时打）、#79（`:223` 同型）。
- **major 2（`-verbose` 的默认值与说明文字没有用例）** → 照改。§2.5 L1 加第三条断言：换掉 `os.Stderr` 调 `run([]string{"-h"})`，用法里含 `-verbose` 那一整段（两行，末尾是 `切换）\n`；默认改成 true 时末尾会多出 ` (default true)`）。§4 新增 #81（默认值改成 true）、#82（说明文字改一个字）；#76 的杀手写明是第一条断言。「只在审查时看」里 `Verbose: *verbose` 那一条保持收窄。
- **major 3（#62 其实不用花钱就能杀）** → **待主代理**，见 §7.4 第 1 条。W1b 的事实成立：一个命令路径不存在的 grok 裁判（同 `cox_x` 的做法）走得到裁判轮的「分不清」那一行，不起任何真 CLI，原型 0.07 秒。但裁决书 F1 的原话是「本刀不造」，补进来就是推翻裁决，本修订不自己定。夹具、整串期望（原型实跑）、#62 在它下面变红的证据都写进 §7.4，采纳即可原样落进 R5。同一个夹具还是裁判轮 `/round <id>:` 原因行的唯一夹具：把那一行在简洁档门成只在发言轮打（新登记 #83），原型上实跑的 R1、R5、R6、R11 都绿，只有这个夹具红（其余用例的裁判轮没有失败的成员，按代码走不到这一行，这一条是推理不是实跑）。#83 与 #62 一样登记为没有杀手，一并待裁。
- **minor 1（#59-#65 的锚点用了「实现方定」的局部变量名）** → §4 开头加一句：锚点里的 `noDelta`、`settled`、`judgeRan` 是原型取的名字，集成员按真文件里的名字改写锚点（只换名字，其余逐字，照旧先核恰好一次）。把这三个名字改成冻结要动 §1.1 E4、E6 的文字，按裁决书的硬约束不自己改，登记 §7.4 第 2 条待主代理。
- **minor 2（新文件是 7 个，不是 8 个）** → 照改。`grep -o 'cox_[a-z_]*_test\.go' | sort -u` 数出 7 个不同的文件名。通用纪律「共八个」「v1 是六个」改成「共七个」「v1 是五个」，「其余六个」改成「其余五个」，§6 第 2 步「8 个文件」改成「7 个文件」，上面 F 记录里的「六 → 八」改成「五 → 七」。
- **minor 3（`reportErr("round")` 的六个打印点没有简洁档夹具）** → 两件一起做。R11 加 `round_report_err`：已取消的 ctx 下 `/round cox_a --go`，走 `round.go:105`，断言前缀 `round: `、恰好一行、时钟 1 次；§4 新增 #80。其余五处（`:131`、`:148`、`:199`、`:227`、`:262`）用取消走不到：`:199`、`:227`、`:262` 前面都有一道 `ctx.Err()` 检查（`:191` 或 `:222`），取消先在那里变成「已跑 k/N」；`:131`、`:148` 与它前面那次成功的 `Participants` 调用之间没有打印点、也没有事件，取消的时刻卡不进去。要往存储层注错才走得到，本刀不造。R11 引言的「同一句话有几个打印点就有几个子测试」因此改成「每一类至少一个打印点；同一句话的多个打印点，取消走得到的逐个走到」，这五处进 §4「只在审查时看」与 §7.2。
- **minor 4（清单判据与 SM1 §1.1 不符）** → 照改。上面「给 SM1 修订人的清单」按 SM1 §1.1 的顺序重写：去掉「有人回复」，补上「走完、没有提前 return」「没带 `--no-summary`」。清单里也补上本修订新增的三条子测试，以及 §7.4 待裁的那一条。判定结果：v1.1 已有的各条不变；新增三条都不中；待裁那一条若采纳则中。

W1b 返回里另有两处观察，W1b 自己判为不列发现，本修订照此不改：一是 A1 `tui` 的 `/use` 同步点等的是输出，不是 busy 清零（与 `rba_wiring` 同一写法，`-count=40` 零偶发）；二是 L1 在「`-verbose` 被登记成非 bool flag」的错实现下会越过 `-version`（那种写法与 `Verbose: *verbose` 编译不过）。

## 与本批另外两刀的文件相交（逐个点名）

本批另两刀：**MD**（#45 第一节，模型识别后 `/add` 写进 config.toml）与 **SM**（#45 第二节第 5-6 条，自动总结）。两份契约与本契约同时起草，下面按 #45 的裁决预判它们会碰的地方；落地顺序建议 **CO1 → SM**（SM 要挂在本刀冻结的 `cmdRound` 收尾位置上），MD 与本刀之间只有锚点冲突、没有语义依赖，谁先谁后都行，但必须串行落地、后落的那一刀按先落的真文件重取锚点。

| 文件 | 本刀改的函数 / 位置 | 可能相交的另一刀 | 相交点 |
|---|---|---|---|
| `internal/repl/round.go` | `cmdRound`（签名改成命名返回值 `(err error)`、入口取时钟、defer 打总用时、两处记 `spawned`、「发言顺序」加门）；`runOneRound`（预算表、no_delta 行、「扇出期间」、结算四处加门；`RoundEnd` 事件多一个字段）；新 `spawnFailedLine`；`renderSettlement` 改调它 | **SM**（必相交） | SM 要在 `cmdRound` 里解析 `--no-summary`、在最后一段之后调一次总结成员。**总结块必须落在 defer 打的总用时那一行之前**（见 §1.1 E7 的位置冻结）；SM 不许另打总用时；SM 自己起进程时的时间计入总用时（它在 `cmdRound` 里） |
| `internal/repl/commands.go` | 命令表末尾加 `verbose` 一行；`postUserText` 两处加门；`dispatch` 里一段注释补半句 | **MD**（`cmdAdd` 与 `add` 那一行 summary）；**SM**（`round` 那一行 summary 可能加 `--no-summary`） | 同一张命令表字面量里的不同行。本刀只在 `config` 那一行之后**追加**一行，不改任何既有行 |
| `internal/repl/repl.go` | `REPL` 结构体末尾（`roundObs RoundObserver` 之后）加两个字段；import 加 `time` | **MD**（可能给 REPL 加识别执行器字段）；**SM**（可能加总结相关字段） | 同一个锚点（结构体末尾）。后落的一刀按真文件重取锚点 |
| `internal/repl/ask.go` | `writeAskRecord`、`writeAskMeter` | SM 若复用 `writeAskRecord` 打总结成员的回复：简洁档下它会打成「<名>（<用时>）: 正文」，与 #45 要的「总的来看（<名> 做总结）：…」不同，SM 必须自己决定走不走它 | 函数体，不是锚点冲突 |
| `internal/repl/observer.go` | `RoundEndEvent` 末尾加 `Concise bool` | SM 若给总结发事件 | 结构体末尾 |
| `internal/tui/roundobs.go` | `RoundEnd` 方法体；`outcomeWord` / `wallText` / `formatElapsed` 三个函数体改成委托 | 无（按 #45 两刀都不碰 TUI 适配器；若 SM 要在 TUI 里给总结单独的呈现，那是 SM 的事） | — |
| `internal/app/app.go` | `Options` 末尾加 `Verbose bool`；`opts.TUI` 两支在 `repl.New` 之后各加一句 `r.SetConcise(!opts.Verbose)`；行式那一支从链式调用拆开 | **MD**（可能给 REPL 注入识别执行器，同样在 `repl.New` 这两处之后） | `app.go:220-229` 这一段 |
| `cmd/council/main.go` | 加 `-verbose` flag 并传给 `app.Options` | MD / SM 若加启动参数 | `fs.Bool` 那几行 |
| `internal/app/app_test.go`（既有测试，授权一处） | `runAppForTest` 的 `Options` 字面量加一行 `Verbose:    true,` | 无 | — |

**显示名**：本刀的显示名取 `r.cfg.Agents[id].DisplayName`，每次打印时现读 `r.cfg`（不缓存）。MD 在运行时换 `r.cfg` 指针（与今天 `configcmd.go:152` 的 `/config reload --apply` 同一处写法）之后，新成员的显示名自动生效；§2.1 K5 用「换掉 `r.cfg` 指针」钉住这一条。

---

## 0. 病根 / 动机（行号以 HEAD `ab53c0b` 为准）

1. **一轮 /ask 打 5-8 行，Griffin 要看的只有一行。** `writeAskRecord`（`internal/repl/ask.go:319-373`）无条件打 `outcome <label>  agent <id>`（`:320`）、provider 会话行（`:328-336`）、`seq N  <id>  <脑子>`（`:352`），再经 `writeAskMeter`（`:385-451`）打用量（`:399` / `:409`）、金额（`:416` / `:420` / `:424`）、冷暖与 `wall_ms`（`:441`）、意外冷启动（`:446`）。正文夹在中间（`:360`）。#45（`:2121`）：默认只显示「成员名: 回复」。
2. **/round 再加一层。** `cmdRound` 打「发言顺序按字典序…」（`internal/repl/round.go:178`）；`runOneRound` 每段打预算表（`:264`）、「扇出期间 REPL 不接受任何命令…」（`:292`）、结算块（`:375`，`renderSettlement` `:562-585`：确知起进程、分不清进程是否起来、用量、金额、冷启动、意外冷启动）。原型实测：详细档下一条 `/round cox_a cox_b cox_j --go --with-judge` 打 49 行，其中给 Griffin 的正文是 3 行 `OK`。
3. **成员名是 id，没有用时。** 屏幕上只有 `agent <id>`；`wall_ms=N` 埋在冷暖行里（`ask.go:441`）。#45（`:2126`）：「Opus 5.0（6.8s）: …」，最后一行总用时。子进程用时的格式今天只有 TUI 有一份：`wallText`（`internal/tui/roundobs.go:139-148`，不到 60 秒截断到一位小数，否则 `formatElapsed` `:176-188`），`repl` 包引不到它（`tui` 引 `repl`，反过来成环）。
4. **格式只能有一份。** `round.go:308-310` 的注释：「复用的还是 writeAskRecord 这一份格式，不另写一份——两份格式会分叉，而分叉的那一天没有任何测试会红」。所以档位判断必须写在既有打印点内部，不许另写一个 `renderConcise`。
5. **TUI 已经有一行总用时。** RB-B 的收尾行 `"/round %s  %s  回来 %d/%d"`（`internal/tui/model.go:447-456`）只在本次提交收到过带名单的 RoundStart 且无错时打（`runSeen`，`model.go:325`），它是防残影的必要条件（RB-B §0 第 3 条），不能去掉。行式 REPL 今天没有总用时。两种模式都不许出现两行总用时。
6. **TUI 的段终态块在简洁档下是重复信息。** `roundObserver.RoundEnd`（`roundobs.go:34-39`）每段结束时把 `endBlockLines`（`:46-72`）逐行打进回滚区；简洁档下每个成员头已经带了名字、结局与用时。
7. **送给模型的 prompt 里的名字是 id**：`internal/turn/router.go:191` 的 `line := m.SenderID + ": " + m.Text`。本刀不碰它（#45：只改屏幕）。
8. **「总用时」打在哪些命令后面（v1.1，F6；主代理定，#45 原话是「每条命令一行」）**：#45（`:2126`）写的是每条命令结束恰好一行总用时。本契约收窄为：**只在 `/round` 真起过进程（至少一段 `spawned`）时打**；预览、没人可问（全员 no_delta）、被拒（`拒绝: …`、argv 整轮拒绝）都不打；起过进程之后被取消（「已跑 k/N 轮，其余取消」）照打，对应 TUI 取消时的收尾行 `/round 已取消  …`（`model.go:447-456`，`runSeen` 仍为真），这是 §1.1 E2 四个判据本来的推论（v1.1 W1b 修订写明，§2.2a R11 `canceled_line` 钉住）；`/ask` 不打，它的用时就在成员头里。这与 TUI 收尾行的门一致（`runSeen`：本次提交收到过带名单的 RoundStart 才打，`model.go:325`），行式与 TUI 两种模式因此在同一批命令后面各恰好一行。§7 open items 同记。

---

## 1. 范围

### 1.1 做

凡写「名字实现方定」处实现方可自取名字（生产代码新标识符不许以 `cox` 开头）；其余名字、签名、字面量、顺序、判据全部冻结。注释用中文、可以扩写，但必须覆盖每条列出的「注释要点」，**不许写与代码不符的话**（上一批独立审查的 major 就是注释说错 ctx）。

**缩进说明**：本文代码块里的缩进可能被排版成空格。锚点一律以 HEAD 真文件为准（每条都给了行号），逐字复制真文件那几行（含 tab）。

**冻结的名字**（测试腿按这些名字写用例）：`(*REPL).SetConcise(on bool)`、REPL 字段 `concise bool` 与 `now func() time.Time`（白盒用例直接读写）、`repl.FormatWall(ms int) string`、`repl.FormatElapsed(d time.Duration) string`、`repl.OutcomeWord(o turn.Outcome) string`、`repl.RoundEndEvent.Concise`、`app.Options.Verbose`。名字实现方定的：显示名、取时钟、`/verbose` 命令函数、`spawnFailedLine`、三个提示常量。

#### A. 新文件 `internal/repl/verbosity.go`

内容冻结如下（函数体逐字；注释要点见下；常量名实现方定、值逐字）：

```go
package repl

import (
	"context"
	"fmt"
	"time"

	"council/internal/turn"
)

const (
	msgVerboseUsage = "用法: /verbose 在简洁与详细之间切换；/verbose on 切到详细；/verbose off 切到简洁"
	msgVerboseOn    = "已切到详细输出：用量、金额、冷暖、会话 id、预算表与结算全部显示；/verbose off 回到简洁"
	msgConciseOn    = "已切到简洁输出：只显示成员的回复、用时，以及关系到钱或内容丢失的提示；/verbose on 切到详细"
)

func (r *REPL) SetConcise(on bool) {
	r.concise = on
}

func (r *REPL) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *REPL) displayName(id string) string {
	if ac, ok := r.cfg.Agents[id]; ok && ac.DisplayName != "" {
		return ac.DisplayName
	}
	return id
}

func cmdVerbose(_ context.Context, r *REPL, args cmdArgs) error {
	switch {
	case len(args.fields) == 0:
		r.concise = !r.concise
	case len(args.fields) == 1 && args.fields[0] == "on":
		r.concise = false
	case len(args.fields) == 1 && args.fields[0] == "off":
		r.concise = true
	default:
		return r.printf("%s\n", msgVerboseUsage)
	}
	if r.concise {
		return r.printf("%s\n", msgConciseOn)
	}
	return r.printf("%s\n", msgVerboseOn)
}

func FormatWall(ms int) string {
	if ms < 0 {
		ms = 0
	}
	if ms < 60000 {
		return fmt.Sprintf("%d.%ds", ms/1000, (ms%1000)/100)
	}
	return FormatElapsed(time.Duration(ms) * time.Millisecond)
}

func FormatElapsed(d time.Duration) string {
	s := int64(d / time.Second)
	if s < 0 {
		s = 0
	}
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh%02dm", s/3600, (s%3600)/60)
}

func OutcomeWord(o turn.Outcome) string {
	switch o {
	case turn.OutcomeSpoke:
		return "完成"
	case turn.OutcomePostRejected:
		return "未入库"
	case turn.OutcomeProviderFailed:
		return "报错"
	case turn.OutcomeUnparsable:
		return "认不出"
	case turn.OutcomeTimeout:
		return "超时"
	case turn.OutcomeSpawnFailed:
		return "没起来"
	case turn.OutcomeRefused:
		return "拒绝"
	case turn.OutcomeCanceled:
		return "取消"
	case turn.OutcomeLost:
		return "丢失"
	default:
		return "未知"
	}
}
```

注释要点（每个函数上方）：
- 文件头（或 `SetConcise` 上方）：两个档位的定义——详细 = 今天的输出一个字节不改；简洁 = #45 第二节的清单（成员头 + 正文，关系到钱或内容丢失的提示照打，用量 / 金额 / 冷暖 / 会话 id / seq / 预算表（带 `--go` 时）/ 结算不打）。零值是详细，所以直接 `New` 出来的 REPL（既有测试）行为不变；app 装配时按 `Options.Verbose` 设定（默认简洁）。档位判断散在各个既有打印点里（`writeAskRecord`、`writeAskMeter`、`runOneRound`、`cmdRound`、`postUserText`），没有第二套渲染函数，理由是 `round.go` 那段「两份格式会分叉」。
- `SetConcise`：与 `SetRoundObserver` 同一条纪律，只许在没有命令在跑时调用（app 在 `Run` / `tui.Run` 之前调；`/verbose` 本身是一条命令，命令是串行派发的）。`/round` 的回调在 turn 的 worker goroutine 上读 `r.concise`，那时 REPL 主 goroutine 阻塞在 `RunRoundPreparedStream` 里，不会有人写它，所以不加锁。
- `clock`：`now` 为 nil 时用 `time.Now`；只有 `cmdRound` 调它（总用时），测试经 `now` 注入假时钟。
- `displayName`：每次现读 `r.cfg`，不缓存——`/config reload --apply`（`configcmd.go:152`）会换掉 `r.cfg` 指针，缓存会让屏幕上的名字停在旧配置；配置里没有这个 id（成员还在、配置删了）或 `display_name` 为空时回落到 id。只用于屏幕，**不进 prompt**（`internal/turn/router.go:191` 仍用 `SenderID`）。
- `cmdVerbose`：不带参数是切换；`on` / `off` 是设定（只认小写、只认恰好一个参数）；别的一律打用法、不改档位。无前置条件（不需要会话、持久化或 Runner）。
- `FormatWall`：子进程用时的唯一格式。负数当 0；不足一分钟向零截断到十分之一秒（不四舍五入：59999 印 `59.9s`，不是 `60.0s`）；一分钟起交给 `FormatElapsed`。简洁档的成员头、`总用时` 那一行、TUI 段终态块的时间格（`tui.wallText` 在它后面接「 子进程」）共用它。
- `FormatElapsed`：从 `internal/tui/roundobs.go:174-188` 原样搬来（原注释搬过来），TUI 表头、在跑行、收尾行经 `tui.formatElapsed` 委托调用它，所以 TUI 的字节不变。
- `OutcomeWord`：从 `internal/tui/roundobs.go:99-123` 原样搬来；简洁档失败行的「<结局>」与 TUI 看板 / 段终态块的状态词是同一份。

#### B. `internal/repl/repl.go`

- import 块（`:12-23`）加 `"time"`。
- `REPL` 结构体末尾，`roundObs RoundObserver`（`:100`）之后、结构体的 `}`（`:101`）之前，加（名字、类型冻结）：

  ```go
  	// （注释要点见下）
  	concise bool
  	// （注释要点见下）
  	now func() time.Time
  ```
  注释要点：`concise` —— 输出档位，零值 false 是详细（今天的输出）；只经 `SetConcise` 与 `/verbose` 改，读它的打印点见 `verbosity.go` 文件头。`now` —— 总用时用的时钟，nil 时用 `time.Now`，测试注入。

#### C. `internal/repl/observer.go`

- `RoundEndEvent`（`:41-47`）的 `Results` 字段之后加一个字段（名字、类型、位置冻结；字段注释对齐交给 gofmt）：

  ```go
  	Concise bool              // 发出这个事件时 REPL 的输出档位是简洁；零值是详细
  ```
- 结构体上方那句「RoundEndEvent 在本段结算文本打完之后发出。」补半句：简洁档下结算块不打（至多一行「分不清进程是否起来 N（…）」），发射时刻仍在同一位置。
- `RoundObserver` 接口、`RoundStartEvent`、`MemberDoneEvent`、`SetRoundObserver` 一个字不改。

#### D. `internal/repl/ask.go`

D1. **`writeAskRecord`（`:311-373`）整个函数换成下面这样**（逐字；函数注释按下面的注释要点改写，原有注释的内容保留）：

```go
func writeAskRecord(r *REPL, rec turn.Record) error {
	if r.concise {
		name := r.displayName(rec.AgentID)
		if rec.Message != nil {
			head := name + "（" + FormatWall(rec.WallMS) + "）:"
			if rec.Message.Text == "" {
				if err := r.write(head + "\n"); err != nil {
					return err
				}
			} else {
				if err := r.write(head + " "); err != nil {
					return err
				}
				if err := writeFullText(r, rec.Message.Text); err != nil {
					return err
				}
			}
			if note := completionNote(rec); note != "" {
				if err := r.printf("%s\n", note); err != nil {
					return err
				}
			}
		} else if err := r.write(name + ": 没有回复（" + OutcomeWord(rec.Outcome) + "）\n"); err != nil {
			return err
		}
	} else if err := r.printf("outcome %s  agent %s\n", outcomeLabel(rec.Outcome), rec.AgentID); err != nil {
		return err
	}
	if rec.MaybeAdvanced {
		if err := r.printf("provider 那边可能已经吃下这批内容\n"); err != nil {
			return err
		}
	}
	if !r.concise {
		if rec.Fresh {
			if err := r.printf("新开了一条 provider 会话 %s\n", orPlaceholder(rec.ProviderSession)); err != nil {
				return err
			}
		} else if rec.ProviderSession != "" {
			if err := r.printf("provider 会话 %s\n", rec.ProviderSession); err != nil {
				return err
			}
		}
	}
	if rec.Drifted {
		if err := r.printf("送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了\n"); err != nil {
			return err
		}
	}
	// 放在 rec.Message 判断之外：turn_failed / post_rejected 这两条路径也可能
	// 带着非空的 DeniedActions（absorbParse 先于失败判定运行），不能只在
	// 「说成功了话」这一支才把这件事说出来。
	if note := deniedActionsNote(rec); note != "" {
		if err := r.printf("%s\n", note); err != nil {
			return err
		}
	}
	if rec.Message != nil && !r.concise {
		m := rec.Message
		if err := r.printf("seq %d  %s  %s\n", m.Seq, m.SenderID, brainCell(m.SenderProvider, m.SenderModel, m.SenderEffort)); err != nil {
			return err
		}
		if note := completionNote(rec); note != "" {
			if err := r.printf("%s\n", note); err != nil {
				return err
			}
		}
		if err := writeFullText(r, m.Text); err != nil {
			return err
		}
	}
	if rec.Outcome == turn.OutcomePostRejected {
		if err := r.printf("正文写不进库（已经付过钱），整段如下:\n"); err != nil {
			return err
		}
		if err := writeFullText(r, rec.DroppedText); err != nil {
			return err
		}
	}
	return writeAskMeter(r, rec)
}
```

冻结要点：
- **详细档逐字节不变**：`r.concise` 为假时，打印的行、顺序、字节与 HEAD 相同（§2.1 K2 用 8 个 Record 逐字节钉住）。
- **简洁档成员块的顺序**：成员头（有正文时：`<名>（<用时>）: ` 紧接正文原样、正文不以换行结尾时补一个换行；正文为空时：`<名>（<用时>）:` 换行；之后若有 `本轮为空` / `本轮被截断` 在正文**之后**一行）或失败行 `<名>: 没有回复（<结局词>）` → `provider 那边可能已经吃下这批内容`（MaybeAdvanced）→ 漂移行 → 被拒工具调用行 → PostRejected 的「正文写不进库（已经付过钱），整段如下:」加整段正文。**简洁档不打**：`outcome` 行、两种 provider 会话行、`seq` 行、`writeAskMeter` 的全部行。
- **成员头与失败行的判据是 `rec.Message != nil`**（这一轮的发言已落库），不是 `Outcome`。失败行**不带用时**：起进程之前被拒的轮 `WallMS` 是 0，印 `（0.0s）` 是假话。
- 名字经 `r.write` 拼接，不当 `printf` 的格式串（`display_name` 里可能有 `%`）。
- 注释要点（函数注释补一段）：简洁档的成员块长什么样、判据为什么是 `Message != nil`、失败行为什么不带用时、哪几类行在简洁档照打（MaybeAdvanced、漂移、被拒工具调用、PostRejected 整段正文——关系到钱或内容丢失，出处 #45 `:2124`）、哪几类不打；「格式只有一份」：简洁档是这个函数里的分支，不是另一个函数。原注释里 PostRejected 整段打出、MaybeAdvanced 的理由保留。

D2. **`writeAskMeter`（`:375-451`）**：函数体第一句（`switch rec.Outcome {` 之前）加

```go
	if r.concise {
		return nil
	}
```
注释要点（函数注释补一句）：简洁档一行都不打（用量、金额、冷暖、`wall_ms`、意外冷启动；金额整类不显示，#45 `:2125`）；子进程用时改由成员头带出。

D3. `cmdAsk`（`:44-175`）**一个字不改**：「正在问 X ...」（`:164`）两档都打（进度信号，id 不换显示名）；`askCause` 那一行（`:171-173`）两档都打。

#### E. `internal/repl/round.go`

E1. import 块（`:3-11`）加 `"time"`。

E2. **`cmdRound` 的签名与开头**（`:63-64`）：`func cmdRound(ctx context.Context, r *REPL, args cmdArgs) error {` 改成命名返回值，函数体最前面加（逐字）：

```go
func cmdRound(ctx context.Context, r *REPL, args cmdArgs) (err error) {
	start := r.clock()
	spawned := false
	defer func() {
		if err != nil || !spawned || !r.concise || r.roundObs != nil {
			return
		}
		err = r.printf("总用时 %s\n", FormatWall(int(r.clock().Sub(start)/time.Millisecond)))
	}()
	goRun := false
```
（`goRun := false` 是原 `:64`，作锚点用。）函数体里既有的 `parts, err := ...`、`snap0, err := ...`、`judgeSnap, err := ...` 在函数块里会复用这个命名结果（左边各有一个新变量，编译通过，原型实测）；循环体里 `ran, err := runOneRound(...)` 与 `n, err := fmt.Sscanf(...)` 在内层作用域遮蔽，行为不变。

冻结要点：
- `start` 是函数的**第一条语句**；时钟在一条 `/round` 里恰好调用 **2 次**（起进程过至少一段、简洁、行式）或 **1 次**（其余情形）。§2.2 R1、R5 数调用次数。
- 判据四条同时成立才打：返回值 `err == nil`（写屏幕失败时不打、不吞错误）；`spawned`（本条命令至少一段真的起了进程，见 E3、E4）；`r.concise`（详细档不打，今天的行式输出逐字节不变）；`r.roundObs == nil`（装了观察者就是 TUI，总用时由 TUI 的收尾行 `/round 结束  …` 出，见 `model.go:447-456`；两种模式都恰好一行）。
- 用时用 `FormatWall`（不到 60 秒一位小数），不用 `FormatElapsed`。

E3. 发言轮循环里（`:212-215`），`if !ran { break }` 之后、`finished++` 之前加一行 `spawned = true`：

```go
		if !ran {
			break
		}
		spawned = true
		finished++
```

E4. 裁判轮（`:235-236`）`_, err = runOneRound(ctx, r, judgePlan, goRun, "裁判轮", 0, 0, judgeSnap)` / `return err` 两行换成（局部变量名冻结为 `judgeRan`，主代理 2026-09-28 裁，§7.4 第 2 条）：

```go
	judgeRan, err := runOneRound(ctx, r, judgePlan, goRun, "裁判轮", 0, 0, judgeSnap)
	if judgeRan {
		spawned = true
	}
	return err
```
理由：只有裁判轮起了进程（发言者全是 no_delta）时也要打总用时（§2.2 R5 `judge_only`）。

E5. 「发言顺序按字典序…」（`:178-180`）包进 `if !r.concise { ... }`（字面量不动）。

E6. **`runOneRound`**（`:259-395`）四处加门、一处加字段：
- 预算表（`:264-266`）包进 `if !r.concise || !goRun { ... }`：不带 `--go` 的纯预览两档都照印；带 `--go` 时简洁档不印。
- `plan = skipNoDelta(plan, rows)`（`:276`）之后、`if len(plan.Speakers) == 0 {`（`:279`）之前加：

  ```go
  	if r.concise {
  		var noDelta []turn.SkipReason
  		for _, s := range plan.Skipped {
  			if s.Kind == turn.SkipNoDelta {
  				noDelta = append(noDelta, s)
  			}
  		}
  		if text := renderSkipped(noDelta); text != "" {
  			if err := r.printf("%s", text); err != nil {
  				return false, err
  			}
  		}
  	}
  ```
  （局部变量名冻结为 `noDelta`，主代理 2026-09-28 裁，§7.4 第 2 条。）理由：预算表藏掉之后 no_delta 的人没有别处可见（详细档里它在依据列）；只取 `SkipNoDelta` 这一档——`PlanRound` 产不出它，`cmdRound` 开头已经用 `renderSkipped` 打过分类跳过（not_in_config 等），全量再打一遍会重复（§2.2 R5 `mixed` 钉住）。复用 `renderSkipped`，不另写格式。
- 「扇出期间 REPL 不接受任何命令…」（`:292-294`）包进 `if !r.concise { ... }`。
- 「本轮问 %d 个: %s」（`:299-301`）**两档都打**（进度信号，名单保持 id），不改。
- 结算（`:375-377`）换成（局部变量名冻结为 `settled`，主代理 2026-09-28 裁，§7.4 第 2 条）：

  ```go
  	settled := turn.Settle(recs)
  	if r.concise {
  		if settled.SpawnFailed > 0 {
  			if err := r.printf("%s", spawnFailedLine(settled.SpawnFailed)); err != nil {
  				return false, err
  			}
  		}
  	} else {
  		if err := r.printf("%s", renderSettlement(settled, forecastCold, unexpected, label)); err != nil {
  			return false, err
  		}
  	}
  ```
- `RoundEnd` 字面量（`:392`）加一个字段：`obs.RoundEnd(RoundEndEvent{Label: label, K: k, N: n, Results: results, Concise: r.concise})`。
- 回调里 `writeAskRecord` 与 `"/round %s: %s\n"` 那一行（`:329-342`）不改：简洁档下后者照打（失败原因）。
- `runOneRound` 函数注释补：简洁档下预算表（带 `--go`）、「扇出期间」、结算块不打，分不清进程是否起来 N（N>0）与 no_delta 跳过另起一行；RoundEnd 带 `Concise`，TUI 据此不打段终态块；「屏幕文本与不观察时逐字节相同」这句仍成立（总用时在 `cmdRound`，不在本函数）。

E7. **给 SM 留的位置（冻结，本刀不实现）**：`cmdRound` 的 defer 在函数返回时才打总用时，所以任何在 `cmdRound` 里、`return` 之前打出的行都在总用时之前。SM 的总结块放在最后一段（裁判轮或最后一个发言轮）之后、`cmdRound` 返回之前，屏幕顺序因此是「各段成员块 → 总结 → 总用时」；TUI 里收尾行在 Submit 返回之后才由 Model 打，自然在总结之后。SM 不许另打一行总用时；SM 的总结调用若起了进程，它的时间计入总用时。`cmdRound` 函数注释补一段写明这个位置（措辞实现方定，不许写与代码不符的话）；同时写明总用时的四个判据与「装了观察者就由 TUI 收尾行出」。

E8. **`renderSettlement`**（`:562-585`）里「分不清进程是否起来」那五行（`:566-570`：`Fprintf`、`if`、`WriteString(括号说明)`、`}`、`WriteString("\n")`）换成一句 `b.WriteString(spawnFailedLine(s.SpawnFailed))`，并在 `settledTokens`（`:587`）之前加（名字实现方定）：

```go
func spawnFailedLine(n int) string {
	line := fmt.Sprintf("分不清进程是否起来 %d", n)
	if n > 0 {
		line += "（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）"
	}
	return line + "\n"
}
```
注释要点：详细档的结算块与简洁档那一行共用它，格式只有一份；`renderSettlement` 的输出逐字节不变。

E9. `cmdRound` 里其余打印点**不加门**（两档都打）：`拒绝: ...` 各句（`:82`、`:86`、`:95`、`:100`、`:126`、`:141`）、`renderSkipped`（`:158-162`）、`renderUnaskedJudges`（`:169-175`）、「已跑 %d/%d 轮，其余取消」（`:192`、`:223`）、`reportErr`；`runOneRound` 里 `argvRefusal`（`:270-275`）、「这一轮没有人可问。」（`:280`）。

#### F. `internal/repl/commands.go`

F1. 命令表（`:131-162`）：`{name: "config", ...}`（`:155`）之后、`}`（`:156`）之前追加一行（字面量冻结，命令函数名实现方定）：

```go
		{name: "verbose", summary: "切换输出档位: /verbose 在简洁与详细之间切换；/verbose on 详细；/verbose off 简洁", run: cmdVerbose},
```
无 `needsSvc` / `needsConv` / `needsRunner`。

F2. `postUserText`（`:327-382`）：
- 「已记下 seq %d」（`:338-340`）包进 `if !r.concise { ... }`。
- 「点名了 %s（用 /ask %s 让他说话）」（`:371-376` 那一支）：简洁档跳过这一行（`continue`），详细档不变。**「%s 不是当前会话的成员」（`:377`）两档都打**；「只有 %s 会看到这条；…」（`:345`）两档都打（收窄不可逆，要当场说）。
- 函数注释补一句：简洁档下成功落库不打「已记下 seq N」、不打「点名了」；两条警告（不是成员、定向收窄）照打。

F3. `dispatch` 里 `:208-213` 那段注释（「屏幕上打的是「已记下 seq N」这种正常成功行，扫一眼发现不了」）补半句：简洁档下这一行也不打，更看不出来。只改注释。

#### G. `internal/tui/roundobs.go`

G1. `RoundEnd`（`:34-39`）逐字换成：

```go
func (o roundObserver) RoundEnd(ev repl.RoundEndEvent) {
	if !ev.Concise {
		for _, line := range endBlockLines(ev) {
			o.send(printLine(line))
		}
	}
	o.send(roundEndMsg{ev: ev})
}
```
`roundEndMsg` 两档都发（Model 靠它，RB-B 的看板与收尾行照旧）。`RoundStart`、`MemberDone`、`endBlockLines` 一个字不改。注释要点：TUI 从事件上的 `Concise` 知道档位（不读任何全局变量、不回调 REPL，适配器仍无状态）；简洁档下每个成员头已经带了名字、结局与用时，段终态块是重复的；活区看板、表头、收尾行不受档位影响；详细档与 RB-B 逐字节相同。

G2. 三个函数体换成委托（签名、名字不变；RB-A 的 `rba_header_test.go:101` 直接调 `formatElapsed`，RB-B 的 K3 / K4 经它们断言，所以名字必须留着）：
- `outcomeWord`（`:99-123`）：`return repl.OutcomeWord(o)`
- `wallText`（`:139-148`）：`return repl.FormatWall(ms) + " 子进程"`
- `formatElapsed`（`:174-188`）：`return repl.FormatElapsed(d)`

  三个函数的注释改成「委托 repl.X；格式只有一份，简洁档成员头与此共用」，原来描述格式的注释搬到 `verbosity.go`。改完后 `roundobs.go` 若有 import 变成未使用（`fmt` 仍被 `segmentCell` 用，`time` 仍被 `elapsedTickMsg` 用），按编译器要求处理，不许留死 import。

#### H. `internal/app/app.go`

- `Options`（`:33-49`）末尾、`OutFile *os.File`（`:48`）之后加字段（名字、类型冻结）：

  ```go
  	// （注释要点见下）
  	Verbose bool
  ```
  注释要点：为 true 时 REPL 用详细档（今天的输出）；零值是简洁，所以 `app.Run` 的默认是简洁（#45）。由 cmd/council 的 `-verbose` 设。测试助手 `runAppForTest` 设为 true（它的既有用例都按详细档标定）。
- TUI 那一支（`:224`）`r := repl.New(cfg, svc, rn, in, lw, rulesCtx)` 之后加一行 `r.SetConcise(!opts.Verbose)`。
- 行式那一支（`:227-228`）：注释「// 行式路径一个字不改。」与 `err = repl.New(cfg, svc, rn, in, out, rulesCtx).Run(ctx)` 换成

  ```go
  		// （注释要点见下）
  		r := repl.New(cfg, svc, rn, in, out, rulesCtx)
  		r.SetConcise(!opts.Verbose)
  		err = r.Run(ctx)
  ```
  注释要点：两条路都在 `Run` 之前设档位；那句「一个字不改」已不成立，删掉，不许留下。
- 横幅、对账通知、漂移通知（`writeBanner` / `writeLostNotice` / `writeDriftNotice`）一个字不改。

#### I. `cmd/council/main.go`

- `noTUI := fs.Bool(...)`（`:43`）之前加（变量名实现方定；flag 名与说明文字冻结）：

  ```go
  	verbose := fs.Bool("verbose", false, "详细输出：每轮的用量、金额、冷暖、provider 会话、预算表与结算全部显示（默认简洁；进去之后也可以用 /verbose 切换）")
  ```
- `app.Options{...}`（`:95-104`）末尾加 `Verbose: *verbose,`（对齐交给 gofmt）。

### 1.2 不做（两条腿都不许越界）

- **不做自动总结（SM）**：不解析 `--no-summary`、不加总结成员、不改 `cmdRound` 的轮次逻辑；只按 §1.1 E7 留位置。
- **不改 prompt / Render**：`internal/turn` 一个文件都不碰（`router.go:191` 的 `SenderID + ": "` 原样）；显示名只进屏幕。
- 不改 `/log`、`/cost`、`/session`、`/who`、`/agents` 的列；不改启动横幅与对账 / 漂移通知；不改 `renderBudget`、`renderSkipped`、`renderUnaskedJudges`、`argvRefusal`、`skipNoDelta`、`askCause`、`completionNote`、`deniedActionsNote`、`outcomeLabel` 的签名与输出。
- 不改 `internal/tui` 的 `model.go`、`run.go`、`writer.go`（收尾行、看板、表头照旧）；不改 `endBlockLines`；不改 `repl.New` 的签名（加参数会动几十处既有调用）；不改 `newRoundObserver` 的签名。
- 不改 `go.mod` / `go.sum`、`internal/config`、`internal/store`、`internal/conversation`、`internal/provider`。
- 「正在问 X ...」「本轮问 N 个: ...」保留 id，不换显示名（进度信号，与用户敲的 id 对得上）。
- 详细档行式模式**不加**总用时（今天的输出逐字节不变；RB-A 的 golden `TestRbaR1NilObserverGolden` 钉着它）。
- **既有 `_test.go` 只授权一处**：`internal/app/app_test.go` 的 `runAppForTest`（`:89-110`），`Options` 字面量（`:93-98`）在 `Err:        &errOut,` 之后加一行 `Verbose:    true,`（gofmt 对齐后的字节就是这样：`Verbose:` 后 4 个空格）。**由实现腿（grok）交付**，与生产改动同一份补丁。
  理由：app 默认简洁之后，原型实测**恰好一条**既有用例变红——`internal/app/s10a_settle_fail_blocks_ask_test.go:TestS10aSettleFailureBlocksAskUntilSettled`，两个子测试都红：`settle_fails` 找不到整行 `outcome refused  agent c1`（简洁档打的是 `C1: 没有回复（拒绝）`），`settle_ok` 找不到整行 `outcome spoke  agent c1`（简洁档打的是 `C1（0.0s）: OK`）。`smoke_test.go`、`s10c2`、`s10d`、`drift`、`persist`、`report`、`step10a4`、`x39f` 只查库或只查横幅，两档都绿。改 `runAppForTest` 一处而不改 s10a 用例：这个助手的全部使用方都是按详细档标定的，一行把它们整体钉回详细档，判别力与改动前相同；简洁档由 §2.4 的新用例直接调 `Run` 覆盖。改后这个文件在 HEAD 上编译不过（HEAD 没有 `Options.Verbose`），这是预期的：它与生产改动同一个提交落地。
- 其余既有 `_test.go` 一个字节都不许改（含注释）。原型实测：只做上面那一处，全量既有用例全绿（含 RB-A、RB-B、TUI-1b/1c/1d、X-39e、S10A/C/D 全部）。
- 测试腿不写生产代码、不改任何既有 `_test.go`；实现腿除上面那一处外不写 `_test.go`。

---

## 2. 验收（测试腿新写的文件）

通用纪律：

- 新文件共七个（§2.1 两个、§2.2、§2.2a、§2.3、§2.4、§2.5；repl 包四个、tui 包一个、app 包一个、cmd/council 一个；v1 是五个，v1.1 加 §2.2a 与 §2.5；W1b 修订更正了原来的「八个」「六个」）；包级标识符（类型、函数、变量、常量）一律以 `cox` 开头，测试函数名 `TestCox...`。**不新写 `TestMain`**：repl 包复用 `session_inflight_test.go:94` 的（假 claude CLI：argv 带 `--resume` 时睡 700ms 再应答，回放 `fakeClaudeSuccessJSON`，正文 `OK`），tui 包复用 `rba_fakecli_test.go` 的，app 包复用 `smoke_test.go:40` 的；cmd/council 包没有 `TestMain`，§2.5 不起进程，也不需要。
- 不起真 provider：外部命令一律是测试二进制自己（`COUNCIL_FAKE_CLI=1`、`Command` 写 `os.Args[0]`），或一个不存在的路径（造 spawn_failed）。store 一律 `t.TempDir()` 之下，不碰 `C:\Users\25142\.council\data`。
- **至少一条走真 `app.Run` + 真 store**（§2.4）；repl 包的端到端用例用真 SQLite（`newRoundStreamService`）。
- 断言整行相等、整串相等或整行正则（`^…$`，只用于 TUI 真 Program 那条里随机器变的时间）；子进程用时随机器变，只允许经下面**冻结的规范化**抹掉：成员头里的 `（\d+\.\ds）` 换成 `（T）`；app 包里整行 `^总用时 \d+\.\ds$` 换成 `总用时 T`；repl 包预算行的命令行余量 `(?m)^(cox_[a-z]  未标定  未标定  预报\S+  不在飞  )-?\d+(  无样本)$` 换成 `${1}H${2}`。v1.1 加两条，各只用于一个子测试：R5 `round2` 里 spawn_failed 的原因行 `(?m)^(/round cox_x: ).+$` 换成 `${1}<原因>`（原因文本含临时目录路径与随机 planned_session id；换之前先守卫恰好 2 行）；R11 `argv_refusal` 里 `超了 \d+ 字节` 换成 `超了 N 字节`（字节数随测试二进制的路径长度变，原型实测 7507）。前提守卫（不成立即 `Fatalf`、不是被测断言）可以用前缀，例如 spawn_failed 的原因行。A1 `tui` / `tui_verbose` 是终端字节流，另有冻结的比对形状（§2.4）。
- 每条注释写【判别力】：它能不能为假、只差一处的对照是哪条。
- 允许只读引用同包既有测试辅助（不许改它们）：
  - repl：`newRoundStreamService`（`step10ux1_round_stream_test.go:94`，真 SQLite）、`rbaBuf`（`rba_golden_test.go:36`，带锁、有 `Reset` / `String`）、`TestMain` 假 CLI。
  - tui：`rbaNewTuiRig`（`rba_fakecli_test.go:119`，字段 `r *repl.REPL`、`lw`、`convID`；三个成员 `rba_a`、`rba_b`、`rba_j` 的 `DisplayName` 都是 `"X"`）、`rbbStartProg`、`(*rbbProg).submit` / `useConv` / `waitFor`、`(*rbbSpy).entries`、`rbbEntry`（字段 `kind`、`text`）、`rbbDoneThenFin`、`rbbWindow`、`rbbEventKinds`、`rbbLogDump`（`rbb_order_test.go`）、`rbbBody`（`rbb_block_test.go:31`）、`newRoundObserver`、`roundEndMsg`。
  - app：`isolateAppEnv`、`tomlLiteral`、`writeTestConfig`（`app_test.go`）、`TestMain` 假 CLI。**不用 `runAppForTest`**（本刀落地后它固定 `Verbose: true`）。tui 包的管道辅助（`tui1dBuf`、`tui1dWaitFor`、`tui1bStripANSI`）在另一个包里引不到，A1 `tui` 自己写 cox 前缀的带锁缓冲、轮询与剥 ANSI（剥 ANSI 用 `github.com/charmbracelet/x/ansi` 的 `Strip`，go.mod 已有，不改）。
  - cmd/council：`run`、`exitOK`、`exitUsage`（`main.go`）。
- **HEAD 上的状态**：`internal/repl/cox_record_verbose_test.go`（§2.1 的 K2 与它的夹具）**只用 HEAD 已有的标识符**，测试腿交付前必须在 HEAD `ab53c0b` 的隔离拷贝里**只放这一个新文件**跑一次 `go test -count=1 -run TestCoxVerboseRecordUnchanged ./internal/repl`，报告命令与结果——必须 8 个子测试全绿（原型实测：在 HEAD 拷贝上全绿，证明 K2 的期望就是今天的输出，不是写出来的）。其余五个 repl / tui / app 文件引用本刀新增的标识符（`SetConcise`、`FormatWall`、`OutcomeWord`、`RoundEndEvent.Concise`、`Options.Verbose`、REPL 字段 `concise` / `now`），在 HEAD 上整包编译不过，是预期的红；报告里写出每个文件引用了哪些新标识符（v1.1 新增的 `cox_keep_test.go` 引用 `SetConcise` 与 REPL 字段 `now`，另复用 `cox_round_test.go` 的装置）。`cmd/council/cox_flag_test.go`（§2.5）不引用任何新 Go 标识符，在 HEAD 上能编译，但 `-verbose` 未登记，是运行期的红（第一条断言）；报告里写明。

### 2.1 `internal/repl/cox_record_verbose_test.go` 与 `internal/repl/cox_record_test.go`（单测，直接调 `writeAskRecord`，不起进程）

两个文件的分工（冻结）：`cox_record_verbose_test.go` 放下面的配置、REPL 构造辅助、8 个 Record 的夹具（表驱动，每项带名字、Record、简洁期望、详细期望）与 **K2**，只用 HEAD 已有的标识符（不调 `SetConcise`，不读 `concise` 字段）。其中的 REPL 构造辅助**不带档位参数**，只做 `New(cfg, nil, nil, nil, &b, RulesContext{})` 并返回 `r` 与 `&b`（于是它在 HEAD 上可编译，得到的是详细档）；`cox_record_test.go` 放 K1、K3、K4、K5，引用前一个文件的夹具与构造辅助，**构造之后自己调 `r.SetConcise(true)`**。

公共装置：配置

```go
&config.Config{Agents: map[string]config.AgentConfig{
	"cox_a": {DisplayName: "甲 A", Provider: "claude", Model: "m", Effort: "high"},
	"cox_b": {DisplayName: "", Provider: "claude", Model: "m", Effort: "high"},
}}
```
（`cox_gone` 不在配置里。）`r := New(cfg, nil, nil, nil, &b, RulesContext{})`（`b` 是 `strings.Builder`，每个子测试各造一个 REPL）；简洁档（K1、K3、K4、K5）构造之后再 `r.SetConcise(true)`，详细档（K2）不调。

**共用的 8 个 Record**（K1、K2 共用，名字即子测试名；未列字段取零值）：

| 名字 | Record |
|---|---|
| `spoke_rich` | `AgentID "cox_a"`、`OutcomeSpoke`、`WallMS 6849`、`Fresh true`、`ProviderSession "sess-x"`、`UsageParsed true`、`CostUSDDelta` 指向 `0.5`、`ColdExpected true`、`Message &{Seq 5, SenderID "cox_a", SenderProvider "claude", SenderModel "m", SenderEffort "high", Text "第一行\n第二行\n"}` |
| `spoke_no_newline` | `"cox_a"`、Spoke、`WallMS 0`、`Message &{Text "OK"}` |
| `spoke_empty` | `"cox_a"`、Spoke、`WallMS 99`、`Message &{Text ""}` |
| `spoke_truncated` | `"cox_a"`、Spoke、`WallMS 60000`、`Message &{Text "半句", Meta {turn.MetaCompletion: "truncated"}}` |
| `timeout_maybe_advanced` | `"cox_a"`、`OutcomeTimeout`、`WallMS 300000`、`MaybeAdvanced true`、`ProviderSession "sess-y"` |
| `post_rejected_drift_denied` | `"cox_b"`、`OutcomePostRejected`、`WallMS 1234`、`Drifted true`、`DroppedText "付过钱的\n两行"`、`DeniedActions [{Action "write_file", DisplayName "WriteToFile"}]`、`UsageParsed true`、`ProviderSession "sess-z"` |
| `refused_not_in_config` | `"cox_gone"`、`OutcomeRefused` |
| `spoke_fallback_id` | `"cox_b"`、Spoke、`WallMS 100`、`Message &{Text "B"}` |

**K1 `TestCoxConciseRecordBlocks`**：简洁档，每个 Record 一个子测试，`b.String()` 整串等于下表（Go 字符串字面量写法）：

| 名字 | 期望 |
|---|---|
| `spoke_rich` | `"甲 A（6.8s）: 第一行\n第二行\n"` |
| `spoke_no_newline` | `"甲 A（0.0s）: OK\n"` |
| `spoke_empty` | `"甲 A（0.0s）:\n本轮为空\n"` |
| `spoke_truncated` | `"甲 A（1m00s）: 半句\n本轮被截断\n"` |
| `timeout_maybe_advanced` | `"甲 A: 没有回复（超时）\nprovider 那边可能已经吃下这批内容\n"` |
| `post_rejected_drift_denied` | `"cox_b: 没有回复（未入库）\n送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了\nprovider 报了 1 个被拒的工具调用（可能不止这些）：write_file(WriteToFile)。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败\n正文写不进库（已经付过钱），整段如下:\n付过钱的\n两行\n"` |
| `refused_not_in_config` | `"cox_gone: 没有回复（拒绝）\n"` |
| `spoke_fallback_id` | `"cox_b（0.1s）: B\n"` |

【判别力】`spoke_rich` 喂的是「原值非空」的全套：会话行、用量、金额、冷暖、seq 都有料，漏掉任何一个门就多出一行；`spoke_empty` 钉「空正文不留尾随空格、提示在头之后」；`spoke_truncated` 钉「提示在正文之后」与 60000 这个边界；`timeout` / `post_rejected` 钉照打的四类行与失败行不带用时；两个 fallback 钉「空 display_name 与配置里没有」两种回落。对照是 K2（同一组 Record 的详细档）。

**K2 `TestCoxVerboseRecordUnchanged`**：详细档（不调 `SetConcise`），同一组 Record，整串等于下表（这就是 HEAD 的输出，原型在 HEAD 函数体上实测一致）：

| 名字 | 期望 |
|---|---|
| `spoke_rich` | `"outcome spoke  agent cox_a\n新开了一条 provider 会话 sess-x\nseq 5  cox_a  claude/m/high\n第一行\n第二行\n用量 in=0 cache_create=? cache_read=0 out=0 thinking=0\n金额 0.5 USD\n冷暖 预报=冷 实测=暖  wall_ms=6849  prompt_bytes=0  scanned_to=0\n"` |
| `spoke_no_newline` | `"outcome spoke  agent cox_a\nseq 0    -\nOK\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=0  prompt_bytes=0  scanned_to=0\n"` |
| `spoke_empty` | `"outcome spoke  agent cox_a\nseq 0    -\n本轮为空\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=99  prompt_bytes=0  scanned_to=0\n"` |
| `spoke_truncated` | `"outcome spoke  agent cox_a\nseq 0    -\n本轮被截断\n半句\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=60000  prompt_bytes=0  scanned_to=0\n"` |
| `timeout_maybe_advanced` | `"outcome timeout  agent cox_a\nprovider 那边可能已经吃下这批内容\nprovider 会话 sess-y\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=300000  prompt_bytes=0  scanned_to=0\n"` |
| `post_rejected_drift_denied` | `"outcome post_rejected  agent cox_b\nprovider 会话 sess-z\n送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了\nprovider 报了 1 个被拒的工具调用（可能不止这些）：write_file(WriteToFile)。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败\n正文写不进库（已经付过钱），整段如下:\n付过钱的\n两行\n用量 in=0 cache_create=? cache_read=0 out=0 thinking=0\n金额未报\n冷暖 预报=暖 实测=暖  wall_ms=1234  prompt_bytes=0  scanned_to=0\n"` |
| `refused_not_in_config` | `"outcome refused  agent cox_gone\n"` |
| `spoke_fallback_id` | `"outcome spoke  agent cox_b\nseq 0    -\nB\n用量 未知（没解析到用量）\n金额未报\n冷暖 预报=暖 实测=未知  wall_ms=100  prompt_bytes=0  scanned_to=0\n"` |

（`seq 0    -` 是 `seq 0  ` + 空 SenderID + `  -`：`brainCell` 三个空串印 `-`。）【判别力】「详细档逐字节不变」的唯一逐字节担保；把门写反（`if !r.concise` 与 `if r.concise` 互换）时 K1、K2 同时红。

**K3 `TestCoxConciseFailureWordsAllOutcomes`**：简洁档，对 `turn.Outcome` 的 9 个非 Spoke 值 `OutcomeUnknown, OutcomePostRejected, OutcomeProviderFailed, OutcomeUnparsable, OutcomeTimeout, OutcomeSpawnFailed, OutcomeRefused, OutcomeCanceled, OutcomeLost` 各喂一个 `Record{AgentID: "cox_a", Outcome: o, WallMS: 1234}`（`Message` 为 nil、`DroppedText` 空），整串等于 `"甲 A: 没有回复（" + 词 + "）\n"`，词依次 `未知、未入库、报错、认不出、超时、没起来、拒绝、取消、丢失`；**唯一例外** `OutcomePostRejected` 的期望是 `"甲 A: 没有回复（未入库）\n正文写不进库（已经付过钱），整段如下:\n"`（`DroppedText` 为空时 `writeFullText` 什么都不写，那一行标题照打——HEAD 既有行为，原型实测）。另断言 `repl.OutcomeWord` 对这 9 个值与 `OutcomeSpoke`（`完成`）逐一相等。
【判别力】结局值全集都走一遍（上一批 W1 的教训：夹具要覆盖变异所在的取值全集）；`WallMS: 1234` 非零，失败行若带用时会印 `（1.2s）`。

**K4 `TestCoxFormatWall`**：`FormatWall` 对 `-1500, 0, 99, 100, 999, 1000, 6849, 59999, 60000, 65432, 3600000` 依次等于 `0.0s, 0.0s, 0.0s, 0.1s, 0.9s, 1.0s, 6.8s, 59.9s, 1m00s, 1m05s, 1h00m`；并且每个值经简洁档成员头（`Record{AgentID: "cox_a", Outcome: OutcomeSpoke, WallMS: ms, Message: &{Text: "x"}}`）整串等于 `"甲 A（" + 期望 + "）: x\n"`。`FormatElapsed` 对 `-1s, 59999ms, 3599s, 7322s` 依次等于 `0s, 59s, 59m59s, 2h02m`。
【判别力】`%.1f` 四舍五入会把 59999 印成 `60.0s`、99 印成 `0.1s`；`<= 60000` 会把 60000 印成 `60.0s`；负数不夹紧时 -1500 印 `-1.-5s`（-1500 而不是 -5：Go 向零截断下 -5 不夹紧也印 `0.0s`，RB-B W1 实测）。

**K5 `TestCoxDisplayNameFollowsCurrentConfig`**：简洁档，同一个 `Record{AgentID: "cox_a", Outcome: OutcomeSpoke, WallMS: 1000, Message: &{Text: "x"}}` 连写三次：第一次原配置；第二次之前把 `r.cfg.Agents["cox_a"]` 的 `DisplayName` 改成 `"乙"`（取出、改、写回 map）；第三次之前把 `r.cfg` 整个换成 `&config.Config{Agents: map[string]config.AgentConfig{"cox_a": {DisplayName: "丙"}}}`。`b.String()` 整串等于 `"甲 A（1.0s）: x\n乙（1.0s）: x\n丙（1.0s）: x\n"`。
【判别力】在 `SetConcise` / `New` 时缓存名字，或持有旧 `*config.Config`，第 2、3 行红。第三步模拟 `/config reload --apply` 与 MD 那一刀的换指针。

### 2.2 `internal/repl/cox_round_test.go`（端到端：真 SQLite、真 Runner、假 CLI）

**装置（冻结）**：

```go
func coxRoundCfg() *config.Config // 名字 cox 前缀、实现方定
```
返回：`Providers: {"claude": {Command: os.Args[0]}, "grok": {Command: filepath.Join(os.TempDir(), "cox-no-such-dir", "cox-missing.exe")}}`；`Agents`：
- `cox_a`：`DisplayName "甲"`，`Provider "claude"`、`Model "claude-fable-5-1"`、`Effort "high"`、`Role "cox"`、`Permissions config.PermissionRead`、`Mode config.ModeDiscussion`；
- `cox_b`：同上但 `DisplayName ""`；
- `cox_j`：同上但 `DisplayName "裁判 J"`、`Mode config.ModeJudge`；
- `cox_x`：`DisplayName "坏 X"`、`Provider "grok"`、`Model "grok-4.7"`、`Effort ""`、其余同 `cox_a`。

装置函数（名字实现方定）收成员列表与一份 `*config.Config`（默认 `coxRoundCfg()`；v1.1 起 §2.2a R11 `argv_refusal` 传一份改过 `cox_a.Model` 的拷贝，`cfg` 同时交给 Runner 与 REPL），按顺序：`t.Setenv("COUNCIL_FAKE_CLI", "1")`；`ctx` 带 2 分钟超时；`svc := newRoundStreamService(t)`；`rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})`；`svc.Create(ctx, "cox")`；按给定顺序 `Join` 成员（`Provider` 取 `"claude"`，`cox_x` 取 `"grok"`；`cox_gone` 也按 `"claude"` 加入，它不在配置里）；`Post` 一条 `SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "seed-msg"`。
REPL：`r := New(cfg, svc, rn, nil, buf, RulesContext{})`（`buf` 是 `*rbaBuf`，§2.2 R10 除外）；`r.Submit(ctx, "/use "+convID)`；清空缓冲；简洁档 `r.SetConcise(true)`；`r.now` 换成一个带锁、计数的假时钟：第 1 次调用返回 `time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)`，之后每次返回它 `+ 7250*time.Millisecond`。
「慢 a」前置（R1-R3 用）：`rn.Ask(ctx, convID, "cox_a")` 一次（首轮，立即回来），再 `Post` 一条 user 消息 `"seed-msg-2"`——于是 `/round` 里 `cox_a` 带 `--resume`、假 CLI 睡 700ms，`cox_b` / `cox_j` 首轮立即回来，完成顺序固定为 `cox_b` 先、`cox_a` 后（同 RB-B RB2 的做法）。
规范化见通用纪律（成员头 `（T）`、余量 `H`）。

**R1 `TestCoxRoundConciseGolden`**：成员 `cox_a, cox_b, cox_j`，慢 a 前置，简洁档，`r.Submit(ctx, "/round cox_a cox_b cox_j --go --with-judge")` 返回 nil。前提守卫：原始缓冲里 `cox_b（` 开头那一行的下标小于 `甲（` 开头那一行（完成顺序；不成立就 `Fatalf` 说前提不成立）。主断言：规范化后整串等于

```
本轮问 2 个: cox_a, cox_b
cox_b（T）: OK
甲（T）: OK
本轮问 1 个: cox_j
裁判 J（T）: OK
总用时 7.2s
```
（末尾有换行。）且假时钟恰好被调用 2 次。
【判别力】这一串同时钉住：发言顺序、预算表、扇出期间、outcome / 会话 / seq / 用量 / 金额 / 冷暖、结算（含「分不清进程是否起来 0」与「意外冷启动」——慢 a 那一轮预报暖实测冷，详细档会打）全部不打；显示名与 id 回落；总用时恰好一行且用 `FormatWall`（`FormatElapsed` 会印 `7s`；用真时钟会印一个大数）。对照是 R2。

**R2 `TestCoxRoundVerboseControl`**：同 R1 装置与命令，但**不**调 `SetConcise`。断言：第一行整行等于 `发言顺序按字典序，重复的 id 已去重，只问一次。`；没有任何一行以 `总用时` 开头；整行 `发言轮结算` 与 `裁判轮结算` 各恰好一次。
【判别力】只差档位一处；总用时的 `r.concise` 判据被拿掉时这里多出一行（RB-A 的 `TestRbaR1NilObserverGolden` 也会红）。

**R3 `TestCoxRoundObserverSuppressesTotal`**：两个子测试 `concise` / `verbose`，各自同 R1 装置与命令，另在 Submit 之前 `r.SetRoundObserver(rec)`，`rec` 是 cox 自己的记录型观察者（带锁；RoundStart / MemberDone 空实现，RoundEnd 追加事件）。
- `concise`：规范化后整串等于 R1 的期望**去掉最后一行 `总用时 7.2s`**；记下的 RoundEnd 恰好 2 个，`Concise` 都是 true。
- `verbose`：记下的 RoundEnd 恰好 2 个，`Concise` 都是 false。
【判别力】装了观察者（TUI）时 REPL 不打总用时，收尾行由 TUI 出；`Concise` 字段随档位（漏赋值时 `concise` 子测试的事件是 false）。对照是 R1（只差有没有观察者）。

**R4 `TestCoxRoundPreviewConcise`**：两个子测试（v1.1 起）：`plain`（v1 的 R4，原样）与 `with_judge`（v1.1，F2）。

`plain`：成员 `cox_a, cox_b`，简洁档，`/round cox_a cox_b`（不带 `--go`）。规范化后整串等于

```
成员  输入token  金额  冷启动  在飞  命令行余量  依据
cox_a  未标定  未标定  预报冷  不在飞  H  无样本
cox_b  未标定  未标定  预报冷  不在飞  H  无样本
合计输入token 0（只含已标定的行）
另有 2 名未标定
```
【判别力】纯预览两档都印预算表（`!r.concise || !goRun` 写成 `!r.concise` 时这里空）；没起进程不打总用时（`spawned` 判据被拿掉时多一行）；「发言顺序」不打。

`with_judge`（v1.1，F2）：成员 `cox_a, cox_j`，简洁档，`/round cox_a cox_j --with-judge`（预览，不带 `--go`）。规范化后整串等于（两张预算表，发言轮在前、裁判轮在后，各带合计与未标定行；末尾有换行）

```
成员  输入token  金额  冷启动  在飞  命令行余量  依据
cox_a  未标定  未标定  预报冷  不在飞  H  无样本
合计输入token 0（只含已标定的行）
另有 1 名未标定
成员  输入token  金额  冷启动  在飞  命令行余量  依据
cox_j  未标定  未标定  预报冷  不在飞  H  无样本
合计输入token 0（只含已标定的行）
另有 1 名未标定
```
且假时钟恰好被调用 1 次。整串里因此**没有**「发言顺序…」（E5 的门）、**没有**「裁判 cox_j 本命令不问…」（带了 `--with-judge`，`renderUnaskedJudges` 不印）、**没有**「总用时」。
【判别力】预览下裁判轮也走到 `runOneRound`、印完预算表返回 false，`judgeRan` 为假；E4 的门写成 `judgeRan || true`（#63）或 `judgeRan || !goRun`（#64）时 `spawned` 被置真，多出 `总用时 7.2s`、时钟 2 次，红（原型实测）。`judgeRan || goRun`（#65）这里杀不到（`goRun` 为假），它的杀手是 R5 `judge_nodelta`（`--go` 下裁判 no_delta、没起进程）——两条用例合起来才覆盖 E4 那道门的两个方向，任何一条被删都会留下缺口。缺席的那几行的正对照：R1（同样 `--with-judge`、裁判真起了进程，总用时在）与 R2（详细档，发言顺序在）。

**R5 `TestCoxRoundNoDeltaConcise`**：五个子测试（v1 三个，v1.1 加 `round2`、`judge_nodelta`），简洁档，各自的前置见表：「先问 b」= `rn.Ask(ctx, convID, "cox_b")` 一次（之后不再发消息，于是 `cox_b` 无新内容）；「先问 j」= `rn.Ask(ctx, convID, "cox_j")` 一次；「慢 a」= §2.2 装置里的慢 a 前置（R1-R3 同一个）。

| 子测试 | 成员（Join 顺序） | 前置 | 命令 | 规范化后整串 | 假时钟调用次数 |
|---|---|---|---|---|---|
| `mixed` | `cox_a, cox_b, cox_gone, cox_j` | 先问 b | `/round cox_a cox_b cox_gone cox_j --go` | `跳过 cox_gone（not_in_config）：cox_gone 不在配置里\n裁判 cox_j 本命令不问，要用 --with-judge 才问。\n跳过 cox_b（no_delta）：cox_b 无新内容，本轮跳过\n本轮问 1 个: cox_a\n甲（T）: OK\n总用时 7.2s\n` | 2 |
| `only` | `cox_b` | 先问 b | `/round cox_b --go` | `跳过 cox_b（no_delta）：cox_b 无新内容，本轮跳过\n这一轮没有人可问。\n` | 1 |
| `judge_only` | `cox_b, cox_j` | 先问 b | `/round cox_b cox_j --go --with-judge` | `跳过 cox_b（no_delta）：cox_b 无新内容，本轮跳过\n这一轮没有人可问。\n本轮问 1 个: cox_j\n裁判 J（T）: OK\n总用时 7.2s\n` | 2 |
| `round2` | `cox_a, cox_x` | 慢 a | `/round cox_a cox_x --go --rounds 2` | `本轮问 2 个: cox_a, cox_x\n坏 X: 没有回复（没起来）\n/round cox_x: <原因>\n甲（T）: OK\n分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）\n跳过 cox_a（no_delta）：cox_a 无新内容，本轮跳过\n本轮问 1 个: cox_x\n坏 X: 没有回复（没起来）\n/round cox_x: <原因>\n分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）\n总用时 7.2s\n` | 2 |
| `judge_nodelta` | `cox_j` | 先问 j | `/round cox_j --go --with-judge` | `这一轮没有人可问。\n跳过 cox_j（no_delta）：cox_j 无新内容，本轮跳过\n这一轮没有人可问。\n` | 1 |

`round2` 的规范化多一步（通用纪律 v1.1 那条）：先守卫原始输出里以 `/round cox_x: ` 开头的行恰好 2 行，再把这两行的原因换成 `<原因>`。原型实测的原因行（Windows，供核对）：`/round cox_x: provider cox_x path C:\Users\…\Temp\cox-no-such-dir\cox-missing.exe exit 0 stderr "" stdout "" planned_session <uuid>: fork/exec …\cox-missing.exe: The system cannot find the path specified.`。慢 a 让第 1 个发言轮的完成顺序固定（`cox_x` 起不来、立刻回来，`cox_a` 带 `--resume` 睡 700ms），所以整串逐字；V1 要的三条（第 2 轮的行按顺序从上一个匹配处往后找、no_delta 行恰好一次、第 2 轮失败行之后还有「分不清」那一行）都被整串蕴含。第 2 轮 `cox_a` 为 no_delta 是今天的行为（第 1 轮它说完之后库里没有对它算新内容的消息；W1 证伪者在 HEAD 详细档上实测同一形状）。

【判别力】`mixed`：no_delta 行只打新追加的那一档（打全量 `plan.Skipped` 时 `cox_gone` 出现两次）、分类跳过与「谁没被问」两档照打、发言者起了进程才记 `spawned`；`only`：没人起进程就没有总用时、时钟只在入口调一次、「这一轮没有人可问。」照打；`judge_only`：只有裁判轮起进程也要记 `spawned`。三者互为对照（只差谁起了进程）。
`round2`（v1.1，F1）：no_delta 行与「分不清进程是否起来 1（…）」在第 2 个发言轮（`k = 2`）照打——门写成 `k <= 1`（#59、#61）时第 2 轮那一行没了，红；对照是 `mixed`（第 1 轮的 no_delta 行）与 R6（第 1 轮的「分不清」那一行），这两条在 #59 / #61 下保持绿（#61 实测 R6 绿）。`judge_nodelta`（v1.1，F1）：no_delta 行在裁判轮（`label` 为「裁判轮」、`k = 0`）照打——门写成只在发言轮（#60）时第 2 行没了，红；同时它是 `--go` 下「裁判轮没起进程」的唯一夹具：`judgeRan` 为假、`goRun` 为真，`spawned` 必须仍为假，没有总用时、时钟 1 次——把 E4 的门写成 `judgeRan || goRun`（#65）或 `judgeRan || true`（#63）时多出一行 `总用时 7.2s`、时钟 2 次，红。对照是 `judge_only`（同样 `--go --with-judge`，裁判真起了进程，总用时在）。

**R6 `TestCoxRoundSpawnFailedConcise`**：成员 `cox_a, cox_x`，简洁档，`/round cox_a cox_x --go`。规范化后按 `"\n"` 切行（去掉末尾空串）恰好 6 行：第 0 行整行 `本轮问 2 个: cox_a, cox_x`；第 4 行整行 `分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）`；第 5 行整行 `总用时 7.2s`；第 1-3 行里恰好一行整行等于 `坏 X: 没有回复（没起来）`、紧随其后的一行以 `/round cox_x: ` 开头（前提守卫，原因文本含路径与随机 id，只查前缀）；整行 `甲（T）: OK` 恰好一次。（两个成员的先后不断言。）
【判别力】spawn_failed 这一档：失败行、`/round <id>:` 原因行、结算里唯一照打的那一行（N>0）都在，而结算的其余行不在；把那一行改成另一份字面量（比如只印 `分不清进程是否起来 1`）时第 4 行红。

**R7 `TestCoxAskSayConcise`**：成员 `cox_a, cox_b`，简洁档，依次 Submit `/ask cox_a 你好 @cox_b @nobody`、`/say hi`、`/to cox_a 只给甲`，全部返回 nil。规范化后整串等于

```
nobody 不是当前会话的成员
正在问 cox_a ...
甲（T）: OK
只有 cox_a 会看到这条；其余成员永远不会 —— 游标只前进
```
且假时钟调用 0 次。
【判别力】「已记下 seq N」（三次落库）与「点名了 cox_b」不打；「不是当前会话的成员」「只有…会看到」「正在问」照打；`/ask` 没有总用时。

**R8 `TestCoxAskRefusedConcise`**：成员 `cox_a`，先 `svc.BeginTurn(ctx, conversation.InflightTurn{TurnID: "cox-stale-turn", ConvID: convID, AgentID: "cox_a", Provider: "claude", StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, Tier: "read"})`；简洁档，`/ask cox_a`。缓冲**原样**（不规范化）整串等于

```
正在问 cox_a ...
甲: 没有回复（拒绝）
这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行
```
【判别力】`/ask` 的原因行（`askCause`）在简洁档照打；失败行不带用时。

**R9 `TestCoxVerboseCommand`**：成员 `cox_a`，从详细档开始（不调 `SetConcise`）。逐步 Submit（每步之前清空缓冲），每步断言缓冲整串等于「期望行 + `\n`」且 `r.concise` 等于期望档位：

| 步 | 命令 | 期望行 | `r.concise` |
|---|---|---|---|
| 1 | `/verbose` | §3.3 简洁提示 | true |
| 2 | `/verbose` | §3.3 详细提示 | false |
| 3 | `/verbose off` | 简洁提示 | true |
| 4 | `/verbose off` | 简洁提示 | true |
| 5 | `/verbose on` | 详细提示 | false |
| 6 | `/verbose on` | 详细提示 | false |
| 7 | `/verbose bad` | §3.3 用法 | false |
| 8 | `/verbose on off` | 用法 | false |
| 9 | `/verbose off` | 简洁提示 | true |
| 10 | `/verbose ON` | 用法 | true |

然后：`/ask cox_a` 规范化后整串等于 `正在问 cox_a ...\n甲（T）: OK\n`；`/verbose on`；`Post` 一条 user 消息 `"more"`；清空；`/ask cox_a` 的输出按 `"\n"` 切开，第 2 行（下标 1）整行等于 `outcome spoke  agent cox_a`；清空；`/help`，存在一行以 `/verbose ` 开头，且去掉 `/verbose` 前缀再 `TrimSpace` 后整串等于 §3.4 的 summary。
【判别力】切换 / 设定 / 用法三种语义、大小写与参数个数、档位确实被后续命令读到、命令表登记。

**R10 `TestCoxRoundWriteErrorKeepsError`**：成员 `cox_a, cox_b`（不做慢 a 前置），简洁档，输出换成 cox 自己的失败写者（带锁；把写入累积到内部缓冲；当某次写入的内容含 `本轮问 ` 时计数，**第 2 次**含它的写入返回包级错误变量 `coxWriteErr = errors.New("cox injected write failure")`、不写入）。`err := r.Submit(ctx, "/round cox_a cox_b --go --rounds 2")`。断言：`errors.Is(err, coxWriteErr)`；内部缓冲不含 `总用时`；计数恰好 2（前提守卫）。
【判别力】第 1 段已经起过进程（`spawned` 为真），第 2 段写屏幕失败；defer 若不看 `err != nil`，会在失败之后再打一行总用时并把返回的错误换成 nil。

### 2.2a `internal/repl/cox_keep_test.go`（v1.1，F3；R11，简洁档照打的行）

#45（`:2125`）列为简洁档照打的四类——「已跑 k/N 轮，其余取消」、所有拒绝句、持久化失败、argv 整轮拒绝——v1 没有一条简洁档夹具，给任何一处加上 `r.concise` 门全绿（W1 的 X5 / X6 / X11 / X14）。本文件每一类至少一个打印点；同一句话的多个打印点，凡是用已取消的 ctx、取消观察者或取消写者走得到的，**逐个走到**（「已跑 k/N」两处 × 装与不装观察者、`拒绝` 六处，各一个子测试）。`reportErr("round", …)` 的六处只有 `round.go:105` 走得到，其余五处见 §4「只在审查时看」（v1.1 W1b 修订；原稿写的是「同一句话有几个打印点就有几个子测试」，对 `reportErr` 不成立）。装置复用 §2.2 的（`coxRoundCfg`、装置函数、REPL 构造、假时钟、规范化、慢 a 前置、记录型观察者；名字实现方定，只读引用同包的 `cox_round_test.go`）。观察者在 v1.1 多一个可选的回调：RoundEnd 追加事件之后、放锁之后调它（R3 不设，行为不变）。W1b 修订另加一个 cox 自己的**取消写者**（`canceled_line`、`canceled_judge_line` 用；名字实现方定）：带锁；把写入累积到内部缓冲，有 `String`；某次写入的内容含给定子串时，写完之后调一次给定的 `cancel`（只调第一次）并数触发次数。它照 R10 失败写者的做法当 REPL 的输出，`/use` 的回显不算进断言（`/use` 之后清空，或 `/use` 之后再换上它）。

**R11 `TestCoxConciseKeepsLines`**，简洁档，子测试：

- `canceled`：成员 `cox_a, cox_b`，慢 a 前置；`ctx2, cancel := context.WithCancel(ctx)`；装记录型观察者，回调是 `cancel`（第 1 段 RoundEnd 时取消）；`r.Submit(ctx2, "/round cox_a cox_b --go --rounds 2")` 返回 nil。前提守卫同 R1（`cox_b（` 那一行在 `甲（` 那一行之前）。规范化后整串等于 `本轮问 2 个: cox_a, cox_b\ncox_b（T）: OK\n甲（T）: OK\n已跑 1/2 轮，其余取消\n`，假时钟恰好 1 次（装了观察者，defer 在判 `r.roundObs != nil` 时就返回，不取第二次时钟）。走到的是发言轮循环里那一处（`round.go:192`）。
- `canceled_judge`：同一句的另一个打印点（裁判轮之前，`round.go:223`）。成员 `cox_a, cox_j`，不要慢 a；同样的取消观察者；`/round cox_a cox_j --go --with-judge`。规范化后整串等于 `本轮问 1 个: cox_a\n甲（T）: OK\n已跑 1/1 轮，其余取消\n`，假时钟 1 次。（裁决只点了 `canceled`；「同类的还有没有别处」：这句有两个打印点，各一条。）
- `canceled_line`（W1b 修订）：与 `canceled` 同一成员（`cox_a, cox_b`）、同一前置（慢 a）、同一命令，**只差不装观察者**（行式）。取消改由取消写者触发，子串是 `甲（`：慢 a 那个最后回来的成员头写出之后才取消。`ctx2, cancel := context.WithCancel(ctx)`；`r.Submit(ctx2, "/round cox_a cox_b --go --rounds 2")` 返回 nil。前提守卫：写者恰好触发 1 次；完成顺序同 R1（`cox_b（` 那一行在 `甲（` 那一行之前）。规范化后整串等于 `本轮问 2 个: cox_a, cox_b\ncox_b（T）: OK\n甲（T）: OK\n已跑 1/2 轮，其余取消\n总用时 7.2s\n`，假时钟恰好 2 次。走到的是 `round.go:192`；defer 的四个判据都成立（没有写错误、第 1 段起过进程、简洁、没装观察者），所以取消之后照打总用时（§0 第 8 条）。
- `canceled_judge_line`（W1b 修订）：`canceled_judge` 的行式镜像，同一句的另一个打印点（`round.go:223`）。成员 `cox_a, cox_j`，不要慢 a，不装观察者，取消写者同上（子串 `甲（`）；`r.Submit(ctx2, "/round cox_a cox_j --go --with-judge")` 返回 nil。前提守卫：写者恰好触发 1 次。规范化后整串等于 `本轮问 1 个: cox_a\n甲（T）: OK\n已跑 1/1 轮，其余取消\n总用时 7.2s\n`，假时钟 2 次。
- `persist_fail`：成员 `cox_a`；`ctx2, cancel := context.WithCancel(ctx)` 后立即 `cancel()`；`r.Submit(ctx2, "/say hi")` 返回 nil。断言：缓冲以 `发言失败: ` 开头、恰好一行（以 `\n` 结尾、只有一个 `\n`）。只断言前缀：后面是存储层的错误链，不是本刀的字面量。原型实测整行 `发言失败: post message: get conversation <convID>: context canceled`（供核对，不是断言）。走的是 `postUserText` → `reportErr`（`commands.go:336`、`repl.go:266`）。
- `round_report_err`（W1b 修订）：成员 `cox_a`；`ctx2, cancel := context.WithCancel(ctx)` 后立即 `cancel()`；`r.Submit(ctx2, "/round cox_a --go")` 返回 nil。断言：缓冲以 `round: ` 开头、恰好一行（以 `\n` 结尾、只有一个 `\n`）；假时钟恰好 1 次。只断言前缀，理由同 `persist_fail`。原型实测整行 `round: list participants <convID>: context canceled`（供核对，不是断言）。走的是 `cmdRound` 里 `r.svc.Participants` 失败那一处 `reportErr("round", err)`（`round.go:105`）：参数解析不碰库，已取消的 ctx 在第一次碰库时就失败，所以只走得到这一处。
- `refusals`：成员 `cox_a`，一个 REPL，六个子子测试，每个之前清空缓冲、换一个新的假时钟；缓冲整串等于「期望行 + `\n`」，假时钟恰好 1 次（`start` 是 `cmdRound` 的第一条语句，拒绝在 defer 里因 `!spawned` 返回）：

  | 子子测试 | 命令 | 走到的打印点 | 期望行 |
  |---|---|---|---|
  | `rounds_no_value` | `/round cox_a --go --rounds` | `round.go:82` | `拒绝: --rounds 需要一个不小于 1 的整数。` |
  | `rounds_not_int` | `/round cox_a --rounds abc --go` | `round.go:86` | `拒绝: --rounds 需要一个不小于 1 的整数。` |
  | `rounds_zero` | `/round --rounds 0 --go` | `round.go:95` | `拒绝: --rounds 需要一个不小于 1 的整数。` |
  | `rounds_without_go` | `/round cox_a --rounds 2` | `round.go:100` | `拒绝: --rounds 2 只在 --go 下可用。` |
  | `unknown_member` | `/round nobody --go` | `round.go:126` | `拒绝: 不是当前活跃成员: nobody` |
  | `empty_roster` | `/round --go` | `round.go:141` | `拒绝: 名单为空。给了 id 才用给的；没给则取当前会话最后一条用户发言里的 mention 与当前活跃成员的交集。不会默认全员。` |

  （`empty_roster` 的名单取自种子消息 `seed-msg`，它没有 mention，交集为空。）
- `argv_refusal`：配置是 `coxRoundCfg()` 的一份，`cox_a` 的 `Model` 换成 `strings.Repeat("m", 40000)`（Model 走 argv，`provider/claude.go:92` 的 `--model`；Role 走 stdin，拉长 Role 压不出负余量——W1 实测）；成员 `cox_a`；`/round cox_a --go`。按通用纪律把 `超了 \d+ 字节` 换成 `超了 N 字节` 之后整串等于 `拒绝整轮: cox_a 超了 N 字节。命令行余量为负的人这一轮必然失败。若只把他们拿掉、其余人继续跑，其余人的钱照花，用户拿到残缺的一轮（五个人里两个必然失败、另外三个的钱照花，就是这个形状）；整轮不跑比部分跑更便宜。\n`，假时钟 1 次。简洁档带 `--go` 不印预算表，屏幕上只有这一行；不起进程（原型实测 0.03 秒，字节数 7507）。

【判别力】每一类都是整行（`persist_fail` 除外，理由见上）：给对应打印点加 `if r.concise { return nil }`（或 argv 处 `return false, nil`），那一行就没了，红（§4 #66-#75，原型逐条实测）。三处「--rounds 需要一个不小于 1 的整数」打同一句话，判别力不在文本、在走到的是哪个调用点：每个子子测试只走一个，另外两个在对方的变异下保持绿（#69 下 `rounds_not_int` 绿、#70 下 `rounds_no_value` 绿、#71 下 `rounds_no_value` 绿，原型实测）；`canceled` 与 `canceled_judge` 同理（#66 下 `canceled_judge` 绿、#67 下 `canceled` 绿）。W1b 修订的三个子测试：`canceled` 与 `canceled_line` 只差有没有观察者，互为「总用时」有无的对照（`canceled_judge` 与 `canceled_judge_line` 同理）；defer 加 `|| ctx.Err() != nil`（#77）时两条 `_line` 红、两条装观察者的绿；「已跑 k/N」只在装了观察者时打（#78、#79）时各自那条 `_line` 红，另一处的 `_line` 与装观察者的那条绿；`:105` 那一处加门（#80）时 `round_report_err` 红、`persist_fail` 绿（原型逐条实测）。缺席「总用时」的正对照：R1（同一装置、没装观察者、起了进程，总用时在）；W1b 修订起更贴近的正对照是 `canceled_line`（同一装置、同一命令，只差没装观察者，总用时在）。原来 §4「只在审查时看」第 3 条（「已跑 k/N」与 `argvRefusal` 要造取消或负余量、本刀不另写用例）是错的，已删。

### 2.3 `internal/tui/cox_concise_test.go`

**T1 `TestCoxRoundEndConciseSendsOnlyEndMsg`**（纯函数，不起 Program）：`ev := repl.RoundEndEvent{Label: "发言轮", K: 1, N: 1, Concise: true, Results: []repl.MemberDoneEvent{{Label: "发言轮", AgentID: "cox_a", Outcome: turn.OutcomeSpoke, WallMS: 6849, RoundID: "r-000000abcdef"}, {Label: "发言轮", AgentID: "cox_b", Outcome: turn.OutcomeTimeout, MaybeAdvanced: true, WallMS: 300000, RoundID: "r-000000abcdef"}}}`；`newRoundObserver(func(msg tea.Msg){ got = append(got, msg) }).RoundEnd(ev)`：`got` 恰好 1 条且 `reflect.DeepEqual(got[0], roundEndMsg{ev: ev})`。对照：同一个 ev 把 `Concise` 改成 false，`got` 恰好 4 条，前 3 条经 `rbbBody` 取正文依次整串等于 `发言轮 (abcdef)  回来 2/2`、`  完成    cox_a  6.8s 子进程`、`  超时    cox_b  5m00s 子进程  可能已吃下`，第 4 条 DeepEqual `roundEndMsg{ev: ev}`。
【判别力】`Results` 非空（包装层要喂原返回值非空的输入：空 Results 本来就不出块，杀不到「忽略 Concise」）；简洁档仍发 `roundEndMsg`（把它也挪进 if 时第 1 条断言红）。

**T2 `TestCoxProgramConciseNoBlockOneTotal`**（真 `tea.Program`、真 REPL、真 Runner、假 CLI）：两个子测试 `concise` / `verbose`。各自 `g := rbaNewTuiRig(t)`；`concise` 子测试 `g.r.SetConcise(true)`（在起 Program 之前）；`pr := rbbStartProg(t, g)`；`pr.useConv(t, g.convID)`；`pr.submit("/round rba_a rba_b --go")`；`pr.waitFor(30*time.Second, rbbDoneThenFin(2))` 不成立就 `Fatalf`；`w := rbbWindow(pr.spy.entries(), "/round rba_a rba_b --go")`。断言：
- 两个子测试：`rbbEventKinds(w)` 的种类串恰好 `RS MD MD RE DONE FIN`；窗口里没有任何 P 以 `总用时` 开头；最后一条 P 整行匹配 `^/round 结束  \d+s  回来 2/2$`。
- `concise`：P 恰好 4 条：第 1 条整行 `本轮问 2 个: rba_a, rba_b`，第 2、3 条都整行匹配 `^X（\d+\.\ds）: OK$`，第 4 条是收尾行；RE 前紧挨着的那一条日志的 kind 是 `MD`（中间没有块）。
- `verbose`：RE 前紧挨着的那一条日志的 kind 是 `P` 且整行匹配 `^  完成    rba_b  \d+\.\ds 子进程$`（段终态块的最后一行：块按名单顺序，`rba_b` 在最后）。
【判别力】TUI 里恰好一行总用时（收尾行），REPL 不打第二行；简洁档没有段终态块、看板与收尾行照旧；两个子测试只差档位。

### 2.4 `internal/app/cox_app_test.go`（真 `app.Run`、真 store）

**A1 `TestCoxAppDefaultConcise`**：四个子测试：`default` / `verbose`（v1，下面这段）与 `tui` / `tui_verbose`（v1.1，见后面「A1 续」）。`default` / `verbose` 各自 `isolateAppEnv(t)`、`t.Setenv("COUNCIL_FAKE_CLI", "1")`、`dir := t.TempDir()`、`dataDir := filepath.Join(dir, "data")`；配置文本（照 `smoke_test.go:101-114` 的体例）：

```
data_dir = <tomlLiteral(dataDir)>

[providers.claude]
command = <tomlLiteral(os.Args[0])>

[agents.c1]
display_name = "成员一"
provider = "claude"
model = "opus"
effort = "high"
role = "cox"
permissions = "read"
mode = "discussion"
```
`writeTestConfig` 写盘；**直接调 `Run`**（不用 `runAppForTest`）：`Run(context.Background(), Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut})`，`verbose` 子测试另加 `Verbose: true`；`script = "/new t\n/join c1\n裸文本 @c1\n/round c1 --go\n/ask c1 再问\n/quit\n"`；`Run` 返回 nil。取「尾部」：`out` 按 `"\n"` 切行，每行反复去掉行首的 `council> `，从以 `c1 已加入会话 ` 开头的那一行**之后**开始收集，每行做成员头规范化，整行匹配 `^总用时 \d+\.\ds$` 的换成 `总用时 T`。
- `default`：尾部（字符串切片）整体等于 `["本轮问 1 个: c1", "成员一（T）: OK", "总用时 T", "正在问 c1 ...", "成员一（T）: OK", "再见", ""]`。
- `verbose`：尾部含整行 `outcome spoke  agent c1`，不含 `总用时 T`。
【判别力】`Options` 零值就是简洁（app 默认），`Verbose: true` 回到今天；行式分支确实调了 `SetConcise`；裸文本在简洁档不打「已记下」「点名了」（所以两个 `council> ` 连在一起）。两个子测试只差 `Verbose`。TUI 分支由下面两个子测试覆盖。

**A1 续（v1.1，F0）：`tui` / `tui_verbose` 两个子测试，走 app.go 的 TUI 分支。** `tui.Run` 收的是 `*os.File`，`os.Pipe()` 给的也是 `*os.File`，TUI 分支单测进得去（先例：`internal/tui/tui1d_run_final_test.go`、`tui1d_run_ctx_test.go`、`rba_wiring_test.go`）。两个子测试共用一个装置函数（名字实现方定），参数只有 `Verbose`：
- 前置同上（`isolateAppEnv`、`COUNCIL_FAKE_CLI=1`、同一份配置文本、`writeTestConfig`）。
- **第 0 段（行式，建会话）**：`Run(context.Background(), Options{ConfigPath: cfgPath, In: strings.NewReader("/new t\n/join c1\nhi\n/quit\n"), Out: &out0, Err: &err0})`（两个子测试这一段完全相同，不带 `Verbose`），返回 nil（前提守卫）；从 `out0` 用正则 `已建会话 (\S+)，标题: t` 取 convID（前提守卫）。行式串行读行，没有 TUI 的 busy 吞 Enter（`internal/tui/model.go:356-359`）那个竞态；用户消息 `hi` 让 c1 在下一段有新内容（没有它 c1 是 no_delta）。第 0 段 `Run` **返回之后**才开第 1 段：同一个 `data_dir`，`store.Open` 的排他锁要等 `Run` 里 defer 的 `Close` 放掉。
- **第 1 段（TUI）**：`inR, inW := os.Pipe()`、`outR, outW := os.Pipe()`；读 goroutine 在 `Run` 之前起（`io.Copy` 进一个带锁的缓冲）；`Run(ctx, Options{ConfigPath: cfgPath, TUI: true, InFile: inR, OutFile: outW, Err: errOut, Verbose: v})`，`ctx` 可取消。**`In`、`Out` 留 nil**（TUI 分支会换掉 Out；In 与 InFile 同一个 fd 是潜在的双读者）；`Err` 接住（带锁缓冲），任何一步等不到都把它打进 `Fatalf`。
- **同步点（每一步都有，缺一不可）**：等原始字节里出现 `ESC[J`（事件循环起来，5 秒）→ 写 `ESC[8;24;100t`（窗口尺寸报告，照 `rba_wiring_test.go`）→ 等「空闲表头」（5 秒）→ 写 `/use <convID>\r` → 等「当前会话:」（5 秒）→ 写 `/round c1 --go\r` → 等收尾行「/round 结束」（15 秒）。`/use` 的回显 `当前会话: <id>，标题: t` 能当同步点：`cmdUse`（`commands.go:792`）不在本刀任何档位门里，两档都打（原型实测两档都出现）。收尾行在 Submit 返回之后才由 Model 打，此前本轮的全部 REPL 输出已经进了回滚区，所以等到它之后再断言缺席是有效的。
- 收尾顺序照 `tui1d_run_final_test.go`：cancel、等 `Run` 返回（5 秒）、关 `inW`、`inR`、`outW`、等读 goroutine（5 秒）、关 `outR`；`t.Cleanup` 里再挂一次（幂等）。
- **比对形状（冻结）**：把 stdout 管道的全部字节剥掉 ANSI 转义（`github.com/charmbracelet/x/ansi` 的 `Strip`；go.mod 里已有这个依赖，不改 go.mod，原型实测），再删掉全部 ASCII 空格（渲染器可能用光标移动跳过没变的空格），记作 flat。同步点里的「空闲表头」是 flat 含 `council空闲`（`Options.Version` 为空串），「当前会话:」「/round 结束」分别是 flat 含 `当前会话:`、`/round结束`。
- `tui`（默认简洁）：flat 匹配正则 `成员一（\d+\.\ds）:OK`；flat **不含** `outcomespokeagentc1`、**不含** `发言轮结算`；flat 匹配 `/round结束\d+s回来1/1`。
- `tui_verbose`（`Verbose: true`）：flat **含** `outcomespokeagentc1` 与 `发言轮结算`；flat **不匹配** `成员一（\d+\.\ds）:OK`；flat 匹配 `/round结束\d+s回来1/1`。
- 原型实测的 flat（**摘录，供核对，不是断言**；简洁档，第 1 段 `/round` 那一截，`\r\n` 照录，省略号是删掉的活区帧）：`>/roundc1--go\r\n\r\n本轮问1个:c1\r\n…在跑c10s…\r\r\n成员一（0.0s）:OK\r\n\r\n/round结束0s回来1/1`；详细档同一截依次有 `发言顺序按字典序…`、预算表、`扇出期间…`、`本轮问1个:c1`、`outcomespokeagentc1`、`新开了一条provider会话…`、`seq4c1claude/opus/high`、`OK`、用量 / 金额 / 冷暖、`发言轮结算` 整块、段终态块 `发言轮(xxxxxx)回来1/1`、`完成c10.0s子进程`、`/round结束0s回来1/1`。原型 `-count=10` 零偶发（两个子测试各约 0.2 秒）。

【为什么是子串】这是终端字节流，不是行；帧与帧之间夹着光标移动与重绘。整串相等做不到，只能剥 ANSI、删空格之后找整句（同 `rba_wiring_test.go` 的 W1，那里写了同样的理由）。每一句都是一整行的全部内容（成员头连正文、outcome 行、结算标题、收尾行），不是行的一截。
【判别力】`tui` 与 `tui_verbose` 装置完全相同、只差 `Options.Verbose`，互为对方缺席断言的正对照：`tui` 里「不含 `outcomespokeagentc1` / `发言轮结算`」由 `tui_verbose` 证明同一装置下这两句确实会出现；`tui_verbose` 里「不匹配成员头」由 `tui` 证明它确实会出现。TUI 分支不调 `SetConcise`（§4 #56）→ 默认档是零值详细，`tui` 红；写反成 `SetConcise(opts.Verbose)`（#57）→ `tui` 与 `tui_verbose` 都红；写死 `SetConcise(true)`（#58）→ `tui_verbose` 红、`tui` 绿（原型实测三条）。

### 2.5 `cmd/council/cox_flag_test.go`（v1.1，F5）

`run` 先建 FlagSet、`Parse`、处理 `-version`，之后才注册信号、碰进程的其余状态，所以 `run([]string{"-verbose", "-version"})` 没有副作用（只往进程 stdout 打一行 `council 0.0.0-dev`）。`-h` 同理：`Parse` 返回 `flag.ErrHelp`，`run` 当场返回 `exitOK`，只往 FlagSet 的输出打一遍用法；那个输出是 `fs.SetOutput(os.Stderr)` 在 `run` 被调用时读到的全局 `os.Stderr`，所以测试换掉 `os.Stderr` 就接得住。

**L1 `TestCoxVerboseFlagRegistered`**，三条断言（第三条是 v1.1 W1b 修订加的）：
1. `run([]string{"-verbose", "-version"})` 等于 `exitOK`；
2. 对照：`run([]string{"-cox-no-such-flag", "-version"})` 等于 `exitUsage`（它会往 stderr 打一遍用法，无害）；
3. 把 `os.Stderr` 临时换成 `os.Pipe()` 的写端：读 goroutine 在调 `run` **之前**起，把读到的字节全收进缓冲（Windows 管道缓冲小，先调后读可能卡死）；调 `run([]string{"-h"})`；还原 `os.Stderr`、关写端、等读 goroutine 读完、关读端（不 `t.Parallel`）。返回值等于 `exitOK`，且捕获的用法里含这一整段（Go 字符串字面量写法，`\t` 是一个 tab）：`"  -verbose\n    \t详细输出：每轮的用量、金额、冷暖、provider 会话、预算表与结算全部显示（默认简洁；进去之后也可以用 /verbose 切换）\n"`。

【为什么是子串】用法块里还有别的 flag（HEAD 的 `-config`、`-no-tui`、`-version`，MD / SM 也可能加），整块逐字会被别的刀改红；这一段是 `-verbose` 在 `flag.PrintDefaults` 格式下的全部两行（名字一行、说明一行），不是行的一截。原型实测的整块用法（供核对，不是断言）：`Usage of council:\n  -config string\n    \t配置文件路径，…\n  -no-tui\n    \t不用 TUI，…\n  -verbose\n    \t详细输出：…切换）\n  -version\n    \t打印版本号后退出，不加载配置\n`。
【判别力】flag 改名（§4 #76）时 `-verbose` 成了未定义的 flag，`Parse` 失败，第一条返回 `exitUsage`（2），红（原型实测；把 HEAD 的 `main.go` 放回原型跑这个文件，同样第一条红：HEAD 上它能编译、运行期红）。对照那一条证明第一条断言能为假：同一个 `run`、同样带 `-version`，一个确实没登记的 flag 必须走 `exitUsage`。第三条钉住 §3.5 的默认值与说明文字：默认改成 true（#81）时 `PrintDefaults` 在说明后面补 ` (default true)`，这一段不再以 `切换）\n` 结尾，红；说明文字改一个字（#82）红（原型实测：两条都只红在第三条，前两条照过）。它在忠实实现上确实为真：默认 false 时 `PrintDefaults` 不印 `(default false)`（原型实测）。【判别不了的】`Verbose: *verbose` 是否真的传进 `app.Options`——`run` 要走到 `app.Run` 就得过了 `-version`、`-h` 那两处返回，之后硬编码进程标准流、注册进程级信号，仍只在审查时看（§4）。

---

## 3. 冻结的字面量（逐字节；用户可见的新字面量全部列出）

### 3.1 成员头与失败行

```
<显示名>（<用时>）: <正文原样>
<显示名>（<用时>）:
<显示名>: 没有回复（<结局词>）
```
- 括号是全角 U+FF08 / U+FF09；冒号是 ASCII `:`；成员头有正文时冒号后一个 ASCII 空格、紧接正文第一行（正文多行照原样，不以换行结尾时补一个 `\n`）；正文为空时冒号后直接换行。
- `<显示名>` = 当前 `r.cfg.Agents[id].DisplayName`，空或配置里没有时是 id。
- `<用时>` = `FormatWall(rec.WallMS)`：`0.0s` … `59.9s`（向零截断到十分之一秒）、`1m00s` … `59m59s`、`1h00m` …；负数当 0。
- `<结局词>` = `OutcomeWord`：`完成`、`未入库`、`报错`、`认不出`、`超时`、`没起来`、`拒绝`、`取消`、`丢失`、`未知`（与 TUI 看板同一份，既有字面量，本刀只是搬家）。

### 3.2 总用时（行式、简洁档、`/round` 至少一段起了进程、无写错误）

```
总用时 <FormatWall(毫秒)>
```
例：`总用时 7.2s`。TUI 里不打这一行，由既有收尾行 `/round 结束  <FormatElapsed>  回来 k/N`（或 `已取消`）代替（字面量不变）。

### 3.3 `/verbose` 的三句

```
已切到简洁输出：只显示成员的回复、用时，以及关系到钱或内容丢失的提示；/verbose on 切到详细
已切到详细输出：用量、金额、冷暖、会话 id、预算表与结算全部显示；/verbose off 回到简洁
用法: /verbose 在简洁与详细之间切换；/verbose on 切到详细；/verbose off 切到简洁
```
（全角冒号 U+FF1A、全角逗号 U+FF0C、顿号 U+3001、全角分号 U+FF1B；`用法:` 是 ASCII 冒号加空格，与既有 `用法: /ask ...` 同体例。）

### 3.4 `/help` 里的一行

summary 逐字：

```
切换输出档位: /verbose 在简洁与详细之间切换；/verbose on 详细；/verbose off 简洁
```

### 3.5 启动参数

flag 名 `verbose`，默认 false，说明文字逐字（默认值与说明文字都由 §2.5 L1 第三条钉住，v1.1 W1b 修订）：

```
详细输出：每轮的用量、金额、冷暖、provider 会话、预算表与结算全部显示（默认简洁；进去之后也可以用 /verbose 切换）
```

### 3.6 简洁档照打、沿用原句的既有字面量（不是本刀新增，列出供对照）

`provider 那边可能已经吃下这批内容`；`送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了`；`provider 报了 %d 个被拒的工具调用（可能不止这些）：%s。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败`；`本轮为空`；`本轮被截断`；`正文写不进库（已经付过钱），整段如下:`；`跳过 %s（%s）：%s`（no_delta 在简洁档借它另起一行）；`裁判 %s 本命令不问，要用 --with-judge 才问。`；`这一轮没有人可问。`；`已跑 %d/%d 轮，其余取消`；`分不清进程是否起来 %d（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）`（简洁档只在 N>0 时打）；`拒绝: ...` 各句与 `argvRefusal`；`/round %s: %s`；`askCause` 各句；`正在问 %s ...`；`本轮问 %d 个: %s`；`%s 不是当前会话的成员`；`只有 %s 会看到这条；其余成员永远不会 —— 游标只前进`；`reportErr` 的 `%s: %v`。

### 3.7 简洁档不打的既有字面量

`outcome %s  agent %s`；`新开了一条 provider 会话 %s`；`provider 会话 %s`；`seq %d  %s  %s`；`writeAskMeter` 的全部（`用量 …`、`金额 …`、`金额未计（…）`、`金额未报`、`冷暖 …`、`意外冷启动：…`）；`发言顺序按字典序，重复的 id 已去重，只问一次。`；`扇出期间 REPL 不接受任何命令，…`；带 `--go` 时的预算表；`renderSettlement` 的整块（N>0 的「分不清」那一行除外）；`已记下 seq %d`；`点名了 %s（用 /ask %s 让他说话）`；TUI 的段终态块（`endBlockLines`）。

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；主代理落地后逐条跑，**从字节副本还原，不用 `git checkout`**。表中全部变异都在原型上实跑过（锚点恰好一次、可编译），点名的杀手确实红了；「K / R / T / A / L」指 §2 的用例（R11 在 §2.2a，L1 在 §2.5）。#1-#55 是 v1 原型（`co1_draft`）上跑的；v1.1 在 `co1_v11` 上重跑了改写后的 #29，并跑了新增的 #56-#76；W1b 修订在 `co1_fix` 上跑了新增的 #77-#83，并重跑了 #62、#76——括号里「实测保持绿」的是同一变异下点名不该红的对照（证明杀手杀的是那个取值，不是连带）。#62 与 #83 没有杀手（都要一个起不来的裁判才走得到，夹具已备好、补不补待主代理，§7.4 第 1 条），见下面「只在审查时看」。

变异行的写法（v1.1 起，F4）：写全替换前后的文本，`原文` → `替换文`；`\t` 是一个 tab、`\n` 是一个换行（Go 字符串字面量写法；#66、#67 里 printf 字面量内的 `\n` 是源码里的两个字符，已在行内注明）；表格里的 `\|` 是 markdown 对 `|` 的转义，源码里是 `|`。锚点在该文件里恰好出现一次（原型核过）。**锚点里的局部变量名**（v1.1 W1b 修订）：#59、#60 的 `noDelta`，#61、#62 的 `settled`，#63-#65 的 `judgeRan`，是原型取的名字；§1.1 E4、E6 允许实现方另取（「局部变量名实现方定」）。集成员按真文件里的名字改写这几条锚点，只换名字、其余逐字，照旧先核恰好一次。（把这三个名字改成冻结要动 §1.1 的文字，待主代理，§7.4 第 2 条。）

| # | 变异（可编译） | 预期被谁杀 |
|---|---|---|
| 1 | ask.go：简洁档名字直接用 `rec.AgentID` | K1、K3、K4、K5、R1、R3、R5、R6、R7、R8、R9、A1 |
| 2 | verbosity.go：`displayName` 去掉 `ac.DisplayName != ""`（空名不回落） | K1（`post_rejected…`、`spoke_fallback_id`）、R1、R3 |
| 3 | verbosity.go：`FormatWall` 用 `fmt.Sprintf("%.1fs", float64(ms)/1000)` | K1、K4、既有 RB-B K1 / K4 |
| 4 | verbosity.go：`ms < 60000` 改 `ms <= 60000` | K1（`spoke_truncated`）、K4 |
| 5 | verbosity.go：负数不夹紧（`if ms < -1<<40`） | K4、既有 RB-B K4 |
| 6 | ask.go：空正文成员头写成 `head + " \n"` | K1 `spoke_empty` |
| 7 | ask.go：简洁档不打 `completionNote` | K1 `spoke_empty` / `spoke_truncated` |
| 8 | ask.go：失败行用 `outcomeLabel`（英文） | K1、K3、R6、R8 |
| 9 | ask.go：MaybeAdvanced 那一行只在详细档打 | K1 `timeout…` |
| 10 | ask.go：漂移行只在详细档打 | K1 `post_rejected…` |
| 11 | ask.go：被拒工具调用行只在详细档打 | K1 `post_rejected…` |
| 12 | ask.go：PostRejected 整段只在详细档打 | K1、K3 |
| 13 | ask.go：简洁档也打 provider 会话行 | K1 `spoke_rich`、R1、R3、R5、R6、R7、R9 |
| 14 | ask.go：简洁档也打 seq / 正文块（`rec.Message != nil` 去掉 `&& !r.concise`） | K1、K4、K5、R1、R3、R5、R6、R7、R9 |
| 15 | ask.go：`writeAskMeter` 在简洁档不早退 | K1、K3、K4、K5、R1、R3、R5、R6、R7、R9、A1 |
| 16 | round.go：预算表门写成 `if !r.concise`（预览也藏） | R4 |
| 17 | round.go：预算表门写成恒真（`--go` 也印） | R1、R3、R5、R6 |
| 18 | round.go：简洁档不打 no_delta 行 | R5 |
| 19 | round.go：no_delta 行打全量 `plan.Skipped`（`if true`） | R5 `mixed` |
| 20 | round.go：「扇出期间」两档都打 | R1、R3、R5、R6 |
| 21 | round.go：「发言顺序」两档都打 | R1、R3、R4、R5、R6、T2 |
| 22 | round.go：简洁档也打整块结算 | R1、R3、R5、R6 |
| 23 | round.go：「分不清」那一行判据写成 `> 1` | R6 |
| 24 | round.go：「分不清」那一行判据写成 `>= 0`（N=0 也打） | R1、R3、R5 |
| 25 | round.go：总用时去掉 `spawned` 判据（`(!spawned && false)`） | R4、R5 `only` |
| 26 | round.go：总用时去掉 `r.roundObs != nil` 判据 | R3、T2 |
| 27 | round.go：总用时去掉 `!r.concise` 判据 | R2、既有 `TestRbaR1NilObserverGolden` |
| 28 | round.go：总用时去掉 `err != nil` 判据 | R10 |
| 29 | round.go：总用时用 `FormatElapsed`（v1.1 写全，F4）：`FormatWall(int(r.clock().Sub(start)/time.Millisecond))` → `FormatElapsed(r.clock().Sub(start).Truncate(time.Millisecond))`（照字面写成 `FormatElapsed(r.clock().Sub(start))` 会删掉 round.go 里 `time` 的唯一用处，编译不过） | R1、R5、R6、A1 `default`（v1.1 原型逐条实测） |
| 30 | round.go：总用时用 `time.Since(start)`（不经注入的时钟） | R1、R5、R6 |
| 31 | round.go：裁判轮起进程不记 `spawned`（`if judgeRan && false`） | R5 `judge_only` |
| 32 | round.go：发言轮起进程不记 `spawned`（删 E3 那一行） | R5 `mixed`、R6 |
| 33 | round.go：`RoundEnd` 不带 `Concise` | R3、T2 |
| 34 | roundobs.go：`RoundEnd` 忽略 `Concise`（`if true`） | T1、T2 |
| 35 | roundobs.go：简洁档连 `roundEndMsg` 也不发 | T1、T2 |
| 36 | app.go：行式分支不调 `SetConcise` | A1 |
| 37 | app.go：行式分支写成 `SetConcise(opts.Verbose)`（反了） | A1、既有 `TestS10aSettleFailureBlocksAskUntilSettled` |
| 38 | commands.go：「已记下 seq」两档都打 | R7、A1 |
| 39 | commands.go：「点名了」两档都打 | R7、A1 |
| 40 | commands.go：「不是当前会话的成员」在简洁档也藏掉 | R7 |
| 41 | verbosity.go：不带参数时设成详细而不是切换 | R9 |
| 42 | verbosity.go：`on` 设成简洁 | R9 |
| 43 | verbosity.go：`on` 接受多余参数（`len(args.fields) >= 1`） | R9 |
| 44 | commands.go：命令表里没有 `verbose`（改名） | R9 |
| 45 | ask.go：「正在问」在简洁档不打 | R7、R8、R9 |
| 46 | ask.go：`/ask` 的原因行在简洁档不打 | R8 |
| 47 | round.go：`/round <id>:` 原因行在简洁档不打 | R6 |
| 48 | round.go：「这一轮没有人可问。」在简洁档不打 | R5 |
| 49 | round.go：「谁没被问」（`renderUnaskedJudges`）在简洁档不打 | R5 `mixed` |
| 50 | round.go：开头的分类跳过（`renderSkipped`）在简洁档不打 | R5 `mixed` |
| 51 | ask.go：成员头用时用 `FormatElapsed` | K1、K4、K5、R1、R3、R5、R6、R7、R9 |
| 52 | ask.go：失败行也带用时 | K1、K3、R6、R8 |
| 53 | round.go：简洁档「分不清」那一行另写一份字面量（`"分不清进程是否起来 %d\n"`） | R6 |
| 54 | round.go：「本轮问」在简洁档不打 | R1、R3、R5、R6、R10、T2 |
| 55 | commands.go：「只有 X 会看到这条」在简洁档不打 | R7 |
| 56 | app.go TUI 分支不调 `SetConcise`（F0 的 X1）：`\t\tr := repl.New(cfg, svc, rn, in, lw, rulesCtx)\n\t\tr.SetConcise(!opts.Verbose)\n` → `\t\tr := repl.New(cfg, svc, rn, in, lw, rulesCtx)\n` | A1 `tui`（`tui_verbose` 实测保持绿） |
| 57 | app.go TUI 分支写反（F0 的 X2）：同一锚点，第二行换成 `\t\tr.SetConcise(opts.Verbose)\n` | A1 `tui`、A1 `tui_verbose` |
| 58 | app.go TUI 分支写死简洁：同一锚点，第二行换成 `\t\tr.SetConcise(true)\n` | A1 `tui_verbose`（`tui` 实测保持绿） |
| 59 | round.go：no_delta 行只在第 1 轮打（F1 的 X7）：`\tif r.concise {\n\t\tvar noDelta []turn.SkipReason` → `\tif r.concise && k <= 1 {\n\t\tvar noDelta []turn.SkipReason` | R5 `round2`（`judge_nodelta` 实测保持绿：裁判轮 `k = 0`） |
| 60 | round.go：no_delta 行只在发言轮打（F1 的 X12）：同一锚点 → `\tif r.concise && label == "发言轮" {\n\t\tvar noDelta []turn.SkipReason` | R5 `judge_nodelta`（`round2` 实测保持绿） |
| 61 | round.go：简洁档「分不清」那一行只在第 1 轮打：`\t\tif settled.SpawnFailed > 0 {` → `\t\tif settled.SpawnFailed > 0 && k <= 1 {` | R5 `round2`（R6 实测保持绿） |
| 62 | round.go：简洁档「分不清」那一行只在发言轮打：同一锚点 → `\t\tif settled.SpawnFailed > 0 && label == "发言轮" {` | **无**：原型实测 repl / tui / app 三包全部 cox 用例都绿。走到它要一个起不来的裁判（grok 裁判），本刀不造（裁决）；列在下面「没有用例能抓」，登记 §7。W1b 修订：命令路径不存在的 grok 裁判不起任何真 CLI、不花钱，原型实测 §7.4 那个夹具在 #62 下红、R5 `round2` 与 R6 照绿；补不补待主代理（§7.4 第 1 条） |
| 63 | round.go：裁判轮没起进程也记 `spawned`（F2 的 X13）：`\tif judgeRan {\n` → `\tif judgeRan \|\| true {\n` | R4 `with_judge`、R5 `judge_nodelta` |
| 64 | round.go：同上的另一写法（F2 的 X8）：`\tif judgeRan {\n` → `\tif judgeRan \|\| !goRun {\n` | R4 `with_judge`（R5 `judge_nodelta` 实测保持绿） |
| 65 | round.go：「带 --go 就当起过进程」：`\tif judgeRan {\n` → `\tif judgeRan \|\| goRun {\n` | R5 `judge_nodelta`（**依赖**：这是 `--go` 下裁判没起进程的唯一夹具；R4 `with_judge` 实测保持绿） |
| 66 | round.go：「已跑 k/N」发言轮循环那一处（`:192`）在简洁档不打（F3 的 X5）：`\n\t\t\treturn r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)` → `\n\t\t\tif r.concise {\n\t\t\t\treturn nil\n\t\t\t}\n\t\t\treturn r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)`（锚点里的 `\n` 在 printf 字面量内是源码里的两个字符 `\` `n`，其余是换行；三个 tab 那一处只有 `:192`） | R11 `canceled`（`canceled_judge` 实测保持绿） |
| 67 | round.go：「已跑 k/N」裁判轮之前那一处（`:223`）在简洁档不打：`\n\t\treturn r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)`（换行后恰好两个 tab，只有 `:223`）→ `\n\t\tif r.concise {\n\t\t\treturn nil\n\t\t}\n\t\treturn r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)` | R11 `canceled_judge`（`canceled` 实测保持绿） |
| 68 | repl.go：`reportErr` 在简洁档不打（F3 的 X11）：`func (r *REPL) reportErr(what string, err error) error {\n\treturn r.printf(` → `func (r *REPL) reportErr(what string, err error) error {\n\tif r.concise {\n\t\treturn nil\n\t}\n\treturn r.printf(` | R11 `persist_fail` |
| 69 | round.go：`:82` 的拒绝在简洁档不打：`if roundsSet \|\| i+1 >= len(args.fields) {\n\t\t\t\treturn r.printf(` → `if roundsSet \|\| i+1 >= len(args.fields) {\n\t\t\t\tif r.concise {\n\t\t\t\t\treturn nil\n\t\t\t\t}\n\t\t\t\treturn r.printf(` | R11 `refusals/rounds_no_value`（`rounds_not_int` 实测保持绿） |
| 70 | round.go：`:86` 的拒绝在简洁档不打：`fmt.Sprintf("%d", rounds) != args.fields[i+1] {\n\t\t\t\treturn r.printf(` → `fmt.Sprintf("%d", rounds) != args.fields[i+1] {\n\t\t\t\tif r.concise {\n\t\t\t\t\treturn nil\n\t\t\t\t}\n\t\t\t\treturn r.printf(` | R11 `refusals/rounds_not_int`（`rounds_no_value` 实测保持绿） |
| 71 | round.go：`:95` 的拒绝在简洁档不打：`if rounds < 1 {\n\t\treturn r.printf(` → `if rounds < 1 {\n\t\tif r.concise {\n\t\t\treturn nil\n\t\t}\n\t\treturn r.printf(` | R11 `refusals/rounds_zero`（`rounds_no_value` 实测保持绿） |
| 72 | round.go：`:100` 的拒绝在简洁档不打：`if rounds > 1 && !goRun {\n\t\treturn r.printf(` → `if rounds > 1 && !goRun {\n\t\tif r.concise {\n\t\t\treturn nil\n\t\t}\n\t\treturn r.printf(` | R11 `refusals/rounds_without_go` |
| 73 | round.go：`:126` 的拒绝在简洁档不打：`if len(unknown) > 0 {\n\t\t\treturn r.printf(` → `if len(unknown) > 0 {\n\t\t\tif r.concise {\n\t\t\t\treturn nil\n\t\t\t}\n\t\t\treturn r.printf(` | R11 `refusals/unknown_member` |
| 74 | round.go：`:141` 的拒绝在简洁档不打（F3 的 X14）：`if len(ids) == 0 {\n\t\treturn r.printf(` → `if len(ids) == 0 {\n\t\tif r.concise {\n\t\t\treturn nil\n\t\t}\n\t\treturn r.printf(` | R11 `refusals/empty_roster` |
| 75 | round.go：argv 整轮拒绝在简洁档不打（F3 的 X6）：`if msg := argvRefusal(rows); msg != "" {\n` → `if msg := argvRefusal(rows); msg != "" {\n\t\tif r.concise {\n\t\t\treturn false, nil\n\t\t}\n` | R11 `argv_refusal` |
| 76 | main.go：flag 改名（F5）：`fs.Bool("verbose", false,` → `fs.Bool("verbos", false,` | L1（第一条断言红；W1b 修订重跑） |
| 77 | round.go：取消之后不打总用时（W1b major 1）：`\t\tif err != nil \|\| !spawned \|\| !r.concise \|\| r.roundObs != nil {\n` → `\t\tif err != nil \|\| !spawned \|\| !r.concise \|\| r.roundObs != nil \|\| ctx.Err() != nil {\n` | R11 `canceled_line`、`canceled_judge_line`（`canceled`、`canceled_judge` 实测保持绿：装了观察者，defer 在 `r.roundObs != nil` 处先返回） |
| 78 | round.go：`:192` 的「已跑 k/N」只在装了观察者时打（W1b major 1 的第二个变体）：`\n\t\t\treturn r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)` → `\n\t\t\tif r.concise && r.roundObs == nil {\n\t\t\t\treturn nil\n\t\t\t}\n\t\t\treturn r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)`（锚点同 #66；printf 字面量内的 `\n` 是源码里的两个字符） | R11 `canceled_line`（`canceled`、`canceled_judge_line` 实测保持绿） |
| 79 | round.go：`:223` 的同型门：`\n\t\treturn r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)` → `\n\t\tif r.concise && r.roundObs == nil {\n\t\t\treturn nil\n\t\t}\n\t\treturn r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)`（锚点同 #67） | R11 `canceled_judge_line`（`canceled_judge`、`canceled_line` 实测保持绿） |
| 80 | round.go：`:105` 的 `reportErr("round", err)` 在简洁档不打（W1b minor 3）：`\tparts, err := r.svc.Participants(ctx, r.cur.ID, false)\n\tif err != nil {\n\t\treturn r.reportErr("round", err)\n` → `\tparts, err := r.svc.Participants(ctx, r.cur.ID, false)\n\tif err != nil {\n\t\tif r.concise {\n\t\t\treturn nil\n\t\t}\n\t\treturn r.reportErr("round", err)\n` | R11 `round_report_err`（`persist_fail` 实测保持绿） |
| 81 | main.go：`-verbose` 默认改成 true（W1b major 2）：`fs.Bool("verbose", false,` → `fs.Bool("verbose", true,` | L1 第三条（前两条实测照过） |
| 82 | main.go：说明文字改一个字（W1b major 2）：`（默认简洁；进去之后` → `（默认精简；进去之后` | L1 第三条（前两条实测照过） |
| 83 | round.go：简洁档的 `/round <id>:` 原因行只在发言轮打（W1b major 3 顺带；#47 的同类）：`\t\tif rec.Err != nil {\n\t\t\tif err := r.printf("/round %s: %s\n", rec.AgentID, askCause(rec.Err)); err != nil {\n` → `\t\tif rec.Err != nil && (!r.concise \|\| label == "发言轮") {\n\t\t\tif err := r.printf("/round %s: %s\n", rec.AgentID, askCause(rec.Err)); err != nil {\n` | **无**：原型实测 R1、R5 五个、R6、R11 各子测试都绿（R6、R5 `round2` 的原因行都在发言轮；其余用例的裁判轮没有失败的成员，走不到这一行），只有 §7.4 待裁的 `judge_spawn_failed` 红。同 #62，列在下面「只在审查时看」，登记 §7 |

**判定为等价、不列杀手的变异**（写在这里，免得被当成漏测）：
- 成员头的判据写成 `rec.Outcome == turn.OutcomeSpoke` 而不是 `rec.Message != nil`：`Record.Message` 的注释是「Outcome 不是 OutcomeSpoke 时为 nil」，成功路径上两者同真同假（原型实测全部用例绿）。冻结 `Message != nil` 是为了不在 `Message` 为 nil 时解引用。
- `start := r.clock()` 挪到参数解析之后、`TakeRoundSnap` 之前：调用次数不变，差的只是几微秒。冻结为第一条语句。
- `writeFullText(r, "")` 与「空正文时不调」：`writeFullText` 对空串直接返回。

**只在审查时看（没有用例能确定地抓，集成员收割时逐字核对）**（v1.1：原第 1 条「TUI 分支的 `SetConcise`」由 §2.4 A1 `tui` / `tui_verbose` 接管、删去；原第 3 条「已跑 k/N 与 argvRefusal」由 §2.2a R11 接管、删去；第 2 条收窄；W1b 修订加 #83 与其余五处 `reportErr("round", …)` 两条）：
- **#62，裁判轮简洁档「分不清」那一行**（v1.1 新增，F1）：§1.1 E6 冻结的判据是 `if settled.SpawnFailed > 0 {`，没有 `label` / `k` 条件；第 2 个发言轮那一侧由 R5 `round2` 钉住（#61），裁判轮那一侧没有夹具（要一个起不来的裁判，本刀不造，§7.2），收割时核对这一行没有被加上发言轮判据。W1b 修订：夹具与期望已备好，补不补待主代理（§7.4 第 1 条）；采纳之后这一条删去。
- **#83，简洁档裁判轮的 `/round <id>:` 原因行**（W1b 修订新增）：§1.1 E6 说回调里 `"/round %s: %s\n"` 那一行不改、简洁档照打；发言轮那一侧由 R6 与 R5 `round2` 钉住（#47），裁判轮那一侧同 #62 没有夹具。收割时核对这一行没有被加上档位或 `label` 判据。与 #62 同一个待裁夹具（§7.4 第 1 条）。
- **`cmdRound` / `runOneRound` 里其余五处 `reportErr("round", …)`**（`round.go:131`、`:148`、`:199`、`:227`、`:262`；W1b 修订新增）：只有 `:105` 由 R11 `round_report_err` 钉住（#80）。`:199`、`:227`、`:262` 前面都有一道 `ctx.Err()` 检查（`:191` 或 `:222`），取消先在那里变成「已跑 k/N」；`:131`、`:148` 与它前面那次成功的 `Participants` 调用之间没有打印点、也没有事件，取消的时刻卡不进去；都要往存储层注错才走得到，本刀不造。收割时核对这五处调用点前面没有加任何档位门（§1.1 E9：`reportErr` 两档都打）。
- `cmd/council/main.go` 里 `Verbose: *verbose` 这一处传递（v1.1 收窄，F5）：flag 是否登记、默认值与说明文字已由 §2.5 L1 钉住（默认值与说明文字是 W1b 修订加的第三条）；`run` 过了 `-version` 之后硬编码进程标准流、注册进程级信号，传递这一步仍不可单测，收割时逐字核对，真机复验 §6 第 5 步之 5、之 6 看。
- 名字经 `r.write` 拼接、不进 `printf` 格式串（`display_name` 里可能有 `%`；变异会被 `go vet` 的 printf 检查在 `go test` 编译期挡住）。
- `internal/turn` 一个文件都没改（`git diff --stat` 里没有 `internal/turn/`），送给模型的 prompt 一个字节不变。
- `tui` 的 `outcomeWord` / `wallText` / `formatElapsed` 三个函数体**真的只剩委托**（`return repl.X(...)`），没有留着旧函数体：留一份旧副本输出逐字节相同，任何用例都抓不到，但那就是「两份格式」，只能看 diff。`verbosity.go` 里的三份函数体必须是从 `roundobs.go` 搬过来的原样逻辑。
- 各处注释要点齐全、没有与代码不符的话；`app.go` 的「行式路径一个字不改」已删；`RoundEndEvent` 的注释写了简洁档下结算不打。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许改这 10 个文件：生产 9 个 —— 新文件 `internal/repl/verbosity.go`；修改 `internal/repl/repl.go`、`internal/repl/observer.go`、`internal/repl/ask.go`、`internal/repl/round.go`、`internal/repl/commands.go`、`internal/tui/roundobs.go`、`internal/app/app.go`、`cmd/council/main.go`；授权的既有测试 1 个 —— `internal/app/app_test.go`（§1.2，只加 `Verbose:    true,` 那一行）。不改 `go.mod`。生产改动原型实测约 190 行（新文件约 110 行），一次交付，不拆 A / B。
- 补丁块，每个文件可以有多个：

  ```
  ===PATCH: internal/repl/round.go===
  <<<<ANCHOR
  （从 HEAD ab53c0b 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点必须从 HEAD `ab53c0b` 的真文件逐字复制（本文代码块里的缩进可能被排成空格，以真文件为准），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。注意：`round.go` 里 `if err := r.printf("%s", text); err != nil {` 出现不止一次，「发言顺序」「扇出期间」「本轮问」各自的 `if err := r.printf(` 行要带上字面量本身当锚点；`ask.go` 里 `if note := completionNote(rec); note != "" {` 在新函数里会出现两次，D1 请整函数替换（锚点取 HEAD `:319` 到 `:373` 的原文，或拆成互不重叠、各自恰好一次的几段）。
- 新文件用 `===FILE: internal/repl/verbosity.go===` 换行接完整内容、末尾 `===END FILE===`。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- 代码必须能 `gofmt`（tab 缩进）；`go vet` 不许有新报告。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（含 U+FF08 / U+FF09 全角括号、U+FF1A 全角冒号、U+FF1B 全角分号、U+3001 顿号、U+300C / U+300D 直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing（`/to` 那句里既有的 `——` 是 U+2014，放行）。本刀新增的字符串字面量只有 §3.1-§3.5 与 `writeAskRecord` 里的 `"（"`、`"）:"`、`": 没有回复（"`、`"）\n"`。
- 生产代码新标识符不许以 `cox` 开头（那是测试腿的前缀）。
- §1.1 里给出的签名、判据、代码块照抄；「名字实现方定」处自取。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：10 个文件是否都给了；§1.1 A-I 与 §1.2 那一处各落在哪个补丁块。
  位置：cmdRound 的 defer 在哪一行之后（贴出锚点的第一行）；runOneRound 的 no_delta 块插在哪两行之间。
  注释：app.go「行式路径一个字不改」是否已删；RoundEndEvent、runOneRound、cmdRound（含给 SM 留的位置）、writeAskRecord、writeAskMeter、postUserText、dispatch 注释、roundobs.go 三个委托函数的注释是否按注释要点改了。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  ```

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/*`、`cmd/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；按锚点落盘；`gofmt -l`；`go vet ./internal/repl ./internal/tui ./internal/app ./cmd/council`；`go build ./...`。逐字核对：§3.1-§3.5 字面量逐字节；`cmdRound` 的 defer 四个判据；`writeAskRecord` 详细档与 HEAD 的差异只在门上（`git diff` 看）；`app_test.go` 只多一行；`internal/turn` 零改动。
2. **核对测试腿报告里 `cox_record_verbose_test.go` 在 HEAD 拷贝上单独跑的结果**（K2 八个子测试全绿，与原型实测一致）。**合并测试腿的 7 个文件**之后，跑 `./internal/repl ./internal/tui ./internal/app ./cmd/council`（另一条流水线也可能在跑 go test；只跑自己的包，3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制），再跑一次全量 `./...`。既有用例必须全绿（原型实测全绿）。A1 `tui` / `tui_verbose`、R5 `round2`、R11 `canceled` / `canceled_judge` / `canceled_line` / `canceled_judge_line` 是时序相关的，合并后各跑一次 `-count=10`（原型零偶发）。
3. **§4 的变异逐条跑**（#1-#83，#62 与 #83 除外——它们没有杀手，只核对确实仍然存活、§7 已登记；主代理若采纳 §7.4 第 1 条，这两条改为照跑、杀手是那个新子测试；#59-#65 的锚点按真文件的局部变量名改写，见 §4 开头；从字节副本还原）；任何一条没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
4. **文档**：
   - `docs/00_DECISIONS.md` #45：第二节标「简洁输出与用时已落地（commit），自动总结未落」；记下本刀的取舍（§1.2 与 §7 open items：总用时只在 `/round` 真起过进程时打（主代理定，#45 原话是每条命令一行）、详细档行式不打总用时、「正在问 / 本轮问」保留 id、PostRejected 在简洁档的头是「没有回复（未入库）」、TUI 与行式的总用时格式不同、裁判轮「分不清」那一行与 `/round <id>:` 原因行的门没有用例（#62、#83；§7.4 第 1 条若被采纳则有）、`reportErr("round", …)` 其余五处调用点没有用例）；§7 的「待 Griffin」四条原样搬过去。
   - `docs/15b_RUN_BOARD_RBB_CONTRACT.md` 落地后订正区追加一条：`RoundEndEvent` 多了 `Concise`（RB-B §1.1 A 的结构体冻结被本刀追加一个字段）；`RoundEnd` 方法体在简洁档不发块（RB-B C1）；`outcomeWord` / `wallText` / `formatElapsed` 的函数体搬到 `repl`（RB-B C4、RB-A D 的 `formatElapsed`），名字与字节不变。
   - `docs/09a_STEP6_API_CONTRACT.md` 或 REPL 的 API 契约：登记 `SetConcise`、`FormatWall`、`FormatElapsed`、`OutcomeWord`、`RoundEndEvent.Concise`、`app.Options.Verbose`、`/verbose` 命令、`-verbose` 启动参数。
   - `docs/README.md`：用法里加 `-verbose` 与 `/verbose`。
   - 本契约归档到 `docs/`（照 `13a` / `15b` 的体例加归档说明）。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的真机复验步骤**（Windows Terminal + PowerShell 5.1；用一次性的 `data_dir` 与一份只含两三个便宜成员的配置，不碰日常库；**会真的调用模型、花钱**，每步开始前由 Griffin 决定跑不跑）。council 内的命令**一行一块，逐块粘贴、逐块回车**（#44：整块粘贴多行会被当成一条提交）。
   1. 在 PowerShell 里构建并看帮助：`council -h` 的输出里有 `-verbose` 与 §3.5 那句说明。
   2. 直接 `council`（进 TUI，默认简洁）。依次输入下面每一块：

      ```
      /new 简洁复验
      ```

      ```
      /add <成员一>
      ```

      ```
      /add <成员二>
      ```

      ```
      /round <成员一> <成员二> --go
      ```
      看：每个成员一行「<显示名>（<N.Ns>）: 正文」，谁先回来先打谁；没有预算表、没有「扇出期间」、没有 outcome / seq / 用量 / 金额 / 冷暖、没有结算块、没有「发言轮 (xxxxxx)  回来 2/2」那一块；最后恰好一行 `/round 结束  Ns  回来 2/2`，**没有**「总用时」。
   3. 同一个 TUI 里：

      ```
      /verbose
      ```
      看到「已切到详细输出：…」。再：

      ```
      /round <成员一> <成员二> --go
      ```
      看：今天的详细输出原样回来（预算表、outcome 行、用量、金额、结算、段终态块、收尾行）。再：

      ```
      /verbose off
      ```
      看到「已切到简洁输出：…」。

      ```
      /ask <成员一> 再说一句
      ```
      看：「正在问 <id> ...」然后「<显示名>（N.Ns）: …」，没有「已记下 seq」。

      ```
      /help
      ```
      看：有 `/verbose` 那一行。

      ```
      /quit
      ```
   4. `council -no-tui`（行式，默认简洁）：

      ```
      /use <上面那个会话的 id 前缀>
      ```

      ```
      /round <成员一> <成员二> --go
      ```
      看：成员头同 TUI；最后一行 `总用时 N.Ns`（恰好一行）。

      ```
      /quit
      ```
   5. `council -no-tui -verbose`：

      ```
      /use <会话 id 前缀>
      ```

      ```
      /round <成员一> <成员二> --go
      ```
      看：今天的详细输出，没有「总用时」。

      ```
      /quit
      ```
   6. `council -verbose`（TUI、详细）：`/round …` 与第 3 步的详细输出相同（确认 `-verbose` 经 `Verbose: *verbose` 传进了 `app.Options`：这一段传递是 §4「只在审查时看」的那一条；TUI 分支按 `Options.Verbose` 设档位已由 §2.4 A1 `tui_verbose` 钉住）。
6. **清理**：本刀在中转根（`E:\multi_model_cowork\`）与 scratchpad 里的全部中间物逐个删除。

---

## 7. Open items（v1.1 新增）

### 7.1 主代理已定（不再等人裁）

- **总用时只在 `/round` 真起过进程时打**（F6；主代理定，#45 原话是「每条命令一行」）：预览、没人可问、被拒都不打；起过进程之后被取消照打（W1b 修订写明，是 §1.1 E2 判据的推论，与 TUI 收尾行 `/round 已取消  …` 对应；R11 `canceled_line` 钉住）；`/ask` 靠成员头里的用时。与 TUI 收尾行的门（`runSeen`）一致。见 §0 第 8 条。
- 已定的 v1 取舍，列出供对照：TUI 的「总用时」是 RB-B 收尾行，用 `FormatElapsed`（整秒）；行式的 `总用时` 用 `FormatWall`（一位小数）；两处格式不同，收尾行的字面量本刀不许改。

### 7.2 已知没有用例、本刀不补

- **裁判轮的简洁档「分不清进程是否起来 N（…）」那一行**（§4 #62）：把门写成只在发言轮打，原型实测全部 cox 用例都绿。走到它要一个起不来的裁判（例如 grok provider、命令路径不存在、`ModeJudge` 的成员），本刀按裁决不造；代码是 §1.1 E6 逐字冻结的，没有 `label` 判据，收割时逐字核对。补法与原型实测的整串期望已备好，见 §7.4 第 1 条（W1b 修订，待主代理）。
- **裁判轮简洁档的 `/round <id>:` 原因行**（§4 #83，W1b 修订新增）：门成只在发言轮打，原型实测 R1、R5、R6、R11 都绿（其余用例的裁判轮没有失败的成员，按代码走不到，未实跑）；与上一条同一个待裁夹具。
- **`cmdRound` / `runOneRound` 里其余五处 `reportErr("round", …)`**（`round.go:131`、`:148`、`:199`、`:227`、`:262`，W1b 修订新增）：用取消走不到，要往存储层注错，本刀不造；§4「只在审查时看」。
- `cmd/council/main.go` 里 `Verbose: *verbose` 的传递（§4「只在审查时看」，真机复验 §6 第 5 步之 5、之 6）。

### 7.3 待 Griffin（只登记，本刀照 v1 做）

- PostRejected 在简洁档的头是「<名>: 没有回复（未入库）」，后面紧跟「正文写不进库（已经付过钱），整段如下:」与整段正文；「没有回复」与后面有正文，措辞有张力。
- 简洁档下 `/say` 与裸文本成功时屏幕上什么都不打（「已记下 seq N」按 #45 藏掉），用户可能不确定有没有记下。
- 「正在问 X ...」「本轮问 N 个: ...」保留 id，没换显示名。
- 详细档的行式模式不打总用时（保持今天的输出逐字节不变；RB-A 的 `TestRbaR1NilObserverGolden` 钉着它）。

### 7.4 待主代理（v1.1 W1b 修订提出；本修订取保守读法，没有进 §1.1、§2 与 §4 的杀手列）

**主代理裁定（2026-09-28 05:30）：两条都采纳，以本段为准，覆盖契约别处与之冲突的说法。**
- 第 1 条：`judge_spawn_failed` 是 §2.2 R5 的**第六个子测试**，照下面第 1 条的配置、规范化、整串期望原样写（测试腿交付）。§4 #62、#83 的杀手改为 R5 `judge_spawn_failed`（集成员照跑，不再是「无」）；§4「只在审查时看」里 #62、#83 两条与 §7.2 的对应条目作废；§6 第 3 步「#62 与 #83 除外」作废。SM1 契约 §1.2 / §8 的「有条件的第 6 处授权」因此生效。
- 第 2 条：§1.1 E4、E6 的三个局部变量名冻结为 `judgeRan`、`noDelta`、`settled`（三处文字已改）。§4 #59-#65 的锚点照原样用，§4 开头「集成员按真文件的名字改写锚点」那一句作废。

1. **补 `judge_spawn_failed`，让 #62、#83 有杀手**（W1b major 3；W1b 原话「请主代理重裁」）。裁决书 F1 的「本刀不造」是按「要一个走 grok 的裁判」的成本定的；实测不花钱：沿用 `cox_x` 的做法（grok provider、命令路径不存在），再设成裁判，不起任何真 CLI，原型 0.07 秒。本修订没有自己推翻裁决。下面是备好的一条，采纳即可原样落进 §2.2 R5（第六个子测试），并改 §4 #62、#83 的杀手列，删去「只在审查时看」与 §7.2 的对应条目：
   - 配置：`coxRoundCfg()` 的一份拷贝，加 `cox_y`：复制 `cox_x`，`DisplayName` 改成 `"坏裁判"`、`Mode` 改成 `config.ModeJudge`；`cfg` 同时交给 Runner 与 REPL（同 R11 `argv_refusal` 的做法）。成员 `cox_a, cox_y`（`cox_y` 以 `"grok"` 加入）；不要前置；简洁档；`r.Submit(ctx, "/round cox_a cox_y --go --with-judge")` 返回 nil。
   - 规范化：先守卫原始输出里以 `/round cox_y: ` 开头的行恰好 1 行，再用 `(?m)^(/round cox_y: ).+$` 换成 `${1}<原因>`；其余同通用纪律。
   - 期望：整串等于 `本轮问 1 个: cox_a\n甲（T）: OK\n本轮问 1 个: cox_y\n坏裁判: 没有回复（没起来）\n/round cox_y: <原因>\n分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）\n总用时 7.2s\n`，假时钟恰好 2 次（原型实跑，`-count=15` 零偶发）。原型实测的原因行（供核对）：`/round cox_y: provider cox_y path C:\Users\…\Temp\cox-no-such-dir\cox-missing.exe exit 0 stderr "" stdout "" planned_session <uuid>: fork/exec …\cox-missing.exe: The system cannot find the path specified.`。
   - 【判别力】裁判轮（`label` 为「裁判轮」、`k = 0`）里的失败行、`/round <id>:` 原因行、简洁档唯一照打的结算行都在。#62（「分不清」那一行只在发言轮打）与 #83（原因行只在发言轮打）下这一条红，R5 `round2` 与 R6 照绿（原型实测）；对照是 R6（同样的失败，但在发言轮）。
   - 代价：SM1 的清单多一条「中」（送进 `cox_a`、`cox_y` 两个，走完、没取消），SM1 落地后这一条的整串在 `总用时 7.2s` 之前多一行提示，SM1 修订人要跟着清点。
2. **把 `noDelta`、`settled`、`judgeRan` 三个局部变量名改成冻结**（W1b minor 1 的第一种修法）。好处：§4 #59-#65 的锚点在真文件上照样恰好一次，集成员不用改写锚点；SM1 取 CO1 落地后的真文件做锚点，名字冻结之后更稳。代价：要改 §1.1 E4、E6 里三处「局部变量名实现方定」的文字；裁决书的硬约束是 §1.1 不许动、要动先交主代理裁。本修订只做了第二种修法（§4 开头注明集成员按真文件改写锚点）。
