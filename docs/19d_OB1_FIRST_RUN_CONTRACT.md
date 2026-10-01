# OB1 冻结契约：首次运行自动配置（v1.2，2026-09-29）

> **归档说明（2026-09-30，主代理）**：本文是 OB1 的冻结契约，派工时的原件是 `E:\multi_model_cowork\ob1_contract.md`（v1.2），原样归档；锚点与行号指 HEAD `cbaf195`。OB1 已于 `3317940` 落地，真机未复验。
> - **实现**：生产代码分四块，四路 grok-4.7 并行实现，每路只拿本块的契约摘录，与 §1.1 逐字节相同（13 个文件）。
> - **测试**：5 个 obx 测试文件由两条 Opus 测试腿写成。
> - **集成**：Opus 集成员做完 §4 的 98 条变异，全被点名杀手杀掉，6 条等价核过。每次运行后都做四项泄漏核对，全部干净。
> - **审查**：Opus 独立审查报 5 条 minor（见下）。
>
> **落地后订正（2026-09-30，主代理；读本文之前先读这里）**：
> - **§7 (u) 已答（集成员与测试腿都实测过）**：v1.2 下 H0 泄漏的 Run 把 `config.toml`（写了 data_dir，3241 B）、`council.db`、`council.lock` 建在隔离 `TMP` 里重建的临时 home 下，canary 数据目录是空的。所以 H0 的正对照是**第 2 项**（隔离 TMP 下出现 `TestObx*`）。第 1 项（canary 下的 `council.db`）降为兜底，只覆盖没写 data_dir 的路径。§4 表头、§2.0 H0/H1、§0 第 18 条里写的「canary 出现 council.db」是 v1.1 的读数。
> - **三处测试没覆盖到，生产代码是对的**（审查 R0、R1、R4，留给后续小刀）：
>   - `RunLoginStatus` 超时杀进程那两行删掉也全绿：L4 `killed` 不看耗时，5 s 宽限期后照样返回同一个错误。
>   - `firstRunCheck` 里 `firstRunStatus(cctx, sc)` 改成父 ctx 也全绿。于是 codex login status 挂住时，首次运行会卡到用户按 Ctrl-C。
>   - `RunLoginStatus` 的 `cmd.Dir = os.TempDir()` 删掉也全绿。
> - **一处冻结注释说少了**（审查 R2）：`app.go` 的 `reportConfigError` 头注释（本文 P4）说默认位置找不到文件时「只有竞态能走到这里」，漏了默认路径不是绝对路径的情形（A14）。D2 表写对了，P4 与 D2 自相矛盾。代码不受影响。
> - **测试腿报的实操提醒**：M55 的锚点按字面在 `app.go` 里出现两次，要带上「指定别处的配置文件，」才恰好一次。
> - **已由 OB2 补上（2026-09-30，`70025db`，契约 `19e`）**：
>   - 上面 R0、R1、R4 三处测试缺口已补（L5、L5b、B3、L6），R2 的注释已改（19e 的 P3）。
>   - §7 (b2) 已定：#48 追加裁决（「支持CPA的那种吧」）与 OB2 把配了 API key 的算作「已登录（API key）」。
>   - D15、§3.1 的登录状态说法，以及 §0 第 14 条表里 grok 那一行的结论，都以 19e 为准。grok 首行 `Model '…' is using its own API key.` 现在是「已登录（API key）」；claude 带 `apiKeySource` 的是「已登录（API key：<来源>）」。
> - **§7 (o) 已由 OB3 做了（2026-09-30，`ef76eef`，契约 `19f`）**：`/add` 在识别结果之后多一段「各家的登录状态」。本文冻结的 `firstRunLoginText` 与 `firstRunLogin` 挪到了 repl（`repl.LoginText`、`repl.LoginCommand`），首次运行改用它们，屏幕输出不变。
> - **§7 (d)「killProcessTree 按 PATH 找 taskkill」：已由 CL1 改**（2026-10-01，`5c4840a`，#50，`19h`）。改用 `windows.GetSystemDirectory()` 拼出系统目录里的 taskkill.exe，不读 SystemRoot，出错时什么都不起、绝不退回按 PATH 找。


- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `cbaf195`（完整 `cbaf195e8cb601bd599174c858604e900fa3d2ce`，= origin/main）。Go 全路径 `"C:\Program Files\Go\bin\go.exe"`（go1.27.0 windows/amd64），`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`，不设 `GOFLAGS`。本机没有 gcc，不能 `-race`。E 盘机（Windows 11，PowerShell 5.1）。
- 本刀的 KEY 是 `OB1`；测试腿的包级标识符前缀是 `obx`（类型、函数、变量、常量一律 `obx` 开头，测试函数 `TestObx...`）。生产代码的新标识符不许以 `obx` 开头。
- 来源：`docs/00_DECISIONS.md` **#48**（`:2266-2282`，Griffin 定方向「要做」，主代理定范围；本契约不改方向）。v1.1 依据主代理裁决 `E:\multi_model_cowork\ob1_rulings.md`（W1 审查 F0-F8 与证伪判词 V0；裁决第 1 条「登录检测」、第 2 条「/add 候选的错话」）。v1.2 依据主代理裁决 `E:\multi_model_cowork\ob1_rulings_v12.md`（第 1 条「生成的配置写 `data_dir`」、第 2 条「`/add` 对在场总结成员的说法」、第 3 条「(b2)、(b3)、(q) 并成一个问题给 Griffin」）。相关：#45（MD1 模型识别与 `/add +编号`、`config.AppendBlock`）、#46（codex）、`docs/16a_MD1_MODEL_DISCOVERY_CONTRACT.md`、`docs/18a_CXH_CODEX_DISCOVERY_CONTRACT.md`、`docs/19a_CH1_CONVERSATION_MODE_CONTRACT.md` 文件头 R0、`docs/07_STEP4_SPEC.md` / `07a_STEP4_API_CONTRACT.md`（Step 4 的配置路径解析、`DefaultTOML`、「不创建目录」）。
- **这推翻了 Step 4「不替用户创建配置文件」**（`internal/app/app.go:346` 的「council 不会替你创建这个文件」、`docs/07_STEP4_SPEC.md:85`、`:221`），但只在默认位置、文件不存在、而且默认路径是绝对路径时；`--config` 或 `COUNCIL_CONFIG` 指定了路径却找不到，照旧报错、不建文件。
- 本文的全部期望值（屏幕逐字节、写出的配置文件字节、退出路径、TUI 管道里的顺序、登录状态）都在隔离拷贝 `E:\Isolating_test\council\ob1_v11\`（`git archive cbaf195`，先打 v1 §1.1、v1 的验收在上面全绿，再按裁决改）上用一份原型实现实跑得出；原型下全量 `go test -count=1 ./...` 九包全绿（含 §1.1 T1-T3 三处授权的测试改动）；§4 的变异在原型上逐条实跑。§1.1 的冻结代码与补丁由脚本从原型文件取出写进本文，再由脚本从本文取回、打在一份新的 `git archive cbaf195` 上，结果与原型逐字节相同（§6 第 1 步照这个办法核）。原型不交付，已删除。
- W1b 修订（见变更记录末尾「W1b 修订」）改动的期望与冻结代码，在另一份原型 `E:\Isolating_test\council\ob1_fix\` 上实跑得出，同样经脚本往返核对（12 个文件逐字节相同）；那份原型只重写了与这次修订相关的用例，用完已删除。
- v1.2 修订（见变更记录末尾「v1.2」）在第三份原型 `E:\Isolating_test\council\ob1_v12\` 上做：`git archive cbaf195` 解出，照 v1.1 §1.1 逐字打上，**§2 的五个测试文件全部重写**，在 v1.1 的代码上全绿之后再按裁决改。v1.2 改动的冻结代码与期望都从这份原型取出，再经脚本往返核对（13 个文件逐字节相同）。原型不交付，用完已删除。

## 变更记录（v1.1、W1b 修订、v1.2；基线仍是 `cbaf195`）

v1.1 照主代理裁决修订。新增与改动的期望值全部来自原型 `E:\Isolating_test\council\ob1_v11\` 实跑，整串冻结；新增变异全部实跑确认点名杀手变红，从字节副本还原、还原后逐个文件 SHA256 核对。W1 的变异名 X1-X5 在本文沿用；v1.1 自己加的变异编号 M23-M28、M39、M56-M59、M67-M74、M80-M93（M26、M28、M59 是存活的等价变异）。

- **F0（blocker）：测试收尾不泄漏。**
  - §2.0 冻结 `obxRun` / `obxRunTee` / `obxDrain`：交给 `Run` 的 ctx 可取消；`obxRun` 内部注册 `t.Cleanup`：`cancel()`，最多等 10 s 让 `Run` 返回，等不到就结束整个测试进程。看门狗只在测试 goroutine 里 `select`。A11 的收尾同样经 `obxDrain`；A12 经 `obxRunTee`。「同类的别处」核过：新用例里直接调 `Run` 的只有 A11；既有测试里走到默认位置的只有 T1（同步调用，`Run` 返回之前用例不往下走，不会泄漏）。
  - **与裁决字面不同：等不到时用 `os.Exit(3)`，不用 `panic`。** 探针实测（§0 第 16 条）：`t.Cleanup` 里 panic 之后，testing 照样跑完其余 cleanup（还原环境变量、删临时目录）才让进程死掉；一个还活着的 goroutine 在这个窗口里读到了还原后的环境变量。这正是 F0 的泄漏机制。`os.Exit` 之后其余 cleanup 一个都不跑、后续用例不跑。代价：临时目录留在 `%TEMP%`，三项泄漏核对的第二项会报出来（这正是要人看见的）。`os.Exit` 这一支本身没有在真装置里触发过，只有探针。
  - A8、A12 的假 claude 用 `obxLoopLines`：保留条件判断（A8 是 `loop.on` 还在），再叠加 `set /a` 计数上限 60000。本机单独实测这种循环每秒约 3400 次，60000 次约 18 s；在 go test 里 M33 下 A8 实测 21.25 s 才数到上限（§0 第 15 条）。给 1 s、3 s 超时留足余量，又保证循环一定会停。
  - M33 的杀法随之改变：假 claude 数到上限自己退出，claude 那一行变成「输出认不出来」，A8 以 errOut 不等而红（A8 实测 21.25 s，整条变异运行约 23 s），不再是 30 s 看门狗。杀手只列 A8。
  - §4 表头与 §6 第 3 步写进运行纪律：父 shell 设 canary 的 `COUNCIL_DATA_DIR`、不设 `COUNCIL_CONFIG`，跑完核三项。canary 这条缺席断言的正对照是同装置的 H0 / H1 一对（§2.0，原型实测：H0 把 `obxDrain` 关掉之后 canary 出现 `council.db`，H1 不关就没有）；W1 的证据 (a) 是旁证。
- **F1**：C1 加 `cwd_planted`（先拿掉宿主注入的 `NoDefaultCurrentDirectoryInExePath`，再断言前提「`exec.LookPath` 返回 `exec.ErrDot`」，期望第 0 项 Path 为空）；§0 第 13 条；§4 X1；§7 (k)。
- **F2**：新增 A12（错误出口一写出「正在检查」就建标记文件，假 claude 等标记）；§4 X3。
- **F3**：`CreateConfigFile` 改成「同目录临时文件写完、关掉，再 `os.Link` 到目标，最后删临时文件名」；`Link` 因 `ErrExist` 以外的原因失败时退回 v1 的 `O_EXCL` 写法（新函数 `createExclusive`），代价写进注释。退回那一支靠包级接缝 `linkFile` 造出来（NTFS 上造不出 `Link` 的其它失败）。新增 C6（并发）、C7（`via_link`、`fallback`、`fallback_exists`）；C5、A6 加「目录里只剩 `config.toml`」；§4 X4、M23-M26，M18 的杀手换成 C7 `fallback_exists`。
- **F4**：新增 A13（`USERPROFILE` 是 `<root>\a<b`）；§4 X5。
- **F5**：P1 的触发条件加 `filepath.IsAbs(path)`；新增 A14（相对 home）；§4 M27。A14 顺带让 `reportConfigError` 非 explicit 的 NotExist 分支可达，v1 只能审查时看的 M55 进表，杀手 A14。
- **F6**：X2 进 §4 表，杀手 A8。原型那一次 A2 也红了，是完成顺序随机碰上的，不算。
- **F8**：D9 的引用改成 §7 (g)；§7 (c) 按裁决第 1 条重写。
- **裁决第 1 条（登录检测）**：先做探针（§0 第 14 条，全部零花费），再定信号。
  - claude：识别那一次 initialize 回应里的 `account`。有 `subscriptionType`（不含控制字符）而且 `tokenSource` 不是 `none`：已登录，显示订阅原值（本机是 `Claude Max`）；`tokenSource` 是 `none` 而且没有 `subscriptionType`：没登录；其余查不出。`claude auth status` 实测能分开，但要多起一个进程，按裁决不用。
  - grok：识别那一次 stdout 的首行是 `You are not authenticated.`：没登录；其余查不出。**与裁决字面不同**：裁决把 `Model 'grok-4.7' is using its own API key.` 称作「真登录态下的首行」，本文不把它当正面信号。这句说的是那个模型配了自己的 API key，不是 `grok login` 做过；把它说成「已登录」就是 §3 严重度规则里的 major（屏幕与证据不符）。`grok --help` 里没有 status 一类的子命令。A15 用这一行钉住「查不出」，M85 是杀手。要不要把它算已登录，列 §7 (b2) 待 Griffin。
  - codex：另跑一次 `codex login status`（实测 0.16 s，零花费，退出码 0 / 1，文字打在 **stderr**：`Logged in using ChatGPT` / `Not logged in`）。`turn.RunDiscovery` 退出码为 0 时不交回 stderr，所以在 `internal/turn` 加一个新文件、一个新函数 `RunLoginStatus`；`internal/provider` 加一个新文件 `login.go`（三档类型与三个判据函数）。现有识别的行为、识别表一个字不改。
  - agy：查不出（v1 已实测改环境变量隔离不了它，本刀不读凭据存储）。
  - 屏幕：每行结果末尾加「；<登录状态>」；原来那句「识别到模型不代表已经登录……要登录或换账号」改成「能列出模型不代表已经登录：……没登录或登录状态查不出的，要登录在终端里运行：」，登录办法只列没登录与查不出的那几家；找到的都已登录时这两段都不打（A6、A12 钉）。
  - 「正在检查」那一行里的「只让它们列出模型」改成「只让它们列出模型、报登录状态」：codex 多跑的是 `login status`，原句对它不再准确。这一处不在裁决里。
  - `/add` 的识别表不显示登录状态（裁决：留 open item，§7 (o)）。
- **裁决第 2 条（F7 与 CH1 R0）**：`printAddCandidates` 候选为空时分三种说法，`resolveAddSegment` 前缀没命中时加总结成员那一句，`cmdAdd` 头注释补上「与总结成员」（P8-P10）。「都在会话里了」那一句原样保留，所以 `mdxAllPresent` 与 `afterCandidates` 不用改；既有测试里只有 `TestMdxAddSkipsUndiscoverableProviders` 的 (a)、(b) 落进新的「一个成员都没有」，授权改两处（T2、T3）。新增 R6；§4 M67-M72。落地后主代理在 `19a` 文件头把 R0 标「已由 OB1 修」。
- **交付分块**：块 A 不变；新增块 B（`internal/provider/login.go`、`internal/turn/loginstatus_exec.go`，只依赖 HEAD）；v1 的块 B 拆成块 C（`internal/repl` 与 T2、T3）与块 D（`internal/app` 与 T1），各自不超过约 300 行；交付顺序 A、B、C、D。
- **夹具**：`obxClaudeOut` 带上 `account`（已登录，订阅 `Claude Obx`）；新增 `obxClaudeNoLoginOut`、`obxGrokKeyOut`；`obxGrokOut` 首行改成 `You are not authenticated.`；假 codex 经 `obxCodexFake` 按第一个参数分成 `debug models` 与 `login status` 两支。A1-A11 的期望全部按原型重取。

### W1b 修订（2026-09-29；第二轮对抗审查 W1b 的六条发现）

处置：F-A（major）、F-B、F-C、F-D、F-E 成立，照改；F-F 取保守读法，不改代码，记 §7 (b3) 待主代理。

原型在 `E:\Isolating_test\council\ob1_fix\`，做法如下：
- `git archive cbaf195` 解出一份，用脚本从本文原始字节取出 6 个 `file` 块、14 对 `anchor` / `replace` 打上（每个锚点恰好一次），再在原型上改。
- 改完用脚本把 F5 与 P6-P10 从原型写回本文，再从本文取回、打在一份新的 `git archive` 上。12 个文件与原型逐字节相同。
- `go build ./...`、`go vet`（五个包）、`gofmt -l` 都干净。全量 `go test -count=1 ./...` 九包全绿；`-run TestObx` 五个包 `-count=3` 全绿。
- **原型只重写了与这次修订相关的用例**：L1-L3 全部；R6 全部，含新增三条；§2.0 装置照写，外加 A1、A9、A15。其余用例这次没有重写，它们的期望与杀手沿用 v1.1 原型的实测。
- 所有 go 命令都照 §4 表头修订后的运行纪律跑。第 1-4 项（文件核对）由驱动脚本每跑完一次就核，次次干净；第 5 项（残留进程）是在变异跑完之后、全部跑完之后各手工核一次，都没有。

变异编号：本次新加 M94-M100。

- **F-A（major）：claude 的「没登录」判据会误伤用 API key 或 Console 账号登录的用户。**
  - 证据（§0 第 20 条）：修订人只读了 `claude.exe`（2.1.283）程序本体，没读任何配置或凭据，复核了 W1b 指的那个拼 `account` 的函数。
    - 这两类用户都能正常使用，`tokenSource` 也是 `none`、也没有 `subscriptionType`，只是多带一个 `apiKeySource`。
    - v1.1 的判据会把他们说成「没登录」，还建议 `claude auth login`。这正是裁决第 3 条的 major。
  - 改 F5：
    - `claudeAccountLine` 加字段 `APIKeySource`（json 名 `apiKeySource`）。
    - 没登录那一支改成：`tokenSource` 是 `none`，而且 `subscriptionType`、`apiKeySource` 都为空。这正好是实测的负面形状，那两类用户落到「登录状态查不出」。
    - 文件头注释与 `claudeLogin` 注释写明：这一条依据程序文本，没有实跑。
  - L1 加 `claude_api_key_env`、`claude_console_login` 两行，期望都是查不出。在 v1.1 的代码上实跑，这两行得到的是没登录（State 2）。
  - §4：
    - 加 M94（去掉 `&& a.APIKeySource == ""`），杀手正是这两行。
    - M82 的锚点行跟着改写，重跑仍被 `claude_none_and_sub` 杀掉。
    - M80、M81、M83 重跑，照旧被杀。
  - 跟着改的：D15、§0 第 14 条表里的结论、§7 (c)、(q)。
  - 证据等级：程序文本加判据单测。本机没有 API key 或 Console 登录态，规则也不许造，所以没有在这两种登录态下实跑。
- **F-B：总结成员加入过又退出时，「它会自己加入做总结」不成立。**
  - 实际行为有两处：
    - `runSummary` 见到退出的总结成员不会拉回，而是打「X 已退出当前会话；要恢复总结用 /join X」（`summary.go:95`）；
    - `resolveAddSegment` 第 0 步放行 `/add X`。
  - v1.1 的 P9、P10 只看它在不在场，这时仍说「它会自己加入」。
  - 改 P6、P9、P10：
    - `printAddCandidates` 多一个参数 `member`，就是 `cmdAdd` 已经建好的那张表。候选为空时先看总结成员是不是加入过又退出。
    - `resolveAddSegment` 前缀没命中、总结成员以这个前缀开头而且不在场时，再分「加入过又退出」与「没加入过」。
    - 两句新说法照 `summary.go:95` 的口径指 `/join`（§3.6）。
  - 顺序冻结：「加入过又退出」排在「一个成员都没有」之前。
    - 这一种与其余成员在不在场无关。
    - 若排在后面：配置里只剩总结成员而它退出了时，会说「配置里还没有可以 /add 的成员」，可 `/add <它>` 其实能加。
    - 新句子写成「除它之外配置里没有可以 /add 的成员」，两种情形都成立。
    - R6 `summary_only_left` 钉住这个顺序（M97）。
  - 同类的别处：
    - P8 头注释说「已退出的成员在候选里」，对总结成员不成立，补一句。
    - P7 的「（与之前逐字节相同）」在 P9 之后不再成立（候选为空时的说法变了），改成指向 `printAddCandidates`。
    - `resolveAddSegment` 第 0 步对**在场**的总结成员也说「多个成员回复之后它会自己加入做总结」。那是 HEAD（CH1）的句子，不在裁决的授权里，登记 §7 (r)。
  - R6：
    - 从八个子测试加到十一个：`summary_left`、`summary_only_left`、`prefix_summary_left`，装置都是加入之后再 `Leave`。
    - §4 加 M95-M100。
    - M71 重跑时 `prefix_summary_left` 也红，杀手列补上；M67-M72 重跑照旧被杀。
  - 新句子的措辞由修订人冻结，可以再改，见 §7 (s)。
  - v1.1 其余用例的期望不动：A10、A11、R1-R5 的配置里都没有 `[summary]`，新的那一支走不到。这是按代码推的，这次没有重跑它们。
- **F-C、F-D：运行纪律补两处缺口**（改 §4 表头、§2.0 H0/H1、§6 第 2、3 步）。
  - F-C：
    - 泄漏的 `Run` 在环境变量还原之后会走到 `LoadRules`，按那一刻的 `os.UserHomeDir()` 去读 `<home>\.council\COUNCIL.md`（`rules.go:40-48`）。父环境的 home 是真的，读的就是 Griffin 的真目录。
    - 这只是读、不落盘，按裁决第 3 条到不了 blocker；但任何文件核对都看不见它，只能靠改环境挡住。
    - 所以父环境的 `USERPROFILE`、`HOME`、`APPDATA`、`LOCALAPPDATA` 一并指到隔离目录下的 canary home。W1b 在这个环境里跑了 H0：canary home 下没有 `.council`。
  - 改了 `APPDATA` 之后，go 读不到 `%APPDATA%\go\env`。本机的 `GOMODCACHE`、`GOCACHE`、`GOTMPDIR`、`GOPROXY` 都写在那份文件里，所以要显式给。go 自己的 telemetry 计数会跟着写进 canary home（修订人实测，是 go 工具的正常行为）。
  - F-D：
    - 修订人用一次性模块做了探针（§0 第 21 条）：测试进程的环境里有 `GOTMPDIR` 时，`t.TempDir()` 建在它下面，不看 `TMP`。
    - 只写在 go 的 env 文件里的 `GOTMPDIR` 传不进测试进程。
    - 所以纪律要求 go 的进程环境里不设 `GOTMPDIR`，泄漏核对再加查 `E:\Go\tmp`。
  - 第五项核对「canary home 下没有 `.council`」照实写：
    - 已知的装置里造不出它为真的情形（泄漏的 `Run` 对 canary home 只读），所以它是兜底，没有正对照。
    - F-C 起作用靠的是改环境，不是这一项核对。
- **F-E：A9 的假 claude 改用 `obxLoopLines`。**
  - 问题：v1.1 的等待循环没有计数上限。测试进程要是在 codex 写标记之前就死了（`obxDrain` 走到 `os.Exit(3)`，或 `go test -timeout`），它会一直占满 CPU。
  - 改法：`obxLoopLines(`not exist "<标记>"`, `type "<claude 夹具>"`)`，与 A12 同一种写法。
  - 原型实测：
    - 未变异时 A9 约 0.07 s；
    - M34（串行）下 A9 在 5.07 s 红，claude 那一行是「超时（5s 内没有回来）」，与 v1.1 是同一个杀法。
  - 同类的别处：§2 里另外两处等待循环（A8、A12）与 L4 `killed` 已经有上限；A6 的假 claude 不循环。
- **F-F：不改代码**，记 §7 (b3) 待主代理。
  - W1b 从 grok 1.0.41 的程序文本读到：`grok models` 的首行是一组互斥的认证状态句，其中「You are logged in with …」是正面信号。
  - 修订人没有复核：程序在 `~/.grok` 下，这次不碰。本机也没有 grok login 态，没实跑过。
  - 裁决要求正面信号「真登录态与没登录时都能稳定区分」，这一句一次都没见过。只凭程序文本加一个「已登录」，万一判错就是裁决第 3 条的 major。
  - F-A 不同：它只是把负面判据收窄到实测的形状，只会把「没登录」变成「查不出」。
- 这次没有动的：A8、A12、L4、X1-X5，以及 v1.1 的其余变异。W1b 核过 v1.1 的其余修订都已落实，包括 F0 的 H0/H1 与 panic / `os.Exit` 探针。

### v1.2（主代理裁决 ob1_rulings_v12.md，2026-09-29）

两处小改，基线仍是 `cbaf195`（之后只有文档提交，代码没动）。变异编号：本次新加 M101-M106。

原型在 `E:\Isolating_test\council\ob1_v12\`，做法如下：
- `git archive cbaf195` 解出一份，用脚本从 v1.1 原文取出 6 个 `file` 块、14 对 `anchor` / `replace` 打上（每个锚点恰好一次）。
- 照 §2 把五个测试文件全部重写：§2.0 与 §2.3 的六段 Go 代码块由脚本从 v1.1 原文逐字插入，L1、L2 的两张表照抄（`\u0007` 那一行落盘后核过是六个 ASCII 字符）。
  - 先在 v1.1 的代码上跑：`-run TestObx` 五个包 90 个用例与子测试全绿；全量 `go test -count=1 ./...` 九包全绿。
  - 改完之后：`-run TestObx` 93 个全绿，`-count=3` 全绿；全量九包全绿；`go vet` 五个包、`gofmt -l ./internal ./cmd` 干净。
- 改完用脚本把 F1、F4 两个整文件从原型写回本文，P11、T4 两对锚点与替换照原型写；再用脚本从本文取回全部 `file` 与 `anchor` / `replace`，打在一份新解出的 `cbaf195` 上：6 个整文件、16 对锚点各恰好一次，13 个文件与原型逐字节相同。§5 的四块另在一份新解出的 `cbaf195` 上逐块累加核过编译与 vet（vet 命中了 go 的缓存，内容与原型逐字节相同）。
- 所有 go 命令都照 §4 表头的运行纪律跑：
  - 四个 profile 变量指 canary home，`COUNCIL_DATA_DIR` 指 canary 数据目录，`TMP`、`TEMP` 指隔离目录；
  - 进程环境里没有 `GOTMPDIR`、`COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOFLAGS`；`GOMODCACHE`、`GOPROXY=off`、`GOCACHE`（隔离目录下）、`GOTOOLCHAIN=local` 显式给；
  - `PATH` 只有 Go 与 System32，去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`、`ANTHROPIC_*`。
  - 每跑完一次核第 1-4 项，次次干净；第 5 项（残留的 `cmd`，按命令行里带原型目录过滤）在全量与变异跑完之后各核一次，都没有。
- 没有调用任何真 CLI。

- **裁决第 1 条：生成的配置写 `data_dir`**（§7 (a) 已定）。
  - 原因：`DefaultDataDir = E:\council-data` 是这台机器专用的（§0 第 12 条）。不写的话，没有 E 盘的新机器上首次运行成功，接着 `store.Open` 失败。本文没有造「没有 E 盘」的用例，这一条只作理由。
  - 冻结（D17）：
    - `FirstRunTOML` 多一个参数 `dataDir`。在 `DefaultTOML()` 与空行之后、`firstRunProvidersComment` 之前，写一行注释 `firstRunDataDirComment`、一行 `data_dir = <dataDir>`、一个空行。
    - 值的写法同 `ProviderBlockTOML` 的 command（`tomlCommandString`：字面量字符串，含单引号或控制字符时退回 `tomlBasicString`）。
    - `firstRun` 传 `filepath.Join(filepath.Dir(path), "data")`。`path` 是 P1 已经要求为绝对路径的默认位置 `<home>\.council\config.toml`，所以值就是 `<home>\.council\data`。
  - 不改：`DefaultDataDir`、`ResolveDataDir`；没写 `data_dir` 的现有配置照旧回落。
  - 跟着改的：F1、F4（整文件）；D7、D14、新增 D17；§0 第 12 条；§1.2；§3.5；§2.0 的 `obxWantFile`、`obxBanner`；C3 拆成 `literal`、`quote_data_dir` 两个子测试；A1 加裁决要的 `LoadAndValidate` 断言。
  - A 系列的期望串：
    - errOut 一个字没变。
    - 配置文件字节经 `obxWantFile` 取，跟着 `FirstRunTOML` 变。
    - 配置里的 `data_dir` 优先于 `COUNCIL_DATA_DIR`（`paths.go:26-42`），所以 A1、A2 的横幅里数据目录从 `<root>\data` 变成 `<home>\.council\data`。
    - A6（准备好的文件没有 `data_dir`）与 A10（显式配置自带 `data_dir`）的数据目录按代码推仍是 `<root>\data`（这两条不比数据目录）；两条不改，实跑全绿。
  - 生成文件的大小：v1.1 在本机四家真实路径下是 2775 字节（§0 第 19 条），与 home 无关；v1.2 与 home 有关，§6 第 5 步那个模拟 home 下是 3114 字节（§0 第 22 条，原型上算出）。
  - 新变异：M101（去掉 `data_dir` 那一行）、M102（`data_dir` 不走 `tomlCommandString`）、M103（app 传错目录）、M106（去掉那一行注释）。
    - M101 的杀手正是裁决点名的两样：C3 `literal` 的字节比对，A1 的 `DataDir` 断言（原型上核过 A1 红的那几行是 `DataDir` 断言与横幅）。
    - A1 的配置文件字节比对是自引用的（`obxWantFile` 也调 `FirstRunTOML`），杀不了 M101，所以字节比对这一半落在 C3。
- **裁决第 2 条：`resolveAddSegment` 第 0 步对在场总结成员的说法**（§7 (r) 已定）。
  - P11：第 0 步拒绝时先看 `active[seg]`。在场的打「X 是总结成员（config.toml 的 [summary]），已经在当前会话里了」；没加入过的仍是 HEAD 那一句；已退出的放行不变（条件一个字不改）。字面量由修订人冻结，见 §7 (s)。
  - 受影响的既有断言：原型上先只套 P11、不改任何测试，全量九包里只有 `TestChxAddHidesSummary/active`（CH1 的 C10，`chx_auto_test.go:731`）红。
    - 授权改这一处（T4），期望换成新句子。
    - 同一个用例的 `explicit`（没加入过）仍用 `rej`（原句），`rej` 照旧用得到。
    - T4 与 P11 同一块（块 C）落盘。
  - R6 加子测试 `summary_present`：同一装置里先 `/add sa-sum`（没加入过，原句，是正对照），再 `obxJoinDirect` sa-sum、`/add sa-sum`（新句子）。
  - 新变异：M104（在场那一支永远不走）、M105（永远走）。这两条的 `-run` 要带上 `TestChxAddHidesSummary`。
  - 落地后主代理在 `19a` 文件头把 R0 与这一条一并标「已由 OB1 修」（§6 第 4 步）。
- **裁决第 3 条**：(b2)、(b3)、(q) 并成 §7 的一个问题 (b2) 给 Griffin，本刀维持「登录状态查不出」加登录办法。上面变更记录里提到 (b3)、(q) 的地方是当时的编号。
- **变异重测**：
  - 新加的 M101-M106 在原型上逐条实跑，点名杀手全部变红（子测试逐个看 `go test -json` 的 fail 事件）。
  - 锚点或杀手受这次改动影响的 M12、M14-M17、M21、M46（锚点行随 F4 改写）、M67-M72、M95-M100 重跑，全部照旧被杀。
    - C3 拆成子测试之后，这几行原来写 C3 的改成 C3 `literal`；M21 另外也让 `quote_data_dir` 红。
    - M46 的杀手多了 A2：重写后的 A2 照 A1 也比配置文件字节与 out（v1.1 原型的 A2 只比 errOut）。
  - 其余各行的「杀手」列沿用 v1.1 与 W1b 的实测，这次没有重跑。
- **H0 / H1（canary 的正对照）在 v1.2 下会变，按代码推演，没有实跑**：
  - v1.1 的 H0 里，泄漏的 `Run` 在已删的临时 home 下重建配置，再按 `COUNCIL_DATA_DIR`（那时已还原成 canary）开库，所以 canary 里出现 `council.db`。
  - v1.2 生成的配置写了 `data_dir = <临时 home>\.council\data`，它优先于环境变量。泄漏的 `Run` 会把库建在重建的临时 home 下，也就是隔离 TMP 里；canary 多半是空的，第 2 项核对（隔离 TMP 下有 `TestObx*`）照样看得见。
  - 修订的硬约束是任何一项核对出现就得停，造泄漏的 H0 不能跑，所以没有重跑。§0 第 18 条、§2.0、§4 表头里 H0 的结论标成 v1.1 的实测；怎么处理见 §7 (u)。

---

**与已落地各刀的相交**（逐个点名；本刀对它们的代码一个字不改，除非下表写了「改」或「加」）：

| 已落地 | 相交的文件 / 函数 | 本刀怎么用 |
|---|---|---|
| MD1 `e9afff9`（16a） | `provider.DiscoverModels`、`provider.DiscoveryCommandFor`、五个 `ErrDiscovery*` 哨兵（`internal/provider/discovery.go`、`discovery_parse.go`）；`turn.RunDiscovery`（`internal/turn/discovery_exec.go`） | 首次运行原样复用做「能不能用」，不改。首次运行经一层闭包截下识别的 stdout 判登录状态（`DiscoverModels` 不交回原始输出） |
| MD1 | `internal/provider`（包约定：不起进程、不读文件、不读环境变量） | **加**：新文件 `login.go`（F5），三档登录状态与三个判据函数，守同一条约定；复用 `claudeDiscoveryResponse`、`hasControlChar`，不改它们 |
| MD1 | `internal/turn`（`capWriter`、`prepareCmd`、`killProcessTree`、`discoveryKillGrace`、`maxDiscoveryStderrBytes`） | **加**：新文件 `loginstatus_exec.go`（F6），新函数 `RunLoginStatus`；`RunDiscovery` 不改 |
| MD1 | `internal/repl/addmodel.go` 的 `discoveryFailureText` | 不改；新文件 `cliadvice.go` 加一个导出薄包装 `DiscoveryFailureText` 给 app 用（失败说法只有一份） |
| MD1 | `internal/repl/commands.go` 的 `cmdAdd`（列表分支与函数头注释） | **改**（P6、P7、P8）：候选表调用多传 `active`、`member`（W1b 修订加 `member`）；识别结果之后接提示行；识别表、失败行、`--refresh` 提示不改 |
| MD1 / CH1 | `internal/repl/commands.go` 的 `printAddCandidates`、`resolveAddSegment` 前缀没命中那一段 | **改**（P9、P10）：裁决第 2 条，候选为空时四种说法、前缀没命中时总结成员那两句（`19a` 文件头 R0；「加入过又退出」那一种是 W1b 修订加的） |
| CH1 | `internal/repl/commands.go` 的 `resolveAddSegment` 第 0 步（`/add <总结成员>`） | **改**（P11，v1.2 裁决第 2 条）：总结成员正在场时换一句「已经在当前会话里了」；没加入过的原句、已退出的放行不变 |
| CH1 测试 | `internal/repl/chx_auto_test.go:731`（C10 `TestChxAddHidesSummary/active`） | **授权改一处**（T4，v1.2）：期望换成 §3.6 的新句子；同一个用例的其余子测试不改 |
| MD1 | `internal/repl/repl.go` 的 `REPL` 结构体末尾（`modelSnap` 之后） | **改**（P5）：加一个字段 `detectCLIs` |
| MD1 | `internal/config/append.go` 的 `tomlBasicString` | 原样复用（路径含单引号时的退路）；`AppendBlock` 不用（首次运行不是追加，是不覆盖地新建） |
| MD1 测试 | `internal/repl/mdx_addmodel_fixture_test.go` 的 `mdxNewRig`、`mdxWantLines`、`mdxHasLine`、`mdxGrokPath`、`mdxAppliedLine`、`mdxAllPresent`、`mdxExec.hook`、`afterCandidates` | 测试腿照用。**授权改一处**（T2）：`mdxR11aBlock` 第一行。`afterCandidates` 只认 `mdxCandHint` 与 `mdxAllPresent`，都不变；今天用它的用例配置里都有成员（全量实跑核过） |
| MD1 测试 | `internal/repl/mdx_addmodel_test.go:590`（R11 (b)） | **授权改一处**（T3）：期望换成「配置里还没有可以 /add 的成员」 |
| CXH `db8c41a`（18a） | codex 的 `custom/` 行（`DiscoveredModel.Custom`） | 首次运行的「识别到 N 个模型」不数 `custom/` 行，与 `/add` 识别表一致 |
| CH1 `4d3dc0b`（19a） | `internal/app/app.go` 两个分支里的 `r.SetAutoReply(!opts.NoAuto)`；`Options.NoAuto`；`[summary]` 总结成员不进 `/add` 候选 | **改**（P2、P3）：锚点带上各分支独有的下一行；上手三步第 3 步按 `NoAuto` 取两句之一；P9、P10 修 R0 的两句错话；P11 改第 0 步对在场总结成员的说法（v1.2） |
| CO1 `1c62dbb` | `app.go` 的 `r.SetConcise(!opts.Verbose)` | 不碰 |
| AT1 / AT1-FU | `internal/tui/*` | 不碰。TUI 模式下首次运行的输出写 `opts.Err`，在 `tui.Run` 之前直写终端（§1.1 D9） |
| Step 4（07 / 07a） | `config.ResolveConfigPath`、`config.DefaultTOML`、`app.reportConfigError`、`config.ResolveDataDir`、`config.DefaultDataDir` | `ResolveConfigPath`、`ResolveDataDir`、`DefaultDataDir` 不改（v1.2 起生成的配置自带 `data_dir`，D17）；`DefaultTOML` **改成中性的全注释文本**（F2）；`reportConfigError` **改**（P4）：去掉打印模板那两行 |
| smoke_test.go 的 `TestMain`（`COUNCIL_FAKE_CLI`） | app 包唯一的 `TestMain` | 不改、不另加。本刀的假 CLI 是只用 cmd 内建命令的 `.cmd` 夹具，不经 `TestMain` |

---

## 0. 病根 / 动机 / 实测

1. **现状（HEAD `cbaf195`，核过真文件）**：`internal/app/app.go:106-114` 加载失败时调 `reportConfigError`；`:338-352` 在默认位置找不到文件时打两行说明、一句「council 不会替你创建这个文件，下面这份模板可以直接抄成 …」和整份 `config.DefaultTOML()`，然后 `Run` 返回错误（`cmd/council/main.go:108-111` 退出码 1）。`DefaultTOML()`（`internal/config/defaults.go:43-103`）是 Griffin 私人的 FSDT 示例（`theory`、`critic`、`[teams.fsdt]`）加他本机的路径注释（`C:\\Users\\25142\\.grok\\bin\\grok.exe`）。`/add` 只识别 `[providers.*]` 里写了的（`internal/repl/addmodel.go:57-70` 的 `discoverableProviders`），不看本机装了什么。
2. **`exec.LookPath` 在这台机器上的实测**（v1 原型的探针程序，go1.27.0；`PATHEXT=.COM;.EXE;.BAT;.CMD;.VBS;.VBE;.JS;.JSE;.WSF;.WSH;.MSC;.CPL`）：

   | id | `exec.LookPath(id)` | 同目录下另有 | `Get-Command -All` |
   |---|---|---|---|
   | claude | `C:\Users\25142\AppData\Roaming\npm\claude.cmd`，err nil，绝对路径 | 无扩展名的 `claude`（sh 脚本）、`claude.ps1` | ps1、cmd、无扩展名三条 |
   | grok | `C:\Users\25142\.grok\bin\grok.exe` | `grok.exe.old`、`agent.exe`、`grove*.exe` | 一条 |
   | agy | `C:\Users\25142\AppData\Local\agy\bin\agy.exe` | 无 | 一条 |
   | codex | `C:\Users\25142\AppData\Roaming\npm\codex.cmd` | 无扩展名的 `codex`、`codex.ps1` | ps1、cmd、无扩展名三条 |

   Go 的 `LookPath` 在 Windows 上只试 `PATHEXT` 里的扩展名，不会选中无扩展名的 sh 脚本，也不会选 `.ps1`（不在 `PATHEXT` 里）；其中 codex 的结果与 #46 记下的、写进 Griffin 配置的 `C:\Users\25142\AppData\Roaming\npm\codex.cmd` 一致，grok 的结果与 HEAD 旧模板注释里的 `C:\Users\25142\.grok\bin\grok.exe` 一致（Griffin 的真配置本刀没有读）。四家一起检测一次约 8 ms（三次 8.6 / 8.0 / 8.0 ms），不起进程。`.cmd` 由 `internal/turn/spawn.go:114-125` 的换行闸与 `CreateProcess` 处理，写绝对路径与 Griffin 手写的配置同一种（#46 真机复验里 `codex.cmd` 已经走通）。npm 全局目录 `node_modules` 下有 `@anthropic-ai\claude-code` 与 `@openai\codex`（这是 §3 安装说明里两个包名的本机证据）；grok 与 agy 在本机没有任何安装方式的证据，不写网址。
3. **四家 CLI 的版本与登录办法**（只跑 `--help` / `--version`，stdin 接空设备，各 1-2 s）：claude `2.1.283 (Claude Code)`，`claude --help` 有 `auth  Manage authentication`，`claude auth --help` 列出 `login [options]  Sign in to your Anthropic account` 与 `status [options]  Show authentication status`；grok `grok 1.0.41 (4220f3b224a6) [stable]`，`grok --help` 有 `login  Sign in to Grok`、`logout`，没有 status、whoami 一类的子命令（v1.1 核过 `grok -h` 的整张子命令表）；codex `codex-cli 0.147.0`，`codex --help` 有 `login  Manage login`，`codex login --help` 列出 `status  Show login status`；agy `1.2.12`，`agy --help`（打在 stderr）与 `agy help models` 里**没有任何 login / auth 子命令**。所以登录办法冻结为 `claude auth login`、`grok login`、`codex login`，agy 用兜底句「agy（按提示登录）」。没有跑任何 login / logout / 改登录态的命令。
4. **真登录态下的识别（基线）**：原型探针调 `provider.DiscoverModels` + `turn.RunDiscovery`，只去掉宿主（Claude 桌面版）注入的 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`，模拟 Griffin 自己的终端：claude 10 个模型（1.70 s；v1.1 复测 1.64 s，10 个），grok 6 个（0.32 s；v1.1 复测 0.31 s，stdout 191 B，首行 `Model 'grok-4.7' is using its own API key.`），agy 14 个（5.35 s），codex 9 行，其中 2 行 `custom/`，所以按 `/add` 的口径是 7 个（0.18 s）。与 #45、#46 的真机记录一致。
5. **「识别成功是否意味着已登录」的隔离实测**（只改子进程环境，全部零花费；另外都去掉 `GEMINI_*`、`GOOGLE_*`、`CPA_*`、`OPENAI_*`、`CODEX_*`、`XAI_*`、`GROK_*`；v1 的隔离目录在 `E:\Isolating_test\council\ob1_draft\ob1probe\iso\`，用完已删）：
   - **claude：识别成功不代表已登录（实测）。** `CLAUDE_CONFIG_DIR` 指空目录：退出码 0、1.08 s、stdout 16651 B，列出 **4 个**模型（`default`→`claude-opus-5-5`、`opus`、`claude-fable-5-1`、`sonnet`、`haiku`；登录态下是 10 个）；initialize 回应里 `account` 只有 `tokenSource`（值 `"none"`）与 `apiProvider`（`"firstParty"`），**没有** `subscriptionType`（登录态下有 `email`、`organization`、`subscriptionType`、`apiProvider`）。再把 `USERPROFILE`、`HOME`、`APPDATA`、`LOCALAPPDATA` 也指空目录：同样 4 个、1.13 s。claude 在隔离目录里建了 `.claude.json`、`backups`、`sessions`。
   - **codex：识别成功不代表已登录（实测）。** `CODEX_HOME` 指空目录：退出码 0、0.16 s、stdout 315324 B，列出 **5 个**（`gpt-5.6-sol` 等，是随 CLI 带的目录；登录态下 7 个，含 `gpt-6-*`）。再把四个 profile 变量也指空目录：同样 5 个。codex 在 `CODEX_HOME` 里建了 `tmp\arg0`。
   - **grok：识别成功不代表已登录（实测）。** `USERPROFILE`、`HOME`、`APPDATA`、`LOCALAPPDATA` 指空目录：退出码 0、2.19 s、stdout 107 B，首行 `You are not authenticated.`，列出 **2 个**（`grok-4.6`、`grok-4.5`）。grok 在空 home 里建了整棵 `.grok`（`config.toml`、`docs`、`logs`、`sessions` 等），前后没有残留 grok 进程。
   - **agy：登录状态未知（隔离无效）。** 只改 `USERPROFILE`、`HOME`：退出码 0、3.37 s，14 个模型；四个 profile 变量全改：退出码 0、4.52 s，14 个。**两次的 stdout 都与登录态基线逐字节相同**（1823 B）。说明 agy 的登录态不在这些目录下（例如在 Windows 凭据管理器里），或者 `models` 根本不需要登录，两者用改环境变量分不开；再往下要读凭据存储，不许做。两次都把子进程的 `PATH` 限成只有 agy 的 bin 目录（挡住常见的开浏览器办法），前后没有新起浏览器进程、没有残留 agy 进程。agy 在空 home 里建了 `.gemini\antigravity-cli`、`.gemini\config`。
   - 结论：四家识别成功都**不能**当成「已登录」；claude、grok、codex 没登录时是「成功但列得少」，不是 `ErrDiscoveryEmpty`。v1 因此只写「识别到 N 个模型」。v1.1 按裁决第 1 条另起一栏登录状态，信号见第 14 条：claude、grok 用识别那一次本来就拿到的输出，codex 另跑 `codex login status`，agy 查不出。MD1 既有的「一个模型都没列出来（没登录？）」是问句，照旧不改。
6. **零成员的配置能加载并通过校验（实测）**：`validate.go` 没有「至少一个成员 / 一个 provider」的规则；原型把 `FirstRunTOML` 的输出与单独的 `DefaultTOML()` 原样写盘再 `config.LoadAndValidate`，都通过（§2 C3、C4）。
7. **新用户的真实形态是 `~/.council` 目录根本不存在（实测）**：这时 `os.Stat(<home>\.council\config.toml)` 返回的错误满足 `errors.Is(err, fs.ErrNotExist)`（Windows 的 ERROR_PATH_NOT_FOUND）；把 `.council` 建成一个普通文件时同样是 `ErrNotExist`，而 `os.MkdirAll` 失败（`mkdir …\.council: The system cannot find the path specified.`）——这就是 §2 A5「写不进去」的确定性夹具。`O_CREATE|O_EXCL` 撞上已有文件时 `errors.Is(err, fs.ErrExist)` 为真（`append.go` 的 `writeBackup` 已靠这一点）。`USERPROFILE` 设成 `<dir>\a<b` 时 `os.Stat` 返回「The filename, directory name, or volume label syntax is incorrect.」，`errors.Is(err, fs.ErrNotExist)` 为假（W1 探针；v1.1 由 A13 在原型上实跑）——这是「Stat 的其它错误」的确定性夹具。
8. **go vet 的坑（原型实测）**：「一家都没找到」那段说明里有 `%APPDATA%`、`%USERPROFILE%`、`%LOCALAPPDATA%`。交给 `fmt.Fprint` 时 `go vet` 的 printf 检查报 `fmt.Fprint call has possible Printf formatting directive %U`，而 `go test` 默认跑这一项，整包测试编译不过。冻结代码用 `io.WriteString`。
9. **顺带发现（同类的别处，不在本刀改）**：`internal/turn/killtree_windows.go:25` 用 `exec.Command("taskkill", ...)`，按 `PATH` 找 taskkill。测试把 `PATH` 限成只有假 CLI 的目录时，超时杀树找不到 taskkill，只杀到 `.cmd` 那一层，孙进程（原型第一次用 `ping` 做超时夹具时）活到自己结束；`RunDiscovery` 等 `discoveryKillGrace`（5 s）才回来。真实用户的 `PATH` 里有 System32，不受影响。§2 的超时夹具因此只用 cmd 内建的循环（没有孙进程），见 §2.0。登记 §7 (d)。`RunLoginStatus` 用同一个 `killProcessTree`，同样受这一条影响。
10. **用 v1 原型二进制模拟一个新用户（零花费）**：`USERPROFILE`、`HOME` 指空目录、`COUNCIL_DATA_DIR` 指隔离目录、`COUNCIL_CONFIG` 不设、去掉宿主变量，`council-ob1.exe < in.txt`（in 只有 `/quit`，行式）：退出码 0、4.7 s；生成的文件 2775 字节（v1 与 v1.1 的生成内容与 home 无关，这个数只由四家的路径定；v1.2 见第 22 条）、无 BOM、LF。**codex 在这个模拟里是 7 个、临时 home 下没有 `.codex`**；而第 5 条里改 `CODEX_HOME` 时是 5 个。只改 `USERPROFILE` / `HOME` 没有把 codex 隔离开，它按什么找自己的默认目录没有查（多半读的仍是 Griffin 的真登录态，[推断]）；agy 同第 5 条。v1.1 的复测见第 19 条。W1b 实测（只改 `USERPROFILE` / `HOME`）：`codex login status` 仍报 `Logged in using ChatGPT`、退出码 0，识别到 7 个模型。所以「codex 读的仍是真登录态」已经不是推断。
11. **TUI 模式的顺序（原型实测，§2 A11）**：`Options{TUI: true, Err: outW, OutFile: outW}`（错误出口与 stdout 是同一根管子），管道里的字节**以首次运行的全部输出开头，紧接横幅第一行「配置文件: …」**，之后才是 bubbletea 的第一帧。这是因为首次运行写 `opts.Err`（`tui.Run` 之前直写），横幅进 `LineWriter` 的 pending、在 `runWith` 第 1 步 `p.Run()` 之前补吐（`internal/tui/run.go:80`）。
12. **数据目录的默认值是本机专用的**：`config.DefaultDataDir = E:\council-data`（`internal/config/schema.go:11`）。
    - v1.1：生成的配置不写 `data_dir`（#48 冻结「只写 `[providers.*]`」），于是 `ResolveDataDir` 回落到环境变量 `COUNCIL_DATA_DIR`，再回落到这个默认值。没有 E 盘的新机器上，首次运行会成功生成配置，随后 `store.Open` 失败、退出码 1。当时登记 §7 (a)。
    - **v1.2 已定（主代理裁决第 1 条，D17）**：生成的配置写一行 `data_dir = '<home>\.council\data'`。`ResolveDataDir` 的顺序是配置里的 `data_dir`、环境变量、默认值（`internal/config/paths.go:26-42`，修订人读过），所以生成的配置此后不看 `COUNCIL_DATA_DIR`。`DefaultDataDir` 不改，没写 `data_dir` 的现有配置照旧回落。
    - **§2 每个走到首次运行的用例仍然必须在 `isolateAppEnv` 之后把 `COUNCIL_DATA_DIR` 设成临时目录**（`isolateAppEnv` 把它清成空串，顺序反了就回落到 E 盘的真数据目录）：A6 准备的文件、T1，以及任何没写 `data_dir` 的路径（例如 M101 下的生成文件）都靠它。起草期间 M33 的泄漏在 `E:\council-data` 新建过一个库（W1 F0），主代理已核实不是 Griffin 的数据并删除（§7 (p)）。
13. **宿主注入了 `NoDefaultCurrentDirectoryInExePath=1`（v1.1 实测）**：本会话的 PowerShell 进程里这个变量是 `1`（Claude 桌面版注入）；W1 核过 Griffin 的 User、Machine 两级都没设。有它时 `exec.LookPath` 根本不看当前目录，v1 与 W1 的全部实测都没覆盖真实终端的当前目录行为。没有它时，当前目录里有 `claude.cmd` 就返回相对路径加 `exec.ErrDot`，而且不再往后查 `PATH`（`lp_windows.go`）。C1 `cwd_planted` 先拿掉它、再断言前提「`LookPath` 返回 `ErrDot`」，免得成为空用例。原型上把它从父环境拿掉，`-run TestObx` 五个包照样全绿。
14. **登录信号探针（v1.1，全部零花费；只改子进程环境；每次 stdin 为空或只给 initialize 那一行，带 60 s 超时）**。「隔离」一律是：去掉宿主变量与第 5 条那几组前缀，`CLAUDE_CONFIG_DIR`、`CODEX_HOME`、`USERPROFILE`、`HOME`、`APPDATA`、`LOCALAPPDATA` 指到 `E:\Isolating_test\council\ob1_v11\probe\` 下新建的空目录。「真登录态」只去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`。email、组织名这类值只记字段名，不记值。

   | 命令 | 环境 | 退出码 | 输出（落在哪一路） | 耗时 | 结论 |
   |---|---|---|---|---|---|
   | claude 识别（initialize 握手） | 真登录态 | 0 | stdout 19660 B；`response.response.account` 有 `apiProvider`（`firstParty`）、`email`、`organization`、`subscriptionType`（`Claude Max`），没有 `tokenSource`；10 个模型 | 1.64 s | 正面信号：`subscriptionType` |
   | claude 识别（initialize 握手） | 隔离 | 0 | stdout 16656 B；`account` 只有 `apiProvider`（`firstParty`）与 `tokenSource`（`none`）；4 个模型 | 1.16 s | 负面标记：`tokenSource` 是 `none`，没有 `subscriptionType`，也没有 `apiKeySource`（W1b 修订加的这一条，见第 20 条） |
   | `claude auth status` | 真登录态 | 0 | stdout 369 B，JSON：`loggedIn` true、`authMethod` `claude.ai`、`apiProvider`、`subscriptionType` `max`，另有 `email`、`orgId`、`orgName`、`configDirectory`、`projectsDirectory`、`analyticsDisabled`；stderr 空 | 0.27 s | 能分开，但多起一个进程；不用 |
   | `claude auth status` | 隔离（两次） | 1 | stdout 317 / 319 B，JSON：`loggedIn` false、`authMethod` `none`；stderr 空 | 0.31 / 0.32 s | 同上 |
   | `codex login status` | 真登录态 | 0 | stdout 空；stderr `Logged in using ChatGPT` | 0.16 s | 正面信号 |
   | `codex login status` | 隔离（两次） | 1 | stdout 空；stderr `Not logged in` | 0.16 / 0.16 s | 负面标记 |
   | `grok models` | 真登录态 | 0 | stdout 191 B，首行 `Model 'grok-4.7' is using its own API key.`；6 个模型 | 0.31 s | 这一行不是登录信号（说的是模型配了 API key），查不出 |
   | `grok models` | 隔离（两次，其中一次另记全文） | 0 | stdout 107 B，首行 `You are not authenticated.`，之后空行、`Default model: grok-4.6`、空行、`Available models:`、`  * grok-4.6 (default)`、`  - grok-4.5`；2 个模型 | 1.86 s | 负面标记：首行 |

   claude 的订阅在两处写法不同：initialize 里是 `Claude Max`，`auth status` 里是 `max`。屏幕显示的是 initialize 里的原值。探针前后没有新起的 claude、grok、codex 残留进程；隔离目录里 claude 建了 `.claude.json` 等、grok 建了 `.grok`，都在隔离目录里，随原型删除。
15. **A8、A12 的假 claude 循环的标定（v1.1 实测）**：只用 cmd 内建命令的循环（`set /a n+=1`、`if %n% geq <上限> exit /b 0`、`if exist "<文件>" goto`）在本机每秒约 3400 次（20000 次 5.85 s、100000 次 29.5 s）。上限取 60000，单独跑约 18 s；在 go test 里（几个包并行、CPU 有争用）更慢：M33 下 A8 等这个循环数到上限，实测 21.25 s（`--- FAIL: TestObxFirstRunFailures (21.25s)`）。A8 的超时是 1 s、A12 是 3 s，快十几倍的机器上也撑得过；又保证无论杀没杀掉，循环都会自己停。未变异时 A8 约 1.04 s、A12 约 0.06 s。
16. **`t.Cleanup` 里 panic 与 `os.Exit` 的区别（v1.1 探针，独立的一次性 Go 模块）**：一个用例先 `t.TempDir()`、`t.Setenv`、起一个轮询环境变量的 goroutine，再注册一个 cleanup。cleanup 里 `panic`：testing 先跑完其余 cleanup（环境变量还原了、临时目录删了），goroutine 看见了还原后的值，然后进程以 panic 退出，后续用例没跑（Go 源码 `testing.go` 的 `runCleanup`：「if a cleanup function panics, we still run the remaining cleanup functions」）。cleanup 里 `os.Exit(3)`：其余 cleanup 一个都没跑，goroutine 没看见还原，临时目录留下，后续用例没跑。所以 `obxDrain` 用 `os.Exit`。
17. **`os.Link` 撞上已有目标（v1.1 实测）**：Windows NTFS 上返回的错误满足 `errors.Is(err, fs.ErrExist)`（C5 的第二次调用、C6 的 16 路并发、A6 的撞车都走这一支）。原型上 C6 的 20 轮 × 16 路每轮恰好一个返回 nil，`-count=3` 零偶发。
18. **canary 的正对照（v1.1 原型实测，H0 / H1）**：同一个装置、只差 `obxDrain` 开没开。装置：M33（没有超时）、`obxRunTimeout` 改成 2 s、再加一个睡 12 s 的后续用例让进程活着；`TMP`、`TEMP` 指到隔离目录下，父进程 `COUNCIL_DATA_DIR` 是 canary。H0（`obxDrain` 一进来就返回，等于 v1 的写法）：A8 在 2 s 看门狗红，测试收尾删掉临时目录、`loop.on` 随之消失、循环停下，泄漏的 `Run` 在已删的临时 home 下重建 `.council\config.toml`（2849 B），再在 canary 里建了 `council.db`（90112 B）与 `council.lock`。H1（照冻结写法）：A8 同样在 2 s 红，canary 空、临时目录没有被重建。H0 留下的东西都在隔离目录里，当场删掉。W1 的证据 (a) 是同一机制的旁证。W1b 在更严的父环境里重跑了一次：四个 profile 变量指到 canary home；`TMP`、`TEMP`、`GOCACHE`、`GOTMPDIR`、`GOPATH` 指到隔离目录。
    - H0：canary 出现 `council.db`（90112 B）与 `council.lock`；已删的临时 home 被重建，`config.toml` 2852 B，建在它设的 `GOTMPDIR` 下（第 21 条）；canary home 下没有 `.council`。
    - H1：全空。
    - 以上都是 v1.1 代码上的实测。v1.2 生成的配置写了 `data_dir`，按代码推，H0 里泄漏的 `Run` 会把库建在重建的临时 home 下，不再落进 canary（变更记录「v1.2」末尾，§7 (u)）；没有实跑。
19. **用 v1.1 原型二进制模拟一个新用户（零花费，同第 10 条的做法）**：退出码 0、5.9 s。stderr：claude `识别到 4 个模型；没登录`，grok `识别到 2 个模型；没登录`，agy `识别到 14 个模型；登录状态查不出`，codex `识别到 7 个模型；已登录`；登录办法列 claude、grok、agy 三家。生成的文件 2775 字节（v1.2 原型上用同样四个路径算 v1.1 的写法，也是 2775；v1.2 的大小见第 22 条），`.council` 下只有 `config.toml`（临时文件删掉了）。codex 在这个模拟里读的仍是 Griffin 的真登录态（第 10 条；起草时是推断，W1b 已实测），所以显示「已登录」。模拟结束后没有残留的 CLI 进程。
20. **claude 的 `account` 在 API key 与 Console 登录时的形状（W1b 修订；程序文本，未实跑）**。
    - 读了什么：修订人只读 `claude.exe`（2.1.283，npm 全局目录 `@anthropic-ai\claude-code\bin` 下）程序本体里 `apiKeySource` 附近的文本，没读任何配置或凭据。
    - 拼 `account` 的函数：
      - 订阅登录时写 `subscription`（回应里是 `subscriptionType`）；
      - 否则来源不是 `profile` 就写 `tokenSource`；
      - 有 API key 时另写 `apiKeySource`；
      - 来源是 `claude.ai`、或 key 来源是 `/login managed key`（Console 登录）时，另写 `organization`、`email`。
    - initialize 回应的 schema 里 `account` 是 `email`、`organization`、`subscriptionType`、`tokenSource`、`apiKeySource`、`apiProvider` 六个可选字段。
    - W1b 另报：`tokenSource` 的来源只在带 claude.ai scope 的 OAuth 令牌时是 `claude.ai`，否则是 `none`。修订人没有复核那个函数本身。
    - 结论：用 `ANTHROPIC_API_KEY` 的用户、用 Console 账号 `claude auth login` 过的用户，都是 `tokenSource` 为 `none`、没有 `subscriptionType`、带 `apiKeySource`。
    - v1.1 的判据把这两种形状判成没登录。原型上这两行判据单测实跑，结果都是 State 2。
    - 本机没有这两种登录态，规则也不许造，没在真实登录态下跑过。
21. **`GOTMPDIR` 与 `t.TempDir()`（W1b 修订，修订人探针，go1.27.0）**。
    - 装置：一次性模块只打印 `os.TempDir()` 与 `t.TempDir()`，父环境的 `TMP`、`TEMP` 指到隔离目录。
    - 不设 `GOTMPDIR`：`t.TempDir()` 在 `TMP` 下。
    - 进程环境里设了 `GOTMPDIR`：`t.TempDir()` 在 `GOTMPDIR` 下，`os.TempDir()` 仍是 `TMP`。
    - `GOTMPDIR` 只写在 go 的 env 文件里（探针把 `APPDATA` 指到隔离目录，在那里放一份只有这一行的 `go\env`）：`t.TempDir()` 在 `TMP` 下，测试进程看到的 `GOTMPDIR` 为空。
    - 本机真实的 `GOTMPDIR`、`GOCACHE`、`GOMODCACHE` 来自 go 的 env 文件（`go env GOENV` 是 `%APPDATA%\go\env`）：
      - `go env` 报出 `E:\Go\tmp`、`E:\Go\cache`、`E:\Go\modcache`；
      - 修订人所在的进程环境里没有这三个变量，这三个值也都不是缺省值；
      - W1b 另核过 `GOTMPDIR` 在进程、User、Machine 三级都没设。
    - 所以今天的默认做法没事。但一旦有人在 shell 里设了 `GOTMPDIR`，「`%TEMP%` 下没有 `TestObx*`」这项核对就永远成立。
    - 另外，改了 `APPDATA` 之后，go 的 telemetry（本机没有 mode 文件，按默认的 local）把计数写进新 `APPDATA` 下的 `go\telemetry`（修订人实测）。
22. **v1.2 原型上的几个事实（修订人，`E:\Isolating_test\council\ob1_v12\`）**。
    - 生成文件的大小：用第 2 条的四个真实路径（只当字符串用，没有碰那几个文件）在原型上算 `FirstRunTOML`。
      - v1.1 的写法是 2775 字节，与第 10、19 条一致。
      - v1.2 的写法、`dataDir` 取 §6 第 5 步的模拟 home `E:\Isolating_test\council\ob1_realcheck\home\.council\data` 时是 3114 字节。多出的 339 字节是注释一行（266 字节）、`data_dir` 一行与一个空行。
    - HEAD 的 `resolveAddSegment` 第 0 步（`commands.go:1029-1034`）对「没加入过」与「在场」打同一句。原型上只套 P11、不改测试时，全量九包只有 `TestChxAddHidesSummary/active` 红。
    - 配置里的 `data_dir` 优先于 `COUNCIL_DATA_DIR`：`paths.go:26-42` 的 `ResolveDataDir`，A1、A2 的横幅实跑印出的是 `<home>\.council\data`。
    - v1.2 修订全程没有调用任何真 CLI。

---

## 1. 范围

### 1.1 做

#### 裁定（主代理定的细节，逐条冻结）

- **D1 触发。** `app.Run` 求出配置路径之后算 `explicit := opts.ConfigPath != "" || os.Getenv(config.EnvConfigPath) != ""`（空串的 `COUNCIL_CONFIG` 与 `ResolveConfigPath` 同一口径，算没设）。`!explicit`、**`filepath.IsAbs(path)`**（v1.1，F5），而且 `os.Stat(path)` 的错误满足 `errors.Is(err, fs.ErrNotExist)` 时进首次运行；Stat 的其它错误（例如没有权限、文件名不合法）不进，交给后面的 `LoadAndValidate` 照常报（§2 A13）；默认路径不是绝对路径（`USERPROFILE` 被设成了相对路径）时也不进，否则会在启动目录下建文件（§2 A14）。`explicit` 为真时行为与 HEAD 逐字节相同（§2 A4）。
- **D2 同类的别处**（还有哪里处理「配置文件不存在」，逐个核过，取舍如下）：

  | 位置 | 今天的行为 | 本刀 |
  |---|---|---|
  | `app.Run` → `reportConfigError`（`app.go:106-114`、`:338-352`） | 默认位置缺文件打模板 | **改**：先走首次运行；`reportConfigError` 去掉模板那两行（P4）。非 explicit 的 NotExist 分支此后只在两种情形可达：竞态（首次运行建好或改用的文件在 `LoadAndValidate` 之前又没了），与默认路径不是绝对路径（A14）；都只打两行说明 |
  | `/config reload`（`internal/repl/configcmd.go:90-94`） | `config.Load` 失败打「重读配置文件失败: …」，旧配置照旧生效 | **不改**：运行中文件被删不是首次运行；悄悄重建一份会让内存与盘上两份配置对不上 |
  | `/add +编号` 的写前守卫（`addmodel.go:404-407`） | `config.Load` 失败打「读配置文件失败: …」 | **不改**，理由同上 |
  | `config.ResolveConfigPath` 取不到用户主目录（`app.go:99-104`） | 报错退出 | **不改**：没有路径就没有「默认位置」 |
  | `config.LoadRules`（全局 / 局部规则文件） | 文件不存在等于没有规则 | 不相干（不是配置文件） |
  | `cmd/council/main.go` 的 `-version`、`-h` | 不加载配置 | 不相干 |
- **D3 检测。** 固定顺序 claude、grok、agy、codex，每家恰好一项（`config.DetectProviders`，F1）。每家先 `exec.LookPath(<id>)`：没有错误**而且**是绝对路径才算找到；否则看一处已知安装位置：claude 与 codex 是 `%APPDATA%\npm\<id>.cmd`，grok 是 `%USERPROFILE%\.grok\bin\grok.exe`，agy 是 `%LOCALAPPDATA%\agy\bin\agy.exe`；那个路径是绝对路径、`os.Stat` 成功、而且是普通文件才算。配置里写的就是这个绝对路径。每次调用都重新检测，不缓存。当前目录里的同名文件：`LookPath` 返回 `exec.ErrDot`，不算找到（§2 C1 `cwd_planted`）；这时 `LookPath` 也不再往后查 `PATH`，`PATH` 上的真安装如果不在三处已知位置就会被判成没找到，本刀不修（§7 (k)）。
- **D4 环境变量为空。** 对应的 `APPDATA` / `USERPROFILE` / `LOCALAPPDATA` 为空时那一处直接算没有，不拼相对路径（`knownInstallPath` 返回空串）；`detectProvider` 另外再要求绝对路径，两道任意一道都挡得住（§4 M7、M8 单独等价，合起来 M9 被杀）。`PATH` 为空时 `LookPath` 自然找不到，走已知位置。`PATHEXT` 为空时用 Go 自己的缺省扩展名（`.com;.exe;.bat;.cmd`），没有夹具，只在审查时看。
- **D5 能不能用。** 找到的每一家各起一个 goroutine 跑 `firstRunCheck`：`context.WithTimeout(ctx, firstRunTimeout)` 一个，识别与登录状态命令共用；`firstRunTimeout` 是包级变量、值 20 s（与 `/add` 的 `discoveryTimeout` 同值；测试可改短）。识别是 `provider.DiscoverModels(cctx, id, path, capture)`，`capture` 包住 `firstRunDiscover`（生产 `turn.RunDiscovery`）、把 stdout 截下来给 D15 用。开始前打一行冻结的「正在检查 … 能不能用……」，**必须在起 goroutine 之前写出**（§2 A12）。全部回来之后再按固定顺序打，不在 goroutine 里写屏幕，结果按下标存、不按完成顺序（§2 A8 钉住）。「识别到 N 个模型」的 N 只数 `Custom` 为假的行（与 `/add` 识别表一致）。失败的说法用 `repl.DiscoveryFailureText(err, firstRunTimeout)`，即 MD1 的四种说法（超时 / 一个模型都没列出来（没登录？）/ 输出认不出来（CLI 版本变了？）/ 其余原样 `err.Error()`）。识别失败的那一家**照样写进配置**（装了就写；登录之后 `/add` 就能识别）。
- **D6 取消。** 识别全部回来之后 `ctx.Err() != nil`：打一行「识别被取消，没有生成配置文件；下次运行 council 会重新检查」，不建目录、不写文件，`firstRun` 返回 `errFirstRunCanceled`，`Run` 见到它返回 `nil`（与 REPL 被取消同一个处置，退出码 0）。
- **D7 写文件。** `config.CreateConfigFile(path, data)`（v1.1，F3）：`os.MkdirAll(filepath.Dir(path), 0o755)`；同一目录里 `os.CreateTemp(dir, <文件名>+".tmp-*")`，写入、关掉；`linkFile(临时文件, path)`（生产就是 `os.Link`）；最后 `defer os.Remove(临时文件)`，成功与失败都删。硬链接在目标已存在时失败，所以**绝不覆盖**，这时的错误满足 `errors.Is(err, fs.ErrExist)`；`path` 一出现就是完整内容，同时启动的另一个 council 撞车改用它时读不到空文件或半个文件（§2 C6、C7 `via_link`）。`linkFile` 因 `ErrExist` 以外的原因失败（例如卷不支持硬链接）时退回 `createExclusive`：v1 的 `O_WRONLY|O_CREATE|O_EXCL` 以 0o644 建 `path`，`Write` 或 `Close` 失败时删掉刚建的文件；仍然绝不覆盖，代价是建出来到写完之间有一个窗口（§2 C7 `fallback`、`fallback_exists`）。`linkFile` 是只为测试留的包级接缝：NTFS 上造不出那种失败。内容是 `config.FirstRunTOML(<配置文件所在目录>\data, detected)`：`DefaultTOML()`、空一行、一行注释 `firstRunDataDirComment` 与一行 `data_dir`（v1.2，D17）、空一行、一行注释「# 下面是 council 第一次运行时在本机找到的工具。」，再按固定顺序每家一段 `config.ProviderBlockTOML(id, path)`，段与段之间空一行；UTF-8 无 BOM、LF。不写任何成员、不写 `[summary]`。`ProviderBlockTOML` 把 command 写成 TOML 字面量字符串（`'…'`，与 Griffin 手写的配置同一种写法）；路径里有单引号或控制字符时字面量字符串装不下，退回 `append.go` 的 `tomlBasicString`（§2 C2 两种都钉）。写之前不在运行时另做加载校验：生成的文本由 §2 C2、C3 在测试里用 `LoadAndValidate` 钉住，写完之后 `Run` 本来就要 `LoadAndValidate` 一次。
- **D8 三种结局**（外加撞车与取消）：
  - 一家都没找到：打头行、四行「没找到」、冻结的安装说明（§3.3），不建目录、不写文件，`firstRun` 返回错误，`Run` 原样返回它（退出码 1）。
  - 写不进去（`CreateConfigFile` 返回的不是 `ErrExist` 的错误）：打「写不进配置文件 <路径>: <错误>」、一句冻结的说明、空一行、再把本来要写的全文（去掉末尾换行）整段打出来，返回错误（退出码 1）。
  - 撞车（`errors.Is(err, fs.ErrExist)`，检查期间被别人建了）：打一行「配置文件在检查期间已经被建好了，改用它: <路径>」加一个空行，返回 `nil`，`Run` 照常加载那个文件。
  - 成功：打「已生成配置文件: …」与上手三步（第 3 步按 `NoAuto` 取两句之一）加一个空行，返回 `nil`，`Run` 照常 `LoadAndValidate`、打横幅、进 TUI 或行式 REPL，不退出。
- **D9 输出流。** 首次运行的**全部**输出写 `Run` 的 `errOut`（`opts.Err`，nil 时是 `io.Discard`），一个字都不写 `out`。理由：TUI 模式下 `Run` 的 `out` 是出口为 nil 的 `LineWriter`，行要等 `tui.Run` 才吐出来，而两种失败结局在那之前就返回了，写进 `out` 的字到不了屏幕；「正在检查」也会被压到识别结束之后才出现。`errOut` 在 Program 起来之前直写终端，单流、顺序天然确定（§0 第 11 条、§2 A11）。代价：`council 2>nul` 会看不到首次运行的说明（§7 (g)）。
- **D10 模板。** `config.DefaultTOML()` 改成中性的**全注释**文本（F2）：去掉 `theory`、`critic`、`[teams.fsdt]` 与 `C:\\Users\\25142\\…` 那行路径注释；保留 Step 4 规格要求必须出现的两件事（`rules_file` 的注释示例，`07_STEP4_SPEC.md:131` 要求的「full 档能写进配置并通过校验，不等于编排器可以自动使用它」）；加上 id 规则、`/add +编号` 挑成员、总结成员怎么配、分组怎么写的注释；内置默认数据目录用 `DefaultDataDir` 拼进去，不手抄。`config.Default()` **不改**。`DefaultTOML` 的调用方核过：生产代码 HEAD 只有 `reportConfigError` 一处（P4 之后没有了），本刀之后只有 `FirstRunTOML` 一处（生成的配置以它开头）；测试里 `defaults_test.go` 三处、`rules_test.go` 一处、`report_test.go` 的 `templateMarker` 注释一处。
- **D11 已有配置的用户（`/add` 提示）。** 不带参数的 `/add` 或 `/add --refresh`：`printDiscoveredModels` 照旧（识别表、失败行、`--refresh` 提示一个字不改）；它返回之后若 `ctx.Err() != nil` 就直接返回（被取消的那一次只该看到「识别被取消」）；否则调 `printUnconfiguredCLIs`：检测器为 nil 时什么都不打；否则对检测出来、`Path` 非空、而且 `r.cfg.Providers` 里没有这个 id 的每一家，按固定顺序打一行冻结的提示（§3.4），再原样打 `config.ProviderBlockTOML(id, path)` 那两行。只打提示，不写文件。配置里已经写了这个 id（不管 command 是什么）就不提示。`/add` 这里不显示登录状态（§7 (o)）。
- **D12 检测何时做。** 不随识别快照缓存：每次 `/add` 列表都现检测一次（约 8 ms，不起进程）；识别被取消的那一次不检测。所以用户照提示追加、`/config reload --apply` 之后，下一次 `/add` 里这一家的提示就没了（§2 R3）。
- **D13 零值与装配。** `REPL.detectCLIs` 的零值 nil 是「不检测、不打提示」，`repl.New` 不装（直接 `New` 出来的 REPL 与既有测试不受本机装了什么影响；照 CO1 `SetConcise`、CH1 `SetAutoReply` 的先例）；`app.Run` 在 TUI 与行式两个分支里都 `r.SetCLIDetector(config.DetectProviders)`。
- **D14 导出与接缝。** 新导出：`config.DetectedProvider`、`config.DetectProviders`、`config.ProviderBlockTOML`、`config.FirstRunTOML`（v1.2 起签名是 `FirstRunTOML(dataDir string, found []DetectedProvider) string`）、`config.CreateConfigFile`、`repl.DiscoveryFailureText`、`(*repl.REPL).SetCLIDetector`；v1.1 加 `provider.LoginState`（`LoginUnknown`、`LoginYes`、`LoginNo`）、`provider.LoginInfo`、`provider.LoginStatusExec`、`provider.LoginFromDiscovery`、`provider.LoginStatusCommandFor`、`provider.ParseLoginStatus`、`turn.RunLoginStatus`。app 包内（测试腿可以用）：`firstRunTimeout`（`var`，`time.Duration`）、`firstRunDiscover`（`var`，`provider.DiscoveryExec`）、`firstRunStatus`（`var`，`provider.LoginStatusExec`）、`errFirstRunCanceled`、`firstRun`、`firstRunCheck`、`firstRunLoginText`。config 包内：`linkFile`（`var`，签名同 `os.Link`）、`createExclusive`、`firstRunDataDirComment`（v1.2）。repl 包内：字段 `detectCLIs`、函数 `printUnconfiguredCLIs`；`printAddCandidates` 多两个参数 `active`、`member`（都是 `map[string]bool`，与 `cmdAdd` 的同名表相同；`member` 是 W1b 修订加的）。
- **D15 登录状态（v1.1，裁决第 1 条）。** 每一家找到的工具一个登录状态，三档之一：**已登录**只在有正面信号时用；**没登录**只在有负面标记时用；**登录状态查不出**是其余一切，包括 agy、字段缺失、形状不认识、CLI 版本不同、识别超时。信号（§0 第 14 条实测）：
  - claude：识别那一次 stdout 里第一份 `request_id` 为 `council-models` 的 `control_response`（与 `parseClaudeDiscovery` 认的是同一行）的 `response.response.account`。`subscriptionType` 非空、不含控制字符、而且 `tokenSource` 不是 `none`：已登录，屏幕带上订阅原值（「已登录（订阅：Claude Max）」），不改写、不猜；`tokenSource` 是 `none`，而且 `subscriptionType`、`apiKeySource` 都为空：没登录；其余（没有这一行、没有 `account`、字段不是字符串、别的组合）查不出。
    - 「`apiKeySource` 为空」是 W1b 修订加的（F-A）。用 API key 或 Console 账号登录的用户也是 `tokenSource` 为 `none`、没有 `subscriptionType`，但带 `apiKeySource`（§0 第 20 条，程序文本）。有了这一条，他们落在查不出，不会被说成没登录。
  - grok：识别那一次 stdout 的首行去掉首尾空白恰好是 `You are not authenticated.`：没登录；其余查不出。登录态下的首行 `Model '<名字>' is using its own API key.` 不当正面信号（§7 (b2)）。
  - codex：识别之后、这一家的 `cctx` 还没到期时，用 `firstRunStatus`（生产 `turn.RunLoginStatus`）跑一次 `codex login status`。退出码 0、而且 stdout 或 stderr 里有一行去掉首尾空白之后以 `Logged in` 开头：已登录；退出码非 0、而且有一行恰好是 `Not logged in`：没登录；起不来、被 ctx 杀掉、其余输出：查不出。
  - agy：查不出。
  - 屏幕：结果行末尾「；<登录状态>」（§3.1）。找到的里面有没登录或查不出的，打一句冻结的说明，再按固定顺序只列这几家的登录办法；找到的全都已登录时这两段都不打。「一家都没找到」的说明不变。
  - `claude auth status` 实测也能分开（§0 第 14 条），但要多起一个进程；裁决要求优先用识别那一次本来就拿到的输出，不用它。
- **D16 `/add` 候选的说法（v1.1，裁决第 2 条；W1b 修订加了「总结成员加入过又退出」）。**
  - `printAddCandidates` 候选为空时，按顺序取第一个成立的：
    1. 有总结成员，而且它在本会话里加入过又退出了（`member[sumID] && !active[sumID]`）→「总结成员 <id> 已退出当前会话，要恢复总结用 /join <id>；除它之外配置里没有可以 /add 的成员」（W1b 修订）。
    2. 配置里除总结成员外一个 agent 都没有 →「配置里还没有可以 /add 的成员」。
    3. 有总结成员、它没加入过（不在场）→「除总结成员 <id> 外，配置里的 agent 都已经在当前会话里了；总结成员不用 /add：多个成员回复之后它会自己加入做总结」。
    4. 其余 →「配置里的 agent 都已经在当前会话里了」（原句，`mdxAllPresent`）。
  - 第 1 种为什么指 `/join`、为什么排最前：
    - `runSummary` 见到退出的总结成员不拉回，打的是「<id> 已退出当前会话；要恢复总结用 /join <id>」（`summary.go:95`）。这时再说「它会自己加入」是假话。
    - 这一种与其余成员在不在场无关。排在第 2 种后面的话，配置里只剩总结成员而它退出了时，会说「配置里还没有可以 /add 的成员」，可 `/add <id>` 由第 0 步放行、其实能加。
    - 「除它之外配置里没有可以 /add 的成员」在「其余都在场」与「一个都没有」两种情形下都成立。§2.2 R6 `summary_only_left` 钉这个顺序（§4 M97）。
  - `resolveAddSegment` 前缀一个候选都没命中时，在「X 开头的都已经在当前会话里了」那一段之前先看：有总结成员、它不在场、而且它的 id 以 X 开头：
    - 加入过又退出的 →「X 开头的除总结成员 <id> 外没有可加入的；<id> 已退出当前会话，要恢复总结用 /join <id>」（W1b 修订）；
    - 没加入过的 →「X 开头的除总结成员 <id> 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结」；
    - 其余照旧：在场的有以 X 开头的，就说都已经在；否则「配置里没有可加入的 X」。
  - 措辞来源：「总结成员不用 /add：多个成员回复之后它会自己加入做总结」与 HEAD 已有的那一句（`resolveAddSegment` 第 0 步）同一种说法；「已退出当前会话，要恢复总结用 /join」照 `summary.go:95`。
  - `resolveAddSegment` 第 0 步（v1.2，主代理裁决第 2 条，P11）：`/add <总结成员的全名>`
    - 它正在场时打「X 是总结成员（config.toml 的 [summary]），已经在当前会话里了」；
    - 没加入过的仍是 HEAD 那一句「X 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结」；
    - 加入过又退出的照旧放行（条件 `!(member[seg] && !active[seg])` 不改）。
- **D17 数据目录（v1.2，主代理裁决第 1 条）。**
  - 生成的配置在 `DefaultTOML()` 与空行之后、`firstRunProvidersComment` 之前，写一行注释（`firstRunDataDirComment`，§3.5）、一行 `data_dir = <值>`、一个空行。`data_dir` 是顶层键，必须写在第一个表之前。
  - 值是 `filepath.Join(filepath.Dir(path), "data")`。首次运行只在 `path` 是默认位置 `<home>\.council\config.toml`、而且是绝对路径时触发（D1），所以值就是 `<home>\.council\data`，与 Griffin 自己配置里的约定一样。
  - 写法同 `ProviderBlockTOML` 的 command：字面量字符串，含单引号或控制字符时退回 `tomlBasicString`（§2 C3 `quote_data_dir`）。
  - `DefaultDataDir`、`ResolveDataDir` 不改；没写 `data_dir` 的现有配置照旧回落。
  - 配置里的 `data_dir` 优先于 `COUNCIL_DATA_DIR`，所以生成的配置此后不看这个环境变量；注释里写明了。

#### 块 A：`internal/config`（两个文件，可单独 `go build ./internal/config`）

**F1. 新文件 `internal/config/firstrun.go`**（逐字）

```file internal/config/firstrun.go
package config

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// firstrun.go 给首次运行自动配置用（00_DECISIONS.md #48，OB1）：找出本机装了
// 哪几家工具、渲染只含 data_dir 与 [providers.*] 的配置文本、不覆盖地建文件。/add 的
// 「本机装了、配置里还没有」提示也用这里的检测与渲染。本文件只看 PATH 与几处
// 已知安装位置，不起任何进程。

// DetectedProvider 是一家工具的检测结果。Path 为空表示没找到；非空时是绝对路径。
type DetectedProvider struct {
	ID   string
	Path string
}

// detectOrder 是检测与屏幕显示的固定顺序。
var detectOrder = []string{"claude", "grok", "agy", "codex"}

// DetectProviders 按 claude、grok、agy、codex 的固定顺序检测，每家恰好一项，
// 没找到的那一项 Path 为空。每次调用都重新检测，不缓存。
func DetectProviders() []DetectedProvider {
	out := make([]DetectedProvider, 0, len(detectOrder))
	for _, id := range detectOrder {
		out = append(out, DetectedProvider{ID: id, Path: detectProvider(id)})
	}
	return out
}

// detectProvider 先 exec.LookPath(id)：没有错误、而且是绝对路径才算找到（LookPath
// 在 Windows 上按 PATHEXT 补扩展名，本机 claude 与 codex 解析成 npm 目录下的 .cmd）。
// 找不到再看 knownInstallPath 给出的那一处：是绝对路径、存在、而且是普通文件才算。
func detectProvider(id string) string {
	if p, err := exec.LookPath(id); err == nil && filepath.IsAbs(p) {
		return p
	}
	p := knownInstallPath(id)
	if p == "" || !filepath.IsAbs(p) {
		return ""
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return p
}

// knownInstallPath 是 PATH 上找不到时再看的那一处：claude 与 codex 是
// %APPDATA%\npm\<id>.cmd（npm 全局安装），grok 是 %USERPROFILE%\.grok\bin\grok.exe，
// agy 是 %LOCALAPPDATA%\agy\bin\agy.exe。对应的环境变量为空时返回空串，不拼相对路径。
func knownInstallPath(id string) string {
	var base string
	var rel []string
	switch id {
	case "claude", "codex":
		base, rel = os.Getenv("APPDATA"), []string{"npm", id + ".cmd"}
	case "grok":
		base, rel = os.Getenv("USERPROFILE"), []string{".grok", "bin", "grok.exe"}
	case "agy":
		base, rel = os.Getenv("LOCALAPPDATA"), []string{"agy", "bin", "agy.exe"}
	default:
		return ""
	}
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, rel...)...)
}

// ProviderBlockTOML 渲染一段 [providers.<id>]：两行，以换行结尾。command 写成
// TOML 字面量字符串（单引号，反斜杠原样，与 Griffin 手写的配置同一种写法）；
// 路径里有单引号或控制字符时字面量字符串装不下，退回 tomlBasicString。
func ProviderBlockTOML(id, command string) string {
	return "[providers." + id + "]\ncommand = " + tomlCommandString(command) + "\n"
}

// tomlCommandString 见 ProviderBlockTOML。FirstRunTOML 写 data_dir 也用它。
func tomlCommandString(s string) string {
	for _, c := range s {
		if c == '\'' || c < 0x20 || c == 0x7f {
			return tomlBasicString(s)
		}
	}
	return "'" + s + "'"
}

// firstRunDataDirComment 是生成的配置里 data_dir 那一行之前的注释。
const firstRunDataDirComment = "# 数据目录（本地库与成员的工作目录），council 第一次运行时定在本文件旁边。可以改成别的绝对路径，重启 council 生效（已有的库不会跟着搬）；写了这一行，环境变量 COUNCIL_DATA_DIR 就不再起作用。\n"

// firstRunProvidersComment 是生成的配置里 data_dir 之后、各段 [providers.*] 之前的那一行注释。
const firstRunProvidersComment = "# 下面是 council 第一次运行时在本机找到的工具。\n"

// FirstRunTOML 返回首次运行要写的配置全文：DefaultTOML()，空一行，
// firstRunDataDirComment 与一行 data_dir = <dataDir>（写法同 ProviderBlockTOML 的 command），
// 空一行，firstRunProvidersComment，再按 found 的顺序每家一段 ProviderBlockTOML，段与段之间
// 空一行。Path 为空的项跳过。不写任何成员，不写 [summary]。
//
// 写 data_dir（OB1 v1.2）：内置默认 DefaultDataDir 是一台机器专用的，不写的话，没有那个盘的
// 机器上首次运行生成了配置，接着打开本地库就失败。data_dir 是顶层键，必须在第一个表之前。
func FirstRunTOML(dataDir string, found []DetectedProvider) string {
	var b strings.Builder
	b.WriteString(DefaultTOML())
	b.WriteString("\n")
	b.WriteString(firstRunDataDirComment)
	b.WriteString("data_dir = " + tomlCommandString(dataDir) + "\n")
	b.WriteString("\n")
	b.WriteString(firstRunProvidersComment)
	n := 0
	for _, d := range found {
		if d.Path == "" {
			continue
		}
		if n > 0 {
			b.WriteString("\n")
		}
		b.WriteString(ProviderBlockTOML(d.ID, d.Path))
		n++
	}
	return b.String()
}

// linkFile 是 CreateConfigFile 用的 os.Link。只为测试留的接缝：NTFS 上造不出
// Link 因 ErrExist 以外的原因失败（例如卷不支持硬链接），退回 createExclusive 的那一支
// 只能换掉它来测。生产代码不改它。
var linkFile = os.Link

// CreateConfigFile 先建 path 所在的目录（MkdirAll，0o755），在同一目录里建一个临时文件
// （os.CreateTemp，名字是 <文件名>.tmp-<随机数字>）写入 data、关掉，再 linkFile(临时文件, path)，
// 最后删掉临时文件的名字（成功与失败都删）。硬链接在目标已存在时失败，所以绝不覆盖已有
// 文件：这时返回的错误满足 errors.Is(err, fs.ErrExist)；而 path 一出现就是完整内容，
// 同时启动的另一个 council 撞车改用它时读不到空文件或半个文件。
//
// linkFile 因 ErrExist 以外的原因失败时退回 createExclusive（O_EXCL 直接建 path）：
// 仍然绝不覆盖，代价是文件建出来到写完之间有一个窗口，那时读到的是空文件或半个文件。
func CreateConfigFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	err = linkFile(tmpPath, path)
	if err == nil || errors.Is(err, fs.ErrExist) {
		return err
	}
	return createExclusive(path, data)
}

// createExclusive 是 CreateConfigFile 的退路：用 O_WRONLY|O_CREATE|O_EXCL 以 0o644 建 path、
// 写入 data，绝不覆盖（目标已存在时返回的错误满足 errors.Is(err, fs.ErrExist)）。Write 或
// Close 失败时删掉这个刚由本函数建出来的文件再返回错误：留下半个文件，下次启动既不会再走
// 首次运行，又加载不了。
func createExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
```

**F2. 整个替换 `internal/config/defaults.go`**（`Default()` 与 HEAD 逐字节相同，只换 `DefaultTOML` 与它的注释；逐字）

```file internal/config/defaults.go
package config

// Default 返回一份能通过 Validate 的最小配置。
// DataDir 留空，由 ResolveDataDir 按环境变量与 DefaultDataDir 回落。
func Default() *Config {
	return &Config{
		DataDir: "",
		// StoreThought 的自然零值是 false，与设计默认值相反（见 schema.go 的
		// 注释），Default() 不经过 Load 的预置逻辑，必须在这里显式写 true。
		StoreThought: true,
		Providers: map[string]ProviderConfig{
			"claude": {Command: "claude"},
		},
		Agents: map[string]AgentConfig{
			"theory": {
				DisplayName: "Theory",
				Provider:    "claude",
				Model:       "opus",
				Effort:      "high",
				Role:        "只做一阶剪切变形理论的推导与校核",
				Permissions: PermissionRead,
				Mode:        ModeDiscussion,
			},
			"critic": {
				DisplayName: "Critic",
				Provider:    "claude",
				Model:       "opus",
				Role:        "质疑推导中的假设与遗漏",
				Permissions: PermissionRead,
				Mode:        ModeDiscussion,
			},
		},
		Teams: map[string]TeamConfig{
			"fsdt": {
				DisplayName: "FSDT 讨论组",
				Members:     []string{"theory", "critic"},
			},
		},
	}
}

// DefaultTOML 返回配置文件开头的说明注释。#48 起全是注释行，没有任何表，
// 单独加载是一份零 provider、零成员的合法配置。首次运行生成的配置以它开头
// （FirstRunTOML），后面接找到的 [providers.*]。
func DefaultTOML() string {
	return `# Council 配置文件。表键即 id，须唯一。
# 手改之后在 council 里敲 /config reload --apply 生效（data_dir 与 rules_file 只能重启生效）。
#
# id 须匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$：可以含点号，但不许以点号开头或结尾；
# id 含点号时表名要加引号，例如 [agents."grok-4.7"]（TOML 把不加引号的点号当成层级）。
# user 与 system 是保留名，不能当 agent id。
#
# data_dir: 数据目录（本地库与成员的工作目录）。填写则必须是绝对路径。
# 不写时按环境变量 COUNCIL_DATA_DIR，再按内置默认 ` + DefaultDataDir + `；目录不存在时 council 启动会建它。
# data_dir = 'D:\council-data'
#
# rules_file: 局部规则文件，相对本文件所在目录解析。
# 全局规则固定在 ~/.council/COUNCIL.md，不在这里写。
# rules_file = 'COUNCIL.md'
#
# store_thought: 是否把 provider 的 thought/思维链摘要存进消息记录。默认 true。
# store_thought = true
#
# [providers.<id>]: council 去哪找这家工具。id 是 claude、grok、agy、codex 之一，
# command 写可执行文件的绝对路径，或 PATH 上的名字。
#
# 成员不用手写：在 council 里敲 /new <标题> 开一个会话，敲 /add 看识别出的模型，
# 再用 /add +编号 挑成员，council 会把那一段 [agents.<id>] 追加到本文件末尾。
# 手写的话照这个格式：
# [agents.sonnet-5-high]
# display_name = 'Sonnet 5'
# provider     = 'claude'
# model        = 'claude-sonnet-5'
# effort       = 'high'
# permissions  = 'read'
# mode         = 'discussion'
# model 与 effort 由对应的工具解释，council 不做枚举校验；effort 可以不写。
# permissions: read / workspace / full，缺省 read。
# 注意：full 档能写进配置并通过校验，不等于编排器可以自动使用它，见 docs/06_SECURITY_MODEL.md 第 2 节。
# mode: discussion / worker / discussion_and_worker / judge / manager，缺省 discussion。
#
# 总结成员：多个成员回复之后，由 [summary] 指定的成员出一段总结，只给你看。
# 先用 /add 加一个成员（它的 permissions 必须是 read），再在本文件末尾加两行：
# [summary]
# agent = '<那个成员的 id>'
#
# 分组：[teams.<id>] 的 members 列出成员 id，/team use <id> 把他们一次加进当前会话。
# [teams.review]
# members = ['sonnet-5-high', 'grok-4.7']
`
}
```

#### 块 B：`internal/provider` 与 `internal/turn`（v1.1 新增；两个新文件，只依赖 HEAD，可单独 `go build ./internal/provider ./internal/turn`）

**F5. 新文件 `internal/provider/login.go`**（逐字）

```file internal/provider/login.go
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

// login.go 是首次运行（00_DECISIONS.md #48，OB1 v1.1）显示的登录状态。与本包其余文件
// 同一条约定：不起进程、不读文件、不读环境变量。claude 与 grok 的信号取自识别那一次
// 本来就拿到的 stdout；codex 的识别输出里没有信号，另跑一次 codex login status，由调用方
// 注入的 LoginStatusExec 执行、把退出码与两路输出交回来判；agy 查不出。
//
// 判据都是 v1.1 探针在本机实测的（claude 2.1.283、grok 1.0.41、codex-cli 0.147.0），
// 只有 claude 没登录那一支的「没有 apiKeySource」取自程序文本（见 claudeLogin）：
// 只有正面信号才算已登录，只有负面标记才算没登录，其余（字段缺失、形状不认识、CLI
// 版本不同、识别超时）一律查不出。

// LoginState 是三档登录状态。零值是查不出。
type LoginState int

const (
	// LoginUnknown 是查不出：没有正面信号，也没有负面标记。
	LoginUnknown LoginState = iota
	// LoginYes 是已登录：有正面信号。
	LoginYes
	// LoginNo 是没登录：有负面标记。
	LoginNo
)

// LoginInfo 是一家的登录状态。Subscription 只有 claude 已登录时非空：initialize 回应里
// account.subscriptionType 的原值（本机实测是 Claude Max），不改写、不猜。
type LoginInfo struct {
	State        LoginState
	Subscription string
}

// LoginStatusExec 是跑登录状态命令的执行器。生产实现是 turn.RunLoginStatus。
// 进程自己以非 0 退出不算错误，退出码照实交回；起不来或被 ctx 杀掉时 err 非 nil。
type LoginStatusExec func(ctx context.Context, cmd DiscoveryCommand) (exitCode int, stdout, stderr []byte, err error)

// grokNotAuthenticated 是 grok models 没登录时 stdout 的首行（实测）。
const grokNotAuthenticated = "You are not authenticated."

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
// 认的是同一行）。account.subscriptionType 非空、不含控制字符、而且 tokenSource 不是 none：
// 已登录，订阅记原值。tokenSource 是 none、subscriptionType 与 apiKeySource 都为空：没登录。
// 其余查不出。
// 实测：登录态下 account 有 email、organization、subscriptionType、apiProvider，没有
// tokenSource；CLAUDE_CONFIG_DIR 指空目录时只有 tokenSource（none）与 apiProvider。
// 用 API key 或 Console 账号登录的用户同样是 tokenSource 为 none、没有 subscriptionType，
// 但带 apiKeySource（claude 2.1.283 的程序文本，未实跑）；要求 apiKeySource 为空，
// 他们就落在查不出，不会被说成没登录。
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
		case a.TokenSource == "none" && a.SubscriptionType == "" && a.APIKeySource == "":
			return LoginInfo{State: LoginNo}
		default:
			return LoginInfo{}
		}
	}
	return LoginInfo{}
}

// grokLogin 只认负面标记：stdout 首行（去掉首尾空白）恰好是 You are not authenticated.
// 就是没登录，其余查不出。登录态下的首行 Model '<名字>' is using its own API key. 说的是
// 那个模型配了自己的 API key，不是登录过，不当正面信号。
func grokLogin(stdout []byte) LoginInfo {
	first, _, _ := strings.Cut(string(stdout), "\n")
	if strings.TrimSpace(first) == grokNotAuthenticated {
		return LoginInfo{State: LoginNo}
	}
	return LoginInfo{}
}

// LoginStatusCommandFor 给出要单独跑一次的登录状态命令。今天只有 codex：codex login status，
// stdin 为空。其余 ok 为假：claude、grok 用识别的输出判，agy 没有零花费的办法。
func LoginStatusCommandFor(providerID, path string) (DiscoveryCommand, bool) {
	if providerID == "codex" {
		return DiscoveryCommand{Path: path, Args: []string{"login", "status"}}, true
	}
	return DiscoveryCommand{}, false
}

// ParseLoginStatus 判登录状态命令的结果。codex：退出码 0、而且 stdout 或 stderr 里有一行
// （去掉首尾空白）以 Logged in 开头，是已登录；退出码非 0、而且有一行恰好是 Not logged in，
// 是没登录；其余查不出。实测两种都打在 stderr：Logged in using ChatGPT（退出码 0）、
// Not logged in（退出码 1）。其余 provider 一律查不出。
func ParseLoginStatus(providerID string, exitCode int, stdout, stderr []byte) LoginInfo {
	if providerID != "codex" {
		return LoginInfo{}
	}
	lines := strings.Split(string(stdout)+"\n"+string(stderr), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
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

**F6. 新文件 `internal/turn/loginstatus_exec.go`**（逐字）

```file internal/turn/loginstatus_exec.go
package turn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"council/internal/provider"
)

// RunLoginStatus 是 provider.LoginStatusExec 的生产实现（OB1 v1.1，今天只给 codex login status 用）。
// 起法与 RunDiscovery 相同：LookPath 解析、工作目录 os.TempDir()、Env 原样继承（要的正是
// 用户真实的登录态）、prepareCmd。stdin 只给 c.Stdin（为空时就是空 stdin，读到即 EOF）；
// CloseStdinAfter 不用。stdout 与 stderr 分开收，各自封顶 maxDiscoveryStderrBytes：
// codex 把状态打在 stderr（实测），RunDiscovery 退出码为 0 时不交回 stderr，所以另写这一个。
//
// 进程自己以非 0 退出不算错误：退出码照实交回，由 provider.ParseLoginStatus 判。
// 起不来、或 ctx 结束（先 killProcessTree 再 Kill，最多等 discoveryKillGrace）时返回错误，
// 这时退出码与输出都不可信，调用方按查不出处理。
func RunLoginStatus(ctx context.Context, c provider.DiscoveryCommand) (int, []byte, []byte, error) {
	if c.Path == "" {
		return 0, nil, nil, errors.New("login status command path is empty")
	}
	path := c.Path
	if resolved, lookErr := exec.LookPath(path); lookErr == nil {
		path = resolved
	}
	cmd := exec.Command(path, c.Args...)
	cmd.Dir = os.TempDir()
	prepareCmd(cmd)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdin = bytes.NewReader(c.Stdin)
	cmd.Stdout = &capWriter{buf: &outBuf, cap: maxDiscoveryStderrBytes}
	cmd.Stderr = &capWriter{buf: &errBuf, cap: maxDiscoveryStderrBytes}
	if err := cmd.Start(); err != nil {
		return 0, nil, nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		if waitErr != nil {
			var ee *exec.ExitError
			if errors.As(waitErr, &ee) {
				return ee.ExitCode(), outBuf.Bytes(), errBuf.Bytes(), nil
			}
			return 0, nil, nil, waitErr
		}
		return 0, outBuf.Bytes(), errBuf.Bytes(), nil
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = killProcessTree(cmd.Process.Pid)
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(discoveryKillGrace):
		}
		return 0, nil, nil, fmt.Errorf("login status killed: %w", ctx.Err())
	}
}
```

#### 块 C：`internal/repl`（v1 块 B 的前一半；依赖块 A）

**F3. 新文件 `internal/repl/cliadvice.go`**（逐字）

```file internal/repl/cliadvice.go
package repl

import (
	"time"

	"council/internal/config"
)

// cliadvice.go 是首次运行自动配置（00_DECISIONS.md #48，OB1）落在 repl 的两件事：
// 把识别失败的说法导出给 app 的首次运行用；/add 列完识别结果之后，对本机装了、
// 配置 [providers] 里还没有的工具各打一行提示和要追加的两行。只打提示，不写文件。

// DiscoveryFailureText 是 discoveryFailureText 的导出包装：识别失败的说法只有
// addmodel.go 那一份，app 的首次运行也用它。
func DiscoveryFailureText(err error, timeout time.Duration) string {
	return discoveryFailureText(err, timeout)
}

// SetCLIDetector 装上 /add 用的本机工具检测。零值 nil 是不检测、不打提示，所以直接
// New 出来的 REPL（既有测试）不受本机装了什么影响；app 装配时装 config.DetectProviders。
// 只许在 Run 或第一次 Submit 之前调。
func (r *REPL) SetCLIDetector(f func() []config.DetectedProvider) {
	r.detectCLIs = f
}

// printUnconfiguredCLIs 在 /add 列完识别结果之后调用。检测不缓存：每次现算
// （只看 PATH 与几处安装位置、不起进程），跟着当前 r.cfg 比，所以追加并
// /config reload --apply 之后提示就不再出现。
func printUnconfiguredCLIs(r *REPL) error {
	if r.detectCLIs == nil {
		return nil
	}
	for _, d := range r.detectCLIs() {
		if d.Path == "" {
			continue
		}
		if _, ok := r.cfg.Providers[d.ID]; ok {
			continue
		}
		if err := r.printf("本机装了 %s（%s），配置里还没有：要用它，把下面两行追加到 %s 末尾，再敲 /config reload --apply 与 /add --refresh\n", d.ID, d.Path, r.rulesCtx.ConfigPath); err != nil {
			return err
		}
		if err := r.write(config.ProviderBlockTOML(d.ID, d.Path)); err != nil {
			return err
		}
	}
	return nil
}
```

**P5. `REPL` 结构体末尾加字段**（`internal/repl/repl.go`，HEAD :118-120）

```anchor internal/repl/repl.go
	// modelSnap 是本会话的识别快照。nil 表示还没识别过。
	modelSnap *modelSnapshot
}
```

```replace
	// modelSnap 是本会话的识别快照。nil 表示还没识别过。
	modelSnap *modelSnapshot
	// detectCLIs 是 /add 用来找「本机装了、配置里还没有」的工具的检测（#48）。
	// 零值 nil 是不检测、不打提示；只经 SetCLIDetector 装，app 装 config.DetectProviders。
	detectCLIs func() []config.DetectedProvider
}
```

**P6. `cmdAdd` 的列表分支**（`internal/repl/commands.go`，HEAD :921-925；v1.1 锚点多带上一句候选表调用，因为它多传一个 `active`）

```anchor internal/repl/commands.go
		if err := printAddCandidates(r, candidates); err != nil {
			return err
		}
		return printDiscoveredModels(ctx, r)
	}
```

```replace
		if err := printAddCandidates(r, candidates, active, member); err != nil {
			return err
		}
		if err := printDiscoveredModels(ctx, r); err != nil {
			return err
		}
		// 识别被取消时不再打提示（#48）：那一次只该看到「识别被取消」。
		if ctx.Err() != nil {
			return nil
		}
		return printUnconfiguredCLIs(r)
	}
```

**P7. `cmdAdd` 头注释**（`internal/repl/commands.go`，HEAD :874）

```anchor internal/repl/commands.go
// （与之前逐字节相同），再打识别出的模型。
```

```replace
// （候选为空时的说法见 printAddCandidates），再打识别出的模型，最后对本机装了、
// 配置里还没有的工具各打一行提示（cliadvice.go，#48）。
```

**P8. `cmdAdd` 头注释里候选的定义**（`internal/repl/commands.go`，HEAD :877-878；v1.1，CH1 R0 的第一处）

```anchor internal/repl/commands.go
// 可一次加多个。候选是「配置里的全部 id，去掉当前在场的成员」，已退出的成员
// 在候选里：Join 对已有的行是 upsert，会把那一行重新激活。
```

```replace
// 可一次加多个。候选是「配置里的全部 id，去掉当前在场的成员与总结成员」，已退出的
// 普通成员在候选里：Join 对已有的行是 upsert，会把那一行重新激活。已退出的总结成员
// 不在候选里，/add <它的全名> 由 resolveAddSegment 第 0 步放行。
```

**P9. `printAddCandidates` 候选为空时的三种说法**（`internal/repl/commands.go`，HEAD :987-994；v1.1）

```anchor internal/repl/commands.go
// printAddCandidates 打不带参数的 /add：候选表，或者「都已经在了」。
//
// display_name 放最后一列：tabwriter 按 rune 计宽，而中文在终端里一个字占两格，
// 放在中间列会把后面的列撑歪；放最后一列就没有后面的列可歪。
func printAddCandidates(r *REPL, candidates []string) error {
	if len(candidates) == 0 {
		return r.printf("配置里的 agent 都已经在当前会话里了\n")
	}
```

```replace
// printAddCandidates 打不带参数的 /add：候选表，或者候选为空时四种说法之一（OB1），
// 按顺序取第一个成立的：总结成员在本会话里加入过又退出了（runSummary 不会再把它拉回来，
// 要恢复得 /join；这一种与其余成员在不在场无关，所以排最前）；配置里除总结成员外一个
// agent 都没有（首次运行生成的配置就是这样）；总结成员没加入过、其余都在场；都在场。
// 前三种以前都打「都已经在当前会话里了」，与事实不符。active、member 与 cmdAdd 的同名表
// 相同：在场的、含已退出在内的全部成员。
//
// display_name 放最后一列：tabwriter 按 rune 计宽，而中文在终端里一个字占两格，
// 放在中间列会把后面的列撑歪；放最后一列就没有后面的列可歪。
func printAddCandidates(r *REPL, candidates []string, active, member map[string]bool) error {
	if len(candidates) == 0 {
		sumID := r.cfg.Summary.Agent
		others := 0
		for id := range r.cfg.Agents {
			if id != sumID {
				others++
			}
		}
		switch {
		case sumID != "" && member[sumID] && !active[sumID]:
			return r.printf("总结成员 %s 已退出当前会话，要恢复总结用 /join %s；除它之外配置里没有可以 /add 的成员\n", sumID, sumID)
		case others == 0:
			return r.printf("配置里还没有可以 /add 的成员\n")
		case sumID != "" && !active[sumID]:
			return r.printf("除总结成员 %s 外，配置里的 agent 都已经在当前会话里了；总结成员不用 /add：多个成员回复之后它会自己加入做总结\n", sumID)
		default:
			return r.printf("配置里的 agent 都已经在当前会话里了\n")
		}
	}
```

**P10. `resolveAddSegment` 前缀没命中时的总结成员那一句**（`internal/repl/commands.go`，HEAD :1078-1080；v1.1）

```anchor internal/repl/commands.go
	// 没找到时区分「以它开头的都已经在场」与「根本没有」：前者用户多半是
	// 想加一个已经在的人，告诉他不用加了，比说「配置里没有」准确。
	for aid := range active {
```

```replace
	// 没找到时区分「以它开头的都已经在场」与「根本没有」：前者用户多半是
	// 想加一个已经在的人，告诉他不用加了，比说「配置里没有」准确。
	// 不在场的总结成员也以它开头时（OB1），先说总结成员：它不是候选，
	// 说「都已经在当前会话里了」是假话。加入过又退出的，runSummary 不会再把它
	// 拉回来，照 runSummary 那句指 /join，不说「它会自己加入」。
	if sumID := r.cfg.Summary.Agent; sumID != "" && !active[sumID] && strings.HasPrefix(sumID, seg) {
		if member[sumID] {
			return "", fmt.Sprintf("%s 开头的除总结成员 %s 外没有可加入的；%s 已退出当前会话，要恢复总结用 /join %s\n", seg, sumID, sumID, sumID)
		}
		return "", fmt.Sprintf("%s 开头的除总结成员 %s 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结\n", seg, sumID)
	}
	for aid := range active {
```

**P11. `resolveAddSegment` 第 0 步对在场总结成员的说法**（`internal/repl/commands.go`，HEAD :1031-1034；v1.2，主代理裁决第 2 条）

```anchor internal/repl/commands.go
	// 指给用户的路，Join 把那一行重新激活。没加入过、或正在场，一律拒绝。
	if sumID := r.cfg.Summary.Agent; sumID != "" && seg == sumID && !(member[seg] && !active[seg]) {
		return "", fmt.Sprintf("%s 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结\n", seg)
	}
```

```replace
	// 指给用户的路，Join 把那一行重新激活。没加入过、或正在场，一律拒绝。
	// 正在场的（OB1 v1.2）说它已经在了：它已经加入，再说「它会自己加入」不准。
	if sumID := r.cfg.Summary.Agent; sumID != "" && seg == sumID && !(member[seg] && !active[seg]) {
		if active[seg] {
			return "", fmt.Sprintf("%s 是总结成员（config.toml 的 [summary]），已经在当前会话里了\n", seg)
		}
		return "", fmt.Sprintf("%s 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结\n", seg)
	}
```

**T2. 授权的既有测试改动**（`internal/repl/mdx_addmodel_fixture_test.go` 的 `mdxR11aBlock` 第一行，HEAD :305；v1.1）

```anchor internal/repl/mdx_addmodel_fixture_test.go
	mdxR11aBlock = `配置里的 agent 都已经在当前会话里了
```

```replace
	mdxR11aBlock = `配置里还没有可以 /add 的成员
```

**T3. 授权的既有测试改动**（`internal/repl/mdx_addmodel_test.go`，HEAD :590；v1.1）

```anchor internal/repl/mdx_addmodel_test.go
		mdxWantLines(t, "(b) /add", g.run("/add"), []string{mdxAllPresent})
```

```replace
		mdxWantLines(t, "(b) /add", g.run("/add"), []string{"配置里还没有可以 /add 的成员"})
```

T2、T3 的理由：R11 的 (a)、(b) 用的配置一个 agent 都没有（`mdxClaudePvTemplate`、`mdxPvOnlyTemplate`），P9 之后第一行是「配置里还没有可以 /add 的成员」。原型上套 P9 而不改这两处，全量 `go test ./internal/repl` 只有 `TestMdxAddSkipsUndiscoverableProviders` 的 (a)、(b) 红（`mdx_addmodel_test.go:582`、`:590`），其余全绿。T2、T3 与 P9 同一次落盘。

**T4. 授权的既有测试改动**（`internal/repl/chx_auto_test.go`，HEAD :731；v1.2，CH1 的 C10 `active`）

```anchor internal/repl/chx_auto_test.go
		smxEq(t, "在场时的 /add smx_s", a.run(t, "/add smx_s"), rej)
```

```replace
		smxEq(t, "在场时的 /add smx_s", a.run(t, "/add smx_s"), "smx_s 是总结成员（config.toml 的 [summary]），已经在当前会话里了\n")
```

T4 的理由：
- CH1 的 C10（`TestChxAddHidesSummary`）`active` 子测试钉的是「总结成员在场时 `/add` 它被拒」，期望是 HEAD 第 0 步的原句。P11 之后在场这一支换了说法，拒绝本身不变。T4 之后它照样挡得住「放行在场」：原型上把第 0 步的条件改成 `!member[seg]`（在场也放行），`active` 红，实际打的是「smx_s 已加入会话 <C>」。
- 原型上只套 P11、不改任何测试时，全量九包只有这一个子测试红。
- 同一个用例的 `explicit`（没加入过）仍用 `rej`（原句），不改；`rej` 照旧用得到，不会出现未用变量。
- T4 与 P11 同一次落盘（块 C）。

#### 块 D：`internal/app`（v1 块 B 的后一半；依赖块 A、块 B、块 C）

**F4. 新文件 `internal/app/firstrun.go`**（逐字）

```file internal/app/firstrun.go
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"council/internal/config"
	"council/internal/provider"
	"council/internal/repl"
	"council/internal/turn"
)

// firstrun.go 是首次运行自动配置（00_DECISIONS.md #48，OB1）。只在默认配置位置、
// 文件不存在时由 Run 调用：找出本机装了哪几家工具，用现有的模型识别（零花费）
// 检查能不能用、看各家的登录状态，生成只含 data_dir 与 [providers.*] 的配置，然后 Run 照常加载它、
// 进 REPL。这推翻了 Step 4「不替用户创建配置文件」，但只在默认位置、文件不存在时；
// 用户用 --config 或 COUNCIL_CONFIG 指定了路径却找不到，照旧报错。
//
// 全部输出写 Run 的 errOut：TUI 模式下 Run 的 out 是出口为 nil 的 LineWriter，
// 行要等 tui.Run 才吐出来，两种失败结局在那之前就返回了，写进 out 的字到不了屏幕；
// errOut 在 Program 起来之前直写终端，而且单流，顺序不会乱。

// firstRunTimeout 是首次运行每一家的超时（识别与登录状态命令共用一个），与 /add 的识别同值。
// 测试可以改短。
var firstRunTimeout = 20 * time.Second

// firstRunDiscover 是首次运行识别用的执行器，生产是 turn.RunDiscovery。
var firstRunDiscover provider.DiscoveryExec = turn.RunDiscovery

// firstRunStatus 是首次运行跑登录状态命令用的执行器，生产是 turn.RunLoginStatus。
var firstRunStatus provider.LoginStatusExec = turn.RunLoginStatus

// errFirstRunCanceled 表示识别期间 ctx 被取消、没有写文件。Run 见到它返回 nil，
// 与 REPL 被取消同一个处置。
var errFirstRunCanceled = errors.New("首次运行被取消")

// firstRunLogin 是各家的登录办法。claude 取自 claude auth --help，grok、codex 取自
// 各自 --help 的 login 子命令；agy 的 --help 里没有登录相关的子命令，写兜底句。
var firstRunLogin = map[string]string{
	"claude": "claude auth login",
	"grok":   "grok login",
	"agy":    "agy（按提示登录）",
	"codex":  "codex login",
}

// msgFirstRunNoneFound 是一家都没找到时的说明。含 %，用 io.WriteString 写：
// 交给 fmt.Fprint 时 go vet 的 printf 检查会把 %U 当成格式动词报错（go test 默认跑这一项）。
// npm 包名是本机 npm 全局目录里 claude.cmd、codex.cmd 对应的包；grok 与 agy 在本机
// 没有安装方式的证据，不写网址。
const msgFirstRunNoneFound = "一个都没找到，没有生成配置文件。至少装一个、在终端里登录好，再运行 council：\n" +
	"  claude  npm install -g @anthropic-ai/claude-code，装好后运行 claude auth login\n" +
	"  grok    照 grok 官方的说明安装，装好后运行 grok login\n" +
	"  agy     照 agy 官方的说明安装，装好后运行 agy，按提示登录\n" +
	"  codex   npm install -g @openai/codex，装好后运行 codex login\n" +
	`council 找的是 PATH，以及这几处默认安装位置：%APPDATA%\npm、%USERPROFILE%\.grok\bin、%LOCALAPPDATA%\agy\bin` + "\n"

// firstRunRow 是一家的检测、识别与登录状态。models 只数不是 custom/ 的行，与 /add 的识别表一致。
type firstRunRow struct {
	d      config.DetectedProvider
	models int
	err    error
	login  provider.LoginInfo
}

// firstRunLoginText 是结果行末尾的登录状态，三档之一；claude 已登录时带上订阅的原值。
func firstRunLoginText(l provider.LoginInfo) string {
	switch l.State {
	case provider.LoginYes:
		if l.Subscription != "" {
			return "已登录（订阅：" + l.Subscription + "）"
		}
		return "已登录"
	case provider.LoginNo:
		return "没登录"
	default:
		return "登录状态查不出"
	}
}

// firstRunCheck 是一家的识别与登录状态，在各自的 goroutine 里跑，识别与登录状态命令共用
// 一个 firstRunTimeout。识别经一层闭包截下 stdout（DiscoverModels 不交回原始输出），
// claude 与 grok 的登录状态从它判；codex 另跑一次 codex login status（识别之后、而且
// 这一家的时间还没用完时才跑），其余查不出。
func firstRunCheck(ctx context.Context, d config.DetectedProvider) firstRunRow {
	cctx, cancel := context.WithTimeout(ctx, firstRunTimeout)
	defer cancel()
	var stdout []byte
	capture := func(c context.Context, cmd provider.DiscoveryCommand) ([]byte, error) {
		out, err := firstRunDiscover(c, cmd)
		stdout = out
		return out, err
	}
	models, err := provider.DiscoverModels(cctx, d.ID, d.Path, capture)
	row := firstRunRow{d: d, err: err, login: provider.LoginFromDiscovery(d.ID, stdout)}
	for _, m := range models {
		if !m.Custom {
			row.models++
		}
	}
	if sc, ok := provider.LoginStatusCommandFor(d.ID, d.Path); ok && cctx.Err() == nil {
		if code, so, se, serr := firstRunStatus(cctx, sc); serr == nil {
			row.login = provider.ParseLoginStatus(d.ID, code, so, se)
		}
	}
	return row
}

// firstRun 的顺序冻结：打头行；检测；一家都没有就列四行「没找到」、打说明、返回错误；
// 否则打「正在检查」，四家并行识别并看登录状态（各自带 firstRunTimeout），全部回来之后
// ctx 已取消就打一行、返回 errFirstRunCanceled，不写文件；再按固定顺序打四行结果；
// 找到的里面有没登录或查不出的，打一句说明和那几家的登录办法；最后建文件。文件已存在
// （检查期间被别人建了）就说一句、返回 nil，由 Run 加载那个文件；写不进去就把整段内容
// 打出来、返回错误；写成了就打上手三步、返回 nil。
func firstRun(ctx context.Context, w io.Writer, path string, noAuto bool) error {
	fmt.Fprintf(w, "第一次运行：还没有配置文件 %s，council 来生成一份。\n", path)
	detected := config.DetectProviders()
	var ids []string
	for _, d := range detected {
		if d.Path != "" {
			ids = append(ids, d.ID)
		}
	}
	if len(ids) == 0 {
		for _, d := range detected {
			fmt.Fprintf(w, "  %-6s  没找到\n", d.ID)
		}
		_, _ = io.WriteString(w, msgFirstRunNoneFound)
		return errors.New("首次运行：本机没找到 claude、grok、agy、codex 中的任何一个")
	}

	fmt.Fprintf(w, "正在检查 %s 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 %v）……\n", strings.Join(ids, "、"), firstRunTimeout)
	rows := make([]firstRunRow, len(detected))
	var wg sync.WaitGroup
	for i, d := range detected {
		rows[i].d = d
		if d.Path == "" {
			continue
		}
		wg.Add(1)
		go func(i int, d config.DetectedProvider) {
			defer wg.Done()
			rows[i] = firstRunCheck(ctx, d)
		}(i, d)
	}
	wg.Wait()
	if ctx.Err() != nil {
		fmt.Fprintf(w, "识别被取消，没有生成配置文件；下次运行 council 会重新检查\n")
		return errFirstRunCanceled
	}

	var needLogin []string
	for _, row := range rows {
		switch {
		case row.d.Path == "":
			fmt.Fprintf(w, "  %-6s  没找到\n", row.d.ID)
			continue
		case row.err != nil:
			fmt.Fprintf(w, "  %-6s  %s  识别失败：%s；%s\n", row.d.ID, row.d.Path, repl.DiscoveryFailureText(row.err, firstRunTimeout), firstRunLoginText(row.login))
		default:
			fmt.Fprintf(w, "  %-6s  %s  识别到 %d 个模型；%s\n", row.d.ID, row.d.Path, row.models, firstRunLoginText(row.login))
		}
		if row.login.State != provider.LoginYes {
			needLogin = append(needLogin, row.d.ID)
		}
	}
	if len(needLogin) > 0 {
		fmt.Fprintf(w, "能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：\n")
		for _, id := range needLogin {
			fmt.Fprintf(w, "  %-6s  %s\n", id, firstRunLogin[id])
		}
	}

	// data_dir 写配置文件旁边的 data（OB1 v1.2）：path 是默认位置 <home>\.council\config.toml，
	// Run 已经要求它是绝对路径，所以这就是 <home>\.council\data。
	text := config.FirstRunTOML(filepath.Join(filepath.Dir(path), "data"), detected)
	if err := config.CreateConfigFile(path, []byte(text)); err != nil {
		if errors.Is(err, fs.ErrExist) {
			fmt.Fprintf(w, "配置文件在检查期间已经被建好了，改用它: %s\n\n", path)
			return nil
		}
		fmt.Fprintf(w, "写不进配置文件 %s: %v\n", path, err)
		fmt.Fprintf(w, "下面是本来要写的内容，请自己存成 %s（UTF-8），再运行 council：\n\n", path)
		fmt.Fprintln(w, strings.TrimRight(text, "\n"))
		return fmt.Errorf("首次运行：写不进配置文件 %s: %w", path, err)
	}
	fmt.Fprintf(w, "已生成配置文件: %s（只写了找到的工具，还没有成员）\n", path)
	fmt.Fprintf(w, "上手三步：\n")
	fmt.Fprintf(w, "  1. /new <标题>  开一个会话\n")
	fmt.Fprintf(w, "  2. /add  看识别出的模型，再用 /add +编号 挑成员\n")
	if noAuto {
		fmt.Fprintf(w, "  3. 直接打字只记下（你用了 -no-auto）；/auto on 之后，直接打字会让会话里的成员回复\n")
	} else {
		fmt.Fprintf(w, "  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个\n")
	}
	fmt.Fprintln(w)
	return nil
}
```

**P0. `Run` 的文档注释**（`internal/app/app.go`，HEAD :61-62）

```anchor internal/app/app.go
// Run 按固定顺序做五件事：求配置文件路径 -> 加载并校验 -> 求数据目录 ->
// 打开 store（含建目录与迁移）-> 起 REPL，退出时关闭 store。
```

```replace
// Run 按固定顺序做五件事：求配置文件路径 -> 加载并校验 -> 求数据目录 ->
// 打开 store（含建目录与迁移）-> 起 REPL，退出时关闭 store。
// 走默认位置而那个文件不存在时，求完路径先走首次运行（firstrun.go，#48），
// 生成配置之后照常往下；首次运行的两种失败结局在这里返回错误，被取消返回 nil。
```

**P1. 触发首次运行**（`internal/app/app.go`，HEAD :106-114；`explicit` 挪到前面、只算一次，原来那三行注释删掉，理由搬进新注释；v1.1 加 `filepath.IsAbs(path)`）

```anchor internal/app/app.go
	cfg, err := config.LoadAndValidate(path)
	if err != nil {
		// 只有在「用户没指定过配置位置」时才把整份模板打出来，那才是真正的首次运行。
		// 用户明确写了 --config 或设了环境变量却找不到文件，多半是打错字，
		// 这时候刷几十行模板只会把那一行真正有用的路径淹掉。
		explicit := opts.ConfigPath != "" || os.Getenv(config.EnvConfigPath) != ""
		reportConfigError(errOut, path, err, explicit)
		return fmt.Errorf("加载配置失败 (%s): %w", path, err)
	}
```

```replace
	// explicit 为真表示用户自己指定过配置位置：--config，或非空的 COUNCIL_CONFIG
	// （空串与 ResolveConfigPath 同一口径，算没设）。
	explicit := opts.ConfigPath != "" || os.Getenv(config.EnvConfigPath) != ""
	// 首次运行（#48）：走默认位置、而且那个文件不存在，才替用户生成配置。
	// 用户指定了路径却找不到，多半是打错字，照旧报错、不建文件。
	// Stat 的其它错误（例如没有权限、文件名不合法）不进首次运行，交给下面的
	// LoadAndValidate 照常报。默认路径不是绝对路径（USERPROFILE 被设成了相对路径）时
	// 也不进：那样会在启动目录下建文件。
	if !explicit && filepath.IsAbs(path) {
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			if frErr := firstRun(ctx, errOut, path, opts.NoAuto); frErr != nil {
				if errors.Is(frErr, errFirstRunCanceled) {
					return nil
				}
				return frErr
			}
		}
	}

	cfg, err := config.LoadAndValidate(path)
	if err != nil {
		reportConfigError(errOut, path, err, explicit)
		return fmt.Errorf("加载配置失败 (%s): %w", path, err)
	}
```

**P2. TUI 分支装检测器**（`internal/app/app.go`，HEAD :236-237；锚点带上本分支独有的 `err = tui.Run(` 那一行，因为 `r.SetAutoReply(!opts.NoAuto)` 在两个分支里各出现一次）

```anchor internal/app/app.go
		r.SetAutoReply(!opts.NoAuto)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
```

```replace
		r.SetAutoReply(!opts.NoAuto)
		r.SetCLIDetector(config.DetectProviders)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
```

**P3. 行式分支装检测器**（`internal/app/app.go`，HEAD :242-243；同上，带 `err = r.Run(ctx)`）

```anchor internal/app/app.go
		r.SetAutoReply(!opts.NoAuto)
		err = r.Run(ctx)
```

```replace
		r.SetAutoReply(!opts.NoAuto)
		r.SetCLIDetector(config.DetectProviders)
		err = r.Run(ctx)
```

**P4. `reportConfigError` 去掉模板**（`internal/app/app.go`，HEAD :336-349）

```anchor internal/app/app.go
// explicit 为真表示用户自己指定过配置位置（--config 或环境变量）。
// 这种情况下不打印模板：他要的是「你找的是这个路径」，不是一屏样例。
func reportConfigError(w io.Writer, path string, err error, explicit bool) {
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(w, "找不到配置文件: %s\n", path)
		if explicit {
			fmt.Fprintf(w, "这个路径是你用 --config 或环境变量 %s 指定的，请确认它没写错。\n", config.EnvConfigPath)
			return
		}
		fmt.Fprintf(w, "可以用 --config <path> 指定别处的配置文件，或设置环境变量 %s 指向它。\n", config.EnvConfigPath)
		fmt.Fprintf(w, "council 不会替你创建这个文件，下面这份模板可以直接抄成 %s:\n\n", path)
		fmt.Fprintln(w, strings.TrimRight(config.DefaultTOML(), "\n"))
		return
	}
```

```replace
// explicit 为真表示用户自己指定过配置位置（--config 或环境变量），这时点明
// 路径是他指定的。默认位置找不到文件的情况，Run 已经先走过首次运行（#48）；
// 还能走到这里的只有竞态（首次运行建好或改用的文件，在 LoadAndValidate 之前
// 又没了），不再打印模板。
func reportConfigError(w io.Writer, path string, err error, explicit bool) {
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(w, "找不到配置文件: %s\n", path)
		if explicit {
			fmt.Fprintf(w, "这个路径是你用 --config 或环境变量 %s 指定的，请确认它没写错。\n", config.EnvConfigPath)
			return
		}
		fmt.Fprintf(w, "可以用 --config <path> 指定别处的配置文件，或设置环境变量 %s 指向它。\n", config.EnvConfigPath)
		return
	}
```

**T1. 授权的既有测试改动**（`internal/app/report_test.go` 的第 3 个子测试，HEAD :66-91；由实现腿与生产代码**同一次交付**，集成员同一次落盘）

```anchor internal/app/report_test.go
	t.Run("完全没指定过，走默认路径时打印模板", func(t *testing.T) {
		home := t.TempDir()
		// os.UserHomeDir 在 Windows 上读 USERPROFILE，在类 Unix 上读 HOME。
		// 两个都设上，这个用例就不挑平台。
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOME", home)
		t.Setenv(config.EnvConfigPath, "")

		var out, errOut bytes.Buffer
		err := Run(context.Background(), Options{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errOut,
		})
		if err == nil {
			t.Fatalf("默认位置没有配置文件时 Run 应当返回错误")
		}

		got := errOut.String()
		if !strings.Contains(got, templateMarker) {
			t.Errorf("首次运行（未指定过配置位置）应当打印模板，实际:\n%s", got)
		}
		if !strings.Contains(got, filepath.Join(home, ".council", "config.toml")) {
			t.Errorf("错误输出里应当含默认配置路径，实际:\n%s", got)
		}
	})
```

```replace
	t.Run("完全没指定过，走默认路径时进首次运行（本机一家工具都没有）", func(t *testing.T) {
		home := t.TempDir()
		empty := t.TempDir()
		// os.UserHomeDir 在 Windows 上读 USERPROFILE，在类 Unix 上读 HOME。
		// 两个都设上，这个用例就不挑平台。
		// #48 起默认位置没有文件会进首次运行、检测本机装了哪些工具：APPDATA、
		// LOCALAPPDATA、PATH 也指到空的临时目录，不许看见跑测试这台机器上真装着的 CLI；
		// COUNCIL_DATA_DIR 指到临时目录，万一走到 store.Open 也不碰内置默认的数据目录。
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOME", home)
		t.Setenv("APPDATA", empty)
		t.Setenv("LOCALAPPDATA", empty)
		t.Setenv("PATH", empty)
		t.Setenv(config.EnvConfigPath, "")
		t.Setenv(config.EnvDataDir, filepath.Join(t.TempDir(), "data"))

		var out, errOut bytes.Buffer
		err := Run(context.Background(), Options{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errOut,
		})
		if err == nil {
			t.Fatalf("默认位置没有配置文件、本机一家工具都没有时 Run 应当返回错误")
		}

		got := errOut.String()
		if strings.Contains(got, templateMarker) {
			t.Errorf("#48 起不再打印模板，实际:\n%s", got)
		}
		if !strings.Contains(got, filepath.Join(home, ".council", "config.toml")) {
			t.Errorf("错误输出里应当含默认配置路径，实际:\n%s", got)
		}
		if !strings.Contains(got, "一个都没找到，没有生成配置文件。") {
			t.Errorf("应当走首次运行「一家都没找到」的结局，实际:\n%s", got)
		}
		if _, statErr := os.Stat(filepath.Join(home, ".council")); !os.IsNotExist(statErr) {
			t.Errorf("一家都没找到时不该建 .council 目录，Stat 结果: %v", statErr)
		}
	})
```

理由：HEAD 的这个子测试只重定向了 `USERPROFILE`、`HOME`，`PATH` 是真的、`COUNCIL_DATA_DIR` 没设。套上本刀的生产代码之后它会从测试进程里真起这台机器上的四家 CLI 做识别（v1.1 起还有一次 `codex login status`）、把配置写进临时 home、然后 `store.Open(E:\council-data)`。**所以 T1 必须与 P1 同一次落盘，任何时候都不许拿 HEAD 的 `report_test.go` 去跑新代码。** 新写法把四个 profile 变量与 `PATH` 指到空临时目录、`COUNCIL_DATA_DIR` 指到临时目录，期望走「一家都没找到」：Run 返回错误、不打模板、含默认路径、含「一个都没找到，没有生成配置文件。」、不建 `.council`。

### 1.2 不做

- 不做自动登录；不读、不写任何 CLI 的凭据或配置文件（检测只看 `PATH` 与三处可执行文件的位置；识别复用 MD1 的零花费命令；登录状态只用识别的输出与 `codex login status`；子进程 `Env` 照旧原样继承）。不跑 `login` / `logout`，不开浏览器。
- 不自动加成员，不自动配总结成员，不写 `[summary]`、`[teams.*]`（`data_dir` 从 v1.2 起写，D17）。
- 不加新命令、新 flag（没有 `council init`、`-check`）。
- 不改 `config.Default()`；不改 MD1 的识别逻辑、识别表格式、失败行、`--refresh` 提示（`/add` 只在表后多提示行）；`/add` 的识别表不显示登录状态；不改 `ResolveConfigPath`、`ResolveDataDir`、`DefaultDataDir`；不改 `/config reload` 与 `/add +编号` 守卫对「文件不存在」的处置（D2）。
- `internal/provider`、`internal/turn` 只各加一个新文件（F5、F6），既有文件一个字不改；`internal/tui`、`internal/store`、`internal/conversation`、`cmd/council` 的任何文件不改。
- **既有 `_test.go` 一律不改**，授权的只有 T1、T2、T3、T4（T4 是 v1.2 加的）。下面两处「仍绿但变空」照实登记、**不改**：
  - `internal/config/defaults_test.go` 的 `TestDefaultTOMLPermissionsAndModeAreLowercase`：模板不再有成员，循环体一次都不跑，永远通过。它原本要钉的事由 §2 C4 接替（模板单独加载是零 provider、零 agent、零 team）。
  - `internal/app/report_test.go` 子测试 1、2 的「不打印模板」缺席断言：P4 之后没有任何代码路径打印模板，它们失去了正对照（原来的正对照是子测试 3）。它们对「显式路径那两行」的断言仍有效；那两行的逐字节断言由 §2 A4 担保。
  - 其余 `DefaultTOML` 相关用例在原型上照旧有意义且全绿：`TestDefaultTOMLLoadsAndValidates`（全注释文本能加载校验）、`TestDefaultTOMLHasNoDecorativeSymbols`、`TestDefaultTOMLMentionsRulesFile`（新模板仍含 `rules_file`）。

---

## 2. 验收（测试腿新写的文件）

五个新文件：`internal/config/obx_firstrun_test.go`（`package config`）、`internal/repl/obx_cliadvice_test.go`（`package repl`）、`internal/app/obx_firstrun_app_test.go`（`package app`）、v1.1 新增 `internal/provider/obx_login_test.go`（`package provider`）与 `internal/turn/obx_loginstatus_exec_test.go`（`package turn`）。包级标识符一律 `obx` 开头；**不许新增 `TestMain`**（app 的在 `smoke_test.go`、repl 的在 `session_inflight_test.go`、turn 的在 `mdx_discovery_exec_test.go`；config 与 provider 包 HEAD 上没有 `TestMain`，本刀也不加）；不许改任何既有测试文件（T1-T4 由实现腿交付）。测试函数与子测试的名字照下面写（§4 按名字点杀手）。

纪律：
- **字面量整行相等**：屏幕输出与文件字节一律整段或整行 `==` 比较；只有写失败那一行（A5）与文件名不合法那一行（A13）之后的操作系统错误文本（随系统语言变）不比。
- **每条都能为假**：每条用例写明它对哪条变异红（§4）；缺席断言都配同一装置的正对照。
- **真用户目录零写入**：凡是会走到默认配置位置、或会检测本机工具的用例，**第一件事**调 `obxGuardRealHome(t)`（见 §2.0），然后才 `t.Setenv`；`USERPROFILE`、`HOME`、`APPDATA`、`LOCALAPPDATA` 指到 `t.TempDir()` 下的目录，`PATH` **只放**一个临时 bin 目录，`COUNCIL_CONFIG` 设成空串（`isolateAppEnv` 做），**然后**再把 `COUNCIL_DATA_DIR` 设成临时目录（顺序反了会被 `isolateAppEnv` 清成空串，回落到 `E:\council-data`）。
- **收尾不泄漏（v1.1，F0）**：凡是另起 goroutine 调 `Run` 的用例，一律经 `obxRun` / `obxRunTee`（A11 直接用 `obxDrain`），不许自己另写看门狗；`t.Fatalf` 只许在测试 goroutine 里调。
- 假 CLI 一律是写在临时 bin 目录里的 `.cmd`，只用 cmd 内建命令（`type`、`echo`、`if`、`goto`、`copy`、`mkdir`、`set`、`exit`），不调外部程序（`PATH` 里没有 System32，见 §0 第 9 条）。不经 `TestMain`，不设 `COUNCIL_FAKE_CLI`。任何用例都不许调用真 CLI。

### 2.0 共用装置与夹具（app 包；照写）

`obxGuardRealHome(t)`：在任何 `t.Setenv` 之前取 `os.UserHomeDir()`（此刻是真用户目录），记下 `obxStatHome(real)`；`t.Cleanup` 里再取一次，不相等就 `t.Errorf`。`obxStatHome(home)` 返回一个可比较的结构体：`<home>\.council` 是否是目录、`<home>\.council\config.toml` 是否存在及其 `Size()`、`ModTime()`。只 `os.Stat`，不打开真文件、不读内容。

**守卫的限制，照实写**：Griffin 的机器上真配置已经存在，守卫只能看出「被改了」（大小、修改时间变了），看不出「被建了」——而硬链接与 `O_EXCL` 本来就不会覆盖已有文件、首次运行在那台机器上根本不会触发。守卫真正起作用是在没有真配置的机器上（CI 或新用户）看出「被建了」。真正挡住写真目录的是上面的环境隔离与 `COUNCIL_DATA_DIR` 的顺序；守卫是最后一道。正对照：`TestObxGuardSeesCreation`（下面 A0）。

`obxIsolate(t) *obxEnv`：先 `obxGuardRealHome(t)`；在一个 `t.TempDir()` 下建 `home`、`appdata`、`local`、`bin`、`fx` 五个目录（`data` 不预建）；`isolateAppEnv(t)`；`t.Setenv` 设 `USERPROFILE` 与 `HOME` 为 home、`APPDATA`、`LOCALAPPDATA`、`PATH` 为 bin（只有它）、`config.EnvDataDir` 为 `<root>\data`；`cfgPath = <home>\.council\config.toml`。

`obxFake(t, dir, id, lines...)`：写 `<dir>\<id>.cmd`，各行以 `\r\n` 连接、末尾再加 `\r\n`。`obxFixture(t, e, name, content)`：写 `<fx>\<name>`。`obxTypeFake(t, e, id, content)`：写夹具 `<fx>\<id>.out`，再写只有一行 `@type "<夹具绝对路径>"` 的假 CLI。`obxEntries(t, dir)`：`os.ReadDir` 的名字列表。

**收尾（v1.1，F0；逐字）**：

```go
// obxRunTimeout 是 obxRun 的看门狗；obxDrainTimeout 是收尾时 cancel 之后等 Run 返回的上限。
const (
	obxRunTimeout   = 30 * time.Second
	obxDrainTimeout = 10 * time.Second
)

// obxDrain 是 obxRun 与 A11 共用的收尾：先 cancel()，再最多等 obxDrainTimeout 让 Run 返回；
// 等不到就 os.Exit 结束整个测试进程。不用 panic：testing 在 cleanup panic 之后照样跑完
// 其余 cleanup（还原环境变量、删临时目录）才退出，还没返回的 Run 正好在这个窗口里读到
// 还原后的环境（v1.1 探针实测）；os.Exit 之后什么都不再执行。
func obxDrain(cancel context.CancelFunc, finished <-chan struct{}, who string) {
	cancel()
	select {
	case <-finished:
	case <-time.After(obxDrainTimeout):
		fmt.Fprintf(os.Stderr, "%s：cancel() 之后 %v Run 仍没有返回；为了不让它在环境变量还原之后接着写盘，直接结束测试进程\n", who, obxDrainTimeout)
		os.Exit(3)
	}
}

// obxRun 把 Out、Err 换成两个 strings.Builder，另起 goroutine 调 Run，返回 (out, errOut, err)。
// 交给 Run 的 ctx 由 ctx 派生、可取消；本函数在内部注册一个 t.Cleanup 调 obxDrain。它比
// obxIsolate 晚注册，按后进先出先于环境变量还原与删临时目录执行，用例体里任何 Fatal 路径
// 都盖得到。看门狗只在测试 goroutine 里 select，不在别的 goroutine 里调 t.Fatalf。
func obxRun(t *testing.T, ctx context.Context, opts Options) (string, string, error) {
	t.Helper()
	return obxRunTee(t, ctx, opts, nil)
}

// obxRunTee 同 obxRun；tee 非 nil 时 Run 的错误出口同时写 tee（A12 用它看「正在检查」何时写出）。
func obxRunTee(t *testing.T, ctx context.Context, opts Options, tee io.Writer) (string, string, error) {
	t.Helper()
	var out, errOut strings.Builder
	opts.Out = &out
	opts.Err = &errOut
	if tee != nil {
		opts.Err = io.MultiWriter(&errOut, tee)
	}
	rctx, cancel := context.WithCancel(ctx)
	finished := make(chan struct{})
	var runErr error
	go func() {
		defer close(finished)
		runErr = Run(rctx, opts)
	}()
	t.Cleanup(func() { obxDrain(cancel, finished, t.Name()) })
	select {
	case <-finished:
		return out.String(), errOut.String(), runErr
	case <-time.After(obxRunTimeout):
		t.Fatalf("Run %v 没有返回", obxRunTimeout)
		return "", "", nil
	}
}
```

`obxRun` 在 `obxIsolate` 之后调用，所以它注册的 cleanup 先于环境变量还原与删临时目录执行。`os.Exit` 那一支只在 `cancel()` 之后 10 s `Run` 还不返回时走到；原型上从没走到过，它的行为由 §0 第 16 条的探针担保。

**canary 的正对照（H0 / H1，原型实测，§0 第 18 条）**：泄漏核对（§4 表头）里的 canary 是缺席断言。它的正对照是同一装置、只差 `obxDrain` 开没开的一对：M33 + `obxRunTimeout` 改成 2 s + 一个睡 12 s 的后续用例 `TestObxZZSleep`，照 §4 表头的运行纪律跑（W1b 修订：四个 profile 变量指到 canary home，`TMP`、`TEMP` 指到隔离目录，go 的进程环境里不设 `GOTMPDIR`），只跑 `^(TestObxFirstRunFailures|TestObxZZSleep)$`。H0 在 `obxDrain` 开头加 `if who != "" { return }`：canary 里出现 `council.db`。H1 不加：canary 空。这一对只在集成员核 F0 时跑，不进 §4 表；跑 H0 之前 canary 必须在隔离目录里，跑完删掉 H0 留下的东西。
- 上面 H0 的结论是 v1.1 代码上的实测。v1.2 生成的配置写了 `data_dir`，它优先于 `COUNCIL_DATA_DIR`；按代码推，H0 里泄漏的 `Run` 会把库建在重建的临时 home 下（隔离 TMP 里），不再落进 canary，第 2 项核对照样看得见。修订时没有重跑，见 §7 (u)。

**等待循环（v1.1；W1b 修订后 A9 也用它；逐字）**：

```go
// obxLoopLimit 是 A8、A9、A12 假 claude 等待循环的计数上限：本机单独实测 cmd 内建的这种循环
// 每秒约 3400 次（60000 次约 18 s），go test 里 M33 下 A8 实测 21.25 s 才数到上限；给 1 s、3 s 的超时留足余量，
// 又保证循环一定会停。
const obxLoopLimit = "60000"

// obxLoopLines 是一个会自己停的等待循环：cond 为真时继续等，计数到 obxLoopLimit 就以 0 退出。
func obxLoopLines(cond string, then ...string) []string {
	lines := []string{"@echo off", "set n=0", ":obxloop", "set /a n+=1", "if %n% geq " + obxLoopLimit + " exit /b 0", "if " + cond + " goto obxloop"}
	return append(lines, then...)
}
```

**假 codex（v1.1；逐字）**：第一个参数是 `login` 时走 status 那几行，否则走 models 那几行。

```go
// obxCodexFake 写假 codex：第一个参数是 login 时（codex login status）跑 status 那几行，
// 否则（codex debug models）跑 models 那几行、以退出码 0 结束。
func obxCodexFake(t *testing.T, e *obxEnv, models, status []string) string {
	t.Helper()
	lines := []string{`@if "%1"=="login" goto obxlogin`}
	lines = append(lines, models...)
	lines = append(lines, "@exit /b 0", ":obxlogin")
	lines = append(lines, status...)
	return obxFake(t, e.bin, "codex", lines...)
}
```

夹具（逐字）：

```go
const (
	obxClaudeOut        = `{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[{"value":"default","resolvedModel":"claude-obx-1"},{"value":"obx-a","resolvedModel":"claude-obx-a","supportedEffortLevels":["low","high"]},{"value":"obx-b","resolvedModel":"claude-obx-b"}],"account":{"apiProvider":"firstParty","subscriptionType":"Claude Obx"}}}}` + "\n"
	obxClaudeNoLoginOut = `{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[{"value":"default","resolvedModel":"claude-obx-1"},{"value":"obx-a","resolvedModel":"claude-obx-a","supportedEffortLevels":["low","high"]},{"value":"obx-b","resolvedModel":"claude-obx-b"}],"account":{"tokenSource":"none","apiProvider":"firstParty"}}}}` + "\n"
	obxGrokOut          = "You are not authenticated.\n\nDefault model: grok-obx-1\n\nAvailable models:\n  * grok-obx-1 (default)\n  - grok-obx-2\n  - grok-obx-3\n"
	obxGrokKeyOut       = "Model 'grok-obx-1' is using its own API key.\n\nDefault model: grok-obx-1\n\nAvailable models:\n  * grok-obx-1 (default)\n"
	obxCodexOut         = `{"models":[{"slug":"gpt-obx","visibility":"list","supported_reasoning_levels":[{"effort":"high"}]},{"slug":"custom/obx","visibility":"list"},{"slug":"obx-hidden","visibility":"hide"}]}` + "\n"
	obxAgyOut           = `{"status":"SUCCESS","command":{"name":"models","data":{"models":[{"id":"a","label":"Gemini Obx"}]}}}` + "\n"
)

var (
	obxCodexLoggedIn = []string{"@echo Logged in using ChatGPT 1>&2", "@exit /b 0"}
	obxCodexNotIn    = []string{"@echo Not logged in 1>&2", "@exit /b 1"}
)

// obxThreeFakes 装 claude（已登录，订阅 Claude Obx）、grok（没登录）、codex（已登录），agy 不装。
func obxThreeFakes(t *testing.T, e *obxEnv) {
	t.Helper()
	obxTypeFake(t, e, "claude", obxClaudeOut)
	obxTypeFake(t, e, "grok", obxGrokOut)
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, obxCodexLoggedIn)
}
```

claude 2 个（`default` 行跳过）、grok 3 个、codex 1 个（`custom/obx` 不数，`obx-hidden` 被 visibility 滤掉；「不数 custom」的杀手就是它）。`obxClaudeOut` 的 `account` 是已登录（订阅 `Claude Obx`），`obxClaudeNoLoginOut` 是没登录的形状（§0 第 14 条）；`obxGrokOut` 首行是没登录的标记，`obxGrokKeyOut` 首行是登录态下那一行（查不出）；`obxCodexLoggedIn` / `obxCodexNotIn` 是 `codex login status` 的两种实测输出，打在 stderr。`obxThreeFakes` 装 claude（已登录）、grok（没登录）、codex（已登录），agy 不装。

期望块（`<home>`、`<bin>` 由 `obxSub` 换成真路径；逐字）：

```go
const obxHead = `第一次运行：还没有配置文件 <home>\.council\config.toml，council 来生成一份。
`

const obxChecking = "正在检查 claude、grok、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"

const obxRowsOK = `  claude  <bin>\claude.cmd  识别到 2 个模型；已登录（订阅：Claude Obx）
  grok    <bin>\grok.cmd  识别到 3 个模型；没登录
  agy     没找到
  codex   <bin>\codex.cmd  识别到 1 个模型；已登录
能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：
  grok    grok login
`

const obxDone = `已生成配置文件: <home>\.council\config.toml（只写了找到的工具，还没有成员）
上手三步：
  1. /new <标题>  开一个会话
  2. /add  看识别出的模型，再用 /add +编号 挑成员
  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个

`

const obxWantNone = `第一次运行：还没有配置文件 <home>\.council\config.toml，council 来生成一份。
  claude  没找到
  grok    没找到
  agy     没找到
  codex   没找到
一个都没找到，没有生成配置文件。至少装一个、在终端里登录好，再运行 council：
  claude  npm install -g @anthropic-ai/claude-code，装好后运行 claude auth login
  grok    照 grok 官方的说明安装，装好后运行 grok login
  agy     照 agy 官方的说明安装，装好后运行 agy，按提示登录
  codex   npm install -g @openai/codex，装好后运行 codex login
council 找的是 PATH，以及这几处默认安装位置：%APPDATA%\npm、%USERPROFILE%\.grok\bin、%LOCALAPPDATA%\agy\bin
`

const obxLoginHead = "能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：\n"
```

`obxWantFile(e, ids...)`：对 claude、grok、agy、codex 四个 id 依次造 `config.DetectedProvider`，`ids` 里有的 Path 为 `<bin>\<id>.cmd`，其余为空，返回 `config.FirstRunTOML(obxGenData(e), 那四项)`；`obxGenData(e)` 是 `<home>\.council\data`（v1.2）。**注意它是自引用的**（期望用被测函数算），只用来钉「写进文件的正是 FirstRunTOML(检测结果)」；`FirstRunTOML` 自己的字节由 C3 用字面量钉，`data_dir` 那一行的有无与值另由 A1 的 `LoadAndValidate` 断言钉（v1.2）。

`obxBanner(e)`：`"配置文件: " + cfgPath + "\n数据目录: " + obxGenData(e) + "\n库文件: " + obxGenData(e)\council.db + " (已有 0 个会话)\n已加载 0 个 agent，0 个 team\n输入 /help 查看可用命令，/quit 退出。\n\ncouncil> 再见\n"`。v1.2：生成的配置写了 `data_dir`，它优先于 `COUNCIL_DATA_DIR`，所以数据目录是 `<home>\.council\data`，不再是 v1.1 的 `<root>\data`。

### 2.1 `internal/config/obx_firstrun_test.go`

装置 `obxDetectEnv(t)`：一个 `t.TempDir()` 下建 `p1`、`p2`、`home`、`appdata`、`local`；`PATH = p1 + os.PathListSeparator + p2`，`USERPROFILE`、`HOME`、`APPDATA`、`LOCALAPPDATA` 指到对应目录。`obxTouch(t, parts...)`：建父目录、写一个空文件、返回路径（空文件就够，检测不执行它）。`obxOnlyEntries(t, dir, names...)`：`os.ReadDir(dir)` 的名字列表 `reflect.DeepEqual` names（临时文件都删掉了）。

- **C1 `TestObxDetectProviders`**，六个子测试，都整份 `reflect.DeepEqual` 或逐项 `==`：
  - `path_and_known`：`p2\claude.cmd`、`p1\codex.cmd`、`home\.grok\bin\grok.exe`；期望 `[{claude p2\claude.cmd} {grok home\.grok\bin\grok.exe} {agy ""} {codex p1\codex.cmd}]`。
  - `npm_and_agy_known`：`appdata\npm\claude.cmd`、`appdata\npm\codex.cmd`、`local\agy\bin\agy.exe`；期望 `[{claude appdata\npm\claude.cmd} {grok ""} {agy local\agy\bin\agy.exe} {codex appdata\npm\codex.cmd}]`。
  - `path_wins`：`p1\claude.cmd` 与 `appdata\npm\claude.cmd` 都有；期望第 0 项的 Path 是 `p1\claude.cmd`。
  - `known_is_dir`：`home\.grok\bin\grok.exe` 建成**目录**；期望第 1 项 Path 为空。
  - `empty_env_no_relative`：在 `<root>\cwd\npm\claude.cmd` 建文件，`t.Chdir(<root>\cwd)`，再 `t.Setenv("APPDATA", "")`；期望第 0 项 Path 为空（两道防线都拿掉时会返回相对路径 `npm\claude.cmd`）。
  - `cwd_planted`（v1.1，F1）：先 `os.LookupEnv("NoDefaultCurrentDirectoryInExePath")` 记下，`t.Cleanup` 里还原（原来没有就 `os.Unsetenv`），再 `os.Unsetenv`；然后 `obxDetectEnv`，在 `<root>\cwd\claude.cmd` 建文件，`t.Chdir(<root>\cwd)`；**前提**：`exec.LookPath("claude")` 的错误满足 `errors.Is(err, exec.ErrDot)`，不满足就 `t.Fatalf("前提不成立…")`；期望第 0 项 Path 为空。
  - 【判别力】前五个同 v1（§4 M1-M10）；`cwd_planted` 对 X1（把当前目录的相对结果转成绝对路径收下）红。前提断言是它的正对照：宿主注入的变量不拿掉时 `LookPath` 根本不看当前目录，这一条会成为空用例（§0 第 13 条）。
- **C2 `TestObxProviderBlockTOML`**：两行表，逐字：

  ```go
  {`C:\Users\a\AppData\Roaming\npm\claude.cmd`, "[providers.claude]\ncommand = 'C:\\Users\\a\\AppData\\Roaming\\npm\\claude.cmd'\n"},
  {`C:\Users\O'Brien\npm\claude.cmd`, "[providers.claude]\ncommand = \"C:\\\\Users\\\\O'Brien\\\\npm\\\\claude.cmd\"\n"},
  ```
  `ProviderBlockTOML("claude", in)` 必须等于第二列；再把结果写进临时文件 `LoadAndValidate`，`cfg.Providers["claude"].Command == in`（往返）。【判别力】M11、M12。
- **C3 `TestObxFirstRunTOML`**（v1.2 拆成两个子测试）：
  - `literal`：`FirstRunTOML(`C:\h\.council\data`, [{claude C:\x\claude.cmd} {grok ""} {agy C:\y\agy.exe} {codex ""}])` 必须等于
    `DefaultTOML() + "\n# 数据目录（本地库与成员的工作目录），council 第一次运行时定在本文件旁边。可以改成别的绝对路径，重启 council 生效（已有的库不会跟着搬）；写了这一行，环境变量 COUNCIL_DATA_DIR 就不再起作用。\ndata_dir = 'C:\\h\\.council\\data'\n\n# 下面是 council 第一次运行时在本机找到的工具。\n[providers.claude]\ncommand = 'C:\\x\\claude.cmd'\n\n[providers.agy]\ncommand = 'C:\\y\\agy.exe'\n"`（Go 字符串字面量）；写盘后 `LoadAndValidate` 成功，`len(Agents)==0`、`len(Teams)==0`、`Summary.Agent==""`、`len(Providers)==2`、`DataDir == C:\h\.council\data`。
  - `quote_data_dir`（v1.2）：`FirstRunTOML(`C:\Users\O'Brien\.council\data`, nil)` 含 `"\ndata_dir = \"C:\\\\Users\\\\O'Brien\\\\.council\\\\data\"\n"`（Go 字符串字面量，基本字符串）；写盘后 `LoadAndValidate` 的 `DataDir` 往返等于输入。
  - 【判别力】`literal` 对 M12、M14-M17、M21、M101、M106 红；`quote_data_dir` 对 M21、M101、M102 红。`data_dir` 含控制字符的那一支没有用例，理由同 §4 的等价变异 M13：值来自 `filepath.Join(<home>…)`，Windows 路径里造不出控制字符。
- **C4 `TestObxDefaultTOMLNeutral`**：`DefaultTOML()` 去掉末尾换行后按 `\n` 切，每一行都以 `#` 开头；不含 `25142`、`theory`、`critic`、`fsdt`、`FSDT`、`剪切`；含 `DefaultDataDir` 的值、`rules_file`、`full 档能写进配置并通过校验，不等于编排器可以自动使用它`、`[summary]`、`/add +编号`；单独写盘 `LoadAndValidate` 成功且 providers、agents、teams 都是零个。【判别力】M21、M22。
- **C5 `TestObxCreateConfigFile`**：`<root>\a\b\config.toml`（两层目录都不存在）写 `"x = 1\n"` 成功、内容逐字；再写一次返回的错误满足 `errors.Is(err, fs.ErrExist)`，内容仍是 `"x = 1\n"`；`<root>\blk` 建成文件，先断言前提 `os.Stat(<root>\blk\config.toml)` 满足 `ErrNotExist`，再 `CreateConfigFile(<root>\blk\config.toml, …)` 返回非 nil、且不满足 `ErrExist`；最后 `obxOnlyEntries(<root>\a\b, "config.toml")`（v1.1）。【判别力】M19、M24（不删临时文件）。
- **C6 `TestObxCreateConfigFileConcurrent`**（v1.1，F3）：20 轮，每轮在一个新子目录里，16 个 goroutine 等同一个 `start` 通道一起 `CreateConfigFile(<轮目录>\config.toml, payload(i))`，`payload(i)` 是 256 KB 的 `'a'+i`。每轮：恰好一个返回 nil（两个 nil 就 `t.Fatalf("…覆盖了别人写的文件")`），其余都满足 `ErrExist`，文件内容 `bytes.Equal` 赢家那一份，`obxOnlyEntries(轮目录, "config.toml")`。【判别力】X4（先 Stat 再写）：原型上 X4 下 C6 一共跑了 6 次（1 次随全表、5 次只跑 C6），6 次都在第 0 轮就有两个 nil。它是概率杀，X4 的确定杀手是 C7 `via_link`。正确实现 `-count=3` 零偶发。
- **C7 `TestObxCreateConfigFileLink`**（v1.1，F3），三个子测试，各自换掉 `linkFile`、`t.Cleanup` 还原：
  - `via_link`：换成一个包装：计数；断言 `newname == path`、`filepath.Dir(oldname)` 是 `path` 所在目录、`oldname != path`；断言此刻 `os.Stat(path)` 满足 `ErrNotExist`（目标不是先建出来再写的）；断言 `os.ReadFile(oldname)` 已经是完整内容 `"x = 1\n"`；再调真的 `os.Link`。期望 `CreateConfigFile` 返回 nil、恰好调了 1 次、内容逐字、`obxOnlyEntries(dir, "config.toml")`。
  - `fallback`：换成总是返回 `obxErrNoLink`（`errors.New("obx: 这个卷不支持硬链接")`，不满足 `ErrExist`）；期望返回 nil、内容逐字、目录里只有 `config.toml`。
  - `fallback_exists`：目标先写成 `"old\n"`，同样换成 `obxErrNoLink`；期望返回的错误满足 `ErrExist`、内容仍是 `"old\n"`、目录里只有 `config.toml`。
  - 【判别力】`via_link` 对 X4、M23（回到 v1 的直接建）红；`fallback` 对 M25（去掉退路）红；`fallback_exists` 对 M18（退路里 `O_EXCL` 换 `O_TRUNC`）、M25 红；三个都对 M24 红。

### 2.2 `internal/repl/obx_cliadvice_test.go`

用 HEAD 的 `mdxNewRig(t, tmpl)` 装置（真 SQLite、真配置文件、假识别执行器；`os.Args[0]` 答 claude 夹具、`mdxGrokPath`（`mdx-grok.exe`）答 grok 夹具）。配置模板：

```go
const obxTemplate = "data_dir = '<dataDir>'\n\n[providers.claude]\ncommand = '<exe>'\n"
```

检测器桩 `obxDetector(list, &calls)`：每次调用 `calls++`，返回 list 的副本。桩的清单（逐字）：

```go
var obxList = []config.DetectedProvider{
	{ID: "claude", Path: `C:\obx\claude.cmd`},
	{ID: "grok", Path: mdxGrokPath},
	{ID: "agy"},
	{ID: "codex", Path: `C:\obx\O'Neil\codex.cmd`},
}
```

期望提示块（六行，`<cfg>` 是 `g.cfgPath`）：

```text
本机装了 grok（mdx-grok.exe），配置里还没有：要用它，把下面两行追加到 <cfg> 末尾，再敲 /config reload --apply 与 /add --refresh
[providers.grok]
command = 'mdx-grok.exe'
本机装了 codex（C:\obx\O'Neil\codex.cmd），配置里还没有：要用它，把下面两行追加到 <cfg> 末尾，再敲 /config reload --apply 与 /add --refresh
[providers.codex]
command = "C:\\obx\\O'Neil\\codex.cmd"
```

- **R1 `TestObxSetCLIDetectorZeroValue`**：`New(nil, nil, nil, strings.NewReader(""), nil, RulesContext{})` 的 `detectCLIs` 是 nil。【判别力】M64。
- **R2 `TestObxAddHint`**：两套 `mdxNewRig(t, obxTemplate)`：`g0` 不装检测器，`base := g0.run("/add")`（正对照：没有提示）；`g` 装 `obxDetector(obxList, &calls)`，`g.run("/add")` 必须**整段等于** `base` 后接上面六行（用 `mdxWantLines`）；`calls == 1`；再 `g.run("/add")`，最后六行仍是那六行，`calls == 2`（不缓存）。claude 配置里有、agy 没装，所以都不提示；codex 的路径含单引号，走基本字符串。【判别力】M11、M12、M60、M61、M63、M65。
- **R3 `TestObxAddHintFollow`**：装同一个桩，`/add` 之后把输出里 grok 那两行（倒数第 5、4 行）加换行追加到配置文件末尾，`/config reload --apply` 的最后一行是 `mdxAppliedLine`；`/add --refresh` 的输出里有 `正在识别模型: claude, grok` 这一行，没有以 `本机装了 grok` 开头的行，最后三行是 codex 那三行。【判别力】M11、M60、M61、M63。
- **R4 `TestObxAddHintCanceled`**：装同一个桩；`ctx, cancel := context.WithCancel(...)`，`g.exec.hook = func(int, provider.DiscoveryCommand) { cancel() }`；`g.runCtx(ctx, "/add")` 的最后一行是 `识别被取消；下次 /add 会重新识别`，`calls == 0`。【判别力】M62。
- **R5 `TestObxDiscoveryFailureTextExported`**：对 `fmt.Errorf("%w: x", provider.ErrDiscoveryTimeout)`、`fmt.Errorf("g: %w", provider.ErrDiscoveryEmpty)`、`fmt.Errorf("g: %w", provider.ErrDiscoveryUnparseable)`、`errors.New("obx 其它")` 四种，`DiscoveryFailureText(err, 7*time.Second) == discoveryFailureText(err, 7*time.Second)`；另外超时那一种整句等于 `超时（7s 内没有回来）`。【判别力】M66。
- **R6 `TestObxAddCandidatesWording`**（v1.1，裁决第 2 条），十二个子测试（`summary_left` 起三个是 W1b 修订加的，`summary_present` 是 v1.2 加的）。配置模板（逐字）：

```go
// obxSumTemplate 是 R6 的配置：两个成员，其中 sa-sum 是总结成员。
const obxSumTemplate = "data_dir = '<dataDir>'\n\n[providers.claude]\ncommand = '<exe>'\n\n[agents.sa-alpha]\nprovider = 'claude'\nmodel = 'claude-obx-a'\n\n[agents.sa-sum]\nprovider = 'claude'\nmodel = 'claude-obx-s'\n\n[summary]\nagent = 'sa-sum'\n"

// obxSumOnlyTemplate 是 R6 的配置：只有一个 agent，它就是总结成员。
const obxSumOnlyTemplate = "data_dir = '<dataDir>'\n\n[providers.claude]\ncommand = '<exe>'\n\n[agents.sa-sum]\nprovider = 'claude'\nmodel = 'claude-obx-s'\n\n[summary]\nagent = 'sa-sum'\n"
```

  `obxJoinDirect(t, g, id)` 绕过 `/add` 直接 `g.svc.Join` 一个 `conversation.Participant{ConvID: g.conv.ID, AgentID: id, DisplayName, Provider 抄配置, Active: true, JoinedAt: time.Now().UTC()}`（总结成员 `/add` 加不进去）。`obxLeaveDirect(t, g, id)`（W1b 修订）直接 `g.svc.Leave(g.ctx, g.conv.ID, id, time.Now().UTC())`，造「加入过又退出」。「第一行」指 `g.run("/add")` 输出的第 0 行。
  - `no_members`：`obxTemplate`；第一行 `配置里还没有可以 /add 的成员`。
  - `summary_only_config`：`obxSumOnlyTemplate`（只有总结成员一个 agent）；第一行同上。
  - `summary_missing`：`obxSumTemplate`；`g.run("/add sa-alpha")` 整段等于 `["sa-alpha 已加入会话 " + g.conv.ID]`；第一行 `除总结成员 sa-sum 外，配置里的 agent 都已经在当前会话里了；总结成员不用 /add：多个成员回复之后它会自己加入做总结`。
  - `all_present`：`obxSumTemplate`；`obxJoinDirect` sa-alpha 与 sa-sum；第一行 `mdxAllPresent`。
  - `prefix_summary`：`obxSumTemplate`；`obxJoinDirect` sa-alpha；`g.run("/add sa")` 整段等于 `["sa 开头的除总结成员 sa-sum 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结"]`（HEAD 上是「sa 开头的都已经在当前会话里了」，假话）。
  - `prefix_summary_only`：同上装置；`g.run("/add sa-s")` 整段等于 `["sa-s 开头的除总结成员 sa-sum 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结"]`（在场的没有以 sa-s 开头的）。
  - `prefix_all_present`：`obxJoinDirect` sa-alpha 与 sa-sum；`g.run("/add sa")` 整段等于 `["sa 开头的都已经在当前会话里了"]`。
  - `prefix_none`：`obxJoinDirect` sa-alpha；`g.run("/add zz")` 整段等于 `["配置里没有可加入的 zz，不带参数的 /add 能列出可加的"]`。
  - `summary_left`（W1b 修订）：`obxSumTemplate`；`obxJoinDirect` sa-alpha 与 sa-sum，再 `obxLeaveDirect` sa-sum；第一行 `总结成员 sa-sum 已退出当前会话，要恢复总结用 /join sa-sum；除它之外配置里没有可以 /add 的成员`。
  - `summary_only_left`（W1b 修订）：`obxSumOnlyTemplate`；`obxJoinDirect` sa-sum，再 `obxLeaveDirect` sa-sum；第一行同 `summary_left`。
  - `prefix_summary_left`（W1b 修订）：同 `summary_left` 的装置；`g.run("/add sa")` 整段等于 `["sa 开头的除总结成员 sa-sum 外没有可加入的；sa-sum 已退出当前会话，要恢复总结用 /join sa-sum"]`。
  - `summary_present`（v1.2）：`obxSumTemplate`；先 `g.run("/add sa-sum")` 整段等于 `["sa-sum 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结"]`（没加入过，HEAD 原句）；再 `obxJoinDirect` sa-sum，`g.run("/add sa-sum")` 整段等于 `["sa-sum 是总结成员（config.toml 的 [summary]），已经在当前会话里了"]`。
  - 【判别力】
    - `summary_only_config` 对 M67、M68 红（只数 `len(Agents)` 时走成「除总结成员外」），也对 M96 红。
    - `summary_missing` 对 M69 红，也对 M96 红：不看 `member` 时，没加入过也被说成已退出。
    - `all_present` 对 M70 红，并且是 `summary_missing` 的正对照（只差总结成员在不在场）。
    - `prefix_summary`、`prefix_summary_only` 对 M71、M99 红。
    - `prefix_all_present` 对 M72 红，是 `prefix_summary` 的正对照。
    - `prefix_none` 钉住原句不变。
    - `summary_left` 对 M95、M100 红，`all_present` 是它的正对照（只差 sa-sum 退没退出）。
    - `summary_only_left` 对 M95、M97、M100 红，`summary_only_config` 是它的正对照（只差 sa-sum 加入又退出过没有）。它钉「加入过又退出」排在「一个成员都没有」之前。
    - `prefix_summary_left` 对 M71、M98 红，`prefix_all_present` 是它的正对照。
    - `summary_present` 对 M104、M105 红：前一半（没加入过）是后一半（在场）的正对照，只差 sa-sum 在不在场。CH1 的 C10 `active`、`explicit` 也分别对 M104、M105 红。
    - v1.1 的代码下（没有 `member` 那一支），三条新用例拿到的是什么（W1b 实测了前两条；修订人在 M95、M98 下的原型上实跑，与之一致）：
      - `summary_left` 的第一行是「除总结成员 sa-sum 外，……总结成员不用 /add：多个成员回复之后它会自己加入做总结」；
      - `prefix_summary_left` 是「sa 开头的除总结成员 sa-sum 外没有可加入的；总结成员不用 /add：……」；
      - `summary_only_left` 的第一行是「配置里还没有可以 /add 的成员」，可 `/add sa-sum` 其实能加。

### 2.3 `internal/app/obx_firstrun_app_test.go`

- **A0 `TestObxGuardSeesCreation`**（守卫的正对照）：对一个临时 home，建 `.council\config.toml` 前后两次 `obxStatHome` 必须不等，且之后 `dirExists`、`fileExists` 都为真。
- **A1 `TestObxFirstRunEndToEnd`**（真 `app.Run`、行式、端到端；**E2 在同一个用例里**）：`obxIsolate`、`obxThreeFakes`；`obxRun(ctx, Options{In: "/quit\n"})`：
  - err 为 nil；`errOut == obxSub(obxHead + obxChecking + obxRowsOK + obxDone)`；
  - 配置文件字节 `== obxWantFile(e, "claude", "grok", "codex")`；
  - v1.2：`config.LoadAndValidate(cfgPath)` 成功，`DataDir == obxGenData(e)`（即 `<home>\.council\data`；主代理裁决第 1 条要的断言）；
  - `out == obxBanner(e)`（这一行同时证明数据目录是生成的配置里写的那一个，在临时 home 里）；
  - **E2**：同一环境再跑一次同样的 `obxRun`：err 为 nil、`errOut == ""`（不再触发首次运行）、`out == obxBanner(e)`、配置文件字节不变。第一次的非空 errOut 就是 E2「空」的正对照。
  - 【判别力】三档里的「已登录（带订阅）」「没登录」「已登录（不带订阅）」都在这一条里：M56、M57、M58、M89、M92 都让它红。M101、M103 也让它红（`DataDir` 断言与横幅）。
- **A2 `TestObxFirstRunNoAuto`**：同 A1 但 `NoAuto: true`；errOut 等于 A1 的期望把 `obxDone` 里第 3 步那一行换成 `  3. 直接打字只记下（你用了 -no-auto）；/auto on 之后，直接打字会让会话里的成员回复`；配置文件字节与 out 的断言同 A1（v1.2 原型照这样写；不跑 E2，不加 `DataDir` 断言）。与 A1 只差一个字段，互为对照。
- **A3 `TestObxFirstRunNoneFound`**：`obxIsolate`，一个假 CLI 都不装；err 非 nil；`errOut == obxSub(obxWantNone)`；`out == ""`；`<home>\.council` 不存在。
- **A4 `TestObxFirstRunExplicitMissing`**，子测试 `flag`、`env`：`obxIsolate`、`obxThreeFakes`（本机「装了」三家，默认位置也没有文件——只要走错了分支就会进首次运行）；`missing := <root>\typo.toml`；`flag` 设 `Options.ConfigPath = missing`，`env` 设 `t.Setenv(config.EnvConfigPath, missing)`；err 非 nil；`errOut == "找不到配置文件: " + missing + "\n这个路径是你用 --config 或环境变量 COUNCIL_CONFIG 指定的，请确认它没写错。\n"`（与 HEAD 逐字节相同）；`missing` 不存在；`<home>\.council` 不存在。A1 是它的正对照（同样三家、同样缺文件，只差显式指定）。
- **A5 `TestObxFirstRunWriteFails`**：`obxIsolate`、`obxThreeFakes`，把 `<home>\.council` 建成内容为 `x` 的普通文件；err 非 nil；`errOut` 以 `obxSub(obxHead + obxChecking + obxRowsOK) + "写不进配置文件 " + cfgPath + ": "` 开头；这之后到第一个 `\n` 之间是操作系统的错误文本（非空，不比内容）；`\n` 之后**整段等于** `"下面是本来要写的内容，请自己存成 " + cfgPath + "（UTF-8），再运行 council：\n\n" + strings.TrimRight(obxWantFile(e, "claude", "grok", "codex"), "\n") + "\n"`；`out == ""`；挡路的文件内容仍是 `x`。
- **A6 `TestObxFirstRunRace`**（撞车）：准备一份文件 `body = "[providers.claude]\ncommand = 'obx-prepared'\n\n[agents.p1]\nprovider = 'claude'\nmodel = 'm'\n"`（`obxFixture` 写成 `prepared.toml`）与 claude 夹具 `obxClaudeOut`；假 claude 三行：`@if not exist "<home>\.council" mkdir "<home>\.council"`、`@copy /y "<prepared>" "<cfgPath>" >nul`、`@type "<claude 夹具>"`（识别期间把文件建好）；只装 claude。err 为 nil；errOut 整段等于
  `obxSub(obxHead + "正在检查 claude 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n" + "  claude  <bin>\\claude.cmd  识别到 2 个模型；已登录（订阅：Claude Obx）\n" + "  grok    没找到\n  agy     没找到\n  codex   没找到\n" + "配置文件在检查期间已经被建好了，改用它: <home>\\.council\\config.toml\n\n")`；配置文件字节仍是 `body`；`out` 含 `已加载 1 个 agent，0 个 team\n`（加载的是那份文件）；v1.1：`obxEntries(<home>\.council)` 等于 `["config.toml"]`（撞车那一支也删掉了临时文件）。【判别力】M41、M42、M24；找到的全都已登录，登录说明与登录办法都不打，是 M74（总打登录说明）的杀手，也是 A1 里那两段的缺席对照。
- **A7 `TestObxFirstRunCanceled`**：`obxIsolate`、`obxThreeFakes`；传一个**已经取消**的 ctx；err 为 nil；`errOut == obxSub(obxHead + obxChecking + "识别被取消，没有生成配置文件；下次运行 council 会重新检查\n")`；`out == ""`；`<home>\.council` 目录与文件都不存在。
- **A8 `TestObxFirstRunFailures`**（失败分类、查不出、失败的也写进配置）：把 `firstRunTimeout` 改成 `1 * time.Second`（`t.Cleanup` 还原）；`loop.on` 由 `obxFixture` 预先写好；假 claude 是 `obxLoopLines(`exist "<fx>\loop.on"`)`（v1.1：条件与计数上限并用，§0 第 15 条）；假 grok 只 `type` 一行 `Available models:\n`；假 codex 两行 `@echo obx-boom 1>&2`、`@exit /b 3`（`debug models` 与 `login status` 都走这两行）。err 为 nil；errOut 整段等于
  `obxSub(obxHead + "正在检查 claude、grok、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 1s）……\n" + "  claude  <bin>\\claude.cmd  识别失败：超时（1s 内没有回来）；登录状态查不出\n" + "  grok    <bin>\\grok.cmd  识别失败：一个模型都没列出来（没登录？）；登录状态查不出\n" + "  agy     没找到\n" + "  codex   <bin>\\codex.cmd  识别失败：model discovery command failed: exit code 3: obx-boom；登录状态查不出\n" + obxLoginHead + "  claude  claude auth login\n  grok    grok login\n  codex   codex login\n" + obxDone)`；配置文件字节 `== obxWantFile(e, "claude", "grok", "codex")`。原型实测约 1.04 s。【判别力】M33、M37、M39、M40、M66、X2（claude 最后超时，一定最后完成）。
- **A9 `TestObxFirstRunParallel`**（并行）：`firstRunTimeout = 5 * time.Second`（还原）；标记文件 `<fx>\codex-started`（不预建）；claude 夹具由 `obxFixture` 写成 `<fx>\claude.out`（内容 `obxClaudeOut`）；假 claude 是 `obxLoopLines(`not exist "<标记>"`, `type "<claude 夹具>"`)`（W1b 修订 F-E：v1.1 是没有计数上限的四行循环，测试进程在 codex 写标记之前死掉时会一直占满 CPU；现在与 A12 同一种写法，数到 `obxLoopLimit` 自己停）；假 codex 是 `obxCodexFake(t, e, [@echo x> "<标记>", @type "<codex 夹具>"], obxCodexNotIn)`。claude 要等 codex 起来才回答：串行（claude 在前）时 claude 5 s 超时。err 为 nil；errOut 整段等于
  `obxSub(obxHead + "正在检查 claude、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 5s）……\n" + "  claude  <bin>\\claude.cmd  识别到 2 个模型；已登录（订阅：Claude Obx）\n" + "  grok    没找到\n  agy     没找到\n" + "  codex   <bin>\\codex.cmd  识别到 1 个模型；没登录\n" + obxLoginHead + "  codex   codex login\n" + obxDone)`。W1b 修订的原型上约 0.07 s。【判别力】M34（W1b 修订的原型上实测：A9 在 5.07 s 红，claude 那一行是「识别失败：超时（5s 内没有回来）」）；codex 没登录这一档（M73、M88、M91 都让它红）。
- **A10 `TestObxAddHintLineMode`**，子测试 `hint`、`configured`（行式分支装了检测器）：`obxIsolate`、`obxThreeFakes`；显式配置 `<root>\explicit.toml` = `"data_dir = '" + <root>\data + "'\n\n" + config.ProviderBlockTOML("claude", <bin>\claude.cmd)`，`configured` 再加 `"\n" + config.ProviderBlockTOML("grok", <bin>\grok.cmd)`；`Options{ConfigPath: 那个文件, In: "/new t\n/add\n/quit\n"}`；err 为 nil、`errOut == ""`（显式路径不走首次运行）；定义 `hint(id) = "本机装了 " + id + "（" + <bin>\<id>.cmd + "），配置里还没有：要用它，把下面两行追加到 " + <explicit.toml> + " 末尾，再敲 /config reload --apply 与 /add --refresh\n[providers." + id + "]\ncommand = '" + <bin>\<id>.cmd + "'\n"`；`out` 必须以 `"EFFORTS 列是它支持的档\n" + [hint("grok")（只在 hint 子测试）] + hint("codex") + "council> 再见\n"` 结尾。两个子测试只差 grok 在不在配置里，互为「grok 提示出现 / 不出现」的对照；claude 在配置里、agy 没装，都不提示。
- **A11 `TestObxFirstRunTUI`**（TUI 顺序与 TUI 分支装了检测器）：`obxIsolate`、`obxThreeFakes`；两根 `os.Pipe`，读 goroutine 把 stdout 那根 `io.Copy` 进 HEAD 的 `coxLockedBuf`；另起 goroutine 调 `Run(ctx, Options{TUI: true, InFile: inR, OutFile: outW, Err: outW})`（错误出口与 stdout 是**同一根管子**），返回时 `close(finished)`；`t.Cleanup`（v1.1）：`obxDrain(cancel, finished, t.Name())`，再关四个端、等读 goroutine 5 s（超时 `t.Errorf`）。步骤：等管道里出现 `coxESC+"[J"`（10 s）；写 `coxESC+"[8;24;100t"`；等 `coxFlat` 里出现 `council空闲`（5 s）；断言管道的原始字节**以** `obxSub(obxHead + obxChecking + obxRowsOK + obxDone) + "配置文件: " + cfgPath + "\n"` **开头**；然后 `obxTypeFake(t, e, "agy", obxAgyOut)`（首次运行之后才装上的 agy）；写 `/new t\r`，等 `coxFlat` 里出现 `已建会话`；写 `/add\r`，10 s 内 `coxFlat` 里要出现 `strings.ReplaceAll("本机装了 agy（" + <bin>\agy.cmd + "），配置里还没有", " ", "")`。
- **A12 `TestObxFirstRunCheckingFirst`**（v1.1，F2）：`firstRunTimeout = 3 * time.Second`（还原）；只装 claude：`obxLoopLines(`not exist "<fx>\checking-seen"`, `type "<claude 夹具>"`)`；`obxRunTee` 的 tee 是 `&obxMarkWriter{path: <fx>\checking-seen}`（逐字见下）。err 为 nil；errOut 整段等于 `obxSub(obxHead + "正在检查 claude 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 3s）……\n" + "  claude  <bin>\\claude.cmd  识别到 2 个模型；已登录（订阅：Claude Obx）\n" + "  grok    没找到\n  agy     没找到\n  codex   没找到\n" + obxDone)`。原型约 0.06 s。【判别力】X3（「正在检查」挪到识别之后打）：claude 等不到标记，3 s 超时。

```go
// obxMarkWriter 一看到「正在检查」就建标记文件（A12）。
type obxMarkWriter struct {
	mu   sync.Mutex
	path string
	buf  strings.Builder
	done bool
}

func (w *obxMarkWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if !w.done && strings.Contains(w.buf.String(), "正在检查") {
		w.done = true
		if err := os.WriteFile(w.path, []byte("x"), 0o644); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
```

- **A13 `TestObxFirstRunBadHome`**（v1.1，F4）：`obxIsolate`、`obxThreeFakes`；`bad := <root>\a<b`，`t.Setenv` 设 `USERPROFILE`、`HOME` 为 bad；先记 `before := obxEntries(<root>)`；`obxRun`。err 非 nil；errOut 以 `"配置文件有问题: " + <bad>\.council\config.toml + "\n"` 开头、**不含** `第一次运行`（之后是操作系统的错误文本，不比）；`out == ""`；`obxEntries(<root>)` 与 before `reflect.DeepEqual`。A1 是「不含第一次运行」的正对照（同样三家、同样没有文件，只差 home 的名字合不合法）。【判别力】X5。
- **A14 `TestObxFirstRunRelativeHome`**（v1.1，F5）：`obxIsolate`、`obxThreeFakes`；建 `<root>\cwd`，`t.Chdir` 过去；`t.Setenv` 设 `USERPROFILE`、`HOME` 为 `relhome`。err 非 nil；`errOut == "找不到配置文件: relhome\\.council\\config.toml\n可以用 --config <path> 指定别处的配置文件，或设置环境变量 COUNCIL_CONFIG 指向它。\n"`（Go 字符串字面量）；`out == ""`；`<root>\cwd\relhome` 不存在。【判别力】M27（去掉 `IsAbs`：首次运行在启动目录下建出 `relhome\.council\config.toml`）；M55（这一支把模板加回来）。
- **A15 `TestObxFirstRunLoginStates`**（v1.1，裁决第 1 条）：`obxIsolate`；claude `obxTypeFake(obxClaudeNoLoginOut)`、grok `obxTypeFake(obxGrokKeyOut)`、agy `obxTypeFake(obxAgyOut)`、codex `obxCodexFake([@type "<codex 夹具>"], obxCodexLoggedIn)`。err 为 nil；errOut 整段等于
  `obxSub(obxHead + "正在检查 claude、grok、agy、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n" + "  claude  <bin>\\claude.cmd  识别到 2 个模型；没登录\n" + "  grok    <bin>\\grok.cmd  识别到 1 个模型；登录状态查不出\n" + "  agy     <bin>\\agy.cmd  识别到 1 个模型；登录状态查不出\n" + "  codex   <bin>\\codex.cmd  识别到 1 个模型；已登录\n" + obxLoginHead + "  claude  claude auth login\n  grok    grok login\n  agy     agy（按提示登录）\n" + obxDone)`；配置文件字节 `== obxWantFile(e, "claude", "grok", "agy", "codex")`。【判别力】claude 没登录、grok 登录态那一行算查不出（M85）、agy 找到时的查不出与兜底句、codex 已登录不列登录办法（M39）。

原型实测：§2 全部用例 `-count=3` 零偶发；A11 约 0.2 s；把宿主注入的 `NoDefaultCurrentDirectoryInExePath` 从父环境拿掉再跑一次，同样全绿。

### 2.4 `internal/provider/obx_login_test.go`（v1.1）

- **L1 `TestObxLoginFromDiscovery`**：表驱动，每行一个同名子测试，`LoginFromDiscovery(id, stdout) == want`（stdout 为空串时传 nil）。拼装函数与整张表逐字：

```go
// obxClaudeInit 拼一行 initialize 回应，account 那一段原样嵌进去。
func obxClaudeInit(requestID, account string) string {
	return `{"type":"control_response","response":{"subtype":"success","request_id":"` + requestID + `","response":{"models":[{"value":"opus","resolvedModel":"claude-obx"}],"account":` + account + `}}}` + "\n"
}
```

```go
	yes := func(sub string) LoginInfo { return LoginInfo{State: LoginYes, Subscription: sub} }
	no := LoginInfo{State: LoginNo}
	unk := LoginInfo{}
	for _, c := range []struct {
		name, id, stdout string
		want             LoginInfo
	}{
		{"claude_real_shape", "claude", `{"type":"system","subtype":"init"}` + "\n" + obxClaudeInit("council-models", `{"email":"obx@example.invalid","organization":"obx","subscriptionType":"Claude Max","apiProvider":"firstParty"}`), yes("Claude Max")},
		{"claude_isolated_shape", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiProvider":"firstParty"}`), no},
		{"claude_no_account", "claude", `{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[]}}}` + "\n", unk},
		{"claude_account_empty", "claude", obxClaudeInit("council-models", `{"apiProvider":"firstParty"}`), unk},
		{"claude_none_and_sub", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","subscriptionType":"max"}`), unk},
		{"claude_sub_control_char", "claude", obxClaudeInit("council-models", `{"subscriptionType":"max\u0007"}`), unk},
		{"claude_sub_not_string", "claude", obxClaudeInit("council-models", `{"subscriptionType":7}`), unk},
		{"claude_other_request_only", "claude", obxClaudeInit("obx-other", `{"subscriptionType":"max"}`), unk},
		{"claude_other_then_ours", "claude", obxClaudeInit("obx-other", `{"subscriptionType":"max"}`) + obxClaudeInit("council-models", `{"tokenSource":"none"}`), no},
		{"claude_api_key_env", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`), unk},
		{"claude_console_login", "claude", obxClaudeInit("council-models", `{"tokenSource":"none","apiKeySource":"/login managed key","organization":"obx","email":"obx@example.invalid","apiProvider":"firstParty"}`), unk},
		{"claude_nil", "claude", "", unk},
		{"grok_not_authenticated", "grok", "You are not authenticated.\n\nDefault model: grok-4.6\n\nAvailable models:\n  * grok-4.6 (default)\n  - grok-4.5\n", no},
		{"grok_not_authenticated_crlf", "grok", "You are not authenticated.\r\nAvailable models:\r\n", no},
		{"grok_api_key_line", "grok", "Model 'grok-4.7' is using its own API key.\n\nAvailable models:\n  * grok-4.7 (default)\n", unk},
		{"grok_marker_not_first", "grok", "Available models:\nYou are not authenticated.\n", unk},
		{"grok_empty", "grok", "", unk},
		{"agy", "agy", `{"status":"SUCCESS"}`, unk},
		{"codex", "codex", `{"models":[]}`, unk},
	} {
```

  注意 `claude_sub_control_char` 那一行里的 `\u0007` 是 JSON 转义（六个 ASCII 字符，在 Go 原始字符串里），不是真的控制字符。【判别力】`claude_none_and_sub` 对 M80、M82 红；`claude_sub_control_char` 对 M81 红；`claude_real_shape`、`claude_other_request_only`、`claude_other_then_ours` 对 M83 红；`grok_marker_not_first` 对 M84 红；`grok_api_key_line` 对 M85 红。
  - W1b 修订加的两行 `claude_api_key_env`、`claude_console_login`，是用 API key、用 Console 账号登录的用户在 initialize 回应里的 `account` 形状，取自 §0 第 20 条的程序文本，未实跑。
    - 期望都是查不出；它们对 M94 红，而且原型上 M94 只让这两行红。
    - v1.1 的代码下这两行实跑都是没登录（State 2），这就是 F-A。
    - `claude_isolated_shape`（实测的负面形状）是它们的正对照：只差有没有 `apiKeySource`。
- **L2 `TestObxParseLoginStatus`**：表驱动，`ParseLoginStatus(id, code, []byte(stdout), []byte(stderr)) == want`。整张表逐字：

```go
	yes := LoginInfo{State: LoginYes}
	no := LoginInfo{State: LoginNo}
	unk := LoginInfo{}
	for _, c := range []struct {
		name, id       string
		code           int
		stdout, stderr string
		want           LoginInfo
	}{
		{"codex_real_logged_in", "codex", 0, "", "Logged in using ChatGPT\n", yes},
		{"codex_logged_in_stdout_crlf", "codex", 0, "Logged in using an API key - sk-obx***\r\n", "", yes},
		{"codex_real_not_logged_in", "codex", 1, "", "Not logged in\n", no},
		{"codex_not_logged_in_padded", "codex", 1, "", "  Not logged in  \r\n", no},
		{"codex_logged_in_but_exit1", "codex", 1, "", "Logged in using ChatGPT\n", unk},
		{"codex_not_logged_in_but_exit0", "codex", 0, "", "Not logged in\n", unk},
		{"codex_other_error", "codex", 3, "", "obx-boom\n", unk},
		{"codex_exit0_silent", "codex", 0, "", "", unk},
		{"claude", "claude", 0, "", "Logged in using ChatGPT\n", unk},
	} {
```

  【判别力】`codex_logged_in_but_exit1` 对 M86 红；`codex_not_logged_in_but_exit0` 对 M87 红；`codex_not_logged_in_padded` 对 M88 红；`claude` 对 M90 红。
- **L3 `TestObxLoginStatusCommandFor`**：`LoginStatusCommandFor("codex", `C:\x\codex.cmd`)` 是 `(DiscoveryCommand{Path: `C:\x\codex.cmd`, Args: []string{"login", "status"}}, true)`（`reflect.DeepEqual`）；claude、grok、agy、obx 四个都是零值与 false。【判别力】M89。

### 2.5 `internal/turn/obx_loginstatus_exec_test.go`（v1.1）

`obxStatusFake(t, lines...)`：在 `t.TempDir()` 里写 `obx-status.cmd`（`\r\n` 连接、末尾加 `\r\n`），返回路径。`PATH` 不改（这里要 System32 里的 taskkill）。

- **L4 `TestObxRunLoginStatus`**，五个子测试，`RunLoginStatus(ctx, provider.DiscoveryCommand{Path: <假 CLI>, Args: []string{"login", "status"}})`，ctx 带超时：
  - `exit0_stderr`：`@echo Logged in using ChatGPT 1>&2`、`@exit /b 0`，10 s；期望 code 0、stdout `""`、stderr `"Logged in using ChatGPT \r\n"`（cmd 的 `echo … 1>&2` 带着重定向前那个空格）、err nil。
  - `exit1_stderr`：`@echo Not logged in 1>&2`、`@exit /b 1`；期望 code 1、stdout `""`、stderr `"Not logged in \r\n"`、err nil。
  - `stdout_and_args`：`@echo %1 %2`、`@exit /b 7`；期望 code 7、stdout `"login status\r\n"`、stderr `""`、err nil。
  - `killed`：`obxLoopLines` 同样的循环（条件是临时目录里的 `loop.on` 还在，上限 60000），300 ms；期望 `errors.Is(err, context.DeadlineExceeded)`。
  - `empty_path`：`DiscoveryCommand{}`，期望 err 非 nil。
  - 【判别力】M91（非 0 退出当错误）红 `exit1_stderr`、`stdout_and_args`；M92（丢 stderr）红 `exit0_stderr`、`exit1_stderr`；M93（`%w` 换 `%v`）红 `killed`。`exit0_stderr` 是 `RunLoginStatus` 存在的理由：`RunDiscovery` 在退出码 0 时不交回 stderr。

---

## 3. 冻结的字面量（新增的每一句用户可见文本；`%s` / `%d` / `%v` / `%-6s` 按冻结代码填）

### 3.1 首次运行的进度与结果（写 errOut）

```text
第一次运行：还没有配置文件 %s，council 来生成一份。
正在检查 %s 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 %v）……
  %-6s  没找到
  %-6s  %s  识别失败：%s；%s
  %-6s  %s  识别到 %d 个模型；%s
能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：
  %-6s  %s
识别被取消，没有生成配置文件；下次运行 council 会重新检查
```

「正在检查」里的 `%s` 是找到的 id 按固定顺序用顿号 `、` 连接；`%v` 是 `firstRunTimeout`（生产 `20s`）。结果行末尾那个 `%s` 是登录状态（`firstRunLoginText`），四种之一：

```text
已登录（订阅：%s）
已登录
没登录
登录状态查不出
```

「已登录（订阅：%s）」的 `%s` 是 claude 的 `subscriptionType` 原值。「能列出模型不代表已经登录……」那一句与其后的登录办法，只在找到的里面有没登录或查不出的时候打，而且只列这几家。登录办法四条（`firstRunLogin`）：`claude auth login`、`grok login`、`agy（按提示登录）`、`codex login`。

### 3.2 首次运行的结局（写 errOut）

```text
配置文件在检查期间已经被建好了，改用它: %s
写不进配置文件 %s: %v
下面是本来要写的内容，请自己存成 %s（UTF-8），再运行 council：
已生成配置文件: %s（只写了找到的工具，还没有成员）
上手三步：
  1. /new <标题>  开一个会话
  2. /add  看识别出的模型，再用 /add +编号 挑成员
  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个
  3. 直接打字只记下（你用了 -no-auto）；/auto on 之后，直接打字会让会话里的成员回复
```

最后两行二选一（`NoAuto` 为真取后一句）。「改用它」与上手三步之后各跟一个空行。

### 3.3 一家都没找到（`msgFirstRunNoneFound`，用 `io.WriteString` 写 errOut）

```text
一个都没找到，没有生成配置文件。至少装一个、在终端里登录好，再运行 council：
  claude  npm install -g @anthropic-ai/claude-code，装好后运行 claude auth login
  grok    照 grok 官方的说明安装，装好后运行 grok login
  agy     照 agy 官方的说明安装，装好后运行 agy，按提示登录
  codex   npm install -g @openai/codex，装好后运行 codex login
council 找的是 PATH，以及这几处默认安装位置：%APPDATA%\npm、%USERPROFILE%\.grok\bin、%LOCALAPPDATA%\agy\bin
```

### 3.4 `/add` 之后的提示（写 REPL 的 out）

```text
本机装了 %s（%s），配置里还没有：要用它，把下面两行追加到 %s 末尾，再敲 /config reload --apply 与 /add --refresh
```

后接 `config.ProviderBlockTOML(id, path)` 的两行：`[providers.<id>]` 与 `command = '<路径>'`（或基本字符串）。

### 3.5 写进配置文件的文本

- `DefaultTOML()` 全文（F2，全注释）；
- `firstRunDataDirComment`（v1.2）：`# 数据目录（本地库与成员的工作目录），council 第一次运行时定在本文件旁边。可以改成别的绝对路径，重启 council 生效（已有的库不会跟着搬）；写了这一行，环境变量 COUNCIL_DATA_DIR 就不再起作用。`
- 一行 `data_dir = '<home>\.council\data'`（路径含单引号或控制字符时是基本字符串，D17）；
- `firstRunProvidersComment`：`# 下面是 council 第一次运行时在本机找到的工具。`
- 每家两行 `[providers.<id>]`、`command = …`。

### 3.6 `/add` 候选为空、前缀没命中（v1.1，写 REPL 的 out）

```text
配置里还没有可以 /add 的成员
除总结成员 %s 外，配置里的 agent 都已经在当前会话里了；总结成员不用 /add：多个成员回复之后它会自己加入做总结
%s 开头的除总结成员 %s 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结
总结成员 %s 已退出当前会话，要恢复总结用 /join %s；除它之外配置里没有可以 /add 的成员
%s 开头的除总结成员 %s 外没有可加入的；%s 已退出当前会话，要恢复总结用 /join %s
%s 是总结成员（config.toml 的 [summary]），已经在当前会话里了
```

第 4、5 行是 W1b 修订加的（总结成员加入过又退出，D16）。最后一行是 v1.2 加的（`resolveAddSegment` 第 0 步，总结成员正在场，P11）；第 0 步对没加入过的那一句「%s 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结」是 HEAD 原有的，不改。原有的「配置里的 agent 都已经在当前会话里了」「%s 开头的都已经在当前会话里了」「配置里没有可加入的 %s，不带参数的 /add 能列出可加的」不改。

### 3.7 删掉或改掉的字面量

- `reportConfigError` 里的「council 不会替你创建这个文件，下面这份模板可以直接抄成 %s:」与其后的整份模板（P4）。
- v1 的「正在检查 %s 能不能用（只让它们列出模型，不提问、不花钱；每家最多 %v）……」改成 §3.1 那一句（多了「、报登录状态」）。
- v1 的「识别到模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。要登录或换账号，在终端里运行：」改成 §3.1 的「能列出模型不代表已经登录：……」那一句；v1 无条件打、列出全部找到的，v1.1 只在有没登录或查不出的时候打、只列那几家。
- v1 的结果行「  %-6s  %s  识别失败：%s」「  %-6s  %s  识别到 %d 个模型」末尾都加了「；%s」。
- v1.2：`resolveAddSegment` 第 0 步在总结成员正在场时不再打 HEAD 那一句，改打 §3.6 最后一行；那一句本身仍用于没加入过的情形。

---

## 4. 集成员的变异清单

**运行纪律（v1.1，F0；W1b 修订补了 F-C、F-D；任何变异、任何全量运行之前都照做）**：
- 父环境：同一条命令里设，或者由驱动脚本传给 go 的子进程。
  - `COUNCIL_DATA_DIR` 设成隔离目录下的一个 canary 目录；`COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES` 不设。
  - `USERPROFILE`、`HOME` 指到隔离目录下的 canary home；`APPDATA`、`LOCALAPPDATA` 指到它下面的 `AppData\Roaming`、`AppData\Local`（W1b F-C）。
    - 理由：泄漏的 `Run` 在环境变量还原之后会走到 `LoadRules`，按那一刻的 `os.UserHomeDir()` 去读 `<home>\.council\COUNCIL.md`（`rules.go:40-48`）。父环境的 home 是真的，读的就是真用户目录。
    - 这是读、不落盘，下面哪一项核对都看不见，只能靠改环境挡住。
    - 这样跑时，`obxGuardRealHome` 守的是 canary home，不是真用户目录。
  - `TMP`、`TEMP` 指到隔离目录；go 的进程环境里不许有 `GOTMPDIR`（W1b F-D）。
    - 理由：测试进程的环境里有 `GOTMPDIR` 时，`t.TempDir()` 建在它下面，不看 `TMP`（§0 第 21 条）。那样第二项核对永远成立，H0 重建的临时 home 也会落在隔离目录之外。
  - 改了 `APPDATA`，go 就读不到 `%APPDATA%\go\env`，要显式给：
    - `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOTOOLCHAIN=local`；
    - `GOCACHE` 指到隔离目录下（修订人实测：冷缓存下 `go build ./...` 8.6 s）；
    - `GOFLAGS` 不设。
    - go 自己的 telemetry 计数会写进 canary home 的 `AppData\Roaming\go\telemetry`，这是 go 工具的正常行为，不算泄漏。
  - `PATH` 只留 `C:\Program Files\Go\bin` 与 `C:\Windows\System32`。去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`、`ANTHROPIC_*`。W1b 与修订人都这样跑过，全量九包全绿。
- 每跑完一条（全量也一样）核五件，任何一件不成立就停下，不再跑下一条，报告主代理：
  1. canary 下没有 `council.db`；
  2. 隔离的 `TMP`、真 `%TEMP%`、`E:\Go\tmp` 下都没有 `TestObx*`；
  3. `E:\council-data` 不存在；
  4. canary home 下没有 `.council`；
  5. 没有残留的 `cmd`、`claude`、`grok`、`codex`、`agy` 进程（按开跑时刻过滤 `StartTime`）。
- 各项核对的正对照：
  - canary（第 1 项）是缺席断言，它的正对照是 §2.0 的 H0 / H1 这一对：原型实测 H0 让 canary 出现 `council.db`，H1 没有。W1 的证据 (a) 是旁证。
    - 这是 v1.1 代码上的实测。v1.2 生成的配置写了 `data_dir`，按代码推，H0 那种泄漏的库落在隔离 TMP 里的临时 home 下，第 1 项看不见、第 2 项看得见；没有实跑，见 §7 (u)。
  - 第 4 项照实写：已知装置里造不出它为真的情形（泄漏的 `Run` 对 canary home 只读），所以没有正对照，是兜底。F-C 靠的是上面改环境，不是这一项。

每条在原型上单独施加、只跑 `go test -json -count=1 -run TestObx ./internal/config ./internal/repl ./internal/app ./internal/provider ./internal/turn`（M104、M105 的 `-run` 是 `TestObx|TestChxAddHidesSummary`，因为点名杀手里有 CH1 的 C10）、从字节副本还原（不用 `git checkout`），还原后逐个文件 SHA256 核对。锚点在文件里恰好出现一次。表格单元里的 `\|` 是 Markdown 的转义，实际文本是一个 `|`。「杀手」列是原型实测变红的用例（一次运行的实测集合，去掉括号里说明的偶发红）；点名的每个子测试都要单独确认变红。几条最初的写法编译不过（变量或 import 未用），表里已换成能编译的近似写法。v1 的 M39（登录办法列没找到的）随 D15 失去意义，编号换给新条目。v1.1 原型上全表 85 条都被点名杀手杀掉，每条之后三项泄漏核对都干净。

W1b 修订在 `ob1_fix` 原型上实跑了这些。照上面修订后的运行纪律跑；每条之后第 1-4 项文件核对都干净，还原后 SHA256 相同；第 5 项残留进程在整批跑完之后核，没有。
- 新加的 M94-M100，全部被点名杀手杀掉；
- 锚点行或上下文变了的 M67-M72、M82，以及同一段的 M80、M81、M83，全部照旧被杀；
- A9 换了假 claude 之后的 M34。

那份原型只有 L1-L3、R6、A1、A9、A15。所以这几行的「杀手」列里，凡是这些之外的用例，都是 v1.1 原型的实测，这次没有重跑。

v1.2 修订在 `ob1_v12` 原型上（§2 全部用例重写）实跑了这些。照上面的运行纪律跑；每条之后第 1-4 项文件核对都干净，还原后 SHA256 相同；第 5 项在整批跑完之后核，没有。
- 新加的 M101-M106，全部被点名杀手杀掉；
- 锚点或杀手受 v1.2 影响的 M12、M14-M17、M21、M46、M67-M72、M95-M100，全部照旧被杀，杀手列按这次实测写（C3 拆成了 `literal`、`quote_data_dir`）。

其余各行这次没有重跑，「杀手」列仍是 v1.1 与 W1b 的实测。

| # | 文件 | 替换前 → 替换后 | 杀手（原型实测） |
|---|---|---|---|
| M1 | config/firstrun.go | `var detectOrder = []string{"claude", "grok", "agy", "codex"}` → `{"grok", "claude", "agy", "codex"}` | C1 `npm_and_agy_known`、`path_and_known`、`path_wins`、A1、A2、A3、A5、A6、A7、A8、A9、A11、A12、A15 |
| M2 | config/firstrun.go | `if p, err := exec.LookPath(id); err == nil && filepath.IsAbs(p) {` → `if p, err := exec.LookPath(id); false && err == nil && filepath.IsAbs(p) {` | C1 `path_and_known`、`path_wins`、A1、A2、A5、A6、A7、A8、A9、A10 `configured`、`hint`、A11、A12、A15 |
| M3 | config/firstrun.go | `base, rel = os.Getenv("APPDATA"), []string{"npm", id + ".cmd"}` → `os.Getenv("LOCALAPPDATA")` | C1 `npm_and_agy_known` |
| M4 | config/firstrun.go | `base, rel = os.Getenv("USERPROFILE"), []string{".grok", "bin", "grok.exe"}` → `os.Getenv("APPDATA")` | C1 `path_and_known` |
| M5 | config/firstrun.go | `[]string{"agy", "bin", "agy.exe"}` → `[]string{"agy", "agy.exe"}` | C1 `npm_and_agy_known` |
| M6 | config/firstrun.go | `if err != nil \|\| !info.Mode().IsRegular() {` → `if err != nil \|\| info == nil {` | C1 `known_is_dir` |
| M9 | config/firstrun.go | M7 与 M8 同时施加 | C1 `empty_env_no_relative` |
| M10 | config/firstrun.go | `detectProvider` 开头先查已知位置、存在即返回（在 `func detectProvider(id string) string {` 之后插入 `if k := knownInstallPath(id); k != "" { if fi, e := os.Stat(k); e == nil && fi.Mode().IsRegular() { return k } }`） | C1 `path_wins` |
| M11 | config/firstrun.go | `if c == '\'' \|\| c < 0x20 \|\| c == 0x7f {` → `if c == 0 && false {`（永远字面量） | C2、R2、R3 |
| M12 | config/firstrun.go | `return "'" + s + "'"` → `return tomlBasicString(s)`（永远基本字符串） | C2、C3 `literal`、R2、A10 `configured`、`hint`（v1.2 重跑） |
| M14 | config/firstrun.go | `FirstRunTOML` 去掉 `if d.Path == "" { continue }` | C3 `literal`、A1、A2、A8、A9、A11、A12（v1.2 重跑） |
| M15 | config/firstrun.go | 去掉段间空行 `if n > 0 { b.WriteString("\n") }` | C3 `literal`（v1.2 重跑） |
| M16 | config/firstrun.go | `b.WriteString(firstRunProvidersComment)` → `_ = firstRunProvidersComment` | C3 `literal`（v1.2 重跑） |
| M17 | config/firstrun.go | 去掉 `b.WriteString(DefaultTOML())` 与其后的 `b.WriteString("\n")` | C3 `literal`（v1.2 重跑） |
| M18 | config/firstrun.go | `createExclusive` 里 `os.O_WRONLY\|os.O_CREATE\|os.O_EXCL` → `os.O_WRONLY\|os.O_CREATE\|os.O_TRUNC` | C7 `fallback_exists` |
| M19 | config/firstrun.go | 去掉 `if err := os.MkdirAll(dir, 0o755); err != nil { return err }` 三行 | C5、C6、C7 `fallback`、`via_link`、A1、A2、A8、A9、A11、A12、A15 |
| M23 | config/firstrun.go | 回到 v1 的直接建：`err = linkFile(tmpPath, path)` → `err = createExclusive(path, data)` | C7 `via_link` |
| M24 | config/firstrun.go | `defer os.Remove(tmpPath)` → `defer func() {}()`（不删临时文件） | C5、C6、C7 `fallback`、`fallback_exists`、`via_link`、A6 |
| M25 | config/firstrun.go | 去掉退路：`CreateConfigFile` 末尾的 `return createExclusive(path, data)` → `return err` | C7 `fallback`、`fallback_exists` |
| X1 | config/firstrun.go | 当前目录的相对结果也收下：`if p, err := exec.LookPath(id); err == nil && filepath.IsAbs(p) { return p }` → `if p, _ := exec.LookPath(id); p != "" { if a, e := filepath.Abs(p); e == nil { return a } }` | C1 `cwd_planted` |
| X4 | config/firstrun.go | 非原子：`err = linkFile(tmpPath, path)` 到 `return createExclusive(path, data)` 那五行 → 先 `os.Stat(path)`，存在就返回 `&fs.PathError{Op: "open", Path: path, Err: fs.ErrExist}`，不存在就 `os.WriteFile(path, data, 0o644)`，其余 `createExclusive` | C6、C7 `via_link`（C6 一共跑了 6 次（1 次随全表、5 次只跑 C6），6 次都在第 0 轮红；确定杀手是 C7 via_link） |
| M101 | config/firstrun.go | 去掉 `b.WriteString("data_dir = " + tomlCommandString(dataDir) + "\n")` 那一行（v1.2：生成的配置不写 `data_dir`） | C3 `literal`、`quote_data_dir`、A1（`DataDir` 断言、横幅、E2 的横幅）、A2（横幅） |
| M102 | config/firstrun.go | `"data_dir = " + tomlCommandString(dataDir)` → `"data_dir = '" + dataDir + "'"`（v1.2：永远字面量） | C3 `quote_data_dir` |
| M106 | config/firstrun.go | `b.WriteString(firstRunDataDirComment)` → `_ = firstRunDataDirComment`（v1.2） | C3 `literal` |
| M21 | config/defaults.go | 模板第一行之后插入 `[providers.claude]` 与 `command = "claude"` 两行 | C3 `literal`、`quote_data_dir`、C4、A1、A2、A8、A9、A11、A12、A15（v1.2 重跑） |
| M22 | config/defaults.go | 删掉「# 注意：full 档能写进配置并通过校验……」那一行 | C4 |
| M30 | app/firstrun.go | `if noAuto {` → `if false {` | A2 |
| M31 | app/firstrun.go | 一家都没找到时 `return errors.New("首次运行：本机没找到 …")` → `return nil` | A3 |
| M32 | app/firstrun.go | `strings.Join(ids, "、")` → `strings.Join(ids, ", ")` | A1、A2、A5、A7、A8、A9、A11、A15 |
| M33 | app/firstrun.go | `cctx, cancel := context.WithTimeout(ctx, firstRunTimeout)` → `cctx, cancel := context.WithCancel(ctx)` | A8（A8 实测 21.25 s，整条运行约 23 s；假 claude 数到 obxLoopLimit 自己以 0 退出，claude 那一行变成「输出认不出来」，A8 以 errOut 不等而红，不是 30 s 看门狗；A9-A11 的偶发红不算） |
| M34 | app/firstrun.go | `go func(i int, d config.DetectedProvider) {` → `func(i int, d config.DetectedProvider) {`（串行） | A9（W1b 修订换了 A9 的假 claude 之后重跑：A9 在 5.07 s 红，claude 那一行是超时） |
| M35 | app/firstrun.go | `if !m.Custom {` → `if !m.Custom \|\| true {`（custom 也数） | A1、A2、A5、A9、A11、A15 |
| M36 | app/firstrun.go | 取消检查 `if ctx.Err() != nil {`（「识别被取消」之前那一处）→ `if false {` | A7 |
| M37 | app/firstrun.go | `repl.DiscoveryFailureText(row.err, firstRunTimeout)` → `repl.DiscoveryFailureText(row.err, firstRunTimeout)[:0] + row.err.Error()` | A8 |
| M38 | app/firstrun.go | 结果表里 `fmt.Fprintf(w, "  %-6s  没找到\n", row.d.ID)` → `_ = 0`（不打） | A1、A2、A5、A6、A8、A9、A11、A12 |
| M39 | app/firstrun.go | `if row.login.State != provider.LoginYes {` → `if row.login.State == provider.LoginNo {`（查不出的不列登录办法） | A8、A15 |
| M74 | app/firstrun.go | `if len(needLogin) > 0 {` → `if true {`（全都已登录也打登录说明） | A6、A12 |
| M40 | app/firstrun.go | `"claude": "claude auth login",` → `"claude": "claude login",` | A8、A15 |
| M41 | app/firstrun.go | 撞车分支的 `return nil` → `return err` | A6 |
| M42 | app/firstrun.go | `if errors.Is(err, fs.ErrExist) {` → `if errors.Is(err, fs.ErrExist) && false {` | A6 |
| M43 | app/firstrun.go | `fmt.Fprintln(w, strings.TrimRight(text, "\n"))` → `_ = text` | A5 |
| M44 | app/firstrun.go | 写不进去时 `return fmt.Errorf("首次运行：写不进配置文件 …")` → `return nil` | A5 |
| M45 | app/firstrun.go | `fmt.Fprintf(w, "上手三步：\n")` → `_ = 0` | A1、A2、A8、A9、A11、A12、A15 |
| M46 | app/firstrun.go | `text := config.FirstRunTOML(filepath.Join(filepath.Dir(path), "data"), detected)` → 最后一个参数换成 `nil`（v1.2：锚点行随 F4 改写） | A1、A2、A5、A8、A15（v1.2 重跑；A2 是重写后的 A2 也比配置文件字节） |
| M56 | app/firstrun.go | 截 stdout 的闭包里 `stdout = out` → `_ = out`（claude、grok 的登录状态全成查不出） | A1、A2、A5、A6、A9、A11、A12、A15 |
| M57 | app/firstrun.go | `return "已登录（订阅：" + l.Subscription + "）"` → `return "已登录"` | A1、A2、A5、A6、A9、A11、A12 |
| M58 | app/firstrun.go | `if sc, ok := provider.LoginStatusCommandFor(d.ID, d.Path); ok && cctx.Err() == nil {` → `…; false && ok && cctx.Err() == nil {`（不跑 codex login status） | A1、A2、A5、A9、A11、A15 |
| M73 | app/firstrun.go | `provider.ParseLoginStatus(d.ID, code, so, se)` → `provider.ParseLoginStatus(d.ID, 0*code, so, se)`（丢掉退出码） | A9 |
| X3 | app/firstrun.go | 「正在检查」那一行挪到 `wg.Wait()` 之后打 | A12 |
| X2 | app/firstrun.go | 结果按完成顺序填回：`rows := make(…)` 之后加 `var finMu sync.Mutex` 与 `var fin []firstRunRow`；goroutine 里 `rows[i] = firstRunCheck(ctx, d)` → `r := firstRunCheck(ctx, d)`、加锁 `fin = append(fin, r)`、`_ = i`；`wg.Wait()` 之后 `for k, j := 0, 0; k < len(rows); k++ { if rows[k].d.Path != "" { rows[k] = fin[j]; j++ } }` | A8（原型那一次 A2 也红了，那是三家完成顺序随机碰上的，不算杀手） |
| M103 | app/firstrun.go | `filepath.Join(filepath.Dir(path), "data")` → `filepath.Join(filepath.Dir(filepath.Dir(path)), "data")`（v1.2：`data_dir` 写成 `<home>\data`；只落在测试的临时 home 里） | A1、A2、A5、A8、A15 |
| M47 | app/app.go | `firstRun(ctx, errOut, path, opts.NoAuto)` → `firstRun(ctx, out, path, opts.NoAuto)` | A1、A2、A3、A5、A6、A7、A8、A9、A12、A15 |
| M48 | app/app.go | `explicit := opts.ConfigPath != "" \|\| os.Getenv(config.EnvConfigPath) != ""` → `explicit := opts.ConfigPath != ""` | A4 `env` |
| M49 | app/app.go | 同一行 → `explicit := os.Getenv(config.EnvConfigPath) != ""` | A4 `flag` |
| M50 | app/app.go | `if !explicit && filepath.IsAbs(path) {` → `if filepath.IsAbs(path) {` | A4 `env`、`flag` |
| M51 | app/app.go | `errors.Is(statErr, fs.ErrNotExist)` → `statErr == nil` | A1、A2、A3、A5、A6、A7、A8、A9、A11、A12、A15 |
| M52 | app/app.go | `if errors.Is(frErr, errFirstRunCanceled) {` → `if false {` | A7 |
| M53 | app/app.go | 行式分支删掉 `r.SetCLIDetector(config.DetectProviders)` | A10 `configured`、`hint` |
| M54 | app/app.go | TUI 分支删掉 `r.SetCLIDetector(config.DetectProviders)` | A11 |
| M55 | app/app.go | `reportConfigError` 非 explicit 的 NotExist 分支在 `return` 之前加回 `fmt.Fprintln(w, strings.TrimRight(config.DefaultTOML(), "\n"))` | A14（v1 只能审查时看；A14 让这一支可达了） |
| X5 | app/app.go | `errors.Is(statErr, fs.ErrNotExist)` → `statErr != nil`（Stat 的任何错误都进首次运行） | A13 |
| M27 | app/app.go | `if !explicit && filepath.IsAbs(path) {` → `if !explicit {` | A14 |
| M60 | repl/cliadvice.go | 删掉 `if _, ok := r.cfg.Providers[d.ID]; ok { continue }` | R2、R3、A10 `configured`、`hint` |
| M61 | repl/cliadvice.go | 删掉 `if d.Path == "" { continue }` | R2、R3、A10 `configured`、`hint` |
| M62 | repl/commands.go | 删掉 `if ctx.Err() != nil { return nil }`（提示前的取消检查） | R4 |
| M63 | repl/commands.go | `return printUnconfiguredCLIs(r)` → `return nil` | R2、R3、A10 `configured`、`hint`、A11 |
| M64 | repl/repl.go | `New` 的字面量里 `discoverTimeout: discoveryTimeout,` 之后加 `detectCLIs: config.DetectProviders,` | R1（R2 也红，但那是因为这台机器上真装了四家，环境相关，不作杀手） |
| M65 | repl/cliadvice.go | 缓存：`for _, d := range r.detectCLIs() {` → `list := r.detectCLIs()` / `r.detectCLIs = func() []config.DetectedProvider { return list }` / `for _, d := range list {` | R2 |
| M66 | repl/cliadvice.go | `return discoveryFailureText(err, timeout)` → `return err.Error()` | R5、A8 |
| M67 | repl/commands.go | `case others == 0:` → `case len(r.cfg.Agents) == 0:` | R6 `summary_only_config` |
| M68 | repl/commands.go | 数成员时 `if id != sumID {` → `if id != "\x00" {`（总结成员也数） | R6 `summary_only_config` |
| M69 | repl/commands.go | `case sumID != "" && !active[sumID]:` → `case false:` | R6 `summary_missing` |
| M70 | repl/commands.go | 同一行 → `case sumID != "":`（总结成员在场也说「除总结成员外」） | R6 `all_present` |
| M71 | repl/commands.go | `if sumID := r.cfg.Summary.Agent; sumID != "" && !active[sumID] && strings.HasPrefix(sumID, seg) {` → `if sumID := r.cfg.Summary.Agent; sumID == "\x00" {`（前缀那一句永远不打） | R6 `prefix_summary`、`prefix_summary_only`、`prefix_summary_left`（最后一个是 W1b 修订加的用例） |
| M72 | repl/commands.go | 同一行去掉 `!active[sumID] &&` | R6 `prefix_all_present` |
| M95 | repl/commands.go | `case sumID != "" && member[sumID] && !active[sumID]:` → `case false:`（W1b 修订 F-B：去掉「加入过又退出」那一支） | R6 `summary_left`、`summary_only_left` |
| M96 | repl/commands.go | 同一行 → `case sumID != "" && !active[sumID]:`（不看 `member`：没加入过也说已退出） | R6 `summary_missing`、`summary_only_config` |
| M97 | repl/commands.go | 把 `case others == 0:` 与它下面那行 `return` 挪到 `case sumID != "" && member[sumID] && !active[sumID]:` 那一支之前（两支四行对调） | R6 `summary_only_left` |
| M98 | repl/commands.go | `resolveAddSegment` 里 `if member[sumID] {` → `if false {`（前缀没命中时不分已退出） | R6 `prefix_summary_left` |
| M99 | repl/commands.go | 同一行 → `if true {`（没加入过也说已退出） | R6 `prefix_summary`、`prefix_summary_only` |
| M100 | repl/commands.go | P6 的 `printAddCandidates(r, candidates, active, member)` → `printAddCandidates(r, candidates, active, nil)`（接线断了） | R6 `summary_left`、`summary_only_left` |
| M104 | repl/commands.go | P11 的 `if active[seg] {` → `if false {`（v1.2：在场的仍打原句；`-run TestObx\|TestChxAddHidesSummary`） | R6 `summary_present`、CH1 C10 `TestChxAddHidesSummary/active` |
| M105 | repl/commands.go | 同一行 → `if true {`（没加入过也说「已经在」；`-run` 同上） | R6 `summary_present`、CH1 C10 `TestChxAddHidesSummary/explicit` |
| M80 | provider/login.go | `case a.SubscriptionType != "" && !hasControlChar(a.SubscriptionType) && a.TokenSource != "none":` → 去掉 `&& a.TokenSource != "none"` | L1 `claude_none_and_sub` |
| M81 | provider/login.go | 同一行去掉 `!hasControlChar(a.SubscriptionType) &&` | L1 `claude_sub_control_char` |
| M82 | provider/login.go | `case a.TokenSource == "none" && a.SubscriptionType == "" && a.APIKeySource == "":` → `case a.TokenSource == "none" && a.APIKeySource == "":`（W1b 修订：锚点行随 F-A 改写，意思不变，仍是去掉「没有订阅」那一条） | L1 `claude_none_and_sub` |
| M94 | provider/login.go | 同一行 → `case a.TokenSource == "none" && a.SubscriptionType == "":`（W1b 修订 F-A：去掉「没有 apiKeySource」，回到 v1.1） | L1 `claude_api_key_env`、`claude_console_login`（只有这两行红） |
| M83 | provider/login.go | `if _, ok := claudeDiscoveryResponse(line); !ok {` → `if len(bytes.TrimSpace(line)) == 0 {`（不认 request_id） | L1 `claude_other_request_only`、`claude_other_then_ours`、`claude_real_shape` |
| M84 | provider/login.go | `if strings.TrimSpace(first) == grokNotAuthenticated {` → 再或上 `strings.Contains(string(stdout), "\n"+grokNotAuthenticated)`（任何一行都算） | L1 `grok_marker_not_first` |
| M85 | provider/login.go | `grokLogin` 开头加 `if strings.Contains(first, "is using its own API key") { return LoginInfo{State: LoginYes} }` | L1 `grok_api_key_line`、A15 |
| M86 | provider/login.go | `if exitCode == 0 && strings.HasPrefix(l, "Logged in") {` → 去掉 `exitCode == 0 &&` | L2 `codex_logged_in_but_exit1` |
| M87 | provider/login.go | `if exitCode != 0 && l == "Not logged in" {` → 去掉 `exitCode != 0 &&` | L2 `codex_not_logged_in_but_exit0` |
| M88 | provider/login.go | 删掉 `l = strings.TrimSpace(l)` | L2 `codex_not_logged_in_padded`、A9 |
| M89 | provider/login.go | `LoginStatusCommandFor` 的 `if providerID == "codex" {` → `if true {` | L3、A1、A2、A5、A6、A9、A11、A12、A15 |
| M90 | provider/login.go | `ParseLoginStatus` 删掉 `if providerID != "codex" { return LoginInfo{} }` | L2 `claude` |
| M91 | turn/loginstatus_exec.go | `return ee.ExitCode(), outBuf.Bytes(), errBuf.Bytes(), nil` → 最后一项 `waitErr`（非 0 退出当错误） | L4 `exit1_stderr`、`stdout_and_args`、A9 |
| M92 | turn/loginstatus_exec.go | `cmd.Stderr = &capWriter{buf: &errBuf, …}` → `buf: nil`（丢掉 stderr） | L4 `exit0_stderr`、`exit1_stderr`、A1、A2、A5、A9、A11、A15 |
| M93 | turn/loginstatus_exec.go | `fmt.Errorf("login status killed: %w", ctx.Err())` → `%v` | L4 `killed` |

**判定为等价、不列杀手的变异**（原型实跑存活，理由）：
- M7 `knownInstallPath` 去掉 `if base == "" { return "" }`：拼出的相对路径被 `detectProvider` 的 `!filepath.IsAbs(p)` 挡住。
- M8 `detectProvider` 的 `if p == "" || !filepath.IsAbs(p) {` → `if p == "" {`：环境变量为空时 `knownInstallPath` 已经返回空串；环境变量本身是相对路径的情形没有夹具（真实 Windows 不会这样设）。M7、M8 合起来是 M9，被杀。
- M13 `tomlCommandString` 只看单引号、不看控制字符：Windows 文件名不许含码位 1-31 的字符，路径里造不出控制字符。v1.2 起 `data_dir` 也经 `tomlCommandString` 写，值同样是 Windows 路径，同一个理由；v1.2 没有重跑这一条。
- M26 `CreateConfigFile` 的 `if err == nil || errors.Is(err, fs.ErrExist) {` → `if err == nil || (errors.Is(err, fs.ErrExist) && false) {`（硬链接撞上已有目标也走退路）：退路的 `O_EXCL` 撞上同一个已有文件，同样返回满足 `ErrExist` 的错误。
- M59 `firstRunCheck` 的 `ok && cctx.Err() == nil` → `ok`（这一家的时间用完了照样起 `codex login status`）：`RunLoginStatus` 一起来就看见 ctx 已结束、杀掉、返回错误，结果同样是查不出；只多起一个进程。没有夹具能看见这个进程。
- M28 `detectProvider` 的 `err == nil && filepath.IsAbs(p)` 去掉 `IsAbs`：go1.19 起 `LookPath` 对相对结果返回 `ErrDot`，`err == nil` 已经蕴含绝对路径（除非 `GODEBUG=execerrdot=0`）；C1 `cwd_planted` 里 `LookPath` 返回的正是带 `ErrDot` 的错误，所以它也存活。

**只在审查时看**（造不出确定的夹具）：
- M20 `createExclusive` 在 `Write` / `Close` 失败时 `os.Remove(path)`（W1 的 X6）：Windows 上造不出「建成功、写失败」。
- v1 的直接建法（先建出空文件再写）那一个「另一个进程读到空文件」的窗口：要看见它，读的一方得在那一刻开着文件，而 Go 在 Windows 上打开文件不带 `FILE_SHARE_DELETE`，同时会挡住临时文件名的删除，测试自己就不稳。改由 C7 `via_link` 断言「硬链接之前目标不存在、临时文件已经写完」，M23（回到直接建）被它确定地杀掉。
- `firstRunDiscover` 生产值是 `turn.RunDiscovery`、`firstRunStatus` 生产值是 `turn.RunLoginStatus`：A1 等端到端用例经它们起真进程（假 `.cmd`），换成别的执行器会让它们全红，不单列。
- `DefaultTOML` 用 `DefaultDataDir` 拼、不手抄：C4 断言含 `DefaultDataDir` 的值，手抄同一个值时等价。
- `obxDrain` 的 `os.Exit` 那一支：只在 `cancel()` 之后 10 s `Run` 还不返回时走到，原型上从没走到过；行为由 §0 第 16 条的探针担保。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许动这些文件：**块 A**：新文件 `internal/config/firstrun.go`（F1）、整个替换 `internal/config/defaults.go`（F2）；**块 B**：新文件 `internal/provider/login.go`（F5）、`internal/turn/loginstatus_exec.go`（F6）；**块 C**：新文件 `internal/repl/cliadvice.go`（F3），补丁 `internal/repl/repl.go`（P5）、`internal/repl/commands.go`（P6-P11），授权的测试改动 `internal/repl/mdx_addmodel_fixture_test.go`（T2）、`internal/repl/mdx_addmodel_test.go`（T3）、`internal/repl/chx_auto_test.go`（T4，v1.2）；**块 D**：新文件 `internal/app/firstrun.go`（F4），补丁 `internal/app/app.go`（P0-P4），授权的测试改动 `internal/app/report_test.go`（T1）。不改 `go.mod`，不新写任何 `obx` 测试文件（那是测试腿的事）。
- 分四块交付，**每次只交一块**，顺序 A、B、C、D：
  - 块 A 约 276 行（F1 185、F2 91；v1.2 后 F1 多了 10 行）。落地后 `go build ./internal/config` 必须能过（它只用 HEAD 已有的 `tomlBasicString`、`DefaultDataDir`），**只跑 `go test ./internal/config`，不跑 app 包**：块 A 单独在时，HEAD 的 `report_test.go` 子测试 3 走旧 `reportConfigError` 把新模板打出来，新模板里没有 `[providers.claude]`，它的 `Contains(templateMarker)` 断言会红（无害：旧代码不进首次运行、不碰 store），要等块 D 连 T1 一起落。
  - 块 B 约 207 行（F5 142、F6 65；W1b 修订后 F5 多了 6 行），只依赖 HEAD。落地后 `go build ./internal/provider ./internal/turn`、`go vet ./internal/provider ./internal/turn` 干净。
  - 块 C 约 123 行（F3 48、P5-P11 的替换文本约 72、T2、T3、T4 各一行；W1b 修订后 P6-P10 多了约 9 行，v1.2 加了 P11 与 T4），依赖块 A。落地后 `go build ./...`、`go vet ./internal/repl` 干净；`go test ./internal/repl` 全绿（T2、T3 与 P9 同一块，缺了它们 `TestMdxAddSkipsUndiscoverableProviders` 的 (a)、(b) 会红；T4 与 P11 同一块，缺了它 `TestChxAddHidesSummary/active` 会红）。
  - 块 D 约 292 行（F4 204、P0-P4 的替换文本约 48、T1 40；v1.2 后 F4 多了 3 行），依赖块 A、B、C。落地后 `go build ./...` 与 `go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn` 必须干净。**P1 与 T1 同一块**（§1.1 T1 的理由）。
  - 上面四块逐块累加打在 `cbaf195` 上的编译与 vet 结果，是 v1.1 原型上用脚本从本文按块取出来实测的（§6 第 1 步同法）；v1.2 在一份新解出的 `cbaf195` 上照同法重核过一次。
- 补丁块（每个文件可以有多个）：

  ```
  ===PATCH: internal/app/app.go===
  <<<<ANCHOR
  （从 HEAD cbaf195 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点照 §1.1 各处的 `anchor` 代码块（本文代码块里的缩进是 tab；若你看到的是空格，以真文件为准），每个在该文件里恰好出现一次，同一文件的多个锚点互不重叠。**注意**：`r.SetAutoReply(!opts.NoAuto)` 在 `app.go` 里出现两次，P2、P3 的锚点必须带上各自的下一行；`commands.go` 有六处补丁（P6-P11），P6 的锚点从 `printAddCandidates(r, candidates)` 那一行起。
- 新文件与整文件替换用 `===FILE: <路径>===` 换行接完整内容、末尾 `===END FILE===`。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略（冻结代码里本来就有的中文省略号 `……` 是字符串内容，照抄）。
- 代码必须能 `gofmt`（tab 缩进）；`go vet` 不许有新报告。**`msgFirstRunNoneFound` 必须用 `io.WriteString` 写**（§0 第 8 条：交给 `fmt.Fprint` / `Fprintf` 时 vet 报 `%U`，`go test` 编译不过）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（全角括号、冒号、分号、逗号、顿号、省略号 `……`、直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的字符串字面量只有 §3 列出的那些，外加 F5、F6 里的错误文本与英文判据串（`You are not authenticated.`、`Logged in`、`Not logged in`、`login status command path is empty`、`login status killed: %w`）。
- 生产代码新标识符不许以 `obx` 开头。
- §1.1 给出的签名、判据、代码块、注释照抄；本刀没有「名字实现方定」处。
- 回复末尾必须有：

  ```
  ===REPORT===
  块：这次交的是块 A、B、C 还是 D；给了哪些文件。
  补全性：F1-F6、P0-P11、T1-T4 各落在哪个 FILE / PATCH 块（只报本块的）。
  锚点：每个锚点是否从 HEAD 真文件逐字复制、在该文件里恰好一次。
  字面量：§3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  ```

---

## 6. 落地后主代理要做的

1. **收割与逐字核对**：越界扫描（主仓库、`internal/*`、`cmd/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；**块 D 的 P1 与 T1 必须同一批落盘，先落 T1 再跑任何 app 包的测试**（§1.1 T1 的理由：HEAD 的子测试 3 套在新代码上会起真 CLI、打开 `E:\council-data`——这是按代码推演的，原型上**没有**拿 HEAD 原文去跑过，正因为这个后果才禁跑）；只落了块 A 时 app 包的那一条假红见 §5。逐字核对：用脚本从本文取出每一对 `anchor` / `replace` 与每个 `file` 代码块（代码块内容加一个换行），打在 HEAD `cbaf195` 的一份 `git archive` 上，结果应当与 grok 落盘后的 13 个文件逐字节相同（v1.2 原型就是这样核过的：16 对锚点各恰好一次、6 个整文件）。块 C 的 P11 与 T4 同一批落盘。`gofmt -l ./internal ./cmd`；`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn`；`go build ./...`。`internal/provider`、`internal/turn` 只多了 F5、F6 两个新文件；`internal/tui`、`cmd/council` 零改动。
2. **合并测试腿的 5 个文件**，先跑 `go test -count=1 ./internal/config ./internal/repl ./internal/app ./internal/provider ./internal/turn`，再跑全量 `go test -count=1 ./...`（原型实测九包全绿）；§2 全部 `TestObx` 跑一次 `-count=3`。**每一次运行都照 §4 表头的运行纪律**（W1b 修订后的版本）：
   - 父环境设 canary 的 `COUNCIL_DATA_DIR`，不设 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`；
   - 四个 profile 变量指到 canary home；`TMP`、`TEMP` 指到隔离目录，不设 `GOTMPDIR`；
   - `GOMODCACHE`、`GOPROXY`、`GOCACHE`、`GOTOOLCHAIN` 显式给。
   - 跑完核五项。第 5 项是没有残留的 `cmd.exe` 循环（A8、A9、A12 的假 claude）：`Get-Process cmd | Where-Object StartTime -gt <开跑时刻>`。
3. **§4 的变异逐条跑**（表内 98 条：v1.1 的 85 条，加 W1b 修订的 M94-M100，加 v1.2 的 M101-M106；M104、M105 的 `-run` 带上 `TestChxAddHidesSummary`），照 §4 表头的运行纪律，从字节副本还原，还原后 SHA256 核对；M7、M8、M13、M26、M28、M59 跑一次确认存活。点名杀手里的每个子测试都要单独确认变红。M33 整条约 23 s（A8 实测 21.25 s，以 errOut 不等而红）。H0 / H1（§2.0）照同一套运行纪律跑一次（父环境的 home 指到 canary home，不设 `GOTMPDIR`）：H0 让 canary 出现 `council.db` 是 v1.1 上预期的正对照，跑完只删 canary 与隔离的 `TMP` 里 H0 留下的东西；v1.2 下 H0 的预期按代码推会变（库落在隔离 `TMP` 里），跑之前先看 §7 (u)。
4. **文档**：
   - `docs/00_DECISIONS.md` #48：标「已落地（commit），真机未复验」；记下 §1.1 D1-D17 的主代理裁定（D17 是 v1.2 的「生成的配置写 `data_dir`」）、§0 第 5、14 条四家「没登录时识别与登录状态的表现」、§7 的待定项。
   - `docs/07_STEP4_SPEC.md`、`07a_STEP4_API_CONTRACT.md` 顶部各加一行订正：「不替用户创建配置文件 / 不创建目录」在默认位置、文件不存在时已被 #48 推翻；`DefaultTOML()` 从 #48 起是全注释的说明文本，不再是 FSDT 示例。
   - `docs/19a_CH1_CONVERSATION_MODE_CONTRACT.md` 文件头 R0 那一条，与第 0 步对在场总结成员的说法（本文 §7 (r)），一并标「已由 OB1 修」（P8-P11）。
   - `docs/README.md`：第一次运行会自动检测并生成 `~/.council/config.toml`（只含找到的工具，外加 `data_dir = ~/.council/data`）；每家显示登录状态（已登录 / 没登录 / 登录状态查不出），claude 已登录时带订阅；四家登录命令；已有配置的用户在 `/add` 里会看到「本机装了 X」的提示，照着追加两行、`/config reload --apply`、`/add --refresh`。
   - 本契约归档为 `docs/19d_OB1_FIRST_RUN_CONTRACT.md`（照 `19a` 的体例在文件头加落地后订正区）。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的真机复验步骤**（Windows Terminal + PowerShell 5.1，全部零花费：只跑识别与 `codex login status`，不提问）。用临时目录模拟一个新用户，不碰他的真配置 `C:\Users\25142\.council\config.toml` 与真库。PowerShell 的命令每条一块，逐块粘贴、逐块回车；council 里的命令也一行一块（#44）。

   **先知道会看到什么**：这个模拟只改了 `USERPROFILE` / `HOME`，而子进程继承环境，所以——claude 与 grok 看到的是「没登录」的样子（claude 约 4 个模型、grok 2 个，两行末尾都是「没登录」），grok 会在临时目录里建一整棵 `.grok`，claude 会建 `.claude`、`.claude.json`，agy 会建 `.gemini`，这些都是那几家 CLI 的正常行为；**codex 与 agy 仍然读你自己的登录态**：
   - codex：原型模拟里是 7 个、「已登录」，临时目录下没有 `.codex`。W1b 实测过，只改 `USERPROFILE` / `HOME` 时 `codex login status` 照旧报 `Logged in using ChatGPT`，它怎么找默认目录没有查。
   - agy：改环境变量隔离不了，登录状态一律「查不出」。

   v1.2 起生成的配置自带 `data_dir = '<模拟 home>\.council\data'`，所以数据目录会落在 `$sim\home\.council\data`。下面第 2 步仍设 `COUNCIL_DATA_DIR`，只是保险：配置里的 `data_dir` 优先于它。

   0. 先确认装的是带本刀的版本（主代理照惯例等 Griffin 说「装」再重装）：

      ```powershell
      council -version
      ```

   1. 建临时目录：

      ```powershell
      $sim = "E:\Isolating_test\council\ob1_realcheck"; New-Item -ItemType Directory -Force "$sim\home","$sim\data" | Out-Null
      ```

   2. 只在这个 PowerShell 窗口里改三个变量（关掉窗口就还原）：

      ```powershell
      $env:USERPROFILE = "$sim\home"; $env:HOME = "$sim\home"; $env:COUNCIL_DATA_DIR = "$sim\data"; $env:COUNCIL_CONFIG = ""
      ```

   3. 第一次运行（进 TUI）：

      ```powershell
      council
      ```

      看：TUI 画出来**之前**，屏幕上先有「第一次运行：还没有配置文件 E:\Isolating_test\council\ob1_realcheck\home\.council\config.toml，council 来生成一份。」「正在检查 claude、grok、agy、codex 能不能用（只让它们列出模型、报登录状态……」，约 6 秒后四行结果：claude「识别到 4 个模型；没登录」、grok「识别到 2 个模型；没登录」、agy「识别到 14 个模型；登录状态查不出」、codex「识别到 7 个模型；已登录」（个数以你那时的 CLI 为准；你的终端里要是设着 `ANTHROPIC_API_KEY`，claude 那一行末尾会是「登录状态查不出」而不是「没登录」，见 §1.1 D15，这是按程序文本推的，没实跑过）；接着「能列出模型不代表已经登录……要登录在终端里运行：」与 claude、grok、agy 三行登录办法（codex 已登录，不列）；「已生成配置文件: …」与上手三步；然后是横幅（「配置文件: …\home\.council\config.toml」「数据目录: …\home\.council\data」「已加载 0 个 agent，0 个 team」）和 TUI 空闲表头。

   4. 在 council 里：

      ```
      /new 首次运行复验
      ```

      ```
      /add
      ```

      看：第一行是「配置里还没有可以 /add 的成员」；「正在识别模型: agy, claude, codex, grok」之后是识别表；**表后没有**任何「本机装了 …」的提示（四家都已经写进配置了）。然后退出：

      ```
      /quit
      ```

   5. 看生成的文件：

      ```powershell
      Get-Content "$sim\home\.council\config.toml" -Encoding UTF8
      ```

      看：开头是全注释的说明；接着一行「# 数据目录（本地库与成员的工作目录），……」与 `data_dir = 'E:\Isolating_test\council\ob1_realcheck\home\.council\data'`；末尾是「# 下面是 council 第一次运行时在本机找到的工具。」和四段 `[providers.*]`，`command` 是单引号括起来的绝对路径；没有 `[agents.*]`、`[summary]`。四家的路径与 §0 第 2 条相同时，文件是 3114 字节（§0 第 22 条，原型上算出）。`$sim\home\.council` 里只有 `config.toml`（没有 `config.toml.tmp-…`）。

   6. 第二次运行不再触发：

      ```powershell
      council -no-tui
      ```

      看：直接是横幅与 `council>`，没有「第一次运行」；敲 `/quit` 退出。

   7. `/add` 的提示：删掉生成的文件里 `[providers.grok]` 那两行（用记事本打开 `$sim\home\.council\config.toml`），再

      ```powershell
      council -no-tui
      ```

      依次敲（一行一块）：

      ```
      /new 提示复验
      ```

      ```
      /add
      ```

      看：识别表之后有一行「本机装了 grok（C:\Users\25142\.grok\bin\grok.exe），配置里还没有：要用它，把下面两行追加到 … 末尾，再敲 /config reload --apply 与 /add --refresh」和那两行；照着追加、`/config reload --apply`、`/add --refresh` 之后提示消失、识别表里多出 grok。`/quit` 退出。

   8. 显式路径找不到时照旧报错：

      ```powershell
      council --config "$sim\typo.toml"
      ```

      看：两行「找不到配置文件: …\typo.toml」「这个路径是你用 --config 或环境变量 COUNCIL_CONFIG 指定的，请确认它没写错。」，没有「第一次运行」，`$sim\typo.toml` 没有被建出来。

   9. 收尾：关掉这个 PowerShell 窗口（三个变量随之还原），确认真配置没动：

      ```powershell
      Get-Item C:\Users\25142\.council\config.toml | Select-Object Length, LastWriteTime
      ```

      与复验前记下的一样即可；`$sim` 整个删掉。

---

## 7. open items

**待 Griffin**

- (b) **agy 的登录状态零花费判断不了**（§0 第 5 条）：首次运行里 agy 一律「登录状态查不出」，所以只要找到 agy，就一定会打登录说明与「agy（按提示登录）」这一行，已经登录的人也会看到。
- (b2) **「配了 API key、或用 login 以外的办法登录」算不算已登录**（v1.2 按主代理裁决第 3 条，把原来的 (b2)、(b3)、(q) 并成这一个问题；三条原是同一件事）。
  - 现状（本刀维持）：下面几种一律显示「登录状态查不出」，并附上那一家的登录办法。这是实话，也不花钱；代价是已经能用的人也会被建议去登录。
  - 涉及的几种首行或字段：
    - grok 的 `Model '<名字>' is using its own API key.`（原 (b2)）：本机登录态下的实测首行，Griffin 的 grok 经 CPA 走 API key。它证明的是那个模型配了自己的 key，不是 `grok login` 做过，所以 Griffin 自己首次运行时 grok 会显示「登录状态查不出」、并被建议 `grok login`。`grok --help` 里没有 status 一类的子命令。
    - grok 的另几句首行（原 (b3)）：W1b 从 grok 1.0.41 的程序文本（`crates/codegen/xai-grok-pager/src/models.rs` 附近）读到，`grok models` 的首行是一组互斥的认证状态句：`You are using XAI_API_KEY`、`You are logged in with …`、`Model '…' is using its own API key`、`You are authenticated via deployment key`、`You are not authenticated`。其中「You are logged in with …」才是 `grok login` 做过的正面信号，另外三句说的都是 key。这段程序文本修订人没有复核（程序在 `~/.grok` 下，不碰），本机也没有 grok 的 login 态，一次都没见过。
    - claude 的 `apiKeySource`（原 (q)）：用 `ANTHROPIC_API_KEY` 或 Console 账号登录的用户，`tokenSource` 是 `none`、没有 `subscriptionType`、带 `apiKeySource`（§0 第 20 条，程序文本，未实跑）。W1b 修订之后他们落在「查不出」，并照「查不出的都列登录办法」附上 `claude auth login`。本机没有这种登录态，不许改登录态去造。
  - 要 Griffin 定的只有一件：配了 key（或别的登录方式）要不要显示成「已登录」，要的话是只说「已登录」三个字，还是另起一种说法（例如「已登录（API key）」）。
  - 改的话：`provider/login.go` 的 `grokLogin`、`claudeLogin` 各加一支；L1 加行、A15 的期望跟着改；§4 的 M85 要改写，另加变异。证据等级要照实写：grok 的 `You are logged in with …` 与 claude 的 `apiKeySource` 都只有程序文本，没有实跑；裁决第 1 条要正面信号「真登录态与没登录时都能稳定区分」，只凭程序文本加一个「已登录」，判错就是裁决第 3 条的 major。
- (c) **登录状态的判据只对实测的版本成立**（claude 2.1.283、grok 1.0.41、codex-cli 0.147.0，§0 第 14 条）。W1b 修订后按新判据重核了「升级之后会不会说错」：
  - 正面信号都是 CLI 明写的值：claude 的 `subscriptionType` 原值、codex 以 `Logged in` 开头的一行。它们在升级后消失，只会退回「查不出」。
  - codex、grok 的负面标记是 CLI 打出来的整句（`Not logged in`、`You are not authenticated.`），文字变了也只会退回「查不出」。
  - **claude 的负面标记不一样**：它是「`tokenSource` 为 `none`」加上两个「缺字段」（没有 `subscriptionType`、没有 `apiKeySource`）。只要出现一种新的登录方式、又不带这两个字段，或者这两个字段改了名，已经能用的用户就会被说成「没登录」。
    - W1b 就找出了一种：API key 与 Console 登录（F-A）。修订后用 `apiKeySource` 挡住了，别的还没见过。
    - claude 每次升级，都要按 §0 第 20 条的办法重核这三个字段。
  - v1.1 这里原写「不会说错成已登录或没登录」，对 claude 这一支不成立，已删。
  - `claude auth status`（JSON，`loggedIn`）与 `codex login status` 都实测能零花费分开两种状态。claude 用识别那一次的 `account`，不用 `auth status`（要多起一个进程）；codex 用 `login status`。
- (e) 「没登录的 claude、grok、codex 也会列出一部分模型」这句只对实测的版本成立；CLI 升级后行为可能变，这句话不会跟着变。

**待主代理**

- (d) `killProcessTree` 按 `PATH` 找 `taskkill`（§0 第 9 条）：`PATH` 里没有 System32 时超时杀树只杀到垫片一层。本刀的测试绕开了它（超时夹具不起孙进程）；`RunLoginStatus` 同样受影响。要不要改成 `%SystemRoot%\System32\taskkill.exe` 另开一刀。
- (f) 「正在检查 claude 能不能用（只让它们列出模型……」只有一家时「它们」读着别扭；改措辞要改 §3.1 与 A6、A12，本刀不改。
- (g) 首次运行输出写 stderr（D9），`council 2>nul` 看不到；接受。
- (h) `TestDefaultTOMLPermissionsAndModeAreLowercase` 变空、`report_test.go` 子测试 1、2 的模板缺席断言失去正对照（§1.2）：要不要在以后某一刀里收拾（删或改），不在本刀。
- (i) `PATHEXT` 为空、环境变量是相对路径这两种取值没有夹具（D4、§4 等价表），只在审查时看。
- (j) 首次运行不跑 `/add` 的「本机装了」提示（首次运行写进去的就是检测到的全部）；首次运行之后才装的工具，由 `/add` 的提示接住（A11 实测）。
- (k) **当前目录里的同名文件会遮住 `PATH` 上的安装**（§0 第 13 条，W1 F1）：真实终端里（没有 `NoDefaultCurrentDirectoryInExePath`）当前目录有 `claude.cmd` 时，`LookPath` 返回 `ErrDot`、不再往后查 `PATH`，检测只剩三处已知位置；装在别处的会被判成没找到。要修得自己遍历 `PATH`，本刀不做。
- (m) **临时文件可能留下**：`CreateConfigFile` 最后删临时文件名时，如果另一个进程正好开着这个配置文件（例如同时启动、撞车改用它的另一个 council 正在读），Windows 上删不掉（Go 打开文件不带 `FILE_SHARE_DELETE`），`~/.council` 里会留一个 `config.toml.tmp-<数字>`。不影响配置本身。没有夹具。
- (n) **类 Unix 上生成的配置是 0600**：`os.CreateTemp` 建的是 0600，硬链接沿用它；v1 的 `O_EXCL` 写法是 0644。Windows 上没有区别。
- (o) **`/add` 的识别表不显示登录状态**（裁决第 1 条：本刀只在首次运行的表里显示）。`/add` 的「一个模型都没列出来（没登录？）」照旧。
- (p) **起草期间的泄漏与处置**（W1 F0）：v1 原型跑 M33 时泄漏的 `Run` 在 `E:\council-data` 新建了一个库（`council.db` 90112 B、空的 `council.lock`，创建于 2026-09-29 17:24:38），并在 `%TEMP%\TestObxFirstRunFailures3217426626` 重建了临时 home。主代理核过：Griffin 的配置写的是 `data_dir = 'C:\Users\25142\.council\data'`，`COUNCIL_DATA_DIR` 在进程、User、Machine 三级都没设，他的 `config.toml` 最后修改是 9/28 19:21，所以都不是他的数据；两处已于 2026-09-29 19:20 逐个删除。v1.1 的 F0 修法与 §4 的运行纪律防的就是这个。
- (s) **W1b 修订那两句新说法由修订人冻结**（§3.6 第 4、5 行）：「总结成员 X 已退出当前会话，要恢复总结用 /join X；除它之外配置里没有可以 /add 的成员」，与前缀那一句。
  - 取舍：
    - 指 `/join`，是照 `runSummary` 的原话（`summary.go:95`）。`/add X` 其实也放行（第 0 步），没写进去，免得两处说法不一。
    - 后半句「除它之外配置里没有可以 /add 的成员」，是为了在「其余都在场」与「一个都没有」两种情形下都成立。
  - 主代理要换措辞的话，改 P9、P10、§3.6 与 R6 三条新用例的期望。
  - v1.2 第 0 步那一句（§3.6 最后一行）「X 是总结成员（config.toml 的 [summary]），已经在当前会话里了」也由修订人冻结：裁决只给了「形如」，修订人保留了括号里的「config.toml 的 [summary]」，与同一步没加入过的那一句对齐。要换的话改 P11、T4、§3.6 与 R6 `summary_present`。
- (t) **「已生成配置文件: %s（只写了找到的工具，还没有成员）」严格说不全**（v1.2 修订时发现，同类的别处）：v1.2 起生成的配置还写了 `data_dir`。括号里要说的是「有工具、没成员」，紧接着的横幅也会打出「数据目录: …」，所以没有误导；但字面上漏了一样。这句不在裁决的授权里，本刀不改。要改的话动 §3.2、F4 与所有带 `obxDone` 的期望。
- (u) **canary 的正对照 H0 在 v1.2 下要重新看**（v1.2 修订时发现，按代码推演，没有实跑）：
  - v1.1 的 H0 让 canary 出现 `council.db`，是因为泄漏的 `Run` 在环境变量还原之后按 `COUNCIL_DATA_DIR`（canary）开库。
  - v1.2 生成的配置写了 `data_dir = <临时 home>\.council\data`，它优先于环境变量，所以按代码推，同样的泄漏会把库建在隔离 TMP 里重建的临时 home 下：第 1 项（canary）看不见，第 2 项（隔离 TMP 下有 `TestObx*`）看得见。
  - 修订人没有跑 H0：这次修订的硬约束是任何一项核对出现就停，而 H0 就是去造这一项。
  - 两条路：集成员照 v1.2 重跑 H0，把「第 2 项出现、canary 空」写成新的正对照（第 1 项此后只剩「没写 `data_dir` 的路径」一类，例如 A6 那种准备好的文件）；或者换一个装置，让泄漏的 `Run` 读的是没有 `data_dir` 的配置。

**已定（v1.2，主代理裁决 `ob1_rulings_v12.md`）**

- (a) **数据目录默认值是本机专用的**（§0 第 12 条）→ 已定：生成的配置写一行 `data_dir = '<home>\.council\data'`（D17、F1、F4）。`DefaultDataDir` 不改，没写 `data_dir` 的现有配置照旧回落。原来的问题：`DefaultDataDir = E:\council-data`，生成的配置不写 `data_dir`，没有 E 盘的新机器上首次运行成功、随后 `store.Open` 失败。
- (r) **`resolveAddSegment` 第 0 步对在场的总结成员也说「它会自己加入做总结」** → 已定：主代理追加授权，在场时改说「X 是总结成员（config.toml 的 [summary]），已经在当前会话里了」（P11、T4、R6 `summary_present`、M104、M105）；没加入过的说法与已退出的放行不变。原来的问题：这是 HEAD（CH1，`commands.go:1032-1034`）的句子，W1b 修订时发现总结成员已经在场时后半句不准，当时不在裁决的授权里。
