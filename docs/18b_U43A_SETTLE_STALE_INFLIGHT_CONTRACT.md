# U43A 冻结契约：/session rebuild 先结清陈旧的在飞记录，再丢 provider 会话（#43(a)）（v1.2，2026-09-28）

> **落地后订正（2026-09-28，主代理；U43A 已于 `17b3236` 落地）**：
> - §4「判定为等价」第 4 条（对调「读失败」与「!found」两支）**不是等价的**：真 store 在读失败时返回 found == false，先判 !found 就把错误吞成「没有这一行」，行为与 M3 相同，杀手是 T7、R4。实现照冻结的顺序写，没有偏离。
> - X2d、X3 两条变异在集成时存活，与契约「只在审查时看」的登记一致；集成员核对 settle_stale.go 与本契约逐字相同（只有一个 defer Unlock、没有 Background）。
> - 独立审查余项（代码都没错）：非成员（DriftAgentGone 放行）+ 陈旧行那条路径没有用例；简洁档下闸被占那句与 L4、L5 没有用例；三处注释措辞偏满（settle_stale.go 说闸被占时那一行「是正在跑的那一轮自己的」、runner.go:361 仍写「留到重启对账」、B3「下一轮付一次入场费」没限定本来有会话）。
> **主代理裁定（2026-09-28 14:30，先读；覆盖契约别处与之冲突的说法）**：
> - §7.2 第 6 条**采纳**：T4c（挂在 ProviderSession 上的一次性确定性探针，写法照 §7.2 第 6 条，原型已实测）成为 §2.1 的正式验收用例，测试腿交付；§4 X2b 的杀手改为 T4c，集成员照跑。X2d 仍只在审查时看（逐字核对 settle_stale.go）。
> - §7.2 第 7 条**不采纳**：真机步骤不加「假 claude」保险；§6.1 冻结的播种脚本另开连接读回并打印那一行，打出 `u43-probe spawned read` 才敲 /ask，这已足够。
> - §7.2 第 1 条（`/help` 里 `/session` 那一行）不并进 CXH，以后单独一刀。**已修（CH1 `4d3dc0b`）**：`/help` 的 `/session` 一行现在提到「先结清这个成员留在库里的陈旧在飞记录」。
## v1.1 变更记录（2026-09-28，基线 HEAD `4dbd17a`）

W1 审查的 F0-F2 与证伪判词 V0-V1（`E:\multi_model_cowork\u43a_w1_findings.md`）经主代理裁决全部接受（`u43a_rulings.md`），凡证伪者更正处按更正做。本版在隔离拷贝 `E:\Isolating_test\council\u43a_v11\`（`git archive 4dbd17a`）上先按 v1 重建全部实现与 §2 全部用例、确认全绿，再逐条落实；新增与改动的期望值都在这份原型上实跑拿到，原型用完已删。

1. **基线 `485581f` -> `4dbd17a`**（FU2 落地）。`git diff --stat 485581f 4dbd17a` 只动 `internal/provider/codex.go`、`internal/repl/addmodel.go`、`internal/repl/latercmd.go` 与 `fuy_codex_test.go`、`fux_review_test.go`、`fuy_x44_test.go`；本契约引用的生产文件与测试辅助文件零 diff，**全部行号不变**。§1.1 的 6 个锚点在 `4dbd17a` 上由脚本逐个核过，各恰好一处；按 §1.1 落盘后 `go build ./...`、`gofmt -l`、`go vet ./internal/conversation ./internal/turn ./internal/repl` 干净；连同 §2 全部用例（含新增的 T4b），全量 `go test -count=1 ./...` 九包全绿，既有 `_test.go` 零改动。§2.2 那个文件单独放进 `4dbd17a` 的拷贝跑：R1 四个、R3、R4、R7、R8 红，R2、R5、R6 绿，与 v1 相同。
2. **F0（major）**：§2.1 加 **T4b `TestU43SettleStaleHoldsGateAcrossRead`**，照 s10a G6 的写法做成确定性探针（一次性、挂在 `InflightFor` 上、`TryLock` 探闸，不睡、不起 goroutine，带同装置的正对照）；§4 加 **X2**（`defer g.mu.Unlock()` -> `g.mu.Unlock()`），杀手 T4b；M1 的杀手加上 T4b。原型实测：X2 下全部 `TestU43` 只有 T4b 红（v1 的 T1-T8、R1-R8 全活）；M1 下 T3、T4、T4b、R2 红；T4b 连跑 10 次全绿；M2 下 T4b 不死锁也不红。审查员那条「hook 里起同 key 的 Ask、等 started 或 300 ms」的可观测版本**不收**（它在好实现下只能靠超时收场；理由写在 T4b【判别力】末）。
3. **F0 附带（V0 第 4(a) 点）**：T4【判别力】里 M1 的失败机制改成原型实测的说法：不是 `ErrInflightMissing` 回滚，而是那一轮的 `RecordUsage` 撞 `turn_usage` 主键（1555），Ask 返回 `OutcomePostRejected`（2），`live` 那一行是 `outcome == 9`、没有 spoke 行。R2【判别力】里 M1 的说法同样按实测改（R2 红在前提守卫「第二轮 `err == nil`」）。`ErrInflightMissing` 只在被结清的是 begun 行时出现，这一支本原型没有造出来，契约里写明是读代码、未实测。F0 原文在 `u43a_w1_findings.md` 里不改；本契约里已经没有「`ErrInflightMissing` 整笔回滚」这个说法。
4. **F0 附带（V0 第 4(d) 点）**：「读完之后、`settleOne` 之前放闸」（X2b）登记进 §4「只在审查时看」：结清与 `g.st = nil` 落在闸外，与并发 Ask 构成数据竞争，本机不能 `-race`。原型实测 X2b 下全部用例活。另试了一个挂在 `ProviderSession` 上的同款探针，它能确定性地杀 X2b；本版照裁决不收，记 §7.2 第 6 条。
5. **F1（major）**：§6.1 的 seed 脚本给出整句插入语句，显式写 `tier='read'`、`planned_session_id=''`、`resume_session_id=''`；§6.4 的期望正文不改。原型照新语句插行实跑，三行屏幕与面包屑正文逐字对上；照 v1 的列插行，正文末尾多出「；工作目录  可能有半截落盘的文件」。§6.4 另补一句：`/log` 那一行在正文后还跟着 ` [event=turn_lost]`（原型实测的既有显示格式），免得真机上被当成不一致。
6. **F2（minor）**：§0 第 2 条「更糟的一支」改成「HEAD 上会话被丢了、成员却仍被锁」（R3 放到 `4dbd17a` 上实跑：只打「已丢弃……」，provider 会话被删，接着的 `/ask` 照样被拒），不再把「标不上可疑」算作伤害。§1.1 B2 冻结注释的前三行改成顺序的真实理由：一是 fail closed，二是两次取闸之间 `Rebuild` 被拒时可疑标记留得住（原型白盒模拟过这个窗口：结清之后由测试占住闸再调 `Rebuild`，provider 会话仍在、`baseline_suspect == 1`）。**这是 §1.1 唯一的冻结代码变动：只动注释（4 行变 5 行），代码一个字不变**；B2 冻结要点加一条「先结清再丢的理由」，R3【判别力】的引用跟着改。M18 的描述「从 `// 先结清陈旧的在飞记录` 起」仍然成立（新注释第一行仍以这几个字开头）。
7. **§4 全部重跑**：表里 24 条（M1-M21、M10b、M10c、X2）在 v1.1 原型上逐条重跑，每条点名的杀手全部变红（锚点恰好一处，从内存字节副本还原，跑完两个生产文件 SHA256 与跑前相同）。「只在审查时看」里的三个变异也跑了：X3（`settleOne` 用 `context.Background()`）全活；X4（`AllInflight` 过滤代替 `InflightFor`）被 T7、R4 顺带杀掉，T4b 红在前提守卫；X2b 全活。v1 说 X4「M10 系杀不到」没错，但它会被夹具顺带杀掉，这一点补进了那一条。
8. **随基线订正的其余几处**：文件头 FU2 那一条（已落地）与「另一份契约」那一条（CXH 现在读得到：文件与本刀零相交，测试前缀不撞）；§5 锚点基线；§6 第 4 步的变异清单加 X2；§7.2 第 1 条（CXH 只改 `commands.go:143`，没有收 `/session` 那一行）；§7.2 新增第 6 条。

### W1b 修订（v1.1 -> v1.2，2026-09-28，基线仍是 `4dbd17a`）

W1b（第二轮审查）的 1 条 major、3 条 minor 全部成立，照改；notes 里的旁注能落到契约上的也一并处理。其中两处涉及方向，不自己定，取保守读法（照旧不收、不加），写进 §7.2 标「待主代理」。**§1.1 冻结代码零变动。** v1.1 快照：`E:\multi_model_cowork\u43a_contract_v11.md`。

原型：`E:\Isolating_test\council\u43a_fix\`（`git archive 4dbd17a`；§1.1 A-E 由脚本从 v1.1 原文逐字取出落盘，6 个锚点各恰好一处；`go build ./...`、`go vet ./internal/conversation ./internal/turn ./internal/repl`、`gofmt -l` 干净）。重建了 6 条契约用例（T3、T4、T4b、T5、R2、R3），外加 4 个不进契约的探针：T4c 候选（挂在 `ProviderSession` 上的一次性探针）、X2b 白盒模拟、X2c 后果探针、§6.1 seed 脚本全程（Python 作为外部进程写 Go 端开着的库）。好实现下全部绿，T4、T4b、T4c 候选各连跑 10 次全绿。变异一律锚点恰好一处、从内存字节副本还原，每条还原后 `settle_stale.go` 的 SHA256 与跑前相同。原型用完已删。

1. **finding 1（major，X2b 的后果写轻了）：接受。** §4「只在审查时看」X2b 那一条按白盒模拟改写后果。本版原型复现了审查员的模拟：开口过的会话，结清前记下 `st0 := g.st`，写一行 spawned 陈旧行，照好实现结清（库里 `baseline_suspect == 1`），再把 `g.st` 放回 `st0`，接着问一轮 `claudeT1JSON`，结果 `rec.BaselineSuspect == false`、那一轮 `turn_usage.cost_usd == 0`（非 NULL）；对照（不放回）是 `true`、NULL。这是白盒模拟，没有复现真并发。同类的 X2d（成功那一支先 `Unlock` 再 `g.st = nil`）登记进「只在审查时看」；「判定为等价」第 1 条收紧为「显式 `Unlock` 必须在 `g.st = nil` 之后」。原型实测：X2b 下 T3、T4、T4b、T5、R2、R3 全绿，T4c 候选红；X2d 下这 6 条与 T4c 候选全绿；收紧后的等价写法下全绿。§7.2 第 6 条补上这两点，并附 T4c 候选的完整写法与实测杀伤面。**收不收 T4c 是方向**：裁决定的是「只在审查时看」，本版照旧不收，待主代理。
2. **finding 2（minor，X2c 没登记）：接受。** §4 表加 X2c（把 `InflightFor` 那一行挪到 `TryLock` 之前），杀手 T4b。T4b【判别力】和 §6 第 4 步的清单都补上 X2c。原型实测：X2c 下 T4b 红在 `sawGateFree` 那条断言上，前提守卫都过；T3、T4、T5、R2、R3 和 T4c 候选都绿。X2c 的后果另用一个不进契约的探针实测了 spawned 那一支：探针挂在 `InflightFor` 上，读完之后放行在飞那一轮并等它跑完，再返回。实测结果是 `SettleStaleInflight` 读到那一轮自己的 spawned 行，此后 `TryLock` 拿到了闸，接着 `Found == true`、`err` 是那一轮 turn_id 上的 `turn_usage` 主键冲突（1555）。在飞那一轮 `outcome == 1`，在飞记录已清，没有 lost 行。照 B2 的代码，屏幕会打 L4「……对它 /ask 仍会被拒」，这是假话：行已经没了（这一句是读代码推出来的，没经 REPL 实跑）。读到 begun 行的那一支会走 L3「没起过进程、没花钱」，同样是假话；这一支是读代码，未实测。
3. **finding 3（minor，T4b 末句）：接受。** 末句改写，大意是：T4 担保的是 M1 下的同一种灾难；X2、X2c 下没有可观测的用例，由 T4b 从结构上担保（闸跨过读）。原型实测 X2 下 T4 绿（`settle err=...in flight found=false`，Ask outcome 1），T4b 红。不收可观测版本的决定不变。
4. **finding 4（minor，seed 脚本没冻结 commit 与读回）：接受并加严。** §6.1 改成冻结整份 `u43_seed.py`：插入语句逐字不变；INSERT 之后 `commit`、关连接；再另开一条新连接读回，读回语句连 `participants`，只认活跃成员；§6 第 3、4 步写明「脚本恰好打出一行 `u43-probe spawned read` 才敲 `/ask`」。本版找到了 `claude_test` 环境（E 盘机：`D:\Install_path\Anaconda3\envs\claude_test\python.exe`，Python 3.13.3，sqlite 3.50.2），原型实测如下，Python 都是在 Go 端 store 开着时作为外部进程跑的：
   - 冻结脚本：打出 `u43-probe spawned read`（CRLF 行尾）；Go 端 `InflightFor` 读到 `tier == "read"`、`work_dir == ""`、`started_at == 2026-09-28T00:00:00Z`。随后 REPL 的 `/ask`、`/session rebuild`、`/log 3` 与 §6.4 逐字相同，`turn_usage` 里 `outcome = 1` 的行数为 0，没有起进程。简洁档、详细档各跑一遍。
   - 去掉 `commit`：什么都不打，退出码 0，Go 端没有行。回滚是静默的。
   - 不 `commit`、在同一条连接上读回：打出那一行，Go 端却没有行。所以读回必须另开连接。
   - 会话 id 抄错（冻结脚本）：什么都不打。`inflight_turns` 对 `conv_id` / `agent_id` 没有外键（`schema.go:389-402`），插入照样成功。读回不连 `participants` 的写法在同一情形下会打出那一行，而 Go 端在真会话上没有行。所以读回要连 `participants`。
   - 库路径抄错：`sqlite3` 在那个路径上新建一个空文件，INSERT 报 `no such table: inflight_turns`，退出码 1，什么都不打。
   审查员提的「一次性配置里 claude 的 command 指向假程序」这道保险会改变 Griffin 要跑的东西，而真 council 启动全程在这种配置下的表现没实测，所以不自己定：记 §7.2 第 7 条，待主代理。本版不加。
5. **notes 旁注**：
   - （notes 1）文件头 CXH 那一条补上 `internal/provider/turn.go`。重读了 13:26 那版 `cxh_contract.md`：CXH 现在改 6 个生产文件，多出来的 A6 只改注释。新测试文件仍是那 5 个 `cxh` 前缀的，仍写明不碰 `internal/turn` 的生产文件。与本刀零相交，落地顺序不受影响。
   - （notes 2）§4 运行方式里「T4b 在 M2 下不死锁也不红」标明是单跑 T4b 的实测：§4 规定 M2 只跑 repl 包，按那个跑法复现不出。本版单跑复核：M2 下 T4b 绿、不死锁，T4c 候选也一样。
   - （notes 3）T4b 的前提守卫「开口之后 `calls == 0 && !sawGateFree`」不可能为假，因为探针武装之前不计数，删去。「恰好 1 次」只挡得住 0 次，这一点写进【判别力】。
   - （notes 4）§6 第 4 步补一句：简洁档是真机默认，被拒的 `/ask` 在简洁档下打 `<成员显示名>: 没有回复（拒绝）`，详细档打 `outcome refused  agent <id>`，下一行都是拒绝那一句。原型实测，简洁档用的 REPL 配置里没有 `ag_wait`，所以显示名退回 id，打的是 `ag_wait: 没有回复（拒绝）`（`ask.go:346`、`:372`，`verbosity.go:50-55`）。
   - （notes 5）审查员越界读了 `~/.claude` 下 harness 存的 tool-results 文件。这与契约无关，记在这里供主代理知悉。本修订没读 `~/.claude` 下的任何文件。
6. **「同类的还有没有别处」（本版自查）**：
   - 「闸必须同时罩住读、结清、`g.st = nil`」这一类，本版看到的有 5 种写法（不敢说已经穷举）：X2（T4b）；X2b（只在审查时看，只有 T4c 候选杀得到）；X2c（T4b，本版登记）；X2d（只在审查时看，没有用例能抓）；闸的 key 写反，即 `sessionKey{convID: agentID, agentID: convID}`。最后一种本版试过：T3、T4、T4b、T5 两个子测试、R2 和 T4c 候选全红，已有用例杀得到，不进表。
   - 真机 seed 在 commit 之外的同类隐式依赖：「会话 id / 成员 id 抄错」和「库路径抄错」，都由新脚本的读回挡住（见上面第 4 条的实测）。
7. **本版没重跑的**：M3-M21、M10b、M10c、X3、X4。本版的改动碰不到它们：§1.1 代码零变动；用例文本只删了 T4b 的一条前提守卫、改了几处【判别力】措辞。本版实跑的变异是 X2、X2b、X2c、X2d、收紧后的等价写法、M1（T3、T4、T4b、R2 红，与表一致）、M2（只跑 T4b 与 T4c 候选）、闸 key 写反。

---

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `4dbd17a`（= origin/main；v1 起草时是 `485581f`，差别见上方变更记录第 1 条）。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`（不设 GOFLAGS）。本机没有 gcc，不能 `-race`。
- 来源：`docs/00_DECISIONS.md` #43 剩余项 (a)（:2085-2086）与 (f)（:2092）；#46 末尾 Griffin 2026-09-28 11:30「接受，这些你可以决定」授权主代理定待定项。本刀是 (a) 的修法；(f)（同进程双重故障留下的行）与 (b)（取消留下的 begun 行）留下的行**顺带**能用本刀解开，但 (b) 的成因本身不修。
- 上游契约：`docs/13_STEP10_CONTRACT.md` §1.4（:263-291，SettleLostTurns 作为第二个写入口的豁免）、§3.5（:457-490，「两个方法都只在启动时、进 REPL 之前跑」）、§3.8（:661-700，A 之五的闸与 2026-09-27 订正，:700「解开的路只有重启对账」）、§3.9（:702-741）、§3.14（:936-943）；`docs/13a_S10_A5_CONTRACT.md`（A 之五）§0 第 5 条、§1.1 E1 注释要点 4、§4「只在审查时看」第 3 条「注释里不许写『/session rebuild 可以解开』」—— **本刀落地后这三处作废**，§6 写订正。
- 与 FU2：**已落地**（`4dbd17a`，只动 `internal/repl/latercmd.go`、`internal/provider/codex.go`、`internal/repl/addmodel.go` 的注释与 `fuy_codex_test.go`、`fux_review_test.go`、`fuy_x44_test.go` 三个测试文件）。与本刀**文件不相交**；v1.1 原型就建在它之上，本刀两个新测试文件与 FU2 的测试同包编译、零撞名（原型实测全量九包全绿）。
- 与 CXH（codex 模型接进 `/add` 识别、`/help` 补 `/add +编号` 与 `--refresh`；v1 起草时叫「另一份契约」，只知道有 `cxh_draft\`）：本刀 v1.1 修订时只读了它当时的文本（`E:\multi_model_cowork\cxh_contract.md`，它也在同步修订；只核文件清单与标识符前缀）。它动的生产文件是 `internal/provider/codex.go`、`internal/provider/discovery.go`、`internal/provider/discovery_parse.go`、`internal/provider/turn.go`（v1.2 补：CXH 13:26 那版新增的 A6，只改注释）、`internal/repl/addmodel.go`、`internal/repl/commands.go`（`:143`，`/add` 那一行），共 6 个，外加三个既有测试文件（`mdx_discovery_test.go`、`cxx_codex_test.go`、`ux2r_add_test.go`）与五个 `cxh` 前缀的新测试文件（其中 `internal/turn/cxh_discovery_exec_test.go` 与本刀的 `u43_settle_stale_test.go` 同包）；它写明「不碰 `internal/turn` 的任何生产文件」、不新增 `TestMain`。与本刀下面点名的文件与函数**零相交**，测试前缀 `cxh` / `u43` 互不撞名，落地先后无要求。CXH 若在之后的修订里改动下列任何一处，照下面的串行规则办。本刀动的文件与函数逐个点名如下，任何一份契约若动到其中任何一处，**两刀串行落地，后落的那一刀按先落之后的真文件重取锚点**；都不动则无顺序要求：
  - `internal/repl/ask.go`：`cmdSessionRebuild` 的函数注释（:248-255）与函数体（:287-291 之间插一段）；`askCause` 里 `ErrInflightConflict` 分支上方的注释（:615-618）。`askCause` 的返回值（:620 那句话）与其余分支一个字不动。
  - `internal/turn/runner.go`：只改 `askUpToWith` 里 A 之五那段注释的两行（:374-375）。代码一个字不动。**#46「待主代理」里「失败轮的 usage_raw 不落库，要改得动 internal/turn」那一刀（不是 CXH；v1.1 修订时还没有它的契约）将来多半改 `runner.go` 的失败路径（`failClosed` / `absorbParse` 一带），与 :374-375 不重叠，但同一文件，照上面的串行规则。**
  - `internal/turn/recover.go`：只在 `settleOne` 的函数注释（:139-140）之后追加三行注释。代码一个字不动。
  - `internal/conversation/service.go`：只改 `SettleLostTurn` 函数注释的一行半（:590-591）。代码一个字不动。
  - 新文件 `internal/turn/settle_stale.go`（新导出 `Runner.SettleStaleInflight`、类型 `StaleSettlement`）。另一份契约若在 `turn` 包里新加同名标识符，编译期就会撞，先落者为准。
  - 本刀**不动** `internal/repl/commands.go`（`/help` 里 `/session` 那一行 :153 不改，见 §7）。#46 裁定的「`/help` 补上 `/add +编号` 与 `--refresh`」那一批就是 CXH，它只改 `:143`，与本刀不相交。
- 本契约的全部期望值（屏幕逐字节、库里的行、错误文本）已在隔离拷贝里用一份原型实现（§1.1 全部生产改动 + §2 全部用例）实跑得出：v1 在 `E:\Isolating_test\council\u43a_draft\`（基线 `485581f`）；v1.1 在 `E:\Isolating_test\council\u43a_v11\`（基线 `4dbd17a`，§1.1 的代码块由脚本从本文逐字取出落盘）上重建全部实现与用例后再落实裁决。v1.1 原型下全量 `go test -count=1 ./...` 九包全绿，**既有 `_test.go` 零改动、零红**；§2.2 那个文件在 `4dbd17a` 的隔离拷贝上单独跑过，红绿见 §2 通用纪律末尾；§4 的全部变异逐条在 v1.1 原型上跑过、全部被点名的杀手杀掉（锚点恰好一次、从内存字节副本还原，跑完核对两个生产文件的 SHA256 与跑前相同）。v1.2（W1b 修订）在 `E:\Isolating_test\council\u43a_fix\`（基线 `4dbd17a`）上重建 §1.1 与 T3、T4、T4b、T5、R2、R3，落实 W1b 的四条发现；新写进契约的期望与说法（§4 的 X2b 白盒模拟、X2c、X2d，§6.1 的整份 seed 脚本）都在这份原型上实跑拿到，范围见上方「W1b 修订」。原型不交付，已删除。

---

## 0. 病根 / 动机（行号以 HEAD `4dbd17a` 为准，均已打开真文件核对；这些文件 `485581f` -> `4dbd17a` 零 diff）

1. **A 之五的闸把人拒了，却没有 REPL 入口能解开。** `internal/turn/runner.go:380-386`：`askUpToWith` 拿到闸之后先 `r.svc.InflightFor(ctx, convID, agentID)`，查到一条不属于本轮的在飞记录就 `refuse(... ErrInflightConflict)`，屏幕上是 `internal/repl/ask.go:620` 那句话（「……重启 council 会在启动时重新对账……」）。`runner.go:374-375` 的注释自己写着：「怎么解开：重启 council，启动对账成功就删掉这一行。/session rebuild 解不开：它只删 provider_sessions 那一行，不碰 inflight_turns。」

2. **`/session rebuild` 在这种状态下「成功」，并且什么也没解开（HEAD 上实测）。** `Runner.Rebuild`（`runner.go:235-256`）只 `DropProviderSession` 并把 `g.st` 置 nil；`cmdSessionRebuild`（`ask.go:256-309`）在 :291 调它。把 §2.2 的 R1（spawned）原样放到 HEAD 的隔离拷贝上跑，屏幕是（提示符 `council> ` 已剥掉）：

   ```
   正在问 ag_wait ...
   outcome refused  agent ag_wait
   这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行
   已丢弃 ag_wait 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢），下一轮按首轮计价
   正在问 ag_wait ...
   outcome refused  agent ag_wait
   这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行
   ```

   「已丢弃……下一轮按首轮计价」之后下一轮照样被拒。**HEAD 上会话被丢了、成员却仍被锁**：`Rebuild` 照样删掉 `provider_sessions` 那一行（下一轮要付入场费），在飞记录原样留着，A 之五照拒。结清会持续失败的那一类也一样（§2.2 R3 的主键冲突夹具原样放到 `4dbd17a` 的隔离拷贝上跑：屏幕只有「已丢弃……」一行，provider 会话被删，接着的 `/ask` 照样被拒）。记账上不多丢钱：那一行删了，下一轮走 Fresh，旧基线不再参与差值（`runner.go:209-215`）；之后对账真结清时 `found == false` 传 nil，`turn_usage` 的 lost 行与面包屑照补（`recover.go:159-169`，§2.2 R7 实测的就是这一支）。伤害是用户付了代价、以为解开了，其实没有。先结清再丢的理由见 §1.1 B2 冻结注释。

3. **今天唯一的解开路是重启。** `internal/app/app.go:193-205`：启动时 `AuditInflight` -> `SettleLostTurns`，失败时 `:202` 往 Err 打 `启动在飞对账补账失败: %v`。这条路结构上只在进 REPL 之前跑（`recover.go:23-31`、13 §3.5）。所以：
   - 同进程里的双重故障（13 §3.14、#43(f)）与 `BeginTurn` 之后的一次取消（#43(b)）留下的行，把这个成员锁到重启为止；
   - 启动对账持续失败时（例如 `turn_usage` 主键冲突，13a E1 `settle_fails` 的夹具），重启也解不开，只能手改 SQLite。

4. **本刀能解开什么、不能解开什么（照实写）。** 不变量 (b) 要求结清必须走启动对账的同一条路。于是：
   - 能解开：闸空闲时留下的、**这一次写得进去**的陈旧行 —— (f) 与 (b) 的行（当时的失败是瞬时的，现在写得进去）、以及任何「重启一次就能解开」的行，现在不用重启。
   - **不能解开**：启动对账**持续**失败的那一类（主键冲突、坏行）。同一条路、同一个失败，本刀对它 fail closed：一行说明、什么都不改、成员照样被拒（§2.2 R3、§2.1 T8 担保）。要解开它得有一条不走启动对账的写法，那违反 (b)，本刀不做，记 §7「待 Griffin」。

5. **13 §1.4 / §3.5 的豁免要换一个依据。** 豁免原文：`SettleLostTurns`「只在启动时、进 REPL 之前跑一次，所以它与 `RecordTurn` 在结构上不可能并发。……哪天它变成可以中途调用的东西，这条豁免同时作废」。本刀让 `settleOne` 多一个中途的调用方，所以必须换依据：**闸**。现查 HEAD 上 `provider_sessions` 与 `inflight_turns` 的全部写入点（`grep "DropProviderSession(|SettleLostTurn(|\.RecordTurn(|SetProviderSession("`，生产代码）：
   - `RecordTurn`：`runner.go:818`、`:845`、`:1112`，都在 `askUpToWith` 持有的 `g.mu` 之下；
   - `DropProviderSession`：`runner.go:242`（`Rebuild`，`TryLock` 之下）、`:498`（`askUpToWith` 指纹不符那一支，`g.mu` 之下）；
   - `SettleLostTurn`：`recover.go:144`、`:171`，只经 `settleOne`，今天只经启动时的 `SettleLostTurns`；
   - `BeginTurn` / `SetInflightPhase` / `ClearInflight`：只在 `askUpToWith` 里（`g.mu` 之下）与上面两个事务内。
   同一个 `(会话, 成员)` 的每一个写入方都在同一把 `g.mu` 之下（启动对账在进 REPL 之前、没有闸也不会并发）。新调用方持有这把闸，就与 `RecordTurn` 在结构上不可能对同一 key 并发；`settleOne` 在事务之外读回 `ProviderSession` 再整行写回，这一读一写之间没有别人改那一行。**`AuditInflight`（全表读）仍然不导出给 REPL**：新调用方只用 A 之五加的定点读 `InflightFor`，而且是在拿到闸之后读 —— 读到的行不可能是正在跑的那一轮自己的（本 key 的每一轮都在同一把闸下写入并在放闸之前清掉或留成陈旧行，这是 A 之五 E1 注释要点 1 的同一条论证）。

6. **为什么入口是 `/session rebuild`，不另开命令。** 任务点名它；它是用户明确敲的维护命令（结清只在用户敲了命令时发生，§1 不变量 (d)）；它本来就要丢 provider 会话，「结清 + 丢会话」合起来就是「把这个成员整个重置」，不留「结清了但会话还在、下一轮金额报不知道」这种半状态。代价照实写在屏幕上：结清之后紧接着的是既有那行「已丢弃……下一轮按首轮计价」（入场费）。重启对账不丢会话、不付入场费，所以重启仍是更便宜的那条路 —— 要不要再开一个「只结清、不丢会话」的入口，记 §7「待 Griffin」。**`/rules reload --rebuild`（`rules.go:221`）也调 `Rebuild`，本刀不让它结清**：那条命令一次重建 N 个成员，用户没有点名任何一个，替他结清违反 (d)；它因此仍会对被锁的成员打「已重建」而对方照样被拒，同类问题记 §7。

**主代理冻结的四条不变量**（削弱任何一条都是 blocker）：
- (a) 本进程的 Runner 对这个 `(会话, 成员)` 有一轮正在跑（它的 sessionKey 闸被占着）时拒绝，一行说明，不读、不碰任何行；
- (b) 陈旧的在飞记录必须经与启动对账同一条路结清（`settleOne(classifyLost(行))`：`turn_usage` 补 lost 行、面包屑、与启动对账一致的 meta、整行回写 `baseline_suspect`、删行，一个事务；begun 行只删行，那是启动对账对 begun 的同一种处置），不许裸删，不许让一次可能花了钱的轮次在只追加的事实源里消失；
- (c) 只动这个 agent 在当前会话里的行；
- (d) 用户可见的每一句都冻结（§3）；结清只在用户敲了 `/session rebuild <id>` 时发生；屏幕说清结清了什么（turn id、phase）、这一轮算丢失（或「没起过进程、没花钱」）、下一轮会怎样（紧跟的既有 Rebuild 那一行：首轮计价 / 本来就是首轮 / 按当前配置会被拒）。

---

## 1. 范围

### 1.1 做

凡写「名字实现方定」处实现方可自取（本刀没有这种地方）；其余名字、签名、字面量、顺序、判据、注释全文**全部冻结，逐字照抄**。生产代码新标识符不许以 `u43` 开头。

**缩进说明**：本文代码块里用的是 tab。锚点一律以 HEAD 真文件为准（每条给了行号），逐字复制真文件那几行（含 tab 与全角标点）。

#### A. 新文件 `internal/turn/settle_stale.go`（全文冻结，逐字）

```go
package turn

import (
	"context"
	"fmt"

	"council/internal/conversation"
)

// StaleSettlement 是 SettleStaleInflight 的结果。
//
// Found 为假：库里没有这一 (会话, 成员) 的在飞记录，什么都没做（err 为 nil 时）。
// Found 为真：Lost 是查到的那一行与 classifyLost 给它的人话；err 为 nil 时
// Settlement 是 SettleLostTurn 的回报，err 非 nil 时这一行没结清、库里什么都没改。
type StaleSettlement struct {
	Found      bool
	Lost       LostTurn
	Settlement conversation.LostSettlement
}

// SettleStaleInflight 把 (convID, agentID) 一条陈旧的在飞记录，按启动对账的同一条路结清。
// 只由 /session rebuild 调用：结清只在用户明确敲了命令时发生。
//
// 取 g.mu 用 TryLock，拿不到就返回包了 ErrTurnInFlight 的错误，什么都不读、
// 什么都不写。本进程里这一 key 的每一轮都在同一把 g.mu 下写入在飞记录，
// 并在释放闸之前清掉或留成陈旧行；闸拿到了，查到的行就不属于任何一轮正在跑的调用。
// 拿不到就拒绝、不等，与 Rebuild 同体例：一轮最长 5 分钟，REPL 不该为一条
// 维护命令卡住这么久；那一行此刻是正在跑的那一轮自己的，不是陈旧的。
//
// 结清走 settleOne(classifyLost(行))，与启动时 SettleLostTurns 逐条走的是同一个函数：
// turn_usage 补一行 OutcomeLost、turn_lost 面包屑、provider_sessions 整行读回再标
// baseline_suspect、删在飞记录，四件一个事务；begun 行（进程没起、没花钱）只删行，
// 那是启动对账对 begun 的同一种处置。不许另写一份，也不许绕过 settleOne 直接删
// inflight_turns 的行：一次可能花了钱的轮次不能在只追加的事实源里消失。
//
// 13 §1.4 / §3.5 给启动对账的豁免是「它与 RecordTurn 在结构上不可能并发」。
// 这里不靠「只在启动时跑」，靠的是这一 key 的闸：provider_sessions 与
// inflight_turns 上这一 (会话, 成员) 的每一个写入方（askUpToWith、Rebuild、本方法）
// 都在同一把 g.mu 之下，启动对账在进 REPL 之前跑。settleOne 在事务之外读回
// ProviderSession，这一读一写之间没有别人会改那一行，理由同此。
//
// 结清成功后把 g.st 置 nil：面包屑追加了、库里的基线可能刚标成可疑，内存快照已经过期，
// 下一轮要从库里重新读回。与启动对账之后内存为空是同一个状态。
// 结清失败时 g.st 不动：库里什么都没改。
//
// 读在飞记录失败返回错误、什么都不做：「读不出来」是「不知道」，不是「没有」。
func (r *Runner) SettleStaleInflight(ctx context.Context, convID, agentID string) (StaleSettlement, error) {
	g := r.gate(sessionKey{convID: convID, agentID: agentID})
	if !g.mu.TryLock() {
		return StaleSettlement{}, fmt.Errorf("settle stale inflight %s/%s: %w", convID, agentID, ErrTurnInFlight)
	}
	defer g.mu.Unlock()

	row, found, err := r.svc.InflightFor(ctx, convID, agentID)
	if err != nil {
		return StaleSettlement{}, fmt.Errorf("settle stale inflight %s/%s: read: %w", convID, agentID, err)
	}
	if !found {
		return StaleSettlement{}, nil
	}

	lost := classifyLost(row)
	got, err := r.settleOne(ctx, lost)
	if err != nil {
		return StaleSettlement{Found: true, Lost: lost}, fmt.Errorf("settle stale inflight %s/%s: %w", convID, agentID, err)
	}
	g.st = nil
	return StaleSettlement{Found: true, Lost: lost, Settlement: got}, nil
}
```

冻结要点（上面代码已体现，这里说判据，免得被「顺手改好」）：
- **闸**：`r.gate(sessionKey{...})` + `TryLock`，拿不到就返回、不 `Lock()` 等待。`ErrTurnInFlight` 复用既有哨兵（`runner.go:976`），不新加哨兵。
- **读**：`r.svc.InflightFor`（A 之五的定点读，走 `(conv_id, agent_id)` 唯一索引），**不许**用 `AllInflight` / `AuditInflight`（全表读，`recover.go:23-31`「不导出给 REPL」）。
- **结清**：`classifyLost(row)` 然后 `r.settleOne(ctx, lost)`，一个字不绕。`settleOne`、`classifyLost`、`lostUsage`、`lostNoticeMessage`、`Service.SettleLostTurn` 的代码一个字不改（§1.2）。
- **ctx** 用调用方的（与启动对账同：`app.go:193-200` 传的是 `app.Run` 的 ctx）。
- **`g.st = nil` 只在结清成功之后**；没查到行、读失败、结清失败三支都不碰 `g.st`。
- 三个错误包装文本冻结（它们经 §1.1 B2 的 `%v` 上屏）：`"settle stale inflight %s/%s: %w"`（闸被占、结清失败两处）、`"settle stale inflight %s/%s: read: %w"`（读失败）。
- 返回值：闸被占与读失败返回 `StaleSettlement{}`（`Found == false`）；结清失败返回 `StaleSettlement{Found: true, Lost: lost}`（`Settlement` 是零值）；成功返回三个字段都填。

#### B. `internal/repl/ask.go`

B1. `cmdSessionRebuild` 的函数注释末尾追加一段。锚点（真文件 :255-256，恰好一次）：

```go
// 审计失败时不放行也不重建：分不出类别就既不能放宽闸，也选不出不说假话的文案。
func cmdSessionRebuild(ctx context.Context, r *REPL, fields []string) error {
```

替换为（逐字）：

```go
// 审计失败时不放行也不重建：分不出类别就既不能放宽闸，也选不出不说假话的文案。
//
// 放行之后、Rebuild 之前，先经 Runner.SettleStaleInflight 结清这个成员在当前会话里
// 陈旧的在飞记录（#43(a)）：被 A 之五的闸拒绝之后，这是不重启就能解开的那条路。
// 结清走启动对账的同一个函数，只动这一 (会话, 成员)；/rules reload --rebuild
// 不经这里，不会替用户结清任何一行。
func cmdSessionRebuild(ctx context.Context, r *REPL, fields []string) error {
```

B2. **本刀的主体。** 在成员/漂移放行判断（:287-289）**之后**、`existed, err := r.rn.Rebuild(ctx, r.cur.ID, agentID)`（:291）**之前**插入。锚点（真文件 :287-291，恰好一次）：

```go
	if !(found || (rowFound && row.Kind == turn.DriftAgentGone)) {
		return r.printf("%s 不在当前会话的活跃成员里，用 /who 看看有哪些\n", agentID)
	}

	existed, err := r.rn.Rebuild(ctx, r.cur.ID, agentID)
```

替换为（逐字）：

```go
	if !(found || (rowFound && row.Kind == turn.DriftAgentGone)) {
		return r.printf("%s 不在当前会话的活跃成员里，用 /who 看看有哪些\n", agentID)
	}

	// 先结清陈旧的在飞记录，再丢 provider 会话，顺序不能反。一是 fail closed：
	// 结清失败或读不出来就停，不调 Rebuild，什么都不改、provider 会话也不丢，
	// 这个成员照样被 /ask 拒绝，用户看到的是原因。二是结清会整行读回
	// provider_sessions 标可疑，两次取闸之间 Rebuild 若被拒，这个标记留得住。
	// 没有在飞记录时一个字不打，往下与本刀之前逐字节相同。
	stale, err := r.rn.SettleStaleInflight(ctx, r.cur.ID, agentID)
	if err != nil {
		if errors.Is(err, turn.ErrTurnInFlight) {
			return r.printf("%s 此刻正有一轮在跑，什么都没丢；等这一轮跑完再敲一次\n", agentID)
		}
		if stale.Found {
			return r.printf("结清 %s 的在飞记录（turn %s）失败，什么都没改，provider 会话也没丢；对它 /ask 仍会被拒: %v\n", agentID, stale.Lost.Turn.TurnID, err)
		}
		return r.printf("读不出 %s 的在飞记录，什么都没改，provider 会话也没丢: %v\n", agentID, err)
	}
	if stale.Found {
		if err := r.printf("已结清 %s 的一轮在飞记录（turn %s，phase %s），对它 /ask 不会再因为这条记录被拒\n", agentID, stale.Lost.Turn.TurnID, stale.Lost.Turn.Phase); err != nil {
			return err
		}
		consequence := "那一轮记为丢失：可能花了钱，用量与金额都不知道（与重启时启动对账的记法相同）\n"
		if stale.Lost.Turn.Phase == conversation.PhaseBegun {
			consequence = "那一轮没起过进程、没花钱，只删掉了这条记录\n"
		}
		if err := r.write(consequence); err != nil {
			return err
		}
	}

	existed, err := r.rn.Rebuild(ctx, r.cur.ID, agentID)
```

冻结要点：
- **位置**：在成员闸与漂移审计**之后**（非成员、拼错的 id、别的会话的悬空行一律照旧在 :288 拒绝，不结清任何东西），在 `Rebuild` **之前**。
- **失败就停、不调 Rebuild**：闸被占、结清失败、读失败三支都 `return`，不往下走到 `Rebuild`。
- **先结清再丢的理由**（v1.1 订正注释措辞，W1 F2）：一是 fail closed，结清失败或读不出来时什么都不改、provider 会话也不丢（§2.2 R3、R4 担保；M11 把顺序反过来时 R3 红）；二是两次取闸之间 `Rebuild` 被拒时可疑标记留得住（见下面「两次取闸之间有一个窗口」那一条）。v1 注释写的「先丢就没有那一行可标了」不是理由：那一行删了，下一轮走 Fresh、旧基线不再参与差值，lost 行与面包屑在 `found == false` 时照补（§0 第 2 条）。
- **闸被占那一句复用 :298 的既有格式串**，逐字相同（不新造字面量；原型实测两处都打同一行）。它在这里可能出现，是因为 `SettleStaleInflight` 先于 `Rebuild` 取闸；`Rebuild` 那一处的分支（:297-299）不动。
- **两次取闸之间有一个窗口，接受**：`SettleStaleInflight` 放闸之后、`Rebuild` 再取闸之前，只有并发的调用方（例如 TUI 里一条 `/round` 正在跑）能插进一轮。插进来时屏幕是结清那两行之后接 :298 那一句「此刻正有一轮在跑，什么都没丢」—— 在飞记录已结清、provider 会话没丢，两句都是真话；结清写下的 `baseline_suspect` 留在库里，插进来的那一轮从库里读回（结清已把 `g.st` 置 nil）就看得见它（v1.1 原型白盒模拟过这个窗口：结清之后由测试占住闸再调 `Rebuild`，返回包了 `ErrTurnInFlight` 的错误，`provider_sessions` 那一行仍在、`baseline_suspect == 1`；这是 B2 注释「二是」那句的依据）。不为关掉这个窗口去改 `Rebuild` 的签名或把两件事并进一把闸（那会改 `Rebuild`，`rules.go:221` 也调它，§1.2）。这一支没有用例（要在两次取闸之间插一轮，只有真并发能造），列进 §4「只在审查时看」。
- **结清失败与读失败按 `stale.Found` 分**，不按错误文本分。
- **begun 与其它 phase 的分支判据是 `== conversation.PhaseBegun`**，与 `settleOne`（`recover.go:143`）同一个判据：非法 phase（schema 没有 CHECK，`schema.go:399`）走「记为丢失」那一句，与启动对账把非法 phase 按起过进程处理（`recover.go:75-79`）一致。
- **与简洁档无关**：这几行不看 `r.concise`，两档逐字相同（与 `cmdSessionRebuild` 既有各行同体例；#45「拒绝句与持久化相关的都照打」）。
- `conversation` 与 `errors` 两个 import HEAD 上已经在（`ask.go:3-12`），不加 import。
- 没有在飞记录时（`Found == false` 且 `err == nil`）不打任何东西，往下与 HEAD 逐字节相同（§2.2 R5 与既有 `session_inflight_test.go`、`rules_test.go`、`x39f_*` 担保）。

B3. `askCause` 里 `ErrInflightConflict` 分支上方的注释改后两行。锚点（真文件 :617-618，恰好一次）：

```go
	// 是同一句话。这句话不提 /session rebuild：那条命令不碰 inflight_turns，
	// 闸空闲时它会「成功」，下一次 /ask 照样被拒。解开只靠重启后的启动对账。
```

替换为（逐字）：

```go
	// 是同一句话。这句话只说重启（A 之五裁定的原句，本刀不改）；
	// /session rebuild <成员> 同样能解开：它先按启动对账的同一条路结清陈旧的
	// 在飞记录，再丢掉 provider 会话，下一轮付一次入场费。
```

`askCause` 的返回值（:620）一个字不动。

#### C. `internal/turn/runner.go`（只改注释）

锚点（真文件 :374-375，恰好一次）：

```go
	// 怎么解开：重启 council，启动对账成功就删掉这一行。/session rebuild 解不开：
	// 它只删 provider_sessions 那一行，不碰 inflight_turns。
```

替换为（逐字）：

```go
	// 怎么解开：重启 council，启动对账成功就删掉这一行；或者 /session rebuild
	// <成员>，它经 SettleStaleInflight 按启动对账的同一条路结清这一行（拿到这一
	// key 的闸之后才读，所以结清的只可能是陈旧行），再丢掉 provider 会话。
	// 结清本身失败（例如 turn_usage 主键冲突）时两条路都解不开。
```

`Rebuild`（:209-256）与 A 之五的闸（:380-386）一个字不动。

#### D. `internal/turn/recover.go`（只加注释）

锚点（真文件 :139-141，恰好一次）：

```go
// settleOne 组一条对账的三个入参。begun 全传 nil，只删行。
// 其余 phase 补 turn_usage、发面包屑；provider 会话行在才整行回写。
func (r *Runner) settleOne(ctx context.Context, row LostTurn) (conversation.LostSettlement, error) {
```

替换为（逐字）：

```go
// settleOne 组一条对账的三个入参。begun 全传 nil，只删行。
// 其余 phase 补 turn_usage、发面包屑；provider 会话行在才整行回写。
//
// 两个调用方：启动时的 SettleLostTurns，与 /session rebuild 经 SettleStaleInflight
// 在持有那一 key 的闸时结清单条。事务之外那次 ProviderSession 读回的安全性，
// 前者靠「进 REPL 之前」，后者靠闸，理由见 SettleStaleInflight。
func (r *Runner) settleOne(ctx context.Context, row LostTurn) (conversation.LostSettlement, error) {
```

`AuditInflight` / `SettleLostTurns` 的注释（:23-38、:111-124）**不改**：它们说的是这两个函数自己，仍然只在启动时跑，照旧成立。

#### E. `internal/conversation/service.go`（只改注释）

锚点（真文件 :590-591，恰好一次）：

```go
// 就回滚）。对账这一侧它只意味着「别人已经处理过了」。结构上不可能发生
// （只在启动时、进 REPL 之前跑一次），真发生了也该跳过不该报错。
```

替换为（逐字）：

```go
// 就回滚）。对账这一侧它只意味着「别人已经处理过了」。结构上不可能发生
// （两个调用方：启动时进 REPL 之前跑一次；/session rebuild 在持有那一
// (会话, 成员) 的闸时结清单条，同一 key 没有别的写入方），真发生了也该跳过不该报错。
```

### 1.2 不做（两条腿都不许越界）

- 不改启动对账本身：`AuditInflight`、`SettleLostTurns`、`settleOne`、`classifyLost`、`orphanSessionID`、`spawnedReason`、`lostUsage`、`lostNoticeMessage`、`Service.SettleLostTurn` 的**代码**一个字不改（§1.1 D、E 只动注释）；`internal/app` 一个字不改（横幅、Err 行、启动顺序）。
- 不改 `Runner.Rebuild`（代码与注释都不动）；不改 `internal/repl/rules.go`（`/rules reload --rebuild` 不结清，§0 第 6 条）；不改 `/help`（`commands.go:153`）。
- 不改 A 之五的闸（`runner.go:380-386`）与拒绝那句话（`ask.go:620`）；不加新哨兵；不加配置键；不改 schema。
- 不修 #43(b)（取消留下 begun 行的成因）、(c)（`/round` 预算表）、(d)（`/session` NEXT 列）、(e)（启动横幅）。
- 不给「启动对账持续失败」开任何绕开启动对账的写法（§0 第 4 条、§7）。
- **既有 `_test.go` 一律不改**（原型实测：§1.1 全部落地后全量既有用例全绿，没有一条需要改）。测试腿不写生产代码；实现腿不写任何 `_test.go`。

---

## 2. 验收（测试腿新写的文件）

通用纪律：

- 新文件两个（§2.1、§2.2）。包级标识符（类型、函数、变量、常量）一律以 `u43` 开头，测试函数名 `TestU43...`。不新写 `TestMain`：repl 包复用 `session_inflight_test.go:94` 的（假 CLI = 测试二进制自己，`COUNCIL_FAKE_CLI=1`，argv 没有 `--resume` 立刻回 `fakeClaudeSuccessJSON`，有就先睡 700ms）。
- **不起真 provider**：turn 包用 `fakeSpawner` 或本文件自造的阻塞 Spawner（赋给 `r.spawn`）；repl 包用 `newInFlightRunner`（Command 是 `os.Args[0]`）。store 一律 `store.Open` 到 `t.TempDir()` 之下，**全部是真 SQLite**，不碰 `C:\Users\25142\.council`。
- 断言整行相等、整段输出按行切片整体相等（`reflect.DeepEqual`）或库里的值精确相等；不写子串断言。例外只有两类：前提守卫（`Fatalf` 前的「前提不成立」）；以及 §2.2 里「整段里不许出现以 X 开头的行」这种**缺席**断言（每一条都配了只差一处的正对照，见各条【判别力】）。
- 屏幕输出的切行规则（repl 包，冻结）：`strings.Split(out, "\n")`，每一行反复剥掉前缀 `"council> "` 直到没有。原型实测：`runREPLRunner(t, svc, rn, "/use "+convID+"\n/session rebuild ag_wait\n")` 在 HEAD 与原型上、没有在飞记录时，剥完是 `["当前会话: <convID>，标题: u43", "已丢弃 ag_wait 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢），下一轮按首轮计价", "", "再见", ""]`（会话标题固定 `"u43"`）。下文「整段等于 [X, Y]」一律指 `["当前会话: <convID>，标题: u43", X, Y, "", "再见", ""]`。
- 每条用例上方注释写【判别力】：它能不能为假、只差一处的对照是哪条、点名杀哪几条 §4 变异。
- 允许只读引用同包既有测试辅助（不许改它们）：
  - turn：`newRealTurnStore`、`realTurnStore.svc` / `.st` / `.rawDB`（`step10a3_fixtures_test.go:43-83`）、`newConv`、`joinAgent`、`postUser`、`newTestConfig`、`newTestRunner`、`newTestRunnerSameWorkDir`、`fakeSpawner`（`enqueueSuccess` / `calls` / `spawn`）、`claudeT1JSON`、`testAgentClaude`（`"alice"`）、`testAgentAgy`（`"nova"`）、`sessionKey`、`(*Runner).gate`、`sessionGate.mu` / `.st`、`sessionState.baselineSuspect`、`Runner.spawn`（白盒）、`MetaEvent`、`MetaAboutAgent`、`eventTurnLost`、`OutcomeLost`、`OutcomeSpoke`、`OutcomeRefused`。
  - repl：`runREPLRunner`（`ask_test.go:31`）、`newInFlightRunner`（`session_inflight_test.go:152`）、`inFlightAgentID`（`"ag_wait"`）、`inFlightProbeDelay`、`newTestREPLConfig`、`New`、`(*REPL).SetConcise`、`(*REPL).Run`、`RulesContext`。repl 包的 `fakeStore` **不用**（本刀一律真库）。
- **HEAD 上的状态**：§2.1 引用本刀新增的 `SettleStaleInflight` / `StaleSettlement`，在 HEAD 上整包编译不过，是预期的红。**§2.2 只用 HEAD 已有的标识符**，测试腿交付前必须在 HEAD 的隔离拷贝里**只放 §2.2 这一个新文件**跑一次，报告里写出命令与每条用例的红绿。原型实测的 HEAD 结果（v1 在 `485581f`、v1.1 在 `4dbd17a` 上各跑一次，逐条相同）：R1 四个子测试红（`/session rebuild` 只打「已丢弃……」，之后 `/ask` 照样被拒，屏幕见 §0 第 2 条）；R3 红（HEAD 打「已丢弃」、provider 会话被删、没有失败那一行）；R4 红（HEAD 不读在飞记录，照样丢会话）；R7 红（HEAD 打「本来就没有开着的会话，下一轮照常是首轮」，之后被拒）；R8 红；**R2、R5、R6 绿**（它们是回归守卫：HEAD 的这三种行为本刀不许变）。

### 2.0 共用夹具（冻结）

- **陈旧的在飞记录**：`conversation.InflightTurn{TurnID: <id>, ConvID: <会话>, AgentID: <成员>, Provider: "claude", PlannedSessionID: <见各条>, ResumeSessionID: <见各条>, CursorSeq: 1, ScannedTo: <见各条>, StartedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Phase: <phase>, Tier: "read", WorkDir: "u43-wd"}`。
- **写入方式**：`phase.Valid()` 为真（`begun` / `spawned` / `returned`）走 `svc.BeginTurn`；为假走裸 SQL（`BeginTurn` 在事务前拒绝非法 phase，schema 没有 CHECK，所以只有裸写造得出来）：
  ```
  INSERT INTO inflight_turns (turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id, cursor_seq, scanned_to, started_at, phase, tier, work_dir) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  ```
  `started_at` 传 `StartedAt.UTC().Format(time.RFC3339Nano)`，`phase` 传 `string(phase)`。
- **phase 全集**：凡按 phase 参数化的用例，一律取 `{conversation.PhaseBegun, conversation.PhaseSpawned, conversation.PhaseReturned, conversation.InflightPhase("u43-bogus")}` 四个，子测试名取 `string(phase)`。第四个是 schema 放得进来、`classifyLost` 有 default 分支（`recover.go:75-79`）处置的非法值 —— 屏幕分支与 `settleOne` 都是「`== begun`」判据，非法值必须落到「记为丢失」那一边（§4 M20、M21 只有它杀得到）。
- **主键冲突夹具**（让结清写失败，fail closed 用）：先 `svc.RecordTurn(ctx, nil, conversation.TurnUsage{TurnID: <同一个 id>, ConvID: <会话>, AgentID: <成员>, Outcome: 7, CreatedAt: time.Now().UTC()}, nil, false)`，**再** `BeginTurn` 那条在飞记录（顺序冻结：反过来 `RecordTurn` 会清掉在飞记录）。结清时 `RecordUsage` 撞 `turn_usage.turn_id` 主键（1555），整笔回滚。这是 13a E1 `settle_fails` 的同一个夹具。

### 2.1 `internal/turn/u43_settle_stale_test.go`（真 store：`newRealTurnStore`）

共同前置（下文称「开口过的会话」）：`rs := newRealTurnStore(t)`、`cfg := newTestConfig()`、`r := newTestRunner(t, cfg, rs.svc)`、`fs := &fakeSpawner{}`、`r.spawn = fs.spawn`；一个开口过的会话 = `newConv` + `joinAgent(alice)` + `postUser("hello")` + `fs.enqueueSuccess(claudeT1JSON)` + `r.Ask` 成功（前提守卫 `Outcome == OutcomeSpoke`）+ `postUser("second")`。此时 `r.gate(key).st != nil` 且 `baselineSuspect == false`，库里 `provider_sessions` 有一行、`baseline_suspect == 0`。

**T1 `TestU43SettleStaleMatchesStartupPath`**（四个 phase 子测试）：同一个 `r` 上建两个开口过的会话 A、B；`psA` = A 的 `ProviderSession`；给 A 写 `u43-stale-a`、给 B 写 `u43-stale-b`（都是 alice，`PlannedSessionID: "u43-planned"`，`ResumeSessionID: psA.ProviderSessionID`，`ScannedTo: 2`，phase 取子测试参数）。记下 A、B 的 `History(ctx, conv, 0, 1000)` 条数。然后：
1. `got, err := r.SettleStaleInflight(ctx, A, alice)`，`err == nil`；
2. **启动对账那一侧作对照**：`r2 := newTestRunnerSameWorkDir(r, cfg, rs.svc)`；`lost, err := r2.AuditInflight(ctx)`，前提守卫 `err == nil`、`len(lost) == 1`、`lost[0].Turn.TurnID == "u43-stale-b"`；`marked, errs := r2.SettleLostTurns(ctx, lost)`，前提守卫 `len(errs) == 0`。
断言（每个子测试全部）：
- `got.Found == true`、`got.Lost.Turn.TurnID == "u43-stale-a"`、`got.Lost.Reason == lost[0].Reason`、`got.Settlement == conversation.LostSettlement{Existed: true, SuspectMarked: phase != PhaseBegun}`；`marked == 1` 当且仅当 `phase != PhaseBegun`（前提守卫）。
- `turn_usage`：经 `rs.rawDB` 读 `u43-stale-a` 与 `u43-stale-b` 两行的这 19 列 `agent_id, provider_session_id, seq, outcome, input_tokens, cache_create, cache_read, output_tokens, thinking_tokens, cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes, delivered_lo, delivered_hi, created_at, identity_fp, round_id`（可空列用 `sql.NullInt64` / `sql.NullFloat64`）：两边行数都等于 `wantN`（begun 为 0，其余为 1）；两行整体 `reflect.DeepEqual`；`wantN == 1` 时另断言绝对值：`outcome == 9`、`seq` 为 NULL、`input_tokens` 为 NULL、`cost_usd` 为 NULL、`provider_session_id == psA.ProviderSessionID`。
- 面包屑：A、B 的 `History` 条数各自恰好多 `wantN`；`wantN == 1` 时两边最后一条的 `Kind`、`SenderID`、`Text` 相等、`Meta` `reflect.DeepEqual`，且 A 那条 `Meta[MetaEvent] == eventTurnLost`。
- `provider_sessions.baseline_suspect`：A、B 都等于 `phase != begun ? 1 : 0`。
- `svc.InflightFor(A, alice)`、`(B, alice)` 都 `ok == false`、`err == nil`。
【判别力】本刀「同一条路」的核心担保：B 是真启动对账写出来的、A 是本刀写出来的，逐列比。另写一份结清（M4）时 spawned / returned / u43-bogus 的 `turn_usage` 行数 0 != 1；不经 `classifyLost` 或把非法 phase 当 begun（M21）时 u43-bogus 红。begun 子测试是对照：两边都 0 行、0 条面包屑、不标。

**T2 `TestU43SettleStaleOnlyTouchesExactKey`**：同一个 `r` 上两个开口过的会话 A、B；再 `joinAgent(A, nova)`。写三行 spawned（`PlannedSessionID: "u43-planned"`、`ResumeSessionID: ""`、`ScannedTo: 2`）：目标 `u43-target`（A, alice）、`u43-other-agent`（A, nova）、`u43-other-conv`（B, alice）。记下 A、B 的 History 条数。`r.SettleStaleInflight(ctx, A, alice)`：`err == nil`、`Found`、`Lost.Turn.TurnID == "u43-target"`。断言：
- 另两行 `InflightFor` 都 `ok`，`TurnID`、`Phase` 相等、`StartedAt.Equal`；
- `turn_usage` 里 `u43-other-agent`、`u43-other-conv` 各 0 行，`u43-target` 1 行；
- B 的 History 条数不变；A 的恰好 +1，最后一条 `Meta[MetaAboutAgent] == "alice"`；
- `baseline_suspect`：(B, alice) 仍 0，(A, alice) 为 1。
【判别力】M10（顺手结清全表）、M10b（结清同会话别的成员）、M10c（结清同成员别的会话）各自只让对应那一行的断言红（原型实测三种都被杀）；(A, alice) 那两条是正对照。

**T3 `TestU43SettleStaleRefusesWhileGateHeld`**（白盒）：一个开口过的会话；写 `u43-stale`（alice，spawned，`ResumeSessionID: ""`）；记下 History 条数。`g := r.gate(sessionKey{convID, alice})`；`g.mu.Lock()`；`got, err := r.SettleStaleInflight(...)`；`g.mu.Unlock()`。断言：`errors.Is(err, ErrTurnInFlight)`、`got == StaleSettlement{}`；在飞记录仍在；`turn_usage` 里 `u43-stale` 0 行；History 条数不变。**对照**（同一个用例里接着）：再调一次 `SettleStaleInflight`，`err == nil` 且 `Found`。
【判别力】M1（不取闸）红；对照证明被拒不是因为这一行本身不可结清。M2（`Lock()` 阻塞等待）会让本用例死锁，由 `go test -timeout` 杀（§4 写明集成员怎么跑）。X2（拿到闸立刻放）杀不到本条：闸被测试占着，`TryLock` 先失败，X2 那一行走不到；X2 由 T4b 担保。

**T4 `TestU43SettleStaleRefusesDuringLiveTurn`**（真的一轮在飞，确定性、不睡）：一个开口过的会话；本文件定义一个阻塞 Spawner（类型名 `u43...`）：被调用时 `close(started)`，然后 `<-release`，再回放 `claudeT1JSON`（退出码 0、未被杀、无错）；`r.spawn` 换成它。起 goroutine 跑 `r.Ask(ctx, conv, alice)`，结果进 channel。`<-started` 之后：前提守卫 `svc.InflightFor(conv, alice)` `ok` 且 `Phase == PhaseSpawned`（在飞那一轮自己的行，`SetInflightPhase(spawned)` 写在 `r.spawn` 之前，13 §3.3a）；记下它的 `TurnID` 为 `live`。调 `got, err := r.SettleStaleInflight(ctx, conv, alice)`；然后 `close(release)`、收 Ask 的结果。断言：
- `errors.Is(err, ErrTurnInFlight)`、`got.Found == false`；
- 那一轮 `err == nil`、`Outcome == OutcomeSpoke`；
- `turn_usage` 里 `turn_id = live` 恰好 1 行、`outcome == 1`；
- `SELECT COUNT(*) FROM turn_usage WHERE outcome = 9` 为 0。
【判别力】这是不变量 (a) 的「可观测的灾难」。原型实测 M1（不取闸）下：本方法读到正在跑那一轮自己的 spawned 行，把它结清成 lost（`Found == true`、`err == nil`；`turn_usage` 以 `live` 为 turn_id 写下 `outcome == 9` 的一行，在飞记录被删）；那一轮随后 `RecordTurn` 时 `RecordUsage` 撞 `turn_usage.turn_id` 主键（1555），Ask 返回 `OutcomePostRejected`（2），错误原文 `post agent message: record turn: record usage <live>: constraint failed: UNIQUE constraint failed: turn_usage.turn_id (1555)` —— `live` 那一行是 `outcome == 9` 不是 1，没有 spoke 行，本用例四条断言全红。**不是** `ErrInflightMissing` 回滚（v1 这里写错了，W1 V0 第 4(a) 点）：`RecordUsage` 在 `ClearInflight` 之前（`conversation/service.go:406`、`:423`），先撞主键。被结清的若是 begun 行（`BeginTurn` 与 `SetInflightPhase(spawned)` 之间），读代码走的才是 `ErrInflightMissing`（`store/tx.go:464-466` 的 `SetInflightPhase`，再到 `refuse` 记账时 `service.go:427-428`）；本原型没有造这个时序，未实测。X2（拿到闸立刻放）杀不到本条：闸被在飞那一轮占着，`TryLock` 先失败；X2 由 T4b 担保。时序靠 channel，不靠睡眠。

**T4b `TestU43SettleStaleHoldsGateAcrossRead`**（v1.1 新增；白盒，确定性：不睡、不起 goroutine；照 `s10a_inflight_gate_test.go` 的 G6 `TestS10aInflightCheckRunsInsideGate` 与 `s10aProbeStore` 的体例）：包级探针类型 `u43...`（指针接收者），内嵌 `*store.Store`，字段 `gate *sessionGate`、`armed bool`、`calls int`、`sawGateFree bool`，**只重写 `InflightFor`**：`armed` 为真时先把 `armed` 置假（一次性）、`calls++`，再对 `gate` 做 `TryLock()`，成功就立刻 `Unlock()` 并置 `sawGateFree = true`；然后（无论武装与否）透传 `s.Store.InflightFor`。装置：`rs := newRealTurnStore(t)`、`probe := &探针{Store: rs.st}`、`svc := conversation.NewService(probe)`、`r := newTestRunner(t, cfg, svc)`、`fs := &fakeSpawner{}`、`r.spawn = fs.spawn`；「开口过的会话」**建在这个 `svc` 与这个 `r` 上**（开口那一轮 Ask 也调 `InflightFor`，此时没武装，照常透传）。不写任何在飞记录。`probe.gate = r.gate(sessionKey{conv, alice})`（在用例 goroutine 里取；探针自己不调 `r.gate`）；`probe.armed = true`；`got, err := r.SettleStaleInflight(ctx, conv, alice)`。前提守卫：`calls == 1`；`err == nil` 且 `got.Found == false`。断言：`sawGateFree == false`。**正对照**（同一个用例里、上面那条断言之后接着）：再 `probe.armed = true`，在闸外直接调 `svc.InflightFor(ctx, conv, alice)`（前提守卫 `err == nil`），断言 `calls == 2 && sawGateFree == true`。
【判别力】不变量 (a) 的另一半：闸必须**跨过读**一直持有，不只是「拿得到」。整段调用在用例 goroutine 里同步执行，Go 的 Mutex 不可重入，好实现下 `InflightFor` 被调用时闸由本方法持有，探针的 `TryLock` 必然失败，不靠任何时序。X2（`defer g.mu.Unlock()` -> `g.mu.Unlock()`）、X2c（`InflightFor` 那一行挪到 `TryLock` 之前，v1.2 登记）与 M1（不取闸）下探针拿得到闸，`sawGateFree` 为真而红（原型实测三者都红在这一条断言上，前提守卫都过）。v1 的全部用例杀不到 X2（原型实测：X2 下 T1-T8、R1-R8 全活）：T3、T4、R2 的闸从头到尾被别人占着，`TryLock` 先失败，X2 那一行走不到；其余用例单线程，闸在不在手结果相同。X2c 同理：闸被占着时，读在 `TryLock` 失败之前已经做完，读没有副作用；闸空闲时，单线程里先读再拿闸，结果相同（v1.2 原型实测 X2c 下 T3、T4、T5、R2、R3 绿，只有本条红；其余用例未逐条实测，按同一论证看不见）。正对照（同一个探针、同一个 key，只差「在不在本方法里读」）证明探针真能看见空闲的闸，不会因为装错而空绿。前提守卫「恰好 1 次」挡住「读被删掉、探针没跑」的空绿；探针一次性，计数最多是 1，所以这条守卫只抓得到 0 次，这就够了。v1.1 另有一条前提守卫「开口之后 `calls == 0 && !sawGateFree`」，它不可能为假（探针武装之前不计数），v1.2 删去。**不收**审查员的可观测版本（探针里起一个同 key 的 Ask、等 `started` 或 300 ms 取先）：好实现下它只能靠超时收场，每次白等 300 ms，违反本节「不靠睡眠」。T4 担保的是 M1 下的同一种灾难。X2 下的灾难（放闸之后插进来一轮，它的行被当成陈旧行结清）没有可观测的用例，X2c 的后果（§4 X2c 那一行）也没有：原型实测 X2 下 T4 绿。这两者都只由本条从结构上担保（闸跨过读）。

**T5 `TestU43SettleStaleClearsMemorySnapshot`**（两个子测试 `spawned` / `begun`，白盒）：一个开口过的会话；前提守卫 `r.gate(key).st != nil && !r.gate(key).st.baselineSuspect`；写 `u43-stale`（alice，子测试的 phase，`ResumeSessionID: ""`）；`r.SettleStaleInflight` 成功。断言：`r.gate(key).st == nil`；`info, ok := r.SessionInfo(ctx, conv, alice)`，`ok == true`、`info.BaselineSuspect` 在 spawned 下为 `true`、在 begun 下为 `false`。
【判别力】不置 nil（M5）时两个子测试的第一条都红，spawned 另因 `SessionInfo` 走内存分支读到旧的 `false` 而红 —— 那就是「库里已标可疑、下一轮却自信地算出金额」。begun 是对照：库里没标，读库也是 `false`。

**T6 `TestU43SettleStaleNoRowIsNoop`**：一个开口过的会话，不写任何在飞记录；`st0 := r.gate(key).st`；记下 History 条数。`got, err := r.SettleStaleInflight(...)`：`err == nil`、`got == StaleSettlement{}`；`r.gate(key).st == st0`（指针相等）；History 条数不变。
【判别力】M7（没查到也清内存快照）红。对照是 T5（只差「有没有那一行」）。

**T7 `TestU43SettleStaleReadErrorTouchesNothing`**：一个开口过的会话；写 `u43-stale`（alice，spawned）。包级包装类型 `u43...`，内嵌 `*store.Store`，只重写 `InflightFor`，恒返回包级错误变量 `u43...`（`errors.New("u43 injected read failure")`）。`bad := newTestRunnerSameWorkDir(r, cfg, conversation.NewService(包装{rs.st}))`；`got, err := bad.SettleStaleInflight(ctx, conv, alice)`。断言：`errors.Is(err, 注入的错误)`、`got == StaleSettlement{}`；经 `rs.svc.InflightFor` 读，那一行仍在；`turn_usage` 里 `u43-stale` 0 行。
【判别力】M3（读失败当「没有」）下 `err == nil` 红。

**T8 `TestU43SettleStaleWriteFailureFailsClosed`**：一个开口过的会话；主键冲突夹具（id `u43-stale`，alice，spawned，`ResumeSessionID: ""`）；记下 `st0 := r.gate(key).st`、`ps0`（`ProviderSession`）、History 条数。`got, err := r.SettleStaleInflight(ctx, conv, alice)`。断言：
- `err != nil`、`got.Found == true`、`got.Lost.Turn.TurnID == "u43-stale"`、`got.Settlement == conversation.LostSettlement{}`；
- `err.Error()` 整串等于 `"settle stale inflight " + conv + "/alice: settle lost turn u43-stale: settle lost turn u43-stale: record usage u43-stale: constraint failed: UNIQUE constraint failed: turn_usage.turn_id (1555)"`（原型实测原文）；
- 在飞记录仍在；`turn_usage` 里 `u43-stale` 恰好 1 行且 `outcome == 7`（种下的那行）；History 条数不变；`ProviderSession` 与 `ps0` 结构体 `==`；`r.gate(key).st == st0`；
- 之后 `fs.enqueueSuccess(claudeT1JSON)`、记下 `len(fs.calls)`，`rec, err := r.Ask(...)`：`errors.Is(err, conversation.ErrInflightConflict)`、`rec.Outcome == OutcomeRefused`、`len(fs.calls)` 不变。
【判别力】M6（结清之前就清内存）红在 `st0`；M8（失败时 `Found` 为假）、M9（失败被吞成成功）红在前两条。最后一段证明「fail closed 之后成员照样被锁」，与 §0 第 4 条的照实写法对上。

### 2.2 `internal/repl/u43_rebuild_settle_test.go`（真 store + 真 Runner + 假 CLI；只用 HEAD 已有标识符）

共同前置（`u43` 前缀的 helper，名字测试腿定）：`st := store.Open(ctx, filepath.Join(t.TempDir(), "u43-data"))`（`t.Cleanup` 关）、`svc := conversation.NewService(st)`、`conv := svc.Create(ctx, "u43")`、`svc.Join(ctx, conv.ID, conversation.Participant{AgentID: "ag_wait", Provider: "claude", JoinedAt: time.Now().UTC()})`、`rn := newInFlightRunner(t, svc)`；`post(text)` = `svc.Post` 一条 `SenderID: ReservedSenderUser`、`Kind: KindUser` 的消息；查库另开 `sql.Open("sqlite", "file:"+filepath.ToSlash(st.Path())+"?_pragma=busy_timeout(5000)")`。「开口过」= `post("seed-msg")` + `rn.Ask(ctx, conv.ID, "ag_wait")` 成功（前提守卫 `Outcome == turn.OutcomeSpoke`）。陈旧行的 id 一律 `u43-stale-turn`、`PlannedSessionID: ""`、`ScannedTo: 1`。常量 `u43...` 逐字复制 §3。
（REPL 用的是 `runREPLRunner` 内部的 `newTestREPLConfig()`，其中没有 `ag_wait`；`/ask ag_wait` 能走到 Runner 是因为它是会话成员 —— `cmdAsk` 在「成员 ∪ 配置」上判精确，`ask.go:91-99`；`/session rebuild` 只看成员与 Runner 的漂移审计。与 13a R2 同体例。）

**R1 `TestU43RebuildSettlesStaleThenAskSpawns`**（phase 全集四个子测试）：开口过；`post("second-msg")`；写陈旧行（phase 取参数，`ResumeSessionID: "2a90d449-a348-497a-8812-e73e74d6898e"`，即假 CLI 回放的会话 id）。`out := runREPLRunner(t, svc, rn, "/use "+conv+"\n/ask ag_wait\n/session rebuild ag_wait\n/ask ag_wait\n")`，`err == nil`。断言（切行规则见通用纪律）：
1. 存在下标 i：第 i 行 == `outcome refused  agent ag_wait`、第 i+1 行 == §3.2 那句拒绝（A 之五原句）；
2. 存在下标 j > i：`lines[j:j+3]` 整体等于 `[§3.1 L1 填入 (ag_wait, u43-stale-turn, <phase>), <phase == begun ? §3.1 L3 : §3.1 L2>, §3.2 既有「已丢弃 ag_wait ……下一轮按首轮计价」]`；
3. 在 `lines[j:]` 里存在整行 `outcome spoke  agent ag_wait`（解开之后真起了进程）；
4. `svc.InflightFor(conv, "ag_wait")` `ok == false`；
5. `SELECT COUNT(*) FROM turn_usage WHERE turn_id = 'u43-stale-turn' AND outcome = 9 AND seq IS NULL AND input_tokens IS NULL AND cost_usd IS NULL AND provider_session_id = '2a90d449-a348-497a-8812-e73e74d6898e' AND created_at = '2026-09-01T00:00:00Z'` 等于 `phase == begun ? 0 : 1`（`created_at` 是 `StartedAt` 的 RFC3339Nano，原型实测）；
6. `SELECT COUNT(*) FROM messages WHERE conv_id = ? AND kind = 'system' AND json_extract(meta, '$.event') = 'turn_lost'` 等于同上（原型实测 modernc 支持这一句）；
7. `SELECT COUNT(*) FROM turn_usage WHERE agent_id = 'ag_wait' AND outcome = 1` 等于 2（首轮 + 解开之后那一轮）。
【判别力】这是任务点名的那条全链路：种陈旧行 -> `/ask` 被拒 -> `/session rebuild` 结清 -> 库里多了与启动对账同形的那一行（同形由 §2.1 T1 逐列担保，这里断绝对值）-> 下一次 `/ask` 起进程（假 CLI）。HEAD 上四个子测试都红在第 2 条（§0 第 2 条的屏幕）。begun 与其余三个互为对照（第二行与第 5、6 条）；u43-bogus 专杀 M20。

**R2 `TestU43RebuildRejectedWhileTurnInFlight`**：开口过；`post("second-msg")`；照 `session_inflight_test.go:238-247` 的体例另起 goroutine 跑 `rn.Ask(ctx, conv, "ag_wait")`（第二轮带 `--resume`，假 CLI 睡 700ms），`sync.WaitGroup` + `t.Cleanup(wg.Wait)`；`time.Sleep(inFlightProbeDelay)`；`out := runREPLRunner(..., "/use "+conv+"\n/session rebuild ag_wait\n")`；然后 `wg.Wait()`，前提守卫第二轮 `err == nil`。断言：
- 整段等于 `[§3.2 既有「ag_wait 此刻正有一轮在跑，什么都没丢；等这一轮跑完再敲一次」]`；
- `SELECT COUNT(*) FROM turn_usage WHERE outcome = 9` 为 0；`WHERE outcome = 1` 为 2；`SELECT COUNT(*) FROM inflight_turns` 为 0；`ProviderSession(conv, "ag_wait")` `found == true`。
【判别力】不变量 (a) 在 REPL 层的担保。M1（不取闸）下本方法结清正在跑那一轮自己的行（原型实测）：整段是「已结清 ag_wait 的一轮在飞记录（turn <那一轮的 id>，phase spawned）……」、L2、再接闸被占那一句（`Rebuild` 仍被在飞那一轮挡住）；那一轮随后 `RecordUsage` 撞 `turn_usage` 主键（1555）失败，本用例**红在前提守卫**「第二轮 `err == nil`」（错误原文 `post agent message: record turn: record usage <id>: constraint failed: UNIQUE constraint failed: turn_usage.turn_id (1555)`），此时库里 lost 行 1、spoke 行 1（只剩首轮）、在飞记录 0；M2（`Lock()` 等待）下等到那一轮跑完、查不到行、接着 `Rebuild` 丢会话：整段变成「已丢弃……」、`found == false`；M13（去掉这一支）下落到读失败那一句。HEAD 上绿（HEAD 的 `Rebuild` 同样拒绝）—— 回归守卫。时序沿用既有 700ms / 350ms，不另定常数。

**R3 `TestU43RebuildSettleFailureFailsClosed`**：开口过；`post("second-msg")`；主键冲突夹具（`u43-stale-turn`，spawned，`ResumeSessionID: "2a90d449-a348-497a-8812-e73e74d6898e"`）；`ps0 := ProviderSession(conv, "ag_wait")`。`out := runREPLRunner(..., "/use "+conv+"\n/session rebuild ag_wait\n/ask ag_wait\n")`。断言：
- 整段等于 `[L4, "正在问 ag_wait ...", "outcome refused  agent ag_wait", §3.2 拒绝那句]`，其中 L4 = §3.1 L4 填入 `(ag_wait, u43-stale-turn)` 再接 `"settle stale inflight " + conv + "/ag_wait: settle lost turn u43-stale-turn: settle lost turn u43-stale-turn: record usage u43-stale-turn: constraint failed: UNIQUE constraint failed: turn_usage.turn_id (1555)"`（原型实测原文）；
- `ProviderSession` `found` 且与 `ps0` 结构体 `==`；`InflightFor` `ok == true`。
【判别力】fail closed 的全链路。M9（失败吞成成功）、M12（失败了还往下走）下整段多出「已结清」「已丢弃」、provider 会话被删；M11（先 Rebuild 再结清）下 provider 会话被删；M8、M17 下打成读失败那一句。HEAD 上红（HEAD 打「已丢弃」并删掉 provider 会话，接着的 `/ask` 照样被拒 —— §0 第 2 条「HEAD 上会话被丢了、成员却仍被锁」）。

**R4 `TestU43RebuildReadFailureTouchesNothing`**：开口过；`ps0`。包级包装类型 `u43...` 内嵌 `*store.Store`、只重写 `InflightFor` 恒返回 `errors.New("u43 injected read failure")`（包级变量）；`badSvc := conversation.NewService(包装{st})`、`badRn := newInFlightRunner(t, badSvc)`；`out := runREPLRunner(t, badSvc, badRn, "/use "+conv+"\n/session rebuild ag_wait\n")`。断言：整段等于 `[§3.1 L5 填入 (ag_wait) 再接 "settle stale inflight " + conv + "/ag_wait: read: u43 injected read failure"]`；经真 `svc` 读 `ProviderSession`，`found` 且与 `ps0` 相等。
【判别力】M3 下读失败被当成「没有」，往下 `Rebuild` 丢会话：整段变「已丢弃」、`found == false`。HEAD 上红。

**R5 `TestU43RebuildWithoutStaleRowUnchanged`**：开口过（不写在飞记录）；记下 `SELECT COUNT(*) FROM messages`。`/use` + `/session rebuild ag_wait`：整段等于 `[§3.2「已丢弃 ag_wait ……下一轮按首轮计价」]`；`messages` 行数不变。
【判别力】「没有在飞记录时与本刀之前逐字节相同」的正面担保；R1 是它只差「有没有那一行」的对照。HEAD 上绿。M11（先 Rebuild）下第二次 Rebuild 打「本来就没有开着的会话」，本用例红（原型实测）。

**R6 `TestU43RebuildNonMemberDoesNotSettle`**：`store.Open` + `Create("u43")`，**不 Join**；写陈旧行（`ag_wait`，spawned，`ResumeSessionID: ""`，`BeginTurn` 不要求成员）；`rn := newInFlightRunner`；`/use` + `/session rebuild ag_wait`。断言：整段等于 `[「ag_wait 不在当前会话的活跃成员里，用 /who 看看有哪些」]`（既有 `ask.go:288`）；`InflightFor` 仍 `ok`。
【判别力】M18（结清挪到成员闸之前）下这一行被结清、整段多出两行。HEAD 上绿。R1 是对照（只差是不是成员）。

**R7 `TestU43RebuildSettlesBootstrapCrash`**：`post("seed-msg")`（**不**先开口，库里没有 `provider_sessions`）；写陈旧行（spawned，`ResumeSessionID: ""`）；`/use` + `/session rebuild ag_wait` + `/ask ag_wait`。断言：存在 j 使 `lines[j:j+3]` 等于 `[L1 填入 (ag_wait, u43-stale-turn, spawned), L2, 既有「本来就没有开着的会话，下一轮照常是首轮」]`；`lines[j:]` 里有整行 `outcome spoke  agent ag_wait`；`SELECT COUNT(*) FROM turn_usage WHERE turn_id = 'u43-stale-turn' AND outcome = 9 AND provider_session_id = ''` 为 1。
【判别力】13 §3.6「崩在 bootstrap 轮的钱必须记得上」在本入口同样成立（没有 `provider_sessions` 行可标，`turn_usage` 照补）；第三行证明 (d) 的「下一轮会怎样」在这一支取的是既有的「本来就是首轮」那一句。HEAD 上红。

**R8 `TestU43RebuildSettleLinesSameInConcise`**：开口过；写陈旧行（spawned，`ResumeSessionID: "2a90d449-..."`）；**自己**造 REPL：`r := New(newTestREPLConfig(), svc, rn, strings.NewReader("/use "+conv+"\n/session rebuild ag_wait\n"), &buf, RulesContext{})`、`r.SetConcise(true)`、`r.Run(ctx)` 返回 nil。断言：整段等于 `[L1 (ag_wait, u43-stale-turn, spawned), L2, 既有「已丢弃……下一轮按首轮计价」]`。
【判别力】默认档是简洁（#45，`app.go` 设 `SetConcise(!opts.Verbose)`），而 R1-R7 走的 `runREPLRunner` 是详细档；M19（简洁档不打结清那几行）只有本用例杀得到。

---

## 3. 冻结的字面量（逐字节）

### 3.1 本刀新增的用户可见字面量（`ask.go` `cmdSessionRebuild`，格式串原文）

```
L1: "已结清 %s 的一轮在飞记录（turn %s，phase %s），对它 /ask 不会再因为这条记录被拒\n"
L2: "那一轮记为丢失：可能花了钱，用量与金额都不知道（与重启时启动对账的记法相同）\n"
L3: "那一轮没起过进程、没花钱，只删掉了这条记录\n"
L4: "结清 %s 的在飞记录（turn %s）失败，什么都没改，provider 会话也没丢；对它 /ask 仍会被拒: %v\n"
L5: "读不出 %s 的在飞记录，什么都没改，provider 会话也没丢: %v\n"
```

- L1 的三个占位依次是 agent id、在飞记录的 turn id、`string(phase)`；L4 是 agent id、turn id、错误值；L5 是 agent id、错误值。
- 标点：全角逗号 U+FF0C、全角括号 U+FF08 / U+FF09、全角冒号 U+FF1A（L2 的「丢失：」）、顿号 U+3001（L3）、全角分号 U+FF1B（L4）；L4、L5 在 `%v` 之前是 **ASCII 冒号 + 一个 ASCII 空格** `": "`（与 `reportErr` 的 `"%s: %v\n"` 同体例）。`/ask`、`turn`、`phase`、`provider` 前后各一个 ASCII 空格。无句号。
- 上屏时的错误文本（经 L4 / L5 的 `%v`）由 §1.1 A 的三个包装冻结：`"settle stale inflight %s/%s: %w"`、`"settle stale inflight %s/%s: read: %w"`，后面接的是既有错误链原文（`settleOne` 的 `"settle lost turn %s: %w"`、`Service.SettleLostTurn` 的 `"settle lost turn %s: %w"`、store 的 `record usage %s: ...`，都不是本刀新增）。

### 3.2 本刀复用、测试要比对的既有文本（不是新增）

- 闸被占：`"%s 此刻正有一轮在跑，什么都没丢；等这一轮跑完再敲一次\n"`（`ask.go:298`，本刀在 B2 多一个调用点，格式串逐字相同）。
- `Rebuild` 之后的三句（`ask.go:303`、`:306`、`:308`，一个字不动）：`"本来就没有开着的会话，下一轮照常是首轮\n"`、`"已清掉 %s 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢）；按当前配置对它 /ask 会被拒，不花钱\n"`、`"已丢弃 %s 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢），下一轮按首轮计价\n"`。它们就是不变量 (d) 的「下一轮会怎样」。
- 非成员：`"%s 不在当前会话的活跃成员里，用 /who 看看有哪些\n"`（`ask.go:288`）。
- A 之五拒绝那句（`ask.go:620`，一个字不动）：
  ```
  这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行
  ```
- `outcome refused  agent %s`、`outcome spoke  agent %s`（`ask.go:380`，两个空格）；`正在问 %s ...`（`ask.go:164`）；`/use` 的 `当前会话: %s，标题: %s`；REPL 退出的 `再见`。
- 常量：`OutcomeLost = 9`、`OutcomeSpoke = 1`、`OutcomeRefused = 7`；`turn_usage.turn_id` 主键冲突扩展码 1555（原型实测原文见 §2.1 T8）。

### 3.3 本刀新增的导出 API（登记进 `docs/09a_STEP6_API_CONTRACT.md`，§6）

```go
type StaleSettlement struct {
	Found      bool
	Lost       LostTurn
	Settlement conversation.LostSettlement
}

func (r *Runner) SettleStaleInflight(ctx context.Context, convID, agentID string) (StaleSettlement, error)
```

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；全部在原型上实测（「原型实测」一律指这个）。主代理落地后逐条跑，**从字节副本还原，不用 `git checkout`**；跑完核对两个生产文件的 SHA256 与跑前相同。运行方式：`go test -count=1 -timeout 150s -run TestU43 ./internal/turn ./internal/repl`；**M2 只跑 `./internal/repl`**（它让 T3、T4 在同一 goroutine 上死锁，只能靠超时杀，拖慢整批；T4b 在 M2 下不死锁也不红：`Lock()` 拿到的闸同样跨过读。这一句是原型里单跑 T4b 的实测，按本节的跑法 M2 不跑 turn 包、复现不出；v1.2 单跑复核过）。锚点均取落地后的真文件，恰好一次。v1.1 在 `4dbd17a` 的原型上把下表当时的 24 条（M1-M21、M10b、M10c、X2）全部重跑过；v1.2 加 X2c 一条，共 25 条，X2c 在 v1.2 原型上实测。

| # | 变异（可编译；「A -> B」是整段替换） | 预期被谁杀（原型实测） |
|---|---|---|
| M1 | settle_stale.go：`if !g.mu.TryLock() {` ...`}` + `defer g.mu.Unlock()` 四行 -> `if false {` ...`}`（删掉 defer，否则解锁未上锁的 mutex 会 panic） | T3、T4、T4b、R2（R2 红在前提守卫，机制见 R2【判别力】） |
| X2 | settle_stale.go：`defer g.mu.Unlock()` -> `g.mu.Unlock()`（拿到闸立刻放，再读、再结清；v1.1 新增，W1 F0） | T4b（原型实测：全部 `TestU43` 里只有它红） |
| X2c | settle_stale.go：删掉 `row, found, err := r.svc.InflightFor(ctx, convID, agentID)` 那一行，插到 `g := r.gate(sessionKey{convID: convID, agentID: agentID})` 那一行之后、`if !g.mu.TryLock() {` 之前（先读再拿闸；两处锚点各恰好一处；v1.2 新增，W1b finding 2）。后果：读到的若是正在跑那一轮的行，而那一轮在 `TryLock` 之前跑完，就去结清一条已经跑完的轮次。spawned 行撞 `turn_usage` 主键、fail closed，但屏幕说「对它 /ask 仍会被拒」是假话（原型探针实测 spawned 这一支）；begun 行则被报成「没起过进程、没花钱」（读代码，未实测） | T4b（原型实测：重建的 T3、T4、T4b、T5、R2、R3 里只有它红，红在 `sawGateFree` 那条断言） |
| M2 | settle_stale.go：`if !g.mu.TryLock() {` -> `if g.mu.Lock(); false {` | R2（T3、T4 死锁，超时） |
| M3 | settle_stale.go：读失败那一支 `if err != nil {` -> `if false {` | T7、R4 |
| M4 | settle_stale.go：`got, err := r.settleOne(ctx, lost)` -> `got, err := r.svc.SettleLostTurn(ctx, row.TurnID, nil, nil, nil)`（裸删） | T1（spawned、returned、u43-bogus）、T2、T5 spawned、T8、R1（spawned、returned、u43-bogus）、R3、R7 |
| M5 | settle_stale.go：删掉 `g.st = nil` | T5 两个子测试 |
| M6 | settle_stale.go：`lost := classifyLost(row)` -> 其后加一行 `g.st = nil`（结清之前就清） | T8 |
| M7 | settle_stale.go：`if !found {` 里 `return` 之前加 `g.st = nil` | T6 |
| M8 | settle_stale.go：结清失败的返回 `StaleSettlement{Found: true, Lost: lost}` -> `StaleSettlement{Lost: lost}` | T8、R3 |
| M9 | settle_stale.go：结清失败的 `return StaleSettlement{Found: true, Lost: lost}, fmt.Errorf("settle stale inflight %s/%s: %w", convID, agentID, err)` -> `return StaleSettlement{Found: true, Lost: lost}, nil` | T8、R3 |
| M10 | settle_stale.go：`got, err := r.settleOne(ctx, lost)` 之前插入：`all, _ := r.svc.AllInflight(ctx)` / `for _, a := range all {` / `if a.TurnID != row.TurnID {` / `_, _ = r.settleOne(ctx, classifyLost(a))` / `}` / `}` | T1（四个）、T2 |
| M10b | 同 M10，条件改 `a.TurnID != row.TurnID && a.ConvID == convID`（结清同会话别的成员） | T2 |
| M10c | 同 M10，条件改 `a.TurnID != row.TurnID && a.AgentID == agentID`（结清同成员别的会话） | T1（四个）、T2 |
| M11 | ask.go：`stale, err := r.rn.SettleStaleInflight(ctx, r.cur.ID, agentID)` 之前插一行 `_, _ = r.rn.Rebuild(ctx, r.cur.ID, agentID)`（先丢会话再结清） | R1（四个）、R3、R4、R5、R8 |
| M12 | ask.go：`stale, err := ...` 之后的 `if err != nil {` -> `if err != nil && errors.Is(err, turn.ErrTurnInFlight) {`（失败了照样往下走到 Rebuild） | R3、R4 |
| M13 | ask.go：B2 里的 `if errors.Is(err, turn.ErrTurnInFlight) {` -> `if false {` | R2 |
| M14 | ask.go：`if stale.Lost.Turn.Phase == conversation.PhaseBegun {` -> `!=` | R1（四个）、R7、R8 |
| M15 | ask.go：同一行 -> `if false && stale.Lost.Turn.Phase == conversation.PhaseBegun {` | R1 begun |
| M16 | ask.go：`if stale.Found {`（打印那一块）-> `if false {` | R1（四个）、R7、R8 |
| M17 | ask.go：错误那一块里的 `if stale.Found {` -> `if !stale.Found {` | R3、R4 |
| M18 | ask.go：把 B2 插入的整段（从 `// 先结清陈旧的在飞记录` 到打印块的 `}`）挪到 `active, err := r.svc.Participants(ctx, r.cur.ID, false)` 之前 | R6 |
| M19 | ask.go：`if stale.Found {`（打印那一块）-> `if stale.Found && !r.concise {` | R8 |
| M20 | ask.go：`if stale.Lost.Turn.Phase == conversation.PhaseBegun {` -> `if stale.Lost.Turn.Phase != conversation.PhaseSpawned && stale.Lost.Turn.Phase != conversation.PhaseReturned {` | R1 u43-bogus |
| M21 | settle_stale.go：`lost := classifyLost(row)` 之后插入 `if !lost.Turn.Phase.Valid() {` / `lost.Turn.Phase = conversation.PhaseBegun` / `}` | T1 u43-bogus、R1 u43-bogus |

**判定为等价、不列杀手的变异**（写在这里，免得被当成漏测）：
- `defer g.mu.Unlock()` 改成在每个 `return` 之前显式 `Unlock`，**且成功那一支的 `Unlock` 在 `g.st = nil` 之后**：可观测行为相同（v1.2 原型实测这种写法下全绿）。`Unlock` 在 `g.st = nil` 之前的那一种不等价，那是 X2d，见下面「只在审查时看」。
- 成功那一支里 `g.st = nil` 与构造返回值的先后：相同。
- B2 里打印结清行用 `r.printf` 还是 `r.write(fmt.Sprintf(...))`：字节相同。
- `SettleStaleInflight` 里 `if !found` 与读失败两支的先后对调：两支互斥，结果相同。

**只在审查时看（没有用例专门抓，集成员收割时逐字核对）**：
- `SettleStaleInflight` 读行用的是 `InflightFor`，不是 `AllInflight` 过滤出同 key 的那一行（后者行为相同、M10 系杀不到，但违反「全表读不导出给 REPL」，13 §3.5）。原型实测这个变异（X4）会被 T7、R4 顺带杀掉：它们的包装只重写了 `InflightFor`，换成 `AllInflight` 就绕过了注入的读失败；T4b 也红，红在前提守卫「探针恰好被调 1 次」（探针同样只挂在 `InflightFor` 上）。这些都是夹具的副作用，不是为它设计的，仍要逐字核对。
- 结清用调用方 `ctx`，不是 `context.Background()`：只在「敲命令的同时取消」时有差别，而那时两种写法都以「结清失败、什么都没改」或「结清成功」收场，屏幕可能不同但都不说假话。原型实测这个变异（X3）全部用例活。
- **「读完之后、`settleOne` 之前放闸」**（X2b：把 `defer g.mu.Unlock()` 换成紧跟在 `InflightFor` 那一行之后的 `g.mu.Unlock()`；v1.1 登记，W1 V0 第 4(d) 点；v1.2 按 W1b finding 1 改写后果）：读仍在闸内，所以读到的行照旧不可能是正在跑那一轮的。插进来的一轮若在结清提交之前做 A 之五的检查，会看到那条陈旧行而被拒（读代码，未实测）。结清与成功之后的 `g.st = nil` 都落在闸外。**后果不只是数据竞争**，设想一个在放闸之后拿到闸的并发 Ask，同时满足两个条件：它的 A 之五检查落在结清提交之后（落在之前会被拒）；它读 `g.st`（`runner.go:447`）又早于结清方的 `g.st = nil`。这时它拿着结清前的内存快照走内存分支（`runner.go:447-456` 只比指纹、不回库读），把库里刚标成可疑的基线当成可信，自信地算出一个金额。这正是 T5 / M5 要防的「库里已标可疑、下一轮却自信地算出金额」。v1.2 原型白盒模拟了这个交错：开口过的会话，结清前记下 `st0 := g.st`，写一行 spawned 陈旧行，照好实现结清（库里 `baseline_suspect == 1`），把 `g.st` 放回 `st0`，再问一轮 `claudeT1JSON`，结果 `rec.BaselineSuspect == false`、那一轮 `turn_usage.cost_usd == 0`（非 NULL）；对照（不放回）是 `rec.BaselineSuspect == true`、`cost_usd` 为 NULL。**这是白盒模拟，没有复现真并发**：窗口只有结清方提交之后到 `g.st = nil` 之间的一条语句，这个交错没有造出来过。它同时仍是数据竞争（两边读写同一个 `g.st`），本机没有 gcc、不能 `-race`。原型实测 X2b 下契约用例全活（v1.1 实测全部；v1.2 重建的 T3、T4、T4b、T5、R2、R3 也活）：T4b 只在读的那一刻探闸，而读仍在闸内。v1.1 修订时另试过一个**不进本契约**的探针，把 T4b 的同款一次性探针挂在 `ProviderSession` 上：`settleOne` 在事务之外读回它；写一行 spawned 陈旧行、武装之后结清。v1.2 原型复测：好实现绿，X2b、X2、M1 都红。它能把本条升成确定性用例（T4c 候选，完整写法见 §7.2 第 6 条）。裁决把本条定为只在审查时看，本版照裁决不收，待主代理。核对办法：`settle_stale.go` 与 §1.1 A 逐字相同，`defer g.mu.Unlock()` 紧跟在 `TryLock` 那一支之后、整个函数只有这一处 `Unlock`。
- **「成功那一支先放闸、再 `g.st = nil`」**（X2d：去掉 `defer g.mu.Unlock()`，在每个 `return` 之前显式 `g.mu.Unlock()`，成功那一支写成 `g.mu.Unlock()` 在 `g.st = nil` 之前；v1.2 登记，W1b finding 1）：结清整个在闸内，只有 `g.st = nil` 落在闸外。后果与 X2b 的后一半相同：并发 Ask 若在这两条语句之间拿到闸，会读到旧快照、自信地算出金额；两边同时也构成数据竞争。放闸与 `g.st = nil` 之间没有任何函数调用可挂探针，**没有用例能抓，T4c 候选也抓不到**。原型实测：重建的 T3、T4、T4b、T5、R2、R3 与 T4c 候选全绿。其余用例没有逐条实测；它们单线程，或者闸被别人占着、走不到成功那一支，按结构看不出差别。所以只靠逐字核对。核对办法同 X2b：整个函数只有 `defer` 那一处 `Unlock`。「判定为等价」第 1 条的显式写法，必须把成功那一支的 `Unlock` 放在 `g.st = nil` 之后。
- `runner.go` 的 diff 只有 :374-375 两行注释变四行；`recover.go`、`service.go` 的 diff 只有注释；`Rebuild`（:209-256）与 `rules.go` 零 diff；`commands.go` 零 diff。
- 全仓 grep「rebuild 解不开」「解开只靠重启」零命中（§1.1 B3、C 改掉的两处之外没有别处；`docs/` 里的由 §6 订正）。
- `ask.go` 的 import 块没有变化。
- B2 两次取闸之间的窗口（§1.1 B2 冻结要点）：核对 `SettleStaleInflight` 成功之后直接往下走到既有的 `Rebuild` 调用，中间没有别的 IO、没有重试、没有等待。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许动这 5 个文件：新建 `internal/turn/settle_stale.go`；改 `internal/repl/ask.go`（B1、B2、B3 三处）、`internal/turn/runner.go`（C）、`internal/turn/recover.go`（D）、`internal/conversation/service.go`（E）。不改任何 `_test.go`，不改 `go.mod`。生产改动约 115 行（新文件约 70 行 + ask.go 约 32 行 + 其余注释），一次交付。
- 新文件：
  ```
  ===FILE: internal/turn/settle_stale.go===
  （完整内容，照 §1.1 A 逐字）
  ===END FILE===
  ```
- 补丁块，每个文件可以有多个：
  ```
  ===PATCH: internal/repl/ask.go===
  <<<<ANCHOR
  （从 HEAD 4dbd17a 真文件里原样复制的若干行，含行首 tab 与全角标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点必须从 HEAD `4dbd17a` 的真文件逐字复制（本文代码块已用 tab，仍以真文件为准；这 4 个被改的文件 `485581f` -> `4dbd17a` 零 diff），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。注意 `ask.go` 里 `此刻正有一轮在跑` 那个格式串落地后会出现两次（B2 新加一次、:298 原有一次）—— B2 的锚点用 :287-291 那五行，不要用含这句话的行做锚点。
- 不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。代码必须能 `gofmt`（tab 缩进）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（含 U+300C / U+300D 直角引号、U+FF08 / U+FF09 全角括号）、希腊字母、数学算符、普通箭头 U+2190-21FF。本刀新增的字符串字面量只有 §3.1 的五个格式串与 §1.1 A 的三个错误包装。
- 生产代码新标识符不许以 `u43` 开头（那是测试腿的前缀）。
- 回复末尾必须有：
  ```
  ===REPORT===
  补全性：5 个文件是否都给了；§1.1 A-E 每一条落在哪个块。
  位置：B2 插在哪两行之间（贴出锚点的第一行与最后一行）。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3.1 五个格式串与 §1.1 A 三个错误包装是否逐字照抄；有没有新增任何别的用户可见字面量。
  ```

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；按锚点落盘；`gofmt -l`；`go vet ./internal/conversation ./internal/turn ./internal/repl`；`go build ./...`。逐字核对：§1.1 A 全文（`git diff --no-index` 对契约代码块）；B2 在 :288 的非成员 `return` 之后、`Rebuild` 之前；§3.1 五个格式串逐字节；`runner.go` / `recover.go` / `service.go` 只有注释 diff；`Rebuild`、`rules.go`、`commands.go`、`app.go` 零 diff。
2. **合并测试腿的 2 个文件**之后，只跑 `./internal/turn ./internal/repl ./internal/conversation ./internal/store ./internal/app`（另一条流水线也可能在跑 go test；3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制）。既有用例必须全绿（原型实测全绿、零改动）。
3. **核对测试腿报告里 §2.2 在 HEAD 上单独跑的结果**与 §2 通用纪律末尾的原型结果一致（R1 四个、R3、R4、R7、R8 红；R2、R5、R6 绿）。
4. **§4 的变异逐条跑**（M1-M21，含 M10b / M10c，外加 v1.1 的 X2 与 v1.2 的 X2c；M2 只跑 repl 包；从副本还原）；任何一条没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
5. **文档**：
   - `docs/13_STEP10_CONTRACT.md`：§1.4 之后追加订正记录（2026-09-28，U43A）：`settleOne` 有了第二个调用方 `Runner.SettleStaleInflight`（`/session rebuild`），豁免的依据从「只在启动时跑」换成「同一 `(会话, 成员)` 的每个写入方都在同一把 `g.mu` 之下」，逐条列出写入点（本契约 §0 第 5 条）；`SettleLostTurns` / `AuditInflight` 本身仍只在启动时跑。§3.5 同样补一句。§3.8 的 2026-09-27 订正末条「解开的路只有重启对账」改注：另有 `/session rebuild`，它先按同一条路结清再丢会话；启动对账**持续**失败时两条路都解不开（fail closed）。
   - `docs/13a_S10_A5_CONTRACT.md` 顶部归档说明追加一条：§0 第 5 条、§1.1 E1 注释要点 4、§4「只在审查时看」第 3 条被 U43A 取代。
   - `docs/09a_STEP6_API_CONTRACT.md`：登记 `Runner.SettleStaleInflight` 与 `StaleSettlement`（§3.3）；`/session rebuild` 的行为多了「先结清」。
   - `docs/00_DECISIONS.md` #43：(a) 标「已落地（`<commit>`）」，写明能解开与不能解开的两类（§0 第 4 条）；(f) 与 (b) 的行现在可以不重启解开，成因不变；§7 的 open items 照录。
   - 契约归档为 `docs/18a_U43A_REBUILD_SETTLE_CONTRACT.md`（或按当时的编号惯例），顶部加归档说明。
   - `.handoff/HANDOFF.md` 收工时记一条。
6. **真机复验**（Griffin，Windows Terminal + PowerShell 5.1；**一次性的 `data_dir`，不碰日常库；全程不花钱**，因为被拒的 `/ask` 在起进程之前就停）。陈旧行真机上很难自然造（重启会被启动对账吃掉），所以用「council 开着时从外面往库里插一行」造：
   1. 主代理先在隔离目录准备一份只含一个 claude 成员的配置（`data_dir` 指向隔离目录）与 Python 脚本 `u43_seed.py`。脚本三个参数：库文件路径（`<data_dir>\council.db`，`store/open.go:34`）、会话 id、成员 id。用 `claude_test` 环境的 python 跑（E 盘机是 `D:\Install_path\Anaconda3\envs\claude_test\python.exe`，v1.2 实测 Python 3.13.3、sqlite 3.50.2；D 盘机的路径没核）。脚本全文冻结如下（v1.2 订正，W1b finding 4）。代码块顶格，逐字照抄；文件存成 UTF-8 无 BOM、LF，全文是 ASCII：

```python
import sqlite3
import sys

db, conv, agent = sys.argv[1], sys.argv[2], sys.argv[3]

con = sqlite3.connect(db, timeout=5.0)
con.execute("INSERT INTO inflight_turns (turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id, cursor_seq, scanned_to, started_at, phase, tier) VALUES ('u43-probe', ?, ?, 'claude', '', '', 0, 0, '2026-09-28T00:00:00Z', 'spawned', 'read')", (conv, agent))
con.commit()
con.close()

check = sqlite3.connect(db, timeout=5.0)
for row in check.execute("SELECT i.turn_id, i.phase, i.tier FROM inflight_turns i JOIN participants p ON p.conv_id = i.conv_id AND p.agent_id = i.agent_id WHERE i.conv_id = ? AND i.agent_id = ? AND p.active = 1", (conv, agent)):
    print(row[0], row[1], row[2])
check.close()
```

      - **插入语句与 v1.1 逐字相同**：会话 id 与成员 id 走参数绑定，其余列全部写成字面量，不靠 schema 的默认值。这是 v1.1 的订正（W1 F1 与 V1）：`tier` 省略时默认 `''`，`classifyLost` 对 `Tier != "read"` 在原因末尾追加「；工作目录 %s 可能有半截落盘的文件」，第 4 步 `/log` 的正文就对不上；`planned_session_id`、`resume_session_id` 显式写空串，钉住「点不出名字」那一支。`work_dir` 不写（默认 `''`，`tier` 为 `read` 时不进任何文本）。原型实测：照这一句插行后，§6 第 4 步的三行屏幕与 `/log` 正文逐字成立；照 v1 的列（没有 `tier`）插行，面包屑正文是「成员 <id> 的一轮在上次退出时没记上账：起过进程，结局未知；这条 provider 会话点不出名字；工作目录  可能有半截落盘的文件」（「工作目录」后面两个空格）。
      - **先 `commit`、关连接，再另开一条连接读回**（v1.2，W1b finding 4）。第 4 步的 `/ask` 不花钱，全靠 A 之五查到这一行：`runner.go:380-386` 是起进程之前唯一的拦截。所以读回就是这一步的闸。Python 的 `sqlite3` 在 DML 之前隐式开事务，不 `commit` 就关连接会静默回滚、不报错；在同一条连接上读回，又看得见自己没提交的写。读回连 `participants`、只认活跃成员，因为 `inflight_turns` 对 `conv_id`、`agent_id` 没有外键（`schema.go:389-402`），会话 id 或成员 id 抄错时插入照样成功。
      - **期望输出恰好一行 `u43-probe spawned read`**（Windows 上行尾是 CRLF，屏幕上看不出）。打出别的、什么都没打、或者报错，都**不要**往下敲第 4 步的 `/ask`，先查三个参数。
      - v1.2 原型实测（Go 端 store 开着，脚本作为外部进程跑）：冻结脚本打出那一行，Go 端 `InflightFor` 读到 `tier == "read"`、`work_dir == ""`，第 4 步整段逐字成立，`turn_usage` 里 `outcome = 1` 的行数为 0；去掉 `commit`，什么都不打、退出码 0、Go 端没有行；不 `commit` 且在同一条连接上读回，打出那一行、Go 端却没有行；会话 id 抄错，什么都不打（读回不连 `participants` 的写法此时会打出那一行）；库路径抄错，`sqlite3` 在那个路径上新建一个空文件，INSERT 报 `no such table: inflight_turns`，退出码 1，什么都不打。
   2. 用那份配置启动 council（TUI 或 `-no-tui` 行式都行），逐条敲（每条单独回车）：

      ```
      /new u43真机
      ```

      ```
      /join <成员 id>
      ```

      记下 `/new` 打出的会话 id。
   3. **council 不关**，另开一个 PowerShell 窗口，用 `claude_test` 的 python 跑 `u43_seed.py <库文件> <会话 id> <成员 id>`。核对它恰好打出一行 `u43-probe spawned read`；打出别的就停在这里，不进第 4 步。
   4. 回到 council。**只有第 3 步恰好打出 `u43-probe spawned read` 才敲**：

      ```
      /ask <成员 id>
      ```

      预期：打「这个成员有一轮没记上账的在飞记录还留在库里……」，没有起进程（不花钱）。它前面那一行随档位不同：简洁档是真机默认，打 `<成员显示名>: 没有回复（拒绝）`；详细档打 `outcome refused  agent <成员 id>`。两档都先打 `正在问 <成员 id> ...`。v1.2 原型两档各跑一遍；原型 REPL 配置里没有这个成员，简洁档的显示名退回 id，打的是 `ag_wait: 没有回复（拒绝）`（`ask.go:346`、`:372`，`verbosity.go:50-55`）。

      ```
      /session rebuild <成员 id>
      ```

      预期三行：「已结清 <成员 id> 的一轮在飞记录（turn u43-probe，phase spawned），对它 /ask 不会再因为这条记录被拒」、「那一轮记为丢失：可能花了钱，用量与金额都不知道（与重启时启动对账的记法相同）」、「本来就没有开着的会话，下一轮照常是首轮」。

      ```
      /log 3
      ```

      预期：最后一条是 system 消息，正文「成员 <成员 id> 的一轮在上次退出时没记上账：起过进程，结局未知；这条 provider 会话点不出名字」（`recover.go:108`、`:229` 拼出来的；措辞见 §7「待 Griffin」第 4 条）。`/log` 那一行的 TEXT 列在正文之后还跟着一个空格与 `[event=turn_lost]`，那是 `/log` 显示 meta 的既有格式，不是正文的一部分（v1.1 原型照 §6.1 的插入语句实跑：`/log 3` 那一行是 `1    system  system  -   -      成员 ag_wait 的一轮在上次退出时没记上账：起过进程，结局未知；这条 provider 会话点不出名字 [event=turn_lost]`，成员 id 换成真机的即可）。`/new` 打的是「已建会话 <id>，标题: u43真机」，会话 id 从这一行抄；`/join` 等同 `/add`（`commands.go:144`）。
   5. **不要**再 `/ask`（会真的花钱）；若要验「解开之后能起进程」，由 §2.2 R1 担保，不必真机。退出 council，删掉隔离目录。
7. 清理本刀在中转根与 scratchpad 里的全部中间物。

---

## 7. open items

### 7.1 待 Griffin

1. **A 之五那句拒绝要不要提 `/session rebuild`**。本刀照范围不改（`ask.go:620` 原句只说重启）。被拒的用户今天从屏幕上看不到「不重启也能解开」。可选的补法：句末加「；不想重启可以敲 /session rebuild <成员>，会丢掉它的 provider 会话、下一轮付一次入场费」。主代理的默认：先不补（改那句要连带改 4 个既有测试文件里逐字复制的常量：`internal/repl/s10a_gate_screen_test.go`、`internal/app/s10a_settle_fail_blocks_ask_test.go`、`internal/repl/cox_round_test.go`、`internal/repl/smx_summary_round_test.go`），等他看过真机复验再说。
2. **启动对账持续失败（例如 `turn_usage` 主键冲突）仍然解不开**。不变量 (b) 下这是结构性的：同一条路、同一个失败，本刀 fail closed。要解开得有一条不走启动对账的写法（例如把冲突的那一行 `turn_usage` 改记到另一个 turn id 再补 lost 行），那是新的记账规则，要他裁。今天只能手改 SQLite；这种状态真实里怎么出现，目前只见过测试夹具造出来的。
3. **`/session rebuild` 结清之后必然丢 provider 会话**（下一轮付入场费，claude 约 1 美元量级）；重启对账只结清、不丢会话。要不要另开一个「只结清、不丢会话」的入口（例如 `/session settle <成员>`）。主代理的默认：不加（入口越少越好；重启就是免费的那条路）。
4. **面包屑措辞「在上次退出时没记上账」**对同进程里的双重故障（#43(f)）留下的行不准（那一轮不是「上次退出时」的）。它是启动对账的 `lostNoticeMessage`（`recover.go:229`），不变量 (b) 要求同一条路，本刀不改；改它会同时改启动横幅之后的面包屑。

### 7.2 待主代理

1. **`/help` 里 `/session` 那一行**（`commands.go:153`「/session rebuild <agent-id> 丢弃并重建」）不提「先结清」。v1 写的是「并进 #46 裁定的 `/help` 那一批」；那一批就是 CXH，v1.1 修订时现查它的文本只改 `commands.go:143`（`/add` 那一行），**没有收这一行**。要么由主代理给 CXH 追加这一行，要么另开一刀；本刀照旧不改。
2. **`/rules reload --rebuild` 同类不结清**（`rules.go:221` 调 `Rebuild`）：对一个被 A 之五锁住的成员它照样打「已重建」而对方下一轮仍被拒。本刀有意不让它结清（用户没点名，违反 (d)）；要不要在它的输出里对「库里还有陈旧在飞记录」的成员加一句提示，另开一刀。
3. **`SettleLostTurn` 的 `ConvMissing` 一支**（13 §3.9）从本入口走不到：外键开着时有成员的会话删不掉，`/use` 要求会话存在；只有「外键关着手改库、同时 council 开着」才到得了。届时屏幕照打 L1 + L2（L2 不声称写了哪一行，仍然成立），不另加分支。
4. §6 第 5 步的文档订正（13 §1.4 / §3.5 / §3.8、13a、09a、00_DECISIONS #43）。
5. #43(b) 的成因（`refuse` 用调用方 ctx，取消时清理写不进）仍在；本刀只让它留下的 begun 行能不重启解开。
6. （v1.1 新增，v1.2 按 W1b finding 1 补充）**X2b 要不要从「只在审查时看」升成用例（T4c）**。裁决把「读完之后、`settleOne` 之前放闸」定为只在审查时看，当时的依据是「后果是本机抓不到的数据竞争」。W1b 指出后果不止于此：在一个窄交错里，并发 Ask 拿着结清前的内存快照，把库里已标可疑的基线当可信，算出确定金额。这是 T5 那一类伤害，白盒模拟的数字见 §4 X2b 那一条；真并发没造出来过。同类的 X2d 连 T4c 也抓不到，收不收 T4c 它都只靠逐字核对。
   T4c 候选的写法（v1.2 原型已写成、已实测，约 40 行）：**`TestU43SettleStaleHoldsGateAcrossSettle`**（白盒，确定性：不睡、不起 goroutine）。包级探针类型 `u43...`（指针接收者），内嵌 `*store.Store`，字段 `gate *sessionGate`、`armed bool`、`calls int`、`sawGateFree bool`，**只重写 `ProviderSession`**；体例与 T4b 的探针相同：`armed` 为真时先置假、`calls++`、对 `gate` 做 `TryLock()`，成功就立刻 `Unlock()` 并置 `sawGateFree = true`，然后无论武装与否都透传 `s.Store.ProviderSession`。装置同 T4b：「开口过的会话」建在包装 `svc` 与这个 `r` 上，开口那一轮对 `ProviderSession` 的读发生在武装之前。经 `svc.BeginTurn` 写 `u43-stale`（alice，spawned，`PlannedSessionID: "u43-planned"`、`ResumeSessionID: ""`、`ScannedTo: 2`，其余照 §2.0；v1.2 原型就是这样写的）。`probe.gate = r.gate(sessionKey{conv, alice})`、`probe.armed = true`，然后 `got, err := r.SettleStaleInflight(ctx, conv, alice)`。前提守卫：`calls == 1`；`err == nil` 且 `got.Found`。断言：`sawGateFree == false`。正对照（断言之后接着）：再武装，在闸外直接调 `svc.ProviderSession(ctx, conv, alice)`（前提守卫 `err == nil`），断言 `calls == 2 && sawGateFree == true`。【判别力】要点：`settleOne` 在事务之外读回 `ProviderSession`（`recover.go:155`），原型实测武装之后 `ProviderSession` 只被调这一次（`calls == 1`）；X2b 下这次读落在闸外。
   原型实测杀伤面：好实现绿；X2b、X2、M1、闸 key 写反四种都红，其中 X2b 只有它杀得到；X2c、X2d、M2 下绿。
   要收，就在 §2.1 T4b 之后加 T4c，§4 把 X2b 挪进变异表（杀手 T4c），并把 T4c 加进 X2、M1 的杀手。本版取保守读法，照裁决不收。
7. （v1.2 新增，W1b finding 4 的可选部分）**真机复验要不要再加一道「假 claude」保险**：一次性配置里 claude 的 `command` 指向一个不存在或不花钱的程序，这样第 4 步的 `/ask` 万一没被拒，也起不出真 claude。
   - 已核（读代码）：`config/validate.go:56` 只查 `command` 非空；A 之五的检查（`runner.go:380-386`）在查配置（`:388`）之前，所以被拒的那一支不碰 `command`。
   - 没核：真 council 在 `command` 不存在时的启动全程（横幅、启动对账、TUI），以及第 4 步的屏幕在这种配置下是否逐字不变。
   v1.2 冻结的读回（§6 第 6 步之 1）已经确定性地挡住了「行没落库还去 `/ask`」，这道保险是第二道。它改变 Griffin 要跑的东西；加不加、怎么加由主代理定，本版不加。
