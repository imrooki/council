# SM1 冻结契约：多成员 /round 之后自动总结（只给用户看）（v1.1，含 W1b 修订，2026-09-28）

> **落地后订正（2026-09-28，主代理；SM1 已于 `a877d44` 落地）**：
> - §2 通用纪律「编译依赖」一句说 app 测试文件同时引用本刀与 CO1 的标识符、只在两刀都落地后才编译得过：不对。`smx_summary_app_test.go` 只引用 CO1 已有的 `app.Options.Verbose`，靠字面 TOML 的 `[summary]` 在运行时依赖本刀（HEAD 上是运行时红，不是编译红）。
> - §1.0 与 §0 第 8 条说「第 1-5 条都不读成员表、多读一次会变红」：只有「不多拍快照」有既有用例担保（两条数 MaxSeq 的用例）；「不读成员表」是设计约束，没有用例数它。落地代码的注释由 fu1 订正。
> - 独立审查余项（代码都没错）：`--rounds N` 时「前几轮都回了、最后一轮全失败」这个取值、总结段写屏幕失败都没有用例；四处注释没跟上（头行漏 `[--no-summary]` 等），由 fu1 补。

> **主代理裁定（2026-09-28 05:30，先读）**：
> - CO1 §7.4 第 1 条已采纳（R5 `judge_spawn_failed` 成为 CO1 的正式用例），所以本契约 §1.2 / §8 的「有条件的第 6 处授权」**生效**：SM1 测试腿交付时照改那一条的期望。
> - CO1 §7.4 第 2 条已采纳：CO1 的局部变量名冻结为 `judgeRan`、`noDelta`、`settled`。本契约取锚点时以 CO1 落地后的真文件为准。
> - 落地顺序：MD1 与 CO1 先落，SM1 最后。SM1 的实现与测试都以落了 MD1、CO1 之后的 main 为基线（锚点、既有用例清单都按那时的真文件重取、重跑）。MD1 在 `internal/turn` 加了本包唯一的 TestMain，本刀不许再加。
> - MD1 的 open item (s)（`/add +N` 的写前守卫要对新增的 `Summary` 字段也有一条子测试）**不在本刀**，两刀都落地之后另补。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `28845e0`（代码与 `ab53c0b` 逐字节相同，之后只有文档提交，本文行号仍以 `ab53c0b` 为准；生产代码最后一次改动在 `a2e3d76`，之后只有测试与文档）。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`。本机没有 gcc，不能 `-race`。
- 本刀 KEY 是 `SM1`；测试腿的包级标识符前缀是 `smx`（类型、函数、变量、常量一律 `smx` 开头，测试函数 `TestSmx...`）。生产代码新标识符不许以 `smx` 开头。
- 来源：`docs/00_DECISIONS.md` **#45 第二节**的第 5、6 条（`:2127-2129`：自动总结「每次自动」、`--no-summary`、推翻 `12` §4.7「不自动触发」与 round.go 头注释「自动链入口必须收显式上限」、代价照实写、总结只给 Griffin 看、总结成员看不到失败所以由 Council 注明），以及第 1 条的例句「总的来看（Fable 5.1 做总结）：…」（`:2121`）。方向（Griffin 已裁）不改；本契约只把没裁的细节冻结。
- **本刀建立在 CO1 v1.1 之上**（`E:\multi_model_cowork\co1_contract.md`，简洁输出档位 + 用时；v1.1 的 §1.1 与 v1 一字不差，只多了测试）。**落地顺序必须是 CO1 → SM1**：SM1 的实现腿在 CO1 落地之后才派工，`internal/repl/round.go`、`ask.go`、`observer.go`、`commands.go` 四个文件的锚点取 **CO1 落地后的真文件**（主代理派工时把这四个文件的全文交给实现腿）；`internal/config/*`、`internal/turn/runner.go` 两处 CO1 不碰，锚点取 HEAD `ab53c0b` 真文件；`internal/repl/summary.go` 是新文件。本文写的 CO1 形状（`cmdRound` 的命名返回值与 defer、`spawned`、`writeAskRecord` 的简洁档分支、`RoundEndEvent.Concise`）全部以 CO1 契约 §1.1 的冻结代码为准。
- 本契约的全部期望值（屏幕逐字节、TUI 真 Program 上的日志、`app.Run` 的输出与写出的 `council-role.txt` 字节、库里的消息行与 `turn_usage`）已在隔离拷贝 `E:\Isolating_test\council\sm1_draft\` 里实跑得出：先把 CO1 §1.1 A-I 与它授权的那一行 `app_test.go` 照 CO1 契约原样落进 HEAD 拷贝（全量 `go test ./...` 八包全绿），再落本刀 §1.1 全部生产改动与 §2 全部用例的参考实现。原型下 `gofmt -l` 为空、`go vet ./internal/...` 无报告、**全量 `go test -count=1 ./...` 八包全绿，HEAD 既有用例零改动、零变红**；§4 的 42 条变异逐条在原型上跑过（锚点恰好一次、从字节副本还原），41 条被点名杀手杀掉，1 条判为等价（原因见 §4）。原型、CO1 基线副本、变异装置已用 `fs.rmSync` 删除。
- v1.1 的新增与改动（见下节）在另一份原型 `E:\Isolating_test\council\sm1_v11\` 上实跑得出：`git archive 28845e0` 解出；先照 CO1 **v1.1** 落 CO1（§1.1 A-I、授权的那一行 `app_test.go`、§2 的七个 cox 文件；原稿写「八个」，W1b 修订更正，同 CO1 的 W1b 修订），全量 `go test -count=1 ./...` 九包全绿；再照本文 §1.1 A-I 逐字落本刀（**§1.1 一个字没改**），此时 CO1 的用例**恰好五条变红**（§1.2 逐条授权，v1 只列了四条，漏了 CO1 v1.1 新增的 R5 `round2`），按 §1.2 改完期望之后，加本刀 §2 的 v1 用例、再加 v1.1 用例。原型下 `gofmt -l` 为空、`go vet ./internal/... ./cmd/...` 无报告、**全量 `go test -count=1 ./...` 九包全绿**，HEAD 既有用例零改动、零变红；v1.1 新增的时序相关用例（S7 `note_refused` / `rounds2_last_record` / `note_order` / 三个详细档子测试、S8 两个详细档子测试、S13 全部、S3 `with_judge_no_summary`、T1 全部）`-count=10` 零偶发。§4 的 #1-#41 在 v1.1 原型上重跑，全部仍被点名杀手杀掉；v1.1 新增的 21 条（#43-#46、#48、#49、#51、#52、#55、#73-#76、#78-#85）逐条实跑（锚点恰好一次、从字节副本还原并比对字节相等），点名杀手实测变红，点名「不该红」的对照实测保持绿。原型不交付，已删除。
- W1b 修订（见下节末尾的「W1b 修订」小节）在第三份原型 `E:\Isolating_test\council\sm1_fix\` 上实跑得出：`git archive 28845e0` 解出；CO1 §1.1 A-I 与它授权的那一行 `app_test.go` 用锚点脚本落盘（`verbosity.go`、D1、E2、E6 的 no_delta 块与结算块、E8、G1、I 那一行直接从 CO1 契约原字节截取，其余按条文写；每个锚点先核恰好一次），HEAD 既有用例全量 `go test -count=1 ./...` 全绿；再照本文 §1.1 用锚点脚本落本刀（A-G、I 的代码块全部从本文原字节截取；H 与 F6 只改注释，原型没落；**§1.1 一个字没改**），`gofmt -l` 为空、`go vet` 无报告，HEAD 既有用例全量仍全绿。CO1 的用例本修订只重写了与总结相交的那一部分：R1、R2、R3 两个、R4 两个、R5 五个加 CO1 §7.4 待裁的 `judge_spawn_failed`、R6、R11 全部（含 CO1 W1b 修订的 `canceled_line`、`canceled_judge_line`、`round_report_err`，`refusals` 的六个子子测试）、T2 两个、A1 `default` / `verbose`。忠实性先验：它们先在「只落 CO1」的拷贝上全绿，再放到落了本刀的原型上跑，**恰好 §1.2 的五条加 `judge_spawn_failed` 变红**（后者见 §1.2 新加的第 6 条）；按 §1.2 改完期望之后全绿。没有重写的 K1-K5、R7-R10、A1 `tui` / `tui_verbose`、L1 按代码走不到 `runSummary`（单测 `writeAskRecord`、`/ask`、`/verbose`、写屏幕出错提前返回、单成员 `/round`、启动参数），沿用 v1.1 原型与 W1b 的实测。本刀 §2 的 v1.1 用例（C1-C5、K1-K2、S1-S13、T1、A1；P1 没写，W1b 修订不碰 turn 包）照本文写，第一次就逐字节全绿；之后才加 W1b 修订的新夹具，新期望取自这份原型的实跑输出（与 W1b 返回里给的串逐字一致）。原型下 `gofmt -l` 为空、`go vet ./internal/... ./cmd/...` 无报告、全量 `go test -count=1 ./...` 全绿；新夹具（S3 两个、S8 `mention_verbose`、S10 `canceled_verbose`、S14 七个、K1 / K2 的新 Record、C5 两个、CO1 的 `judge_spawn_failed`）`-count=10` 零偶发。§4 的 #86-#97 与重跑的 #12、#13、#14、#18、#21、#24、#25、#27、#73-#76、#78-#81、#84 逐条实跑（锚点恰好一次、从字节副本还原并比对字节相等），范围是 repl / tui / app 三包的 `TestSmx*`、`TestCox*`（上面那个子集）、`TestWithJudge*`，「预期被谁杀」写的是实测红集的全集。原型不交付，已删除。

## v1.1 变更记录（W1 的 F0-F8 与主代理裁决，2026-09-28）

W1：审查员照 v1 忠实实现，§2 全部期望第一次就过，41 条列出的变异全被点名杀手杀掉；发现 9 条：4 条 major（F0-F3，都是自拟变异能活，夹具走不到变异所在的分支或取值）、5 条 minor（F4-F8）。证伪者复核 F0-F3，全部成立，并指出其中几条修法的漏洞；主代理裁决全部接受、凡证伪者更正处按更正做。**§1.1 冻结的生产代码一个字没改**（九条全是测试侧与文字侧的缺口），所以本节没有「§1.1 改动」一项。

- **F0（major，「两档都打」的行只有简洁档夹具）** → §2.4 S7 加 `left_verbose`、`all_fail_verbose`、`note_verbose`（整行相等），S8 加 `named_verbose`、`preview_verbose`（V0 更正：只加 `named_verbose` 挡不住 `(r.concise || goRun)`）；§2.5 T1 加 `verbose_unconfigured`（与 `concise_unconfigured` 同装置、同窗口、同 kinds 串，互为正对照）。§4 新增 #73-#76、#78、#84。
- **F1（major，判据顺序没有双条件夹具）** → 新增 §2.4 S13 `TestSmxRoundSummaryJudgmentOrder`，六个双条件子测试：`single_unconfigured`、`canceled_unconfigured`、`all_fail_unconfigured`（F1 原文）与 `left_single`、`left_canceled`、`left_all_fail`（V1 更正：「已退出」判据的位置没人钉）。§4 新增 #43（杀手写全套，含 CO1 的 A1 `default`、R5 `mixed`）、#44、#45 与「已退出判据挪到第 2 / 3 / 5 条之前」三条（#79-#81）。实测发现一件 V1 没料到的事：真 store 在 ctx 已取消时读成员表会报错，所以「挪到取消之前」（#79、#80）先打出的是 `round: list participants …: context canceled`，不是已退出行；既有 S10 与新的 `canceled_unconfigured` 因此也杀它们，`left_canceled` 的独有判别力只在「读成员表不看 ctx」时才显出来（【判别力】照写）。
- **F2（major，`--with-judge` 与 `--no-summary` 的组合）** → §2.4 S3 加 `with_judge_no_summary`。§4 新增 #48 与 V2 指出的两个反方向变异 #82（`if noSummary && !withJudge`）、#83（`--no-summary` 早退挪到裁判块之前）。
- **F3（major，注明行只走了「没起来」）** → §2.4 S7 加 `note_refused`（按 V3 更正：整串逐字冻结，8 行；慢 a 固定顺序；陈旧在飞记录的 TurnID 是字面量 `smx-stale-turn`；原因行是确定的 `askCause` 文本，没有规范化）。§4 新增 #55，以及 V3 第 4 点的 #85（注明行直接读配置里的显示名、不回落到 id：`smx_b` 显示名为空，只有 `note_refused` 抓得到）；「只在审查时看」加一条：判据必须恰好是 `rec.Message == nil`（`!rec.Outcome.Ingested()` 这种枚举结局的写法漏 PostRejected，原型实测它活过全部 smx / cox 用例）。
- **F4（minor，「取最后一条记录」有现成夹具）** → §2.4 S7 加 `rounds2_last_record`（钩子只触发一次）；「只在审查时看」原第 1 条删去，挪进变异表成为 #46。
- **F5（minor，进程内换 role 之后是每轮都被拒）** → §1.0 代价 (b)(d) 改写成两句；§6 第 6 步之 1（改配置那一步）加同样的提示。原型用一条临时探针核过（不交付）：`rn.Ask` 开出 smx_s 的进程内会话、换一份 smx_s role 不同的配置之后，连跑两条 `/round` 都是「总的来看：没有总结（拒绝）」加 `/round smx_s: 身份或脑子在这条 provider 会话开着的时候被改了；Step 6 不静默重建`，`/session rebuild smx_s` 之后恢复。
- **F6（minor，自动 Join 的字面量没人钉）** → §2.4 S1 断言 smx_s 那一行 `DisplayName == "总结 S"`、`Provider == "claude"`。§4 新增 #51、#52。「同类的还有没有别处」：同一个字面量的 `ConvID`、`JoinedAt` 两个字段也试了，都是等价变异（`Service.Join` 无条件用参数覆盖 `ConvID`；store 对零值 `JoinedAt` 补 `time.Now`），列进 §4 的等价清单。
- **F7（minor，跨段顺序没有夹具）** → 加夹具（没有删注释）：§2.0 `smxCfg` 加一个走 grok（起不来）的裁判 `smx_k`（`坏 K`），§2.4 S7 加 `note_order`，钉住「坏 X（没起来）、坏 K（没起来）」的先后。加 `smx_k` 不牵动任何既有期望（它只在 `note_order` 里被 Join，原型全量实测）。§4 新增 #49。顺带：这条夹具（以及 v1 就有的 S7 `summary_fails`）走到了 CO1 §7.2 说「没有用例能抓」的 CO1 #62（裁判轮 / 非发言轮段的简洁档「分不清」那一行），原型实测 CO1 #62 在本刀落地后被这两条杀掉，见 §4 末尾。
- **F8（minor，相交表引用了不存在的节）** → 新增 §7 open items、§8 existing_tests_changed（从 v1 起草人的报告抄进来，按 v1.1 更新），相交表两处引用改指这两节。
- **与 CO1 v1.1 的协调** → §1.2 的授权清单由四条改成五条（加 CO1 R5 `round2`），并列出实跑核对过不受影响的 CO1 v1.1 新用例。
- **与 MD1 的协调** → §2.1 加 C5 `TestSmxSummaryThenAppendedAgent`（`[summary]` 之后再接一段 `[agents."x"]`，字面 TOML，不调 MD1 的代码）；§2 通用纪律写明本刀不在 `internal/turn` 新增 `TestMain`、也不依赖 MD1 的那个。
- **v1 open items 的裁定** → §7 登记：writeAskRecord 拆分（主代理接受）；总结头不带用时、简洁档总结前照打「本轮问 1 个」、`DefaultSummaryRole` 措辞（待 Griffin）；名单拒绝、「多个成员」的计数法、`/leave` 即停总结（主代理定）；总结成员看到的是上次开口以来的全部新消息（已知现状）。
- 其余连带的改动：标题、仓库行、CO1 基线改成 v1.1；§1.1 E 的注释要点补两处「由哪条用例钉住」的出处与一句「注明行判据恰好是 `rec.Message == nil`」（只改注释要点，代码一字未改）；§2 通用纪律「新文件共七个」改成六个（v1 就写错了：§2.1-§2.6 是六个文件）；§6 第 3、4 步的用例与变异范围；契约头的实跑说明。

**v1.1 新增或改动的用例一览**：C5（新）；S1（加两条断言）；S3 `with_judge_no_summary`（新）；S7 `left_verbose`、`all_fail_verbose`、`note_verbose`、`note_refused`、`rounds2_last_record`、`note_order`（新）；S8 `named_verbose`、`preview_verbose`（新）；S13 整个用例（新，六个子测试）；T1 `verbose_unconfigured`（新）；§2.0 `smxCfg` 加 `smx_k`；CO1 的授权改动加 R5 `round2`。**v1.1 新增的变异**：#43-#46、#48、#49、#51、#52、#55、#73-#76、#78-#85（编号沿用 W1 的，W1 没编号的六条给 #79-#84，V3 第 4 点那一条给 #85），共 21 条，全部实测被点名杀手杀掉；另有两条新的等价变异（Join 的 `ConvID`、`JoinedAt`）与一条「只在审查时看」（`!rec.Outcome.Ingested()`）。

### W1b 修订（第二轮审查的 3 条 major、6 条 minor，另加修订人自拟的同类项一条，2026-09-28）

W1b：照 v1.1 忠实实现（CO1 v1.1 含它的 W1b 修订，加本刀 §1.1），§2 全部期望第一次就逐字节复现；21 条 v1.1 新变异逐条实跑，红集与 §4 逐字一致；自拟变异里 3 条 major、6 条 minor，没有 blocker（照契约忠实实现不会做错，也不花钱）。本修订逐条处理如下。**§1.1 冻结的生产代码一个字没改**（修订前后 §1.1 A-I 的全部代码块逐字节比对相同），所以仍然没有「§1.1 改动」一项。新期望全部取自修订原型 `E:\Isolating_test\council\sm1_fix\` 的实跑输出，新变异全部实跑（做法见契约头）。W1b 的发现编号照它返回的顺序记作 W1b-1 到 W1b-9。

- **W1b-1（major，判据第 1-3 条的详细档没有夹具）** → 照改。§2.4 S3 加 `single_verbose`（整串）与 `no_summary_verbose`（从最后一个整行 `发言轮结算` 起逐行）；S10 拆成两个子测试：`concise`（v1 原样）与新的 `canceled_verbose`；S2 加三条库断言与「整行 `总结轮结算` 恰好 1 次」，成为详细档缺席断言名副其实的正对照（W1b-7 同一处）。§4 新增 #86（W1b 的 PA）、#87（PB）、#88（PC），各自实测只被对应的新夹具杀。PA 在 `round.go`，而 §6 第 2 步原来的逐字核对清单不含 `round.go`：补上 F1-F5。
- **W1b-2（major，详细档 mention 取名单没有夹具）** → 照改。§2.4 S8 加 `mention_verbose`；§4 新增 #89，实测杀手只有它（`named_verbose`、`preview_verbose` 保持绿）。连带：#76 的红集多出 `mention_verbose`；#84 下它实测保持绿（带 `--go`）。【判别力】写明三条取名单的路在详细档各有一个子测试。
- **W1b-3（major，总结头「显示名为空时回落到 id」没有夹具）** → 照改，取 W1b 的第一种修法（单测）。§2.3 的 Record 表加第六个 `summary_fallback_id`（`smx_b`，显示名是空串），K1 期望 `"总的来看（smx_b 做总结）：B\n"`；K2 同样走它。W1b 没有实跑这一条，本修订原型实跑：忠实实现上绿，§4 新增的 #90 下只有 K1 的这一个子测试红（K2 的同名子测试保持绿：K2 在「：」之后切头，头不在比较范围内）。反方向的 #38（头直接用 id）杀不到它（回落之后与 id 同串），由 `spoke_rich` 等杀，两条互补。
- **W1b-4（minor，与 CO1 的清点没跟上 CO1 的 W1b 修订）** → 照改三处。(1) §1.2 与 §8 加一条**有条件的第六处授权**：若主代理采纳 CO1 §7.4 第 1 条（R5 `judge_spawn_failed`），它的期望整串在 `总用时 7.2s` 之前插入提示行，由 7 行变 8 行（整串由本修订原型实跑冻结）；时钟 2 次、原因行规范化不变。采不采纳在 CO1，本刀不定，登记 §7.4。(2) 契约头「八个 cox 文件」改成「七个」。(3) §1.2 的「不受影响」清单补上 R11 `canceled_line`、`canceled_judge_line`、`round_report_err` 与 L1 第三条（前三条本修订原型实跑绿；L1 沿用 W1b 的实测，按代码不走 `/round`）。
- **W1b-5（minor，CO1 #83 也被 S7 两条杀）** → 照改。§4 末尾改成「CO1 #62 与 #83 都被 S7 `note_order`、`summary_fails` 杀」。本修订原型实测：两条变异下这两个子测试都红（另有 CO1 待裁的 `judge_spawn_failed` 红）。正常跑时 #83 先由前提守卫报（`note_order` 的「原因行恰好 2 行」、`summary_fails` 的「第 5 行以 `/round smx_z: ` 开头」）；把两处守卫临时关掉再跑，两条的整串 / 逐行主断言同样红（主断言本来就含那一行）。所以不把守卫升格为被测断言，用例结构不动，只在 §4 写明。
- **W1b-6（minor，进程内换总结成员：被拒的是作为发言者的 X）** → 照改。§1.0 代价 (b)、§6 第 6 步之 1、§7.3 拆成两种情形。本修订原型用三条临时探针核过（不交付，模拟 `/config reload --apply`：`rn.SetConfig` 加换 `r.cfg`，`Load` 补的默认 role 手工写进配置）：只改总结成员自己的 role，连跑两轮都是「总的来看：没有总结（拒绝）」加 `/round smx_s: 身份或脑子在这条 provider 会话开着的时候被改了；Step 6 不静默重建`；把总结成员从开过会话的 X 换成没开过会话的 Y，Y 的总结照常，X 作为发言者被拒（`总结 S: 没有回复（拒绝）` 加同一句原因，注明行写 `总结 S（拒绝）`）；换成一个开过会话、配置里没写 role 的 Y，Y 的总结被拒，`/session rebuild Y` 之后恢复。另写明：漂移只在 role 真的变了时发生，X 或 Y 在配置里写了自己的 role 时，默认 role 不补也不撤，身份不变。
- **W1b-7（minor，#75 不是「静默拉回」；缺席断言点名的正对照不断言那条性质）** → 照改两处。(1) #75 的说明改成「详细档不认已退出：照起总结进程、落库被拒成 post_rejected（钱照花，正文整段打在屏幕上）」。本修订原型探针在 #75 下读到：smx_s 仍未激活、库里没有 smx_s 的消息、屏幕有 `本轮问 1 个: smx_s`、`outcome post_rejected  agent smx_s`、「正文写不进库（已经付过钱），整段如下:」与 `总结轮结算`，`left_verbose` 红在末行。(2) S2 加库断言（见 W1b-1）；`left_verbose` / `all_fail_verbose` 的【判别力】写明这几条缺席断言守的是「真拉回 / 真 Join」一类写法，#75 由末行断言杀；S13 的「没有 smx_s 的消息」点名正对照 S1（S13 一律简洁档）。
- **W1b-8（minor，「第 1-6 条都不拍快照」说多了）** → 照改。S13 末段改成「第 1-4 条」；第 5 条、第 6 条之前多拍一次快照（W1b 的两条）本修订原型实跑，三包全部 smx / cox / `TestWithJudge*` 用例全绿，进 §4「只在审查时看」（行为上无害：多一次读库，不花钱）。
- **W1b-9（minor，C5 说「照 MD1 `AgentBlockTOML` 的形状」不准）** → 照改，取 W1b 的第一种修法：C5 `table_after_summary` 追加的那段换成 MD1 契约 §2.3 C1(a) 期望串的原样（含 `# council /add 追加：…` 注释行与带点号的 `[agents."opus-5.5-1m-high"]`），分隔照 MD1 `AppendBlock` 的规则（原文以 `\n` 结尾，再补一个 `\n`）；对照 `keys_without_header` 换成同一段去掉表头那一行，错误整串列出七个键。相交表 `schema.go` 那一行与 §7.3 同步改写。
- **修订人自拟的同类项（不是 W1b 的发现；「同类的还有没有别处」）：装了观察者（TUI）这一维** → 加夹具。W1b-1 补的是档位这一维；同一组判据还有「装没装观察者」一维（`app.Run` 默认走 TUI，装观察者），而 T1 只有「总结照跑」「没配」两种形状。本修订原型给判据第 1、2、3、5、6 条、注明行、名单拒绝各加一个 `&& r.roundObs == nil`，七条全部活过当时的全部用例（其中第 1、2、5、6 条与名单拒绝是花钱方向）。新增 §2.4 S14 `TestSmxRoundSummaryObservedGates`（repl 包，装 S12 的记录型观察者，七个子测试），§4 新增 #91-#97，各自实测只被对应的子测试杀；正对照是 S12（同一个观察者、两个成员、总结照跑）。观察者下的预览 / mention 两条取名单的路与真 TUI 里的 Ctrl-C 不另造夹具，进「只在审查时看」，由 §6 第 2 步的逐字核对担保。

W1b 返回里的其余内容：通用纪律、F0-F8 与 open items 的落实核对都是「落实」；「`!rec.Outcome.Ingested()` 活过全部用例」「Join 的 `ConvID` / `JoinedAt` 是等价变异」「#80 的 `context canceled` 机理」与契约一致。这些不需要改。

**W1b 修订新增或改动的用例一览**：C5（两个子测试都换夹具）；K1 / K2（Record 表加 `summary_fallback_id`）；S2（加四条断言）；S3 `single_verbose`、`no_summary_verbose`（新）；S8 `mention_verbose`（新）；S10（拆成 `concise` 与新的 `canceled_verbose`）；S14 整个用例（新，七个子测试）；CO1 的授权改动加有条件的第 6 条（R5 `judge_spawn_failed`）。**新增变异**：#86-#97，共 12 条，全部实测只被对应的新夹具杀。**新增的「只在审查时看」**：第 5、6 条之前多拍快照；观察者下的预览 / mention 与真 TUI 取消。

## 与本批另外两刀的文件相交（逐个点名）

| 文件 | 本刀改的函数 / 位置 | 相交的另一刀 | 相交点与处置 |
|---|---|---|---|
| `internal/repl/round.go` | `cmdRound`：flag 循环加 `--no-summary`；`snap0` 之后取总结成员并拒绝它出现在名单里；发言轮循环前加 `asked`；两处 `runOneRound` 调用多传两个参数；裁判轮从「早退」改成 `if` 块；函数末尾调 `runSummary`；头注释「本片不引入任何配置键，也不引入自动链…」那一句改写。`runOneRound`：签名多两个参数、回调改调 `render`、`writeErr` 检查之后把 `recs` 追加进 `asked`、函数注释补两个参数 | **CO1**（必相交，CO1 先落） | CO1 在同一个函数里加了命名返回值、`start`/`spawned`/defer、四处 `r.concise` 门与 `RoundEnd` 的 `Concise` 字段。本刀按 CO1 §1.1 E7 留的位置把总结放在「最后一段之后、`cmdRound` 返回之前」，所以屏幕顺序是「各段 → 总结 → CO1 的总用时」；本刀**不另打总用时**，总结起进程的时间计入 CO1 的总用时（它在 `cmdRound` 里）。锚点取 CO1 落地后的真文件 |
| `internal/repl/ask.go` | CO1 的 `writeAskRecord` 拆成 `writeAskRecord` / `writeSummaryRecord` 两个薄包装 + `writeRecord(r, rec, summary bool)`；只改简洁档的两个「头」 | **CO1**（改 CO1 冻结的函数体，主代理已接受，见 §7.1） | CO1 §1.1 D1 冻结了这个函数体。本刀的改动只在简洁档成员头与失败行两处加 `summary` 分支，`summary == false` 时字节不变（CO1 的 K1-K5 与本刀 §2.3 K2 共同担保）。锚点取 CO1 落地后的真文件 |
| `internal/repl/observer.go` | `RoundStartEvent` 的 `Label` / `K` / `N` 三行字段注释 | **CO1**（CO1 改的是 `RoundEndEvent`，相邻结构体） | 只改注释，不改字段。锚点取 CO1 落地后的真文件 |
| `internal/repl/commands.go` | 命令表里 `round` 那一行的 summary 字面量 | **CO1**（在同一张表末尾追加 `verbose` 一行）；**MD1**（改 `cmdAdd`，不改表） | 三刀改的是不同的行 / 函数。本刀只替换 `round` 那一行（HEAD `:152`） |
| `internal/repl/summary.go` | 新文件 | 无 | — |
| `internal/config/schema.go` | `Config` 末尾加 `Summary SummaryConfig`；新类型 `SummaryConfig`；新常量 `DefaultSummaryRole` | **MD1**（新增 `internal/config/append.go`，不碰 `schema.go`） | 语义相交：MD1 把新 agent 表整段追加到文件末尾。`[summary]` 若已在文件末尾，后面再追加 `[agents."x"]` 仍是合法 TOML（TOML 的表不限顺序，只要求顶层键在所有表之前）。MD1 追加完做 Load + Validate，会经过本刀的 `summary.agent` 校验；已合法的配置追加之后仍合法（v1.1：§2.1 C5 钉住「`[summary]` 之后再接一张 agents 表」Load + Validate 通过、新 agent 已定义、`summary.agent` 不变，并用「同一段不带表头」作对照；W1b 修订把追加段换成 MD1 契约 §2.3 C1(a) 期望串的原样：注释行加带引号的 `[agents."opus-5.5-1m-high"]`） |
| `internal/config/load.go` | `applyDefaults` 在 agent 循环之后加一段：给总结成员补默认 role | **MD1**（不碰 `load.go`） | 无锚点冲突 |
| `internal/config/validate.go` | `Validate` 在 teams 循环之后、`data_dir` 检查之前加 `summary.agent` 两条校验 | **MD1**（它的 `NewAgentIDProblem` 在 `append.go`，不碰 `validate.go`） | 无锚点冲突 |
| `internal/turn/runner.go` | `askUpToWith` 里 agent 消息字面量（HEAD `:778-788`）之后加三行：总结成员的消息 `Recipients = []string{agentID}` | **MD1**（新增 `internal/turn/discovery_exec.go` 并在 turn 包加本包第一个 `TestMain`（`mdx_discovery_exec_test.go`）；不碰 `runner.go`） | 无锚点冲突。**本刀不许在 `internal/turn` 新增 `TestMain`**（一个包只能有一个），turn 包的用例也不依赖 MD1 的那个（用既有 `fakeSpawner`，不起进程） |
| `internal/repl/cox_round_test.go`、`internal/tui/cox_concise_test.go`（CO1 测试腿的文件，本刀落地时已是既有用例） | 五个用例的期望各加一行「没配总结成员」的提示（§1.2 逐处授权；v1.1 加 R5 `round2`；W1b 修订另加有条件的第 6 处 R5 `judge_spawn_failed`，只在主代理采纳 CO1 §7.4 第 1 条时生效） | **CO1** | 见 §1.2 与 §8 existing_tests_changed |

不碰：`internal/tui/*`（生产）、`internal/app/*`、`cmd/council/*`、`internal/conversation/*`、`internal/store/*`、`internal/provider/*`、`internal/turn` 里除 `runner.go` 那三行以外的一切（`router.go`、`render.go`、`scheduler.go`、`plan.go`、`budget.go` 一个字不改）。

---

## 0. 病根 / 动机（行号以 HEAD `ab53c0b` 为准，逐条打开真文件核对过）

1. **今天没有总结。** `cmdRound`（`internal/repl/round.go:63`）在最后一个发言轮之后，没带 `--with-judge` 就 `return nil`（`:217-219`），带了就跑一次裁判轮（`:220-236`）。头注释写死了「本片不引入任何配置键，也不引入自动链：上限就是人按一次 /round --go 跑一遍名单。将来的自动链入口必须收一个显式上限」（`:52-54`）；`docs/12_STEP9_CONTRACT.md` §4.7 两处「不自动触发」（`:1148`、`:1289`）。#45 推翻这两处：上限是「每条 /round 恰好多一次调用」，有界。
2. **成员的回复一律广播。** agent 消息在 `askUpToWith` 里构造（`internal/turn/runner.go:778-788`），没有 `Recipients`。可见性只有一个判据 `recipientOK`（`internal/turn/router.go:89-94`：空 = 广播；非空 = 只给名单里的 id），bootstrap（`selectBootstrap`，`:141`）与增量（`filterDelta`，`internal/turn/render.go:76`）共用它。自己的消息两条路都不给自己：增量路径 `Select` 恒传 `skipSelf = true`（`router.go:76`），bootstrap 只收「别人」的最后一条（`:152`）。所以「只给用户看」可以不动路由、只在写入时给总结成员的消息填 `Recipients = [自己]`：其余成员 `recipientOK` 为假，自己又被 self 规则排除 —— **没有任何成员的 prompt 会带上它**，而 `/log` 照样看得到（`cmdLog` 不过滤，`TO` 列印 `Recipients`，`internal/repl/commands.go:1167-1177`）。`/to` 是今天唯一的 `Recipients` 写入方（`commands.go:412-461`，经 `postUserText` `:327-334`）；`Service.Post` 事务外校验收件人「存在」（`internal/conversation/service.go:229-233`），`RecordTurn` 不校验（`:327-350`）。`[自己]` 两条都过（总结成员是参与者），将来谁给 `RecordTurn` 补上这条校验也不会拒它；`["user"]` 在 `Post` 那条下会被拒（`user` 是保留 sender，不是参与者，`internal/config/validate.go:38-41`），所以不用它。
3. **总结成员必须是活跃参与者。** `RecordTurn` 在事务内对非保留 sender 调 `RequireActiveParticipant`（`service.go:389-391`）：不在会话里的 agent 起进程、花了钱，落库时被拒成 `OutcomePostRejected`（`cmdAsk` 头注释 `internal/repl/ask.go:51-60` 记过这个坑）。所以总结之前必须 Join；`AddParticipant` 是 upsert、无条件 `active = 1`（`internal/store/tx.go:181-189`）—— 对 `/leave` 过的成员再 Join 等于静默拉回，`/team use` 明文不做这件事（`commands.go:613-625`）。
4. **总结成员看不到失败。** `filterDelta` 丢掉 `KindSystem`（`render.go:73-75`），`selectBootstrap` 同样（`router.go:144-148`）；`turn_failed` / `post_rejected` 都是 system 消息。#45（`:2129`）：某成员没发言时由 Council 在总结上方注明。
5. **总结要知道自己在做一致性判断，但 Griffin 不想手写长 role。** 身份块由 `renderIdentityCore` 拼（`render.go:109-127`：`You are <名> (id=<id>).` / `Provider: <p>` / role），整块进 `IdentityFingerprint` 的 role 段；`identityFPFor` 在 `askUpToWith`、`budgetOne`、`auditDriftAgainst`、`rulesInfoFor` 四处调用，**全部读同一个 `*config.Config`**。另一种放法 —— 每轮在 prompt 末尾附一段说明 —— 违反 `RenderDelta` 注释里的那条（`render.go:36-40`：delta 的内容必须只由 messages 决定），还要在 `budgetOne` 的余量计算里复制一份（`internal/turn/budget.go:153-163`），且每轮重复进会话历史。
6. **配置只能加一张表。** TOML 的顶层键必须在所有表之前；`Config`（`internal/config/schema.go:41-68`）今天只有顶层键和三张 map；`Load` 拒绝未知键（`internal/config/load.go:32-34`、`unknownKeysError` `:53-68`）。`dottedIDHints` 只看 map 型字段（`tableFieldNames`，`:139-163`），加一个结构体字段不影响它。
7. **代价（写进 #45 的「代价照实写」）。** 每条多成员 `/round` 多一次**串行**调用（在所有发言段、裁判段之后），墙钟照加。总结成员首次开口是 bootstrap，必冷；距上次开口超过 `CacheTTL`（1 小时，`internal/turn/session.go:21`、`coldExpectedAt` `:344-354`）也按冷计价。claude 冷启动实测 47749 token 入场、0.955 美元（D-4；本契约夹具 `fakeClaudeSuccessJSON` 的 `total_cost_usd` 就是这一次：`0.9551999999999999`）。它是一次正常的 Ask：`turn_usage` 一行、自己的 `round_id`（`RunRoundPreparedStream` 现铸，`internal/turn/scheduler.go:102-118`），`/cost` 按成员与按 `--round` 都看得到。
8. **两条既有用例钉着快照次数。** `TestWithJudgeRunsOneExtraRoundWithOwnID`（`internal/repl/round_test.go:361`）与 `TestWithJudgeRunsOnlyOnceAcrossMultipleRounds`（`:398`）数 `Store.MaxSeq` 的调用次数（发言轮、裁判轮各一次）。它们的配置没有 `[summary]`、名单是两个成员（一个发言、一个裁判）。所以「没配总结」这条路上**不许多拍一次快照**、也不许多读一次成员表（原型第一版在判配置之前拍快照，这两条当场变红）。

---

## 1. 范围

### 1.1 做

**冻结粒度**：下面的代码是原型实跑过的版本。**标识符名、签名、判据、字面量、顺序全部冻结**（测试腿白盒引用 `writeAskRecord`、`writeSummaryRecord`（§2.3）与 `config.SummaryConfig`、`Config.Summary`、`config.DefaultSummaryRole`；不引用 `runSummary`、`writeRecord`、`summaryLabel`）；只有注释由实现方写，必须覆盖每条列出的「注释要点」，**不许写与代码不符的话**。只有明说「名字实现方定」处有自由（本刀只有 `summary.go` 里两个消息常量名与 `runSummary` 内部的局部变量名）。代码块里的缩进可能被排成空格，落盘一律 tab（gofmt）。

#### 1.0 设计与论证（冻结，实现与测试都按这里）

- **怎么参与**：总结成员**不需要用户先 /add**。到了要总结的那一刻，`runSummary` 读一次成员表（含已退出）：
  - 不在表里 → 用配置里的 `display_name` / `provider` `Join`（与 `cmdAdd` `commands.go:894-901` 同一种 `Participant` 字面量），**不打任何行**（`/who` 里看得到它，屏幕上它的名字随后出现在总结头里）；
  - 在表里但已退出 → **不拉回**，打一行「没有总结（X 已退出当前会话…）」，这一轮不总结。理由同 `/team use` 的裁决（`commands.go:613-625`）：`/leave` 是用户的明确意图，而且对总结成员来说它正好是「这个会话里别再总结」的开关；
  - 在表里且活跃 → 直接问。
- **怎么问（与 PlanRound、askUpToWith、闸、在飞记录、指纹的关系）**：总结是**第三种段**，走与裁判轮完全相同的路 `runOneRound → RunRoundPreparedStream → askUpToWith`，`Plan{Speakers: [总结成员], Judges: [], Skipped: []}` 直接构造（与裁判轮 `judgePlan` 同一先例，`round.go:230-234`），label 冻结为 `总结轮`。于是：
  - 不经 `PlanRound`（它只给用户点名的名单分类；总结成员不是用户点的，而且 §1.1 F2 保证它**不会**出现在任何 `/round` 名单里）；
  - `BudgetWith` 照算（详细档印它的预算行，`argvRefusal` 照拦超长命令行）；`skipNoDelta` 照判；
  - `askUpToWith` 的每一道闸照走：A 之五的在飞记录拒绝（`runner.go:380-386`）、配置 / provider / 权限档 / 工作目录拒绝、指纹比对与 `session_rebuilt`、`BeginTurn` 在飞记录与 `spawned` / `returned` 两档、5 分钟硬超时、`RecordTurn` 落库与 `expectInflight`；闸按 (会话, 总结成员) 一把，与发言成员互不相干；**没有第二条起进程路径**；
  - 自己一个 `round_id`、`round_started` / `round_finished` 两条面包屑（system 消息，不进任何 prompt）；
  - 观察者照发 RoundStart / MemberDone / RoundEnd 三种事件，TUI 看板因此多一段 `总结轮`，不改 TUI 一行代码。
- **只给用户看**：`askUpToWith` 构造 agent 消息时，若 `cfg.Summary.Agent == agentID`（`cfg` 是这一轮已经快照好的那份），`msg.Recipients = []string{agentID}`（§0 第 2 条的论证）。**这条按配置判，不按调用路径判**：`/ask <总结成员>` 的回复同样只给用户看。后果照写：总结成员以后每一轮都看不到自己以前的总结（self 规则），这是想要的（不被自己带偏）。
- **总结成员不许在 `/round` 名单里**：`cmdRound` 在拿到 `snap0` 之后、打印任何东西之前，若名单（给了 id 就是给的，没给就是 mention 与活跃成员的交集）含总结成员，整条命令拒绝。理由：它的会话里留着总结；它若以发言者身份说话，那段话同样只给用户看（上一条），其余成员看不到，等于白花钱还让人以为它参与了讨论。拒绝与 `--go`、`--no-summary` 无关（预览也拒）。
- **指令放哪里**：`config.Load` 的 `applyDefaults` 给**总结成员且 role 为空**的那一个 agent 填 `DefaultSummaryRole`（§3.1 逐字冻结）。论证：
  - 不泄漏：role 只进总结成员自己的身份块（claude 的 `--append-system-prompt-file`，`internal/provider/claude.go:88-110`），不进任何 delta；
  - 不让其他成员漂移：别人的身份块不含它；
  - 指纹稳定：常量不变，指纹就不变；四处 `identityFPFor` 读的是同一份 `*config.Config`，不需要在四处各补一次；
  - 用户自己写了 role 就用用户的（默认只填空值）；
  - 代价照写：(a) 不经 `Load` 直接构造的 `Config`（测试）拿不到默认值，要自己填 `Role`；(b) **同一个进程里** `/config reload --apply` 让一个**在这个进程里开过会话**的 agent 的 role 变了，这个 agent 以后每次被问都在起进程之前被 `ErrIdentityDrift` 拒绝（`runner.go:449-455`；原因行 `/round <id>: 身份或脑子在这条 provider 会话开着的时候被改了；Step 6 不静默重建`，不花钱），**要先 `/session rebuild <这个 agent 的 id>` 才恢复**，不会自己好。落到总结上分两种（W1b 修订拆开）：情形一：当前总结成员的 role 变了 —— 改了它的 role；给一个开过会话、配置里没写 role 的 agent 首次加上 `[summary]`；把 `agent` 换成一个开过会话、配置里没写 role 的 Y —— 此后每条 `/round` 的总结都是「总的来看：没有总结（拒绝）」加上面那句原因，要 `/session rebuild <总结成员 id>`；情形二：把 `agent` 从 X 换走、X 的 role 原是 `Load` 补的默认值（配置里没写）—— X 失去默认 role，X 若开过会话，以后作为发言者或被 `/ask` 时被拒（X 那一行是「没有回复（拒绝）」加同一句原因，那一轮总结上方的注明行写 X「（拒绝）」），要 `/session rebuild X`；新总结成员 Y 若在这个进程里没开过会话，总结照常。配置里自己写了 role 的一方，默认 role 不补也不撤，身份不变，不受影响；(c) 将来改 `DefaultSummaryRole` 的措辞，所有总结成员各重建一次会话（冷启动一次），其余成员不受影响；(d) **重启之后**（走读库那条路，`runner.go:469-503`）才会自动作废重建一次、冷启动一次；启动漂移审计与 `/config reload` 的预览照报它的 role 变了 —— 预期行为，不是 bug。
- **配置与校验**：一张表 `[summary]`，唯一的键 `agent`（其余键被 `Load` 以未知键拒绝）。没有另外的开关：删掉这张表 = 关；`agent = ""` 或空表 = 没配。`Validate`：`agent` 非空时必须是已定义的 agent（大小写敏感，与 teams 的 `undefined agent` 同体例）；**且其 `permissions` 必须是 `read`** —— 总结是自动起的进程，没有人当场点名，不能带写盘能力。**mode 不要求**：总结路径不经 `PlanRound`，mode 在这条路上只影响 thought 可见性（team 的 `thought_visible_to` 按 mode 判，`internal/turn/runner.go:979-1008`），那是用户经 team 配置要的东西，不拦。
- **什么时候总结（判据冻结，按顺序）**：`cmdRound` 走完发言轮与（若有）裁判轮、且没有提前 `return` 之后：
  1. 带了 `--no-summary` → 不总结，一行都不打（没配总结时的提示也不打）；
  2. 本条命令里**真正送进 `RunRoundPreparedStream` 的成员**（发言轮各段与裁判轮的 `recs`，按 `AgentID` 去重）少于 2 个 → 不总结，一行都不打。这就是「单个成员的 /round 不总结」：`/round a --go`、`/round a b --go` 而 b 无新内容、`/round x --go --rounds 2` 都不总结；`/round a --go --with-judge`（一个发言者加一个裁判）算两个成员，总结。没带 `--go` 的预览一个成员都没送进去，自然不总结；
  3. `ctx.Err() != nil`（Ctrl-C 已经发生）→ 不总结，一行都不打（TUI 收尾行已经是 `/round 已取消`；行式模式下 main.go 的 NotifyContext 随后让进程退出）；
  4. 没配总结成员（`snap0.Cfg.Summary.Agent` 为空）→ 简洁档打一行提示（§3.2 冻结），详细档一行都不打（详细档在没配时与 CO1 落地后的输出逐字节相同）；
  5. 这些成员里**没有一个**这一条命令里落了库的发言（`rec.Message != nil`）→ 打一行「没有总结（这条 /round 没有成员回复）」，不 Join、不起进程、不花钱；
  6. 总结成员已退出 → 打一行（§3.2），不总结；
  7. 否则：取一次新快照（`TakeRoundSnap`，与裁判轮同一位置的理由：看见的是前面各段写完之后的历史），必要时 Join，若有成员**最后一次**没有回复就先打一行注明（§3.2），再跑总结段。
  第 1-6 条都不拍快照、第 1-5 条都不读成员表（§0 第 8 条）。
- **`--rounds N`**：只在最后之后总结一次（发言轮循环与裁判轮都结束之后才到这里）。总结成员看到的是它自上次开口以来的全部新消息（首次开口是 bootstrap：会话标题、全部用户消息、每个其他成员窗口内最后一条，`router.go:96-173` 的既有规则），不是「只有这一条命令」。
- **TUI**：总结段是看板上的第三种段（表头 `总结轮 (xxxxxx)  回来 0/1`）；收尾行 `/round 结束  <时长>  回来 k/N` 的 k、N 把总结段算进去（`rosterN` 加 1、回来一个加 1，`internal/tui/model.go:229`、`:256`），所以两个成员加总结是 `回来 3/3`；没配、`--no-summary`、单成员时仍是 `回来 2/2` / `1/1`。详细档下 CO1 的段终态块照打（`总结轮 (xxxxxx)  回来 1/1` 加成员行），简洁档不打（CO1 G1）。
- **屏幕（冻结，见 §3）**：简洁档，成功时一行头 `总的来看（<总结成员显示名> 做总结）：` 紧接正文（全角括号、全角冒号、冒号后不留空格；正文多行照原样，不以换行结尾时补一个）；失败时一行 `总的来看：没有总结（<结局词>）`，原因行是 `runOneRound` 回调里既有的 `/round <id>: <原因>`（CO1 E6 说它两档都打）；其余照打的行（可能已吃下、漂移、被拒工具调用、正文写不进库整段）与成员块同一份代码。总结头**不带用时**（#45 的原话没有；它的用时进 CO1 的总用时与 TUI 看板）。「本轮问 1 个: <总结成员 id>」两档都打（CO1 保留它的同一个理由：进度信号；一次总结可能要几十秒）。详细档：总结段照裁判轮的详细格式打，标签 `总结轮`（预算表、扇出期间、`本轮问 1 个`、`outcome …` 起的整块明细、`总结轮结算` 块）。
- **格式只有一份**：总结段复用 `runOneRound` 与 `writeAskRecord` 那一份函数体；新格式只有 #45 规定的两个「头」，写在同一个函数里的分支（`writeRecord(r, rec, summary)`），不另写一个渲染函数。

#### A. `internal/config/schema.go`

A1. `Config` 结构体最后一个字段 `Teams`（`:67`，tag 是 `toml:"teams"`）之后、结构体的 `}`（`:68`）之前加一个字段（名字、类型、tag 冻结；上方注释按要点写）：

```go
	Summary SummaryConfig `toml:"summary"`
```

注释要点：`[summary]` 表，#45 第二节；要写成**表**而不是顶层键，因为 TOML 的顶层键必须写在所有表之前，而 Griffin 要能把它追加到文件末尾；零值（没有这张表、空表、`agent = ""`）= 没配；`Validate` 的两条规则见 `validate.go`；默认 role 见 `DefaultSummaryRole`。

A2. 在 `Config` 结构体之后、`ProviderConfig`（`:70`）之前加（类型名、字段、tag、常量名与值逐字冻结）：

```go
type SummaryConfig struct {
	Agent string `toml:"agent"`
}

const DefaultSummaryRole = "你是这场讨论的总结人，不参与讨论。每次被叫到时，读用户的问题和各成员的发言，做一致性判断：他们在哪些点上一致，在哪些点上有分歧、各自的依据是什么，有没有人答非所问或明显出错。你的话只给用户看，其余成员看不到；不要替任何一方继续论证，不要提出新的方案。用中文回答：先用一句话给出总的判断，再分条列出一致之处与分歧之处，写得简短。"
```

注释要点：`SummaryConfig` —— 只有一个键；唯一的开关就是有没有配 `agent`；多成员 `/round` 之后由这个 agent 出一段「总的来看」，只给用户看。`DefaultSummaryRole` —— 总结成员的 role 为空时由 `Load` 填进去（不要求 Griffin 手写长 role）；它进身份块、进指纹：改这句话会让每个总结成员各重建一次会话（冷启动一次），其余成员不受影响；只填空值，用户自己写了 role 就用用户的。

#### B. `internal/config/load.go`

`applyDefaults`（`:178-198`）里 agents 循环结束的 `}`（`:191`）之后、`for id, team := range cfg.Teams {`（`:192`）之前加（逐字）：

```go
	if id := cfg.Summary.Agent; id != "" {
		if agent, ok := cfg.Agents[id]; ok && agent.Role == "" {
			agent.Role = DefaultSummaryRole
			cfg.Agents[id] = agent
		}
	}
```

锚点建议取 `:190-192` 三行（`		cfg.Agents[id] = agent` / `	}` / `	for id, team := range cfg.Teams {`，在文件里恰好一次）。
注释要点：只给总结成员一个 agent 补、只补空值；放在 `Load` 而不是 `Validate` 或 turn：`askUpToWith`、`budgetOne`、`AuditDrift`、`/rules` 的探针四处算指纹，读的都是 `Load` 交出的这一份配置，放这里四处自动一致；`agent` 指向不存在的 id 时什么都不做，由 `Validate` 报；代价是不经 `Load` 构造的配置（测试）拿不到默认值。

#### C. `internal/config/validate.go`

`Validate` 里 teams 循环结束的 `}`（`:131`）之后、`if c.DataDir != "" && !filepath.IsAbs(c.DataDir) {`（`:133`）之前加（逐字；错误文本见 §3.4）：

```go
	if id := c.Summary.Agent; id != "" {
		agent, ok := c.Agents[id]
		if !ok {
			add("summary.agent", fmt.Sprintf("undefined agent %q", id))
		} else if agent.Permissions != PermissionRead {
			add("summary.agent", fmt.Sprintf("agent %q must have permissions = \"read\": the summary runs without being asked", id))
		}
	}
```

注释要点：空串 = 没配，不报；大小写敏感（与 teams 的 `undefined agent` 同体例）；只读的理由（总结是自动起的进程，没有人当场点名，不给写盘能力）；mode 不要求的理由（总结路径不经 `PlanRound`，mode 在这条路上只影响 thought 可见性）。

#### D. `internal/turn/runner.go`

`askUpToWith` 里 agent 消息字面量 `msg := conversation.Message{ ... Meta: meta, }`（`:778-788`）之后、`r.commitSession(g, st, &rec, fp, res)`（`:789`）之前加（逐字）：

```go
	if cfg.Summary.Agent != "" && agentID == cfg.Summary.Agent {
		msg.Recipients = []string{agentID}
	}
```

锚点建议取 `:787-789`（`		Meta:           meta,` / `	}` / `	r.commitSession(g, st, &rec, fp, res)`）。`cfg` 在这里一定非 nil（`:388` 已拒绝 nil）。
注释要点（中文，写在这三行上方）：#45「总结只给 Griffin 看」；机制是 `recipientOK`（router.go）这唯一的可见性判据 —— 名单只有它自己，其余成员 bootstrap 与增量两条路都挑不到，它自己又被 self 规则排除，所以没有任何成员的 prompt 会带上这条；`/log` 不过滤，照样看得到（TO 列是它自己）；为什么是 `[自己]` 不是 `["user"]`（`user` 是保留 sender、不是参与者，`Service.Post` 的收件人校验会拒它；`RecordTurn` 今天不校验收件人，将来补上时 `[自己]` 仍然合法）；按配置判、不按调用路径判，所以 `/ask <总结成员>` 也只给用户看；`cfg` 是这一轮入场时的快照，与指纹同一份。

#### E. 新文件 `internal/repl/summary.go`

内容冻结如下（函数体、判据顺序、字面量逐字；两个消息常量名与局部变量名实现方定；注释按要点写）：

```go
package repl

import (
	"context"
	"strings"
	"time"

	"council/internal/conversation"
	"council/internal/turn"
)

const summaryLabel = "总结轮"

const (
	msgSummaryNotConfigured = "总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）"
	msgSummaryNoReply       = "总的来看：没有总结（这条 /round 没有成员回复）"
)

func runSummary(ctx context.Context, r *REPL, asked []turn.Record, sumID string) error {
	var order []string
	last := make(map[string]turn.Record, len(asked))
	replied := false
	for _, rec := range asked {
		if _, seen := last[rec.AgentID]; !seen {
			order = append(order, rec.AgentID)
		}
		last[rec.AgentID] = rec
		if rec.Message != nil {
			replied = true
		}
	}
	if len(order) < 2 {
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	if sumID == "" {
		if r.concise {
			return r.printf("%s\n", msgSummaryNotConfigured)
		}
		return nil
	}
	if !replied {
		return r.printf("%s\n", msgSummaryNoReply)
	}
	parts, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return r.reportErr("round", err)
	}
	joined := false
	for _, p := range parts {
		if p.AgentID != sumID {
			continue
		}
		if !p.Active {
			return r.printf("总的来看：没有总结（%s 已退出当前会话；要恢复总结用 /join %s）\n", sumID, sumID)
		}
		joined = true
	}
	snap, err := r.rn.TakeRoundSnap(ctx, r.cur.ID)
	if err != nil {
		return r.reportErr("round", err)
	}
	if !joined {
		ac := snap.Cfg.Agents[sumID]
		if err := r.svc.Join(ctx, r.cur.ID, conversation.Participant{
			ConvID:      r.cur.ID,
			AgentID:     sumID,
			DisplayName: ac.DisplayName,
			Provider:    ac.Provider,
			Active:      true,
			JoinedAt:    time.Now().UTC(),
		}); err != nil {
			return r.reportErr("round", err)
		}
	}
	var missing []string
	for _, id := range order {
		if rec := last[id]; rec.Message == nil {
			missing = append(missing, r.displayName(id)+"（"+OutcomeWord(rec.Outcome)+"）")
		}
	}
	if len(missing) > 0 {
		if err := r.write("这些成员最后一次没有回复，总结看不到他们这一次的话：" + strings.Join(missing, "、") + "\n"); err != nil {
			return err
		}
	}
	plan := turn.Plan{
		Speakers: []string{sumID},
		Judges:   []string{},
		Skipped:  []turn.SkipReason{},
	}
	_, err = runOneRound(ctx, r, plan, true, summaryLabel, 0, 0, snap, writeSummaryRecord, nil)
	return err
}
```

注释要点：
- 文件头 / `runSummary` 上方：§1.0「什么时候总结」的七条判据与顺序（逐条对应代码）；为什么第 1-6 条不拍快照、第 1-5 条不读成员表（§0 第 8 条，两条既有用例数 `MaxSeq`）；`asked` 是本条命令里真正送进 `RunRoundPreparedStream` 的记录（发言轮各段与裁判轮），按 `AgentID` 去重、以第一次出现的顺序排（发言轮是字典序，裁判随后；v1.1 由 S7 `note_order` 钉住）；注明行的判据恰好是 `rec.Message == nil`（不枚举结局，PostRejected 也要注明；§4「只在审查时看」第 1 条）；「最后一次没有回复」取的是每个成员在 `asked` 里的**最后一条**记录（v1.1 由 S7 `rounds2_last_record` 钉住；`--rounds N` 时前面几轮说过话、最后一轮失败的成员也要注明，总结看到的是它前几轮的话）；总结成员看不到 system 消息（`filterDelta` / `selectBootstrap` 都丢 `KindSystem`），失败只能由 Council 注明（#45）；已退出不拉回的理由（`/team use` 同一裁决，也是「这个会话别再总结」的开关）；Join 不打行的理由；`sumID` 来自 `cmdRound` 的 `snap0.Cfg`，这里的 `snap` 是同一条命令里再拍的一次，命令串行、`/config reload` 只在两条命令之间换配置，所以两次的 `Cfg` 是同一个指针（`sumID` 非空时 `snap.Cfg` 非 nil）；总结段走与裁判轮同一条路（`runOneRound` → `askUpToWith`），A 之五在飞闸、指纹、`BeginTurn`、超时、`RecordTurn` 一道都没绕过；返回的 `bool` 不看（能走到这里说明前面已有成员落了发言，CO1 的 `spawned` 已经是真）；总结的屏幕位置在 CO1 defer 打的总用时之前（CO1 E7）。
- `summaryLabel`：TUI 表头、详细档结算块标题（`总结轮结算`）、段终态块标题都用它；与「发言轮」「裁判轮」同一体例。

#### F. `internal/repl/round.go`（锚点取 CO1 落地后的真文件）

F1. `cmdRound` 的局部变量：`withJudge := false`（HEAD `:65`）之后加一行 `noSummary := false`；flag 循环里 `case "--with-judge":` 那一支（HEAD `:75-77`）之后加：

```go
		case "--no-summary":
			noSummary = true
			continue
```

F2. `snap0` 之后（HEAD `:150-153` 那一段，CO1 不碰）整段换成（逐字）：

```go
	var agents map[string]config.AgentConfig
	sumID := ""
	if snap0.Cfg != nil {
		agents = snap0.Cfg.Agents
		sumID = snap0.Cfg.Summary.Agent
	}
	if sumID != "" {
		for _, id := range ids {
			if id == sumID {
				return r.printf("拒绝: %s 是总结成员（config.toml 的 [summary]），不在 /round 里发言；它的话只给你看，其余成员看不到。\n", id)
			}
		}
	}
```

（原来的四行是 `	var agents map[string]config.AgentConfig` / `	if snap0.Cfg != nil {` / `		agents = snap0.Cfg.Agents` / `	}`。）拒绝在 `PlanRound`、`renderSkipped`、「发言顺序」之前，所以被拒时屏幕上只有这一行、一个进程都不起；CO1 的 defer 因 `spawned` 为假不打总用时。

F3. `finished := 0`（HEAD `:187`）之后加一行 `var asked []turn.Record`；发言轮调用（HEAD `:202`）改成：

```go
		ran, err := runOneRound(ctx, r, plan, goRun, "发言轮", k, rounds, snap, writeAskRecord, &asked)
```

F4. 函数末尾：CO1 落地后的这一段（HEAD `:217-237` 加 CO1 E4 的改动）

```go
	if !withJudge || len(plan.Judges) == 0 {
		return nil
	}
	// （裁判轮注释两行）
	if ctx.Err() != nil {
		return r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)
	}
	judgeSnap, err := r.rn.TakeRoundSnap(ctx, r.cur.ID)
	...
	judgeRan, err := runOneRound(ctx, r, judgePlan, goRun, "裁判轮", 0, 0, judgeSnap)
	if judgeRan {
		spawned = true
	}
	return err
}
```

整段换成（逐字；原有两行裁判轮注释、`judgePlan` 上方一行注释原样保留、随块缩进一层）：

```go
	if withJudge && len(plan.Judges) > 0 {
		// 裁判只在发言轮循环停下之后跑一次。每个发言轮跟一次就是把一次 flag
		// 收成 N 倍入场费。裁判轮自己再取快照，看见的是发言轮写完之后的历史。
		if ctx.Err() != nil {
			return r.printf("已跑 %d/%d 轮，其余取消\n", finished, rounds)
		}
		judgeSnap, err := r.rn.TakeRoundSnap(ctx, r.cur.ID)
		if err != nil {
			return r.reportErr("round", err)
		}
		// Plan 的三个切片都要非 nil。PlanRound 给出的就是非 nil 的空切片。
		judgePlan := turn.Plan{
			Speakers: plan.Judges,
			Judges:   []string{},
			Skipped:  []turn.SkipReason{},
		}
		judgeRan, err := runOneRound(ctx, r, judgePlan, goRun, "裁判轮", 0, 0, judgeSnap, writeAskRecord, &asked)
		if judgeRan {
			spawned = true
		}
		if err != nil {
			return err
		}
	}
	if noSummary {
		return nil
	}
	return runSummary(ctx, r, asked, sumID)
}
```

（块内的 `judgeSnap, err :=` 在块作用域里声明了新的 `err`，块内 `return err` 返回的是它，赋给命名返回值；原型实测编译、`go vet` 均无报告。）

F5. `runOneRound` 签名（HEAD `:259`）改成（参数名、类型、顺序冻结）：

```go
func runOneRound(ctx context.Context, r *REPL, plan turn.Plan, goRun bool, label string, k, n int, snap turn.RoundSnap, render func(*REPL, turn.Record) error, asked *[]turn.Record) (bool, error) {
```

回调里 `if err := writeAskRecord(r, rec); err != nil {`（HEAD `:329`）改成 `if err := render(r, rec); err != nil {`；`if writeErr != nil { return false, writeErr }`（HEAD `:357-359`）之后加：

```go
	if asked != nil {
		*asked = append(*asked, recs...)
	}
```

F6. 注释（必须改，否则与代码不符）：
- `cmdRound` 头注释里「本片不引入任何配置键，也不引入自动链：上限就是人按一次 /round --go 跑一遍名单。将来的自动链入口必须收一个显式上限，没有显式上限一律 refuse，不给默认值。」（HEAD `:52-54`）：这一句删掉，改写成与既有体例一致的「第六次」段落（放在「第五次」之后）：#45（2026-09-27，Griffin）推翻了「不引入配置键、不引入自动链」——`[summary]` 是配置键，自动总结是自动链；上限是「每条 /round 恰好多一次调用」（最多一个总结段，只在最后），有界、不循环；触发判据见 `summary.go`；总结成员不许出现在名单里（F2）；`--no-summary` 跳过。`--go`、`--rounds`、`--with-judge`、`--no-summary` 四个 flag 都可以出现在 fields 任意位置（原句「三个 flag」改成「四个」）。
- `runOneRound` 函数注释补：`render` 是逐条渲染用的函数（发言轮、裁判轮传 `writeAskRecord`，总结段传 `writeSummaryRecord`；格式仍只有一份，差别只在简洁档的两个头）；`asked` 非 nil 时在 `writeErr` 检查之后把这一段的 `recs` 追加进去（写屏幕失败的那一段不追加，调用方随即返回错误、不会总结）；总结段传 nil。
- CO1 在 `cmdRound` 注释里写的「SM 的位置」那一段（CO1 E7）改成现在时：总结段已经在这里。

#### G. `internal/repl/ask.go`（锚点取 CO1 落地后的真文件）

CO1 §1.1 D1 的 `writeAskRecord` 从函数签名到 `} else if err := r.printf("outcome %s  agent %s\n", outcomeLabel(rec.Outcome), rec.AgentID); err != nil {` 为止的这一段，换成下面这段（逐字）；这一行之后的函数体（`return err` 起到 `return writeAskMeter(r, rec)`、`}`）一个字节不改：

```go
func writeAskRecord(r *REPL, rec turn.Record) error {
	return writeRecord(r, rec, false)
}

func writeSummaryRecord(r *REPL, rec turn.Record) error {
	return writeRecord(r, rec, true)
}

func writeRecord(r *REPL, rec turn.Record, summary bool) error {
	if r.concise {
		name := r.displayName(rec.AgentID)
		if rec.Message != nil {
			head := name + "（" + FormatWall(rec.WallMS) + "）:"
			sep := " "
			if summary {
				head = "总的来看（" + name + " 做总结）："
				sep = ""
			}
			if rec.Message.Text == "" {
				if err := r.write(head + "\n"); err != nil {
					return err
				}
			} else {
				if err := r.write(head + sep); err != nil {
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
		} else {
			line := name + ": 没有回复（" + OutcomeWord(rec.Outcome) + "）\n"
			if summary {
				line = "总的来看：没有总结（" + OutcomeWord(rec.Outcome) + "）\n"
			}
			if err := r.write(line); err != nil {
				return err
			}
		}
	} else if err := r.printf("outcome %s  agent %s\n", outcomeLabel(rec.Outcome), rec.AgentID); err != nil {
```

与 CO1 D1 的差别只有四处：函数名与两个薄包装；`sep`；简洁档成功头的 `summary` 分支；失败行由 `else if err := r.write(...)` 改成 `else { line := ...; if summary {...}; if err := r.write(line) ... }`。`summary == false` 时两档输出与 CO1 D1 逐字节相同；`summary == true` 且详细档时与 `writeAskRecord` 逐字节相同（§2.3 K2 担保）。名字经 `r.write` 拼接，不进 `printf` 格式串（CO1 同一条）。
注释要点：CO1 的函数注释原样保留在 `writeRecord` 上；`writeAskRecord` / `writeSummaryRecord` 各一句说明；`writeRecord` 注释补一段：`summary` 只改简洁档的两个头（#45 规定的「总的来看（<名> 做总结）：」与「总的来看：没有总结（<结局>）」），其余照打的行（可能已吃下、漂移、被拒工具调用、正文写不进库整段）与成员块同一份代码；详细档不看 `summary`（总结段照裁判轮的详细格式打，#45）；为什么是一个参数而不是另写一个函数（`round.go` 那段「两份格式会分叉」）；总结头为什么全角冒号、不带用时（#45 的原话）。

#### H. `internal/repl/observer.go`（锚点取 CO1 落地后的真文件）

`RoundStartEvent` 三行字段注释改成（字段、类型、顺序不变；对齐交给 gofmt）：
- `Label`：`"发言轮"、"裁判轮" 或 "总结轮"，即 runOneRound 的 label 参数`
- `K`：`发言轮：第 K 轮（1 起）；裁判轮与总结轮：0`
- `N`：`发言轮：本条命令的 --rounds 总数；裁判轮与总结轮：0`

#### I. `internal/repl/commands.go`

命令表 `round` 那一行（HEAD `:152`）的 summary 换成（逐字，§3.3），其余字段不变：

```go
		{name: "round", summary: "按名单问一轮，--go 时发言者同时起进程；多个成员跑完自动总结（只给你看）: /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]", needsSvc: true, needsConv: true, needsRunner: true, run: cmdRound},
```

### 1.2 不做（两条腿都不许越界）

- **不改发言成员的 prompt**：`internal/turn` 的 `router.go`、`render.go`、`scheduler.go`、`budget.go`、`plan.go` 一个字不碰；`runner.go` 只加 §1.1 D 那三行。总结成员的 role 由 `config` 补，不在 turn 里拼。
- **不做 `/judge` 命令**；不解析总结的内容；不自动触发第二次；不把总结结果回灌给任何成员。
- **不接 codex**。
- 不改 `internal/tui`（生产）、`internal/app`、`cmd/council`、`internal/conversation`、`internal/store`、`internal/provider`；不改 `go.mod` / `go.sum`；不改 `repl.New` 的签名；不加 REPL 字段。
- 不改 CO1 冻结的其余一切：`writeAskMeter`、`cmdVerbose`、`FormatWall`、`OutcomeWord`、CO1 的 defer 与 `spawned` 判据、`RoundEndEvent`、TUI 适配器。
- 不改 `/log`、`/cost`、`/who`、`/session`、`/agents` 的列与格式（总结消息在 `/log` 里照常出现，`TO` 列就是既有的 `Recipients` 列）。
- **HEAD 既有 `_test.go` 一个字节都不改**（原型实测：本刀落地后 HEAD 的全部既有用例原样全绿）。
- **CO1 测试腿的文件（本刀落地时已是既有用例）授权改五处（W1b 修订：另有一处有条件的第 6 处），只改期望、不改装置与断言结构，由本刀的测试腿交付**（与 §2 的新文件同一次交付；实现腿不碰任何 `_test.go`）。理由：CO1 这五条用例都是「简洁档、没配 `[summary]`、本条命令真问到两个以上成员、有人回复、没取消」，本刀按 #45 在这种情形下多打一行提示（§3.2 第 3 行，下称「提示行」），位置在最后一段之后、CO1 的总用时之前。**这五条是实跑得出的全集**：v1.1 原型上先落 CO1 v1.1 的生产代码与七个 cox 文件（九包全绿），再只落本刀 §1.1 的生产代码、一个 `_test.go` 都不改，跑全量 `./...`，恰好这五条变红（v1 只列了前四条：第 5 条 R5 `round2` 是 CO1 v1.1 新增的用例）。W1b 与 W1b 修订的原型各自复现了这五条（W1b 修订的原型重写的是 CO1 用例的一个子集，见契约头）；另有第 6 条是**有条件的**：它对应 CO1 §7.4 待裁、还没进 CO1 §2 的夹具：
  1. `internal/repl/cox_round_test.go` 的 `TestCoxRoundConciseGolden`（CO1 §2.2 R1）：期望串在 `总用时 7.2s` 那一行之前插入提示行（成员 `cox_a`、`cox_b`、`cox_j` 三个都被问到）；时钟调用次数仍是 2。
  2. 同文件 `TestCoxRoundObserverSuppressesTotal` 的 `concise` 子测试（CO1 R3）：期望是「新的 R1 期望去掉最后一行 `总用时 7.2s`」，即以提示行结尾（装了观察者不打总用时，但提示行照打）。若 CO1 测试腿是从 R1 的常量派生的，只改 R1 一处即可；若是另写的字面量，同样插入提示行。
  3. 同文件 `TestCoxRoundSpawnFailedConcise`（CO1 R6，成员 `cox_a`、`cox_x`）：「恰好 6 行」改为「恰好 7 行」；第 4 行（`分不清进程是否起来 1（…）`）不变；第 5 行整行等于提示行；第 6 行整行 `总用时 7.2s`（原来在第 5 行）。
  4. `internal/tui/cox_concise_test.go` 的 `TestCoxProgramConciseNoBlockOneTotal` 的 `concise` 子测试（CO1 T2）：P「恰好 4 条」改为「恰好 5 条」；第 4 条（下标 3）整行等于提示行；第 5 条是收尾行 `^/round 结束  \d+s  回来 2/2$`（`回来 2/2` 不变：没配总结时没有总结段）。「RE 前紧挨着的是 MD」「窗口里没有任何 P 以 `总用时` 开头」不变。
  5. （v1.1）`internal/repl/cox_round_test.go` 的 `TestCoxRoundNoDeltaConcise` 的 `round2` 子测试（CO1 v1.1 R5，成员 `cox_a`、`cox_x`，慢 a 前置，`/round cox_a cox_x --go --rounds 2`）：第 1 轮问到两个成员、`cox_a` 回复了、没取消，所以四条全中。期望整串在最后一行 `总用时 7.2s` 之前插入提示行，即由 11 行变成 12 行，末尾三行依次是 `分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）`、提示行、`总用时 7.2s`；其余 10 行、`<原因>` 的规范化与守卫、时钟调用次数（2）都不变。
  6. （W1b 修订，**有条件**：只在主代理采纳 CO1 §7.4 第 1 条、把 `judge_spawn_failed` 落进 CO1 R5 时生效；没采纳则这一条作废，见 §7.4）`internal/repl/cox_round_test.go` 的 `TestCoxRoundNoDeltaConcise` 的 `judge_spawn_failed` 子测试（成员 `cox_a` 与起不来的裁判 `cox_y`，`/round cox_a cox_y --go --with-judge`）：送进 `cox_a`（发言轮）与 `cox_y`（裁判轮）两个、`cox_a` 回复了、没取消，四条全中。期望整串由 7 行变 8 行，在 `总用时 7.2s` 之前插入提示行，即整串等于（Go 字符串字面量写法）`"本轮问 1 个: cox_a\n甲（T）: OK\n本轮问 1 个: cox_y\n坏裁判: 没有回复（没起来）\n/round cox_y: <原因>\n分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）\n总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = \"<agent-id>\"；不想看这一行就加 --no-summary）\n总用时 7.2s\n"`；时钟 2 次、原因行的守卫（恰好 1 行）与规范化都不变。W1b 修订原型实跑：只落 CO1 时它照 CO1 §7.4 的 7 行整串绿；落了本刀、不改期望时红，差的正是这一行；改成上面的 8 行之后绿，`-count=10` 零偶发。
  CO1 的其余用例原型实跑不受影响（全绿）：R2 / R3-`verbose` / T2-`verbose`（详细档不打提示）、R4 `plain` 与 `with_judge`（预览，一个成员都没送进去）、R5 的 `mixed`（只问到 `cox_a`）、`only`（没人）、`judge_only`（只问到 `cox_j`）、`judge_nodelta`（没人）、R7-R9（`/ask`、`/verbose`）、R10（写屏幕出错，`cmdRound` 提前返回）、R11 全部（`canceled` 与 `canceled_judge` 在 `cmdRound` 里打完「已跑 k/N 轮，其余取消」就返回、到不了总结；`refusals`、`argv_refusal` 一个都没问；`persist_fail` 是 `/say`；CO1 W1b 修订加的 `canceled_line` 同样在 `round.go:192` 打完「已跑 1/2 轮」就返回，`canceled_judge_line` 在 `:223` 返回，`round_report_err` 在 `:105` 返回，三条都到不了 `runSummary`，W1b 修订原型实跑绿）、A1 `default` / `verbose` / `tui` / `tui_verbose`（`/round c1 --go` 单成员）、L1（含 CO1 W1b 修订加的第三条 `-h`；不走 `/round`）、K1-K5、T1。
- 测试腿不写生产代码；实现腿不写任何 `_test.go`。

---

## 2. 验收（测试腿新写的文件）

通用纪律：

- 新文件共六个（§2.1-§2.6，repl 包两个；v1 写成「七个」是错的）；包级标识符一律以 `smx` 开头，测试函数 `TestSmx...`。**不新写 `TestMain`**：repl 包复用 `session_inflight_test.go:94` 的（假 claude CLI：argv 带 `--resume` 时睡 700ms 再应答，回放 `fakeClaudeSuccessJSON`，正文 `OK`），tui 包复用 `rba_fakecli_test.go` 的，app 包复用 `smoke_test.go:40` 的；turn 包用既有 `fakeSpawner`（`fixtures_test.go:170`），不起进程、不需要 `TestMain`。**`internal/turn` 里一个 `TestMain` 都不许加**（MD1 的测试腿会在 turn 包加本包第一个 `TestMain`，`mdx_discovery_exec_test.go`；一个包只能有一个），本刀 turn 包的用例也不许依赖它（它落没落地，P1 都照样能跑）。
- 不起真 provider：外部命令一律是测试二进制自己（`COUNCIL_FAKE_CLI=1`，`Command` 写 `os.Args[0]`），或一个不存在的路径（造 spawn_failed）。store 一律 `t.TempDir()` 之下，不碰 `C:\Users\25142\.council\data`。**至少一条走真 `app.Run` + 真 store + `config.Load`**（§2.6）；repl 包端到端用例用真 SQLite（`newRoundStreamService`）。
- 断言整行相等、整串相等、切片 `reflect.DeepEqual` 或整行正则（`^…$`，只用于随机器变的时间与 round id 末 6 位）。随机器变的值只允许经下面**冻结的规范化**抹掉：成员头里的 `（\d+\.\ds）` 换成 `（T）`；repl 包预算行 `(?m)^(smx_[a-z]  未标定  未标定  预报\S+  不在飞  )-?\d+(  无样本)$` 换成 `${1}H${2}`；`wall_ms=\d+` 换成 `wall_ms=W`；app 包整行 `^总用时 \d+\.\ds$` 用正则判。v1.1 加一条，只用于 S7 `note_order`：spawn_failed 的原因行 `(?m)^(/round smx_[xk]: ).+$` 换成 `${1}<原因>`（原因文本含临时目录路径与随机 planned_session id；换之前先守卫恰好 2 行，同 CO1 v1.1 R5 `round2` 的做法）。前提守卫（不成立即 `Fatalf` 说前提不成立，不是被测断言）可以用前缀，例如 spawn_failed 的原因行 `/round smx_x: `。
- 每条注释写【判别力】：它能不能为假、只差一处的对照是哪条。
- 允许只读引用同包既有测试辅助（不许改它们）：repl：`newRoundStreamService`（`step10ux1_round_stream_test.go:94`）、`rbaBuf`（`rba_golden_test.go:36`）；tui：`rbaTuiConfig`、`rbaTuiRig`（字段 `svc`、`rn`、`r`、`lw`、`convID`）、`NewLineWriter`、`rbbStartProg`、`(*rbbProg).submit` / `useConv` / `waitFor`、`(*rbbSpy).entries`、`rbbEntry`、`rbbDoneThenFin`、`rbbWindow`、`rbbEventKinds`、`rbbLogDump`；turn：`newTestConfig`、`newTestService`、`newConv`、`joinAgent`、`postUser`、`newTestRunner`、`fakeSpawner`、`claudeT1JSON`、`testAgentClaude`；app：`isolateAppEnv`、`tomlLiteral`、`writeTestConfig`。**不用 `runAppForTest`**（CO1 落地后它固定 `Verbose: true`）。
- 编译依赖（预期的红，测试腿报告里写出每个文件引用了哪些新标识符）：`internal/config/smx_summary_config_test.go` 与 `internal/turn/smx_summary_private_test.go` 只引用本刀新增的 `config.SummaryConfig` / `Config.Summary` / `config.DefaultSummaryRole`，**不依赖 CO1**：在「HEAD + 本刀 §1.1 A-D」上就能编译运行（主代理可以先落 config 与 runner、先跑这两个包）；repl（两个文件）、tui、app 四个文件同时引用本刀的标识符（`writeSummaryRecord`、`Config.Summary`）与 CO1 的标识符（`SetConcise`、REPL 字段 `now`、`RoundEndEvent.Concise`、`app.Options.Verbose`），只在 CO1 与本刀都落地之后编译得过。

### 2.0 共用装置（冻结，逐字节）

**repl 包配置** `smxCfg(summary string) *config.Config`（名字 smx 前缀、实现方定；内容冻结）：`Providers`：`"claude": {Command: os.Args[0]}`、`"grok": {Command: filepath.Join(os.TempDir(), "smx-no-such-dir", "smx-missing.exe")}`；`Agents`（全部 `Effort "high"`、`Role "smx"`、`Permissions config.PermissionRead`，除注明外 `Provider "claude"`、`Model "claude-fable-5-1"`、`Mode config.ModeDiscussion`）：

| id | DisplayName | 其余 |
|---|---|---|
| `smx_a` | `甲` | |
| `smx_b` | `""` | |
| `smx_j` | `裁判 J` | `Mode config.ModeJudge` |
| `smx_s` | `总结 S` | |
| `smx_x` | `坏 X` | `Provider "grok"`、`Model "grok-4.7"`、`Effort ""` |
| `smx_y` | `坏 Y` | 同 `smx_x` |
| `smx_z` | `坏 Z` | 同 `smx_x` |
| `smx_k` | `坏 K` | 同 `smx_x`，另加 `Mode config.ModeJudge`（v1.1，F7：一个起不来的裁判） |

`Summary: config.SummaryConfig{Agent: summary}`。（`smx_k` 只在 S7 `note_order` 里被 Join；加它不牵动任何既有期望，原型全量实测。）

**装置函数** `smxSetup(t, summary, members, concise, slowA)`（名字实现方定）按顺序：`t.Setenv("COUNCIL_FAKE_CLI", "1")`；`ctx` 带 2 分钟超时；`svc := newRoundStreamService(t)`；`rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})`；`svc.Create(ctx, "smx")`（**标题 `smx` 冻结**，S2 的 `prompt_bytes=59` 依赖它）；按给定顺序 `Join` 成员（`Provider` 取配置里的 provider）；`Post` 一条 user 消息 `seed-msg`；`slowA` 为真时再 `rn.Ask(ctx, convID, "smx_a")` 一次、`Post` 一条 user 消息 `seed-msg-2`（于是 `/round` 里 `smx_a` 带 `--resume`、假 CLI 睡 700ms，首次开口的成员立即回来，完成顺序固定为「别人先、甲后」，同 CO1 R1 的做法）；`r := New(cfg, svc, rn, nil, buf, RulesContext{})`（`buf` 是 `*rbaBuf`）；`r.Submit(ctx, "/use "+convID)`；清空缓冲；`concise` 为真时 `r.SetConcise(true)`；`r.now` 换成带锁、计数的假时钟：第 1 次返回 `time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)`，之后每次返回它 `+ 7250*time.Millisecond`（与 CO1 R1 相同，总用时恒为 `7.2s`）。

**提示行**（下文简称）：§3.2 第 3 行。**spawn_failed 结算行**：`分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）`（CO1 的既有字面量）。**拒绝行**：`拒绝: smx_s 是总结成员（config.toml 的 [summary]），不在 /round 里发言；它的话只给你看，其余成员看不到。\n`（S8 起共用）。**在飞原因**：`这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行`（既有 `askCause` 对 A 之五在飞闸的文本，与 CO1 R8 同一句）。

**钩子写者**（v1 的 S10 起用，v1.1 推广；名字实现方定）：嵌 `rbaBuf`，写入照常累积；另带一个前缀串和一个回调，**第一次**写入的内容以该前缀开头时调一次回调（带锁记下已触发，之后不再调；回调在锁外调）。装法：`r.out` 换成它（白盒，同包）。**陈旧在飞记录**（v1.1，S7 `note_refused` / `rounds2_last_record`）：`svc.BeginTurn(ctx, conversation.InflightTurn{TurnID: "smx-stale-turn", ConvID: convID, AgentID: "smx_b", Provider: "claude", StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, Tier: "read"})`（写法同 CO1 R8；TurnID 是固定字面量）。

### 2.1 `internal/config/smx_summary_config_test.go`（`package config`）

装置：配置文本 `smxBase`（冻结，Go 字符串拼接）：

```
[providers.claude]
command = "claude"

[agents.a]
provider = "claude"
model = "m"

[agents.sm]
provider = "claude"
model = "m"

[agents.w]
provider = "claude"
model = "m"
permissions = "workspace"

[agents.f]
provider = "claude"
model = "m"
permissions = "full"

[agents.r]
provider = "claude"
model = "m"
role = "自己写的"
```

各用例在它后面**追加**一段（模拟 Griffin 把表追加到文件末尾），写进 `t.TempDir()` 下的 `config.toml` 再 `Load`。

- **C1 `TestSmxSummaryLoadDefaults`**：四个子测试，`Load` 成功且 `Validate` 为 nil，断言 `(Summary.Agent, Agents["sm"].Role, Agents["a"].Role, Agents["r"].Role)`：`absent`（不追加）→ `("", "", "", "自己写的")`；`appended`（追加 `"\n[summary]\nagent = \"sm\"\n"`）→ `("sm", DefaultSummaryRole 的 §3.1 字面量, "", "自己写的")`；`own_role`（`agent = "r"`）→ `("r", "", "", "自己写的")`；`empty_table`（只追加 `"\n[summary]\n"`）→ `("", "", "", "自己写的")`。
  【判别力】`appended` 同时钉「补了」与「只补总结成员」（`a` 的 role 仍为空）；`own_role` 钉「只补空值」。对照是 `absent`。
- **C2 `TestSmxSummaryValidateErrors`**：五个子测试，追加 `"\n[summary]\nagent = \"<id>\"\n"`、`Load` 成功后 `Validate` 的 `Error()` 整串等于（nil 记作空串）：`ghost` → `summary.agent: undefined agent "ghost"`；`upper`（`SM`）→ `summary.agent: undefined agent "SM"`；`workspace`（`w`）→ `summary.agent: agent "w" must have permissions = "read": the summary runs without being asked`；`full`（`f`）→ 同一句换成 `"f"`；`ok`（`sm`）→ 空串。
  【判别力】取值全集：未定义、大小写不同、`workspace`、`full`、`read` 各一；`full` 杀「写成 `== PermissionWorkspace`」。
- **C3 `TestSmxSummaryUnknownKey`**：追加 `"\n[summary]\nagent = \"sm\"\nmode = \"x\"\n"`，`Load` 的错误整串等于 `"unknown config keys in " + <文件路径> + ": summary.mode"`。【判别力】唯一的键是冻结的；多开一个键这里变绿（与既有未知键拒绝同一条路）。
- **C4 `TestSmxDefaultSummaryRoleBytes`**：`DefaultSummaryRole` 整串等于 §3.1。
- **C5 `TestSmxSummaryThenAppendedAgent`**（v1.1，与 MD1 的协调；W1b 修订换了夹具）：MD1 的 `/add +编号` 把新 agent 表整段追加到 config.toml 末尾；Griffin 的 `[summary]` 也是追加在末尾的。这里直接写字面 TOML，不调 MD1 的代码，但追加段取 MD1 契约 §2.3 C1(a) 期望串的**原样**（W1b 修订：原稿自拟的 `[agents."x"]` 表名不含点号却加了引号、也没有注释行，与 MD1 的真实输出不符）。记 `smxMD1Block` = （Go 字符串字面量写法，冻结）`"# council /add 追加：claude 识别出的 claude-opus-5-5[1m]（订阅）\n[agents.\"opus-5.5-1m-high\"]\ndisplay_name = \"Opus 5.5 1m\"\nprovider = \"claude\"\nmodel = \"claude-opus-5-5[1m]\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"`。两个子测试：
  - `table_after_summary`：`smxBase + "\n[summary]\nagent = \"sm\"\n" + "\n" + smxMD1Block`（中间那个 `"\n"` 是 MD1 `AppendBlock` 的分隔规则：原文以 `\n` 结尾时再补一个 `\n`）。`Load` 成功、`Validate` 为 nil；`Agents["opus-5.5-1m-high"]` 存在且 `DisplayName == "Opus 5.5 1m"`、`Model == "claude-opus-5-5[1m]"`、`Role == ""`；`Summary.Agent == "sm"`；`Agents["sm"].Role` 等于 §3.1（默认 role 照补）。
  - `keys_without_header`（对照）：同一段去掉表头那一行，即 `smxBase + "\n[summary]\nagent = \"sm\"\n" + "\n" + strings.Replace(smxMD1Block, "[agents.\"opus-5.5-1m-high\"]\n", "", 1)`，`Load` 的错误整串等于 `"unknown config keys in " + <文件路径> + ": summary.display_name, summary.effort, summary.mode, summary.model, summary.permissions, summary.provider, summary.role"`（七个键按字典序、`", "` 分隔，W1b 修订原型实跑）。
  【判别力】`table_after_summary` 能为假：追加的键若落进 `[summary]`（没有表头），它们就成了 `summary` 的未知键，`Load` 拒绝 —— `keys_without_header` 与它只差表头那一行，证明同一装置下这条路确实会红，所以前一条的绿说明的是「表头把 `[summary]` 关上了」，不是「什么都会过」。注释行夹在 `[summary]` 之后、表头之前也不碍事（两个子测试都带着它）。这是给集成的守卫，不对应 §1.1 里哪一处可变异的代码。

### 2.2 `internal/turn/smx_summary_private_test.go`（`package turn`）

**P1 `TestSmxSummaryMessageUserOnly`**：两个子测试 `configured`（`Summary.Agent = "sm"`）与 `control`（`Summary.Agent = ""`），其余逐步相同：`cfg := newTestConfig()`；`cfg.Agents["sm"]` 与 `cfg.Agents["late"]` 都等于 `cfg.Agents[testAgentClaude]`；`svc := newTestService(t)`；`conv := newConv(t, svc)`；依次 `joinAgent` `alice`、`sm`、`late`；`r := newTestRunner(t, cfg, svc)`、`r.spawn` 换成一个 `fakeSpawner` 的 `spawn`；然后：
1. `postUser(t, svc, conv, "q1")`；入队 `claudeT1JSON`；`r.Ask(ctx, conv, "alice")`。
2. 入队 `strings.Replace(claudeT1JSON, "\"result\":\"OK\"", "\"result\":\"smx-summary-text\"", 1)`；`srec, _ := r.Ask(ctx, conv, "sm")`。断言 `srec.Message.Seq == 5`；`configured`：`srec.Message.Recipients` 等于 `[]string{"sm"}`；`control`：长度 0。
3. `postUser(..., "q2")`；入队 `claudeT1JSON`；`arec := r.Ask(ctx, conv, "alice")`；断言 `arec.Delivered`、最后一次 spawn 的 `cmd.Stdin` 字节、`len(arec.Message.Recipients) == 0`：
   - `configured`：`[]int64{6}`、`"user: q2"`；
   - `control`：`[]int64{5, 6}`、`"sm: smx-summary-text\n\nuser: q2"`。
4. 入队 `claudeT1JSON`；`lrec := r.Ask(ctx, conv, "late")`（首次开口，bootstrap）；断言 `lrec.Delivered` 与 `cmd.Stdin`：
   - `configured`：`[]int64{1, 6, 7}`、`"turn 包测试会话\n\nuser: q1\n\nuser: q2\n\nalice: OK"`；
   - `control`：`[]int64{1, 5, 6, 7}`、`"turn 包测试会话\n\nuser: q1\n\nsm: smx-summary-text\n\nuser: q2\n\nalice: OK"`。

【判别力】增量（alice 续会话）与 bootstrap（late 首次）两条路都覆盖，且断言的是**真正送进子进程的 stdin 字节**；`control` 证明同一装置下总结文本本来会进 prompt（断言能为假）；步骤 3 的 `Recipients` 长度 0 钉「只给总结成员填，别人不填」。

### 2.3 `internal/repl/smx_summary_record_test.go`（单测，直接调 `writeAskRecord` / `writeSummaryRecord`，不起进程）

装置：`r := New(smxCfg("smx_s"), nil, nil, nil, &b, RulesContext{})`（`b` 是 `strings.Builder`，每次一个新 REPL），按需 `r.SetConcise(...)`。六个 Record（未列字段取零值；v1 五个，W1b 修订加 `summary_fallback_id`）：

| 名字 | Record |
|---|---|
| `spoke_rich` | `AgentID "smx_s"`、`OutcomeSpoke`、`WallMS 6849`、`Fresh true`、`ProviderSession "sess-x"`、`UsageParsed true`、`CostUSDDelta` 指向 `0.5`、`ColdExpected true`、`Message &{Seq 5, SenderID "smx_s", SenderProvider "claude", SenderModel "m", SenderEffort "high", Text "第一行\n第二行\n"}` |
| `spoke_empty` | `"smx_s"`、Spoke、`WallMS 99`、`Message &{Text ""}` |
| `spoke_truncated` | `"smx_s"`、Spoke、`WallMS 60000`、`Message &{Text "半句", Meta {turn.MetaCompletion: "truncated"}}` |
| `timeout_maybe` | `"smx_s"`、`OutcomeTimeout`、`WallMS 300000`、`MaybeAdvanced true`、`ProviderSession "sess-y"` |
| `post_rejected` | `"smx_b"`、`OutcomePostRejected`、`WallMS 1234`、`Drifted true`、`DroppedText "付过钱的\n两行"`、`DeniedActions [{Action "write_file", DisplayName "WriteToFile"}]`、`UsageParsed true`、`ProviderSession "sess-z"` |
| `summary_fallback_id`（W1b 修订） | `"smx_b"`、Spoke、`WallMS 100`、`Message &{Text "B"}`（`smx_b` 在 `smxCfg` 里的显示名是空串） |

- **K1 `TestSmxSummaryRecordConcise`**：简洁档 `writeSummaryRecord`，每个 Record 一个子测试，整串等于：
  - `spoke_rich`：`"总的来看（总结 S 做总结）：第一行\n第二行\n"`
  - `spoke_empty`：`"总的来看（总结 S 做总结）：\n本轮为空\n"`
  - `spoke_truncated`：`"总的来看（总结 S 做总结）：半句\n本轮被截断\n"`
  - `timeout_maybe`：`"总的来看：没有总结（超时）\nprovider 那边可能已经吃下这批内容\n"`
  - `post_rejected`：`"总的来看：没有总结（未入库）\n送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了\nprovider 报了 1 个被拒的工具调用（可能不止这些）：write_file(WriteToFile)。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败\n正文写不进库（已经付过钱），整段如下:\n付过钱的\n两行\n"`
  - `summary_fallback_id`（W1b 修订）：`"总的来看（smx_b 做总结）：B\n"`
  另对 `turn.Outcome` 的 9 个非 Spoke 值 `OutcomeUnknown, OutcomePostRejected, OutcomeProviderFailed, OutcomeUnparsable, OutcomeTimeout, OutcomeSpawnFailed, OutcomeRefused, OutcomeCanceled, OutcomeLost` 各喂 `Record{AgentID: "smx_s", Outcome: o, WallMS: 1234}`，整串等于 `"总的来看：没有总结（" + 词 + "）\n"`，词依次 `未知、未入库、报错、认不出、超时、没起来、拒绝、取消、丢失`；`OutcomePostRejected` 另接 `"正文写不进库（已经付过钱），整段如下:\n"`。
  【判别力】结局值全集都走一遍；`WallMS` 非零，失败头若带用时会多出 `（1.2s）`；冒号若写成 ASCII 或多一个空格，三个 Spoke 子测试红。`summary_fallback_id`（W1b 修订）是总结头里「显示名为空时回落到 id」的唯一夹具（其余夹具的总结成员都有显示名）：头绕过 `displayName`、直接读 `r.cfg.Agents[id].DisplayName`（#90）时印成 `总的来看（ 做总结）：B`，红；它与 #85（注明行的同一种回落）是同类的两处。反方向的 #38（头直接用 id）杀不到它（回落之后与 id 同串），由 `spoke_rich` 等三个 Spoke 子测试杀，两者互补。
- **K2 `TestSmxSummaryRecordParity`**：每个 Record 一个子测试：(a) 详细档 `writeSummaryRecord` 与 `writeAskRecord` 的输出逐字节相等；(b) 简洁档两者去掉「头」之后逐字节相等 —— 头的切法：`Message` 非 nil 且正文非空时，`writeAskRecord` 的输出从第一个 `": "` 之后切、`writeSummaryRecord` 的从第一个 `"："`（U+FF1A）之后切；其余情形两者都从第一个 `"\n"` 起切（含该换行）。
  【判别力】钉住「格式只有一份」：把照打的任何一行只放进其中一条路，(b) 红；详细档若看了 `summary`，(a) 红。（W1b 修订）K2 同样走 `summary_fallback_id`，两档对照照过；它在「：」之后切头，所以 #90 下 K2 的这一个子测试保持绿（原型实测），头的回落只由 K1 钉。

### 2.4 `internal/repl/smx_summary_round_test.go`（端到端：真 SQLite、真 Runner、假 CLI；装置见 §2.0）

**S1 `TestSmxRoundSummaryConciseGolden`**：`smxSetup(t, "smx_s", [smx_a, smx_b], concise=true, slowA=true)`；`/round smx_a smx_b --go` 返回 nil。前提守卫：原始缓冲里 `smx_b（` 的下标小于 `甲（`。主断言：规范化后整串等于

```
本轮问 2 个: smx_a, smx_b
smx_b（T）: OK
甲（T）: OK
本轮问 1 个: smx_s
总的来看（总结 S 做总结）：OK
总用时 7.2s
```

（末尾有换行），且假时钟恰好被调用 2 次。库：`Participants(includeInactive=true)` 里 `smx_s` 存在且 `Active`，且（v1.1，F6）这一行 `DisplayName == "总结 S"`、`Provider == "claude"`（自动 Join 用的是配置里的两个字段）；`History` 里 SenderID 为 `smx_s` 的消息恰好 1 条，`Recipients` 等于 `[]string{"smx_s"}`、`Text` 等于 `OK`；`TurnUsage(ctx, conv, "smx_s", 10)` 恰好 1 行、`RoundID` 非空且不等于 `TurnUsage(ctx, conv, "smx_b", 10)` 那唯一一行的 `RoundID`。然后 `Post` 一条 user 消息 `after`，`rec, err := rn.Ask(ctx, conv, "smx_b")`：`err` 为 nil，`rec.Delivered` 含 `after` 的 seq、**不含**总结消息的 seq。
【判别力】一条用例同时钉：屏幕格式与位置（总结在 CO1 的总用时之前）、自动 Join（v1.1 连同 Join 字面量的 `DisplayName` / `Provider`：写成空串时这里红，§4 #51、#52，原型实测）、库里的 `Recipients`、单独一轮 `round_id`、以及真 Runner 上下一轮不带总结（私密性的端到端）。对照：S3 `no_summary`（只差 flag）、S3 `unconfigured`（只差配置）。

**S2 `TestSmxRoundSummaryVerbose`**：同 S1 但**不**调 `SetConcise`。规范化后按 `"\n"` 切行（去掉末尾空串）；找到最后一个整行 `发言轮结算` 的下标 i，从 i 往后找第一个整行 `成员  输入token  金额  冷启动  在飞  命令行余量  依据` 的下标 j；`lines[j:]` 逐行等于：

```
成员  输入token  金额  冷启动  在飞  命令行余量  依据
smx_s  未标定  未标定  预报冷  不在飞  H  无样本
合计输入token 0（只含已标定的行）
另有 1 名未标定
扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。
本轮问 1 个: smx_s
outcome spoke  agent smx_s
新开了一条 provider 会话 2a90d449-a348-497a-8812-e73e74d6898e
seq 12  smx_s  claude/claude-fable-5-1/high
OK
用量 in=47751 cache_create=47749 cache_read=0 out=4 thinking=0
金额 0.9551999999999999 USD
冷暖 预报=冷 实测=冷  wall_ms=W  prompt_bytes=59  scanned_to=9
总结轮结算
确知起进程 1
分不清进程是否起来 0
用量 输入 47751  输出 4  cache_read 0  cache_create 47749  thinking 0
金额 0.9551999999999999 USD
冷启动 预报 1  实测 1
```

且整串不含 `总的来看`、不含 `总用时`，假时钟恰好被调用 1 次。（`prompt_bytes=59` 是总结成员的 bootstrap：`smx` + 两条 user 种子 + 两个成员各一条 `OK`；`seq 12`、`scanned_to=9` 由 §2.0 装置决定。）（W1b 修订加四条）按行切开后整行 `总结轮结算` 恰好 1 次；库：`Participants(includeInactive=true)` 里 `smx_s` 存在且 `Active`；`History` 里 SenderID 为 `smx_s` 的消息恰好 1 条、`Recipients` 等于 `[]string{"smx_s"}`。
【判别力】「详细档照裁判轮的详细格式、标签 `总结轮`」的逐字节担保；label 写错时 `总结轮结算` 红。（W1b 修订）S2 同时是详细档下各条缺席断言的正对照：同为详细档、同样配了 `smx_s`、同一条命令形状，这里总结照跑 —— `总结轮结算` 出现一次、`smx_s` 被 Join 且激活、落了一条只给自己的消息；S3 `single_verbose` / `no_summary_verbose`、S7 `left_verbose` / `all_fail_verbose`、S10 `canceled_verbose` 的「没有 `总结轮结算`」「smx_s 不在表里 / 仍未激活」「没有 smx_s 的消息」因此都能为假。v1.1 原稿点名 S2 作正对照时它只断言屏幕行，W1b 指出后补上。

**S3 `TestSmxRoundSummaryTriggers`**（v1 七个子测试，v1.1 加一个，W1b 修订加两个）：表驱动，每个子测试 `smxSetup` 后 Submit 一条命令（返回 nil），断言规范化后整串、假时钟调用次数、`smx_s` 是不是参与者（`Participants(includeInactive=true)` 里有没有）：

| 子测试 | summary | 成员 | concise | slowA | 命令 | 整串 | 时钟 | smx_s 在表里 |
|---|---|---|---|---|---|---|---|---|
| `unconfigured` | `""` | a, b | 是 | 是 | `/round smx_a smx_b --go` | `本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n` + 提示行 + `\n总用时 7.2s\n` | 2 | 否 |
| `unconfigured_no_summary` | `""` | a, b | 是 | 是 | `/round smx_a smx_b --go --no-summary` | `本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n总用时 7.2s\n` | 2 | 否 |
| `no_summary` | `smx_s` | a, b | 是 | 是 | `/round smx_a smx_b --go --no-summary` | 同上 | 2 | 否 |
| `single` | `smx_s` | a, b | 是 | 否 | `/round smx_a --go` | `本轮问 1 个: smx_a\n甲（T）: OK\n总用时 7.2s\n` | 2 | 否 |
| `preview` | `smx_s` | a, b | 是 | 否 | `/round smx_a smx_b` | `成员  输入token  金额  冷启动  在飞  命令行余量  依据\nsmx_a  未标定  未标定  预报冷  不在飞  H  无样本\nsmx_b  未标定  未标定  预报冷  不在飞  H  无样本\n合计输入token 0（只含已标定的行）\n另有 2 名未标定\n` | 1 | 否 |
| `with_judge` | `smx_s` | a, j | 是 | 否 | `/round smx_a smx_j --go --with-judge` | `本轮问 1 个: smx_a\n甲（T）: OK\n本轮问 1 个: smx_j\n裁判 J（T）: OK\n本轮问 1 个: smx_s\n总的来看（总结 S 做总结）：OK\n总用时 7.2s\n` | 2 | 是 |
| `ask` | `smx_s` | a, b | 是 | 否 | `/ask smx_a` | `正在问 smx_a ...\n甲（T）: OK\n` | 0 | 否 |
| `with_judge_no_summary`（v1.1，F2） | `smx_s` | a, j | 是 | 否 | `/round smx_a smx_j --go --with-judge --no-summary` | `本轮问 1 个: smx_a\n甲（T）: OK\n本轮问 1 个: smx_j\n裁判 J（T）: OK\n总用时 7.2s\n` | 2 | 否 |
| `single_verbose`（W1b 修订） | `smx_s` | a, b | 否 | 否 | `/round smx_a --go` | 见表后 | 1 | 否 |

`single_verbose` 的整串（Go 字符串字面量写法，W1b 修订原型实跑）：`"发言顺序按字典序，重复的 id 已去重，只问一次。\n成员  输入token  金额  冷启动  在飞  命令行余量  依据\nsmx_a  未标定  未标定  预报冷  不在飞  H  无样本\n合计输入token 0（只含已标定的行）\n另有 1 名未标定\n扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。\n本轮问 1 个: smx_a\noutcome spoke  agent smx_a\n新开了一条 provider 会话 2a90d449-a348-497a-8812-e73e74d6898e\nseq 4  smx_a  claude/claude-fable-5-1/high\nOK\n用量 in=47751 cache_create=47749 cache_read=0 out=4 thinking=0\n金额 0.9551999999999999 USD\n冷暖 预报=冷 实测=冷  wall_ms=W  prompt_bytes=19  scanned_to=1\n发言轮结算\n确知起进程 1\n分不清进程是否起来 0\n用量 输入 47751  输出 4  cache_read 0  cache_create 47749  thinking 0\n金额 0.9551999999999999 USD\n冷启动 预报 1  实测 1\n"`。它的末行 `冷启动 预报 1  实测 1` 与总结段结算块的末行相同，所以不能只断言末行，必须整串相等。

另一个子测试不进上表（断言形状不同）：**`no_summary_verbose`**（W1b 修订）：`smxSetup(t, "smx_s", [smx_a, smx_b], concise=false, slowA=true)`，`/round smx_a smx_b --go --no-summary` 返回 nil。规范化后按 `"\n"` 切行（去掉末尾空串），从最后一个整行 `发言轮结算` 起到末尾逐行等于 `发言轮结算`、`确知起进程 2`、`分不清进程是否起来 0`、`用量 输入 95502  输出 8  cache_read 0  cache_create 95498  thinking 0`、`金额 0.9551999999999999 USD`、`冷启动 预报 1  实测 2`、`意外冷启动 smx_a`（7 行）；整行 `总结轮结算` 0 次；假时钟恰好 1 次；`smx_s` 不在成员表里。

【判别力】`unconfigured` / `no_summary` / `single` / `preview` / `ask` 与 S1 各只差一处（配置、flag、人数、`--go`、命令）；`unconfigured_no_summary` 杀「`--no-summary` 只在配了时才生效」；`with_judge` 钉「裁判算进人数」与「总结在裁判轮之后」。`with_judge_no_summary`（v1.1）是 `withJudge` 与 `noSummary` 同时为真的唯一夹具（其余三格里下面三条变异与原代码行为相同），与 `with_judge` 只差 `--no-summary`：它杀「`--no-summary` 连裁判轮一起跳过」（#48：裁判那两行没了）；也杀反方向的「带裁判时 `--no-summary` 失效」（#82）与「`--no-summary` 早退挪到裁判块之前」（#83）—— smx_a 加 smx_j 算两个成员，`--no-summary` 一失效就多出总结两行、smx_s 被 Join，早退一提前就连裁判两行一起没了（三条原型逐条实测红）。summary 配成 `smx_s` 而不是空串：没配时 #82 只会多打一行提示，「smx_s 不在表里」那条就用不上了。
（W1b 修订）`single_verbose`、`no_summary_verbose` 与各自的简洁档原型（`single`、`no_summary`）只差档位，是判据第 2 条、第 1 条在详细档的唯一夹具（v1.1 原稿带这两个条件的夹具全是简洁档）：判据写成只在简洁档生效（#87 `len(order) < 2 && r.concise`、#86 `noSummary && r.concise`）时，详细档单成员 / 带 `--no-summary` 也去总结，多出整段 `总结轮`、smx_s 被 Join，这两条各自红，其余全绿（原型实测；两条都是花钱方向）。缺席的「没有 `总结轮结算`」「smx_s 不在表里」的正对照是 S2（同为详细档、同样配了 smx_s，那里总结照跑）。

**S4 `TestSmxRoundSummaryVerboseUnconfigured`**：`smxSetup(t, "", [smx_a, smx_b], concise=false, slowA=true)`，`/round smx_a smx_b --go`：最后一行整行 `意外冷启动 smx_a`（CO1 落地后的详细档结算块末行），整串不含 `总的来看`、不含 `总结轮`。【判别力】详细档没配时一行都不多（提示行只在简洁档）。

**S5 `TestSmxRoundSummaryRounds2`**：`smxSetup(t, "smx_s", [smx_a, smx_b], true, false)`，`/round smx_a smx_b --go --rounds 2`：规范化后恰好 9 行；整行 `本轮问 2 个: smx_a, smx_b` 恰好 2 次；整行 `本轮问 1 个: smx_s` 恰好 1 次且是倒数第 3 行；倒数第 2 行 `总的来看（总结 S 做总结）：OK`；最后一行 `总用时 7.2s`。（第二轮两个成员都带 `--resume`，先后不定，所以不断言整串。）【判别力】`--rounds N` 只在最后总结一次。

**S6 `TestSmxRoundSingleMemberTwoRounds`**：`smxSetup(t, "smx_s", [smx_a, smx_x], true, false)`，`/round smx_x --go --rounds 2`：规范化后恰好 9 行；第 0、4 行 `本轮问 1 个: smx_x`；第 1、5 行 `坏 X: 没有回复（没起来）`；第 2、6 行以 `/round smx_x: ` 开头（前提守卫）；第 3、7 行 spawn_failed 结算行；第 8 行 `总用时 7.2s`；`smx_s` 不在成员表里。
【判别力】同一个成员被问两次（两条记录）仍是「单成员」：人数若按记录条数算，会打出「没有总结（这条 /round 没有成员回复）」并多一行。

**S7 `TestSmxRoundSummaryNotesAndFailures`**：十一个子测试（v1 五个；v1.1 加 `left_verbose`、`all_fail_verbose`、`note_verbose`、`note_refused`、`rounds2_last_record`、`note_order`，见本节末尾）。
- `spawnfail_note`：`smxSetup(t, "smx_s", [smx_a, smx_x], true, false)`，`/round smx_a smx_x --go`：恰好 9 行；第 0 行 `本轮问 2 个: smx_a, smx_x`；第 1-3 行里恰好一行整行 `甲（T）: OK`，恰好一行整行 `坏 X: 没有回复（没起来）` 且它不在第 3 行、紧随其后一行以 `/round smx_x: ` 开头（前提守卫）；第 4-8 行依次整行等于：spawn_failed 结算行、`这些成员最后一次没有回复，总结看不到他们这一次的话：坏 X（没起来）`、`本轮问 1 个: smx_s`、`总的来看（总结 S 做总结）：OK`、`总用时 7.2s`。
- `note_multi`：成员 `smx_a, smx_x, smx_y`，`/round smx_a smx_x smx_y --go`：倒数第 4 行整行 `这些成员最后一次没有回复，总结看不到他们这一次的话：坏 X（没起来）、坏 Y（没起来）`，倒数第 2 行 `总的来看（总结 S 做总结）：OK`。（分隔符是顿号 U+3001；顺序是第一次被问到的顺序，即字典序。）
- `all_fail`：成员 `smx_x, smx_y`，`/round smx_x smx_y --go`：恰好 8 行；第 5 行 `分不清进程是否起来 2（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）`；第 6 行 `总的来看：没有总结（这条 /round 没有成员回复）`；第 7 行 `总用时 7.2s`；`smx_s` 不在成员表里（没 Join、没起进程）。
- `summary_fails`：`smxSetup(t, "smx_z", [smx_a, smx_b], true, true)`（总结成员的 provider 起不来），`/round smx_a smx_b --go`：恰好 8 行，第 5 行以 `/round smx_z: ` 开头（前提守卫），逐行等于 `本轮问 2 个: smx_a, smx_b`、`smx_b（T）: OK`、`甲（T）: OK`、`本轮问 1 个: smx_z`、`总的来看：没有总结（没起来）`、（第 5 行原样）、spawn_failed 结算行、`总用时 7.2s`。
- `left`：`smxSetup(t, "smx_s", [smx_a, smx_b, smx_s], true, true)`，随后 `svc.Leave(ctx, convID, "smx_s", time.Now().UTC())`；`/round smx_a smx_b --go`：整串等于 `本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n总的来看：没有总结（smx_s 已退出当前会话；要恢复总结用 /join smx_s）\n总用时 7.2s\n`；之后 `smx_s` 仍是 `Active == false`，库里没有 SenderID 为 `smx_s` 的消息。
【判别力】注明行的显示名、结局词、分隔符、顺序、「只列没回复的」；失败头与原因行；没人回复时不花钱；已退出不拉回。

v1.1 加的六个子测试：

- `left_verbose`（F0）：同 `left` 但**不** `SetConcise`（详细档）。断言：按 `"\n"` 切行（去掉末尾空串）后最后一行整行等于 `总的来看：没有总结（smx_s 已退出当前会话；要恢复总结用 /join smx_s）`；之后 `smx_s` 仍是 `Active == false`；库里没有 SenderID 为 `smx_s` 的消息。（「最后一行」可以这样判，是因为详细档在 `runSummary` 返回之后不再打任何行：CO1 的总用时只在简洁档打，CO1 E2 的判据 `!r.concise`。）
- `all_fail_verbose`（F0）：同 `all_fail` 但详细档。断言：最后一行整行等于 `总的来看：没有总结（这条 /round 没有成员回复）`（理由同上）；`smx_s` 不在成员表里。
- `note_verbose`（F0）：同 `spawnfail_note` 但详细档（成员 `smx_a, smx_x`，`/round smx_a smx_x --go`）。断言：按行切开后，**恰好一行**整行等于 `这些成员最后一次没有回复，总结看不到他们这一次的话：坏 X（没起来）`（整行相等，不是子串）。
- `note_refused`（F3）：`smxSetup(t, "smx_s", [smx_a, smx_b], true, true)`（慢 a）；Submit 之前给 `smx_b` 种一条陈旧在飞记录（§2.0）；`/round smx_a smx_b --go`。规范化后整串等于（8 行，末尾有换行；拒绝不计入「分不清进程是否起来」，所以没有结算行；原因行是确定的在飞原因，不规范化）：

  ```
  本轮问 2 个: smx_a, smx_b
  smx_b: 没有回复（拒绝）
  /round smx_b: <在飞原因>
  甲（T）: OK
  这些成员最后一次没有回复，总结看不到他们这一次的话：smx_b（拒绝）
  本轮问 1 个: smx_s
  总的来看（总结 S 做总结）：OK
  总用时 7.2s
  ```
  （`<在飞原因>` 是 §2.0 冻结的那一句原文，不是占位符规范化。）假时钟恰好 2 次。慢 a 让 `smx_b` 的拒绝行（起进程之前就拒）必在 `甲（` 之前，所以整串逐字。
- `rounds2_last_record`（F4）：`smxSetup(t, "smx_s", [smx_a, smx_b], true, true)`（慢 a）；输出换成钩子写者，前缀 `甲（`，回调是「给 `smx_b` 种陈旧在飞记录」（§2.0，ctx 用 `context.Background()`：回调跑在 turn 的 worker goroutine 上），**只触发一次**；`/round smx_a smx_b --go --rounds 2`。前提守卫：钩子触发过。规范化后整串等于（11 行，末尾有换行）：

  ```
  本轮问 2 个: smx_a, smx_b
  smx_b（T）: OK
  甲（T）: OK
  本轮问 2 个: smx_a, smx_b
  smx_b: 没有回复（拒绝）
  /round smx_b: <在飞原因>
  甲（T）: OK
  这些成员最后一次没有回复，总结看不到他们这一次的话：smx_b（拒绝）
  本轮问 1 个: smx_s
  总的来看（总结 S 做总结）：OK
  总用时 7.2s
  ```
  假时钟恰好 2 次。第 1 轮 `smx_b` 首次开口先回来、已结清，`甲（` 写出时种记录；第 2 轮两人都有新内容、都被问到，`smx_b` 撞上在飞闸被拒，`smx_a` 带 `--resume` 睡 700ms 后回来。
- `note_order`（F7）：`smxSetup(t, "smx_s", [smx_a, smx_x, smx_k], true, true)`（慢 a）；`/round smx_a smx_x smx_k --go --with-judge`。前提守卫：原始输出里以 `/round smx_x: ` 或 `/round smx_k: ` 开头的行恰好 2 行；按通用纪律把这两行的原因换成 `<原因>` 之后整串等于（13 行，末尾有换行）：

  ```
  本轮问 2 个: smx_a, smx_x
  坏 X: 没有回复（没起来）
  /round smx_x: <原因>
  甲（T）: OK
  分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）
  本轮问 1 个: smx_k
  坏 K: 没有回复（没起来）
  /round smx_k: <原因>
  分不清进程是否起来 1（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）
  这些成员最后一次没有回复，总结看不到他们这一次的话：坏 X（没起来）、坏 K（没起来）
  本轮问 1 个: smx_s
  总的来看（总结 S 做总结）：OK
  总用时 7.2s
  ```
  假时钟恰好 2 次。

【判别力】（v1.1 六个）三个详细档子测试与各自的简洁档原型（`left`、`all_fail`、`spawnfail_note`）只差档位：已退出行、「没有成员回复」行、注明行若只在简洁档打（#75、#74、#73），这三条红（原型逐条实测）；其中 #75 是花钱方向，#74 也是（没人回复还去 Join、起进程）。#75 的实际后果（W1b 修订更正，原稿沿用 V0 的「静默拉回」说法不对）：详细档不认已退出，`joined` 照样置真、不 Join，smx_s 保持未激活，总结进程照起，落库时 `RequireActiveParticipant` 把它拒成 post_rejected —— 钱照花，正文整段打在屏幕上（修订原型探针在 #75 下读到：smx_s 仍未激活、库里没有 smx_s 的消息、屏幕有 `本轮问 1 个: smx_s`、`outcome post_rejected  agent smx_s`、「正文写不进库（已经付过钱），整段如下:」与 `总结轮结算`）。所以杀 #75 的是 `left_verbose` 的末行断言（末行变成 `冷启动 预报 1  实测 1`）；它的「没有 smx_s 的消息」「smx_s 仍未激活」两条缺席断言在 #75 下照样成立，守的是「真拉回」一类写法（把它 Join 回来再问）。`left_verbose` 的这两条与 `all_fail_verbose` 里「smx_s 不在表里」的缺席断言，正对照是 S2（同为详细档、同样配了 smx_s、同一条命令形状，只差「smx_s 已退出」/「没人回复」这一个条件：那里总结照跑；W1b 修订给 S2 加了库断言，smx_s 被 Join 且激活、恰好 1 条 smx_s 的消息都显式断言了）。`note_refused` 是注明行走「拒绝」这个结局值的唯一夹具：判据写成只认 `OutcomeSpawnFailed`（#55）时这里红，`spawnfail_note` / `note_multi` 实测保持绿（它们只走了「没起来」）；它同时是注明行里「显示名为空」的唯一夹具（`smx_b` 的 `DisplayName` 是空串）：名字不经 `displayName`、直接读 `r.cfg.Agents[id].DisplayName`（#85）时注明行印成 `：（拒绝）`，红，`spawnfail_note` / `note_multi` 实测保持绿（它们的成员都有显示名；V3 第 4 点）。`rounds2_last_record` 是「前面说过话、最后一次没回复」的唯一夹具：取第一条记录而不是最后一条（#46）时没有注明行，红；`note_refused` 实测保持绿（它的 `smx_b` 只有一条记录）。`note_order` 是「一个发言者和一个裁判都没回复、又有人回复了」的唯一夹具：注明行的先后取自 `asked`（发言轮各段在前、裁判在后），把后一段排到前面（#49）时变成「坏 K（没起来）、坏 X（没起来）」，红。
另：`note_order` 与 v1 的 `summary_fails` 顺带走到了 CO1 §7.2 登记为「没有用例能抓」的 CO1 #62 与 #83（简洁档「分不清」那一行、`/round <id>:` 原因行被加上「只在发言轮」的门时，裁判轮与总结轮那一段的这一行就没了）：原型实测两条变异都被这两个子测试杀掉（W1b 修订补上 #83），见 §4 末尾。#83 下先报的是前提守卫（`note_order` 的「原因行恰好 2 行」、`summary_fails` 的「第 5 行以 `/round smx_z: ` 开头」）；两处的整串 / 逐行主断言本来就含那一行，W1b 修订原型把守卫临时关掉再跑，主断言同样红，所以守卫不升格，结构不动。

**S8 `TestSmxRoundRefusesSummaryMember`**：拒绝行 = `拒绝: smx_s 是总结成员（config.toml 的 [summary]），不在 /round 里发言；它的话只给你看，其余成员看不到。\n`。成员一律 `smx_a, smx_s`（不 slowA）。七个子测试（v1 四个，都是 `concise`；v1.1 加两个详细档、W1b 修订再加一个，见下）：`named`（`/round smx_a smx_s --go`）、`preview`（`/round smx_a smx_s`）、`mention`（先 Submit 裸文本 `看看 @smx_a @smx_s`、清空缓冲，再 `/round --go`）三者整串等于拒绝行、假时钟恰好 1 次、库里没有 SenderID 为 `smx_s` 的消息；`control_unconfigured`（summary 为 `""`，`/round smx_a smx_s --go --no-summary`）整串以 `本轮问 2 个: smx_a, smx_s\n` 开头。
v1.1（F0）加两个详细档子测试（成员同上、**不** `SetConcise`、不 slowA）：`named_verbose`（`/round smx_a smx_s --go`）与 `preview_verbose`（`/round smx_a smx_s`），各自整串等于拒绝行、假时钟恰好 1 次、库里没有 SenderID 为 `smx_s` 的消息。
W1b 修订（W1b-2）再加 `mention_verbose`：成员同上、**不** `SetConcise`、不 slowA；先 Submit 裸文本 `看看 @smx_a @smx_s`、清空缓冲（详细档这一步会打「已记下 seq N」与两行「点名了」，清掉不算），再 `/round --go`；整串等于拒绝行、假时钟恰好 1 次、库里没有 SenderID 为 `smx_s` 的消息。
【判别力】给名单、预览、mention 三条取名单的路都拒；对照证明拒绝来自配置而不是别的。（W1b 修订）三条取名单的路在详细档各有一个子测试（`named_verbose`、`preview_verbose`、`mention_verbose`）。`mention_verbose` 杀「拒绝只在简洁档或给了 id 时生效」（#89：`if id == sumID && (r.concise || len(given) > 0) {`，`given` 是 `cmdRound` 里既有的局部变量，编译得过）：详细档用 mention 推名单时总结成员以发言者身份起进程、花钱，而它的话其余成员看不到；#89 下只有它红，`named_verbose` / `preview_verbose`（都走 `given`）与简洁档的 `mention` 实测保持绿。它在 #84 下保持绿（带 `--go`），在 #76 下红。v1.1 的两个详细档子测试与 `named` / `preview` 只差档位：拒绝只在简洁档生效（#76）时两条都红 —— 这是白花钱方向（详细档让总结成员以发言者身份起进程，它的话别人看不到）。只加 `named_verbose` 挡不住 `if id == sumID && (r.concise || goRun) {`（#84，V0 更正）：`named_verbose` 带 `--go`，在 #84 下实测保持绿，只有 `preview_verbose` 红；两条合起来才覆盖这道门在详细档的两个方向。

**S9 `TestSmxAskSummaryMemberPrivate`**：成员 `smx_a, smx_s`，`/ask smx_s 你怎么看`：规范化后整串 `正在问 smx_s ...\n总结 S（T）: OK\n`；SenderID 为 `smx_s` 的消息恰好 1 条、`Recipients` 等于 `[]string{"smx_s"}`。【判别力】私密按配置判、不按调用路径判；`/ask` 走成员头格式（不是总结头）。

**S10 `TestSmxRoundCanceledNoSummary`**：两个子测试（W1b 修订把 v1 的用例体原样挪进 `concise`，再加 `canceled_verbose`）。
- `concise`（v1 原样）：`smxSetup(t, "smx_s", [smx_a, smx_b], true, true)`；输出换成 smx 自己的写者（嵌 `rbaBuf`；某次写入的内容以 `甲（` 开头时调用 `cancel()`，写入照常累积；v1.1 起就是 §2.0 的钩子写者）；`ctx, cancel := context.WithCancel(<装置 ctx>)`；`r.Submit(ctx, "/round smx_a smx_b --go")` 返回 nil；规范化后整串 `本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n总用时 7.2s\n`；`smx_s` 不在成员表里。
- `canceled_verbose`（W1b 修订，W1b-1）：同 S2 的装置（`smxSetup(t, "smx_s", [smx_a, smx_b], false, true)`，不 `SetConcise`）；钩子写者的前缀换成 `outcome spoke  agent smx_a`（详细档里慢 a 那一块的首行，它最后回来），回调 `cancel`；同样的命令，返回 nil。前提守卫：钩子恰好触发 1 次。断言：规范化后按 `"\n"` 切行，最后一行整行 `意外冷启动 smx_a`（发言轮结算块的末行；详细档在 `runSummary` 之后不再打任何行，同 S7 `left_verbose`）；整行 `总结轮结算` 0 次；`smx_s` 不在成员表里。
【判别力】最后一个成员回来时取消：这一段照常收完，总结不跑（也不 Join）。`concise` 的对照是 S1（只差取消）；`canceled_verbose` 的对照是 S2（只差取消，那里有 `总结轮结算`、smx_s 被 Join）。`canceled_verbose` 是判据第 3 条在详细档的唯一夹具：判据写成只在简洁档生效（#88 `if ctx.Err() != nil && r.concise {`）时，详细档取消之后照走后面几条，第 5 条之后读成员表，ctx 已取消，真 store 报错，末行变成 `round: list participants <convID>: context canceled`（违反「取消后一行都不打」，不花钱），只有它红，`concise` 实测保持绿。

**S11 `TestSmxRoundHelpLine`**：`/help` 的输出里以 `/round ` 开头的那一行，去掉 `/round` 前缀再 `TrimSpace` 后整串等于 §3.3。

**S12 `TestSmxRoundSummaryObserverEvents`**：同 S1 装置与命令，Submit 之前 `r.SetRoundObserver(obs)`，`obs` 是 smx 自己的记录型观察者（带锁；RoundStart 记 `"RS"` 与事件、MemberDone 记 `"MD:" + Label + ":" + AgentID`、RoundEnd 记 `"RE"` 与事件）。断言：规范化后整串等于 S1 的期望去掉最后一行 `总用时 7.2s`；记下的序列恰好 `RS, MD:发言轮:smx_b, MD:发言轮:smx_a, RE, RS, MD:总结轮:smx_s, RE`；第二个 RoundStart 的 `Label "总结轮"`、`K 0`、`N 0`、`Members 1`、`Speakers [smx_s]`；第二个 RoundEnd 的 `Label "总结轮"`、`Concise true`、`Results` 恰好 1 条且 `Outcome == turn.OutcomeSpoke`。
【判别力】TUI 看板靠的三种事件在总结段照发、label 冻结。

**S13 `TestSmxRoundSummaryJudgmentOrder`**（v1.1，F1）：§1.0「什么时候总结」的判据顺序。每个子测试同时偏离**两个**条件（v1 的夹具每个只偏离一个，调换顺序的变异因此全活）。表驱动；每个子测试 `smxSetup(t, <summary>, <成员>, true, <慢 a>)`（一律简洁档）；「离开」= Submit 之前 `svc.Leave(ctx, convID, "smx_s", time.Now().UTC())`；「取消」= S10 的做法（`ctx, cancel := context.WithCancel(<装置 ctx>)`，输出换成钩子写者、前缀 `甲（`、回调 `cancel`，用这个 ctx Submit）；Submit 返回 nil。断言：规范化后的输出（整串，或标明的行）、假时钟调用次数、库里没有 SenderID 为 `smx_s` 的消息。

| 子测试 | summary | 成员 | 慢 a | 离开 | 取消 | 命令 | 规范化后的输出 | 时钟 |
|---|---|---|---|---|---|---|---|---|
| `single_unconfigured` | `""` | a, b | 否 | 否 | 否 | `/round smx_a --go` | 整串 `本轮问 1 个: smx_a\n甲（T）: OK\n总用时 7.2s\n` | 2 |
| `canceled_unconfigured` | `""` | a, b | 是 | 否 | 是 | `/round smx_a smx_b --go` | 整串等于 S10 的期望：`本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n总用时 7.2s\n` | 2 |
| `all_fail_unconfigured` | `""` | x, y | 否 | 否 | 否 | `/round smx_x smx_y --go` | 恰好 8 行；第 5 行 `分不清进程是否起来 2（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）`；第 6 行整行等于提示行；第 7 行 `总用时 7.2s`（两个失败成员的先后不定，同 S7 `all_fail` 只断言这三行） | 2 |
| `left_single` | `smx_s` | a, b, s | 是 | 是 | 否 | `/round smx_a --go` | 整串 `本轮问 1 个: smx_a\n甲（T）: OK\n总用时 7.2s\n` | 2 |
| `left_canceled` | `smx_s` | a, b, s | 是 | 是 | 是 | `/round smx_a smx_b --go` | 整串等于 S10 的期望（同上） | 2 |
| `left_all_fail` | `smx_s` | x, y, s | 否 | 是 | 否 | `/round smx_x smx_y --go` | 恰好 8 行；第 5 行同上；第 6 行整行 `总的来看：没有总结（这条 /round 没有成员回复）`；第 7 行 `总用时 7.2s` | 2 |

【判别力】前三条钉「没配」（第 4 条）与第 2、3、5 条的先后：没配挪到人数之前（#43）→ `single_unconfigured` 多一行提示，红（CO1 的 A1 `default`、R5 `mixed` / `only` / `judge_only` / `judge_nodelta`、R4 两个子测试、R11 `argv_refusal` 同样红 —— 它们都是简洁档、没配、问到不足两个成员；W1 说的「SM1 自己没有这个判别力」由 `single_unconfigured` 补上）；没配挪到取消之前（#44）→ `canceled_unconfigured` 多一行提示，红，S10（v1 的用例体，W1b 修订起是它的 `concise` 子测试）实测保持绿（它配了 smx_s）；没回复挪到没配之前（#45）→ `all_fail_unconfigured` 第 6 行变成「没有成员回复」，红，S7 `all_fail` 实测保持绿。后三条钉「已退出」（第 6 条，它要 `sumID` 非空，与第 4 条的先后看不出来）与第 2、3、5 条的先后：读成员表、判已退出挪到人数之前（#79）→ `left_single` 多一行已退出行，红；挪到没回复之前（#81）→ `left_all_fail` 第 6 行变成已退出行，红；挪到取消之前（#80）→ `left_canceled` 红，**但原型实测它红的原因不是已退出行**：ctx 已取消时真 store 的 `Participants` 返回 `list participants <convID>: context canceled`，屏幕多出的是 `round: list participants …: context canceled` 这一行（`reportErr`），所以既有 S10 与 `canceled_unconfigured` 在 #80、#79 下同样红（W1b 修订重跑：S10 的两个子测试与 S14 `canceled_observed` 也红）。`left_canceled` 的独有判别力只在「读成员表不看 ctx」时才显出来（那时只有它会打出已退出行），留着它守这一种将来。「取消」三条的时钟都是 2：CO1 的 defer 在简洁、起过进程、没装观察者时照打总用时。六条的「库里没有 SenderID 为 `smx_s` 的消息」是缺席断言，正对照是 S1（同为简洁档、同样配了 smx_s、两成员都回复、没取消，那里恰好 1 条 smx_s 的消息；W1b 修订点名）。

「第 1-4 条都不拍快照」由既有 `TestWithJudge*` 两条守着（§0 第 8 条、§4 #34）：它们没配 `[summary]`，在 `runSummary` 里到第 4 条（`sumID` 为空）就返回，碰不到第 5、6 条（W1b 修订更正：原稿写「第 1-6 条」说多了）。第 5 条、第 6 条之前多拍一次快照没有用例能数（W1b 的两条，修订原型实测三包全部 smx / cox / `TestWithJudge*` 用例全绿），行为上无害（多一次读库，不花钱；§0 第 8 条管的只是没配那条路），进 §4「只在审查时看」。「第 1-5 条都不读成员表」没有用例能数（`Service.Participants` 不经 `MaxSeq`，#79 下这两条实测保持绿），它的行为后果由上面的 #79-#81 钉住。

**S14 `TestSmxRoundSummaryObservedGates`**（W1b 修订，修订人自拟的同类项：装了观察者这一维）：装了观察者（`r.roundObs != nil`，TUI 就是这样装的）时 §1.0 的判据照样生效。表驱动；每个子测试 `smxSetup(t, "smx_s", <成员>, true, <慢 a>)`（一律简洁档、配了 smx_s）；Submit 之前 `r.SetRoundObserver(obs)`，`obs` 是 S12 的记录型观察者加一个可选回调（RoundEnd 记下事件、放锁之后调它；S12 不设，行为不变）；「离开」同 S13；「取消」= `ctx, cancel := context.WithCancel(<装置 ctx>)`，回调设为 `cancel`（第一段 RoundEnd 时取消），用这个 ctx Submit。Submit 返回 nil。装了观察者时 CO1 的总用时不打（CO1 E2 的第四条判据），所以期望里都没有 `总用时`。断言：规范化后的输出（整串或标明的行）、记下的事件序列（标明的才断言）、库里没有 SenderID 为 `smx_s` 的消息（`note_observed` 除外）、smx_s 在成员表里的状态（标明的才断言）。

| 子测试 | 成员 | 慢 a | 离开 | 取消 | 命令 | 规范化后的输出 | 事件序列 | smx_s |
|---|---|---|---|---|---|---|---|---|
| `single_observed` | a, b | 否 | 否 | 否 | `/round smx_a --go` | 整串 `本轮问 1 个: smx_a\n甲（T）: OK\n` | `RS, MD:发言轮:smx_a, RE` | 不在表里 |
| `no_summary_observed` | a, b | 是 | 否 | 否 | `/round smx_a smx_b --go --no-summary` | 整串 `本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n` | `RS, MD:发言轮:smx_b, MD:发言轮:smx_a, RE` | 不在表里 |
| `canceled_observed` | a, b | 是 | 否 | 是 | `/round smx_a smx_b --go` | 整串同上 | 同上 | 不在表里 |
| `left_observed` | a, b, s | 是 | 是 | 否 | `/round smx_a smx_b --go` | 整串 `本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n总的来看：没有总结（smx_s 已退出当前会话；要恢复总结用 /join smx_s）\n` | 同上 | 在表里且未激活 |
| `named_observed` | a, s | 否 | 否 | 否 | `/round smx_a smx_s --go` | 整串等于拒绝行 | 不断言（没有事件） | 不断言（装置 Join 过） |
| `all_fail_observed` | x, y | 否 | 否 | 否 | `/round smx_x smx_y --go` | 恰好 7 行；第 5 行 `分不清进程是否起来 2（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）`；第 6 行整行 `总的来看：没有总结（这条 /round 没有成员回复）`（两个失败成员的先后不定，同 S7 `all_fail`） | 不断言 | 不在表里 |
| `note_observed` | a, x | 否 | 否 | 否 | `/round smx_a smx_x --go` | 整行 `这些成员最后一次没有回复，总结看不到他们这一次的话：坏 X（没起来）` 恰好 1 次；最后一行整行 `总的来看（总结 S 做总结）：OK` | 不断言 | 不断言（总结照跑） |

【判别力】S14 各子测试与 S12 只差一个条件（单成员、`--no-summary`、取消、已退出、名单含总结成员、没人回复、有人没回复）；S12 用同一个观察者证明总结段在这种装置下照跑（事件序列多出 `RS, MD:总结轮:smx_s, RE`、smx_s 被 Join、落了消息），所以这里的缺席（没有总结段的事件、smx_s 不在表里、没有 smx_s 的消息）能为假。判据第 1、2、3、5、6 条、名单拒绝、注明行各加一个 `&& r.roundObs == nil`（§4 #91、#92、#93、#94、#95、#96、#97）时，对应的子测试各自红，其余全绿（修订原型逐条实测；加 S14 之前这七条活过全部用例）。其中 #91、#92、#94、#95、#96 是花钱方向（TUI 里 `--no-summary` 失效、单成员也总结、没人回复还 Join 起进程、已退出照起进程再被拒成 post_rejected、总结成员以发言者身份起进程）。这里用 repl 包的记录型观察者代替真 TUI Program：判据只看 `r.roundObs` 是不是 nil，与观察者是谁无关；真 Program 上的对照仍是 §2.5 T1。观察者下名单拒绝的预览 / mention 两条路、真 TUI 里的 Ctrl-C 不另造夹具（§4「只在审查时看」）。

### 2.5 `internal/tui/smx_summary_tui_test.go`（真 `tea.Program`、真 REPL、真 Runner、假 CLI）

装置 `smxNewTuiRig(t, summary)`（名字实现方定）：照 `rbaNewTuiRig` 的步骤（`rba_fakecli_test.go:119-159`）自己搭一份并返回 `*rbaTuiRig`，差别只有：`cfg := rbaTuiConfig()` 之后加 `cfg.Agents["smx_s"]`（等于 `cfg.Agents["rba_a"]` 但 `DisplayName "总结 S"`）与 `cfg.Summary = config.SummaryConfig{Agent: summary}`；只 Join `rba_a`、`rba_b`（不 Join `rba_j`、不 Join `smx_s`）；会话标题 `smx`；种子消息 `seed-msg`。

**T1 `TestSmxProgramSummarySegment`**：四个子测试（v1 三个，v1.1 加 `verbose_unconfigured`）。各自 `g := smxNewTuiRig(t, <summary>)`；需要简洁档的在起 Program 之前 `g.r.SetConcise(true)`；`pr := rbbStartProg(t, g)`；`pr.useConv(t, g.convID)`；`pr.submit("/round rba_a rba_b --go")`；`pr.waitFor(30*time.Second, rbbDoneThenFin(2))` 不成立就 `Fatalf`；`w, _ := rbbWindow(pr.spy.entries(), "/round rba_a rba_b --go")`；`kinds, idx := rbbEventKinds(w)`；`ps` = 窗口里全部 P 的正文。
- `concise`（summary `smx_s`）：`kinds` 恰好 `RS MD MD RE RS MD RE DONE FIN`；`ps` 恰好 6 条：`本轮问 2 个: rba_a, rba_b`、两条整行匹配 `^X（\d+\.\ds）: OK$`、`本轮问 1 个: smx_s`、`总的来看（总结 S 做总结）：OK`、整行匹配 `^/round 结束  \d+s  回来 3/3$`；第二个 RS（`w[idx[4]].rs`）的 `Label "总结轮"`、`Speakers [smx_s]`。
- `verbose`（summary `smx_s`，不 `SetConcise`）：`kinds` 同上；`ps` 里恰好有一条整行匹配 `^总结轮 \([0-9a-f]{6}\)  回来 1/1$`，紧随其后一条整行匹配 `^  完成    smx_s  \d+\.\ds 子进程$`；最后一条整行匹配 `^/round 结束  \d+s  回来 3/3$`。
- `concise_unconfigured`（summary `""`）：`kinds` 恰好 `RS MD MD RE DONE FIN`；`ps` 恰好 5 条，第 4 条（下标 3）整行等于提示行；最后一条整行匹配 `^/round 结束  \d+s  回来 2/2$`。
- `verbose_unconfigured`（v1.1，F0；summary `""`，不 `SetConcise`）：同一个窗口（`rbbWindow(…, "/round rba_a rba_b --go")`，从回显到其后第一个 FIN），`kinds` 恰好 `RS MD MD RE DONE FIN`（与 `concise_unconfigured` 同一串）；`ps` 里**没有**任何一条整行等于提示行；最后一条整行匹配 `^/round 结束  \d+s  回来 2/2$`。
【判别力】看板多一段、收尾行的人数把总结段算进去（`3/3` 对 `2/2`）、详细档段终态块照打；前三个子测试两两只差一处。`verbose_unconfigured` 的「没有提示行」是缺席断言，正对照是 `concise_unconfigured`：同一个装置、同一个窗口、同一个 kinds 串，只差档位，那一条证明这个窗口里确实会出现提示行，所以这里的绿不是窗口取错了。它杀「装了观察者时详细档也打提示行」（`if r.concise || r.roundObs != nil {`，#78）—— 这一格 v1 没人管：v1 的 T1 `verbose` 配了 `[summary]`，S4 是行式（没有观察者），CO1 的 R3-`verbose` / T2-`verbose` 只看事件与块，提示行落在 RE 与 DONE 之间、不在它们的断言里（V0 核过）；原型实测 #78 下只有它红，`concise_unconfigured` 保持绿。

### 2.6 `internal/app/smx_summary_app_test.go`（真 `app.Run`、真 store、真 `config.Load`）

**A1 `TestSmxAppSummaryEndToEnd`**：两个子测试 `verbose=false` / `verbose=true`。各自 `isolateAppEnv(t)`、`t.Setenv("COUNCIL_FAKE_CLI", "1")`、`dir := t.TempDir()`、`dataDir := filepath.Join(dir, "data")`；配置文本（**`[summary]` 追加在文件末尾**，冻结）：

```
data_dir = <tomlLiteral(dataDir)>

[providers.claude]
command = <tomlLiteral(os.Args[0])>

[agents.c1]
display_name = "成员一"
provider = "claude"
model = "opus"
effort = "high"
role = "smx"
permissions = "read"
mode = "discussion"

[agents.c2]
display_name = "成员二"
provider = "claude"
model = "opus"
effort = "high"
role = "smx"
permissions = "read"
mode = "discussion"

[agents.sm]
display_name = "总结"
provider = "claude"
model = "opus"
effort = "high"
permissions = "read"
mode = "discussion"

[summary]
agent = "sm"
```

（`sm` 没写 role。）`writeTestConfig` 写盘；直接调 `Run(context.Background(), Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: <子测试>})`，返回 nil；`script = "/new t\n/join c1\n/join c2\n裸文本\n/round c1 c2 --go\n/log 30\n/quit\n"`。取「尾部」：`out` 按 `"\n"` 切行，每行反复去掉行首 `council> `，从以 `c2 已加入会话 ` 开头的那一行**之后**开始收集，每行做成员头规范化。
- `verbose=false`：尾部第 0 行 `本轮问 2 个: c1, c2`；第 1、2 行排序后等于 `["成员一（T）: OK", "成员二（T）: OK"]`（两人都是首次开口，先后不定）；第 3 行 `本轮问 1 个: sm`；第 4 行 `总的来看（总结 做总结）：OK`；第 5 行整行匹配 `^总用时 \d+\.\ds$`。
- `verbose=true`：尾部（以 `"\n"` 连起来）含整行 `outcome spoke  agent sm` 与整行 `总结轮结算`，不含 `总的来看`、不含 `总用时`。
- 两个子测试：尾部含整行 `10   sm      agent   sm  claude/opus/high  OK`（`/log` 里总结那一行：seq 10、`TO` 列是 `sm`；列宽由这份数据决定，原型两档实测都是这一行）；`<dataDir>\workdir\sm\council-role.txt` 的字节整串等于 `"You are 总结 (id=sm).\nProvider: claude\n" + <§3.1 字面量>`；`<dataDir>\workdir\c1\council-role.txt` 整串等于 `"You are 成员一 (id=c1).\nProvider: claude\nsmx"`。
【判别力】`[summary]` 追加在末尾能被 `Load` 读、`Validate` 放行；默认 role 经真 `Load` 进了真正写给 CLI 的身份文件（而 `c1` 的 role 没被动）；app 默认简洁档下的总结格式；`/log` 看得到、TO 列说明它只给用户看。

---

## 3. 冻结的字面量（逐字节；用户可见的新字面量全部列出）

### 3.1 默认 role（`config.DefaultSummaryRole`，进总结成员的身份块）

```
你是这场讨论的总结人，不参与讨论。每次被叫到时，读用户的问题和各成员的发言，做一致性判断：他们在哪些点上一致，在哪些点上有分歧、各自的依据是什么，有没有人答非所问或明显出错。你的话只给用户看，其余成员看不到；不要替任何一方继续论证，不要提出新的方案。用中文回答：先用一句话给出总的判断，再分条列出一致之处与分歧之处，写得简短。
```

（一行，不含换行；全角逗号、句号、冒号、分号、顿号。）

### 3.2 屏幕上的新行（简洁档；每行末尾一个 `\n`）

```
总的来看（<总结成员显示名> 做总结）：<正文原样>
总的来看：没有总结（<结局词>）
总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = "<agent-id>"；不想看这一行就加 --no-summary）
总的来看：没有总结（这条 /round 没有成员回复）
总的来看：没有总结（<总结成员 id> 已退出当前会话；要恢复总结用 /join <总结成员 id>）
这些成员最后一次没有回复，总结看不到他们这一次的话：<显示名>（<结局词>）、<显示名>（<结局词>）
拒绝: <总结成员 id> 是总结成员（config.toml 的 [summary]），不在 /round 里发言；它的话只给你看，其余成员看不到。
```

- 第 1 行：全角括号 U+FF08 / U+FF09、全角冒号 U+FF1A，冒号后紧接正文第一行（不留空格）；正文为空时冒号后直接换行、下一行 `本轮为空`；截断时正文之后一行 `本轮被截断`（CO1 的 `completionNote`）。`<显示名>` 同 CO1（`r.cfg.Agents[id].DisplayName`，空或没有时是 id）。
- 第 2 行：`<结局词>` 是 CO1 的 `OutcomeWord`；原因在下一行，是既有的 `/round <id>: <原因>`。
- 第 3 行（提示行）只在简洁档打；第 4、5、6 行两档都打；第 7 行两档都打（`拒绝:` 是 ASCII 冒号加空格，与既有 `拒绝: ...` 各句同体例）。
- 第 6 行的分隔符是顿号 U+3001；`<结局词>` 是 `OutcomeWord`。
- 「本轮问 1 个: <总结成员 id>」是既有字面量，两档都打。

详细档：总结段的标签 `总结轮`，于是出现既有格式的 `总结轮结算`（`renderSettlement` 的 `%s结算`）；TUI 段表头与段终态块标题 `总结轮 (<round id 末 6 位>)  回来 k/N`（既有 `segmentCell`）。

### 3.3 `/help` 里 `round` 那一行的 summary

```
按名单问一轮，--go 时发言者同时起进程；多个成员跑完自动总结（只给你看）: /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]
```

### 3.4 `Validate` 的两条错误（`ValidationError.Error()` 的整行，`path: msg`）

```
summary.agent: undefined agent "<id>"
summary.agent: agent "<id>" must have permissions = "read": the summary runs without being asked
```

### 3.5 配置与 flag

- TOML：表名 `summary`，唯一的键 `agent`（字符串）。
- `/round` 的 flag：`--no-summary`（与 `--go`、`--rounds N`、`--with-judge` 一样可以出现在任意位置）。

### 3.6 沿用的既有字面量（不是本刀新增，列出供对照）

`本轮问 %d 个: %s`；`/round %s: %s`；`分不清进程是否起来 %d（不在 PATH、服务睡着、起来后立刻非零退出，三者无法区分）`；`provider 那边可能已经吃下这批内容`；`送出去的会话 id 与回来的不一样：…`；`provider 报了 %d 个被拒的工具调用（可能不止这些）：…`；`正文写不进库（已经付过钱），整段如下:`；`本轮为空`；`本轮被截断`；`%s结算`；`/round 结束  %s  回来 %d/%d`；`总用时 %s`（CO1）；`%s: %v`（`reportErr`）。

---

## 4. 集成员的变异清单

「预期被谁杀」是原型上实跑确认的杀手（子测试名写在括号里）；主代理落地后逐条跑，**从字节副本还原，不用 `git checkout`**。锚点在该文件里恰好一次；全部可编译。

v1.1：#1-#41 在 v1.1 原型（CO1 v1.1 + 本刀）上逐条重跑，点名杀手全部仍红。#43 起是 v1.1 新增的，编号沿用 W1 自拟变异的编号（M43 → #43，余同），W1 没编号的六条给 #79-#84，#85 是 V3 第 4 点那一条；W1 自拟里被 v1 验收已经杀掉的（M50、M54、M58、M62、M65、M71、M72）不再列。新增各行**写全替换前后的文本**（写法同 CO1 v1.1 §4：`\t` 是一个 tab、`\n` 是一个换行；Go 字符串字面量里的 `\n` 在行内注明是源码里的两个字符；表格里的 `\|` 是 markdown 对 `|` 的转义）。新增各条在原型上跑的是 repl / tui / app 三包全部 `TestSmx*`、`TestCox*`、`TestWithJudge*`（加上点名杀手所在的包），「预期被谁杀」列的是**实测变红的全集**，括号里「实测保持绿」的是点名的对照。新增各条的锚点都取单独一行（实现方在函数体里加的注释不会把锚点拆开；`summary.go` 里 `if ctx.Err() != nil {` 只有一处）。三条换序变异（#79-#81）与 #43-#45 用「在锚点之前插入一段判据」写成（后面原位的那一段仍在，在插入点已返回的情形下走不到，行为等于把判据挪了过去），插入的三段原文列在表后。

W1b 修订：#86-#97 是新增的（#86-#90 对应 W1b-1 的 PA / PB / PC、W1b-2、W1b-3；#91-#97 是修订人自拟的「装了观察者」同类项），写法同上，锚点都与既有行同一句（`if noSummary {`、`if len(order) < 2 {`、`if ctx.Err() != nil {`、`if id == sumID {`、`if !replied {`、`if !p.Active {`、`if len(missing) > 0 {` 在各自的文件里都恰好一处；`ask.go` 的 `head = "总的来看（" + name + " 做总结）："` 恰好一处）。它们在修订原型上跑的范围是 repl / tui / app 三包的 `TestSmx*`、`TestCox*`（契约头写明的那个子集）、`TestWithJudge*`，「预期被谁杀」是实测红集的**全集**，每一条都只有一个子测试红。修订原型还按最终的夹具集重跑了 #12、#13、#14、#18、#21、#24、#25、#27、#73-#76、#78-#81、#84（全集写在下面「W1b 修订重跑」一段；#76、#79、#80、#84 的变化同步写进了表里）。#43-#46、#48、#49、#51、#52、#55、#82、#83、#85 没有重跑：W1b 修订的新夹具要么配了 smx_s 且不带裁判（块 N、块 R 在配了时是空操作或与原位同序；没有带裁判或失败裁判的新夹具），要么只有一条记录、成员都有显示名，按代码走不到这些变异的差别（推理，不是实测）。

| # | 文件 | 变异（锚点 → 替换） | 预期被谁杀 |
|---|---|---|---|
| 1 | config/load.go | `ok && agent.Role == "" {` → `ok && agent.Role == "" && false {` | C1（`appended`）、A1（两个子测试） |
| 2 | config/load.go | 整段改成「配了 summary 就给**所有** role 为空的 agent 补默认」（`for id, agent := range cfg.Agents { if agent.Role == "" {...} }`） | C1（`appended`、`own_role`） |
| 3 | config/load.go | `ok && agent.Role == "" {` → `ok {`（覆盖用户写的 role） | C1（`own_role`） |
| 4 | config/validate.go | `add("summary.agent", fmt.Sprintf("undefined agent %q", id))` → `_ = fmt.Sprint(id)` | C2（`ghost`、`upper`） |
| 5 | config/validate.go | `agent.Permissions != PermissionRead {` → `agent.Permissions == PermissionWorkspace {` | C2（`full`） |
| 6 | config/validate.go | `} else if agent.Permissions != PermissionRead {` → `} else if false && agent.Permissions != PermissionRead {` | C2（`workspace`、`full`） |
| 7 | config/schema.go | 默认 role 末尾 `写得简短。"` → `写得简短"` | C4、A1 |
| 8 | turn/runner.go | `msg.Recipients = []string{agentID}` → `msg.Recipients = nil` | P1（`configured`）、S1、S9、A1 |
| 9 | turn/runner.go | `agentID == cfg.Summary.Agent {` → `agentID != cfg.Summary.Agent {` | P1（`configured`）、S1、S9 |
| 10 | turn/runner.go | `msg.Recipients = []string{agentID}` → `msg.Recipients = []string{"user"}` | P1（`configured`）、S1、S9、A1 |
| 11 | repl/round.go | `case "--no-summary":` → `case "--no-summary-x":`（flag 不认，落成 id） | S3（`no_summary`） |
| 12 | repl/round.go | `if noSummary {` → `if noSummary && false {` | S3（`no_summary`） |
| 13 | repl/round.go | `if id == sumID {` → `if id == sumID && false {` | S8（`named`、`preview`、`mention`） |
| 14 | repl/round.go | `if id == sumID {` → `if id == sumID && goRun {` | S8（`preview`） |
| 15 | repl/round.go | 裁判轮 `judgeSnap, writeAskRecord, &asked)` → `judgeSnap, writeAskRecord, nil)` | S3（`with_judge`） |
| 16 | repl/round.go | 发言轮 `k, rounds, snap, writeAskRecord, &asked)` → `k, rounds, snap, writeAskRecord, nil)` | S1、S3（`unconfigured`、`with_judge`）、S7（五个子测试）、T1（三个子测试）、A1 |
| 17 | repl/round.go | `if err := render(r, rec); err != nil {` → `if err := writeAskRecord(r, rec); err != nil {` | S1、S12、T1（`concise`）、A1（`verbose=false`） |
| 18 | repl/summary.go | `if len(order) < 2 {` → `if len(order) < 1 {` | S3（`single`） |
| 19 | repl/summary.go | `if len(order) < 2 {` → `if len(order) < 3 {` | S1、S3（`unconfigured`、`with_judge`）、T1（三个子测试）、A1 |
| 20 | repl/summary.go | `if len(order) < 2 {` → `if len(asked) < 2 {`（按记录条数数人） | S6 |
| 21 | repl/summary.go | `if ctx.Err() != nil {` → `if ctx.Err() != nil && false {` | S10 |
| 22 | repl/summary.go | `if r.concise {` → `if true {`（详细档也打提示行） | S4 |
| 23 | repl/summary.go | `if r.concise {` → `if false {`（简洁档不打提示行） | S3（`unconfigured`）、T1（`concise_unconfigured`） |
| 24 | repl/summary.go | `if !replied {` → `if !replied && false {` | S7（`all_fail`） |
| 25 | repl/summary.go | `if !p.Active {` → `if !p.Active && false {`（拉回已退出的） | S7（`left`） |
| 26 | repl/summary.go | `if !joined {` → `if !joined && false {`（不 Join） | S1、S3（`with_judge`）、T1（`concise`、`verbose`）、A1 |
| 27 | repl/summary.go | `if len(missing) > 0 {` → `if len(missing) > 99 {` | S7（`spawnfail_note`、`note_multi`） |
| 28 | repl/summary.go | `strings.Join(missing, "、")` → `strings.Join(missing, "，")` | S7（`note_multi`） |
| 29 | repl/summary.go | `r.displayName(id)+"（"` → `id+"（"` | S7（`spawnfail_note`、`note_multi`） |
| 30 | repl/summary.go | `if rec := last[id]; rec.Message == nil {` → `if rec := last[id]; rec.Message == nil \|\| rec.Message != nil {` | S1、S7（`spawnfail_note`、`note_multi`、`summary_fails`） |
| 31 | repl/summary.go | `snap, writeSummaryRecord, nil)` → `snap, writeAskRecord, nil)` | S1、S12、T1（`concise`）、A1（`verbose=false`） |
| 32 | repl/summary.go | `const summaryLabel = "总结轮"` → `"裁判轮"` | S2、S12、T1（`concise`、`verbose`） |
| 33 | repl/summary.go | `const summaryLabel = "总结轮"` → `"总结"` | S2、S12、T1（`concise`、`verbose`） |
| 34 | repl/summary.go | `if sumID == "" {` 之前插一行 `_, _ = r.rn.TakeRoundSnap(ctx, r.cur.ID)`（没配时也拍快照） | 既有 `TestWithJudgeRunsOneExtraRoundWithOwnID`、`TestWithJudgeRunsOnlyOnceAcrossMultipleRounds` |
| 35 | repl/ask.go | `" 做总结）："` → `" 做总结）:"`（ASCII 冒号） | S1、T1（`concise`）、A1（`verbose=false`）、K1（三个 Spoke 子测试）、K2（`spoke_rich`、`spoke_truncated`） |
| 36 | repl/ask.go | `sep = ""` → `sep = " "` | S1、T1（`concise`）、A1（`verbose=false`） |
| 37 | repl/ask.go | `line = "总的来看：没有总结（" + OutcomeWord(rec.Outcome) + "）\n"` → `line = name + ": 没有回复（" + OutcomeWord(rec.Outcome) + "）\n"` | S7（`summary_fails`）、K1（`timeout_maybe`、`post_rejected`） |
| 38 | repl/ask.go | `head = "总的来看（" + name` → `head = "总的来看（" + rec.AgentID` | S1、T1（`concise`）、A1（`verbose=false`） |
| 39 | repl/ask.go | `"总的来看：没有总结（" + OutcomeWord(rec.Outcome)` → `"总的来看：没有总结（" + outcomeLabel(rec.Outcome)` | K1（`timeout_maybe`、`post_rejected` 与九个结局值） |
| 40 | repl/round.go | `if noSummary {` → `if noSummary && sumID != "" {` | S3（`unconfigured_no_summary`） |
| 41 | repl/commands.go | `round` 的 summary 改回 HEAD 原文 `按名单问一轮，--go 时发言者同时起进程: /round [id...] [--go]` | S11 |
| 42 | repl/summary.go | `Active:      true,` → `Active:      false,` | **等价**（见下） |
| 43 | repl/summary.go | 判据顺序「没配 → 人数 → 取消」（F1 的 M43）：`\tif len(order) < 2 {\n` → 块 N + `\tif len(order) < 2 {\n` | S13（`single_unconfigured`、`canceled_unconfigured`）；CO1 的 A1（`default`）、R5（`mixed`、`only`、`judge_only`、`judge_nodelta`）、R4（`plain`、`with_judge`）、R11（`argv_refusal`） |
| 44 | repl/summary.go | 判据顺序「人数 → 没配 → 取消」（M44）：`\tif ctx.Err() != nil {\n` → 块 N + `\tif ctx.Err() != nil {\n` | S13（`canceled_unconfigured`）（S10 实测保持绿） |
| 45 | repl/summary.go | 判据顺序「没回复 → 没配」（M45）：`\tif sumID == "" {\n` → 块 R + `\tif sumID == "" {\n` | S13（`all_fail_unconfigured`）（S7 `all_fail` 实测保持绿） |
| 46 | repl/summary.go | 取第一条记录而不是最后一条（F4 的 M46，等于把赋值挪进 `!seen` 分支）：`\t\tlast[rec.AgentID] = rec\n` → `\t\tif _, seen := last[rec.AgentID]; !seen {\n\t\t\tlast[rec.AgentID] = rec\n\t\t}\n` | S7（`rounds2_last_record`）（`note_refused` 实测保持绿） |
| 48 | repl/round.go | `--no-summary` 连裁判轮一起跳过（F2 的 M48）：`if withJudge && len(plan.Judges) > 0 {` → `if withJudge && len(plan.Judges) > 0 && !noSummary {` | S3（`with_judge_no_summary`） |
| 49 | repl/round.go | 跨段顺序反过来（F7 的 M49）：`*asked = append(*asked, recs...)` → `*asked = append(append([]turn.Record{}, recs...), *asked...)` | S7（`note_order`、`rounds2_last_record`） |
| 51 | repl/summary.go | Join 的 Provider 写空（F6 的 M51）：`Provider:    ac.Provider,` → `Provider:    "",` | S1 |
| 52 | repl/summary.go | Join 的 DisplayName 写空（F6 的 M52）：`DisplayName: ac.DisplayName,` → `DisplayName: "",` | S1 |
| 55 | repl/summary.go | 注明行只认「没起来」（F3 的 M55）：`if rec := last[id]; rec.Message == nil {` → `if rec := last[id]; rec.Outcome == turn.OutcomeSpawnFailed {` | S7（`note_refused`、`rounds2_last_record`）（`spawnfail_note`、`note_multi` 实测保持绿） |
| 73 | repl/summary.go | 注明行只在简洁档打（F0 的 M73）：`if len(missing) > 0 {` → `if len(missing) > 0 && r.concise {` | S7（`note_verbose`） |
| 74 | repl/summary.go | 「没有成员回复」只在简洁档打，详细档照样 Join、起进程（M74）：`if !replied {` → `if !replied && r.concise {` | S7（`all_fail_verbose`） |
| 75 | repl/summary.go | 已退出只在简洁档拦，详细档不认已退出：照起总结进程、落库被拒成 post_rejected，钱照花（M75；W1b 修订更正了「静默拉回」的说法，见 S7【判别力】）：`if !p.Active {` → `if !p.Active && r.concise {` | S7（`left_verbose`，红在末行断言） |
| 76 | repl/round.go | 拒绝总结成员只在简洁档生效（M76）：`if id == sumID {` → `if id == sumID && r.concise {` | S8（`named_verbose`、`preview_verbose`、`mention_verbose`；最后一条是 W1b 修订加的，修订原型重跑的全集） |
| 78 | repl/summary.go | TUI 详细档没配时也打提示行（M78）：`if r.concise {` → `if r.concise \|\| r.roundObs != nil {` | T1（`verbose_unconfigured`）（`concise_unconfigured` 实测保持绿） |
| 79 | repl/summary.go | 判据顺序「已退出 → 人数」（V1）：`\tif len(order) < 2 {\n` → 块 L + `\tif len(order) < 2 {\n` | S13（`left_single`、`left_canceled`、`canceled_unconfigured`、`left_all_fail`）、S10（`left_canceled`、`canceled_unconfigured` 与 S10 红在 `round: list participants …: context canceled` 那一行，见 S13 的【判别力】；`left_single`、`left_all_fail` 红在已退出行顶替了原来那一行）（`TestWithJudge*` 两条实测保持绿）。W1b 修订重跑的全集：S13 这四条、S10 的 `concise` 与 `canceled_verbose`、S14 `canceled_observed`（后三条都红在 `context canceled` 那一行） |
| 80 | repl/summary.go | 判据顺序「已退出 → 取消」（V1）：`\tif ctx.Err() != nil {\n` → 块 L + `\tif ctx.Err() != nil {\n` | S13（`left_canceled`、`canceled_unconfigured`、`left_all_fail`）、S10（前两条与 S10 红在 `context canceled` 那一行；`left_all_fail` 红在已退出行顶替了「没有成员回复」）。W1b 修订重跑的全集：S13 这三条、S10 的 `concise` 与 `canceled_verbose`、S14 `canceled_observed` |
| 81 | repl/summary.go | 判据顺序「已退出 → 没回复」（V1）：`\tif !replied {\n` → 块 L + `\tif !replied {\n` | S13（`left_all_fail`） |
| 82 | repl/round.go | 带裁判时 `--no-summary` 失效（V2）：`if noSummary {` → `if noSummary && !withJudge {` | S3（`with_judge_no_summary`） |
| 83 | repl/round.go | `--no-summary` 早退挪到裁判块之前（V2）：`\tif withJudge && len(plan.Judges) > 0 {\n` → `\tif noSummary {\n\t\treturn nil\n\t}\n\tif withJudge && len(plan.Judges) > 0 {\n` | S3（`with_judge_no_summary`） |
| 84 | repl/round.go | 拒绝在详细档只在 `--go` 时生效（V0）：`if id == sumID {` → `if id == sumID && (r.concise \|\| goRun) {` | S8（`preview_verbose`）（`named_verbose` 实测保持绿；W1b 修订加的 `mention_verbose` 带 `--go`，重跑实测同样保持绿） |
| 85 | repl/summary.go | 注明行直接读配置里的显示名、不回落到 id（V3 第 4 点）：`r.displayName(id)+"（"` → `r.cfg.Agents[id].DisplayName+"（"` | S7（`note_refused`、`rounds2_last_record`）（`spawnfail_note`、`note_multi` 实测保持绿） |
| 86 | repl/round.go | 详细档 `--no-summary` 失效（W1b-1 的 PA，花钱）：`if noSummary {` → `if noSummary && r.concise {` | S3（`no_summary_verbose`）（S2、S3 `no_summary`、`single_verbose`、`with_judge_no_summary` 实测保持绿） |
| 87 | repl/summary.go | 详细档单成员也总结（W1b-1 的 PB，花钱）：`if len(order) < 2 {` → `if len(order) < 2 && r.concise {` | S3（`single_verbose`）（S2、S3 `single` 实测保持绿） |
| 88 | repl/summary.go | 详细档取消之后照走后面几条（W1b-1 的 PC）：`if ctx.Err() != nil {` → `if ctx.Err() != nil && r.concise {` | S10（`canceled_verbose`，末行变成 `round: list participants <convID>: context canceled`）（S10 `concise`、S2 实测保持绿） |
| 89 | repl/round.go | 拒绝只在简洁档或给了 id 时生效，详细档 mention 推出来的名单不拒（W1b-2，花钱）：`if id == sumID {` → `if id == sumID && (r.concise \|\| len(given) > 0) {` | S8（`mention_verbose`）（`mention`、`named_verbose`、`preview_verbose` 实测保持绿） |
| 90 | repl/ask.go | 总结头绕过 `displayName`、不回落到 id（W1b-3）：`head = "总的来看（" + name + " 做总结）："` → `head = "总的来看（" + r.cfg.Agents[rec.AgentID].DisplayName + " 做总结）："` | K1（`summary_fallback_id`）（K2 `summary_fallback_id`、S1、S7 `spawnfail_note` / `note_refused` 实测保持绿） |
| 91 | repl/round.go | 装了观察者时 `--no-summary` 失效（W1b 修订自拟，花钱）：`if noSummary {` → `if noSummary && r.roundObs == nil {` | S14（`no_summary_observed`）（S12 实测保持绿） |
| 92 | repl/summary.go | 装了观察者时单成员也总结（花钱）：`if len(order) < 2 {` → `if len(order) < 2 && r.roundObs == nil {` | S14（`single_observed`）（S12 实测保持绿） |
| 93 | repl/summary.go | 装了观察者时取消之后照走：`if ctx.Err() != nil {` → `if ctx.Err() != nil && r.roundObs == nil {` | S14（`canceled_observed`）（S12 实测保持绿） |
| 94 | repl/summary.go | 装了观察者时没人回复照 Join、起进程（花钱）：`if !replied {` → `if !replied && r.roundObs == nil {` | S14（`all_fail_observed`）（S12 实测保持绿） |
| 95 | repl/summary.go | 装了观察者时不认已退出，照起进程再被拒成 post_rejected（花钱）：`if !p.Active {` → `if !p.Active && r.roundObs == nil {` | S14（`left_observed`）（S12 实测保持绿） |
| 96 | repl/round.go | 装了观察者时名单拒绝失效（花钱）：`if id == sumID {` → `if id == sumID && r.roundObs == nil {` | S14（`named_observed`）（S12 实测保持绿） |
| 97 | repl/summary.go | 装了观察者时不打注明行：`if len(missing) > 0 {` → `if len(missing) > 0 && r.roundObs == nil {` | S14（`note_observed`）（S12 实测保持绿） |

表中插入的三段（逐字；`\n` 在 `printf` 字面量里是源码里的两个字符）：

块 N（第 4 条「没配」的原文）：

```go
	if sumID == "" {
		if r.concise {
			return r.printf("%s\n", msgSummaryNotConfigured)
		}
		return nil
	}
```

块 R（第 5 条「没回复」的原文）：

```go
	if !replied {
		return r.printf("%s\n", msgSummaryNoReply)
	}
```

块 L（第 6 条「已退出」：读成员表、出错走 `reportErr`、已退出就打那一行返回；与原位那一段同样的读法与出错处理，只是换了变量名以免重复声明）：

```go
	ps, perr := r.svc.Participants(ctx, r.cur.ID, true)
	if perr != nil {
		return r.reportErr("round", perr)
	}
	for _, p := range ps {
		if p.AgentID == sumID && !p.Active {
			return r.printf("总的来看：没有总结（%s 已退出当前会话；要恢复总结用 /join %s）\n", sumID, sumID)
		}
	}
```

**W1b 修订重跑**（按最终的夹具集，范围同上，全集；v1 的 #1-#41 在表里写的是点名杀手，这里补上新夹具带来的红，表里不改）：
- #12：S3 `unconfigured_no_summary`、`no_summary`、`with_judge_no_summary`、`no_summary_verbose`；S14 `no_summary_observed`。
- #13：S8 `named`、`preview`、`mention`、`named_verbose`、`preview_verbose`、`mention_verbose`；S14 `named_observed`。
- #14：S8 `preview`、`preview_verbose`（`mention_verbose` 带 `--go`，保持绿）。
- #18：S3 `single`、`single_verbose`；S6；S13 `single_unconfigured`、`left_single`；S14 `single_observed`；CO1 R5 `mixed`、`judge_only` 与 A1 `default`（没配、简洁、问到一个成员，多一行提示）。
- #21：S10 `concise`、`canceled_verbose`；S13 `canceled_unconfigured`、`left_canceled`；S14 `canceled_observed`。
- #24：S7 `all_fail`、`all_fail_verbose`；S13 `left_all_fail`；S14 `all_fail_observed`。
- #25：S7 `left`、`left_verbose`；S14 `left_observed`。
- #27：S7 `spawnfail_note`、`note_multi`、`note_verbose`、`note_refused`、`rounds2_last_record`、`note_order`；S14 `note_observed`。
- #73、#74、#75、#78、#81：红集与表里相同（各一个子测试）。#76、#79、#80、#84 的变化已写进表里。

**判定为等价、不列杀手的变异**（写在这里，免得被当成漏测）：
- #42 `Join` 的 `Participant.Active` 写成 false：`AddParticipant` 的 SQL 无条件写 `active = 1`（`internal/store/tx.go:181-189`），`Active` 字段不落库。冻结 `true` 是为了与 `cmdAdd` 的字面量同形。原型实测 S1、T1、A1 全绿。
- `runner.go` 的 `cfg.Summary.Agent != "" &&` 去掉：`agentID` 恒非空（`askUpToWith` 由已铸 turn id 的调用方给出，配置里没有空 id），两者同真同假。
- `runOneRound` 里 `*asked = append(...)` 挪到 `writeErr` 检查之前：写屏幕失败时 `cmdRound` 随即返回错误、不会走到 `runSummary`，多追加的记录没人读。
- `runSummary` 里 `TakeRoundSnap` 与 `Join` 对调：Join 不写消息、不改 `MaxSeq`，快照相同。
- （v1.1，F6 的「同类」）Join 字面量的 `ConvID:      r.cur.ID,` 写成 `""`：`Service.Join` 无条件用参数覆盖 `p.ConvID`（`internal/conversation/service.go:105`）。`JoinedAt:    time.Now().UTC(),` 写成 `time.Time{}`：`AddParticipant` 对零值补 `time.Now().UTC()`（`internal/store/tx.go:173-176`）。两条原型实测 repl / tui / app 全部 smx / cox 用例绿。

**只在审查时看（没有用例能确定地抓，集成员收割时逐字核对）**（v1.1：原第 1 条「取最后一条记录」由 S7 `rounds2_last_record` 接管、挪进变异表成为 #46，删去；加第 1 条）：
- （v1.1，V3）注明行的判据必须**恰好**是 `rec.Message == nil`，不许写成枚举结局的形式：`!rec.Outcome.Ingested()`（`Ingested` 对 Spoke 与 PostRejected 为真）会漏掉 PostRejected（没落库、`Message` 为 nil，却不注明），`rec.Outcome == SpawnFailed || rec.Outcome == Refused` 之类漏得更多。假 CLI 造不出 PostRejected（要库写失败），原型实测 `!rec.Outcome.Ingested()` 活过 repl / tui / app 全部 smx / cox 用例。
- `Participants` / `TakeRoundSnap` / `Join` 出错那三支走 `reportErr("round", err)`：要注入库错误，本刀不另写用例；核对三处都是 `return r.reportErr(...)`，且没有吞掉错误继续起进程。（v1.1 实测旁证：ctx 已取消时 `Participants` 真会出错，§2.4 S13 的【判别力】；原位那一支在今天的判据顺序下走不到取消，所以仍只能审查。）
- `runSummary` 用的 `sumID` 来自 `snap0.Cfg`、Join 用的 `display_name` / `provider` 来自新快照的 `snap.Cfg`：同一条命令里两者是同一个指针，用例分不出。
- F6 / G / H 的注释要点齐全、没有与代码不符的话：`cmdRound` 头注释「本片不引入任何配置键，也不引入自动链…」已改写成「第六次」段落、「三个 flag」已改成「四个」；CO1 写的「SM 的位置」已改成现在时；`RoundStartEvent` 三行注释已改；`runner.go` 那三行上方写了为什么是 `[自己]`。
- `internal/turn` 除 `runner.go` 那三行外零改动（`git diff --stat` 里 `internal/turn/` 只有 `runner.go`，且 `+3`（不计注释））；送给**发言成员**的 prompt 一个字节不变（P1 `configured` 的 alice / late 两步与 `control` 的差别只在总结那条消息）。
- `writeRecord` 在 `summary == false` 时与 CO1 D1 逐字节相同：`git diff` 看 ask.go 只有 §1.1 G 列的四处差别。
- （W1b 修订，W1b-8）第 5 条、第 6 条之前多拍一次快照：`summary.go` 在 `\tif !replied {\n` 之前、或在 `\tparts, err := r.svc.Participants(ctx, r.cur.ID, true)\n` 之前插一行 `\t_, _ = r.rn.TakeRoundSnap(ctx, r.cur.ID)\n`。数 `MaxSeq` 的两条既有用例没配 `[summary]`、到第 4 条就返回，碰不到这里；修订原型实测两条都活过三包全部 smx / cox / `TestWithJudge*` 用例。行为上无害（多一次读库，不花钱）。核对 `runSummary` 里 `TakeRoundSnap` 只出现一次、在已退出那一段之后。
- （W1b 修订，自拟同类项的余项）装了观察者时名单拒绝的另两条路（预览、mention）与真 TUI 里的 Ctrl-C：S14 只用 `named_observed` 与「RoundEnd 回调里取消」代表。给这两条路另加观察者条件、或只在真 TUI 取消时不认取消的写法没有夹具；核对 `round.go` F2 与 `summary.go` 里没有任何 `roundObs`（§6 第 2 步逐字核对）。

**与 CO1 #62、#83 的交叉（v1.1，F7 顺带；W1b 修订补上 #83，给主代理）**：CO1 v1.1 §4 #62（`round.go` 的 `\t\tif settled.SpawnFailed > 0 {` → `\t\tif settled.SpawnFailed > 0 && label == "发言轮" {`）与 CO1 W1b 修订登记的 #83（`round.go` 的 `\t\tif rec.Err != nil {\n\t\t\tif err := r.printf("/round %s: %s\n", rec.AgentID, askCause(rec.Err)); err != nil {` → `\t\tif rec.Err != nil && (!r.concise || label == "发言轮") {\n\t\t\tif err := r.printf("/round %s: %s\n", rec.AgentID, askCause(rec.Err)); err != nil {`，printf 字面量里的 `\n` 是源码里的两个字符）在 CO1 自己的 §2 用例下都没有杀手（CO1 §7.2）。本刀落地之后，本刀的 S7 `note_order`（裁判 `smx_k` 起不来，裁判轮那一段的结算行与原因行）与 v1 就有的 S7 `summary_fails`（总结成员 `smx_z` 起不来，总结轮那一段的结算行与原因行）都走到这两道门：原型实测两条变异下这两个子测试都红（另外 CO1 §7.4 待裁的 `judge_spawn_failed` 也红）。#83 下先报的是两处前提守卫（`note_order` 的「原因行恰好 2 行」变成 1 行；`summary_fails` 的「第 5 行以 `/round smx_z: ` 开头」不成立，因为那一行没了）；W1b 修订原型把两处守卫临时关掉再跑，`note_order` 的整串断言与 `summary_fails` 的逐行断言同样红（它们本来就含那一行），所以杀手是用例本身，不是只靠守卫。CO1 的 §7.2 可以在本刀落地后改成「由 SM1 的 S7 `note_order` / `summary_fails` 杀」；本契约不改 CO1 的文件。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- **基线**：本刀在 CO1 落地之后派工。主代理会把 CO1 落地后的 `internal/repl/round.go`、`internal/repl/ask.go`、`internal/repl/observer.go`、`internal/repl/commands.go` 全文交给你；这四个文件的锚点**从那份全文**逐字复制。`internal/config/schema.go`、`load.go`、`validate.go`、`internal/turn/runner.go` 的锚点从 HEAD `ab53c0b` 真文件逐字复制（CO1 不碰它们）。
- 只许改这 9 个生产文件：新文件 `internal/repl/summary.go`；修改 `internal/config/schema.go`、`internal/config/load.go`、`internal/config/validate.go`、`internal/turn/runner.go`、`internal/repl/round.go`、`internal/repl/ask.go`、`internal/repl/observer.go`、`internal/repl/commands.go`。**不写任何 `_test.go`**（§1.2 授权的 CO1 用例期望更新由测试腿做）。不改 `go.mod`。生产改动原型实测约 230 行（含新文件约 100 行），**一次交付，不拆 A / B**。
- 补丁块，每个文件可以有多个：

  ```
  ===PATCH: internal/repl/round.go===
  <<<<ANCHOR
  （从基线真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点必须逐字复制（本文代码块里的缩进可能被排成空格，以真文件为准），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。注意：`round.go` 里 `return r.reportErr("round", err)`、`if err != nil {`、`if judgeRan {` 出现不止一次，锚点要带上相邻的独有行；F4 整段替换时锚点取 CO1 落地后从 `if !withJudge || len(plan.Judges) == 0 {` 到函数末尾 `}` 的原文（或拆成互不重叠、各自恰好一次的几段）；`ask.go` 的 G 从 `func writeAskRecord(r *REPL, rec turn.Record) error {` 到 `} else if err := r.printf("outcome %s  agent %s\n", outcomeLabel(rec.Outcome), rec.AgentID); err != nil {` 整段替换。
- 新文件用 `===FILE: internal/repl/summary.go===` 换行接完整内容、末尾 `===END FILE===`。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- 代码必须能 `gofmt`（tab 缩进）；`go vet` 不许有新报告。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（含 U+FF08 / U+FF09 全角括号、U+FF1A 全角冒号、U+FF1B 全角分号、U+FF0C 全角逗号、U+3001 顿号、U+3002 句号、U+300C / U+300D 直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing（既有注释里的 `──`、`——` U+2014 放行）。本刀新增的字符串字面量只有 §3.1-§3.4、`summaryLabel` 的 `总结轮`、`writeRecord` 里的 `"总的来看（"`、`" 做总结）："`、`"总的来看：没有总结（"`、`"）\n"`，以及 `"--no-summary"`。
- 生产代码新标识符不许以 `smx` 开头（那是测试腿的前缀）。
- §1.1 里给出的签名、判据、代码块照抄；「名字实现方定」处自取。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：9 个文件是否都给了；§1.1 A-I 各落在哪个补丁块。
  位置：runSummary 的调用在哪两行之间（贴出锚点第一行）；runner.go 的三行插在哪两行之间；load.go 与 validate.go 的新块各在哪两行之间。
  注释：cmdRound 头注释「本片不引入任何配置键，也不引入自动链」是否已改写成「第六次」段落、「三个 flag」是否改成「四个」；CO1 写的「SM 的位置」是否改成现在时；runOneRound 两个新参数、RoundStartEvent 三行、writeRecord、runner.go 三行、summary.go、schema.go、load.go、validate.go 的注释是否按注释要点写了。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  ```

---

## 6. 落地后主代理要做的

1. **派工顺序**：CO1 全部落地（含它的测试腿、变异、审查）之后，再派本刀的实现腿与测试腿（背对背）。把 CO1 落地后的 `round.go`、`ask.go`、`observer.go`、`commands.go` 全文随任务书交给实现腿；测试腿看仓库（CO1 已落地）与本契约。
2. **收割实现腿**：越界扫描（主仓库、`internal/*`、`cmd/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；按锚点落盘；`gofmt -l`；`go vet ./internal/...`；`go build ./...`。逐字核对：§3 字面量逐字节；`summary.go` 与 §1.1 E 逐字相同（注释除外）；`round.go` 的 F1-F5（flag 分支、F2 整段、`asked` 声明与两处 `runOneRound` 调用、F4 整段、`runOneRound` 的签名与 `render` 回调、`asked` 追加）与 §1.1 F 逐字相同（注释除外；W1b 修订补上，原清单不含 `round.go`，而 #86、#89、#91、#96 这类判据改动在 `round.go`）；`load.go`、`validate.go` 的新块与 §1.1 B、C 逐字相同；`runner.go` 只多三行代码；`ask.go` 的差别只在 §1.1 G 的四处；`internal/turn` 其余零改动；`summary.go` 里、`round.go` 的 F1-F4 新增代码与 F5 的三处改动里都没有 `roundObs`（CO1 defer 里既有的 `r.roundObs != nil` 与 `runOneRound` 既有的 `obs := r.roundObs` 不算）。
3. **合并测试腿**：六个新文件 + CO1 五个用例的期望更新（§1.2；若主代理已采纳 CO1 §7.4 第 1 条，再加有条件的第 6 条；核对 diff 只动期望串 / 行数 / 下标，没动装置与断言结构）。v1.1 的时序相关用例（S7 `note_refused`、`rounds2_last_record`、`note_order`，S13 全部，T1 全部）与 W1b 修订的新夹具（S3 `single_verbose`、`no_summary_verbose`，S8 `mention_verbose`，S10 `canceled_verbose`，S14 全部）合并后各跑一次 `-count=10`（原型零偶发）。先跑 `./internal/config ./internal/turn ./internal/repl ./internal/tui ./internal/app`（另一条流水线也可能在跑 go test；只跑自己的包，3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制），再跑一次全量 `./...`。HEAD 既有用例必须原样全绿（原型实测全绿）。
4. **§4 的变异逐条跑**（#1-#41、#43-#46、#48、#49、#51、#52、#55、#73-#76、#78-#97；#42 与等价清单不跑；从字节副本还原）；任何一条没被点名杀手杀掉，先判是用例空、还是契约写错；契约错就改契约再改任务书。点名「实测保持绿」的对照也要看一眼确实是绿的。
5. **文档**：
   - `docs/00_DECISIONS.md` #45：第二节标「自动总结已落地（commit）」，记下本刀的取舍：人数按「真正被问到的成员（含裁判）去重 ≥ 2」；总结成员不许在 `/round` 名单里；`/ask` 它可以、只给用户看；`/leave` 它 = 这个会话不再总结；总结头全角冒号、不带用时；没配时只在简洁档提示；取消后不总结也不提示；默认 role 由 `Load` 补、代价四条（§1.0，含 v1.1 改写的「进程内改了总结成员的 role 要先 `/session rebuild`」）；总结成员必须 `read`；§7 的「待 Griffin」三条原样搬过去。
   - `docs/12_STEP9_CONTRACT.md` §4.7（`:1148`、`:1289`）后加订正：「不自动触发」被 #45 推翻，自动总结是有界的（每条 /round 最多多一次调用）；不解析 verdict、不循环两条仍成立。
   - `docs/04_CONTEXT_ROUTER.md`：补一段 —— agent 消息今天也可能带 `Recipients`（总结成员的消息只给自己，即对其余成员不可见），可见性仍只走 `recipientOK`。
   - `docs/06_SECURITY_MODEL.md`：总结成员必须 `permissions = "read"`（自动起的进程不带写盘能力）。
   - `docs/09a_STEP6_API_CONTRACT.md` 或 REPL / config 的 API 登记处：`Config.Summary`、`SummaryConfig`、`DefaultSummaryRole`、`/round --no-summary`、`runOneRound` 的新签名（包内）。
   - `docs/README.md`：`[summary]` 的配置片段（追加在文件末尾）与 `--no-summary`；代价一句（每条多成员 `/round` 多一次串行调用，首次与间隔超过 1 小时是冷启动，claude 约 1 美元量级）。
   - 本契约归档到 `docs/`（照 `13a` / `15b` 的体例加归档说明）；`.handoff/HANDOFF.md` 收工时记一条。
6. **给 Griffin 的真机复验步骤**（Windows Terminal + PowerShell 5.1；用一次性的 `data_dir` 与一份只含两个便宜成员加一个总结成员的配置，不碰日常库；**会真的调用模型、花钱**：每条多成员 `/round` 多一次调用，总结成员首次开口是冷启动，claude 约 1 美元量级；每步开始前由 Griffin 决定跑不跑）。council 内的命令**一行一块，逐块粘贴、逐块回车**（#44：整块粘贴多行会被当成一条提交）。
   1. 在配置文件**末尾**追加一个总结成员与一张表（`<总结成员 id>` 换成新起的 id，不要用已经在讨论里用的成员；`permissions` 必须是 `read`）：

      ```
      [agents.<总结成员 id>]
      display_name = "<显示名>"
      provider = "claude"
      model = "<模型>"
      effort = "high"
      permissions = "read"
      mode = "discussion"

      [summary]
      agent = "<总结成员 id>"
      ```
      不写 `role`（Council 会补默认的那句）。若故意把 `permissions` 写成 `workspace`，启动时应当报 `summary.agent: agent "<id>" must have permissions = "read": …` 并拒绝进入。
      （v1.1，F5；W1b 修订拆成两种）改完配置**重启** council 再往下做。若改的时候 council 开着、用的是 `/config reload --apply`：情形一：总结成员在这个进程里已经开过会话、它的 role 又变了（改了它的 role；或者把 `[summary]` 指向了一个开过会话、配置里没写 role 的成员），此后每条 `/round` 的总结都会显示「总的来看：没有总结（拒绝）」加一行 `/round <id>: 身份或脑子在这条 provider 会话开着的时候被改了；Step 6 不静默重建`（不花钱），不会自己好：要先敲 `/session rebuild <总结成员 id>`（下一次总结按首轮计价，冷启动一次）；情形二：把总结成员从 X 换成别人，而 X 在这个进程里开过会话、配置里没写 role：新总结成员照常总结，但 X 以后作为发言者或被 `/ask` 时被拒（同一句原因），要 `/session rebuild X`。配置里自己写了 role 的成员不受影响。重启之后都会自动作废重建一次、冷启动一次。
   2. 启动 `council`（TUI、默认简洁），依次输入每一块：

      ```
      /new 总结复验
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
      看：两个成员头之后是「本轮问 1 个: <总结成员 id>」，再是一行「总的来看（<显示名> 做总结）：…」；看板上多一段 `总结轮`；收尾行 `/round 结束  Ns  回来 3/3`。

      ```
      /who
      ```
      看：总结成员已经在成员表里（没有手动 /add 过它）。

      ```
      /log 30
      ```
      看：总结那一行的 `TO` 列是总结成员自己的 id；其余成员的行 `TO` 是 `-`。
   3. 同一个 TUI 里：

      ```
      /round <成员一> <成员二> --go --no-summary
      ```
      看：没有总结，收尾行 `回来 2/2`。

      ```
      /round <成员一> --go
      ```
      看：单个成员，没有总结。

      ```
      /round <成员一> <总结成员 id> --go
      ```
      看：一行「拒绝: <总结成员 id> 是总结成员…」，不起进程。

      ```
      /verbose
      ```

      ```
      /round <成员一> <成员二> --go
      ```
      看：总结段照裁判轮的详细格式：预算表、「本轮问 1 个」、`outcome spoke  agent <总结成员 id>` 整块、`总结轮结算`、段终态块 `总结轮 (xxxxxx)  回来 1/1`。

      ```
      /cost
      ```
      看：总结成员单独一行，金额计入。

      ```
      /quit
      ```
   4. （可选，多花一次钱）检验「只给你看」：重新进 `council`，`/use` 回到那个会话，然后

      ```
      /ask <成员一> 刚才有没有人对这一轮做过总结？如果有，原话是什么？
      ```
      看：成员一不应当能复述总结的内容（它的 prompt 里没有那条消息）。
   5. `council -no-tui`（行式，默认简洁）里 `/use` 回那个会话、`/round <成员一> <成员二> --go`：总结在「总用时 N.Ns」那一行之前。
7. **清理**：本刀在中转根（`E:\multi_model_cowork\`）与 scratchpad 里的全部中间物逐个删除（本契约归档后删掉中转根里这一份）。

---

## 7. Open items（v1.1 新增，F8：从 v1 起草人报告的 open_items 抄进来，按 v1.1 与主代理裁定更新）

### 7.1 主代理已定（不再等人裁）

- **改了 CO1 §1.1 D1 逐字冻结的 `writeAskRecord` 函数体**（拆成 `writeRecord(r, rec, summary)` 加 `writeAskRecord` / `writeSummaryRecord` 两个薄包装，`summary` 只改简洁档的两个头）：**主代理接受**。担保：CO1 的 K1-K5（`summary == false` 的简洁档）与 K2（详细档八个 Record）、本刀 §2.3 K2 的两档对照。锚点按 CO1 v1.1 的冻结代码取（CO1 v1.1 的 §1.1 与 v1 一字不差）。替代做法（在 `summary.go` 另写一份照打行）违反「格式只有一份」，不采用。
- **总结成员出现在 `/round` 名单里时整条命令拒绝**，包括 mention 推出来的名单与不带 `--go` 的预览；`/ask` 总结成员允许，回复只给用户看（主代理定）。
- **「多个成员」按本条命令里真正被问到的成员（含裁判）去重计数，至少 2 个才总结**：`/round a --go --with-judge` 会总结，`/round a b --go` 而 b 无新内容不总结（主代理定）。
- **`/leave` 总结成员 = 这个会话不再总结**：已退出不拉回，打一行说明怎么恢复（主代理定；与 `/team use` 不拉回已退出成员同一裁决）。
- **硬依赖**：落地顺序必须是 CO1 → SM1。SM1 的 `round.go`、`ask.go`、`observer.go`、`commands.go` 锚点取 CO1 落地后的真文件，派工时把这四个文件全文交给实现腿；CO1 测试腿的五处期望（§1.2）在 SM1 落地时一起改。若 CO1 测试腿的写法与 CO1 契约的描述不同（例如 R3 不是从 R1 的常量派生），SM1 测试腿按语义改：只插提示行、改行数和下标。

### 7.2 待 Griffin（只登记，本刀照 v1 做）

- **总结头不带用时**：照 #45 原话「总的来看（Fable 5.1 做总结）：…」；但 Griffin 另裁过「显示每个模型的用时」。总结的用时今天只进 CO1 的总用时与 TUI 看板。要带就改 §3.2 第 1 行与 `writeRecord` 的一处（`head = "总的来看（" + name + " 做总结）："`）。
- **简洁档在总结之前照打「本轮问 1 个: <总结成员 id>」**：作进度信号（CO1 保留同类行的同一个理由；一次总结可能要几十秒）。嫌多就在 `runOneRound` 加一个门。
- **`DefaultSummaryRole` 的中文措辞**是起草人写的：改它会让每个总结成员各重建一次会话（冷启动一次），其余成员不受影响。

### 7.3 已知现状（不改，照写）

- **总结成员看到的是它自上次开口以来的全部新消息**，不只是这一条 `/round`（包括 `/ask`、单成员 `/round`、带 `--no-summary` 的那些轮）；首次开口是 bootstrap，每个其他成员只取窗口内最后一条，所以 `--rounds N` 的首次总结看不到前几轮。要精确到本条命令，得给 turn 加一个下界，本刀不做。
- **代价**（照 #45 写进 §0 与 README）：每条多成员 `/round` 多一次串行调用。总结成员首次开口，或距上次开口超过 1 小时（`CacheTTL`），按冷启动计价：claude 约 47749 token、0.955 美元。
- **进程内 `/config reload --apply` 让一个开过会话的 agent 的 role 变了，它以后每次被问都被拒**，要先 `/session rebuild <它的 id>`：总结成员自己的 role 变了是每轮的总结被拒；换掉的旧总结成员（配置里没写 role、失去了默认 role）是它作为发言者或被 `/ask` 时被拒，新总结成员若没开过会话则照常（§1.0 代价 (b)，v1.1 F5，W1b 修订拆成两种，修订原型探针核过）。
- **MD1 交叉**：MD1 把新 agent 表整段追加到文件末尾；`[summary]` 已在末尾时再追加一张 agents 表仍是合法 TOML，MD1 写后的 Load + Validate 会经过本刀的 `summary.agent` 校验。v1.1 由 §2.1 C5 钉住，W1b 修订把追加段换成 MD1 契约 §2.3 C1(a) 期望串的原样（注释行加带引号的 `[agents."opus-5.5-1m-high"]`）；集成时仍建议跑一次「末尾已有 `[summary]` 再 `/add` 识别出的模型」的真机或用例（C5 不调 MD1 的代码）。
- **文档**：要订正 `12_STEP9_CONTRACT` §4.7（`:1148`、`:1289`）里的「不自动触发」；`04_CONTEXT_ROUTER` 要写明 agent 消息今天也可能带 `Recipients`；`06_SECURITY_MODEL` 要写明总结成员必须 `read`。都列在 §6 第 5 步。

### 7.4 待主代理（W1b 修订提出；本修订取保守读法）

- **CO1 §7.4 第 1 条（R5 `judge_spawn_failed`）采不采纳，连带本刀 §1.2 与 §8 的第 6 条授权**：采纳在 CO1，本刀不定。采纳，则本刀测试腿按 §1.2 第 6 条改那个子测试的期望（整串已由 W1b 修订原型实跑冻结，8 行）；不采纳，则第 6 条作废，本刀不写那个子测试。派本刀测试腿时，主代理按 CO1 落地后的真文件告诉它是哪一种。另：CO1 §7.4 第 2 条若把 `judgeRan` 等局部变量名改成冻结，本刀 §1.1 F4 里的 `judgeRan` 与之同名，无冲突。

## 8. existing_tests_changed（v1.1 新增，F8）

- **HEAD 既有 `_test.go`：零改动。** 原型上 CO1 v1.1 与本刀都落地后，全量 `go test -count=1 ./...` 九包全绿。`TestWithJudgeRunsOneExtraRoundWithOwnID` 与 `TestWithJudgeRunsOnlyOnceAcrossMultipleRounds`（`internal/repl/round_test.go:361`、`:398`）是「没配时不拍快照」的守卫，§1.0 的判据顺序避开了它们变红（§4 #34 是它们的变异）。
- **CO1 测试腿的文件（本刀落地时已是既有用例）改五处（W1b 修订：另有一处有条件的第 6 处），只改期望，由本刀测试腿交付**（§1.2 逐条写了改前改后）：
  1. `internal/repl/cox_round_test.go`：`TestCoxRoundConciseGolden`（CO1 R1）。为什么红：简洁档、没配 `[summary]`、问到 3 个成员，本刀照 #45 打提示行。改法：期望串在 `总用时 7.2s` 之前插入提示行；时钟次数仍是 2。
  2. 同文件 `TestCoxRoundObserverSuppressesTotal/concise`（CO1 R3）。为什么红：同上，装了观察者时提示行照打。改法：期望等于新 R1 去掉最后一行，即以提示行结尾（CO1 v1.1 仍从 R1 的常量派生，改 R1 一处即可）。
  3. 同文件 `TestCoxRoundSpawnFailedConcise`（CO1 R6，成员 `cox_a`、`cox_x`）。为什么红：提示行插在「分不清进程是否起来 1（…）」与「总用时」之间。改法：6 行改成 7 行，第 5 行是提示行，第 6 行是 `总用时 7.2s`。
  4. `internal/tui/cox_concise_test.go`：`TestCoxProgramConciseNoBlockOneTotal/concise`（CO1 T2）。为什么红：P 多一条提示行。改法：P 由恰好 4 条改为恰好 5 条，下标 3 是提示行，最后一条仍匹配 `^/round 结束  \d+s  回来 2/2$`。
  5. （v1.1）`internal/repl/cox_round_test.go`：`TestCoxRoundNoDeltaConcise/round2`（CO1 v1.1 R5）。为什么红：第 1 轮问到 `cox_a`、`cox_x` 两个、`cox_a` 回复了、没取消，本刀打提示行。改法：期望整串在最后一行 `总用时 7.2s` 之前插入提示行（11 行变 12 行）；规范化、守卫与时钟次数（2）不变。
  6. （W1b 修订，**有条件**，只在主代理采纳 CO1 §7.4 第 1 条时生效，见 §7.4）同文件 `TestCoxRoundNoDeltaConcise/judge_spawn_failed`。为什么红：送进 `cox_a`（发言轮）与起不来的裁判 `cox_y`（裁判轮）两个、`cox_a` 回复了、没取消，本刀打提示行。改法：期望整串在 `总用时 7.2s` 之前插入提示行（7 行变 8 行，整串见 §1.2 第 6 条）；原因行守卫、规范化与时钟次数（2）不变。
- CO1 其余用例原型实跑不受影响，逐条列在 §1.2 末尾（W1b 修订补上 CO1 W1b 修订加的 R11 三条与 L1 第三条）。
- 本刀新增的六个测试文件（§2.1-§2.6）不是既有用例，不在本节。
