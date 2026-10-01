# S10D2 冻结契约：/log 的标记列（v1.1，2026-09-27）

> **归档说明（2026-09-27）**：这是 Step 10 D 之二（`13` §7.2 `/log` 标记列）派工时的冻结契约原文 v1.1，从中转根 `E:\multi_model_cowork\s10d2_contract.md` 原样归档。文中行号指 HEAD `92dfc89`；落地提交 `a2e3d76`。文中的 `E:\` 路径是当时的中转与隔离目录，已清理。
> **落地后的订正**：独立审查只报了两处注释措辞（cmdLog 注释「标记只接在占位符之后」、`runFakeProviderCLI` 的文档注释没提新分支），落地时已改。变异 35 种写法全杀、6 条等价确认，无新缺口。

## v1.1 相对 v1 的改动

基线：主仓库 HEAD 已是 `750c2d4`，相对 v1 引用的 `92dfc89` 只多了 `internal/app/s10c2_crash_e2e_test.go` 与文档，所有生产 `.go` 文件逐字节相同，本契约正文里以 `92dfc89` 为准的生产代码行号与锚点仍然有效，正文里的 `92dfc89` 不改。§1.1 的生产改动在 v1.1 里**一个字没变**，变的只有验收、种子、变异清单与说明。v1.1 新增与改动的全部期望值在隔离拷贝 `E:\Isolating_test\council\s10d2_v11\` 里用原型（HEAD `750c2d4` + §1.1 的实现 + §2.0 的授权改动 + §2 的全部用例，含 v1.1 新增的）实跑得出，v1 的全部期望行在同一份原型上逐字节复现；原型不交付、已删除。

裁决来源：标记体例由 Griffin 2026-09-27 裁定（英文键名体例照 v1 冻结）；两处对 13 §7.2 的偏离由主代理裁定认可；其余改动来自 W1 验证的 1 条 major 与 6 条 minor。

1. **开头的偏离声明从一处改成两处**：(a) 措辞冻结，推翻 13 §7.2「措辞不进契约」；(b) E 不限 system 消息，是 §7.2「system 消息那一行末尾打 `Meta["event"]`」的超集。
2. **开头「期望值来源」那一条**补上 v1.1 的来源（`s10d2_v11`）。
3. **§1.1 A2**：「`meta` 为 nil 时返回空串」这条防御保留，补一句：没有任何 store 会把 nil 传进新函数，§3.4 第 17 行并没有测到这条路。
4. **§2 通用纪律「HEAD 上的状态」**：新增 E1c，在 HEAD 上应红的条数从 6 改成 7；并注明这项清点在 `750c2d4` 上做（同包已有 `s10c2_crash_e2e_test.go`，名字不撞这条约束在那里才真正被检验）。
5. **§2.1 新增 E1c `TestS10dE2EEmptyTextThoughtDroppedMarked`**（W1 major）：grok 原件把 `"text": "OK",` 换成 `"text": "",`，`store_thought = false`，期望 seq 3 那一行是 `- [thought_dropped]`；前提守卫含「替换之后的字节与原件不相等」与 `meta["completion"] == "empty"`。
6. **§2.2**：种子表从 18 条改成 20 条；L1 的 Kind 分配改成「第 1-17 条与第 19 条及以后是 system，第 18 条是 user」；L1 / L2 断言里「紧跟其后的 18 行」改成 20 行；L1 的【判别力】补第 18、19、20 行分别杀什么（L2 的【判别力】不变）。
7. **§3.0** 理由 1 里「L1 的 18 行」改成 20 行（同一个计数）。
8. **§3.3**：新增 E1c 的两个期望行 `G3E_OFF` / `G3E_PLAIN`，以及 E1c 的库内消息顺序（与 E1a 相同，agent 那一行仍是 seq 3）。
9. **§3.4**：第 18 行的 Meta 改成三键俱全（`event` / `thought_dropped` / `denied_actions`），期望行带三个标记，对照改成 15；**追加**第 19 行（正文空串，`event` + `thought_dropped`，W1 major 的杀手）与第 20 行（`denied_actions` 是 map，不标）；不插队、不重新编号，§4 里对第 1-18 行的引用全部不变。表下补第 18、19、20 行的说明与 `TestServiceDoesNotMarkDroppedWhenThoughtAlreadyEmpty` 的引用。
10. **§4**：表头补「预期被谁杀」是下界那一句；#2 补第 19、20 行；#6 补第 19 行与 E1c；新增 #29「正文为空时只打 D」、#30「D 也认 map」；等价清单补 X2（E 用 `strings.Fields` 重新拼接）与 X5（agent 消息上不打 E）。
11. **§6**：第 3 步的红绿清点跟着改（E1c 应红，条数 7）；第 4 步的变异条数从 28 改成 30。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `92dfc89`。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`。本机无 gcc，不跑 `-race`。
- 来源：`docs/13_STEP10_CONTRACT.md` §7.2（`/log` 的标记列）。相关节：§6.2（C2 说「`/log` 能看见 `turn_lost` 面包屑」，证据是 `cmdLog` 与 SQL 没有 kind 谓词）；§3.8（settle 失败不拦 REPL，理由是「用户连 `/log` 都进不去才更糟」）；§3.11 / 3.11a / 3.11b（启动横幅的首行、两个数、对账先于漂移审计）；§4 / 4a（event 取值注释那两处）；§9（D 片依赖 A 片）。
- 之后改动了来源的订正：`docs/README.md` 路线图 Step 10 行（A 之一到之四、C1、C3、D 之一已落；A 之五、C2、D 之二、D 之三未落）；`docs/00_DECISIONS.md` #9(a) 第 8 条（`thought_dropped` 没有任何读者，`:544-546`）与同条的「第三条剩余项」（`app.go` 那行 `svc.SetThoughtPolicy(cfg.StoreThought)` 删掉之后全套仍绿，`:483-492`）。
- **本契约对 §7.2 的两处偏离**（主代理 2026-09-27 已裁定认可）：
  - (a) §7.2 写「措辞不进契约」。本契约把三个标记的字面量**逐字节冻结**（理由见 §3.0；体例由 Griffin 2026-09-27 裁定：英文键名体例，`" [event=<值>]"`、`" [thought_dropped]"`、`" [denied_actions]"`，顺序 E、T、D，每个前面恰好一个 ASCII 空格）。
  - (b) §7.2 写「system 消息那一行末尾打 `Meta["event"]`」。本契约的 E **不看消息的 Kind**（§1.1 A2），是 §7.2 的超集：今天只有 system 面包屑写 `event`（`internal/turn/runner.go:1212-1214` 的 `postSystem`、`conversation/service.go:142-166` 的 `BindTeam`、`internal/turn/recover.go:220-237` 的 `lostNoticeMessage`（turn_lost，直接组 Meta、不经 `postSystem`）；`agentMeta` `:1271-1320` 不写），生产上与「只对 system 打」等价，少一个分支。
  - 落地后主代理在 13 §7.2 补订正，两处都写（§6 第 5 步）。
- **§9 的前置已满足**：D 片依赖 A 片（面包屑要在 `/log` 里看得见）。`turn_lost` 面包屑由 A 之四（`9e85bea`）的 `lostNoticeMessage`（`internal/turn/recover.go:216-240`）写入，今天就在库里。A 之五（§3.8 的闸与 `/session` 列）未落，不阻塞本刀。
- **与本批另一刀是否相交**：本刀的生产改动只有 `internal/repl/commands.go` 一个文件（`cmdLog` 的一格、一个新的小函数、import 两行）。A 之五按 §3.8 / 3.11 落在 `internal/turn`（Ask 在 `BeginTurn` 之前的检查）、`conversation.SessionInfo` 与 `internal/repl/ask.go` 的 `cmdSession`，**文件不相交**。唯一可能相交的是测试侧：本刀授权改 `internal/app/smoke_test.go` 的 `runFakeProviderCLI`（§1.2、§2.0），如果同批有另一刀（例如 C2）也要改 app 包的假 CLI，两处要串行落地。
- **schema**：本刀不碰 store，不需要任何迁移。主代理现查的 `idx_inflight_turns_conv_agent`（`internal/store/schema.go:404`）与本刀无关，结论不变。
- 本契约的全部期望值（`/log` 的每一整行、库里的 `thought` / `meta`、全量 `go test ./...` 的结果）已在隔离拷贝 `E:\Isolating_test\council\s10d2_draft\` 里用一份原型实现实跑得出，原型不交付、已删除。v1.1 新增与改动的期望值（§3.3 的 `G3E_OFF` / `G3E_PLAIN`、§3.4 的第 18-20 行、§4 的 #29 / #30 与改动过的杀手）在 `E:\Isolating_test\council\s10d2_v11\`（HEAD `750c2d4`）里用第二份原型实跑得出，v1 的全部期望行在这份原型上逐字节复现；这份原型同样不交付、已删除。

---

## 0. 病根 / 动机（行号以 HEAD 92dfc89 为准）

1. **`/log` 一个 Meta 键都不读。** `internal/repl/commands.go:1121-1167` 的 `cmdLog` 打六列 `SEQ SENDER KIND TO BRAIN TEXT`，循环（`:1156-1165`）里只用了 `Seq`、`SenderID`、`Kind`、`Recipients`、脑子三列与 `Text`。于是：
   - `thought_dropped`：`internal/conversation/service.go:67-79` 的 `applyThoughtPolicy` 在丢掉非空 thought 时写这个键。`00_DECISIONS.md` #9(a) 第 8 条（`:544-546`）自己记着「`thought_dropped` 今天没有任何读者」，「不许静默丢」只兑现了机器可读那一半。
   - `denied_actions`：`internal/turn/runner.go:1298-1302`（`agentMeta`）与 `:1331-1337`（`attachSessionMeta`，`turn_failed` / `post_rejected` 面包屑）写这个键。`:1299-1300` 的注释写着「重启之后 /log 也要能看到」，今天兑现不了：屏幕上只有 `/ask` 那一刻 `internal/repl/ask.go:470-489` 的 `deniedActionsNote` 说一次。
   - `event`：全集 8 个值。`internal/turn/runner.go:29-35` 的 7 个（`session_opening`、`turn_failed`、`post_rejected`、`session_rebuilt`、`turn_lost`、`round_started`、`round_finished`），加 `internal/conversation/types.go:85` 的 `team_bound`（`BindTeam` 写，`service.go:142-166`）。面包屑的正文是中文描述，读的人看不出是哪一种事件。
2. **Meta 的值有两种形状，同一个键。** 真库经 `internal/store/read.go:88` 读回、`internal/store/open.go:282-294` 的 `decodeMeta` 做 `json.Unmarshal` 进 `map[string]any`：`event` 是 `string`，`thought_dropped` 是 `bool`，`denied_actions` 是 `[]any`（元素是 `map[string]any`，键是 `Action` / `DisplayName`，因为 `provider.DeniedAction`（`internal/provider/turn.go:140-143`）没有 json tag），整数是 `float64`。而 repl 包的 `fakeStore`（`internal/repl/conversation_test.go:478-495`）不经 JSON，Meta 原样存，`denied_actions` 是 `[]provider.DeniedAction`。**只对着其中一种写判据，另一种上标记会静默消失。**
3. **只有 grok 会给 thought。** `internal/provider/grok.go:246` 透传 `thought`；`claude.go:231` 与 `agy.go:258` 把它清空。§7.2 的正反向验收因此必须用 grok 形状的假 CLI。既有三个 `TestMain`（`internal/app/smoke_test.go:40-56`、`internal/repl/session_inflight_test.go:94-100`、`internal/tui/rba_fakecli_test.go:51-78`）只会吐 claude 的 JSON；`turn.Runner` 的 `spawn` 字段不导出（`internal/turn/runner.go:96`），包外注入不了。所以端到端验收要给其中一个假 CLI 加一条分支（§2.0）。
4. **装配缺口顺手堵上。** `internal/app/app.go:159` 的 `svc.SetThoughtPolicy(cfg.StoreThought)` 今天删掉之后全套仍绿（#9(a) 第三条剩余项）。本刀的端到端验收走 `app.Run` + TOML 里的 `store_thought = false`，原型实测删掉那一行后该用例的前提守卫当场 Fatalf（§4 #24）。
5. **标记只能放在 TEXT 格的末尾，不能加列。** `internal/repl/repl.go:270-276` 立的规矩：tabwriter 按 rune 数算宽，可能含中文的字段一律放最后一列，后面不能再有列被它撑歪。TEXT 是最后一格（不以 tab 结尾，不参与对齐），在它后面追加字节不影响任何一列的宽度；新加一列放在 TEXT 之后会被中文撑歪，放在 TEXT 之前会让所有没有标记的行也多一列占位符，并改变表头。

---

## 1. 范围

### 1.1 做

凡写「名字实现方定」处实现方可自取名字（不许以 `s10d` 开头）；其余判据、字面量、顺序全部冻结。

#### A. `internal/repl/commands.go`（唯一改动的生产文件）

**A1. `cmdLog` 只改一格。** `:1163` 这一行

```go
			orPlaceholder(oneLine(m.Text)),
```

改成 TEXT 格 = `orPlaceholder(oneLine(m.Text))` **后面接上**标记串（A2 的返回值）。也就是：

- 先对正文做 `oneLine` 再做 `orPlaceholder`，**然后**拼标记。正文为空时这一格以占位符 `-` 开头，后面跟标记（例：`- [denied_actions]`）。不许把标记拼进 `orPlaceholder` 的参数里。
- 没有任何标记时，标记串是空串，这一行与 HEAD **逐字节相同**。
- `cmdLog` 的其余部分一个字不改：表头 `"SEQ\tSENDER\tKIND\tTO\tBRAIN\tTEXT"`、其余五格、`/log [n]` 的参数解析与两句用法提示、`MaxSeq` / `History` 的读法、空会话那一句。

**A2. 新增一个小函数**（名字实现方定，不许以 `s10d` 开头；放在 `cmdLog` 之后；签名 `func <名字>(meta map[string]any) string`），语义冻结如下。

它按**固定顺序** E、T、D 检查三件事，每命中一件就追加 **一个 ASCII 空格 + 该标记**（标记的字节见 §3.1），都不命中返回空串。`meta` 为 nil 时返回空串（对 nil map 取下标是合法的，不需要特判）。**这条防御保留，但今天没有任何路径走得到它**：真库写入时 `internal/store/open.go:271-273` 的 `encodeMeta` 把 nil 编成 `{}`，读回时 `:282-293` 的 `decodeMeta` 遇到 `""` / `"null"` / JSON null 一律返回空 map、从不返回 nil；repl 包的 `fakeStore`（`internal/repl/conversation_test.go:490-492`）在 `AppendMessage` 里把 nil 换成空 map。所以 §3.4 第 17 行（Meta 写的是 `nil`）传进新函数的是空 map，**并没有测到 nil 这条路**，不要把它当成 nil 的担保。三件事**都不看消息的 Kind**（system、agent、user 一视同仁）。

| | 读的键 | 判据（冻结） | 追加 |
|---|---|---|---|
| E | `event` | 值断言成 `string` 成功，且 `oneLine(值)` 非空 | `" [event=" + oneLine(值) + "]"` |
| T | `thought_dropped` | 值断言成 `bool` 成功，且为 `true` | `" [thought_dropped]"` |
| D | `denied_actions` | `reflect.ValueOf(值).Kind() == reflect.Slice` 且 `Len() > 0` | `" [denied_actions]"` |

逐条说死容易写歪的地方：

- E：值不是 `string`（真库里的 JSON 数字是 `float64`、`bool`、nil、键缺席）一律不标，**不许 `fmt.Sprint` 兜底**。先 `oneLine` 再判空：`"  "` 经 `oneLine`（`:1255-1257`，替换换行与 tab 再 `TrimSpace`）变成空串，不标。标记里的值是 `oneLine` 之后的值，含换行 / tab 的值被压成一行。**不建翻译表、不设白名单**：任何字符串值原样印出（8 个已知值与将来新增的值同一条路）。
- T：只认 `bool` 的 `true`。`false`、字符串 `"true"`、键缺席都不标。
- D：任何切片类型都算（真库的 `[]any`、fakeStore 的 `[]provider.DeniedAction`、其它切片），**长度大于 0** 才标。空切片、typed nil 切片、nil、字符串、map 都不标（数组 `reflect.Array` 不算切片；今天没有任何写者会写数组，见 §4 等价清单）。**只读 `denied_actions`，不读 `denied_actions_raw`**。**不印条数、不印 action 名**：`ask.go:478-483` 与 `ask_test.go:655-661` 已钉「这个长度是下界不是清点」，`/log` 再印一个数会被当成清点；action 名还要跨两种键形状取。
- 三个键名用既有常量：`turn.MetaEvent`（或同值的 `conversation.MetaEvent`）、`conversation.MetaThoughtDropped`、`turn.MetaDeniedActions`。不新增键名常量。三个标记字面量写成常量还是内联，实现方定。

**A3. import**：`commands.go` 今天的 import（`:3-15`）加两项：标准库 `"reflect"`，以及 `"council/internal/turn"`。`repl` 包的别的文件早已导入 `turn`（`ask.go:10` 等），没有新的依赖边。

**A4. 注释**：`cmdLog` 的文档注释（`:1110-1120`）补一段，说明 TEXT 格末尾的三种标记、固定顺序、为什么不加列（§0 第 5 条）、为什么不印条数（§1.1 A2 D 条）。原有「不打印 thought」那一段保留：本刀打的是「thought 被丢了」这个标记，不是 thought 本身。新函数的注释写明两种 Meta 形状（§0 第 2 条），这是用 `reflect` 而不是对 `[]any` 做类型断言的理由。

### 1.2 不做（两条腿都不许越界）

- 生产代码只改 `internal/repl/commands.go`。不改 `internal/turn`、`internal/conversation`、`internal/store`、`internal/app`、`internal/tui`、`internal/provider`、`internal/config`、`cmd/`、`go.mod`。
- 不加列、不改表头、不改 `/help` 里 `/log` 那一行（`:147`）、不改 `ask.go` 的 `deniedActionsNote`、不改 `orPlaceholder` / `oneLine` / `brainCell`。
- 不做 A 之五（§3.8 的闸、`/session` 的 `BaselineSuspect` 列）、不做 C2（崩溃对账端到端）、不做 D 之三（`14_MVP_CHECKLIST.md`）、不碰 `/cost`、不碰启动横幅（§3.11 / 3.11a / 3.11b 冻结的文本与顺序一个字不动）。
- 不做 schema 迁移。
- **既有 `_test.go` 只授权改一处**：`internal/app/smoke_test.go` 的 `runFakeProviderCLI`（`:52-56`），由测试腿按 §2.0 的逐字代码加一条环境变量分支。理由：§0 第 3 条，thought 只有 grok 给，三个既有假 CLI 只吐 claude 的 JSON，包外注入不了 spawn，同包又不能有第二个 `TestMain`。这条分支在环境变量不设时不执行，`TestSmokeAllPackagesWireTogether` 的行为逐字节不变（原型全量实跑仍绿）。**同一文件的其余部分（含注释、`TestMain`、常量）一个字不改**，其它既有 `_test.go` 一个都不改。
- 测试腿不写生产代码，实现腿不写 `_test.go`。

### 1.3 既有钉住 `/log` 的用例（逐个核对过，本刀一个都不改）

原型（HEAD `92dfc89` + §1.1 的实现 + §2.0 的授权改动 + 本契约的全部新用例）跑全量 `go test -count=1 ./...` 全绿，下面这些用例无一变红：

| 文件:用例 | 它钉的是什么 | 为什么本刀不动它 |
|---|---|---|
| `internal/repl/conversation_test.go:688` `TestNewSayLogRoundTrip` | `/log` 输出里出现正文与 agent id（`strings.Contains`） | 消息 Meta 为空 map，没有标记，行与 HEAD 逐字节相同 |
| `conversation_test.go:744` `TestLogShowsAppendedOrder` | 三条正文的相对顺序（`strings.LastIndex`） | 同上；标记只追加在行尾，不改顺序 |
| `conversation_test.go:797` `TestLogReadsFromStore` | 塞进库的两条正文与 sender 出现且有序（`strings.Index`） | `seedMessage`（`:154-166`）不带 Meta，没有标记 |
| `conversation_test.go:825` `TestCommandsWithoutCurrentConversation`（`log` 子用例） | 没有当前会话时 `/log` 给提示 | 走 `needsConv` 兜底，进不到表格 |
| `conversation_test.go:642` `TestStepFiveCommandsWithoutService`、`:672` `TestStepFiveCommandsPanicSafety`（`newStepFiveCommands` 的 `/log` 项，`:593`） | svc 为 nil 时 `/log` 给答复、不 panic | 走 `needsSvc` 兜底，进不到表格 |
| `conversation_test.go:960` `TestLeaveIsSoftDelete` | `/leave` 之后 `/log` 仍读得到正文（`strings.Contains`） | 没有标记 |
| `internal/repl/bare_text_test.go:203` `TestLogShowsRecordedBrain` | BRAIN 格是消息里记的脑子（`strings.Contains`） | 不碰 BRAIN 格 |
| `bare_text_test.go:267` `TestLogAndWhoReadBrainFromDifferentSources` | `/log` 显示旧脑子、不显示配置里的新脑子 | 同上 |
| `internal/repl/multiline_test.go:216` `TestOneLineNeutralizesTabwriterSeparators` | `oneLine` 把全部 tabwriter 分隔符中和掉 | 不改 `oneLine`；本刀让 `event` 的值也过它，这条顺带担保标记里不会带 tab / 换行 |
| `internal/app/smoke_test.go:82` `TestSmokeAllPackagesWireTogether` | 跑 `/log`，但只查库、不看屏幕 | §2.0 的分支在 `S10D_FAKE_STDOUT_FILE` 不设时不执行，行为逐字节不变 |

`internal/tui` 的既有用例里没有任何一条跑 `/log`（现查零命中）。

---

## 2. 验收（测试腿新写的文件）

通用纪律：

- 包级标识符一律以 `s10d` 开头（类型、函数、变量、常量）。测试函数名 `TestS10d...`，名字照下面各条冻结的写，主代理要按名字对账。不新增 `TestMain`（三个包都已有）。
- 允许只读引用同包既有测试辅助，不许改它们：app 包的 `isolateAppEnv`、`tomlLiteral`、`writeTestConfig`、`runAppForTest`（`internal/app/app_test.go:42-110`），以及 `smoke_test.go` 已注册的 sqlite 驱动；repl 包的 `newRoundStreamService`（`step10ux1_round_stream_test.go:94-109`）、`newTestService`、`runREPLSvc`（`conversation_test.go:599-634`）、`fakeStore` 的 `seedConversation`（`:138-142`）与 `AppendMessage`（`:478-495`）；tui 包不需要任何既有辅助。
- 不起真 provider：假 CLI 一律是测试二进制自己（`COUNCIL_FAKE_CLI=1`，command 写 `os.Args[0]`）。store 一律落在 `t.TempDir()` 下，不碰 `C:\Users\25142\.council\data`。app 包的用例另外 `t.Setenv("COUNCIL_GLOBAL_RULES", <t.TempDir() 下一个不存在的文件>)`，不读本机的全局规则文件（文件不存在等于没有规则，`internal/app/app.go:114`）。
- 断言一律**整行相等**。`Fatalf` 前提守卫（前提不成立即停，不是被测断言）允许用子串；「整个输出里没有任何一行含 `[thought_dropped]`」这类**反向全局扫描**也允许用子串，因为它只能让用例更严。
- 每条用例的注释写【判别力】：能不能为假、只差一处的对照是哪条。
- 输出切行：`strings.Split(out, "\n")`（REPL 只写 `\n`）。`/log` 的表头那一行前面带着提示符 `council> `（行式 REPL 先打提示符再读命令，不回显），数据行不带。
- **HEAD 上的状态**：本刀的测试文件不引用任何新标识符，在 HEAD 上**编译得过**。测试腿交付前必须在 HEAD `750c2d4`（或之后）的隔离拷贝里（加上 §2.0 那处授权改动）跑一次三个新文件，报告里写出命令与逐条结果。**预期**：`TestS10dE2EThoughtKeptNotMarked`、`TestS10dE2ENoDeniedActionsNotMarked` 在 HEAD 上**绿**（它们只断言没有标记的行，HEAD 本来就是这样）；其余 7 条（`TestS10dE2EThoughtDroppedMarked`、`TestS10dE2EEmptyTextThoughtDroppedMarked`、`TestS10dE2EDeniedActionsMarked`、`TestS10dLogMarksRealStore`、`TestS10dLogMarksFakeStore`、`TestS10dLogMarksEventFullSet`、`TestS10dLogMarksTUILineWriter`）在 HEAD 上**红**，而且红在「期望有标记的那一行」上，不是红在前提守卫上（`TestS10dE2EEmptyTextThoughtDroppedMarked` 的四道前提守卫（替换确实发生；`thought` 为空；`thought_dropped` 是 bool 的 true；`completion == "empty"`）在 HEAD 上都成立，红在 `G3E_OFF` 那一处连续四行上，HEAD 打出的是 `G3E_PLAIN`；v1.1 原型实测如此）。哪一条的 HEAD 结果与此不符，先查装置，不许交付。**v1.1 注**：这项清点在 HEAD `750c2d4`（或之后）的隔离拷贝里做，不在 `92dfc89` 上做：app 包同包已有 `s10c2_crash_e2e_test.go`（前缀 `s10c`），本刀的新文件与它共用 `smoke_test.go` 的 `TestMain`，「名字不撞」这条约束只有在 `750c2d4` 上才被真正检验。

### 2.0 授权改动：`internal/app/smoke_test.go` 的 `runFakeProviderCLI`（逐字）

在 `:53` `_, _ = io.ReadAll(os.Stdin)` 之后、`:54` `fmt.Fprint(os.Stdout, fakeClaudeSuccessJSON)` 之前插入下面这段（tab 缩进，照抄）：

```go
	if p := os.Getenv("S10D_FAKE_STDOUT_FILE"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "S10D_FAKE_STDOUT_FILE: %v\n", err)
			os.Exit(3)
		}
		_, _ = os.Stdout.Write(b)
		os.Exit(0)
	}
```

读文件而不是把 JSON 塞进环境变量：grok 的原件带换行与中文，放进文件不必考虑环境块的编码与长度。子进程继承环境（三家适配器的 `Command.Env` 都是 nil：`claude.go:108`、`grok.go:140`、`agy.go:139`）。这段不加注释以外的任何东西；函数的文档注释可以补一句「`S10D_FAKE_STDOUT_FILE` 非空时改吐该文件的字节」。

### 2.1 `internal/app/s10d_log_marks_e2e_test.go`（真 `app.Run`、真库、真 Runner、假 CLI）

**原件常量**（逐字节复制，不许手写，注释登记来源与「又多一份拷贝」）：

- `s10dGrokT1JSON`：逐字节复制 `internal/turn/fixtures_test.go:53-77` 的 `grokT1JSON`（`thought` 非空，`text` 是 `OK`）。
- `s10dAgyDeniedJSON`：逐字节复制 `internal/turn/fixtures_test.go:137` 的 `agyDeniedActionJSON`（`response` 是空串，`denied_actions` 一个元素）。

**共用装置**（一个包级 helper，名字以 `s10d` 开头）：入参是 provider 名（`"grok"` / `"agy"`）、`store_thought` 那一行（空串表示不写这一行）、假 CLI 要吐的字节；按这个顺序：

1. `isolateAppEnv(t)`；`t.Setenv("COUNCIL_FAKE_CLI", "1")`；`dir := t.TempDir()`；`dataDir := filepath.Join(dir, "data")`。
2. 把假 CLI 的字节写进 `filepath.Join(dir, "fake_stdout.json")`，`t.Setenv("S10D_FAKE_STDOUT_FILE", 那个路径)`；`t.Setenv("COUNCIL_GLOBAL_RULES", filepath.Join(dir, "no-such-COUNCIL.md"))`。
3. 配置正文照 §3.3 逐字拼，`writeTestConfig(t, dir, 正文)`。
4. 脚本照 §3.3 逐字，`runAppForTest(t, context.Background(), cfgPath, strings.NewReader(脚本))`；返回的 error 必须是 nil（前提守卫，Fatalf 时带上 Out 与 Err）。
5. 返回 Out 按 `"\n"` 切开的行，以及库文件路径 `filepath.Join(dataDir, "council.db")`（读库照 `smoke_test.go:124-129` 的体例 `sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))`）。

读库的前提守卫统一取 `SELECT thought, meta FROM messages WHERE kind = 'agent'`，**恰好一行**，`meta` 用 `json.Unmarshal` 进 `map[string]any`。

**E1a `TestS10dE2EThoughtDroppedMarked`**：grok，`store_thought = false`，吐 `s10dGrokT1JSON`。
- 前提守卫：agent 那一行 `thought == ""`；`meta["thought_dropped"]` 是 `bool` 的 `true`。
- 断言：Out 的行里**恰好一处**连续出现 §3.3 的四行 `G_HDR`、`G1`、`G2`、`G3_OFF`（整行相等，顺序照写）；没有任何一行等于 `G3_ON`。
- 【判别力】HEAD 上 `G2` 与 `G3_OFF` 都不成立（没有标记）。对照是 E1b：同一装置、同一份 grok 原件，只差 `store_thought` 那一行。`G2` 同时担保 `event` 标记在真实生产面包屑（`session_opening`）上生效。前提守卫担保 `app.go:159` 那行接线：删掉它，thought 照存、守卫 Fatalf（原型实测，§4 #24）。

**E1b `TestS10dE2EThoughtKeptNotMarked`**：grok，`store_thought = true`，吐同一份 `s10dGrokT1JSON`。
- 前提守卫：agent 那一行 `thought != ""`（§7.2：「那一轮的 thought 非空」，缺了它一次「这轮本来就没 thought」的巧合会把判据骗过去）；`meta` 里没有 `thought_dropped` 这个键。
- 断言：`G_HDR`、`G1`、`G3_ON` 各**恰好一行**（整行相等）；没有任何一行等于 `G3_OFF`；Out 里没有任何一行含子串 `[thought_dropped]`。
- 【判别力】「标记总是打」的实现在这里红（`G3_ON` 找不到）。同一位置（seq 3 那一行）在 E1a 里是 `G3_OFF`，两行只差末尾的 ` [thought_dropped]`。本条在 HEAD 上绿，是回归钉。

**E1c `TestS10dE2EEmptyTextThoughtDroppedMarked`**（v1.1 新增）：grok，`store_thought = false`，吐的字节 = 对 `s10dGrokT1JSON` 做 `strings.Replace(原件, "\"text\": \"OK\",", "\"text\": \"\",", 1)`，也就是把子串

```
"text": "OK",
```

换成

```
"text": "",
```

（这个子串在 `internal/turn/fixtures_test.go` 整个文件里只出现一次，就在 `:54` 的 `grokT1JSON` 里；结果是同一份原件只把正文清空，thought 与其余字节不变，不是另一份手写的 JSON。）
- 前提守卫（照 E2b 的体例）：替换之后的字节与原件不相等（替换确实发生了）；agent 那一行 `thought == ""`；`meta["thought_dropped"]` 是 `bool` 的 `true`；`meta["completion"] == "empty"`（`internal/turn/runner.go:1280` 与 `:1358-1364` 的 `completionMeta`，grok 的 `completionFrom` 在正文为空、`stopReason` 为 `end_turn` 时给 `CompletionEmpty`）。
- 断言：Out 的行里**恰好一处**连续出现 §3.3 的四行 `G_HDR`、`G1`、`G2`、`G3E_OFF`（整行相等，顺序照写）；没有任何一行等于 `G3E_PLAIN`。
- 【判别力】这是 W1 的 major：「正文为空时只打 D、丢掉 E / T」的写法（§4 #29）活过了 v1 的全部验收，因为 v1 里正文为空的行（E2a、§3.4 第 16 行）只带 D；而生产上 grok 正文为空、thought 非空、`store_thought = false` 时真会产出「空正文加 `thought_dropped`」的行，本条用真实生产路径把它钉住。HEAD 上打出的是 `G3E_PLAIN`（红）。对照有两条：E1a（同一份原件、同一个开关，只差 `text` 的字节，`G3_OFF` 与 `G3E_OFF` 只差 `OK` / `-`）；E2a（同是占位符 `-` 加一个标记，换的是 T 与 D）。`completion == "empty"` 这道守卫担保 TEXT 格的 `-` 真是因为正文为空，不是标记吃掉了正文。本条计入 §2 通用纪律「HEAD 上的状态」里应红的那一组。

**E2a `TestS10dE2EDeniedActionsMarked`**：agy，不写 `store_thought`，吐 `s10dAgyDeniedJSON`。
- 前提守卫：`meta["denied_actions"]` 是长度 1 的 `[]any`。
- 断言：Out 的行里**恰好一处**连续出现 `A_HDR`、`A1`、`A2`、`A3_DENIED`；没有任何一行等于 `A3_PLAIN`。
- 【判别力】正文被清空的这一行是最有分辨力的形状：TEXT 格是占位符 `-` 加标记。标记吃掉占位符、或占位符被挪到拼接之后（`orPlaceholder(oneLine(Text)+标记)`）都会改变这一行。真库路径担保 `[]any` 形状上的判据。

**E2b `TestS10dE2ENoDeniedActionsNotMarked`**：agy，不写 `store_thought`，吐的字节 = 对 `s10dAgyDeniedJSON` 做 `strings.Replace(原件, 子串, "", 1)`，子串逐字是：

```
,"denied_actions":[{"action":"write_file","display_name":"WriteToFile"}]
```

（结果就是同一份原件去掉 `denied_actions` 这个键，其余字节不变，不是另一份手写的 JSON。）
- 前提守卫：替换之后的字节与原件不相等（替换确实发生了）；`meta` 里没有 `denied_actions` 这个键。
- 断言：`A_HDR`、`A1`、`A3_PLAIN` 各**恰好一行**；没有任何一行等于 `A3_DENIED`；Out 里没有任何一行含子串 `[denied_actions]`。
- 【判别力】与 E2a 只差 `denied_actions` 这个键；两行只差末尾的 ` [denied_actions]`。本条在 HEAD 上绿，是回归钉。

### 2.2 `internal/repl/s10d_log_marks_test.go`（判据的边界，两种 Meta 形状各跑一遍）

种子表是 §3.4 的 20 条，**顺序冻结**。

**L1 `TestS10dLogMarksRealStore`**：`svc := newRoundStreamService(t)`（真库，`store.Open` 到 `t.TempDir()` 下）；`conv, err := svc.Create(ctx, "s10d")`；按 §3.4 的顺序逐条 `svc.Post`（第 1-17 条与第 19 条及以后 `SenderID: conversation.ReservedSenderSystem, Kind: conversation.KindSystem`，第 18 条 `SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser`；`ConvID: conv.ID`；`Text` 与 `Meta` 照表，Meta 的 Go 值照表逐字写，`nil` 就是 nil）；每条 Post 必须成功（前提守卫）。然后 `out, err := runREPLSvc(t, svc, "/use "+conv.ID+"\n/log 50\n")`，err 必须是 nil。
- 断言：`R_HDR` 在输出行里**恰好一行**；紧跟其后的 20 行与 §3.4 的期望 20 行**逐行整行相等**。
- 【判别力】每条种子与相邻种子只差一个值，逐条对照写在 §3.4 表的「对照」列。真库把 `[]provider.DeniedAction` 编成 JSON 再读回 `[]any`、把 `5` 读回 `float64`、把 typed nil 读回 nil：「只认 `[]provider.DeniedAction`」的实现在这里红（第 10、12、15、16 行）。v1.1 追加的三行：第 18 行（user 消息三键俱全）一行杀「只对 system 打 E」「user 消息上不打 T」「user 消息上不打 D」三种写法；第 19 行（正文为空、E 加 T）杀「正文为空时只打 D」（§4 #29）；第 20 行（`denied_actions` 是 map）杀「D 也认 map」（§4 #30），真库把 map 读回 `map[string]any`，fakeStore 原样是 `map[string]any`，两种形状上都是 map。

**L2 `TestS10dLogMarksFakeStore`**：`svc, f := newTestService(t)`；`conv := f.seedConversation("s10dconv", "s10d")`；按 §3.4 的顺序逐条 `f.AppendMessage(ctx, conversation.Message{...})`（字段同 L1）；`runREPLSvc(t, svc, "/use s10dconv\n/log 50\n")`。
- 断言：同 L1，期望的 20 行**与 L1 完全相同**。
- 【判别力】fakeStore 不经 JSON，`denied_actions` 原样是 `[]provider.DeniedAction` / `[]string` / typed nil：「只认 `[]any`」的实现在这里红（第 10、12、15、16 行）；「键在就标」或「非 nil 就标」在第 13 行（typed nil 切片装进 `any` 之后不等于 nil）红。L1 与 L2 必须同时存在，缺一条就有一种形状没人担保。

**L3 `TestS10dLogMarksEventFullSet`**：真库（`newRoundStreamService(t)`），`svc.Create(ctx, "s10d-ev")`，按 §3.5 的顺序 Post 8 条 system 消息，`Text: "e"`，`Meta: map[string]any{"event": <值>}`；`runREPLSvc(t, svc, "/use "+conv.ID+"\n/log\n")`。
- 断言：`R_HDR` 恰好一行，紧跟其后的 8 行与 §3.5 逐行整行相等。
- 【判别力】担保「没有白名单、没有翻译表」：漏掉任何一个已知事件（例如白名单里没写 `team_bound`，它是 conversation 包的常量、不在 `record.go:218` 那条注释里）都会红。

### 2.3 `internal/tui/s10d_log_marks_tui_test.go`（TUI 走 LineWriter，文本相同）

**U1 `TestS10dLogMarksTUILineWriter`**：
- 装置：`ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)`；`st, err := store.Open(ctx, filepath.Join(t.TempDir(), "council-data"))`，`t.Cleanup` 关库；`svc := conversation.NewService(st)`；`svc.Create(ctx, "s10d")`；按 §3.6 的顺序 Post 4 条 system 消息（`Text: "中文正文"`）。`input := "/use " + conv.ID + "\n/log\n"`。
- 跑两遍，同一个 svc、同一份 input，各用一个新的 REPL：
  - 行式：`var buf bytes.Buffer`；`repl.New(nil, svc, nil, strings.NewReader(input), &buf, repl.RulesContext{}).Run(ctx)` 返回 nil。
  - TUI 出口：`lw := NewLineWriter(func(line string) { 加锁 append 进切片 })`；`repl.New(nil, svc, nil, strings.NewReader(input), lw, repl.RulesContext{}).Run(ctx)` 返回 nil；然后 `lw.Flush()`。
- 断言 1：LineWriter 吐出的行里，`R_HDR` 恰好一行，紧跟其后的 4 行与 §3.6 逐行整行相等。
- 断言 2：LineWriter 吐出的行与 `strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")` 逐行相等（长度也相等）。
- 【判别力】断言 1 在 HEAD 上红（没有标记）。断言 2 是回归钉：LineWriter 只切行不改字节（`internal/tui/writer.go:30-31`），今天恒真，写下来是为了哪天有人在 TUI 这一侧对 `/log` 的行做了加工时它会红；它单独不构成本刀的证据。`printLine`（`internal/tui/model.go:175-181`）对非空行是恒等的，所以不必起真 Program。

---

## 3. 冻结的字面量

### 3.0 为什么冻结（对 §7.2「措辞不进契约」的裁法）

§7.2 照 `11a` §7 的先例写「措辞不进契约」。本契约**不采用**，把三个标记逐字节冻结，理由三条：

1. **这三个标记是本刀唯一的可观测面。** 不冻结时测试只能写「有 / 无」与「两行不同」：E1a 与 E1b 两行不同，并不能区分「打了 thought 的标记」与「把 denied 的标记错打在这一行」；L1 的 20 行也只剩「和某一行不同」这种弱判据。冻结之后每一行都能整行比对，§4 的每条变异都能点名到具体一行。
2. **本项目的惯例是用户可见的新字面量逐字节冻结**（`15a` §3、`13` §3.11 的横幅首行、§3.11a 的两个数）。`11a` §7 放开的是一段点名明细的措辞，不是一个要被人和测试同时识别的标记。
3. **冻结成本低**：标记落在 TEXT 格末尾，不参与列宽，改字面量只动 §3.1 与各期望行的尾巴，不牵连对齐。

字面量用 ASCII、与 Meta 键名一致：用户在 SQLite 里看到的键名就是 `/log` 上的标记，不需要一份对照表；`event` 的值本来就是 ASCII id，配中文前缀没有增益；与 `/log` 的 ASCII 表头、`/cost` 的 ASCII 列名同一体例；方括号、等号、下划线都是 ASCII，不存在模糊宽度字形与装饰符号。中文的备选措辞记在 open items，由 Griffin 定。

### 3.1 三个标记（每个前面恰好一个 ASCII 空格，按 E、T、D 的顺序）

```
 [event=<值>]
 [thought_dropped]
 [denied_actions]
```

`<值>` 是 `oneLine(meta["event"])`。三者同时命中时 TEXT 格的尾巴是 ` [event=<值>] [thought_dropped] [denied_actions]`。

### 3.2 不变的表头

表头的格式串仍是 `"SEQ\tSENDER\tKIND\tTO\tBRAIN\tTEXT"`。本契约里出现的三种表头行（前面带提示符）：

```
council> SEQ  SENDER  KIND    TO  BRAIN               TEXT
council> SEQ  SENDER  KIND    TO  BRAIN                        TEXT
council> SEQ  SENDER  KIND    TO  BRAIN  TEXT
```

依次记作 `G_HDR`（grok 端到端）、`A_HDR`（agy 端到端）、`R_HDR`（repl / tui 种子用例）。BRAIN 列的宽度随最长的脑子格变（`grok/grok-4.6/high` 18 个字节、`agy/gemini-3.8-flash-high/-` 27 个字节、全是 `-` 时 5 个字节），不是本刀改的。

### 3.3 端到端（§2.1）的配置、脚本与期望行

配置正文（`<dataDir>` 与 `<exe>` 分别是 `tomlLiteral(dataDir)` 与 `tomlLiteral(os.Args[0])` 的结果，即带单引号的 TOML 字面字符串；`<store_thought 行>` 是 `store_thought = false` 或 `store_thought = true` 或整行不写）：

grok：

```
data_dir = <dataDir>
<store_thought 行>

[providers.grok]
command = <exe>

[agents.g1]
display_name = "G1"
provider = "grok"
model = "grok-4.6"
effort = "high"
role = "r"
permissions = "read"
mode = "discussion"
```

agy（不写 `store_thought`）：

```
data_dir = <dataDir>

[providers.agy]
command = <exe>

[agents.g1]
display_name = "G1"
provider = "agy"
model = "gemini-3.8-flash-high"
role = "r"
permissions = "read"
mode = "discussion"
```

脚本（六行，每行以 `\n` 结尾）：

```
/new t
/join g1
hello-s10d
/ask g1
/log
/quit
```

库里的消息顺序（原型实测）：seq 1 是 user 的 `hello-s10d`；seq 2 是 bootstrap 轮追加的 `session_opening` 面包屑；seq 3 是 g1 的发言。E1c（正文被清空的 grok 原件）的顺序与此相同（v1.1 原型实测：正文为空的一轮不多追加任何面包屑，agent 那一行仍是 seq 3，`G_HDR`、`G1`、`G2` 与 E1a 逐字节相同）。

grok 端到端的期望行：

```
G_HDR  = council> SEQ  SENDER  KIND    TO  BRAIN               TEXT
G1     = 1    user    user    -   -                   hello-s10d
G2     = 2    system  system  -   -                   正在为 agent g1 开启一条新的 provider 会话 [event=session_opening]
G3_OFF = 3    g1      agent   -   grok/grok-4.6/high  OK [thought_dropped]
G3_ON  = 3    g1      agent   -   grok/grok-4.6/high  OK
```

E1c（v1.1 新增）的 seq 3 那一行（`G_HDR`、`G1`、`G2` 同上）：

```
G3E_OFF   = 3    g1      agent   -   grok/grok-4.6/high  - [thought_dropped]
G3E_PLAIN = 3    g1      agent   -   grok/grok-4.6/high  -
```

agy 端到端的期望行：

```
A_HDR     = council> SEQ  SENDER  KIND    TO  BRAIN                        TEXT
A1        = 1    user    user    -   -                            hello-s10d
A2        = 2    system  system  -   -                            正在为 agent g1 开启一条新的 provider 会话 [event=session_opening]
A3_DENIED = 3    g1      agent   -   agy/gemini-3.8-flash-high/-  - [denied_actions]
A3_PLAIN  = 3    g1      agent   -   agy/gemini-3.8-flash-high/-  -
```

（`名字 = ` 只是本文的标签，不是行的一部分；行的内容从 `=` 后面那个空格之后开始，行尾没有空格。）

### 3.4 L1 / L2 的种子与期望行

`da` 指 `[]provider.DeniedAction{{Action: "write_file", DisplayName: "WriteToFile"}}`（每处用的时候各写一份字面量，或用一个包级变量，测试腿定）。

| # | Text | Meta（Go 值，逐字） | 对照（最接近的那一条；第 18、19 行的对照不止差一处） |
|---|---|---|---|
| 1 | `ev-turn-lost` | `map[string]any{"event": "turn_lost"}` | 2 |
| 2 | `ev-empty` | `map[string]any{"event": ""}` | 1 |
| 3 | `ev-num` | `map[string]any{"event": 5}` | 1 |
| 4 | `ev-nl` | `map[string]any{"event": "a\nb\tc"}` | 1 |
| 5 | `ev-space` | `map[string]any{"event": "  "}`（两个空格） | 2 |
| 6 | `td-false` | `map[string]any{"thought_dropped": false}` | 8 |
| 7 | `td-str` | `map[string]any{"thought_dropped": "true"}` | 8 |
| 8 | `td-true` | `map[string]any{"thought_dropped": true}` | 6 |
| 9 | `da-empty` | `map[string]any{"denied_actions": []any{}}` | 10 |
| 10 | `da-one` | `map[string]any{"denied_actions": da}` | 9 |
| 11 | `da-str` | `map[string]any{"denied_actions": "x"}` | 12 |
| 12 | `da-strs` | `map[string]any{"denied_actions": []string{"x"}}` | 11 |
| 13 | `da-nil` | `map[string]any{"denied_actions": []provider.DeniedAction(nil)}` | 10 |
| 14 | `raw-only` | `map[string]any{"denied_actions_raw": []any{map[string]any{"action": "w"}}}` | 10 |
| 15 | `all3` | `map[string]any{"event": "turn_failed", "thought_dropped": true, "denied_actions": da}` | 1、8、10 |
| 16 | （空串） | `map[string]any{"denied_actions": da}` | 10 |
| 17 | `plain` | `nil` | 1 |
| 18 | `ev-on-user` | `map[string]any{"event": "team_bound", "thought_dropped": true, "denied_actions": da}`（**user** 消息） | 15 |
| 19 | （空串） | `map[string]any{"event": "turn_failed", "thought_dropped": true}` | 16 |
| 20 | `da-map` | `map[string]any{"denied_actions": map[string]any{"action": "w"}}` | 10 |

第 4 条的 Go 源码里就是 `"a\nb\tc"` 这个带转义的字符串字面量（一个换行、一个 tab）。

v1.1 对第 18 行的改动与追加的第 19、20 行（**追加在末尾，不插队、不重新编号**，§4 里对第 1-18 行的引用全部不变）：

- 第 18 行（user 消息）的 Meta 从 v1 的只有 `event` 改成三键俱全（`da` 同上），期望行带三个标记。对照改成第 15 行（三个键相同，差的是 Kind 与 `event` 的值）。这一行一次杀三种写法：「只对 system 打 E」（§4 #15）、「user 消息上不打 T」、「user 消息上不打 D」；v1.1 原型实测三者各只红 L1 / L2 的第 18 行。
- 第 19 行：正文为空、`event` 加 `thought_dropped`、没有 `denied_actions`。对照是第 16 行（同是空正文，第 16 行只有 D）。杀「正文为空时只打 D」（§4 #29）：v1 里正文为空的只有第 16 行与 E2a，恰好都只带 D，这种写法活过了 v1 的全部验收（W1 的 major）。
- 第 20 行：`denied_actions` 是一个非空 map。对照是第 10 行（同一个键，非空切片换成非空 map）。杀「D 也认 map」（§4 #30）。
- 「thought 本来为空时不误报」：库这一层的既有担保是 `internal/conversation/thought_policy_test.go:81` 的 `TestServiceDoesNotMarkDroppedWhenThoughtAlreadyEmpty`（thought 本来是空串时 `applyThoughtPolicy` 不写 `thought_dropped`）；`/log` 只读这个键，不重新判断，所以本刀不另写这一向的用例。

期望行（`R_HDR` 之后紧跟的 20 行，L1 与 L2 相同）：

```
1    system  system  -   -      ev-turn-lost [event=turn_lost]
2    system  system  -   -      ev-empty
3    system  system  -   -      ev-num
4    system  system  -   -      ev-nl [event=a b c]
5    system  system  -   -      ev-space
6    system  system  -   -      td-false
7    system  system  -   -      td-str
8    system  system  -   -      td-true [thought_dropped]
9    system  system  -   -      da-empty
10   system  system  -   -      da-one [denied_actions]
11   system  system  -   -      da-str
12   system  system  -   -      da-strs [denied_actions]
13   system  system  -   -      da-nil
14   system  system  -   -      raw-only
15   system  system  -   -      all3 [event=turn_failed] [thought_dropped] [denied_actions]
16   system  system  -   -      - [denied_actions]
17   system  system  -   -      plain
18   user    user    -   -      ev-on-user [event=team_bound] [thought_dropped] [denied_actions]
19   system  system  -   -      - [event=turn_failed] [thought_dropped]
20   system  system  -   -      da-map
```

### 3.5 L3 的事件全集与期望行

顺序：`session_opening`、`turn_failed`、`post_rejected`、`session_rebuilt`、`turn_lost`、`round_started`、`round_finished`、`team_bound`。

```
1    system  system  -   -      e [event=session_opening]
2    system  system  -   -      e [event=turn_failed]
3    system  system  -   -      e [event=post_rejected]
4    system  system  -   -      e [event=session_rebuilt]
5    system  system  -   -      e [event=turn_lost]
6    system  system  -   -      e [event=round_started]
7    system  system  -   -      e [event=round_finished]
8    system  system  -   -      e [event=team_bound]
```

### 3.6 U1 的种子与期望行

四条 system 消息，`Text` 都是 `中文正文`，Meta 依次是：`map[string]any{"event": "turn_lost"}`、`map[string]any{"thought_dropped": true}`、`map[string]any{"denied_actions": da}`、`nil`。

```
1    system  system  -   -      中文正文 [event=turn_lost]
2    system  system  -   -      中文正文 [thought_dropped]
3    system  system  -   -      中文正文 [denied_actions]
4    system  system  -   -      中文正文
```

### 3.7 生产代码新增的字符串字面量

只有 §3.1 的三个（以及拼 E 标记用的 `" [event="` 与 `"]"` 两段）。不新增任何别的用户可见文本。

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；主代理落地后逐条跑，从副本还原，不用 `git checkout`。L1 / L2 后面的数字是 §3.4 的行号。

**v1.1 注：「预期被谁杀」是下界，实测常为超集**（W1 实测 #10、#11、#16、#21、#23a、#26、#27、#28a 都比预测多杀），以 L1 / L2 逐行比对为准：点名的杀手必须红，多红的不算偏差。

| # | 变异（可编译） | 预期被谁杀 |
|---|---|---|
| 1 | `cmdLog` 不调新函数（回到 HEAD） | E1a；E2a；L1；L2；L3；U1 断言 1 |
| 2 | 新函数恒返回三个标记（不看判据） | E1b；E2b；L1 / L2 第 2、3、6、7、9、11、13、14、17、19、20 行 |
| 3 | 标记拼在正文前面（`标记 + 正文`） | E1a；L1 / L2 第 1、8、10 行 |
| 4 | 标记取代正文（TEXT 格只放标记，没有标记时放正文） | E1a（`OK` 没了）；L1 / L2 第 1 行 |
| 5 | `orPlaceholder(oneLine(m.Text) + 标记)`（占位符挪到拼接之后） | E2a；L1 / L2 第 16 行 |
| 6 | 正文为空时不打标记 | E2a；E1c；L1 / L2 第 16、19 行 |
| 7 | 分隔符改成两个空格 | 所有带标记的期望行（E1a、E2a、L1、L2、L3、U1） |
| 8 | 顺序改成 T、E、D（或任何别的排列） | L1 / L2 第 15 行 |
| 9 | 新加一列 MARK（表头多一格） | E1a、E2a（`G_HDR` / `A_HDR`）；L1；L2；L3；U1（`R_HDR`） |
| 10 | E：去掉 `oneLine`，值原样拼进去 | L1 / L2 第 4 行（这一行被换行拆成两行） |
| 11 | E：只做 `strings.TrimSpace` 不替换换行与 tab | L1 / L2 第 4 行 |
| 12 | E：先判空再 `oneLine`（`v != ""` 之后才压行） | L1 / L2 第 5 行（打出 ` [event=]`） |
| 13 | E：不判空（`ok` 就打） | L1 / L2 第 2、5 行 |
| 14 | E：非 string 用 `fmt.Sprint` 兜底 | L1 / L2 第 3 行（打出 ` [event=5]`） |
| 15 | E：只对 `KindSystem` 打 | L1 / L2 第 18 行 |
| 16 | E：白名单只含 `internal/turn` 的 7 个事件 | L1 / L2 第 18 行；L3 第 8 行 |
| 17 | T：键在就打（`_, ok := meta[k]`） | L1 / L2 第 6、7 行 |
| 18 | T：`fmt.Sprint(v) == "true"` | L1 / L2 第 7 行 |
| 19 | T：只对 `KindAgent` 打 | L1 / L2 第 8、15 行；U1 断言 1 |
| 20 | D：只认 `[]any`（`v, ok := x.([]any); ok && len(v) > 0`） | L2 第 10、12、15、16 行 |
| 21 | D：只认 `[]provider.DeniedAction` | E2a；L1 第 10、12、15、16 行；U1 断言 1 |
| 22 | D：去掉 `Len() > 0`（任何切片都打） | L1 / L2 第 9 行；L2 第 13 行 |
| 23 | D：键在就打 / 值非 nil 就打 | L1 / L2 第 9、11 行；L2 第 13 行 |
| 24 | `internal/app/app.go:159` 的 `svc.SetThoughtPolicy(cfg.StoreThought)` 删掉（本刀不改 app.go，这条担保装配接线） | E1a 前提守卫（thought 照存，原型实测） |
| 25 | `app.go:159` 改成 `svc.SetThoughtPolicy(true)` | E1a 前提守卫 |
| 26 | D：读 `denied_actions_raw` 而不是 `denied_actions` | L1 / L2 第 10 行（没有 raw 键）与第 14 行（有 raw 键） |
| 27 | D：同时印条数（例如 ` [denied_actions=1]`） | E2a；L1 / L2 第 10、15、16 行 |
| 28 | 标记字面量写错一个字节（例如 `[thought dropped]`、`[event:turn_lost]`） | 所有含该标记的期望行 |
| 29 | 正文为空时只打 D（丢掉 E 与 T；正文非空时照常三件都查） | L1 / L2 第 19 行；E1c（打出 `G3E_PLAIN`）。第 16 行与 E2a 在这个变异下**仍绿**（它们只带 D），这正是 v1 没抓住它的原因 |
| 30 | D 也认 map（`(Kind() == reflect.Slice \|\| Kind() == reflect.Map) && Len() > 0`） | L1 / L2 第 20 行 |
| 31 | T：user 消息上不打（T 的条件加 `k != conversation.KindUser`） | L1 / L2 第 18 行（v1.1 W1b 实测各只红这一行） |
| 32 | D：user 消息上不打（D 的条件加 `k != conversation.KindUser`） | L1 / L2 第 18 行（同上） |

**判定为等价、不列杀手的变异**（写在这里，免得被当成漏测）：

- T 写成 `meta[k] == true`（接口与 `true` 比较）：只有动态类型是 `bool` 且值为 true 时相等，与冻结判据逐值等价。
- D 同时接受 `reflect.Array`：今天没有任何写者往 Meta 里放数组（真库读回的永远是切片，`runner.go` 写的是切片），可观测行为不变。（v1.1 原型实测：全部 s10d 用例仍绿。）
- （X2，v1.1 补）E 用 `strings.Join(strings.Fields(值), " ")` 代替 `oneLine`：两者只在值中间含连续空白或 `oneLine` 不认的 Unicode 空白时不同，而 `event` 的值今天都是 ASCII id 常量（`internal/turn/runner.go:29-35` 的 7 个与 `conversation/types.go:85` 的 `team_bound`），可观测行为不变。（v1.1 原型实测：全部 s10d 用例仍绿。）
- （X5，v1.1 补）agent 消息上不打 E：今天写 `event` 的只有 system 面包屑（`runner.go:1212-1214` 的 `postSystem`、`BindTeam`、`recover.go:220-237` 的 `lostNoticeMessage`），`agentMeta`（`runner.go:1271-1320`）不写 `MetaEvent`，agent 消息的 Meta 里没有这个键，可观测行为不变。（v1.1 原型实测：全部 s10d 用例仍绿。）注意它与 #15「只对 `KindSystem` 打」不同：#15 连 user 消息也不打，被 §3.4 第 18 行杀。
- 键名用 `conversation.MetaEvent` 还是 `turn.MetaEvent`：同一个字符串（`types.go:79-82` 的注释）。
- 用 `strings.Builder` 还是 `+` 拼接、新函数放在文件哪里、三个标记写成常量还是内联。

**只在审查时看（没有用例能抓，集成员收割时逐字核对）**：

- `commands.go` 是唯一改动的生产文件；`git diff --stat` 里没有别的生产文件。
- `cmdLog` 的文档注释按 §1.1 A4 补了；「不打印 thought」那一段还在。
- 生产代码没有以 `s10d` 开头的标识符；除 §3.7 外没有新增字符串字面量。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许改这 1 个文件：`internal/repl/commands.go`。不写任何 `_test.go`，不改 `go.mod`，不改别的生产文件。
- 用补丁块，同一文件可以有多个：

  ```
  ===PATCH: internal/repl/commands.go===
  <<<<ANCHOR
  （从真文件里原样复制的若干行，含行首 tab，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```

  锚点必须从 HEAD `92dfc89` 的真文件逐字复制（含 tab 与中文标点），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。**提醒**：`return tw.Flush()` 在这个文件里出现 8 次，单独拿它当锚点不唯一；`:1163` 那一行 `orPlaceholder(oneLine(m.Text)),` 在文件里只出现一次；import 块里的 `"council/internal/conversation"` 只出现一次。
- 不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。代码必须能 `gofmt`（tab 缩进，import 按字母序：`"reflect"` 在 `"fmt"` 与 `"sort"` 之间，`"council/internal/turn"` 在 `"council/internal/conversation"` 之后）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的字符串字面量只有 §3.7 列出的那些。
- 生产代码的新标识符不许以 `s10d` 开头（那是测试腿的前缀）。
- 估计改动约 40 行，不拆块。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：§1.1 A1-A4 每一条落在哪个补丁块。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3.1 的三个标记是否逐字照抄；有没有新增任何别的字符串字面量。
  判据：E、T、D 三条判据各用什么写法实现（逐条一句）。
  ```

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/repl`、中转根有没有 grok 写出来的新文件）；符号禁令扫描；按锚点落盘；`gofmt -l ./internal/repl`；`go vet ./internal/repl ./internal/app ./internal/tui`；`go build ./...`。逐字核对 §4「只在审查时看」三条。
2. **合并测试腿**：三个新文件加 §2.0 那一处授权改动。核对授权改动逐字等于 §2.0 的代码块、`smoke_test.go` 其余部分逐字节不变（`git diff internal/app/smoke_test.go` 只有那一段）。只跑 `./internal/repl`、`./internal/app`、`./internal/tui` 三个包（3 分钟没输出只停自己的 go 进程，改 `go test -c` 再跑二进制）。既有用例必须全绿（原型实测全量 `go test ./...` 全绿）。
3. **核对测试腿报告里 HEAD 上的逐条结果**：E1b、E2b 绿，其余 7 条（含 v1.1 新增的 E1c `TestS10dE2EEmptyTextThoughtDroppedMarked`）红且红在期望有标记的那一行（§2 通用纪律最后一条）；E1c 的四道前提守卫在 HEAD 上必须都成立（替换确实发生；`thought` 为空；`thought_dropped` 是 bool 的 true；`completion == "empty"`），红在 `G3E_OFF` 那一处连续四行上。清点在 HEAD `750c2d4` 或之后做。
4. **§4 的 32 条变异逐条跑**（#24、#25 改的是 `app.go`，只用来证明 E1a 担保了接线，跑完还原）；任何一条没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
5. **文档**：
   - `docs/13_STEP10_CONTRACT.md` §7.2 末尾补订正：「措辞不进契约」改为「三个标记逐字节冻结在 `s10d2` 契约 §3.1」，附 §3.0 的三条理由；标记的判据（E 不看 kind、D 认任何非空切片、不印条数）一并写明。
   - `docs/00_DECISIONS.md` #9(a) 第 8 条（`:544-546`）与 `:479-481` 的「`thought_dropped` 今天仍然没有任何读者」补一句「D 之二（`<commit>`）起 `/log` 读它」；同条「第三条剩余项」（`:483-492`）补「`TestS10dE2EThoughtDroppedMarked` 的前提守卫担保了 `app.go` 那一行接线」。`store_thought` 以外的接线（`rules` 等）仍是剩余项，不因此消失。
   - `docs/README.md` 路线图 Step 10 行：D 之二改成已落（带 commit），D 之三仍未落。
   - `internal/turn/runner.go:1299-1300` 的注释「重启之后 /log 也要能看到」从这一刀起成立，不用改；在 13 §7.2 的订正里点一句它此前是一句没兑现的注释。
   - 原件拷贝的登记：`grokT1JSON` 与 `agyDeniedActionJSON` 此前各有两份（`internal/provider/fixtures_test.go:23`、`:127`，`internal/turn/fixtures_test.go:53`、`:137`），本刀在 app 包加第三份，由新文件的注释登记。既有的「几处要一起改」计数（`smoke_test.go:70-73`、`session_inflight_test.go:82`、`rba_fakecli_test.go:41-43`）数的都是 claude 的 `claudeT1JSON`，本刀不牵连，不过期。
   - 给 C2 的契约起草人留一句：`13` §6.2 的「`/log` 能看见那条面包屑」现在可以整行断言 `[event=turn_lost]`，不必再只拿 `cmdLog` 的函数体当证据。
6. **真机复验**（Griffin；Windows Terminal + PowerShell 5.1，行式与 TUI 各一遍）：
   1. 配置里 `store_thought = false`，有一个 grok 成员：`/ask` 它一轮，`/log` 里它那一行末尾是 ` [thought_dropped]`；前一行 `session_opening` 面包屑末尾是 ` [event=session_opening]`。
   2. 改回 `store_thought = true`（或删掉这一行），再 `/ask` 一轮，`/log` 里新的那一行末尾**没有** ` [thought_dropped]`，上一轮那一行的标记还在（开关不回溯）。
   3. 一个 agy 成员在 workspace 档被要求越界写一个文件：`/log` 里它那一行是 `- [denied_actions]`（正文被清空）。
   4. 退出重进（重启之后），`/log` 仍能看到上面三处标记。
   5. 没有任何标记的行与改动前逐字节一致（拿一个旧会话 `/log` 对比即可）。
7. **清理**本刀在中转根与 scratchpad 里的全部中间物（本契约文件除外，归档前由主代理决定去留）。
