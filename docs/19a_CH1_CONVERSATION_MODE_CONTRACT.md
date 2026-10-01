# CH1 冻结契约：对话模式（v1.7，2026-09-29）

> **归档说明（2026-09-29，主代理）**：本文是 CH1 的冻结契约，派工时的原件是 `E:\multi_model_cowork\ch1_contract.md`（v1.7，含「定向核验之后的订正」），原样归档；锚点与行号指 HEAD `3d134ee`，本刀 12 个文件在 `dd0c15b` 上逐字节相同。CH1 已于 `4d3dc0b` 落地，真机未复验：
> - 生产代码 8 个文件与 6 处授权测试改动由 grok-4.7 实现。grok 只拿到本文 §1、§3、§5 的摘录，因为全文太大。实现与 §1.1 逐字节相同。
> - 四个 `chx_*_test.go` 由 Opus 测试腿写成。
> - Opus 集成员：§4 的 86 条变异全被点名杀手杀掉，E1-E3 存活，没有机械修正。
> - Opus 独立审查：4 条 minor（见下）。
>
> 判据「想点名却一个都没认出来」前后改了四次（v1.1 按字节，v1.3 收窄，v1.5 按成员判，v1.6 放宽跳过规则，v1.7 显示名不分大小写），每一次的理由都在变更记录里。
>
> **落地后订正（2026-09-29，主代理；读本文之前先读这里）**：
> - **`/add` 的三处注释与提示没跟上 A12**（审查 R0，不花钱）：
>   - `cmdAdd` 头注释仍写「候选是配置里的全部 id 去掉当前在场的成员」；
>   - 只剩总结成员没加入时，`/add` 打「配置里的 agent 都已经在当前会话里了」，`/add <前缀>` 打「X 开头的都已经在当前会话里了」，这两句都是假话。Griffin 的配置在第一次总结之前正是这个状态。
>   - 这三处不在冻结的锚点里，留给后续小刀，代码改动走委派链。
>   - **已由 OB1 修（`3317940`，`19d` 的 P8-P11）**：候选为空、前缀没命中时分清「一个成员都没有」「总结成员已退出」「除总结成员外都在」；总结成员在场时 `/add` 它改说「已经在当前会话里了」。
> - **`@a@b` 紧贴时第二个静默漏问**（审查 R1）：第二个 `@` 左边是 id 字符，ParseMentions 与 `hasStrayAt` 都跳过它，只问第一个，也不打第 4 句。这是 §7.3 第 4 条左边界残余的另一种后果。判据不改；README 写明两个点名之间要空一格。
> - **没点名时 worker、manager 模式与 workspace 权限的成员也会被问**（审查 R2）：`PlanRound` 把它们都收成发言者，所以每一句随手打的话都会让它们带着写盘权限起一次进程。写盘被限在各自的工作目录里，full 档直接拒绝。§7.3 第 2 条取的其实是花钱更多的读法，那一节标题里「都先取了保守读法」对第 2 条不准。「只问 discussion 模式」「只问 read 权限」两个选项交给 Griffin；他今天的成员全是 discussion。
> - **测试腿的 slowA 守卫写法偏离了 §2**（审查 R3）：`chxGuardSlowA` 只在两个成员头都在时才判先后，经审查接受。这样真的漏杀会报成整串不对，不会被记成「前提不成立」。以后的契约照这种写法。
> - **审查员另找的两条等价变异**：`roundCmd` 的 `rest` 置空（`cmdRound` 不读 rest）；`known[p.AgentID] = true` 挪到剔除总结成员之后（配置校验要求总结成员必在配置里）。
> - **AT1 契约的 (l) 已核**：本文最终文本里 AT1 的名字只出现在「不相交」那一句与 v1.7 的核对段；本刀的 `atSkip` 在 `internal/repl`，AT1 的 `at*` 方法在 `internal/tui` 的 `Model` 上，不撞；本刀的文件清单里没有 `internal/tui` 的生产文件，也没有 `mentioncand.go`。集成员核过「候选 = 活跃成员去掉总结成员」在 `mentioncand.go` 与 `autoreply.go` 两处一致：只差一个 `sumID != ""` 守卫，id 不会为空，两者等价。
> - **API 登记**：§6 第 4 步要求在 `09a` 登记 `SetAutoReply`、`app.Options.NoAuto`、`/auto`、`-no-auto`。仓库里没有集中的 API 登记处（CO1 的 `SetConcise` 也只登在 `16b`），所以只登在本文 §1.1 与 README「对话模式怎么用」一节。
> - **`/team use` 会 Join 总结成员：已由 CL1 修**（2026-10-01，`5c4840a`，`00_DECISIONS.md` #50，契约 `19h`）。team 里的总结成员不再被当普通成员加入，打一行说明。


## 变更记录（v1.1、v1.2、v1.3、v1.4、v1.5、v1.6、v1.7；基线 HEAD `3d134ee`）

v1 以 `0adbc3b` 为基线；此后 main 只多一个纯文档提交 `3d134ee`（只改 `docs/00_DECISIONS.md` 的 #47 AT1 补充）。本刀涉及的代码文件在两者之间逐字节相同，所以下文锚点与行号不变，基线一律改写成 `3d134ee`。v1.1 照主代理裁决（`E:\multi_model_cowork\ch1_rulings.md`，依据 W1 审查 F0-F8 与证伪判词 V0-V3）修订；新增与改动的期望值全部来自原型 `E:\Isolating_test\council\ch1_v11\` 实跑，新增变异全部实跑确认点名杀手变红。W1 的变异名与本文编号对照：X1 = M31，X3 = M32，X15 = M33，X16 = M34，X17 = M35，X17b = M35b，V3 的 PU1 = M35c。

- **F0（改冻结代码）**：裸文本里有「想点名却一个都没认出来」的 `@` 时谁都不问，打一行新字面量 `msgAutoStrayAt`；同一条里既有认出的点名、又有没认出的 `@` 时照问认出的那些，先打一行 `msgAutoStrayAtPartly`。判据是新函数 `hasStrayAt`（§1.1 E），与 `ParseMentions` 同一条左边界、同一套取 token 规则，半角 `@` 与全角 `＠`（U+FF20）都算；`a@b.com`、`a＠b.com` 按左边界不算。推翻 v1「解析出零个 mention 就问全员」在这种情形下的行为。改了 §0 第 10 条、新增 §0 第 11 条、§1.1 E 全文、冻结要点 (c) (e) (f)、§3.2、C3 加 11 行（其中 `glued_at` 只把 §0 第 11 条 (b) 的残余钉成实测）、C11 加两行、§4 M36-M48、§6 第 5 步、§7.1 第 12-13 条。
- **F1**：C3 加 `summary_and_member`；§4 加 M32。
- **F2（按 V2 降为 minor）**：C11 改成表，加 `bare_concise`（简洁档；假时钟恰好 1 次，原型实测，理由见 C11），并按「同类的还有没有别处」补 `no_members_concise`、`summary_mentioned_concise` 与 F0 两句的 `stray_concise`、`partly_concise`；§4 加 M33、M34、M45-M48。
- **F3**：新增 C12（`plain` / `mention` 两个子测试，读成员表失败那两支都可测；按「同类的别处」再加 `write_fail`，钉那一支里 `reportErr` 的写屏幕错误）；§4 加 M35、M35b、M35c、M35d，「只在审查时看」删掉 postUser 那一条、订正关于 ctx 的说法；§2 允许引用的辅助加 `installHook`、`requireFired`、`smxHookWriter`。
- **F4**：C5 `auto` 加「恰好 1 行以 `  scanned_to=2` 结尾」；§4 加 M31。
- **F5**：授权 `internal/app/smx_summary_app_test.go` 的 `Options` 字面量加 `NoAuto: true`（§1.1 I6）；§1.2、§5、§6 的处数与对数同步。
- **F6**：`/auto` 打开时按有没有配 `[summary]` 选两句之一（新常量 `msgAutoOnNoSummary`，§3.1）；C9 改成三个子测试；§4 加 M49、M49b。
- **F7**：PlanRound 不改；§7.3 登记两条。
- **F8**：§1.2 的处数改对（六处、四个文件）。
- **v1 open items 的裁定**（主代理）：TUI `/say` 首行出路只记下、行式草稿 `/end` 自动回复——维持；「成员超过 K 个先确认」——不加；judge 行与「没配总结成员」提示每句都打——维持；TUI 收尾行仍写 `/round 结束`——维持。前两条从 §7.3 移进 §7.1 第 15、16 条；后两条在 §7.1 第 3-5 条标「维持」，收尾行那条从 §7.3 删去，judge / 总结提示那条在 §7.3 第 3 条留一句「他真机用过觉得吵再说」。`/team use` 会 Join 总结成员——不在本刀，留 §7.2 第 1 条；REPL 与 Runner 同一个配置指针——留「只在审查时看」（§7.2 第 2 条）。
- **两处与裁决字面不同，主代理核**：(1) `msgAutoStrayAt` 指给用户看 id 的命令写 `/who` 而不是裁决说的 `/add`：`/add` 列的是**不在**当前会话里的配置 agent，能 `@` 到的只有当前会话的成员，`/who` 才列它们。(2) 裁决要求的「可以用 @ 补全」写成「TUI 里敲 @ 会弹出可点的成员」，这是 AT1 的功能，AT1 还没落地；落地顺序见 §6 第 0 步。（v1.2 注：(2) 的措辞在 W1b 修订里改了，「可点的」与 AT1 不符，见下方 W1b 修订 F3。）

### W1b 修订（v1.1 → v1.2，2026-09-28）

W1b 只审了 v1 → v1.1 这次修订，报了两条 major、两条 minor。v1.1 快照：`E:\multi_model_cowork\ch1_contract_v11.md`。原型 `E:\Isolating_test\council\ch1_fix\`：`git archive 3d134ee`（zip）解出，用脚本从本文逐字取出 28 对 `anchor` / `replace` 与 1 个 `file` 打上去（每个锚点在未改动的 archive 上恰好一次，打出 12 个文件），加上 §2.1 的 C3、C11、C13 三个用例（测试侧的 `STRAY` / `PARTLY` 由脚本从本文 §3.2 逐字生成）。v1.2 在原型上重跑过的：`go build ./...`、`gofmt -l ./internal ./cmd`（空）、`go vet ./internal/repl ./internal/app ./internal/tui ./cmd/council`（干净）、`go test -count=1 ./internal/repl`（整包）、C3 / C11 / C13 `-count=3`、C3 的 slowA 七行 `-count=10`；§4 里杀手落在 C3 / C11 / C13 的 32 条变异逐条、逐个杀手单跑（清单见 §4 表头的说明），另核 M50-M55 让 v1.1 的 C3 28 行、C11 8 行全部存活。**没重跑的**：C1、C2、C4-C10、C12 与 tui、app、cmd 三个包的用例（v1.2 没改它们的期望，也没改它们走到的代码；全量 `go test ./...` 也没跑，只跑了 `./internal/repl` 整包）；杀手不在 C3 / C11 / C13 的 32 条变异（M1-M9 里除 M7b 以外的 9 条、M14-M25 共 15 条、M27、M29、M35、M35b、M35c、M35d、M49、M49b）；E1-E3。原型不交付，已删除。

- **F1（major，照改）**：没认出的 `@` 夹具原来全在文本第 0 字节，左边界检查在 `i > 0` 处短路、从没真正求值。C3 加 9 行：`mid_space_display`、`mid_space_fullwidth`、`mid_cjk_display`、`line2_display`、`line2_fullwidth`、`line2_mention`（第 2 行的正对照）、`stray_after_mention`、`glued_left_cjk_tail`、`ps_hashtable`。期望在原型上实跑。§4 登记 W1b 的 X1-X6 为 M50-M55，每个点名的杀手单跑都红；C3 v1.1 的 28 行与 C11 8 行对这六条全部存活（W1b 实测，v1.2 在原型上复核）。
- **F2（major，方向题，取保守读法、登记待主代理）**：`hasStrayAt` 把代码、PowerShell、SQL、IM 里的 `@`（`@{`、`@(`、`@"`、`@@`、`@_x`、`` `@` ``、`/@张三`、`@所有人`）都当成想点名，这类消息谁都不问。W1b 给了两个方向：(A) 收窄判据，这些回到问全员；(B) 保留判据、写明并给出路。(A) 是花钱方向，本次不定，取 (B)，(A) 登记 §7.2 第 8 条待主代理。(B) 落实为：`msgAutoStrayAt` 加一段出路「这条本来就不是点名的话，接着敲 /round <成员 id...> --go 让成员回复」（改冻结字面量；这条出路新增 C13 实测：成员读到的正是那一条，`scanned_to=2`）；C3 加 `sql_identity`、`at_everyone_cn`（`ps_hashtable` 已在 F1 里），把现状钉成实测；§7.1 第 12 条的理由改写（贴进来的代码删不掉 `@`，代价是多敲一条 `/round`，不是「改写法重发」）；§7.3 第 5 条换成实测清单；README 那句（§6 第 4 步）同步；§0 第 11 条 (a) 的 NONE 一支（`@Override`、`@property`、`npm i @types/node`、`@all`）登记进 §7.3 第 6 条。
- **F3（minor，照改）**：两句说明改了三处。「TUI 里敲 @ 会弹出可点的成员」改成「TUI 里敲 @ 会列出成员的 id 与显示名」（AT1 不做鼠标点选，上下键选、Tab 或回车补全，每行 id 与显示名；AT1 契约写明那句措辞归 CH1 改）；「/who 能看到成员 id」改成「/who 第一列是成员 id」，并在前面写明「后面紧跟成员 id，不是显示名」（`/who` 的列是 `AGENT`、`JOINED_PROVIDER`、`ACTIVE`、`JOINED_AT`、`LEFT_AT`、`CONFIG_BRAIN`，没有显示名；本仓库没有一条命令同时列当前成员的 id 与显示名）；左边界写成充分条件「半角 @，前面空一格或在行首」（C3 `line2_mention` 钉「第 2 行行首」也算行首）。TUI 半句仍放在两句末尾，§6 第 0 步 (b) 仍是整段删掉末半句。
- **F4（minor，照改）**：花钱方向的残余按完整的类写：`@` 或 `＠` 左边紧贴 ASCII 字母、数字或 `_ @ . -` 时都照问全员。§0 第 11 条 (b)、§7.3 第 4 条、README 那句同步，列出实测例子。C3 加 `glued_fullwidth`（`hi＠smx_b 看一下`）。
- **顺带订正（「同类的还有没有别处」查到）**：v1.1 的 C3 判别力说 `glued_at`「不杀任何变异」，不对：M38（「含 `@` 就算」）下 `hi@smx_b` 也被当成没认出的 `@`，打 `STRAY`，原型实测红。已改写，并把 `glued_at` 加进 M38 的杀手；`glued_left_cjk_tail`、`glued_fullwidth` 加进 M39 的杀手；新增的 C13 第 1 步在 M31（先问一轮再落库）下整串反序、在 M50 下照问 smx_a，加进这两条的杀手（都是原型实测红）。M50-M55 另在原型上把 C3、C11、C13 全部用例各跑一遍，红的集合与 §4 的杀手栏恰好相同（「只被 X 杀」是实测的）。
- **冻结代码的改动只有 `autoreply.go` 一个文件**：`msgAutoStrayAt`、`msgAutoStrayAtPartly` 两个字面量，`msgAutoStrayAt` 的注释加一行，`hasStrayAt` 的注释加一行（新文件 158 行 → 160 行）。28 对锚点一对没动，`hasStrayAt` 的判据一个字节没动。

### v1.3（主代理裁决 `E:\multi_model_cowork\ch1_rulings_v13.md`，v1.2 → v1.3，2026-09-28）

v1.2 快照：`E:\multi_model_cowork\ch1_contract_v12.md`；取快照之后主代理直接改的三处（§6 第 0 步定 (a)、§7.2 第 4 条、§7.2 第 5 条接受 `/who`）原样保留。原型 `E:\Isolating_test\council\ch1_v13\`：`git archive 3d134ee`（zip）解出。先照 v1.2 的 §1.1 用脚本逐字落盘（28 对锚点各恰好一次、1 个新文件、12 个文件），写 C3（40 行）、C5（5 个子测试）、C11（8 行）、C13 四个用例（测试侧的 `NONE` / `EMPTY` / `STRAY` / `PARTLY` 由脚本从本文 §3.2 逐字生成），在 v1.2 上 57 个 PASS 全绿；再从同一个 zip 新解一份、照本版 §1.1 重打，把 v1.2 的测试期望原样搬上去，红的恰好是 C3 `stray_bad_id`、`ps_hashtable`、`sql_identity` 与 C13（裁决书预期的三行翻转，实测一致），然后按下面改期望、加夹具，新期望全部是原型实跑的输出。

- **裁决第 1 条（改冻结代码，方向 (A)）**：`hasStrayAt` 的判据（§1.1 E）改成：半角 `@` 取不出合法 token、**并且**下一个字节是 ≥ 0x80、ASCII 空白或文本结尾才算没认出；全角 `＠` 照旧只看左边界。实现是把 v1.2 的 `if !mentionIDRe.MatchString(tok) {` 一行换成 `if !mentionIDRe.MatchString(tok) && (i+1 == len(text) || text[i+1] >= 0x80 || unicode.IsSpace(rune(text[i+1]))) {`，import 加 `unicode`。ASCII 空白取 `unicode.IsSpace`：空格、`\t`、`\n`、`\r` 之外，`\v`、`\f` 也算，理由写在 §7.1 第 12 条（与 `strings.Fields` 同一组；不花钱的方向；键盘敲不出来），C3 `stray_vtab`、`stray_formfeed` 钉着。取 token 那一步在新判据下不改变结果（那三种字节 `mentionTokenChar` 都为假，token 必为空），按裁决书的字面保留、注释写明是防御性的（§7.2 第 7 条）；因此 v1.2 的 M40、M41、M42 都变成等价，移进等价表 E4-E6，另登记 E7（不跑 id 正则）。裁决书只点了 M42，M40、M41 是按「同类的还有没有别处」查到的。注释删掉 v1.2「代码里的 @{、@(……同样算」那句，改写成新判据的理由，并写明「不许加 `@所有人` / `@all` 就是问全体的语义」。
  - C3：`stray_bad_id`、`ps_hashtable`、`sql_identity` 翻成 `smxS1Want`、时钟 2，改成 `slowA`；加 `double_at`（`@@smx_b 看一下`）、`backtick_at`（`` `@` 这个符号怎么打 ``）、`ps_array`（`$a = @() 然后 $a += 1`），同样问全员；加 STRAY 夹具 `stray_eot`（`你看 @`）、`stray_newline`（`第一行 @\n第二行`），按「同类的别处」再加 `stray_crlf`（`\r`，Windows 粘贴的换行）、`stray_tab`、`stray_vtab`、`stray_formfeed` 与 `price_at`（`价格 @ 5 元`，C13 `stray` 的简洁档对照）。仍是 STRAY 的十行（`stray_display`、`stray_fullwidth`、`stray_space`、`stray_no_members`、`mid_space_display`、`mid_space_fullwidth`、`mid_cjk_display`、`line2_display`、`line2_fullwidth`、`at_everyone_cn`）与三行 PARTLY（`partly`、`partly_none`、`stray_after_mention`）实测照绿，与裁决书预期一致。C3 共 50 行。
  - C13：v1.2 的输入 `$h = @{ … }` 翻成问全员，换成 `价格 @ 5 元`（`@` 仍在句中，C13 照旧杀 M50）；C13 的目的不变。
  - §4：M36 的杀手去掉 `stray_bad_id`；M38 补六行翻转；M50 的杀手去掉 `ps_hashtable`、`sql_identity`，补 `stray_eot`、`stray_newline`、`stray_crlf`、`price_at`、C13 `stray`；M53 换锚点（语义不变）；新增 M56-M64（裁决书点名的六条：去掉收窄、收窄套到全角、空白不算、文本结尾不算、≥ 0x80 不算、换行不算；另加 `\r`、`\t`、`\v` 与 `\f` 三条），每条的点名杀手单跑都红，且 M50-M64 各自在 C3、C5、C11、C13 全部用例下红的集合与杀手栏恰好相同。
  - 改了 §0 第 11 条、冻结要点 (c)、§3.2 说明、§6 第 4 步 README 那句、§6 第 5 步第 7 项（贴 PowerShell 那一步 v1.3 起花钱，换成零花费的 `价格 @ 5 元`，PowerShell 那步标花钱、由 Griffin 定）、§7.1 第 12 条、§7.2 第 7 条、§7.2 第 8 条（已定 (A)）、§7.3 第 5 条（分成「现在问全员」「仍谁都不问」两组）。
- **裁决第 2 条（改冻结字面量）**：`msgAutoNoneMentioned` 末尾加「；这条本来就不是点名的话，接着敲 /round <成员 id...> --go 让成员回复」，与第 3 句那一段逐字相同；注释加两行说明为什么。判据不改。C3 期望含第 1 句的五行（`not_member`、`left`、`summary_mentioned`、`only_summary_mentioned`、`partly_none`）与「同类的别处」C5 `auto_not_member` 在原型上重跑，全绿（期望是符号 `NONE`，整串随 §3.2 更新）。按「同类的别处」给 C13 加子测试 `none`（`npm i @types/node 报错`），把第 1 句的出路也钉成实测；它另在 M12、M13、M31 下红。§3.2、§7.3 第 6 条（已定：加出路，判据不改）同步。
- **与裁决书字面不同的一处（主代理核）**：裁决书第 1 条要把 C11 的 `stray_concise`、`partly_concise` 换成新判据下仍是 STRAY 的输入。它们的输入 `@甲 你好`、`@甲 @smx_a 你好` 里没认出的 `@` 后面是非 ASCII，新判据下本来就仍是 STRAY（原型实跑：两行照绿，M36、M60 下都红，M46、M45 各红一行），所以没换。裁决书这半句的前提（它们翻了）在原型上不成立。
- **新发现，登记待 Griffin**：PowerShell here-string 收尾的 `"@`（后面是换行或空格）在新判据下仍算没认出，整条 here-string 仍谁都不问（§7.3 第 5 条）；`@-smx_b`、`@(smx_b)`、`@"smx_b"` 这类想点名的写法在新判据下问全员（§7.3 第 7 条，新增）。
- **冻结代码的改动只有 `autoreply.go` 一个文件**（新文件 160 行 → 171 行）：import 加 `"unicode"`；`msgAutoNoneMentioned` 字面量与它的注释加两行；`msgAutoStrayAt` 的 v1.2 注释一行改写成两行；`hasStrayAt` 的头注释改写（10 行 → 17 行）、判据那一行改写。28 对锚点一对没动。
- **v1.3 在原型上重跑了的**：`go build ./...`、`gofmt -l ./internal ./cmd`（空）、`go vet ./internal/repl ./internal/app ./internal/tui ./cmd/council`（干净）；全量 `go test -count=1 ./...` 九包全绿；C3 / C5 / C11 / C13 `-count=3`（207 个 PASS）；C3 的十三行 `slowA` `-count=10`（130 次）；§4 杀手落在 C3 / C5 / C11 / C13 的 42 条变异与 E1-E7（清单见 §4 表头的说明）；§7.3 第 4-7 条列出的每一种写法（36 条探针，v1.2 与 v1.3 两个原型各跑一遍）。**没重跑的**：C1、C2、C4、C6-C10、C12 与 tui、app、cmd 三个包的 `TestChx` 用例（原型没写；它们的输入里没有 `@`，或只有认得出的 `@smx_a`，走到的代码与期望 v1.3 都没改）；杀手不在这四个用例的 28 条变异。

### W1c 修订（v1.3 → v1.4，2026-09-29）

W1c 审 v1.3：没有 blocker；裁决书两条的落实 W1c 逐条核过（rulings_check），本次不动；报了 3 条 major、3 条 minor，都出在新判据（半角 `@` 的下一个字节是 ≥ 0x80、ASCII 空白或文本结尾才算没认出）的边界上。v1.3 快照：`E:\multi_model_cowork\ch1_contract_v13.md`（逐字节拷贝）。原型 `E:\Isolating_test\council\ch1_fix3\`：`git archive 3d134ee`（tar）解出，用脚本从本文逐字取出 28 对锚点与 1 个新文件打上（每个锚点恰好一次，12 个文件，`autoreply.go` 171 行）；C3、C5（五个子测试都写了）、C11、C13 由脚本从本文的表格与 §3.2 逐字生成。W1c 的白盒与端到端探针在本原型上逐条复现：白盒 114 条输入，冻结的 `hasStrayAt` 与本地照抄的 v1.3 判据零分歧（v1.2 判据也用本地照抄）；端到端 13 条（成员 smx_a、smx_b，简洁档，问全员的几条用 `slowA`），结果与白盒分桶一致；另跑 `/round <成员 id...>` 模板照字面敲的 3 条。

处理原则（本任务的纪律）：成立的照改；花钱与不花钱之间的取舍不在修订里定，取裁决书的读法（判据照 v1.3），写进 §7.2 标「待主代理」，并把每个方案在原型上当作变体实跑一遍，写出它会翻哪几行冻结期望。

- **F1（major，成立；方向待主代理，§7.2 第 9 条）**：提示句里的 `@<成员 id>` 照字面敲成 `@<smx_b> 看一下` 会问全员加一次总结（v1.2 下是第 3 句）。判据与字面量都不改：按裁决书的读法 `<` 是「别的 ASCII」，回到问全员；§3.2 两句是主代理接受过的冻结字面量（§7.2 第 5 条）。C3 加 `angle_template`，把现状钉成实测。两个出路写进 §7.2 第 9 条：(a) `<` 也算没认出，原型变体实跑，C3 / C5 / C11 / C13 全部用例里只翻 `angle_template`；(b) 提示去掉尖括号，要改冻结字面量，没跑。同类的另一半 `/round <成员 id...> --go` 照字面敲不花钱：`/round <smx_a> --go` 打「拒绝: 不是当前活跃成员: <smx_a>」、不起进程（原型实测，简洁、详细两档相同），不用改。§0 第 11 条、§3.2 说明、§7.3 第 7 条同步。
- **F2（major，成立；文档照改，改判据待主代理，§7.2 第 10 条）**：numpy / torch 的矩阵乘 `A @ B` 仍谁都不问，契约没登记；§0 第 11 条与 README 那句拿「Python `@_cache`」当例子，读者会以为贴 Python 就问全员。§0 第 11 条、§3.2 说明、§6 第 4 步 README、§7.1 第 12 条、§7.3 第 5 条照实写：`@_cache` 换掉，写明矩阵乘、装饰器、splatting 仍谁都不问，出路是多敲一条 `/round`。改判据的两个方向写进 §7.2 第 10 条：(b) `@` 左右都是 ASCII 空白时不算，原型变体实跑，翻 `price_at`、`stray_newline`、`stray_crlf` 与 C13 `stray`（与 W1c 按判据推的一致，现在是实测）；(c) 只在空白后面紧跟成员 id 时才算，要改签名，没跑。`@smx_b y = A @ x` 的第 4 句误报（端到端实测）登记进 §7.3 第 5 条。
- **F3（major，成立；方向待主代理，§7.2 第 11 条）**：`@` 左边紧贴 ASCII 符号、右边是空白或文本结尾的代码写法仍谁都不问：here-string 收尾的 `"@` / `'@`，Makefile 与 bash 的 `$@`，Mathematica 的 `/@`。W1c 建议主代理现在就定；这是把「谁都不问」改成「问全员」，本修订不定，把 W1c 的方案与实测影响写进 §7.2 第 11 条：方案在原型上作为变体实跑，现有 C3 / C5 / C11 / C13 一行不翻；探针里九条不是点名的翻成问全员，四条想点名的（`"@甲"`、`'@甲'`、`/@甲`、`$@甲`）也翻。§7.3 第 5 条补 `$@`、`/@`，here-string 那句改成指向 §7.2 第 11 条；README 同步。
- **F4（minor，照改）**：§7.3 第 6 条补 PowerShell splatting、Python 装饰器等十三条探针（`Get-ChildItem @params`、`@staticmethod` 另做了端到端）；README 写明 splatting 与装饰器谁都不问。判据不改。
- **F5（minor，照改）**：§7.1 第 13 条加 v1.3 的例外（「不能静默吞掉」对 `@(smx_a)` 这一类不成立）；§7.3 第 7 条登记「与已认出的点名混用时静默少问」；C3 加 `partly_ascii` 钉成实测，正对照是 `stray_after_mention`。
- **F6（minor，照改）**：§7.3 第 7 条把这一类写成「`@` 后面紧跟不是空白的 ASCII 字节、又取不出合法 id」，附 29 种写法的探针清单，点名 markdown 复制来的 `@**smx_b**`、`` @`smx_b` `` 与 `@_`、`@.` 开头的两种（它们是 token 字符，不是符号）。判据不改。
- **顺带（「同类的还有没有别处」）**：§6 第 5 步给 Griffin 的真机步骤用 `@<成员乙>` 这类占位，照 F1 连尖括号一起敲会问全员，在步骤开头加一句「连尖括号一起换掉」；§6 第 4 步 README 那句加一句「演示点名用真实 id，不用 `@<id>` 占位」。§6 第 2 步的 `-count=10` 清单加 `angle_template`；C3 说明的 `slowA` 清单同步；v1.3 C3 说明里「M56 只被上面翻转的六行杀」补上两行新夹具。在 `4908f13` 的全部 Go 文件与 `docs/README.md` 里另查了一遍 `@<`：只在两处注释里出现（AT1 的 `tui/model.go` 写 `"@<id> "`，补全插入的是真实 id；`turn/router.go` 是历史说明），用户可见的只有本刀 §3.2 这两句。

**冻结代码与字面量一个字节没动**：§1.1 的 28 对锚点、`autoreply.go` 全文（171 行）、§3 的字面量都没改。改的只有 C3 加两行（共 52 行）、§4 M38 与 M56 的杀手栏（各补 `angle_template`、`partly_ascii`），以及上面列的文字。修订完之后从本文重新取出锚点与新文件，分别打在新解出的 `3d134ee` 与 AT1 落地后的 `4908f13`（与 AT1 的 `4202b6c` 代码相同，只多两处文档）上，两份打出的 12 个文件与原型逐字节相同（SHA256 比对）。

**v1.4 在原型上重跑了的**：`go build ./...`、`gofmt -l ./internal ./cmd`（空）、`go vet ./internal/repl ./internal/app ./internal/tui ./cmd/council`（干净）；全量 `go test -count=1 ./...` 九包全绿；C3 / C5 / C11 / C13 `-count=3`（213 个 PASS）；C3 的十四行 `slowA` `-count=10`（140 次）；§4 杀手落在这四个用例的 42 条变异（M5、M7b、M8-M13b、M26-M28、M30-M34、M36-M39、M43-M48、M50-M64）：锚点各恰好一次，`go test -c` 编过，四个用例全跑一遍记下红的集合，点名杀手里落在这四个用例的 116 个子测试逐个单跑全部变红（v1.3 是 112 个，多出的是两行新夹具在 M38、M56 下），M50-M64 各自红的集合与杀手栏恰好相同，还原后 SHA256 相同；E1、E3-E7 下这四个用例全绿；§7.2 第 9-11 条的三个方案各作为变体跑一遍。另在 `4908f13` 的一份拷贝上打好 12 个文件、放进同一份 C3 / C5 / C11 / C13，`go vet ./internal/repl ./internal/tui` 干净，`go test -count=1 ./internal/repl ./internal/tui` 全绿。**没重跑的**：C1、C2、C4、C6-C10、C12 与 tui、app、cmd 三个包的 `TestChx` 用例（原型没写；它们的输入里没有 `@`，或只有认得出的 `@smx_a`，v1.4 没改它们走到的代码与期望）；杀手不在这四个用例的 28 条变异；E2（改的是 `/add`，C10 原型没写）；§7.2 第 10 条方案 (c)（要改签名）与第 9 条方案 (b)（要改冻结字面量）。

### v1.5（主代理裁决 `E:\multi_model_cowork\ch1_rulings_v15.md`，v1.4 → v1.5，2026-09-29）

v1.4 快照：`E:\multi_model_cowork\ch1_contract_v14.md`（逐字节拷贝，只读）。原型 `E:\Isolating_test\council\ch1_v15\`：`git archive 3d134ee`（tar）解出，先照 v1.4 的 §1.1 用脚本逐字落盘（28 对锚点各恰好一次、1 个新文件、12 个文件，`autoreply.go` 171 行），C3（52 行）、C5（五个子测试）、C11（8 行）、C13（两个子测试）由脚本从 v1.4 的表格与 §3.2 逐字生成，在 v1.4 上 67 个子测试全绿；再换上本版 §1.1 E 与 §3.2、期望先不改跑一遍，红的恰好是 C3 `stray_bad_id`、`at_everyone_cn`、`double_at`、`stray_eot`、`stray_newline`、`stray_crlf`、`price_at`、`angle_template`、`partly_ascii` 九行，与裁决书第 2 条的预期逐行一致（C13 `stray` 的旧输入 `价格 @ 5 元` 另在 C13 的详细档装置上跑一遍，问了 smx_a、`scanned_to=2`，同样翻了，见下）；然后按下面改期望、加夹具，新期望全部是原型实跑的输出。

- **为什么换判据（裁决书第 0 条）**：v1.1 到 v1.4 都靠「@ 后面紧跟的那一个字节」猜这个 @ 是不是想点名，每一轮审查都在两个方向上找到新洞（照提示字面敲的 `@<smx_b>` 问全员；numpy 的 `A @ B`、`$@`、here-string 收尾的 `"@` 谁都不问）。本版改问「这个 @ 是不是冲着一个我们认识的成员去的」。§7.2 第 9、10、11 条随之关掉。
- **新判据（裁决书第 1 条，改冻结代码，只改 `autoreply.go`）**：`hasStrayAt` 换签名 `hasStrayAt(text string, known map[string]bool, names []string) bool`，旧判据整段删掉（取 token 过 id 正则、下一个字节是 ≥ 0x80 / ASCII 空白 / 文本结尾那几支、`unicode` 的 import），不在旧判据上叠。判据全文见 §1.1 E 的注释与 §7.1 第 12 条。「它自己没有被 ParseMentions 认成 mention」的实现是「半角 `@` 的下一个字节不是 ASCII 字母或数字」：左边界允许时，`mentionIDPattern` 只要求首字符是字母数字，剥掉尾点之后结尾一定合（中间的字符都在 `mentionTokenChar` 里），所以「下一个字节是字母数字」与「ParseMentions 认成了 mention」互为充要（读 `mention.go` 推出；这一条缺了 `@smx_b` 会被判成没认出，M73 实测红十三个子测试）。`known` = `r.cfg.Agents` 的全部 key 并上当前会话活跃成员的 id，`names` = 配置里的 `display_name` 经 TrimSpace 之后非空的，都由 `autoReplyRound` 在读成员表之后构造。新增小辅助 `asciiAlnum`，冻结名字表补上。
- **期望的翻转（裁决书第 2 条；逐行实测，与裁决书的预期全部一致，没有一行写进「与裁决书不同」）**：翻成问全员（`smxS1Want`、时钟 2，改用 `slowA`）：`at_everyone_cn`、`price_at`、`stray_eot`、`stray_newline`、`stray_crlf`。裁决书说「可能不翻」的 `stray_tab`、`stray_vtab`、`stray_formfeed` 实测仍是 `STRAY`（`\t`、`\v`、`\f` 被跳过，后面是成员 `smx_b`）。翻成 `STRAY`（时钟 0，去掉 `slowA`）：`angle_template`、`stray_bad_id`（`-` 被跳过）、`double_at`（第二个 `@` 被跳过）。翻成 `PARTLY` + `ONE_B`：`partly_ascii`（W1c 第 4 条的静默少问修好，§7.1 第 13 条的例外说明删掉）。要核仍是 `STRAY` 的九行（`stray_display`、`stray_fullwidth`、`stray_space`、`stray_no_members`、`mid_space_display`、`mid_space_fullwidth`、`mid_cjk_display`、`line2_display`、`line2_fullwidth`）与要核仍问全员的（`email`、`email_fullwidth`、`glued_at`、`glued_left_cjk_tail`、`glued_fullwidth`、`ps_hashtable`、`sql_identity`、`backtick_at`、`ps_array`）实测照绿；`stray_no_members` 会话里没有成员，靠配置里的显示名「甲」命中。
- **新夹具（C3 加 15 行，共 67 行）**：裁决书点名的问全员三行 `numpy_matmul`（`y = A @ x + b`）、`bash_dollar_at`（`echo $@`）、`ps_herestring_close`（`$s = @'`、换行、`hello`、换行、`'@`），STRAY 五行 `paren_id`、`backtick_id`、`bold_id`、`space_display`、`config_only`，正对照 `newline_cut`（`@\nsmx_b 看一下`，问全员）。`config_only` 用 `@ smx_x 看一下`：smx_x 在 `smxCfg` 里、装置不加入它；没照裁决书的例子另配一个 `smx_c`，因为 `smxSetup` 的配置是只读引用的既有辅助，改不了。按「同类的还有没有别处」另加六行，各给新判据里没人杀的一支配杀手：`cr_cut`（`@\rsmx_b 看一下`，CR 也截断；`stray_crlf` 在 CR 后面还有 LF，杀不了只漏 CR 的变异，M69b）、`session_only`（成员里有一个配置里没有的 `smx_q`，#45 改名的情形，M68）、`space_dot`（`看一下 @ smx_b.`，M71）、`space_upper`（`@ SMX_B 看一下`，M72）、`fullwidth_everyone`（`＠所有人 看一下`：全角后面不是成员也算；已有三行全角后面都是 `smx_b`，按成员判照样 `STRAY`，杀不了 M70）、`skip_then_display`（`价格 @ 5 元 @甲 看一下`：前一个 `@` 不冲着成员时要接着往后看，M53b）。
- **C13 `stray` 换输入**：v1.4 的 `价格 @ 5 元` 翻成问全员，换成 `看一下 ＠所有人`（全角、在句中；这句本来就想问全体，第 3 句的出路正好用得上）。第 1 步的整串随第 3 句更新；它在 M31、M37、M50、M54、M70 下红（实测）。
- **提示句不再教用户敲尖括号（裁决书第 3 条，改冻结字面量）**：`msgAutoNoneMentioned`、`msgAutoStrayAt`、`msgAutoStrayAtPartly` 三句里的 `@<成员 id>` 与 `/round <成员 id...> --go` 换成文字说法：点名写「半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首」；出路写「接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复」。第 1 句与第 3 句的出路一段仍逐字相同。第 3、4 句原来的「/who 第一列是成员 id」并进前面的括号，末半句 TUI 那段不变。C3、C5 `auto_not_member`、C11、C13 的期望写的是符号 `STRAY` / `PARTLY` / `NONE`，整串随 §3.2 更新、原型重跑全绿。在全文里查过 `<成员 id`：用户可见的字面量里已经没有；剩下的都在说明文字与变更记录里（引用旧字面量）。`/add` 的 `TAIL` 行（`用 /add <编号|全名|前缀> 加入`）与 `/help` 的 `/session rebuild <agent-id>` 是既有命令的用法行，不在本刀，照旧。
- **不改的（裁决书第 4 条）**：「点的不是成员」那一支不改，`@params`、`@staticmethod`、`@Override`、`@all` 照旧谁都不问（端到端：`Get-ChildItem @params` 是 `params 不是当前会话的成员` 加第 1 句、时钟 0）。§7.3 第 8 条写给 Griffin。
- **§4**：M56-M64 与 E4-E7 整行删去（锚点都在删掉的判据里，编号空着不复用）；M36、M38 换锚点（`hasStrayAt(text, known, names)`）；M53 换锚点（「认出的点名之后就不再往后看」，仍是「第一个合法起头的 `@` 定胜负」）；M36、M38、M50-M55 的杀手栏按新判据重审，写成实测红的集合。新登记十四条：M53b、M65-M76 与 M69b，每条点名杀手单跑都红。裁决书点名的七条对应：去掉按成员判 = M65，去掉显示名那一支 = M66，匹配集合只用会话成员 = M67，只用配置 = M68，换行不截断 = M69（CR 另登记 M69b），全角也按成员判 = M70，不剥尾点 / 不折小写 = M71 / M72（都不等价，`space_dot`、`space_upper` 各杀一条）。按「同类的别处」另加五条：M53b（第一个没认出的合法 `@` 定胜负）、M73（去掉「认成 mention 就跳过」）、M74（非 ASCII 也跳过）、M75（字母数字也跳过）、M76（空显示名也参与）。
- **审查的收口规则（裁决书第 5 条；W1d 审查员照这个定严重度）**：这是判据的第四次修订，要有出口。只有两类算 blocker 或 major：冲着一个认识的成员去的 `@` 却问了全员（花钱）；一个认出的成员被静默漏问。别的探针结果（不冲着成员的写法问了全员、个别写法谁都不问）一律是 minor，记进 §7.3，照样发。
- **冻结代码的改动只有 `autoreply.go` 一个文件**（171 行 → 205 行）：import 去掉 `"unicode"`；三句字面量与它们的注释；`autoReplyRound` 头注释一处措辞、构造 `known` 与 `names`（成员循环里加一行 `known[p.AgentID] = true`）、调用改成 `hasStrayAt(text, known, names)`；`hasStrayAt` 的头注释与函数体整段换掉（注释里写明一个 `@` 不算就接着看后面的 `@`，这是 M53、M53b 与 `skip_then_display` 钉的那一条）；新增 `asciiAlnum`。28 对锚点一对没动。
- **改的文字**：§0 第 10、11 条，§1.1 冻结的名字，冻结要点 (c)，§2 C3 表后的说明（新写 v1.5 一段；前几版说明里提到已删变异、已翻期望的地方随手订正），C11、C13 的判别力，§3.2 说明，§4 表头说明、表后说明与「只在审查时看」，§5 的行数，§6 第 2 步的 `-count=10` 清单、第 3 步的条数、第 4 步 README 那句、第 5 步真机步骤，§7.1 第 12、13 条，§7.2 第 7 条与第 9-11 条（关掉），§7.3 第 5、6、7 条与新增第 8、9 条。

**v1.5 在原型上重跑了的**：`go build ./...`、`gofmt -l ./internal ./cmd`（空）、`go vet ./internal/repl ./internal/app ./internal/tui ./cmd/council`（干净）；全量 `go test -count=1 ./...` 九包全绿；C3（67 行）/ C5 / C11 / C13 共 82 个子测试 `-count=3`（246 个 PASS）；C3 的 21 行 `slowA` `-count=10`（210 次）；§4 杀手落在这四个用例的 47 条变异与 E1、E3（清单见 §4 表头说明的 v1.5 一段）：锚点各恰好一次，`go test -c` 编过，四个用例全跑一遍记下红的集合，红的集合与点名杀手里落在这四个用例的子测试逐个单跑，点名的 214 个全部变红，M36、M38、M50-M55、M53b、M65-M76 各自红的集合与杀手栏恰好相同，E1、E3 下全绿，还原后 SHA256 相同；§4 全表 78 行的锚点在 v1.5 原型上各恰好一次；另把去掉显示名 TrimSpace 的变异跑了一遍（82 个子测试全绿，记进「只在审查时看」）。探针约 50 条（端到端，成员 smx_a、smx_b，简洁档，问全员的用 `slowA`），结果写在 §7.2 第 9-11 条的关闭说明与 §7.3 第 5-9 条。§1.1 冻结代码块的符号禁令扫描 0 命中，全角 `＠` 只在注释里。修订完之后从本文重新取出 28 对锚点与新文件，分别打在新解出的 `3d134ee` 与 AT1 落地后的 `4908f13` 上，两份打出的 12 个文件与原型逐字节相同（SHA256 比对）；在 `4908f13` 那一份上放进同一份 C3 / C5 / C11 / C13，`gofmt -l`（空），`go vet ./internal/repl ./internal/tui ./internal/app ./cmd/council` 干净，`go test -count=1 ./internal/repl ./internal/tui` 全绿。顺序说明：变异、`-count=3`、`-count=10` 跑完之后，`hasStrayAt` 的头注释补了两行（「一个 @ 不算，就接着看后面的 @」），代码一个字节没动；补完之后重跑了 gofmt、vet、全量 `go test -count=1 ./...`（九包全绿）、78 行锚点核对、符号扫描、两个基线的逐字节比对与 `4908f13` 上的 repl、tui 两包，变异没有重跑。**没重跑的**：C1、C2、C4、C6-C10、C12 与 tui、app、cmd 三个包的 `TestChx` 用例（原型没写；它们的输入里没有 `@`，或只有认得出的 `@smx_a`，`hasStrayAt` 的结果不变，但 `autoReplyRound` 多了构造匹配集合的几行，这几条路径没有端到端复核）；杀手不在这四个用例的 28 条变异；E2（改的是 `/add`，C10 原型没写）。

### W1d 修订（v1.5 → v1.6，2026-09-29）

W1d 审 v1.5（只审 v1.4 → v1.5 这次修订）：裁决书第 1-6 条的落实逐条核过（rulings_check），本次不动；报了 2 条 major、2 条 minor。收口规则第 (1) 类（冲着认识的成员去、却问了全员）有两处，第 (2) 类（认出的成员被静默漏问）没找到。v1.5 快照：`E:\multi_model_cowork\ch1_contract_v15.md`（逐字节拷贝）。原型 `E:\Isolating_test\council\ch1_fix5\`：`git archive 3d134ee`（tar）解出，用脚本从 v1.5 逐字取出 28 对锚点与 1 个新文件打上（每个锚点恰好一次，12 个文件，`autoreply.go` 205 行），C3（67 行）、C5（五个子测试）、C11（8 行）、C13（两个子测试）由脚本从 v1.5 的表格与 §3.2 逐字生成，82 个子测试全绿（与 W1d 相同）；W1d F1 的白盒探针在这份 v1.5 原型上逐条复现，都不算（问全员），与 W1d 说的一致。然后从同一个 tar 新解、照本版 §1.1 重打，按下面改。

处理原则（本任务的纪律）：收口规则那两类的 major 照改，新期望原型实跑、新变异实测杀手变红；minor 只记进 §7.3、不改判据；涉及花钱方向、裁决书已经写明了的，照裁决书，写进 §7.2 标「待主代理」。

- **F1（major，成立，照改；改冻结代码，只改 `autoreply.go`）**：v1.5 的跳过循环只跳 ASCII 字节，`@` 后面先跟一个非 ASCII 的分隔符（全角括号、引号、冒号、逗号，U+3000，NBSP，零宽字符）再跟成员 id 或显示名时，取不出 token、显示名也对不上，这句问全员加一次总结；与认出的点名混用时只问认出的那个、不打第 4 句。这是相对 v1.4 的回退（v1.4 快照 §7.3 第 7 条把这些列为「对照（不花钱）……都是第 3 句」）；v1.5 的 §7.1 第 13 条说「修好」、§7.3 第 7 条说「都没有了」，对这一类不成立。改法：跳过那一段挪进新函数 `atSkip(s string) int`（冻结名字表补上）：ASCII 照旧，除了字母、数字、CR、LF 都跳；非 ASCII 按 rune 解码，只跳 `unicode.In(r, unicode.Zs, unicode.P, unicode.S, unicode.M, unicode.Cf)` 这五类，字母、汉字、数字不跳（显示名与 id 要从它们开始比）。import 加回 `"unicode"` 并加 `"unicode/utf8"`：裁决书第 1 条删掉的是旧判据（按下一个字节判空白）连同它的 import，这里的 `unicode` 是新判据的跳过规则用的，不是两套判据叠在一起。与 W1d 的建议有三处不同，都是按「同类的还有没有别处」查到的：
  - 多跳符号 S 与组合记号 M。W1d 建议只跳空白、标点、Cf；中文输入法全角标点下敲出的 `＜ ＋ ～ ｜ ￥`（S）与 emoji（S，后面常跟变体选择符 U+FE0F，是 M）在 v1.4 下都算第 3 句，只跳空白、标点、Cf 时 `@＜smx_b＞`、`@` 加 emoji 加 `smx_b` 仍问全员（M80「S 不跳」下 C3 `fw_angle_id`、`emoji_vs_id` 实测都问全员；v1.5 原型上两种白盒都不算）。类别由原型里一段 Go 探针打出来定，不凭记忆：U+FF1C、U+FF0B、U+FF5E、U+FF5C 是 Sm，U+FFE5 是 Sc，U+1F600、U+2764 是 So，U+FE0F 是 Mn，U+200B、U+200D、U+2060、U+FEFF 是 Cf，U+3000、U+00A0、U+2002、U+2005 是 Zs。
  - 用 Zs 而不是 `unicode.IsSpace`：`IsSpace` 含 U+0085、U+2028、U+2029 这三个换行，用它就得另写一张停止表。U+2028 是 Zl、U+2029 是 Zp、U+0085 是 Cc，都不在五类里，自然停下，与 CR / LF 一致（白盒三种都不算；端到端 U+2029、U+0085 两条探针与 C3 `ls_cut` 都问全员）。
  - 显示名改成在跳过途中每个位置都比（含 `@` 紧后面、还没跳的时候），不再只在跳完之后比一次。原因是显示名自己可能以会被跳过的字符开头：v1.5 对以 ASCII 标点开头的显示名（`(bot) X`、`[AI] Opus`）就对不上（v1.5 原型白盒实测 `@(bot) X 看一下` 不算），W1d 没报；照 W1d 的写法只放宽跳过、仍在跳完之后比，以全角标点开头的显示名（`【测试】乙`、`「乙」`）也会从 v1.5 的「算」变成「不算」，是修法自己带来的新回退。`smxCfg` 里没有这种显示名，新加白盒用例 C14 `TestChxHasStrayAtNames`（七行）钉它；M84（显示名只在跳完之后比，就是 W1d 建议的写法）被 C14 四行杀。
  - C3 加 9 行（共 76 行）：`fw_paren_id`、`ideo_space_id`、`zwsp_id`、`fw_angle_id`、`emoji_vs_id`、`ideo_space_display`（`STRAY`）、`partly_fullwidth`（`PARTLY` + `ONE_B`）、`ls_cut`（问全员，`slowA`），外加 F2 的 `display_case`；前 7 行拿到 v1.5 原型上跑都红。不可见字符在表里写 `{U+XXXX}`，Go 源码里拼 `string(rune(0x…))`；没有用 U+200D。`slowA` 21 行 → 23 行，§6 第 2 步同步。
  - §4：M53b、M66、M69、M69b、M74、M75 换锚点；新增 M77-M84 八条；M36、M38、M43、M44、M50、M53、M65、M66、M74、M75、M76、M77、M79 的杀手栏按实测补。§6 第 3 步：M1-M84 共 83 条。
  - 改的文字：§0 第 11 条，§1.1 冻结的名字，冻结要点 (c)，§2 通用纪律（C14 白盒），C3 表后说明（`{U+XXXX}` 记法、`slowA` 清单、v1.6 一段），C14，§4 表头说明、表后说明与「只在审查时看」（非法 UTF-8），§5 的行数，§6 第 2-5 步，§7.1 第 12、13 条，§7.3 第 5、7 条（删掉「这一类的花钱代价与静默少问都没有了」，照 v1.6 的重跑重写）。
- **F2（W1d 定为 major；根源在裁决书第 1 条，照裁决书，判据不改，登记 §7.2 第 12 条待主代理）**：显示名「逐字节比，不折大小写」是裁决书第 1 条写明的，修订不推翻。把现状钉成 C3 `display_case`（`@ 坏 x 看一下` 问全员，`slowA`；照 v1.4 `angle_template` 的先例只钉现状、不登记变异）；W1d 的方向 (a) 在 v1.6 原型上作为变体实跑：98 个子测试里只翻 `display_case`，白盒另翻 `@ opus-5.0- 看一下`、`@ opus-5.0-maxx`、`@ opus-5` 三种（显示名按前缀比那一类，往谁都不问那一侧，不花钱）。(a)、(b) 两个方向与采纳时要改的地方写在 §7.2 第 12 条。§7.3 第 9 条第 2 小条的归类理由改掉（不再说按裁决书第 5 条算 minor，指向 §7.2 第 12 条）。
- **F3（minor，照记，判据不改）**：§7.3 第 9 条加一小条（显示名里的空格多敲、少敲，或 id 用全角字母敲）。探针在 v1.6 原型上重跑：`@ 总结S 看一下`、`@ 总结  S 看一下`、`@ｓｍｘ_ｂ 看一下` 端到端问全员；ASCII 显示名配置的三种白盒不算，三种正对照白盒算。
- **F4（minor，照记，判据不改）**：§7.3 第 9 条加一小条（`@` 后面隔着空格跟 id 再带尾部 `-` 或 `_`）。`@ smx_b- 看一下`、`@ smx_b_ 看一下` 端到端问全员，不带空格的 `@smx_b- 看一下` 端到端是「smx_b- 不是当前会话的成员」加第 1 句、时钟 0；ASCII 配置的三种白盒不算。

**冻结代码的改动只有 `autoreply.go` 一个文件**（205 行 → 233 行）：import 加 `"unicode"`、`"unicode/utf8"`；`hasStrayAt` 注释里讲跳过与比较的那一段改写（5 行 → 8 行）；函数体里跳过与显示名那两段换成一个循环：每个位置先比显示名，再用 `atSkip` 跳一个字符，跳不动就停（显示名那一段挪到取 token 之前）；新增 `atSkip`；`asciiAlnum` 的注释改一处（「只给 hasStrayAt 与 atSkip 用」）。四句字面量、`autoReplyRound` 与 28 对锚点一个字节没动。

**v1.6 在原型上重跑了的**：变异跑完之后冻结代码没有再动。在最终的本文上重新取出 28 对锚点与新文件，打在新解出的 `3d134ee` 上：`go build ./...`、`gofmt -l ./internal ./cmd`（空）、`go vet ./internal/repl ./internal/app ./internal/tui ./cmd/council`（干净）；全量 `go test -count=1 ./...` 九包全绿；C3（76 行）/ C5 / C11 / C13 / C14 共 98 个子测试 `-count=3`（294 个 PASS）；C3 的 23 行 `slowA` `-count=10`（230 次）；§4 杀手落在这五个用例的 55 条变异（v1.5 的 47 条加 M77-M84）逐条跑过，点名杀手 225 个子测试逐个单跑全红，杀手栏补过的 13 条又重跑一遍（174 个单跑全红），E1、E3 下全绿，还原后 SHA256 都相同（清单与结果见 §4 表头说明的 v1.6 一段）；§4 全表 86 行的锚点各恰好一次；「只在审查时看」新加的非法 UTF-8 那一处作为变异跑过（98 个子测试全绿）；§7.2 第 12 条方案 (a) 作为变体跑过；探针：白盒约 90 条（`smxCfg` 的 known / names 照 `autoReplyRound` 的写法构造，另一份 ASCII 显示名配置，另几组自定义显示名），同一份探针在 v1.5 原型上也跑一遍做对照；端到端 18 条（成员 smx_a、smx_b，简洁档，问全员的用 `slowA`）。§1.1 冻结代码块（57 块）的符号禁令扫描 0 命中，全角 `＠` 只在注释里（4 行）。两个基线：同一份本文分别打在新解出的 `3d134ee` 与 `4908f13` 上，每个锚点在未改动的文件里恰好一次，打出的 12 个文件两边逐字节相同，也与跑变异、探针的那一份原型逐字节相同（SHA256）；在 `4908f13` 那一份上放进同一份 C3 / C5 / C11 / C13 / C14，`gofmt -l`（空），`go vet ./internal/repl ./internal/tui ./internal/app ./cmd/council` 干净，`go test -count=1 ./internal/repl ./internal/tui` 全绿。**没重跑的**：C1、C2、C4、C6-C10、C12 与 tui、app、cmd 三个包的 `TestChx` 用例（原型没写；v1.6 只改 `hasStrayAt` 与新增的 `atSkip`，这些用例的输入里没有 `@`，或只有认得出的 `@smx_a`）；杀手不在这五个用例的 28 条变异；E2（改的是 `/add`，C10 原型没写）；`4908f13` 之后 main 上的 `ee247c6`、`dd0c15b`（AT1-FU：`git diff --stat 4908f13 dd0c15b` 只有 `internal/tui/model.go`、新测试 `internal/tui/atf_review_test.go` 与三份文档，不碰本刀 12 个文件）上没有重打。

### v1.7（主代理裁决 `E:\multi_model_cowork\ch1_rulings_v17.md`，v1.6 → v1.7，2026-09-29）

v1.6 快照：`E:\multi_model_cowork\ch1_contract_v16.md`（动手前逐字节拷贝，SHA256 与改动前的本文相同）。原型 `E:\Isolating_test\council\ch1_v17\`：`git archive 3d134ee` 与 `git archive dd0c15b`（tar）各解一份。先照 v1.6 的 §1.1 用脚本逐字落盘（28 对锚点各恰好一次、1 个新文件、12 个文件，`autoreply.go` 233 行），C3（76 行）、C5（五个子测试）、C11（8 行）、C13（两个子测试）、C14（7 行）由脚本从 v1.6 的表格与 §3.2 逐字生成，98 个子测试全绿；再只换显示名比较那一行（裁决书的写法）、期望先不改跑一遍，红的恰好只有 C3 `display_case`（输出变成第 3 句，`smxRequireBBeforeA` 前提不成立），与裁决书的预期一致；然后照下面改。新期望全部是原型实跑的输出。锚点基线仍是 `3d134ee`；逐字节核对的第二个基线从 `4908f13` 换成当前 main `dd0c15b`。

- **裁决书第 1 条（改冻结代码，只改 `autoreply.go`）**：`hasStrayAt` 显示名那一支的 `if strings.HasPrefix(text[j:], name) {` 换成 `if len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {`（W1d 实跑过的写法），「跳过途中每个位置都比」照旧。注释里讲显示名比较的那一小条从 2 行改成 5 行，写明不分大小写与切片安全的理由。import 不变。新文件 233 行 → 236 行。
- **切片安全（裁决书要求写成实测的事实）**：比法取「只靠长度守卫、按显示名的字节长度切」，不另判 `utf8.ValidString`，也不按 rune 边界走。依据是原型上的白盒探针与一段只调 `strings.EqualFold` 的 Go 探针：
  - 长度守卫挡住越界：剩下的文本比显示名短时不切。去掉守卫（M86）时，`你看 @` 必定切片越界 panic；`看一下 @甲` 只在比「甲」长的显示名先被比到时才 panic。`names` 由 map 遍历构造，顺序随机，定向核验人实测约 6/7 panic。
  - 切口落在多字节字符中间时不算：显示名 `Opus`、剩下 `Op坏 看一下`；被截断的「甲」（`\xe7\x94`）；截断的「甲」后面跟 `A`。切出来的那一段不是合法 UTF-8，`EqualFold` 把每个坏字节当 U+FFFD（Go 探针：`strings.EqualFold("\x80", string(rune(0xFFFD)))` 为 true），对不上显示名里对应的字符。结果与 v1.6 逐字节比相同，都不 panic。
  - 与 v1.6 不同的非法 UTF-8 情形只有一种，要三件事同时成立：显示名自己含 U+FFFD；文本在那个位置是一个坏字节；后面还有一个折叠前后字节数不同的字符把长度补齐。探针：显示名是 U+FFFD 加 `k`，文本是 `@ `、0x80、开尔文符号 U+212A，v1.7 算、v1.6 不算。它只往谁都不问那一侧偏，不花钱。
  - 加 `utf8.ValidString` 的变体在原型上实跑，五个用例 100 个子测试全绿，没有用例能区分，写进 §4「只在审查时看」。
- **与裁决书字面不同的一处（主代理核）**：裁决书标题写「ASCII 大小写不敏感」，正文指定的 `strings.EqualFold` 按 Unicode 的简单大小写折叠比，不只 ASCII。照「以 W1d 实跑过的写法为准」落实成 `EqualFold`，没有另写只折 ASCII 的比较；注释写「不分大小写，用 strings.EqualFold 比」，不承诺「只折 ASCII」。白盒实测：字节长度相同的非 ASCII 大小写也算（`é` 与 `É`、`σ` 与 `Σ`、`д` 与 `Д`、全角 `ａ` 与 `Ａ`）；字节长度不同的不算，与 v1.6 相同（开尔文符号与 `k`、`ſ` 与 `s`、`ẞ` 与 `ß`、`ı` 与 `I`）。多算的都在谁都不问那一侧，不花钱；写进 §7.1 第 12 条与 §7.3 第 9 条第 2 小条。主代理要只折 ASCII，得换一种写法、原型重跑。
- **C3**：`display_case` 翻成 `STRAY`、时钟 0、去掉 `slowA`（`slowA` 23 行 → 22 行，§6 第 2 步同步）。C3 仍是 76 行。
- **同装置对照（裁决书第 1 条）**：`smxSetup` 的配置是只读引用的既有辅助，去不掉「坏 X」，改在 C14 白盒里做。C14 加两行，共 9 行：`display_case_hit` 的 names 是 `smxCfg` 里全部七个非空显示名，`@ 坏 x 看一下` 算；`display_case_miss` 是同一份去掉「坏 X」，不算。
- **§4**：新增 M85（显示名逐字节比，回到 v1.6，裁决书点名）、M86（去掉长度守卫，panic）、M87（长度守卫的 `>=` 写成 `>`）。M66、M84 换锚点。M36、M66 补 `display_case`，M66 另补 C14 `display_case_hit`；M38、M76 拿掉 `display_case`。§6 第 3 步：M1-M87 共 86 条。
- **定向核验之后的订正（主代理，2026-09-29 04:10）**：
  - 定向核验人五项全部成立：
    - `display_case` 翻转；同装置对照成立；
    - 切片安全与本文一致；
    - M85、M87、M66、M84 的红集合恰好；
    - 两个基线的 12 个文件逐字节相同，`dd0c15b` 上 repl、tui、app、cmd 全绿；
    - v1.6 → v1.7 的冻结代码只动了显示名比较那一行与它的注释。
  - 唯一一条 minor：M86 下 `mid_space_display` 是否 panic 取决于 `names` 的 map 遍历顺序（分进程单跑 60 次，52 次 panic、8 次绿）。已从 M86 的杀手栏拿掉，只留 `stray_eot`（20/20 次 panic）。本节切片安全一条与 §4 表后说明同步改了。只改文字，冻结代码与期望都不动。
- **§7.2 第 12 条**：已定 (a)，写了关闭说明，v1.6 的登记原文保留。**§7.3 第 9 条**：第 2 小条改成「v1.7 已修」并写实测；第 3 小条补 v1.7 下更宽的前缀命中（ASCII 显示名配置里 `@ opus-5.0- 看一下`、`@ opus-5.0-maxx`、`@ opus-5`、`@ opusx 看一下` 被短显示名 `Opus` 命中，方向是不花钱，判据不再改）；第 6、7 小条（F3、F4）照裁决书第 2 条维持 minor、只登记，探针在 v1.7 原型上重跑、结果不变，第 7 小条里那三种白盒改写成 v1.7 的结果。
- **其余照 v1.6**（裁决书第 2 条）：收口规则不变。
- **改的文字**：标题与本节；§0 第 11 条；冻结要点 (c)；C3 表与表后说明（`slowA` 清单、v1.6 一段加注、新写 v1.7 一段）；C14 说明与判别力；§4 表头说明、表后说明与「只在审查时看」；§5 的行数；§6 第 2、3、4、5 步（第 4 步 README 那句加「大小写不同也算」，第 5 步加一步零花费的真机复验）；§7.1 第 12 条；§7.2 第 12 条；§7.3 第 5 条（重跑说明）与第 9 条。下面「仓库」一条补了 `dd0c15b` 的说明。
- **给实现腿的范围（裁决书第 3 条）**：W2 只给 grok §1.1、§3、§5。用脚本从本文逐块核过：
  - 全文的 `anchor` / `replace` / `file` 代码块共 57 块（28 对加 1 个新文件），全部在 §1.1 里。
  - §3 的 5 个 `text` 块是冻结字面量，13 行逐行在 §1.1 的替换块与新文件里原样出现。§5 只有两个补丁格式的示例块。
  - §1.1、§3、§5 指向别处的只有理由与动机（§0 各条、§1.2 的表、§3.2 说明里提到的 §6、§7），没有 grok 要照抄的代码、锚点或字面量落在这三节之外。§2 的 3 个 `text` 块是测试腿的期望，§6 的 18 个块是给 Griffin 的真机步骤，都不是 grok 要的。
- **AT1 的 (l)（裁决书第 3 条）**：
  - 在最终文本里 grep AT1 的名字：`MentionCandidate`、`MentionCandidates`、`MentionBoundaryOK`、`MentionTokenRune`、`mentionName` 只出现在 §1.1 开头那句「本刀的名字与它们不相交」；`mentionSource`、`atState`、`atMaxRows` 与 AT1 的 `at` 方法，除本段之外全文没有出现。`dd0c15b` 的 `internal/tui/model.go` 里 AT1 的 `at` 名字是类型 `atState`、常量 `atMaxRows` 与 `Model` 的方法 `atCursor`、`atArm`、`atRecheck`、`atLine`、`atMatches`、`atVisible`、`atLines`、`atAccept`。
  - 本文里以 `at` 开头的标识符只有本刀的 `atSkip`。它与 AT1 的 `at*` 不撞：AT1 的在 `internal/tui` 包，是 `Model` 的方法；`atSkip` 是 `internal/repl` 包的函数。`dd0c15b` 的 `internal/repl` 全部 Go 文件里没有 `atSkip`，也没有本刀别的新名字（`sayText`、`postUser`、`autoReplyRound`、`cmdAuto`、`hasStrayAt`、`asciiAlnum`、`roundCmd`、`SetAutoReply`、`autoReply`、`msgAuto` 开头的常量）。
  - 本刀的文件清单（§5 与 §1.1 的 12 个目标文件）里没有 `internal/tui` 的生产文件，也没有 `mentioncand.go`。

**v1.7 在原型上重跑了的**：在最终的本文上重新取出 28 对锚点与新文件，打在新解出的 `3d134ee` 上：`go build ./...`、`gofmt -l ./internal ./cmd`（空）、`go vet ./internal/repl ./internal/app ./internal/tui ./cmd/council`（干净）；全量 `go test -count=1 ./...` 九包全绿；C3（76 行）/ C5 / C11 / C13 / C14（9 行）共 100 个子测试 `-count=3`（300 个 PASS）；C3 的 22 行 `slowA` `-count=10`（220 次）；§4 杀手落在这五个用例的 58 条变异（v1.6 的 55 条加 M85-M87）与 E1、E3 逐条跑过，结果见 §4 表头说明的 v1.7 一段；§4 全表 89 行（86 条 M、3 条 E）的锚点各恰好一次；「只在审查时看」新加的 `utf8.ValidString` 那一处作为变体跑过（100 个子测试全绿）；探针：白盒 74 条（`smxCfg` 的 known 与 names 照 `autoReplyRound` 的写法构造；一份 ASCII 显示名配置；切口、非法 UTF-8、非 ASCII 大小写几组自定义显示名），同一份探针在 v1.6 原型上也跑一遍做对照；端到端 10 条（成员 smx_a、smx_b，简洁档，问全员的用 `slowA`）。§1.1 冻结代码块（57 块）的符号禁令扫描 0 命中，全角 `＠` 只在注释里（4 行）。两个基线：同一份本文分别打在新解出的 `3d134ee` 与 `dd0c15b` 上，每个锚点在未改动的文件里恰好一次，打出的 12 个文件两边逐字节相同，也与跑变异、探针的那一份原型逐字节相同（SHA256）。在 `dd0c15b` 那一份上放进同一份 C3 / C5 / C11 / C13 / C14：`gofmt -l`（空）；`go vet ./internal/repl ./internal/tui ./internal/app ./cmd/council` 干净；`go test -count=1 ./internal/repl ./internal/tui ./internal/app ./cmd/council` 四包全绿；另单跑四个授权改动的测试文件（`app_test.go`、`cox_app_test.go`、`fux_review_test.go`、`smx_summary_app_test.go`）里的 7 个测试函数，全绿（12 个子测试）。**没重跑的**：C1、C2、C4、C6-C10、C12 与 tui、app、cmd 三个包的 `TestChx` 用例（原型没写；v1.7 只改显示名比较那一行，这些用例的输入里没有 `@`，或只有认得出的 `@smx_a`）；杀手不在这五个用例的 28 条变异；E2（改的是 `/add`，C10 原型没写）。

---

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `3d134ee`（= origin/main；与 v1 基线 `0adbc3b` 只差一个纯文档提交，代码文件逐字节相同）。v1.7 注：锚点基线仍是 `3d134ee`；此后 main 前进到 `dd0c15b`（AT1 与 AT1-FU 落地），`git diff --stat 3d134ee dd0c15b` 里没有本刀 12 个文件中的任何一个，§1.1 在 `dd0c15b` 上同样逐字打得上（v1.7 实测）。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`（不设 GOFLAGS）。本机没有 gcc，不能 `-race`。E 盘机（Windows 11）。
- 来源：`docs/00_DECISIONS.md` **#47**（:2195-2201，Griffin 定方向「裸文本记下之后自动让成员回复；没点名就是当前会话全部讨论成员；消息里 @ 了谁就只有谁；多个成员回复时照 SM1 自动总结」，主代理定细节）；#44（X44，TUI 多行提交先过 `HasLaterCommandLine`；`/say` 首行出路**留**）、#45（CO1 简洁输出、SM1 自动总结、MD1 模型识别；`/add` 的候选里列出了总结成员 `fable-5.1-summary`，:2153「下一刀从候选里藏掉」）、#46（主代理定的待定项；`/help` 的 `/session` 行 U43A 没收，18b §7.2 第 1 条「以后单独一刀」）。
- 上游契约：`docs/12_STEP9_CONTRACT.md`（/round 的原始设计、「名单为空则拒绝，绝不默认全员」）、`docs/13a_S10_A5_CONTRACT.md`（A 之五在飞闸，本刀一个不绕）、`docs/16b_CO1_CONCISE_OUTPUT_CONTRACT.md`（REPL 档位零值 + app 默认 + `runAppForTest` 退出的先例，本刀开关照抄这个形状）、`docs/16c_SM1_AUTO_SUMMARY_CONTRACT.md`（总结成员、触发判据、名单里出现总结成员整条拒绝）、`docs/17a_X44_MULTILINE_GUARD_CONTRACT.md`、`docs/18b_U43A_SETTLE_STALE_INFLIGHT_CONTRACT.md`（`/session rebuild` 先结清）。
- **与已落地各刀的相交文件与函数**（逐个点名；本刀是这些文件此刻唯一的改动方，落地顺序无要求）：
  - `internal/repl/commands.go`：`cmdArgs`（CO1/SM1 未动）、`sayCmd`（Step 5）、命令表里 `round` / `session` / `verbose` 三行（SM1 改过 round 的 summary，CO1 加了 verbose，CXH 改过 add；本刀只动 round 的写法、session 的 summary、在 verbose 之后追加一行）、`dispatch` 的裸文本分支、`collectDraft` 的 `/end` 分支、`postUserText`（CO1 在这里加了「点名了」的档位门）、`cmdAdd` 的候选循环与 `resolveAddSegment` 第 1 步之前（MD1 / CXH 改的是 `cmdAdd` 开头的 `+` / `--refresh` 两支与 `printDiscoveredModels`，本刀不碰）、`cmdSay` 最后一行。
  - `internal/repl/seam.go`：`Submit` 的裸文本分支（TUI1、X44 未动这一行）。
  - `internal/repl/paste.go`：`finishPaste` 末尾。
  - `internal/repl/repl.go`：REPL 结构体（CO1 加了 `concise`、`now`，MD1 加了 `discover` 等；本刀在 `now` 之后插一个字段）。
  - `internal/repl/round.go`：只改 `cmdRound` 头注释（加「第七次」一段），代码一个字不动。
  - 新文件 `internal/repl/autoreply.go`。
  - `internal/app/app.go`：`Options`（CO1 的 `Verbose` 之后追加 `NoAuto`）、`Run` 末尾两个分支各加一行。
  - `cmd/council/main.go`：`-no-auto` 与 `app.Options` 字面量（CO1 的 `-verbose` 之后）。
  - 授权改动的既有测试：`internal/app/app_test.go`（`runAppForTest` 加一行，同 CO1）、`internal/app/cox_app_test.go`（三处 `Options` 字面量）、`internal/app/fux_review_test.go`（一处 `Options` 字面量）、`internal/app/smx_summary_app_test.go`（一处 `Options` 字面量，v1.1 F5），共四个文件六处，理由见 §1.2。
- **本契约的全部期望值都在隔离拷贝里用一份原型实跑得出**：v1.1 的原型是 `E:\Isolating_test\council\ch1_v11\`（`git archive 3d134ee` 解出；v1 的原型 `ch1_draft` 已删除）。原型 = §1.1 全部生产改动 + §1.2 授权的六处测试改动 + §2 全部新用例（原型的测试文件名与本契约相同）。原型上 `go build ./...`、`gofmt -l ./internal ./cmd`（空）、`go vet ./internal/repl ./internal/app ./internal/tui ./cmd/council` 干净；全量 `go test -count=1 ./...` 九包全绿；§2 全部新用例 `-count=3` 全绿；§4 的变异逐条跑过（锚点恰好一次、从内存字节副本还原、还原后 SHA256 与跑前相同），被点名的杀手全部变红，E1-E3 三条等价变异全部存活。§1.1 的每一对「锚点 / 替换」与新文件由脚本从本文逐字取出、打在 HEAD `3d134ee` 的一份 `git archive` 上，打出来的 12 个文件与原型逐字节相同（见 §6 第 1 步）。原型不交付，已删除。v1.2 新增与改动的期望在原型 `ch1_fix` 上实跑，重跑了什么、没重跑什么见变更记录的「W1b 修订」。v1.3 新增与改动的期望在原型 `ch1_v13` 上实跑，重跑了什么、没重跑什么见变更记录的「v1.3」。v1.4 新增的期望在原型 `ch1_fix3` 上实跑，重跑了什么、没重跑什么见变更记录的「W1c 修订」。v1.5 新增与改动的期望在原型 `ch1_v15` 上实跑，重跑了什么、没重跑什么见变更记录的「v1.5」。v1.6 新增与改动的期望在原型 `ch1_fix5` 上实跑，重跑了什么、没重跑什么见变更记录的「W1d 修订」。v1.7 新增与改动的期望在原型 `ch1_v17` 上实跑，重跑了什么、没重跑什么见变更记录的「v1.7」。
- HEAD 上的基线：同一份 `git archive` 未改动时全量 `go test -count=1 ./...` 九包全绿（v1.1 在 `3d134ee` 上重跑过）。

---

## 0. 病根 / 动机（行号以 HEAD `3d134ee` 为准，与 v1 的 `0adbc3b` 在这些代码文件上相同，均已打开真文件核对）

1. **今天裸文本只记下，要成员说话必须另敲命令。** `internal/repl/commands.go:222-228`：不以 `/` 开头的一行交给 `sayCmd`（`:120-126`），`cmdSay`（`:1123-1136`）调 `postUserText`（`:329-388`）落一条 user 消息就结束。TUI 的 `Submit`（`seam.go:68-74`）、行式括号粘贴 `finishPaste`（`paste.go:125-128`）走同一个表项；多行草稿 `/end`（`commands.go:297`）直接调 `postUserText`。要成员回复，用户得再敲 `/ask <id>` 或 `/round <ids> --go`。Griffin 真机试完简洁输出之后的原话见 #47：「每次都要 /ask 这样，能否默认就是对话状态」。
2. **`/round` 的既有设计明令「绝不默认全员」。** `round.go:50-53`（头注释）与 `:167-169`（`拒绝: 名单为空。……不会默认全员。`）。理由是「一次敲击起 N 个进程、付 N 次可能的入场费」。#47 推翻这一条，**但只对裸文本**：`/round` 名单为空照旧拒绝（本刀不动 `cmdRound` 的代码）。
3. **`/round` 不带 id 的那一支不能直接拿来当对话模式。** `round.go:155-166`：不给 id 时取「当前会话最后一条用户发言里的 mention 与活跃成员的交集」，交集为空打 `:168` 的拒绝。对话模式要的是「没点名就问全员」「点了名但交集为空谁都不问、打一句说明」「点了总结成员不整条拒绝」（`round.go:183-189` 对名单里出现总结成员整条拒绝）。所以名单必须在 REPL 里按这条消息自己算，永远以显式 id 调 `/round`。
4. **总结成员第一次总结之后就是活跃成员。** `summary.go:103-115`：`runSummary` 在总结成员不在表里时 `Join`。于是从第二句话起，「全部活跃成员」里就有它；不剔掉的话 `round.go:183-189` 会把整句拒绝（「拒绝: <id> 是总结成员……」）。这是真机第二句话就会撞上的路径，原型用 §4 M10 实测过：不剔时第二句整句被拒。
5. **`postUserText` 吞掉 Post 失败。** `commands.go:337-339`：`Post` 失败时 `return r.reportErr("发言失败", err)`，而 `reportErr`（`repl.go:284-286`）打印成功时返回 nil。调用方只看 error 分不出消息有没有落库。自动一轮必须只在落库成功之后才起：成员回复的必须是用户刚说的那句话。
6. **CO1 的「点名了 X（用 /ask X 让他说话）」在自动一轮之前是假话。** `commands.go:374-381`：详细档下，点到的名字在成员表里（含已退出的）就打这一行。自动回复时那一轮马上就问，「用 /ask 让他说话」不成立。`/say`、`/to`、`/ask <id> 文本` 之后没有自动一轮，这句仍然对。
7. **`/say` 的帮助说「直接敲文本也一样」。** `commands.go:122`。对话模式下直接打字会让成员回复、`/say` 不会，这句变成假话（「同类的还有没有别处」查到的第二处；第一处是第 6 条）。
8. **`/add` 的候选里列出了总结成员。** `commands.go:864-869`：候选 = 配置里全部 id 去掉在场成员。#45 真机原件（00_DECISIONS :2153）第 4 行就是 `fable-5.1-summary`。把它当普通成员加进来，下一条 `/round` 或下一句话就会让它进名单（第 4 条）。显式 `/add <总结成员 id>` 在 `resolveAddSegment`（`:979`）第 1 步「配置全名精确命中」就放行了。**同类的别处**：`summary.go:95` 在总结成员已退出时打「要恢复总结用 /join <id>」——`/join` 与 `/add` 是同一个处理函数，拦显式 `/add` 时不能把这条恢复路一起拦掉。
9. **`/help` 的 `/session` 那一行没跟上 U43A。** `commands.go:153`「/session rebuild <agent-id> 丢弃并重建」。U43A 落地后 `/session rebuild` 先结清这个成员留在库里的陈旧在飞记录、再丢会话（18b；§7.2 第 1 条记「以后单独一刀」）。
10. **代价照实写**：对话模式下**每发一句话就是 N 次调用**（N = 名单人数；每个成员第一次开口或间隔超过 1 小时是冷启动，claude 量级每人一次约 0.4-1.0 美元的入场费），**N ≥ 2 且有人回复时再多一次总结调用**（总结成员首次开口同样冷启动），**没有预览**（`/round` 不带 `--go` 的预算表在这条路上不出现）。成员很多时，每句话都按全员付费；只想让一个人说话要写半角 `@` 紧跟他的 id（例如 `@grok-4.7`，不带尖括号，不是显示名；`@` 前面空一格或在行首），只想记下不让人回复要用 `/say`，整段关掉用 `/auto off` 或 `-no-auto`。「成员超过 K 个先确认」不加（主代理裁定：Griffin 要的是直接对话），代价写在这里与 README（§6 第 4 步）。取消照 `/round` 的既有行为（`round.go:60-66`）：行式模式下第一次 Ctrl-C 腰斩当前轮并退出 council；TUI 下 Ctrl-C 只取消这一次提交（`tui/model.go:337-348`）。
11. **（v1.1，W1 F0）想点名却没认出来，v1 会问全员。** `ParseMentions`（`mention.go:62-94`）只认半角 `@` 后面紧跟 `[A-Za-z0-9_.-]` 的一段、且整段合 id 正则。下面三种写法各在不同一步失败，都解析出零个 mention：`@甲`（`@` 后是非 ASCII 字节，显示名写法；`smx_a` 的显示名就是「甲」，屏幕上成员头印的也是显示名）、`@ smx_b`（`@` 后是空格）、`＠smx_b`（全角 `＠` 是 `EF BC A0` 三个字节，里面没有 `@` 字节）。v1 的判据「解析出零个 mention 就问全员」会把这几句当成没点名：用户明确只想问一个人，结果按全员付费、成员多时再加一次总结，而且因为没有 token，连「不是当前会话的成员」那一行都不打，失败是静默的。AT1（TUI 的 `@` 补全）只能缓解 TUI 下经补全插入的 id；行式模式、不走补全直接敲、中文输入法下的全角 `＠` 仍然暴露。v1.1 改为：这种「想点名却一个都没认出来」的 `@` 让这句**谁都不问**，打一行说明（§3.2 第 3 句）；判据见 §1.1 E 的 `hasStrayAt`。**v1.3 收窄，v1.5 改成按成员判（主代理裁决 `ch1_rulings_v15.md`；Griffin 23:00「如果不 @ 成员，默认是所有加入的都回复，然后有总结的」）**：v1.3、v1.4 靠 `@` 后面紧跟的那一个字节猜它是不是想点名，两个方向都漏（照抄提示的 `@<smx_b>` 问全员；`A @ B`、`$@` 谁都不问）。v1.5 只问「这个 `@` 是不是冲着一个认识的成员去的」：半角 `@` 没被 ParseMentions 认成 mention 时，跳过它后面紧跟的空格、tab、标点这类 ASCII 字节（遇到换行就停、不算），剩下的开头恰好是一个成员 id（配置里的 agent 或当前会话的成员；折小写、剥尾点），或以配置里的一个显示名开头，才算没认出、谁都不问。v1.6（W1d 修订）：中文输入法下的分隔符也跳，全角括号、引号、冒号、逗号、全角空格、NBSP、零宽字符、全角 `＜` 这类非 ASCII 的空白、标点、符号（v1.5 只跳 ASCII，`@（smx_b）` 反而问全员，是相对 v1.4 的回退）；显示名在跳过途中的每个位置都比。v1.7（主代理裁决 `ch1_rulings_v17.md`）：显示名不分大小写，`@ 坏 x` 冲着显示名「坏 X」，与 id 那一支折小写一致（C3 `display_case`）。所以 `@甲`、`@ smx_b`、`@<smx_b>`、`@(smx_b)`、`@**smx_b**`、`@-smx_b`、`@（smx_b）`、`@`（全角空格）`smx_b` 都谁都不问（C3 `stray_display`、`stray_space`、`angle_template`、`paren_id`、`bold_id`、`stray_bad_id`、`fw_paren_id`、`ideo_space_id`）；不冲着成员的 `@` 回到问全员：numpy / torch 的矩阵乘 `A @ B`、`echo $@`、here-string 收尾的 `'@`、`价格 @ 5 元`、句末单独一个 `@`、`@所有人`、PowerShell `@{`、SQL `@@IDENTITY`（C3 `numpy_matmul`、`bash_dollar_at`、`ps_herestring_close`、`price_at`、`stray_eot`、`at_everyone_cn`、`ps_hashtable`、`sql_identity`，原型 `ch1_v15` 实测）。`@所有人` 问全员只是因为它不冲着任何成员，本刀**不许**加「`@所有人` / `@all` 就是问全体」的语义。Python 装饰器 `@staticmethod`、PowerShell splatting `@params` 解析得出 token，是「点的不是成员」，谁都不问，第 1 句给 `/round` 出路（裁决书第 4 条：不改，§7.3 第 8 条）。全角 `＠` 照旧只看左边界：它永远构不成 mention，后面是什么都算没认出（C3 `stray_fullwidth`、`fullwidth_everyone`）。剩下的代价登记在 §7.3 第 5、7、9 条。**同类的别处**（照 V0 列全）：(a) `@Override`、`@param` 这类代码注解解析得出合法 token，是「点的不是成员」，走 `msgAutoNoneMentioned`、谁都不问，而且 postUser 先打「override 不是当前会话的成员」，这个方向看得见、不花钱，判据不改（v1.2：实测清单登记在 §7.3 第 6 条；v1.3 主代理定：判据不改，第 1 句末尾加与第 3 句逐字相同的 `/round` 出路）。(b) `@` 或 `＠` 左边紧贴 ASCII 字母、数字或 `_ @ . -` 时，按 ParseMentions 的左边界这个 `@` 不能起头，`hasStrayAt` 也不看它：`a@b.com` 这类邮箱正是为此设计的，照问全员（C3 `email`、`email_fullwidth` 钉着）；代价是 `hi@smx_b`、`看一下.@smx_b`、`看一下-@smx_b`、`看一下_@smx_b`、`hi＠smx_b`、`第2＠smx_b` 这类紧贴写法既不算点名、也不算没认出的点名，照问全员（原型实测；C3 `glued_at`、`glued_fullwidth` 钉着其中两种；`glued_left_cjk_tail` 的 `发到 x@甲 就行` 钉「左边界挡住的 `@` 后面即使取不出 token 也不算没认出」），是 v1.5 仍留着的花钱方向残余（§7.3 第 4 条；判据只改了 `@` 右边，左边界没动）。中文、空白、换行、中文标点紧贴 `@` 都不挡（`请@smx_b看一下`、`（@smx_b）看一下` 原型实测都只问 smx_b）。

---

## 1. 范围

### 1.1 做

凡写「名字实现方定」处实现方可自取（**本刀没有这种地方**）；其余名字、签名、字面量、顺序、判据、注释全文**全部冻结，逐字照抄**。生产代码新标识符不许以 `chx` 开头；也不许以 `mention` / `Mention` 开头（AT1 契约 `E:\multi_model_cowork\at1_contract.md` 在 `internal/repl` 新增 `MentionCandidate`、`MentionCandidates`、`MentionBoundaryOK`、`MentionTokenRune`、`mentionName`，本刀的名字与它们不相交）。

**冻结的名字**（测试腿按这些名字写用例）：REPL 字段 `autoReply bool`；`(*REPL).SetAutoReply(on bool)`；`cmdArgs` 字段 `bare bool`；包级变量 `roundCmd`；函数 `sayText`、`postUser`、`autoReplyRound`、`cmdAuto`、`hasStrayAt`（v1.1；v1.5 签名改成 `hasStrayAt(text string, known map[string]bool, names []string) bool`）、`asciiAlnum`（v1.5）、`atSkip`（v1.6，签名 `atSkip(s string) int`）；常量 `msgAutoUsage`、`msgAutoOn`、`msgAutoOnNoSummary`（v1.1）、`msgAutoOff`、`msgAutoNoneMentioned`、`msgAutoNoMembers`、`msgAutoStrayAt`（v1.1）、`msgAutoStrayAtPartly`（v1.1）；`app.Options.NoAuto`；启动参数 `-no-auto`；会话命令 `/auto`。

**格式说明**：下面每一处改动写成一对代码块，`anchor <文件>` 是 HEAD `3d134ee` 真文件里**恰好出现一次**的原文（含行首 tab 与全角标点，逐字复制），紧跟的 `replace` 是替换后的完整文本；`file <文件>` 是新文件全文。代码块里的缩进是 tab。实现腿交付时照 §5 的 `===PATCH` 格式，锚点以 HEAD 真文件为准。

#### A. `internal/repl/commands.go`

**A1. `cmdArgs` 加 `bare`**（HEAD :39-44）

```anchor internal/repl/commands.go
// fields 只按空白切，alpha,grok-4.7 会被当成一整段。
type cmdArgs struct {
	fields []string
	rest   string
}
```

```replace
// fields 只按空白切，alpha,grok-4.7 会被当成一整段。
// bare 为 true 表示这段话是裸文本（不以 / 开头的一行、TUI 的一次提交、行式括号粘贴），
// 只有 dispatch、Submit、finishPaste 交给 sayCmd 时置上；/say <文本> 走斜杠命令那条路，永远是 false。
// 只有 cmdSay 读它（#47 对话模式）。
type cmdArgs struct {
	fields []string
	rest   string
	bare   bool
}
```

**A2. `/say` 的帮助**（HEAD :122；§0 第 7 条）

```anchor internal/repl/commands.go
	summary:   "发言: /say <文本>；直接敲文本也一样；/say 不带参数进多行输入；/say /foo 发字面量 /foo",
```

```replace
	summary:   "只记下、不让任何人回复: /say <文本>；/say /foo 发字面量 /foo；/say 不带参数进多行输入（/end 发送时与直接打字相同）",
```

**A3. 包级变量 `roundCmd`**（插在 `sayCmd` 之后、`newCommandTable` 注释之前；HEAD :125-128）

```anchor internal/repl/commands.go
	run:       cmdSay,
}

// newCommandTable 构造命令表。
```

```replace
	run:       cmdSay,
}

// roundCmd 是 /round 的表项。照 sayCmd 的先例单独命名，是因为对话模式（#47）的自动一轮复用它：
// 经 runCommand 走同一份前置条件（needsRunner 在这里拦住 rn 为 nil），再进 cmdRound 这一个处理函数，
// 屏幕与手敲同样名单的 /round <ids> --go 是同一份输出。
var roundCmd = command{
	name:        "round",
	summary:     "按名单问一轮，--go 时发言者同时起进程；多个成员跑完自动总结（只给你看）: /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]",
	needsSvc:    true,
	needsConv:   true,
	needsRunner: true,
	run:         cmdRound,
}

// newCommandTable 构造命令表。
```

（`roundCmd` 的 summary 与 HEAD :152 那一行逐字相同，`/help` 里 `/round` 那一行不变。）

**A4. 命令表里 round 换成 `roundCmd`，session 改帮助**（HEAD :152-153；§0 第 9 条）

```anchor internal/repl/commands.go
		{name: "round", summary: "按名单问一轮，--go 时发言者同时起进程；多个成员跑完自动总结（只给你看）: /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]", needsSvc: true, needsConv: true, needsRunner: true, run: cmdRound},
		{name: "session", summary: "列出当前会话每个成员的 provider 会话；/session rebuild <agent-id> 丢弃并重建", needsSvc: true, needsConv: true, needsRunner: true, run: cmdSession},
```

```replace
		roundCmd,
		{name: "session", summary: "列出当前会话每个成员的 provider 会话；/session rebuild <agent-id> 先结清这个成员留在库里的陈旧在飞记录，再丢弃并重建", needsSvc: true, needsConv: true, needsRunner: true, run: cmdSession},
```

（`/session` 的新说法只写「先结清……再丢弃并重建」，不写「那一轮记为丢失」：18b 里 begun 行只删行、记的是「没起过进程、没花钱」，说成丢失对它不准。）

**A5. 登记 `/auto`**（HEAD :156，追加在 verbose 之后）

```anchor internal/repl/commands.go
		{name: "verbose", summary: "切换输出档位: /verbose 在简洁与详细之间切换；/verbose on 详细；/verbose off 简洁", run: cmdVerbose},
```

```replace
		{name: "verbose", summary: "切换输出档位: /verbose 在简洁与详细之间切换；/verbose on 详细；/verbose off 简洁", run: cmdVerbose},
		{name: "auto", summary: "切换直接打字要不要让成员回复: /auto 切换；/auto on 自动回复（点名了谁就只问谁，没点名问全部成员）；/auto off 只记下", run: cmdAuto},
```

（`/auto` 无前置条件，照 `/verbose`。）

**A6. `dispatch` 的裸文本分支置 `bare`**（HEAD :227）

```anchor internal/repl/commands.go
		return r.runCommand(ctx, sayCmd, cmdArgs{fields: strings.Fields(text), rest: text})
```

```replace
		// bare 置上：这是裸文本，自动回复打开时落库之后问一轮（#47）。
		return r.runCommand(ctx, sayCmd, cmdArgs{fields: strings.Fields(text), rest: text, bare: true})
```

**A7. 多行草稿 `/end` 走 `sayText`，算裸文本**（HEAD :297）

```anchor internal/repl/commands.go
		return postUserText(ctx, r, text)

	case draftCancelToken:
```

```replace
		// 草稿发送算裸文本（#47）：自动回复打开时落库之后问一轮。
		return sayText(ctx, r, text, true)

	case draftCancelToken:
```

**A8. `postUserText` 变成薄包装，新增 `sayText`、本体改名 `postUser`**（HEAD :316-330，注释与签名）

```anchor internal/repl/commands.go
// postUserText 以保留 sender "user" 的身份把 text 追加成一条消息。
//
// 五条路共用它：/say 带参数、裸文本、多行草稿发送、/ask 带文本、/to。
// 共用的理由和 sayCmd 一样 —— 发出去的消息形状必须一模一样，分多份写
// 早晚会走失同步。recipients 为空（含未传）即广播，四条既有发言路径都
// 走这一支，形状不许分叉；/to 是 Recipients 列的第一个生产写入方，把
// 非空列表从这里传进去，而不是另写一份 Post。
//
// recipients 用变长参数，是为了让既有的 postUserText(ctx, r, text)
// 调用继续表示广播，不必改那四处调用点；/to 把解析好的 id 列表展开传入。
//
// 三个脑子字段一律不填：人不是模型，没有脑子可记。
// 简洁档下成功落库不打「已记下 seq N」、不打「点名了」；两条警告（不是成员、定向收窄）照打。
func postUserText(ctx context.Context, r *REPL, text string, recipients ...string) error {
	posted, err := r.svc.Post(ctx, conversation.Message{
```

```replace
// postUserText 以保留 sender "user" 的身份把 text 追加成一条消息。
//
// 五条路共用一个本体 postUser：/say 带参数、裸文本、多行草稿发送经 sayText，
// /ask 带文本、/to 经本函数。共用的理由和 sayCmd 一样 —— 发出去的消息形状
// 必须一模一样，分多份写早晚会走失同步。recipients 为空（含未传）即广播，
// 四条既有发言路径都走这一支，形状不许分叉；/to 是 Recipients 列的第一个
// 生产写入方，把非空列表从这里传进去，而不是另写一份 Post。
//
// recipients 用变长参数，是为了让 postUserText(ctx, r, text) 继续表示广播；
// /to 把解析好的 id 列表展开传入。
//
// 三个脑子字段一律不填：人不是模型，没有脑子可记。
// 简洁档下成功落库不打「已记下 seq N」、不打「点名了」；两条警告（不是成员、定向收窄）照打。
//
// 本函数是 postUser 的薄包装（auto 恒为 false）：/ask 带文本与 /to 的输出与 #47 之前逐字节相同。
func postUserText(ctx context.Context, r *REPL, text string, recipients ...string) error {
	_, err := postUser(ctx, r, text, false, recipients)
	return err
}

// sayText 是 cmdSay 与多行草稿发送共用的落库入口（#47 对话模式）。
// bare 为 true 表示这段话是裸文本；它与 r.autoReply 同时为真才是「自动」：
// 落库成功之后紧接着经 autoReplyRound 问一轮。Post 失败（posted 为 false）时一个进程都不起：
// 成员回复的必须是用户刚说的那句话，而那句话没进库。
// 其余情形（/say <文本>、自动回复关着）与 postUserText 逐字节相同。
func sayText(ctx context.Context, r *REPL, text string, bare bool) error {
	auto := bare && r.autoReply
	posted, err := postUser(ctx, r, text, auto, nil)
	if err != nil || !posted || !auto {
		return err
	}
	return autoReplyRound(ctx, r, text)
}

// postUser 是 postUserText 的本体。返回的 bool 表示这条消息落了库：
// Post 失败时打「发言失败: ...」、返回 false 与 nil（reportErr 打印成功时返回 nil，
// 所以只看 error 分不出有没有落库）。error 非 nil 只表示写屏幕失败。
//
// auto 为 true 表示随后要自动问一轮：这时不打「点名了 X（用 /ask X 让他说话）」——
// 那一轮马上就问，「用 /ask 让他说话」是假话。「不是当前会话的成员」照打。
func postUser(ctx context.Context, r *REPL, text string, auto bool, recipients []string) (bool, error) {
	posted, err := r.svc.Post(ctx, conversation.Message{
```

**A9. `postUser` 的返回值**（HEAD :337-344，Post 失败与「已记下」）

```anchor internal/repl/commands.go
	if err != nil {
		return r.reportErr("发言失败", err)
	}
	if !r.concise {
		if err := r.printf("已记下 seq %d\n", posted.Seq); err != nil {
			return err
		}
	}
```

```replace
	if err != nil {
		return false, r.reportErr("发言失败", err)
	}
	if !r.concise {
		if err := r.printf("已记下 seq %d\n", posted.Seq); err != nil {
			return true, err
		}
	}
```

**A10. 定向收窄那一行**（HEAD :349-351）

```anchor internal/repl/commands.go
		if err := r.printf("只有 %s 会看到这条；其余成员永远不会 —— 游标只前进\n", strings.Join(recipients, ", ")); err != nil {
			return err
		}
```

```replace
		if err := r.printf("只有 %s 会看到这条；其余成员永远不会 —— 游标只前进\n", strings.Join(recipients, ", ")); err != nil {
			return true, err
		}
```

**A11. 点名提示段**（HEAD :362-388；§0 第 6 条）

```anchor internal/repl/commands.go
	tokens := ParseMentions(text)
	if len(tokens) == 0 {
		return nil
	}
	ps, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return nil
	}
	known := make(map[string]struct{}, len(ps))
	for _, p := range ps {
		known[p.AgentID] = struct{}{}
	}
	for _, tok := range tokens {
		if _, ok := known[tok]; ok {
			if !r.concise {
				if err := r.printf("点名了 %s（用 /ask %s 让他说话）\n", tok, tok); err != nil {
					return err
				}
			}
			continue
		}
		if err := r.printf("%s 不是当前会话的成员\n", tok); err != nil {
			return err
		}
	}
	return nil
}
```

```replace
	tokens := ParseMentions(text)
	if len(tokens) == 0 {
		return true, nil
	}
	ps, err := r.svc.Participants(ctx, r.cur.ID, true)
	if err != nil {
		return true, nil
	}
	known := make(map[string]struct{}, len(ps))
	for _, p := range ps {
		known[p.AgentID] = struct{}{}
	}
	for _, tok := range tokens {
		if _, ok := known[tok]; ok {
			if !r.concise && !auto {
				if err := r.printf("点名了 %s（用 /ask %s 让他说话）\n", tok, tok); err != nil {
					return true, err
				}
			}
			continue
		}
		if err := r.printf("%s 不是当前会话的成员\n", tok); err != nil {
			return true, err
		}
	}
	return true, nil
}
```

**A12. `/add` 的候选藏掉总结成员**（HEAD :861-869；§0 第 8 条）

```anchor internal/repl/commands.go
	// 一个都回显全名，用户当场看得见加的是谁。
	var candidates []string
	for _, id := range sortedKeys(r.cfg.Agents) {
		if !active[id] {
			candidates = append(candidates, id)
		}
	}
```

```replace
	// 一个都回显全名，用户当场看得见加的是谁。
	// 总结成员（[summary] agent）不进候选（#47）：它不当普通成员加，多个成员回复之后由
	// runSummary 自己 Join。候选只管列表与编号；显式点它的名字由 resolveAddSegment 拦。
	sumID := r.cfg.Summary.Agent
	var candidates []string
	for _, id := range sortedKeys(r.cfg.Agents) {
		if !active[id] && id != sumID {
			candidates = append(candidates, id)
		}
	}
```

**A13. 显式 `/add <总结成员>` 拒绝，已退出的放行**（HEAD :979-980）

```anchor internal/repl/commands.go
func resolveAddSegment(r *REPL, seg string, candidates []string, active, member map[string]bool) (id, problem string) {
```

```replace
func resolveAddSegment(r *REPL, seg string, candidates []string, active, member map[string]bool) (id, problem string) {
	// 0. 总结成员（#47）排在全名之前拦：它在配置里，不拦的话第 1 步就放行了。
	// 唯一放行的是「它是这个会话里已退出的成员」：那是 runSummary 那句「要恢复总结用 /join <id>」
	// 指给用户的路，Join 把那一行重新激活。没加入过、或正在场，一律拒绝。
	if sumID := r.cfg.Summary.Agent; sumID != "" && seg == sumID && !(member[seg] && !active[seg]) {
		return "", fmt.Sprintf("%s 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结\n", seg)
	}
```

（锚点是函数签名这一整行，文件里唯一；插入块紧跟其后，原来的「// 1. 全名精确命中排在最前，……」注释与第 1 步原样留在插入块后面。拒绝走 `cmdAdd` 既有的「全或无」：同一条 `/add` 里任何一段是总结成员，一个都不加，逐段打问题行。）

**A14. `cmdSay` 最后一行**（HEAD :1135）

```anchor internal/repl/commands.go
		return r.printf("%s\n", msgDraftEnter)
	}
	return postUserText(ctx, r, text)
}
```

```replace
		return r.printf("%s\n", msgDraftEnter)
	}
	// args.bare 分开两条路：裸文本在自动回复打开时落库之后问一轮，/say <文本> 永远只记下（#47）。
	return sayText(ctx, r, text, args.bare)
}
```

#### B. `internal/repl/seam.go`：`Submit` 的裸文本分支（HEAD :73）

```anchor internal/repl/seam.go
		return r.runCommand(ctx, sayCmd, cmdArgs{fields: strings.Fields(text), rest: text})
```

```replace
		// bare 置上：这是裸文本，自动回复打开时落库之后问一轮（#47）。
		return r.runCommand(ctx, sayCmd, cmdArgs{fields: strings.Fields(text), rest: text, bare: true})
```

（TUI 的多行提交先过 `tui/model.go:478` 的 `repl.HasLaterCommandLine`，过了才进 `Submit`，本刀不改那一道。首行只写 `/say` 的多行提交走斜杠命令那条路，`bare` 为 false，照 #47「/say 保持只记下」不问任何人。）

#### C. `internal/repl/paste.go`：`finishPaste` 末尾（HEAD :125-128）

```anchor internal/repl/paste.go
	return r.runCommand(ctx, sayCmd, cmdArgs{
		fields: strings.Fields(body),
		rest:   body,
	})
```

```replace
	// 粘贴进来的一段与敲进来的一行同样是裸文本（#47），bare 置上。
	return r.runCommand(ctx, sayCmd, cmdArgs{
		fields: strings.Fields(body),
		rest:   body,
		bare:   true,
	})
```

（「同类的还有没有别处」：#47 点名了三条裸文本路径，行式括号粘贴是第四条，同样经 `sayCmd`。主代理定它算裸文本：同一段话敲进去与粘进去行为不该不同。已在草稿里时粘贴并进草稿（`paste.go:121-124`），不走这里，由 `/end` 那一支负责。）

#### D. `internal/repl/repl.go`：REPL 结构体（HEAD :107-108）

```anchor internal/repl/repl.go
	// now 是总用时用的时钟。nil 时用 time.Now，测试注入。
	now func() time.Time
```

```replace
	// now 是总用时用的时钟。nil 时用 time.Now，测试注入。
	now func() time.Time
	// autoReply 为 true 时裸文本落库之后自动问一轮（#47 对话模式，见 autoreply.go）。
	// 零值 false 是只记下（今天的行为），所以直接 New 出来的 REPL（既有测试）行为不变；
	// app 装配时按 Options.NoAuto 设定（默认打开）。只经 SetAutoReply 与 /auto 改。
	autoReply bool
```

#### E. 新文件 `internal/repl/autoreply.go`（全文冻结，逐字）

```file internal/repl/autoreply.go
package repl

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	msgAutoUsage = "用法: /auto 在自动回复与只记下之间切换；/auto on 直接打字就让成员回复；/auto off 直接打字只记下"
	// msgAutoOn 与 msgAutoOnNoSummary 按配置里有没有 [summary] 选一句（v1.1）：
	// 没配总结成员时多个成员回复之后打的是 SM1 的「没有总结（没配总结成员…）」，说「自动总结」是假话。
	msgAutoOn          = "已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁），多个成员回复之后自动总结；/auto off 关掉"
	msgAutoOnNoSummary = "已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁）；没配总结成员，多个成员回复之后不做总结；/auto off 关掉"
	msgAutoOff         = "已关闭自动回复：直接打字只记下，不问任何人；要成员说话用 /ask 或 /round；/auto on 打开"
	// msgAutoNoneMentioned 是「这条消息点了名，但点到的没有一个是能回复的成员」时的一行。
	// 成因有三种（点的不是成员、点的已退出、点的是总结成员），一句话覆盖，不分因；
	// 点的不是成员时 postUser 已经先打了「<tok> 不是当前会话的成员」。
	// 贴进来的代码注解（@Override、@property）、npm i @types/node、@all 也解析得出 token、落到这一支，
	// 这些 @ 删不掉，所以末尾给与 msgAutoStrayAt 逐字相同的出路（v1.3）：消息已经记下了，/round 显式给 id 就让成员回复。
	// 用户可见的提示里不写尖括号占位符（v1.5）：照字面敲出来的尖括号不是合法的 id。
	msgAutoNoneMentioned = "点名的人都不是当前会话里能回复的成员，这条只记下了、没有问任何人；不点名就会问全部成员；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复"
	// msgAutoNoMembers 是「没点名，会话里也没有能回复的成员」时的一行：没有活跃成员，或只剩总结成员。
	msgAutoNoMembers = "当前会话还没有能回复的成员，这条只记下了；用 /add 加成员之后，直接打字就会有人回复"
	// msgAutoStrayAt 是「有 @ 冲着认识的成员去、却一个点名都没认出来」时的一行（v1.1；v1.5 按成员判）：
	// 谁都不问，不落到「没点名问全部成员」。全角 ＠ 不看后面是什么，不是点名的全角 ＠ 也会落到这一支，
	// 所以这一行同时给出路（v1.2）：消息已经记下了，/round 显式给 id 就让成员回复。
	// 不写尖括号占位符（v1.5）：照字面敲 @ 加尖括号加 id，按 hasStrayAt 仍是没认出的点名，用户会再看到这一行。
	msgAutoStrayAt = "有 @ 没认出点的是谁，这条只记下了、没有问任何人；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复；TUI 里敲 @ 会列出成员的 id 与显示名"
	// msgAutoStrayAtPartly 是「认出了一部分点名、还有冲着成员去的 @ 没认出来」时的一行（v1.1）：
	// 照问认出的那些（与「点名里夹着不在场的人照问其余」一致），这一行告诉用户有一处没算进去。
	msgAutoStrayAtPartly = "还有 @ 没认出点的是谁，没算进点名；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；TUI 里敲 @ 会列出成员的 id 与显示名"
)

// 对话模式（00_DECISIONS #47，2026-09-28，Griffin）。
//
// 裸文本（不以 / 开头的一行、TUI 的一次提交、行式括号粘贴、多行草稿 /end）落库之后，
// 自动回复打开时紧接着问一轮，等价于手敲 /round <ids> --go：一轮、不带裁判、多个成员回复时照 SM1 自动总结。
// /say <文本>、/to、/ask <id> 文本、/round 一律不变。
//
// SetAutoReply 设定对话模式：on 为 true 是自动回复，false 是只记下。
// 与 SetConcise 同一条纪律，只许在没有命令在跑时调用
// （app 在 Run / tui.Run 之前调；/auto 本身是一条命令，命令是串行派发的）。
func (r *REPL) SetAutoReply(on bool) {
	r.autoReply = on
}

// cmdAuto 切换或设定对话模式。写法照 cmdVerbose：不带参数是切换；on / off 是设定
// （只认小写、只认恰好一个参数）；别的一律打用法、不改。无前置条件。
// 打开时按配置里有没有 [summary] 选 msgAutoOn 或 msgAutoOnNoSummary（r.cfg 经 New 兜底恒非 nil）。
func cmdAuto(_ context.Context, r *REPL, args cmdArgs) error {
	switch {
	case len(args.fields) == 0:
		r.autoReply = !r.autoReply
	case len(args.fields) == 1 && args.fields[0] == "on":
		r.autoReply = true
	case len(args.fields) == 1 && args.fields[0] == "off":
		r.autoReply = false
	default:
		return r.printf("%s\n", msgAutoUsage)
	}
	if r.autoReply {
		if r.cfg.Summary.Agent == "" {
			return r.printf("%s\n", msgAutoOnNoSummary)
		}
		return r.printf("%s\n", msgAutoOn)
	}
	return r.printf("%s\n", msgAutoOff)
}

// autoReplyRound 在一条裸文本落库之后定名单，再经 roundCmd 问一轮。
//
// 名单只按这条消息算，永远传显式 id，不走 /round 不带 id 的那一支（lastUserMentionIDs）：
// 那一支对「点了名但交集为空」打 /round 自己的「名单为空」拒绝，对点了总结成员的整条拒绝，都不是这里要的。
//   - 候选 = 当前会话的活跃成员（Participants includeInactive=false）去掉总结成员（r.cfg.Summary.Agent）。
//     总结成员出现在 /round 名单里会让整条拒绝；它第一次总结之后已经是活跃成员，所以第二句话起一定要剔。
//   - 这条消息里有 mention（ParseMentions）：名单 = mention 与候选的交集，按 mention 出现的顺序；
//     交集为空时打 msgAutoNoneMentioned，谁都不问。
//   - 没有 mention：名单 = 全部候选；候选为空时打 msgAutoNoMembers，谁都不问。
//   - v1.1：有冲着认识的成员去、却没认出来的 @（hasStrayAt，v1.5 按成员判）时，没有 mention 就打
//     msgAutoStrayAt、谁都不问（想点名却没认出来，不能当成没点名去问全部成员）；有 mention 就先打
//     msgAutoStrayAtPartly，再照上面有 mention 的规则走。
//
// 名单的顺序不影响屏幕：PlanRound 按字典序排、去重，「本轮问 N 个」与手敲同样名单的 /round 相同。
// judge 模式的成员照 PlanRound 的既有规则进 Judges，不带 --with-judge，所以照打「裁判 X 本命令不问」。
//
// 走 r.runCommand(ctx, roundCmd, ...) 而不是直接调 cmdRound：rn 为 nil 时由 needsRunner 答
// msgNoRunner（这条消息已经记下了），不空指针。闸、A 之五在飞记录、指纹、BeginTurn、记账、
// 总结、总用时、观察者全在 cmdRound 那一条路上，这里一个都不另写。
//
// 代价：每条裸文本起 N 个进程，多个成员回复时再多一次总结调用，没有预览。取消照 /round 的既有行为。
func autoReplyRound(ctx context.Context, r *REPL, text string) error {
	parts, err := r.svc.Participants(ctx, r.cur.ID, false)
	if err != nil {
		return r.reportErr("round", err)
	}
	sumID := r.cfg.Summary.Agent
	// hasStrayAt 的匹配集合（v1.5）：配置里全部 agent 的 id，并上当前会话活跃成员的 id
	// （配置里有、会话里没加入的算；会话里有、配置里已经没有的也算，#45 的改名）。
	// 显示名取配置里的 display_name，TrimSpace 之后为空的不要。
	known := make(map[string]bool, len(r.cfg.Agents)+len(parts))
	names := make([]string, 0, len(r.cfg.Agents))
	for id, a := range r.cfg.Agents {
		known[id] = true
		if name := strings.TrimSpace(a.DisplayName); name != "" {
			names = append(names, name)
		}
	}
	byLower := make(map[string]string, len(parts))
	all := make([]string, 0, len(parts))
	for _, p := range parts {
		known[p.AgentID] = true
		if p.AgentID == sumID {
			continue
		}
		byLower[strings.ToLower(p.AgentID)] = p.AgentID
		all = append(all, p.AgentID)
	}
	ids := all
	stray := hasStrayAt(text, known, names)
	if mentions := ParseMentions(text); len(mentions) > 0 {
		if stray {
			if err := r.printf("%s\n", msgAutoStrayAtPartly); err != nil {
				return err
			}
		}
		ids = make([]string, 0, len(mentions))
		for _, m := range mentions {
			if canon, ok := byLower[m]; ok {
				ids = append(ids, canon)
			}
		}
		if len(ids) == 0 {
			return r.printf("%s\n", msgAutoNoneMentioned)
		}
	} else if stray {
		return r.printf("%s\n", msgAutoStrayAt)
	}
	if len(ids) == 0 {
		return r.printf("%s\n", msgAutoNoMembers)
	}
	fields := append(append([]string(nil), ids...), "--go")
	return r.runCommand(ctx, roundCmd, cmdArgs{fields: fields, rest: strings.Join(fields, " ")})
}

// hasStrayAt 报告 text 里有没有「冲着一个认识的成员去、却没被 ParseMentions 认成 mention」的 @
// （v1.1，只给 autoReplyRound 用；v1.5 按成员判）。要判的是这个 @ 是不是冲着成员去的：是，就不能把这句
// 当成没点名去问全员（花钱）；不是（numpy 的 A @ B、$@、here-string 收尾的 '@、价格 @ 5 元、@所有人、@{），
// 就是没点名，按 Griffin 的原话「如果不 @ 成员，默认是所有加入的都回复」问全员。
//
// 只看 ParseMentions 允许 mention 起头的位置：文本开头，或前一个字节不在 mentionLeftForbidden 里
// （a@b.com、a＠b.com 这类邮箱不看）。
// 全角 ＠（U+FF20）到这里就算，后面是什么都一样：ParseMentions 只认半角，它永远构不成 mention，
// 中文输入法下敲出来的 ＠ 多半是想点名。不许「为了一致」把全角也改成按成员判。
// 半角 @ 的下一个字节是 ASCII 字母或数字时，ParseMentions 已经把它认成 mention（mentionIDRe 只要求
// 首字符是字母数字，剥掉尾点之后结尾也一定合），点的不是成员时走 msgAutoNoneMentioned，这里跳过。
// 其余的半角 @ 按成员判：从下一个字符起，一个一个跳过 @ 与成员之间的分隔符（atSkip：空格、tab、
// 标点这类 ASCII 字节，以及全角括号、引号、冒号、全角空格、NBSP、零宽字符这类非 ASCII 的分隔符；
// v1.6 起非 ASCII 的也跳，中文输入法下 @（smx_b）与 @(smx_b) 是同一个意图）。遇到 CR / LF、字母、
// 汉字、数字就停；停在 CR / LF 上时下面两条都不会命中：行末的 @ 与下一行开头的词不算一回事。
// 两条命中一条就算：
//   - 跳过途中的每一个位置（含 @ 紧后面、还没跳的时候），剩下的文本以 names 里的一个显示名开头
//     （v1.7 起不分大小写，用 strings.EqualFold 比：@ 坏 x 也冲着显示名「坏 X」，与 id 那一支折小写一致；
//     先按显示名的字节长度切出一段，剩下的不够长就不比，切片不会越界；切口落在多字节字符中间时，
//     这一段的尾巴不是合法 UTF-8，EqualFold 按 U+FFFD 比，对不上；显示名自己含 U+FFFD 时才可能对上，
//     那也只往谁都不问那一侧偏）；每个位置都看，是因为显示名自己也可能以括号、引号开头；
//   - 跳过之后按 mentionTokenChar 取最长的一段，剥掉尾部的点、转小写，恰好是 known 里的一个 id。
//
// 一个 @ 不算，就接着看后面的 @：认成 mention 的与不冲着成员的都不定胜负
// （@smx_b @甲 算，价格 @ 5 元 @甲 也算）。
// known 与 names 由 autoReplyRound 在读成员表之后给：配置里全部 agent 的 id 并上当前会话活跃成员的 id，
// 配置里的显示名。@所有人 不冲着任何成员，所以问全员；这里不许加「@所有人 / @all 就是问全体」的语义。
// 只读 mention.go 的两个辅助，不改 ParseMentions；mention.go 的 tokenizer 改了，这里要跟着改。
func hasStrayAt(text string, known map[string]bool, names []string) bool {
	for i := 0; i < len(text); i++ {
		full := strings.HasPrefix(text[i:], string(rune(0xFF20)))
		if text[i] != '@' && !full {
			continue
		}
		if i > 0 && mentionLeftForbidden(text[i-1]) {
			continue
		}
		if full {
			return true
		}
		j := i + 1
		if j < len(text) && asciiAlnum(text[j]) {
			continue
		}
		for {
			for _, name := range names {
				if len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {
					return true
				}
			}
			n := atSkip(text[j:])
			if n == 0 {
				break
			}
			j += n
		}
		k := j
		for k < len(text) && mentionTokenChar(text[k]) {
			k++
		}
		if known[strings.ToLower(strings.TrimRight(text[j:k], "."))] {
			return true
		}
	}
	return false
}

// atSkip 报告 s 开头的那一个字符是不是 @ 与成员之间可以跳过的分隔符（v1.6，只给 hasStrayAt 用）：
// 是，返回它的字节数；不是，返回 0。ASCII 里除了字母、数字、CR、LF 都跳（空格、tab、
// < ( " 反引号 * - _ . @ 等）；非 ASCII 只跳空白（Zs）、标点（P）、符号（S）、组合记号（M）、
// 格式字符（Cf）。字母与汉字不跳，要留给显示名与 id 去比；U+2028、U+2029、U+0085 这几个换行
// 不在这几类里，与 CR / LF 一样停下。
func atSkip(s string) int {
	if s == "" || asciiAlnum(s[0]) || s[0] == '\n' || s[0] == '\r' {
		return 0
	}
	if s[0] < 0x80 {
		return 1
	}
	r, size := utf8.DecodeRuneInString(s)
	if unicode.In(r, unicode.Zs, unicode.P, unicode.S, unicode.M, unicode.Cf) {
		return size
	}
	return 0
}

// asciiAlnum 报告 c 是不是 ASCII 字母或数字（v1.5，只给 hasStrayAt 与 atSkip 用）。
func asciiAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
```

**冻结要点**（削弱任何一条都是 blocker）：
- (a) 只有裸文本且 `r.autoReply` 为真才自动；`/say <文本>`、`/to`、`/ask <id> 文本`、`/round` 行为与 HEAD 逐字节相同。
- (b) Post 失败（`posted == false`）或写屏幕失败（`err != nil`）时不起任何进程、不读成员表、不碰时钟。这一条说的是 `postUser` 返回的失败（落库之前与落库那一步）；自动一轮开始之后的写屏幕失败照原样并进返回值（C11 v1.1 各行、C12 `write_fail`）。
- (c) 名单永远显式；剔总结成员；「有 mention」判据是 `len(ParseMentions(text)) > 0`，与交集是否为空无关。v1.1：另有「没认出来的 @」判据 `hasStrayAt`，与 ParseMentions 同一条左边界（`mentionLeftForbidden`），半角 `@` 与全角 `＠`（U+FF20，代码里写 `string(rune(0xFF20))`）都看。v1.5 按成员判（旧判据整段删掉）：全角 `＠` 到左边界就算；半角 `@` 的下一个字节是 ASCII 字母或数字时 ParseMentions 已经认成 mention，不算；其余的半角 `@` 用 `atSkip` 一个字符一个字符地跳过分隔符（ASCII 里除了字母、数字、CR、LF 都跳；v1.6 起非 ASCII 的空白 Zs、标点 P、符号 S、组合记号 M、格式字符 Cf 也跳，字母、汉字、数字、U+2028 / U+2029 / U+0085 不跳），停在 CR / LF 上不算；跳过途中每一个位置（含还没跳的时候）剩下的文本以 `names` 里的显示名开头（v1.7 起不分大小写：先按显示名的字节长度切一段，剩下的够长才用 `strings.EqualFold` 比；v1.6 及以前逐字节比），或跳过之后按 `mentionTokenChar` 取最长一段、剥尾点、转小写恰好是 `known` 里的 id，才算（v1.6：显示名改成每个位置都比，v1.5 只在跳完之后比）。`known` 是配置里全部 agent 的 id 并上当前会话活跃成员的 id，`names` 是配置里 TrimSpace 之后非空的显示名。它只决定「谁都不问」与「多打一行」，**从不往名单里加人**。两个判据都在读成员表之后算。
- (d) 自动一轮经 `r.runCommand(ctx, roundCmd, …)` 进 `cmdRound`，参数恰好是 `ids` 加一个 `--go`：不带 `--with-judge`、不带 `--rounds`、不带 `--no-summary`。
- (e) 四条「不起进程」的分支（有没认出的 @ 而一个 mention 都没有、交集为空、没有可问的成员、rn 为 nil）各打一行冻结字面量，消息照常记下。先后冻结（原型实测）：没认出的 @ 先于「没有可问的成员」（C3 `stray_no_members`）；有 mention 时 `msgAutoStrayAtPartly` 打在 postUser 的「<tok> 不是当前会话的成员」之后、`msgAutoNoneMentioned` 或这一轮的屏幕之前（C3 `partly_none`、`partly`）。
- (f) 没有新增「自动问 N 个」之类的额外行：屏幕从 `cmdRound` 出，只有一份格式（§3.7）。**唯一的例外**（v1.1）：点名里认出了一部分、还有没认出的 @ 时，这一轮的屏幕之前多一行 `msgAutoStrayAtPartly`；这一行之后的屏幕仍然与手敲同样名单的 `/round <ids> --go` 逐字节相同。没有这种 @ 的消息，屏幕与手敲逐字节相同（C1）。

#### F. `internal/repl/round.go`：`cmdRound` 头注释加「第七次」（HEAD :58-60；代码一个字不动）

```anchor internal/repl/round.go
// 总结成员不许出现在名单里。--no-summary 跳过。
//
// Ctrl-C 的真实行为（2026-09-21 实测）：
```

```replace
// 总结成员不许出现在名单里。--no-summary 跳过。
//
// 第七次（#47，2026-09-28，Griffin）：对话模式推翻了上面「绝不默认全员」，但只对裸文本。
// 自动回复打开时，裸文本落库之后由 autoReplyRound（autoreply.go）定名单：没点名就是全部活跃成员
// （去掉总结成员），再经 roundCmd 进本函数，名单总是显式给出。本函数自己的规则一条不变：
// 手敲的 /round 名单为空照旧拒绝，不默认全员。
//
// Ctrl-C 的真实行为（2026-09-21 实测）：
```

#### G. `internal/app/app.go`

**G1. `Options` 加 `NoAuto`**（HEAD :50-54）

```anchor internal/app/app.go
	// 测试助手 runAppForTest 设为 true（它的既有用例都按详细档标定）。
	Verbose bool
}
```

```replace
	// 测试助手 runAppForTest 设为 true（它的既有用例都按详细档标定）。
	Verbose bool

	// NoAuto 为 true 时裸文本只记下，不自动让成员回复（#47 对话模式）。
	// 零值是自动回复，所以 app.Run 的默认是对话模式。由 cmd/council 的 -no-auto 设。
	// 测试助手 runAppForTest 设为 true（它的既有用例都按只记下标定）。
	NoAuto bool
}
```

（字段名取 `NoAuto` 而不是更长的名字，是为了 `cmd/council/main.go` 与 `runAppForTest` 两处多行字面量里 gofmt 不重排对齐：键长不超过 `ConfigPath`。原型实测 `NoAutoReply` 会让两处各重排 5-8 行。）

**G2. 两个分支都设对话模式**（HEAD :229-236）

```anchor internal/app/app.go
		r.SetConcise(!opts.Verbose)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
	} else {
		// 行式与 TUI 都在 Run 之前设档位。
		r := repl.New(cfg, svc, rn, in, out, rulesCtx)
		r.SetConcise(!opts.Verbose)
		err = r.Run(ctx)
```

```replace
		r.SetConcise(!opts.Verbose)
		r.SetAutoReply(!opts.NoAuto)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
	} else {
		// 行式与 TUI 都在 Run 之前设档位与对话模式。
		r := repl.New(cfg, svc, rn, in, out, rulesCtx)
		r.SetConcise(!opts.Verbose)
		r.SetAutoReply(!opts.NoAuto)
		err = r.Run(ctx)
```

#### H. `cmd/council/main.go`

**H1. 登记 `-no-auto`**（HEAD :44）

```anchor cmd/council/main.go
	noTUI := fs.Bool("no-tui", false, "不用 TUI，走行式 REPL（TUI 在你的终端上出问题时的退路）")
```

```replace
	noTUI := fs.Bool("no-tui", false, "不用 TUI，走行式 REPL（TUI 在你的终端上出问题时的退路）")
	noAuto := fs.Bool("no-auto", false, "直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）")
```

**H2. 传进 `app.Options`**（HEAD :105-106）

```anchor cmd/council/main.go
		Verbose:    *verbose,
	}); err != nil {
```

```replace
		Verbose:    *verbose,
		NoAuto:     *noAuto,
	}); err != nil {
```

（`NoAuto:` 后面是 5 个空格，与上面各行同一列；gofmt 不重排。）

#### I. 授权的既有测试改动（理由见 §1.2 的表；只有这六处）

**I1.** `internal/app/app_test.go`（HEAD :98-99，`runAppForTest`）

```anchor internal/app/app_test.go
		Verbose:    true,
	}
```

```replace
		Verbose:    true,
		NoAuto:     true,
	}
```

**I2.** `internal/app/cox_app_test.go`（HEAD :146）

```anchor internal/app/cox_app_test.go
	if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader("/new t\n/join c1\nhi\n/quit\n"), Out: &out0, Err: &err0}); err != nil {
```

```replace
	if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader("/new t\n/join c1\nhi\n/quit\n"), Out: &out0, Err: &err0, NoAuto: true}); err != nil {
```

**I3.** `internal/app/cox_app_test.go`（HEAD :176）

```anchor internal/app/cox_app_test.go
		runDone <- Run(ctx, Options{ConfigPath: cfgPath, TUI: true, InFile: inR, OutFile: outW, Err: errOut, Verbose: verbose})
```

```replace
		runDone <- Run(ctx, Options{ConfigPath: cfgPath, TUI: true, InFile: inR, OutFile: outW, Err: errOut, Verbose: verbose, NoAuto: true})
```

**I4.** `internal/app/cox_app_test.go`（HEAD :246）

```anchor internal/app/cox_app_test.go
		if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: verbose}); err != nil {
```

```replace
		if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: verbose, NoAuto: true}); err != nil {
```

**I5.** `internal/app/fux_review_test.go`（HEAD :25）

```anchor internal/app/fux_review_test.go
		if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: verbose}); err != nil {
```

```replace
		if err := coxRunApp(t, Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: verbose, NoAuto: true}); err != nil {
```

**I6.** `internal/app/smx_summary_app_test.go`（HEAD :125，`TestSmxAppSummaryEndToEnd` 的 `Run`；v1.1 F5）

```anchor internal/app/smx_summary_app_test.go
				done <- Run(context.Background(), Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: tc.verbose})
```

```replace
				done <- Run(context.Background(), Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: tc.verbose, NoAuto: true})
```

（I2-I6 的锚点都是整行，在各自文件里恰好一次。I2 行首 1 个 tab，I3-I5 行首 2 个 tab，I6 行首 4 个 tab。）

### 1.2 不做（两条腿都不许越界）

- 不改 `cmdRound`、`runOneRound`、`runSummary`、`PlanRound` 与 `internal/turn` 的任何代码：`/round` 名单为空照旧拒绝；SM1 的总结判据（按本条命令真问到的成员去重计数 ≥ 2、ctx、没配、没人回复、已退出）一条不改；名单里出现总结成员整条拒绝不改。
- 不加任何配置键（开关只是启动参数 `-no-auto`、`app.Options.NoAuto`、会话命令 `/auto`）。
- 不改 `/say`（除帮助文字）、`/to`、`/ask`、`/round`、`/team use` 的行为；不改 `HasLaterCommandLine` 与 TUI 的回显、看板、收尾行（收尾行照旧写 `/round 结束`，§7.1 第 3 条）。
- 不改 MD1 的识别表（`printDiscoveredModels` 及其以下）；只改 `/add` 的配置候选与显式解析。
- 不做「只记下不回」之外的新语义：不引入「自动问 N 个」提示行、不做预览、不做成本确认。
- 不改 `internal/tui` 的任何生产代码。
- **既有 `_test.go` 一律不改，下面六处（四个文件）除外**（逐处授权，原型实测不改就红或就失去原意）：

| 文件:位置 | 改前 | 改后 | 理由 |
|---|---|---|---|
| `internal/app/app_test.go:98`（`runAppForTest` 的 `Options` 字面量，`Verbose:    true,` 之后） | 无 | 加一行 `		NoAuto:     true,` | 同 CO1 的先例。用它的 smoke、s10a、s10c2、s10d、cxx、drift、persist、step10a4、x39f 等用例的脚本里有裸文本（例如 `cxx_codex_e2e_test.go:105` 的 `hello-cxx` 之后紧跟 `/ask sol`），都按「只记下」标定；不加这一行，裸文本会先自动问一轮，后面的 `/ask` 变成 no_delta 或多一轮记账。gofmt 不重排（键长与 `ConfigPath` 同列）。 |
| `internal/app/cox_app_test.go:146`（`coxRunTui` 第 0 段） | `…Out: &out0, Err: &err0});` | `…Out: &out0, Err: &err0, NoAuto: true});` | 原型实测不加时 `TestCoxAppDefaultConcise/tui` 与 `/tui_verbose` 红：第 0 段的 `hi` 自动问过 c1，第 1 段 `/round c1 --go` 变成「跳过 c1（no_delta）」「这一轮没有人可问。」，15 秒等不到收尾行。这条用例验的是 CO1 的档位，不是 CH1。 |
| `internal/app/cox_app_test.go:176`（`coxRunTui` 第 1 段 TUI 的 `Run`） | `…Err: errOut, Verbose: verbose})` | `…Err: errOut, Verbose: verbose, NoAuto: true})` | 同上一行：第 1 段只敲 `/use` 与 `/round c1 --go`，没有裸文本，加不加今天都不变；加上是为了这条 CO1 用例在两段上都明确处在「只记下」，与第 0 段一致，不依赖默认值。 |
| `internal/app/cox_app_test.go:246`（`TestCoxAppDefaultConcise` 的 `lineMode`） | `…Err: &errOut, Verbose: verbose}); err != nil {` | `…Err: &errOut, Verbose: verbose, NoAuto: true}); err != nil {` | 原型实测不加时 `default` 子测试红：脚本 `裸文本 @c1` 先自动问了 c1，期望尾部多出一段、`/round c1 --go` 变成 no_delta。 |
| `internal/app/fux_review_test.go:25` | `…Err: &errOut, Verbose: verbose}); err != nil {` | `…Err: &errOut, Verbose: verbose, NoAuto: true}); err != nil {` | 原型实测不加时 `TestFuxAppVerboseMentionHint/verbose` 红：它钉「详细档裸文本 @c1 打一次『点名了 c1（用 /ask c1 让他说话）』」，对话模式下这一行被 §1.1 A11 有意压掉。这条用例验的是 CO1 手动档的门，加上 `NoAuto: true` 保住原意；自动档那一侧由 §2 C5 钉。 |
| `internal/app/smx_summary_app_test.go:125`（`TestSmxAppSummaryEndToEnd` 的 `Run`；v1.1 F5） | `…Err: &errOut, Verbose: tc.verbose})` | `…Err: &errOut, Verbose: tc.verbose, NoAuto: true})` | 不改也绿，但绿得是巧合：脚本 `裸文本\n/round c1 c2 --go` 在默认档下，裸文本自动问了 c1、c2 并总结，屏幕前 6 行与原先 `/round` 那 6 行逐字相同，之后的 `/round` 两人都 no_delta、不写库，`/log` 的 seq 10 仍是总结那一行。它验的就成了对话模式那一轮，与用例头注释写的 `/round` 不符。加上 `NoAuto: true`，裸文本只记下，这条用例回到验 `/round` 的总结；原型实测加上之后两个子测试照样绿（行为与 HEAD 上逐字节相同）。 |

  （表里 `cox_app_test.go` 算一个文件三处，四个文件共六处。）`report_test.go` 的四处 `Options` 原型实测不受影响。repl、tui 包的既有用例都直接 `New`，零值只记下，不受影响。

---

## 2. 验收（测试腿新写的文件）

通用纪律：
- 新文件四个：`internal/repl/chx_auto_test.go`、`internal/tui/chx_program_test.go`、`internal/app/chx_app_test.go`、`cmd/council/chx_flag_test.go`。包级标识符（类型、函数、变量、常量）一律以 `chx` 开头（唯一例外是 C11 按 Go 的 `errXxx` 惯例冻结的哨兵名 `errChxWrite`，v1 起就是这个名字），测试函数名 `TestChx...`。**不新写 `TestMain`**：repl 包复用 `session_inflight_test.go` 的（假 claude CLI：argv 带 `--resume` 时睡 700ms 再应答，正文 `OK`），tui 包复用 `rba_fakecli_test.go` 的，app 包复用 `smoke_test.go` 的。
- 不起真 provider：外部命令一律是测试二进制自己（`COUNCIL_FAKE_CLI=1`、`Command` 写 `os.Args[0]`），或一个不存在的路径。store 一律 `t.TempDir()` 之下，不碰 `C:\Users\25142\.council`。
- 至少一条走真 `app.Run` + 真 store（§2.3，行式与 TUI 两个分支）；至少一条走真 `tea.Program`（§2.2；§2.3 A2 也经真 `tui.Run`）。
- 断言整串相等或整行相等；随机器变的只许经下面**冻结的规范化**抹掉：repl 包用既有的 `smxNormalize`（成员头 `（\d+\.\ds）` 换成 `（T）`、预算行余量换 `H`、`wall_ms=W`）；app 包成员头用既有的 `smxAppTail`（它把 `（\d+\.\ds）` 换成 `（T）`），整行 `^总用时 \d+\.\ds$` 换成 `总用时 T`；tui 包 `P` 行里 `（\d+\.\ds）` 换 `（T）`，整行 `^/round 结束  \d+s  回来 2/2$` 换成一个固定串。两个成员并发回来、先后不固定的地方，要么用 `slowA`（`smxSetup` 的慢 a：smx_a 先开口一次，下一轮带 `--resume` 睡 700ms，于是 smx_b 先回来，用 `smxRequireBBeforeA` 做前提守卫），要么把那两行排序后比（写明是哪两行）。
- 每条注释写【判别力】：它能不能为假、只差一处的对照是哪条、点名杀掉 §4 的哪几条。
- 允许只读引用同包既有测试辅助（不许改它们）：
  - repl：`smxSetup`、`smxRig` 及其字段（`ctx`、`svc`、`r`、`buf`、`clk`、`convID`）与方法（`submit`、`post`、`member`、`msgsFrom`、`requireNoMsgs`、`requireSummaryMsg`、`installHook`（v1.1））、`smxHookWriter` 及其方法（`String` 经嵌入的 `rbaBuf`；v1.1）、`requireFired`（v1.1）、`smxCfg`、`smxNormalize`、`smxLines`、`smxCount`、`smxEq`、`smxClockIs`、`smxRequireBBeforeA`、常量 `smxS1Want`、`smxSumHeadOK`（`smx_summary_round_test.go`）；`newRoundStreamService`（`step10ux1_round_stream_test.go:94`）；`rbaBuf`（`rba_golden_test.go:36`）。白盒可以读写 REPL 字段 `autoReply`、`in`、`out`；C14（v1.6）直接调 `hasStrayAt`。
  - tui：`rbaNewTuiRig`（字段 `r`、`lw`、`convID`；成员 `rba_a`、`rba_b`（discussion）、`rba_j`（judge），显示名都是 `"X"`，**没配总结成员**）、`rbbStartProg`、`(*rbbProg).submit` / `useConv` / `waitFor`、`rbbProg` 的字段 `spy`、`(*rbbSpy).entries`、`rbbEntry`（`kind`、`text`、`rs`）、`rbbCount`、`rbbLogDump`（`rbb_order_test.go`）。
  - app：`isolateAppEnv`、`tomlLiteral`、`writeTestConfig`（`app_test.go`）、`coxRunApp`、`coxLockedBuf`、`coxWaitFor`、`coxFlat`、`coxESC`（`cox_app_test.go`）、`smxAppTail`（`smx_summary_app_test.go`）。**不用 `runAppForTest`**（本刀落地后它固定 `NoAuto: true`）。
  - cmd/council：`run`、`exitOK`、`exitUsage`。
- 装置要点（冻结）：`smxSetup(t, "smx_s", members, concise, slowA)` 的 REPL 与 Runner 用同一份 `smxCfg("smx_s")`（总结成员 `smx_s`，显示名「总结 S」；`smx_a` 显示名「甲」，`smx_b` 显示名为空、回落 id，`smx_j` 是 judge），它在返回前 `/use` 过会话、清空了缓冲、发过 `seed-msg`（`slowA` 时另有 `seed-msg-2`），假时钟第 1 次返回基准、之后每次返回基准 +7250ms（所以一条命令的总用时恒为 `7.2s`，同一装置第二条命令的总用时是 `0.0s`）。打开自动回复 = 构造之后 `g.r.SetAutoReply(true)`；「手动」= 不调。
- HEAD 上的状态：repl、tui、app 三个新文件引用本刀新增的标识符（`SetAutoReply`、REPL 字段 `autoReply`、`app.Options.NoAuto`），在 HEAD 上整包编译不过，是预期的红；报告里写出每个文件引用了哪些新标识符。`cmd/council/chx_flag_test.go` 不引用任何新 Go 标识符，在 HEAD 上能编译，但 `-no-auto` 未登记，是运行期的红（原型把 `main.go` 换回 HEAD 原文实跑过：F1 红、既有 `TestCoxVerboseFlagRegistered` 绿）。

下面「输出」一律指 `g.buf.String()` 经 `smxNormalize` 之后（C6 行式那一条是整个 `Run` 的输出，含提示符）。字面量里的 `\n` 是换行。

### 2.1 `internal/repl/chx_auto_test.go`（端到端：真 SQLite、真 Runner、假 CLI）

**C1 `TestChxAutoEqualsManualRound`**（屏幕与手敲 `/round <ids> --go` 逐字节相同）。两个子测试，装置都是 `smxSetup(t, "smx_s", {smx_a, smx_b}, concise=true, slowA=true)`：
- `auto`：打开自动回复，`Submit("你好")`；前提守卫 `smxRequireBBeforeA`；输出 == `smxS1Want`（即 `"本轮问 2 个: smx_a, smx_b\nsmx_b（T）: OK\n甲（T）: OK\n本轮问 1 个: smx_s\n总的来看（总结 S 做总结）：OK\n总用时 7.2s\n"`）；假时钟恰好 2 次；`requireSummaryMsg`（smx_s 被 Join 且激活、恰好 1 条只给自己的总结）；库里 SenderID 为 `user` 的消息正文依次恰好是 `seed-msg`、`seed-msg-2`、`你好`。
- `manual`：不打开，`Submit("你好")`，缓冲整串 == `""`，假时钟 0 次，`requireNoMsgs(smx_s)`；再 `Submit("/round smx_a smx_b --go")`，前提守卫同上，输出 == `smxS1Want`；user 消息同上。
【判别力】`auto` 与 `manual` 第二步是同一个期望，这就是「与手敲同样名单的 `/round --go` 一致」的逐字节担保；`manual` 第一步是自动那一侧的缺席对照（同装置、只差开关）。杀 M2、M14、M26、M29。

**C2 `TestChxAutoSecondMessage`**：
- `second`：同 C1 `auto` 的装置与第一句（输出 == `smxS1Want`）；前提守卫：此后 `member("smx_s")` 存在且 `Active`；清空缓冲，`Submit("再说一句")`；输出按行切恰好 6 行，把第 2、3 行排序后，6 行依次等于 `本轮问 2 个: smx_a, smx_b`、`smx_b（T）: OK`、`甲（T）: OK`、`本轮问 1 个: smx_s`、`总的来看（总结 S 做总结）：OK`、`总用时 0.0s`（第二句里两人都带 `--resume`、都睡 700ms，先后不固定，所以排序）；库里 smx_s 的消息恰好 2 条。
- `summary_member_from_start`：成员 `{smx_a, smx_b, smx_s}`（smx_s 一开始就是活跃成员），`slowA=true`，打开自动回复，`Submit("你好")`，前提守卫同上，输出 == `smxS1Want`。
【判别力】§0 第 4 条那条真机第二句的路径；不剔总结成员（M10）时两个子测试都变成「拒绝: smx_s 是总结成员……」。对照是 C1 `auto`（总结成员尚未加入时剔不剔都一样，所以 C1 杀不了 M10）。

**C3 `TestChxAutoRoster`**（名单全集，表驱动；每行一个子测试；装置 `smxSetup(t, "smx_s", 成员, true, slowA)` + 打开自动回复；`leaveB` 的行在提交前 `svc.Leave(ctx, convID, "smx_b", now)`，失败即前提不成立）。每行断言：输出整串 == 期望；假时钟调用次数 == 表中值；库里最后一条 user 消息正文 == 输入（消息照常记下）；时钟为 0 的行另断言 smx_a、smx_b、smx_s、smx_j 都没有消息（`requireNoMsgs`）。下表 `ONE_A` = `"本轮问 1 个: smx_a\n甲（T）: OK\n总用时 7.2s\n"`，`ONE_B` = `"本轮问 1 个: smx_b\nsmx_b（T）: OK\n总用时 7.2s\n"`，`JUDGE` = `"裁判 smx_j 本命令不问，要用 --with-judge 才问。\n"`，`NONE` = §3.2 第 1 句加 `\n`，`EMPTY` = §3.2 第 2 句加 `\n`，`STRAY` = §3.2 第 3 句加 `\n`，`PARTLY` = §3.2 第 4 句加 `\n`（后两个 v1.1）。输入里的 `＠` 是全角 U+FF20（Go 源码里直接写这个字符，或拼 `string(rune(0xFF20))`，都行）。

| 子测试 | 成员 | slowA | leaveB | 输入 | 期望输出 | 时钟 |
|---|---|---|---|---|---|---|
| `subset` | smx_a, smx_b | 否 | 否 | `@smx_b 看一下` | `ONE_B` | 2 |
| `upper` | smx_a, smx_b | 否 | 否 | `@SMX_B 看一下` | `ONE_B` | 2 |
| `order` | smx_a, smx_b | 是 | 否 | `@smx_b @smx_a 看一下` | `smxS1Want` | 2 |
| `all` | smx_a, smx_b | 是 | 否 | `看一下` | `smxS1Want` | 2 |
| `not_member` | smx_a, smx_b | 否 | 否 | `@ghost 看一下` | `"ghost 不是当前会话的成员\n"` + `NONE` | 0 |
| `left` | smx_a, smx_b | 否 | 是 | `@smx_b 看一下` | `NONE` | 0 |
| `left_all` | smx_a, smx_b | 否 | 是 | `看一下` | `ONE_A` | 2 |
| `left_mixed` | smx_a, smx_b | 否 | 是 | `@smx_b @smx_a 看一下` | `ONE_A` | 2 |
| `summary_mentioned` | smx_a, smx_s | 否 | 否 | `@smx_s 看一下` | `NONE` | 0 |
| `mixed` | smx_a, smx_b | 否 | 否 | `@ghost @smx_a 看一下` | `"ghost 不是当前会话的成员\n"` + `ONE_A` | 2 |
| `no_members` | （空） | 否 | 否 | `看一下` | `EMPTY` | 0 |
| `only_summary` | smx_s | 否 | 否 | `看一下` | `EMPTY` | 0 |
| `only_summary_mentioned` | smx_s | 否 | 否 | `@smx_s 看一下` | `NONE` | 0 |
| `summary_plus_one` | smx_a, smx_s | 否 | 否 | `看一下` | `ONE_A` | 2 |
| `judge` | smx_a, smx_j | 否 | 否 | `看一下` | `JUDGE` + `ONE_A` | 2 |
| `judge_only` | smx_a, smx_j | 否 | 否 | `@smx_j 看一下` | `JUDGE` + `"这一轮没有人可问。\n"` | 1 |
| `summary_and_member`（v1.1） | smx_a, smx_s | 否 | 否 | `@smx_s @smx_a 看一下` | `ONE_A` | 2 |
| `stray_display`（v1.1） | smx_a, smx_b | 否 | 否 | `@甲 看一下` | `STRAY` | 0 |
| `stray_fullwidth`（v1.1） | smx_a, smx_b | 否 | 否 | `＠smx_b 看一下` | `STRAY` | 0 |
| `stray_space`（v1.1） | smx_a, smx_b | 否 | 否 | `@ smx_b 看一下` | `STRAY` | 0 |
| `stray_bad_id`（v1.1；v1.3 翻；v1.5 翻回） | smx_a, smx_b | 否 | 否 | `@-smx_b 看一下` | `STRAY` | 0 |
| `stray_no_members`（v1.1） | （空） | 否 | 否 | `@甲 看一下` | `STRAY` | 0 |
| `email`（v1.1） | smx_a, smx_b | 是 | 否 | `发到 a@b.com 就行` | `smxS1Want` | 2 |
| `email_fullwidth`（v1.1） | smx_a, smx_b | 是 | 否 | `发到 a＠b.com 就行` | `smxS1Want` | 2 |
| `trailing_dot`（v1.1） | smx_a, smx_b | 否 | 否 | `看一下 @smx_b.` | `ONE_B` | 2 |
| `glued_at`（v1.1） | smx_a, smx_b | 是 | 否 | `hi@smx_b 看一下` | `smxS1Want` | 2 |
| `partly`（v1.1） | smx_a, smx_b | 否 | 否 | `@甲 @smx_b 看一下` | `PARTLY` + `ONE_B` | 2 |
| `partly_none`（v1.1） | smx_a, smx_b | 否 | 否 | `@甲 @ghost 看一下` | `"ghost 不是当前会话的成员\n"` + `PARTLY` + `NONE` | 0 |
| `mid_space_display`（v1.2） | smx_a, smx_b | 否 | 否 | `看一下 @甲` | `STRAY` | 0 |
| `mid_space_fullwidth`（v1.2） | smx_a, smx_b | 否 | 否 | `看一下 ＠smx_b` | `STRAY` | 0 |
| `mid_cjk_display`（v1.2） | smx_a, smx_b | 否 | 否 | `请@甲看一下` | `STRAY` | 0 |
| `line2_display`（v1.2） | smx_a, smx_b | 否 | 否 | `第一行\n@甲 看一下` | `STRAY` | 0 |
| `line2_fullwidth`（v1.2） | smx_a, smx_b | 否 | 否 | `第一行\n＠smx_b 看一下` | `STRAY` | 0 |
| `line2_mention`（v1.2） | smx_a, smx_b | 否 | 否 | `第一行\n@smx_b 看一下` | `ONE_B` | 2 |
| `stray_after_mention`（v1.2） | smx_a, smx_b | 否 | 否 | `@smx_b @甲 看一下` | `PARTLY` + `ONE_B` | 2 |
| `glued_left_cjk_tail`（v1.2） | smx_a, smx_b | 是 | 否 | `发到 x@甲 就行` | `smxS1Want` | 2 |
| `ps_hashtable`（v1.2；v1.3 翻） | smx_a, smx_b | 是 | 否 | `$h = @{ Name = 'x' } 这行为什么报错` | `smxS1Want` | 2 |
| `sql_identity`（v1.2；v1.3 翻） | smx_a, smx_b | 是 | 否 | `SELECT @@IDENTITY 返回空` | `smxS1Want` | 2 |
| `at_everyone_cn`（v1.2；v1.5 翻） | smx_a, smx_b | 是 | 否 | `@所有人 看一下` | `smxS1Want` | 2 |
| `glued_fullwidth`（v1.2） | smx_a, smx_b | 是 | 否 | `hi＠smx_b 看一下` | `smxS1Want` | 2 |
| `double_at`（v1.3；v1.5 翻） | smx_a, smx_b | 否 | 否 | `@@smx_b 看一下` | `STRAY` | 0 |
| `backtick_at`（v1.3） | smx_a, smx_b | 是 | 否 | `` `@` 这个符号怎么打 `` | `smxS1Want` | 2 |
| `ps_array`（v1.3） | smx_a, smx_b | 是 | 否 | `$a = @() 然后 $a += 1` | `smxS1Want` | 2 |
| `stray_eot`（v1.3；v1.5 翻） | smx_a, smx_b | 是 | 否 | `你看 @` | `smxS1Want` | 2 |
| `stray_newline`（v1.3；v1.5 翻） | smx_a, smx_b | 是 | 否 | `第一行 @\n第二行` | `smxS1Want` | 2 |
| `stray_crlf`（v1.3；v1.5 翻） | smx_a, smx_b | 是 | 否 | `第一行 @\r\n第二行` | `smxS1Want` | 2 |
| `stray_tab`（v1.3） | smx_a, smx_b | 否 | 否 | `@\tsmx_b 看一下` | `STRAY` | 0 |
| `stray_vtab`（v1.3） | smx_a, smx_b | 否 | 否 | `@\vsmx_b 看一下` | `STRAY` | 0 |
| `stray_formfeed`（v1.3） | smx_a, smx_b | 否 | 否 | `@\fsmx_b 看一下` | `STRAY` | 0 |
| `price_at`（v1.3；v1.5 翻） | smx_a, smx_b | 是 | 否 | `价格 @ 5 元` | `smxS1Want` | 2 |
| `angle_template`（v1.4；v1.5 翻） | smx_a, smx_b | 否 | 否 | `@<smx_b> 看一下` | `STRAY` | 0 |
| `partly_ascii`（v1.4；v1.5 翻） | smx_a, smx_b | 否 | 否 | `@smx_b @(smx_a) 看一下` | `PARTLY` + `ONE_B` | 2 |
| `numpy_matmul`（v1.5） | smx_a, smx_b | 是 | 否 | `y = A @ x + b` | `smxS1Want` | 2 |
| `bash_dollar_at`（v1.5） | smx_a, smx_b | 是 | 否 | `echo $@` | `smxS1Want` | 2 |
| `ps_herestring_close`（v1.5） | smx_a, smx_b | 是 | 否 | `$s = @'\nhello\n'@` | `smxS1Want` | 2 |
| `newline_cut`（v1.5） | smx_a, smx_b | 是 | 否 | `@\nsmx_b 看一下` | `smxS1Want` | 2 |
| `cr_cut`（v1.5） | smx_a, smx_b | 是 | 否 | `@\rsmx_b 看一下` | `smxS1Want` | 2 |
| `paren_id`（v1.5） | smx_a, smx_b | 否 | 否 | `@(smx_b) 看一下` | `STRAY` | 0 |
| `backtick_id`（v1.5） | smx_a, smx_b | 否 | 否 | `` @`smx_b` 看一下 `` | `STRAY` | 0 |
| `bold_id`（v1.5） | smx_a, smx_b | 否 | 否 | `@**smx_b** 看一下` | `STRAY` | 0 |
| `space_display`（v1.5） | smx_a, smx_b | 否 | 否 | `@ 甲 看一下` | `STRAY` | 0 |
| `config_only`（v1.5） | smx_a, smx_b | 否 | 否 | `@ smx_x 看一下` | `STRAY` | 0 |
| `session_only`（v1.5） | smx_a, smx_b, smx_q | 否 | 否 | `@ smx_q 看一下` | `STRAY` | 0 |
| `space_dot`（v1.5） | smx_a, smx_b | 否 | 否 | `看一下 @ smx_b.` | `STRAY` | 0 |
| `space_upper`（v1.5） | smx_a, smx_b | 否 | 否 | `@ SMX_B 看一下` | `STRAY` | 0 |
| `fullwidth_everyone`（v1.5） | smx_a, smx_b | 否 | 否 | `＠所有人 看一下` | `STRAY` | 0 |
| `skip_then_display`（v1.5） | smx_a, smx_b | 否 | 否 | `价格 @ 5 元 @甲 看一下` | `STRAY` | 0 |
| `fw_paren_id`（v1.6） | smx_a, smx_b | 否 | 否 | `@（smx_b）看一下` | `STRAY` | 0 |
| `ideo_space_id`（v1.6） | smx_a, smx_b | 否 | 否 | `@{U+3000}smx_b 看一下` | `STRAY` | 0 |
| `zwsp_id`（v1.6） | smx_a, smx_b | 否 | 否 | `@{U+200B}smx_b 看一下` | `STRAY` | 0 |
| `fw_angle_id`（v1.6） | smx_a, smx_b | 否 | 否 | `@＜smx_b＞ 看一下` | `STRAY` | 0 |
| `emoji_vs_id`（v1.6） | smx_a, smx_b | 否 | 否 | `@{U+2764}{U+FE0F}smx_b 看一下` | `STRAY` | 0 |
| `ideo_space_display`（v1.6） | smx_a, smx_b | 否 | 否 | `@{U+3000}甲 看一下` | `STRAY` | 0 |
| `partly_fullwidth`（v1.6） | smx_a, smx_b | 否 | 否 | `@smx_b @（smx_a）看一下` | `PARTLY` + `ONE_B` | 2 |
| `ls_cut`（v1.6） | smx_a, smx_b | 是 | 否 | `@{U+2028}smx_b 看一下` | `smxS1Want` | 2 |
| `display_case`（v1.6；v1.7 翻） | smx_a, smx_b | 否 | 否 | `@ 坏 x 看一下` | `STRAY` | 0 |

输入里的 `\n` 是换行（`line2_*` 三行是一条两行的消息，一次 `Submit`）；v1.3 的 `\r`、`\t`、`\v`、`\f` 同样是 Go 解释型字符串里的转义，各是一个控制字节（0x0D、0x09、0x0B、0x0C），`stray_newline`、`stray_crlf` 也是一次 `Submit` 的两行消息；v1.5 的 `ps_herestring_close` 是三行、`newline_cut` 是两行（`cr_cut` 的 `\r` 是 0x0D，没有 LF），也各是一次 `Submit`。v1.6 的 `{U+XXXX}` 是一个 Unicode 码位的记法（`{U+3000}` 是全角空格，`{U+200B}` 是零宽空格，`{U+2764}{U+FE0F}` 是一个红心 emoji 加变体选择符 VS16，`{U+2028}` 是行分隔符），Go 源码里写成 `string(rune(0x3000))` 这样拼进字符串，不直接写这个字符（不可见，编辑工具与控制台都可能改掉它；U+2764、U+FE0F 另在符号禁令的码位里，只许这样拼）；`fw_paren_id`、`partly_fullwidth` 的全角括号与 `fw_angle_id` 的全角 `＜`、`＞` 是可见字符，照写或拼 `string(rune(0x…))` 都行。`slowA` 的行（`order`、`all`、`email`、`email_fullwidth`、`glued_at`、`glued_left_cjk_tail`、`glued_fullwidth`，v1.3 加 `ps_hashtable`、`sql_identity`、`backtick_at`、`ps_array`，v1.5 加 `at_everyone_cn`、`stray_eot`、`stray_newline`、`stray_crlf`、`price_at`、`numpy_matmul`、`bash_dollar_at`、`ps_herestring_close`、`newline_cut`、`cr_cut`，v1.6 加 `ls_cut`、`display_case`，共 23 行；v1.7 `display_case` 翻成 `STRAY`、去掉 `slowA`，共 22 行；v1.3、v1.4 加过的 `stray_bad_id`、`double_at`、`angle_template` 在 v1.5 翻成 `STRAY`，去掉 `slowA`）另用 `smxRequireBBeforeA` 做前提守卫。`session_only` 的成员 `smx_q` 有意不在 `smxCfg` 里：`smxSetup` 照样 `Join`（Provider 为空串，真 store 收下，原型实测）。这一行时钟为 0，smx_q 永远不会被问；不要把它改成会问到 smx_q 的输入或装置（例如去掉 `@` 后面的空格、或期望问全员），那样会撞上 provider 为空，红得像 store 的毛病。
【判别力】「有 mention 但交集为空」三种成因（`not_member` / `left` / `summary_mentioned`，外加 `only_summary_mentioned`）与「没有 mention」（`all` / `left_all` / `summary_plus_one`）、「没有可问的成员」（`no_members` / `only_summary`）两边都有；`left` 与 `left_all` 只差有没有 mention，`left_mixed` 钉「点到的已退出成员静默不问、其余照问」（手敲 `/round smx_b smx_a --go` 会整条拒绝，这里有意不同，§7.1 第 11 条）；`only_summary` 与 `only_summary_mentioned` 只差 mention，钉两条分支的先后与两句字面量不互换（M13、M13b）；`summary_plus_one` 与 `only_summary` 只差 smx_a；`judge_only` 钉「判了 judge 的名单照 PlanRound 走到 `cmdRound`、只打那两行、时钟 1 次、总用时不打」（名单显式传进去而不是在 REPL 里另判 judge）；`upper` 钉 mention 折小写之后能对上；`order` 钉名单顺序不影响屏幕（E1 等价）。杀 M10、M11、M12、M13、M13b、M26、M30。
v1.1 的 12 行：`glued_at` 只钉现状、不证明它对：`@` 左边紧贴 ASCII 字母时按左边界既不算点名、也不算没认出的点名，照问全员（§0 第 11 条 (b)、§7.3 第 4 条登记的花钱方向残余），存在的理由主要是让那条披露是实测的；它顺带杀 M38（「含 `@` 就算」时 `hi@smx_b` 也打 `STRAY`；v1.1 写的「不杀任何变异」不对，v1.2 订正，原型实测）。`summary_and_member` 钉「点到的总结成员静默不问、其余照问」（对照 `summary_mentioned`：同装置，只少 `@smx_a`）；`@smx_s` 写在前面，是为了连「只看第一个 mention 是不是总结成员」那种写法一起杀（杀 M32）。`stray_display` / `stray_fullwidth` / `stray_space` 是「想点名却没认出来」在 tokenizer 里失败的三种方式（`@` 后非 ASCII、没有 `@` 字节、`@` 后空格），各自的正对照是 `subset`（同装置、同一个意图写成 `@smx_b`），时钟 0、没有成员消息，这几行只在 `hasStrayAt` 生效时成立（杀 M36，`stray_fullwidth` 另杀 M37）。v1.1 的第四种 `stray_bad_id`（`@-smx_b`，取出的 token 不合 id 正则）在 v1.3 翻成问全员：`@` 的下一个字节是 `-`，见下面 v1.3 一段；它原来杀的 M42 变成等价（§4 E6）。v1.5 按成员判，`-` 被跳过、后面是成员 `smx_b`，这一行翻回 `STRAY`（E4-E7 随旧判据删去）。`stray_no_members` 钉没认出的 @ 先于「没有可问的成员」。`email` / `email_fullwidth` 是左边界的对照：`a@b.com`、`a＠b.com` 不算想点名，照问全员（同装置、同样没有 mention 的 `all` 给同一个期望）；`email` 杀「含 @ 就算」（M38），`email_fullwidth` 杀「全角不看左边界」（M39）。`trailing_dot` 与 `upper` 在 v1.1、v1.2 钉 `hasStrayAt` 与 ParseMentions 同样剥尾点、同样折小写：不剥或不折时这两行多出一行 `PARTLY`（M40、M41）。v1.3 收窄之后 `@smx_b.`、`@SMX_B` 的下一个字节是 ASCII 字母，不剥不折也不算没认出，M40、M41 变成等价（§4 E4、E5，原型实跑两行照绿）；这两行照旧钉「这样写认得出点名」。v1.5：这两行走的是「认成 mention 就跳过」那一支（M73 下都红）；新判据里跳过之后那一段的剥尾点、折小写由 `space_dot`、`space_upper` 钉（M71、M72）。`partly` 钉「认出一部分照问、先打一行」（对照 `subset`：同装置只多一个 `@甲`；杀 M36、M43、M44）；`partly_none` 钉三行的先后（postUser 的「不是成员」在前、`PARTLY` 居中、`NONE` 在后，原型实测）。
v1.2 的 12 行（W1b）：v1.1 各行里期望 `STRAY` / `PARTLY` 的那些，没认出的 `@` 都在文本第 0 字节，`hasStrayAt` 的左边界检查 `i > 0 && mentionLeftForbidden(text[i-1])` 在 `i > 0` 处短路、一次都没真正求值。这几行让它在 `i > 0` 时被求值，左邻各不相同：`mid_space_display`、`mid_space_fullwidth` 是空格，`mid_cjk_display` 是中文的 UTF-8 续字节（不在 `mentionLeftForbidden` 里），`line2_display`、`line2_fullwidth` 是换行，`glued_left_cjk_tail` 是 `x`，`glued_fullwidth` 是 `i`。`line2_mention` 是第 2 行的正对照（同装置、同位置写成 `@smx_b`，只问 smx_b），同时钉 §3.2 说的「行首」含第 2 行起的行首。`stray_after_mention` 是没认出的 `@` 排在认出的点名之后（对照 `partly`：同样两个 `@`，顺序相反）。`glued_left_cjk_tail` 是左边界挡住、后面又取不出 token 的半角 `@`，照问全员（对照 `stray_display`：同一个 `@甲`，只差左边紧贴的 `x`）；`glued_fullwidth` 是全角的同一件事（对照 `stray_fullwidth`：只差左边的 `hi`），也是 §7.3 第 4 条花钱残余的实测。`ps_hashtable`、`sql_identity`、`at_everyone_cn` 在 v1.2 把「代码与 IM 写法里的 `@` 谁都不问」钉成实测；v1.3 采纳方向 (A)（§7.2 第 8 条），前两行翻成问全员，`at_everyone_cn` 仍是 `STRAY`（`@` 后是非 ASCII），与 `stray_display` 同类、不另杀变异（M60 的杀手之一；v1.5 翻成问全员，见下面 v1.5 一段）。杀手（v1.2 在 `ch1_fix` 上实测，每个点名的子测试单跑都红；另把 C3、C11、C13 全部用例在每条变异下各跑一遍，红的恰好是下面列的这些；v1.3 在 `ch1_v13` 上把 C3、C5、C11、C13 全部用例重跑一遍，M50 的杀手按 §4 更新，M51-M55 不变）：M50（只看第 0 字节）被 `mid_space_display`、`mid_space_fullwidth`、`mid_cjk_display`、`line2_display`、`line2_fullwidth`、`stray_after_mention` 杀（v1.2 还有 `ps_hashtable`、`sql_identity` 与 C13 的旧输入 `$h = @{…}`，v1.3 前两行翻了、C13 换了输入；v1.3 另有 `stray_eot`、`stray_newline`、`stray_crlf`、`price_at` 与 C13 `stray`）；M51（半角不看左边界）只被 `glued_left_cjk_tail` 杀；M52（左邻是非 ASCII 字节也挡）只被 `mid_cjk_display` 杀；M53（第一个合法起头的 `@` 定胜负）只被 `stray_after_mention` 杀；M54（全角只认第 0 字节）被 `mid_space_fullwidth`、`line2_fullwidth` 杀；M55（换行也挡）被 `line2_display`、`line2_fullwidth` 杀。`glued_left_cjk_tail`、`glued_fullwidth` 另杀 M39。这六条变异让 v1.1 的 C3 28 行与 C11 8 行全部存活（W1b 实测，v1.2 复核）。v1.5：M53 换了锚点，M50-M55 的杀手栏按新判据重测，以 §4 为准。
v1.3 的 10 行与 3 行翻转（主代理裁决 `ch1_rulings_v13.md` 第 1 条；v1.5 注：本段说的 M56-M64 随 v1.3 判据删去，`stray_bad_id`、`double_at` 翻回 `STRAY`，`stray_eot`、`stray_newline`、`stray_crlf`、`price_at` 翻成问全员，以下面 v1.5 一段为准）：收窄之后半角 `@` 只有下一个字节是 ≥ 0x80、ASCII 空白或文本结尾才算没认出。翻成问全员的三行 `stray_bad_id`（下一个字节是 `-`）、`ps_hashtable`（`{`）、`sql_identity`（第一个 `@` 后面是 `@`；第二个 `@` 左边是 `@`，被左边界挡住），与新加的 `double_at`（`@@smx_b`，同 `sql_identity` 的两步）、`backtick_at`（`@` 后面是反引号）、`ps_array`（`(`），期望都是 `smxS1Want`、时钟 2，所以都用 `slowA`；这是花钱方向，是 Griffin 要的方向。它们的对照：同装置、同样没有 mention 的 `all` 给同一个期望；`stray_display` 是只差下一个字节（非 ASCII）的反面。仍是 `STRAY` 的新夹具：`stray_eot`（`你看 @`）钉「文本结尾」一支；`stray_newline`（`第一行 @\n第二行`）、`stray_crlf`（`第一行 @\r\n第二行`）钉空白里的换行与回车，`@` 在句中、消息是两行；`stray_tab`、`stray_vtab`、`stray_formfeed` 钉 `\t`、`\v`、`\f`（`\v`、`\f` 也算空白是 v1.3 冻结的取舍，理由见 §7.1 第 12 条）；`price_at`（`价格 @ 5 元`）是「不是点名、仍谁都不问」的实测，也是 C13 `stray` 在简洁档下的对照。这 7 行都是时钟 0、四个成员都没有消息，只在 `hasStrayAt` 生效时成立（M36 下全红，原型实测）。杀手（`ch1_v13` 实测，每个点名的子测试单跑都红；另把 C3、C5、C11、C13 全部用例在每条变异下各跑一遍，M56-M64 红的集合恰好是 §4 杀手栏列的这些）：M56（去掉收窄，回到 v1.2）只被上面翻转的六行杀（v1.4 另加 `angle_template`、`partly_ascii` 两行，见下一段）；M57（收窄也套到全角）被 `stray_fullwidth`、`mid_space_fullwidth`、`line2_fullwidth` 杀；M58（空白不算）被 `stray_space`、`stray_newline`、`stray_crlf`、`stray_tab`、`stray_vtab`、`stray_formfeed`、`price_at` 与 C13 `stray` 杀；M59（文本结尾不算）只被 `stray_eot` 杀；M60（≥ 0x80 不算）被 `@` 后是非 ASCII 的九行与 C11 `stray_concise`、`partly_concise` 杀；M61（换行不算）只被 `stray_newline` 杀；M62（`\r` 不算）只被 `stray_crlf` 杀；M63（`\t` 不算）只被 `stray_tab` 杀；M64（`\v`、`\f` 不算）只被 `stray_vtab`、`stray_formfeed` 杀。
v1.4 的 2 行（W1c F1、F5；都只钉现状、不证明它对，方向待 §7.2 第 9 条；v1.5 注：两行都翻了，`angle_template` 是 `STRAY`、`partly_ascii` 是 `PARTLY` + `ONE_B`，见下面 v1.5 一段）：`angle_template`（`@<smx_b> 看一下`）是照抄 §3.2 提示里 `@<成员 id>` 模板的写法，`@` 的下一个字节是 `<`，按 v1.3 判据不算没认出，这句问全员（`smxS1Want`、时钟 2，所以用 `slowA`；v1.2 下是 `STRAY`），是 §7.3 第 7 条花钱代价的实测。它的对照：同装置、同样没有 mention 的 `all` 给同一个期望；`stray_display` 是只差下一个字节（非 ASCII）的反面。`partly_ascii`（`@smx_b @(smx_a) 看一下`）钉 §7.1 第 13 条 v1.3 的例外：`@(smx_a)` 既不是 mention、也不算没认出，被静默吞掉，只问 smx_b、不打 `PARTLY`（`ONE_B`、时钟 2；v1.2 下是 `PARTLY` + `ONE_B`）。这一行是缺席断言（整串里没有 `PARTLY` 那一行），同装置的正对照是 `stray_after_mention`（同一位置写成 `@甲`，打 `PARTLY` + `ONE_B`）。两行都能为假：M56（去掉收窄）与 M38（含 `@` 就算）下都红，另有 M26 下 `angle_template` 红、M30 下 `partly_ascii` 红；M36、M50、M53 下照绿（原型 `ch1_fix3` 实测；M50-M64 红的集合与杀手栏恰好相同，也是在这两行加进来之后重跑的）。采纳 §7.2 第 9 条 (a) 时 `angle_template` 翻成 `STRAY`（原型变体实跑，C3 / C5 / C11 / C13 里只翻这一行）；`partly_ascii` 用的是 `@(`，不受 (a) 影响，所以不写成 `@<smx_a>`。
v1.5 的 15 行与 9 行翻转（主代理裁决 `ch1_rulings_v15.md` 第 1、2 条；原型 `ch1_v15` 实测，C3 共 67 行）：判据改成按成员判（§1.1 E、§7.1 第 12 条）。翻成问全员的五行 `at_everyone_cn`、`price_at`、`stray_eot`、`stray_newline`、`stray_crlf`：`@` 后面跳过 ASCII 空白与标点之后不是任何成员的 id 或显示名，或者跳过时停在换行、文本结尾上；期望 `smxS1Want`、时钟 2，改用 `slowA`，对照是同装置、同样没有 mention 的 `all`。翻成 `STRAY` 的三行 `angle_template`、`stray_bad_id`、`double_at`：`<`、`-`、第二个 `@` 被跳过，后面是成员 `smx_b`，时钟 0，去掉 `slowA`。`partly_ascii` 翻成 `PARTLY` + `ONE_B`：`@(smx_a)` 跳过 `(` 之后是成员 `smx_a`，打第 4 句、照问 smx_b（与 `stray_after_mention` 同形，不再是缺席断言）。新夹具：问全员的 `numpy_matmul`、`bash_dollar_at`、`ps_herestring_close`（裁决书点名，是 W1c 第 1、2 条的代表写法）与正对照 `newline_cut`、`cr_cut`（`@` 与成员 id 之间隔着 LF 或单独一个 CR 时不算冲着成员，问全员；与 `stray_space`、`stray_tab` 只差 `@` 与 `smx_b` 之间那个字节）；`STRAY` 的 `paren_id`、`backtick_id`、`bold_id`（`@` 后面是 `(`、反引号、`**`，后两种是从 markdown 复制来的写法）、`space_display`（跳过空格之后是显示名「甲」）、`config_only`（`@ smx_x`：smx_x 在配置里、没加入）、`session_only`（`@ smx_q`：smx_q 在会话里、配置里没有）、`space_dot`、`space_upper`（跳过之后取出的那一段也要剥尾点、折小写）、`fullwidth_everyone`（全角后面不冲着成员照样算）、`skip_then_display`（前一个 `@` 不冲着成员，后一个 `@甲` 冲着，要接着往后看）。这些 `STRAY` 行都是时钟 0、四个成员都没有消息，正对照是 `subset`（同装置、写成 `@smx_b` 只问 smx_b）与 `all`。杀手（每个点名的子测试单跑都红；另把 C3、C5、C11、C13 全部用例在每条变异下各跑一遍，M36、M38、M50-M55、M53b、M65-M76 各自红的集合与 §4 杀手栏恰好相同）：M65（跳过之后有任何一段就算）被 `ps_hashtable`、`sql_identity`、`price_at`、`numpy_matmul` 杀；M66（去掉显示名）被 `@` 后是显示名的十行与 C11 `stray_concise`、`partly_concise` 杀；M67（只用会话成员）只被 `config_only` 杀；M68（只用配置）只被 `session_only` 杀；M69（LF 不截断）只被 `newline_cut` 杀；M69b（CR 不截断）只被 `cr_cut` 杀；M70（全角也按成员判）被 `fullwidth_everyone` 与 C13 `stray` 杀；M71、M72 各只被 `space_dot`、`space_upper` 杀；M53（认出的点名之后不再往后看）被 `stray_after_mention`、`partly_ascii` 杀；M53b（第一个没认出的合法 `@` 定胜负）只被 `skip_then_display` 杀；M73（去掉「认成 mention 就跳过」）被点名认出的十二行与 C5 `auto` 杀；M74-M76 的杀手见 §4。
v1.6 的 9 行（W1d 修订；原型 `ch1_fix5` 实测，C3 共 76 行）：W1d F1 查出 v1.5 只跳 ASCII 字节，`@` 后面先跟一个非 ASCII 的分隔符再跟成员时问全员，与认出的点名混用时静默少问（相对 v1.4 的回退）。v1.6 由 `atSkip` 也跳非 ASCII 的空白、标点、符号、组合记号、格式字符（§1.1 E、§7.1 第 12 条），七行把每一类钉成实测：`fw_paren_id`（全角括号，P）、`ideo_space_id`（全角空格 U+3000，Zs）、`zwsp_id`（零宽空格 U+200B，Cf）、`fw_angle_id`（全角 `＜`，S：中文输入法全角标点下敲出的 `<`）、`emoji_vs_id`（红心 U+2764 是 S，后面的变体选择符 U+FE0F 是 M）、`ideo_space_display`（全角空格之后是显示名「甲」）、`partly_fullwidth`（`@smx_b @（smx_a）`：与认出的点名混用时打第 4 句、照问 smx_b，就是 W1d 说的静默少问）。这七行拿到 v1.5 的原型上跑都红（前六行问全员，`partly_fullwidth` 只有 `ONE_B`、没有第 4 句），是回退的实测。`STRAY` 的六行时钟 0、四个成员都没有消息，正对照是 `subset`（同装置写成 `@smx_b`）与 `paren_id`（同一个意图的半角括号写法）。`ls_cut`（`@`、U+2028、`smx_b`）是 `cr_cut` 的非 ASCII 同类：行分隔符不在跳过的五类里，与 CR / LF 一样停下，问全员（`slowA`）；它与 `ideo_space_id` 只差 `@` 与 `smx_b` 之间那一个字符。`display_case`（`@ 坏 x 看一下`）只钉现状、不证明它对：显示名逐字节比、不折大小写（裁决书第 1 条），smx_x 的显示名是「坏 X」，敲成小写就不算，问全员（`slowA`）；W1d 定为 major，方向待主代理（§7.2 第 12 条），采纳大小写不敏感的比法时这一行翻成 `STRAY`（原型变体实跑：98 个子测试里只翻这一行）；它的对照是 `space_display`（同样隔着空格跟显示名，大小写没有差别）。（v1.7 注：主代理定了方向 (a)，这一行翻成 `STRAY`，见下面 v1.7 一段。）杀手（每个点名的子测试单跑都红；另把 C3、C5、C11、C13、C14 全部用例在每条变异下各跑一遍，红的集合与 §4 杀手栏恰好相同）：M77（非 ASCII 一个都不跳，回到 v1.5）被上面七行与 C14 `letters_stop_control` 杀；M78（Zs 不跳）被 `ideo_space_id`、`ideo_space_display` 杀；M79（P 不跳）被 `fw_paren_id`、`partly_fullwidth` 与 C14 `letters_stop_control` 杀；M80（S 不跳）被 `fw_angle_id`、`emoji_vs_id` 杀；M81（M 不跳）只被 `emoji_vs_id` 杀；M82（Cf 不跳）只被 `zwsp_id` 杀；M83（Zl、Zp 也跳）只被 `ls_cut` 杀。M36 下七行都红，M38、M76 下 `ls_cut`、`display_case` 红（v1.7 起 `display_case` 是 `STRAY`，这两条下不再红，已从杀手栏拿掉），M74 下 `ls_cut` 红，M75 下除 `ideo_space_display` 外的六行红，M50、M53 下 `partly_fullwidth` 红，都已补进各自的杀手栏。
v1.7 的 1 行翻转（主代理裁决 `ch1_rulings_v17.md` 第 1 条，§7.2 第 12 条定方向 (a)；原型 `ch1_v17` 实测，C3 仍是 76 行）：显示名改成不分大小写的前缀比（§1.1 E：先按显示名的字节长度切一段，剩下的够长才用 `strings.EqualFold` 比）。`display_case`（`@ 坏 x 看一下`）翻成 `STRAY`、时钟 0、四个成员都没有消息，去掉 `slowA`（`slowA` 23 行 → 22 行）。先把 v1.6 的期望原样放在改了这一行代码的原型上跑，C3 / C5 / C11 / C13 / C14 的 98 个子测试里红的恰好只有 `display_case`（输出是第 3 句，`smxRequireBBeforeA` 前提不成立），与裁决书的预期一致。它的正对照仍是 `space_display`（同样隔着空格跟显示名，大小写相同）；同装置的缺席对照在 C14 `display_case_miss`（`smxSetup` 的配置是只读引用的既有辅助，去不掉「坏 X」，改在白盒里去掉这个显示名）。杀手（原型 `ch1_v17` 实测，每个点名的子测试单跑都红）：M85（显示名逐字节比，回到 v1.6）被 `display_case` 与 C14 `display_case_hit` 杀，红的集合恰好是这两个；M36（去掉 `hasStrayAt`）、M66（去掉显示名那一支）下它问全员，已补进这两条的杀手栏；M38、M76 下它照样是 `STRAY`，已从这两条的杀手栏拿掉。另两条新变异落在长度守卫上：M86（去掉长度守卫）在剩下的文本比某个显示名短时切片越界 panic，`stray_eot` 单跑必定 panic；`mid_space_display`（`看一下 @甲`：`@` 后面只剩 3 个字节）只在比「甲」长的显示名先被比到时 panic，`names` 由 map 遍历构造、顺序随机，定向核验人分进程单跑 60 次，52 次 panic、8 次绿，所以不作 M86 的杀手；M87（`>=` 写成 `>`，差一）只被 `mid_space_display` 杀（显示名「甲」恰好到文本结尾）。

**C4 `TestChxAutoOnlyBareText`**（只有裸文本触发；表驱动；成员 `{smx_a}`，slowA 否）：

| 子测试 | 自动回复 | 输入 | 期望输出 | 时钟 |
|---|---|---|---|---|
| `bare` | 开 | `你好` | `ONE_A` | 2 |
| `bare_manual` | 关 | `你好` | `""` | 0 |
| `say` | 开 | `/say 你好` | `""` | 0 |
| `to` | 开 | `/to smx_a 你好` | `"只有 smx_a 会看到这条；其余成员永远不会 —— 游标只前进\n"` | 0 |
| `ask` | 开 | `/ask smx_a 你好` | `"正在问 smx_a ...\n甲（T）: OK\n"` | 0 |
| `round_empty` | 开 | `/round --go` | `"拒绝: 名单为空。给了 id 才用给的；没给则取当前会话最后一条用户发言里的 mention 与当前活跃成员的交集。不会默认全员。\n"` | 1 |

【判别力】`bare` 是其余五行的正对照（同装置、只差命令或开关）；`round_empty` 钉「/round 名单为空照旧拒绝」（最后一条 user 消息是 `seed-msg`，没有 mention）。杀 M5、M6。

**C5 `TestChxAutoVerboseMentionLine`**（详细档；成员 `{smx_a}`，`concise=false`，slowA 否）：
- `auto`：打开自动回复，`Submit("@smx_a 你好")`；输出（不规范化）按行切：第 1 行 == `已记下 seq 2`，第 2 行 == `发言顺序按字典序，重复的 id 已去重，只问一次。`；没有任何一行以 `点名了` 开头；整行 `本轮问 1 个: smx_a` 与 `outcome spoke  agent smx_a` 各恰好 1 次；**恰好 1 行以 `  scanned_to=2` 结尾**（v1.1 F4：详细档的「冷暖 预报=… scanned_to=N」行，`ask.go:540`；这条消息的 seq 是 2，成员读到的必须是它）。
- `auto_not_member`：打开，`Submit("@ghost 你好")`，整串 == `"已记下 seq 2\nghost 不是当前会话的成员\n"` + `NONE`（v1.3：`NONE` 是 §3.2 第 1 句加 `\n`，末尾多了 `/round` 出路，原型重跑；v1.5 出路改成不带尖括号的说法，原型 `ch1_v15` 重跑）。
- `manual`：不打开，`Submit("@smx_a 你好")`，整串 == `"已记下 seq 2\n点名了 smx_a（用 /ask smx_a 让他说话）\n"`。
- `auto_say`：打开，`Submit("/say @smx_a 你好")`，整串同 `manual`。
- `auto_to`：打开，`Submit("/to smx_a @smx_a 你好")`，整串 == `"已记下 seq 2\n只有 smx_a 会看到这条；其余成员永远不会 —— 游标只前进\n点名了 smx_a（用 /ask smx_a 让他说话）\n"`。
【判别力】「点名了」门的两侧：`auto` 与 `manual` 只差开关；`auto_say` 与 `auto` 只差命令（`/say` 走 `sayText` 的 bare=false 一侧）；`auto_to` 是 `postUserText` 包装层那一侧（`/to` 与 `/ask 文本` 只经包装），喂的是会打出「点名了」的输入，包装层若把 auto 传成 true 这一行会消失。杀 M8、M9、M27、M5。`auto` 的 `scanned_to=2` 是「先落库、再问一轮」的直接断言：把自动一轮挪到 `postUser` 之前（M31）时，原型实测这一行是 `scanned_to=1`、「已记下」变成 `seq 6` 且排到最后，这条断言单独就为假（第 1 行的断言同时也红）。杀 M31。

**C6 `TestChxAutoLineModePaths`**（行式 `Run` 的三条裸文本路径与 `/say`；成员 `{smx_a}`，`concise=true`，slowA 否；装置之后白盒 `g.r.in = strings.NewReader(脚本)`，`g.r.Run(g.ctx)` 返回 nil；输出是整个缓冲经 `smxNormalize`；另断言假时钟次数，与库里 user 消息恰好是 `seed-msg` 与表中「正文」两条）。记 `ROUND` = `ONE_A`，`ENTER` = `"进入多行输入：整段粘贴或逐行敲；单独一行 /end 发送，单独一行 /cancel 放弃\n"`，`D3` = `"   ...>    ...>    ...> "`（三个多行提示符）：

| 子测试 | 自动回复 | 脚本 | 期望输出 | 正文 | 时钟 |
|---|---|---|---|---|---|
| `single_auto` | 开 | `"你好\n"` | `"council> " + ROUND + "council> \n再见\n"` | `你好` | 2 |
| `single_manual` | 关 | `"你好\n"` | `"council> council> \n再见\n"` | `你好` | 0 |
| `draft_auto` | 开 | `"/say\n第一行\n第二行\n/end\n"` | `"council> " + ENTER + D3 + ROUND + "council> \n再见\n"` | `第一行\n第二行` | 2 |
| `draft_manual` | 关 | 同上 | `"council> " + ENTER + D3 + "council> \n再见\n"` | `第一行\n第二行` | 0 |
| `paste_auto` | 开 | `"\x1b[200~第一行\n第二行\x1b[201~\n"` | `"council> council> " + ROUND + "council> \n再见\n"` | `第一行\n第二行` | 2 |
| `paste_manual` | 关 | 同上 | `"council> council> council> \n再见\n"` | `第一行\n第二行` | 0 |
| `say_auto` | 开 | `"/say 你好\n"` | `"council> council> \n再见\n"` | `你好` | 0 |

【判别力】三条路径各自自动 / 手动两侧只差开关；`say_auto` 与 `single_auto` 只差 `/say `。`draft_auto` 担保 #47「行式多行草稿发送算裸文本」、`paste_auto` 担保本刀补的第四条路。杀 M1、M3、M4、M5、M6。

**C7 `TestChxAutoPostFailNoRound`**（成员 `{smx_a}`，concise，打开自动回复）：
- `canceled`：`ctx2` 由 `g.ctx` 派生并立即取消，`g.r.Submit(ctx2, "你好")` 返回 nil；输出按行切**恰好 1 行**且以 `发言失败: ` 开头（前缀只用在这一行：原因文本含会话 id）；假时钟 0 次；库里 user 消息恰好只有 `seed-msg`。
- `live`：同装置，用 `g.ctx` 提交 `你好`，输出 == `ONE_A`（正对照）。
【判别力】Post 失败时 `postUser` 返回 `(false, nil)`；若 `sayText` 不看 `posted`（M7），取消的 ctx 让 `autoReplyRound` 的读成员表失败，多打一行 `round: …`，第一条断言红。杀 M7。

**C8 `TestChxAutoNoRunner`**（rn 为 nil）：`g := smxSetup(t, "smx_s", {smx_a}, true, false)` 只拿它的 `svc`、`convID`、`ctx`；另造 `r := New(smxCfg("smx_s"), g.svc, nil, nil, b, RulesContext{})`（`b` 是新 `rbaBuf`），`SetConcise(true)`，按子测试 `SetAutoReply`；`Submit("/use "+convID)` 后清空 `b`，`Submit("你好")` 返回 nil：
- `auto`：`b.String()` == `"provider 调用未就绪，本命令不可用\n"`；
- `manual`：== `""`。
两个子测试都断言库里 user 消息恰好是 `seed-msg`、`你好`（消息照常记下）。
【判别力】自动一轮经 `runCommand(roundCmd)` 才有 needsRunner 这道闸；直接调 `cmdRound`（M15）或 `roundCmd` 不要 Runner（M16）时空指针 panic，整个测试二进制红。

**C9 `TestChxAutoCommand`**（三个子测试 `steps`、`summary`、`no_summary`；v1.1 F6 把 v1 的单个用例拆成这三个）。
`steps`：`r := New(nil, nil, nil, nil, b, RulesContext{})`（cfg 为 nil 时 New 兜底成空配置，没有 `[summary]`，所以打开时是 `ON` 的无总结那一句），依次 `Submit` 下面 9 步，每步之前清空 `b`，每步断言 `b.String()` 与之后的 `r.autoReply`：

| 步 | 命令 | 输出 | 之后的 autoReply |
|---|---|---|---|
| 1 | `/auto` | `ON` | true |
| 2 | `/auto` | `OFF` | false |
| 3 | `/auto on` | `ON` | true |
| 4 | `/auto on` | `ON` | true |
| 5 | `/auto x` | `USAGE` | true |
| 6 | `/auto off` | `OFF` | false |
| 7 | `/auto off` | `OFF` | false |
| 8 | `/auto on off` | `USAGE` | false |
| 9 | `/auto ON` | `USAGE` | false |

（`ON` = §3.1 第 2 句（`msgAutoOnNoSummary`）加 `\n`，`OFF` / `USAGE` = §3.1 第 3、4 句各加 `\n`；v1 的 `ON` 是第 1 句，v1.1 改了。）再 `Submit("/help")`，按行切，下面 4 行**各恰好出现一次**（整行相等；命令名之后的空白是 tabwriter 补的，原型实测到第 12 列，即 `/auto` 后 7 个空格、`/say` 后 8 个、`/session` 后 4 个、`/round` 后 6 个）：

```text
/auto       切换直接打字要不要让成员回复: /auto 切换；/auto on 自动回复（点名了谁就只问谁，没点名问全部成员）；/auto off 只记下
/say        只记下、不让任何人回复: /say <文本>；/say /foo 发字面量 /foo；/say 不带参数进多行输入（/end 发送时与直接打字相同）
/session    列出当前会话每个成员的 provider 会话；/session rebuild <agent-id> 先结清这个成员留在库里的陈旧在飞记录，再丢弃并重建
/round      按名单问一轮，--go 时发言者同时起进程；多个成员跑完自动总结（只给你看）: /round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]
```

`summary` / `no_summary`（v1.1）：`r := New(smxCfg(sum), nil, nil, nil, b, RulesContext{})`，`summary` 的 `sum` 是 `"smx_s"`，`no_summary` 的是 `""`；依次 `Submit` `/auto on`、`/auto off`、`/auto`，每步之前清空 `b`，三步的输出依次 == 打开句、`OFF`、打开句，其中打开句 `summary` 是 §3.1 第 1 句（`msgAutoOn`）加 `\n`，`no_summary` 是第 2 句（`msgAutoOnNoSummary`）加 `\n`。
【判别力】第 4、7 步钉 on / off 是设定不是切换；第 5、8、9 步钉用法不改状态（第 5 步在 true 上、第 8、9 步在 false 上，两侧都有）；`/round` 那一行钉 `roundCmd` 的 summary 与 HEAD 逐字相同。杀 M21、M21b。`summary` 与 `no_summary` 只差配置里的 `[summary]`，钉两句按配置选、两侧都有夹具（`/auto on` 设定与 `/auto` 切换两条路都走到）：无条件打第 1 句（M49）时 `no_summary` 与 `steps` 红，无条件打第 2 句（M49b）时 `summary` 红。

**C10 `TestChxAddHidesSummary`**（`/add` 与总结成员）。装置（每个子测试新造）：`svc := newRoundStreamService(t)`，`svc.Create(ctx, "chx")`，`cfg := smxCfg(sum)` 后 **`cfg.Providers = nil`**（没有 provider 就不识别模型，不打识别表），`r := New(cfg, svc, nil, nil, b, RulesContext{})`，`/use` 之后清空。输出里把会话 id 换成 `<C>`。记 `TAIL` = `"用 /add <编号|全名|前缀> 加入，多个用空格或逗号隔开，例如 /add 1,2\n"`，`REJ` = §3.3 那一句（`%s` 是 `smx_s`）加 `\n`。两张候选表（整串，列间是 tabwriter 的空格）：

```text
WITH_SUM:
NO  AGENT  PROVIDER  DISPLAY_NAME
1   smx_a  claude    甲
2   smx_b  claude    -
3   smx_j  claude    裁判 J
4   smx_k  grok      坏 K
5   smx_x  grok      坏 X
6   smx_y  grok      坏 Y
7   smx_z  grok      坏 Z

NO_SUM:
NO  AGENT  PROVIDER  DISPLAY_NAME
1   smx_a  claude    甲
2   smx_b  claude    -
3   smx_j  claude    裁判 J
4   smx_k  grok      坏 K
5   smx_s  claude    总结 S
6   smx_x  grok      坏 X
7   smx_y  grok      坏 Y
8   smx_z  grok      坏 Z
```

（`WITH_SUM:` / `NO_SUM:` 这两个标签行与中间空行不是输出的一部分；每张表之后紧跟 `TAIL`。）
- `list`：`sum="smx_s"` 时 `/add` 整串 == `WITH_SUM` + `TAIL`；`sum=""` 时 == `NO_SUM` + `TAIL`。
- `number`：`sum="smx_s"` 时 `/add 5` == `"smx_x 已加入会话 <C>\n"`；`sum=""` 时 == `"smx_s 已加入会话 <C>\n"`。
- `explicit`：对 `/add smx_s`、`/join smx_s`、`/add smx_a,smx_s` 三条，`sum="smx_s"` 时整串都 == `REJ`，且之后活跃成员为空（全或无：`smx_a` 也没加）；对照 `sum=""` 时输出含 `"smx_s 已加入会话 <C>\n"`。
- `active`：`sum="smx_s"`，先 `svc.Join` smx_s（`Active: true`），`/add smx_s` == `REJ`。
- `left_rejoin`：`sum="smx_s"`，先 `svc.Join` smx_s、再 `svc.Leave`（前提守卫：此后活跃成员为空），`/join smx_s` == `"smx_s 已加入会话 <C>\n"`，之后活跃成员恰好是 `[smx_s]`；再 `/add` == `WITH_SUM` + `TAIL`。
【判别力】`list` / `number` 两侧只差配置；`explicit` 三条经同一个 `resolveAddSegment`；`active`、`left_rejoin`、`explicit` 覆盖放行条件 `member && !active` 的全部三种取值（没加入过、在场、已退出）。杀 M22、M23、M24、M25。

**C11 `TestChxAutoWriteFailure`**（写屏幕失败；包装层喂原返回值非空的输入；v1.1 改成表驱动，档位按行给）：装置 `smxSetup(t, "smx_s", 成员, 档位, false)`，打开自动回复，白盒 `g.r.out` 换成一个每次 `Write` 都返回 `(0, errChxWrite)` 的写者（`errChxWrite` 是包级哨兵）。每行断言：`Submit(输入)` 返回的 error 满足 `errors.Is(err, errChxWrite)`；假时钟次数 == 表中值；smx_a、smx_b、smx_s、smx_j 都没有消息；库里 user 消息恰好是 `seed-msg` 与表中「正文」两条。

| 子测试 | 成员 | 档位 | 输入 | 正文 | 时钟 |
|---|---|---|---|---|---|
| `bare` | smx_a | 详细 | `你好` | `你好` | 0 |
| `say` | smx_a | 详细 | `/say 你好` | `你好` | 0 |
| `to` | smx_a | 详细 | `/to smx_a 你好` | `你好` | 0 |
| `bare_concise`（v1.1） | smx_a | 简洁 | `你好` | `你好` | 1 |
| `no_members_concise`（v1.1） | （空） | 简洁 | `你好` | `你好` | 0 |
| `summary_mentioned_concise`（v1.1） | smx_a, smx_s | 简洁 | `@smx_s 你好` | `@smx_s 你好` | 0 |
| `stray_concise`（v1.1） | smx_a | 简洁 | `@甲 你好` | `@甲 你好` | 0 |
| `partly_concise`（v1.1） | smx_a | 简洁 | `@甲 @smx_a 你好` | `@甲 @smx_a 你好` | 0 |

【判别力】详细档下 `postUser` 在「已记下 seq N」处写失败，返回 `(true, err)`：`bare` 钉 `sayText` 必须先看 err（M7b：只看 `!posted` 时会带着失败的屏幕去问一轮，时钟被调用）；`to` 钉包装层把 err 原样交出（M28）。v1.1：简洁档落库不写屏幕，写失败发生在自动一轮里面，这五行各喂 `autoReplyRound` 的一处写屏幕：`bare_concise` 走到 `cmdRound`，它的 `start := r.clock()` 调一次时钟，随后「本轮问 1 个」写失败、defer 不打总用时、不再碰时钟，所以时钟**恰好 1 次**（原型实测；写失败之前没有并发，这个值是确定的）；这一条同时是前提守卫，证明写失败发生在 `cmdRound` 里面而不是更早。`bare_concise` 与 `bare` 只差档位，互为对照。`bare_concise` 杀 `sayText` 吞掉 `autoReplyRound` 的错误（M33）与 `autoReplyRound` 吞掉 `runCommand(roundCmd)` 的错误（M34）；`no_members_concise`、`summary_mentioned_concise`、`stray_concise` 分别杀吞掉 `msgAutoNoMembers`（M47）、`msgAutoNoneMentioned`（M48）、`msgAutoStrayAt`（M46）写错误的变异；`partly_concise` 杀吞掉 `msgAutoStrayAtPartly` 写错误（M45：吞掉之后照问一轮，`cmdRound` 里的写失败照样返回 `errChxWrite`，但时钟变成 1，由时钟那一条杀），并与 C3 `partly` 一起杀 M36。后果说明（照 V2）：生产上 TUI 的写者 `*tui.LineWriter` 永远返回 nil（`tui/writer.go:68-72`），行式模式下一次写提示符同样会以写错误退出，所以这几条变异的真实后果都是零；这里钉的是包装层的卫生。v1.3：裁决书第 1 条要把 `stray_concise`、`partly_concise` 换成新判据下仍是 STRAY 的输入；它们的输入 `@甲 你好`、`@甲 @smx_a 你好` 里没认出的 `@` 后面是非 ASCII，新判据下本来就仍是 STRAY（原型实跑：两行照绿；M36、M60 下两行都红，M46 下 `stray_concise` 红，M45 下 `partly_concise` 红），所以输入不换。整张表在 v1.3 原型上重跑，八行全绿（`summary_mentioned_concise` 走的是改了字面量的第 1 句，写失败照样返回 `errChxWrite`）。v1.5：两行的 `@甲` 按成员判仍是没认出（配置里 smx_a 的显示名是「甲」），输入不换；整张表在原型 `ch1_v15` 上重跑八行全绿；M36、M66 下两行都红，M74 下 `stray_concise` 红，M45、M46 照旧各杀一行（M60 随 v1.3 判据删去）。

**C12 `TestChxAutoParticipantsFailAfterPost`**（v1.1 F3：Post 已成功、随后读成员表失败）。三个子测试；前两个 `plain`（输入 `你好`）与 `mention`（输入 `@smx_a 你好`）装置相同：`smxSetup(t, "smx_s", {smx_a}, concise=false, slowA=false)`，打开自动回复；`ctx, cancel := context.WithCancel(g.ctx)`；`w := g.installHook("已记下 seq ", func() error { cancel(); return nil })`；`g.r.Submit(ctx, 输入)` 返回 nil。断言：`requireFired(t, w, 1)`（前提守卫：钩子在「已记下 seq 」那次写入时触发，恰好一次）；`w.String()` 按行切**恰好两行**，第 1 行 == `已记下 seq 2`，第 2 行匹配 `^round: .*: context canceled$`（原因文本含会话 id，只钉头尾）；假时钟 0 次；smx_a、smx_b、smx_s、smx_j 都没有消息；库里 user 消息恰好是 `seed-msg` 与输入。
【判别力】钩子在写者的 `Write` 里同步调用，与 `postUser` 的 printf 在同一个 goroutine 上，这时 `Post` 已经成功；取消之后 `autoReplyRound` 的 `Participants` 必然失败，走 `return r.reportErr("round", err)`。`plain` 在 postUser 里没有 token、不读成员表，直接到 `autoReplyRound`；`mention` 先经 postUser 读成员表那一支（`if err != nil { return true, nil }`，同样因取消失败），再到 `autoReplyRound`。所以：那一支静默返回 nil（M35）时两个子测试的输出都只剩第 1 行；把 parts 置空（M35b）时第 2 行变成 `EMPTY`（`plain`）或 `NONE`（`mention`），是对用户说假话；postUser 那一支返回 `false`（M35c）时 `mention` 的输出只剩第 1 行，`plain` 照绿（它不走那一支），所以 `plain`、`mention` 两个子测试缺一不可。`mention` 的正对照是 C5 `auto`（同装置、同输入、不取消：这一轮照常起进程、smx_a 有消息）。
第三个子测试 `write_fail`（「同类的别处」：这一支里 `reportErr` 的写屏幕错误也要并进返回值）：装置同上，输入 `你好`，`g.r.out` 换成测试自己的写者：第一次写入照常收下（记下内容），内容以 `已记下 seq ` 开头时在写入里调 `cancel`；之后每次写都返回 `(0, errChxWrite)`。断言：第一次写入的内容 == `"已记下 seq 2\n"`（前提守卫：取消发生在 Post 成功之后、恰好这一处）；`Submit` 的返回值满足 `errors.Is(err, errChxWrite)`；假时钟 0 次；四个成员都没有消息；user 消息恰好是 `seed-msg`、`你好`。把这一支写成 `_ = r.reportErr(…); return nil`（M35d）时 `Submit` 返回 nil，红；它与 `plain` 只差写者。

**C13 `TestChxAutoStrayThenRound`**（v1.2，W1b F2：`msgAutoStrayAt` 给的出路要实测；v1.3 改成两个子测试 `stray`、`none`）。两个子测试装置相同：`smxSetup(t, "smx_s", {smx_a}, concise=false, slowA=false)`，打开自动回复；只差输入 `IN` 与第 1 步的期望 `W1`：
- `stray`：`IN` = `看一下 ＠所有人`（v1.5：不是点名的全角 `＠`，`＠` 在第 10 字节；全角到左边界就算没认出，这句本来就想问全体，正好照第 3 句的出路补问。v1.3、v1.4 的输入 `价格 @ 5 元` 在 v1.5 翻成问全员，换掉；再早 v1.2 的 `$h = @{ Name = 'x' } 这行为什么报错` 在 v1.3 翻了），`W1` = `"已记下 seq 2\n"` + `STRAY`。
- `none`（v1.3）：`IN` = `npm i @types/node 报错`（解析得出 token `types`，是「点的不是成员」），`W1` = `"已记下 seq 2\ntypes 不是当前会话的成员\n"` + `NONE`。
- 第 1 步：`Submit(IN)` 返回 nil；输出（不规范化）整串 == `W1`；假时钟 0 次；smx_a 没有消息。
- 第 2 步：清空缓冲，`Submit("/round smx_a --go")` 返回 nil；输出按行切，**恰好 1 行以 `  scanned_to=2` 结尾**（同 C5 `auto`：详细档的「冷暖 预报=… scanned_to=N」行）；整行 `本轮问 1 个: smx_a` 与 `outcome spoke  agent smx_a` 各恰好 1 次；smx_a 的消息恰好 1 条；库里 user 消息恰好是 `seed-msg`、`IN`。
【判别力】主要的理由是让 §3.2 第 3 句（`stray`）与 v1.3 第 1 句（`none`）的出路是实测的：「这条已经记下了、接着敲 /round、空格、成员 id、空格、--go 就让成员回复」（v1.5 的说法，第 2 步敲的就是 `/round smx_a --go`），而成员读到的正是这一条（`scanned_to=2`，这条消息的 seq 是 2）。能为假的情形：这条消息若没进库，库里就没有 seq 2，第 2 步不会有以 `  scanned_to=2` 结尾的行，第 1 步的整串也少了「已记下 seq 2」。第 1 步是详细档下「先落库、再判名单」的直接断言（另一个是 C5 `auto`）：把自动一轮挪到 `postUser` 之前（M31）时，原型实测两个子测试第 1 步的整串都变成说明句在前、`已记下 seq 2` 在后，红；杀 M31（`stray` v1.2 补、`none` v1.3 补）。`stray` 同时是 C3 `fullwidth_everyone` 在详细档下、`＠` 挪到句中的对照（v1.5；v1.3、v1.4 对照的是 `price_at`，那一行 v1.5 翻成问全员）：`＠` 在第 10 字节，M50（只看第 0 字节）、M54（全角只认第 0 字节）下这一步不打 `STRAY`、照问 smx_a，红；M37（不认全角）、M70（全角也按成员判：`所有人` 不是成员）同样红；杀 M50、M54、M37、M70（原型 `ch1_v15` 实测；v1.4 这里杀的 M58 随旧判据删去）。`none` 另在 M12、M13 下红（原型实测，第 1 步不打第 1 句）。

**C14 `TestChxHasStrayAtNames`**（v1.6，W1d 修订：白盒，同包直接调 `hasStrayAt(text, known, names)`，表驱动，每行一个子测试）。`known` 固定为 `{"smx_a": true, "smx_b": true}`，`names` 按行给；每行断言返回值 == 表中「算」（是 = `true`，否 = `false`）。为什么要白盒：`smxSetup` 的配置是只读引用的既有辅助，里面没有以括号、引号开头的显示名，端到端造不出这几种输入。v1.7 加两行（共 9 行）：`names` 一格里用顿号隔开、各自带反引号的是多个显示名，照写的顺序放进 `names`（顺序不影响结果）。

| 子测试 | names | 输入 | 算 |
|---|---|---|---|
| `lead_fw_bracket` | `【测试】乙` | `@【测试】乙 看一下` | 是 |
| `lead_fw_bracket_space` | `【测试】乙` | `@ 【测试】乙 看一下` | 是 |
| `lead_ascii_paren` | `(bot) X` | `@(bot) X 看一下` | 是 |
| `lead_ascii_paren_space` | `(bot) X` | `@ (bot) X 看一下` | 是 |
| `lead_negative` | `(bot) X` | `@(bot) 看一下` | 否 |
| `letters_stop` | `甲` | `@所有人，smx_b 也看一下` | 否 |
| `letters_stop_control` | `甲` | `@，smx_b 也看一下` | 是 |
| `display_case_hit`（v1.7） | `甲`、`裁判 J`、`总结 S`、`坏 X`、`坏 Y`、`坏 Z`、`坏 K` | `@ 坏 x 看一下` | 是 |
| `display_case_miss`（v1.7） | `甲`、`裁判 J`、`总结 S`、`坏 Y`、`坏 Z`、`坏 K` | `@ 坏 x 看一下` | 否 |

【判别力】钉 v1.6 的「显示名在跳过途中每个位置都比」与「字母、汉字不跳」，两件事在 `smxCfg` 上都造不出来。`lead_fw_bracket`、`lead_fw_bracket_space` 的显示名以全角方括号开头，`lead_ascii_paren`、`lead_ascii_paren_space` 以 ASCII 括号开头：显示名只在跳完之后比（M84，就是 W1d 建议的写法）时，开头那个括号先被跳掉，四行都变成 `false`，红；去掉显示名那一支（M66）时同样四行红。v1.5 的原型上 `lead_ascii_paren`、`lead_ascii_paren_space` 就是 `false`（实测红；`lead_fw_bracket*` 绿，因为 v1.5 不跳全角方括号）：以 ASCII 标点开头的显示名 v1.5 就对不上，W1d 没报，是按「同类的别处」查到的；只照 W1d 的建议放宽跳过、不改比较的位置，全角的两行也会变成 `false`（相对 v1.5 的新回退），所以两件事一起改。`lead_negative` 是同一组 names 的反面（`@(bot) 看一下`，只差显示名的后半截 ` X`），期望 `false`：M65（跳过之后取得出任何一段就算）下 `bot` 这一段让它变成 `true`，红。`letters_stop` 钉「非 ASCII 的字母、汉字不跳」：`@所有人，smx_b 也看一下` 不冲着成员（端到端探针：问全员，`smxS1Want`、时钟 2），M74（非 ASCII 的字母、汉字也跳）下跳过「所有人，」之后是 `smx_b`，变成 `true`，红；v1.6 显示名在跳之前就比，`@甲` 这类在 `@` 紧后面就命中，C3 里原来杀 M74 的显示名各行不再杀它，只剩这一行与 C3 `ls_cut`。`letters_stop_control` 是只差「所有人」三个字的正对照（`@，smx_b 也看一下`：全角逗号被跳过，`true`），M77、M79（全角逗号不跳）与 M75（ASCII 字母数字也跳，`smx_b` 被一路跳过）下都红，v1.5 的原型上也红。
v1.7 的两行（主代理裁决 `ch1_rulings_v17.md` 第 1 条的同装置对照）：两行的输入都是 C3 `display_case` 的 `@ 坏 x 看一下`，`names` 都是 `smxCfg` 里 TrimSpace 之后非空的显示名（`autoReplyRound` 照这样构造），只差有没有「坏 X」。`display_case_hit` 有它，`true`；`display_case_miss` 去掉它，`false`。这一对证明命中是因为「坏 X」这个名字：同表里的「坏 Y」「坏 Z」「坏 K」都以「坏 」开头，比到第 4 个字节就对不上，不会让它命中；显示名比较若只比开头几个字节，`display_case_miss` 会变成 `true`。`display_case_hit` 被 M85（逐字节比）与 M66（去掉显示名那一支）杀（原型 `ch1_v17` 实测）；`display_case_miss` 是缺席断言，§4 里没有让它变红的变异（v1.7 逐条跑的 58 条里，跑完五个用例的 57 条下它都绿；M86 在 C3 就 panic，没跑到 C14），它的正对照就是只差一个名字的 `display_case_hit`。

### 2.2 `internal/tui/chx_program_test.go`（真 `tea.Program`）

**T1 `TestChxTuiMultilineBareAutoRound`**：两个子测试 `auto` / `manual`，只差 `SetAutoReply`。装置：`g := rbaNewTuiRig(t)`；`g.r.SetConcise(true)`；按子测试 `g.r.SetAutoReply(...)`；`pr := rbbStartProg(t, g)`；`pr.useConv(t, g.convID)`；`pr.submit("第一行\n第二行")`（粘贴两行再回车：一次 TUI 多行提交，第 2 行不以 `/` 开头，过得了 X44）；等到日志里 `DONE` 至少 2 个（20 秒，前提守卫）；`auto` 再等 `FIN` 至少 1 个（5 秒）。取日志里从 `P "> 第一行"` 起（含）的全部条目，丢掉 `TICK`；`P` 记成 `"P " + 正文`（正文里 `（\d+\.\ds）` 换 `（T）`，整行匹配 `^/round 结束  \d+s  回来 2/2$` 的换成 `/round 结束 FIN`），`RS` 记成 `"RS " + Label` 并断言它的 `Speakers` == `[rba_a rba_b]`，其余记 kind。整个序列：
- `auto` ==
  `P > 第一行`、`P > 第二行`、`P 裁判 rba_j 本命令不问，要用 --with-judge 才问。`、`P 本轮问 2 个: rba_a, rba_b`、`RS 发言轮`、`P X（T）: OK`、`MD`、`P X（T）: OK`、`MD`、`RE`、`P 总的来看：没有总结（没配总结成员：在 config.toml 末尾加一张 [summary] 表，写 agent = "<agent-id>"；不想看这一行就加 --no-summary）`、`DONE`、`P /round 结束 FIN`、`FIN`
- `manual` == `P > 第一行`、`P > 第二行`、`DONE`
【判别力】TUI 那条裸文本路径经 `Submit` 的 `bare`，多行正文整段是一条消息；`manual` 是同装置只差开关的缺席对照。`auto` 同时钉住：judge 照 PlanRound 进「本命令不问」；没配总结成员的简洁档照 SM1 打那句提示（§7.1 第 4 条）；收尾行仍写 `/round 结束`（§7.1 第 3 条）。杀 M2。

### 2.3 `internal/app/chx_app_test.go`（真 `app.Run`、真 store、真 config.Load、假 CLI）

配置（冻结，照 `smx_summary_app_test.go` 的体例）：`data_dir` 为临时目录；`[providers.claude] command = os.Args[0]`；三个 agent `c1`（display_name `成员一`）、`c2`（`成员二`）、`sm`（`总结`，不写 role），都是 `provider = "claude"`、`model = "opus"`、`effort = "high"`、`permissions = "read"`、`mode = "discussion"`，c1、c2 的 role 是 `chx`；末尾 `[summary] agent = "sm"`。`isolateAppEnv`、`t.Setenv("COUNCIL_FAKE_CLI", "1")`、`writeTestConfig`。

**A1 `TestChxAppDefaultAutoLine`**（行式分支）：`script = "/new t\n/join c1\n/join c2\n你好\n/quit\n"`；`coxRunApp(t, Options{ConfigPath, In: script, Out, Err, NoAuto: <子测试>})` 返回 nil；尾部 = `smxAppTail(out)`（从 `c2 已加入会话 ` 那一行之后），再把整行 `^总用时 \d+\.\ds$` 换成 `总用时 T`：
- `default`（不设 `NoAuto`）：恰好 8 行；第 2、3 行排序后，8 行依次是 `本轮问 2 个: c1, c2`、`成员一（T）: OK`、`成员二（T）: OK`、`本轮问 1 个: sm`、`总的来看（总结 做总结）：OK`、`总用时 T`、`再见`、`""`。
- `no_auto`（`NoAuto: true`）：尾部 == `["再见", ""]`。
【判别力】app 默认是对话模式、`NoAuto` 关得掉；两子测试只差 `NoAuto`。杀 M6、M17、M19a。

**A2 `TestChxAppDefaultAutoTui`**（TUI 分支，经真 `tui.Run`）：两个子测试 `default` / `no_auto`，只差第 1 段的 `NoAuto`。
- 第 0 段（行式，两子测试相同，不设 `NoAuto`）：`coxRunApp(t, Options{ConfigPath, In: "/new t\n/join c1\n/quit\n", Out: &out0, Err: &err0})` 返回 nil；用 `已建会话 (\S+)，标题: t` 取 convID（前提守卫）。第 0 段没有裸文本。
- 第 1 段（TUI）：`os.Pipe()` 两对；读 goroutine 在 `Run` 之前起（`io.Copy` 进 `coxLockedBuf`）；`Run(ctx, Options{ConfigPath, TUI: true, InFile: inR, OutFile: outW, Err: errOut, NoAuto: <子测试>})`，`In`、`Out` 留 nil。同步点照 `coxRunTui`：等 `ESC[J`；写 `ESC[8;24;100t`；等 flat（`coxFlat`）含 `council空闲`；写 `"/use "+convID+"\r"`；等 flat 含 `当前会话:`；写 `"你好\r"`；**等 flat 里 `>你好` 之后出现 `council空闲`**（15 秒；这一次提交回到空闲，自动那一轮若有就已经跑完）；写 `"/who\r"`；等 flat 含 `AGENTJOINED_PROVIDER`（5 秒）；然后收尾（cancel、等 Run 返回、关管道、等读 goroutine）。取 flat 里从第一个 `>你好` 起的后半段：
  - `default`：含 `本轮问1个:c1`，匹配 `成员一（\d+\.\ds）:OK`，匹配 `/round结束\d+s回来1/1`。
  - `no_auto`：不含 `本轮问`，不匹配 `成员一（\d+\.\ds）:OK`，不含 `/round结束`。
【判别力】`no_auto` 的缺席断言与 `default` 同装置、只差 `NoAuto`；`/who` 是缺席一侧的同步点（命令串行，`/who` 的表头出现时 `你好` 那次提交一定已经结束，所以缺席不是没等够）。等空闲那一步避开 TUI 忙时吞回车（`tui/model.go:357-359`）。杀 M2、M18、M19b。

### 2.4 `cmd/council/chx_flag_test.go`

**F1 `TestChxNoAutoFlagRegistered`**：`run([]string{"-no-auto", "-version"}) == exitOK`；对照 `run([]string{"-chx-no-such-flag", "-version"}) == exitUsage`；把 `os.Stderr` 换成管道跑 `run([]string{"-h"})`（返回 `exitOK`），捕获的用法里含子串（逐字，全部两行）：

```text
  -no-auto
    	直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）
```

（第二行行首是 4 个空格加 1 个 tab；Go 写法 `"  -no-auto\n    \t直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）\n"`。）
【判别力】照 `cox_flag_test.go`：改名（M20）时 Parse 失败；默认改成 true（M20b）时 PrintDefaults 在说明后补 ` (default true)`，这一段不再以「切换）\n」结尾。`NoAuto: *noAuto` 是否真的传进 `app.Options` 只在审查时看（§4）。

---

## 3. 冻结的字面量（逐字节；用户可见的新字面量全部列出）

### 3.1 `/auto` 的四句（依次 `msgAutoOn`、`msgAutoOnNoSummary`、`msgAutoOff`、`msgAutoUsage`，打印时各加 `\n`）

```text
已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁），多个成员回复之后自动总结；/auto off 关掉
已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁）；没配总结成员，多个成员回复之后不做总结；/auto off 关掉
已关闭自动回复：直接打字只记下，不问任何人；要成员说话用 /ask 或 /round；/auto on 打开
用法: /auto 在自动回复与只记下之间切换；/auto on 直接打字就让成员回复；/auto off 直接打字只记下
```

打开时（`/auto on`，或 `/auto` 切到打开）配置里 `[summary] agent` 非空打第 1 句，为空打第 2 句（v1.1 F6；没配总结成员时多人回复之后打的是 SM1 的「总的来看：没有总结（没配总结成员…）」，第 1 句的「自动总结」对它不成立）。

### 3.2 自动一轮不起进程或点名没认全时的四句（依次 `msgAutoNoneMentioned`、`msgAutoNoMembers`、`msgAutoStrayAt`、`msgAutoStrayAtPartly`，打印时各加 `\n`；后两句 v1.1；第 1 句 v1.3 加了出路；v1.5 第 1、3、4 句不再写尖括号占位符）

```text
点名的人都不是当前会话里能回复的成员，这条只记下了、没有问任何人；不点名就会问全部成员；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复
当前会话还没有能回复的成员，这条只记下了；用 /add 加成员之后，直接打字就会有人回复
有 @ 没认出点的是谁，这条只记下了、没有问任何人；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复；TUI 里敲 @ 会列出成员的 id 与显示名
还有 @ 没认出点的是谁，没算进点名；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；TUI 里敲 @ 会列出成员的 id 与显示名
```

第 3 句在「有没认出的 @、一个 mention 都没有」时打，谁都不问；第 4 句在「有 mention、也有没认出的 @」时打在这一轮（或第 1 句）之前，不是「不起进程」的一句。v1.5（主代理裁决 `ch1_rulings_v15.md` 第 3 条）：三句里不再出现尖括号占位符。v1.4 教的是「点名写 @<成员 id>」，用户照字面敲成 `@<smx_b> 看一下`，v1.5 按成员判它是没认出的点名（C3 `angle_template`），又看到同一句提示，形成循环；所以点名的写法改成文字说法「半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首」，出路改成「接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复」（v1.4 的 `/round <成员 id...> --go` 照字面敲不花钱，打「拒绝: 不是当前活跃成员: <smx_a>」，但照样是一次白敲）。第 1 句与第 3 句的出路一段仍逐字相同（分号之后的整段一字不差）。三句逐字节冻结（全角括号、顿号、半角 `@`、`/round`、`--go` 都照打）。
v1.3（主代理裁决第 2 条）：第 1 句末尾加「；这条本来就不是点名的话，接着敲 /round <成员 id...> --go 让成员回复」（v1.5 改成上面不带尖括号的说法），与第 3 句那一段逐字相同（分号之后的整段一字不差）。理由：`@Override`、`@property`、`npm i @types/node`、`@all` 解析得出 token，照旧算「点的不是成员」、谁都不问（判据不改：一个敲错的成员 id 与代码里的 `@override` 分不开，敲错时问全员是花钱方向的误判），而第 1 句原来的「不点名就会问全部成员」对贴进来的代码用不上（`@` 删不掉）。出路在 C13 `none` 实测：成员读到的正是这一条。第 1 句的前半「不点名就会问全部成员」不删：点了已退出的成员或总结成员时（C3 `left`、`summary_mentioned`），它仍是对的指引。
v1.2 按 W1b 改了三处（变更记录 W1b 修订 F2、F3）：
- **左边界写成充分条件**「半角 @，前面空一格或在行首」：完整的规则是 `@` 左边不能紧贴 ASCII 字母、数字或 `_ @ . -`（中文与中文标点紧贴也行），屏幕上只给一条一定对的写法。「行首」含第 2 行起的行首（C3 `line2_mention`）。
- **「不是显示名」**：最常见的没认出写法 `@甲` 是照着成员头上印的显示名敲的；`/who` 第一列（`AGENT`）是成员 id，但 `/who` 不列显示名，本仓库也没有一条命令同时列当前成员的 id 与显示名，所以两句都先写明「不是显示名」。指 id 的命令写 `/who`（列当前会话成员，能 `@` 到的正是它们），不写 `/add`（它列的是不在会话里的配置 agent）。
- **第 3 句的出路**「这条本来就不是点名的话，接着敲 /round <成员 id...> --go 让成员回复」（v1.2 的说法；v1.5 改成不带尖括号的文字说法，意思不变）：贴进来的代码、SQL、PowerShell 里的 `@` 删不掉（§7.1 第 12 条、§7.3 第 5 条）。消息已经记下了，显式给 id 的 `/round` 不看这条消息里有没有 mention，成员读到的正是这一条（C13 实测）。第 4 句不加这段：那种情形已经照问了认出的成员。v1.3 收窄之后，`@{`、`@(`、`@@` 这类代码写法回到问全员、不再打第 3 句；仍会打第 3 句的「不是点名」主要是 `@所有人`、`价格 @ 5 元`、`https://medium.com/@张三/…`、PowerShell here-string 收尾的 `"@`（§7.3 第 5 条的「仍谁都不问」一组），出路照旧有用（C13 `stray` 的输入就是 `价格 @ 5 元`）。v1.4 按 W1c 补全这份清单（原型 `ch1_fix3` 探针）：最常见的一种是 numpy / torch 的矩阵乘 `A @ B`（`@` 两边是空格），另有 Makefile 与 bash 的 `$@`、Mathematica 的 `f @ x` 与 `f /@ list`、`明天 @ 3pm 开会`；Python 装饰器与 PowerShell splatting 打的是第 1 句（解析得出 token），同样要多敲一条 `/round`（§7.3 第 6 条）。v1.5 按成员判之后上面这份清单几乎全部回到问全员、不再打第 3 句（`@所有人`、`价格 @ 5 元`、`A @ B`、`$@`、here-string 收尾、`https://medium.com/@张三/…` 都实测问全员，§7.3 第 5 条）；仍会打第 3 句的「不是点名」只剩全角 `＠`（`看一下 ＠所有人`，C13 `stray` 的新输入）与碰巧以成员 id 或显示名开头的写法（§7.3 第 9 条），出路照旧有用。
- 末半句「TUI 里敲 @ 会列出成员的 id 与显示名」是 AT1 的功能（AT1 契约：候选每行 id 与显示名，上下键选、Tab 或回车补全，不做鼠标点选；v1.1 写的「弹出可点的成员」与它不符，已改），仍放在两句末尾，落地顺序见 §6 第 0 步。
第四种不起进程的情形（rn 为 nil）沿用既有 `msgNoRunner`：`provider 调用未就绪，本命令不可用`。

### 3.3 显式 `/add` 总结成员的拒绝（`%s` 是那一段原文，末尾自带 `\n`）

```text
%s 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结
```

### 3.4 `/help` 里的三行 summary（`/round` 那一行不变）

```text
auto:    切换直接打字要不要让成员回复: /auto 切换；/auto on 自动回复（点名了谁就只问谁，没点名问全部成员）；/auto off 只记下
say:     只记下、不让任何人回复: /say <文本>；/say /foo 发字面量 /foo；/say 不带参数进多行输入（/end 发送时与直接打字相同）
session: 列出当前会话每个成员的 provider 会话；/session rebuild <agent-id> 先结清这个成员留在库里的陈旧在飞记录，再丢弃并重建
```

（行首 `auto:` / `say:` / `session:` 与其后的空格是本表的标签，不是字面量的一部分。）

### 3.5 启动参数

`-no-auto`，布尔，默认 false，说明：

```text
直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）
```

### 3.6 自动模式下不再打印的既有字面量

裸文本且自动回复打开时，详细档不打 `点名了 %s（用 /ask %s 让他说话）`（`/say`、`/to`、`/ask <id> 文本`、自动回复关着时照打）。

### 3.7 沿用、不新增的屏幕

自动一轮的屏幕全部来自 `cmdRound` 的既有字面量：`本轮问 N 个: …`、成员头、失败行、跳过行、`裁判 X 本命令不问，要用 --with-judge 才问。`、`这一轮没有人可问。`、总结段、`总用时 …`、详细档的预算表 / 扇出 / 结算、TUI 看板与收尾行 `/round 结束 …`。**没有**「自动问 N 个」之类的新行。唯一的例外是 §3.2 第 4 句（v1.1，冻结要点 (f)）。

---

## 4. 集成员的变异清单

运行方式：锚点在落地后的真文件上恰好出现一次（用下表「替换前」整段做锚点）；从内存字节副本还原，还原后核对 SHA256；每条先 `go build ./...`，再只跑点名的杀手（`go test -count=1 -run '<正则>' <包>`，子测试用 `^父$/^子$`；v1.2 的 C13 没有子测试，v1.3 起有 `stray`、`none` 两个，杀手栏写成 C13 加子测试名，例如 C13 `stray` 跑 `-run '^TestChxAutoStrayThenRound$/^stray$'`）。原型上每条都实跑过，结果与本表一致（v1.1 在 `ch1_v11` 上用脚本从本表逐行取出「替换前 / 替换后」重跑了全部条目，每个点名的杀手——表里每个子测试逐个单跑——都实测变红）。下表「替换前 / 替换后」里的 `\n` 是换行、`\t` 是 tab、`\|` 是 `|`；**例外**：Go 字符串 `"%s\n"` 里的 `\n` 是源码里的两个字符（反斜杠加 n），不是换行（M12、M13、M13b、M32、M44-M48 的 `r.printf("%s\n", …)`）；v1.5 起 Go 字符字面量 `'\n'`、`'\r'` 里的同样是源码里的两个字符（M69、M69b）。「预期被谁杀」一栏列出的每一个都必须红；v1.1 新增的是 M31-M49b，v1.2 新增的是 M50-M55（W1b 的 X1-X6），并给 M38、M39 补了杀手；v1.3 新增的是 M56-M64（裁决书第 1 条点名的六条加三条空白字符），v1.2 的 M40、M41、M42 在 v1.3 的判据下等价、移进下面的等价表（E4、E5、E6），M53 换了锚点（判据那一行变了，语义不变：第一个合法起头的 `@` 定胜负），M36、M38、M50 的杀手栏改了，M31 补了 C13 `none`。v1.2 在原型 `ch1_fix` 上用脚本从本表逐行取出重跑了杀手落在 C3 / C11 / C13 的 32 条（M7b、M10、M11、M12、M13、M13b、M26、M28、M30、M31、M32、M33、M34、M36-M48、M50-M55；M10、M26、M31 只核了它们在 C3 / C13 的杀手，C2 `second` / `summary_member_from_start`、C1 `auto`、C5 `auto` 没跑），点名杀手里的 C3 / C11 子测试与 C13 逐个单跑都红，锚点各恰好一次，还原后 SHA256 相同；另核了 M50-M55 各自让 v1.1 的 C3 28 行与 C11 8 行全部存活。其余 32 条 v1.2 没重跑（它们的杀手不在 C3 / C11 / C13，期望与走到的代码 v1.2 都没改）。v1.3 在原型 `ch1_v13` 上用脚本从本表逐行取出，重跑了杀手落在 C3 / C5 / C11 / C13 的 42 条（M5、M7b、M8-M13b、M26-M28、M30-M34、M36-M39、M43-M48、M50-M64）：锚点各恰好一次，每条先 `go test -c` 编过，把 C3、C5、C11、C13 全部用例跑一遍记下红的集合，再把点名杀手里落在这四个用例的 112 个子测试逐个单跑，全部变红；还原后 SHA256 相同。M5、M9、M10、M26、M31 的杀手栏里落在 C1、C2、C4、C6 与 `TestFuxAppVerboseMentionHint` 的那些没跑（原型没写这些用例）。其余 28 条（M1-M4、M6、M7、M14-M25、M29、M35-M35d、M49、M49b）v1.3 没重跑：它们的杀手不在这四个用例，走到的代码与期望 v1.3 都没改（v1.3 只改 `hasStrayAt` 与第 1 句，这些用例的输入里没有 `@`，或只有认得出的 `@smx_a`）。v1.4（W1c 修订）C3 加了 `angle_template`、`partly_ascii` 两行，在原型 `ch1_fix3` 上用脚本从本表逐行取出，把同样这 42 条重跑一遍：锚点各恰好一次，`go test -c` 编过，C3、C5、C11、C13 全部用例跑一遍记下红的集合，点名杀手里落在这四个用例的 116 个子测试逐个单跑全部变红，还原后 SHA256 相同；M50-M64 各自红的集合与杀手栏恰好相同；两行新夹具在 42 条里的红：M38、M56 下两行都红（这两条的杀手栏已补），M26 下 `angle_template` 红、M30 下 `partly_ascii` 红（M26、M30 的杀手栏本来就不说恰好，不补），其余 38 条下两行都绿。v1.4 没有新增变异：两行新夹具钉的是现状，对应的方向题（§7.2 第 9 条）还没定。v1.5（主代理裁决 `ch1_rulings_v15.md`，判据改成按成员判）：M56-M64 整行删去（它们变异的是 v1.3 判据的各支，判据删掉就没有意义了，编号空着不复用）；M36、M38 换锚点（调用改成 `hasStrayAt(text, known, names)`），M53 换锚点（「认出的点名之后不再往后看」）；M36、M38、M50-M55 的杀手栏按新判据重审，改成实测红的集合；新增 M53b、M65-M76 与 M69b 十四条。在原型 `ch1_v15` 上用脚本从本表逐行取出，重跑了杀手落在 C3 / C5 / C11 / C13 的 47 条（M5、M7b、M8-M13b、M26-M28、M30-M34、M36-M39、M43-M48、M50-M55、M53b、M65-M76、M69b）：锚点各恰好一次，`go test -c` 编过，把这四个用例全部跑一遍记下红的集合，再把红的集合与点名杀手里落在这四个用例的子测试逐个单跑，点名的 214 个全部变红，还原后 SHA256 相同；M36、M38、M50-M55、M53b、M65-M76 各自红的集合与杀手栏恰好相同；另把全表 78 行（75 条 M、3 条 E）的锚点在 v1.5 原型上逐条核过，各恰好一次。其余 28 条（M1-M4、M6、M7、M14-M25、M29、M35-M35d、M49、M49b）v1.5 没重跑：杀手不在这四个用例，原型没写那些用例；v1.5 只改 `autoreply.go`，这 28 条里落在 `autoreply.go` 的九条（M14、M15、M21、M21b、M35、M35b、M35d、M49、M49b）锚点在新文件里照旧恰好一次；它们的杀手用例的输入里没有 `@`，或只有认得出的 `@smx_a`。v1.6（W1d 修订：跳过的字符放宽到非 ASCII 的空白、标点、符号、组合记号、格式字符，显示名在跳过途中每个位置都比）：跳过那一段挪进新函数 `atSkip`，M53b、M66、M69、M69b、M74、M75 换了锚点（M53b、M66、M69、M69b、M75 语义不变；M74 原来是「非 ASCII 也跳」，现在是「非 ASCII 的字母、汉字、数字、换行也跳」）；新增 M77-M84 八条。在原型 `ch1_fix5` 上用脚本从本表逐行取出，重跑了杀手落在 C3 / C5 / C11 / C13 / C14 的 55 条（v1.5 的 47 条加 M77-M84）：锚点各恰好一次，`go test -c` 编过，把这五个用例全部跑一遍记下红的集合，再把点名杀手里落在这五个用例的子测试逐个单跑，全部变红，还原后 SHA256 相同。红的集合比杀手栏多出来的，在说「恰好相同」的那些行里都补进了杀手栏（M36、M38、M50、M53、M65、M66、M74、M75、M76、M77、M79），M43、M44 补了 `partly_fullwidth`；补过的这 13 条又从本表取出重跑一遍，新点名的杀手逐个单跑都红。M36、M38、M50-M55、M53b、M65-M84 各自红的集合与杀手栏恰好相同。杀手栏不说恰好的几条另外还红的（实测，杀手栏不补）：M9 另红 C5 `auto`、`auto_say`、`auto_to`；M10 另红 C3 `summary_and_member`、C11 `summary_mentioned_concise`；M12 另红 C3 `partly_none`、C5 `auto_not_member`、C11 `summary_mentioned_concise`、C13 `none`；M13 另红 C3 `partly_none`、C5 `auto_not_member`、C13 `none`；M26、M30、M31 另红很多行（M26 下没有 mention 的各行都走进了「有 mention」那一支）；M37 另红 `mid_space_fullwidth`、`line2_fullwidth`；M43 另红 `stray_after_mention`、C11 `partly_concise`；M44 另红 `stray_after_mention`。另把全表 86 行（83 条 M、3 条 E）的锚点在 v1.6 原型上逐条核过，各恰好一次；E1、E3 下这五个用例全绿。其余 28 条 v1.6 没重跑，理由同 v1.5（v1.6 只改 `hasStrayAt`、加 `atSkip`；它们的杀手用例的输入里没有 `@`，或只有认得出的 `@smx_a`）。v1.7（主代理裁决 `ch1_rulings_v17.md`：显示名不分大小写）：M66、M84 换了锚点（显示名比较那一行变了；M66 语义不变；M84 仍是「显示名只在跳完之后比一次」，替换前后两段里的比较都换成新写法）；新增 M85-M87 三条；M36、M66 的杀手栏补 `display_case`，M66 另补 C14 `display_case_hit`；M38、M76 的杀手栏拿掉 `display_case`（它翻成 `STRAY` 之后，这两条下照样是 `STRAY`）。在原型 `ch1_v17` 上用脚本从本表逐行取出，重跑了杀手落在 C3 / C5 / C11 / C13 / C14 的 58 条（v1.6 的 55 条加 M85-M87）与 E1、E3：锚点各恰好一次，`go test -c` 编过，把这五个用例全部跑一遍记下红的集合，再把点名杀手里落在这五个用例的子测试逐个单跑，全部变红（第一遍 260 个单跑，除了 M38、M76 下的 `display_case` 两个（就是上面拿掉的）都红；改完杀手栏之后 M36、M38、M66、M76、M86 又重跑一遍，115 个单跑全红），还原后 SHA256 都相同；E1、E3 下五个用例的 100 个子测试全绿。M36、M38、M50-M55、M53b、M65-M85、M87 各自红的集合与杀手栏恰好相同。M86 是 panic：整个测试二进制在第一处 panic 就退出（全量那一遍停在 `mid_space_display` 或 `stray_eot`，看 map 的遍历顺序），不说恰好。杀手栏不说恰好的几条另外还红的与 v1.6 写的相同（M9、M10、M12、M13、M26、M30、M31、M37、M43、M44），另有 M33 另红 C11 `no_members_concise`、`partly_concise`、`stray_concise`、`summary_mentioned_concise`（v1.6 没记；走到的代码 v1.7 没改）。§4 全表 89 行（86 条 M、3 条 E）的锚点在 v1.7 原型上逐条核过，各恰好一次。其余 28 条 v1.7 没重跑，理由同 v1.6（v1.7 只改显示名比较那一行；它们的杀手用例的输入里没有 `@`，或只有认得出的 `@smx_a`）。

| # | 文件 | 替换前 | 替换后 | 预期被谁杀 |
|---|---|---|---|---|
| M1 | commands.go（dispatch） | `rest: text, bare: true})` | `rest: text, bare: false})` | C6 `single_auto` |
| M2 | seam.go | `rest: text, bare: true})` | `rest: text, bare: false})` | C1 `auto`；T1 `auto`；A2 `default` |
| M3 | paste.go | `bare:   true,` | `bare:   false,` | C6 `paste_auto` |
| M4 | commands.go | `return sayText(ctx, r, text, true)` | `return sayText(ctx, r, text, false)` | C6 `draft_auto` |
| M5 | commands.go | `return sayText(ctx, r, text, args.bare)` | `return sayText(ctx, r, text, true)` | C4 `say`；C6 `say_auto`；C5 `auto_say` |
| M6 | commands.go | `auto := bare && r.autoReply` | `auto := bare` | C4 `bare_manual`；C6 `single_manual`；A1 `no_auto` |
| M7 | commands.go | `if err != nil \|\| !posted \|\| !auto {` | `if err != nil \|\| !posted && false \|\| !auto {` | C7 `canceled` |
| M7b | commands.go | `if err != nil \|\| !posted \|\| !auto {` | `if !posted \|\| !auto {` | C11 `bare` |
| M8 | commands.go | `if !r.concise && !auto {` | `if !r.concise {` | C5 `auto` |
| M9 | commands.go | `if !r.concise && !auto {` | `if !r.concise && auto {` | C5 `manual`；`TestFuxAppVerboseMentionHint/verbose` |
| M10 | autoreply.go | `if p.AgentID == sumID {` | `if false && p.AgentID == sumID {` | C2 `second`、`summary_member_from_start`；C3 `summary_plus_one`、`only_summary`、`summary_mentioned`、`only_summary_mentioned` |
| M11 | autoreply.go | `r.svc.Participants(ctx, r.cur.ID, false)` | `r.svc.Participants(ctx, r.cur.ID, true)` | C3 `left`、`left_all`、`left_mixed` |
| M12 | autoreply.go | `\t\t\treturn r.printf("%s\n", msgAutoNoneMentioned)` | `\t\t\tids = all` | C3 `not_member`、`left`、`summary_mentioned`、`only_summary_mentioned` |
| M13 | autoreply.go | `return r.printf("%s\n", msgAutoNoneMentioned)` | `return r.printf("%s\n", msgAutoNoMembers)` | 同 M12 |
| M13b | autoreply.go | `\t\treturn r.printf("%s\n", msgAutoNoMembers)` | `\t\treturn r.printf("%s\n", msgAutoNoneMentioned)` | C3 `no_members`、`only_summary` |
| M14 | autoreply.go | `fields := append(append([]string(nil), ids...), "--go")` | `fields := append([]string(nil), ids...)` | C1 `auto` |
| M15 | autoreply.go | `return r.runCommand(ctx, roundCmd, cmdArgs{` | `return cmdRound(ctx, r, cmdArgs{` | C8 `auto`（panic） |
| M16 | commands.go（roundCmd） | `\tneedsRunner: true,\n\trun:         cmdRound,` | `\tneedsRunner: false,\n\trun:         cmdRound,` | C8 `auto`（panic） |
| M17 | app.go | `\t\tr.SetConcise(!opts.Verbose)\n\t\tr.SetAutoReply(!opts.NoAuto)\n\t\terr = r.Run(ctx)` | `\t\tr.SetConcise(!opts.Verbose)\n\t\terr = r.Run(ctx)` | A1 `default` |
| M18 | app.go | `\t\tr.SetAutoReply(!opts.NoAuto)\n\t\terr = tui.Run(` | `\t\terr = tui.Run(` | A2 `default` |
| M19a | app.go | `\t\tr.SetAutoReply(!opts.NoAuto)\n\t\terr = r.Run(ctx)` | `\t\tr.SetAutoReply(opts.NoAuto)\n\t\terr = r.Run(ctx)` | A1 两个子测试 |
| M19b | app.go | `\t\tr.SetAutoReply(!opts.NoAuto)\n\t\terr = tui.Run(` | `\t\tr.SetAutoReply(true)\n\t\terr = tui.Run(` | A2 `no_auto` |
| M20 | main.go | `fs.Bool("no-auto", false,` | `fs.Bool("no-autox", false,` | F1 |
| M20b | main.go | `fs.Bool("no-auto", false,` | `fs.Bool("no-auto", true,` | F1 |
| M21 | autoreply.go | `r.autoReply = !r.autoReply` | `r.autoReply = true` | C9 `steps` |
| M21b | autoreply.go | `args.fields[0] == "off":\n\t\tr.autoReply = false` | `args.fields[0] == "off":\n\t\tr.autoReply = true` | C9 `steps` |
| M22 | commands.go | `if !active[id] && id != sumID {` | `if !active[id] && (id != sumID \|\| true) {` | C10 `list`、`number` |
| M23 | commands.go | `sumID != "" && seg == sumID && !(member[seg] && !active[seg])` | `false && sumID != "" && seg == sumID && !(member[seg] && !active[seg])` | C10 `explicit`、`active` |
| M24 | commands.go | `seg == sumID && !(member[seg] && !active[seg])` | `seg == sumID` | C10 `left_rejoin` |
| M25 | commands.go | `seg == sumID && !(member[seg] && !active[seg])` | `seg == sumID && !member[seg]` | C10 `active` |
| M26 | autoreply.go | `if mentions := ParseMentions(text); len(mentions) > 0 {` | `if mentions := ParseMentions(text); len(mentions) >= 0 {` | C1 `auto`；C3 `all` |
| M27 | commands.go | `\t_, err := postUser(ctx, r, text, false, recipients)` | `\t_, err := postUser(ctx, r, text, true, recipients)` | C5 `auto_to` |
| M28 | commands.go | `\t_, err := postUser(ctx, r, text, false, recipients)\n\treturn err` | `\t_, err := postUser(ctx, r, text, false, recipients)\n\t_ = err\n\treturn nil` | C11 `to` |
| M29 | commands.go | `\tif len(tokens) == 0 {\n\t\treturn true, nil\n\t}` | `\tif len(tokens) == 0 {\n\t\treturn false, nil\n\t}` | C1 `auto` |
| M30 | commands.go | `\t\t}\n\t}\n\treturn true, nil\n}` | `\t\t}\n\t}\n\treturn false, nil\n}` | C3 `subset`、`mixed` |
| M31 | commands.go（sayText） | `\tauto := bare && r.autoReply\n\tposted, err := postUser(ctx, r, text, auto, nil)\n\tif err != nil \|\| !posted \|\| !auto {\n\t\treturn err\n\t}\n\treturn autoReplyRound(ctx, r, text)` | `\tauto := bare && r.autoReply\n\tif auto {\n\t\tif err := autoReplyRound(ctx, r, text); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\tposted, err := postUser(ctx, r, text, auto, nil)\n\tif err != nil \|\| !posted \|\| !auto {\n\t\treturn err\n\t}\n\treturn nil` | C5 `auto`；C13 `stray`（v1.2 补）、`none`（v1.3 补） |
| M32 | autoreply.go | `\t\tfor _, m := range mentions {` | `\t\tfor _, m := range mentions {\n\t\t\tif m == sumID {\n\t\t\t\treturn r.printf("%s\n", msgAutoNoneMentioned)\n\t\t\t}` | C3 `summary_and_member` |
| M33 | commands.go（sayText） | `\treturn autoReplyRound(ctx, r, text)\n}` | `\t_ = autoReplyRound(ctx, r, text)\n\treturn nil\n}` | C11 `bare_concise` |
| M34 | autoreply.go | `\treturn r.runCommand(ctx, roundCmd, cmdArgs{fields: fields, rest: strings.Join(fields, " ")})` | `\t_ = r.runCommand(ctx, roundCmd, cmdArgs{fields: fields, rest: strings.Join(fields, " ")})\n\treturn nil` | C11 `bare_concise` |
| M35 | autoreply.go | `\tif err != nil {\n\t\treturn r.reportErr("round", err)\n\t}` | `\tif err != nil {\n\t\treturn nil\n\t}` | C12 `plain`、`mention` |
| M35b | autoreply.go | `\tif err != nil {\n\t\treturn r.reportErr("round", err)\n\t}` | `\tif err != nil {\n\t\tparts = nil\n\t}` | C12 `plain`、`mention` |
| M35c | commands.go（postUser） | `\t\treturn true, nil\n\t}\n\tknown := make(` | `\t\treturn false, nil\n\t}\n\tknown := make(` | C12 `mention` |
| M35d | autoreply.go | `\t\treturn r.reportErr("round", err)` | `\t\t_ = r.reportErr("round", err)\n\t\treturn nil` | C12 `write_fail` |
| M36 | autoreply.go | `\tstray := hasStrayAt(text, known, names)` | `\tstray := false && hasStrayAt(text, known, names)` | C3 `stray_display`、`stray_fullwidth`、`stray_space`、`stray_bad_id`、`stray_no_members`、`partly`、`partly_none`、`mid_space_display`、`mid_space_fullwidth`、`mid_cjk_display`、`line2_display`、`line2_fullwidth`、`stray_after_mention`、`double_at`、`stray_tab`、`stray_vtab`、`stray_formfeed`、`angle_template`、`partly_ascii`、`paren_id`、`backtick_id`、`bold_id`、`space_display`、`config_only`、`session_only`、`space_dot`、`space_upper`、`fullwidth_everyone`、`skip_then_display`、`fw_paren_id`、`ideo_space_id`、`zwsp_id`、`fw_angle_id`、`emoji_vs_id`、`ideo_space_display`、`partly_fullwidth`、`display_case`（v1.7 补）；C11 `stray_concise`、`partly_concise`；C13 `stray` |
| M37 | autoreply.go | `full := strings.HasPrefix(text[i:], string(rune(0xFF20)))` | `full := false && strings.HasPrefix(text[i:], string(rune(0xFF20)))` | C3 `stray_fullwidth`、`fullwidth_everyone`（v1.5 补）；C13 `stray`（v1.5 补） |
| M38 | autoreply.go | `\tstray := hasStrayAt(text, known, names)` | `\tstray := strings.Contains(text, "@")` | C3 `subset`、`upper`、`order`、`not_member`、`left`、`left_mixed`、`summary_mentioned`、`mixed`、`only_summary_mentioned`、`judge_only`、`summary_and_member`、`stray_fullwidth`、`email`、`trailing_dot`、`glued_at`、`mid_space_fullwidth`、`line2_fullwidth`、`line2_mention`、`glued_left_cjk_tail`、`ps_hashtable`、`sql_identity`、`at_everyone_cn`、`backtick_at`、`ps_array`、`stray_eot`、`stray_newline`、`stray_crlf`、`price_at`、`numpy_matmul`、`bash_dollar_at`、`ps_herestring_close`、`newline_cut`、`cr_cut`、`fullwidth_everyone`、`ls_cut`；C5 `auto`、`auto_not_member`；C13 `stray`、`none` |
| M39 | autoreply.go | `if i > 0 && mentionLeftForbidden(text[i-1]) {` | `if false && i > 0 && mentionLeftForbidden(text[i-1]) {` | C3 `email_fullwidth`、`glued_left_cjk_tail`、`glued_fullwidth`（后两个 v1.2 补） |
| M43 | autoreply.go | `\t\tif stray {\n\t\t\tif err := r.printf(` | `\t\tif false && stray {\n\t\t\tif err := r.printf(` | C3 `partly`、`partly_none`、`partly_ascii`（v1.5 补）、`partly_fullwidth`（v1.6 补） |
| M44 | autoreply.go | `\t\t\tif err := r.printf("%s\n", msgAutoStrayAtPartly); err != nil {\n\t\t\t\treturn err\n\t\t\t}` | `\t\t\treturn r.printf("%s\n", msgAutoStrayAt)` | C3 `partly`、`partly_none`、`partly_ascii`（v1.5 补）、`partly_fullwidth`（v1.6 补） |
| M45 | autoreply.go | `\t\t\tif err := r.printf("%s\n", msgAutoStrayAtPartly); err != nil {\n\t\t\t\treturn err\n\t\t\t}` | `\t\t\t_ = r.printf("%s\n", msgAutoStrayAtPartly)` | C11 `partly_concise` |
| M46 | autoreply.go | `\t\treturn r.printf("%s\n", msgAutoStrayAt)` | `\t\t_ = r.printf("%s\n", msgAutoStrayAt)\n\t\treturn nil` | C11 `stray_concise` |
| M47 | autoreply.go | `\t\treturn r.printf("%s\n", msgAutoNoMembers)` | `\t\t_ = r.printf("%s\n", msgAutoNoMembers)\n\t\treturn nil` | C11 `no_members_concise` |
| M48 | autoreply.go | `\t\t\treturn r.printf("%s\n", msgAutoNoneMentioned)` | `\t\t\t_ = r.printf("%s\n", msgAutoNoneMentioned)\n\t\t\treturn nil` | C11 `summary_mentioned_concise` |
| M49 | autoreply.go | `\t\tif r.cfg.Summary.Agent == "" {` | `\t\tif false && r.cfg.Summary.Agent == "" {` | C9 `steps`、`no_summary` |
| M49b | autoreply.go | `\t\tif r.cfg.Summary.Agent == "" {` | `\t\tif true \|\| r.cfg.Summary.Agent == "" {` | C9 `summary` |
| M50 | autoreply.go | `for i := 0; i < len(text); i++ {` | `for i := 0; i < len(text) && i == 0; i++ {` | C3 `mid_space_display`、`mid_space_fullwidth`、`mid_cjk_display`、`line2_display`、`line2_fullwidth`、`stray_after_mention`、`partly_ascii`、`space_dot`、`skip_then_display`、`partly_fullwidth`；C13 `stray` |
| M51 | autoreply.go | `if i > 0 && mentionLeftForbidden(text[i-1]) {` | `if full && i > 0 && mentionLeftForbidden(text[i-1]) {` | C3 `glued_left_cjk_tail` |
| M52 | autoreply.go | `if i > 0 && mentionLeftForbidden(text[i-1]) {` | `if i > 0 && (mentionLeftForbidden(text[i-1]) \|\| text[i-1] >= 0x80) {` | C3 `mid_cjk_display` |
| M53 | autoreply.go | `\t\tif j < len(text) && asciiAlnum(text[j]) {\n\t\t\tcontinue\n\t\t}` | `\t\tif j < len(text) && asciiAlnum(text[j]) {\n\t\t\treturn false\n\t\t}` | C3 `stray_after_mention`、`partly_ascii`、`partly_fullwidth` |
| M54 | autoreply.go | `full := strings.HasPrefix(text[i:], string(rune(0xFF20)))` | `full := i == 0 && strings.HasPrefix(text[i:], string(rune(0xFF20)))` | C3 `mid_space_fullwidth`、`line2_fullwidth`；C13 `stray` |
| M55 | autoreply.go | `if i > 0 && mentionLeftForbidden(text[i-1]) {` | `if i > 0 && (mentionLeftForbidden(text[i-1]) \|\| text[i-1] == 0x0a) {` | C3 `line2_display`、`line2_fullwidth` |
| M53b | autoreply.go | `\t\t\treturn true\n\t\t}\n\t}\n\treturn false\n}` | `\t\t\treturn true\n\t\t}\n\t\treturn false\n\t}\n\treturn false\n}` | C3 `skip_then_display` |
| M65 | autoreply.go | `\t\tif known[strings.ToLower(strings.TrimRight(text[j:k], "."))] {` | `\t\tif k > j \|\| known[strings.ToLower(strings.TrimRight(text[j:k], "."))] {` | C3 `ps_hashtable`、`sql_identity`、`price_at`、`numpy_matmul`；C14 `lead_negative` |
| M66 | autoreply.go | `\t\t\t\tif len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {` | `\t\t\t\tif false && len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {` | C3 `stray_display`、`stray_no_members`、`partly`、`partly_none`、`mid_space_display`、`mid_cjk_display`、`line2_display`、`stray_after_mention`、`space_display`、`skip_then_display`、`ideo_space_display`、`display_case`（v1.7 补）；C11 `stray_concise`、`partly_concise`；C14 `lead_fw_bracket`、`lead_fw_bracket_space`、`lead_ascii_paren`、`lead_ascii_paren_space`、`display_case_hit`（v1.7 补） |
| M67 | autoreply.go | `\t\tknown[id] = true` | `\t\t_ = id` | C3 `config_only` |
| M68 | autoreply.go | `\t\tknown[p.AgentID] = true\n\t\tif p.AgentID == sumID {` | `\t\tif p.AgentID == sumID {` | C3 `session_only` |
| M69 | autoreply.go | `\|\| s[0] == '\n' \|\| s[0] == '\r' {` | `\|\| s[0] == '\r' {` | C3 `newline_cut` |
| M69b | autoreply.go | `\|\| s[0] == '\n' \|\| s[0] == '\r' {` | `\|\| s[0] == '\n' {` | C3 `cr_cut` |
| M70 | autoreply.go | `\t\tif full {\n\t\t\treturn true\n\t\t}\n\t\tj := i + 1\n\t\tif j < len(text) && asciiAlnum(text[j]) {` | `\t\tj := i + 1\n\t\tif full {\n\t\t\tj = i + 3\n\t\t} else if j < len(text) && asciiAlnum(text[j]) {` | C3 `fullwidth_everyone`；C13 `stray` |
| M71 | autoreply.go | `known[strings.ToLower(strings.TrimRight(text[j:k], "."))]` | `known[strings.ToLower(text[j:k])]` | C3 `space_dot` |
| M72 | autoreply.go | `known[strings.ToLower(strings.TrimRight(text[j:k], "."))]` | `known[strings.TrimRight(text[j:k], ".")]` | C3 `space_upper` |
| M73 | autoreply.go | `\t\tif j < len(text) && asciiAlnum(text[j]) {` | `\t\tif false && j < len(text) && asciiAlnum(text[j]) {` | C3 `subset`、`upper`、`order`、`left`、`left_mixed`、`summary_mentioned`、`mixed`、`only_summary_mentioned`、`judge_only`、`summary_and_member`、`trailing_dot`、`line2_mention`；C5 `auto` |
| M74 | autoreply.go | `\tif unicode.In(r, unicode.Zs, unicode.P, unicode.S, unicode.M, unicode.Cf) {` | `\tif true \|\| unicode.In(r, unicode.Zs, unicode.P, unicode.S, unicode.M, unicode.Cf) {` | C3 `ls_cut`；C14 `letters_stop` |
| M75 | autoreply.go | `if s == "" \|\| asciiAlnum(s[0]) \|\| s[0] == '\n'` | `if s == "" \|\| s[0] == '\n'` | C3 `stray_space`、`stray_bad_id`、`double_at`、`stray_tab`、`stray_vtab`、`stray_formfeed`、`angle_template`、`partly_ascii`、`paren_id`、`backtick_id`、`bold_id`、`config_only`、`session_only`、`space_dot`、`space_upper`、`fw_paren_id`、`ideo_space_id`、`zwsp_id`、`fw_angle_id`、`emoji_vs_id`、`partly_fullwidth`；C14 `letters_stop_control` |
| M76 | autoreply.go | `\t\tif name := strings.TrimSpace(a.DisplayName); name != "" {` | `\t\tif name := strings.TrimSpace(a.DisplayName); true {` | C3 `ps_hashtable`、`sql_identity`、`at_everyone_cn`、`backtick_at`、`ps_array`、`stray_eot`、`stray_newline`、`stray_crlf`、`price_at`、`numpy_matmul`、`bash_dollar_at`、`ps_herestring_close`、`newline_cut`、`cr_cut`、`ls_cut` |
| M77 | autoreply.go | `\tif unicode.In(r, unicode.Zs, unicode.P, unicode.S, unicode.M, unicode.Cf) {` | `\tif false && unicode.In(r, unicode.Zs, unicode.P, unicode.S, unicode.M, unicode.Cf) {` | C3 `fw_paren_id`、`ideo_space_id`、`zwsp_id`、`fw_angle_id`、`emoji_vs_id`、`ideo_space_display`、`partly_fullwidth`；C14 `letters_stop_control` |
| M78 | autoreply.go | `unicode.In(r, unicode.Zs, unicode.P,` | `unicode.In(r, unicode.P,` | C3 `ideo_space_id`、`ideo_space_display` |
| M79 | autoreply.go | `unicode.Zs, unicode.P, unicode.S,` | `unicode.Zs, unicode.S,` | C3 `fw_paren_id`、`partly_fullwidth`；C14 `letters_stop_control` |
| M80 | autoreply.go | `unicode.P, unicode.S, unicode.M,` | `unicode.P, unicode.M,` | C3 `fw_angle_id`、`emoji_vs_id` |
| M81 | autoreply.go | `unicode.S, unicode.M, unicode.Cf)` | `unicode.S, unicode.Cf)` | C3 `emoji_vs_id` |
| M82 | autoreply.go | `unicode.M, unicode.Cf)` | `unicode.M)` | C3 `zwsp_id` |
| M83 | autoreply.go | `unicode.In(r, unicode.Zs,` | `unicode.In(r, unicode.Z,` | C3 `ls_cut` |
| M84 | autoreply.go | `\t\tfor {\n\t\t\tfor _, name := range names {\n\t\t\t\tif len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {\n\t\t\t\t\treturn true\n\t\t\t\t}\n\t\t\t}\n\t\t\tn := atSkip(text[j:])\n\t\t\tif n == 0 {\n\t\t\t\tbreak\n\t\t\t}\n\t\t\tj += n\n\t\t}` | `\t\tfor atSkip(text[j:]) > 0 {\n\t\t\tj += atSkip(text[j:])\n\t\t}\n\t\tfor _, name := range names {\n\t\t\tif len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {\n\t\t\t\treturn true\n\t\t\t}\n\t\t}` | C14 `lead_fw_bracket`、`lead_fw_bracket_space`、`lead_ascii_paren`、`lead_ascii_paren_space` |
| M85 | autoreply.go | `\t\t\t\tif len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {` | `\t\t\t\tif strings.HasPrefix(text[j:], name) {` | C3 `display_case`；C14 `display_case_hit` |
| M86 | autoreply.go | `if len(text)-j >= len(name) && strings.EqualFold(` | `if strings.EqualFold(` | C3 `stray_eot`（panic；`mid_space_display` 的 panic 取决于 map 遍历顺序，约 6/7，不作杀手） |
| M87 | autoreply.go | `if len(text)-j >= len(name) &&` | `if len(text)-j > len(name) &&` | C3 `mid_space_display` |

（M7 写成 `!posted && false` 而不是直接删掉 `!posted`：删掉时 `posted` 未使用，编译不过。M22 同理保留 `sumID` 的使用。表里的 `\|` 是 markdown 转义，真文本是 `|`。M31 = W1 的 X1（先问一轮再落库）；M32 = X3（点名里有总结成员就整句不问）；M33 = X15、M34 = X16（包装层吞错误）；M35 = X17（读成员表失败时静默）、M35b = X17b（读成员表失败时当成没有成员）；M35c = V3 的 PU1（postUser 那一支返回 false）；M35d 是 V2「同类的别处」里 `reportErr("round")` 那一处。M36-M44 是 F0 的判据与先后（其中 M40-M42 在 v1.3 移进等价表 E4-E6，编号空着不复用；E4-E7 在 v1.5 随旧判据删去），M45-M48 是「同类的别处」：自动一轮里每一处写屏幕的错误都要并进返回值，M49、M49b 是 F6。M50-M55 = W1b 的 X1-X6，都是 `hasStrayAt` 在 `i > 0` 处的判据：M50 只看第 0 字节，M51 半角不看左边界，M52 左邻非 ASCII 也挡，M53 第一个合法起头的 `@` 定胜负，M54 全角只认第 0 字节，M55 换行也挡；M52、M55 的 `\|\|` 是 markdown 转义，真文本是 `||`。M56-M64（v1.3 收窄后判据的各支）在 v1.5 随判据删去，编号空着。v1.5 的 M53 换了锚点：认出的点名（下一个字节是字母数字）那一处的 `continue` 改成 `return false`，第一个合法起头的 `@` 若是认出的点名就定了胜负；M53b 是另一半：第一个没认出、又不冲着成员的合法 `@` 定胜负（`return true` 的循环之后多一句 `return false`）。M65-M76 是新判据的各支：M65 去掉按成员判（跳过之后取得出任何一段就算），M66 去掉显示名那一支，M67 匹配集合只用会话成员（配置那一行 `known[id] = true` 换成 `_ = id`，免得 `id` 未使用编译不过），M68 只用配置，M69 / M69b LF / CR 不截断，M70 全角也按成员判（从全角 `＠` 的三个字节之后开始跳过与匹配），M71 不剥尾点，M72 不折小写，M73 去掉「认成 mention 就跳过」，M74 非 ASCII 字节也跳过（v1.6 换锚点：非 ASCII 的字母、汉字、数字、换行也跳），M75 字母数字也跳过，M76 显示名为空的也参与（空串是任何文本的前缀）。M65 的 `\|\|` 是 markdown 转义，M69、M69b 的 `'\n'`、`'\r'` 见表头的例外。v1.6 的 M77-M84 是放宽之后的各支：M77 非 ASCII 一个都不跳（就是 v1.5 的判据），M78-M82 各从 `unicode.In` 的五类里去掉一类（Zs、P、S、M、Cf），M83 把 Zs 换成 Z（行分隔符 U+2028、段分隔符 U+2029 也跳），M84 显示名只在跳完之后比一次（就是 W1d 建议的写法：显示名以会被跳过的字符开头时永远对不上）。v1.7 的 M85-M87 落在显示名比较那一行：M85 换回 `strings.HasPrefix`（v1.6 的逐字节比，裁决书点名的那一条）；M86 去掉长度守卫 `len(text)-j >= len(name) &&`，剩下的文本比显示名短时 `text[j:j+len(name)]` 越界 panic（同 M15、M16 标 panic）；M87 把 `>=` 写成 `>`，显示名恰好到文本结尾时对不上。M74 的 `\|\|`、M69、M69b 的 `'\n'`、`'\r'` 与 M75 的 `\|\|` 同上。）

**判定为等价（跑过，全部 `TestChx` 存活，不算漏杀）**：v1.1 在原型上照下表逐条重跑，`internal/repl`、`internal/tui`、`internal/app`、`cmd/council` 四个包的全部 `TestChx` 都绿。v1.3 新加 E4-E7（E4-E6 是 v1.2 的 M40-M42），在原型 `ch1_v13` 上与 E1-E3 一起用脚本从本表取出重跑：锚点各恰好一次，C3、C5、C11、C13 全部用例都绿（原型只写了这四个用例；E4-E7 只改 `hasStrayAt`，它只经 `autoReplyRound` 被调用，其余用例的裸文本里没有 `@` 或只有认得出的 `@smx_a`）。v1.4 在原型 `ch1_fix3` 上把 E1、E3-E7 连同 C3 的两行新夹具重跑，这四个用例全绿（E2 改的是 `/add`，C10 原型没写，没跑）。v1.5 删去 E4-E7（锚点在删掉的 v1.3 判据里；它们说的「不剥尾点、不折小写」在新判据里不等价，登记成 M71、M72），在原型 `ch1_v15` 上重跑 E1、E3，C3（67 行）、C5、C11、C13 全绿（E2 同上没跑）。

| # | 文件 | 替换前 | 替换后 | 为什么等价 |
|---|---|---|---|---|
| E1 | autoreply.go | `ids = append(ids, canon)` | `ids = append([]string{canon}, ids...)` | 名单倒序：PlanRound 按字典序排，屏幕不变。 |
| E2 | commands.go | `if sumID := r.cfg.Summary.Agent; sumID != "" && seg == sumID` | `if sumID := r.cfg.Summary.Agent; seg == sumID` | `splitAddArgs` 丢掉空段，seg 永远非空，没配总结成员时两者都是 false。 |
| E3 | autoreply.go | `if canon, ok := byLower[m]; ok {` | `if _, ok := byLower[m]; ok {\n\t\t\t\tcanon := m` | 取 `m` 本身当 `canon`：id 正则强制全小写（`mention.go:22-25`、`config/validate.go` 的 idPattern），`strings.ToLower(id) == id`。 |

**只在审查时看**（没有用例能区分，集成员逐字核对）：
- `cmd/council/main.go` 的 `NoAuto:     *noAuto,` 是否真的传进 `app.Options`（同 CO1 对 `Verbose` 的处理：`run` 要走到 `app.Run` 就得过 `-version`、`-h` 两处返回，之后碰进程的标准流与信号）。
- v1 在这里列过「`postUser` 里读成员表失败那一支」，说真 store 上造不出「Post 成功、紧接着 Participants 失败」而 ctx 未取消的状态。那句字面上对，但由此推出「没有用例能区分」是错的：在「已记下 seq 」那次写入的钩子里取消 ctx，`Post` 已成功、随后读成员表必然失败，这一支与 `autoReplyRound` 的同一支都可测（v1.1 C12，杀 M35、M35b、M35c）；后者 `reportErr` 的写屏幕错误用「第一次写照常收下并取消、之后每次写都失败」的写者也可测（C12 `write_fail`，杀 M35d）。已移出本清单。
- `SetAutoReply` 与 `cmdAuto` 的并发纪律：TUI 在 `p.Run` 之前、行式在 `Run` 之前调；`/auto` 是串行派发的命令。本机不能 `-race`。
- （v1.6）`atSkip` 遇到非法 UTF-8 字节时，`utf8.DecodeRuneInString` 返回 U+FFFD（符号 So）、长度 1，于是按符号跳过这一个字节；用例里没有非法 UTF-8 的输入，改成「非法字节不跳」的变异没有用例能区分（原型 `ch1_fix5` 实测：在 `r, size := utf8.DecodeRuneInString(s)` 之后加「`r == utf8.RuneError` 就返回 0」，C3 / C5 / C11 / C13 / C14 的 98 个子测试全绿），集成员照 §1.1 E 逐字核对。
- （v1.7）显示名比较不先判 `utf8.ValidString`，只靠长度守卫。取舍的理由写在 `hasStrayAt` 的注释里，原型 `ch1_v17` 的白盒探针实测：剩下的文本比显示名短时长度守卫挡住，不切片（去掉守卫就 panic，M86）；切口落在多字节字符中间时（显示名 `Opus`、剩下 `Op坏 看一下`，切出 `Op` 加半个「坏」），那一段不是合法 UTF-8，`strings.EqualFold` 把坏字节当 U+FFFD，对不上，不算，与 v1.6 逐字节比的结果相同；剩下的是被截断的「甲」（`\xe7\x94`）或它后面跟 `A` 时也不算，都不 panic。唯一与 v1.6 不同的非法 UTF-8 情形要三件事同时成立：显示名自己含 U+FFFD，文本在那个位置是一个坏字节，后面还有一个折叠后字节数不同的字符把长度补齐（探针：显示名 U+FFFD 加 `k`，文本 `@ `、坏字节 0x80、开尔文符号 U+212A，v1.7 算、v1.6 不算）；这只会往谁都不问那一侧偏，不花钱。加上 `utf8.ValidString(text[j:j+len(name)])` 的变体在原型上实跑：C3 / C5 / C11 / C13 / C14 的 100 个子测试全绿，没有用例能区分，集成员照 §1.1 E 逐字核对。
- （v1.5）`autoReplyRound` 给显示名做的 `strings.TrimSpace`：`smxCfg` 里没有首尾带空白的显示名，去掉 TrimSpace 的变异没有用例能区分（原型 `ch1_v15` 实测：`strings.TrimSpace(a.DisplayName)` 换成 `a.DisplayName` 时 C3 / C5 / C11 / C13 的 82 个子测试全绿；装置是只读引用的既有辅助，不另造配置）。集成员核对这一处照 §1.1 E 逐字；「空显示名不参与」那一半有 M76 钉着。
- 注释与代码一致：`postUserText` 头注释说「五条路共用一个本体 postUser」，`cmdArgs.bare` 注释说「只有 dispatch、Submit、finishPaste 置上」（草稿走 `sayText(…, true)`，不经 `cmdArgs`），`round.go`「第七次」一段。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许改这 12 个文件：生产 8 个 —— 新文件 `internal/repl/autoreply.go`；修改 `internal/repl/commands.go`、`internal/repl/seam.go`、`internal/repl/paste.go`、`internal/repl/repl.go`、`internal/repl/round.go`、`internal/app/app.go`、`cmd/council/main.go`；授权的既有测试 4 个文件共 6 处 —— `internal/app/app_test.go`（1 处）、`internal/app/cox_app_test.go`（3 处）、`internal/app/fux_review_test.go`（1 处）、`internal/app/smx_summary_app_test.go`（1 处，v1.1），逐字照 §1.1 I1-I6（理由见 §1.2 的表）。不改 `go.mod`，不新写任何测试文件（那是测试腿的事）。生产改动原型实测约 330 行（新文件 236 行（v1.2 比 v1.1 多两行注释；v1.3 比 v1.2 多 11 行：`import "unicode"` 一行、注释十行；v1.5 比 v1.4 多 34 行：去掉 `import "unicode"`，`autoReplyRound` 构造匹配集合、`hasStrayAt` 换判据、新增 `asciiAlnum`；v1.6 比 v1.5 多 28 行：import 加 `"unicode"`、`"unicode/utf8"` 两行，`hasStrayAt` 的注释多 3 行、跳过那一段多 4 行，新增 `atSkip` 连注释与空行 19 行；v1.7 比 v1.6 多 3 行：`hasStrayAt` 注释里讲显示名比较的那一小条从 2 行变成 5 行，代码只改显示名比较那一行、行数不变），改动的 7 个文件共加约 100 行），一次交付，不拆块。
- 补丁块，每个文件可以有多个：

  ```
  ===PATCH: internal/repl/commands.go===
  <<<<ANCHOR
  （从 HEAD 3d134ee 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点照 §1.1 各处的 `anchor` 代码块，以 HEAD `3d134ee` 真文件为准（本文代码块里的缩进是 tab；若你看到的是空格，以真文件为准），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。**注意**：`commands.go` 与 `seam.go` 各有一行 `return r.runCommand(ctx, sayCmd, cmdArgs{fields: strings.Fields(text), rest: text})`，是两个文件里各自的唯一一处，分别打补丁；`commands.go` 里 `return err` 出现很多次，A9-A11 的锚点必须带上 §1.1 给的整段上下文。§1.1 A13 的锚点只有函数签名那一行。
- 新文件用 `===FILE: internal/repl/autoreply.go===` 换行接完整内容、末尾 `===END FILE===`。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- 代码必须能 `gofmt`（tab 缩进）；`go vet` 不许有新报告。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（全角括号、冒号、分号、逗号、顿号、直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、既有的 `——`（U+2014）、全角 `＠`（U+FF20，只出现在 `autoreply.go` 的注释里；代码里一律写 `string(rune(0xFF20))`，不写反斜杠 u 的转义、也不直接写这个字符，免得编辑工具把转义还原成字符、或与半角 `@` 看混）。本刀新增的字符串字面量只有 §3.1-§3.5 与 §1.1 A13 那一句。
- 生产代码新标识符不许以 `chx` 开头（那是测试腿的前缀）。
- §1.1 里给出的签名、判据、代码块、注释照抄；本刀没有「名字实现方定」处。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：12 个文件是否都给了；§1.1 A1-A14、B、C、D、E、F、G1-G2、H1-H2、I1-I6 各落在哪个补丁块。
  锚点：每个锚点是否从 HEAD 真文件逐字复制、在该文件里恰好一次。
  字面量：§3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  ```

---

## 6. 落地后主代理要做的

0. **派工前定落地顺序（v1.1；v1.2 改了那半句的措辞）**：**已定（主代理，2026-09-28 23:00）：(a)，AT1 先落，本刀同一批紧跟；本刀 W2 的基线是 AT1 落地后的提交，集成员在 worktree 里核「候选 = 活跃成员去掉总结成员」在 `mentioncand.go` 与 `autoreply.go` 两处一致。** §3.2 第 3、4 句的末半句「TUI 里敲 @ 会列出成员的 id 与显示名」是 AT1 的功能（`E:\multi_model_cowork\at1_contract.md`）；AT1 契约 v1.1 已撤回「建议 CH1 先落」，改由这里定。CH1 先落的话，CH1 落地到 AT1 落地之间这半句对用户是假话。三选一，由主代理（或 Griffin）在派工前定，定了再派：(a) AT1 先落或与 CH1 同一批落；(b) CH1 先落，派工前先改本契约，把两句末尾的「，TUI 里敲 @ 会列出成员的 id 与显示名」整段删掉（`msgAutoStrayAt`、`msgAutoStrayAtPartly` 与 §3.2、C3 / C11 / C13 的期望一起改，原型重跑），AT1 落地时再加回；(c) 接受这段时间的不准确（不建议）。
1. **收割**：越界扫描（主仓库、`internal/*`、`cmd/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；按锚点落盘；`gofmt -l ./internal ./cmd`；`go vet ./internal/repl ./internal/app ./internal/tui ./cmd/council`；`go build ./...`。**逐字核对**：把落盘结果与本契约 §1.1 逐对比较——最省事的办法是用脚本从本文取出每一对 `anchor` / `replace` 代码块与 `file` 代码块，打在 HEAD `3d134ee` 的一份 `git archive` 上，结果应当与 grok 落盘后的 8 个生产文件逐字节相同（原型就是这样核过的：生产 22 对（A1-A14、B、C、D、F、G1-G2、H1-H2）加测试 6 对（I1-I6），共 28 对锚点各恰好一次，加一个新文件，打出来的 12 个文件与原型逐字节相同）；§1.1 I1-I6 六处测试改动逐字；`internal/turn`、`internal/tui` 生产代码零改动。
2. **合并测试腿的 4 个文件**，先跑 `./internal/repl ./internal/tui ./internal/app ./cmd/council`（另一条流水线也可能在跑 go test；只跑自己的包，3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制），再跑一次全量 `go test -count=1 ./...`。既有用例必须全绿（原型实测全绿）。时序相关的 A2、T1、C1、C2、C3 `order` / `all` / `email` / `email_fullwidth` / `glued_at` / `glued_left_cjk_tail` / `glued_fullwidth` / `ps_hashtable` / `sql_identity` / `backtick_at` / `ps_array` / `at_everyone_cn` / `stray_eot` / `stray_newline` / `stray_crlf` / `price_at` / `numpy_matmul` / `bash_dollar_at` / `ps_herestring_close` / `newline_cut` / `cr_cut` / `ls_cut` 各跑一次 `-count=10`（v1.7 的 22 行 `slowA`：v1.6 比 v1.5 多 `ls_cut`、`display_case`，`display_case` 在 v1.7 翻成 `STRAY`、不再用 `slowA`，移出这份清单；`stray_bad_id`、`double_at`、`angle_template` 在 v1.5 翻成 `STRAY`、不再用 `slowA`，移出这份清单）（v1.1 原型上这些 `-count=10` 零偶发，§2 全部新用例 `-count=3` 零偶发；v1.2 原型上 C3 的前七行 `-count=10`、C3 / C11 / C13 `-count=3` 零偶发；v1.3 原型上 C3 的这十三行 `-count=10`（130 次）、C3 / C5 / C11 / C13 `-count=3`（207 个 PASS）零偶发；v1.4 原型 `ch1_fix3` 上 C3 的这十四行 `-count=10`（140 次）、C3 / C5 / C11 / C13 `-count=3`（213 个 PASS）零偶发；v1.5 原型 `ch1_v15` 上 C3 的这 21 行 `-count=10`（210 次）、C3 / C5 / C11 / C13 `-count=3`（246 个 PASS）零偶发；v1.6 原型 `ch1_fix5` 上 C3 的这 23 行 `-count=10`（230 次）、C3 / C5 / C11 / C13 / C14 `-count=3`（294 个 PASS）零偶发；v1.7 原型 `ch1_v17` 上 C3 的这 22 行 `-count=10`（220 次）、C3 / C5 / C11 / C13 / C14 `-count=3`（300 个 PASS）零偶发）。核对测试腿报告里 `cmd/council/chx_flag_test.go` 在 HEAD 上的红（第一条断言）。
3. **§4 的变异逐条跑**（M1-M87 共 86 条，含 b、c、d 条；v1 的 35 条加 v1.1 的 23 条加 v1.2 的 6 条，v1.3 减去移进等价表的 M40-M42 三条、加 M56-M64 九条，v1.5 删去 M56-M64 九条、加 M53b、M65-M76、M69b 十四条，v1.6 加 M77-M84 八条，v1.7 加 M85-M87 三条），从字节副本还原；E1-E3 跑一次确认存活（E4-E7 在 v1.5 删去）。点名杀手里的每个子测试都要单独确认变红，不以整个父测试红代替。任何一条没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
4. **文档**：
   - `docs/00_DECISIONS.md` #47：标「已落地（commit），真机未复验」；记下主代理定的细节（§7.1 全部）；§7.2、§7.3 原样搬过去。#45 那一条「`/add` 的配置候选里列出了总结成员……下一刀从候选里藏掉（待主代理）」标已修。18b §7.2 第 1 条（`/help` 的 `/session` 行）标已修。
   - `docs/README.md`：用法里加 `-no-auto` 与 `/auto`，说明默认是对话模式、代价（每句 N 次调用 + 可能一次总结）、点名要写半角 `@` 紧跟成员 id（`/who` 第一列，不带尖括号，不是显示名），`@` 前面空一格或在行首。另写一句（v1.2 写，v1.3、v1.4 改过，v1.5 按「冲着成员去」重写）：一个 `@` 没认成点名、却冲着一个认识的成员去（`@` 后面隔着空格、tab 或标点跟着成员 id，例如 `@ grok-4.7`、`@(grok-4.7)`、`@**grok-4.7**`，中文输入法下的全角括号、引号、冒号、全角空格也一样，例如 `@（grok-4.7）`；或跟着成员的显示名，大小写不同也算，例如显示名是 `Opus` 时的 `@ opus`（v1.7）；或者是全角 `＠`），这句谁都不问、打一行说明，改好写法重发，或照说明接着敲 `/round`；不冲着成员的 `@`（numpy / torch 的 `A @ B`、`$@`、PowerShell 的 `@{` 与 here-string 收尾、SQL 的 `@@IDENTITY`、`价格 @ 5 元`、句末单独一个 `@`、`@所有人`）当成没点名，问全部成员（花钱）；`@` 与 id 之间隔着换行也不算点名，同样问全部成员。`@所有人` 问全员只是因为它不冲着任何成员，不是「问全体」的语义。`@all` 与 Python 装饰器 `@staticmethod`、PowerShell splatting `@params`、`@Override` 一样解析得出 id，算「点的不是成员」，谁都不问、打两行说明，出路也是 `/round`。`@` 或 `＠` 左边紧贴 ASCII 字母、数字或 `_ @ . -` 时不算点名，照问全员（例如 `hi@grok-4.7`、`hi＠grok-4.7`）。README 里演示点名的例子用真实的 id（例如 `@grok-4.7`），不用尖括号占位符。
   - REPL 的 API 契约（`docs/09a_STEP6_API_CONTRACT.md` 或当前登记处）：登记 `SetAutoReply`、`app.Options.NoAuto`、`/auto`、`-no-auto`。
   - `docs/12_STEP9_CONTRACT.md` 顶部加一行订正：「绝不默认全员」对裸文本已被 #47 推翻，`/round` 自身不变。
   - 本契约归档为 `docs/19a_CH1_CONVERSATION_MODE_CONTRACT.md`（照 `18b` 的体例在文件头加落地后订正区）。
   - `.handoff/HANDOFF.md` 收工时记一条。
5. **给 Griffin 的真机复验步骤**（Windows Terminal + PowerShell 5.1）。前两步零花费；**第 4 步起会真的调用模型、花钱**，每一步开始前由 Griffin 决定跑不跑。建议用一次性的 `data_dir` 与只含两个便宜成员加总结成员的配置，不碰日常库。council 内的命令**一行一块，逐块粘贴、逐块回车**（#44：整块粘贴多行会被当成一条提交）。下面 `<成员甲>`、`<成员乙>` 换成配置里的两个讨论成员 id（例如 `opus-5.0`、`grok-4.7`），`<总结成员>` 是 `[summary] agent` 的值。换的时候连尖括号一起换掉（v1.4，W1c F1；v1.5 改）：第 6 步写成 `@<grok-4.7> 你再补一句` 在 v1.5 下谁都不问、打一行「有 @ 没认出点的是谁……」（C3 `angle_template`），得删掉尖括号重发，白敲一次（不花钱；v1.4 下是问全员加一次总结）。
   1. （零花费）PowerShell 里 `council -h`：输出里有 `-no-auto` 与 §3.5 那句说明。
   2. （零花费）`council` 进 TUI（默认简洁、默认对话模式），依次：

      ```
      /new 对话模式复验
      ```

      ```
      /add
      ```

      看：候选表里**没有** `<总结成员>`，其余与以前相同；识别表照旧。

      ```
      /add <总结成员>
      ```

      看：一行「<总结成员> 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结」，没有加入。

      ```
      /help
      ```

      看：有 `/auto` 一行；`/say` 那一行开头是「只记下、不让任何人回复」；`/session` 那一行有「先结清这个成员留在库里的陈旧在飞记录」。
   3. （零花费）会话里还没有成员时直接打一句：

      ```
      还没人
      ```

      看：一行「当前会话还没有能回复的成员，这条只记下了；用 /add 加成员之后，直接打字就会有人回复」，没有起进程。
   4. （**花钱：两个成员各一次，首次开口是冷启动**）

      ```
      /add <成员甲>,<成员乙>
      ```

      ```
      用一句话介绍你自己
      ```

      看：`本轮问 2 个: …`、两人的回复（带用时）、`本轮问 1 个: <总结成员>`、「总的来看（… 做总结）：…」、收尾行 `/round 结束 … 回来 2/2`。
   5. （**花钱：两人各一次 + 一次总结**）第二句话，确认总结成员已加入之后不会整句被拒：

      ```
      再用一句话说说你们的分歧
      ```

      看：与第 4 步同形，没有「拒绝: … 是总结成员」。
   6. （**花钱：只有成员乙一次**）

      ```
      @<成员乙> 你再补一句
      ```

      看：只问了成员乙，没有总结（一个人）。
   7. （零花费）

      ```
      @nobody 这句谁都不该回
      ```

      看：「nobody 不是当前会话的成员」与「点名的人都不是当前会话里能回复的成员……」两行（v1.3 起第二行末尾有出路；v1.5 的说法是「这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复」），没有起进程。

      下面 `<甲的显示名>` 换成成员甲在配置里的 `display_name`（v1.1）：

      ```
      @<甲的显示名> 这句也谁都不该回
      ```

      看：一行「有 @ 没认出点的是谁，这条只记下了、没有问任何人；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名……」（v1.5 的说法），没有起进程。再用中文输入法打一个全角 `＠` 紧跟 `<成员乙>` 试一次，看到同一行、同样没有起进程。（v1.5）再试一次 `@` 后面空一格再跟 `<成员乙>`，同样是这一行、没有起进程。（v1.6）再敲一次半角 `@` 紧跟中文输入法的全角括号括起来的 `<成员乙>`（`@（<成员乙>）`），同样是这一行、没有起进程（v1.5 下这一句会问全员加一次总结）。（v1.7）`<甲的显示名>` 里有英文字母的话，再敲一次 `@`、空格、大小写换过的显示名（例如显示名是 `Opus` 就敲 `@ opus 这句也谁都不该回`），同样是这一行、没有起进程（v1.6 下这一句会问全员加一次总结）；显示名全是中文就跳过这一步。

      （v1.5 改，零花费）一句不是点名的全角 `＠`（中文输入法下敲出来的）：

      ```
      看一下 ＠所有人
      ```

      看：同一行说明（末尾有「这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复」），没有起进程。要不要照它接着敲 `/round <成员乙> --go`（**花钱：成员乙一次**）由 Griffin 定。v1.3、v1.4 这一步用的是 `价格 @ 5 元`，v1.5 起它问全员（**花钱**），不再是零花费的一步。

      （v1.3，**花钱：两个成员各一次 + 一次总结**，由 Griffin 定跑不跑）贴一行 PowerShell，确认收窄之后它会问全员：

      ```
      $h = @{ Name = 'x' } 这行为什么报错
      ```

      看：与第 4 步同形（`本轮问 2 个: …`、两人回复、总结），没有「有 @ 没认出」那一行。v1.2 的这一步是零花费的，v1.3 起不是（v1.5 照旧问全员：`@{` 后面的 `Name` 不是成员）。

      ```
      /say 这句只记下
      ```

      看：什么都不打（简洁档），没有起进程。

      ```
      /auto off
      ```

      ```
      关掉之后这句也只记下
      ```

      看：没有起进程。

      ```
      /auto on
      ```

      看：「已打开自动回复：……多个成员回复之后自动总结；/auto off 关掉」（配了总结成员时是这一句；v1.1）。

      ```
      /log 7
      ```

      看：上面几句都记下了。
   8. （零花费）`/quit` 之后用 `council -no-auto` 再进同一个 `data_dir`，`/use` 进这个会话（id 用 `/conv` 查），直接打一句，确认只记下不回复。
   9. 行式模式（`council -no-tui`）照第 4、6、7 步各选一步复验一次（第 4、6 步花钱，由 Griffin 定）；行式下 Ctrl-C 会腰斩这一轮并退出 council（`/round` 的既有行为），复验时不要按。

---

## 7. open items

### 7.1 主代理已定（#47 授权定小事；本契约照这些写，Griffin 不同意时再改）

1. **复用方式**：自动一轮经 `r.runCommand(ctx, roundCmd, cmdArgs{fields: ids + "--go"})` 进 `cmdRound`，不抽函数、不另写扇出。理由：屏幕与手敲同样名单的 `/round <ids> --go` 逐字节相同是构造上成立的（C1 用同一个期望钉两侧；唯一例外是第 13 条的那一行，打在这一轮的屏幕之前）；needsRunner 闸自动生效；闸、在飞记录、指纹、BeginTurn、记账、总结、总用时、观察者一个不绕。
2. **名单顺序**：按 mention 出现顺序收集，交给 PlanRound 按字典序排、去重；屏幕上「本轮问 N 个」是字典序（与 `/round` 相同）。
3. **TUI 收尾行仍写 `/round 结束 …`**：自动一轮在 TUI 下的收尾行字面里有 `/round`，对用户是略不准的（他没敲 `/round`）；本刀不改 TUI（§1.2）。v1.1 主代理裁定：维持。
4. **没配总结成员时的提示**：简洁档下，两人及以上的自动一轮之后照 SM1 打「总的来看：没有总结（没配总结成员……不想看这一行就加 --no-summary）」。对话模式下它**每句话都会出现一次**，而裸文本没有地方加 `--no-summary`。Griffin 的配置已经配了总结成员，本刀照 SM1 原样（T1 钉着）。v1.1 主代理裁定：维持，不压。
5. **judge 成员**：没点名时全员里的 judge 照 PlanRound 进 Judges，每句话都打一行「裁判 X 本命令不问，要用 --with-judge 才问。」（C3 `judge` 钉着）。v1.1 主代理裁定：维持（Griffin 今天没有 judge 成员）。
6. **行式括号粘贴算裸文本**（#47 点名三条路，这是第四条；§1.1 C）。
7. **CO1「点名了」行**：只在「裸文本且自动回复打开」时不打；`/say`、`/to`、`/ask <id> 文本`、自动回复关着时照打（C5）。
8. **`/add` 总结成员**：候选里永远不列；显式点名时拒绝，**唯一放行的是它在当前会话里是已退出成员**（`summary.go:95` 指给用户的恢复路 `/join <id>` 必须留着）。在场时拒绝（这时 `/add` 本来也只是刷新快照，拒绝无害）。
9. **开关命名**：`app.Options.NoAuto`（键长照 gofmt 对齐选的，§1.1 G1）、`-no-auto`、`/auto [on|off]`、REPL `SetAutoReply` / 字段 `autoReply`。不加额外的「自动问 N 个」行。
10. **rn 为 nil**：消息照常记下，打既有的 `msgNoRunner`（生产上 app 在 svc 非 nil 时一定造 Runner，这一支只在测试装置里出现）。
11. **点名里夹着已退出的成员或总结成员**：名单是 mention 与**活跃**成员（去掉总结成员）的交集，点到的已退出成员或总结成员静默不问、其余照问（C3 `left_mixed`、`summary_and_member`）；手敲 `/round <已退出> <在场> --go` 会整条「拒绝: 不是当前活跃成员」、名单里有总结成员会整条「拒绝: … 是总结成员」，这里有意不同：用户说话时点了一个刚退出的人或总结成员，不该让在场的人也不回。全部点名都失效时才打 §3.2 第 1 句。「静默」的一处例外见 §7.2 第 6 条。
12. **（v1.1 F0；v1.3 收窄；v1.5 改成按成员判）想点名却一个都没认出来：谁都不问。** 判据 `hasStrayAt`（§1.1 E）。v1.5 只问「这个 `@` 是不是冲着一个认识的成员去的」（主代理裁决 `ch1_rulings_v15.md`）：只看 ParseMentions 允许 mention 起头的位置；全角 `＠` 到这里就算；半角 `@` 的下一个字节是 ASCII 字母或数字时，ParseMentions 已经把它认成 mention，不算；其余的半角 `@` 跳过后面紧跟的、既不是字母数字也不是 CR / LF 的 ASCII 字节（空格、tab、`\v`、`\f`、标点），停在 CR / LF 上不算，跳过之后恰好是匹配集合里的一个 id（按 `mentionTokenChar` 取最长一段、剥尾点、转小写），或以配置里的一个显示名开头（v1.5、v1.6 逐字节，不折大小写；v1.7 起不分大小写，见本条末尾），才算。v1.6（W1d 修订 F1）：跳过的字符由 `atSkip` 定，非 ASCII 的空白（Zs：全角空格、NBSP、U+2002 这类）、标点（P：全角括号、引号、冒号、逗号、直角引号、`【】`）、符号（S：全角 `＜ ＋ ～ ｜ ￥` 与 emoji）、组合记号（M：emoji 后面的 VS16）、格式字符（Cf：零宽空格、U+2060、U+FEFF）也跳；字母、汉字、数字不跳（显示名与 id 要从它们开始比），U+2028、U+2029、U+0085 这几个换行不在这几类里，与 CR / LF 一样停下（白盒探针实测三种都不算；C3 `ls_cut` 钉 U+2028，端到端问全员）。显示名改成在跳过途中每一个位置都比（含 `@` 紧后面、还没跳的时候）：显示名自己以括号、引号这类会被跳过的字符开头时（例如 `【测试】乙`、`(bot) X`），只在跳完之后比就永远对不上（C14）。匹配集合 = 配置里全部 agent 的 id 并上当前会话活跃成员的 id：配置里有、会话里没加入的也算（`@ smx_x 看一下` 谁都不问、不问全员，C3 `config_only`）；会话里有、配置里已经没有的也算（#45 的改名，C3 `session_only`）。这样的 `@` 存在、且这条消息一个 mention 都没有时，打 §3.2 第 3 句，不起进程。它先于「没有可问的成员」判（C3 `stray_no_members`：会话里没有成员，靠配置里的显示名「甲」命中）：用户想点名，先告诉他点名没认出来，比告诉他会话里没人更对症（这时第 3 句的 `/round` 出路用不上，`/round` 自己会打「不是当前活跃成员」，看得见，不花钱）。理由：`@甲`、`＠smx_b`、`@ smx_b`、`@<smx_b>`、`@(smx_b)`、`@（smx_b）` 这类消息的意图是「只问某个人」，当成「没点名」去问全员是花钱方向的误判；不冲着成员的 `@`（贴进来的 numpy `A @ B`、`$@`、PowerShell 的 `@{` 与 here-string 收尾、SQL 的 `@@IDENTITY`、`价格 @ 5 元`、句末单独一个 `@`、`@所有人`）就是没点名，按 Griffin 的原话「如果不 @ 成员，默认是所有加入的都回复」问全员（C3 `numpy_matmul`、`bash_dollar_at`、`ps_herestring_close`、`ps_hashtable`、`sql_identity`、`price_at`、`stray_eot`、`at_everyone_cn`）。换行截断：`@` 与成员 id 之间隔着 LF 或 CR 时不当成冲着成员（行末的 `@` 与下一行开头的词不是一回事；C3 `newline_cut`、`cr_cut` 问全员）；tab、`\v`、`\f` 照样跳过（C3 `stray_tab`、`stray_vtab`、`stray_formfeed` 仍是 `STRAY`）。全角 `＠` 不按成员判：它几乎只会是中文输入法下敲出来的，后面是什么都算（C3 `stray_fullwidth`、`fullwidth_everyone`）；不许为了一致改成按成员判（M70）。`@所有人` 问全员只是因为它不冲着任何成员，这里**不许**加「`@所有人` / `@all` 就是问全体」的语义，与 v1.3 的禁令不矛盾。邮箱按左边界不算（C3 `email`、`email_fullwidth`）。v1.3、v1.4 按「`@` 后面紧跟的那一个字节」判，两个方向都漏（§7.2 第 9-11 条），v1.5 整段删掉，不在旧判据上叠。v1.7（主代理裁决 `ch1_rulings_v17.md`，§7.2 第 12 条定 (a)）：显示名不分大小写。id 那一支本来就折小写（`@ SMX_B 看一下` 是第 3 句），显示名那一支逐字节比时，`@ 坏 x 看一下`（显示名「坏 X」）冲着认识的成员去、却问了全员加一次总结，属于收口规则第 (1) 类。写法：剩下的文本不短于显示名的字节长度，并且 `strings.EqualFold(剩下的文本[:len(显示名)], 显示名)`（W1d 实跑过的写法），「跳过途中每个位置都比」照旧。切片安全是实测的事实，不是假设（原型 `ch1_v17` 白盒探针）：长度守卫挡住越界（去掉守卫就 panic，M86）；切口落在多字节字符中间时那一段不是合法 UTF-8，`EqualFold` 按 U+FFFD 比，对不上，不算；唯一与逐字节比不同的非法 UTF-8 情形要显示名自己含 U+FFFD，只往谁都不问那一侧偏。所以不另判 `utf8.ValidString`，也不按 rune 边界切（§4「只在审查时看」）。`strings.EqualFold` 按 Unicode 的简单大小写折叠比，不只 ASCII：字节长度相同的非 ASCII 大小写（`é` 与 `É`、希腊字母、西里尔字母、全角拉丁字母）也算；折叠前后字节长度不同的（开尔文符号 U+212A 与 `k`、`ſ` 与 `s`、`ẞ` 与 `ß`）因为是按显示名的字节长度切的，对不上。中文显示名没有大小写，不受影响。实测清单在 §7.3 第 9 条。
13. **（v1.1 F0）点名里认出了一部分、还有没认出的 `@`：照问认出的那些，先打一行。** 打 §3.2 第 4 句，然后照有 mention 的规则走（交集非空就问交集，为空就打第 1 句）。理由：与第 11 条「点名里夹着不在场的人照问其余」同一个方向——用户点了名，认得出的人是他明确要问的，问他们不会多花钱；没认出的那一处不能静默吞掉，所以打一行，用户看得见、可以补问。这一行打在 postUser 的「<tok> 不是当前会话的成员」之后、第 1 句或这一轮的屏幕之前（C3 `partly`、`partly_none`，原型实测）。v1.4 写明的例外（`@smx_b @(smx_a) 看一下` 静默少问一个）在 v1.5 修好：`@(smx_a)` 跳过 `(` 之后是成员，照打第 4 句（C3 `partly_ascii`：`PARTLY` + `ONE_B`）；`@<smx_a>`、`@"smx_a"`、`@-smx_a`、`@_smx_a` 同理（接在 `@smx_b` 后面，原型 `ch1_v15` 端到端都是 `PARTLY` + `ONE_B`）。与认出的点名混用时，不冲着成员的 `@` 也不再误报第 4 句：`@smx_b y = A @ x` 只问 smx_b、不打第 4 句（原型 `ch1_v15` 端到端）。v1.6（W1d 修订 F1）：v1.5 只跳 ASCII，全角写法 `@smx_b @（smx_a）看一下` 在 v1.5 下仍然静默少问 smx_a（W1d 端到端：`ONE_B`、没有第 4 句），v1.5 这里写的「修好」对这一类不成立；v1.6 跳非 ASCII 的分隔符之后打第 4 句、照问 smx_b（C3 `partly_fullwidth`：`PARTLY` + `ONE_B`；`@smx_b @`全角空格`smx_a 看一下`、`@smx_b @`零宽空格`smx_a 看一下` 端到端同样是 `PARTLY` + `ONE_B`，`@smx_b @“smx_a”看一下` 白盒算没认出；原型 `ch1_fix5`）。
14. **（v1.1 F6）`/auto` 打开时的提示按配置选句**：配了 `[summary] agent` 说「多个成员回复之后自动总结」，没配说「没配总结成员，多个成员回复之后不做总结」（§3.1）。只看配置里有没有写，不看总结成员此刻是否已退出当前会话（已退出时 SM1 打「要恢复总结用 /join <id>」那一行，看得见）。
15. **`/say` 首行出路 vs 行式草稿的不对称：维持**（v1 列在 §7.3 第 1 条，v1.1 主代理裁定）。行式模式里 `/say`（不带参数）进草稿、`/end` 发送**会自动回复**（#47 字面：草稿发送算裸文本）；TUI 里首行只写 `/say` 的多行提交**不会自动回复**：那是 X44 给的显式「只记下」出路，#47 定了 `/say` 保持只记下。一段含 `/` 开头行的长文，在 TUI 下要让成员回复，先 `/say` 记下、再 `/round <ids> --go`。
16. **「成员超过 K 个先确认」不加**（v1 列在 §7.3 第 2 条，v1.1 主代理裁定）：Griffin 要的是直接对话。代价在 §0 第 10 条与 README 写明（每句 N 次调用、N ≥ 2 时再加一次总结、没有预览；他的 `teams.all` 有 7-8 个成员时，随手一句就是 7-8 次调用加一次总结）。

### 7.2 待主代理

1. `/team use` 会把 team 里的总结成员 `Join` 进来（`commands.go:687-719` 不看 `[summary]`）；配置校验是否禁止总结成员进 team 本刀没核实。与 `/add` 同类，本刀不做（v1.1 主代理裁定：不在本刀，留登记）。
2. 独立审查时核对：`SetAutoReply` 的并发纪律、REPL 与 Runner 持有同一个配置指针（§4「只在审查时看」；v1.1 主代理裁定：后者保留为只在审查时看）。v1 这一条里的「`postUser` 读成员表失败那一支」已由 C12 覆盖，删去。
3. **两处总结成员 id 的来源**：`autoReplyRound` 读 `r.cfg.Summary.Agent`，`cmdRound` 读 `snap0.Cfg.Summary.Agent`（Runner 的配置）。两者一致依赖「REPL 与 Runner 永远持有同一个 `*config.Config`」：读代码核过（未实测）——`/config reload --apply` 两处一起换（`configcmd.go:153-158`），不带 `--apply` 的预览临时换 Runner、`defer` 换回（`:159-161`），`/add +编号` 两处一起换（`addmodel.go:454-455`），app 装配时传同一个指针（`app.go:174`、`:229` / `:234`）。哪天有一条路只换一处，自动一轮可能把总结成员放进名单、被 `cmdRound` 整句拒绝；测试装置必须让 REPL 与 Runner 用同一份配置（`smxSetup` 就是）。
4. **（v1.1）AT1 与 CH1 的落地顺序**：§3.2 第 3、4 句的末半句提到 AT1 的 `@` 补全，派工前按 §6 第 0 步三选一。**已定：(a)，AT1 先落（§6 第 0 步）。**
5. **（v1.1；v1.2 改）两处与裁决字面不同**：`msgAutoStrayAt` / `msgAutoStrayAtPartly` 指 id 的命令写 `/who` 而不是裁决说的 `/add`（`/add` 列的是不在会话里的配置 agent，能 `@` 到的只有当前会话成员，`/who` 才列它们；v1.2 写成「/who 第一列是成员 id」，因为 `/who` 不列显示名；v1.5 并进点名那一段的括号：「成员 id（/who 第一列）」）；「可以用 @ 补全」写成「TUI 里敲 @ 会列出成员的 id 与显示名」（行式模式没有补全，AT1 之后也没有，所以限定 TUI；v1.1 写的「弹出可点的成员」与 AT1 不符，AT1 不做鼠标点选，v1.2 照 AT1 的候选行改写）。主代理不同意就改 §3.2 与 E，原型重跑 C3 / C11 / C13。**已定（主代理）：两处都接受，关掉。** `/add` 列的是不在会话里的 agent，裁决书 F0 写「/add 能看到 id」是主代理写错了。
6. **（v1.1，V1 的残余）总结成员第一次总结之前被点名**：真机上第一句话之前总结成员还不是会话成员，这时 `@<总结成员> @<成员>` 会让 postUser 先打一行「<总结成员> 不是当前会话的成员」，再照问 `<成员>`；第 11 条的「静默」从第一次总结之后才成立。只是措辞，不花钱，也不影响名单（C3 `summary_and_member` 的装置里 smx_s 一开始就是成员）。
7. **（v1.1；v1.5 改）mention.go 与 `hasStrayAt` 的同步**：`hasStrayAt` 只读引用 mention.go 的 `mentionLeftForbidden`、`mentionTokenChar`，与 ParseMentions 同一条左边界。v1.5 起「这个 `@` 有没有被 ParseMentions 认成 mention」由「下一个字节是不是 ASCII 字母或数字」决定：它与 ParseMentions 今天的规则等价（`mentionIDPattern` 只要求首字符是字母数字，剥掉尾点之后结尾一定合；读 `mention.go` 推出，M73 实测少了这一步 `@smx_b` 会被判成没认出），但等价靠的是 `mentionIDPattern` 与 `mentionTokenChar` 今天的样子。以后改 ParseMentions（例如接受全角 `＠`、显示名，或改 id 正则的首字符）时，`hasStrayAt` 这一步要跟着改，否则会出现「既算点名又算没认出」的双重判定，或者漏判；函数注释里写了这句，mention.go 这一刀不许动，没加反向提醒。v1.3、v1.4 留着的「取 token 再过 id 正则」一步按裁决书第 1 条整段删掉。接受全角 `＠` 那一种仍要手改 `full` 那一支。
8. **（v1.2，W1b F2 的方向 (A)）收窄 `hasStrayAt` 的判据。已定 (A)（主代理，2026-09-28 23:05，`ch1_rulings_v13.md` 第 1 条），v1.3 落实；v1.5 这条收窄的判据整段删掉，换成按成员判（§7.1 第 12 条），方向 (A) 的本意（贴进来的代码问全员）保留**：半角 `@` 取不出合法 token 时，只有下一个字节是 ≥ 0x80、ASCII 空白（含 `\v`、`\f`，理由见 §7.1 第 12 条）或文本结尾才算没认出；下一个字节是别的 ASCII（`{ ( " ' @ _ $ -`、反引号等）不算，回到问全员。全角 `＠` 不收窄，出现在左边界允许起头的位置就算。W1b 原文「`@` 后面紧跟非 ASCII 字节、空白或文本结尾」没有照抄：照抄会让 `＠smx_b` 问全员，推翻 C3 `stray_fullwidth`。依据是 Griffin 23:00 的原话「如果不 @ 成员，默认是所有加入的都回复，然后有总结的」：他大量把 PowerShell 贴进来给成员看，v1.2 让这类消息谁都不问，与原话相反。代价（花钱方向，Griffin 要的方向）：这类消息每句都按全员付费；`@-smx_b`、`@(smx_b)`、`@"smx_b"` 这类想点名的写法也问全员（§7.3 第 7 条）。改了什么：`hasStrayAt` 的判据与注释（§1.1 E），C3 翻三行、加十行，C13 换输入，§4 的 M36、M38、M50 杀手栏、M53 锚点，M40-M42 移进等价表（E4-E6），新增 M56-M64 与 E7。v1.2 与 v1.3 的判据下各种写法的实测结果见 §7.3 第 5 条。
9. **（v1.4，W1c F1，major）提示句里的 `@<成员 id>` 照字面敲会问全员。v1.5 已关。**（主代理裁决 `ch1_rulings_v15.md`：按成员判之后 `@<smx_b>` 是冲着成员去的没认出点名，谁都不问，C3 `angle_template` 翻成 `STRAY`，效果与下面方案 (a) 相同；提示句同时去掉了尖括号，即方案 (b)，§3.2。以下是 v1.4 的登记原文。） §3.2 第 3、4 句教用户「点名写 @<成员 id>」，尖括号照打。用户点名刚失败、照提示重试时敲成 `@<smx_b> 看一下`：`@` 的下一个字节是 `<`，按 v1.3 判据不算没认出，句子里也没有 mention，于是问全员加一次总结（C3 `angle_template` 钉着现状：`smxS1Want`、时钟 2；v1.2 下是第 3 句、谁都不问）。W1c 的判断：这是 v1.3 新引入的花钱方向回退。原型 `ch1_fix3` 探针（成员 smx_a、smx_b）：`@<smx_b> 看一下`、`看一下 @<smx_b>`、`第一行`（换行）`@<smx_b>`、`@<smx_b>` 都问全员（前两条另做了端到端，`smxS1Want`、时钟 2）；`@smx_b @<smx_a> 看一下` 只问 smx_b、不打第 4 句（端到端，与 §7.1 第 13 条的例外同一类）。同类的另一半 `/round <成员 id...> --go` 照字面敲不花钱：`/round <smx_a> --go` 打「拒绝: 不是当前活跃成员: <smx_a>」、时钟 1、四个成员都没有消息（端到端，简洁、详细两档相同）。本版取裁决书的读法（`<` 是「别的 ASCII」，回到问全员），判据与字面量都不改。两个出路，由主代理选一个或都做：
   - (a) 半角 `@` 的下一个字节是 `<` 也算没认出。原型上作为变体实跑（判据那一行的 `|| text[i+1] >= 0x80 ||` 换成 `|| text[i+1] >= 0x80 || text[i+1] == '<' ||`）：C3、C5、C11、C13 全部用例里只有 `angle_template` 变红，其余照绿；上面五条探针都回到 v1.2 的结果（四条是第 3 句，`@smx_b @<smx_a> 看一下` 打第 4 句再问 smx_b）。采纳时：`hasStrayAt` 的判据与注释改一行，`angle_template` 的期望改成 `STRAY`、时钟 0、去掉 `slowA`；M38、M56 的杀手栏去掉它（按判据推，那两条下它照样打 `STRAY`，未实测）；加一条变异「去掉 `<` 这一支」，杀手 `angle_template`；§7.3 第 7 条与 README 同步，原型重跑。W1c 的说法：粘贴的代码里几乎没有 `@<`，Discord、Slack 的点名写法是 `<@id>`，`<` 在 `@` 左边，不受影响（这一句没实测）。
   - (b) 两句提示去掉尖括号，改成不会被照抄的说法（W1c 举的例子：「点名写半角 @ 紧跟 /who 第一列的成员 id」）。要改冻结字面量 `msgAutoStrayAt`、`msgAutoStrayAtPartly`（§1.1 E、§3.2，主代理在第 5 条接受过的措辞），C3、C11、C13 里期望含 `STRAY` / `PARTLY` 的行随符号整串更新、原型重跑，§6 第 5 步引的那句同步；没实跑。第 3 句的 `/round <成员 id...> --go` 不必跟着改（上面实测照抄不花钱）。
10. **（v1.4，W1c F2，major）`@` 后面是空白的那一支也命中最常见的贴代码写法：numpy / torch 的矩阵乘 `A @ B`。v1.5 已关。**（主代理裁决 `ch1_rulings_v15.md`：按成员判，`A @ B` 的 `B` 不是成员，问全员，C3 `numpy_matmul`；效果接近下面方案 (c)，但匹配集合是配置里的 agent 并上会话成员，不只当前成员；`看一下 @ smx_b` 这类想点名的写法仍是 `STRAY`，不像方案 (b) 那样翻成问全员。以下是 v1.4 的登记原文。） PEP 8 把矩阵乘写成 `A @ B`，`@` 的下一个字节是空格，按判据算没认出，整句谁都不问。Griffin 做 PINN / FEM，贴 numpy、torch 代码给成员看是常见用法；按他 23:00 的原话，这类消息应当问全员。原型 `ch1_fix3` 探针（成员 smx_a、smx_b）都是第 3 句：`y = A @ x + b`（端到端：第 3 句、时钟 0、四个成员都没有消息）、`C = A @ B.T`、`out = x @ self.W`、`import numpy as np`（换行）`K = B.T @ D @ B`（换行）`u = np.linalg.solve(K, f)`、`x = A @`（换行）`B`；同一支还有 `明天 @ 3pm 开会`、`联系 name @ example.com`、Mathematica 的 `f @ x`。与认出的点名混用时第 4 句是误报：`@smx_b y = A @ x` 输出 `PARTLY` + `ONE_B`（端到端），说「还有 @ 没认出」，那个 `@` 其实不是点名。本版取裁决书的读法（裁决书第 1 条点名「ASCII 空白」算没认出），判据不改，文档照实写（§0 第 11 条、§3.2 说明、§6 第 4 步 README、§7.1 第 12 条、§7.3 第 5 条）。两个改判据的方向：
   - (b) `@` 左右两个字节都是 ASCII 空白时不算没认出（文本开头、结尾不算空白）。原型上作为变体实跑（取 token 那一行之后加：左邻、右邻都满足 `unicode.IsSpace` 就 `continue`）：C3、C5、C11、C13 全部用例里变红的恰好是 `price_at`、`stray_newline`、`stray_crlf` 与 C13 `stray`，其余照绿（与 W1c 按判据推的一致）。探针里上面矩阵乘五条、`明天 @ 3pm 开会`、`联系 name @ example.com`、`f @ x` 都翻成问全员，`@smx_b y = A @ x` 只问 smx_b、不再误报第 4 句；代价是 `看一下 @ smx_b`（想点名，`@` 后面多敲了一个空格）也翻成问全员；`@ smx_b 看一下`（`@` 在文本开头）、`你看 @`（文本结尾）、`@ 看一下` 不变，仍是第 3 句。采纳时上面四行重冻、C13 `stray` 换一个仍是第 3 句且 `@` 在句中的输入，加变异，原型重跑。
   - (c) 空白那一支只在空白后面紧跟当前成员的 id 时才算：要把成员表传进 `hasStrayAt`，改签名与 `autoReplyRound` 的调用。没实跑，影响未测。
11. **（v1.4，W1c F3，major）`@` 左边紧贴 ASCII 符号、右边是空白或文本结尾的代码写法仍谁都不问：here-string 收尾、`$@`、`/@`。v1.5 已关。**（主代理裁决 `ch1_rulings_v15.md`：按成员判，这些 `@` 后面不是成员，问全员，C3 `bash_dollar_at`、`ps_herestring_close`；另探针 `$s = @"`（换行）`hello`（换行）`"@ | Out-File a.txt`、`$(CC) -o $@ $^`、`f /@ list`、`https://medium.com/@张三/post 看看` 在原型 `ch1_v15` 上都问全员。没有采纳下面的左邻规则，`"@甲"` 这类想点名的写法仍是 `STRAY`（原型探针 `"@甲" 看一下`）。以下是 v1.4 的登记原文，原文写的「待主代理」已不适用。） 原型 `ch1_fix3` 探针都是第 3 句：PowerShell here-string 的收尾 `写成 @"`（换行）`多行`（换行）`"@ 对吗`、`写成 @'`（换行）`多行`（换行）`'@ 对吗`、`$s = @"`（换行）`hello`（换行）`"@ | Out-File a.txt`、`$s = @'`（换行）`hello`（换行）`'@`（收尾后面是文本结尾）；Makefile 与 bash 的 `$@`：行首带 tab 的 Makefile 规则行 `$(CC) -o $@ $^`、`echo $@`、`cmd $@`（换行）`exit 0`；Mathematica 的 `f /@ list`；URL `https://medium.com/@张三/post 看看`（右邻是非 ASCII）。端到端：here-string 单引号版、Makefile 那一行、`echo $@` 都是第 3 句、时钟 0。对照：`exec "$@"`、`for a in $@; do echo $a; done` 的 `@` 后面紧跟 ASCII 符号，v1.3 起问全员（前者端到端）。裁决书收窄的依据是「他大量用 PowerShell，把 @{、@( 这类代码贴进来」；here-string（Griffin 自己的规则文件要求多行字符串用 `@'…'@`）与 #44 的整块粘贴合在一起，常见的多行 PS 粘贴仍谁都不问：`@{`、`@(` 修好了，here-string 没修好。本版取裁决书的读法（只看右邻），判据不改。W1c 给的方案：
   - 在 `hasStrayAt` 里加一条局部规则：半角 `@` 的左邻恰好是 `"`、`'`、`$`、`/` 之一时不算没认出；不改 ParseMentions。原型上作为变体实跑（取 token 那一行之后加：左邻在这四个字节里就 `continue`）：C3、C5、C11、C13 全部用例照绿，没有一行翻（现有夹具里没有这一类）；探针里上面九条都翻成问全员；代价是 `"@甲" 看一下`、`'@甲' 看一下`、`看一下 /@甲`、`看一下 $@甲` 这类想点名的写法也翻成问全员（v1.3 下是第 3 句）；`(@甲) 看一下` 不受影响（左邻 `(` 不在集合里，仍是第 3 句），所以集合要取窄，不要扩成全部 ASCII 符号。采纳时 C3 要加夹具（here-string 收尾、`$@` 各一行，期望问全员、用 `slowA`）与对应变异（去掉这条规则，杀手是这两行），原型重跑。全角 `＠` 左邻是引号的情形 W1c 没提，没测。
   不采纳的话，这一类写在 §7.3 第 5 条「仍谁都不问」一组与 README（v1.4 已写），出路是第 3 句末尾的 `/round`。
12. **（v1.6，W1d 修订 F2，W1d 定为 major）显示名的大小写：`@` 与显示名之间隔着分隔符、大小写敲得与配置不同时，这句问全员。v1.7 已定 (a)。**（主代理裁决 `ch1_rulings_v17.md` 第 1 条：显示名按大小写不敏感的前缀比，写法照下面方案 (a)，§1.1 E 与 §7.1 第 12 条；C3 `display_case` 翻成 `STRAY`、时钟 0、去掉 `slowA`；同装置对照是 C14 `display_case_hit` / `display_case_miss`；登记 M85「显示名逐字节比」，杀手 `display_case` 与 C14 `display_case_hit`；白盒另翻的 `@ opus-5.0- 看一下`、`@ opus-5.0-maxx`、`@ opus-5` 写进 §7.3 第 9 条，方向是不花钱，判据不再改。原型 `ch1_v17` 上的实跑与方案 (a) 在 v1.6 原型上的变体结果一致：五个用例里只翻 `display_case`，上面几条白盒探针照样翻成算。下面方案 (a) 末尾「非 ASCII 字母的显示名没测」一句 v1.7 已测，结果写在 §7.1 第 12 条与 §7.3 第 9 条。以下是 v1.6 的登记原文，原文写的「待主代理」已不适用。） 裁决书第 1 条写明显示名「逐字节比，不折大小写」，本版照裁决书，判据不改。W1d 的判断：同一个函数里 id 那一支折小写（`@ SMX_B 看一下` 是 `STRAY`，C3 `space_upper`），显示名那一支逐字节比；用户冲着配置里的显示名去、只是大小写不同，按收口规则算第 (1) 类（冲着认识的成员去却问了全员，花钱），v1.5 的 §7.3 第 9 条把它按裁决书第 5 条归成 minor 不对。影响面：只在显示名含 ASCII 字母、而且 `@` 与名字之间隔着分隔符时；不隔的 `@Opus 看一下` 由 ParseMentions 取出 token `opus`，走「点的不是成员」、不花钱（白盒：ParseMentions 返回 `opus`，`hasStrayAt` 不算）。Griffin 的显示名是不是 ASCII 本刀没核（约束不许读 `~/.council`）。现状钉在 C3 `display_case`（v1.6 新加：`@ 坏 x 看一下`，smx_x 的显示名是「坏 X」，`smxS1Want`、时钟 2、`slowA`；照 v1.4 `angle_template` 的先例只钉现状、不证明它对，没有登记变异）。两个方向：
   - (a) 显示名改成大小写不敏感的前缀比：§1.1 E 的 `if strings.HasPrefix(text[j:], name) {` 换成 `if len(text)-j >= len(name) && strings.EqualFold(text[j:j+len(name)], name) {`。原型 `ch1_fix5`（v1.6）上作为变体实跑（从字节副本还原，还原后 SHA256 相同）：C3 / C5 / C11 / C13 / C14 的 98 个子测试里只有 `display_case` 变红（它变成 `STRAY`、时钟 0，`smxRequireBBeforeA` 前提不成立），其余照绿；白盒探针（ASCII 显示名配置 `Opus`、`Opus Max`、`Grok 4.7`、`GPT 5.6 Sol`、`甲 乙`）：`@ opus 看一下`、`@ OPUS 看一下`、`@ opus max 看一下`、`@ grok 4.7 看一下` 翻成算，另有 `@ opus-5.0- 看一下`、`@ opus-5.0-maxx`、`@ opus-5` 也翻成算（显示名 `Opus` 不分大小写时是这几段的前缀，属于 §7.3 第 9 条「按前缀比、不看边界」那一类，往谁都不问那一侧偏，不花钱），其余探针不变。采纳时：`display_case` 的期望改成 `STRAY`、时钟 0、去掉 `slowA`（从 §6 第 2 步的 `-count=10` 清单拿掉）；加一条变异「显示名逐字节比」（换回 `strings.HasPrefix`，杀手 `display_case`）；§1.1 E 注释、冻结要点 (c)、§7.1 第 12 条、§7.3 第 9 条同步，原型重跑。`strings.EqualFold` 按 Unicode 的简单大小写折叠比，不只 ASCII；中文显示名没有大小写，不受影响；非 ASCII 字母的显示名没测。
   - (b) 维持逐字节比：大小写不同就不是配置里那串显示名，这一条按收口规则降为 minor，§7.3 第 9 条的归类理由写成「敲出来的不是配置里那串显示名」，`display_case` 照旧。

### 7.3 待 Griffin（都先取了保守读法，只登记）

1. **（v1.1 F7）只有 judge 的会话、不点名**：名单是 [judge]，PlanRound 把他归进 Judges，每句裸文本都打「裁判 X 本命令不问，要用 --with-judge 才问。」与「这一轮没有人可问。」、调一次时钟、不起进程（W1 实测），而不是 §3.2 第 2 句那种「用 /add 加成员」的指引。要不要改成打第 2 句，等他定；本刀照 PlanRound 不改。
2. **（v1.1 F7）worker、manager 模式的成员也算会被问的成员**：PlanRound 把 ModeDiscussion、ModeWorker、ModeDiscussionAndWorker、ModeManager 都收成发言者（`internal/turn/plan.go:153`，读代码，未实测），所以对话模式没点名时这几种模式的成员每句话都会被问；#47 的原话是「全部讨论成员」。他今天的成员全是 discussion，要不要只问 discussion 模式，等他定；本刀照 PlanRound 不改。
3. **（v1.1）judge 与「没配总结成员」两行每句话都打**：v1.1 主代理裁定维持（§7.1 第 4、5 条）；他真机用过觉得吵再说。
4. **（v1.1；v1.2 按 W1b F4 写全）`@` 或 `＠` 左边紧贴 ASCII 字母、数字或 `_ @ . -` 的写法仍问全员**：按 ParseMentions 的左边界这个 `@` 不能起头，`hasStrayAt` 也跳过它，既不算点名、也不算没认出的点名，照问全员，是花钱方向的残余。原型实测（成员 smx_a、smx_b，都问了全员）：`hi@smx_b 看一下`（C3 `glued_at`）、`看一下.@smx_b`、`看一下-@smx_b`、`看一下_@smx_b`、`hi＠smx_b 看一下`（C3 `glued_fullwidth`）、`第2＠smx_b 看一下`，以及 `发到 x@甲 就行`（C3 `glued_left_cjk_tail`）；v1.3 在原型 `ch1_v13` 上逐条重跑，结果不变（收窄只看 `@` 右边，这一条是左边界）；v1.5 按成员判也只看 `@` 右边，C3 `glued_at`、`glued_fullwidth`、`glued_left_cjk_tail` 在原型 `ch1_v15` 上照旧问全员。用户只想问一个人，却按全员付费。放宽左边界就会把 `a@b.com`、`git@github.com:foo/bar.git` 这类当成点名（后者原型实测今天照问全员），本刀不动；§3.2 两句只给「前面空一格或在行首」这条一定对的写法。要不要处理，等他定。
5. **（v1.1；v1.2 写成实测清单；v1.3、v1.4 分两组补全；v1.5 按成员判重跑）不是点名的 `@`**：v1.2 下这些都算「没认出的点名」、谁都不问；v1.3、v1.4 只有一部分回到问全员（两组清单见 v1.4 快照 `ch1_contract_v14.md` 的这一条）。v1.5 按成员判之后在原型 `ch1_v15` 上重跑（成员 smx_a、smx_b，简洁档，问全员的用 `slowA`，自动回复打开，都是端到端）：
   - **问全员**（输出 == `smxS1Want`、时钟 2，花钱方向，Griffin 要的方向）：C3 钉着的 `ps_hashtable`、`ps_array`、`sql_identity`、`backtick_at`、`at_everyone_cn`、`price_at`、`stray_eot`、`stray_newline`、`stray_crlf`、`numpy_matmul`、`bash_dollar_at`、`ps_herestring_close`；另有探针 `C = A @ B.T`、`写成 @"`（换行）`多行`（换行）`"@ 对吗`、`$s = @"`（换行）`hello`（换行）`"@ | Out-File a.txt`、`$(CC) -o $@ $^`（行首不带 tab）、`f /@ list`、`https://medium.com/@张三/post 看看`、`@全体成员 看一下`、`联系 name @ example.com`。与认出的点名混用时不再误报第 4 句：`@smx_b y = A @ x` 只问 smx_b（`ONE_B`、时钟 2）。
   - **仍谁都不问**（第 3 句、时钟 0）：全角 `＠` 后面不是成员的：`＠所有人 看一下`（C3 `fullwidth_everyone`）、`看一下 ＠所有人`（C13 `stray`）、`价格 ＠ 5 元`。全角不按成员判是裁决书定的；与半角的不对称见第 9 条。
   仍谁都不问的这一组，出路是第 3 句写在屏幕上的 `/round`（C13 `stray` 实测）。`@所有人` 问全员只是因为它不冲着任何成员，本刀不加「`@所有人` / `@all` 就是问全体」的语义。v1.4 登记的 here-string 收尾、`$@`、`/@`、矩阵乘这几类（原 §7.2 第 10、11 条）都在上面「问全员」一组里，已关。v1.6（W1d 修订）放宽了跳过的字符之后，在原型 `ch1_fix5` 上把「问全员」一组的白盒探针重跑一遍（`C = A @ B.T`、`echo $@`、`$h = @{ Name = 'x' } 这行为什么报错`、`SELECT @@IDENTITY 返回空`、`https://medium.com/@张三/post 看看`、`@全体成员 看一下`、`y = A @ x + b`、`价格 @ 5 元`、`@所有人 看一下`、`@ 看一下`），仍都不算；另查了会被新规则跳过的非 ASCII 字符后面不是成员的写法：`价格 @ ￥5`、`@（笑）`、`@……`、`邮件 @：见附件`、`@“所有人” 看一下`、`@ 【公告】 看一下`、`@所有人，smx_b 也看一下` 都不算、问全员（其中 `价格 @ ￥5`、`@（笑）`、`@所有人，smx_b 也看一下` 端到端 `smxS1Want`、时钟 2；最后一种另由 C14 `letters_stop` 钉着：汉字不跳，`@所有人` 后面的 `smx_b` 不算点名）。v1.7（显示名不分大小写）在原型 `ch1_v17` 上把上面两组白盒探针（`smxCfg` 的 known 与 names）重跑一遍，结果都不变、仍不算。
6. **（v1.2，W1b F2 查到，v1 就有）解析得出 id 的代码写法是「点的不是成员」，同样谁都不问。已定（主代理，`ch1_rulings_v13.md` 第 2 条）：加出路，判据不改，v1.3 落实**：§0 第 11 条 (a) 那一支。原型实测（成员 smx_a、smx_b，时钟 0；v1.3 在 `ch1_v13` 上重跑）：`这段代码有什么问题`（换行）`@Override`（换行）`public void run() {}`、`@property`（换行）`def x(self): pass`、`npm i @types/node 报错`、`@all 看一下`，输出都是一行「<tok> 不是当前会话的成员」（tok 依次是 `override`、`property`、`types`、`all`）加 §3.2 第 1 句。v1.3 给第 1 句末尾加了与第 3 句逐字相同的出路「；这条本来就不是点名的话，接着敲 /round <成员 id...> --go 让成员回复」（C13 `none` 实测成员读到的正是那一条）。判据不改：一个敲错的成员 id 与代码里的 `@override` 分不开，敲错时问全员是花钱方向的误判。`@all` 在 IM 里是「问全体」，这里仍谁都不问，不加这种语义。v1.4（W1c F4）按「同类的别处」在原型 `ch1_fix3` 上补测，都是「<tok> 不是当前会话的成员」加第 1 句、谁都不问：PowerShell splatting `Get-ChildItem @params`（端到端：`params 不是当前会话的成员` 加第 1 句、时钟 0）、`Invoke-Foo @PSBoundParameters`、`$p = @{ Path = 'x' }`（换行）`Get-ChildItem @p`（v1.2 下另多一行第 4 句，v1.3 起 `@{` 不算没认出，只剩 `p 不是当前会话的成员` 加第 1 句）；Python 装饰器 `@staticmethod`（换行）`def f(): pass`（端到端，同上）、`@torch.no_grad()`（换行）`def f(): pass`、`@dataclass`（换行）`class P: pass`；批处理的 `@echo off`；CSS 的 `@media screen { }`；Perl 的 `print @ARGV;`；Julia 的 `@time f(x)`；MATLAB 的 `fzero(@myfun, 1)`；JSON-LD 的 `{"@context": "x"}`；IM 的 `@here 看一下`（后面几条是白盒分桶）。贴 PowerShell 的读者容易以为 PS 都问全员：`@{`、`@(` 问全员，splatting 谁都不问；贴 Python 的装饰器也谁都不问。README 那句写明（§6 第 4 步），出路都是第 1 句末尾的 `/round`。v1.5（裁决书第 4 条：这一支不改）在原型 `ch1_v15` 上重跑探针：`Get-ChildItem @params`、`@staticmethod`（换行）`def f(): pass`、`@all 看一下`、`$p = @{ Path = 'x' }`（换行）`Get-ChildItem @p` 都是「<tok> 不是当前会话的成员」加第 1 句、时钟 0（端到端）；第 1 句的出路改成不带尖括号的说法。写给 Griffin 的说明见第 8 条。
7. **（v1.3 登记；v1.4 写全；v1.5 翻过来）`@` 后面紧跟一个不是空白的 ASCII 字节、又取不出合法 id 的点名写法**：v1.3、v1.4 下这一类问全员（`@-smx_b`、`@(smx_b)`、`@"smx_b"`、`@<smx_b>`、`@**smx_b**`、`` @`smx_b` ``、`@_smx_b`、`@.smx_b` 等 29 种，清单见 v1.4 快照），与认出的点名写在一起时静默少问一个。v1.5 按成员判：跳过这些字节之后是成员，就是没认出的点名，谁都不问、打第 3 句（C3 `stray_bad_id`、`paren_id`、`angle_template`、`bold_id`、`backtick_id`、`double_at`；探针 `@_smx_b 看一下`、`@.smx_b 看一下`、`"@甲" 看一下`、`＠（smx_b）看一下` 端到端都是第 3 句）；与认出的点名写在一起时打第 4 句、照问认出的（C3 `partly_ascii`；`@smx_b @<smx_a>`、`@"smx_a"`、`@-smx_a`、`@_smx_a` 端到端都是 `PARTLY` + `ONE_B`）。v1.5 这里写的「这一类的花钱代价与静默少问都没有了」只对 ASCII 字节成立（W1d 修订 F1）：`@` 后面先跟一个非 ASCII 的分隔符（全角括号、引号、冒号、逗号、全角空格 U+3000、NBSP、零宽字符）再跟成员 id 或显示名时，v1.5 只跳 ASCII，这句问全员加一次总结，与认出的点名混用时静默少问一个；这是相对 v1.4 的回退（v1.4 快照这一条的「对照（不花钱）」清单里这些都是第 3 句）。v1.6 跳非 ASCII 的空白、标点、符号、组合记号、格式字符（§7.1 第 12 条），在原型 `ch1_fix5` 上重跑：W1d 列的 19 种单独写法、2 种 ASCII 显示名配置下的写法与 4 种混用写法白盒全部翻成算（v1.5 原型上同一份探针全部不算）；端到端是 C3 `fw_paren_id`、`ideo_space_id`、`zwsp_id`、`ideo_space_display`、`partly_fullwidth` 与探针 `@“smx_b”看一下`、`@`NBSP`smx_b 看一下`、`@（甲）看一下`（都是第 3 句、时钟 0）、`@smx_b @`全角空格`smx_a 看一下`、`@smx_b @`零宽空格`smx_a 看一下`（`PARTLY` + `ONE_B`）。按「同类的别处」另查了 v1.4 同样算第 3 句的全角符号与 emoji：`@＜smx_b＞`、`@～smx_b`、`@＋smx_b`、`@￥smx_b`、`@｀smx_b｀`、`@` 加 U+1F600 加 `smx_b`、`@` 加 U+2764 U+FE0F 加 `smx_b`，v1.5 都不算、问全员，v1.6 都算（白盒；端到端 C3 `fw_angle_id`、`emoji_vs_id`）。剩下的：这类写法后面跟的不是成员时（`@(x)`、`@{ Name = 'x' }`）照问全员，那是贴进来的代码，是 Griffin 要的方向。
8. **（v1.5，裁决书第 4 条）「点的不是成员」那一支不改，写给 Griffin**：贴进来的代码里的 `@params`（PowerShell splatting）、`@staticmethod`（Python 装饰器）、`@Override`、`@all` 解析得出 id，现在谁都不问：屏幕上先打「<tok> 不是当前会话的成员」，再打第 1 句，第 1 句末尾给了 `/round` 出路（第 6 条实测）。要改成问全员，代价是敲错 id 也会问全员（`@grok-4.8` 这类敲错的成员 id 与 `@staticmethod` 在判据上分不开；#47 记下的主代理裁定是敲错 id 不该问全员）。另记一处看得见的不对称：`@所有人` 问全员（C3 `at_everyone_cn`），`@all` 谁都不问（探针 `@all 看一下` 端到端：`all 不是当前会话的成员` 加第 1 句、时钟 0）。
9. **（v1.5 新登记；v1.6 按 W1d 改第 2 小条的归类、补第 6、7 小条；v1.7 第 2 小条已修，其余照裁决书第 5 条都算 minor）按成员判之后剩下的几处**（原型 `ch1_v15` 端到端，成员 smx_a、smx_b，简洁档；v1.6 的几处在原型 `ch1_fix5` 上重跑，v1.7 的几处在原型 `ch1_v17` 上重跑，写明了）：
   - 全角 `＠` 不按成员判：`＠所有人 看一下`、`价格 ＠ 5 元` 谁都不问（第 3 句），半角的 `@所有人`、`价格 @ 5 元` 问全员。不花钱，出路在屏幕上。
   - （v1.7 已修）显示名的大小写：v1.5、v1.6 逐字节比，smx_x 的显示名是「坏 X」，`@ 坏 x 看一下` 问全员加一次总结（W1d 定为第 (1) 类 major）；v1.7 不分大小写（§7.2 第 12 条定 (a)），这一句是第 3 句、时钟 0（C3 `display_case`），`@ 坏 X 看一下` 照旧是第 3 句。原型 `ch1_v17` 重跑：端到端 `@ 总结 s 看一下`、`@ 裁判 j 看一下` 都是第 3 句、时钟 0（v1.6 都问全员）；`@smx_b @ 坏 x 看一下` 是 `PARTLY` + `ONE_B`（v1.6 只有 `ONE_B`、不打第 4 句）；白盒 `@坏 x 看一下`、`@（坏 x）看一下`、`看一下 @ 坏 x`、`@ 坏 k 看一下` 从不算翻成算；ASCII 显示名配置（`Opus`、`Opus Max`、`Grok 4.7`、`GPT 5.6 Sol`、`甲 乙`）下 `@ opus 看一下`、`@ OPUS 看一下`、`@ opus max 看一下`、`@ OPUS MAX 看一下`、`@ grok 4.7 看一下`、`@ gpt 5.6 sol 看一下`、`@（opus）看一下` 从不算翻成算。不隔分隔符的 `@Opus 看一下`、`@Grok 4.7 看一下` 照旧由 ParseMentions 取出 token（`opus`、`grok`），走「点的不是成员」、不花钱。`strings.EqualFold` 不只折 ASCII（白盒）：`@ élan`（显示名 `Élan`）、`@ σοφία`（`Σοφία`）、`@ дима`（`Дима`）、`@ ａＩ`（全角 `Ａｉ`）都算；折叠前后字节长度不同的写法不算，与 v1.6 相同、问全员：`@`、空格、开尔文符号 U+212A（显示名 `k`），`@ STRAẞE`（`Straße`），`@ iſ`（`is`），`@ ıris`（`Iris`）。这几种敲出来的不是配置里那串原文，按收口规则算 minor。
   - 显示名按前缀比、不看边界：`@甲方 看一下` 以显示名「甲」开头，算冲着成员，谁都不问（第 3 句）。不花钱。v1.7 不分大小写之后这一类更宽（白盒，ASCII 显示名配置，原型 `ch1_v17`）：`@ opus-5.0- 看一下`、`@ opus-5.0-maxx`、`@ opus-5`、`@ opusx 看一下` 都以显示名 `Opus` 开头（不分大小写），算、谁都不问（v1.6 下前三种不算、问全员，见第 7 小条）；`@ op 看一下`、`@ grok 看一下`（显示名是 `Grok 4.7`）照旧不算。敲错的 id 恰好以一个显示名开头时不花钱，判据不再改（裁决书第 1 条）。
   - `@` 后面隔着空格跟一个敲错的 id：`@ smxb 看一下` 不冲着认识的成员，问全员（`smxS1Want`、时钟 2）；不带空格的 `@smxb 看一下` 是「点的不是成员」、谁都不问（端到端：`smxb 不是当前会话的成员` 加第 1 句、时钟 0；第 8 条）。花钱，但写法本身两处都错。
   - 代码里恰好有与成员 id 同名的标识符：`y = A @ smx_b` 算冲着成员，谁都不问（第 3 句）。不花钱。
   - （v1.6，W1d 修订 F3）显示名里的空格多敲、少敲，或 id 用全角字母敲出来，都对不上，问全员：`@ 总结S 看一下`（显示名「总结 S」，少一个空格）、`@ 总结  S 看一下`（多一个空格）、`@ｓｍｘ_ｂ 看一下`（全角字母）端到端都是 `smxS1Want`、时钟 2；白盒 ASCII 显示名配置里 `@甲乙 看一下`、`@ 甲乙 看一下`（显示名「甲 乙」）、`@ GPT  5.6 Sol 看一下`（显示名「GPT 5.6 Sol」，多一个空格）也都不算。正对照 `@ 总结 S 看一下`、`@甲 乙 看一下`、`@ GPT 5.6 Sol 看一下` 都算（白盒）。与 W1d 在 v1.5 上的结果相同（原型 `ch1_fix5` 重跑）。v1.7 在原型 `ch1_v17` 上重跑：`@ 总结S 看一下`、`@ 总结  S 看一下`、`@ｓｍｘ_ｂ 看一下` 端到端照旧 `smxS1Want`、时钟 2，白盒结果不变（不分大小写管不到空格与全角字母）。敲出来的不是配置里那串原文，按收口规则算 minor；不为此做模糊匹配。
   - （v1.6，W1d 修订 F4）`@` 后面隔着空格跟 id 再带尾部 `-` 或 `_`：`@ smx_b- 看一下`、`@ smx_b_ 看一下` 问全员（v1.6 端到端都是 `smxS1Want`、时钟 2）。取 token 照 ParseMentions 只剥尾点，`smx_b-`、`smx_b_` 不是认识的 id。不带空格的 `@smx_b- 看一下` 由 ParseMentions 认成 token `smx_b-`，打「smx_b- 不是当前会话的成员」加第 1 句、时钟 0、不花钱（v1.6 端到端）。同一个手误差一个空格，一个花钱一个不花；与第 4 小条的 `@ smxb` 同类（v1.6 端到端照旧 `smxS1Want`、时钟 2）。白盒 ASCII 配置里 `@ opus-5.0- 看一下`、`@ opus-5.0-maxx`、`@ opus-5` v1.6 下也不算，v1.7 起它们以显示名 `Opus` 开头（不分大小写），算、谁都不问（第 3 小条）；`@ opus-5.0-max 看一下`（`opus-5.0-max` 是配置 id）算，前缀关系本身没问题。v1.7 在原型 `ch1_v17` 上重跑：`@ smx_b- 看一下`、`@ smx_b_ 看一下`、`@ smxb 看一下` 端到端照旧 `smxS1Want`、时钟 2（smxCfg 里没有以 `smx` 开头的显示名），`@smx_b- 看一下` 照旧是「smx_b- 不是当前会话的成员」加第 1 句、时钟 0。按收口规则算 minor，判据不改。
