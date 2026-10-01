# AT1 冻结契约：输入框 @ 成员补全（v1.1 + W1b 修订，2026-09-28）

> **归档说明（2026-09-28，主代理）**：本文是 AT1 的冻结契约，派工时的原件是 `E:\multi_model_cowork\at1_contract.md`（v1.1 + W1b 修订），原样归档；文中行号指 HEAD `3d134ee`。AT1 已于 `4202b6c` 落地：生产代码三个文件由 grok-4.7 实现，与 §1.1 逐字节相同；五个测试文件由 Opus 测试腿写成；§4 的 57 条变异全被点名杀手杀掉，4 条等价的核过理由。真机未复验。
>
> **落地后订正（2026-09-28，主代理；读本文之前先读这里，覆盖正文里与之冲突的说法）**：
> - **状态（2026-09-29）**：下面前五条（Ctrl-V、带修饰键的回车、A41 的理由、四处注释、等价清单）已由 AT1-FU 修，`ee247c6`，契约归档 `19c_AT1FU_FOLLOWUP_CONTRACT.md`。第六条（§2.0 的两处说法）已直接改在正文 §2.0。
> - **§1.1 D7「焦点、光标闪烁等交给 textarea（不重判：文字与光标不变）」这句不对。** textarea 自己的 Ctrl-V（`KeyMap.Paste`）读剪贴板之后交回一条未导出的 `pasteMsg`，它不是 `tea.PasteMsg`，走 `updateInput` 最后的 default 分支，会插入文字，但既不重判也不收起列表。于是列表带着旧前缀开着，补全时按旧前缀的长度退格，删掉的是刚粘进来的字。不走花钱路（Submit 仍是 0 次）。这条路只在终端把 Ctrl-V 原样交给程序时才走得到；Windows Terminal 默认自己接管 Ctrl-V，发的是括号粘贴（`tea.PasteMsg`），会正常收起。主代理裁定：后续一刀 AT1-FU 在 `New` 里解绑 `ta.KeyMap.Paste`。这样粘贴只剩终端括号粘贴一条路，用一条确定性断言担保。代价：终端把 Ctrl-V 交给程序时，Ctrl-V 什么都不做。
> - **列表开着时带修饰键的回车（Shift+Enter、Alt+Enter）只补全、不提交**：今天的代码是对的，B4 的截获不看修饰键。但没有用例担保：把截获收窄成只认无修饰键，全部 TestAtx 照绿，而那时带修饰键的回车会掉进既有分支、真的提交。AT1-FU 给 M5 加这一步，§4 加 A52。
> - **§4 等价变异 A41 的理由不成立**：Ctrl-T（textarea 的 transposeLeft）在行尾会对调最后两个字，光标不动，于是 @ 左边那个字不用挪光标就能改。A41 仍然等价，但理由是「@ 那一处判据兜着」；两处判据一起去掉时，全部 TestAtx 照绿。AT1-FU 给 M7 加 Ctrl-T 一步，登记双变异。
> - **四处既有注释与新行为不符**（补丁只换函数体，没带注释）：`termHeight`（「不参与 View」）、`chrome`（「往活区里加一行只改这里」）、`frameHeight`（「固定行数加 textarea 的高度」）、`beginSubmit`（「空闲时 Ctrl-C」会改提示）。AT1-FU 补一个只改注释的 B12。
> - **§4 的等价清单不全**。下面这些在全部 atx 用例下存活，按可达性都等价，AT1-FU 登记理由，不加用例：
>   - atArm 的 `HasSelection()`；
>   - atLines 与 atAccept 对 sel 的夹取；
>   - atArm 的 `col < 1 || col > len(line)`；
>   - `c.Blink = true`：依赖 bubbles 默认的 `Blink: true`，升级 bubbles 时与 V1 一起复核；
>   - atRecheck 的 `m.at.col >= len(line)`；
>   - mentioncand 的 `sumID != ""`。
> - **§2.0 的两处说法**：
>   - 「五个文件都引用本刀新增的标识符」对 `atx_run_test.go` 不成立。R1 只用既有名字，它在 HEAD 上编不过只是因为同包。
>   - HEAD 核对只跑 `go vet` 不够：vet 每个包只报第一个错。要用 `go test -run '^$' -gcflags=-e` 列全部错误。这次集成员就是这样核的：46 条错误全是新名字。
> - **§7 待主代理 (l)、(m) 已定**：(m) 保留 R1 的等待步；落地顺序定为 CH1 §6 第 0 步的 (a)，AT1 先落、CH1 同一批紧跟。(l) 在 CH1 定稿后再核一次，结果记进 CH1 的归档（`19a`）。


- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `3d134ee`（= origin/main；最后一次生产改动在 `c0e6a27` CX1，之后只有文档）。本文行号一律以 HEAD `3d134ee` 为准。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`（不设 `GOFLAGS`）。本机没有 gcc，不能 `-race`。依赖版本：`charm.land/bubbletea/v2 v2.0.9`、`charm.land/bubbles/v2 v2.2.1`、ultraviolet 伪版本 `v0.0.0-20260811164956-006e29f97886`、`github.com/charmbracelet/x/ansi v0.11.8`。
- 本刀 KEY 是 `AT1`；**测试腿的包级标识符前缀是 `atx`**（类型、函数、变量、常量一律 `atx` 开头，测试函数 `TestAtx...`）。生产代码的新标识符不许以 `atx` 开头（本刀生产代码的新名字以 `at`、`mention`、`Mention` 开头，见 §5）。
- 来源：`docs/00_DECISIONS.md` **#47 的「补充」**（`:2202`）。**方向（Griffin 2026-09-28 定，不许改）**：TUI 输入框里敲 `@` 弹出可选成员的补全列表。主代理定、本契约冻结的细节：候选 = 当前会话活跃成员（不含 `[summary]` 总结成员），每行 id 与显示名；`@` 之后按已输入前缀筛选；上下键选、Tab 或回车补全（补上完整 id 加一个空格）、Esc 关；列表开着时回车只补全、不提交；列表放在输入框**下方**。放上方还是下方由 VT 回放实测定（§0 第 3 条：下方，而且必须同时钉住真光标，否则两种放法都留残影）。
- **原型**：本文全部期望值都是隔离拷贝 `E:\Isolating_test\council\at1_draft\`（`git archive 3d134ee` 解出）里一份原型实跑出来的：§1.1 的三处生产改动逐字落盘；§2 的五个测试文件按本文写成。原型下 `gofmt -l internal` 为空、`go vet ./internal/tui ./internal/repl` 无报告、`go build ./...` 通过；**全量 `go test -count=1 ./...` 九个有测试的包全绿，HEAD 既有用例零改动、零变红**。§0 的残影数字出自同一拷贝里的一份带开关的探针（放法上 / 下 × 钉光标 / 不钉，开关只在探针里，契约代码不带）。§4 的变异逐条在原型上跑过（锚点恰好一次、从字节副本还原并比对字节相等），点名杀手实测变红。原型、探针与变异装置已用 `fs.rmSync` 删除。
- **v1.1 的原型**：`E:\Isolating_test\council\at1_v11\`（同样 `git archive 3d134ee` 解出）。先把 v1 原样重建：§1.1 由脚本从 v1 文本逐字取出打上（十二个锚点各恰好一次），§2 的五个文件照 v1 文本重写，第一次就全绿，全量 `go test -count=1 ./...` 九个包全绿。再逐条落实 v1.1 的改动（见下面的变更记录）；新增与改动的期望值都是这份原型实跑出来的。v1.1 的终态：`gofmt -l internal` 为空、`go vet ./internal/tui ./internal/repl` 无报告、`go build ./...` 通过，全量 `go test -count=1 ./...` 九个包全绿，P1、R1、V1 `-count=3` 全绿；从本文 v1.1 的 §1.1 用脚本重新取出打在另一份 `git archive 3d134ee` 上，三个生产文件与原型逐字节相同（§1.1 A、B、C 与 v1 逐字节相同）。§4 的全部变异在 v1.1 的测试上整表重跑。§0 表里新填的格子与表后另测的场景出自同一拷贝里一份带放法切换的探针（切换写在探针的驱动脚本里，改的是原型 `model.go` 的副本，契约代码不带）。原型、探针与变异装置已用 `fs.rmSync` 删除。
- **W1b 修订的原型**：`E:\Isolating_test\council\at1_fix\`（同样 `git archive 3d134ee` 解出）。§1.1 由脚本从本文逐字取出打上（十二个锚点各恰好一次），`gofmt -l internal` 为空、`go vet ./internal/tui ./internal/repl` 无报告、`go build ./...` 通过。只写了本轮新增或改写说明要用到的用例：M13 第 4 步与它的对照、M3 第一步与 M9「只喂粘贴」那段、M7 的「@b ← ← → → →」与「退格一次 / 两次」两段、M2 `space`、两个探针（前缀对 `grok-4.7` / `grok-4.7-high`；按 Griffin 配置里的 id 造的候选）；忠实实现上每条第一次跑就绿。A49 下 M13 第 4 步红、对照绿；A51 下 M7 那一步红；一条不登记的演示变异见 M13【判别力】（每条都是锚点恰好一次、从字节副本还原、还原后 SHA256 相等）。本轮**没有**跑全量 `./...`、没有重跑 §4 整表、没有重跑 R1（§2.4 里 W1b 的 R1 读数标明出处，照引）。原型与变异装置已用 `fs.rmSync` 删除。

### v1.1 变更记录（2026-09-28，照主代理裁决书 `at1_rulings.md`；W1 原文 `at1_w1_findings.md` F0-F6）

§1.1 冻结的生产代码一个字节都没改；下面全是测试侧与说明侧的改动。

- **F0**：§4 等价变异 C3 的理由改正：store 的 `Participants` 带 `ORDER BY agent_id ASC`（`store/read.go:63`；函数在 `:57`，`:56` 的注释写着「结果按 agent_id 升序」）。删掉「没有 ORDER BY」「不依赖扫描顺序」两句。W1 与裁决书写的 `:62` 按真文件改成 `:63`。
- **F1**：§0 表「退出」一行上方两格按 v1.1 原型复测填上；表后补一段：列表开着时窗口变尺寸、空闲时插一行、粘贴收起、Ctrl-C 收起、8 行输入、长行折行、中文前缀，在「下方 + 钉光标」下的实测读数。
- **F2**：三处夹具。C1 的 `alpha` 显示名加 `0x7f`、`0x9b`（期望的 `Name` 随之变成 `"A B C D E"`）；M4 末尾加「前缀没变、选中项保留」一段（Delete）；M13 加第 4 步「候选先非 nil、再被 nil 整份换掉」。§4 新登记三条变异：A49（W1 的 X22）、A50（X31）、C10（`unicode.IsControl` 改成只认 `r < 0x20`），各自的点名杀手实测变红，在 v1 的测试上实测存活。
- **F3**：不改代码。§6 真机步骤里光标形状那一步加一句（列表开着时 `@` 后面那一格可能同时有反白方块与闪烁竖线，请说观感）；§7 (a) 并进这一条。原型实测：列表开着时 `View().Content` 第 3 行 `@` 后面那一格是 `ESC[7m` 反白，`View().Cursor` 在同一格 (7, 2)。
- **F4**：不改代码。§6 已知代价加一句（句末是点名、已经敲完整个 id 时，第一次回车只补空格，第二次才提交）；§6 真机步骤 3 后面加一步验它（原来的 4-9 顺延成 5-10）；§7 待 Griffin 加 (g)。原型实测：`你怎么看 @rba_b` 回车 → 值 `"你怎么看 @rba_b "`、`Submit` 0 次；再回车 → `Submit` 1 次，收到 `"你怎么看 @rba_b "`。
- **F5**：相交表的 CH1 一列按 CH1 契约的真文件改写：v1（20:05，与它的 v1 快照同字节）与修订期间写出来的 v1.1（21:18，21:32 又核一次，下面用到的事实没变）各核一次，生产文件清单两版相同。删掉猜的 `addmodel.go`，写明名字零碰撞与 REPL 零值「只记下」。裁决书列的 CH1 文件清单少了 `seam.go`、`paste.go`，按真文件补上。落地说明加「候选规则两份」「语义方向一致」两段；§7 (c) 补「CH1 下 @ 裁判不会让它回复」。按「同类的还有没有别处」多查到一处，写进落地说明、请主代理定：CH1 v1.1 的新字面量引用了本刀的功能（「TUI 里敲 @ 会弹出可点的成员」），v1 的「建议 CH1 先落」因此撤回，落地顺序按 CH1 §6 第 0 步定；「可点的」与本刀（不做鼠标点选）不符。原 §7 待主代理 (f)（W1）与 (g)（按 CH1 真文件核相交表）已在本版完成，删去；新加待主代理 (l)：CH1 定稿或落地后再核一次。
- **F6**：§7 待 Griffin 加 (f)：`@` 前面是括号、中文或全角标点时也弹列表。
- **裁决书之外（修订中实测发现，请主代理确认或退回）**：§2.4 R1 在「看到 `当前会话:`」之后、敲 `@` 之前加一步「等 `当前会话:` 后面出现空闲表头」。原因：`当前会话:` 是 `Submit` 里打的，那一刻还在忙，候选随之后的 `submitDoneMsg` 才到；`@` 抢在它前面进事件循环时忙时不开，主断言 1 假红。原型上 v1 的 R1 在一次变异跑里就这样红过一次（与变异无关）；把 `replAdapter.MentionCandidates` 临时拖慢 200ms（只在原型里）后 v1 的 R1 必红、加了这一步的 R1 绿，P1、V1 按 `submitDoneMsg` 同步、不受影响。未改动时 R1 连跑 80 次零红，所以这是低概率的时序竞态，不是必现。

### W1b 修订（2026-09-28，W1b 返回 5 条 minor，没有 blocker、没有 major）

§1.1 冻结的生产代码一个字节都没改；§4 加一条 A51（给 M7 一处既有断言登记杀手，见 W1b-3）。新期望与 A51 都在 `at1_fix` 原型上实跑（见文件头）。照「同类的还有没有别处」多改的几处标在各条的「同类」后面。

- **W1b-1 (m) 与 R1 等待步**：成立一半。§2.4【v1.1 那一步的来历】补上 W1b 的独立复测读数，标明出处、没有重跑；「80 次零红」后面补一句（它不说明撞不上，有并发负载时撞得上）。§7 (m) 加 W1b 的建议（保留并关掉），**没有**照 W1b 改成「已定：保留」、没有删「或退回 v1 的写法」：(m) 是问主代理的，修订人不替主代理定，仍标待主代理。
- **W1b-2「1 个匹配」「唯一匹配」**：成立。按 `docs/00_DECISIONS.md:1740`、`:2152` 核过：Griffin 的配置里有 `opus-5.0` / `opus-5.0-max`、`fable-5.1` / `fable-5.1-max` 两对。§6 第 4 步落地记录的「（1 个匹配）」改成「至少 1 个匹配……补出来的就是它」；§6 真机步骤 4 的括号换成具体 id，并说明敲完 `opus-5.0`、`fable-5.1` 会多一行；§7 (g) 给 Griffin 并列 (i)「唯一匹配」与 (ii)「等于选中项」两个改法，由他定，本刀不改。原型探针：候选 `atxMembers` 加 `{grok-4.7-high, G}`，敲 `x @grok-4.7` → 行 == `["> @grok-4.7       Grok 4.7", "  @grok-4.7-high  G"]`；回车 → 值 `"x @grok-4.7 "`、没有 `submitDone`、`Submit` 0 次；再回车 → `Submit` 1 次，收到 `"x @grok-4.7 "`。按 Griffin 的七个非总结成员 id 造候选（显示名留空）：敲完 `opus-5.0` → `["> @opus-5.0", "  @opus-5.0-max"]`，`fable-5.1` 同形，`grok-4.7`、`opus-5.5` 各一行；回车都补成敲完的那个加空格。**同类**：§0 第 1 条举的 `grok-4.7-high` 不是任何真 id（`docs/` 里查不到），换成真配置里的例子并按行号引。
- **W1b-3 M13 第 4 步的对照**：成立。第 4 步加「只把 nil 换成 atxMembers → 行 == `ALL`、帧 14」这条对照，两条都加值 `"@ @"`；「敲 `@` → `ALL`」降为前提；【判别力】写明为什么前提不算对照（`@` 在行首与在空格之后走的判据不同）。原型：忠实实现两条都绿；A49 下第 4 步红、对照绿；演示变异「左边界改成不在行首一律不开」下第 4 步照绿、对照红、M2 `space` 红（不登记进 §4）。**同类**：M9「只喂粘贴 `@b` → 行 nil」同样是没有杀手的缺席断言，【判别力】写明同装置的正对照是 M3 第一步（只差敲与粘贴），并加值 `"@b"`、帧 8，防止粘贴没进去时空绿；原型两条都绿。M7 里又找到两条同样的：「→ → → 行 nil」加值 `"@b"` 与光标 (行 0, 列 2)，【判别力】写明对照是 M3 第一步（值与光标都相同，只差来路），并登记杀手 A51（光标一落到 `@` 后面就开；原型实测它让这一步变成 `B3`，锚点恰好一次、从字节副本还原、SHA256 相等；这一步 v1.1 就有，A51 只是补登记）；「退格两次 → 行 nil」前面加只退格一次的对照（行 == `ALL`、值 `"@"`）。原型这几条忠实实现上全绿。M 系列别的缺席断言没有按「只差一处」的标准逐条重审。
- **W1b-4「窗口变尺寸」结论句**：成立。§0「另测」表下的结论句拆开：其余六类照旧；「窗口变尺寸」两行只说明 `WindowSizeMsg` 引起的帧高变化（模拟器不缩），真终端缩放没测。§6 第 4 步落地记录要求照抄这条限定。**同类**：结论句说「由真机步骤 8 看」，但步骤 8 在残影检查（步骤 7）之后、原文只看列表行数、不看残影；步骤 8 补上「Esc 后往上滚动看回滚区」与「不开列表同样拉一次」的对照。
- **W1b-5 CH1 的遗留**：成立。本刀不改 CH1。§7 (l) 加两件转交 CH1 修订人的事：删 CH1 §6 第 0 步那句「AT1 契约自己建议 CH1 先落」；「可点的成员」换措辞（CH1 22:10 那一份已改了字面量与 §3.2，只剩两处说明文字，见下）。**同类**：W1b 列了 `:480`、`:483`、`:1059`、`:1060`、`:1218`；grep 全文另有 `:17`、`:1063`、`:1361` 三处说明文字也写着「可点的」，一并列进 (l)。落地说明里「那句措辞归 CH1 改」后面补一句指向 (l)。`ch1_contract.md` 本轮只读；收尾时它的 mtime 从 21:32:47 变成 22:10:00（CH1 那条流水线的 W1b 修订，v1.2），又只读核了一次：两句字面量与 §3.2 已改成「会列出成员的 id 与显示名」，剩 `:1247`、`:1390` 两处旧说法与「建议 CH1 先落」那句，(l) 按它写了；文件清单仍是 12 个、没有 `internal/tui` 的生产文件与 `mentioncand.go`；grep `at[A-Z]*`、`mention[A-Z]*`、`Mention[A-Z]*` 仍只命中 HEAD 既有的三个名字与引用本刀名字的那一句（`:70`），零碰撞。CH1 之后若再变，以 (l) 的「定稿后再核」为准。

## 与 CH1 及已落地各刀的相交表

CH1（对话模式，另一条流水线起草，契约 `E:\multi_model_cowork\ch1_contract.md`）：v1.1 按它的**真文件**核过，两个版本各核一次：CH1 v1（2026-09-28 20:05，与 `ch1_contract_v1.md` 逐字节相同），与本文修订期间写出来的 CH1 v1.1（同一路径，21:18 那一份，21:32 又核一次，下面用到的事实没变；基线 `3d134ee`；它还在收尾，行号以它定稿为准，下面尽量按节号引）。两版的生产文件清单相同（CH1 v1 §5「只许改这 11 个文件」、v1.1 §5「只许改这 12 个文件」，多出来的一个是测试文件）：生产 8 个 —— 新文件 `internal/repl/autoreply.go`；改 `internal/repl/commands.go`、`seam.go`、`paste.go`、`repl.go`、`round.go`（只改 `cmdRound` 头注释）、`internal/app/app.go`、`cmd/council/main.go`；授权改 `internal/app` 的四个既有测试文件共六处（`app_test.go`、`cox_app_test.go`、`fux_review_test.go`、`smx_summary_app_test.go`，最后一个是 v1.1 加的）；新测试文件前缀 `chx`。「`/add` 候选藏总结成员」改的是 `commands.go` 的 `cmdAdd`，**不碰 `addmodel.go`**。**不改 `internal/tui` 的任何生产代码**（CH1 §1.2）。

| 本刀的文件 / 函数 | CH1（对话模式） | X44（`e85d1bc`） | RB-A / RB-B（`69fed6c` / `572467c`） | CO1 / SM1 / MD1 / CX1 / U43A | 结论 |
|---|---|---|---|---|---|
| 新文件 `internal/repl/mentioncand.go`：类型 `MentionCandidate`、方法 `(*REPL).MentionCandidates`、函数 `MentionBoundaryOK`、`MentionTokenRune`、`mentionName` | CH1：新文件 `autoreply.go`；改 `commands.go`（`dispatch` 裸文本分支、`postUserText`、`sayCmd`、`cmdAdd` 候选循环等）、`seam.go`（`Submit` 裸文本分支）、`paste.go`（`finishPaste` 末尾）、`repl.go`（REPL 结构体加 `autoReply`）、`round.go`（只改头注释）；加会话命令 `/auto`。只读 `mention.go` 的 `mentionLeftForbidden`、`mentionTokenChar`、`mentionIDRe`（v1.1 的 `hasStrayAt`），不改 `mention.go`、`addmodel.go` | 不碰（`latercmd.go`） | 不碰 | 不碰 | **按文件不相交**：本刀在 `internal/repl` 只加一个新文件，不改任何既有文件，**不改 `mention.go`**（只读引用它的 `mentionLeftForbidden` `:105-113`、`mentionTokenChar` `:118-126`）、不改 `ParseMentions`（`:62-94`）；CH1 的 8 个生产文件里没有 `mentioncand.go`、`mention.go`。**按名字零碰撞**：CH1 的新名字是 REPL 字段 `autoReply`、`SetAutoReply`、`cmdArgs.bare`、`roundCmd`、`sayText`、`postUser`、`autoReplyRound`、`cmdAuto`、`hasStrayAt`（v1.1）、`msgAuto*` 常量（v1 五个，v1.1 加 `msgAutoOnNoSummary`、`msgAutoStrayAt`、`msgAutoStrayAtPartly`）、`app.Options.NoAuto`（CH1 §1.1「冻结的名字」）；CH1 v1.1 还明令它的生产新名字不许以 `mention` / `Mention` 开头、并点名了本刀的五个名字。对 CH1 v1 与 v1.1（21:18、21:32）全文 grep `at[A-Z]*`、`mention[A-Z]*`、`Mention[A-Z]*` 标识符：只命中 HEAD 既有的 `mentionLeftForbidden`、`mentionTokenChar`、`mentionIDRe` 与 CH1 v1.1 引用本刀名字的那一句，零碰撞。CH1 定稿或落地后再 grep 一次（§7 (l)）。**按语义相交**：CH1 的「裸文本有 @ 就只问被点的」用的是 `ParseMentions`；本刀补全出来的文字 `@<id> ` 就是 ParseMentions 认得的形状（左边界同一条规则，§1.1 D1），不需要 CH1 做任何配合。「候选 = 活跃成员去掉总结成员」这条规则两边各有一份（本刀 `MentionCandidates`，CH1 `autoReplyRound`，CH1 §1.1 E），写法差一个 `sumID != ""` 守卫（只在成员 id 是空串时才有区别），见下面的落地说明 |
| `internal/tui/model.go`：`submitDoneMsg` 加字段、`mentionSource`、`atState`、`atMaxRows`、`Model` 加两个字段、`updateInput` 的 `submitDoneMsg` / `KeyPressMsg` / `PasteMsg` 三支、`View`、`frameHeight`、`beginSubmit` 一行、`submitCmd`，新方法 `atCursor`、`atArm`、`atRecheck`、`atLine`、`atMatches`、`atVisible`、`atLines`、`atAccept` | 不碰：CH1 不改 `internal/tui` 的任何生产代码（CH1 §1.2）。CH1 在 tui 包新加的 `chx_program_test.go` T1 用 `rbbStartProg`、粘贴两行再回车（`PasteMsg` 加 Enter），不逐键敲 `@`，本刀的回车截获碰不到它；本刀落地后它经 `replAdapter` 每次 Submit 之后多读一次成员表，T1 只比日志里 P / RS / MD / RE / DONE / FIN 的序列，不受影响（按 CH1 文本读，未实跑） | `beginSubmit` 的门（`:478-481`）一个字不动；本刀只在它下面的 `m.ta.Reset()`（`:489`）之后加一行 | `chrome()`、`boardLimit()`、`closingLine()`、finalize 路径一个字不动；`View` 里 `pinCursor` 的优先级保留在最前 | CO1 只改过 `roundobs.go` 的委托；MD1、SM1、CX1、U43A 不碰 `internal/tui` | 不相交；本刀的补丁锚点全在 HEAD `model.go` 里恰好一次（§1.1 B） |
| `internal/tui/run.go`：`replAdapter` 加方法 `MentionCandidates` | 不碰 | 不碰 | `Run` 的观察者装配（`:56-58`）一个字不动 | 不碰 | 不相交；锚点是 `IsQuit` 方法（`:182-185`） |
| 测试：`internal/tui`、`internal/repl` 两个包 | CH1 的测试前缀是 `chx`（四个新文件：`internal/repl/chx_auto_test.go`、`internal/tui/chx_program_test.go`、`internal/app/chx_app_test.go`、`cmd/council/chx_flag_test.go`）。**REPL 层零值是「只记下」**：CH1 的 REPL 字段 `autoReply` 零值 false，CH1 §1.1 D 的注释原文「零值 false 是只记下（今天的行为）」；自动回复是 app 层的默认（`app.Options.NoAuto` 零值），由 `app.Run` 在进 TUI 前 `SetAutoReply`。`rbaNewTuiRig` / `repl.New` 直接造的 REPL、直接调 `tui.Run` 的都是零值 | 前缀 `mlx` | 前缀 `rba`、`rbb` | `cox`、`smx`、`mdx`、`cxh`… | 不相交，文件名与前缀零碰撞。本刀的 P1、R1 在真 REPL 上提交两条裸文本（`@rba_b @`、`@rba_a hi`），依赖「`repl.New` 零值只记下」：CH1 的零值正是这样，前提成立；R1 直接调 `tui.Run`，不经 `app.Run`，不吃 app 层的默认（§7 (i)） |

**落地顺序**：代码上与 CH1 互不依赖，先落后落都能编译、都不用改对方的文件。后落的一方按当时的真文件重取锚点：本刀的锚点全在 `internal/tui/model.go`、`internal/tui/run.go`，CH1 不碰这两个文件；`internal/repl` 这边本刀只新增文件。后落的一方落地前在先落一方落地后的 main 上重跑一次自己的全部新用例与全量 `./...`。

- **说明文字上有一处依赖（v1.1 新发现，主代理定）**：CH1 v1.1 的两句新字面量 `msgAutoStrayAt`、`msgAutoStrayAtPartly` 末尾写着「TUI 里敲 @ 会弹出可点的成员」，说的就是本刀的功能。v1 这里写的「建议 CH1 先落」只从代码看；照那个顺序，CH1 落地到本刀落地之间这半句对用户是假话。落地顺序改由主代理按 CH1 §6 第 0 步的三选一定（本刀先落或同一批；CH1 先落并先删那半句；接受这段时间不准确）。**已定（主代理，2026-09-28 22:30）：(a)，本刀先落、CH1 同一批紧跟着落**；两刀的生产文件零重叠（本刀 `mentioncand.go`（新）、`tui/model.go`、`tui/run.go`；CH1 是 repl / app / cmd 下 8 个文件，没有 `internal/tui` 的生产文件），所以 CH1 那半句从它落地的那个提交起就是真话；「候选规则两份一致」由后落地的 CH1 一方核；两刀都落地之后才重装。另外「可点的」与本刀不符：本刀不做鼠标点选（§1.2），是上下键选、Tab 或回车补全。那句措辞归 CH1 改，本刀不动。（W1b 修订：CH1 22:10 那一份已把两句字面量改成「TUI 里敲 @ 会列出成员的 id 与显示名」；剩下要转交 CH1 修订人的列在 §7 (l)。）
- **候选规则有两份**：「候选 = 当前会话的活跃成员去掉总结成员（`r.cfg.Summary.Agent`）」在本刀的 `MentionCandidates` 与 CH1 的 `autoReplyRound`（`autoreply.go`）里各写了一份。后落地的一方核对两者一致（今天只差一个 `sumID != ""` 守卫，只在成员 id 是空串时才有区别）；以后要合成一份，另开一刀，本刀与 CH1 都不做。
- **语义方向一致**：CH1 v1.1 把「试图点名却一个都没认出来」定为谁都不问、只打一行说明（`hasStrayAt`，半角 `@` 与全角 `＠` 都算；CH1 v1.1 §1.1 E）。本刀的列表只可能把 `@` 补成认得出的 id，不会造出新的「没认出的 @」；全角 `＠` 不开列表（B6 只认 `msg.Text == "@"`；原型实测：敲 `string(rune(0xff20))` 之后行 nil，同装置敲半角 `@` 行是 6 行），落到 CH1 那一支，由它的说明句提示改写半角 `@`。

---

## 0. 病根 / 动机 / 实测（行号以 HEAD `3d134ee` 为准）

1. **今天的输入框没有任何补全。** `@` 是普通字符（`internal/tui/model.go:369-372` 交给 textarea）。成员 id 是 `opus-5.0-max`、`fable-5.1-max`（Griffin 的真配置，`docs/00_DECISIONS.md:1740`）、`gpt-6-sol-low`（CXH 真机复验时在隔离配置里 `/add` 起的，`:2186`）这类长串，Griffin 要记住并逐字敲对，敲错了 `ParseMentions`（`internal/repl/mention.go:62-94`）解析不出、CH1 的对话模式就会问错人或谁都不问。
2. **mention 的字法已经冻结在 `ParseMentions`**：左边界（`@` 的前一个字符不能是 `[A-Za-z0-9_@.-]`，`mentionLeftForbidden` `:105-113`；行首、空白、中文、括号都可以）；token 字符类是 ASCII 的 `[A-Za-z0-9_.-]`（`mentionTokenChar` `:118-126`）；解析时 ASCII 小写折叠，id 全小写（`mentionIDPattern` `:99`）。所以补全只能按 **id 前缀、大小写不敏感**筛：按显示名筛做不到——敲一个中文字，mention 就在那里断了（token 字符类只吃 ASCII），这不是选择题。
3. **残影规律：两种放法裸用都有残影，下方加钉光标才是零。** 源码机制（bubbletea v2.0.9 / ultraviolet 上面那个伪版本）：
   - 帧高一变，`cursed_renderer.go:329-340` 置 `Erase` 并按新高 `Resize`；ultraviolet `terminal_renderer.go:1306-1308` 先把旧缓冲缩到新高，再走 `clearUpdate`（`:1310-1312`）→ `clearBelow(0)`（`:1188-1191`）→ `move`：`move` 用的高度已经是新高，于是 `:457-463` 把记下的光标行**静默夹到新高 - 1、不发任何移动**。真光标还在旧行上，整帧往下错开。这就是 RB-B 的公式：**残影行数 = max(0, 缩高前光标相对帧顶的行 - (新帧高 - 1))**（#41 硬约束第 1 条）。
   - 光标停在哪：`View.Cursor == nil`（Council 今天只在 RB-B 的收尾窗口里设它，`model.go:394-396`）时，一帧结束光标停在最后改动的那一格（`cursed_renderer.go:516-527` 只处理行尾）；而**每次帧高变化之后**，`terminal_renderer.go:1352-1354` 都把光标移到新帧的**最后一行**。补全列表的开、筛、关全是帧高变化，所以不钉光标时，光标几乎总停在帧底，下一次缩高就留下「缩掉的行数」那么多行残影，而且残影一旦出现，之后再长高也清不掉。
   - 钉光标（`View.Cursor` 非 nil）时每帧结束都 `MoveTo(光标)`（`cursed_renderer.go:512-515`），缩高从钉住的行开始算。列表在输入框**下方**、光标钉在输入框里正在打字的那一格：光标行 = 2 + 输入框内的行（0-5）≤ 7 = 列表收起后的新帧高 8 - 1，公式恒为 0。列表在**上方**：光标行 = 2 + 列表行数 k + 输入框内的行，收起后新帧高 - 1 = 7，残影 = max(0, k + 行 - 5)，列表满 6 行或光标不在输入框第 1 行时就有残影。
   - **VT 回放实测**（探针：`internal/tui/rbb_vt_test.go` 的模拟器 `rbbNewVT` 与录制器 `rbbRecorder`，真 `tea.Program`、`tea.WithWindowSize(80, 24)`、`tea.WithInput(nil)`；先提交一次让假 Submitter 打出一行 `ATXEND` 并交回 8 个候选（`alpha b1 b2 b3 beta bravo c1 c2`，列表开着时 5 行候选加 1 行提示 = 6 行）；之后每个键 `p.Send` 完等录制器多出一块（至多 150ms）再发下一个；回放后「残影」= 最后一个表头行与 `ATXEND` 之间多出来的行数；**每格跑 10 次，10 次读数完全相同**）：

     | 场景（按键序列） | 下方 + 钉光标（本契约） | 下方、不钉 | 上方 + 钉光标 | 上方、不钉 |
     |---|---|---|---|---|
     | `open_esc`：`@`、Esc | 0 | 6 | 1 | 6 |
     | `down_tab`：`@`、↓、Tab | 0 | 6 | 1 | 0 |
     | `narrow`：`@`、`b`、`r`、`x`、退格、Esc（6 → 5 → 1 → 0 → 1 → 0 行） | 0 | 7 | 0 | 7 |
     | `backspace`：`@`、`a`、退格、退格 | 0 | 11 | 1 | 11 |
     | `multiline`：`x` Ctrl-J `y` Ctrl-J `z` Ctrl-J `@`、↓、Esc（光标在输入框第 4 行） | 0 | 6 | 4 | 0 |
     | `blink`：`@`、等 700ms、Esc | 0 | 6 | 1 | 1 |
     | `enter_accept`：`@`、`b`、回车 | 0 | 6 | 0 | 6 |
     | 退出：`@`、Ctrl-C、Ctrl-C，回放后再喂 `exitClearSeq`，屏幕与回滚区里没有表头、横线、候选行 | 5/5 干净 | 0/5 干净 | 0/5 干净（v1.1 测） | 0/5 干净（v1.1 测） |

     读法：不钉光标时残影就是「缩掉的行数」且逐次累加（`backspace` 的 11 = 6 → 1 缩 5，再 1 → 6 长回去，再 6 → 0 缩 6）；等一次光标闪烁（700ms）也没有把光标带回安全处（`blink` 一行）。「上方」在 `down_tab`、`multiline` 不钉时为 0 只是那一步最后改的恰好是输入框行，换个序列就有（`open_esc`）。**下方 + 钉光标七个场景 70 次全 0、退出 5/5 干净**，这是本契约的放法；§2.5 的 V1 把它冻结成守卫。另测：列表开着时 Ctrl-C 若直接退出（不先收起），钉光标下退出也是 5/5 干净——所以 Ctrl-C 先收起（§1.1 D7）不是为了残影，是为了退出路上没有钉住的真光标、X-39e 的退出尾巴字节不变（`x39e_*` 用例），以及与 Esc 同一种「先收起」的语义。

     **v1.1 复测**（`at1_v11` 原型，夹具换成 V1 的 `atxMembers`，即 §2.5 的装置，列表开着时同样是 5 行候选加 1 行提示；放法由驱动脚本改 `model.go` 的副本切换：上方 = A4，不钉 = A1，上方 + 钉光标 = A4 再把钉住的光标 Y 加上列表行数；每格 5 次）：上表七个场景 × 四种放法 28 格，每格 5 次读数相同，且与 v1 的数字逐格相同；退出一行上方两格都是 0/5 干净（上面表里标了「v1.1 测」）。表头行数、未知序列另记：凡残影 0 的格子表头 1 行、未知序列 0；未知序列在全部格子里都是 0。

     **v1.1 另测的场景**（同一装置，每个 5 次，5 次读数相同；「窗口变尺寸」只给 Program 发 `WindowSizeMsg`，模拟器仍是 80x24，量的是帧高变化本身）：

     | 场景（按键序列） | 下方 + 钉光标（本契约） | 下方、不钉 |
     |---|---|---|
     | 窗口变尺寸：`@`、`WindowSizeMsg{80, 12}`、`WindowSizeMsg{80, 24}`、Esc | 0 | 9 |
     | 窗口变尺寸：`@`、`WindowSizeMsg{80, 12}`、Esc | 0 | 6 |
     | 空闲时插一行：`@`、`p.Send(printLine("ATXEND"))`、Esc（残影从插进来的那一行量） | 0 | 0 |
     | 粘贴收起：`@`、`PasteMsg{"br"}` | 0 | 6 |
     | Ctrl-C 收起：`@`、Ctrl-C（一次，不退出） | 0 | 6 |
     | 8 行输入（textarea 滚动）：`a` Ctrl-J … `g` Ctrl-J（7 次）、`@`、↓、Esc | 0 | 6 |
     | 长行折行：88 个 `x`、空格、`@`（共 90 字，折成两行）、↓、Esc | 0 | 6 |
     | 中文前缀：`中`、`文`、`@`、↓、Esc | 0 | 6 |
     | 上方放法的对照序列：`x` Ctrl-J `@`、↓、Tab | 0 | 6 |

     下方 + 钉光标这一列全部是残影 0、表头 1 行、未知序列 0：列表开着时的另外六类（空闲时插一行、粘贴收起、Ctrl-C 收起、8 行输入、长行折行、中文前缀），钉光标下都不留残影。「窗口变尺寸」两行只说明**由 `WindowSizeMsg` 引起的帧高变化**（模拟器不缩）在钉光标下不留残影；**真终端缩放没测**（终端自己的重排与滚动不在模拟器里），由 §6 真机步骤 8 看（W1b 修订）。这些场景没有进 V1（W1 提过把第一行加进 V1 当守卫，裁决书没有要求，本版不加）。
4. **候选从哪来、什么时候取。** 活跃成员在库里（`svc.Participants(ctx, convID, false)`，`internal/repl/round.go:130` 同一个读法），总结成员 id 在 `r.cfg.Summary.Agent`（`round.go:181`），显示名在 `r.cfg.Agents[id].DisplayName`（`verbosity.go:50-55` 同一个来源）。REPL 这几个字段都不加锁、只许在没有命令在跑时读（`observer.go:50-54`、`verbosity.go:27-31` 的同一条纪律）。**在 Update 里读库**会阻塞事件循环；**弹出时在 Cmd 里异步读**会与紧跟着的一次回车起的 `Submit`（`/new`、`/use` 写 `r.cur`，`commands.go:672`、`:735`、`:797`）并发读写。本契约的做法：**每次 `Submit` 无错返回之后，在同一个 goroutine 上接着取一次候选**，随 `submitDoneMsg` 交回事件循环。论证：
   - 与命令天然串行（同一个 goroutine、`Submit` 已经返回），不与任何命令并发读 REPL；不在 Update 里读库。
   - 快照永远是新的：会话与成员只会被命令改（`/use`、`/new`、`/team use`、`/add`、`/remove`、`/config reload …`），每条命令都走 `Submit`，之后立刻重取。启动时 `r.cur` 恒为 nil（`app.go:230-231` 在 `tui.Run` 之前只调 `SetConcise`，没有任何路径在进 TUI 前选会话），所以初值 nil 就是对的，不需要启动时取。
   - 代价：每次提交多一次读库。原型在 `t.TempDir()` 下的真 SQLite 上连调 200 次 `MentionCandidates`（6 个成员、1 个已离开）：p50 < 0.5ms（Windows 计时器分辨率以下）、p99 0.52ms、最大 1.0ms。只加在忙的时间上，不进事件循环。
   - 取候选用 Model 的根 ctx（`m.ctx`），不用这一条命令的 `runCtx`：命令被 Ctrl-C 取消过时 `runCtx` 已取消，读库必失败（原型实测 `context canceled`），候选会被清空。
5. **忙时不弹。** 一轮在跑时敲 `@` 不开补全（§1.1 D1）。于是补全列表与 RB-B 看板永不同屏：开着列表 ⇒ 不忙且不在收尾窗口 ⇒ `floor == 0`（`floor` 只在忙时带名单的 RoundStart 上增长、在 `beginSubmit` 与 `boardFinalizeMsg` 归零，`model.go:240-244`、`:278`、`:494`）；而列表开着时回车只补全、不提交，也就不会从「开着列表」直接进入忙。RB-B 的「一次提交内只长不缩」「唯一缩高点在 finalize」两条不变量因此不受本刀影响，`boardLimit()` 不改。唯一的例外窗口是 RB-B 剩余项 (h)（出错 / 退出的 Sequence 两次 Send 之间进来一次 Enter），本刀不扩大它。

---

## 1. 范围

### 1.1 做

本刀的生产代码**全部逐字冻结**（三处，合计约 260 行，含注释）。没有「名字实现方定」的地方。

#### A. 新文件 `internal/repl/mentioncand.go`（逐字照抄）

```go
package repl

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

// MentionCandidate 是 TUI 输入框 @ 补全列表里的一行（00_DECISIONS #47 补充，AT1）。
// ID 是成员的 agent id（全小写，与 ParseMentions 解析出的 token 同一空间）；
// Name 是配置里的 display_name 压成一行、去掉控制字符之后的样子，配置里没有这个 id 或 display_name 为空时是空串。
type MentionCandidate struct {
	ID   string
	Name string
}

// MentionCandidates 返回当前会话的活跃成员，去掉 [summary] 总结成员，按 ID 字节序升序。
// 只读：不打印、不改 REPL 的任何字段、不写库。
// 没有当前会话、或持久化没起来时返回 nil, nil。
//
// 调用纪律与 SetRoundObserver 相同：只许在没有 Submit 在跑的时候调用。
// TUI 在执行 Submit 的那个 goroutine 上、Submit 返回之后调用它（internal/tui 的 submitCmd），
// 与命令串行，所以不加锁。
func (r *REPL) MentionCandidates(ctx context.Context) ([]MentionCandidate, error) {
	if r.svc == nil || r.cur == nil {
		return nil, nil
	}
	parts, err := r.svc.Participants(ctx, r.cur.ID, false)
	if err != nil {
		return nil, err
	}
	sumID := r.cfg.Summary.Agent
	out := make([]MentionCandidate, 0, len(parts))
	for _, p := range parts {
		if sumID != "" && p.AgentID == sumID {
			continue
		}
		name := ""
		if ac, ok := r.cfg.Agents[p.AgentID]; ok {
			name = mentionName(ac.DisplayName)
		}
		out = append(out, MentionCandidate{ID: p.AgentID, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// mentionName 把 display_name 变成补全列表里能放进一行的样子：控制字符换成空格，再去掉首尾空白。
// 配置层不校验 display_name，换行或 ESC 进了活区会把帧高算错。
func mentionName(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
}

// MentionBoundaryOK 报告 prev 贴在 @ 左边时，这个 @ 能不能当 mention 的起点。
// 与 ParseMentions 的左边界是同一条规则（mentionLeftForbidden）；非 ASCII 字符不挡，
// 因为 ParseMentions 按字节看时，多字节字符的最后一个字节不在禁止集合里。
func MentionBoundaryOK(prev rune) bool {
	return prev >= 0x80 || !mentionLeftForbidden(byte(prev))
}

// MentionTokenRune 报告 r 是不是 ParseMentions 最长匹配吃进去的字符（mentionTokenChar，ASCII 的 [A-Za-z0-9_.-]）。
func MentionTokenRune(r rune) bool {
	return r < 0x80 && mentionTokenChar(byte(r))
}
```

- 五个名字冻结（测试腿直接调用 `MentionCandidates`、`MentionBoundaryOK`、`MentionTokenRune` 与类型 `MentionCandidate` 的字段 `ID`、`Name`）。导出是因为调用方在 `internal/tui`；判据放在 `repl` 包、委托给 `mention.go` 的两个未导出函数，左边界与字符类只有一份（§4「只在审查时看」第 1 条）。
- 不改 `mention.go`、`commands.go`、`seam.go`、`round.go`、`repl.go` 的任何一行。

#### B. `internal/tui/model.go`：十一个补丁（锚点逐字取自 HEAD，含行首 tab，每个在文件里恰好出现一次；按文件里的先后顺序列）

**B1** 锚点（HEAD `:31-34`）：

```go
// submitDoneMsg 把后台提交的结果送回事件循环。
type submitDoneMsg struct {
	err error
}
```

替换成：

```go
// submitDoneMsg 把后台提交的结果送回事件循环。
// members 是提交之后在同一个 goroutine 上取的 @ 补全候选（见 submitCmd）；err 非 nil 时不取，为 nil。
type submitDoneMsg struct {
	err     error
	members []repl.MentionCandidate
}

// mentionSource 是 Submitter 可以另外实现的能力：列出 @ 补全的候选。
// 不并进 Submitter：既有的假 Submitter 都没有这个方法，没实现的一律当作没有候选。
type mentionSource interface {
	MentionCandidates(ctx context.Context) ([]repl.MentionCandidate, error)
}

// atState 是输入框 @ 补全的状态（00_DECISIONS #47 补充，AT1）。零值是「没有在补全」。
// row、col 是那个 @ 在 textarea 里的行与 rune 下标；prefix 是 @ 与光标之间的字，原样；
// sel 是选中项在筛选结果里的下标。
type atState struct {
	armed  bool
	row    int
	col    int
	prefix string
	sel    int
}

// atMaxRows 是补全列表最多显示的候选行数。
const atMaxRows = 5
```

**B2** 锚点（HEAD `:120-122`）：

```go
	// pinCursor 为真时 View 把光标钉在表头行首。
	pinCursor bool
}
```

替换成：

```go
	// pinCursor 为真时 View 把光标钉在表头行首。
	pinCursor bool

	// members 是 @ 补全的候选快照，只在 submitDoneMsg 里整份换掉。
	members []repl.MentionCandidate
	// at 是 @ 补全的状态。
	at atState
}
```

**B3** 锚点（HEAD `:312-313`）：

```go
	case submitDoneMsg:
		m.busy = false
```

替换成：

```go
	case submitDoneMsg:
		m.members = msg.members
		m.busy = false
```

**B4** 锚点（HEAD `:335-338`）：

```go
	case tea.KeyPressMsg:
		switch {
		case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':
			if m.finalizing {
```

替换成：

```go
	case tea.KeyPressMsg:
		// @ 补全列表在屏幕上时，这六个键归它，不交给下面的既有分支：
		// Ctrl-C 与 Esc 只收起列表（不退出、不取消、不改状态栏）；上下键移动选中项（到头不回绕）；
		// Tab 与回车用选中项补全，回车在这里不提交。列表不在屏幕上时一律走既有分支。
		if m.atVisible() {
			switch {
			case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':
				m.at = atState{}
				return m, nil
			case msg.Code == tea.KeyEscape:
				m.at = atState{}
				return m, nil
			case msg.Code == tea.KeyUp:
				if m.at.sel > 0 {
					m.at.sel--
				}
				return m, nil
			case msg.Code == tea.KeyDown:
				if m.at.sel < len(m.atMatches())-1 {
					m.at.sel++
				}
				return m, nil
			case msg.Code == tea.KeyTab, msg.Code == tea.KeyEnter:
				return m.atAccept()
			}
		}
		switch {
		case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':
			if m.finalizing {
```

**B5** 锚点（HEAD `:352-354`）：

```go
		case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'j':
			m.ta.InsertRune('\n')
			return m, nil
```

替换成：

```go
		case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'j':
			m.ta.InsertRune('\n')
			m = m.atRecheck()
			return m, nil
```

**B6** 锚点（HEAD `:369-379`）：

```go
		default:
			var cmd tea.Cmd
			m.ta, cmd = m.ta.Update(msg)
			return m, cmd
		}

	case tea.PasteMsg:
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		m.status = fmt.Sprintf("粘贴了 %d 个字符", utf8.RuneCountInString(msg.Content))
		return m, cmd
```

替换成：

```go
		default:
			var cmd tea.Cmd
			m.ta, cmd = m.ta.Update(msg)
			m = m.atRecheck()
			if !m.at.armed && msg.Text == "@" {
				m = m.atArm()
			}
			return m, cmd
		}

	case tea.PasteMsg:
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		m.status = fmt.Sprintf("粘贴了 %d 个字符", utf8.RuneCountInString(msg.Content))
		m.at = atState{}
		return m, cmd
```

**B7** 锚点（HEAD `:389-398`，整个 `View`）：

```go
func (m Model) View() tea.View {
	content := lipgloss.JoinVertical(lipgloss.Left, append(m.chrome(), m.ta.View())...)
	v := tea.NewView(content)
	// Council 使用内联模式，活区之外的原生回滚区必须保留。
	v.AltScreen = false
	if m.pinCursor {
		v.Cursor = tea.NewCursor(0, 0)
	}
	return v
}
```

替换成：

```go
func (m Model) View() tea.View {
	pop := m.atLines()
	parts := append(append(m.chrome(), m.ta.View()), pop...)
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	v := tea.NewView(content)
	// Council 使用内联模式，活区之外的原生回滚区必须保留。
	v.AltScreen = false
	if m.pinCursor {
		v.Cursor = tea.NewCursor(0, 0)
	} else if len(pop) > 0 {
		v.Cursor = m.atCursor()
	}
	return v
}

// atCursor 是补全列表在屏幕上时钉住的真光标：落在输入框里正在打字的那一格。
// 钉它是为了缩高（收起列表、筛短列表）时不留残影：渲染器从上一帧结束时的光标行开始缩，
// 残影行数 = max(0, 光标行 - (新帧高 - 1))；列表在输入框下方，光标钉在输入框里，这个数恒为 0。
// 不钉时光标停在上一帧最后改动的那一行（往往是列表最后一行），收起多少行就留多少行残影。
// ta 的副本关掉虚拟光标之后 Cursor() 才给出位置；只动副本，Model 里的 textarea 不变。
func (m Model) atCursor() *tea.Cursor {
	ta := m.ta
	ta.SetVirtualCursor(false)
	c := ta.Cursor()
	if c == nil {
		return tea.NewCursor(0, 0)
	}
	c.Y += len(m.chrome())
	c.Shape = tea.CursorBar
	c.Blink = true
	c.Color = nil
	return c
}
```

**B8** 锚点（HEAD `:461-463`）：

```go
func (m Model) frameHeight() int {
	return len(m.chrome()) + m.ta.Height()
}
```

替换成：

```go
func (m Model) frameHeight() int {
	return len(m.chrome()) + m.ta.Height() + len(m.atLines())
}

// atArm 在刚敲进一个 @ 之后开始补全：光标前一格是 @、它的左边界合法、此刻不忙也不在收口窗口。
func (m Model) atArm() Model {
	if m.busy || m.finalizing || m.ta.HasSelection() {
		return m
	}
	line := []rune(m.atLine())
	col := m.ta.Column()
	if col < 1 || col > len(line) || line[col-1] != '@' {
		return m
	}
	if col >= 2 && !repl.MentionBoundaryOK(line[col-2]) {
		return m
	}
	m.at = atState{armed: true, row: m.ta.Line(), col: col - 1}
	return m
}

// atRecheck 在输入框变过之后重判补全还成不成立，不成立就收起。前缀变了选中项回到第一个。
func (m Model) atRecheck() Model {
	if !m.at.armed {
		return m
	}
	if m.ta.HasSelection() || m.ta.Line() != m.at.row {
		m.at = atState{}
		return m
	}
	line := []rune(m.atLine())
	col := m.ta.Column()
	if col <= m.at.col || m.at.col >= len(line) || col > len(line) || line[m.at.col] != '@' {
		m.at = atState{}
		return m
	}
	if m.at.col > 0 && !repl.MentionBoundaryOK(line[m.at.col-1]) {
		m.at = atState{}
		return m
	}
	for _, r := range line[m.at.col+1 : col] {
		if !repl.MentionTokenRune(r) {
			m.at = atState{}
			return m
		}
	}
	if p := string(line[m.at.col+1 : col]); p != m.at.prefix {
		m.at.prefix = p
		m.at.sel = 0
	}
	return m
}

// atLine 是光标所在的那一行。
func (m Model) atLine() string {
	lines := strings.Split(m.ta.Value(), "\n")
	if row := m.ta.Line(); row >= 0 && row < len(lines) {
		return lines[row]
	}
	return ""
}

// atMatches 是按前缀筛过的候选：id 以前缀的小写形式开头。顺序同 members。
func (m Model) atMatches() []repl.MentionCandidate {
	p := strings.ToLower(m.at.prefix)
	var out []repl.MentionCandidate
	for _, c := range m.members {
		if strings.HasPrefix(c.ID, p) {
			out = append(out, c)
		}
	}
	return out
}

// atVisible 报告补全列表此刻在不在屏幕上。只有在屏幕上时上下键、Tab、回车、Esc、Ctrl-C 才归它。
func (m Model) atVisible() bool {
	return len(m.atLines()) > 0
}

// atLines 是补全列表的行，放在输入框下方。不在补全、终端高未知、没有匹配、或放不下一行时为 nil。
func (m Model) atLines() []string {
	if !m.at.armed || m.termHeight <= 0 {
		return nil
	}
	list := m.atMatches()
	if len(list) == 0 {
		return nil
	}
	avail := m.termHeight - 1 - (len(m.chrome()) + m.ta.Height())
	show := min(len(list), atMaxRows, avail)
	if show < 1 {
		return nil
	}
	sel := min(max(m.at.sel, 0), len(list)-1)
	top := max(0, sel-show+1)
	width := min(max(m.width, 20), 120)
	idW := 0
	for _, c := range list {
		idW = max(idW, ansi.StringWidth(c.ID))
	}
	lines := make([]string, 0, show+1)
	for i := top; i < top+show; i++ {
		mark := "  "
		if i == sel {
			mark = "> "
		}
		line := mark + "@" + list[i].ID
		if list[i].Name != "" {
			line = mark + "@" + padCells(list[i].ID, idW) + "  " + list[i].Name
		}
		lines = append(lines, ansi.Truncate(line, width, ""))
	}
	if len(list) > show && avail > show {
		lines = append(lines, ansi.Truncate(fmt.Sprintf("  共 %d 个，上下键翻看", len(list)), width, ""))
	}
	return lines
}

// atAccept 用选中的候选替换 @ 与已输入的前缀：删掉它们，再插入 "@<id> "。光标落在那个空格之后。
func (m Model) atAccept() (tea.Model, tea.Cmd) {
	list := m.atMatches()
	pick := list[min(max(m.at.sel, 0), len(list)-1)]
	n := 1 + len([]rune(m.at.prefix))
	var cmds []tea.Cmd
	for i := 0; i < n; i++ {
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		cmds = append(cmds, cmd)
	}
	m.ta.InsertString("@" + pick.ID + " ")
	m.at = atState{}
	return m, tea.Batch(cmds...)
}
```

**B9** 锚点（HEAD `:489-490`）：

```go
	m.ta.Reset()
	m.busy = true
```

替换成：

```go
	m.ta.Reset()
	m.at = atState{}
	m.busy = true
```

**B10** 锚点（HEAD `:513`，一行）：

```go
	cmds = append(cmds, submitCmd(m.sub, runCtx, text))
```

替换成：

```go
	cmds = append(cmds, submitCmd(m.sub, runCtx, baseCtx, text))
```

**B11** 锚点（HEAD `:542-547`，整个 `submitCmd`）：

```go
// submitCmd 在 Bubble Tea 管理的命令 goroutine 中调用 Submit。
func submitCmd(sub Submitter, ctx context.Context, text string) tea.Cmd {
	return func() tea.Msg {
		return submitDoneMsg{err: sub.Submit(ctx, text)}
	}
}
```

替换成：

```go
// submitCmd 在 Bubble Tea 管理的命令 goroutine 中调用 Submit。
// Submit 无错返回之后，在同一个 goroutine 上接着取 @ 补全的候选（sub 实现了 mentionSource 时）：
// 与命令串行，不与任何 Submit 并发读 REPL；也不在 Update 里读库。
// 取候选用 listCtx（Model 的根 ctx），不用 ctx：这一条命令被 Ctrl-C 取消过时 ctx 已经取消，读库会失败。
// 取失败时候选为 nil，列表不弹。
func submitCmd(sub Submitter, ctx, listCtx context.Context, text string) tea.Cmd {
	return func() tea.Msg {
		err := sub.Submit(ctx, text)
		if err != nil {
			return submitDoneMsg{err: err}
		}
		var members []repl.MentionCandidate
		if src, ok := sub.(mentionSource); ok {
			if list, lerr := src.MentionCandidates(listCtx); lerr == nil {
				members = list
			}
		}
		return submitDoneMsg{members: members}
	}
}
```

- `import` 不变（`context`、`fmt`、`strings`、`ansi`、`repl` 在 HEAD `:3-16` 都已 import）。`chrome()`、`boardLimit()`、`closingLine()`、`quitCmd()`、`New`、`Init`、`Update`、`update`、`printLine`、`beginSubmit` 其余部分（含 X44 的门 `:478-481`）一个字节都不改。
- 注意 B4 与 B6 之间、B6 的 `default` 之前，HEAD 的 Enter 分支（`:356-364`）与 Esc 分支（`:366-367`）**原样保留**：列表不在屏幕上时回车照旧提交、Esc 照旧什么都不做（X44 冻结的「Esc 不清提示」）。

#### C. `internal/tui/run.go`：一个补丁

锚点（HEAD `:182-185`）：

```go
// IsQuit 转发到包级函数 repl.IsQuit。用 errors.Is 的那一层在 repl 里，这里不再写一份。
func (a replAdapter) IsQuit(err error) bool {
	return repl.IsQuit(err)
}
```

替换成：

```go
// IsQuit 转发到包级函数 repl.IsQuit。用 errors.Is 的那一层在 repl 里，这里不再写一份。
func (a replAdapter) IsQuit(err error) bool {
	return repl.IsQuit(err)
}

// MentionCandidates 原样转发，让 replAdapter 满足 mentionSource。
// 只由 submitCmd 在 Submit 返回之后、同一个 goroutine 上调用。
func (a replAdapter) MentionCandidates(ctx context.Context) ([]repl.MentionCandidate, error) {
	return a.r.MentionCandidates(ctx)
}
```

- `run.go` 的 import 不变（`context`、`repl` 已在 `:3-12`）。`Run`、`runWith`、`exitClearSeq`、`Submit` 一个字节都不改。

#### D. 冻结的论证（实现方照此实现；测试腿照此出用例）

- **D1 什么时候开（armed）。** 只在 `updateInput` 的按键 default 分支里、textarea 处理完这个键之后，并且 `msg.Text == "@"`、此前没有在补全时尝试开：光标前一格（rune）是 `@`；那个 `@` 在行首，或它左边那个 rune 满足 `repl.MentionBoundaryOK`（与 `ParseMentions` 的左边界同一条规则：行首、空白、括号、中文、全角标点可以；`[A-Za-z0-9_@.-]` 不行，所以邮箱 `a@b`、`@@` 不开）；不忙（`!m.busy`）、不在收尾窗口（`!m.finalizing`）、textarea 没有选区。记下 `row = ta.Line()`、`col = ta.Column() - 1`（rune 下标）、`prefix = ""`、`sel = 0`。**只有敲出来的 `@` 能开**：粘贴进来的 `@` 不开（`PasteMsg` 一律收起，D2），`@` 已经在那里、光标挪回它后面也不开。与任务书「行首或空白之后」的差别：任务书同时要求「与 `ParseMentions` 的左边界规则一致」，两者冲突处以后者为准，所以 `(@`、`中@`、`，@` 也开（M2 钉住）。
- **D2 什么时候收（disarm）。** 补全中（armed）每一次「交给 textarea 的按键」（default 分支）与 Ctrl-J 之后重判（`atRecheck`），任何一条不成立就收起（`m.at = atState{}`）：光标仍在记下的那一行；没有选区；光标列 > `@` 的列；那一格仍是 `@`；`@` 的左边界仍合法；`@` 与光标之间全是 `repl.MentionTokenRune`（ASCII `[A-Za-z0-9_.-]`，与 `ParseMentions` 的字符类同一份）。成立时 `prefix` = `@` 与光标之间的字（原样，大小写不改），**前缀变了 `sel` 归 0**，前缀没变 `sel` 保留（Delete、End、行尾按 → 这类键；M4 的 Delete 那段钉住，v1.1）。另外四处直接收起、不重判：列表在屏幕上时的 Esc、Ctrl-C（D7）；补全之后（D8）；`PasteMsg`（粘贴一律收起，粘进来的东西不开补全）；`beginSubmit` 在 `m.ta.Reset()` 之后（B9）。收起之后**只有再敲一个 `@` 才会重开**：Esc 收起后继续打字不重开（M6），光标左移到 `@` 前面再右移回来不重开（M7）。「光标在 mention 中间」（例如 `@b` 左移一格）仍算补全中，前缀按光标之前的字算（`""`，列出全部）；补全只替换 `@` 到光标这一段，光标右边的字原样留下（D8）。
- **D3 「补全中」与「在屏幕上」是两件事。** armed 但以下任一成立时列表不在屏幕上（`atLines()` 为 nil、帧高不加、不钉光标）：没有匹配（筛空）；终端高未知（`termHeight <= 0`，与 `boardLimit()` 同一规则）；放不下一行（D9 的 `avail < 1`）。**不在屏幕上时六个键全部走既有分支**：回车照常提交、Esc 照常什么都不做、上下键交给 textarea、Ctrl-C 照常退出或取消。筛空之后退格回到有匹配的前缀，列表重新出现（M3）；窗口拉矮到放不下再拉高，列表重新出现（M10）。筛空时不显示「没有匹配的成员」一行：少一次帧高变化，而且用户看得见自己打了什么（主代理定）。
- **D4 候选的来源与时机。** §0 第 4 条的论证冻结为：`submitCmd` 在 `sub.Submit` **返回 nil 之后**、同一个 goroutine 上，若 `sub` 实现了 `mentionSource`，用 `listCtx`（`beginSubmit` 里的 `baseCtx`，即 `m.ctx` 或兜底的 Background）调 `MentionCandidates`；返回 error 时候选为 nil；`Submit` 返回非 nil 时**不取**，`submitDoneMsg.members` 为 nil。`updateInput` 收到任何 `submitDoneMsg` 都先 `m.members = msg.members`（整份替换，含 nil；出错与退出两支也换，反正马上退出；「含 nil」由 M13 第 4 步钉住，v1.1）。没实现 `mentionSource` 的 Submitter（既有的全部测试假件）永远没有候选，行为与 HEAD 相同。
- **D5 候选集合（`repl.MentionCandidates`）。** 当前会话的活跃成员（`Participants(ctx, r.cur.ID, false)`，与 `/round` 取名单同一个读法）；去掉 id 恰好等于 `r.cfg.Summary.Agent` 的那一个（配置没写总结成员时不去）；裁判模式的成员**照列**（它们是成员，`ParseMentions` 认得，CH1 怎么处理 @ 裁判由 CH1 定，§7 (c)）；配置里已经没有这个 id 的成员照列、`Name` 为空。`Name` = `mentionName(display_name)`：`unicode.IsControl` 的字符换成空格、再去首尾空白（只剩空白的显示名变成空串，按「没有显示名」显示）。按 `ID` 字节序升序。`r.cfg` 经 `repl.New` 兜底恒非 nil（`repl.go:138-140`），`Agents` 为 nil map 时读取安全、`Summary.Agent` 为空串。没有当前会话、`svc == nil` 时 `nil, nil`；读库失败时 `nil, err`。只剩总结成员时是长度 0 的非 nil 切片（列表不弹，与 nil 同效）。
- **D6 筛选。** `atMatches` = `members` 里 `strings.HasPrefix(ID, strings.ToLower(prefix))` 的项，顺序同 `members`（已按 id 排序）。只按 id 前缀（§0 第 2 条）。前缀可以含 `.`、`-`、`_`（`@grok-4.` 筛出 `grok-4.7`）。
- **D7 按键表（列表在屏幕上时；判定顺序即 B4 里 `switch` 的顺序，在既有分支之前）。**

  | 键 | 判据（`tea.KeyPressMsg`） | 行为 | 返回的 Cmd | 状态栏 |
  |---|---|---|---|---|
  | Ctrl-C | `msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c'` | 只收起列表；不退出、不取消（此时不忙，本来也没有可取消的）。再按一次才走既有的退出 | nil | 不变 |
  | Esc | `msg.Code == tea.KeyEscape` | 只收起列表；输入框文字与光标不动 | nil | 不变（X44 的提示挂着时仍挂着，M6） |
  | ↑ | `msg.Code == tea.KeyUp`（不看修饰键） | `sel` 减 1，到 0 停住（不回绕）；输入框不动 | nil | 不变 |
  | ↓ | `msg.Code == tea.KeyDown`（不看修饰键） | `sel` 加 1，到最后一个停住（不回绕） | nil | 不变 |
  | Tab、回车 | `msg.Code == tea.KeyTab`，或 `msg.Code == tea.KeyEnter`（都不看修饰键） | 补全（D8）；**回车不提交**（不回显、不调 `Submit`、不进忙、不经 X44 的门） | textarea 的 Cmd 合成的 `tea.Batch`（光标闪烁用；不含任何提交） | 不变 |

  列表在屏幕上时其余按键与消息（全部走既有分支，行为与 HEAD 相同，之后按 D2 重判）：可打印字符与退格、←→、Home/End 等交给 textarea（重判，可能收起或改前缀）；Ctrl-J 插换行（重判：换了行，收起）；`PasteMsg` 插入、改状态栏为 `粘贴了 N 个字符`、收起；`tea.WindowSizeMsg` 照旧只更新尺寸、返回 nil Cmd（列表按新 `termHeight` 重算行数，可能变少、消失或恢复，不改 armed，M10）；焦点、光标闪烁等交给 textarea（不重判：文字与光标不变）。忙时的消息（tick、RoundStart 等）在补全中到不了（补全中必不忙，§0 第 5 条）。
- **D8 补全的做法。** 取 `atMatches()[sel]`（`sel` 夹在范围内）；经 `m.ta.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})` 连退 `1 + len([]rune(prefix))` 次（删掉 `@` 与前缀；前缀全是 ASCII，rune 数 = 列数差），再 `m.ta.InsertString("@" + id + " ")`；收起。`atAccept` 只在 `atVisible()` 为真时可达（B4），匹配必不为空，**不做空判、不许加防御分支**（加了是等价改动，但违反逐字照抄，收割时会撞；A39 证明这个依赖被用例钉着）。结果：`@` 与光标之间的字换成 `@<完整 id> `（id 是库里的小写原样，`@GR` 补成 `@grok-4.7 `），光标在那个空格之后，光标右边的字、其它行一个字节不动（M5 用「补完再打一个 z」钉住光标位置）。不用 `SetValue`：它把光标放到整段末尾，多行时回不到原位。
- **D9 渲染（冻结）。** 列表是 `View` 里输入框**下面**的若干行（B7：`chrome()`、`ta.View()`、`atLines()` 依次 `JoinVertical`），**不进 `chrome()`**（`chrome()` 是输入框上方的固定行，RB-B 的 `boardLimit()` 不因本刀改变），但**进 `frameHeight()`**（B8，X-39e 的退出清屏按它清）。行数：`avail = termHeight - 1 - (len(chrome()) + ta.Height())`（补全中 `floor == 0`，即 `termHeight - 9`），候选行 `show = min(匹配数, 5, avail)`，`show < 1` 时不显示；匹配数 > `show` 且 `avail > show` 时多一行提示 `  共 N 个，上下键翻看`（N = 匹配总数）。所以帧高 ≤ `termHeight - 1`（与 RB-B「帧高必须 ≤ 终端高 - 1」同一条）。窗口：选中项总在窗口里，`top = max(0, sel - show + 1)`（选中项在前 `show` 个里时从头显示，否则选中项贴底）。每行：选中的前缀 `> `，其余 `  `（两个 ASCII 空格；#41 约定：模糊宽度符号等 Griffin 真机试过再换）；接 `@` 与 id；有显示名时 id 用 `padCells` 补到本次匹配里 id 的最大显示宽度、再接两个空格与显示名；没有显示名时 id 后面什么都不接（不留尾空格）。每行（含提示行）用 `ansi.Truncate(行, min(max(width, 20), 120), "")` 截到横线的宽度，不用 lipgloss `Style.Width`（它折行，帧高会算错）。
- **D10 钉光标（冻结）。** 列表在屏幕上、且 `pinCursor` 为假时，`View.Cursor` = 输入框光标的位置（textarea 副本关掉虚拟光标后 `Cursor()` 给的坐标，Y 加 `len(chrome())`），形状 `tea.CursorBar`、`Blink` 为真、`Color` 为 nil；列表不在屏幕上时 `View.Cursor` 与 HEAD 相同（nil，或 RB-B 的 (0,0)）。`pinCursor` 的优先级最高（B7 的 `if` 顺序）；补全中本来不会有 `pinCursor`（§0 第 5 条）。**字节副作用**（原型实测，§2.5 V1 的装置里）：列表出现的那一帧多出显示光标 `ESC[?25h` 与光标形状 `ESC[5 q`（闪烁竖线），收起的那一帧多出 `ESC[?25l` 与 `ESC[0 q`（恢复终端默认形状）。`ESC[0 q` 会把用户自己设的光标形状复位成终端默认，与 RB-B 已知代价同一件事（#41），真终端未测（§7 (a)）。
- **D11 与 X44、RB-A、RB-B、X-39e 的关系。** 回车在列表开着时由 B4 截走，到不了 `beginSubmit`，X44 的门对「列表开着」这种状态不起作用，也不需要：补全不提交。列表不在屏幕上时回车、Esc、粘贴、窗口尺寸与 HEAD 逐字节相同（X44 的 M 系列、RB-A 的 Acc1「空闲时 WindowSizeMsg 返回 nil Cmd」原样绿）。列表开着时 Esc 只收起、不写 `m.status`，所以 X44 提示照样挂着（M6）。RB-A 的 tick 只在忙时装，补全中不忙，互不相干。X-39e：退出只可能在列表收起之后（Ctrl-C 先收起；`/quit` 要回车提交，列表开着时回车只补全），所以退出那一帧没有钉住的真光标，退出尾巴与 HEAD 相同；`frameHeight` 仍把列表算进去（防御：被杀的路不清屏，正常退出路上列表必已收起，`exitClearSeq` 与 HEAD 相同），M15 直接对 `exitClearSeq` 钉住这一点。

### 1.2 不做（两条腿都不许越界）

- 不改 REPL 的 mention 解析与路由：`mention.go`（`ParseMentions`、`mentionLeftForbidden`、`mentionTokenChar`、`mentionIDPattern`）、`round.go`、`ask.go`、`commands.go`、`seam.go` 一个字节都不动。不改 CH1 的对话模式逻辑。
- 不做「@ 所有人」之类的新语义；不做 `/` 命令补全；不按显示名筛；不做「没有匹配」提示行；不在忙时开补全；不做鼠标点选；不做回绕。
- 不改 `internal/tui` 里除 `model.go`、`run.go` 以外的文件（`roundobs.go`、`writer.go`、`vt_*.go`）；`run.go` 只加 C 那一个方法。不改 `internal/app`、`cmd/`、`go.mod`、`go.sum`。
- **既有 `_test.go` 一个字节都不许改（含注释），本刀零授权。** 原型实测：三处生产改动落盘后全量 `go test -count=1 ./...` 全绿，没有任何既有用例需要改（`Submitter` 接口不变是关键：既有假件 `tui1bSubmitter`、`rbbVTSub`、`mlx` 的计数 Submitter 等都不实现 `mentionSource`，行为不变）。
- 测试腿不写生产代码，实现腿不写 `_test.go`。测试腿不加 `TestMain`（沿用 `internal/tui/rba_fakecli_test.go` 的那一个）。

---

## 2. 验收（测试腿新写的文件）

### 2.0 通用纪律

- 新文件五个：`internal/repl/atx_cand_test.go`（C1、C2）、`internal/tui/atx_model_test.go`（M1-M15）、`internal/tui/atx_program_test.go`（P1）、`internal/tui/atx_run_test.go`（R1）、`internal/tui/atx_vt_test.go`（V1）。都不带构建标签。
- **包级标识符一律以 `atx` 开头**，测试函数 `TestAtx...`。只读复用同包既有辅助、不许改它们：tui 包 `tui1bNew`、`tui1bSubmitter`（方法 `count`、`text`）、`tui1bPrinter`（方法 `cmd`）、`tui1bNopDrainer`、`tui1bStep`、`tui1bRun`（返回的 `tui1bTrace` 的字段 `msgs`、`log` 与方法 `hasQuit`）、`tui1bType`、`tui1bKey`、`tui1bCtrl`、`tui1bEnter`、`tui1bEsc`、`tui1bStripANSI`、`tui1dBuf`、`tui1dWaitFor`、`tui1dESC`、`rbaFlat`、`rbaNewTuiRig`、`rbaTuiRig`（字段 `svc`、`r`、`lw`、`convID`）、`rbbNewVT`（字段 `unknown`，方法 `feed`、`all`）、`rbbRecorder`（方法 `count`、`snapshot`）、`rbbWaitUntil`，生产的 `replAdapter`、`newRoundObserver`、`printLine`、`Run`、`exitClearSeq`、`New`、`Options`；repl 包 `newRoundStreamService`、`New`、`RulesContext`、`ParseMentions`。
- 可以读 `Model` 的未导出字段 `ta`、`finalizing` 与方法 `chrome()`、`frameHeight()`、`atLines()`；可以构造未导出的消息 `submitDoneMsg{members: …}`、`roundStartMsg{ev: …}`、`boardFinalizeMsg{}`，可以引用接口名 `mentionSource`。这些名字本刀冻结。**不许**读 `m.at`、`m.members`（内部状态，用行为断言），不许引用 `atMaxRows` 与提示行的格式串（测试文件自己写字面量，§3）。
- 不起真 provider；store 一律在 `t.TempDir()` 下；不碰 `C:\Users\25142\.council`。
- 断言整串（整列）相等；例外：R1 的终端字节流（剥 ANSI、删空格之后找子串，理由同 RB-A W1）与前提守卫。每条用例注释写【判别力】：能不能为假、只差一处的对照是哪条。缺席断言都配同装置的正对照（列表不在 ↔ 同一装置里列表在；没提交 ↔ 同一装置里真的提交）。
- 不可见字符用十六进制码位写（`string(rune(0x1b))`、`string(rune(0x09))`、`rune(0xff0c)`），源码里不放这些字符本身，不用反斜杠 u 转义。
- **HEAD 上的预期**：除 `atx_run_test.go` 之外的四个文件引用本刀新增的标识符（`atLines`、`submitDoneMsg.members`、`repl.MentionCandidate`、`MentionCandidates`、`MentionBoundaryOK`、`MentionTokenRune`），HEAD 上两个包都编译不过，这是预期的红（同 RB-B §2）。`atx_run_test.go` 只用既有名字，它在 HEAD 上编不过只是因为同包。测试腿交付时报告：在派工时的 main HEAD 的隔离拷贝里放入五个文件，用 `go test -count=1 -run '^$' -gcflags=-e ./internal/tui ./internal/repl` 列出全部编译错误，核对它们只涉及这些新名字。`go vet` 每个包只报第一个错，不够。（2026-09-29 按落地后订正第六条改。）

**公用夹具（冻结，M 系列用）**：

- `atxMembers`（8 个，已按 id 排序）：`{alpha, "Alpha"}`、`{b1, ""}`、`{beta, "贝塔"}`、`{bravo, "Bravo"}`、`{c1, "C1"}`、`{c2, ""}`、`{c3, ""}`、`{grok-4.7, "Grok 4.7"}`（`repl.MentionCandidate{ID, Name}`）。
- 「新 Model(w, h, list)」= `tui1bNew(t, nil)` → `tui1bStep(WindowSizeMsg{w, h})` → `tui1bStep(submitDoneMsg{members: list})`。表头 `H(s)` = `"council 0.0.0-test   " + s`。
- `ALL`（`@` 之后、80x24 的整列，6 行）：
  ```
  > @alpha     Alpha
    @b1
    @beta      贝塔
    @bravo     Bravo
    @c1        C1
    共 8 个，上下键翻看
  ```
  （逐行：`"> @alpha     Alpha"`、`"  @b1"`、`"  @beta      贝塔"`、`"  @bravo     Bravo"`、`"  @c1        C1"`、`"  共 8 个，上下键翻看"`；id 列宽 8 = `grok-4.7`。）
- `B3`（`@b` 之后）：`"> @b1"`、`"  @beta   贝塔"`、`"  @bravo  Bravo"`（id 列宽 5）。

### 2.1 `internal/repl/atx_cand_test.go`

装置：`svc := newRoundStreamService(t)`；`svc.Create(ctx, "atx")`；按给定顺序 `svc.Join(ctx, conv.ID, conversation.Participant{AgentID: id, Provider: "claude", JoinedAt: time.Now().UTC()})`；要离开的 `svc.Leave(ctx, conv.ID, id, time.Now().UTC())`；`r := New(cfg, svc, nil, nil, &bytes.Buffer{}, RulesContext{})`（Runner 为 nil，`/use` 不需要它）。配置 `cfg(sum)` = `Agents`：`zeta`（`DisplayName: "Zeta 名"`）、`alpha`（v1.1：`"A" + string(rune(0x0a)) + "B" + string(rune(0x09)) + "C" + string(rune(0x7f)) + "D" + string(rune(0x9b)) + "E" + string(rune(0x1b))`，即 A、换行、B、tab、C、DEL、D、C1 控制字符 CSI、E、ESC）、`sum`（`"Sum"`）、`left`（`"Left"`）、`mid`（`"  "` 两个空格）；`Summary: config.SummaryConfig{Agent: sum}`。

**C1 `TestAtxCandidates`**（子测试或顺序断言，全部整值相等）：
1. `cfg("sum")`，按 `zeta, sum, gone, alpha, left, mid` 加入、`left` 离开。`/use` 之前 `r.MentionCandidates(ctx)` == `(nil, nil)`（切片为 nil）。
2. `r.Submit(ctx, "/use "+convID)` 之后 == `[{alpha, "A B C D E"}, {gone, ""}, {mid, ""}, {zeta, "Zeta 名"}]`、err nil。（`sum` 被去掉；`left` 已离开；`gone` 不在配置里、名字空；`mid` 只剩空白、名字空；`alpha` 的五个控制字符各换成一个空格、末尾那个被去掉；按 id 排序而不是加入顺序。）
3. 对照：`cfg("")`（没有总结成员），按 `zeta, sum` 加入并 `/use` → `[{sum, "Sum"}, {zeta, "Zeta 名"}]`。它是第 2 步「sum 不在」的正对照。
4. `cfg("sum")`，只有 `sum` 加入并 `/use` → 长度 0、err nil（只剩总结成员）。
5. `New(cfg("sum"), nil, nil, nil, nil, RulesContext{})`（依次是 svc nil、rn nil、in nil、out nil；out 由 `New` 兜底成 `io.Discard`）→ `(nil, nil)`。
6. 第 2 步那个 REPL 用已取消的 ctx 调 → 切片 nil、err 非 nil。

【判别力】第 2 步一条整值比较同时钉住：去总结成员（C1 变异）、只取活跃（C2）、显示名清洗（C4、C9；C10：判据换成只认 `r < 0x20` 时 DEL 与 CSI 原样留下，名字成了 `"A B C" + 0x7f + "D" + 0x9b + "E"`，v1 的显示名只有 0x20 以下的控制字符，C10 在 v1 上实测存活）。DEL、CSI 夹在字母中间而不是放在末尾，是为了每一个都单独看得见（放在末尾时，只要其中一个被 TrimSpace 当空白吃掉就看不出）。排序（C3）按 store 的约定等价，见 §4。第 1 步钉住「没有会话」那道门（C5，去掉门会空指针 panic）；第 6 步钉住读库错误原样返回（C8）。

**C2 `TestAtxCandHelpersMatchParser`**：对 rune 集合 {0x20..0x7e 全部 ASCII 可打印字符, 0x4e2d `中`, 0x3000, 0xff0c, 0xe9, 0x0a} 逐个断言：
- `MentionBoundaryOK(c) == (len(ParseMentions(string(c) + "@ab")) == 1)`；
- `MentionTokenRune(c) == (ParseMentions("@a" + string(c) + "b") 恰好一个元素且等于 strings.ToLower("a" + string(c) + "b"))`。

【判别力】拿真解析器当判据，左边界与字符类任何一处与 `ParseMentions` 分叉都红（C6：`中` 的低字节 0x2d 是 `-`；C7 同理）。

### 2.2 `internal/tui/atx_model_test.go`（纯 Model，不起 Program）

逐条冻结的步骤与期望（全部原型实跑）。「值」= `m.ta.Value()`；「行」= `m.atLines()`；「帧」= `m.frameHeight()`；「光标」= `m.View().Cursor`；「表头」= `m.chrome()[0]`。敲字符用 `tui1bType` / `tui1bKey`，Ctrl 键用 `tui1bCtrl`，方向键等用 `tea.KeyPressMsg{Code: tea.KeyUp}` 这种只带 Code 的键。

- **M1 `TestAtxOpenShowsList`**：新 Model(80, 24, atxMembers)。前提：行 nil、帧 8、光标 nil。敲 `@`：行 == `ALL`；`View().Content` 按 `\n` 切行、每行 `tui1bStripANSI` 再去掉行尾 ASCII 空格之后，第 9 行起（下标 8 起）整列 == `ALL`（列表在输入框下方）；帧 14；光标 == `(X 7, Y 2)`、`Shape == tea.CursorBar`、`Blink == true`、`Color == nil`；表头 `H("空闲")`；值 `"@"`。【判别力】前提是同装置的缺席对照；A1、A2、A3、A4、A5、A21、A23、A25、A46 在这里红。
- **M2 `TestAtxLeftBoundary`**（每个子测试一个新 Model(80, 24, atxMembers)，先依次喂「前置键」再敲 `@`）：开（行 == `ALL`）：`start`（无前置）、`space`（` `）、`paren`（`(`）、`han`（`中`）、`fullcomma`（`rune(0xff0c)`）、`newline`（`x` 再 Ctrl-J）；不开（行 nil、帧 8）：`letter`（`a`）、`upper`（`Z`）、`digit`（`9`）、`under`（`_`）、`dot`（`.`）、`dash`（`-`）、`at`（`@`，即 `@@`）。【判别力】开与不开同一装置互为对照；A12（左边界不查）让不开的七个全红；C6 让 `han` 红。
- **M3 `TestAtxFilter`**：新 Model(80, 24, atxMembers)，依次：敲 `@b` → 行 == `B3`、帧 11；再敲 `x` → 行 nil、帧 8、光标 nil；退格 → 行 == `B3`（补全中没丢）；敲 `R` → 行 == `["> @bravo  Bravo"]`。另起新 Model 敲 `@GROK-4.` → `["> @grok-4.7  Grok 4.7"]`；另起敲 `@a` → `["> @alpha  Alpha"]`；另起敲 `@c` → `["> @c1  C1", "  @c2", "  @c3"]`。【判别力】`@a` 钉住「前缀」不是「包含」（A44：`beta`、`bravo` 也含 a）；`R`、`GROK` 钉住小写折叠（A11）；`x` 之后的缺席与退格之后的出现互为对照；A17（default 分支不重判）红在第一步。
- **M4 `TestAtxUpDown`**：新 Model(80, 24, atxMembers)，敲 `@`：↑（Cmd 为 nil）→ 行 == `ALL`（到顶不动）；↓ 五次 → `["  @b1", "  @beta      贝塔", "  @bravo     Bravo", "  @c1        C1", "> @c2", "  共 8 个，上下键翻看"]`；再 ↓ 五次 → `["  @bravo     Bravo", "  @c1        C1", "  @c2", "  @c3", "> @grok-4.7  Grok 4.7", "  共 8 个，上下键翻看"]`；↑ 一次 → `["  @beta      贝塔", "  @bravo     Bravo", "  @c1        C1", "  @c2", "> @c3", "  共 8 个，上下键翻看"]`；↓ 一次；↑ 十次 → `ALL`；值仍 `"@"`。再 ↓ ↓ 之后敲 `b` → `B3`（前缀变了选中项归零）。对照（列表不在时上下键归 textarea）：另起新 Model 敲 `a`、Ctrl-J、`b`、↑、`z` → 值 `"az\nb"`。**前缀没变、选中项保留（v1.1）**：另起新 Model 敲 ` y`（空格与 y），← ←（光标回到行首），敲 `@`，↓ ↓ → 值 `"@ y"`、行 == `["  @alpha     Alpha", "  @b1", "> @beta      贝塔", "  @bravo     Bravo", "  @c1        C1", "  共 8 个，上下键翻看"]`（前提）；按 Delete（`tea.KeyPressMsg{Code: tea.KeyDelete}`，删光标右边的空格）→ 值 `"@y"`、行仍是同一整列（选中项还在 beta）。【判别力】↓ 到底再 ↑ 一次钉住「到底不越界」（A9：越界之后显示被夹住看不出，↑ 一次才露馅）；A8、A10、A24、A43、A47 红在对应那一步。Delete 那段与「↓ ↓ 之后敲 b」互为对照：一个前缀变了（选中项归零），一个前缀没变（`@` 与光标之间仍是空串，选中项留在 beta）；A50（前缀没变也归零）让 Delete 之后变成 `ALL`，v1 没有这段时 A50 实测存活。同类的键还有 End、行尾按 →，机制相同，只钉 Delete 一个。
- **M5 `TestAtxAccept`**：
  1. 两个子步骤各一个新 Model(80, 24, atxMembers)，分别用 Tab 与回车：敲 `hi @br` 再按键 → 把返回的 Cmd 用 `tui1bRun` 跑到底，产出里没有 `submitDoneMsg`（日志里没有 `submitDone`）；`sub.count() == 0`；值 `"hi @bravo "`；行 nil、帧 8、光标 nil；表头 `H("空闲")`；再敲 `z` → 值 `"hi @bravo z"`。
  2. 中间补全：敲 `x y`，← ←（光标在 `x` 之后），敲 ` @c`，↓，Tab，敲 `z` → 值 `"x @c2 z y"`。
  3. 多行与大小写：敲 `hi `，Ctrl-J，敲 `x @GR` → 光标 `(11, 3)`；回车；敲 `z` → 值 `"hi \nx @grok-4.7 z"`。
  4. 对照（补全之后回车真的提交）：敲 `@b`，↓，回车 → 值 `"@beta "`；再回车，`tui1bRun` 跑它的 Cmd → `sub.count() == 1`、`sub.text(0) == "@beta "`。
  【判别力】第 1 步「没提交」是缺席断言，对照是第 4 步同装置第二次回车真的提交（X44 教训：先跑 Cmd 再读计数）；「再打一个 z」钉住光标落点（A29 少空格、A28 少删 `@`、A30 不收起）；A37（回车不截）让回车那个子步骤红、A38（Tab 不截）让 Tab 那个子步骤红；A11 让第 3 步红。
- **M6 `TestAtxEsc`**：新 Model(80, 24, atxMembers)，敲 `@b`，Esc → Cmd nil、行 nil、值 `"@b"`、光标 nil；敲 `r` → 行 nil（Esc 收起后继续打字不重开）；敲 ` @` → 行 == `ALL`（新的 `@` 重开）。X44 提示：另起新 Model，敲 `a`、Ctrl-J、`/b`、回车 → 表头 == `H("一次只能一条命令，逐条回车；整段当消息发，首行只写 /say")`（X44 挡下）；敲 ` @` → 行非 nil（前提）；Esc → 表头仍是那句提示、值 `"a\n/b @"`、行 nil。【判别力】A7（列表开着时 Esc 不截）→ 第一步行不为 nil；提示那段是「列表开着时 Esc 不改状态栏」的正面钉。
- **M7 `TestAtxCloseOnContextLoss`**：新 Model(80, 24, atxMembers)：敲 `@b`，← → 行 == `ALL`（光标在 `@` 与 `b` 之间，前缀空）；再 ← → 行 nil；→ → → 行 nil（不重开）、值 `"@b"`、`m.ta.Line() == 0`、`m.ta.Column() == 2`（W1b 修订加值与光标：落点与 M3 第一步相同）。另起：敲 `@b `（空格）→ 行 nil；退格 → 行 nil、值 `"@b"`（空格把补全收掉了，删掉空格也不回来）。另起：敲 `@b`、退格 → 行 == `ALL`、值 `"@"`（对照，W1b 修订）；再退格 → 行 nil、值 `""`。另起：敲 `@b`、Ctrl-J → 行 nil；↑ → 行 nil。另起（换行）：敲 `@alp`、Esc、Ctrl-J、`@zz`（补全中、没有匹配）、↑ → 行 nil；敲 `z` → 值 `"@alzp\n@zz"`（↑ 走了 textarea）。另起（选区）：敲 `@b`，`tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}` → 行 nil。【判别力】A18（不查字符类）→ 空格那段退格之后重开；A19（不查行）→ 换行那段 ↑ 之后出现 `["> @alpha  Alpha"]`；A20（不查选区）→ 选区那段出现 `ALL`；A16（Ctrl-J 不重判）→ Ctrl-J 那段红；A40（`<=` 写成 `<`）→ 第二次 ← panic。（W1b 修订）「→ → → 行 nil」原来没有点名杀手：A51（不要求这个键敲出的是 `@`，光标一落到 `@` 后面就开）让它变成 `B3`，原型实测；它同装置的正对照是 M3 第一步（敲 `@b` → `B3`），值与光标都相同，只差来路。「退格两次 → 行 nil」的对照是只退格一次（`ALL`）。
- **M8 `TestAtxCtrlC`**：新 Model(80, 24, atxMembers)，敲 `@`，Ctrl-C → Cmd nil、行 nil、表头 `H("空闲")`、值 `"@"`；再 Ctrl-C → `tui1bRun` 的产出 `hasQuit()`、表头 `H("正在退出")`。【判别力】第二次就是对照（列表不在时 Ctrl-C 照旧退出）；A6 让第一次就退出。
- **M9 `TestAtxPasteCloses`**：新 Model(80, 24, atxMembers)，敲 `@`，`tea.PasteMsg{Content: "br"}` → 行 nil、值 `"@br"`、表头 `H("粘贴了 2 个字符")`；敲 `a` → 行 nil。另起：只喂 `tea.PasteMsg{Content: "@b"}` → 行 nil、帧 8、值 `"@b"`（W1b 修订加帧与值：值防止粘贴根本没进去时行 nil 空绿）。【判别力】A15（粘贴不收起）→ 第一步行 == `ALL`（前缀没重算）。「只喂粘贴 `@b` → 行 nil」是缺席断言，没有登记的变异杀它，它钉的是 D1「粘贴进来的 `@` 不开」；同装置的正对照是 M3 第一步（新 Model(80, 24, atxMembers) 敲 `@b` → `B3`、帧 11），两条只差敲与粘贴（W1b 修订；原型实跑两条都绿）。
- **M10 `TestAtxResize`**：新 Model(80, 24, atxMembers)，敲 `@`，依次喂 `WindowSizeMsg{80, h}`，每步 Cmd 为 nil、行与帧如下：h=15 → `ALL`、14；h=14 → `ALL` 前 5 行（没有提示行）、13；h=12 → 前 3 行、11；h=10 → 前 1 行、9；h=9 → nil、8；h=24 → `ALL`、14（补全没丢）；最后 `WindowSizeMsg{0, 0}` → 行 nil。【判别力】h=14 钉住提示行的「放得下才加」（A22）与 `avail` 的 -1（A45 让 h=14 多出一行）；h=24 那步是 h=9 缺席的对照。
- **M11 `TestAtxNoCandidates`**：两个子测试，候选分别是 nil 与长度 0 的切片：新 Model(80, 24, 候选)，敲 `@` → 行 nil、帧 8、光标 nil；回车，`tui1bRun` 跑 Cmd → `sub.count() == 1`、`sub.text(0) == "@"`。【判别力】正对照是 M1（同装置换夹具）；A39（在补全中就截键）让回车 panic（空列表取下标）。
- **M12 `TestAtxBusy`**：
  1. 新 Model(80, 24, atxMembers)，敲 `x`、回车（进忙）；敲 `@` → 行 nil；喂 `submitDoneMsg{members: atxMembers}` → 行 nil；敲 `b` → 行 nil；敲 ` @` → 行 == `ALL`（空闲后新敲的 `@` 才开）。
  2. 另起：敲 `@zz`（补全中、没有匹配）、回车（真的提交，进忙）；敲 `@` → 行 nil。
  3. 另起：敲 `x`、回车；喂 `roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 1, Speakers: []string{"rba_a"}}}`；喂 `submitDoneMsg{members: atxMembers}`；前提 `m.finalizing` 为真；敲 `@` → 行 nil；喂 `boardFinalizeMsg{}`；敲 ` @` → 行 == `ALL`。
  【判别力】A13 让第 1 步红；A31（`beginSubmit` 不收起）让第 2 步红；A14 让第 3 步红；每一步的「之后新敲 ` @` 就开」是同装置的正对照。
- **M13 `TestAtxSubmitCmdFetches`**（自己写一个 `atxSrcSub`：实现 `Submit`、`IsQuit`（恒 false）、`MentionCandidates`，加锁记下两个方法各自收到的 ctx，`Submit` 返回可配的 err，`MentionCandidates` 返回可配的列表与 err）：
  1. `root, cancel := context.WithCancel(context.Background())`；`New(Options{Submit: src, Out: tui1bNopDrainer{}, Version: "t", Ctx: root, Println: (&tui1bPrinter{}).cmd})`，`WindowSizeMsg{80, 24}`，敲 `x`，回车（保留 Cmd），**忙时 Ctrl-C**（取消本条命令的 ctx），再 `tui1bRun` 那条 Cmd：产出里的 `submitDoneMsg.members` == `[{zz, ""}]`；`MentionCandidates` 恰好被调 1 次、收到的 ctx `== root`（同一个值）；`Submit` 收到的 ctx 的 `Err()` 非 nil。把这条 `submitDoneMsg` 喂回 Model，敲 `@` → 行 == `["> @zz"]`。
  2. `Submit` 返回 `errors.New("x")`：产出的 `submitDoneMsg.members` 为 nil，`MentionCandidates` 没被调。
  3. `MentionCandidates` 返回 `(列表, errors.New("db"))`：`members` 为 nil。
  4. （v1.1）候选先非 nil、再被整份换成 nil：新 Model(80, 24, atxMembers)，敲 `@` → 行 == `ALL`（前提）；Esc；喂 `submitDoneMsg{members: nil}`（第 3 步那种取失败送回来的就是这条）；敲 ` @` → 值 `"@ @"`、行 nil、帧 8。**对照（W1b 修订）**：同一序列只把 `nil` 换成 `atxMembers`，敲 ` @` → 值 `"@ @"`、行 == `ALL`、帧 14。
  【判别力】A33（用 `runCtx` 取）→ 第 1 步 ctx 不等于 root；A34（出错也取）→ 第 2 步被调；A35（不看 err）→ 第 3 步不为 nil；A32（`submitDoneMsg` 不换候选）→ 第 1 步最后一句与几乎全部 M 用例红；A49（`members` 为 nil 时不换，旧名单一直留着）→ 第 4 步最后一句是 `ALL`，v1 没有第 4 步时 A49 实测存活。第 4 步「行 nil」是缺席断言，它的对照是只差 nil 的那一条，不是前面的「敲 `@` → `ALL`」：那个 `@` 在行首，第 4 步的 `@` 在空格之后，左边界判据只在后者上跑；「Esc 之后、喂过 `submitDoneMsg` 之后再敲 ` @` 能重开」只有对照能证明。原型实跑：忠实实现上两条都绿；A49 下第 4 步红（行 == `ALL`）、对照绿；演示变异（不登记，M2 `space` 本来就杀它）把左边界判据改成「不在行首一律不开」时，第 4 步照绿、对照红——没有对照，第 4 步会空绿（W1b 修订）。
- **M14 `TestAtxWidth`**：候选 `[{long, "一二三四五六七八九十一二三四五六七八九十"}, {x, ""}]`：宽 40 → 行 == `["> @long  一二三四五六七八九十一二三四五", "  @x"]`（首行显示宽 39：再放一个字就 41）；宽 10（兜底到 20）→ `["> @long  一二三四五", "  @x"]`；候选 `[{long, strings.Repeat("ab", 70)}]`、宽 200 → 首行 `ansi.StringWidth == 120`。【判别力】A26（不截）、A27（不封 120）。
- **M15 `TestAtxExitClearCountsList`**：新 Model(80, 24, atxMembers)，敲 `@`，`exitClearSeq(m, nil)` == `tui1dESC + "[13A\r" + tui1dESC + "[J"`（帧 14）。【判别力】A5（`frameHeight` 不数列表）→ `[7A`。对照：X-39e A 系列（没有列表时同一函数清 8 行）。

### 2.3 `internal/tui/atx_program_test.go`（真 `tea.Program`、真 REPL、真 Runner、假 CLI）

**装置（冻结）**：`g := rbaNewTuiRig(t)`（成员 `rba_a`、`rba_b`、`rba_j`，显示名都是 `X`，没有总结成员）。计数 Submitter（atx 自己的类型）：字段 `inner replAdapter`；`Submit` 在锁里记下文字再委托 `inner.Submit`；`IsQuit` 委托；**`MentionCandidates` 委托 `inner.MentionCandidates`**（经 `replAdapter`，不直接调 `g.r`，这样 C 那一个方法有担保）。spy（atx 自己的一份）：`Init`、`View` 委托；`Update` 先委托、断回 `Model` 存回、再记日志、**返回 spy 自己**；只记 `tea.KeyPressMsg`（种类 `KEY`）与 `submitDoneMsg`（`DONE`），每条同时记下 Update 之后的 `atLines()`、`ta.Value()`、`chrome()[0]`；日志加锁。`ctx, cancel := context.WithCancel(...)`；`m := New(Options{Submit: 计数, Out: g.lw, Version: "t", Ctx: ctx})`；`p := tea.NewProgram(spy, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignalHandler(), tea.WithWindowSize(100, 24))`；先 `g.r.SetRoundObserver(newRoundObserver(p.Send))`，再 `g.lw.SetEmit(func(line string) { p.Send(printLine(line)) })`，再 `go p.Run()`；`t.Cleanup` 里 cancel 并等 Run 返回（5 秒）。「发 k」= `p.Send(k)` 之后等日志条数加一（5 秒，前提）。

**P1 `TestAtxProgramCompletes`**：粘贴 `"/use " + g.convID` 并回车，等 1 条 `DONE`（10 秒，前提）。记下此刻日志长度 `b`。依次发：`tea.KeyPressMsg{Code: '@', Text: "@"}` → 日志第 `b` 条的行 == `["> @rba_a  X", "  @rba_b  X", "  @rba_j  X"]`；发 ↓、发回车 → 第 `b+2` 条：值 `"@rba_b "`、行 nil、表头 `"council t   空闲"`；发 `@`、发 Esc → 第 `b+4` 条：值 `"@rba_b @"`、行 nil。等 300ms：计数 Submitter 收到的文字整列 == `["/use " + g.convID]`、`DONE` 恰好 1 条。对照：再发回车 → 等 2 条 `DONE`（10 秒）→ 收到的文字整列 == `["/use " + g.convID, "@rba_b @"]`。

【判别力】这是跨并发原语的那一条：候选经真 `replAdapter` → 真 REPL → 真库，在 Submit 的 goroutine 上取回、经 `submitDoneMsg` 进事件循环；按键经真事件循环。A36（`replAdapter` 返回 nil）→ 第一句行为 nil；「回车没有提交」是缺席断言，对照是最后同装置那次回车真的提交；A7、A28、A29、A30、A37 也在这里红（§4）。

### 2.4 `internal/tui/atx_run_test.go`（真 `tui.Run`，`os.Pipe` 做 in 与 stdout）

**装置（冻结，形状照 `rba_wiring_test.go`）**：`g := rbaNewTuiRig(t)`；`inR, inW := os.Pipe()`、`outR, outW := os.Pipe()`；读 goroutine 在 Run 之前起（`io.Copy` 进 `&tui1dBuf{}`）；`go func() { runDone <- Run(ctx, g.r, g.lw, "test", inR, outW) }()`；收尾照 `rba_wiring_test.go:63-85`，挂进 `t.Cleanup`。「flat」= `rbaFlat(buf.String())`。

**R1 `TestAtxRunPipe`**：原始字节出现 `tui1dESC + "[J"`（5 秒）→ 写 `tui1dESC + "[8;24;100t"` → flat 含 `counciltest空闲`（5 秒）→ 写 `"/use " + g.convID + "\r"` → flat 含 `当前会话:`（5 秒）→ **（v1.1）flat 里第一个 `当前会话:` 之后出现 `counciltest空闲`**（5 秒，前提；`当前会话:` 是 `Submit` 里打的，那一刻还在忙，候选随之后的 `submitDoneMsg` 才到，不等这一步，`@` 可能抢在它前面进事件循环、忙时不开，主断言 1 假红）→ **缺席前提**：flat 不含 `>@rba_aX` → 写 `"@"` → flat 含 `>@rba_aX`（5 秒，**主断言 1**：真 Run 装上的 `replAdapter` 取到了候选）→ 写 `"\r"`、等 300ms → 写 `"hi\r"` → flat 含 `已记下seq2`（5 秒）→ 等 200ms → `g.svc.History(context.Background(), g.convID, 0, 100)` 里 `SenderID == conversation.ReservedSenderUser` 的消息正文整列 == `["seed-msg", "@rba_a hi"]`（**主断言 2**：第一次回车补全、没有提交 `@`，第二次回车提交的是补全后的整句）。

【为什么是子串】终端字节流，同 RB-A W1。【v1.1 那一步的来历】原型上 v1 的 R1 在一次变异跑里假红过一次，症状就是 `@` 落在忙的 Model 上；把 `replAdapter.MentionCandidates` 临时拖慢 200ms（只在原型里）后 v1 的 R1 必红、加了这一步的 R1 绿；未改动时 v1 的 R1 连跑 80 次零红。按代码读，回车之后表头的「空闲」只由 `submitDoneMsg` 那一支写回（HEAD `model.go:326`、`:330`；另一处是 `New` 的初值），`当前会话:` 插进回滚区时随之重画的表头还是「正在运行」（假红那次的字节里看得到），所以这一步成立时候选已经到了。P1、V1 按 `submitDoneMsg` 同步，没有这个问题。**W1b 独立复测**（它自己的原型、忠实实现；W1b 修订照引、没有重跑）：不加延迟时 v1 形状 60 次红 1 次——那一次另一条流水线的 `go test` 正在并发跑，正是 §6 第 2 步预期的环境；列表没出来，W1b 判定是 `@` 在 `submitDoneMsg` 之前进了事件循环——v1.1 形状 60/60 绿；`MentionCandidates` 临时拖慢 200ms 时 v1 形状 5/5 红、v1.1 形状 5/5 绿。所以上面的「80 次零红」不说明撞不上：有并发负载时撞得上。【判别力】A36 → 主断言 1 等不到；回车不截（A37）→ 第一次回车提交了 `@`，主断言 2 的用户消息多出一条；A28、A29 → 主断言 2 的正文不对。只看用户消息，不看 agent 消息：CH1 若把 REPL 零值改成自动回复，这里仍成立（§7 (i)）。

### 2.5 `internal/tui/atx_vt_test.go`（VT 回放守卫）

**装置（冻结）**：atx 自己的假 Submitter：`Submit` 用 `p.Send(printLine("ATXEND"))` 打一行、返回 nil；`IsQuit` 恒 false；`MentionCandidates` 返回 `atxMembers` 的拷贝。spy 同 P1 的形状，只数 `submitDoneMsg`。录制器 `rbbRecorder`；`New(Options{Submit: 假件, Out: tui1bNopDrainer{}, Version: "t", Ctx: ctx})`；`tea.NewProgram(spy, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(录制器), tea.WithoutSignalHandler(), tea.WithWindowSize(80, 24))`。一次跑：等录制器有第 1 块（5 秒）→ 粘贴 `"/use x"`、回车 → 等 1 条 `DONE`（5 秒）→ 睡 100ms → 对序列里每个键：记下录制器块数 n、`p.Send`、`rbbWaitUntil(150ms, 块数 > n)`（等不到不算错）→ 睡 150ms → 非退出场景 cancel 并等 Run 返回；退出场景等 Run 自己返回（5 秒）。回放：`rbbNewVT(80, 24)`，先喂 `S01`…`S30` 各一行（`"S%02d\r\n"`），再按序喂全部块；退出场景之后再喂 `exitClearSeq(spy 里的 Model, Run 的 err)`。读数：`all()` 里最后一个等于 `ATXEND` 的下标 s、最后一个以 `council t   ` 开头的下标 h，残影 = h - s - 1；以 `council t   ` 开头的行数；`unknown` 条数；「画过」= 原始块里有含 `@alpha` 的块；「值」= spy 里 Model 的 `ta.Value()`；退出场景的「干净」= `all()` 里没有以 `council t` 或 `─` 开头的行、也没有含 `@alpha` 的行。

**V1 `TestAtxVTNoResidue`**，七个子测试，每个跑 5 次，每次：前提 画过 为真且 值 等于下表（`Fatalf`）；残影 0、表头行 1、unknown 0（非退出场景）或 干净（退出场景）。键记号：`k(r)` = `tea.KeyPressMsg{Code: r, Text: string(r)}`；Esc、↓、Tab、回车、退格为只带 Code 的键；Ctrl-J、Ctrl-C 为 `tea.KeyPressMsg{Code: 'j' / 'c', Mod: tea.ModCtrl}`。

| 子测试 | 键序列 | 结束时的值 |
|---|---|---|
| `open_esc` | `k('@')`、Esc | `"@"` |
| `down_tab` | `k('@')`、↓、Tab | `"@b1 "` |
| `narrow` | `k('@')`、`k('b')`、`k('r')`、`k('x')`、退格、Esc | `"@br"` |
| `backspace` | `k('@')`、`k('a')`、退格、退格 | `""` |
| `multiline` | `k('x')`、Ctrl-J、`k('y')`、Ctrl-J、`k('z')`、Ctrl-J、`k('@')`、↓、Esc | `"x\ny\nz\n@"` |
| `enter_accept` | `k('@')`、`k('b')`、回车 | `"@b1 "` |
| `exit`（退出场景） | `k('@')`、Ctrl-C、Ctrl-C | （不比较） |

【判别力】A1（不钉光标）→ 七个子测试全红（§0 第 3 条的探针在同样的键序列上读到残影 6/6/7/11/6/6、`exit` 0/5 干净）；V1 守的是**钉光标**。放上方（A4）V1 杀不到——A4 只挪列表、光标仍钉在第 2 行起，公式为 0；放法由 M1 的「View 第 9 行起」整列守（§4）。「画过」前提防止列表根本没出现时残影为 0 的空绿；「值」前提防止键序列没按预期生效。A17、A28、A29、A37、A38 也会让个别子测试的「值」前提红（§4）。原型实测正确实现 `-count=3` 零偶发；每个子测试约 1.5 秒。**升级 bubbletea 或 ultraviolet 时重跑**（与 RB-B 守卫同一条）。Linux 上的 `mapNL` 由 `rbbNewVT` 已有的处理覆盖，本刀没在 Linux 上跑过（§7 (h)）。

---

## 3. 冻结的字面量

### 3.1 本刀新增的用户可见字面量

- 提示行格式串：`  共 %d 个，上下键翻看`（前面两个 ASCII 空格；`%d` 是匹配总数；`，` 是 U+FF0C；行尾无空格无句号）。UTF-8 逐字节：`20 20 e5 85 b1 20 25 64 20 e4 b8 aa ef bc 8c e4 b8 8a e4 b8 8b e9 94 ae e7 bf bb e7 9c 8b`。例：`  共 8 个，上下键翻看`（29 字节，显示宽 21）。
- 选中标记 `> `（`>` 加一个空格）、未选中 `  `（两个空格）、`@`、id 与显示名之间两个空格。
- 光标：`tea.CursorBar`、闪烁（终端收到 `ESC[5 q`）。

### 3.2 测试要比对的既有文本（不是本刀新增，HEAD 原样）

`空闲`、`正在退出`、`粘贴了 %d 个字符`（`model.go:378`）、`一次只能一条命令，逐条回车；整段当消息发，首行只写 /say`（`model.go:466`）、`当前会话:`、`已记下 seq %d`、`council <版本>   <状态>` 的表头形状、`exitClearSeq` 的 `ESC[<h-1>A\rESC[J`。

---

## 4. 集成员的变异清单

「预期被谁杀」是原型实跑时变红的用例（至少这些）；主代理落地后逐条跑（锚点恰好一次、从字节副本还原，不用 `git checkout`）。记号：`<NL>` 是换行，`<TAB>` 是一个 tab；「→」左边是原文（逐字，含行首 tab），右边是变异后；每条都能编译。跑法：`model.go` / `run.go` 的变异跑 `go test -count=1 -v -run TestAtx ./internal/tui`，`mentioncand.go` 的跑 `./internal/repl`。v1 原型上 53 条全部被点名杀手杀掉，4 条等价（表后）。v1.1 加 A49、A50、C10 三条（W1 F2），共 56 条；在 `at1_v11` 原型上、对 v1.1 的测试整表重跑（锚点恰好一次、从字节副本还原、还原后 SHA256 相等）：56 条的点名杀手全部实测变红（点名到子测试的逐个核对；A39、A40、C5 是 panic，整个测试二进制中止，点名的杀手照样红），4 条等价全部存活。新三条在 v1 的测试上实测存活（换回 v1 的 M4、M13、C1 重跑；R1 用 v1.1 那一版，避开 §2.4 说的竞态），说明杀它们的正是 v1.1 新加的那几步。W1b 修订加 A51 一条（共 57 条）：在 `at1_fix` 原型上实测被 M7 的「→ → → 行 nil」那一步杀掉（锚点恰好一次、从字节副本还原、还原后 SHA256 相等）；那一步 v1.1 就有，A51 只是把它的杀手登记下来。本轮没有整表重跑。

| # | 文件 | 变异 | 预期被谁杀 |
|---|---|---|---|
| A1 | model.go（B7） | `<TAB>} else if len(pop) > 0 {<NL><TAB><TAB>v.Cursor = m.atCursor()<NL><TAB>}` → `<TAB>}`（不钉光标） | 原型 A1 下 **V1 七个子测试全红**（点名杀手；残影数字见 §0 第 3 条的探针表，探针夹具不同、列表行数相同）；M1、M5 |
| A2 | model.go（B7） | 删掉整行 `<TAB>c.Y += len(m.chrome())<NL>` | M1、M5（光标 Y）。V1 杀不到：钉在更上面的行，公式仍为 0 |
| A3 | model.go（B7） | `<TAB>c.Shape = tea.CursorBar` → `<TAB>c.Shape = tea.CursorBlock` | M1 |
| A4 | model.go（B7） | `<TAB>parts := append(append(m.chrome(), m.ta.View()), pop...)` → `<TAB>parts := append(append(m.chrome(), pop...), m.ta.View())`（放上方） | **M1**（View 第 9 行起的整列）。V1 杀不到：光标仍钉在第 2 行起，公式为 0——放法由 M1 守，钉光标由 V1 守 |
| A5 | model.go（B8） | `<TAB>return len(m.chrome()) + m.ta.Height() + len(m.atLines())` → `<TAB>return len(m.chrome()) + m.ta.Height()` | **M15**；M1、M3、M10 |
| A6 | model.go（B4） | 删掉 `<TAB><TAB><TAB>case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':<NL><TAB><TAB><TAB><TAB>m.at = atState{}<NL><TAB><TAB><TAB><TAB>return m, nil<NL>` | M8 |
| A7 | model.go（B4） | 删掉 `<TAB><TAB><TAB>case msg.Code == tea.KeyEscape:<NL><TAB><TAB><TAB><TAB>m.at = atState{}<NL><TAB><TAB><TAB><TAB>return m, nil<NL>` | M6、P1 |
| A8 | model.go（B4） | `<TAB><TAB><TAB><TAB>if m.at.sel > 0 {` → `<TAB><TAB><TAB><TAB>if true {` | M4 |
| A9 | model.go（B4） | `<TAB><TAB><TAB><TAB>if m.at.sel < len(m.atMatches())-1 {` → `<TAB><TAB><TAB><TAB>if true {` | M4（↓ 到底再 ↑ 一次那一步） |
| A10 | model.go（B8） | `<TAB><TAB>m.at.prefix = p<NL><TAB><TAB>m.at.sel = 0<NL>` → `<TAB><TAB>m.at.prefix = p<NL>` | M4（`b` 之后） |
| A11 | model.go（B8） | `<TAB>p := strings.ToLower(m.at.prefix)` → `<TAB>p := m.at.prefix` | M3、M5 |
| A12 | model.go（B8） | `<TAB>if col >= 2 && !repl.MentionBoundaryOK(line[col-2]) {` → `<TAB>if false {` | M2 的七个「不开」子测试 |
| A13 | model.go（B8） | `<TAB>if m.busy \|\| m.finalizing \|\| m.ta.HasSelection() {` → `<TAB>if m.finalizing \|\| m.ta.HasSelection() {` | M12 |
| A14 | model.go（B8） | 同一行 → `<TAB>if m.busy \|\| m.ta.HasSelection() {` | M12（第 3 步） |
| A15 | model.go（B6） | `PasteMsg` 分支里删掉 `<TAB><TAB>m.at = atState{}<NL>`（即 `…utf8.RuneCountInString(msg.Content))<NL><TAB><TAB>m.at = atState{}<NL>` → `…utf8.RuneCountInString(msg.Content))<NL>`） | M9 |
| A16 | model.go（B5） | `<TAB><TAB><TAB>m.ta.InsertRune('\n')<NL><TAB><TAB><TAB>m = m.atRecheck()<NL>` → `<TAB><TAB><TAB>m.ta.InsertRune('\n')<NL>` | M7 |
| A17 | model.go（B6） | `<TAB><TAB><TAB>m.ta, cmd = m.ta.Update(msg)<NL><TAB><TAB><TAB>m = m.atRecheck()<NL>` → `<TAB><TAB><TAB>m.ta, cmd = m.ta.Update(msg)<NL>` | M2 `at`、M3、M4、M5、M7、M12、V1 `enter_accept` |
| A18 | model.go（B8） | `<TAB><TAB>if !repl.MentionTokenRune(r) {` → `<TAB><TAB>if r < 0 {` | M7（空格之后退格） |
| A19 | model.go（B8） | `<TAB>if m.ta.HasSelection() \|\| m.ta.Line() != m.at.row {` → `<TAB>if m.ta.HasSelection() {` | M7（换行那段） |
| A20 | model.go（B8） | 同一行 → `<TAB>if m.ta.Line() != m.at.row {` | M7（选区那段） |
| A21 | model.go（B8） | `<TAB>show := min(len(list), atMaxRows, avail)` → `<TAB>show := min(len(list), atMaxRows+1, avail)` | M1、M2、M4、M6、M7、M10 |
| A22 | model.go（B8） | `<TAB>if len(list) > show && avail > show {` → `<TAB>if len(list) > show {` | M10（h=14） |
| A23 | model.go（B8） | 同一行 → `<TAB>if false {` | M1、M2、M4、M6、M7、M10 |
| A24 | model.go（B8） | `<TAB>top := max(0, sel-show+1)` → `<TAB>top := 0` | M4 |
| A25 | model.go（B8） | `<TAB><TAB>line := mark + "@" + list[i].ID<NL><TAB><TAB>if list[i].Name != "" {<NL><TAB><TAB><TAB>line = mark + "@" + padCells(list[i].ID, idW) + "  " + list[i].Name<NL><TAB><TAB>}` → `<TAB><TAB>line := mark + "@" + padCells(list[i].ID, idW) + "  " + list[i].Name`（没有显示名也补空格） | M1、M2、M3、M4、M6、M7 |
| A26 | model.go（B8） | `<TAB><TAB>lines = append(lines, ansi.Truncate(line, width, ""))` → `<TAB><TAB>lines = append(lines, line)` | M14 |
| A27 | model.go（B8） | `<TAB>width := min(max(m.width, 20), 120)` → `<TAB>width := max(m.width, 20)` | M14（宽 200） |
| A28 | model.go（B8） | `<TAB>n := 1 + len([]rune(m.at.prefix))` → `<TAB>n := len([]rune(m.at.prefix))` | M5、P1、R1、V1 `down_tab`、`enter_accept` |
| A29 | model.go（B8） | `<TAB>m.ta.InsertString("@" + pick.ID + " ")` → `<TAB>m.ta.InsertString("@" + pick.ID)` | M5、P1、R1、V1 `down_tab`、`enter_accept` |
| A30 | model.go（B8） | `<TAB>m.ta.InsertString("@" + pick.ID + " ")<NL><TAB>m.at = atState{}<NL>` → `<TAB>m.ta.InsertString("@" + pick.ID + " ")<NL>` | M5、P1 |
| A31 | model.go（B9） | `<TAB>m.ta.Reset()<NL><TAB>m.at = atState{}<NL>` → `<TAB>m.ta.Reset()<NL>` | M12（第 2 步） |
| A32 | model.go（B3） | 删掉 `<TAB><TAB>m.members = msg.members<NL>` | 几乎全部 M 用例（M1 起；M5 等 panic） |
| A33 | model.go（B11） | `<TAB><TAB><TAB>if list, lerr := src.MentionCandidates(listCtx); lerr == nil {` → `…src.MentionCandidates(ctx)…` | M13（第 1 步 ctx 不等于 root） |
| A34 | model.go（B11） | `<TAB><TAB><TAB>return submitDoneMsg{err: err}<NL>` → `<TAB><TAB><TAB>if src, ok := sub.(mentionSource); ok {<NL><TAB><TAB><TAB><TAB>_, _ = src.MentionCandidates(listCtx)<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>return submitDoneMsg{err: err}<NL>`（出错也取） | M13（第 2 步） |
| A35 | model.go（B11） | `<TAB><TAB><TAB>if list, lerr := src.MentionCandidates(listCtx); lerr == nil {` → `<TAB><TAB><TAB>if list, _ := src.MentionCandidates(listCtx); true {` | M13（第 3 步） |
| A36 | run.go（C） | `<TAB>return a.r.MentionCandidates(ctx)` → `<TAB>return nil, nil` | **P1、R1**（只有它们经过真 `replAdapter`） |
| A37 | model.go（B4） | `<TAB><TAB><TAB>case msg.Code == tea.KeyTab, msg.Code == tea.KeyEnter:` → `<TAB><TAB><TAB>case msg.Code == tea.KeyTab:`（回车不截，照旧提交） | M5（回车那个子步骤）、P1、R1、V1 `enter_accept` |
| A38 | model.go（B4） | 同一行 → `<TAB><TAB><TAB>case msg.Code == tea.KeyEnter:`（Tab 不截） | M5（Tab 那个子步骤）、V1 `down_tab` |
| A39 | model.go（B4） | `<TAB><TAB>if m.atVisible() {` → `<TAB><TAB>if m.at.armed {` | M11（空列表时回车 panic） |
| A40 | model.go（B8） | `<TAB>if col <= m.at.col \|\| …` → `<TAB>if col < m.at.col \|\| …`（其余不变） | M7（第二次 ← panic） |
| A43 | model.go（B4） | `<TAB><TAB><TAB><TAB><TAB>m.at.sel--` → `<TAB><TAB><TAB><TAB><TAB>m.at.sel++` | M4 |
| A44 | model.go（B8） | `<TAB><TAB>if strings.HasPrefix(c.ID, p) {` → `<TAB><TAB>if strings.Contains(c.ID, p) {` | M3（`@a`） |
| A45 | model.go（B8） | `<TAB>avail := m.termHeight - 1 - (len(m.chrome()) + m.ta.Height())` → `<TAB>avail := m.termHeight - (len(m.chrome()) + m.ta.Height())` | M10 |
| A46 | model.go（B8） | 提示行的 `len(list)), width, ""))` → `len(list)-show), width, ""))` | M1、M2、M4、M6、M7、M10 |
| A47 | model.go（B8） | `<TAB><TAB>if i == sel {` → `<TAB><TAB>if i == top {` | M4 |
| C1 | mentioncand.go | `<TAB><TAB>if sumID != "" && p.AgentID == sumID {` → `<TAB><TAB>if sumID == "-" && p.AgentID == sumID {` | C1（第 2 步） |
| C2 | mentioncand.go | `<TAB>parts, err := r.svc.Participants(ctx, r.cur.ID, false)` → `…r.cur.ID, true)` | C1（`left` 出现） |
| C4 | mentioncand.go | `<TAB><TAB><TAB>name = mentionName(ac.DisplayName)` → `<TAB><TAB><TAB>name = ac.DisplayName` | C1 |
| C5 | mentioncand.go | `<TAB>if r.svc == nil \|\| r.cur == nil {` → `<TAB>if r.svc == nil {` | C1（第 1 步 panic） |
| C6 | mentioncand.go | `<TAB>return prev >= 0x80 \|\| !mentionLeftForbidden(byte(prev))` → `<TAB>return !mentionLeftForbidden(byte(prev))` | C2 |
| C7 | mentioncand.go | `<TAB>return r < 0x80 && mentionTokenChar(byte(r))` → `<TAB>return mentionTokenChar(byte(r))` | C2 |
| C8 | mentioncand.go | `<TAB><TAB>return nil, err<NL><TAB>}<NL><TAB>sumID` → `<TAB><TAB>return nil, nil<NL><TAB>}<NL><TAB>sumID` | C1（第 6 步） |
| C9 | mentioncand.go | `<TAB>return strings.TrimSpace(strings.Map(func(r rune) rune {` → `<TAB>return (strings.Map(func(r rune) rune {` | C1（`mid` 的名字成了两个空格） |
| A49 | model.go（B3） | `<TAB><TAB>m.members = msg.members<NL>` → `<TAB><TAB>if msg.members != nil {<NL><TAB><TAB><TAB>m.members = msg.members<NL><TAB><TAB>}<NL>`（v1.1；W1 的 X22：候选为 nil 时不整份替换） | M13（第 4 步） |
| A50 | model.go（B8） | `<TAB>if p := string(line[m.at.col+1 : col]); p != m.at.prefix {` → `<TAB>if p := string(line[m.at.col+1 : col]); true {`（v1.1；W1 的 X31：前缀没变也把选中项归零） | M4（Delete 那段） |
| C10 | mentioncand.go | `<TAB><TAB>if unicode.IsControl(r) {` → `<TAB><TAB>if r < 0x20 && unicode.IsControl(r) {`（v1.1；等于只认 `r < 0x20`，留着 `unicode` 的引用好让它能编译） | C1（第 2 步 `alpha` 的名字） |
| A51 | model.go（B6） | `<TAB><TAB><TAB>if !m.at.armed && msg.Text == "@" {` → `<TAB><TAB><TAB>if !m.at.armed {`（W1b 修订；不要求这个键敲出的是 `@`，光标一落到 `@` 后面就开，违反 D1「只有敲出来的 `@` 能开」） | M7（「→ → → 行 nil」那一步，变成 `B3`） |

（表里的 `\|` 是 markdown 转义，源码里就是 `||`。编号 A41、A42、A48、C3 在下面。A49、A50、C10 是 v1.1 新加的，编号接着 v1 往下排；A51 是 W1b 修订加的。）

**判定为等价、不列杀手的变异**（原型实测存活，按代码论证）：
- A41 `atRecheck` 里 `<TAB>if m.at.col > 0 && !repl.MentionBoundaryOK(line[m.at.col-1]) {` → `<TAB>if false {`：补全中 `@` 左边那个字改不了——要改它光标得先挪到 `@` 左边（第一条判据已收起），粘贴一律收起，删除只删光标左边。按可达性等价，防御性保留。
- A42 `<TAB><TAB><TAB>if !m.at.armed && msg.Text == "@" {` → `<TAB><TAB><TAB>if msg.Text == "@" {`：补全中再敲 `@`，重判时 `@` 不是 token 字符，先收起了，`!armed` 恒真。
- A48 `<TAB>if !m.at.armed \|\| m.termHeight <= 0 {` → `<TAB>if !m.at.armed {`：`termHeight <= 0` 时 `avail` 必为负，`show < 1` 那一句同样返回 nil。保留是为了与 `boardLimit()` 同一个写法、读起来直白。
- C3 删掉 `sort.Slice(…)` 那一行（`<TAB>_ = sort.Slice`）：真 store 的 `Participants` 按约定带 `ORDER BY agent_id ASC`（`store/read.go:63`；函数在 `:57`，`:56` 的注释「结果按 agent_id 升序」；`conversation.Service.Participants` 原样转发），`MentionCandidates` 里的 `sort.Slice` 与它重复，属于防御。这是按 store 的约定等价，不是按实测；C1 的整值比较看不出，**只在审查时看**。

**只在审查时看（没有用例能抓，集成员收割时逐字核对）**：
- `MentionBoundaryOK`、`MentionTokenRune` 委托 `mention.go` 的 `mentionLeftForbidden`、`mentionTokenChar`，不是另写一份字符表（另写一份逐字相同时 C2 全绿，分叉的那一天才会红）。
- `model.go` 调的是 `repl.MentionBoundaryOK`、`repl.MentionTokenRune`，不是在 `tui` 里另写判据。
- `atCursor` 只动 textarea 的**副本**（`ta := m.ta`）；`c == nil` 那一支在 textarea 聚焦时到不了（`New` 里就 `Focus()` 了，之后没有 `Blur`）。
- 取候选在 `submitCmd` 的同一个 goroutine 上、`Submit` 之后，没有 `go func`、没有 channel；`mentionSource` 没并进 `Submitter`（并进去的话既有假件编译不过，全量 `./...` 会红）。
- 三段注释与 §1.1 逐字相同，没有与代码不符的话。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- **只许动这三个文件**：新建 `internal/repl/mentioncand.go`；修改 `internal/tui/model.go`、`internal/tui/run.go`。不写任何 `_test.go`，不改别的文件（`mention.go`、`seam.go`、`commands.go`、`roundobs.go`、`go.mod`、`go.sum` 都不许动）。
- 生产改动约 260 行，**一次交付**（不拆块；约 400 行以内）。
- 新文件用：

  ```
  ===FILE: internal/repl/mentioncand.go===
  （完整内容）
  ===END FILE===
  ```
- 已有文件用补丁块，每个补丁一个块，`model.go` 十一个（B1-B11，按文件里的先后顺序）、`run.go` 一个（C）：

  ```
  ===PATCH: internal/tui/model.go===
  <<<<ANCHOR
  （从 HEAD 真文件里原样复制的若干行，含行首 tab，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点就是 §1.1 B、C 给出的锚点块（派工时的 main HEAD；写本文时是 `3d134ee`），替换文本就是对应的替换块。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- §1.1 A 的整个文件、B1-B11 与 C 的替换块**逐字照抄**（含注释、空行与 gofmt 的缩进）。代码必须能 `gofmt`。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的字符串字面量只有 §3.1 那一条格式串与 `"> "`、`"  "`、`"@"`、`" "`、`"\n"`、`""` 这几个 ASCII 片段；补全列表里不许用 `…`（U+2026，模糊宽度）或任何 `●○◐×▶` 之类的符号。
- 生产代码的新标识符只有：`MentionCandidate`（字段 `ID`、`Name`）、`MentionCandidates`、`MentionBoundaryOK`、`MentionTokenRune`、`mentionName`、`mentionSource`、`atState`（字段 `armed`、`row`、`col`、`prefix`、`sel`）、`atMaxRows`、`Model` 的字段 `members`、`at`、`submitDoneMsg` 的字段 `members`、方法 `atCursor`、`atArm`、`atRecheck`、`atLine`、`atMatches`、`atVisible`、`atLines`、`atAccept`，全部冻结；不许以 `atx` 开头。`submitCmd` 的签名改为 `(sub Submitter, ctx, listCtx context.Context, text string)`。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：三个文件是否都给了；§1.1 A、B1-B11、C 各落在哪个块。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3.1 的字面量是否逐字照抄；有没有新增任何别的字符串字面量。
  ```

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；十二个锚点各恰好一次后落盘；`gofmt -l internal/repl/mentioncand.go internal/tui/model.go internal/tui/run.go` 为空；`go vet ./internal/repl ./internal/tui`；`go build ./...`；`git diff --stat` 只有这三个文件；逐字节核对 §1.1 A、B、C 与 §3.1（十六进制比对提示格式串）。
2. **合并测试腿的五个文件**。只跑 `./internal/repl ./internal/tui`（另一条流水线也可能在跑 go test；3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制），再跑一次全量 `./...`。既有用例必须全绿（原型实测全绿、零改动）。P1、R1、V1 各 `-count=3`。
3. **§4 的变异逐条跑**（等价清单里的不跑；从字节副本还原，不用 `git checkout`）。任何一条没被点名杀手杀掉，先判是用例空、还是契约写错；契约错就改契约再改任务书。
4. **文档**：
   - `docs/00_DECISIONS.md` #47「补充」那一段后面补「AT1 落地记录」：放法与钉光标的实测（§0 第 3 条的表；「另测」表里「窗口变尺寸」两行照抄 §0 的限定：只给 Program 发 `WindowSizeMsg`、模拟器不缩，真终端缩放没测，以真机步骤 8 为准，不许记成「窗口变尺寸不留残影」）、候选取法（提交之后同一 goroutine，§0 第 4 条）、按键表（D7）、忙时不弹、只按 id 前缀筛的原因、已知代价（`ESC[5 q` / `ESC[0 q` 复位用户的光标形状；列表开着时第一次 Ctrl-C 只收起；粘贴进来的 `@` 不开补全；忙时敲的 `@` 要回到空闲后重敲；v1.1 加：句末是点名、已经敲完整个 id 时，列表仍开着（至少 1 个匹配：敲完的 id 若是别的 id 的开头部分，例如 `opus-5.0` 与 `opus-5.0-max`，列表有两行；敲完的那个按字节序排第一、默认选中，补出来的就是它），第一次回车只补一个空格，第二次才提交；列表开着时 `@` 后面那一格可能同时看到 textarea 的反白方块与钉住的闪烁竖线）。
   - 本契约归档进 `docs/`（按现有编号体例取名，例如 `18c_AT1_MENTION_COMPLETION_CONTRACT.md`，避开 CH1 的归档名；文件头加归档说明，写明行号指 HEAD `3d134ee`）。
   - `docs/15_TUI_SPEC_MAPPING.md` 若有「输入框」一节，补一句指向本契约。
5. **给 Griffin 的真机复验步骤**（Windows Terminal + PowerShell 5.1；每条 council 内命令单独一行、单独一块，逐条回车；不要整块粘贴）：
   1. 启动 TUI，选一个有两个以上成员的会话（把 `<会话id>` 换成真的）：

      ```
      /use <会话id>
      ```
   2. 在输入框里敲一个 `@`（不回车）。预期：输入框**下方**出现成员列表，第一行前面是 `> `，每行 `@<id>` 加显示名；总结成员不在里面；一个竖线形状的光标停在 `@` 后面。
   3. 按 ↓ 两次、↑ 一次：`> ` 跟着移动。按回车：输入框里变成 `@<那个 id> `（后面带一个空格），列表消失，**没有**回显、没有提交。接着敲 `你好`，再回车：这一次才提交（照你的会话配置回复或记下）。
   4. （v1.1）句末点名：敲 `你怎么看 @` 再把会话里某个成员的 id **完整敲完**（例如 `grok-4.7`；敲完的是 `opus-5.0` 或 `fable-5.1` 时列表还会多一行 `-max` 那个，`> ` 仍在你敲完的那个上，结果一样；W1b 修订），直接按回车：预期输入框变成 `你怎么看 @<id> `（多了一个空格），列表消失，**没有**提交。再按一次回车才提交。请说一下这一下是否别扭（§7 (g)）。
   5. 再敲 `@` 加 id 的前两个字母（大写也行）：列表只剩以它开头的成员。继续敲一个不存在的字母：列表消失；退格：列表回来。按 Esc：列表消失，输入框里的字不变；继续打字列表不再出现。
   6. 敲一个 `@`，按 Ctrl-C：只是列表消失，程序不退出。
   7. 残影检查：把上面几步做完之后，往上滚动窗口，看回滚区里有没有多出来的 `council …` 表头行、横线或 `@…` 列表行（应当没有，只有正常的命令回显）。
   8. 窄窗口：把窗口拉到只有十来行高，敲 `@`：列表只显示放得下的几行（高到 15 行左右才会出现最后一行「共 N 个，上下键翻看」）；拉到 9 行以下列表不出现。再拉回原高度，列表回来。（W1b 修订）接着按 Esc，往上滚动看回滚区有没有多出来的 `council …` 表头行、横线或 `@…` 列表行：§0 表里「窗口变尺寸」两行只在模拟器里量过（模拟器不缩），真终端缩放只有这一步看。要是有，不开列表、同样拉一次窗口作对照，看是不是不开列表也有（那就不是本刀带来的）。
   9. 忙时（会花一次调用的钱；不想花钱就跳过）：发一句让成员回复的话，跑的过程中敲 `@`：列表**不**出现（这是本刀的设计）；这一轮结束之后再敲一个 `@` 才出现。
   10. 光标形状：列表出现时光标变成闪烁竖线，列表消失后光标恢复成终端的默认形状。**如果你在 Windows Terminal 里自己设过光标形状（比如方块），列表关掉后它会被复位成默认——请告诉我看到的是什么。** （v1.1）另外，列表开着时 `@` 后面那一格可能同时看到一个反白的方块（输入框自己画的光标）和一根闪烁的竖线（钉住的真光标），请说一下观感：两个都在是否碍眼（§7 (a)）。
6. 清理本刀在中转根与 scratchpad 里的全部中间物。

---

## 7. open items

### 待 Griffin

- (a) **光标形状**：列表开着时钉的是闪烁竖线（`ESC[5 q`），关掉时发 `ESC[0 q` 恢复终端默认；若你自定义过光标形状会被复位（与 RB-B 收尾窗口同一种代价，真终端没测）。要换成方块或下划线只改 B7 的一个常量。（v1.1 并进来）列表开着时屏幕上有两个光标：输入框自己画的反白方块照旧在画（`atCursor` 只在 textarea 的副本上关掉虚拟光标，Model 里的那个不动），钉住的闪烁竖线落在同一格。原型实测：敲 `@` 之后 `View().Content` 第 3 行 `@` 后面那一格是 `ESC[7m` 反白，`View().Cursor` 是 (7, 2)、竖线、闪烁；Esc 之后反白还在、真光标藏起来。想只留一个，要在列表开着时让 Model 自己的 textarea 也关掉虚拟光标、收起时恢复，这会改帧内容，要重测 V1；等你看过真机再定。
- (b) **忙时不弹**：一轮在跑时敲 `@` 不出列表（保证列表与运行看板不同屏，残影论证最简单）。开了 CH1 之后每句话都是一轮，要是嫌「等回来才能补全」不方便，另开一刀做「忙时也弹」，要重新测列表与看板同屏时的残影。
- (c) **裁判模式的成员也列出来**：它们是成员、`@` 得到；CH1 怎么对待被 @ 的裁判由 CH1 定。要不要从列表里藏掉，等你定。（v1.1 补）按 CH1 契约的文本：自动那一轮不带 `--with-judge`，裁判照 PlanRound 的既有规则进「本命令不问」，所以用列表补出 `@<裁判 id>` 也**不会**让裁判回复；列出来只是让你打得出它的 id。
- (d) **只按 id 筛**：显示名是中文时敲中文筛不了（mention 只认 ASCII，§0 第 2 条）。
- (e) **第一次 Ctrl-C 只关列表**：列表开着时要按两次 Ctrl-C 才退出。
- (f) （v1.1）**`@` 前面是括号、中文或全角标点时也弹列表**：例如 `(@`、`中@`、`，@`。左边界与点名解析同一条规则（§1.1 D1），这些位置的点名本来就有效，所以列表跟着弹。方向说明里写的是「行首或空白之后」；要只在行首或空白之后弹，改 `atArm` 一行，并改 M2 的 `paren`、`han`、`fullcomma` 三个子测试。
- (g) （v1.1）**已经敲完整个 id 时回车只补空格**：句末是点名（`你怎么看 @<id>`）时列表仍开着，第一次回车补一个空格，第二次才提交（§6 真机步骤 4）。方向安全、不花钱。要不要改、怎么改，等你定；本刀不改。两种改法并列（W1b 修订）：
  - (i) 前缀恰好等于**唯一**匹配的 id 时，回车直接提交。你的配置里有一个 id 是另一个开头部分的对子：`opus-5.0` 与 `opus-5.0-max`、`fable-5.1` 与 `fable-5.1-max`（`docs/00_DECISIONS.md:1740`）。一对都在会话里时，敲完 `opus-5.0`、`fable-5.1` 列表有两行，照 (i) 仍要回车两次；敲完 `grok-4.7`、`opus-5.5`（`:2152`）只有一行，回车一次。同一个动作两种结果。
  - (ii) 前缀（转小写之后）恰好等于**选中项**的 id 时，回车直接提交。敲完的 id 总排第一、默认选中（原型实测：`opus-5.0` 那两行、`grok-4.7` 加一个 `grok-4.7-high` 那两行，都是敲完的那个带 `> `），所以上面四个都是回车一次；↓ 到别的项再回车，前缀不等于选中项，仍是补全。
  - 两种都只改 B4 的回车那一支、加用例，Tab 照旧只补全。

### 待主代理

（v1 的 (f)「派工前做一轮 W1」、(g)「按 CH1 真文件核相交表」已在 v1.1 完成，删去；字母不重排，(f)、(g) 让给上面两条待 Griffin。）

- (h) V1 只在 Windows 上跑过；Linux 上依赖 `rbbNewVT` 的 `mapNL` 处理（#42 (a)），没验。另一个要看的机制：V1 每个键之后 `rbbWaitUntil(150ms, 块数 > n)` 等不到不算错；慢机器（云端会话、`-race`）上几个键可能合进一次 flush，缩高没有单独发生，A1 这类变异就观测不到、V1 变成空绿。在那种环境跑时先确认 A1 下 V1 仍红，否则把等待加长或改成等块数确实增加再发下一个键。
- (i) P1、R1 依赖 `repl.New` 零值「裸文本只记下」。v1.1 核过 CH1 契约：REPL 字段 `autoReply` 零值 false 就是只记下，自动回复是 app 层的默认，P1、R1 都不经 `app.Run`，前提成立。CH1 落地后仍在它落地后的 main 上复跑 P1、R1 确认（万一零值被改，两条按只看用户消息的写法仍成立，§2.4，但会起假 CLI、变慢）。
- (j) 候选快照只在提交之后刷新：将来若有路径在进 TUI 之前选会话（例如启动时自动 `/use` 上次的会话），快照要到第一次提交之后才有，届时在 `tui.Run` 里 `p.Run` 之前补取一次（不许用 `p.Send`，#39 (a)）。
- (k) 归档文件名（§6 第 4 步）按当时 `docs/` 的编号定。
- (l) （v1.1）CH1 契约 v1.1 在本文修订期间还在收尾（本文核的是 21:18 与 21:32 两份）。CH1 定稿后、以及两刀落地前，再对它 grep 一次本刀的名字（`MentionCandidate`、`MentionCandidates`、`MentionBoundaryOK`、`MentionTokenRune`、`mentionName`、`mentionSource`、`atState`、`atMaxRows` 与 `at` 开头的八个方法），再核一次它的文件清单里没有 `internal/tui` 的生产文件、没有 `mentioncand.go`；并按上面「落地顺序」定下 CH1 §6 第 0 步的三选一。（W1b 修订）另有两件要由主代理转给 CH1 的修订人（本刀只读 CH1、不改它）。行号先按 `ch1_contract.md` 21:32 那一份；收尾时它变成了 22:10 那一份（CH1 自己的 W1b 修订，v1.2），又只读核了一次，结果写在括号里：
  1. CH1 §6 第 0 步（21:32 的 `:1218`；22:10 的 `:1247` 仍是原话）还写着「AT1 契约自己建议 CH1 先落」。AT1 v1.1 已撤回这句（见上面「落地顺序」下的说明），那里改成引用本文的说明。
  2. 「TUI 里敲 @ 会弹出可点的成员」在 21:32 那一份出现在 `msgAutoStrayAt`、`msgAutoStrayAtPartly` 两个字面量（`:480`、`:483`）、§3.2 第 3、4 句（`:1059`、`:1060`），以及说明文字 `:17`、`:1063`、`:1361`。本刀不做鼠标点选（§1.2），是上下键选、Tab 或回车补全。（22:10 那一份已改：两个字面量与 §3.2 两句换成「TUI 里敲 @ 会列出成员的 id 与显示名」（`:492`、`:495`、`:1084`、`:1085`），与本刀每行 id 与显示名一致（D9）；`:17` 加了注，`:1092` 是新说明。还剩两处旧说法：§6 第 0 步 `:1247` 引的半句、§7 那条说明 `:1390`。要转交的就剩这两处与第 1 条。）
- (m) （v1.1）§2.4 R1 新加的那一步（等 `当前会话:` 之后的空闲表头）不在裁决书里，是修订中实测发现的时序竞态（见变更记录最后一条与 §2.4【v1.1 那一步的来历】）。**已定（主代理，2026-09-28 22:30）：保留，关掉。** 理由：W1b 独立复测，有并发 `go test` 时 v1 形状 60 次红 1 次、v1.1 形状 60/60 绿；`MentionCandidates` 拖慢 200ms 时 v1 形状 5/5 红、v1.1 形状 5/5 绿。这是证实的竞态，不是偶发。
