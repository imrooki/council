# AT1-FU 冻结契约：@ 补全的落地后跟进（v1.2 = v1.1 + W1b 修订，2026-09-29）

> **归档说明（2026-09-29，主代理）**：本文是 AT1-FU 的冻结契约，派工时的原件是 `E:\multi_model_cowork\at1fu_contract.md`（v1.2 = v1.1 + W1b 修订），原样归档；文中行号指 HEAD `4908f13`。AT1-FU 已于 `ee247c6` 落地，真机未复验：
> - `internal/tui/model.go` 的五个补丁由 grok-4.7 实现，与 §1.1 逐字节相同，落地后 SHA256 `6d868927…206e`；
> - `internal/tui/atf_review_test.go` 由 Opus 测试腿写成，第一次跑测试之前先做了剪贴板审计；
> - Opus 集成员：§4 的 8 条变异全被点名杀手杀掉，没有机械修正；
> - Opus 独立审查：1 条 minor（见下）。
>
> **订正（2026-09-30，主代理）**：§6 第 5 步第 1、2 项当时标「默认不做」，§7 (j) 待主代理。当时的理由已经不成立，是主代理的记忆索引写过时了。§7 (j) 定为：第 1、2 项可以做。测试里「不执行 Ctrl-V / Ctrl+Shift+C 返回的 Cmd」的纪律照旧保留：单元测试本来就不该碰真剪贴板。
>
> **落地后订正（2026-09-29，主代理）**：
> - **列表分支 Ctrl-C 判据的 Alt 那一半没有用例担保。** 契约 §0 R5 测过 Ctrl+Alt+C，决定不加用例，因为 A60 已由 F5 杀掉。审查员另做了一条变异：判据写成只放过 Shift（`msg.Mod&^tea.ModShift == tea.ModCtrl`），全部 TestAtx 与 TestAtf 照绿；那时列表开着按 Ctrl+Alt+C 会掉进空闲的 Ctrl-C 分支，直接退出程序。不花钱、不丢账。这是契约接受的缺口，要补用例以后单开一刀。
> - 审查员另外实测：§1.1 那句「换键（SetKeys("ctrl+shift+v")）只让 F1 第 2 步红」成立；Unbind 挪到 Focus 之后仍在 return 之前，是等价写法。
> - `core.autocrlf=true` 的检出里，git apply 写出的工作树是 CRLF，原始 SHA 对不上；按 LF 规范化后与契约一致。本仓库 `core.autocrlf=false`，没有这个问题。
> - **列表分支 Ctrl-C 判据的 Alt 那一半：已由 CL1 补用例**（2026-10-01，`5c4840a`，#50，`19h` 的 C3）。审查员当年那条变异（只放过 Shift）现在有杀手；CL1 的 W1 另查出锁定键（NumLock、CapsLock）那一半，也一并补了。


## v1.1 变更记录（2026-09-29）

依据：W1 审查（`E:\multi_model_cowork\at1fu_w1_findings.md`，F0-F3，全部 minor）与主代理裁决书（`E:\multi_model_cowork\at1fu_rulings.md`，唯一权威）。v1 快照在 `E:\multi_model_cowork\at1fu_contract_v1.md`。

- **§1.1 冻结的生产代码一个字节没改**（五个补丁、落地后 SHA256、782 行、13 增 5 删都与 v1 相同）。改动全在测试侧与说明侧。
- **原型换了一份**：v1.1 的全部新期望值与全部变异读数出自 `E:\Isolating_test\council\at1fu_v11\`（见下面「原型」一条）。v1 的实测读数凡是本版照旧引用的（R0、R1、R2 的探针读数，R0「同类」的两条 grep，§2 F1-F3 的期望值，§4 A52-A54 与全部等价变异），都在这份原型上重跑过，结果与 v1 相同；只有「E4 两半在 v1 用例集上各 61 条 PASS」是 W1 的读数，引用处注明了出处。
- **F0（接受）**：`Unbind()` 与 `SetEnabled(false)` 用例分得开。改 §1.1 末条（原来说「只能审查时看」）；§0 R0 补这条事实；§2.1 第 5 步的 Fatalf 消息写明它也拦 `SetEnabled(false)`；§4 登记 A56（杀手 F1 第 5 步的前提）；「只在审查时看」移走 B13 那一条。正对照不改成 `SetEnabled(true)` + `SetKeys`（实测会放过 A56）。
- **F1（接受）**：§0 R0 的 HEAD 读数、原型读数与代价段各加有选区时的读数（HEAD 先删选区再交回 `Paste`；补丁之后选区与文字都不动）。不加用例。
- **F2 与 §7 (b)（改为本刀一并做，只加测试）**：§0 新加 R5；§2 新加 F4 `TestAtfModifiedListKeys`（六个子测试）与 F5 `TestAtfCtrlShiftCClosesList`；§3 冻结新字面量；§4 登记 A57-A60（W1 的 O5-O8），每条实测点名杀手变红；§7 (b) 改「已做」。F5 不执行 Ctrl+Shift+C 返回的 Cmd（它是 textarea 的 `CopySelection`，写剪贴板），只判 nil 或比 `tea.Quit` 的函数指针。
- **F3（接受）**：§7 (a) 的 E4 那句改成两半各自单独跑过。
- **§7 (a) 关闭**：X1-X4、MX2-MX5 与 E1-E7 按 AT1 W2 原始报告摘录（`E:\multi_model_cowork\at1fu_source_findings.md`）逐条对上，写进 §4 等价清单开头。
- **§7 其余**：(c) 留作真机步骤，可选、未测，补 Windows Terminal 自己占键的说明；(d) 主代理定不加提示，移到「待 Griffin」，低优先；(e) 按 F5 更新（合成消息已测，真终端未测）；(f) 主代理落地时改归档；(g) 维持；(h) 维持，读数换成本版原型的；新加 (i)（F1 第 7 步恒真，只登记，不改）。
- **数字**：`-run '^(TestAtx|TestAtf)' ./internal/tui ./internal/repl` 不变异时 PASS 行 61 → **69**（F4 七行、F5 一行）。A52-A54 与全部等价变异都在 69 行的用例集上重跑过。§6 的变异从三条变成八条。
- **顺手的订正与补充**：§0 R0「同类」里 textarea `CopySelection` 绑定的行号 v1 写成 `:124`，实际是 `textarea.go:125`（`:124` 是 `SelectAll`），已改；§0 R5 加了一条 HEAD 全仓库 grep（带修饰键的 ↑、↓、Esc 与 Ctrl+Shift+C 在 `*.go` 里无命中）。

### W1b 修订（v1.2，2026-09-29）

依据：W1b 第二轮对抗审查（只审 v1.1 的修订；没有 blocker，1 major、4 minor）。v1.1 快照在 `E:\multi_model_cowork\at1fu_contract_v11.md`。本节是 v1.2 的变更记录；上面 v1.1 那几条是历史记录，保留原话，与本节冲突时以本节为准。

- **§1.1 冻结的生产代码一个字节没改**。v1.2 原型 `E:\Isolating_test\council\at1fu_fix\`（`git archive 4908f13` 解出）从本文 §1.1 按程序取出十个 go 代码块打在 HEAD 字节上，五个锚点各恰好一次，结果 SHA256 仍是 `6d868927a5b0cb8514f95c27d761f0c9587b7c212b913e02fd0cfd956c73206e`、782 行、无 BOM、无 CR。**没有新用例、没有新变异、期望值与 PASS 行数（69）不变**：五条全是措辞与流程。v1.2 新写成事实的读数都出自这份原型（下面逐条注明），交付前已删。原型的探针同样没有执行过任何 Ctrl-V 返回的 Cmd：只判 nil、只比 `textarea.Paste` 的函数指针，报失败时的实参只带 `cmd == nil` 这个布尔值。
- **finding 1（major，接受，按 W1b 建议改并收紧）剪贴板审计要在第一次 `go test` 之前**。契约规定的运行里有两次正好走进失败分支、手里拿着 `textarea.Paste`：测试腿在 HEAD 上跑 F1（第 3、4 步红），集成员跑 A53（F1 第 3、4 步红）；A60 下 F5 第 2 步的失败分支拿着 `tea.Quit`。v1.1 把审计放在 §6 第 3 步变异之后，§2.0 也没让测试腿先自查；绿跑走不到失败分支，失败消息的实参或 atf 助手里若调用了 Cmd，第一次读剪贴板就发生在审计之前。改动：
  - §2.0 新加「Cmd 变量只许三种用法」：与 nil 比较；`reflect.ValueOf(...)` 的唯一实参、紧跟 `.Pointer()`；传给 `atxWantNilCmd`（HEAD `atx_model_test.go:143-148`，函数体只有 `if cmd != nil { t.Errorf(...) }`，消息里不带 cmd，已读核）。原样赋值、原样当 atf 助手的返回值可以，去处同样受这条约束。`Errorf` / `Fatalf` / `Logf` 的实参里不许有这个变量（要报就报 `cmd == nil`）。接收它的 atf 助手，函数体里对形参的每一处用法同样只许这三种。比 W1b 建议的「只许出现 nil 比较或 reflect」多写明了 `atxWantNilCmd` 的许可与传递规则，免得字面 grep 把 F4、F5 必然用到的 `atxWantNilCmd` 判成违规。
  - §2.0 新加「先审计、后跑」：测试腿在第一次 `go test` 之前（尤其在「HEAD 上的预期」那次运行之前）逐处核对，交付时报告核对结果。
  - §6：审计从第 3 步移到第 2 步合并之后、第一次 `go test` 之前，写明「审计不过，测试与变异都不跑，退回测试腿」。第 3 步那句「另外核一次」删掉，改成指回第 2 步。
  - 已读核：`tui1bStep`（`tui1b_model_test.go:251-259`）、`tui1bType`（`:280-286`）、`atxSteps`（`atx_model_test.go:85-91`）都不执行 Cmd；§2.0 点名的助手里执行 Cmd 的只有 `tui1bRun`（`:178-217`，`msg := c()` 在 `:195`）。
- **finding 2（minor，接受）正对照的禁令写窄了**。v1.1 只禁「在 `SetKeys` 之前加 `SetEnabled(true)`」。v1.2 原型实测（A56 下，照 §2.1 写的 F1 把第 5、6 步的正对照换成下面的写法，其余不动）：`SetKeys` 之前 `SetEnabled(true)`、`SetKeys` 之后再 `SetEnabled(true)`、整体赋值 `m.ta.KeyMap.Paste = textarea.New().KeyMap.Paste`、整体赋值 `m.ta.KeyMap = textarea.New().KeyMap`，四种都让 `Enabled()` 变 true、按 Ctrl-V 的 Cmd 是 `textarea.Paste`（只比指针），F1 整条 PASS，A56 被放过；同一次跑里只调 `SetKeys("ctrl+v")` 的那一种在第 5 步前提 Fatalf（`SetKeys` 之后 `Enabled()` 为 false）。不变异时五种都 PASS。改成写不变量：§2.1 第 5 步（第 6 步同）、§2.0 那处括号（改成限定写法，并限定 `textarea.New()` 只许读）、§0 R0 末句、§2.1【判别力】、§6 第 2 步的审计清单。§0 R0 里「`Unbind()` 之后单独 `SetEnabled(true)` 恢复不了」那一段是另一件事（为什么不拿它当对照），照旧。
- **finding 3（minor，接受）§4 两行锚点的理由写得不准**。v1.2 原型实测（落地后与 HEAD 的 `model.go` 字节上各跑一遍）：带三个行首 tab 的单行 `<TAB><TAB><TAB>case msg.Code == tea.KeyEscape:` 与 `<TAB><TAB><TAB>case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':` 按字节串找各恰好一次；不带 tab 各两处（落地后 `:378` 与 `:426`、`:375` 与 `:396`，前者三个 tab、后者两个 tab；HEAD `:372` 与 `:420`、`:369` 与 `:390`）；两行锚点各恰好一次。§4 的理由按此改写，仍用两行锚点。W1b 另报「只用单行打的 A59、A60 读数与两行锚点相同」，本版没有重跑，引用处注明出处。
- **finding 4（minor，接受）「tui1bRun 只跑补全返回的 Cmd」对 F2 第 2 段不成立**。v1.2 原型实测：第 1 段（列表开着）三个键返回的 Cmd 都非 nil，`tui1bRun` 产出都是 `["other:cursor.BlinkMsg" "other:cursor.blinkCanceled" "other:cursor.blinkCanceled"]`；第 2 段（列表不在）Shift+Enter、Alt+Enter 返回提交的 Sequence，产出 `["print:> hi @br" "submitDone"]`、`Submit` 1 次、收到 `"hi @br"`；Shift+Tab 返回 nil，产出为空。改 §2.0 那句与 §6 的「参数只许是……」。
- **finding 5（minor，接受）真机步骤不再让 Griffin 按 Ctrl+Shift+C**。§6 第 5 步第 4 项去掉那一下，改成一句说明（由 F5 的合成消息担保；WT 默认占用它、写剪贴板，真机不测）；§7 (c)、(e) 跟着改。**同类（W1b 没点）**：同一步第 1、2 项要 Griffin 先复制一段字再按 Ctrl-V，同样经过剪贴板。这是方向问题，取保守读法：两项标「默认不做」，去留写进 §7 (j) 待主代理；步骤原文不删。
- **W1b「判定不报的」两条**（F5 第 2 步不断言表头「空闲」；R5「没有选区」不进断言）：同意，不改。W1b 的 rulings_check 里「契约把 F4 收紧成一律不调 `tui1bRun`」：同意是收紧，不改。

## 基线与总则

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `4908f13`（完整 `4908f139e4062aac294974153b7268684f4657be`，= origin/main）。**本文行号一律以 HEAD `4908f13` 为准**；落地后的行号另外注明。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`（不设 `GOFLAGS`）。本机没有 gcc，不能 `-race`。依赖版本（`go.mod`）：`charm.land/bubbletea/v2 v2.0.9`、`charm.land/bubbles/v2 v2.2.1`、`github.com/charmbracelet/x/ansi v0.11.8`、ultraviolet `v0.0.0-20260811164956-006e29f97886`（indirect）。下文引 bubbles 源码指 `E:\Go\modcache\charm.land\bubbles\v2@v2.2.1\`，引 ultraviolet 指同一模块缓存里的 `github.com\charmbracelet\ultraviolet@v0.0.0-20260811164956-006e29f97886\`。
- 本刀 KEY 是 `AT1-FU`；**测试腿的包级标识符前缀是 `atf`**（类型、函数、变量、常量一律 `atf` 开头，测试函数 `TestAtf...`）。本刀生产代码**没有新标识符**：只在 `New` 里加一行调用、改四段注释。
- 来源：`docs/19b_AT1_MENTION_COMPLETION_CONTRACT.md` 文件头的「落地后订正」（`:5-20`）前五条；`docs/00_DECISIONS.md` #47 的「AT1 落地记录」（`:2203-2224`，其中 `:2207`「Opus 独立审查：2 major、3 minor，都不是实现偏离契约，交后续一刀 AT1-FU」）。方向由主代理裁定（R0-R4；R5 依 v1.1 裁决书，只加测试），本契约不改方向。
- **原型**：v1 的期望值与变异读数出自隔离拷贝 `E:\Isolating_test\council\at1fu_draft\`（已删）。**v1.1 的全部期望值、全部变异读数出自隔离拷贝 `E:\Isolating_test\council\at1fu_v11\`**（`git -C D:\DeskTop\My_Project\council archive 4908f13` 解出）。它的 `model.go` 是脚本从本文 §1.1 的十个 go 代码块逐字取出锚点与替换块、打在 HEAD 的 `model.go` 字节上得到的（五个锚点各恰好一次），SHA256 `6d868927a5b0cb8514f95c27d761f0c9587b7c212b913e02fd0cfd956c73206e`，782 行（HEAD 的 `model.go` 是 `c937571c90e213040e7cec201af23b8e4b375824c746caadec530f5ec02513bf`）。原型下 `gofmt -l internal` 为空、`go vet ./internal/tui ./internal/repl` 无报告、`go build ./...` 通过；**全量 `go test -count=1 ./...` 一次调用，九个有测试的包全绿**（`cmd/council`、`internal/app`、`config`、`conversation`、`provider`、`repl`、`store`、`tui`、`turn`），**HEAD 既有用例零改动、零变红**。§2 的五个测试函数在原型里写成（原型的测试文件不交付，测试腿照 §2 自己写）；§4 的变异逐条在原型上跑过（锚点恰好一次、从字节副本还原、还原后 SHA256 相等），跑法是 `go test -count=1 -v -run '^(TestAtx|TestAtf)' ./internal/tui ./internal/repl`（不变异时 **69 条 PASS 行**，含子测试：TestAtx 55 行、TestAtf 14 行；v1 的 TestAtf 是 6 行，合计 61 行）。原型、探针与变异装置在交付前用 `fs.rmSync` 删除。**v1.2（W1b 修订）新写成事实的读数出自第三份隔离拷贝 `E:\Isolating_test\council\at1fu_fix\`**（同样由 `git archive 4908f13` 解出、§1.1 十个块打在 HEAD 字节上，SHA256 同上），逐条见上面「W1b 修订」；交付前同样删除。
- **剪贴板禁令（本刀全程）**：不碰真剪贴板。三份原型里都**没有执行过** textarea 的 `Paste` 命令，也没有执行过任何读剪贴板的代码；v1.1 的原型与探针同样**没有执行过** Ctrl+Shift+C（textarea 的 `CopySelection`，写剪贴板）之后返回的任何 Cmd，只判 nil 或比函数指针；v1.2 的原型没有按过 Ctrl+Shift+C。§0 里凡是说到 `pasteMsg` 之后会怎样，都是按代码读的，不是实跑。§2 的用例同样不许执行，而且测试腿与集成员都要在**第一次 `go test` 之前**按 §2.0「Cmd 变量只许三种用法」核对测试文件（v1.2，§2.0「先审计、后跑」、§6 第 2 步）。

## 与 CH1 及 AT1 的相交表

CH1（对话模式）由另一条流水线修订（`E:\Isolating_test\council\ch1_*`、`E:\multi_model_cowork\ch1_*`）。**本刀起草时没有打开任何 `ch1_*` 文件**（派工要求不许碰）；下表的 CH1 一列依据是派工书的说明（「它改 `internal/repl`、`internal/app`、`cmd/council`，不碰 `internal/tui` 的生产文件」）与 `19b` 相交表里已核过的 CH1 文件清单（`19b` `:55-62`）。

| 本刀的文件 | CH1 | AT1（`4202b6c`，已落地） | 结论 |
|---|---|---|---|
| `internal/tui/model.go`：`New` 加一行（B13）、四段注释（B12a-d） | 不碰 `internal/tui` 的生产代码（派工书；`19b` `:55`「CH1 §1.2」） | B13 的锚点（`:171-172`）与 B12 的四个锚点都是 AT1 之前就有的行；AT1 改了 `frameHeight` 的函数体、在 `View` 之后加了 `atCursor`、在 `frameHeight` 之后加了 `atArm` 等方法，没改这几段注释（`19b` `:9`） | 按文件不相交 |
| 新文件 `internal/tui/atf_review_test.go`（前缀 `atf`） | CH1 在 tui 包新加的是 `chx_program_test.go`（前缀 `chx`，`19b` `:62`） | 只读复用 `atx_model_test.go` 的夹具与断言助手（§2.0 逐个点名），不改它 | 文件名与前缀零碰撞 |
| 行为：Ctrl-V 不再交给 textarea | 按 `19b` `:60` 的记载，CH1 的 `chx_program_test.go` T1 用 `PasteMsg` 粘贴（终端括号粘贴那条路），不按 Ctrl-V；本刀不动 `tea.PasteMsg` 那一支。按记载读，未实跑 CH1 的文件 | AT1 的 M9、V1 等粘贴用例都喂 `tea.PasteMsg`；HEAD 全部 `_test.go` 里没有任何用例喂 Ctrl-V（§0 R0 的「同类」） | 不相交 |

**落地顺序**：与 CH1 互不依赖，先落后落都能编译。后落的一方按当时的真文件重取锚点（本刀五个锚点全在 `internal/tui/model.go`）。CH1 的流水线可能同时在跑 `go test`：本刀集成时全量 `./...` 若红在 `internal/tui` 以外的包，先单独重跑那个包再判。

---

## 0. 病根 / 实测（行号以 HEAD `4908f13` 为准）

「HEAD 上」的读数是原型打补丁之前、在同一份隔离拷贝里用一个一次性的探针用例读的（探针用完即删，不交付）；v1.1 新加的 HEAD 读数（R0「有选区时」、R5）是把 v1.1 原型的 `model.go` 换回 HEAD 字节、跑一次性探针读的，读完从字节副本换回、SHA256 核对相等。「原型上」是打完 §1.1 之后 §2 的用例与同一个探针读的。两边的装置都是 AT1 的「新 Model(80, 24, atxMembers)」（`atxNew`，`atx_model_test.go:73-79`）。探针里任何 Cmd 都没有执行，只判 nil、或比 `textarea.Paste` / `tea.Quit` 的函数指针。

### R0 Ctrl-V 的第二条粘贴路

- **机制（按代码读）**：textarea 的默认键表把 `ctrl+v` 绑给 `Paste`（bubbles `textarea/textarea.go:108`）。`Update` 里命中它时 `return m, Paste`（`:1369-1371`），**不经 `tea.Batch`，返回的就是包级函数 `textarea.Paste` 本身**。`Paste` 被执行时读剪贴板（`:2011-2018`），交回未导出的 `pasteMsg`；它回到 Council 时走 `updateInput` 最后的 default 分支（`model.go:440-443`），textarea 在 `:1441-1443` 把字插进去。这条路不经 `tea.PasteMsg` 那一支（`model.go:433-438`：收起列表、状态栏换成 `粘贴了 N 个字符`），也不经按键 default 分支的 `atRecheck`（`:426`）。后果照 `19b` `:6`：列表带着旧前缀开着，补全时按旧前缀的长度退格。**本刀没有实跑 `pasteMsg` 之后的这段**：不许执行 `Paste`，`pasteMsg` 又是 textarea 包未导出的类型，tui 包里构造不出来。
- **HEAD 上（实测）**：`tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}.String() == "ctrl+v"`。`m.ta.KeyMap.Paste.Enabled() == true`。新 Model 什么都不敲、直接按 Ctrl-V：返回的 Cmd 是 `textarea.Paste`（`reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(textarea.Paste).Pointer()`，**只比函数指针，没有调用它**），值 `""` 不变。敲 `@b`（行 == `B3`）再按 Ctrl-V：Cmd 同样是 `textarea.Paste`，值 `"@b"`、行仍 `B3`。把 §2 F1 放在 HEAD 的 `model.go` 上跑，第 3 步（敲 `x` 再按）同样得到非 nil 的 Cmd（§2.0「HEAD 上的预期」）。
  - **有选区时（v1.1 实测）**：新 Model 敲 `abc`，按 `tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}`（选中 `c`；`HasSelection() == true`，光标 (0, 2)），再按 Ctrl-V：Cmd 是 `textarea.Paste`（只比指针，没有调用），值 `"ab"`，`HasSelection() == false`，光标 (0, 2)。textarea 命中 `Paste` 时先 `m.deleteSelection()` 再 `return m, Paste`（`textarea.go:1369-1371`）：**HEAD 上选区在读剪贴板之前就删掉了**。
- **原型上（实测）**：`m.ta.KeyMap.Paste.Enabled() == false`；敲 `x` 按 Ctrl-V → Cmd == nil、值 `"x"`；敲 `@b` 按 Ctrl-V → Cmd == nil、值 `"@b"`、行 == `B3`。解绑之后 Ctrl-V 落到 textarea 的 default 分支，`msg.Text == ""`（`:1433-1436`）什么都不插；光标不动，没有闪烁 Cmd；`Batch` 全是 nil，返回 nil。
  - **有选区时（v1.1 实测）**：同上的装置（`abc`、Shift+←），按 Ctrl-V：Cmd == nil，值 `"abc"`，`HasSelection() == true`，光标 (0, 2)。**补丁之后选区与文字都不动。**
- **正对照为什么不用 `SetEnabled(true)`（实测）**：`Unbind()` 把 `keys` 置 nil（bubbles `key/key.go:118-121`），`Enabled()` 要求 `!disabled && keys != nil`（`:106-108`），`SetEnabled(true)` 只翻 `disabled`（`:111-113`）。原型实测：`Unbind()` 之后 `SetEnabled(true)`，`Enabled()` 仍是 false，按 Ctrl-V 的 Cmd 仍是 nil——拿它当正对照是空的。改用 `SetKeys("ctrl+v")`（`:84-86`）：`Enabled()` 变 true，按 Ctrl-V 的 Cmd 是 `textarea.Paste`（函数指针相等），值不变。§2 F1 用的就是这个。
- **`SetEnabled(false)` 与 `Unbind()` 用例分得开（v1.1，W1 F0，实测）**：`SetKeys` 只换 `keys`，不清 `disabled`（`:84-86`）。B13 若写成 `SetEnabled(false)`，`disabled` 留着，第 5 步 `SetKeys("ctrl+v")` 之后 `Enabled()` 仍是 false，F1 第 5 步的前提 Fatalf（§4 A56：原型实测只红这一处，第 2、3、4 步照绿）。所以正对照**只许一种写法**：对 `m.ta.KeyMap.Paste` 只调一次 `SetKeys("ctrl+v")`，别的一概不碰（§2.1 第 5 步）。第 5 步能杀 A56，靠的是「正对照只换 `keys`、不碰 `disabled`」。v1.2 原型（`at1fu_fix`）实测，A56 下把第 5、6 步的正对照换成下面四种之一，`Enabled()` 都变 true、按 Ctrl-V 的 Cmd 都是 `textarea.Paste`（只比指针，没有调用），照 §2.1 写的 F1 整条 PASS，A56 被放过：`SetKeys` 之前 `SetEnabled(true)`；`SetKeys` 之后再 `SetEnabled(true)`；整体赋值 `m.ta.KeyMap.Paste = textarea.New().KeyMap.Paste`；整体赋值 `m.ta.KeyMap = textarea.New().KeyMap`。同一次跑里只调 `SetKeys` 的写法在第 5 步前提 Fatalf。（v1.1 只禁了第一种，W1b finding 2。）
- **代价**：终端把 Ctrl-V 原样交给程序时（没有自己接管粘贴的终端），Ctrl-V 什么都不做，也没有提示。Windows Terminal 默认自己接管 Ctrl-V、发括号粘贴（`tea.PasteMsg`），不受影响（`19b` `:6`；真终端本刀没测）。
  - 有选区时：HEAD 上这样的终端里按 Ctrl-V，选区先被删掉、再读剪贴板；补丁之后选区与文字都不动（上面两条实测）。按裁决不加用例。
- **同类**：
  - HEAD 全仓库的 `*_test.go` 里没有用例喂 Ctrl-V：在仓库根对 `*_test.go` grep `'v'|x16|\\u0016|ctrl\+v|KeyMap\.Paste`（`x16`、`\u0016` 两种是原始字节 0x16 的写法），无命中；对全仓库 grep `ModCtrl.*'v'|'v'.*ModCtrl|tui1bCtrl\('v'\)|ctrl\+v|KeyMap\.Paste`，只命中 `docs/19b` `:6`。
  - textarea 里交回 `pasteMsg` 的只有 `Paste` 这一个函数，引用它的只有 `:1371` 一处；读剪贴板的只有 `:2013` 一处。`CopySelection`（`ctrl+shift+c`，`:125`、`:1428-1431`、`:1827-1840`）是**写**剪贴板；Council 的 Ctrl-C 判据 `msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c'`（`model.go:369`、`:390`）在它之前截走带 Ctrl 的 `c`，textarea 收不到。v1 时这句是按代码读的；v1.1 用合成消息实测了（§0 R5、§2 F5：列表开着只收起、不在时进空闲 Ctrl-C 分支），真终端送进来什么形状仍未测（§7 (e)）。
  - Council 的 Model 里只有一个 textarea（`model.go:95`），没有别的 bubbles 组件。

### R1 带修饰键的回车与 Tab

- **机制**：B4 的截获（`model.go:385`）`case msg.Code == tea.KeyTab, msg.Code == tea.KeyEnter:` 不看修饰键；HEAD 既有的回车分支（`:410`）`case msg.Code == tea.KeyEnter:` 也不看修饰键。Shift+Tab 在 bubbletea v2 里是 `tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}`：ultraviolet `decoder.go:484-485` 把 `CSI Z` 解成它、`key_table.go:98` 同一个映射、`key_test.go:1072-1078` 的用例名就叫 `shift+tab`；原型实测 `.String() == "shift+tab"`。Shift+Enter、Alt+Enter 写成 `{Code: tea.KeyEnter, Mod: tea.ModShift}`、`{Code: tea.KeyEnter, Mod: tea.ModAlt}`，`.String()` 分别是 `"shift+enter"`、`"alt+enter"`（实测）。真终端会不会把这三个键区分开送进来，本刀没测（§7 (c)）。
- **HEAD 上 = 原型上（实测，本刀不改这段行为，只加担保）**：敲 `hi @br`（行 == `["> @bravo  Bravo"]`）再按键，把返回的 Cmd 用 `tui1bRun` 跑到底：三个键都是值 `"hi @bravo "`、行 nil、帧 8、`Submit` 0 次、产出里没有 `submitDone`（产出只有 textarea 的光标闪烁消息）。
- **对照（列表不在时的既有行为，实测冻结）**：同样敲 `hi @br`，先按 Esc（行 nil），再按同一个键：Shift+Enter、Alt+Enter 都**提交**（`Submit` 1 次、收到 `"hi @br"`，产出 `[print:> hi @br submitDone]`，值 `""`）；Shift+Tab 什么都不做（值 `"hi @br"`、`Submit` 0 次、产出为空）。
- **缺口**：把 B4 的截获收窄成只认无修饰键（§4 A52），全部 TestAtx 照绿（原型实测：只有本刀的 F2 红）；那时列表开着按 Shift+Enter、Alt+Enter 会掉进 `:410` 真的提交 `"hi @br"`，Shift+Tab 什么都不做、列表仍开着。

### R2 Ctrl-T 与 A41

- **机制**：Ctrl-T 是 textarea 的 `TransposeCharacterBackward`（`textarea.go:116`），`transposeLeft`（`:886-897`）：光标在行尾时先左移一格，对调光标左右两个字，再右移一格。敲 `  @`（两个空格加 `@`，光标列 3 = 行尾）按 Ctrl-T：对调下标 1、2，值变成 ` @ `，光标回到列 3。`@` 从下标 2 挪到下标 1，下标 2 成了空格。
- **HEAD 上 = 原型上（实测）**：敲 `  @` → 行 == `ALL`（前提）；按 `tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}` → 值 `" @ "`、`m.ta.Line() == 0`、`m.ta.Column() == 3`、行 nil、帧 8（返回的 Cmd 是 nil：光标列没变，没有闪烁 Cmd；§2 不断言它）。收起是 `atRecheck` 的 `line[m.at.col] != '@'`（`model.go:575`）判的：`m.at.col` 是 2，那一格是空格。
- **双变异（A54）**：`:575` 去掉 `|| line[m.at.col] != '@'`、同时 `:579` 的左边界判据换成 `if false {`：原型实测 Ctrl-T 之后列表仍是 `ALL`（前缀仍是空串）、帧 14；全部 TestAtx 照绿，只有本刀 F3 红。
- **各自单独去掉都等价（原型实测全部 TestAtx 与 TestAtf 存活；理由见 §4「判定为等价」的 A41、A55）**：同一步里只去掉 `@` 那一处，左边界判据看到下标 1 是 `@`（`MentionBoundaryOK('@')` 为假，`@` 在 `mentionLeftForbidden` 里，`internal/repl/mention.go:105-113`），收起；只去掉左边界那一处（即 A41），`@` 那一处看到下标 2 是空格，收起。

### R3 四处过时注释

HEAD 上与 AT1 之后的行为不符的四段（`19b` `:9`），逐条按代码核：

- `termHeight`（`:100-102`）写「只给退出清屏用……不参与 View」。实际用在三处：`run.go:161-164` 的 `exitClearSeq`、`model.go:521-526` 的 `boardLimit`（决定看板能不能长高，进 `chrome()`，进 View）、`:624`、`:631` 的 `atLines`（决定列表显示几行，进 View）。
- `chrome`（`:482-483`）写「View 与 frameHeight 共用它：往活区里加一行只改这里」。实际还有 `atCursor`（`:475`）与 `atLines`（`:631`）用它的行数；列表在 textarea 下方，不在 `chrome()` 里；而且 `boardLimit`（`:525`）写死了 `2 + m.ta.Height()`，往表头与横线之外再加固定行时它也要改——这一点在 AT1 之前就不完全对。
- `frameHeight`（`:541`）写「固定行数加 textarea 的高度」。实际（`:544`）还加 `len(m.atLines())`。
- `beginSubmit`（`:682-684`）写提示留到「成功提交、粘贴、空闲时 Ctrl-C」。按 `model.go` 里全部 `m.status =`（`:345`、`:349`、`:354`、`:358`、`:398`、`:402`、`:412`、`:436`、`:691`、`:704`）核：提示挂着时一定空闲（`beginSubmit` 在 `:410-418` 只有不忙、不在收尾窗口时才进），这时能改写状态栏的只有成功提交（`:704`）、粘贴（`:436`）、列表不在屏幕上时的 Ctrl-C（`:402`）；列表在屏幕上时 Ctrl-C 被 B4 截走、只收起（`:369-371`），列表那六个键（`:367-388`）都不写 `m.status`。

原型上把四段换成 §1.1 的新注释，`gofmt -l` 为空，全量测试照绿（只改注释）。

### R4 等价清单

`19b` `:10-16` 列的六条（七处）在原型上逐条实测：全部 TestAtx 与本刀 TestAtf（`./internal/tui ./internal/repl`）下存活。v1.1 在 69 行的用例集上逐条重跑，E4 的两半也各自单独跑了，全部存活（§4）。理由与 X / MX 编号的对应逐条写在 §4「判定为等价」，不加用例。

### R5 列表开着时带修饰键的 ↑、↓、Esc 与 Ctrl+Shift+C（v1.1 新加，W1 F2 / 原 §7 (b)）

- **机制**：B4 截获的另外四个键同样不看多余的修饰键：`case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':`（HEAD `:369`）、`case msg.Code == tea.KeyEscape:`（`:372`）、`case msg.Code == tea.KeyUp:`（`:375`）、`case msg.Code == tea.KeyDown:`（`:380`）。`19b` D7 冻结的就是「不看修饰键」，今天的代码是对的，只是没担保。键的形状与 `.String()`（原型实测，整串冻结）：`{Code: tea.KeyUp, Mod: tea.ModShift}` = `"shift+up"`、`{Code: tea.KeyUp, Mod: tea.ModAlt}` = `"alt+up"`、`{Code: tea.KeyDown, Mod: tea.ModShift}` = `"shift+down"`、`{Code: tea.KeyDown, Mod: tea.ModAlt}` = `"alt+down"`、`{Code: tea.KeyEscape, Mod: tea.ModShift}` = `"shift+esc"`、`{Code: tea.KeyEscape, Mod: tea.ModAlt}` = `"alt+esc"`、`{Code: 'c', Mod: tea.ModCtrl | tea.ModShift}` = `"ctrl+shift+c"`。`shift+up`、`shift+down` 是 textarea 的 `SelectLineUp` / `SelectLineDown`（`textarea.go:122-123`、`:1418-1425`）；`ctrl+shift+c` 是 textarea 的 `CopySelection`（`:125`，**写**剪贴板，`:1827-1840`）；`alt+up`、`alt+down`、`shift+esc`、`alt+esc` 在 textarea 的默认键表里没有绑定（`:89-127`）。
- **HEAD 上 = 原型上（实测，本刀不改这段行为，只加担保）**：新 Model 敲 `@b`（行 == `B3`，帧 11）再按键：
  - Shift+↓、Alt+↓：Cmd nil，值 `"@b"`，行 == `["  @b1", "> @beta   贝塔", "  @bravo  Bravo"]`（第二项选中，下称 `B3sel1`），帧 11；
  - 先按无修饰 ↓（行 == `B3sel1`），再按 Shift+↑、Alt+↑：Cmd nil，值 `"@b"`，行 == `B3`，帧 11；
  - Shift+Esc、Alt+Esc：Cmd nil，值 `"@b"`，行 nil，帧 8；
  - Ctrl+Shift+C：Cmd nil（**只判 nil，没有执行**），值 `"@b"`，行 nil，帧 8，表头 `council 0.0.0-test   空闲` 不变。
- **对照（列表不在时的既有行为，实测冻结）**：新 Model 敲 `@b`、按 Esc（行 nil），↑ 那两个键之前同样先按一次无修饰 ↓，再按同一个键：
  - 六个键（Shift / Alt 的 ↑、↓、Esc）：Cmd nil，值 `"@b"`，光标 (0, 2)，行 nil，帧 8，没有选区。单行输入里 `SelectLineUp` / `SelectLineDown` 不动光标、不起选区；`alt+up`、`alt+down` 在 textarea 里没绑定、`msg.Text` 为空，什么都不插；Esc 两个键到不了 textarea，走 Council 空闲的 Esc 分支（`:420`，什么都不做）。
  - Ctrl+Shift+C：进空闲的 Ctrl-C 分支（`:390`）：Cmd 是 `tea.Quit`（`reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()`，**没有执行**），表头 `council 0.0.0-test   正在退出`，值 `"@b"`。
- **缺口（实测）**：四处各自收窄（§4 A57-A60，即 W1 的 O5-O8），v1 的全部 TestAtx 与 TestAtf 存活（原型实测：`-run` 只挑 v1 的函数，四条各 61 条 PASS 行、零 FAIL）。收窄之后列表开着时（原型探针实测）：
  - A57、A58：带修饰键的 ↑、↓ 掉进 textarea，单行里什么都不做，选中项不动（↑ 之后行仍是 `B3sel1`，↓ 之后行仍是 `B3`）；
  - A59：带修饰键的 Esc 掉进空闲的 Esc 分支，列表不收（行 == `B3`、帧 11）；
  - A60（Ctrl-C 收窄成 `msg.Mod == tea.ModCtrl`）：Ctrl+Shift+C 掉进空闲的 Ctrl-C 分支，Cmd 是 `tea.Quit`（比指针，没有执行），表头 `正在退出`，行仍 == `B3`、帧 11——**列表开着时按 Ctrl+Shift+C 直接退出程序**。不花钱。
- **同类（实测，不加用例）**：Ctrl+Alt+C（`{Code: 'c', Mod: tea.ModCtrl | tea.ModAlt}`，`"ctrl+alt+c"`）与 Ctrl+Shift+C 读数相同：列表开着只收起（Cmd nil），列表不在时 Cmd 是 `tea.Quit`；A60 下列表开着时 Cmd 是 `tea.Quit`、行仍 == `B3`。A60 已由 F5 杀，不另加。加上 R1 的 Tab、回车，列表的六个键（`19b` D7）现在都有带修饰键的担保。HEAD 全仓库的 `*.go` 里没有别的用例或代码喂这几种带修饰键的键（grep `ctrl\+shift\+c|CopySelection|ModCtrl \| tea\.ModShift|ModShift \| tea\.ModCtrl|KeyUp, Mod|KeyDown, Mod|KeyEscape, Mod` 无命中）。
- **真终端（未测）**：Windows Terminal 默认自己占用 Ctrl+Shift+C（复制），也常占 Shift+↑ / Shift+↓，这些键在 WT 里可能根本到不了程序。合成消息的用例是担保，真机只是看看（§6 第 5 步、§7 (c)）。

---

## 1. 范围

### 1.1 做

本刀的生产代码**全部逐字冻结**：只动 `internal/tui/model.go`，五个补丁（锚点逐字取自 HEAD `4908f13`，含行首 tab，每个在文件里恰好出现一次，互不重叠；按文件里的先后顺序列）。编号接着 AT1 的 B1-B11：B12a-B12d 是 R3 的四段注释，B13 是 R0 那一行。

**B12a** 锚点（HEAD `:100-103`）：

```go
	// termHeight 是最近一条 WindowSizeMsg 带来的真终端高度。
	// 那条消息的宽或高 <= 0 时它是 0。上面的 height 会被兜底成 24，不能拿来判「终端高度已知」。
	// 只给退出清屏用（run.go 的 exitClearSeq），不参与 View。
	termHeight int
```

替换成：

```go
	// termHeight 是最近一条 WindowSizeMsg 带来的真终端高度。
	// 那条消息的宽或高 <= 0 时它是 0。上面的 height 会被兜底成 24，不能拿来判「终端高度已知」。
	// 用在三处：退出清屏（run.go 的 exitClearSeq）、看板行数上限（boardLimit）、@ 补全列表的行数（atLines）。
	// 后两处决定 View 画几行：它为 0 时看板行数不再增长、补全列表不上屏。
	termHeight int
```

**B13** 锚点（HEAD `:171-172`）：

```go
	// Enter 由外层提交；多行输入只通过 Ctrl-J 插入。
	ta.KeyMap.InsertNewline.Unbind()
```

替换成：

```go
	// Enter 由外层提交；多行输入只通过 Ctrl-J 插入。
	ta.KeyMap.InsertNewline.Unbind()
	// Ctrl-V 不交给 textarea 读剪贴板：它的 Paste 读完之后交回一条未导出的 pasteMsg，
	// 那条消息走 updateInput 最后的 default 分支，不经 tea.PasteMsg 那一支，
	// 既不收起 @ 补全列表，也不把状态栏换成「粘贴了 N 个字符」（X44 的提示靠这一步换掉）。
	// 解绑之后粘贴只剩终端的括号粘贴（tea.PasteMsg）一条路；终端把 Ctrl-V 原样交给程序时，它什么都不做。
	ta.KeyMap.Paste.Unbind()
```

**B12b** 锚点（HEAD `:482-484`）：

```go
// chrome 返回活区里 textarea 上方的固定行：表头、横线，再加 floor 行看板。
// View 与 frameHeight 共用它：往活区里加一行只改这里，帧高跟着变。
func (m Model) chrome() []string {
```

替换成：

```go
// chrome 返回活区里 textarea 上方的行：表头、横线，再加 floor 行看板。
// View、frameHeight、atCursor、atLines 都从它取这几行或它的行数；textarea 下方的 @ 补全列表不在这里（见 atLines）。
// 往表头与横线之外再加固定行时，boardLimit 里写死的 2（表头与横线）要跟着改。
func (m Model) chrome() []string {
```

**B12c** 锚点（HEAD `:541-543`）：

```go
// frameHeight 是 View 的行数：固定行数加 textarea 的高度。
// 不数 View 里的换行：退出清屏要的是「帧有几行」，与帧里写了什么无关。
func (m Model) frameHeight() int {
```

替换成：

```go
// frameHeight 是 View 的行数：chrome 的行数、textarea 的高度，再加 @ 补全列表的行数（列表不在屏幕上时为 0）。
// 不数 View 里的换行：退出清屏要的是「帧有几行」，与帧里写了什么无关。
func (m Model) frameHeight() int {
```

**B12d** 锚点（HEAD `:682-685`）：

```go
// 多行输入里第 2 行起有斜杠命令时不提交（repl.HasLaterCommandLine）：输入框原样保留、
// 不回显、不进忙，只把状态栏换成 oneCommandHint。这句提示留到下一次改写状态栏的事件
// （成功提交、粘贴、空闲时 Ctrl-C）为止；打字、tick、窗口尺寸变化都不动它。
func (m Model) beginSubmit() (tea.Model, tea.Cmd) {
```

替换成：

```go
// 多行输入里第 2 行起有斜杠命令时不提交（repl.HasLaterCommandLine）：输入框原样保留、
// 不回显、不进忙，只把状态栏换成 oneCommandHint。这句提示留到下一次改写状态栏的事件
// （成功提交、粘贴、@ 补全列表不在屏幕上时的空闲 Ctrl-C）为止；打字、tick、窗口尺寸变化都不动它，
// @ 补全列表在屏幕上时归列表的六个键（Ctrl-C、Esc、上、下、Tab、回车）也不动它。
func (m Model) beginSubmit() (tea.Model, tea.Cmd) {
```

- 落地后 `model.go` 共 782 行（HEAD 774 行）：B12a 多 1 行、B13 多 5 行、B12b 多 1 行、B12c 不变、B12d 多 1 行。
- `import` 不变。除这五处以外 `model.go` 一个字节都不改；`run.go`、`roundobs.go`、`writer.go`、`vt_*.go`、`internal/repl`、`internal/app`、`cmd/`、`go.mod`、`go.sum` 一个字节都不改。
- B13 必须是 `Unbind()`，不是 `SetEnabled(false)`：两者对按键的效果相同（`key.Matches` 只认 `Enabled()` 为真的绑定），但用例分得开（v1.1 改，原来写的「用例分不开、只在审查时看」不对）：`SetKeys` 不清 `disabled`，B13 写成 `SetEnabled(false)` 时 F1 第 5 步的前提（`SetKeys("ctrl+v")` 之后 `Enabled()` 为真）Fatalf，原型实测（§4 A56）。也不是给 `Paste` 换键：换成 `SetKeys("ctrl+shift+v")` 时 F1 第 2 步红（原型实测，只红这一步；不登记进 §4）。放在局部变量 `ta` 上、`return Model{…}` 之前（textarea 是值类型，`Model` 里存的是这时的拷贝）。

### 1.2 不做（两条腿都不许越界）

- 不改任何代码逻辑：不改 B4 的截获（R1、R5 只加担保，今天的行为是对的）；不改 `atRecheck`、`atArm`（R2、R4 只登记变异与理由）；不给 Ctrl-V 加提示或别的行为（§7 (d)）。
- 不改 `docs/`（归档与 DECISIONS 由主代理在落地后做，§6）。
- **既有 `_test.go` 一个字节都不许改（含注释），本刀零授权**：`atx_*_test.go`、`tui1b_*`、`rba_*`、`rbb_*` 以及其它全部既有测试文件。原型实测不需要改任何一个（全量 `./...` 全绿）。
- 不为 §4「判定为等价」的变异加用例。原 §7 (b) 的同类（↑、↓、Esc、Ctrl-C 带修饰键）按裁决在本刀做成 F4、F5；除此之外不为同类发现加用例（例如 §0 R5 的 Ctrl+Alt+C）。
- 测试腿不写生产代码，实现腿不写 `_test.go`。测试腿不加 `TestMain`。

---

## 2. 验收（测试腿新写的文件）

### 2.0 通用纪律

- 新文件一个：`internal/tui/atf_review_test.go`，`package tui`，不带构建标签。五个测试函数（名字冻结）：`TestAtfCtrlVUnbound`（F1）、`TestAtfModifiedEnterTab`（F2，三个子测试 `shift_enter`、`alt_enter`、`shift_tab`）、`TestAtfCtrlTAtLeftEdge`（F3）、`TestAtfModifiedListKeys`（F4，六个子测试 `shift_up`、`alt_up`、`shift_down`、`alt_down`、`shift_esc`、`alt_esc`）、`TestAtfCtrlShiftCClosesList`（F5，没有子测试）。
- **包级标识符一律以 `atf` 开头**。只读复用同包既有辅助、不许改它们：
  - `atx_model_test.go`：`atxMembers`、`atxCands`、`atxNew`（返回 `(Model, *tui1bSubmitter)`）、`atxALL`、`atxB3`、`atxH`、`atxWantLines`、`atxWantValue`、`atxWantFrame`、`atxWantHead`、`atxWantNilCmd`、`atxNoSubmitDone`；
  - `tui1b_model_test.go`：`tui1bType`、`tui1bStep`、`tui1bRun`（返回 `tui1bTrace`，读字段 `log`）、`tui1bEsc`、`tui1bSubmitter`（方法 `count`、`text`）。
- 可以读 `Model` 的未导出字段 `ta`（`ta.Value()`、`ta.Line()`、`ta.Column()`、`ta.KeyMap.Paste.Enabled()`）与方法 `atLines()`、`frameHeight()`、`chrome()`（经 `atxWantHead`）。**写 `m.ta` 只许一种写法、只在 F1 第 5、6 步**（v1.2 改成限定写法）：这两步的正对照里各调一次 `m.ta.KeyMap.Paste.SetKeys("ctrl+v")`；别处一律不写，第 5、6 步里也不许有别的写法（不调 `SetEnabled`、不整体赋值 `m.ta.KeyMap.Paste` 或 `m.ta.KeyMap`，理由见 §2.1 第 5 步）。不许读 `m.at`、`m.members`。
- 可以 import `charm.land/bubbles/v2/textarea`（只用 `textarea.New`、`textarea.New().KeyMap.CopySelection.Keys()` 与 `textarea.Paste` 的函数值；`textarea.New()` 只许拿来读，即 F1 第 1 步的 `textarea.New().KeyMap.Paste.Enabled()` 与 F5 第 1 步的 `CopySelection.Keys()`，不许把它的 `KeyMap` 或其中的绑定赋给 `m.ta`）、`reflect`、`council/internal/repl`（F3 的对照候选要 `repl.MentionCandidate`）。`tea.Quit` 只许当函数值拿来比指针。
- **剪贴板禁令（硬性）**：
  - Ctrl-V 按下之后返回的 Cmd **只许判 nil、或用 `reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(textarea.Paste).Pointer()` 比函数指针**；不许调用它（`cmd()`）、不许交给 `tui1bRun`、不许 `tea.Batch` / `tea.Sequence` 之后再跑、不许起 `tea.Program` 按 Ctrl-V、不许调用 `textarea.Paste`。
  - **Ctrl+Shift+C（textarea 的 `CopySelection`，写剪贴板）按下之后返回的 Cmd 同样待遇**：只许判 nil、或用 `reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()` 比函数指针；不许调用、不许交给 `tui1bRun`、不许起 `tea.Program` 按它、不许调用 `tea.Quit`。
  - 不许写任何读或写剪贴板的代码（`github.com/atotto/clipboard` 之类一律不 import）。
  - **Cmd 变量只许三种用法（v1.2，W1b finding 1）**：Ctrl-V 与 Ctrl+Shift+C 按下之后拿到的 Cmd（`tui1bStep` 或 `Update` 的第二个返回值），拿到之后在文件里出现的每一处只许是：
    1. 与 nil 比较（`== nil`、`!= nil`）；
    2. `reflect.ValueOf(...)` 的唯一实参，紧跟 `.Pointer()`；
    3. 传给 `atxWantNilCmd`（`atx_model_test.go:143-148`，函数体只有 `if cmd != nil { t.Errorf(...) }`，消息里不带 cmd）。
    原样赋给另一个变量、原样当 atf 助手的返回值可以，去处同样受这条约束。除此之外一律不许：不当 `Errorf` / `Fatalf` / `Logf` / `Sprintf` 之类的实参（要报就报 `cmd == nil` 这个布尔值），不传给 `tui1bRun` 或别的既有助手，不放进 `tea.Batch` / `tea.Sequence`，不存进切片、map 或结构体。测试腿自己写的 atf 助手若接收这个 Cmd，函数体里对那个形参的每一处用法同样只许这三种。**失败分支同样适用**：绿跑走不到失败分支，所以「跑绿了」证明不了失败分支合规，要逐处读。
  - **先审计、后跑（v1.2，W1b finding 1）**：测试腿在第一次 `go test` 之前，按上一条逐处核对自己的文件，并核：没有 `cmd()` 式的调用、没有 `textarea.Paste(`、没有 `tea.Quit(`、没有剪贴板包的 import、`tui1bRun` 只在 F2 里。尤其要在下面「HEAD 上的预期」那次运行之前做完：那次 F1 第 3、4 步走进失败分支，手里拿着的正是 `textarea.Paste`。交付时报告核对结果：逐个列出 Ctrl-V、Ctrl+Shift+C 的 Cmd 变量与它的每一处用法（行号加用法属于三种里的哪一种）。
  - 全文件只有 F2 调 `tui1bRun`（v1.2 按实际改写，W1b finding 4）：第 1 段跑列表开着时那三个键（Shift+Enter、Alt+Enter、Shift+Tab）补全返回的 Cmd，产出只有 textarea 的光标闪烁（v1.2 原型实测三个键都是 `["other:cursor.BlinkMsg" "other:cursor.blinkCanceled" "other:cursor.blinkCanceled"]`）；第 2 段跑列表不在时同样三个键返回的 Cmd：Shift+Enter、Alt+Enter 是提交的 Sequence（`beginSubmit` 末尾的 `tea.Sequence(cmds...)`，HEAD `:727`、落地后 `:735`；里面是假 Println 与调假 `Submit` 的 `submitCmd`，产出 `["print:> hi @br" "submitDone"]`），Shift+Tab 返回的是 nil（`tui1bRun` 什么都不跑，产出为空）。这几个 Cmd 都不经 Paste、CopySelection 绑定。F4、F5 一律不调 `tui1bRun`（F4 的键返回 nil，不用跑；F5 禁止跑）。
- **每一段各起一个新 Model，不许从同一个 Model 分叉**：`Model` 是值，但 textarea 的缓冲是共享的切片，拷贝之后两边的编辑互相看得见。原型的第一版从同一个 Model 分别敲 `x` 与 `@b`，第二段的值读成了 `"@bx"`。下文每个「新 Model」都是一次 `atxNew`。
- 断言整串（整列）相等。每条用例注释写【判别力】：能不能为假、只差一处的对照是哪条。缺席断言都配同装置的正对照。
- 不可见字符用十六进制码位写；源码里不放这些字符本身，不用反斜杠 u 转义（本刀的用例用不到不可见字符）。
- **HEAD 上的预期**：本文件只引用 HEAD 已有的名字，在 HEAD 上**编译得过**；原型实测（v1.1，把 `model.go` 换回 HEAD 字节跑 `-run '^TestAtf'`）：F1 红（三处：第 2 步 `Paste` 应当已解绑、第 3 步列表不在时 Cmd 非 nil、第 4 步列表开着时 Cmd 非 nil；第 5 步的前提在 HEAD 上成立，不红），F2、F3、F4、F5 绿（它们担保的是今天已经对的行为，杀手在 §4）。**这次运行之前必须先做完上面的「先审计、后跑」**（第 3、4 步的失败分支拿着 `textarea.Paste`）。测试腿交付时报告这一读数与审计结果。

**夹具**：`atxMembers`、`atxALL`、`atxB3` 照 AT1（`atx_model_test.go:33-59`）。「新 Model」= `atxNew(t, 80, 24, atxCands(atxMembers))`，F3 的对照另给候选。F4 另要一列 `B3sel1`（`@b` 之后选中项在第二项，整列见 §3；测试腿自己写成 `atf` 开头的包级变量，名字自定）。

### 2.1 F1 `TestAtfCtrlVUnbound`（R0）

1. 前提：`textarea.New().KeyMap.Paste.Enabled() == true`（bubbles 默认是启用的；不成立就 `Fatalf`）。
2. 新 Model：`m.ta.KeyMap.Paste.Enabled() == false`。
3. 新 Model，敲 `x`，按 `tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}`：Cmd == nil；值 `"x"`。
4. 新 Model，敲 `@b`：行 == `atxB3`（前提）；按 Ctrl-V：Cmd == nil；值 `"@b"`；行 == `atxB3`。
5. 正对照（列表不在）：新 Model，`m.ta.KeyMap.Paste.SetKeys("ctrl+v")`；前提 `Enabled() == true`（不成立就 `Fatalf`；**Fatalf 的消息写明这一步也拦 B13 写成 `SetEnabled(false)`**，原型用的是 `第 5 步（前提）：SetKeys 之后 Paste 应当启用（New 里若用 SetEnabled(false) 代替 Unbind，SetKeys 不清 disabled，这里就红）`，措辞测试腿可改，意思不许丢）；敲 `x`，按 Ctrl-V：Cmd 是 `textarea.Paste`（函数指针相等，**不调用**）；值 `"x"`。**正对照的写法只许一种（v1.2 收紧，W1b finding 2）**：对 `m.ta.KeyMap.Paste` 只调这一次 `SetKeys("ctrl+v")`，前提 `Enabled()` 紧跟在它之后判；任何位置都不许调 `SetEnabled`（之前、之后都不许），不许整体赋值 `m.ta.KeyMap.Paste` 或 `m.ta.KeyMap`（从 `textarea.New()` 拷过来也不许）。这几种写法会把 A56 留下的 `disabled` 翻掉或连绑定一起换掉，对照照绿、A56 被放过（§0 R0，v1.2 原型实测四种）。
6. 正对照（列表开着）：新 Model，`SetKeys("ctrl+v")`（写法同第 5 步，只许这一种）；敲 `@b`：行 == `atxB3`（前提）；按 Ctrl-V：Cmd 是 `textarea.Paste`；值 `"@b"`；行 == `atxB3`。
7. 第 3、4 步两个 Model 的 `sub.count() == 0`。

【判别力】第 2 步与第 1 步只差「经没经过 `New`」；第 3、4 步的「Cmd 为 nil」与第 5、6 步只差 `SetKeys` 那一行，第 5、6 步证明 Ctrl-V 这个键确实到了 textarea、Paste 绑定在时 Cmd 就是 `textarea.Paste`，所以 nil 不是「键根本没到」。A53（去掉 `Unbind`）让第 2、3、4 步红（原型实测，三处）。A56（`Unbind()` 换成 `SetEnabled(false)`）下第 2、3、4 步照绿（`Enabled()` 为假、Cmd 为 nil，与 `Unbind` 一样），只有第 5 步的前提 Fatalf（原型实测）：第 5 步既是正对照，又是 A56 的杀手，所以它的前提不许弱化，正对照也只许第 5 步写明的那一种写法（别的写法翻掉或换掉 `disabled`，A56 就活了，§0 R0 实测四种）。不用 `SetEnabled(true)` 当对照的原因见 §0 R0（实测它恢复不了）。第 4 步的「行仍 `B3`」与「值不变」在 A53 下照绿（`Paste` 返回之前什么都没改），杀手是 Cmd 那一句。第 7 步见 §7 (i)。

### 2.2 F2 `TestAtfModifiedEnterTab`（R1，三个子测试）

键：`shift_enter` = `tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}`；`alt_enter` = `tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}`；`shift_tab` = `tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}`。每个子测试两段：

1. **列表开着**：新 Model，敲 `hi @br`：行 == `["> @bravo  Bravo"]`（前提）；按键，把返回的 Cmd 用 `tui1bRun` 跑到底：产出里没有 `submitDone`（`atxNoSubmitDone`）；`sub.count() == 0`；值 `"hi @bravo "`；行 nil；帧 8。
2. **对照（列表不在）**：另起新 Model，敲 `hi @br`，按 Esc：行 nil（前提）；按同一个键，`tui1bRun` 跑它的 Cmd：
   - `shift_enter`、`alt_enter`：`sub.count() == 1`、`sub.text(0) == "hi @br"`；产出里有 `submitDone`；值 `""`；行 nil。
   - `shift_tab`：`sub.count() == 0`；产出为空（`len(tr.log) == 0`）；值 `"hi @br"`；行 nil。

【判别力】第 1 段「没提交」是缺席断言，对照是第 2 段同装置（只差列表开没开）真的提交（X44 教训：先跑 Cmd 再读计数）。A52（B4 只截无修饰键）下原型实测（v1.1 重跑）：`shift_enter`、`alt_enter` 第 1 段产出 `[print:> hi @br submitDone]`、`Submit` 1 次、值 `""`；`shift_tab` 第 1 段值 `"hi @br"`、行 `["> @bravo  Bravo"]`、帧 9；三个子测试全红，全部 TestAtx 照绿。`shift_tab` 的对照证明列表不在时 Shift+Tab 什么都不做，所以第 1 段的 `"hi @bravo "` 只能来自截获。

### 2.3 F3 `TestAtfCtrlTAtLeftEdge`（R2）

1. 新 Model，敲 `  @`（两个空格、`@`）：行 == `atxALL`（前提）。
2. 按 `tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}`（Cmd 不断言）：值 `" @ "`；`m.ta.Line() == 0` 且 `m.ta.Column() == 3`；行 nil；帧 8。
3. 对照：候选 `[]repl.MentionCandidate{{ID: "ab"}, {ID: "ba"}}` 的新 Model（`atxNew(t, 80, 24, 这份候选)`），敲 `  @ab`：行 == `["> @ab"]`（前提）；按 Ctrl-T：值 `"  @ba"`；光标 (0, 5)；行 == `["> @ba"]`。

【判别力】第 2 步「行 nil」是缺席断言：前提（第 1 步）是同一个 Model 在按这一键之前列表开着；值与光标证明 Ctrl-T 到了 textarea；第 3 步证明 Ctrl-T 本身不收列表（只对调前缀里的两个字、`@` 与它左边不动时照开），所以第 2 步的收起来自 `@` 被挪走。A54（`@` 那一处与左边界那一处同时去掉）下原型实测：第 2 步行 == `atxALL`、帧 14，F3 红，全部 TestAtx 照绿。只去掉其中一处（A41、A55）F3 照绿，见 §4。

### 2.4 F4 `TestAtfModifiedListKeys`（R5，六个子测试）

键（形状与 `.String()` 见 §0 R5）：`shift_up` = `tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}`；`alt_up` = `tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt}`；`shift_down` = `tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}`；`alt_down` = `tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt}`；`shift_esc` = `tea.KeyPressMsg{Code: tea.KeyEscape, Mod: tea.ModShift}`；`alt_esc` = `tea.KeyPressMsg{Code: tea.KeyEscape, Mod: tea.ModAlt}`。这几个键没有剪贴板绑定。每个子测试两段：

1. **列表开着**：新 Model，敲 `@b`：行 == `atxB3`（前提）。`shift_up`、`alt_up` 两个子测试先按一次无修饰的 ↓（`tea.KeyPressMsg{Code: tea.KeyDown}`）：行 == `B3sel1`（前提）。按键：Cmd == nil（`atxWantNilCmd`）；值 `"@b"`；行与帧：
   - `shift_up`、`alt_up`：行 == `atxB3`；帧 11；
   - `shift_down`、`alt_down`：行 == `B3sel1`；帧 11；
   - `shift_esc`、`alt_esc`：行 nil；帧 8。
2. **对照（列表不在）**：另起新 Model，敲 `@b`，按 Esc：行 nil（前提）。`shift_up`、`alt_up` 两个子测试同样先按一次无修饰的 ↓。按同一个键：Cmd == nil；值 `"@b"`；`m.ta.Line() == 0` 且 `m.ta.Column() == 2`；行 nil；帧 8。

【判别力】第 1 段的「行」能为假，是杀手：A57、A58 下带修饰键的 ↑、↓ 掉进 textarea，单行里 `SelectLineUp` / `SelectLineDown` 什么都不做，选中项不动：`shift_up`、`alt_up` 的行仍是 `B3sel1`，`shift_down`、`alt_down` 的行仍是 `atxB3`（原型实测，各红两个子测试）。↑ 的两个子测试必须先按一次无修饰 ↓：选中项在第一项时 ↑ 本来就不动（到头不回绕），不先下移，A57 下行照样是 `atxB3`，分不开。无修饰 ↓ 走列表分支，A57 不碰它。Esc 两个子测试的「行 nil」「帧 8」是缺席断言，前提是同一个 Model 按键之前行 == `atxB3`；A59 下它们掉进空闲的 Esc 分支（什么都不做），行仍 == `atxB3`、帧 11（原型实测，两个子测试红）。第 2 段是同装置对照（只差列表开没开）：列表不在时这六个键不动值、不动光标、不开列表，所以第 1 段里选中项的移动与列表的收起只能来自列表分支。第 1 段的 Cmd == nil 在 A57-A59 下照绿（textarea 什么都没做时不交回 Cmd；空闲 Esc 分支返回 nil；原型探针实测），杀手是「行」那一句。

### 2.5 F5 `TestAtfCtrlShiftCClosesList`（R5）

键：`tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModShift}`（下称 Ctrl+Shift+C）。**本用例按下它之后返回的 Cmd 一律不执行**（§2.0 剪贴板禁令）。

1. 前提（不成立就 `Fatalf`）：这个键的 `.String() == "ctrl+shift+c"`；`textarea.New().KeyMap.CopySelection.Keys()` 整列 == `[]string{"ctrl+shift+c"}`。
2. 列表开着：新 Model，敲 `@b`：行 == `atxB3`（前提）；按 Ctrl+Shift+C：Cmd == nil（只判 nil）；行 nil；值 `"@b"`。
3. 对照（列表不在）：另起新 Model，敲 `@b`，按 Esc：行 nil（前提）；按 Ctrl+Shift+C：Cmd 是 `tea.Quit`（`reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()`，**不执行**；Cmd 为 nil 时也算红，不许先判非 nil 就放过）；表头 == `atxH("正在退出")`；值 `"@b"`。

【判别力】第 2 步的「Cmd == nil」与「行 nil」都能为假：A60（列表分支的 Ctrl-C 收窄成 `msg.Mod == tea.ModCtrl`）下 Ctrl+Shift+C 掉进空闲的 Ctrl-C 分支，Cmd 是 `tea.Quit`（原型探针比指针，没执行）、行仍 == `atxB3`、帧 11、表头「正在退出」，即列表开着时按 Ctrl+Shift+C 直接退出程序；F5 第 2 步两处红（原型实测）。第 3 步是同装置对照（只差列表开没开）：证明这个键确实到了 Council 的 Ctrl-C 判据、列表不在时就是退出，所以第 2 步的 nil 不是「键没到」。第 3 步能比 `tea.Quit` 指针，前提是 `atxNew` 用的是 `tui1bNew(t, nil)`：Drainer 是什么都不产出的假件，`quitCmd` 里没有要先打印的行，`tea.Sequence` 只剩 `tea.Quit` 一条时把它原样交回（`tui1b_model_test.go` 里 `tui1bRun` 上方注释引的 bubbletea `commands.go:36-54`）；换成会产出行的 Drainer，Cmd 就不再是 `tea.Quit` 本身，这条断言会假红。第 1 步证明这个键在 textarea 里就是 `CopySelection`：Council 截走它，textarea 才收不到、才不写剪贴板（写不写剪贴板本身不测，禁令）。不断言 `Submit` 次数：两步都不执行 Cmd，而今天的 `Submit` 只在 Cmd 里调用，计数恒为 0，那句断言拦不住本刀关心的任何改法（§7 (i)）。

---

## 3. 冻结的字面量

- **本刀没有新的用户可见字面量**：B13 与 B12 只加注释，生产代码不新增任何字符串。
- 注释里的中文字面照 §1.1 逐字。
- 用例要比对的既有文本（HEAD 原样，不是本刀新增）：补全列表的行（`atxALL`、`atxB3`、`"> @bravo  Bravo"`、`"> @ab"`、`"> @ba"`，格式见 `19b` §3.1）、Submit 收到的 `"hi @br"`、补全后的值 `"hi @bravo "`。
- v1.1 新加（同样是 HEAD 原样的既有文本，原型实测，整串冻结）：
  - `B3sel1`（F4）：`["  @b1", "> @beta   贝塔", "  @bravo  Bravo"]`（与 `atxB3` 只差选中标记 `> ` 在第二行；`@b1` 那一行没有尾随空格，`@beta` 之后三个空格，`@bravo` 之后两个空格）；
  - F5 第 3 步的表头：`atxH("正在退出")`，即 `"council 0.0.0-test   正在退出"`；
  - F5 第 1 步：`"ctrl+shift+c"`（键的 `.String()`）与 `[]string{"ctrl+shift+c"}`（`CopySelection.Keys()`）。

---

## 4. 集成员的变异清单

跑法：`go test -count=1 -v -run '^(TestAtx|TestAtf)' ./internal/tui ./internal/repl`。锚点在**落地后**的 `model.go` 里恰好一次（下表同时给 HEAD 行号与落地后行号），从字节副本还原（不用 `git checkout`），还原后 SHA256 与跑前相同。记号同 `19b` §4：`<TAB>` 是一个 tab，`<NL>` 是换行，表里的 `\|` 是 markdown 转义。「预期被谁杀」是原型实跑变红的用例；「TestAtx」一栏是同一次跑里全部 TestAtx 的结果。不变异时 69 条 PASS 行（v1.1 的用例集；v1 是 61 行）。v1.1 在 69 行的用例集上把八条逐条重跑，PASS 行数：A52 65、A53 68、A54 68、A56 68、A57 66、A58 66、A59 66、A60 68；每条都只红点名的那个函数（或那几个子测试），TestAtx 零 FAIL。A59、A60 的锚点是两行（v1.2 按实测改写理由，W1b finding 3）：不带行首 tab 按行 grep 时，`case msg.Code == tea.KeyEscape:` 与 `case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':` 各有两处（列表分支三个 tab、既有分支两个 tab：落地后 `:378` 与 `:426`、`:375` 与 `:396`；HEAD `:372` 与 `:420`、`:369` 与 `:390`）；带上三个 tab 的单行按字节串找已经恰好一次（v1.2 原型实测，落地后与 HEAD 都是）。这里仍用两行锚点（同样实测恰好一次），是为了让不数 tab 的核对也只命中一次。W1b 报告只用三个 tab 的单行打这两条变异，读数与两行锚点相同（本版没有重跑这一条）。

| # | 文件 | 变异 | 预期被谁杀 | TestAtx |
|---|---|---|---|---|
| A52 | model.go（B4，HEAD `:385`，落地后 `:391`） | `<TAB><TAB><TAB>case msg.Code == tea.KeyTab, msg.Code == tea.KeyEnter:` → `<TAB><TAB><TAB>case msg.Code == tea.KeyTab && msg.Mod == 0, msg.Code == tea.KeyEnter && msg.Mod == 0:` | **F2 三个子测试**（`shift_enter`、`alt_enter` 提交了 `"hi @br"`；`shift_tab` 列表没收、值没补全） | 全绿 |
| A53 | model.go（B13，落地后 `:178`） | 删掉整行 `<TAB>ta.KeyMap.Paste.Unbind()<NL>`（注释留着） | **F1** 第 2、3、4 步 | 全绿 |
| A54 | model.go（`atRecheck`，两处同时改：HEAD `:575` / 落地后 `:582`，HEAD `:579` / 落地后 `:586`） | `<TAB>if col <= m.at.col \|\| m.at.col >= len(line) \|\| col > len(line) \|\| line[m.at.col] != '@' {` → `<TAB>if col <= m.at.col \|\| m.at.col >= len(line) \|\| col > len(line) {`；**并且** `<TAB>if m.at.col > 0 && !repl.MentionBoundaryOK(line[m.at.col-1]) {` → `<TAB>if false {` | **F3** 第 2 步（行 == `ALL`、帧 14） | 全绿 |
| A56 | model.go（B13，落地后 `:178`） | `<TAB>ta.KeyMap.Paste.Unbind()<NL>` → `<TAB>ta.KeyMap.Paste.SetEnabled(false)<NL>` | **F1 第 5 步的前提**（Fatalf：`SetKeys` 之后 `Enabled()` 仍是 false；第 2、3、4 步照绿） | 全绿 |
| A57 | model.go（B4 的 ↑，HEAD `:375`，落地后 `:381`） | `<TAB><TAB><TAB>case msg.Code == tea.KeyUp:` → `<TAB><TAB><TAB>case msg.Code == tea.KeyUp && msg.Mod == 0:` | **F4 `shift_up`、`alt_up`** 第 1 段（行仍是 `B3sel1`，不是 `atxB3`） | 全绿 |
| A58 | model.go（B4 的 ↓，HEAD `:380`，落地后 `:386`） | `<TAB><TAB><TAB>case msg.Code == tea.KeyDown:` → `<TAB><TAB><TAB>case msg.Code == tea.KeyDown && msg.Mod == 0:` | **F4 `shift_down`、`alt_down`** 第 1 段（行仍是 `atxB3`，不是 `B3sel1`） | 全绿 |
| A59 | model.go（B4 的 Esc，HEAD `:372-373`，落地后 `:378-379`） | `<TAB><TAB><TAB>case msg.Code == tea.KeyEscape:<NL><TAB><TAB><TAB><TAB>m.at = atState{}` → `<TAB><TAB><TAB>case msg.Code == tea.KeyEscape && msg.Mod == 0:<NL><TAB><TAB><TAB><TAB>m.at = atState{}` | **F4 `shift_esc`、`alt_esc`** 第 1 段（行 == `atxB3`、帧 11） | 全绿 |
| A60 | model.go（B4 的 Ctrl-C，HEAD `:369-370`，落地后 `:375-376`） | `<TAB><TAB><TAB>case msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'c':<NL><TAB><TAB><TAB><TAB>m.at = atState{}` → `<TAB><TAB><TAB>case msg.Mod == tea.ModCtrl && msg.Code == 'c':<NL><TAB><TAB><TAB><TAB>m.at = atState{}` | **F5 第 2 步**（Cmd 非 nil：原型探针比指针是 `tea.Quit`，没执行；行 == `atxB3`） | 全绿 |

**判定为等价、不列杀手的变异**（原型实测：全部 TestAtx 与 TestAtf 存活；v1 时 61 条 PASS 行，v1.1 在 69 行的用例集上逐条重跑，仍是 69 条、零 FAIL，与不变异时相同；按代码论证）。`19b` 文件头（`:8`、`:10-16`）列的是 A41 与六条（七处）。

**编号对应（v1.1 核实，关掉原 §7 (a)）**：依据 AT1 W2 的两份原始报告（主代理摘录在 `E:\multi_model_cowork\at1fu_source_findings.md`：测试腿的 `contract_gaps`，独立审查的 R2、R4）。逐条按变异内容对上：

| 本文编号 | 原始报告编号 | 变异 |
|---|---|---|
| E1 | 测试腿 X1 | `atArm` 去掉 `m.ta.HasSelection()` |
| E2 | 测试腿 X2 | `atLines` 的 `sel := min(max(m.at.sel, 0), len(list)-1)` → `sel := m.at.sel` |
| E3 | 测试腿 X3 | `atAccept` 的 `list[min(max(m.at.sel, 0), len(list)-1)]` → `list[m.at.sel]` |
| E4 | 测试腿 X4 | `atArm` 去掉 `col < 1 \|\| col > len(line) \|\|`（两处一起，报告里也是一条） |
| E5 | 审查 R4 的 MX2 | 删掉 `c.Blink = true` |
| E6 | 审查 R4 的 MX4 | `atRecheck` 去掉 `m.at.col >= len(line) \|\|` |
| E7 | 审查 R4 的 MX5 | `mentioncand` 去掉 `sumID != "" &&` |
| A55 | 审查 R2 的 MX3 | `atRecheck` 去掉 `line[m.at.col] != '@'`（Ctrl-T 下由左边界兜着） |
| A54 | 审查 R2 的「两处一起去掉」 | MX3 与 A41 同时去掉（本刀 F3 杀） |

`19b` 文件头第 5 条的六条（七处）就是 E1-E7；MX3 在第 3 条（A41 的理由）里，本文登记为 A55。审查 R4 里的 MX6（`make([]MentionCandidate, 0, len(parts))` 改成 `var out []MentionCandidate`）是对照，被 `TestAtxCandidates` 杀掉，不是等价变异，不登记。测试腿报告里说 X2 在合成路径上可杀（补全中喂一条更短的 `submitDoneMsg`），契约没有这条路，理由见下面 E2、E3。

- **A41**（HEAD `:579`，落地后 `:586`）`<TAB>if m.at.col > 0 && !repl.MentionBoundaryOK(line[m.at.col-1]) {` → `<TAB>if false {`。**`19b` §4 原来的理由（「@ 左边那个字改不了」）不成立，改为**：补全中 `@` 左边那个字能被改动的唯一路子是光标在行尾、`@` 就在光标左边一格时按 Ctrl-T（`transposeLeft` 对调光标左边两个字，`textarea.go:886-897`），这一下同时把 `@` 挪走、把原来左边那个字放到 `@` 的位置，`:575` 的 `line[m.at.col] != '@'` 就收起（左边那个字不可能是 `@`：当初能开，它就过了 `MentionBoundaryOK`，`@` 在禁止集合里）。其余编辑碰不到它：插入只在光标处（光标在 `@` 右边）；退格、`ctrl+w`、`ctrl+u` 删的是一段以光标结尾的连续字，删得到左边那个字就一定连 `@` 一起删掉，光标落到 `@` 的列或更左，`:575` 的 `col <= m.at.col` 收起；`ctrl+k`、Delete、`alt+u` / `alt+l` / `alt+c` 只动光标右边；有选区时第一条判据收起；`tea.PasteMsg` 一律收起；textarea 自己的 `pasteMsg` 这条路 B13 之后不存在了。所以由 `@` 那一处兜底。**与 A55 一起去掉就不等价了**（A54，F3 杀）。
- **A55**（HEAD `:575`，落地后 `:582`）只去掉 `@` 那一处：`<TAB>if col <= m.at.col \|\| m.at.col >= len(line) \|\| col > len(line) \|\| line[m.at.col] != '@' {` → `<TAB>if col <= m.at.col \|\| m.at.col >= len(line) \|\| col > len(line) {`。补全中 `m.at.col` 那一格的 `@` 被换掉只有这几种：被删掉（光标随之落到 `@` 的列或更左，`col <= m.at.col` 收起）；光标在行尾、`@` 在光标左边一格时 Ctrl-T，`@` 挪到左边一格，左边界判据（`:579`）看到 `@` 收起（这正是 F3 那一步；`@` 在行首时这一行只有一个字，`transposeLeft` 在 `len < 2` 时什么都不做）；`@` 在光标左边两格且光标在行尾时 Ctrl-T，或光标紧贴 `@` 右边、不在行尾时 Ctrl-T，`@` 挪到前缀里，前缀循环（`:583-588`）遇到非 token 字符 `@` 收起。所以由左边界与前缀循环兜底。
- **E1** `atArm` 的选区判据（HEAD `:549`，落地后 `:556`）`<TAB>if m.busy \|\| m.finalizing \|\| m.ta.HasSelection() {` → `<TAB>if m.busy \|\| m.finalizing {`：`atArm` 只在按键 default 分支、textarea 处理完 `msg.Text == "@"` 的键之后调用（`:427-429`）；这个键落在 textarea 的 default 分支（textarea 的键表里没有绑 `@` 的键），那里先 `deleteSelection()`（`textarea.go:1437`，它在末尾 `ClearSelection()`，`:1783-1818`）再插字，所以调用 `atArm` 时一定没有选区。
- **E2** `atLines` 对 sel 的夹取（HEAD `:636`，落地后 `:643`）`<TAB>sel := min(max(m.at.sel, 0), len(list)-1)` → `<TAB>sel := m.at.sel`；**E3** `atAccept` 对 sel 的夹取（HEAD `:664`，落地后 `:671`）`<TAB>pick := list[min(max(m.at.sel, 0), len(list)-1)]` → `<TAB>pick := list[m.at.sel]`：列表在屏幕上时 `0 <= sel < len(atMatches())` 恒成立。`sel` 开补全时是 0；前缀变了归 0（`:589-592`）；↑ 只在 `> 0` 时减（`:376`），↓ 只在 `< len(m.atMatches())-1` 时加（`:381`）；筛选结果只随前缀与 `members` 变，前缀一变 `sel` 就归 0，`members` 只在 `submitDoneMsg` 里换（`:340`），而补全中不会来 `submitDoneMsg`（开补全要求不忙、不在收尾窗口；每条 `submitDoneMsg` 都来自一次 `beginSubmit`，`beginSubmit` 在 `:702` 收起补全）。所以两处夹取都是恒等。
- **E4** `atArm` 的列判据（HEAD `:554`，落地后 `:561`）`<TAB>if col < 1 \|\| col > len(line) \|\| line[col-1] != '@' {` → `<TAB>if line[col-1] != '@' {`：`col > len(line)` 不可能（textarea 的光标列总在 `[0, 本行长度]` 里，`atLine` 取的就是光标那一行）；`col < 1` 只在刚才那个 `@` 没插进去时才会发生，而 Council 的 textarea 没有设字数上限（`CharLimit` 默认 0 = 不限，`textarea.go:32`、`:407`、`:549-553`；`MaxContentHeight` 默认 0），插一个字不会被丢，插完光标列至少是 1。**依赖 bubbles 默认值与 Council 不设 `CharLimit`**：将来设了字数上限，满了之后在行首敲 `@` 时这个变异会越界 panic，届时它就不等价了（§7 (g)）。两半也各自单独跑过：只去掉 `col < 1 \|\|`（→ `<TAB>if col > len(line) \|\| line[col-1] != '@' {`）、只去掉 `col > len(line) \|\|`（→ `<TAB>if col < 1 \|\| line[col-1] != '@' {`），v1 用例集上各 61 条 PASS（W1 实测），v1.1 用例集上各 69 条 PASS（本版原型实测），都存活，都在上面的理由之内。
- **E5** `c.Blink = true`（HEAD `:477`，落地后 `:483`）：删掉整行 `<TAB>c.Blink = true<NL>`。`ta.Cursor()` 已经把 `Blink` 设成 `m.styles.Cursor.Blink`（`textarea.go:1774`），bubbles 的默认样式里它是 `true`（`:464`），Council 没有改 textarea 的样式。**依赖 bubbles 默认 `Blink: true`**：升级 bubbles 时与 AT1 的 V1 一起复核。
- **E6** `atRecheck` 的 `m.at.col >= len(line)`（HEAD `:575`，落地后 `:582`）`<TAB>if col <= m.at.col \|\| m.at.col >= len(line) \|\| col > len(line) \|\| line[m.at.col] != '@' {` → `<TAB>if col <= m.at.col \|\| col > len(line) \|\| line[m.at.col] != '@' {`：纯逻辑蕴含。走到 `line[m.at.col]` 时前两条已经为假，即 `m.at.col < col <= len(line)`，`m.at.col < len(line)` 必然成立，不会越界。
- **E7** `internal/repl/mentioncand.go:36` 的 `<TAB><TAB>if sumID != "" && p.AgentID == sumID {` → `<TAB><TAB>if p.AgentID == sumID {`：只在某个成员的 id 是空串时才有区别。库里的成员行只经 `conversation.Service.Join`（`internal/conversation/service.go:101-113`）写入，它的调用方是 `/team use`（`internal/repl/commands.go:713`）、`/add`（`:928`）、总结成员自动加入（`summary.go:105`，此前 `sumID == ""` 已在 `:76` 返回）、`/add +编号`（`addmodel.go:470`），传的 id 都来自配置里的 agent 表，而配置的 agent id 必须匹配 `^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$`（`internal/config/validate.go:25`、`:64`，至少一个字符）。按可达性等价（手改库文件不算）。

**只在审查时看（没有用例能抓，集成员收割时逐字核对）**：

- 五段注释与 §1.1 逐字相同（注释里没有与代码不符的话，是本刀的全部目的）。
- `model.go` 除五个补丁之外零改动：`git diff --stat` 只有 `internal/tui/model.go`（13 行增、5 行删）与新测试文件。
- （v1.1 移走 v1 的第一条「B13 是 `Unbind()` 不是 `SetEnabled(false)`、不是换键」：`SetEnabled(false)` 由 F1 第 5 步的前提拦（A56），换键由 F1 第 2 步拦（§1.1 末条），都实测过；位置由 §6 第 1 步落盘后 `model.go` 的 SHA256 逐字节核对管。v1 的最后一条「↑↓ 带修饰键见 §7 (b)」也移走：已登记为 A57-A60。）

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- **只许动一个文件**：`internal/tui/model.go`。不写任何 `_test.go`，不改别的文件。
- 生产改动很小（五个补丁，净增 8 行），**一次交付**。
- 每个补丁一个块，共五个，按 §1.1 的顺序（B12a、B13、B12b、B12c、B12d）：

  ```
  ===PATCH: internal/tui/model.go===
  <<<<ANCHOR
  （从 HEAD 真文件里原样复制的若干行，含行首 tab，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点就是 §1.1 给出的锚点块（派工时的 main HEAD；写本文时是 `4908f13`），替换文本就是对应的替换块，**逐字照抄**（含注释、空行、行首 tab）。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。代码必须能 `gofmt`。
- 锚点在派工时的 HEAD 上必须各恰好一次；若不是（HEAD 变了），不要自己改锚点，在 REPORT 里写明。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（本刀注释里的 `「」`、`（）`、`；`、`、` 都是）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。
- 不许新增任何标识符、任何字符串字面量、任何 import。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：五个补丁是否都给了，各落在哪个块。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：有没有新增任何字符串字面量或标识符（应当没有）。
  ```

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；五个锚点各恰好一次后落盘；`gofmt -l internal/tui` 为空；`go vet ./internal/tui`；`go build ./...`；`git diff --stat` 只有 `model.go`；逐字节核对 §1.1（落盘后 `model.go` 的 SHA256 应为 `6d868927a5b0cb8514f95c27d761f0c9587b7c212b913e02fd0cfd956c73206e`，前提是派工时的 `model.go` 仍是 HEAD `4908f13` 那一份，SHA256 `c937571c90e213040e7cec201af23b8e4b375824c746caadec530f5ec02513bf`）。
2. **合并测试腿的 `atf_review_test.go`**。**先做剪贴板审计，再跑任何测试**（v1.2 从第 3 步移到这里，W1b finding 1）：按 §2.0「Cmd 变量只许三种用法」逐处核对 Ctrl-V 与 Ctrl+Shift+C 返回的 Cmd 变量，范围包括 `Errorf` / `Fatalf` / `Logf` 的实参与它经过的每个 atf 助手的函数体（传给 `atxWantNilCmd` 是许可的）；并核：全文件没有 `cmd()` 式的调用、没有 `textarea.Paste(`、没有 `tea.Quit(`、没有剪贴板包的 import；`tui1bRun` 只在 F2 里，参数只许是 F2 那三个键（Shift+Enter、Alt+Enter、Shift+Tab）在两段里返回的 Cmd；F1 第 5、6 步对 `m.ta.KeyMap.Paste` 只有 `SetKeys("ctrl+v")`，没有 `SetEnabled`、没有整体赋值（§2.1 第 5 步）；与测试腿交付时报告的审计结果对一遍。**审计不过，测试与变异一条都不跑**，退回测试腿改。原因：第 3 步的 A53 让 F1 第 3、4 步走进失败分支、手里拿着 `textarea.Paste`，A60 让 F5 第 2 步拿着 `tea.Quit`；绿跑走不到这些分支，暴露不出失败分支里的违规。审计过了再跑：只跑 `./internal/tui ./internal/repl`（另一条流水线也可能在跑 go test；3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制），再跑一次全量 `./...`；红在别的包先单独重跑那个包再判。既有用例必须全绿（原型实测全绿、零改动）。
3. **§4 的八条变异逐条跑**（A52、A53、A54、A56、A57、A58、A59、A60；等价清单里的不跑）；从字节副本还原，不用 `git checkout`。A59、A60 用两行锚点（§4）。任何一条没被点名杀手杀掉，先判是用例空、还是契约写错。剪贴板审计在第 2 步、第一次 `go test` 之前已经做过（v1.2）；变异期间若改过测试文件（本刀不应当改），改完重新审计再跑。
4. **文档**：
   - `docs/19b_AT1_MENTION_COMPLETION_CONTRACT.md` 文件头「落地后订正」：
     - 第 1 条（D7 / Ctrl-V，`:6`）末尾标「已由 AT1-FU 修（`<提交>`）：`New` 里解绑 `ta.KeyMap.Paste`，F1 担保」；
     - 第 2 条（带修饰键的回车，`:7`）标「已由 AT1-FU 修：F2 担保，§4 A52；列表其余四个键（↑、↓、Esc、Ctrl-C）带修饰键的担保见 AT1-FU F4、F5，§4 A57-A60」；
     - 第 3 条（A41 理由，`:8`）标「已由 AT1-FU 修：理由改写见 AT1-FU §4，双变异登记为 A54，F3 杀」；
     - 第 4 条（四处注释，`:9`）标「已由 AT1-FU 修：B12a-d」；
     - 第 5 条（等价清单，`:10-16`）标「已由 AT1-FU 登记：AT1-FU §4 E1-E7（编号与 X / MX 的对应见 AT1-FU §4「编号对应」表）」；
     - 第 6、7 条（`:17-20`）本刀不涉及，不标。
   - `docs/00_DECISIONS.md` #47「AT1 落地记录」之后补一行「AT1-FU 落地记录」：解绑 Ctrl-V（代价：终端把 Ctrl-V 原样交给程序时它什么都不做；有选区时也不再先删选区）、五个新测试函数（F1-F5）、八条新变异（A52-A54、A56-A60）、等价清单（E1-E7 与 X / MX 的对应），真机复验情况。
   - 本契约归档进 `docs/`（按现有编号体例，例如 `19c_AT1FU_MENTION_FOLLOWUP_CONTRACT.md`，避开 CH1 的归档名；文件头加归档说明，写明行号指 HEAD `4908f13`）。
5. **给 Griffin 的真机步骤**（可选；本刀的行为改动只有 Ctrl-V 一处，终端接管粘贴时看不出差别。Windows Terminal + PowerShell 5.1；未实测）。**第 1、2 项要先复制一段字再按 Ctrl-V，会经过剪贴板；这两项当时默认不做，做不做由主代理定（§7 (j)，v1.2）。** 其余各项都不碰剪贴板：
   1. 启动 TUI，复制一段文字（不含换行），在输入框里按 Ctrl-V：预期照常粘进来，表头状态显示 `粘贴了 N 个字符`（这是 Windows Terminal 自己接管 Ctrl-V、发括号粘贴的那条路，本刀没动它）。
   2. 选一个有成员的会话，敲 `@b`（列表出现）之后按 Ctrl-V 粘一段字：预期列表收起、字粘进来、状态栏同上。如果粘不进来、或列表没收起，请告诉我（说明你的终端把 Ctrl-V 原样交给了程序，本刀之后它什么都不做）。
   3. 列表开着时按 Shift+Enter：预期只补全、不提交（与回车一样）。列表不在时按 Shift+Enter：预期提交（与回车一样）。
   4. （v1.1 加，v1.2 改）列表开着时按 Shift+↓：预期选中项下移一格（与 ↓ 一样）；按 Shift+Esc 或 Alt+Esc：预期只收起。Windows Terminal 常占 Shift+↑ / Shift+↓，这些键在 WT 里可能根本到不了程序（看不出任何变化就是这种情况，不算坏）。合成消息的用例（F4）是担保，这一步只是看看。**Ctrl+Shift+C 不在真机步骤里**（v1.2，W1b finding 5）：它由 F5 的合成消息担保；WT 默认自己占用它（复制，写剪贴板；本刀没测），占用时按了也到不了程序、看不出本刀的行为，所以真机不测。
6. 清理本刀在中转根与 scratchpad 里的全部中间物。

---

## 7. open items

### 待主代理

- (a) **已关（v1.1）**：X1-X4、MX2-MX5 与 E1-E7 按 AT1 W2 原始报告摘录（`E:\multi_model_cowork\at1fu_source_findings.md`）逐条对上，写在 §4「编号对应」表：X1-X4 = E1-E4，MX2 = E5，MX4 = E6，MX5 = E7，MX3 = A55，审查 R2 的「两处一起去掉」= A54；没有落在这些之外的一条。E4 两半各自单独跑过（v1 用例集上各 61 条 PASS，W1 实测；v1.1 用例集上各 69 条 PASS，本版原型实测），都存活，都在 E4 的理由之内。
- (b) **已做（v1.1）**：↑、↓、Esc、Ctrl-C 带修饰键的担保按裁决在本刀一并做了：§0 R5、§2 F4 与 F5、§4 A57-A60（W1 的 O5-O8），每条实测点名杀手变红。只加测试，生产代码不动。
- (c) **真终端没测（留作真机步骤，可选）**：Shift+Enter、Alt+Enter、Shift+Tab 在 Windows Terminal 里会不会以带修饰键的形状送进来（而不是被终端吃掉或变成别的序列），本刀没测；F2 用的是合成消息，形状按 ultraviolet 的解码表（§0 R1）。F4、F5 的键同样只用了合成消息；Windows Terminal 默认自己占用 Ctrl+Shift+C（复制），也常占 Shift+↑ / Shift+↓，这些键在 WT 里可能根本到不了程序。合成消息的用例是担保，真机只是看看（§6 第 5 步第 3、4 项）。Ctrl+Shift+C 不列入真机步骤（v1.2，W1b finding 5：WT 下没有诊断价值，还会写剪贴板）。
- (e) **`CopySelection`（`ctrl+shift+c`）写剪贴板**：Council 的 Ctrl-C 判据（`Contains(ModCtrl) && Code == 'c'`）先截走，textarea 收不到。v1.1 用合成消息 `{Code: 'c', Mod: tea.ModCtrl | tea.ModShift}` 实测了这一点（F5：列表开着只收起、Cmd nil；列表不在时进空闲 Ctrl-C 分支、Cmd 是 `tea.Quit`；都没有执行 Cmd），A60 登记了收窄的后果。仍没测的：真终端里 Ctrl+Shift+C 送进来的是不是这个形状（WT 默认自己占用它）。v1.2 起真机也不安排测它（§6 第 5 步第 4 项：WT 默认占用它时到不了程序，按了还会写剪贴板）。它是写不是读，不在 R0 的范围里。
- (f) **`19b` 文件头第 6 条（§2.0 的两处说法）** 是纯文档订正，本刀不涉及；主代理落地、归档时改。
- (g) **E4、E5 依赖 bubbles 的默认值**（`CharLimit` 0、`MaxContentHeight` 0、`Styles.Cursor.Blink` true）：升级 bubbles，或 Council 将来给 textarea 设字数上限 / 换样式时，这两条要重判，E4 届时会变成可达的越界 panic。
- (h) v1 原型的全量 `./...` 分两次调用各跑一次；v1.1 原型的全量 `./...` 一次调用跑了一次，九个包全绿。变异表只在 `./internal/tui ./internal/repl` 上跑。
- (i) **（v1.1 新登记，只登记、不改）F1 第 7 步恒真**：第 3、4 步按了 Ctrl-V 之后不执行 Cmd（禁令），而今天的 `Submit` 只在 `submitCmd` 返回的 Cmd 被执行时调用（HEAD `model.go:760-762`），所以这两个 Model 的 `sub.count()` 恒为 0：只有「在 `Update` 里同步调 `Submit`」这类改法才能让它变红，按 Ctrl-V 的任何写法都不会。它不是 v1.1 的裁决范围，本版不删不改；F5 没有照抄它（§2.5 末句）。要不要在归档时删掉或注明，由主代理定。
- (j) **（v1.2 新登记，待主代理定）§6 第 5 步第 1、2 项的去留**：W1b finding 5 只点了 Ctrl+Shift+C；照同一条理由问「同类」，第 1、2 项要 Griffin 先复制一段字、再在 TUI 里按 Ctrl-V，同样经过剪贴板。这两项是本刀唯一行为改动（Ctrl-V）的真机观察，不做就没有 Ctrl-V 的真机观察（第 3、4 项看的是别的键）。本版取保守读法：步骤原文保留，标「默认不做」。可选的处理：维持默认不做；以后再做；或者换到另一台机器上做。由主代理定。

### 待 Griffin

- (d) **Ctrl-V 的代价没有提示（低优先）**：终端把 Ctrl-V 原样交给程序时，解绑之后按 Ctrl-V 什么都不做、状态栏也不说。主代理已定**不加提示**：Windows Terminal 默认自己接管 Ctrl-V，走不到这条路。若你用的终端会把 Ctrl-V 原样交给程序、又想要一句提示，再开一刀。
