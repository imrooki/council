# X44 冻结契约：TUI 多行提交里第 2 行起有斜杠命令就不提交（v1.2，2026-09-28）

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `2a8737f`（生产代码与 `ab53c0b` 逐字节相同；最后一次生产改动在 `a2e3d76`，之后只有测试与文档）。本文行号一律以 HEAD `2a8737f` 为准；测试腿跑 HEAD 红绿分布、实现腿取锚点，用的是派工时的 main HEAD（§2.0 末条、§5；v1.2 W1b 修订）。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`。本机没有 gcc，不能 `-race`。依赖版本：`charm.land/bubbletea/v2 v2.0.9`、`charm.land/bubbles/v2 v2.2.1`、ultraviolet 伪版本 `v0.0.0-20260811164956-006e29f97886`、`github.com/charmbracelet/x/ansi v0.11.8`。
- 本刀 KEY 是 `X44`；**测试腿的包级标识符前缀是 `mlx`**（类型、函数、变量、常量一律 `mlx` 开头，测试函数 `TestMlx...`）。生产代码的新标识符不许以 `mlx` 开头。
- 来源：`docs/00_DECISIONS.md` **#44**（`:2096-2103`）。**方向（Griffin 2026-09-27 夜裁，不许改）**：TUI 回车时，输入框文本多于一行、且第 2 行起任意一行以 `/` 开头，就**不提交**，文本原样留在输入框，状态栏提示一次只能一条命令；单行输入、以及第 2 行起没有 `/` 开头的多行（#44 引的未决 #12：长文粘贴）行为与 HEAD 逐字节相同。行首空白算不算、提示的措辞与持续时间、要不要留出路，由本契约冻结（§1.1 D2-D7）。不做自动拆成多条逐条执行，不改行式模式，不改 REPL 的命令解析。
- **原型**：本契约的全部期望值都在隔离拷贝 `E:\Isolating_test\council\x44_draft\`（`git archive 2a8737f` 解出）里用一份原型实跑得出：§1.1 的两处生产改动逐字落盘；§2 的四个测试文件按本文写成。原型下 `gofmt -l internal/repl/latercmd.go internal/tui/model.go` 为空、`go vet ./internal/repl ./internal/tui` 无报告、**全量 `go test -count=1 ./...` 八个有测试的包全绿（`cmd/council` 无测试文件），HEAD 既有用例零改动、零变红**；P1、R1（真 Program、真 Run）`-count=10` 零偶发。§4 的变异逐条在原型上跑过（锚点恰好一次、从字节副本还原并比对字节相等），点名杀手实测变红。另在同一拷贝里把两处生产改动退回 HEAD、只留 §2 的三个 `internal/tui` mlx 文件（model / program / run）实跑：挡下类用例全红、对照类全绿（§2.0 末条）。原型与变异装置已用 `fs.rmSync` 删除。
- **v1.1 原型**：`E:\Isolating_test\council\x44_v11\`（`git archive 2a8737f` 解出；§1.1 的三个代码块由脚本从本文抽出逐字落盘，锚点恰好一次，提示串十六进制与 §3.1 相同）。先按 v1 的 §2 重写四个测试文件、全绿，再逐条加 v1.1 的用例。v1.1 的每个新期望值都是这份原型的实跑输出；§4 全部 42 条非等价变异（v1 的 27 条 X1-X14、Y1-Y4、Y7-Y15 + v1.1 新登记的 15 条 Z 系列）逐条重跑，锚点恰好一次、从字节副本还原并比对字节相等，点名杀手实测全红。全量 `go test -count=1 ./...` 全绿、既有用例零改动；P1、R1 与新增的 M 用例 `-count=10` 零偶发。§2.0 末条的 HEAD 红绿分布（含新用例）实测与本文一致。另在 `git archive 80a9385`（MD1、CO1 已落地的 main）上同样落盘 §1.1 与四个测试文件：锚点恰好一次，`gofmt`、`vet` 干净，四个 mlx 文件全绿，全量 `./...` 九个有测试的包全绿（CO1 给 `cmd/council` 加了测试文件 `cox_flag_test.go`）。原型已用 `fs.rmSync` 删除。
- **v1.2 原型**：`E:\Isolating_test\council\x44_fix\`，两份 `git archive 80a9385` 解出的拷贝：`proto`（§1.1 三个代码块由脚本从本文抽出逐字落盘，锚点恰好一次，提示串十六进制与 §3.1 相同）与 `head`（不带生产改动）。两份里都只放按本文 §2.2 写成的 `internal/tui/mlx_model_test.go`（M1-M9）；L1、L2、P1、R1 本次没有改动，没有重写、没有重跑。`proto` 上 `gofmt -l`（`latercmd.go`、`model.go`、`mlx_model_test.go`）为空，`go vet ./internal/repl ./internal/tui` 无报告，M1-M9 全绿、`-count=5` 零偶发，全量 `go test -count=1 ./...` 九个有测试的包全绿，`go.mod`、`go.sum` 与 `head` 逐字节相同。`head` 上的红绿分布见 §2.0 末条。§4 全部 48 条（v1.1 的 42 条 + W1b 修订新登记的 Z17-Z22）逐条在 `proto` 上跑 `go test -count=1 -v -run TestMlx ./internal/tui`：变异文本由脚本从本文 §4 直接解析，锚点恰好一次，从字节副本还原并比对字节相等（48 条全部还原成功，收尾时两个生产文件的 SHA-256 与开跑前相同）。原型已用 `fs.rmSync` 删除。

## v1.1 变更记录（2026-09-28，落实主代理对 W1 的裁决 `x44_rulings.md`）

§1.1 冻结的生产代码**一个字节没改**（这几条都是测试侧的缺口）。改动全在验收、变异清单与给 Griffin 的步骤里：

- **F0 只查最后一行能活。** L1 加 J18 `"a\n/b\nc"`、J19 `"a\nb\n/c\nd"`；M1 加 `middle`（粘贴 J19 那条 4 行文本）。§4 登记 Z1（只查最后一行）、Z11（只查第 2 行或最后一行）。D1 改成由 J18、J19 钉住「任意一行」（J3 只钉住「不只看第 2 行」）。L1 标题「取值全集」改成「取值表」。
- **F1 空行当命令能活。** L1 加 K18 `"a\n\nb"`、K19 `"a\n   \nb"`（都是 false）；M2 加 `para`（粘贴 `"a\r\n\r\nb"`）。§4 登记这一族的两种写法：Z2（剥完是空的行算命令，K18、K19、M2 `para` 杀）、Z16（`line == ""` 算命令，K18、M2 `para` 杀；K19 杀不到它，所以两条都要）。§2.0 写明 M2 `para` 在 HEAD 上绿。D1、D8 补一句。
- **F2 文字与光标不动没钉住（minor，照补）。** M1 加 `trail`（粘贴 `"a\n/c\n"`，前提值同）；M1 每个子测试挡下之后再打一个 z，值 = 前提值 + `"z"`；新加 M8 `TestMlxBlockKeepsCursor`（粘贴 j2 → Ctrl-Home → 右移一格 → 回车被挡 → 打 z，值 = `"一z句话\n/round rba_a rba_b --go"`，光标在中间，往两头移都看得见）。§4 登记 Z3（`SetValue(text)`）、Z9（`CursorEnd`）、Z10（`MoveToEnd`）、Z12（`MoveToBegin`）、Z13（`CursorStart`）、Z14（`SetCursorColumn(0)`）。D5 的措辞改成与 M1、M8 的实际断言一致。
- **F3 Esc 与空闲 Ctrl-C 没测。** M3 在第 6 步之后插两步：第 7 步 Esc（Cmd nil、表头仍是提示、值不变），第 8 步 `tea.FocusMsg{}`（走 `updateInput` 最外层 default，表头仍是提示）；原第 7、8 步顺延为第 9、10 步（第 2-6 步编号不变，Y13-Y15 的点名不动）。新加 M7 `TestMlxIdleCtrlCQuitsAfterBlock`（挡下 → 空闲 Ctrl-C → 表头 `council t   正在退出`、叶子里有 quit、没有 BatchMsg）。§4 登记 Z4（空闲 Ctrl-C 在提示挂着时只清提示不退出）、Z7（Esc 清提示）、Z15（外层 default 清提示）；另补登记 W1 已实测被杀的 Z5（粘贴不改写提示，M4 杀）、Z6（Ctrl-J 清提示，M3 第 9 步杀），让 D6 列的每个端点都有一条变异。D6 的清单补上「其它消息（焦点、光标闪烁等）」，点名改成 M3、M4、M7。§2.0 写明 M7、M8 在 HEAD 上红。
- **F4** X5 的杀手改成「L1 K7、K8；M2 `leadblank`、`bomline`」（K11 在 X5 下仍是 false：`TrimSpace(lines[0])` 剥掉了行首空格；原型复测只有 K7、K8 两行红）。
- **F5** 相交表加一行 `postUserText`；CO1 那一列补全它的生产文件。§6 第 4、7 步改成：CO1 已落地时默认简洁档不打「已记下 seq N」，改用 `/log 3` 确认（或 `-verbose` 启动）。
- **随新用例一起更新的点名**（都是原型实测，只加不减）：X1 加 J19、M1 `middle`；X13 的 M1 例外加 `middle`、M 系列扩到 M3-M8；Y1、Y2、Y3、Y9、Y10 扩到 M3-M8；Y4 加 M8；Y11、Y12 加 M3 第 6 步与 M8。
- **变异编号对照**：Z1-Z7、Z9、Z10 沿用 W1 的编号与含义（Z5、Z6 的形状按 W1 的描述在原型上重写）；证伪者 V0 叫「Z2」的那条（只查第 2 行或最后一行）在本文是 **Z11**，避免与 W1 的 Z2（空行算命令）撞号；Z12-Z16 是 v1.1 新编。W1 的 Z8 没给形状（W1 报它被 L1 J17、L2 杀），与 X14 同族，未单独登记。
- **基线说明**：本文行号仍以 HEAD `2a8737f` 为准。修订期间 main 前进到 `80a9385`（`1c62dbb` CO1、`e9afff9` MD1、`80a9385` 文档）：`internal/tui/model.go`、`internal/repl/seam.go` 逐字节不变（§1.1 B 的锚点仍恰好一次）；`commands.go` 与 `repl.go` 的行号后移，本文引用的几处在 `80a9385` 上是：`isTrimmable` `:267`（原 `:266`）、`postUserText` `:329`（原 `:327`）、「已记下 seq %d」`:341`（原 `:338`，已包进 `if !r.concise`）、`restOf` `:492`（原 `:486`）、`cmdNew` 的 `title := args.rest` `:726`（原 `:720`）、`cmdSay` `:1123`（原 `:1097`）、`repl.go` 的「未知命令，试 /help」`:44`（原 `:42`）。W2 派工时按 `80a9385` 的真文件再核一次锚点。

## W1b 修订（v1.2，2026-09-28，处理第二轮审查 W1b 的三条发现）

§1.1 冻结的生产代码仍然**一个字节没改**；三条发现都是测试侧或说明文字的缺口，全部成立、全部照改，没有改变用户能看到的行为，没有推翻裁决书。v1.1 原文快照：`E:\multi_model_cowork\x44_contract_v11.md`。

- **发现 1（minor，成立）：回车之后不跑 Cmd 就读 `sub.count()`，这一句永远是绿的。** `Submit` 只在 `submitCmd` 返回的 Cmd 被执行时才会被调；不跑 Cmd，不管门写成什么样计数都是 0。W1b 实测改之前这一句在 HEAD 上绿；v1.2 原型实测改之后在 HEAD 上红（M1 十个子测试与 M8 `block` 都是 `got 1 want 0`，M5 是 `got 2 want 1`）。照改：
  - M1、M3 第 6 步、M5 最后一次回车、M6 最后一次回车、M8 `block`：回车之后先 `tui1bRun(t, cmd)`（返回值不用；Cmd 为 nil 时它什么都不做，忠实实现不受影响），再断言 `sub.count()`。
  - M8 拆成两个子测试：`block`（v1.1 的 M8）与 `control`（同一装置，只差一个 `/`：粘贴 K3 的文本，Ctrl-Home、右移、回车，真的提交）。`block` 的缺席断言（Cmd 为 nil、count 为 0、值不变、表头是提示）以 `control` 为对照。
  - D5 的「Submit 没被调」改成对应新写法；§2.0 末条改成 M8 `block` 五句全红、`control` 绿。
  - §4：Y1、Y3、Y12 实测跑 Cmd 之后 count 也红（Y12 在 M1、M3 第 6 步、M5、M6、M8 `block` 上都是 Cmd 与 count 两句红），点名里注明；Y11 只回显不提交，count 不红，不注。
- **发现 2（minor，成立）：「删掉犯规的行之后提示还挂着」没被钉住。** 按键 default 里「值不再犯规才清提示」（Z17，W1b 称 S1）在 v1.1 的全部用例下存活：M3 从头到尾没进过「值不犯规」的状态。它违反 §1.1 B 冻结的注释与 D6 冻结的「已知代价」。照改：新加 M9 `TestMlxHintStaysAfterFix`。**问了一句「同类的还有没有别处」**：同一种「值不犯规才清」的写法可以放在 D6 列的每一个不清提示的端点上，M3 同样杀不到（Ctrl-J 那一处除外：M3 第 9 步按完 Ctrl-J 之后值恰好不犯规）。所以 M9 在删掉犯规的行之后再依次喂 Esc、窗口尺寸、tick、焦点消息，每一步都断言提示还在；§4 登记 Z17（按键 default）、Z18（Esc）、Z19（窗口尺寸）、Z20（tick）、Z21（最外层 default），杀手是 M9 对应的那一步；Ctrl-J 那一处（W1b 称 S1b，W1b 已实测被 M3 第 9 步杀）补登记为 Z22。六条全部实跑，点名杀手实测变红，最早红的就是点名的那一步。M9 最后一步回车照常提交，同时证明删掉犯规的行之后能正常发出。D6 的「钉住的用例」加上 M9；§2.0 记 M9 在 HEAD 上红在前提那一句。
- **发现 3（minor，成立）：§2.0 末条、§5 仍写 `2a8737f`。** 改成「派工时的 main HEAD（写本文时是 `80a9385`；`model.go`、`seam.go` 与 `2a8737f` 逐字节相同）」，文件头第一条补一句指过去。`80a9385` 上 M1-M9 的实测分布与 §2.0 末条一致（v1.2 原型；P1、R1 在 `80a9385` 上不带生产改动还没人跑过，§2.0 末条已写明由测试腿实跑报告）。其余几处 `2a8737f`（文件头的仓库描述与两段原型说明、基线说明、§0 标题、§6 第 4 步的归档说明）指的是行号基线与历史，照旧。
- **随新用例一起更新的点名**（v1.2 原型实测，只加不减）：X13、Y1、Y2、Y3、Y9、Y10 由「M3-M8」改成「M3-M7、M8 `block`、M9」；Y4 加 M5、M6、M9；Y11 加 M5、M6；Y13 加 M9 第 2 步，Y14 加 M9 第 4 步，Y15 加 M9 第 5 步，Z7 加 M9 第 3 步，Z15 加 M9 第 6 步；Z12、Z13、Z14 加 M9；§4 里凡写 M8 的地方改成 M8 `block`（M8 `control` 在 48 条变异下全部绿，它只做对照）。
- **没改的**：W1b 附注里的 S4（`roundEndMsg` 分支清提示）W1b 自己没列为发现；E7 按可达性论证它等价，维持，不加用例。W1b 附注里「任务书列了 CX1 但没给输入」这一条同样适用于本次修订：本次只改 X44。

## 与 MD1 / CO1 / SM1 的相交表

三刀的契约：`E:\multi_model_cowork\md1_contract.md`（模型识别 + `/add`）、`co1_contract.md`（简洁输出）、`sm1_contract.md`（自动总结）。本刀只动两个生产文件：新文件 `internal/repl/latercmd.go`、`internal/tui/model.go` 的 `beginSubmit` 一处补丁。

| 本刀的文件 / 函数 | MD1 | CO1 | SM1 | 结论 |
|---|---|---|---|---|
| `internal/repl/latercmd.go`（新文件，函数 `HasLaterCommandLine`） | 新文件是 `addmodel.go`，改 `repl.go`、`commands.go`、`configcmd.go`（只改头注释）；另有 `internal/provider/discovery.go`、`discovery_parse.go`（新）、`capabilities.go`、`claude.go`、`agy.go`、`grok.go`，`internal/turn/discovery_exec.go`（新），`internal/config/append.go`（新）（v1.1 补全，照 `e9afff9` 的实改文件核对） | 新文件是 `verbosity.go`，改 `repl.go`、`observer.go`、`ask.go`、`round.go`、`commands.go`；另有 `internal/tui/roundobs.go`（`RoundEnd` 方法体，`outcomeWord` / `wallText` / `formatElapsed` 改成委托，输出字节不变）、`internal/app/app.go`、`cmd/council/main.go`（v1.1 补全，照 CO1 契约的文件清单与 `1c62dbb` 的实改文件核对） | 新文件是 `summary.go`，改 `round.go`、`ask.go`、`observer.go`、`commands.go` | 不相交：文件名不同；三份契约里没有 `HasLaterCommandLine`、`latercmd`、`oneCommandHint`、`mlx` 任何一个名字（现查零命中）。本刀**不碰** `seam.go`、`commands.go`（`isTrimmable` 只读引用，定义在 `commands.go:266-275`，三刀都不改它） |
| `internal/tui/model.go` 的 `beginSubmit`（`:465-504`）与新常量 `oneCommandHint` | `internal/tui` 一个字不改（MD1 §1.1 B 末条）；引用 `model.go:341-347`（Ctrl-C） | 明写不改 `model.go`（CO1 §1.2）；引用 `:325`、`:356-359`、`:447-456` | 不改 `internal/tui` 生产代码；引用 `:229`、`:256` | 不相交。本刀的插入点在 `:462` 之后，三份契约引用的 `model.go` 行号（229、256、325、341-347、356-359、447-456）全在它前面，含义不变 |
| 测试：`internal/tui`、`internal/repl` 两个包 | 测试前缀 `mdx`；在 `internal/turn` 加 TestMain | 前缀 `cox` | 前缀 `smx` | 不相交。本刀不加 `TestMain`（沿用 `internal/tui/rba_fakecli_test.go` 的那一个）。三刀的 TUI 用例都是单行提交（`rbbStartProg` 的 `submit` 粘贴单行、CO1 A1 在管道里逐条写 `...\r`），三份契约里没有任何「第 2 行起以 `/` 开头」的多行提交（现查零命中），落地后不会被本刀挡下 |
| D3 出路的终点：`postUserText`（`commands.go:327` 起，「已记下 seq %d」在 `:338-340`）（v1.1 加，F5） | 不改 | 把「已记下 seq %d」那三行包进 `if !r.concise { ... }`（CO1 契约 F2）；app 装配时默认简洁（`r.SetConcise(!opts.Verbose)`），零值是详细 | 不改 | 按文件名不相交，按输出相交：本刀的 `/say` 出路最后走 `cmdSay` → `postUserText`。mlx 用例不受影响：L2、P1、R1 的装置都直接 `repl.New`（`rbaNewRig`、`rbaNewTuiRig`），零值是详细档，照打「已记下 seq N」（v1.1 原型在 `80a9385` 上实测四个 mlx 文件全绿）。受影响的只有给 Griffin 的真机步骤：CO1 已落地（`1c62dbb`），默认简洁档下屏幕上不打「已记下 seq N」，§6 第 4、7 步已改成用 `/log 3` 确认（或 `-verbose` 启动） |

**落地顺序**：与三刀互不依赖，先落后落都行。后落时按当时的真文件重取 `model.go` 的锚点（三刀都不改 `model.go`，锚点应仍恰好一次；主代理落盘前核一次）。本刀若在 MD1 之后落：MD1 新增的 `/add +N` 等单行命令不受影响；给 Griffin 的真机步骤里一条命令一行（§6）。

---

## 0. 病根 / 动机（行号以 HEAD `2a8737f` 为准）

1. **TUI 的一次提交可以是多行。** 三种来路都会让输入框里出现换行：Ctrl-J（`internal/tui/model.go:352-354`）；括号粘贴（`:375-379` 的 `tea.PasteMsg` 交给 textarea）；以及终端把粘贴当成原始字节送来时，LF 字节被解成 Ctrl-J（ultraviolet `decoder.go:1157-1161`，`0x0a + 0x60 = 'j'` 带 Ctrl），CR 字节被解成回车（`:1140-1144`）。textarea 收进来的文字先过 sanitizer（bubbles v2.2.1 `internal/runeutil/runeutil.go` 的 `Sanitize`，`textarea.go:543-547` 调用）：`\r` 与 `\n` **各自**变成一个换行（所以 CRLF 粘贴会多出空行），tab 变 4 个空格，其余控制字符丢掉；BOM（U+FEFF）与零宽字符（U+200B/C/D）不是控制字符，原样保留。所以 `ta.Value()` 里永远没有 `\r` 与 tab。
2. **`beginSubmit`（`model.go:465-504`）把整段文字一次交给 `Submit`，回显却逐行印。** `:467` 只去掉末尾的 `\r\n`；`:472-476` 按 `\n` 切行、每行回显 `"> " + 行`；`:502` 整段交给 `submitCmd`。
3. **REPL 的 `Submit`（`internal/repl/seam.go:56-89`）只把整段的第一个词当命令。** `:64` 用 `isTrimmable`（`commands.go:266-275`：`unicode.IsSpace` 加 BOM 与三个零宽字符）剥掉整段首尾；`:68` 以 `/` 开头才是命令；`:76` 的 `strings.Fields` 跨行切词，第一个词是命令名；`:88` 的 `restOf`（`commands.go:486-492`）把第一个空白之后的**全部内容**（含后面几行）当参数。于是 Griffin 的第一块 `/new 真机复验` 换行 `/add opus-5.0,grok-4.7` 成了一条 `/new`，标题是后面全部（`cmdNew` 的 `title := args.rest`，`commands.go:720`）；第二块「一句话」换行 `/round … --go` 不以 `/` 开头，整段走 `sayCmd`（`seam.go:68-74`）记成一条用户消息，`/round` 没执行。回显却印成 `> /new …`、`> /add …`、`> /round …`，看起来都执行了。原型在真 `tui.Run` 上复现：HEAD 上 `/new 真机复验\nadd rba_a` 建出一个标题为 `真机复验\nadd rba_a` 的会话（R1 `control_new` 的对照就是它）。
4. **状态栏是一个字符串 `m.status`，写它的地方（HEAD 全部）**：`New` 的 `空闲`（`:166`）；`submitDoneMsg` 的四支（`:317`、`:321`、`:326`、`:330`）；忙时 Ctrl-C 的 `正在取消`（`:345`）、空闲 Ctrl-C 的 `正在退出`（`:349`）；忙时 Enter 的 `一轮正在跑，先等它回来或按 Ctrl-C 取消`（`:358`）；`PasteMsg` 的 `粘贴了 N 个字符`（`:378`，RB-A 剩余项 (a)：忙时也照改，既有行为）；`beginSubmit` 的 `正在运行`（`:480`）。**不写** status 的：tick（`:208-214`，只在忙且代次对时改已用时长）、`WindowSizeMsg`（`:293-310`）、打字与光标键（`:369-372` 交给 textarea）、Esc（`:366-367`）、Ctrl-J（`:352-354`）、RoundStart / MemberDone / RoundEnd / boardFinalize（`:216-283`）。不忙时表头就是 `council <版本>   <status>`（`roundobs.go:202-205`）。
5. **表头不截断，渲染器截断。** `chrome()`（`model.go:409`）拼表头时不按宽度截；bubbletea 的渲染器用 `uv.NewStyledString(view.Content)`（`cursed_renderer.go:301`），`Wrap` 默认 false，超宽的行在终端宽度处被截掉（ultraviolet `styled.go:50-61`、`:164-175`），不折行、不改帧高（RB-A 剩余项 (c) 同一件事）。所以状态栏提示长了只会被截尾，不会把活区撑高。
6. **行式模式不受影响。** 非 TUI 路径由 `dispatch`（`commands.go:171-236`）逐行读、逐行判命令，一次粘贴三行就是三条独立输入。本刀不改它。

---

## 1. 范围

### 1.1 做

本刀的生产代码**全部逐字冻结**（两处，合计约 50 行，含注释）。没有「名字实现方定」的地方。

#### A. 新文件 `internal/repl/latercmd.go`（逐字照抄）

```go
package repl

import "strings"

// HasLaterCommandLine 报告一次多行提交里，第 2 行起有没有哪一行单独提交时会被 Submit 当成斜杠命令。
// TUI 在回车时用它挡住「整块粘贴几条命令、按一次回车」（00_DECISIONS #44）：
// Submit 只把整段的第一个词当命令，后面几行全成了它的参数或正文，而回显照样逐行印成「> /...」。
//
// 行号与判定都按 Submit 的看法算：
//   - 先像 Submit 那样把整段首尾的空白与四个不可见字符剥掉（isTrimmable），再按 "\n" 切行；
//     剥掉之后的第一行才是第 1 行，前导空行不算。
//   - 一行剥掉行首的空白与四个不可见字符之后以 "/" 开头，就算命令行。行首缩进不改变判定：
//     Submit 收到 "  /new x" 同样当命令执行。
//
// 例外：第 1 行去掉行尾空白后恰好是 "/say" 时返回 false。那时 Submit 的第一个词就是 /say，
// 它把 /say 之后的全部内容原样当成一条消息发出（seam.go 的 restOf 分支），这是用户明说的「整段是消息」。
// 行尾只去空白（unicode.IsSpace），不去四个不可见字符：Submit 按空白切词，"/say" 后面紧跟一个
// 零宽字符时第一个词不是 /say，Submit 会答「未知命令」，那种输入不豁免。
// 第 1 行是 "/say 某些字" 时也不豁免。
func HasLaterCommandLine(text string) bool {
	lines := strings.Split(strings.TrimFunc(text, isTrimmable), "\n")
	if len(lines) < 2 {
		return false
	}
	if strings.TrimSpace(lines[0]) == "/say" {
		return false
	}
	for _, line := range lines[1:] {
		if strings.HasPrefix(strings.TrimLeftFunc(line, isTrimmable), "/") {
			return true
		}
	}
	return false
}
```

- 名字 `HasLaterCommandLine` 冻结（测试腿直接调用）。导出是因为调用方在 `internal/tui`；放在 `repl` 包是因为判据就是「这一行单独交给 `Submit` 会不会被当成命令」，要与 `Submit` 用同一个 `isTrimmable`，不另写一份。
- 不改 `seam.go`、`commands.go` 的任何一行；不改 `isTrimmable`、`restOf`、`dispatch`、`Submit`。

#### B. `internal/tui/model.go`：一个补丁

锚点（HEAD `:465-470`，逐字，含行首 tab，在文件里恰好出现一次）：

```go
// beginSubmit 把当前输入定稿到回滚区，并在后台启动一次提交。
func (m Model) beginSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimRight(m.ta.Value(), "\r\n")
	if strings.TrimSpace(text) == "" {
		return m, nil
	}
```

替换成（逐字）：

```go
// oneCommandHint 是多行提交被挡下时状态栏的那句话（00_DECISIONS #44）。
const oneCommandHint = "一次只能一条命令，逐条回车；整段当消息发，首行只写 /say"

// beginSubmit 把当前输入定稿到回滚区，并在后台启动一次提交。
//
// 多行输入里第 2 行起有斜杠命令时不提交（repl.HasLaterCommandLine）：输入框原样保留、
// 不回显、不进忙，只把状态栏换成 oneCommandHint。这句提示留到下一次改写状态栏的事件
// （成功提交、粘贴、空闲时 Ctrl-C）为止；打字、tick、窗口尺寸变化都不动它。
func (m Model) beginSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimRight(m.ta.Value(), "\r\n")
	if strings.TrimSpace(text) == "" {
		return m, nil
	}
	if repl.HasLaterCommandLine(text) {
		m.status = oneCommandHint
		return m, nil
	}
```

- 这是 `model.go` 唯一的改动。`import` 不变（`council/internal/repl` 在 `:15` 已经 import）。`beginSubmit` 其余部分（`:472-503`）、`Update`、`updateInput` 的 Enter 分支（`:356-364`）、`PasteMsg` 分支、`View`、`chrome`、`frameHeight` 一个字节都不改。
- 常量名 `oneCommandHint` 冻结；测试腿**不许**引用它（测试文件自己写一份字面量，§3.1），否则「措辞被改」的变异杀不到。

#### C. 冻结的论证（实现方照此实现；测试腿照此出用例）

- **D1 判据（`HasLaterCommandLine` 的语义，逐条对应代码）。** 整段先按 `Submit` 的规矩剥掉首尾 `isTrimmable` 字符，再按 `\n` 切行，**剥掉之后的第一行才是第 1 行**：`"\n/new x"` 对 `Submit` 就是一条 `/new x`，不挡（M2 `leadblank`、L1 K7）；`BOM + "\n/new x"` 同理（M2 `bomline`、L1 K8，用来区分「用 `isTrimmable`」与「用 `TrimSpace`」）。第 2 行起**任意一行**剥掉行首 `isTrimmable` 字符后以 ASCII `/` 开头就挡。「任意一行」由 L1 J18（`/` 行在第 2 行、后面还有正文）、J19（`/` 行既不是第 2 行也不是最后一行）与 M1 `middle` 钉住：只查最后一行（Z1）、只查第 2 行或最后一行（Z11）都会红；J3、M1 `line3` 只钉住「不只看第 2 行」。中间夹空行也挡：L1 J9、M1 `crlf`。空行、剥完只剩空白的行不是命令行（L1 K18、K19，M2 `para`：段落之间隔空行的长文照常提交）。第 1 行是不是命令都一样（L1 J2：第 1 行是正文；J1：第 1 行是命令）。`/` 不在行首的不挡（L1 K5 `a\nb/c`、K6 `a\nhttp://x`）；全角斜杠 U+FF0F 不是 `/`，不挡（L1 K13，与 `Submit` 的 `HasPrefix(text, "/")` 一致）。
- **D2 行首空白算「以 / 开头」（冻结：算）。** 理由：(1) 判据是「这一行单独交给 `Submit` 会不会被当成命令」，而 `Submit`（`seam.go:64`）与行式 `dispatch`（`commands.go:214`）都先剥 `isTrimmable` 再判 `/`，`"  /add x"` 单独提交就是命令；(2) Griffin 粘贴的块可能带缩进（Markdown 列表里的代码块）、可能带全角空格 U+3000（中文输入法）、可能带网页复制来的 BOM 或零宽字符（`commands.go:206-213` 的注释写过这类实证）；只认第 0 列的 `/` 会让这些块照样一次提交、照样逐行回显成 `> /...`，就是 #44 要防的那种误会。代价：缩进的代码注释（`// ...`）、缩进的路径行也被挡，出路见 D3。L1 J4-J8、J16，M1 `tab`、`bom`、`keyboard` 钉住。
- **D3 出路：第 1 行只写 `/say`（冻结：留这条出路）。** 第 1 行去掉行尾空白后恰好是 `/say` 时不挡。论证：
  1. 不新增任何解析规则、按键或字面量：这就是 HEAD 上 `Submit` 已有的语义——第一个词是 `/say`、其后非空时，`restOf` 把后面**全部内容**当成一条消息发出（`seam.go:81-88` → `cmdSay`，`commands.go:1097-1110` → `postUserText`）。消息正文 = 第 1 行之后的全部行（整体去掉首尾空白，中间一字不改）。L2 在真 REPL 上逐条核对这一点。
  2. 只认「第 1 行**只有** `/say`」：`/say 一句话` 换行 `/round …` 照样挡（L1 J10）。主代理给 Griffin 的步骤块若写成「`/say xxx` 再一行 `/round`」，这正是 #44 那种误会，不能放过；而「第 1 行单独一个 `/say`、下面贴一大段」是用户明说「整段是消息」，几乎不会误用。
  3. 行尾只去 `unicode.IsSpace`（`strings.TrimSpace`），**不去** BOM 与零宽字符：`Submit` 按 `unicode.IsSpace` 切词，`"/say" + U+200B` 这个词不是 `/say`，`Submit` 答 `未知命令，试 /help`、什么都不发（L2 实测）。那种输入若被豁免，提交后输入框被清空、内容丢失，所以不豁免、照样挡（L1 J17）。大小写敏感（`/SAY` 在 `Submit` 里是未知命令，L1 J12）。`/sayx` 不是 `/say`（L1 J11）。
  4. 这条出路在 TUI 里的操作：挡下之后按 Ctrl-Home（textarea 的 InputBegin，`textarea.go:109`）回到开头，敲 `/say`，按 Ctrl-J，再回车。M3、P1、R1 都照这个操作走一遍，并核对发出去的正文与原来粘贴的逐字节相同（这同时证明挡下时文字一个字节没丢）。
  5. 不给 `/ask <id>`、`/to <ids>` 的多行正文开同样的出路（它们也把后面全部当正文，`ask.go:49`、`commands.go:416`）：v1 只开 `/say`，列进 open items 待 Griffin。
- **D4 挡在哪、与既有门的顺序。** 门在 `beginSubmit` 里、空输入判断（`:468-470`）之后、回显之前。`beginSubmit` 只由 Enter 分支调用（`:364`，现查唯一调用点），所以回车的判定顺序是：忙（`:357-360`，打忙时那句、`return`）→ 收口窗口（`:361-363`，静默 `return`）→ 空输入（静默 `return`）→ **本刀的门** → 正常提交。忙时、收口窗口里、空输入时的行为与 HEAD 逐字节相同（M5、M6）。退出中那个间隙（RB-B 剩余项 (h)）不在本刀范围。
- **D5 挡下时只改 status。** 返回 `(m, nil)`：不回显、不 `Reset` 输入框（文字与光标位置都不动）、不置忙、不动 `run` / 看板 / `floor` / `tickGen` / `tickPending` / `cancel`。帧高不变（提示在表头那一行里，不加行；`exitClearSeq` 因此不变）。验收（v1.1 与断言对齐）：M1 断言 Cmd 为 nil、把这条 Cmd 跑到底（`tui1bRun`）之后 Submit 没被调（v1.2：`Submit` 只在 Cmd 被执行时才会被调，不跑就读计数，这一句恒绿）、没有回显、表头是提示、值逐字节等于前提值、帧高仍是 8，再打一个 z、值等于前提值 + `"z"`（光标仍在原处；`trail` 的前提值以换行结尾，钉住「不把去掉末尾换行的 `text` 写回输入框」）；M8 `block` 把光标放在第 1 行第 1 个字之后再回车，打 z 之后是 `"一z句话\n/round rba_a rba_b --go"`，钉住光标不往头、不往尾挪；M8 `control`（同一装置，只差一个 `/`，真的提交）是 `block` 那几条缺席断言的对照（v1.2）。`run` / 看板 / `floor` / `tickGen` / `tickPending` / `cancel` 没有逐个断言：置忙由表头多出 `  0s` 看出（Y9）、帧高由 `frameHeight` 看出；其余几个字段在门里被改写的变异没有登记，归「只在审查时看」（§4 末）。
- **D6 提示持续到什么时候（冻结）。** 提示就是 `m.status`，留到下一个写 status 的事件为止：**成功提交**（→ `正在运行`）、**粘贴**（→ `粘贴了 N 个字符`，`:378`，与 RB-A 剩余项 (a) 同一处，既有行为不改）、**空闲时 Ctrl-C**（→ `正在退出`，随即退出）。原样再按一次回车 → 同一句提示（幂等，仍不提交）。**不**清提示的：打字、退格、光标键、Ctrl-J、Esc、窗口尺寸变化、tick（空闲时 `elapsedTickMsg` 在 `:210` 被忽略）、其它消息（焦点、光标闪烁等 `updateInput` 最外层 default `:381-384` 交给 textarea 的消息）、RoundStart / MemberDone / RoundEnd / boardFinalize（空闲时被忽略或不碰 status）。挡下时不忙，所以 tick 根本不会被装上，不存在「被 tick 立刻盖掉」。理由：不加计时器、不加新状态，与既有的 `粘贴了 N 个字符` 同一种持续方式。已知代价：用户把犯规的行删掉之后、按回车之前，提示还挂着（接受）；「值不再犯规才清提示」也不许，哪个端点都不许（v1.2）。钉住的用例：M3（打字、退格、窗口尺寸、tick、再回车、Esc、焦点消息、Ctrl-J）、M4（粘贴改写）、M7（空闲 Ctrl-C 照旧退出）、M9（v1.2：删掉犯规的行之后，退格、Esc、窗口尺寸、tick、焦点消息都不清提示；Ctrl-J 那一处由 M3 第 9 步钉住，那时值恰好也不犯规）。RoundStart / MemberDone / RoundEnd / boardFinalize 在提示挂着时到不了（挡下只发生在不忙且不在收口窗口时，下一次提交先把 status 写成 `正在运行`），对应的变异在可达状态下等价，不列。
- **D7 措辞与宽度。** 提示 `一次只能一条命令，逐条回车；整段当消息发，首行只写 /say`：显示宽度 55 列（`ansi.StringWidth`，原型实测），UTF-8 80 字节。说清三件事：一次只能一条命令（Griffin 裁定的原话）、命令要逐条回车、整段当消息发的出路。表头 `council 0.2.0-s10   ` 占 20 列，合起来 75 列，80 列终端放得下；更窄时被渲染器截尾（§0 第 5 条），帧不变形。「没提交」三个字没写进提示：挡下时没有回显、输入框原样，已经看得出没提交；写进去就超过 80 列。
- **D8 单行与长文不变。** 单行输入、以及第 2 行起没有 `/` 开头的多行，`HasLaterCommandLine` 返回 false，`beginSubmit` 其余代码一个字节不变，回显、Submit 收到的文字、状态都与 HEAD 相同。既有担保：`TestTui1bAcc6MultilineSubmitKeepsNewlineByteForByte`（`a\nb` 键盘路与粘贴路）、`TestTui1bAcc6SubmitEchoesEachLineBeforeSubmitting`（`a\nb` 的回显与顺序）原样绿；本刀另加 M2、P1 `control`、R1 `control_*`。段落之间隔空行的长文（CRLF 粘贴后每个空行变三个）照常提交：L1 K18、K19，M2 `para`（v1.1）。
- **D9 行式模式不改。** `HasLaterCommandLine` 只被 TUI 调用；`dispatch`、`Submit`、`collectDraft` 不变，行式输出逐字节不变（全量既有用例原样绿）。

### 1.2 不做（两条腿都不许越界）

- 不改行式模式；不改 `internal/repl` 里除新文件 `latercmd.go` 以外的任何文件（`seam.go`、`commands.go`、`repl.go` 一个字节都不动）；不改 REPL 的命令解析（`Submit`、`dispatch`、`restOf`、`isTrimmable`）。
- 不做「自动拆成多条逐条执行」；不做「再按一次回车就照发」（Griffin 被挡之后的第一反应多半就是再按一次回车，那等于没挡）；不加清空输入框的按键；不改 `PasteMsg` 的状态文字；不在活区加行；不改 `/ask`、`/to`。
- 不改 `internal/tui` 里除 `model.go` 以外的文件（`run.go`、`roundobs.go`、`writer.go`、`vt_*.go`）；不改 `internal/app`、`cmd/`、`go.mod`、`go.sum`。
- **既有 `_test.go` 一个字节都不许改（含注释），本刀零授权。** 原型实测：两处生产改动落盘后全量 `go test ./...` 全绿，没有任何既有用例需要改。
- 测试腿不写生产代码，实现腿不写 `_test.go`。测试腿不加 `TestMain`。

---

## 2. 验收（测试腿新写的文件）

### 2.0 通用纪律

- 新文件四个：`internal/repl/mlx_latercmd_test.go`（L1、L2）、`internal/tui/mlx_model_test.go`（M1-M9）、`internal/tui/mlx_program_test.go`（P1）、`internal/tui/mlx_run_test.go`（R1）。都不带构建标签（`os.Pipe` 两个平台都能用，先例 `rba_wiring_test.go`）。
- **包级标识符一律以 `mlx` 开头**，测试函数 `TestMlx...`。只读复用同包既有辅助、不许改它们：tui 包 `tui1bSubmitter`、`tui1bPrinter`、`tui1bNopDrainer`、`tui1bStep`、`tui1bRun`（与它返回的 `tui1bTrace` 的字段 `log`、`sawBatch` 和方法 `hasQuit`）、`tui1bType`、`tui1bKey`、`tui1bCtrl`、`tui1bEnter`、`tui1bEsc`（v1.1，M3 第 7 步）、`tui1dBuf`、`tui1dWaitFor`、`tui1dESC`、`rbaFlat`、`rbaT0`、`rbaNewTuiRig`、`rbaTuiRig`（字段 `svc`、`r`、`lw`、`convID`）、生产的 `replAdapter`、`newRoundObserver`、`printLine`、`Run`、TestMain 假 CLI（`rba_fakecli_test.go`）；repl 包 `rbaNewRig`、`rbaRig`（字段 `ctx`、`svc`、`convID`）与其方法 `newREPL`、`rbaBuf`（`Reset`、`String`）。
- 可以读 `Model` 的既有未导出字段 `ta`（`model.go:73`）、`tickGen`（`:94`）与方法 `chrome()`、`frameHeight()`；这些是 HEAD 的名字，本刀不改。
- **提示字面量在测试文件里自己写一份**（§3.1，逐字节），不许引用 `oneCommandHint`。下文记 `H` = 那一串，`HDR` = `"council t   " + H`。
- **不可见字符与特殊空白一律用十六进制码位写**：`string(rune(0xFEFF))`（BOM）、`string(rune(0x200B))`（零宽空格）、`string(rune(0x3000))`（全角空格）、`string(rune(0x00A0))`（不换行空格）、`string(rune(0xFF0F))`（全角斜杠）。不许在源码里放这些字符本身，也不许用反斜杠 u 转义（编辑与传输路径会把转义还原成真字符；U+200D 还在符号禁令区间里，本刀用不到它）。
- 不起真 provider；store 一律在 `t.TempDir()` 下（两个 rig 已如此）；不碰 `C:\Users\25142\.council`。
- 断言整串相等。例外：R1 的终端字节流（剥 ANSI、删空格之后找子串，理由同 RB-A W1）、前提守卫。每条用例注释写【判别力】：能不能为假、只差一处的对照是哪条。
- **HEAD 上的预期（测试腿交付前实跑并报告命令与结果）**：在**派工时的 main HEAD** 的隔离拷贝里（写本文时是 `80a9385`；`internal/tui/model.go`、`internal/repl/seam.go` 与 `2a8737f` 逐字节相同。v1.2 修订）只放 `internal/tui` 的三个 mlx 文件（不放生产改动、不放 repl 的 mlx 文件；这三个文件只用 HEAD 已有的标识符，必须能编译），跑 `go test -count=1 -run TestMlx ./internal/tui`：**M1 全部子测试（含 v1.1 的 `middle`、`trail`）、M3、M4、M5、M6、M7、M8 `block`、M9、P1 `reject`、R1 `reject_cr`、`reject_new` 红；M2 全部子测试（含 v1.1 的 `para`）、M8 `control`、P1 `control`、R1 `control_cr`、`control_new` 绿**（v1.1 原型在 `2a8737f` 上实测：10 个 M1 子测试、M3-M8、P1 `reject`、R1 两个 `reject_*` 红；7 个 M2 子测试、P1 `control`、R1 两个 `control_*` 绿。v1.2 原型在 `80a9385` 上实测 `mlx_model_test.go`：10 个 M1 子测试、M3-M7、M8 `block`、M9 红；7 个 M2 子测试、M8 `control` 绿；按用例算，与 W1b 在 `80a9385` 上对 v1.1 的 M1-M8 的实测一致。P1、R1 的 HEAD 分布只在 `2a8737f` 上实测过：v1.1 原型在 `80a9385` 上只跑了带 §1.1 的全绿，W1b 与 v1.2 都没在 `80a9385` 上不带生产改动跑它们；测试腿照本条实跑报告，与上文不一致就停下报主代理）。M1 每个子测试六句全红（Cmd、跑 Cmd 之后的 count、回显、表头、值、最后一句；count 那句是 v1.2 改成先跑 Cmd 之后才会红的）。M7、M9 在 HEAD 上红在前提那一句（回车照样提交、表头是 `正在运行  0s`，不是提示）；M8 `block` 五句全红（Cmd、count、表头、值、最后一句）；M5、M6 红在最后一次回车的 Cmd、count、表头、值四句（M5 的 count 是 `got 2 want 1`）。`internal/repl/mlx_latercmd_test.go` 引用新函数，HEAD 上编译不过，是预期的红。

### 2.1 `internal/repl/mlx_latercmd_test.go`

**L1 `TestMlxLaterCommandLineTable`：判据的取值表。** 对下表每一行断言 `HasLaterCommandLine(输入) == 期望`（表驱动，失败信息带编号与 `%q` 的输入）。`BOM`、`ZWSP`、`IDSP`、`NBSP`、`FWSL` 依次是 U+FEFF、U+200B、U+3000、U+00A0、U+FF0F（按 §2.0 用十六进制写）。

| 编号 | 输入（Go 表达式） | 期望 | 钉住的是什么 |
|---|---|---|---|
| J1 | `"/new 真机复验\n/add opus-5.0,grok-4.7"` | true | Griffin 第一块（第 1 行是命令） |
| J2 | `"一句话\n/round rba_a rba_b --go"` | true | Griffin 第二块（第 1 行是正文） |
| J3 | `"a\nb\n/c"` | true | 第 3 行，不只看第 2 行 |
| J4 | `"a\n  /c"` | true | 行首空格 |
| J5 | `"a\n\t/c"` | true | 行首 tab |
| J6 | `"a\n" + IDSP + "/c"` | true | 行首全角空格（`TrimLeft(" \t")` 抓不到） |
| J7 | `"a\n" + BOM + "/c"` | true | 行首 BOM（`unicode.IsSpace` 抓不到） |
| J8 | `"a\n" + ZWSP + "/c"` | true | 行首零宽空格 |
| J9 | `"a\n\n/c"` | true | 中间夹空行（CRLF 粘贴的样子） |
| J10 | `"/say hi\n/round x"` | true | 第 1 行不只有 `/say`，不豁免 |
| J11 | `"/sayx\n/y"` | true | `/sayx` 不是 `/say` |
| J12 | `"/SAY\n/y"` | true | 大小写敏感 |
| J13 | `"a\n//c"` | true | 双斜杠也算 |
| J14 | `"a\n/"` | true | 光一个斜杠也算 |
| J15 | `"/new t\n/add x\n"` | true | 末尾换行不影响 |
| J16 | `"a\n" + NBSP + "/c"` | true | 行首不换行空格 |
| J17 | `"/say" + ZWSP + "\n/x"` | true | `/say` 后紧跟零宽字符：`Submit` 不认，不豁免 |
| J18 | `"a\n/b\nc"` | true | （v1.1）`/` 行后面还有正文：不只看最后一行 |
| J19 | `"a\nb\n/c\nd"` | true | （v1.1）`/` 行既不是第 2 行也不是最后一行：任意一行 |
| K1 | `"/new 真机复验"` | false | 单行 |
| K2 | `"a\nb"` | false | 长文 |
| K3 | `"一句话\nround rba_a rba_b --go"` | false | J2 去掉一个 `/` 的对照 |
| K4 | `"/new 真机复验\nadd opus-5.0,grok-4.7"` | false | J1 去掉第 2 行 `/` 的对照（第 1 行不参与判定） |
| K5 | `"a\nb/c"` | false | `/` 不在行首 |
| K6 | `"a\nhttp://x"` | false | 同上 |
| K7 | `"\n/new x"` | false | 前导空行不算第 1 行 |
| K8 | `BOM + "\n/new x"` | false | 整段按 `isTrimmable` 剥首（`TrimSpace` 剥不掉 BOM） |
| K9 | `"/say\n/round x\n/add y"` | false | 出路 |
| K10 | `"/say  \n/x"` | false | 出路：第 1 行行尾空格 |
| K11 | `"  /say\n/x"` | false | 出路：第 1 行行首空格（整段剥首） |
| K12 | `"/new x\n"` | false | 末尾换行剥掉后是单行 |
| K13 | `"a\n" + FWSL + "c"` | false | 全角斜杠不是 `/` |
| K14 | `""` | false | 空 |
| K15 | `"/say"` | false | 单行 `/say` |
| K16 | `"\t\n  \n"` | false | 只有空白 |
| K17 | `"/say" + NBSP + "\n/x"` | false | 出路：行尾不换行空格按空白去掉（`Submit` 也按它切词） |
| K18 | `"a\n\nb"` | false | （v1.1）中间夹空行的长文：空行不是命令行 |
| K19 | `"a\n   \nb"` | false | （v1.1）中间夹一行只有空格：剥完是空的行也不是命令行 |

【判别力】§4 的 X、Z 系列逐条点名到行。J 与 K 成对出现：J2/K3、J1/K4 只差一个 `/`；J17/K17 只差 `/say` 后面那个字符是不是空白；J9/K18 都是中间夹空行，只差第 3 行是不是 `/` 开头。J18、J19 抓「只查最后一行」（Z1）与「只查第 2 行或最后一行」（Z11，只有 J19 杀得到）；K18 抓「空行算命令」的两种写法（Z2、Z16），K19 只抓剥完再判空的那一种（Z2），所以两行都要。

**L2 `TestMlxSayExemptionAgreesWithSubmit`：出路与 `Submit` 的真实行为一致。** 每行一个子测试、各自一套装置：`g := rbaNewRig(t)`；`buf := &rbaBuf{}`；`r := g.newREPL(t, buf, buf.Reset)`；前提守卫 `g.svc.MaxSeq(g.ctx, g.convID) == 1`（只有种子消息）。先断言 `HasLaterCommandLine(输入) == 期望挡`；再 `r.Submit(g.ctx, 输入)` 返回 nil；`buf.String()` 整串等于期望输出；`MaxSeq` 等于期望；期望发出消息时 `g.svc.History(g.ctx, g.convID, 1, 10)` 恰好一条、`Text` 整串等于期望正文。

| 输入 | 期望挡 | 期望输出 | 之后 MaxSeq | 期望正文 |
|---|---|---|---|---|
| `"/say\n/round x\n/add y"` | false | `"已记下 seq 2\n"` | 2 | `"/round x\n/add y"` |
| `"/say  \n/x"` | false | `"已记下 seq 2\n"` | 2 | `"/x"` |
| `"  /say\n/x"` | false | `"已记下 seq 2\n"` | 2 | `"/x"` |
| `"/say" + NBSP + "\n/x"` | false | `"已记下 seq 2\n"` | 2 | `"/x"` |
| `"/say\n一句话\n/round rba_a rba_b --go"` | false | `"已记下 seq 2\n"` | 2 | `"一句话\n/round rba_a rba_b --go"` |
| `"/say" + ZWSP + "\n/x"` | true | `"未知命令，试 /help\n"` | 1 | （不发） |

【判别力】前五行担保「豁免的输入在 `Submit` 里真的是一条完整的消息」，最后一行担保「不豁免的那个边界在 `Submit` 里真的不是 `/say`」（若被豁免，TUI 会把它提交、清空输入框，而 `Submit` 什么都不发，内容丢失）。出路被删（X7）→ 前五行第一句红；豁免改成 `TrimFunc(isTrimmable)`（X14）→ 最后一行第一句红。`Submit` 的行为是 HEAD 的，这里只做交叉核对；将来 `Submit` 改了切词规则，这条会先红，提醒回来改出路。

### 2.2 `internal/tui/mlx_model_test.go`（纯 Model，不起 Program）

**公共装置（冻结）**：`sub := &tui1bSubmitter{}`；`pr := &tui1bPrinter{}`；`m := New(Options{Submit: sub, Out: tui1bNopDrainer{}, Version: "t", Println: pr.cmd, Now: func() time.Time { return rbaT0 }})`；`m, _ = tui1bStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})`。「粘贴 s」= `tui1bStep(t, m, tea.PasteMsg{Content: s})`；「回车」= `tui1bStep(t, m, tui1bEnter())`；「表头」= `m.chrome()[0]`；「值」= `m.ta.Value()`；「叶子」= `tui1bRun(t, cmd).log`（同时断言 `sawBatch == false`）。`j2` = `"一句话\n/round rba_a rba_b --go"`。

**M1 `TestMlxEnterBlocked`：挡下（每个子测试一个新 Model）。** 先把输入框弄成下表的样子，前提守卫「值」等于「前提值」、`m.frameHeight() == 8`；然后 `m, cmd := tui1bStep(t, m, tui1bEnter())`。断言：返回的 Cmd 为 nil；（v1.2）`tui1bRun(t, cmd)`（返回值不用；Cmd 为 nil 时它什么都不做），之后 `sub.count() == 0`；`pr.snapshot()` 长度 0；表头 == `HDR`；「值」仍等于前提值（逐字节）；`m.frameHeight() == 8`。（v1.1）最后再 `tui1bType(t, m, "z")`：「值」== 前提值 + `"z"`（挡下之后光标还在原处，打的字接在原文末尾）。

| 子测试 | 怎么放进输入框 | 前提值 |
|---|---|---|
| `griffin1` | 粘贴 `"/new 真机复验\n/add opus-5.0,grok-4.7"` | 同粘贴内容 |
| `griffin2` | 粘贴 `j2` | `j2` |
| `crlf` | 粘贴 `"a\r\n/c"` | `"a\n\n/c"` |
| `cr` | 粘贴 `"a\r/c"` | `"a\n/c"` |
| `tab` | 粘贴 `"a\n\t/c"` | `"a\n    /c"`（四个空格） |
| `line3` | 粘贴 `"a\nb\n/c"` | 同粘贴内容 |
| `middle` | （v1.1）粘贴 `"a\nb\n/c\nd"` | 同粘贴内容 |
| `trail` | （v1.1）粘贴 `"a\n/c\n"` | 同粘贴内容（末尾的换行留在输入框里，第 3 行是空行；帧高仍是 8，原型实测） |
| `bom` | 粘贴 `"a\n" + BOM + "/c"` | 同粘贴内容 |
| `keyboard` | `tui1bType(t, m, "a")`、`tui1bCtrl('j')`、`tui1bType(t, m, "  /b")` | `"a\n  /b"` |

【判别力】门被删（Y1）→ Cmd 非 nil、跑 Cmd 之后 Submit 被调、值被清空；只改 status 不返回（Y3）→ 表头是 `正在运行`；挡下时 `Reset`（Y4）→ 值红；挡下时回显（Y11）→ Cmd 非 nil、`pr` 有记录（count 仍是 0：只回显不提交）；照样起 submitCmd（Y12）→ Cmd 非 nil、跑 Cmd 之后 count 是 1（v1.2 原型实测；不先跑 Cmd 的话 count 那句在任何变异下都是绿的，W1b 发现 1）；置忙（Y9）→ 表头多出 `  0s`；措辞改了（Y10）→ 表头红。（v1.1）`middle` 抓只查最后一行（Z1）与只查第 2 行或最后一行（Z11）；`trail` 抓门里把 `text`（已去掉末尾换行）写回输入框（Z3：回车之后值就成了 `"a\n/c"`，打 z 之后是 `"a\n/cz"`，原型实测）；「打 z」那一句抓门里把光标往回挪（原型实测：Z12 `MoveToBegin` 的 z 跑到整段最前面；Z13 `CursorStart`、Z14 `SetCursorColumn(0)` 的 z 跑到末行行首，例如 `griffin2` 打出 `"一句话\nz/round rba_a rba_b --go"`；Z13、Z14 在 `trail` 上看不见，光标本来就在空的末行行首）。往末尾挪（Z9、Z10）在 M1 上看不见（粘贴之后光标本来就在末尾），由 M8 `block` 抓。前提值那一列同时钉住 §0 第 1 条的 sanitizer 行为（`\r`→换行、tab→四空格、BOM 保留）：升级 bubbles 后它变了，前提守卫先红。对照：M2（同一装置，放行；`ctl1`/`ctl2` 与 `griffin2`/`griffin1` 只差一个 `/`，回车之后 count 是 1）。

**M2 `TestMlxEnterPasses`：放行（每个子测试一个新 Model）。** 粘贴、回车，跑叶子。断言：叶子整列等于下表；`sub.count() == 1`；`sub.text(0)` 等于「Submit 收到」；表头 == `"council t   正在运行  0s"`。

| 子测试 | 粘贴 | Submit 收到 | 叶子 |
|---|---|---|---|
| `ctl1` | `"一句话\nround rba_a rba_b --go"` | 同粘贴 | `["print:> 一句话", "print:> round rba_a rba_b --go", "submitDone"]` |
| `ctl2` | `"/new 真机复验\nadd opus-5.0,grok-4.7"` | 同粘贴 | `["print:> /new 真机复验", "print:> add opus-5.0,grok-4.7", "submitDone"]` |
| `leadblank` | `"\n/new x"` | `"\n/new x"` | `["print:> ", "print:> /new x", "submitDone"]` |
| `bomline` | `BOM + "\n/new x"` | `BOM + "\n/new x"` | `["print:> " + BOM, "print:> /new x", "submitDone"]` |
| `say` | `"/say\n/round x"` | 同粘贴 | `["print:> /say", "print:> /round x", "submitDone"]` |
| `trail` | `"/new x\n"` | `"/new x"` | `["print:> /new x", "submitDone"]` |
| `para` | （v1.1）`"a\r\n\r\nb"` | `"a\n\n\n\nb"` | `["print:> a", "print:> ", "print:> ", "print:> ", "print:> b", "submitDone"]` |

【判别力】`ctl1`/`ctl2` 与 M1 `griffin2`/`griffin1` 只差一个 `/`；`leadblank`、`bomline` 抓「整段不剥首」「用 TrimSpace 剥首」（X5、X6）；`say` 抓出路被删（X7）；`ctl2` 抓「连第 1 行一起判」（X12）；（v1.1）`para` 抓「空行算命令行」的两种写法（Z2、Z16），走的是真 sanitizer：CRLF 粘贴里每个 `\r`、`\n` 各变一个换行，段落之间的一个空行成了三个（与 M1 `crlf` 同一条 §0 第 1 条的机制；原型实测值与叶子如表）。回显与 Submit 收到的文字与 HEAD 相同（HEAD 上本组全绿，含 `para`，§2.0）。

**M3 `TestMlxHintPersistsAndSayEscape`：提示持续到哪里、出路走通。** 一个 Model，逐步断言：

| 步 | 动作 | 断言 |
|---|---|---|
| 1 | 粘贴 `j2`；回车 | 表头 `HDR`；值 `j2` |
| 2 | `tui1bType(t, m, "z")` | 表头 `HDR`；值 `j2 + "z"` |
| 3 | `tea.KeyPressMsg{Code: tea.KeyBackspace}` | 表头 `HDR`；值 `j2` |
| 4 | `tea.WindowSizeMsg{Width: 100, Height: 24}` | 表头 `HDR` |
| 5 | `elapsedTickMsg{gen: m.tickGen, at: rbaT0.Add(3 * time.Second)}` | Cmd 为 nil；表头 `HDR` |
| 6 | 回车；（v1.2）`tui1bRun(t, cmd)`（返回值不用，Cmd 为 nil 时什么都不做） | Cmd 为 nil；表头 `HDR`；跑 Cmd 之后 `sub.count() == 0`；值 `j2` |
| 7 | （v1.1）`tui1bEsc()` | Cmd 为 nil；表头 `HDR`；值 `j2` |
| 8 | （v1.1）`tea.FocusMsg{}`（走 `updateInput` 最外层 default，交给 textarea；返回的 Cmd 不断言） | 表头 `HDR`；值 `j2` |
| 9 | `tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}`；`tui1bType(t, m, "/say")`；`tui1bCtrl('j')` | 表头 `HDR`；值 `"/say\n" + j2` |
| 10 | 回车，跑叶子 | 叶子 `["print:> /say", "print:> 一句话", "print:> /round rba_a rba_b --go", "submitDone"]`；`sub.text(0) == "/say\n" + j2`；表头 `"council t   正在运行  0s"`；值 `""` |

v1.1 在第 6 步之后插了第 7、8 步，v1 的第 7、8 步顺延为第 9、10 步；第 1-6 步不变。第 8 步的 `FocusMsg` 在 `model.go` 里没有自己的分支，落进 `updateInput` 最外层 default（`:381-384`）交给 textarea；textarea 只把它转给自己的光标（bubbles v2.2.1 `textarea.go:1462` → `cursor.go:126`），不改值，所以值那一句恒为 `j2`，这一步只为表头那一句。

【判别力】打字、窗口尺寸、tick 任何一处清掉提示（Y13、Y14、Y15）→ 第 2、4、5 步红（第 5 步的 tick 代次取当前值，保证「清提示」写在 `!busy` 门之前时可观测）；第 6 步是幂等，挡下时照样起 submitCmd（Y12）→ 第 6 步的 Cmd 与 count 两句红（v1.2 原型实测；count 那句的对照是第 10 步：同一个 Model，真的提交）；（v1.1）Esc 清提示（Z7）→ 第 7 步红；外层 default 清提示（Z15）→ 第 8 步红；Ctrl-J 清提示（Z6）→ 第 9 步红；第 9-10 步证明出路可用、且挡下时文字一个字节没丢（发出去的正是 `"/say\n" + j2`）。有的变异在后面几步也红（原型实测：Y13 红在第 2、3、4、5、9 步，第 6 步的回车又把提示写了回来，第 6-8 步绿），点名只写最早那一步。

**M4 `TestMlxPasteReplacesHint`。** 粘贴 `j2`、回车（表头 `HDR`，前提），再粘贴 `"z"`：表头 == `"council t   粘贴了 1 个字符"`；值 == `j2 + "z"`。【判别力】钉住 D6「粘贴改写提示」这一条（既有 `:378`），同时说明挡下之后再粘贴会接在原文后面（值那一句）。

**M5 `TestMlxBusyEnterUnchanged`：忙时的门在前。** `tui1bType(t, m, "x")`；回车并跑叶子（`sub.count() == 1`，前提）；粘贴 `j2`；回车：Cmd 为 nil、表头 == `"council t   一轮正在跑，先等它回来或按 Ctrl-C 取消  0s"`、值 `j2`；喂 `submitDoneMsg{}`：表头 == `"council t   空闲"`；再回车：Cmd 为 nil；（v1.2）`tui1bRun(t, cmd)`（返回值不用）之后 `sub.count() == 1`；表头 `HDR`；值 `j2`。
【判别力】门挪到 Enter 分支最前面（Y7）→ 忙时表头是提示而不是忙时那句，红。后半段证明忙时回车没吞掉文字，空闲后照样挡；挡下时照样起 submitCmd（Y12）→ 最后一次回车的 Cmd 与 count（`got 2 want 1`）两句红（v1.2 原型实测；count 的对照是开头那次真的提交）。

**M6 `TestMlxFinalizeWindowUnchanged`：收口窗口的门在前。** `tui1bType(t, m, "x")`；回车（丢掉 Cmd，不跑）；喂 `tea.WindowSizeMsg{Width: 100, Height: 24}`（吃掉待装的 tick）；喂 `roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 2, Speakers: []string{"a", "b"}}}`；喂 `submitDoneMsg{}`，跑它的叶子（前提：恰好 `["print:/round 结束  0s  回来 0/2", "other:tui.boardFinalizeMsg"]`；**不把** `boardFinalizeMsg` 喂回去，于是处在收口窗口里）；粘贴 `j2`：表头 == `"council t   粘贴了 27 个字符"`、`m.frameHeight() == 10`；回车：Cmd 为 nil、表头仍是 `"council t   粘贴了 27 个字符"`、值 `j2`；喂 `boardFinalizeMsg{}`：`m.frameHeight() == 8`；回车：Cmd 为 nil；（v1.2）`tui1bRun(t, cmd)`（返回值不用）之后 `sub.count() == 0`（开头那次回车的 Cmd 丢掉没跑，所以是 0）；表头 `HDR`；值 `j2`。
【判别力】门放在 busy 判断之后、finalizing 判断之前（Y8）或最前面（Y7）→ 窗口里的回车打出提示，红。挡下时照样起 submitCmd（Y12）→ 最后一次回车的 Cmd 与 count 两句红（v1.2 原型实测）。

**M7 `TestMlxIdleCtrlCQuitsAfterBlock`：挡下之后空闲 Ctrl-C 照旧退出（v1.1，F3）。** 粘贴 `j2`；回车；前提守卫表头 == `HDR`（`Fatalf`）。然后 `m, cmd := tui1bStep(t, m, tui1bCtrl('c'))`：表头 == `"council t   正在退出"`；`tr := tui1bRun(t, cmd)`，`tr.hasQuit()` 为真；`tr.sawBatch == false`。
【判别力】提示挂着时空闲 Ctrl-C 只清提示、不退出（Z4）→ 表头是 `council t   空闲`、叶子是空的，两句都红（原型实测）。不能并进 M3：M3 第 10 步之后忙，Ctrl-C 走 `正在取消`；插在第 1-9 步中间又会让程序退出、后面各步的表头不再是提示。HEAD 上红在前提那一句（回车照样提交，表头是 `正在运行  0s`）。

**M8 `TestMlxBlockKeepsCursor`：挡下时光标不动（v1.1，F2）。** v1.2 拆成两个子测试，各自一个新 Model：

- `block`：粘贴 `j2`；`tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}`；`tea.KeyPressMsg{Code: tea.KeyRight}`；前提守卫「值」== `j2`（`Fatalf`）。`m, cmd := tui1bStep(t, m, tui1bEnter())`：Cmd 为 nil；（v1.2）`tui1bRun(t, cmd)`（返回值不用；Cmd 为 nil 时什么都不做）之后 `sub.count() == 0`；表头 == `HDR`；「值」== `j2`。再 `tui1bType(t, m, "z")`：「值」== `"一z句话\n/round rba_a rba_b --go"`（原型实测）。
- `control`（v1.2，W1b 发现 1）：粘贴 `"一句话\nround rba_a rba_b --go"`（L1 K3，与 `j2` 只差一个 `/`）；同样 Ctrl-Home、右移；前提守卫「值」== 粘贴内容（`Fatalf`）。`m, cmd := tui1bStep(t, m, tui1bEnter())`：Cmd 非 nil（`Fatalf`）；叶子 == `["print:> 一句话", "print:> round rba_a rba_b --go", "submitDone"]`（`sawBatch == false`）；`sub.count() == 1`；`sub.text(0) == "一句话\nround rba_a rba_b --go"`；表头 == `"council t   正在运行  0s"`；「值」== `""`（v1.2 原型实测）。

【判别力】光标停在第 1 行第 1 个字之后，离开头、离行尾、离整段末尾都有距离，门里往任何一头挪光标都看得见（原型实测）：写回 `text`（Z3）与挪到整段末尾（Z10）→ `"一句话\n/round rba_a rba_b --goz"`；挪到本行末尾（Z9）→ `"一句话z\n/round rba_a rba_b --go"`；挪到整段开头（Z12）或本行行首（Z13、Z14）→ `"z一句话\n/round rba_a rba_b --go"`。`block` 的缺席断言（Cmd 为 nil、跑 Cmd 之后 count 为 0、值不变）的对照是 `control`：同一装置、同样的按键，只差一个 `/`，Cmd 非 nil、count 为 1、值被清空、表头换成 `正在运行`。挡下时照样起 submitCmd（Y12）→ `block` 的 Cmd 与 count 两句红；挡下时回显（Y11）→ `block` 的 Cmd 红（v1.2 原型实测）。HEAD 上 `block` 五句全红、`control` 绿（§2.0）。`control` 在 §4 的 48 条变异下全部绿，它只做对照。

**M9 `TestMlxHintStaysAfterFix`：删掉犯规的行之后提示仍挂着，回车照常提交（v1.2，W1b 发现 2）。** 一个 Model，逐步断言：

| 步 | 动作 | 断言 |
|---|---|---|
| 1 | 粘贴 `"a\n/c"`；回车 | 前提守卫：表头 `HDR`（`Fatalf`） |
| 2 | `tea.KeyPressMsg{Code: tea.KeyBackspace}` 两次 | 表头 `HDR`；值 `"a\n"` |
| 3 | `tui1bEsc()` | Cmd 为 nil；表头 `HDR`；值 `"a\n"` |
| 4 | `tea.WindowSizeMsg{Width: 100, Height: 24}` | 表头 `HDR` |
| 5 | `elapsedTickMsg{gen: m.tickGen, at: rbaT0.Add(3 * time.Second)}` | Cmd 为 nil；表头 `HDR` |
| 6 | `tea.FocusMsg{}`（返回的 Cmd 不断言） | 表头 `HDR`；值 `"a\n"` |
| 7 | 回车，跑叶子 | 叶子 `["print:> a", "submitDone"]`；`sub.count() == 1`；`sub.text(0) == "a"`；表头 `"council t   正在运行  0s"`；值 `""` |

第 2 步第一次退格删掉 `c`（值 `"a\n/"` 仍犯规，L1 J14），第二次删掉 `/`，值 `"a\n"` 不再犯规（去掉末尾换行后是单行）。第 3-6 步是同一个状态下 D6 列的其它不清提示的端点，与 M3 第 7、4、5、8 步同样的消息。以上期望值都是 v1.2 原型的实跑输出。
【判别力】按键之后值不再犯规就清提示（Z17）→ 第 2 步表头红；同一写法放在 Esc（Z18）、窗口尺寸（Z19）、tick（Z20）、`updateInput` 最外层 default（Z21）→ 第 3、4、5、6 步表头红（v1.2 原型实测，各自最早红的就是这一步）。无条件清提示的 Y13、Y14、Y15、Z7、Z15 也在对应那一步红。M3 杀不到 Z17-Z21：M3 第 2-8 步值里一直有 `/round` 那一行。第 5 步 tick 的代次取当前值，理由同 M3 第 5 步。第 7 步证明删掉犯规的行之后回车能正常提交（发出去的是 `"a"`：`beginSubmit` 去掉了末尾换行），同时是第 2-6 步「表头仍是提示」的对照：同一个 Model、同一个值，回车之后表头换成 `正在运行`。挡下时 `Reset`（Y4）、门里把光标往回挪（Z12、Z13、Z14）→ 第 2 步的值红。HEAD 上红在前提那一句（回车照样提交，表头是 `正在运行  0s`）。

### 2.3 `internal/tui/mlx_program_test.go`（真 `tea.Program`、真 REPL、真 Runner、假 CLI）

**装置（冻结）**：
- `g := rbaNewTuiRig(t)`。
- 计数 Submitter（mlx 自己的类型）：包住 `replAdapter{r: g.r}`，`Submit` 先在锁里记下收到的文字、再委托；`IsQuit` 委托。
- mlx spy（照 RB-A `rbaSpy` 的形状写 mlx 自己的一份，不复用 `rbaSpy`：它不记按键）：`Init`、`View` 委托；`Update` 先委托、把返回值断回 `Model` 存回、再按消息记一条日志、**返回 spy 自己**。日志种类：`KEY`（`tea.KeyPressMsg`，文本 = `msg.String()`，回车是 `"enter"`）、`PASTE`、`DONE`（`submitDoneMsg`）、`RS`（`roundStartMsg`）、`P`（printLine，正文取法同 RB-A：`fmt.Sprintf("%T", msg) == "tea.printLineMessage"` 且 `reflect.ValueOf(msg).FieldByName("messageBody").String()`）；其余消息不记。每条同时记下 Update 之后的 `chrome()[0]` 与 `ta.Value()`。日志加锁。
- `ctx, cancel := context.WithCancel(context.Background())`；`m := New(Options{Submit: 计数 Submitter, Out: g.lw, Version: "t", Ctx: ctx})`；`p := tea.NewProgram(spy, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignalHandler(), tea.WithWindowSize(100, 24))`；先 `g.r.SetRoundObserver(newRoundObserver(p.Send))`，再 `g.lw.SetEmit(func(line string) { p.Send(printLine(line)) })`，再 `go p.Run()`；`t.Cleanup` 里 cancel 并等 Run 返回（5 秒超时）。
- 「提交 s」= `p.Send(tea.PasteMsg{Content: s})` 再 `p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})`。先提交 `"/use " + g.convID`，等日志里有 1 条 `DONE`（10 秒，前提）。

**P1 `TestMlxProgramBlocksLaterCommand`**，两个子测试，各自一套装置，只差粘贴内容里的一个 `/`：

- `reject`：提交 `j2`；等日志里有 2 条 `KEY "enter"`（5 秒，前提）。第 2 条 `KEY "enter"` 那条日志的表头 == `HDR`、值 == `j2`。然后依次 Send：`tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}`；`/`、`s`、`a`、`y` 四个 `tea.KeyPressMsg{Code: r, Text: string(r)}`；`tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}`；`tea.KeyPressMsg{Code: tea.KeyEnter}`。等日志里有 2 条 `DONE`（10 秒，前提），再等 300ms。断言：
  1. 计数 Submitter 收到的文字整列 == `["/use " + g.convID, "/say\n" + j2]`。
  2. 第 2 条 `KEY "enter"` 之后、第 3 条 `KEY "enter"` 之前，没有任何 `P`。
  3. 第 3 条 `KEY "enter"` 之后、下一条 `DONE` 之前的 `P` 正文整列 == `["> /say", "> 一句话", "> /round rba_a rba_b --go", "已记下 seq 2"]`。
  4. 全部日志里 `RS` 0 条、`DONE` 恰好 2 条；最后一条日志是 `DONE`，它的表头 == `"council t   空闲"`。
  5. `g.svc.History(context.Background(), g.convID, 0, 100)` 恰好 2 条：seq 1 `"seed-msg"`、seq 2 的 `Text` == `j2`。
- `control`：提交 `"一句话\nround rba_a rba_b --go"`；等日志里有 2 条 `DONE`（10 秒，前提），再等 300ms。断言：
  1. 收到的文字整列 == `["/use " + g.convID, "一句话\nround rba_a rba_b --go"]`。
  3. 第 2 条 `KEY "enter"` 之后、下一条 `DONE` 之前的 `P` 正文整列 == `["> 一句话", "> round rba_a rba_b --go", "已记下 seq 2"]`。
  4. 同 `reject` 第 4 条。
  5. seq 2 的 `Text` == `"一句话\nround rba_a rba_b --go"`，共 2 条。

【判别力】这是跨并发原语的那一条：回车经真事件循环、Submit 在 Cmd goroutine 上、REPL 的行经 `p.Send` 回来。`reject` 第 2 条是缺席断言，对照是 `control` 第 3 条（同一装置，只差一个 `/`，回车之后立刻有回显与「已记下」）。门被删（Y1）或挡下时照样起 submitCmd（Y12）→ 第 1 条多出一条、第 5 条多出一条消息；挡下时回显（Y11）→ 第 2 条红；出路被删（X7）→ 第 3 步那次回车又被挡，等不到第 2 条 `DONE`，前提不成立而红。

### 2.4 `internal/tui/mlx_run_test.go`（真 `tui.Run`，`os.Pipe` 做 in 与 stdout，括号粘贴字节）

**装置（冻结，形状照 `rba_wiring_test.go`）**：`g := rbaNewTuiRig(t)`；`inR, inW := os.Pipe()`、`outR, outW := os.Pipe()`；读 goroutine 在 Run 之前起（`io.Copy` 进 `&tui1dBuf{}`）；`go func() { runDone <- Run(ctx, g.r, g.lw, "test", inR, outW) }()`；收尾顺序照 `rba_wiring_test.go:63-85`（cancel、等 Run 5 秒、关 inW、inR、outW、等读 goroutine 5 秒、关 outR），挂进 `t.Cleanup`（幂等）。「flat」= `rbaFlat(buf.String())`（剥 ANSI、删 ASCII 空格）。`HF` = `HDR` 的版本换成 `test` 再删空格，即 `"counciltest一次只能一条命令，逐条回车；整段当消息发，首行只写/say"`。
**同步点（每个子测试都走）**：原始字节出现 `tui1dESC + "[J"`（5 秒）→ 写 `tui1dESC + "[8;24;100t"` → flat 含 `counciltest空闲`（5 秒）→ 写 `"/use " + g.convID + "\r"` → flat 含 `当前会话:`（5 秒）→ 写 `tui1dESC + "[200~" + 粘贴内容 + tui1dESC + "[201~"` → 写 `"\r"`。以上任何一步等不到都是前提不成立（`Fatalf`）。
「出路按键」= 写 `tui1dESC + "[1;5H" + "/say" + "\n" + "\r"`（Ctrl-Home、四个字、LF 字节解成 Ctrl-J、CR 解成回车；§0 第 1 条）。

**R1 `TestMlxRunPipeBlocksLaterCommand`**，四个子测试，各自一套装置：

| 子测试 | 粘贴内容（括号粘贴里的原始字节） | 之后 | 断言 |
|---|---|---|---|
| `reject_cr` | `"一句话\r/round rba_a rba_b --go"`（CR 分行，Windows Terminal 的样子） | flat 含 `HF`（5 秒，**主断言**）；写出路按键；flat 含 `已记下seq2`（5 秒）；再等 200ms | `History(…, 0, 100)` 恰好 2 条、seq 2 的 `Text` == `"一句话\n/round rba_a rba_b --go"`；`Conversations` 恰好 1 个 |
| `control_cr` | `"一句话\rround rba_a rba_b --go"` | flat 含 `已记下seq2`（5 秒）；再等 200ms | seq 2 的 `Text` == `"一句话\nround rba_a rba_b --go"`、共 2 条；`Conversations` 1 个；flat **不含** `HF` |
| `reject_new` | `"/new 真机复验\n/add rba_a"`（LF 分行） | flat 含 `HF`（5 秒，主断言）；写出路按键；flat 含 `已记下seq2`（5 秒）；再等 200ms | seq 2 的 `Text` == `"/new 真机复验\n/add rba_a"`、共 2 条；`Conversations` 恰好 1 个（`/new` 没执行） |
| `control_new` | `"/new 真机复验\nadd rba_a"` | flat 含 `已建会话`（5 秒）；再等 200ms | `Conversations` 恰好 2 个、其中一个 `Title` == `"真机复验\nadd rba_a"`；`History` 只有 1 条（种子）；flat **不含** `HF` |

【为什么是子串】终端字节流，同 RB-A W1。【判别力】这条走的是真输入解码：括号粘贴 → `PasteMsg` → sanitizer 把 CR 变换行 → 回车 → 门。门被删（Y1）→ `reject_*` 等不到 `HF`；措辞改了（Y10）→ 同上。`reject_new` 的「会话仍是 1 个」是缺席断言，对照是 `control_new`（只差一个 `/`，同一装置里 `/new` 真的建出了第 2 个会话，标题就是 #44 里 Griffin 看到的那种）；`control_*` 的「不含 `HF`」是缺席断言，对照是同表的 `reject_*`。`reject_*` 发出去的正文与粘贴内容（换行归一之后）逐字节相同，证明挡下时一个字节没丢。

---

## 3. 冻结的字面量

### 3.1 本刀新增的用户可见字面量（只有这一条）

```
一次只能一条命令，逐条回车；整段当消息发，首行只写 /say
```

- UTF-8 80 字节，逐字节（十六进制）：`e4 b8 80 e6 ac a1 e5 8f aa e8 83 bd e4 b8 80 e6 9d a1 e5 91 bd e4 bb a4 ef bc 8c e9 80 90 e6 9d a1 e5 9b 9e e8 bd a6 ef bc 9b e6 95 b4 e6 ae b5 e5 bd 93 e6 b6 88 e6 81 af e5 8f 91 ef bc 8c e9 a6 96 e8 a1 8c e5 8f aa e5 86 99 20 2f 73 61 79`。
- 标点：`，` 是 U+FF0C，`；` 是 U+FF1B；`/say` 前面是一个 ASCII 空格；行尾没有空格、没有句号。
- 显示宽度 55 列。不忙时表头 = `"council " + 版本 + "   " + 这一串`（例：测试里 `council t   一次只能一条命令，逐条回车；整段当消息发，首行只写 /say`）。

### 3.2 测试要比对的既有文本（不是本刀新增，HEAD 原样）

`空闲`、`正在运行`、`粘贴了 %d 个字符`（`model.go:378`）、`一轮正在跑，先等它回来或按 Ctrl-C 取消`（`:358`）、`/round 结束  0s  回来 0/2`（RB-B 收尾行）、`已记下 seq %d`（`commands.go:338`）、`未知命令，试 /help`（`repl.go:42`）、`当前会话: <id>，标题: <title>`、`已建会话 <id>，标题: <title>`。

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；主代理落地后逐条跑（锚点恰好一次、从字节副本还原，不用 `git checkout`）。记号：`<NL>` 是换行，`<TAB>` 是一个 tab；「→」左边是原文，右边是变异后。每条都已在原型上实跑，点名杀手实测变红。

| # | 文件 | 变异（可编译） | 预期被谁杀 |
|---|---|---|---|
| X1 | latercmd.go | `range lines[1:] {` → `range lines[1:2] {` | L1 J3、J9、J19；M1 `line3`、`crlf`、`middle` |
| X2 | latercmd.go | `strings.HasPrefix(strings.TrimLeftFunc(line, isTrimmable), "/")` → `strings.HasPrefix(line, "/")` | L1 J4-J8、J16；M1 `tab`、`bom`、`keyboard` |
| X3 | latercmd.go | 同上原文 → `strings.HasPrefix(strings.TrimSpace(line), "/")` | L1 J7、J8；M1 `bom` |
| X4 | latercmd.go | 同上原文 → `strings.HasPrefix(strings.TrimLeft(line, " \t"), "/")` | L1 J6、J7、J8、J16；M1 `bom` |
| X5 | latercmd.go | `strings.Split(strings.TrimFunc(text, isTrimmable), "\n")` → `strings.Split(text, "\n")` | L1 K7、K8；M2 `leadblank`、`bomline`（v1.1 去掉 K11：X5 下 K11 仍是 false，行首空格由 `TrimSpace(lines[0])` 剥掉） |
| X6 | latercmd.go | 同上原文 → `strings.Split(strings.TrimSpace(text), "\n")` | L1 K8；M2 `bomline` |
| X7 | latercmd.go | `if strings.TrimSpace(lines[0]) == "/say" {` → `if false {` | L1 K9、K10、K11、K17；L2 前五行；M2 `say`；M3；P1 `reject`；R1 `reject_cr`、`reject_new` |
| X8 | latercmd.go | 同上原文 → `if strings.HasPrefix(lines[0], "/say") {` | L1 J10、J11 |
| X9 | latercmd.go | 同上原文 → `if strings.EqualFold(strings.TrimSpace(lines[0]), "/say") {` | L1 J12 |
| X10 | latercmd.go | 同上原文 → `if lines[0] == "/say" {` | L1 K10、K17 |
| X11 | latercmd.go | `strings.HasPrefix(strings.TrimLeftFunc(line, isTrimmable), "/")` → `strings.Contains(line, "/")` | L1 K5、K6 |
| X12 | latercmd.go | `range lines[1:] {` → `range lines {` | L1 K4；M2 `ctl2`；R1 `control_new` |
| X13 | latercmd.go | `if len(lines) < 2 {` → `if len(lines) < 3 {` | L1 J1、J2 等全部两行的 J；M1（除 `line3`、`crlf`、`middle`）；M3-M7、M8 `block`、M9；P1 `reject`；R1 `reject_*` |
| X14 | latercmd.go | `if strings.TrimSpace(lines[0]) == "/say" {` → `if strings.TrimFunc(lines[0], isTrimmable) == "/say" {` | L1 J17；L2 最后一行 |
| Y1 | model.go | `if repl.HasLaterCommandLine(text) {` → `if false {` | M1 全部（v1.2：跑 Cmd 之后 count 也红）；M3-M7、M8 `block`、M9；P1 `reject`；R1 `reject_*` |
| Y2 | model.go | `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, nil` → `<TAB><TAB>return m, nil` | M1 全部（表头）；M3-M7、M8 `block`、M9；P1 `reject`；R1 `reject_*` |
| Y3 | model.go | `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, nil<NL><TAB>}` → `<TAB><TAB>m.status = oneCommandHint<NL><TAB>}`（不返回，落进正常提交） | M1 全部（v1.2：跑 Cmd 之后 count 也红）；M3-M7、M8 `block`、M9；P1 `reject`；R1 `reject_*` |
| Y4 | model.go | `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, nil` → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>m.ta.Reset()<NL><TAB><TAB>return m, nil` | M1 全部（值）；M3、M4、M5、M6、M8 `block`、M9 第 2 步；P1 `reject`；R1 `reject_*` |
| Y7 | model.go | `<TAB><TAB>case msg.Code == tea.KeyEnter:<NL><TAB><TAB><TAB>if m.busy {` → `<TAB><TAB>case msg.Code == tea.KeyEnter:<NL><TAB><TAB><TAB>if repl.HasLaterCommandLine(m.ta.Value()) {<NL><TAB><TAB><TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB><TAB><TAB>return m, nil<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>if m.busy {`（门挪到 Enter 分支最前面） | M5；M6 |
| Y8 | model.go | `<TAB><TAB><TAB>if m.finalizing {<NL><TAB><TAB><TAB><TAB>return m, nil<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>return m.beginSubmit()` → `<TAB><TAB><TAB>if repl.HasLaterCommandLine(m.ta.Value()) {<NL><TAB><TAB><TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB><TAB><TAB>return m, nil<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>if m.finalizing {<NL><TAB><TAB><TAB><TAB>return m, nil<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>return m.beginSubmit()`（门挪到 busy 之后、finalizing 之前） | M6 |
| Y9 | model.go | `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, nil` → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>m.busy = true<NL><TAB><TAB>return m, nil` | M1 全部（表头多 `  0s`）；M3-M7、M8 `block`、M9；P1 `reject`；R1 `reject_*` |
| Y10 | model.go | `const oneCommandHint = "一次只能一条命令，逐条回车；整段当消息发，首行只写 /say"` → `const oneCommandHint = "一次只能一条命令，没提交：请逐条回车"` | M1 全部；M3-M7、M8 `block`、M9；P1 `reject`；R1 `reject_*` |
| Y11 | model.go | `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, nil` → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, m.println("> " + text)` | M1 全部（Cmd 非 nil、`pr` 有记录；count 不红：只回显不提交）；M3 第 6 步；M5、M6 最后一次回车（Cmd）；M8 `block`（Cmd）；P1 `reject` 第 2 条 |
| Y12 | model.go | `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, nil` → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, submitCmd(m.sub, m.ctx, text)` | M1 全部（Cmd；v1.2：跑 Cmd 之后 count 也红）；M3 第 6 步（Cmd、count）；M5、M6 最后一次回车（Cmd、count）；M8 `block`（Cmd、count）；P1 `reject` 第 1、5 条；R1 `reject_*`（多出一条消息） |
| Y13 | model.go | `<TAB><TAB>default:<NL><TAB><TAB><TAB>var cmd tea.Cmd<NL><TAB><TAB><TAB>m.ta, cmd = m.ta.Update(msg)<NL><TAB><TAB><TAB>return m, cmd<NL><TAB><TAB>}` → `<TAB><TAB>default:<NL><TAB><TAB><TAB>if m.status == oneCommandHint {<NL><TAB><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>var cmd tea.Cmd<NL><TAB><TAB><TAB>m.ta, cmd = m.ta.Update(msg)<NL><TAB><TAB><TAB>return m, cmd<NL><TAB><TAB>}`（KeyPress 的 default 分支，`:369-373`；锚点在文件里恰好一次） | M3 第 2 步；M9 第 2 步 |
| Y14 | model.go | `<TAB><TAB>m.width = width<NL><TAB><TAB>m.height = height` → `<TAB><TAB>m.width = width<NL><TAB><TAB>m.height = height<NL><TAB><TAB>if m.status == oneCommandHint {<NL><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB>}` | M3 第 4 步；M9 第 4 步 |
| Y15 | model.go | `<TAB>case elapsedTickMsg:<NL>` → `<TAB>case elapsedTickMsg:<NL><TAB><TAB>if m.status == oneCommandHint {<NL><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB>}<NL>`（插在 `if !m.busy` 之前） | M3 第 5 步；M9 第 5 步 |
| Z1 | latercmd.go | （v1.1，F0）`range lines[1:] {` → `range lines[len(lines)-1:] {`（只查最后一行） | L1 J18、J19；M1 `middle` |
| Z11 | latercmd.go | （v1.1，F0；V0 称 Z2）`for _, line := range lines[1:] {` → `for _, line := range []string{lines[1], lines[len(lines)-1]} {`（只查第 2 行或最后一行） | L1 J19；M1 `middle` |
| Z2 | latercmd.go | （v1.1，F1）`if strings.HasPrefix(strings.TrimLeftFunc(line, isTrimmable), "/") {` → `if tl := strings.TrimLeftFunc(line, isTrimmable); tl == "" \|\| strings.HasPrefix(tl, "/") {`（剥完是空的行算命令行） | L1 K18、K19；M2 `para` |
| Z16 | latercmd.go | （v1.1，F1）同上原文 → `if line == "" \|\| strings.HasPrefix(strings.TrimLeftFunc(line, isTrimmable), "/") {`（原样是空串的行算命令行） | L1 K18；M2 `para` |
| Z3 | model.go | （v1.1，F2）`<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>return m, nil` → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>m.ta.SetValue(text)<NL><TAB><TAB>return m, nil`（把去掉末尾换行的 `text` 写回输入框） | M1 `trail`；M8 `block` |
| Z9 | model.go | （v1.1，F2）同上原文 → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>m.ta.CursorEnd()<NL><TAB><TAB>return m, nil` | M8 `block` |
| Z10 | model.go | （v1.1，F2）同上原文 → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>m.ta.MoveToEnd()<NL><TAB><TAB>return m, nil` | M8 `block` |
| Z12 | model.go | （v1.1，F2）同上原文 → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>m.ta.MoveToBegin()<NL><TAB><TAB>return m, nil` | M8 `block`；M1 全部（打 z）；M3 第 2 步；M4；M9 第 2 步 |
| Z13 | model.go | （v1.1，F2）同上原文 → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>m.ta.CursorStart()<NL><TAB><TAB>return m, nil` | M8 `block`；M1 除 `trail`（打 z）；M3 第 2 步；M4；M9 第 2 步 |
| Z14 | model.go | （v1.1，F2）同上原文 → `<TAB><TAB>m.status = oneCommandHint<NL><TAB><TAB>m.ta.SetCursorColumn(0)<NL><TAB><TAB>return m, nil` | M8 `block`；M1 除 `trail`（打 z）；M3 第 2 步；M4；M9 第 2 步 |
| Z4 | model.go | （v1.1，F3）`<TAB><TAB><TAB>}<NL><TAB><TAB><TAB>m.status = "正在退出"<NL>` → `<TAB><TAB><TAB>}<NL><TAB><TAB><TAB>if m.status == oneCommandHint {<NL><TAB><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB><TAB><TAB>return m, nil<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>m.status = "正在退出"<NL>`（空闲 Ctrl-C 在提示挂着时只清提示、不退出。锚点带上前一行忙时分支收尾的 `}`：不带的话 `submitDoneMsg` 的 IsQuit 分支里也有同样的「`m.status = "正在退出"`」一行，命中两次） | M7 |
| Z7 | model.go | （v1.1，F3）`<TAB><TAB>case msg.Code == tea.KeyEscape:<NL><TAB><TAB><TAB>return m, nil` → `<TAB><TAB>case msg.Code == tea.KeyEscape:<NL><TAB><TAB><TAB>if m.status == oneCommandHint {<NL><TAB><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>return m, nil` | M3 第 7 步；M9 第 3 步 |
| Z15 | model.go | （v1.1，F3）`<NL><TAB>default:<NL><TAB><TAB>var cmd tea.Cmd<NL>` → `<NL><TAB>default:<NL><TAB><TAB>if m.status == oneCommandHint {<NL><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB>}<NL><TAB><TAB>var cmd tea.Cmd<NL>`（`updateInput` 最外层的 default，`:381-384`。锚点以换行加一个 tab 开头：KeyPress 里那个 default 是两个 tab，`update` 里那个 default 下一行是 `return m.updateInput(msg)`，都不会命中） | M3 第 8 步；M9 第 6 步 |
| Z5 | model.go | （v1.1 补登记，W1 已报被杀）`<TAB><TAB>m.status = fmt.Sprintf("粘贴了 %d 个字符", utf8.RuneCountInString(msg.Content))<NL>` → `<TAB><TAB>if m.status != oneCommandHint {<NL><TAB><TAB><TAB>m.status = fmt.Sprintf("粘贴了 %d 个字符", utf8.RuneCountInString(msg.Content))<NL><TAB><TAB>}<NL>`（粘贴不改写提示） | M4 |
| Z6 | model.go | （v1.1 补登记，W1 已报被杀）`<TAB><TAB><TAB>m.ta.InsertRune('\n')<NL>` → `<TAB><TAB><TAB>m.ta.InsertRune('\n')<NL><TAB><TAB><TAB>if m.status == oneCommandHint {<NL><TAB><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB><TAB>}<NL>`（Ctrl-J 清提示） | M3 第 9 步 |
| Z17 | model.go | （W1b，发现 2；W1b 称 S1）`<TAB><TAB>default:<NL><TAB><TAB><TAB>var cmd tea.Cmd<NL><TAB><TAB><TAB>m.ta, cmd = m.ta.Update(msg)<NL><TAB><TAB><TAB>return m, cmd<NL><TAB><TAB>}` → `<TAB><TAB>default:<NL><TAB><TAB><TAB>var cmd tea.Cmd<NL><TAB><TAB><TAB>m.ta, cmd = m.ta.Update(msg)<NL><TAB><TAB><TAB>if m.status == oneCommandHint && !repl.HasLaterCommandLine(m.ta.Value()) {<NL><TAB><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>return m, cmd<NL><TAB><TAB>}`（KeyPress 的 default 分支：按键之后值不再犯规就清提示。锚点与 Y13 相同） | M9 第 2 步 |
| Z18 | model.go | （W1b，发现 2 的同类）`<TAB><TAB>case msg.Code == tea.KeyEscape:<NL><TAB><TAB><TAB>return m, nil` → `<TAB><TAB>case msg.Code == tea.KeyEscape:<NL><TAB><TAB><TAB>if m.status == oneCommandHint && !repl.HasLaterCommandLine(m.ta.Value()) {<NL><TAB><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB><TAB>}<NL><TAB><TAB><TAB>return m, nil`（Esc 时值不犯规就清提示。锚点与 Z7 相同） | M9 第 3 步 |
| Z19 | model.go | （W1b，发现 2 的同类）`<TAB><TAB>m.width = width<NL><TAB><TAB>m.height = height` → `<TAB><TAB>m.width = width<NL><TAB><TAB>m.height = height<NL><TAB><TAB>if m.status == oneCommandHint && !repl.HasLaterCommandLine(m.ta.Value()) {<NL><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB>}`（窗口尺寸变化时值不犯规就清提示。锚点与 Y14 相同） | M9 第 4 步 |
| Z20 | model.go | （W1b，发现 2 的同类）`<TAB>case elapsedTickMsg:<NL>` → `<TAB>case elapsedTickMsg:<NL><TAB><TAB>if m.status == oneCommandHint && !repl.HasLaterCommandLine(m.ta.Value()) {<NL><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB>}<NL>`（tick 时值不犯规就清提示，插在 `if !m.busy` 之前。锚点与 Y15 相同） | M9 第 5 步 |
| Z21 | model.go | （W1b，发现 2 的同类）`<NL><TAB>default:<NL><TAB><TAB>var cmd tea.Cmd<NL>` → `<NL><TAB>default:<NL><TAB><TAB>if m.status == oneCommandHint && !repl.HasLaterCommandLine(m.ta.Value()) {<NL><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB>}<NL><TAB><TAB>var cmd tea.Cmd<NL>`（`updateInput` 最外层 default：其它消息到来时值不犯规就清提示。锚点与 Z15 相同） | M9 第 6 步 |
| Z22 | model.go | （W1b，发现 2 的同类；W1b 称 S1b）`<TAB><TAB><TAB>m.ta.InsertRune('\n')<NL>` → `<TAB><TAB><TAB>m.ta.InsertRune('\n')<NL><TAB><TAB><TAB>if m.status == oneCommandHint && !repl.HasLaterCommandLine(m.ta.Value()) {<NL><TAB><TAB><TAB><TAB>m.status = "空闲"<NL><TAB><TAB><TAB>}<NL>`（Ctrl-J 之后值不犯规就清提示。锚点与 Z6 相同） | M3 第 9 步 |

Z2、Z16 两行里的 `\|\|` 是 Markdown 表格的转义，源码里是 `||`（两个竖线，没有反斜杠）。v1.1 新登记的 Z 系列 15 条都在原型上实跑：锚点恰好一次、从字节副本还原并比对字节相等，点名杀手全部实测变红（杀手之外另有被杀的用例，不一一列）。Z 编号与 W1 的对照见文件头「v1.1 变更记录」。（v1.2）W1b 修订新登记 Z17-Z22 六条（「值不再犯规才清提示」放在 D6 列的六个不清提示的端点上；Z17、Z22 即 W1b 的 S1、S1b），并在 v1.2 原型上把全部 48 条逐条重跑一遍 `mlx_model_test.go`（M1-M9）：锚点恰好一次，从字节副本还原并比对字节相等，M 系列的点名杀手全部实测变红；L1、L2、P1、R1 上的点名沿用 v1.1 原型的实测，本次没重跑（这几个文件本次没有改动）。X8-X11、X14 只由 L1、L2 杀，M 系列在它们下面全绿，与点名一致。

**判定为等价、不列杀手的变异**（原型实测存活或按代码论证，免得被当成漏测）：
- E1 门里用 `m.ta.Value()` 代替 `text`：`HasLaterCommandLine` 整段剥首尾，`:467` 去掉的末尾 `\r\n` 本来就会被剥掉（实测存活）。
- E2 门挪到空输入判断之前：只有空白时整段剥完是空串、一行，返回 false，照样落到空输入那一支。
- E3 门挪到 `lines := strings.Split(text, "\n")` 之后、`cmds := ...` 之前：中间没有副作用。
- E4 删掉 `if len(lines) < 2 { return false }`：单行时 `/say` 判断与循环都给 false（冗余的早退，保留是为了读起来直白）。
- E5 行首 `TrimLeftFunc` 换成两头都剥的 `TrimFunc`：只看前缀，右边剥不剥无关。
- E6 `strings.TrimSpace(lines[0])` 换成 `strings.TrimRightFunc(lines[0], unicode.IsSpace)`（加 import）：`lines[0]` 的左端已被整段 `isTrimmable` 剥过（它是 `unicode.IsSpace` 的超集），`TrimSpace` 的左剥是空操作。
- E7（v1.1）在 `roundStartMsg`、`memberDoneMsg`、`roundEndMsg`、`boardFinalizeMsg` 分支里清提示：提示只在不忙、不在收口窗口时出现，这四种消息在那时要么被忽略、要么到不了（下一次提交先把 status 写成 `正在运行`），可达状态下看不见（按代码论证，D6 末句；V3 的判断）。（v1.2）W1b 在附注里指出：在 M3 里喂一条合成的 `roundEndMsg{}` 就能杀掉「RoundEnd 清提示」；那是提示挂着时到不了的消息，这条仍按可达性判等价，不加用例（W1b 自己也没列为发现）。

**只在审查时看（没有用例能抓，集成员收割时逐字核对）**：
- `model.go` 调的是 `repl.HasLaterCommandLine`，不是在 `tui` 里另写一份判据。另写一份逐字相同时全绿，分叉的那一天才会有一边错。
- 两段注释（`latercmd.go` 的函数注释、`model.go` 常量与 `beginSubmit` 的注释）与 §1.1 逐字相同，没有与代码不符的话。
- `beginSubmit` 仍只有 Enter 分支一个调用点。
- （v1.1）门里除了 `m.status = oneCommandHint` 与 `return m, nil` 没有别的语句：写 `run`、`rows`、`tickGen`、`tickPending`、`cancel` 这类字段的变异，挡下时不忙、表头与帧高都不变，本刀的用例看不见（D5 末句；按代码论证，没有逐条实跑）。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- **只许动这两个文件**：新建 `internal/repl/latercmd.go`；修改 `internal/tui/model.go`。不写任何 `_test.go`，不改别的文件（`seam.go`、`commands.go`、`run.go`、`go.mod`、`go.sum` 都不许动）。
- 生产改动约 50 行，**一次交付**（不拆块）。
- 新文件用：

  ```
  ===FILE: internal/repl/latercmd.go===
  （完整内容）
  ===END FILE===
  ```
- 已有文件用补丁块：

  ```
  ===PATCH: internal/tui/model.go===
  <<<<ANCHOR
  （从 HEAD 真文件里原样复制的若干行，含行首 tab，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  本刀的锚点就是 §1.1 B 给出的那 6 行（派工时的 main HEAD 的 `model.go:465-470`；写本文时是 `80a9385`，`model.go` 与 `2a8737f` 逐字节相同，v1.2 修订），替换文本就是 §1.1 B 的替换块。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- §1.1 A 的整个文件、§1.1 B 的替换块**逐字照抄**（含注释、空行与 gofmt 的缩进）。代码必须能 `gofmt`。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的字符串字面量只有 §3.1 那一条（外加 `latercmd.go` 里的 `"\n"`、`"/say"`、`"/"` 三个 ASCII 片段）。
- 生产代码的新标识符只有 `HasLaterCommandLine`、`oneCommandHint` 两个，都冻结；不许以 `mlx` 开头。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：两个文件是否都给了；§1.1 A、B 各落在哪个块。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3.1 的字面量是否逐字照抄；有没有新增任何别的字符串字面量。
  ```

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；锚点恰好一次后落盘；`gofmt -l internal/repl/latercmd.go internal/tui/model.go` 为空；`go vet ./internal/repl ./internal/tui`；`go build ./...`；`git diff --stat` 只有这两个文件；逐字节核对 §1.1 A、B 与 §3.1（十六进制比对提示串）。
2. **合并测试腿的四个文件**，核对测试腿报告里「HEAD 上 tui 三个 mlx 文件的红绿分布」与 §2.0 一致。只跑 `./internal/repl ./internal/tui`（另一条流水线也可能在跑 go test；3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制），再跑一次全量 `./...`。既有用例必须全绿（原型实测全绿、零改动）。P1、R1 各 `-count=10`。
3. **§4 的变异逐条跑**（等价清单里的不跑；从字节副本还原）。任何一条没被点名杀手杀掉，先判是用例空、还是契约写错；契约错就改契约再改任务书。
4. **文档**：
   - `docs/00_DECISIONS.md` #44 标题状态改成「已实现（X44，`<commit>`），待真机复验」；补「落地记录」：Griffin 的裁定原话；行首空白算的理由（D2）；`/say` 出路的设计与论证（D3，含「第 1 行只有 `/say`」与「零宽字符不豁免」两条边界）；提示的措辞、宽度与持续规则（D6、D7）；已知代价：缩进的代码注释 `// …` 与 `/` 开头的路径行也被挡、要走 `/say`；`/ask`、`/to` 的多行正文里有 `/` 开头的行时发不出去（没有出路）；TUI 没有一键清空输入框（挡下之后要逐行删或走 `/say`）；80 列以下的窄终端提示被截尾（与 RB-A 剩余项 (c) 同一件事）；删掉犯规的行后、回车前提示还挂着。
   - 本契约归档进 `docs/`（按现有编号体例取名，避免与 MD1 / CO1 / SM1 的归档撞号；文件头加归档说明，写明行号指 HEAD `2a8737f`）。
   - 主代理给 Griffin 的 council 内命令继续一条一块（#44 末段的做法不变）。
5. **给 Griffin 的真机复验步骤**（Windows Terminal + PowerShell 5.1；每条 council 内命令单独一行、单独一块，逐条回车；多行输入一律用 Ctrl-J 在输入框里拼，不要整块粘贴步骤本身）：
   1. 启动 TUI，先选一个会话（把 `<会话id>` 换成真的）：

      ```
      /use <会话id>
      ```
   2. 在输入框里敲下面这一行，**先别回车**：

      ```
      /new 真机复验 x44
      ```

      按 Ctrl-J 换行，再敲下面这一行，然后回车：

      ```
      /add opus-5.0,grok-4.7
      ```

      预期：回滚区什么都不多；输入框里两行原样还在；表头变成 `council <版本>   一次只能一条命令，逐条回车；整段当消息发，首行只写 /say`，80 列窗口里整句看得全。
   3. 按一个字母再退格、按一下 Esc、拖一下窗口大小：提示不消失。再按一次回车：仍然不提交、提示不变。
   4. 按 Ctrl-Home 回到开头，敲 `/say`，按 Ctrl-J，再回车。预期：回显三行 `> /say`、`> /new 真机复验 x44`、`> /add opus-5.0,grok-4.7`；没有建新会话、没有加成员。CO1 已落地（`1c62dbb`）：默认简洁档下回显之后**不打** `已记下 seq N`（这是 CO1 的既有行为，不是出路失败；用 `-verbose` 启动才打）。用下面这条确认那条消息落了库、正文就是那两行：

      ```
      /log 3
      ```
   5. 缩进也挡：敲 `随便一句`，Ctrl-J，敲两个空格再敲 `/log 3`，回车。预期：同第 2 步的提示，不提交。然后照第 4 步把它当消息发掉（或逐行删掉）。
   6. 整块粘贴（真正撞出 #44 的那条路）：在记事本里写两行——第一行 `一句话`、第二行 `/log 3`——全选复制，粘进 TUI 输入框，回车。预期：表头先是 `粘贴了 N 个字符`，回车后换成提示，不提交。
   7. 长文照常：在记事本里写两行不以 `/` 开头的字（例如 `第一行` 与 `第二行`），粘进来回车。预期：与改动前一样，回显两行、不出提示。默认简洁档下不打 `已记下 seq N`（CO1；`-verbose` 启动时照打），用下面这条确认最后一条消息就是那两行：

      ```
      /log 3
      ```
   8. 忙时（会花一次调用的钱；不想花钱就跳过这一步）：把 `<成员id>` 换成会话里的一个真成员，回车：

      ```
      /round <成员id> --go
      ```

      它跑的过程中照第 2 步在输入框里拼两行再回车。预期：表头是 `一轮正在跑，先等它回来或按 Ctrl-C 取消`（与以前一样）；这一轮结束之后再回车，才出本刀的提示。
   9. 行式路径不变：在 PowerShell 里用管道或文件喂一个两行脚本（第一行 `/use <会话id>`、第二行 `/log 3`）给非终端的 council：两条都照常执行，没有任何提示。
6. 清理本刀在中转根与 scratchpad 里的全部中间物。

---

## 7. open items

### 待 Griffin

- (a) **`/say` 这条出路要不要留。** 本契约留了（D3）：第 1 行只写 `/say`、下面是任意多行，整段当一条消息发。不要的话删掉 `latercmd.go` 里那一个 `if`、提示去掉后半句即可（§4 X7 就是这个形状）。
- (b) **`/ask <id>`、`/to <ids>` 要不要同样的出路。** 它们也把后面全部当正文。今天多行正文里有 `/` 开头的行时发不出去；变通是先 `/say`（第 1 行只写 `/say`）发正文，再单独 `/ask <id>`（不带文本）问。`/to` 的定向消息没有变通。
- (c) **缩进的代码注释与路径行也被挡**（D2 的代价）：粘一段 Go 代码当消息时，`// 注释` 那种行会触发提示，要走 `/say`。要不要把 `//` 开头的行放过（`Submit` 会把 `//x` 当未知命令，但用户意图几乎不会是命令），等你定；本刀照你的原话「以 / 开头」一律挡。
- (d) **挡下之后没有一键清空输入框。** Esc 什么都不做（`model.go:366-367`），空闲时 Ctrl-C 直接退出程序；要清只能逐行删。要不要给 Esc 绑「清空输入框」，另议。

### 待主代理

- (e) 派工前对本契约做一轮对抗审查（W1）——（v1.1）已做，发现 F0-F5 按裁决书落实（文件头「v1.1 变更记录」）。（v1.2）第二轮审查 W1b 已做，三条发现（都是 minor）全部成立、照改（文件头「W1b 修订」）；§1.1 未改，没有新增待 Griffin 的事项。仍未验证的一条：R1 的前提（括号粘贴在管道上被解成 `PasteMsg`、CR 被 sanitizer 变成换行）在 Linux 上是否同样成立（W1 与 v1.1 原型都只在 Windows 上跑过）。
- (f) 归档文件名（§6 第 4 步）按当时 `docs/` 的编号定，避开 MD1 / CO1 / SM1。
- (g) `seam.go:50-55` 与 `msgDraftNotInTUI`（`seam.go:11`，「TUI 下直接在输入框里换行即可，不需要多行模式」）仍然成立：单行 `/say` 照旧提示它，多行消息照旧用 Ctrl-J 换行；只是第 2 行起有 `/` 开头的行时要第 1 行写 `/say`。要不要在那句提示里补半句，等 (a) 定了再说，本刀不动它。
