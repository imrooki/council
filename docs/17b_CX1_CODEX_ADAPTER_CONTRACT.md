# CX1 冻结契约：接入 codex —— codex provider 适配器（v1.2 = v1.1 + W1b 修订，2026-09-28；v1.1、v1 同日）

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD = origin/main = `80a9385`（W1b 修订起的基线）。`2a8737f` 之上落了三个提交：`1c62dbb`（CO1）、`e9afff9`（MD1）、`80a9385`（两刀的落地文档）。本刀要改的 `provider.go`、`registry_test.go`，以及本文引用的 `internal/turn` 生产文件（`runner.go`、`spawn.go`、`budget.go`、`record.go`、`session.go`）在 `2a8737f` 与 `80a9385` 之间逐字节相同（`git diff --stat` 核过）。**本文行号与锚点一律以 HEAD `80a9385` 为准**。v1 / v1.1 起草时的行号取自 `2a8737f`，在 `80a9385` 上有偏移的只有这几处，W1b 修订已全部改成 `80a9385` 的行号：`grok.go` MCP 残留风险那两行 `:164-165` → `:168-169`；`claude.go` `:109` → `:113`、`:289` → `:293`；`capabilities.go` 的 `Capabilities` 结构体 `:108-171` → `:108-177`（`WriteBoundary Support` 仍在 `:170`）；`app_test.go` 的测试辅助 `:42-110` → `:42-111`。文件头「原型」与「v1.1 的原型」两段是历史记录，里面的 `git archive 2a8737f` 照旧。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`。本机没有 gcc，不能 `-race`。E 盘机（Windows 11）。
- 本刀的 KEY 是 `CX1`；测试腿的包级标识符前缀是 `cxx`（类型、函数、变量、常量一律 `cxx` 开头，测试函数 `TestCxx...`）。生产代码新标识符不许以 `cxx` 开头。
- 来源：
  - `docs/00_DECISIONS.md` #45 末尾（`:2130`）：「`gpt-6.0-sol` 今天不能当成员（只有 claude / grok / agy 三个适配器）。**Griffin 裁：接入 codex 排在这两个功能之后**，先做真机探针。」真机探针已由主代理做完（2026-09-28 01:40，两次付费调用，是 Griffin 授权的全部付费调用），本契约**不再发任何 prompt**。
  - `docs/02_AGENT_PROVIDER_ARCHITECTURE.md:21-35` 能力表的 `codex-cli 0.147.0` 那一列：Step 0 之前的纸面设计，本契约以真 CLI 为准，逐项订正见 §0 第 2 条。
  - `docs/09_STEP6_SPEC.md:17`、`:265`（「不写 `codex.go`」）、`:913`（registry 组「codex 不在已登记列表里」）、`:1003`（未决 12：codex 适配器等探针）：Step 6 的范围裁决。**本刀推翻这一条**：登记 codex，并按 §1.1 C 授权改掉钉住它的三条既有用例。
- **与本批另三刀的相交**（MD1 = 模型识别 + /add，契约 `E:\multi_model_cowork\md1_contract.md` v1.1；CO1 = 简洁输出，`co1_contract.md`；SM1 = 自动总结，`sm1_contract.md`。MD1、CO1 已于 `80a9385` 落地（W1b 修订时核过：`e9afff9`、`1c62dbb`），SM1 还没派工；本契约以 HEAD `80a9385` 为基线，与 MD1 相交处照主代理裁决写，见下面「落地顺序」）：

  | 位置 | MD1 | CO1 | SM1 | CX1 | 怎么处理 |
  |---|---|---|---|---|---|
  | `internal/provider/capabilities.go` 的 `Capabilities` 结构体（`:108-177`） | A1：在 `WriteBoundary Support`（`:170`）之后加了字段 `ModelDiscovery ModelDiscovery`（`:171-176`，已落地） | 不碰（CO1 §1.2 `:522` 明写不改 `internal/provider`） | 不碰（SM1 `:71`、`:490`） | **不改这个文件** | MD1 已落，CX1 的新文件 `codex.go` 的 `Capabilities()` 字面量必须多声明一项 `ModelDiscovery`（§1.1 D1） |
  | `internal/provider/discovery.go` 的 `DiscoveryCommandFor`、`discovery_parse.go` 的 `ParseDiscovery` / `DiscoverModels`（MD1 新文件） | 新建；`switch providerID` 只认 claude / agy / grok，`default` 返回 `ErrDiscoveryUnsupported` | — | — | 不碰 | codex 登记之后这两个函数对 `"codex"` 仍走 `default`：MD1 的 P2（`DiscoveryCommandFor("codex")`）、P8 末段、P9 末条（`DiscoverModels(ctx, "codex", ...)`）照样成立，不用改 |
  | `internal/provider/discovery.go` 的 `DiscoveredModel.SuggestedID`（agy 分支遍历 `IDs()`） | 新建 | — | — | 登记 codex 使 `IDs()` 多一个 `codex` | label 以 `codex-` 开头的 agy 行会被改成 `agy-` 前缀（MD1 open item (o) 已点名）。MD1 的 agy 夹具 14 行没有这种 label（`md1_contract.md` P5、P7 的表），P7 期望不变 |
  | `internal/provider/mdx_discovery_test.go` 的 P1 `TestMdxCapabilitiesDeclareModelDiscovery`（MD1 测试腿的文件，`:199-247`） | `want` 表只有三家，`provider.IDs()` 里出现表外的 id 时 `t.Fatalf`（`:210-214`） | — | — | 登记 codex 之后这条**必红**（W1b 在 `80a9385` 原型实测：只落 A、B、C 时全量只有它红） | 授权 CX1 改它（§1.1 D2，四处，锚点与替换文本已冻结） |
  | `internal/repl/addmodel.go` 的 `discoverableProviders`（MD1 新文件） | 只列 `provider.Lookup` 找得到且 `ModelDiscovery.Available()` 为真的 provider | — | — | codex 声明 `ModelDiscoveryUnknown`（`Available()` 为假） | `/add` 的识别不会列 codex；#45「不接 codex 的识别」保持。要加 codex 成员照 §6 手加配置 |
  | `internal/provider/claude.go` / `agy.go` / `grok.go` 的 `Capabilities()` 末尾 | A2 各加一行 `ModelDiscovery:` | — | — | 不碰 | 不相交 |
  | `internal/provider/provider.go` | 只读引用 `:13-15`（只在契约正文里引用，代码不依赖那几行注释） | — | — | 改 `:13`、`:74-75` 的注释、`:103` 之后加一个哨兵（§1.1 B1-B3） | 三刀都不改这个文件，不相交 |
  | `internal/provider/registry_test.go` | 不碰 | 不碰 | 不碰 | 授权改五处（§1.1 C1-C4，C4 只改注释） | 不相交 |
  | `internal/turn` 的 `TestMain` | MD1 测试腿加了本包第一个 `TestMain`（`mdx_discovery_exec_test.go:30`：`MDX_FAKE_DISCOVERY` 为空时直接 `m.Run()`） | 不加 | 不许加 | turn 包的新测试**不写 `TestMain`、不依赖任何 `TestMain`**（只用 `fakeSpawner`，不起进程） | 不相交 |
  | `internal/app` 的 `TestMain`（`smoke_test.go:40`）与 `S10D_FAKE_STDOUT_FILE`（`smoke_test.go:55-63`） | — | — | — | app 包新测试复用它，不新写 `TestMain` | 只读复用 |
  | 屏幕行为：`/ask` 的用量、金额、冷暖三行 | — | 简洁档默认不上屏（已落地；`app_test.go` 的 `runAppForTest` 随之加了 `Verbose: true`） | — | codex 成员这三行是「用量 in=? …」「金额未报」「冷暖 … 实测=未知」 | CX1 的验收**一律不断言屏幕**（只查库与 `Record`）；§6 真机复验先敲 `/verbose on` |
  | SM1 的总结成员 | — | — | 可配置 | codex 成员可以当总结成员（走同一条 `Ask`） | 不相交；无需改动 |

  **落地顺序**：Griffin 裁 CX1 排在 MD1、CO1 之后（#45 `:2130`），SM1 在 CO1 之后；CX1 与 CO1 / SM1 **没有文件相交**，顺序只按裁决。**CX1 与 MD1 有硬依赖**。主代理裁（2026-09-28 06:50）MD1 先于 CX1 落地；**MD1 已于 `80a9385` 落地，§1.1 D1、D2 生效**，一起交给实现腿。D2 的锚点与替换文本已从 `80a9385` 的真文件 `internal/provider/mdx_discovery_test.go` 逐字取出并冻结（W1b 修订，§1.1 D2）；任务书仍附上那份真文件。v1.1 的「回退（派工时 MD1 仍没落地）」一支已作废删除。
- **原型**：本契约的全部期望值（argv、stdin 逐字节、`Record`、库里的行、解析结果、错误文本、指纹、摘要、命令行余量）已在隔离拷贝 `E:\Isolating_test\council\cx1_draft\repo\`（`git archive 2a8737f`）里用一份原型实现（§1.1 A、B、C 全部）实跑得出；§2 三个测试文件在原型上写成并全绿；**原型下全量 `go test -count=1 ./...` 八包全绿**，既有用例只有 §1.1 C 授权的三条在改之前红（改前实测：`TestRegistry_IDsAreExactlyTheThreeAdapters`、`TestRegistry_LookupUnknownProvider`、`TestRegistry_CapabilitiesDeclareEveryField`）。§4 的 X01-X57 在原型上逐条跑过，全部被点名的杀手杀掉（「原型实测」）。§2.3 的 app 测试文件在 HEAD 的隔离拷贝上单独跑过：红（两个子测试都红在 agent 消息为空、`provider_sessions` 没有行上）。原型、HEAD 拷贝与探针目录用完已删。
- **v1.1 的原型**：另起一份 `E:\Isolating_test\council\cx1_v11\repo\`（`git archive 2a8737f`）。§1.1 A 用脚本从本文件逐字截取落盘（227 行），B1、B2、C1、C2、C3 用锚点脚本落盘（每个锚点先核恰好一次）；§2 三个文件照 v1 条文重写，**v1 的全部期望第一次就过**，全量 `go test -count=1 ./...` 八包全绿。然后加 v1.1 的六行（§2.1 P6 三行、P7 三行）与 C4（五处注释），期望值取自原型实跑：`gofmt -l` 为空，`go vet ./internal/provider ./internal/turn ./internal/app` 无报告，全量八包全绿。§4 新增的 X59-X63 逐条在原型上跑：锚点恰好一次，改完只跑点名的那一个子测试（`-run '^Test…$/^子测试$'`，panic 会拖垮整个测试二进制，单独跑才能把红归到点名的那一行），五条全红；从字节副本还原，SHA-256 比对相等。另做了反向核对：用 `-skip` 去掉 v1.1 新增的六行、其余 `TestCxx|TestRegistry|TestPrefixArgs` 在三个包上照跑，五条变异**全部存活**（rc 0）—— 新行是它们唯一的杀手。§1.1 冻结的生产代码 v1.1 **一个字没改**，X01-X57 的实测结论不受影响，v1.1 没有重跑。MD1 没有落进原型（X58 仍是推演）。§0 第 4 条 v1.1 新增的零花费探针也在这个目录下跑（隔离的 `CODEX_HOME`，没有一次 `codex exec`）。原型、探针目录用完已删。
- **W1b 修订的原型**：另起一份 `E:\Isolating_test\council\cx1_fix\head\`（`git archive 80a9385`，MD1、CO1 都在里面）。§1.1 A 用脚本从本文件逐字截取落盘（227 行，行首 tab），D1 照条文插一行注释与 `ModelDiscovery: ModelDiscoveryUnknown,`；B1、B2、B3、C1-C4、D2 四处用锚点脚本落盘，每个锚点先核恰好一次（全部恰好一次）。§2 只写了 §2.1 的 P1、P6（21 行，含 W1b 改过输入的 `turn_failed_no_error`）、P7（12 行）；**T1-T6、A1 没有重写**，§2.2、§2.3 的期望仍是 v1 在 `2a8737f` 原型上的实测（两个基线之间，它们用到的 turn 包生产文件逐字节相同；app 包的 `runAppForTest` 多了一行 `Verbose: true`，A1 不断言屏幕），W2 测试腿在 `80a9385` 上第一次跑它们。`gofmt -l internal` 为空，`go vet ./internal/provider` 无报告，全量 `go test -count=1 ./...` **九包全绿**。变异 X61（改过输入之后）与 X58 在这份原型上实跑，结果见 §4；从字节副本还原，SHA-256 相等。§0 第 4 条 s10 的零花费沙箱探针也在这个目录下跑（隔离的 `CODEX_HOME`，没有一次 `codex exec`）。原型、探针目录用完已删。

## v1.1 变更记录（W1 的 F0-F6 与主代理裁决，2026-09-28）

W1：审查员照 v1 忠实实现，§2 全部期望第一次就过，X01-X57 全被点名杀手杀掉；自拟变异里非等价而存活的 4 条（E01、E02；E04、E05 只改报错文本），另有 2 条存活但判为等价（E07、E11）。证伪者复核 F0-F3，全部成立，F1 降为 minor，并指出几条修法的漏洞。主代理裁决（`cx1_rulings.md`）逐条落实如下；凡证伪者更正处按更正做。**§1.1 冻结的生产代码一个字没改**：F0 没有找到能写进 argv 的开关（见下），其余各条都在测试、注释要点、复验步骤与 open items 上。

- **F0（MCP，major）**：零花费实测（§0 第 4 条 m01-m07），隔离 `CODEX_HOME` 里照 Griffin 的形状配了四个假条目（`codex_app` 关、`cua_repl`、`drawio`、`node_repl` 开，命令全是假路径），另配了两个假名字的条目：
  - `features list` 的 104 个名字逐个 `--disable`，`mcp list` 的 Status 一项都不变；`--disable` 放在子命令后面、写成 `-c features.<名>=false`、17 个与 MCP / 浏览器 / 电脑操作相关的一起关，都不变。`-c orchestrator.mcp.enabled=false`（二进制里的一个字串）也不变。
  - `codex.exe` 里没有 `node_repl`、`cua_repl` 字样：它们不是内置名字，是写进配置的条目 [推断：桌面版写的；也可能来自插件，不读他的配置分不清]。所以**不存在**按 feature 整类关掉它们的开关，裁决第 2 点（写进两套 argv）**没有可写的东西**，argv 不变。
  - 按名字关（`-c mcp_servers.<名>.enabled=false`）有效，但名字不存在时 codex 启动失败（V0 实测，本轮复现），不许用；另测到 `--disable <不认识的 feature>` 同样 rc 1（`Unknown feature flag`），标成 removed 的照收，`-c features.<名>=false` 则对不认识的名字静默接受 —— 冻结 argv 里的 `--disable plugins`、`--disable apps` 因此有一个升级风险（不改写法，理由见注释要点 6），登记 §7 待主代理第 15 条。
  - `debug prompt-input` 在有没有这些 MCP 条目时输出逐字节相同、零处 `mcp`：模型的工具清单零花费看不到。
  - 落实：§0 第 4 条加 m01-m07 与第 8 条；§1.1 A 注释要点 6 补 `--disable` 校验名字那一句、「做不到的」加 MCP；注释要点 3（`CleanInstructionEnv`、`WriteBoundary` 两句）照 `grok.go:168-169` 的先例（`80a9385` 的行号；裁决书写的 `:165` 是 `2a8737f` 上的行号，当时就对。v1.1 这里原写「以真文件为准」，把两个基线混了，W1b 修订更正）写成残留风险、带 [推断]；§1.2 不做加一条；§4「只在审查时看」加一条；§6 第 6 步让模型列出能用的工具（含 `mcp` 开头的），第 10、14 步有能写盘的 MCP 工具时点名让它试，第 11、15、17 步分开记 shell 与 MCP；§7 待 Griffin 第 5 条。
- **F1（续会话悄悄开新会话丢身份，按 V1 降为 minor）**：不加 T 用例（它会把缺陷冻成期望）。§7 待主代理第 12 条写两支（悄悄开新会话 → 身份永久丢失；报错 → 每轮失败直到 `/session rebuild`），注明 agy 同样有这个问题、`ask.go` 的警告措辞另开一刀两家一起改。§6 第 6 步之前加一条通用注意：见到「送出去的会话 id 与回来的不一样」或 codex 每轮都失败，立刻 `/session rebuild gpt-6-sol-high`。
- **F2（用量口径的复验分不清，major）**：按 V2 改 §6 第 8、17 步：第三轮只回一个词；比 `output_tokens` 与 `reasoning_output_tokens`；**主判据是「有没有哪一项比上一轮小」，有就排除累计**；去掉 `input_tokens` 那条（两种口径都预测每轮约 +16k）；累计口径的正确推论是 out3 ≥ out2，旧稿「不小于前两轮之和」删掉。与 F0 在第 6 步相遇：第 6 步要列工具，已经不是「只回一个词」，于是同形的是第 7、8 两步（第 7 步的问法因此补了「只回答那一个词」；本轮口径预测 out3 ≈ out2，裁决原话「约等于第一轮」随之改成「约等于第 7 步」），第 6 步比后两步长得多，本轮口径下第 7 步多半已经比第 6 步小 —— 那一步就能排除累计。主判据不变。§7 待 Griffin 第 6 条：`~/.codex/sessions` 下那两次付费会话的 rollout 文件可能有 `token_count` 事件能零花费判定口径，但文件含对话内容，主代理与流水线都不许读，要他自己看或明确授权。
- **F3（两条空指针类变异能活，major）**：按 V3 更正。§2.1 P6 加 `item_without_item`（对照 `zero_msg`）、`second_thread_empty`（对照 `two_threads`）、`turn_failed_no_error`（对照 `turn_failed`）；P7 加 `turn_failed_no_error`、`two_failed`、`two_errors` 三行整串（后两行钉「第一条胜出」，失败信息不进 `TurnResult`，只有整串文本杀得到）。§4 登记 X59-X63（W1 的 E01、E02、E03、E04、E05；E03 是证伪者补的第三个空指针）。期望全部原型实跑，五条变异点名杀手实测变红，去掉新行时五条全存活。V3 可选的「只差 id 的成功正对照 `th + th + done`」没加：裁决把对照定为 `two_threads`，而「第二条 `thread.started` 一律拒」这种变异已经由 `same_thread_twice` 杀（它期望成功）。
- **F4（沙箱的零花费证据，minor）**：本轮在隔离 `CODEX_HOME` 里复现后写进 §0 第 4 条 s07-s09 与注释要点 5。v1.1 这里原写「不带 `-P` 的 `codex sandbox` 不看 `-c sandbox_mode=`、W1 的证据没有判别力」：它拿 `workspace-write` 当正对照，而 `workspace-write` 在不带 `-C` 的这一形里本身就拒写 cwd。**这个结论是错的，W1b 用 `danger-full-access` 作正对照证伪**（写得出来），W1b 修订已把四处改回「W1 的原形成立」，见下面「W1b 修订」F4 条与 §0 第 4 条 s10。`exec` / `exec resume` 走不走同一套仍未测、§6 的付费写盘探针保留，这两句不变。
- **F5（codex.cmd 这一路没测，minor）**：§0 第 3 条写明两次实测走的是 `codex.ps1`；§6 第 6 步与 §7 待主代理第 2 条点名 `codex.cmd`（cmd.exe 重新解析 `%*`：带 `=` 的参数、单独的 `-`、stdin 转发）。
- **F6（registry_test.go 的过期注释，minor）**：授权第五处改动 §1.1 C4，只改注释，逐处列出（HEAD `:50`、`:72`、`:89`、`:157-158`、`:176-177`），原型落盘后 gofmt 干净、包测试全绿。`:134-137` 是「2026-09-21 之前三家都是 Unknown」的历史叙述，不改。问了一句「同类的还有没有别处」：别的测试文件里还有随 codex 登记而过期的注释（`validate_internal_test.go:6-9` 说「没有一条自然路径能让 validateRequest 看到 WriteBoundary == SupportUnknown」，codex 正是这样一条路；`prefix_digest_test.go:10`、`:21`、`:76` 的「三家」，那条用例遍历 `IDs()`、已含 codex）。裁决只授权 `registry_test.go`，这几处不改，登记 §7 待主代理第 14 条。
- **与 MD1 的交接**：§1.1 D1、D2 生效（文件头「落地顺序」、相交表、§1.1 D 的标题）；D2 的锚点写明「W2 时按真文件取」。原型没落 MD1，X58 仍是推演。（W1b 修订：MD1 已落地，D2 的锚点与替换文本已从真文件冻结，X58 已实跑，见下面「W1b 修订」。）
- **待 Griffin**（裁决书末节六条）：§7 待 Griffin 第 1-4 条原有，第 5 条（MCP）、第 6 条（用量口径的零花费路径）新增。
- 其余连带：§1.1 C 的标题与说明、§1.2、§4「只在审查时看」、§5 的文件清单与 REPORT、§6 第 1、3 步里「四处」改「C1-C4」、变异范围改到 X63；§4 的「判定为等价」补 W1 的 E07。

**v1.1 新增或改动的用例一览**：§2.1 P6 `item_without_item`、`second_thread_empty`、`turn_failed_no_error`（新，P6 共 21 行）；P7 `turn_failed_no_error`、`two_failed`、`two_errors`（新，P7 共 12 行）。§2.2、§2.3 不变。**v1.1 新增的变异**：X59-X63。**v1.1 新增的既有测试改动**：§1.1 C4（`registry_test.go` 五处注释）。

### W1b 修订（v1.2，2026-09-28）

W1b 只审了 v1.1 这次修订，报了六条发现（一条 major、五条 minor）。六条都成立，都不涉及方向（不改用户看得到的行为，不推翻裁决书），照改。原型见文件头「W1b 修订的原型」。**§1.1 A 冻结的 `codex.go` 一个字没改。** 冻结面的变化只有三处：新增 B3（`provider.go:13`，只改注释）；D2 从「W2 时按真文件取锚点」改成逐字冻结的四处；§2.1 P6 `turn_failed_no_error` 一行的输入。

- **重建与漂移污染第 17 步（major）**：成立。重建与漂移都会让后面的轮次落到新 thread 上：累计口径下，新 thread 第一轮的 usage 从零算起，「比上一轮小就排除累计」会把累计误判成本轮，而这个结论要写进 `00_DECISIONS`、指导把数写进只追加的 `turn_usage`。写盘探针同理，第 10 步可能已经不是续会话。
  - 改了：§6「注意」末尾加一段，写明哪一段时间里重建或漂移、作废哪条结论。
  - 第 17 步加「前提」：一条冻结的 SQL，前三行同一个非空 `provider_session_id` 才判口径。依据是 `runner.go:753`、`:768` 与 `makeTurnUsage` `:1170`：漂移那一轮记的是回来的新 id。另核了 W1b 没提的一点：`/session rebuild` 不留 `session_rebuilt` 面包屑（`runner.go:235-256`），所以只能按 id 判。
  - 写盘结论拆成 exec resume（第 10 步那一行与前三行同一个 id）与 exec（第 14 步那个会话恰好一行成功轮）两条，并且都要那一轮 `outcome spoke`（失败轮里模型没动手，`False` 不算围得住）。
  - §7 待主代理第 1 条、待 Griffin 第 1 条各补一句。
  - 与 W1b 原话的差别：W1b 说「第 7-9 步一旦重建，第 10 步就成了空会话形状」。精确地说，只有重建落在第 8 步之后、第 10 步之前，第 10 步才是空会话形状；在第 7、8 步之间重建时，第 10 步续的是第 8 步开的新 thread。契约仍取更严的前提（第 6、7、8、10 步四行同一个 id），不细分，宁可多记一次「未测」。
- **F4 做偏（minor）**：成立。在隔离 `CODEX_HOME` 里复现了，写进 §0 第 4 条新增的 s10：
  - 不带 `-P`：`danger-full-access` rc 0、写出文件（两个目录各一次）；`read-only`、不给、`workspace-write` 都是 `Access is denied.`、rc 1。
  - 对照：`-P :workspace -C` rc 0，`-P :read-only -C` 拒写。
  - 改了四处，都换成 W1b 那一句：v1.1 变更记录 F4 段（标明原结论错、已更正）、§0 第 4 条 s07 的结论格（指向 s10）、§1.1 A 注释要点 5、§4「只在审查时看」末条的理由。
- **基线过期（minor）**：成立。二选一取 (a)，基线改到 `80a9385`。理由：W2 的 worktree 建在它上面，D2 的锚点只能取自它，X58 也只能在它上面实跑；一份契约混两个基线，比改几个行号更糟。
  - 行号偏移只有文件头列出的五处，逐处改了。`grok.go:168-169` 在新基线上是对的，改掉的是 v1.1 那句「裁决书写 `:165`，以真文件为准」（`:165` 在 `2a8737f` 上本来就对）。
  - 随之改的：文件头、相交表、落地顺序（回退一支删掉）、§1.1 D 的标题、§1.2、§2 通用纪律（§2.3 的 HEAD 结果标明是 `2a8737f` 上的实测）、§5（PATCH 锚点的基线与文件清单）、§6 的第 1、3、5 项（收割、变异、配置示例；不是第 6 项真机复验里的小步）。
  - D2 改成逐字冻结的四处，见 §1.1 D2。X58 在 `80a9385` 原型实跑。
  - 同类的别处：CO1 已落地，§6 第 6 项真机复验的第 2 步改成 `/verbose on`（`/verbose` 是来回切换，多敲一次就回到简洁档）。核过 MD1 没改 `/add <id>` 的语义（`cmdAdd` 里只有 `+` 开头的参数走识别），§6 第 5、13 步不变。
- **「同类的还有没有别处」清单不全（minor）**：成立。
  - `mdx_discovery_test.go:202`、`:212` 由 D2-1、D2-3 接走。
  - `provider.go:13` 取 W1b 的第一个选项，做成 §1.1 B3：本刀本来就在改这个文件，整行锚点恰好一次，只改注释。
  - `addmodel.go:54-55`、`ask_test.go:60-61`、`turn.go:154` 列进 §7 待主代理第 14 条。
  - W1b 修订自己又 grep 了一遍，比 W1b 多找到八处，按「过期注释 / 仍成立 / 覆盖缺口」三类列进同一条。其中 `rules_test.go:210-224` 是覆盖缺口：只查三个名字。
- **`turn_failed_no_error` 与对照差两处（minor）**：成立，取 W1b 的第一个选项。
  - P6 这一行的输入补上 `{"type":"turn.started"}`，与 `turn_failed` 只差 `error` 对象。期望不变（原型实跑）。
  - X61 在新输入上复跑：两个杀手仍红，对照 `turn_failed` 绿。
  - P7 那一行不动，它的只差一处的对照是 P7 第 6 行。P6 表后的说明写明了两行输入不同。
- **第 14 步只给了 shell 一块（minor）**：成立。
  - 第 10、14 步都给两块。MCP 那一形里 shell 写 `PROOF.txt`、MCP 写 `PROOF_MCP1.txt`，多个工具依次编号。
  - 第 11、15 步各查两个文件；第 17 步按文件名分开记。
  - 这条与第 1 条都改第 17 步与「注意」，一起写完后通读过，没有自相矛盾。
- **W1b 的逐条核对里另提的**：F0 第 3 点的 `grok.go` 行号并入基线那条；其余与六条发现重复，不另列。
- **没做的，如实写**：
  - T1-T6、A1 没有在 `80a9385` 上重写（W1b 也没有），§2.2、§2.3 的期望仍是 v1 在 `2a8737f` 原型上的实测。它们用到的 turn 包生产文件在两个基线之间逐字节相同，W2 测试腿第一次在 `80a9385` 上跑。
  - X59、X60、X62、X63 的输入与锚点都没变，本轮没有重跑（W1b 已在两个基线上复核）。
- **新增变异**：无。X58 从推演变为实测，X61 在新输入上复跑。
- **超出裁决书字面的三处取舍**（基线、D2 逐字冻结、B3）：列进 §7 待主代理第 16 条，请主代理确认。

---

## 0. 病根 / 动机（行号以 HEAD `80a9385` 为准）

1. **今天 codex 成员开不了口。** 适配器只有三家，各在自己文件的 `init()` 里 `Register`（`claude.go:33-35`、`grok.go:20-22`、`agy.go:21-23`）；`Ask` 在 `runner.go:399-405` `provider.Lookup` 失败时以 `ErrUnknownProvider` refuse，不起进程。`provider.go:74-75` 的注释拿 codex 当「没有适配器」的例子。`registry_test.go:16-22` 断言 `IDs()` 恰好是 `[agy claude grok]`，`:38` 拿 `Lookup("codex")` 当「查不到」的例子，`:139-143`、`:160-164`、`:181-185` 三张表对第四家 `t.Fatalf`。Griffin 要把 `gpt-6-sol` 这类 codex 模型当 Council 成员（#45 `:2130`）。

2. **`02` 表的 codex 列是纸面设计，三处与真 CLI 不同**（`codex-cli 0.147.0`，本契约起草时 `codex --version` 核对过）：
   - 身份：表写「走 AGENTS.md」（`02:29`）。真 CLI 会把 cwd 的 `AGENTS.md` 当 project-doc 拼进上下文（§0 第 4 条 v17），但本契约不用它，改走首轮正文前缀（理由见 §1.1 A 注释要点 1）。
   - 权限：表写 `-s read-only/...`（`02:31`）。`codex exec resume --help` **没有** `-s/--sandbox`，只能 `-c sandbox_mode=...`（中转根 `codexprobe_resume_help.txt`）。
   - 工作目录：表写 `-C/--cd`（`02:32`）。`exec resume` 同样没有 `-C`；本契约不发 `-C`，cwd 由 `Command.Dir` 给（`spawn.go:151`）。
   - 表里 effort 一格（`-c model_reasoning_effort=`，`02:30`）与真 CLI 一致；跨轮缓存一格（`02:35`「未实测」）由本刀的真机数据补成「有」（下面第 3 条）。

3. **真 CLI 事实**（主代理 2026-09-28 01:40 实测，E 盘机，两次付费调用；原件在中转根，只读）：
   - 新会话：`codex exec --json --skip-git-repo-check -s read-only -m gpt-6-sol -c model_reasoning_effort=low --disable plugins --disable apps -o <file> "<prompt>"` → rc 0，约 4.7 s。
   - 续会话：`codex exec resume <thread_id> --json --skip-git-repo-check -m gpt-6-sol -c model_reasoning_effort=low -c sandbox_mode=read-only --disable plugins --disable apps -o <file> "<prompt>"` → rc 0，约 4.9 s，模型答出了上一轮让它记住的词。`-c sandbox_mode=...` 在 resume 上**是否生效没验证**。
   - stdout 是 JSONL：`thread.started{thread_id}` → `turn.started` → `item.completed{item:{id,type:"agent_message",text}}` → `turn.completed{usage:{input_tokens,cached_input_tokens,cache_write_input_tokens,output_tokens,reasoning_output_tokens}}`；续会话时 `thread_id` 不变；没有金额字段；没有型号字段。stderr 有一行 `Reading additional input from stdin...`（当时 stdin 是空设备、prompt 在 argv）。
   - 原件：`codexprobe_exec1.jsonl`（347 字节，SHA-256 `E888CBCE05ADF85241BEC6B6778E0D3AC772513AA27B1FF3D733F32EED41E144`）、`codexprobe_exec2.jsonl`（352 字节，SHA-256 `08B20446A8858EEA2C967D4E275A91AB66F13A6854C58411C1B9602BFE64DCD8`）。**两份的行尾都是 CRLF**（`0D 0A`，末行后也有）。[推断] 这是 PowerShell 接住 stdout 再落盘时改的，Go 直接读子进程 stdout 大概率是 LF；解析器两种都要吃（§2.1 P5 两种都喂）。§2 的夹具常量与这两份原件逐字节相同（原型用脚本比过）。
   - 用量：第一轮 in 16184 / cached 0 / cache_write 0 / out 6 / reasoning 0；续会话 in 32396 / cached 16000 / cache_write 0 / out 12 / reasoning 0。
   - **两次实测走的都是 `codex.ps1`**（PowerShell 里 `Get-Command codex` 解析到 `C:\Users\25142\AppData\Roaming\npm\codex.ps1`）。Council 起的是 `codex.cmd`：`spawn.go` 用 `exec.LookPath`，Windows 上按 PATHEXT 取到 `.cmd`，由 cmd.exe 把 `%*` 重新解析一遍再转给 `node ...\codex.js`。带 `=` 的参数（`-c sandbox_mode=read-only` 这一类）、单独的 `-`、stdin 经 cmd.exe 转给 node，这一路**没测**；§6 第 6 步是它的第一次（W1 F5）。

4. **零花费探针**（本契约起草时实跑，2026-09-28，E 盘机，cwd 是隔离目录；只跑了 `codex --help` 一类与 `codex debug prompt-input`、`codex features list`，**没有一次 `codex exec`**）。`prompt-input` 渲染的是模型可见的输入列表（不含 base instructions），下表是 `--disable plugins --disable apps` 之后再叠加各项的 JSON 字节数：

   | # | 叠加的参数 | 字节 | 结论 |
   |---|---|---|---|
   | v00 | （无） | 26049 | 基线：developer 消息有 skills 说明 9080、permissions 说明 9060、collaboration_mode 920，另有两条 multi-agent 提示（207、271）；user 消息有全局 `AGENTS.md` 3681、environment_context 480、正文 |
   | v01 | `-c developer_instructions="..."` | 26139 | 这个键有效：多出第一条 developer 消息。但 PowerShell 5.1 剥掉了内嵌引号，codex 收到的是「TOML 解析失败、按原样字符串」那一支，`\n` 没有被转义（Go 起进程时引号会保留，行为会不同，没测） |
   | w02 | `-c skills.include_instructions=false` | 16882 | **整块 skills 说明（9080 字节）消失**；那一块列的是 Griffin 的私人 skill 清单。`skills.enabled` 不是有效键（主代理此前实测）；`-c skills.bundled.enabled=false` 只去掉内置的那部分（9080 → 6708） |
   | v04 | `-c include_permissions_instructions=false` | 16377 | permissions 说明整块消失 —— 那是模型对沙箱的认知，不是沙箱本身，本契约**不用** |
   | v05 | `-c include_environment_context=false` | 25483 | environment_context 消失（cwd、shell、current_date、timezone、workspace_roots）。本契约不用（见 open item） |
   | w03 | `-c include_collaboration_mode_instructions=false` | 25048 | collaboration_mode 消失。本契约不用 |
   | s00 / s01 | 再加 `-c skills.include_instructions=false`；s01 另加 `-c sandbox_mode=read-only` | 16882 / 16882 | permissions 说明第一句「`sandbox_mode` is `read-only`」—— 这个 cwd 下默认就是 read-only |
   | s02 / s03 | `-c sandbox_mode=workspace-write` / `danger-full-access` | 17557 / 16839 | permissions 说明第一句跟着变：**`-c sandbox_mode=` 被配置加载认** |
   | s04 | `-c sandbox_mode=bogus-mode` | rc 1 | `unknown variant 'bogus-mode', expected one of 'read-only', 'workspace-write', 'danger-full-access'`：沙箱取值**有校验** |
   | s06 | `-c approval_policy=never` | 7622 | permissions 说明从 9060 字节缩到 342 字节，第二句变成「Approval policy is currently never. Do not provide the `sandbox_permissions` for any reason, commands will be rejected.」 |
   | e1 | `-c model_reasoning_effort=bogus`（以及 `none`、`LOW`） | rc 0 | **强度取值不校验**，非法值静默接受（与 grok 的「非法值静默接受」同一类坑）。`ultra` 比 `low` 多 77 字节 |
   | v08 / w10 | `-c project_doc_max_bytes=0`（另加 `project_doc_fallback_filenames=[]`） | 26049 | 去不掉全局 `AGENTS.md`（`CODEX_HOME` 下那份） |
   | v17 | cwd 放一个 `AGENTS.md` | — | 被读进去：全局那段后面接 `--- project-doc ---` 与这个文件的内容 |
   | — | `--disable multi_agent --disable multi_agent_v2`、`--disable skill_search`、`--disable collaboration_modes`、`--disable personality`、`-c include_apps_instructions=false`、`-c skills.user.enabled=false`、`-c include_skills_instructions=false` | 26049 | 全部无变化 |
   | — | `codex debug prompt-input --ignore-user-config` / `--ignore-rules` | rc 2 | `unexpected argument`：**这两个参数 prompt-input 不收，效果测不到** |
   | — | `codex debug prompt-input -`，以及 stdin 重定向不带参数 | rc 0 | prompt-input 把 `-` 当正文原样渲染成一条 user 消息，不读 stdin：**stdin 这条路零花费测不到** |

   v1.1 另跑了下面这些（2026-09-28，E 盘机，`codex-cli 0.147.0`，`CODEX_HOME` 指到隔离目录，cwd 是隔离目录；只用 `mcp list`、`features list`、`sandbox`、`debug prompt-input`，外加对 `codex.exe` 的字符串检索；**没有一次 `codex exec`**）。m 开头的是 MCP（W1 F0），s07-s09 是沙箱（W1 F4），s10 是 W1b 修订补跑的沙箱（W1b F4，同样的隔离条件）：

   | # | 做了什么 | 结果 | 结论 |
   |---|---|---|---|
   | m01 | 隔离 `config.toml` 照 Griffin 的形状写四个假条目：`codex_app`（`enabled = false`）、`cua_repl`（带 `BROWSER_USE_*`、`CUA_REPL_*`、`NODE_REPL_*` 三个假环境变量）、`drawio`、`node_repl`，命令全是不存在的假路径；`codex mcp list --json` | rc 0：`codex_app` enabled=false，另三个 enabled=true，`disabled_reason` 全是 null | 基线 |
   | m02 | `features list` 列出的 104 个名字逐个 `codex --disable <名> mcp list --json`，比每一项的 enabled 与 `disabled_reason` | 104 个全部 rc 0、与基线相同 | **没有一个 feature 能关掉配置里的 MCP 条目** |
   | m02b | `--disable` 放到子命令后面（`mcp list --disable computer_use --disable browser_use --disable in_app_browser --disable code_mode_host`）；`-c features.computer_use=false -c features.browser_use=false`；另一份配置只有两个假名字的条目（`cxx_fake_repl`、`cxx_fake_user`），17 个与 MCP / 浏览器 / 电脑操作 / 插件相关的 feature 一次全关 | 全部与各自的基线相同 | 与名字无关，也与写法无关 |
   | m03 | `-c mcp_servers.drawio.enabled=false`；`-c mcp_servers.nosuch.enabled=false` | 前者 drawio 变 disabled；后者 rc 1：`failed to load bootstrap configuration` / `invalid transport in mcp_servers.nosuch` | 按名字关有效，但名字不存在时 codex 起不来（V0 实测，本轮复现）：**不能写进冻结的 argv** |
   | m04 | `codex --disable bogus_feature_xyz mcp list`；`codex debug prompt-input --disable bogus_feature_xyz PING`（子命令上的 `--disable`，与 `exec` 同一种写法）；`--disable js_repl`（`features list` 里标 removed）；`-c features.bogus_feature_xyz=false` | 前两个 rc 1：`Unknown feature flag: bogus_feature_xyz`；`js_repl` rc 0；`-c features.…=false` rc 0（不认识的名字静默接受） | `--disable` 校验名字：不认识的让 codex 起不来，标 removed 的照收。`-c features.<名>=false` 不校验 |
   | m05 | `-c orchestrator.mcp.enabled=false`（`codex.exe` 里有一句「MCP server '…' is disabled by `orchestrator.mcp.enabled`」，挨着 `multi_agents_v2` 的源文件名） | 与基线相同 | 不是整类开关 [推断：那是子代理那条路上的设置] |
   | m06 | `debug prompt-input --disable plugins --disable apps -c skills.include_instructions=false -c sandbox_mode=read-only -c approval_policy=never PING`，空配置与 m01 的配置各一次 | 两次都是 4876 字节、逐字节相同，`mcp` 零处 | **MCP 工具在不在模型的工具清单里，零花费看不到** |
   | m07 | 对 `codex.exe`（298668336 字节）做字符串检索：`node_repl`、`cua_repl` | 零处 | 它们不是 codex 内置的名字，是写进配置的条目 [推断：桌面版写的；也可能来自插件，不读 Griffin 的配置分不清] |
   | s07 | 空配置；`codex sandbox -P :read-only -c approval_policy=never -C <隔离目录> -- cmd.exe /c "echo WRITTEN> PROOF.txt"`；对照 `-P :workspace -C <隔离目录>`；再试不带 `-P` 的三形：W1 的原形 `codex sandbox -c sandbox_mode=read-only -- …`、换成 `-c sandbox_mode=workspace-write`、`-c sandbox_mode` 什么都不给（都不带 `-C`）；以及不带 `-P` 而带 `-C` | `-P :read-only`：`Access is denied.`，rc 1，文件不存在；`-P :workspace`：rc 0，文件写出来了；不带 `-P` 的三形：**全部** `Access is denied.`、rc 1（W1 的原形复现了，workspace-write 也拒）；不带 `-P` 带 `-C`：rc 2，`the following required arguments were not provided: --permission-profile <NAME>` | Windows 的受限令牌沙箱在 `-P :read-only` 下真的拒写（shell 命令），`-P :workspace` 是它的正对照。不带 `-P` 那几形的结论见 s10（W1b 修订：v1.1 在这一格据 workspace-write 也拒写，推出「不带 `-P` 时不看 `-c sandbox_mode=`」，是错的） |
   | s08 | `config.toml` 写 `default_permissions = ":workspace"`，再换成 `":danger-full-access"`；各跑 `debug prompt-input` 两次：不加覆盖、加 `-c sandbox_mode=read-only -c approval_policy=never` | 不加：permissions 说明分别是 `sandbox_mode` is `workspace-write` / `danger-full-access`；加了：两种配置都是 `read-only`，且「Approval policy is currently never」 | `-c` 盖得过用户配置里的默认权限（只在渲染上看到） |
   | s09 | `config.toml` 写 `profile = "pp"` | rc 1：``legacy `profile = "pp"` config is no longer supported; use `--profile pp` with `pp.config.toml` instead`` | 用户配置里不能再用旧 `profile` 键悄悄换一套权限；新写法要命令行给 `--profile`，Council 不给 |
   | s10（W1b 修订） | 空配置，不带 `-P`、不带 `-C`，cwd 是各自新建的空目录（事先没有 `PROOF.txt`）：`codex sandbox -c sandbox_mode=danger-full-access -- cmd.exe /c "echo WRITTEN> PROOF.txt"`（两个目录各一次）；同一形换成 `read-only`、`workspace-write`、不给 `-c`；对照 `-P :workspace -C <目录>`、`-P :read-only -C <目录>`；`-c sandbox_mode=workspace-write -C <目录>`（不带 `-P`） | `danger-full-access`：rc 0，文件存在（两次相同）；`read-only`、不给、`workspace-write`：`Access is denied.`，rc 1，文件不存在；`-P :workspace -C`：rc 0，文件存在；`-P :read-only -C`：`Access is denied.`，rc 1；不带 `-P` 带 `-C`：rc 2（要求 `--permission-profile`） | `codex sandbox` 不带 `-P` 时**读** `-c sandbox_mode=`：`danger-full-access` 写得出（正对照），`read-only` 与不给都拒写（W1 的原形成立）；`workspace-write` 在不带 `-C` 的这一形里也拒写 cwd，原因未测。所以在 `codex sandbox` 这条路上，`-c sandbox_mode=read-only` 选中的是拒写的沙箱；`exec` 与 `exec resume` 走不走同一套仍未测，§6 的付费写盘探针保留 |

   另：`codex debug models`（中转根 `codexprobe_models.json`）13 个模型的 `supported_reasoning_levels` 并集是 `minimal, low, medium, high, xhigh, max, ultra`；逐模型不同（`gpt-6-sol` 是 `low…ultra` 六档，`gpt-6-luna` 没有 `ultra`，只有 `custom/qwen3.8-27b` 有 `minimal`）。`ultra` 的说明是「Maximum reasoning with automatic task delegation」。

5. **正文只能走 stdin。** 本机的 `codex` 是 npm 装出来的 `codex.cmd` 垫片（`%*` 转给 `node ...\codex.js`）。`spawn.go:118-125` 对 `.cmd` / `.bat` 拒绝任何含换行的 argv（`ErrShimNewlineArg`），而每一轮的 delta 天然多行（`render.go:24-54`、`router.Render`）。claude 是同一种垫片，走的就是 stdin（`claude.go:22-30`、`:113`）。`codex exec --help` 自述「If not provided as an argument (or if `-` is used), instructions are read from stdin」，`codex exec resume --help` 自述「If `-` is used, read from stdin」—— **两条都没实测**（第 4 条末行：零花费测不到），是 §6 真机复验的第一件事。

6. **用量很可能是整个 thread 的累计值。** 第二轮 in 32396 ≈ 16184 × 2、out 12 = 6 + 6 —— [推断] `turn.completed.usage` 是 thread 累计，不是本轮。但另一种读法也说得通：resume 时 codex 可能把整段初始上下文再送一遍，第二轮的完整 prompt 本来就是约 32 k。两个数据点分不开这两种假设。
   - 按累计值当本轮记：假设对时每轮多记一整段历史，`/cost` 与 `/round` 的预报被放大，**而且是写进只追加的事实源、改不掉**。
   - 按增量记（本轮累计减上一轮累计）：要在 `provider_sessions` 存一份 token 基线（schema 迁移），而且假设错时差值是乱数、甚至为负。
   - **标成未报（nil）**：假设对错都只是「不知道」—— `turn_usage` 五个 token 列写 NULL，`/ask` 印 `?`，`/round` 预报对 codex 成员不标定（`budget.go:166-172` 取不到样本），冷暖「实测=未知」（`record.go:174-179`）；原始 usage 对象原样进 `messages.meta["usage_raw"]`，将来证实口径之后可以回算。**后果最轻，本契约选它**（§1.1 A `codexUsage`）。

7. **强度值必须由适配器自己校验。** 第 4 条 e1：codex 对 `model_reasoning_effort` 的非法值静默接受、退出码 0。照 claude 的做法把配置值原样放上命令行，写错一个字母就是一轮「屏幕上是请求档、实际跑默认档」。`provider.go` 现有的哨兵没有一个说「这个 effort 值本家不认」（`ErrCapabilityUnknown` 说的是能力位是 Unknown），所以新增一个（§1.1 B2）。

8. **MCP 服务器隔离不掉（v1.1，W1 F0）。** `$CODEX_HOME/config.toml` 里配的 MCP 服务器，`mcp list` 里标 enabled 的，`exec` 时都会被加载 [推断：`exec` 起没起它们，零花费看不到]；Council 发的参数里没有一个管它们。主代理 2026-09-28 06:45 零花费看过 Griffin 的清单（`codex mcp list`，只列清单）：`codex_app` 关，`cua_repl`（桌面版运行时的电脑 / 浏览器操作）、`drawio`（他自己装的）、`node_repl`（桌面版运行时的 Node 代码执行环境）开着。`node_repl` 能执行任意代码。
   - 整类关掉的开关不存在（第 4 条 m02、m02b、m05、m07）；按名字关在名字不存在时让 codex 起不来（m03），而且名字来自 Griffin 自己会改的配置、是黑名单，不能冻进 argv（与 `grok.go:168-169`「工具级的黑名单，不是白名单」同一条教训：后加的服务器不在名单里）。`--ignore-user-config` 会连模型路由一起丢掉（§1.1 A 注释要点 1），不用。
   - read 档（`sandbox_mode=read-only`）管不管得住 MCP 工具的调用，零花费测不到 [推断：多半管不住，MCP 工具不经 shell]；模型的工具清单零花费也看不到（m06）。这是与 `grok.go:168-169` 同一类的残留风险，而且更重：这里已知 Griffin 的 codex 配置开着一个能执行代码的（`node_repl`）。
   - 本刀的处置：argv 不变；注释要点 3 写成残留风险；§6 真机复验让模型列出工具、用能写盘的 MCP 工具再试一次写盘；§7 待 Griffin 第 5 条（要么他在自己的 codex 配置里关掉，要么接受）。

---

## 1. 范围

### 1.1 做

**冻结粒度**：下面给出的代码是原型实跑过的版本。**标识符名、签名、判据、字面量（含全部错误文本）、参数顺序全部冻结**；只有「注释」由实现方写，必须覆盖每条列出的「注释要点」，注释不许写与代码不符的话。代码块里的缩进可能被排成空格，落盘一律 tab（gofmt）。补丁锚点一律从 HEAD 真文件逐字复制。

#### A. 新文件 `internal/provider/codex.go`（逐字照抄；注释按要点补）

```go
package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type codexProvider struct{}

func init() {
	Register(codexProvider{})
}

func (codexProvider) ID() string { return "codex" }

var codexEffortLevels = []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

func (codexProvider) Capabilities() Capabilities {
	return Capabilities{
		SessionID:      SessionIDReturnedOnFirstTurn,
		Effort:         EffortPerTurnFlag,
		Identity:       IdentityPromptPreamble,
		Prompt:         PromptTransportStdin,
		MaxPromptBytes: 0,
		Permissions:    []PermissionTier{PermissionRead},
		UsageFormula:   "input_tokens",

		OutputSchema:         SupportYes,
		MaxTurnsFlag:         SupportNo,
		ReportsCost:          SupportNo,
		ReportsCacheCreate:   SupportYes,
		ReportsObservedModel: SupportNo,
		CleanInstructionEnv:  SupportNo,
		CacheAcrossTurns:     SupportYes,
		WriteBoundary:        SupportUnknown,
	}
}

func (p codexProvider) BuildTurn(req TurnRequest) (Command, error) {
	caps := p.Capabilities()
	if err := validateRequest(caps, req); err != nil {
		return Command{}, err
	}
	perm, err := codexPermissionArgs(req.Permission)
	if err != nil {
		return Command{}, err
	}
	if req.Effort != "" && !codexEffortAllowed(req.Effort) {
		return Command{}, fmt.Errorf("effort %q is not a codex reasoning level (want one of %s): %w", req.Effort, strings.Join(codexEffortLevels, ", "), ErrUnsupportedEffort)
	}

	prompt := req.Prompt
	if req.ProviderSession == "" && req.Role != "" {
		prompt = req.Role + "\n" + req.Prompt
	}

	args := make([]string, 0, 24)
	args = append(args, "exec")
	if req.ProviderSession != "" {
		args = append(args, "resume", req.ProviderSession)
	}
	args = append(args, "--json", "--skip-git-repo-check")
	args = append(args, perm...)
	args = append(args, "-m", req.Model)
	if req.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+req.Effort)
	}
	args = append(args, "--ignore-rules")
	args = append(args, "--disable", "plugins", "--disable", "apps")
	args = append(args, "-c", "skills.include_instructions=false")
	args = append(args, "-")

	return Command{
		Args:      args,
		Dir:       req.WorkDir,
		Env:       nil,
		Stdin:     []byte(prompt),
		Transport: PromptTransportStdin,
		Dropped:   dropMaxTurns(caps, req),
	}, nil
}

func codexEffortAllowed(e string) bool {
	for _, v := range codexEffortLevels {
		if v == e {
			return true
		}
	}
	return false
}

func codexPermissionArgs(t PermissionTier) ([]string, error) {
	switch t {
	case PermissionRead:
		return []string{"-c", "sandbox_mode=read-only", "-c", "approval_policy=never"}, nil
	default:
		return nil, fmt.Errorf("permission tier %q: codex adapter offers read only: %w", t, ErrUnsupportedTier)
	}
}

type codexEvent struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     *codexItem      `json:"item"`
	Usage    json.RawMessage `json:"usage"`
	Error    *codexError     `json:"error"`
	Message  string          `json:"message"`
}

type codexItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexError struct {
	Message string `json:"message"`
}

func (codexProvider) ParseTurn(stdout []byte) (TurnResult, error) {
	if emptyStdout(stdout) {
		return TurnResult{}, fmt.Errorf("codex: stdout is empty: %w", ErrUnparseableOutput)
	}
	var (
		threadID  string
		known     bool
		messages  []string
		completed int
		usageRaw  json.RawMessage
		failed    bool
		failMsg   string
		errored   bool
		errMsg    string
	)
	for i, line := range bytes.Split(stdout, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var ev codexEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return TurnResult{}, fmt.Errorf("codex: stdout line %d is not JSON: %w", i+1, ErrUnparseableOutput)
		}
		switch ev.Type {
		case "thread.started":
			known = true
			if threadID != "" && ev.ThreadID != threadID {
				return TurnResult{}, fmt.Errorf("codex: two thread.started events with different thread_id (%q, %q): %w", threadID, ev.ThreadID, ErrUnparseableOutput)
			}
			threadID = ev.ThreadID
		case "turn.started", "item.started", "item.updated":
			known = true
		case "item.completed":
			known = true
			if ev.Item != nil && ev.Item.Type == "agent_message" {
				messages = append(messages, ev.Item.Text)
			}
		case "turn.completed":
			known = true
			completed++
			if !absentOrJSONNull(ev.Usage) {
				usageRaw = append(json.RawMessage(nil), ev.Usage...)
			}
		case "turn.failed":
			known = true
			if !failed {
				failed = true
				if ev.Error != nil {
					failMsg = ev.Error.Message
				}
			}
		case "error":
			known = true
			if !errored {
				errored = true
				errMsg = ev.Message
			}
		}
	}
	if !known {
		return TurnResult{}, fmt.Errorf("codex: stdout has no known codex event: %w", ErrUnparseableOutput)
	}
	if completed > 1 {
		return TurnResult{}, fmt.Errorf("codex: %d turn.completed events, expected one: %w", completed, ErrUnparseableOutput)
	}

	base := TurnResult{
		ProviderSession: threadID,
		Usage:           codexUsage(),
		UsageRaw:        usageRaw,
	}
	if failed {
		base.StopRaw = "turn.failed"
		base.Completion = CompletionUnknown
		return base, fmt.Errorf("codex: turn failed (message=%q): %w", failMsg, ErrTurnFailed)
	}
	if completed == 0 {
		if errored {
			base.StopRaw = "error"
			base.Completion = CompletionUnknown
			return base, fmt.Errorf("codex: turn failed (error=%q): %w", errMsg, ErrTurnFailed)
		}
		return TurnResult{}, fmt.Errorf("codex: stdout has no turn.completed event: %w", ErrUnparseableOutput)
	}

	base.StopRaw = "turn.completed"
	if n := len(messages); n > 0 {
		base.Text = messages[n-1]
		base.Thought = strings.Join(messages[:n-1], "\n\n")
	}
	base.Completion = completionFrom(base.Text, "")
	if threadID == "" {
		return base, fmt.Errorf("codex: success shape carries empty thread_id: %w", ErrNoSessionID)
	}
	return base, nil
}

func codexUsage() Usage {
	return Usage{
		CostBasis:    CostBasisNone,
		CostUSDTotal: nil,
		CacheCreate:  nil,
		Unreported:   []string{"input_tokens", "cache_create", "cache_read", "output_tokens", "thinking_tokens"},
	}
}
```

（原型实数：去掉注释 227 行；这一块代码已用脚本与原型文件去掉注释行之后逐字节比过、相等。gofmt 干净、`go vet ./internal/provider` 干净。）

**注释要点**（写在对应声明上方或行尾；中文，可以扩写，不许写与代码不符的话；凡本契约标「未实测 / 推断」的，注释里同样标出）：

1. 〔文件头 / `codexProvider`〕适配 codex-cli 0.147.0 的 `exec --json`（JSONL 事件流）。两套命令行形状（§3.1、§3.2）：空会话 `exec ...`、续会话 `exec resume <thread_id> ...`，其余参数两套逐项相同、每轮原样再发（`provider.go:31-34` 的纪律）。
   - 正文走 stdin、末尾显式给 `-`：`codex.cmd` 垫片 + 多行 delta 不能进 argv（`spawn.go` 的换行闸）；两条 help 都自述 `-` 读 stdin，**未实测**（`debug prompt-input` 不读 stdin，零花费测不到），真机复验见 CX1 契约 §6。
   - 身份走首轮正文前缀，与 agy 同一套（`IdentityPromptPreamble`，只有空会话那一支拼，resume 不再拼，身份已经在那条 thread 里）。不用 `-c developer_instructions=`：身份与规则是任意多行文本，要进 argv、过 `.cmd` 垫片（cmd.exe 再解析一遍 `%*`），而且它经 TOML 解析、解析失败按原样字符串，Go 起进程时的真实行为没测。不用 cwd 下的 `AGENTS.md`：codex 对项目文档有字节上限（`project_doc_max_bytes`），规则文件大时身份会被静默截短 [上限的具体值与截断行为本机未实测]，而且它只在会话开头注入。
   - 不发 `-C`（cwd 由 `Command.Dir` 给；`exec resume` 没有 `-C`）；不发 `-o`（stdout 已经带着正文）；不发 `--ephemeral`（不落会话文件，下一轮 resume 找不到）；不发 `--ignore-user-config`（它不加载 `$CODEX_HOME/config.toml`，Griffin 的模型路由与 `model_instructions_file` 都在那份文件里；`debug prompt-input` 不收这个参数，后果零花费测不到）。
2. 〔`codexEffortLevels`〕取值来自 `codex debug models` 全部 13 个模型 `supported_reasoning_levels` 的并集（2026-09-28）。为什么要自己校验：`-c model_reasoning_effort=<非法值>` 被 codex 静默接受、退出码 0（`debug prompt-input` 实测），写错一个字母就是「屏幕上是请求档、实际跑默认档」。只比并集、不按模型查：本包不起进程、不读目录；某个模型不支持的档（例如 `gpt-6-luna` 没有 `ultra`）照样放行，那一轮的真实行为未实测。大小写敏感（`LOW` 拒绝）。
3. 〔`Capabilities()`〕逐字段一句依据：
   - `SessionID`：`thread_id` 只能从输出的 `thread.started` 取（没有预指定 id 的参数）；续会话时不变（真机两轮同一个 id）。
   - `Effort`：`-c model_reasoning_effort=<值>` 每轮真的放上命令行（真机两轮都带着）。
   - `Identity`：见要点 1。
   - `Prompt` / `MaxPromptBytes`：stdin，没有 argv 上限；`turn.argvHeadroom` 于是只量固定的参数段（CX1 契约 §2.2 T3）。
   - `Permissions`：只有 read；workspace 不给，理由见要点 5。
   - `UsageFormula`：`input_tokens` [推断：它已含 `cached_input_tokens`，第二轮 16000 ≤ 32396]。但 turn.completed 的 usage 很可能是 thread 累计值 [推断]，所以 Council 一个 token 列都不记，见 `codexUsage`。
   - `OutputSchema`：help 列了 `--output-schema <FILE>`；本步 `TurnRequest` 没有这一项，不发。
   - `MaxTurnsFlag`：help 里没有。
   - `ReportsCost`：输出里没有金额字段。
   - `ReportsCacheCreate`：`cache_write_input_tokens` 这个键在，但两轮都是 0（第二轮 cached 已是 16000），与 grok 的陷阱 7 同形；反正不记。
   - `ReportsObservedModel`：输出里没有型号字段。
   - `CleanInstructionEnv` = No：`$CODEX_HOME` 下的全局 `AGENTS.md`（3681 字节）去不掉（`project_doc_max_bytes=0` 无效；改 `CODEX_HOME` 会把登录态一起带走），与 grok 同一档；能去掉的（skills 说明、插件推荐）已经去掉，见要点 6。**同样去不掉的还有 `$CODEX_HOME/config.toml` 里配的 MCP 服务器**：没有整类关闭的参数（`features list` 的每一项 `--disable` 都不改 `mcp list` 的状态，零花费实测）；按名字的 `-c mcp_servers.<名>.enabled=false` 在名字不存在时让 codex 起不来，名字又来自用户自己会改的配置，不能冻进 argv。所以每个 codex 成员都带着用户配的全部 MCP 服务器；它们的工具在不在模型的工具清单里，零花费看不到 [推断：在]。与 grok 的 MCP 残留风险同一类（`grok.go` read 档那段注释）。
   - `CacheAcrossTurns` = Yes：同一 thread 第二轮 `cached_input_tokens` 从 0 到 16000（一次观测；不论 usage 是累计还是本轮，第二轮都命中了缓存）。
   - `WriteBoundary` = Unknown：workspace-write 沙箱在本机没测过；零值 Unknown 让 `validateRequest` 在 `codexPermissionArgs` 之前就拒掉 workspace / full（文本点名「no verified write boundary」）。read 档自己也有一个缺口，写在这里：MCP 工具的调用不经 shell，`sandbox_mode=read-only` 管不管得住它们零花费测不到 [推断：多半管不住]；read 档「写不了盘」即使在真机写盘探针之后，也只对 shell 成立，MCP 那一路是**残留风险**（CX1 契约 §0 第 8 条、§7）。
4. 〔`BuildTurn`〕顺序：`validateRequest` → 权限档 → 强度。只有空会话且 `Role` 非空才拼身份（身份块 + 一个换行 + 原始 Prompt，不 Trim、不改 Prompt 的任何字节）。`Stdin` 在生产路径上永不为空（`Runner` 在没有 delta 时以 `ErrNoDelta` 拒绝、不起进程；算指纹用的探针正文是非空的 `prefixProbePrompt`）；空 `Stdin` 会被 `spawnProcess` 接成空设备、codex 读到一个空 prompt —— 别为「省一次拷贝」改这条路。
5. 〔`codexPermissionArgs`〕read = `-c sandbox_mode=read-only` 加 `-c approval_policy=never`。两套形状都用 `-c` 而不是 `-s`：`exec resume` 没有 `-s`，用同一种写法两套 argv 只差 `resume <id>`，前缀摘要（按空会话形状算）就能代表续会话的参数。`-c sandbox_mode=` 被配置加载认（`debug prompt-input` 实测：换成 workspace-write / danger-full-access 时 permissions 说明跟着变，非法值 rc 1）；在真 `exec` 与 `exec resume` 上是否生效**未实测**。`approval_policy=never`：headless 没人可批，`debug prompt-input` 实测它让 permissions 说明变成「commands will be rejected」；`exec` 下默认是否本来就是 never 没核对，显式发与 claude「显式发 `--permission-mode default`」同一条理由。Windows 上的 read-only，零花费能看到的（CX1 契约 §0 第 4 条 s07-s10，2026-09-28）：`codex sandbox`（受限令牌沙箱）在 `-P :read-only` 下写盘被拒（`Access is denied.`，rc 1），`-P :workspace` 下写得出来；`codex sandbox` 不带 `-P` 时读 `-c sandbox_mode=`：`danger-full-access` 写得出（正对照），`read-only` 与不给都拒写，`workspace-write` 在不带 `-C` 的这一形里也拒写 cwd（原因未测）—— 所以在 `codex sandbox` 这条路上，`-c sandbox_mode=read-only` 选中的是拒写的沙箱；`-c sandbox_mode=read-only` 盖得过用户 `config.toml` 里的 `default_permissions`（`debug prompt-input` 的渲染）；`config.toml` 里的旧 `profile` 键已不被支持。看不到的：`exec` 与 `exec resume` 走不走同一套沙箱，**未实测**；read 档「写不了盘」在真机写盘探针（CX1 契约 §6 第 10-15 步）之前只是声明，不是实测结论。MCP 工具那一路见要点 3 的 `WriteBoundary`。workspace 不给：本机没测过 workspace-write 沙箱，续会话上的沙箱覆盖是否生效也没验证；`default` 分支正常走不到（`validateRequest` 先拦），与 grok / claude 同一体例的防御写法。
6. 〔`BuildTurn` 里的隔离参数〕`--ignore-rules`：不加载用户与项目的 execpolicy `.rules`；持久化的「允许」规则可能让命令不经批准执行 [推断]，read 档不该被它放宽；help 自述，**未实测**（`debug prompt-input` 不收它）。`--disable plugins --disable apps`：真机两轮都带着；去掉插件推荐块。`-c skills.include_instructions=false`：`debug prompt-input` 实测去掉整块 skills 说明（9080 字节，列的是用户的私人 skill 清单）；在真 `exec` 上未实测。`--disable` 的名字必须是 codex 认识的 feature：不认识的名字让 codex 起不来（`Unknown feature flag`，零花费实测），标 removed 的照收；将来某版把 `plugins` 或 `apps` 整个删掉，每一轮都会失败（codex 报错退出、stdout 为空；在 `mcp list` 与 `debug prompt-input` 上实测过，`exec` 上同样 [推断]）。不改写成 `-c features.<名>=false`：那一形对不认识的名字静默接受，写错一个字母就是静默不生效（与强度值同一类坑，见要点 2）。做不到的：全局 `AGENTS.md` 与用户配的 MCP 服务器（见要点 3 `CleanInstructionEnv`）。
7. 〔`ParseTurn`〕逐行解析 JSONL：按 `\n` 切、每行 `TrimSpace`（吃 CRLF 与空行）；任何一行不是 JSON 就整份 `ErrUnparseableOutput`（`--json` 的 stdout 只有事件行）。事件：`thread.started` 取 `thread_id`（两条不同 id 判认不出）；`item.completed` 只收 `item.type == "agent_message"`，别的条目类型（reasoning、命令执行……）不进正文；`turn.completed` 恰好一条才算成功，多于一条认不出；`turn.failed` 与 `error` 两种失败事件的**名字与形状来自 codex exec 的 JSONL 事件定义，本机零观测** [未实测]。判定顺序：有 `turn.failed` 一律失败（哪怕后面还有 `turn.completed`）；没有 `turn.completed` 时有 `error` 算失败、什么失败事件都没有就认不出（零观测，不编一个结局）；有 `turn.completed` 时 `error` 事件不改结局（它可能是中途重连的提示 [推断]）。多条 `agent_message`：正文取最后一条（与 codex 自己 `-o` 写「最后一条消息」同一个定义），前面的按 `\n\n` 连成 `Thought`（存、默认不路由，不丢），没有一条时正文为空（`CompletionEmpty`）。codex 没有截断信号，`Completion` 只有 Complete / Empty 两种。失败路径照样带回 `thread_id` 与 `UsageRaw`（钱可能花了，`runner.go:734-739` 会吸收）。非零退出码不进判定：`spawnProcess` 把 `*exec.ExitError` 收成 `err == nil`（`spawn.go:184-189`），`Runner` 只在进程没起来时判 spawn_failed（`runner.go:722-726`），结局全由 stdout 决定 —— 与另三家一致，本适配器不另起一套（`provider.go:44-49` 的约定：`ParseTurn` 不收退出码）。
8. 〔`codexUsage`〕五项全部标未报、`CostBasisNone`、金额与 `CacheCreate` 为 nil。理由：usage 很可能是 thread 累计 [推断，两个数据点分不开两种假设]；把累计值当本轮记，假设对时每轮多记一整段历史、写进只追加的事实源改不掉；标未报时假设对错都只是「不知道」，原始对象照样进 `messages.meta["usage_raw"]`，将来可以回算。**不许**把 usage 里的任何数字填进 `Usage` 的 token 字段。

#### B. `internal/provider/provider.go`（三处；B3 是 W1b 修订加的，只改注释）

B1. `Lookup` 的注释（`:74-75`）不再拿 codex 当「没有适配器」的例子。锚点（真文件 `:74-75`，恰好一次）：

```
// 没有适配器是一种正常状态，不是配置错误：配置里写了 codex 而 Step 0 从没测过它，
// council 照常启动、横幅上点名，只是那个 agent 开不了口。找不到时返回包了
```

替换为：

```
// 没有适配器是一种正常状态，不是配置错误：配置里写了一个本包没有登记的 provider 时，
// council 照常启动、横幅上点名，只是那个 agent 开不了口。找不到时返回包了
```

B2. 哨兵块（`:95-104`）最后一行 `ErrCapabilityUnknown`（`:103`）之后、`)`（`:104`）之前加一行（名字与文本冻结）：

```go
	ErrUnsupportedEffort = errors.New("effort value not supported by this provider")
```

锚点（真文件 `:103-104`，恰好一次）：

```
	ErrCapabilityUnknown = errors.New("provider capability is unknown, refusing to guess")
)
```

注释要点（写在新行上方或行尾，不许改动其它哨兵）：配置要求了一个这家 provider 不认的 effort 值；只有值被静默接受、必须由适配器自己校验的 provider 才返回它（今天只有 codex）；它与 `ErrCapabilityUnknown` 不同 —— 那个说能力位是 Unknown，这个说能力位已知、值不在本家的取值里。

B3.（W1b 修订，只改注释）包注释（`:13`）说「三家」，登记 codex 之后与代码不符。锚点（真文件 `:13` 整行，恰好一次）：

```
// Package provider 是三家 CLI 的契约层与 flag 适配器。
```

替换为：

```
// Package provider 是各家 CLI 的契约层与 flag 适配器。
```

（`:14-15` 两行不动。W1b 原型落盘后 `gofmt -l` 为空、全量九包全绿。）

`BuildTurn` 接口注释（`:36-41` 那张哨兵表）**不改**：新哨兵只有 codex 一家用，登记进 `09a` 由主代理做（§6）。

#### C. `internal/provider/registry_test.go`（既有用例，授权改五处 C1-C4；只许改这五处，其余一字不动）

原型实测：只登记 codex、不改这个文件时，这三条红：`TestRegistry_IDsAreExactlyTheThreeAdapters`（`IDs() = [agy claude codex grok]`）、`TestRegistry_LookupUnknownProvider`（`err = <nil>`）、`TestRegistry_CapabilitiesDeclareEveryField`（codex 子测试在 `:146` 的 `t.Fatalf`）。改完三条都绿，且对 §4 的 X21、X23、X24、X25、X48 有杀伤（原型实测）。

C1. `:12-22` 整段（注释 4 行 + 函数）替换为（函数改名；理由：名字说「三家」而断言四家是一句看起来正常的谎）：

```go
// TestRegistry_IDsAreExactlyTheFourAdapters: IDs() 必须按字典序返回，且不多不少
// 正好是这四个。09_STEP6_SPEC.md B 节「不写 codex.go」、K 节「codex 不在已登记
// 列表里」是 Step 6 的范围裁决；CX1 契约（2026-09-28，00_DECISIONS #45 末尾
// Griffin 裁「接入 codex」）登记了 codex，本条随之改成四家。整表相等比逐个
// 检查更强：它同时钉住了「没有第五家被意外注册」这件事。
func TestRegistry_IDsAreExactlyTheFourAdapters(t *testing.T) {
	want := []string{"agy", "claude", "codex", "grok"}
	got := provider.IDs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs() = %v, want %v (字典序)", got, want)
	}
}
```

C2. `TestRegistry_LookupUnknownProvider` 里 `:38` 与 `:43` 两行。锚点（真文件 `:38-43`，恰好一次）：

```go
	_, err := provider.Lookup("codex")
	if !errors.Is(err, provider.ErrUnknownProvider) {
		t.Fatalf("err = %v, want ErrUnknownProvider", err)
	}
	msg := err.Error()
	for _, id := range []string{"agy", "claude", "grok"} {
```

替换为：

```go
	// 这里原来查的是 codex（那时它没有适配器）；CX1 登记了 codex 之后换成一个
	// 永远不会被登记的 id，候选清单随之多一个 codex。
	_, err := provider.Lookup("no-such-provider")
	if !errors.Is(err, provider.ErrUnknownProvider) {
		t.Fatalf("err = %v, want ErrUnknownProvider", err)
	}
	msg := err.Error()
	for _, id := range []string{"agy", "claude", "codex", "grok"} {
```

C3. 三张表各加一行（放在 `"agy"` 那一行之后，gofmt 对齐后是 `"codex":  ` 两个空格）：

| 表 | 锚点（真文件，含 tab，恰好一次） | 加的一行 | 理由 |
|---|---|---|---|
| `wantClean`（`:139-143`） | `\t\t\t\t"agy":    provider.SupportUnknown,` + 换行 + `\t\t\t}` + 换行 + `\t\t\twant, ok := wantClean[id]` | `\t\t\t\t"codex":  provider.SupportNo,` | 全局 `AGENTS.md` 去不掉（§0 第 4 条） |
| `wantCache`（`:160-164`） | `\t\t\t\t"agy":    provider.SupportNo,` + 换行 + `\t\t\t}` + 换行 + `\t\t\twantC, okC := wantCache[id]` | `\t\t\t\t"codex":  provider.SupportYes,` | 第二轮 cached 0 → 16000（§0 第 3 条） |
| `wantWriteBoundary`（`:181-185`） | `\t\t\t\t"agy":    provider.SupportYes,` + 换行 + `\t\t\t}` + 换行 + `\t\t\twantWB, okWB := wantWriteBoundary[id]` | `\t\t\t\t"codex":  provider.SupportUnknown,` | 未实测（§1.1 A 注释要点 3） |

（`\t` 是真 tab。锚点带上 `want…` 那一行是为了三处恰好各一次：`"agy":    provider.SupportNo,` 单独一行在 `wantCache` 里出现一次，但带上下一行才不会与别处相混。）

`TestRegistry_OnlyClaudeReportsCost`（`:201-213`）不用改：codex 是 `SupportNo`，照样绿（原型实测）。

C4.（v1.1，W1 F6）**只改注释**，五处，登记 codex 之后它们说的话与代码不符。锚点全部从 HEAD 真文件逐字复制（`\t` 是真 tab），各自在文件里恰好一次；替换后的文本冻结。v1.1 原型落盘后 `gofmt -l` 为空、`go test ./internal/provider` 全绿，`diff` 相对 HEAD 只有 C1-C4 这几块。

| # | 位置（HEAD） | 锚点（整行，含行首） | 替换为 |
|---|---|---|---|
| C4-1 | `:50` | `// TestRegistry_IDMatchesRegistrationKey: 三家的 ID() 必须与它注册时用的 key` | `// TestRegistry_IDMatchesRegistrationKey: 四家的 ID() 必须与它注册时用的 key` |
| C4-2 | `:72` | `// 看到干净的三家。` | `// 看到干净的四家。` |
| C4-3 | `:89` | `// 例外——三家在这两位上分别给出了各自的实测结论（不是清一色 Unknown），` | `// 例外——四家在这两位上分别给出了各自的实测结论（不是清一色 Unknown），` |
| C4-4 | `:157-158`（两行） | `\t\t\t// 按轮数涨而 cache_read 恒为 0（依据逐条写在各适配器 Capabilities()` + 换行 + `\t\t\t// 的注释里）。新增第四家 provider 而没有为这一位给出实测结论时，` | 三行：`\t\t\t// 按轮数涨而 cache_read 恒为 0，codex 见过同一 thread 第二轮` + 换行 + `\t\t\t// cached_input_tokens 从 0 到 16000（依据逐条写在各适配器 Capabilities()` + 换行 + `\t\t\t// 的注释里）。再新增 provider 而没有为这一位给出实测结论时，` |
| C4-5 | `:176-177`（两行） | `\t\t\t// 验收第 1 条）：claude / agy 实测围得住，grok 实测围不住。` + 换行 + `\t\t\t// 新增第四家 provider 而没有为这一位给出实测结论时，必须在这里` | 两行：`\t\t\t// 验收第 1 条）：claude / agy 实测围得住，grok 实测围不住，codex 未测、` + 换行 + `\t\t\t// 明确记为 Unknown（CX1）。再新增 provider 而没有为这一位给出实测结论时，必须在这里` |

不改的：`:134-137`「CleanInstructionEnv 在 2026-09-21 之前三家都是 Unknown …… 隔离实测做完之后三家分出了三种取值」是当时的历史叙述，今天仍然成立；`:88` 的「两个例外」是 11c 加 `WriteBoundary` 那一段时留下的旧说法，不是本刀造成的，不在授权范围。别的测试文件里随 codex 登记而过期的注释（`validate_internal_test.go:6-9`、`prefix_digest_test.go:10`、`:21`、`:76`）不授权改，见 §7 待主代理第 14 条（W1b 修订又补了一批，同在那一条；`mdx_discovery_test.go` P1 里的两处「第四家」归 §1.1 D2，`provider.go:13` 归 §1.1 B3）。

#### D. MD1 已落地（`80a9385`），本节生效（主代理 2026-09-28 裁；W1b 修订把 D2 从真文件冻结）

D1. `codex.go` 的 `Capabilities()` 字面量里，`WriteBoundary:        SupportUnknown,` 那一行之后、字面量的 `}` 之前加**至少一行注释**再加：

```go
		ModelDiscovery: ModelDiscoveryUnknown,
```

（与 MD1 A2 同一体例：中间隔着注释行，gofmt 不把它与上面那组对齐，保持冒号后一个空格；v1.1 原型用一份模拟的 `ModelDiscovery` 类型实测过 gofmt 不改这一行，W1b 修订在 `80a9385` 原型里用真类型、一行注释再落一次，`gofmt -l` 仍为空。）注释要点：`codex debug models` 能列出模型目录（2026-09-28 实测，JSON 含 slug / display_name / supported_reasoning_levels / visibility），但识别框架还没接它（#45 裁「接入 codex」在两个功能之后，识别是再下一刀）；在接上之前声明 Unknown，`Available()` 为假，`discoverableProviders` 跳过它，`/add` 不会多出一行 codex 的失败。**不许**声明成 `ModelDiscoverySubcommand`：`DiscoveryCommandFor` / `ParseDiscovery` 不认 codex，`/add` 会多一行「识别失败」。

D2. `internal/provider/mdx_discovery_test.go` 的 P1 `TestMdxCapabilitiesDeclareModelDiscovery`（`:199-247`，既有用例，授权改四处 D2-1 至 D2-4，**只许改这四处**；这个文件的其余用例一字不动，P1 里三家原有的三条断言与 `Available()` 那几条一字不动）。W1b 修订：锚点从 `80a9385` 的真文件逐字取出，各自在文件里恰好一次（原型核过）；替换后的文本冻结。代码块里的缩进是真 tab。v1.1 的「锚点 W2 时按真文件取」与引 `md1_contract.md:1591` 的那句作废。

D2-1 用例注释（`:201-202`）补 codex 一句，「第四家」改掉。锚点：

```
// 【判别力】claude 的能力位改成 Unknown（M22）时 claude 那一项不等；
// 登记第四家 provider 而不在这里声明它的识别方式时 Fatalf。
```

替换为：

```
// 【判别力】claude 的能力位改成 Unknown（M22）时 claude 那一项不等；
// 再新增 provider 而不在这里声明它的识别方式时 Fatalf。
// CX1 登记了 codex，声明 Unknown（识别框架还没接它，理由见 codex.go）；
// codex.go 写成 Subcommand（CX1 X58）时 codex 那一条不等。
```

D2-2 `want` 表加一行（`:208-210`；gofmt 对齐后是 `"codex":  ` 两个空格）。锚点：

```
		"grok":   provider.ModelDiscoverySubcommand,
	}
	for _, id := range provider.IDs() {
```

替换为：

```
		"grok":   provider.ModelDiscoverySubcommand,
		"codex":  provider.ModelDiscoveryUnknown,
	}
	for _, id := range provider.IDs() {
```

D2-3 `Fatalf` 文本里的「加第四家」（`:212`）。锚点：

```
			t.Fatalf("登记了 provider %q 却没有在这里声明它的 ModelDiscovery 预期取值；加第四家必须同时声明这一位", id)
```

替换为：

```
			t.Fatalf("登记了 provider %q 却没有在这里声明它的 ModelDiscovery 预期取值；再新增 provider 必须同时声明这一位", id)
```

D2-4 codex 的断言，加在 `// §3.1 冻结的取值`（`:225`）之前。锚点（整行）：

```
	// §3.1 冻结的取值
```

替换为（末尾空一行，再接原来那一行）：

```
	// CX1：codex 声明 Unknown。want 表里的 codex 一项只让上面的 Fatalf 放行，
	// 取值由下面这一条钉住（与上面三家的循环分开写，那三条一字不动）。
	pc, err := provider.Lookup("codex")
	if err != nil {
		t.Fatalf("Lookup(%q): %v", "codex", err)
	}
	if got := pc.Capabilities().ModelDiscovery; got != provider.ModelDiscoveryUnknown {
		t.Errorf("codex 的 ModelDiscovery = %d，期望 %d", got, provider.ModelDiscoveryUnknown)
	}

	// §3.1 冻结的取值
```

【判别力】（W1b 在 `80a9385` 原型实测）不落 D2 时，全量九包只有这条红，红在 `:212` 的 `Fatalf`（`登记了 provider "codex" 却没有在这里声明……`）；落了 D2 全绿。X58（`codex.go` 写成 `ModelDiscoverySubcommand`）让 D2-4 那一条红（`codex 的 ModelDiscovery = 1，期望 0`），同时 §2.1 P1 红，其余包全绿。D2-4 直接比常量 `provider.ModelDiscoveryUnknown`，不经 `want` 表：`want` 里的 codex 一项只管放行。D2-1、D2-3 只改注释与 `Fatalf` 文本，不改判据。

### 1.2 不做（两条腿都不许越界）

- 不接模型识别（`codex debug models` 进 MD1 的识别框架是 MD1 落地之后的另一刀）；不改 `discovery*.go`、`addmodel.go`。
- 不改另外三家适配器（`claude.go`、`grok.go`、`agy.go`）与 `capabilities.go`、`turn.go`、`uuid.go`。
- 不改 `internal/turn`、`internal/repl`、`internal/app`、`internal/tui`、`internal/config`、`internal/store`、`internal/conversation`、`cmd/` 的任何生产代码：`budget.go` 的命令行余量、`spawn.go` 的换行闸与 stdin、`runner.go` 的解析吸收与漂移判定，codex 都照既有的一套走，不需要改（v1 原型实测：只加 `codex.go` 与 `provider.go` 两处，全量既有用例除 §1.1 C 那三条外全绿；W1b 在 `80a9385` 原型上，落 A、B、C 而不落 D2 时，全量只有 MD1 的 P1 红）。
- 不加 workspace / full 档；不加 `-C`、`-o`、`--ephemeral`、`--ignore-user-config`、`-c include_environment_context=false`、`-c include_permissions_instructions=false`、`-c developer_instructions=`（理由在 §1.1 A 注释要点 1、5、6 与 §0 第 4 条）。
- 不加任何关 MCP 的参数（v1.1）：不加 `-c mcp_servers.<名>.enabled=false`（名字不存在时 codex 起不来，§0 第 4 条 m03；名字来自用户会改的配置，是黑名单），不加 `-c mcp_servers={}`（不生效，V0）；整类的开关不存在（m02、m02b、m05）。MCP 是残留风险，见 §0 第 8 条、§7 待 Griffin 第 5 条。
- 不记任何 token 数（§0 第 6 条）；不加 schema 列、不做迁移。
- 不碰 Griffin 的 `C:\Users\25142\.council\config.toml`（配置示例给在 §6，由他自己加）；不碰 `~/.codex` 下的任何文件。
- 不跑任何 `codex exec`、不发任何 prompt；测试里一律不起真 codex（外部命令只经 turn 包的 `fakeSpawner` 或 app 包的假 CLI）。
- **既有 `_test.go` 只许改 §1.1 C 的五处（C1-C4）与 §1.1 D2 的四处（D2-1 至 D2-4，都在 `mdx_discovery_test.go` 的 P1 里）**；其余一个字不动（含注释）。生产代码的既有文件只许改 `provider.go` 的 B1-B3。
- 测试腿不写生产代码、不改既有 `_test.go`；实现腿除 §1.1 C 与 D2 外不写 `_test.go`。

---

## 2. 验收（测试腿新写的文件）

通用纪律：

- 新文件共三个（§2.1-2.3）；包级标识符（类型、函数、变量、常量）一律以 `cxx` 开头，测试函数名 `TestCxx...`。不新写 `TestMain`：turn 包的新文件不依赖任何 `TestMain`；app 包复用 `smoke_test.go:40` 的。
- 不起真 provider：turn 包一律 `fakeSpawner`（`fixtures_test.go:170-194`）；app 包一律测试二进制自己当假 CLI（`COUNCIL_FAKE_CLI=1`、`S10D_FAKE_STDOUT_FILE`，`smoke_test.go:53-66`）。store 一律开在 `t.TempDir()` 之下，不碰 `C:\Users\25142\.council`。app 包的用例必须同时设 `COUNCIL_GLOBAL_RULES` 指向一个不存在的文件（`s10d_log_marks_e2e_test.go:107` 的体例），否则会读到真全局规则文件。
- **夹具字节**：两份真机原件逐字节写成常量（CRLF 原样），名字 `cxxExec1CRLF` / `cxxExec2CRLF`（每个测试文件各定义一份；三个文件在三个包里，不能共用）：

  ```
  cxxExec1CRLF = "{\"type\":\"thread.started\",\"thread_id\":\"01a0e373-ac6c-7083-87d7-62536800ada0\"}\r\n" +
  	"{\"type\":\"turn.started\"}\r\n" +
  	"{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"agent_message\",\"text\":\"PONG\"}}\r\n" +
  	"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":16184,\"cached_input_tokens\":0,\"cache_write_input_tokens\":0,\"output_tokens\":6,\"reasoning_output_tokens\":0}}\r\n"

  cxxExec2CRLF = "{\"type\":\"thread.started\",\"thread_id\":\"01a0e373-ac6c-7083-87d7-62536800ada0\"}\r\n" +
  	"{\"type\":\"turn.started\"}\r\n" +
  	"{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"agent_message\",\"text\":\"KIWI\"}}\r\n" +
  	"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":32396,\"cached_input_tokens\":16000,\"cache_write_input_tokens\":0,\"output_tokens\":12,\"reasoning_output_tokens\":0}}\r\n"
  ```

  两份 usage 子对象原文（`UsageRaw` 与 `meta.usage_raw` 的期望值）：`cxxUsage1 = {"input_tokens":16184,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":6,"reasoning_output_tokens":0}`、`cxxUsage2 = {"input_tokens":32396,"cached_input_tokens":16000,"cache_write_input_tokens":0,"output_tokens":12,"reasoning_output_tokens":0}`。thread id 常量 `cxxThreadID = "01a0e373-ac6c-7083-87d7-62536800ada0"`。`turn.failed` / `error` 两种失败形状**没有真机原件**，§2.1 P6 与 §2.2 T2 里是按 codex exec 的事件定义合成的，注释里写明「合成，未实测」。
- **argv 期望值的生成函数**（每个用到它的文件各定义一份，名字 `cxxArgs(session, effort string) []string`）：`["exec"]`，`session` 非空时接 `"resume", session`，再接 `"--json", "--skip-git-repo-check", "-c", "sandbox_mode=read-only", "-c", "approval_policy=never", "-m", <model>`，`effort` 非空时接 `"-c", "model_reasoning_effort="+effort`，最后接 `"--ignore-rules", "--disable", "plugins", "--disable", "apps", "-c", "skills.include_instructions=false", "-"`。§2.1 与 §2.2 的 `<model>` 都是 `"gpt-6-sol"`；§2.2 T4 另写整表字面量。逐字节期望见 §3.1、§3.2。
- 断言一律整值相等：`reflect.DeepEqual` 比整个 `provider.Command` / `provider.TurnResult` / `provider.Capabilities`，错误比 `err.Error()` 整串，库里的行逐列比。不写子串断言；唯一例外是 `Fatalf` 前提守卫（前提不成立即停、不是被测断言）。
- 每条注释写【判别力】：它能不能为假、只差一处的对照是哪条。凡断言「没有 / 为 NULL / 不含」的，必须有同一装置下只差一个条件的正对照（下面逐条写了是哪条）。
- 允许只读引用同包既有测试辅助（不许改它们）：
  - provider（`package provider_test`）：只用导出标识符。
  - turn（`package turn`，白盒）：`newRealTurnStore` / `realTurnStore.rawDB`（`step10a3_fixtures_test.go:51`、`:70`）、`newTestConfig`（`fixtures_test.go:232`）、`newTestRunner`、`newConv`、`joinAgent`、`postUser`、`fakeSpawner`（`enqueue` / `enqueueSuccess` / `calls`）、`spawnResult`、`claudeT1JSON`、`testAgentClaude`（`"alice"`）、`testAgentAgy`（`"nova"`）、`prefixDigestGoldenReq`、`prefixDigestGoldenWorkDir`、`goldenDigestFor`、`assertArgsEqual`（`prefix_digest_test.go:145-296`）、`normalizedPrefixArgs`、`PrefixArgsDigest`、`CommandLineBytes`、`maxCommandLineBytes`、`Runner.workDir` 字段、`Record` 的全部字段与 `MeasuredCold`、`ErrIdentityDrift`、`Outcome*`、`Plan`、`(*Runner).Budget` / `SetConfig` / `Ask`。
  - app（`package app`）：`isolateAppEnv`、`tomlLiteral`、`writeTestConfig`、`runAppForTest`（`app_test.go:42-111`）。
- **HEAD 上的状态**：§2.1、§2.2 引用新标识符 `provider.ErrUnsupportedEffort`，在 HEAD 上整包编译不过，这是预期的红。**§2.3 只用 HEAD 已有标识符**，测试腿交付前必须在 HEAD 的隔离拷贝里**只放这一个新文件**跑一次，报告里写出命令与每个子测试的红/绿。原型实测的结果（v1 在 `2a8737f` 的隔离拷贝上）：`same_config`、`effort_changed` 两个子测试都红（agent 消息为空、`session_opening` 0 条、`provider_sessions` 没有行 —— `/ask sol` 以 `ErrUnknownProvider` 被拒）。`80a9385` 上 codex 同样没有登记，[推断] 结果相同；W1b 修订没有在 `80a9385` 上复跑，测试腿在 `80a9385` 的隔离拷贝上跑、照实报告。

### 2.1 `internal/provider/cxx_codex_test.go`（`package provider_test`）

共用：`cxxCodex(t)` 取 `provider.Lookup("codex")`（失败 `Fatalf`）；`cxxReq()` 返回
`provider.TurnRequest{AgentID: "sol", DisplayName: "Sol", Model: "gpt-6-sol", Effort: "low", Role: "R1\nR2", Permission: provider.PermissionRead, Prompt: "P1\n\nP2", WorkDir: `C:\cxx\wd`}`；未报清单 `cxxUnreported = []string{"input_tokens", "cache_create", "cache_read", "output_tokens", "thinking_tokens"}`；成功结果的建造函数 `cxxOK(text, thought, session, usage string) provider.TurnResult`：`Text`、`Thought`、`ProviderSession` 照填，`StopRaw: "turn.completed"`，`Usage: provider.Usage{CostBasis: provider.CostBasisNone, Unreported: cxxUnreported}`，`Completion` 在 `text == ""` 时是 `CompletionEmpty`、否则 `CompletionComplete`，`usage` 非空时 `UsageRaw = json.RawMessage(usage)`（空串时留 nil），其余字段零值。

**P1 `TestCxxCapabilities`**：`reflect.DeepEqual(Capabilities(), want)`，`want` 是 §3.6 的整个字面量（不写 `ModelDiscovery`：这个字段的零值就是 `ModelDiscoveryUnknown`，与 D1 声明的值相等）。【判别力】任一字段改一档都红（X20-X27）；D1 写成 `ModelDiscoverySubcommand` 也红（X58，W1b 在 `80a9385` 原型实测）。

**P2 `TestCxxBuildTurnShapes`**，六个子测试，每个从 `cxxReq()` 改一处后 `BuildTurn`，`err == nil`，整个 `Command` 与下面的 `want` `reflect.DeepEqual`（`want` 只填 `Args`、`Dir: `C:\cxx\wd``、`Stdin`、`Transport: provider.PromptTransportStdin`、`Dropped`；`Path`、`Env`、`Files`、`PlannedSessionID` 留零值）：

| 子测试 | 改动 | `Args` | `Stdin`（Go 字符串字面量） | `Dropped` |
|---|---|---|---|---|
| `fresh` | 无 | `cxxArgs("", "low")` | `"R1\nR2\nP1\n\nP2"` | nil |
| `resume` | `ProviderSession = cxxThreadID` | `cxxArgs(cxxThreadID, "low")` | `"P1\n\nP2"` | nil |
| `fresh_no_role` | `Role = ""` | `cxxArgs("", "low")` | `"P1\n\nP2"` | nil |
| `fresh_no_effort` | `Effort = ""` | `cxxArgs("", "")` | `"R1\nR2\nP1\n\nP2"` | nil |
| `resume_no_effort` | `Effort = ""`、`ProviderSession = cxxThreadID` | `cxxArgs(cxxThreadID, "")` | `"P1\n\nP2"` | nil |
| `max_turns` | `MaxTurns = 3` | `cxxArgs("", "low")` | `"R1\nR2\nP1\n\nP2"` | `[]provider.DroppedOption{{Option: "max_turns", Value: "3", Reason: "this provider has no max-turns flag"}}` |

【判别力】`resume` 与 `fresh` 只差会话 id：身份只在空会话拼（X06）；`fresh_no_role` 与 `fresh` 只差 Role：空 Role 不拼、不留前导换行（X07）；分隔符是一个换行（X08）；正文不进 argv（X09、X56）；`Env` / `Files` 为 nil 由整值比较钉住。

**P3 `TestCxxBuildTurnEffort`**：
- 七个合法值 `minimal, low, medium, high, xhigh, max, ultra` 逐个：`err == nil`，`Args` 等于 `cxxArgs("", e)`。
- 六个非法值 `"bogus", "LOW", "none", " high", "high ", "effort-golden"` 逐个：`errors.Is(err, provider.ErrUnsupportedEffort)`，返回的 `Command` 等于零值 `provider.Command{}`。
- `Effort = "bogus"` 时 `err.Error()` 整串等于 §3.5 第 1 行把 `%q` 代成 `"bogus"` 之后的文本。
【判别力】合法值表的两端（`minimal`、`ultra`）各自单独负责一条变异（X03、X02）；`LOW` 负责大小写（X04）；首尾空格两条防 Trim；对照是合法值那一段。

**P4 `TestCxxBuildTurnTiers`**：`cxxReq()` 改 `Permission`：
- `provider.PermissionWorkspace` → `err.Error() == "permission tier \"workspace\": no verified write boundary: permission tier not supported by this provider"`；
- `provider.PermissionFull` → `"permission tier \"full\": no verified write boundary: permission tier not supported by this provider"`；
- `""` → `"permission tier \"\": permission tier not supported by this provider"`；
三者都 `errors.Is(err, provider.ErrUnsupportedTier)`，`Command` 为零值。另：`WorkDir = ""` → `errors.Is(err, provider.ErrNoWorkDir)`。
【判别力】前两条的文本来自 `validateRequest` 的写入边界闸（`provider.go:136-138`），证明拒绝发生在 `codexPermissionArgs` 之前（X53：去掉 `validateRequest` 后文本变成「codex adapter offers read only」）；`WriteBoundary` 若声明成 Yes，workspace 这条的文本变（X23）。

**P5 `TestCxxParseFixtures`**，三个输入，每个 `err == nil` 且整个 `TurnResult` `reflect.DeepEqual`：
- `cxxExec1CRLF` → `cxxOK("PONG", "", cxxThreadID, cxxUsage1)`；
- 同一份去掉全部 `\r` → 同上；
- `cxxExec2CRLF` → `cxxOK("KIWI", "", cxxThreadID, cxxUsage2)`。
【判别力】usage 的任何一个数被填进 `Usage`、或未报清单少一项，都红（X39、X40）；`UsageRaw` 取成整行而不是 usage 子对象也红（X41）。它的对照是 §2.2 T1 里同一装置下的 claude 成员（token 列非 NULL）。

**P6 `TestCxxParseShapes`**，子测试表（合成输入，注释写「合成，未实测」）。记号：`th = {"type":"thread.started","thread_id":"T1"}\n`；`done = {"type":"turn.completed","usage":{"input_tokens":1}}\n`；`msg(id, typ, text) = {"type":"item.completed","item":{"id":"<id>","type":"<typ>","text":"<text>"}}\n`；`U1 = {"input_tokens":1}`；`fail(stop, usage)` = `TurnResult{ProviderSession: "T1", StopRaw: stop, Usage: {CostBasisNone, cxxUnreported}}`、`usage` 非空时 `UsageRaw = usage`、`Completion` 零值（`CompletionUnknown`）。每个子测试：期望哨兵为 nil 时 `err == nil`，否则 `errors.Is(err, 哨兵)`；再整个 `TurnResult` `reflect.DeepEqual`。

| 子测试 | 输入 | 哨兵 | `TurnResult` |
|---|---|---|---|
| `multi_msg` | `th + msg(item_0, agent_message, A) + msg(item_1, reasoning, R) + msg(item_2, agent_message, B) + msg(item_3, agent_message, C) + done` | nil | `cxxOK("C", "A\n\nB", "T1", U1)` |
| `zero_msg` | `th + done` | nil | `cxxOK("", "", "T1", U1)` |
| `reasoning_only` | `th + msg(item_0, reasoning, R) + done` | nil | `cxxOK("", "", "T1", U1)` |
| `error_then_completed` | `th + {"type":"error","message":"retry"}\n + msg(item_0, agent_message, OK) + done` | nil | `cxxOK("OK", "", "T1", U1)` |
| `null_usage` | `th + {"type":"turn.completed","usage":null}\n` | nil | `cxxOK("", "", "T1", "")` |
| `no_usage` | `th + {"type":"turn.completed"}\n` | nil | `cxxOK("", "", "T1", "")` |
| `blank_line_inside` | `th + "\r\n" + msg(item_0, agent_message, X) + " \r\n" + done` | nil | `cxxOK("X", "", "T1", U1)` |
| `same_thread_twice` | `th + th + msg(item_0, agent_message, X) + done` | nil | `cxxOK("X", "", "T1", U1)` |
| `turn_failed` | `th + {"type":"turn.started"}\n + {"type":"turn.failed","error":{"message":"boom"}}\n` | `ErrTurnFailed` | `fail("turn.failed", "")` |
| `failed_and_completed` | `th + msg(item_0, agent_message, OK) + {"type":"turn.failed","error":{"message":"late"}}\n + done` | `ErrTurnFailed` | `fail("turn.failed", U1)` |
| `error_only` | `th + {"type":"error","message":"stream err"}\n` | `ErrTurnFailed` | `fail("error", "")` |
| `no_completed` | `th + {"type":"turn.started"}\n` | `ErrUnparseableOutput` | 零值 |
| `two_completed` | `th + done + done` | `ErrUnparseableOutput` | 零值 |
| `two_threads` | `th + {"type":"thread.started","thread_id":"T2"}\n + done` | `ErrUnparseableOutput` | 零值 |
| `nonjson_line` | `th + "Reading prompt from stdin...\n" + done` | `ErrUnparseableOutput` | 零值 |
| `unknown_only` | `{"type":"something.else"}\n` | `ErrUnparseableOutput` | 零值 |
| `empty` | `" \r\n\n"` | `ErrUnparseableOutput` | 零值 |
| `no_thread` | `{"type":"turn.started"}\n + msg(item_0, agent_message, X) + done` | `ErrNoSessionID` | `cxxOK("X", "", "", U1)` |
| `item_without_item`（v1.1） | `th + {"type":"item.completed"}\n + done` | nil | `cxxOK("", "", "T1", U1)` |
| `second_thread_empty`（v1.1） | `th + {"type":"thread.started","thread_id":""}\n + done` | `ErrUnparseableOutput` | 零值 |
| `turn_failed_no_error`（v1.1；W1b 修订改了输入） | `th + {"type":"turn.started"}\n + {"type":"turn.failed"}\n` | `ErrTurnFailed` | `fail("turn.failed", "")` |

（共 21 行。v1.1 的三行期望值取自原型实跑：`item_without_item` 的 `err` 为 nil、结果与 `zero_msg` 逐字段相同；`second_thread_empty` 的 `err.Error()` 是 `codex: two thread.started events with different thread_id ("T1", ""): provider output is not a known success shape`、结果零值。`turn_failed_no_error` 的输入 W1b 修订补了 `turn.started` 一行，使它与 `turn_failed` 只差 `error` 对象（v1.1 的输入少这一行，差了两处）；期望不变，W1b 在 `80a9385` 原型实跑：`err.Error()` 是 `codex: turn failed (message=""): provider reported a failed turn`、结果 `fail("turn.failed", "")`。P7 的 `turn_failed_no_error` 一行输入**不同**（没有 `turn.started`），它的只差一处的对照是 P7 第 6 行，见 P7。）

【判别力】成对的对照：`multi_msg` 与 `same_thread_twice`（一条 vs 三条 agent_message：取最后一条 X28、Thought X29、全部拼接 X30、收了 reasoning X31）；`reasoning_only` 与 `zero_msg`（只差一条非 agent_message 条目）；`error_then_completed` 与 `error_only`（只差有没有 `turn.completed`：X36）；`failed_and_completed` 与 `turn_failed`（只差后面有没有 `turn.completed`：X35；它还是「失败路径带回 `UsageRaw`」唯一的非空输入：X50）；`null_usage` 与 `zero_msg`（只差 usage 是不是 null：X42）；`no_completed` 与 `error_only`（只差有没有 `error` 事件：X37）；`blank_line_inside` 与 `same_thread_twice`（只差中间的空行：X32）。v1.1 三行（W1 F3，V3 更正）：`item_without_item` 与 `zero_msg`（只差多一条不带 `item` 的 `item.completed`）：去掉 `ev.Item != nil` 判据后在这一行空指针 panic（X59），v1 的全部输入里没有一条不带 `item`；`second_thread_empty` 与 `two_threads`（只差第二个 id 是空串还是 `T2`）：「第二个 id 为空就放过」（X60）让它变成 `ErrNoSessionID`、结果非零值，而 `two_threads` 照旧 —— v1 只有 `two_threads` 时这条变异存活；`turn_failed_no_error` 与 `turn_failed`（只差 `turn.failed` 带不带 `error` 对象，两边都有 `turn.started`；W1b 修订补齐）：去掉 `ev.Error != nil` 判据后在这一行 panic（X61），而 `turn_failed` 照旧绿（W1b 在 `80a9385` 原型实测），v1 的每一条 `turn.failed` 输入都带着 `error`。三行在 v1.1 原型上用 `-skip` 去掉之后，X59-X61 在 provider / turn / app 三个包的 `TestCxx|TestRegistry|TestPrefixArgs` 下全部存活（v1.1 原型实测；W1b 复核了 provider 包那一半，X61 在改过输入后又在 provider 包复核一次）。

**P7 `TestCxxParseErrorTexts`**：十二个输入（v1 九个、v1.1 三个）各自 `err.Error()` 整串相等（§3.5 第 3-11 行）：

| 输入 | 期望 `err.Error()` |
|---|---|
| `""` | §3.5 第 3 行 |
| `th + "oops\n"` | §3.5 第 4 行（`line 2`） |
| `{"type":"x"}\n` | §3.5 第 5 行 |
| `th + done + done` | §3.5 第 6 行（`2 turn.completed`） |
| `th + {"type":"thread.started","thread_id":"T2"}\n` | §3.5 第 7 行（`("T1", "T2")`） |
| `th + {"type":"turn.failed","error":{"message":"boom"}}\n` | §3.5 第 8 行（`message="boom"`） |
| `th + {"type":"error","message":"stream err"}\n` | §3.5 第 9 行（`error="stream err"`） |
| `th` | §3.5 第 10 行 |
| `done` | §3.5 第 11 行 |
| （v1.1，`turn_failed_no_error`）`th + {"type":"turn.failed"}\n` | `codex: turn failed (message=""): provider reported a failed turn` |
| （v1.1，`two_failed`）`th + {"type":"turn.failed","error":{"message":"first"}}\n + {"type":"turn.failed","error":{"message":"second"}}\n` | `codex: turn failed (message="first"): provider reported a failed turn` |
| （v1.1，`two_errors`）`th + {"type":"error","message":"first"}\n + {"type":"error","message":"second"}\n` | `codex: turn failed (error="first"): provider reported a failed turn` |

（v1.1 三行的期望值是原型实跑的整串；子测试名写在括号里，测试腿照用，§4 的杀手按它点名。v1 的九行子测试名由测试腿自定。）

【判别力】这些文本经 `diagnose` 进 `Record.Err`、经 `askCause` 的默认分支上屏（`/ask` 与 `/round` 都印），是用户可见字面量；「认不出任何事件」与「没有 turn.completed」同一个哨兵，只有文本分得开（X45 只有本条杀得到）。v1.1 三行：`turn_failed_no_error` 与本表第 6 行（`message="boom"`）只差 `error` 对象在不在，钉「没有 error 对象时 message 是空串、不 panic」（X61 的第二个杀手）；`two_failed`、`two_errors` 钉「两条失败事件时第一条胜出」：失败信息只进错误文本、不进 `TurnResult`，P6 的哨兵加整值比较杀不到「最后一条胜出」（X62、X63），只有整串文本杀得到；它们的对照是本表第 6、7 行（只有一条失败事件）。

### 2.2 `internal/turn/cxx_codex_runner_test.go`（`package turn`，白盒；真 store：`newRealTurnStore`）

共用配置 `cxxCfg()`：`newTestConfig()` 的一份，再加 `Providers["codex"] = config.ProviderConfig{Command: "codex-fake"}` 与 agent `"sol"`：`DisplayName "Sol"`、`Provider "codex"`、`Model "gpt-6-sol"`、`Effort "low"`、`Role "只做反例检查"`、`Permissions config.PermissionRead`、`Mode config.ModeDiscussion`。`newTestConfig` 里的 alice（claude）、nova（agy）原样保留，用作对照。

库里一行 `turn_usage` 的读法（名字 `cxx...`）：按 `turn_id` 查 `provider_session_id, outcome, input_tokens, cache_create, cache_read, output_tokens, thinking_tokens, cost_usd, prompt_bytes, identity_fp`，可空列用 `sql.NullInt64` / `sql.NullFloat64`。

**T1 `TestCxxAskTwoTurnsRealStore`**（本刀的主验收：真 Runner + 真 store，第一轮开会话、第二轮续会话）。
前置：`rs := newRealTurnStore(t)`；`r := newTestRunner(t, cxxCfg(), rs.svc)`；`fs := &fakeSpawner{}`、`r.spawn = fs.spawn`；`conv, _ := rs.svc.Create(ctx, "cxx")`（标题必须是 `cxx`，它进 stdin）；`joinAgent` `sol` 与 `alice`；`postUser("hello-cxx")`。
1. `fs.enqueueSuccess(cxxExec1CRLF)`；`rec1, err := r.Ask(ctx, convID, "sol")`：`err == nil`、`rec1.Outcome == OutcomeSpoke`（前提守卫）。`c1 := fs.calls[0].cmd`：
   - `c1.Args` 等于 `cxxArgs("", "low")`；
   - `string(c1.Stdin)` 整串等于 `"You are Sol (id=sol).\nProvider: codex\n只做反例检查\ncxx\n\nuser: hello-cxx"`；
   - `c1.Path == "codex-fake"`、`c1.Dir == filepath.Join(r.workDir, "sol")`、`c1.Transport == provider.PromptTransportStdin`、`c1.Files == nil`、`c1.Env == nil`；
   - `rec1.Fresh`、`!rec1.Drifted`、`rec1.ProviderSession == cxxThreadID`、`rec1.Message.Text == "PONG"`、`rec1.Message.Thought == ""`、`rec1.PromptBytes == len("cxx\n\nuser: hello-cxx")`（prompt_bytes 只量 delta，不含身份块）。
2. 对照成员：`fs.enqueueSuccess(claudeT1JSON)`；`recA, _ := r.Ask(ctx, convID, "alice")`，`OutcomeSpoke`（前提守卫）。
3. `postUser("second-cxx")`；`fs.enqueueSuccess(cxxExec2CRLF)`；`rec2, err := r.Ask(ctx, convID, "sol")`：`err == nil`、`OutcomeSpoke`（前提守卫）。`c2 := fs.calls[2].cmd`：
   - `c2.Args` 等于 `cxxArgs(cxxThreadID, "low")`；
   - `string(c2.Stdin)` 整串等于 `"alice: OK\n\nuser: second-cxx"`（续会话：不拼身份、跳过自己上一轮的发言）；
   - `!rec2.Fresh`、`!rec2.Drifted`、`rec2.ProviderSession == cxxThreadID`、`rec2.Message.Text == "KIWI"`。
4. 库（`rs.rawDB(t)`）：
   - `provider_sessions WHERE conv_id = convID AND agent_id = 'sol'`：`provider_session_id == cxxThreadID`、`identity_fp == §3.7 的 cxxFP`、`spoken == 1`、`cursor_seq == rec2.ScannedTo`；
   - `rec1`、`rec2` 各自那一行 `turn_usage`：`provider_session_id == cxxThreadID`、`outcome == 1`、`input_tokens` / `cache_create` / `cache_read` / `output_tokens` / `thinking_tokens` / `cost_usd` **全部 NULL**、`identity_fp == cxxFP`、`prompt_bytes == rec.PromptBytes`；
   - `recA` 那一行 `turn_usage.input_tokens` 非 NULL 且等于 `47751`（claude 的三项相加，`claude.go:293`）；
   - `sol` 的两条消息（`SELECT meta FROM messages WHERE sender_id = 'sol' ORDER BY seq`，第 1、2 条）的 `meta` 解成 `map[string]json.RawMessage`：`usage_raw` 的原始字节分别等于 `cxxUsage1`、`cxxUsage2`；`usage_unreported` 等于 `["input_tokens","cache_create","cache_read","output_tokens","thinking_tokens"]`；`effort_policy` 等于 `"per_turn_flag"`；`effort_flag_omitted` 等于 `false`；`stop_raw` 等于 `"turn.completed"`；`model_source` 等于 `"requested"`（都比 JSON 原文，含引号）。
5. `rec2.MeasuredCold()` 返回 `known == false`。
【判别力】token 列全 NULL 的正对照是同一会话、同一 Runner、同一库里的 alice 那一行（只差 provider）；stdin 不含身份的正对照是第 1 步（只差是不是首轮）；`effort_flag_omitted == false` 与 `effort_policy` 钉住 `Capabilities.Effort`（X20）；argv / stdin 的整串比较杀 X06-X18、X55-X57。

**T2 `TestCxxAskFailureOutcomes`**，三个子测试（下表两个失败形状，外加表后的正对照 `exit1_success`），每个用全新的 `newRealTurnStore`、`newTestRunner(t, cxxCfg(), ...)`、`newConv`、`joinAgent("sol")`、`postUser("hello-cxx")`，`fs.enqueue(spawnResult{stdout: <输入>, exitCode: 1})`，`rec, err := r.Ask(ctx, convID, "sol")`：

| 子测试 | 输入（合成，未实测） | `rec.Outcome` | 哨兵 | 那一行 `turn_usage.provider_session_id` |
|---|---|---|---|---|
| `turn_failed` | `{"type":"thread.started","thread_id":"cxx-failed-thread"}\n{"type":"turn.started"}\n{"type":"turn.failed","error":{"message":"cxx boom"}}\n` | `OutcomeProviderFailed` | `provider.ErrTurnFailed` | `"cxx-failed-thread"` |
| `no_completed` | `{"type":"thread.started","thread_id":"cxx-cut-thread"}\n{"type":"turn.started"}\n` | `OutcomeUnparsable` | `provider.ErrUnparseableOutput` | `""` |

表里两个子测试都断言：`rs.svc.ProviderSession(ctx, convID, "sol")` 的 `found == false`；`turn_usage` 那一行 `outcome == int(<结局>)`、`input_tokens` / `cache_read` / `output_tokens` 为 NULL。然后 `fs.enqueueSuccess(cxxExec1CRLF)` 再 `Ask` 一次：`err == nil`、`OutcomeSpoke`、`Fresh == true`、`fs.calls[1].cmd.Args` 等于 `cxxArgs("", "low")`（失败轮没有留下会话，下一轮仍是新会话）。
第三个子测试 `exit1_success`（正对照）：同样的装置，`fs.enqueue(spawnResult{stdout: []byte(cxxExec1CRLF), exitCode: 1})`，`rec, err := r.Ask(...)`：`err == nil`、`rec.Outcome == OutcomeSpoke`、`rec.Message.Text == "PONG"`、`rs.svc.ProviderSession(ctx, convID, "sol")` 的 `found == true` 且 `ProviderSessionID == cxxThreadID`（原型实测）。它钉的是「退出码不进判定、结局只看 stdout」（`spawn.go:184-189`、`runner.go:722-726`），与前两个子测试只差 stdout。

【判别力】`turn_failed` 与 `no_completed` 只差最后有没有 `turn.failed`：前者吸收了 thread id（`runner.go:737-739`），后者没有；把 `turn.failed` 归成认不出（X38）或把「没有 turn.completed」归成失败（X37）都会让 `provider_session_id` 一格对调；失败分支丢掉会话 id（X49）红前者。`exit1_success` 与另两个只差 stdout，退出码同为 1：失败的两条不是因为退出码。

**T3 `TestCxxBudgetHeadroomIgnoresPromptLength`**：一个 `newRealTurnStore`、一个 `r := newTestRunner(t, cxxCfg(), rs.svc)`；两个会话，各自 `joinAgent` `sol` 与 `nova`，第一个会话 `postUser("short")`、第二个 `postUser(strings.Repeat("长", 3000))`；各自 `r.Budget(ctx, convID, Plan{Speakers: []string{"sol", "nova"}})`（`err == nil`，每行 `ArgvHeadroom != nil` 是前提守卫）。断言：
- `sol` 两次的 `ArgvHeadroom` 都等于 `maxCommandLineBytes - CommandLineBytes("codex-fake", cxxArgs("", "low"))`，且这个数等于 `32542`；
- `nova` 第一次减第二次等于 `3000*3 - len("short")`（`8995`）。
【判别力】正文若进了 argv（X09），`sol` 两次不相等；`nova`（agy，正文在 argv）是同一装置下只差 provider 的正对照，证明两段 prompt 的长度差确实传到了命令行余量。`32542` 依赖 `exec.LookPath("codex-fake")` 找不到（`budget.go:366-368` 用原值）。

**T4 `TestCxxPrefixArgsGolden`**：`req := prefixDigestGoldenReq()`、`req.Effort = "high"`（既有 golden 请求的 `effort-golden` 在 codex 上是非法值）：
- `normalizedPrefixArgs(p, req)` 用 `assertArgsEqual(t, "codex", got, want)` 比，`want` 是 §3.1 的空会话形状把 `<model>` 换成 `model-golden`、`<effort>` 换成 `high` 的 19 个元素；
- `PrefixArgsDigest(p, req)` 等于 `goldenDigestFor(want)`，且等于 `"19/282ff78dfd33"`；
- `PrefixArgsDigest(p, prefixDigestGoldenReq())`（`Effort "effort-golden"`）：`errors.Is(err, provider.ErrUnsupportedEffort)`。
【判别力】钉 argv 本身（11b 契约 4.2 的体例）；最后一条是指纹算不出时 `Ask` 在花钱之前拒绝的根据（T6 在 Runner 上验）。

**T5 `TestCxxEffortChangeIsDrift`**：一个 Runner，`sol` 首轮成功（`enqueueSuccess(cxxExec1CRLF)`，前提守卫）；`cfg2 := cxxCfg()`，把 `sol` 的 `Effort` 改成 `"high"`，`r.SetConfig(cfg2)`；`postUser("second-cxx")`；`rec, err := r.Ask(...)`：`rec.Outcome == OutcomeRefused`、`errors.Is(err, ErrIdentityDrift)`、`len(fs.calls) == 1`，且 `err.Error()` 整串等于 §3.7 的漂移文本。
【判别力】`EffortPerTurnFlag` 才会让 `effort` 一段进指纹（`runner.go:1419-1427`）；声明成 `EffortProcessExternalConfig` 时文本只剩「命令行」一段（X20）。

**T6 `TestCxxInvalidEffortRefusedBeforeSpawn`**：`cxxCfg()` 把 `sol` 的 `Effort` 改成 `"turbo"`；`joinAgent("sol")`、`postUser("hello-cxx")`；`fs.enqueueSuccess(cxxExec1CRLF)`（不拒的话会真的成功）；`rec, err := r.Ask(...)`：`rec.Outcome == OutcomeRefused`、`errors.Is(err, provider.ErrUnsupportedEffort)`、`len(fs.calls) == 0`、`rs.svc.History(ctx, convID, 0, 100)` 恰好 1 条（没有 `session_opening`）、`err.Error()` 整串等于 §3.7 的 `turbo` 文本；那一行 `turn_usage` 的 `outcome == 7`、`identity_fp == ""`。
【判别力】对照是 T1 第 1 步（只差 effort 合法：起进程、有 `session_opening`）。

### 2.3 `internal/app/cxx_codex_e2e_test.go`（`package app`；只用 HEAD 已有标识符）

**A1 `TestCxxCodexResumesAcrossRestart`**，两个子测试 `same_config`（第二次启动的 effort `"low"`，期望 `session_opening` 1 条、`session_rebuilt` 0 条）与 `effort_changed`（第二次启动的 effort `"high"`，期望 2 条、1 条）。每个子测试：
1. `isolateAppEnv(t)`；`t.Setenv("COUNCIL_FAKE_CLI", "1")`；`dir := t.TempDir()`；`dataDir := filepath.Join(dir, "data")`；`fake := filepath.Join(dir, "fake_stdout.jsonl")`；`t.Setenv("S10D_FAKE_STDOUT_FILE", fake)`；`t.Setenv("COUNCIL_GLOBAL_RULES", filepath.Join(dir, "no-such-COUNCIL.md"))`。
2. 配置正文（`<effort>` 第一次是 `low`）：

   ```
   data_dir = <tomlLiteral(dataDir)>

   [providers.codex]
   command = <tomlLiteral(os.Args[0])>

   [agents.sol]
   display_name = "Sol"
   provider = "codex"
   model = "gpt-6-sol"
   effort = "<effort>"
   role = "cxx"
   permissions = "read"
   mode = "discussion"
   ```
   `writeTestConfig(t, dir, 正文)`。
3. 把 `cxxExec1CRLF` 写进 `fake`；`runAppForTest(t, ctx, cfgPath, strings.NewReader("/new t\n/join sol\nhello-cxx\n/ask sol\n/quit\n"))`，`err == nil`（前提守卫）；开库读 `SELECT id FROM conversations`（恰好一行，前提守卫）得 `convID`，关库。
4. 用子测试的第二个 effort 重写配置文件；把 `cxxExec2CRLF` 写进 `fake`；`runAppForTest(..., "/use "+convID+"\nsecond-cxx\n/ask sol\n/quit\n")`，`err == nil`。
5. 开库断言：`SELECT text FROM messages WHERE kind = 'agent' ORDER BY seq` 恰好 `["PONG", "KIWI"]`；`kind = 'system' AND json_extract(meta, '$.event') = 'session_opening'` 的条数、`= 'session_rebuilt'` 的条数等于子测试的期望；`provider_sessions WHERE agent_id = 'sol'` 恰好一行、`provider_session_id == cxxThreadID`；`turn_usage WHERE outcome = 1` 恰好 2 行，且两行的 `input_tokens`、`cache_read`、`output_tokens` 都是 NULL。**不断言屏幕**（CO1 会改屏幕）。
【判别力】这是唯一一条走真 `spawnProcess`（stdin 经管道写给真子进程、stdout 的 CRLF 原样回到 `ParseTurn`）、真 `app.Run` 两次启动（指纹与会话从库里读回）的用例；`same_config` 的 `session_rebuilt == 0` 由只差 effort 的 `effort_changed`（`1`）作正对照。原型实测：HEAD 上两个子测试都红；X39、X40、X48 各让两个子测试都红。

---

## 3. 冻结的字面量

### 3.1 空会话 argv（`<model>` 是配置的 model，`<effort>` 非空时才有那两个元素）

```
exec
--json
--skip-git-repo-check
-c
sandbox_mode=read-only
-c
approval_policy=never
-m
<model>
-c                              （仅当 effort 非空）
model_reasoning_effort=<effort> （仅当 effort 非空）
--ignore-rules
--disable
plugins
--disable
apps
-c
skills.include_instructions=false
-
```

一行一个元素（括号里是说明，不是元素的一部分）。effort 非空时 19 个元素，空时 17 个。

### 3.2 续会话 argv

§3.1 的第一个元素 `exec` 之后紧接两个元素 `resume`、`<thread_id>`，其余逐项相同（effort 非空时 21 个元素）。

### 3.3 stdin

- 空会话且 `Role` 非空：`Role + "\n" + Prompt`（一个 LF；不 Trim）。
- 空会话且 `Role` 为空、以及续会话：`Prompt` 原样。
- `Command.Transport == PromptTransportStdin`；`Env`、`Files`、`Path`、`PlannedSessionID` 零值。

### 3.4 强度取值（大小写敏感，顺序即报错文本里的顺序）

```
minimal, low, medium, high, xhigh, max, ultra
```

### 3.5 错误文本（全部在 `codex.go` / `provider.go` 的冻结代码里；经 `diagnose` / `refuse` 进 `Record.Err`，经 `askCause` 默认分支上屏）

```
effort "<值>" is not a codex reasoning level (want one of minimal, low, medium, high, xhigh, max, ultra): effort value not supported by this provider
permission tier "<档>": codex adapter offers read only: permission tier not supported by this provider
codex: stdout is empty: provider output is not a known success shape
codex: stdout line <n> is not JSON: provider output is not a known success shape
codex: stdout has no known codex event: provider output is not a known success shape
codex: <n> turn.completed events, expected one: provider output is not a known success shape
codex: two thread.started events with different thread_id ("<a>", "<b>"): provider output is not a known success shape
codex: turn failed (message="<msg>"): provider reported a failed turn
codex: turn failed (error="<msg>"): provider reported a failed turn
codex: stdout has no turn.completed event: provider output is not a known success shape
codex: success shape carries empty thread_id: provider output carries no session id
```

`<值>`、`<档>`、`<a>`、`<b>`、`<msg>` 是 Go 的 `%q`（带双引号、按 Go 规则转义），上面把引号写在了模板里；`<n>` 是十进制。第 2 行正常走不到（`validateRequest` 先拦）。新哨兵文本：`effort value not supported by this provider`。

### 3.6 `Capabilities()`（P1 的 `want`）

```go
provider.Capabilities{
	SessionID:            provider.SessionIDReturnedOnFirstTurn,
	Effort:               provider.EffortPerTurnFlag,
	Identity:             provider.IdentityPromptPreamble,
	Prompt:               provider.PromptTransportStdin,
	MaxPromptBytes:       0,
	Permissions:          []provider.PermissionTier{provider.PermissionRead},
	UsageFormula:         "input_tokens",
	OutputSchema:         provider.SupportYes,
	MaxTurnsFlag:         provider.SupportNo,
	ReportsCost:          provider.SupportNo,
	ReportsCacheCreate:   provider.SupportYes,
	ReportsObservedModel: provider.SupportNo,
	CleanInstructionEnv:  provider.SupportNo,
	CacheAcrossTurns:     provider.SupportYes,
	WriteBoundary:        provider.SupportUnknown,
}
```

`Usage.Unreported` 的顺序：`input_tokens, cache_create, cache_read, output_tokens, thinking_tokens`。`StopRaw` 只有三个取值：`turn.completed`、`turn.failed`、`error`（认不出时是空串）。

### 3.7 原型实测的派生值（测试逐字节比）

- T1 的身份指纹（`provider_sessions.identity_fp` 与 `turn_usage.identity_fp`；Go 字面量，`\n` 是 LF）：
  `"56:You are Sol (id=sol).\nProvider: codex\n只做反例检查;9:gpt-6-sol;3:low;4:read;0:;0:;15:19/c6979ae99b51;"`
  （第七段是 `sol` 的空会话 argv 摘要；codex 的 argv 不含工作目录，所以这个值与机器、临时目录无关。）
- T4：`"19/282ff78dfd33"`（`model-golden` / `high`）。
- T5 的漂移文本：
  ```
  effort "low" -> "high"; 命令行 "19/c6979ae99b51" -> "19/121092233b57": identity or brain changed since this session was opened
  ```
- T6 的拒绝文本：
  ```
  prefix args digest for sol: effort "turbo" is not a codex reasoning level (want one of minimal, low, medium, high, xhigh, max, ultra): effort value not supported by this provider
  ```
- T3：`32542`（`codex-fake` 与 §3.1 的 19 个元素）；agy 对照差 `8995`。

### 3.8 测试要比对或复验要看的既有文本（不是本刀新增）

- `unknown provider %q (registered: %s): provider not registered`（`provider.go:80`）：`%s` 从 `agy, claude, grok` 变成 `agy, claude, codex, grok`（`IDs()` 的字典序）。这是本刀让用户可见文本变化的唯一既有格式。
- 详细档 `/ask` 下 codex 成员的三行（原型在 app 包实跑的真实输出，§6 复验用；CO1 的简洁档默认不印）：`用量 in=? cache_create=? cache_read=? out=? thinking=?`、`金额未报`、`冷暖 预报=<冷|暖> 实测=未知  wall_ms=<n>  prompt_bytes=<n>  scanned_to=<n>`。
- `/rules` 的逐 provider 行（`repl/rules.go:266-304` 的标签，不含任何 provider 名）：codex 一行会是「叠加在用户全局规则之上」「进缓存前缀，首轮付」与一个命令行余量。

**本刀新增的用户可见字面量只有 §3.5 的 11 行模板与新哨兵文本。** 注释不是字面量。

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；主代理落地后逐条跑，**从字节副本还原，不用 `git checkout`**（本刀落地前 `codex.go` 只活在工作树里）。锚点都是单行、在落地文件里应当恰好一处（实现方加的注释不会与这些代码行重合；若某条锚点因为注释措辞撞了第二处，按真文件改成带上下文的锚点，不改变异的语义）。表里「原型实测」= 在原型上跑过，点名的杀手确实红；杀手按 `包:用例/子测试` 写。未写文件名的都是 `internal/provider/codex.go`。

| # | 变异（替换前 -> 替换后） | 预期被谁杀 |
|---|---|---|
| X01 | `if req.Effort != "" && !codexEffortAllowed(req.Effort) {` -> `if false && req.Effort != "" && !codexEffortAllowed(req.Effort) {` | P3；T4；T6（原型实测） |
| X02 | `"max", "ultra"}` -> `"max"}` | P3；T6（报错文本里的清单变了）（原型实测） |
| X03 | `{"minimal", "low",` -> `{"low",` | P3；T6（原型实测） |
| X04 | `if v == e {` -> `if strings.EqualFold(v, e) {` | P3（`LOW`）（原型实测） |
| X05 | `strings.Join(codexEffortLevels, ", "), ErrUnsupportedEffort)` -> `strings.Join(codexEffortLevels, ", "), ErrCapabilityUnknown)` | P3；T4；T6（原型实测） |
| X06 | `if req.ProviderSession == "" && req.Role != "" {` -> `if req.Role != "" {` | P2 `resume`、`resume_no_effort`；T1 第 3 步 stdin（原型实测） |
| X07 | `if req.ProviderSession == "" && req.Role != "" {` -> `if req.ProviderSession == "" {` | P2 `fresh_no_role`（原型实测） |
| X08 | `prompt = req.Role + "\n" + req.Prompt` -> `prompt = req.Role + "\n\n" + req.Prompt` | P2 `fresh` 等三个；T1 第 1 步 stdin（原型实测） |
| X09 | `args = append(args, "-")` -> `args = append(args, prompt)` | P2 全部；P3；T1；T2；T3；T4；T5（原型实测） |
| X10 | `return []string{"-c", "sandbox_mode=read-only", "-c", "approval_policy=never"}, nil` -> `return []string{"-s", "read-only", "-c", "approval_policy=never"}, nil`（CLI 上多半等价；冻结的是两套形状同一种写法） | P2；P3；T1；T2；T3；T4；T5（原型实测） |
| X11 | 同一行 -> `return []string{"-c", "sandbox_mode=read-only"}, nil` | 同 X10（原型实测） |
| X12 | `args = append(args, "--ignore-rules")` -> `args = append(args)` | 同 X10（原型实测） |
| X13 | `args = append(args, "-c", "skills.include_instructions=false")` -> `args = append(args)` | 同 X10（原型实测） |
| X14 | `args = append(args, "-")` -> `args = append(args)` | 同 X10（原型实测） |
| X15 | `args = append(args, "--disable", "plugins", "--disable", "apps")` -> `args = append(args)` | 同 X10（原型实测） |
| X16 | `args = append(args, "resume", req.ProviderSession)` -> `args = append(args, "resume")` | P2 `resume`、`resume_no_effort`；T1 第 3 步 argv（原型实测） |
| X17 | `args = append(args, "--json", "--skip-git-repo-check")` -> `args = append(args, "--skip-git-repo-check")` | 同 X10（原型实测） |
| X18 | `"model_reasoning_effort="+req.Effort` -> `"model_reasoning_effort=\""+req.Effort+"\""` | P2；P3；T1；T2；T3；T4；T5（原型实测） |
| X19 | `Dropped:   dropMaxTurns(caps, req),` -> `Dropped:   nil,` | P2 `max_turns`（原型实测） |
| X20 | `Effort:         EffortPerTurnFlag,` -> `Effort:         EffortProcessExternalConfig,` | P1；T1 meta；T5 文本（原型实测） |
| X21 | `Prompt:         PromptTransportStdin,` -> `Prompt:         PromptTransportArgv,` | P1；既有 `TestRegistry_CapabilitiesDeclareEveryField/codex`（Argv 不许 `MaxPromptBytes` 为 0）（原型实测） |
| X22 | `Permissions:    []PermissionTier{PermissionRead},` -> `Permissions:    []PermissionTier{PermissionRead, PermissionWorkspace},` | P1；既有 `TestPrefixArgsDigestConformance/codex`（第二档 BuildTurn 失败）（原型实测） |
| X23 | `WriteBoundary:        SupportUnknown,` -> `WriteBoundary:        SupportYes,` | P1；P4（workspace 文本变）；既有 `TestRegistry_CapabilitiesDeclareEveryField/codex`（原型实测） |
| X24 | `CleanInstructionEnv:  SupportNo,` -> `CleanInstructionEnv:  SupportUnknown,` | P1；既有 `TestRegistry_CapabilitiesDeclareEveryField/codex`（原型实测） |
| X25 | `CacheAcrossTurns:     SupportYes,` -> `CacheAcrossTurns:     SupportNo,` | 同 X24（原型实测） |
| X26 | `ReportsObservedModel: SupportNo,` -> `ReportsObservedModel: SupportYes,` | P1（原型实测） |
| X27 | `SessionID:      SessionIDReturnedOnFirstTurn,` -> `SessionID:      SessionIDPreassignable,` | P1（原型实测） |
| X28 | `base.Text = messages[n-1]` -> `base.Text = messages[0]` | P6 `multi_msg`（原型实测） |
| X29 | `base.Thought = strings.Join(messages[:n-1], "\n\n")` -> `base.Thought = ""` | P6 `multi_msg`（原型实测） |
| X30 | `base.Text = messages[n-1]` -> `base.Text = strings.Join(messages, "\n\n")` | P6 `multi_msg`（原型实测） |
| X31 | `if ev.Item != nil && ev.Item.Type == "agent_message" {` -> `if ev.Item != nil {` | P6 `multi_msg`、`reasoning_only`（原型实测） |
| X32 | `line = bytes.TrimSpace(line)` -> `line = bytes.TrimRight(line, "\n")` | P6 `blank_line_inside`（原型实测）。说明：`encoding/json` 把行尾 `\r` 当空白，所以两份真机原件在这条变异下照样解析成功 —— 只有中间夹着一个只有 `\r` 的空行才看得出来，这就是 `blank_line_inside` 存在的理由 |
| X33 | `if completed > 1 {` -> `if completed > 2 {` | P6 `two_completed`；P7（原型实测） |
| X34 | `if threadID != "" && ev.ThreadID != threadID {` -> `if false && threadID != "" && ev.ThreadID != threadID {` | P6 `two_threads`；P7（原型实测） |
| X35 | `if failed {` -> `if failed && completed == 0 {` | P6 `failed_and_completed`（原型实测） |
| X36 | `if completed == 0 {` -> `if completed == 0 \|\| errored {` | P6 `error_then_completed`（原型实测） |
| X37 | `return TurnResult{}, fmt.Errorf("codex: stdout has no turn.completed event: %w", ErrUnparseableOutput)` -> `base.StopRaw = "incomplete"; return base, fmt.Errorf("codex: stdout has no turn.completed event: %w", ErrTurnFailed)` | P6 `no_completed`；P7；T2 `no_completed`（原型实测） |
| X38 | `return base, fmt.Errorf("codex: turn failed (message=%q): %w", failMsg, ErrTurnFailed)` -> `return TurnResult{}, fmt.Errorf("codex: turn failed (message=%q): %w", failMsg, ErrUnparseableOutput)` | P6 `turn_failed`、`failed_and_completed`；P7；T2 `turn_failed`（原型实测） |
| X39 | `Unreported:   []string{"input_tokens", "cache_create", "cache_read", "output_tokens", "thinking_tokens"},` -> `Unreported:   []string{"cache_create"},`（「把 0 当实测值记进去」的那一类） | P5；P6 多条；T1（token 列变成 0）；T2 `turn_failed`；A1 两个子测试（原型实测） |
| X40 | 同一行 -> 去掉 `"cache_read", ` | 同 X39（原型实测） |
| X41 | `usageRaw = append(json.RawMessage(nil), ev.Usage...)` -> `usageRaw = append(json.RawMessage(nil), line...)` | P5；P6 多条；T1 meta（原型实测） |
| X42 | `if !absentOrJSONNull(ev.Usage) {` -> `if len(ev.Usage) > 0 {` | P6 `null_usage`（原型实测） |
| X43 | `if threadID == "" {` -> `if false && threadID == "" {` | P6 `no_thread`；P7（原型实测） |
| X44 | `return TurnResult{}, fmt.Errorf("codex: stdout line %d is not JSON: %w", i+1, ErrUnparseableOutput)` -> `_ = i; continue` | P6 `nonjson_line`；P7（原型实测） |
| X45 | `if !known {` -> `if false && !known {` | P7（`{"type":"x"}` 一行的文本变成「no turn.completed」；同一个哨兵，只有文本分得开）（原型实测） |
| X46 | `base.Completion = completionFrom(base.Text, "")` -> `base.Completion = CompletionComplete` | P6 `zero_msg`、`reasoning_only`、`null_usage`、`no_usage`（原型实测） |
| X47 | `base.StopRaw = "turn.completed"` -> `base.StopRaw = ""` | P5；P6；T1 meta（原型实测） |
| X48 | `Register(codexProvider{})` -> `_ = codexProvider{}` | P1-P7；既有 `TestRegistry_IDsAreExactlyTheFourAdapters`、`TestRegistry_LookupUnknownProvider`；T1-T6；A1（原型实测） |
| X49 | `base.StopRaw = "turn.failed"` -> `base.StopRaw = "turn.failed"; base.ProviderSession = ""` | P6 `turn_failed`、`failed_and_completed`；T2 `turn_failed`（原型实测） |
| X50 | `base.StopRaw = "turn.failed"` -> `base.StopRaw = "turn.failed"; base.UsageRaw = nil` | P6 `failed_and_completed`（原型实测） |
| X52 | `internal/provider/provider.go`：`ErrUnsupportedEffort = errors.New("effort value not supported by this provider")` -> `ErrUnsupportedEffort = errors.New("unsupported effort")` | P3（整串）；T6（整串）（原型实测） |
| X53 | `if err := validateRequest(caps, req); err != nil {` -> `if err := validateRequest(caps, req); false && err != nil {` | P4（原型实测） |
| X55 | `args = append(args, "--json", "--skip-git-repo-check")` -> `args = append(args, "--json")` | 同 X10（原型实测） |
| X56 | `Stdin:     []byte(prompt),` -> `Stdin:     []byte(req.Prompt + prompt[:0]),` | P2 `fresh`、`fresh_no_effort`、`max_turns`；T1 第 1 步 stdin（原型实测） |
| X57 | `return []string{"-c", "sandbox_mode=read-only", "-c", "approval_policy=never"}, nil` -> `return []string{"-c", "approval_policy=never", "-c", "sandbox_mode=read-only"}, nil`（CLI 上等价；冻结的是顺序，它进前缀摘要） | P2；P3；T1；T2；T4；T5（原型实测） |
| X58 | `ModelDiscovery: ModelDiscoveryUnknown,` -> `ModelDiscovery: ModelDiscoverySubcommand,` | P1；D2 改过的 MD1 P1 `TestMdxCapabilitiesDeclareModelDiscovery`（红在 D2-4：`codex 的 ModelDiscovery = 1，期望 0`）（W1b 修订在 `80a9385` 原型实测：锚点恰好一次，只这两条红，其余八包全绿；W1b 审查员照条文自拟 D2 时结论相同） |
| X59 | `if ev.Item != nil && ev.Item.Type == "agent_message" {` -> `if ev.Item.Type == "agent_message" {` | P6 `item_without_item`（空指针 panic）（v1.1 原型实测；W1 的 E01。锚点与 X31 是同一行，替换后的文本不同） |
| X60 | `if threadID != "" && ev.ThreadID != threadID {` -> `if threadID != "" && ev.ThreadID != "" && ev.ThreadID != threadID {` | P6 `second_thread_empty`（v1.1 原型实测；W1 的 E02。锚点与 X34 是同一行） |
| X61 | `if ev.Error != nil {` -> `if true {` | P6 `turn_failed_no_error`（空指针 panic）；P7 `turn_failed_no_error`（空指针 panic）（v1.1 原型实测；证伪者 V3 补的 E03。W1b 修订改了 P6 那一行的输入之后在 `80a9385` 原型复跑：两个杀手都红、都有 `panic: runtime error: invalid memory address or nil pointer dereference`；对照 P6 `turn_failed`、P7 `turn_failed` 绿；`-skip '/turn_failed_no_error$'` 之后 `TestCxx|TestRegistry|TestMdx` 在 provider 包 rc 0） |
| X62 | `if !failed {` -> `if true {` | P7 `two_failed`（报错文本变成 `message="second"`）（v1.1 原型实测；W1 的 E04） |
| X63 | `if !errored {` -> `if true {` | P7 `two_errors`（报错文本变成 `error="second"`）（v1.1 原型实测；W1 的 E05） |

（X51、X54 编号空缺：原型里判为等价，见下。表里的 `\|\|` 是 markdown 表格的转义，落到代码里是 `||`：X36 的替换后文本是 `if completed == 0 || errored {`。）

v1.1 新增的 X59-X63 怎么跑（v1.1 原型的做法，集成员照做）：
- 每条只改一处，锚点在 `codex.go` 里恰好一次（实现方加的注释若撞了第二处，按真文件改成带上下文的锚点，不改变异的语义），改完**只跑点名的那一个子测试**：`go test -count=1 -v -run '^TestCxxParseShapes$/^item_without_item$' ./internal/provider` 这种写法。X59、X61 是空指针 panic，panic 会拖垮整个测试二进制、连带别的子测试一起「失败」，不单独跑就分不清红在哪一行。判红的依据是输出里有 `--- FAIL: TestCxxParseShapes/item_without_item` 这一行（先核 `=== RUN` 那一行在，防 `-run` 什么都没匹配上）。
- 从字节副本还原，比对 SHA-256 相等。
- v1.1 原型上另做了反向核对：`-skip` 掉 v1.1 新增的六行，`-run 'TestCxx|TestRegistry|TestPrefixArgs'` 在 provider / turn / app 三个包上照跑，五条变异全部存活（rc 0）。集成员不必重做这一条。

**判定为等价、不列杀手的变异**：
- X51：`threadID = ev.ThreadID` -> `if threadID == "" { threadID = ev.ThreadID }`：前面那道「两条不同 id 判认不出」已经保证能走到赋值时两者相等。
- X54：把强度校验挪到 `validateRequest` 之前：只有「档位与强度同时非法」的输入分得开（报哪一条错），没有一条用例构造它，可观测结局都是起进程之前拒绝。
- `case "turn.started", "item.started", "item.updated":` 里去掉任一类型：只影响「整份 stdout 只有这一种事件」时报哪一句（同一个哨兵）；这种输出零观测，不单列用例。
- `codexItem.ID` 字段删掉：解析不读它。
- （v1.1 补，W1 的 E07）`usageRaw = append(json.RawMessage(nil), ev.Usage...)` 改成只在 `usageRaw == nil` 时赋值（第一条胜出）：多于一条 `turn.completed` 已判认不出，走不到第二次赋值。W1 另报的 E11（身份块改写）也判等价。

**只在审查时看（没有用例能确定地抓，集成员收割时逐字核对）**：
- 注释里凡涉及 stdin 读法、`-c sandbox_mode` 在 resume 上是否生效、`--ignore-rules`、`approval_policy` 在 exec 下的默认值、失败事件形状、usage 口径的，都必须带「未实测」或「推断」；不许写成已验证。
- `codexPermissionArgs` 的 `default` 分支文本（§3.5 第 2 行）：正常走不到。
- `ErrUnsupportedEffort` 放在 `ErrCapabilityUnknown` 之后、`)` 之前；`provider.go` 只有 B1、B2、B3 三处，其余一字不动（`git diff` 的改动行只落在 `:13`、`:74`、`:103-104` 附近）。
- （W1b 修订）`mdx_discovery_test.go` 只有 D2-1 至 D2-4（`git diff` 的改动行只落在 P1 的 `:201-225` 之间）。
- `registry_test.go` 只有 §1.1 C1-C4（`git diff` 的改动行只落在 C1-C4 列出的位置；C4 只动注释行）。
- （v1.1）注释要点 3 里 MCP 那几句（`CleanInstructionEnv` 的「MCP 服务器去不掉」、`WriteBoundary` 的「read 档对 MCP 管不管得住」）必须带 [推断]，不许写成「sandbox 拦得住 MCP」或「MCP 已隔离」；注释要点 5 的 Windows 沙箱那一段不许写成「`exec` / `exec resume` 已证实走受限令牌沙箱」（W1b 修订换了理由：`codex sandbox` 不带 `-P` 时读 `-c sandbox_mode=`：`danger-full-access` 写得出，`read-only` 与不给都拒写，`workspace-write` 在不带 `-C` 的这一形里也拒写 cwd、原因未测。所以在 `codex sandbox` 这条路上，`-c sandbox_mode=read-only` 选中的是拒写的沙箱；`exec` 与 `exec resume` 走不走同一套仍未测，§6 的付费写盘探针保留。v1.1 原写的「`codex sandbox` 不带 `-P` 时不看 `-c sandbox_mode=`」是错的，注释里也不许出现）。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许改这些文件：新文件 `internal/provider/codex.go`（含 §1.1 D1）；补丁 `internal/provider/provider.go`（§1.1 B1-B3 三处，B3 只改注释）、`internal/provider/registry_test.go`（§1.1 C1-C4 五处，授权的既有测试，C4 只改注释）、`internal/provider/mdx_discovery_test.go`（§1.1 D2 四处，都在 P1 里，锚点与替换文本照 §1.1 D2 逐字）。MD1 已于 `80a9385` 落地，D1、D2 一律交付。不写别的 `_test.go`，不改 `go.mod`。
- 生产改动约 230 行冻结代码加注释，估计 350-400 行（v1.1 的注释要点比 v1 多几句），**一次交付**（不拆块）。
- 新文件：

  ```
  ===FILE: internal/provider/codex.go===
  （完整内容）
  ===END FILE===
  ```
- 补丁块，每个文件可以有多个：

  ```
  ===PATCH: internal/provider/provider.go===
  <<<<ANCHOR
  （从 HEAD 80a9385 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点必须从 HEAD `80a9385` 的真文件逐字复制（本文代码块里的缩进可能被排成空格，以真文件为准），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。`registry_test.go` 的三张表里 `"agy":` 那一行的文本彼此相近，锚点照 §1.1 C3 带上下一行的 `want… := …[id]`。
- 不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。代码必须能 `gofmt`（tab 缩进）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。
- 生产代码新标识符不许以 `cxx` 开头（那是测试腿的前缀）。
- §1.1 给出的代码、签名、判据、字面量、参数顺序照抄；只有注释由你写，覆盖每条注释要点，不许写与代码不符的话。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：给了哪些文件；§1.1 A、B1、B2、B3、C1、C2、C3（三行）、C4（五处注释）、D1、D2（四处）各落在哪个块。
  冻结代码：codex.go 除注释外是否与契约 §1.1 A 逐字相同；有没有改动任何标识符、字面量、顺序。
  注释要点：§1.1 A 的 1-8 条各写在哪个声明上；哪些地方标了「未实测 / 推断」。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3.5 与新哨兵文本是否逐字照抄；有没有新增任何别的用户可见字面量。
  ```

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；按锚点落盘；`gofmt -l`；`go vet ./internal/provider ./internal/turn ./internal/app`；`go build ./...`。逐字核对：`codex.go` 去掉注释之后与 §1.1 A 逐字相同（脚本比，不目测）；`provider.go` 只有 B1-B3 三处；`registry_test.go` 只有 C1-C4；`mdx_discovery_test.go` 只有 D2-1 至 D2-4，与 §1.1 D2 的替换文本逐字相同；注释里「未实测 / 推断」的标注齐全（§4「只在审查时看」）。
2. **合并测试腿的三个文件**之后，只跑 `./internal/provider ./internal/turn ./internal/app`，再跑全量 `go test -count=1 ./...`（原型实测全绿）。另一条流水线也在跑 go test 时，3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制。核对测试腿报告里 §2.3 在 HEAD 上单独跑的结果与 §2 通用纪律末尾写的一致（两个子测试都红）。
3. **§4 的变异逐条跑**（X01-X63，含 X58；从字节副本还原；X59-X63 照 §4 表后的写法单独 `-run` 点名的子测试）；任何一条没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
4. **文档**：
   - `docs/02_AGENT_PROVIDER_ARCHITECTURE.md:21-35` 的 codex 列按真 CLI 订正：单轮 `exec --json`；恢复 `exec resume <thread_id>`（没有 `-s`、没有 `-C`）；身份「首轮正文前缀（stdin）」；effort `-c model_reasoning_effort=`（codex 不校验值，适配器校验）；权限 `-c sandbox_mode=read-only` + `-c approval_policy=never`（workspace 未开放）；工作目录「进程 cwd」；跨轮缓存「有（2026-09-28 一次观测：同一 thread 第二轮 cached 0 → 16000）」。表头下写一句「codex 列 2026-09-28 按 CX1 订正」。
   - `docs/05_MESSAGE_PROTOCOL.md` 第 3 节补 codex 一行：会话 id 取 `thread.started.thread_id`；usage 在 `turn.completed`，[推断] thread 累计，Council 一个 token 列都不记；正文取最后一条 `agent_message`，前面的进 thought。
   - `docs/06_SECURITY_MODEL.md` 第 2 节（`:56` 那句「codex 尚无适配器，整列删掉」）：补 codex 的 read 档实际发的参数与「workspace 未开放、`WriteBoundary` 未实测」；第 4 节补「全局 `~/.codex/AGENTS.md` 隔离不掉，与 grok 同档」。
   - `docs/09a_STEP6_API_CONTRACT.md`：登记 `codexProvider`、`ErrUnsupportedEffort`（何时返回、与 `ErrCapabilityUnknown` 的区别）；`:375` 那句 Lookup 注释跟着 B1 改。
   - `docs/09_STEP6_SPEC.md:265`、`:913`、`:1003` 各加一行订正：「2026-09-28 CX1 登记了 codex，本条作废」。
   - `docs/00_DECISIONS.md`：新条目记 CX1 的裁决（stdin + `-`、身份前缀、`-c` 两套同形、read 档四个参数、强度并集校验、usage 全部标未报的理由）与 §7 的 open items；#45 末尾那句「`gpt-6.0-sol` 今天不能当成员」后面补「CX1 已接入（<commit>），真机复验见 #<n>」。
   - `docs/README.md` 路线图：codex 适配器已落（写 commit）。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的配置示例**（他自己加进 `C:\Users\25142\.council\config.toml`，改之前先备份；主代理不碰这份文件）。id 照 #45 的写法「模型-版本-强度」：

   ```toml
   [providers.codex]
   command = "codex"

   [agents.gpt-6-sol-high]
   display_name = "GPT-6 Sol"
   provider     = "codex"
   model        = "gpt-6-sol"
   effort       = "high"
   role         = "（照你别的成员写）"
   permissions  = "read"
   mode         = "discussion"
   ```

   `effort` 只能是 `minimal / low / medium / high / xhigh / max / ultra` 之一（小写），写别的值时 `/ask` 在起进程之前就拒绝、不花钱；`gpt-6-sol` 支持 `low` 到 `ultra` 六档；`ultra` 会让模型自动分派子任务，花费可能成倍（未实测）。`permissions` 只能是 `read`。MD1 已落地，但 `/add` 的识别清单里不会出现 codex 的模型（本刀没接识别，D1 声明 Unknown），codex 成员照上面手加。
6. **真机复验**（Griffin；这几步是**付费调用**，每一步的 `/ask` 都会真的问一次 `gpt-6-sol`，要他授权再做）。**每条 council 内命令单独一块，逐条粘贴、逐条回车**（#44：整块粘贴会被当成一条提交）。
   1. 按第 5 步加好配置、备份原文件，重新打开 council：

      ```
      council
      ```
   2. CO1 已落地，默认是简洁档；先切到详细档，才看得到用量、金额、冷暖三行（W1b 修订：用 `/verbose on` 而不是来回切换的 `/verbose`，敲几次都是详细档）：

      ```
      /verbose on
      ```
   3. 看指令环境一行：codex 那一行应当是「叠加在用户全局规则之上」「进缓存前缀，首轮付」与一个几万字节的命令行余量：

      ```
      /rules
      ```
   4. 新建一个会话：

      ```
      /new codex复验
      ```
   5. 把成员加进来：

      ```
      /add gpt-6-sol-high
      ```
   注意（v1.1，W1 F1；适用于下面每一步）：只要屏幕上出现「送出去的会话 id 与回来的不一样」，或 codex 成员从某一轮起每轮都失败，先停下，敲下面这一条再往下做（codex 的身份只在空会话那一轮随正文发，CLI 悄悄开的新会话里没有身份；报错那一支则每轮都失败，直到重建。见 §7 待主代理第 12 条）：

      ```
      /session rebuild gpt-6-sol-high
      ```
      重建与漂移都会让后面的轮次落到另一个 codex 会话（thread）上（W1b 修订）：发生在第 6 步成功之后、第 8 步做完之前，第 17 步的用量口径判定作废；发生在第 6 步成功之后、第 10 步做完之前（含第 10 步自己漂移），续会话写盘（第 10 步）的结论作废。照实做完、照实交给主代理即可，第 17 步按库里的会话 id 核这两条前提，不靠记忆；要重做作废的那几步，是新的付费调用，另要 Griffin 授权。
   6. 第一轮（**付费**；这一步验的是 stdin 与 `-` 这条路，本契约唯一没法零花费验证的地方。它也是 Council 起的 `codex.cmd` 这一路的第一次：主代理两次实测走的是 `codex.ps1`，cmd.exe 重新解析 `%*` 时带 `=` 的参数、单独的 `-`、stdin 转发都在这一步第一次过，见 §0 第 3 条。v1.1 起顺带让模型列出它能用的工具，见 §0 第 8 条）：

      ```
      /ask gpt-6-sol-high 先逐行列出你此刻能调用的全部工具名（含 mcp 开头的，一个不漏），列完另起一行只写 PONG，并记住单词 KIWI
      ```
      期望：`outcome spoke`；一行 `provider 会话 <一个 UUID>`；正文里有工具清单与单独一行 `PONG`；`用量 in=? cache_create=? cache_read=? out=? thinking=?`；`金额未报`；`冷暖 预报=冷 实测=未知`。若是 `outcome unparsable` / `provider_failed`，把整段输出原样交给主代理（多半是 stdin 没被读到、`codex.cmd` 经 cmd.exe 转参时改坏了某个参数，或 `-c sandbox_mode` / `--ignore-rules` / `skills.include_instructions` 里有一个在真 `exec` 上不被接受）。
      工具清单原样交给主代理：有没有 `mcp` 开头的、有没有能执行代码或写文件的（`node_repl` 一类）—— 第 10、14 步要用。另注意正文开头是不是「收到，Griffin。」：全局 `~/.codex/AGENTS.md` 隔离不掉（`CleanInstructionEnv = No`），出现的话记下来。
   7. 第二轮（**付费**；验续会话；只回一个词，第 17 步要拿它与第 8 步比）：

      ```
      /ask gpt-6-sol-high 上一轮让你记住的单词是什么？只回答那一个词
      ```
      期望：正文里有 `KIWI`；`provider 会话` 那一行与第 6 步是**同一个 UUID**；`冷暖 预报=暖`。
   8. 第三轮（**付费**；给主代理判 usage 是不是累计用的；与第 7 步同形，只回一个词）：

      ```
      /ask gpt-6-sol-high 只回答一个词：PONG
      ```
   9. 看会话表：一行 `gpt-6-sol-high`，会话 id 与第 6、7 步相同：

      ```
      /session
      ```
   10. **必做**：只读档写盘探针，续会话形状（**付费**，要他另外授权；read 档「写不了盘」在这一步之前只是声明，claude 的 read 档当初也是这样一次写盘探针之后才认的；续会话这一次最要紧，因为 `-c sandbox_mode` 在 `exec resume` 上是否生效没验证）。问法看第 6 步的工具清单，二选一：
       - 清单里**没有**能执行代码或写文件的 MCP 工具：

         ```
         /ask gpt-6-sol-high 在当前目录建一个 PROOF.txt 并写入 WRITTEN
         ```
       - 清单里**有**（v1.1，W1 F0；例如 `node_repl`）：把 `<工具名>` 换成清单里的那个名字，shell 与 MCP 各试一次、各写各的文件、分别报告（W1b 修订：shell 写 `PROOF.txt`，MCP 写 `PROOF_MCP1.txt`，文件名分得开是谁写的；有几个就各点一次名，第二个起在句中接着写「再用 <工具名2> 建 PROOF_MCP2.txt」，依此类推）：

         ```
         /ask gpt-6-sol-high 在当前目录建文件并写入 WRITTEN，逐个做：先用 shell 建 PROOF.txt，再用 <工具名> 建 PROOF_MCP1.txt；逐个报告成功还是被拒、写到了哪个完整路径
         ```
   11. 在另一个 PowerShell 窗口里看有没有写出文件（`<data_dir>` 换成启动横幅「数据目录」那一行的路径；清单里没有 MCP 写盘工具时只看第一行，有第二个起照样多加 `PROOF_MCP2.txt` 一行）；每一行都应当是 `False`。任何一行是 `True` 就到此为止，不做第 12-15 步，立刻告诉主代理、在配置里去掉 codex 成员。另看第 10 步模型的报告：MCP 工具的工作目录不一定是成员的工作目录，文件可能写到了别处 —— 模型报告用 MCP 写成功了，按它报的路径再 `Test-Path` 一次；是 `True` 同样到此为止：

       ```
       Test-Path "<data_dir>\workdir\gpt-6-sol-high\PROOF.txt"
       Test-Path "<data_dir>\workdir\gpt-6-sol-high\PROOF_MCP1.txt"
       ```
   12. 同一个探针的空会话形状：新建一个会话：

       ```
       /new codex写盘探针
       ```
   13. 把成员加进来：

       ```
       /add gpt-6-sol-high
       ```
   14. 再问一次（**付费**），问法与第 10 步相同，二选一：
       - 清单里**没有**能执行代码或写文件的 MCP 工具：

         ```
         /ask gpt-6-sol-high 在当前目录建一个 PROOF.txt 并写入 WRITTEN
         ```
       - 清单里**有**（`<工具名>` 与文件名照第 10 步）：

         ```
         /ask gpt-6-sol-high 在当前目录建文件并写入 WRITTEN，逐个做：先用 shell 建 PROOF.txt，再用 <工具名> 建 PROOF_MCP1.txt；逐个报告成功还是被拒、写到了哪个完整路径
         ```
   15. 在 PowerShell 里再看一次，每一行都应当仍是 `False`（只读档两条会话共用同一个工作目录，所以第 11 步必须全是 `False` 才看得出这一次）；模型报告 MCP 写成功时，照第 11 步按它报的路径再看一次：

       ```
       Test-Path "<data_dir>\workdir\gpt-6-sol-high\PROOF.txt"
       Test-Path "<data_dir>\workdir\gpt-6-sol-high\PROOF_MCP1.txt"
       ```
   16. 退出：

       ```
       /quit
       ```
   17. 做完把第 6、7、8、10、14 步的屏幕输出与第 11、15 步的结果交给主代理。主代理只读查库，先核前提，再取第 6-8 步三条 `messages.meta.usage_raw`，逐轮比 `output_tokens` 与 `reasoning_output_tokens`（v1.1 按 W1 F2 / V2 重写；前提是 W1b 修订加的）：
       - **前提（先核，不成立就不判）**：`SELECT COUNT(*) FROM conversations WHERE title = 'codex复验'` 恰好是 1；再用下面这条取「codex复验」里 `gpt-6-sol-high` 成功的轮（`outcome = 1`），按 `seq` 排：

         ```sql
         SELECT tu.seq, tu.provider_session_id, json_extract(m.meta, '$.usage_raw')
         FROM turn_usage tu JOIN messages m ON m.conv_id = tu.conv_id AND m.seq = tu.seq
         WHERE tu.conv_id = (SELECT id FROM conversations WHERE title = 'codex复验')
           AND tu.agent_id = 'gpt-6-sol-high' AND tu.outcome = 1
         ORDER BY tu.seq
         ```
         前三行应当是第 6、7、8 步（与屏幕上的正文核对），第四行是第 10 步。**口径判定只在前三行的 `provider_session_id` 是同一个非空值时做**；否则记「口径未判定（重建或漂移污染）」，不下结论，重做要 Griffin 另行授权，或走 §7 待 Griffin 第 6 条。依据：漂移那一轮的 `turn_usage` 记的是 codex **回来的**新 id（`runner.go:753` 判漂移、`:768` 取回来的 id，`makeTurnUsage` 在 `:1170` 写进这一列）；`/session rebuild` 之后的下一轮是空会话、开新 thread、拿到新 id。`/session rebuild` 本身不在库里留面包屑（`runner.go:235-256` 只删行与内存快照），所以只能按 id 判，不能数 `session_rebuilt` 事件。
       - **主判据：这两项里任何一项比上一轮小，就排除累计**（累计口径下每一项只增不减）。
       - 辅助预测：第 7、8 步都只回一个词，本轮口径预测第 8 步的 out 约等于第 7 步的 out；累计口径只推得出第 8 步 ≥ 第 7 步（第 7 步的值加上第 8 步自己的产出），**不是**「不小于前两轮之和」（v1 这句写错了，已删）。第 6 步列了工具、比后两步长得多，本轮口径下第 7 步多半已经比第 6 步小 —— 那一步就能排除累计。
       - `input_tokens` 不作判据：两种读法都预测每轮约 +16k（§0 第 6 条的第二种读法：续会话时把整段初始上下文再送一遍）。
       - 没有任何一项变小时，只能记「与累计相容、未证实」，不记成「证实累计」。零花费的另一条路见 §7 待 Griffin 第 6 条。
       据此在 `00_DECISIONS` 定 usage 口径（§7 待主代理第 1 条）。写盘按文件名分开记（W1b 修订）：
       - shell（`PROOF.txt`）：exec resume 与 exec 分开认，每一次都要「那一轮 `outcome spoke`、模型回答说 shell 写不了、对应那一步的 `PROOF.txt` 是 `False`」（失败轮里模型没动手，`False` 不算围得住）。
         - exec resume（第 10 步）：另要上面那条查询的第四行存在、`provider_session_id` 与前三行相同（第 10 步真是续会话；屏幕上应是「冷暖 预报=暖」）。不成立记「exec resume 未测」。
         - exec（第 14 步）：「codex写盘探针」是新会话，第 14 步必然是空会话形状；那个会话里 `gpt-6-sol-high` 恰好一行 `outcome = 1`。不成立记「exec 未测」。
         两条都成立，才把「read 档的 shell 在 exec 与 exec resume 上都围得住」记成已验一次；只成立一条，只记那一条。
       - MCP（`PROOF_MCP<n>.txt`，逐个工具单独记）：清单里没有可试的 MCP 写盘工具，记「没有可试的」；试了且两次都被拒（对应文件两次都 `False`、模型报告被拒），记「这一个工具在这两次被拒」，不推广到别的 MCP 工具；写出来了（任何一次 `True`，含按模型报的路径查到的），残留风险成真，停用 codex 成员并告诉 Griffin（§7 待 Griffin 第 5 条）。不要拿 MCP 的结果去认 shell 的结论，反过来也一样：文件名分得开，模型的自述只作参考。
7. **清理**：本刀在中转根与 scratchpad 里的全部中间物（任务书、grok 回复、收割脚本）；`codexprobe_*` 是主代理的探针原件，是否保留由主代理定。

---

## 7. open items

### 待 Griffin

1. **真机复验要花钱**：§6 第 6-8 步是三次付费调用，第 10、14 步的写盘探针另两次。stdin 这条路与 read 档围不围得住只能这样验。中途若重建或漂移，第 17 步会判某些结论作废（W1b 修订，§6「注意」），重做作废的几步是新的付费调用，另要他授权。
2. **全局 `~/.codex/AGENTS.md` 隔离不掉**（3681 字节，里面有「第一句话输出：收到，Griffin。」这类规则）：每个 codex 成员每轮都带着它，回复可能以那句话开头。可选的办法都在 Griffin 手里：在那份文件里加一句「在 Council 里被调用时不适用」；或者接受。Council 这边能做的只有换 `CODEX_HOME`，而那会把登录态一起带走（与 claude 的旧死结同一类），本刀不做。
3. **`ultra` 档**：说明是「Maximum reasoning with automatic task delegation」，会自动分派子任务，花费可能成倍；本刀放行（它在 `gpt-6-sol` 的合法档里）。要不要在 Council 里禁掉，等 Griffin 定。
4. **workspace 档**：本刀只给 read。要让 codex 成员改文件，需要一次付费探针证实 workspace-write 沙箱在 Windows 上围得住、并且在续会话上仍然生效，之后另开一刀。
5. **MCP（v1.1，W1 F0）**：Griffin 的 codex 开着 `node_repl`（能执行任意代码）、`cua_repl`（电脑 / 浏览器操作）、`drawio`（主代理 2026-09-28 06:45 零花费看过清单）。每个 codex 成员都会带着它们，read 档管不管得住零花费测不到 [推断：多半管不住]；Council 没有能冻进 argv 的整类开关（§0 第 8 条），也不碰他的配置。两条路由他选：
   - 在他自己的 codex 配置里把它们关掉（`enabled = false`；这会同样影响他平时直接用 codex 与桌面版），然后再做 §6；
   - 或者明确接受这个残留风险，照 §6 做，第 10、14 步用能写盘的那个 MCP 工具试一次写盘。
   他可以先零花费看一眼 Council 实际带着哪些：在自己的终端里敲 `codex --disable plugins --disable apps mcp list`（Council 每轮都发这两个参数）。若 `cua_repl`、`node_repl` 这时不见了，说明它们来自插件、已经被 Council 关掉 [未测：本契约的隔离探针造不出插件带来的条目]。
6. **用量口径的零花费判定（v1.1，W1 F2）**：`~/.codex/sessions` 下那两次付费会话（2026-09-28 01:40 主代理的两次调用）的 rollout 文件里，可能有 `token_count` 事件同时带着「累计」与「本轮」两份用量，拿 16184、32396 一对就能零花费判定口径、省掉 §6 第 8 步那一次付费。但那些文件含对话内容，主代理与流水线都不许读：要 Griffin 自己看，或者明确授权主代理读那两个文件。

### 待主代理

1. **usage 口径**：`turn.completed.usage` 是 thread 累计还是本轮 [推断累计]。§6 第 17 步：第 6-8 步的三轮数据落库后判定（v1.1 的主判据：`output_tokens`、`reasoning_output_tokens` 任何一项比上一轮小就排除累计；都没变小只记「与累计相容、未证实」）；也可能由待 Griffin 第 6 条零花费判定。判定的前提（W1b 修订）：第 6-8 步三条成功轮的 `turn_usage.provider_session_id` 是同一个非空值，重建或漂移污染时记「未判定」，见 §6 第 17 步。若证实累计，可以另开一刀「按 provider 会话存 token 基线、记增量」（要 schema 列）；若证实是本轮，改成照 grok / agy 的方式记数。在那之前 codex 成员的 `/cost` 与 `/round` 预报只有「?」。
2. **stdin 与 `-`，以及 `codex.cmd` 这一路**：两条 help 自述、未实测；主代理的两次付费实测走的是 `codex.ps1`，Council 起的是 `codex.cmd`（cmd.exe 重新解析 `%*`：带 `=` 的参数、单独的 `-`、stdin 转发），这一路也没测（§0 第 3 条，W1 F5）。§6 第 6 步是唯一的验证。失败时按输出判是哪一个参数不被接受、还是 cmd.exe 改坏了哪个参数，改契约再改代码。
3. **`-c sandbox_mode=read-only` 在 `exec resume` 上是否生效**、`approval_policy=never` 在 `exec` 下是否本来就是默认、`--ignore-rules` 与 `-c skills.include_instructions=false` 在真 `exec` 上是否被接受：零花费都测不到（`debug prompt-input` 不收前两个参数、不代表 exec 的默认）。§6 第 6、7、10、14 步顺带验。
4. **失败形状零观测**：`turn.failed`、`error` 事件是按 codex exec 的事件定义合成的；第一次真机失败（例如写错 model 名）时把 stdout 原件存进中转根，补进 §2 的夹具。
5. **逐模型强度子集**：并集放行 `gpt-6-luna` + `ultra` 这类组合，真实行为未知。等 MD1 之后接 `codex debug models` 时，可以在识别那一层只给该模型支持的档（与 claude 的做法一致）。
6. **身份的更强通道**：`-c developer_instructions=` 在 `debug prompt-input` 里有效（成为第一条 developer 消息），但要进 argv、过 `.cmd` 垫片、经 TOML 解析；若将来要让身份不落在 user 消息里，先在 Go 里用 `exec.Command` 对 `debug prompt-input` 实测转义，再另开一刀（会改所有 codex 成员的前缀，全员重建一次）。
7. **身份前缀与标题之间只有一个换行**（`...只做反例检查\ncxx\n\nuser: ...`）：与 agy 同一套，模型可能把会话标题读成角色的一部分。要改就 agy 与 codex 一起改（改 agy 会让它的全部会话重建），不在本刀。
8. **`include_environment_context=false`**：能去掉 cwd、时区与日期；本刀没用（日期对模型有用、影响未测）。要不要加，等复验之后定。
9. **codex 的 `Thought` 是中途的说明文字，不是推理**：多条 `agent_message` 时前面几条进 `Thought`（§1.1 A 注释要点 7）。`store_thought = false` 时它们被丢掉、`/log` 打 `[thought_dropped]`；团队的 `thought_visible_to` 含裁判时裁判看得到这些说明。语义与 grok 的 thought 不同，落地后在 `05_MESSAGE_PROTOCOL.md` 写明；要不要给 codex 单开一种处理，等真机上见到多条消息的样本再定。
10. **MD1 的 `SuggestedID`**：codex 登记之后，agy 里 label 以 `codex-` 开头的行会被改前缀（MD1 open item (o)）；今天的 agy 清单没有这种行，落地后在 MD1 的真机复验里顺带看一眼。
11. **codex 的模型识别**（`codex debug models` 接进 MD1 的框架）：MD1 落地之后另一刀；届时把 D1 的 `ModelDiscoveryUnknown` 改成 `ModelDiscoverySubcommand`，并给 `DiscoveryCommandFor` / `ParseDiscovery` 加 codex 一支。
12. **续会话时 codex 对不认识的 thread id 怎么办，零观测（v1.1，W1 F1，按 V1 降为 minor）**：两支都没测。
    - 悄悄开新会话：`runner.go` 判 `Drifted`、打一行警告、把新 id 写进 `provider_sessions`；之后每轮都是续会话形状，而身份前缀只在空会话那一轮发（`IdentityPromptPreamble`），**这个成员从此没有身份、没有历史**，`outcome` 仍是 spoke。claude（`--append-system-prompt-file`）与 grok（`--system-prompt-override`）每轮重发身份，不受影响。
    - 报错：`failClosed` 不清会话，死 id 每轮重试、每轮失败，直到 `/session rebuild`。
    - **agy 有同样的问题**（同是 `IdentityPromptPreamble`）。今天 `ask.go` 的警告「送出去的会话 id 与回来的不一样」不提身份丢了、也不叫用户重建；改措辞要另开一刀，agy 与 codex 两家一起改（本刀不许动 `internal/repl`）。不加 T 用例：它会把缺陷冻成期望（V1）。§6 第 6 步之前的「注意」已经教用户见到就重建。
13. **read 档对 MCP 管不管得住（v1.1，W1 F0）**：零花费测不到（§0 第 8 条）。§6 第 10、14 步在清单里有 MCP 写盘工具时顺带试一次；MCP 写 `PROOF_MCP<n>.txt`、shell 写 `PROOF.txt`（W1b 修订），结果按第 17 步按文件名分开记，不与 shell 的结论混在一起。若 Griffin 选了「在他的配置里关掉」，复验时第 6 步的清单里应当已经没有它们，照实记。
14. **别的测试文件里随 codex 登记而过期的注释（v1.1，问「同类的还有没有别处」得出，未授权改）**：`internal/provider/validate_internal_test.go:6-9` 说三家的 `WriteBoundary` 从来不是零值、「没有一条自然路径能让 validateRequest 看到 WriteBoundary == SupportUnknown」—— codex 声明的正是 `SupportUnknown`，§2.1 P4 走的就是这条自然路径；`internal/turn/prefix_digest_test.go:10`、`:21`、`:76` 的「三家」（`TestPrefixArgsDigestConformance` 遍历 `IDs()`，已含 codex，原型实测 codex 子测试绿）。裁决只授权了 `registry_test.go`，这几处留给主代理定：并进本刀的授权，还是落地后另改。`internal/provider/build_test.go` 的 `allProviderIDs` 写死三家，那几条用例（`WorkDirRequired`、`FullRejectedForAllThree`）不覆盖 codex，但注释说的「三家」仍然成立；codex 的同类覆盖在 §2.1 P4。
    W1b 修订又问了一遍，在 `80a9385` 上 `git grep` 了「三家 / 第四家 / 三个适配器 / 例如 codex」一类字样，逐处看过上下文。本刀已接走的：`mdx_discovery_test.go:202`、`:212` 的「第四家」归 §1.1 D2（D2-1、D2-3），`provider.go:13` 的「三家 CLI」归 §1.1 B3。其余按性质分三类，都**未授权改**，留给主代理定：
    - **过期注释（登记 codex 之后与代码不符）**：`internal/repl/addmodel.go:54-55`（MD1 的文件）「provider.Lookup 找不到（没有适配器，例如 codex）」—— 与 B1 修掉的 `provider.go:74-75` 是同一句话，CX1 不许改 `internal/repl`；`internal/repl/ask_test.go:60-61`「registry 只登记了 claude / grok / agy 三个 id」—— 它要论证的「`pv_alpha` 这类 id 不在 registry 里」仍然成立，用例不受影响；`internal/provider/turn.go:154` `ProviderSession` 行尾的「三家必须都填」—— codex 同样必填（空时 `ErrNoSessionID`，§2.1 P6 `no_thread`），`turn.go` 在 §1.2 不许改。
    - **仍然成立、只是没提 codex**：`internal/provider/turn.go:181`「三家里目前只有 agy 会填它」（`DeniedActions`；codex 也恒为 nil）；`internal/turn/spawn.go:74`「本项目三个适配器发出的参数里不存在这样的值」（以减号开头的值；codex 的 `-` 是位置参数，前一个元素 `skills.include_instructions=false` 不以减号开头，不会被误报成某个 flag 的值）；`internal/turn/budget.go:260`「三家适配器都填了 CacheAcrossTurns，这条分支今天走不到」（codex 声明 `SupportYes`，仍走不到）；`internal/turn/session.go:92`、`runround_test.go:118`、`conformance_test.go:78`、`x38a_reported_test.go:14` 说的是三家的共同做法，codex 同样如此。
    - **覆盖缺口**：`internal/repl/rules_test.go:210-224` 的 `TestCacheLabelNeverNamesAProvider` 只查 `claude`、`grok`、`agy` 三个名字不出现在 `cacheLabel` 的文本里，不查 `codex`。今天 `rules.go:285-294` 的三档文本里没有 `codex`（`git grep -i codex internal/repl/rules.go` 零处），是缺口不是缺陷；CX1 不许改 `internal/repl`。
15. **`--disable plugins`、`--disable apps` 的升级风险（v1.1）**：`--disable` 校验名字，不认识的名字让 codex 起不来（§0 第 4 条 m04），标 removed 的照收。将来某版若把这两个 feature 名整个删掉，每个 codex 成员每轮都会失败（stdout 为空，认不出）。升级 codex 之后先零花费跑一次 `codex --disable plugins --disable apps features list` 看 rc。不改成 `-c features.<名>=false`：那一形对错名静默接受。
16. **W1b 修订里三处超出裁决书字面的取舍（请确认）**：三处都不改用户看得到的行为，判为不涉及方向，所以照改了，列在这里请主代理确认。
    - 基线改到 `80a9385`。派工的任务书写的是「HEAD = 2a8737f」，已经过期（`git rev-parse HEAD origin/main` 都是 `80a9385`）。契约照「以 HEAD 为基线」的字面取了真 HEAD。要退回 `2a8737f`，只需把文件头列的五处行号改回去。
    - D2 逐字冻结。裁决书写的是「修订时写明 W2 时按真文件取，不要编造它的内容」，那时 MD1 还没落地。现在真文件在了，冻结的四处都从它逐字取出，没有编造，并且用脚本核过：按契约落到原文件上，结果与原型测过的文件逐字节相同。要照字面改回「W2 时取」，这四处可以当参考文本附给实现腿。
    - B3（`provider.go:13` 的注释）。裁决书 F6 只授权了 `registry_test.go` 的注释。B3 在本刀本来就在改的文件里，只改注释。不要的话删掉 B3 一节，§4、§5、§6 第 1 项里的「三处」改回「两处」。
