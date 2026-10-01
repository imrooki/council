# MD1 冻结契约：模型识别 + /add 直接加入识别出的模型（追加写 config.toml）（v1.1，2026-09-28；v1 为 2026-09-27）

> **落地后订正（2026-09-28，主代理；MD1 已于 `e9afff9` 落地）**：
> - §4「判定为等价、不列杀手的变异」第 4 条（三家识别改成并发）**已不等价**：v1.1 F3 的父 ctx break 之后，并发写法下三家都已起进程，R14 first、late 会红（集成员实测）；R1 靠调用顺序断言，受调度影响，不作杀手。
> - §1.1 B2 注释要点 6、7 里对 `configcmd.go` 的行号引用（`:124-126`、`:149-155`）按 HEAD 写，B5 给该文件头加了 3 行之后都偏了 3 行；落地代码的注释改为按名字引用（cmdConfigReload 的 data_dir / rules_file 那一支、--apply 的换指针三句），由审查余项那一刀（fu1）订正。
> - 契约 open item (s)：写前守卫对 SM1 新增的 `Config.Summary` 字段的子测试由 fu1 补。
> - 独立审查余项（代码都没错）：两处 nil 防崩判据与守卫的读失败分支没有用例、三处注释措辞，由 fu1 补。
> - **open item (c) 已由 AD1 解决**（2026-10-01，`00_DECISIONS.md` #49，契约 `19g`）。已在配置里的模型仍然不编号，`+编号` 不变；换一档强度改用按名字加，例如 `/add <识别表 ID 列的名字>`、`/add <模型名> <强度>`、`/add <模型名> --effort <强度>`。快照里的全部行都能按名字加，走的是与 `+编号` 同一套写配置流程。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `28845e0` = origin/main（v1 起草时是 `ab53c0b`；两者之间只多一个改 `.handoff/HANDOFF.md` 的文档提交，代码逐字节相同。生产代码最后一次改动在 `a2e3d76`；本文生产代码行号与锚点仍以 `ab53c0b` 为准）。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`。本机没有 gcc，不能 `-race`。
- 本刀的 KEY 是 `MD1`；测试腿的包级标识符前缀是 `mdx`（类型、函数、变量、常量一律 `mdx` 开头，测试函数 `TestMdx...`）。
- 来源：
  - `docs/00_DECISIONS.md` **#45 第一节**（`:2107-2118`）全部：识别（agy / claude / grok 三种方式）、写进 config.toml（只追加、写前备份、写后 Load + Validate、两处换指针、再 Join）、id 跟现有写法、只在两家重名时加来源前缀、agy 的 Claude / GPT-OSS 行标注走 agy 额度、/add 时按需识别、claude 选强度（默认 high，只给支持的档）、grok 不问强度、agy 强度在名字里、新成员 read / discussion、已有成员一个不动、补 `ModelDiscovery` 能力位。**方向照 #45，本契约不改方向。**
  - #45 推翻的旧裁决：`docs/11d_STEP8C_CONTRACT.md:626-632`（不做运行时新增 agent、不写回 toml）与 `internal/repl/configcmd.go:3-7` 头注释。
  - `docs/00_DECISIONS.md` #36(g)（`:1701-1708`，点号与连字符撞 claude 项目目录）、#37(h)（`:1766-1772`，id 可以是 Windows 保留设备名）。
  - `docs/02_AGENT_PROVIDER_ARCHITECTURE.md:34`（能力发现一行）、`:64`（`ModelDiscovery` 设计位）、`:73-79`（三级回退）。
- **与本批另两刀的相交表**（v1.1 按主代理裁决书写定；CO1 = #45 第二节的简洁输出与每个模型的用时，SM1 = 多成员 `/round` 之后的自动总结）：
  - **落地顺序**：MD1 与 CO1 的实现并行，谁先落都行；后落的一方按真文件重取 `repl.go` 的锚点（`REPL` 结构体末尾、`New` 字面量）。SM1 在 CO1 之后落。MD1 内部的块序见 §5（W1b 修订后五块：A-1a 最先；A-1b、A-2 都只依赖 A-1a，两者先后不限；B-2 最后；B-1 随时）。
  - **`TestMain`**：MD1 的测试腿在 `internal/turn` 新增本包第一个 `TestMain`（HEAD 上零命中）。SM1 已被告知不许在 turn 包新增 `TestMain`，也不许依赖它；CO1 不新写 `TestMain`。

  | 位置 | MD1 | CO1 | SM1 | 怎么处理 |
  |---|---|---|---|---|
  | `internal/repl/repl.go` 的 `REPL` 结构体末尾（`roundObs RoundObserver` 之后）与 `New` 字面量末尾（`rulesCtx: rulesCtx,` 之后） | 加三个字段、两行（B3） | 结构体末尾加两个字段 | 可能加总结相关字段 | 同一锚点。后落的一方按先落地后的真文件重取锚点 |
  | `internal/repl/repl.go` 的 import 块 | 加 `"time"` 与 `"council/internal/provider"` | 也加 `"time"` | 视其契约 | 后落的一方先看 `"time"` 是否已在，不许重复 import（重复会编译不过） |
  | `internal/repl/commands.go` | 只改 `cmdAdd`（头注释与函数开头两处，B4）；不改 `/add` 的 summary | 改命令表等 | 视其契约 | 函数不相交；锚点按落地时的真文件核对 |
  | `internal/turn` | 新增生产文件 `discovery_exec.go`；测试腿新增 `TestMain` | 一个文件都不碰 | 不许新增 `TestMain` | 只有 MD1 加 `TestMain` |
  | 配置文件 | `/add +编号` 把 `[agents."<id>"]` 追加到文件末尾 | 不碰 | 新增 `[summary]` 表；SM1 自己有一条 config 用例核对「以 `[summary]` 结尾的配置后面再接一段 agent 表，照样能加载」 | 不冲突。守卫是整份 `reflect.DeepEqual`，SM1 加的字段会自动一起比。但 R7 是按 Config 的字段一个一个钉的（W1b 修订后六个子测试对六个字段），SM1 加 `Summary` 之后，「守卫漏比 Summary」这种写法没有子测试钉着；要不要、由哪一刀补 R7 的 `summary` 子测试，**待主代理**（B2 注释要点 6、open item (s)） |
  | `internal/repl/ask.go`、`round.go`、`internal/app/app.go`、`cmd/council/main.go` | 一个字不碰 | 改 | SM1 改 `round.go` 的收尾 | MD1 不相交 |
- （v1 的说法，v1.1 的原型见下面「v1.1 变更记录」）本契约 v1 的全部期望值（屏幕逐字节、写出的配置文件字节、备份文件、库里的成员行、执行器行为）已在隔离拷贝 `E:\Isolating_test\council\md1_draft\` 里用一份原型实现（§1.1 的全部生产改动）实跑得出；**原型下全量 `go test -count=1 ./...` 八包全绿，既有用例零红、零改动**；§4 的变异（M68、M69 两条除外，见 §4 表后说明）在原型上逐条跑过（「原型实测」：原型的场景输出与金标准逐字节比对、或原型用例失败），全部被杀；§2 的杀手名是按原型场景对应到测试腿用例的。原型的 Go 执行器还对三家真 CLI 实跑过一次（claude 走隔离方式，见 §0 第 5 条）。原型不交付，已删除。

### v1.1 变更记录（2026-09-28，照主代理对 W1 的裁决书 `md1_rulings.md` 逐条落实）

v1.1 新增与改动的每一个期望值，都在 `E:\Isolating_test\council\md1_v11\` 重建的原型上实跑得出（git archive `28845e0`，§1.1 冻结代码由脚本从本文逐字取出落盘，§2 的用例照本文重写，期望值块也由脚本从本文取出，所以本文与原型是同一份字节）；每条新增变异都在原型上实跑，点名的杀手实际变红（「原型实测」）。F6、F7 两条没有派证伪者，下面标「未经证伪者」，下一轮审查请先看它们。

- **F0**（T1 的假 CLI 分不出「第一行就关 stdin」）：§2.2 `handshake` 模式改成两次检查（打任何东西之前一次，exit 7 不变；打完 system 帧之后再一次，新增 exit 8）；T1【判别力】补 S11；§4 新增 M76（W1 的 S11，杀手 T1，走 exit 8）。M20、M24 仍走 exit 7（原型实测）。
- **F1**（每家各自超时）：§2.0 假执行器冻结四步顺序，第 3 步「ctx 已结束就返回 `ctx.Err()`」放在记录调用之后；R6【判别力】补 S14，并写明超时的那一家不能排在最后；§4 新增 M77（W1 的 S14，杀手 R6 `each_kind`）。R6 四段期望值不因这一步变（原型实测）。
- **F2**（守卫只比 Agents）：§1.1 B2 冻结代码在整份 `DeepEqual` 之前加一支：`data_dir` 或 `rules_file` 与 `r.cfg` 不同时打新字面量「配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入」（登记 §3.2）。措辞与裁决书略有不同：不写「再 /add +N」，因为重启之后是新快照，编号要重新看。B2 注释要点 5、6 跟着改；R7 改成四个互不连带的子测试（`agent`、`store_thought`、`data_dir`、`rules_file`）；§4 新增 M78（只比 Agents，W1 的 S3）、M79（只比 Agents 与 Providers）、M80（去掉新的那一支）、M81（两支对调）、M82（新的那一支只判 data_dir，本轮「同类还有没有别处」补的）；M52 的杀手写细到子测试。〔W1b 修订：R7 再加 `teams`、`providers` 两个子测试，见「W1b 修订」第 1 条。〕
- **F3**（取消被缓存）：§1.1 B2 冻结代码三处：`takeModelSnapshot` 循环开头判**父 ctx**、已取消就 break；`printDiscoveredModels` 拍完之后父 ctx 已取消就不赋给 `r.modelSnap`，只打「识别被取消；下次 /add 会重新识别」；`printModelSnapshot` 在失败行之后，只要有失败，再打「失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh」（两句登记 §3.2）。B2 注释要点 1、2 改，新增注释要点 11（判父 ctx 不判 cctx、小竞态接受）；R6 四段末尾各多一行提示、每段再喂一次 `/add` 核对重印；新增 R14 `TestMdxAddCancelNotCached`；§4 新增 M83（守卫改成 false）、M84（去掉 break）、M85（改判 cctx：放在 `cancel()` 之后，循环开头的 cctx 不在作用域里、编译不过，所以取这个能编译的写法；杀手是 R1）、M86（提示行去掉）；M67、M77 的锚点按新代码重取。§6 真机复验在第一次 `/add` 时加一步 Ctrl-C：R14 只在 repl 层验过取消，TUI 那条路要真机看。〔W1b 修订：R14 拆成 `first`、`late`；§4 M85 的杀手描述按实跑订正，见「W1b 修订」第 2、4 条。〕
- **F4**（注释要点 4 / open item (c) 与代码不符）：按 V4 订正 B2 注释要点 4 与 open item (c)：准确判据是「拍快照那一刻还不在配置里、因而拿到编号的行」（含快照之后手改配置再 reload 进来的）；写明 `/add --refresh` 与新会话都会让这类行失去编号；去掉「Griffin 的 opus-5.0 / opus-5.0-max 就是这种写法」。「要不要给已配置的 claude 行也编号」列进 open item (c)，标**待 Griffin**；(d) role 空串、(e) `/help` 看不到新用法也标**待 Griffin**。不改代码。
- **F5**（快照跨 reload）：新增 R13 `TestMdxAddSnapshotAcrossReload`，按 V5 的更正写三个子测试：`alias`（`[agents.snt]` 整段冻结、带 `provider = 'claude'`，先断言 reload 整行前提）、`deleted`（识别部分整块比对 R1 第 2-27 行，调用次数仍 3，不用缺席断言）、`provider`（`sonnet-4.6` 去掉强度的配置变体，`/add +11` 加入）。§4 新增 M87（W1 的 S2）、M88（W1 的 S46）、M89（sameBrainAgent 去掉 provider 判据；V5 只推演过，本轮原型实跑确认：`+11` 被拒成同脑子 `sonnet-4.6`）。open item (i) 注明由 R13 钉着。
- **F6**（「识别出的模型都已经在配置里了」没人走到；**未经证伪者**）：R11 加子测试 (c)：配置只有 grok 一家、6 个模型都已配成 agent，配置文本与识别部分两行整段冻结；【判别力】写明它与 R6 `all_down` 是一对对照（`okCount > 0` 与 `== 0`）；§4 新增 M90（W1 的 S1，只红 R11 (c)）。
- **F7**（控制字符与空 model 过滤；**未经证伪者**）：P8 加第 15-18 行：grok 名字含 ESC、agy label 含 BEL、claude `resolvedModel` 含 BEL、claude `value` 与 `resolvedModel` 都空，期望都是 Empty。BEL 在 JSON 里写成 JSON 转义，裸字节会先让 JSON 解析失败。§4 新增 M91（W1 的 S4）、M92（S5）、M93（S6）、M94（S6b）。「同类还有没有别处」另补两行：第 20 行 grok 空名字（M95）、第 21 行 DEL 0x7f（M96）。
- **F8**：P8 加第 19 行：subtype 为 `error` 但带合法 `response.models`，期望 Unparseable；§4 新增 M97（W1 的 S39）。
- **F9**：B2 注释要点 7 改成：id 若与库里遗留的旧 agent 同名，`Join` 会重新激活那一行，旧 provider 会话在下一次 `/ask` 时按漂移处理（拒绝并提示重建），不会悄悄续上。不改代码。〔W1b 订正：这句只对「旧 agent 的配置与这次写的不同」成立；配置逐字相同时旧会话会被续上。注释要点 7 已改成两支，见下面「W1b 修订」第 3 条。〕
- **F10**：A4 注释要点 4 的理由改成「行宽没有上界……`bufio.Scanner` 超限会整体报错，所以用 `ReadBytes`，再按 `maxSpawnOutputBytes` 封顶」。不改代码。
- **F11**：§3.3 补上 `stdin pipe: %w`、`stdout pipe: %w`、`backup names … all taken` 三条，注明是冻结代码里的英文文本、会拼在别的行后面上屏。§1.1 B1 `writeBackup` 的报错文本改成 `backup names %s and %s-2..%d all taken`，补上不带后缀的名字；B1 注释要点 7 跟着改。新增 C6 `TestMdxWriteBackupAllTaken` 逐字钉住新文本与第 99 个名字这个边界；§4 新增 M98（循环边界，W1 当时判为「后果轻、不单列」的那一条）、M99（回到 v1 文本）。
- **F12**：§1.1 B2 `cmdAddDiscovered` 在 `r.modelSnap == nil` 时，配置里一家能识别的 provider 都没有，就打新字面量「配置里没有能识别模型的 provider，/add +编号 用不了」（登记 §3.2），不再叫用户先敲 `/add`。B2 注释要点 5 跟着改；R11 (b) 的期望改成这一行，【判别力】写明它与 R4 第一步是一对对照；§4 新增 M100（去掉这一支）、M101（这一支无条件成立）。
- **F13**：§1.1 B2 在 `AppendBlock` 报错且 `backup` 非空时打「写配置文件失败（原文件备份在 %s）: %v」（登记 §3.2），`backup` 为空时照旧 `reportErr`；B2 注释要点 5 补一句。新增 R15 `TestMdxAddWriteFailureShowsBackup`：把配置文件设成只读，Windows 上 `Rename` 覆盖只读目标会失败，这时备份已建好（本机实跑造得出，不用注入）。整行用锚定正则比对，只有 `CreateTemp` 的随机后缀是 `\d+`。§4 新增 M102；`backup` 为空那一支造不出确定的夹具，写进「只在审查时看」。〔W1b 订正：备份阶段那一支造得出，现由 R16 钉着，见「W1b 修订」第 5 条。〕
- **交付切分**（裁决书「交付切分」）：§5 写死 A-1 / A-2 / B-1 / B-2 四块，逐块列文件、冻结代码行数（原型实数）、加注释的估计、块间依赖与每块落地后跑的命令；grok 每次只交一块。与 v1 的「按文件再拆」退路相比，四个能力位补丁从 A-1 挪到 A-2，`configcmd.go` 头注释从 B-2 挪到 B-1，两处都只为平衡行数，依赖不变。A-1 仍有 417 行冻结代码，是最大的一块。〔W1b 修订：A-1 已拆成 A-1a、A-1b，见「W1b 修订」第 9 条。〕
- **相交表**（裁决书「与另两刀的协调」）：文件头原来那段「相交」换成相交表：写明落地顺序（MD1 与 CO1 并行、后落者重取 `repl.go` 锚点，SM1 在 CO1 之后）；只有 MD1 在 turn 包加 `TestMain`；CO1 也往 `repl.go` 的 import 加 `"time"`，后落者不许重复。
- **待 Griffin**（裁决书末节）：open item (c)（已配置的 claude 行要不要也编号、以便换强度）、(d)（新成员 role 写空串）、(e)（`/help` 里 `/add` 的 summary 不改，看不到 `+编号` 与 `--refresh`）三条标**待 Griffin**，不改方向。

#### W1b 修订（2026-09-28，逐条处理 W1b 第二轮审查的九条发现）

原型在 `E:\Isolating_test\council\md1_fix\`（git archive `28845e0`；§1.1 的冻结代码与补丁由脚本从本文逐字取出落盘；用完已删）。新增与改动的期望值都在这份原型上实跑得出，R1、R7、R14 的期望块由脚本从本文取出，不手抄。新增变异都实跑过，点名的杀手实际变红；实测范围见 §4 表后「W1b 修订的实测」。最后从改完的本文重新取代码、重建原型复核：生产代码 gofmt、go vet 干净；全量 `go test -count=1 ./...` 全绿，跑的是 HEAD 既有用例加本轮重写的四条（R1、R7、R14、R16）。v1.1 测试腿的其余用例（P、T、C 各条，R2-R6、R8-R13、R15）本轮没有落进原型。

本轮冻结代码只有一处变动：A3 拆成两个文件，逻辑一个字不改。会改变用户可见行为的修法一律没做，登记成 open item (p)-(s)，**待主代理**。

1. **发现 1（major，守卫「整份比」只钉住 Config 六个字段里的四个）：成立，照改。**
   - R7 加 `teams`、`providers` 两个子测试，期望都是「先 /config reload --apply」那一行（原型实跑）。原型另外实测：这两种改动 `/config reload --apply` 都会应用，所以那句是真话。
   - §4 新增 M103（整份比之前抹平 Teams）、M104（抹平 Providers），各只红对应的子测试；M109（只比 Agents 与 StoreThought，W1b 的 X3）红 `teams`、`providers`。M52、M78、M79 的杀手列按六个子测试重写。
   - B2 注释要点 6 改成「六个字段各有一个子测试」，并写明以后给 Config 加字段要同时加子测试。SM1 的 `Summary` 由哪一刀补子测试是跨契约的事，写进文件头相交表与 open item (s)，**待主代理**。
2. **发现 2（major，R14 只在第一家取消）：成立，照改。**
   - R14 拆成 `first`（现有写法）与 `late`（第 3 次调用、`mdx-grok.exe` 时取消）。`late` 的期望：同样两行、调用次数 3、`r.modelSnap == nil`；第二次 `/add` 的识别部分等于 R1 的 27 行，调用次数 6（原型实跑，`-count=5` 稳定）。
   - §4 新增 M105（W1b 的 X5，判 `okCount == 0`）、M106（X7，判 `len(snap.rows) == 0`）。「同类还有没有别处」另补 M107：只在循环提前结束时才算取消。三条都只有 `late` 杀得到。
   - 所以 `late` 的取消必须落在最后一家：落在中间一家时，循环会在 grok 之前 break，M107 就看不出来。这一条写进了 R14 的注释要点。
   - M83、M84 的杀手列按两个子测试订正（M84 只有 `first` 杀）。B2 注释要点 11 补一条：两处判断缺一不可。
3. **发现 3（major，注释要点 7 说「不会悄悄续上」，实测会续上）：成立，只改文字。**
   - 原型两支都实跑了。同 id、同配置重新加入：第二次 `/ask` 不打「新开了一条 provider 会话」，冷暖预报是暖，旧会话被续上。用 `/add +14 --effort max --id sonnet-5-high` 以另一个强度占回同一个 id：第二次 `/ask` 是 `outcome refused`。
   - 注释要点 7 改成两支；v1.1 变更记录 F9 条加了订正。这偏离了裁决书 F9 的原话，因为那句话本身与代码行为不符（裁决书通用纪律：契约的话不许与冻结代码不符）。
   - W1b 建议把探针冻结成一条验收，**不采纳**：它钉的是 `Join` 的 upsert 与会话指纹这些既有代码的行为，§4 没有任何 MD1 的变异会被它杀，写进来是一条没有判别力的验收。
4. **发现 4（minor，§4 M85 的描述与实测不符）：成立，照改。** 单独施加 M85、只跑 R1，实测识别部分只剩 `正在识别模型: agy, claude, grok` 一行，调用次数 1。§4 M85 的杀手列按实跑重写，并写明它几乎让每条 repl 用例都红，只是能编译的近似。v1.1 变更记录 F3 条的「杀手是 R1」本来就对，只加了指向这里的订正。
5. **发现 5（minor，`backup` 为空那一支造得出夹具）：成立，照改。**
   - 新增 R16 `TestMdxAddBackupStageFailure`：预建 [-2 s, +8 s] 时间窗里每一秒的 99 个备份名（1089 个文件），`/add +14` 就确定地在备份阶段失败。期望是一行锚定正则，只通配时间戳，两处必须相等（原型实跑，`-count=5` 稳定）。
   - §4 新增 M108（`if backup != "" {` -> `if true {`，W1b 的 X4），只红 R16。
   - 「只在审查时看」删掉备份阶段那半句，读原文件阶段的竞态保留；§3.3 补一句 R16 在屏幕上钉着那条英文文本；R16 写明了时间窗前提。
6. **发现 6（minor，注释要点 5 自相矛盾）：成立，只改文字。**
   - B2 注释要点 5 改成按阶段分：备份之前失败，盘上与内存都不动；备份阶段一般不留文件，但写到一半失败会留下残缺文件、路径不上屏；备份之后失败，备份留着、路径上屏；`Join` 失败不回滚。
   - B1 注释要点 7 补上残缺文件的如实描述。`writeBackup` 写失败时删掉那个文件要改冻结代码，登记 open item (p)，**待主代理**。
7. **发现 7（minor，claude 的强度档没过控制字符过滤；`hasControlChar` 不认 C1）：成立，不改代码。** 两种修法都改冻结代码，而且会改变哪些行、哪些强度被列出，属行为改动。登记 open item (q)，**待主代理**。
8. **发现 8（minor，守卫不做 Validate，「让它生效」在校验失败时不成立）：成立，不改代码。** B2 注释要点 6 补一句如实描述。在守卫里加 Validate 要加新字面量、改用户看到的行，登记 open item (r)，**待主代理**。
9. **发现 9（minor，A-1 超过 400 行）：成立，照改，但 W1b 的切法有错。**
   - W1b 说两半「都不依赖别的块、落地后各跑 go build」。按它的切法，上半的 `DiscoveryCommandFor` 引用下半的 `isClaudeDiscoveryResponse`，`ParseDiscovery` 引用三个解析器，两个文件互相依赖，哪一个单独都编译不过。
   - 本轮改成单向依赖：claude 那两个类型与 `claudeDiscoveryResponse`、`isClaudeDiscoveryResponse` 留在 `discovery.go`（265 行）；`DiscoverModels`、`ParseDiscovery`、三个解析器与 `hasControlChar` 挪到新文件 `discovery_parse.go`（162 行，§1.1 A3b）。代码逐字不变，只挪声明的位置；两个文件的 import 都是原来那六个。
   - 原型逐块实测：只落 A-1a 时 `go build ./internal/provider` 能过；A-1a 加 A-2、没有 A-1b 时 `go build ./...` 也能过。
   - 跟着改的地方：A3 注释要点按文件标了归属，新增第 15 条（`discovery_parse.go` 的文件头）；§4 表 M2-M5、M12-M18、M91-M97 的文件列改成 `discovery_parse.go`，每行锚点在新文件里恰好一处；§5 表拆成 A-1a、A-1b，写明单向依赖与块序；「只在审查时看」的 import 核对改成两个文件，并加一条「`discovery.go` 不引用下半」；文件头相交表、A 与 B 部分标题、§5 回执格式、§6 收割里的块名跟着改。
   - 「同类还有没有别处」：B-2 也有 430-470 行，但 `addmodel.go` 与 B3、B4 互相依赖，拆开的任何一半单独都编译不过，所以不拆；§5 写了 grok 超时时的退路。

待 Griffin 的仍是 open item (c)(d)(e) 三条，本轮没有新增。

---

## 0. 病根 / 动机（行号以 HEAD `ab53c0b` 为准）

1. **今天 `/add` 只能加配置里已有的 agent。** `cmdAdd`（`internal/repl/commands.go:824-910`）的候选是 `sortedKeys(r.cfg.Agents)` 去掉在场成员（`:841-846`），不带参数时 `printAddCandidates`（`:916-935`）只打这一张表。新增 agent 的唯一路径是手改 `config.toml` 再 `/config reload --apply`（`internal/repl/configcmd.go:3-7` 头注释、`docs/11d_STEP8C_CONTRACT.md:626-632`）。Griffin 要的是「Council 识别出订阅里有哪些模型，选一个直接加进来」（#45）。
2. **`ModelDiscovery` 能力位只在设计里。** `docs/02_AGENT_PROVIDER_ARCHITECTURE.md:64` 写了 `ModelDiscovery // Subcommand | ConfiguredList | Passthrough | Unknown`，`internal/provider/capabilities.go:108-171` 的 `Capabilities` 没有这一项；provider 包自己约定「不起进程、不读文件、不读环境变量」（`internal/provider/provider.go:13-15`）。
3. **现有起进程的函数做不了 claude 的握手。** 唯一的生产起进程路径是 `turn.spawnProcess`（`internal/turn/spawn.go:97-201`），stdin 要么是一次性给定的字节、要么是 null device（`:159-168`），进程结束才返回。claude 的识别要「写一行 initialize → 等到匹配的 control_response → 再关 stdin」，这个时序 `spawnProcess` 给不了；超时杀树的 `killProcessTree` 在 `turn` 包里未导出（`internal/turn/killtree_windows.go:17-28`）。所以真执行器只能住在 `turn` 包。
4. **id 规则散在三处，新 id 要一起过。** `internal/config/validate.go:25` 的 `idPattern`、`:38-41` 的保留名（`user` / `system`）；Windows 设备名 `Validate` 不拦（#37(h)，read 档的工作目录是 `filepath.Join(<data_dir>/workdir, agentID)`）；「只差点号与连字符」的两个 id 会落进同一个 claude 项目目录（#36(g)，实证：`.council` 的点号被换成 `-`；`_` 也被换成 `-`）。
5. **三家识别命令的实测（2026-09-27，本契约起草时在隔离目录实跑，只读、零推理、零花费）**：
   - `grok models`：0.38 s，退出码 0，stdout 191 字节（§2.0 的 grok 夹具就是这份原件），stderr 空。
   - `agy --output-format json models`（stdin 接 NUL）：4.1 s，退出码 0，stdout 1823 字节（§2.0 的 agy 夹具就是这份原件），**stderr 一行 `Fetching available models...`**。stdout 顶层的 `response` 是同一份清单的制表符文本、`conversation_id` 是空串 —— 不能拿 `ParseTurn` 去解（会报 `ErrNoSessionID`），只读 `command.data.models`。
   - claude 只用隔离方式（`CLAUDE_CONFIG_DIR` 指向临时目录、`ANTHROPIC_API_KEY` 是假 key、`ANTHROPIC_BASE_URL` 指向本地 sink，claude 2.1.283）：写入 initialize 之后 0.48 s 收到 `control_response`，**关 stdin 之后 0.165 s 退出、退出码 0**；stdout 3 行：一行 12 KB 的 `{"type":"system","subtype":"commands_changed",...}`、一行 17 KB 的 control_response、末尾换行；sink 只收到一个 `HEAD /api/hello`，没有 `/v1/messages`。`response.response.models[]` 每行有 `value / resolvedModel / displayName / description / supportsEffort / supportedEffortLevels / ...`；haiku 行没有 `supportedEffortLevels`。这次隔离探针里**没有出现 `disabled` 字段**（#45 与任务书要求「disabled 行不给选」，本契约照做，字段名 `disabled` 按 [未验证 字段名] 处理，夹具里那一行是合成的）。#45 说 Griffin 的真实登录跑出 10 行，但 #45 没有记下那 10 行原文 —— **本契约的 claude 夹具是隔离探针的形状（6 行）加 3 行合成行**，不是 Griffin 的原件。
   - 原型的 Go 执行器（§1.1 A4）对三家真 CLI 各跑一次（claude 同上隔离）：claude 5 行 0.79 s（sink 仍只收到 `HEAD /api/hello`）、agy 14 行 5.26 s、grok 6 行 0.34 s，建议 id 与本契约 §2.1 P7 的表一致。
6. **别名不等于识别结果。** Griffin 的配置里 opus 写的是 `model = 'opus'`（别名）；隔离探针里同一个模型是 `value: "opus[1m]"`、`resolvedModel: "claude-opus-5-5[1m]"`，两者都不等于 `opus`。「已在配置里」的判据若只比 `resolvedModel`，他已有的 opus 会被当成「没配置」列出来 —— 所以判据要同时比 `value`（§1.1 B2 `configuredAgentFor`）。反过来，Max 订阅下 value 很可能就是 `opus`，那时这一行会被正确地藏起来（§2.4 R12 是这对对照）。

---

## 1. 范围

### 1.1 做

**冻结粒度**：下面给出的代码是原型实跑过的版本。**标识符名、签名、判据、字面量、顺序全部冻结**（测试腿白盒引用 `r.discover`、`r.discoverTimeout`、`r.modelSnap`、`r.dispatch`、`writeBackup`）；只有「注释」由实现方写，必须覆盖每条列出的「注释要点」，注释不许写与代码不符的话。代码块里的缩进可能被排成空格，落盘一律 tab（gofmt）。锚点一律从 HEAD 真文件逐字复制。

#### A 部分（provider + turn；交付时拆成 A-1a、A-1b、A-2，见 §5）

**A1. `internal/provider/capabilities.go`**：`Capabilities` 结构体最后一个字段 `WriteBoundary Support`（`:170`）之后、结构体的 `}`（`:171`）之前加一个字段。锚点（真文件 `:170-171`）：

```go
	WriteBoundary Support
}
```

替换为：

```go
	WriteBoundary Support

	// （注释要点见下）
	ModelDiscovery ModelDiscovery
}
```

注释要点：这一位说的是「这家 CLI 能不能、以什么方式列出它能用的模型」（02 第 1.2 节 `:64` 设计过、#45 补上）；零值 `ModelDiscoveryUnknown`，与其它能力位同一条纪律（漏填 = 未实测，不是支持）；取值定义与三家的依据见 `discovery.go`。

**A2. 三家适配器的 `Capabilities()`**：各自在 `WriteBoundary:` 那一行之后、字面量的 `}` 之前加**至少一行注释**再加一行 `ModelDiscovery:`。`WriteBoundary:` 那一行**一个字节不动**：中间隔着注释行时 gofmt 不会把它与新行对齐（原型实测：没有注释行时 gofmt 会把它改成冒号后两个空格，有注释行时保持一个空格）。

| 文件 | 锚点（真文件行号，三行，含 tab） | 替换为 |
|---|---|---|
| `internal/provider/claude.go` | `:70-72`：`\t\tWriteBoundary: SupportYes,` / `\t}` / `}` | `\t\tWriteBoundary: SupportYes,` / `\t\t// <注释，一行或多行>` / `\t\tModelDiscovery: ModelDiscoveryControlHandshake,` / `\t}` / `}` |
| `internal/provider/agy.go` | `:68-70`：`\t\tWriteBoundary: SupportYes,` / `\t}` / `}` | `\t\tWriteBoundary: SupportYes,` / `\t\t// <注释>` / `\t\tModelDiscovery: ModelDiscoverySubcommand,` / `\t}` / `}` |
| `internal/provider/grok.go` | `:68-70`：`\t\tWriteBoundary: SupportNo,` / `\t}` / `}` | `\t\tWriteBoundary: SupportNo,` / `\t\t// <注释>` / `\t\tModelDiscovery: ModelDiscoverySubcommand,` / `\t}` / `}` |

（表里 `\t` 是真 tab、`/` 分隔行。）注释要点：claude = SDK 控制协议 initialize 握手（没有列表子命令，#45）；agy = `agy --output-format json models`，按账号从服务器拉、反映订阅，强度编在 label 里；grok = `grok models`，**只回显本机 grok 配置里手写的模型、不反映订阅**，真实清单要带 CPA 密钥问中转，#45 裁「只列本地配置、不碰密钥」。
**A3. 新文件 `internal/provider/discovery.go`**（逐字照抄下面的代码；注释由实现方按要点补。W1b 修订：v1.1 的 `discovery.go` 是一个 417 行的文件，按交付切分拆成本文件与 A3b 的 `discovery_parse.go` 两个。代码逐字不变，只挪了声明的位置；依赖是单向的：本文件单独能编译；`discovery_parse.go` 要用本文件的 `DiscoveredModel`、`ModelSource` 的三个非零取值、`DiscoveryExec`、五个 `ErrDiscovery*`、`DiscoveryCommandFor`、`claudeDiscoveryResponse`（连同它返回的 `claudeDiscoveryLine` 与 `claudeModelOption` 的字段）和常量 `claudeDiscoveryRequestID`，反过来不用（W1b 原型用脚本求交核对过这份清单）。265 行）：

```go
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ModelDiscovery 是 02 第 1.2 节设计过的能力位：这家 CLI 能不能、以什么方式
// 列出它能用的模型。零值 Unknown，与其它能力位同一条纪律。
type ModelDiscovery int

const (
	ModelDiscoveryUnknown          ModelDiscovery = iota
	ModelDiscoverySubcommand                      // agy --output-format json models / grok models
	ModelDiscoveryControlHandshake                // claude：SDK 控制协议 initialize 握手
	ModelDiscoveryUnsupported
)

// Available 报告这一位是不是一种能跑的识别方式。
func (d ModelDiscovery) Available() bool {
	return d == ModelDiscoverySubcommand || d == ModelDiscoveryControlHandshake
}

// ModelSource 是识别结果每一行的来源标注。
type ModelSource int

const (
	ModelSourceUnknown ModelSource = iota
	ModelSourceSubscription
	ModelSourceLocalConfig
	ModelSourceAgyQuota
)

// Label 是屏幕上的来源标注。
func (s ModelSource) Label() string {
	switch s {
	case ModelSourceSubscription:
		return "订阅"
	case ModelSourceLocalConfig:
		return "本地配置"
	case ModelSourceAgyQuota:
		return "走 agy 额度"
	default:
		return "-"
	}
}

// DiscoveredModel 是识别出的一行。
type DiscoveredModel struct {
	Provider string
	Model    string
	Alias    string
	Name     string
	Efforts  []string
	Source   ModelSource
}

// DefaultEffort 是加入时不指定强度用的档。
func (m DiscoveredModel) DefaultEffort() string {
	if len(m.Efforts) == 0 {
		return ""
	}
	for _, e := range m.Efforts {
		if e == "high" {
			return "high"
		}
	}
	return m.Efforts[0]
}

// SuggestedID 是建议的 agent id。
func (m DiscoveredModel) SuggestedID(effort string) string {
	switch m.Provider {
	case "claude":
		id := claudeIDBase(m.Model)
		if effort != "" {
			id += "-" + sanitizeModelID(effort)
		}
		return id
	case "agy":
		id := sanitizeModelID(m.Model)
		for _, other := range IDs() {
			if other == "agy" {
				continue
			}
			if strings.HasPrefix(id, other+"-") {
				return "agy-" + strings.TrimPrefix(id, other+"-")
			}
		}
		return id
	default:
		return sanitizeModelID(m.Model)
	}
}

// SuggestedDisplayName 是写进配置的 display_name。
func (m DiscoveredModel) SuggestedDisplayName(effort string) string {
	switch m.Provider {
	case "claude":
		name := prettyModelName(claudeIDBase(m.Model))
		if effort != "" && effort != "high" {
			name += " " + effort
		}
		return name
	case "agy":
		return "Agy " + m.Name
	default:
		return prettyModelName(sanitizeModelID(m.Model))
	}
}

func sanitizeModelID(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	sep := false
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' {
			if sep && b.Len() > 0 {
				b.WriteByte('-')
			}
			sep = false
			b.WriteRune(c)
			continue
		}
		sep = true
	}
	return strings.Trim(b.String(), "-.")
}

func claudeIDBase(model string) string {
	s := strings.TrimPrefix(strings.ToLower(model), "claude-")
	tail := ""
	if i := strings.IndexByte(s, '['); i >= 0 {
		tail = s[i:]
		s = s[:i]
	}
	parts := strings.Split(s, "-")
	if len(parts) > 1 && len(parts[len(parts)-1]) == 8 && allASCIIDigits(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}
	var out []string
	prevDigit := false
	for _, p := range parts {
		digit := allASCIIDigits(p)
		if digit && prevDigit {
			out[len(out)-1] += "." + p
		} else {
			out = append(out, p)
		}
		prevDigit = digit
	}
	base := sanitizeModelID(strings.Join(out, "-"))
	if t := sanitizeModelID(tail); t != "" {
		base += "-" + t
	}
	return base
}

func allASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func prettyModelName(s string) string {
	parts := strings.Split(s, "-")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if p[0] >= 'a' && p[0] <= 'z' {
			p = string(p[0]-'a'+'A') + p[1:]
		}
		out = append(out, p)
	}
	return strings.Join(out, " ")
}

// DiscoveryCommand 是一次识别要跑的外部命令。
type DiscoveryCommand struct {
	Path            string
	Args            []string
	Stdin           []byte
	CloseStdinAfter func(line []byte) bool
}

// DiscoveryExec 是可注入的执行器。
type DiscoveryExec func(ctx context.Context, cmd DiscoveryCommand) ([]byte, error)

var (
	ErrDiscoveryTimeout     = errors.New("model discovery timed out")
	ErrDiscoveryFailed      = errors.New("model discovery command failed")
	ErrDiscoveryUnparseable = errors.New("model discovery output is not a known shape")
	ErrDiscoveryEmpty       = errors.New("model discovery listed no models")
	ErrDiscoveryUnsupported = errors.New("provider has no model discovery")
)

const claudeDiscoveryRequestID = "council-models"

const claudeInitializeRequest = `{"type":"control_request","request_id":"council-models","request":{"subtype":"initialize"}}` + "\n"

// DiscoveryCommandFor 给出 providerID 这一家的识别命令。
func DiscoveryCommandFor(providerID, path string) (DiscoveryCommand, error) {
	switch providerID {
	case "claude":
		return DiscoveryCommand{
			Path:            path,
			Args:            []string{"-p", "--safe-mode", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config"},
			Stdin:           []byte(claudeInitializeRequest),
			CloseStdinAfter: isClaudeDiscoveryResponse,
		}, nil
	case "agy":
		return DiscoveryCommand{Path: path, Args: []string{"--output-format", "json", "models"}}, nil
	case "grok":
		return DiscoveryCommand{Path: path, Args: []string{"models"}}, nil
	default:
		return DiscoveryCommand{}, fmt.Errorf("provider %q: %w", providerID, ErrDiscoveryUnsupported)
	}
}

type claudeDiscoveryLine struct {
	Type     string `json:"type"`
	Response *struct {
		Subtype   string `json:"subtype"`
		RequestID string `json:"request_id"`
		Response  *struct {
			Models []claudeModelOption `json:"models"`
		} `json:"response"`
	} `json:"response"`
}

type claudeModelOption struct {
	Value                 string   `json:"value"`
	ResolvedModel         string   `json:"resolvedModel"`
	DisplayName           string   `json:"displayName"`
	SupportedEffortLevels []string `json:"supportedEffortLevels"`
	Disabled              bool     `json:"disabled"`
}

func claudeDiscoveryResponse(line []byte) (claudeDiscoveryLine, bool) {
	var v claudeDiscoveryLine
	if err := json.Unmarshal(bytes.TrimSpace(line), &v); err != nil {
		return claudeDiscoveryLine{}, false
	}
	if v.Type != "control_response" || v.Response == nil || v.Response.RequestID != claudeDiscoveryRequestID {
		return claudeDiscoveryLine{}, false
	}
	return v, true
}

func isClaudeDiscoveryResponse(line []byte) bool {
	_, ok := claudeDiscoveryResponse(line)
	return ok
}
```

**A3b. 新文件 `internal/provider/discovery_parse.go`**（逐字照抄；注释按要点补。162 行：`DiscoverModels`、`ParseDiscovery`、三家的解析器与 `hasControlChar`，都是从 v1.1 的 `discovery.go` 原样挪过来的）：

```go
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// DiscoverModels 跑一家的识别命令并解析。
func DiscoverModels(ctx context.Context, providerID, path string, exec DiscoveryExec) ([]DiscoveredModel, error) {
	cmd, err := DiscoveryCommandFor(providerID, path)
	if err != nil {
		return nil, err
	}
	out, err := exec(ctx, cmd)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrDiscoveryTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrDiscoveryFailed, err)
	}
	return ParseDiscovery(providerID, out)
}

// ParseDiscovery 解析一家识别命令的 stdout。
func ParseDiscovery(providerID string, stdout []byte) ([]DiscoveredModel, error) {
	switch providerID {
	case "claude":
		return parseClaudeDiscovery(stdout)
	case "agy":
		return parseAgyDiscovery(stdout)
	case "grok":
		return parseGrokDiscovery(stdout)
	default:
		return nil, fmt.Errorf("provider %q: %w", providerID, ErrDiscoveryUnsupported)
	}
}

func parseClaudeDiscovery(stdout []byte) ([]DiscoveredModel, error) {
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		v, ok := claudeDiscoveryResponse(line)
		if !ok {
			continue
		}
		if v.Response.Subtype != "success" || v.Response.Response == nil {
			return nil, fmt.Errorf("claude: control_response subtype %q: %w", v.Response.Subtype, ErrDiscoveryUnparseable)
		}
		var out []DiscoveredModel
		for _, o := range v.Response.Response.Models {
			if o.Value == "default" || o.Disabled {
				continue
			}
			model := o.ResolvedModel
			if model == "" {
				model = o.Value
			}
			if model == "" || hasControlChar(model) {
				continue
			}
			out = append(out, DiscoveredModel{
				Provider: "claude",
				Model:    model,
				Alias:    o.Value,
				Name:     o.DisplayName,
				Efforts:  o.SupportedEffortLevels,
				Source:   ModelSourceSubscription,
			})
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("claude: %w", ErrDiscoveryEmpty)
		}
		return out, nil
	}
	return nil, fmt.Errorf("claude: no control_response for request %s: %w", claudeDiscoveryRequestID, ErrDiscoveryUnparseable)
}

func parseAgyDiscovery(stdout []byte) ([]DiscoveredModel, error) {
	var v struct {
		Status  string `json:"status"`
		Command *struct {
			Name string `json:"name"`
			Data struct {
				Models []struct {
					ID    string `json:"id"`
					Label string `json:"label"`
				} `json:"models"`
			} `json:"data"`
		} `json:"command"`
	}
	if err := json.Unmarshal(stdout, &v); err != nil {
		return nil, fmt.Errorf("agy: stdout is not JSON: %w", ErrDiscoveryUnparseable)
	}
	if v.Status != "SUCCESS" || v.Command == nil || v.Command.Name != "models" {
		return nil, fmt.Errorf("agy: status %q: %w", v.Status, ErrDiscoveryUnparseable)
	}
	var out []DiscoveredModel
	for _, m := range v.Command.Data.Models {
		if m.Label == "" || hasControlChar(m.Label) {
			continue
		}
		src := ModelSourceAgyQuota
		if strings.HasPrefix(m.Label, "Gemini ") {
			src = ModelSourceSubscription
		}
		out = append(out, DiscoveredModel{
			Provider: "agy",
			Model:    m.Label,
			Alias:    m.ID,
			Name:     m.Label,
			Source:   src,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("agy: %w", ErrDiscoveryEmpty)
	}
	return out, nil
}

func parseGrokDiscovery(stdout []byte) ([]DiscoveredModel, error) {
	var out []DiscoveredModel
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimRight(line, "\r")
		var rest string
		switch {
		case strings.HasPrefix(line, "  * "):
			rest = line[4:]
		case strings.HasPrefix(line, "  - "):
			rest = line[4:]
		default:
			continue
		}
		name := strings.TrimSuffix(rest, " (default)")
		if name == "" || strings.ContainsAny(name, " \t") || hasControlChar(name) {
			continue
		}
		out = append(out, DiscoveredModel{
			Provider: "grok",
			Model:    name,
			Name:     name,
			Source:   ModelSourceLocalConfig,
		})
	}
	if len(out) == 0 {
		if bytes.Contains(stdout, []byte("Available models:")) {
			return nil, fmt.Errorf("grok: %w", ErrDiscoveryEmpty)
		}
		return nil, fmt.Errorf("grok: no model list: %w", ErrDiscoveryUnparseable)
	}
	return out, nil
}

func hasControlChar(s string) bool {
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}
```

A3 / A3b 的注释要点（每条都要有，放在对应声明上方；方括号里是它落在哪个文件，W1b 修订按拆分后的位置标）：

1. 〔`discovery.go`〕文件头：本文件是 02 第 1.2 节 `ModelDiscovery` 与第 1.3 节「能力发现」第一级（子命令）的落地；**本包仍然不起进程**（`provider.go:13-15` 的约定不变），外部命令一律经调用方注入的 `DiscoveryExec` 执行，生产实现是 `turn.RunDiscovery`，测试用夹具字节。识别输出的解析在 `discovery_parse.go`（第 15 条）。
2. 〔`discovery.go`〕`ModelDiscovery` 四个取值各一句；`Available` 只认两种能跑的方式。
3. 〔`discovery.go`〕`ModelSource.Label` 三种标注的来由：claude 行与 agy 的 Gemini 行是订阅；grok 行只反映本地配置；agy 清单里的 Claude / GPT-OSS 行走 agy 的额度（#45「显示并标注走 agy 额度」）。判「Gemini 行」的依据是 label 以 `Gemini ` 开头。
4. 〔`discovery.go`〕`DiscoveredModel` 各字段：`Model` 是**要写进配置 `model` 字段的值**；`Alias` 是同一模型的另一种写法（claude 的 `value`、agy 的 `id`），只用来判「已在配置里」；`Efforts` 只有 claude 有，键缺席时是 nil。
5. 〔`discovery_parse.go`，`parseClaudeDiscovery`〕**`Model` 取 `resolvedModel` 而不是 `value` 的理由**（claude）：`value` 多是别名（`sonnet`、`opus[1m]`），别名指向哪个模型由 CLI / 服务端决定、会漂；写别名的 agent 在别名漂走之后换了脑子而配置一个字没变，身份指纹（含 model 字段）不动，旧 provider 会话照样续上、`sender_model` 记的是别名。写 `resolvedModel` 等于把模型钉死：换模型只能新建 id（#45「换模型只新建 id」），指纹跟着变；而且它与输出里 `modelUsage` 的键是同一种写法（`claude-fable-5-1`），`ObservedModel` 与配置可以直接对上。`resolvedModel` 为空时退回 `value`。
6. 〔`discovery.go`〕`DefaultEffort`：支持 `high` 就是 `high`（#45「默认 high」），否则取该模型的第一档；没有强度（haiku）是空串。
7. 〔`discovery.go`〕`SuggestedID` 三家的规则（与 #45「模型-版本-强度」的写法对齐）：claude 去掉 `claude-` 前缀、去掉末尾 8 位日期段、相邻的纯数字段用点号连成版本号（`4-5` -> `4.5`）、方括号尾巴（`[1m]`）变成 `-1m`、再接 `-<强度>`；agy 把 label 规整成 id（小写，`[a-z0-9.]` 之外的连续字符换成一个 `-`，去掉首尾的 `-` 与 `.`），label 属于另一家已登记 provider 的模型家族（规整后以 `claude-` / `grok-` 开头）时把那个前缀换成 `agy-`（#45「只在两家重名时加来源前缀」，例：`Claude Opus 4.6 (Thinking)` -> `agy-opus-4.6-thinking`）；grok 把名字规整成 id。建议 id 不保证合法或不重名，合法性由 `config.NewAgentIDProblem` 在加入时判。
8. 〔`discovery.go`〕`SuggestedDisplayName`：claude 由 id 的模型段转成词首大写的名字，强度不是 `high`、也不空时在后面加一个空格和强度（与 Griffin 现有 `Opus 5.0` / `Opus 5.0 max` 的写法一致）；agy 是 `Agy ` + label；grok 由名字转。display_name 进身份块、进指纹（`internal/turn/render.go:27-33` 的订正），所以加入时就要定好，事后改要付一次入场费。
9. 〔`discovery.go`〕`DiscoveryCommand.CloseStdinAfter`：nil 表示执行器写完 `Stdin` 立刻关 stdin；非 nil 时执行器逐行读 stdout，第一次有一行让它返回 true 时才关 stdin。claude 的握手必须收到回应才关（#45，§0 第 5 条的实测：关 stdin 之后 0.165 s 正常退出）。
10. 〔哨兵在 `discovery.go`；映射写在 `discovery_parse.go` 的 `DiscoverModels` 上方〕五个哨兵各一句，并写明 `DiscoverModels` 的映射：执行器的错误链里有 `context.DeadlineExceeded` -> `ErrDiscoveryTimeout`；其它执行器错误 -> `ErrDiscoveryFailed`（执行器的错误文本用 `%v` 接在后面，不 `%w`）；解析不出形状 -> `ErrDiscoveryUnparseable`；形状对但一行可选的都没有 -> `ErrDiscoveryEmpty`（多半是没登录）；没有识别方式的 provider -> `ErrDiscoveryUnsupported`。
11. 〔`discovery.go`，`DiscoveryCommandFor` 与 `claudeDiscoveryResponse`〕claude 的命令行与 stdin 那一行：flag 照 #45 原文（`-p --safe-mode --input-format stream-json --output-format stream-json --verbose --no-session-persistence --strict-mcp-config`）；`--safe-mode` 关掉用户的 CLAUDE.md、skills、hooks、MCP，只留登录态；`request_id` 固定为 `council-models`；只认 `type == "control_response"` 且 `response.request_id` 匹配的那一行，别的行（`system` 帧、别的 request 的回应）一律跳过。
12. 〔`discovery_parse.go`〕`default` 行不单列（它只是「当前默认是谁」的指针，#45）；`disabled` 为真的行不给选（字段名未实测，见 §0 第 5 条）；`model` 为空或含控制字符的行跳过。
13. 〔`discovery_parse.go`〕agy：只读 `command.data.models[]{id,label}`，`status` 必须是 `SUCCESS`、`command.name` 必须是 `models`；label 就是配置里 `model` 的写法（#45）；为什么不走 `ParseTurn`（`conversation_id` 恒为空串）。
14. 〔`discovery_parse.go`〕grok：只认两个空格开头的 `  * <名字>` 与 `  - <名字>` 两种行，`* ` 那一行末尾的 ` (default)` 去掉；名字含空白就跳过；一行都没有时，输出里有 `Available models:` 算「查不到」、否则算「认不出」；行尾 `\r` 先剥掉。
15. 〔`discovery_parse.go`〕文件头：本文件是 A3 的下半（W1b 修订从 `discovery.go` 拆出来，只为交付时每块在 400 行上下，代码没有改）：`DiscoverModels`、`ParseDiscovery`、三家的解析器与 `hasControlChar`。它用 `discovery.go` 的类型、哨兵、`DiscoveryCommandFor`、`claudeDiscoveryResponse` 与 `claudeDiscoveryRequestID`（完整清单见 A3 抬头）；`discovery.go` 不用本文件的任何东西。与 `discovery.go` 同一条约定：本包不起进程，本文件也不 import `os/exec`、`os`。

**A4. 新文件 `internal/turn/discovery_exec.go`**（逐字照抄；注释按要点补）：

```go
package turn

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"council/internal/provider"
)

const maxDiscoveryStderrBytes = 64 << 10

const discoveryKillGrace = 5 * time.Second

// RunDiscovery 是 provider.DiscoveryExec 的生产实现。
func RunDiscovery(ctx context.Context, c provider.DiscoveryCommand) ([]byte, error) {
	if c.Path == "" {
		return nil, errors.New("discovery command path is empty")
	}
	path := c.Path
	if resolved, lookErr := exec.LookPath(path); lookErr == nil {
		path = resolved
	}

	cmd := exec.Command(path, c.Args...)
	cmd.Dir = os.TempDir()
	prepareCmd(cmd)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &capWriter{buf: &stderrBuf, cap: maxDiscoveryStderrBytes}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	var closeOnce sync.Once
	closeStdin := func() { closeOnce.Do(func() { _ = stdin.Close() }) }

	if len(c.Stdin) > 0 {
		_, _ = stdin.Write(c.Stdin)
	}
	if c.CloseStdinAfter == nil {
		closeStdin()
	}

	var outBuf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		br := bufio.NewReader(stdout)
		for {
			line, readErr := br.ReadBytes('\n')
			if len(line) > 0 {
				if remain := maxSpawnOutputBytes - outBuf.Len(); remain > 0 {
					if len(line) > remain {
						outBuf.Write(line[:remain])
					} else {
						outBuf.Write(line)
					}
				}
				if c.CloseStdinAfter != nil && c.CloseStdinAfter(line) {
					closeStdin()
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					_, _ = io.Copy(io.Discard, stdout)
				}
				break
			}
		}
		done <- cmd.Wait()
	}()

	select {
	case waitErr := <-done:
		closeStdin()
		if waitErr != nil {
			var ee *exec.ExitError
			if errors.As(waitErr, &ee) {
				if line := lastNonEmptyLine(stderrBuf.String()); line != "" {
					return outBuf.Bytes(), fmt.Errorf("exit code %d: %s", ee.ExitCode(), line)
				}
				return outBuf.Bytes(), fmt.Errorf("exit code %d", ee.ExitCode())
			}
			return outBuf.Bytes(), waitErr
		}
		return outBuf.Bytes(), nil
	case <-ctx.Done():
		if pid > 0 {
			_ = killProcessTree(pid)
		}
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		closeStdin()
		select {
		case <-done:
		case <-time.After(discoveryKillGrace):
		}
		return nil, fmt.Errorf("model discovery killed: %w", ctx.Err())
	}
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}
```

注释要点：

1. `RunDiscovery` 是 `provider.DiscoveryExec` 的生产实现，由 `repl.New` 装上；为什么不复用 `spawnProcess`（§0 第 3 条：它的 stdin 只能一次给定，给不了「收到回应再关」）。
2. 工作目录是 `os.TempDir()`：不继承编排器的启动目录，理由与 `spawnProcess` 拦空 `Dir`（`spawn.go:98-104`）相同 —— cwd 下的项目级指令文件会被读进去；识别用不着 Council 的 workdir，也不该为它建目录。
3. `Env` 不设（原样继承）：识别要用户真实的登录态，改 HOME 或 `CLAUDE_CONFIG_DIR` 会把登录态一起带走（与 `spawn.go:152` 同一个理由）。
4. stdout 逐行读，封顶复用 `maxSpawnOutputBytes`。行宽没有上界：claude 的 `system` 帧带着整份 commands 清单，随安装的命令增长，实测已有 12 KB 与 17 KB 的行。`bufio.Scanner` 超过它的单行上限会整体报错，所以用 `ReadBytes`，再按 `maxSpawnOutputBytes` 封顶；stderr 单独收、不并进 stdout（agy 成功时也往 stderr 打 `Fetching available models...`），只在退出码非 0 时取最后一个非空行拼进错误。
5. stdin 的关闭时机与 `CloseStdinAfter` 的约定（见 A3 注释要点 9）；进程结束后无论如何关一次。
6. 超时是主路径：ctx 结束的那一刻先 `killProcessTree` 再 `Kill`（Windows 上 `claude.cmd` 垫片之下才是干活的 node），再最多等 `discoveryKillGrace` 让读 goroutine 收尾；超时路径返回的错误用 `%w` 包住 `ctx.Err()`，上层据此判超时。
7. 退出码非 0 返回 `exit code N[: <stderr 最后一个非空行>]`，stdout 照样返回（调用方不用）。

#### B 部分（config + repl；交付时拆成 B-1、B-2，见 §5；B-2 依赖 A-1a、A-1b、A-2 的 `provider.DiscoverModels` 等与 `turn.RunDiscovery`）

**B1. 新文件 `internal/config/append.go`**（逐字照抄；注释按要点补）：

```go
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var windowsDeviceNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// NewAgentIDProblem 判断 id 能不能当一个新 agent 的 id。
func NewAgentIDProblem(c *Config, id string) string {
	const hint = "；用 --id <新 id> 换一个"
	if !idRegexp.MatchString(id) {
		return fmt.Sprintf("id %s 不合法（要匹配 %s）", id, idPattern) + hint
	}
	if reservedAgentIDs[id] {
		return fmt.Sprintf("id %s 是保留名", id) + hint
	}
	if windowsDeviceNames[strings.SplitN(id, ".", 2)[0]] {
		return fmt.Sprintf("id %s 是 Windows 保留设备名，不能当工作目录名", id) + hint
	}
	if _, ok := c.Agents[id]; ok {
		return fmt.Sprintf("配置里已经有 agent %s", id) + hint
	}
	fold := foldAgentID(id)
	for _, other := range sortedIDs(c.Agents) {
		if foldAgentID(other) == fold {
			return fmt.Sprintf("id %s 与已有的 agent %s 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录", id, other) + hint
		}
	}
	return ""
}

func foldAgentID(id string) string {
	b := []byte(id)
	for i, c := range b {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			b[i] = '-'
		}
	}
	return string(b)
}

// AgentBlockTOML 渲染追加到配置文件末尾的一段。
func AgentBlockTOML(id string, a AgentConfig, comment string) string {
	var b strings.Builder
	if comment != "" {
		b.WriteString("# ")
		for _, c := range comment {
			if c < 0x20 || c == 0x7f {
				c = ' '
			}
			b.WriteRune(c)
		}
		b.WriteString("\n")
	}
	if strings.Contains(id, ".") {
		b.WriteString("[agents.\"" + id + "\"]\n")
	} else {
		b.WriteString("[agents." + id + "]\n")
	}
	kv := func(k, v string) {
		b.WriteString(k + " = " + tomlBasicString(v) + "\n")
	}
	kv("display_name", a.DisplayName)
	kv("provider", a.Provider)
	kv("model", a.Model)
	if a.Effort != "" {
		kv("effort", a.Effort)
	}
	kv("role", a.Role)
	kv("permissions", string(a.Permissions))
	kv("mode", string(a.Mode))
	return b.String()
}

func tomlBasicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '"':
			b.WriteString(`\"`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, c)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

const maxBackupAttempts = 99

// AppendBlock 把 block 追加到 path 末尾：先备份，再临时文件加替换。
func AppendBlock(path, block string) (backupPath string, orig []byte, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("读原文件: %w", err)
	}
	orig, err = os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("读原文件: %w", err)
	}
	backupPath, err = writeBackup(path, orig, time.Now())
	if err != nil {
		return "", nil, fmt.Errorf("备份: %w", err)
	}
	sep := ""
	if len(orig) > 0 {
		if orig[len(orig)-1] == '\n' {
			sep = "\n"
		} else {
			sep = "\n\n"
		}
	}
	content := make([]byte, 0, len(orig)+len(sep)+len(block))
	content = append(content, orig...)
	content = append(content, sep...)
	content = append(content, block...)
	if err := writeFileAtomic(path, content, info.Mode().Perm()); err != nil {
		return backupPath, orig, fmt.Errorf("写入: %w", err)
	}
	return backupPath, orig, nil
}

// RestoreFile 把 path 原子地写回 orig。
func RestoreFile(path string, orig []byte) error {
	perm := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	return writeFileAtomic(path, orig, perm)
}

func writeBackup(path string, orig []byte, now time.Time) (string, error) {
	base := path + ".bak-add-" + now.Format("20060102-150405")
	for i := 1; i <= maxBackupAttempts; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(orig); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("backup names %s and %s-2..%d all taken", base, base, maxBackupAttempts)
}

func writeFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-add-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
```

注释要点：

1. 文件头：`/add +编号`（#45）把一段 `[agents."<id>"]` **追加**到生效配置文件末尾：不重写、原有字节一个不动（包括注释与 Griffin 的单引号写法）；写前备份；临时文件加改名的原子替换；写后由调用方 `LoadAndValidate`，失败用 `RestoreFile` 写回原字节。推翻了 `11d:626-632`，理由见 #45。
2. `windowsDeviceNames`：清单与 #37(h)；判的是 id 里**第一个点号之前**那一段（`nul.1`、`com1.x` 也拦）—— 本机 Windows 11 上带扩展名的已不算保留名（#37(h) 审查员核对过），这里偏保守，代价只是这几个名字不能用。
3. `NewAgentIDProblem` 五道检查的顺序（正则 -> 保留名 -> 设备名 -> 已存在 -> 折叠后撞名）与每一道的出处（`validate.go:25` / `:38-41`、#37(h)、配置里已有、#36(g)）；返回空串表示可用；每句末尾都提示 `--id`。
4. `foldAgentID`：claude 给工作目录起项目名时把字母数字之外的字符都换成 `-`（#36(g) 的实证是点号，本机也见过下划线），所以折叠规则是「`[a-z0-9]` 之外一律换 `-`」；只比新 id 与配置里已有的 id。
5. `AgentBlockTOML`：表名只在 id 含点号时加引号（与 Griffin 文件的写法一致：`[agents."grok-4.7"]`、`[agents.agy-flash-high]`）；值一律用 TOML 基本字符串、按 TOML 规则转义；`effort` 为空时不写这一行（与 grok / agy 成员的现有写法一致）；`role` 写空串（#45 没定 role，新成员的 role 留给 Griffin 自己填，填了会改指纹、要付一次入场费 —— 见 open item）；注释行里的控制字符换成空格。
6. `AppendBlock`：分隔规则（原文件空 -> 直接接；以 `\n` 结尾 -> 再补一个 `\n`，即空一行；否则补 `\n\n`）；**不检测 CRLF**（原文件是 CRLF 时追加段仍是 LF，TOML 两种都认）；三个阶段的错误各自加前缀 `读原文件: ` / `备份: ` / `写入: `；写入失败时备份已经在了，返回它的路径。
7. `writeBackup`：名字是 `<配置文件>.bak-add-<本地时间 YYYYMMDD-HHMMSS>`，撞名依次试 `-2` 到 `-99`；用 `O_EXCL` 建，**绝不覆盖**已有备份（同一秒里连加两个时，第二个备份的是第一次追加之后的文件，第一个备份才是原件）；与 Griffin 手工备份 `config.toml.bak-20260923` 的名字不相交。备份在成功之后也保留。99 个名字全被占时报错，错误文本里写出不带后缀的名字与 `-2..99` 两种。某个名字已经建好、写到一半（`Write` 或 `Close`）失败时，那个残缺的文件不删、返回空串（W1b 修订补的如实描述，open item (p)）。
8. `writeFileAtomic`：同目录临时文件（`<名字>.tmp-add-*`）写完、`Sync`、关闭、按原权限 `Chmod`、再 `Rename` 覆盖（Windows 上 `os.Rename` 会替换已存在的目标）；任何一步失败都删掉临时文件。

**B2. 新文件 `internal/repl/addmodel.go`**（逐字照抄；注释按要点补）：

```go
package repl

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

const discoveryTimeout = 20 * time.Second

const msgAddModelUsage = "用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别"

type modelSnapshot struct {
	rows     []snapRow
	failures []snapFailure
	okCount  int
}

type snapRow struct {
	no int
	m  provider.DiscoveredModel
}

type snapFailure struct {
	provider string
	err      error
}

func discoverableProviders(cfg *config.Config) []string {
	var out []string
	for _, pid := range sortedKeys(cfg.Providers) {
		p, err := provider.Lookup(pid)
		if err != nil {
			continue
		}
		if !p.Capabilities().ModelDiscovery.Available() {
			continue
		}
		out = append(out, pid)
	}
	return out
}

func configuredAgentFor(cfg *config.Config, m provider.DiscoveredModel) string {
	for _, id := range sortedKeys(cfg.Agents) {
		a := cfg.Agents[id]
		if a.Provider != m.Provider {
			continue
		}
		if a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias) {
			return id
		}
	}
	return ""
}

func sameBrainAgent(cfg *config.Config, m provider.DiscoveredModel, effort string) string {
	for _, id := range sortedKeys(cfg.Agents) {
		a := cfg.Agents[id]
		if a.Provider != m.Provider || a.Effort != effort {
			continue
		}
		if a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias) {
			return id
		}
	}
	return ""
}

func takeModelSnapshot(ctx context.Context, r *REPL, pids []string) *modelSnapshot {
	snap := &modelSnapshot{}
	next := 1
	for _, pid := range pids {
		if ctx.Err() != nil {
			break
		}
		cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout)
		models, err := provider.DiscoverModels(cctx, pid, r.cfg.Providers[pid].Command, r.discover)
		cancel()
		if err != nil {
			snap.failures = append(snap.failures, snapFailure{provider: pid, err: err})
			continue
		}
		snap.okCount++
		for _, m := range models {
			row := snapRow{m: m}
			if configuredAgentFor(r.cfg, m) == "" {
				row.no = next
				next++
			}
			snap.rows = append(snap.rows, row)
		}
	}
	return snap
}

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
	return printModelSnapshot(r, r.modelSnap)
}

func printModelSnapshot(r *REPL, snap *modelSnapshot) error {
	var visible []snapRow
	for _, row := range snap.rows {
		if row.no > 0 && configuredAgentFor(r.cfg, row.m) == "" {
			visible = append(visible, row)
		}
	}
	if len(visible) > 0 {
		if err := r.printf("识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:\n"); err != nil {
			return err
		}
		tw := r.newTabWriter()
		fmt.Fprintln(tw, "NO\tPROVIDER\tID\tMODEL\tEFFORTS\tSOURCE")
		for _, row := range visible {
			m := row.m
			fmt.Fprintf(tw, "+%d\t%s\t%s\t%s\t%s\t%s\n",
				row.no, m.Provider, m.SuggestedID(m.DefaultEffort()), m.Model, effortsCell(m.Efforts), m.Source.Label())
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if err := r.printf("用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档\n"); err != nil {
			return err
		}
	} else if snap.okCount > 0 {
		if err := r.printf("识别出的模型都已经在配置里了\n"); err != nil {
			return err
		}
	}
	for _, f := range snap.failures {
		if err := r.printf("识别 %s 的模型失败: %s\n", f.provider, discoveryFailureText(f.err, r.discoverTimeout)); err != nil {
			return err
		}
	}
	if len(snap.failures) > 0 {
		return r.printf("失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh\n")
	}
	return nil
}

func effortsCell(efforts []string) string {
	if len(efforts) == 0 {
		return emptyCell
	}
	return strings.Join(efforts, "|")
}

func discoveryFailureText(err error, timeout time.Duration) string {
	switch {
	case errors.Is(err, provider.ErrDiscoveryTimeout):
		return fmt.Sprintf("超时（%v 内没有回来）", timeout)
	case errors.Is(err, provider.ErrDiscoveryEmpty):
		return "一个模型都没列出来（没登录？）"
	case errors.Is(err, provider.ErrDiscoveryUnparseable):
		return "输出认不出来（CLI 版本变了？）"
	default:
		return err.Error()
	}
}

func (s *modelSnapshot) row(n int) (snapRow, bool) {
	if n < 1 {
		return snapRow{}, false
	}
	for _, row := range s.rows {
		if row.no == n {
			return row, true
		}
	}
	return snapRow{}, false
}

func cmdAddDiscovered(ctx context.Context, r *REPL, fields []string) error {
	tok := fields[0]
	if !isASCIIDigits(tok[1:]) {
		return r.printf("%s\n", msgAddModelUsage)
	}
	var newID, effort string
	haveID, haveEffort := false, false
	for i := 1; i < len(fields); i++ {
		switch fields[i] {
		case "--id":
			if haveID || i+1 >= len(fields) {
				return r.printf("%s\n", msgAddModelUsage)
			}
			haveID = true
			newID = fields[i+1]
			i++
		case "--effort":
			if haveEffort || i+1 >= len(fields) {
				return r.printf("%s\n", msgAddModelUsage)
			}
			haveEffort = true
			effort = fields[i+1]
			i++
		default:
			return r.printf("%s\n", msgAddModelUsage)
		}
	}
	if r.rn == nil {
		return r.printf("%s\n", msgNoRunner)
	}
	if r.modelSnap == nil {
		if len(discoverableProviders(r.cfg)) == 0 {
			return r.printf("配置里没有能识别模型的 provider，/add +编号 用不了\n")
		}
		return r.printf("还没有识别结果：先敲 /add 列出识别出的模型，再用 /add +编号 加入\n")
	}
	n, err := strconv.Atoi(tok[1:])
	row, ok := snapRow{}, false
	if err == nil {
		row, ok = r.modelSnap.row(n)
	}
	if !ok {
		return r.printf("识别结果里没有 %s，不带参数的 /add 能列出现有编号\n", tok)
	}
	m := row.m

	switch {
	case m.Provider != "claude":
		if haveEffort {
			return r.printf("%s 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort\n", m.Provider)
		}
		effort = ""
	case !haveEffort:
		effort = m.DefaultEffort()
	case len(m.Efforts) == 0:
		return r.printf("%s 没有可选的强度，不能带 --effort\n", m.Model)
	default:
		found := false
		for _, e := range m.Efforts {
			if e == effort {
				found = true
				break
			}
		}
		if !found {
			return r.printf("%s 支持的强度是 %s，没有 %s\n", m.Model, strings.Join(m.Efforts, "|"), effort)
		}
	}

	if dup := sameBrainAgent(r.cfg, m, effort); dup != "" {
		return r.printf("配置里已经有同一个模型、同一强度的 agent %s，直接 /add %s\n", dup, dup)
	}
	if !haveID {
		newID = m.SuggestedID(effort)
	}
	if problem := config.NewAgentIDProblem(r.cfg, newID); problem != "" {
		return r.printf("%s\n", problem)
	}

	path := r.rulesCtx.ConfigPath
	if path == "" {
		return r.printf("不知道配置文件在哪，没法写入\n")
	}
	onDisk, err := config.Load(path)
	if err != nil {
		return r.reportErr("读配置文件失败", err)
	}
	if onDisk.DataDir != r.cfg.DataDir || onDisk.RulesFile != r.cfg.RulesFile {
		return r.printf("配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入\n")
	}
	if !reflect.DeepEqual(onDisk, r.cfg) {
		return r.printf("配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add %s\n", tok)
	}

	ac := config.AgentConfig{
		DisplayName: m.SuggestedDisplayName(effort),
		Provider:    m.Provider,
		Model:       m.Model,
		Effort:      effort,
		Role:        "",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	}
	comment := fmt.Sprintf("council /add 追加：%s 识别出的 %s（%s）", m.Provider, m.Model, m.Source.Label())
	backup, orig, err := config.AppendBlock(path, config.AgentBlockTOML(newID, ac, comment))
	if err != nil {
		if backup != "" {
			return r.printf("写配置文件失败（原文件备份在 %s）: %v\n", backup, err)
		}
		return r.reportErr("写配置文件失败", err)
	}

	newCfg, err := config.LoadAndValidate(path)
	if err != nil {
		if rerr := config.RestoreFile(path, orig); rerr != nil {
			return r.printf("写入后的配置没通过加载或校验，恢复原文件也失败了（备份在 %s）: %v；%v\n", backup, err, rerr)
		}
		return r.printf("写入后的配置没通过加载或校验，已恢复原文件（备份在 %s）: %v\n", backup, err)
	}

	r.rn.SetConfig(newCfg)
	r.cfg = newCfg
	r.svc.SetThoughtPolicy(newCfg.StoreThought)

	if err := r.printf("已把 agent %s 追加进 %s（原文件备份在 %s）\n", newID, path, backup); err != nil {
		return err
	}
	joined := newCfg.Agents[newID]
	p := conversation.Participant{
		ConvID:      r.cur.ID,
		AgentID:     newID,
		DisplayName: joined.DisplayName,
		Provider:    joined.Provider,
		Active:      true,
		JoinedAt:    time.Now().UTC(),
	}
	if err := r.svc.Join(ctx, r.cur.ID, p); err != nil {
		return r.reportErr("加入失败", err)
	}
	return r.printf("%s 已加入会话 %s\n", newID, r.cur.ID)
}
```

注释要点：

1. 文件头：`/add` 的「识别出的模型」一半（#45 第一节）。识别只在 `/add`（不带参数）或 `/add --refresh` 时做、不在启动时做；每家一次、串行、各自带 `discoverTimeout` 超时；一家失败只印一行、不拦别家；识别中途被取消时不存快照（见注释要点 11）。
2. `modelSnapshot`：本会话的识别快照（识别被取消的那一次不存）。**编号在拍快照时定**（按 provider 字典序、每家按 CLI 的输出顺序，只给「拍快照那一刻还不在配置里」的行编号），之后每次显示都按**当前** `r.cfg` 过滤掉已经在配置里的行、**不重新编号**，所以两次 `/add` 之间编号不漂；`/config reload --apply` 不作废快照，只影响过滤；`/add --refresh` 才重拍。
3. `discoverableProviders`：只看配置里写了的 provider，按字典序；`provider.Lookup` 找不到（没有适配器，例如 codex）或 `ModelDiscovery` 不可用的一律静默跳过（#45「不接 codex」）。
4. `configuredAgentFor` 与 `sameBrainAgent` 的区别：前者判「这一行要不要显示」，只看 provider 与 model（或 `Alias`，§0 第 6 条）；后者判「这次加入是不是与已有成员同一个脑子」，再加上强度。所以某一行只要**拍快照那一刻还不在配置里、因而拿到了编号**，它的模型以某个强度加入、行被藏起来之后，`/add +编号 --effort <另一档>` 仍然能用同一个编号再加一个强度。这类行包括本会话 `/add +编号` 加进来的，也包括快照之后手改配置、再 `/config reload --apply` 进来的。拍快照时已在配置里的行根本没有编号，没有 +N 可用；`/add --refresh` 或开新会话重拍之后，上面那类行也算「已在配置里」，同样失去编号。给这些模型换强度，目前只能手改配置（open item (c)）。
5. `cmdAddDiscovered` 的顺序（参数形状 -> Runner 在不在 -> 有没有快照（没有时先看配置里有没有能识别的 provider，一家都没有就直说，不叫用户先敲 /add）-> 编号 -> 强度 -> 同脑子 -> id -> 配置文件路径 -> 守卫（先 data_dir / rules_file，再整份比）-> 追加 -> 加载校验 -> 换指针 -> Join）。每一步失败都只打一行；失败之后盘上与内存留下什么，按阶段分（W1b 修订：v1.1 这里写的「不改任何状态」与下面几种情况不符）：
    - 备份之前（参数到守卫，以及 `AppendBlock` 的读原文件阶段）：盘上文件与内存一个不动。
    - `AppendBlock` 的备份阶段：一般不留文件，照旧打 `写配置文件失败: 备份: …`（R16 走的是「99 个名字全被占」这一种）。例外：备份文件已经建好、写到一半失败（例如磁盘满）时，`writeBackup` 返回空串，那个残缺的备份文件留在盘上，路径不上屏（B1 注释要点 7、open item (p)）。
    - 备份之后：写入阶段失败时原文件没有被替换（临时文件加改名，改名失败原文件原样）；写后加载校验不过时用 `RestoreFile` 写回原字节。两种都留着备份文件，屏幕那一行带上它的路径，否则用户不知道同目录多了一个文件（R15、R8）。
    - 换指针之后只剩 `Join`：它失败时配置已经写进去、指针已经换了，不回滚（注释要点 9）。
6. 守卫：写之前按 `config.Load` 重读同一个文件，与 `r.cfg` 比，分两支，**顺序冻结**。先比 `DataDir` 与 `RulesFile`：不同就打「只能重启生效」那一行。这两个键 `/config reload` 会拒绝（`configcmd.go:124-126`），叫用户先 reload 是死胡同，所以单独一句；这一句不带编号，因为重启之后没有快照，编号要重新看。再做整份 `reflect.DeepEqual`：不相等说明文件在上次加载之后被改过（改的是注释之外的东西），这时追加会把用户没应用的改动一起生效，所以拒绝、让用户先 `/config reload --apply`。只改注释不算改过（按加载结果比，不按字节比）。必须整份比：漏比哪个字段，那个字段在文件里没应用的改动就会随这次追加悄悄换进内存。`Providers` 尤其要紧：`/config reload` 换 command 时会响亮告警（`providerCommandChanges`），这里一个字都不会打。R7 的六个子测试各只动 Config 的一个字段（W1b 修订）：`agent`、`store_thought`、`teams`、`providers` 钉整份比，`data_dir`、`rules_file` 钉前一支。以后给 Config 加字段（例如 SM1 的 `Summary`），要同时给 R7 加一个子测试，否则「漏比新字段」的写法没有用例抓（文件头相交表、open item (s)）。守卫只做 `config.Load`、不做 `Validate`：文件被改得通不过校验时，这里照样打「先 /config reload --apply 让它生效」，而 reload 会拒绝并列出校验错误，这半句在这种情况下不成立（open item (r)）。
7. 写后换指针与 `/config reload --apply` 同一套三句（`configcmd.go:149-155`：`r.rn.SetConfig`、`r.cfg =`、`r.svc.SetThoughtPolicy`），两处持有者一起换、绝不原地改 `r.cfg.Agents`；不跑 `AuditDrift`：追加的只是一张新表，守卫又保证写前文件与内存一致，已有成员的配置不变。新 id 若恰好与库里遗留的旧 agent 同名（以前用过、后来从配置删掉，库里还留着它的参与者行与 provider 会话），`Join` 是 upsert，会把那一行重新激活。旧 provider 会话怎么办，看它的身份指纹与新写的配置对不对得上（W1b 修订：v1.1 这里只写了「对不上」一支，原型实测两支都会发生）：
    - 对得上：例如当初也是 `/add +编号` 以同一模型、同一强度加的，写出的那一段逐字相同。下一次 `/ask` 续上旧会话（不打「新开了一条 provider 会话」）。同一个脑子、同一份身份，续上是对的。
    - 对不上：旧 agent 的 role、display_name、model 或强度与这次写的不同，例如用 `--id` 以另一个强度占回同一个 id。下一次 `/ask` 按漂移拒绝（`outcome refused`），要重建才能接着用。
    两种都不必在这里跑 `AuditDrift`。
8. 新成员固定 `permissions = read`、`mode = discussion`（#45）；`role` 为空串（见 B1 注释要点 5）；display_name 与 id 取 provider 包的建议（id 可用 `--id` 改）。
9. 加入失败（`Join` 报错）时配置已经写进去了、不回滚：配置本身合法，用户改天 `/add <id>` 就能加入。
10. `discoveryFailureText`：四种失败的中文说法；`ErrDiscoveryFailed` 与其它未归类的错误原样打 `err.Error()`。
11. 取消与失败的缓存（v1.1）：
    - TUI 里提交在跑时按 Ctrl-C 会取消这一次提交的 ctx（`tui/model.go:341-347`）。`takeModelSnapshot` 在循环开头判**父 ctx**，已取消就 break，后面几家不再起进程；`printDiscoveredModels` 拍完之后父 ctx 已取消，就不赋给 `r.modelSnap`，只打「识别被取消」一行，下次 `/add` 重新识别。
    - 判的必须是父 ctx，不能是每家自己的 `cctx`：`cancel()` 之后 `cctx.Err()` 永远非 nil，判它等于每家识别完就停。
    - 两处判断缺一不可（W1b 修订）：循环开头那一处管「取消之后不再起后面几家」；拍完之后那一处管「不存快照」。取消落在最后一家识别期间时，循环不会再回到开头，只有拍完之后那一处看得见它；这时前几家的成功行已经在快照里，照样整份丢掉，不能只在「一行都没拿到」或「循环提前结束」时才算取消（R14 `late`）。
    - 小竞态：三家都跑完之后才到的取消，也会丢掉这一份完整结果，代价只是下次多识别一次，接受。
    - 其它失败（「没登录？」、瞬时超时）照样随快照留到本会话结束，所以 `printModelSnapshot` 在失败行之后再打一行 `--refresh` 提示。这一行随快照每次重印；用户照「没登录？」登录之后再敲 `/add`，看到的仍是旧结果，得知道怎么重来。

**B3. `internal/repl/repl.go`**：三处。

- import 块（`:12-23`）。锚点：

  ```go
  	"io"
  	"text/tabwriter"

  	"council/internal/config"
  	"council/internal/conversation"
  	"council/internal/turn"
  )
  ```

  替换为：

  ```go
  	"io"
  	"text/tabwriter"
  	"time"

  	"council/internal/config"
  	"council/internal/conversation"
  	"council/internal/provider"
  	"council/internal/turn"
  )
  ```

- 结构体末尾（`:100-101`）。锚点：

  ```go
  	roundObs RoundObserver
  }
  ```

  替换为（字段名、类型、顺序冻结；每个字段上方一行注释，要点：`discover` 是识别模型用的执行器，`New` 装成 `turn.RunDiscovery`，测试换成夹具；`discoverTimeout` 是每一家识别的超时，`New` 装成 `discoveryTimeout`；`modelSnap` 是本会话的识别快照，nil 表示还没识别过）：

  ```go
  	roundObs RoundObserver

  	discover        provider.DiscoveryExec
  	discoverTimeout time.Duration
  	modelSnap       *modelSnapshot
  }
  ```

- `New` 的字面量（`:137-139`）。锚点：

  ```go
  		cmds:     newCommandTable(),
  		rulesCtx: rulesCtx,
  	}
  ```

  替换为：

  ```go
  		cmds:     newCommandTable(),
  		rulesCtx: rulesCtx,

  		discover:        turn.RunDiscovery,
  		discoverTimeout: discoveryTimeout,
  	}
  ```

  `New` 的签名不动（`app.go:224`、`:228` 与大量既有测试都按六个参数调它）。

**B4. `internal/repl/commands.go`**：`cmdAdd` 两处，加头注释要点。

- 函数开头（`:824-825`）。锚点：

  ```go
  func cmdAdd(ctx context.Context, r *REPL, args cmdArgs) error {
  	participants, err := r.svc.Participants(ctx, r.cur.ID, true)
  ```

  替换为：

  ```go
  func cmdAdd(ctx context.Context, r *REPL, args cmdArgs) error {
  	if len(args.fields) > 0 && strings.HasPrefix(args.fields[0], "+") {
  		return cmdAddDiscovered(ctx, r, args.fields)
  	}
  	refresh := false
  	if len(args.fields) > 0 && args.fields[0] == "--refresh" {
  		if len(args.fields) != 1 {
  			return r.printf("%s\n", msgAddModelUsage)
  		}
  		refresh = true
  		r.modelSnap = nil
  	}

  	participants, err := r.svc.Participants(ctx, r.cur.ID, true)
  ```

- 不带参数那一支（`:848-850`）。锚点：

  ```go
  	if len(args.fields) == 0 {
  		return printAddCandidates(r, candidates)
  	}
  ```

  替换为：

  ```go
  	if len(args.fields) == 0 || refresh {
  		if err := printAddCandidates(r, candidates); err != nil {
  			return err
  		}
  		return printDiscoveredModels(ctx, r)
  	}
  ```

- `cmdAdd` 的头注释（`:813-823`）补一段，要点：第一个参数以 `+` 开头时交给 `cmdAddDiscovered`（`addmodel.go`，识别出的模型加入，#45）；`--refresh` 丢掉识别快照、其余与不带参数相同；不带参数时先打配置里的候选（与之前逐字节相同），再打识别出的模型。原有「只加配置里已定义的 agent」那句改成「只加配置里已定义的 agent（识别出的模型先被追加进配置文件，再按配置加入）」。
- `/add` 与 `/join` 的 summary（`:143-144`）**不改**：`TestUx2rJoinIsAdd`（`ux2r_add_test.go:448-454`）逐字钉着；新用法由不带参数的 `/add` 在表后提示（§3.2 的提示行）。

**B5. `internal/repl/configcmd.go`**：只改头注释（`:3-7`），代码一个字不动。锚点（真文件 `:3-7`）：

```go
// configcmd.go 实现 /config 与 /config reload [--apply]（11d 契约 4.1）。
//
// 新增 agent 的路径是「改 config.toml -> /config reload --apply -> /join」，
// 不做运行时凭空加一个 agent：那要再写一套与 Config.Validate 重复的校验，
// 还要决定写不写回用户手写的 toml（写回丢注释与格式，不写回就是第二事实源）。
```

替换为（逐字）：

```go
// configcmd.go 实现 /config 与 /config reload [--apply]（11d 契约 4.1）。
//
// 新增 agent 有两条路：手改 config.toml -> /config reload --apply -> /join；
// 或者 /add +编号（addmodel.go）把识别出的模型整段追加到配置文件末尾，再走与
// 本文件 --apply 同一套换指针流程（r.rn.SetConfig、r.cfg、SetThoughtPolicy）。
// 11d 当初否决「运行时新增 agent」的两个理由（写回会丢注释与格式；不写回就是
// 第二事实源）被「只追加、落盘」回应了，00_DECISIONS.md #45 推翻了那条裁决。
// 追加之后仍由 Config.Validate 校验，不另写一套运行时校验。
```

### 1.2 不做（两条腿都不许越界）

- 不在启动时识别；`app.go`、`cmd/council/main.go`、`internal/tui` 一个字不改。
- 不碰 CPA 密钥、不读 `~/.grok` / `~/.claude` / `~/.gemini` 下的任何文件（识别只经三家 CLI 自己）；不接 codex（`discoverableProviders` 对没有适配器的 provider 静默跳过）。
- 不改任何已有 agent；不改 `config.Validate`（设备名与折叠撞名只在 `/add +编号` 这条路上拦，`Validate` 仍放过已有配置里的这类名字 —— 见 open item）；不改 `/ask` 的前缀规则、不改 `resolveAddSegment` 与 `printAddCandidates`（`/add 1,+2` 这种混写仍走旧路径，`+2` 那一段报「配置里没有可加入的 +2」）。
- 不改 `/add` / `/join` 的 summary；不加新命令；不给 `/config reload` 加任何行为。
- 不做并发识别（三家串行）；不做 CRLF 检测。
- 不改 `turn.spawnProcess`、`killtree_*.go`；不改 `provider.Provider` 接口（识别是包级函数，不是接口方法 —— 加接口方法会让所有测试假件编译不过）。
- **既有 `_test.go` 一律不改**（原型实测：全量既有用例零红，不需要任何授权修改）。测试腿不写生产代码；实现腿不写 `_test.go`。

---

## 2. 验收（测试腿新写的文件）

通用纪律：

- 新文件四到五个（§2.1-§2.4，repl 那份允许拆成「夹具」与「用例」两个文件）；包级标识符一律以 `mdx` 开头，测试函数名 `TestMdx...`。
- **不许调用真 provider CLI**：provider / repl 的用例一律注入假执行器（夹具字节）；turn 的执行器用例用测试二进制自己扮演 CLI（§2.2 的 `TestMain`）。store 一律 `store.Open` 到 `t.TempDir()` 之下，不碰 `C:\Users\25142\.council\data`；配置文件一律写在 `t.TempDir()` 里。
- **至少一条走真 store**：§2.4 的 repl 用例全部用真 SQLite（`store.Open` + `conversation.NewService`）、真配置文件（`config.LoadAndValidate`）、真 `turn.NewRunner`，R2 还经 `/ask` 让真 Runner 起一次假 CLI（repl 包既有 `TestMain` 的 `COUNCIL_FAKE_CLI=1`，`session_inflight_test.go:94`）。**app 包本刀不加用例**：`app.Run` 内部 `repl.New`、没有注入执行器的口子，而 app 包的假 CLI（`smoke_test.go:40`）只会回放一轮 claude 输出、做不了握手；为测试加注入口子要改 `app.go`，越界。
- 断言整行相等或整段字节相等，不写子串断言。例外只有这几个：`Fatalf` 前提守卫；R15、R16 的整行锚定正则（R15 是 v1.1 加的，只通配 `CreateTemp` 的随机后缀；R16 是 W1b 修订加的，只通配备份名里的时间戳、并断言两处时间戳相等；其余 `QuoteMeta` 逐字）。每条注释写【判别力】：它能不能为假、只差一处的对照是哪条。
- repl 用例的屏幕：用 `var out bytes.Buffer`、`r := New(cfg, svc, rn, strings.NewReader(""), &out, RulesContext{ConfigPath: cfgPath})`，然后**直接调 `r.dispatch(ctx, line)`**（白盒，同包）一行一行喂命令 —— 这样两条命令之间可以改盘上的文件；`dispatch` 不打提示符，输出按 `"\n"` 切行比较。`r.discover` 换成假执行器、`r.discoverTimeout = 200 * time.Millisecond` 都在第一次 `dispatch` 之前设。
- 允许只读引用同包既有测试辅助（不许改它们）：repl 的 `TestMain` / `COUNCIL_FAKE_CLI` 约定（`session_inflight_test.go:94-122`）、`newFakeStore` 等假件（R9 以外用不上）。
- **HEAD 上的状态**：四个文件都引用本刀新增的标识符，在 HEAD 上整包编译不过，这是预期的红。

### 2.0 共用夹具（冻结，逐字节）

**claude 夹具**（1586 字节，LF，末尾一个换行；第 1 行是 system 帧，第 2 行是 request_id 不匹配的诱饵回应，第 3 行是真回应；第 3 行里 `opus-legacy`（`disabled: true`）、`mdx-lowmed`（强度不含 high）、`claude-mdx-plain-1-2`（没有 `resolvedModel`）三行是合成的，其余 6 行照隔离探针的字段取值）：

```
{"type":"system","subtype":"commands_changed","commands":[]}
{"type":"control_response","response":{"subtype":"success","request_id":"mdx-other","response":{"models":[{"value":"decoy","resolvedModel":"claude-decoy-9","displayName":"Decoy"}]}}}
{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[{"value":"default","resolvedModel":"claude-opus-5-5[1m]","displayName":"Default (recommended)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"opus[1m]","resolvedModel":"claude-opus-5-5[1m]","displayName":"Opus (1M context)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"claude-fable-5-1","resolvedModel":"claude-fable-5-1","displayName":"Fable","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"sonnet","resolvedModel":"claude-sonnet-5","displayName":"Sonnet","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"sonnet[1m]","resolvedModel":"claude-sonnet-5[1m]","displayName":"Sonnet 5 (1M context)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"haiku","resolvedModel":"claude-haiku-4-5-20251001","displayName":"Haiku"},{"value":"opus-legacy","resolvedModel":"claude-opus-4-1","displayName":"Opus 4.1","supportedEffortLevels":["high"],"disabled":true},{"value":"mdx-lowmed","resolvedModel":"claude-mdx-lowmed-2-0","displayName":"Mdx","supportedEffortLevels":["low","medium"]},{"value":"claude-mdx-plain-1-2","displayName":"Plain","supportedEffortLevels":["high","max"]}]},"pending_permission_requests":[]}}
```

**agy 夹具**（1823 字节，2026-09-27 实跑 `agy --output-format json models` 的 stdout 原件，一行，末尾一个换行；注意 `response` 字段里的 `\t` `\n` 是 JSON 转义的两个字符）：

```
{"conversation_id":"","status":"SUCCESS","response":"gemini-3.8-flash-high\tGemini 3.8 Flash (High)\ngemini-3.8-flash-medium\tGemini 3.8 Flash (Medium)\ngemini-3.8-flash-low\tGemini 3.8 Flash (Low)\ngemini-3.7-flash-high\tGemini 3.7 Flash (High)\ngemini-3.7-flash-medium\tGemini 3.7 Flash (Medium)\ngemini-3.7-flash-low\tGemini 3.7 Flash (Low)\ngemini-3.6-flash-high\tGemini 3.6 Flash (High)\ngemini-3.6-flash-medium\tGemini 3.6 Flash (Medium)\ngemini-3.6-flash-low\tGemini 3.6 Flash (Low)\ngemini-3.1-pro-high\tGemini 3.1 Pro (High)\ngemini-3.1-pro-low\tGemini 3.1 Pro (Low)\nclaude-sonnet-4-6\tClaude Sonnet 4.6 (Thinking)\nclaude-opus-4-6-thinking\tClaude Opus 4.6 (Thinking)\ngpt-oss-120b-medium\tGPT-OSS 120B (Medium)\n","duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0},"command":{"name":"models","data":{"models":[{"id":"gemini-3.8-flash-high","label":"Gemini 3.8 Flash (High)"},{"id":"gemini-3.8-flash-medium","label":"Gemini 3.8 Flash (Medium)"},{"id":"gemini-3.8-flash-low","label":"Gemini 3.8 Flash (Low)"},{"id":"gemini-3.7-flash-high","label":"Gemini 3.7 Flash (High)"},{"id":"gemini-3.7-flash-medium","label":"Gemini 3.7 Flash (Medium)"},{"id":"gemini-3.7-flash-low","label":"Gemini 3.7 Flash (Low)"},{"id":"gemini-3.6-flash-high","label":"Gemini 3.6 Flash (High)"},{"id":"gemini-3.6-flash-medium","label":"Gemini 3.6 Flash (Medium)"},{"id":"gemini-3.6-flash-low","label":"Gemini 3.6 Flash (Low)"},{"id":"gemini-3.1-pro-high","label":"Gemini 3.1 Pro (High)"},{"id":"gemini-3.1-pro-low","label":"Gemini 3.1 Pro (Low)"},{"id":"claude-sonnet-4-6","label":"Claude Sonnet 4.6 (Thinking)"},{"id":"claude-opus-4-6-thinking","label":"Claude Opus 4.6 (Thinking)"},{"id":"gpt-oss-120b-medium","label":"GPT-OSS 120B (Medium)"}]}}}
```

**grok 夹具**（191 字节，2026-09-27 实跑 `grok models` 的 stdout 原件，LF，末尾一个换行）：

```
Model 'grok-4.7' is using its own API key.

Default model: grok-4.7

Available models:
  * grok-4.7 (default)
  - grok-4.6
  - grok-4.5
  - grok-4.3
  - grok-composer-2.5-fast
  - grok-build
```

**repl 用例的配置文件**（`<dataDir>` 是 `filepath.Join(dir, "data")`，`<exe>` 是 `os.Args[0]`，两处都用 TOML 单引号字面串原样拼进去；文件以 LF 结尾）：

```
# mdx 原有注释：这一行与下面每个字节都不许被改动
data_dir = '<dataDir>'

[providers.claude]
command = '<exe>'

[providers.agy]
command = 'mdx-agy.exe'

[providers.grok]
command = 'mdx-grok.exe'

[agents."opus-5.0"]
display_name = 'Opus 5.0'
provider     = 'claude'
model        = 'opus[1m]'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents."fable-5.1"]
display_name = 'Fable 5.1'
provider     = 'claude'
model        = 'claude-fable-5-1'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents."sonnet-4.6"]
display_name = 'Sonnet 4.6'
provider     = 'claude'
model        = 'claude-sonnet-4-6'
effort       = 'high'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents."grok-4.7"]
display_name = 'Grok 4.7'
provider     = 'grok'
model        = 'grok-4.7'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[agents.agy-flash-high]
display_name = 'Agy Gemini 3.8 Flash (High)'
provider     = 'agy'
model        = 'Gemini 3.8 Flash (High)'
role         = 'r'
permissions  = 'read'
mode         = 'discussion'

[teams.all]
display_name = '全员'
members = ['opus-5.0', 'grok-4.7']
```

**假执行器**（repl 用）：`func(ctx context.Context, c provider.DiscoveryCommand) ([]byte, error)`，按 `c.Path` 查表：`<exe>` -> claude 夹具、`mdx-agy.exe` -> agy 夹具、`mdx-grok.exe` -> grok 夹具。每次调用的顺序冻结（v1.1）：
1. 记下这一次的 `DiscoveryCommand`（加锁）。调用次数的断言只看这一步，与下面几步的位置无关。
2. 若用例装了钩子（只有 R14 装），调钩子。
3. `if err := ctx.Err(); err != nil { return nil, err }`：ctx 已经结束就不管查表结果，直接返回 ctx 的错误，照真执行器的样子（真执行器在 ctx 结束时杀进程、返回包住 `ctx.Err()` 的错误）。
4. 按查表结果返回。要模拟超时时：`select { case <-ctx.Done(): return nil, ctx.Err(); case <-time.After(3 * time.Second): return nil, errors.New("mdx: ctx never cancelled") }`（后一支是防挂，不是被测行为）。

第 3 步是 v1.1 加的：没有它，「三家共用一个 deadline」（M77）下排在超时那一家之后的 grok 照样回 `mdx boom`，R6 `each_kind` 看不出区别。

### 2.1 `internal/provider/mdx_discovery_test.go`（`package provider_test`，只用导出标识符）

**P1 `TestMdxCapabilitiesDeclareModelDiscovery`**：`provider.Lookup` 三家，`Capabilities().ModelDiscovery` 依次为 claude `ModelDiscoveryControlHandshake`、agy `ModelDiscoverySubcommand`、grok `ModelDiscoverySubcommand`；`provider.IDs()` 里出现这三家之外的 id 时 `t.Fatalf`（加第四家必须同时声明这一位）。`Available()`：Subcommand、ControlHandshake 为真，Unknown、Unsupported 为假。

**P2 `TestMdxDiscoveryCommandFor`**：`provider.DiscoveryCommandFor(<id>, "P")`：
- claude：`Path == "P"`；`Args` 等于 `[]string{"-p", "--safe-mode", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config"}`；`string(Stdin)` 等于 §3.4 那一行（含末尾 `\n`）；`CloseStdinAfter != nil`。
- agy：`Args` 等于 `[]string{"--output-format", "json", "models"}`；`Stdin == nil`；`CloseStdinAfter == nil`。
- grok：`Args` 等于 `[]string{"models"}`；`Stdin == nil`；`CloseStdinAfter == nil`。
- `"codex"`：`errors.Is(err, provider.ErrDiscoveryUnsupported)`。

**P3 `TestMdxClaudeCloseStdinPredicate`**：取 claude 的 `CloseStdinAfter`，对 claude 夹具按 `"\n"` 切出的四段（第 4 段是空串）依次为 `false, false, true, false`；对 `{"type":"control_request","request_id":"council-models"}` 为 `false`；对 `not json` 为 `false`。【判别力】第 2 段是诱饵（request_id 不匹配），只认 `control_response` 的写法在它上面为真。

**P4 `TestMdxParseClaudeFixture`**：`provider.ParseDiscovery("claude", claude 夹具)`，`err == nil`，结果与下面 7 个元素 `reflect.DeepEqual`（`Source` 全是 `provider.ModelSourceSubscription`；haiku 那行 `Efforts` 是 nil）：

| Model | Alias | Name | Efforts |
|---|---|---|---|
| `claude-opus-5-5[1m]` | `opus[1m]` | `Opus (1M context)` | `low medium high xhigh max` |
| `claude-fable-5-1` | `claude-fable-5-1` | `Fable` | `low medium high xhigh max` |
| `claude-sonnet-5` | `sonnet` | `Sonnet` | `low medium high xhigh max` |
| `claude-sonnet-5[1m]` | `sonnet[1m]` | `Sonnet 5 (1M context)` | `low medium high xhigh max` |
| `claude-haiku-4-5-20251001` | `haiku` | `Haiku` | nil |
| `claude-mdx-lowmed-2-0` | `mdx-lowmed` | `Mdx` | `low medium` |
| `claude-mdx-plain-1-2` | `claude-mdx-plain-1-2` | `Plain` | `high max` |

（`Provider` 全是 `"claude"`。）【判别力】不跳 `default`、不跳 `disabled`、不认 request_id、`Model` 取 `value`、不退回 `value` 五种写法各让这张表的某一行变（原型实测，§4 M1-M5）。

**P5 `TestMdxParseAgyFixture`**：`ParseDiscovery("agy", agy 夹具)` 14 行，逐行 `Provider "agy"`、`Model == Name ==` label、`Alias ==` id、`Efforts == nil`，顺序与夹具相同；前 11 行（label 以 `Gemini ` 开头）`Source == ModelSourceSubscription`，后 3 行（`Claude Sonnet 4.6 (Thinking)`、`Claude Opus 4.6 (Thinking)`、`GPT-OSS 120B (Medium)`）`Source == ModelSourceAgyQuota`。`ModelSource` 三个值的 `Label()` 依次是 `订阅`、`本地配置`、`走 agy 额度`，`ModelSourceUnknown.Label()` 是 `-`。

**P6 `TestMdxParseGrokFixture`**：`ParseDiscovery("grok", grok 夹具)` 6 行，`Model == Name`，依次 `grok-4.7`、`grok-4.6`、`grok-4.5`、`grok-4.3`、`grok-composer-2.5-fast`、`grok-build`；`Alias == ""`、`Efforts == nil`、`Source == ModelSourceLocalConfig`。另：把夹具里 `Available models:` 之后的三行换成 CRLF 的 `"Available models:\r\n  * grok-4.7 (default)\r\n  - grok-4.6\r\n"`，得 2 行 `grok-4.7`、`grok-4.6`。【判别力】` (default)` 不剥（M13）或只认 `  - `（M14）时第 1 行 `grok-4.7` 消失（repl 的 R1 杀不到这两条：它的配置里 grok-4.7 已在、这一行本来就被藏起来）。

**P7 `TestMdxSuggestedNames`**：对 P4 / P5 / P6 解析出的每一行，断言 `DefaultEffort()`、`SuggestedID(DefaultEffort())`、`SuggestedID("max")`、`SuggestedDisplayName(DefaultEffort())`、`SuggestedDisplayName("max")` 五个值（原型实测，逐字）：

```
claude-opus-5-5[1m]
    def="high" id="opus-5.5-1m-high" idmax="opus-5.5-1m-max" dn="Opus 5.5 1m" dnmax="Opus 5.5 1m max"
claude-fable-5-1
    def="high" id="fable-5.1-high" idmax="fable-5.1-max" dn="Fable 5.1" dnmax="Fable 5.1 max"
claude-sonnet-5
    def="high" id="sonnet-5-high" idmax="sonnet-5-max" dn="Sonnet 5" dnmax="Sonnet 5 max"
claude-sonnet-5[1m]
    def="high" id="sonnet-5-1m-high" idmax="sonnet-5-1m-max" dn="Sonnet 5 1m" dnmax="Sonnet 5 1m max"
claude-haiku-4-5-20251001
    def="" id="haiku-4.5" idmax="haiku-4.5-max" dn="Haiku 4.5" dnmax="Haiku 4.5 max"
claude-mdx-lowmed-2-0
    def="low" id="mdx-lowmed-2.0-low" idmax="mdx-lowmed-2.0-max" dn="Mdx Lowmed 2.0 low" dnmax="Mdx Lowmed 2.0 max"
claude-mdx-plain-1-2
    def="high" id="mdx-plain-1.2-high" idmax="mdx-plain-1.2-max" dn="Mdx Plain 1.2" dnmax="Mdx Plain 1.2 max"
Gemini 3.8 Flash (High)
    def="" id="gemini-3.8-flash-high" idmax="gemini-3.8-flash-high" dn="Agy Gemini 3.8 Flash (High)" dnmax="Agy Gemini 3.8 Flash (High)"
Gemini 3.8 Flash (Medium)
    def="" id="gemini-3.8-flash-medium" idmax="gemini-3.8-flash-medium" dn="Agy Gemini 3.8 Flash (Medium)" dnmax="Agy Gemini 3.8 Flash (Medium)"
Gemini 3.8 Flash (Low)
    def="" id="gemini-3.8-flash-low" idmax="gemini-3.8-flash-low" dn="Agy Gemini 3.8 Flash (Low)" dnmax="Agy Gemini 3.8 Flash (Low)"
Gemini 3.7 Flash (High)
    def="" id="gemini-3.7-flash-high" idmax="gemini-3.7-flash-high" dn="Agy Gemini 3.7 Flash (High)" dnmax="Agy Gemini 3.7 Flash (High)"
Gemini 3.7 Flash (Medium)
    def="" id="gemini-3.7-flash-medium" idmax="gemini-3.7-flash-medium" dn="Agy Gemini 3.7 Flash (Medium)" dnmax="Agy Gemini 3.7 Flash (Medium)"
Gemini 3.7 Flash (Low)
    def="" id="gemini-3.7-flash-low" idmax="gemini-3.7-flash-low" dn="Agy Gemini 3.7 Flash (Low)" dnmax="Agy Gemini 3.7 Flash (Low)"
Gemini 3.6 Flash (High)
    def="" id="gemini-3.6-flash-high" idmax="gemini-3.6-flash-high" dn="Agy Gemini 3.6 Flash (High)" dnmax="Agy Gemini 3.6 Flash (High)"
Gemini 3.6 Flash (Medium)
    def="" id="gemini-3.6-flash-medium" idmax="gemini-3.6-flash-medium" dn="Agy Gemini 3.6 Flash (Medium)" dnmax="Agy Gemini 3.6 Flash (Medium)"
Gemini 3.6 Flash (Low)
    def="" id="gemini-3.6-flash-low" idmax="gemini-3.6-flash-low" dn="Agy Gemini 3.6 Flash (Low)" dnmax="Agy Gemini 3.6 Flash (Low)"
Gemini 3.1 Pro (High)
    def="" id="gemini-3.1-pro-high" idmax="gemini-3.1-pro-high" dn="Agy Gemini 3.1 Pro (High)" dnmax="Agy Gemini 3.1 Pro (High)"
Gemini 3.1 Pro (Low)
    def="" id="gemini-3.1-pro-low" idmax="gemini-3.1-pro-low" dn="Agy Gemini 3.1 Pro (Low)" dnmax="Agy Gemini 3.1 Pro (Low)"
Claude Sonnet 4.6 (Thinking)
    def="" id="agy-sonnet-4.6-thinking" idmax="agy-sonnet-4.6-thinking" dn="Agy Claude Sonnet 4.6 (Thinking)" dnmax="Agy Claude Sonnet 4.6 (Thinking)"
Claude Opus 4.6 (Thinking)
    def="" id="agy-opus-4.6-thinking" idmax="agy-opus-4.6-thinking" dn="Agy Claude Opus 4.6 (Thinking)" dnmax="Agy Claude Opus 4.6 (Thinking)"
GPT-OSS 120B (Medium)
    def="" id="gpt-oss-120b-medium" idmax="gpt-oss-120b-medium" dn="Agy GPT-OSS 120B (Medium)" dnmax="Agy GPT-OSS 120B (Medium)"
grok-4.7
    def="" id="grok-4.7" idmax="grok-4.7" dn="Grok 4.7" dnmax="Grok 4.7"
grok-4.6
    def="" id="grok-4.6" idmax="grok-4.6" dn="Grok 4.6" dnmax="Grok 4.6"
grok-4.5
    def="" id="grok-4.5" idmax="grok-4.5" dn="Grok 4.5" dnmax="Grok 4.5"
grok-4.3
    def="" id="grok-4.3" idmax="grok-4.3" dn="Grok 4.3" dnmax="Grok 4.3"
grok-composer-2.5-fast
    def="" id="grok-composer-2.5-fast" idmax="grok-composer-2.5-fast" dn="Grok Composer 2.5 Fast" dnmax="Grok Composer 2.5 Fast"
grok-build
    def="" id="grok-build" idmax="grok-build" dn="Grok Build" dnmax="Grok Build"
```

**P8 `TestMdxParseRejects`**（表驱动，每行给 provider、输入字节、期望哨兵；全部 `len(结果) == 0`）：

| # | provider | 输入（Go 字符串字面量） | 期望 |
|---|---|---|---|
| 1 | claude | `"{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"mdx-other\",\"response\":{\"models\":[{\"value\":\"x\",\"resolvedModel\":\"claude-x-1\"}]}}}\n"` | Unparseable |
| 2 | claude | `"{\"type\":\"control_response\",\"response\":{\"subtype\":\"error\",\"request_id\":\"council-models\",\"error\":\"boom\"}}\n"` | Unparseable |
| 3 | claude | `"{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[]}}}\n"` | Empty |
| 4 | claude | `"{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"default\",\"resolvedModel\":\"claude-a-1\"},{\"value\":\"b\",\"resolvedModel\":\"claude-b-1\",\"disabled\":true}]}}}\n"` | Empty |
| 5 | claude | `""` | Unparseable |
| 6 | agy | `"not json"` | Unparseable |
| 7 | agy | `"{\"status\":\"ERROR\",\"command\":{\"name\":\"models\",\"data\":{\"models\":[{\"id\":\"a\",\"label\":\"A\"}]}}}"` | Unparseable |
| 8 | agy | `"{\"status\":\"SUCCESS\"}"` | Unparseable |
| 9 | agy | `"{\"status\":\"SUCCESS\",\"command\":{\"name\":\"agents\",\"data\":{\"models\":[{\"id\":\"a\",\"label\":\"A\"}]}}}"` | Unparseable |
| 10 | agy | `"{\"status\":\"SUCCESS\",\"command\":{\"name\":\"models\",\"data\":{\"models\":[]}}}"` | Empty |
| 11 | agy | `"{\"status\":\"SUCCESS\",\"command\":{\"name\":\"models\",\"data\":{\"models\":[{\"id\":\"a\",\"label\":\"\"}]}}}"` | Empty |
| 12 | grok | `""` | Unparseable |
| 13 | grok | `"Available models:\n"` | Empty |
| 14 | grok | `"Available models:\n  - bad name\n"` | Empty |
| 15 | grok | `"Available models:\n  - a\x1bb\n"` | Empty |
| 16 | agy | `"{\"status\":\"SUCCESS\",\"command\":{\"name\":\"models\",\"data\":{\"models\":[{\"id\":\"a\",\"label\":\"A\\u0007\"}]}}}"` | Empty |
| 17 | claude | `"{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"x\",\"resolvedModel\":\"claude-x\\u0007\"}]}}}\n"` | Empty |
| 18 | claude | `"{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"\",\"resolvedModel\":\"\",\"displayName\":\"E\"}]}}}\n"` | Empty |
| 19 | claude | `"{\"type\":\"control_response\",\"response\":{\"subtype\":\"error\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"x\",\"resolvedModel\":\"claude-x-1\"}]}}}\n"` | Unparseable |
| 20 | grok | `"Available models:\n  - \n"` | Empty |
| 21 | grok | `"Available models:\n  - a\x7fb\n"` | Empty |

「Unparseable」断言 `errors.Is(err, provider.ErrDiscoveryUnparseable) && !errors.Is(err, provider.ErrDiscoveryEmpty)`，「Empty」反之。另 `ParseDiscovery("codex", ...)` -> `ErrDiscoveryUnsupported`。【判别力】第 7 行的 models 是合法的，去掉 `status` 判据（M16）时它解析出 1 行；第 12、13 行只差「有没有 `Available models:`」（M15 对调两者时两行一起红）。
- 第 15-18 行（v1.1，F7）各让一个防御过滤单独负责，每一行里的那个模型都是这一次调用唯一的模型，过滤掉就是 Empty：
  - 15：grok 名字含 ESC；
  - 16：agy label 含 BEL；
  - 17：claude `resolvedModel` 含 BEL；
  - 18：claude 的 `value` 与 `resolvedModel` 都是空串。
- 第 16、17 行里的 BEL 在 JSON 文本里是 JSON 的 unicode 转义（字面量里是两个反斜杠加 `u0007`），不是裸控制字节。裸字节会让 `json.Unmarshal` 先失败、判成 Unparseable，走不到过滤。
- 第 19 行（v1.1，F8）subtype 是 `error`，但带着一份合法的 models：去掉 subtype 判据（M97）时解析出 1 行。
- 第 20、21 行是本轮「同类还有没有别处」补的：grok 的空名字（`  - ` 后面什么都没有，M95），以及 DEL 0x7f（`hasControlChar` 的第二个判据，M96）。
- v1.1 原型实测，以下变异都只红 P8：M91 红第 15、21 行；M92 红第 16 行；M93 红第 18 行；M94 红第 17 行；M95 红第 20 行；M96 红第 21 行；M97 红第 19 行。

**P9 `TestMdxDiscoverModelsMapsExecErrors`**：`provider.DiscoverModels(ctx, "grok", "P", exec)`：
- exec 返回 `fmt.Errorf("killed: %w", context.DeadlineExceeded)`：`err.Error() == "model discovery timed out: killed: context deadline exceeded"`、`errors.Is(err, ErrDiscoveryTimeout)`、`!errors.Is(err, ErrDiscoveryFailed)`。
- exec 返回 `errors.New("mdx boom")`：`err.Error() == "model discovery command failed: mdx boom"`、`errors.Is(err, ErrDiscoveryFailed)`、`!errors.Is(err, ErrDiscoveryTimeout)`。
- exec 返回 `fmt.Errorf("x: %w", context.Canceled)`：`errors.Is(err, ErrDiscoveryFailed)`（取消不是超时）。
- exec 返回 grok 夹具：6 行（同 P6）；exec 收到的 `DiscoveryCommand` 的 `Path` / `Args` / `Stdin` 与 `DiscoveryCommandFor("grok", "P")` 相等。
- `DiscoverModels(ctx, "codex", "P", exec)`：`ErrDiscoveryUnsupported`，且 exec **一次都没被调**。

### 2.2 `internal/turn/mdx_discovery_exec_test.go`（`package turn`；本包 HEAD 没有 `TestMain`，这个文件加一个）

`TestMain`：环境变量 `MDX_FAKE_DISCOVERY` 为空时 `os.Exit(m.Run())`；否则测试二进制扮演识别 CLI，按取值（冻结）：

- `handshake`（v1.1 改成两次检查，步骤冻结）：
  1. `bufio.NewReader(os.Stdin)` 读第一行（含 `\n`）存为 `first`；另起 goroutine `io.ReadAll` 剩余部分、读完关一个 channel `eof`。
  2. 打任何东西之前 `select` 等 `eof` 或 300 ms：`eof` 先到就打 `mdx-stdin-closed-before-response\n` 并 `os.Exit(7)`。
  3. 打印 `{"type":"system","subtype":"commands_changed"}\n`。
  4. 再 `select` 一次，等 `eof` 或 300 ms：`eof` 先到就打 `mdx-stdin-closed-after-system-frame\n` 并 `os.Exit(8)`。
  5. 打印 `fmt.Sprintf("{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"sonnet\",\"resolvedModel\":\"claude-sonnet-5\",\"displayName\":\"Sonnet\"}]}},\"mdx_first\":%q}\n", first)`。
  6. 等 `eof`，打印 `mdx-after-eof\n`，`os.Exit(0)`。

  四种执行器行为各走各的出口：写完就关 stdin 走 exit 7；stdout 来第一行就关走 exit 8；永不关等满 10 s 超时；收到匹配的 control_response 才关走 exit 0。只查一次（只留第 4 步）不行：写完就关的执行器下，第一行字节已经在管道里，假 CLI 照样读到 `first`、打完 system 帧才看见 eof，会走 exit 8，与 M20、M24 的「exit 7」对不上。
- `readall`：`io.ReadAll(os.Stdin)`，打印 `fmt.Sprintf("mdx-readall stdin=%d args=%q\n", len(b), os.Args[1:])`，退出 0。
- `fail`：stderr 写 `mdx first\nmdx last line\n\n`，stdout 写 `mdx partial stdout\n`，`os.Exit(3)`。
- `failsilent`：`os.Exit(4)`。
- `hang`：`time.Sleep(60 * time.Second)`。
- `cwd`：打印 `os.Getwd()` 的结果（不加换行），退出 0。

用例（每个都 `t.Setenv("MDX_FAKE_DISCOVERY", <模式>)`，`Path` 一律 `os.Args[0]`）：

- **T1 `TestMdxRunDiscoveryClaudeHandshake`**：`cmd, _ := provider.DiscoveryCommandFor("claude", os.Args[0])`，10 s 超时的 ctx，`out, err := RunDiscovery(ctx, cmd)`：`err == nil`；`string(out)` 逐字节等于 `{"type":"system","subtype":"commands_changed"}\n` + 上面那行 control_response（`mdx_first` 取 §3.4 那一行经 `%q` 的结果）+ `mdx-after-eof\n`；`provider.ParseDiscovery("claude", out)` 得 1 行（`Model "claude-sonnet-5"`）。【判别力】执行器写完就关 stdin（M24）或 claude 的 `CloseStdinAfter` 为 nil（M20）时假 CLI 走 exit 7、`out` 是 `mdx-stdin-closed-before-response\n`；stdout 来第一行就关 stdin、不看是不是匹配的回应（M76，W1 的 S11）时假 CLI 走 exit 8，`err.Error() == "exit code 8"`、`out` 是 `{"type":"system","subtype":"commands_changed"}\n` + `mdx-stdin-closed-after-system-frame\n`（v1.1 原型实测，三条各自只红 T1，M20 另红 P2、P3）；执行器永不关 stdin 时假 CLI 等不到 EOF，10 s 超时报错（原型实测）。
- **T2 `TestMdxRunDiscoveryClosesStdinImmediately`**：`DiscoveryCommandFor("agy", os.Args[0])`，10 s ctx：`err == nil`，`string(out) == "mdx-readall stdin=0 args=[\"--output-format\" \"json\" \"models\"]\n"`；另一次 `DiscoveryCommand{Path: os.Args[0], Args: []string{"x"}, Stdin: []byte("abc")}`：`"mdx-readall stdin=3 args=[\"x\"]\n"`。【判别力】`CloseStdinAfter == nil` 时不关 stdin（M23）则 10 s 超时。
- **T3 `TestMdxRunDiscoveryNonZeroExit`**：`fail`：`err.Error() == "exit code 3: mdx last line"`、`string(out) == "mdx partial stdout\n"`；`failsilent`：`err.Error() == "exit code 4"`、`len(out) == 0`。
- **T4 `TestMdxRunDiscoveryTimeoutKillsTree`**：`hang`，300 ms ctx，量 `RunDiscovery` 的耗时：`errors.Is(err, context.DeadlineExceeded)`、`err.Error() == "model discovery killed: context deadline exceeded"`、`out == nil`、耗时 `< 4*time.Second`。【判别力】不杀进程（M25）时要等满 `discoveryKillGrace`（5 s）才返回（原型实测）。
- **T5 `TestMdxRunDiscoveryWorkDirIsTemp`**：`cwd`：`err == nil`，`filepath.Clean(string(out)) == filepath.Clean(os.TempDir())`。
- **T6 `TestMdxRunDiscoveryStartErrors`**：`RunDiscovery(ctx, provider.DiscoveryCommand{})` 的 `err.Error() == "discovery command path is empty"`；`Path` 是 `t.TempDir()` 下不存在的 `mdx-no-such.exe` 时 `err != nil`（文本随系统，不比）。

### 2.3 `internal/config/mdx_append_test.go`（`package config`）

- **C1 `TestMdxAgentBlockTOML`**：三例，返回值与下面的 Go 字符串字面量相等（原型实测）：
  - (a) `AgentBlockTOML("opus-5.5-1m-high", AgentConfig{DisplayName: "Opus 5.5 1m", Provider: "claude", Model: "claude-opus-5-5[1m]", Effort: "high", Permissions: PermissionRead, Mode: ModeDiscussion}, "council /add 追加：claude 识别出的 claude-opus-5-5[1m]（订阅）")` ==
    `"# council /add 追加：claude 识别出的 claude-opus-5-5[1m]（订阅）\n[agents.\"opus-5.5-1m-high\"]\ndisplay_name = \"Opus 5.5 1m\"\nprovider = \"claude\"\nmodel = \"claude-opus-5-5[1m]\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"`
  - (b) `AgentBlockTOML("g2", AgentConfig{DisplayName: "Grok 4.6", Provider: "grok", Model: "grok-4.6", Permissions: PermissionRead, Mode: ModeDiscussion}, "")` ==
    `"[agents.g2]\ndisplay_name = \"Grok 4.6\"\nprovider = \"grok\"\nmodel = \"grok-4.6\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"`
  - (c) `AgentBlockTOML("x_1", AgentConfig{DisplayName: "a\"b\\c\td", Provider: "agy", Model: "M\x7f", Role: "r\n2", Permissions: PermissionRead, Mode: ModeDiscussion}, "line1\nline2\x7f")` ==
    `"# line1 line2 \n[agents.x_1]\ndisplay_name = \"a\\\"b\\\\c\\u0009d\"\nprovider = \"agy\"\nmodel = \"M\\u007F\"\nrole = \"r\\u000A2\"\npermissions = \"read\"\nmode = \"discussion\"\n"`
  - 另：(a)(b)(c) 各自前面拼一行 `data_dir = ''\n[providers.claude]\ncommand = 'c'\n[providers.grok]\ncommand = 'g'\n[providers.agy]\ncommand = 'a'\n` 写成文件，`Load` 成功且 `Agents[<id>]` 与传入的 `AgentConfig` 相等（(c) 的 `Model` 读回 `"M\x7f"`、`Role` 读回 `"r\n2"`）。【判别力】总加引号（M29）时 (b) 红；从不加引号（M30）时 (a) 红且读回失败。
- **C2 `TestMdxAppendBlockSeparators`**：`dir := t.TempDir()`，四个文件原文依次 `""`、`"a\n"`、`"a"`、`"a\r\n"`，各调 `AppendBlock(p, "[agents.z]\n")`：`err == nil`；返回的 `orig` 等于原文；文件变成依次 `"[agents.z]\n"`、`"a\n\n[agents.z]\n"`、`"a\n\n[agents.z]\n"`、`"a\r\n\n[agents.z]\n"`；返回的备份路径的文件名匹配 `^c\d\.toml\.bak-add-\d{8}-\d{6}$`（文件名取 `c0.toml` … `c3.toml`），备份内容等于原文。四次之后 `dir` 里只有这 8 个文件（没有 `*.tmp-add-*`）。
- **C3 `TestMdxWriteBackupNeverOverwrites`**：白盒，同一个 `now := time.Date(2026, 9, 27, 15, 30, 12, 0, time.Local)` 连调三次 `writeBackup(p, []byte("v0"|"v1"|"v2"), now)`：返回的文件名依次是 `c9.toml.bak-add-20260927-153012`、`…-153012-2`、`…-153012-3`，内容依次 `v0`、`v1`、`v2`。【判别力】去掉 `O_EXCL`（M35）时三次都是同一个名字、只剩 `v2`（原型实测）。
- **C6 `TestMdxWriteBackupAllTaken`**（v1.1 新增，F11）：白盒。`p := filepath.Join(dir, "c9.toml")`，`now` 同 C3，`base := p + ".bak-add-20260927-153012"`。先建好 `base` 与 `base-2` … `base-98` 共 98 个文件，内容都是 `old`。`writeBackup(p, []byte("v99"), now)` 返回 `base + "-99"`、`err == nil`，文件内容 `v99`。再调一次 `writeBackup(p, []byte("v100"), now)`：返回 `""`，`err.Error() == "backup names " + base + " and " + base + "-2..99 all taken"`。目录里恰好 99 个文件，98 个旧文件内容仍是 `old`。【判别力】第 99 个名字是边界：循环写成 `i < maxBackupAttempts`（M98）时第一次就报错；报错文本回到 v1 只写 `-2..99` 的写法（M99）时第二次的文本不等（v1.1 原型实测，两条都只红 C6）。
- **C4 `TestMdxRestoreFile`**：文件原文 `changed`，`RestoreFile(p, []byte("orig bytes"))` 之后内容 `orig bytes`、`err == nil`、目录里没有 `*.tmp-add-*`。
- **C5 `TestMdxNewAgentIDProblem`**：`cfg := &Config{Agents: map[string]AgentConfig{"opus-5.0": {}, "grok-4.7": {}, "a_b": {}}}`，逐个断言返回值（原型实测，逐字）：

```
"sonnet-5-high" -> ""
"opus-5.5-1m-high" -> ""
"Bad" -> "id Bad 不合法（要匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$）；用 --id <新 id> 换一个"
"-x" -> "id -x 不合法（要匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$）；用 --id <新 id> 换一个"
"x." -> "id x. 不合法（要匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$）；用 --id <新 id> 换一个"
"user" -> "id user 是保留名；用 --id <新 id> 换一个"
"system" -> "id system 是保留名；用 --id <新 id> 换一个"
"nul" -> "id nul 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"con" -> "id con 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"aux" -> "id aux 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"prn" -> "id prn 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"com1" -> "id com1 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"com9" -> "id com9 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"lpt1" -> "id lpt1 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"lpt9" -> "id lpt9 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"com0" -> ""
"lpt0" -> ""
"com10" -> ""
"nul.1" -> "id nul.1 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"com1.x" -> "id com1.x 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"
"con-x" -> ""
"nulx" -> ""
"opus-5.0" -> "配置里已经有 agent opus-5.0；用 --id <新 id> 换一个"
"grok-4.7" -> "配置里已经有 agent grok-4.7；用 --id <新 id> 换一个"
"opus-5-0" -> "id opus-5-0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"
"opus_5.0" -> "id opus_5.0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"
"opus-5_0" -> "id opus-5_0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"
"opus-5.00" -> ""
"a-b" -> "id a-b 与已有的 agent a_b 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"
"a.b" -> "id a.b 与已有的 agent a_b 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"
"a_b" -> "配置里已经有 agent a_b；用 --id <新 id> 换一个"
```

### 2.4 `internal/repl/mdx_addmodel_test.go`（`package repl`；可拆一个 `mdx_addmodel_fixture_test.go`）

共用装置（冻结）：`t.Setenv("COUNCIL_FAKE_CLI", "1")`；`dir := t.TempDir()`；配置写到 `filepath.Join(dir, "config.toml")`（§2.0 的配置文本，R8、R11、R12、R13 `provider` 用变体）；`cfg, err := config.LoadAndValidate(cfgPath)`；`st, _ := store.Open(ctx, filepath.Join(dir, "data"))`，`t.Cleanup` 关库；`svc := conversation.NewService(st)`；`conv, _ := svc.Create(ctx, "mdx")`；`rn := turn.NewRunner(cfg, svc, filepath.Join(dir, "workdir"), turn.RulesBlocks{})`；`r := New(cfg, svc, rn, strings.NewReader(""), &out, RulesContext{ConfigPath: cfgPath})`；`r.discover = <§2.0 假执行器>`；`r.discoverTimeout = 200 * time.Millisecond`；先 `r.dispatch(ctx, "/use "+conv.ID)`。下文「输出」都指某条 `dispatch` 之前与之后 `out` 的增量，按 `"\n"` 切行（不含提示符）。`<conv>` 是 `conv.ID`，`<cfg>` 是 `cfgPath`，`<bak>` 是 `dir` 里当时唯一新出现的、名字匹配 `^config\.toml\.bak-add-\d{8}-\d{6}(-\d+)?$` 的文件的完整路径。

- **R10 `TestMdxNewWiresDiscovery`**：`r := New(nil, nil, nil, nil, nil, RulesContext{})`：`reflect.ValueOf(r.discover).Pointer() == reflect.ValueOf(turn.RunDiscovery).Pointer()`；`r.discoverTimeout == 20*time.Second`；`r.modelSnap == nil`。

- **R1 `TestMdxAddListsDiscovered`**：`dispatch("/add")`。输出的前 7 行是配置里的候选（既有格式，不在本刀断言范围，但必须以既有表头 `NO  AGENT           PROVIDER  DISPLAY_NAME` 开头、以既有提示行 `用 /add <编号|全名|前缀> 加入，多个用空格或逗号隔开，例如 /add 1,2` 结尾），紧接着的 27 行**整行逐字**等于：

```
正在识别模型: agy, claude, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                       MODEL                         EFFORTS                    SOURCE
+1   agy       gemini-3.8-flash-medium  Gemini 3.8 Flash (Medium)     -                          订阅
+2   agy       gemini-3.8-flash-low     Gemini 3.8 Flash (Low)        -                          订阅
+3   agy       gemini-3.7-flash-high    Gemini 3.7 Flash (High)       -                          订阅
+4   agy       gemini-3.7-flash-medium  Gemini 3.7 Flash (Medium)     -                          订阅
+5   agy       gemini-3.7-flash-low     Gemini 3.7 Flash (Low)        -                          订阅
+6   agy       gemini-3.6-flash-high    Gemini 3.6 Flash (High)       -                          订阅
+7   agy       gemini-3.6-flash-medium  Gemini 3.6 Flash (Medium)     -                          订阅
+8   agy       gemini-3.6-flash-low     Gemini 3.6 Flash (Low)        -                          订阅
+9   agy       gemini-3.1-pro-high      Gemini 3.1 Pro (High)         -                          订阅
+10  agy       gemini-3.1-pro-low       Gemini 3.1 Pro (Low)          -                          订阅
+11  agy       agy-sonnet-4.6-thinking  Claude Sonnet 4.6 (Thinking)  -                          走 agy 额度
+12  agy       agy-opus-4.6-thinking    Claude Opus 4.6 (Thinking)    -                          走 agy 额度
+13  agy       gpt-oss-120b-medium      GPT-OSS 120B (Medium)         -                          走 agy 额度
+14  claude    sonnet-5-high            claude-sonnet-5               low|medium|high|xhigh|max  订阅
+15  claude    sonnet-5-1m-high         claude-sonnet-5[1m]           low|medium|high|xhigh|max  订阅
+16  claude    haiku-4.5                claude-haiku-4-5-20251001     -                          订阅
+17  claude    mdx-lowmed-2.0-low       claude-mdx-lowmed-2-0         low|medium                 订阅
+18  claude    mdx-plain-1.2-high       claude-mdx-plain-1-2          high|max                   订阅
+19  grok      grok-4.6                 grok-4.6                      -                          本地配置
+20  grok      grok-4.5                 grok-4.5                      -                          本地配置
+21  grok      grok-4.3                 grok-4.3                      -                          本地配置
+22  grok      grok-composer-2.5-fast   grok-composer-2.5-fast        -                          本地配置
+23  grok      grok-build               grok-build                    -                          本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
```

  且这 27 行之后没有别的非空行。假执行器恰好被调 3 次，`Path` 依次 `mdx-agy.exe`、`<exe>`、`mdx-grok.exe`，每次的 `Args` / `Stdin` 与 `provider.DiscoveryCommandFor(<provider>, <path>)` 相等。`r.modelSnap != nil`。【判别力】表里没有 `claude-opus-5-5[1m]`（配置的 `opus[1m]` 经 `Alias` 藏起来，M47）、没有 `claude-fable-5-1`、没有 `Gemini 3.8 Flash (High)`、没有 `grok-4.7`；`+11 agy agy-sonnet-4.6-thinking` 这一行在（它的 Alias `claude-sonnet-4-6` 等于配置里 claude 成员 `sonnet-4.6` 的 model，不比 provider 的写法（M48）会把它藏起来）；编号从 +1 连续到 +23（拍快照时把已配置的也编号（M72）会从 +2 起）。对照是 R12。

- **R2 `TestMdxAddPlusWritesConfigAndJoins`**：记 `orig := 文件字节`、`cfg0 := r.cfg`。
  1. `dispatch("/add")`（同 R1）。
  2. `dispatch("/add +14")`：输出恰好两行 `已把 agent sonnet-5-high 追加进 <cfg>（原文件备份在 <bak>）`、`sonnet-5-high 已加入会话 <conv>`。文件字节 == `orig` + §3.5 的第一段（`sonnet-5-high` 那段，含开头的空行）；`<bak>` 的字节 == `orig`；此刻 `dir` 里恰好三项：`config.toml`、`data`、`<bak>` 的文件名。`r.cfg != cfg0`（指针换了），`reflect.DeepEqual(r.cfg, 重新 config.LoadAndValidate(cfgPath))`；`r.cfg.Agents["sonnet-5-high"] == config.AgentConfig{DisplayName: "Sonnet 5", Provider: "claude", Model: "claude-sonnet-5", Effort: "high", Role: "", Permissions: config.PermissionRead, Mode: config.ModeDiscussion}`；其余 5 个 agent 与 `cfg0.Agents` 逐个相等。`svc.Participants(ctx, conv.ID, true)` 恰好一行：`AgentID "sonnet-5-high"`、`DisplayName "Sonnet 5"`、`Provider "claude"`、`Active`。
  3. `dispatch("/add")`：输出里没有 `正在识别模型` 开头的行；配置候选之后紧接的 25 行整行逐字等于：

```
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                       MODEL                         EFFORTS                    SOURCE
+1   agy       gemini-3.8-flash-medium  Gemini 3.8 Flash (Medium)     -                          订阅
+2   agy       gemini-3.8-flash-low     Gemini 3.8 Flash (Low)        -                          订阅
+3   agy       gemini-3.7-flash-high    Gemini 3.7 Flash (High)       -                          订阅
+4   agy       gemini-3.7-flash-medium  Gemini 3.7 Flash (Medium)     -                          订阅
+5   agy       gemini-3.7-flash-low     Gemini 3.7 Flash (Low)        -                          订阅
+6   agy       gemini-3.6-flash-high    Gemini 3.6 Flash (High)       -                          订阅
+7   agy       gemini-3.6-flash-medium  Gemini 3.6 Flash (Medium)     -                          订阅
+8   agy       gemini-3.6-flash-low     Gemini 3.6 Flash (Low)        -                          订阅
+9   agy       gemini-3.1-pro-high      Gemini 3.1 Pro (High)         -                          订阅
+10  agy       gemini-3.1-pro-low       Gemini 3.1 Pro (Low)          -                          订阅
+11  agy       agy-sonnet-4.6-thinking  Claude Sonnet 4.6 (Thinking)  -                          走 agy 额度
+12  agy       agy-opus-4.6-thinking    Claude Opus 4.6 (Thinking)    -                          走 agy 额度
+13  agy       gpt-oss-120b-medium      GPT-OSS 120B (Medium)         -                          走 agy 额度
+15  claude    sonnet-5-1m-high         claude-sonnet-5[1m]           low|medium|high|xhigh|max  订阅
+16  claude    haiku-4.5                claude-haiku-4-5-20251001     -                          订阅
+17  claude    mdx-lowmed-2.0-low       claude-mdx-lowmed-2-0         low|medium                 订阅
+18  claude    mdx-plain-1.2-high       claude-mdx-plain-1-2          high|max                   订阅
+19  grok      grok-4.6                 grok-4.6                      -                          本地配置
+20  grok      grok-4.5                 grok-4.5                      -                          本地配置
+21  grok      grok-4.3                 grok-4.3                      -                          本地配置
+22  grok      grok-composer-2.5-fast   grok-composer-2.5-fast        -                          本地配置
+23  grok      grok-build               grok-build                    -                          本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
```

     假执行器的调用次数仍是 3。
  4. `dispatch("/agents")`：输出里有整行 `sonnet-5-high   claude    claude-sonnet-5          high    read         discussion`。
  5. `dispatch("hello")`，`dispatch("/ask sonnet-5-high")`：输出里有整行 `outcome spoke  agent sonnet-5-high`。
  【判别力】只换 `r.cfg` 不换 Runner（M55）时第 5 步是 `outcome refused  agent sonnet-5-high`；只换 Runner（M56）时第 3 步 `+14` 行还在、第 4 步没有这一行；不 Join（M57）时第 2 步库里没有成员、第 3 步的配置候选多出 `sonnet-5-high`；显示时重新编号（M49）时第 3 步的 `+15` 变 `+14`（原型实测）。

- **R3 `TestMdxAddEffortAndDuplicate`**：`dispatch("/add")` 之后依次：
  1. `/add +14` -> `已把 agent sonnet-5-high 追加进 <cfg>（原文件备份在 <bak1>）` / `sonnet-5-high 已加入会话 <conv>`
  2. `/add +14 --effort max` -> `已把 agent sonnet-5-max 追加进 …（…<bak2>）` / `sonnet-5-max 已加入会话 <conv>`
  3. `/add +14` -> 一行 `配置里已经有同一个模型、同一强度的 agent sonnet-5-high，直接 /add sonnet-5-high`
  4. `/add +17` -> 加入 `mdx-lowmed-2.0-low`（默认强度 `low`：该模型不支持 high）
  5. `/add +16` -> 加入 `haiku-4.5`（没有强度，块里没有 `effort` 行）
  6. `/add +19 --id g2` -> 加入 `g2`
  7. `/add +18 --effort max --id mdx-plain-max` -> 加入 `mdx-plain-max`

  文件字节 == `orig` + §3.5 的六段全文（Go 字面量逐字）；`dir` 里的备份文件恰好 6 个、两两不同名，第 k 个备份的字节等于第 k 次追加之前的文件字节。库里在场成员恰好 `g2`、`haiku-4.5`、`mdx-lowmed-2.0-low`、`mdx-plain-max`、`sonnet-5-high`、`sonnet-5-max`。【判别力】第 3 步与第 2 步只差 `--effort`：同脑子判据不看强度（M60）时第 2 步就被拒；去掉同脑子判据（M61）时第 3 步变成「配置里已经有 agent sonnet-5-high；用 --id <新 id> 换一个」（原型实测）。

- **R4 `TestMdxAddRefusals`**：`dispatch("/add +1")` **在** `/add` 之前 -> `还没有识别结果：先敲 /add 列出识别出的模型，再用 /add +编号 加入`；然后 `dispatch("/add")`；再逐条喂下表左列，每条的输出恰好是右列那一行：

```
/add +x
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +3 --id
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +3 --foo
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add --refresh x
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +3 --id a --id b
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +3 --effort a --effort b
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add +99
识别结果里没有 +99，不带参数的 /add 能列出现有编号

/add +0
识别结果里没有 +0，不带参数的 /add 能列出现有编号

/add +99999999999999999999
识别结果里没有 +99999999999999999999，不带参数的 /add 能列出现有编号

/add +16 --effort high
claude-haiku-4-5-20251001 没有可选的强度，不能带 --effort

/add +14 --effort ultra
claude-sonnet-5 支持的强度是 low|medium|high|xhigh|max，没有 ultra

/add +19 --effort high
grok 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort

/add +3 --effort high
agy 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort

/add +14 --id Bad
id Bad 不合法（要匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$）；用 --id <新 id> 换一个

/add +14 --id user
id user 是保留名；用 --id <新 id> 换一个

/add +14 --id nul
id nul 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个

/add +14 --id com1.x
id com1.x 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个

/add +14 --id opus-5.0
配置里已经有 agent opus-5.0；用 --id <新 id> 换一个

/add +14 --id opus-5_0
id opus-5_0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个

/add +14 --id opus_5.0
id opus_5.0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个

/add +
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别

/add 1,+2
配置里没有可加入的 +2，不带参数的 /add 能列出可加的
```

  全部喂完之后：文件字节 == `orig`；`dir` 里没有任何 `config.toml.bak-add-*`；库里没有成员；`r.cfg` 指针没变；假执行器调用次数 3。

- **R5 `TestMdxAddSnapshotStable`**：`dispatch("/add")`、`dispatch("/add")`：执行器调用次数 3、3；把假执行器对 `mdx-grok.exe` 的返回换成 `"Available models:\n  * grok-4.7 (default)\n  - grok-9.9\n"`；`dispatch("/add")`：调用次数仍 3，识别部分的行与 R1 的第 2-27 行逐字相同（没有 `正在识别模型` 行）；`dispatch("/add --refresh")`：调用次数 6，输出里有整行 `正在识别模型: agy, claude, grok` 与整行 `+19  grok      grok-9.9                 grok-9.9                      -                          本地配置`，没有含 `grok-4.6` 的行，`+1` 到 `+18` 那 18 行与 R1 相同。【判别力】每次都重新识别（M50）时第二次调用次数是 6；`--refresh` 不清快照（M51）时最后一步没有 `正在识别模型` 行（原型实测）。

- **R6 `TestMdxAddDiscoveryFailures`**，四个子测试（各自一套新装置）。先喂一次 `/add`，识别部分（配置候选之后）的行整行逐字等于下面四段（v1.1：每段末尾多一行 `--refresh` 提示，因为四段都有失败行）；再喂一次 `/add`，识别部分逐字等于同一段去掉第一行 `正在识别模型: …`，假执行器调用次数不变。第二次喂的是 v1.1 加的：它证明失败行与提示行随快照一起重印。
  - `each_kind`：agy 返回 `{"status":"SUCCESS","command":{"name":"models","data":{"models":[]}}}`、claude 走超时（§2.0 的超时假执行器）、grok 返回错误 `errors.New("mdx boom")`：

```
正在识别模型: agy, claude, grok
识别 agy 的模型失败: 一个模型都没列出来（没登录？）
识别 claude 的模型失败: 超时（200ms 内没有回来）
识别 grok 的模型失败: model discovery command failed: mdx boom
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh
```

  - `unparseable`：agy 返回 `not json`、claude 返回 `{"type":"result"}\n`、grok 返回 `Available models:\n`：

```
正在识别模型: agy, claude, grok
识别 agy 的模型失败: 输出认不出来（CLI 版本变了？）
识别 claude 的模型失败: 输出认不出来（CLI 版本变了？）
识别 grok 的模型失败: 一个模型都没列出来（没登录？）
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh
```

  - `all_down`：三家都返回错误，文本依次 `mdx agy down`、`mdx claude down`、`mdx grok down`：

```
正在识别模型: agy, claude, grok
识别 agy 的模型失败: model discovery command failed: mdx agy down
识别 claude 的模型失败: model discovery command failed: mdx claude down
识别 grok 的模型失败: model discovery command failed: mdx grok down
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh
```

  - `one_down`：只有 grok 返回错误 `mdx grok down`，识别部分整行逐字等于下面这段（agy 与 claude 的行照常编号、照常显示，表后照样打失败行；原型实测）：

```
正在识别模型: agy, claude, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                       MODEL                         EFFORTS                    SOURCE
+1   agy       gemini-3.8-flash-medium  Gemini 3.8 Flash (Medium)     -                          订阅
+2   agy       gemini-3.8-flash-low     Gemini 3.8 Flash (Low)        -                          订阅
+3   agy       gemini-3.7-flash-high    Gemini 3.7 Flash (High)       -                          订阅
+4   agy       gemini-3.7-flash-medium  Gemini 3.7 Flash (Medium)     -                          订阅
+5   agy       gemini-3.7-flash-low     Gemini 3.7 Flash (Low)        -                          订阅
+6   agy       gemini-3.6-flash-high    Gemini 3.6 Flash (High)       -                          订阅
+7   agy       gemini-3.6-flash-medium  Gemini 3.6 Flash (Medium)     -                          订阅
+8   agy       gemini-3.6-flash-low     Gemini 3.6 Flash (Low)        -                          订阅
+9   agy       gemini-3.1-pro-high      Gemini 3.1 Pro (High)         -                          订阅
+10  agy       gemini-3.1-pro-low       Gemini 3.1 Pro (Low)          -                          订阅
+11  agy       agy-sonnet-4.6-thinking  Claude Sonnet 4.6 (Thinking)  -                          走 agy 额度
+12  agy       agy-opus-4.6-thinking    Claude Opus 4.6 (Thinking)    -                          走 agy 额度
+13  agy       gpt-oss-120b-medium      GPT-OSS 120B (Medium)         -                          走 agy 额度
+14  claude    sonnet-5-high            claude-sonnet-5               low|medium|high|xhigh|max  订阅
+15  claude    sonnet-5-1m-high         claude-sonnet-5[1m]           low|medium|high|xhigh|max  订阅
+16  claude    haiku-4.5                claude-haiku-4-5-20251001     -                          订阅
+17  claude    mdx-lowmed-2.0-low       claude-mdx-lowmed-2-0         low|medium                 订阅
+18  claude    mdx-plain-1.2-high       claude-mdx-plain-1-2          high|max                   订阅
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
识别 grok 的模型失败: model discovery command failed: mdx grok down
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh
```

  【判别力】`one_down` 是「一家失败不拦别家」与「失败行在表后照样打」（M66）的对照；`each_kind` 的 claude 行防「超时不带进 ctx」（M64，假执行器会等 3 s 后报 `mdx: ctx never cancelled`）与超时文案映射（M65）；`each_kind` 的 grok 行防「三家共用一个 deadline」（M77，W1 的 S14）：共用时 claude 超时之后 grok 拿到的 ctx 已经过期，假执行器第 3 步返回 `context.DeadlineExceeded`，grok 那一行变成 `识别 grok 的模型失败: 超时（200ms 内没有回来）`（v1.1 原型实测，只红 R6 `each_kind`）。四段末尾的 `--refresh` 提示行防「失败被缓存却不提示怎么重来」（M86，四段都红）；它的对照是 R1（没有失败行，「这 27 行之后没有别的非空行」）。第二次 `/add` 让「每次都重新识别」（M50）与「有快照也打 `正在识别模型`」（M67）在 R6 里也红（v1.1 原型实测）。
  注释要点（测试腿写进用例注释）：`each_kind` 里超时的那一家（claude）**不能排在最后**，它后面必须还有一家（grok）。provider 按字典序识别，改夹具时若让超时的那一家落到最后，M77 就没有可观测的后果，这条对照会悄悄失效。

- **R7 `TestMdxAddGuard`**（v1.1 改成四个子测试，W1b 修订再加 `teams`、`providers` 两个，共六个，各自一套新装置，互不连带）：每个子测试先做同一段前缀：`dispatch("/add")`；往文件末尾追加 `"\n# edited\n"`（只加注释）；`dispatch("/add +14")` -> 正常加入（两行同 R2 第 2 步）；记 `base := 文件字节`、`cfg1 := r.cfg`、`dataLine := "data_dir = '" + filepath.Join(dir, "data") + "'\n"`（前提守卫：`dataLine` 在 `base` 里恰好一处；改完之后的文本不等于 `base`）。然后把文件写成下表的「改法」，记为 `edited`，`dispatch("/add +15")`，输出恰好一行（下表右列）；之后文件字节 == `edited`、备份文件恰好 1 个（前缀那一次的）、`r.cfg == cfg1`（指针没换）、库里在场成员只有 `sonnet-5-high`。

  | 子测试 | 改法（Go 表达式） | 输出（恰好这一行） |
  |---|---|---|
  | `agent` | `base + "\n[agents.zzz]\nprovider = 'claude'\nmodel = 'm'\n"` | `配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add +15` |
  | `store_thought` | `strings.Replace(base, dataLine, dataLine+"store_thought = false\n", 1)` | `配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add +15` |
  | `data_dir` | `strings.Replace(base, dataLine, "data_dir = '"+filepath.Join(dir, "data2")+"'\n", 1)` | `配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入` |
  | `rules_file` | `strings.Replace(base, dataLine, dataLine+"rules_file = 'mdx-rules.md'\n", 1)` | `配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入` |
  | `teams` | `strings.Replace(base, "members = ['opus-5.0', 'grok-4.7']", "members = ['opus-5.0']", 1)` | `配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add +15` |
  | `providers` | `strings.Replace(base, "command = 'mdx-grok.exe'", "command = 'mdx-grok2.exe'", 1)` | `配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add +15` |

  （顶层键必须插在第一个 `[表]` 之前，所以插在 `data_dir` 那一行后面；装置的 `r.cfg.StoreThought` 是 `Load` 的默认值 true。`teams`、`providers` 两行是 W1b 修订加的，前提守卫：被替换的那一串在 `base` 里恰好一处。）
  【判别力】前缀里只改注释那一步是「按加载结果比、不按字节或 mtime 比」的对照。六个子测试各只动 Config 的一个字段：`agent` 只动 Agents；`store_thought` 只动一个 reload 会应用的顶层键；`teams` 只动 Teams（从 `all` 里去掉 grok-4.7）；`providers` 只动 Providers（grok 的 command 换名）；`data_dir`、`rules_file` 只动那两个只能重启生效的键。`store_thought`、`teams`、`providers` 三种改动 `/config reload --apply` 都会应用（W1b 原型实测：reload 的输出里有整行 `已应用新配置`，`providers` 那次先打一行 command 改动的告警），所以「先 reload」对它们是真话。下面各条变异都只红 R7 里写出的子测试（v1.1 原型实测 M52、M78-M82；W1b 原型重跑了这几条并实测 M103、M104、M109，跑的是 R1、R7、R14、R16，R7 之外都没红）：
  - 去掉守卫（M52）：`agent`、`store_thought`、`teams`、`providers` 红，`sonnet-5-1m-high` 被加进来了。
  - 守卫只比 Agents（M78）：`store_thought`、`teams`、`providers` 红。只比 Agents 与 Providers（M79）：`store_thought`、`teams` 红。
  - 整份比之前先把 Teams 抹平（M103）：只有 `teams` 红；先把 Providers 抹平（M104）：只有 `providers` 红。v1.1 的四个子测试下这两条活过全部验收（W1b 的发现 1）。只比 Agents 与 StoreThought（M109）：`teams`、`providers` 红。
  - 去掉 data_dir / rules_file 那一支（M80），或两支顺序对调（M81）：`data_dir`、`rules_file` 红，输出变成「先 /config reload --apply」那一行。那是死胡同，reload 会拒绝这两个键。
  - 那一支只判 data_dir（M82）：只有 `rules_file` 红。

- **R8 `TestMdxAddRestoresOnInvalidConfig`**：配置变体：§2.0 的文本去掉 `[agents."grok-4.7"]` 整段（连同它后面的空行）、`members = ['opus-5.0', 'grok-4.7']` 改成 `members = ['opus-5.0']`。`dispatch("/add")`（此时 grok 的 6 行里 `grok-4.7` 也显示，是 `+19`，`grok-4.6` 是 `+20`）；把文件里的 `"[providers.grok]\ncommand = 'mdx-grok.exe'\n\n"` 删掉，记为 `noGrok`；`dispatch("/config reload --apply")`（前提守卫：输出里有整行 `已应用新配置`）；记 `cfg1 := r.cfg`；`dispatch("/add +20")` -> 恰好一行 `写入后的配置没通过加载或校验，已恢复原文件（备份在 <bak>）: agents.grok-4.6.provider: agent "grok-4.6" references undefined provider "grok"`；文件字节 == `noGrok`；`<bak>` 的字节 == `noGrok`；`r.cfg == cfg1`（指针没换）；库里没有成员。【判别力】不恢复（M54）时文件多出 grok-4.6 那一段（原型实测）。这一步同时覆盖 P6 的 `grok-4.7` 行在 repl 屏幕上可见。

- **R9 `TestMdxAddNoRunnerNoPath`**：(a) `New(cfg, svc, nil, …, RulesContext{ConfigPath: cfgPath})`，`/use`、`/add`、`/add +14` -> 最后一条恰好一行 `provider 调用未就绪，本命令不可用`；(b) Runner 正常、`RulesContext{}`，同样三条 -> 最后一条恰好一行 `不知道配置文件在哪，没法写入`。两种情况文件字节不变、没有备份、库里没有成员。【判别力】去掉 Runner 判据（M62）时 (a) 在 `r.rn.SetConfig` 上 panic（原型实测：`dispatch` 直接 panic，用例要 `recover` 住再判红，或用 `t.Run` 让 panic 只挂这一条）；去掉路径判据（M63）时 (b) 变成 `读配置文件失败: read config file : open : …`。

- **R11 `TestMdxAddSkipsUndiscoverableProviders`**：(a) 配置只有两个 provider：`[providers.claude] command = '<exe>'` 与 `[providers.pv_mdx] command = 'mdx-pv.exe'`，没有 agent：`/add` 的输出整行逐字：

```
配置里的 agent 都已经在当前会话里了
正在识别模型: claude
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO  PROVIDER  ID                  MODEL                      EFFORTS                    SOURCE
+1  claude    opus-5.5-1m-high    claude-opus-5-5[1m]        low|medium|high|xhigh|max  订阅
+2  claude    fable-5.1-high      claude-fable-5-1           low|medium|high|xhigh|max  订阅
+3  claude    sonnet-5-high       claude-sonnet-5            low|medium|high|xhigh|max  订阅
+4  claude    sonnet-5-1m-high    claude-sonnet-5[1m]        low|medium|high|xhigh|max  订阅
+5  claude    haiku-4.5           claude-haiku-4-5-20251001  -                          订阅
+6  claude    mdx-lowmed-2.0-low  claude-mdx-lowmed-2-0      low|medium                 订阅
+7  claude    mdx-plain-1.2-high  claude-mdx-plain-1-2       high|max                   订阅
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
```

  执行器恰好被调 1 次（`Path == <exe>`）；(b) 配置只有 `pv_mdx`：`/add` 的输出恰好一行 `配置里的 agent 都已经在当前会话里了`（没有 `正在识别模型` 行），执行器 0 次；接着 `/add +1` -> `配置里没有能识别模型的 provider，/add +编号 用不了`（v1.1，F12：v1 在这里叫用户「先敲 /add」，而这份配置下 `/add` 永远不会识别，那是一条走不通的路）。【判别力】不跳过没有适配器的 provider（M46）时 (a) 的首行是 `正在识别模型: claude, pv_mdx`。(b) 的 `/add +1` 与 R4 第一步（有能识别的 provider、只是还没识别）是一对对照：去掉新的那一支（M100）时 (b) 回到「还没有识别结果」那一行；那一支无条件成立（M101）时 R4 第一步变成「没有能识别模型的 provider」（v1.1 原型实测，各只红一处）。

  (c)（v1.1 新增，F6）配置只有 grok 一家，grok 夹具的 6 个模型都已配成 agent。配置文件整段冻结（`<dataDir>` 同 §2.0，文件以 LF 结尾）：

```
data_dir = '<dataDir>'

[providers.grok]
command = 'mdx-grok.exe'

[agents."grok-4.7"]
provider = 'grok'
model = 'grok-4.7'

[agents."grok-4.6"]
provider = 'grok'
model = 'grok-4.6'

[agents."grok-4.5"]
provider = 'grok'
model = 'grok-4.5'

[agents."grok-4.3"]
provider = 'grok'
model = 'grok-4.3'

[agents."grok-composer-2.5-fast"]
provider = 'grok'
model = 'grok-composer-2.5-fast'

[agents.grok-build]
provider = 'grok'
model = 'grok-build'
```

  `/add` 的识别部分（配置候选之后）整行逐字：

```
正在识别模型: grok
识别出的模型都已经在配置里了
```

  执行器恰好被调 1 次（`Path == "mdx-grok.exe"`）。【判别力】(c) 与 R6 `all_down` 是一对对照：两者都没有可见行；(c) 有一家识别成功（`okCount > 0`），所以打「都已经在配置里了」；`all_down` 三家全失败（`okCount == 0`），不打这一行。去掉这一支（M90，W1 的 S1）时 (c) 只剩第一行（v1.1 原型实测，只红 R11 (c)）。

- **R12 `TestMdxAddAliasControl`**：配置变体：§2.0 文本里 opus-5.0 的 `model        = 'opus[1m]'` 换成 `model        = 'opus'`（Griffin 真实写法）。`/add` 的识别部分整行逐字：

```
正在识别模型: agy, claude, grok
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO   PROVIDER  ID                       MODEL                         EFFORTS                    SOURCE
+1   agy       gemini-3.8-flash-medium  Gemini 3.8 Flash (Medium)     -                          订阅
+2   agy       gemini-3.8-flash-low     Gemini 3.8 Flash (Low)        -                          订阅
+3   agy       gemini-3.7-flash-high    Gemini 3.7 Flash (High)       -                          订阅
+4   agy       gemini-3.7-flash-medium  Gemini 3.7 Flash (Medium)     -                          订阅
+5   agy       gemini-3.7-flash-low     Gemini 3.7 Flash (Low)        -                          订阅
+6   agy       gemini-3.6-flash-high    Gemini 3.6 Flash (High)       -                          订阅
+7   agy       gemini-3.6-flash-medium  Gemini 3.6 Flash (Medium)     -                          订阅
+8   agy       gemini-3.6-flash-low     Gemini 3.6 Flash (Low)        -                          订阅
+9   agy       gemini-3.1-pro-high      Gemini 3.1 Pro (High)         -                          订阅
+10  agy       gemini-3.1-pro-low       Gemini 3.1 Pro (Low)          -                          订阅
+11  agy       agy-sonnet-4.6-thinking  Claude Sonnet 4.6 (Thinking)  -                          走 agy 额度
+12  agy       agy-opus-4.6-thinking    Claude Opus 4.6 (Thinking)    -                          走 agy 额度
+13  agy       gpt-oss-120b-medium      GPT-OSS 120B (Medium)         -                          走 agy 额度
+14  claude    opus-5.5-1m-high         claude-opus-5-5[1m]           low|medium|high|xhigh|max  订阅
+15  claude    sonnet-5-high            claude-sonnet-5               low|medium|high|xhigh|max  订阅
+16  claude    sonnet-5-1m-high         claude-sonnet-5[1m]           low|medium|high|xhigh|max  订阅
+17  claude    haiku-4.5                claude-haiku-4-5-20251001     -                          订阅
+18  claude    mdx-lowmed-2.0-low       claude-mdx-lowmed-2-0         low|medium                 订阅
+19  claude    mdx-plain-1.2-high       claude-mdx-plain-1-2          high|max                   订阅
+20  grok      grok-4.6                 grok-4.6                      -                          本地配置
+21  grok      grok-4.5                 grok-4.5                      -                          本地配置
+22  grok      grok-4.3                 grok-4.3                      -                          本地配置
+23  grok      grok-composer-2.5-fast   grok-composer-2.5-fast        -                          本地配置
+24  grok      grok-build               grok-build                    -                          本地配置
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
```

  【判别力】与 R1 只差配置里 opus 的 model 一处：`opus` 既不等于 `value`（`opus[1m]`）也不等于 `resolvedModel`，于是 `+14 claude opus-5.5-1m-high` 出现、其后的编号整体后移一位。

- **R13 `TestMdxAddSnapshotAcrossReload`**（v1.1 新增，F5）：三个子测试，各自一套新装置，覆盖「快照拍完之后配置被 `/config reload --apply` 换掉」的三条分支。每个子测试里的 reload 都先断言前提：输出里有整行 `已应用新配置`，没有就 `t.Fatalf`。
  - `alias`（sameBrainAgent 的别名分支）：`dispatch("/add")`；往文件末尾追加这一段（Go 字符串字面量，逐字冻结）`"\n[agents.snt]\nprovider = 'claude'\nmodel = 'sonnet'\neffort = 'high'\n"`；reload；记 `after := 文件字节`、`cfg1 := r.cfg`；`dispatch("/add +14")` -> 恰好一行 `配置里已经有同一个模型、同一强度的 agent snt，直接 /add snt`；之后文件字节 == `after`、没有任何备份、`r.cfg == cfg1`、库里没有成员。
  - `deleted`（已删成员的行不重现）：`dispatch("/add")`；把文件里 `[agents."fable-5.1"]` 整段连同它后面的空行删掉（前提守卫：那一段在文件里恰好一处）；reload；再 `dispatch("/add")`，识别部分整块逐字等于 R1 代码块的第 2-27 行（26 行，没有 `正在识别模型` 行）；假执行器调用次数仍是 3。
  - `provider`（sameBrainAgent 的 provider 判据）：配置变体是 §2.0 文本里 `sonnet-4.6` 那段去掉强度，即把 `"model        = 'claude-sonnet-4-6'\neffort       = 'high'\n"` 换成 `"model        = 'claude-sonnet-4-6'\n"`（前提守卫：恰好一处）。`dispatch("/add")`；`dispatch("/add +11")` -> 两行 `已把 agent agy-sonnet-4.6-thinking 追加进 <cfg>（原文件备份在 <bak>）`、`agy-sonnet-4.6-thinking 已加入会话 <conv>`。
  【判别力】v1.1 原型实测，三条变异各只红对应的子测试：
  - `alias`：`[agents.snt]` 必须带 `provider = 'claude'`，否则 Validate 拒绝、reload 不生效，前提断言先拦下。sameBrainAgent 去掉 Alias 判据（M87，W1 的 S2）时，`/add +14` 加进了一个同脑子的 `sonnet-5-high`。
  - `deleted`：整块比对，不断言「没有含 claude-fable-5-1 的行」：reload 若悄悄失败，那种缺席断言在变异上也会过。显示时不看 `row.no > 0`（M88，W1 的 S46）时，表里多出一行 `+0   claude    fable-5.1-high           claude-fable-5-1              low|medium|high|xhigh|max  订阅`。
  - `provider`：agy 的 `+11`（Alias `claude-sonnet-4-6`、强度空）与 claude 成员 `sonnet-4.6`（model `claude-sonnet-4-6`，这个变体里强度也空）只差 provider。去掉 `a.Provider != m.Provider`（M89）时 `+11` 被拒成 `配置里已经有同一个模型、同一强度的 agent sonnet-4.6，直接 /add sonnet-4.6`。R1 的配置里 `sonnet-4.6` 带 `effort = 'high'`，强度判据先把它挡掉，所以 v1 的用例都看不见这一条。

- **R14 `TestMdxAddCancelNotCached`**（v1.1 新增，F3；W1b 修订拆成 `first`、`late` 两个子测试，各自一套新装置）：`ctx1, cancel1 := context.WithCancel(context.Background())`；给假执行器装钩子（钩子在记录调用之后、判 ctx 之前，见 §2.0）：`first` 在第 1 次调用、且 `Path == "mdx-agy.exe"` 时调 `cancel1()`；`late` 在第 3 次调用、且 `Path == "mdx-grok.exe"` 时调 `cancel1()`（取消落在最后一家）。两个子测试的 `r.dispatch(ctx1, "/add")` 的识别部分（配置候选之后）都整行逐字等于：

```
正在识别模型: agy, claude, grok
识别被取消；下次 /add 会重新识别
```

  假执行器调用次数：`first` 是 1（取消之后循环开头就 break，claude、grok 不再起）；`late` 是 3（三家都起过，grok 那一次拿到的是已取消的 ctx，按失败记进快照）。两个子测试都 `r.modelSnap == nil`。然后用新的 `context.Background()` 再 `dispatch("/add")`：识别部分逐字等于 R1 代码块的 27 行；调用次数 `first` 是 4、`late` 是 6；`r.modelSnap != nil`（W1b 原型实测两个子测试，`-count=5` 稳定）。
  【判别力】`first` 走循环开头的 break；`late` 走「拍完之后判父 ctx」那一处，而且这时快照里已经有 agy、claude 的成功行（`okCount` 是 2）：
  - 取消之后照样缓存（M83，`printDiscoveredModels` 的父 ctx 判据改成 false）：两个子测试都红。`first` 的第一次输出变成 `识别 agy 的模型失败: model discovery command failed: context canceled` 加 `--refresh` 提示行，第二次 `/add` 不再识别，调用次数停在 1（v1.1 原型实测）。
  - 去掉循环开头的 break（M84）：只有 `first` 红，输出相同，但调用次数是 3，取消之后 claude、grok 仍被起了一次。`late` 杀不到它：grok 是最后一家，后面没有要跳过的。
  - 只在「一行都没拿到」时才算取消（M105 判 `snap.okCount == 0`，M106 判 `len(snap.rows) == 0`），或只在「循环提前结束」时才算取消（M107 判 `len(snap.failures)+snap.okCount < len(pids)`）：只有 `late` 红，部分快照连同一条 `context canceled` 的失败行被存下来，第二次 `/add` 不再识别（W1b 原型实测）。
  - 注释要点（测试腿写进用例注释）：`late` 的取消必须落在**最后一家**。挪到中间一家（claude）时，循环会在 grok 之前 break，按推演 M107 就没有可观测的后果了。这与 R6 `each_kind`「超时的那一家不能排在最后」是同一类坑，方向相反。
  - 这一条只管「取消」。缓存「没登录？」、瞬时超时这类失败是另一回事，由 R6 四段末尾的 `--refresh` 提示行管（M86）。

- **R15 `TestMdxAddWriteFailureShowsBackup`**（v1.1 新增，F13；只在 Windows 上跑，别的系统 `t.Skip`）：`dispatch("/add")`；`os.Chmod(cfgPath, 0o444)`（`t.Cleanup` 里改回 0o644）；`dispatch("/add +14")`；随即改回 0o644。Windows 上 `os.Rename` 覆盖只读的目标会失败（Win32 `MoveFileEx` 的语义；Go 1.27.0 本机实测），而这时备份已经建好。将来的 Go 若改用忽略只读属性的改名方式，这条用例的前提就不成立，要先看 `os.Rename` 还失败不失败。输出恰好一行，整行匹配下面这个锚定的正则。`QuoteMeta` 的部分逐字；只有 `CreateTemp` 的随机后缀写成 `\d+`，它每次都变，没法逐字冻结：

```
"^" + regexp.QuoteMeta("写配置文件失败（原文件备份在 "+<bak>+"）: 写入: rename "+filepath.Join(dir, "config.toml.tmp-add-")) + `\d+` + regexp.QuoteMeta(" "+<cfg>+": Access is denied.") + "$"
```

  原型实跑的一行（临时目录名缩写成 `…`）：`写配置文件失败（原文件备份在 C:\…\001\config.toml.bak-add-20260928-015749）: 写入: rename C:\…\001\config.toml.tmp-add-965830712 C:\…\001\config.toml: Access is denied.`。系统错误文本 `Access is denied.`：Go 在 Windows 上先按 en-US 向系统取，取不到才回落到系统语言；本机 Windows 11 中文版、Go 1.27.0 实测得英文。哪天回来的是中文，就是这条假设变了，不是被测代码错了。之后：文件字节 == 原文；`<bak>` 的字节 == 原文；`dir` 里恰好三项 `config.toml`、`data`、`<bak>` 的文件名（临时文件已被删）；`r.cfg` 指针没变；库里没有成员。
  【判别力】整行只有一处 `\d+` 通配，其余逐字；正对照是 R2 第 2 步（写入成功，那一行同样带备份路径）。备份路径不上屏（M102，`if backup != "" {` 改成 `if false {`）时，这一行变成 `写配置文件失败: 写入: rename …`（v1.1 原型实测，只红 R15；`-count=5` 稳定）。R16 是它的另一半：`backup` 为空的那一支。

- **R16 `TestMdxAddBackupStageFailure`**（W1b 修订新增）：`dispatch("/add")`；记 `orig := 文件字节`、`cfg0 := r.cfg`、`now := time.Now()`。对 `s` 从 -2 到 8 的每一秒，令 `base := cfgPath + ".bak-add-" + now.Add(time.Duration(s)*time.Second).Format("20060102-150405")`，建好 `base` 与 `base + "-2"` … `base + "-99"`，内容都是 `old`（前提守卫：一共建了 1089 个互不相同的名字）。然后 `dispatch("/add +14")`：输出恰好一行，整行匹配下面这个锚定的正则，两个捕获组相等，且 `<cfg>.bak-add-<捕获的时间戳>` 是预建的名字之一：

```
"^" + regexp.QuoteMeta("写配置文件失败: 备份: backup names "+<cfg>+".bak-add-") + `(\d{8}-\d{6})` + regexp.QuoteMeta(" and "+<cfg>+".bak-add-") + `(\d{8}-\d{6})` + regexp.QuoteMeta("-2..99 all taken") + "$"
```

  原型实跑的一行（临时目录名缩写成 `…`）：`写配置文件失败: 备份: backup names C:\…\001\config.toml.bak-add-20260928-025251 and C:\…\001\config.toml.bak-add-20260928-025251-2..99 all taken`。之后：文件字节 == `orig`；`r.cfg == cfg0`（指针没换）；库里没有成员；`dir` 里除了预建的 1089 个文件，恰好还有 `config.toml`、`data` 两项（没有新备份，没有临时文件）。
  【判别力】整行只通配时间戳，且两处必须相等。它与 R15 是一对：R15 走 `backup` 非空那一支，R16 走 `backup` 为空那一支。`if backup != "" {` 改成 `if true {`（M108）时这一行变成 `写配置文件失败（原文件备份在 ）: 备份: backup names …`，R16 红（W1b 原型实测；本轮只跑了 R1、R7、R14、R16 四条，其中只有 R16 红）；改成 `if false {`（M102）时 R16 不变（W1b 原型实测），红的是 R15（v1.1 实测）。R16 本身 `-count=5` 稳定。它顺带把 §3.3 那条英文文本经 `写配置文件失败: 备份: …` 上屏这件事钉在了 repl 屏幕上（v1.1 只有 C6 在 config 包里钉着文本）。
  前提（写进用例注释）：`writeBackup` 的名字只到秒，所以要预建一个时间窗。W1b 原型实测，建 1089 个文件加 `/add +14` 共约 0.5 s，`/add +14` 本身约 20 ms。机器慢到 `/add +14` 在 `now` 之后 8 秒还没走到 `writeBackup`，它会拿到一个空闲的名字、写入成功，这条用例红在「输出恰好一行」上：那是前提坏了，不是被测代码错了。窗口往前多留 2 秒，是防墙上时钟在 `now` 之后被往回拨。

---

## 3. 冻结的字面量

### 3.1 能力位、哨兵与类型

```go
ModelDiscoveryUnknown          ModelDiscovery = iota // 0
ModelDiscoverySubcommand                              // 1
ModelDiscoveryControlHandshake                        // 2
ModelDiscoveryUnsupported                             // 3

ModelSourceUnknown ModelSource = iota // 0，Label "-"
ModelSourceSubscription               // 1，Label "订阅"
ModelSourceLocalConfig                // 2，Label "本地配置"
ModelSourceAgyQuota                   // 3，Label "走 agy 额度"

ErrDiscoveryTimeout     = errors.New("model discovery timed out")
ErrDiscoveryFailed      = errors.New("model discovery command failed")
ErrDiscoveryUnparseable = errors.New("model discovery output is not a known shape")
ErrDiscoveryEmpty       = errors.New("model discovery listed no models")
ErrDiscoveryUnsupported = errors.New("provider has no model discovery")
```

`discoveryTimeout = 20 * time.Second`（repl）；`discoveryKillGrace = 5 * time.Second`、`maxDiscoveryStderrBytes = 64 << 10`（turn）；`maxBackupAttempts = 99`（config）。

### 3.2 屏幕上的新字面量（逐字节；`%s` / `%d` / `%v` 处按 §1.1 的代码填）

```
正在识别模型: %s
识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:
NO	PROVIDER	ID	MODEL	EFFORTS	SOURCE
+%d	%s	%s	%s	%s	%s
用 /add +编号 加入；--id <新 id> 改名；claude 可加 --effort <强度> 换档，EFFORTS 列是它支持的档
识别出的模型都已经在配置里了
识别 %s 的模型失败: %s
失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh
识别被取消；下次 /add 会重新识别
超时（%v 内没有回来）
一个模型都没列出来（没登录？）
输出认不出来（CLI 版本变了？）
用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别
配置里没有能识别模型的 provider，/add +编号 用不了
还没有识别结果：先敲 /add 列出识别出的模型，再用 /add +编号 加入
识别结果里没有 %s，不带参数的 /add 能列出现有编号
%s 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort
%s 没有可选的强度，不能带 --effort
%s 支持的强度是 %s，没有 %s
配置里已经有同一个模型、同一强度的 agent %s，直接 /add %s
不知道配置文件在哪，没法写入
配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入
配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add %s
写入后的配置没通过加载或校验，恢复原文件也失败了（备份在 %s）: %v；%v
写入后的配置没通过加载或校验，已恢复原文件（备份在 %s）: %v
写配置文件失败（原文件备份在 %s）: %v
已把 agent %s 追加进 %s（原文件备份在 %s）
```

（表头与行是 tabwriter 的输入，`\t` 是分隔；上面写成真 tab。全角标点：`，` U+FF0C、`；` U+FF1B、`：` U+FF1A、`（` `）` U+FF08 / U+FF09、`？` U+FF1F、`、` U+3001。`EFFORTS` 列的多档用 ASCII `|` 连，空为 `-`；`支持的强度是 %s` 的 `%s` 同样用 `|` 连。）另沿用既有字面量：`%s 已加入会话 %s`、`加入失败`、`provider 调用未就绪，本命令不可用`（`msgNoRunner`）、`reportErr` 的 `%s: %v` 体例，前缀新增两个：`读配置文件失败`、`写配置文件失败`。

### 3.3 `config.NewAgentIDProblem` 的五句（不含换行；`%s` 依次是 id、正则 / 已有 id）

```
id %s 不合法（要匹配 %s）；用 --id <新 id> 换一个
id %s 是保留名；用 --id <新 id> 换一个
id %s 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个
配置里已经有 agent %s；用 --id <新 id> 换一个
id %s 与已有的 agent %s 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个
```

`AppendBlock` 的阶段前缀：`读原文件: `、`备份: `、`写入: `。下面这些是冻结代码里的英文错误文本：它们不单独成行，但会拼在别的行后面上屏（与 open item (l) 同一类）：
- 执行器与 provider，经 `识别 %s 的模型失败: %s` 上屏：`discovery command path is empty`、`stdin pipe: %w`、`stdout pipe: %w`、`exit code %d: %s`、`exit code %d`、`model discovery killed: %w`、`%w: %v`（`DiscoverModels` 的两种包装）。
- 备份，经 `写配置文件失败: 备份: …` 上屏：`backup names %s and %s-2..%d all taken`。两个 `%s` 都是不带后缀的备份名，意思是不带后缀的那个名字与 `-2` 到 `-99` 全被占了。v1 只写了 `-2..99`，漏了不带后缀的那个；v1.1 补上，C6 逐字钉着。W1b 修订的 R16 在 repl 屏幕上钉着它经 `写配置文件失败: 备份: …` 上屏的整行。

### 3.4 claude 的 initialize 请求行（stdin 的全部字节，92 字节，含末尾 `\n`）

```
{"type":"control_request","request_id":"council-models","request":{"subtype":"initialize"}}
```

### 3.5 追加段（R3 全部七步之后，文件相对原文多出的字节；Go 字符串字面量，逐字）

```
"\n# council /add 追加：claude 识别出的 claude-sonnet-5（订阅）\n[agents.sonnet-5-high]\ndisplay_name = \"Sonnet 5\"\nprovider = \"claude\"\nmodel = \"claude-sonnet-5\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：claude 识别出的 claude-sonnet-5（订阅）\n[agents.sonnet-5-max]\ndisplay_name = \"Sonnet 5 max\"\nprovider = \"claude\"\nmodel = \"claude-sonnet-5\"\neffort = \"max\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：claude 识别出的 claude-mdx-lowmed-2-0（订阅）\n[agents.\"mdx-lowmed-2.0-low\"]\ndisplay_name = \"Mdx Lowmed 2.0 low\"\nprovider = \"claude\"\nmodel = \"claude-mdx-lowmed-2-0\"\neffort = \"low\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：claude 识别出的 claude-haiku-4-5-20251001（订阅）\n[agents.\"haiku-4.5\"]\ndisplay_name = \"Haiku 4.5\"\nprovider = \"claude\"\nmodel = \"claude-haiku-4-5-20251001\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：grok 识别出的 grok-4.6（本地配置）\n[agents.g2]\ndisplay_name = \"Grok 4.6\"\nprovider = \"grok\"\nmodel = \"grok-4.6\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n\n# council /add 追加：claude 识别出的 claude-mdx-plain-1-2（订阅）\n[agents.mdx-plain-max]\ndisplay_name = \"Mdx Plain 1.2 max\"\nprovider = \"claude\"\nmodel = \"claude-mdx-plain-1-2\"\neffort = \"max\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
```

R2 第 2 步只追加其中第一段（从开头到第一个 `mode = "discussion"\n` 为止，含开头那个 `\n`）。追加段的注释行是 `# council /add 追加：<provider> 识别出的 <model>（<来源>）`。

### 3.6 用户可见字面量清单

本刀新增的用户可见字面量就是 §3.2、§3.3 列的全部、§3.1 的三个 `Label()` 取值、§3.5 的注释行格式、备份文件名格式 `<配置文件>.bak-add-YYYYMMDD-HHMMSS[-N]`、`AgentBlockTOML` 写出的键名与顺序。实现腿不许新增任何别的用户可见字面量。

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；主代理落地后逐条跑，**从副本还原，不用 `git checkout`**。表里的变异都是原型上跑过的（「原型实测」：原型的 dump 与金标准比对不一致或原型用例失败），杀手名是 §2 里对应的用例。

| # | 文件 | 变异（原文 -> 改成；` <NL> ` 是换行，tab 省略） | 预期被谁杀 |
|---|---|---|---|
| M1 | `internal/provider/discovery.go` | `v.Type != "control_response" \|\| v.Response == nil \|\| v.Response.RequestID != claudeDiscoveryRequestID` -> `v.Type != "control_response" \|\| v.Response == nil` | P3 第 2 段（谓词共用这个判据）、P4、R1 |
| M2 | `internal/provider/discovery_parse.go` | `if o.Value == "default" \|\| o.Disabled {` -> `if o.Disabled {` | P4、R1 |
| M3 | `internal/provider/discovery_parse.go` | `if o.Value == "default" \|\| o.Disabled {` -> `if o.Value == "default" {` | P4、R1 |
| M4 | `internal/provider/discovery_parse.go` | `model := o.ResolvedModel <NL> if model == "" { <NL> model = o.Value <NL> }` -> `model := o.Value` | P4、R1、R2（追加段的 model 行） |
| M5 | `internal/provider/discovery_parse.go` | `model := o.ResolvedModel <NL> if model == "" { <NL> model = o.Value <NL> }` -> `model := o.ResolvedModel` | P4、R1 |
| M6 | `internal/provider/discovery.go` | `for _, e := range m.Efforts { <NL> if e == "high" { <NL> return "high" <NL> } <NL> } <NL> return m.Efforts[0]` -> `return m.Efforts[0]` | P7、R1 |
| M7 | `internal/provider/discovery.go` | `return m.Efforts[0] <NL> }` -> `return "high" <NL> }` | P7、R1、R3 第 4 步 |
| M8 | `internal/provider/discovery.go` | `if len(parts) > 1 && len(parts[len(parts)-1]) == 8` -> `if false && len(parts) > 1 && len(parts[len(parts)-1]) == 8` | P7、R1、R3 第 5 步 |
| M9 | `internal/provider/discovery.go` | `if digit && prevDigit {` -> `if false && digit && prevDigit {` | P7、R1 |
| M10 | `internal/provider/discovery.go` | `if t := sanitizeModelID(tail); t != "" {` -> `if t := sanitizeModelID(tail); false && t != "" {` | P7、R1 |
| M11 | `internal/provider/discovery.go` | `if strings.HasPrefix(id, other+"-") {` -> `if false && strings.HasPrefix(id, other+"-") {` | P7、R1 |
| M12 | `internal/provider/discovery_parse.go` | `src := ModelSourceAgyQuota` -> `src := ModelSourceSubscription` | P5、R1 |
| M13 | `internal/provider/discovery_parse.go` | `name := strings.TrimSuffix(rest, " (default)")` -> `name := rest` | P6、R8（R1 杀不到：grok-4.7 在 R1 里本来就被藏起来） |
| M14 | `internal/provider/discovery_parse.go` | `case strings.HasPrefix(line, "  * "):` -> `case false && strings.HasPrefix(line, "  * "):` | P6、R8（同上） |
| M15 | `internal/provider/discovery_parse.go` | `if bytes.Contains(stdout, []byte("Available models:")) {` -> `if !bytes.Contains(stdout, []byte("Available models:")) {` | P8 第 12、13 行、R6 `unparseable` |
| M16 | `internal/provider/discovery_parse.go` | `if v.Status != "SUCCESS" \|\| v.Command == nil` -> `if v.Command == nil` | P8 第 7 行 |
| M17 | `internal/provider/discovery_parse.go` | `if errors.Is(err, context.DeadlineExceeded) { <NL> return nil, fmt.Errorf("%w: %v", ErrDiscoveryTimeout, err)` -> `if false && errors.Is(err, context.DeadlineExceeded) { <NL> return nil, fmt.Errorf("%w: %v", ErrDiscoveryTimeout, err)` | P9、R6 `each_kind` |
| M18 | `internal/provider/discovery_parse.go` | `return nil, fmt.Errorf("%w: %v", ErrDiscoveryFailed, err)` -> `return nil, fmt.Errorf("%w: %v", ErrDiscoveryTimeout, err)` | P9、R6 `each_kind` / `all_down` / `one_down` |
| M19 | `internal/provider/discovery.go` | `[]string{"-p", "--safe-mode", "--input-format"` -> `[]string{"-p", "--input-format"` | P2 |
| M20 | `internal/provider/discovery.go` | `CloseStdinAfter: isClaudeDiscoveryResponse,` -> `CloseStdinAfter: nil,` | P2、T1（假 CLI 走 exit 7） |
| M21 | `internal/provider/discovery.go` | `func isClaudeDiscoveryResponse(line []byte) bool { <NL> _, ok := claudeDiscoveryResponse(line) <NL> return ok <NL> }` -> `func isClaudeDiscoveryResponse(line []byte) bool { <NL> return bytes.Contains(line, []byte("control_response")) <NL> }` | P3 第 2 段 |
| M22 | `internal/provider/claude.go` | `ModelDiscovery: ModelDiscoveryControlHandshake,` -> `ModelDiscovery: ModelDiscoveryUnknown,` | P1、R1（首行变成 `正在识别模型: agy, grok`） |
| M23 | `internal/turn/discovery_exec.go` | `if c.CloseStdinAfter == nil { <NL> closeStdin() <NL> }` -> `if false { <NL> closeStdin() <NL> }` | T2（10 s 超时） |
| M24 | `internal/turn/discovery_exec.go` | `if c.CloseStdinAfter == nil { <NL> closeStdin() <NL> }` -> `if true { <NL> closeStdin() <NL> }` | T1（exit 7） |
| M25 | `internal/turn/discovery_exec.go` | `if pid > 0 { <NL> _ = killProcessTree(pid) <NL> } <NL> if cmd.Process != nil { <NL> _ = cmd.Process.Kill() <NL> }` -> `_ = pid` | T4（耗时 >= 5 s） |
| M26 | `internal/turn/discovery_exec.go` | `if waitErr != nil {` -> `if false && waitErr != nil {` | T3 |
| M27 | `internal/turn/discovery_exec.go` | `cmd.Stderr = &capWriter{buf: &stderrBuf, cap: maxDiscoveryStderrBytes}` -> `cmd.Stderr = nil` | T3（错误文本少了 stderr 那一段） |
| M28 | `internal/turn/discovery_exec.go` | `cmd.Dir = os.TempDir()` -> `cmd.Dir = os.Getenv("MDX_NO_SUCH_VAR")` | T5 |
| M29 | `internal/config/append.go` | `if strings.Contains(id, ".") {` -> `if true {` | C1 (b)、R2（追加段表名） |
| M30 | `internal/config/append.go` | `if strings.Contains(id, ".") {` -> `if false {` | C1 (a) 与读回、R3 第 4 步（带点号的 id 变成嵌套表，写后加载失败被恢复） |
| M31 | `internal/config/append.go` | `if a.Effort != "" { <NL> kv("effort", a.Effort)` -> `if true { <NL> kv("effort", a.Effort)` | C1 (b)、R3 第 5、6 步 |
| M32 | `internal/config/append.go` | `b.WriteString(`\"`)` -> `b.WriteString(`"`)` | C1 (c) |
| M33 | `internal/config/append.go` | `sep = "\n\n"` -> `sep = "\n"` | C2 第 3 个文件 |
| M34 | `internal/config/append.go` | `sep = "\n" <NL> } else {` -> `sep = "" <NL> } else {` | C2 第 2 个文件、R2（追加段前没有空行） |
| M35 | `internal/config/append.go` | `os.O_WRONLY\|os.O_CREATE\|os.O_EXCL` -> `os.O_WRONLY\|os.O_CREATE\|os.O_TRUNC` | C3 |
| M36 | `internal/config/append.go` | `backupPath, err = writeBackup(path, orig, time.Now())` -> `backupPath, err = path+".bak-add-nobak", nil` | C2（备份名）、R2（`<bak>` 找不到） |
| M39 | `internal/config/append.go` | `if windowsDeviceNames[strings.SplitN(id, ".", 2)[0]] {` -> `if false {` | C5、R4（`--id nul`） |
| M40 | `internal/config/append.go` | `if windowsDeviceNames[strings.SplitN(id, ".", 2)[0]] {` -> `if windowsDeviceNames[id] {` | C5 `nul.1`、R4（`--id com1.x`） |
| M41 | `internal/config/append.go` | `if !((c >= 'a' && c <= 'z') \|\| (c >= '0' && c <= '9')) {` -> `if c == '.' {` | C5 `opus_5.0`、R4（`--id opus-5_0`） |
| M42 | `internal/config/append.go` | `if foldAgentID(other) == fold {` -> `if false && foldAgentID(other) == fold {` | C5、R4 |
| M43 | `internal/config/append.go` | `if _, ok := c.Agents[id]; ok {` -> `if _, ok := c.Agents[id]; false && ok {` | C5 `opus-5.0`、R4（错误文本变成折叠那一句） |
| M44 | `internal/config/append.go` | `if reservedAgentIDs[id] {` -> `if false {` | C5、R4（`user` 被写进去后 Validate 拒、走恢复） |
| M46 | `internal/repl/addmodel.go` | `p, err := provider.Lookup(pid) <NL> if err != nil { <NL> continue <NL> }` -> `p, err := provider.Lookup(pid) <NL> if err != nil { <NL> out = append(out, pid) <NL> continue <NL> }` | R11 (a) |
| M47 | `internal/repl/addmodel.go` | `if a.Provider != m.Provider { <NL> continue <NL> } <NL> if a.Model == m.Model \|\| (m.Alias != "" && a.Model == m.Alias) {` -> `if a.Provider != m.Provider { <NL> continue <NL> } <NL> if a.Model == m.Model {` | R1（多出 opus 行、编号后移） |
| M48 | `internal/repl/addmodel.go` | `if a.Provider != m.Provider { <NL> continue <NL> }` -> `if false { <NL> continue <NL> }` | R1（`+11 agy agy-sonnet-4.6-thinking` 被藏、编号前移） |
| M49 | `internal/repl/addmodel.go` | `for _, row := range visible { <NL> m := row.m` -> `for vi, row := range visible { <NL> row.no = vi + 1 <NL> m := row.m` | R2 第 3 步 |
| M50 | `internal/repl/addmodel.go` | `if r.modelSnap == nil { <NL> if err := r.printf("正在识别模型` -> `if true { <NL> if err := r.printf("正在识别模型` | R2 第 3 步、R5 |
| M51 | `internal/repl/commands.go` | `refresh = true <NL> r.modelSnap = nil` -> `refresh = true` | R5 |
| M52 | `internal/repl/addmodel.go` | `if !reflect.DeepEqual(onDisk, r.cfg) {` -> `if false && !reflect.DeepEqual(onDisk, r.cfg) {` | R7 `agent`、`store_thought`、`teams`、`providers` |
| M54 | `internal/repl/addmodel.go` | `if rerr := config.RestoreFile(path, orig); rerr != nil {` -> `_ = orig <NL> if rerr := error(nil); rerr != nil {` | R8 |
| M55 | `internal/repl/addmodel.go` | `r.rn.SetConfig(newCfg) <NL> r.cfg = newCfg` -> `r.cfg = newCfg` | R2 第 5 步（`outcome refused`） |
| M56 | `internal/repl/addmodel.go` | `r.rn.SetConfig(newCfg) <NL> r.cfg = newCfg` -> `r.rn.SetConfig(newCfg)` | R2 第 3、4 步；R3 第 5 步（守卫拒绝） |
| M57 | `internal/repl/addmodel.go` | `if err := r.svc.Join(ctx, r.cur.ID, p); err != nil { <NL> return r.reportErr("加入失败", err) <NL> }` -> `_ = p` | R2 第 2、3 步 |
| M59 | `internal/repl/addmodel.go` | `if !found {` -> `if false && !found {` | R4（`--effort ultra`） |
| M60 | `internal/repl/addmodel.go` | `if a.Provider != m.Provider \|\| a.Effort != effort {` -> `if a.Provider != m.Provider {` | R3 第 2 步 |
| M61 | `internal/repl/addmodel.go` | `if dup := sameBrainAgent(r.cfg, m, effort); dup != "" {` -> `if dup := sameBrainAgent(r.cfg, m, effort); false && dup != "" {` | R3 第 3 步 |
| M62 | `internal/repl/addmodel.go` | `if r.rn == nil { <NL> return r.printf("%s\n", msgNoRunner) <NL> } <NL> ` -> `（删掉）` | R9 (a)（panic） |
| M63 | `internal/repl/addmodel.go` | `if path == "" { <NL> return r.printf("不知道配置文件在哪，没法写入\n") <NL> } <NL> ` -> `（删掉）` | R9 (b) |
| M64 | `internal/repl/addmodel.go` | `cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout)` -> `cctx, cancel := context.WithCancel(ctx)` | R6 `each_kind`（假执行器 3 s 后报 `mdx: ctx never cancelled`） |
| M65 | `internal/repl/addmodel.go` | `case errors.Is(err, provider.ErrDiscoveryTimeout):` -> `case false:` | R6 `each_kind` |
| M66 | `internal/repl/addmodel.go` | `for _, f := range snap.failures {` -> `for _, f := range map[bool][]snapFailure{true: snap.failures}[len(visible) == 0] {` | R6 `one_down` |
| M67 | `internal/repl/addmodel.go` | `if r.modelSnap == nil { <NL> if err := r.printf("正在识别模型: %s\n", strings.Join(pids, ", ")); err != nil { <NL> return err <NL> } <NL> snap := takeModelSnapshot(ctx, r, pids)` -> `if err := r.printf("正在识别模型: %s\n", strings.Join(pids, ", ")); err != nil { <NL> return err <NL> } <NL> if r.modelSnap == nil { <NL> snap := takeModelSnapshot(ctx, r, pids)` | R2 第 3 步、R5 |
| M70 | `internal/repl/commands.go` | `if len(args.fields) != 1 { <NL> return r.printf("%s\n", msgAddModelUsage)` -> `if false { <NL> return r.printf("%s\n", msgAddModelUsage)` | R4（`/add --refresh x`） |
| M71 | `internal/repl/addmodel.go` | `if row.no > 0 && configuredAgentFor(r.cfg, row.m) == "" {` -> `if row.no > 0 {` | R2 第 3 步（`+14` 行还在） |
| M72 | `internal/repl/addmodel.go` | `if configuredAgentFor(r.cfg, m) == "" { <NL> row.no = next` -> `if true { <NL> row.no = next` | R1（编号从 +2 起） |
| M73 | `internal/config/append.go` | `kv("role", a.Role) <NL> ` -> `（删掉）` | C1、R2 |
| M74 | `internal/repl/addmodel.go` | `Permissions: config.PermissionRead,` -> `Permissions: config.PermissionWorkspace,` | R2 第 2、4 步 |
| M75 | `internal/repl/addmodel.go` | `DisplayName: m.SuggestedDisplayName(effort),` -> `DisplayName: m.Name,` | R2 第 2 步（`display_name = "Sonnet"`）、R3 |
| M68 | `internal/repl/repl.go` | `New` 里 `discover:        turn.RunDiscovery,` -> `discover:        nil,` | R10 |
| M69 | `internal/repl/repl.go` | `New` 里 `discoverTimeout: discoveryTimeout,` -> `discoverTimeout: time.Second,` | R10 |
| M76 | `internal/turn/discovery_exec.go` | `if c.CloseStdinAfter != nil && c.CloseStdinAfter(line) {` -> `if c.CloseStdinAfter != nil {` | T1（exit 8；W1 的 S11） |
| M77 | `internal/repl/addmodel.go` | `for _, pid := range pids { <NL> if ctx.Err() != nil { <NL> break <NL> } <NL> cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout) <NL> models, err := provider.DiscoverModels(cctx, pid, r.cfg.Providers[pid].Command, r.discover) <NL> cancel()` -> `cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout) <NL> defer cancel() <NL> for _, pid := range pids { <NL> if ctx.Err() != nil { <NL> break <NL> } <NL> models, err := provider.DiscoverModels(cctx, pid, r.cfg.Providers[pid].Command, r.discover)` | R6 `each_kind`（grok 行变超时；W1 的 S14） |
| M78 | `internal/repl/addmodel.go` | `if !reflect.DeepEqual(onDisk, r.cfg) {` -> `if !reflect.DeepEqual(onDisk.Agents, r.cfg.Agents) {` | R7 `store_thought`、`teams`、`providers`（W1 的 S3） |
| M79 | `internal/repl/addmodel.go` | `if !reflect.DeepEqual(onDisk, r.cfg) {` -> `if !reflect.DeepEqual(onDisk.Agents, r.cfg.Agents) \|\| !reflect.DeepEqual(onDisk.Providers, r.cfg.Providers) {` | R7 `store_thought`、`teams` |
| M80 | `internal/repl/addmodel.go` | `if onDisk.DataDir != r.cfg.DataDir \|\| onDisk.RulesFile != r.cfg.RulesFile {` -> `if false {` | R7 `data_dir`、`rules_file` |
| M81 | `internal/repl/addmodel.go` | `if onDisk.DataDir != r.cfg.DataDir \|\| onDisk.RulesFile != r.cfg.RulesFile { <NL> return r.printf("配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入\n") <NL> } <NL> if !reflect.DeepEqual(onDisk, r.cfg) { <NL> return r.printf("配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add %s\n", tok) <NL> }` -> `if !reflect.DeepEqual(onDisk, r.cfg) { <NL> return r.printf("配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add %s\n", tok) <NL> } <NL> if onDisk.DataDir != r.cfg.DataDir \|\| onDisk.RulesFile != r.cfg.RulesFile { <NL> return r.printf("配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入\n") <NL> }` | R7 `data_dir`、`rules_file`（两支对调） |
| M82 | `internal/repl/addmodel.go` | `if onDisk.DataDir != r.cfg.DataDir \|\| onDisk.RulesFile != r.cfg.RulesFile {` -> `if onDisk.DataDir != r.cfg.DataDir {` | R7 `rules_file` |
| M83 | `internal/repl/addmodel.go` | `snap := takeModelSnapshot(ctx, r, pids) <NL> if ctx.Err() != nil {` -> `snap := takeModelSnapshot(ctx, r, pids) <NL> if false {` | R14 `first`、`late`（取消的失败被缓存，第二次不再识别） |
| M84 | `internal/repl/addmodel.go` | `for _, pid := range pids { <NL> if ctx.Err() != nil { <NL> break <NL> } <NL> cctx, cancel` -> `for _, pid := range pids { <NL> cctx, cancel` | R14 `first`（调用次数 3 而不是 1；`late` 杀不到：grok 是最后一家，后面没有要跳过的） |
| M85 | `internal/repl/addmodel.go` | `if ctx.Err() != nil { <NL> break <NL> } <NL> cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout) <NL> models, err := provider.DiscoverModels(cctx, pid, r.cfg.Providers[pid].Command, r.discover) <NL> cancel()` -> `cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout) <NL> models, err := provider.DiscoverModels(cctx, pid, r.cfg.Providers[pid].Command, r.discover) <NL> cancel() <NL> if cctx.Err() != nil { <NL> break <NL> }` | R1（判的是每家自己的 cctx：`cancel()` 之后它永远非 nil，第一家识别完就 break，连这一家自己的结果也没收进快照；识别部分只剩 `正在识别模型: agy, claude, grok` 一行，调用次数 1。W1b 订正：v1.1 这里写的「表里只剩 agy」不对。它几乎让每条 repl 用例都红，只是一个能编译的近似，不说明「判 cctx」有专门的担保） |
| M86 | `internal/repl/addmodel.go` | `if len(snap.failures) > 0 { <NL> return r.printf("失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh\n")` -> `if false { <NL> return r.printf("失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh\n")` | R6 四段 |
| M87 | `internal/repl/addmodel.go` | `if a.Provider != m.Provider \|\| a.Effort != effort { <NL> continue <NL> } <NL> if a.Model == m.Model \|\| (m.Alias != "" && a.Model == m.Alias) {` -> `if a.Provider != m.Provider \|\| a.Effort != effort { <NL> continue <NL> } <NL> if a.Model == m.Model {` | R13 `alias`（W1 的 S2） |
| M88 | `internal/repl/addmodel.go` | `if row.no > 0 && configuredAgentFor(r.cfg, row.m) == "" {` -> `if configuredAgentFor(r.cfg, row.m) == "" {` | R13 `deleted`（W1 的 S46） |
| M89 | `internal/repl/addmodel.go` | `if a.Provider != m.Provider \|\| a.Effort != effort {` -> `if a.Effort != effort {` | R13 `provider`（V5 第 (4) 点） |
| M90 | `internal/repl/addmodel.go` | `} else if snap.okCount > 0 {` -> `} else if false {` | R11 (c)（W1 的 S1） |
| M91 | `internal/provider/discovery_parse.go` | `if name == "" \|\| strings.ContainsAny(name, " \t") \|\| hasControlChar(name) {` -> `if name == "" \|\| strings.ContainsAny(name, " \t") {` | P8 第 15、21 行（W1 的 S4） |
| M92 | `internal/provider/discovery_parse.go` | `if m.Label == "" \|\| hasControlChar(m.Label) {` -> `if m.Label == "" {` | P8 第 16 行（W1 的 S5） |
| M93 | `internal/provider/discovery_parse.go` | `if model == "" \|\| hasControlChar(model) {` -> `if hasControlChar(model) {` | P8 第 18 行（W1 的 S6） |
| M94 | `internal/provider/discovery_parse.go` | `if model == "" \|\| hasControlChar(model) {` -> `if model == "" {` | P8 第 17 行（W1 的 S6b） |
| M95 | `internal/provider/discovery_parse.go` | `if name == "" \|\| strings.ContainsAny(name, " \t") \|\| hasControlChar(name) {` -> `if strings.ContainsAny(name, " \t") \|\| hasControlChar(name) {` | P8 第 20 行 |
| M96 | `internal/provider/discovery_parse.go` | `if c < 0x20 \|\| c == 0x7f { <NL> return true` -> `if c < 0x20 { <NL> return true` | P8 第 21 行 |
| M97 | `internal/provider/discovery_parse.go` | `if v.Response.Subtype != "success" \|\| v.Response.Response == nil {` -> `if v.Response.Response == nil {` | P8 第 19 行（W1 的 S39） |
| M98 | `internal/config/append.go` | `for i := 1; i <= maxBackupAttempts; i++ {` -> `for i := 1; i < maxBackupAttempts; i++ {` | C6（第 99 个名字） |
| M99 | `internal/config/append.go` | `fmt.Errorf("backup names %s and %s-2..%d all taken", base, base, maxBackupAttempts)` -> `fmt.Errorf("backup names %s-2..%d all taken", base, maxBackupAttempts)` | C6（v1 的文本） |
| M100 | `internal/repl/addmodel.go` | `if len(discoverableProviders(r.cfg)) == 0 { <NL> return r.printf("配置里没有能识别模型的 provider，/add +编号 用不了\n")` -> `if false { <NL> return r.printf("配置里没有能识别模型的 provider，/add +编号 用不了\n")` | R11 (b) |
| M101 | `internal/repl/addmodel.go` | `if len(discoverableProviders(r.cfg)) == 0 { <NL> return r.printf("配置里没有能识别模型的 provider，/add +编号 用不了\n")` -> `if true { <NL> return r.printf("配置里没有能识别模型的 provider，/add +编号 用不了\n")` | R4（第一步） |
| M102 | `internal/repl/addmodel.go` | `if backup != "" {` -> `if false {` | R15 |
| M103 | `internal/repl/addmodel.go` | `if !reflect.DeepEqual(onDisk, r.cfg) {` -> `onDisk.Teams = r.cfg.Teams <NL> if !reflect.DeepEqual(onDisk, r.cfg) {` | R7 `teams`（W1b 的 X1） |
| M104 | `internal/repl/addmodel.go` | `if !reflect.DeepEqual(onDisk, r.cfg) {` -> `onDisk.Providers = r.cfg.Providers <NL> if !reflect.DeepEqual(onDisk, r.cfg) {` | R7 `providers`（W1b 的 X2） |
| M105 | `internal/repl/addmodel.go` | `snap := takeModelSnapshot(ctx, r, pids) <NL> if ctx.Err() != nil {` -> `snap := takeModelSnapshot(ctx, r, pids) <NL> if ctx.Err() != nil && snap.okCount == 0 {` | R14 `late`（W1b 的 X5） |
| M106 | `internal/repl/addmodel.go` | `snap := takeModelSnapshot(ctx, r, pids) <NL> if ctx.Err() != nil {` -> `snap := takeModelSnapshot(ctx, r, pids) <NL> if ctx.Err() != nil && len(snap.rows) == 0 {` | R14 `late`（W1b 的 X7） |
| M107 | `internal/repl/addmodel.go` | `snap := takeModelSnapshot(ctx, r, pids) <NL> if ctx.Err() != nil {` -> `snap := takeModelSnapshot(ctx, r, pids) <NL> if ctx.Err() != nil && len(snap.failures)+snap.okCount < len(pids) {` | R14 `late`（只在循环提前结束时才算取消；W1b 修订「同类还有没有别处」补的） |
| M108 | `internal/repl/addmodel.go` | `if backup != "" {` -> `if true {` | R16（W1b 的 X4） |
| M109 | `internal/repl/addmodel.go` | `if !reflect.DeepEqual(onDisk, r.cfg) {` -> `if !reflect.DeepEqual(onDisk.Agents, r.cfg.Agents) \|\| onDisk.StoreThought != r.cfg.StoreThought {` | R7 `teams`、`providers`（W1b 的 X3） |

M68、M69 没进 v1 原型的金标准比对（原型的 repl 场景都手动覆盖了这两个字段），由 R10 按构造担保；v1.1 原型上实跑过，都只红 R10。M37、M38、M45、M53、M58 编号空着：原型阶段判为等价或只在审查时看（见下）。

**v1.1 的实测（2026-09-28，`E:\Isolating_test\council\md1_v11\`）**：
- 表是由脚本从本文逐行解析成变异的：` <NL> ` 拆行，行首缩进按宽松匹配，锚点在文件里必须恰好命中一次，否则记为失败。
- 每条变异都从字节副本还原，还原后比对字节相等。跑的是四个包的全部 `TestMdx*`。
- M76 起是 v1.1 新增，括号里是 W1 的原编号；M76-M102 每一条都实际变红，点名的杀手在其中。
- 旧行 M1-M75 在 v1.1 原型上照样全部被点名的杀手杀掉。
- M13 的单元格里写着「R1 杀不到」，那是说明，不是杀手。

**W1b 修订的实测（2026-09-28，`E:\Isolating_test\council\md1_fix\`，用完已删）**：
- 原型照样由脚本从本文逐字取出 §1.1 的冻结代码与补丁落盘（拆分之后的 A3、A3b 两个文件）。
- 变异器同上：锚点必须恰好一处，从字节副本还原并比对字节相等。
- 本轮只重写了 R1、R7（六个子测试）、R14（两个子测试）、R16 四条用例，下面说的「实测」都只在这四条上：
  - M103-M109 每条的点名杀手实际变红，而且在这四条用例里只红表里写的子测试。
  - M52、M78、M79 重跑，按新的子测试订正了杀手列；M83、M84 按 R14 的两个子测试订正。
  - M85 单独施加、只跑 R1，按实跑输出订正了描述。
  - M102 在 R16 上不红：它只动 `backup` 非空那一支。
- 拆分之后，表里每一行的锚点都在对应文件里重新核对过：恰好一处，施加后 `go vet` 通过。M1-M21、M91-M97 的代码字节没变，只换了文件列，杀手没有重跑。
- 其余旧行的杀手（R2-R6、R8-R13、R15，以及 P、T、C 各条）本轮没有重跑，沿用 v1.1 的实测。

**判定为等价、不列杀手的变异**（写在这里，免得被当成漏测）：

- `discoverableProviders` 不看 `ModelDiscovery.Available()`、只看 `Lookup`：今天登记的三家都可用，可观测行为相同；反方向（把某家的能力位改成 Unknown）由 M22 担保。
- 去掉 `r.svc.SetThoughtPolicy(newCfg.StoreThought)`：追加一段 agent 不会改 `store_thought`，新旧值相同。
- `cmdAdd` 里 `+` 分支挪到读成员名单之后：只多一次库读。
- 三家识别改成并发：屏幕按 provider 字典序拼出来时输出相同，只差耗时。

**只在审查时看（没有用例能确定地抓，集成员收割时逐字核对）**：

- `writeFileAtomic` 必须是「同目录临时文件 + `Sync` + `Rename`」，失败删临时文件；改成直接 `os.WriteFile` 在用例里观测不到（只有写到一半断电才看得出）。
- `RunDiscovery` 的 `Env` 保持 nil（继承登录态）；stdout / stderr 封顶值；`discoveryKillGrace` 的取值。
- `configcmd.go` 头注释逐字等于 §1.1 B5；各文件的注释要点齐全、没有与代码不符的话（上一批独立审查的 major 就是注释说错）。
- provider 包仍然不 import `os/exec`、`os`（`discovery.go` 与 `discovery_parse.go` 都只 import `bytes`、`context`、`encoding/json`、`errors`、`fmt`、`strings`）。
- （W1b 修订）`discovery.go` 不引用 `discovery_parse.go` 里的任何标识符（`DiscoverModels`、`ParseDiscovery`、三个解析器、`hasControlChar`）：A-1a 落地后单独 `go build ./internal/provider` 能过，就是这条的核对。
- （v1.1；W1b 修订后只剩读阶段）`AppendBlock` 在读原文件阶段失败、`backup` 为空的那一支没有用例：要在守卫的 `Load` 成功之后、`AppendBlock` 之前把文件弄没，是竞态。备份阶段那一半 v1.1 写的是「本机造不出确定的写法」，这句不对：预建一个时间窗里的全部备份名就造得出，现在由 R16 钉着（M108）。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- **分五块交付（v1.1 写死四块，W1b 修订把 A-1 拆成 A-1a、A-1b），每块是一次独立调用，grok 每次只交一块**。行数是原型里冻结代码的实数（A-1a、A-1b 是 W1b 原型的实数，其余是 v1.1 的），「加注释」是按注释要点估的上限：

  | 块 | 文件（§1.1 条目） | 冻结代码 | 加注释约 | 依赖（编译要用前面哪块的标识符） | 这块落地后主代理跑 |
  |---|---|---|---|---|---|
  | **A-1a** | 新文件 `internal/provider/discovery.go`（A3） | 265 行 | 320-360 行 | 无：只用 provider 包既有的 `IDs()` | `go build ./internal/provider`（单独能过：本文件不引用 A-1b 的任何标识符，W1b 原型实测） |
  | **A-1b** | 新文件 `internal/provider/discovery_parse.go`（A3b） | 162 行 | 200-230 行 | A-1a：`DiscoveredModel`、`ModelSource` 的三个非零取值、五个 `ErrDiscovery*`、`DiscoveryExec`、`DiscoveryCommandFor`、`claudeDiscoveryResponse`（及它返回的 `claudeDiscoveryLine`、`claudeModelOption` 的字段）、常量 `claudeDiscoveryRequestID` | `go build ./internal/provider` |
  | **A-2** | 新文件 `internal/turn/discovery_exec.go`（A4）；补丁 `internal/provider/capabilities.go`（A1）、`claude.go` / `agy.go` / `grok.go`（A2） | 130 + 9 行 | 190-220 行 | 只依赖 A-1a（W1b 原型实测：只有 A-1a、没有 A-1b 时 `go build ./...` 也过）：`provider.DiscoveryCommand`、`provider.ModelDiscovery` 及其三个取值 `ModelDiscoveryControlHandshake`、`ModelDiscoverySubcommand`（A1、A2 的字段与字面量）；另用 turn 包既有的 `prepareCmd`、`capWriter`、`maxSpawnOutputBytes`、`killProcessTree` | `go build ./...`（此时 repl 还没引用，应当编译过） |
  | **B-1** | 新文件 `internal/config/append.go`（B1）；补丁 `internal/repl/configcmd.go` 头注释（B5，只改注释） | 201 行 + 8 行注释 | 250-280 行 | 无：config 包既有的 `idRegexp`、`idPattern`、`reservedAgentIDs`、`sortedIDs`、`AgentConfig`；B5 不涉及代码 | `go build ./internal/config ./internal/repl` |
  | **B-2** | 新文件 `internal/repl/addmodel.go`（B2）；补丁 `internal/repl/repl.go` 三处（B3）、`internal/repl/commands.go` 两处加头注释（B4） | 335 + 9 + 15 行 | 430-470 行 | A-1a：`provider.DiscoveredModel`、`DiscoveryExec`、`ModelDiscovery.Available`、`ModelSource.Label`、五个 `ErrDiscovery*`；A-1b：`DiscoverModels`；另用 provider 包既有的 `Lookup`；A-2：`turn.RunDiscovery`、`Capabilities().ModelDiscovery` 字段；B-1：`config.NewAgentIDProblem`、`AgentBlockTOML`、`AppendBlock`、`RestoreFile` | `go build ./...`、`go vet` 四个包，再合并测试腿 |

  - 顺序：A-1a 最先；A-1b 与 A-2 都只依赖 A-1a，两者先后不限；B-2 在 A-1a、A-1b、A-2 之后；B-1 不依赖任何一块，可以排在 B-2 之前的任何位置。签名一律以 §1.1 为准，前面的块落地之后按真文件核对。
  - 拆分（W1b 修订）：v1.1 的 A-1 是一个 417 行冻结代码的文件，加注释约 500 行，超过「每块 400 行上下」，也超过 grok 单次约 400 行、8 分钟的纪律，是最容易超时或截断的一块。现在按声明整段挪成两个文件，代码一个字不改，依赖单向（§1.1 A3 抬头）。注释要点必须齐，但每条写短。超时或截断就整块重交，不许省略。
  - B-2 仍是 430-470 行，略超 400，不拆：`addmodel.go` 用 B3 加的三个字段，B3、B4 又用 `addmodel.go` 的标识符，两边互相依赖，拆开的任何一半单独都编译不过。grok 若在 B-2 上超时，退路是把 B3、B4 的补丁单独交一次（B-2b），与 `addmodel.go` 一起落地之后再编译。
  - 只许改表里列出的文件；不改 `go.mod`；不写 `_test.go`。
- 新文件：

  ```
  ===FILE: internal/provider/discovery.go===
  （完整内容）
  ===END FILE===
  ```

- 补丁块，每个文件可以有多个：

  ```
  ===PATCH: internal/repl/commands.go===
  <<<<ANCHOR
  （从 HEAD ab53c0b 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```

  锚点必须从 HEAD `ab53c0b` 的真文件逐字复制（`28845e0` 的代码与它逐字节相同；本文代码块里的缩进可能被排成空格，以真文件为准），在该文件里恰好出现一次；若 CO1 已先落地，`repl.go` 的锚点按落地后的真文件重取（见文件头的相交表）；同一文件的多个锚点互不重叠。三个适配器的锚点三行（`WriteBoundary:` 那一行 + `\t}` + `}`）在各自文件里恰好一次（`Capabilities()` 字面量的结尾）。`commands.go` 的两个锚点见 §1.1 B4，第二个锚点里的 `return printAddCandidates(r, candidates)` 在全文件恰好一次。
- 不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。代码必须能 `gofmt`（tab 缩进；A2 的对齐见表）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的字符串字面量只有 §3 列的那些。
- 生产代码新标识符不许以 `mdx` 开头（那是测试腿的前缀）。
- §1.1 给出的代码照抄；只有注释由你写，且必须覆盖每条「注释要点」，不许写与代码不符的话。
- 每块回复末尾必须有：

  ```
  ===REPORT===
  补全性：本块（A-1a / A-1b / A-2 / B-1 / B-2 之一）的文件是否都给了；本块对应 §1.1 的哪几条。
  锚点：每个补丁块锚点的第一行。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  ```

---

## 6. 落地后主代理要做的

1. **收割**（A-1a、A-1b、A-2、B-1、B-2 各一次，每块落地后跑 §5 表里那一列的命令）：越界扫描（主仓库、`internal/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；按锚点落盘；`gofmt -l`；`go vet ./internal/provider ./internal/turn ./internal/config ./internal/repl`；`go build ./...`。逐字核对：§3 的字面量；B5 的注释；A2 三处对齐。
2. **合并测试腿的文件**之后，只跑 `./internal/provider ./internal/turn ./internal/config ./internal/repl`，再跑全量 `go test -count=1 ./...`（原型实测全绿、既有用例零改动）。另一条流水线也在跑 go test 时，3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制。
3. **§4 的变异逐条跑**（从副本还原）；任何一条没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
4. **文档**：
   - `docs/02_AGENT_PROVIDER_ARCHITECTURE.md`：`:34` 能力发现那一行补 claude（initialize 握手）；`:64` 与 1.3 节补「已落地（commit）」与实际的取值（`ControlHandshake` 是设计之外新增的一种）；grok 那一级只反映本地配置。
   - `docs/11d_STEP8C_CONTRACT.md` §4（`:626-632`）后追加订正记录：被 #45 推翻，新增 agent 多一条 `/add +编号` 的路。
   - `docs/09a_STEP6_API_CONTRACT.md`：登记 `Capabilities.ModelDiscovery`、`provider.DiscoverModels` / `DiscoveryCommandFor` / `ParseDiscovery` / `DiscoveredModel` / 五个哨兵、`turn.RunDiscovery`。
   - `docs/README.md` 路线图：#45 第一节已落（写 commit）。
   - `docs/00_DECISIONS.md` #45 第一节补落地记录与本刀的 open items；本契约原文归档进 `docs/`（按现有编号体例取名，文件头写归档说明）。本刀的 open items（起草时已知，不在本刀修）：
     (a) claude 夹具是 API key 隔离探针的形状，#45 没记 Griffin 真实登录的 10 行，`disabled` 字段名未实测 —— 真机复验时把 `/add` 的整张表记进 #45；
     (b) `--model <resolvedModel>`（尤其带 `[1m]` 的写法）能不能被 claude 接受，没有真推理就证实不了（真机复验第 5 步的可选项，要花钱）；
     (c) **待 Griffin**：「已在配置里」按模型判、不按（模型, 强度）判。一行被藏起来之后，只有它在拍快照那一刻还不在配置里、拿到过编号，才能 `/add +N --effort <另一档>` 再加一个强度，而且要记得编号。拍快照时已在配置里的模型没有编号：开会话前就配好的都是，Griffin 的 fable-5.1 一定是，opus 在 Max 订阅下 value 若是 `opus` 也是。`/add --refresh` 与新会话也会让本会话加进来的行失去编号。这些模型换强度只能手改配置。要不要给已配置的 claude 行也编号，以便换强度，由 Griffin 定；
     (d) **待 Griffin**：新成员的 `role` 是空串，Griffin 现有成员都有一句 role；事后补 role 会改指纹、付一次入场费。要不要默认抄一句，由 Griffin 定；
     (e) **待 Griffin**：`/add` 的 summary 没改（`TestUx2rJoinIsAdd` 逐字钉着），`/help` 里看不到 `+编号` 与 `--refresh`，只有不带参数的 `/add` 表后那一行提示。要改就得授权改那条既有测试；
     (f) 设备名与折叠撞名只在 `/add +编号` 这条路上拦，手写配置仍能写进 `nul`、`opus-5-0` 这种 id（#37(h) 仍未修）；
     (g) 不检测 CRLF（CRLF 文件追加段是 LF，TOML 两种都认）；
     (h) 三家串行识别，agy 一家就要 4-5 s；
     (i) 拍快照时已在配置里的行不编号，之后那个 agent 被 `/config reload --apply` 删掉了，这一行也不会重新出现，要 `/add --refresh`（R13 `deleted` 钉着）；
     (j) `turn` 包的 `TestMain` 由本刀测试腿新增；SM1 已被告知不许在 turn 包新增、也不许依赖它，CO1 不新写（见文件头相交表）；
     (k) `repl.go` 与 CO1 相邻改动（结构体末尾、`New` 字面量、import 的 `"time"`），后落地的一方按真文件重取锚点、不重复 import（见文件头相交表）；
     (l) `ErrDiscoveryFailed` 那一类失败行里是 Go 的英文错误文本（`model discovery command failed: ...`）；
     (m) `/add 1,+2` 这种混写走旧路径，`+2` 那一段报「配置里没有可加入的 +2」，不说「+编号 要单独一条」；
     (n) 备份文件只增不删（`config.toml.bak-add-*` 会越积越多）；
     (o) agy 的「两家重名」前缀按已登记的 provider id 判，将来接 codex 之后 `codex-` 开头的 label 也会被改成 `agy-` 前缀，届时要复核。
     下面四条是 W1b 修订登记的，都**待主代理**（不是待 Griffin：前三条要不要改冻结代码、第四条怎么跨契约分工，由主代理定；本轮取保守读法，冻结代码一个字没改）：
     (p) **待主代理**：`writeBackup` 已经建好某个备份名、写到一半（`Write` 或 `Close`）失败时（例如磁盘满），那个残缺的文件不删、返回空串，屏幕那一行是 `写配置文件失败: 备份: …`，不提这个文件（与 (n) 同类，盘上多一个用户不知道的文件）。修法是在这两处失败时 `os.Remove(name)`，要改冻结代码；
     (q) **待主代理**：F7 的「同类还有没有别处」没做完。claude 行的 `supportedEffortLevels` 原样取自 JSON，不过控制字符过滤：它经 EFFORTS 列、`支持的强度是 %s，没有 %s` 那一行上屏，强度不是 `high` 时还原样拼进 display_name（进身份块与指纹；写进配置时按 TOML 转义，不会写坏文件）。`hasControlChar` 只判 C0 与 DEL，不判 U+0080-U+009F（其中 U+009B 在不少终端里等同 CSI）。两处的修法都会改变哪些行、哪些强度被列出，属行为改动：一是 `parseClaudeDiscovery` 跳过含控制字符的强度档（或整行）并把 `hasControlChar` 扩到 C1，P8 加两行、§4 加变异；二是维持现状。W1b 只读代码推演过，没有实跑；
     (r) **待主代理**：`/config reload` 有两种拒绝：`data_dir` / `rules_file` 变了（F2 已单列一句），与新配置通不过 `Validate`（`configcmd.go:93-117`，「配置非法，旧配置原样生效」）。守卫只做 `config.Load`、不做 `Validate`，所以文件被改得通不过校验时，`/add +编号` 照样打「先 /config reload --apply 让它生效」，而 reload 一定会拒绝。不是死胡同（reload 会列出校验错误，用户照着改），但「让它生效」这半句在这里不成立。修法是守卫在整份比之前对 `onDisk` 调 `Validate`、失败时单打一行（新字面量、R7 子测试与变异），属行为改动；
     (s) **待主代理**：R7 按 Config 的字段一个一个钉守卫（六个字段六个子测试，B2 注释要点 6）。SM1 给 Config 加 `Summary` 字段之后，「守卫漏比 Summary」这种写法没有子测试抓。要不要补、由哪一刀的测试腿补（MD1 不知道 `[summary]` 的键名与 reload 会不会应用它，写不出那个子测试），要通知 SM1 契约，由主代理定。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **真机复验**（Griffin，Windows Terminal + PowerShell 5.1）。**会写配置文件，所以用一份拷贝**：先在 PowerShell 里准备（这一块不是 council 命令）：

   ```powershell
   New-Item -ItemType Directory -Force E:\council-md1-check | Out-Null
   Copy-Item C:\Users\25142\.council\config.toml E:\council-md1-check\config.toml
   ```

   用记事本把 `E:\council-md1-check\config.toml` 里的 `data_dir` 改成 `'E:\council-md1-check\data'`（不碰日常库），然后启动：

   ```powershell
   council --config E:\council-md1-check\config.toml
   ```

   进去之后下面每条命令**单独敲、逐行回车**，不要整块粘贴（#44）：

   ```
   /new 识别复验
   ```

   ```
   /add
   ```

   （v1.1，F3）看到 `正在识别模型: agy, claude, grok` 之后、表出来之前（agy 一家就要 4-5 秒），按一次 Ctrl-C。看：只多一行 `识别被取消；下次 /add 会重新识别`，不出表、不出失败行。这一步验的是 TUI 的 Ctrl-C 真能取消识别、取消之后不缓存；R14 只在 repl 层用已取消的 ctx 验过，没走 TUI（`tui/model.go:341-347`）。按晚了、表已经出来，这一步就作废，记下来即可。

   ```
   /add
   ```

   看：先打配置里的候选（与以前一样），再打一行 `正在识别模型: agy, claude, grok`（上一步取消过，这里应当重新识别；若这一行没出现、直接出了表或失败行，说明取消被缓存了），几秒后出表；表里 claude 的行来源是 `订阅`，grok 的行是 `本地配置`，agy 的 Claude / GPT-OSS 行是 `走 agy 额度`；你已经配置过的（`Gemini 3.8 Flash (High)`、`grok-4.7`、`claude-fable-5-1`）不在表里；记下你的 opus 那一行在不在（在，说明 Max 下 value 不是 `opus`，记进 #45）。**把整张表截图或复制给主代理**：这就是 #45 缺的那 10 行 claude 原件。

   ```
   /add
   ```

   看：这次没有 `正在识别模型` 那一行，秒出，编号与上一次一模一样。

   挑一个 claude 行（例如 sonnet），把它的编号记为 N：

   ```
   /add +N
   ```

   看：两行「已把 agent … 追加进 …（原文件备份在 …）」与「… 已加入会话 …」；用记事本打开 `E:\council-md1-check\config.toml`，原有内容一个字没动，末尾多了一段 `# council /add 追加：…` 开头的表；同目录多了一个 `config.toml.bak-add-<日期>-<时间>`。

   ```
   /agents
   ```

   看：新 agent 在，`EFFORT` 是 `high`、`PERMISSIONS` 是 `read`、`MODE` 是 `discussion`。

   ```
   /add
   ```

   看：刚加的那一行不见了，其余编号不变。

   ```
   /add +N --effort max
   ```

   看：加进来一个 `-max` 结尾的新成员（同一模型换强度是新 id）。

   ```
   /add --refresh
   ```

   看：又出现 `正在识别模型` 那一行。

   ```
   /quit
   ```

   再用同一条命令启动一次 council，横幅里的 agent 数比拷贝前多 2，没有报配置错误。

   **可选、要花钱**（一次 claude 冷启动，约 1 美元量级）：`/ask` 新加的那个成员一次，确认 `--model claude-sonnet-5` 这种 resolvedModel 写法真能用；若想确认带 `[1m]` 的写法（例如 `claude-sonnet-5[1m]`），要另加一个那样的成员再 `/ask` 一次。**这两件事原型与测试都担保不了**（不许起真 claude 推理），见 open items。

   复验完删掉 `E:\council-md1-check`（它的 data 是一次性的）。
6. 清理本刀在中转根与 scratchpad 里的全部中间物。
