# CL1 冻结契约：一批小修补（v1.1，2026-10-01）

> **归档说明（2026-10-01，主代理）**：本文是 CL1 的冻结契约 v1.1，原样归档。派工时的原件是 `E:\multi_model_cowork\cl1_contract.md`，锚点与行号指 HEAD `483a766`。CL1 与本文、`00_DECISIONS.md` #50 在同一个提交里落地（`5c4840a`），2026-10-01 Griffin 说「装」后装为 `council 0.2.0-1001b`（vcs.revision `6037094`），真机未复验。
> - **契约经过**：
>   - v1 → W1：没有 major，5 条 minor，全收。
>   - v1.1 → 定向核验：只报了两处计数写错（「18 条有杀手」应为 20 条，S3 应为 322 行），主代理直接改了契约文字。本文是改过之后的版本。
> - **实现**：生产代码分两块。**两路 grok 都没交出代码**：经 CPA 的 grok 调用被上游以 HTTP 426 拒绝，报「Grok CLI version (0.2.120) is outdated」，而本机 grok 是 1.0.41；连只让它回「OK」的极小提问也一样。这是基础设施问题，与任务无关。按委派链，两块改由 Opus 实现，与 §1.1 逐字节相同（9 个文件，含授权的既有测试改动）。
> - **测试**：3 个 clx 测试文件由一条 Opus 测试腿写成，与 §2 逐字节相同。
> - **集成**：Opus 集成员没有做机械修正。§4 的 22 条变异里 20 条被点名杀手杀掉，E1、E2 等价；没有点名的叶子一个都没红。每次运行后四项泄漏核对与残留进程核对都干净。
> - **审查**：Opus 独立审查报 1 条 minor（见下），不是实现偏离。审查员另拟两条变异：M6（`cmd.Run` 改成 `cmd.Start`）被杀；M2（`pid <= 0` 改成 `pid < 0`）存活，但不承重，因为调用方传的总是 Start 之后的 pid。
>
> **落地后订正（2026-10-01，主代理；读本文之前先读这里）**：
> - **B2 注释里「（或当前目录）」那半句说错了机制**（审查）：
>   - `systemTaskkillPath` 的头注释说，PATH 前面的目录或当前目录里有同名的 taskkill，按名字起就会执行它。PATH 那一半成立（HEAD 实测）；当前目录那一半在本机的 Go 1.27 下不成立。
>   - 没设 `NoDefaultCurrentDirectoryInExePath` 时，`exec.LookPath` 找到 `.\taskkill.cmd` 但返回 ErrDot，`exec.Command` 拒绝起它；设了（本宿主就设了 =1）就根本不搜当前目录。
>   - 所以 HEAD 在这种情形下的真实后果是「taskkill 起不来、树杀不掉、只剩 Kill 兜底」，不是「执行了假的」。CL1 改用绝对路径，顺带把这个杀不掉的情形也修了。
>   - B2 是冻结块，这次不改字节；以后有人动 `killtree_windows.go` 时把那半句改正。
> - **§6 第 5 步第 0 条**说「主代理照惯例等 Griffin 说『装』」，照此办理：落地时没有装，等 Griffin 说了「装」才装的 `0.2.0-1001b`。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `483a766`（完整 `483a7665469cc1fe6a9a3808522c16aef11196b1`，= origin/main）。代码与 `d4ad85f`（AD1 落地）相同，之后只有文档提交。Go 全路径 `"C:\Program Files\Go\bin\go.exe"`；本机没有 gcc，不能 `-race`。E 盘机（Windows 11，PowerShell 5.1）。
- 本刀的 KEY 是 `CL1`；测试腿的包级标识符前缀是 `clx`（类型、函数、变量、常量一律 `clx` 开头，测试函数 `TestClx...`）。生产代码的新标识符不许以 `clx` 开头。本文的生产代码新标识符只有三个：`taskkillPath`（包级变量，Windows 与非 Windows 各一份）、`systemTaskkillPath`（只在 Windows），以及 `cmdTeamUse` 里的局部变量 `sumID`。
- 来源（逐条点名出处）：
  - Griffin 2026-10-01 原话「开始做那批小修补吧」；清单与方向是主代理定的（任务书第 1-8 条），本文只冻结细节。
  - 第 1 条（`/help` 里 `/add` 那一行）：`docs/19g_AD1_ADD_BY_NAME_CONTRACT.md` D14 与 §7 (e)；`docs/00_DECISIONS.md` #49「待定」第一条、#46 裁定（「`/help` 补上 `/add +编号` 与 `--refresh`」之后 AD1 又多了按名字加）。
  - 第 2 条（`--id` 缺值守卫的用例）：19g 文件头「落地后订正」第一条（审查 M9）。
  - 第 3 条（两处注释）：19g 文件头「落地后订正」第二条。
  - 第 4 条（`LoginCommand` 的注释）：`docs/19f_OB3_ADD_LOGIN_STATUS_CONTRACT.md` 文件头与 §7 (k)；#48「待主代理」第一条。
  - 第 5 条（`/team use` 与总结成员）：#47 的主代理裁定「总结成员（`[summary] agent`）永远不当发言者」与同刀的「`/add` 的候选里藏掉总结成员」；#47 CH1 落地记录「待主代理」第二条「`/team use` 会 Join 总结成员」（`19a` 文件头）。
  - 第 6 条（`killProcessTree` 按 PATH 找 taskkill）：`docs/19d_OB1_FIRST_RUN_CONTRACT.md` §7 (d)；#48 OB2「待主代理」第三条；`docs/19e_OB2_API_KEY_LOGIN_CONTRACT.md` E7（L5b 的写法）。
  - 第 7 条（Ctrl+Alt+C）：`docs/19c_AT1FU_FOLLOWUP_CONTRACT.md` 文件头「落地后订正」第一条；#47 AT1-FU 落地记录的审查 minor。
- 证据等级照实写：「实测」是本机跑过的；「原型实跑」是在下面的隔离拷贝里跑过的；「HEAD 实跑」是在新解出的 HEAD 拷贝里跑过的；[未验证] / [推断] 照字面。本刀没有调用任何真 provider CLI，没有向任何模型提问。

**v1.1 变更记录（2026-10-01，照主代理裁决书 `cl1_rulings.md` 修订；W1 原文 `cl1_w1_findings.md`；v1 快照 `cl1_contract_v1.md`；基线仍是 HEAD `483a766`）**：

- 做法：`git archive 483a766` 解到 `E:\Isolating_test\council\cl1_v11\proto\`（372 个文件），由脚本把 v1 的 15 对锚点与 3 个整文件逐字打上（各恰好一次），先跑全量 `go test -count=1 ./...` 九包全绿（v1 基线），再按裁决改。v1 字节另存一份作对照。
- **m1（X1）**：C5 加第 2 步，接缝指向另一个 `t.TempDir()` 里、名字不叫 taskkill、不在 PATH 上的假 `clx-fake-tk.cmd`，断言它被执行、参数 `/T /F /PID <pid>`（第 1 步原样保留，仍是 C4、C6 的正对照）；新加 C8 `TestClxKillTreeIgnoresPathTaskkillExe`：测试里用 `go build` 现编一个假 `taskkill.exe`（GOCACHE、GOTMPDIR、输出目录都在本用例的 `t.TempDir()` 里），排在 PATH 最前，断言 `killProcessTree` 不执行它、循环被杀，同一装置接缝指向它时被执行（正对照）。编译稳定、耗时可接受（空缓存单独编 2.9 s 与 3.6 s，C8 整条 4.07-4.59 s），所以没有退到「只在审查时看」。§4 登记 X1，杀手 C5、C8，实测变红。与裁决的字面差一点：裁决说 C5「改指向」，本文保留第 1 步、另加第 2 步，因为 C4、C6 的「假的没被执行」要配「同一个假文件被执行时标记出现」的正对照，第 1 步就是它。C5 的【判别力】注释只点了 V11、X1；§4 实测 V8 也杀 C5（V8 下第 2 步起的是 PATH 上的假 `taskkill.cmd`，标记写在第 1 步的目录里，第 2 步的标记文件不出现），注释没列这一条，不算说错。
- **m2（X2）**：C7 末步记下结果、`t.Setenv("SystemRoot", `Z:\clx-nope`)`、再调一次，断言不变。§4 登记 X2，杀手 C7，实测变红（失败消息里结果变成 `Z:\clx-nope\System32\taskkill.exe`）。
- **m3（X4）**：C3 第 2 步改成三种键的小表（Ctrl 加 Alt、NumLock、CapsLock，各起新 Model，第 3 步对照照留），测试函数改名 `TestClxCtrlCModsClosesList`、子测试 `alt`、`numlock`、`capslock`。§4 登记 X4，杀手 `numlock`、`capslock` 两个子测试，实测变红（`alt` 照绿，这正是 X4 的形状）。「同类的还有没有别处」：列表分支的 Esc、↑、↓、Tab、回车只比 `msg.Code`，NumLock 开着时照常，一次性探针实跑确认，没有缺口，记 §7 (m)。
- **m4**：D6 收窄成「路径不随 SystemRoot 变」；§7 加 (l)（SystemRoot 坏掉时 taskkill.exe 自己失败，只剩 Kill 兜底，HEAD 相同，补 `cmd.Env` 的改法本刀不做）。B2 的注释属于冻结的生产代码块，按裁决一个字节不改，它那句话只管路径这层意思写在 (l) 与 §4「只在审查时看」。
- **m5**：§7 (b) 照 W1 的实测改写、去掉 [推断]；v1.1 另在原型里复测了一次（`GOARCH=386` 与 `amd64` 都打出 `C:\Windows\system32 <nil>`）。
- **(f)**：命令表 team 的 summary 补「（总结成员除外）」（A9，§3 第 3 句）。先核过：全树里这一句只有命令表一处，没有既有测试钉着（v1.1 全量运行也没有既有用例因它变红）。新用例 C9 `TestClxHelpTeamLine` 按 `/help` 整行钉住，期望取原型实跑（另用原型构建的 `council.exe -no-tui` 在沙盒里敲 `/help` 核过同一行）。§4 登记 V17，杀手 C9，实测变红。这是 v1.1 唯一的生产代码改动。
- **(g)**：B8（oby 里 L5 的【判别力】）、B9（obx 文件头的括号）两处只改注释。`obx_loginstatus_exec_test.go` 因此进了改动文件表，改动或新增的文件由 11 个变成 12 个。
- (c) 留 §7 原文；(a)、(d)、(h)、(i)、(j) 原文不动（(d) 里「C4-C7」是 v1 的说法，按裁决不改）；`commands.go:964` 的空段用法句记 §7 (k)。
- 过程中自己发现、已改的一处：第一批变异之后发现 C5 第 2 步与 C8 正对照用的是已被第一步杀掉（变异 V11 下）的循环的 pid，变异下会对一个已退出、可能被系统复用的 pid 起真 taskkill。改成这两步各自另起一个循环，turn 的 10 条变异全部重跑（杀手与第一批逐条相同），§4 记的是重跑那一批。
- **生产代码比对**（脚本）：v1 的 10 个生产代码冻结块（A1-A6、B1-B4）在 v1.1 里锚点与替换文本都逐字节相同；v1.1 新增的生产代码块只有 A9 一个。
- **验证结果**（每一次都照 §4 表头的运行纪律，每次 go test 之后四项核对都干净）：最终字节上 `gofmt -l ./internal ./cmd` 空，`go build ./...`、`go vet ./...` 干净，`GOOS=linux go vet ./internal/turn ./internal/repl ./internal/tui` 干净；全量 `go test -count=1 ./...` 九包全绿（repl 76.4 s、tui 61.8 s、turn 19.3 s、app 11.8 s，其余不到 4 s）；`go test -count=3 -run 'TestClx|TestObyRunLoginStatusKilled' ./internal/repl ./internal/tui ./internal/turn` 全绿。§4 的 22 条变异（v1 的 18 条全部重跑，加 V17、X1、X2、X4）：锚点各恰好一次，还原后 SHA256 一致，20 条被点名杀手杀掉，E1、E2 为 0 个，见 §4「v1.1 重跑」。往返核对与编码扫描见本文末尾「往返核对（v1.1）」：18 对锚点、3 个整文件从本文取回、打在新解出的 `483a766` 上，整棵树 375 个文件与原型逐字节相同。

**严重度的判法（W1 与以后的审查员照这个判，逐字执行）**：只有下面四种算 major 以上：
- 不该写配置时写了；
- 对上已知成员或模型，却被静默丢掉或加错；
- 该杀的进程没杀或杀错了进程、或执行了不该执行的程序；
- 测试写出了隔离目录。

措辞、覆盖缺口、说法不够准，一律算 minor。

**起草记录（v1，原型实跑）**：

- 隔离目录 `E:\Isolating_test\council\cl1_draft\`：`git -C <仓库> archive 483a766` 解出多份（原型 `proto\`、HEAD 对照 `head\`、变异用的三份工作副本 `m1\`、`m2\`、`m3\`、只套生产代码的 `pc\`、往返核对用的 `rt0\` 与 `rt\`；每份解出时 372 个文件）；主仓库一个字节没写（只用了 `git archive`、`git rev-parse` 与 `git status`）。原型不交付，交付前连同隔离目录一起删除。
- 每一次 go build / vet / test / 变异运行都在同一条 PowerShell 命令里先 dot-source 纯 ASCII 的 `goenv.ps1`（§4 表头），每次 go test 之后核四项，次次干净（本文末尾「泄漏核对」）。
- 原型（最终字节）上：`go build ./...`、`go vet ./...` 干净；`gofmt -l ./internal ./cmd` 空；`GOOS=linux go vet ./internal/turn ./internal/repl ./internal/tui` 干净（HEAD 上同样干净，见 D8）。
- 原型上全量 `go test -count=1 ./...` 九包全绿（app 11.0 s、repl 78.2 s、tui 62.0 s、turn 14.3 s、config 3.5 s、store 3.1 s，其余不到 1 s）。`go test -count=3 -run TestClx ./internal/repl ./internal/tui ./internal/turn` 全绿（repl 0.1 s、tui 0.1 s、turn 1.8 s）。
- 既有测试只改了任务书授权的那几处（§1.1 的 A7、A8、B5-B7），原型上全绿。另做了一份「只套生产代码（A1-A6、B1-B4）、既有测试保持 HEAD、不加 clx 文件」的拷贝：repl 包红的恰好是 `TestCxhHelpAddLine`、`TestUx2rJoinIsAdd`（钉着旧 summary），turn 包全绿；但在这份拷贝上施加 V13（19e 的 N32，只删 `Kill`），HEAD 写法的 L5b 照绿（原型实跑）——taskkill 从系统目录照样找得到，L5b 已经测不到「只剩 Kill」。所以 A7、A8 与 B5-B7 三处既有测试的改动都是必须的。
- §4 的 18 条变异（V1-V16、E1、E2）逐条在原型上跑：16 条被点名杀手杀掉，E1、E2 等价、实测 0 个用例变红。每条从字节副本还原、SHA256 一致，每条之后四项核对干净。
- 往返核对：见本文末尾「往返核对」。

**与已落地各刀的相交**（逐个点名；本刀对它们的代码一个字不改，除非下表写了「改」）：

| 已落地 | 相交的文件 / 函数 | 本刀怎么用 |
|---|---|---|
| Step 9 片 2c（`/team use`）| `internal/repl/commands.go`：`cmdTeamUse`（HEAD `:665-767`）、命令表里 team 的 summary（HEAD `:154`）；`team_use_test.go` 的 `newTeamTrioConfig`、`newJoinTrackingStore`、`joinTrackingStore`、`joinCountFor` | **改**（A2 头注释加一段，A3 在「两张表都查不到」那一支 Join 之前加总结成员那一条；v1.1 起 A9 给 team 的 summary 补「（总结成员除外）」）。在场、已退出两支一个字不改；`team_use_test.go` 只读复用，它的六个测试函数原样绿 |
| CH1（19a，#47）总结成员不当普通成员加 | `resolveAddConfig` 第 0 步、`printAddCandidates` 的说法、`runSummary`（`summary.go:85-115`）| 不改。A3 的新句照第 0 步的句式写（§3） |
| MD1、UX2R（`/help` 的 add 行）| 命令表 `commands.go:159`；`cxh_fixture_test.go:223` 的 `cxhHelpAddLine`（`TestCxhHelpAddLine`，`cxh_addmodel_test.go:219`）、`ux2r_add_test.go:448` 的 `wantAddSummary`（`TestUx2rJoinIsAdd`，`:411`）| **改**（A1 换 summary；A7、A8 是任务书授权改的两处期望，新期望取原型实跑） |
| AD1（19g，#49）| `addname.go` 文件头、`parseAddNameArgs`（`:251-278`）；`commands.go` 里 `cmdAdd` 的头注释（`:877-879`）；`adn_addname_test.go` 的装置 | **改**（A4、A5 只改注释）；`parseAddNameArgs` 一个字不改；`adn` 文件只读复用（`adnNewRig`、`adnRefused`、`adnAdded`、`adnNoDiscovery`、`adnUsage`、`adnBlockSol`），不改 |
| OB3（19f，#48）| `internal/repl/addlogin.go` 的 `LoginCommand` 注释（`:29-30`）；`app/firstrun.go:63-65` 的限定说法、`msgFirstRunNoneFound`（`:48-53`）| **改**（A6 只改注释）。`firstrun.go` 一个字不改 |
| OB1、OB2（19d、19e，#48）| `internal/turn/killtree_windows.go` 的 `killProcessTree`；`killtree_other.go`；`loginstatus_exec.go`、`discovery_exec.go`、`spawn.go` 三处调用方；`oby_loginstatus_exec_test.go` 的 L5b、`obx_loginstatus_exec_test.go` 的 `obxStatusFake`、`obxLoopLines` | **改**（B1-B4 生产代码；B5-B7 是任务书授权改的 L5b；v1.1 起 B8 改 oby 里 L5 的【判别力】一句注释、B9 改 `obx_loginstatus_exec_test.go` 文件头一句注释，都是主代理裁决 (g) 授权的）。三处调用方一个字不改；`obx` 的辅助只读复用，代码一个字不改 |
| AT1、AT1-FU（19b、19c，#47）| `internal/tui/model.go:375` 列表分支的 Ctrl-C 判据；`atx_model_test.go`、`tui1b_model_test.go` 的辅助；`atf_review_test.go` 的 F5 | 不改生产代码；新测试只读复用 atx、tui1b 的辅助 |
| TestMain | repl、tui、turn 包 | 不改、不另加 |

---

## 0. 动机 / 实测（每一条的现状，HEAD 实跑与原型实跑）

1. **`/help` 里 `/add` 那一行**（第 1 条）。HEAD `commands.go:159` 的 summary 只说 `/add +编号`，没提 AD1 的按名字加（19g D14 有意不改，§7 (e)）。在 `internal` 下的 Go 文件里按头段「把 agent 加进当前会话」搜：只有 `commands.go:159`、`cxh_fixture_test.go:223`、`ux2r_add_test.go:448` 三处（实测；`docs` 里的契约另有引用，不算）。原型换了 summary 之后，用沙盒配置起原型的 `council -no-tui`（§6 第 5 步同一份配置）敲 `/help`，那一行逐字是（原型实跑；COMMAND 列由 tabwriter 补到 12 个字符）：

   ```text
   /add        把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别
   ```

   格式与命令表里其它带用法的行（`/round`、`/session`、`/auto` 等）相同：`/命令` 补到 12 个字符，冒号后接用法，分号隔开。
2. **`--id` 缺值守卫**（第 2 条）。HEAD `addname.go:257` 的 `if a.haveID || i+1 >= len(fields) {` 是对的：HEAD 上 `/add gpt-6.1-sol --id` 打按名字加的用法那一句、不识别、不写（C1 在 HEAD 拷贝上实跑是绿的）。但去掉 `i+1 >= len(fields)` 之后整个 repl 包 0 个既有用例变红（§4 V2 的杀手只有本刀新加的 C1，原型实跑），后果是越界 panic：`dispatch("/add gpt-6.1-sol --id") panic: runtime error: index out of range [2] with length 2`（原型实跑，`mdxRig.runCtx` 接住时打的那一句）。`--effort` 那一半（`:264`）有既有杀手 `TestAdnFlags` 的 `/add gpt-6.1-sol --effort`（§4 V3，原型实跑）。
3. **两处注释**（第 3 条）。HEAD `commands.go:877-879`（`cmdAdd` 头注释 AD1 那一段）说「有段在配置里对不上时到识别快照里按名字找，找到就写进配置并加入」；`addname.go:13-15` 说「找到就走与 /add +编号 同一个写配置流程」。两处都漏了：两个词与带 `--id` / `--effort` 时，名字在配置里只是前缀命中也会去识别（19g D7、D9）；D17 的结局可能只是加入已有成员，或者拒绝、列出，都不写配置。代码不受影响。
4. **`LoginCommand` 的注释**（第 4 条）。HEAD `addlogin.go:29` 说「首次运行的登录办法与 /add 登录段里没登录那一行用的都是它」；`app/firstrun.go:63-65` 已经限定成「结果之后列出的登录办法同样只有 repl.LoginCommand 那一份；msgFirstRunNoneFound 里的安装说明另写着各家的登录办法」。`msgFirstRunNoneFound`（`firstrun.go:48-53`）另写着四家的登录办法（「装好后运行 claude auth login」等），不经 `LoginCommand`。
5. **`/team use` 把总结成员当普通成员加入**（第 5 条）。
   - HEAD `cmdTeamUse`（`commands.go:733-765`）对 team 的每个成员：已是成员（在场或已退出）的跳过（已退出的点名一行），其余一律 Join，不看 `[summary]`。
   - **配置校验允许**：`config/validate.go:83-131` 对 team 的 members 只查「不许为空」「不许重复」「必须是已定义的 agent」，不看总结成员；`:136-143` 对 `[summary] agent` 只查「已定义」与 `permissions = "read"`。下面这份沙盒配置（§6 第 5 步）加载、校验都过（「已加载 3 个 agent，1 个 team」，HEAD 与原型都实跑过）。
   - **HEAD 实跑**（沙盒配置：`cl1-a`、`cl1-b`、`cl1-sum` 三个 agent，`[summary] agent = 'cl1-sum'`，`[teams.cl1] members = ['cl1-a', 'cl1-b', 'cl1-sum']`；用 HEAD 构建的 `council -no-tui -config <沙盒>` 敲 `/new cl1check`、`/team use cl1`、`/who`，会话 id 写成 `<conv>`）：

     ```text
     已绑定 team cl1
     cl1-a 已加入会话 <conv>
     cl1-b 已加入会话 <conv>
     cl1-sum 已加入会话 <conv>
     ```

     `/who` 里 `cl1-sum` 是在场成员。原型同一份配置、同几条命令（原型实跑）：

     ```text
     已绑定 team cl1
     cl1-a 已加入会话 <conv>
     cl1-b 已加入会话 <conv>
     cl1-sum 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结
     ```

     `/who` 里只有 `cl1-a`、`cl1-b`。
   - HEAD 拷贝上跑 C2：`not_member` 红（`ag_alpha 已加入会话 convclx01`、Join 次数多了 `ag_alpha:1`、在场多了 `ag_alpha`），`no_summary`、`present`、`left` 绿（HEAD 实跑）。
   - **同类的还有没有别处**（逐个核过，生产代码里调 `r.svc.Join` 的五处）：`addmodel.go:521`（新成员，id 是新写进配置的，不可能是已有的总结成员）；`addname_existing.go:88`（D17 的 S 已去掉总结成员，19g V78）；`commands.go:995`（`cmdAdd`，`resolveAddConfig` 第 0 步拦总结成员，只放过「本会话里已退出的总结成员」那条 `/join` 恢复路）；`summary.go:105`（`runSummary` 自己 Join，正是它该加入的地方）；`commands.go:759`（`cmdTeamUse`，本刀改的这一处）。`/join` 与 `/add` 是同一个 handler（`commands.go:160`）。读 team 的别的路径：`cmdTeams`（`commands.go:624-654`）只列表；`turn/runner.go:1016` 只按 team 定 thought 的可见性，不 Join。所以只有 `/team use` 这一处。
   - **v1.1 补**：命令表里 team 的 summary（HEAD `commands.go:154`）是「绑定当前会话到一个 team，并把尚未加入的成员一次性加进来: /team use <team-id>」，没提总结成员这个例外。在 HEAD 拷贝的全树里搜「一次性加进来」，只有这一处（另有 `docs/14_MVP_CHECKLIST.md` 的 C6 标题用的是另一种说法），`internal` 下没有测试钉着它（实测）。主代理裁决 (f) 定改（A9），新期望由新用例 C9 钉住（§2.1）。
6. **`killProcessTree` 按 PATH 找 taskkill**（第 6 条）。
   - HEAD `killtree_windows.go:25`：`exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))`，`exec.Command` 对不带路径的名字走 `exec.LookPath`，按 PATH 的目录顺序、每个目录按 PATHEXT 找。
   - **HEAD 实跑**（C4 的装置去掉接缝那几行，放进 HEAD 拷贝跑一次，跑完即删）：`t.TempDir()` 里放一个假的 `taskkill.cmd`（只把参数写进标记文件、以 0 退出），目录排在 PATH 最前；`exec.LookPath("taskkill")` 解析到它；起一个 cmd 循环，调 `killProcessTree(pid)`：假的被执行了（标记文件出现），循环 10 s 内没有退出（「该杀的进程没杀，执行了不该执行的程序」）。
   - 三处调用方（`spawn.go:192`、`discovery_exec.go:133`、`loginstatus_exec.go:56`）都不看返回值，紧接着调 `cmd.Process.Kill()`。
   - **同类的还有没有别处**（全仓库 `exec.Command` 与 `LookPath`，生产代码）：按名字起系统程序的只有这一处。`spawn.go:110/150`、`discovery_exec.go:35/39`、`loginstatus_exec.go:29/32` 起的是配置里写的 provider 命令（用户自己给的名字或路径，按 PATH 解析是本意）；`budget.go:366`、`rules_report.go:184` 的 `LookPath` 只为算命令行长度与打报告，不起进程；`config/firstrun.go:40` 的 `LookPath(id)` 是检测本身（当前目录同名文件遮住 PATH 那一条是 19d §7 (k)，本刀不做）。第三方依赖（bubbletea、atotto/clipboard 等）不在本刀范围。
   - `golang.org/x/sys` 是 `go.mod` 的直接依赖（v0.47.0，modcache 里有），`windows.GetSystemDirectory` 在 `E:\Go\modcache\golang.org\x\sys@v0.47.0\windows\security_windows.go:780`（实测）。`internal/turn` 在 HEAD 上没有引用 x/sys。
   - L5b（`oby_loginstatus_exec_test.go:47-73`）今天靠 `t.Setenv("PATH", t.TempDir())` 让 `killProcessTree` 找不到 taskkill。改成按系统目录找之后这个模拟失效：只套生产代码、L5b 保持 HEAD 时施加 19e 的 N32（只删 `Kill`，本文 V13），L5b 照绿（原型实跑）。所以任务书授权把它改成换接缝（B5-B7）；改后 V13 下 L5b 红：`RunLoginStatus 用了 5.3007957s 才返回，应当小于 2.5s（找不到 taskkill 时也要靠 Kill 杀掉）`（原型实跑）。
   - **v1.1 补（W1 审查员的三条，v1.1 原型实跑复核）**：
     - X1（`exec.Command(filepath.Base(path), ...)`，去掉目录之后按 PATH 找 `taskkill.exe`）活过 v1 的 C4-C7：C4 的假文件只有 `taskkill.cmd`，按 `taskkill.exe` 这个全名找时绕得过它；v1 的 C5 让接缝指向的假文件名字与目录本身就在 PATH 上。v1.1 给 C5 加第 2 步（接缝指向不在 PATH 上、名字不叫 taskkill 的 `clx-fake-tk.cmd`），另加 C8（Go 现编的假 `taskkill.exe` 排在 PATH 最前），见 §2.3、§4 X1。
     - X2（`systemTaskkillPath` 改用 `SystemRoot` 拼 `System32`）在本机与 `GetSystemDirectory` 给出同一个路径，v1 的 C7 照绿；v1.1 的 C7 末步把 `SystemRoot` 改成 `Z:\clx-nope` 再调一次，断言结果不变，见 §4 X2。
     - `SystemRoot` 坏掉（空、相对路径、不存在的目录、unset）时，`systemTaskkillPath` 给的路径不变，PATH 上的假 taskkill 也不会被执行；但系统目录里的 `taskkill.exe` 自己会以 exit status 1 退出、杀不掉树，只剩 `Kill` 兜底；HEAD 按名字起 taskkill 时相同（W1 审查员的探针实测，本文没有复跑）。D6 因此收窄成「路径不随 SystemRoot 变」，这一条记 §7 (l)。
7. **列表分支的 Ctrl+Alt+C**（第 7 条）。HEAD `tui/model.go:375` 的判据 `msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c'` 是对的：C3 在 HEAD 拷贝上实跑是绿的（列表开着时 Ctrl+Alt+C 只收起）。19c 头那条审查变异（`msg.Mod&^tea.ModShift == tea.ModCtrl`，本文 V15）在原型上实跑：整个 tui 包只有 C3 红，`TestAtfCtrlShiftCClosesList` 照绿——缺口属实。V15 下 C3 第 2 步的三处失败是「Cmd 应当是 nil，实际非 nil」「行仍是 atxB3」「表头 正在退出」（原型实跑）。键 `tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModAlt}` 的 `String()` 是 `ctrl+alt+c`（原型实跑）。
   - **v1.1 补（W1 审查员 m3）**：判据写成 `msg.Mod&^(tea.ModShift|tea.ModAlt) == tea.ModCtrl`（X4）能同时活过 v1 的 C3 与 `TestAtfCtrlShiftCClosesList`。Windows 控制台的输入会把 CapsLock、NumLock、ScrollLock 的状态放进 `Mod`（ultraviolet `decoder.go:2082-2101` 的 `translateControlKeyState`，W1 审查员读的源码；bubbletea v2.0.9 `mod.go:24-25` 导出 `tea.ModCapsLock`、`tea.ModNumLock`，v1.1 核过）。v1.1 把 C3 第 2 步改成三种键的小表（Ctrl 加 Alt、NumLock、CapsLock，各起新 Model），见 §2.2、§4 X4。锁定键不进 `String()`：`tea.ModCtrl | tea.ModNumLock` 与 `tea.ModCtrl | tea.ModCapsLock` 的 `c` 键 `String()` 都是 `ctrl+c`（原型实跑）。
   - **同类的还有没有别处**（v1.1，只核、不改代码）：列表分支其余五个键（`model.go:378-391` 的 Esc、↑、↓、Tab、回车）只比 `msg.Code`、不看 `Mod`，NumLock 开着时照常。原型上用一次性探针实跑过（列表开着，先按 ↓ 再按这个键，带 `tea.ModNumLock` 与不带两种，行、值、表头、Cmd 是否为 nil 逐项相同；探针跑完即删，不交付）。没有缺口，记 §7 (m)。

---

## 1. 范围

### 1.1 做

#### 裁定（主代理定的方向，起草人冻结的细节；逐条冻结）

- **D1 `/help` 的 add 行**（A1）。summary 冻结成 §3 第 1 句。照着任务书「识别出的模型可以直接 /add 它的名字」写，括号里点明名字是识别表里的 ID 或模型名（与 `msgAddNameUsage` 的「<识别表里的 ID 或模型名>」同一个说法）；`+编号` 留着；`[--id <新 id>] [--effort <强度>]` 两种都能带（AD1 D9 与 +编号 相同）；「配置里还没有就先写进配置再加入」照实说：按名字加对上配置里已有的同模型成员时只加入、不写（D17），+编号 的行本来就不在配置里。两个词的写法（`/add <模型名> <强度>`）不写进 `/help`：`--effort` 已经说了怎么带强度，`/help` 一行装不下全部写法，细节在 README。格式照命令表的其它行：冒号后接用法、分号隔开、`/add --refresh 重新识别` 收尾不动。`join` 的 summary「同 /add」不动。A7、A8 是任务书授权改的两处期望，新期望取原型实跑（`TestCxhHelpAddLine` 整行相等、`TestUx2rJoinIsAdd` summary 字段相等，原型上都绿）。
- **D2 `--id` 缺值的用例**（C1，只加测试）。新测试文件里，开关打开的 `adnNewRig` 上敲 `/add gpt-6.1-sol --id` 与 `/add --id`，各自整行是 `adnUsage`、配置字节与备份个数不变、在场成员不变（`adnRefused`），两条之后执行器 0 次（`adnNoDiscovery`）；同一装置接着敲 `/add gpt-6.1-sol --id sol` 是正对照（识别两家、新建 `sol`、写配置并加入，`adnAdded`）。变异 V2（`--id` 那一半去掉 `i+1 >= len(fields)`）的判法：第一条命令在 `parseAddNameArgs` 里越界 panic，`mdxRig.runCtx`（`mdx_addmodel_fixture_test.go:543-559`）`recover` 之后 `Fatalf("dispatch(%q) panic: %v")`，这个用例红、包里其余用例照跑。panic 没有被接住（整个测试二进制崩掉、`go test -json` 里这个用例的 fail 事件带 `panic:` 输出）同样算红。不改 `adn_addname_test.go`。
- **D3 两处注释**（A4、A5，只改注释）。补的两件事照 19g 文件头：两个词与带 `--id` / `--effort` 时名字在配置里只是前缀命中也会去识别快照里找；找到之后不一定写配置——没给强度也没给 `--id` 的单个名字，配置里已有同模型成员时只是加入那个成员，或拒绝、列出（`addNameExisting`）。`addname.go` 文件头第一段原有的「快照里的每一行都能按名字加……」两句原样留着。
- **D4 `LoginCommand` 的注释**（A6，只改注释）。照 `firstrun.go:63-65` 的限定说法：首次运行**在结果之后列出的**登录办法与 /add 登录段里没登录那一行用的都是它；`msgFirstRunNoneFound` 另写着、不经它，指向 19f §7 (j)。`addlogin.go` 文件头「首次运行与 /add 共用的两样东西」那一句说的是「共用」，不是「只有一份」，成立，不改。
- **D5 `/team use` 与总结成员**（A2、A3）。
  - 在「两张表里都查不到」那一支、`r.cfg.Agents[agentID]` 之前插一条：`[summary] agent` 非空、而且等于这个成员时，打 §3 第 2 句、`continue`，不 Join、不读它的配置。判据只比 id（与 `resolveAddConfig` 第 0 步、`printAddCandidates` 同一个 `r.cfg.Summary.Agent`）。
  - **已是成员的两支不变**：在场的照旧静默跳过；已退出的照旧打「X 已退出，未自动拉回，要加回用 /join」（与 `runSummary` 那句「要恢复总结用 /join」一致，`/join` 一个已退出的总结成员由第 0 步放行）。
  - 措辞照 #47 的总结成员说法：「是总结成员（config.toml 的 [summary]）」照第 0 步，「多个成员回复之后它会自己加入做总结」照第 0 步与 `printAddCandidates`，中间换成「不随 team 加入」（`/team use` 里没有敲 `/add`，说「不用 /add」不对题）。
  - 顺序：与其它成员一起按 `t.Members` 的顺序逐行打，打在它的位置上（沙盒实跑：`cl1-sum` 排在最后，那一行也在最后）。
  - 不改「已绑定 team」那一行、`BindTeam`、`r.cur` 的刷新、成员表的读法。
  - **v1.1（主代理裁决 (f)）**：命令表里 `team` 的 summary 在「尚未加入的成员」之后补「（总结成员除外）」，冻结成 §3 第 3 句（A9）；这是 v1.1 唯一的生产代码改动。HEAD 上没有测试钉着这一句（§0 第 5 条），新用例 C9 按 `/help` 整行钉住它（§4 V17）。
  - 头注释（A2）在「只有两张表里都查不到的才 Join」那一段后面加一段说明例外。
- **D6 `killProcessTree` 只起系统目录里的 taskkill.exe**（B1、B2；任务书第 6 条）。
  - 新包级变量 `taskkillPath`（接缝，类型 `func() (string, error)`），默认值是新函数 `systemTaskkillPath`。`killProcessTree` 先调接缝：返回错误就原样返回这个错误、**什么都不起**；否则 `exec.Command(<完整路径>, "/T", "/F", "/PID", <pid>)`，其余（`HideWindow`、`cmd.Run()`、`pid <= 0` 直接返回）不变。
  - `systemTaskkillPath` 取 `windows.GetSystemDirectory()`（`golang.org/x/sys/windows`），拼上 `taskkill.exe`。**不读 `SystemRoot`**（任务书的例子是 `%SystemRoot%\System32\taskkill.exe`，「SystemRoot 为空时怎么办你定」）：理由是系统目录由系统调用给出，**路径不随 `SystemRoot` 变**（v1.1 收窄）：`SystemRoot` 为空、被改、被子环境清掉，`systemTaskkillPath` 给的路径都不变（C7 末步担保），所以算路径时没有「为空时怎么办」这一种情形。这句话只管路径：`SystemRoot` 坏掉时系统目录里的 `taskkill.exe` 照样被起，但它自己会失败，只剩 `Kill` 兜底，与 HEAD 相同（§7 (l)）。`x/sys` 是现成的直接依赖，不加新依赖。系统调用失败时返回错误，`killProcessTree` 照上一条什么都不起，**绝不退回按 PATH 找**。三处调用方本来就不看返回值、紧接着调 `cmd.Process.Kill()`，所以最坏是只杀到垫片那一层，与 HEAD 上「PATH 里没有 taskkill」相同。
  - 本机核对：`systemTaskkillPath()` 等于 `%SystemRoot%\System32\taskkill.exe`（不分大小写），PATH 最前放了假 `taskkill.cmd` 时也不变；v1.1 起 C7 末步把 `SystemRoot` 改成 `Z:\clx-nope` 再调一次，结果不变（C7，原型实跑；C7 先用 `SystemRoot` 核对路径，再改它证明函数不读它）。
  - 接缝是包级变量，只给测试换：L5b 换成返回错误的函数（模拟找不到 taskkill），C5 换成指向假文件的函数（第 1 步指向 PATH 上的假 `taskkill.cmd`，第 2 步指向不在 PATH 上的假 `clx-fake-tk.cmd`），C8 的正对照换成指向 Go 现编的假 `taskkill.exe`。`internal/turn` 的测试没有 `t.Parallel`（全包 grep 0 处，实测），换包级变量不会与别的用例并发；换的用例都用 `t.Cleanup` 还原。
- **D7 非 Windows 的同名变量**（B3、B4）。`oby_loginstatus_exec_test.go` 没有构建约束，L5b 改成引用 `taskkillPath` 之后，如果它只在 `killtree_windows.go` 里声明，`GOOS=linux go vet ./internal/turn` 会编译不过（HEAD 上这条是干净的，实测）。所以 `killtree_other.go` 里留一个同名变量（返回「taskkill is windows-only」错误），注释写明它只为让不带构建约束的测试在别的平台编译得过，那边的 `killProcessTree` 不调它。另一种做法是给 oby 文件加 `//go:build windows`，那是改 L5b 之外的既有测试，任务书没授权，不做。
- **D8 平台核对**。原型上 `GOOS=linux go vet ./internal/turn ./internal/repl ./internal/tui` 干净（原型实跑）；新 turn 测试文件名以 `_windows_test.go` 结尾（只在 Windows 编译），因为它起 `.cmd`、引用只在 Windows 上有的 `systemTaskkillPath`。
- **D9 Ctrl+Alt+C 的用例**（C3，只加测试）。新测试文件，照 19c F5 的形状：列表开着时按 Ctrl+Alt+C，Cmd 为 nil、行 nil、值仍是 `@b`、表头仍是「空闲」；对照（同装置、列表不在）同一个键的 Cmd 是 `tea.Quit`（只比函数指针）、表头「正在退出」。Cmd 一律不执行，用法只许 19c §2.0 的三种。§4 登记 V15（19c 头的那条）与 V16（19c 的 A60）。**v1.1（主代理裁决 m3）**：键改成小表，Ctrl 加 Alt、NumLock、CapsLock 三种，每种一个子测试、各起新 Model，三步都照上面走；测试函数改名 `TestClxCtrlCModsClosesList`（文件名不变）。§4 另登记 X4。
- **D10 交付分块与跨文件约束**：
  - **块 A（`internal/repl`，只依赖 HEAD）**：A1-A6 与 A9 生产代码七处补丁（`commands.go` 五处、`addname.go` 一处、`addlogin.go` 一处，替换文本合计 40 行，其中真正改行为的只有 A1、A9（两句 help 文本）与 A3（14 行）），外加 A7、A8 两处授权的既有测试期望（各 1 行）。**A1 与 A7、A8 必须一起落**：只落 A1，`TestCxhHelpAddLine`、`TestUx2rJoinIsAdd` 红（§4 V1 的反面，两条既有用例钉着 summary）。A9 没有既有用例钉着，单落不会让既有用例变红；钉它的是测试腿的 C9。
  - **块 B（`internal/turn`，只依赖 HEAD）**：B1-B4 生产代码四处补丁（`killtree_windows.go` 两处、`killtree_other.go` 两处，替换文本合计 57 行），外加 B5-B7 三处授权的 L5b 改动（`oby_loginstatus_exec_test.go`，替换文本合计 15 行）与 B8、B9 两处授权的注释（`oby_loginstatus_exec_test.go`、`obx_loginstatus_exec_test.go` 各 1 行）。**B5-B7 依赖 B1-B4**（引用 `taskkillPath`），B3、B4 是为了 B5-B7 在非 Windows 上也编译得过；四处生产补丁与三处 L5b 补丁要一起落。B8、B9 只改注释，说的是 B1-B4 之后的行为，与它们一起落。
  - 块 A 与块 B 互不依赖，可以分两路并行实现、分开落地。`internal/tui` 没有生产代码改动。
  - 测试腿的三个文件：S1（`internal/repl/clx_cl1_test.go`，201 行）依赖块 A（C2 要 A3，C9 要 A9；C1 在 HEAD 上就绿）；S2（`internal/tui/clx_ctrlaltc_test.go`，70 行）只依赖 HEAD；S3（`internal/turn/clx_killtree_windows_test.go`，322 行）依赖块 B（引用 `taskkillPath`、`systemTaskkillPath`）。
  - 都在 400 行以内。

#### 块 A：`internal/repl`（七处生产补丁（v1.1 加 A9）、两处授权的既有测试期望；只依赖 HEAD）

**A1. `internal/repl/commands.go`：命令表里 add 的 summary**（HEAD `:159`）

```anchor internal/repl/commands.go
		{name: "add", summary: "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别", needsSvc: true, needsConv: true, run: cmdAdd},
```

```replace
		{name: "add", summary: "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别", needsSvc: true, needsConv: true, run: cmdAdd},
```

**A2. `internal/repl/commands.go`：`cmdTeamUse` 头注释加一段**（HEAD `:676-678`）

```anchor internal/repl/commands.go
// 只有两张表里都查不到的才 Join。点名这件事进契约：静默跳过等于
// 让用户以为整个 team 都进来了。
//
```

```replace
// 只有两张表里都查不到的才 Join。点名这件事进契约：静默跳过等于
// 让用户以为整个 team 都进来了。
//
// 例外是总结成员（[summary] agent，#47，CL1）：配置校验允许它出现在 team 的 members 里，
// 两张表里都查不到它时也不 Join，同样点名一行（「不随 team 加入」），多个成员回复之后由
// runSummary 自己加入。在场、已退出的总结成员照上面两条走，与普通成员相同。
//
```

**A3. `internal/repl/commands.go`：`cmdTeamUse` 里总结成员不 Join**（HEAD `:744-747`）

```anchor internal/repl/commands.go
			continue
		}

		ac, ok := r.cfg.Agents[agentID]
```

```replace
			continue
		}

		// 总结成员（[summary] agent）不当普通成员加（#47，CL1）：它在 team 的 members 里、本会话里
		// 又还不是成员时不 Join，打一行说明；多个成员回复之后由 runSummary 自己 Join。已是成员的
		// （在场、已退出）走上面那一支，与它是不是总结成员无关。
		if sumID := r.cfg.Summary.Agent; sumID != "" && agentID == sumID {
			if err := r.printf("%s 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结\n", agentID); err != nil {
				return err
			}
			continue
		}

		ac, ok := r.cfg.Agents[agentID]
```

**A4. `internal/repl/commands.go`：`cmdAdd` 头注释 AD1 那一段**（HEAD `:877-879`，只改注释）

```anchor internal/repl/commands.go
// 开关打开时（SetAddByName，app 装配时打开），带参数的 /add 先交给 cmdAddByName
// （addname.go，AD1）：每一段都在配置里命中、也没有 --id / --effort 时它不插手，
// 下面照旧；有段在配置里对不上时到识别快照里按名字找，找到就写进配置并加入。
```

```replace
// 开关打开时（SetAddByName，app 装配时打开），带参数的 /add 先交给 cmdAddByName
// （addname.go，AD1）：每一段都在配置里命中、也没有 --id / --effort 时它不插手，
// 下面照旧。有段在配置里对不上时到识别快照里按名字找；两个词的写法与带 --id / --effort
// 的写法里，名字在配置里只是前缀命中也会去快照里找。找到之后多半是写进配置并加入；没给
// 强度、也没给 --id 的单个名字，若配置里已有同一个模型的成员，就只是加入那个成员，或者
// 拒绝、列出，都不写配置（addname_existing.go 的 addNameExisting）。
```

**A5. `internal/repl/addname.go`：文件头第一段**（HEAD `:13-17`，只改注释）

```anchor internal/repl/addname.go
// /add <名字> 的名字在配置里对不上（resolveAddConfig 的第 0-3 步都没命中）时，到本会话
// 的识别快照里找；找到就走与 /add +编号 同一个写配置流程（addDiscoveredModel），屏幕
// 输出与写进文件的字节都相同。快照里的每一行都能按名字加，包括拍快照时已在配置里、
// 没有编号的行（16a open item (c)）；custom/ 开头的 codex 行不进快照，加不了。本会话
// 还没有快照时先识别一次（ensureModelSnapshot，与不带参数的 /add 同一份快照）。
```

```replace
// /add <名字> 的名字在配置里对不上（resolveAddConfig 的第 0-3 步都没命中）时，到本会话
// 的识别快照里找；两个词的写法（/add <模型名> <强度>）与带 --id / --effort 的写法里，名字
// 在配置里只是前缀命中（configPrefixOnly）也会去找。找到之后要新建成员的，走与 /add +编号
// 同一个写配置流程（addDiscoveredModel），屏幕输出与写进文件的字节都相同；不是每一种都
// 写配置：没给强度也没给 --id 的单个名字可能只是加入配置里已有的成员，或者拒绝、列出，
// 见下面「单个名字没给强度」那一段。快照里的每一行都能按名字加，包括拍快照时已在配置里、
// 没有编号的行（16a open item (c)）；custom/ 开头的 codex 行不进快照，加不了。本会话
// 还没有快照时先识别一次（ensureModelSnapshot，与不带参数的 /add 同一份快照）。
```

**A6. `internal/repl/addlogin.go`：`LoginCommand` 的注释**（HEAD `:29-30`，只改注释）

```anchor internal/repl/addlogin.go
// LoginCommand 是一家的登录办法；首次运行的登录办法与 /add 登录段里没登录那一行用的都是它。
// 不认识的 id 返回空串。
```

```replace
// LoginCommand 是一家的登录办法；首次运行在结果之后列出的登录办法与 /add 登录段里没登录那一行
// 用的都是它。首次运行一家都没找到时的安装说明（app 包的 msgFirstRunNoneFound）另写着各家的
// 登录办法，不经它（19f §7 (j)）。不认识的 id 返回空串。
```

**A7. `internal/repl/cxh_fixture_test.go`：`cxhHelpAddLine`（任务书授权的既有期望）**（HEAD `:223`）

```anchor internal/repl/cxh_fixture_test.go
	cxhHelpAddLine  = "/add        把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别"
```

```replace
	cxhHelpAddLine  = "/add        把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别"
```

**A8. `internal/repl/ux2r_add_test.go`：`wantAddSummary`（任务书授权的既有期望）**（HEAD `:448`）

```anchor internal/repl/ux2r_add_test.go
	const wantAddSummary = "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别"
```

```replace
	const wantAddSummary = "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别"
```

**A9. `internal/repl/commands.go`：命令表里 team 的 summary**（v1.1，主代理裁决 (f)；HEAD `:154`，在 A1 之上，文件顺序排在 A1 之前）

```anchor internal/repl/commands.go
		{name: "team", summary: "绑定当前会话到一个 team，并把尚未加入的成员一次性加进来: /team use <team-id>", needsSvc: true, needsConv: true, run: cmdTeam},
```

```replace
		{name: "team", summary: "绑定当前会话到一个 team，并把尚未加入的成员（总结成员除外）一次性加进来: /team use <team-id>", needsSvc: true, needsConv: true, run: cmdTeam},
```

#### 块 B：`internal/turn`（四处生产补丁、三处授权的 L5b 改动、两处授权的注释（v1.1 加 B8、B9）；只依赖 HEAD）

**B1. `internal/turn/killtree_windows.go`：import 加 `path/filepath` 与 `golang.org/x/sys/windows`**（HEAD `:5-9`）

```anchor internal/turn/killtree_windows.go
import (
	"os/exec"
	"strconv"
	"syscall"
)
```

```replace
import (
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)
```

**B2. `internal/turn/killtree_windows.go`：接缝、`systemTaskkillPath`、`killProcessTree`**（HEAD `:17-28`）

```anchor internal/turn/killtree_windows.go
func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	// /T 杀树，/F 强制。Windows 上 exec.CommandContext 杀掉的往往只是
	// claude.cmd / grok.cmd 那层垫片，真正干活的 node.exe 是它的子进程，
	// 会活着把这一轮写进 CLI 自己的会话文件（09 G 节第 7 行）。
	// 尽力而为：taskkill 失败不把 Spawn 改判成功，Runner 仍置 MaybeAdvanced。
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
```

```replace
// taskkillPath 返回 killProcessTree 要起的 taskkill.exe 的完整路径（CL1，19d §7 (d)）。
// 包级变量只为测试：换成返回错误的函数，模拟找不到 taskkill（oby 的 L5b）；换成指向
// 假文件的函数，证明 killProcessTree 起的就是它返回的那个路径（clx 的正对照）。
var taskkillPath = systemTaskkillPath

// systemTaskkillPath 是系统目录（GetSystemDirectory，一般是 C:\Windows\System32）下的
// taskkill.exe。不按 PATH 找：PATH 里排在前面的目录（或当前目录）有一个叫 taskkill 的
// .exe、.cmd、.bat，按名字起就会执行它，而它拿到的是要杀的 pid。也不读 SystemRoot：
// 系统目录由系统调用给出，不随环境变量变，没有「SystemRoot 为空」这一种情形。系统
// 调用失败时返回错误，killProcessTree 照样不起任何程序，绝不退回按 PATH 找。
func systemTaskkillPath() (string, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "taskkill.exe"), nil
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	// /T 杀树，/F 强制。Windows 上 exec.CommandContext 杀掉的往往只是
	// claude.cmd / grok.cmd 那层垫片，真正干活的 node.exe 是它的子进程，
	// 会活着把这一轮写进 CLI 自己的会话文件（09 G 节第 7 行）。
	// 尽力而为：taskkill 失败不把 Spawn 改判成功，Runner 仍置 MaybeAdvanced。
	// 取不到 taskkill 的路径时直接返回错误、不起任何程序；三处调用方（spawn.go、
	// discovery_exec.go、loginstatus_exec.go）都不看返回值，接着调 cmd.Process.Kill。
	path, err := taskkillPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(path, "/T", "/F", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
```

**B3. `internal/turn/killtree_other.go`：import 加 `errors`**（HEAD `:5-8`）

```anchor internal/turn/killtree_other.go
import (
	"os/exec"
	"syscall"
)
```

```replace
import (
	"errors"
	"os/exec"
	"syscall"
)
```

**B4. `internal/turn/killtree_other.go`：非 Windows 的同名变量**（HEAD `:17`，D7）

```anchor internal/turn/killtree_other.go
func killProcessTree(pid int) error {
```

```replace
// taskkillPath 只在 Windows 上有用（killtree_windows.go 的 killProcessTree 调它）。这里留一个
// 同名变量，只为让没有构建约束的测试文件（oby_loginstatus_exec_test.go 的 L5b 换掉它）在别的
// 平台也编译得过；下面的 killProcessTree 不调它。
var taskkillPath = func() (string, error) {
	return "", errors.New("taskkill is windows-only")
}

func killProcessTree(pid int) error {
```

**B5. `internal/turn/oby_loginstatus_exec_test.go`：import 去掉 `os/exec`（L5b 改完之后不再用它）**（HEAD `:6-8`）

```anchor internal/turn/oby_loginstatus_exec_test.go
	"os"
	"os/exec"
	"path/filepath"
```

```replace
	"os"
	"path/filepath"
```

**B6. `internal/turn/oby_loginstatus_exec_test.go`：文件头说 PATH 的那一句**（HEAD `:18`）

```anchor internal/turn/oby_loginstatus_exec_test.go
// 假 CLI 复用 obx_loginstatus_exec_test.go 的 obxStatusFake、obxLoopLines（只读复用）。PATH 只有 L5b 改（t.Setenv 成一个空目录，模拟找不到 taskkill），其余不改。
```

```replace
// 假 CLI 复用 obx_loginstatus_exec_test.go 的 obxStatusFake、obxLoopLines（只读复用）。PATH 一律不改；L5b 换掉包级接缝
// taskkillPath（CL1 起 killProcessTree 只起系统目录里的 taskkill.exe，不再按 PATH 找），模拟找不到 taskkill。
```

**B7. `internal/turn/oby_loginstatus_exec_test.go`：L5b 换接缝**（HEAD `:47-55`）

```anchor internal/turn/oby_loginstatus_exec_test.go
// TestObyRunLoginStatusKilledWithoutTaskkill 是 L5b（R0）：同 L5，但 PATH 只有一个空目录，
// killProcessTree 找不到 taskkill（契约 19d §0 第 9 条），只剩 cmd.Process.Kill 那一行在杀。
// 前提：这个 PATH 下 exec.LookPath("taskkill") 必须失败。
// 【判别力】N30、N32（只删 Kill 那一行：L5 里 taskkill 还在，看不出来）。
func TestObyRunLoginStatusKilledWithoutTaskkill(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("taskkill"); err == nil {
		t.Fatalf("前提不成立：PATH 只有空目录时仍找得到 taskkill")
	}
```

```replace
// TestObyRunLoginStatusKilledWithoutTaskkill 是 L5b（R0）：同 L5，但 taskkillPath 换成返回错误的函数，
// killProcessTree 取不到 taskkill、什么都不起（CL1 改法，原来是把 PATH 设成空目录），只剩 cmd.Process.Kill 那一行在杀。
// 前提：换掉之后 taskkillPath() 必须返回错误。本包的测试没有 t.Parallel，换包级变量不会与别的用例并发。
// 【判别力】N30、N32（只删 Kill 那一行：L5 里 taskkill 还在，看不出来）。
func TestObyRunLoginStatusKilledWithoutTaskkill(t *testing.T) {
	old := taskkillPath
	t.Cleanup(func() { taskkillPath = old })
	taskkillPath = func() (string, error) { return "", errors.New("oby: no taskkill") }
	if _, err := taskkillPath(); err == nil {
		t.Fatalf("前提不成立：换掉接缝之后 taskkillPath() 仍然没有返回错误")
	}
```

**B8. `internal/turn/oby_loginstatus_exec_test.go`：L5 的【判别力】里说 taskkill 在 PATH 上的那一句**（v1.1，主代理裁决 (g)，只改注释；HEAD `:23`，在 B6 与 B7 之间）

```anchor internal/turn/oby_loginstatus_exec_test.go
// 只删 Kill 那一行（N32）在这里看不出来（taskkill 在 PATH 上），由 L5b 杀。
```

```replace
// 只删 Kill 那一行（N32）在这里看不出来（系统目录里的 taskkill 照样杀树），由 L5b 杀。
```

**B9. `internal/turn/obx_loginstatus_exec_test.go`：文件头说 PATH 不改的理由那一句**（v1.1，主代理裁决 (g)，只改注释；HEAD `:17`）

```anchor internal/turn/obx_loginstatus_exec_test.go
// 内建命令的 .cmd；PATH 不改（超时杀树要 System32 里的 taskkill）。
```

```replace
// 内建命令的 .cmd；PATH 不改（CL1 起超时杀树只起系统目录里的 taskkill.exe，与 PATH 无关）。
```

### 1.2 不做

- 不改守卫提示让重敲的命令换档的问题（19g 头第三条，D12 与 V20 有意冻结；以后与 +编号 一起改成回显完整的原命令）。
- 不改 `msgFirstRunNoneFound`，也不把它与 `loginCommands` 连起来（19f §7 (j)）；A6 只把注释写准。
- 不修当前目录同名文件遮住 PATH 上的安装（19d §7 (k)，`config/firstrun.go` 的检测）。
- 不碰 codex 失败轮的 `usage_raw`（#46）；不改「三家」之类的计数说法。
- 不改 `internal/tui` 的生产代码；不改 `loginstatus_exec.go`、`discovery_exec.go`、`spawn.go`（三处调用方）、`internal/app`、`internal/config`、`internal/provider`、`internal/store`、`internal/conversation`、`cmd/council` 的任何文件；`internal/repl` 只改 `commands.go`、`addname.go`、`addlogin.go`；`internal/turn` 只改 `killtree_windows.go`、`killtree_other.go`。
- 命令表里只改 `add`（A1）与 `team`（A9，v1.1）两行的 summary；别的命令行不动，`join` 的「同 /add」不动。
- 既有 `_test.go` 只改任务书与主代理裁决授权的那几处：`cxh_fixture_test.go:223`（A7）、`ux2r_add_test.go:448`（A8）、`oby_loginstatus_exec_test.go` 的 L5b（B5-B7：L5b 函数头注释与开头几行、它不再用的 `os/exec` import、文件头说 L5b 改 PATH 的那一句）；v1.1 加两处只改注释的：oby 里 L5 的【判别力】「taskkill 在 PATH 上」那一句（B8）、`obx_loginstatus_exec_test.go` 文件头「PATH 不改（超时杀树要 System32 里的 taskkill）」那一句（B9）。

---

## 2. 验收（测试腿新写的文件）

三个新文件：`internal/repl/clx_cl1_test.go`（S1，`package repl`）、`internal/tui/clx_ctrlaltc_test.go`（S2，`package tui`）、`internal/turn/clx_killtree_windows_test.go`（S3，`package turn`，只在 Windows 编译）。包级标识符一律 `clx` 开头；**不许新增 `TestMain`**；不许改任何既有测试文件（既有测试的授权改动是 §1.1 的 A7、A8、B5-B9，由实现腿交）。三个文件照下面逐字写（§4 按名字点杀手）。用例编号：C1、C2、C9 在 repl，C3 在 tui，C4-C8 在 turn（C8、C9 是 v1.1 加的）。

纪律：
- 字面量整行相等：repl 的屏幕输出按行 `==` 比较（`mdxWantLines`）；tui 的行、值、表头逐字相等（atx 的辅助）；turn 的假 taskkill 收到的参数整串相等。
- 每条都能为假：每个测试写明它对哪条变异红（【判别力】，§4 的「杀手」列是原型实测）。缺席断言配同一装置、只差一处的正对照：C1 的「不识别、不写」配同一装置接着带值的那一条（识别、写了）；C2 的 `not_member` 配只差 `[summary]` 的 `no_summary`（照旧 Join）、`present` 与没配 `[summary]` 的同一装置逐行比；C3 每种键的第 2 步配同一种键的第 3 步（列表不在时同一个键就是退出）；C4、C6 的「假的没被执行」配 C5 第 1 步（接缝指向同一个假文件时标记出现）；C5 第 2 步的不在 PATH 上的假文件由同一个辅助 `clxWriteFakeCmd` 写出、脚本相同；C8 的「假 taskkill.exe 没被执行」配同一装置接着把接缝指向它（标记出现）；C4、C8 另配「循环真被杀掉」（等它退出，最多 10 s）。
- repl 的用例不起任何进程：C1 的识别用 `adnNewRig` 装好的假执行器，C2、C9 不识别、不起进程（`New` 直接造 REPL，`joinTrackingStore` 记账）。
- tui 的用例不执行任何 Cmd（19c §2.0 的三种用法）。
- turn 的用例照 19e L5-L6 的做法：假 CLI 是 `t.TempDir()` 里只用 cmd 内建命令的 `.cmd`（`obxStatusFake`、`obxLoopLines` 只读复用；假 taskkill 自己写）；PATH 只用 `t.Setenv` 改；真 taskkill 只对测试自己起的 cmd 循环的 pid 起；每条用例结束时 `t.Cleanup` 删掉循环的开关文件、`Kill`、等它退出，不留残留 `cmd.exe`。
- C8 的假 `taskkill.exe` 是例外：用 `go build` 现编（源码只用标准库，只把参数写进同目录的标记文件）。`go` 按 PATH 找，`go test` 起测试二进制时把 GOROOT/bin 排在 PATH 最前（原型实跑：PATH 只留 `C:\Windows\System32`、用全路径起 `go.exe test`，C8 照绿）。编译的 `GOCACHE`、`GOTMPDIR` 与输出目录都在本用例的 `t.TempDir()` 里，另设 `GOENV=off`、`GOFLAGS` 为空、`GOWORK=off`、`GOPROXY=off`、`GOTOOLCHAIN=local`；同一份源码在隔离目录里用空缓存单独编了两次，2.9 s 与 3.6 s（实测）；C8 整条 4.07-4.59 s（原型实跑三次）。
- C4、C8 各有一条前提断言：PATH 最前放了假 taskkill 之后，`exec.LookPath("taskkill")` 必须解析到它（否则「标记不存在」可能只是按名字也找不到它，判前提不成立）。C5 第 2 步反过来断言 `exec.LookPath("clx-fake-tk")` 找不到（它的目录不在 PATH 上）。

### 2.1 `internal/repl/clx_cl1_test.go`（S1）

- **C1 `TestClxAddIDMissingValue`**（D2）：`/add gpt-6.1-sol --id`、`/add --id` 各打 `adnUsage`、不写、不变；执行器 0 次；正对照 `/add gpt-6.1-sol --id sol` 识别、新建 `sol`、写配置并加入。
- **C2 `TestClxTeamUseSummaryMember`**（D5）：`newTeamTrioConfig`（`tm_trio` = `ag_alpha`、`ag_mid`、`ag_zeta`），`ag_alpha`（`PermissionRead`，真配置也过得了校验）当总结成员。四个子测试：`not_member`（总结成员还不是成员：打 §3 第 2 句、不 Join 它、另两个照旧加入）、`no_summary`（同一装置只差没配 `[summary]`：`ag_alpha` 照旧加入）、`present`（总结成员已在场：与没配 `[summary]` 的同一装置逐行相同）、`left`（总结成员已退出：照旧打「已退出，未自动拉回」、不拉回）。
- **C9 `TestClxHelpTeamLine`**（v1.1，D5 末条）：C2 的装置（没配 `[summary]`）敲 `/help`，输出里有一行与 `clxHelpTeamLine`（`/team` 补到 12 个字符再接 §3 第 3 句）整行相等。

```file internal/repl/clx_cl1_test.go
package repl

// clx_cl1_test.go 是 CL1（一批小修补，契约 §2.1）在 repl 包的验收：
//   - C1：/add <名字> --id 缺值时打用法（AD1 的 M9，19g 文件头），不识别、不写配置；
//   - C2：/team use 不把总结成员（[summary] agent）当普通成员加入（#47 在 /team use 漏了）；
//   - C9：/help 里 team 那一行说明总结成员除外（v1.1）。
// 只读复用 adn_addname_test.go（adnNewRig、adnRefused、adnAdded、adnNoDiscovery、adnUsage、adnBlockSol、
// adnDiscovering）、mdx_addmodel_fixture_test.go（mdxLines、mdxWantLines、mdxHasLine）、team_use_test.go（newTeamTrioConfig、
// newJoinTrackingStore、joinTrackingStore）与 conversation_test.go 的 fakeStore；不改它们。
// repl 的用例不起任何进程：C1 的识别用 mdxExec（adnNewRig 装好的假执行器），C2、C9 不识别。

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
)

// TestClxAddIDMissingValue 是 C1（19g 文件头的 M9）：--id 是最后一个词、后面没有值时打按名字加的用法那一句，
// 执行器 0 次、配置文件字节与备份个数都不变。同一装置接着敲 --id 带值的同一条命令是正对照：识别两家、新建 sol、
// 写配置并加入（证明这个装置下按名字加是通的，「不识别、不写」来自守卫）。
// 【判别力】V2（--id 那一半去掉 i+1 >= len(fields)）：第一条命令在 parseAddNameArgs 里越界 panic，mdxRig.runCtx
// 接住 panic、Fatalf 判红（「dispatch("/add gpt-6.1-sol --id") panic: runtime error: index out of range ...」），
// 后面几条不再跑。--effort 那一半（V3）的杀手是 TestAdnFlags 的 /add gpt-6.1-sol --effort。
func TestClxAddIDMissingValue(t *testing.T) {
	g := adnNewRig(t, true)
	for _, cmd := range []string{"/add gpt-6.1-sol --id", "/add --id"} {
		adnRefused(t, g, cmd, []string{adnUsage})
	}
	adnNoDiscovery(t, g, "--id 缺值的两条")
	adnAdded(t, g, "/add gpt-6.1-sol --id sol", "sol", adnBlockSol, true)
}

// clxSumLine 是 /team use 跳过总结成员时打的那一行（契约 §3，逐字）。
func clxSumLine(id string) string {
	return id + " 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结"
}

// clxTeamConv 是 C2 各子测试的会话 id（直接塞进 fakeStore，再 /use 它）。
const clxTeamConv = "convclx01"

// clxTeamRig 是 C2 的装置：newTeamTrioConfig（tm_trio = ag_alpha、ag_mid、ag_zeta）加上 summary 指定的总结成员
// （空串是没配），joinTrackingStore 记每一次 Join；seed 里的成员先以在场塞进会话，left 里的再标成已退出。
// 不经 Run：直接调 dispatch，按行取每条命令的输出增量（与 mdxRig.run 同一种取法）。
type clxTeamRig struct {
	t     *testing.T
	ctx   context.Context
	store *joinTrackingStore
	r     *REPL
	out   *bytes.Buffer
}

func clxNewTeamRig(t *testing.T, summary string, seed, left []string) *clxTeamRig {
	t.Helper()
	cfg := newTeamTrioConfig()
	cfg.Summary = config.SummaryConfig{Agent: summary}
	store := newJoinTrackingStore()
	ctx := context.Background()
	store.fakeStore.seedConversation(clxTeamConv, "CL1 team 与总结成员")
	for _, id := range seed {
		if err := store.fakeStore.AddParticipant(ctx, conversation.Participant{
			ConvID: clxTeamConv, AgentID: id, Provider: "fake", JoinedAt: replTestNow, Active: true,
		}); err != nil {
			t.Fatalf("装置：塞入 %s 失败: %v", id, err)
		}
	}
	for _, id := range left {
		if err := store.fakeStore.DeactivateParticipant(ctx, clxTeamConv, id, replTestNow.Add(1)); err != nil {
			t.Fatalf("装置：让 %s 退出失败: %v", id, err)
		}
	}
	out := &bytes.Buffer{}
	r := New(cfg, conversation.NewService(store), nil, strings.NewReader(""), out, RulesContext{})
	g := &clxTeamRig{t: t, ctx: ctx, store: store, r: r, out: out}
	g.run("/use " + clxTeamConv)
	return g
}

// run 喂一行，返回这一行的输出增量（按行）；dispatch 的 panic 与错误都判红。
func (g *clxTeamRig) run(line string) []string {
	g.t.Helper()
	before := g.out.Len()
	var panicked any
	var err error
	func() {
		defer func() { panicked = recover() }()
		err = g.r.dispatch(g.ctx, line)
	}()
	if panicked != nil {
		g.t.Fatalf("dispatch(%q) panic: %v", line, panicked)
	}
	if err != nil {
		g.t.Fatalf("dispatch(%q) 返回错误: %v", line, err)
	}
	return mdxLines(g.out.String()[before:])
}

// joins 是 /team use 期间每个成员各被 Join 了几次（只列次数非 0 的）。
func (g *clxTeamRig) joins() map[string]int {
	got := map[string]int{}
	for _, c := range g.store.joinCalls {
		if c.convID == clxTeamConv {
			got[c.agentID]++
		}
	}
	return got
}

// active 是会话里在场成员的 id（排序）。
func (g *clxTeamRig) active() []string {
	g.t.Helper()
	ps, err := g.store.Participants(g.ctx, clxTeamConv, true)
	if err != nil {
		g.t.Fatalf("Participants: %v", err)
	}
	var out []string
	for _, p := range ps {
		if p.Active {
			out = append(out, p.AgentID)
		}
	}
	sort.Strings(out)
	return out
}

// TestClxTeamUseSummaryMember 是 C2：tm_trio 的 members 里有总结成员 ag_alpha（PermissionRead，真配置也过得了校验）。
//   - not_member：ag_alpha 还不是本会话的成员。打 clxSumLine、不 Join 它；ag_mid、ag_zeta 照旧加入。
//   - no_summary：同一装置只差 [summary] 没配（正对照）：ag_alpha 照旧加入。证明「不 Join」来自总结成员这一条。
//   - present：ag_alpha 已在场（总结成员加入过）：输出与没配 [summary] 的同一装置逐行相同，不打 clxSumLine。
//   - left：ag_alpha 已退出：照旧打「已退出，未自动拉回」，不打 clxSumLine，不拉回。
//
// 【判别力】not_member 对 V4（去掉总结成员这一条：Join 了 ag_alpha）、V6（打了说明却不 continue：说明之后照样 Join）
// 红；present、left 对 V5（这一条挪到「已是成员」那一支之前：在场、已退出的总结成员也打 clxSumLine）红；V7（判据换成
// 「配了总结成员就跳过所有缺的成员」：ag_mid、ag_zeta 也打 clxSumLine、没加入）下 not_member、present、left 三条都红。
// no_summary 是 not_member 的对照，在这几条变异下都绿（它没配 [summary]）。
func TestClxTeamUseSummaryMember(t *testing.T) {
	const cmd = "/team use tm_trio"
	bound := "已绑定 team tm_trio"
	joined := func(id string) string { return id + " 已加入会话 " + clxTeamConv }

	t.Run("not_member", func(t *testing.T) {
		g := clxNewTeamRig(t, "ag_alpha", nil, nil)
		mdxWantLines(t, cmd, g.run(cmd), []string{bound, clxSumLine("ag_alpha"), joined("ag_mid"), joined("ag_zeta")})
		if got, want := g.joins(), map[string]int{"ag_mid": 1, "ag_zeta": 1}; !reflect.DeepEqual(got, want) {
			t.Errorf("Join 次数应当是 %v（总结成员 ag_alpha 一次都不 Join），实际 %v", want, got)
		}
		if got, want := g.active(), []string{"ag_mid", "ag_zeta"}; !reflect.DeepEqual(got, want) {
			t.Errorf("在场成员应当是 %q，实际 %q", want, got)
		}
	})

	t.Run("no_summary", func(t *testing.T) {
		g := clxNewTeamRig(t, "", nil, nil)
		mdxWantLines(t, cmd, g.run(cmd), []string{bound, joined("ag_alpha"), joined("ag_mid"), joined("ag_zeta")})
		if got, want := g.joins(), map[string]int{"ag_alpha": 1, "ag_mid": 1, "ag_zeta": 1}; !reflect.DeepEqual(got, want) {
			t.Errorf("没配 [summary] 时 Join 次数应当是 %v，实际 %v", want, got)
		}
	})

	t.Run("present", func(t *testing.T) {
		g := clxNewTeamRig(t, "ag_alpha", []string{"ag_alpha"}, nil)
		got := g.run(cmd)
		c := clxNewTeamRig(t, "", []string{"ag_alpha"}, nil)
		want := c.run(cmd)
		mdxWantLines(t, cmd+"（总结成员已在场）", got, []string{bound, joined("ag_mid"), joined("ag_zeta")})
		mdxWantLines(t, cmd+"（与没配 [summary] 的同一装置比）", got, want)
		if n := g.joins()["ag_alpha"]; n != 0 {
			t.Errorf("已在场的 ag_alpha 不该再 Join，实际 %d 次", n)
		}
	})

	t.Run("left", func(t *testing.T) {
		g := clxNewTeamRig(t, "ag_alpha", []string{"ag_alpha"}, []string{"ag_alpha"})
		mdxWantLines(t, cmd, g.run(cmd), []string{bound, "ag_alpha 已退出，未自动拉回，要加回用 /join", joined("ag_mid"), joined("ag_zeta")})
		if n := g.joins()["ag_alpha"]; n != 0 {
			t.Errorf("已退出的 ag_alpha 不该被拉回，实际 Join %d 次", n)
		}
		if got, want := g.active(), []string{"ag_mid", "ag_zeta"}; !reflect.DeepEqual(got, want) {
			t.Errorf("在场成员应当是 %q，实际 %q", want, got)
		}
	})
}

// clxHelpTeamLine 是 /help 输出里 team 那一整行（COMMAND 列按 tabwriter 补到 12 个字符；契约 §3 第 3 句）。
const clxHelpTeamLine = "/team       绑定当前会话到一个 team，并把尚未加入的成员（总结成员除外）一次性加进来: /team use <team-id>"

// TestClxHelpTeamLine 是 C9（v1.1）：/help 里 team 那一行说明总结成员不随 team 加入（C2 的行为在帮助里的说法）。
// 用 C2 的装置（没配 [summary]）敲 /help，按整行找。
// 【判别力】V17（team 的 summary 换回 HEAD 的旧句，不提总结成员）：没有与 clxHelpTeamLine 整行相等的一行，红。
func TestClxHelpTeamLine(t *testing.T) {
	g := clxNewTeamRig(t, "", nil, nil)
	lines := g.run("/help")
	if !mdxHasLine(lines, clxHelpTeamLine) {
		t.Errorf("/help 输出里没有与 clxHelpTeamLine 整行相等的一行：\n%s", strings.Join(lines, "\n"))
	}
}
```

### 2.2 `internal/tui/clx_ctrlaltc_test.go`（S2）

- **C3 `TestClxCtrlCModsClosesList`**（D9；v1.1 由 `TestClxCtrlAltCClosesList` 改成三种键的小表）：子测试 `alt`、`numlock`、`capslock`，键分别是 `Code: 'c'` 加 `tea.ModCtrl | tea.ModAlt`、`tea.ModCtrl | tea.ModNumLock`、`tea.ModCtrl | tea.ModCapsLock`。每个子测试：前提（键的 `String()` 依次是 `ctrl+alt+c`、`ctrl+c`、`ctrl+c`）；列表开着时按下：Cmd nil、行 nil、值 `@b`、表头「空闲」；对照（另起 Model，列表不在）：Cmd 是 `tea.Quit`（只比函数指针）、表头「正在退出」、值 `@b`。

```file internal/tui/clx_ctrlaltc_test.go
package tui

// clx_ctrlaltc_test.go 是 CL1（一批小修补，契约 §2.2）第 7 条在 tui 包的验收：@ 补全列表开着时按 Ctrl+Alt+C
// 只收起列表，不退出、不提交（19c 文件头：列表分支 Ctrl-C 判据的 Alt 那一半没有用例）；v1.1 起同样核 Ctrl 加
// NumLock、CapsLock（Windows 控制台把锁定键的状态放进 Mod）。纯 Model 驱动，不起 Program。
// 只读复用 atx_model_test.go（atxNew、atxCands、atxMembers、atxB3、atxH、atxWant*）与 tui1b_model_test.go
// （tui1bType、tui1bStep、tui1bEsc），不改它们。
//
// Cmd 纪律（同 19c §2.0）：按键之后拿到的 Cmd 一律不执行，只有三种用法：与 nil 比较；作为 reflect.ValueOf 的唯一
// 实参并紧跟 .Pointer()；传给 atxWantNilCmd。失败消息里只报「cmd == nil」这个布尔值。

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// clxCtrlCKeys 是 C3 的三种键：Ctrl+C 再加 Alt、NumLock、CapsLock 之一。str 是键的 String()：锁定键不进 String()，
// 所以后两种都是 ctrl+c。
var clxCtrlCKeys = []struct {
	name string
	key  tea.KeyPressMsg
	str  string
}{
	{"alt", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModAlt}, "ctrl+alt+c"},
	{"numlock", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModNumLock}, "ctrl+c"},
	{"capslock", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModCapsLock}, "ctrl+c"},
}

// TestClxCtrlCModsClosesList 是 C3：列表开着时 Ctrl+C 带上 Alt、NumLock 或 CapsLock，只收起列表，Cmd 为 nil、值不变、
// 表头仍是「空闲」（不退出、不取消、不提交）。每种键各起新 Model。第 3 步是同装置对照（只差列表开没开）：列表不在时
// 同一个键交回 tea.Quit、表头是「正在退出」，证明这个键确实到了 Council 的 Ctrl-C 判据，第 2 步的 nil 不是「键没到」。
// 【判别力】V15（列表分支的判据写成只放过 Shift：msg.Mod&^tea.ModShift == tea.ModCtrl，19c 文件头审查员的那条）与
// V16（A60，收窄成 msg.Mod == tea.ModCtrl）下，三种键都掉进空闲的 Ctrl-C 分支：第 2 步 Cmd 非 nil（退出）、行仍是 atxB3、
// 表头变成「正在退出」，三处红。X4（只放过 Shift 与 Alt：msg.Mod&^(tea.ModShift|tea.ModAlt) == tea.ModCtrl）下
// numlock、capslock 两种红，alt 照绿。V15 下 TestAtfCtrlShiftCClosesList 照绿（这正是 19c 头说的缺口）。
func TestClxCtrlCModsClosesList(t *testing.T) {
	for _, k := range clxCtrlCKeys {
		t.Run(k.name, func(t *testing.T) {
			// 第 1 步（前提）：键的写法。
			if s := k.key.String(); s != k.str {
				t.Fatalf("第 1 步（前提）：键的 String() 应当是 %q，实际 %q", k.str, s)
			}

			// 第 2 步：列表开着。
			m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
			m = tui1bType(t, m, "@b")
			atxWantLines(t, "第 2 步（前提）：敲 @b", m, atxB3)
			atxWantHead(t, "第 2 步（前提）：敲 @b", m, atxH("空闲"))
			m, cmd := tui1bStep(t, m, k.key)
			atxWantNilCmd(t, "第 2 步：列表开着时按下", cmd)
			atxWantLines(t, "第 2 步：列表开着时按下", m, nil)
			atxWantValue(t, "第 2 步：列表开着时按下", m, "@b")
			atxWantHead(t, "第 2 步：列表开着时按下", m, atxH("空闲"))

			// 第 3 步：对照（列表不在）。另起新 Model。
			m3, _ := atxNew(t, 80, 24, atxCands(atxMembers))
			m3 = tui1bType(t, m3, "@b")
			m3, _ = tui1bStep(t, m3, tui1bEsc())
			atxWantLines(t, "第 3 步（前提）：敲 @b 再按 Esc", m3, nil)
			m3, cmd3 := tui1bStep(t, m3, k.key)
			if reflect.ValueOf(cmd3).Pointer() != reflect.ValueOf(tea.Quit).Pointer() {
				t.Errorf("第 3 步：列表不在时按下，Cmd 应当是 tea.Quit（只比函数指针，不执行），实际不是（cmd == nil：%v）", cmd3 == nil)
			}
			atxWantHead(t, "第 3 步：列表不在时按下", m3, atxH("正在退出"))
			atxWantValue(t, "第 3 步：列表不在时按下", m3, "@b")
		})
	}
}
```

### 2.3 `internal/turn/clx_killtree_windows_test.go`（S3）

- **C4 `TestClxKillTreeIgnoresPathTaskkill`**（D6）：假 `taskkill.cmd` 排在 PATH 最前，`killProcessTree(pid)` 返回 nil、循环 10 s 内退出（真 taskkill 杀掉了它）、标记文件不存在。
- **C5 `TestClxKillTreeUsesSeam`**：同一装置。第 1 步接缝指向 PATH 上的那个假 `taskkill.cmd`：返回 nil、标记文件内容是 `/T /F /PID <pid>\r\n`（C4、C6 的正对照）。第 2 步（v1.1，主代理裁决 m1）接缝指向另一个 `t.TempDir()` 里、名字不叫 taskkill 的假 `clx-fake-tk.cmd`（这个目录不进 PATH；前提：`exec.LookPath("clx-fake-tk")` 找不到）：返回 nil、它自己目录里的标记文件内容同样是 `/T /F /PID <pid>\r\n`。
- **C6 `TestClxKillTreeSeamErrorNoFallback`**：同一装置，接缝返回错误：`killProcessTree` 返回的就是这个错误（`errors.Is`）、标记文件不存在。
- **C7 `TestClxSystemTaskkillPath`**：前提 `SystemRoot` 非空；PATH 最前有假文件时 `systemTaskkillPath()` 仍等于 `%SystemRoot%\System32\taskkill.exe`（不分大小写）、不是假文件、是存在的文件；接缝的默认值就是 `systemTaskkillPath`（比函数指针）。末步（v1.1，主代理裁决 m2）：记下结果，`t.Setenv("SystemRoot", `Z:\clx-nope`)`，再调一次，结果不变、不报错。
- **C8 `TestClxKillTreeIgnoresPathTaskkillExe`**（v1.1，主代理裁决 m1 的第二条）：用 Go 现编一个假 `taskkill.exe`（只把参数以空格连接写进同目录的 `clx-marker.txt`），它的目录排在 PATH 最前（前提：`exec.LookPath("taskkill")` 解析到它）；`killProcessTree(pid)` 返回 nil、循环 10 s 内退出、标记文件不存在。正对照（同一装置）：接缝指向这个 exe，返回 nil、标记文件内容是 `/T /F /PID <pid>`（没有换行）。

```file internal/turn/clx_killtree_windows_test.go
//go:build windows

package turn

// clx_killtree_windows_test.go 是 CL1（一批小修补，契约 §2.3）第 6 条在 turn 包的验收：killProcessTree 只起
// 系统目录里的 taskkill.exe，不按 PATH 找（19d §7 (d)）；取不到路径时返回错误、什么都不起，绝不退回按 PATH 找。
// 装置：t.TempDir() 里一个假的 taskkill.cmd（只把收到的参数写进同目录的标记文件、以 0 退出，不杀任何进程），
// 它的目录排在 PATH 最前；C8 另用 Go 现编一个假的 taskkill.exe（同样只写标记文件）排在 PATH 最前。被杀的是测试
// 自己起的一个 cmd 循环（obx_loginstatus_exec_test.go 的 obxStatusFake、obxLoopLines，只读复用）。真 taskkill
// 只会对这个循环的 pid 起。每条用例结束时（t.Cleanup）删掉循环的开关文件、Kill 并等它退出，不留残留 cmd.exe。
// 本包的测试没有 t.Parallel，换包级变量 taskkillPath 不会与别的用例并发。

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// clxKillWait 是等循环退出的上限：真 taskkill 杀掉它通常不到 1 s。
const clxKillWait = 10 * time.Second

// clxFake 是一套装置：假 taskkill 的路径与标记文件、被杀的循环与它的退出信号。
type clxFake struct {
	path   string
	marker string
	pid    int
	done   chan struct{} // 循环退出（cmd.Wait 返回）时关闭
}

// clxWriteFakeCmd 在 dir 里写一个名叫 name 的假 taskkill（.cmd，只把收到的参数写进同目录的 clx-marker.txt、
// 以 0 退出），返回它的路径与标记文件的路径。
func clxWriteFakeCmd(t *testing.T, dir, name string) (path, marker string) {
	t.Helper()
	path, marker = filepath.Join(dir, name), filepath.Join(dir, "clx-marker.txt")
	script := strings.Join([]string{"@echo off", `>"%~dp0clx-marker.txt" echo %*`, "exit /b 0"}, "\r\n") + "\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatalf("写假 taskkill %s: %v", name, err)
	}
	return path, marker
}

// clxPrependPath 把 dir 排在 PATH 最前，再核前提：此时 exec.LookPath("taskkill") 解析到的就是 want
// （按名字找就会执行它），否则判前提不成立。
func clxPrependPath(t *testing.T, dir, want string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got, err := exec.LookPath("taskkill"); err != nil || !strings.EqualFold(got, want) {
		t.Fatalf("前提不成立：PATH 最前放了假的 taskkill，exec.LookPath(\"taskkill\") 应当解析到 %s，实际 %q（err %v）", want, got, err)
	}
}

// clxStartLoop 起一个会一直等的 cmd 循环，返回它的 pid 与退出信号；用例结束时删开关文件、Kill 并等它退出。
func clxStartLoop(t *testing.T) (int, chan struct{}) {
	t.Helper()
	on := filepath.Join(t.TempDir(), "loop.on")
	if err := os.WriteFile(on, []byte("x"), 0o644); err != nil {
		t.Fatalf("写 loop.on: %v", err)
	}
	p := obxStatusFake(t, obxLoopLines(`exist "`+on+`"`)...)
	cmd := exec.Command(p)
	if err := cmd.Start(); err != nil {
		t.Fatalf("起循环: %v", err)
	}
	pid := cmd.Process.Pid
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = os.Remove(on)
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(clxKillWait):
			t.Errorf("清理：循环（pid %d）在 %v 内没有退出", pid, clxKillWait)
		}
	})
	return pid, done
}

// clxNewFake 写假的 taskkill.cmd、把它的目录排在 PATH 最前（核 LookPath 的前提），再起一个会一直等的 cmd 循环。
func clxNewFake(t *testing.T) *clxFake {
	t.Helper()
	dir := t.TempDir()
	f := &clxFake{}
	f.path, f.marker = clxWriteFakeCmd(t, dir, "taskkill.cmd")
	clxPrependPath(t, dir, f.path)
	f.pid, f.done = clxStartLoop(t)
	return f
}

// clxMarkerExists 报告标记文件 marker 在不在，即写它的假 taskkill 有没有被执行过。
func clxMarkerExists(t *testing.T, marker string) bool {
	t.Helper()
	_, err := os.Stat(marker)
	if err == nil {
		return true
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("查标记文件: %v", err)
	}
	return false
}

// clxSetSeam 把 taskkillPath 换成 fn，用例结束时还原。
func clxSetSeam(t *testing.T, fn func() (string, error)) {
	t.Helper()
	old := taskkillPath
	t.Cleanup(func() { taskkillPath = old })
	taskkillPath = fn
}

// clxWaitDone 等循环退出，最多 clxKillWait。
func clxWaitDone(t *testing.T, pid int, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(clxKillWait):
		t.Errorf("该杀的循环（pid %d）在 %v 内没有退出：killProcessTree 没有起真的 taskkill", pid, clxKillWait)
	}
}

// TestClxKillTreeIgnoresPathTaskkill 是 C4：假的 taskkill.cmd 排在 PATH 最前，killProcessTree 不执行它，
// 而且该杀的循环真被杀掉了（起的是系统目录里的 taskkill.exe）。
// 【判别力】V8（按名字起 taskkill，HEAD 的写法）：执行了假的，标记出现、循环没死，两处红；V9（systemTaskkillPath
// 按 PATH 找）同样两处红；V12（接缝的默认值换成按 PATH 找）同样。同装置的正对照是 TestClxKillTreeUsesSeam 第 1 步：
// 接缝指向这个假文件时标记出现，证明标记机制有效、「标记不存在」不是假文件没法执行。
func TestClxKillTreeIgnoresPathTaskkill(t *testing.T) {
	f := clxNewFake(t)
	if err := killProcessTree(f.pid); err != nil {
		t.Errorf("killProcessTree 应当返回 nil（系统目录里的 taskkill 杀掉了循环），实际 %v", err)
	}
	clxWaitDone(t, f.pid, f.done)
	if clxMarkerExists(t, f.marker) {
		t.Errorf("killProcessTree 执行了 PATH 最前的假 taskkill（标记文件 %s 出现了）", f.marker)
	}
}

// TestClxKillTreeUsesSeam 是 C5：同一装置，killProcessTree 起的就是接缝返回的那个路径，参数是 /T /F /PID <pid>。
//   - 第 1 步：接缝指向 PATH 最前的那个假 taskkill.cmd（TestClxKillTreeIgnoresPathTaskkill、
//     TestClxKillTreeSeamErrorNoFallback 的正对照：同一个文件被执行时标记出现）。
//   - 第 2 步：接缝指向另一个 t.TempDir() 里、名字不叫 taskkill 的假 clx-fake-tk.cmd（这个目录不进 PATH），
//     证明起的是完整路径本身，不是去掉目录之后再按名字找。
//
// 【判别力】V11（killProcessTree 不经接缝、直接调 systemTaskkillPath）：起的是真 taskkill，两步的标记都不出现，红；
// X1（按 filepath.Base(path) 起，即去掉目录再按 PATH 找）：第 1 步照绿（名字与目录都在 PATH 上），第 2 步按名字
// 找不到 clx-fake-tk.cmd，返回错误、标记不出现，红。
func TestClxKillTreeUsesSeam(t *testing.T) {
	f := clxNewFake(t)
	want := "/T /F /PID " + strconv.Itoa(f.pid) + "\r\n"

	clxSetSeam(t, func() (string, error) { return f.path, nil })
	if err := killProcessTree(f.pid); err != nil {
		t.Errorf("第 1 步：假 taskkill 以 0 退出，killProcessTree 应当返回 nil，实际 %v", err)
	}
	if b, err := os.ReadFile(f.marker); err != nil {
		t.Errorf("第 1 步：接缝指向 PATH 上的假 taskkill 时它应当被执行、写出标记文件，实际读不到：%v", err)
	} else if got := string(b); got != want {
		t.Errorf("第 1 步：假 taskkill 收到的参数应当是 %q，实际 %q", want, got)
	}

	// 第 2 步另起一个循环：变异下第 1 步可能已经起了真 taskkill、杀掉了 f.pid，这一步若再起真 taskkill，
	// 对象仍是测试自己起的、活着的进程，不会是一个已经退出、可能被系统复用的 pid。
	pid2, _ := clxStartLoop(t)
	want2 := "/T /F /PID " + strconv.Itoa(pid2) + "\r\n"
	off, offMarker := clxWriteFakeCmd(t, t.TempDir(), "clx-fake-tk.cmd")
	if _, err := exec.LookPath("clx-fake-tk"); err == nil {
		t.Fatalf("前提不成立：clx-fake-tk.cmd 的目录不在 PATH 上，按名字却找得到它")
	}
	clxSetSeam(t, func() (string, error) { return off, nil })
	if err := killProcessTree(pid2); err != nil {
		t.Errorf("第 2 步：假 clx-fake-tk.cmd 以 0 退出，killProcessTree 应当返回 nil，实际 %v", err)
	}
	if b, err := os.ReadFile(offMarker); err != nil {
		t.Errorf("第 2 步：接缝指向不在 PATH 上的 %s 时它应当被执行、写出标记文件，实际读不到：%v", off, err)
	} else if got := string(b); got != want2 {
		t.Errorf("第 2 步：假 clx-fake-tk.cmd 收到的参数应当是 %q，实际 %q", want2, got)
	}
}

// TestClxKillTreeSeamErrorNoFallback 是 C6：同一装置，接缝返回错误时 killProcessTree 原样返回这个错误、
// 什么都不起，不退回按 PATH 找（假的 taskkill.cmd 就在 PATH 最前）。
// 【判别力】V10（取不到路径时退回按名字起 taskkill）：执行了假的，标记出现，返回值也不再是那个错误，红；
// V11 下起的是真 taskkill、返回 nil，红。正对照同 TestClxKillTreeUsesSeam 第 1 步（标记机制有效）。
func TestClxKillTreeSeamErrorNoFallback(t *testing.T) {
	f := clxNewFake(t)
	errNo := errors.New("clx: no taskkill")
	clxSetSeam(t, func() (string, error) { return "", errNo })
	if err := killProcessTree(f.pid); !errors.Is(err, errNo) {
		t.Errorf("接缝返回错误时 killProcessTree 应当返回这个错误，实际 %v", err)
	}
	if clxMarkerExists(t, f.marker) {
		t.Errorf("接缝返回错误时 killProcessTree 退回按 PATH 起了假的 taskkill（标记文件 %s 出现了）", f.marker)
	}
}

// TestClxSystemTaskkillPath 是 C7：systemTaskkillPath 给的是 %SystemRoot%\System32\taskkill.exe（本机核对用），
// PATH 最前有假的 taskkill.cmd 时也不变；接缝的默认值就是它。最后把 SystemRoot 改成一个不存在的目录，
// systemTaskkillPath 的结果仍不变（它不读 SystemRoot）。
// 前提：SystemRoot 非空（核对用）。
// 【判别力】V9（按 PATH 找：给的是假文件）、V12（默认值不是 systemTaskkillPath）；X2（用 SystemRoot 拼 System32：
// 本机前几步照绿，改 SystemRoot 之后结果跟着变），红。
func TestClxSystemTaskkillPath(t *testing.T) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		t.Fatalf("前提不成立：SystemRoot 为空，没有可核对的系统目录")
	}
	f := clxNewFake(t)
	got, err := systemTaskkillPath()
	if err != nil {
		t.Fatalf("systemTaskkillPath 应当成功，实际 %v", err)
	}
	if want := filepath.Join(root, "System32", "taskkill.exe"); !strings.EqualFold(got, want) {
		t.Errorf("systemTaskkillPath 应当是 %s（不分大小写），实际 %s", want, got)
	}
	if strings.EqualFold(got, f.path) {
		t.Errorf("systemTaskkillPath 给的是 PATH 最前的假文件 %s", got)
	}
	if st, err := os.Stat(got); err != nil || st.IsDir() {
		t.Errorf("%s 应当是一个存在的文件（err %v）", got, err)
	}
	if reflect.ValueOf(taskkillPath).Pointer() != reflect.ValueOf(systemTaskkillPath).Pointer() {
		t.Errorf("接缝 taskkillPath 的默认值应当是 systemTaskkillPath")
	}

	const bogus = `Z:\clx-nope`
	t.Setenv("SystemRoot", bogus)
	again, err := systemTaskkillPath()
	if err != nil || again != got {
		t.Errorf("SystemRoot 改成 %s 之后 systemTaskkillPath 应当仍是 %s，实际 %q（err %v）", bogus, got, again, err)
	}
}

// clxFakeExeSrc 是 C8 现编的假 taskkill.exe 的源码：只把收到的参数（以空格连接）写进同目录的 clx-marker.txt。
const clxFakeExeSrc = `package main

import (
	"os"
	"path/filepath"
	"strings"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(exe), "clx-marker.txt"), []byte(strings.Join(os.Args[1:], " ")), 0o644)
}
`

// clxBuildFakeExe 用 go build 把 clxFakeExeSrc 编成 dir 里的 taskkill.exe，返回它的路径。go 按 PATH 找（go test
// 起测试二进制时把 GOROOT/bin 排在 PATH 最前）。编译用的 GOCACHE、GOTMPDIR 与输出目录都在本用例的 t.TempDir() 里；
// 另设 GOENV=off、GOFLAGS 为空、GOWORK=off、GOPROXY=off、GOTOOLCHAIN=local。源码只用标准库，不碰模块缓存。
func clxBuildFakeExe(t *testing.T, dir string) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("前提不成立：找不到 go，编不了假的 taskkill.exe：%v", err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(clxFakeExeSrc), 0o644); err != nil {
		t.Fatalf("写假 taskkill.exe 的源码: %v", err)
	}
	gotmp := filepath.Join(src, "gotmp")
	if err := os.Mkdir(gotmp, 0o755); err != nil {
		t.Fatalf("建 GOTMPDIR: %v", err)
	}
	exe := filepath.Join(dir, "taskkill.exe")
	cmd := exec.Command(gobin, "build", "-o", exe, "main.go")
	cmd.Dir = src
	cmd.Env = append(os.Environ(),
		"GOCACHE="+filepath.Join(src, "gocache"), "GOTMPDIR="+gotmp, "GOENV=off", "GOFLAGS=", "GOWORK=off",
		"GOPROXY=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编假的 taskkill.exe 失败: %v\n%s", err, out)
	}
	return exe
}

// TestClxKillTreeIgnoresPathTaskkillExe 是 C8：PATH 最前放一个真能运行的假 taskkill.exe（Go 现编，只写标记文件），
// killProcessTree 不执行它，该杀的循环真被杀掉了。同一装置接着把接缝指向这个 exe（正对照）：它被执行、标记文件
// 内容是 /T /F /PID <pid>，证明这个 exe 能跑、「标记不存在」来自 killProcessTree 没起它。
// 与 C4 的区别：C4 的假文件是 taskkill.cmd，按 taskkill.exe 这个全名找时绕得过它；这里排在 PATH 最前的就叫 taskkill.exe。
// 【判别力】X1（按 filepath.Base(path) 即 taskkill.exe 起）、V8、V9、V12：执行了假的，标记出现、循环没死，红；
// V11（不经接缝）：正对照那一步标记不出现，红。
func TestClxKillTreeIgnoresPathTaskkillExe(t *testing.T) {
	dir := t.TempDir()
	exe := clxBuildFakeExe(t, dir)
	marker := filepath.Join(dir, "clx-marker.txt")
	clxPrependPath(t, dir, exe)
	pid, done := clxStartLoop(t)

	if err := killProcessTree(pid); err != nil {
		t.Errorf("killProcessTree 应当返回 nil（系统目录里的 taskkill 杀掉了循环），实际 %v", err)
	}
	clxWaitDone(t, pid, done)
	if clxMarkerExists(t, marker) {
		t.Errorf("killProcessTree 执行了 PATH 最前的假 taskkill.exe（标记文件 %s 出现了）", marker)
	}

	// 正对照另起一个循环：变异下这一步若起了真 taskkill，对象仍是测试自己起的、活着的进程（理由同 C5 第 2 步）。
	pid2, _ := clxStartLoop(t)
	clxSetSeam(t, func() (string, error) { return exe, nil })
	if err := killProcessTree(pid2); err != nil {
		t.Errorf("正对照：假 taskkill.exe 以 0 退出，killProcessTree 应当返回 nil，实际 %v", err)
	}
	b, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("正对照：接缝指向假 taskkill.exe 时它应当被执行、写出标记文件，实际读不到：%v", err)
	}
	if got, want := string(b), "/T /F /PID "+strconv.Itoa(pid2); got != want {
		t.Errorf("正对照：假 taskkill.exe 收到的参数应当是 %q，实际 %q", want, got)
	}
}
```

---

## 3. 冻结的字面量（新增或改动的用户可见文本）

1. 命令表里 `add` 的 summary（A1；`/help` 里那一行是 `/add` 补到 12 个字符再接它）：

   ```text
   把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别
   ```

2. `/team use` 跳过总结成员时的那一行（A3；`<id>` 是总结成员的 id，行末换行）：

   ```text
   <id> 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结
   ```

3. 命令表里 `team` 的 summary（A9，v1.1；`/help` 里那一行是 `/team` 补到 12 个字符再接它，即 C9 的 `clxHelpTeamLine`，原型实跑）：

   ```text
   绑定当前会话到一个 team，并把尚未加入的成员（总结成员除外）一次性加进来: /team use <team-id>
   ```

复用、不改的：按名字加的用法那一句 `msgAddNameUsage`（C1 期望，19g §3）；`/team use` 的「已绑定 team %s」「%s 已退出，未自动拉回，要加回用 /join」「%s 已加入会话 %s」。生产代码里新增的错误文本 `taskkill is windows-only`（B4，只在非 Windows、只给测试编译用，不上屏）不是用户可见字面量。

---

## 4. 集成员的变异清单

**运行纪律（每一次 go build / vet / test / 变异运行之前都照做）**：
- 同一条 PowerShell 命令里先设环境再跑（每次工具调用都是新进程，分两次调用等于没设）：
  - `USERPROFILE`、`HOME` 指到隔离目录下的 canary home（起草时是 `E:\Isolating_test\council\cl1_draft\canaryhome`，v1.1 修订时是 `E:\Isolating_test\council\cl1_v11\canaryhome`）；`APPDATA`、`LOCALAPPDATA` 指到它下面的 `AppData\Roaming`、`AppData\Local`（先建好）；
  - `COUNCIL_DATA_DIR` 指到隔离目录下的 canary 数据目录；`TMP`、`TEMP` 指到隔离目录下的 `tmp`（并行的工作副本各用自己的 `tmp\m1`、`tmp\m2`、`tmp\m3`，核对也只扫自己的）；
  - 用 `[Environment]::SetEnvironmentVariable('<名>', $null, 'Process')` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`（不要用 `Remove-Item Env:`，会被路径保护整条拦下）；
  - `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE` 指到隔离目录下、`GOTOOLCHAIN=local` 显式给；**不要清 `SystemRoot`、`windir`**（C7 要用 `SystemRoot` 核对；`GetSystemDirectory` 不靠它）；
  - `PATH` 只留 `C:\Program Files\Go\bin` 与 `C:\Windows\System32`，去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`。
  - 起草人把这些写成一个纯 ASCII 的 `goenv.ps1`，每条命令开头 dot-source 它，再跑 go 或变异驱动（node 由同一个 PowerShell 起，环境随之继承；node 用全路径 `C:\Program Files\nodejs\node.exe`）。集成员自己写的 `.ps1` 若含中文，要存成带 BOM 的 UTF-8。
- 每次 go test（全量、每条变异）之后核四项，任何一项出现就停下、报告实际路径，不删隔离目录以外的任何东西：
  1. canary 数据目录下没有 `council.db`；
  2. 隔离 `TMP`、真 `%TEMP%`（`C:\Users\25142\AppData\Local\Temp`）、`E:\Go\tmp` 下没有 `TestOb*`、`TestAdn*`、`TestClx*` 目录；
  3. `E:\council-data` 不存在；
  4. canary home 下没有 `.council`。
  另核一项残留进程：开跑之后新起、还活着的 `cmd.exe`、`taskkill.exe`、`node`、`go`、`*.test`、`council` 进程（按 `CreationDate` 晚于开跑时刻筛；排除宿主自己的 shell 启动器）。起草期间与 v1.1 修订期间核过的每一轮（全量运行与每一批变异之后）都是 0。

每条在原型上单独施加、从字节副本还原（不用 `git checkout`），还原后 SHA256 核对。替换前后都是**整行**（行首是 tab）：驱动把「替换前」与「替换后」各自接上行尾的换行，在文件里按整行数（前后都是换行或文件头），恰好一次才施加；「替换后」为空的，是把替换前那几行整段删掉。包与 `-run`：一律跑目标文件所在的**整个包**（`./internal/repl`、`./internal/turn`、`./internal/tui`，不带 `-run`，既有用例的杀手也点出来），`go test -json -count=1`，按 fail 事件点名，「杀手」列是原型上这一次实测变红的全部叶子用例（父测试自己的断言红时写父测试名）。耗时是整条变异（编译加测试）的墙钟。

驱动（v1 起草时，下面一段的数字是 v1 的；v1.1 的见再下一段）：原型的三份工作副本 `m1`、`m2`、`m3` 并行跑（每份各自的 `TMP`、`TEMP`）。每条之前核目标文件 SHA256 与原型相同，施加、跑、从内存里的字节副本写回、再核 SHA256；每条之后核四项。驱动从本节解析替换前后（先由脚本把起草清单写成本节的代码块，再从本节解析回来与清单逐条比过，18 条全同；最后一轮由这份解析结果驱动）。最后一轮跑在最终字节上：18 条全部还原一致、四项核对次次干净、残留进程 0、没有一条编译失败；16 条的杀手列就是这一轮实测变红的全部用例，E1、E2 为 0 个。起草中途（测试文件的【判别力】注释改写之前）先跑过一轮，杀手列与这一轮逐条相同；那一轮里 V9 的第一种写法（替换后写 `_ = dir`）编译不过（`path/filepath` 没用到），改成现在的 `_ = filepath.Join(dir, "taskkill.exe")` 之后重跑。耗时（三路并行时的墙钟）：repl 每条 78.4-83.0 s，turn 每条 13.7-29.1 s，tui 每条 63.3-63.6 s；两批分别 244.6 s 与 127.1 s。

**v1.1 重跑**（隔离目录 `E:\Isolating_test\council\cl1_v11\`，原型照 v1 逐字落盘、先全绿，再按主代理裁决改；三份工作副本 `m1`、`m2`、`m3` 并行，各用 `tmp\m1`、`tmp\m2`、`tmp\m3`）：本节 22 条（v1 的 18 条加 V17、X1、X2、X4）全部在 v1.1 的最终字节上重跑，驱动从本节解析替换前后，下面每条的耗时与杀手列都是这一轮的实测（由脚本从结果文件写进本节，再解析回来逐条比过，22 条全同）。22 条锚点各恰好一次、全部还原后 SHA256 一致、四项核对次次干净、每批之后残留进程 0、没有一条编译失败；20 条有杀手，E1、E2 为 0 个。分三批：tui 与 turn 一批（178.3 s），repl 一批（240.2 s），turn 又一批（108.0 s）。turn 重跑一批的原因：第一批之后发现 C5 第 2 步与 C8 正对照用的是已被杀掉的循环的 pid，变异（V11）下会对一个已退出、可能被系统复用的 pid 起真 taskkill；改成各自另起一个循环之后重跑 turn 的 10 条，杀手与第一批逐条相同，本节记的是重跑那一批。耗时（三路并行时的墙钟）：repl 每条 76.8-82.0 s，turn 每条 19.4-43.2 s，tui 每条 71.0-71.1 s。与 v1 相比，杀手列变了的是 V8、V9、V11、V12（多了 C5 或 C8，或两者）与 V15、V16（C3 改名、拆成三个子测试）；V1-V7、V10、V13、V14 的杀手与 v1 相同，E1、E2 仍为 0 个。

**V1** `internal/repl/commands.go`：命令表里 add 的 summary 换回 HEAD 的旧句（不提按名字加）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 79.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		{name: "add", summary: "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别", needsSvc: true, needsConv: true, run: cmdAdd},
```

替换后：

```go
		{name: "add", summary: "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别", needsSvc: true, needsConv: true, run: cmdAdd},
```

杀手（v1.1 原型实测，2 个叶子用例）：`TestCxhHelpAddLine`、`TestUx2rJoinIsAdd`

**V2** `internal/repl/addname.go`：parseAddNameArgs 里 --id 那一半去掉 i+1 >= len(fields)（19g 头的 M9）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 82 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			if a.haveID || i+1 >= len(fields) {
```

替换后：

```go
			if a.haveID {
```

杀手（v1.1 原型实测，1 个叶子用例）：`TestClxAddIDMissingValue`

**V3** `internal/repl/addname.go`：parseAddNameArgs 里 --effort 那一半去掉 i+1 >= len(fields)（对照：杀手是既有的 TestAdnFlags）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 82 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			if a.haveEffort || i+1 >= len(fields) {
```

替换后：

```go
			if a.haveEffort {
```

杀手（v1.1 原型实测，1 个叶子用例）：`TestAdnFlags`

**V4** `internal/repl/commands.go`：cmdTeamUse 去掉总结成员那一条（HEAD 的行为：照普通成员 Join）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 76.8 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if sumID := r.cfg.Summary.Agent; sumID != "" && agentID == sumID {
```

替换后：

```go
		if sumID := r.cfg.Summary.Agent; false && sumID != "" && agentID == sumID {
```

杀手（v1.1 原型实测，1 个叶子用例）：`TestClxTeamUseSummaryMember/not_member`

**V5** `internal/repl/commands.go`：总结成员那一条挪到「已是成员」那一支之前（在场、已退出的总结成员也走它）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 79.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		p, found := existing[agentID]
```

替换后：

```go
		p, found := existing[agentID]
		if sumID := r.cfg.Summary.Agent; sumID != "" && agentID == sumID {
			found = false
		}
```

杀手（v1.1 原型实测，2 个叶子用例）：`TestClxTeamUseSummaryMember/left`、`TestClxTeamUseSummaryMember/present`

**V6** `internal/repl/commands.go`：打了说明却不 continue（说明之后照样 Join）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 78 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			if err := r.printf("%s 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结\n", agentID); err != nil {
				return err
			}
			continue
		}
```

替换后：

```go
			if err := r.printf("%s 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结\n", agentID); err != nil {
				return err
			}
		}
```

杀手（v1.1 原型实测，1 个叶子用例）：`TestClxTeamUseSummaryMember/not_member`

**V7** `internal/repl/commands.go`：判据换成「配了总结成员就跳过所有缺的成员」（去掉 agentID == sumID）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 77.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if sumID := r.cfg.Summary.Agent; sumID != "" && agentID == sumID {
```

替换后：

```go
		if sumID := r.cfg.Summary.Agent; sumID != "" {
```

杀手（v1.1 原型实测，3 个叶子用例）：`TestClxTeamUseSummaryMember/left`、`TestClxTeamUseSummaryMember/not_member`、`TestClxTeamUseSummaryMember/present`

**V8** `internal/turn/killtree_windows.go`：killProcessTree 按名字起 taskkill（HEAD 的写法，按 PATH 找）。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 41.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	cmd := exec.Command(path, "/T", "/F", "/PID", strconv.Itoa(pid))
```

替换后：

```go
	_ = path
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
```

杀手（v1.1 原型实测，3 个叶子用例）：`TestClxKillTreeIgnoresPathTaskkill`、`TestClxKillTreeIgnoresPathTaskkillExe`、`TestClxKillTreeUsesSeam`

**V9** `internal/turn/killtree_windows.go`：systemTaskkillPath 按 PATH 找 taskkill。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 43.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return filepath.Join(dir, "taskkill.exe"), nil
```

替换后：

```go
	_ = filepath.Join(dir, "taskkill.exe")
	return exec.LookPath("taskkill")
```

杀手（v1.1 原型实测，3 个叶子用例）：`TestClxKillTreeIgnoresPathTaskkill`、`TestClxKillTreeIgnoresPathTaskkillExe`、`TestClxSystemTaskkillPath`

**V10** `internal/turn/killtree_windows.go`：取不到 taskkill 的路径时退回按名字起 taskkill。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 23.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	path, err := taskkillPath()
	if err != nil {
		return err
	}
```

替换后：

```go
	path, err := taskkillPath()
	if err != nil {
		path = "taskkill"
	}
```

杀手（v1.1 原型实测，1 个叶子用例）：`TestClxKillTreeSeamErrorNoFallback`

**V11** `internal/turn/killtree_windows.go`：killProcessTree 不经接缝、直接调 systemTaskkillPath。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 23.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	path, err := taskkillPath()
```

替换后：

```go
	path, err := systemTaskkillPath()
```

杀手（v1.1 原型实测，3 个叶子用例）：`TestClxKillTreeIgnoresPathTaskkillExe`、`TestClxKillTreeSeamErrorNoFallback`、`TestClxKillTreeUsesSeam`

**V12** `internal/turn/killtree_windows.go`：接缝的默认值换成按 PATH 找。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 40.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
var taskkillPath = systemTaskkillPath
```

替换后：

```go
var taskkillPath = func() (string, error) { return exec.LookPath("taskkill") }
```

杀手（v1.1 原型实测，3 个叶子用例）：`TestClxKillTreeIgnoresPathTaskkill`、`TestClxKillTreeIgnoresPathTaskkillExe`、`TestClxSystemTaskkillPath`

**V13** `internal/turn/loginstatus_exec.go`：RunLoginStatus 超时那一支只删 cmd.Process.Kill 那一行（19e 的 N32）。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 25.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			_ = killProcessTree(cmd.Process.Pid)
			_ = cmd.Process.Kill()
```

替换后：

```go
			_ = killProcessTree(cmd.Process.Pid)
```

杀手（v1.1 原型实测，1 个叶子用例）：`TestObyRunLoginStatusKilledWithoutTaskkill`

**V14** `internal/turn/loginstatus_exec.go`：RunLoginStatus 超时那一支两行都删（19e 的 N30）。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 34.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			_ = killProcessTree(cmd.Process.Pid)
			_ = cmd.Process.Kill()
```

替换后：

```go
```

杀手（v1.1 原型实测，2 个叶子用例）：`TestObyRunLoginStatusKilledPromptly`、`TestObyRunLoginStatusKilledWithoutTaskkill`

**V15** `internal/tui/model.go`：列表分支的 Ctrl-C 判据写成只放过 Shift（19c 文件头审查员的那条）。 包 `./internal/tui`（整个包，不带 -run）；v1.1 原型实测 71.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':
				m.at = atState{}
```

替换后：

```go
			case msg.Mod&^tea.ModShift == tea.ModCtrl && msg.Code == 'c':
				m.at = atState{}
```

杀手（v1.1 原型实测，3 个叶子用例）：`TestClxCtrlCModsClosesList/alt`、`TestClxCtrlCModsClosesList/capslock`、`TestClxCtrlCModsClosesList/numlock`

**V16** `internal/tui/model.go`：列表分支的 Ctrl-C 判据收窄成只认 Ctrl（19c 的 A60）。 包 `./internal/tui`（整个包，不带 -run）；v1.1 原型实测 71 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':
				m.at = atState{}
```

替换后：

```go
			case msg.Mod == tea.ModCtrl && msg.Code == 'c':
				m.at = atState{}
```

杀手（v1.1 原型实测，4 个叶子用例）：`TestAtfCtrlShiftCClosesList`、`TestClxCtrlCModsClosesList/alt`、`TestClxCtrlCModsClosesList/capslock`、`TestClxCtrlCModsClosesList/numlock`

**V17** `internal/repl/commands.go`：命令表里 team 的 summary 换回 HEAD 的旧句（不提总结成员，v1.1 主代理裁决 (f)）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 78.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		{name: "team", summary: "绑定当前会话到一个 team，并把尚未加入的成员（总结成员除外）一次性加进来: /team use <team-id>", needsSvc: true, needsConv: true, run: cmdTeam},
```

替换后：

```go
		{name: "team", summary: "绑定当前会话到一个 team，并把尚未加入的成员一次性加进来: /team use <team-id>", needsSvc: true, needsConv: true, run: cmdTeam},
```

杀手（v1.1 原型实测，1 个叶子用例）：`TestClxHelpTeamLine`

**X1** `internal/turn/killtree_windows.go`：killProcessTree 按 filepath.Base(path) 起（去掉目录之后按 PATH 找 taskkill.exe；W1 审查员的 X1，v1.1 主代理裁决 m1）。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 30.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	cmd := exec.Command(path, "/T", "/F", "/PID", strconv.Itoa(pid))
```

替换后：

```go
	cmd := exec.Command(filepath.Base(path), "/T", "/F", "/PID", strconv.Itoa(pid))
```

杀手（v1.1 原型实测，2 个叶子用例）：`TestClxKillTreeIgnoresPathTaskkillExe`、`TestClxKillTreeUsesSeam`

**X2** `internal/turn/killtree_windows.go`：systemTaskkillPath 改用 SystemRoot 拼 System32（W1 审查员的 X2，v1.1 主代理裁决 m2）。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 20.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	dir, err := windows.GetSystemDirectory()
```

替换后：

```go
	_, _ = windows.GetSystemDirectory()
	root, _ := syscall.Getenv("SystemRoot")
	dir, err := filepath.Join(root, "System32"), error(nil)
```

杀手（v1.1 原型实测，1 个叶子用例）：`TestClxSystemTaskkillPath`

**X4** `internal/tui/model.go`：列表分支的 Ctrl-C 判据写成只放过 Shift 与 Alt（锁定键那一半漏掉；W1 审查员的 X4，v1.1 主代理裁决 m3）。 包 `./internal/tui`（整个包，不带 -run）；v1.1 原型实测 71 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':
				m.at = atState{}
```

替换后：

```go
			case msg.Mod&^(tea.ModShift|tea.ModAlt) == tea.ModCtrl && msg.Code == 'c':
				m.at = atState{}
```

杀手（v1.1 原型实测，2 个叶子用例）：`TestClxCtrlCModsClosesList/capslock`、`TestClxCtrlCModsClosesList/numlock`

**判定为等价、不列杀手的变异**（同样实跑，0 个用例变红）：

**E1** `internal/turn/loginstatus_exec.go`：RunLoginStatus 超时那一支只删 killProcessTree 那一行（19e 的 N31，等价）。 包 `./internal/turn`（整个包，不带 -run）；v1.1 原型实测 19.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			_ = killProcessTree(cmd.Process.Pid)
			_ = cmd.Process.Kill()
```

替换后：

```go
			_ = cmd.Process.Kill()
```

杀手：无（v1.1 原型实测 0 个用例变红，等价）

**E2** `internal/repl/commands.go`：cmdTeamUse 总结成员那一条去掉 sumID != "" 那一半（等价：team 的 members 里没有空串）。 包 `./internal/repl`（整个包，不带 -run）；v1.1 原型实测 78.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if sumID := r.cfg.Summary.Agent; sumID != "" && agentID == sumID {
```

替换后：

```go
		if sumID := r.cfg.Summary.Agent; agentID == sumID {
```

杀手：无（v1.1 原型实测 0 个用例变红，等价）

**只在审查时看**：
- A2、A4、A5、A6 与 A3、B2、B4 里的注释行；B6、B7 的注释；v1.1 的 B8、B9 两句注释。注释要与代码相符（A4、A5 对 19g D7、D9、D17，A6 对 `firstrun.go:63-65`，B2 对三处调用方，B8、B9 对 B1、B2）。B2 注释里「没有『SystemRoot 为空』这一种情形」说的是路径（上一句「系统目录由系统调用给出，不随环境变量变」），不是 taskkill.exe 自己能不能跑；B2 属于 v1 冻结的生产代码块，v1.1 按主代理裁决一个字节不改，这层意思记在 §7 (l)。
- A3 里 `r.printf` 写屏幕失败那一支（`return err`）：与 `cmdTeamUse` 里其余 `printf` 失败的写法相同，没有用例（写屏幕失败的装置本包没有）。
- B2 里 `systemTaskkillPath` 的系统调用失败那一支：`GetSystemDirectory` 在正常系统上不失败，没有装置；它的后果由 C6（接缝返回错误时什么都不起）担保——`killProcessTree` 不区分错误来自接缝的哪一种实现。
- V8-V12、X1、X2 的杀手只在 Windows 上跑得到（S3 只在 Windows 编译）；非 Windows 的 B4 只保编译（D7），没有行为用例。
- C8 要在测试里调 `go build`：找不到 `go` 或编译失败时判前提不成立（`Fatalf`），不跳过。
- 19e 的 N31（本文 E1）照旧等价：L5 的循环没有孙进程，`Kill` 一样杀得掉。

**AD1、OB2、AT1-FU 的既有变异在本刀下的状态**：19g 的变异锚点不在 A4、A5 改的注释行里（它们是代码行），原样可跑；A5 改了 `addname.go` 第 13-17 行的注释，之后的行号后移 3 行。19e 的 N30、N31、N32 锚点在 `loginstatus_exec.go`，本刀不改那个文件，原样可跑（本文 V14、E1、V13 就是它们），N32 的杀手仍是 L5b。19c 的 A60（本文 V16）锚点在 `model.go`，原样可跑，杀手比 19c 多了 C3。v1.1 的 A9 是一行换一行，`commands.go` 的行号不变；B8、B9 同样一行换一行。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许动这些文件：**块 A**：补丁 `internal/repl/commands.go`（A1-A4、A9）、`internal/repl/addname.go`（A5）、`internal/repl/addlogin.go`（A6）、`internal/repl/cxh_fixture_test.go`（A7）、`internal/repl/ux2r_add_test.go`（A8）；**块 B**：补丁 `internal/turn/killtree_windows.go`（B1、B2）、`internal/turn/killtree_other.go`（B3、B4）、`internal/turn/oby_loginstatus_exec_test.go`（B5-B8）、`internal/turn/obx_loginstatus_exec_test.go`（B9）。不改 `go.mod`、`go.sum`（`golang.org/x/sys` 已是直接依赖），不改别的既有 `_test.go`，不新写任何 `clx` 测试文件（那是测试腿的事）。
- 行数：块 A 替换文本合计 42（生产代码 40 行，A7、A8 各 1 行） 行；块 B 合计 74（生产代码 57 行，B5-B7 合计 15 行，B8、B9 各 1 行） 行。两块可以分两路、分两次交，也可以一次交完；两块互不依赖。块 A 里 A1 与 A7、A8 要一起交；块 B 里 B1-B9 要一起交（D10）。
- 补丁块的格式：

  ```
  ===PATCH: internal/turn/killtree_windows.go===
  <<<<ANCHOR
  （从 HEAD 483a766 真文件里原样复制的若干整行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```

  锚点照 §1.1 各处的 `anchor` 代码块（本文代码块里的缩进是 tab；若你看到的是空格，以真文件为准）；同一文件的多个锚点互不重叠，按文件顺序排（`commands.go` 里依次是 A9、A1、A2、A3、A4（A9 在 `:154`，A1 之上），B1、B2 与 B3、B4 同样，`oby_loginstatus_exec_test.go` 里依次是 B5、B6、B8、B7）。**每个 `===PATCH:`、`<<<<ANCHOR`、`====`、`>>>>`、`===END PATCH===` 标记都单独起一行、从行首开始**（OB2 的块 A 把标记接在开场白末尾，按行首解析的落地脚本漏了文件）。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。本刀没有整文件交付（没有 `===FILE:`）。
- 代码必须能 `gofmt`（tab 缩进）；`go vet` 不许有新报告；`GOOS=linux go vet ./internal/turn` 也要过（D7）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（全角括号、冒号、分号、逗号、顿号、直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增或改动的用户可见字符串只有 §3 的三句（第 3 句是 v1.1 的 A9）。
- 生产代码新标识符不许以 `clx` 开头。§1.1 给出的签名、判据、代码、注释照抄；本刀没有「名字实现方定」处。
- 回复末尾必须有：

  ```
  ===REPORT===
  块：给了哪些补丁（块 A、块 B 各自）。
  补全性：A1-A9、B1-B9 各落在哪个 PATCH 块。
  锚点：每个锚点是否从 HEAD 483a766 真文件逐字复制、在该文件里恰好一次。
  字面量：§3 的三句是否逐字照抄；有没有新增任何别的用户可见字面量。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  ```

---

## 6. 落地后主代理要做的

1. **收割与逐字核对**：越界扫描（主仓库、`internal/*`、`cmd/*`、中转根有没有实现腿写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；标记不在行首的也要找。逐字核对：用脚本从本文取出每一对 `anchor` / `replace` 与每个 `file` 代码块，打在 HEAD `483a766` 的一份 `git archive` 上，结果应当与落盘后的 12 个文件（`commands.go`、`addname.go`、`addlogin.go`、`cxh_fixture_test.go`、`ux2r_add_test.go`、`killtree_windows.go`、`killtree_other.go`、`oby_loginstatus_exec_test.go`、`obx_loginstatus_exec_test.go`（v1.1 的 B9），外加测试腿的三个 `clx` 文件）逐字节相同（v1.1 就是这样核的：18 对锚点各恰好一次，3 个整文件）。`gofmt -l ./internal ./cmd`；`go vet ./...`；`GOOS=linux go vet ./internal/turn ./internal/repl ./internal/tui`；`go build ./...`。`internal/tui`、`internal/app`、`internal/config`、`internal/provider`、`internal/store`、`internal/conversation`、`cmd/council` 零改动。
   - grep 核对：生产代码（非 `_test.go`）里搜 `"taskkill"`（带引号的裸名），命中应当是 0 处；搜 `taskkillPath`，命中只在 `killtree_windows.go`、`killtree_other.go` 两个文件。在 `internal` 与 `cmd` 下的 `*.go` 文件里搜 `把 agent 加进当前会话`，命中恰好三处（`commands.go` 的命令表、A7、A8；`docs` 不在范围内）。搜 `总结成员除外`，命中恰好三处：`commands.go` 的命令表（A9）、`clx_cl1_test.go` 的文件头注释与 `clxHelpTeamLine`。
2. **合并测试腿的 3 个文件**，先跑 `go test -count=1 ./internal/repl ./internal/tui ./internal/turn`，再跑全量 `go test -count=1 ./...`（原型九包全绿）；`-run 'TestClx|TestObyRunLoginStatusKilled'`（repl、tui、turn）跑一次 `-count=3`。**每一次运行都照 §4 表头的运行纪律**，跑完核四项与残留进程。C8 在测试里调 `go build` 编一个只写标记文件的小程序（§2 纪律），落地机器上要有 `go`（`go test` 会把 GOROOT/bin 排在 PATH 最前）。
3. **§4 的 22 条变异逐条跑**，照 §4 表头的纪律，从字节副本还原、SHA256 核对；点名杀手里的每个用例都要单独确认变红，E1、E2 确认 0 个变红。耗时（v1.1，三路并行时的墙钟，§4）：repl 每条 76.8-82.0 s、turn 每条 19.4-43.2 s、tui 每条 71.0-71.1 s；v1.1 三批实测 178.3 s、240.2 s、108.0 s（第三批是 turn 的重跑，原因见 §4）。PowerShell 单次工具调用上限 10 分钟，按每批 8 分钟以内分批跑。
4. **文档**（主代理定写法；下面是起草人的建议）：
   - `docs/00_DECISIONS.md`：新立 **#50「CL1 一批小修补（2026-10-01，Griffin「开始做那批小修补吧」）」**，与代码同一个提交：逐条记七处改了什么、委派链、验证等级、§7 的待定项。另在三条旧条目里各加一行指向 #50：#47（CH1「待主代理」第二条「`/team use` 会 Join 总结成员」已由 CL1 修；AT1-FU 审查那条 Ctrl+Alt+C 的 minor 已补用例）、#48（OB2「待主代理」第三条「`killProcessTree` 按 PATH 找 taskkill」已修；OB3「待主代理」第一条「`LoginCommand` 的注释说多了」已改）、#49（「待定」第一条 `/help` 已补；19g 头的 M9 与两处注释已做）。
   - 各契约文件头的订正：19g 头「落地后订正」第一、二条与 §7 (e) 记「CL1 已做（#50）」；19f 头与 §7 (k) 同样；19d §7 (d) 记「CL1 已改成系统目录（GetSystemDirectory），不读 SystemRoot」；19c 头第一条记「CL1 补了 C3，V15 现在有杀手」；19e 的 L5b 写法（E7、§2.3）记「CL1 改成换接缝，PATH 不再改」；19a 文件头「`/team use` 会 Join 总结成员」记已修。
   - 本契约归档到 `docs/19h_CL1_SMALL_FIXES_CONTRACT.md`（照 19d-19g 的体例在文件头加落地后订正区）。
   - `docs/README.md`：`/team use` 那一节（若有）加一句「team 里的总结成员不随 team 加入，多个成员回复之后它自己加入」；`/help` 那一处若引用了 add 行，照 §3 第 1 句改；若引用了 team 行，照 §3 第 3 句改（v1.1）。
   - `docs/14_MVP_CHECKLIST.md`：加一项 **M8「CL1 一批小修补」**，步骤照下面第 5 步（`-version`、`/help`、`/team use`）。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的真机复验步骤**（E 盘机，零花费：不提问、不识别、不起任何 provider 进程；用一份**沙盒配置**，不碰你的真配置与日常库）。下面每一条命令都对着命令表（`commands.go:149-173`：`/new`、`/help`、`/team`、`/who`、`/quit` 都在）与启动参数（`cmd/council/main.go:41-45`：`-config`、`-version`、`-no-tui` 都在）核过；沙盒那几条已在原型上实跑（§0 第 1、5 条）。
   0. 先确认装的是带本刀的版本（主代理照惯例等 Griffin 说「装」再重装）：

      ```powershell
      council -version
      ```

   1. **只敲斜杠命令，不要直接打字**：对话模式默认打开，直接打字会让会话里的成员回复（要花钱）。
   2. 建沙盒（PowerShell；`data_dir` 指到沙盒里，库建在沙盒里，不碰 `C:\Users\25142\.council`）：

      ```powershell
      $sb = "$env:TEMP\cl1check"
      New-Item -ItemType Directory -Force $sb | Out-Null
      $cfg = @(
        "data_dir = '$sb\data'",
        "",
        "[providers.claude]",
        "command = 'claude'",
        "",
        "[agents.cl1-a]",
        "provider = 'claude'",
        "model = 'sonnet'",
        "",
        "[agents.cl1-b]",
        "provider = 'claude'",
        "model = 'sonnet'",
        "",
        "[agents.cl1-sum]",
        "provider = 'claude'",
        "model = 'sonnet'",
        "",
        "[summary]",
        "agent = 'cl1-sum'",
        "",
        "[teams.cl1]",
        "members = ['cl1-a', 'cl1-b', 'cl1-sum']"
      ) -join "`n"
      [System.IO.File]::WriteAllText("$sb\config.toml", $cfg + "`n", (New-Object System.Text.UTF8Encoding $false))
      council -no-tui -config "$sb\config.toml"
      ```

      写成无 BOM 的 UTF-8。这一段起草时原样跑过（把 `$env:TEMP` 指到隔离目录）：写出的配置与 §0 第 5 条用的那一份只差末尾一个换行，原型的 `council -no-tui -config` 加载它，`/team use cl1` 打的就是下面那四行（原型实跑）。预期开头几行里有「已加载 3 个 agent，1 个 team」。`[providers.claude] command = 'claude'` 只是让配置过得了校验，下面几条命令都不起它。
   3. 依次敲：

      ```text
      /new cl1check
      /help
      /team use cl1
      /who
      /quit
      ```

      预期：
      - `/help` 里 `/add` 那一行是 §0 第 1 条那一整行（「识别出的模型可以直接 /add 它的名字……」）；`/team` 那一行是「/team」补到 12 个字符再接 §3 第 3 句（「……并把尚未加入的成员（总结成员除外）一次性加进来: /team use <team-id>」）。v1.1 用原型构建的 `council.exe -no-tui` 照这一段的沙盒配置与这五条命令实跑过（stdin 由 cmd 从文件重定向，`COUNCIL_DATA_DIR` 与 `data_dir` 都指到隔离目录）：两行逐字如此，`/team use cl1` 打的就是下面四行，`/who` 只有 `cl1-a`、`cl1-b`。
      - `/team use cl1` 打四行（会话 id 以屏幕为准）：

        ```text
        已绑定 team cl1
        cl1-a 已加入会话 <会话 id>
        cl1-b 已加入会话 <会话 id>
        cl1-sum 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结
        ```

      - `/who` 只列 `cl1-a`、`cl1-b`，没有 `cl1-sum`。
   4. 也可以用 TUI 跑同一份（去掉 `-no-tui`），`/help` 那一行在窄终端里可能折行，内容应当相同 [推断，TUI 没有实跑这一步]。用完删沙盒：`Remove-Item -Recurse -Force "$env:TEMP\cl1check"`（只删这个沙盒目录）。
   5. **你自己的配置里**若已有含总结成员的 team，可以直接在真配置里开一个新会话敲 `/team use <那个 team>` 看同一行；没有就用上面的沙盒。第 6 条（taskkill）与第 7 条（Ctrl+Alt+C）没有零花费、可观察的真机步骤：前者只在超时杀进程时起作用，后者 Windows Terminal 可能自己占用这个键（19c §7 (c) 同类），以单元测试为准。
   - D 盘机照同样的步骤做，[未验证]。

---

## 7. open items

- (a) **本刀不做、留作记录的**（任务书第 8 条）：守卫提示让重敲的命令换档（19g 头第三条，以后与 +编号 一起改成回显完整的原命令）；`msgFirstRunNoneFound` 与 `loginCommands` 的同步（19f §7 (j)，A6 只把注释写准、指向它）；当前目录同名文件遮住 PATH 上的安装（19d §7 (k)）；codex 失败轮的 `usage_raw`（#46）；「三家」之类的计数说法。
- (b) **`systemTaskkillPath` 不读 `SystemRoot`**（D6）：任务书的例子是 `%SystemRoot%\System32\taskkill.exe`；本文改用 `GetSystemDirectory`，理由在 D6。代价：多引用一个 x/sys 的函数（`internal/turn` 第一次引用 x/sys）。32 位进程在 64 位系统上调 `GetSystemDirectory` 拿到的字符串同样是 `C:\Windows\system32`，由 WOW64 的文件系统重定向器把访问映射到 `SysWOW64`，那里同样有 taskkill.exe（v1.1 改写，W1 审查员实测，v1.1 复测：在原型里临时放一个只打印 `windows.GetSystemDirectory()` 的小程序，`GOARCH=386` 与 `GOARCH=amd64` 各构建一次，两者都打出 `C:\Windows\system32 <nil>`，跑完即删；`C:\Windows\SysWOW64\taskkill.exe` 存在；Council 是 64 位构建）。主代理若要照字面用 `SystemRoot`，改法是 B2 里 `systemTaskkillPath` 的函数体，`SystemRoot` 为空时同样返回错误（不退回 PATH），C7 不用改。
- (c) **`killtree_other.go` 里只为测试编译而留的变量**（D7）：生产代码里多了一个在非 Windows 上没人调的变量。另一条路是给 oby 文件加 `//go:build windows`（任务书没授权）。
- (d) **spawn 与 discovery 两条路没有自己的「PATH 里的假 taskkill」用例**：三处调用方共用 `killProcessTree`，C4-C7 直接测它；`spawn.go`、`discovery_exec.go` 超时那一支本刀没有另加装置（它们的既有超时用例照旧绿）。
- (e) **C4 的「循环真被杀掉」用的是 10 s 上限**：真 taskkill 杀一个 cmd 循环实测不到 1 s（C4 整条 0.47 s，原型实跑）；机器很忙时 10 s 仍够宽。
- (f) **`team` 在命令表里的 summary**：v1 留作 open item；**v1.1 已改**（主代理裁决 (f)）：A9 在「尚未加入的成员」之后补「（总结成员除外）」，§3 第 3 句；HEAD 上没有测试钉着它，新用例 C9 钉住，§4 V17 实测被 C9 杀。
- (g) **两处测试注释**：v1 留作 open item；**v1.1 已改**（主代理裁决 (g)，只改注释）：oby 里 L5 的【判别力】改成「系统目录里的 taskkill 照样杀树」（B8）；`obx_loginstatus_exec_test.go` 文件头的括号改成「CL1 起超时杀树只起系统目录里的 taskkill.exe，与 PATH 无关」（B9）。
- (h) **C2 的 `present` 子测试**比的是两份装置的逐行输出，另加一份字面期望；它杀 V5、V7，不杀 V4、V6（在场的分支本来就不 Join）。
- (i) **`/team use` 打这一行的位置**按 `t.Members` 的顺序（D5）：总结成员排在 team 前面时，这一行在「已加入」各行之间。没有另做排序。
- (j) **E2 的等价**依赖「team 的 members 里没有空串」：`config.Validate` 要求成员是已定义的 agent，agent id 有正则、不许为空；直接构造 `Config` 的测试才可能有空串成员，本包没有。
- (k) **`cmdAdd` 空段时的用法句也没提按名字加**（v1.1 记录，W1 审查员附带看到）：`commands.go:964`（打完本刀补丁之后的行号；HEAD 是 `:947`）的「用法: /add 列出可加入的；/add <全名|编号|前缀>[,...] 一次加多个」。它不是 `/help`，不在本刀范围；`ux2r_fixture_test.go:54` 的 `ux2rMsgAddUsage` 钉着它，要改就要连那条期望一起授权。
- (l) **`SystemRoot` 坏掉时 taskkill.exe 自己会失败**（v1.1 记录，主代理裁决 m4）：进程环境里的 `SystemRoot` 为空、是相对路径、指向不存在的目录或被 unset 时，`systemTaskkillPath` 给的路径不变，系统目录里的 `taskkill.exe` 照样被起，但它以 exit status 1 退出、杀不掉进程树，只剩三处调用方紧接着调的 `cmd.Process.Kill` 兜底（最坏只杀到垫片那一层）。HEAD 按名字起 taskkill 时相同，是既有行为（W1 审查员在实现拷贝与 HEAD 拷贝上各用探针实测；v1.1 没有复跑这一条）。给 taskkill 的 `cmd.Env` 补一个 `SystemRoot`（例如取自 `windows.GetWindowsDirectory`）是一种改法，本刀不做。B2 注释里「没有『SystemRoot 为空』这一种情形」说的是路径，不是 taskkill.exe 能不能跑；B2 属于 v1 冻结的生产代码块，v1.1 一个字节不改。
- (m) **列表分支其余按键在锁定键开着时**（v1.1，主代理裁决 m3 的「同类的还有没有别处」，只核、不改代码）：`model.go:378-391` 的 Esc、↑、↓、Tab、回车只比 `msg.Code`、不看 `Mod`，NumLock（以及别的锁定键）开着时照常。原型上一次性探针实跑：列表开着，先按 ↓ 再按这个键，带 `tea.ModNumLock` 与不带两种，行、值、表头、Cmd 是否为 nil 逐项相同（探针跑完即删）。没有缺口。

**泄漏核对（起草期间，照 §4 表头）**：隔离目录 `E:\Isolating_test\council\cl1_draft\`：原型 `proto\`；HEAD 对照 `head\`（跑过三次 HEAD 探针：C2、C3 的 clx 文件各放进去跑一次，C4 去掉接缝那几行的探针跑一次，跑完即删，删后 `head\` 与新解出的 HEAD 全树比过 0 差异，见「往返核对」）；变异用的 `m1\`、`m2\`、`m3\`；只套生产代码的 `pc\`；沙盒 `sandbox\`（原型与 HEAD 各构建一个 `council.exe`，用沙盒配置 `-no-tui` 跑 §0 第 1、5 条，库建在 `sandbox\data`）；往返核对用的 `rt0\`、`rt\`（只比字节、没有跑 go）。每一次 go build / vet / test 与每一条变异都在同一条 PowerShell 命令里先 dot-source 纯 ASCII 的 `goenv.ps1`：`USERPROFILE`、`HOME` 指到 `canaryhome`，`APPDATA`、`LOCALAPPDATA` 指到它下面的 `AppData\Roaming`、`AppData\Local`（先建好），`COUNCIL_DATA_DIR` 指到 `canarydata`，`TMP`、`TEMP` 指到 `tmp`（变异驱动给三份副本各设 `tmp\m1`、`tmp\m2`、`tmp\m3`，`pc\` 用 `tmp\pc`，每次只核本副本的 tmp 加真 `%TEMP%` 与 `E:\Go\tmp`），用 `[Environment]::SetEnvironmentVariable` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`、`CLAUDECODE` 与 `CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`，显式给 `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE`（隔离目录下的 `gocache`）、`GOTOOLCHAIN=local`，`PATH` 收窄到 Go 与 System32，`SystemRoot`、`windir` 不动。每次 go test 之后、每条变异之后都核四项（canarydata 下没有 `council.db`；隔离 `tmp`、真 `%TEMP%`、`E:\Go\tmp` 下没有 `TestOb*`、`TestAdn*`、`TestClx*` 目录；`E:\council-data` 不存在；canaryhome 下没有 `.council`），次次干净；残留进程（开跑之后新起、还活着的 `cmd`、`taskkill`、`node`、`go`、`*.test`、`council` 等）次次为 0。没有打开 `C:\Users\25142\.council` 下的任何文件，没有读任何 CLI 的配置与凭据，没有读写剪贴板，没有调用任何真 provider CLI（沙盒配置里的 `claude` 只是一个名字，几条命令都不起它），没有问任何模型；主仓库只用了 `git archive`、`git rev-parse` 与 `git status`。过程上一处偏离：只套生产代码的那一次运行给 `go test` 加了 `2>$null`（丢弃 stderr；不是 `2>&1`，只影响屏幕收集，结论用的是它打出的 `--- FAIL` 与 `ok` 行）。

**往返核对（v1）**：

- 写入：§1.1 的 15 对 `anchor` / `replace` 由脚本生成：锚点按整行从新解出的 HEAD 取（脚本核过各恰好一次），替换文本从原型取，同一文件的锚点互不重叠；§2 的三个测试文件由脚本从原型整文件取出写进本文；§4 的替换前后由脚本从起草清单写进本文，再从本文解析回来与清单逐条比过（18 条全同），最后一轮变异由这份解析结果驱动；耗时与杀手由脚本从最后一轮的结果文件写进本文，再从本文解析回来与结果文件比过（18 条全同：16 条有杀手，E1、E2 为 0）。
- 取回：再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在又一份新解出的 `483a766`（`git archive`，372 个文件）上：15 对锚点各恰好一次（在新解出的 HEAD 原文里与打补丁途中都是恰好一次），3 个整文件。整棵树 375 个文件与原型逐字节相同（0 个差异、两边没有多出的文件）；改到或新增的 11 个文件是 `commands.go`、`addname.go`、`addlogin.go`、`cxh_fixture_test.go`、`ux2r_add_test.go`、`killtree_windows.go`、`killtree_other.go`、`oby_loginstatus_exec_test.go` 与三个 `clx` 测试文件。`head\` 与这份新解出的 HEAD 全树逐字节相同（探针都删干净了）。
- 编码与符号：本文与这 11 个 Go 文件都是 UTF-8 无 BOM、只有 LF；符号禁令扫描（U+1F300-1FAFF、U+2700-27BF、U+2600-26FF、U+2B00-2BFF、U+2460-24FF、U+FE0F、U+200D）0 命中。
- 本文（v1）约 98 KB。

**泄漏核对（v1.1 修订期间，照 §4 表头）**：隔离目录 `E:\Isolating_test\council\cl1_v11\`：原型 `proto\`；HEAD 对照 `head\`（只读，没有放任何探针进去）；v1 字节对照 `v1ref\`（只比字节）；变异用的 `m1\`、`m2\`、`m3\`；往返核对用的 `rt\`（只比字节、没有跑 go）；沙盒 `sbx\`（原型构建的 `council.exe -no-tui` 跑 §6 第 5 步的五条命令一次，`COUNCIL_DATA_DIR` 与配置的 `data_dir` 都指到 `sbx\cl1check\data`，库建在那里）；空缓存编译计时用的 `probe_exe\`。每一次 go build / vet / test、每一批变异与那次沙盒运行，都在同一条 PowerShell 命令里先 dot-source 纯 ASCII 的 `goenv.ps1`（与 v1 相同的一组变量：`USERPROFILE`、`HOME`、`APPDATA`、`LOCALAPPDATA` 指到 `canaryhome` 及其下，`COUNCIL_DATA_DIR` 指到 `canarydata`，`TMP`、`TEMP` 指到 `tmp` 下的子目录，用 `SetEnvironmentVariable` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`、`CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`，显式给 `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE`（隔离目录下的 `gocache`）、`GOTOOLCHAIN=local`，`PATH` 收窄到 Go 与 System32，`SystemRoot`、`windir` 不动）；只有核「go test 会把 GOROOT/bin 排在 PATH 最前」的那一次在 dot-source 之后把 `PATH` 再收窄成 `C:\Windows\System32`、用全路径起 `go.exe`。每次 go test 之后、每批变异的每一条之后都核四项（canarydata 下没有 `council.db`；本次用的隔离 tmp 子目录、真 `%TEMP%`、`E:\Go\tmp` 下没有 `TestOb*`、`TestAdn*`、`TestClx*` 目录；`E:\council-data` 不存在；canaryhome 下没有 `.council`），次次干净；每批变异之后、最终字节上的全量运行与 `-count=3` 之后、沙盒运行之后核残留进程（开跑之后新起、还活着的 `cmd`、`taskkill`、`go`、`node`、`*.test`、`council`），次次为 0。没有打开 `C:\Users\25142\.council` 下的任何文件，没有读任何 CLI 的配置与凭据，没有读写剪贴板，没有调用任何真 provider CLI（沙盒配置里的 `claude` 只是一个名字，五条命令都不起它），没有问任何模型；主仓库只用了 `git archive`、`git rev-parse` 与 `git status`。原型里临时放过两个一次性探针（tui 的 NumLock 探针测试文件、打印 `GetSystemDirectory` 的小程序目录），跑完即删，往返核对证明原型最终字节里没有它们。

**往返核对（v1.1）**：

- 写入：§1.1 新增的三对锚点（A9、B8、B9）的锚点按整行从 HEAD 取、替换文本从原型取；§2 的三个测试文件由脚本从原型整文件写回本文；§4 新增的四条（V17、X1、X2、X4）的替换前后写进本节，22 条全部由驱动从本节解析出来跑；耗时与杀手由脚本从结果文件写进本节，再从本文解析回来与结果文件逐条比过（22 条全同：20 条有杀手，E1、E2 为 0），三个整文件也解析回来与原型逐字节比过。
- 取回：再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在又一份新解出的 `483a766`（`git archive`，372 个文件）上：18 对锚点各恰好一次（在没动过的 HEAD 原文里与打补丁途中都是恰好一次），3 个整文件。整棵树 375 个文件与原型逐字节相同（0 个差异、两边没有多出的文件）；相对 HEAD 改到或新增的 12 个文件是 `commands.go`、`addname.go`、`addlogin.go`、`cxh_fixture_test.go`、`ux2r_add_test.go`、`killtree_windows.go`、`killtree_other.go`、`oby_loginstatus_exec_test.go`、`obx_loginstatus_exec_test.go` 与三个 `clx` 测试文件。替换文本行数：块 A 生产代码 40 行、合计 42 行；块 B 生产代码 57 行、合计 74 行（与 D10、§5 一致）。
- 生产代码冻结块与 v1 比（脚本）：v1 的 10 个生产代码块在本文里锚点与替换文本都逐字节相同，新增的只有 A9。
- 编码与符号：本文与这 12 个 Go 文件都是 UTF-8 无 BOM、只有 LF；符号禁令扫描 0 命中。§6 第 1 步的 grep 核对在原型上成立：生产代码里带引号的 `"taskkill"` 0 处；`taskkillPath` 只在 `killtree_windows.go`、`killtree_other.go`；「把 agent 加进当前会话」恰好三处；「总结成员除外」恰好三处。
- 本文（v1.1）约 134 KB。
