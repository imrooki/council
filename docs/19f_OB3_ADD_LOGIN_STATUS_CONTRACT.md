# OB3 冻结契约：/add 显示各家的登录状态与 OB2 审查余项（v1.1，2026-09-30）

> **归档说明（2026-09-30，主代理）**：本文是 OB3 的冻结契约，派工时的原件是 `E:\multi_model_cowork\ob3_contract.md`（v1.1），原样归档；锚点与行号指 HEAD `35520ff`。OB3 已于 `ef76eef` 落地，2026-09-30 Griffin 说「装」后装为 `council 0.2.0-0930c`（vcs.revision `23a3232`），真机未复验。
> - **实现**：生产代码分两块，两路 grok-4.7 并行实现，每路只拿本块的契约摘录（块 A 23,509 B、块 B 19,881 B），与 §1.1 逐字节相同（7 个文件，含 T1、T2 两处授权的测试改动）。这一次两路 grok 的 `===FILE` / `===PATCH` 标记都在行首。
> - **测试**：3 个 obz 测试文件由一条 Opus 测试腿写成，与 §2 逐字节相同。
> - **集成**：Opus 集成员没有做机械修正。§4 的 36 条变异（Q1-Q36）全被点名杀手杀掉，没有登记等价变异。每次运行后四项泄漏核对全部干净。
> - **审查**：Opus 独立审查报 1 条 minor（见下），不是实现偏离。审查员另拟了 8 条变异：6 条被杀；X4、X5（吞掉写错误）存活，实际等价，同一个坏 writer 的下一次写照样报错。
>
> **落地后订正（2026-09-30，主代理；读本文之前先读这里）**：
> - **一处注释说多了**（审查，即本文 §7 (k)）：`internal/repl/addlogin.go` 里 `LoginCommand` 的注释说首次运行的登录办法用的都是它，但 `msgFirstRunNoneFound`（一家都没找到时的安装说明）另写着四家的登录办法。按 B2 的限定读：说的是结果之后列出的登录办法。要改就是块 A 的一处注释，留给后续小刀。
> - **§6 第 5 步第 0 条**说「主代理照惯例等 Griffin 说『装』」，照此办理：落地时没有装，等 Griffin 说了「装」才装的 `0.2.0-0930c`。
> - **测试腿报的一点**：`shared_deadline` 不加 50 ms 的 sleep 时，只有一定概率杀得到 Q34；加了之后，审查员 -count=5 次次都杀得到。
> - **§6 第 5 步第 2 条写错了一个命令**（主代理落地时核对 `commands.go` 的命令表发现）：「`/list` 之后 `/use`」里的 `/list` 不存在，列会话的命令是 `/conv`。`14_MVP_CHECKLIST.md` 的 M6 已按 `/conv` 写。
> - **§7 (k)「LoginCommand 的注释说多了」：已由 CL1 改**（2026-10-01，`5c4840a`，#50，`19h` 的 A6），照 firstrun.go B2 的限定说法。

**v1.1 变更记录（2026-09-30，照主代理的 v1 → v1.1 裁决书；W1 审查员 1 条 major、2 条 minor 全部接受）**

- 输入：裁决书 `ob3_rulings.md`（唯一权威）、W1 原文 `ob3_w1_findings.md`、v1 快照 `ob3_contract_v1.md`。基线仍是 HEAD `35520ff`。v1.1 原型：`E:\Isolating_test\council\ob3_v11\proto\`（`git archive 35520ff`，先照 v1 的 §1.1 与 §2 逐字落盘、全绿，再改）。
- **只改了测试、注释与文字**：块 A 的生产代码一个字节不变（`repl.go`、`addmodel.go`、`addlogin.go` 与 v1 逐字节相同，脚本比过）；块 B 只改 B2 的注释（`firstrun.go` 与 v1 相比只有这一处：一行注释换成两行）；`app.go` 与 v1 相同；T1、T2 不变。
- **M1（major，W1 审查员与反驳者）：共用超时与父 ctx 取消没有用例钉住。**
  - S2 的假执行器 `obzStatusExec` 多记两样：deadline 的值（`deadlineAt`），以及钩子之后此刻 ctx 是否已结束（`canceled`，按次记在切片里）。冻结的顺序注释从四步改成五步，写的就是代码做的。
  - Z5 加子测试 `shared_deadline`：包一层 `g.r.discover`，记下识别 ctx 的 deadline，再 `time.Sleep(50 * time.Millisecond)`；断言登录状态执行器拿到的 deadline 与它 `Equal`。识别那一侧恰好一个 deadline 是 `Fatalf` 前提，防的是两边零值相等时的空绿。
  - Z8 在第一次 `/add` 之后立刻断言记下的是 `[true]`（放在第二次 `/add` 的几处 `Fatalf` 之前），第二次之后断言整份是 `[true false]`（没有钩子的那一次是正对照）。
  - `obzWantCodexCalls` 取裁决书的第一种办法，改措辞：报错消息只说「ctx 没有 deadline」，注释写明 deadline 是否同一个由 `shared_deadline` 查。
  - §4 登记 Q32（Background 派生的新 discoverTimeout）、Q33（WithDeadline(Background, 同一个 deadline)）、Q34（父 ctx 派生的新 discoverTimeout，cctx 已到期时照旧交 cctx，D9 的判断不变）。原型实测：Q32 被 `shared_deadline` 与 Z8 的两个子测试杀；Q33 只被 Z8 的两个子测试杀；Q34 只被 `shared_deadline` 杀。失败消息核过，是新断言红的（deadline 相差约 50 ms；`canceled` 记下的是 `[false]`）。
  - sleep 必需性（原型实测，`-count=5`）：删掉那一行 sleep 之后，Q34 五次都活下来，Q32 只剩 Z8 杀它。**与裁决书的出入**：裁决书写「去掉 sleep 之后 Q33 活下来」，实测改成对 Q34。Q33 的 deadline 按构造就与识别的相同，拖不拖都只有 Z8 杀得到它；靠 sleep 才杀得到、别处又杀不到的是 Q34（§4「Q32-Q34 为什么三条都要登记」与「50 ms 是必需的」两段）。
  - D2、D6 的文字没改。
- **m1（minor）：识别报错但 stdout 有内容时照样判登录状态，没有用例。**
  - Z6 加子测试 `grok_exit_error`：识别夹具换成返回没登录那一句加一个模型、同时带 `errors.New("exit code 1")` 的闭包；登录段里 grok 是没登录，带登录办法。§4 登记 Q35（`/add` 的闭包只在 `err == nil` 时截 stdout），只有它杀得到。
  - 同类的另一处（首次运行 `firstRunCheck`，OB1 留下的）一起补：S3 加 Z13 `TestObzFirstRunGrokExitError`。假 grok 用 obx 的 `obxFake` 写，只读复用，不改 obx 文件：`@type` 一份夹具，往 stderr 打一行 `obz-grok-exit`，`@exit /b 1`。§4 登记 Q36（`firstRunCheck` 里的同一条变异），只有它杀得到。
  - 前提先查过（原型探针实测）：`turn.RunDiscovery` 在退出码非 0 时把 stdout 连同错误一起交回，`DiscoverModels` 调的就是截 stdout 的那层闭包。
  - 实测中的发现：在 app 用例的隔离 `PATH` 下，只 `@type` 再 `@exit /b 1` 的假 CLI，错误尾巴是 cmd 打的一行找不到命令的报错，来源没查。所以假 grok 自己打最后一行 stderr（§2.3 Z13、§7 (l)）。
- **m2（minor）：「登录办法只有一份」不准。**
  - B2 的冻结注释改成限定说法：结果之后列出的登录办法只有 `repl.LoginCommand` 那一份；`msgFirstRunNoneFound` 里的安装说明另写着，首次运行的输出不变，不动它。
  - D4 的标题同样限定（问过「同类的还有没有别处」：D4 与 B2 是同一句话）。
  - D10 补上 `msgFirstRunNoneFound` 这第三处；§7 加 (j)（改 `loginCommands` 时要同步它）。
  - §6 第 1 步加登录办法字面量的 grep 核对。v1.1 原型上实测命中五处：`msgFirstRunNoneFound` 的四行，外加 `firstRunCheck` 头注释里「codex login status」那一行（HEAD 原有，说的是查登录状态的命令）。
  - §4「只在审查时看」加上「首次运行另留一份内容相同的登录办法表」这条行为等价的变异，并写明理由。
- **起草人的 open items**：(a) 授权、(b) 保留、(c) 接受、(d) 接受，改记为主代理已裁定；(e)-(i) 照原文留在 §7。另加 (j)、(k)、(l)。(k) 是 A6 里 `LoginCommand` 的注释，与 v1 的 B2 注释是同一种说法。裁决书要块 A 一个字节不变，所以没改。
- **跟着同步的**：§2 用例编号（Z10-Z13 在 app）、§2 纪律里的正对照与「首次运行」两句；§2.2、§2.3 的用例说明；各测试的【判别力】注释，按实测杀手改过（Z5、Z6、Z8、Z13），只加不减；S3 文件头的复用清单，按实际用到的 obx 标识符补上 `obxFake`、`obxSub`、`obxHead`、`obxLoginHead`、`obxDone`；§4 的条数 31 改成 36；D12 与 §5 的行数（块 B 替换文本 19 改成 20 行，合计约 174 行，脚本数过）；§6 第 3 步的条数。
- **v1.1 原型实跑**（每一次都在同一条 PowerShell 命令里先 dot-source 隔离环境，见 §7 末尾）：
  - `go build ./...` 干净；`go vet`（五个包）干净；`gofmt -l ./internal ./cmd` 空。
  - 全量 `go test -count=1 ./...` 九包全绿（app 9.2 s、repl 73.2 s、tui 69.3 s、turn 15.6 s、config 11.3 s，其余几秒）。
  - `-run 'TestOb[xyz]'` 五个包 `-count=3` 全绿（app 13.3 s、repl 3.7 s、provider 0.5 s、turn 6.6 s、config 7.7 s）。
  - §4 的 36 条变异全部重跑，驱动脚本直接从本文 §4 取替换前后：每条锚点恰好一次、全部被杀、从字节副本还原后 SHA256 一致。杀手清单与耗时都换成这一次的实测。与 v1 相比，没有哪一条少了杀手（脚本比过），有 12 条多了新子测试。先后顺序照实写：这批变异跑在改 S2、S3 的【判别力】注释与 Z13 注释之前，当时的原型与交付的文件只差注释；之后的 gofmt、vet、全量、`-count=3` 与往返核对都跑在最终的字节上。S2、S3 文件头的复用清单按标识符数过：各自列出的与实际用到的 mdx、cxh、obx、cox 标识符完全相同（S2 25 个，S3 22 个）。
  - 只取块 A 与 S1、S2 打在另一份新解出的 `35520ff` 上：`go build`、`go vet`（五个包）干净，`gofmt -l` 空，全量九包全绿（app 11.1 s、repl 78.0 s、tui 70.9 s、turn 16.4 s，其余几秒）。D12 的说法在 v1.1 下仍成立。
- **往返核对（v1.1）**：
  - 写回：§1.1 的四个 `file` 块、B2 的 `replace` 块、§4 全部 36 条的耗时与杀手行，由脚本从原型与变异结果写回本文。
  - 取回：再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在又一份新解出的 `35520ff` 上。12 对锚点各恰好一次、同一文件互不重叠，4 个整文件。
  - 结果：改到的 10 个文件、整棵树 366 个文件与原型逐字节相同（0 个差异、两边没有多出的文件）。
  - 编码：本文与三个测试文件都是 UTF-8 无 BOM、只有 LF；符号禁令扫描 0 命中（本文全文、10 个改动或新增的 Go 文件）。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `35520ff`（完整 `35520ff9ed230f55986e380a9f25869de7f7c60f`，= origin/main）。代码与 `70025db`（OB2 落地）相同，之后只有文档提交。Go 全路径 `"C:\Program Files\Go\bin\go.exe"`（go1.27.0 windows/amd64）。本机没有 gcc，不能 `-race`。E 盘机（Windows 11，PowerShell 5.1）。
- 本刀的 KEY 是 `OB3`；测试腿的包级标识符前缀是 `obz`（类型、函数、变量、常量一律 `obz` 开头，测试函数 `TestObz...`）。生产代码的新标识符不许以 `obz` 开头（本文的生产代码新标识符：`LoginText`、`LoginCommand`、`SetLoginStatusExec`、`loginCommands`、`msgLoginHead`、`snapLogin`、`snapshotLogin`、`printLoginStatus`、字段 `loginStatus`、`logins`）。
- 来源：
  - Griffin 2026-09-30「可以」：在 `/add` 那张识别表里每家加上登录状态，细节由主代理定（本文 §1.1 的 D1-D12 是主代理的冻结裁定，方向不改）。
  - `docs/19e_OB2_API_KEY_LOGIN_CONTRACT.md` §7 (a)（`/add` 的识别表不显示登录状态，候选的下一刀）与文件头「落地后订正」的前两条（grok 缺三行负对照；`oby_firstrun_app_test.go` 文件头的复用夹具清单不准）。
  - `docs/19d_OB1_FIRST_RUN_CONTRACT.md` §7 (o)（`/add` 不显示登录状态，留作 open item）。
- 本文的全部期望值都在隔离拷贝上用一份原型实现实跑得出（v1 在 `E:\Isolating_test\council\ob3_draft\proto\`，v1.1 在 `E:\Isolating_test\council\ob3_v11\proto\`，都是 `git archive 35520ff` 解出，362 个文件）。下面这几条是 v1 起草时的记录；v1.1 改过的测试与重跑的结果见上面的「v1.1 变更记录」：
  - 原型下 `go build ./...` 干净；`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn` 干净；`gofmt -l ./internal ./cmd` 空。
  - 原型下全量 `go test -count=1 ./...` 九包全绿（app 7.9 s、repl 71.9 s、tui 62.4 s、turn 13.3 s，其余几秒）；`-run 'TestOb[xyz]'` 五个包（app、repl、provider、turn、config）`-count=3` 全绿（app 12.8 s、repl 2.9 s、provider 0.03 s、turn 6.3 s、config 7.9 s）。
  - §4 的 31 条变异在原型上逐条实跑，全部被点名杀手杀掉；每条都从字节副本还原、SHA256 核对一致。
  - 只取块 A（§1.1）与 S1、S2 两个测试文件打在另一份新解出的 `35520ff` 上：`go build ./...`、`go vet`（五个包）干净，`gofmt -l` 空，全量九包全绿（app 7.6 s、repl 71.2 s、tui 62.1 s、turn 13.1 s，其余几秒）。
  - §1.1 的冻结代码、§2 的三个测试文件由脚本从原型取出写进本文；锚点由脚本从一份新解出的 HEAD 逐行取出，核过各恰好一次、同一文件互不重叠。再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在第三份新解出的 `35520ff` 上：12 对锚点各恰好一次、4 个整文件，改到的 10 个文件与原型逐字节相同，整棵树 366 个文件与原型一个字节不差。原型不交付，用完删除。
  - 每一次 go build / vet / test / 变异运行都在同一条 PowerShell 命令里先设隔离环境（§4 表头），每次 go test 之后核四项，次次干净（§7 末尾「泄漏核对」）。
- 证据等级照实写：「实测」是本机跑过的；「原型实跑」是在上面的隔离拷贝里跑过的；「19d / 19e 实测」是引用那两份契约的记录，本刀没有重跑任何真 CLI。

**与已落地各刀的相交**（逐个点名；本刀对它们的代码一个字不改，除非下表写了「改」或「授权改」）：

| 已落地 | 相交的文件 / 函数 | 本刀怎么用 |
|---|---|---|
| MD1（16a）、CXH（18a）| `internal/repl/addmodel.go`：`modelSnapshot`、`takeModelSnapshot`、`printDiscoveredModels` | **改**（A2-A5）：快照多一个字段 `logins`；识别经闭包截下 stdout；装了执行器时每家识别之后判登录状态；`printDiscoveredModels` 在 `printModelSnapshot` 之后打登录段。`printModelSnapshot`、识别表格式、失败行、`--refresh` 提示、`正在识别模型: …` 一个字不改 |
| MD1、CXH 测试 | `mdx_addmodel_fixture_test.go`（`mdxNewRig`、`mdxExec`、`mdxResp`、`mdxWantLines`、`mdxHasLine`、`mdxReplaceOnce`、`mdxSplit`、`mdxConfigText`、夹具与模板、`mdxAppliedLine`、`mdxGrokPath`）、`cxh_fixture_test.go`（`cxhNewRig`、`cxhConfigTemplate`、`cxhCodexOnlyTemplate`、`cxhCodexPath`、`cxhCodexFixture`、`cxhListBlock`、`cxhGrokDownBlock`、`cxhOneGpt55`）| 只读复用。它们的 REPL 由 `New` 直接造出、不装执行器，输出一个字节不变（D5） |
| OB1 `3317940`（19d）| `internal/repl/cliadvice.go`（`SetCLIDetector` 的先例、`printUnconfiguredCLIs`）| 不改。登录段在它之前打（D1）；新 setter 照它的写法与注释 |
| OB1 测试 | `internal/repl/obx_cliadvice_test.go`（`obxDetector`、`obxList`、`obxHintLines`）| 只读复用（Z3） |
| OB1、OB2 | `internal/app/firstrun.go`：`firstRunLogin`、`firstRunLoginText`、收登录办法那一处 | **改**（B1-B3）：两样都挪到 repl 导出（`LoginCommand`、`LoginText`）；`firstRunLoginText` 留名字、就是 `repl.LoginText`；首次运行的屏幕输出一个字节不变。`firstRunCheck`、`firstRunTimeout`、`firstRunStatus` 不改 |
| OB1 | `internal/app/app.go` 装配（`:251-262`）| **改**（B4、B5）：TUI 与行式两处各加一行 `r.SetLoginStatusExec(turn.RunLoginStatus)` |
| OB1 测试 | `internal/app/obx_firstrun_app_test.go`：A10 `TestObxAddHintLineMode`；§2.0 装置 | **授权改一处**（T2，A10 的期望：识别表之后、提示之前多了登录段，位置是裁决冻结的，见 §0 第 4 条与 §7 (a)）。§2.0 装置（`obxIsolate`、`obxRun`、`obxDrain`、`obxTypeFake`、`obxCodexFake`、`obxFixture`、`obxIn`、`obxEnv`、`obxNotExist`、`obxClaudeOut`、`obxGrokOut`、`obxCodexOut`、`obxCodexLoggedIn`）只读复用 |
| OB2 `70025db`（19e）测试 | `internal/app/oby_firstrun_app_test.go` 文件头 | **授权改一处注释**（T1，19e 头第二条）：复用夹具清单去掉 `obxFake`、补上 `obxIn`、`obxEnv`。B0 按 `firstRunLoginText` 这个名字调，所以 app 里留这个名字（D3） |
| OB1、OB2 | `internal/provider/login.go`（`LoginFromDiscovery`、`LoginStatusCommandFor`、`ParseLoginStatus`、`LoginStatusExec`）| 不改。判据一个不动；本刀只补三行负对照（S1 的 K3） |
| OB1、OB2 | `internal/turn/loginstatus_exec.go`（`RunLoginStatus`）| 不改；app 装它 |
| cox 测试 | `internal/app/cox_app_test.go`（`coxLockedBuf`、`coxWaitFor`、`coxFlat`、`coxESC`）| 只读复用（Z11） |
| TestMain | app 的在 `smoke_test.go`，repl、config、turn 各有；provider 没有 | 不改、不另加 |

---

## 0. 动机 / 实测

1. **现状（HEAD `35520ff`，核过真文件）**：
   - 登录状态只在首次运行里显示：`internal/app/firstrun.go:97-119` 的 `firstRunCheck` 经一层闭包截下识别的 stdout，claude、grok 用 `provider.LoginFromDiscovery` 判，codex 在这一家的 `firstRunTimeout` 还没用完时另跑 `provider.LoginStatusCommandFor` 给的命令（执行器 `firstRunStatus`，生产是 `turn.RunLoginStatus`），执行器报错就是查不出；`:72-91` 的 `firstRunLoginText` 是四档说法；`:44-51` 的 `firstRunLogin` 是四家的登录办法，`:175-184` 只给既不是已登录、也不是已登录（API key）的那几家列登录办法。
   - `/add`（`internal/repl/commands.go:883` 的 `cmdAdd`，`:922-934`）不带参数或带 `--refresh` 时依次打配置里的候选、`printDiscoveredModels`（`addmodel.go:161-177`）、`printUnconfiguredCLIs`（`cliadvice.go:29-48`）。识别只看配置里写了的 provider（`addmodel.go:57-70`，字典序），`takeModelSnapshot`（`:120-150`）每家一个 `discoverTimeout`，直接把 `r.discover` 交给 `provider.DiscoverModels`，不截 stdout，不判登录状态。没登录时唯一的线索是失败行「一个模型都没列出来（没登录？）」（`:253`），而没登录的 claude、grok、codex 照样能列出一部分模型（19d §0 实测），所以大多数时候什么线索都没有。
   - `/join` 与 `/add` 是同一个 handler（`commands.go:865-866` 注释），下面说的 `/add` 都包括 `/join`。
2. **现在的 `/add` 长什么样**（A10 `TestObxAddHintLineMode/configured` 的装置：显式配置写了 claude、grok 两家，claude 已登录（订阅 Claude Obx）、grok 没登录，codex 装了、配置里没有；行式 `app.Run` 敲 `/new t`、`/add`。实测：在一份新解出的 HEAD 上把 A10 的期望临时加一个字、让它红，取它打出的实际 `out`，下面是 `/add` 那一段，`<root>` 是用例的临时目录；A10 在 HEAD 上钉住的是从「用 /add +编号 加入……」那一行到结尾）：

   ```text
   council> 配置里还没有可以 /add 的成员
   正在识别模型: claude, grok
   识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
   NO  PROVIDER  ID          MODEL         EFFORTS   SOURCE
   +1  claude    obx-a-high  claude-obx-a  low|high  订阅
   +2  claude    obx-b       claude-obx-b  -         订阅
   +3  grok      grok-obx-1  grok-obx-1    -         本地配置
   +4  grok      grok-obx-2  grok-obx-2    -         本地配置
   +5  grok      grok-obx-3  grok-obx-3    -         本地配置
   用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
   本机装了 codex（<root>\bin\codex.cmd），配置里还没有：要用它，把下面两行追加到 <root>\explicit.toml 末尾，再敲 /config reload --apply 与 /add --refresh
   [providers.codex]
   command = '<root>\bin\codex.cmd'
   ```

   同一个装置在原型上（原型实跑，A10 未改期望时的失败输出里取的实际字节）：「用 /add +编号 加入……」那一行之后、「本机装了 codex」之前多出三行，其余逐字节相同：

   ```text
   各家的登录状态（本会话的快照，/add --refresh 重新检查）:
     claude  已登录（订阅：Claude Obx）
     grok    没登录；在终端里运行 grok login，登录之后 /add --refresh
   ```

   grok 那一家识别成功（列出 3 个模型）却是没登录：这正是只看识别表看不出来的情况。
3. **codex login status 的代价**（19d §0 实测，本刀没重跑）：`codex login status` 0.16 s、零花费、退出码 0 / 1、文字打在 stderr（`Logged in using ChatGPT` / `Not logged in`）。`/add` 每次重拍快照（本会话第一次 `/add`、每次 `/add --refresh`）只多这一次；claude、grok 从识别那一次本来就拿到的 stdout 判，不多起进程；agy 查不出、不起进程。
4. **原型实测**：
   - 原型只套生产代码（块 A、块 B）、不加任何新测试、不改任何既有测试时，全量只有 A10 的两个子测试红（`TestObxAddHintLineMode/hint`、`/configured`），红的原因正是上面第 2 条多出的那一段。T2 改的就是它，新期望取自原型的实际输出。
   - 只套块 A（与 S1、S2）时全量九包全绿：app 不装执行器，`/add` 不打登录段，A10 仍是 HEAD 的期望（D12）。
   - Z5 `hang`：登录状态命令挂住时，未变异的 `/add` 在 200 ms（装置的 `discoverTimeout`）一带返回、codex 那一行是查不出；Q6（改用父 ctx）下要等假执行器 3 s 兜底、那一行变成已登录。
5. 没有调用任何真 provider CLI、没有向任何模型提问。测试里的「codex」「claude」「grok」一律是注入的假执行器（repl 包）或临时目录里只用 cmd 内建命令的 `.cmd`（app 包）。

---

## 1. 范围

### 1.1 做

#### 裁定（主代理定的细节，逐条冻结）

- **D1 显示什么、在哪。** 不带参数的 `/add` 与 `/add --refresh`（连同 `/join`），在识别结果之后打一段登录状态：段头一行，然后配置里能识别的每一家（与「正在识别模型: …」同一组、同一顺序，即字典序）各一行。整段在 `printModelSnapshot` 已有的全部输出（表、custom 行、失败行与 `--refresh` 提示）之后、`printUnconfiguredCLIs` 之前：`printDiscoveredModels` 在 `printModelSnapshot` 返回之后调 `printLoginStatus`（A5）。行的对齐与首次运行相同：两个空格缩进、id 左对齐占 6 格、两个空格、状态（`"  %-6s  %s"`，不用 tabwriter）。字面量见 §3。`/add +编号`、`/add <id>` 不打。
- **D2 判据与首次运行完全相同**（`snapshotLogin`，A6）：claude、grok 从识别那一次的 stdout 判（`takeModelSnapshot` 经一层闭包截下，同 `firstRunCheck`），`provider.LoginFromDiscovery`；codex 在识别之后、这一家的 ctx（识别用的那一个 `cctx`，`discoverTimeout`）还没到期时另跑一次 `provider.LoginStatusCommandFor(pid, r.cfg.Providers[pid].Command)` 给的命令，执行器报错就是查不出，否则 `provider.ParseLoginStatus`；agy 查不出。识别与登录状态命令共用这一家的一个超时，所以 `cancel()` 挪到判登录状态之后（A4）。provider 包的判据一个不改。
- **D3 四档说法只有一份。** `repl.LoginText`（A6）的函数体与 HEAD 的 `firstRunLoginText` 逐字节相同（挪过去；脚本比过，函数名之外 370 字节一致）。app 里 `firstRunLoginText` 这个名字留着，改成 `var firstRunLoginText = repl.LoginText`（B2）：它就是同一个函数（Z12 用函数指针相等钉住），首次运行两处调用点不用改，OB2 的 B0 按这个名字调、不用改。首次运行的屏幕输出一个字节不变（OB1 的 obx、OB2 的 oby 用例全绿，§0 第 4 条）。
- **D4 结果之后列出的登录办法只有一份。**（v1.1 限定：`msgFirstRunNoneFound` 里的安装说明另写着各家的登录办法，见 D10、§7 (j)。）HEAD 的 `firstRunLogin` 表连同注释挪到 repl，成为不导出的 `loginCommands`（脚本比过，名字之外逐字节一致），经导出函数 `repl.LoginCommand(providerID)` 取（不认识的 id 返回空串；不导出可写的 map）。首次运行收登录办法那一处改用它（B1、B3），输出不变。`/add` 的登录段里只有「没登录」那一家在同一行末尾带「在终端里运行 <登录办法>，登录之后 /add --refresh」；「查不出」不带（agy 永远查不出，每次 `/add` 都提示是噪声）；已登录、已登录（API key）不带。今天能落在没登录的只有 claude、grok、codex（provider 包的判据），三家都有登录办法。
- **D5 开关。** 照 `SetCLIDetector` 的先例：`REPL` 加字段 `loginStatus provider.LoginStatusExec`（A1）与 setter `SetLoginStatusExec`（A6），只许在 `Run` 或第一次 `Submit` 之前调。**零值 nil 表示 `/add` 不做登录检测、不打登录段、不多起任何进程**：快照的 `logins` 留 nil，`printLoginStatus` 见到空的就什么都不打；截 stdout 的闭包照样装（它只是包一层 `r.discover`，不起进程）。所以直接 `New` 出来的 REPL（MD1、CXH、tui 包等既有测试）输出一个字节不变。app 在 TUI 与行式两处装配都装 `turn.RunLoginStatus`（B4、B5）。
- **D6 快照语义不变。** 登录状态是快照的一部分（`modelSnapshot.logins`，A2），与识别同一次拍，随快照留到本会话结束；只有 `/add --refresh`（`cmdAdd` 已有的 `r.modelSnap = nil`）或新会话重拍；`/config reload --apply` 不作废（HEAD 就不作废快照，本刀不碰 `configcmd.go`）。取消语义不变：codex 登录状态命令算在这一家的识别里，期间父 ctx 被取消时，由 `takeModelSnapshot` 的循环头（后面几家不再识别）与 `printDiscoveredModels` 拍完之后那一处（整份丢掉、只打「识别被取消；下次 /add 会重新识别」）接住，两处都有用例（Z8 的 `not_last`、`last`）。
- **D7 快照之后从配置里删掉的 provider：照打。** 登录段只照快照打，不按当前 `r.cfg` 滤。理由（问过「同类的还有没有别处」）：快照里另外两样按 provider 记的东西——失败行（`printModelSnapshot` 的 `snap.failures` 循环）与识别表的行——都不按当前配置里有没有这个 provider 滤：失败行原样重印；表按当前配置滤的是「已经配成成员的模型」（`configuredAgentFor`），不是 provider。登录段与失败行同一个处置；段头写明是本会话的快照，`/add --refresh` 之后那一家就没了（Z7 第 3、4 步）。
- **D8 识别失败的那一家也有一行。** 登录状态在 `err` 判断之前判（A4），照样从截下的输出判，判不出就是查不出；codex 识别失败时登录状态命令照跑（只要 ctx 没到期），与首次运行相同（Z6）。
- **D9 codex 的超时与报错**：识别本身超时（`cctx` 已到期）时不跑登录状态命令（Z5 `discovery_timeout` 数调用次数）；执行器报错时不看它交回的输出（Z4 `exec_error` 故意带了 ChatGPT 那一句）。
- **D10 同类的还有没有别处**（逐个核过）：
  - 登录状态的说法与登录办法在生产代码里只有 `firstrun.go` 用（`firstRunLoginText` 两处、`firstRunLogin` 一处），B1-B3 全改成 repl 那一份。第三处（v1.1 补上，W1 m2）：同一个 `firstrun.go` 的 `msgFirstRunNoneFound`（HEAD `:57-62`）在一家都没找到时的安装说明里把四家的登录办法又写了一遍（「装好后运行 claude auth login」「运行 grok login」「运行 agy，按提示登录」「运行 codex login」）。它是首次运行的既有输出，本刀不动（首次运行的屏幕输出一个字节不变），改 `loginCommands` 时要同步它（§7 (j)）；§6 第 1 步 grep 核对 app 里再没有别处；`internal/tui` 的 Go 文件里没有「已登录」「没登录」「查不出」「登录」任何一个字面量（全文搜过）。
  - 装配 REPL 的生产代码只有 `app.go` 两处（B4、B5）；`internal/tui` 的测试与 `tui1*`、`rba_*`、`smx_*` 等用例直接 `repl.New`，不装执行器，不受影响。
  - `/add` 的另外两条出口（`/add +编号` 的 `cmdAddDiscovered`、`/add <id>`）不打识别结果，不打登录段；`cmdAddDiscovered` 读快照只读 `rows`，不碰 `logins`。
  - 横幅、`/providers`、`/help`、`printUnconfiguredCLIs`、识别失败的说法、「正在识别模型: …」一律不改（§1.2）。
  - 文档：`docs/README.md` 的 `/add` 与「第一次运行」两节、`14_MVP_CHECKLIST.md`，落地后由主代理订正（§6 第 4 步）。
- **D11 顺带补 OB2 审查剩下的**（19e 文件头）：
  - S1（provider 包新文件）加 K3：`grok_model_key_prefixed`（`obz Model 'grok-4.7' is using its own API key.`）、`grok_xai_api_key_prefixed`（`obz You are using XAI_API_KEY.`）、`grok_not_authenticated_prefixed`（`obz You are not authenticated.`），期望都是查不出；每行配一个只差句前 `obz ` 的正对照。§4 登记 Q1、Q2、Q3（19e 头点名的三种放宽），点名杀手原型实测变红。
  - T1：`oby_firstrun_app_test.go` 文件头的复用夹具清单去掉没直接用的 `obxFake`，补上直接用到的 `obxIn`、`obxEnv`。只改注释（原型上按标识符数过：`obxFake` 0 处，`obxIn` 3 处，`obxEnv` 1 处）。
- **D12 交付分块与跨文件约束**：
  - **块 A（`internal/repl`）**：A1-A5 是两个文件（`repl.go`、`addmodel.go`）里的五处补丁，A6 是一个新文件。只依赖 HEAD。只落块 A 时全量仍绿（原型核过：app 不装执行器，`/add` 输出与 HEAD 相同；首次运行仍用 app 自己那一份说法，与 A6 逐字相同）。
  - **块 B（`internal/app`）**：B1-B5 与授权的测试改动 T1、T2。依赖块 A（用到 `repl.LoginText`、`repl.LoginCommand`、`(*REPL).SetLoginStatusExec`）。
  - **跨文件约束**：B2、B1 是「挪」不是「改」：A6 的 `LoginText` 函数体、`loginCommands` 表必须与 HEAD 的 `firstRunLoginText`、`firstRunLogin` 逐字节相同（本文 A6 就是这样取的）；T2 的新期望逐字依赖 A6 的 `msgLoginHead` 与两种行格式；S3 的 Z12 依赖 B2 的写法（`var firstRunLoginText = repl.LoginText`）。写侧（A6）的字面量若改，读侧 T2、S2、S3 跟着改。
  - 测试腿的三个文件：S1 只依赖 HEAD；S2 依赖块 A；S3 依赖块 A 与块 B。
  - 行数：块 A 是 A6 新文件 112 行、A1-A5 的替换文本合计 42 行；块 B 是 B1-B5 与 T1、T2 的替换文本合计 20 行（v1.1：B2 的注释从两行变成三行）；两块合计约 174 行（不算锚点）。

#### 块 A：`internal/repl`（五处补丁、一个新文件；只依赖 HEAD）

**A1. `internal/repl/repl.go`：`REPL` 加字段 `loginStatus`**（HEAD `:122-123`）

```anchor internal/repl/repl.go
	detectCLIs func() []config.DetectedProvider
}
```

```replace
	detectCLIs func() []config.DetectedProvider
	// loginStatus 是 /add 跑登录状态命令用的执行器（#48，OB3，见 addlogin.go）。
	// 零值 nil 是不做登录检测、不打登录段；只经 SetLoginStatusExec 装，app 装 turn.RunLoginStatus。
	loginStatus provider.LoginStatusExec
}
```

**A2. `internal/repl/addmodel.go`：`modelSnapshot` 加字段 `logins`**（HEAD `:34-40`）

```anchor internal/repl/addmodel.go
// 只影响过滤；/add --refresh 才重拍。
type modelSnapshot struct {
	rows     []snapRow
	failures []snapFailure
	okCount  int
	custom   int
}
```

```replace
// 只影响过滤；/add --refresh 才重拍。
//
// logins 是各家的登录状态（OB3，addlogin.go），与识别同一次拍，顺序与 pids 相同；
// 没装登录状态执行器时为 nil。
type modelSnapshot struct {
	rows     []snapRow
	failures []snapFailure
	okCount  int
	custom   int
	logins   []snapLogin
}
```

**A3. `internal/repl/addmodel.go`：`takeModelSnapshot` 头注释补一段**（HEAD `:119-120`）

```anchor internal/repl/addmodel.go
// 才算取消（R14 late）。两处判断缺一不可。
func takeModelSnapshot(ctx context.Context, r *REPL, pids []string) *modelSnapshot {
```

```replace
// 才算取消（R14 late）。两处判断缺一不可。
//
// 装了登录状态执行器时（OB3），每家识别之后接着判登录状态，codex 还要另跑一次登录
// 状态命令；它算在这一家里，期间的取消同样由上面两处接住，整份快照丢掉。
func takeModelSnapshot(ctx context.Context, r *REPL, pids []string) *modelSnapshot {
```

**A4. `internal/repl/addmodel.go`：`takeModelSnapshot` 截下 stdout、判登录状态（`cancel` 挪到之后）**（HEAD `:127-129`）

```anchor internal/repl/addmodel.go
		cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout)
		models, err := provider.DiscoverModels(cctx, pid, r.cfg.Providers[pid].Command, r.discover)
		cancel()
```

```replace
		cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout)
		// 经一层闭包截下 stdout（DiscoverModels 不交回原始输出），登录状态从它判（OB3）。
		var stdout []byte
		capture := func(c context.Context, cmd provider.DiscoveryCommand) ([]byte, error) {
			out, err := r.discover(c, cmd)
			stdout = out
			return out, err
		}
		models, err := provider.DiscoverModels(cctx, pid, r.cfg.Providers[pid].Command, capture)
		// 识别失败的那一家也判。cancel 必须在这之后：登录状态命令与识别共用 cctx。
		if r.loginStatus != nil {
			snap.logins = append(snap.logins, snapLogin{provider: pid, info: snapshotLogin(cctx, r, pid, stdout)})
		}
		cancel()
```

**A5. `internal/repl/addmodel.go`：`printDiscoveredModels` 在识别表之后打登录段**（HEAD `:174-177`）

```anchor internal/repl/addmodel.go
		r.modelSnap = snap
	}
	return printModelSnapshot(r, r.modelSnap)
}
```

```replace
		r.modelSnap = snap
	}
	if err := printModelSnapshot(r, r.modelSnap); err != nil {
		return err
	}
	return printLoginStatus(r, r.modelSnap)
}
```

**A6. 新文件 `internal/repl/addlogin.go`**（整文件，112 行）

```file internal/repl/addlogin.go
package repl

import (
	"context"

	"council/internal/provider"
)

// addlogin.go 是 /add 识别结果之后的「各家的登录状态」（00_DECISIONS.md #48，OB3），以及
// 首次运行与 /add 共用的两样东西：四档登录状态的说法（LoginText）、各家的登录办法
// （LoginCommand）。判据全在 provider 包（LoginFromDiscovery、LoginStatusCommandFor、
// ParseLoginStatus），这里只取数与上屏。
//
// 登录状态是识别快照的一部分：与识别表同一次拍、随快照留到本会话结束，只有
// /add --refresh 或新会话重拍；/config reload --apply 不作废。

// msgLoginHead 是登录段的段头。
const msgLoginHead = "各家的登录状态（本会话的快照，/add --refresh 重新检查）:\n"

// loginCommands 是各家的登录办法。claude 取自 claude auth --help，grok、codex 取自
// 各自 --help 的 login 子命令；agy 的 --help 里没有登录相关的子命令，写兜底句。
var loginCommands = map[string]string{
	"claude": "claude auth login",
	"grok":   "grok login",
	"agy":    "agy（按提示登录）",
	"codex":  "codex login",
}

// LoginCommand 是一家的登录办法；首次运行的登录办法与 /add 登录段里没登录那一行用的都是它。
// 不认识的 id 返回空串。
func LoginCommand(providerID string) string {
	return loginCommands[providerID]
}

// LoginText 是登录状态的说法，四档之一；claude 已登录时带上订阅的原值，已登录（API key）
// 时带上 key 来源的原值（OB2）。首次运行的结果行与 /add 的登录段用的都是它。
func LoginText(l provider.LoginInfo) string {
	switch l.State {
	case provider.LoginYes:
		if l.Subscription != "" {
			return "已登录（订阅：" + l.Subscription + "）"
		}
		return "已登录"
	case provider.LoginAPIKey:
		if l.KeySource != "" {
			return "已登录（API key：" + l.KeySource + "）"
		}
		return "已登录（API key）"
	case provider.LoginNo:
		return "没登录"
	default:
		return "登录状态查不出"
	}
}

// SetLoginStatusExec 装上 /add 跑登录状态命令（今天只有 codex login status）用的执行器。
// 零值 nil 是不做登录检测：识别快照里没有登录状态，/add 不打登录段，也不多起任何进程，
// 所以直接 New 出来的 REPL（既有测试）输出不变；app 装配时装 turn.RunLoginStatus。
// 只许在 Run 或第一次 Submit 之前调。
func (r *REPL) SetLoginStatusExec(f provider.LoginStatusExec) {
	r.loginStatus = f
}

// snapLogin 是快照里一家的登录状态。
type snapLogin struct {
	provider string
	info     provider.LoginInfo
}

// snapshotLogin 判一家的登录状态，判据与首次运行的 firstRunCheck 相同：claude、grok 从识别
// 那一次截下的 stdout 判；codex 在识别之后、而且这一家的 ctx 还没到期时另跑一次登录状态命令
// （用配置里的 command），执行器报错就是查不出；其余查不出。ctx 是这一家识别用的那一个，
// 识别与登录状态命令共用一个 discoverTimeout。
func snapshotLogin(ctx context.Context, r *REPL, pid string, stdout []byte) provider.LoginInfo {
	info := provider.LoginFromDiscovery(pid, stdout)
	if sc, ok := provider.LoginStatusCommandFor(pid, r.cfg.Providers[pid].Command); ok && ctx.Err() == nil {
		if code, so, se, err := r.loginStatus(ctx, sc); err == nil {
			info = provider.ParseLoginStatus(pid, code, so, se)
		}
	}
	return info
}

// printLoginStatus 在识别表（连同 custom 行、失败行与 --refresh 提示）之后打登录段：
// 段头一行，然后快照里的每一家一行，顺序与「正在识别模型」相同。快照里没有登录状态
// （没装执行器）时什么都不打。
//
// 只照快照打，不按当前 r.cfg 滤：快照之后 /config reload --apply 删掉的 provider 照样
// 有一行，与失败行同一个处置（识别表按当前配置滤的是已经配成成员的模型，不是 provider）。
//
// 只有没登录的那一家在行末带登录办法与「登录之后 /add --refresh」：查不出的不带（agy 永远
// 查不出，每次都提示是噪声），已登录、已登录（API key）的不带。
func printLoginStatus(r *REPL, snap *modelSnapshot) error {
	if len(snap.logins) == 0 {
		return nil
	}
	if err := r.write(msgLoginHead); err != nil {
		return err
	}
	for _, l := range snap.logins {
		if l.info.State == provider.LoginNo {
			if err := r.printf("  %-6s  %s；在终端里运行 %s，登录之后 /add --refresh\n", l.provider, LoginText(l.info), LoginCommand(l.provider)); err != nil {
				return err
			}
			continue
		}
		if err := r.printf("  %-6s  %s\n", l.provider, LoginText(l.info)); err != nil {
			return err
		}
	}
	return nil
}
```

#### 块 B：`internal/app`（五处补丁、两处授权的测试改动；依赖块 A）

**B1. `internal/app/firstrun.go`：删掉 `firstRunLogin` 表（挪到 repl）**（HEAD `:44-53`）

```anchor internal/app/firstrun.go
// firstRunLogin 是各家的登录办法。claude 取自 claude auth --help，grok、codex 取自
// 各自 --help 的 login 子命令；agy 的 --help 里没有登录相关的子命令，写兜底句。
var firstRunLogin = map[string]string{
	"claude": "claude auth login",
	"grok":   "grok login",
	"agy":    "agy（按提示登录）",
	"codex":  "codex login",
}

// msgFirstRunNoneFound 是一家都没找到时的说明。含 %，用 io.WriteString 写：
```

```replace
// msgFirstRunNoneFound 是一家都没找到时的说明。含 %，用 io.WriteString 写：
```

**B2. `internal/app/firstrun.go`：`firstRunLoginText` 就是 `repl.LoginText`**（HEAD `:72-91`）

```anchor internal/app/firstrun.go
// firstRunLoginText 是结果行末尾的登录状态，四档之一；claude 已登录时带上订阅的原值，
// 已登录（API key）时带上 key 来源的原值（OB2）。
func firstRunLoginText(l provider.LoginInfo) string {
	switch l.State {
	case provider.LoginYes:
		if l.Subscription != "" {
			return "已登录（订阅：" + l.Subscription + "）"
		}
		return "已登录"
	case provider.LoginAPIKey:
		if l.KeySource != "" {
			return "已登录（API key：" + l.KeySource + "）"
		}
		return "已登录（API key）"
	case provider.LoginNo:
		return "没登录"
	default:
		return "登录状态查不出"
	}
}
```

```replace
// firstRunLoginText 是结果行末尾的登录状态。它就是 repl.LoginText（OB3）：四档说法只有
// 那一份，/add 的登录段用的是同一个函数。结果之后列出的登录办法同样只有 repl.LoginCommand
// 那一份；msgFirstRunNoneFound 里的安装说明另写着各家的登录办法，首次运行的输出不变，不动它。
var firstRunLoginText = repl.LoginText
```

**B3. `internal/app/firstrun.go`：登录办法用 `repl.LoginCommand`**（HEAD `:182`）

```anchor internal/app/firstrun.go
			fmt.Fprintf(w, "  %-6s  %s\n", id, firstRunLogin[id])
```

```replace
			fmt.Fprintf(w, "  %-6s  %s\n", id, repl.LoginCommand(id))
```

**B4. `internal/app/app.go`：TUI 分支装登录状态执行器**（HEAD `:254-255`）

```anchor internal/app/app.go
		r.SetCLIDetector(config.DetectProviders)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
```

```replace
		r.SetCLIDetector(config.DetectProviders)
		r.SetLoginStatusExec(turn.RunLoginStatus)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
```

**B5. `internal/app/app.go`：行式分支装登录状态执行器**（HEAD `:261-262`）

```anchor internal/app/app.go
		r.SetCLIDetector(config.DetectProviders)
		err = r.Run(ctx)
```

```replace
		r.SetCLIDetector(config.DetectProviders)
		r.SetLoginStatusExec(turn.RunLoginStatus)
		err = r.Run(ctx)
```

**T1. `internal/app/oby_firstrun_app_test.go`：（授权，只改注释）文件头的复用夹具清单**（HEAD `:7`）

```anchor internal/app/oby_firstrun_app_test.go
// obxFixture、obxFake、obxSetTimeout、obxSub、obxWantFile、obxReadFile、obxHead、obxLoginHead、obxDone、
```

```replace
// obxFixture、obxSetTimeout、obxSub、obxWantFile、obxReadFile、obxIn、obxEnv、obxHead、obxLoginHead、obxDone、
```

**T2. `internal/app/obx_firstrun_app_test.go`：（授权）A10 的期望加上登录段**（HEAD `:716-719`）

```anchor internal/app/obx_firstrun_app_test.go
			wantTail := "EFFORTS 列是它支持的档\n"
			if mode == "hint" {
				wantTail += hint("grok")
			}
```

```replace
			// OB3 起识别表之后、提示之前是各家的登录状态（配置里写了的几家，字典序；OB3 契约 T2）。
			wantTail := "EFFORTS 列是它支持的档\n各家的登录状态（本会话的快照，/add --refresh 重新检查）:\n  claude  已登录（订阅：Claude Obx）\n"
			if mode == "hint" {
				wantTail += hint("grok")
			} else {
				wantTail += "  grok    没登录；在终端里运行 grok login，登录之后 /add --refresh\n"
			}
```

锚点与替换的取法：每个代码块的内容是两道围栏之间的各行，以 LF 连接，末尾再加一个 LF；`file` 块同样（整文件就是块内容加一个 LF）。锚点由脚本从 HEAD `35520ff` 真文件逐行取出，在该文件里恰好出现一次，同一文件的锚点互不重叠；同一文件的几个补丁互不依赖先后。标题里的 `:行号` 是锚点在 HEAD 文件里的起止行。

### 1.2 不做

- 不在启动时检测登录状态；不改横幅、`/providers`、`/help`；不加新命令、新 flag。
- 不做 key 能不能用的连通性检测（「配了 key」不等于「key 能用」，19e §7 (g)）；不读、不写任何 CLI 的配置或凭据；不跑 `login` / `logout`。
- 不改识别失败那几行的说法（包括「一个模型都没列出来（没登录？）」）；不改「正在识别模型: …」那一行；不改识别表、custom 行、`--refresh` 提示、`printUnconfiguredCLIs`。
- 不改 `internal/provider` 的任何生产代码（判据不动）；不改 `internal/turn`、`internal/config`、`internal/tui`、`internal/store`、`cmd/council` 的任何文件；`internal/repl` 只改 `repl.go`、`addmodel.go`，加 `addlogin.go`；`internal/app` 只改 `firstrun.go`、`app.go`。
- 首次运行的屏幕输出一个字节不变。
- **既有 `_test.go` 只许改 T1、T2**，其余一律不改（obx、oby、mdx、cxh、cox 的装置只读复用）。

---

## 2. 验收（测试腿新写的文件）

三个新文件：`internal/provider/obz_login_test.go`（`package provider`）、`internal/repl/obz_addlogin_test.go`（`package repl`）、`internal/app/obz_addlogin_app_test.go`（`package app`）。包级标识符一律 `obz` 开头；**不许新增 `TestMain`**；不许改任何既有测试文件（T1、T2 由实现腿交付）。三个文件照下面逐字写（§4 按名字点杀手）。用例编号 Z1-Z8 在 repl、Z10-Z13 在 app（Z13 是 v1.1 加的）、K3 在 provider；Z9 空着不用。

纪律（与 19d §2 开头相同）：
- 字面量整行相等：屏幕输出整段或整行 `==` 比较（`mdxWantLines` 逐行、`strings.HasSuffix` 整段结尾）。只有 Z11（TUI）比的是去掉 ANSI 与空格之后的包含关系（屏幕是 bubbletea 渲染的，没有逐字节的整行）。
- 每条都能为假：每条用例写明它对哪条变异红（§4 的「杀手」列是原型实测）；缺席断言配同一装置的正对照（Z3 的 g0 以 g 为正对照；Z10、Z11 的标记文件以 Run 之前「不存在」为前提；K3 每行配只差句前 `obz ` 的正对照；v1.1：Z8 按次记下的「ctx 已结束」以没有钩子的那一次 false 为正对照，Z5 `shared_deadline` 以识别那一侧恰好一个 deadline 为 `Fatalf` 前提）；时间断言配输出断言（Z5 `hang` 在 Q6 下输出先变）。
- repl 的用例不起任何进程：识别用 `mdxExec`（`r.discover`），登录状态用 setter 装的 `obzStatusExec`。
- app 的用例一律经 `obxIsolate`（第一件事是 `obxGuardRealHome`）；另起 goroutine 调 `Run` 的，行式经 `obxRun`，TUI 照 A11 用 `obxDrain` 收尾；假 CLI 一律是临时目录里只用 cmd 内建命令的 `.cmd`，不设 `COUNCIL_FAKE_CLI`，不调用任何真 CLI。Z10、Z11 用显式配置，不走首次运行（`errOut` 为空）；Z13（v1.1）测的就是首次运行，不给配置路径，默认配置位置落在 `obxIsolate` 的临时 home 下，与 obx 的首次运行用例同一个装置。

### 2.1 `internal/provider/obz_login_test.go`（S1）

- **K3 `TestObzGrokPrefixedLines`**：表驱动，`LoginFromDiscovery("grok", []byte(stdout)) == want`。三行句前多 `obz ` 的期望查不出（19e 头点名的三行），三行不带前缀的正对照（CPA 那一句、`XAI_API_KEY` 那一句是已登录（API key），没登录那一句是没登录）。

```file internal/provider/obz_login_test.go
package provider

import "testing"

// obz_login_test.go 是 OB3（契约 §2.1）补 OB2 审查剩下的三行负对照（契约 19e 文件头）：grok 首行的三句
// 句前多了字时都是查不出。每行配一个只差句前 "obz " 的正对照（同一张表、同一个断言）。

// TestObzGrokPrefixedLines 是 K3。
// 【判别力】grok_model_key_prefixed 对 Q1（CPA 那一句放宽成只看后缀加长度）；grok_xai_api_key_prefixed 对
// Q2（XAI_API_KEY 那一句整句相等放宽成 HasSuffix）；grok_not_authenticated_prefixed 对 Q3（没登录那一句
// 整句相等放宽成 HasSuffix）。三行不带前缀的是正对照：夹具与判据都对时它们不是查不出。
func TestObzGrokPrefixedLines(t *testing.T) {
	for _, c := range []struct {
		name, stdout string
		want         LoginInfo
	}{
		{"grok_model_key", "Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n", LoginInfo{State: LoginAPIKey}},
		{"grok_model_key_prefixed", "obz Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n", LoginInfo{}},
		{"grok_xai_api_key", "You are using XAI_API_KEY.\n", LoginInfo{State: LoginAPIKey}},
		{"grok_xai_api_key_prefixed", "obz You are using XAI_API_KEY.\n", LoginInfo{}},
		{"grok_not_authenticated", "You are not authenticated.\n", LoginInfo{State: LoginNo}},
		{"grok_not_authenticated_prefixed", "obz You are not authenticated.\n", LoginInfo{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := LoginFromDiscovery("grok", []byte(c.stdout)); got != c.want {
				t.Errorf("LoginFromDiscovery(grok, %q) = %+v，期望 %+v", c.stdout, got, c.want)
			}
		})
	}
}
```

### 2.2 `internal/repl/obz_addlogin_test.go`（S2）

- **Z1 `TestObzLoginText`**：`LoginText` 四档六种说法逐字；`LoginCommand` 四家与一个不认识的 id（空串）。
- **Z2 `TestObzSetLoginStatusExecZeroValue`**：`New` 出来的 `loginStatus` 是 nil；setter 装得上。
- **Z3 `TestObzAddLoginSection`**：MD1 的配置（agy、claude、grok）；claude 夹具加上订阅 `Claude Obz`，grok 夹具首行换成 `You are not authenticated.`（模型行不变）；两套装置都装 OB1 的检测器桩。g0 不装执行器：没有段头。g 装：输出整段等于 g0 的输出在最后三行（codex 的提示）之前插入登录段——`agy` 查不出（不带登录办法）、`claude` 已登录（订阅：Claude Obz）、`grok` 没登录（带登录办法）。三家都不跑登录状态命令。
- **Z4 `TestObzAddLoginCodex`**：CXH 的配置（codex、grok），五个子测试是 codex 登录状态命令的五种结果（已登录、已登录（API key）、没登录、执行器报错、认不出）；grok 用 MD1 的原夹具（CPA 那一句），已登录（API key）。识别部分整段等于 CXH 的 R1 块加登录段；命令恰好跑一次、`{Path: cxh-codex.exe, Args: [login status]}`、ctx 带 deadline。
- **Z5 `TestObzAddLoginSharedTimeout`**：`hang`（命令挂住，被这一家的 200 ms 打断，查不出，`/add` 小于 2 s）；`discovery_timeout`（codex 识别本身超时，登录状态命令一次都不跑）；`shared_deadline`（v1.1，W1 M1：包一层 `g.r.discover`，记下识别 ctx 的 deadline、`time.Sleep(50 * time.Millisecond)` 再返回；登录状态执行器记下的 deadline 与它 `Equal`。识别那一侧恰好一个 deadline 是 `Fatalf` 前提，挡住两边都没有 deadline 时零值相等的空绿；失败消息印出两个 deadline）。
- **Z6 `TestObzAddLoginDiscoveryFailed`**：`grok_empty`（grok 只打了没登录那一句、一个模型都没有：失败行照旧，登录段里 grok 没登录）；`grok_exit_error`（v1.1，W1 m1：识别夹具换成返回没登录那一句加一个模型、同时带 `errors.New("exit code 1")` 的闭包，失败行照旧，登录段里 grok 没登录、带登录办法）；`codex_failed`（codex 识别报错，登录状态命令照跑，已登录）。
- **Z7 `TestObzAddLoginSnapshot`**：第 1 步拍快照；第 2 步回应换成没登录，再 `/add` 登录段逐字不变、不重跑；第 3 步配置换成只有 codex、`/config reload --apply`，快照不作废，登录段仍有 grok（D7）；第 4 步 `/add --refresh` 重拍，只剩 codex、没登录，命令共跑两次。
- **Z8 `TestObzAddLoginCanceled`**：登录状态命令期间取消。`not_last`（codex、grok，取消落在 codex）：识别部分只有两行（「正在识别模型: codex, grok」「识别被取消；下次 /add 会重新识别」），快照为 nil，grok 没有识别；`last`（只有 codex）：同样只打两行。两种之后再 `/add` 都重新识别、登录段照常。v1.1（W1 M1）：假执行器在钩子之后按次记下 `ctx.Err() != nil`（切片，同 deadlines 的记法；单个 bool 会被第二次 `/add` 覆盖成 false）；第一次 `/add` 之后立刻断言记下的是 `[true]`（放在第二次 `/add` 的几处 `Fatalf` 之前），第二次之后断言整份是 `[true false]`（没有钩子的那一次是 false，是正对照）。
- **`obzWantCodexCalls` 的措辞（v1.1）**：它只查 ctx 有没有 deadline，报错消息改成只说这一件（v1 的消息说「应当与识别共用这一家的 discoverTimeout」，代码不查）；deadline 是不是同一个由 `shared_deadline` 查。

```file internal/repl/obz_addlogin_test.go
package repl

// obz_addlogin_test.go 是 OB3（00_DECISIONS.md #48：/add 的识别结果之后显示各家的登录状态）在 repl 包的验收：
// Z1 说法与登录办法、Z2 零值、Z3 登录段的位置与三种状态、Z4 codex 的登录状态命令、Z5 共用超时、
// Z6 识别失败的那一家、Z7 快照语义、Z8 取消。
// 装置只读复用 mdx_addmodel_fixture_test.go（mdxNewRig、mdxRig、mdxResp 与 g.exec 的 set、count，mdxWantLines、
// mdxHasLine、mdxReplaceOnce、mdxSplit、mdxConfigText、mdxConfigTemplate、mdxGrokOnlyTemplate、mdxClaudeFixture、
// mdxGrokFixture、mdxGrokPath、mdxAppliedLine）、cxh_fixture_test.go（cxhNewRig、cxhConfigTemplate、
// cxhCodexOnlyTemplate、cxhCodexPath、cxhCodexFixture、cxhListBlock、cxhGrokDownBlock、cxhOneGpt55）与
// obx_cliadvice_test.go（obxDetector、obxList、obxHintLines）。登录状态命令一律由 obzStatusExec 假执行器回应，
// 不起任何进程。

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/provider"
)

// obzHead 是登录段的段头（契约 §3）。
const obzHead = "各家的登录状态（本会话的快照，/add --refresh 重新检查）:"

// 登录段里各家的行（契约 §3 的格式套上各档的说法）。
const (
	obzAgyUnknown   = "  agy     登录状态查不出"
	obzClaudeSub    = "  claude  已登录（订阅：Claude Obz）"
	obzGrokNo       = "  grok    没登录；在终端里运行 grok login，登录之后 /add --refresh"
	obzGrokKey      = "  grok    已登录（API key）"
	obzCodexYes     = "  codex   已登录"
	obzCodexKey     = "  codex   已登录（API key）"
	obzCodexNo      = "  codex   没登录；在终端里运行 codex login，登录之后 /add --refresh"
	obzCodexUnknown = "  codex   登录状态查不出"
)

// obzCanceledLine 是识别被取消时那一行（既有字面量）。
const obzCanceledLine = "识别被取消；下次 /add 会重新识别"

// obzCodexOnlyBlock 是只有 codex 一家、识别出 cxhOneGpt55 那一条（已配成成员）时的识别部分（既有字面量）。
var obzCodexOnlyBlock = []string{"正在识别模型: codex", "识别出的模型都已经在配置里了"}

// obzRefreshHint 是有失败时失败行之后那一行（既有字面量）。
const obzRefreshHint = "失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh"

// obzStatusResp 是假登录状态执行器的回应。wait 为真时先等 ctx 结束（返回 ctx.Err()），
// 3 s 还没结束就照其余几项返回：没被这一家的超时约束时，结果看得见。
type obzStatusResp struct {
	code           int
	stdout, stderr string
	err            error
	wait           bool
}

// obzStatusExec 是 provider.LoginStatusExec 的假实现。每次调用的五步顺序冻结：
// 记录调用、ctx 有没有 deadline 与 deadline 的值（加锁）-> 钩子 -> 按次记下此刻 ctx 是否已结束（加锁）
// -> ctx 已结束就返回 ctx.Err() -> 按 resp 返回。
type obzStatusExec struct {
	mu         sync.Mutex
	calls      []provider.DiscoveryCommand
	deadlines  []bool
	deadlineAt []time.Time
	canceled   []bool
	resp       obzStatusResp
	hook       func()
}

func (e *obzStatusExec) run(ctx context.Context, c provider.DiscoveryCommand) (int, []byte, []byte, error) {
	e.mu.Lock()
	e.calls = append(e.calls, c)
	at, has := ctx.Deadline()
	e.deadlines = append(e.deadlines, has)
	e.deadlineAt = append(e.deadlineAt, at)
	resp, hook := e.resp, e.hook
	e.mu.Unlock()
	if hook != nil {
		hook()
	}
	e.mu.Lock()
	e.canceled = append(e.canceled, ctx.Err() != nil)
	e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, nil, nil, err
	}
	if resp.wait {
		select {
		case <-ctx.Done():
			return 0, nil, nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return resp.code, []byte(resp.stdout), []byte(resp.stderr), resp.err
}

func (e *obzStatusExec) set(r obzStatusResp) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.resp = r
}

func (e *obzStatusExec) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

// obzCanceledCopy 是按次记下的「钩子之后 ctx 是否已结束」的副本。
func obzCanceledCopy(e *obzStatusExec) []bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]bool(nil), e.canceled...)
}

// obzInstall 给装置装上假登录状态执行器（经 SetLoginStatusExec）。
func obzInstall(g *mdxRig, r obzStatusResp) *obzStatusExec {
	e := &obzStatusExec{resp: r}
	g.r.SetLoginStatusExec(e.run)
	return e
}

// obzCodexCmd 是 codex 登录状态命令应有的样子：配置里的 command，参数 login status，stdin 为空。
var obzCodexCmd = provider.DiscoveryCommand{Path: cxhCodexPath, Args: []string{"login", "status"}}

// obzWantCodexCalls 断言假执行器恰好被调 n 次、每次都是 obzCodexCmd、ctx 都带 deadline。
// 它只查有没有 deadline；与识别的 deadline 是不是同一个，由 Z5 的 shared_deadline 查。
func obzWantCodexCalls(t *testing.T, e *obzStatusExec, n int) {
	t.Helper()
	e.mu.Lock()
	calls := append([]provider.DiscoveryCommand(nil), e.calls...)
	deadlines := append([]bool(nil), e.deadlines...)
	e.mu.Unlock()
	if len(calls) != n {
		t.Fatalf("登录状态命令应当恰好跑 %d 次，实际 %d 次：%+v", n, len(calls), calls)
	}
	for i, c := range calls {
		if !reflect.DeepEqual(c, obzCodexCmd) {
			t.Errorf("第 %d 次登录状态命令是 %+v，期望 %+v", i+1, c, obzCodexCmd)
		}
		if !deadlines[i] {
			t.Errorf("第 %d 次登录状态命令的 ctx 没有 deadline", i+1)
		}
	}
}

// obzClaudeSubFixture 是 mdxClaudeFixture 在 council-models 回应里加上 account（订阅 Claude Obz）。
func obzClaudeSubFixture(t *testing.T) string {
	t.Helper()
	return mdxReplaceOnce(t, mdxClaudeFixture, `]},"pending_permission_requests"`, `],"account":{"subscriptionType":"Claude Obz"}},"pending_permission_requests"`)
}

// obzGrokNoFixture 是 mdxGrokFixture 把首行换成 You are not authenticated.（模型行不变）。
func obzGrokNoFixture(t *testing.T) string {
	t.Helper()
	return mdxReplaceOnce(t, mdxGrokFixture, "Model 'grok-4.7' is using its own API key.", "You are not authenticated.")
}

// obzSection 拼出登录段：段头加各行。
func obzSection(rows ...string) []string {
	return append([]string{obzHead}, rows...)
}

// TestObzLoginText 是 Z1：LoginText 的四档六种说法与 LoginCommand 的四家（不认识的 id 是空串）。
// 【判别力】Q20-Q23（说法）、Q24（登录办法）。
func TestObzLoginText(t *testing.T) {
	for _, c := range []struct {
		name string
		in   provider.LoginInfo
		want string
	}{
		{"yes_sub", provider.LoginInfo{State: provider.LoginYes, Subscription: "Claude Obz"}, "已登录（订阅：Claude Obz）"},
		{"yes", provider.LoginInfo{State: provider.LoginYes}, "已登录"},
		{"key_source", provider.LoginInfo{State: provider.LoginAPIKey, KeySource: "ANTHROPIC_API_KEY"}, "已登录（API key：ANTHROPIC_API_KEY）"},
		{"key", provider.LoginInfo{State: provider.LoginAPIKey}, "已登录（API key）"},
		{"no", provider.LoginInfo{State: provider.LoginNo}, "没登录"},
		{"unknown", provider.LoginInfo{}, "登录状态查不出"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := LoginText(c.in); got != c.want {
				t.Errorf("LoginText(%+v) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
	for id, want := range map[string]string{
		"claude": "claude auth login",
		"grok":   "grok login",
		"agy":    "agy（按提示登录）",
		"codex":  "codex login",
		"obz":    "",
	} {
		if got := LoginCommand(id); got != want {
			t.Errorf("LoginCommand(%q) = %q，期望 %q", id, got, want)
		}
	}
}

// TestObzSetLoginStatusExecZeroValue 是 Z2：直接 New 出来的 REPL 不做登录检测；SetLoginStatusExec 装得上。
// 【判别力】Q12（setter 不赋值）。
func TestObzSetLoginStatusExecZeroValue(t *testing.T) {
	r := New(nil, nil, nil, strings.NewReader(""), nil, RulesContext{})
	if r.loginStatus != nil {
		t.Errorf("New 出来的 REPL 的 loginStatus 应当是 nil")
	}
	e := &obzStatusExec{}
	r.SetLoginStatusExec(e.run)
	if r.loginStatus == nil {
		t.Errorf("SetLoginStatusExec 之后 loginStatus 不该是 nil")
	}
}

// TestObzAddLoginSection 是 Z3：MD1 的配置（agy、claude、grok），claude 已登录（订阅）、grok 没登录、agy 查不出。
// g0 不装执行器、g 装；两者都装 OB1 的检测器桩（codex 装了、配置里没有，末尾三行提示）。
// g 的输出等于 g0 的输出在最后三行提示之前插入登录段；g0 没有登录段（g 是它的正对照）。
// 三家都不跑登录状态命令（只有 codex 跑）。
// 【判别力】Q4、Q5、Q12、Q13、Q14、Q15、Q16、Q22、Q23、Q25。
func TestObzAddLoginSection(t *testing.T) {
	setup := func(withExec bool) (*mdxRig, *obzStatusExec) {
		g := mdxNewRig(t, mdxConfigTemplate)
		g.exec.set(g.r.cfg.Providers["claude"].Command, mdxResp{out: []byte(obzClaudeSubFixture(t))})
		g.exec.set(mdxGrokPath, mdxResp{out: []byte(obzGrokNoFixture(t))})
		calls := 0
		g.r.SetCLIDetector(obxDetector(obxList, &calls))
		var e *obzStatusExec
		if withExec {
			e = obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		}
		return g, e
	}
	g0, _ := setup(false)
	base := g0.run("/add")
	if mdxHasLine(base, obzHead) {
		t.Errorf("不装执行器时不该有登录段：%q", base)
	}
	if len(base) < 3 {
		t.Fatalf("前提不成立：g0 的 /add 输出不足 3 行：%q", base)
	}
	mdxWantLines(t, "前提：g0 的最后三行是 codex 的提示", base[len(base)-3:], obxHintLines(g0.cfgPath)[3:])

	g, e := setup(true)
	got := g.run("/add")
	want := append([]string(nil), base[:len(base)-3]...)
	want = append(want, obzSection(obzAgyUnknown, obzClaudeSub, obzGrokNo)...)
	want = append(want, obxHintLines(g.cfgPath)[3:]...)
	mdxWantLines(t, "装了执行器的 /add", got, want)
	if n := e.count(); n != 0 {
		t.Errorf("agy、claude、grok 都不跑登录状态命令，实际跑了 %d 次", n)
	}
}

// TestObzAddLoginCodex 是 Z4：CXH 的配置（codex、grok），codex 的登录状态命令五种结果；grok 是 MD1 的原夹具
// （经 CPA 的首行），已登录（API key）。识别部分整段等于 CXH 的 R1 块加登录段；命令恰好跑一次、形状对、ctx 带 deadline。
// exec_error 的回应同时带了 ChatGPT 那一句：执行器报错时不看输出（chatgpt 是只差 err 的正对照）。
// 【判别力】五个子测试都对 Q4、Q6、Q7、Q11、Q12、Q14、Q16、Q20、Q25；chatgpt、api_key、not_logged_in 另对 Q10；
// exec_error 另对 Q9；not_logged_in 另对 Q15、Q23、Q24。
func TestObzAddLoginCodex(t *testing.T) {
	for _, c := range []struct {
		name string
		resp obzStatusResp
		row  string
	}{
		{"chatgpt", obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"}, obzCodexYes},
		{"api_key", obzStatusResp{code: 0, stderr: "Logged in using an API key - sk-obz***\n"}, obzCodexKey},
		{"not_logged_in", obzStatusResp{code: 1, stderr: "Not logged in\n"}, obzCodexNo},
		{"exec_error", obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n", err: errors.New("obz exec failed")}, obzCodexUnknown},
		{"unparseable", obzStatusResp{code: 0, stdout: "obz\n"}, obzCodexUnknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
			e := obzInstall(g, c.resp)
			got := g.afterCandidates(g.run("/add"))
			want := append(mdxSplit(cxhListBlock), obzSection(c.row, obzGrokKey)...)
			mdxWantLines(t, "识别部分", got, want)
			obzWantCodexCalls(t, e, 1)
		})
	}
}

// TestObzAddLoginSharedTimeout 是 Z5：登录状态命令与识别共用这一家的 discoverTimeout（装置是 200 ms）。
// hang：命令挂住，这一家的 ctx 到期时被打断，codex 是查不出，/add 远在假执行器 3 s 兜底之前返回；
// 没被约束时 3 s 后返回 ChatGPT 那一句，那一行会变成已登录。
// discovery_timeout：codex 的识别本身超时，ctx 已到期，登录状态命令一次都不跑。
// shared_deadline：识别拖 50 ms 再返回，登录状态命令拿到的 deadline 与识别的 deadline 相等（Equal），
// 不是另起的一段。
// 【判别力】hang 对 Q6、Q7、Q11、Q12、Q14、Q16、Q25；discovery_timeout 对 Q6、Q8、Q12、Q14、Q16、Q17、Q25；
// shared_deadline 对 Q6、Q7、Q10、Q11、Q12、Q16、Q25、Q32、Q34（Q34 只有它杀得到；去掉那 50 ms 时，
// 原型上它连跑 5 次都杀不到 Q32、Q34）。
func TestObzAddLoginSharedTimeout(t *testing.T) {
	t.Run("hang", func(t *testing.T) {
		g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhOneGpt55)
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n", wait: true})
		start := time.Now()
		got := g.afterCandidates(g.run("/add"))
		el := time.Since(start)
		want := append([]string(nil), obzCodexOnlyBlock...)
		want = append(want, obzSection(obzCodexUnknown)...)
		mdxWantLines(t, "识别部分", got, want)
		if el >= 2*time.Second {
			t.Errorf("/add 用了 %v，应当小于 2s（登录状态命令受这一家的 200ms 约束）", el)
		}
		obzWantCodexCalls(t, e, 1)
	})
	t.Run("discovery_timeout", func(t *testing.T) {
		g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhOneGpt55)
		g.exec.set(cxhCodexPath, mdxResp{timeout: true})
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		got := g.afterCandidates(g.run("/add"))
		want := []string{
			"正在识别模型: codex",
			"识别 codex 的模型失败: 超时（200ms 内没有回来）",
			obzRefreshHint,
		}
		want = append(want, obzSection(obzCodexUnknown)...)
		mdxWantLines(t, "识别部分", got, want)
		if n := e.count(); n != 0 {
			t.Errorf("识别超时之后这一家的 ctx 已到期，登录状态命令不该跑，实际跑了 %d 次", n)
		}
	})
	t.Run("shared_deadline", func(t *testing.T) {
		g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhOneGpt55)
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		var mu sync.Mutex
		var disc []time.Time
		inner := g.r.discover
		g.r.discover = func(c context.Context, cmd provider.DiscoveryCommand) ([]byte, error) {
			d, ok := c.Deadline()
			mu.Lock()
			if ok {
				disc = append(disc, d)
			}
			mu.Unlock()
			out, err := inner(c, cmd)
			// 识别拖 50 ms 再返回：登录状态命令若另起一段新的超时，它的 deadline 会比识别的晚约 50 ms。
			// 不拖时两个 deadline 在 Windows 的时钟精度下可能相等，比不出来。
			time.Sleep(50 * time.Millisecond)
			return out, err
		}
		got := g.afterCandidates(g.run("/add"))
		want := append([]string(nil), obzCodexOnlyBlock...)
		want = append(want, obzSection(obzCodexYes)...)
		mdxWantLines(t, "识别部分", got, want)
		obzWantCodexCalls(t, e, 1)
		mu.Lock()
		discAt := append([]time.Time(nil), disc...)
		mu.Unlock()
		if len(discAt) != 1 {
			t.Fatalf("前提不成立：识别应当恰好一次、ctx 带 deadline，实际记下 %d 个 deadline", len(discAt))
		}
		e.mu.Lock()
		loginAt := e.deadlineAt[0]
		e.mu.Unlock()
		if !loginAt.Equal(discAt[0]) {
			t.Errorf("登录状态命令的 deadline 应当就是识别的 deadline：识别 %v，登录状态命令 %v", discAt[0], loginAt)
		}
	})
}

// TestObzAddLoginDiscoveryFailed 是 Z6：识别失败的那一家照样有一行。
// grok_empty：grok 只打了没登录那一句、一个模型都没有，识别失败，登录状态从截下的 stdout 判出没登录。
// grok_exit_error：grok 的识别报错（exit code 1）但 stdout 有没登录那一句与一个模型，登录状态照样从 stdout
// 判出没登录（与首次运行同一个判据，app 包的 Z13 是首次运行那一侧）。
// codex_failed：codex 的识别报错，登录状态命令照样跑，已登录。
// 【判别力】grok_empty 对 Q4、Q12、Q15、Q16、Q17、Q23；grok_exit_error 对 Q4、Q12、Q15、Q16、Q17、Q23、Q35
// （Q35 只有它杀得到）；codex_failed 对 Q6、Q7、Q10、Q11、Q12、Q16、Q17、Q25。
func TestObzAddLoginDiscoveryFailed(t *testing.T) {
	t.Run("grok_empty", func(t *testing.T) {
		g := mdxNewRig(t, mdxGrokOnlyTemplate)
		g.exec.set(mdxGrokPath, mdxResp{out: []byte("You are not authenticated.\n\nAvailable models:\n")})
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		got := g.afterCandidates(g.run("/add"))
		want := []string{
			"正在识别模型: grok",
			"识别 grok 的模型失败: 一个模型都没列出来（没登录？）",
			obzRefreshHint,
		}
		want = append(want, obzSection(obzGrokNo)...)
		mdxWantLines(t, "识别部分", got, want)
		if n := e.count(); n != 0 {
			t.Errorf("grok 不跑登录状态命令，实际跑了 %d 次", n)
		}
	})
	t.Run("grok_exit_error", func(t *testing.T) {
		g := mdxNewRig(t, mdxGrokOnlyTemplate)
		g.r.discover = func(context.Context, provider.DiscoveryCommand) ([]byte, error) {
			return []byte("You are not authenticated.\n\nAvailable models:\n  * grok-obz (default)\n"), errors.New("exit code 1")
		}
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		got := g.afterCandidates(g.run("/add"))
		want := []string{
			"正在识别模型: grok",
			"识别 grok 的模型失败: model discovery command failed: exit code 1",
			obzRefreshHint,
		}
		want = append(want, obzSection(obzGrokNo)...)
		mdxWantLines(t, "识别部分", got, want)
		if n := e.count(); n != 0 {
			t.Errorf("grok 不跑登录状态命令，实际跑了 %d 次", n)
		}
	})
	t.Run("codex_failed", func(t *testing.T) {
		g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhOneGpt55)
		g.exec.set(cxhCodexPath, mdxResp{err: errors.New("obz codex down")})
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		got := g.afterCandidates(g.run("/add"))
		want := []string{
			"正在识别模型: codex",
			"识别 codex 的模型失败: model discovery command failed: obz codex down",
			obzRefreshHint,
		}
		want = append(want, obzSection(obzCodexYes)...)
		mdxWantLines(t, "识别部分", got, want)
		obzWantCodexCalls(t, e, 1)
	})
}

// TestObzAddLoginSnapshot 是 Z7：登录状态随快照留到本会话结束。
// 第 2 步：执行器的回应换成没登录，再 /add 不重新识别、不重跑命令，登录段逐字不变。
// 第 3 步：配置里删掉 grok（换成只有 codex 的配置）并 /config reload --apply，快照不作废，登录段仍有 grok 那一行
// （只照快照打，不按当前配置滤）。
// 第 4 步：/add --refresh 重拍，只剩 codex，而且是没登录。
// 【判别力】Q18（登录段按当前配置滤）、Q19（reload 作废快照）；第 4 步钉 --refresh 重跑命令。另有 Q4、Q6、Q7、
// Q10、Q11、Q12、Q14、Q15、Q16、Q20、Q23、Q24、Q25。
func TestObzAddLoginSnapshot(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
	e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
	first := obzSection(obzCodexYes, obzGrokKey)

	got := g.afterCandidates(g.run("/add"))
	mdxWantLines(t, "第 1 步", got, append(mdxSplit(cxhListBlock), first...))
	obzWantCodexCalls(t, e, 1)
	discovered := g.exec.count()

	e.set(obzStatusResp{code: 1, stderr: "Not logged in\n"})
	got = g.afterCandidates(g.run("/add"))
	mdxWantLines(t, "第 2 步", got, append(mdxSplit(cxhListBlock)[1:], first...))
	obzWantCodexCalls(t, e, 1)

	g.writeFile([]byte(mdxConfigText(cxhCodexOnlyTemplate, g.dir)))
	reload := g.run("/config reload --apply")
	if !mdxHasLine(reload, mdxAppliedLine) {
		t.Fatalf("前提不成立：/config reload --apply 没有应用：%q", reload)
	}
	got = g.run("/add")
	if mdxHasLine(got, "正在识别模型: codex") || mdxHasLine(got, "正在识别模型: codex, grok") {
		t.Errorf("/config reload --apply 不该作废快照：%q", got)
	}
	if len(got) < 3 {
		t.Fatalf("第 3 步的输出不足 3 行：%q", got)
	}
	mdxWantLines(t, "第 3 步的最后三行", got[len(got)-3:], first)
	obzWantCodexCalls(t, e, 1)
	if n := g.exec.count(); n != discovered {
		t.Errorf("第 3 步不该重新识别：识别次数从 %d 变成 %d", discovered, n)
	}

	// 只有 codex 一家时的识别部分：CXH R9 的块（grok 失败、表里只有 codex 的行）去掉头一行与末尾两行 grok 的失败。
	down := mdxSplit(cxhGrokDownBlock)
	if down[0] != "正在识别模型: codex, grok" || down[len(down)-2] != "识别 grok 的模型失败: model discovery command failed: cxh grok down" {
		t.Fatalf("前提不成立：cxhGrokDownBlock 的头尾不是预期的样子：%q", down)
	}
	got = g.afterCandidates(g.run("/add --refresh"))
	want := append([]string{"正在识别模型: codex"}, down[1:len(down)-2]...)
	want = append(want, obzSection(obzCodexNo)...)
	mdxWantLines(t, "第 4 步", got, want)
	obzWantCodexCalls(t, e, 2)
}

// TestObzAddLoginCanceled 是 Z8：codex 的登录状态命令期间父 ctx 被取消，整份快照丢掉，只打「识别被取消」。
// not_last：codex、grok 两家，取消落在 codex 那一家，由循环头接住，grok 不再识别。
// last：只有 codex 一家，循环不再回到开头，由拍完之后那一处接住。
// 两种都不存快照：下一次 /add 重新识别，登录段照常出现。
// 钩子取消父 ctx 之后，登录状态命令的 ctx 已经结束：假执行器按次记下，钩子那一次是 true；没有钩子的
// 下一次是 false（正对照）。
// 【判别力】not_last 对 Q26（识别次数）；两个子测试都对 Q27、Q32、Q33（Q33 只有 Z8 杀得到），另有 Q6、Q7、
// Q10、Q11、Q12、Q16、Q25。
func TestObzAddLoginCanceled(t *testing.T) {
	for _, c := range []struct {
		name, tmpl, out, head string
		row                   []string
	}{
		{"not_last", cxhConfigTemplate, cxhCodexFixture(), "正在识别模型: codex, grok", obzSection(obzCodexYes, obzGrokKey)},
		{"last", cxhCodexOnlyTemplate, cxhOneGpt55, "正在识别模型: codex", obzSection(obzCodexYes)},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := cxhNewRig(t, c.tmpl, c.out)
			e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.hook = cancel
			got := g.afterCandidates(g.runCtx(ctx, "/add"))
			mdxWantLines(t, "被取消的那一次", got, []string{c.head, obzCanceledLine})
			if g.r.modelSnap != nil {
				t.Errorf("被取消的那一次不该存快照")
			}
			if n := g.exec.count(); n != 1 {
				t.Errorf("取消落在 codex 那一家，之后不该再识别别家：识别次数 %d，期望 1", n)
			}
			obzWantCodexCalls(t, e, 1)
			if cc := obzCanceledCopy(e); len(cc) != 1 || !cc[0] {
				t.Errorf("钩子取消父 ctx 之后，登录状态命令的 ctx 应当已经结束（按次记下的是 %v，期望 [true]）", cc)
			}

			e.mu.Lock()
			e.hook = nil
			e.mu.Unlock()
			again := g.afterCandidates(g.run("/add"))
			if len(again) == 0 || again[0] != c.head {
				t.Fatalf("取消之后的下一次 /add 应当重新识别，第一行是 %q，实际 %q", c.head, again)
			}
			if len(again) < len(c.row) {
				t.Fatalf("下一次 /add 的识别部分不足 %d 行：%q", len(c.row), again)
			}
			mdxWantLines(t, "下一次 /add 的登录段", again[len(again)-len(c.row):], c.row)
			// 正对照：没有钩子的那一次，登录状态命令的 ctx 没结束，记下的是 false。
			if cc := obzCanceledCopy(e); !reflect.DeepEqual(cc, []bool{true, false}) {
				t.Errorf("两次登录状态命令按次记下的 ctx 是否已结束是 %v，期望 [true false]", cc)
			}
		})
	}
}
```

### 2.3 `internal/app/obz_addlogin_app_test.go`（S3）

- **Z10 `TestObzAddLoginLineMode`**：行式 `app.Run`，显式配置写了 claude、codex、grok（claude 已登录（订阅 Claude Obx）、grok 没登录、codex 的 login status 先写标记文件再打 ChatGPT 那一句）；敲 `/new t`、`/add`、`/quit`。`out` 的结尾整段等于「EFFORTS 列是它支持的档」那一行、custom 说明行、登录段、`council> 再见`；标记文件在 Run 之前不存在、之后存在（假 codex 的 login status 那一支确实被 `/add` 调到）。
- **Z11 `TestObzAddLoginTUI`**：同 Z10 的装置走 TUI（照 A11 的 `os.Pipe` 与收尾），`/new t` 之后 `/add`，屏幕（去掉 ANSI 与空格）出现段头与 `codex已登录`，标记文件存在。
- **Z12 `TestObzFirstRunLoginTextIsRepl`**：`firstRunLoginText` 与 `repl.LoginText` 的函数指针相等；六种输入逐个相等。
- **Z13 `TestObzFirstRunGrokExitError`**（v1.1，W1 m1 的同类，首次运行那一侧）：不给配置路径、走首次运行，只装 grok。假 grok 用 `obxFake` 写（只读复用，不改 obx 文件）：`@type` 一份夹具（没登录那一句加一个模型）、往 stderr 打一行 `obz-grok-exit`、`@exit /b 1`。`errOut` 整段相等：grok 那一行是「识别失败：model discovery command failed: exit code 1: obz-grok-exit；没登录」，之后列出 `grok login`。识别失败那一段证明假 grok 确实以非 0 退出（前提），「没登录」而不是「登录状态查不出」证明 stdout 被用上了。
  - 为什么要自己打那一行 stderr（原型实测）：在 app 用例的隔离 `PATH` 下，这个假 CLI 只 `@type` 再 `@exit /b 1` 时，错误是 `exit code 1: operable program or batch file.`，也就是 cmd 在批处理的正文之前往 stderr 打了一行找不到命令的报错；同样的批处理在 turn 包的用例里（不改 `PATH`）是 `exit code 1`。来源没有查。不自己打最后一行，识别失败的说法就随机器而变。
  - 前提核过：`turn.RunDiscovery` 在退出码非 0 时把 stdout 连同错误一起交回（`discovery_exec.go:114-124`，原型探针实测：out 是夹具全文、err 是 `exit code 1`），`provider.DiscoverModels` 调的就是截 stdout 的那层闭包：闭包先截下 out，`DiscoverModels` 拿到返回值之后才判 err（`discovery_parse.go:33-39`）。

Z10 的登录段（`obzSection`）与 §3 逐字相同：

```text
各家的登录状态（本会话的快照，/add --refresh 重新检查）:
  claude  已登录（订阅：Claude Obx）
  codex   已登录
  grok    没登录；在终端里运行 grok login，登录之后 /add --refresh
```

```file internal/app/obz_addlogin_app_test.go
package app

// obz_addlogin_app_test.go 是 OB3（00_DECISIONS.md #48：/add 的识别结果之后显示各家的登录状态）在 app 包的验收：
// Z10 行式分支、Z11 TUI 分支都装上了登录状态执行器（经真 app.Run 走一次 /add，假 codex 的 login status
// 那一支被调到、登录段出现）；Z12 首次运行的说法就是 repl.LoginText 那一个函数；Z13 首次运行里识别报错但
// stdout 有内容时照样判登录状态。
// 装置与夹具只读复用 obx_firstrun_app_test.go 的 §2.0（obxIsolate、obxRun、obxDrain、obxFake、obxTypeFake、obxCodexFake、
// obxFixture、obxIn、obxEnv、obxNotExist、obxSub、obxClaudeOut、obxGrokOut、obxCodexOut、obxCodexLoggedIn、obxHead、
// obxLoginHead、obxDone）与 cox_app_test.go 的 coxLockedBuf、coxWaitFor、coxFlat、coxESC；纪律同契约 19d §2 开头。
// Z10、Z11 用显式配置，不走首次运行；Z13 不给配置路径，走首次运行。

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/provider"
	"council/internal/repl"
)

// obzSection 是 Z10 的配置（claude、codex、grok）下 /add 打出的登录段（契约 §3）。
const obzSection = "各家的登录状态（本会话的快照，/add --refresh 重新检查）:\n" +
	"  claude  已登录（订阅：Claude Obx）\n" +
	"  codex   已登录\n" +
	"  grok    没登录；在终端里运行 grok login，登录之后 /add --refresh\n"

// obzCustomNote 是 obxCodexOut 里那一条 list 的 custom/ 记录带出的说明行（既有字面量）。
const obzCustomNote = "codex 目录里还有 1 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员\n"

// obzSetup 装 claude（已登录，订阅 Claude Obx）、grok（没登录）、codex（debug models 照常；login status 先写
// 标记文件 <fx>\status-ran 再打 Logged in using ChatGPT），写一份只有这三家 provider 的显式配置。
// 返回配置路径与标记文件路径。
func obzSetup(t *testing.T, e *obxEnv) (string, string) {
	t.Helper()
	obxTypeFake(t, e, "claude", obxClaudeOut)
	obxTypeFake(t, e, "grok", obxGrokOut)
	marker := filepath.Join(e.fx, "status-ran")
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, append([]string{`@echo x> "` + marker + `"`}, obxCodexLoggedIn...))
	explicit := filepath.Join(e.root, "explicit.toml")
	body := "data_dir = '" + filepath.Join(e.root, "data") + "'\n"
	for _, id := range []string{"claude", "codex", "grok"} {
		body += "\n" + config.ProviderBlockTOML(id, filepath.Join(e.bin, id+".cmd"))
	}
	if err := os.WriteFile(explicit, []byte(body), 0o644); err != nil {
		t.Fatalf("前提不成立：写显式配置失败：%v", err)
	}
	return explicit, marker
}

// TestObzAddLoginLineMode 是 Z10：行式分支。/add 的识别部分之后是登录段，codex 那一行来自假 codex 的
// login status（标记文件证明那一支被调到）；三家都在配置里、agy 没装，登录段之后没有提示。
// 标记在 Run 之前不存在（前提）。
// 【判别力】Q29（行式分支不装执行器：没有登录段、没有标记），另有 Q22、Q23。
func TestObzAddLoginLineMode(t *testing.T) {
	e := obxIsolate(t)
	explicit, marker := obzSetup(t, e)
	obxNotExist(t, marker)

	out, errOut, err := obxRun(t, context.Background(), Options{ConfigPath: explicit, In: obxIn("/new t\n/add\n/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s", err, errOut)
	}
	if errOut != "" {
		t.Errorf("显式路径不走首次运行，errOut 应为空，实际 %q", errOut)
	}
	wantTail := "EFFORTS 列是它支持的档\n" + obzCustomNote + obzSection + "council> 再见\n"
	if !strings.HasSuffix(out, wantTail) {
		t.Errorf("out 结尾不等\n实际 %q\n期望结尾 %q", out, wantTail)
	}
	if _, serr := os.Stat(marker); serr != nil {
		t.Errorf("假 codex 的 login status 应当被 /add 调到（标记文件 %s）：%v", marker, serr)
	}
}

// TestObzAddLoginTUI 是 Z11：TUI 分支。同 Z10 的装置，经 os.Pipe 走真 tui.Run，/new 之后 /add，
// 屏幕上（去掉 ANSI 与空格）出现登录段的段头与 codex 那一行，标记文件证明 login status 被调到。
// 【判别力】Q28（TUI 分支不装执行器）。
func TestObzAddLoginTUI(t *testing.T) {
	e := obxIsolate(t)
	explicit, marker := obzSetup(t, e)
	obxNotExist(t, marker)

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提不成立：os.Pipe() 造输入管道失败：%v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = inW.Close()
		_ = inR.Close()
		t.Fatalf("前提不成立：os.Pipe() 造输出管道失败：%v", err)
	}
	buf := &coxLockedBuf{}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(buf, outR)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = Run(ctx, Options{TUI: true, ConfigPath: explicit, InFile: inR, OutFile: outW, Err: outW})
	}()
	t.Cleanup(func() {
		obxDrain(cancel, finished, t.Name())
		_ = inW.Close()
		_ = inR.Close()
		_ = outW.Close()
		select {
		case <-readDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：关掉 outW 之后 5 秒读 goroutine 仍没有结束")
		}
		_ = outR.Close()
	})

	flatHas := func(d time.Duration, want string) bool {
		return coxWaitFor(buf, d, func(raw string) bool { return strings.Contains(coxFlat(raw), want) })
	}
	write := func(what, s string) {
		t.Helper()
		if _, err := inW.Write([]byte(s)); err != nil {
			t.Fatalf("前提不成立：往输入管道写 %s 失败：%v", what, err)
		}
	}

	if !coxWaitFor(buf, 10*time.Second, func(raw string) bool { return strings.Contains(raw, coxESC+"[J") }) {
		t.Fatalf("前提不成立：10 秒内 stdout 管道里没有出现 ESC[J；内容 %q", buf.String())
	}
	write("窗口尺寸报告", coxESC+"[8;24;100t")
	if !flatHas(5*time.Second, "council空闲") {
		t.Fatalf("前提不成立：5 秒内没有看到空闲表头；flat %q", coxFlat(buf.String()))
	}
	write("/new", "/new t\r")
	if !flatHas(10*time.Second, "已建会话") {
		t.Fatalf("前提不成立：10 秒内没有看到「已建会话」；flat %q", coxFlat(buf.String()))
	}
	write("/add", "/add\r")
	head := strings.ReplaceAll("各家的登录状态（本会话的快照，/add --refresh 重新检查）:", " ", "")
	if !flatHas(10*time.Second, head) {
		t.Errorf("/add 之后 10 秒内没有看到登录段的段头 %q；flat %q", head, coxFlat(buf.String()))
	}
	if !flatHas(5*time.Second, "codex已登录") {
		t.Errorf("/add 之后没有看到登录段里 codex 那一行；flat %q", coxFlat(buf.String()))
	}
	if _, serr := os.Stat(marker); serr != nil {
		t.Errorf("假 codex 的 login status 应当被 /add 调到（标记文件 %s）：%v", marker, serr)
	}
}

// obzGrokExitOut 是 Z13 假 grok 打的输出：没登录那一句加一个模型。假 grok 打完它，往 stderr 打一行 obz-grok-exit，
// 再以退出码 1 结束。stderr 最后那一行由假 grok 自己打：隔离的 PATH 下 cmd 在跑批处理之前会往 stderr 打一行
// 找不到命令的报错（原型实测，没有查来源），不自己打一行，识别失败的说法就随机器而变。
const obzGrokExitOut = "You are not authenticated.\n\nAvailable models:\n  * grok-obz (default)\n"

// TestObzFirstRunGrokExitError 是 Z13：首次运行里识别报错（退出码非 0）但 stdout 有内容时，登录状态照样从
// stdout 判（与 /add 的 Z6 grok_exit_error 同一个判据）。只装 grok，它打出没登录那一句与模型列表、以 1 退出：
// 结果行是识别失败（exit code 1）而且没登录，之后列出 grok 的登录办法。识别失败那一段证明假 grok 确实以非 0
// 退出（前提）；「没登录」而不是「登录状态查不出」证明 stdout 被用上了。
// 【判别力】Q36（firstRunCheck 只在识别无错时才截 stdout，只有它杀得到），另有 Q23、Q30。
func TestObzFirstRunGrokExitError(t *testing.T) {
	e := obxIsolate(t)
	gfx := obxFixture(t, e, "grok.out", obzGrokExitOut)
	obxFake(t, e.bin, "grok", `@type "`+gfx+`"`, "@echo obz-grok-exit 1>&2", "@exit /b 1")

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 grok 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"+
		"  claude  没找到\n"+
		"  grok    <bin>\\grok.cmd  识别失败：model discovery command failed: exit code 1: obz-grok-exit；没登录\n"+
		"  agy     没找到\n  codex   没找到\n"+
		obxLoginHead+"  grok    grok login\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
}

// TestObzFirstRunLoginTextIsRepl 是 Z12：首次运行的说法就是 repl.LoginText 那一个函数（四档说法只有一份），
// 六种输入逐个相等是它的行为对照。
// 【判别力】Q31（首次运行另留一份说法）。
func TestObzFirstRunLoginTextIsRepl(t *testing.T) {
	if reflect.ValueOf(firstRunLoginText).Pointer() != reflect.ValueOf(repl.LoginText).Pointer() {
		t.Errorf("firstRunLoginText 应当就是 repl.LoginText，不是另一份")
	}
	for _, in := range []provider.LoginInfo{
		{State: provider.LoginYes, Subscription: "Claude Obz"},
		{State: provider.LoginYes},
		{State: provider.LoginAPIKey, KeySource: "apiKeyHelper"},
		{State: provider.LoginAPIKey},
		{State: provider.LoginNo},
		{},
	} {
		if got, want := firstRunLoginText(in), repl.LoginText(in); got != want {
			t.Errorf("firstRunLoginText(%+v) = %q，repl.LoginText 是 %q", in, got, want)
		}
	}
}
```

---

## 3. 冻结的字面量（新增的用户可见文本）

登录段的段头（`msgLoginHead`）与两种行（`printLoginStatus`），Go 的格式串逐字：

```text
各家的登录状态（本会话的快照，/add --refresh 重新检查）:\n
  %-6s  %s\n
  %-6s  %s；在终端里运行 %s，登录之后 /add --refresh\n
```

- 段头与识别表的段头「识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:」同一个款式：全角括号与逗号、半角冒号结尾。
- 第二行用于已登录、已登录（API key）、查不出；第三行只用于没登录。两种行里第一个 `%s` 是 provider id（左对齐占 6 格，与首次运行的结果行、登录办法行同一种对齐），第二个 `%s` 是 `LoginText` 的说法（四档六种，与首次运行逐字相同：`已登录（订阅：<原值>）`、`已登录`、`已登录（API key：<原值>）`、`已登录（API key）`、`没登录`、`登录状态查不出`），第三行的第三个 `%s` 是 `LoginCommand` 的登录办法（`claude auth login`、`grok login`、`codex login`；agy 的 `agy（按提示登录）` 今天不会出现在这一行）。「；」「，」是全角。
- 原型实跑的各种行（逐字）：

```text
  agy     登录状态查不出
  claude  已登录（订阅：Claude Obz）
  codex   已登录
  codex   已登录（API key）
  codex   没登录；在终端里运行 codex login，登录之后 /add --refresh
  codex   登录状态查不出
  grok    已登录（API key）
  grok    没登录；在终端里运行 grok login，登录之后 /add --refresh
```

- 本刀没有别的新增或改动的用户可见字面量。`LoginText` 的六种说法与四条登录办法是从 app 挪到 repl 的，字节不变。

---

## 4. 集成员的变异清单

**运行纪律（每一次 go build / vet / test / 变异运行之前都照做，与 19e §4 表头同一套）**：
- 同一条 PowerShell 命令里先设环境再跑（每次工具调用都是新进程，分两次调用等于没设）：
  - `USERPROFILE`、`HOME` 指到隔离目录下的 canary home（起草时是 `E:\Isolating_test\council\ob3_draft\canaryhome`）；`APPDATA`、`LOCALAPPDATA` 指到它下面的 `AppData\Roaming`、`AppData\Local`（先建好）；
  - `COUNCIL_DATA_DIR` 指到隔离目录下的 canary 数据目录；`TMP`、`TEMP` 指到隔离目录下的 `tmp`；
  - 用 `[Environment]::SetEnvironmentVariable('<名>', $null, 'Process')` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`（不要用 `Remove-Item Env:`，会被路径保护整条拦下）；
  - `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE` 指到隔离目录下、`GOTOOLCHAIN=local` 显式给；
  - `PATH` 只留 `C:\Program Files\Go\bin` 与 `C:\Windows\System32`，去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`。
  - 起草人把这些写成一个纯 ASCII 的 `goenv.ps1`，每条命令开头 dot-source 它，再跑 go 或变异驱动（node 脚本由同一个 PowerShell 起，环境随之继承；node 用全路径 `C:\Program Files\nodejs\node.exe`，它不在上面的 `PATH` 里）。集成员自己写的 `.ps1` 若含中文，要存成带 BOM 的 UTF-8。
- 每次 go test（全量、每条变异）之后核四项，任何一项出现就停下、报告实际路径，不删隔离目录以外的任何东西：
  1. canary 数据目录下没有 `council.db`；
  2. 隔离 `TMP`、真 `%TEMP%`（`C:\Users\25142\AppData\Local\Temp`）、`E:\Go\tmp` 下没有 `TestOb*` 目录（`TestObz*` 也在这个模式里）；
  3. `E:\council-data` 不存在；
  4. canary home 下没有 `.council`。
  - 整批变异跑完之后另核一次：没有开跑之后新起、还活着的 `cmd`、`grok`、`codex`、`claude`、`agy` 进程。
- 起草人照这套纪律跑完全部变异与全量：每条之后四项核对都干净；整批之后没有残留进程。

每条在原型上单独施加、从字节副本还原（不用 `git checkout`），还原后 SHA256 核对；「替换前」在文件里恰好出现一次（驱动脚本数过）。包与 `-run`：provider 的变异跑 `./internal/provider ./internal/app`、`-run 'TestOb[xyz]'`；repl 的跑 `./internal/repl`（Q20-Q24 另加 `./internal/app`）、`-run 'TestOb[xyz]|TestMdx|TestCxh'`；app 的跑 `./internal/app`、`-run 'TestOb[xyz]'`。一律 `go test -json -count=1`，按子测试的 fail 事件点名，「杀手」列是原型上这一次实测变红的全部叶子用例（父测试自己的断言红时写父测试名）。耗时是整条变异（编译加测试）的墙钟。替换前后按原文逐字给出（行首是 tab）；「替换后」为空的，是把替换前那几行整段删掉。

v1.1：36 条（Q1-Q31 与新加的 Q32-Q36）在 v1.1 的原型上全部重跑一遍，驱动脚本直接从本文 §4 取「替换前」「替换后」，每条的锚点恰好一次、还原后 SHA256 一致、四项核对干净。下面每条的耗时与杀手清单都是这一次的实测：新加的子测试让 Q1-Q31 里有几条多了杀手（例如 Q4、Q12、Q15、Q16、Q17、Q23 多了 `grok_exit_error`，Q6、Q7、Q10、Q11、Q12、Q16、Q25 多了 `shared_deadline`，Q23、Q30 多了 Z13），没有哪一条少了 v1 的杀手。

**Q1** `internal/provider/login.go`：grok 的 CPA 那一句从「前缀加后缀」放宽成只看后缀加长度（19e 头第一条）。整行替换；原型实测 6.2 s，还原后 SHA256 一致。

替换前：

```go
	case first == grokXAIAPIKey, first == grokDeploymentKey, wrapped(first, grokModelKeyPrefix, grokModelKeySuffix):
```

替换后：

```go
	case first == grokXAIAPIKey, first == grokDeploymentKey, len(first) > len(grokModelKeyPrefix)+len(grokModelKeySuffix) && strings.HasSuffix(first, grokModelKeySuffix):
```

杀手（原型实测，1 个叶子用例）：`TestObzGrokPrefixedLines/grok_model_key_prefixed`

**Q2** `internal/provider/login.go`：You are using XAI_API_KEY. 从整句相等放宽成 HasSuffix（19e 头第一条）。整行替换；原型实测 6.2 s，还原后 SHA256 一致。

替换前：

```go
	case first == grokXAIAPIKey, first == grokDeploymentKey, wrapped(first, grokModelKeyPrefix, grokModelKeySuffix):
```

替换后：

```go
	case strings.HasSuffix(first, grokXAIAPIKey), first == grokDeploymentKey, wrapped(first, grokModelKeyPrefix, grokModelKeySuffix):
```

杀手（原型实测，1 个叶子用例）：`TestObzGrokPrefixedLines/grok_xai_api_key_prefixed`

**Q3** `internal/provider/login.go`：You are not authenticated. 从整句相等放宽成 HasSuffix（19e 头第二条）。整行替换；原型实测 6.2 s，还原后 SHA256 一致。

替换前：

```go
	case first == grokNotAuthenticated:
```

替换后：

```go
	case strings.HasSuffix(first, grokNotAuthenticated):
```

杀手（原型实测，1 个叶子用例）：`TestObzGrokPrefixedLines/grok_not_authenticated_prefixed`

**Q4** `internal/repl/addmodel.go`：闭包不截 stdout（claude、grok 的登录状态从识别输出判不出来）。整行替换；原型实测 4.1 s，还原后 SHA256 一致。

替换前：

```go
			stdout = out
```

替换后：

```go
			_ = out
```

杀手（原型实测，10 个叶子用例）：`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginDiscoveryFailed/grok_empty`、`TestObzAddLoginDiscoveryFailed/grok_exit_error`、`TestObzAddLoginSection`、`TestObzAddLoginSnapshot`

**Q5** `internal/repl/addmodel.go`：没装执行器也判登录状态（零值 nil 不再表示不检测）。整行替换；原型实测 4.7 s，还原后 SHA256 一致。

替换前：

```go
		if r.loginStatus != nil {
```

替换后：

```go
		if true {
```

杀手（原型实测，24 个叶子用例）：`TestCxhAddCodexEffortRefusals`、`TestCxhAddCodexFailures/empty`、`TestCxhAddCodexFailures/unparseable`、`TestCxhAddCodexWritesConfig`、`TestCxhAddHintWhenCodexHidden`、`TestCxhAddListNoCustom/none`、`TestCxhAddListNoCustom/one`、`TestCxhAddListsCodex`、`TestCxhAddNoteBeforeFailures`、`TestCxhAddNoteWhenAllConfigured`、`TestMdxAddAliasControl`、`TestMdxAddCancelNotCached/first`、`TestMdxAddCancelNotCached/late`、`TestMdxAddDiscoveryFailures/all_down`、`TestMdxAddDiscoveryFailures/each_kind`、`TestMdxAddDiscoveryFailures/one_down`、`TestMdxAddDiscoveryFailures/unparseable`、`TestMdxAddListsDiscovered`、`TestMdxAddPlusWritesConfigAndJoins`、`TestMdxAddSkipsUndiscoverableProviders/a`、`TestMdxAddSkipsUndiscoverableProviders/c`、`TestMdxAddSnapshotAcrossReload/deleted`、`TestMdxAddSnapshotStable`、`TestObzAddLoginSection`

**Q6** `internal/repl/addmodel.go`：登录状态命令用父 ctx，不与识别共用这一家的超时。行内片段替换；原型实测 7.0 s，还原后 SHA256 一致。

替换前：

```go
info: snapshotLogin(cctx, r, pid, stdout)})
```

替换后：

```go
info: snapshotLogin(ctx, r, pid, stdout)})
```

杀手（原型实测，12 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginDiscoveryFailed/codex_failed`、`TestObzAddLoginSharedTimeout/discovery_timeout`、`TestObzAddLoginSharedTimeout/hang`、`TestObzAddLoginSharedTimeout/shared_deadline`、`TestObzAddLoginSnapshot`

**Q7** `internal/repl/addmodel.go`：cancel() 挪到判登录状态之前（登录状态命令拿到的是已取消的 cctx）。整行替换；原型实测 4.7 s，还原后 SHA256 一致。

替换前：

```go
		if r.loginStatus != nil {
			snap.logins = append(snap.logins, snapLogin{provider: pid, info: snapshotLogin(cctx, r, pid, stdout)})
		}
		cancel()
```

替换后：

```go
		cancel()
		if r.loginStatus != nil {
			snap.logins = append(snap.logins, snapLogin{provider: pid, info: snapshotLogin(cctx, r, pid, stdout)})
		}
```

杀手（原型实测，11 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginDiscoveryFailed/codex_failed`、`TestObzAddLoginSharedTimeout/hang`、`TestObzAddLoginSharedTimeout/shared_deadline`、`TestObzAddLoginSnapshot`

**Q8** `internal/repl/addlogin.go`：这一家的 ctx 已到期也跑登录状态命令。行内片段替换；原型实测 5.1 s，还原后 SHA256 一致。

替换前：

```go
ok && ctx.Err() == nil {
```

替换后：

```go
ok {
```

杀手（原型实测，1 个叶子用例）：`TestObzAddLoginSharedTimeout/discovery_timeout`

**Q9** `internal/repl/addlogin.go`：执行器报错时仍按它交回的输出判。行内片段替换；原型实测 6.0 s，还原后 SHA256 一致。

替换前：

```go
if code, so, se, err := r.loginStatus(ctx, sc); err == nil {
```

替换后：

```go
if code, so, se, err := r.loginStatus(ctx, sc); err == nil || true {
```

杀手（原型实测，1 个叶子用例）：`TestObzAddLoginCodex/exec_error`

**Q10** `internal/repl/addlogin.go`：不用登录状态命令的结果。整行替换；原型实测 4.1 s，还原后 SHA256 一致。

替换前：

```go
			info = provider.ParseLoginStatus(pid, code, so, se)
```

替换后：

```go
			_ = provider.ParseLoginStatus(pid, code, so, se)
```

杀手（原型实测，8 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginDiscoveryFailed/codex_failed`、`TestObzAddLoginSharedTimeout/shared_deadline`、`TestObzAddLoginSnapshot`

**Q11** `internal/repl/addlogin.go`：登录状态命令的路径不用配置里的 command。行内片段替换；原型实测 4.2 s，还原后 SHA256 一致。

替换前：

```go
provider.LoginStatusCommandFor(pid, r.cfg.Providers[pid].Command)
```

替换后：

```go
provider.LoginStatusCommandFor(pid, pid)
```

杀手（原型实测，11 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginDiscoveryFailed/codex_failed`、`TestObzAddLoginSharedTimeout/hang`、`TestObzAddLoginSharedTimeout/shared_deadline`、`TestObzAddLoginSnapshot`

**Q12** `internal/repl/addlogin.go`：SetLoginStatusExec 不赋值。整行替换；原型实测 4.0 s，还原后 SHA256 一致。

替换前：

```go
	r.loginStatus = f
```

替换后：

```go
	_ = f
```

杀手（原型实测，16 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginDiscoveryFailed/codex_failed`、`TestObzAddLoginDiscoveryFailed/grok_empty`、`TestObzAddLoginDiscoveryFailed/grok_exit_error`、`TestObzAddLoginSection`、`TestObzAddLoginSharedTimeout/discovery_timeout`、`TestObzAddLoginSharedTimeout/hang`、`TestObzAddLoginSharedTimeout/shared_deadline`、`TestObzAddLoginSnapshot`、`TestObzSetLoginStatusExecZeroValue`

**Q13** `internal/repl/addlogin.go`：快照里没有登录状态时也打段头。整行替换；原型实测 5.7 s，还原后 SHA256 一致。

替换前：

```go
	if len(snap.logins) == 0 {
		return nil
	}
```

替换后：整段删掉（替换成空串）。

杀手（原型实测，23 个叶子用例）：`TestCxhAddCodexFailures/empty`、`TestCxhAddCodexFailures/unparseable`、`TestCxhAddCodexWritesConfig`、`TestCxhAddHintWhenCodexHidden`、`TestCxhAddListNoCustom/none`、`TestCxhAddListNoCustom/one`、`TestCxhAddListsCodex`、`TestCxhAddNoteBeforeFailures`、`TestCxhAddNoteWhenAllConfigured`、`TestMdxAddAliasControl`、`TestMdxAddCancelNotCached/first`、`TestMdxAddCancelNotCached/late`、`TestMdxAddDiscoveryFailures/all_down`、`TestMdxAddDiscoveryFailures/each_kind`、`TestMdxAddDiscoveryFailures/one_down`、`TestMdxAddDiscoveryFailures/unparseable`、`TestMdxAddListsDiscovered`、`TestMdxAddPlusWritesConfigAndJoins`、`TestMdxAddSkipsUndiscoverableProviders/a`、`TestMdxAddSkipsUndiscoverableProviders/c`、`TestMdxAddSnapshotAcrossReload/deleted`、`TestMdxAddSnapshotStable`、`TestObzAddLoginSection`

**Q14** `internal/repl/addlogin.go`：不是已登录的都带登录办法（查不出、已登录（API key）也带）。行内片段替换；原型实测 4.2 s，还原后 SHA256 一致。

替换前：

```go
if l.info.State == provider.LoginNo {
```

替换后：

```go
if l.info.State != provider.LoginYes {
```

杀手（原型实测，10 个叶子用例）：`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginSection`、`TestObzAddLoginSharedTimeout/discovery_timeout`、`TestObzAddLoginSharedTimeout/hang`、`TestObzAddLoginSnapshot`

**Q15** `internal/repl/addlogin.go`：没登录的也不带登录办法。行内片段替换；原型实测 4.1 s，还原后 SHA256 一致。

替换前：

```go
if l.info.State == provider.LoginNo {
```

替换后：

```go
if false {
```

杀手（原型实测，5 个叶子用例）：`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginDiscoveryFailed/grok_empty`、`TestObzAddLoginDiscoveryFailed/grok_exit_error`、`TestObzAddLoginSection`、`TestObzAddLoginSnapshot`

**Q16** `internal/repl/addmodel.go`：登录段打在识别表之前。整行替换；原型实测 4.1 s，还原后 SHA256 一致。

替换前：

```go
	if err := printModelSnapshot(r, r.modelSnap); err != nil {
		return err
	}
	return printLoginStatus(r, r.modelSnap)
```

替换后：

```go
	if err := printLoginStatus(r, r.modelSnap); err != nil {
		return err
	}
	return printModelSnapshot(r, r.modelSnap)
```

杀手（原型实测，15 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginDiscoveryFailed/codex_failed`、`TestObzAddLoginDiscoveryFailed/grok_empty`、`TestObzAddLoginDiscoveryFailed/grok_exit_error`、`TestObzAddLoginSection`、`TestObzAddLoginSharedTimeout/discovery_timeout`、`TestObzAddLoginSharedTimeout/hang`、`TestObzAddLoginSharedTimeout/shared_deadline`、`TestObzAddLoginSnapshot`

**Q17** `internal/repl/addmodel.go`：识别失败的那一家不判登录状态、登录段里没有它。整行替换；原型实测 4.1 s，还原后 SHA256 一致。

替换前：

```go
		if r.loginStatus != nil {
```

替换后：

```go
		if r.loginStatus != nil && err == nil {
```

杀手（原型实测，4 个叶子用例）：`TestObzAddLoginDiscoveryFailed/codex_failed`、`TestObzAddLoginDiscoveryFailed/grok_empty`、`TestObzAddLoginDiscoveryFailed/grok_exit_error`、`TestObzAddLoginSharedTimeout/discovery_timeout`

**Q18** `internal/repl/addlogin.go`：登录段按当前 r.cfg 滤掉配置里已经没有的 provider。整行替换；原型实测 5.6 s，还原后 SHA256 一致。

替换前：

```go
	for _, l := range snap.logins {
```

替换后：

```go
	for _, l := range snap.logins {
		if _, ok := r.cfg.Providers[l.provider]; !ok {
			continue
		}
```

杀手（原型实测，1 个叶子用例）：`TestObzAddLoginSnapshot`

**Q19** `internal/repl/configcmd.go`：/config reload --apply 作废识别快照（连同登录状态）。整行替换；原型实测 4.1 s，还原后 SHA256 一致。

替换前：

```go
		r.cfg = newCfg
```

替换后：

```go
		r.cfg = newCfg
		r.modelSnap = nil
```

杀手（原型实测，4 个叶子用例）：`TestMdxAddRestoresOnInvalidConfig`、`TestMdxAddSnapshotAcrossReload/alias`、`TestMdxAddSnapshotAcrossReload/deleted`、`TestObzAddLoginSnapshot`

**Q20** `internal/repl/addlogin.go`：LoginText 的已登录（API key）落到查不出。整行替换；原型实测 6.7 s，还原后 SHA256 一致。

替换前：

```go
	case provider.LoginAPIKey:
```

替换后：

```go
	case -1:
```

杀手（原型实测，14 个叶子用例）：`TestObxFirstRunLoginStates`、`TestObyFirstRunAPIKey`、`TestObyFirstRunAPIKeyAllIn`、`TestObyFirstRunLoginText/key`、`TestObyFirstRunLoginText/key_source`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginSnapshot`、`TestObzLoginText/key`、`TestObzLoginText/key_source`

**Q21** `internal/repl/addlogin.go`：LoginText 的已登录（API key）不带来源。行内片段替换；原型实测 6.7 s，还原后 SHA256 一致。

替换前：

```go
return "已登录（API key：" + l.KeySource + "）"
```

替换后：

```go
return "已登录（API key）"
```

杀手（原型实测，4 个叶子用例）：`TestObyFirstRunAPIKey`、`TestObyFirstRunAPIKeyAllIn`、`TestObyFirstRunLoginText/key_source`、`TestObzLoginText/key_source`

**Q22** `internal/repl/addlogin.go`：LoginText 的已登录不带订阅。行内片段替换；原型实测 6.7 s，还原后 SHA256 一致。

替换前：

```go
return "已登录（订阅：" + l.Subscription + "）"
```

替换后：

```go
return "已登录"
```

杀手（原型实测，13 个叶子用例）：`TestObxAddHintLineMode/configured`、`TestObxAddHintLineMode/hint`、`TestObxFirstRunCheckingFirst`、`TestObxFirstRunEndToEnd`、`TestObxFirstRunNoAuto`、`TestObxFirstRunParallel`、`TestObxFirstRunRace`、`TestObxFirstRunTUI`、`TestObxFirstRunWriteFails`、`TestObyFirstRunLoginText/yes_sub`、`TestObzAddLoginLineMode`、`TestObzAddLoginSection`、`TestObzLoginText/yes_sub`

**Q23** `internal/repl/addlogin.go`：LoginText 的没登录落到查不出。整行替换；原型实测 6.7 s，还原后 SHA256 一致。

替换前：

```go
	case provider.LoginNo:
```

替换后：

```go
	case -1:
```

杀手（原型实测，16 个叶子用例）：`TestObxAddHintLineMode/configured`、`TestObxFirstRunEndToEnd`、`TestObxFirstRunLoginStates`、`TestObxFirstRunNoAuto`、`TestObxFirstRunParallel`、`TestObxFirstRunTUI`、`TestObxFirstRunWriteFails`、`TestObyFirstRunLoginText/no`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginDiscoveryFailed/grok_empty`、`TestObzAddLoginDiscoveryFailed/grok_exit_error`、`TestObzAddLoginLineMode`、`TestObzAddLoginSection`、`TestObzAddLoginSnapshot`、`TestObzFirstRunGrokExitError`、`TestObzLoginText/no`

**Q24** `internal/repl/addlogin.go`：codex 的登录办法改了一个字（首次运行与 /add 同一份）。行内片段替换；原型实测 6.7 s，还原后 SHA256 一致。

替换前：

```go
"codex":  "codex login",
```

替换后：

```go
"codex":  "codex auth login",
```

杀手（原型实测，6 个叶子用例）：`TestObxFirstRunFailures`、`TestObxFirstRunParallel`、`TestObyFirstRunCodexStatusTimeout`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginSnapshot`、`TestObzLoginText`

**Q25** `internal/repl/addlogin.go`：登录段的行不与首次运行同一种对齐（id 占 7 格）。行内片段替换；原型实测 3.9 s，还原后 SHA256 一致。

替换前：

```go
r.printf("  %-6s  %s\n", l.provider, LoginText(l.info))
```

替换后：

```go
r.printf("  %-7s  %s\n", l.provider, LoginText(l.info))
```

杀手（原型实测，13 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginCodex/api_key`、`TestObzAddLoginCodex/chatgpt`、`TestObzAddLoginCodex/exec_error`、`TestObzAddLoginCodex/not_logged_in`、`TestObzAddLoginCodex/unparseable`、`TestObzAddLoginDiscoveryFailed/codex_failed`、`TestObzAddLoginSection`、`TestObzAddLoginSharedTimeout/discovery_timeout`、`TestObzAddLoginSharedTimeout/hang`、`TestObzAddLoginSharedTimeout/shared_deadline`、`TestObzAddLoginSnapshot`

**Q26** `internal/repl/addmodel.go`：去掉 takeModelSnapshot 循环头的取消判断。整行替换；原型实测 4.2 s，还原后 SHA256 一致。

替换前：

```go
		if ctx.Err() != nil {
			break
		}
```

替换后：整段删掉（替换成空串）。

杀手（原型实测，2 个叶子用例）：`TestMdxAddCancelNotCached/first`、`TestObzAddLoginCanceled/not_last`

**Q27** `internal/repl/addmodel.go`：去掉 printDiscoveredModels 拍完之后的取消判断。整行替换；原型实测 4.2 s，还原后 SHA256 一致。

替换前：

```go
		if ctx.Err() != nil {
			return r.printf("识别被取消；下次 /add 会重新识别\n")
		}
```

替换后：整段删掉（替换成空串）。

杀手（原型实测，5 个叶子用例）：`TestMdxAddCancelNotCached/first`、`TestMdxAddCancelNotCached/late`、`TestObxAddHintCanceled`、`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`

**Q28** `internal/app/app.go`：TUI 分支不装登录状态执行器。行内片段替换；原型实测 20.6 s，还原后 SHA256 一致。

替换前：

```go
		r.SetLoginStatusExec(turn.RunLoginStatus)
		err = tui.Run(
```

替换后：

```go
		err = tui.Run(
```

杀手（原型实测，1 个叶子用例）：`TestObzAddLoginTUI`

**Q29** `internal/app/app.go`：行式分支不装登录状态执行器。整行替换；原型实测 5.6 s，还原后 SHA256 一致。

替换前：

```go
		r.SetLoginStatusExec(turn.RunLoginStatus)
		err = r.Run(ctx)
```

替换后：

```go
		err = r.Run(ctx)
```

杀手（原型实测，3 个叶子用例）：`TestObxAddHintLineMode/configured`、`TestObxAddHintLineMode/hint`、`TestObzAddLoginLineMode`

**Q30** `internal/app/firstrun.go`：首次运行的登录办法不用 repl.LoginCommand。行内片段替换；原型实测 5.6 s，还原后 SHA256 一致。

替换前：

```go
repl.LoginCommand(id))
```

替换后：

```go
id)
```

杀手（原型实测，10 个叶子用例）：`TestObxFirstRunEndToEnd`、`TestObxFirstRunFailures`、`TestObxFirstRunLoginStates`、`TestObxFirstRunNoAuto`、`TestObxFirstRunParallel`、`TestObxFirstRunTUI`、`TestObxFirstRunWriteFails`、`TestObyFirstRunAPIKey`、`TestObyFirstRunCodexStatusTimeout`、`TestObzFirstRunGrokExitError`

**Q31** `internal/app/firstrun.go`：首次运行另留一个函数（行为相同，但不是同一个函数）。整行替换；原型实测 5.6 s，还原后 SHA256 一致。

替换前：

```go
var firstRunLoginText = repl.LoginText
```

替换后：

```go
var firstRunLoginText = func(l provider.LoginInfo) string { return repl.LoginText(l) }
```

杀手（原型实测，1 个叶子用例）：`TestObzFirstRunLoginTextIsRepl`

**Q32** `internal/repl/addlogin.go`（v1.1）：登录状态命令改用从 context.Background() 派生的一段新的 discoverTimeout（审查员的写法）：既不与识别共用这一家的 deadline，也收不到父 ctx 的取消。行内片段替换；原型实测 3.0 s，还原后 SHA256 一致。

替换前：

```go
r.loginStatus(ctx, sc); err == nil {
```

替换后：

```go
func() (int, []byte, []byte, error) { c, cc := context.WithTimeout(context.Background(), r.discoverTimeout); defer cc(); return r.loginStatus(c, sc) }(); err == nil {
```

杀手（原型实测，3 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`、`TestObzAddLoginSharedTimeout/shared_deadline`

**Q33** `internal/repl/addlogin.go`（v1.1）：登录状态命令改用 context.WithDeadline(context.Background(), <这一家 ctx 的 deadline>)：deadline 与识别相同，收不到父 ctx 的取消。行内片段替换；原型实测 2.9 s，还原后 SHA256 一致。

替换前：

```go
r.loginStatus(ctx, sc); err == nil {
```

替换后：

```go
func() (int, []byte, []byte, error) { d, _ := ctx.Deadline(); c, cc := context.WithDeadline(context.Background(), d); defer cc(); return r.loginStatus(c, sc) }(); err == nil {
```

杀手（原型实测，2 个叶子用例）：`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`

**Q34** `internal/repl/addmodel.go`（v1.1）：登录状态命令改用从父 ctx 派生的一段新的 discoverTimeout：收得到取消，预算是新的；这一家的 ctx 已到期时照旧交 cctx（D9 的判断不变）。行内片段替换；原型实测 3.0 s，还原后 SHA256 一致。

替换前：

```go
info: snapshotLogin(cctx, r, pid, stdout)})
```

替换后：

```go
info: func() provider.LoginInfo { if cctx.Err() != nil { return snapshotLogin(cctx, r, pid, stdout) }; lctx, lcancel := context.WithTimeout(ctx, r.discoverTimeout); defer lcancel(); return snapshotLogin(lctx, r, pid, stdout) }()})
```

杀手（原型实测，1 个叶子用例）：`TestObzAddLoginSharedTimeout/shared_deadline`

**Q35** `internal/repl/addmodel.go`（v1.1）：`/add` 只在识别无错时才截 stdout（识别报错但有输出时，claude、grok 的登录状态判不出）。整行替换；原型实测 2.9 s，还原后 SHA256 一致。

替换前：

```go
			stdout = out
```

替换后：

```go
			if err == nil {
				stdout = out
			}
```

杀手（原型实测，1 个叶子用例）：`TestObzAddLoginDiscoveryFailed/grok_exit_error`

**Q36** `internal/app/firstrun.go`（v1.1）：首次运行的 firstRunCheck 只在识别无错时才截 stdout（与 Q35 同一条，OB1 留下的缺口）。整行替换；原型实测 4.8 s，还原后 SHA256 一致。

替换前：

```go
		stdout = out
```

替换后：

```go
		if err == nil {
			stdout = out
		}
```

杀手（原型实测，1 个叶子用例）：`TestObzFirstRunGrokExitError`


**Q32-Q34 为什么三条都要登记（v1.1）**：共用超时与收得到取消是两条独立的性质。Q32 两样都丢，Q33 只丢取消（deadline 与识别相同），Q34 只丢共用超时（收得到取消）。`shared_deadline` 杀 Q32、Q34，杀不到 Q33；Z8 的新断言杀 Q32、Q33，杀不到 Q34。两条新断言缺一条，就有一条变异活下来。

**`shared_deadline` 里那 50 ms 是必需的（v1.1 原型实测）**：把 S2 里那一行 `time.Sleep(50 * time.Millisecond)` 删掉（从字节副本还原，SHA256 一致），`-count=5` 跑 Q32、Q33、Q34：Q34 五次都活下来（0 个用例变红）；Q32 只剩 Z8 的两个子测试杀它，`shared_deadline` 五次都没红；Q33 照旧只被 Z8 杀。也就是说，不拖那 50 ms，识别与登录状态命令的两个 deadline 在本机的时钟精度下相等，`shared_deadline` 比不出来。这是时钟粒度造成的，不是确定的行为，只能说 5 次里 5 次相等。裁决书写的是「去掉 sleep 之后 Q33 活下来」，实测改成对 Q34：Q33 的 deadline 按构造就与识别的相同，拖不拖都只有 Z8 杀得到它；靠 sleep 才杀得到、别处又杀不到的是 Q34。

**判定为等价、不列杀手的变异**：本刀没有登记。只在审查时看的：A1、A2、A3、A4 里的注释行，A6 的文件头与各函数的注释，B2 的注释，T1（只改注释）。另有一条行为等价、没有用例担保的（v1.1，W1 m2）：在 `firstrun.go` 里另留一份内容相同的登录办法表，例如把 `repl.LoginCommand(id))` 换成 `map[string]string{"claude": "claude auth login", "grok": "grok login", "agy": "agy（按提示登录）", "codex": "codex login"}[id])`。它今天输出不变，任何用例都杀不到（W1 实测 `./internal/repl`、`./internal/app` 0 个变红），坏处只在以后 `loginCommands` 改了它不跟着改。只在审查时看；§6 第 1 步的 grep 核对管它。

**19d、19e 的变异在本刀下的状态**：19d 的 M40（`"claude": "claude auth login",`）、M57（订阅那一句 `return`）与 19e 的 N21、N22（`LoginAPIKey` 那一支）的锚点在 HEAD 的 `firstRunLogin` 表与 `firstRunLoginText` 函数体里；本刀之后那几行原样挪到 A6（`loginCommands`、`LoginText`），锚点文本不变、所在文件变成 `internal/repl/addlogin.go`。Q20、Q21、Q22、Q24 是它们在 A6 上的同类，原型实测的杀手里照样有首次运行的用例（A1、A8、A9、B0-B3 等，见上）；M40 本身（claude 那一条登录办法）没有在 A6 上重跑。19d 的 M39、M74、M56、M58、M59、M73 与 19e 的 N20、N23 的锚点不在 B1-B3 改动的行上，没有重跑。`login.go` 的全部变异不受影响（生产代码没改）。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许动这些文件：**块 A**：补丁 `internal/repl/repl.go`（A1）、`internal/repl/addmodel.go`（A2-A5），新文件 `internal/repl/addlogin.go`（A6）；**块 B**：补丁 `internal/app/firstrun.go`（B1-B3）、`internal/app/app.go`（B4、B5），授权的测试改动 `internal/app/oby_firstrun_app_test.go`（T1）、`internal/app/obx_firstrun_app_test.go`（T2）。不改 `go.mod`，不新写任何 `obz` 测试文件（那是测试腿的事）。
- 行数：块 A 是 A6 新文件 112 行、A1-A5 的替换文本合计 42 行；块 B 是 B1-B5 与 T1、T2 的替换文本合计 20 行；两块合计约 174 行（不算锚点）。**一次交完**，先块 A 后块 B；落地时可以只落块 A 先跑（D12）。
- 整文件用 `===FILE: <路径>===` 换行接完整内容、末尾 `===END FILE===`。补丁块：

  ```
  ===PATCH: internal/app/firstrun.go===
  <<<<ANCHOR
  （从 HEAD 35520ff 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点照 §1.1 各处的 `anchor` 代码块（本文代码块里的缩进是 tab；若你看到的是空格，以真文件为准）；同一文件的多个锚点互不重叠。**每个 `===FILE:`、`===PATCH:` 标记都单独起一行、从行首开始**（OB2 的块 A 把标记接在开场白末尾，按行首解析的落地脚本漏了文件）。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- 代码必须能 `gofmt`（tab 缩进）；`go vet` 不许有新报告。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（全角括号、冒号、分号、逗号、顿号、直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的用户可见字符串只有 §3 的段头与两种行格式。
- 生产代码新标识符不许以 `obz` 开头。§1.1 给出的签名、判据、代码、注释照抄；本刀没有「名字实现方定」处。
- 回复末尾必须有：

  ```
  ===REPORT===
  块：给了哪些文件（块 A、块 B 各自）。
  补全性：A1-A6、B1-B5、T1、T2 各落在哪个 FILE / PATCH 块。
  锚点：每个锚点是否从 HEAD 35520ff 真文件逐字复制、在该文件里恰好一次。
  字面量：§3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  ```

---

## 6. 落地后主代理要做的

1. **收割与逐字核对**：越界扫描（主仓库、`internal/*`、`cmd/*`、中转根有没有实现腿写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；标记不在行首的也要找（OB2 的教训）。逐字核对：用脚本从本文取出每一对 `anchor` / `replace` 与每个 `file` 代码块（内容加一个 LF），打在 HEAD `35520ff` 的一份 `git archive` 上，结果应当与落盘后的 10 个文件（`repl.go`、`addmodel.go`、`addlogin.go`、`firstrun.go`、`app.go`、`oby_firstrun_app_test.go`、`obx_firstrun_app_test.go`，外加测试腿的三个 `obz` 文件）逐字节相同（起草时就是这样核的：12 对锚点各恰好一次、4 个整文件）。`gofmt -l ./internal ./cmd`；`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn`；`go build ./...`。`internal/provider`、`internal/turn`、`internal/config`、`internal/tui`、`cmd/council` 的生产代码零改动。
   - **登录办法字面量的 grep 核对（v1.1，W1 m2）**：在 `internal/app` 的非测试 `.go` 文件里搜 `claude auth login|grok login|codex login|按提示登录`。命中只许是 `msgFirstRunNoneFound` 的四行（落地后的 `firstrun.go:49-52`），外加 `firstRunCheck` 头注释里提到登录状态命令的那一行（`:70`，「codex 另跑一次 codex login status」，HEAD 原有，说的是查登录状态的命令，不是登录办法）。v1.1 原型上实测就是这五处。多出任何一处，就是首次运行又另留了一份登录办法（§4「只在审查时看」那一条），退回实现腿。
2. **合并测试腿的 3 个文件**，先跑 `go test -count=1 ./internal/provider ./internal/repl ./internal/app`，再跑全量 `go test -count=1 ./...`（原型九包全绿）；`-run 'TestOb[xyz]'` 五个包（app、repl、provider、turn、config）跑一次 `-count=3`。**每一次运行都照 §4 表头的运行纪律**，跑完核四项与残留进程。
3. **§4 的 36 条变异逐条跑**，照 §4 表头的纪律，从字节副本还原、SHA256 核对；点名杀手里的每个子测试都要单独确认变红。Q28 约 21 s（Z11 等满 10 s），其余每条 3-10 s。
4. **文档**：
   - `docs/00_DECISIONS.md` #48 之下记「OB3 已落地（commit），真机未复验」：D1-D12 的主代理裁定、§0 第 2-4 条、§7 的待定项；「待主代理」里 `/add` 显示登录状态与「三行负对照」两条划掉。
   - `docs/19e_OB2_API_KEY_LOGIN_CONTRACT.md` 文件头「落地后订正」加一条：§7 (a) 已由 OB3 做了；头里「grok 还缺三行负对照」与「一处冻结注释不准」两条已由 OB3 的 K3、T1 补上。`docs/19d_OB1_FIRST_RUN_CONTRACT.md` 头里同样记一句 §7 (o) 已由 OB3 做了。
   - `docs/README.md`：`/add` 那一节写上识别结果之后多一段「各家的登录状态」（本会话的快照、没登录的那一家带登录办法、`/add --refresh` 重新检查）；「第一次运行」那一节补一句登录状态以后在 `/add` 里也看得到。
   - 本契约归档到 `docs/`（照 19d、19e 的体例在文件头加落地后订正区）。
   - `docs/14_MVP_CHECKLIST.md` 加一项「`/add` 显示各家的登录状态」，步骤照下面第 5 步。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的真机复验步骤**（E 盘机，零花费：只多跑一次 `codex login status`，其余是 `/add` 本来就做的识别，不提问）。用你自己的真配置，不用临时目录：
   0. 先确认装的是带本刀的版本（主代理照惯例等 Griffin 说「装」再重装）：

      ```powershell
      council -version
      ```

   1. 照常开 council（TUI 或 `council -no-tui` 都行）。
   2. 开一个会话（`/add` 要有当前会话）。`/new` 会在你的真库里建一个空会话；不想多一个会话，就 `/list` 之后 `/use <已有会话的 id>`：

      ```text
      /new ob3check
      ```

   3. 敲：

      ```text
      /add
      ```

      看：候选、「正在识别模型: agy, claude, codex, grok」（以你配置里写了的为准，字典序）、识别表与后面那几行照旧；然后是新的一段，按同样的顺序每家一行。照 19d §0 与 19e §0 的实测，预期是：

      ```text
      各家的登录状态（本会话的快照，/add --refresh 重新检查）:
        agy     登录状态查不出
        claude  已登录（订阅：Claude Max）
        codex   已登录
        grok    已登录（API key）
      ```

      - claude 的「订阅：」后面是你账户的原值（19d 实测是 Claude Max）；
      - codex 是「已登录」不是「已登录（API key）」：你用的是 ChatGPT 登录（19e §0 第 4 条实测）；
      - grok 是经 CPA 的 API key（19e §0 第 2 条实测的首行）；Mac 上的 CPA 睡着时这一行会不会变没测过（19e §7 (g)）；
      - agy 永远查不出，这一行不带登录办法；
      - 个数与名字以你那时的 CLI 为准；四家里某一家是「没登录」时，那一行末尾会有「在终端里运行 <登录办法>，登录之后 /add --refresh」。
   4. 再敲一次 `/add`：不再出现「正在识别模型」，登录段逐字相同（快照）。敲 `/add --refresh`：重新识别，登录段重新检查一次。
   5. `/quit` 退出。第 2 步用 `/new` 建的空会话留在库里，不影响别的。
   - D 盘机照同样的步骤做，[未验证]。

---

## 7. open items

**主代理已裁定（v1.1，照裁决书；(a)-(d) 不再是待定项，留在这里说明来由）**

- (a) **一处既有测试的期望跟着改了**（T2，A10 `TestObxAddHintLineMode`）：A10 用真 `app.Run` 敲 `/add` 并钉住结尾整段，登录段按裁决夹在识别表与「本机装了……」提示之间，A10 的结尾必然多出这一段。这是本刀唯一改动的既有断言（另一处 T1 只改注释），新期望取自原型实跑；它顺带成了「行式分支装了执行器」的第二个杀手（Q29）。**主代理裁定：接受，授权**（登录段的位置是主代理定的，A10 用 HasSuffix 钉结尾，必须改）。
- (b) **agy 那一行每次都在**：裁决要「每家各一行」，agy 永远是「登录状态查不出」，只是不带登录办法。嫌它占地方的话，可以改成查不出的不列，但那样「查不出」与「没装执行器」在屏幕上分不开。**主代理裁定：保留**（每家一行、不带提示，正好让「查不出」与「没装执行器」在屏幕上分得开）。
- (c) **快照之后删掉的 provider 照打**（D7）：与失败行同一个处置；段头写着「本会话的快照」。要改成按当前配置滤，是 Q18 那一行，Z7 第 3 步跟着改。**主代理裁定：接受**，理由照 D7。
- (d) **`/join` 也打登录段**：它与 `/add` 是同一个 handler（HEAD 就是这样），不另处理。**主代理裁定：接受。**

**待主代理**
- (e) **每次重拍多一次 `codex login status`**（0.16 s，19d 实测）：只在本会话第一次 `/add` 与 `/add --refresh` 时跑；超时与识别共用 `discoveryTimeout`（20 s）。
- (f) **`firstRunLoginText` 是一个包级变量**（D3）：它可以被重新赋值，生产代码与测试都没有这样做；Z12 钉住它就是 `repl.LoginText`。Z12 用的 `reflect.Value.Pointer` 对函数值，Go 文档不保证不同函数的指针一定不同，所以它只在一个方向上可靠：相等时未必真是同一个函数；原型上 Q31（另写一个函数）实测变红。
- (g) **Z11 在变异下要等满 10 s**（Q28 整条约 21 s）：TUI 的屏幕只能轮询，没有「登录段一定已经打完」的同步点；未变异时 Z11 约 0.2 s。
- (h) **`LoginCommand` 对不认识的 id 返回空串**：今天能落在没登录的只有 claude、grok、codex，三家都有登录办法；以后 provider 包给新的一家加了没登录的判据，要同时在 `loginCommands` 里加它的登录办法，否则那一行会是「在终端里运行 ，登录之后 /add --refresh」。
- (i) **「已登录（API key）」不等于 key 能用**（19e §7 (g)）：`/add` 里同样只说配了 key。
- (j) **改 `loginCommands` 时要同步 `msgFirstRunNoneFound`**（v1.1，W1 m2）：一家都没找到时的安装说明（`firstrun.go` 的 `msgFirstRunNoneFound`）另写着四家的登录办法，它是首次运行的既有输出，本刀不动。`loginCommands` 以后改了，这一句不会跟着改；没有用例把两处连起来。
- (k) **A6 里 `LoginCommand` 的注释**（v1.1 看到，没改）：它写「首次运行的登录办法与 /add 登录段里没登录那一行用的都是它」，与 v1 的 B2 注释是同一种说法，没把 `msgFirstRunNoneFound` 算进去。裁决书要块 A 的生产代码一个字节不变，所以没改；按 B2 的限定读：说的是结果之后列出的登录办法。要改就是块 A 的一处注释，重跑 §6 第 1 步的逐字核对即可。
- (l) **Z13 的假 grok 自己往 stderr 打最后一行**（v1.1，原型实测，见 §2.3 Z13）：app 用例的隔离 `PATH` 下，cmd 在批处理的正文之前往 stderr 打一行找不到命令的报错，来源没查。以后在 app 包里写「以非 0 退出」的假 CLI，都要自己打最后一行 stderr，否则识别失败的说法随机器而变（A8 的假 codex 也是先往 stderr 打一行 `obx-boom` 再以 3 退出）。

**泄漏核对（起草期间，照 §4 表头）**：原型、块 A 单独落地的那一份与往返核对用的那一份上，每一次 go build / vet / test 与每一条变异都在同一条 PowerShell 命令里先 dot-source 隔离环境（`USERPROFILE`、`HOME` 指 canary home，`APPDATA`、`LOCALAPPDATA` 指它下面的 `AppData\Roaming`、`AppData\Local`；`COUNCIL_DATA_DIR`、`TMP`、`TEMP` 指隔离目录；清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`；`GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE` 指隔离目录、`GOTOOLCHAIN=local`；`PATH` 只留 Go 与 System32，去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`）。每次 go test 与每条变异之后四项核对全部干净；变异整批与最后一次全量之后，没有开跑之后新起、还活着的 `cmd`、`grok`、`codex`、`claude`、`agy`、`node` 进程（按命令行排除了宿主自己每次工具调用起的 shell 启动器 `cmd.exe /d /s /c ... CLAUDE_CODE_SHELL_LAUNCHER_SCRIPT ...`：只按名字与启动时间筛会把它们算进来）。没有打开 `C:\Users\25142\.council` 下的任何文件，没有读任何 CLI 的配置与凭据，没有读写剪贴板，主仓库一个字节没写（只用了 `git archive`）。

**泄漏核对（v1.1 修订期间，同一套纪律）**：
- 范围：v1.1 原型、只落块 A 的那一份、往返核对用的那一份，以及原型上的两个临时探针（turn 与 app 包各一个 `zzprobe_test.go`，跑完即删、往返核对时原型里没有它们）。
- 纪律：每一次 go build / vet / test 与每一条变异都在同一条 PowerShell 命令里先 dot-source 同样的隔离环境，隔离目录是 `E:\Isolating_test\council\ob3_v11\`。
- 结果：每次 go test 与每条变异之后四项核对全部干净（包括 36 条变异与 sleep 探针的 3 条）。
- 残留进程：整批之后查了开跑之后新起、还活着的 `cmd`、`node`、`go`、`*.test`、`grok`、`codex`、`claude`、`agy`、`council` 进程。没有本刀起的：按命令行排除宿主的 shell 启动器之后，列出来的只有用户自己的 Codex 桌面程序与它的 MCP 进程，命令行里都不指向隔离目录。
- 没有打开 `C:\Users\25142\.council`，没有读任何 CLI 的配置与凭据，没有读写剪贴板，没有调用任何真 provider CLI，没有问任何模型，主仓库一个字节没写。
