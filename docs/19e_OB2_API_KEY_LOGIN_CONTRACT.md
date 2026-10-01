# OB2 冻结契约：API key 算已登录（CPA）与 OB1 审查余项（v1.1，2026-09-30）

> **归档说明（2026-09-30，主代理）**：本文是 OB2 的冻结契约，派工时的原件是 `E:\multi_model_cowork\ob2_contract.md`（v1.1），原样归档；锚点与行号指 HEAD `e60bceb`。派工时 HEAD 已是 `6bd6a3e`，两者之间只有文档提交，锚点在 `6bd6a3e` 上核过，各恰好一次。OB2 已于 `70025db` 落地，2026-09-30 装为 `council 0.2.0-0930b`，真机未复验。
> - **实现**：生产代码分两块，两路 grok-4.7 并行实现，每路只拿本块的契约摘录（块 A 25,661 B、块 B 17,150 B），与 §1.1 逐字节相同（5 个文件）。
> - **测试**：3 个 oby 测试文件由一条 Opus 测试腿写成，与 §2 逐字节相同。
> - **集成**：Opus 集成员没有做机械修正。§4 的 40 条杀手变异（4.1 的 27 条、4.2 的 13 条）全被点名杀手杀掉，N31、M59 核过是等价的。每次运行后都做四项泄漏核对，全部干净。
> - **审查**：Opus 独立审查报 3 条 minor（见下），都不是实现偏离。
>
> **落地后订正（2026-09-30，主代理；读本文之前先读这里）**：
> - **grok 还缺三行负对照，生产代码是对的**（审查，留给后续小刀）：
>   - 下面两种放宽全绿存活：CPA 那一句从「前缀加后缀」放宽成只看后缀；`You are using XAI_API_KEY.` 从整句相等放宽成 `HasSuffix`。这是**不安全的方向**：句前多了字也会说成「已登录（API key）」。v1.1 只给 deployment key 那一句加了句前多字的负对照（X2）。补法：K1 加 `grok_model_key_prefixed`（`oby Model 'grok-4.7' is using its own API key.`）与 `grok_xai_api_key_prefixed`（`oby You are using XAI_API_KEY.`），期望都是查不出。
>   - `You are not authenticated.` 从整句相等放宽成 `HasSuffix` 也存活。方向是安全的（多出来的会说成没登录）。补法同上，加 `grok_not_authenticated_prefixed`。
> - **一处冻结注释不准**（审查）：§2.2 `oby_firstrun_app_test.go` 文件头列的复用夹具，列了文件里没直接用的 `obxFake`，漏了直接用到的 `obxIn`、`obxEnv`。代码不受影响。
> - **§6 第 5 步第 0 条**说「主代理照惯例等 Griffin 说『装』」。这一刀 Griffin 事先说了「完成之后你直接安装就行」，所以落地后直接装了 `0.2.0-0930b`。
> - **集成员的实操提醒**：grok 块 A 把 `===FILE: internal/provider/login.go===` 接在开场白那一行的末尾，不在行首。按 `^===FILE:` 解析的落地脚本会漏掉这个文件。
> - **已由 OB3 补上（2026-09-30，`ef76eef`，契约 `19f`）**：
>   - §7 (a) 已做：`/add` 在识别结果之后多一段「各家的登录状态」，判据与首次运行相同。
>   - 上面「grok 还缺三行负对照」已补（19f 的 K3，`internal/provider/obz_login_test.go`）；「一处冻结注释不准」已改（19f 的 T1）。
> - **L5b 的写法已由 CL1 改**（2026-10-01，`5c4840a`，#50，`19h` 的 B5-B7）：不再把 PATH 设成空目录来模拟找不到 taskkill，改用接缝 `taskkillPath`；它对 N32 的杀伤力实测保住。本文 E7 与 §2.3 里「PATH 只有一个空目录」的说法以 19h 为准。
> - **§7 (g)「CPA 不通时」2026-10-01 真撞上了一次**：经 CPA 的 grok 调用被上游以 HTTP 426 拒绝（`00_DECISIONS.md` #50），这时首次运行与 `/add` 照样显示「已登录（API key）」。

**v1.1 变更记录**（2026-09-30，照主代理裁决书；审查 W1 的 F0-F3 全是 minor，全部接受；基线仍是 HEAD `e60bceb`；v1 另存快照）：

- 改动面：文件头「本文 v1 的全部期望值……」那一句（补上 v1.1 的实跑目录）、§2 的三个测试文件与 §2.1 的说明、§0 第 3 条、§4（表头一句与 §4.1 的表）、§6 第 3 步、§7 (d)、末尾的泄漏核对。§1.1 的冻结代码（F1、P1-P3、T1-T8）一个字节没改；§5 的行数不变（F1 仍是 202 行，两块合计仍约 250 行）。
- **F0**：§2.3 文件头注释「PATH 不改。」改成「PATH 只有 L5b 改（t.Setenv 成一个空目录，模拟找不到 taskkill），其余不改。」只改注释。
- **F1**：§2.2 B3 头注释里「十几秒」改成「远超 obyB3Limit（实测约 24.5 s）」。只改注释。
- **F2**：K1 加四行：`grok_xai_api_key_tail`（`You are using XAI_API_KEY. oby`）、`grok_deployment_key_prefixed`（`oby You are authenticated via deployment key.`）、`grok_not_authenticated_tail`（`You are not authenticated. oby`，19d 遗留的同类），期望都是查不出（最后一行不是没登录）；`grok_cpa_padded`（CPA 那一句首尾各带一个空格，19d 遗留的同类），期望已登录（API key）。§4.1 登记 X1、X2、X3、X11 四条变异（编号沿用审查 W1；与 19d 的 X1-X5 不是同一组），点名的杀手在原型上实测变红。加了这几行之后，N1 的杀手多了 `grok_cpa_padded`（实测）；§4.1、§4.2 里其余 login.go 的变异在 v1.1 原型上重跑，杀手与 v1 相同。
- **F3**：§0 第 3 条与 §7 (d) 补上 grok.exe 里另两句认证句与各自的偏移（v1.1 复核）；K1 加负对照 `grok_weblogin_deprecated`，期望查不出。
- F2、F3 新加的五行都只在新文件 `oby_login_test.go` 里，既有 obx 文件不改（T1-T8 之外仍一律不改）。
- v1.1 的实跑：隔离拷贝 `E:\Isolating_test\council\ob2_v11\`。先照 v1 的 §1.1 与 §2 把冻结代码逐字打在 `git archive e60bceb`（358 个文件）上，`go build ./...`、`go vet`（五个包）干净，`gofmt -l` 空，全量九包全绿（app 9.3 s、repl 75.2 s、tui 62.8 s、turn 14.8 s）；再由脚本从本文 v1.1 取出全部锚点与新文件，打在另一份新解出的 `e60bceb` 上作 v1.1 原型（11 对锚点各恰好一次、4 个整文件、361 个文件），与 v1 原型相比只差 §2 的三个测试文件。v1.1 原型上 build、vet 干净，`gofmt -l ./internal ./cmd` 空；K1 二十三行全绿；全量 `go test -count=1 ./...` 九包全绿（app 8.7 s、repl 73.9 s、tui 62.4 s、turn 14.2 s）；`-run 'TestOb[xy]'` 三个包 `-count=3` 全绿。变异：X1、X2、X3、X11 各自只红点名的那一行；§4.1 与 §4.2 里 login.go 的全部变异（N1-N10、N12-N17、M80-M84、M86-M89、M94）重跑，杀手除 N1 多一行外与 v1 相同；N23 重跑，B3 红，耗时 25.18 s。另跑一条不登记的探针：`grokLoggedInPrefix` 缩成 `You are ` 时 `grok_weblogin_deprecated` 变红（连同 `grok_logged_in_empty`），这一行不是空用例。firstrun.go、loginstatus_exec.go 的其余变异（N20-N22、N30、N32、N33、M39、M56、M58，等价的 N31、M59）没重跑：v1.1 没动生产代码，B3 与 turn 那个文件只改了注释。E12（只落块 A）没重跑，块 A、块 B 一个字节没变。最后由脚本从本文取出全部锚点与新文件，打在第三份新解出的 `e60bceb` 上：8 个文件与 v1.1 原型逐字节相同，整棵树 361 个文件一个字节不差。v1.1 没有跑任何真 CLI，只读了 grok.exe 的程序本体（§0 第 3 条）。原型不交付，用完已删除。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `e60bceb`（完整 `e60bcebf4232c8057d88b7af2614edced7d9e511`，= origin/main）。代码与 `3317940`（OB1 落地）相同，之后只有文档提交。Go 全路径 `"C:\Program Files\Go\bin\go.exe"`（go1.27.0 windows/amd64）。本机没有 gcc，不能 `-race`。E 盘机（Windows 11，PowerShell 5.1）。
- 本刀的 KEY 是 `OB2`；测试腿的包级标识符前缀是 `oby`（类型、函数、变量、常量一律 `oby` 开头，测试函数 `TestOby...`）。生产代码的新标识符不许以 `oby` 开头。
- 来源：
  - `docs/00_DECISIONS.md` **#48 末尾的「追加裁决（2026-09-30，Griffin）」**（`:2301-2308`）：主代理问「配了 API key 算不算已登录」，Griffin 答「支持CPA的那种吧」。配了 API key（包括 grok 经 CPA 走的 key）算正面状态，显示「已登录（API key）」一类，不再显示「登录状态查不出」，也不附登录办法。细节由主代理定（本文 §1.1，方向不改）。
  - `docs/19d_OB1_FIRST_RUN_CONTRACT.md` 文件头「落地后订正」里审查剩下的 R0、R1、R2、R4，与 §7 (b2)（v1.2 把原来的 (b2)、(b3)、(q) 并成的那一个问题）。
- **这推翻了 19d 的两处**：D15 里「grok 首行 `Model '<名字>' is using its own API key.` 不当正面信号」与「claude 带 `apiKeySource` 的落在查不出」；§0 第 14 条表里 grok 那一行的结论「这一行不是登录信号」。其余三档（已登录 / 没登录 / 登录状态查不出）的判据与说法不变。
- 本文 v1 的全部期望值都在隔离拷贝 `E:\Isolating_test\council\ob2_draft\proto\`（`git archive e60bceb` 解出）上用一份原型实现实跑得出（v1.1 加的五行与四条变异另在 `E:\Isolating_test\council\ob2_v11\` 上实跑，见文件头的 v1.1 变更记录）：
  - 原型下全量 `go test -count=1 ./...` 九包全绿；`-run 'TestOb[xy]'` 三个包 `-count=3` 全绿；`gofmt -l ./internal ./cmd` 空；`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn` 干净。
  - §4 的变异在原型上逐条实跑。
  - §1.1 的冻结代码、§2 的三个测试文件由脚本从原型取出写进本文，锚点由脚本从一份新解出的 HEAD 逐行取出、核过各恰好一次；再由脚本从本文取回、打在另一份新解出的 `e60bceb` 上，8 个文件与原型逐字节相同，整棵树 361 个文件与原型一个字节不差（§6 第 1 步照这个办法核）。只取块 A 打在第三份新解出的 `e60bceb` 上，`go build ./...`、`go vet` 干净，全量九包全绿（E12）。原型不交付，用完已删除。
  - 每一次 go build / vet / test / 变异运行都在同一条 PowerShell 命令里先设隔离环境（§4 表头），跑完核四项，次次干净（§7 末尾「泄漏核对」）。
- 证据等级照实写：「实测」是本机跑过的；「程序文本，未实跑」是只在 CLI 程序本体的字节里读到的句子或代码，本机没有那种登录态，规则也不许去造。

**与已落地各刀的相交**（逐个点名；本刀对它们的代码一个字不改，除非下表写了「改」或「授权改」）：

| 已落地 | 相交的文件 / 函数 | 本刀怎么用 |
|---|---|---|
| OB1 `3317940`（19d）| `internal/provider/login.go`（`LoginState`、`LoginInfo`、`claudeLogin`、`grokLogin`、`ParseLoginStatus`）| **整文件替换**（F1）：加第四档 `LoginAPIKey`、`LoginInfo.KeySource`、三家的新判据；`LoginFromDiscovery`、`LoginStatusCommandFor`、`LoginStatusExec`、`claudeAccountLine` 不改 |
| OB1 | `internal/app/firstrun.go` 的 `firstRunLoginText`、`firstRun` 里收登录办法那一处 | **改**（P1、P2）：新档的说法；已登录（API key）的不列登录办法 |
| OB1 | `internal/app/app.go` 的 `reportConfigError` 头注释（19d 的 P4 冻结文本）| **只改注释**（P3，审查 R2）：补上默认路径不是绝对路径（19d A14）那一种 |
| OB1 | `internal/turn/loginstatus_exec.go`（`RunLoginStatus`）| 不改。R0、R4 只补测试（§2.3） |
| OB1 测试 | `internal/provider/obx_login_test.go`（L1、L2）| **授权改五处**（T1-T5）：L1 的 `claude_api_key_env`、`claude_console_login`、`grok_api_key_line` 与 L2 的 `codex_logged_in_stdout_crlf` 期望翻成已登录（API key），外加 L1 的【判别力】注释。`obxClaudeInit` 只读复用 |
| OB1 测试 | `internal/app/obx_firstrun_app_test.go`（A15 与 §2.0 装置）| **授权改三处**（T6-T8）：A15 的 grok 那一行与登录办法、头注释。§2.0 装置（`obxIsolate`、`obxRun`、`obxTypeFake`、`obxCodexFake`、`obxFixture`、`obxSetTimeout`、`obxSub`、`obxWantFile`、`obxReadFile`、`obxIn`、`obxHead`、`obxLoginHead`、`obxDone`、`obxClaudeOut`、`obxAgyOut`、`obxCodexOut`、`obxEnv`）只读复用 |
| OB1 测试 | `internal/turn/obx_loginstatus_exec_test.go`（L4）| 不改。`obxStatusFake`、`obxLoopLines` 只读复用 |
| OB1 / MD1 | `internal/repl/*`（`/add` 识别表、`cliadvice.go`）| 不改：`/add` 的识别表不显示登录状态（§7 (a)） |
| TestMain | app 的在 `smoke_test.go`，turn 的在 `mdx_discovery_exec_test.go`；provider 包没有 | 不改、不另加 |

---

## 0. 动机 / 实测

1. **现状（HEAD `e60bceb`，核过真文件）**：
   - `internal/provider/login.go:106-112` 的 `grokLogin` 只认负面标记 `You are not authenticated.`，其余查不出；`:91-98` 的 `claudeLogin` 带 `apiKeySource` 的落在查不出（`:94` 要求它为空才算没登录）；`:131-141` 的 `ParseLoginStatus` 对 codex 以 `Logged in` 开头的一律已登录。
   - `internal/app/firstrun.go:73-85` 的 `firstRunLoginText` 三档；`:169` 把 `State != LoginYes` 的都收进登录办法。
   - 所以 Griffin 的 grok（经 CPA 走 API key）首次运行时显示「登录状态查不出」、并被建议 `grok login`（A15 用 `obxGrokKeyOut` 钉着这一行）。
2. **grok：本机实测**（`grok 1.0.41 (4220f3b224a6) [stable]`；`grok models`，stdin 为空，只去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`、`ANTHROPIC_*`；零花费）：

   | 环境 | 退出码 | stdout | 首行 | 临时 home 里建了什么 |
   |---|---|---|---|---|
   | 真环境 | 0 | 191 B，LF，0 个 CR，0.30 s | `Model 'grok-4.7' is using its own API key.` | 不适用 |
   | 只把 `USERPROFILE` 指到空目录 | 0 | 107 B，1.86 s | `You are not authenticated.` | `.grok`、`AppData` |
   | 只把 `HOME` 指到空目录 | 0 | 191 B | `Model 'grok-4.7' is using its own API key.` | 什么都没建 |
   | `USERPROFILE` 指空目录，`GROK_HOME=C:\Users\25142\.grok` | 0 | 191 B，0.28 s | `Model 'grok-4.7' is using its own API key.` | 什么都没建 |

   - 真环境的全部 stdout（夹具 `obyGrokCPAOut` 逐字用它）：`"Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n  - grok-4.6\n  - grok-4.5\n  - grok-4.3\n  - grok-composer-2.5-fast\n  - grok-build\n"`。与 19d §0 第 4、14 条记的首行、字节数相同。
   - 结论一：grok 按 `USERPROFILE` 找自己的目录，不看 `HOME`。所以 19d §6 第 5 步那种「改 `USERPROFILE` 模拟新用户」下 grok 一定是「没登录」，看不到「已登录（API key）」。
   - 结论二：`GROK_HOME` 指回真 grok 目录时，临时 `USERPROFILE` 下 grok 读的是真配置、首行是 CPA 那一句，临时 home 里没有建任何东西。§6 第 5 步给 Griffin 的模拟用这个办法。`GROK_HOME` 这个名字是在 grok.exe 的字节里找到的（下一条），只实测了这一种取值（指向 `.grok` 目录本身）；指向用户目录的取值可能让 grok 往真用户目录根下建东西，所以没有试。
3. **grok：程序文本，未实跑**（只读程序本体 `C:\Users\25142\.grok\bin\grok.exe`，154082120 B；没读 `~/.grok` 下的任何配置或凭据）。办法：node 把整个文件读进内存，按 ASCII 串 `indexOf` 全文搜，每个串各命中恰好 1 次。命中处是一张挨在一起的格式串表，紧跟着 `crates/codegen/xai-grok-pager/src/models.rs`，再往后是 `Default model: `、`Available models:`、`  - `、`  * `、` (default)` 这几段（`grok models` 的其余输出）：

   | 偏移 | 原文（占位符写成 `{}`） | 判据 |
   |---|---|---|
   | `0x853e9d2`（139717074）| `You are using XAI_API_KEY.` | 整句相等 → 已登录（API key） |
   | `0x853e9ee`（139717102）| `You are logged in with {}.` | 前缀 `You are logged in with `、后缀 `.`、中间非空 → 已登录 |
   | `0x853ea0b`（139717131）| `Model '{}' is using its own API key.` | 前缀 `Model '`、后缀 `' is using its own API key.`、中间非空 → 已登录（API key）；本机实测过 |
   | `0x853ea31`（139717169）| `You are authenticated via deployment key.` | 整句相等 → 已登录（API key） |
   | `0x853ea5b`（139717211）| `You are not authenticated.` | 整句相等 → 没登录（不变）；本机实测过 |

   - 这一段的原始字节（从 `0x853e9ce` 起）：`60010a00 596f7520...4b45592e 0a 17 596f7520617265206c6f6767656420696e207769746820 c002 2e0a 00 07 4d6f64656c2027 c01c 27206973...6b65792e 0a 00 596f75...6b65792e 0a 596f75...617465642e 0a`。每句都以 `.` 加 LF 结尾；**`You are using XAI_API_KEY.` 与 `You are authenticated via deployment key.` 的句尾都有句点**（任务书里写的两句没有句点，以字节为准）。
   - 占位符的解读是 [推断]：`0x17`（23）正好是 `You are logged in with ` 的长度、`0x07` 正好是 `Model '` 的长度，像是「长度前缀 + 字面段」，`c0 02`、`c0 1c` 是插参数的位置。`Model '{}'` 那一句本机实测的原文正好是这个形状（`{}` 是 `grok-4.7`），旁证了这个读法。`You are logged in with {}.` 里 `{}` 是什么（账号、邮箱）看不出，所以只要求非空、不上屏。
   - 五句都是整行（每句后面紧跟 LF），所以按「首行去掉首尾空白之后」判，与 19d 的 `You are not authenticated.` 同法。
   - **另有两句不在这张表里**（v1.1 补登，审查 W1 F3；同法复核，程序本体仍是 154082120 B，两句各命中恰好 1 次）：
     - `0x866f46d`（140964973）`You are authenticated via XAI_API_KEY (environment variable).`：前面是 `No cached session to log out of.`，后面是 `Logged out (was signed in as {})`、`Logged out`、`XAI_API_KEY is still set and will be used for aut…`，看上下文像是 `logout` 那一路的提示 [推断]。
     - `0x86ed829`（141482025）`You are using a deprecated authentication method (WebLogin).`：后面是 `This auth method is no longer supported and will cause errors.` 与一段叫人运行 `grok update` 的修复提示，看上下文像是启动时的警告 [推断]。
     - 两句会不会出现在 `grok models` 的首行，没实跑。它们不列入判据，按 E3 都落在查不出：前一句与 `You are using XAI_API_KEY.` 不相等，后一句不以 `You are logged in with ` 开头。K1 的 `grok_weblogin_deprecated` 钉住后一句查不出。
   - `grok.exe` 里另有 `GROK_HOME`、`GROK_CONFIG_PATH`、`GROK_DEPLOYMENT_KEY`、`GROK_CODE_XAI_API_KEY` 等环境变量名（按 `GROK_[A-Z_]+` 全文列出过），本刀只用到 `GROK_HOME`（第 2 条）。
4. **codex：程序文本，未实跑**（`codex-cli 0.147.0`；npm 全局目录 `@openai\codex` 包里真正的程序是 `node_modules\@openai\codex-win32-x64\vendor\x86_64-pc-windows-msvc\bin\codex.exe`，298668336 B；外层 `bin\codex.js`（7236 B）里搜 `login`、`Logged in` 零命中，它只负责起这个程序）。同法全文搜：`Logged in using` 共 5 处命中，全在下表这一段（段前是 `cli\src\login.rs`）；`Not logged in` 3 处命中，另两处是 MCP 状态界面的文字（`0xe31e102`、`0xe48fae8`），与 `login status` 无关：

   | 偏移 | 原文 | 判据（退出码 0 时） |
   |---|---|---|
   | `0xddb07d8`（232458200）| `Not logged in`（后接 `Successfully logged out`、`Error logging out: {}`）| 不变：退出码非 0 且整行相等 → 没登录 |
   | `0xddb0831`（232458289）| `Logged in using an API key - {}`（前面的长度字节 `0x1d` = 29 = `Logged in using an API key - ` 的长度）| 以 `Logged in using an API key` 开头 → 已登录（API key） |
   | 紧跟其后 | `Unexpected error retrieving API key: {}` | 不认（查不出） |
   | `0xddb087c`（232458364）| `Logged in using ChatGPT` | 已登录（不变）；本机实测过 |
   | `0xddb0908`（232458504）| `Logged in using access token` | 已登录（不变） |
   | `0xddb0925`（232458533）| `Logged in using personal access token` | 已登录（不变） |
   | `0xddb094b`（232458571）| `Logged in using Amazon Bedrock API key` | 整行相等 → 已登录（API key） |
   | 紧跟其后 | `Error checking login status: {}` | 不认（查不出） |

   - 本机实测（真登录态，零花费）：`codex login status` 退出码 0、stdout 空、stderr `Logged in using ChatGPT\n`，与 19d §0 第 14 条相同。API key 与 Bedrock 两句打在哪一路没实测；`ParseLoginStatus` 本来就两路都看。
5. **claude：程序文本，未实跑**（`2.1.283 (Claude Code)`；只读 npm 全局目录 `@anthropic-ai\claude-code\bin\claude.exe`，244960928 B；与 19d §0 第 20 条读的是同一个程序，这次复核了拼 `account` 的函数本身）：
   - `0xc242dd1`（203697617）拼 `account` 的函数（另有一份异步版在 `0xc244118`，写法相同）：`…r.tokenSource=n;else if(pt())r.subscription=hKn();else if(n!=="profile")r.tokenSource=n;let{key:s,source:g}=nf();if(s)r.apiKeySource=g;if(n==="claude.ai"||g==="/login managed key"){…organization…}…email…`。所以 `apiKeySource` 只在真有 key 时才写，值是 key 的来源；订阅那一支与 key 那一支互不排斥，两者可以同时出现。
   - `0xc10ddc0` initialize 回应里 `account` 的 schema：`email`、`organization`、`subscriptionType`、`tokenSource`、`apiKeySource` 都是可选字符串，`apiProvider` 是枚举。
   - `0xc1051ad` 另一个 schema（`Rq`，`0xc11f45d` 处 `system/init` 消息的 `apiKeySource` 用它）的枚举是 `ANTHROPIC_API_KEY`、`apiKeyHelper`、`/login managed key`、`none`、`user`、`project`、`org`、`temporary`、`oauth`，说明里把 `none` 写成「no API key in use」。`account` 那一支按上面的函数不会写出 `none`；判据仍把 `none` 排除在外（E2）。
   - `0x5fc7890` 一带是给人看的来源说法（`the ANTHROPIC_API_KEY environment variable`、`the apiKeyHelper setting`、`a Console API key saved by a previous /login`），本刀不用，屏幕显示 `apiKeySource` 的原值。
   - claude 的实测形状（19d §0 第 14 条）：登录态 `account` 有 `email`、`organization`、`subscriptionType`（`Claude Max`）、`apiProvider`，没有 `tokenSource`；`CLAUDE_CONFIG_DIR` 指空目录时只有 `tokenSource`（`none`）与 `apiProvider`。
6. **审查余项在原型上的实测**：
   - R0：未变异时 L5 0.63-0.75 s（`taskkill` 在 `PATH` 上）、L5b 0.30 s（`PATH` 只有空目录）；两行都删掉（N30）时 L5、L5b 都是 5.30 s（等满 `discoveryKillGrace` 5 s）；只删 `Kill` 那一行（N32）时 L5 0.63 s 照样过、L5b 5.30 s。
   - R1：未变异时 B3 2.03 s；`firstRunStatus` 改用父 ctx（N23）时 B3 24.51 s 才返回，而且 codex 那一行变成「已登录」（假 codex 数到上限之后打 `Logged in using ChatGPT`）。同一台机器上 N30 下 B3 是 7.03 s（`PATH` 里没有 taskkill、又没有 `Kill`，等满 5 s 宽限期），仍在 10 s 之内。
   - R4：测试进程的当前目录是包目录，与 `os.TempDir()`（隔离 `TMP`）不同；删掉 `cmd.Dir` 那一行（N33）时假 CLI 打出的是包目录。
7. **原型全量**：九包全绿（app 11.9 s、repl 73.8 s、tui 64.0 s、turn 16.7 s，其余几秒）。原型只套生产代码、不改任何既有测试时，全量只有这五个叶子用例红：`TestObxLoginFromDiscovery/claude_api_key_env`、`/claude_console_login`、`/grok_api_key_line`、`TestObxParseLoginStatus/codex_logged_in_stdout_crlf`、`TestObxFirstRunLoginStates`（A15）。T1-T8 正是改它们。
8. 没有调用任何会花钱或改登录态的命令。真 CLI 只跑过：`grok --version`、`codex --version`、`claude --version`、`grok models`（四种环境，见第 2 条）、`codex login status`。

---

## 1. 范围

### 1.1 做

#### 裁定（主代理定的细节，逐条冻结）

- **E1 第四档。** `provider.LoginState` 在 `LoginNo` 之后加 `LoginAPIKey`（值 3；前三档的值不变，零值仍是查不出）。`provider.LoginInfo` 加字段 `KeySource string`：只有 claude 已登录（API key）时非空，是 `account.apiKeySource` 的原值，不改写。屏幕说法（§3）：`KeySource` 非空时「已登录（API key：<原值>）」，否则「已登录（API key）」。与「已登录（订阅：<原值>）」同一个款式；不附登录办法。
- **E2 claude。** 识别那一次 initialize 回应里 `account` 的判据，按顺序取第一个成立的：
  1. `subscriptionType` 非空、不含控制字符、而且 `tokenSource` 不是 `none` → 已登录，带订阅原值（**不变**）；
  2. `apiKeySource` 非空、不是 `none`、不含控制字符 → 已登录（API key），带来源原值（**新**）；
  3. `tokenSource` 是 `none`、`subscriptionType` 与 `apiKeySource` 都为空 → 没登录（**不变**）；
  4. 其余查不出。
  - **订阅与 key 同时在时显示订阅**（第 1 支排在前面）。理由：裁决要求现有三档的判据不变，订阅用户本来就满足已登录那一支；把 key 排前面等于改了已登录的判据。程序文本（§0 第 5 条）证明两者能同时出现，但看不出 claude 这时按哪一个计费，登记 §7 (c)。
  - **`none` 不算 key**：同一个程序里描述来源的枚举把 `none` 说成没用 key；`account` 那一支按程序文本不会写出它，排除只是保守。`apiKeySource` 是 `none` 而 `tokenSource` 也是 `none` 时，第 3 支不成立（`apiKeySource` 不为空），落在查不出——与 19d 对「形状不认识」的处置一致。
  - **控制字符**：与订阅同一个理由（原值要上屏）。
  - 19d 的 L1 两行 `claude_api_key_env`、`claude_console_login` 从查不出翻成已登录（API key），来源分别是 `ANTHROPIC_API_KEY`、`/login managed key`（T2、T3）。
- **E3 grok。** 只看识别那一次 stdout 的首行，去掉首尾空白（与 19d 同）。按 §0 第 3 条的五句：
  - 不含占位符的三句**整句相等**：`You are not authenticated.` → 没登录（不变）；`You are using XAI_API_KEY.`、`You are authenticated via deployment key.` → 已登录（API key）。理由：程序文本里这三句是固定字面量，整句相等最严，CLI 改了措辞只会退回查不出。
  - 含占位符的两句**前缀加后缀、中间至少一个字节**：`Model '` … `' is using its own API key.` → 已登录（API key）；`You are logged in with ` … `.` → 已登录。理由：中间是变量（模型名、登录的内容），只能按两头认；要求中间非空，是为了挡住占位符为空这种认不出的形状。
  - grok 的已登录（API key）不带来源（`KeySource` 为空）：三句都不给来源值，`XAI_API_KEY` 是句子本身，不另拼。
  - 证据：`Model '…'` 那一句本机实测（经 CPA），其余三句正面句只在程序文本里见过（`You are logged in with …` 也一样），契约与代码注释都这样标。
  - 19d L1 的 `grok_api_key_line` 翻成已登录（API key）（T4）；A15 的 grok 那一行跟着翻，登录办法里去掉 grok（T7、T8）。
- **E4 codex。** `codex login status` 逐行（去掉首尾空白）看，每一行依次试三条，取第一行认得出的：
  1. 退出码 0、而且以 `Logged in using an API key` 开头，或整行恰好是 `Logged in using Amazon Bedrock API key` → 已登录（API key）（**新**）；
  2. 退出码 0、而且以 `Logged in` 开头 → 已登录（**不变**；`ChatGPT`、`access token`、`personal access token` 三句都落在这里）；
  3. 退出码非 0、而且整行是 `Not logged in` → 没登录（**不变**）。
  - 第 1 条必须排在第 2 条前面（同一行里先试），否则 API key 那两句先被第 2 条接走。
  - `Logged in using an API key` 按前缀认：程序文本里后面是 ` - ` 加一个占位符（多半是打码的 key，[推断]），那一段不上屏。前缀里不带 ` - `，是为了占位符为空时（去掉首尾空白之后行尾就是 `-`）仍认得出。
  - **Bedrock 算 API key**：句子本身写的就是 `API key`。它按整行相等认，后面多了字就落回第 2 条（已登录）。要不要算，列 §7 (b) 给 Griffin。
  - `access token`、`personal access token` 不算 API key：句子说的是 token，不是 key，照旧已登录。
  - 19d L2 的 `codex_logged_in_stdout_crlf`（`Logged in using an API key - sk-obx***`）翻成已登录（API key）（T5）。
- **E5 agy**：不变，查不出。
- **E6 登录办法**：`firstRun` 收登录办法的条件从「不是已登录」改成「既不是已登录、也不是已登录（API key）」（P2）。那一句「能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：」一个字不改：它只在有没登录或查不出的时候打，本来就不涉及已登录（API key）的那几家。找到的全是已登录或已登录（API key）时，这两段都不打（B2 钉）。
- **E7 R0（`RunLoginStatus` 超时杀进程那两行）**：新增 L5：同 19d L4 `killed` 的装置，外加「返回耗时小于 `discoveryKillGrace` 的一半」（2.5 s）；前提断言 `discoveryKillGrace/2` 大于 ctx 的 300 ms。再加 L5b：同 L5，但 `PATH` 只有一个空目录（`killProcessTree` 找不到 taskkill，只剩 `cmd.Process.Kill`），前提断言 `exec.LookPath("taskkill")` 失败。两行都删掉（N30）杀手是 L5、L5b；只删 `Kill`（N32）杀手是 L5b；只删 `killProcessTree`（N31）等价（§4）。
- **E8 R1（codex login status 受这一家的 `firstRunTimeout` 约束）**：新增 B3：只装假 codex，`debug models` 照常返回，`login status` 挂在一个有上限的等待循环里，数到上限（60000）之后**不退出，而是落到循环后面打 `Logged in using ChatGPT`、以 0 退出**。`firstRunTimeout` 设 2 s。期望 codex 那一行是「识别到 1 个模型；登录状态查不出」，`Run` 的耗时小于 10 s。`firstRunStatus` 改用父 ctx（N23）时，那一行变成「已登录」，耗时 24.5 s（§0 第 6 条），两条断言都红。这样杀手不只靠耗时。
- **E9 R4（`cmd.Dir = os.TempDir()`）**：新增 L6：假 CLI `@echo %CD%`，stdout 去掉末尾 `\r\n` 之后与 `os.TempDir()` 比（两边 `filepath.Clean`、`strings.EqualFold`）；前提断言测试进程自己的当前目录不是 `os.TempDir()`（否则删掉那一行也看不出来）。
- **E10 R2**：只改 `reportConfigError` 的头注释（P3），补上「默认路径不是绝对路径」那一种；代码不动。没有变异，只在审查时看。
- **E11 同类的别处**（逐个核过）：
  - `provider.LoginYes` 在生产代码里的用处只有 `firstrun.go` 两处（`firstRunLoginText`、收登录办法），都改了（P1、P2）；`login.go` 自己的几处在 F1 里。`/add` 的识别表不看登录状态（不改，§7 (a)）。
  - `firstRun` 头注释「找到的里面有没登录或查不出的，打一句说明和那几家的登录办法」、`firstRunCheck` 的注释仍然准确，不改。
  - 文档：`docs/README.md` 里「每家显示登录状态（已登录 / 没登录 / 登录状态查不出）」、19d 的 D15 与 §3.1 的四种说法、§7 (b2)，落地后由主代理订正（§6 第 4 步）。
  - `turn.RunDiscovery` 的杀进程写法与 `RunLoginStatus` 相同，它的超时由 MD1 的用例覆盖，本刀不碰。
- **E12 交付分块**：块 A（`internal/provider`：F1 与 T1-T5）只依赖 HEAD；块 B（`internal/app`：P1-P3 与 T6-T8）依赖块 A（用到 `provider.LoginAPIKey`）。只落块 A 时全量仍绿：`firstRunLoginText` 把新档当查不出、照旧列登录办法，正好是 HEAD 的 A15 期望（原型上核过）。

#### 块 A：`internal/provider`（一个整文件、五处授权的测试改动；可单独 `go build ./internal/provider`）

**F1. 整个替换 `internal/provider/login.go`**（逐字）

```file internal/provider/login.go
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

// login.go 是首次运行（00_DECISIONS.md #48，OB1 v1.1；OB2 加了「已登录（API key）」）显示的
// 登录状态。与本包其余文件同一条约定：不起进程、不读文件、不读环境变量。claude 与 grok 的
// 信号取自识别那一次本来就拿到的 stdout；codex 的识别输出里没有信号，另跑一次
// codex login status，由调用方注入的 LoginStatusExec 执行、把退出码与两路输出交回来判；
// agy 查不出。
//
// 证据等级（claude 2.1.283、grok 1.0.41、codex-cli 0.147.0）：
//   - 本机实测过的：claude 的 subscriptionType 与没登录的形状；grok 的
//     You are not authenticated. 与 Model '<模型>' is using its own API key.（本机经 CPA 的 grok
//     实测是这一行）；codex 的 Logged in using ChatGPT 与 Not logged in。
//   - 只在程序文本里见过、没实跑的：claude 的 apiKeySource；grok 的
//     You are using XAI_API_KEY.、You are authenticated via deployment key.、
//     You are logged in with <内容>.；codex 的 Logged in using an API key - <内容>、
//     Logged in using Amazon Bedrock API key。
//
// 只有正面信号才算已登录或已登录（API key），只有负面标记才算没登录，其余（字段缺失、
// 形状不认识、CLI 版本不同、识别超时）一律查不出。

// LoginState 是四档登录状态。零值是查不出。
type LoginState int

const (
	// LoginUnknown 是查不出：没有正面信号，也没有负面标记。
	LoginUnknown LoginState = iota
	// LoginYes 是已登录：有正面信号。
	LoginYes
	// LoginNo 是没登录：有负面标记。
	LoginNo
	// LoginAPIKey 是已登录（API key）：有「用 API key 一类的 key 认证」的正面信号（OB2）。
	LoginAPIKey
)

// LoginInfo 是一家的登录状态。Subscription 只有 claude 已登录时非空：initialize 回应里
// account.subscriptionType 的原值（本机实测是 Claude Max），不改写、不猜。KeySource 只有
// claude 已登录（API key）时非空：account.apiKeySource 的原值（例如 ANTHROPIC_API_KEY、
// /login managed key），同样不改写。
type LoginInfo struct {
	State        LoginState
	Subscription string
	KeySource    string
}

// LoginStatusExec 是跑登录状态命令的执行器。生产实现是 turn.RunLoginStatus。
// 进程自己以非 0 退出不算错误，退出码照实交回；起不来或被 ctx 杀掉时 err 非 nil。
type LoginStatusExec func(ctx context.Context, cmd DiscoveryCommand) (exitCode int, stdout, stderr []byte, err error)

// grok models 的 stdout 首行是一组互斥的认证状态句（grok 1.0.41 的程序文本里挨在一起，
// 见 OB2 契约 §0）。不含变量的三句整句相等；含变量的两句按前缀与后缀认，中间不能为空。
const (
	// grokNotAuthenticated 是没登录（实测）。
	grokNotAuthenticated = "You are not authenticated."
	// grokXAIAPIKey 按字面说的是在用 XAI_API_KEY（程序文本，未实跑）。
	grokXAIAPIKey = "You are using XAI_API_KEY."
	// grokDeploymentKey 按字面说的是用 deployment key 认证（程序文本，未实跑）。
	grokDeploymentKey = "You are authenticated via deployment key."
	// grokModelKeyPrefix、grokModelKeySuffix 夹着模型名：那个模型配了自己的 API key
	// （实测，本机经 CPA 的 grok 就是这一行）。
	grokModelKeyPrefix = "Model '"
	grokModelKeySuffix = "' is using its own API key."
	// grokLoggedInPrefix、grokLoggedInSuffix 夹着一段内容（程序文本里是占位符）：按字面说的是
	// 登录过（程序文本，未实跑）。
	grokLoggedInPrefix = "You are logged in with "
	grokLoggedInSuffix = "."
)

// codex login status 的 API key 登录（程序文本，未实跑）。第一句在程序文本里后面接
// 「 - 」与一个占位符，按前缀认，那一段不上屏；第二句整句相等。
const (
	codexAPIKeyPrefix  = "Logged in using an API key"
	codexBedrockAPIKey = "Logged in using Amazon Bedrock API key"
)

// LoginFromDiscovery 从识别那一次的 stdout 判登录状态。claude 看 initialize 回应的 account，
// grok 看 stdout 首行；其余返回查不出。stdout 为 nil（例如识别超时）也是查不出。
func LoginFromDiscovery(providerID string, stdout []byte) LoginInfo {
	switch providerID {
	case "claude":
		return claudeLogin(stdout)
	case "grok":
		return grokLogin(stdout)
	default:
		return LoginInfo{}
	}
}

// claudeAccountLine 只读 initialize 回应里 response.response.account 的三个键。
type claudeAccountLine struct {
	Response *struct {
		Response *struct {
			Account *struct {
				TokenSource      string `json:"tokenSource"`
				SubscriptionType string `json:"subscriptionType"`
				APIKeySource     string `json:"apiKeySource"`
			} `json:"account"`
		} `json:"response"`
	} `json:"response"`
}

// claudeLogin 取第一份 request_id 为 council-models 的 control_response（与 parseClaudeDiscovery
// 认的是同一行），按顺序取第一个成立的：
//  1. subscriptionType 非空、不含控制字符、而且 tokenSource 不是 none：已登录，订阅记原值。
//  2. apiKeySource 非空、不是 none、不含控制字符：已登录（API key），来源记原值。
//  3. tokenSource 是 none、subscriptionType 与 apiKeySource 都为空：没登录。
//  4. 其余查不出。
//
// 实测：登录态下 account 有 email、organization、subscriptionType、apiProvider，没有
// tokenSource；CLAUDE_CONFIG_DIR 指空目录时只有 tokenSource（none）与 apiProvider。
// apiKeySource 取自 claude 2.1.283 的程序文本，未实跑：拼 account 的函数只在有 key 时写它，
// 值是 key 的来源（ANTHROPIC_API_KEY、apiKeyHelper、/login managed key 等）；同一份程序里
// system/init 消息的 apiKeySource 枚举把 none 说成没有用 key，所以 none 不算。订阅排在前面：
// 订阅与 key 同时在时显示订阅，已登录那一档的判据不变。
func claudeLogin(stdout []byte) LoginInfo {
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		if _, ok := claudeDiscoveryResponse(line); !ok {
			continue
		}
		var v claudeAccountLine
		if err := json.Unmarshal(bytes.TrimSpace(line), &v); err != nil || v.Response == nil || v.Response.Response == nil || v.Response.Response.Account == nil {
			return LoginInfo{}
		}
		a := v.Response.Response.Account
		switch {
		case a.SubscriptionType != "" && !hasControlChar(a.SubscriptionType) && a.TokenSource != "none":
			return LoginInfo{State: LoginYes, Subscription: a.SubscriptionType}
		case a.APIKeySource != "" && a.APIKeySource != "none" && !hasControlChar(a.APIKeySource):
			return LoginInfo{State: LoginAPIKey, KeySource: a.APIKeySource}
		case a.TokenSource == "none" && a.SubscriptionType == "" && a.APIKeySource == "":
			return LoginInfo{State: LoginNo}
		default:
			return LoginInfo{}
		}
	}
	return LoginInfo{}
}

// grokLogin 只看 stdout 首行（去掉首尾空白）：You are not authenticated. 是没登录；
// You are using XAI_API_KEY.、You are authenticated via deployment key.、
// Model '<模型>' is using its own API key. 是已登录（API key）；You are logged in with <内容>.
// 是已登录；其余查不出。
func grokLogin(stdout []byte) LoginInfo {
	first, _, _ := strings.Cut(string(stdout), "\n")
	first = strings.TrimSpace(first)
	switch {
	case first == grokNotAuthenticated:
		return LoginInfo{State: LoginNo}
	case first == grokXAIAPIKey, first == grokDeploymentKey, wrapped(first, grokModelKeyPrefix, grokModelKeySuffix):
		return LoginInfo{State: LoginAPIKey}
	case wrapped(first, grokLoggedInPrefix, grokLoggedInSuffix):
		return LoginInfo{State: LoginYes}
	default:
		return LoginInfo{}
	}
}

// wrapped 报告 s 是否以 prefix 开头、以 suffix 结尾，而且两者之间至少还有一个字节。
func wrapped(s, prefix, suffix string) bool {
	return len(s) > len(prefix)+len(suffix) && strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix)
}

// LoginStatusCommandFor 给出要单独跑一次的登录状态命令。今天只有 codex：codex login status，
// stdin 为空。其余 ok 为假：claude、grok 用识别的输出判，agy 没有零花费的办法。
func LoginStatusCommandFor(providerID, path string) (DiscoveryCommand, bool) {
	if providerID == "codex" {
		return DiscoveryCommand{Path: path, Args: []string{"login", "status"}}, true
	}
	return DiscoveryCommand{}, false
}

// ParseLoginStatus 判登录状态命令的结果。codex：逐行（去掉首尾空白）看，取第一行认得出的。
// 退出码 0、而且以 Logged in using an API key 开头或恰好是 Logged in using Amazon Bedrock API key，
// 是已登录（API key）；退出码 0、而且以 Logged in 开头的其余各行，是已登录；退出码非 0、
// 而且恰好是 Not logged in，是没登录；其余查不出。实测两种都打在 stderr：
// Logged in using ChatGPT（退出码 0）、Not logged in（退出码 1）；API key 那两句是程序文本，
// 未实跑。其余 provider 一律查不出。
func ParseLoginStatus(providerID string, exitCode int, stdout, stderr []byte) LoginInfo {
	if providerID != "codex" {
		return LoginInfo{}
	}
	lines := strings.Split(string(stdout)+"\n"+string(stderr), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if exitCode == 0 && (strings.HasPrefix(l, codexAPIKeyPrefix) || l == codexBedrockAPIKey) {
			return LoginInfo{State: LoginAPIKey}
		}
		if exitCode == 0 && strings.HasPrefix(l, "Logged in") {
			return LoginInfo{State: LoginYes}
		}
		if exitCode != 0 && l == "Not logged in" {
			return LoginInfo{State: LoginNo}
		}
	}
	return LoginInfo{}
}
```

**T1. `internal/provider/obx_login_test.go`：L1 的【判别力】注释**

```anchor internal/provider/obx_login_test.go
// grok_api_key_line 对 M85；claude_api_key_env、claude_console_login 对 M94（claude_isolated_shape
// 是它们的正对照，只差有没有 apiKeySource）。
```

```replace
// grok_api_key_line 对 OB2 的 N1；claude_api_key_env、claude_console_login 对 OB2 的 N5（claude_isolated_shape
// 是它们的正对照，只差有没有 apiKeySource）。OB2 起这三行的期望是已登录（API key）。
```

**T2. 同一文件：L1 `claude_api_key_env` 的期望**

```anchor internal/provider/obx_login_test.go
		{"claude_api_key_env", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`), unk},
```

```replace
		{"claude_api_key_env", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`), LoginInfo{State: LoginAPIKey, KeySource: "ANTHROPIC_API_KEY"}},
```

**T3. 同一文件：L1 `claude_console_login` 的期望**

```anchor internal/provider/obx_login_test.go
		{"claude_console_login", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"/login managed key","organization":"obx","email":"obx@example.invalid","apiProvider":"firstParty"}`), unk},
```

```replace
		{"claude_console_login", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"/login managed key","organization":"obx","email":"obx@example.invalid","apiProvider":"firstParty"}`), LoginInfo{State: LoginAPIKey, KeySource: "/login managed key"}},
```

**T4. 同一文件：L1 `grok_api_key_line` 的期望**

```anchor internal/provider/obx_login_test.go
		{"grok_api_key_line", "grok", "Model 'grok-4.7' is using its own API key.\n\nAvailable models:\n  * grok-4.7 (default)\n", unk},
```

```replace
		{"grok_api_key_line", "grok", "Model 'grok-4.7' is using its own API key.\n\nAvailable models:\n  * grok-4.7 (default)\n", LoginInfo{State: LoginAPIKey}},
```

**T5. 同一文件：L2 `codex_logged_in_stdout_crlf` 的期望**

```anchor internal/provider/obx_login_test.go
		{"codex_logged_in_stdout_crlf", "codex", 0, "Logged in using an API key - sk-obx***\r\n", "", yes},
```

```replace
		{"codex_logged_in_stdout_crlf", "codex", 0, "Logged in using an API key - sk-obx***\r\n", "", LoginInfo{State: LoginAPIKey}},
```

#### 块 B：`internal/app`（三处补丁、三处授权的测试改动；依赖块 A）

**P1. `internal/app/firstrun.go`：`firstRunLoginText` 加第四档**

```anchor internal/app/firstrun.go
// firstRunLoginText 是结果行末尾的登录状态，三档之一；claude 已登录时带上订阅的原值。
func firstRunLoginText(l provider.LoginInfo) string {
	switch l.State {
	case provider.LoginYes:
		if l.Subscription != "" {
			return "已登录（订阅：" + l.Subscription + "）"
		}
		return "已登录"
```

```replace
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
```

**P2. 同一文件：已登录（API key）的不列登录办法**

```anchor internal/app/firstrun.go
		if row.login.State != provider.LoginYes {
			needLogin = append(needLogin, row.d.ID)
		}
```

```replace
		if row.login.State != provider.LoginYes && row.login.State != provider.LoginAPIKey {
			needLogin = append(needLogin, row.d.ID)
		}
```

**P3. `internal/app/app.go`：`reportConfigError` 头注释（审查 R2，只改注释）**

```anchor internal/app/app.go
// 路径是他指定的。默认位置找不到文件的情况，Run 已经先走过首次运行（#48）；
// 还能走到这里的只有竞态（首次运行建好或改用的文件，在 LoadAndValidate 之前
// 又没了），不再打印模板。
```

```replace
// 路径是他指定的。默认位置找不到文件的情况，Run 通常已经先走过首次运行（#48）；
// 还能走到这里的只有两种：竞态（首次运行建好或改用的文件，在 LoadAndValidate
// 之前又没了），与默认路径不是绝对路径（USERPROFILE 被设成了相对路径，Run 不进
// 首次运行，免得在启动目录下建文件）。两种都只打两行说明，不再打印模板。
```

**T6. `internal/app/obx_firstrun_app_test.go`：A15 的头注释**

```anchor internal/app/obx_firstrun_app_test.go
// TestObxFirstRunLoginStates 是 A15：四家都装，claude 没登录、grok 登录态那一行（查不出）、
// agy 找到时的查不出与兜底句、codex 已登录（不列登录办法）。
// 【判别力】M85（grok 的 API key 行算已登录）、M39、M89、M92、M56。
```

```replace
// TestObxFirstRunLoginStates 是 A15：四家都装，claude 没登录、grok 登录态那一行（OB2 起是已登录（API key），
// 不列登录办法）、agy 找到时的查不出与兜底句、codex 已登录（不列登录办法）。
// 【判别力】OB2 的 N1、N20、N21（grok 那一行与登录办法）、M39、M89、M92、M56。
```

**T7. 同一文件：A15 的 grok 那一行**

```anchor internal/app/obx_firstrun_app_test.go
		"  grok    <bin>\\grok.cmd  识别到 1 个模型；登录状态查不出\n"+
```

```replace
		"  grok    <bin>\\grok.cmd  识别到 1 个模型；已登录（API key）\n"+
```

**T8. 同一文件：A15 的登录办法（去掉 grok）**

```anchor internal/app/obx_firstrun_app_test.go
		obxLoginHead+"  claude  claude auth login\n  grok    grok login\n  agy     agy（按提示登录）\n"+obxDone)
```

```replace
		obxLoginHead+"  claude  claude auth login\n  agy     agy（按提示登录）\n"+obxDone)
```

锚点与替换的取法：每个代码块的内容是两道围栏之间的各行，以 LF 连接，末尾再加一个 LF；`file` 块同样（整文件就是块内容加一个 LF）。锚点由脚本从 HEAD `e60bceb` 真文件逐行取出，在该文件里恰好出现一次，同一文件的锚点互不重叠。

### 1.2 不做

- 不改 `/add` 的识别表（登录状态进 `/add` 是另一件事，§7 (a)）；不改 `internal/repl`、`internal/config`、`internal/tui`、`internal/store`、`cmd/council` 的任何文件。
- 不做 CPA 连通性检测（要读 grok 的配置，规则不许）；不读、不写任何 CLI 的配置或凭据；不跑 `login` / `logout`，不开浏览器。
- 不加新命令、新 flag；不改识别逻辑（`DiscoverModels`、各家解析、识别表格式、失败行）；不改 `firstRunCheck`、`firstRun` 的其余部分、`turn.RunLoginStatus`、`turn.RunDiscovery`。
- 不改 §3 以外的任何用户可见字面量；「能列出模型不代表已经登录……」那一句、四条登录办法、`msgFirstRunNoneFound` 都不改。
- **既有 `_test.go` 只许改 T1-T8**，其余一律不改（obx 的装置只读复用）。
- agy 不变。

---

## 2. 验收（测试腿新写的文件）

三个新文件：`internal/provider/oby_login_test.go`（`package provider`）、`internal/app/oby_firstrun_app_test.go`（`package app`）、`internal/turn/oby_loginstatus_exec_test.go`（`package turn`）。包级标识符一律 `oby` 开头；**不许新增 `TestMain`**；不许改任何既有测试文件（T1-T8 由实现腿交付）。三个文件照下面逐字写（§4 按名字点杀手）。

纪律（与 19d §2 开头相同）：
- 字面量整行相等：屏幕输出与文件字节整段 `==` 比较。
- 每条都能为假：每条用例写明它对哪条变异红（§4）；缺席断言配同一装置的正对照（B2 的「不打登录说明」以 B1 为正对照，只差装不装 agy）；时间断言配前提断言，保证变异下看得见（L5、L5b、L6）。
- 真用户目录零写入：app 包的用例一律经 `obxIsolate`（第一件事是 `obxGuardRealHome`），另起 goroutine 调 `Run` 的一律经 `obxRun`。
- 假 CLI 一律是临时目录里只用 cmd 内建命令的 `.cmd`，不调用任何真 CLI；不设 `COUNCIL_FAKE_CLI`。

### 2.1 `internal/provider/oby_login_test.go`

- **K1 `TestObyLoginFromDiscovery`**：表驱动，`LoginFromDiscovery(id, []byte(stdout)) == want`。claude 六行（`apiKeyHelper`、没有 `tokenSource` 只有 key、来源是 `none`、来源含控制字符、来源不是字符串、订阅与 key 同时在）；grok 十七行（本机实测的 CPA 全文、CRLF、首尾带空格、模型名为空、少句点、`XAI_API_KEY` 与少句点、句后多字、deployment key 与句后多字、句前多字、`logged in with` 与内容为空、少句点、不在首行、`You are not authenticated.` 句后多字、程序文本里的 WebLogin 过时句）；其中首尾带空格、`XAI_API_KEY` 句后多字、deployment key 句前多字、`You are not authenticated.` 句后多字、WebLogin 这五行是 v1.1 加的（审查 W1 F2、F3）。`claude_key_control_char` 那一行的 `\u0007` 是 JSON 转义（Go 原始字符串里的六个 ASCII 字符），不是真的控制字符。
- **K2 `TestObyParseLoginStatus`**：表驱动，`ParseLoginStatus(id, code, []byte(stdout), []byte(stderr)) == want`。codex 七行（API key 句、同一句但退出码 1、Bedrock 整句、Bedrock 句后多字、`ChatGPT`、`access token`、`personal access token`），外加 claude 一行（查不出）。

```file internal/provider/oby_login_test.go
package provider

import "testing"

// oby_login_test.go 是 OB2（00_DECISIONS.md #48 追加裁决：配了 API key 算已登录）在 provider 包的验收：
// K1 从识别输出判出已登录（API key）与 grok 的已登录，K2 从 codex login status 判出已登录（API key）。
// 拼 claude 那一行复用 obx_login_test.go 的 obxClaudeInit（只读复用）。期望值逐字取自契约。

// obyGrokCPAOut 是本机实测的 grok models 全部 stdout（grok 1.0.41，经 CPA 走 API key；191 字节，LF）。
const obyGrokCPAOut = "Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n  - grok-4.6\n  - grok-4.5\n  - grok-4.3\n  - grok-composer-2.5-fast\n  - grok-build\n"

// TestObyLoginFromDiscovery 是 K1。
// 【判别力】见契约 §4：grok_cpa_real、grok_cpa_padded 对 N1；grok_xai_api_key 对 N2；grok_deployment_key 对 N3；
// grok_logged_in 对 N4；claude_key_helper、claude_key_no_token_source 对 N5、N9；claude_key_none 对 N6、M94；
// claude_key_control_char 对 N7、M94；claude_sub_and_key 对 N8；grok_logged_in_empty、grok_model_key_empty_name 对 N10；
// grok_xai_api_key_tail 对 X1；grok_deployment_key_prefixed 对 X2；grok_not_authenticated_tail 对 X3；
// grok_cpa_padded 对 X11（grok_cpa_crlf 只多 CR，X11 下不红）。
// 期望查不出的各行是负对照：少句点、名字为空、句后多字、句前多字、不在首行、来源是 none 或含控制字符、类型不对；
// grok_weblogin_deprecated 是程序文本里另一句认证句（契约 §0 第 3 条），不列入判据。
func TestObyLoginFromDiscovery(t *testing.T) {
	key := func(src string) LoginInfo { return LoginInfo{State: LoginAPIKey, KeySource: src} }
	unk := LoginInfo{}
	for _, c := range []struct {
		name, id, stdout string
		want             LoginInfo
	}{
		{"claude_key_helper", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"apiKeyHelper","apiProvider":"firstParty"}`), key("apiKeyHelper")},
		{"claude_key_no_token_source", "claude", obxClaudeInit("council-models", `{"apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`), key("ANTHROPIC_API_KEY")},
		{"claude_key_none", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"none","apiProvider":"firstParty"}`), unk},
		{"claude_key_control_char", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"k\u0007","apiProvider":"firstParty"}`), unk},
		{"claude_key_not_string", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":7}`), unk},
		{"claude_sub_and_key", "claude", obxClaudeInit("council-models", `{"subscriptionType":"Claude Max","apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`), LoginInfo{State: LoginYes, Subscription: "Claude Max"}},
		{"grok_cpa_real", "grok", obyGrokCPAOut, key("")},
		{"grok_cpa_crlf", "grok", "Model 'grok-4.7' is using its own API key.\r\n\r\nAvailable models:\r\n", key("")},
		{"grok_cpa_padded", "grok", " Model 'grok-4.7' is using its own API key. \n\nAvailable models:\n", key("")},
		{"grok_model_key_empty_name", "grok", "Model '' is using its own API key.\n", unk},
		{"grok_model_key_no_period", "grok", "Model 'grok-4.7' is using its own API key\n", unk},
		{"grok_xai_api_key", "grok", "You are using XAI_API_KEY.\n\nDefault model: grok-oby\n\nAvailable models:\n  * grok-oby (default)\n", key("")},
		{"grok_xai_api_key_no_period", "grok", "You are using XAI_API_KEY\n", unk},
		{"grok_xai_api_key_tail", "grok", "You are using XAI_API_KEY. oby\n", unk},
		{"grok_deployment_key", "grok", "You are authenticated via deployment key.\n\nDefault model: grok-oby\n", key("")},
		{"grok_deployment_key_tail", "grok", "You are authenticated via deployment key. oby\n", unk},
		{"grok_deployment_key_prefixed", "grok", "oby You are authenticated via deployment key.\n", unk},
		{"grok_logged_in", "grok", "You are logged in with oby@example.invalid.\n\nDefault model: grok-oby\n", LoginInfo{State: LoginYes}},
		{"grok_logged_in_empty", "grok", "You are logged in with .\n", unk},
		{"grok_logged_in_no_period", "grok", "You are logged in with oby@example.invalid\n", unk},
		{"grok_key_not_first", "grok", "Default model: grok-oby\nYou are using XAI_API_KEY.\n", unk},
		{"grok_not_authenticated_tail", "grok", "You are not authenticated. oby\n", unk},
		{"grok_weblogin_deprecated", "grok", "You are using a deprecated authentication method (WebLogin).\n", unk},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := LoginFromDiscovery(c.id, []byte(c.stdout)); got != c.want {
				t.Errorf("LoginFromDiscovery(%s, %q) = %+v，期望 %+v", c.id, c.stdout, got, c.want)
			}
		})
	}
}

// TestObyParseLoginStatus 是 K2。
// 【判别力】codex_api_key_stderr 对 N12、N17；codex_api_key_exit1 对 N14；codex_bedrock 对 N13；
// codex_bedrock_tail 对 N16。codex_chatgpt_real、codex_access_token、codex_personal_access_token 钉住
// 「其余以 Logged in 开头的照旧是已登录」。
func TestObyParseLoginStatus(t *testing.T) {
	key := LoginInfo{State: LoginAPIKey}
	yes := LoginInfo{State: LoginYes}
	unk := LoginInfo{}
	for _, c := range []struct {
		name, id       string
		code           int
		stdout, stderr string
		want           LoginInfo
	}{
		{"codex_api_key_stderr", "codex", 0, "", "Logged in using an API key - sk-oby***\n", key},
		{"codex_api_key_exit1", "codex", 1, "", "Logged in using an API key - sk-oby***\n", unk},
		{"codex_bedrock", "codex", 0, "", "Logged in using Amazon Bedrock API key\n", key},
		{"codex_bedrock_tail", "codex", 0, "", "Logged in using Amazon Bedrock API key (oby)\n", yes},
		{"codex_chatgpt_real", "codex", 0, "", "Logged in using ChatGPT\n", yes},
		{"codex_access_token", "codex", 0, "", "Logged in using access token\n", yes},
		{"codex_personal_access_token", "codex", 0, "", "Logged in using personal access token\n", yes},
		{"claude_api_key_line", "claude", 0, "", "Logged in using an API key - sk-oby***\n", unk},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseLoginStatus(c.id, c.code, []byte(c.stdout), []byte(c.stderr)); got != c.want {
				t.Errorf("ParseLoginStatus(%s, %d, %q, %q) = %+v，期望 %+v", c.id, c.code, c.stdout, c.stderr, got, c.want)
			}
		})
	}
}
```

### 2.2 `internal/app/oby_firstrun_app_test.go`

- **B0 `TestObyFirstRunLoginText`**：`firstRunLoginText` 的六种输入各一个子测试，逐字比说法（§3）。
- **B1 `TestObyFirstRunAPIKey`**：四家都装：claude 的 `account` 换成 `tokenSource` 为 `none`、`apiKeySource` 为 `ANTHROPIC_API_KEY`（由 `obxClaudeOut` 替换一处得到，前提断言那一处恰好一次）；grok 是本机实测的 CPA 全文；agy 是 `obxAgyOut`；codex 的 `login status` 打 `Logged in using an API key - sk-oby***`。期望 errOut 整段相等（下面代码里的 `want`），登录说明只列 agy；配置文件字节等于 `obxWantFile(e, "claude", "grok", "agy", "codex")`。
- **B2 `TestObyFirstRunAPIKeyAllIn`**：同 B1 但不装 agy。期望 errOut 整段相等，没有登录说明与登录办法。
- **B3 `TestObyFirstRunCodexStatusTimeout`**：E8。期望 errOut 整段相等、`Run` 的耗时小于 `obyB3Limit`（10 s）。
- 三条端到端的 errOut 期望（`obxSub` 换掉 `<home>`、`<bin>` 之前）：

```text
B1:
第一次运行：还没有配置文件 <home>\.council\config.toml，council 来生成一份。
正在检查 claude、grok、agy、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……
  claude  <bin>\claude.cmd  识别到 2 个模型；已登录（API key：ANTHROPIC_API_KEY）
  grok    <bin>\grok.cmd  识别到 6 个模型；已登录（API key）
  agy     <bin>\agy.cmd  识别到 1 个模型；登录状态查不出
  codex   <bin>\codex.cmd  识别到 1 个模型；已登录（API key）
能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：
  agy     agy（按提示登录）
已生成配置文件: <home>\.council\config.toml（只写了找到的工具，还没有成员）
上手三步：
  1. /new <标题>  开一个会话
  2. /add  看识别出的模型，再用 /add +编号 挑成员
  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个

B2（与 B1 只差：「正在检查」里没有 agy、agy 那一行是「  agy     没找到」、没有登录说明与登录办法）:
第一次运行：还没有配置文件 <home>\.council\config.toml，council 来生成一份。
正在检查 claude、grok、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……
  claude  <bin>\claude.cmd  识别到 2 个模型；已登录（API key：ANTHROPIC_API_KEY）
  grok    <bin>\grok.cmd  识别到 6 个模型；已登录（API key）
  agy     没找到
  codex   <bin>\codex.cmd  识别到 1 个模型；已登录（API key）
已生成配置文件: <home>\.council\config.toml（只写了找到的工具，还没有成员）
上手三步：
  1. /new <标题>  开一个会话
  2. /add  看识别出的模型，再用 /add +编号 挑成员
  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个

B3:
第一次运行：还没有配置文件 <home>\.council\config.toml，council 来生成一份。
正在检查 codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 2s）……
  claude  没找到
  grok    没找到
  agy     没找到
  codex   <bin>\codex.cmd  识别到 1 个模型；登录状态查不出
能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：
  codex   codex login
已生成配置文件: <home>\.council\config.toml（只写了找到的工具，还没有成员）
上手三步：
  1. /new <标题>  开一个会话
  2. /add  看识别出的模型，再用 /add +编号 挑成员
  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个
```

  三段各自末尾还有一个空行（`obxDone` 的最后一个 `\n`）。grok 那一行的「识别到 6 个模型」来自实测全文里的六个模型（`grok-4.7`、`grok-4.6`、`grok-4.5`、`grok-4.3`、`grok-composer-2.5-fast`、`grok-build`）。

```file internal/app/oby_firstrun_app_test.go
package app

// oby_firstrun_app_test.go 是 OB2（00_DECISIONS.md #48 追加裁决：配了 API key 算已登录）在 app 包的验收：
// B0 登录状态的四档说法，B1、B2 首次运行端到端（已登录（API key）那几行不附登录办法），
// B3 补 OB1 审查余项 R1（codex login status 受这一家的 firstRunTimeout 约束）。
// 装置与夹具只读复用 obx_firstrun_app_test.go 的 §2.0（obxIsolate、obxRun、obxTypeFake、obxCodexFake、
// obxFixture、obxFake、obxSetTimeout、obxSub、obxWantFile、obxReadFile、obxHead、obxLoginHead、obxDone、
// obxClaudeOut、obxAgyOut、obxCodexOut）；纪律同契约 19d §2 开头。

import (
	"context"
	"strings"
	"testing"
	"time"

	"council/internal/provider"
)

// obyGrokCPAOut 是本机实测的 grok models 全部 stdout（grok 1.0.41，经 CPA 走 API key；191 字节，LF）。
const obyGrokCPAOut = "Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n  - grok-4.6\n  - grok-4.5\n  - grok-4.3\n  - grok-composer-2.5-fast\n  - grok-build\n"

// obyClaudeAccountOld、obyClaudeAccountKey：把 obxClaudeOut 的 account 换成用 ANTHROPIC_API_KEY 的形状。
const (
	obyClaudeAccountOld = `"account":{"apiProvider":"firstParty","subscriptionType":"Claude Obx"}`
	obyClaudeAccountKey = `"account":{"tokenSource":"none","apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`
)

// obyCodexKeyIn 是 codex login status 用 API key 登录时的输出（程序文本的句子，key 那一段是假的）。
var obyCodexKeyIn = []string{"@echo Logged in using an API key - sk-oby*** 1>&2", "@exit /b 0"}

// obyKeyFakes 装 claude（API key）、grok（本机实测的 CPA 输出）、codex（API key）；withAgy 为真时再装 agy。
func obyKeyFakes(t *testing.T, e *obxEnv, withAgy bool) {
	t.Helper()
	if strings.Count(obxClaudeOut, obyClaudeAccountOld) != 1 {
		t.Fatalf("前提不成立：obxClaudeOut 里的 account 不是恰好一处 %s", obyClaudeAccountOld)
	}
	obxTypeFake(t, e, "claude", strings.Replace(obxClaudeOut, obyClaudeAccountOld, obyClaudeAccountKey, 1))
	obxTypeFake(t, e, "grok", obyGrokCPAOut)
	if withAgy {
		obxTypeFake(t, e, "agy", obxAgyOut)
	}
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, obyCodexKeyIn)
}

// obyRowClaude、obyRowGrok、obyRowCodex 是 B1、B2 里 claude、grok、codex 三行（agy 那一行各自给）。
const (
	obyRowClaude = "  claude  <bin>\\claude.cmd  识别到 2 个模型；已登录（API key：ANTHROPIC_API_KEY）\n"
	obyRowGrok   = "  grok    <bin>\\grok.cmd  识别到 6 个模型；已登录（API key）\n"
	obyRowCodex  = "  codex   <bin>\\codex.cmd  识别到 1 个模型；已登录（API key）\n"
)

// TestObyFirstRunLoginText 是 B0：firstRunLoginText 的四档六种说法。
// 【判别力】N21（API key 那一档落到查不出）、N22（不带来源）。
func TestObyFirstRunLoginText(t *testing.T) {
	for _, c := range []struct {
		name string
		in   provider.LoginInfo
		want string
	}{
		{"yes_sub", provider.LoginInfo{State: provider.LoginYes, Subscription: "Claude Oby"}, "已登录（订阅：Claude Oby）"},
		{"yes", provider.LoginInfo{State: provider.LoginYes}, "已登录"},
		{"key_source", provider.LoginInfo{State: provider.LoginAPIKey, KeySource: "/login managed key"}, "已登录（API key：/login managed key）"},
		{"key", provider.LoginInfo{State: provider.LoginAPIKey}, "已登录（API key）"},
		{"no", provider.LoginInfo{State: provider.LoginNo}, "没登录"},
		{"unknown", provider.LoginInfo{}, "登录状态查不出"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := firstRunLoginText(c.in); got != c.want {
				t.Errorf("firstRunLoginText(%+v) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

// TestObyFirstRunAPIKey 是 B1：四家都装，claude、grok、codex 都是已登录（API key），agy 查不出。
// 登录说明照打，但只列 agy。与 B2 只差装不装 agy，是 B2「登录说明不打」的正对照。
// 【判别力】N1、N5、N9、N12、N20、N21、N22。
func TestObyFirstRunAPIKey(t *testing.T) {
	e := obxIsolate(t)
	obyKeyFakes(t, e, true)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 claude、grok、agy、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"+
		obyRowClaude+obyRowGrok+
		"  agy     <bin>\\agy.cmd  识别到 1 个模型；登录状态查不出\n"+
		obyRowCodex+
		obxLoginHead+"  agy     agy（按提示登录）\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if file, wantFile := obxReadFile(t, e.cfgPath), obxWantFile(e, "claude", "grok", "agy", "codex"); file != wantFile {
		t.Errorf("配置文件字节不等\n实际 %q\n期望 %q", file, wantFile)
	}
}

// TestObyFirstRunAPIKeyAllIn 是 B2：同 B1 但不装 agy。找到的全是已登录（API key），登录说明与登录办法都不打。
// 【判别力】N20（已登录（API key）的也列登录办法）、N1、N5、N12、N21、N22。
func TestObyFirstRunAPIKeyAllIn(t *testing.T) {
	e := obxIsolate(t)
	obyKeyFakes(t, e, false)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 claude、grok、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"+
		obyRowClaude+obyRowGrok+
		"  agy     没找到\n"+
		obyRowCodex+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if strings.Contains(errOut, obxLoginHead) {
		t.Errorf("找到的全都已登录（API key）时不该打登录说明，errOut %q", errOut)
	}
}

// obyStatusLimit 是 B3 假 codex 等待循环的计数上限，与 obxLoopLimit 同值；数到上限之后不退出，
// 而是落到循环后面那两行（打 Logged in using ChatGPT、以 0 退出），这样没被杀掉时结果看得见。
const obyStatusLimit = "60000"

// TestObyFirstRunCodexStatusTimeout 是 B3（R1）：假 codex 的 debug models 照常返回，login status 挂在
// 等待循环里。firstRunTimeout 是 2 s：这一家的 ctx 到期时 login status 被杀掉，codex 那一行是登录状态查不出，
// Run 在 obyB3Limit 之内返回。firstRunStatus 改用父 ctx 时，循环数到上限、打出 Logged in using ChatGPT，
// 那一行变成已登录，而且 Run 要等远超 obyB3Limit（实测约 24.5 s）。
// 【判别力】N23。
func TestObyFirstRunCodexStatusTimeout(t *testing.T) {
	e := obxIsolate(t)
	obxSetTimeout(t, 2*time.Second)
	loopOn := obxFixture(t, e, "loop.on", "x")
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, []string{
		"@echo off", "set n=0", ":obyloop", "set /a n+=1",
		"if %n% geq " + obyStatusLimit + " goto obydone",
		`if exist "` + loopOn + `" goto obyloop`,
		":obydone", "echo Logged in using ChatGPT 1>&2", "exit /b 0",
	})

	start := time.Now()
	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	el := time.Since(start)
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 2s）……\n"+
		"  claude  没找到\n  grok    没找到\n  agy     没找到\n"+
		"  codex   <bin>\\codex.cmd  识别到 1 个模型；登录状态查不出\n"+
		obxLoginHead+"  codex   codex login\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if el >= obyB3Limit {
		t.Errorf("Run 用了 %v 才返回，应当小于 %v（login status 受这一家的 firstRunTimeout 约束）", el, obyB3Limit)
	}
}

// obyB3Limit 是 B3 对 Run 耗时的上限：firstRunTimeout（2 s）加上起进程、杀进程与 REPL 的余量。
const obyB3Limit = 10 * time.Second
```

### 2.3 `internal/turn/oby_loginstatus_exec_test.go`

- **L5 `TestObyRunLoginStatusKilledPromptly`**（R0）、**L5b `TestObyRunLoginStatusKilledWithoutTaskkill`**（R0，`PATH` 只有空目录）、**L6 `TestObyRunLoginStatusDir`**（R4）：见 E7、E9。

```file internal/turn/oby_loginstatus_exec_test.go
package turn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/provider"
)

// oby_loginstatus_exec_test.go 是 OB2（契约 §2.3）补 OB1 审查余项 R0、R4 在 turn 包的验收：
// RunLoginStatus 被 ctx 杀掉时及时返回（杀进程那两行起作用），子进程的工作目录是 os.TempDir()。
// 假 CLI 复用 obx_loginstatus_exec_test.go 的 obxStatusFake、obxLoopLines（只读复用）。PATH 只有 L5b 改（t.Setenv 成一个空目录，模拟找不到 taskkill），其余不改。

// TestObyRunLoginStatusKilledPromptly 是 L5（R0）：ctx 300 ms 到期，RunLoginStatus 必须在
// discoveryKillGrace 的一半之内返回。杀进程那两行都删掉时，它要等满 discoveryKillGrace 才返回。
// 【判别力】N30。只删 killProcessTree 那一行（N31）是等价变异：Kill 照样杀得掉没有孙进程的 cmd 循环；
// 只删 Kill 那一行（N32）在这里看不出来（taskkill 在 PATH 上），由 L5b 杀。
func TestObyRunLoginStatusKilledPromptly(t *testing.T) {
	if discoveryKillGrace/2 <= 300*time.Millisecond {
		t.Fatalf("前提不成立：discoveryKillGrace/2 = %v 不大于 ctx 的 300 ms，断言分不开杀没杀", discoveryKillGrace/2)
	}
	dir := t.TempDir()
	on := filepath.Join(dir, "loop.on")
	if err := os.WriteFile(on, []byte("x"), 0o644); err != nil {
		t.Fatalf("写 loop.on: %v", err)
	}
	p := obxStatusFake(t, obxLoopLines(`exist "`+on+`"`)...)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, _, err := RunLoginStatus(ctx, provider.DiscoveryCommand{Path: p, Args: []string{"login", "status"}})
	el := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("被 ctx 杀掉时 err 应当满足 errors.Is(err, context.DeadlineExceeded)，实际 %v", err)
	}
	if el >= discoveryKillGrace/2 {
		t.Errorf("RunLoginStatus 用了 %v 才返回，应当小于 %v（杀进程之后不该等满宽限期）", el, discoveryKillGrace/2)
	}
}

// TestObyRunLoginStatusKilledWithoutTaskkill 是 L5b（R0）：同 L5，但 PATH 只有一个空目录，
// killProcessTree 找不到 taskkill（契约 19d §0 第 9 条），只剩 cmd.Process.Kill 那一行在杀。
// 前提：这个 PATH 下 exec.LookPath("taskkill") 必须失败。
// 【判别力】N30、N32（只删 Kill 那一行：L5 里 taskkill 还在，看不出来）。
func TestObyRunLoginStatusKilledWithoutTaskkill(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("taskkill"); err == nil {
		t.Fatalf("前提不成立：PATH 只有空目录时仍找得到 taskkill")
	}
	dir := t.TempDir()
	on := filepath.Join(dir, "loop.on")
	if err := os.WriteFile(on, []byte("x"), 0o644); err != nil {
		t.Fatalf("写 loop.on: %v", err)
	}
	p := obxStatusFake(t, obxLoopLines(`exist "`+on+`"`)...)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, _, err := RunLoginStatus(ctx, provider.DiscoveryCommand{Path: p, Args: []string{"login", "status"}})
	el := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("被 ctx 杀掉时 err 应当满足 errors.Is(err, context.DeadlineExceeded)，实际 %v", err)
	}
	if el >= discoveryKillGrace/2 {
		t.Errorf("RunLoginStatus 用了 %v 才返回，应当小于 %v（找不到 taskkill 时也要靠 Kill 杀掉）", el, discoveryKillGrace/2)
	}
}

// TestObyRunLoginStatusDir 是 L6（R4）：假 CLI 打出当前目录，应当是 os.TempDir()。
// 前提：测试进程自己的当前目录不是 os.TempDir()，否则删掉 cmd.Dir 那一行也看不出来。
// 【判别力】N33。
func TestObyRunLoginStatusDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("前提不成立：取不到当前目录：%v", err)
	}
	tmp := filepath.Clean(os.TempDir())
	if strings.EqualFold(filepath.Clean(wd), tmp) {
		t.Fatalf("前提不成立：测试进程的当前目录 %s 就是 os.TempDir()，看不出 cmd.Dir 设没设", wd)
	}
	p := obxStatusFake(t, "@echo %CD%", "@exit /b 0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, so, se, err := RunLoginStatus(ctx, provider.DiscoveryCommand{Path: p, Args: []string{"login", "status"}})
	if err != nil {
		t.Fatalf("err 应当是 nil，实际 %v", err)
	}
	if code != 0 || len(se) != 0 {
		t.Errorf("退出码应当是 0、stderr 为空，实际 %d、%q", code, se)
	}
	got := strings.TrimSuffix(string(so), "\r\n")
	if !strings.EqualFold(filepath.Clean(got), tmp) {
		t.Errorf("子进程的当前目录应当是 os.TempDir() %q，实际 stdout %q", tmp, so)
	}
}
```

---

## 3. 冻结的字面量（新增的用户可见文本）

结果行末尾的登录状态（`firstRunLoginText`）从四种变成六种，新增的是后两种：

```text
已登录（订阅：%s）
已登录
没登录
登录状态查不出
已登录（API key：%s）
已登录（API key）
```

- 「已登录（API key：%s）」的 `%s` 是 claude 的 `apiKeySource` 原值（例如 `ANTHROPIC_API_KEY`、`/login managed key`、`apiKeyHelper`）；grok 与 codex 的已登录（API key）不带来源。
- 括号、冒号是全角（与「已登录（订阅：%s）」同）；`API key` 是半角，中间一个空格。
- 这两种不附登录办法（E6）。
- 本刀没有别的新增或改动的用户可见字面量。判据用的英文串（`You are using XAI_API_KEY.`、`You are authenticated via deployment key.`、`Model '`、`' is using its own API key.`、`You are logged in with `、`.`、`Logged in using an API key`、`Logged in using Amazon Bedrock API key`）不上屏。

---

## 4. 集成员的变异清单

**运行纪律（每一次 go build / vet / test / 变异运行之前都照做，与 19d §4 表头同一套，外加这次的硬性要求）**：
- 同一条 PowerShell 命令里先设环境再跑（每次工具调用都是新进程，分两次调用等于没设）：
  - `USERPROFILE`、`HOME` 指到 `E:\Isolating_test\council\ob2_draft\canaryhome`（或集成员自己的隔离目录下的 canary home）；`APPDATA`、`LOCALAPPDATA` 指到它下面的 `AppData\Roaming`、`AppData\Local`（先建好）；
  - `COUNCIL_DATA_DIR` 指到隔离目录下的 canary 数据目录；`TMP`、`TEMP` 指到隔离目录；
  - 用 `[Environment]::SetEnvironmentVariable('<名>', $null, 'Process')` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`（不要用 `Remove-Item Env:`，会被路径保护整条拦下）；
  - `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE` 指到隔离目录下、`GOTOOLCHAIN=local` 显式给；
  - `PATH` 只留 `C:\Program Files\Go\bin` 与 `C:\Windows\System32`，去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`。
  - 起草人把这些写成一个 `goenv.ps1`，每条命令开头 dot-source 它，再跑 go 或变异驱动脚本（驱动脚本由同一个 PowerShell 起，环境随之继承）。起草人那份是纯 ASCII；集成员自己写的 `.ps1` 若含中文，要存成带 BOM 的 UTF-8（PS 5.1 读无 BOM 的 .ps1 按 GBK 解），与「跨进程文件一律无 BOM」相反。canary home、canary 数据目录、隔离 `TMP` 要先建好（起草人的那一套已随隔离目录删除）。
- 每次 go test（全量、每条变异）之后核四项，任何一项出现就停下、报告实际路径，不删隔离目录以外的任何东西：
  1. canary 数据目录下没有 `council.db`；
  2. 隔离 `TMP`、真 `%TEMP%`（`C:\Users\25142\AppData\Local\Temp`）、`E:\Go\tmp` 下没有 `TestOb*` 目录（`TestOby*` 也在这个模式里）；
  3. `E:\council-data` 不存在；
  4. canary home 下没有 `.council`。
  - 另在整批变异跑完之后核一次：没有开跑之后新起、还活着的 `cmd`、`grok`、`codex`、`claude`、`agy` 进程（N23、N30、N32 会留下自己会停的等待循环，最长约 20 s）。
- 起草人照这套纪律跑完全部变异与全量：每条之后四项核对都干净；每批之后没有残留进程。

每条在原型上单独施加、从字节副本还原（不用 `git checkout`），还原后 SHA256 核对；锚点在文件里恰好出现一次（驱动脚本数过）。`-run` 一律是 `TestOb[xy]`；包：`login.go` 的变异跑 `./internal/provider ./internal/app`，`firstrun.go` 的跑 `./internal/app`，`loginstatus_exec.go` 的跑 `./internal/turn ./internal/app`；用 `go test -json -count=1`，按子测试的 fail 事件点名。「杀手」列是原型上这一次实测变红的全部叶子用例；点名的每个子测试都要单独确认变红。编号 N11、N18、N19、N24-N29 空着不用。X1、X2、X3、X11 是 v1.1 补登的（编号沿用审查 W1，与 19d 的 X1-X5 不是同一组）。

### 4.1 本刀新加的变异

| # | 文件 | 替换前 → 替换后 | 杀手（原型实测） |
|---|---|---|---|
| N1 | provider/login.go | `case first == grokXAIAPIKey, first == grokDeploymentKey, wrapped(first, grokModelKeyPrefix, grokModelKeySuffix):` → 第三项换成 `false`（CPA 那一句不算） | K1 `grok_cpa_real`、`grok_cpa_crlf`、`grok_cpa_padded`、L1 `grok_api_key_line`、A15、B1、B2 |
| N2 | provider/login.go | 同一行 → 第一项换成 `false` | K1 `grok_xai_api_key` |
| N3 | provider/login.go | 同一行 → 第二项换成 `false` | K1 `grok_deployment_key` |
| N4 | provider/login.go | `case wrapped(first, grokLoggedInPrefix, grokLoggedInSuffix):` → `case false:` | K1 `grok_logged_in` |
| N5 | provider/login.go | `case a.APIKeySource != "" && a.APIKeySource != "none" && !hasControlChar(a.APIKeySource):` → `case false:` | K1 `claude_key_helper`、`claude_key_no_token_source`、L1 `claude_api_key_env`、`claude_console_login`、B1、B2 |
| N6 | provider/login.go | 同一行去掉 `a.APIKeySource != "none" &&` | K1 `claude_key_none` |
| N7 | provider/login.go | 同一行去掉 `&& !hasControlChar(a.APIKeySource)` | K1 `claude_key_control_char` |
| N8 | provider/login.go | 订阅那一支（两行）与 key 那一支（两行）对调顺序 | K1 `claude_sub_and_key` |
| N9 | provider/login.go | `return LoginInfo{State: LoginAPIKey, KeySource: a.APIKeySource}` → `return LoginInfo{State: LoginAPIKey}` | K1 `claude_key_helper`、`claude_key_no_token_source`、L1 `claude_api_key_env`、`claude_console_login`、B1、B2 |
| N10 | provider/login.go | `wrapped` 里 `len(s) > len(prefix)+len(suffix)` → `>=` | K1 `grok_logged_in_empty`、`grok_model_key_empty_name` |
| N12 | provider/login.go | `if exitCode == 0 && (strings.HasPrefix(l, codexAPIKeyPrefix) \|\| l == codexBedrockAPIKey) {` → `if false {` | K2 `codex_api_key_stderr`、`codex_bedrock`、L2 `codex_logged_in_stdout_crlf`、B1、B2 |
| N13 | provider/login.go | 同一行 `l == codexBedrockAPIKey` → `false` | K2 `codex_bedrock` |
| N14 | provider/login.go | 同一行去掉 `exitCode == 0 &&`（写成 `if strings.HasPrefix(l, codexAPIKeyPrefix) \|\| l == codexBedrockAPIKey {`） | K2 `codex_api_key_exit1` |
| N15 | provider/login.go | API key 那一段 `if`（三行）与通用 `Logged in` 那一段 `if`（三行）对调顺序 | K2 `codex_api_key_stderr`、`codex_bedrock`、L2 `codex_logged_in_stdout_crlf`、B1、B2 |
| N16 | provider/login.go | 同 N13 那一行 `l == codexBedrockAPIKey` → `strings.HasPrefix(l, codexBedrockAPIKey)` | K2 `codex_bedrock_tail` |
| N17 | provider/login.go | 同一行 `strings.HasPrefix(l, codexAPIKeyPrefix)` → `l == codexAPIKeyPrefix` | K2 `codex_api_key_stderr`、L2 `codex_logged_in_stdout_crlf`、B1、B2 |
| X1 | provider/login.go | `grokLogin` 里 `first == grokXAIAPIKey` → `strings.HasPrefix(first, grokXAIAPIKey)`（整句相等放宽成前缀；v1.1） | K1 `grok_xai_api_key_tail` |
| X2 | provider/login.go | 同一行 `first == grokDeploymentKey` → `strings.HasSuffix(first, grokDeploymentKey)`（整句相等放宽成后缀；v1.1） | K1 `grok_deployment_key_prefixed` |
| X3 | provider/login.go | `case first == grokNotAuthenticated:` → `case strings.HasPrefix(first, grokNotAuthenticated):`（整句相等放宽成前缀；19d 遗留的同类，v1.1） | K1 `grok_not_authenticated_tail` |
| X11 | provider/login.go | `first = strings.TrimSpace(first)` → `first = strings.TrimRight(first, "\r")`（只去 CR；19d 遗留的同类，v1.1） | K1 `grok_cpa_padded`（`grok_cpa_crlf` 只多 CR，不红） |
| N20 | app/firstrun.go | `if row.login.State != provider.LoginYes && row.login.State != provider.LoginAPIKey {` → `if row.login.State != provider.LoginYes {`（已登录（API key）的也列登录办法） | A15、B1、B2 |
| N21 | app/firstrun.go | `case provider.LoginAPIKey:` → `case -1:`（新档落到查不出） | B0 `key`、`key_source`、A15、B1、B2 |
| N22 | app/firstrun.go | `return "已登录（API key：" + l.KeySource + "）"` → `return "已登录（API key）"` | B0 `key_source`、B1、B2 |
| N23 | app/firstrun.go | `if code, so, se, serr := firstRunStatus(cctx, sc); serr == nil {` → `firstRunStatus(ctx, sc)`（审查 R1） | B3（errOut 不等，耗时 24.51 s） |
| N30 | turn/loginstatus_exec.go | 删掉 `_ = killProcessTree(cmd.Process.Pid)` 与 `_ = cmd.Process.Kill()` 两行（审查 R0） | L5、L5b（都是 5.30 s） |
| N32 | turn/loginstatus_exec.go | 只删 `_ = cmd.Process.Kill()` | L5b |
| N33 | turn/loginstatus_exec.go | `cmd.Dir = os.TempDir()` → `_ = os.TempDir()`（审查 R4；`os` 包仍要用到，所以不能整行删） | L6 |

**判定为等价、不列杀手的变异**（原型实跑存活）：
- N31 只删 `_ = killProcessTree(cmd.Process.Pid)`：`cmd.Process.Kill()` 照样杀掉 `.cmd` 那一层；本刀的夹具只用 cmd 内建循环，没有孙进程，树杀与单杀看不出差别。要看出来得有起孙进程的夹具，那要 System32 里的外部程序（19d §0 第 9 条），不做，登记 §7 (e)。
- M59（19d 的，这次重跑）`firstRunCheck` 的 `ok && cctx.Err() == nil` → `ok`：照旧存活，理由同 19d。B3 里 `cctx` 是在 login status 跑着的时候到期的，不是之前，所以走不到这一支。

### 4.2 19d 的变异在本刀下的状态

受 F1、P1、P2、T1-T8 影响的，这次在原型上重跑（锚点写成原型里的新文本）：

| # | 文件 | 替换前 → 替换后 | 杀手（原型实测） |
|---|---|---|---|
| M39（锚点改写） | app/firstrun.go | `if row.login.State != provider.LoginYes && row.login.State != provider.LoginAPIKey {` → `if row.login.State == provider.LoginNo {` | A8、A15、B1、B3 |
| M56 | app/firstrun.go | `stdout = out` → `_ = out` | A1、A2、A5、A6、A9、A11、A12、A15、B1、B2 |
| M58 | app/firstrun.go | `…; ok && cctx.Err() == nil {` → `…; false && ok && cctx.Err() == nil {` | A1、A2、A5、A9、A11、A15、B1、B2 |
| M80 | provider/login.go | 订阅那一支去掉 `&& a.TokenSource != "none"` | L1 `claude_none_and_sub` |
| M81 | provider/login.go | 订阅那一支去掉 `!hasControlChar(a.SubscriptionType) &&` | L1 `claude_sub_control_char` |
| M82 | provider/login.go | `case a.TokenSource == "none" && a.SubscriptionType == "" && a.APIKeySource == "":` → `case a.TokenSource == "none" && a.APIKeySource == "":` | L1 `claude_none_and_sub` |
| M83 | provider/login.go | `if _, ok := claudeDiscoveryResponse(line); !ok {` → `if len(bytes.TrimSpace(line)) == 0 {` | L1 `claude_other_request_only`、`claude_other_then_ours`、`claude_real_shape` |
| M84（锚点改写） | provider/login.go | `case first == grokNotAuthenticated:` → `case first == grokNotAuthenticated \|\| strings.Contains(string(stdout), "\n"+grokNotAuthenticated):` | L1 `grok_marker_not_first` |
| M86 | provider/login.go | `if exitCode == 0 && strings.HasPrefix(l, "Logged in") {` → 去掉 `exitCode == 0 &&` | L2 `codex_logged_in_but_exit1`、K2 `codex_api_key_exit1` |
| M87 | provider/login.go | `if exitCode != 0 && l == "Not logged in" {` → 去掉 `exitCode != 0 &&` | L2 `codex_not_logged_in_but_exit0` |
| M88 | provider/login.go | 删掉 `l = strings.TrimSpace(l)` | L2 `codex_not_logged_in_padded`、A9 |
| M89 | provider/login.go | `LoginStatusCommandFor` 的 `if providerID == "codex" {` → `if true {` | L3、A1、A2、A5、A6、A9、A11、A12、A15、B1、B2 |
| M94（杀手换了） | provider/login.go | 没登录那一支去掉 `&& a.APIKeySource == ""` | K1 `claude_key_none`、`claude_key_control_char`（L1 那两行此后先被 key 那一支接走，杀不了它了） |

- **M85 作废**：它的锚点（`grokLogin` 里的 `if strings.TrimSpace(first) == grokNotAuthenticated {`）在 F1 里没有了，意思也反了（19d 要钉「CPA 那一行不算已登录」，本刀要它算已登录（API key））。由 N1 接替。
- 19d 其余各行（M1-M38、M40-M55、M57、M60-M74、M90-M93、M95-M106、X1-X5）的锚点与杀手不受本刀影响，没有重跑；它们「杀手」列里凡是写了 A15 的，A15 的期望这次改了（T6-T8），但那几条变异是靠 A15 的别的行杀的（检测顺序、目录、配置文件字节），按代码推仍红，没有实跑。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许动这些文件：**块 A**：整个替换 `internal/provider/login.go`（F1），授权的测试改动 `internal/provider/obx_login_test.go`（T1-T5）；**块 B**：补丁 `internal/app/firstrun.go`（P1、P2）、`internal/app/app.go`（P3），授权的测试改动 `internal/app/obx_firstrun_app_test.go`（T6-T8）。不改 `go.mod`，不新写任何 `oby` 测试文件（那是测试腿的事）。
- 两块合计约 250 行（F1 202 行，其余是锚点与替换），**一次交完**，先块 A 后块 B；落地时可以只落块 A 先跑（E12）。
- 整文件用 `===FILE: <路径>===` 换行接完整内容、末尾 `===END FILE===`。补丁块：

  ```
  ===PATCH: internal/app/firstrun.go===
  <<<<ANCHOR
  （从 HEAD e60bceb 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点照 §1.1 各处的 `anchor` 代码块（本文代码块里的缩进是 tab；若你看到的是空格，以真文件为准）；同一文件的多个锚点互不重叠。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略（冻结代码里本来就有的中文省略号 `……` 是字符串内容，照抄）。
- 代码必须能 `gofmt`（tab 缩进）；`go vet` 不许有新报告。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（全角括号、冒号、分号、逗号、顿号、省略号 `……`、直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的用户可见字符串只有 §3 的两种。
- 生产代码新标识符不许以 `oby` 开头。§1.1 给出的签名、判据、代码、注释照抄；本刀没有「名字实现方定」处。
- 回复末尾必须有：

  ```
  ===REPORT===
  块：给了哪些文件（块 A、块 B 各自）。
  补全性：F1、P1-P3、T1-T8 各落在哪个 FILE / PATCH 块。
  锚点：每个锚点是否从 HEAD e60bceb 真文件逐字复制、在该文件里恰好一次。
  字面量：§3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  ```

---

## 6. 落地后主代理要做的

1. **收割与逐字核对**：越界扫描（主仓库、`internal/*`、`cmd/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描。逐字核对：用脚本从本文取出每一对 `anchor` / `replace` 与每个 `file` 代码块（内容加一个 LF），打在 HEAD `e60bceb` 的一份 `git archive` 上，结果应当与落盘后的 8 个文件（`login.go`、`firstrun.go`、`app.go`、`obx_login_test.go`、`obx_firstrun_app_test.go`，外加测试腿的三个 `oby` 文件）逐字节相同（起草时就是这样核的：11 对锚点各恰好一次、4 个整文件）。`gofmt -l ./internal ./cmd`；`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn`；`go build ./...`。`internal/turn`、`internal/repl`、`internal/config`、`internal/tui`、`cmd/council` 零改动。
2. **合并测试腿的 3 个文件**，先跑 `go test -count=1 ./internal/provider ./internal/app ./internal/turn`，再跑全量 `go test -count=1 ./...`（原型九包全绿）；`-run 'TestOb[xy]'` 三个包跑一次 `-count=3`。**每一次运行都照 §4 表头的运行纪律**，跑完核四项与残留进程。
3. **§4 的变异逐条跑**（4.1 的 27 条杀手变异（v1.1 加了 X1、X2、X3、X11）与 N31、M59 两条等价，4.2 的 13 条），照 §4 表头的纪律，从字节副本还原、SHA256 核对；点名杀手里的每个子测试都要单独确认变红。N23 整条约 27 s、N30 约 17 s。
4. **文档**：
   - `docs/00_DECISIONS.md` #48 追加裁决之下记「OB2 已落地（commit），真机未复验」：E1-E12 的主代理裁定、§0 第 2-5 条的实测与程序文本、§7 的待定项。
   - `docs/19d_OB1_FIRST_RUN_CONTRACT.md` 文件头「落地后订正」加一条：R0、R1、R2、R4 已由 OB2 补上；§7 (b2) 已由 #48 追加裁决与 OB2 定；D15 与 §3.1 的登录状态说法、§0 第 14 条 grok 那一行的结论以 OB2 为准。
   - `docs/README.md`：登录状态改成四档说法（已登录（订阅：…）/ 已登录 / 已登录（API key）/ 没登录 / 登录状态查不出），配了 API key 的不附登录办法。
   - 本契约归档为 `docs/19e_OB2_API_KEY_LOGIN_CONTRACT.md`（照 19d 的体例在文件头加落地后订正区）。
   - `docs/14_MVP_CHECKLIST.md` 的 A3 步骤按下面第 5 步补 `GROK_HOME` 那一句。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的真机复验步骤**（E 盘机，Windows Terminal + PowerShell 5.1，全部零花费：只跑识别与 `codex login status`，不提问）。办法同 19d §6 第 5 步：用临时目录模拟一个新用户，不碰真配置 `C:\Users\25142\.council\config.toml` 与真库。**多了一个变量 `GROK_HOME`**：grok 按 `USERPROFILE` 找自己的目录（§0 第 2 条实测），只改 `USERPROFILE` 的话 grok 读不到你的 CPA 配置、会显示「没登录」；`GROK_HOME` 指回你自己的 `.grok` 目录，grok 读的就是真配置（本机实测一次：首行是 CPA 那一句，临时目录里什么都没建）。council 本身不读这个目录。PowerShell 的命令每条一块，逐块粘贴、逐块回车；council 里的命令也一行一块。

   0. 先确认装的是带本刀的版本（主代理照惯例等 Griffin 说「装」再重装）：

      ```powershell
      council -version
      ```

   1. 建临时目录：

      ```powershell
      $sim = "E:\Isolating_test\council\ob2_realcheck"; New-Item -ItemType Directory -Force "$sim\home","$sim\data" | Out-Null
      ```

   2. 只在这个 PowerShell 窗口里改五个变量（关掉窗口就还原）。**`GROK_HOME` 必须第一个设**，它要用改之前的 `USERPROFILE`：

      ```powershell
      $env:GROK_HOME = "$env:USERPROFILE\.grok"; $env:USERPROFILE = "$sim\home"; $env:HOME = "$sim\home"; $env:COUNCIL_DATA_DIR = "$sim\data"; $env:COUNCIL_CONFIG = ""
      ```

   3. 第一次运行（行式，看得清）：

      ```powershell
      council -no-tui
      ```

      看：「第一次运行：还没有配置文件 E:\Isolating_test\council\ob2_realcheck\home\.council\config.toml，council 来生成一份。」「正在检查 claude、grok、agy、codex 能不能用……」，几秒后四行结果（下面的模型个数，claude 4、agy 14、codex 7 是 19d §0 第 19 条 09-29 的读数，grok 6 是本刀起草时的读数；个数一律以你那时的 CLI 为准，要看的是每行末尾的登录状态）：
      - claude「识别到 4 个模型；没登录」（它读的是临时 home；你的终端里要是设着 `ANTHROPIC_API_KEY`，会是「已登录（API key：ANTHROPIC_API_KEY）」，这一种按程序文本推，没实跑过）；
      - **grok「识别到 6 个模型；已登录（API key）」**（这就是本刀要的那一行）；
      - agy「识别到 14 个模型；登录状态查不出」；
      - codex「识别到 7 个模型；已登录」（它读的仍是你自己的登录态，19d §6 已写明；你用的是 ChatGPT 登录，所以不是「已登录（API key）」）。
      - 接着「能列出模型不代表已经登录……要登录在终端里运行：」，**只列 claude 与 agy 两行**（grok、codex 不列）；「已生成配置文件: …」与上手三步；然后是横幅和 `council>`。敲 `/quit` 退出。

   4. （可选的对照）新开一个 PowerShell 窗口，照第 1-3 步再做一次、第 2 步**不设 `GROK_HOME`**（先删掉 `$sim\home\.council`）：grok 那一行应当是「识别到 2 个模型；没登录」，登录办法里多出 grok。这说明第 3 步看到的「已登录（API key）」确实来自你的 CPA 配置。

   5. 收尾：关掉这个 PowerShell 窗口（变量随之还原），确认真配置没动：

      ```powershell
      Get-Item C:\Users\25142\.council\config.toml | Select-Object Length, LastWriteTime
      ```

      与复验前记下的一样即可；`$sim` 整个删掉。

   - Mac 上的 CPA 睡着、掉网时 grok 的首行会不会变，没测（§7 (g)）。D 盘机（CPA 在本机）照同样的步骤做，[未验证]。

---

## 7. open items

**待 Griffin**

- (b) **codex 的 `Logged in using Amazon Bedrock API key` 算不算「已登录（API key）」**：本文按句子本身写的 `API key` 算了（E4）。`access token`、`personal access token` 两句算「已登录」。你不用这几种登录，影响的是别的用户；要换就改 F1 的 `codexBedrockAPIKey` 那一支与 K2 两行。
- (c) **claude 订阅与 API key 同时在时显示订阅**（E2）：程序文本证明两者能同时出现，但看不出这时 claude 实际按哪一个计费。屏幕说「已登录（订阅：…）」，不提 key。
- (d) **正面信号大多只有程序文本**：grok 的 `You are using XAI_API_KEY.`、`You are authenticated via deployment key.`、`You are logged in with …`，codex 的两句 API key，claude 的 `apiKeySource`，本机都没有那种登录态，没实跑。只有 grok 的 CPA 那一句与 codex 的 ChatGPT 那一句实测过。判错的后果：说成已登录（API key）的用户其实用不了，或者反过来，只会在这几种登录方式上出现；判据都是整句或两头都卡住的写法，CLI 改了措辞一律退回「登录状态查不出」。版本是 claude 2.1.283、grok 1.0.41、codex-cli 0.147.0，升级之后要按 §0 第 3-5 条的办法重核。grok.exe 里另有 `You are authenticated via XAI_API_KEY (environment variable).`（`0x866f46d`）与 `You are using a deprecated authentication method (WebLogin).`（`0x86ed829`）两句（v1.1 补登，§0 第 3 条），不在 models.rs 那张表里，不列入判据，落在查不出；它们会不会出现在 `grok models` 的首行没实跑。前一句若真出现在首行，按字面是配了 key，本文会说成查不出：方向安全，只是少认一种。

**待主代理**

- (a) **`/add` 的识别表不显示登录状态**（19d §7 (o)，裁决没排进来）：要显示的话是另一刀，涉及 `internal/repl` 的识别表格式与 MD1 的既有测试。
- (e) **`killProcessTree` 按 `PATH` 找 `taskkill`**（19d §7 (d)）仍开着。L5b 这次钉住了「找不到 taskkill 时 `Kill` 兜底」；树杀本身（有孙进程时）仍没有夹具（N31 等价）。
- (f) **B3 的耗时阈值 10 s**：未变异 2.03 s，N23 下 24.51 s，中间留了很大余量；但 N30 下 B3 是 7.03 s，所以 B3 不是 N30 的杀手（L5、L5b 是）。极慢的机器上 B3 未变异也可能接近 10 s，没见过。
- (g) **CPA 不通时的 grok**：E 盘机的 CPA 跑在同一 WiFi 的 Mac 上，Mac 睡眠或掉网时 `grok models` 的首行、退出码没测。首行若仍是 CPA 那一句，首次运行会说「已登录（API key）」，可实际用不了；这是「配了 key」与「key 能用」的差别，裁决说的是前者，本刀不做连通性检测（§1.2）。
- (h) **`GROK_HOME` 只实测了一种取值**（指向 `.grok` 目录本身，E 盘机）。§6 第 5 步只用这一种。
- (i) **显示原值**：claude 的来源原样上屏，`/login managed key` 读起来像一句英文说明，不像一个名字；照裁决「显示 apiKeySource 的原值」，不翻译。
- (j) **19d §4 的杀手列**：凡是写了 A15 的变异，除 §4.2 重跑的那几条外，按代码推仍红、没有实跑（§4.2 末尾）。
- (k) **本刀没有用原型二进制做首次运行的模拟**：隔离纪律要求原型二进制运行时 `USERPROFILE` 指向 canary home，首次运行会在那里建 `.council`，正好触发第 4 项核对。§6 第 5 步的期望（grok「识别到 6 个模型；已登录（API key）」）由三样拼出：真 `grok models` 在同样环境下的实测输出（§0 第 2 条）、K1 `grok_cpa_real` 对这份输出的判定、B1 对整条链路的端到端。

**泄漏核对（起草期间，照 §4 表头）**：原型上的每一次 go build / vet / test 与每一条变异都在同一条 PowerShell 命令里 dot-source 了隔离环境；每次 go test 之后四项核对全部干净（canary 数据目录没有 `council.db`；隔离 `TMP`、真 `%TEMP%`、`E:\Go\tmp` 下没有 `TestOb*`；`E:\council-data` 不存在；canary home 下没有 `.council`）；每批变异之后没有残留的 `cmd`、`grok`、`codex`、`claude`、`agy` 进程。探针期间 grok 在隔离目录下的两个临时 home 里建过 `.grok`、`AppData`（§0 第 2 条），随隔离目录删除。

**泄漏核对（v1.1，照 §4 表头）**：v1.1 原型上的每一次 go build / vet / test 与每一条变异，都在同一条 PowerShell 命令里先 dot-source 隔离环境再跑（`USERPROFILE`、`HOME` 指 canary home，`APPDATA`、`LOCALAPPDATA` 指它下面的 `AppData\Roaming`、`AppData\Local`；`COUNCIL_DATA_DIR`、`TMP`、`TEMP` 指隔离目录；清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`；`GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE` 指隔离目录、`GOTOOLCHAIN=local`；`PATH` 只留 Go 与 System32，去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`）。每次 go test 与每条变异之后四项核对全部干净；两批变异之后都没有开跑之后新起、还活着的 `cmd`、`grok`、`codex`、`claude`、`agy` 进程。
