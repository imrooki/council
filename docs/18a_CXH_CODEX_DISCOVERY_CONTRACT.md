# CXH 冻结契约：codex 模型接进 /add 识别，/help 补上 /add +编号 与 --refresh（v1.1，2026-09-28）

> **落地后订正（2026-09-28，主代理；CXH 已于 `db8c41a` 落地）**：独立审查 5 条 minor，代码都没错：
> - codex 一支的 slug 规整（sanitizeModelID）与 agy- / grok- 前缀的撞名改写没有用例覆盖（只有 claude- 前缀有）。
> - codex.go ParseTurn 注释「在 turn.failed / error 两种失败结局上也带回 thread_id 与 UsageRaw」：error 结局只在 completed == 0 时成立，UsageRaw 永远为空；ModelDiscovery 注释把「零花费」与「实测」写进同一个括号，零花费是强推断（本地 debug 子命令、约 0.2 秒、不调用模型），没有计费观测。
> - 默认强度 high 的理由只写了一面：codex 自己给 gpt-6-sol 的默认是 medium、gpt-6-astra 是 low，Council 默认 high 比它们贵、推理更长（主代理维持 high，已在 Griffin 的配置注释里写明想省就 `--effort medium`）。
> **主代理裁定（2026-09-28 14:30，先读）**：§7 待主代理里「U43A 交来的 `/help` `/session` 那一行」不并进本刀，以后单独一刀；§7 待主代理第 11、12 条（custom 来源的两处冻结注释补 [推断]、「三家」之类过期计数）留给以后的注释小刀，本刀不改。
## v1.1 变更记录（2026-09-28，基线 `4dbd17a`）

依据：W1 审查的 F0-F5 与证伪者判词 V0、主代理裁决书（全部接受）。§1.1 冻结的生产代码只改裁决点名的几处；新增与改动的期望值都在原型 `E:\Isolating_test\council\cxh_v11\`（`git archive 4dbd17a` 解包，补丁与夹具块由脚本从本文逐字取出）上实跑得出；新增变异都实跑确认点名的杀手变红。

- **基线换成 `4dbd17a`**（= origin/main，FU2 已落地）。全文行号与锚点按 `4dbd17a` 重取：A、B 两段补丁的每个锚点在 `4dbd17a` 真文件里恰好一处（脚本核对）。FU2 没有改到 v1 的任何锚点，只让 `codex.go` 的行号后移：`ModelDiscovery` 那段 `:108-115` → `:110-117`，强度参数 `:170-172` → `:173-175`，`ParseTurn` 注释 `:254-276` → `:258-282`。其余被引用的文件在两个提交之间没有变动（`addmodel.go` 只在 `:376-378` 改了注释、行数不变；W1b 修订按 `git diff -U0 485581f 4dbd17a` 订正了 v1.1 写的 `:373-378`，默认 3 行上下文的 hunk 头是 `@@ -373,9 +373,9 @@`），行号不变。文件头的 FU2 相交表改写成「已落地、三处含注释的锚点都没被动过」，`addmodel.go:53-55` 那一块保留。
- **F0（major）**：§2.0 F2 加 R8 的配置与 codex 输出（`cxhCodexOnlyTemplate`、`cxhGpt55AndGrokCustom`），G2 加两段期望（`cxhAllConfiguredBlock`、`cxhGrokDownBlock`，原型实跑取得）；§2.3 加 R8 `TestCxhAddNoteWhenAllConfigured`、R9 `TestCxhAddNoteBeforeFailures`；§4 加 C44（W1 的 X1，`&& len(visible) > 0`）、C45（W1 的 X2，写成说明行块与失败行循环整段对调的一处替换）、C47（W1 的 X4，`&& snap.okCount > 1`）。§4「只在审查时看」原第 3 条（「R6 empty 钉住另一端」）删掉，位置改由 R1、R8、R9 担保，写进 R8、R9 的【判别力】；§2.3 R6 的「Empty 时不打 custom 说明」那句成立，保留。
- **F1**：C4 的替换文本改成解析不到的路径 `cxh-nosuch-codex`，跑变异时不起真 codex（原型实测：T1 红在 `exec: "cxh-nosuch-codex": executable file not found in %PATH%`，stdout 空）；杀手补 turn:TestCxhRunDiscoveryCodexCommand。
- **F2**：C40 的杀手填实测的 provider:TestCxhCodexCapabilityAndCommand；C17 补 repl:TestCxhAddCodexFailures。§4 表的杀手列按 v1.1 原型重跑 47 条的结果整列重写：v1 列出的杀手一个不少；加了 R8、R9 之后另有 21 行多出 R8 或 R9（C1、C5-C11、C13、C14、C18-C20、C23、C28-C31、C35、C36、C42），C4 除了 turn 也多出 R8、R9。
- **F3**：A1 `DiscoveredModel` 注释改成「claude 的 JSON 里这个键缺席时是 nil，codex 一档都没有时是 nil」（原型探针：claude 的 `"supportedEffortLevels":[]` 解出非 nil 的空切片，审查员说得对）；A2 `parseCodexDiscovery` 头注释里「codex 自己的选择器也不列它们」标成 `[推断]`（取裁决给的第一种写法：保留过滤的理由，不当事实写）。
- **F4**：§6 第 5 步加前提：先在备份副本里查有没有以 `[providers.codex]` 开头的行，有就跳过追加，只做加载校验并把现有 `command` 原样报给 Griffin。
- **F5**：P4 加子测试 `vendor_slash`；§4 加 C46（custom 判据写成含斜杠），原型实测杀手只有 P4（P2 不红，G1 不动）。
- **顺带并进本刀（FU2 独立审查余项，只改注释）**：A3 `codex.go` 多 3 处：`codexPermissionArgs` 注释「workspace 不给」那段的「见上一段」指错了段落，改指「Windows 上的 read-only」那一段（文件里顶层引号用「」，不用『』），并按文件原有的约 80 列折行；`ParseTurn` 注释与 `codexUsage` 注释补「UsageRaw 只来自那一条 turn.completed，没有 turn.completed 的失败轮它为空」（原型探针实测：turn.failed 加一条带 usage 的 turn.completed 时 UsageRaw 非空；只有 turn.failed、只有 error、turn.completed 的 usage 为 null 时都为空）。新增 A6 `internal/provider/turn.go` 2 处：`Usage` 注释 `:92-94`（「三家的原始 usage 对象整个存进 messages.meta["usage_raw"]」改成只有成功轮），`UsageRaw` 字段注释 `:173` 之后补一句今天只有成功轮兑现；`:171-172` 是 09 F 节 / 05 第 3 节承诺的引文，引文本身不改。真文件里说「各家原始 usage 都进 usage_raw」的就这两处（`internal/provider` 与 `internal/turn` 的生产文件 grep 过；`internal/turn/runner.go:1341-1344` 那段在只有成功轮才走到的 `agentMeta` 里，说的没错，不动）。B 部分新增 B7：`cxx_codex_test.go:8-9` 文件头注释（不是变红的用例，裁决只授权改这一处注释）。§1.2、§5 跟着改。
- **顺手订正（裁决没点名，只改契约正文，不动冻结代码）**：§4「只在审查时看」第 1 条原写 `codex.go` 注释里「custom 来源」的推断标注齐全，`codex.go` 的注释根本不提 custom，这半句删掉。A1 的 `Custom` 注释与 A2 的 `codexCustomSlugPrefix` 注释把 custom 行的来源写成事实、没标推断，属冻结代码，本版不改，列进 §7 待主代理第 11 条。
- **规模**：A 部分 6 个文件，原型 +169 / -35 行（v1 是 5 个文件 +149 / -22）；B 部分 3 个文件 +21 / -21。变异 47 条：43 条非等价，全被点名的杀手杀掉；4 条等价（C21、C22、C24、C41），实跑存活。

### W1b 修订（2026-09-28，第二轮审查 W1b 的 4 条 minor；基线仍是 `4dbd17a`）

依据：W1b 的返回（没有 blocker，没有 major，4 条 minor）。四条都处理了：三条照改，第 3 条是方向问题，取保守读法登记待主代理。另外按「同类的还有没有别处」补了一条用例与一条变异（见下面「同类」一项）。原型在 `E:\Isolating_test\council\cxh_fix\`：`git archive 4dbd17a` 解包，脚本从修订后的本文逐字取出两段补丁与四个夹具 / 期望块落盘（锚点数 12/3/5/2/5/1 与 7/3/1，每个恰好一处），§2 的用例照本文重写。实跑结果：`gofmt -l` 为空，`go vet` 三包干净，`go build ./...` 通过；全量 `go test -count=1 ./...` 九包全绿；只落 A 部分时恰好 6 条既有用例红（B1-B6）；§4 的 C1-C50 全部重跑，每条实际变红的用例集合与杀手列逐条相等，等价的 4 条存活，每条都从字节副本还原并逐字节核对相等。

- **W1b-1（P4 的 `vendor_slash`，照改）**：输入从 `vendor/cxh-x` 换成 `vendor/custom/cxh-x`，整值 DeepEqual 里的 `Model` 跟着改。§4 新增 C48：判据写成 `strings.Contains(m.Slug, codexCustomSlugPrefix)`，杀手只有 provider:TestCxhParseCodexRejects。原型实测：C46、C48 都只让 P4 红，P4 里只有 `vendor_slash` 子测试红。负对照：同一装置只把输入换回 `vendor/cxh-x`，C48 存活，C46 照样红。P4 的【判别力】改写。G1、G2 不动：这个输入写在 §2.1 正文里，不在期望块里。
- **W1b-2（「说明行打在表格之前」能写成一处替换，照改）**：§4 新增 C49，把说明行块与它前面的 `if len(visible) > 0 {...} else if snap.okCount > 0 {...}` 整段对调。替换前的文本由脚本从原型的落地文件取出（1443 字节、36 行），恰好一处，能编译。实测杀手是 repl 的 R1、R2、R5、R8、R9，其中 R2 只有 `one` 子测试红。§4「只在审查时看」原第 3 条删掉；R1、R2、R5、R8、R9 的【判别力】补上 C49。
- **W1b-3（U43A 交来的 `/session` 那一行，方向问题，不自己定）**：取保守读法，本刀不收 `commands.go:153`，登记为 §7 待主代理第 13 条，并列出并进时要补的锚点、字面量、用例与变异。文件头「U43A」那一条把「v1 契约」改成「当前的契约（现查是 v1.1）」，并写明这一项的去向。
- **W1b-4（ParseTurn、codexUsage 两段注释里「失败路径上也带回 thread_id」太宽，照改；属冻结代码）**：A3 的两段替换文本改写，锚点不变（`:277-279`、`:385-388`）。
  - ParseTurn 那段写明三种失败结局各带回什么：turn.failed / error（ErrTurnFailed）带回 thread_id 与 UsageRaw；成功形状但 thread_id 为空（ErrNoSessionID）带回 UsageRaw；认不出的形状（ErrUnparseableOutput）返回零值。ErrNoSessionID 那一种是本次问「同类的还有没有别处」补出来的，W1b 给的修法里没有它。
  - codexUsage 那段改成「一部分失败结局上也带回 thread_id 或 UsageRaw（哪几种、各带回什么见 ParseTurn 的注释）」，保留「UsageRaw 只来自那一条 turn.completed」的限定。
  - 原型探针用七种输入调 ParseTurn，实测：
    - 只有 thread.started 加 turn.failed：session=T1，UsageRaw 空；
    - 再加一条带 usage 的 turn.completed：session=T1，UsageRaw 非空；
    - thread.started 加 error：session=T1，UsageRaw 空；
    - 没有 thread.started 的成功形状：报 ErrNoSessionID，session 空，UsageRaw 非空；
    - 两条 turn.completed、只有 thread.started、非 JSON：都报 ErrUnparseableOutput，结果是零值 TurnResult。
  - 「失败轮的 UsageRaw 被丢弃」对这三种都成立：`runner.go:729-741` 对 ErrUnparseableOutput 以外的错误（含 ErrTurnFailed 与 ErrNoSessionID）先 absorbParse 再 fail-closed，对 ErrUnparseableOutput 跳过 absorbParse；absorbParse（`:1227-1237`）不碰 UsageRaw。
  - §4 没有哪条变异碰这两段注释。
- **同类（本次加问，审查员没提）**：W1b-1 那一类是「前缀判据写成包含」。CXH 自己的代码里还有一处同类：`SuggestedID` 的 codex 撞名规则 `strings.HasPrefix(id, other+"-")`。写成 `strings.Contains` 时，原型上 provider、repl、turn 三个包的 CXH 正则全部存活，因为夹具里会撞名的 slug（`claude-cxh-9`）都以 provider id 开头。处理：P3 加子测试 `infix`（`cxh-claude-x` 以 high 得到 `cxh-claude-x-high`），§4 新增 C50，实测只有 P3 的 `infix` 子测试红。夹具与期望块不动。MD1 的 agy 一支有同一条规则、同一个缺口：写成 `Contains` 时 provider、repl、turn 三个包的全量用例都不红（原型实测）。那是已落地的 MD1 冻结代码，本刀不改，记进 §7 待主代理第 14 条。
- **行号订正**：文件头「`addmodel.go` 只在 `:373-378` 改了注释」改成 `:376-378`。这是 W1b 指出的，已用 `git diff -U0 485581f 4dbd17a` 核对。
- **规模**：A 部分 +170 / -33（v1.1 是 +169 / -35）。差别全在 `codex.go` 那两段注释，其余五个文件的补丁与 v1.1 逐字节相同；`codex.go` 现在是 +25 / -15。B 部分 +21 / -21，不变。变异 50 条：46 条非等价，全被点名的杀手杀掉；4 条等价，不变。§1.1 A 部分的标题、§5 的规模、§6 第 3 步（C1-C50）都跟着改了。
- **没动的**：§1.1 其余冻结代码、B 部分、§2.0 的全部夹具与期望块、§3 的字面量，一个字节都没改。

---

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `4dbd17a`（= origin/main）。本文行号与锚点一律以 `4dbd17a` 为准（v1.1 修订时逐个打开真文件核对）。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`（不设 `GOFLAGS`）。本机没有 gcc，不能 `-race`。
- 本刀的 KEY 是 `CXH`；测试腿的包级标识符前缀是 `cxh`（类型、函数、变量、常量一律 `cxh` 开头，测试函数 `TestCxh...`）。`4dbd17a` 上 `internal/` 里 `cxh` / `Cxh` 零命中（v1.1 修订时 grep 过，不分大小写）。生产代码的新标识符不许以 `cxh` 开头。
- 来源（方向照原话，本契约不改方向）：
  - `docs/00_DECISIONS.md` #46「待主代理」最后一句（`:2165`）：codex 模型识别（`codex debug models` 接进 MD1 框架）另开一刀，届时 `ModelDiscoveryUnknown` 改成 Subcommand；
  - #46 末尾 Griffin 2026-09-28 11:30 的裁定（`:2169-2175`）：`/help` 补上 `/add +编号` 与 `--refresh`（「唯一要动代码的一条，下一批做」）；其余待定项授权主代理定；
  - #45 第一节（`:2114-2125`）：识别后直接 /add、id 跟「模型-版本-强度」、只在两家重名时加来源前缀、选强度只给该模型支持的档；
  - 已落地契约：`docs/16a_MD1_MODEL_DISCOVERY_CONTRACT.md`（识别框架、快照、`/add +编号`；open item (e) `/help` 看不到新用法）、`docs/17b_CX1_CODEX_ADAPTER_CONTRACT.md` §7 待主代理第 5 条（逐模型强度子集）、第 11 条（codex 识别另一刀）、第 14 条（`addmodel.go:54-55` 过期注释）。
- **与 FU2（已落地，`4dbd17a`）**：FU2 改了 `latercmd.go`、`codex.go`、`addmodel.go` 的注释与测试。CXH 在其中两个生产文件上动的位置，逐处（`4dbd17a` 行号）：
  | 文件 | CXH 动的位置（`4dbd17a` 行号） | 性质 |
  |---|---|---|
  | `internal/provider/codex.go` | `codexEffortLevels` 声明（`:50`，锚点是这一行代码，在它上方补一段注释）；`Capabilities()` 里 `ModelDiscovery` 那段注释与字段（`:110-117`，锚点含 7 行注释）；`codexPermissionArgs` 注释「workspace 不给」那段（`:227-229`）；`ParseTurn` 注释里说失败路径的三行（`:277-279`）；`codexUsage` 注释（`:385-388`） | 注释 + 一个字段值 |
  | `internal/repl/addmodel.go` | `discoverableProviders` 头注释（`:53-55`，锚点全是注释）；`modelSnapshot` 结构体末尾（`:37-39`）；`takeModelSnapshot` 循环（`:134-135`）；`printModelSnapshot` 提示行（`:200-207`）；`cmdAddDiscovered` 强度 switch 第一支（`:325`） | 代码 + 注释 |
  - v1 担心的三处含注释的锚点，FU2 一处都没动（原型脚本在 `4dbd17a` 上逐个核对，每个恰好一处）：`codex.go:110-117` 仍是「声明 Unknown、不许声明成 Subcommand」的原文，以 CXH 的替换文本整段替换；`codex.go:42-49` 的注释里没有「识别层」一类的话，`:50` 上方照补；`addmodel.go:53-55` 仍写「没有适配器，例如 codex」「#45 不接 codex」，**保留 CXH 这一块**。`codexPermissionArgs`、`ParseTurn`、`codexUsage` 三处注释里被 CXH 替换的文字正是 FU2 写的，CXH 在它上面订正（见变更记录「顺带并进」），不回退 FU2 的其它改动。
  - FU2 没有动 B 部分的三个既有测试文件（`485581f..4dbd17a` 之间这三个文件零改动）。
  - **U43A**（#43(a)，`/session rebuild` 结清陈旧在飞记录）：按它当前的契约（`E:\multi_model_cowork\u43a_contract.md`；W1b 修订时现查，标题是 v1.1）现查，它动 `internal/repl/ask.go`、`internal/turn/runner.go`、`internal/turn/recover.go`、`internal/conversation/service.go`，新建 `internal/turn/settle_stale.go` 与 `u43` 前缀的测试文件，不新写 `TestMain`；与 CXH 动的文件（上表加 `discovery.go`、`discovery_parse.go`、`provider/turn.go`、`commands.go:143`、B 部分的三个测试文件与 §2 的五个新文件）**不相交**。同包只在 `repl`、`turn` 两个包的测试：标识符前缀 `u43` 与 `cxh` 不撞。两刀落地先后无要求；U43A 修订后若动到 CXH 的任何文件，两刀串行落地，后落者按先落之后的真文件重取锚点。U43A v1.1 的 §7.2 待主代理第 1 条把 `/help` 里 `/session` 那一行（`commands.go:153`，不提「先结清」）交给主代理定：并进 CXH 还是另开一刀。本刀取保守读法，不收这一行，登记为 §7 待主代理第 13 条；两刀都不动 `:153`，不相交的结论不变。
- 本契约的全部期望值都在隔离拷贝 `E:\Isolating_test\council\cxh_v11\` 的原型上实跑得出：`git archive 4dbd17a` 解包，§1.1 的两段补丁、§2.0 的夹具块与期望块由脚本从本文逐字取出落盘（所以本文与原型是同一份字节），测试照 §2 写。**原型下全量 `go test -count=1 ./...` 九包全绿**；只落 §1.1 A 部分（生产代码）、不落 B 部分时，恰好 6 条既有用例变红（§1.1 B1-B6 逐条对应；B7 只改注释，不对应红的用例）。§4 的 50 条变异在原型上逐条实跑，46 条被点名的杀手杀掉，4 条判为等价（§4 表后）。W1b 修订后在 `E:\Isolating_test\council\cxh_fix\` 按同一办法（`git archive 4dbd17a`，脚本从修订后的本文逐字取补丁与夹具块）重建原型，把这一段的每一项重跑了一遍：九包全绿；只落 A 部分时恰好 6 条既有用例红；50 条变异的实际红集合与杀手列逐条相等。原型不交付，用完已删。（v1 的原型在 `cxh_draft\`、基线 `485581f`；v1.1 的在 `cxh_v11\`；都已删。）

---

## 0. 病根 / 动机（行号以 HEAD `4dbd17a` 为准；第 5 条的实测是 v1 起草时在 `485581f` 上做的，FU2 只改注释与测试，不影响它）

1. **codex 接进来了，/add 却列不出它的模型。** `internal/provider/codex.go:110-117` 声明 `ModelDiscovery: ModelDiscoveryUnknown`，`internal/repl/addmodel.go:56-69` 的 `discoverableProviders` 因 `Available()` 为假静默跳过它；`DiscoveryCommandFor`（`internal/provider/discovery.go:253-269`）与 `ParseDiscovery`（`internal/provider/discovery_parse.go:44-55`）都没有 codex 一支，走 `ErrDiscoveryUnsupported`。Griffin 要加 `gpt-6-sol` 只能照 17b §6 手写配置。`addmodel.go:53-55` 的头注释还写着「没有适配器，例如 codex」「#45 不接 codex」，CX1 落地后已过期（17b §7 待主代理第 14 条）。
2. **强度分支把 codex 当成 grok / agy。** `cmdAddDiscovered` 的强度 switch（`addmodel.go:324-345`）第一支 `case m.Provider != "claude":`（`:325`）对一切非 claude 的行拒绝 `--effort`、强度记空串。codex 的强度是每轮 `-c model_reasoning_effort=`（CX1，`codex.go:173-175`），在 council 里选，必须像 claude 一样只给该模型支持的档。
3. **CX1 只按并集校验强度。** `codexEffortLevels`（`codex.go:42-50`）是 13 个模型 `supported_reasoning_levels` 的并集；`gpt-6-luna` + `ultra` 这类组合照样放行（17b §7 待主代理第 5 条）。目录里每个模型有自己的档，识别层可以按模型收窄。
4. **`/help` 看不到新用法。** `internal/repl/commands.go:143` 的 `/add` summary 只有 `/add 列出可加的；/add <全名|编号|前缀>[,...] 一次加多个`（MD1 open item (e)；Griffin 11:30 授权补上）。
5. **`codex debug models` 的实测**（codex-cli 0.147.0，E 盘机，2026-09-28 起草时在隔离目录 `cxh_draft\` 作 cwd 实跑；零花费）：
   - 退出码 0，182 ms，stdout 581,928 字节，**整份是一行**（只有末尾一个 LF），无 BOM，stderr 空。顶层 `{"models":[...]}`，13 条记录；每条的键：`slug, display_name, description, default_reasoning_level, supported_reasoning_levels, shell_type, visibility, supported_in_api, priority, additional_speed_tiers, service_tiers, default_service_tier, availability_nux, upgrade, model_messages, include_*_usage_instructions, ..., base_instructions`（首条缺 `auto_compact_token_limit`）。每条 41-56 KB，其中 `model_messages` 21-36 KB（`instructions_template` 等大字符串）。`supported_reasoning_levels` 每项是 `{effort, description}`。
   - `visibility`：`list` 9 条（`gpt-6-sol`、`gpt-6-astra`、`gpt-6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.5`、`custom/cpa-grok-4.7`、`custom/cpa-gemini-3.8-flash-high`），`hide` 4 条（`gpt-reserve`、`gpt-5.6-sol-1m`、`codex-auto-review`、`custom/qwen3.8-27b`）。
   - `default_reasoning_level` 各不相同：`gpt-6-sol` medium、`gpt-6-astra` low、`gpt-5.5` xhigh、两条 custom high；`supported_reasoning_levels`：`gpt-6-sol` 等 low..ultra 六档，`gpt-6-luna`、`gpt-5.6-luna` 没有 ultra，`gpt-5.5` 到 xhigh 为止，`custom/qwen3.8-27b` 有 minimal，`custom/cpa-gemini-3.8-flash-high` 只有 high。全部 7 条非 custom 的 list 行都有 high。
   - **Council 的真执行器实跑一次**：原型里 `provider.DiscoverModels(ctx, "codex", "codex", turn.RunDiscovery)`。`exec.LookPath("codex")` 在本机（go1.27.0）解析到 `C:\Users\25142\AppData\Roaming\npm\codex.cmd`，退出码 0、166 ms、581,928 字节，解析出 9 行（7 条可选 + 2 条 custom）。**所以 `codex.cmd` 这条路跑 `debug models` 已零花费验过一次**；`codex.cmd` 跑 `exec`（带 `=` 的参数、单独的 `-`、stdin）仍是 CX1 §7 待主代理第 2 条，本刀不涉及。
   - 581,928 字节 < `maxSpawnOutputBytes` = 8,388,608（`internal/turn/spawn.go:19`）。`RunDiscovery` 逐行用 `ReadBytes` 读、按 `maxSpawnOutputBytes` 封顶（`internal/turn/discovery_exec.go:86-107`），没有单行上限，一行 581 KB 不会被截或报错（上面的实跑就是这一路）。
   - **没有验证的**：`debug models` 会不会联网 [未验证]；目录是否反映订阅 [未验证，推断：不反映，它更像 codex 内置加本地配置的模型目录]；`custom/` 行来自 Griffin 自己 codex 配置里的自定义 provider，经 CPA 转发到 grok / gemini [推断：依据是任务书的说明与 display_name 里的「(CPA)」；本契约不读 `~/.codex`]。
6. **既有用例钉着旧行为**（原型实跑，只落 §1.1 A 部分时恰好这 6 条红）：`mdx_discovery_test.go` 的 P1、P2、P8 的 `codex` 子测试、P9（都拿 codex 当「没有识别方式」的例子）；`cxx_codex_test.go` 的 `TestCxxCapabilities`（`want` 不写 `ModelDiscovery`，靠零值等于 Unknown）；`ux2r_add_test.go` 的 `TestUx2rJoinIsAdd`（逐字钉 `/add` 的 summary）。授权改法在 §1.1 B 部分。

---

## 1. 范围

### 1.1 做

**冻结粒度**：下面两段补丁（A 部分生产代码、B 部分授权改动的既有测试）**逐字冻结，含注释**。实现腿原样交出这两段补丁（格式见 §5），不许改任何一个字节：标识符、判据、顺序、字面量、注释都一样。代码块里是真 tab（gofmt 过）；落盘前 `gofmt -l` 必须为空。锚点每个在 `4dbd17a` 真文件里恰好出现一次（v1.1 原型脚本逐个核对过）；FU2 已落地，与它的关系见文件头。

#### 设计裁定（主代理授权范围内，由本契约定；理由写在这里，代码在下面的补丁里）

1. **命令**：`DiscoveryCommandFor("codex", path)` 返回 `{Path: path, Args: ["debug", "models"]}`，`Stdin`、`CloseStdinAfter` 都是 nil（写完空 stdin 就关，与 agy / grok 同）。
2. **过滤**（`parseCodexDiscovery`，顺序冻结）：先 `visibility` 不是 `"list"`（含缺席、大小写不同）就跳过，同 claude 的 disabled 行；再 slug 为空或含控制字符（`hasControlChar`）就跳过。其余的行都返回；slug 以 `custom/` 开头的行 `Custom` 为真。**一行非 custom 的都没有**时返回 `ErrDiscoveryEmpty`（custom 行不算「可选」，与哨兵的定义「形状对，但一行可选的模型都没有」一致）。顶层 JSON 解不开、或没有 `models` 数组（缺席或 null）时返回 `ErrDiscoveryUnparseable`。
3. **custom/ 行怎么处理**：provider 包不丢它们，标 `DiscoveredModel.Custom = true` 返回；repl 的 `takeModelSnapshot` 不给它们编号、不放进快照的行，只数一共几行（`modelSnapshot.custom`）；`printModelSnapshot` 在识别表（或「识别出的模型都已经在配置里了」那一行）之后、失败行之前，**只在数目 > 0 时**打一行带数目的说明（§3 字面量 L3；「都已经在配置里了」那一支由 R8 担保，「失败行之前」由 R9 担保，v1.1 补）。理由：列出来就能经第二条路加出一个其实是 grok / gemini 的「codex 成员」（Griffin 的 grok 与 gemini 各有自己的适配器）；带数目让「先滤 visibility 再认 custom」这个顺序可观测（`custom/qwen3.8-27b` 是 hide，不计入）。**待 Griffin** 可改成「列出并标注」（§7）。
4. **来源标注**：新增 `ModelSourceCodexCatalog`（取值 4，排在 `ModelSourceAgyQuota` 之后），`Label()` 返回 `codex 目录`。**不标「订阅」**：目录是否反映订阅没有验证。
5. **强度**：`Efforts` 取该模型自己的 `supported_reasoning_levels[].effort`，**只收 `codexEffortLevels` 里有的档**（CX1 的适配器只放行这些，识别层列出适配器会拒的档没有意义；大小写敏感，`LOW` 不收），保持模型自己的顺序，一档都没有时是 nil。这收掉了 CX1 open item「只校验并集」在识别层的那一半：`/add +编号 --effort` 只许选该模型的档。手写进配置的成员仍只按并集校验（CX1 不改）。
6. **默认强度与 claude 一致**（复用 `DiscoveredModel.DefaultEffort()`：有 high 取 high，否则取第一档，一档都没有是空串），**不取** `default_reasoning_level`。理由：(a) #45 主代理定的惯例是「默认 high」，任务书自己的例子是 `gpt-6-sol-high`；(b) `default_reasoning_level` 各模型不同（`gpt-6-sol` medium、`gpt-6-astra` low、`gpt-5.5` xhigh），用它会让建议 id 的后缀随模型漂、`gpt-5.5` 默认就是更贵的 xhigh；(c) `SuggestedDisplayName` 「强度为 high 时不写」的规则与默认 high 配套，默认取别的档时每个新成员名字都带档；(d) 真目录里 7 条可选行全有 high，实际生效的只是 high。`gpt-5.5`（默认 xhigh）与 `gpt-6-sol`（默认 medium）都在夹具里，「改用 default_reasoning_level」的写法在 P3 与 R1 上都会变。
7. **建议 id**：`sanitizeModelID(slug)`；与 agy 同一条撞名规则（MD1 已冻结）：规整后若以**另一家已登记 provider 的 id 加 `-`** 开头（今天是 `agy-`、`claude-`、`grok-`），把那个前缀换成 `codex-`；再接 `-<强度>`（强度为空不接）。例：`gpt-6-sol` + high → `gpt-6-sol-high`；`gpt-5.6-sol` + high → `gpt-5.6-sol-high`（带点号，配置表名加引号）；合成的 `claude-cxh-9` → `codex-cxh-9-high`。
8. **display_name**：取目录里的 `display_name`（`GPT-6-Sol`），含控制字符时解析层记空串；为空时退回 `prettyModelName(sanitizeModelID(slug))`；强度不空且不是 high 时后接一个空格和强度（同 claude：`GPT-6-Sol max`）。不加 `Codex ` 前缀（agy 加前缀是因为它经 agy 跑别家的模型；codex 的行是 codex 自己的模型）。17b §6 手写示例用的是 `GPT-6 Sol`，识别路径取 CLI 原名，display_name 进指纹，所以定死（**待 Griffin** 可改，§7）。
9. **Alias 留空**：codex 配置的 model 写的就是 slug（CX1 argv `-m <model>`），没有第二种写法。「已在配置里」按 provider + model 判（MD1 既有判据，不改）。
10. **提示行**：表后那一行在**可见行里有 codex 行时**写成「claude 与 codex 可加 --effort」，否则与 MD1 的原句逐字相同（既有 5 个金标准块因此一个字不用改，它们就是这一条的反面对照）。
11. **`/add` 的 summary**：见 §3 L4。「+编号」照提示行写法（`/add +编号`），不用 `msgAddModelUsage` 的 `+<编号>`：summary 是说明，照 `用 /add +编号 加入` 那一句的读法；`[--id <新 id>] [--effort <强度>]` 与 `msgAddModelUsage` 逐字相同。第一句从「把配置里的 agent 加进当前会话」改成「把 agent 加进当前会话」，因为 `/add +编号` 加的不是配置里已有的 agent。`/join` 的 summary 维持「同 /add」：`/join` 与 `/add` 是同一个 `cmdAdd`（TestUx2rJoinIsAdd 钉着），`/join +3`、`/join --refresh` 行为相同，这句话仍然成立。
12. **Griffin 的配置**：只有写了 `[providers.codex]` 才进识别（`discoverableProviders` 的既有规则，不改）。他的配置今天没有这一段，要加哪一行见 §6 第 5 步（主代理按授权带备份追加，不在本刀代码里做）。

#### A 部分：生产代码（6 个文件；原型 +170 / -33 行，含注释；W1b 修订后实测）

| # | 文件 | 位置（`4dbd17a`） | 改动 |
|---|---|---|---|
| A1 | `internal/provider/discovery.go` | `:24` 常量注释；`:37` `ModelSource` 注释；`:47-48` 常量块末尾；`:50` `Label` 注释；`:57-58` `Label` 的 AgyQuota 一支；`:66-74` `DiscoveredModel` 注释与结构体；`:93-94` `SuggestedID` 头注释末行；`:113-116` `SuggestedID` 的 default 一支之前；`:120` `SuggestedDisplayName` 头注释；`:129-131` `SuggestedDisplayName` 的 agy 一支之后；`:251` `DiscoveryCommandFor` 头注释；`:264-265` grok 一支之后 | `ModelSourceCodexCatalog` 与标签；`Custom` 字段；codex 的建议 id / 显示名；codex 的识别命令 |
| A2 | `internal/provider/discovery_parse.go` | `:13-14` 文件头；`:50-51` `ParseDiscovery` 的 grok 一支之后；`:192` `hasControlChar` 注释之前 | `parseCodexDiscovery` 与它的类型、常量 |
| A3 | `internal/provider/codex.go` | `:50` `codexEffortLevels` 上方补注释；`:110-117`；`:227-229` `codexPermissionArgs` 注释；`:277-279` `ParseTurn` 注释；`:385-388` `codexUsage` 注释 | `ModelDiscovery: ModelDiscoverySubcommand`；三处注释订正（v1.1 顺带并进，只改注释） |
| A4 | `internal/repl/addmodel.go` | `:53-55`；`:37-39`；`:134-135`；`:200-207`；`:325` | custom 计数与说明行；提示行二选一；强度 switch 放行 codex |
| A5 | `internal/repl/commands.go` | `:143` | `/add` 的 summary |
| A6 | `internal/provider/turn.go` | `:92-94` `Usage` 注释；`:173` `UsageRaw` 字段注释末行 | 只改注释：usage_raw 只有成功轮进（v1.1 顺带并进） |

冻结补丁（逐字；锚点 `<<<<ANCHOR` 与 `====` 之间，替换文本 `====` 与 `>>>>` 之间）：

```text cxh:patch-prod
===PATCH: internal/provider/discovery.go===
<<<<ANCHOR
	// ModelDiscoverySubcommand 是子命令列出模型。agy 跑 agy --output-format json models，grok 跑 grok models。
	ModelDiscoverySubcommand
====
	// ModelDiscoverySubcommand 是子命令列出模型。agy 跑 agy --output-format json models，grok 跑 grok models，codex 跑 codex debug models。
	ModelDiscoverySubcommand
>>>>
<<<<ANCHOR
// ModelSource 是识别结果每一行的来源标注。三种非零取值的来由：claude 的行，以及 agy 清单里 label 以 "Gemini " 开头的行，是订阅；grok 的行只反映本机 grok 配置里手写的模型，不反映订阅；agy 清单里的 Claude 行与 GPT-OSS 行走的是 agy 的额度（#45 要求显示并标注）。判「Gemini 行」的依据是 label 以 Gemini 加一个空格开头，判据本身在 discovery_parse.go。
type ModelSource int
====
// ModelSource 是识别结果每一行的来源标注。四种非零取值的来由：claude 的行，以及 agy 清单里 label 以 "Gemini " 开头的行，是订阅；grok 的行只反映本机 grok 配置里手写的模型，不反映订阅；agy 清单里的 Claude 行与 GPT-OSS 行走的是 agy 的额度（#45 要求显示并标注）；codex 的行来自 codex debug models 列出的模型目录，它反不反映订阅没有验证，所以不标订阅。判「Gemini 行」的依据是 label 以 Gemini 加一个空格开头，判据本身在 discovery_parse.go。
type ModelSource int
>>>>
<<<<ANCHOR
	ModelSourceAgyQuota
)
====
	ModelSourceAgyQuota
	// ModelSourceCodexCatalog 是 codex debug models 列出的模型目录。屏幕标注「codex 目录」。
	ModelSourceCodexCatalog
)
>>>>
<<<<ANCHOR
// Label 是屏幕上的来源标注。三种非零取值分别是「订阅」「本地配置」「走 agy 额度」；零值与任何未列出的取值都是「-」。
====
// Label 是屏幕上的来源标注。四种非零取值分别是「订阅」「本地配置」「走 agy 额度」「codex 目录」；零值与任何未列出的取值都是「-」。
>>>>
<<<<ANCHOR
	case ModelSourceAgyQuota:
		return "走 agy 额度"
====
	case ModelSourceAgyQuota:
		return "走 agy 额度"
	case ModelSourceCodexCatalog:
		return "codex 目录"
>>>>
<<<<ANCHOR
// Model 是要写进配置 model 字段的值。Alias 是同一模型的另一种写法（claude 的 value、agy 的 id），只用来判「已在配置里」，不写进配置。Efforts 只有 claude 有；JSON 里这个键缺席时是 nil，不是空切片。
type DiscoveredModel struct {
	Provider string
	Model    string
	Alias    string
	Name     string
	Efforts  []string
	Source   ModelSource
}
====
// Model 是要写进配置 model 字段的值。Alias 是同一模型的另一种写法（claude 的 value、agy 的 id），只用来判「已在配置里」，不写进配置。Efforts 只有 claude 与 codex 有；claude 的 JSON 里这个键缺席时是 nil，codex 一档都没有时是 nil。
//
// Custom 只有 codex 的行可能为真：slug 以 custom/ 开头，是用户在自己的 codex 配置里自定义的模型（Griffin 的是经 CPA 转发的 grok 与 gemini），列出来就能经第二条路加出一个其实是别家模型的 codex 成员。repl 不给这种行编号、不显示，只数一共几行。
type DiscoveredModel struct {
	Provider string
	Model    string
	Alias    string
	Name     string
	Efforts  []string
	Source   ModelSource
	Custom   bool
}
>>>>
<<<<ANCHOR
		return id
	default:
		return sanitizeModelID(m.Model)
	}
}
====
		return id
	case "codex":
		id := sanitizeModelID(m.Model)
		for _, other := range IDs() {
			if other == "codex" {
				continue
			}
			if strings.HasPrefix(id, other+"-") {
				id = "codex-" + strings.TrimPrefix(id, other+"-")
				break
			}
		}
		if effort != "" {
			id += "-" + sanitizeModelID(effort)
		}
		return id
	default:
		return sanitizeModelID(m.Model)
	}
}
>>>>
<<<<ANCHOR
	case "agy":
		return "Agy " + m.Name
	default:
====
	case "agy":
		return "Agy " + m.Name
	case "codex":
		name := m.Name
		if name == "" {
			name = prettyModelName(sanitizeModelID(m.Model))
		}
		if effort != "" && effort != "high" {
			name += " " + effort
		}
		return name
	default:
>>>>
<<<<ANCHOR
	case "grok":
		return DiscoveryCommand{Path: path, Args: []string{"models"}}, nil
====
	case "grok":
		return DiscoveryCommand{Path: path, Args: []string{"models"}}, nil
	case "codex":
		return DiscoveryCommand{Path: path, Args: []string{"debug", "models"}}, nil
>>>>
<<<<ANCHOR
// grok：把名字按同一套规则规整成 id。
func (m DiscoveredModel) SuggestedID(effort string) string {
====
// grok：把名字按同一套规则规整成 id。
// codex：把 slug 按同一套规则规整成 id；与 agy 同一条撞名规则，规整后若以另一家已登记 provider 的 id 加 - 开头，就把那个前缀换成 codex-；再接 -<强度>（强度为空时不接）。gpt-6-sol 以 high 加入得到 gpt-6-sol-high。
func (m DiscoveredModel) SuggestedID(effort string) string {
>>>>
<<<<ANCHOR
// claude：由 id 的模型段转成词首大写的名字；强度不是 high、也不空时，在后面加一个空格和强度（与现有的 Opus 5.0、Opus 5.0 max 写法一致）。agy：Agy 加一个空格再加 label。grok：由规整后的名字转成词首大写。
====
// claude：由 id 的模型段转成词首大写的名字；强度不是 high、也不空时，在后面加一个空格和强度（与现有的 Opus 5.0、Opus 5.0 max 写法一致）。agy：Agy 加一个空格再加 label。grok：由规整后的名字转成词首大写。codex：取目录里的 display_name（例如 GPT-6-Sol），为空时由规整后的 slug 转成词首大写；强度的写法同 claude。
>>>>
<<<<ANCHOR
// agy 跑 --output-format json models，写完空 stdin 就关。grok 跑 models，同样写完就关。
====
// agy 跑 --output-format json models，写完空 stdin 就关。grok 跑 models，同样写完就关。codex 跑 debug models，同样写完就关。
>>>>
===END PATCH===
===PATCH: internal/provider/discovery_parse.go===
<<<<ANCHOR
// 每块在 400 行上下，代码没有改。本文件是 DiscoverModels、ParseDiscovery、三家的
// 解析器与 hasControlChar。
====
// 每块在 400 行上下，代码没有改。本文件是 DiscoverModels、ParseDiscovery、各家的
// 解析器与 hasControlChar（codex 的解析器由 CXH 加）。
>>>>
<<<<ANCHOR
	case "grok":
		return parseGrokDiscovery(stdout)
====
	case "grok":
		return parseGrokDiscovery(stdout)
	case "codex":
		return parseCodexDiscovery(stdout)
>>>>
<<<<ANCHOR
// hasControlChar 报告 s 里有没有码位小于 0x20 的字符或 0x7f。三家解析器说的控制字符就是这一处。
====
// codexCustomSlugPrefix 是用户在自己的 codex 配置里自定义转发的模型的 slug 前缀。
const codexCustomSlugPrefix = "custom/"

// codexCatalogModel 是 codex debug models 输出里 models 数组的一行，只读四个键；其余的键（含很大的 model_messages）一律不解。
type codexCatalogModel struct {
	Slug                     string `json:"slug"`
	DisplayName              string `json:"display_name"`
	Visibility               string `json:"visibility"`
	SupportedReasoningLevels []struct {
		Effort string `json:"effort"`
	} `json:"supported_reasoning_levels"`
}

// parseCodexDiscovery 只读顶层 models 数组（codex debug models 的 stdout 是一行 JSON）。
//
// 顶层解不开、或没有 models 数组（缺席或 null），返回 ErrDiscoveryUnparseable。
// visibility 不是 list 的行跳过（[推断] codex 自己的选择器也不列它们；同 claude
// 的 disabled 行）；slug 为空或 hasControlChar 为真的行跳过。强度只收
// codexEffortLevels 里有的档，保持该模型自己的顺序，一档都没有时是 nil；
// display_name 含控制字符时记空串，建议名退回由 slug 生成。slug 以 custom/
// 开头的行照样返回、Custom 为真（repl 只数它们）。一行非 custom 的都没有时返回
// ErrDiscoveryEmpty。Model 取 slug，Alias 留空（codex 配置的 model 写的就是 slug）。
func parseCodexDiscovery(stdout []byte) ([]DiscoveredModel, error) {
	var v struct {
		Models *[]codexCatalogModel `json:"models"`
	}
	if err := json.Unmarshal(stdout, &v); err != nil {
		return nil, fmt.Errorf("codex: stdout is not JSON: %w", ErrDiscoveryUnparseable)
	}
	if v.Models == nil {
		return nil, fmt.Errorf("codex: no models array: %w", ErrDiscoveryUnparseable)
	}
	var out []DiscoveredModel
	selectable := 0
	for _, m := range *v.Models {
		if m.Visibility != "list" || m.Slug == "" || hasControlChar(m.Slug) {
			continue
		}
		var efforts []string
		for _, l := range m.SupportedReasoningLevels {
			if codexEffortAllowed(l.Effort) {
				efforts = append(efforts, l.Effort)
			}
		}
		name := m.DisplayName
		if hasControlChar(name) {
			name = ""
		}
		custom := strings.HasPrefix(m.Slug, codexCustomSlugPrefix)
		if !custom {
			selectable++
		}
		out = append(out, DiscoveredModel{
			Provider: "codex",
			Model:    m.Slug,
			Name:     name,
			Efforts:  efforts,
			Source:   ModelSourceCodexCatalog,
			Custom:   custom,
		})
	}
	if selectable == 0 {
		return nil, fmt.Errorf("codex: %w", ErrDiscoveryEmpty)
	}
	return out, nil
}

// hasControlChar 报告 s 里有没有码位小于 0x20 的字符或 0x7f。四家解析器说的控制字符就是这一处。
>>>>
===END PATCH===
===PATCH: internal/provider/codex.go===
<<<<ANCHOR
var codexEffortLevels = []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"}
====
//
// 识别那一层（discovery_parse.go 的 parseCodexDiscovery）按模型收窄：/add +编号
// 只给该模型自己 supported_reasoning_levels 里、同时在本清单里的档。手写进配置的
// 成员不经识别，仍只按本清单校验。
var codexEffortLevels = []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"}
>>>>
<<<<ANCHOR
		// ModelDiscovery = Unknown：codex debug models 能列出模型目录
		// （2026-09-28 实测，JSON 含 slug / display_name /
		// supported_reasoning_levels / visibility），但识别框架还没接它
		// （#45 裁「接入 codex」在两个功能之后，识别是再下一刀）。在接上之前
		// 声明 Unknown，Available() 为假，discoverableProviders 跳过它，/add
		// 不会多出一行 codex 的失败。不许声明成 ModelDiscoverySubcommand：
		// DiscoveryCommandFor / ParseDiscovery 不认 codex，/add 会多一行「识别失败」。
		ModelDiscovery: ModelDiscoveryUnknown,
====
		// ModelDiscovery = Subcommand：codex debug models（零花费；codex-cli
		// 0.147.0 上 2026-09-28 实测约 0.2 秒、stdout 是一行约 581 KB 的 JSON，
		// 在 maxSpawnOutputBytes 之内）。命令与解析在 discovery.go 的
		// DiscoveryCommandFor 与 discovery_parse.go 的 parseCodexDiscovery。
		// 它列的是 codex 的模型目录（是否联网、是否反映订阅都没有验证），
		// 来源标注是「codex 目录」，不是「订阅」。
		ModelDiscovery: ModelDiscoverySubcommand,
>>>>
<<<<ANCHOR
// workspace 不给：workspace-write 作为写入边界在 exec / exec resume 上没测过
// （codex sandbox 上的零花费结果见上一段）。default 分支正常走不到（validateRequest 先拦），与 grok /
// claude 同一体例的防御写法。
====
// workspace 不给：workspace-write 作为写入边界在 exec / exec resume 上没测过
// （codex sandbox 上的零花费结果见本注释「Windows 上的 read-only」那一段）。
// default 分支正常走不到（validateRequest 先拦），与 grok / claude 同一体例的
// 防御写法。
>>>>
<<<<ANCHOR
// ParseTurn 在失败路径上也带回 thread_id 与 UsageRaw（钱可能花了）；这两样里
// runner 今天只吸收 thread_id（runner.go 的 absorbParse 不碰 UsageRaw），失败轮
// 的 UsageRaw 被丢弃，只有成功轮经 agentMeta 进 meta.usage_raw。
====
// ParseTurn 在 turn.failed / error 两种失败结局上（ErrTurnFailed）也带回
// thread_id 与 UsageRaw（钱可能花了）；成功形状但 thread_id 为空时
// （ErrNoSessionID）带回 UsageRaw；认不出的形状（ErrUnparseableOutput）返回
// 零值，两样都没有。UsageRaw 只来自那一条 turn.completed（它的 usage 键缺席
// 或为 null 时也为空），没有 turn.completed 的失败轮它为空。这两样里 runner
// 今天只吸收 thread_id（runner.go 的 absorbParse 不碰 UsageRaw），失败轮的
// UsageRaw 被丢弃，只有成功轮经 agentMeta 进 meta.usage_raw。
>>>>
<<<<ANCHOR
// 时假设对错都只是「不知道」。ParseTurn 在失败路径上也带回 thread_id 与 UsageRaw，
// 但 runner 今天只吸收 thread_id，失败轮的 UsageRaw 被丢弃，只有成功轮的原始对象进
// messages.meta["usage_raw"]，将来可以回算。不许把 usage 里的任何数字填进
// Usage 的 token 字段。
====
// 时假设对错都只是「不知道」。ParseTurn 在一部分失败结局上也带回 thread_id
// 或 UsageRaw（哪几种、各带回什么见 ParseTurn 的注释；UsageRaw 只来自那一条
// turn.completed，没有 turn.completed 的失败轮它为空），但 runner 今天只吸收
// thread_id，失败轮的 UsageRaw 被丢弃，只有成功轮的原始对象进
// messages.meta["usage_raw"]，将来可以回算。不许把 usage 里的任何数字填进
// Usage 的 token 字段。
>>>>
===END PATCH===
===PATCH: internal/provider/turn.go===
<<<<ANCHOR
// 一处定义。三家的原始 usage 对象整个存进 messages.meta["usage_raw"]，将来改口径
// 可回算。跨 provider 比较之前，先看 Meta["usage_raw"] 与各自的 UsageFormula——
// 这一列的数字来自三条不同的算法，不是同一个数量级的噪声。
====
// 一处定义。只有成功轮的原始 usage 对象整个存进 messages.meta["usage_raw"]
// （失败轮也可能带回 UsageRaw，但 runner.go 的 absorbParse 不吸收它），将来改
// 口径可回算。跨 provider 比较之前，先看 Meta["usage_raw"] 与各自的
// UsageFormula——这一列的数字来自三条不同的算法，不是同一个数量级的噪声。
>>>>
<<<<ANCHOR
	// 那条承诺就是空头的 —— 编排层手里只有归一化结果，没别的可写。
====
	// 那条承诺就是空头的 —— 编排层手里只有归一化结果，没别的可写。今天兑现
	// 的只有成功轮：失败轮也可能带回 UsageRaw，但 runner.go 的 absorbParse 不
	// 吸收它，只有成功轮经 agentMeta 进 meta.usage_raw。
>>>>
===END PATCH===
===PATCH: internal/repl/addmodel.go===
<<<<ANCHOR
// discoverableProviders 只看配置里写了的 provider，按字典序。
// provider.Lookup 找不到（没有适配器，例如 codex）或 ModelDiscovery 不可用的，
// 一律静默跳过（#45 不接 codex）。
====
// discoverableProviders 只看配置里写了的 provider，按字典序。
// provider.Lookup 找不到（没有适配器）或 ModelDiscovery 不可用的，一律静默
// 跳过。四家已登记的适配器今天都声明了能跑的识别方式。
>>>>
<<<<ANCHOR
	failures []snapFailure
	okCount  int
}
====
	failures []snapFailure
	okCount  int
	custom   int
}
>>>>
<<<<ANCHOR
		for _, m := range models {
			row := snapRow{m: m}
====
		for _, m := range models {
			// custom/ 开头的 codex 行不编号、不进快照的行，只计数（printModelSnapshot 表后说明）。
			if m.Custom {
				snap.custom++
				continue
			}
			row := snapRow{m: m}
>>>>
<<<<ANCHOR
		if err := r.printf("用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档\n"); err != nil {
			return err
		}
	} else if snap.okCount > 0 {
		if err := r.printf("识别出的模型都已经在配置里了\n"); err != nil {
			return err
		}
	}
====
		// 可见行里有 codex 的行时，提示里带上 codex；没有时与 MD1 的原句逐字相同。
		who := "claude"
		for _, row := range visible {
			if row.m.Provider == "codex" {
				who = "claude 与 codex"
				break
			}
		}
		if err := r.printf("用 /add +编号 加入；--id <新 id> 改名；%s 可加 --effort <强度> 换档，EFFORTS 列是它支持的档\n", who); err != nil {
			return err
		}
	} else if snap.okCount > 0 {
		if err := r.printf("识别出的模型都已经在配置里了\n"); err != nil {
			return err
		}
	}
	// 只在确实跳过了 custom/ 行时说明一句，带数量。
	if snap.custom > 0 {
		if err := r.printf("codex 目录里还有 %d 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员\n", snap.custom); err != nil {
			return err
		}
	}
>>>>
<<<<ANCHOR
	case m.Provider != "claude":
====
	// claude 与 codex 的强度在 council 里选，只给该模型自己支持的档（codex 的档已由识别层按模型收窄）。
	case m.Provider != "claude" && m.Provider != "codex":
>>>>
===END PATCH===
===PATCH: internal/repl/commands.go===
<<<<ANCHOR
		{name: "add", summary: "把配置里的 agent 加进当前会话: /add 列出可加的；/add <全名|编号|前缀>[,...] 一次加多个", needsSvc: true, needsConv: true, run: cmdAdd},
====
		{name: "add", summary: "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别", needsSvc: true, needsConv: true, run: cmdAdd},
>>>>
===END PATCH===
```

#### B 部分：授权改动的既有测试（3 个文件：6 条用例的期望，加 1 处文件头注释；实现腿与 A 部分一起交出）

逐处（改前改后就是下面补丁的锚点与替换文本）：

| # | 文件:用例 | 为什么红（原型实跑，只落 A 部分时） | 授权的改法 |
|---|---|---|---|
| B1 | `internal/provider/mdx_discovery_test.go:206` `TestMdxCapabilitiesDeclareModelDiscovery`（P1） | `:234-236` 断言 codex 为 Unknown，实际 1（Subcommand） | `:211` want 表与 `:228-236` 单独那一条改成 Subcommand；`:203-204` 注释跟着改 |
| B2 | 同文件 `:267` `TestMdxDiscoveryCommandFor`（P2） | `:313-315` 断言 `DiscoveryCommandFor("codex")` 是 Unsupported，实际返回了命令 | 例子从 `codex` 换成没有登记的 `nosuch`（这条断言要钉的是 default 分支，今天四家已登记的 provider 都有识别方式，只有未登记的 id 走得到它） |
| B3 | 同文件 `:466` `TestMdxParseRejects/codex`（P8） | `ParseDiscovery("codex", "x")` 现在是 Unparseable | 子测试改名 `nosuch`，id 换成 `nosuch`，期望不变 |
| B4 | 同文件 `:531` `TestMdxDiscoverModelsMapsExecErrors`（P9） | `DiscoverModels("codex", 喂 grok 夹具)` 现在调了执行器一次、返回 Unparseable | id 换成 `nosuch`，期望（Unsupported、执行器 0 次）不变；`:530` 注释跟着改 |
| B5 | `internal/provider/cxx_codex_test.go:130` `TestCxxCapabilities`（CX1 P1） | `want` 不写 `ModelDiscovery`（零值 Unknown），整值 DeepEqual 不等 | `want` 加 `ModelDiscovery: provider.ModelDiscoverySubcommand,`；`:127-129` 注释改 |
| B6 | `internal/repl/ux2r_add_test.go:411` `TestUx2rJoinIsAdd` | `:448` 的 `wantAddSummary` 逐字钉旧 summary | 换成 §3 L4 的新 summary；`/join` 的「同 /add」断言（`:452`）不动 |
| B7 | `internal/provider/cxx_codex_test.go:8-9` 文件头注释（v1.1 顺带并进） | 不红：只是注释还写着失败夹具按「codex exec 的事件定义」合成、未实测，与 FU2 订正后的 `codex.go` 说法不一致 | 改成「都是合成的：失败形状是推断，本机零观测」；裁决授权改这一个既有测试文件的这一处注释，代码一个字节不动 |

B 部分之外的既有测试**一个字节都不许改**。原型实跑：A + B 之后全量九包全绿，没有第 7 条变红（B7 只改注释，本来就不红）。

```text cxh:patch-tests
===PATCH: internal/provider/mdx_discovery_test.go===
<<<<ANCHOR
// CX1 登记了 codex，声明 Unknown（识别框架还没接它，理由见 codex.go）；
// codex.go 写成 Subcommand（CX1 X58）时 codex 那一条不等。
====
// CX1 登记了 codex；CXH 把它接进识别框架，声明 Subcommand（codex debug models）；
// codex.go 退回 Unknown 时 codex 那一条不等。
>>>>
<<<<ANCHOR
		"codex":  provider.ModelDiscoveryUnknown,
====
		"codex":  provider.ModelDiscoverySubcommand,
>>>>
<<<<ANCHOR
	// CX1：codex 声明 Unknown。want 表里的 codex 一项只让上面的 Fatalf 放行，
	// 取值由下面这一条钉住（与上面三家的循环分开写，那三条一字不动）。
	pc, err := provider.Lookup("codex")
	if err != nil {
		t.Fatalf("Lookup(%q): %v", "codex", err)
	}
	if got := pc.Capabilities().ModelDiscovery; got != provider.ModelDiscoveryUnknown {
		t.Errorf("codex 的 ModelDiscovery = %d，期望 %d", got, provider.ModelDiscoveryUnknown)
	}
====
	// CXH：codex 声明 Subcommand。want 表里的 codex 一项只让上面的 Fatalf 放行，
	// 取值由下面这一条钉住（与上面三家的循环分开写，那三条一字不动）。
	pc, err := provider.Lookup("codex")
	if err != nil {
		t.Fatalf("Lookup(%q): %v", "codex", err)
	}
	if got := pc.Capabilities().ModelDiscovery; got != provider.ModelDiscoverySubcommand {
		t.Errorf("codex 的 ModelDiscovery = %d，期望 %d", got, provider.ModelDiscoverySubcommand)
	}
>>>>
<<<<ANCHOR
	if _, err := provider.DiscoveryCommandFor("codex", "P"); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Errorf("codex: err = %v，期望 ErrDiscoveryUnsupported", err)
	}
====
	if _, err := provider.DiscoveryCommandFor("nosuch", "P"); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Errorf("nosuch: err = %v，期望 ErrDiscoveryUnsupported", err)
	}
>>>>
<<<<ANCHOR
	t.Run("codex", func(t *testing.T) {
		got, err := provider.ParseDiscovery("codex", []byte("x"))
		if len(got) != 0 || !errors.Is(err, provider.ErrDiscoveryUnsupported) {
			t.Errorf("codex: got=%#v err=%v，期望空结果与 ErrDiscoveryUnsupported", got, err)
		}
	})
====
	t.Run("nosuch", func(t *testing.T) {
		got, err := provider.ParseDiscovery("nosuch", []byte("x"))
		if len(got) != 0 || !errors.Is(err, provider.ErrDiscoveryUnsupported) {
			t.Errorf("nosuch: got=%#v err=%v，期望空结果与 ErrDiscoveryUnsupported", got, err)
		}
	})
>>>>
<<<<ANCHOR
// codex 没有识别方式，执行器一次都不许被调。
====
// 没有登记的 provider（nosuch）没有识别方式，执行器一次都不许被调。
>>>>
<<<<ANCHOR
	if _, err := provider.DiscoverModels(ctx, "codex", "P", countExec); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Errorf("codex: err = %v，期望 ErrDiscoveryUnsupported", err)
	}
	if calls != 0 {
		t.Errorf("codex: 执行器被调了 %d 次，期望 0 次", calls)
	}
====
	if _, err := provider.DiscoverModels(ctx, "nosuch", "P", countExec); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Errorf("nosuch: err = %v，期望 ErrDiscoveryUnsupported", err)
	}
	if calls != 0 {
		t.Errorf("nosuch: 执行器被调了 %d 次，期望 0 次", calls)
	}
>>>>
===END PATCH===
===PATCH: internal/provider/cxx_codex_test.go===
<<<<ANCHOR
// 失败形状没有真机原件，P6 / P7 里凡用到它们的输入都是按 codex exec 的事件定义
// 合成的，未实测。
====
// 失败形状没有真机原件，P6 / P7 里凡用到它们的输入都是合成的：失败形状是推断，
// 本机零观测。
>>>>
<<<<ANCHOR
// 【判别力】任一字段改一档都不等（X20-X27）。want 不写 ModelDiscovery，
// 它的零值就是 ModelDiscoveryUnknown，与 D1 声明的值相等；codex.go 写成
// ModelDiscoverySubcommand（X58）时这一项不等，照样红。
====
// 【判别力】任一字段改一档都不等（X20-X27）。ModelDiscovery 由 CXH 改成
// ModelDiscoverySubcommand，want 显式写出；codex.go 退回 Unknown 时这一项不等。
>>>>
<<<<ANCHOR
		WriteBoundary:        provider.SupportUnknown,
	}
====
		WriteBoundary:        provider.SupportUnknown,
		ModelDiscovery:       provider.ModelDiscoverySubcommand,
	}
>>>>
===END PATCH===
===PATCH: internal/repl/ux2r_add_test.go===
<<<<ANCHOR
	const wantAddSummary = "把配置里的 agent 加进当前会话: /add 列出可加的；/add <全名|编号|前缀>[,...] 一次加多个"
====
	const wantAddSummary = "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别"
>>>>
===END PATCH===
```

### 1.2 不做（两条腿都不许越界）

- 不改 codex 的会话、解析、权限、隔离参数、`codexEffortLevels` 的取值与 `BuildTurn` 的并集校验（CX1 已冻结）。A3 的代码改动只有 `ModelDiscovery` 一个字段；其余都是注释：`codexEffortLevels` 上方补一段，`codexPermissionArgs`、`ParseTurn`、`codexUsage` 三处注释订正（v1.1），这三个函数的函数体一个字节不动。
- 不接 codex 的用量口径；不改失败轮 `usage_raw` 不落库的现状（A3、A6 只把注释写准）；不碰 `internal/turn` 的任何生产文件（`RunDiscovery` 原样够用，§0 第 5 条实跑过）。A6 的 `turn.go` 在 `internal/provider`，只改注释。
- 不改 claude / agy / grok 的识别命令、解析器与来源标注；不改 `DefaultEffort`、`sanitizeModelID`、`prettyModelName`、`configuredAgentFor`、`sameBrainAgent`、`printDiscoveredModels`、`discoveryFailureText`、`msgAddModelUsage`、`config.AppendBlock` / `AgentBlockTOML`。
- 不改 `/join` 的 summary；不改 MD1 的任何既有字面量（提示行原句在「可见行里没有 codex」时逐字不变）。
- 不碰 Griffin 的 `config.toml` 与他的 codex 配置（§6 第 5 步由主代理在落地后按授权做）。
- 测试腿：只新建 §2 列的 5 个文件；不改任何既有 `_test.go`（B 部分由实现腿交）；不新增 `TestMain`（turn 包复用 MD1 的 `TestMain` 与 `MDX_FAKE_DISCOVERY=readall`，repl 包复用 MD1 的装置）；不起真 codex / 任何真 provider CLI。

---

## 2. 验收（测试腿新写的文件）

通用纪律：
- 新文件的包级标识符一律 `cxh` 开头，测试函数 `TestCxh...`。
- 经真实路径：provider 用例只调导出标识符（`package provider_test`）；repl 用例走真 `dispatch`、真 SQLite、真配置文件、真 `turn.Runner`（MD1 的 `mdxNewRig`）；turn 用例走真 `RunDiscovery` 起测试二进制自己。**外部命令一律经可注入的执行器（repl 的 `r.discover`，已由 `mdxNewRig` 换成 `mdxExec`）或测试二进制扮演的假 CLI，用 §2.0 的夹具字节，不许起真 codex。**
- 屏幕输出一律整行相等（`mdxWantLines` / `mdxHasLine`），不写子串断言；前提守卫（夹具字节数、`mdxReplaceOnce` 恰好一处）例外。
- 复用的 MD1 装置（`internal/repl/mdx_addmodel_fixture_test.go`，HEAD 既有，只读不改）：`mdxNewRig`、`mdxRig` 的 `run` / `afterCandidates` / `readFile` / `bakNames` / `newBak` / `joinedLines` / `exec`（`set` / `count` / `callsCopy`）/ `r`、`mdxResp`、`mdxWantLines`、`mdxSplit`、`mdxReplaceOnce`、`mdxHasLine`、`mdxGrokPath`、`mdxGrokFixture`（假执行器按 `mdx-grok.exe` 默认回它）。`mdxNewRig` 把 `<dataDir>` 换成真路径、设 `COUNCIL_FAKE_CLI=1`、先 `/use` 到新会话；`afterCandidates` 取配置候选表之后的行。

### 2.0 共用夹具（冻结，逐字节；两个包各放一份）

**夹具块 F1**（`internal/provider/cxh_fixture_test.go` 与 `internal/repl/cxh_fixture_test.go` 都逐字包含它；前者 `package provider_test`，后者 `package repl`；两者都要 import `"strings"`）。`cxhCodexFixture()` 共 24,916 字节，`cxhBigModelMessages()` 22,000 字节；真输出裁剪的部分与合成的部分在注释里分开写了。第 1 条的 `model_messages` 是合成内容（键形状照真输出，不放 codex 真实的指令文本），足以证明解析器只读顶层四个键、不怕 20 KB 以上的嵌套值与里面的诱饵键。**注意**：原始字符串里的 `\"`、`\n`、`\u0007`、`\u001b` 是 JSON 转义（反斜杠加字符），不是 Go 转义；编辑链路不许把它们还原成真字符（仓库踩过，见 MD1 的教训）。

```go cxh:fixture-shared
// cxhBigModelMessages 生成 gpt-6-sol 那一行 model_messages 的值：合成内容（不是 codex 的真实指令文本），
// 键形状照真输出；22000 字节，含 JSON 转义的引号与换行、花括号方括号、中文，
// 以及一个嵌套的诱饵对象（带 slug / visibility / supported_reasoning_levels 三个键）。
func cxhBigModelMessages() string {
	chunk := `line with \"quotes\", braces {}, brackets [], a key \"slug\":\"cxh-decoy\" and 中文\n`
	return `{"instructions_template":"` + strings.Repeat(chunk, 250) + `","instructions_variables":null,"approvals":{"on_request":null,"never":null},"decoy":{"slug":"cxh-decoy","visibility":"list","supported_reasoning_levels":[{"effort":"low"}]},"token_budget":{"reminder_threshold_tokens":6144}}`
}

// cxhCodexHead 是第 1 条记录（gpt-6-sol，真输出裁剪）到 model_messages 键为止。
const cxhCodexHead = `{"models":[{"slug":"gpt-6-sol","display_name":"GPT-6-Sol","description":"Workhorse model for coding and everyday work.","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low","description":"Fast responses with lighter reasoning"},{"effort":"medium","description":"Balances speed and reasoning depth for everyday tasks"},{"effort":"high","description":"Greater reasoning depth for complex problems"},{"effort":"xhigh","description":"Extra high reasoning depth for complex problems"},{"effort":"max","description":"Maximum reasoning depth for the hardest problems"},{"effort":"ultra","description":"Maximum reasoning with automatic task delegation"}],"visibility":"list","supported_in_api":true,"priority":0,"model_messages":`

// 两条 visibility 为 list 的 custom/ 记录（真输出裁剪），R2 要单独删掉它们。
const cxhRecGrokCustom = `{"slug":"custom/cpa-grok-4.7","display_name":"Grok 4.7 (CPA)","default_reasoning_level":"high","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},`
const cxhRecGeminiCustom = `{"slug":"custom/cpa-gemini-3.8-flash-high","display_name":"Gemini 3.8 Flash High (CPA)","default_reasoning_level":"high","supported_reasoning_levels":[{"effort":"high"}],"visibility":"list"},`

// cxhCodexTail 从 gpt-6-sol 的 model_messages 之后接到文件末尾：
// 真输出裁剪的 gpt-6-luna、gpt-reserve（hide）、gpt-5.6-sol、gpt-5.5、两条 custom/（list）、
// custom/qwen3.8-27b（hide，display_name 换成了合成值）；其后全是合成记录：
// claude-cxh-9（display_name 空、强度含 turbo 与 LOW）、cxh-bell（display_name 含 BEL、没有强度键）、
// cxh-min（只有 minimal 与 low）、customer-cxh（custom 没有斜杠）、slug 含 ESC、slug 空、没有 visibility 键。
const cxhCodexTail = `,"context_window":272000},` +
	`{"slug":"gpt-6-luna","display_name":"GPT-6-Luna","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"}],"visibility":"list"},` +
	`{"slug":"gpt-reserve","display_name":"GPT-Reserve","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"}],"visibility":"hide"},` +
	`{"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}],"visibility":"list"},` +
	`{"slug":"gpt-5.5","display_name":"GPT-5.5","default_reasoning_level":"xhigh","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},` +
	cxhRecGrokCustom +
	`{"slug":"custom/qwen3.8-27b","display_name":"Qwen custom","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"minimal"}],"visibility":"hide"},` +
	cxhRecGeminiCustom +
	`{"slug":"claude-cxh-9","display_name":"","supported_reasoning_levels":[{"effort":"low"},{"effort":"turbo"},{"effort":"high"},{"effort":"LOW"}],"visibility":"list"},` +
	`{"slug":"cxh-bell","display_name":"Cxh\u0007Bell","visibility":"list"},` +
	`{"slug":"cxh-min","display_name":"Cxh Min","supported_reasoning_levels":[{"effort":"minimal"},{"effort":"low"}],"visibility":"list"},` +
	`{"slug":"customer-cxh","display_name":"Customer Cxh","visibility":"list"},` +
	`{"slug":"cxh\u001bx","display_name":"Esc","visibility":"list"},` +
	`{"slug":"","display_name":"Empty","visibility":"list"},` +
	`{"slug":"cxh-novis","display_name":"No Vis","supported_reasoning_levels":[{"effort":"high"}]}` +
	`]}` + "\n"

// cxhCodexFixture 是整份夹具：24916 字节，一行，末尾一个换行。
func cxhCodexFixture() string {
	return cxhCodexHead + cxhBigModelMessages() + cxhCodexTail
}
```

**夹具块 F2**（只在 `internal/repl/cxh_fixture_test.go`，另要 import `"testing"`）：

```go cxh:fixture-repl
// cxhCodexPath 是 cxh 配置里 codex 的 command；假执行器按它查表。
const cxhCodexPath = "cxh-codex.exe"

// cxhConfigTemplate 是 repl 用例的配置文件；<dataDir> 由 mdxConfigText 换成真路径。
// codex 与 grok 两家；gpt-5.5 以 high 已配成 agent（识别表里藏起那一行、不给编号）。
const cxhConfigTemplate = `# cxh 原有注释：这一行与下面每个字节都不许被改动
data_dir = '<dataDir>'

[providers.codex]
command = 'cxh-codex.exe'

[providers.grok]
command = 'mdx-grok.exe'

[agents."grok-4.7"]
display_name = 'Grok 4.7'
provider     = 'grok'
model        = 'grok-4.7'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents."gpt-5.5-high"]
display_name = 'GPT-5.5'
provider     = 'codex'
model        = 'gpt-5.5'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'
`

// cxhOneGpt55 是只有 gpt-5.5 一条记录的 codex 输出（R3）。
const cxhOneGpt55 = `{"models":[{"slug":"gpt-5.5","display_name":"GPT-5.5","default_reasoning_level":"xhigh","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"}]}` + "\n"

// cxhOnlyHiddenAndCustom 是 R6 empty 的 codex 输出：一条 hide、一条 list 的 custom/。
const cxhOnlyHiddenAndCustom = `{"models":[{"slug":"gpt-reserve","visibility":"hide"},{"slug":"custom/cpa-grok-4.7","display_name":"Grok 4.7 (CPA)","visibility":"list"}]}` + "\n"

// cxhCodexOnlyTemplate 是 R8 的配置文件：只有 codex 一家；[agents."gpt-5.5-high"] 那一段与 cxhConfigTemplate 里的逐字相同。
const cxhCodexOnlyTemplate = `# cxh 原有注释：这一行与下面每个字节都不许被改动
data_dir = '<dataDir>'

[providers.codex]
command = 'cxh-codex.exe'

[agents."gpt-5.5-high"]
display_name = 'GPT-5.5'
provider     = 'codex'
model        = 'gpt-5.5'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'
`

// cxhGpt55AndGrokCustom 是 R8 的 codex 输出：cxhOneGpt55 里的那一条 gpt-5.5，再接一条 list 的 custom/（与 cxhRecGrokCustom 同一条记录，没有末尾逗号）。
const cxhGpt55AndGrokCustom = `{"models":[{"slug":"gpt-5.5","display_name":"GPT-5.5","default_reasoning_level":"xhigh","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},{"slug":"custom/cpa-grok-4.7","display_name":"Grok 4.7 (CPA)","default_reasoning_level":"high","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"}]}` + "\n"

// cxhNewRig 用 MD1 的装置（mdxNewRig：真 SQLite、真配置文件、真 Runner、假执行器），
// 再给假执行器登记 codex 的输出。
func cxhNewRig(t *testing.T, tmpl, codexOut string) *mdxRig {
	t.Helper()
	g := mdxNewRig(t, tmpl)
	g.exec.set(cxhCodexPath, mdxResp{out: []byte(codexOut)})
	return g
}
```

**期望块 G1**（只在 `internal/provider/cxh_fixture_test.go`，另要 import `"council/internal/provider"`）。**`cxhSuggestedGolden` 的每个第二行开头是 4 个空格，不是 tab**（原型脚本曾把它误转成 tab、P3 当场变红；这 4 个空格与 MD1 P7 的格式串 `"%s\n    def=%q ..."` 对应）：

```go cxh:golden-provider
// cxhWantCodex 是 P2 的 10 行期望，顺序同夹具。Alias 一律空；Efforts 一档都没有时是 nil。
func cxhWantCodex() []provider.DiscoveredModel {
	row := func(model, name string, efforts []string, custom bool) provider.DiscoveredModel {
		return provider.DiscoveredModel{Provider: "codex", Model: model, Name: name, Efforts: efforts, Source: provider.ModelSourceCodexCatalog, Custom: custom}
	}
	six := []string{"low", "medium", "high", "xhigh", "max", "ultra"}
	return []provider.DiscoveredModel{
		row("gpt-6-sol", "GPT-6-Sol", six, false),
		row("gpt-6-luna", "GPT-6-Luna", []string{"low", "medium", "high", "xhigh", "max"}, false),
		row("gpt-5.6-sol", "GPT-5.6-Sol", six, false),
		row("gpt-5.5", "GPT-5.5", []string{"low", "medium", "high", "xhigh"}, false),
		row("custom/cpa-grok-4.7", "Grok 4.7 (CPA)", []string{"low", "medium", "high", "xhigh"}, true),
		row("custom/cpa-gemini-3.8-flash-high", "Gemini 3.8 Flash High (CPA)", []string{"high"}, true),
		row("claude-cxh-9", "", []string{"low", "high"}, false),
		row("cxh-bell", "", nil, false),
		row("cxh-min", "Cxh Min", []string{"minimal", "low"}, false),
		row("customer-cxh", "Customer Cxh", nil, false),
	}
}

// cxhSuggestedGolden 是 P3 的期望块：P2 解析出的非 custom 行，每行打五个值（格式同 MD1 的 P7）。
const cxhSuggestedGolden = `gpt-6-sol
    def="high" id="gpt-6-sol-high" idmax="gpt-6-sol-max" dn="GPT-6-Sol" dnmax="GPT-6-Sol max"
gpt-6-luna
    def="high" id="gpt-6-luna-high" idmax="gpt-6-luna-max" dn="GPT-6-Luna" dnmax="GPT-6-Luna max"
gpt-5.6-sol
    def="high" id="gpt-5.6-sol-high" idmax="gpt-5.6-sol-max" dn="GPT-5.6-Sol" dnmax="GPT-5.6-Sol max"
gpt-5.5
    def="high" id="gpt-5.5-high" idmax="gpt-5.5-max" dn="GPT-5.5" dnmax="GPT-5.5 max"
claude-cxh-9
    def="high" id="codex-cxh-9-high" idmax="codex-cxh-9-max" dn="Claude Cxh 9" dnmax="Claude Cxh 9 max"
cxh-bell
    def="" id="cxh-bell" idmax="cxh-bell-max" dn="Cxh Bell" dnmax="Cxh Bell max"
cxh-min
    def="minimal" id="cxh-min-minimal" idmax="cxh-min-max" dn="Cxh Min minimal" dnmax="Cxh Min max"
customer-cxh
    def="" id="customer-cxh" idmax="customer-cxh-max" dn="Customer Cxh" dnmax="Customer Cxh max"
`

// cxhRejectRows 是 P4 的表：每一行都得不到结果，只差报哪个哨兵。
var cxhRejectRows = []struct {
	no   int
	in   string
	want string
}{
	{1, "not json", "Unparseable"},
	{2, "", "Unparseable"},
	{3, "null", "Unparseable"},
	{4, "{}", "Unparseable"},
	{5, `{"models":null}`, "Unparseable"},
	{6, `[{"slug":"gpt-6-sol","visibility":"list"}]`, "Unparseable"},
	{7, `{"models":{"slug":"gpt-6-sol","visibility":"list"}}`, "Unparseable"},
	{8, `{"models":[]}`, "Empty"},
	{9, `{"models":[{"slug":"gpt-reserve","visibility":"hide"}]}`, "Empty"},
	{10, `{"models":[{"slug":"custom/cpa-grok-4.7","visibility":"list"}]}`, "Empty"},
	{11, `{"models":[{"slug":"","visibility":"list"}]}`, "Empty"},
	{12, `{"models":[{"slug":"a\u0007b","visibility":"list"}]}`, "Empty"},
	{13, `{"models":[{"slug":"gpt-6-sol"}]}`, "Empty"},
	{14, `{"models":[{"slug":"gpt-6-sol","visibility":"List"}]}`, "Empty"},
}
```

**期望块 G2**（只在 `internal/repl/cxh_fixture_test.go`）：

```go cxh:golden-repl
// cxhNote 是夹具里有 2 条 list 的 custom/ 记录时，识别表之后多出的那一行。
const cxhNote = "codex 目录里还有 2 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员"

// cxhNoteOne 是只剩 1 条时的那一行（R2 one）。
const cxhNoteOne = "codex 目录里还有 1 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员"

// cxhListBlock 是 R1 的「识别部分」（配置候选之后的全部行）。
const cxhListBlock = `正在识别模型: codex, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                      MODEL                   EFFORTS                          SOURCE
+1   codex     gpt-6-sol-high          gpt-6-sol               low|medium|high|xhigh|max|ultra  codex 目录
+2   codex     gpt-6-luna-high         gpt-6-luna              low|medium|high|xhigh|max        codex 目录
+3   codex     gpt-5.6-sol-high        gpt-5.6-sol             low|medium|high|xhigh|max|ultra  codex 目录
+4   codex     codex-cxh-9-high        claude-cxh-9            low|high                         codex 目录
+5   codex     cxh-bell                cxh-bell                -                                codex 目录
+6   codex     cxh-min-minimal         cxh-min                 minimal|low                      codex 目录
+7   codex     customer-cxh            customer-cxh            -                                codex 目录
+8   grok      grok-4.6                grok-4.6                -                                本地配置
+9   grok      grok-4.5                grok-4.5                -                                本地配置
+10  grok      grok-4.3                grok-4.3                -                                本地配置
+11  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -                                本地配置
+12  grok      grok-build              grok-build              -                                本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
` + cxhNote

// cxhGrokOnlyTable 是 codex 一行都没显示时的识别表（R3、R6 共用），提示行与 MD1 的原句逐字相同。
const cxhGrokOnlyTable = `识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO  PROVIDER  ID                      MODEL                   EFFORTS  SOURCE
+1  grok      grok-4.6                grok-4.6                -        本地配置
+2  grok      grok-4.5                grok-4.5                -        本地配置
+3  grok      grok-4.3                grok-4.3                -        本地配置
+4  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -        本地配置
+5  grok      grok-build              grok-build              -        本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档`

// cxhAllConfiguredBlock 是 R8 的识别部分：codex 识别出的可选行都已配成 agent，只剩 1 条 custom/。
const cxhAllConfiguredBlock = `正在识别模型: codex
识别出的模型都已经在配置里了
` + cxhNoteOne

// cxhGrokDownBlock 是 R9 的识别部分：grok 识别失败，表里只有 codex 的行（各列宽度只由 codex 的行定，
// 与 cxhListBlock 不同），说明行在失败行之前。
const cxhGrokDownBlock = `正在识别模型: codex, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO  PROVIDER  ID                MODEL         EFFORTS                          SOURCE
+1  codex     gpt-6-sol-high    gpt-6-sol     low|medium|high|xhigh|max|ultra  codex 目录
+2  codex     gpt-6-luna-high   gpt-6-luna    low|medium|high|xhigh|max        codex 目录
+3  codex     gpt-5.6-sol-high  gpt-5.6-sol   low|medium|high|xhigh|max|ultra  codex 目录
+4  codex     codex-cxh-9-high  claude-cxh-9  low|high                         codex 目录
+5  codex     cxh-bell          cxh-bell      -                                codex 目录
+6  codex     cxh-min-minimal   cxh-min       minimal|low                      codex 目录
+7  codex     customer-cxh      customer-cxh  -                                codex 目录
用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
` + cxhNote + `
识别 grok 的模型失败: model discovery command failed: cxh grok down
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh`

// cxhRefusalPairs 是 R4：每对是一条命令与它恰好打出的一行。
const cxhRefusalPairs = `/add +1 --effort minimal
gpt-6-sol 支持的强度是 low|medium|high|xhigh|max|ultra，没有 minimal

/add +2 --effort ultra
gpt-6-luna 支持的强度是 low|medium|high|xhigh|max，没有 ultra

/add +4 --effort turbo
claude-cxh-9 支持的强度是 low|high，没有 turbo

/add +1 --effort HIGH
gpt-6-sol 支持的强度是 low|medium|high|xhigh|max|ultra，没有 HIGH

/add +5 --effort high
cxh-bell 没有可选的强度，不能带 --effort

/add +8 --effort high
grok 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort`

// cxhAddSteps 是 R5 依次敲的六条命令与各自加出的 id。
var cxhAddSteps = []struct{ cmd, id string }{
	{"/add +1", "gpt-6-sol-high"},
	{"/add +1 --effort ultra", "gpt-6-sol-ultra"},
	{"/add +3", "gpt-5.6-sol-high"},
	{"/add +4", "codex-cxh-9-high"},
	{"/add +5", "cxh-bell"},
	{"/add +6", "cxh-min-minimal"},
}

// cxhAppendAll 是 R5 六步之后配置文件相对原文多出的字节。
const cxhAppendAll = "\n# council /add 追加：codex 识别出的 gpt-6-sol（codex 目录）\n[agents.gpt-6-sol-high]\ndisplay_name = \"GPT-6-Sol\"\nprovider = \"codex\"\nmodel = \"gpt-6-sol\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 gpt-6-sol（codex 目录）\n[agents.gpt-6-sol-ultra]\ndisplay_name = \"GPT-6-Sol ultra\"\nprovider = \"codex\"\nmodel = \"gpt-6-sol\"\neffort = \"ultra\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 gpt-5.6-sol（codex 目录）\n[agents.\"gpt-5.6-sol-high\"]\ndisplay_name = \"GPT-5.6-Sol\"\nprovider = \"codex\"\nmodel = \"gpt-5.6-sol\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 claude-cxh-9（codex 目录）\n[agents.codex-cxh-9-high]\ndisplay_name = \"Claude Cxh 9\"\nprovider = \"codex\"\nmodel = \"claude-cxh-9\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 cxh-bell（codex 目录）\n[agents.cxh-bell]\ndisplay_name = \"Cxh Bell\"\nprovider = \"codex\"\nmodel = \"cxh-bell\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：codex 识别出的 cxh-min（codex 目录）\n[agents.cxh-min-minimal]\ndisplay_name = \"Cxh Min minimal\"\nprovider = \"codex\"\nmodel = \"cxh-min\"\neffort = \"minimal\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"

// cxhAfterBlock 是 R5 六步之后再敲 /add 的识别部分（不再识别，没有「正在识别模型」行）。
const cxhAfterBlock = `识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                      MODEL                   EFFORTS                    SOURCE
+2   codex     gpt-6-luna-high         gpt-6-luna              low|medium|high|xhigh|max  codex 目录
+7   codex     customer-cxh            customer-cxh            -                          codex 目录
+8   grok      grok-4.6                grok-4.6                -                          本地配置
+9   grok      grok-4.5                grok-4.5                -                          本地配置
+10  grok      grok-4.3                grok-4.3                -                          本地配置
+11  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -                          本地配置
+12  grok      grok-build              grok-build              -                          本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
` + cxhNote

// cxhHelpAddLine、cxhHelpJoinLine 是 /help 输出里的两整行（COMMAND 列按 tabwriter 补到 12 个字符）。
const (
	cxhHelpAddLine  = "/add        把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别"
	cxhHelpJoinLine = "/join       同 /add"
)
```

### 2.1 `internal/provider/cxh_discovery_test.go`（`package provider_test`；夹具在 `cxh_fixture_test.go`）

先写一个前提守卫 `cxhCheckFixture(t)`：`len(cxhCodexFixture()) == 24916`、`len(cxhBigModelMessages()) == 22000`，不等就 `Fatalf`。P2、P3、P5 开头调用它。

- **P1 `TestCxhCodexCapabilityAndCommand`**：`provider.Lookup("codex")` 的 `Capabilities().ModelDiscovery == provider.ModelDiscoverySubcommand` 且 `Available()` 为真；`DiscoveryCommandFor("codex", "P")` 的 `Path == "P"`、`Args` DeepEqual `[]string{"debug", "models"}`、`Stdin == nil`、`CloseStdinAfter == nil`；`provider.ModelSourceCodexCatalog == 4` 且 `.Label() == "codex 目录"`；`provider.ModelSource(5).Label() == "-"`。
  【判别力】能力位退回 Unknown（C1）；命令少了 `debug`（C2）；给了非 nil 的 Stdin（C3）；codex 那一支删掉（C5，err 非 nil）；标签写成「订阅」（C19）；在 AgyQuota 与 CodexCatalog 之间多插一个常量（C40，取值变 5）。
- **P2 `TestCxhParseCodexFixture`**：`ParseDiscovery("codex", []byte(cxhCodexFixture()))` 无错，结果与 `cxhWantCodex()` 整值 `reflect.DeepEqual`（10 行，顺序同夹具，含两条 `Custom` 为真的行，`cxh-bell` 与 `customer-cxh` 的 `Efforts` 是 nil）。
  【判别力】不滤 visibility（C7：多出 `gpt-reserve`、`cxh-novis`，hide 的 `custom/qwen3.8-27b` 变成一条 Custom 行）；只滤 `hide`（C8：多出 `cxh-novis`）；不滤空 slug / 控制字符 slug（C9、C10）；强度不按 `codexEffortLevels` 收（C11：`claude-cxh-9` 多出 `turbo`、`LOW`）；display_name 不滤控制字符（C12）；custom 恒假（C13）；前缀写成 `custom`（C14：`customer-cxh` 变成 Custom）；来源写错（C18）；Efforts 写成空切片（C43）；codex 那一支从 `ParseDiscovery` 删掉（C6）。夹具第 1 条的 `model_messages` 里有嵌套的 `slug` / `visibility` 诱饵，按键名在全文里扫的写法会多出 `cxh-decoy` 一行。
- **P3 `TestCxhSuggestedNames`**：对 P2 的结果跳过 `Custom` 为真的行，每行按 MD1 P7 同一格式打 `m.Model` 与 `def=%q id=%q idmax=%q dn=%q dnmax=%q`（`def := m.DefaultEffort()`，`id = SuggestedID(def)`、`idmax = SuggestedID("max")`、`dn`、`dnmax` 同理），拼起来与 `cxhSuggestedGolden` 整段相等。再开子测试 `infix`（W1b 修订）：`provider.DiscoveredModel{Provider: "codex", Model: "cxh-claude-x"}.SuggestedID("high") == "cxh-claude-x-high"`（`claude-` 在 slug 中间，不是前缀，不换）。
  【判别力】撞名前缀规则去掉（C20：`claude-cxh-9-high`）；撞名判据写成「含」而不是「以……开头」（C50：`strings.Contains(id, other+"-")`）时 `infix` 得到 `codex-cxh-claude-x-high`（夹具里会撞名的 `claude-cxh-9` 以 provider id 开头，golden 部分不红；W1b 修订原型实测只有 `infix` 子测试红，没有它时 C50 在三个包里全部存活）；强度为空也接 `-`（C23：`cxh-bell-`）；display_name 为空不回退（C25）；强度 high 也写进显示名（C26）；codex 那一支从 `SuggestedDisplayName` / `SuggestedID` 删掉走 default（C27 `Gpt 6 Sol`、C28 `gpt-6-sol`）。「默认强度改用 default_reasoning_level」这种写法要改 `DiscoveredModel`，不在变异表里，但它会让 `gpt-6-sol`（medium）、`gpt-5.5`（xhigh）两行变；`cxh-min` 一行钉住「没有 high 取第一档」。
- **P4 `TestCxhParseCodexRejects`**：先跑**正对照** `{"models":[{"slug":"gpt-6-sol","visibility":"list"}]}`：无错，恰好一行 `{Provider: "codex", Model: "gpt-6-sol", Source: provider.ModelSourceCodexCatalog}`（其余字段零值，Efforts nil），不成立就 `Fatalf`。再开子测试 `vendor_slash`（v1.1，F5；W1b 修订换了输入）：`{"models":[{"slug":"vendor/custom/cxh-x","visibility":"list"}]}` 无错，恰好一行、`Custom` 为假，整值 DeepEqual `[]provider.DiscoveredModel{{Provider: "codex", Model: "vendor/custom/cxh-x", Source: provider.ModelSourceCodexCatalog}}`。再对 `cxhRejectRows` 每一行开子测试 `row%02d`：结果为空，`want == "Unparseable"` 时错误链里有 `ErrDiscoveryUnparseable` 且没有 `ErrDiscoveryEmpty`，`want == "Empty"` 时反过来。
  【判别力】第 9-14 行各只比正对照差一处（hide、custom/、空 slug、BEL、缺 visibility、`List`）：第 9 行由 C7 负责，第 10 行由 C13、C15、C16，第 11 行由 C9，第 12 行由 C10，第 13、14 行由 C7、C8（只滤 `hide` 的写法放过缺席与 `List`）；第 10 行（只有一条 custom）钉住「custom 不算可选」（C15、C16）；第 3-5 行在去掉 nil 检查（C17）时解引用 nil 指针 panic、子测试红；第 1-7 行与第 8 行对调哨兵的写法两个方向都红。`vendor_slash` 钉住「custom 是 `custom/` 前缀，不是包含 `custom/`，也不是含斜杠」：判据写成 `strings.Contains(m.Slug, "/")`（C46）或 `strings.Contains(m.Slug, codexCustomSlugPrefix)`（C48）时这一行被当成 custom，一行可选的都没有，报 Empty（W1b 修订原型实测：两条都只有 P4 红，P4 里只有 `vendor_slash` 这一个子测试红；夹具与真目录里含 `custom/` 的 slug 都以它开头，P2 不红）。输入里的 `custom/` 不能去掉：v1.1 的输入 `vendor/cxh-x` 只杀 C46，C48 在它上面存活（同一装置只差这一处输入，W1b 修订原型实测）。
- **P5 `TestCxhDiscoverModelsCodex`**：用一个记录调用的执行器（返回 `cxhCodexFixture()` 的字节）调 `provider.DiscoverModels(ctx, "codex", "cxh-codex.exe", exec)`：无错，结果 DeepEqual `cxhWantCodex()`；执行器恰好被调 1 次，收到的 `Path == "cxh-codex.exe"`、`Args == {"debug","models"}`、`Stdin == nil`、`CloseStdinAfter == nil`。
  【判别力】`Path` 不用传进来的 path、写死成别的值（C4 写死成解析不到的 `cxh-nosuch-codex`）；C2、C3、C5、C6 同 P1 / P2。

### 2.2 `internal/turn/cxh_discovery_exec_test.go`（`package turn`；复用 MD1 的 `TestMain`，不新增）

- **T1 `TestCxhRunDiscoveryCodexCommand`**：`t.Setenv("MDX_FAKE_DISCOVERY", "readall")`；`cmd, _ := provider.DiscoveryCommandFor("codex", os.Args[0])`；10 s 超时的 ctx 调 `RunDiscovery(ctx, cmd)`：`err == nil`，stdout 逐字等于 `"mdx-readall stdin=0 args=[\"debug\" \"models\"]\n"`（`readall` 模式的格式见 `mdx_discovery_exec_test.go:68-71`）。
  【判别力】这是 codex 的命令经 Council 真执行器落到子进程 argv 的唯一一条：少了 `debug`（C2）或 codex 一支删掉（C5，`DiscoveryCommandFor` 报错）都红；`Path` 写死成 `cxh-nosuch-codex`（C4）时 `RunDiscovery` 报 `exec: "cxh-nosuch-codex": executable file not found in %PATH%`、stdout 空（v1.1 原型实测；v1 的 C4 写死成 `"codex"`，会按 PATH 起本机真的 codex，已改）；给了非空 stdin 的写法 `stdin=` 不是 0。

### 2.3 `internal/repl/cxh_addmodel_test.go`（`package repl`；夹具在 `cxh_fixture_test.go`）

装置一律 `g := cxhNewRig(t, <配置>, <codex 输出>)`，配置除 R8 用 `cxhCodexOnlyTemplate` 外都是 `cxhConfigTemplate`。`g.run("/add")` 的前几行是 MD1 既有的配置候选表（`cxhConfigTemplate` 是 `gpt-5.5-high`、`grok-4.7` 两行，`cxhCodexOnlyTemplate` 只有 `gpt-5.5-high` 一行，都以既有提示行结尾），用 `g.afterCandidates` 取之后的「识别部分」再比。

- **R0 `TestCxhFixtureIntact`**：`len(cxhCodexFixture()) == 24916`（前提守卫，repl 包那一份夹具没抄坏）。
- **R1 `TestCxhAddListsCodex`**：`/add` 的识别部分 == `mdxSplit(cxhListBlock)`（17 行）；假执行器恰好被调 2 次，第 1 次 `Path == cxhCodexPath`、`Args == {"debug","models"}`、`Stdin == nil`，第 2 次 `Path == mdxGrokPath`。
  【判别力】首行「正在识别模型: codex, grok」钉住 codex 进了识别（C1、C4、C5、C6 时只剩 grok 或多一行失败）；`gpt-5.5` 已配成 agent 而没有编号、`+1` 到 `+7` 连续；来源列「codex 目录」（C18、C19）；EFFORTS 列按模型（C11）；提示行是「claude 与 codex」那一句（C35）；最后一行说明「还有 2 个」（C29、C30、C42：custom 行被编号或不计数）；说明行在表格与提示行之后（C49：说明行块与它前面的 if / else 块整段对调时，说明行跑到首行之后、表头之前）；建议 id 列（C20、C23、C28）。与 R3 是一对：R3 的可见行里没有 codex 行，提示行回到原句。
- **R2 `TestCxhAddListNoCustom`**：两个子测试，夹具都由 `mdxReplaceOnce` 从 `cxhCodexFixture()` 删掉记录得到（恰好一处是前提）。`one`：删掉 `cxhRecGrokCustom`，识别部分 == `cxhListBlock` 去掉最后一行、再接 `cxhNoteOne`。`none`：再删掉 `cxhRecGeminiCustom`，识别部分 == `cxhListBlock` 去掉最后一行（没有说明行）。
  【判别力】与 R1 只差 custom 记录的条数（2 / 1 / 0），表格逐字相同：`if snap.custom > 0` 写成 `> 1`（C31）在 R2 里只有 `one` 红（R8 也只剩 1 条 custom，同样红）；写成恒真（C32）时 `none` 多出「还有 0 个」（MD1 的既有用例也红）；数目写死成 2 的写法 `one` 红；说明行打在表格之前（C49）时只有 `one` 红（`none` 没有说明行，W1b 修订原型实测）。hide 的 `custom/qwen3.8-27b` 一直在夹具里，从不计入。
- **R3 `TestCxhAddHintWhenCodexHidden`**：codex 输出用 `cxhOneGpt55`（只有 `gpt-5.5` 一条，且它已配成 agent）。识别部分 == `"正在识别模型: codex, grok"` 接 `mdxSplit(cxhGrokOnlyTable)`（提示行是 MD1 原句）。
  【判别力】codex 那一行在快照里（编号 0）但不可见：提示行按快照全部行判（C33）、恒写「claude 与 codex」（C34）、判据写反（C36）都红。它与 R1 是一对。
- **R4 `TestCxhAddCodexEffortRefusals`**：先 `/add` 拍快照、记下配置文件字节；把 `cxhRefusalPairs` 按空行切成 6 对，每对第 1 行敲进去，输出 == 第 2 行这一行；最后配置文件字节不变。
  【判别力】`gpt-6-sol --effort minimal`：minimal 在 CX1 的并集里、不在这个模型的档里——识别层按并集放行的写法在这里加出一个成员；`gpt-6-luna --effort ultra` 同理（luna 没有 ultra）；`claude-cxh-9 --effort turbo`：不按 `codexEffortLevels` 收（C11）时 turbo 被当成支持的档；`HIGH`：大小写敏感；`cxh-bell --effort high`：没有强度的 codex 行；`+8` 是 grok 行，拒绝句与 MD1 逐字相同（强度 switch 把 codex 与 grok 一起放行的写法在这里红）。强度 switch 退回 `!= "claude"`（C37）时前 5 对都变成 grok / agy 那一句。正对照是 R5（同一装置，`+1` 默认与 `+1 --effort ultra` 成功）。
- **R5 `TestCxhAddCodexWritesConfig`**：记下原文件；`/add`；按 `cxhAddSteps` 依次敲 6 条，每条前后用 `g.bakNames()` / `g.newBak(...)` 取新备份，输出 == `g.joinedLines(id, bak)`（两行）；6 步之后文件 == 原文 + `cxhAppendAll`（整串相等）；`g.r.cfg.Agents["gpt-6-sol-high"]` DeepEqual `config.AgentConfig{DisplayName: "GPT-6-Sol", Provider: "codex", Model: "gpt-6-sol", Effort: "high", Role: "", Permissions: config.PermissionRead, Mode: config.ModeDiscussion}`；再敲 `/add`，识别部分 == `mdxSplit(cxhAfterBlock)`；假执行器总共只被调 2 次（快照沿用）。
  【判别力】默认强度 high、`--effort ultra` 成功（C37 时 `+1` 写出没有 effort 行的 `gpt-6-sol`，`--effort ultra` 被拒）；显示名（C12 `Cxh\u0007Bell`、C25、C26、C27）；带点号 id 的表名加引号；`cxh-bell` 没有 effort 行；`cxh-min` 默认 minimal；追加注释里的来源「（codex 目录）」（C18、C19）；6 步之后 `+2`、`+7` 仍在（编号不漂），说明行照样重印，而且在表格之后（C49 时 `cxhAfterBlock` 那一段红）。
- **R6 `TestCxhAddCodexFailures`**：两个子测试。`empty`：codex 输出 `cxhOnlyHiddenAndCustom`；`unparseable`：codex 输出 `` `{"data":[]}` + "\n" ``。识别部分 == `"正在识别模型: codex, grok"` + `mdxSplit(cxhGrokOnlyTable)` + 失败行 + `"失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh"`，失败行分别是 `"识别 codex 的模型失败: 一个模型都没列出来（没登录？）"` 与 `"识别 codex 的模型失败: 输出认不出来（CLI 版本变了？）"`。
  【判别力】`empty` 里有一条 list 的 custom 行：把 custom 算成可选（C15、C16）时它不再失败、改打说明行；两个子测试只差 codex 的输出，一个走 Empty、一个走 Unparseable。`unparseable` 喂的 `{"data":[]}` 没有 `models` 键：去掉 nil 检查（C17）时解引用 nil 指针 panic，装置（`runCtx`）接住判红（v1.1 原型实测）。`empty` 也钉住「Empty 时不打 custom 说明」（custom 行随失败一起丢掉）。
- **R7 `TestCxhHelpAddLine`**：`/help` 的输出里有与 `cxhHelpAddLine` 整行相等的一行，也有与 `cxhHelpJoinLine` 整行相等的一行。
  【判别力】summary 少了 `--refresh`（C39）；`/join` 被跟着改的写法。`TestUx2rJoinIsAdd`（B6）钉 `summary` 字段本身，R7 钉经 `cmdHelp` 渲染出的整行。
- **R8 `TestCxhAddNoteWhenAllConfigured`**（v1.1，F0）：`g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhGpt55AndGrokCustom)`（配置只有 codex 一家，`gpt-5.5` 以 high 已配成 agent；codex 输出是这一条 `gpt-5.5` 加一条 list 的 `custom/cpa-grok-4.7`）。`/add` 的识别部分 == `mdxSplit(cxhAllConfiguredBlock)`，恰好三行：`正在识别模型: codex`、`识别出的模型都已经在配置里了`、`cxhNoteOne`；假执行器恰好被调 1 次。
  【判别力】设计裁定 3 的第一件事：识别出的可选行都已配成 agent、没有表格时，custom 说明行照样打在「都已经在配置里了」那一行之后。说明行只在可见行非空时打（C44，W1 的 X1）或只在成功识别的家数大于 1 时打（C47，W1 的 X4）都少了第三行；`> 0` 写成 `> 1`（C31）同样少（这里只剩 1 条 custom）；说明行块与它前面的 if / else 块整段对调（C49）时第二、三行次序反了。这是 v1 没有用例走到的一支：v1 里 custom > 0 的用例（R1、R2 one、R4、R5）可见行都非空、两家都识别成功。
- **R9 `TestCxhAddNoteBeforeFailures`**（v1.1，F0）：`g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())`，再 `g.exec.set(mdxGrokPath, mdxResp{err: errors.New("cxh grok down")})`。`/add` 的识别部分 == `mdxSplit(cxhGrokDownBlock)`（14 行）：首行、只有 codex 七行的识别表、L2 提示行、`cxhNote`、`识别 grok 的模型失败: model discovery command failed: cxh grok down`、`失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh`。表格的各列宽度只由 codex 的行定（`NO` 列 4 个字符，`ID` 列 18 个），与 `cxhListBlock` 不同，所以整段冻结成 G2 常量，不从 `cxhListBlock` 切。
  【判别力】设计裁定 3 的第二件事：说明行在失败行之前。把说明行块整段挪到失败行循环之后（C45，W1 的 X2，写成两段相邻文本对调的一处替换）时最后三行次序变；说明行只在成功识别的家数大于 1 时打（C47）时少了说明行。R8 与 R9 各杀对方杀不掉的那一条：C44 只有 R8 红（R9 可见行非空），C45 只有 R9 红（R8 没有失败行），C47 两条都红（v1.1 原型实测）。说明行打在表格与提示行之后这一端由 C49 登记（W1b 修订：说明行块与它前面的 if / else 块整段对调，是一处替换；R1、R2、R5、R8、R9 都红，原型实测）。

---

## 3. 冻结的字面量

用户可见的新字面量（逐字节；`%d` 处是 custom 行数）：

- **L1** 来源标签：`codex 目录`（识别表 SOURCE 列；追加进配置的注释行 `# council /add 追加：codex 识别出的 <slug>（codex 目录）`，格式是 MD1 既有的，只是标签新）。
- **L2** 提示行（可见行里有 codex 行时）：
  ```text
  用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
  ```
  可见行里没有 codex 行时仍是 MD1 原句（`claude 可加`），逐字不变。
- **L3** custom 说明行（`snap.custom > 0` 时）：
  ```text
  codex 目录里还有 %d 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员
  ```
- **L4** `/add` 的 summary（改前是 `把配置里的 agent 加进当前会话: /add 列出可加的；/add <全名|编号|前缀>[,...] 一次加多个`）：
  ```text
  把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别
  ```
  `/help` 里渲染成的整行见 G2 的 `cxhHelpAddLine`（COMMAND 列补到 12 个字符）。`/join` 的 summary `同 /add` 不变。
- 对 codex 行**新变得可达**的既有字面量（MD1 冻结，本刀不改文字）：`%s 支持的强度是 %s，没有 %s`、`%s 没有可选的强度，不能带 --effort`、`识别 %s 的模型失败: ...`、`正在识别模型: %s`。

不上屏的新字面量（冻结在 A2 的代码里，只进错误链；`discoveryFailureText` 把 Unparseable / Empty 映射成中文句，这些英文不会单独上屏）：`codex: stdout is not JSON: %w`、`codex: no models array: %w`、`codex: %w`。

---

## 4. 集成员的变异清单

在 §1.1 A + B 与 §2 全部落地、全量绿之后逐条跑：从落地后文件的**字节副本**还原（不许 `git checkout`，未提交的实现只活在工作树里）；每条只施加一处替换（「替换前」在文件里恰好一处）；跑「杀手」列点名的包与用例（`go test -count=1 -run <用例> <包>`）。原型实测的杀手见下表（「原型实测」列是 `-run '^(TestCxh|TestMdx|TestCxx)'`（provider）、`'^(TestCxh|TestMdx|TestUx2r)'`（repl）、`'^TestCxh'`（turn）下实际变红的用例；改 `internal/repl` 的变异只跑 repl，provider、turn 两个包不依赖 repl）。v1.1 在 `4dbd17a` 的原型上把 47 条全部重跑了一遍，杀手列整列按实测结果重写，每条都从字节副本还原并逐字节核对相等。跑哪一条都不起真 codex（v1.1 起 C4 写死的是解析不到的路径）。C44-C47 是 v1.1 新增（W1 的 X1、X2、X3、X4）。C48-C50 是 W1b 修订新增；W1b 修订在 `E:\Isolating_test\council\cxh_fix\` 的原型上把 C1-C50 全部重跑了一遍，每条实际变红的用例集合与杀手列逐条相等（没有漏红，也没有表外多出的红用例），等价的 4 条都存活，每条都从字节副本还原并逐字节核对相等。表内字符串按 JSON 写（`\n` 是换行、`\t` 是 tab、`\"` 是双引号）；`\|` 是表格里的竖线转义，文件里就是 `|`。

| # | 变异（文件：替换前 → 替换后，JSON 写法） | 预期被谁杀（原型实测） |
|---|---|---|
| C1 | `internal/provider/codex.go`：`"ModelDiscovery: ModelDiscoverySubcommand,"` → `"ModelDiscovery: ModelDiscoveryUnknown,"` | provider:TestCxhCodexCapabilityAndCommand,TestCxxCapabilities,TestMdxCapabilitiesDeclareModelDiscovery；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddHintWhenCodexHidden,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddCodexFailures,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures |
| C2 | `internal/provider/discovery.go`：`"Args: []string{\"debug\", \"models\"}}, nil"` → `"Args: []string{\"models\"}}, nil"` | provider:TestCxhCodexCapabilityAndCommand,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex；turn:TestCxhRunDiscoveryCodexCommand |
| C3 | `internal/provider/discovery.go`：`"Args: []string{\"debug\", \"models\"}}, nil"` → `"Args: []string{\"debug\", \"models\"}, Stdin: []byte{}}, nil"` | provider:TestCxhCodexCapabilityAndCommand,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex |
| C4 | `internal/provider/discovery.go`：`"return DiscoveryCommand{Path: path, Args: []string{\"debug\", \"models\"}}, nil"` → `"return DiscoveryCommand{Path: \"cxh-nosuch-codex\", Args: []string{\"debug\", \"models\"}}, nil"` | provider:TestCxhCodexCapabilityAndCommand,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddHintWhenCodexHidden,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddCodexFailures,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures；turn:TestCxhRunDiscoveryCodexCommand |
| C5 | `internal/provider/discovery.go`：`"\tcase \"codex\":\n\t\treturn DiscoveryCommand{"` → `"\tcase \"codex-x\":\n\t\treturn DiscoveryCommand{"` | provider:TestCxhCodexCapabilityAndCommand,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddHintWhenCodexHidden,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddCodexFailures,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures；turn:TestCxhRunDiscoveryCodexCommand |
| C6 | `internal/provider/discovery_parse.go`：`"\tcase \"codex\":\n\t\treturn parseCodexDiscovery(stdout)"` → `"\tcase \"codex-x\":\n\t\treturn parseCodexDiscovery(stdout)"` | provider:TestCxhParseCodexFixture,TestCxhSuggestedNames,TestCxhParseCodexRejects,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddHintWhenCodexHidden,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddCodexFailures,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures |
| C7 | `internal/provider/discovery_parse.go`：`"if m.Visibility != \"list\" \|\| m.Slug == \"\" \|\| hasControlChar(m.Slug) {"` → `"if m.Slug == \"\" \|\| hasControlChar(m.Slug) {"` | provider:TestCxhParseCodexFixture,TestCxhSuggestedNames,TestCxhParseCodexRejects,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddCodexFailures,TestCxhAddNoteBeforeFailures |
| C8 | `internal/provider/discovery_parse.go`：`"if m.Visibility != \"list\" \|\| m.Slug == \"\" \|\| hasControlChar(m.Slug) {"` → `"if m.Visibility == \"hide\" \|\| m.Slug == \"\" \|\| hasControlChar(m.Slug) {"` | provider:TestCxhParseCodexFixture,TestCxhSuggestedNames,TestCxhParseCodexRejects,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C9 | `internal/provider/discovery_parse.go`：`"if m.Visibility != \"list\" \|\| m.Slug == \"\" \|\| hasControlChar(m.Slug) {"` → `"if m.Visibility != \"list\" \|\| hasControlChar(m.Slug) {"` | provider:TestCxhParseCodexFixture,TestCxhSuggestedNames,TestCxhParseCodexRejects,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C10 | `internal/provider/discovery_parse.go`：`"if m.Visibility != \"list\" \|\| m.Slug == \"\" \|\| hasControlChar(m.Slug) {"` → `"if m.Visibility != \"list\" \|\| m.Slug == \"\" {"` | provider:TestCxhParseCodexFixture,TestCxhSuggestedNames,TestCxhParseCodexRejects,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C11 | `internal/provider/discovery_parse.go`：`"if codexEffortAllowed(l.Effort) {"` → `"if l.Effort != \"\" {"` | provider:TestCxhParseCodexFixture,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexEffortRefusals,TestCxhAddNoteBeforeFailures |
| C12 | `internal/provider/discovery_parse.go`：`"if hasControlChar(name) {\n\t\t\tname = \"\""` → `"if false {\n\t\t\tname = \"\""` | provider:TestCxhParseCodexFixture,TestCxhSuggestedNames,TestCxhDiscoverModelsCodex；repl:TestCxhAddCodexWritesConfig |
| C13 | `internal/provider/discovery_parse.go`：`"custom := strings.HasPrefix(m.Slug, codexCustomSlugPrefix)"` → `"custom := false"` | provider:TestCxhParseCodexFixture,TestCxhSuggestedNames,TestCxhParseCodexRejects,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddCodexFailures,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures |
| C14 | `internal/provider/discovery_parse.go`：`"const codexCustomSlugPrefix = \"custom/\""` → `"const codexCustomSlugPrefix = \"custom\""` | provider:TestCxhParseCodexFixture,TestCxhSuggestedNames,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C15 | `internal/provider/discovery_parse.go`：`"if !custom {\n\t\t\tselectable++\n\t\t}"` → `"selectable++"` | provider:TestCxhParseCodexRejects；repl:TestCxhAddCodexFailures |
| C16 | `internal/provider/discovery_parse.go`：`"if selectable == 0 {"` → `"if len(out) == 0 {"` | provider:TestCxhParseCodexRejects；repl:TestCxhAddCodexFailures |
| C17 | `internal/provider/discovery_parse.go`：`"if v.Models == nil {"` → `"if false {"` | provider:TestCxhParseCodexRejects；repl:TestCxhAddCodexFailures |
| C18 | `internal/provider/discovery_parse.go`：`"Source:   ModelSourceCodexCatalog,"` → `"Source:   ModelSourceSubscription,"` | provider:TestCxhParseCodexFixture,TestCxhParseCodexRejects,TestCxhDiscoverModelsCodex；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C19 | `internal/provider/discovery.go`：`"return \"codex 目录\""` → `"return \"订阅\""` | provider:TestCxhCodexCapabilityAndCommand；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C20 | `internal/provider/discovery.go`：`"if strings.HasPrefix(id, other+\"-\") {\n\t\t\t\tid = \"codex-\""` → `"if false {\n\t\t\t\tid = \"codex-\""` | provider:TestCxhSuggestedNames；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C21 | `internal/provider/discovery.go`：`"if other == \"codex\" {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif strings.HasPrefix(id, other+\"-\") {\n\t\t\t\tid = \"codex-\""` → `"if false {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif strings.HasPrefix(id, other+\"-\") {\n\t\t\t\tid = \"codex-\""` | 等价，原型实测存活（见表后） |
| C22 | `internal/provider/discovery.go`：`"id = \"codex-\" + strings.TrimPrefix(id, other+\"-\")\n\t\t\t\tbreak"` → `"id = \"codex-\" + strings.TrimPrefix(id, other+\"-\")"` | 等价，原型实测存活（见表后） |
| C23 | `internal/provider/discovery.go`：`"if effort != \"\" {\n\t\t\tid += \"-\" + sanitizeModelID(effort)\n\t\t}\n\t\treturn id\n\tdefault:"` → `"if true {\n\t\t\tid += \"-\" + sanitizeModelID(effort)\n\t\t}\n\t\treturn id\n\tdefault:"` | provider:TestCxhSuggestedNames；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C24 | `internal/provider/discovery.go`：`"if effort != \"\" {\n\t\t\tid += \"-\" + sanitizeModelID(effort)\n\t\t}\n\t\treturn id\n\tdefault:"` → `"if effort != \"\" {\n\t\t\tid += \"-\" + effort\n\t\t}\n\t\treturn id\n\tdefault:"` | 等价，原型实测存活（见表后） |
| C25 | `internal/provider/discovery.go`：`"name := m.Name\n\t\tif name == \"\" {"` → `"name := m.Name\n\t\tif false {"` | provider:TestCxhSuggestedNames；repl:TestCxhAddCodexWritesConfig |
| C26 | `internal/provider/discovery.go`：`"name := m.Name\n\t\tif name == \"\" {\n\t\t\tname = prettyModelName(sanitizeModelID(m.Model))\n\t\t}\n\t\tif effort != \"\" && effort != \"high\" {"` → `"name := m.Name\n\t\tif name == \"\" {\n\t\t\tname = prettyModelName(sanitizeModelID(m.Model))\n\t\t}\n\t\tif effort != \"\" {"` | provider:TestCxhSuggestedNames；repl:TestCxhAddCodexWritesConfig |
| C27 | `internal/provider/discovery.go`：`"\tcase \"codex\":\n\t\tname := m.Name"` → `"\tcase \"codex-x\":\n\t\tname := m.Name"` | provider:TestCxhSuggestedNames；repl:TestCxhAddCodexWritesConfig |
| C28 | `internal/provider/discovery.go`：`"\tcase \"codex\":\n\t\tid := sanitizeModelID(m.Model)"` → `"\tcase \"codex-x\":\n\t\tid := sanitizeModelID(m.Model)"` | provider:TestCxhSuggestedNames；repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C29 | `internal/repl/addmodel.go`：`"if m.Custom {\n\t\t\t\tsnap.custom++"` → `"if false {\n\t\t\t\tsnap.custom++"` | repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures |
| C30 | `internal/repl/addmodel.go`：`"snap.custom++\n\t\t\t\tcontinue"` → `"continue"` | repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures |
| C31 | `internal/repl/addmodel.go`：`"if snap.custom > 0 {"` → `"if snap.custom > 1 {"` | repl:TestCxhAddListNoCustom,TestCxhAddNoteWhenAllConfigured |
| C32 | `internal/repl/addmodel.go`：`"if snap.custom > 0 {"` → `"if true {"` | repl:TestCxhAddListNoCustom,TestCxhAddHintWhenCodexHidden,TestCxhAddCodexFailures,TestMdxAddListsDiscovered,TestMdxAddPlusWritesConfigAndJoins,TestMdxAddSnapshotStable,TestMdxAddDiscoveryFailures,TestMdxAddSkipsUndiscoverableProviders,TestMdxAddAliasControl,TestMdxAddSnapshotAcrossReload,TestMdxAddCancelNotCached |
| C33 | `internal/repl/addmodel.go`：`"for _, row := range visible {\n\t\t\tif row.m.Provider == \"codex\" {"` → `"for _, row := range snap.rows {\n\t\t\tif row.m.Provider == \"codex\" {"` | repl:TestCxhAddHintWhenCodexHidden |
| C34 | `internal/repl/addmodel.go`：`"who := \"claude\""` → `"who := \"claude 与 codex\""` | repl:TestCxhAddHintWhenCodexHidden,TestCxhAddCodexFailures,TestMdxAddListsDiscovered,TestMdxAddPlusWritesConfigAndJoins,TestMdxAddSnapshotStable,TestMdxAddDiscoveryFailures,TestMdxAddSkipsUndiscoverableProviders,TestMdxAddAliasControl,TestMdxAddSnapshotAcrossReload,TestMdxAddCancelNotCached |
| C35 | `internal/repl/addmodel.go`：`"who = \"claude 与 codex\""` → `"who = \"claude\""` | repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteBeforeFailures |
| C36 | `internal/repl/addmodel.go`：`"if row.m.Provider == \"codex\" {\n\t\t\t\twho"` → `"if row.m.Provider != \"codex\" {\n\t\t\t\twho"` | repl:TestCxhAddHintWhenCodexHidden,TestCxhAddCodexFailures,TestCxhAddNoteBeforeFailures,TestMdxAddListsDiscovered,TestMdxAddPlusWritesConfigAndJoins,TestMdxAddSnapshotStable,TestMdxAddDiscoveryFailures,TestMdxAddSkipsUndiscoverableProviders,TestMdxAddAliasControl,TestMdxAddSnapshotAcrossReload,TestMdxAddCancelNotCached |
| C37 | `internal/repl/addmodel.go`：`"case m.Provider != \"claude\" && m.Provider != \"codex\":"` → `"case m.Provider != \"claude\":"` | repl:TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig |
| C38 | `internal/repl/addmodel.go`：`"case m.Provider != \"claude\" && m.Provider != \"codex\":"` → `"case m.Provider == \"grok\":"` | repl:TestMdxAddRefusals |
| C39 | `internal/repl/commands.go`：`"把识别出的模型写进配置并加入；/add --refresh 重新识别\""` → `"把识别出的模型写进配置并加入\""` | repl:TestCxhHelpAddLine,TestUx2rJoinIsAdd |
| C40 | `internal/provider/discovery.go`：`"\tModelSourceAgyQuota\n\t// ModelSourceCodexCatalog"` → `"\tModelSourceAgyQuota\n\tModelSourceMutGap\n\t// ModelSourceCodexCatalog"` | provider:TestCxhCodexCapabilityAndCommand |
| C41 | `internal/repl/addmodel.go`：`"if row.m.Provider == \"codex\" {\n\t\t\t\twho = \"claude 与 codex\"\n\t\t\t\tbreak"` → `"if row.m.Provider == \"codex\" {\n\t\t\t\twho = \"claude 与 codex\""` | 等价，原型实测存活（见表后） |
| C42 | `internal/repl/addmodel.go`：`"snap.custom++\n\t\t\t\tcontinue"` → `"snap.custom++"` | repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexEffortRefusals,TestCxhAddCodexWritesConfig,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures |
| C43 | `internal/provider/discovery_parse.go`：`"var efforts []string"` → `"efforts := []string{}"` | provider:TestCxhParseCodexFixture,TestCxhParseCodexRejects,TestCxhDiscoverModelsCodex |
| C44 | `internal/repl/addmodel.go`：`"if snap.custom > 0 {"` → `"if snap.custom > 0 && len(visible) > 0 {"` | repl:TestCxhAddNoteWhenAllConfigured |
| C45 | `internal/repl/addmodel.go`：`"\t// 只在确实跳过了 custom/ 行时说明一句，带数量。\n\tif snap.custom > 0 {\n\t\tif err := r.printf(\"codex 目录里还有 %d 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员\\n\", snap.custom); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\tfor _, f := range snap.failures {\n\t\tif err := r.printf(\"识别 %s 的模型失败: %s\\n\", f.provider, discoveryFailureText(f.err, r.discoverTimeout)); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n"` → `"\tfor _, f := range snap.failures {\n\t\tif err := r.printf(\"识别 %s 的模型失败: %s\\n\", f.provider, discoveryFailureText(f.err, r.discoverTimeout)); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\t// 只在确实跳过了 custom/ 行时说明一句，带数量。\n\tif snap.custom > 0 {\n\t\tif err := r.printf(\"codex 目录里还有 %d 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员\\n\", snap.custom); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n"` | repl:TestCxhAddNoteBeforeFailures |
| C46 | `internal/provider/discovery_parse.go`：`"custom := strings.HasPrefix(m.Slug, codexCustomSlugPrefix)"` → `"custom := strings.Contains(m.Slug, \"/\")"` | provider:TestCxhParseCodexRejects |
| C47 | `internal/repl/addmodel.go`：`"if snap.custom > 0 {"` → `"if snap.custom > 0 && snap.okCount > 1 {"` | repl:TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures |
| C48 | `internal/provider/discovery_parse.go`：`"custom := strings.HasPrefix(m.Slug, codexCustomSlugPrefix)"` → `"custom := strings.Contains(m.Slug, codexCustomSlugPrefix)"` | provider:TestCxhParseCodexRejects |
| C49 | `internal/repl/addmodel.go`：`"\tif len(visible) > 0 {\n\t\tif err := r.printf(\"识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:\\n\"); err != nil {\n\t\t\treturn err\n\t\t}\n\t\ttw := r.newTabWriter()\n\t\tfmt.Fprintln(tw, \"NO\\tPROVIDER\\tID\\tMODEL\\tEFFORTS\\tSOURCE\")\n\t\tfor _, row := range visible {\n\t\t\tm := row.m\n\t\t\tfmt.Fprintf(tw, \"+%d\\t%s\\t%s\\t%s\\t%s\\t%s\\n\",\n\t\t\t\trow.no, m.Provider, m.SuggestedID(m.DefaultEffort()), m.Model, effortsCell(m.Efforts), m.Source.Label())\n\t\t}\n\t\tif err := tw.Flush(); err != nil {\n\t\t\treturn err\n\t\t}\n\t\t// 可见行里有 codex 的行时，提示里带上 codex；没有时与 MD1 的原句逐字相同。\n\t\twho := \"claude\"\n\t\tfor _, row := range visible {\n\t\t\tif row.m.Provider == \"codex\" {\n\t\t\t\twho = \"claude 与 codex\"\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n\t\tif err := r.printf(\"用 /add +编号 加入；--id <新 id> 改名；%s 可加 --effort <强度> 换档，EFFORTS 列是它支持的档\\n\", who); err != nil {\n\t\t\treturn err\n\t\t}\n\t} else if snap.okCount > 0 {\n\t\tif err := r.printf(\"识别出的模型都已经在配置里了\\n\"); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\t// 只在确实跳过了 custom/ 行时说明一句，带数量。\n\tif snap.custom > 0 {\n\t\tif err := r.printf(\"codex 目录里还有 %d 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员\\n\", snap.custom); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n"` → `"\t// 只在确实跳过了 custom/ 行时说明一句，带数量。\n\tif snap.custom > 0 {\n\t\tif err := r.printf(\"codex 目录里还有 %d 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员\\n\", snap.custom); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\tif len(visible) > 0 {\n\t\tif err := r.printf(\"识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:\\n\"); err != nil {\n\t\t\treturn err\n\t\t}\n\t\ttw := r.newTabWriter()\n\t\tfmt.Fprintln(tw, \"NO\\tPROVIDER\\tID\\tMODEL\\tEFFORTS\\tSOURCE\")\n\t\tfor _, row := range visible {\n\t\t\tm := row.m\n\t\t\tfmt.Fprintf(tw, \"+%d\\t%s\\t%s\\t%s\\t%s\\t%s\\n\",\n\t\t\t\trow.no, m.Provider, m.SuggestedID(m.DefaultEffort()), m.Model, effortsCell(m.Efforts), m.Source.Label())\n\t\t}\n\t\tif err := tw.Flush(); err != nil {\n\t\t\treturn err\n\t\t}\n\t\t// 可见行里有 codex 的行时，提示里带上 codex；没有时与 MD1 的原句逐字相同。\n\t\twho := \"claude\"\n\t\tfor _, row := range visible {\n\t\t\tif row.m.Provider == \"codex\" {\n\t\t\t\twho = \"claude 与 codex\"\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n\t\tif err := r.printf(\"用 /add +编号 加入；--id <新 id> 改名；%s 可加 --effort <强度> 换档，EFFORTS 列是它支持的档\\n\", who); err != nil {\n\t\t\treturn err\n\t\t}\n\t} else if snap.okCount > 0 {\n\t\tif err := r.printf(\"识别出的模型都已经在配置里了\\n\"); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n"` | repl:TestCxhAddListsCodex,TestCxhAddListNoCustom,TestCxhAddCodexWritesConfig,TestCxhAddNoteWhenAllConfigured,TestCxhAddNoteBeforeFailures |
| C50 | `internal/provider/discovery.go`：`"if strings.HasPrefix(id, other+\"-\") {\n\t\t\t\tid = \"codex-\""` → `"if strings.Contains(id, other+\"-\") {\n\t\t\t\tid = \"codex-\""` | provider:TestCxhSuggestedNames |

**判定为等价、不列杀手的变异**（原型实跑都存活，逐条论证）：

- **C21**（撞名循环里去掉 `if other == "codex" { continue }`）：对 `codex` 自己，替换是把 `codex-` 换成 `codex-`，结果不变。永远等价。
- **C22**（撞名循环里去掉 `break`）：替换之后 id 以 `codex-` 开头；`IDs()` 按字典序是 agy、claude、codex、grok，codex 被跳过，其余三家的前缀都不可能再匹配 `codex-`。永远等价。
- **C24**（`sanitizeModelID(effort)` 换成 `effort`）：进到这里的强度只可能是该模型 `Efforts` 里的值（`cmdAddDiscovered` 先校验），而 `Efforts` 只收 `codexEffortLevels` 的七个小写纯字母值，`sanitizeModelID` 对它们是恒等。等价（与 MD1 claude 一支同一写法，保留是为一致）。
- **C41**（提示行循环里去掉 `break`）：之后每次命中都是给 `who` 赋同一个值。永远等价。

**只在审查时看**（没有能编译、能观测的变异，或观测要造不现实的输入）：
- `codex.go` `:50` 上方补的注释、`:110-117` 的新注释与 v1.1 订正的三处注释（`:227-229`、`:277-279`、`:385-388`）逐字等于 A3，`turn.go` 两处逐字等于 A6，`cxx_codex_test.go:8-9` 逐字等于 B7（脚本比）；`:110-117` 的新注释里「是否联网、是否反映订阅都没有验证」的标注在。（v1 这里还写了「custom 来源」的推断标注齐全，`codex.go` 的注释不提 custom，v1.1 删掉；custom 来源在 A1、A2 的注释里写成了事实，见 §7 待主代理第 11 条。）
- `addmodel.go:53-55` 按文件头相交表处理后，头注释不再说 codex 没有适配器。
- `ModelSourceCodexCatalog` 放在常量块末尾：C40 钉住「插在中间会改值」，放在 AgyQuota 之前的写法 MD1 的 P5（AgyQuota == 3）会红。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘**（不建文件、不改文件、不跑会写盘的命令），只在回复正文里输出。
- **只交两段补丁**：§1.1 A 部分（6 个生产文件：`internal/provider/discovery.go`、`internal/provider/discovery_parse.go`、`internal/provider/codex.go`、`internal/provider/turn.go`、`internal/repl/addmodel.go`、`internal/repl/commands.go`）与 B 部分（3 个既有测试文件：`internal/provider/mdx_discovery_test.go`、`internal/provider/cxx_codex_test.go`、`internal/repl/ux2r_add_test.go`），**逐字等于**本文 `cxh:patch-prod` 与 `cxh:patch-tests` 两个代码块（含注释；不许改任何字节，不许增删锚点）。不许交任何别的文件，不许写新测试文件（那是测试腿的事）。
- 格式：每个文件一段
  ```text
  ===PATCH: <仓库相对路径>===
  <<<<ANCHOR
  <锚点原文，恰好在 HEAD 真文件出现一次，含行首 tab>
  ====
  <替换文本>
  >>>>
  ...（同一文件的下一处）
  ===END PATCH===
  ```
  新文件用 `===FILE: 路径===` ... `===END FILE===`（本刀没有新的生产文件）。锚点从 HEAD `4dbd17a` 真文件逐字取，缩进是 tab。
- 符号禁令：代码与注释里不许出现 U+1F300-1FAFF、U+2700-27BF、U+2600-26FF、U+2B00-2BFF、U+2460-24FF、U+FE0F、U+200D；中文、希腊字母、数学算符、普通箭头放行。本文补丁里没有这些码位（v1.1 修订后重扫过）。
- 生产代码的新标识符不许以 `cxh` 开头（补丁里是 `ModelSourceCodexCatalog`、`Custom`、`codexCustomSlugPrefix`、`codexCatalogModel`、`parseCodexDiscovery`，以及局部变量）。
- 规模：A 部分原型 +170 / -33 行（含注释；W1b 修订后实测），不超过 400 行，**一块交付**，不拆。B 部分 +21 / -21 行，同一次交出。
- 末尾写 `===REPORT===`，逐条列：交了哪几个文件、每个文件几处锚点（A：discovery.go 12、discovery_parse.go 3、codex.go 5、turn.go 2、addmodel.go 5、commands.go 1；B：mdx_discovery_test.go 7、cxx_codex_test.go 3、ux2r_add_test.go 1）、有没有偏离本文的地方（应当没有）。

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；FU2 已在 `4dbd17a` 落地（文件头），落盘前确认 HEAD 仍是 `4dbd17a`，不是的话按新 HEAD 逐个复核锚点；按锚点落盘（每个恰好一处）；`gofmt -l`（空）；`go vet ./internal/provider ./internal/repl ./internal/turn`；`go build ./...`。逐字核对：A、B 两段落盘后与本文两个代码块逐字相同（脚本比，不目测）。
2. **合并测试腿的 5 个文件**之后，先只跑 `./internal/provider ./internal/repl ./internal/turn`，再跑全量 `go test -count=1 ./...`（原型实测九包全绿）。另一条流水线也在跑 go test 时，3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制；多测试方并行时只跑自己的包，避免假红。
3. **§4 的变异逐条跑**（C1-C50，含 4 条等价的也跑一次确认存活），从字节副本还原；任何一条非等价变异没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
4. **文档**：
   - `docs/00_DECISIONS.md`：#46 末尾补一段「CXH 落地（<commit>）」：codex 进识别（`codex debug models`）、`custom/` 行不列只计数、来源「codex 目录」、默认 high、逐模型强度（CX1 open item 5 的识别层一半收掉）、`/help` 补 `+编号` 与 `--refresh`（#46 11:30 裁定那一条完成）；§7 的待 Griffin 各项照列。#45 第一节「三家的识别方式」后补一句 codex 的识别方式。
   - `docs/02_AGENT_PROVIDER_ARCHITECTURE.md`：`ModelDiscovery` 那一行（`:64` 附近）与 codex 列补「Subcommand（`codex debug models`）」。
   - `docs/16a` 与 `docs/17b` 各在文件头加一行落地后订正：16a open item (e) 已由 CXH 完成；17b §7 待主代理第 5 条（识别层一半）、第 11 条、第 14 条里 `addmodel.go:54-55` 那一处已由 CXH 处理。
   - `docs/README.md` 路线图加一行；`.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的配置加一段**（Griffin 已授权主代理带备份追加；主代理做，不在本刀代码里）：先原样备份 `C:\Users\25142\.council\config.toml`（逐字节比对相同）。**前提（v1.1，F4）**：先在备份副本里查有没有（去掉行首空白后）以 `[providers.codex]` 开头的行。**有**就跳过追加：配置不动，只做下面那次加载校验，并把那张表里现有的 `command` 值原样报给 Griffin（他若照 17b §6 手加过 codex 成员，这张表一定已经在，`config/validate.go:69` 要求 agent 引用的 provider 在 `[providers]` 里）。**没有**才追加：只在文件末尾追加下面几行（LF、无 BOM；原有字节逐字节不变），写后用 Council 的加载校验核一次（拷到隔离目录、只换 data_dir、行式启动敲 `/providers`，不发任何提问）：
   ```toml

   # codex 模型识别（CXH）：/add 会跑一次 codex debug models（零花费）列出 codex 的模型
   [providers.codex]
   command = 'codex'
   ```
   行首检查只认 `[providers.codex]` 这一种写法；同一张表的别的写法（`[providers."codex"]`、`[providers]` 下的 `codex = {...}`）行首检查看不见，追加之后是重复的表，加载校验会失败（v1.1 原型探针：`[providers.codex]`、`[providers."codex"]`、`[providers]` 下 `codex = {...}` 三种写法后面再接一张 `[providers.codex]`，`config.Load` 都报 `Key 'providers.codex' has already been defined.`）：那时从备份原样恢复，把两段原文一起报给 Griffin，不自己合并。
   `command = 'codex'` 由 `exec.LookPath` 解析到 `codex.cmd`（本机 go1.27.0 实测，§0 第 5 条）。加了这一段之后，`/add` 不带参数时会多识别 codex 一家（约 0.2 秒）；codex 成员仍要经 `/add +编号` 或手写 `[agents.*]` 才有，**这一段本身不加任何成员、不花钱**。
6. **真机复验（Griffin；全部零花费，不问任何模型）**。每条 council 内命令单独一块，逐条粘贴、逐条回车（#44）：
   1. 启动：
      ```
      council
      ```
   2. 新建一个会话：
      ```
      /new codex识别复验
      ```
   3. 列出可加的（第一次会识别；codex 一家约 0.2 秒）：
      ```
      /add
      ```
      期望：识别部分首行「正在识别模型: 」后面列出他配置里全部能识别的 provider，其中有 `codex`；表里有 7 行 `codex`（`gpt-6-sol`、`gpt-6-astra`、`gpt-6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.5`，建议 id 都以 `-high` 结尾），SOURCE 列是 `codex 目录`，EFFORTS 列 `gpt-6-luna` 没有 ultra、`gpt-5.5` 到 xhigh 为止；提示行是「claude 与 codex 可加 --effort」；其后一行「codex 目录里还有 2 个 custom/ 开头的模型没有列出……」。codex 行的条数与名字以他那台机器的 `codex debug models` 为准，与上面不同就原样交给主代理。
   4. 看帮助里 `/add` 那一行：
      ```
      /help
      ```
      期望：`/add` 那一行含 `/add +编号 [--id <新 id>] [--effort <强度>]` 与 `/add --refresh 重新识别`；`/join` 那一行仍是「同 /add」。
   5. （可选，会追加写他的配置文件，写前自动备份；不花钱）把 `gpt-6-sol` 以默认 high 加进来，`+N` 换成第 3 步表里 `gpt-6-sol` 那一行的编号：
      ```
      /add +N
      ```
      期望两行：「已把 agent gpt-6-sol-high 追加进 <配置路径>（原文件备份在 <备份路径>）」与「gpt-6-sol-high 已加入会话 <会话 id>」。若他 17b §6 之后已经手加过 `gpt-6-sol-high`，这一步会被拒（`配置里已经有同一个模型、同一强度的 agent …` 或 `配置里已经有 agent …`），属预期。
   6. 看成员配置：
      ```
      /agents
      ```
      期望：`gpt-6-sol-high  codex  gpt-6-sol  high  read  discussion` 一行（列宽以屏幕为准）。
   7. 退出：
      ```
      /quit
      ```
   第 5 步之后若要真的问 `gpt-6-sol-high`，那是付费调用，走 17b §6 第 6 步起的流程，要他另外授权。
7. **清理**：本刀在中转根与 scratchpad 里的全部中间物（任务书、grok 回复、收割脚本）。

---

## 7. open items

### 待 Griffin（都先取了保守读法，改哪一条都只动字面量或一个判据）

1. **`custom/` 行不列、只说一句**（§1.1 设计裁定 3）。另一种读法是列出来、SOURCE 标「经 codex 转发」让他自己选；代价是能经 codex 加出一个其实是 grok / gemini 的成员（身份、用量口径、隔离参数都按 codex 算）。说明行的措辞（L3）也可改。
2. **来源标签「codex 目录」**（L1）。目录是否反映订阅没验证，所以不标「订阅」；他若确认 codex 的目录就是他订阅里能用的模型，可以改标「订阅」。
3. **默认强度 high**，不取 codex 自己的 `default_reasoning_level`（设计裁定 6）。
4. **display_name 取 CLI 原名 `GPT-6-Sol`**（17b §6 的手写示例是 `GPT-6 Sol`）。display_name 进指纹，已加入的成员事后改名要付一次入场费。
5. **`/join` 的 summary 维持「同 /add」**（设计裁定 11）。

### 待主代理

1. **FU2 相交**（v1.1 已结）：FU2 已在 `4dbd17a` 落地，没有动 v1 的任何锚点，`addmodel.go:53-55` 那一块保留（文件头）。落盘时 HEAD 若已不是 `4dbd17a`，按新 HEAD 逐个复核锚点。U43A 与本刀文件不相交（文件头）。
2. **Empty 的中文句「一个模型都没列出来（没登录？）」对 codex 不准**：`debug models` 与登录的关系没验证 [未验证]；codex 的目录里全是 hide / custom 时也走这一句。MD1 的通用措辞，改它会动 MD1 冻结的字面量，本刀不改。
3. **visibility 缺席的行一律跳过**：将来某版 codex 若去掉这个键，整份清单会变成 Empty。升级 codex 之后零花费跑一次 `codex debug models` 看键还在不在。`supported_in_api` 本刀不看（今天 13 条全是 true）。
4. **夹具里的真名**：`custom/cpa-grok-4.7`、`custom/cpa-gemini-3.8-flash-high` 两个 slug 与 display_name 取自 Griffin 自己 codex 配置派生的输出，进了测试文件；`custom/qwen3.8-27b` 的 display_name 已换成合成值、description 全删。要不要把前两个也换成合成名，主代理定（换了要同步改 F1 的 `cxhRecGrokCustom`、`cxhRecGeminiCustom`，F2 的 `cxhOnlyHiddenAndCustom`、`cxhGpt55AndGrokCustom`，G1 `cxhWantCodex` 的两行，以及两处夹具字节数前提守卫；G2 的屏幕期望块不受影响，custom 行不上屏）。
5. **agy 反向撞名**：`IDs()` 含 codex 之后，agy 里 label 以 `Codex ` 开头的行会被改成 `agy-` 前缀（MD1 open item (o)、17b §7 待主代理第 10 条）；今天 agy 清单没有这种行，不改。
6. **手写配置的 codex 成员仍只按并集校验强度**（CX1 不改）：`gpt-6-luna` 写 `ultra` 照样放行，真实行为未实测。要按模型校验就得在 `BuildTurn` 里查目录（本包不起进程），不在本刀。
7. **快照里的强度与真实调用之间没有再核对**：快照拍下之后 codex 升级、某模型去掉了某一档，`/add +编号 --effort` 仍按旧快照放行；`/add --refresh` 或新会话重拍即可。与 MD1 的快照语义一致，不改。
8. **codex 的识别用的是 `codex.cmd`**（§0 第 5 条实跑过）；Council 起 `exec` 走 `codex.cmd` 这一路仍只在 CX1 §6 第 6 步验证，与本刀无关。
9. **提示行在只有 codex 可见时仍说「claude 与 codex」**：那一句说的是哪几家能带 `--effort`，不是当前表里有哪几家；与 MD1 原句（没有 claude 行时也说 claude）同一读法。
10. **R5 的装置写真临时目录**：与 MD1 R2 相同，每步一个备份文件，`t.TempDir()` 清理。
11. **custom 行的来源在冻结代码里写成了事实**（v1.1 审 F3 时「同类的还有没有别处」找到，裁决没有点名，本版不改冻结代码）：A1 `DiscoveredModel` 的 `Custom` 注释写「是用户在自己的 codex 配置里自定义的模型（Griffin 的是经 CPA 转发的 grok 与 gemini）」，A2 `codexCustomSlugPrefix` 注释写「是用户在自己的 codex 配置里自定义转发的模型的 slug 前缀」；§0 第 5 条把「经 CPA 转发到 grok / gemini」标为 [推断]（本契约不读 `~/.codex`）。L3 用户可见句「它们是你在 codex 配置里自定义转发的模型，多半是别家的」前半句同样没有限定。要不要在两处注释里补 `[推断]`（只改注释，不动变异表与期望块），由主代理定；L3 是待 Griffin 第 1 条的措辞，一并交他。
12. **`internal/provider/turn.go` 里「三家」的计数已过期**（v1.1 顺带并进时看到，不在裁决范围）：`:90`「三家各自的计算公式」、`:94`「三条不同的算法」、`:169-170`「三家的 InputTokens 由三条不同公式算出」，`internal/turn/runner.go:1342` 同一句；codex 接入后是四家（`codex.go:70` 也声明了自己的 `UsageFormula`）。本刀只改了 A6 那两句关于 usage_raw 的话，这些计数留给以后的注释清理。
13. **`/help` 里 `/session` 那一行要不要并进本刀**（W1b 修订登记；来源是 U43A v1.1 §7.2 待主代理第 1 条）：`commands.go:153` 的 summary「列出当前会话每个成员的 provider 会话；/session rebuild <agent-id> 丢弃并重建」不提 U43A 加的「先结清陈旧的在飞记录」。U43A 写明「要么由主代理给 CXH 追加这一行，要么另开一刀」，自己不改。本刀取保守读法：不收，A5 仍只有 `:143` 一处。主代理若定并进，要改的是：A5 多一处锚点（§5 REPORT 里 `commands.go` 从 1 处变 2 处）；新 summary 整串冻结进 §3；§2.3 补一条经 `/help` 渲染出的整行断言（R7 的体例）；§4 补一条变异（例如删掉新加的那半句）并实跑点名杀手。现查：`_test.go` 里没有用例钉 `/session` 的 summary（`丢弃并重建` 在 `internal/` 下只有 `commands.go:153` 与 `ask.go:178` 的注释两处），所以不产生新的 B 部分。并进与否都不改变与 U43A 的文件不相交（U43A 不动 `commands.go`）；措辞要等 U43A 落地后的实际行为定，先落 CXH 再补这一行也可以。
14. **MD1 agy 一支的撞名判据没有「中缀」用例**（W1b 修订问「同类的还有没有别处」时找到，不在本刀范围）：`discovery.go` 的 agy 一支 `if strings.HasPrefix(id, other+"-") {` 写成 `strings.Contains` 时，provider、repl、turn 三个包的全量 `go test` 都不红（原型实测，从字节副本还原）。MD1 的夹具里会撞名的 label（`Claude Sonnet 4.6 (Thinking)` 等）都以 provider id 开头。codex 一支同一个缺口由本刀的 P3 `infix` 与 C50 补上。agy 那一支要不要补一条 label 中间含 `claude-` 的用例，由主代理定；补的话只加测试，不改 MD1 的生产代码。
