# RB-B 冻结契约：成员行进活区 + 只长不缩 + 段终态块进回滚区 + 收尾行（v1.1，2026-09-24）

> **归档说明（2026-09-25）**：这是 RB-B（#41 第 3 刀）派工时的冻结契约原文（v1.1），从中转根 `E:\multi_model_cowork\rb_b_contract.md` 原样归档。文中行号都指 HEAD `a728cee`；落地提交是 `572467c`。文中的 `E:\` 路径是当时的中转与隔离目录，已清理。
> **落地后的订正（独立审查，正文未改，以这里为准）**：
> - 「唯一缩高点在 finalize」「floor 只在收口时归零」只在正常路径上成立。出错或 IsQuit 退出时，submitDone 返回的 Sequence 两次 Send 之间若进来一次 Enter，beginSubmit 会开新提交、在没钉光标的情况下把 floor 清零（RB-A 起就不挡这个间隙的 Enter）。§4 等价清单第 2 条「随后退出」的论证因此不严密。代码注释已补例外；要修得单独一刀给退出中的状态加门。
> - §1.1 D1 的写时拷贝在 RoundStart 路径上没有用例钉住：把新段的看板改成复用旧数组（`m.rows[:0]` 再 append）全绿。实现用的是新分配，正确；补法是 B1-7 前后存 `saved2` 断言不变，并列为变异 #67（~~未做~~ **已补，2026-09-25 `6797ce4`**；同类的四个缺口由 `rbb_cow_test.go` 补上，`1a85a03`，见 `00_DECISIONS.md` #41 (i)）。
> - 等价清单补三条（审查员实测存活、判等价）：MD 找行时去掉 `!done`（名单已去重）；finalize 不清 `runSeen` 与两个累计数（D12 在下一次 beginSubmit 清）；`rosterN` 按 `Members` 累加（与 `len(Speakers)` 恒等）。
> - **§2.5 只写了 Windows 上 `\n` 的语义**（2026-09-25 补）：非 Windows 上 bubbletea 的 mapNl 为真（`WithInput(nil)` 时），渲染器假定终端把 `\n` 映射成 `\r\n`，模拟器按 `rbbRendererMapsNL` 照样解释（`d589334`）。
>   Linux 上复核：§4 点名给 G 系列的 #46、#50、#51、#55、#62、#66 都照样被杀；两处出入：#50 的「公式残影 2 每个场景至少 9/10」在 Linux 的 G2、G3 只有 5-8/10（随时序变，不影响杀）；
>   #55 在 Linux 上只由 `exitClearSeq` 整串比较杀掉，屏幕是干净的。详见 `00_DECISIONS.md` #42 (a)。


> v1.1 相对 v1 的改动（派工前 W1：审查员照契约忠实实现、实测残影、跑 66 条变异加 13 条自拟变异；1 条 major 经证伪者降为 minor，另 3 条 minor）：K4 的负数边界 -5 → -1500（-5 杀不到 #22）；#15 杀手改为 B1-6 与 H1（B1-6 的 RE 带一项 Results）；#2、#51 的杀法说明按实测改写；等价清单补两条；F.3 已知代价补上 W1 的实测范围。生产代码的要求一个字没变。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `a728cee`（RB-A 实现 `69fed6c`，补 T4b 与文档 `dd24a61`）。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`，bubbletea `charm.land/bubbletea/v2 v2.0.9`，ultraviolet 伪版本 `v0.0.0-20260811164956-006e29f97886`，`github.com/charmbracelet/x/ansi v0.11.8`（go.mod 里已有，标 `// indirect`）。本机没有 gcc，不能 `-race`。
- 来源：`docs/00_DECISIONS.md` #41（1927-1994 行）分刀里的**第 3 刀**，含「RB-A 落地记录」「RB-A 剩余项」「第 3 刀（RB-B）前置探针」三段；`docs/15_TUI_SPEC_MAPPING.md` §3.1（96-105 行的订正与进展）；探针原始数据 `E:\multi_model_cowork\rb_o2_probe_result.json`（R1-R11）；渲染约束报告 `E:\multi_model_cowork\rb_report_render.md`；Griffin 规格 §7（`rb_tui_spec_griffin.md` 253-260 行，「每次多成员调用在时间线里是一组运行」）。
- 本刀做：成员行进活区（表头、横线、成员行、输入框）；一次提交内只长不缩的高度策略；各段收轮时把终态块打进回滚区；无错收尾时打一行收尾行、钉光标、唯一一次缩高；VT 回放守卫钉住残影为 0。**不做**：字形（第 4 刀）、展开折叠、快捷键、Compare view。
- 本契约的全部期望值（活区各时刻的行、回滚块与收尾行的逐字节文本、真 `tea.Program` 上的 Update 顺序、RB-A 与 X-39e 既有用例在原型上的结果、VT 回放下的残影与变异结果）都已在隔离拷贝 `E:\Isolating_test\council\rb_b_draft\` 里用一份原型实现实跑得出；原型不交付，已删除。原型在全部既有用例上的结果：`./internal/tui`、`./internal/repl` 两包全绿（含 RB-A 的 T1-T8、T4b、O1-O4、W1、R1-R6，X-39e 全部，TUI-1b 全部），前提是 §1.2 授权的 15 处编译修正。

## 与 RB-A 的关系（逐条点名，RB-A 契约原文见 `docs/15a_RUN_BOARD_RBA_CONTRACT.md`）

被本刀**改写或解冻**的 RB-A 冻结条款：

| RB-A 条款 | 原文要点 | RB-B 处置 |
|---|---|---|
| §1.1 A（`observer.go` 逐字声明） | `RoundStartEvent{Label,K,N,Members}`、`RoundEndEvent{Label,K,N}` | 两个结构体末尾**追加**一个字段（§1.1 A）。原字段、顺序、类型不动。代价：两者含切片，**不再可比较**，见下面 §1.2 的 15 处编译修正 |
| §1.1 C3 / C5（事件字面量） | `RoundStartEvent{Label: label, K: k, N: n, Members: len(plan.Speakers)}`；`RoundEndEvent{Label: label, K: k, N: n}` | 各多一个字段（§1.1 B）。发射点位置、`obs != nil` 门、C2 的「读一次」、C4 的 MemberDone 一个字不改 |
| §1.1 D（`RoundEnd` 方法体「一句 `o.send(roundEndMsg{ev: ev})`」）与 G.1（「适配器每个方法体只有一句 `o.send(...)`」） | 适配器三个方法各一句 | **只改 `RoundEnd`**：先逐行 `o.send(printLine(块的一行))`，再 `o.send(roundEndMsg{ev: ev})`（§1.1 C）。仍不持锁、不读写共享状态、不调 REPL；`RoundStart`、`MemberDone` 两个方法体不动 |
| §1.1 E6（四种消息的分支） | `roundStartMsg` / `memberDoneMsg` 只改表头材料；`roundEndMsg` 不改任何状态 | `roundStartMsg`、`memberDoneMsg` 两支**追加**看板状态（§1.1 D2、D3）；`roundEndMsg` **仍不改任何状态、仍返回 nil**；新增 `boardFinalizeMsg` 一支（D4），同样在进 textarea 之前返回、Cmd 为 nil |
| §1.1 E7（「submitDone 的 Cmd 一个字不变」） | 三支 status 文字与 Cmd 不变 | **只在「本次提交收到过带名单的 RoundStart」且 err 为 nil 时解冻**（D5）。其余情形（/ask、早退的 /round、RB-A 用例那种不带名单的事件、出错、IsQuit）与 RB-A 逐字节相同 |
| §1.1 E8（`chrome()` 仍然只返回两行） | 表头、横线 | 返回表头、横线、再加 0 到 floor 行看板行（D9）。`frameHeight` 函数体不改，帧高随 chrome 变 |
| §1.1 E9（Ctrl-C、Enter 的现有行为一个字不改） | | finalize 窗口里 Ctrl-C 与 Enter 被挡（D6、D7）；忙时 Ctrl-C 额外置一个取消标志（D6）。其余情形不变 |
| §1.2「不改活区高度、不加成员行」 | | 作废（这正是本刀） |
| §1.2「既有 `_test.go` 一个都不许改」 | | 本刀授权**恰好 15 处**编译修正（§1.2），不改任何断言的语义 |
| E4 的推论「第 1 次真提交代次 1，第 2 次 2」 | | 只在两次提交之间没有 finalize 时成立：finalize 会 `tickGen++`（D4），之后的提交代次多跳 1。没有任何既有用例依赖跨 finalize 的代次（原型实测） |
| RB-A 剩余项 (d)「RoundStartEvent 不带成员名单」 | | 本刀解决：`Speakers` 是新分配的切片（§1.1 A、B），由 RB3 担保 |
| RB-A 剩余项 (e)「被杀的路上 Submit 可能在 Run 返回后继续调观察者，RB-B 若要在收轮时 Println 要重新论证」 | | 重新论证见 §1.1 F.4：块的行经 `p.Send`，Run 返回后是空操作，**块丢失**（REPL 自己的明细与结算经 LineWriter 第 3 步照样落到 stdout）；收尾行由 Update 返回，Run 返回后 Update 不再跑，**收尾行也不打**。都不阻塞、不 panic。记为已知代价 |

**不动**的 RB-A 条款：C1、C2、C4、C6（行式输出逐字节不变）、C7 的既有注释、D 的另外两个方法与 `formatElapsed`、E1-E5、F（`run.go` 装配那一行逐字不变）、G.2-G.5、§3.1 表头格式一个字不改、§3.2、§3.4 golden。X-39e：`exitClearSeq`、`runWith` 一个字不改；「帧高 8」只在没有看板行时成立（D9 之后帧高 = 2 + 看板行 + 6）。

---

## 0. 病根 / 动机（行号以 HEAD `a728cee` 为准）

1. **表头只有总览，看不见谁卡住。** RB-A 的表头（`internal/tui/roundobs.go:61-83` 的 `headerStatus` / `segmentCell`）只给「发言轮 1/2 (688558)  回来 2/3  12s」。三个人里哪一个没回来、回来的那个是超时还是被拒、有没有付了钱没入库，都要回滚区翻明细。Griffin 规格 §7 与 #41 D1 要的是「每成员一行：状态、名字、用时」。
2. **数据通道缺名单与结果。** `internal/repl/observer.go:21-26` 的 `RoundStartEvent` 只有人数 `Members`，没有名单（RB-A 剩余项 (d)）；`:41-45` 的 `RoundEndEvent` 没有本段结果。TUI 适配器要在 RoundEnd 时无状态地拼回滚块，只能从事件拿数据。`round.go:323` 的 `RunRoundPreparedStream` 返回的 `recs` 已经按 `plan.Speakers` 的顺序排好（`internal/turn/scheduler.go:35`「返回顺序跟 plan.Speakers，不按完成顺序」），`round.go:373` 的结算就用它。
3. **缩高会留残影。** 活区从 8+N 行缩回 8 行的那一次 flush，若此前渲染器光标停在新帧高以下，旧帧顶部留在屏幕上（残影行数 = max(0, 缩高前光标相对帧顶的行 - 7)，`rb_report_render.md` 第 5-6 条，8 组数据全吻合）。探针（`rb_o2_probe_result.json`）判定唯一按构造安全的做法是 C6：无错收尾那一次 Update 钉光标 + `tea.Sequence(Println(非空收尾行), finalize)`，finalize 是唯一缩高点；实测 3240 次残影全 0，去掉光标钉或去掉 Println 都会脏。
4. **帧高必须 ≤ 终端高 - 1。** 否则 Println 把帧顶卷进回滚区，帧比终端高时每秒 60 帧整屏重画（探针 e2：71/120 有残影、约 11 KB/300ms）。看板行数上限 = 终端高 - (2 + 输入框高) - 1。
5. **看板行必须进 `chrome()`**（`internal/tui/model.go:311-321`，View 与 `frameHeight` `:325-327` 共用），否则 X-39e 的退出清屏（`run.go:153-169` 的 `exitClearSeq`）少擦。每行必须单行：用 `ansi.StringWidth` 补齐、`ansi.Truncate` 截断，不用 lipgloss `Style.Width`（会把长中文名折成多行，`rb_report_render.md` 第 18 条）。
6. **REPL 还在写的时候，进回滚区的行只能经 `p.Send(printLine)`**，不能由 Update 返回 `tea.Println`（Cmd 各起 goroutine，与 LineWriter 的行不保序，#41 硬约束）。收尾行是唯一的例外：submitDone 时 REPL 已经写完（`submitCmd` 的 Submit 在同一个 goroutine 上先把全部行 Send 完，才返回 submitDoneMsg）。

---

## 1. 范围

### 1.1 做

凡写「名字实现方定」处实现方可自取名字（不许以 `rbb` 或 `rba` 开头）；其余名字、签名、字面量、顺序全部冻结。

#### A. `internal/repl/observer.go`

两个结构体整体替换成下面这样（逐字；字段注释对齐由 gofmt 决定，原有四个字段的注释文字不改）：

```go
// RoundStartEvent 在打出「本轮问 N 个: ...」之后、起进程之前发出。
type RoundStartEvent struct {
	Label    string   // "发言轮" 或 "裁判轮"，即 runOneRound 的 label 参数
	K        int      // 发言轮：第 K 轮（1 起）；裁判轮：0
	N        int      // 发言轮：本条命令的 --rounds 总数；裁判轮：0
	Members  int      // len(plan.Speakers)，skipNoDelta 之后、真正起进程的人数
	Speakers []string // plan.Speakers 的新分配拷贝，顺序即起进程的顺序；len 恒等于 Members，不与 plan 共享底层数组
}
```

```go
// RoundEndEvent 在本段结算文本打完之后发出。
type RoundEndEvent struct {
	Label   string
	K       int
	N       int
	Results []MemberDoneEvent // 本段每个成员的结果，新分配；按 Speakers 的顺序（即 turn 返回 recs 的顺序），不按回来的顺序
}
```

`RoundObserver` 接口、`MemberDoneEvent`、`SetRoundObserver` 一个字不改。接口上方注释里「事件是值，接收方可以留着用，与发射方不共享任何可变内存」这一句保留，现在它由「两个切片都是新分配」撑着。

#### B. `internal/repl/round.go`

- **B1 开轮字面量。** `:304` 逐字改成
  `obs.RoundStart(RoundStartEvent{Label: label, K: k, N: n, Members: len(plan.Speakers), Speakers: append([]string(nil), plan.Speakers...)})`
  必须是新分配：`plan` 在 skipNoDelta 没跳过任何人时就是 `cmdRound` 的那一份（`skipNoDelta` 在 `:647-649` 原样返回），它的 `Speakers` 随后原样交给 `RunRoundPreparedStream`（`:323`）并在第 2 到第 N 轮复用。直接传 `plan.Speakers` 时，观察者改一个元素就会改掉真正要问的人（原型实测：变异后第 1 段问到一个叫 `rbb_zz` 的人、判 refused，第 2 段只剩 1 人）。
- **B2 收轮字面量。** `:376-378` 整段逐字换成：

  ```go
  	if obs != nil {
  		results := make([]MemberDoneEvent, 0, len(recs))
  		for _, rec := range recs {
  			results = append(results, MemberDoneEvent{
  				Label:         label,
  				AgentID:       rec.AgentID,
  				Outcome:       rec.Outcome,
  				MaybeAdvanced: rec.MaybeAdvanced,
  				Drifted:       rec.Drifted,
  				Dropped:       rec.DroppedText != "",
  				WallMS:        rec.WallMS,
  				RoundID:       rec.RoundID,
  			})
  		}
  		obs.RoundEnd(RoundEndEvent{Label: label, K: k, N: n, Results: results})
  	}
  ```
  局部变量名 `results` 可由实现方改。**不许**在 onDone 闭包里按回来的顺序累积（那是回来顺序，且要在闭包里加状态）；**不许**把 C4 的 MemberDone 字面量抽成共用函数（RB-A 冻结了 C4 的写法）。映射与 C4 逐字段相同，所以 `Results` 里某成员那一项与该成员的 MemberDone 事件 `reflect.DeepEqual`（RB1、H 系列据此断言）。
- **B3 行式不变。** 观察者为 nil 时本函数与 HEAD 逐字节同输出；观察者非 nil 时屏幕文本也与 HEAD 逐字节相同。RB-A 的 R1、R2 必须原样绿（原型实测绿）。
- **B4 注释。** `runOneRound` 文档注释（`:249-256`）补一句：RoundStart 带名单的新分配拷贝、RoundEnd 带按名单顺序的本段结果（取自 `recs`，turn 保证它与 `plan.Speakers` 同序），供 TUI 无状态地拼回滚块。措辞实现方定，不许写与本契约不符的话。
- 不改 `cmdRound`、`writeAskRecord`、`renderSettlement` 以及发射点以外的任何一行。

#### C. `internal/tui/roundobs.go`

- **C1 `RoundEnd` 方法体**逐字换成（`:31` 那一行）：

  ```go
  func (o roundObserver) RoundEnd(ev repl.RoundEndEvent) {
  	for _, line := range <块函数>(ev) {
  		o.send(printLine(line))
  	}
  	o.send(roundEndMsg{ev: ev})
  }
  ```
  `<块函数>` 名字实现方定，签名 `func(ev repl.RoundEndEvent) []string`，纯函数。`RoundStart`、`MemberDone` 两个方法不动。
- **C2 块函数的行为（冻结）。** `len(ev.Results) == 0` 时返回 nil（于是只发一条 `roundEndMsg`，RB-A 的 T6 因此原样绿：它构造的 RoundEnd 事件不带 Results）。否则返回 `1 + len(ev.Results)` 行：
  - 第 1 行 = 段标题 = RB-A 的 `segmentCell(roundRun{has: true, label: ev.Label, k: ev.K, n: ev.N, members: len(ev.Results), back: len(ev.Results), roundID: rid})`，`rid` 取 `ev.Results` 里**第一个非空** `RoundID`（全空时为空串）。与表头的段同一个函数、同一种格式（§3.1）。
  - 其后按 `ev.Results` 的顺序每人一行，格式见 §3.2 的「终态行」，其中 id 列宽 = `ev.Results` 里全部 `AgentID` 的 `ansi.StringWidth` 最大值。**块的行不截断**（它进回滚区，insertAbove 自己按终端宽折行计数）。
- **C3 新类型（名字冻结）：** `type boardFinalizeMsg struct{}`，放在本文件或 `model.go`（实现方定）。测试腿按这个名字构造与识别。
- **C4 格式辅助函数**（状态词、时间格、标记、补齐）放在本文件，名字实现方定；块函数与 D9 的活区行必须调用**同一组**函数（两份格式会分叉）。
- 新 import：`strings`、`github.com/charmbracelet/x/ansi`、`council/internal/turn`（状态词按 `turn.Outcome` 分支）。不改 `go.mod`（`x/ansi` 已在 go.mod，`// indirect` 标记会变得不准，接受，不跑 `go mod tidy`）。`tui` 引 `turn` 与 `repl` 都不成环（两者都不引 `tui`）。

#### D. `internal/tui/model.go`

- **D1 字段。** `Model`（`:53-98`）末尾加字段，名字实现方定（下文反引号里的 `segStart`、`floor`、`runSeen`、`canceling`、`finalizing`、`pinCursor` 只是指代用的名字），语义冻结：
  - 看板行：当前段的名单与每人的状态（未回来 / 回来后的那条 `repl.MemberDoneEvent`），按名单顺序；
  - 本段起点 `segStart`（`time.Time`）；
  - `floor`（int）：本次提交里看板已占的行数，一次提交内**只增不减**；
  - `runSeen`（bool）：本次提交收到过「忙时、带非空名单」的 `roundStartMsg`；
  - 本次提交的两个累计数：回来总数、名单长度总和（int）；
  - `canceling`（bool）：本次提交里忙时按过 Ctrl-C；
  - `finalizing`（bool）：处于 submitDone 到 finalize 之间的窗口；
  - `pinCursor`（bool）：View 要不要钉光标。
  - **Model 是值类型**（TUI-1b 契约 5.1）：看板切片在改某一行之前**先拷贝**（copy-on-write），不许原地改旧切片（B1-4 用「早先保存的 Model 值渲染不变」钉住）。
- **D2 `roundStartMsg` 分支**（`:185-196`）：RB-A 的「`!busy` 忽略、当前段整体换掉」原样保留，其后追加：`len(ev.Speakers) > 0` 时——
  1. `runSeen = true`；名单长度总和 += `len(ev.Speakers)`；
  2. 看板行整体换成 `ev.Speakers` 各一行、全部「未回来」（上一段的行不保留，不追加）；
  3. `segStart` = 时钟()（`m.now`，nil 时兜底 `time.Now`，与 `beginSubmit` 同一写法）；
  4. 设 `want = len(ev.Speakers)`，`lim` = D10 的上限；**当且仅当 `want > floor && want <= lim` 时 `floor = want`**。否则 floor 不变（可能是 0：降级；也可能 > 0 而本段放不下：见 D9 的空行规则）。
  `len(ev.Speakers) == 0`（RB-A 的既有用例全是这种）时只做 RB-A 那部分，看板相关状态一个都不动。Cmd 仍为 nil。
- **D3 `memberDoneMsg` 分支**（`:198-206`）：RB-A 的「`!busy || !run.has` 忽略、back++、roundID 只取第一个」原样保留，其后追加：回来总数 += 1；在看板行里找 `id == ev.AgentID` 且未回来的**第一行**，拷贝切片后把那一行置为「回来」并存下 `ev`。找不到时看板不变。按 AgentID 找，不按回来的次序（B1-4：第 3 个人先回来，改的是第 3 行）。Cmd 仍为 nil。
- **D4 新分支 `boardFinalizeMsg`**（放在 `update` 的 switch 里，与 RB-A 四种消息同层，**不交给 textarea**，返回 Cmd 为 nil）：`!finalizing` 时什么都不改。否则一次做完：`finalizing = false`、`pinCursor = false`、看板行清空、`floor = 0`、`runSeen = false`、两个累计数清零、`tickGen++`。**这是唯一让 frameHeight 变小的地方。**
- **D5 `submitDoneMsg` 分支**（`:238-252`）：`busy = false`、`cancel = nil`、`run` 清空、IsQuit 支、err 支——**一个字不改**（err 支不清看板、不钉光标：看板行留在 View 里，frameHeight 把它们算进去，`exitClearSeq` 按 `min(8+看板行, 终端高)` 清）。原来的最后两句 `m.status = "空闲"; return m, nil` 换成：
  - `!runSeen`：`m.status = "空闲"; return m, nil`（与 RB-A 逐字节相同）。
  - 否则：先按 D11 算出收尾行（此时 status 还没改）；`m.status = "空闲"`；`finalizing = true`；`pinCursor = floor > 0`；返回 `tea.Sequence(m.println(收尾行), func() tea.Msg { return boardFinalizeMsg{} })`。
  必须用 `m.println`（`Options.Println` 注入的那个），不许直接调 `tea.Println`；必须 `tea.Sequence`、Println 在前，**不许 `tea.Batch`**。`busy` 在这里已经是 false，所以窗口里表头就是「council t   空闲」（RB-A 的 O1/O2「DONE 处表头 == 空闲」因此原样绿）；E5 的 tick 在这次 Update 不会并进来（处理后不忙）。
- **D6 Ctrl-C**（`:256-265`）：最前面加「`finalizing` 为真 → `return m, nil`」（挡住，不退出、不改 status）。忙时那一支在 `m.status = "正在取消"` 旁边加 `canceling = true`。空闲那一支不变。
  理由（选「挡住」而不是「先取消光标钉再退出」）：窗口只有 Sequence 里 Println 到 finalize 这一小段；挡住不改变任何退出字节，X-39e 的退出尾巴原样；若在钉着光标时退出，最后一帧带真光标，close 不再写 `?25h`、尾巴形状变（Q1 的变异实测：`?25h` 挪到换行之前、帧 11 行、清屏变 `ESC[10A`）。
- **D7 Enter**（`:271-276`）：`if m.busy {...}` 之后、`return m.beginSubmit()` 之前加「`finalizing` 为真 → `return m, nil`」（不提交、不改 status、不清输入框，用户在 finalize 之后再按一次）。理由：窗口里若允许提交，新提交的回显 Sequence 与上一条的收尾行 Sequence 是两个 goroutine，回显可能跑到收尾行前面；更不能在钉着光标、看板未清时开新一轮。
- **D8 `View`**（`:301-307`）：`v.AltScreen = false` 之后加：`pinCursor` 为真时 `v.Cursor = tea.NewCursor(0, 0)`。钉在 (0,0)（表头行首）而不是 (0,1)：insertAbove 结束时 `SetPosition(0,0)`（`cursed_renderer.go:801`），两者重合，窗口里的每次 flush 都以 `MoveTo(0,0)` 结尾（`:512-515`）。
- **D9 `chrome()`**（`:311-321`）：返回 `append([]string{header, rule}, 看板行...)`。表头与横线的拼法不变（RB-A §3.1）。看板行：
  - `floor == 0`：没有看板行。
  - 否则恰好 `floor` 行。若当前段名单长度 `<= floor`：前 `len(名单)` 行是每人一行（§3.2），其余是空串 `""`（垫行）；若当前段名单长度 `> floor`（本段放不下、又不许缩）：`floor` 行**全是空串**（表头仍有总览）。
  - 每人一行：未回来用「在跑行」、回来后用「终态行」（§3.2）；id 列宽 = 当前段名单全部 id 的 `ansi.StringWidth` 最大值；整行再 `ansi.Truncate(行, width, "")`，`width` 就是 `chrome()` 里已有的那个局部变量（`m.width`，小于 20 时取 20）。不加省略号（U+2026 是模糊宽度，`15` §13）。
  - `frameHeight` 函数体不改。
- **D10 上限**：`termHeight <= 0` 时上限为 0（终端高未知就不出看板）；否则 `termHeight - (2 + m.ta.Height()) - 1`。用 `termHeight` 不用 `height`：`height` 在 `WindowSizeMsg{0,0}` 之后被兜底成 24，会按一个假终端高出看板。上限只在 D2 长高时看；**窗口尺寸变化不改 floor**（包括变矮，见 F.3 的已知代价）。
- **D11 收尾行**（格式见 §3.4）：`"/round " + 动词 + "  " + formatElapsed(时钟() - 本次提交起点) + "  回来 " + 回来总数 + "/" + 名单长度总和`。动词：`canceling` 为真时 `已取消`，否则 `结束`。**用 `canceling` 标志，不看 status 文字**：忙时按 Enter 或粘贴会把 status 改成别的（B2-5 实测：Ctrl-C 之后再按 Enter，status 是「一轮正在跑…」，收尾行仍须是「已取消」）。时间取 submitDone 这一刻的时钟，不取最近一拍（B1-9：最后一拍在 65s、时钟在 125s，收尾行是 `2m05s`）。收尾行永远非空。
- **D12 `beginSubmit`**（`:330-362`）：在 `m.run = roundRun{}` 旁边把看板行、`floor`、`runSeen`、两个累计数、`canceling` 全部清空。`finalizing` 与 `pinCursor` 在这里必然已是 false（D7 挡住了窗口里的提交），不必清。返回的 Cmd 一个字节不变（RB-A E4）。
- **D13 import**：`github.com/charmbracelet/x/ansi`、`council/internal/repl`（看板行存 `repl.MemberDoneEvent`）。

#### E. 不改的文件

`internal/tui/run.go`（观察者装配那一行逐字不变，RB-A F 与 §4「只在审查时看」）、`writer.go`、`vt_*.go`、`internal/turn`、`internal/app`、`cmd/`、`go.mod`、`go.sum`。

#### F. 并发、顺序与高度（冻结的论证，实现方照此实现，不许加锁）

1. **块的行在该段结算文本之后、下一段预算表之前。** RoundEnd 在 Submit goroutine 上、结算 `r.printf` 返回之后调用（RB-A C5）；适配器在同一个 goroutine 上依次 `p.Send(printLine(...))` 块的每一行，再 Send `roundEndMsg`；LineWriter 的 emit 也是同一个 goroutine 上的 `p.Send`，都交给同一个无缓冲 `p.msgs`。所以事件循环收到的顺序是：结算各行 → 块各行 → roundEndMsg → 下一段的预算表。原型在真 Program 上实测：`发言轮结算` … `冷启动 预报 2  实测 2` → `发言轮 1/2 (3df5e5)  回来 2/2` → 两行成员 → RE → `成员  输入token  金额 …`。
2. **收尾行在全部 REPL 文本之后。** submitDoneMsg 是 `submitCmd` 在 Submit 返回之后才产出的，Submit 的每一行都已同步 Send 并在事件循环里 insertAbove 完；收尾行经 D5 的 Sequence 在 submitDone 那次 Update 之后才 Send。Sequence 逐条执行、Println 的 Send 被事件循环收下之后才执行 finalize（`tea.go:900-924`），所以 insertAbove（光标归 (0,0)）先于缩高。
3. **高度。** 一次提交内 floor 只在 D2 增长、只在 D4 归零；D5 err 支、IsQuit 支、窗口尺寸变化都不改它。唯一缩高点在 D4，且只在 D5 的无错路径之后到达。残影 = max(0, 缩高前渲染器光标相对帧顶的行 - 7)：钉住的 View 被 flush 过，光标在 0 行（`MoveTo(view.Cursor)`）；没被 flush 过，光标停在 insertAbove 留下的 0 行。两种都是 0（探针 C6 3240 次全 0；本刀原型在 G1-G3 上 30/30 为 0，外加一个不拉宽的 b2 场景 10/10 为 0，屏幕读数与公式读数一致）。
   **已知代价（不修，写明范围）：** 提交中途把窗口拉矮到 `termHeight < 9 + floor`（帧 8+floor 超过终端高 - 1）时，按「不许缩」帧高保持不变，进入「帧顶被裁、Println 把帧顶卷进回滚区、帧比终端高时每秒 60 帧整屏重画」的状态（探针 e2：约 11 KB/300ms；渲染报告 F3：回滚区里会留下表头片段），持续到本次提交的 finalize。finalize 那次缩高本身在 C6 下仍然干净（探针 e2 在 C6 下 120/120 残影为 0）。只有中途拉矮窗口才进入；启动时终端就矮的走 D2 的降级（0 行），不进入。真终端上窗口变矮时终端自己的重排（Windows Terminal 把顶行推进回滚区）VT 模拟器无法复现，本刀不测。W1 审查员用自定的 VT resize 语义（顶行推进回滚区）实测 8 人、80x24 起步、RS 之后拉矮（各 6 次）：终端高 17 全干净；终端高 16（= 8 + floor）不整屏重画，但回滚区多一条表头片段；终端高 14 约 14 KB/300ms（60fps 整屏重画），回滚区多一条表头与一条横线。18 次 finalize 之后屏幕上残影全为 0。即：整屏重画只在 `termHeight < 8 + floor` 时出现，`termHeight == 8 + floor` 时只在回滚区留一条表头片段。
4. **被杀的路（SIGTERM、根 ctx 取消）。** Run 返回后 `p.Send` 走 `p.ctx.Done()` 那一支，是空操作（`tea.go:1192-1196`）：此后到达的 RoundEnd 的块丢失、`roundEndMsg` 丢失；Update 不再运行，收尾行不打。都不阻塞、不 panic。REPL 自己的明细与结算经 `runWith` 第 3 步落到 stdout，账不丢。块只是索引，丢了接受。
5. **字节副作用（探针 R11，本刀原型复测一致）。** 只有当 finalize 窗口里恰好发生一次 flush（自然时序下几乎不发生；窗口拉宽到 40ms 时每次都发生）时，才多出 `ESC[1 q` 与 `ESC[?25h`（缩高时再出 `ESC[?25l` 与 `ESC[0 q`）；没有 OSC 12/112（`Cursor.Color` 为 nil）。`ESC[0 q` 会把用户自定义的光标形状复位成终端默认，真终端未测（§6 真机复验第 7 步）。正常退出的尾巴与 RB-A 相同（Q1 实测）。

### 1.2 不做（两条腿都不许越界）

- 不改 §1.1 E 列出的文件；不改 `turn` 包；不改 `repl.New`、`Submit`、`writeAskRecord`、`renderSettlement`、`cmdRound`。
- 不加字形（实心圆、半填圆、空心圆、乘号、对勾 U+2713 等一概不用，本文件不写出它们，免得被照抄）：本刀新增字面量只有中文、ASCII 与 ASCII 标点（§3）。不做展开折叠、快捷键、Compare view；不在活区里加段标题行（段信息由 RB-A 表头显示，§3.1 一个字不改）。
- 不给 finalize 以外的任何路径缩高（包括窗口尺寸变化、降级）。
- 测试腿不写生产代码，实现腿不写 `_test.go`。
- **既有 `_test.go` 只授权下面 15 处编译修正，其余一个字节都不许改（含注释）。**
  理由：A 节给两个事件追加了切片字段，结构体不再可比较，`!=` 是**编译错误**（原型实测 `invalid operation: ... (struct containing []string cannot be compared)`），两个包的测试整包编译不过。修正只把比较投影到 RB-A 冻结的那几个字段上（RS 比 Label/K/N/Members，RE 比 Label/K/N），判别力与改动前相同；追加字段由 rbb 用例另行断言。失败信息里的 `%+v` 会多印出 Speakers / Results，无害。改后两个文件在 HEAD `a728cee` 上也能编译、全绿（投影只读旧字段）。

  **`internal/repl/rba_observer_test.go`**（11 处，行号为 HEAD）：

  | 行 | 原文片段 | 改成 |
  |---|---|---|
  | 206、414 | `evs[0].rs != want {` | `rbaRS(evs[0].rs) != rbaRS(want) {` |
  | 210、418 | `evs[2].re != want {` | `rbaRE(evs[2].re) != rbaRE(want) {` |
  | 213 | `evs[3].rs != want {` | `rbaRS(evs[3].rs) != rbaRS(want) {` |
  | 217 | `evs[5].re != want {` | `rbaRE(evs[5].re) != rbaRE(want) {` |
  | 268 | `got != wantRS[s] {` | `rbaRS(got) != rbaRS(wantRS[s]) {` |
  | 271 | `got != wantRE[s] {` | `rbaRE(got) != rbaRE(wantRE[s]) {` |
  | 512、530、581 | `if evs[0].rs != wantRS {` | `if rbaRS(evs[0].rs) != rbaRS(wantRS) {` |

  **`internal/tui/rba_order_test.go`**（4 处）：

  | 行 | 原文片段 | 改成 |
  |---|---|---|
  | 127 | `got != wantRS[s] {` | `rbaRS(got) != rbaRS(wantRS[s]) {` |
  | 130 | `got != wantRE[s] {` | `rbaRE(got) != rbaRE(wantRE[s]) {` |
  | 276 | `evs[0].rs != want {` | `rbaRS(evs[0].rs) != rbaRS(want) {` |
  | 279 | `evs[3].re != want {` | `rbaRE(evs[3].re) != rbaRE(want) {` |

  并在两个文件**末尾**各追加下面这一段（tui 包那份把两个参数类型写成 `repl.RoundStartEvent` / `repl.RoundEndEvent`）；这是本刀在既有文件里唯一新增的标识符，沿用该文件的 `rba` 前缀：

  ```go
  // rbaRSKey / rbaREKey 是 RB-A 冻结的那几个字段。RB-B 给两种事件追加了切片字段，
  // 整值不再可比较（编译错误）；这里只比 RB-A 冻结的字段，判别力与改动前相同。
  // 追加字段由 RB-B 的 rbb 用例另行断言。
  type rbaRSKey struct {
  	Label         string
  	K, N, Members int
  }

  type rbaREKey struct {
  	Label string
  	K, N  int
  }

  func rbaRS(ev RoundStartEvent) rbaRSKey { return rbaRSKey{ev.Label, ev.K, ev.N, ev.Members} }
  func rbaRE(ev RoundEndEvent) rbaREKey   { return rbaREKey{ev.Label, ev.K, ev.N} }
  ```
  「原文片段」在各自文件里出现的次数恰好等于表中列出的行数（原型用恰好计数的替换脚本改的）。

---

## 2. 验收（测试腿新写的文件）

通用纪律（同 RB-A §2，补充处加粗）：

- **新文件的包级标识符一律以 `rbb` 开头**（类型、函数、变量、常量），测试函数名 `TestRbb...`。只读复用同包既有辅助：repl 包 `rbaNewRig`、`rbaRig.newREPL`、`rbaBuf`、`rbaRec`、`rbaEv`、`rbaKinds`、`rbaDump`、`newRoundStreamService`、TestMain 假 CLI（`--resume` 时睡 700ms，`session_inflight_test.go:64`、`:107-121`）；tui 包 `tui1bSubmitter`、`tui1bPrinter`、`tui1bNopDrainer`、`tui1bStep`、`tui1bRun`、`tui1bType`、`tui1bKey`、`tui1bCtrl`、`tui1bEnter`、`tui1bStripANSI`、`tui1dBuf`、`tui1dWaitFor`、`tui1dTail`、`tui1dESC`、`x39eClear`、`rbaT0`、`rbaRID`、`rbaRID2`、`rbaNewTuiRig`、`rbaTuiRig`、`rbaFakeSessionID`、TestMain 假 CLI（`RBA_FAKE_SLEEP_MS`、`RBA_FAKE_RESUME_SESSION`）。不许改它们。
- 不起真 provider；store 一律 `t.TempDir()` 下；不碰 `C:\Users\25142\.council\data`。
- 断言整行相等或整行正则（`^...$`）。例外：终端字节流（W 类、Q1 的尾巴）与 `Fatalf` 前提守卫可以用子串，写明理由。
- 每条注释写【判别力】：能不能为假、只差一处的对照是哪条。
- **跨并发原语的顺序至少一条用真 `tea.Program` 跑（H 系列）；残影守卫用 VT 回放（G 系列）。**
- HEAD 上的状态：rbb 文件引用本刀新增的字段与 `boardFinalizeMsg`，HEAD 上整包编译不过，是预期的红。**§1.2 的两个既有文件改动在 HEAD 上必须能编译且 RB-A 用例全绿**：测试腿交付前在 HEAD 的隔离拷贝里只放这两个改过的文件（不放任何 rbb 文件），跑 `go test ./internal/repl -run TestRba` 与 `go test ./internal/tui -run TestRba`，报告命令与结果。
- 帧高相关断言都同时断言 `m.frameHeight() == strings.Count(m.View().Content, "\n") + 1`（看板行必须真的在 View 里、且每行单行）。

### 2.1 `internal/repl/rbb_events_test.go`

装置一律 RB-A §2.0 的 repl 包装置（`rbaNewRig`、`newREPL` 之后清空缓冲、直接 `r.Submit`）。记录型观察者用 `rbaRec`，或 rbb 自己的一个（RB3 需要）。

**RB1 名单与结果（`--rounds 2 --with-judge`）。** 被测命令 `"/round rba_a rba_b rba_j --go --rounds 2 --with-judge"`（与 RB-A R3 相同）。断言：
1. 三个 RS 的 `Speakers` 依次 `reflect.DeepEqual` `[]string{"rba_a","rba_b"}`、`[]string{"rba_a","rba_b"}`、`[]string{"rba_j"}`，且各自 `len(Speakers) == Members`。
2. 三个 RE 的 `Results` 的 AgentID 序列依次等于同段 RS 的 `Speakers`；`len(Results) == Members`。
3. 每个 RE 的 `Results[i]` 与同段里 `AgentID` 相同的那个 MD 事件 `reflect.DeepEqual`（每段每人恰好一个 MD）。
【判别力】Speakers 缺省 / Results 缺省 → 第 1、2 条红；Results 的 WallMS、RoundID、Label、Outcome 映射错 → 第 3 条红（本命令里这四个字段都非零；三个布尔在这条命令里全为 false，由 H2、H3 担保）。对照：RB-A R3 同一命令只看旧字段。

**RB2 结果按名单顺序，不按回来顺序。** 装置同上，但在 `newREPL` 之前先 `g.rn.Ask(g.ctx, g.convID, "rba_a")` 一次，再 `g.svc.Post` 一条 `SenderID: conversation.ReservedSenderUser`、`Kind: conversation.KindUser`、`Text: "seed-msg-2"`（于是 rba_a 带 `--resume`、假 CLI 睡 700ms，rba_b 首轮立即回来）。被测命令 `"/round rba_a rba_b --go"`。
前提（不成立就 `Fatalf` 说前提不成立）：事件种类恰好 `RS MD MD RE`，两个 MD 的 AgentID 依次是 `rba_b`、`rba_a`（原型实测 wall 42ms 与 742ms）。主断言：RE 的 `Results` AgentID 依次是 `rba_a`、`rba_b`，且 `Results[0]` DeepEqual 第 2 个 MD、`Results[1]` DeepEqual 第 1 个 MD。
【判别力】在 onDone 里按回来顺序累积 Results → 顺序是 `rba_b, rba_a`，红。对照：前提那一句（回来顺序确实与名单相反）。

**RB3 名单是新分配的。** 装置同 RB1。rbb 观察者在 `RoundStart` 里先把 `append([]string(nil), ev.Speakers...)` 记下，**然后**执行 `ev.Speakers[0] = "rbb_zz"`；MD / RE 照常记录。被测命令 `"/round rba_a rba_b --go --rounds 2"`。断言：两个 RS 记下的拷贝都等于 `[]string{"rba_a","rba_b"}`；MD 恰好 4 个，AgentID 都在 `{rba_a, rba_b}` 里、`Outcome` 都是 `turn.OutcomeSpoke`；两个 RE 的 Results AgentID 都是 `rba_a, rba_b`。
【判别力】`Speakers: plan.Speakers`（不拷贝）→ 第 1 段问到 `rbb_zz`（判 refused，Outcome 7），第 2 段 RS 只剩 1 人，红（原型实测）。对照：同一观察者不改写时（RB1）形状相同。

**RB4 名单取 skipNoDelta 之后的。** RB-A R4 的装置与命令（先 `rn.Ask(ctx, convID, "rba_b")`、之后不发新消息，`"/round rba_a rba_b --go"`）。断言 RS 的 `Speakers` DeepEqual `[]string{"rba_a"}`，RE 的 Results 只有 `rba_a` 一项。
【判别力】从预算行（`rows`）取名单 → 多出 `rba_b`，红。

（行式路径不变由 RB-A 的 R1 / R2 原样担保：R2 装了观察者，事件带了新字段，屏幕仍与 golden 整串相等。）

### 2.2 `internal/tui/rbb_block_test.go`（适配器，纯函数，不起 Program）

公共：`newRoundObserver(func(msg tea.Msg) { got = append(got, msg) })`，调 `RoundEnd(ev)`。printLine 消息的正文用 RB-A 的取法：`fmt.Sprintf("%T", msg) == "tea.printLineMessage"` 且 `reflect.ValueOf(msg).FieldByName("messageBody").String()`（不调生产的 `printLine` 造期望值）。

**K1 块的行、顺序、最后一条是 roundEndMsg。** `ev = repl.RoundEndEvent{Label: "发言轮", K: 1, N: 2, Results: [...]}`，Results 依次：
`{Label: "发言轮", AgentID: "rbb_a", Outcome: turn.OutcomeSpoke, WallMS: 18234, RoundID: rbaRID}`、
`{Label: "发言轮", AgentID: "rbb_long_name", Outcome: turn.OutcomePostRejected, Dropped: true, Drifted: true, WallMS: 999, RoundID: rbaRID}`、
`{Label: "发言轮", AgentID: "克劳德.甲", Outcome: turn.OutcomeTimeout, MaybeAdvanced: true, WallMS: 300000, RoundID: rbaRID}`。
`got` 恰好 5 条：前 4 条是 printLine，正文依次等于 §3.5 的 K1 四行；第 5 条 `reflect.DeepEqual(got[4], roundEndMsg{ev: ev})`。
【判别力】先发 roundEndMsg 再发块 → 第 1 条类型不对，红；标题用 `back: 0` 或不带 roundID → 第 1 行红；终态行用「在跑」→ 红。对照：K7（Results 为空只发 1 条）。

**K2 裁判轮、SpawnFailed、空 RoundID。** `{Label: "裁判轮", Results: [{Label: "裁判轮", AgentID: "rbb_j", Outcome: turn.OutcomeSpawnFailed}]}` → 正文恰好 `裁判轮  回来 1/1`、`  没起来  rbb_j  ?`，再加 roundEndMsg，共 3 条。

**K3 状态词全表。** `{Label: "发言轮", K: 1, N: 1, Results: 10 项}`，AgentID 依次 `o0`…`o9`，Outcome 依次 `turn.OutcomeUnknown, OutcomeSpoke, OutcomePostRejected, OutcomeProviderFailed, OutcomeUnparsable, OutcomeTimeout, OutcomeSpawnFailed, OutcomeRefused, OutcomeCanceled, OutcomeLost`，WallMS 都是 1234，布尔全 false → 11 行正文恰好等于 §3.5 的 K3 表。
【判别力】任意一个 Outcome 的词或时间格映射错 → 对应那一行红；标签补齐按字节数（`len`）而不是显示宽度 → 两字词后少两个空格，红。

**K4 子进程时长的格式边界。** `{Label: "发言轮", K: 1, N: 1}`，Results 11 项，AgentID `wa`…`wk`，Outcome 都是 Spoke，WallMS 依次 `-1500, 0, 99, 100, 999, 1000, 18234, 59999, 60000, 65432, 3600000` → 12 行正文恰好等于 §3.5 的 K4 表。
【判别力】用 `%.1f` 四舍五入（59999 → `60.0s`，99 → `0.1s`）、`< 60000` 写成 `<= 60000`、负数不夹紧（-1500 会印成 `-1.-5s`；v1 用的 -5 在 Go 的向零截断下不夹紧也印 `0.0s`，杀不到，W1 实测）→ 对应行红。

**K5 id 按显示宽度补齐，三个标记的顺序。** Results：`{AgentID: "克劳德", Outcome: Spoke, WallMS: 1000}`、`{AgentID: "ab", Outcome: Spoke, WallMS: 1000}`、`{AgentID: "m1", Outcome: PostRejected, MaybeAdvanced: true, Dropped: true, Drifted: true, WallMS: 1000}`，`Label: "发言轮", K: 1, N: 1` → 正文恰好是 §3.5 的 K5 四行。
【判别力】id 列宽按字节数（「克劳德」9 字节、宽 6）→ 后两行多 3 个空格，红；标记顺序变 → 最后一行红。

**K6 段标题取第一个非空 RoundID。** Results：`{AgentID: "x", Outcome: Spoke, WallMS: 1000, RoundID: ""}`、`{AgentID: "y", Outcome: Spoke, WallMS: 1000, RoundID: rbaRID}`，`Label: "发言轮", K: 1, N: 1` → 第 1 行正文恰好 `发言轮 (688558)  回来 2/2`。
【判别力】只看 `Results[0].RoundID` → 标题没有括号，红。

**K7 Results 为空只发一条。** `RoundEnd(repl.RoundEndEvent{Label: "发言轮", K: 2, N: 3})` → `got` 恰好 1 条且 DeepEqual `roundEndMsg{ev: ev}`（与 RB-A T6 同一件事，这里单独钉）。

### 2.3 `internal/tui/rbb_board_test.go`（纯 Model，不起 Program）

公共装置：`now := rbaT0`；`sub := &tui1bSubmitter{}`；`pr := &tui1bPrinter{}`；`m := New(Options{Submit: sub, Out: tui1bNopDrainer{}, Version: "t", Println: pr.cmd, Now: func() time.Time { return now }})`；喂 `tea.WindowSizeMsg{Width: 100, Height: 24}`；「提交」= `tui1bType(t, m, "x")` 再 `tui1bStep(t, m, tui1bEnter())`，丢掉 Cmd 不跑；「消费」= 再喂一条与装置相同尺寸的 WindowSizeMsg、丢掉 Cmd（把 RB-A E5 的待装 tick 吃掉）。「看板行」= `m.chrome()[2:]`；「帧高」= `m.frameHeight()`，且每次都同时断言它等于 `strings.Count(m.View().Content, "\n") + 1`；「光标」= `m.View().Cursor`；「叶子」= `tui1bRun(t, cmd).log`，同时断言 `sawBatch == false`。表头 = `m.chrome()[0]` 整串。

**B1 主步骤表。** 提交、消费之后按 §3.6 的 B1 表逐步喂消息（每步之后断言表中列出的全部项；表中没列的项不断言）。
【判别力】逐项见 §4 的点名；要点：B1-2 的 `7s`（在跑行从本段起点算，不从提交起点算）；B1-3（窗口外的 finalize 不做事，且 COW）；B1-4（按 AgentID 改行、COW：B1-2 时存下的 Model 值渲染不变）；B1-7（新段整体换行、垫空行、不缩）；B1-9（收尾行文本、Sequence 顺序、光标钉在 (0,0)、表头空闲、帧高不变）；B1-10、B1-11（窗口里 Enter / Ctrl-C 被挡）；B1-12（finalize 缩高、取消光标钉）；B1-14（finalize 之后能提交）。对照：B2-4 的前一半（不带名单的 RB-A 式事件走 RB-A 路径）。

**B2 边界。** 每条另起一个 Model（装置相同，只是尺寸不同），逐条按 §3.6 的 B2 表：
- B2-1 降级：尺寸 `{100,12}`（上限 3），4 人 → 0 行；收尾行照打、**光标不钉**。
- B2-2 上限边界：尺寸 `{100,12}`，3 人 → 3 行、帧高 11（= 终端高 - 1）。
- B2-3 终端高未知：尺寸 `{0,0}`（`height` 兜底 24、`termHeight` 为 0），3 人 → 0 行。
- B2-4 只长不缩、窗口变矮不缩、放不下时整块垫空行、出错路径保留看板：尺寸 `{100,24}`。
- B2-5 取消与标志：Ctrl-C 之后 status 被 Enter 改掉，收尾行仍是「已取消」。
- B2-6 每次提交重置：先一次不带名单、按过 Ctrl-C 的提交，再一次带名单的提交，收尾行是「结束」、计数只含第二次。
- B2-7 截断：宽 30。
【判别力】见 §4。B2-1 与 B2-2 只差人数（上限两侧的对照）；B2-3 与 B2-2 只差终端高是否已知。

### 2.4 `internal/tui/rbb_order_test.go`（真 `tea.Program`，真 REPL，真 Runner，假 CLI）

装置：`rbaNewTuiRig(t)`；一个 rbb spy（照 RB-A `rbaSpy` 的形状：`Init`、`View` 委托，`Update` 先委托、把返回值断回 `Model` 存回、再按消息记一条日志、**返回 spy 自己**），日志种类：`P`（printLine 正文，识别法同 RB-A）、`RS`、`MD`、`RE`、`DONE`、`FIN`（`boardFinalizeMsg`）、`TICK`；每条同时记下 Update 之后的 `chrome()` 整个切片的拷贝与 `frameHeight()`。起 Program 的选项与顺序逐字照 RB-A §2.0 的真 Program 装置（`WithContext`、`WithInput(nil)`、`WithOutput(io.Discard)`、`WithoutSignalHandler`、`WithWindowSize(100, 24)`；先 `SetRoundObserver(newRoundObserver(p.Send))`，再 `lw.SetEmit(func(line string) { p.Send(printLine(line)) })`，再 `go p.Run()`；收尾 cancel 并等 Run 返回，5 秒超时）。提交 = `PasteMsg` + Enter；先 `/use`，等第 1 个 DONE（/use 不出看板，不会有 FIN，不必等）。

「块」= RE 之前紧挨着的 `1 + len(RE.Results)` 条 P。期望的成员行由 §3.2 的格式从 RE 的 `Results` 算出（本组用例里 id 都是 5 个字符，宽度相同，不需要补齐；时长 = `fmt.Sprintf("%d.%ds", ms/1000, (ms%1000)/100)`，假 CLI 远小于 60 秒）。

**H1 三段、块的位置、收尾行、finalize 后可再提交。** 不设 `RBA_FAKE_SLEEP_MS`。提交 `"/round rba_a rba_b rba_j --go --rounds 2 --with-judge"`，等第 2 个 DONE 与其后的第 1 个 FIN（30 秒）。第 1-6 条只看该命令回显那条 P 之后、到这个 FIN 为止（含）的日志：
1. 非 P、非 TICK 条目恰好 `RS MD MD RE RS MD MD RE RS MD RE DONE FIN`。
2. 每个 RE 的 `Results` 与同段 MD 按 AgentID 一一对应且 DeepEqual；Results 的 AgentID 序列等于同段 RS 的 `Speakers`。
3. 每段的块：第 1 条整行等于段标题（`发言轮 1/2 (<末6>)  回来 2/2`、`发言轮 2/2 (<末6>)  回来 2/2`、`裁判轮 (<末6>)  回来 1/1`，末 6 位取该段 MD 的 RoundID）；其后每条整行等于 `"  完成    " + id + "  " + 时长 + " 子进程"`，顺序 `rba_a, rba_b` / `rba_a, rba_b` / `rba_j`。块的第一条之前那条 P 以 `冷启动 预报 ` 开头或以 `意外冷启动 ` 开头（结算的最后一行；前提守卫，子串）。
4. 第 1、2 个 RE 之后的第一条 P 整行等于 `成员  输入token  金额  冷启动  在飞  命令行余量  依据`（下一段的预算表）；第 3 个 RE 之后下一条非 TICK 条目是 DONE（中间没有 P）。
5. DONE 与 FIN 之间恰好一条 P，整行匹配 `^/round 结束  \d+s  回来 5/5$`；DONE 那条日志的表头整串等于 `council t   空闲`。
6. 看板：每个 RS、MD、RE、DONE 条目记下的帧高都是 10（2 + floor 2 + 6），FIN 条目是 8。每个 MD 条目记下的 chrome 里，该 MD 的 AgentID 那一行整行等于它的终态行（同第 3 条的算法）；第 3 段（裁判轮）的 MD 条目 chrome 恰好 4 项，第 4 项是空串 `""`。每个 RS 条目的看板行都整行匹配 `^  在跑    rba_[abj]  \d+s$` 或为 `""`（裁判段的垫行）。
7. FIN 之后提交 `"/use " + convID`：10 秒内出现第 3 个 DONE，且它之前有回显 P `"> /use " + convID`。
【判别力】这是跨并发原语的顺序那一条：块的 Send 与 LineWriter 的 Send 在同一个 goroutine、同一个 `p.msgs`，收尾行经 Cmd。块挪到 Update 里 `tea.Println` → T7 与第 3 条红；先发 RE 再发块 → 第 3 条红（RE 之前的不是块）；收尾行用 Batch 或提前发 → 第 5 条红；finalize 不清 `finalizing` → 第 7 条红（Enter 被挡、等不到 DONE）。

**H2 取消路径。** `t.Setenv("RBA_FAKE_SLEEP_MS", "3000")`；提交同 H1 的命令；日志出现第 1 个 RS 后再等 300ms，`p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})`；等第 2 个 DONE 与其后的 FIN（15 秒）。断言：非 P、非 TICK 条目恰好 `RS MD MD RE DONE FIN`；Results 与 MD DeepEqual（两个 `MaybeAdvanced` 为 true、`Outcome` 为 `turn.OutcomeCanceled`）；块的成员行整行等于 `"  取消    " + id + "  " + 时长 + " 子进程  可能已吃下"`；RE 与 DONE 之间恰好一条 P，整行 `已跑 1/2 轮，其余取消`；DONE 与 FIN 之间恰好一条 P，整行匹配 `^/round 已取消  \d+s  回来 2/2$`。
【判别力】Results 的 `MaybeAdvanced` 或 `Outcome` 映射错 → 块行与 DeepEqual 红（RB1 抓不到这两个，因为那条命令里它们是 false / Spoke）；收尾行用 status 文字而不是标志——这条不抓（此时 status 恰是「正在取消」），由 B2-5 抓。

**H3a 漂移。** `t.Setenv("RBA_FAKE_RESUME_SESSION", "11111111-2222-4333-8444-555555555555")`，提交 `"/round rba_a rba_b --go --rounds 2"`，等 DONE 与 FIN。第 2 段块的成员行整行等于 `"  完成    " + id + "  " + 时长 + " 子进程  (前缀已断)"`，第 1 段不带标记；两段 Results 与 MD DeepEqual。
**H3b 正文丢了。** `t.Setenv("RBA_FAKE_SLEEP_MS", "1500")`，提交 `"/round rba_a rba_b --go"`，日志出现 RS 后立刻 `g.svc.Leave(context.Background(), g.convID, "rba_b", time.Now().UTC())`（必须成功，否则前提不成立）；等 DONE 与 FIN。块的 rba_b 行整行等于 `"  未入库  rba_b  " + 时长 + " 子进程  正文见上方"`，rba_a 行 `"  完成    rba_a  " + 时长 + " 子进程"`；Results 与 MD DeepEqual。
【判别力】Results 的 `Drifted` / `Dropped` 映射错 → 红；同段另一个成员、同命令的另一段是对照。

### 2.5 `internal/tui/rbb_vt_test.go`（VT 回放守卫，探针 R10）

**VT 模拟器（测试代码里重写；探针装置已删）。** 宽 W、高 H 的格子屏，外加回滚区（字符串切片）与光标 (x, y)。语义冻结如下（原型按此实现，40 次跑模拟器零未知序列）：
- 可打印字符：宽度取 `ansi.StringWidth(string(r))`（0 宽的丢掉）；有「待折行」标志时先回到 x=0 再做一次 LF；宽 2 且 x == W-1 时先折行；写入时若踩到宽字符的一半，把另一半清空；宽 2 的字符占两格（第二格标记为续格）；写完 x 前进宽度，越过 W-1 时 x = W-1 并置「待折行」。任何光标移动、CR、LF 都清「待折行」。
- `\r` 回行首；`\n` **只下移不回车**（Windows 上 bubbletea 的 mapNl 为 false，渲染器自己写 `\r\n`），在最底行时整屏上卷一行、顶行文本进回滚区；`\b` 左移（不越过 0）；`\t` 到下一个 8 的倍数列（不越过 W-1）。其余 C0 控制字符忽略。
- `ESC M`（RI）：在顶行时整屏下卷，否则上移一行。`ESC 7` / `ESC 8` 存取光标。`ESC =`、`ESC >` 忽略。`ESC ]`、`ESC P`、`ESC _`、`ESC ^` 吞到 BEL 或 `ESC \`。
- CSI（`ESC [` 参数 终止字节）：参数以 `?`、`>`、`=` 开头，或含中间字节（0x20-0x2F，例如 `ESC[1 q`、`ESC[?2026$p`）的一律忽略。支持：`A` CUU、`B` CUD、`C` CUF、`D` CUB、`E` CNL、`F` CPL、`G` CHA、`H`/`f` CUP、`d` VPA（都夹在屏内，缺省参数 1）、`J` ED（0 光标到屏尾 / 1 屏首到光标 / 2 全屏）、`K` EL（0/1/2）、`L` IL、`M` DL（在光标行插入 / 删除，x 回 0）、`P` DCH、`@` ICH、`X` ECH、`S` SU（顶行进回滚区）、`T` SD、`b` REP（重复上一个可打印字符）；`m`、`t`、`n`、`c`、`r`、`u`、`s` 忽略。**其余终止字节记为错误，用例红。**
- 行文本 = 该行格子按序拼接（续格跳过、空格子当空格），去掉右侧空格。「全部行」= 回滚区 + 屏幕各行。

**录制器**：一个带锁的 `io.Writer`，每次 `Write` 存成一块（块的下标即写入次序）。

**被测程序**：rbb 假 Submitter（持有 `p *tea.Program`，在 `NewProgram` 之后赋值）：`Submit` 依次——用 `p.Send(printLine(...))` 打一行 `L0001 本轮问`（全局编号，四位，自增，带锁）；`obs := newRoundObserver(p.Send)`；`obs.RoundStart(repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: n, Speakers: ids})`；每人一个 goroutine：睡 `30 + rand.IntN(220)` 毫秒，然后在一把互斥锁里打一行 `Lxxxx outcome spoke  agent <id>` 再 `obs.MemberDone(repl.MemberDoneEvent{Label: "发言轮", AgentID: id, Outcome: turn.OutcomeSpoke, WallMS: 睡的毫秒, RoundID: rbaRID})`；等全部回来；打一行 `Lxxxx 发言轮结算`；`obs.RoundEnd(...)`，Results 按 ids 顺序；再睡 gap；返回 nil。`IsQuit` 恒 false。ids 为 `rbb_a`、`rbb_b`…（ASCII，宽度相同）。
Model：`New(Options{Submit: 假 Submitter, Out: tui1bNopDrainer{}, Version: "t", Ctx: ctx, Println: 拉宽函数或 nil})`。**拉宽函数**（只在测试里）：`func(line string) tea.Cmd { return tea.Sequence(tea.Println(line), func() tea.Msg { time.Sleep(40 * time.Millisecond); return nil }) }`——嵌套的 Sequence 由 `execSequenceMsg` 同步展开（`tea.go:918-919`），于是 insertAbove 与 finalize 之间隔开 40ms，不改生产代码。
spy 包住 Model（`Init`/`View` 委托、返回 spy 自己），在 `submitDoneMsg` 与 `boardFinalizeMsg` 进 Update 时记下录制器当时的块数（`doneMark`、`finMark`，finalize 那个在委托之前记），并记下「见过 RS」「见过 FIN」。
Program：`tea.NewProgram(spy, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(录制器), tea.WithoutSignalHandler(), tea.WithWindowSize(80, 24))`。

**一次跑**：等录制器至少有 1 块；`PasteMsg{"/round x"}` + Enter；若本场景要打字：另起 goroutine，等「见过 RS」后 Send 三次 `tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}`，然后每 7ms Send 一次 `tea.KeyPressMsg{Code: 'q', Text: "q"}`，直到被叫停；主 goroutine 等「见过 FIN」（10 秒，否则前提不成立）、再等 250ms、叫停打字、等 100ms、记下 `end` = 当前块数；cancel ctx、等 Run 返回。回放：新 VT 80×24，先喂 30 行 `S01\r\n` … `S30\r\n`；按序喂 `chunks[:end]`。
- **公式读数**：「缩高块」= 下标 ≥ `finMark` 的第一块中含 `ESC[J` 的那块。喂它之前，取屏幕上最后一个以 `council t   ` 开头的行号 hdr，`relY = 光标 y - hdr`，公式残影 = `max(0, relY - 7)`。找不到缩高块算前提不成立。
- **屏幕读数**：在「全部行」里，以 `/round ` 开头的行必须恰好 1 行（下标 s）且整行匹配 `^/round 结束  \d+s  回来 N/N$`（N = 人数）；以 `council t   ` 开头的行必须恰好 1 行（下标 h，且 h > s）；屏幕残影 = `h - s - 1`。以 `L` 加四位数字开头的行，编号必须从 1 起连续递增。模拟器错误必须为空。
- 每次跑断言：屏幕残影 == 0、公式残影 == 0（二者互相核对）。

**场景与次数（冻结）**：
- **G1（b2，缩高前后一直打字）**：4 人、打字、gap 50ms、拉宽 40ms，跑 10 次。
- **G2（c，8 行看板）**：8 人、不打字、gap 50ms、拉宽 40ms，跑 10 次。
- **G3（c，自然时序）**：8 人、不打字、gap 50ms、不拉宽，跑 10 次。
- G1、G2 的前提（「去掉光标钉」只有在收尾行 insertAbove 之后、finalize 之前还有一次 flush 时才可观测）：记 `sumIdx` = 原始块里第一个含 `/round 结束` 的块的下标（收尾行的 insertAbove 那一块）；一次跑「窗口可观测」当且仅当存在下标 j 满足 `sumIdx < j < finMark`。在收尾行存在的那些跑次里统计，10 次里少于 8 次可观测就 Fatalf「前提不成立」。这个统计放在逐次主断言**之后**做（收尾行缺失的变异先被主断言抓）。这个前提与光标钉无关：正确实现与「去掉光标钉」的变异上都成立（原型实测：正确实现 c 场景每次 1 块、b2 场景每次 2-3 块；去掉光标钉的变异 c 场景每次 1 块且残影 2）。不要用「窗口里出现 `ESC[1 q`」当前提：那一串正是光标钉产生的，去掉光标钉时它消失，变异会被前提而不是被残影断言抓住。
- **G4（出错退出时看板在场）**：8 人、gap 50ms、不拉宽；假 Submitter 在 RoundEnd 与 gap 之后返回 `fmt.Errorf("坏了")`（而不是 nil）；不打字；等 Run 自己返回（10 秒）拿到 err；`seq := exitClearSeq(spy 里的 Model, err)`，断言整串等于 `"\x1b[15A\r\x1b[J"`（帧高 16 = 2 + 8 + 6，终端 24），再把全部块与 `seq` 喂进 VT：全部行里没有以 `council t` 开头、也没有以 `─` 开头的行；`坏了` 恰好 1 行，它之下的屏幕行全是空行。跑 5 次。
- 全部四个场景约 25 秒（原型：每次 0.5-0.8 秒）。
【判别力】原型在正确实现上 G1-G3 各 10/10 残影为 0、屏幕与公式读数一致；变异实测：去掉光标钉 → G1、G2 各 10/10 残影 2（自然时序的 G3 抓不到，0/10，这正是要拉宽的原因）；去掉收尾行（Sequence 里只剩 finalize）→ 三个场景都没有收尾行，公式残影 2（每个场景 ≥9/10）；收尾行为空串 → 没有收尾行，G3 公式残影 2（10/10），拉宽的 G1/G2 公式为 0 但收尾行缺失照样红；看板行只在 View 里、不在 chrome 里 → G4 的 `seq` 不等、屏幕留表头。**升级 bubbletea 或 ultraviolet 时重跑本文件**：构造依赖 flush 以 `MoveTo(view.Cursor)` 结尾（`cursed_renderer.go:512-515`）与 insertAbove 的 `SetPosition(0,0)`（`:801`）。

### 2.6 `internal/tui/rbb_quit_windows_test.go`（只在 Windows 上编译；finalize 窗口里的 Ctrl-C，真 `runWith`）

照 `x39e_rig_windows_test.go` 的形状，但自己搭：`buf := &tui1dBuf{}`；`lw := NewLineWriter(nil)`；rbb 假 Submitter（持有 `p` 与 `lw`）：`lw.Write("本轮问 3 个: a, b, c\n")`、`RoundStart{发言轮,1,1,3,[a b c]}`、三人依次（各睡 30ms）写 `outcome spoke  agent <id>\n` 并 `MemberDone{Outcome: Spoke, WallMS: 30, RoundID: "0123456789abcdef"}`、写 `发言轮结算\n`、`RoundEnd`（Results 三人）、返回 nil。Model：`New(Options{Submit: 假件, Out: lw, Version: "t", Ctx: ctx, Println: 拉宽 400ms 的函数})`；`p := tea.NewProgram(m, WithContext, WithInput(nil), WithOutput(buf), WithoutSignalHandler, WithWindowSize(80, 24))`；`go func() { done <- runWith(p, lw, buf) }()`；等去 ANSI 后出现 80 个 `─`；提交 `"/round a b c --go"`；等去 ANSI 后出现 `/round 结束`（5 秒，前提）。
1. 立刻 Send 空闲式 Ctrl-C（`tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}`）：100ms 内 `runWith` **不许返回**。
2. 再等 500ms（窗口结束、finalize 已到），再 Send Ctrl-C：5 秒内 `runWith` 返回 nil。
3. 缓冲里最后一个 `pinTail`（与 X-39e A1 相同：`"\r\n\n\n\n\n\n\n" + ESC[J + ESC[?25h + ESC[?2004l`）存在，它之前以 `pinHead`（`"council t   正在退出" + ESC[21G`）结尾，它之后的整串恰好等于 `x39eClear(8)`。
【为什么是子串】终端字节流。【判别力】窗口里不挡 Ctrl-C → 第 1 条红（原型实测在窗口里退出，尾巴里出现 `ESC[1 q`、`?25h` 挪到换行之前、帧 11 行、清屏变 `ESC[10A\r ESC[J`）；finalize 不清光标钉 → 退出时仍带真光标，第 3 条红。对照：X-39e A1（没有看板时同一尾巴）。

---

## 3. 冻结的字面量

### 3.1 段标题（块的第 1 行）

与 RB-A §3.1 的「段」同一个格式、同一个函数：`label` + （`n > 1` 时 `" k/n"`）+ （roundID 非空时 `" (" + 末 6 位 + ")"`）+ `"  回来 " + 回来 + "/" + 人数`。块里回来 == 人数 == `len(Results)`。例：

```
发言轮 1/2 (688558)  回来 3/3
裁判轮  回来 1/1
发言轮 (688558)  回来 2/2
```

### 3.2 成员行

```
在跑行：  "  " + 补齐("在跑", 6) + "  " + 补齐(id, idW) + "  " + formatElapsed(本段已用)
终态行：  "  " + 补齐(状态词, 6) + "  " + 补齐(id, idW) + "  " + 时间格 [ + "  " + 标记串 ]（标记串非空时才有这一段）
补齐(s, w)：s 后面补 ASCII 空格到 ansi.StringWidth 为 w；已经 >= w 时原样
idW：同一组行（活区：当前段名单；块：该 RE 的 Results）里 id 的 ansi.StringWidth 最大值
```
分隔全是两个 ASCII 空格。活区行最后 `ansi.Truncate(行, width, "")`；块的行不截断。「本段已用」的可观测定义：本段起点 = 收到该段 `roundStartMsg` 时的时钟；已用 = 最近一拍（`elapsedTickMsg.at`）减本段起点，负数印 `0s`（`formatElapsed` 自己夹紧）；本段开始后还没来新拍时印 `0s`。实现上等价于 `formatElapsed(m.elapsed - m.segStart.Sub(m.runStart))`，写法实现方定，可观测值以 B1 表为准。

### 3.3 状态词、时间格、标记

| `turn.Outcome` | 状态词 | 时间格 |
|---|---|---|
| （未回来） | `在跑` | `formatElapsed(本段已用)` |
| `OutcomeSpoke` | `完成` | 子进程时长 + ` 子进程` |
| `OutcomePostRejected` | `未入库` | 子进程时长 + ` 子进程` |
| `OutcomeProviderFailed` | `报错` | 子进程时长 + ` 子进程` |
| `OutcomeUnparsable` | `认不出` | 子进程时长 + ` 子进程` |
| `OutcomeTimeout` | `超时` | 子进程时长 + ` 子进程` |
| `OutcomeSpawnFailed` | `没起来` | `?` |
| `OutcomeRefused` | `拒绝` | `-` |
| `OutcomeCanceled` | `取消` | 子进程时长 + ` 子进程` |
| `OutcomeLost` | `丢失` | `-` |
| `OutcomeUnknown` 及其他值 | `未知` | `-` |

- 「回来」不是状态词（表头的「回来 x/y」数的是全部 MemberDone，含超时与被拒，两处同一个词不许两个意思）。`OutcomeLost` 不经回调（`15` §3.1 订正），只为全表完整。
- **子进程时长**（D4 裁定：显示 `rec.WallMS`，只量子进程那一段；跑完那一刻读数比活计时小，接受）：`ms < 0` 时当 0；`ms < 60000` 时 `fmt.Sprintf("%d.%ds", ms/1000, (ms%1000)/100)`（**截断到十分之一秒，不四舍五入**）；否则 `formatElapsed(time.Duration(ms) * time.Millisecond)`。
- **标记**（多个时按这个顺序、两个空格分隔）：`MaybeAdvanced` → `可能已吃下`；`Dropped` → `正文见上方`；`Drifted` → `(前缀已断)`（ASCII 括号：它只是提示，与前两个不同级，`15` §3.1 订正）。PostRejected 与 Dropped 都在：状态词说「未入库」，标记指向回滚区里 `writeAskRecord` 打出的那段正文。

### 3.4 收尾行

```
/round 结束  <formatElapsed(submitDone 时刻的时钟 - 本次提交起点)>  回来 <回来总数>/<名单长度总和>
/round 已取消  <同上>  回来 <同上>/<同上>
```
两个空格分隔。原型实测例：`/round 结束  2m05s  回来 4/4`、`/round 已取消  12s  回来 2/4`、`/round 结束  0s  回来 5/5`、`/round 结束  0s  回来 0/4`（降级）。

### 3.5 适配器用例的逐字节期望（原型实跑）

K1（4 行正文）：
```
发言轮 1/2 (688558)  回来 3/3
  完成    rbb_a          18.2s 子进程
  未入库  rbb_long_name  0.9s 子进程  正文见上方  (前缀已断)
  超时    克劳德.甲      5m00s 子进程  可能已吃下
```
K3（11 行正文）：
```
发言轮  回来 10/10
  未知    o0  -
  完成    o1  1.2s 子进程
  未入库  o2  1.2s 子进程
  报错    o3  1.2s 子进程
  认不出  o4  1.2s 子进程
  超时    o5  1.2s 子进程
  没起来  o6  ?
  拒绝    o7  -
  取消    o8  1.2s 子进程
  丢失    o9  -
```
K4（12 行正文）：
```
发言轮  回来 11/11
  完成    wa  0.0s 子进程
  完成    wb  0.0s 子进程
  完成    wc  0.0s 子进程
  完成    wd  0.1s 子进程
  完成    we  0.9s 子进程
  完成    wf  1.0s 子进程
  完成    wg  18.2s 子进程
  完成    wh  59.9s 子进程
  完成    wi  1m00s 子进程
  完成    wj  1m05s 子进程
  完成    wk  1h00m 子进程
```
K5（4 行正文）：
```
发言轮  回来 3/3
  完成    克劳德  1.0s 子进程
  完成    ab      1.0s 子进程
  未入库  m1      1.0s 子进程  可能已吃下  正文见上方  (前缀已断)
```
这些块里每一行都没有行尾空格。测试腿把它们写成 Go 字符串常量逐行比较。

### 3.6 Model 用例的步骤表（原型实跑）

记号：`T0` = `rbaT0`；`RID` / `RID2` = RB-A 的 `rbaRID` / `rbaRID2`；`RS{...}` = `roundStartMsg{ev: repl.RoundStartEvent{...}}`；`MD{...}` = `memberDoneMsg{ev: repl.MemberDoneEvent{...}}`；`RE{...}` 同理；`FIN` = `boardFinalizeMsg{}`；`TICK(g, s)` = `elapsedTickMsg{gen: g, at: T0.Add(s * time.Second)}`；`空` = `""`。「看板行」按顺序列出，Go 字符串字面量写法；「光标」写 `nil` 或 `(0,0)`（后者 = `Cursor != nil && Cursor.X == 0 && Cursor.Y == 0`）。

**B1 主步骤表**（尺寸 `{100,24}`，提交、消费之后；这时 tickGen 是 1）：

| 步 | 动作 | 断言 |
|---|---|---|
| B1-1 | `now = T0+5s`；`RS{Label: "发言轮", K: 1, N: 2, Members: 3, Speakers: []string{"rbb_a", "rbb_long_name", "克劳德.甲"}}` | Cmd nil；帧高 11；光标 nil；表头 `council t   正在运行  发言轮 1/2  回来 0/3  0s`；看板行 `"  在跑    rbb_a          0s"`、`"  在跑    rbb_long_name  0s"`、`"  在跑    克劳德.甲      0s"` |
| B1-2 | `TICK(1, 12)` | 表头 `council t   正在运行  发言轮 1/2  回来 0/3  12s`；看板行三行的末尾都是 `7s`：`"  在跑    rbb_a          7s"`、`"  在跑    rbb_long_name  7s"`、`"  在跑    克劳德.甲      7s"` |
| B1-3 | 存下 `saved := m`；对 `m` 喂 `FIN` 得到 `mx`（**丢掉 mx，继续用 m**） | `mx` 的 Cmd nil、帧高 11、看板行与 B1-2 相同（窗口外的 finalize 不做事） |
| B1-4 | `MD{Label: "发言轮", AgentID: "rbb_a", Outcome: turn.OutcomeSpoke, WallMS: 18234, RoundID: RID}`，再 `MD{Label: "发言轮", AgentID: "克劳德.甲", Outcome: turn.OutcomeTimeout, MaybeAdvanced: true, WallMS: 300000, RoundID: RID}` | 表头 `council t   正在运行  发言轮 1/2 (688558)  回来 2/3  12s`；看板行 `"  完成    rbb_a          18.2s 子进程"`、`"  在跑    rbb_long_name  7s"`、`"  超时    克劳德.甲      5m00s 子进程  可能已吃下"`；**`saved.chrome()[2:]` 仍等于 B1-2 的三行** |
| B1-5 | `TICK(1, 65)` | 表头末尾 `1m05s`；看板第 2 行 `"  在跑    rbb_long_name  1m00s"`，另两行不变 |
| B1-6 | `MD{Label: "发言轮", AgentID: "rbb_long_name", Outcome: turn.OutcomePostRejected, Dropped: true, Drifted: true, WallMS: 999, RoundID: RID}`，再 `RE{Label: "发言轮", K: 1, N: 2, Results: []repl.MemberDoneEvent{{Label: "发言轮", AgentID: "rbb_a", Outcome: turn.OutcomeSpoke, WallMS: 1000, RoundID: RID}}}`（v1.1：带一项 Results，让纯 Model 层钉住 roundEndMsg 不返回 Println） | RE 的 Cmd nil；表头 `council t   正在运行  发言轮 1/2 (688558)  回来 3/3  1m05s`；看板第 2 行 `"  未入库  rbb_long_name  0.9s 子进程  正文见上方  (前缀已断)"`；帧高 11 |
| B1-7 | `now = T0+70s`；`RS{Label: "裁判轮", Members: 1, Speakers: []string{"rbb_j"}}` | 表头 `council t   正在运行  裁判轮  回来 0/1  1m05s`；看板行 `"  在跑    rbb_j  0s"`、空、空；帧高 11 |
| B1-8 | `MD{Label: "裁判轮", AgentID: "rbb_j", Outcome: turn.OutcomeRefused, WallMS: 0, RoundID: RID2}` | 表头 `council t   正在运行  裁判轮 (aa08ae)  回来 1/1  1m05s`；看板行 `"  拒绝    rbb_j  -"`、空、空 |
| B1-9 | `now = T0+125s`；`submitDoneMsg{}` | 表头 `council t   空闲`；看板行不变；帧高 11；光标 (0,0)；叶子恰好 `["print:/round 结束  2m05s  回来 4/4", "other:tui.boardFinalizeMsg"]` |
| B1-10 | `tui1bType(t, m, "y")`；`Enter` | Cmd nil；表头 `council t   空闲`；帧高 11 |
| B1-11 | `tui1bCtrl('c')` | Cmd nil；表头 `council t   空闲`；光标 (0,0) |
| B1-12 | `FIN` | Cmd nil；帧高 8；`len(m.chrome()) == 2`；光标 nil；表头 `council t   空闲` |
| B1-13 | `FIN` | Cmd nil；帧高 8 |
| B1-14 | `Enter` | 叶子恰好 `["print:> y", "submitDone"]`；跑完之后 `sub.count() == 1`；表头 `council t   正在运行  0s`；帧高 8 |

**B2 边界表**（`now` 恒为 T0，除非注明）：

| 条 | 尺寸 | 动作 | 断言 |
|---|---|---|---|
| B2-1 | `{100,12}` | 提交、消费；`RS{Label: "发言轮", K: 1, N: 1, Members: 4, Speakers: []string{"a","b","c","d"}}`；`submitDoneMsg{}` | RS 之后帧高 8、`len(chrome) == 2`；submitDone 之后光标 **nil**、帧高 8、叶子恰好 `["print:/round 结束  0s  回来 0/4", "other:tui.boardFinalizeMsg"]` |
| B2-2 | `{100,12}` | 提交、消费；`RS{...Members: 3, Speakers: []string{"a","b","c"}}` | 帧高 11；看板行 `"  在跑    a  0s"`、`"  在跑    b  0s"`、`"  在跑    c  0s"` |
| B2-3 | `{0,0}`（装置与消费都用这个尺寸） | 提交、消费；`RS{...Members: 3, Speakers: []string{"a","b","c"}}` | 帧高 8；`len(chrome) == 2` |
| B2-4 | `{100,24}` | 提交、消费；`RS{Label: "发言轮", K: 1, N: 3, Members: 2, Speakers: []string{"a","b"}}` → 帧高 10；`RS{K: 2, N: 3, Members: 3, Speakers: a,b,c}`（Label 同）→ 帧高 11；`RS{K: 3, N: 3, Members: 1, Speakers: []string{"a"}}`；`tea.WindowSizeMsg{Width: 100, Height: 10}`；`RS{Label: "裁判轮", Members: 5, Speakers: []string{"j1","j2","j3","j4","j5"}}`；`submitDoneMsg{err: errors.New("坏了")}` | 第 3 个 RS 之后帧高 11、看板行 `"  在跑    a  0s"`、空、空；窗口变矮之后帧高 11、看板行不变；裁判 RS 之后帧高 11、看板行空、空、空、表头 `council t   正在运行  裁判轮  回来 0/5  0s`；出错之后表头 `council t   发生错误，正在退出`、帧高 11、光标 nil、叶子恰好 `["print:坏了", "quit"]`、`exitClearSeq(m, nil) == "\x1b[9A\r\x1b[J"`（min(11, 10) = 10） |
| B2-5 | `{100,24}` | 提交、消费；`RS{Label: "发言轮", K: 1, N: 2, Members: 4, Speakers: a,b,c,d}`；`MD{AgentID: "a", Outcome: turn.OutcomeCanceled, MaybeAdvanced: true, WallMS: 300}`；`tui1bCtrl('c')`；`tui1bType("z")` + `Enter`；`MD{AgentID: "b", Outcome: turn.OutcomeSpawnFailed, WallMS: 5}`；`now = T0+12s`；`submitDoneMsg{}` | submitDone 之前：表头 `council t   一轮正在跑，先等它回来或按 Ctrl-C 取消  发言轮 1/2  回来 2/4  0s`，帧高 12，看板行 `"  取消    a  0.3s 子进程  可能已吃下"`、`"  没起来  b  ?"`、`"  在跑    c  0s"`、`"  在跑    d  0s"`；之后叶子恰好 `["print:/round 已取消  12s  回来 2/4", "other:tui.boardFinalizeMsg"]` |
| B2-6 | `{100,24}` | 提交、消费；`RS{Label: "发言轮", K: 1, N: 2, Members: 3}`（**不带名单**）；`MD{AgentID: "q", RoundID: RID}`；`tui1bCtrl('c')`；`submitDoneMsg{}`；再提交、消费；`RS{Label: "发言轮", K: 1, N: 1, Members: 1, Speakers: []string{"a"}}`；`MD{AgentID: "a", Outcome: turn.OutcomeSpoke, WallMS: 1000}`；`submitDoneMsg{}` | 第一次 submitDone：Cmd **nil**、光标 nil、帧高 8、表头 `council t   空闲`（RB-A 路径）；第二次：叶子恰好 `["print:/round 结束  0s  回来 1/1", "other:tui.boardFinalizeMsg"]`、帧高 9、看板行 `"  完成    a  1.0s 子进程"`、光标 (0,0) |
| B2-7 | `{100,24}` | 提交、消费；`tea.WindowSizeMsg{Width: 30, Height: 24}`；`RS{Label: "发言轮", K: 1, N: 1, Members: 2, Speakers: []string{"克劳德.甲乙丙丁戊己", "b"}}`；`MD{AgentID: "克劳德.甲乙丙丁戊己", Outcome: turn.OutcomePostRejected, Dropped: true, Drifted: true, WallMS: 61000}` | 帧高 10；看板行 `"  未入库  克劳德.甲乙丙丁戊己 "`（末尾一个空格）与 `"  在跑    b" + strings.Repeat(" ", 19)`；两行的 `ansi.StringWidth` 都是 30 |

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；主代理落地后逐条跑，从副本还原，不用 `git checkout`。每条都注明杀手在什么状态下观测。

| # | 变异（可编译） | 预期被谁杀 |
|---|---|---|
| 1 | round.go：`Speakers: plan.Speakers`（不拷贝） | RB3（原型实测：问到 `rbb_zz`、第 2 段只剩 1 人） |
| 2 | round.go：RS 不带 Speakers | RB1 第 1 条；H1（没有名单就永远不出看板、不 finalize，H1 等 FIN 30 秒后按前提不成立失败，W1 实测） |
| 3 | round.go：Speakers 取预算行的 id（skipNoDelta 之前） | RB4 |
| 4 | round.go：RE 不带 Results | RB1 第 2 条；H1 第 3 条（没有块） |
| 5 | round.go：Results 在 onDone 里按回来顺序累积 | RB2（rba_a 慢 700ms 的状态下） |
| 6 | round.go：Results 里 `WallMS: 0` | RB1 第 3 条；H1 第 3 条 |
| 7 | round.go：Results 里 `RoundID: ""` | RB1 第 3 条；H1 第 3 条（标题没有括号） |
| 8 | round.go：Results 里 `Label: ""` | RB1 第 3 条 |
| 9 | round.go：Results 里 `Outcome: turn.OutcomeSpoke`（写死） | H2（取消路径，Outcome 是 Canceled） |
| 10 | round.go：Results 里 `MaybeAdvanced: false` | H2 |
| 11 | round.go：Results 里 `Drifted: false` | H3a |
| 12 | round.go：Results 里 `Dropped: false` | H3b |
| 13 | roundobs.go：RoundEnd 先发 roundEndMsg 再发块 | K1；H1 第 3 条 |
| 14 | roundobs.go：Results 为空时也打段标题 | K7；RB-A T6（既有，恰好 1 条） |
| 15 | model.go：块改由 roundEndMsg 分支返回 `tea.Println` | B1-6（v1.1 起 RE 带一项 Results）；H1 第 3 条。RB-A T7 杀不到：它的 RE 不带 Results，拼出的是空 Sequence、Cmd 仍为 nil（W1 实测） |
| 16 | 块标题 `back: 0` | K1 |
| 17 | 块标题只看 `Results[0].RoundID` | K6 |
| 18 | 状态词任意一项写错（例如 Refused 印成 `拒收`） | K3 |
| 19 | 时间格：SpawnFailed 印 `-`，或 Refused 印 `?` | K3；K2 |
| 20 | 子进程时长用 `fmt.Sprintf("%.1fs", float64(ms)/1000)` | K4（`59999` → `60.0s`、`99` → `0.1s`） |
| 21 | 子进程时长 `ms < 60000` 改 `ms <= 60000` | K4（`60000`） |
| 22 | 子进程时长不夹负数 | K4（`-1500` → `-1.-5s`；生产上 WallMS 来自单调钟、不会为负，这是防御性代码） |
| 23 | 标记顺序换成 Drifted 在前 | K5；K1 |
| 24 | 补齐按 `len()` 字节数而不是显示宽度（状态词或 id） | K3（状态词）；K5（id） |
| 25 | 活区不截断 | B2-7 |
| 26 | 活区截断用 `lipgloss.NewStyle().Width(width).Render` | B2-7（折成多行，帧高 ≠ View 行数） |
| 27 | 看板行数取 `Members` 而不是 `len(Speakers)` | RB-A T5（既有：H5 状态帧高 8）；B2-6 第一次 submitDone |
| 28 | `runSeen` 在不带名单的 RS 上也置位 | RB-A T2 的 H15（既有）；B2-6 第一次 submitDone（Cmd 应为 nil） |
| 29 | D2：`floor = len(Speakers)`（每段重设，可缩） | B2-4（第 3 个 RS 后帧高应仍为 11） |
| 30 | D10：不设上限 | B2-1 |
| 31 | D10：上限多 1（`termHeight - (2 + ta.Height())`） | B2-1（4 行出现） |
| 32 | D10：上限少 1（`... - 2`） | B2-2（3 行不出现） |
| 33 | D10：用 `m.height` 而不是 `m.termHeight` | B2-3 |
| 34 | WindowSizeMsg 按新上限夹 floor | B2-4（窗口变矮之后帧高应仍为 11） |
| 35 | 放不下时照样显示名单前 floor 行 | B2-4（裁判 RS 后应为三行空串） |
| 36 | 放不下时照样长高（忽略上限，只看 `want > floor`） | B2-4（帧高应为 11，变异是 13） |
| 37 | 在跑行用 `formatElapsed(m.elapsed)`（从提交起点算） | B1-2（`12s` ≠ `7s`） |
| 38 | MD 按回来的次序改第 k 行 | B1-4（第 3 个人先回来） |
| 39 | MD 原地改看板切片（不 COW） | B1-4 的 `saved` 那一句 |
| 40 | 新段 RS 把行追加到旧行后面 | B1-7 |
| 41 | 收尾行动词看 status 文字 | B2-5 |
| 42 | 收尾行时间用 `m.elapsed`（最近一拍） | B1-9（`1m05s` ≠ `2m05s`） |
| 43 | 收尾行计数只算当前段 | B1-9（`1/1` ≠ `4/4`） |
| 44 | D5 用 `tea.Batch(println, finalize)` | B1-9（`tui1bRun` 倒序跑 Batch，叶子顺序反、`sawBatch` 为真）；B2-1、B2-5、B2-6 |
| 45 | D5 直接调 `tea.Println` 而不是 `m.println` | B1-9（叶子是 `other:tea.printLineMessage`） |
| 46 | D5 不钉光标 | B1-9；G1、G2（拉宽状态下 10/10 残影 2） |
| 47 | D5 光标钉在 `(0, 8)` | B1-9 |
| 48 | D5 `floor == 0` 时也钉光标 | B2-1（光标应为 nil） |
| 49 | D5 `floor == 0` 时不打收尾行（返回 nil） | B2-1 |
| 50 | D5 不打收尾行（Sequence 里只有 finalize） | B1-9；G1-G3（没有收尾行；公式残影 2） |
| 51 | D5 收尾行为空串 | B1-9；G1、G2、G3 的「收尾行恰好 1 条」主断言（30/30）。G3 自然时序下公式残影 2 约占半数（W1 实测 6/10），不作为保证 |
| 52 | D5 无论 runSeen 都走新路径 | RB-A `TestTui1bAcc12SubmitDoneWithoutErrorGoesIdle`（既有：返回 nil Cmd）；B2-6 |
| 53 | D5 新路径不把 status 改成 `空闲` | B1-9；RB-A O1/O2（既有：DONE 处表头 == 空闲） |
| 54 | D5 err 支也钉光标或也走 Sequence(收尾行) | B2-4（叶子应恰好 `print:坏了, quit`，光标 nil） |
| 55 | D5 err 支清看板 | B2-4（帧高 11、`exitClearSeq`）；G4 |
| 56 | D6 窗口里不挡 Ctrl-C | B1-11（叶子含 quit）；Q1 第 1 条（原型实测在窗口里退出、尾巴变形） |
| 57 | D7 窗口里不挡 Enter | B1-10（Cmd 非 nil） |
| 58 | D4 不清 floor | B1-12（帧高 8） |
| 59 | D4 不清 pinCursor | B1-12（光标 nil）；Q1 第 3 条 |
| 60 | D4 不清 finalizing | B1-14（Enter 被挡）；H1 第 7 条 |
| 61 | D4 不看 finalizing 就清（窗口外的 finalize 也清） | B1-3 |
| 62 | D9 看板行只进 View、不进 chrome | 每条 B 用例的「帧高 == View 行数」；G4 |
| 63 | D12 beginSubmit 不清 `canceling` | B2-6（第二次收尾行变成 `已取消`） |
| 64 | D12 beginSubmit 不清两个累计数 | B2-6（`回来 2/1`；第一次提交里不带名单的那个 MD 也计了回来数） |
| 65 | D6 忙时 Ctrl-C 不置 canceling | B2-5；H2 第 5 条 |
| 66 | D8 钉光标用 `v.Cursor = nil`（从不钉，但 pinCursor 照样置位） | B1-9（光标）；G1、G2 |

**判定为等价、不列杀手的变异**（免得被当成漏测）：
- D4 不做 `tickGen++`：D5 已置 `busy = false`，旧代次的拍被 RB-A E6 的 `!busy` 门挡掉；下一次 beginSubmit 照样 `tickGen++`。可观测行为不变（只影响跨 finalize 的代次编号，没有断言依赖它）。
- D12 不清看板行 / `floor` / `runSeen`：能走到 beginSubmit 的前一次提交要么没出过看板（这些本来就是零值），要么经过 finalize（D4 已清），要么走了 err / IsQuit（随后退出）。只清 `canceling` 与两个累计数有可观测后果（#63、#64）。
- D3 找不到 AgentID 时追加一行：真路径上名单与 MD 的 AgentID 同源，找不到只发生在 RB-A 的合成事件上（没有名单、看板为空，追加也不显示——`floor` 为 0）。
- D2 的 `segStart` 在 `len(Speakers) == 0` 时也取：此时看板为空，不显示在跑行。
- `boardFinalizeMsg` 分支落进 textarea 而不是提前返回、但 finalize 逻辑照做：textarea 忽略这条消息。
- （v1.1，W1 实测存活）finalize 不清看板行：`floor` 已归零、看板不显示，D12 在下一次提交时再清。
- （v1.1，W1 实测存活）回来总数 `++` 挪到 memberDoneMsg 的 `!busy || !run.has` 门之前：真路径上 MD 不会在门外到达。

**只在审查时看（没有用例能抓，集成员收割时逐字核对）**：
- `run.go` 装配那一行仍逐字是 `r.SetRoundObserver(newRoundObserver(p.Send))`（RB-A §4 同条）。适配器的 RoundEnd 里若改成 `go o.send(...)` 或经缓冲 channel 转发，块会跑到下一段的预算表后面，H1 第 3、4 条**可能**抓到、也可能不抓（取决于调度），不当作保证。
- 适配器不持锁、不读写任何共享状态（RB-A G.1 的其余部分）。
- 块函数与活区行调用同一组格式函数（§1.1 C4）。两份实现逐字相同时所有用例都绿，分叉的那一天才会有一边红。
- B4 的注释措辞、D 各处新增注释：与本契约不符的话（例如把 `Drifted` 写成「身份漂移」、把 finalize 以外的路径写成可以缩高）只能靠审查。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许改这 4 个文件：`internal/repl/observer.go`、`internal/repl/round.go`、`internal/tui/roundobs.go`、`internal/tui/model.go`。不写任何 `_test.go`（§1.2 的 15 处由测试腿改），不改 `run.go`、`go.mod`、`go.sum`。
- 生产改动原型约 260 行，**一次交付**（不拆块）。
- 已有文件用补丁块，每个文件可以有多个：

  ```
  ===PATCH: internal/tui/model.go===
  <<<<ANCHOR
  （从真文件里原样复制的若干行，含行首 tab，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点必须从 HEAD `a728cee` 的真文件逐字复制（含 tab 与中文标点），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。本刀不新建文件；若要新建，用 `===FILE: 路径===` 换行接完整内容、末尾 `===END FILE===`。
- 代码必须能 `gofmt`（tab 缩进，结构体字段与注释的对齐按 gofmt）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的字符串字面量只有 §3 列出的那些（外加 `"  "`、`" "`、`"%d.%ds"` 这类拼接用的 ASCII 片段）。
- 生产代码的新标识符不许以 `rbb` 或 `rba` 开头（那是测试腿的前缀）。冻结的名字只有：字段 `Speakers`、`Results`，类型 `boardFinalizeMsg`；其余「名字实现方定」。
- §1.1 里逐字给出的代码（A 的两个结构体、B1 的字面量、B2 的整段、C1 的方法体）照抄。
- 已知的编译后果（不是你的错，不要为它改生产代码）：追加切片字段后，既有的 `rba_observer_test.go` 与 `rba_order_test.go` 在测试编译时会报 15 处「struct containing []string cannot be compared」，由测试腿按 §1.2 修。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：4 个文件是否都给了；§1.1 A、B1-B4、C1-C4、D1-D13 每一条落在哪个补丁块。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3 的字面量是否逐字照抄；有没有新增任何别的字符串字面量。
  ```

---

## 6. 落地后主代理要做的

1. 收割：越界扫描（主仓库与中转根有没有 grok 写出来的新文件）；符号禁令扫描；按锚点落盘；`gofmt -l`；`go vet ./internal/repl ./internal/tui`；`go build ./...`；`git diff --stat` 只有 §5 的 4 个生产文件。逐字核对 `run.go` 未动、装配那一行不变。
2. 合并测试腿的文件（§1.2 的 2 个既有文件修正 + §2 的 6 个新文件）后，只跑 `./internal/repl` 与 `./internal/tui` 两个包（3 分钟没输出只停自己的 go 进程，改 `go test -c` 再跑二进制）。既有用例必须全绿；G 系列约 25 秒、H 系列约 5 秒是预期。
3. 核对测试腿报告里「§1.2 两个文件在 HEAD 上单独编译、RB-A 用例全绿」的证据。
4. §4 的 66 条变异逐条跑（等价清单里的不跑；从副本还原）。任何一条没被点名杀手杀掉，先判是用例空、还是契约写错；契约错就改契约再改任务书。
5. 文档：
   - `docs/00_DECISIONS.md` #41 标题状态改成「第 1、2 刀已实现（RB-A，`69fed6c`），第 3 刀已实现（RB-B，`<commit>`，待真机复验），第 4 刀未做」；在「第 3 刀前置探针」一段之后补「RB-B 落地记录」：收尾行的措辞（Griffin 2026-09-24 裁：一行运行小结）、降级时也打收尾行的理由（行为一致；此时 REPL 已写完，Println 顺序安全；并且窗口挡住 Enter，下一条命令的回显不会跑到收尾行前面）、Ctrl-C 选「挡住」的理由与 Q1 的实测、上限用 `termHeight` 的理由、`canceling` 标志的理由、中途拉矮窗口的已知代价与范围、被杀路上块与收尾行丢失、§1.2 的 15 处编译修正与为什么绕不开（结构体不可比较）。
   - 「RB-A 剩余项」：(d) 标记已解决（`Speakers` 新分配，RB3 担保）；(e) 补一句重新论证的结论（§1.1 F.4）。新增剩余项：`x/ansi` 在 go.mod 里的 `// indirect` 标记不准（未跑 tidy）；`ESC[0 q` 复位用户光标形状未在真终端测；窗口里被挡的 Enter 是静默的（输入框保留文字，要再按一次）；Ctrl-C 按在最后一个成员回来之后也会印「已取消」。
   - `docs/15_TUI_SPEC_MAPPING.md` §3.1 补一句：成员行已进活区（RB-B），行状态按「结局 × 标记」两维显示，终态块进回滚区，表头总览仍在、是降级形态。
   - 本契约归档成 `docs/15b_RUN_BOARD_RBB_CONTRACT.md`（加归档说明：行号指 HEAD `a728cee`）；在 `15a_RUN_BOARD_RBA_CONTRACT.md` 顶部归档说明里补一句「§1.1 A/C3/C5/D/E6-E9/G.1 与 §1.2 的部分条款已被 RB-B 改写，见 15b 开头的对照表」。
6. 真机复验（Griffin，Windows Terminal + PowerShell 5.1，另在 conhost 上各跑一遍）：
   1. `/round a b --go`：输入框上方出现两行「在跑」，秒数每秒走；回来的人变成「完成  …  x.xs 子进程」；段结束时回滚区出现一行段标题加两行成员；命令结束时回滚区多一行 `/round 结束  Ns  回来 2/2`，活区缩回 8 行，**提示符上方不留任何旧表头、横线或成员行**。
   2. `/round a b j --go --rounds 2 --with-judge`：三段依次出现，裁判段只有 1 行、下面垫一行空行（不缩）；滚回历史，每段的明细、结算、块依次完整。
   3. 跑的过程中一直在输入框里打字、按 Ctrl-J 换行，直到命令结束之后：缩高后不留残影（G1 的真机版）。
   4. 一轮进行中按 Ctrl-C：出现「已跑 k/N 轮，其余取消」，然后 `/round 已取消  …`，回到空闲。
   5. 把窗口高度拉到 12 行左右再跑 4 个人的 /round：不出成员行、只有表头总览，结束时仍打收尾行。再在 24 行下开跑、中途把窗口拉矮到 10 行：记录看到了什么（已知代价，F.3），结束后是否有残影。
   6. 有看板时让命令出错退出（或在跑的时候关窗）：退出后提示符上方不留帧。
   7. 先在 shell 里设一个非默认的光标形状（例如竖线），跑一次 /round：结束后光标形状是否被复位成默认（F.5 的 `ESC[0 q`）。
   8. 非终端的行式路径（`council < script.txt`）跑一次 /round：输出与改动前逐行一致，没有块、没有收尾行。
7. 清理本刀在中转根与 scratchpad 里的全部中间物。
