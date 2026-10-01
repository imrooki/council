# AD1 冻结契约：/add 按名字加识别出的模型（v1.3，2026-10-01）

> **归档说明（2026-10-01，主代理）**：本文是 AD1 的冻结契约 v1.3，原样归档。派工时的原件是 `E:\multi_model_cowork\ad1_contract.md`，锚点与行号指 HEAD `64c4b49`。AD1 与本文、`00_DECISIONS.md` #49 在同一个提交里落地（`d4ad85f`），2026-10-01 Griffin 说「装」后装为 `council 0.2.0-1001a`（vcs.revision `e8f4cf0`），真机未复验。
> - **契约经过**：一共四版。
>   - v1 → W1：1 条 major，开关打开时配置里的拒绝型命中没有用例担保。
>   - v1.1 → W1b：1 条 major，只敲模型名会给会话里已有的模型新建重复成员，敲两次结果不同。
>   - 这两条 major 都由反驳者证实；主代理定了一条统一规则，改成 v1.2。
>   - v1.2 → W1c：按文件头写死的严重度判法，没有 major。
>   - v1.3 只改测试、文字与分块，经定向核验通过。
> - **实现**：生产代码分四块（A、B-甲、B-乙、C），三路 grok-4.7 并行实现，每路只拿自己那几块的契约摘录（55-64 KB），与 §1.1 逐字节相同（6 个文件）。
> - **测试**：2 个 adn 测试文件由一条 Opus 测试腿写成，与 §2 逐字节相同。
> - **集成**：Opus 集成员没有做机械修正。
>   - §4 的 92 条变异里 90 条被点名杀手杀掉，V7、V70 等价。
>   - 774 次点名杀手检查全部变红，没有漏的，也没有多的。
>   - 每次运行后四项泄漏核对全部干净。
>   - 实现腿的原文由集成员从任务文本转写，转写后打在干净 HEAD 上，与照契约打出的树逐字节相同。
> - **审查**：Opus 独立审查报 3 条 minor（见下），都不是实现偏离。
>   - 另做了开关关着时的差分探针：约 70 条命令、4 套装置，与 HEAD 逐字节相同。
>   - 另拟 9 条变异，7 条被杀；M6（Join 的 Active 写 false）是等价的，因为 store 把 active 写死成 1；M9 是覆盖缺口，见下。
>
> **落地后订正（2026-10-01，主代理；读本文之前先读这里）**：
> - **一处承重但零覆盖的守卫**（审查，M9）：
>   - `parseAddNameArgs` 里 `--id` 后面缺值那一半守卫（`i+1 >= len(fields)`），删掉之后 0 个用例变红；删掉的后果是 `/add x --id` 越界 panic。
>   - 生产代码是对的，开关打开的探针里打的是用法那一句。
>   - 补法：N14 加一行 `{"/add gpt-6.1-sol --id", adnUsage}`，§4 登记这条变异。留给后续小刀。
> - **两处注释说得不全**（审查）：`commands.go` 里 cmdAdd 头注释新加的那一段、`addname.go` 的文件头第一段，都说「配置里对不上才到快照里找，找到就写进配置并加入」。它们漏了两种情形：
>   - 两个词与带参数时，只是前缀命中也会去识别（D7、D9）；
>   - D17 的结局可能只是加入已有成员，或者拒绝、列出，都不写配置。
>   代码不受影响。
> - **守卫提示会让重敲的命令换档**（审查，与 HEAD 的 +编号 相同）：守卫拒绝时打「再 /add <名字>」，不带第二个词与 `--effort`，照着重敲会落到默认档，或者落进 D17。D12 与 V20 有意这样冻结，以后要改就与 +编号 一起改成回显完整的原命令。
> - **§6 第 5 步第 0 条**说「主代理照惯例等 Griffin 说『装』」，照此办理：落地时没有装，等 Griffin 说了「装」才装的 `0.2.0-1001a`。
> - **已由 CL1 做了**（2026-10-01，`5c4840a`，#50，`19h`）：上面第一条（`--id` 缺值守卫的用例，19h 的 C1）、第二条（两处注释，19h 的 A4、A5），以及 §7 (e)（`/help` 补上按名字加，19h 的 A1）。第三条（守卫提示让重敲的命令换档）仍留着。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `64c4b49`（完整 `64c4b49adc668336fe5ee88a63acd31660e257bf`，= origin/main）。代码与 `ef76eef`（OB3 落地）相同，之后只有文档提交。Go 全路径 `"C:\Program Files\Go\bin\go.exe"`（go1.27.0 windows/amd64）。本机没有 gcc，不能 `-race`。E 盘机（Windows 11，PowerShell 5.1）。
- 本刀的 KEY 是 `AD1`；测试腿的包级标识符前缀是 `adn`（类型、函数、变量、常量一律 `adn` 开头，测试函数 `TestAdn...`）。生产代码的新标识符不许以 `adn` 开头。本文的生产代码新标识符：`SetAddByName`、字段 `addByName`、`cmdAddByName`、`addByOneName`、`parseAddNameArgs`、`snapshotForName`、`matchDiscoveredName`、`idFormEffort`、`hasEffort`、`nameMatch`、`addNameArgs`、`msgAddNameNotFound`、`msgAddNameUsage`、`msgAddNameOneAtATime`、`msgAddNameEffortTwice`、`msgAddNameFlagsConfig`、`msgAddNameAmbiguous`，以及 v1.1 新增的 `configPrefixOnly`、`effortOfAny`、`anyRowHasEfforts`（`addname.go`），`ensureModelSnapshot`、`addDiscoveredModel`（`addmodel.go`，从既有代码抽出），`resolveAddConfig`、`addSegmentNotFound`（`commands.go`，从 `resolveAddSegment` 拆出），v1.1 新增的常量 `ideographicSpace`（`commands.go`，A8），以及 v1.2 新文件 `addname_existing.go`（A9）里的 `addNameExisting`、`sameModelAgents`、`joinConfiguredAgent`、`msgAddNamePresent`、`msgAddNamePresentEffort`、`msgAddNameConfigured`、`msgAddNameConfiguredEffort`。
- 来源：
  - Griffin 2026-09-30 在真机上敲 `/add gpt-6.1-sol xigh`、`/add gpt-6.1-sol-xhigh`、`/add gpt-6.1-sol-high`，都被「配置里没有可加入的 …」挡回（主代理转述；任务书同一处又说他第一次敲的是 `/add gpt-6.1-sol xhigh`，本文两种都按原样实跑，见 §0 第 3 条与 §7 (a)）。他的原话：「我需要能/add 加入订阅的，不是说只有我们事先选好的那种，你懂吧」。
  - `docs/00_DECISIONS.md` #45 第一节：识别之后直接 `/add` 写进 config.toml；「已有成员一个不动，换模型只新建 id」（`:2128`）。
  - `docs/16a_MD1_MODEL_DISCOVERY_CONTRACT.md` open item (c)（`:2504`）：拍快照时已在配置里的模型没有编号，换强度只能手改配置。#46（`:2180`）的主代理裁定维持了「不编号、手改配置」。本刀换一条路解决：不给它们编号，而是让它们能按名字加（D3）。
- 证据等级照实写：「实测」是本机跑过的；「原型实跑」是在下面的隔离拷贝里跑过的；「主代理实测」是任务书转述、本刀没有重跑的；[未验证] / [推断] 照字面。

**严重度的判法（v1.2 起，W1c 与以后的审查员照这个判，逐字执行）**：只有下面四种算 major 以上：
- 不该写配置时写了；
- 对上已知成员或模型，却被静默丢掉或加错；
- 开关关着时行为变了（U+3000 那一条除外）；
- 测试写出了隔离目录。

措辞、覆盖缺口、说法不够准，一律算 minor。

**v1.3 变更记录（2026-10-01）**：照主代理裁决书 `E:\multi_model_cowork\ad1_rulings_v13.md`（唯一权威）修订；W1c 原文 `ad1_w1c_findings.md`，v1.2 快照 `ad1_contract_v12.md`。基线仍是 HEAD `64c4b49`。**只改测试、文字与分块，生产代码一个字节不改。** 原型在 `E:\Isolating_test\council\ad1_v13\`（`git archive 64c4b49` 解出，先把 v1.2 的 §1.1 与 §2 逐字落盘，build、vet、gofmt 干净、全量九包全绿，再改 S1），交付前连同隔离目录一起删除；主仓库一个字节没写。

- **冻结代码没动**（脚本从 v1.2 与 v1.3 两份契约各自解析、逐块比）：16 个生产代码块（14 对锚点与替换文本、A9、`addname.go`）与 v1.2 逐字节相同，S2 整文件相同。块 B 拆成两块只动了标题与说明，代码块的顺序与字节不变。所以开关关着时的行为、D1-D17 的实现都与 v1.2 相同。A9 文件头注释里的「调用方是块 B 的 addname.go」是冻结代码，不改；「块 B」指的是 v1.3 的块 B-乙，在 A9 的标题里注明。
- **裁决书逐条**：
  - not_applied (w)：接受，留在 §7 (w)，写明前缀命中的可能是另一个模型、有意保留，并加 m1 的用例。(x)、(y)：接受，§7 (x)、(y) 各加一句。(v)、(s)、(t)、(u)：§7 原文不动。
  - 没有用例担保的两处：`sameModelAgents` 的 provider 那一半由 m4 补上（N22 other_provider_same_model，V91）；`joinConfiguredAgent` 的 Join 失败那一支留在 §4「只在审查时看」。
  - m1：新 N25 `TestAdnSingleIDConfigPrefix`（W1c 的 w2 装置：配置里有另一个模型的 `gpt-6.1-sol-xhigh-old`，`/add gpt-6.1-sol-xhigh` 按前缀加入它、回显它的 id、执行器 0 次、不写），同一装置再敲一次是正对照（识别、新建、写）。杀手 V29（第一条就识别、新建，原型实测）。
  - m2：N14 的表里加 `/add 1 --effort high`（期望 D9 那一句、执行器 0 次）；§4 登记 V92（W7），杀手是父测试 `TestAdnFlags`；§4「只在审查时看」里 `configPrefixOnly` 那一句改成 v1.3 订正。
  - m3：N22 加 present_among_absent（`adnTwoSolAgents`，`/add sol-a` 之后 `/add gpt-6.1-sol`）与 present_among_absent_grok（两个 g46 成员，加入一个之后 `/add grok-4.6`），§4 登记 V87（W1）、V88（W2）。
  - m4：N22 加 other_provider_same_model（配置加 `[agents.gx] provider = 'grok' model = 'gpt-6.1-sol'`，`/add gpt-6.1-sol` 新建 codex 的 `gpt-6.1-sol-high`、写配置），§4 登记 V91（W5）；one_absent_config 之后断言会话成员记录里 sol-a 的显示名与 provider 是配置里的值，§4 登记 V89（W3）、V90（W4）；W6 进「只在审查时看」，写明理由。
  - m5：§7 新 (ab) 两条，不改代码。
  - 分块：v1.2 的块 B（419 行）照 §7 (aa) 的办法拆成块 B-甲（`commands.go` 的 B1-B4，64 行）与块 B-乙（`addname.go`，355 行），四块都在 400 行以内（D16、§5）。两块互相引用、要一起落地才编译得过（原型实测：只落块 A 加其中一块，`go build` 都不过），写在 D16 与块 B-甲的说明里。裁决书说的「D12 写明理由」按上下文是 D16（分块写在 D16）；拆得开，所以没有写「拆不开的理由」。
- **测试改了什么**（S1 整文件，脚本从原型写回；S2 不变；既有测试文件零改动）：**没有改任何既有期望**。新加 N25；N14 表里一条；N22 三个子用例；one_absent_config 的夹具多一行 `display_name = 'Sol A'`、加成员记录断言（配置不写显示名时它是空串，V89 看不出来，所以加）；文件头与 N14、N22、N25 的注释与【判别力】。S1 从 965 行到 1027 行。
- **本文定的、裁决书没写到的**：92 条变异全部重跑、杀手列与耗时全部换成 v1.3 的实测（理由见 §4 v1.3 段：新用例会被 V1 这类整体变异一起杀，只跑新的六条，旧的杀手列就不再是全部）；m1 做成独立的测试函数 N25，正对照是同一条命令再敲一次；one_absent_config 夹具加显示名；两块的名字用「块 B-甲」「块 B-乙」（不用 B1、B2，免得与补丁编号 B1-B5 撞名）；§5 改成四块可以分几次交。
- **契约文字**：标题；本段；D9（编号命中一句）、D16（块 B 拆分、合计行数）、D17（S 的 provider 那一半与 (ab)，第 2、3 种结局各加一句）；§1.1 块 B-甲、块 B-乙的标题与说明，块 C 的标题，A8、A9 标题里的「块 B」；§2 的开头、纪律加 v1.3 一条、N14、N22、新 N25；§4 的 v1.2 段标题、新 v1.3 段、V87-V92、全部 92 条的耗时与杀手列、「只在审查时看」三处；§5 的文件、行数与交付次序、REPORT 里的块；§6 第 1 条、第 3 条；§7 (w)(x)(y)(z)(aa) 各加一句，新 (ab)；文末泄漏核对与往返核对加 v1.3。§3 不变（没有新字面量）。其余原文不动。
- **实跑结果**（v1.3 原型；每次都照 §4 表头的隔离纪律，跑后四项核对干净）：
  - 改 S1 之前（v1.2 的字节）：`go build ./...`、`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn` 干净，`gofmt -l ./internal ./cmd` 空，全量 `go test -count=1 ./...` 九包全绿（app 10.9 s、repl 86.3 s、tui 69.9 s、turn 16.1 s、config 11.7 s、store 4.2 s，其余几秒）。
  - 最终字节：build、vet 干净，gofmt 空；全量 `go test -count=1 ./...` 九包全绿（app 8.6 s、repl 76.3 s、tui 61.4 s、turn 13.1 s、config 3.1 s、store 3.1 s，其余不到 1 s）；`go test -count=3 -run TestAdn ./internal/repl ./internal/app` 全绿（repl 7.2 s、app 0.7 s）。
  - 只落块 A 加块 B-甲、只落块 A 加块 B-乙（v1.2 的冻结代码，新解出的 HEAD 上）：`go build` 都不过，报的 undefined 见块 B-甲的说明。只落块 A 全量九包全绿是 v1.2 原型核过的，块 A 的字节没变，v1.3 没有重跑。
  - 变异：§4 的 92 条全部重跑（§4 v1.3 段）。第一轮 92 条跑在【判别力】注释改写之前的字节上（与最终字节只差注释行，脚本核过），774 个杀手检查，V7、V70 仍是 0 个用例变红，其余 90 条的杀手列就是这一轮实测变红的全部用例；与 v1.2 比没有一条少了杀手，13 条多出来的都是 v1.3 的新用例。新登记的 V87-V92 在最终字节上再跑一轮，六条的点名杀手全部实测变红、与第一轮逐条相同。每条「替换前」恰好一次，从字节副本还原、SHA256 一致，每条之后四项核对干净，没有一条编译失败。
  - N25 的【判别力】另用一次性探针核过是哪一条断言红（V29 红在第一条命令的「不识别、不写、加入 -old」，V1、V17、V30、V32、V34、V37、V38 红在正对照那一次）。
  - 往返核对与编码、符号扫描：见本文末尾「往返核对（v1.3）」。

**v1.2 变更记录（2026-09-30）**：照主代理裁决书 `E:\multi_model_cowork\ad1_rulings_v12.md`（唯一权威）修订；W1b 原文 `ad1_w1b_findings.md`，v1.1 快照 `ad1_contract_v11.md`。基线仍是 HEAD `64c4b49`。原型在 `E:\Isolating_test\council\ad1_v12\`（`git archive 64c4b49` 解出，先把 v1.1 的 §1.1 与 §2 逐字落盘、build、vet、gofmt 干净、全量九包全绿，再改），交付前连同隔离目录一起删除；主仓库一个字节没写。

- **冻结代码哪些没动**（脚本逐块比过 v1.1 与 v1.2）：A1 的锚点、A2-A8、B1-B4、C1、C2 的锚点与替换文本、S2 整文件，与 v1.1 逐字节相同。所以 `cmdAddDiscovered` 与 `addDiscoveredModel` 的冻结顺序与注释要点、`resolveAddConfig` 的第 0-3 步与字面量（与 HEAD 逐字节相同）、`addSegmentNotFound` 都照旧。开关关着时行为与 HEAD 相同（U+3000 那一条除外）：v1.2 的代码改动只在 `addname.go`（只经 B2 的 `if r.addByName` 走到）与新文件 A9（只有 `addname.go` 调它），A1 只改注释；既有测试文件零改动、全绿，N1、N17 off_config 与 N9 关上开关那一条照旧绿。证据等级：按代码路径与上面这些用例判，v1.2 没有重跑 W1b 那种开关关着的差分探针。
- **生产代码改了什么**（`addname.go` 整文件、A1 的替换文本、新文件 A9；§1.1 的代码块由脚本从原型写回）：
  1. 裁决书规则二、规则三：新 D17。单个名字没给强度（没有第二个词、`--effort`，名字也不是带强度的 ID）也没给 `--id` 时，`addByOneName` 在快照里恰好对上一行之后、取强度之前调 `addNameExisting`（A9）：S 是配置里同 provider、同 model 的全部 agent（`sameModelAgents`，判据同 `configuredAgentFor`、收集全部，不按 id 前缀），去掉总结成员；S 空照 v1.1 新建；有在场的拒绝、点名；全不在场恰好一个加入它（`joinConfiguredAgent`，照 `cmdAdd` 按配置加入那一支）；不止一个列出。后三种都不写配置。这修的是 W1b 的 major：v1.1 里同一条 `/add gpt-6.1-sol` 敲两次，第二次会写配置、让同一个模型两个成员在场。
  2. 裁决书规则一（原 §7 (r)）：带 `--id` / `--effort` 的写法，名字在配置里只是前缀命中（`configPrefixOnly`）时先识别，快照里对得上就按识别结果加，对不上照旧打「对上的是配置里的 agent」（D9）。对 `--id` 与 `--effort` 一视同仁，理由与边界见 D9、§7 (w)(x)。
  3. W1b minor 1：两个词里「第二段是敲错的强度」多两条判据：不以 `+` 开头；`addSegmentNotFound(第二段)` 打的是「都没有」那一句（即不是在场成员或总结成员的前缀）。
  4. W1b minor 3：A1 的字段注释补上「splitAddArgs 按全角空格切段那一条除外，它不看开关」。
  5. 注释跟着改：`addname.go` 文件头（D17 一段）、`msgAddNameFlagsConfig`、`cmdAddByName` 顺序冻结的第 1、4 步、`addByOneName` 的顺序（加 D17 那一步）。
  6. 新增四句用户可见字面量（§3「v1.2 新增四句」）。
  - D17 起先写在 `addname.go` 与 `addmodel.go` 里，块 B 到了 479 行；第一轮变异之后挪进块 A 的新文件 A9（代码逐字搬），`addmodel.go` 回到与 v1.1 逐字节相同，块 B 419 行（D16、§7 (aa)）。
- **测试改了什么**（S1 整文件，脚本从原型写回；S2 不变；既有测试文件零改动）：
  - 期望改了的三处：N9 的 `/add grok-4.6`（裁决书「授权改动的期望」，新期望取原型实测）；N15 alias_same_brain 改敲 `/add opus[1m] --effort high`、另加 alias_configured（不带强度的 `/add opus[1m]` 在 D17 之后按规则二第 4 步必然变成加入，正是裁决书用例 5 的形状）；N14 的 `/add grok --id x` 挪进 N23 prefix_no_match（D9 的读法让它先识别，§7 (x)）。后两处裁决书没有点名授权，是执行规则一、规则二的直接后果，在这里点名。
  - 新用例：N22（D17，裁决书用例 1-5 与总结成员）、N23（D9，用例 6、7）、N24（W1b minor 1 的 H、H2、H3、C_plus_second）；W1b minor 4：N6 typo_second_row、N9 summary_prefix、N14 plus_letters。新的装置与夹具：`adnCodexSol61`、`adnBlockSol`、`adnBlock61High`、`adnTwoSolAgents`、`adnPresent`、`adnConfigured`、`adnFlagsConfig`、`adnJoinedNoWrite`。
- **裁决书逐条**：规则一 → D9（N23，V76、V84）；规则二 → D17（N22，V71-V75、V78-V80）；规则三 → D9 末条与 D17（N23 id_only_present，V77）；第四条字面量 → §3；第五条 N9 → 改了；「必须有的用例」1-7 → N22 repeat、prefix_other_model、alias_present / alias_config_present、several_absent、one_absent_config / one_absent_renamed、N23 effort_over_prefix / effort_over_other_model、id_only_present，各配 V71-V77，点名杀手实测变红；minor 1 → D7（N24，V85、V86）；minor 2 → 由规则一解决（N23 effort_over_other_model）；minor 3 → A1；minor 4 → N6、N9、N14 三个子用例（V67、V69、V68），V106 登记为 V70（等价，grok、agy 那一半标 [推断]）；not_applied (v)、(s)、(t)、(u) 留在 §7 原文不动，(r) 移进 D9（§7 留一行指向）；严重度判法写在本文开头。
- **本文定的、裁决书没写到的**：带参数压过前缀命中对 `--id` 同样生效（§7 (x)）；单段带强度的 ID 不压过配置前缀（§7 (w)）；总结成员不进 S（advisor 提的漏洞：配置第 0 步按字节比，`/add GROK-4.7` 走得到快照，不去掉就会被第 4 步当普通成员加入；§7 (y)，N22 summary_excluded，V78）；D17 的四句措辞、没有强度可选的行不带「强度」那半句。
- **契约文字**：标题；开头的标识符清单；本段与严重度判法；相交表加一行；D6、D7、D9、D12、D15、D16 改写或加段，新 D17；块 A 的标题与 A9；§1.2；§2 的开头、纪律、N6、N9、N14、N15 与新 N22-N24；§3 的复用说明与「v1.2 新增四句」；§4 的 v1.2 说明段、V26 的替换前后、V67-V86、全部 86 条的杀手列与耗时、等价与「只在审查时看」；§5 的文件、行数、字面量；§6 第 1 条（8 个文件、4 个整文件、grep 核对改成 4 行）、第 3 条（86 条）、第 5 条第 4 步加「敲两次」；§7 的 (b)、(c) 订正，(r) 改成指向，新 (w)(x)(y)(z)(aa)；文末泄漏核对与往返核对加 v1.2。(a)、(d)-(q)、(s)-(v) 原文不动。
- **实跑结果**（v1.2 原型；每次都照 §4 表头的隔离纪律，跑后四项核对干净；build、vet、gofmt、全量、`-count=3`、只落块 A 与往返核对都跑在最终字节上）：
  - `go build ./...`、`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn` 干净，`gofmt -l ./internal ./cmd` 空。
  - 全量 `go test -count=1 ./...` 九包全绿（app 17.1 s、repl 86.9 s、tui 68.6 s、turn 21.4 s、config 10.3 s、store 9.8 s，其余几秒）。
  - `go test -count=3 -run TestAdn ./internal/repl ./internal/app` 全绿（repl 8.3 s、app 0.8 s）。
  - 只落块 A（A1-A9，打在新解出的 HEAD 上）：build、vet 干净，gofmt 空，全量九包全绿（app 11.6 s、repl 81.2 s、tui 71.6 s、turn 18.4 s）。
  - §4 的 86 条变异跑了两轮（§4 的 v1.2 说明段），最后一轮在最终字节上：84 条的点名杀手全部实测变红（共 736 个杀手检查），V7、V70 登记为等价、实测 0 个用例变红；每条「替换前」恰好一次，从字节副本还原、SHA256 一致，每条之后四项核对干净。两轮杀手列逐条相同。
  - v1.2 的 S1 在 v1.1 的生产代码上跑过一次，改了行为的用例全红（§2 纪律的 v1.2 一条）。
  - 往返核对与编码、符号扫描：见本文末尾「往返核对（v1.2）」。

**v1.1 变更记录（2026-09-30）**：照主代理裁决书 `E:\multi_model_cowork\ad1_rulings.md`（唯一权威）修订；W1 原文 `ad1_w1_findings.md`，v1 快照 `ad1_contract_v1.md`。基线仍是 HEAD `64c4b49`。原型在 `E:\Isolating_test\council\ad1_v11\`（`git archive 64c4b49` 解出，先把 v1 的 §1.1 与 §2 逐字落盘、全量九包全绿，再改），交付前连同隔离目录一起删除；主仓库一个字节没写。

- **冻结代码哪些没动**（脚本逐块比过 v1 与 v1.1）：A1-A7、B1-B4、C1、C2 的锚点与替换文本、S2 整文件，与 v1 逐字节相同。所以 `cmdAddDiscovered` 与 `addDiscoveredModel` 的冻结顺序与注释要点、`resolveAddConfig` 的第 0-3 步与字面量（与 HEAD 逐字节相同）、`addSegmentNotFound` 都照旧。
- **生产代码改了什么**（`addname.go` 整文件与新增的 A8；§1.1 的代码块由脚本从原型写回）：
  1. 裁决 (d)：`idFormEffort` 逐档比完再比不带强度的 `SuggestedID("")`（D3）。副作用是 codex 的模型名改按 ID 形式对上，结果不变。
  2. 裁决 (b)：两个词的写法也认「第一段在配置里只是前缀命中」（新函数 `configPrefixOnly`，D7）。
  3. 裁决 (a)：第二段是敲错的强度时交给 `addByOneName`，打现成的「支持的强度是」、歧义候选或「已经带着强度」（新函数 `anyRowHasEfforts`，D7）。另把两个词里「几行取并集」抽成 `effortOfAny`。
  4. `/add --id x +1`：带参数写法里名字是 + 加数字时打 `msgAddModelUsage`（D9）。
  5. W1 M1 的可选项：「没命中」的判据只留一份（`miss` 闭包提到 anyMiss 循环之前，三处共用），理由见 D2。
  6. U+3000：A8，`splitAddArgs` 另按全角空格切（常量 `ideographicSpace`），不看开关（D1）。原型上全量九包全绿，没有既有用例因此变红，所以做了。
  7. 注释跟着改：`addname.go` 文件头、`SetAddByName`、`nameMatch`、`matchDiscoveredName`、`idFormEffort`、`cmdAddByName` 的顺序冻结第 1、2、4 步、`addByOneName`，以及 A8 的两段注释。没有新增用户可见字面量（§3）。
- **测试改了什么**（S1 整文件，脚本从原型写回；S2 不变；既有测试文件零改动）：W1 M1 → N5 refusal_* 四条与 N7 all_config_refusal；m1 → N7 ambiguous_seg；m2 → N10 flag_not_last；m3 → N8 no_providers；m4 → N15 claude_alias_fold、haiku_fold；m5 → N6 union_second_row（另起夹具 `adnCodexIDFirst`，没动 `adnCodexOut`）；m8 → N4 改名 ambiguous_with_effort、另加 no_efforts_codex（用 CXH 整份夹具的 `cxh-bell`）；(a) → N3 typo、upper 改期望，加 id_then_typo、ambiguous_typo、second_is_model；(b) → 新 N16；(d) → N15 claude_base 改为加入、加 claude_base_second；+ 加数字 → N14 一条；U+3000 → 新 N17。另加 N15 claude_model、claude_model_fold 与 N16 里 `XHIGH` 那一条：S1 定稿之前先跑过一批（V1-V14），V8（模型名形式不认 Model）与 V11（第二个词当强度时不分大小写）存活——(d) 之后 codex 模型名改走 ID 形式，Model 那一支没人走；(a) 之后 `XHIGH` 不论认不认作强度都打强度那一句——于是加了这三处，删掉那批结果从头跑。v1 N3 no_efforts 的期望不变（(a) 只给有强度的行）。
- **契约文字**：§0 第 2、4、5 条，相交表加 UX2R 一行，D1、D2、D3、D5、D7、D8、D9、D10、D15、D16，§2 的用例清单，§3 的复用说明，§4（表头、V4、V10、V12、V13、新 V49-V66、「只在审查时看」），§5 的行数与清单，§6 第 1 条（14 对锚点；grep 核对改成 3 行，m6）、第 3 条（66 条）、第 4 条（#49 与代码同一个提交，m7）、第 5 条（真机预期按 xhigh 存在写，(n)；两个词那一步按 (a)、(b) 写），§7 的 (a)(b)(d)(l)(n) 改写、加 (r)-(v)。(c)、(e)-(k)、(m)、(o)-(q) 原文不动。
- **实跑结果**（v1.1 原型；每次都照 §4 表头的隔离纪律，跑后四项核对干净。build、vet、gofmt、全量、`-count=3` 与往返核对跑在最终字节上；只落块 A 与块 A 加块 B 那两次跑在 S1 改那两行注释之前）：
  - `go build ./...`、`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn` 干净，`gofmt -l ./internal ./cmd` 空。
  - 全量 `go test -count=1 ./...` 九包全绿（app 8.8 s、repl 75.9 s、tui 62.0 s、turn 13.2 s、config 2.7 s、store 2.9 s，其余不到 1 s；改那两行注释之前的一次同样全绿）。
  - `go test -count=3 -run TestAdn ./internal/repl ./internal/app` 全绿（repl 5.3 s、app 0.7 s）。
  - 只落块 A（A1-A8，打在新解出的 HEAD 上）：build、vet 干净，gofmt 空，全量九包全绿（app 11.6 s、repl 74.6 s、tui 71.1 s、turn 16.4 s）。再在它上面落块 B 与 S1：build、vet 干净，`./internal/repl`（79.3 s）、`./internal/app`（8.9 s）绿。
  - §4 的 66 条变异完整跑了两轮：第一轮在 S1 的代码定稿之后、【判别力】注释改写之前；最后一轮在最终代码上（驱动脚本从本文 §4 解析替换前后；之后 S1 只改过两行注释，见 §4 表头）。两轮的杀手列逐条相同；§4 写的是最后一轮。65 条被点名杀手杀掉，V7 登记为等价、实测存活。每条「替换前」在文件里恰好一次，从字节副本还原、SHA256 一致，每条之后四项核对干净。第一轮里 V58 最初的写法编译不过（`typo` 声明了不用），改成 `|| typo && false` 后重跑；两轮用的都是改后的写法。最后一轮之后又把 V60、V61 的替换前后改成不以空格结尾的写法（语义不变，免得编辑器删掉行尾空格就改了变异），这两条按新写法单独重跑，杀手与前两轮相同。
  - 往返核对与编码、符号扫描：见本文末尾「往返核对（v1.1）」。

**起草记录（v1，原型实跑）**：

- 隔离目录 `E:\Isolating_test\council\ad1_draft\`：`git -C <仓库> archive 64c4b49` 解出多份（原型 `proto\`、HEAD 对照 `head\`、只落块 A 再落块 B 的 `blockA\`，以及每一次往返核对各一份新解出的），各 367 个文件；主仓库一个字节没写。原型不交付，交付前连同隔离目录一起删除。
- 每一次 go build / vet / test / 变异运行都在同一条 PowerShell 命令里先 dot-source 隔离环境（§4 表头），每次 go test 之后核四项，次次干净（§7 末尾「泄漏核对」）。
- 原型上：`go build ./...` 干净；`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn` 干净；`gofmt -l ./internal ./cmd` 空。
- 原型上全量 `go test -count=1 ./...` 九包全绿（最后一次，跑在最终字节上：app 10.6 s、repl 76.5 s、tui 62.2 s、turn 15.1 s、config 3.7 s，其余几秒）。**既有测试文件一个字节没改**，全绿。
- `-run 'TestAdn'`（repl、app）`-count=3` 全绿（repl 4.3 s、app 0.7 s）。
- 只落块 A（`blockA\`，A1-A7 打在新解出的 HEAD 上）：build、vet 干净，gofmt 空，全量九包全绿（app 11.6 s、repl 74.5 s、tui 69.8 s、turn 16.7 s）。再在它上面落块 B 与 S1（不落块 C、不落 S2）：build、vet 干净，`./internal/repl`（77.5 s）、`./internal/app`（9.8 s）绿。
- §4 的 48 条变异逐条在原型上跑：47 条被点名杀手杀掉，V7 登记为等价、实测存活。每条从字节副本还原、SHA256 一致。
- 往返核对：§1.1 的冻结代码、§2 的两个测试文件由脚本从原型取出写进本文；锚点由脚本从新解出的 HEAD 逐行取出，核过各恰好一次。再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在又一份新解出的 `64c4b49` 上，与原型逐字节比（结果写在本文末尾「往返核对」一节）。

**与已落地各刀的相交**（逐个点名；本刀对它们的代码一个字不改，除非下表写了「改」）：

| 已落地 | 相交的文件 / 函数 | 本刀怎么用 |
|---|---|---|
| MD1（16a）| `internal/repl/addmodel.go`：`modelSnapshot`、`snapRow`（`no` 为 0 的行）、`takeModelSnapshot`、`printDiscoveredModels`、`cmdAddDiscovered`、`configuredAgentFor`、`sameBrainAgent` | **改**（A2-A7）：`printDiscoveredModels` 的「没有快照就识别、取消就不存」抽成 `ensureModelSnapshot`，按名字加与它共用；`cmdAddDiscovered` 从强度起的写配置流程抽成 `addDiscoveredModel`，按名字加与它共用。冻结顺序与注释逐条搬（§1.1「注释对账」）。快照格式、编号规则、识别表、失败行、`--refresh` 提示一个字不改 |
| MD1 测试 | `mdx_addmodel_fixture_test.go`（`mdxNewRig`、`mdxNewRigOpts`、`mdxRigOpts`、`mdxRig`、`mdxResp`、`mdxWantLines`、`mdxHasLine`、`mdxReplaceOnce`、`mdxConfigTemplate`、`mdxGrokFixture`、`mdxGrokPath`）| 只读复用。它们的 REPL 由 `New` 直接造出、开关关着，输出一个字节不变（D1） |
| CXH（18a）| codex 的识别行（`Model` 是 slug、`Alias` 空、`Efforts` 按模型收窄）；`cxh_fixture_test.go`（`cxhNewRig`、`cxhConfigTemplate`、`cxhCodexPath`）| 只读复用。按名字加对 codex 行与 claude 行一视同仁；`custom/` 行不进快照，照旧加不了 |
| CH1（19a）的 `resolveAddSegment` 规则 | `internal/repl/commands.go`：`cmdAdd`、`resolveAddSegment` 第 0-3 步（总结成员、全名、编号、候选前缀）与「没找到」的三句 | **改**（B1-B4）：第 0-3 步原样挪进 `resolveAddConfig`（一行不改），「没找到」三句原样挪进 `addSegmentNotFound`；开关打开时最后一句换成新的一句（D13）。开关关着时两个函数拼起来与 HEAD 的 `resolveAddSegment` 逐字节同义（全量绿） |
| CH1、UX2R 的按配置加入（v1.2）| `internal/repl/commands.go`：`cmdAdd` 末尾逐个 Join 的那一支（`:976-991`，`conversation.Participant` 的写法与「%s 已加入会话 %s」）；`addmodel.go`：`configuredAgentFor`、`sameBrainAgent` 的「同一个模型」判据 | 不改。D17 第 4 步照那一支的写法另写 `joinConfiguredAgent`（A9，不抽共用函数：抽出来要改 `cmdAdd` 的循环，那里 `reportErr` 的无条件 return 是故意的）；D17 的 S 照 `configuredAgentFor` 的判据另写 `sameModelAgents`（A9，收集全部），两个既有函数一个字不改 |
| UX2R（`/add` 一次加多个）| `internal/repl/commands.go`：`splitAddArgs`（ASCII 空白、半角逗号、全角逗号切段）、`fullwidthComma` | **改**（A8，v1.1）：另按全角空格 U+3000 切（常量 `ideographicSpace`，写法照 `fullwidthComma`）。不看开关；`parseAddNameArgs` 也经它切名字 |
| OB1（19d）| `printAddCandidates` 的四种候选为空的说法；`resolveAddSegment` 里总结成员前缀那两句 | 不改。那两句留在 `addSegmentNotFound`，排在识别快照之后（D6） |
| OB3（19f）| `modelSnapshot.logins`、`printLoginStatus`、`SetLoginStatusExec` | 不改。按名字加自动识别时登录状态照样拍进快照，但不打登录段（D5）；之后不带参数的 `/add` 照这份快照打（S1 的 N11）。`SetAddByName` 照 `SetLoginStatusExec` 的写法 |
| OB1 | `internal/repl/cliadvice.go`（`SetCLIDetector` 的先例）| 不改 |
| OB1、OB3 | `internal/app/app.go` 装配（`:251-264`）| **改**（C1、C2）：TUI 与行式两处各加一行 `r.SetAddByName(true)` |
| OB1、OB3 测试 | `internal/app/obx_firstrun_app_test.go` §2.0 装置、`cox_app_test.go` | 只读复用（S2） |
| TestMain | app 的在 `smoke_test.go`，repl 有 | 不改、不另加 |

---

## 0. 动机 / 实测

1. **现状（HEAD `64c4b49`，核过真文件）**：
   - `cmdAdd`（`internal/repl/commands.go:883`）只有三种走法：第一个参数以 `+` 开头交给 `cmdAddDiscovered`（`addmodel.go:319`）；`--refresh` 或不带参数时列候选与识别表；其余参数按 `splitAddArgs` 切段，每段交给 `resolveAddSegment`（`:1058`），它只在配置里找（总结成员、全名、编号、候选前缀），找不到就是「配置里没有可加入的 %s，不带参数的 /add 能列出可加的」（`:1128`）。
   - 识别表（`printModelSnapshot`，`addmodel.go:206-258`）的 ID 列是 `SuggestedID(DefaultEffort())`，表下的提示只说「用 /add +编号 加入」。ID 列的名字本身在 HEAD 上是敲不进去的。
   - 拍快照时已在配置里的行 `no` 为 0（`takeModelSnapshot`，`:160-163`），识别表不显示、没有 +N，换强度只能手改配置（`configuredAgentFor` 的注释，`:79-86`；16a open item (c)）。
2. **Griffin 这台机器上的事实**：`codex debug models` 列出了 `gpt-6.1-sol`（主代理实测，任务书转述）。v1.1：主代理 2026-09-30 在 E 盘机上又跑了一次 `codex debug models`（零花费，裁决书转述，本刀没有重跑任何真 CLI）：`gpt-6.1-sol` 的 `supported_reasoning_levels` 是 low、medium、high、xhigh、max、ultra 六档，`default_reasoning_level` 是 medium。所以 xhigh 存在（§7 (n) 关闭）。council 的默认档取 `DefaultEffort()`（清单里有 high 就取 high），不取 codex 的 medium：按名字加不带强度时得到 `gpt-6.1-sol-high`，与 +编号 相同。本文的 codex 夹具 `adnCodexOut` 仍是合成的：`gpt-6.1-sol` 取他敲的名字，只给了 `low|medium|high|xhigh` 四档（夹具不跟真机改，免得动既有期望；各条用例不依赖 max、ultra 两档）。
3. **HEAD 上这几条命令的输出**（原型实跑：在新解出的 HEAD 上用 CXH 的配置（codex、grok 两家，`grok-4.7` 与 `gpt-5.5-high` 已配成 agent）与 `adnCodexOut` 敲，逐字）：

   ```text
   /add gpt-6.1-sol xigh
   配置里没有可加入的 gpt-6.1-sol，不带参数的 /add 能列出可加的
   配置里没有可加入的 xigh，不带参数的 /add 能列出可加的

   /add gpt-6.1-sol xhigh
   配置里没有可加入的 gpt-6.1-sol，不带参数的 /add 能列出可加的
   配置里没有可加入的 xhigh，不带参数的 /add 能列出可加的

   /add gpt-6.1-sol-xhigh
   配置里没有可加入的 gpt-6.1-sol-xhigh，不带参数的 /add 能列出可加的

   /add gpt-6.1-sol-high
   配置里没有可加入的 gpt-6.1-sol-high，不带参数的 /add 能列出可加的

   /add gpt-6.1-sol --effort xhigh
   配置里没有可加入的 gpt-6.1-sol，不带参数的 /add 能列出可加的
   配置里没有可加入的 --effort，不带参数的 /add 能列出可加的
   配置里没有可加入的 xhigh，不带参数的 /add 能列出可加的
   ```

   之后同一会话敲不带参数的 `/add`，识别表第一行正是 `gpt-6.1-sol-high`（识别部分逐字）：

   ```text
   正在识别模型: codex, grok
   识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
   NO  PROVIDER  ID                      MODEL                   EFFORTS                SOURCE
   +1  codex     gpt-6.1-sol-high        gpt-6.1-sol             low|medium|high|xhigh  codex 目录
   +2  codex     adn-x-high              adn-x                   high                   codex 目录
   +3  codex     adn-x-high              adn-x-high              -                      codex 目录
   +4  grok      grok-4.6                grok-4.6                -                      本地配置
   +5  grok      grok-4.5                grok-4.5                -                      本地配置
   +6  grok      grok-4.3                grok-4.3                -                      本地配置
   +7  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -                      本地配置
   +8  grok      grok-build              grok-build              -                      本地配置
   用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
   codex 目录里还有 1 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员
   ```

   也就是说：表里写着 `gpt-6.1-sol-high`，照着敲 `/add gpt-6.1-sol-high` 却被说「配置里没有」。`gpt-5.5` 已配成 `gpt-5.5-high`，表里根本没有它，`/add gpt-5.5-xhigh` 在 HEAD 上无路可走。
4. **原型上同样几条**（开关打开，新会话里第一次就敲，要自动识别；逐字，`<cfg>`、`<bak>`、`<conv>` 是配置路径、备份路径、会话 id）：

   ```text
   /add gpt-6.1-sol-xhigh
   正在识别模型: codex, grok
   已把 agent gpt-6.1-sol-xhigh 追加进 <cfg>（原文件备份在 <bak>）
   gpt-6.1-sol-xhigh 已加入会话 <conv>

   /add gpt-6.1-sol xhigh
   （与上一条逐字相同，写进文件的字节也相同：S1 的 N2 id、N3 model_effort）

   /add gpt-6.1-sol-high
   正在识别模型: codex, grok
   已把 agent gpt-6.1-sol-high 追加进 <cfg>（原文件备份在 <bak>）
   gpt-6.1-sol-high 已加入会话 <conv>

   /add gpt-6.1-sol xigh
   正在识别模型: codex, grok
   gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 xigh
   ```

   （`xigh` 那一条是 v1.1 按裁决 (a) 改过的结果，S1 的 N3 typo 钉着；v1 打的是「gpt-6.1-sol 是识别出的模型，识别出的模型一次加一个：单独 /add gpt-6.1-sol」与「配置里和识别结果里都没有 xigh；……」两行。强度那一句是 +编号 那条路现成的字面量，列的是夹具的四档；真机上是六档。）

   追加的那一段（`gpt-6.1-sol-xhigh`）与 `/add +1 --effort xhigh` 写出的逐字节相同（N2 plus）：

   ```text

   # council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）
   [agents."gpt-6.1-sol-xhigh"]
   display_name = "GPT-6.1-Sol xhigh"
   provider = "codex"
   model = "gpt-6.1-sol"
   effort = "xhigh"
   role = ""
   permissions = "read"
   mode = "discussion"
   ```

   `xigh` 那一条在 v1 是「一次加一个」加「都没有 xigh」两行，用户体验最差的正是他可能真敲的那一条（v1 的 §7 (a)）；v1.1 按裁决 (a) 改成上面那一行（D7）。
5. **既有测试**：原型只套生产代码（块 A、B、C）、不加新测试时，全量九包全绿；加上 S1、S2 仍全绿。**本刀不改任何既有测试**（`existing_tests_changed` 为空）。v1.1 的 A8（全角空格切段）不看开关，原型上全量九包照样全绿，没有既有用例因此变红（裁决书「U+3000」一条的前提成立，所以做了）。开关零值是关，直接 `New` 出来的 REPL 输出不变（D1）；app 装配打开开关之后，app 包里既有用例敲的 `/add`、`/join` 全是配置里的 agent（`/join c1` 这类，逐个核过），在配置里命中，不经按名字加（D2）。
6. 没有调用任何真 provider CLI、没有向任何模型提问。测试里的「codex」「grok」「claude」「agy」一律是注入的假执行器（repl 包）或临时目录里只用 cmd 内建命令的 `.cmd`（app 包）。

---

## 1. 范围

### 1.1 做

#### 裁定（主代理定的方向，起草人冻结的细节；逐条冻结）

- **D1 开关。** 照 `SetCLIDetector`、`SetLoginStatusExec` 的先例：`REPL` 加字段 `addByName bool`（A1）与 setter `SetAddByName(on bool)`（`addname.go`），只许在 `Run` 或第一次 `Submit` 之前调。**零值 false 是关：`/add` 不按名字识别、不多起任何进程，说法与 HEAD 逐字节相同**（`addSegmentNotFound` 的最后一句只在开关打开时换）。v1.1 唯一的例外是 A8：`splitAddArgs` 按全角空格 U+3000 切段，不看开关（裁决书「U+3000」一条；N17 off_config 钉着关着时也生效）。所以直接 `New` 出来的 REPL（MD1、CXH、OB1、OB3、ux2r、chx、tui 包等既有用例）行为不变。app 在 TUI 与行式两处装配都打开（C1、C2）。`/join` 与 `/add` 是同一个 handler（`commands.go:159-160`），同样生效。
- **D2 什么时候插手。** 开关打开、`/add` 带参数、第一个词不以 `+` 开头、也不是 `--refresh` 时，`cmdAdd` 在切段之后先调 `cmdAddByName`（B2）：
  - fields（按空白切的词）里有恰好等于 `--id` 或 `--effort` 的词：按名字加的参数形式（D9）。
  - 否则逐段调 `resolveAddConfig`（配置里的第 0-3 步）。**「命中」的定义**：某一步返回了 id（加入）或拒绝的那一行；总结成员、只是成员不在配置里、全是 ASCII 数字（含超出范围）、候选前缀唯一、候选前缀歧义，都算命中。**「没命中」只有一种**：第 3 步的前缀在候选里一个都没找到（`resolveAgentRef` 的 `Matches` 为空）。`resolveAddConfig` 用两个返回值都是空串表示没命中（命中时恰好一个非空）。v1.1：`cmdAddByName` 里「没命中」只有一个判据 `miss`（两个返回值都是空串），anyMiss、两个词、多段三处都用它（裁决书 M1 的可选项：两份拷贝抽成一处，理由是一个判据只该有一个写法；每个调用处仍各有变异与杀手，V49、V50，判据本身是 V51）。
  - 每一段都命中：`cmdAddByName` 返回 done 为假，`cmdAdd` 照 HEAD 的循环接着解析，行为一个字节不变（不识别、不起进程）。N5 prefix、N7 all_config 钉住（V29）；v1.1 另有开关打开时的拒绝型命中：N5 的 refusal_out_of_range、refusal_ambiguous、refusal_summary、refusal_member_only 与 N7 的 all_config_refusal，每条一个新装置，断言整行说法、执行器 0 次、没有「正在识别模型」，同一装置接着按名字加是正对照（V49、V51）。
  - 有段没命中：接 D5、D7、D8。
- **D3 两种名字形式**（`matchDiscoveredName`、`idFormEffort`）。在识别快照的**全部行**里找（`snap.rows`，含 `no` 为 0、拍快照时已在配置里的行，这就是 16a open item (c) 的解法；`custom/` 行本来就不进快照），按快照顺序，每一行至多对上一次：
  - **ID 形式**先比：有强度的行按 `Efforts` 的顺序逐档比 `SuggestedID(档)`，取第一个对上的，这一档就是名字里带的强度；都对不上再比不带强度的 `SuggestedID("")`（v1.1，裁决 (d)），对上时带的强度是空串。没有强度可选的行（grok、agy、claude 的 haiku、codex 的 `cxh-bell` 这类）只比 `SuggestedID("")`。
  - **模型名形式**后比：等于 `Model`，或 `Alias` 非空且等于 `Alias`（claude 的 value，例如 `sonnet[1m]`；agy 的 id）。
  - 带的强度是空串时（模型名形式，或不带强度的 ID 形式），强度取 `--effort` 或两个词写法的第二个词，都没有取 `DefaultEffort()`（与 +编号 相同）。所以 claude 的 `/add sonnet-5` 加 `sonnet-5-high`（N15 claude_base），`/add sonnet-5 xhigh` 加 `sonnet-5-xhigh`（claude_base_second）。v1 里这一种两种形式都不是、打「都没有」。
  - codex 的 `SuggestedID("")` 是规整后的 slug，一般就等于 `Model`（slug 被规整或加了 `codex-` 前缀的除外，例如 CXH 夹具的 `claude-cxh-9`），所以 codex 的模型名多半按不带强度的 ID 形式对上；强度同样另取，结果与模型名形式相同，不会多出一行匹配（每行至多对上一次，ID 形式对上就 `continue`）。模型名形式里 `Model` 那一支因此只剩 claude（与 codex 规整过的 slug）走得到，由 N15 的 claude_model、claude_model_fold 担保（V8、V2）。
- **D4 大小写。** 名字按 `strings.EqualFold` 比（ID 与模型名两种形式都是）；写进配置的 id 是 `SuggestedID`（或 `--id`），不是敲的原样（N2 fold：`/add GPT-6.1-Sol-XHIGH` 写的是 `gpt-6.1-sol-xhigh`）。**强度一律区分大小写**：`--effort` 走 `addDiscoveredModel` 与 +编号 同一个判据（CXH 的 R4 钉着 `--effort HIGH` 被拒，N4 effort_case 同样）；两个词写法的第二个词也按字节比（`hasEffort`，N3 upper：`XHIGH` 不是一档强度）。配置里的解析（第 0-3 步）照旧按字节比（`GPT-5.5-HIGH` 在配置里对不上，到快照里对上，N5 same_brain）。`EqualFold` 按 Go 的 Unicode 简单大小写折叠，另会把少数非 ASCII 字母当成 ASCII 字母的大小写（§7 (i)）。
- **D5 快照。** 有段没命中时，本会话还没有快照、配置里又有能识别的 provider，就先识别一次（`snapshotForName` 调 `ensureModelSnapshot`）：打「正在识别模型: …」、同一个 `takeModelSnapshot`、同一个 `discoverTimeout`、同一套取消语义，拍下的就是不带参数的 `/add` 用的那一份快照（N11）。OB3 的登录状态照样拍进快照，**这里不打登录段**（V33），也**不打识别表、custom 行、失败行与 `--refresh` 提示**；它们在不带参数的 `/add` 里照快照打。识别被取消：只打「识别被取消；下次 /add 会重新识别」那一行，不存快照，什么都不加（N10，V42；带 `--id`/`--effort` 那条路同样，v1.1 的 N10 flag_not_last，V53）。已有快照直接用，不重拍（N8、N11，V31）；`/add --refresh` 才重拍。配置里一家能识别的都没有、又没有快照时不识别，快照留 nil，按名字一个都对不上（v1.1 的 N8 no_providers，V54）。
- **D6 优先级。** 配置里的解析（第 0-3 步）一律优先、行为不变（D2）。例外只有「在配置里只是前缀命中」（`configPrefixOnly`）的两处：两个词的写法（v1.1，裁决 (b)，D7）与带 `--id` / `--effort` 的写法（v1.2，裁决书规则一，D9），名字在快照里对得上时按识别结果加；全名、编号、总结成员、只是成员、前缀歧义仍然优先。单段不带参数的前缀命中照旧加入配置里的那个 agent、不识别（裁决书规则二）。识别快照里的精确命中排在「X 开头的都已经在当前会话里了」与总结成员前缀那两句之前：那两句是「没找到」的说法，现在在 `addSegmentNotFound` 里，只在快照里也对不上时才打（N9，V34；开关打开时总结成员前缀那一句由 N9 summary_prefix 钉着，V69）。v1.2：快照里对上一行之后，单个名字没给强度也没给 `--id` 时先过 D17 那道门，所以 `grok-4.6-a` 在场时 `/add grok-4.6` 打 D17 的「grok-4.6 对上的模型已经有成员在当前会话里：grok-4.6-a」，不再是 v1.1 的同脑子那一句（N9，裁决书「授权改动的期望」）。数字段仍是配置候选的编号；识别表的编号仍只用 `+N`（`cmdAddDiscovered`，不改）。
- **D7 两个词的写法。** 恰好两段、第二段没命中配置、第一段在快照里至少对上一行，而且第一段满足下面两种之一：
  - **没命中配置**（v1 就有）；
  - **在配置里只是前缀命中**（v1.1，裁决 (b)，`configPrefixOnly`）：`resolveAddConfig` 给出了 id，但不是全名（全名命中时 id 等于这一段）、这一段也不全是数字（编号走第 2 步）；总结成员、只是成员、前缀歧义、超出范围的编号都走 problem，不算。这样判是因为 `resolveAddConfig` 的第 0-3 步与字面量要与 HEAD 逐字节相同，不能给它加返回值。

  在此之上：
  - 第二段恰好是对上的某一行支持的一档强度（按字节比，对上多行时取这几行的并集，`effortOfAny`）：第二段当强度，按一个名字加（N3 model_effort）。第一段在配置里只是前缀命中时同样如此：同一模型同一强度已在配置里，打同脑子那一句、不写（N16 `/add gpt-6.1-sol xhigh`）；强度不同就新建 agent、写配置（N16 `/add gpt-6.1-sol high`）。单段 `/add gpt-6.1-sol`（只有前缀命中）仍按配置前缀加入已有的 agent，**不变**（N16 最后一条）：用户没说强度时，加入已有的成员比悄悄写配置更安全。v1.2：这个成员在场时它不是候选，配置里没命中，到快照里对上之后由 D17 拒绝、不写配置（N22 repeat），所以同一条命令敲两次，第一次加入、第二次说已在会话里。
  - **第二段是敲错的强度**（v1.1，裁决 (a)；v1.2 按 W1b minor 1 加了后两条）：第一段没命中配置（前缀命中的不算）、对上的行里至少一行有强度可选（`anyRowHasEfforts`）、第二段不是其中任何一行的一档强度、第二段在快照里也对不上任何行、第二段不以 `+` 开头、而且 `addSegmentNotFound(第二段)` 打的是 D13 的「都没有」那一句（也就是第二段既不是在场成员的前缀、也不是总结成员的前缀；它们指的是配置里的人，照多段说，N24，V85、V86）：同样按一个名字交给 `addByOneName`，由它打现成的说法，不写配置——对上一行、按模型名形式（或不带强度的 ID 形式）时是 `addDiscoveredModel` 强度那一步的「<model> 支持的强度是 <档|档>，没有 <词>」（逐字复用 +编号 那条路的字面量，N3 typo、upper）；对上多行时是 D11 的歧义候选（ambiguous_typo）；第一段是带强度的 ID 形式时是 D10 的「已经带着强度」（id_then_typo）。第二段在快照里对得上模型时照 D8「一次加一个」（second_is_model）。只限有强度可选的行，理由是那一句只对这种行成立：grok 这类没有强度的行走 `addDiscoveredModel` 会打「不能带 --effort」，而用户没敲 `--effort`，所以 `/add grok-4.6 high` 照旧按多段（N3 no_efforts）。
  - 第一段是带强度的 ID 形式、第二段又是一档强度：报错，**相同也报错**（N3 id_then_effort；两个词的写法只给模型名形式与不带强度的 ID 形式用）。

  其余情形照旧按多段解析：第二段是配置候选的编号或前缀（N3 digit、effort_is_config_prefix）、第一段在配置里按全名命中（config_first）、第一段在配置里只是前缀命中而第二段不是一档强度（N16 `/add gpt-6.1-sol xigh`、`XHIGH`：只打「都没有」那一句）、第二段在快照里是一个模型（second_is_model）、对上的行都没有强度可选而第二段又不是一档强度（no_efforts）。两个词再加 `--id` 或 `--effort` 是四个以上的词，名字不止一个，按 D9 打用法（N14）。
- **D8 一次只加一个识别出的模型。** 多段（两段以上、不是 D7 的写法）里只要有一段在快照里对上（一行或多行都算；多行那一种 v1.1 由 N7 ambiguous_seg 担保，V52），就整条拒绝：一个都不加、不写配置、不 Join 任何人（全或无，配置里命中的那些段也不加）。逐段按出现顺序各打一行：配置里命中的照配置的说法（加入成功的那种不打），快照里对上的打「<段> 是识别出的模型，识别出的模型一次加一个：单独 /add <段>」，都对不上的打 `addSegmentNotFound`（N7）。多段里一段都没在快照里对上时，照同一个循环打各段的说法（那些没命中的段是新的一句）。
- **D9 `--id` 与 `--effort`。** fields 里有恰好等于这两个词之一的词，就是按名字加的参数形式（`parseAddNameArgs`）：每个参数至多一次、后面要有值；其余的词都是名字，按 `splitAddArgs` 再切，**切完恰好一段**，否则打用法（`/add a,b --effort x`、`/add a b --effort x`、`/add --effort x`、`/add x --id a --id b`，N14）。参数可以在名字前（N4 flag_first）。v1.1：名字是 `+` 加 ASCII 数字（`/add --id x +1`）时打既有的 `msgAddModelUsage`（「用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别」，意思是 +编号 要放在第一个词），不打「都没有 +1」，不识别（N14，V65）。只认 `+` 加数字，照裁决书的字面；`/add --id x +abc` 照旧落到「都没有 +abc」（§7 (u)）。名字在配置里命中一句拒绝时不识别，打那一句（`/add 99 --id z`）；按全名或编号命中一个 agent 时不识别，打「<名字> 对上的是配置里的 agent <id>，--id 与 --effort 只用于加识别出的模型；直接 /add <id>」（N14）。v1.3：编号命中不算下面那一条的「只是前缀命中」（`configPrefixOnly` 的 `!isASCIIDigits`），`/add 1 --effort high` 不识别、直接打这一句（N14 的一条，V92，W1c 的 W7）。没命中接 D5 与 `addByOneName`。
  - **v1.2（裁决书规则一，原 §7 (r)）：名字在配置里只是前缀命中**（`configPrefixOnly`，判法同 D7）时先识别（D5），名字在快照里对得上（一行或多行）就交给 `addByOneName`，按识别结果加（多行照 D11 列候选）；对不上照旧打上面那一句，只是先识别了一次（N23 prefix_no_match）。例：配置里有不在场的 `gpt-6.1-sol-xhigh`（同一个模型），或者有另一个模型的 `gpt-6.1-sol-mini`，`/add gpt-6.1-sol --effort high` 都新建 `gpt-6.1-sol-high`（N23 effort_over_prefix、effort_over_other_model，V76）。v1.1 在这里打「对上的是配置里的 agent gpt-6.1-sol-xhigh……」，指向一个用户没要的强度，或另一个模型。
  - 这一条对 `--id` 与 `--effort` 一视同仁（本文定的边界，裁决书规则一的标题只列了三种「明确的强度」）：规则一说「扩到带参数的写法」，带参数的写法就是本条（`--id` 或 `--effort`）；规则三又说只有 `--id` 时算作明确要新建。所以只有 `--id` 时同样按识别结果加（N23 id_over_prefix）。代价：`/add grok --id x`（grok 是 grok-4.7 的前缀、快照里对不上）现在先识别、再打那一句，v1.1 是不识别直接打（N14 的这一条因此挪进 N23 prefix_no_match，§7 (x)）。
  - 单段不带参数的前缀命中不在此列：要知道单段名字是不是带强度的 ID，得先识别，这正是裁决书规则二不肯让每条 `/add op` 多等的那一次（§7 (w)）。
  - **只有 `--id`、没有 `--effort` 时**（裁决书规则三）算作明确要新建：`addByOneName` 跳过 D17 那道门，按 `DefaultEffort()` 新建、写配置；同脑子检查照旧（同一模型同一强度已在配置里就打同脑子那一句）。所以这个模型已有在场成员时，`/add gpt-6.1-sol --id sol` 照样新建 `sol`（N23 id_only_present，V77）。
  - 与 HEAD 的差别：HEAD 把 `--effort` 当成一段、永远报「配置里没有可加入的 --effort」、一个都不加；所以带这两个词的命令在 HEAD 上只有报错这一种结局，本刀只改了报错的说法与「现在能加了」。
- **D10 强度对账**（`addByOneName`）。ID 形式带了强度（非空）时：给了不同的 `--effort` 报错；给了相同的照常（N4 id_same_effort，V15）；后面跟了第二个词（D7，一档强度或敲错的强度）报错。报错一行：「<名字> 已经带着强度 <名字里的>，不能再给强度 <另给的>」。ID 形式没带强度（没有强度可选的行）时 `--effort` 原样交给 `addDiscoveredModel`，由它照 +编号 的说法拒绝（grok、agy 那一句，或「<model> 没有可选的强度，不能带 --effort」，N4 grok_effort、N15 haiku_effort）。
- **D11 歧义。** 一个名字在快照里对上多行（不同 provider，或同一 provider 的不同模型）时整条拒绝、不猜，先打「<名字> 对上了识别出的 <N> 个模型，不猜；用 +编号 或带强度的 ID 指明:」，再按快照顺序逐行打「  <provider 左对齐 6 格>  <Model>」，有编号的行尾加「  +<N>」，没有编号的不加（N6；V9、V36）。
- **D12 写配置流程只有一份**（`addDiscoveredModel`，A6、A7）：`cmdAddDiscovered` 从强度起的全部代码原样搬进 `addDiscoveredModel`（函数体一行不改，参数名 `m`、`tok` 照旧），+编号 与按名字加都调它：强度、同脑子、id、配置文件路径、守卫（先 data_dir 与 rules_file，再整份比）、追加并备份、加载校验、换指针、Join。屏幕输出与写进文件的字节相同（N2 plus 与 id 同一份期望）。
  - `tok` 是守卫拒绝时「再 /add <tok>」里的那一段：+编号 是 `+N` 那个词（HEAD 原样）；按名字加是敲的名字（第一个词），同样不带 `--id`、`--effort` 与第二个词（N12，V20）。
  - Runner 的判断：+编号 照 HEAD 在参数形状之后、快照之前；按名字加放在「快照里恰好对上一行、强度对账过了」之后、`addDiscoveredModel` 之前（N13，V19）。v1.2 的 D17 那道门排在强度对账与 Runner 之前：它的三种结局都不写配置，加入走的是与 `cmdAdd` 按配置加入同一种 Join，那一支本来也不判 Runner。app 里 svc 在 Runner 就在（`app.go:193-199`），只有测试会走到这一句。
  - 新成员照 #45：permissions read、mode discussion、role 空串；display_name 与 id 取 provider 包的建议，id 可用 `--id` 改。不加新的权限默认值。
- **D13 都对不上时的说法。** 开关打开时，配置里与识别快照里都对不上、也不是「开头的都已经在」与总结成员前缀那两句时，打「配置里和识别结果里都没有 <段>；不带参数的 /add 列出配置里的成员与识别出的模型，识别表里的模型可以直接 /add 它的 ID」。识别有失败的那一家时说法不变（N8 codex_failed）；配置里一家能识别的都没有时也是这一句（§7 (g)）。开关关着时是 HEAD 的旧句（N1，V40）。
- **D14 `/help` 不改。** 命令表里 `add` 的 summary（`commands.go:159`）被两处既有测试逐字钉着：`cxh_fixture_test.go:223` 的 `cxhHelpAddLine`（`cxh_addmodel_test.go:222` 查它）与 `ux2r_add_test.go:448` 的 `wantAddSummary`。改它要授权改这两处；本刀的目标是既有测试零改动，而且新的「都对不上」那一句已经指了路。不改，列 §7 (e)，附一版可用的措辞。
- **D15 同类的还有没有别处**（逐个核过）：
  - 往 `r.cfg.Agents` 追加的路：只有 `addDiscoveredModel`（本刀合成一份）。`/team use` 只加配置里已有的 team 成员，不识别，不动。
  - 触发识别的路：`printDiscoveredModels`（不带参数、`--refresh`）与 `snapshotForName`（本刀），都经 `ensureModelSnapshot`；首次运行（`app/firstrun.go` 的 `firstRunCheck`）有它自己的识别，不经快照，不动。
  - 打「配置里没有可加入的」的地方：只有 `addSegmentNotFound`（原 `resolveAddSegment` 的末尾）。
  - 注释里说「只能手改配置」「只在不带参数的 /add 时识别」的：`addmodel.go` 文件头（A2）、`configuredAgentFor`（A3）、`takeModelSnapshot` 指向 `printDiscoveredModels` 的那一句（A4），都改了。
  - 只说「/add +编号」、今天仍然成立的：`configcmd.go:5-7`（新增 agent 的两条路）、`provider/codex.go:51-53`（强度按模型收窄）、`config/append.go:13`、`config/defaults.go:68`、`app/firstrun.go:177`（首次运行的上手说明，屏幕输出）、`printModelSnapshot` 表下那一行（任务书冻结不改）、守卫里 data_dir 那一句（「重启之后先敲 /add 看编号，再用 /add +编号 加入」，按名字加走到这里也照打，与 +编号 相同）。它们说的是真的，只是没提按名字加；不改，列 §7 (k)。
  - HEAD 的一处过时引用：`printDiscoveredModels` 注释里的 `tui/model.go:341-347` 在 HEAD 上指的是窗口尺寸那一支（`:336-343`），忙时按 Ctrl-C 调 `m.cancel` 的在 `:396-406`。这一段注释搬到 `ensureModelSnapshot` 时改成按名字指（「tui/model.go 里忙时按 Ctrl-C 那一支调 m.cancel」），不再写行号。
  - v1.1 另核的几处：
    - 「配置里只是前缀命中、却想按识别结果换一档」这个坑（裁决 (b)）还有一处：带参数的写法 `/add gpt-6.1-sol --effort high`，在 `gpt-6.1-sol-xhigh` 已在配置里、不在场时，名字按前缀命中，照 D9 打「gpt-6.1-sol 对上的是配置里的 agent gpt-6.1-sol-xhigh，--id 与 --effort 只用于加识别出的模型；直接 /add gpt-6.1-sol-xhigh」。裁决书只改了两个词那一支，这一处**不改**，列 §7 (r)。（v1.2：已按裁决书规则一改，见 D9 的 v1.2 一条。）
    - 按段切名字的地方：`cmdAdd` 的 `segs` 与 `parseAddNameArgs` 都经 `splitAddArgs`，A8 对两处同时生效；`args.fields` 用 `strings.Fields`，本来就在 U+3000 上切（N17 on_flag 实测）。别的命令不经 `splitAddArgs`，不受影响。
    - 打 +编号 用法（`msgAddModelUsage`）的地方：`cmdAdd` 的 `--refresh` 带多余参数、`cmdAddDiscovered` 的参数形状，以及 v1.1 的 D9（带参数写法里名字是 + 加数字）。
    - 「<model> 支持的强度是 …，没有 …」只有 `addDiscoveredModel` 一处；裁决 (a) 经 `addByOneName` 走到它，不另留一份。
  - v1.2 另核的几处：
    - 「只是前缀命中」的判据 `configPrefixOnly` 现在有两个调用处：两个词（D7）与带参数（D9）；单段不带参数的那一处（`cmdAdd` 照 HEAD 的循环）不用它，理由见 D9 与 §7 (w)。
    - 「同一个模型」的判据（provider 相同，model 等于 `Model` 或非空的 `Alias`）现在有三份：`configuredAgentFor`（取第一个）、`sameBrainAgent`（再比强度）、`sameModelAgents`（收集全部，A9）。前两个是 HEAD 的，一个字不改；三处要一起改（§7 (z)）。
    - 往会话里 Join 一个配置里的 agent 的写法（`conversation.Participant` 加「已加入会话」）现在有三处：`cmdAdd` 的循环、`commands.go:747-764`（团队那一支）、`joinConfiguredAgent`（A9）。`addDiscoveredModel` 末尾的 Join 是新成员那一支，另一回事。
    - 总结成员不当普通成员加（#47）：`cmdAdd` 的候选不含它；D17 的 S 同样去掉它（N22 summary_excluded，V78）。配置第 0 步按字节比，所以 `/add GROK-4.7` 走得到快照，不去掉的话会被 D17 第 4 步当普通成员加入。
    - 同脑子那一句现在只在给了明确的强度（带强度的 ID、两个词、`--effort`）或只有 `--id` 时打得到；没给强度的单个名字先过 D17。S1 里原来经没给强度的名字走到同脑子那一句的只有 N9 的 `/add grok-4.6` 与 N15 alias_same_brain 两条（逐条核过），前者照裁决书授权改期望，后者改成带 `--effort high`、另加 alias_configured（§2 的 v1.2 说明）。
- **D16 交付分块与跨文件约束**：
  - **块 A（`internal/repl`，只依赖 HEAD）**：A1（`repl.go`）、A2-A7（`addmodel.go`）、A8（`commands.go` 的 `splitAddArgs`，v1.1）八处补丁，替换文本合计 110 行（v1.2 的 A1 注释多一行）；A9（v1.2）新文件 `internal/repl/addname_existing.go`（92 行：D17 的四句、`addNameExisting`、`sameModelAgents`、`joinConfiguredAgent`）。块 A 合计 202 行。除 A8 的全角空格切段外行为不变（A9 在块 B-乙落地之前没有调用方）：只落块 A 时全量九包全绿（v1.2 原型核过，含 A8、A9）。A8 放在块 A 而不是块 B-甲：它只依赖 HEAD，放在块 A 可以随块 A 单独落地先跑。A9 同理：它只用到 HEAD 的代码；v1.2 起先把 D17 写在 `addname.go` 与 `addmodel.go` 里时，块 B 到了 479 行。
  - **块 B-甲（`internal/repl/commands.go`，依赖块 A）**（v1.3 从 v1.2 的块 B 拆出）：B1-B4 四处补丁，替换文本合计 64 行。
  - **块 B-乙（`internal/repl/addname.go`，依赖块 A）**（v1.3 从 v1.2 的块 B 拆出）：新文件 `addname.go`（B5，355 行；v1.1 是 331 行）。用到块 A 的 `REPL.addByName`（A1）、`ensureModelSnapshot`（A5）、`addDiscoveredModel`（A7）、`addNameExisting`（A9）；`parseAddNameArgs` 经 `splitAddArgs` 切名字，A8 之后同样按全角空格切（行为上的依赖，编译上不依赖）；块 A 与块 B-甲、块 B-乙之间没有别的约束。
  - **块 B-甲与块 B-乙互相依赖，要一起落地才编译得过**：B2 调 `addname.go` 的 `cmdAddByName`，B4 的 `addSegmentNotFound` 引用 `addname.go` 的 `msgAddNameNotFound`；`addname.go` 调 B4 拆出的 `resolveAddConfig`、`addSegmentNotFound`。v1.3 原型实测：只落块 A 加块 B-甲、只落块 A 加块 B-乙，`go build` 都不过（报的 undefined 见 §1.1 块 B-甲的说明）。所以两块可以分两次交给实现腿，但落地时要一起落，中间没有能单独编译、单独跑测试的状态；能单独落地并跑全量的只有块 A（v1.2 原型核过）。拆法照 v1.2 的 §7 (aa)：v1.2 的块 B 是 419 行，比「每块约 400 行」多 19 行，v1.2 加在 `addname.go` 里的（带参数压过前缀命中、敲错的强度的两条新判据、D17 的调用处）都在 `cmdAddByName`、`addByOneName` 的冻结顺序里，文件本身拆不开（拆成两个 `.go` 文件就改了字节），所以按文件切成 64 行与 355 行两块，冻结代码的字节一个不变。
  - **块 C（`internal/app`，依赖块 B-乙，因而也要块 B-甲一起落了才编译得过）**：C1、C2（`app.go`）两处补丁，替换文本合计 6 行，用到块 B-乙的 `(*REPL).SetAddByName`。
  - **跨文件约束**：B4 把 `resolveAddSegment` 的第 0-3 步挪进 `resolveAddConfig`、末尾三句挪进 `addSegmentNotFound`，这两处的中文字面量必须与 HEAD 逐字节相同（本文的 B4 就是这样取的：只把 `return "", fmt.Sprintf(` 改成 `return fmt.Sprintf(`，外加开关那三行）；`addname.go` 的 `msgAddNameNotFound` 被 B4 的 `addSegmentNotFound` 引用；S1 的期望逐字依赖 `addname.go` 的六个 `msgAddName*` 与 A7 搬过去的既有说法（v1.1 另依赖 `msgAddModelUsage` 与 `addDiscoveredModel` 的「支持的强度是」那一句）；写侧（`addname.go`、B4）的字面量若改，读侧 S1 跟着改。S1 的 N17 依赖 A8。v1.2：S1 的 N9、N15 alias_configured、N22 逐字依赖 A9 的四句（`adnPresent`、`adnConfigured` 两个拼句函数照它们拼）与既有的「已加入会话」；`addname.go` 的敲错的强度那一条判据拿 `addSegmentNotFound` 的返回值与 `fmt.Sprintf(msgAddNameNotFound, 第二段)` 比，`msgAddNameNotFound` 或 `addSegmentNotFound` 的「都没有」那一支若改，这一条跟着改（N24 钉着）。`addname.go` 的 `addByOneName` 调 A9 的 `addNameExisting`，A9 的 `sameModelAgents` 与 `addmodel.go` 的 `configuredAgentFor`、`sameBrainAgent` 是同一条「同一个模型」判据的三份，改一处要改三处（§7 (z)）。
  - 测试腿的两个文件：S1（`internal/repl/adn_addname_test.go`）依赖块 A、B-甲、B-乙；S2（`internal/app/adn_addname_app_test.go`）依赖块 A、B-甲、B-乙、C。
  - 合计约 627 行（不算锚点：块 A 110 加 92、块 B-甲 64、块 B-乙 355、块 C 6）。四块都在 400 行以内：块 A 202 行、块 B-甲 64 行、块 B-乙 355 行、块 C 6 行（v1.2 的块 B 是块 B-甲加块 B-乙，419 行）。
- **D17 没给强度的单个名字与配置里已有的同模型成员**（v1.2，裁决书规则二、规则三；W1b 的 major）。
  - **什么时候进门**：`addByOneName` 在快照里恰好对上一行之后、取强度之前（`addNameExisting`，A9），条件是四样都没有：第二个词、`--effort`、`--id`、名字里带的强度（带强度的 ID 形式）。所以只有「单段、配置里没命中、在快照里按模型名形式或不带强度的 ID 形式对上一行」会进门。配置的第 0-3 步先走，行为一个字节不变：前缀命中照旧加入、不识别（每条 `/add op` 不多等一次识别）。对上多行时照 D11 列候选，不进门。
  - **S**：配置里与对上的那一行 provider 相同、model 相同（等于 `Model` 或非空的 `Alias`）的全部 agent，判据与 `configuredAgentFor` 相同、收集全部（`sameModelAgents`，按 id 排序），去掉 `[summary]` 的总结成员（#47；D15 v1.2 那一条）。**判据是 provider 加 model，不是 id 前缀**：id 前缀会把 `/add gpt-6.1` 挡在在场的 `gpt-6.1-sol-xhigh` 上（N22 prefix_other_model，V72），别名形式（`sonnet[1m]` 对 `sonnet-5-1m-xhigh`、`opus[1m]` 对 `opus-5.0`）又会漏掉（N22 alias_present、alias_config_present）。provider 那一半（v1.3）：grok 的成员 model 字符串恰好是 `gpt-6.1-sol` 时不进 codex 那一行的 S，`/add gpt-6.1-sol` 照常新建 codex 的 `gpt-6.1-sol-high`、写配置（N22 other_provider_same_model，V91）。S 只从配置里取：已从配置里删掉、仍在场的成员不进 S（§7 (ab)）。
  - **四种结局**，按顺序：
    1. S 为空：出门，照 v1.1 按 `DefaultEffort()` 新建、写配置（N4 default_effort 等）。
    2. S 里有在场的成员：拒绝，不写配置，点名在场的那几个（按 id 排序，用「, 」连）。那一行有强度可选时句末提示怎样另加一档：「<名字> 对上的模型已经有成员在当前会话里：<ids>；要另加一档强度，用 /add <名字> <强度> 或 --effort <强度>」；grok、agy 这类没有强度的行不提强度：「<名字> 对上的模型已经有成员在当前会话里：<ids>」（N22 repeat、alias_present、alias_config_present、one_absent_renamed，N9；V71、V79）。只点名在场的，S 里不在场的不列（v1.3：N22 present_among_absent、present_among_absent_grok，一个在场、一个不在场；V87、V88）。
    3. S 全不在场、恰好一个：加入它，不写配置，屏幕与 `cmdAdd` 按配置加入相同：「<id> 已加入会话 <会话 id>」（`joinConfiguredAgent`；N22 one_absent_config、one_absent_renamed，N15 alias_configured；V75）。会话成员记录里的显示名与 provider 取配置里的值（v1.3：one_absent_config 断言，V89、V90）。
    4. S 全不在场、不止一个：列出来，不写配置、不猜：「<名字> 对上的模型在配置里有这几个成员：<ids>；用 /add <id> 加入其中一个，或用 /add <名字> <强度> 带强度新加一档」；没有强度可选的行去掉「，或用……」那半句（N22 several_absent、several_absent_grok；V74、V80）。
  - 所以同一条命令敲两次结果不变：`gpt-6.1-sol-xhigh` 在配置里、不在场时，第一次 `/add gpt-6.1-sol` 按配置前缀加入它，第二次它在场、不是候选了，到快照里对上、由第 2 种拒绝（N22 repeat）。v1.1 的第二次会按默认档新建 `gpt-6.1-sol-high`、写配置、让同一个模型两个成员同时在场（W1b 的 major，反驳者实跑过）。
  - 给了明确的强度（两个词、`--effort`、带强度的 ID）或只有 `--id` 时不进门，照 v1.1：同一模型同一强度已在配置里打同脑子那一句、不写（N5 same_brain、N16、N15 alias_same_brain），强度不同就新建（N23；V77、V81、V82、V83）。

#### 注释对账（抽共用函数时搬的注释，逐条）

`cmdAddDiscovered` 的头注释（HEAD `addmodel.go:295-318`）：

| 要点（HEAD 原文） | 去处 |
|---|---|
| 顺序冻结：参数形状，Runner 在不在，有没有快照（没有时先看配置里有没有能识别的 provider，一家都没有就直说，不叫用户先敲 /add），编号 | 留在 `cmdAddDiscovered` 头注释（A6），逐字 |
| ……强度，同脑子，id，配置文件路径，守卫（先 data_dir 与 rules_file，再整份比），追加，加载校验，换指针，Join | `addDiscoveredModel` 头注释（A7）「顺序冻结（接着调用方的那几步）」，逐字；`cmdAddDiscovered` 那边写「从强度起交给 addDiscoveredModel」 |
| 每一步失败都只打一行 | 两处都写 |
| 失败之后留下什么，按阶段分：备份之前 / AppendBlock 的备份阶段（R16、B1 注释要点 7、open item (p)）/ 备份之后（R15、R8）/ 换指针之后只剩 Join | `addDiscoveredModel` 头注释，四段逐字 |
| 新成员固定 permissions 为 read、mode 为 discussion（#45）；role 空串；display_name 与 id 取建议，--id 可改 | `addDiscoveredModel` 头注释，逐字 |
| （新增）调用方已判过什么、haveEffort / haveID 为假时不看对应的值、`tok` 的含义 | `addDiscoveredModel` 头注释 |

`cmdAddDiscovered` 函数体里的两段注释（守卫那一段 HEAD `:403-424`、换指针那一段 `:462-474`）在搬过去的函数体里，一行不改（A7 的锚点只覆盖 `m := row.m` 那三行，函数体其余部分原地不动、成为 `addDiscoveredModel` 的函数体）。强度那一支的一行注释（`:366`）同样原地不动。

`printDiscoveredModels` 的头注释（HEAD `:170-178`）：

| 要点 | 去处 |
|---|---|
| 只在本会话还没有快照时识别 | `printDiscoveredModels`（加「（ensureModelSnapshot）」）与 `ensureModelSnapshot` 都写 |
| 一家能识别的都没有时直接返回，不打「正在识别模型」 | 留在 `printDiscoveredModels`，逐字 |
| 拍完之后父 ctx 已取消，就不赋给 r.modelSnap，只打「识别被取消」那一行，下次 /add 重新识别 | `ensureModelSnapshot`，逐字，加「返回 canceled 为真」；`printDiscoveredModels` 另写「识别被取消时只有那一行，不打表、不打登录段」 |
| TUI 里提交在跑时按 Ctrl-C，取消的是这一次提交的 ctx（tui/model.go:341-347） | `ensureModelSnapshot`；行号换成按名字指（D15 最后一条） |
| 小竞态：三家都跑完之后才到的取消，也会丢掉这一份完整结果；代价只是下次多识别一次，接受 | `ensureModelSnapshot`，逐字 |
| （新增）两条路共用：同一份快照、同一句「正在识别模型」、同一个 discoverTimeout、同一套取消语义；err 只来自写屏幕 | `ensureModelSnapshot` |

`resolveAddSegment`（HEAD `commands.go:1054-1129`）：头注释三行原样留下，加一句指向拆出的两个函数；第 0-3 步的注释随代码原样进 `resolveAddConfig`（一行不改）；末尾「没找到时区分……」那五行注释从函数体里挪成 `addSegmentNotFound` 的头注释，字不改、去掉行首的 tab，再加一段开关打开时的说明。

其余注释改动（都在块 A）：`addmodel.go` 文件头（A2：识别也在按名字加而没有快照时做；「一家失败只印一行」改成「失败行随快照在不带参数的 /add 里打」）；`configuredAgentFor`（A3：open item (c) 那一句）；`takeModelSnapshot`（A4：`printDiscoveredModels` 换成 `ensureModelSnapshot`）；`splitAddArgs` 头注释（A8：切分符里加上全角空格，另加 `ideographicSpace` 的注释，写法理由照 `fullwidthComma`）。

#### 块 A：`internal/repl`（八处补丁、一个新文件；只依赖 HEAD；A8 是 v1.1 加的，A9 是 v1.2 加的）

**A1. `internal/repl/repl.go`：`REPL` 加字段 `addByName`**（HEAD `:125-126`）

```anchor internal/repl/repl.go
	loginStatus provider.LoginStatusExec
}
```

```replace
	loginStatus provider.LoginStatusExec
	// addByName 为 true 时，/add <名字> 在配置里对不上就到识别快照里按名字找（AD1，见 addname.go）。
	// 零值 false 是关：/add 的行为与 AD1 之前逐字节相同（splitAddArgs 按全角空格切段那一条除外，
	// 它不看开关）；只经 SetAddByName 改，app 装配时打开。
	addByName bool
}
```

**A2. `internal/repl/addmodel.go`：文件头注释**（HEAD `:19-21`）

```anchor internal/repl/addmodel.go
// 识别只在不带参数的 /add 或 /add --refresh 时做，不在启动时做。每家一次、
// 串行，各自带 discoverTimeout；一家失败只印一行，不拦别家。识别中途被取消
// 时不存快照，见 takeModelSnapshot 与 printDiscoveredModels。
```

```replace
// 识别只在不带参数的 /add、/add --refresh，以及按名字加识别出的模型而本会话
// 还没有快照时（AD1，addname.go）做，不在启动时做。每家一次、串行，各自带
// discoverTimeout；一家失败不拦别家，失败行随快照在不带参数的 /add 里打，每家
// 一行。识别中途被取消时不存快照，见 takeModelSnapshot 与 ensureModelSnapshot。
```

**A3. `internal/repl/addmodel.go`：`configuredAgentFor` 注释里 open item (c) 那一句**（HEAD `:86-87`）

```anchor internal/repl/addmodel.go
// 编号。这些模型换强度目前只能手改配置（open item (c)）。
func configuredAgentFor(cfg *config.Config, m provider.DiscoveredModel) string {
```

```replace
// 编号。这些模型换强度用 /add <ID>（按名字加识别出的模型，AD1，addname.go）：
// 快照里的每一行都能按名字加，不看有没有编号。开关关着时（直接 New 出来的 REPL）
// 仍然只能手改配置（16a open item (c)）。
func configuredAgentFor(cfg *config.Config, m provider.DiscoveredModel) string {
```

**A4. `internal/repl/addmodel.go`：`takeModelSnapshot` 注释里的函数名**（HEAD `:121`）

```anchor internal/repl/addmodel.go
// 回到开头，不存快照要靠 printDiscoveredModels 拍完之后那一处。这时前几家的
```

```replace
// 回到开头，不存快照要靠 ensureModelSnapshot 拍完之后那一处。这时前几家的
```

**A5. `internal/repl/addmodel.go`：`printDiscoveredModels` 拆出 `ensureModelSnapshot`**（HEAD `:170-198`）

```anchor internal/repl/addmodel.go
// printDiscoveredModels 只在本会话还没有快照时识别。一家能识别的都没有时
// 直接返回，不打「正在识别模型」。
//
// 拍完之后父 ctx 已取消，就不赋给 r.modelSnap，只打「识别被取消」那一行，
// 下次 /add 重新识别。TUI 里提交在跑时按 Ctrl-C，取消的是这一次提交的 ctx
// （tui/model.go:341-347）。
//
// 小竞态：三家都跑完之后才到的取消，也会丢掉这一份完整结果。代价只是下次
// 多识别一次，接受。
func printDiscoveredModels(ctx context.Context, r *REPL) error {
	pids := discoverableProviders(r.cfg)
	if len(pids) == 0 {
		return nil
	}
	if r.modelSnap == nil {
		if err := r.printf("正在识别模型: %s\n", strings.Join(pids, ", ")); err != nil {
			return err
		}
		snap := takeModelSnapshot(ctx, r, pids)
		if ctx.Err() != nil {
			return r.printf("识别被取消；下次 /add 会重新识别\n")
		}
		r.modelSnap = snap
	}
	if err := printModelSnapshot(r, r.modelSnap); err != nil {
		return err
	}
	return printLoginStatus(r, r.modelSnap)
}
```

```replace
// printDiscoveredModels 只在本会话还没有快照时识别（ensureModelSnapshot）。一家能
// 识别的都没有时直接返回，不打「正在识别模型」。识别被取消时只有「识别被取消」
// 那一行，不打表、不打登录段。
func printDiscoveredModels(ctx context.Context, r *REPL) error {
	pids := discoverableProviders(r.cfg)
	if len(pids) == 0 {
		return nil
	}
	if canceled, err := ensureModelSnapshot(ctx, r, pids); canceled || err != nil {
		return err
	}
	if err := printModelSnapshot(r, r.modelSnap); err != nil {
		return err
	}
	return printLoginStatus(r, r.modelSnap)
}

// ensureModelSnapshot 在本会话还没有快照时按 pids 识别一次、存下；已有快照什么都
// 不做（/add --refresh 才重拍）。不带参数的 /add（printDiscoveredModels）与按名字加
// 识别出的模型（AD1，addname.go）共用它：两条路是同一份快照、同一句「正在识别模型」、
// 同一个 discoverTimeout、同一套取消语义。
//
// 拍完之后父 ctx 已取消，就不赋给 r.modelSnap，只打「识别被取消」那一行、返回
// canceled 为真，下次 /add 重新识别。TUI 里提交在跑时按 Ctrl-C，取消的是这一次提交
// 的 ctx（tui/model.go 里忙时按 Ctrl-C 那一支调 m.cancel）。
//
// 小竞态：三家都跑完之后才到的取消，也会丢掉这一份完整结果。代价只是下次
// 多识别一次，接受。
//
// err 只来自写屏幕；canceled 为真时 err 是打「识别被取消」那一行的结果。
func ensureModelSnapshot(ctx context.Context, r *REPL, pids []string) (canceled bool, err error) {
	if r.modelSnap != nil {
		return false, nil
	}
	if err := r.printf("正在识别模型: %s\n", strings.Join(pids, ", ")); err != nil {
		return false, err
	}
	snap := takeModelSnapshot(ctx, r, pids)
	if ctx.Err() != nil {
		return true, r.printf("识别被取消；下次 /add 会重新识别\n")
	}
	r.modelSnap = snap
	return false, nil
}
```

**A6. `internal/repl/addmodel.go`：`cmdAddDiscovered` 头注释（失败阶段等要点搬到 A7）**（HEAD `:295-319`）

```anchor internal/repl/addmodel.go
// cmdAddDiscovered 处理 /add +编号。顺序冻结：参数形状，Runner 在不在，有没有
// 快照（没有时先看配置里有没有能识别的 provider，一家都没有就直说，不叫用户
// 先敲 /add），编号，强度，同脑子，id，配置文件路径，守卫（先 data_dir 与
// rules_file，再整份比），追加，加载校验，换指针，Join。每一步失败都只打一行。
//
// 失败之后留下什么，按阶段分：
//
// 备份之前（参数到守卫，以及 AppendBlock 的读原文件阶段）：盘上文件与内存一个不动。
//
// AppendBlock 的备份阶段：一般不留文件，屏幕是「写配置文件失败: 备份: 」后接
// 备份错误。R16 走的是 99 个名字全被占这一种。例外：备份文件已经建好、写到一半
// 失败（例如磁盘满）时，writeBackup 返回空串，那个残缺文件留在盘上，路径不上屏
// （B1 注释要点 7、open item (p)）。
//
// 备份之后：写入阶段失败时原文件没有被替换（临时文件加改名，改名失败原文件原样）；
// 写后加载校验不过时用 RestoreFile 写回原字节。两种都留着备份，屏幕那一行带上
// 它的路径（R15、R8）。
//
// 换指针之后只剩 Join。它失败时配置已经写进去、指针已经换了，不回滚：配置本身
// 合法，用户改天 /add <id> 就能加入。
//
// 新成员固定 permissions 为 read、mode 为 discussion（#45）。role 为空串
// （见 append.go 里 AgentBlockTOML 的注释）。display_name 与 id 取 provider 包
// 的建议，id 可用 --id 改。
func cmdAddDiscovered(ctx context.Context, r *REPL, fields []string) error {
```

```replace
// cmdAddDiscovered 处理 /add +编号。顺序冻结：参数形状，Runner 在不在，有没有
// 快照（没有时先看配置里有没有能识别的 provider，一家都没有就直说，不叫用户
// 先敲 /add），编号；从强度起交给 addDiscoveredModel（按名字加识别出的模型，AD1，
// 走的也是它）。每一步失败都只打一行。
func cmdAddDiscovered(ctx context.Context, r *REPL, fields []string) error {
```

**A7. `internal/repl/addmodel.go`：`cmdAddDiscovered` 从强度起交给 `addDiscoveredModel`（函数体其余部分原地不动）**（HEAD `:363-365`）

```anchor internal/repl/addmodel.go
	m := row.m

	switch {
```

```replace
	return addDiscoveredModel(ctx, r, row.m, effort, haveEffort, newID, haveID, tok)
}

// addDiscoveredModel 把识别出的一行 m 以某个强度写进配置并加入当前会话：/add +编号
// （cmdAddDiscovered）与按名字加（AD1，addname.go 的 addByOneName）共用它，所以
// 两条路的屏幕输出与写进文件的字节相同。调用方已经判过参数形状、Runner 在不在、
// 快照与这一行。haveEffort 为假时 effort 不看；haveID 为假时 newID 不看。tok 是
// 守卫拒绝时「再 /add <tok>」里的那一段：+编号 是 +N 那个词，按名字加是敲的名字，
// 两种都不带 --id、--effort 与第二个词。
//
// 顺序冻结（接着调用方的那几步）：强度，同脑子，id，配置文件路径，守卫（先
// data_dir 与 rules_file，再整份比），追加，加载校验，换指针，Join。每一步失败
// 都只打一行。
//
// 失败之后留下什么，按阶段分：
//
// 备份之前（参数到守卫，以及 AppendBlock 的读原文件阶段）：盘上文件与内存一个不动。
//
// AppendBlock 的备份阶段：一般不留文件，屏幕是「写配置文件失败: 备份: 」后接
// 备份错误。R16 走的是 99 个名字全被占这一种。例外：备份文件已经建好、写到一半
// 失败（例如磁盘满）时，writeBackup 返回空串，那个残缺文件留在盘上，路径不上屏
// （B1 注释要点 7、open item (p)）。
//
// 备份之后：写入阶段失败时原文件没有被替换（临时文件加改名，改名失败原文件原样）；
// 写后加载校验不过时用 RestoreFile 写回原字节。两种都留着备份，屏幕那一行带上
// 它的路径（R15、R8）。
//
// 换指针之后只剩 Join。它失败时配置已经写进去、指针已经换了，不回滚：配置本身
// 合法，用户改天 /add <id> 就能加入。
//
// 新成员固定 permissions 为 read、mode 为 discussion（#45）。role 为空串
// （见 append.go 里 AgentBlockTOML 的注释）。display_name 与 id 取 provider 包
// 的建议，id 可用 --id 改。
func addDiscoveredModel(ctx context.Context, r *REPL, m provider.DiscoveredModel, effort string, haveEffort bool, newID string, haveID bool, tok string) error {
	switch {
```

**A8. `internal/repl/commands.go`：`splitAddArgs` 也按全角空格 U+3000 切**（HEAD `:1042-1047`；v1.1 新增。块 A 里唯一改 `commands.go` 的一处，只依赖 HEAD；在文件里位于块 B-甲的 B2 与 B3 之间，与 B1-B4 互不重叠）

```anchor internal/repl/commands.go
// splitAddArgs 把 /add 的参数按 ASCII 空白、半角逗号、全角逗号切开，丢掉空段。
// 按 rune 判而不是按字节：全角逗号是多字节的，按字节切会把它劈成乱码。
func splitAddArgs(s string) []string {
	return strings.FieldsFunc(s, func(c rune) bool {
		switch c {
		case ' ', '\t', '\n', '\r', '\v', '\f', ',', fullwidthComma:
```

```replace
// ideographicSpace 是全角空格的码位，写成数字常量的理由同 fullwidthComma。中文输入法
// 全角状态下敲空格出的就是它；args.fields（strings.Fields）本来就在它上面切，splitAddArgs
// 也切，两处对齐（AD1）。这一条不看 SetAddByName 的开关。
const ideographicSpace rune = 0x3000

// splitAddArgs 把 /add 的参数按 ASCII 空白、全角空格、半角逗号、全角逗号切开，丢掉空段。
// 按 rune 判而不是按字节：全角逗号是多字节的，按字节切会把它劈成乱码。
func splitAddArgs(s string) []string {
	return strings.FieldsFunc(s, func(c rune) bool {
		switch c {
		case ' ', '\t', '\n', '\r', '\v', '\f', ideographicSpace, ',', fullwidthComma:
```

**A9. 新文件 `internal/repl/addname_existing.go`**（v1.2，D17：四句、`addNameExisting`、`sameModelAgents`、`joinConfiguredAgent`；只用到 HEAD 的代码，块 B-乙的 `addByOneName` 调它。文件头注释里的「块 B」是 v1.2 的叫法，指 v1.3 的块 B-乙；冻结代码不改，这一句照旧）

```file internal/repl/addname_existing.go
package repl

import (
	"context"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// addname_existing.go 是按名字加识别出的模型（AD1，addname.go）里「这个模型配置里已有成员」那一道门（D17，v1.2）：
// 单个名字没给强度也没给 --id 时，addByOneName 先调 addNameExisting，配置里有同一个模型的成员就加入、拒绝或列出，
// 都不写配置。这个文件只用到 AD1 之前就有的代码，放在块 A（契约 D16）；调用方是块 B 的 addname.go。

// D17 的说法（§3 冻结）。都是整行，含换行。
const (
	// msgAddNamePresent 是没给强度的名字对上的模型在配置里已有成员、其中有在场的；那一行没有强度可选。
	msgAddNamePresent = "%s 对上的模型已经有成员在当前会话里：%s\n"
	// msgAddNamePresentEffort 同上，那一行有强度可选，句末说怎样另加一档。
	msgAddNamePresentEffort = "%s 对上的模型已经有成员在当前会话里：%s；要另加一档强度，用 /add %s <强度> 或 --effort <强度>\n"
	// msgAddNameConfigured 是没给强度的名字对上的模型在配置里有不止一个成员、都不在场；那一行没有强度可选。
	msgAddNameConfigured = "%s 对上的模型在配置里有这几个成员：%s；用 /add <id> 加入其中一个\n"
	// msgAddNameConfiguredEffort 同上，那一行有强度可选。
	msgAddNameConfiguredEffort = "%s 对上的模型在配置里有这几个成员：%s；用 /add <id> 加入其中一个，或用 /add %s <强度> 带强度新加一档\n"
)

// addNameExisting 是 addByOneName 在没给强度、也没给 --id 时先过的一道门（D17）。S 是配置里
// 与 m 同一个模型的全部 agent（sameModelAgents，判据同 configuredAgentFor：provider 相同，
// model 等于 Model 或非空的 Alias；不看 id 前缀），去掉总结成员（它不当普通成员加，#47）。
// S 为空时 done 为假，接着按默认档新建。其余三种都不写配置：S 里有在场的就拒绝、点名在场
// 的那几个（有强度可选的行另说怎样加一档）；全不在场、恰好一个就加入它，屏幕与 cmdAdd 按
// 配置加入的那一支相同；不止一个就列出来，不猜。所以同一条命令敲两次，第二次说已在会话里。
func addNameExisting(ctx context.Context, r *REPL, name string, m provider.DiscoveredModel, active map[string]bool) (bool, error) {
	var s, present []string
	for _, id := range sameModelAgents(r.cfg, m) {
		if id == r.cfg.Summary.Agent {
			continue
		}
		s = append(s, id)
		if active[id] {
			present = append(present, id)
		}
	}
	switch {
	case len(s) == 0:
		return false, nil
	case len(present) > 0 && len(m.Efforts) > 0:
		return true, r.printf(msgAddNamePresentEffort, name, strings.Join(present, ", "), name)
	case len(present) > 0:
		return true, r.printf(msgAddNamePresent, name, strings.Join(present, ", "))
	case len(s) == 1:
		return true, joinConfiguredAgent(ctx, r, s[0])
	case len(m.Efforts) > 0:
		return true, r.printf(msgAddNameConfiguredEffort, name, strings.Join(s, ", "), name)
	}
	return true, r.printf(msgAddNameConfigured, name, strings.Join(s, ", "))
}

// sameModelAgents 按 configuredAgentFor 的判据（provider 相同，model 等于 Model 或非空的
// Alias）收集配置里与 m 同一个模型的全部 agent，按 id 排序，不看强度。addNameExisting 用它
// 判「这个模型配置里已有哪些成员」。
func sameModelAgents(cfg *config.Config, m provider.DiscoveredModel) []string {
	var out []string
	for _, id := range sortedKeys(cfg.Agents) {
		a := cfg.Agents[id]
		if a.Provider == m.Provider && (a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias)) {
			out = append(out, id)
		}
	}
	return out
}

// joinConfiguredAgent 把配置里的 agent id 加入当前会话：与 cmdAdd 按配置加入的那一支
// （commands.go）同一个 Participant 写法、同一句「已加入会话」；Join 失败时照那一支打
// 「加入失败」（reportErr）后返回。
func joinConfiguredAgent(ctx context.Context, r *REPL, id string) error {
	ac := r.cfg.Agents[id]
	p := conversation.Participant{
		ConvID:      r.cur.ID,
		AgentID:     id,
		DisplayName: ac.DisplayName,
		Provider:    ac.Provider,
		Active:      true,
		JoinedAt:    time.Now().UTC(),
	}
	if err := r.svc.Join(ctx, r.cur.ID, p); err != nil {
		return r.reportErr("加入失败", err)
	}
	return r.printf("%s 已加入会话 %s\n", id, r.cur.ID)
}
```

#### 块 B-甲：`internal/repl/commands.go`（B1-B4 四处补丁，替换文本合计 64 行；依赖块 A，与块 B-乙互相依赖）

v1.3 把 v1.2 的块 B（419 行）拆成块 B-甲与块 B-乙两块，只重排、代码字节不变（D16）。两块互相引用，**要一起落地才编译得过**：B2 调块 B-乙的 `cmdAddByName`，B4 的 `addSegmentNotFound` 引用块 B-乙的 `msgAddNameNotFound`；块 B-乙的 `addname.go` 调 B4 拆出的 `resolveAddConfig` 与 `addSegmentNotFound`。v1.3 原型实测：只落块 A 加块 B-甲，`go build` 报 `undefined: cmdAddByName`、`undefined: msgAddNameNotFound`；只落块 A 加块 B-乙，报 `undefined: resolveAddConfig`、`undefined: addSegmentNotFound`。

**B1. `internal/repl/commands.go`：`cmdAdd` 头注释**（HEAD `:875-877`）

```anchor internal/repl/commands.go
// 配置里还没有的工具各打一行提示（cliadvice.go，#48）。
//
// 不带参数时列出可加入的候选（带编号）；其余参数按全名 / 编号 / 前缀加入，
```

```replace
// 配置里还没有的工具各打一行提示（cliadvice.go，#48）。
//
// 开关打开时（SetAddByName，app 装配时打开），带参数的 /add 先交给 cmdAddByName
// （addname.go，AD1）：每一段都在配置里命中、也没有 --id / --effort 时它不插手，
// 下面照旧；有段在配置里对不上时到识别快照里按名字找，找到就写进配置并加入。
//
// 不带参数时列出可加入的候选（带编号）；其余参数按全名 / 编号 / 前缀加入，
```

**B2. `internal/repl/commands.go`：`cmdAdd` 切段之后先交给 `cmdAddByName`**（HEAD `:936-937`）

```anchor internal/repl/commands.go
	segs := splitAddArgs(args.rest)
	if len(segs) == 0 {
```

```replace
	segs := splitAddArgs(args.rest)
	if r.addByName {
		if done, err := cmdAddByName(ctx, r, args.fields, segs, candidates, active, member); done {
			return err
		}
	}
	if len(segs) == 0 {
```

**B3. `internal/repl/commands.go`：`resolveAddSegment` 拆成 `resolveAddConfig` 与 `addSegmentNotFound`（头）**（HEAD `:1057-1059`）

```anchor internal/repl/commands.go
// 顺序是写死的，先命中的算数：全名精确 > 编号 > 前缀。
func resolveAddSegment(r *REPL, seg string, candidates []string, active, member map[string]bool) (id, problem string) {
	// 0. 总结成员（#47）排在全名之前拦：它在配置里，不拦的话第 1 步就放行了。
```

```replace
// 顺序是写死的，先命中的算数：全名精确 > 编号 > 前缀。配置里的第 0-3 步在
// resolveAddConfig，都没命中时的说法在 addSegmentNotFound：按名字加识别出的模型
// （AD1，addname.go）要在这两者之间插一步，到识别快照里找。
func resolveAddSegment(r *REPL, seg string, candidates []string, active, member map[string]bool) (id, problem string) {
	if id, problem := resolveAddConfig(r, seg, candidates, active, member); id != "" || problem != "" {
		return id, problem
	}
	return "", addSegmentNotFound(r, seg, active, member)
}

// resolveAddConfig 是 resolveAddSegment 在配置里的第 0-3 步。某一步命中时返回 id
// （加入）或 problem（拒绝的那一行，含换行），恰好一个非空。两个都是空串表示没命中，
// 只有一种情形：第 3 步的前缀在候选里一个都没找到。总结成员、只是成员不在配置里、
// 全是数字（含超出范围）、前缀歧义，都算命中。
func resolveAddConfig(r *REPL, seg string, candidates []string, active, member map[string]bool) (id, problem string) {
	// 0. 总结成员（#47）排在全名之前拦：它在配置里，不拦的话第 1 步就放行了。
```

**B4. `internal/repl/commands.go`：同上（尾）**（HEAD `:1110-1129`）

```anchor internal/repl/commands.go
		return "", fmt.Sprintf("%s 有歧义，可能是：%s\n", seg, strings.Join(refErr.Matches, ", "))
	}
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
		if strings.HasPrefix(aid, seg) {
			return "", fmt.Sprintf("%s 开头的都已经在当前会话里了\n", seg)
		}
	}
	return "", fmt.Sprintf("配置里没有可加入的 %s，不带参数的 /add 能列出可加的\n", seg)
}
```

```replace
		return "", fmt.Sprintf("%s 有歧义，可能是：%s\n", seg, strings.Join(refErr.Matches, ", "))
	}
	return "", ""
}

// addSegmentNotFound 是一段在配置里没命中（resolveAddConfig 两个返回值都是空串）、
// 开关打开时在识别快照里也没对上时的说法，一整行，含换行。
//
// 没找到时区分「以它开头的都已经在场」与「根本没有」：前者用户多半是
// 想加一个已经在的人，告诉他不用加了，比说「配置里没有」准确。
// 不在场的总结成员也以它开头时（OB1），先说总结成员：它不是候选，
// 说「都已经在当前会话里了」是假话。加入过又退出的，runSummary 不会再把它
// 拉回来，照 runSummary 那句指 /join，不说「它会自己加入」。
//
// 开关打开时（AD1）最后一句换成同时指出两条路的那一句：不带参数的 /add 列出两边，
// 识别表里的模型可以直接 /add 它的 ID。前两句不变，而且排在识别快照之后
// （addname.go 先在快照里找，对不上才到这里）。
func addSegmentNotFound(r *REPL, seg string, active, member map[string]bool) string {
	if sumID := r.cfg.Summary.Agent; sumID != "" && !active[sumID] && strings.HasPrefix(sumID, seg) {
		if member[sumID] {
			return fmt.Sprintf("%s 开头的除总结成员 %s 外没有可加入的；%s 已退出当前会话，要恢复总结用 /join %s\n", seg, sumID, sumID, sumID)
		}
		return fmt.Sprintf("%s 开头的除总结成员 %s 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结\n", seg, sumID)
	}
	for aid := range active {
		if strings.HasPrefix(aid, seg) {
			return fmt.Sprintf("%s 开头的都已经在当前会话里了\n", seg)
		}
	}
	if r.addByName {
		return fmt.Sprintf(msgAddNameNotFound, seg)
	}
	return fmt.Sprintf("配置里没有可加入的 %s，不带参数的 /add 能列出可加的\n", seg)
}
```

#### 块 B-乙：`internal/repl/addname.go`（B5 新文件，355 行；依赖块 A，与块 B-甲互相依赖）

块 B-乙用到块 A 的 `REPL.addByName`（A1）、`ensureModelSnapshot`（A5）、`addDiscoveredModel`（A7）、`addNameExisting`（A9），块 B-甲的 `resolveAddConfig`、`addSegmentNotFound`（B4）；`parseAddNameArgs` 经 `splitAddArgs` 切名字，A8 之后同样按全角空格切（行为上的依赖，编译上不依赖）。跨文件约束见 D16。

**B5. 新文件 `internal/repl/addname.go`**

```file internal/repl/addname.go
package repl

import (
	"context"
	"fmt"
	"strings"

	"council/internal/provider"
)

// addname.go 是 /add 按名字加识别出的模型（AD1，00_DECISIONS.md #49）。
//
// /add <名字> 的名字在配置里对不上（resolveAddConfig 的第 0-3 步都没命中）时，到本会话
// 的识别快照里找；找到就走与 /add +编号 同一个写配置流程（addDiscoveredModel），屏幕
// 输出与写进文件的字节都相同。快照里的每一行都能按名字加，包括拍快照时已在配置里、
// 没有编号的行（16a open item (c)）；custom/ 开头的 codex 行不进快照，加不了。本会话
// 还没有快照时先识别一次（ensureModelSnapshot，与不带参数的 /add 同一份快照）。
//
// 两种名字形式（matchDiscoveredName）：
//   - ID 形式：等于某一行在它支持的某一档强度下的 SuggestedID，强度随名字定；或等于
//     不带强度的 SuggestedID("")（没有强度可选的行只有这一种，例如 grok、agy、claude 的
//     haiku；有强度的行例如 claude 的 sonnet-5），强度与模型名形式一样另取。
//   - 模型名形式：等于某一行的 Model，或非空的 Alias；强度取 --effort 或两个词写法的
//     第二个词，都没给取 DefaultEffort。
//
// 单个名字没给强度（名字不带强度、没有第二个词、没有 --effort）也没给 --id 时，先看配置里
// 这个模型已有的成员（addname_existing.go 的 addNameExisting）：有就加入、拒绝或列出，都不写
// 配置；没有才按默认档新建。
//
// 名字按 strings.EqualFold 比，写进配置的 id 是 SuggestedID（或 --id），不是敲的原样。
// 强度一律区分大小写：--effort 与两个词写法的第二个词，都与 /add +编号 的 --effort
// 同一条判据。

// 按名字加识别出的模型的说法（§3 冻结）。都是整行，含换行。
const (
	// msgAddNameNotFound 是配置里与识别快照里都对不上时的那一句（addSegmentNotFound）。
	msgAddNameNotFound = "配置里和识别结果里都没有 %s；不带参数的 /add 列出配置里的成员与识别出的模型，识别表里的模型可以直接 /add 它的 ID\n"
	// msgAddNameUsage 是带 --id 或 --effort 时参数形状不对。
	msgAddNameUsage = "用法: /add <识别表里的 ID 或模型名> [--id <新 id>] [--effort <强度>]；带 --id 或 --effort 时只能加一个\n"
	// msgAddNameOneAtATime 是多段里有一段要按识别结果加。
	msgAddNameOneAtATime = "%s 是识别出的模型，识别出的模型一次加一个：单独 /add %s\n"
	// msgAddNameEffortTwice 是 ID 形式已带强度，又给了不同的 --effort，或后面又跟了第二个词。
	msgAddNameEffortTwice = "%s 已经带着强度 %s，不能再给强度 %s\n"
	// msgAddNameFlagsConfig 是带 --id 或 --effort、名字却在配置里命中了一个 agent（全名或编号，或只是
	// 前缀命中而名字在识别快照里对不上）。
	msgAddNameFlagsConfig = "%s 对上的是配置里的 agent %s，--id 与 --effort 只用于加识别出的模型；直接 /add %s\n"
	// msgAddNameAmbiguous 是一个名字对上快照里的多行；后面逐行列出候选。
	msgAddNameAmbiguous = "%s 对上了识别出的 %d 个模型，不猜；用 +编号 或带强度的 ID 指明:\n"
)

// SetAddByName 打开或关上「/add 按名字加识别出的模型」（AD1）。零值 false 是关：/add
// 不按名字识别、不多起任何进程、说法与 AD1 之前逐字节相同（只有 splitAddArgs 按全角
// 空格切段这一条不看开关），所以直接 New 出来的 REPL（既有测试）行为不变；app 装配时在
// TUI 与行式两处都打开。/join 与 /add 是同一个 handler，同样生效。只许在 Run 或第一次
// Submit 之前调。
func (r *REPL) SetAddByName(on bool) {
	r.addByName = on
}

// nameMatch 是识别快照里按名字对上的一行。byID 为真表示按 ID 形式对上，effort 是名字
// 里带的那一档（不带强度的 SuggestedID("") 对上时是空串）；模型名形式对上时 byID 为假、
// effort 为空串。effort 为空串时，两种形式的强度都由 addByOneName 另取。
type nameMatch struct {
	row    snapRow
	byID   bool
	effort string
}

// matchDiscoveredName 在快照里按名字找行，按快照的顺序返回，每一行至多一次。snap 为
// nil 时一行都没有。一行先按 ID 形式比（idFormEffort），对不上再按模型名形式比 Model
// 与非空的 Alias。codex 的 SuggestedID("") 多半就是 Model，这时它的模型名按 ID 形式
// 对上，强度同样另取，结果相同。
func matchDiscoveredName(snap *modelSnapshot, name string) []nameMatch {
	if snap == nil {
		return nil
	}
	var out []nameMatch
	for _, row := range snap.rows {
		m := row.m
		if e, ok := idFormEffort(m, name); ok {
			out = append(out, nameMatch{row: row, byID: true, effort: e})
			continue
		}
		if strings.EqualFold(name, m.Model) || (m.Alias != "" && strings.EqualFold(name, m.Alias)) {
			out = append(out, nameMatch{row: row})
		}
	}
	return out
}

// idFormEffort 报告 name 是不是 m 的 ID 形式；是的话返回名字里带的那一档。有强度的行
// 先按 Efforts 的顺序逐档比 SuggestedID(档)，取第一个对上的；都对不上再比不带强度的
// SuggestedID("")，没有强度可选的行只比这一个。按 SuggestedID("") 对上时返回空串。
func idFormEffort(m provider.DiscoveredModel, name string) (string, bool) {
	for _, e := range m.Efforts {
		if strings.EqualFold(name, m.SuggestedID(e)) {
			return e, true
		}
	}
	return "", strings.EqualFold(name, m.SuggestedID(""))
}

// addNameArgs 是 --id 与 --effort 两个参数；have 为假时对应的值不看。
type addNameArgs struct {
	newID      string
	haveID     bool
	effort     string
	haveEffort bool
}

// cmdAddByName 是 cmdAdd 在开关打开时先走的一步。done 为假表示这条命令与按名字加无关
// （没有 --id / --effort，而且每一段都在配置里命中），cmdAdd 照 AD1 之前的做法接着解析，
// 行为一个字节不变。done 为真时这条命令已经处理完（加了、拒了、或识别被取消），err
// 只来自写屏幕与存储。
//
// 顺序冻结：
//  1. fields 里有恰好等于 --id 或 --effort 的词：参数形状（每个参数至多一次、后面要有值、
//     恰好一个名字，否则打用法）；名字是 + 加数字时打 +编号 的用法（+编号 只在第一个词
//     时才认）；名字在配置里命中一句拒绝就打那一句；按全名（或编号）命中一个 agent 就说
//     这两个参数只用于识别出的模型；只是前缀命中时先识别（第 3 步），名字在快照里对得上
//     就按识别结果加，对不上照样说那一句；没命中接第 3 步。
//  2. 没有：逐段在配置里解析（resolveAddConfig），全部命中返回 done 为假。命中与否只有
//     一个判据（miss），单段、多段、两个词三处都用它。
//  3. 本会话还没有快照、配置里又有能识别的 provider，就识别一次（ensureModelSnapshot）；
//     被取消只有「识别被取消」那一行，什么都不加。带参数的那一个名字接 addByOneName。
//  4. 两个词的写法：恰好两段，第二段没命中配置，第一段没命中配置或者在配置里只是前缀
//     命中（configPrefixOnly），第一段在快照里对上，而且第二段恰好是对上的某一行支持的
//     一档强度（区分大小写，几行取并集）：第二段当强度，接 addByOneName。第一段没命中
//     配置、对上的行里至少一行有强度可选、第二段在快照里也对不上任何行、不以 + 开头、
//     也不是在场成员或总结成员的前缀（addSegmentNotFound 打的是「都没有」那一句）时，
//     第二段多半是敲错的强度，同样接 addByOneName，由它打现成的说法（addDiscoveredModel
//     那句「支持的强度是」，对上多行时列候选，ID 形式已带强度时是「已经带着强度」），
//     不写配置。
//  5. 只有一段：接 addByOneName。
//  6. 其余多段：全或无，一个都不加，逐段按出现顺序打说法：命中配置的照配置的说法（加入
//     成功的那种不打），在快照里对上的打「识别出的模型一次加一个」，都对不上的打
//     addSegmentNotFound。
func cmdAddByName(ctx context.Context, r *REPL, fields, segs []string, candidates []string, active, member map[string]bool) (bool, error) {
	flagged := false
	for _, f := range fields {
		if f == "--id" || f == "--effort" {
			flagged = true
			break
		}
	}
	if flagged {
		name, a, ok := parseAddNameArgs(fields)
		if !ok {
			return true, r.printf("%s", msgAddNameUsage)
		}
		if strings.HasPrefix(name, "+") && isASCIIDigits(name[1:]) {
			return true, r.printf("%s\n", msgAddModelUsage)
		}
		prefixID := ""
		if id, problem := resolveAddConfig(r, name, candidates, active, member); problem != "" {
			return true, r.printf("%s", problem)
		} else if id != "" {
			if !configPrefixOnly(name, id) {
				return true, r.printf(msgAddNameFlagsConfig, name, id, id)
			}
			prefixID = id
		}
		if canceled, err := snapshotForName(ctx, r); canceled || err != nil {
			return true, err
		}
		if prefixID != "" && len(matchDiscoveredName(r.modelSnap, name)) == 0 {
			return true, r.printf(msgAddNameFlagsConfig, name, prefixID, prefixID)
		}
		return true, addByOneName(ctx, r, name, "", a, active, member)
	}

	ids := make([]string, len(segs))
	problems := make([]string, len(segs))
	miss := func(i int) bool { return ids[i] == "" && problems[i] == "" }
	anyMiss := false
	for i, seg := range segs {
		ids[i], problems[i] = resolveAddConfig(r, seg, candidates, active, member)
		if miss(i) {
			anyMiss = true
		}
	}
	if !anyMiss {
		return false, nil
	}
	if canceled, err := snapshotForName(ctx, r); canceled || err != nil {
		return true, err
	}

	if len(segs) == 2 && miss(1) && (miss(0) || configPrefixOnly(segs[0], ids[0])) {
		ms := matchDiscoveredName(r.modelSnap, segs[0])
		typo := miss(0) && anyRowHasEfforts(ms) && len(matchDiscoveredName(r.modelSnap, segs[1])) == 0
		// 第二段以 + 开头，或是在场成员、总结成员的前缀时，它指的是 +编号 或配置里的人，
		// 不当敲错的强度，照多段说（W1b minor 1）。
		typo = typo && !strings.HasPrefix(segs[1], "+") && addSegmentNotFound(r, segs[1], active, member) == fmt.Sprintf(msgAddNameNotFound, segs[1])
		if effortOfAny(ms, segs[1]) || typo {
			return true, addByOneName(ctx, r, segs[0], segs[1], addNameArgs{}, active, member)
		}
	}
	if len(segs) == 1 {
		return true, addByOneName(ctx, r, segs[0], "", addNameArgs{}, active, member)
	}
	for i, seg := range segs {
		problem := problems[i]
		if miss(i) {
			if len(matchDiscoveredName(r.modelSnap, seg)) > 0 {
				problem = fmt.Sprintf(msgAddNameOneAtATime, seg, seg)
			} else {
				problem = addSegmentNotFound(r, seg, active, member)
			}
		}
		if problem == "" {
			continue
		}
		if err := r.printf("%s", problem); err != nil {
			return true, err
		}
	}
	return true, nil
}

// configPrefixOnly 报告 seg 在配置里只是按前缀命中了 id（resolveAddConfig 的第 3 步给出了
// 唯一的候选）：不是全名（全名命中时 id 等于 seg）、不是编号（全是数字的段走第 2 步）。
// 总结成员、只是成员、前缀歧义与超出范围的编号都走 problem，id 是空串，不算。
func configPrefixOnly(seg, id string) bool {
	return id != "" && id != seg && !isASCIIDigits(seg)
}

// effortOfAny 报告 e 是不是 ms 里任意一行支持的一档（区分大小写，几行取并集）。
func effortOfAny(ms []nameMatch, e string) bool {
	for _, mt := range ms {
		if hasEffort(mt.row.m, e) {
			return true
		}
	}
	return false
}

// anyRowHasEfforts 报告 ms 里是否至少一行有强度可选（claude、codex 的多数行）。
func anyRowHasEfforts(ms []nameMatch) bool {
	for _, mt := range ms {
		if len(mt.row.m.Efforts) > 0 {
			return true
		}
	}
	return false
}

// parseAddNameArgs 从 fields 里取出 --id、--effort 与唯一的名字。每个参数至多一次、
// 后面要有值；其余的词都是名字，恰好一个，而且按 /add 的分隔符（splitAddArgs）切出来
// 也恰好一段（a,b --effort x 算两个名字）。
func parseAddNameArgs(fields []string) (string, addNameArgs, bool) {
	var a addNameArgs
	var names []string
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "--id":
			if a.haveID || i+1 >= len(fields) {
				return "", addNameArgs{}, false
			}
			a.haveID = true
			a.newID = fields[i+1]
			i++
		case "--effort":
			if a.haveEffort || i+1 >= len(fields) {
				return "", addNameArgs{}, false
			}
			a.haveEffort = true
			a.effort = fields[i+1]
			i++
		default:
			names = append(names, splitAddArgs(fields[i])...)
		}
	}
	if len(names) != 1 {
		return "", addNameArgs{}, false
	}
	return names[0], a, true
}

// snapshotForName 是按名字加用的「没有快照就识别一次」：配置里一家能识别的都没有时
// 不识别（快照留 nil，按名字一个都对不上）。已有快照直接用，不重拍。
func snapshotForName(ctx context.Context, r *REPL) (bool, error) {
	if r.modelSnap != nil {
		return false, nil
	}
	pids := discoverableProviders(r.cfg)
	if len(pids) == 0 {
		return false, nil
	}
	return ensureModelSnapshot(ctx, r, pids)
}

// hasEffort 报告 e 是不是 m 支持的一档（区分大小写）。
func hasEffort(m provider.DiscoveredModel, e string) bool {
	for _, x := range m.Efforts {
		if x == e {
			return true
		}
	}
	return false
}

// addByOneName 按一个名字加识别出的模型。second 是两个词写法的第二个词（一档强度，或
// cmdAddByName 第 4 步认定的敲错的强度），没有时是空串。顺序冻结：在快照里找（一行都
// 没有打 addSegmentNotFound；多行整条拒绝、逐行列出候选），没给强度也没给 --id 时先看
// 配置里这个模型已有的成员（addNameExisting，有就不写配置），名字里带的强度与 --effort
// 或第二个词对账，Runner 在不在，然后交给 addDiscoveredModel（强度、同脑子、id、配置
// 文件路径、守卫、追加、加载校验、换指针、Join）。第二个词不是那一行的一档强度时，由
// addDiscoveredModel 的强度那一步拒绝，不写配置。
//
// ID 形式带了强度时：给了不同的 --effort 报错；给了相同的照常；后面又跟了第二个词，
// 相同也报错（两个词的写法只给模型名形式与不带强度的 ID 形式用）。
func addByOneName(ctx context.Context, r *REPL, name, second string, a addNameArgs, active, member map[string]bool) error {
	ms := matchDiscoveredName(r.modelSnap, name)
	switch {
	case len(ms) == 0:
		return r.printf("%s", addSegmentNotFound(r, name, active, member))
	case len(ms) > 1:
		if err := r.printf(msgAddNameAmbiguous, name, len(ms)); err != nil {
			return err
		}
		for _, mt := range ms {
			no := ""
			if mt.row.no > 0 {
				no = fmt.Sprintf("  +%d", mt.row.no)
			}
			if err := r.printf("  %-6s  %s%s\n", mt.row.m.Provider, mt.row.m.Model, no); err != nil {
				return err
			}
		}
		return nil
	}
	mt := ms[0]
	if second == "" && !a.haveEffort && !a.haveID && !(mt.byID && mt.effort != "") {
		if done, err := addNameExisting(ctx, r, name, mt.row.m, active); done {
			return err
		}
	}
	effort, haveEffort := a.effort, a.haveEffort
	if mt.byID && mt.effort != "" {
		if second != "" {
			return r.printf(msgAddNameEffortTwice, name, mt.effort, second)
		}
		if haveEffort && effort != mt.effort {
			return r.printf(msgAddNameEffortTwice, name, mt.effort, effort)
		}
		effort, haveEffort = mt.effort, true
	} else if second != "" {
		effort, haveEffort = second, true
	}
	if r.rn == nil {
		return r.printf("%s\n", msgNoRunner)
	}
	return addDiscoveredModel(ctx, r, mt.row.m, effort, haveEffort, a.newID, a.haveID, name)
}
```

#### 块 C：`internal/app`（两处补丁；依赖块 B-甲与块 B-乙）

**C1. `internal/app/app.go`：TUI 分支打开开关**（HEAD `:255-256`）

```anchor internal/app/app.go
		r.SetLoginStatusExec(turn.RunLoginStatus)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
```

```replace
		r.SetLoginStatusExec(turn.RunLoginStatus)
		r.SetAddByName(true)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
```

**C2. `internal/app/app.go`：行式分支打开开关**（HEAD `:263-264`）

```anchor internal/app/app.go
		r.SetLoginStatusExec(turn.RunLoginStatus)
		err = r.Run(ctx)
```

```replace
		r.SetLoginStatusExec(turn.RunLoginStatus)
		r.SetAddByName(true)
		err = r.Run(ctx)
```

### 1.2 不做

- 不改识别本身（`internal/provider` 的任何文件，包括 `SuggestedID`、`DefaultEffort`、各家解析器）；不改识别表、表下提示行、custom 行、失败行、`--refresh` 提示、登录段、「正在识别模型: …」那一行。
- 不改 `/add +编号` 的行为（`cmdAddDiscovered` 只把后半段交给共用函数，屏幕与文件逐字节不变，MD1、CXH 的用例全绿）；不改 `/add --refresh`、不带参数的 `/add`。
- 不允许加识别结果里没有的模型（不做「盲写模型名」）：快照里对不上就是 D13 那一句。`custom/` 行照旧加不了。
- 不加新命令、新 flag；不改 `/help`（D14）；不改权限默认值（新成员仍是 read、discussion）。
- 不改 `internal/provider`、`internal/turn`、`internal/config`、`internal/tui`、`internal/store`、`internal/conversation`、`cmd/council` 的任何文件；`internal/repl` 只改 `repl.go`、`addmodel.go`、`commands.go`，加 `addname.go` 与 `addname_existing.go`（v1.2）；`internal/app` 只改 `app.go`。
- **既有 `_test.go` 一个都不改**（mdx、cxh、obx、obz、cox、ux2r、chx 的装置只读复用）。

---

## 2. 验收（测试腿新写的文件）

两个新文件：`internal/repl/adn_addname_test.go`（S1，`package repl`）、`internal/app/adn_addname_app_test.go`（S2，`package app`）。包级标识符一律 `adn` 开头；**不许新增 `TestMain`**；不许改任何既有测试文件。两个文件照下面逐字写（§4 按名字点杀手）。用例编号 N1-N17、N22-N25 在 repl，N20、N21 在 app；N18、N19 空着不用（N16、N17 是 v1.1 加的，N22-N24 是 v1.2 加的，N25 是 v1.3 加的）。

纪律（与 19d §2 开头相同）：
- 字面量整行相等：屏幕输出整行 `==` 比较（`mdxWantLines` 逐行）；写配置的用例核文件字节（原文加冻结的那一段）、新出现恰好一个备份且字节等于原文、新成员在场（`adnAdded`）；拒绝的用例核屏幕整行、文件字节不变、备份个数不变、在场成员不变（`adnRefused`）。只有 N21（TUI）屏幕比的是去掉 ANSI 与空格之后的包含关系（bubbletea 渲染，没有逐字节的整行），文件照样按字节比；N20 的会话 id 与备份时间戳按形状比（正则），其余整段相等。
- 每条都能为假：每个测试写明它对哪条变异红（【判别力】，§4 的「杀手」列是原型实测）；缺席断言配同一装置的正对照（N1 关着不识别、不写，同一配置打开开关的装置写了；N9 同一装置关上开关是对照；N6 cross_provider 里带强度的 ID 在同一份快照上照常加入；N2 plus 与 id 同一份期望；v1.1 的 N5 refusal_* 与 N7 all_config_refusal 在同一装置上接着按名字加、识别了两家；N8 no_providers 的「不识别」以 snapshot_kept 在有 provider 的配置上打出「正在识别模型」为对照，C103 这一类变异由整行说法里多出的空列表「正在识别模型: 」判红）。
- repl 的用例不起任何进程：识别用 `mdxExec`（`r.discover`），登录状态用 `adnLoginExec`。开关用 `SetAddByName` 打开。
- v1.2：D17 的「加入、不写配置」用 `adnJoinedNoWrite` 核（屏幕整行、文件字节与备份个数不变、那个成员在场）；拒绝与列出照旧用 `adnRefused`。D17 的几条缺席断言各配同一装置的正对照：repeat 与 one_absent_renamed 第一次加入、第二次拒绝；several_absent 与 one_absent_config 接着带强度新建、写了；summary_excluded 接着按名字加别的、写了。N23 prefix_no_match 的「照旧打那一句」由 effort_over_prefix（同一种前缀命中、快照里对得上）对照。v1.2 的 S1 在 v1.1 的生产代码上跑过一次（新解出的 HEAD 落 v1.1 的全部冻结代码，再换上 v1.2 的 S1）：改了行为的那些全红（N9 父测试、N15 alias_configured、N22 的七条、N23 的四条、N24 的四条）；照旧绿的是三条本来就该过的正对照（N22 prefix_other_model、summary_excluded，N23 id_only_present），以及只补覆盖、不改行为的 N6 typo_second_row、N9 summary_prefix、N14 plus_letters 与改成带 `--effort` 的 N15 alias_same_brain（它们的判别力由 V67、V69、V68、V81 担保）。
- v1.3（W1c 的五条 minor）：新加的都只补覆盖、钉现状，生产代码没改，所以没有「在旧代码上该红」的一轮；判别力由新登记的 V87-V92 与 V29 担保（原型实测）。缺席断言的正对照：N25 第一次「不识别、不写」，同一装置第二次识别、写了；N14 新加的 `/add 1 --effort high`（执行器 0 次）的正对照是 N23 prefix_no_match：同一种装置（`adnNewRig`）上，只是前缀命中的 `/add grok --id x` 识别了两家；N22 present_among_absent、present_among_absent_grok 的「不列不在场的」是整行相等里少了一个 id，由 V87、V88（列出整个 S）判红；other_provider_same_model 的「gx 不在场」配同一条命令写了 `gpt-6.1-sol-high`。one_absent_config 的夹具多了一行 `display_name = 'Sol A'`：配置不写显示名时它是空串，V89（写成空串）看不出来。
- app 的用例一律经 `obxIsolate`（第一件事是 `obxGuardRealHome`）；行式经 `obxRun`，TUI 照 Z11 用 `obxDrain` 收尾；假 codex 是临时目录里只用 cmd 内建命令的 `.cmd`，不设 `COUNCIL_FAKE_CLI`，不调用任何真 CLI；显式配置，不走首次运行（`errOut` 为空）。

### 2.1 `internal/repl/adn_addname_test.go`（S1）

- **N1 `TestAdnSwitchOffUnchanged`**：`New` 出来开关是关；setter 开、关。关着的装置：三条命令（ID 形式、两个词、带 `--effort`）逐字是 HEAD 的旧句，执行器 0 次、没有快照、文件不变。同一配置、同一条命令，开着的装置加入了（正对照）。
- **N2 `TestAdnAddByID`**：`id`（新会话第一次就敲 `/add gpt-6.1-sol-xhigh`，识别两家、写、加入）、`plus`（先 `/add` 再 `/add +1 --effort xhigh`，与 id 同一份期望）、`join`（`/join` 同样）、`fold`（大小写）。
- **N3 `TestAdnTwoWords`**：`model_effort`（`/add gpt-6.1-sol xhigh`，与 N2 同一段字节）；拒绝的十一种（upper、digit、typo、id_then_effort、id_then_other、id_then_typo、ambiguous_typo、second_is_model、no_efforts、config_first、effort_is_config_prefix）。v1.1：typo、upper 按裁决 (a) 改打强度那一句；id_then_typo、ambiguous_typo、second_is_model 是新加的。
- **N4 `TestAdnModelForm`**：加入的七种（default_effort、id_high、effort_flag、flag_first、id_flag、id_same_effort、grok）；拒绝的六种（id_other_effort、unsupported、effort_case、grok_effort、ambiguous_with_effort、no_efforts_codex）。v1.1：no_efforts_row 名不副实（它测到的是歧义），改名 ambiguous_with_effort；另加 no_efforts_codex（CXH 整份夹具的 `cxh-bell`：codex 里没有强度、也没有歧义的一行，`/add cxh-bell --effort high` 打「cxh-bell 没有可选的强度，不能带 --effort」）。
- **N5 `TestAdnConfiguredRow`**：拍快照时已在配置里、没有编号的 `gpt-5.5` 加另一档（other_effort）；配置前缀优先、不识别（prefix）；同脑子（same_brain）。v1.1（W1 的 major）：开关打开时配置里的拒绝型命中，四条各一个新装置：refusal_out_of_range（`/add 99`）、refusal_ambiguous（`/add g`）、refusal_summary（`cxhNewRig`，配置是 `cxhConfigTemplate` 末尾加 `[summary] agent = 'grok-4.7'`，`/add grok-4.7`）、refusal_member_only（`adnNewRig` 之后用本文件的 `adnSeedMember` 经 `conversation.Service.Join` 塞一个不在配置里的成员 `gpt-6.1-sol`，`/add gpt-6.1-sol`）；各自断言整行说法、执行器 0 次（`adnNoDiscovery`）、没有「正在识别模型」，同一装置接着 `/add gpt-6.1-sol-xhigh` 识别并加入是正对照。
- **N6 `TestAdnAmbiguous`**：同一家、两个词、跨两家、带没有编号的行；v1.1 的 union_second_row（裁决书 m5：只给它用的夹具 `adnCodexIDFirst`，没有强度的 adn-x-high 排在 adn-x 之前；配置多一个 `adn-x-high-old`，第一段在配置里只是前缀命中；`/add adn-x-high high` 列候选）；v1.2 的 typo_second_row（W1b minor 4，同一份夹具、不加配置前缀，`/add adn-x-high xigh` 列候选，杀 V67）。
- **N7 `TestAdnMultiSegment`**：一次加一个（with_config、three），每段都在配置里命中时照旧加入、不识别（all_config）；v1.1：all_config_refusal（`/add grok-4.7,99`，只打范围那一句、执行器 0 次，新装置加正对照）、ambiguous_seg（一段对上多行也是「一次加一个」）。
- **N8 `TestAdnNotFound`**：新的一句、快照不重拍、custom/ 加不了（snapshot_kept）；codex 识别失败时说法不变、失败行不打、grok 照常能加（codex_failed）；v1.1：配置里一家能识别的都没有时不识别（no_providers）。
- **N9 `TestAdnPriority`**：快照里的精确命中排在「开头的都已经在」之前；快照里也对不上时照旧落到那一句；同一装置关上开关是对照。v1.2：`grok-4.6-a` 在场时 `/add grok-4.6` 的期望从同脑子那一句改成 D17 的「grok-4.6 对上的模型已经有成员在当前会话里：grok-4.6-a」（裁决书「授权改动的期望」，取原型实测）；另加 summary_prefix（W1b minor 4：开关打开、配置末尾加 `[summary] agent = 'grok-4.7'`，`/add gro` 打总结成员前缀那一句，杀 V69）。
- **N10 `TestAdnCanceled`**：自动识别期间取消（落在第一家、最后一家），之后重新识别并加入；v1.1：带 `--effort` 那条路取消落在第一家（flag_not_last），恰好两行、快照为 nil。
- **N11 `TestAdnSharesSnapshot`**：按名字加拍的快照（含登录状态）就是不带参数的 `/add` 用的那一份；反过来也一样。
- **N12 `TestAdnGuard`**：守卫那一句里「再 /add」后面的那一段（三种敲法）。
- **N13 `TestAdnNoRunner`**：Runner 不在。
- **N14 `TestAdnFlags`**：参数形状与名字在配置里命中（十一条，一次都不识别；v1.1 加 `/add --id x +1` 打 +编号 的用法）。v1.2：`/add grok --id x` 挪出去（grok 在配置里只是前缀命中，D9 的 v1.2 一条要先识别，挪进 N23 prefix_no_match），剩十条；另加 plus_letters（W1b minor 4，新装置，`/add --id x +abc` 识别之后打「都没有 +abc」，杀 V68）。v1.3（W1c 的 W7）：表里加 `/add 1 --effort high`（编号命中 `gpt-5.5-high`，打「1 对上的是配置里的 agent gpt-5.5-high，--id 与 --effort 只用于加识别出的模型；直接 /add gpt-5.5-high」，执行器 0 次），回到十一条，杀 V92。
- **N15 `TestAdnOtherProviders`**：MD1 的配置（agy、claude、grok）：claude 的别名、没有 high 时取第一档、claude 的 ID、agy 的 id、haiku 带 --effort、别名同脑子、不带强度的 claude ID 段；v1.1：不带强度的 claude ID 段按默认档加（claude_base）、跟第二个词按那一档加（claude_base_second），大小写（claude_alias_fold `/add SONNET[1m]`、haiku_fold `/add HAIKU-4.5`），claude 的 Model（claude_model、claude_model_fold）。v1.2：alias_same_brain 改成 `/add opus[1m] --effort high`（给了强度，照旧打同脑子那一句）；另加 alias_configured（`/add opus[1m]` 不给强度，按 D17 第 3 种加入 `opus-5.0`、不写配置）。v1.1 的 alias_same_brain 敲的是不带强度的 `/add opus[1m]`，D17 之后它的结局按裁决书规则二第 4 步必然变成加入，这正是裁决书「必须有的用例」第 5 条的形状（别名形式、恰好一个不在场），所以这一处期望跟着改；改动在变更记录里点名。
- **N16 `TestAdnTwoWordsConfigPrefix`**（v1.1，裁决 (b)）：同一装置先按名字加 `gpt-6.1-sol-xhigh` 再 `/leave`；`/add gpt-6.1-sol xhigh` 打同脑子那一句、不写；`/add gpt-6.1-sol xigh` 与 `XHIGH` 只打「都没有」（敲错的强度那一支只给配置里没命中的第一段）；`/add gpt-6.1-sol high` 新建 `gpt-6.1-sol-high`；单段 `/add gpt-6.1-sol` 仍按配置前缀加入 `gpt-6.1-sol-xhigh`；执行器共 2 次。
- **N17 `TestAdnIdeographicSpace`**（v1.1）：全角空格切段。开着：`/add gpt-6.1-sol<U+3000>xhigh` 加入（on_two_words）、`/add gpt-6.1-sol<U+3000>--effort<U+3000>xhigh` 加入（on_flag，实测 `args.fields` 在 U+3000 上切）；关着：`/add grok-4.7<U+3000>gpt-5.5-high` 两个都加入、不识别（off_config）。源码里全角空格写成 `"\xe3\x80\x80"`（`adnIdeoSpace`），不写字面字符。
- **N22 `TestAdnExistingMembers`**（v1.2，D17）：repeat（裁决书用例 1：`/add gpt-6.1-sol-xhigh`、`/leave`，第一次 `/add gpt-6.1-sol` 加入 xhigh、不写，第二次打拒绝句、不写、备份数不变、在场只有 xhigh）；prefix_other_model（用例 2，新夹具 `adnCodexSol61`：gpt-6.1-sol 与 gpt-6.1 两个模型，xhigh 在场时 `/add gpt-6.1` 仍写出 `gpt-6.1-high` 并加入）；alias_present、alias_config_present（用例 3：`/add sonnet[1m] xhigh` 加进来之后 `/add sonnet[1m]`；`opus-5.0` 在场时 `/add opus[1m]`）；several_absent、several_absent_grok（用例 4：配置末尾加两个同模型成员，列出，前者带强度后半句、后者不带）；one_absent_config、one_absent_renamed（用例 5：配置里起的名字 `sol-a`，`--id` 起的名字 `sol-x`，加入、不写）；summary_excluded（`[summary] agent = 'grok-4.7'`，`/add GROK-4.7` 不把总结成员当普通成员加入，落到同脑子那一句）。v1.3（W1c 的 W1-W5）：present_among_absent（`adnTwoSolAgents`，`/add sol-a` 之后 `/add gpt-6.1-sol` 打「gpt-6.1-sol 对上的模型已经有成员在当前会话里：sol-a；要另加一档强度，……」，不列不在场的 sol-b，杀 V87）；present_among_absent_grok（两个 g46 成员，`/add g46-a` 之后 `/add grok-4.6` 打「grok-4.6 对上的模型已经有成员在当前会话里：g46-a」，杀 V88）；other_provider_same_model（配置末尾加 `[agents.gx] provider = 'grok' model = 'gpt-6.1-sol'`，`/add gpt-6.1-sol` 识别之后新建 codex 的 `gpt-6.1-sol-high`、写配置，gx 不在场，杀 V91）；one_absent_config 的夹具加 `display_name = 'Sol A'`，加入之后断言会话成员记录里 sol-a 的显示名是 `Sol A`、provider 是 `codex`（杀 V89、V90）。前两条的期望第一行都是「正在识别模型: codex, grok」（`/add sol-a`、`/add g46-a` 按全名命中，不识别）。
- **N25 `TestAdnSingleIDConfigPrefix`**（v1.3，§7 (w)，W1c 的 w2 装置）：配置末尾加 `[agents."gpt-6.1-sol-xhigh-old"] provider = 'codex' model = 'gpt-5.5' effort = 'xhigh'`（另一个模型）。第一次 `/add gpt-6.1-sol-xhigh` 按配置前缀加入 `gpt-6.1-sol-xhigh-old`、回显它的 id、不写、执行器 0 次（钉住 HEAD 的前缀语义，杀 V29）；同一装置第二次同一条命令是正对照：-old 在场、不再是候选，识别两家、新建 `gpt-6.1-sol-xhigh`、写配置（同一条命令敲两次结果不同）。
- **N23 `TestAdnFlagsOverConfigPrefix`**（v1.2，D9，裁决书用例 6、7）：effort_over_prefix、effort_over_other_model（配置里有不在场的 `gpt-6.1-sol-xhigh`，或另一个模型的 `gpt-6.1-sol-mini`，`/add gpt-6.1-sol --effort high` 都新建 `gpt-6.1-sol-high`）；id_over_prefix（只有 `--id`，同样按识别结果新建 `sol`）；id_only_present（用例 7：xhigh 在场，`/add gpt-6.1-sol --id sol` 照样新建、写配置）；prefix_no_match（`/add grok --id x` 先识别、再打「对上的是配置里的 agent grok-4.7」，`/add grok --effort high` 同一句、不再识别）。
- **N24 `TestAdnTypoIsReference`**（v1.2，W1b minor 1 的四种情形）：present_prefix（H：grok-4.7 在场，`/add gpt-6.1-sol grok`）、id_present_prefix（H2：`/add gpt-6.1-sol-xhigh grok`）、summary_prefix（H3：总结成员 grok-4.7 没加入，`/add gpt-6.1-sol gro`）、plus（C_plus_second：`/add gpt-6.1-sol +1`）。四条都照多段每段一行，不写配置。

```file internal/repl/adn_addname_test.go
package repl

// adn_addname_test.go 是 AD1（00_DECISIONS.md #49：/add 按名字加识别出的模型）在 repl 包的验收，契约 §2.1。
// 装置只读复用 mdx_addmodel_fixture_test.go 与 cxh_fixture_test.go（mdxNewRig、mdxNewRigOpts、mdxRigOpts、
// mdxRig、mdxResp、mdxWantLines、mdxHasLine、mdxReplaceOnce、mdxConfigTemplate、mdxGrokFixture、mdxGrokPath、cxhNewRig、
// cxhConfigTemplate、cxhCodexPath、cxhCodexFixture）。repl 的用例不起任何进程：识别用 mdxExec（r.discover），登录状态用
// 本文件的 adnLoginExec。开关用 SetAddByName 打开；不调它的装置就是开关关着（N1 的对照）。v1.1（契约变更记录）
// 加了 N16、N17 与若干子用例；v1.2 加了 N22-N24 与若干子用例，改了 N9、N14 的一条与 N15 alias_same_brain；
// v1.3 加了 N25、N14 的一条、N22 的三个子用例与 one_absent_config 的成员记录断言（夹具多一行 display_name）；
// 【判别力】里的变异编号与杀手是 v1.3 原型上的实测。

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
)

// adnCodexOut 是合成的 codex debug models 输出（不是真输出）：gpt-6.1-sol 照 Griffin 这次敲的名字取；
// gpt-5.5 在 cxhConfigTemplate 里已以 high 配成 gpt-5.5-high（拍快照时已在配置里、没有编号）；
// adn-x（只有 high）与 adn-x-high（没有强度）的 ID 都是 adn-x-high，是同一家里的歧义；
// custom/ 那一条不进快照。
const adnCodexOut = `{"models":[` +
	`{"slug":"gpt-6.1-sol","display_name":"GPT-6.1-Sol","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},` +
	`{"slug":"gpt-5.5","display_name":"GPT-5.5","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},` +
	`{"slug":"adn-x","display_name":"Adn X","supported_reasoning_levels":[{"effort":"high"}],"visibility":"list"},` +
	`{"slug":"adn-x-high","display_name":"Adn X High","visibility":"list"},` +
	`{"slug":"custom/adn-cpa","display_name":"Adn CPA","visibility":"list"}` +
	`]}` + "\n"

// adnCodexIDFirst 只给 N6 union_second_row 用（不动 adnCodexOut，免得 +N 的编号跟着变）：adn-x-high（没有强度）
// 排在 adn-x（只有 high）之前，两行的 ID 都是 adn-x-high；两个词写法里强度取几行的并集，要看到第二行才认得 high。
const adnCodexIDFirst = `{"models":[` +
	`{"slug":"adn-x-high","display_name":"Adn X High","visibility":"list"},` +
	`{"slug":"adn-x","display_name":"Adn X","supported_reasoning_levels":[{"effort":"high"}],"visibility":"list"}` +
	`]}` + "\n"

// adnCodexSol61 只给 N22 prefix_other_model 用（不动 adnCodexOut）：gpt-6.1 与 gpt-6.1-sol 是两个模型，前者的名字
// 是后者 ID 的前缀。按 id 前缀判「同一个模型已有成员」的实现会把 /add gpt-6.1 挡在在场的 gpt-6.1-sol-xhigh 上。
const adnCodexSol61 = `{"models":[` +
	`{"slug":"gpt-6.1-sol","display_name":"GPT-6.1-Sol","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},` +
	`{"slug":"gpt-6.1","display_name":"GPT-6.1","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"}` +
	`]}` + "\n"

// adnIdeoSpace 是全角空格 U+3000 的 UTF-8 字节，写成十六进制转义（编辑链路会把 \u 转义还原成真字符）。
const adnIdeoSpace = "\xe3\x80\x80"

// adnGrokExtra 是 MD1 的 grok 夹具末尾多两个模型：gpt-6.1-sol（与 codex 的模型名相同）、gpt-5.5（与 codex 里
// 已配成 agent、没有编号的那一行相同），跨 provider 的歧义用。
const adnGrokExtra = mdxGrokFixture + "  - gpt-6.1-sol\n  - gpt-5.5\n"

// 按名字加的几种说法（契约 §3，逐字）。
const (
	adnDiscovering = "正在识别模型: codex, grok"
	adnCanceled    = "识别被取消；下次 /add 会重新识别"
)

// adnNotFound 是配置里与识别快照里都对不上时的那一行。
func adnNotFound(name string) string {
	return "配置里和识别结果里都没有 " + name + "；不带参数的 /add 列出配置里的成员与识别出的模型，识别表里的模型可以直接 /add 它的 ID"
}

// adnOldNotFound 是开关关着时（AD1 之前）的那一行。
func adnOldNotFound(name string) string {
	return "配置里没有可加入的 " + name + "，不带参数的 /add 能列出可加的"
}

// adnOneAtATime 是多段里有一段要按识别结果加时，那一段的说法。
func adnOneAtATime(name string) string {
	return name + " 是识别出的模型，识别出的模型一次加一个：单独 /add " + name
}

const adnUsage = "用法: /add <识别表里的 ID 或模型名> [--id <新 id>] [--effort <强度>]；带 --id 或 --effort 时只能加一个"

// 写进配置的几段（契约 §2.1 冻结；与 /add +编号 写出的字节相同，N2 的 plus 子测试钉这一条）。
const (
	adnBlockSolXhigh = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.\"gpt-6.1-sol-xhigh\"]\ndisplay_name = \"GPT-6.1-Sol xhigh\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"xhigh\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockSolHigh  = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.\"gpt-6.1-sol-high\"]\ndisplay_name = \"GPT-6.1-Sol\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockSolLow   = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.\"gpt-6.1-sol-low\"]\ndisplay_name = \"GPT-6.1-Sol low\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"low\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockSolX     = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.sol-x]\ndisplay_name = \"GPT-6.1-Sol xhigh\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"xhigh\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlock55Xhigh  = "\n# council /add 追加：codex 识别出的 gpt-5.5（codex 目录）\n[agents.\"gpt-5.5-xhigh\"]\ndisplay_name = \"GPT-5.5 xhigh\"\nprovider = \"codex\"\nmodel = \"gpt-5.5\"\neffort = \"xhigh\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockGrok46   = "\n# council /add 追加：grok 识别出的 grok-4.6（本地配置）\n[agents.\"grok-4.6\"]\ndisplay_name = \"Grok 4.6\"\nprovider = \"grok\"\nmodel = \"grok-4.6\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockGrok46A  = "\n# council /add 追加：grok 识别出的 grok-4.6（本地配置）\n[agents.\"grok-4.6-a\"]\ndisplay_name = \"Grok 4.6\"\nprovider = \"grok\"\nmodel = \"grok-4.6\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockSol      = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.sol]\ndisplay_name = \"GPT-6.1-Sol\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlock61High   = "\n# council /add 追加：codex 识别出的 gpt-6.1（codex 目录）\n[agents.\"gpt-6.1-high\"]\ndisplay_name = \"GPT-6.1\"\nprovider = \"codex\"\nmodel = \"gpt-6.1\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
)

// adnPresent 是没给强度的名字对上的模型已有成员在场时的那一行；hint 为真时带「要另加一档强度」的后半句。
func adnPresent(name, ids string, hint bool) string {
	s := name + " 对上的模型已经有成员在当前会话里：" + ids
	if hint {
		s += "；要另加一档强度，用 /add " + name + " <强度> 或 --effort <强度>"
	}
	return s
}

// adnConfigured 是没给强度的名字对上的模型在配置里有不止一个成员、都不在场时的那一行。
func adnConfigured(name, ids string, hint bool) string {
	s := name + " 对上的模型在配置里有这几个成员：" + ids + "；用 /add <id> 加入其中一个"
	if hint {
		s += "，或用 /add " + name + " <强度> 带强度新加一档"
	}
	return s
}

// adnFlagsConfig 是带 --id 或 --effort、名字在配置里对上一个 agent 时的那一行。
func adnFlagsConfig(name, id string) string {
	return name + " 对上的是配置里的 agent " + id + "，--id 与 --effort 只用于加识别出的模型；直接 /add " + id
}

// adnJoinedNoWrite 敲 cmd，断言屏幕整行等于 want（最后一行是 id 的加入那一行），而且没写配置：文件字节、备份个数
// 不变，id 在场。
func adnJoinedNoWrite(t *testing.T, g *mdxRig, cmd, id string, want []string) {
	t.Helper()
	orig := g.readFile()
	baks := g.bakNames()
	mdxWantLines(t, cmd, g.run(cmd), append(want, id+" 已加入会话 "+g.conv.ID))
	if got := g.readFile(); string(got) != string(orig) {
		t.Errorf("%s：配置文件不该变\n实际 %q", cmd, got)
	}
	if got := g.bakNames(); !reflect.DeepEqual(got, baks) {
		t.Errorf("%s：不该多出备份，之前 %q 之后 %q", cmd, baks, got)
	}
	if !mdxHasLine(g.activeIDs(), id) {
		t.Errorf("%s：%s 应当在场，在场的是 %q", cmd, id, g.activeIDs())
	}
}

// adnNewRig 是 CXH 的配置（codex、grok 两家；grok-4.7、gpt-5.5-high 已配成 agent）加 adnCodexOut；
// on 为真时打开开关。
func adnNewRig(t *testing.T, on bool) *mdxRig {
	t.Helper()
	g := cxhNewRig(t, cxhConfigTemplate, adnCodexOut)
	if on {
		g.r.SetAddByName(true)
	}
	return g
}

// adnAdded 敲 cmd，断言这一次写进了一段、加入了 id：屏幕整行相等（discover 为真时第一行是
// 「正在识别模型」），文件等于原文加 block，新出现恰好一个备份、字节等于原文，id 在场。
func adnAdded(t *testing.T, g *mdxRig, cmd, id, block string, discover bool) {
	t.Helper()
	orig := g.readFile()
	baks := g.bakNames()
	lines := g.run(cmd)
	bak := g.newBak(baks)
	var want []string
	if discover {
		want = append(want, adnDiscovering)
	}
	mdxWantLines(t, cmd, lines, append(want, g.joinedLines(id, bak)...))
	if got := string(g.readFile()); got != string(orig)+block {
		t.Errorf("%s：配置文件不等\n实际 %q\n期望 %q", cmd, got, string(orig)+block)
	}
	if b, err := os.ReadFile(bak); err != nil || string(b) != string(orig) {
		t.Errorf("%s：备份 %s 应当是原文（err %v）", cmd, bak, err)
	}
	if !mdxHasLine(g.activeIDs(), id) {
		t.Errorf("%s：%s 应当在场，在场的是 %q", cmd, id, g.activeIDs())
	}
}

// adnRefused 敲 cmd，断言屏幕整行等于 want，而且什么都没变：配置文件字节、备份个数、在场成员。
func adnRefused(t *testing.T, g *mdxRig, cmd string, want []string) {
	t.Helper()
	orig := g.readFile()
	baks := g.bakNames()
	active := g.activeIDs()
	mdxWantLines(t, cmd, g.run(cmd), want)
	if got := g.readFile(); string(got) != string(orig) {
		t.Errorf("%s：配置文件不该变\n实际 %q", cmd, got)
	}
	if got := g.bakNames(); !reflect.DeepEqual(got, baks) {
		t.Errorf("%s：不该多出备份，之前 %q 之后 %q", cmd, baks, got)
	}
	if got := g.activeIDs(); !reflect.DeepEqual(got, active) {
		t.Errorf("%s：在场成员不该变，之前 %q 之后 %q", cmd, active, got)
	}
}

// adnLoginExec 是登录状态执行器：只数次数，一律答 codex 已登录（ChatGPT）。
type adnLoginExec struct{ n int }

func (e *adnLoginExec) run(ctx context.Context, c provider.DiscoveryCommand) (int, []byte, []byte, error) {
	e.n++
	return 0, nil, []byte("Logged in using ChatGPT\n"), nil
}

// adnSeedMember 往当前会话塞一个在场、却不在配置里的成员，搭 TestUx2rAddB3GoneFromConfigMember 那种「从配置
// 删掉、仍是成员」的状态。ux2r 的 seedActive 挂在 ux2rFix 上，mdxRig 用不了；这里照 chxAddRig.join 的写法，
// 经 conversation.Service.Join 直接塞。不经 /join：/join 与 /add 是同一个 handler，只加配置里的 agent。
func adnSeedMember(t *testing.T, g *mdxRig, id, prov string) {
	t.Helper()
	p := conversation.Participant{ConvID: g.conv.ID, AgentID: id, Provider: prov, Active: true, JoinedAt: time.Now().UTC()}
	if err := g.svc.Join(g.ctx, g.conv.ID, p); err != nil {
		t.Fatalf("装置：塞入成员 %s 失败: %v", id, err)
	}
}

// adnNoDiscovery 断言执行器一次都没被调过（配置里命中时不识别）。
func adnNoDiscovery(t *testing.T, g *mdxRig, what string) {
	t.Helper()
	if n := g.exec.count(); n != 0 {
		t.Errorf("%s：配置里命中时不该识别，执行器被调了 %d 次", what, n)
	}
}

// TestAdnSwitchOffUnchanged 是 N1：开关零值是关；关着时 /add 的说法与 AD1 之前逐字相同、不识别、不写文件。
// 同一个配置、同一条命令，打开开关的装置是正对照：它识别了两家、写进了配置。
// 【判别力】V40（关着时也换成新说法）、V1（SetAddByName 不记下参数）；正对照 on 对 V38（cmdAdd 不接 cmdAddByName）、V32（从不自动识别）红。
func TestAdnSwitchOffUnchanged(t *testing.T) {
	if r := New(nil, nil, nil, nil, nil, RulesContext{}); r.addByName {
		t.Fatalf("New 出来的 REPL 开关应当是关")
	}
	r := New(nil, nil, nil, nil, nil, RulesContext{})
	r.SetAddByName(true)
	if !r.addByName {
		t.Errorf("SetAddByName(true) 之后开关应当是开")
	}
	r.SetAddByName(false)
	if r.addByName {
		t.Errorf("SetAddByName(false) 之后开关应当是关")
	}

	g := adnNewRig(t, false)
	adnRefused(t, g, "/add gpt-6.1-sol-xhigh", []string{adnOldNotFound("gpt-6.1-sol-xhigh")})
	adnRefused(t, g, "/add gpt-6.1-sol xhigh", []string{adnOldNotFound("gpt-6.1-sol"), adnOldNotFound("xhigh")})
	adnRefused(t, g, "/add gpt-6.1-sol --effort xhigh", []string{adnOldNotFound("gpt-6.1-sol"), adnOldNotFound("--effort"), adnOldNotFound("xhigh")})
	if n := g.exec.count(); n != 0 {
		t.Errorf("开关关着时不该识别，执行器被调了 %d 次", n)
	}
	if g.r.modelSnap != nil {
		t.Errorf("开关关着时不该有快照")
	}

	on := adnNewRig(t, true)
	adnAdded(t, on, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	if n := on.exec.count(); n != 2 {
		t.Errorf("正对照：开关打开时应当识别 codex、grok 两家，执行器被调了 %d 次", n)
	}
}

// TestAdnAddByID 是 N2：Griffin 这次的第二条命令。新会话里第一次就敲 ID 形式，先识别再写配置、加入；
// 与 /add +编号 --effort xhigh 写出的字节、打出的两行相同（plus）；/join 是同一个 handler（join）；
// 名字不分大小写，写进去的 id 是建议的小写（fold）。
// 【判别力】fold 对 V3（ID 形式按字节比）；id、join、fold 对 V17（名字里的强度不用）、V37（只认默认档的 ID）、V30、V32；plus 是对照：它与 id 是同一份期望，V47（+编号 丢掉 --effort）只有它与 MD1、CXH 的用例杀得到。
func TestAdnAddByID(t *testing.T) {
	t.Run("id", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		if n := g.exec.count(); n != 2 {
			t.Errorf("应当识别 codex、grok 各一次，执行器被调了 %d 次", n)
		}
	})
	t.Run("plus", func(t *testing.T) {
		g := adnNewRig(t, true)
		g.run("/add")
		adnAdded(t, g, "/add +1 --effort xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, false)
	})
	t.Run("join", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/join gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	t.Run("fold", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add GPT-6.1-Sol-XHIGH", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
}

// TestAdnTwoWords 是 N3：Griffin 这次的第一条命令的两种敲法。/add gpt-6.1-sol xhigh 与 ID 形式写出相同的字节；
// 他真机上敲的 xigh 是敲错的强度（裁决 (a)）：只打 +编号 那条路现成的「支持的强度是……，没有 xigh」，不写（typo）；
// 第二个词区分大小写，XHIGH 同样是敲错的强度（upper）；ID 形式已带强度再跟第二个词，一档强度相同也拒、敲错的也拒
// （id_then_effort、id_then_other、id_then_typo）；第一段对上多行时列候选（ambiguous_typo）；第二个词在快照里也是一个
// 模型时照多段说「一次加一个」（second_is_model）；第二个词是编号时是配置里的一段，不当强度（digit）；没有强度可选的
// grok 行后面跟 high 照多段（no_efforts：强度那一句只给有强度的行）；第二个词在配置里命中（是某个候选的前缀）时不当强度
// （effort_is_config_prefix）；第一个词在配置里按全名命中时不是两个词的写法（config_first）。
// 【判别力】model_effort 对 V18（第二个词不当强度）、V10（不认两个词）；typo、upper、id_then_typo、ambiguous_typo 对 V58（敲错的强度那一支去掉）；upper 另对 V48；second_is_model 对 V59；no_efforts 对 V60、V39；digit、effort_is_config_prefix、no_efforts、second_is_model 对 V27（不说「一次加一个」）；digit、effort_is_config_prefix 对 V12；id_then_* 对 V14；config_first 对 V13、V63、V39；ambiguous_typo 对 V9。
func TestAdnTwoWords(t *testing.T) {
	t.Run("model_effort", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	for _, c := range []struct {
		name, cmd string
		want      []string
	}{
		{"upper", "/add gpt-6.1-sol XHIGH", []string{adnDiscovering, "gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 XHIGH"}},
		{"digit", "/add gpt-6.1-sol 2", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol")}},
		{"typo", "/add gpt-6.1-sol xigh", []string{adnDiscovering, "gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 xigh"}},
		{"id_then_effort", "/add gpt-6.1-sol-xhigh xhigh", []string{adnDiscovering, "gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 xhigh"}},
		{"id_then_other", "/add gpt-6.1-sol-xhigh low", []string{adnDiscovering, "gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 low"}},
		{"id_then_typo", "/add gpt-6.1-sol-xhigh xigh", []string{adnDiscovering, "gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 xigh"}},
		{"ambiguous_typo", "/add adn-x-high xigh", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x  +2", "  codex   adn-x-high  +3"}},
		{"second_is_model", "/add gpt-6.1-sol grok-4.6", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol"), adnOneAtATime("grok-4.6")}},
		{"no_efforts", "/add grok-4.6 high", []string{adnDiscovering, adnOneAtATime("grok-4.6"), adnNotFound("high")}},
		{"config_first", "/add gpt-5.5-high xhigh", []string{adnDiscovering, adnNotFound("xhigh")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			adnRefused(t, g, c.cmd, c.want)
		})
	}
	t.Run("effort_is_config_prefix", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.high-adn]\nprovider = 'grok'\nmodel = 'grok-4.5'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add gpt-6.1-sol high", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol")})
	})
}

// TestAdnModelForm 是 N4：模型名形式与 --effort、--id；Griffin 这次的第三条命令（id_high，识别表 ID 列原样）；ID 形式与 --effort 的对账；强度区分大小写；
// grok 不能带 --effort；对上多行时带 --effort 也列候选（ambiguous_with_effort）；codex 里没有强度、也没有歧义的一行
// 带 --effort 打「没有可选的强度」（no_efforts_codex）。
// 【判别力】effort_flag、flag_first、unsupported、effort_case、grok_effort、ambiguous_with_effort、no_efforts_codex、id_flag、id_same_effort、id_other_effort 对 V21（不认参数）、V26（带参数时不识别）；id_flag 另对 V17；id_same_effort 对 V15；id_other_effort 对 V16；default_effort、grok、effort_flag、flag_first、unsupported、effort_case、grok_effort、ambiguous_with_effort、no_efforts_codex 对 V35（ID 对上之后不 continue）；effort_case 对 V48（强度不分大小写）；ambiguous_with_effort 对 V9；id_high 是 Griffin 的原样命令，只有通用的几条杀它（high 是默认档，V17、V37 下结果相同）。codex 的模型名现在按 SuggestedID("") 对上，模型名形式的 Model 那一支由 N15 的 claude_model 担保（V8、V2）。
func TestAdnModelForm(t *testing.T) {
	for _, c := range []struct {
		name, cmd, id, block string
	}{
		{"default_effort", "/add gpt-6.1-sol", "gpt-6.1-sol-high", adnBlockSolHigh},
		{"id_high", "/add gpt-6.1-sol-high", "gpt-6.1-sol-high", adnBlockSolHigh},
		{"effort_flag", "/add GPT-6.1-SOL --effort low", "gpt-6.1-sol-low", adnBlockSolLow},
		{"flag_first", "/add --effort low gpt-6.1-sol", "gpt-6.1-sol-low", adnBlockSolLow},
		{"id_flag", "/add gpt-6.1-sol-xhigh --id sol-x", "sol-x", adnBlockSolX},
		{"id_same_effort", "/add gpt-6.1-sol-xhigh --effort xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh},
		{"grok", "/add grok-4.6", "grok-4.6", adnBlockGrok46},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			adnAdded(t, g, c.cmd, c.id, c.block, true)
		})
	}
	for _, c := range []struct {
		name, cmd string
		want      []string
	}{
		{"id_other_effort", "/add gpt-6.1-sol-xhigh --effort high", []string{adnDiscovering, "gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 high"}},
		{"unsupported", "/add gpt-6.1-sol --effort ultra", []string{adnDiscovering, "gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 ultra"}},
		{"effort_case", "/add gpt-6.1-sol --effort HIGH", []string{adnDiscovering, "gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 HIGH"}},
		{"grok_effort", "/add grok-4.6 --effort high", []string{adnDiscovering, "grok 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort"}},
		{"ambiguous_with_effort", "/add adn-x-high --effort high", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x  +2", "  codex   adn-x-high  +3"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			adnRefused(t, g, c.cmd, c.want)
		})
	}
	t.Run("no_efforts_codex", func(t *testing.T) {
		// CXH 的整份夹具：cxh-bell 是 codex 里没有强度、也没有歧义的一行（只按 SuggestedID("") 对上一行）。
		g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add cxh-bell --effort high", []string{adnDiscovering, "cxh-bell 没有可选的强度，不能带 --effort"})
	})
}

// TestAdnConfiguredRow 是 N5（16a open item (c)）：gpt-5.5 拍快照时已在配置里（gpt-5.5-high），没有编号，
// 照样能按 ID 形式加另一档（xhigh）；/add gpt-5.5 是配置里 gpt-5.5-high 的前缀，照配置加入、不识别（prefix）；
// 大写的 GPT-5.5-HIGH 配置里对不上（配置按字节比），到快照里是同一个模型同一档，照 +编号 的同脑子那一句（same_brain）。
// refusal_*（W1 的 major）：开关打开时配置里的拒绝型命中照旧只打配置的那一句、不识别。
// 【判别力】other_effort、same_brain 对 V5（只找有编号的行）；same_brain 另对 V3；prefix 对 V29（每段都命中也插手）；refusal_* 对 V49（anyMiss 只看 id）、V51（共用判据只看 id）、V29、V41，正对照那一次加入另对 V17、V37。
func TestAdnConfiguredRow(t *testing.T) {
	t.Run("other_effort", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-5.5-xhigh", "gpt-5.5-xhigh", adnBlock55Xhigh, true)
	})
	t.Run("prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		mdxWantLines(t, "/add gpt-5.5", g.run("/add gpt-5.5"), []string{"gpt-5.5-high 已加入会话 " + g.conv.ID})
		if n := g.exec.count(); n != 0 {
			t.Errorf("配置里命中时不该识别，执行器被调了 %d 次", n)
		}
	})
	t.Run("same_brain", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add GPT-5.5-HIGH", []string{adnDiscovering, "配置里已经有同一个模型、同一强度的 agent gpt-5.5-high，直接 /add gpt-5.5-high"})
	})
	// 开关打开时，配置里的拒绝型命中（超出范围的编号、前缀歧义、总结成员、只是成员不在配置里）照旧只打配置的那一句、
	// 不识别。每条一个新装置：执行器 0 次与「没有正在识别模型那一行」只在还没拍快照时有判别力。同一装置接着按名字加
	// 一个配置里没有的模型，是正对照：它识别了、写了。
	refusals := []struct {
		name, cmd, want string
		rig             func(t *testing.T) *mdxRig
	}{
		{"refusal_out_of_range", "/add 99", "编号 99 超出范围（1-2）", func(t *testing.T) *mdxRig { return adnNewRig(t, true) }},
		{"refusal_ambiguous", "/add g", "g 有歧义，可能是：gpt-5.5-high, grok-4.7", func(t *testing.T) *mdxRig { return adnNewRig(t, true) }},
		{"refusal_summary", "/add grok-4.7", "grok-4.7 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结", func(t *testing.T) *mdxRig {
			// cxhNewRig 的配置末尾加 [summary]：cxh 与 mdx 的配置里都没有总结成员。
			g := cxhNewRig(t, cxhConfigTemplate+"\n[summary]\nagent = 'grok-4.7'\n", adnCodexOut)
			g.r.SetAddByName(true)
			return g
		}},
		{"refusal_member_only", "/add gpt-6.1-sol", "配置里没有 agent gpt-6.1-sol，用 /agents 看看有哪些", func(t *testing.T) *mdxRig {
			// 成员 id 恰好等于识别表里的模型名：把拒绝当成没命中的实现会到快照里对上它、写进配置。
			g := adnNewRig(t, true)
			adnSeedMember(t, g, "gpt-6.1-sol", "codex")
			return g
		}},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			g := c.rig(t)
			adnRefused(t, g, c.cmd, []string{c.want})
			adnNoDiscovery(t, g, c.cmd)
			adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		})
	}
}

// TestAdnAmbiguous 是 N6：一个名字对上快照里的多行时整条拒绝、逐行列出候选，不猜。同一家（adn-x 的 high 档与
// adn-x-high 的 ID 都是 adn-x-high）、跨两家（codex 的模型名 gpt-6.1-sol 与 grok 的 gpt-6.1-sol）；两个词的写法里
// 第一段有歧义时照样列候选（two_words）；第二个词的强度取几行的并集（union_second_row）；第二个词是敲错的强度、有强度的那一行
// 排在第二时同样列候选（typo_second_row，v1.2）。
// 【判别力】same_provider、two_words、cross_provider、no_number、union_second_row、typo_second_row 对 V9（取第一行）、V35；two_words、union_second_row、typo_second_row 另对 V10；union_second_row 对 V57（只看第一行的强度）、V62（不认配置前缀命中的第一段）；typo_second_row 对 V67（只看第一行有没有强度可选）、V58；no_number 对 V36（没有编号也带 +0）、V5；cross_provider 另对 V31。
func TestAdnAmbiguous(t *testing.T) {
	t.Run("same_provider", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add adn-x-high", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x  +2", "  codex   adn-x-high  +3"})
	})
	t.Run("two_words", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add adn-x-high high", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x  +2", "  codex   adn-x-high  +3"})
	})
	t.Run("cross_provider", func(t *testing.T) {
		g := adnNewRig(t, true)
		g.exec.set(mdxGrokPath, mdxResp{out: []byte(adnGrokExtra)})
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnDiscovering, "gpt-6.1-sol 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   gpt-6.1-sol  +1", "  grok    gpt-6.1-sol  +9"})
		// 带强度的 ID 只对上 codex 那一行，照常加入（正对照：同一装置、同一份快照）。
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, false)
	})
	t.Run("no_number", func(t *testing.T) {
		// codex 的 gpt-5.5 拍快照时已在配置里，没有编号，那一行不带 +N。GPT-5.5 大写：配置按字节比对不上。
		g := adnNewRig(t, true)
		g.exec.set(mdxGrokPath, mdxResp{out: []byte(adnGrokExtra)})
		adnRefused(t, g, "/add GPT-5.5", []string{adnDiscovering, "GPT-5.5 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   gpt-5.5", "  grok    gpt-5.5  +10"})
	})
	t.Run("union_second_row", func(t *testing.T) {
		// 两个词的写法、第一段在配置里只是前缀命中（adn-x-high-old），快照里对上两行，没有强度的那一行排在前面：
		// high 是第二行（adn-x）的一档，按一个名字处理、列候选。只看第一行的实现认不出 high，落到多段的说法。
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.adn-x-high-old]\nprovider = 'grok'\nmodel = 'grok-4.5'\n", adnCodexIDFirst)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add adn-x-high high", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x-high  +1", "  codex   adn-x  +2"})
	})
	t.Run("typo_second_row", func(t *testing.T) {
		// v1.2（W1b minor 4）：同一份夹具、第一段没命中配置，第二段是敲错的强度。有强度的那一行排在第二：只看第一行
		// 有没有强度的实现认不出这是敲错的强度，落到多段的说法。
		g := cxhNewRig(t, cxhConfigTemplate, adnCodexIDFirst)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add adn-x-high xigh", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x-high  +1", "  codex   adn-x  +2"})
	})
}

// TestAdnMultiSegment 是 N7：多段里只要有一段要按识别结果加，就整条拒绝、一个都不加（grok-4.7 在配置里、
// 本来能加也不加）；每段按出现顺序各打一行。一段对上多行也是「一次加一个」（ambiguous_seg）；每段都命中配置、其中一段
// 是拒绝句时不识别（all_config_refusal）。
// 【判别力】with_config、three、ambiguous_seg 对 V27；three 另对 V28（配置里的拒绝句不打）、V39、V50（多段那一处只看 id）、V51；ambiguous_seg 对 V52（对上多行不算）；all_config 对 V29；all_config_refusal 对 V49、V51、V29、V41。
func TestAdnMultiSegment(t *testing.T) {
	t.Run("with_config", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add grok-4.7,gpt-6.1-sol-xhigh", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol-xhigh")})
	})
	t.Run("three", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add gpt-6.1-sol-xhigh nosuch 99", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol-xhigh"), adnNotFound("nosuch"), "编号 99 超出范围（1-2）"})
	})
	t.Run("all_config", func(t *testing.T) {
		// 每一段都在配置里命中：照 AD1 之前加入，不识别。
		g := adnNewRig(t, true)
		mdxWantLines(t, "/add grok-4.7,gpt-5.5-high", g.run("/add grok-4.7,gpt-5.5-high"), []string{"grok-4.7 已加入会话 " + g.conv.ID, "gpt-5.5-high 已加入会话 " + g.conv.ID})
		if n := g.exec.count(); n != 0 {
			t.Errorf("每段都在配置里命中时不该识别，执行器被调了 %d 次", n)
		}
	})
	t.Run("all_config_refusal", func(t *testing.T) {
		// 每一段都在配置里命中、其中一段是拒绝句：照 AD1 之前全或无，只打那一句，不识别。新装置（理由同 N5 的 refusal_*），
		// 同一装置接着按名字加是正对照。
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add grok-4.7,99", []string{"编号 99 超出范围（1-2）"})
		adnNoDiscovery(t, g, "/add grok-4.7,99")
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	t.Run("ambiguous_seg", func(t *testing.T) {
		// 多段里一段在快照里对上多行（codex 与 grok 都有 gpt-6.1-sol），同样是「一次加一个」，不是「都没有」。
		g := adnNewRig(t, true)
		g.exec.set(mdxGrokPath, mdxResp{out: []byte(adnGrokExtra)})
		adnRefused(t, g, "/add gpt-6.1-sol grok-4.7", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol")})
	})
}

// TestAdnNotFound 是 N8：配置里与快照里都对不上时的新说法；已有快照不重拍（第二次不再识别）；custom/ 行加不了。
// codex_failed：codex 识别失败时，它的模型对不上、说法不变，失败行不在这里打；grok 的照常能加。
// no_providers：配置里一家能识别的都没有时不识别。
// 【判别力】snapshot_kept、codex_failed 对 V31（每次重拍）、V39（最后一句仍是旧说法）；codex_failed 另对 V35；no_providers 对 V54（照样调识别）、V39。
func TestAdnNotFound(t *testing.T) {
	t.Run("snapshot_kept", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add nosuch", []string{adnDiscovering, adnNotFound("nosuch")})
		adnRefused(t, g, "/add custom/adn-cpa", []string{adnNotFound("custom/adn-cpa")})
		if n := g.exec.count(); n != 2 {
			t.Errorf("已有快照不该重拍，执行器共被调了 %d 次（应当是 2）", n)
		}
	})
	t.Run("codex_failed", func(t *testing.T) {
		g := adnNewRig(t, true)
		g.exec.set(cxhCodexPath, mdxResp{err: errors.New("adn codex down")})
		adnRefused(t, g, "/add gpt-6.1-sol-xhigh", []string{adnDiscovering, adnNotFound("gpt-6.1-sol-xhigh")})
		adnAdded(t, g, "/add grok-4.6", "grok-4.6", adnBlockGrok46, false)
	})
	t.Run("no_providers", func(t *testing.T) {
		// 配置里一家能识别的 provider 都没有：不识别（没有「正在识别模型」那一行），快照留 nil，只打都没有那一句。
		// 有 provider 时同一条命令先识别，见 snapshot_kept。
		g := mdxNewRig(t, "data_dir = '<dataDir>'\n")
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add nosuch", []string{adnNotFound("nosuch")})
		if n := g.exec.count(); n != 0 {
			t.Errorf("没有能识别的 provider 时不该识别，执行器被调了 %d 次", n)
		}
		if g.r.modelSnap != nil {
			t.Errorf("没有能识别的 provider 时快照应当留 nil")
		}
	})
}

// TestAdnPriority 是 N9：识别快照里的精确命中排在「X 开头的都已经在当前会话里了」之前。先按名字加一个 grok-4.6-a
// （在场），再 /add grok-4.6：配置里对不上，快照里是同一个模型，这个模型已有在场的成员，打 v1.2 的拒绝句（D17；
// grok 没有强度可选，不带「另加一档」的后半句）；同一装置把开关关上是对照，打的是「开头的都已经在」。快照里也对不上的
// grok-4 照旧落到「开头的都已经在」。summary_prefix（v1.2，W1b minor 4）：开关打开时总结成员前缀那一句同样排在快照
// 之后、照旧打。
// 【判别力】V34（「开头的都已经在」排到快照之前）只有它杀得到；另对 V21、V26、V29、V30、V31、V35；v1.2 另对 V71（去掉
// 「有在场的」那一支）、V79（在场的那句一律带「另加一档」）；summary_prefix 对 V69（开关打开时跳过总结成员前缀那一句）。
func TestAdnPriority(t *testing.T) {
	g := adnNewRig(t, true)
	mdxWantLines(t, "/add grok-4.7", g.run("/add grok-4.7"), []string{"grok-4.7 已加入会话 " + g.conv.ID})
	adnAdded(t, g, "/add grok-4.6 --id grok-4.6-a", "grok-4.6-a", adnBlockGrok46A, true)
	adnRefused(t, g, "/add grok-4.6", []string{adnPresent("grok-4.6", "grok-4.6-a", false)})
	adnRefused(t, g, "/add grok-4", []string{"grok-4 开头的都已经在当前会话里了"})
	g.r.SetAddByName(false)
	adnRefused(t, g, "/add grok-4.6", []string{"grok-4.6 开头的都已经在当前会话里了"})

	t.Run("summary_prefix", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[summary]\nagent = 'grok-4.7'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add gro", []string{adnDiscovering, "gro 开头的除总结成员 grok-4.7 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结"})
	})
}

// TestAdnCanceled 是 N10：自动识别期间取消，只打两行，什么都不加、不存快照；之后再敲重新识别、照常加入。
// not_last 取消落在 codex（grok 不再识别），last 落在 grok；flag_not_last 是带 --effort 那条路，取消落在 codex。
// 【判别力】V42（取消之后照样存快照）；flag_not_last 对 V53（带参数那条路被取消不停），另对 V21、V26、V35；not_last、last 另对 V17、V37（取消之后那一次加入的字节）。
func TestAdnCanceled(t *testing.T) {
	for _, c := range []struct {
		name   string
		at     int
		before int
		cmd    string
	}{
		{"not_last", 1, 1, "/add gpt-6.1-sol-xhigh"},
		{"last", 2, 2, "/add gpt-6.1-sol-xhigh"},
		{"flag_not_last", 1, 1, "/add gpt-6.1-sol --effort xhigh"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			g.exec.hook = func(n int, _ provider.DiscoveryCommand) {
				if n == c.at {
					cancel()
				}
			}
			orig := g.readFile()
			mdxWantLines(t, "取消的那次", g.runCtx(ctx, c.cmd), []string{adnDiscovering, adnCanceled})
			if g.r.modelSnap != nil {
				t.Fatalf("取消之后不该存快照")
			}
			if n := g.exec.count(); n != c.before {
				t.Errorf("取消之后执行器应当共被调 %d 次，实际 %d", c.before, n)
			}
			if string(g.readFile()) != string(orig) || len(g.bakNames()) != 0 || len(g.activeIDs()) != 0 {
				t.Fatalf("取消之后不该写配置、不该有备份、不该有人加入")
			}
			g.exec.hook = nil
			adnAdded(t, g, c.cmd, "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		})
	}
}

// TestAdnSharesSnapshot 是 N11：按名字加与不带参数的 /add 是同一份快照。by_name_first：按名字加时自动识别
// （登录状态一起拍），之后不带参数的 /add 不再识别、照这份快照打表与登录段（登录状态执行器只跑过一次）；
// list_first：先不带参数的 /add，再按名字加，不再识别。
// 【判别力】by_name_first 对 V33（按名字加打登录段）；list_first 对 V31（每次重拍）。
func TestAdnSharesSnapshot(t *testing.T) {
	t.Run("by_name_first", func(t *testing.T) {
		g := adnNewRig(t, true)
		le := &adnLoginExec{}
		g.r.SetLoginStatusExec(le.run)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		mdxWantLines(t, "之后的 /add", g.afterCandidates(g.run("/add")), []string{
			"识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:",
			"NO  PROVIDER  ID                      MODEL                   EFFORTS  SOURCE",
			"+2  codex     adn-x-high              adn-x                   high     codex 目录",
			"+3  codex     adn-x-high              adn-x-high              -        codex 目录",
			"+4  grok      grok-4.6                grok-4.6                -        本地配置",
			"+5  grok      grok-4.5                grok-4.5                -        本地配置",
			"+6  grok      grok-4.3                grok-4.3                -        本地配置",
			"+7  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -        本地配置",
			"+8  grok      grok-build              grok-build              -        本地配置",
			"用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档",
			"codex 目录里还有 1 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员",
			"各家的登录状态（本会话的快照，/add --refresh 重新检查）:",
			"  codex   已登录",
			"  grok    已登录（API key）",
		})
		if n := g.exec.count(); n != 2 {
			t.Errorf("之后的 /add 不该重新识别，执行器共被调了 %d 次", n)
		}
		if le.n != 1 {
			t.Errorf("登录状态执行器应当只在按名字加那一次跑过 1 次，实际 %d 次", le.n)
		}
	})
	t.Run("list_first", func(t *testing.T) {
		g := adnNewRig(t, true)
		g.run("/add")
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, false)
		if n := g.exec.count(); n != 2 {
			t.Errorf("已有快照不该重拍，执行器共被调了 %d 次", n)
		}
	})
}

// TestAdnGuard 是 N12：文件在上次加载之后被改过时，守卫那一句里「再 /add」后面是敲的名字，不带 --effort 与第二个词。
// 改的是 grok-4.7 的 display_name（不是注释），整份比对不上。
// 【判别力】two_words 对 V20（再 /add 后面带上第二个词）；flag 对 V21、V26。
func TestAdnGuard(t *testing.T) {
	for _, c := range []struct {
		name, cmd, retry string
	}{
		{"id", "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh"},
		{"two_words", "/add gpt-6.1-sol xhigh", "gpt-6.1-sol"},
		{"flag", "/add gpt-6.1-sol --effort xhigh", "gpt-6.1-sol"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			g.writeFile([]byte(adnReplaceOnce(t, g.readFile(), "display_name = 'Grok 4.7'", "display_name = 'Grok 4.7 adn'")))
			adnRefused(t, g, c.cmd, []string{adnDiscovering, "配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add " + c.retry})
		})
	}
}

// adnReplaceOnce 把 b 里恰好一处 old 换成 repl（经 mdxReplaceOnce，不是恰好一处就 Fatal）。
func adnReplaceOnce(t *testing.T, b []byte, old, repl string) string {
	t.Helper()
	return mdxReplaceOnce(t, string(b), old, repl)
}

// TestAdnNoRunner 是 N13：Runner 不在时，按名字对上之后打 Runner 那一句，不写配置。
// 【判别力】V19（不判 Runner：写完配置之后在 SetConfig 处 panic，装置接住判红）。
func TestAdnNoRunner(t *testing.T) {
	g := mdxNewRigOpts(t, cxhConfigTemplate, mdxRigOpts{noRunner: true})
	g.exec.set(cxhCodexPath, mdxResp{out: []byte(adnCodexOut)})
	g.r.SetAddByName(true)
	adnRefused(t, g, "/add gpt-6.1-sol-xhigh", []string{adnDiscovering, "provider 调用未就绪，本命令不可用"})
}

// TestAdnFlags 是 N14：带 --id / --effort 时的参数形状；名字在配置里按全名命中、或命中一句拒绝时不识别。
// /add --id x +1：+编号 只在第一个词时才认，这里打 +编号 的用法，不说「都没有 +1」。v1.2：/add grok --id x 的 grok 在配置里
// 只是前缀命中，先识别（D9），挪到 N23 prefix_no_match；plus_letters（W1b minor 4）：/add --id x +abc 不是 + 加数字，照常
// 识别、落到「都没有 +abc」（§7 (u)），新装置。v1.3（W1c 的 W7）：/add 1 --effort high 按编号命中 gpt-5.5-high，
// 编号不算前缀命中，不识别、直接打「对上的是配置里的 agent」。
// 【判别力】V22（多个名字取第一个）、V23（名字不按逗号切）、V24（命中 agent 不说明）、V25（命中的拒绝句不打）、V65（+ 加数字不打 +编号 的用法）、V21；/add 1 --effort high 对 V92（configPrefixOnly 把编号命中也算前缀命中）；plus_letters 对 V68（+ 开头一律打 +编号 的用法）。
func TestAdnFlags(t *testing.T) {
	g := adnNewRig(t, true)
	for _, c := range []struct {
		cmd  string
		want string
	}{
		{"/add grok-4.7 --effort high", "grok-4.7 对上的是配置里的 agent grok-4.7，--id 与 --effort 只用于加识别出的模型；直接 /add grok-4.7"},
		{"/add 99 --id z", "编号 99 超出范围（1-2）"},
		{"/add --effort high", adnUsage},
		{"/add a b --effort high", adnUsage},
		{"/add a,b --effort high", adnUsage},
		{"/add gpt-6.1-sol --effort", adnUsage},
		{"/add gpt-6.1-sol --id a --id b", adnUsage},
		{"/add gpt-6.1-sol --effort low --effort high", adnUsage},
		{"/add gpt-6.1-sol xhigh --id x", adnUsage},
		{"/add --id x +1", "用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别"},
		{"/add 1 --effort high", adnFlagsConfig("1", "gpt-5.5-high")},
	} {
		adnRefused(t, g, c.cmd, []string{c.want})
	}
	if n := g.exec.count(); n != 0 {
		t.Errorf("这几条都不该识别，执行器被调了 %d 次", n)
	}

	t.Run("plus_letters", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add --id x +abc", []string{adnDiscovering, adnNotFound("+abc")})
	})
}

// TestAdnOtherProviders 是 N15：MD1 的配置（agy、claude、grok）。claude 的别名（sonnet[1m]）是模型名形式、默认 high，没有 high 的（mdx-lowmed）取第一档；
// claude 的 Model（claude-sonnet-5）同样是模型名形式；claude 的 ID 形式带强度；agy 的 id（Alias）；claude 没有强度的行不能带
// --effort；别名 opus[1m] 对上已配成 opus-5.0（model 写的就是这个别名）的那一行：带 --effort high 是同脑子（alias_same_brain），
// 不带强度时加入 opus-5.0、不写配置（alias_configured，v1.2 的 D17）；不带强度的 claude ID 段（sonnet-5）
// 按 DefaultEffort 加，跟第二个词时按那一档加（裁决 (d)）；别名与没有强度的 ID 不分大小写（claude_alias_fold、haiku_fold）。
// 【判别力】claude_alias、claude_alias_fold、claude_first_effort、alias_same_brain、alias_configured 对 V6（不认 Alias）；claude_alias_fold 对 V55（别名按字节比）、V2；claude_model、claude_model_fold 对 V8（不认 Model）；claude_model_fold 另对 V2；alias_same_brain、alias_configured 另对 V5；alias_configured 另对 V73（同一个模型不认别名写法）、V75（恰好一个不在场时不加入）；claude_base、claude_base_second 对 V64（有强度的行不认不带强度的 ID）、V4；claude_base_second 另对 V10、V18；haiku_effort、haiku_fold 对 V4；haiku_fold 对 V56（不带强度的 ID 按字节比）；agy_alias 对 V35；claude_id 对 V17、V37。
func TestAdnOtherProviders(t *testing.T) {
	const disc = "正在识别模型: agy, claude, grok"
	for _, c := range []struct {
		name, cmd, id string
	}{
		{"claude_alias", "/add sonnet[1m]", "sonnet-5-1m-high"},
		{"claude_alias_fold", "/add SONNET[1m]", "sonnet-5-1m-high"},
		{"claude_model", "/add claude-sonnet-5", "sonnet-5-high"},
		{"claude_model_fold", "/add Claude-Sonnet-5", "sonnet-5-high"},
		{"claude_first_effort", "/add mdx-lowmed", "mdx-lowmed-2.0-low"},
		{"claude_id", "/add sonnet-5-xhigh", "sonnet-5-xhigh"},
		{"claude_base", "/add sonnet-5", "sonnet-5-high"},
		{"claude_base_second", "/add sonnet-5 xhigh", "sonnet-5-xhigh"},
		{"haiku_fold", "/add HAIKU-4.5", "haiku-4.5"},
		{"agy_alias", "/add gemini-3.7-flash-high", "gemini-3.7-flash-high"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := mdxNewRig(t, mdxConfigTemplate)
			g.r.SetAddByName(true)
			orig := g.readFile()
			baks := g.bakNames()
			lines := g.run(c.cmd)
			bak := g.newBak(baks)
			mdxWantLines(t, c.cmd, lines, append([]string{disc}, g.joinedLines(c.id, bak)...))
			if len(g.readFile()) <= len(orig) {
				t.Errorf("%s 之后配置文件应当变长", c.cmd)
			}
		})
	}
	for _, c := range []struct {
		name, cmd, want string
	}{
		{"haiku_effort", "/add haiku-4.5 --effort high", "claude-haiku-4-5-20251001 没有可选的强度，不能带 --effort"},
		{"alias_same_brain", "/add opus[1m] --effort high", "配置里已经有同一个模型、同一强度的 agent opus-5.0，直接 /add opus-5.0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := mdxNewRig(t, mdxConfigTemplate)
			g.r.SetAddByName(true)
			adnRefused(t, g, c.cmd, []string{disc, c.want})
		})
	}
	t.Run("alias_configured", func(t *testing.T) {
		// v1.2（D17 第 4 步）：没给强度、对上的模型在配置里恰好有一个不在场的成员（opus-5.0，model 写的是别名），
		// 加入它、不写配置。v1.1 这里打同脑子那一句；同脑子那一句现在只在给了强度时打（alias_same_brain）。
		g := mdxNewRig(t, mdxConfigTemplate)
		g.r.SetAddByName(true)
		adnJoinedNoWrite(t, g, "/add opus[1m]", "opus-5.0", []string{disc})
	})
}

// TestAdnTwoWordsConfigPrefix 是 N16（裁决 (b)）：模型加进配置、又不在场之后，第一段在配置里只是前缀命中，
// 两个词的写法照样成立。先按名字加 gpt-6.1-sol-xhigh 再 /leave；/add gpt-6.1-sol xhigh 是同一模型同一强度，打同脑子
// 那一句、不写；/add gpt-6.1-sol xigh 不是一档强度，第一段又在配置里命中，照多段只说 xigh 都没有（敲错的强度那一支只给
// 配置里没命中的第一段）；/add gpt-6.1-sol high 新建 gpt-6.1-sol-high。单段 /add gpt-6.1-sol 仍按配置前缀加入已有的
// gpt-6.1-sol-xhigh（对照：单段不变）。之后都不再识别。XHIGH 不是一档强度（区分大小写），同 xigh。
// 【判别力】V61（敲错的强度那一支也给前缀命中）、V11（第二个词当强度时不分大小写）只有它杀得到；V62（不认配置前缀命中的
// 第一段）另有 union_second_row；另对 V10、V17、V18、V29、V30、V31、V35、V37、V39。
func TestAdnTwoWordsConfigPrefix(t *testing.T) {
	g := adnNewRig(t, true)
	adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	mdxWantLines(t, "/leave", g.run("/leave gpt-6.1-sol-xhigh"), []string{"gpt-6.1-sol-xhigh 已退出会话 " + g.conv.ID})
	adnRefused(t, g, "/add gpt-6.1-sol xhigh", []string{"配置里已经有同一个模型、同一强度的 agent gpt-6.1-sol-xhigh，直接 /add gpt-6.1-sol-xhigh"})
	adnRefused(t, g, "/add gpt-6.1-sol xigh", []string{adnNotFound("xigh")})
	adnRefused(t, g, "/add gpt-6.1-sol XHIGH", []string{adnNotFound("XHIGH")})
	adnAdded(t, g, "/add gpt-6.1-sol high", "gpt-6.1-sol-high", adnBlockSolHigh, false)
	mdxWantLines(t, "/add gpt-6.1-sol", g.run("/add gpt-6.1-sol"), []string{"gpt-6.1-sol-xhigh 已加入会话 " + g.conv.ID})
	if n := g.exec.count(); n != 2 {
		t.Errorf("只在第一次识别，执行器共被调了 %d 次（应当是 2）", n)
	}
}

// TestAdnIdeographicSpace 是 N17：全角空格 U+3000 与半角空格一样切段，不看开关。开着：两个词的写法用全角空格照样加入
// （on_two_words）；fields 本来就在全角空格上切，带 --effort 的写法同样加入（on_flag）。关着：两个配置里的全名用全角空格
// 隔开，两个都加入、不识别（off_config）。
// 【判别力】on_two_words、off_config 对 V66（splitAddArgs 不按全角空格切）；on_flag 不经 splitAddArgs 切段（fields 已经切开），
// V66 下照样绿，它实测的是「fields 在全角空格上切」这一条前提，另对 V21、V26。
func TestAdnIdeographicSpace(t *testing.T) {
	t.Run("on_two_words", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol"+adnIdeoSpace+"xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	t.Run("on_flag", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol"+adnIdeoSpace+"--effort"+adnIdeoSpace+"xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	t.Run("off_config", func(t *testing.T) {
		g := adnNewRig(t, false)
		cmd := "/add grok-4.7" + adnIdeoSpace + "gpt-5.5-high"
		mdxWantLines(t, cmd, g.run(cmd), []string{"grok-4.7 已加入会话 " + g.conv.ID, "gpt-5.5-high 已加入会话 " + g.conv.ID})
		adnNoDiscovery(t, g, cmd)
	})
}

// adnTwoSolAgents 是 cxhConfigTemplate 末尾加两个 gpt-6.1-sol 的成员（id 不以模型名开头，所以 /add gpt-6.1-sol 在配置里
// 按前缀对不上）。
const adnTwoSolAgents = "\n[agents.sol-a]\nprovider = 'codex'\nmodel = 'gpt-6.1-sol'\neffort = 'high'\n\n[agents.sol-b]\nprovider = 'codex'\nmodel = 'gpt-6.1-sol'\neffort = 'xhigh'\n"

// TestAdnExistingMembers 是 N22（v1.2，D17）：没给强度也没给 --id 的单个名字（模型名形式或不带强度的 ID 形式）在配置里
// 没命中、在快照里对上一行时，先看配置里这个模型已有的成员（provider 与 model 相同，不看 id 前缀；不含总结成员）：有在场的
// 就拒绝、点名；全不在场、恰好一个就加入它；不止一个就列出来。三种都不写配置。
// repeat：同一条 /add gpt-6.1-sol 敲两次，第一次（成员不在场，配置前缀命中）加入 gpt-6.1-sol-xhigh，第二次打拒绝句，不写、
// 不多出成员（W1b 的 major）。prefix_other_model：gpt-6.1 与 gpt-6.1-sol 是两个模型，在场的 gpt-6.1-sol-xhigh 不挡
// /add gpt-6.1。alias_present、alias_config_present：claude 的别名形式，同一个模型的成员在场时拒绝（前者是按名字加进来的，
// 配置里 model 写的是 Model；后者是 model 写别名的 opus-5.0）。several_absent：两个不在场的同模型成员，列出来；同一装置
// 带强度照常新建（正对照）。one_absent_config、one_absent_renamed：恰好一个不在场、id 又不以名字开头（配置里起的名字、
// --id 起的名字），加入它；同一装置带强度照常新建，或再敲一次打拒绝句。several_absent_grok：没有强度可选的行，列出来时
// 不提强度。summary_excluded：总结成员不算（大写绕过配置第 0 步的字节比），落到新建、再被同脑子那一句拦下，不加入总结
// 成员；同一装置接着加别的是正对照。
// v1.3（W1c 的 W1-W5）：present_among_absent、present_among_absent_grok：S 里一个在场、一个不在场，拒绝句只点名在场的
// （有强度可选的行与 grok 各一条）；other_provider_same_model：grok 的 gx 的 model 字符串恰好是 gpt-6.1-sol，不进 codex 那一行的
// S，照常新建 gpt-6.1-sol-high、写配置；one_absent_config 另断言会话成员记录里的显示名与 provider 取配置里的值。
// 【判别力】（v1.3）present_among_absent 对 V87（在场那一句列出整个 S）、present_among_absent_grok 对 V88（同上，没有强度的行）；
// one_absent_config 对 V89、V90（成员记录的显示名、provider 写成空串）；other_provider_same_model 对 V91（S 不比 provider）；
// present_among_absent、present_among_absent_grok 另对 V5、V29、V71、V72，present_among_absent_grok 另对 V79；新的三条都对
// V1、V30、V32、V35、V38 红。
// 【判别力】repeat、one_absent_renamed、alias_present、alias_config_present 对 V71（去掉「有在场的」那一支）；prefix_other_model、
// alias_present、alias_config_present、one_absent_config、one_absent_renamed、several_absent、several_absent_grok 对 V72（S 按 id
// 前缀判；repeat 杀不到它：在场的 gpt-6.1-sol-xhigh 恰好以名字开头）；alias_config_present 对 V73（S 不认别名写法）；
// several_absent、several_absent_grok 对 V74（不止一个时加入第一个）；one_absent_config、one_absent_renamed 对 V75（恰好一个时
// 不加入）；summary_excluded 对 V78（总结成员算进 S）；several_absent 对 V80（列出时一律不提强度）；正对照那几次新建另对
// V81（one_absent_config）、V82（several_absent）；另对 V30、V32。
func TestAdnExistingMembers(t *testing.T) {
	const mdxDisc = "正在识别模型: agy, claude, grok"
	t.Run("repeat", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		mdxWantLines(t, "/leave", g.run("/leave gpt-6.1-sol-xhigh"), []string{"gpt-6.1-sol-xhigh 已退出会话 " + g.conv.ID})
		adnJoinedNoWrite(t, g, "/add gpt-6.1-sol", "gpt-6.1-sol-xhigh", nil)
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnPresent("gpt-6.1-sol", "gpt-6.1-sol-xhigh", true)})
		if got := g.activeIDs(); !reflect.DeepEqual(got, []string{"gpt-6.1-sol-xhigh"}) {
			t.Errorf("在场成员应当只有 gpt-6.1-sol-xhigh，实际 %q", got)
		}
		if n := g.exec.count(); n != 2 {
			t.Errorf("只在第一次识别，执行器共被调了 %d 次（应当是 2）", n)
		}
	})
	t.Run("prefix_other_model", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate, adnCodexSol61)
		g.r.SetAddByName(true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		adnAdded(t, g, "/add gpt-6.1", "gpt-6.1-high", adnBlock61High, false)
	})
	t.Run("alias_present", func(t *testing.T) {
		g := mdxNewRig(t, mdxConfigTemplate)
		g.r.SetAddByName(true)
		baks := g.bakNames()
		lines := g.run("/add sonnet[1m] xhigh")
		mdxWantLines(t, "/add sonnet[1m] xhigh", lines, append([]string{mdxDisc}, g.joinedLines("sonnet-5-1m-xhigh", g.newBak(baks))...))
		adnRefused(t, g, "/add sonnet[1m]", []string{adnPresent("sonnet[1m]", "sonnet-5-1m-xhigh", true)})
	})
	t.Run("alias_config_present", func(t *testing.T) {
		g := mdxNewRig(t, mdxConfigTemplate)
		g.r.SetAddByName(true)
		mdxWantLines(t, "/add opus-5.0", g.run("/add opus-5.0"), []string{"opus-5.0 已加入会话 " + g.conv.ID})
		adnRefused(t, g, "/add opus[1m]", []string{mdxDisc, adnPresent("opus[1m]", "opus-5.0", true)})
	})
	t.Run("several_absent", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+adnTwoSolAgents, adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnDiscovering, adnConfigured("gpt-6.1-sol", "sol-a, sol-b", true)})
		adnAdded(t, g, "/add gpt-6.1-sol low", "gpt-6.1-sol-low", adnBlockSolLow, false)
	})
	t.Run("one_absent_config", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.sol-a]\ndisplay_name = 'Sol A'\nprovider = 'codex'\nmodel = 'gpt-6.1-sol'\neffort = 'xhigh'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnJoinedNoWrite(t, g, "/add gpt-6.1-sol", "sol-a", []string{adnDiscovering})
		// v1.3（W1c 的 W3、W4）：会话成员记录里的显示名与 provider 取配置里的值。夹具的 display_name 非空，
		// 换成空串才看得出来。
		found := false
		for _, p := range g.participants() {
			if p.AgentID != "sol-a" {
				continue
			}
			found = true
			if p.DisplayName != "Sol A" || p.Provider != "codex" {
				t.Errorf("sol-a 的成员记录应当是显示名 Sol A、provider codex，实际 %q、%q", p.DisplayName, p.Provider)
			}
		}
		if !found {
			t.Errorf("会话成员记录里没有 sol-a")
		}
		adnAdded(t, g, "/add gpt-6.1-sol --effort low", "gpt-6.1-sol-low", adnBlockSolLow, false)
	})
	t.Run("one_absent_renamed", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh --id sol-x", "sol-x", adnBlockSolX, true)
		mdxWantLines(t, "/leave", g.run("/leave sol-x"), []string{"sol-x 已退出会话 " + g.conv.ID})
		adnJoinedNoWrite(t, g, "/add gpt-6.1-sol", "sol-x", nil)
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnPresent("gpt-6.1-sol", "sol-x", true)})
	})
	t.Run("several_absent_grok", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.g46-a]\nprovider = 'grok'\nmodel = 'grok-4.6'\n\n[agents.g46-b]\nprovider = 'grok'\nmodel = 'grok-4.6'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add grok-4.6", []string{adnDiscovering, adnConfigured("grok-4.6", "g46-a, g46-b", false)})
	})
	t.Run("summary_excluded", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[summary]\nagent = 'grok-4.7'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add GROK-4.7", []string{adnDiscovering, "配置里已经有同一个模型、同一强度的 agent grok-4.7，直接 /add grok-4.7"})
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, false)
	})
	t.Run("present_among_absent", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+adnTwoSolAgents, adnCodexOut)
		g.r.SetAddByName(true)
		mdxWantLines(t, "/add sol-a", g.run("/add sol-a"), []string{"sol-a 已加入会话 " + g.conv.ID})
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnDiscovering, adnPresent("gpt-6.1-sol", "sol-a", true)})
	})
	t.Run("present_among_absent_grok", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.g46-a]\nprovider = 'grok'\nmodel = 'grok-4.6'\n\n[agents.g46-b]\nprovider = 'grok'\nmodel = 'grok-4.6'\n", adnCodexOut)
		g.r.SetAddByName(true)
		mdxWantLines(t, "/add g46-a", g.run("/add g46-a"), []string{"g46-a 已加入会话 " + g.conv.ID})
		adnRefused(t, g, "/add grok-4.6", []string{adnDiscovering, adnPresent("grok-4.6", "g46-a", false)})
	})
	t.Run("other_provider_same_model", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.gx]\nprovider = 'grok'\nmodel = 'gpt-6.1-sol'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnAdded(t, g, "/add gpt-6.1-sol", "gpt-6.1-sol-high", adnBlockSolHigh, true)
		if mdxHasLine(g.activeIDs(), "gx") {
			t.Errorf("grok 的 gx 不该被当成 codex 的 gpt-6.1-sol 加入，在场的是 %q", g.activeIDs())
		}
	})
}

// TestAdnFlagsOverConfigPrefix 是 N23（v1.2，裁决书规则一、规则三，原 §7 (r)）：带 --effort 或 --id 时，名字在配置里只是
// 前缀命中、在识别快照里又对得上，按识别结果加（D9）。effort_over_prefix：配置里有不在场的 gpt-6.1-sol-xhigh（同一个模型），
// /add gpt-6.1-sol --effort high 新建 gpt-6.1-sol-high；effort_over_other_model：配置里的 gpt-6.1-sol-mini 是另一个模型，
// 同一条命令同样新建；id_over_prefix：只有 --id 时同样按识别结果、按默认档新建。id_only_present：只有 --id、这个模型已有
// 在场成员时照样新建（--id 是明确要新建，不过 D17 那道门）。prefix_no_match：名字在快照里对不上时照旧打「对上的是配置里的
// agent」，只是先识别了一次。
// 【判别力】effort_over_prefix、effort_over_other_model、id_over_prefix、prefix_no_match 对 V76（前缀命中照旧打「对上的是配置里的
// agent」、不识别）；id_over_prefix、id_only_present 对 V77（只有 --id 也过 D17 那道门）；effort_over_prefix 对 V81（--effort 也过
// 那道门）；prefix_no_match 对 V84（前缀命中、快照里对不上时落到「都没有」），另对 V24、V26。
func TestAdnFlagsOverConfigPrefix(t *testing.T) {
	addThenLeave := func(t *testing.T, g *mdxRig) {
		t.Helper()
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		mdxWantLines(t, "/leave", g.run("/leave gpt-6.1-sol-xhigh"), []string{"gpt-6.1-sol-xhigh 已退出会话 " + g.conv.ID})
	}
	t.Run("effort_over_prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		addThenLeave(t, g)
		adnAdded(t, g, "/add gpt-6.1-sol --effort high", "gpt-6.1-sol-high", adnBlockSolHigh, false)
	})
	t.Run("effort_over_other_model", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.\"gpt-6.1-sol-mini\"]\nprovider = 'codex'\nmodel = 'gpt-6.1-sol-mini'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnAdded(t, g, "/add gpt-6.1-sol --effort high", "gpt-6.1-sol-high", adnBlockSolHigh, true)
	})
	t.Run("id_over_prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		addThenLeave(t, g)
		adnAdded(t, g, "/add gpt-6.1-sol --id sol", "sol", adnBlockSol, false)
	})
	t.Run("id_only_present", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		adnAdded(t, g, "/add gpt-6.1-sol --id sol", "sol", adnBlockSol, false)
		if got := g.activeIDs(); !reflect.DeepEqual(got, []string{"gpt-6.1-sol-xhigh", "sol"}) {
			t.Errorf("在场成员应当是 gpt-6.1-sol-xhigh 与 sol，实际 %q", got)
		}
	})
	t.Run("prefix_no_match", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add grok --id x", []string{adnDiscovering, adnFlagsConfig("grok", "grok-4.7")})
		adnRefused(t, g, "/add grok --effort high", []string{adnFlagsConfig("grok", "grok-4.7")})
		if n := g.exec.count(); n != 2 {
			t.Errorf("只在第一次识别，执行器共被调了 %d 次（应当是 2）", n)
		}
	})
}

// TestAdnTypoIsReference 是 N24（v1.2，W1b minor 1）：两个词的第二段是在场成员的前缀、总结成员的前缀或以 + 开头时，它指的
// 是配置里的人或 +编号，不当敲错的强度，照多段说（每段一行）。present_prefix、id_present_prefix：grok-4.7 在场，第二段
// grok；summary_prefix：总结成员 grok-4.7 没加入，第二段 gro；plus：第二段 +1。四条都不写配置。敲错的强度那一支仍然成立
// 的正对照是 N3 的 typo、id_then_typo、ambiguous_typo。
// 【判别力】present_prefix、id_present_prefix、summary_prefix 对 V85（去掉「addSegmentNotFound 打的是都没有」那一条）；plus 对
// V86（去掉「不以 + 开头」那一条）；summary_prefix 另对 V69；四条都对 V27。
func TestAdnTypoIsReference(t *testing.T) {
	joinGrok := func(t *testing.T, g *mdxRig) {
		t.Helper()
		mdxWantLines(t, "/add grok-4.7", g.run("/add grok-4.7"), []string{"grok-4.7 已加入会话 " + g.conv.ID})
	}
	t.Run("present_prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		joinGrok(t, g)
		adnRefused(t, g, "/add gpt-6.1-sol grok", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol"), "grok 开头的都已经在当前会话里了"})
	})
	t.Run("id_present_prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		joinGrok(t, g)
		adnRefused(t, g, "/add gpt-6.1-sol-xhigh grok", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol-xhigh"), "grok 开头的都已经在当前会话里了"})
	})
	t.Run("summary_prefix", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[summary]\nagent = 'grok-4.7'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add gpt-6.1-sol gro", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol"), "gro 开头的除总结成员 grok-4.7 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结"})
	})
	t.Run("plus", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add gpt-6.1-sol +1", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol"), adnNotFound("+1")})
	})
}

// TestAdnSingleIDConfigPrefix 是 N25（v1.3，§7 (w)，W1c 的 w2 装置）：单段、不带参数、带强度的 ID 在配置里只是前缀命中时，
// 照 HEAD 的前缀语义加入配置里的那个 agent、不识别、不写，即使它是另一个模型（gpt-6.1-sol-xhigh-old 的 model 是 gpt-5.5）；
// 屏幕回显的是实际加入的 id。有意保留，钉住现状，以后要改就是有意改。同一装置再敲一次是正对照：-old 在场之后不是候选，
// 配置里没命中，识别之后按 ID 形式新建 gpt-6.1-sol-xhigh、写配置（同一条命令敲两次结果不同，§7 (w)）。
// 【判别力】V29（每一段都在配置里命中时也插手，也就是单段前缀命中也先识别：第一条就识别、新建）；正对照那一次另对 V1、
// V17、V30、V32、V34、V37、V38 红。
func TestAdnSingleIDConfigPrefix(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.\"gpt-6.1-sol-xhigh-old\"]\nprovider = 'codex'\nmodel = 'gpt-5.5'\neffort = 'xhigh'\n", adnCodexOut)
	g.r.SetAddByName(true)
	adnJoinedNoWrite(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh-old", nil)
	adnNoDiscovery(t, g, "/add gpt-6.1-sol-xhigh")
	adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	if n := g.exec.count(); n != 2 {
		t.Errorf("正对照：第二次应当识别 codex、grok 两家，执行器共被调了 %d 次", n)
	}
}
```

### 2.2 `internal/app/adn_addname_app_test.go`（S2）

- **N20 `TestAdnAppLineMode`**：行式分支。显式配置只有 codex 一家（假 codex 打 obx 的 `obxCodexOut`，`gpt-obx` 只有 high 一档）；`/new t`、`/add gpt-obx-high`、`/quit`。out 的结尾（正则）：「正在识别模型: codex」、追加那一行、加入那一行、「council> 再见」；文件等于原文加冻结的一段；恰好一个备份、字节等于原文。
- **N21 `TestAdnAppTUI`**：TUI 分支。同 N20 的装置，经 `os.Pipe` 走真 `tui.Run`；屏幕上（去掉 ANSI 与空格）出现「gpt-obx-high已加入会话」，文件与备份同 N20。

```file internal/app/adn_addname_app_test.go
package app

// adn_addname_app_test.go 是 AD1（00_DECISIONS.md #49：/add 按名字加识别出的模型）在 app 包的验收，契约 §2.2：
// N20 行式分支、N21 TUI 分支都打开了开关（经真 app.Run，新会话里第一次就敲 /add gpt-obx-high，先识别、再写进
// 显式配置、加入）。开关关着时这一条打的是「配置里没有可加入的 gpt-obx-high……」，文件不变。
// 装置与夹具只读复用 obx_firstrun_app_test.go 的 §2.0（obxIsolate、obxRun、obxDrain、obxFixture、obxCodexFake、
// obxCodexOut、obxCodexLoggedIn、obxIn、obxEnv、obxReadFile）与 cox_app_test.go 的 coxLockedBuf、
// coxWaitFor、coxFlat、coxESC；纪律同契约 19d §2 开头。两条都用显式配置，不走首次运行。

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"council/internal/config"
)

// adnAppBlock 是 /add gpt-obx-high 写进配置的那一段（obxCodexOut 里 gpt-obx 只有 high 一档，display_name 缺席）。
const adnAppBlock = "\n# council /add 追加：codex 识别出的 gpt-obx（codex 目录）\n[agents.gpt-obx-high]\ndisplay_name = \"Gpt Obx\"\nprovider = \"codex\"\nmodel = \"gpt-obx\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"

// adnAppSetup 装假 codex（debug models 打 obxCodexOut，login status 打已登录），写一份只有 codex 一家的显式配置。
// 返回配置路径与它的原文。
func adnAppSetup(t *testing.T, e *obxEnv) (string, string) {
	t.Helper()
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, obxCodexLoggedIn)
	explicit := filepath.Join(e.root, "explicit.toml")
	body := "data_dir = '" + filepath.Join(e.root, "data") + "'\n\n" + config.ProviderBlockTOML("codex", filepath.Join(e.bin, "codex.cmd"))
	if err := os.WriteFile(explicit, []byte(body), 0o644); err != nil {
		t.Fatalf("前提不成立：写显式配置失败：%v", err)
	}
	return explicit, body
}

// adnAppBaks 是 dir 里 explicit.toml 的备份文件（config.AppendBlock 的命名）。
func adnAppBaks(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "explicit.toml.bak-add-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	return m
}

// adnAppFileAdded 断言显式配置等于原文加 adnAppBlock，恰好一个备份、字节等于原文。
func adnAppFileAdded(t *testing.T, e *obxEnv, explicit, orig string) {
	t.Helper()
	if got := obxReadFile(t, explicit); got != orig+adnAppBlock {
		t.Errorf("显式配置不等\n实际 %q\n期望 %q", got, orig+adnAppBlock)
	}
	baks := adnAppBaks(t, e.root)
	if len(baks) != 1 {
		t.Fatalf("应当恰好一个备份，实际 %q", baks)
	}
	if got := obxReadFile(t, baks[0]); got != orig {
		t.Errorf("备份 %s 应当是原文，实际 %q", baks[0], got)
	}
}

// TestAdnAppLineMode 是 N20：行式分支。新会话里第一次就敲 /add gpt-obx-high：先识别（正在识别模型: codex），
// 再追加、加入，out 的结尾整段相等（会话 id 与备份时间戳按形状比）；文件与备份按字节比。
// 【判别力】V45（行式分支不打开开关）。
func TestAdnAppLineMode(t *testing.T) {
	e := obxIsolate(t)
	explicit, orig := adnAppSetup(t, e)

	out, errOut, err := obxRun(t, context.Background(), Options{ConfigPath: explicit, In: obxIn("/new t\n/add gpt-obx-high\n/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s", err, errOut)
	}
	if errOut != "" {
		t.Errorf("显式路径不走首次运行，errOut 应为空，实际 %q", errOut)
	}
	q := regexp.QuoteMeta(explicit)
	tail := regexp.MustCompile(`council> 正在识别模型: codex\n已把 agent gpt-obx-high 追加进 ` + q + `（原文件备份在 ` + q + `\.bak-add-\d{8}-\d{6}）\ngpt-obx-high 已加入会话 [0-9a-f]{32}\ncouncil> 再见\n$`)
	if !tail.MatchString(out) {
		t.Errorf("out 结尾不对\n实际 %q\n期望形如 %q", out, tail.String())
	}
	adnAppFileAdded(t, e, explicit, orig)
}

// TestAdnAppTUI 是 N21：TUI 分支。同 N20 的装置，经 os.Pipe 走真 tui.Run，/new 之后 /add gpt-obx-high，
// 屏幕上（去掉 ANSI 与空格）出现「gpt-obx-high已加入会话」，文件与备份按字节比。
// 【判别力】V46（TUI 分支不打开开关）。
func TestAdnAppTUI(t *testing.T) {
	e := obxIsolate(t)
	explicit, orig := adnAppSetup(t, e)

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
	write("/add", "/add gpt-obx-high\r")
	if !flatHas(10*time.Second, "gpt-obx-high已加入会话") {
		t.Fatalf("/add gpt-obx-high 之后 10 秒内没有看到「gpt-obx-high已加入会话」；flat %q", coxFlat(buf.String()))
	}
	adnAppFileAdded(t, e, explicit, orig)
}
```

---

## 3. 冻结的字面量（新增的用户可见文本）

`addname.go` 的六个说法与候选行，Go 的格式串逐字：

```text
配置里和识别结果里都没有 %s；不带参数的 /add 列出配置里的成员与识别出的模型，识别表里的模型可以直接 /add 它的 ID\n
用法: /add <识别表里的 ID 或模型名> [--id <新 id>] [--effort <强度>]；带 --id 或 --effort 时只能加一个\n
%s 是识别出的模型，识别出的模型一次加一个：单独 /add %s\n
%s 已经带着强度 %s，不能再给强度 %s\n
%s 对上的是配置里的 agent %s，--id 与 --effort 只用于加识别出的模型；直接 /add %s\n
%s 对上了识别出的 %d 个模型，不猜；用 +编号 或带强度的 ID 指明:\n
  %-6s  %s%s\n
```

- 第一句（`msgAddNameNotFound`）只在开关打开时替换 HEAD 的「配置里没有可加入的 %s，不带参数的 /add 能列出可加的」；开关关着时仍是旧句。两个 `/add` 之间的「；」「，」是全角。
- 用法那一句的半角冒号与 `msgAddModelUsage`（「用法: /add +<编号> …」）同一个款式。
- 候选行：第一个 `%s` 是 provider（左对齐占 6 格，与登录段、首次运行同一种对齐），第二个是 `Model`，第三个是空串或「  +<N>」（两个空格、半角加号、拍快照时的编号）。
- 原型实跑的各种行（逐字）：

```text
配置里和识别结果里都没有 xigh；不带参数的 /add 列出配置里的成员与识别出的模型，识别表里的模型可以直接 /add 它的 ID
gpt-6.1-sol 是识别出的模型，识别出的模型一次加一个：单独 /add gpt-6.1-sol
gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 high
grok-4.7 对上的是配置里的 agent grok-4.7，--id 与 --effort 只用于加识别出的模型；直接 /add grok-4.7
adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:
  codex   adn-x  +2
  codex   adn-x-high  +3
GPT-5.5 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:
  codex   gpt-5.5
  grok    gpt-5.5  +10
```

- v1.1 没有新增用户可见字面量：裁决 (a) 敲错的强度复用 `addDiscoveredModel` 的「%s 支持的强度是 %s，没有 %s」（经 `addByOneName` 走到，不另留一份）；带参数写法里名字是 + 加数字时复用 `msgAddModelUsage`（`addmodel.go:28`，「用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别」）；裁决 (b) 与 (d) 走的是既有的加入、同脑子两句。原型实跑（逐字，夹具四档）：

```text
gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 xigh
gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 xigh
配置里已经有同一个模型、同一强度的 agent gpt-6.1-sol-xhigh，直接 /add gpt-6.1-sol-xhigh
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别
cxh-bell 没有可选的强度，不能带 --effort
```

- 按名字加用到、但不是本刀新增的字面量（逐字复用，出处）：「正在识别模型: %s」「识别被取消；下次 /add 会重新识别」（`ensureModelSnapshot`，原 `printDiscoveredModels`）；追加与加入的两行、同脑子那一句、守卫两句、强度三句、id 检查的说法（`addDiscoveredModel`，原 `cmdAddDiscovered`）；`msgNoRunner`；「X 开头的都已经在当前会话里了」与总结成员前缀两句（`addSegmentNotFound`，原 `resolveAddSegment`）。v1.2 另复用：「%s 已加入会话 %s」与「加入失败」（`joinConfiguredAgent` 照 `cmdAdd` 按配置加入那一支的写法，A9）。

**v1.2 新增四句**（`addname_existing.go` 的常量，D17；上面「v1.1 没有新增」那一条是 v1.1 的记录）。同脑子那一句说的是「同一个模型、同一强度」，强度不同时是假话；HEAD 的「X 开头的都已经在」按 id 前缀说，同样不合适（裁决书第四条），所以新写。Go 的格式串逐字：

```text
%s 对上的模型已经有成员在当前会话里：%s\n
%s 对上的模型已经有成员在当前会话里：%s；要另加一档强度，用 /add %s <强度> 或 --effort <强度>\n
%s 对上的模型在配置里有这几个成员：%s；用 /add <id> 加入其中一个\n
%s 对上的模型在配置里有这几个成员：%s；用 /add <id> 加入其中一个，或用 /add %s <强度> 带强度新加一档\n
```

- 带后半句的两句里第一、第三个 `%s` 是敲的名字，不带后半句的两句只有前两个 `%s`；敲的名字照原样打、不折大小写。第二个 `%s` 是成员 id，按 id 排序、用半角逗号加一个空格连（与 HEAD 的「有歧义，可能是：a, b」同一种连法）。「<强度>」「<id>」是字面的尖括号提示，不是占位符。中文标点（冒号、分号、逗号）是全角。
- 有强度可选的行（claude、codex 的多数行）用带后半句的两句；grok、agy 这类没有强度的行用不带的两句，理由是那半句对它们是假话（`--effort` 会被 `addDiscoveredModel` 拒）。
- 原型实跑的各种行（逐字，S1 的 N22、N9）：

```text
gpt-6.1-sol 对上的模型已经有成员在当前会话里：gpt-6.1-sol-xhigh；要另加一档强度，用 /add gpt-6.1-sol <强度> 或 --effort <强度>
sonnet[1m] 对上的模型已经有成员在当前会话里：sonnet-5-1m-xhigh；要另加一档强度，用 /add sonnet[1m] <强度> 或 --effort <强度>
grok-4.6 对上的模型已经有成员在当前会话里：grok-4.6-a
gpt-6.1-sol 对上的模型在配置里有这几个成员：sol-a, sol-b；用 /add <id> 加入其中一个，或用 /add gpt-6.1-sol <强度> 带强度新加一档
grok-4.6 对上的模型在配置里有这几个成员：g46-a, g46-b；用 /add <id> 加入其中一个
sol-a 已加入会话 <会话 id>
```

---

## 4. 集成员的变异清单

**运行纪律（每一次 go build / vet / test / 变异运行之前都照做，与 19f §4 表头同一套）**：
- 同一条 PowerShell 命令里先设环境再跑（每次工具调用都是新进程，分两次调用等于没设）：
  - `USERPROFILE`、`HOME` 指到隔离目录下的 canary home（起草时是 `E:\Isolating_test\council\ad1_draft\canaryhome`）；`APPDATA`、`LOCALAPPDATA` 指到它下面的 `AppData\Roaming`、`AppData\Local`（先建好）；
  - `COUNCIL_DATA_DIR` 指到隔离目录下的 canary 数据目录；`TMP`、`TEMP` 指到隔离目录下的 `tmp`；
  - 用 `[Environment]::SetEnvironmentVariable('<名>', $null, 'Process')` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`（不要用 `Remove-Item Env:`，会被路径保护整条拦下）；
  - `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE` 指到隔离目录下、`GOTOOLCHAIN=local` 显式给；
  - `PATH` 只留 `C:\Program Files\Go\bin` 与 `C:\Windows\System32`，去掉宿主注入的 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`。
  - 起草人把这些写成一个纯 ASCII 的 `goenv.ps1`，每条命令开头 dot-source 它，再跑 go 或变异驱动（node 由同一个 PowerShell 起，环境随之继承；node 用全路径 `C:\Program Files\nodejs\node.exe`）。集成员自己写的 `.ps1` 若含中文，要存成带 BOM 的 UTF-8。
- 每次 go test（全量、每条变异）之后核四项，任何一项出现就停下、报告实际路径，不删隔离目录以外的任何东西：
  1. canary 数据目录下没有 `council.db`；
  2. 隔离 `TMP`、真 `%TEMP%`（`C:\Users\25142\AppData\Local\Temp`）、`E:\Go\tmp` 下没有 `TestOb*` 或 `TestAdn*` 目录；
  3. `E:\council-data` 不存在；
  4. canary home 下没有 `.council`。

每条在原型上单独施加、从字节副本还原（不用 `git checkout`），还原后 SHA256 核对；「替换前」在文件里恰好出现一次（驱动脚本数过）。包与 `-run`：`addname.go` 的变异跑 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；`commands.go`、`addmodel.go` 的变异跑整个 `./internal/repl`（不带 `-run`，既有用例的杀手也点出来）；`app.go` 的跑 `./internal/app`、`-run 'TestAdn|TestObz|TestObx'`。一律 `go test -json -count=1`，按 fail 事件点名，「杀手」列是原型上这一次实测变红的全部叶子用例（父测试自己的断言红时写父测试名）。耗时是整条变异（编译加测试）的墙钟。替换前后按原文逐字给出（行首是 tab）；「替换后」为空的，是把替换前那几行整段删掉。v1.1：共 66 条。v1 的 48 条里 V4、V10、V12、V13 随代码改了替换前后（V4 的那一行少了一级缩进，V10、V12、V13 的两个词条件换了形状，V13 从「去掉 miss(0)」改成「第一段的条件换成 true」），其余 44 条替换前后逐字不变；V49-V66 是 v1.1 新登记的（W1 的 C101、C103、C104、C105、C106、C107、C115 与裁决 (a)、(b)、(d)、+ 加数字、U+3000 各自的变异）。v1.1 的杀手列与耗时是**最后一轮**实跑的结果：66 条全部跑在 v1.1 的最终代码上（§2 两个文件的【判别力】注释写完之后），之后生产代码没再改，S1 只改过两行注释（N10 的【判别力】一行、`adnSeedMember` 的注释一行；脚本核过去掉注释行之后逐字节相同，行数不变），gofmt、vet、全量、`-count=3` 与往返核对都跑在改完之后的最终字节上。驱动脚本从本节解析出替换前后（与起草时用的清单逐条比过，66 条全同），一条一条施加、跑、从字节副本还原；V60、V61 例外：它们最后改成不以空格结尾的写法，是先改清单、按清单重跑、再写回本节，写回之后从本节解析出来与清单比过，全同。

**v1.2（上一段是 v1.1 的记录；数目、杀手列与耗时以下面 v1.3 那一段为准）**：共 86 条。`addname_existing.go`（A9）的变异与 `addname.go` 同一个包与 `-run`（它只有 AD1 的代码）。
- V1-V66 的替换前后只有 V26 改了：带参数那一支在识别之后多了「前缀命中、快照里对不上」那一行（D9），V26 的替换前换成那三行识别加这一行、替换后只留这一行，意思不变（带参数时没有快照也不识别）。其余 65 条逐字不变，锚点在 v1.2 代码里各恰好一次（驱动脚本数过）。
- V67-V86 是 v1.2 新登记的：V67、V68、V69 是 W1b 自拟的 V101、V102、V103（v1.1 里存活的覆盖缺口，v1.2 加了 N6 typo_second_row、N14 plus_letters、N9 summary_prefix 之后都有杀手）；V70 是 W1b 的 V106，登记为等价；V71-V77 依次对应裁决书「必须有的用例」第 1-7 条；V78-V86 是 D17 的其余几处判据、D9 的前缀命中那两行与 W1b minor 1 的两条新判据。V71、V72、V74、V75、V78、V79、V80 在 A9；V73 起先写在 `addmodel.go`（整包），第一轮之后随 `sameModelAgents` 挪进 A9，替换前后不变，改跑 A9 的 `-run`。
- 跑了两轮：第一轮在 D17 还放在 `addname.go` 与 `addmodel.go` 里的代码上；之后把 D17 挪进 A9（块 B 从 479 行降到 419 行；代码逐字搬，注释改了两处指向新文件的说法，新文件另加了文件头注释），S1 的【判别力】注释按第一轮的实测改写；最后一轮 86 条全部跑在最终字节上（与往返核对用的是同一份字节）。两轮的杀手列逐条相同（脚本比过，86 条 0 处不同）。本节写的是最后一轮：杀手列与耗时由脚本从这一轮的结果文件写进来，再从本节解析回来与结果文件比过，全同。
- 驱动：原型的三份工作副本 `m1`、`m2`、`m3` 并行跑（每份各自的 `TMP`、`TEMP` 是隔离 `tmp` 下的 `m1`、`m2`、`m3`，其余环境同上）。每条之前对工作副本里的目标文件核 SHA256（与原型相同），施加、跑、从内存里的字节副本写回、再核 SHA256，每条之后核四项（本副本的 tmp、真 `%TEMP%`、`E:\Go\tmp`、canarydata、`E:\council-data`、canaryhome）。杀手的认法：`go test -json` 里 fail 的用例取叶子，另加有自己断言输出（`_test.go:行号:`）的父测试。最后一轮 736 个杀手检查、86 条全部还原一致、四项核对次次干净；V7、V70 为 0 个用例变红，其余 84 条的杀手列就是这一轮实测变红的全部用例。耗时（并行三路时的墙钟）：`addname.go`、A9 的每条 36-45 s，整包 repl 的每条 81-89 s，V45 4.7 s，V46 14.4 s。

**v1.3（上面两段是 v1.1、v1.2 的记录，数目、杀手列与耗时以本段为准）**：共 92 条。生产代码与 v1.2 逐字节相同，V1-V86 的替换前后一个字不改；V87-V92 是 v1.3 新登记的，依次是 W1c 的 W1、W2、W3、W4、W5、W7（V87-V91 在 A9，V92 在 `addname.go`，包与 `-run` 同 `addname.go`）。W1c 的 W6 不登记，进「只在审查时看」。
- 为什么 92 条全部重跑：v1.3 的 S1 多了 N25、N22 的三个子用例与 N14 的一条，V1 这类整体变异会把新用例一起杀掉，只跑新的六条的话，V1-V86 的杀手列就不再是「实测变红的全部用例」。所以 92 条全部在 v1.3 原型上重跑，杀手列与耗时一律换成这一轮的实测。
- 第一轮：92 条跑在 S1 的【判别力】注释改写之前的字节上（与最终字节只差注释：脚本去掉 `//` 注释行之后逐字节相同，S1 行数 1018 → 1027）。与 v1.2 的杀手列比（脚本比过）：没有一条少了杀手；13 条多了杀手，多出来的全是 v1.3 的新用例（V1、V5、V17、V29、V30、V32、V34、V35、V37、V38、V71、V72、V79）；V7、V70 仍是 0 个用例变红。
- 第二轮：注释改完之后，V87-V92 在最终字节上再跑一次，杀手与第一轮逐条相同。本节 V87-V92 的耗时取第二轮，其余取第一轮。
- 驱动与 v1.2 相同：三份工作副本 `m1`、`m2`、`m3` 并行，各自的 `TMP`、`TEMP`；每条之前核目标文件 SHA256 与原型相同，施加（「替换前」恰好一次：整行的按行首到行尾数，一行里的一段按子串数），跑 `go test -json -count=1`，从内存里的字节副本写回，再核 SHA256；每条之后核四项（本副本的 tmp、真 `%TEMP%`、`E:\Go\tmp`、canarydata、`E:\council-data`、canaryhome）。杀手的认法同 v1.2。第一轮 774 个杀手检查、92 条全部还原一致、四项核对次次干净、没有一条编译失败；第二轮 6 条同样。耗时（并行三路时的墙钟）：`addname.go`、A9 的每条 33.4-42.7 s，整包 repl 的每条 76.7-88.0 s，V45 4.0 s，V46 13.7 s。第一轮分三次调用跑完（每次不超过 10 分钟）。

**V1** `internal/repl/addname.go`：SetAddByName 不记下参数，开关永远是关。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	r.addByName = on
```

替换后：

```go
	r.addByName = false
```

杀手（原型实测，98 个叶子用例）：`TestAdnAddByID/fold`、`TestAdnAddByID/id`、`TestAdnAddByID/join`、`TestAdnAmbiguous/cross_provider`、`TestAdnAmbiguous/no_number`、`TestAdnAmbiguous/same_provider`、`TestAdnAmbiguous/two_words`、`TestAdnAmbiguous/typo_second_row`、`TestAdnAmbiguous/union_second_row`、`TestAdnCanceled/flag_not_last`、`TestAdnCanceled/last`、`TestAdnCanceled/not_last`、`TestAdnConfiguredRow/other_effort`、`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnConfiguredRow/same_brain`、`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/other_provider_same_model`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnExistingMembers/repeat`、`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/several_absent_grok`、`TestAdnExistingMembers/summary_excluded`、`TestAdnFlags`、`TestAdnFlags/plus_letters`、`TestAdnFlagsOverConfigPrefix/effort_over_other_model`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnFlagsOverConfigPrefix/prefix_no_match`、`TestAdnGuard/flag`、`TestAdnGuard/id`、`TestAdnGuard/two_words`、`TestAdnIdeographicSpace/on_flag`、`TestAdnIdeographicSpace/on_two_words`、`TestAdnModelForm/ambiguous_with_effort`、`TestAdnModelForm/default_effort`、`TestAdnModelForm/effort_case`、`TestAdnModelForm/effort_flag`、`TestAdnModelForm/flag_first`、`TestAdnModelForm/grok`、`TestAdnModelForm/grok_effort`、`TestAdnModelForm/id_flag`、`TestAdnModelForm/id_high`、`TestAdnModelForm/id_other_effort`、`TestAdnModelForm/id_same_effort`、`TestAdnModelForm/no_efforts_codex`、`TestAdnModelForm/unsupported`、`TestAdnMultiSegment/all_config_refusal`、`TestAdnMultiSegment/ambiguous_seg`、`TestAdnMultiSegment/three`、`TestAdnMultiSegment/with_config`、`TestAdnNoRunner`、`TestAdnNotFound/codex_failed`、`TestAdnNotFound/no_providers`、`TestAdnNotFound/snapshot_kept`、`TestAdnOtherProviders/agy_alias`、`TestAdnOtherProviders/alias_configured`、`TestAdnOtherProviders/alias_same_brain`、`TestAdnOtherProviders/claude_alias`、`TestAdnOtherProviders/claude_alias_fold`、`TestAdnOtherProviders/claude_base`、`TestAdnOtherProviders/claude_base_second`、`TestAdnOtherProviders/claude_first_effort`、`TestAdnOtherProviders/claude_id`、`TestAdnOtherProviders/claude_model`、`TestAdnOtherProviders/claude_model_fold`、`TestAdnOtherProviders/haiku_effort`、`TestAdnOtherProviders/haiku_fold`、`TestAdnPriority`、`TestAdnSharesSnapshot/by_name_first`、`TestAdnSharesSnapshot/list_first`、`TestAdnSingleIDConfigPrefix`、`TestAdnSwitchOffUnchanged`、`TestAdnTwoWords/ambiguous_typo`、`TestAdnTwoWords/config_first`、`TestAdnTwoWords/digit`、`TestAdnTwoWords/effort_is_config_prefix`、`TestAdnTwoWords/id_then_effort`、`TestAdnTwoWords/id_then_other`、`TestAdnTwoWords/id_then_typo`、`TestAdnTwoWords/model_effort`、`TestAdnTwoWords/no_efforts`、`TestAdnTwoWords/second_is_model`、`TestAdnTwoWords/typo`、`TestAdnTwoWords/upper`、`TestAdnTwoWordsConfigPrefix`、`TestAdnTypoIsReference/id_present_prefix`、`TestAdnTypoIsReference/plus`、`TestAdnTypoIsReference/present_prefix`、`TestAdnTypoIsReference/summary_prefix`

**V2** `internal/repl/addname.go`：模型名形式按字节比（不折大小写）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 39.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if strings.EqualFold(name, m.Model) || (m.Alias != "" && strings.EqualFold(name, m.Alias)) {
```

替换后：

```go
		if name == m.Model || (m.Alias != "" && name == m.Alias) {
```

杀手（原型实测，2 个叶子用例）：`TestAdnOtherProviders/claude_alias_fold`、`TestAdnOtherProviders/claude_model_fold`

**V3** `internal/repl/addname.go`：有强度的行，ID 形式按字节比（不折大小写）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if strings.EqualFold(name, m.SuggestedID(e)) {
```

替换后：

```go
		if name == m.SuggestedID(e) {
```

杀手（原型实测，2 个叶子用例）：`TestAdnAddByID/fold`、`TestAdnConfiguredRow/same_brain`

**V4** `internal/repl/addname.go`：不认不带强度的 ID 形式（SuggestedID("")，没有强度的行与有强度的行都不认）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return "", strings.EqualFold(name, m.SuggestedID(""))
```

替换后：

```go
	return "", false
```

杀手（原型实测，4 个叶子用例）：`TestAdnOtherProviders/claude_base`、`TestAdnOtherProviders/claude_base_second`、`TestAdnOtherProviders/haiku_effort`、`TestAdnOtherProviders/haiku_fold`

**V5** `internal/repl/addname.go`：按名字只找有编号的行（拍快照时已在配置里的行加不了，open item (c) 没解决）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	for _, row := range snap.rows {
		m := row.m
```

替换后：

```go
	for _, row := range snap.rows {
		if row.no == 0 {
			continue
		}
		m := row.m
```

杀手（原型实测，12 个叶子用例）：`TestAdnAmbiguous/no_number`、`TestAdnConfiguredRow/other_effort`、`TestAdnConfiguredRow/same_brain`、`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/several_absent_grok`、`TestAdnExistingMembers/summary_excluded`、`TestAdnOtherProviders/alias_configured`、`TestAdnOtherProviders/alias_same_brain`

**V6** `internal/repl/addname.go`：模型名形式不认 Alias。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if strings.EqualFold(name, m.Model) || (m.Alias != "" && strings.EqualFold(name, m.Alias)) {
```

替换后：

```go
		if strings.EqualFold(name, m.Model) {
```

杀手（原型实测，7 个叶子用例）：`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/alias_present`、`TestAdnOtherProviders/alias_configured`、`TestAdnOtherProviders/alias_same_brain`、`TestAdnOtherProviders/claude_alias`、`TestAdnOtherProviders/claude_alias_fold`、`TestAdnOtherProviders/claude_first_effort`

**V7** `internal/repl/addname.go`：去掉 Alias 非空的判据（名字非空，EqualFold(name, "") 恒假，等价）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
(m.Alias != "" && strings.EqualFold(name, m.Alias))
```

替换后：

```go
strings.EqualFold(name, m.Alias)
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

等价（原型实测 0 个用例变红）：名字非空时 `strings.EqualFold(name, "")` 恒为假，去掉 `m.Alias != ""` 不改结果。只登记、不要求杀手。

**V8** `internal/repl/addname.go`：模型名形式只认 Alias，不认 Model。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if strings.EqualFold(name, m.Model) || (m.Alias != "" && strings.EqualFold(name, m.Alias)) {
```

替换后：

```go
		if m.Alias != "" && strings.EqualFold(name, m.Alias) {
```

杀手（原型实测，2 个叶子用例）：`TestAdnOtherProviders/claude_model`、`TestAdnOtherProviders/claude_model_fold`

**V9** `internal/repl/addname.go`：对上多行时不拒绝，取第一行。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	case len(ms) > 1:
```

替换后：

```go
	case len(ms) > 1 && false:
```

杀手（原型实测，8 个叶子用例）：`TestAdnAmbiguous/cross_provider`、`TestAdnAmbiguous/no_number`、`TestAdnAmbiguous/same_provider`、`TestAdnAmbiguous/two_words`、`TestAdnAmbiguous/typo_second_row`、`TestAdnAmbiguous/union_second_row`、`TestAdnModelForm/ambiguous_with_effort`、`TestAdnTwoWords/ambiguous_typo`

**V10** `internal/repl/addname.go`：不认两个词的写法。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.8 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if len(segs) == 2 && miss(1) && (miss(0) || configPrefixOnly(segs[0], ids[0])) {
```

替换后：

```go
	if false && len(segs) == 2 && miss(1) && (miss(0) || configPrefixOnly(segs[0], ids[0])) {
```

杀手（原型实测，16 个叶子用例）：`TestAdnAmbiguous/two_words`、`TestAdnAmbiguous/typo_second_row`、`TestAdnAmbiguous/union_second_row`、`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/several_absent`、`TestAdnGuard/two_words`、`TestAdnIdeographicSpace/on_two_words`、`TestAdnOtherProviders/claude_base_second`、`TestAdnTwoWords/ambiguous_typo`、`TestAdnTwoWords/id_then_effort`、`TestAdnTwoWords/id_then_other`、`TestAdnTwoWords/id_then_typo`、`TestAdnTwoWords/model_effort`、`TestAdnTwoWords/typo`、`TestAdnTwoWords/upper`、`TestAdnTwoWordsConfigPrefix`

**V11** `internal/repl/addname.go`：第二个词当强度时不区分大小写。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if x == e {
```

替换后：

```go
		if strings.EqualFold(x, e) {
```

杀手（原型实测，1 个叶子用例）：`TestAdnTwoWordsConfigPrefix`

**V12** `internal/repl/addname.go`：两个词的写法不要求第二个词在配置里没命中。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if len(segs) == 2 && miss(1) && (miss(0) || configPrefixOnly(segs[0], ids[0])) {
```

替换后：

```go
	if len(segs) == 2 && (miss(0) || configPrefixOnly(segs[0], ids[0])) {
```

杀手（原型实测，2 个叶子用例）：`TestAdnTwoWords/digit`、`TestAdnTwoWords/effort_is_config_prefix`

**V13** `internal/repl/addname.go`：两个词的写法不看第一段在配置里的结果（全名、编号命中也算）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if len(segs) == 2 && miss(1) && (miss(0) || configPrefixOnly(segs[0], ids[0])) {
```

替换后：

```go
	if len(segs) == 2 && miss(1) && true {
```

杀手（原型实测，1 个叶子用例）：`TestAdnTwoWords/config_first`

**V14** `internal/repl/addname.go`：ID 形式已带强度、后面又跟一档强度时不拒绝（照名字里的强度加）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if second != "" {
			return r.printf(msgAddNameEffortTwice, name, mt.effort, second)
		}
```

替换后（为空：把替换前那几行整段删掉）：


杀手（原型实测，3 个叶子用例）：`TestAdnTwoWords/id_then_effort`、`TestAdnTwoWords/id_then_other`、`TestAdnTwoWords/id_then_typo`

**V15** `internal/repl/addname.go`：ID 形式已带强度、--effort 给了相同的一档也拒绝。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 40.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if haveEffort && effort != mt.effort {
```

替换后：

```go
		if haveEffort {
```

杀手（原型实测，1 个叶子用例）：`TestAdnModelForm/id_same_effort`

**V16** `internal/repl/addname.go`：ID 形式已带强度、--effort 给了不同的一档时不拒绝（照名字里的强度加）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 40.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if haveEffort && effort != mt.effort {
			return r.printf(msgAddNameEffortTwice, name, mt.effort, effort)
		}
```

替换后（为空：把替换前那几行整段删掉）：


杀手（原型实测，1 个叶子用例）：`TestAdnModelForm/id_other_effort`

**V17** `internal/repl/addname.go`：ID 形式名字里带的强度不用（落到默认档）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 38.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		effort, haveEffort = mt.effort, true
	} else if second != "" {
```

替换后：

```go
	} else if second != "" {
```

杀手（原型实测，26 个叶子用例）：`TestAdnAddByID/fold`、`TestAdnAddByID/id`、`TestAdnAddByID/join`、`TestAdnAmbiguous/cross_provider`、`TestAdnCanceled/last`、`TestAdnCanceled/not_last`、`TestAdnConfiguredRow/other_effort`、`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/repeat`、`TestAdnExistingMembers/summary_excluded`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnModelForm/id_flag`、`TestAdnMultiSegment/all_config_refusal`、`TestAdnOtherProviders/claude_id`、`TestAdnSharesSnapshot/by_name_first`、`TestAdnSharesSnapshot/list_first`、`TestAdnSingleIDConfigPrefix`、`TestAdnSwitchOffUnchanged`、`TestAdnTwoWordsConfigPrefix`

**V18** `internal/repl/addname.go`：两个词的写法第二个词不当强度（落到默认档）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	} else if second != "" {
		effort, haveEffort = second, true
	}
```

替换后：

```go
	}
```

杀手（原型实测，8 个叶子用例）：`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/several_absent`、`TestAdnIdeographicSpace/on_two_words`、`TestAdnOtherProviders/claude_base_second`、`TestAdnTwoWords/model_effort`、`TestAdnTwoWords/typo`、`TestAdnTwoWords/upper`、`TestAdnTwoWordsConfigPrefix`

**V19** `internal/repl/addname.go`：按名字加不判 Runner 在不在。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if r.rn == nil {
		return r.printf("%s\n", msgNoRunner)
	}
	return addDiscoveredModel(
```

替换后：

```go
	return addDiscoveredModel(
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnNoRunner`

**V20** `internal/repl/addname.go`：守卫那一句里「再 /add」后面带上第二个词。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return addDiscoveredModel(ctx, r, mt.row.m, effort, haveEffort, a.newID, a.haveID, name)
```

替换后：

```go
	return addDiscoveredModel(ctx, r, mt.row.m, effort, haveEffort, a.newID, a.haveID, strings.TrimSpace(name+" "+second))
```

杀手（原型实测，1 个叶子用例）：`TestAdnGuard/two_words`

**V21** `internal/repl/addname.go`：不认 --id / --effort（按多段解析）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if f == "--id" || f == "--effort" {
```

替换后：

```go
		if f == "--no-such-flag" {
```

杀手（原型实测，25 个叶子用例）：`TestAdnCanceled/flag_not_last`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnFlags`、`TestAdnFlags/plus_letters`、`TestAdnFlagsOverConfigPrefix/effort_over_other_model`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnFlagsOverConfigPrefix/prefix_no_match`、`TestAdnGuard/flag`、`TestAdnIdeographicSpace/on_flag`、`TestAdnModelForm/ambiguous_with_effort`、`TestAdnModelForm/effort_case`、`TestAdnModelForm/effort_flag`、`TestAdnModelForm/flag_first`、`TestAdnModelForm/grok_effort`、`TestAdnModelForm/id_flag`、`TestAdnModelForm/id_other_effort`、`TestAdnModelForm/id_same_effort`、`TestAdnModelForm/no_efforts_codex`、`TestAdnModelForm/unsupported`、`TestAdnOtherProviders/alias_same_brain`、`TestAdnOtherProviders/haiku_effort`、`TestAdnPriority`

**V22** `internal/repl/addname.go`：带参数时多个名字不拒绝，取第一个。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if len(names) != 1 {
```

替换后：

```go
	if len(names) == 0 {
```

杀手（原型实测，1 个叶子用例）：`TestAdnFlags`

**V23** `internal/repl/addname.go`：带参数时名字不按逗号切（a,b 算一个名字）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			names = append(names, splitAddArgs(fields[i])...)
```

替换后：

```go
			names = append(names, fields[i])
```

杀手（原型实测，1 个叶子用例）：`TestAdnFlags`

**V24** `internal/repl/addname.go`：带参数、名字在配置里命中一个 agent 时不说明，接着到快照里找。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		} else if id != "" {
```

替换后：

```go
		} else if id != "" && false {
```

杀手（原型实测，2 个叶子用例）：`TestAdnFlags`、`TestAdnFlagsOverConfigPrefix/prefix_no_match`

**V25** `internal/repl/addname.go`：带参数、名字在配置里命中的是一句拒绝时不打它，接着到快照里找。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if id, problem := resolveAddConfig(r, name, candidates, active, member); problem != "" {
```

替换后：

```go
		if id, problem := resolveAddConfig(r, name, candidates, active, member); problem != "" && false {
```

杀手（原型实测，1 个叶子用例）：`TestAdnFlags`

**V26** `internal/repl/addname.go`：带参数时没有快照也不识别。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if canceled, err := snapshotForName(ctx, r); canceled || err != nil {
			return true, err
		}
		if prefixID != "" && len(matchDiscoveredName(r.modelSnap, name)) == 0 {
```

替换后：

```go
		if prefixID != "" && len(matchDiscoveredName(r.modelSnap, name)) == 0 {
```

杀手（原型实测，20 个叶子用例）：`TestAdnCanceled/flag_not_last`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnFlags/plus_letters`、`TestAdnFlagsOverConfigPrefix/effort_over_other_model`、`TestAdnFlagsOverConfigPrefix/prefix_no_match`、`TestAdnGuard/flag`、`TestAdnIdeographicSpace/on_flag`、`TestAdnModelForm/ambiguous_with_effort`、`TestAdnModelForm/effort_case`、`TestAdnModelForm/effort_flag`、`TestAdnModelForm/flag_first`、`TestAdnModelForm/grok_effort`、`TestAdnModelForm/id_flag`、`TestAdnModelForm/id_other_effort`、`TestAdnModelForm/id_same_effort`、`TestAdnModelForm/no_efforts_codex`、`TestAdnModelForm/unsupported`、`TestAdnOtherProviders/alias_same_brain`、`TestAdnOtherProviders/haiku_effort`、`TestAdnPriority`

**V27** `internal/repl/addname.go`：多段里在快照里对上的一段不说「一次加一个」，打没找到。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			if len(matchDiscoveredName(r.modelSnap, seg)) > 0 {
```

替换后：

```go
			if false {
```

杀手（原型实测，11 个叶子用例）：`TestAdnMultiSegment/ambiguous_seg`、`TestAdnMultiSegment/three`、`TestAdnMultiSegment/with_config`、`TestAdnTwoWords/digit`、`TestAdnTwoWords/effort_is_config_prefix`、`TestAdnTwoWords/no_efforts`、`TestAdnTwoWords/second_is_model`、`TestAdnTypoIsReference/id_present_prefix`、`TestAdnTypoIsReference/plus`、`TestAdnTypoIsReference/present_prefix`、`TestAdnTypoIsReference/summary_prefix`

**V28** `internal/repl/addname.go`：多段里在配置里命中的拒绝句不打。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 37.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		problem := problems[i]
```

替换后：

```go
		problem := ""
```

杀手（原型实测，1 个叶子用例）：`TestAdnMultiSegment/three`

**V29** `internal/repl/addname.go`：每一段都在配置里命中时也插手（不交回 cmdAdd）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if !anyMiss {
```

替换后：

```go
	if !anyMiss && false {
```

杀手（原型实测，15 个叶子用例）：`TestAdnConfiguredRow/prefix`、`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnMultiSegment/all_config`、`TestAdnMultiSegment/all_config_refusal`、`TestAdnPriority`、`TestAdnSingleIDConfigPrefix`、`TestAdnTwoWordsConfigPrefix`、`TestAdnTypoIsReference/id_present_prefix`、`TestAdnTypoIsReference/present_prefix`

**V30** `internal/repl/addname.go`：只有一段时不按名字加（落进多段的逐段说法）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if len(segs) == 1 {
		return true, addByOneName(ctx, r, segs[0], "", addNameArgs{}, active, member)
	}
```

替换后（为空：把替换前那几行整段删掉）：


杀手（原型实测，52 个叶子用例）：`TestAdnAddByID/fold`、`TestAdnAddByID/id`、`TestAdnAddByID/join`、`TestAdnAmbiguous/cross_provider`、`TestAdnAmbiguous/no_number`、`TestAdnAmbiguous/same_provider`、`TestAdnCanceled/last`、`TestAdnCanceled/not_last`、`TestAdnConfiguredRow/other_effort`、`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnConfiguredRow/same_brain`、`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/other_provider_same_model`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnExistingMembers/repeat`、`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/several_absent_grok`、`TestAdnExistingMembers/summary_excluded`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnGuard/id`、`TestAdnModelForm/default_effort`、`TestAdnModelForm/grok`、`TestAdnModelForm/id_high`、`TestAdnMultiSegment/all_config_refusal`、`TestAdnNoRunner`、`TestAdnNotFound/codex_failed`、`TestAdnOtherProviders/agy_alias`、`TestAdnOtherProviders/alias_configured`、`TestAdnOtherProviders/claude_alias`、`TestAdnOtherProviders/claude_alias_fold`、`TestAdnOtherProviders/claude_base`、`TestAdnOtherProviders/claude_first_effort`、`TestAdnOtherProviders/claude_id`、`TestAdnOtherProviders/claude_model`、`TestAdnOtherProviders/claude_model_fold`、`TestAdnOtherProviders/haiku_fold`、`TestAdnPriority`、`TestAdnSharesSnapshot/by_name_first`、`TestAdnSharesSnapshot/list_first`、`TestAdnSingleIDConfigPrefix`、`TestAdnSwitchOffUnchanged`、`TestAdnTwoWordsConfigPrefix`

**V31** `internal/repl/addname.go`：按名字加每次都重拍快照。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if r.modelSnap != nil {
		return false, nil
	}
	pids := discoverableProviders(r.cfg)
```

替换后：

```go
	r.modelSnap = nil
	pids := discoverableProviders(r.cfg)
```

杀手（原型实测，17 个叶子用例）：`TestAdnAmbiguous/cross_provider`、`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/repeat`、`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/summary_excluded`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnFlagsOverConfigPrefix/prefix_no_match`、`TestAdnNotFound/codex_failed`、`TestAdnNotFound/snapshot_kept`、`TestAdnPriority`、`TestAdnSharesSnapshot/list_first`、`TestAdnTwoWordsConfigPrefix`

**V32** `internal/repl/addname.go`：按名字加从不自动识别（只用已有快照）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return ensureModelSnapshot(ctx, r, pids)
}
```

替换后：

```go
	return false, nil
}
```

杀手（原型实测，95 个叶子用例）：`TestAdnAddByID/fold`、`TestAdnAddByID/id`、`TestAdnAddByID/join`、`TestAdnAmbiguous/cross_provider`、`TestAdnAmbiguous/no_number`、`TestAdnAmbiguous/same_provider`、`TestAdnAmbiguous/two_words`、`TestAdnAmbiguous/typo_second_row`、`TestAdnAmbiguous/union_second_row`、`TestAdnCanceled/flag_not_last`、`TestAdnCanceled/last`、`TestAdnCanceled/not_last`、`TestAdnConfiguredRow/other_effort`、`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnConfiguredRow/same_brain`、`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/other_provider_same_model`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnExistingMembers/repeat`、`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/several_absent_grok`、`TestAdnExistingMembers/summary_excluded`、`TestAdnFlags/plus_letters`、`TestAdnFlagsOverConfigPrefix/effort_over_other_model`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnFlagsOverConfigPrefix/prefix_no_match`、`TestAdnGuard/flag`、`TestAdnGuard/id`、`TestAdnGuard/two_words`、`TestAdnIdeographicSpace/on_flag`、`TestAdnIdeographicSpace/on_two_words`、`TestAdnModelForm/ambiguous_with_effort`、`TestAdnModelForm/default_effort`、`TestAdnModelForm/effort_case`、`TestAdnModelForm/effort_flag`、`TestAdnModelForm/flag_first`、`TestAdnModelForm/grok`、`TestAdnModelForm/grok_effort`、`TestAdnModelForm/id_flag`、`TestAdnModelForm/id_high`、`TestAdnModelForm/id_other_effort`、`TestAdnModelForm/id_same_effort`、`TestAdnModelForm/no_efforts_codex`、`TestAdnModelForm/unsupported`、`TestAdnMultiSegment/all_config_refusal`、`TestAdnMultiSegment/ambiguous_seg`、`TestAdnMultiSegment/three`、`TestAdnMultiSegment/with_config`、`TestAdnNoRunner`、`TestAdnNotFound/codex_failed`、`TestAdnNotFound/snapshot_kept`、`TestAdnOtherProviders/agy_alias`、`TestAdnOtherProviders/alias_configured`、`TestAdnOtherProviders/alias_same_brain`、`TestAdnOtherProviders/claude_alias`、`TestAdnOtherProviders/claude_alias_fold`、`TestAdnOtherProviders/claude_base`、`TestAdnOtherProviders/claude_base_second`、`TestAdnOtherProviders/claude_first_effort`、`TestAdnOtherProviders/claude_id`、`TestAdnOtherProviders/claude_model`、`TestAdnOtherProviders/claude_model_fold`、`TestAdnOtherProviders/haiku_effort`、`TestAdnOtherProviders/haiku_fold`、`TestAdnPriority`、`TestAdnSharesSnapshot/by_name_first`、`TestAdnSingleIDConfigPrefix`、`TestAdnSwitchOffUnchanged`、`TestAdnTwoWords/ambiguous_typo`、`TestAdnTwoWords/config_first`、`TestAdnTwoWords/digit`、`TestAdnTwoWords/effort_is_config_prefix`、`TestAdnTwoWords/id_then_effort`、`TestAdnTwoWords/id_then_other`、`TestAdnTwoWords/id_then_typo`、`TestAdnTwoWords/model_effort`、`TestAdnTwoWords/no_efforts`、`TestAdnTwoWords/second_is_model`、`TestAdnTwoWords/typo`、`TestAdnTwoWords/upper`、`TestAdnTwoWordsConfigPrefix`、`TestAdnTypoIsReference/id_present_prefix`、`TestAdnTypoIsReference/plus`、`TestAdnTypoIsReference/present_prefix`、`TestAdnTypoIsReference/summary_prefix`

**V33** `internal/repl/addname.go`：按名字加自动识别之后打登录段。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return ensureModelSnapshot(ctx, r, pids)
}
```

替换后：

```go
	canceled, err := ensureModelSnapshot(ctx, r, pids)
	if !canceled && err == nil {
		err = printLoginStatus(r, r.modelSnap)
	}
	return canceled, err
}
```

杀手（原型实测，1 个叶子用例）：`TestAdnSharesSnapshot/by_name_first`

**V34** `internal/repl/addname.go`：「开头的都已经在」与总结成员前缀两句排到识别快照之前。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	ms := matchDiscoveredName(r.modelSnap, name)
	switch {
```

替换后：

```go
	if s := addSegmentNotFound(r, name, active, member); strings.Contains(s, "开头的") {
		return r.printf("%s", s)
	}
	ms := matchDiscoveredName(r.modelSnap, name)
	switch {
```

杀手（原型实测，5 个叶子用例）：`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/repeat`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnPriority`、`TestAdnSingleIDConfigPrefix`

**V35** `internal/repl/addname.go`：一行按 ID 形式对上之后不 continue，同一行可能按模型名形式再算一次。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.8 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			out = append(out, nameMatch{row: row, byID: true, effort: e})
			continue
```

替换后：

```go
			out = append(out, nameMatch{row: row, byID: true, effort: e})
```

杀手（原型实测，42 个叶子用例）：`TestAdnAmbiguous/cross_provider`、`TestAdnAmbiguous/no_number`、`TestAdnAmbiguous/same_provider`、`TestAdnAmbiguous/two_words`、`TestAdnAmbiguous/typo_second_row`、`TestAdnAmbiguous/union_second_row`、`TestAdnCanceled/flag_not_last`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/other_provider_same_model`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnExistingMembers/repeat`、`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/several_absent_grok`、`TestAdnExistingMembers/summary_excluded`、`TestAdnFlagsOverConfigPrefix/effort_over_other_model`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnGuard/flag`、`TestAdnGuard/two_words`、`TestAdnIdeographicSpace/on_flag`、`TestAdnIdeographicSpace/on_two_words`、`TestAdnModelForm/ambiguous_with_effort`、`TestAdnModelForm/default_effort`、`TestAdnModelForm/effort_case`、`TestAdnModelForm/effort_flag`、`TestAdnModelForm/flag_first`、`TestAdnModelForm/grok`、`TestAdnModelForm/grok_effort`、`TestAdnModelForm/no_efforts_codex`、`TestAdnModelForm/unsupported`、`TestAdnNotFound/codex_failed`、`TestAdnOtherProviders/agy_alias`、`TestAdnPriority`、`TestAdnTwoWords/ambiguous_typo`、`TestAdnTwoWords/model_effort`、`TestAdnTwoWords/typo`、`TestAdnTwoWords/upper`、`TestAdnTwoWordsConfigPrefix`

**V36** `internal/repl/addname.go`：候选行里没有编号的也带 +0。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			if mt.row.no > 0 {
```

替换后：

```go
			if mt.row.no >= 0 {
```

杀手（原型实测，1 个叶子用例）：`TestAdnAmbiguous/no_number`

**V37** `internal/repl/addname.go`：有强度的行只认默认档的 ID。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	for _, e := range m.Efforts {
		if strings.EqualFold(name, m.SuggestedID(e)) {
			return e, true
		}
	}
```

替换后：

```go
	if e := m.DefaultEffort(); strings.EqualFold(name, m.SuggestedID(e)) {
		return e, true
	}
```

杀手（原型实测，36 个叶子用例）：`TestAdnAddByID/fold`、`TestAdnAddByID/id`、`TestAdnAddByID/join`、`TestAdnAmbiguous/cross_provider`、`TestAdnCanceled/last`、`TestAdnCanceled/not_last`、`TestAdnConfiguredRow/other_effort`、`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/repeat`、`TestAdnExistingMembers/summary_excluded`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnGuard/id`、`TestAdnModelForm/id_flag`、`TestAdnModelForm/id_other_effort`、`TestAdnModelForm/id_same_effort`、`TestAdnMultiSegment/all_config_refusal`、`TestAdnMultiSegment/three`、`TestAdnMultiSegment/with_config`、`TestAdnNoRunner`、`TestAdnOtherProviders/claude_id`、`TestAdnSharesSnapshot/by_name_first`、`TestAdnSharesSnapshot/list_first`、`TestAdnSingleIDConfigPrefix`、`TestAdnSwitchOffUnchanged`、`TestAdnTwoWords/id_then_effort`、`TestAdnTwoWords/id_then_other`、`TestAdnTwoWords/id_then_typo`、`TestAdnTwoWordsConfigPrefix`、`TestAdnTypoIsReference/id_present_prefix`

**V38** `internal/repl/commands.go`：cmdAdd 不接 cmdAddByName（开关打开也不按名字加）。 包 `./internal/repl`（整包，不带 -run）；原型实测 79.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if r.addByName {
		if done, err := cmdAddByName(
```

替换后：

```go
	if false {
		if done, err := cmdAddByName(
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，97 个叶子用例）：`TestAdnAddByID/fold`、`TestAdnAddByID/id`、`TestAdnAddByID/join`、`TestAdnAmbiguous/cross_provider`、`TestAdnAmbiguous/no_number`、`TestAdnAmbiguous/same_provider`、`TestAdnAmbiguous/two_words`、`TestAdnAmbiguous/typo_second_row`、`TestAdnAmbiguous/union_second_row`、`TestAdnCanceled/flag_not_last`、`TestAdnCanceled/last`、`TestAdnCanceled/not_last`、`TestAdnConfiguredRow/other_effort`、`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnConfiguredRow/same_brain`、`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/other_provider_same_model`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnExistingMembers/repeat`、`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/several_absent_grok`、`TestAdnExistingMembers/summary_excluded`、`TestAdnFlags`、`TestAdnFlags/plus_letters`、`TestAdnFlagsOverConfigPrefix/effort_over_other_model`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnFlagsOverConfigPrefix/prefix_no_match`、`TestAdnGuard/flag`、`TestAdnGuard/id`、`TestAdnGuard/two_words`、`TestAdnIdeographicSpace/on_flag`、`TestAdnIdeographicSpace/on_two_words`、`TestAdnModelForm/ambiguous_with_effort`、`TestAdnModelForm/default_effort`、`TestAdnModelForm/effort_case`、`TestAdnModelForm/effort_flag`、`TestAdnModelForm/flag_first`、`TestAdnModelForm/grok`、`TestAdnModelForm/grok_effort`、`TestAdnModelForm/id_flag`、`TestAdnModelForm/id_high`、`TestAdnModelForm/id_other_effort`、`TestAdnModelForm/id_same_effort`、`TestAdnModelForm/no_efforts_codex`、`TestAdnModelForm/unsupported`、`TestAdnMultiSegment/all_config_refusal`、`TestAdnMultiSegment/ambiguous_seg`、`TestAdnMultiSegment/three`、`TestAdnMultiSegment/with_config`、`TestAdnNoRunner`、`TestAdnNotFound/codex_failed`、`TestAdnNotFound/snapshot_kept`、`TestAdnOtherProviders/agy_alias`、`TestAdnOtherProviders/alias_configured`、`TestAdnOtherProviders/alias_same_brain`、`TestAdnOtherProviders/claude_alias`、`TestAdnOtherProviders/claude_alias_fold`、`TestAdnOtherProviders/claude_base`、`TestAdnOtherProviders/claude_base_second`、`TestAdnOtherProviders/claude_first_effort`、`TestAdnOtherProviders/claude_id`、`TestAdnOtherProviders/claude_model`、`TestAdnOtherProviders/claude_model_fold`、`TestAdnOtherProviders/haiku_effort`、`TestAdnOtherProviders/haiku_fold`、`TestAdnPriority`、`TestAdnSharesSnapshot/by_name_first`、`TestAdnSharesSnapshot/list_first`、`TestAdnSingleIDConfigPrefix`、`TestAdnSwitchOffUnchanged`、`TestAdnTwoWords/ambiguous_typo`、`TestAdnTwoWords/config_first`、`TestAdnTwoWords/digit`、`TestAdnTwoWords/effort_is_config_prefix`、`TestAdnTwoWords/id_then_effort`、`TestAdnTwoWords/id_then_other`、`TestAdnTwoWords/id_then_typo`、`TestAdnTwoWords/model_effort`、`TestAdnTwoWords/no_efforts`、`TestAdnTwoWords/second_is_model`、`TestAdnTwoWords/typo`、`TestAdnTwoWords/upper`、`TestAdnTwoWordsConfigPrefix`、`TestAdnTypoIsReference/id_present_prefix`、`TestAdnTypoIsReference/plus`、`TestAdnTypoIsReference/present_prefix`、`TestAdnTypoIsReference/summary_prefix`

**V39** `internal/repl/commands.go`：开关打开时最后一句仍是旧说法。 包 `./internal/repl`（整包，不带 -run）；原型实测 88.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if r.addByName {
		return fmt.Sprintf(msgAddNameNotFound, seg)
	}
```

替换后（为空：把替换前那几行整段删掉）：


杀手（原型实测，14 个叶子用例）：`TestAdnAmbiguous/typo_second_row`、`TestAdnFlags/plus_letters`、`TestAdnMultiSegment/three`、`TestAdnNotFound/codex_failed`、`TestAdnNotFound/no_providers`、`TestAdnNotFound/snapshot_kept`、`TestAdnTwoWords/ambiguous_typo`、`TestAdnTwoWords/config_first`、`TestAdnTwoWords/id_then_typo`、`TestAdnTwoWords/no_efforts`、`TestAdnTwoWords/typo`、`TestAdnTwoWords/upper`、`TestAdnTwoWordsConfigPrefix`、`TestAdnTypoIsReference/plus`

**V40** `internal/repl/commands.go`：开关关着时最后一句也换成新说法。 包 `./internal/repl`（整包，不带 -run）；原型实测 88.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if r.addByName {
		return fmt.Sprintf(msgAddNameNotFound, seg)
```

替换后：

```go
	if true {
		return fmt.Sprintf(msgAddNameNotFound, seg)
```

杀手（原型实测，4 个叶子用例）：`TestAdnSwitchOffUnchanged`、`TestMdxAddRefusals`、`TestObxAddCandidatesWording/prefix_none`、`TestUx2rAddNotFound`

**V41** `internal/repl/commands.go`：resolveAddSegment 只把非空 id 当命中（拒绝句被没找到的说法盖掉）。 包 `./internal/repl`（整包，不带 -run）；原型实测 83.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
id != "" || problem != "" {
```

替换后：

```go
id != "" {
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，18 个叶子用例）：`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnMultiSegment/all_config_refusal`、`TestChxAddHidesSummary/active`、`TestChxAddHidesSummary/explicit`、`TestObxAddCandidatesWording/summary_present`、`TestUx2rAddAllOrNothingValidFirst`、`TestUx2rAddB3GoneFromConfigMember/active_add`、`TestUx2rAddB3GoneFromConfigMember/active_join`、`TestUx2rAddB3GoneFromConfigMember/left_add`、`TestUx2rAddB3GoneFromConfigMember/left_join`、`TestUx2rAddIndexOutOfRange/0`、`TestUx2rAddIndexOutOfRange/99`、`TestUx2rAddIndexOutOfRange/99999999999999999999999`、`TestUx2rAddIndexOutOfRange/候选为空`、`TestUx2rAddReportsEverySegmentError`

**V42** `internal/repl/addmodel.go`：ensureModelSnapshot 取消之后照样存快照。 包 `./internal/repl`（整包，不带 -run）；原型实测 83.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if ctx.Err() != nil {
		return true, r.printf("识别被取消；下次 /add 会重新识别\n")
	}
	r.modelSnap = snap
```

替换后：

```go
	r.modelSnap = snap
```

杀手（原型实测，8 个叶子用例）：`TestAdnCanceled/flag_not_last`、`TestAdnCanceled/last`、`TestAdnCanceled/not_last`、`TestMdxAddCancelNotCached/first`、`TestMdxAddCancelNotCached/late`、`TestObxAddHintCanceled`、`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`

**V43** `internal/repl/addmodel.go`：printDiscoveredModels 识别被取消之后接着打表（快照为 nil）。 包 `./internal/repl`（整包，不带 -run）；原型实测 83.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
canceled, err := ensureModelSnapshot(ctx, r, pids); canceled || err != nil {
```

替换后：

```go
canceled, err := ensureModelSnapshot(ctx, r, pids); err != nil && canceled {
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，5 个叶子用例）：`TestMdxAddCancelNotCached/first`、`TestMdxAddCancelNotCached/late`、`TestObxAddHintCanceled`、`TestObzAddLoginCanceled/last`、`TestObzAddLoginCanceled/not_last`

**V44** `internal/repl/addmodel.go`：cmdAddDiscovered 交给共用函数的守卫词不是 +N。 包 `./internal/repl`（整包，不带 -run）；原型实测 76.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return addDiscoveredModel(ctx, r, row.m, effort, haveEffort, newID, haveID, tok)
```

替换后：

```go
	return addDiscoveredModel(ctx, r, row.m, effort, haveEffort, newID, haveID, "+")
```

杀手（原型实测，5 个叶子用例）：`TestFuxAddGuardCoversSummary`、`TestMdxAddGuard/agent`、`TestMdxAddGuard/providers`、`TestMdxAddGuard/store_thought`、`TestMdxAddGuard/teams`

**V45** `internal/app/app.go`：行式分支不打开开关。 包 `./internal/app`、`-run 'TestAdn|TestObz|TestObx'`；原型实测 4.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		r.SetAddByName(true)
		err = r.Run(ctx)
```

替换后：

```go
		err = r.Run(ctx)
```

杀手（原型实测，1 个叶子用例）：`TestAdnAppLineMode`

**V46** `internal/app/app.go`：TUI 分支不打开开关。 包 `./internal/app`、`-run 'TestAdn|TestObz|TestObx'`；原型实测 13.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		r.SetAddByName(true)
		err = tui.Run(
```

替换后：

```go
		err = tui.Run(
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnAppTUI`

**V47** `internal/repl/addmodel.go`：cmdAddDiscovered 交给共用函数时丢掉 --effort（+编号 一律按默认档加）。 包 `./internal/repl`（整包，不带 -run）；原型实测 77.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return addDiscoveredModel(ctx, r, row.m, effort, haveEffort, newID, haveID, tok)
```

替换后：

```go
	return addDiscoveredModel(ctx, r, row.m, effort, false, newID, haveID, tok)
```

杀手（原型实测，5 个叶子用例）：`TestAdnAddByID/plus`、`TestCxhAddCodexEffortRefusals`、`TestCxhAddCodexWritesConfig`、`TestMdxAddEffortAndDuplicate`、`TestMdxAddRefusals`

**V48** `internal/repl/addmodel.go`：共用函数里强度不分大小写（+编号 与按名字加的 --effort 同时变）。 包 `./internal/repl`（整包，不带 -run）；原型实测 77.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			if e == effort {
```

替换后：

```go
			if strings.EqualFold(e, effort) {
```

杀手（原型实测，3 个叶子用例）：`TestAdnModelForm/effort_case`、`TestAdnTwoWords/upper`、`TestCxhAddCodexEffortRefusals`

**V49** `internal/repl/addname.go`：单段与多段的「没命中」在 anyMiss 那一处只看 id（配置里的拒绝句当成没命中，W1 的 C115）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if miss(i) {
			anyMiss = true
```

替换后：

```go
		if ids[i] == "" {
			anyMiss = true
```

杀手（原型实测，5 个叶子用例）：`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnMultiSegment/all_config_refusal`

**V50** `internal/repl/addname.go`：多段逐段打说法时，「没命中」只看 id（配置里的拒绝句被快照或都没有的说法盖掉，C115 在多段那一处的同类）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if miss(i) {
			if len(matchDiscoveredName(r.modelSnap, seg)) > 0 {
```

替换后：

```go
		if ids[i] == "" {
			if len(matchDiscoveredName(r.modelSnap, seg)) > 0 {
```

杀手（原型实测，1 个叶子用例）：`TestAdnMultiSegment/three`

**V51** `internal/repl/addname.go`：「没命中」的共用判据只看 id（anyMiss、两个词、多段三处同时变）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	miss := func(i int) bool { return ids[i] == "" && problems[i] == "" }
```

替换后：

```go
	miss := func(i int) bool { return ids[i] == "" }
```

杀手（原型实测，6 个叶子用例）：`TestAdnConfiguredRow/refusal_ambiguous`、`TestAdnConfiguredRow/refusal_member_only`、`TestAdnConfiguredRow/refusal_out_of_range`、`TestAdnConfiguredRow/refusal_summary`、`TestAdnMultiSegment/all_config_refusal`、`TestAdnMultiSegment/three`

**V52** `internal/repl/addname.go`：多段里一段在快照里对上多行时不算「一次加一个」（W1 的 C105）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			if len(matchDiscoveredName(r.modelSnap, seg)) > 0 {
```

替换后：

```go
			if len(matchDiscoveredName(r.modelSnap, seg)) == 1 {
```

杀手（原型实测，1 个叶子用例）：`TestAdnMultiSegment/ambiguous_seg`

**V53** `internal/repl/addname.go`：带参数那条路的自动识别被取消时不停（W1 的 C101）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if canceled, err := snapshotForName(ctx, r); canceled || err != nil {
```

替换后：

```go
		if _, err := snapshotForName(ctx, r); err != nil {
```

杀手（原型实测，1 个叶子用例）：`TestAdnCanceled/flag_not_last`

**V54** `internal/repl/addname.go`：配置里一家能识别的都没有时照样调识别（W1 的 C103）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	if len(pids) == 0 {
		return false, nil
	}
```

替换后（为空：把替换前那几行整段删掉）：


杀手（原型实测，1 个叶子用例）：`TestAdnNotFound/no_providers`

**V55** `internal/repl/addname.go`：别名按字节比（W1 的 C106）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
(m.Alias != "" && strings.EqualFold(name, m.Alias))
```

替换后：

```go
(m.Alias != "" && name == m.Alias)
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnOtherProviders/claude_alias_fold`

**V56** `internal/repl/addname.go`：不带强度的 ID 形式按字节比（W1 的 C107）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return "", strings.EqualFold(name, m.SuggestedID(""))
```

替换后：

```go
	return "", name == m.SuggestedID("")
```

杀手（原型实测，1 个叶子用例）：`TestAdnOtherProviders/haiku_fold`

**V57** `internal/repl/addname.go`：两个词的写法只看对上的第一行支持的强度（W1 的 C104）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if hasEffort(mt.row.m, e) {
			return true
		}
```

替换后：

```go
		if hasEffort(mt.row.m, e) {
			return true
		}
		break
```

杀手（原型实测，1 个叶子用例）：`TestAdnAmbiguous/union_second_row`

**V58** `internal/repl/addname.go`：第二段是敲错的强度时不按一个名字处理（裁决 (a) 整支去掉）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if effortOfAny(ms, segs[1]) || typo {
```

替换后：

```go
		if effortOfAny(ms, segs[1]) || typo && false {
```

杀手（原型实测，5 个叶子用例）：`TestAdnAmbiguous/typo_second_row`、`TestAdnTwoWords/ambiguous_typo`、`TestAdnTwoWords/id_then_typo`、`TestAdnTwoWords/typo`、`TestAdnTwoWords/upper`

**V59** `internal/repl/addname.go`：裁决 (a) 不看第二段在快照里是不是一个模型。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
 && len(matchDiscoveredName(r.modelSnap, segs[1])) == 0
```

替换后（为空：把替换前那一段删掉）：


（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnTwoWords/second_is_model`

**V60** `internal/repl/addname.go`：裁决 (a) 不要求对上的行有强度可选（grok 这类也打强度那一句）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 37.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
anyRowHasEfforts(ms) && len(
```

替换后：

```go
len(
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnTwoWords/no_efforts`

**V61** `internal/repl/addname.go`：裁决 (a) 也给配置里前缀命中的第一段。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 33.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
typo := miss(0) && anyRowHasEfforts
```

替换后：

```go
typo := anyRowHasEfforts
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnTwoWordsConfigPrefix`

**V62** `internal/repl/addname.go`：两个词的写法不认配置里只是前缀命中的第一段（裁决 (b) 整支去掉）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
(miss(0) || configPrefixOnly(segs[0], ids[0]))
```

替换后：

```go
miss(0)
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，2 个叶子用例）：`TestAdnAmbiguous/union_second_row`、`TestAdnTwoWordsConfigPrefix`

**V63** `internal/repl/addname.go`：configPrefixOnly 把全名与编号命中也算成前缀命中。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return id != "" && id != seg && !isASCIIDigits(seg)
```

替换后：

```go
	return id != ""
```

杀手（原型实测，2 个叶子用例）：`TestAdnFlags`、`TestAdnTwoWords/config_first`

**V64** `internal/repl/addname.go`：有强度的行不认不带强度的 ID 形式（退回 v1，裁决 (d)）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 33.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return "", strings.EqualFold(name, m.SuggestedID(""))
```

替换后：

```go
	return "", len(m.Efforts) == 0 && strings.EqualFold(name, m.SuggestedID(""))
```

杀手（原型实测，2 个叶子用例）：`TestAdnOtherProviders/claude_base`、`TestAdnOtherProviders/claude_base_second`

**V65** `internal/repl/addname.go`：带参数的写法里名字是 + 加数字时不打 +编号 的用法。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if strings.HasPrefix(name, "+") && isASCIIDigits(name[1:]) {
```

替换后：

```go
		if false {
```

杀手（原型实测，1 个叶子用例）：`TestAdnFlags`

**V66** `internal/repl/commands.go`：splitAddArgs 不按全角空格切。 包 `./internal/repl`（整包，不带 -run）；原型实测 77.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
'\f', ideographicSpace, ','
```

替换后：

```go
'\f', ','
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，2 个叶子用例）：`TestAdnIdeographicSpace/off_config`、`TestAdnIdeographicSpace/on_two_words`

**V67** `internal/repl/addname.go`：只看对上的第一行有没有强度可选（W1b 的 V101，anyRowHasEfforts）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if len(mt.row.m.Efforts) > 0 {
			return true
		}
```

替换后：

```go
		if len(mt.row.m.Efforts) > 0 {
			return true
		}
		break
```

杀手（原型实测，1 个叶子用例）：`TestAdnAmbiguous/typo_second_row`

**V68** `internal/repl/addname.go`：带参数的写法里名字以 + 开头就打 +编号 的用法，不看后面是不是数字（W1b 的 V102）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
strings.HasPrefix(name, "+") && isASCIIDigits(name[1:])
```

替换后：

```go
strings.HasPrefix(name, "+")
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnFlags/plus_letters`

**V69** `internal/repl/commands.go`：开关打开时跳过总结成员前缀那两句（W1b 的 V103）。 包 `./internal/repl`（整包，不带 -run）；原型实测 79.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
sumID != "" && !active[sumID] && strings.HasPrefix(sumID, seg)
```

替换后：

```go
sumID != "" && !r.addByName && !active[sumID] && strings.HasPrefix(sumID, seg)
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，2 个叶子用例）：`TestAdnPriority/summary_prefix`、`TestAdnTypoIsReference/summary_prefix`

**V70** `internal/repl/addname.go`：idFormEffort 先比不带强度的 SuggestedID("")、再逐档比（W1b 的 V106）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 37.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	for _, e := range m.Efforts {
		if strings.EqualFold(name, m.SuggestedID(e)) {
			return e, true
		}
	}
	return "", strings.EqualFold(name, m.SuggestedID(""))
```

替换后：

```go
	if strings.EqualFold(name, m.SuggestedID("")) {
		return "", true
	}
	for _, e := range m.Efforts {
		if strings.EqualFold(name, m.SuggestedID(e)) {
			return e, true
		}
	}
	return "", false
```

等价（原型实测 0 个用例变红）：claude、codex 的有强度的行，`SuggestedID(档)` 总比 `SuggestedID("")` 多一段「-<档>」，同一行的两种 ID 不会相等，先比哪一种结果都一样；没有强度可选的行循环一次都不走，两种写法相同。grok、agy 那一半要等价，还需要它们的行 `Efforts` 永远为空（它们的 `SuggestedID` 不看强度，行里若有档，两种 ID 会相等、先比哪一种就决定带不带强度）[推断：按 provider 包的解析器读出来的，本刀没有逐家实测]。只登记、不要求杀手（W1b 的 V106）。

**V71** `internal/repl/addname_existing.go`：D17 去掉「S 里有在场的」那一支（裁决书用例 1）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	case len(present) > 0 && len(m.Efforts) > 0:
		return true, r.printf(msgAddNamePresentEffort, name, strings.Join(present, ", "), name)
	case len(present) > 0:
		return true, r.printf(msgAddNamePresent, name, strings.Join(present, ", "))
```

替换后（为空：把替换前那几行整段删掉）：


杀手（原型实测，7 个叶子用例）：`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnExistingMembers/repeat`、`TestAdnPriority`

**V72** `internal/repl/addname_existing.go`：D17 的 S 按 id 前缀判，不按 provider 与 model（裁决书用例 2，反驳者的探针）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 42.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	for _, id := range sameModelAgents(r.cfg, m) {
```

替换后：

```go
	for _, id := range sortedKeys(r.cfg.Agents) {
		if !strings.HasPrefix(id, name) {
			continue
		}
```

杀手（原型实测，10 个叶子用例）：`TestAdnExistingMembers/alias_config_present`、`TestAdnExistingMembers/alias_present`、`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnExistingMembers/prefix_other_model`、`TestAdnExistingMembers/present_among_absent`、`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/several_absent_grok`、`TestAdnOtherProviders/alias_configured`

**V73** `internal/repl/addname_existing.go`：sameModelAgents 不认 model 写成别名的成员（裁决书用例 3）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 42.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
a.Provider == m.Provider && (a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias))
```

替换后：

```go
a.Provider == m.Provider && a.Model == m.Model
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，2 个叶子用例）：`TestAdnExistingMembers/alias_config_present`、`TestAdnOtherProviders/alias_configured`

**V74** `internal/repl/addname_existing.go`：D17 的 S 全不在场、不止一个时加入第一个（裁决书用例 4）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.1 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	case len(s) == 1:
```

替换后：

```go
	case len(s) >= 1:
```

杀手（原型实测，2 个叶子用例）：`TestAdnExistingMembers/several_absent`、`TestAdnExistingMembers/several_absent_grok`

**V75** `internal/repl/addname_existing.go`：D17 的 S 全不在场、恰好一个时不加入（落到列出那一句，裁决书用例 5）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 38.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	case len(s) == 1:
		return true, joinConfiguredAgent(ctx, r, s[0])
```

替换后（为空：把替换前那几行整段删掉）：


杀手（原型实测，3 个叶子用例）：`TestAdnExistingMembers/one_absent_config`、`TestAdnExistingMembers/one_absent_renamed`、`TestAdnOtherProviders/alias_configured`

**V76** `internal/repl/addname.go`：带参数的写法里名字在配置里只是前缀命中时照旧打「对上的是配置里的 agent」，不按识别结果加（裁决书用例 6，原 §7 (r)）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 38.0 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
			if !configPrefixOnly(name, id) {
```

替换后：

```go
			if true {
```

杀手（原型实测，4 个叶子用例）：`TestAdnFlagsOverConfigPrefix/effort_over_other_model`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`、`TestAdnFlagsOverConfigPrefix/prefix_no_match`

**V77** `internal/repl/addname.go`：只有 --id 时也过 D17 那道门（裁决书用例 7）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.2 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
second == "" && !a.haveEffort && !a.haveID && !(mt.byID && mt.effort != "")
```

替换后：

```go
second == "" && !a.haveEffort && !(mt.byID && mt.effort != "")
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，2 个叶子用例）：`TestAdnFlagsOverConfigPrefix/id_only_present`、`TestAdnFlagsOverConfigPrefix/id_over_prefix`

**V78** `internal/repl/addname_existing.go`：D17 的 S 不去掉总结成员（/add GROK-4.7 会把总结成员当普通成员加入）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 37.3 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if id == r.cfg.Summary.Agent {
			continue
		}
```

替换后（为空：把替换前那几行整段删掉）：


杀手（原型实测，1 个叶子用例）：`TestAdnExistingMembers/summary_excluded`

**V79** `internal/repl/addname_existing.go`：D17 在场的那一句一律带「要另加一档强度」（grok 这类没有强度的行也带）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 37.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	case len(present) > 0 && len(m.Efforts) > 0:
```

替换后：

```go
	case len(present) > 0:
```

杀手（原型实测，2 个叶子用例）：`TestAdnExistingMembers/present_among_absent_grok`、`TestAdnPriority`

**V80** `internal/repl/addname_existing.go`：D17 列出的那一句一律不提强度（有强度可选的行也不提）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	case len(m.Efforts) > 0:
```

替换后：

```go
	case false:
```

杀手（原型实测，1 个叶子用例）：`TestAdnExistingMembers/several_absent`

**V81** `internal/repl/addname.go`：给了 --effort 也过 D17 那道门。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.8 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
second == "" && !a.haveEffort && !a.haveID
```

替换后：

```go
second == "" && !a.haveID
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，3 个叶子用例）：`TestAdnExistingMembers/one_absent_config`、`TestAdnFlagsOverConfigPrefix/effort_over_prefix`、`TestAdnOtherProviders/alias_same_brain`

**V82** `internal/repl/addname.go`：两个词的写法（有第二个词）也过 D17 那道门。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.8 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
if second == "" && !a.haveEffort
```

替换后：

```go
if !a.haveEffort
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，2 个叶子用例）：`TestAdnExistingMembers/several_absent`、`TestAdnTwoWordsConfigPrefix`

**V83** `internal/repl/addname.go`：名字是带强度的 ID 时也过 D17 那道门。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.4 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
 && !(mt.byID && mt.effort != "")
```

替换后（为空：把替换前那一段删掉）：


（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，2 个叶子用例）：`TestAdnConfiguredRow/other_effort`、`TestAdnConfiguredRow/same_brain`

**V84** `internal/repl/addname.go`：带参数的写法里名字只是前缀命中、快照里又对不上时不打「对上的是配置里的 agent」（落到都没有那一句）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.6 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if prefixID != "" && len(matchDiscoveredName(r.modelSnap, name)) == 0 {
```

替换后：

```go
		if false {
```

杀手（原型实测，1 个叶子用例）：`TestAdnFlagsOverConfigPrefix/prefix_no_match`

**V85** `internal/repl/addname.go`：敲错的强度那一支不看第二段是不是在场成员或总结成员的前缀（W1b minor 1）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 34.9 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
 && addSegmentNotFound(r, segs[1], active, member) == fmt.Sprintf(msgAddNameNotFound, segs[1])
```

替换后（为空：把替换前那一段删掉）：


（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，3 个叶子用例）：`TestAdnTypoIsReference/id_present_prefix`、`TestAdnTypoIsReference/present_prefix`、`TestAdnTypoIsReference/summary_prefix`

**V86** `internal/repl/addname.go`：敲错的强度那一支不排除 + 开头的第二段（W1b minor 1）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 33.8 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
typo && !strings.HasPrefix(segs[1], "+") && addSegmentNotFound
```

替换后：

```go
typo && addSegmentNotFound
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnTypoIsReference/plus`

**V87** `internal/repl/addname_existing.go`：D17 在场的那一句（有强度可选的行）列出整个 S，不只在场的（W1c 的 W1）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 38.8 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
msgAddNamePresentEffort, name, strings.Join(present, ", ")
```

替换后：

```go
msgAddNamePresentEffort, name, strings.Join(s, ", ")
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnExistingMembers/present_among_absent`

**V88** `internal/repl/addname_existing.go`：D17 在场的那一句（没有强度的行）列出整个 S，不只在场的（W1c 的 W2）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 38.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
msgAddNamePresent, name, strings.Join(present, ", ")
```

替换后：

```go
msgAddNamePresent, name, strings.Join(s, ", ")
```

（这一条的替换前后是一行里的一段，不含换行；代码块末尾的换行是排版加的。）

杀手（原型实测，1 个叶子用例）：`TestAdnExistingMembers/present_among_absent_grok`

**V89** `internal/repl/addname_existing.go`：joinConfiguredAgent 写进会话成员记录的显示名是空串（W1c 的 W3）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 38.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		DisplayName: ac.DisplayName,
```

替换后：

```go
		DisplayName: "",
```

杀手（原型实测，1 个叶子用例）：`TestAdnExistingMembers/one_absent_config`

**V90** `internal/repl/addname_existing.go`：joinConfiguredAgent 写进会话成员记录的 provider 是空串（W1c 的 W4）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 35.7 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		Provider:    ac.Provider,
```

替换后：

```go
		Provider:    "",
```

杀手（原型实测，1 个叶子用例）：`TestAdnExistingMembers/one_absent_config`

**V91** `internal/repl/addname_existing.go`：sameModelAgents 不比 provider（另一家 model 字符串相同的成员也进 S，W1c 的 W5）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
		if a.Provider == m.Provider && (a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias)) {
```

替换后：

```go
		if a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias) {
```

杀手（原型实测，1 个叶子用例）：`TestAdnExistingMembers/other_provider_same_model`

**V92** `internal/repl/addname.go`：configPrefixOnly 去掉 `!isASCIIDigits(seg)`，编号命中也算前缀命中（W1c 的 W7）。 包 `./internal/repl`、`-run 'TestAdn|TestMdx|TestCxh|TestObz|TestObx|TestUx2r|TestChx'`；原型实测 36.5 s，还原后 SHA256 一致，四项核对干净。

替换前：

```go
	return id != "" && id != seg && !isASCIIDigits(seg)
```

替换后：

```go
	return id != "" && id != seg
```

杀手（原型实测，1 个叶子用例）：`TestAdnFlags`

**判定为等价、不列杀手的变异**：V7、V70（见上）。

**只在审查时看**：A1-A8、B1、B3、B4 里的注释行；`addname.go`、`addname_existing.go` 的文件头与各函数的注释；`ensureModelSnapshot`、`addDiscoveredModel` 的头注释（注释对账表逐条核）。另有几条行为等价或没有用例担保的：
- （v1.2）V72（S 按 id 前缀判）杀不到 N22 repeat：那里在场的 `gpt-6.1-sol-xhigh` 恰好以敲的名字开头，按 id 前缀也收得到它。V72 由 prefix_other_model（多收了别的模型）与别名、改过名的几条（漏收）担保。
- （v1.2 写的「`sameModelAgents` 去掉 provider 那一半没有夹具」，v1.3 已有用例：N22 other_provider_same_model，V91。）
- （v1.2）`joinConfiguredAgent` 的 Join 失败那一支（「加入失败」）没有用例，与 §7 (o) 同一类（v1.3 裁决书：留在这里）。
- （v1.3，W1c 的 W6）`addNameExisting` 只在总结成员不在场时才把它从 S 里去掉（`if id == r.cfg.Summary.Agent && !active[id]`）：没有用例杀得到（W1c 实测 0 个用例变红，本刀没有重跑）。不补的理由（裁决书）：这个变异只改说法、从不写配置。按代码读 [推断，未实跑]：它只在总结成员在场时起作用，那时总结成员进了 S，走「有在场的就拒绝、点名」那一支，打一句拒绝、不写、不加入；总结成员不在场时照旧去掉，与现在相同。所以它不会多写配置，也不会把总结成员当普通成员加入。
- （v1.2）D17 里「S 为空」与「有在场的」的先后：S 为空时 present 也为空，两种排法结果相同。
- （v1 的第一条，`idFormEffort` 在有强度的行上也比 `SuggestedID("")`，v1.1 按裁决 (d) 改成了功能，由 V64 与 N15 claude_base、claude_base_second 担保，不再列在这里。）
- `matchDiscoveredName` 按快照倒序或按 provider 分组返回：歧义时候选行的顺序会变（N6 的期望按快照顺序，会红）；只对上一行时结果不变。
- `effortOfAny` 的「几行取并集」（裁决书 m5）：「只看第一行」（V57）只在第一段对上多行时起作用。第一段**没命中配置**时，对上多行、其中有一行有强度、第二段在快照里又对不上模型，那么第二段认不认作强度都交给 `addByOneName`，都打歧义候选（裁决 (a)），看不出差别；只有第二段恰好又是快照里的一个模型名时才看得出来，那种夹具太造作，没写。V57 由第一段**在配置里只是前缀命中**的那一支担保（N6 union_second_row：那一支没有 (a)，认不出强度就落到多段的说法）。
- （v1.3 订正）`configPrefixOnly` 里 `!isASCIIDigits(seg)` 那一半：v1.2 这里写「去掉这一半只在某个模型名或 ID 恰好全是数字时看得出来」，v1.2 起已经不对——带参数的写法（D9）也调 `configPrefixOnly`，去掉之后 `/add 1 --effort high` 这类编号命中会被当成前缀命中、先识别（W1c 的 W7）。v1.3 登记为 V92，杀手是 N14 表里新加的 `/add 1 --effort high`（父测试 `TestAdnFlags`）。两个词写法那一处（第一段是编号、第二段是一档强度、第一段又在快照里对上模型）仍没有用例：`/add 1 high` 在快照里对不上任何模型，走不到这一支。

**MD1、CXH、OB3 的变异在本刀下的状态**：MD1、CXH 里锚点落在 `cmdAddDiscovered` 从强度起那一段（强度、同脑子、id、守卫、追加、加载校验、换指针、Join）的变异，锚点文本原样搬进 `addDiscoveredModel`，文件不变（`addmodel.go`），行号后移；V44、V47、V48 是这一段在本刀的代表，原型实测杀手里照样是 MD1、CXH 的用例。锚点落在 `printDiscoveredModels` 里「没有快照就识别」那几行的（例如 16a 的 R14 取消、19f 的 Q 系列里取消那几条）现在在 `ensureModelSnapshot` 里，行首缩进少了一级（原来在 `if r.modelSnap == nil {` 里），原样重跑要把替换前后的缩进各减一个 tab；V42、V43 是它们在本刀的代表。锚点落在 `resolveAddSegment` 末尾三句的（ux2r、CH1、OB1 的几条），现在在 `addSegmentNotFound` 里，`return "", fmt.Sprintf(` 变成了 `return fmt.Sprintf(`；V40、V41 是代表。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许动这些文件：**块 A**：补丁 `internal/repl/repl.go`（A1）、`internal/repl/addmodel.go`（A2-A7）、`internal/repl/commands.go`（A8，只有 `splitAddArgs` 那一处），新文件 `internal/repl/addname_existing.go`（A9，v1.2）；**块 B-甲**：补丁 `internal/repl/commands.go`（B1-B4）；**块 B-乙**：新文件 `internal/repl/addname.go`（B5）；**块 C**：补丁 `internal/app/app.go`（C1、C2）。（v1.3 把 v1.2 的块 B 拆成块 B-甲与块 B-乙，代码不变。）不改 `go.mod`，不改任何既有 `_test.go`，不新写任何 `adn` 测试文件（那是测试腿的事）。
- 行数：块 A 替换文本合计 110 行加新文件 92 行（202 行）；块 B-甲替换文本合计 64 行；块 B-乙新文件 355 行；块 C 合计 6 行。四块都在 400 行以内。按块 A、块 B-甲、块 B-乙、块 C 的顺序交；四块可以分几次交（每次一块或几块），也可以一次交完。落地时可以只落块 A 先跑；块 B-甲与块 B-乙互相引用，要一起落地才编译得过，块 C 在它们之后（D16）。
- 整文件用 `===FILE: <路径>===` 换行接完整内容、末尾 `===END FILE===`。补丁块：

  ```
  ===PATCH: internal/repl/commands.go===
  <<<<ANCHOR
  （从 HEAD 64c4b49 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```

  锚点照 §1.1 各处的 `anchor` 代码块（本文代码块里的缩进是 tab；若你看到的是空格，以真文件为准）；同一文件的多个锚点互不重叠（A2-A7 在 `addmodel.go` 里按文件顺序排，B1-B4 在 `commands.go` 里同样；A8 也在 `commands.go`，位于 B2 与 B3 之间，与 B1-B4 互不重叠，先落哪个都行）。**每个 `===FILE:`、`===PATCH:` 标记都单独起一行、从行首开始**（OB2 的块 A 把标记接在开场白末尾，按行首解析的落地脚本漏了文件）。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- A7 是「把 `m := row.m` 那三行换成 return、新函数的头注释与签名」，锚点以下的函数体（从 `switch {` 之后到函数结尾）**一个字节都不动**，它们原地成为 `addDiscoveredModel` 的函数体。B4 同理：`addSegmentNotFound` 里的四个 `fmt.Sprintf` 字面量与 HEAD 逐字节相同。
- 代码必须能 `gofmt`（tab 缩进）；`go vet` 不许有新报告。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（全角括号、冒号、分号、逗号、顿号、直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的用户可见字符串只有 §3 的六句与候选行（v1.1 没有新增）与 v1.2 的四句（A9）。A8 的全角空格写成数字常量 `0x3000`，不写字面字符、不写反斜杠 u 转义。
- 生产代码新标识符不许以 `adn` 开头。§1.1 给出的签名、判据、代码、注释照抄；本刀没有「名字实现方定」处。
- 回复末尾必须有：

  ```
  ===REPORT===
  块：给了哪些文件（块 A、块 B-甲、块 B-乙、块 C 各自）。
  补全性：A1-A9、B1-B5、C1、C2 各落在哪个 FILE / PATCH 块。
  锚点：每个锚点是否从 HEAD 64c4b49 真文件逐字复制、在该文件里恰好一次。
  字面量：§3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  ```

---

## 6. 落地后主代理要做的

1. **收割与逐字核对**：越界扫描（主仓库、`internal/*`、`cmd/*`、中转根有没有实现腿写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；标记不在行首的也要找。逐字核对：用脚本从本文取出每一对 `anchor` / `replace` 与每个 `file` 代码块，打在 HEAD `64c4b49` 的一份 `git archive` 上，结果应当与落盘后的 8 个文件（`repl.go`、`addmodel.go`、`commands.go`、`addname.go`、`addname_existing.go`、`app.go`，外加测试腿的两个 `adn` 文件）逐字节相同（起草时就是这样核的：v1 是 13 对锚点、v1.1 是 14 对锚点，各恰好一次，3 个整文件；v1.2 是 14 对锚点、4 个整文件；v1.3 相同，只是块 B 拆成两块）。`gofmt -l ./internal ./cmd`；`go vet ./internal/app ./internal/config ./internal/repl ./internal/provider ./internal/turn`；`go build ./...`。`internal/provider`、`internal/turn`、`internal/config`、`internal/tui`、`internal/store`、`internal/conversation`、`cmd/council` 零改动。
   - **旧句只剩一处的 grep 核对**：`internal/repl` 的非测试 `.go` 文件里搜 `不带参数的 /add 能列出可加的`，命中只许是 `addSegmentNotFound` 的那一行；搜 `msgAddNameNotFound`，命中恰好 4 行：`addname.go` 的定义、定义上面那一行文档注释、`commands.go` 里 `addSegmentNotFound` 的引用，以及 v1.2 `addname.go` 里敲错的强度那一条判据（拿 `addSegmentNotFound` 的返回值与它比，D7；只比、不打）（v1.2 原型实测：`addname.go:36` 注释、`addname.go:37` 定义、`addname.go:194` 判据、`commands.go:1167` 引用；行号以落地后为准）。多出任何一处就是另留了一份「没找到」的说法。
2. **合并测试腿的 2 个文件**，先跑 `go test -count=1 ./internal/repl ./internal/app`，再跑全量 `go test -count=1 ./...`（原型九包全绿）；`-run 'TestAdn'`（repl、app）跑一次 `-count=3`。**每一次运行都照 §4 表头的运行纪律**，跑完核四项与残留进程（没有开跑之后新起、还活着的 `cmd`、`node`、`go`、`*.test`、`grok`、`codex`、`claude`、`agy`、`council` 进程；按命令行排除宿主自己的 shell 启动器）。
3. **§4 的 92 条变异逐条跑**（v1.3；v1.2 是 86 条），照 §4 表头的纪律，从字节副本还原、SHA256 核对；点名杀手里的每个子测试都要单独确认变红。`addname.go` 与 `addname_existing.go` 的 79 条（`-run` 里带了 chx 的慢用例）v1.1 单跑实测每条约 33 s，v1.2 三路并行时每条 36-45 s，v1.3 三路并行时 33.4-42.7 s；整包 repl 的 11 条（`commands.go`、`addmodel.go`）v1.1 单跑约 75 s，v1.2 并行时 81-89 s，v1.3 并行时 76.7-88.0 s；app 的两条 4-15 s。v1.2 原型上三份工作副本并行跑，三批共约 22 分钟（§4 v1.2 那一段）；v1.3 同样三路并行，分三次调用跑完；串行估约 60 分钟 [估算，没有串行跑过]。PowerShell 单次工具调用上限 10 分钟，按每批 8 分钟以内分批跑。
4. **文档**：
   - `docs/00_DECISIONS.md` 新立 **#49**，**与代码在同一个提交里立**（由主代理在落地时写；代码与测试的文件头都引用 #49，分开提交会让中间那个提交引用一条不存在的决策）：记 Griffin 2026-09-30 原话（「我需要能/add 加入订阅的，不是说只有我们事先选好的那种，你懂吧」）与他敲被挡回的几条命令（§0 第 3 条）；主代理裁定（任务书 1-11 条）与本文 D1-D16；落地 commit；§7 的待定项。#45「待 Griffin」里「已配置的 claude 行不编号（换强度手改配置）」与 #46 裁定里同一条，记一句「换强度现在用 /add <ID>（AD1，#49）」。
   - `docs/16a_MD1_MODEL_DISCOVERY_CONTRACT.md` 文件头加一条：open item (c) 已由 AD1 解决（不编号，按名字加；+编号 不变）。
   - `docs/README.md` 的 `/add` 那一节：识别表 ID 列里的名字可以直接 `/add <ID>`（或模型名加 `--effort`、或两个词 `/add <模型名> <强度>`，强度与名字之间的空格可以是全角空格），没有快照时先自动识别；强度敲错时列出这个模型支持的档；一次加一个；`/leave` 撤回、备份在哪。
   - 本契约归档到 `docs/`（照 19d-19f 的体例在文件头加落地后订正区）。
   - `docs/14_MVP_CHECKLIST.md` 的 3.5 节加一项 **M7「`/add` 按名字加识别出的模型（AD1）」**，步骤照下面第 5 步。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的真机复验步骤**（E 盘机，零花费：只做识别，与不带参数的 `/add` 同一套，不提问）。用你自己的真配置，**会写配置、写前自动备份**。下面每一条命令都对着命令表（`commands.go:147-174`）与启动参数（`cmd/council/main.go:42-44`）核过：`-version`、`-no-tui`、`/new`、`/add`、`/who`、`/leave`、`/quit` 都在。
   0. 先确认装的是带本刀的版本（主代理照惯例等 Griffin 说「装」再重装）：

      ```powershell
      council -version
      ```

   1. **只敲斜杠命令，不要直接打字**：对话模式默认打开，直接打字会让会话里的成员回复（要花钱）。
   2. 照常开 council（TUI 或 `council -no-tui` 都行），开一个新会话：

      ```text
      /new ad1check
      ```

   3. **第一次就敲（本会话还没有识别过，要自动识别）**：

      ```text
      /add gpt-6.1-sol-xhigh
      ```

      预期三行：

      ```text
      正在识别模型: agy, claude, codex, grok
      已把 agent gpt-6.1-sol-xhigh 追加进 <你的配置文件>（原文件备份在 <你的配置文件>.bak-add-<日期>-<时间>）
      gpt-6.1-sol-xhigh 已加入会话 <会话 id>
      ```

      - 「正在识别模型」后面以你配置里写了的 provider 为准（字典序）；识别要几秒到十几秒（#45 的真机原件约 10 秒）。
      - 配置文件默认是 `C:\Users\25142\.council\config.toml`（以那一行实际打出来的为准）。**把那一行里的备份路径记下来**，第 5 步要用。
      - codex 的 `gpt-6.1-sol` 有 `xhigh` 这一档：主代理 2026-09-30 在 E 盘机上跑 `codex debug models` 实测，它支持 low、medium、high、xhigh、max、ultra（§0 第 2 条）。
      - 你的配置里若已经有同一个模型、同一档的成员（别的 id），打的是「配置里已经有同一个模型、同一强度的 agent <id>，直接 /add <id>」，什么都不写。
   4. 看一眼成员，再撤回（`/leave` 只是退出这个会话，配置里那一段还在）：

      ```text
      /who
      /leave gpt-6.1-sol-xhigh
      ```

      预期最后一行是「gpt-6.1-sol-xhigh 已退出会话 <会话 id>」。

      v1.2 的「敲两次」（D17，零花费，不写配置）：接着敲两次同一条，再撤回：

      ```text
      /add gpt-6.1-sol
      /add gpt-6.1-sol
      /leave gpt-6.1-sol-xhigh
      ```

      预期：第一条打「gpt-6.1-sol-xhigh 已加入会话 <会话 id>」（配置里按前缀对上它，不识别、不写）；第二条打「gpt-6.1-sol 对上的模型已经有成员在当前会话里：gpt-6.1-sol-xhigh；要另加一档强度，用 /add gpt-6.1-sol <强度> 或 --effort <强度>」，什么都不写；`/leave` 那一行同上。原型实跑的形状（N22 repeat），真机未跑。你的配置里若另有 `gpt-6.1-sol` 开头的成员，第一条可能对上的是它、或打「有歧义」，以屏幕为准。
   5. **要把配置也撤回**：先 `/quit`，再在 PowerShell 里用第 3 步记下的备份覆盖回去（备份就是追加之前的原文件，逐字节相同）：

      ```powershell
      Copy-Item -LiteralPath '<第 3 步那一行里的备份路径>' -Destination '<第 3 步那一行里的配置文件路径>'
      ```

      不撤回也行：那一段就是一个普通成员，以后 `/add gpt-6.1-sol-xhigh` 直接加入（配置里的全名，不再识别）。
   6. **两个词的写法**，要在「配置里还没有这个模型的成员」时试（第 5 步撤回之后）。重开 council（新进程才是新快照），新会话里第一次就敲：

      ```text
      /new ad1check2
      /add gpt-6.1-sol xigh
      /add gpt-6.1-sol xhigh
      ```

      预期：`/add gpt-6.1-sol xigh`（敲错的强度，裁决 (a)）先打「正在识别模型: …」，再打一行「gpt-6.1-sol 支持的强度是 low|medium|high|xhigh|max|ultra，没有 xigh」，什么都不写（档的顺序照主代理实测的 `supported_reasoning_levels` 推 [推断]，以屏幕为准）。`/add gpt-6.1-sol xhigh` 打追加与加入两行（快照已经拍过，不再有「正在识别模型」），写进与第 3 步同样的一段。
      - 没撤回（配置里还有不在场的 `gpt-6.1-sol-xhigh`）就敲这两条：`xigh` 那一条只打「配置里和识别结果里都没有 xigh；……」（第一段在配置里按前缀命中，敲错的强度那一支只给配置里没命中的第一段，D7）；`xhigh` 那一条打「配置里已经有同一个模型、同一强度的 agent gpt-6.1-sol-xhigh，直接 /add gpt-6.1-sol-xhigh」，什么都不写（裁决 (b)，D7，N16）。两条都是原型实跑的形状（N16），真机未跑。
      - 不带强度的 `/add gpt-6.1-sol`（配置里还没有这个模型的成员时）加的是 `gpt-6.1-sol-high`：council 的默认档是 high，不是 codex 的 medium，与 +编号 相同（§0 第 2 条）。
   7. 撤回同第 4、5 步；`/quit` 退出。第 2、6 步建的空会话留在库里，不影响别的。
   - D 盘机照同样的步骤做，[未验证]。

---

## 7. open items

- (a) **Griffin 真敲的第一条是哪一种**（v1.1 已按裁决改）：任务书一处写 `/add gpt-6.1-sol xigh`、一处写 `/add gpt-6.1-sol xhigh`。v1 里 `xigh` 打「一次加一个」与「都没有 xigh」两行；v1.1 按裁决 (a) 改成只打一行现成的「gpt-6.1-sol 支持的强度是 <档|档>，没有 xigh」、不写配置（D7，N3 typo）。取舍（裁决书没写到、本文定的）：只给有强度可选的行（grok 这类走过去会打「不能带 --effort」，用户没敲 `--effort`，那句是假话；所以 N3 no_efforts 照旧按多段）；只给配置里没命中的第一段（前缀命中的照旧按多段，N16 `xigh`、`XHIGH`）；第一段是带强度的 ID 形式时打的是 D10 的「已经带着强度 xhigh，不能再给强度 xigh」而不是强度那一句（N3 id_then_typo），两句都成立，后者更准。
- (b) **模型加进配置之后，模型名形式会先在配置里按前缀对上**（v1.1 已按裁决改两个词那一支）：配置里有 `gpt-6.1-sol-xhigh`（不在场）时，单段 `/add gpt-6.1-sol` 仍按前缀加入它（裁决：不变，用户没说强度时加入已有的成员比悄悄写配置更安全）；两个词 `/add gpt-6.1-sol xhigh` 打同脑子那一句、不写，`/add gpt-6.1-sol high` 新建 `gpt-6.1-sol-high`（D7，N16）。~~在场时不受影响（在场成员不是候选）。带参数的那一支没改，见 (r)。~~ v1.2 订正：「在场时不受影响」只对 (b) 那一支（两个词、配置前缀命中）成立，读起来却像在场时行为没变，这是 W1b 的 major 指出的：v1.1 里这个成员在场时，单段 `/add gpt-6.1-sol` 在配置里没命中（在场成员不是候选），到快照里对上后按默认档新建 `gpt-6.1-sol-high`、写配置，同一条命令敲两次结果不同。v1.2 由 D17 拒绝、不写（N22 repeat）。带参数的那一支 v1.2 已改（D9）。
- (c) **同脑子那一句可能指向在场成员**（N9）：`grok-4.6-a` 在场时 `/add grok-4.6` 打「配置里已经有同一个模型、同一强度的 agent grok-4.6-a，直接 /add grok-4.6-a」，照做是一次 upsert，无害；HEAD 在这里说的是「开头的都已经在当前会话里了」。这是 D6 的顺序（任务书第 6 条）带来的。（v1.2 订正：没给强度的这一条现在先过 D17，打「grok-4.6 对上的模型已经有成员在当前会话里：grok-4.6-a」，N9 的期望照裁决书授权改了。同脑子那一句仍可能指向在场成员，但只在给了明确的强度或只有 `--id` 时：例如 `grok-4.6-a` 在场时 `/add grok-4.6 --id grok-4.6-b` 打同脑子那一句、指向在场的 `grok-4.6-a`、不写配置（v1.2 原型上一次性探针实跑，探针跑完即删）。grok 这类没有强度的行因此只能靠配置里还没有这个模型时 `--id` 起名，第二个同模型的成员要手改配置，与 v1.1 相同。）
- (d) **有强度的行，不带强度的 ID 段**（v1.1 已按裁决改）：claude 的 `/add sonnet-5`（表里 ID 是 `sonnet-5-high`、Model 是 `claude-sonnet-5`、Alias 是 `sonnet`）v1 打「都没有」；v1.1 的 `idFormEffort` 在逐档比完之后再比 `SuggestedID("")`，对上时强度按 `--effort`、第二个词或 `DefaultEffort()` 取（D3，N15 claude_base 加 `sonnet-5-high`、claude_base_second 加 `sonnet-5-xhigh`，V64）。副作用：codex 的模型名（`SuggestedID("")` 等于 `Model` 的那些）现在按不带强度的 ID 形式对上，强度同样另取，结果不变；模型名形式里 `Model` 那一支只剩 claude 与规整过的 codex slug 走得到，另加 N15 claude_model、claude_model_fold 担保（v1.1 原型上先跑的一批里 V8 存活，V2 只剩 claude_alias_fold 一条杀手）。
- (e) **`/help` 没改**（D14）。要改就授权改 `cxh_fixture_test.go:223` 与 `ux2r_add_test.go:448` 两处期望。一版可用的 summary（起草人拟，未实跑）：「把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；/add <识别表里的 ID> 或 /add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别」。
- (f) **按名字加自动识别时不打失败行**（D5）：某一家识别失败时，它的模型对不上，打的仍是「都没有」那一句（任务书第 8 条：失败不改变这句的判断）；失败行要敲不带参数的 `/add` 才看得见（快照里留着）。要不要在「都没有」之后补一句「识别有 N 家失败，不带参数的 /add 看详情」，待定。
- (g) **配置里一家能识别的都没有时**，「都没有」那一句仍提「识别结果」与「识别表」。这种配置下识别表本来就不会出现，这半句不成立；OB1 首次运行生成的配置总有 provider，实际少见。
- (h) **快照之后从配置里删掉的 provider**（`/config reload --apply`）：它的行仍在快照里，按名字照样对得上，与 +编号 同一种暴露；写进去的成员 provider 不在 `[providers]` 里时，写后加载校验应当不过、恢复原文件 [推断，没有实跑这一种]。
- (i) **`strings.EqualFold` 不只折 ASCII**：按 Go 的 Unicode 简单大小写折叠，开尔文符号 U+212A 与 `k`、长 s U+017F 与 `s` 也算相等 [按 Go 文档，本刀未实测]。写进配置的 id 是建议的 id，不是敲的原样，所以只影响「对不对得上」，不会写出怪 id。任务书写的是「ASCII 不区分大小写（EqualFold）」，本文照括号里的函数做。
- (j) **守卫里 data_dir / rules_file 那一句**提「再用 /add +编号 加入」，按名字加走到这里也照打（D12：与 +编号 相同）。重启之后按名字加也行，这半句只是没提。
- (k) **只提 +编号 的几处注释与文字**（D15）：`configcmd.go:5-7`、`provider/codex.go:51-53`、`config/append.go:13`、`config/defaults.go:68`、`app/firstrun.go:177`、识别表下那一行。都成立，只是不全；不改。
- (l) **两个词、第一段有歧义**：第二段是对上的任意一行的一档强度（并集）就按一个名字处理、列候选（N6 two_words、union_second_row）；不是、但第一段没命中配置、对上的行里有一行有强度、第二段在快照里又对不上模型时，照裁决 (a) 同样列候选（N3 ambiguous_typo）；其余按多段（「一次加一个」）。
- (m) **`--effort` 与值用逗号粘在一起**（`/add x --effort,high`）：fields 里没有恰好等于 `--effort` 的词，不是参数形式，按多段解析（`--effort` 与 `high` 各一段）。
- (n) **真机上 `gpt-6.1-sol` 的强度档**（v1.1 关闭）：主代理 2026-09-30 实测 `codex debug models`，六档 low、medium、high、xhigh、max、ultra，默认 medium（§0 第 2 条）；§6 第 5 步的预期照这个写。本文夹具仍是合成的四档。
- (o) **Join 失败**（换指针之后）与 +编号 相同：配置已写、指针已换、不回滚，屏幕是「加入失败: …」。`addDiscoveredModel` 这一支（+编号 与按名字加共用）没有用例：`internal/repl` 的测试里打「加入失败」的只有 `ux2r_add_test.go`，钉的是 `cmdAdd` 按配置加入的那一支（grep 核过）；本刀没有另加。
- (q) **敲错配置里的名字也要先等一次识别**：开关打开时，一段在配置里没命中（例如把 `opus-5.0` 敲成 `opsu-5.0`），本会话又还没有快照，就先识别再说「都没有」。真机上四家约 10 秒（#45 原件），HEAD 是立刻报错。识别过一次之后快照留着，后面的敲错不再等。零花费。
- (p) **HEAD 注释里一处过时的行号**（D15 最后一条）：`tui/model.go:341-347` 在 HEAD 上已经不指 Ctrl-C 那一支。本刀随注释搬家改成按名字指；别的契约若还引用这组行号，同样过时。
- (r) **v1.2 已按裁决书规则一解决、移进 D9**（「v1.2（裁决书规则一，原 §7 (r)）」那一条，N23）。下面是 v1.1 的原文，留作记录：**带参数的写法同样有 (b) 那个坑，没改**（D15）：配置里有不在场的 `gpt-6.1-sol-xhigh` 时，`/add gpt-6.1-sol --effort high` 的名字在配置里按前缀命中，照 D9 打「gpt-6.1-sol 对上的是配置里的 agent gpt-6.1-sol-xhigh，--id 与 --effort 只用于加识别出的模型；直接 /add gpt-6.1-sol-xhigh」，不新建 `gpt-6.1-sol-high`。裁决书只改了两个词那一支，这一支要不要照 (b) 放开（名字在快照里精确对上、配置里只是前缀命中时按识别结果加），待定。v1.1 原型上用一次性探针实跑过（adn 的装置：先按名字加 `gpt-6.1-sol-xhigh` 再 `/leave`，然后敲这一条），逐字就是上面那一句；探针跑完即删，没有写成用例。
- (s) **(a) 与 (b) 的边界照代码写死**：敲错的强度那一支（(a)）只给配置里没命中的第一段；配置里只是前缀命中的第一段只认真强度（(b)）。所以同一个 `/add gpt-6.1-sol xigh`，配置里还没有这个模型时打强度那一句，有了（不在场）之后只打「都没有 xigh」（N3 typo 与 N16）。理由：前缀命中时第一段本来指着配置里的一个成员，把第二段当敲错的强度是多猜一步。
- (t) **全角空格只加在 `splitAddArgs`**（A8）：`args.rest` 里的其它不可见或全角字符（例如 U+00A0 不换行空格）不切。`strings.Fields` 按 `unicode.IsSpace` 切，比 `splitAddArgs` 宽，二者仍不完全对齐；裁决书只点了 U+3000。
- (u) **带参数写法里名字只认 `+` 加数字**（D9，照裁决书的字面）：`/add --id x +abc` 照旧先识别、再落到「配置里和识别结果里都没有 +abc；……」（v1.1 原型上一次性探针实跑：新装置里打「正在识别模型: codex, grok」与这一句两行；探针跑完即删）。`cmdAdd` 自己在第一个词以 `+` 开头时一律交给 +编号，不看后面是不是数字。
- (v) **裁决书对 N7 all_config_refusal 的期待与实测不同**：裁决书说它「担保多段那一支的 miss 判据（v1 的 :716）」。v1.1 把两份拷贝抽成一个 `miss` 之后分调用处实测：all_config_refusal 杀的是 anyMiss 那一处（V49）与共用判据（V51）——多段、每段都命中时根本走不到逐段那个循环；逐段循环那一处（V50，v1 的 :716 的位置）只有 N7 three 杀得到。两处都有杀手，只是分工与裁决书写的不同。
- (w) **单段、不带参数、带强度的 ID 在配置里只是前缀命中时仍按配置加入**（v1.2，本文定的边界）：配置里有不在场的 `gpt-6.1-sol-xhigh-old` 时，`/add gpt-6.1-sol-xhigh` 按前缀加入它、不识别（v1.2 原型上一次性探针实跑：「gpt-6.1-sol-xhigh-old 已加入会话 …」，执行器 0 次）。裁决书规则一把「带强度后缀的 ID」算作明确的强度，但要知道一段是不是带强度的 ID 得先识别，而规则二正是不让单段的前缀命中多等这一次（「每条 `/add op` 都得多等一次识别」）。所以规则一只在带参数（D9）与两个词（D7）两处实施。要放开，得让单段前缀命中也识别，代价是每条前缀命中都等一次识别（没有快照时）。v1.3：主代理裁定**接受，留在这里**；W1c 实测（本刀 v1.3 原型上由 N25 实跑）：前缀命中的可能是另一个模型——配置里的 `gpt-6.1-sol-xhigh-old` 是 `gpt-5.5`、`xhigh` 时，`/add gpt-6.1-sol-xhigh` 照样加入它，回显「gpt-6.1-sol-xhigh-old 已加入会话 …」。这是 HEAD 的前缀语义，有意保留：这种情形要求配置里某个 agent 的 id 以另一个完整的 ID 开头，很少见；加入之后屏幕回显的是实际加入的 id。同一条命令敲第二次时，-old 已在场、不是候选，识别之后新建 `gpt-6.1-sol-xhigh`、写配置，两次结果不同。N25 `TestAdnSingleIDConfigPrefix` 把现状钉住（V29 杀它），以后要改就是有意改。
- (x) **只有 `--id`、名字在配置里只是前缀命中时也先识别**（v1.2，D9）：`/add grok --id x` 在 v1.1 不识别、直接打「对上的是配置里的 agent grok-4.7」；v1.2 先识别（新会话里多一行「正在识别模型」、多等一次），快照里对不上才打同一句（N23 prefix_no_match）。N14 的这一条因此挪走。这是本文对「扩到带参数的写法」的读法（D9 第二条）；若主代理认为规则一只该管 `--effort`，改法是在 `cmdAddByName` 带参数那一支的前缀判断上加 `a.haveEffort`，N23 id_over_prefix 与 prefix_no_match 的第一条跟着改。v1.3：主代理裁定**接受**，与规则三「只有 --id 算新建」一致，不改。
- (y) **总结成员不进 D17 的 S**（v1.2，D17）：`[summary]` 的 `grok-4.7` 不在场时，`/add GROK-4.7`（大写，配置第 0 步按字节比对不上）落到新建，再被同脑子那一句拦下：「配置里已经有同一个模型、同一强度的 agent grok-4.7，直接 /add grok-4.7」，照做会被第 0 步拒绝（「是总结成员……不用 /add」）。说法绕了一步，但不写配置、不把总结成员当普通成员加入（N22 summary_excluded）。这一句是 v1.1 就有的结局，本刀没改。v1.3：主代理裁定**接受**，绕路的说法留在这里。
- (z) **「同一个模型」的判据有三份**（v1.2，D15）：`configuredAgentFor`、`sameBrainAgent`（HEAD）与 `sameModelAgents`（A9）。v1.2 不动前两个（改 HEAD 的函数要重跑 MD1、CXH 的全部判据），第三个照抄判据、收集全部。以后改判据（例如 model 的大小写）要三处一起改；`sameModelAgents` 的 provider 那一半 v1.2 没有用例担保，v1.3 由 N22 other_provider_same_model 担保（V91）。
- (aa) **块 B 419 行**（v1.2，D16）：超出「每块约 400 行」19 行。能挪的（D17 的门、四句、两个辅助函数）已放进块 A 的 A9；剩下的新增都在 `cmdAddByName`、`addByOneName` 的冻结顺序里。给实现腿切块 B 时，若 419 行放不进一次 8 分钟的产出，可以把 B1-B4（64 行）与 `addname.go`（355 行）分两次给；两次都落了才编译得过（B2 调 `addname.go` 的 `cmdAddByName`，B4 引用 `msgAddNameNotFound`）。v1.3：已照这个办法拆成块 B-甲（B1-B4，64 行）与块 B-乙（`addname.go`，355 行），代码字节不变（D16）；两块互相依赖、要一起落地，v1.3 原型上只落其中一块时 `go build` 都不过（实测）。
- (ab) **v1.3 写下、不改代码的两处边界**（W1c minor 5，主代理裁定写进这里）：
  1. **已从配置里删掉、但仍在场的成员不进 S**（D17）：所以 `/add <模型>` 会再新建一个同模型的成员、写配置，同一个模型两个成员同时在场。会话成员记录里没有 model，从配置里无从知道这个成员是哪个模型。这种情形少见：要在会话开着时改配置、删掉在场的成员。W1c 的 mo 探针实跑过（`adnSeedMember` 塞一个配置里没有的在场成员 `gpt-6.1-sol-xhigh`，`/add gpt-6.1-sol` 识别、写、加入 `gpt-6.1-sol-high`）[W1c 实测，本刀未重跑]。按冻结的文字（S 取自配置）这一写是对的，不算「不该写却写了」。
  2. **配置第 3 步可能前缀加入另一个模型的 agent**：配置里有 `gpt-6.1-sol-mini`（另一个模型）、`gpt-6.1-sol-high` 在场时，`/add gpt-6.1-sol` 按前缀加入 `gpt-6.1-sol-mini`、不识别（W1c 的 tw 探针）[W1c 实测，本刀未重跑]。这是 HEAD 的前缀语义，裁决书规则二有意保留；与 (w) 同一类（N25 钉的是带强度的 ID 那一种）。

**泄漏核对（起草期间，照 §4 表头）**：原型、HEAD 对照、只落块 A 再落块 B 的那一份上（往返核对用的那几份只比字节、没有跑 go），每一次 go build / vet / test 与每一条变异都在同一条 PowerShell 命令里先 dot-source 隔离环境。每次 go test 与每条变异之后四项核对全部干净（驱动脚本每条之后核一次，另在 PowerShell 里核）。没有打开 `C:\Users\25142\.council` 下的任何文件，没有读任何 CLI 的配置与凭据，没有读写剪贴板，没有调用任何真 provider CLI（含 `codex debug models`），没有问任何模型，主仓库一个字节没写（只用了 `git archive`）。

**往返核对（v1，历史记录；v1.1 的见下）**：

- 写入：§1.1 的 13 对 `anchor` / `replace`（锚点由脚本按整行从新解出的 HEAD 取、替换文本从原型取，各锚点在 HEAD 里恰好一次）与 `addname.go`、§2 的两个测试文件，由脚本从原型取出写进本文；§4 的替换前后取自驱动脚本的变异清单，耗时与杀手取自这一次的结果文件（48 条，47 条有杀手，V7 为 0）。
- 取回：再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在又一份新解出的 `64c4b49` 上：13 对锚点各恰好一次（在原文与打补丁途中都是恰好一次，同一文件互不重叠），3 个整文件。改到或新增的 7 个文件（`repl.go`、`addmodel.go`、`commands.go`、`addname.go`、`app.go`、两个 `adn` 测试文件）、整棵树 370 个文件与原型逐字节相同（0 个差异、两边没有多出的文件）。
- 编码与符号：本文与这 7 个 Go 文件都是 UTF-8 无 BOM、只有 LF；符号禁令扫描（U+1F300-1FAFF、U+2700-27BF、U+2600-26FF、U+2B00-2BFF、U+2460-24FF、U+FE0F、U+200D）0 命中。
- 本文约 165 KB，超过 160 KB：§1.1 按块 A、B、C 组织。给实现腿切摘录时按块取：块 A 是 A1-A7，块 B 是 B1-B5，块 C 是 C1、C2；每块都连同本文开头的标识符清单、D1-D16 与注释对账、§3、§5。

**泄漏核对（v1.1，照 §4 表头）**：隔离目录 `E:\Isolating_test\council\ad1_v11\`（原型 `proto\`、只落块 A 再落块 B 的 `blockA\`、往返核对用的 `rt1\`、`rt2\`，后两份只比字节、没有跑 go）。每一次 go build / vet / test 与每一条变异都在同一条 PowerShell 命令里先 dot-source 隔离环境：`USERPROFILE`、`HOME` 指到 `canaryhome`，`APPDATA`、`LOCALAPPDATA` 指到它下面，`COUNCIL_DATA_DIR` 指到 `canarydata`，`TMP`、`TEMP` 指到 `tmp`，用 `SetEnvironmentVariable` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`，显式给 `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE`（隔离目录下）、`GOTOOLCHAIN=local`，`PATH` 收窄到 Go 与 System32，去掉 `CLAUDECODE`、`CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`。每次 go test 之后、每条变异之后都核四项（canarydata 下没有 `council.db`；隔离 `tmp`、真 `%TEMP%`、`E:\Go\tmp` 下没有 `TestOb*`、`TestAdn*` 目录；`E:\council-data` 不存在；canaryhome 下没有 `.council`），次次干净。两处 [推断] 用一次性探针（`zz_probe_test.go`，跑完即删，删后与 `rt1\` 全树比过 0 差异）实跑过，同样在隔离环境里。没有打开 `C:\Users\25142\.council` 下的任何文件，没有读任何 CLI 的配置与凭据，没有读写剪贴板，没有调用任何真 provider CLI，没有问任何模型；主仓库只用了 `git archive` 与 `git status`，`git status --porcelain` 为空、HEAD 仍是 `64c4b49`。

**往返核对（v1.1）**：

- 写入：`addname.go`、§2 的两个测试文件与 A8 的替换文本由脚本从原型取出写进本文；A8 的锚点由脚本从新解出的 HEAD 按整行取（`commands.go:1042-1047`）；其余 13 对与 v1 逐字节相同（脚本逐块比过）。§4 的替换前后由脚本写进本文，再由驱动脚本从本文解析回来跑最后一轮（与起草清单 66 条逐条比过，全同）；耗时与杀手取自最后一轮的结果文件（66 条，65 条有杀手，V7 为 0）。
- 取回：再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在又一份新解出的 `64c4b49`（`git archive`，367 个文件）上：14 对锚点各恰好一次（在原文与打补丁途中都是恰好一次，同一文件互不重叠），3 个整文件。整棵树 370 个文件与原型逐字节相同（0 个差异、两边没有多出的文件）；改到或新增的 7 个文件是 `repl.go`、`addmodel.go`、`commands.go`、`addname.go`、`app.go` 与两个 `adn` 测试文件。
- 编码与符号：本文与这 7 个 Go 文件都是 UTF-8 无 BOM、只有 LF；符号禁令扫描（U+1F300-1FAFF、U+2700-27BF、U+2600-26FF、U+2B00-2BFF、U+2460-24FF、U+FE0F、U+200D）0 命中；全角空格 U+3000 的字面字符 0 个（源码里是 `0x3000` 与 `"\xe3\x80\x80"`）。
- 本文约 228 KB，超过 160 KB：§1.1 仍按块 A、B、C 组织。给实现腿切摘录时按块取：块 A 是 A1-A8，块 B 是 B1-B5，块 C 是 C1、C2；每块都连同本文开头的标识符清单、v1.1 变更记录、D1-D16 与注释对账、§3、§5。

**泄漏核对（v1.2，照 §4 表头）**：隔离目录 `E:\Isolating_test\council\ad1_v12\`（原型 `proto\`；变异用的三份工作副本 `m1\`、`m2\`、`m3\`；只落块 A 的 `blockA\`；拿 v1.2 的 S1 跑 v1.1 生产代码的 `v11chk\`；往返核对用的 `rt1\`、`rt2\`，后两份只比字节、没有跑 go）。每一次 go build / vet / test 与每一条变异都在同一条 PowerShell 命令里先 dot-source 纯 ASCII 的 `goenv.ps1`：`USERPROFILE`、`HOME` 指到 `canaryhome`，`APPDATA`、`LOCALAPPDATA` 指到它下面的 `AppData\Roaming`、`AppData\Local`（先建好），`COUNCIL_DATA_DIR` 指到 `canarydata`，`TMP`、`TEMP` 指到 `tmp`，用 `[Environment]::SetEnvironmentVariable` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`、`CLAUDECODE` 与 `CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`，显式给 `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE`（隔离目录下的 `gocache`）、`GOTOOLCHAIN=local`，`PATH` 收窄到 Go 与 System32（node 用全路径起，环境随之继承）。偏离一处：变异驱动给三份工作副本各设了自己的 `TMP`、`TEMP`（隔离 `tmp` 下的 `m1`、`m2`、`m3`），免得一份副本正在跑的测试目录被另一份的核对当成泄漏；每条变异之后核的是本副本的 tmp 加真 `%TEMP%` 与 `E:\Go\tmp`。每次 go test 之后、每条变异之后都核四项（canarydata 下没有 `council.db`；隔离 `tmp`（含三个子目录）、真 `%TEMP%`、`E:\Go\tmp` 下没有 `TestOb*`、`TestAdn*` 目录；`E:\council-data` 不存在；canaryhome 下没有 `.council`），次次干净。§7 (c)、(w) 各用一次性探针（`zz_probe_test.go`，跑完即删，删后原型与 `rt1\` 全树比过 0 差异）实跑过，同样在隔离环境里。没有打开 `C:\Users\25142\.council` 下的任何文件，没有读任何 CLI 的配置与凭据，没有读写剪贴板，没有调用任何真 provider CLI，没有问任何模型；主仓库只用了 `git archive`、`git rev-parse` 与 `git status`，`git status --porcelain` 为空、HEAD 仍是 `64c4b49`。

**往返核对（v1.2）**：

- 写入：A1 的替换文本、A9、`addname.go`、§2 的两个测试文件由脚本从原型取出写进本文；其余 13 对锚点与替换文本、A1 的锚点与 v1.1 逐字节相同（脚本逐块比过）。§4 的替换前后：V1-V66 除 V26 外与 v1.1 相同，V26 与 V67-V86 由脚本写进本文；驱动脚本每一轮都从本文解析替换前后来跑。耗时与杀手由脚本从最后一轮的结果文件写进本文，再解析回来与结果文件比过，86 条全同（84 条有杀手，V7、V70 为 0）。
- 取回：再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在又一份新解出的 `64c4b49`（`git archive`，367 个文件）上：14 对锚点各恰好一次（在原文与打补丁途中都是恰好一次，同一文件互不重叠），4 个整文件。整棵树 371 个文件与原型逐字节相同（0 个差异、两边没有多出的文件）；改到或新增的 8 个文件是 `repl.go`、`addmodel.go`、`commands.go`、`addname.go`、`addname_existing.go`、`app.go` 与两个 `adn` 测试文件（其中 `addmodel.go`、`commands.go`、`app.go` 与 v1.1 相同）。
- 编码与符号：本文与这 8 个 Go 文件都是 UTF-8 无 BOM、只有 LF（不合格 0 个）；符号禁令扫描（U+1F300-1FAFF、U+2700-27BF、U+2600-26FF、U+2B00-2BFF、U+2460-24FF、U+FE0F、U+200D）0 命中；8 个 Go 文件里全角空格 U+3000 的字面字符 0 个（源码里是 `0x3000` 与 `"\xe3\x80\x80"`）。
- 本文约 304 KB，超过 160 KB：§1.1 仍按块 A、B、C 组织。给实现腿切摘录时按块取：块 A 是 A1-A9，块 B 是 B1-B5，块 C 是 C1、C2；每块都连同本文开头的标识符清单、严重度判法、v1.2 与 v1.1 变更记录、D1-D17 与注释对账、§3、§5。块 B 419 行，放不进一次产出时的切法见 §7 (aa)。

**泄漏核对（v1.3，照 §4 表头）**：隔离目录 `E:\Isolating_test\council\ad1_v13\`（原型 `proto\`；变异用的三份工作副本 `m1\`、`m2\`、`m3\`；分块核对用的 `blkBa\`、`blkBb\`（只 build，不 test）；锚点核对用的 `headchk\` 与往返核对用的 `rt\`、`rt2\`、`rt3\`、`rt4\`（每改完一轮文字就在新解出的一份上重做一次往返），这几份只比字节、没有跑 go）。每一次 go build / vet / test 与每一条变异都在同一条 PowerShell 命令里先 dot-source 纯 ASCII 的 `goenv.ps1`：`USERPROFILE`、`HOME` 指到 `canaryhome`，`APPDATA`、`LOCALAPPDATA` 指到它下面的 `AppData\Roaming`、`AppData\Local`（先建好），`COUNCIL_DATA_DIR` 指到 `canarydata`，`TMP`、`TEMP` 指到 `tmp`（变异驱动给三份副本各设 `tmp\m1`、`tmp\m2`、`tmp\m3`，每条之后只核本副本的 tmp 加真 `%TEMP%` 与 `E:\Go\tmp`），用 `[Environment]::SetEnvironmentVariable` 清掉 `COUNCIL_CONFIG`、`COUNCIL_GLOBAL_RULES`、`GOTMPDIR`、`GOFLAGS`、`CLAUDECODE` 与 `CLAUDE_*`（`CLAUDE_ENV_FILE` 保留）、`ANTHROPIC_*`，显式给 `GOMODCACHE=E:\Go\modcache`、`GOPROXY=off`、`GOCACHE`（隔离目录下的 `gocache`）、`GOTOOLCHAIN=local`，`PATH` 收窄到 Go 与 System32（node 用全路径起，环境随之继承）。每次 go test 之后、每条变异之后都核四项（canarydata 下没有 `council.db`；隔离 `tmp`、真 `%TEMP%`、`E:\Go\tmp` 下没有 `TestOb*`、`TestAdn*` 目录；`E:\council-data` 不存在；canaryhome 下没有 `.council`），次次干净。没有打开 `C:\Users\25142\.council` 下的任何文件，没有读任何 CLI 的配置与凭据，没有读写剪贴板，没有调用任何真 provider CLI，没有问任何模型；主仓库只用了 `git archive`、`git rev-parse` 与 `git status`。过程上一处偏离：第一次跑新用例（`-run 'TestAdnFlags$|TestAdnExistingMembers|TestAdnSingleIDConfigPrefix' -v`）时给 `go test` 加了 `2>&1` 再经 `Select-String` 过滤，违反「调原生命令不加 2>&1」；那一次只影响屏幕输出的收集，结果（全绿）之后由全量与 `-count=3` 重跑确认，之后没有再这样做。

**往返核对（v1.3）**：

- 写入：S1 由脚本从原型取出写进本文（1027 行）；16 个生产代码块与 S2 由脚本与 v1.2 逐块比过，逐字节相同（代码块一个没改，只重排了块 B 的标题与说明）。§4 的 V87-V92 手写进本文，驱动脚本从本文解析 92 条替换前后来跑（「替换前」在原型里各恰好一次，驱动数过）；耗时与杀手由脚本从结果文件写进本文（V1-V86 取第一轮，V87-V92 取第二轮），再从本文解析回来与结果文件比过，92 条全同（90 条有杀手，V7、V70 为 0）。
- 取回：再由脚本从本文取回全部 `anchor` / `replace` 与 `file` 代码块，打在又一份新解出的 `64c4b49`（`git archive`，367 个文件）上：14 对锚点各恰好一次（在新解出的 HEAD 原文里与打补丁途中都是恰好一次），4 个整文件。整棵树 371 个文件与原型逐字节相同（0 个差异、两边没有多出的文件）。
- 编码与符号：本文与改到或新增的 8 个 Go 文件都是 UTF-8 无 BOM、只有 LF；符号禁令扫描（U+1F300-1FAFF、U+2700-27BF、U+2600-26FF、U+2B00-2BFF、U+2460-24FF、U+FE0F、U+200D）0 命中；8 个 Go 文件里全角空格 U+3000 的字面字符 0 个。
- 本文约 343 KB，超过 160 KB：§1.1 按块 A、B-甲、B-乙、C 组织。给实现腿切摘录时按块取：块 A 是 A1-A9，块 B-甲是 B1-B4，块 B-乙是 B5，块 C 是 C1、C2；每块都连同本文开头的标识符清单、严重度判法、v1.3、v1.2 与 v1.1 变更记录、D1-D17 与注释对账、§3、§5。块 B-甲与块 B-乙可以分两次给，但要一起落地（D16）。