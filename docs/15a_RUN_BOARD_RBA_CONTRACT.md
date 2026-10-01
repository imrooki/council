# RB-A 冻结契约：表头已用时间 + 轮次观察者通道 + 表头总览（v1.3，2026-09-24）

> **归档说明（2026-09-24）**：这是 RB-A（#41 第 1、2 刀）派工时的冻结契约原文，从中转根 `E:\multi_model_cowork\rb_a_contract.md` 原样归档到这里，给第 3 刀（RB-B）起草时复用 §1.1、§3.1 与 §4。
> 文中的行号都指 HEAD `3143bc8`，落地提交是 `69fed6c`（实现与 6 个测试文件）与其后补 T4b 的那一条。文中出现的 `E:\` 路径是当时的中转与隔离目录，已清理。
> 「只长不缩」探针的结论与 RB-B 的硬要求不在本文，见 `00_DECISIONS.md` #41。
> **2026-09-25 补**：RB-B（`572467c`）改写了本文的部分条款：两种事件追加了 `Speakers` / `Results` 切片字段（事件不再可比较，本文 R2-R4、O1、O2 的整值比较已按 RB-B 契约 §1.2 改成只比冻结字段）；E6 的 roundEndMsg、E7 的 submitDone 在「本次提交出过看板」时不再原样；C3/C5 的发射点带上新字段。逐条对照见 `15b_RUN_BOARD_RBB_CONTRACT.md` 开头的「与 RB-A 的关系」。


> **落地状态**：v1.2 已实现并提交 `69fed6c`（实现 grok-4.7，测试 Opus 测试腿，§4 #1-#50 中 49 条被杀、#36 等价）。v1.3 只改测试侧与注释措辞，生产代码不变。
>
> v1.3 相对 v1.2 的改动（落地后独立审查 1 major、5 minor）：新增 T4b 与变异 #51（E5 丢掉原 Cmd 没有用例抓）；§1.1 A 的 MemberDone 调用纪律、C7、F 的措辞与落地的注释对齐（notifyEach 那两条路在 Submit goroutine 上、不持锁；program 接口里只有 Send 与 Run）；T6「四个 bool」改三个；#41 杀手只留 T5；等价清单补三条；§2 通用纪律允许前提守卫用子串。

> v1.2 相对 v1.1 的改动（W1b 只审 v1.1 的改动，无 blocker / major，实测了 R6a-c 与 #46-#48）：R6c 的括号理由改成实测走的路（refused、进程没起，Ask+Post 只是保险；只 Ask 不 Post 会零事件）；R6 前提段补「三条的 Submit 返回非 nil 写错误属预期」；O4 假 CLI 睡眠 2300 → 2800ms（断言文字不变）；T2【判别力】里 H13a 的措辞。生产代码的要求一个字没变。
>
> v1.1 相对 v1 的改动（派工前对抗审 W1 的 3 条 major、3 条 minor，三条 major 各经一个证伪者确认）：T3 补续订那一拍的内容断言（带 1.5 秒超时）；O4 第 1 条补一句 2s；新增 T8（起点重取、已用时长清零）；新增 R6（写屏幕失败不发事件）；C4 订正「结果等价」的措辞；§4 补 7 条变异（#44-#50），#31 改杀手，#36 挪进等价清单，新增「只在审查时看」一条（F 那一行不许包 goroutine）；§6 第 1、4、5 步补核对项、变异条数与剩余项。生产代码的要求一个字没变。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `3143bc8`。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`，bubbletea `charm.land/bubbletea/v2 v2.0.9`。
- 来源：`docs/00_DECISIONS.md` #41（TUI 第 2 刀运行看板，1927 行起）分刀里的第 1 刀（表头已用时间）与第 2 刀（观察者通道 + 表头总览）合并成一刀；`docs/15_TUI_SPEC_MAPPING.md` §3.1 及其 2026-09-24 订正。
- **活区高度不变**：帧高仍是 8（表头 1 行 + 横线 1 行 + textarea 6 行）。X-39e 的契约与用例（`x39e_*_test.go`）一个字不动，本刀落地后必须原样全绿。
- 与别的刀的关系：RB-B（成员行进活区、只长不缩的高度策略）将复用本刀的 `repl.RoundObserver`、三种事件、TUI 侧的 Run 状态与 tick 机制；本刀不做 RB-B 的任何部分。本刀改动的生产文件只有 §1.1 列出的 6 个，与 turn 包、app 包、store 包不相交。
- 本契约的全部期望值（表头整串、行式 /round 的 golden、事件顺序、cancel / drift / post_rejected 路径上的字段）已在隔离拷贝里用一份原型实现实跑得出，原型不交付、已删除。

---

## 0. 病根 / 动机（行号以 HEAD 为准）

1. **忙时表头不动。** `internal/tui/model.go:230` 表头是 `fmt.Sprintf("council %s   %s", m.version, m.status)`，`beginSubmit`（`:242-265`）把 status 置成「正在运行」之后，直到 `submitDoneMsg`（`:151-164`）之前没有任何东西再改它。一轮 /round 的硬超时是 5 分钟、期间零输出（D-11 推翻理由，`00_DECISIONS.md:199-201`「每个成员一行活的计时器……光这一件就能解决『我不知道它有没有反应』」），用户分不清在跑、卡死、还是敲错了。
2. **repl → tui 没有结构化通道。** REPL 的全部输出经 `LineWriter` 变成 `printLine`（`run.go:83`、`model.go:116-122`），在事件循环里是 bubbletea 的未导出类型 `printLineMessage`（`tea.go:869-870` 先 insertAbove，再照样进 Update），Model 拿不到「开轮 / 谁回来了 / 收轮」这些结构化事实。唯一的钩子 `turn.RoundProgress` 在 `round.go:304-321` 只喂给 `writeAskRecord`。
3. **`runOneRound` 不知道自己是第几轮。** `round.go:248` 的签名已收 `label`，但 `cmdRound` 的 for 循环（`:188-216`）里的 `k` 与 `rounds` 没有传进去；裁判轮（`:235`）是单独一段。
4. **tick 不能装在 `beginSubmit` 返回的 Cmd 上。** `beginSubmit` 返回 `tea.Sequence(回显..., submitCmd)`（`:264`）；既有冻结用例 `TestTui1bAcc6SubmitEchoesEachLineBeforeSubmitting`（`tui1b_model_test.go:622-641`）要求这条 Cmd 展开后**恰好**是 `["print:> a","print:> b","submitDone"]` 且没有 `tea.BatchMsg`。把 tick 用 Batch 并进去会撞 `sawBatch`，放进 Sequence 会多一个叶子（`tea.Tick` 的 Cmd 是同步 `<-t.C`，`commands.go:154-164`，还会让 Submit 晚 1 秒）。所以本刀用「**提交后的下一条 Update 再装 tick**」（§1.1 E5），不改任何既有用例。
5. **已知代价，照实写**：表头过宽时由渲染器裁掉右侧、不折行（#41 硬约束「帧高只数行数……过宽的行会被裁掉，不会折行」），窄窗口下已用时间可能被裁掉看不见。本刀不处理。

---

## 1. 范围

### 1.1 做

凡写「名字实现方定」处实现方可自取名字（不许以 `rba` 开头）；其余名字、签名、字面量、顺序全部冻结。

#### A. `internal/repl/observer.go`（新文件）

逐字照下面的声明写（注释可以扩写，但不许改任何标识符、字段顺序、字段类型；`import "council/internal/turn"`）：

```go
// RoundObserver 接收 /round 每一段的三种事件。nil 表示不观察，行式路径不装。
//
// 调用纪律（实现方必须遵守，TUI 适配器依赖它）：
//   - RoundStart / RoundEnd 在执行 Submit 的那个 goroutine 上调用，不持任何锁。
//   - MemberDone 通常在 turn 包的 worker goroutine 上、RunRoundWithStream 的 progressMu 里调用；
//     铸 round id 失败或 round_started 写不进时，turn 改走 notifyEach，在执行 Submit 的那个
//     goroutine 上同步调用、不持锁。两种情形在同一段内都串行。实现只许做 O(1) 的轻活
//     （例如投递一条小消息），不许回调 Runner 或 REPL，不许等待任何要取 progressMu 的东西。
//   - 事件是值，接收方可以留着用，与发射方不共享任何可变内存。
type RoundObserver interface {
	RoundStart(ev RoundStartEvent)
	MemberDone(ev MemberDoneEvent)
	RoundEnd(ev RoundEndEvent)
}

// RoundStartEvent 在打出「本轮问 N 个: ...」之后、起进程之前发出。
type RoundStartEvent struct {
	Label   string // "发言轮" 或 "裁判轮"，即 runOneRound 的 label 参数
	K       int    // 发言轮：第 K 轮（1 起）；裁判轮：0
	N       int    // 发言轮：本条命令的 --rounds 总数；裁判轮：0
	Members int    // len(plan.Speakers)，skipNoDelta 之后、真正起进程的人数
}

// MemberDoneEvent 在一个成员的明细文本全部写完之后发出。只带小值，不带 turn.Record。
type MemberDoneEvent struct {
	Label         string       // 同本段 RoundStartEvent.Label
	AgentID       string       // rec.AgentID
	Outcome       turn.Outcome // rec.Outcome
	MaybeAdvanced bool         // rec.MaybeAdvanced
	Drifted       bool         // rec.Drifted
	Dropped       bool         // rec.DroppedText != ""
	WallMS        int          // rec.WallMS
	RoundID       string       // rec.RoundID（铸 id 失败时是空串）
}

// RoundEndEvent 在本段结算文本打完之后发出。
type RoundEndEvent struct {
	Label string
	K     int
	N     int
}

// SetRoundObserver 晚绑定观察者（照 LineWriter.SetEmit 的先例），传 nil 即卸下。
// 不是并发安全的：只许在还没有 Submit 在跑的时候调用（TUI 在 p.Run 之前装）。
func (r *REPL) SetRoundObserver(obs RoundObserver) {
	r.<字段> = obs
}
```

`<字段>` 是 B 里加的字段，名字实现方定。

#### B. `internal/repl/repl.go`

- `REPL` 结构体（`:46-97`）末尾加一个 `RoundObserver` 类型的未导出字段，名字实现方定，零值 nil。
- **`New`（`:118-136`）签名与函数体一个字不改**（16 个调用点）。

#### C. `internal/repl/round.go`

- **C1 签名。** `runOneRound` 改成
  `func runOneRound(ctx context.Context, r *REPL, plan turn.Plan, goRun bool, label string, k, n int, snap turn.RoundSnap) (bool, error)`。
  调用点只有两处，逐字改成：`:202` → `runOneRound(ctx, r, plan, goRun, "发言轮", k, rounds, snap)`；`:235` → `runOneRound(ctx, r, judgePlan, goRun, "裁判轮", 0, 0, judgeSnap)`。
- **C2 读一次。** 在 `:288-290`「本轮问」那次 `r.printf` 返回 nil **之后**、`RunRoundPreparedStream` 之前，把观察者字段读进一个局部变量（名字实现方定，下文记作 `obs`）；本段的三种事件一律经这个局部变量发，不再读字段。
- **C3 开轮。** 紧接 C2：`obs != nil` 时调用
  `obs.RoundStart(RoundStartEvent{Label: label, K: k, N: n, Members: len(plan.Speakers)})`。
  `plan` 是 `:265` skipNoDelta 之后的那个。`:249-273` 的四条早退路径（BudgetWith 出错、没带 `--go`、argv 整轮拒绝、没有人可问）上**一个事件都不发**。
- **C4 成员结束。** 在 onDone 闭包（`:304-321`）里：本次调用写完 `writeAskRecord`、并且（`rec.Err != nil` 时）写完那一行 `"/round %s: %s\n"` 之后，`obs != nil` 时调用
  `obs.MemberDone(MemberDoneEvent{Label: label, AgentID: rec.AgentID, Outcome: rec.Outcome, MaybeAdvanced: rec.MaybeAdvanced, Drifted: rec.Drifted, Dropped: rec.DroppedText != "", WallMS: rec.WallMS, RoundID: rec.RoundID})`。
  它必须是闭包在成功路径上的最后一个动作。**本次调用里任何一次写屏幕失败（进门时 `writeErr` 已非 nil，或本次的 `writeAskRecord` / `"/round ..."` 那一行返回错误）时不发**。现有「`/round` 那一行写失败只记 `writeErr`、不 return」的写法要改成写失败即 return。（在 HEAD 上两种写法等价，因为那一支之后本来没有别的动作；本刀在那一支之后加了 MemberDone，不 return 就会在写失败后照发事件，所以这一改是必需的，由 R6c 担保。）
  铸 id 失败与 round_started 写不进时 turn 走 `notifyEach`（`scheduler.go:103-113`、`:174-181`），回调照样逐个触发，这两条路上 MemberDone 照发、`RoundID` 为空串——不为它们另写分支。
- **C5 收轮。** `:340-342` 结算那次 `r.printf` 返回 nil 之后、`return true, nil` 之前，`obs != nil` 时调用 `obs.RoundEnd(RoundEndEvent{Label: label, K: k, N: n})`。`:322-324` 的 `writeErr != nil` 早退与结算写失败的早退上不发。
- **C6 行式不变。** 观察者为 nil 时本函数与 HEAD 逐字节同输出；观察者非 nil 时屏幕文本**也**与 HEAD 逐字节相同（事件不打印任何东西）。不许在发射点附近新增任何 `r.printf` / `r.write`。
- **C7 注释。** `runOneRound` 的文档注释（`:239-247`）补一段：k、n 的含义（裁判轮传 0,0），以及三个发射点的位置和「先有文本、再有事件」的顺序理由；`:291-302` 那段注释补一句 MemberDone 在回调末尾发（通常在 progressMu 里；notifyEach 那两条路上在本 goroutine、不持锁）、观察者只许做轻活。措辞实现方定，不许写与本契约不符的话。

#### D. `internal/tui/roundobs.go`（新文件，文件名实现方定）

逐字冻结的声明（方法体按注释实现；`import` 需要 `fmt`、`time`、`tea "charm.land/bubbletea/v2"`、`"council/internal/repl"`）：

```go
// 三种事件在事件循环里的样子。字段名 ev 冻结，测试腿按它构造与比较。
type roundStartMsg struct{ ev repl.RoundStartEvent }
type memberDoneMsg struct{ ev repl.MemberDoneEvent }
type roundEndMsg struct{ ev repl.RoundEndEvent }

// elapsedTickMsg 是表头计时的一拍。gen 是发出这一拍时的代次，at 是 tea.Tick 交来的时刻。
type elapsedTickMsg struct {
	gen uint64
	at  time.Time
}

// roundObserver 把 repl 的轮次事件原样包成上面三种消息交给 send。
// 生产上 send 就是 (*tea.Program).Send。不持任何锁、不读写任何共享状态、不调 REPL / Runner。
type roundObserver struct{ send func(tea.Msg) }

func newRoundObserver(send func(tea.Msg)) roundObserver { return roundObserver{send: send} }

func (o roundObserver) RoundStart(ev repl.RoundStartEvent) { o.send(roundStartMsg{ev: ev}) }
func (o roundObserver) MemberDone(ev repl.MemberDoneEvent) { o.send(memberDoneMsg{ev: ev}) }
func (o roundObserver) RoundEnd(ev repl.RoundEndEvent)     { o.send(roundEndMsg{ev: ev}) }

// formatElapsed 把已用时间印成表头上的那一格。规则与边界见契约 §3.2。
func formatElapsed(d time.Duration) string
```

同一文件里还放表头拼接用的辅助函数与 Run 状态类型，名字实现方定，拼法见 §3.1。

#### E. `internal/tui/model.go`

- **E1 `Options`**（`:32-45`）末尾加字段 `Now func() time.Time`（注释：nil 时用 `time.Now`；测试注入假钟用）。
- **E2 `Model`**（`:49-78`）加字段（名字实现方定，**唯一例外**：代次字段叫 `tickGen`、类型 `uint64`）：时钟函数、本次提交的起点时刻、已用时长、`tickGen`、「待装 tick」标志、本次提交的 Run 状态（当前段的 label、k、n、members、已回来数、roundID）。
- **E3 `New`**：时钟取 `opts.Now`，nil 时取 `time.Now`。`tickGen` 初值 0。其余不变。
- **E4 `beginSubmit`**（`:242-265`）：在置 `busy = true`、`status = "正在运行"` 的同一处，依次做：清空 Run 状态；起点 = 时钟()（时钟字段为 nil 时兜底 `time.Now`，与 `:257-260` 的 ctx 兜底同一写法）；已用时长 = 0；`tickGen++`；置「待装 tick」。**返回的 Cmd 一个字节不变**：仍是 `tea.Sequence(回显..., submitCmd)`，不许把 tick 并进去（§0 第 4 条）。于是第 1 次真提交的 tick 代次是 1，第 2 次是 2，以此类推；空输入不提交（`:244-246`）时这些都不做。
- **E5 提交后的下一条 Update 装 tick（冻结）。** `Update` 对**任意**消息：记下处理前的 `busy && 待装`；按下面 E6 与既有分支处理这条消息；若处理前那个条件为真、且处理后仍 `busy`、`tickGen` 未变，则把返回的 Cmd 换成 `tea.Batch(原 Cmd, tick)` 并清掉「待装」，否则原样返回。tick 是
  `tea.Tick(time.Second, func(t time.Time) tea.Msg { return elapsedTickMsg{gen: <装的时候的 tickGen>, at: t} })`。
  开始提交的那次 Enter 本身不装（处理前 busy 为 false），所以 Acc6 不受影响；真程序里回显那条 `printLineMessage` 必然紧跟着进 Update，tick 在毫秒内装上。结构（把旧 Update 改名成内部函数再包一层，或别的写法）实现方定，行为冻结。
- **E6 新分支。** 这四种消息都在进 textarea 之前返回，**不交给 textarea**，返回的 Cmd 除 E5 的 tick 外一律 nil（不许返回 `tea.Println`）：
  - `elapsedTickMsg`：`!busy` 或 `msg.gen != tickGen` 时什么都不改、返回 nil（**只在 busy 时续订**，旧代次的拍丢掉）。否则已用时长 = `msg.at.Sub(起点)`，返回下一拍（同一个 `tickGen` 的 `tea.Tick(time.Second, ...)`）。**不碰 `status`**。
  - `roundStartMsg`：`!busy` 时忽略。否则当前段整体换成 `{label: ev.Label, k: ev.K, n: ev.N, members: ev.Members, 已回来: 0, roundID: ""}`。
  - `memberDoneMsg`：`!busy` 或当前没有段时忽略。否则已回来数 +1；当前段 roundID 为空串时取 `ev.RoundID`（可能仍是空串），非空后不再改。
  - `roundEndMsg`：不改任何状态。
- **E7 `submitDoneMsg`**（`:151-164`）：在现有 `m.busy = false; m.cancel = nil` 旁边清空 Run 状态。其余三支（退出、出错退出、回空闲）的 status 文字与 Cmd 一个字不变。
- **E8 `chrome()`**（`:223-233`）：表头改成 `fmt.Sprintf("council %s   %s", m.version, <状态格>)`，状态格按 §3.1 拼。仍然只返回两行（表头、横线），横线与宽度规则不变；`frameHeight`（`:237-239`）不改。
- **E9** Ctrl-C（`:168-177`）、Ctrl-J、Enter（`:183-188`）、Esc、PasteMsg、WindowSizeMsg、`quitCmd`、`submitCmd`、`printLine`、`View` 的现有行为一个字不改（它们返回的 Cmd 只会被 E5 在「提交后第一条 Update」时并进一个 tick）。

#### F. `internal/tui/run.go`

- 在 `Run` 里、`p := tea.NewProgram(...)`（`:44-49`）之后、`return runWith(p, out, stdout)`（`:52`）之前加：

  ```go
  	if r != nil {
  		r.SetRoundObserver(newRoundObserver(p.Send))
  	}
  ```
  `r != nil` 这道门是必需的：既有用例 `TestTui1dRunFinalIsRealStdout`（`tui1d_run_final_test.go:85`）以 `Run(ctx, nil, ...)` 调用，没有这道门会空指针 panic（原型实测）。
- **不装在 `runWith` 里、不挨着两次 SetEmit**：`runWith(p program, out, final)`（`:59`）手里没有 REPL，`program` 接口里也拿不到 REPL（只有 Send 与 Run）；x39e 与 tui1d 的既有用例直接调用 `runWith(p, lw, final)`，它的签名因此冻结。在 Run 里装与在 runWith 第 1 步前装在时序上等价：两处都在 `p.Run()` 之前、都在任何 Submit 之前。这一行上方写一段注释说明这三点（为什么在这里、为什么要 nil 门、为什么不会在 Run 之前 Send）。
- `runWith`、`exitClearSeq`、`replAdapter`、`program` 接口、`tea.NewProgram` 的选项一个字不改。

#### G. 并发与锁（冻结的论证，实现方照此实现，不许加锁）

1. 适配器每个方法体只有一句 `o.send(...)`，不持有任何锁、不读写任何共享状态、不调 REPL 或 Runner。
2. RoundStart / RoundEnd 在 Submit goroutine 上调用：此刻 `r.printf` 已返回，不持 LineWriter.mu（`writer.go:169-196` 的禁令针对的正是这把锁），也不持 progressMu。
3. MemberDone 在 worker goroutine 上、progressMu 内调用，`p.Send` 会阻塞到事件循环收下。事件循环（Update / View / 渲染）**从不取 progressMu、从不同步等 REPL 或 Runner**：Update 对这四种新消息只改 Model 字段并立即返回，所以没有等待环。这与 HEAD 上 `writeAskRecord → LineWriter.Write → emit(p.Send)` 已经在 progressMu 里发生是同一形状（`scheduler.go:231-233`）。代价是既有的那一条：慢的 Update 会延长 progressMu 与信号量槽位的持有（`scheduler.go:219-230` 注释），这四个分支都是 O(1)。
4. 观察者在 `p.Run()` 之前装上，而第一个 Submit 只可能在事件循环起来之后由 Enter 触发，所以不存在 Run 之前的 Send（`15` §16 的禁令）。Run 返回后 `p.Send` 走 `p.ctx.Done()` 那一支，不阻塞（`tea.go:1192-1196`）。
5. 同一段内的事件顺序由结构保证：RoundStart 与「本轮问」那一行在同一 goroutine 上先后发生；MemberDone 与该成员的明细在同一次 onDone 调用里先后发生，onDone 被 progressMu 串行化；RoundEnd 在 `RunRoundPreparedStream` 返回（`wg.Wait` 之后）再打完结算之后。`p.Send` 与 `LineWriter` 的 emit 都是同步交给同一个无缓冲 `p.msgs`，所以事件循环收到的顺序就是发射顺序。

### 1.2 不做（两条腿都不许越界）

- 不改 `internal/turn` 的任何文件（`RoundProgress`、`RunRound*` 全家的签名冻结，见 `scheduler.go:84-91` `RunRoundPrepared` 与 `:140-147` `RunRoundWith` 的注释「签名一个字不改」）；不加 onStart 钩子。
- 不改 `repl.New`、`repl.Submit`、`writeAskRecord`、`renderBudget`、`renderSettlement`、`askCause` 以及 `cmdRound` 里除 C1 两个调用点以外的任何一行。
- 不改活区高度、不加成员行（那是 RB-B）；不加字形（● ◐ ○ × 等一概不用，本刀新增字面量只有 ASCII、中文与 `/`、`(`、`)`）；不做展开折叠、不加快捷键、不改 Compare view。
- 不改 `runWith`、`exitClearSeq`、`LineWriter`、`printLine`、`quitCmd`。
- 不改 `internal/app`、`cmd/`、`go.mod`。
- **既有 `_test.go` 一个都不许改**（含注释）。本刀没有授权修改任何既有用例。已知副作用：`TestTui1bAcc8BusyCtrlCCancelsWithoutQuitting` 的第二个 Cmd 会带上 E5 装的 tick，`tui1bRun` 同步跑它，这条用例从约 0 秒变成约 1 秒（原型实测 1.00s，仍绿）。这是预期，不是失败。
- 测试腿不写生产代码，实现腿不写 `_test.go`。

---

## 2. 验收（测试腿新写的文件）

通用纪律：

- 包级标识符一律以 `rba` 开头（类型、函数、变量、常量）；唯一例外是 `TestMain`（Go 要求这个名字）。测试函数名 `TestRba...`。
- 允许只读引用同包既有测试辅助：repl 包的 `newRoundStreamService`（`step10ux1_round_stream_test.go:98-113`）与 TestMain 假 CLI（`session_inflight_test.go:85-127`）；tui 包的 `tui1bSubmitter`、`tui1bPrinter`、`tui1bNopDrainer`、`tui1bStep`、`tui1bRun`、`tui1bType`、`tui1bStripANSI`、`tui1dBuf`、`tui1dWaitFor`、`tui1dESC`。不许改它们。
- 不起真 provider：假 CLI 一律是测试二进制自己（`COUNCIL_FAKE_CLI=1`，Command 写 `os.Args[0]`）。store 一律 `store.Open` 到 `t.TempDir()` 下，不碰 `C:\Users\25142\.council\data`。
- 断言整行相等或整行正则（`^...$`）；不写子串断言。唯一例外是 W1（渲染字节流，见该条理由）。`Fatalf` 前提守卫（前提不成立即停、不是被测断言）允许用子串，例如 R5 的 `strings.Contains(buf, "成员  输入token")`。
- 每条注释写【判别力】：能不能为假、只差一处的对照是哪条。
- HEAD 上的状态：除 `rba_golden_test.go` 外，所有新文件引用本刀新增的标识符，**在 HEAD 上整包编译不过，这是预期的红**。`rba_golden_test.go` 不引用任何新标识符，测试腿交付前必须在 HEAD 的隔离拷贝里**只放这一个新文件**跑一次，证明 R1 在 HEAD 上是绿的（证明 golden 是 HEAD 的真输出），在报告里写出命令与结果。

### 2.0 共用装置（冻结）

**repl 包装置**（`rba_golden_test.go` 里定义，`rba_observer_test.go` 共用）：

- 配置：`Providers: {"claude": {Command: os.Args[0]}}`；三个 agent，`DisplayName: "X"`、`Provider: "claude"`、`Model: "claude-fable-5-1"`、`Effort: "high"`、`Role: "rba"`、`Permissions: config.PermissionRead`：`rba_a`、`rba_b` 的 `Mode` 是 `config.ModeDiscussion`，`rba_j` 的 `Mode` 是 `config.ModeJudge`。
- 顺序：`t.Setenv("COUNCIL_FAKE_CLI", "1")`；`svc := newRoundStreamService(t)`；`rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})`；`svc.Create(ctx, "rba")`；按 `rba_a`、`rba_b`、`rba_j` 的顺序 `svc.Join(ctx, id, conversation.Participant{AgentID: id, Provider: "claude", JoinedAt: time.Now().UTC()})`；`svc.Post` 一条 `SenderID: conversation.ReservedSenderUser`、`Kind: conversation.KindUser`、`Text: "seed-msg"`。
- REPL：`New(cfg, svc, rn, nil, out, RulesContext{})`，`out` 是一个自带互斥锁的缓冲（包级类型 `rba...`）。先 `r.Submit(ctx, "/use "+convID)`，**清空缓冲**，再 Submit 被测命令。直接调 `Submit`（不走 `Run`，不会有提示符）。

**tui 包装置**（`rba_fakecli_test.go`）：

- `TestMain`：`os.Getenv("COUNCIL_FAKE_CLI") == "1"` 时扮演 claude CLI：读完 stdin；环境变量 `RBA_FAKE_SLEEP_MS` 能解析成正整数时先睡这么多毫秒；若环境变量 `RBA_FAKE_RESUME_SESSION` 非空且 `os.Args[1:]` 里有 `"--resume"`，把 JSON 里的 `2a90d449-a348-497a-8812-e73e74d6898e` 替换成它（只替换第一次出现）；把 JSON 打到 stdout，`os.Exit(0)`。否则 `os.Exit(m.Run())`。本包此前没有 TestMain（现查零命中）。
- JSON 常量逐字节复制 `internal/repl/session_inflight_test.go` 的 `fakeClaudeSuccessJSON`（不手写，理由见那条常量的注释）；本文件注释照那条注释的体例登记「这是第 5 份拷贝」。
- 配置与会话种子同 repl 包装置（三个 agent、`seed-msg`），store 用 `store.Open(ctx, filepath.Join(t.TempDir(), "council-data"))` 包 `conversation.NewService`，`t.Cleanup` 关 store。REPL 的 `out` 是 `*LineWriter`：`repl.New(cfg, svc, rn, nil, lw, repl.RulesContext{})`。装置函数要把 `svc` 与 `convID` 交回给用例（O3 要调 `svc.Leave`）。
- **真 Program 装置**（O1-O4 用）：`lw := NewLineWriter(nil)`；`m := New(Options{Submit: replAdapter{r: r}, Out: lw, Version: "t", Ctx: ctx})`；一个包级 spy 类型包住 Model（`Init`、`View` 委托；`Update` 先委托、把返回的 `tea.Model` 断回 `Model` 存回去，再按消息类型记一条日志：`roundStartMsg`、`memberDoneMsg`、`roundEndMsg`、`submitDoneMsg`、`elapsedTickMsg` 各记类型与载荷，`fmt.Sprintf("%T", msg) == "tea.printLineMessage"` 时用 `reflect.ValueOf(msg).FieldByName("messageBody").String()` 记正文；每条都同时记下 Update 之后的 `chrome()[0]`）；日志带互斥锁。
  `p := tea.NewProgram(spy, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignalHandler(), tea.WithWindowSize(100, 24))`；然后**按这个顺序**：`r.SetRoundObserver(newRoundObserver(p.Send))`；`lw.SetEmit(func(line string) { p.Send(printLine(line)) })`；`go p.Run()`。收尾 cancel ctx 并等 Run 返回（5 秒超时）。
  提交一条命令 = `p.Send(tea.PasteMsg{Content: 文本})` 再 `p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})`。先提交 `"/use "+convID` 并等到第 1 条 submitDone，再提交被测命令。

### 2.1 `internal/repl/rba_golden_test.go`（HEAD 上必须绿）

**R1 nil 观察者的行式 golden。** 装置同 2.0；被测命令 `"/round rba_a rba_j --go --with-judge"`；缓冲内容按 §3.4 的两条规则规范化后，必须与 §3.4 的 golden **整串相等**。
【判别力】发射点附近多打任何一行、或 nil 观察者被解引用（panic）都红。对照：R2 同一装置、同一命令，只多装一个观察者。

### 2.2 `internal/repl/rba_observer_test.go`

**R2 装了观察者，屏幕文本仍与 golden 相等。** 同 R1，只在 `/use` 之后、被测命令之前 `r.SetRoundObserver(rec)`，`rec` 是记录型观察者。规范化后与 §3.4 golden 整串相等；并且 `rec` 收到的事件恰好是 `[RS{发言轮,1,1,1}, MD{rba_a}, RE{发言轮,1,1}, RS{裁判轮,0,0,1}, MD{rba_j}, RE{裁判轮,0,0}]`（RS/RE 用结构体整值相等比较；MD 比 `Label`、`AgentID`、`Outcome == turn.OutcomeSpoke`、三个布尔为 false）。
【判别力】观察者被调用时顺手 printf 一行 → 与 golden 不等；只在 nil 时才不打印的写法也被抓。

**R3 顺序与快照（`--rounds 2 --with-judge`）。** 装置同 2.0，被测命令 `"/round rba_a rba_b rba_j --go --rounds 2 --with-judge"`（第 2 轮两人带 `--resume`，repl 包的假 CLI 各睡 700ms，属预期）。记录型观察者在**每个事件进门时**把缓冲当前内容整串存下（快照）。断言：
1. 事件种类序列恰好是 `RS MD MD RE RS MD MD RE RS MD RE`。
2. 三个 RS 依次整值等于 `{发言轮,1,2,2}`、`{发言轮,2,2,2}`、`{裁判轮,0,0,1}`；三个 RE 依次整值等于 `{发言轮,1,2}`、`{发言轮,2,2}`、`{裁判轮,0,0}`。
3. 每个 RS 的快照按 `\n` 切行后最后一个非空行整行等于 `本轮问 2 个: rba_a, rba_b`（前两个）/ `本轮问 1 个: rba_j`（第三个）。
4. 每个 MD：取本快照相对上一个事件快照新增的文本，按行切：第一行整行等于 `"outcome spoke  agent " + ev.AgentID`；块内以 `outcome ` 开头的行恰好 1 行；块内恰好 1 行匹配 `^冷暖 预报=(冷|暖) 实测=(冷|暖|未知)  wall_ms=(\d+)  prompt_bytes=\d+  scanned_to=\d+$`，且捕获的 wall_ms 等于 `ev.WallMS`。
5. 每个 MD：`Label` 等于本段 RS 的 Label；`Outcome == turn.OutcomeSpoke`；`MaybeAdvanced`、`Drifted`、`Dropped` 为 false；`RoundID` 非空，同段相等，三段两两不等。每段两个 MD 的 AgentID 集合为 `{rba_a, rba_b}`、`{rba_a, rba_b}`、`{rba_j}`。
6. 每个 RE：相对上一个事件（该段最后一个 MD）新增文本的第一行整行等于 `Label + "结算"`，且块内没有以 `成员  ` 开头的行（下一段的预算表还没打）。
【判别力】MD 挪到 writeAskRecord 之前 → 第 4 条块为空，红；RE 挪到结算之前 → 第 6 条红；RS 挪到「本轮问」之前 → 第 3 条红；k/N 传反或裁判传 1,1 → 第 2 条红；WallMS 或 RoundID 映射错 → 第 4、5 条红。

**R4 Members 取 skipNoDelta 之后的名单。** 装置同 2.0，另外在 `/use` 之前先 `rn.Ask(ctx, convID, "rba_b")` 一次（之后不再发新消息，rba_b 于是无新内容）；被测命令 `"/round rba_a rba_b --go"`。事件恰好是 `[RS{发言轮,1,1,1}, MD{rba_a}, RE{发言轮,1,1}]`；RS 快照最后一个非空行整行等于 `本轮问 1 个: rba_a`（原型实测：预算表两行、rba_b 那行依据是「无新内容，本轮跳过」）。
【判别力】`Members: len(rows)`（2）红。对照：R3 同一字段在没人被跳过时等于名单长度。

**R5 早退路径不发事件。** 装置同 2.0，装记录型观察者，被测命令 `"/round rba_a rba_j --with-judge"`（没有 `--go`）。事件列表为空；并且再用一个全新装置、不装观察者跑同一命令，两份缓冲各自按 §3.4 的两条规则规范化后整串相等。
【判别力】RS 挪到 `if !goRun` 之前、或 RE 放进 defer 在每条 return 上都发 → 红。

**R6 写屏幕失败时不发事件（C4、C5）。** 装置同 2.0，但 REPL 的 `out` 换成一个包级失败写者类型（`rba` 开头，repl 包测试里现在没有会写失败的写者，要新写）：自带互斥锁、内部缓冲照写；字段 `failOn string`（非空且本次写入的内容含它时，**不写入**、计数 +1、返回一个非 nil 错误）与 `onWrite func(s string)`（非 nil 时每次写入先调它，在判失败之前）。`/use` 与清空缓冲照 2.0；在被测命令之前装记录型观察者。三个子用例各用一个全新装置：
- **R6a**：`failOn = "outcome "`，被测命令 `"/round rba_a --go"`。事件恰好是 `[RS{发言轮,1,1,1}]`。
- **R6b**：`failOn = "结算"`，被测命令 `"/round rba_a --go"`。事件种类恰好是 `RS MD`，MD 的 `AgentID` 是 `rba_a`。
- **R6c**：在 `/use` 之前先 `rn.Ask(ctx, convID, "rba_a")` 一次，再 `svc.Post` 一条 `SenderID: conversation.ReservedSenderUser`、`Kind: conversation.KindUser`、`Text: "seed-msg-2"`。（实际走的路，W1b 实测：cancel 在写「本轮问」时同步触发，早于 `RunRoundPreparedStream`；进去后写 round_started 时 ctx 已取消，turn 走 `notifyEach`，结局是 refused、进程根本没起，被拦下的那一行是 `/round rba_a: round_started: ... context canceled`。Ask+Post 在这条路上不承重，只是保险：将来若写库不看 ctx、进程真起来了，有新内容的 rba_a 会带 `--resume`，repl 包的假 CLI 睡 700ms，cancel 仍落在飞行窗口里拿到 canceled。**只 Ask 不 Post 不行**：rba_a 会被 skipNoDelta 跳过，打出「这一轮没有人可问。」、零事件，按前提不成立红。）被测命令用一个可取消的 ctx 调 `r.Submit(cmdCtx, "/round rba_a --go")`；`onWrite` 在写入内容含 `本轮问` 时调 `cancel()`；`failOn = "/round rba_a: "`。事件恰好是 `[RS{发言轮,1,1,1}]`。
- 三条的前提（不成立就 `Fatalf` 说前提不成立，不算实现错）：失败写者的计数恰好是 1。R6c 另要求缓冲里有整行 `本轮问 1 个: rba_a`，且恰好有一行以 `outcome ` 开头（说明 writeAskRecord 写成功了、失败落在 `/round` 那一行；结局是 refused、canceled 还是别的不押）。
- **三条的 `r.Submit` 都返回非 nil 的写错误**（形如 `repl: 写输出失败: <失败写者的错误>`），这是预期：不当失败、不当前提不成立，**可以**断言它非 nil。不要照 R1-R5 的习惯写 `if err := r.Submit(...); err != nil { t.Fatalf(...) }`，那在正确实现上会假红。
【判别力】writeAskRecord 失败那一支 return 之前照发 MD（#46）→ R6a 变成 `RS MD` 红；`/round` 那一行写失败后不 return、回到 HEAD 写法（#47）→ R6c 变成 `RS MD` 红；结算写失败的早退上照发 RE（#48）→ R6b 变成 `RS MD RE` 红。对照：R4 的形状，写者不失败时同一装置跑 `/round rba_a ... --go` 是完整的 `RS MD RE`。

### 2.3 `internal/tui/rba_fakecli_test.go`

只放 2.0 的 TestMain、假 CLI、JSON 常量与装置函数，不放用例。

### 2.4 `internal/tui/rba_header_test.go`（纯 Model，不起 Program）

公共：`rbaT0` 是一个固定时刻；Model 用 `New(Options{Submit: &tui1bSubmitter{}, Out: tui1bNopDrainer{}, Version: "t", Println: (&tui1bPrinter{}).cmd, Now: func() time.Time { return rbaT0 }})`，再喂一条 `tea.WindowSizeMsg{Width: 100, Height: 24}`。「提交」= `tui1bType` 敲一个可见字符 + `tui1bStep(..., Enter)`（丢掉 Cmd，不跑）。**每次提交之后先喂一条 `tea.WindowSizeMsg{Width: 100, Height: 24}` 把 E5 的待装 tick 消费掉（丢掉它的 Cmd、不跑）**，再喂被测消息。表头一律取 `m.chrome()[0]` 整串比较。

**T1 `formatElapsed` 边界表**：逐条整串相等于 §3.2 的表（含负数）。
【判别力】每一个边界值都是某条变异的唯一杀手：`< 60` 写成 `<= 60`（60s）、`%02d` 写成 `%d`（65s、3720s）、小时格带秒（3659s）、不夹负数（-1s）。

**T2 表头整串序列**。按 §3.3 的步骤表逐步喂消息，每步之后表头整串等于表中那一行。
【判别力】状态文字被 tick 覆盖（H11、H12、H14）、不忙时仍拼已用时间（H13a，杀 #40；只在不忙时拼段、不拼时间的写法是等价的，E7 在 submitDone 时已清掉 Run 状态；H13b 只是回归钉、没有判别力，理由见 §4 等价清单里的 #36）、roundID 被后来者覆盖（H6）、取前 6 位而不是后 6 位（H5）、`n > 1` 写成 `n >= 1`（H9）、分隔写成一个空格（全部）、已用时长按拍数累加而不是 `at - 起点`（H3 一拍跳到 12s）、两处都不清 Run 状态（H15）各自被抓。

**T3 代次与 busy。** 提交第 1 次（代次 1）→ `submitDoneMsg{}` → 提交第 2 次（代次 2）→ 消费待装 tick → 喂 `elapsedTickMsg{gen: 1, at: rbaT0.Add(65 * time.Second)}`：表头整串仍是 `council t   正在运行  0s`，返回 Cmd 为 nil。再喂 `elapsedTickMsg{gen: 2, at: 同一时刻}`：表头整串是 `council t   正在运行  1m05s`，返回 Cmd 非 nil（续订）。**接着跑这条续订 Cmd**：它就是 E6 返回的那一条 `tea.Tick`（此时待装已被消费，E5 不会再并 Batch）；在新 goroutine 里调 `cmd()`，把结果送进容量 1 的 channel，测试 goroutine 用 `select` 最多等 1500ms：超时就 `Fatalf`（续订间隔不是 1 秒）；收到的消息必须能断言成 `elapsedTickMsg` 且 `gen == 2`（收到 `tea.BatchMsg` 或别的类型也 `Fatalf`）。**不许在那个 goroutine 里调 `t` 的任何方法**，也不许用 `tui1bRun`（它会调 `t.Fatalf`，而且没有超时：间隔写成 1 分钟时会让用例挂 1 分钟而不是红）。这一步多花约 1 秒，属预期。然后 `submitDoneMsg{}`，再喂 `elapsedTickMsg{gen: 2, at: rbaT0.Add(70 * time.Second)}`：表头整串是 `council t   空闲`，返回 Cmd 为 nil。
【判别力】前两次喂的消息只差 gen 一处（对照）；去掉 gen 检查 → 第一句红；续订那一拍代次写错（#49）→ `gen == 2` 红；续订间隔不是 1 秒（#50）→ 1.5 秒超时红；不看 busy 就续订 → 最后一句红。T3 是 #49、#50 的主杀手。

**T4 提交后下一条 Update 才装 tick、只装一次。** 提交后**不**消费待装：喂 `tea.WindowSizeMsg{100,24}` 得 Cmd1，`tui1bRun(t, Cmd1)` 展开后的叶子里恰好有一条 `elapsedTickMsg` 且 `gen == 1`（约 1 秒，预期）；再喂同样一条 `tea.WindowSizeMsg{100,24}` 得 Cmd2，Cmd2 必须是 nil。
【判别力】从不装 → Cmd1 为 nil 红；每条 Update 都装 → Cmd2 非 nil 红；两条消息完全相同，只差先后（对照）。在 Enter 那一次就装由既有 `TestTui1bAcc6SubmitEchoesEachLineBeforeSubmitting` 抓。

**T4b 提交后第一条 Update 装 tick 时保留原 Cmd（v1.3 补，落地后另起提交）。** 提交后**不**消费待装：喂一条会让 textarea 返回非 nil Cmd 的消息（`tui1bKey('z')`，或 `tea.PasteMsg{Content: "abc"}`）得 Cmd1，`tui1bRun(t, Cmd1)` 展开后：恰好 1 条 `elapsedTickMsg` 且 `gen == 1`，**并且至少还有 1 片不是 `elapsedTickMsg` 的叶子**（忠实实现上是 textarea 的光标闪烁消息，审查员实测 ticks=1、others=1）。前提：同一条消息在已消费待装之后喂，返回的 Cmd 非 nil（否则按前提不成立 Fatalf：说明这条消息本来就不产 Cmd，测不到丢弃）。
【判别力】E5 把原 Cmd 丢掉、只返回 tick（#51）→ others=0，红。T4 用的 WindowSizeMsg 本来就返回 nil Cmd，抓不到这一条。对照：前提那一步（同一条消息、已消费待装）。

**T5 帧高不变。** 在 T2 的 H5 状态（忙、有段、有 roundID）下：`m.frameHeight() == 8`，且 `strings.Count(m.View().Content, "\n") + 1 == 8`；喂 `tea.WindowSizeMsg{Width: 20, Height: 24}` 后两者仍是 8（表头比 20 宽也不折行）。
【判别力】把段信息做成第二行 → 红。对照：X-39e 的 A9 在空闲状态下钉同一个 8。

**T6 适配器逐字映射。** `newRoundObserver` 传一个记录型 `send`；分别调用三个方法，各传一个每个字段都非零值的事件（MemberDone 的三个 bool 用 true、Outcome 用 `turn.OutcomePostRejected`、WallMS 1234、RoundID 任意非空），`send` 恰好收到 1 条消息，且与 `roundStartMsg{ev: ev}` / `memberDoneMsg{ev: ev}` / `roundEndMsg{ev: ev}` `reflect.DeepEqual`。
【判别力】方法之间串线、丢字段、发指针 → 红。

**T7 新消息不产 Cmd、不打印。** 消费待装之后，喂 `roundStartMsg`、`memberDoneMsg`、`roundEndMsg` 各一条，返回的 Cmd 都是 nil。
【判别力】分支里返回 `tea.Println`（违反 #41「不能由 Update 返回 tea.Println」）→ 红。

**T8 每次提交都重取起点、已用时长清零（E4）。** 与公共装置只差时钟：`now := rbaT0`，`Now: func() time.Time { return now }`（闭包读用例里的局部变量，改它就改了时钟）。
1. 提交（代次 1），消费；喂 `elapsedTickMsg{gen: 1, at: rbaT0.Add(45 * time.Second)}` → 表头整串 `council t   正在运行  45s`。
2. 喂 `submitDoneMsg{}`；`now = rbaT0.Add(100 * time.Second)`；提交（代次 2），消费 → 表头整串 `council t   正在运行  0s`。
3. 喂 `elapsedTickMsg{gen: 2, at: rbaT0.Add(105 * time.Second)}` → 表头整串 `council t   正在运行  5s`。
【判别力】beginSubmit 不把已用时长清零（#44）→ 第 2 步显示 `45s` 红；起点只在第一次提交时取（#45，例如 `if 起点.IsZero()` 才取）→ 第 3 步显示 `1m45s` 红。T 系列别的用例时钟恒为 `rbaT0`，抓不到这两条；第 2、3 步只差已用时长的来源（对照）。

### 2.5 `internal/tui/rba_order_test.go`（真 `tea.Program`，真 REPL，真 Runner，假 CLI）

用 2.0 的真 Program 装置。日志里「P」指 printLine 正文，事件按记录顺序。

**O1 `--rounds 2 --with-judge` 的 Update 顺序。** 不设 `RBA_FAKE_SLEEP_MS`。提交 `"/round rba_a rba_b rba_j --go --rounds 2 --with-judge"`，等第 2 条 submitDone（30 秒超时）。只看第 2 次 Enter 回显那条 P（`> /round ...`）之后的日志：
1. 非 P、非 tick 的条目种类序列恰好是 `RS MD MD RE RS MD MD RE RS MD RE DONE`；RS/RE 载荷同 R3 第 2 条。
2. 每个 RS 之前最近一条 P 整行等于 `本轮问 2 个: rba_a, rba_b` / `本轮问 2 个: rba_a, rba_b` / `本轮问 1 个: rba_j`。
3. 每个 MD：与上一个事件之间的 P 块，第一条整行等于 `"outcome spoke  agent " + ev.AgentID`；块内以 `outcome ` 开头的恰好 1 条；块内恰好 1 条匹配 R3 第 4 条的冷暖正则且 wall_ms 等于 `ev.WallMS`。
4. MD 的字段断言同 R3 第 5 条。
5. 每个 RE 与上一个 MD 之间的 P 块第一条整行等于 `Label + "结算"`。
6. 表头（每条日志记下的 `chrome()[0]`）整行匹配：RS 处 `^council t   正在运行  发言轮 1/2  回来 0/2  \d+s$`、`^council t   正在运行  发言轮 2/2  回来 0/2  \d+s$`、`^council t   正在运行  裁判轮  回来 0/1  \d+s$`；每段第 j 个 MD 处 `^council t   正在运行  <段> \(<RoundID 末 6 位>\)  回来 j/<Members>  \d+s$`（`<段>` 是 `发言轮 1/2`、`发言轮 2/2`、`裁判轮`，末 6 位用 `regexp.QuoteMeta`）；DONE 处整串等于 `council t   空闲`。
【判别力】这是「跨并发原语的顺序」那一条：MD 在 progressMu 里经真 `p.Send` 投递，明细行经 LineWriter 经同一个 `p.Send` 投递，两者进 Update 的先后只有在真 Program 上才是真的。MD 挪到 writeAskRecord 之前 → 第 3 条红（块为空，或块属于另一个成员）。

**O2 取消路径。** `t.Setenv("RBA_FAKE_SLEEP_MS", "3000")`。提交同 O1 的命令；日志出现第 1 个 RS 后再等 300ms，`p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})`；等第 2 条 submitDone（15 秒超时）。断言：
1. 非 P、非 tick 条目序列恰好是 `RS MD MD RE DONE`（没有第 2 个 RS）；RS 整值 `{发言轮,1,2,2}`，RE 整值 `{发言轮,1,2}`。
2. 两个 MD：`Outcome == turn.OutcomeCanceled`、`MaybeAdvanced == true`、`Drifted`、`Dropped` 为 false。
3. 每个 MD 的 P 块：第一条整行等于 `"outcome canceled  agent " + ev.AgentID`；第二条整行等于 `provider 那边可能已经吃下这批内容`；**块的最后一条**以 `"/round " + ev.AgentID + ": "` 开头（askCause 那一行；它必须在 MD 之前）。
4. RE 与 DONE 之间恰好一条 P，整行等于 `已跑 1/2 轮，其余取消`。
5. 两个 MD 处的表头整行匹配 `^council t   正在取消  发言轮 1/2 \([0-9a-f]{6}\)  回来 [12]/2  \d+s$`；DONE 处整串等于 `council t   空闲`。
【判别力】MD 挪到 askCause 那一行之前 → 第 3 条最后一行红；`MaybeAdvanced` 写死 false → 第 2 条红；tick 或取消把「正在取消」盖掉 → 第 5 条红。对照：O1 同一命令不取消，走完三段。

**O3 Drifted 与 Dropped 走真路径。**
- O3a：`t.Setenv("RBA_FAKE_RESUME_SESSION", "11111111-2222-4333-8444-555555555555")`，提交 `"/round rba_a rba_b --go --rounds 2"`。第 1 段两个 MD 的 `Drifted` 为 false、第 2 段两个为 true，四个 `Outcome` 都是 `turn.OutcomeSpoke`；第 2 段每个 MD 的 P 块里恰好一条整行等于 `送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了`。
- O3b：`t.Setenv("RBA_FAKE_SLEEP_MS", "1500")`，提交 `"/round rba_a rba_b --go"`；日志出现 RS 后立刻 `svc.Leave(context.Background(), convID, "rba_b", time.Now().UTC())`（必须成功，否则按前提不成立 Fatalf）；等 submitDone。`rba_b` 的 MD：`Outcome == turn.OutcomePostRejected`、`Dropped == true`；`rba_a` 的 MD：`Outcome == turn.OutcomeSpoke`、`Dropped == false`；`rba_b` 的 P 块里恰好一条整行等于 `正文写不进库（已经付过钱），整段如下:`。
【判别力】`Drifted` / `Dropped` 写死 false → 红；同段 / 同轮另一个成员是对照。原型实测两条路径都按上述出现。

**O4 真 Program 上 tick 只在 busy 时走。** `t.Setenv("RBA_FAKE_SLEEP_MS", "2800")`。`/use` 的 submitDone 之后先等 1500ms（让 `/use` 那次装的旧 tick 落地），再提交 `"/round rba_a --go"`，等 submitDone（15 秒超时），再等 2500ms。断言：
1. 第 2 条 submitDone 之前**存在**至少一条 `elapsedTickMsg`，它那条日志记下的表头整行匹配 `^council t   正在运行  发言轮  回来 0/1  [1-9]s$`（N=1 不印 `1/1`）。不要求每一拍都匹配：落在 MD 之后的拍，表头是 `发言轮 (xxxxxx)  回来 1/1  3s` 之类，同样正确。
   另外，第 2 条 submitDone 之前**存在**一条 `elapsedTickMsg`，它那条日志记下的表头**整串等于** `council t   正在运行  发言轮  回来 0/1  2s`（续订的第 2 拍，约在 2.0s 到；假 CLI 睡 2800ms，这一拍比 MD 早约 800ms。W1 审查员在 2300ms 下实测连跑 8 次全绿；v1.2 为有负载的机器多留 500ms 余量）。
2. 第 2 条 submitDone 之后的 `elapsedTickMsg` 至多 1 条（在途的那一拍），且它们之后的表头整串都是 `council t   空闲`。
【判别力】从不装 tick → 第 1 条红；续订代次错（#49）或间隔错（#50）→ 第 1 条的 `2s` 那一句红（T3 是主杀手，这里是真 Program 上的辅助）；不看 busy 续订 → 2.5 秒里至少 2 拍，第 2 条红。

### 2.6 `internal/tui/rba_wiring_test.go`（真 `Run`，担保 F 那一行真的接上了）

照 `tui1d_run_ctx_test.go` 的形状：`os.Pipe` 两端做 in / stdout，读 goroutine 在 Run 之前起；tui 装置造 REPL（out 是 `lw`）；`t.Setenv("RBA_FAKE_SLEEP_MS", "2500")`；`go Run(ctx, r, lw, "test", inR, outW)`；收尾顺序照 tui1d 那条（cancel、等 Run、关 inW、inR、outW、等读 goroutine、关 outR，每步 5 秒超时）。

记 `flat` = `tui1bStripANSI(缓冲)` 再删掉所有 ASCII 空格（渲染器可能用光标移动跳过没变的空格，原型实测会整行重写，但不押这一点）。

1. 前提：5 秒内缓冲里出现 `tui1dESC + "[J"`。然后往 inW 写 `tui1dESC + "[8;24;100t"`（XTWINOPS 窗口尺寸报告：ultraviolet `decoder.go:656-663` 解成 `WindowSizeEvent`，bubbletea `input.go:42` 转成 `tea.WindowSizeMsg`，`tea.go:857-858` 让渲染器按 100×24 出帧；**原型实测可行**，这是管道上唯一能让表头文字进 stdout 的办法，Run 的选项冻结、不许加 `WithWindowSize`）。5 秒内 `flat` 含 `counciltest空闲`，否则按前提不成立 Fatalf。
2. 前提：写 `"/use " + convID + "\r"`，5 秒内 `flat` 含 `当前会话:`。
3. 主断言：写 `"/round rba_a rba_b --go\r"`，10 秒内 `flat` 含 `counciltest正在运行发言轮回来0/2`。
4. 主断言：10 秒内 `flat` 含 `counciltest正在运行发言轮回来0/21s`（真 Run 上 E5 的 tick 装上了、跑了一拍）。
5. 收尾前：10 秒内 `flat` 含 `发言轮结算`（这一轮确实跑完、子进程都回来了，不留子进程给别的用例）。
【为什么是子串】这是终端字节流，不是一行字符串；断言的串是一整个表头去掉空格后的全部内容，前后都有别的帧。
【判别力】删掉 F 那一行、或装一个 send 是空函数的适配器 → 第 3 条红，而第 1、2 条（前提）照样绿；E5 从不装 tick → 第 4 条红。F 的 `r != nil` 门被删由既有 `TestTui1dRunFinalIsRealStdout` 抓（panic）。

---

## 3. 冻结的字面量

### 3.1 表头

`chrome()` 第 1 行，一个 Go 字符串，不含换行，由 `fmt.Sprintf("council %s   %s", m.version, 状态格)` 生成（`council`、版本、三个空格、状态格）。状态格：

```
不忙：  status
忙：    status + [ "  " + 段 ]（有段时） + "  " + formatElapsed(已用时长)
段：    label
        + " " + k + "/" + n                 （仅当 n > 1；k、n 用 %d）
        + " (" + 末6(roundID) + ")"          （仅当 roundID 非空）
        + "  回来 " + 已回来 + "/" + members （%d/%d）
末6(s)：len(s) > 6 时取 s[len(s)-6:]，否则整串（roundID 是 ASCII）
```

分隔全是两个 ASCII 空格（`"  "`）；括号是 ASCII `(` `)`；`回来` 两个汉字后一个 ASCII 空格。

原型实跑得出的整串（Version 为 `t` 或 `test`），逐字节：

```
council t   空闲
council t   正在运行  0s
council t   正在运行  发言轮 1/2  回来 0/2  0s
council t   正在运行  发言轮 1/2 (688558)  回来 1/2  0s
council t   正在运行  裁判轮  回来 0/1  0s
council t   正在运行  裁判轮 (aa08ae)  回来 1/1  0s
council t   正在取消  发言轮 1/2 (fc9f8f)  回来 2/2  0s
council test   正在运行  发言轮  回来 0/2  1s
```

### 3.2 `formatElapsed`

`s := int64(d / time.Second)`（向零截断；负数夹成 0）；`s < 60` → `fmt.Sprintf("%ds", s)`；`s < 3600` → `fmt.Sprintf("%dm%02ds", s/60, s%60)`；否则 `fmt.Sprintf("%dh%02dm", s/3600, (s%3600)/60)`（丢掉秒）。

| 输入 | 输出 |
|---|---|
| `-1 * time.Second` | `0s` |
| `0` | `0s` |
| `999 * time.Millisecond` | `0s` |
| `1 * time.Second` | `1s` |
| `59*time.Second + 999*time.Millisecond` | `59s` |
| `60 * time.Second` | `1m00s` |
| `65 * time.Second` | `1m05s` |
| `3599 * time.Second` | `59m59s` |
| `3600 * time.Second` | `1h00m` |
| `3659 * time.Second` | `1h00m` |
| `3720 * time.Second` | `1h02m` |
| `36000 * time.Second` | `10h00m` |
| `100*time.Hour + 59*time.Minute + 59*time.Second` | `100h59m` |

### 3.3 T2 的步骤表（Model 单测，Version `t`，时钟恒为 `rbaT0`）

「消费」= 喂一条 `tea.WindowSizeMsg{Width: 100, Height: 24}`、丢掉 Cmd。`RID` = `"467afaff-4b9d-4694-8a4e-66f322688558"`，`RID2` = `"d4515aea-24a4-4b12-b95e-cdd3a3aa08ae"`。

| 步 | 喂的消息 | 之后 `chrome()[0]` 整串 |
|---|---|---|
| H1 | （刚造好、已喂尺寸） | `council t   空闲` |
| H2 | 提交（代次 1），消费 | `council t   正在运行  0s` |
| H3 | `elapsedTickMsg{gen: 1, at: rbaT0.Add(12*time.Second)}` | `council t   正在运行  12s` |
| H4 | `roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}}` | `council t   正在运行  发言轮 1/2  回来 0/3  12s` |
| H5 | `memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rba_a", RoundID: RID}}` | `council t   正在运行  发言轮 1/2 (688558)  回来 1/3  12s` |
| H6 | `memberDoneMsg{ev: repl.MemberDoneEvent{Label: "发言轮", AgentID: "rba_b", RoundID: RID2}}` | `council t   正在运行  发言轮 1/2 (688558)  回来 2/3  12s` |
| H7 | `roundEndMsg{ev: repl.RoundEndEvent{Label: "发言轮", K: 1, N: 2}}` | `council t   正在运行  发言轮 1/2 (688558)  回来 2/3  12s` |
| H8 | `roundStartMsg{ev: repl.RoundStartEvent{Label: "裁判轮", K: 0, N: 0, Members: 1}}` | `council t   正在运行  裁判轮  回来 0/1  12s` |
| H9 | `roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 2}}` | `council t   正在运行  发言轮  回来 0/2  12s` |
| H10a | `memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "rba_a", RoundID: ""}}` | `council t   正在运行  发言轮  回来 1/2  12s` |
| H10b | `memberDoneMsg{ev: repl.MemberDoneEvent{AgentID: "rba_b", RoundID: "abc"}}` | `council t   正在运行  发言轮 (abc)  回来 2/2  12s` |
| H11a | `roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}}`，再 `memberDoneMsg{ev: repl.MemberDoneEvent{RoundID: RID}}` | `council t   正在运行  发言轮 1/2 (688558)  回来 1/3  12s` |
| H11b | `tui1bCtrl('c')`（busy，取消） | `council t   正在取消  发言轮 1/2 (688558)  回来 1/3  12s` |
| H11c | `elapsedTickMsg{gen: 1, at: rbaT0.Add(65*time.Second)}` | `council t   正在取消  发言轮 1/2 (688558)  回来 1/3  1m05s` |
| H12a | `tui1bType` 一个字符后 `tui1bEnter()`（busy 时 Enter） | `council t   一轮正在跑，先等它回来或按 Ctrl-C 取消  发言轮 1/2 (688558)  回来 1/3  1m05s` |
| H12b | `elapsedTickMsg{gen: 1, at: rbaT0.Add(3720*time.Second)}` | `council t   一轮正在跑，先等它回来或按 Ctrl-C 取消  发言轮 1/2 (688558)  回来 1/3  1h02m` |
| H13a | `submitDoneMsg{}` | `council t   空闲` |
| H13b | `roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3}}` 与 `memberDoneMsg{ev: repl.MemberDoneEvent{RoundID: RID}}`（不忙） | `council t   空闲` |
| H14a | 提交（代次 2），消费；再 `submitDoneMsg{err: errors.New("坏了")}`（`tui1bSubmitter` 的 IsQuit 为 false） | `council t   发生错误，正在退出` |
| H14b | `elapsedTickMsg{gen: 2, at: rbaT0.Add(99*time.Second)}` | `council t   发生错误，正在退出` |

H15 另起一个 Model：提交（代次 1）、消费、喂 H4 与 H5 的两条消息、`submitDoneMsg{}`、再提交（代次 2）、消费 → `council t   正在运行  0s`（没有上一次的段）。

### 3.4 R1 / R2 的 golden

规范化规则（对整个缓冲，按顺序）：
1. 正则 `wall_ms=\d+` 全部替换成 `wall_ms=W`（墙钟随机）。
2. 多行模式正则 `^(rba_[a-z]  未标定  未标定  预报\S+  不在飞  )-?\d+(  无样本)$` 全部替换成 `${1}H${2}`（命令行余量随测试二进制路径长度变，原型在两个包里分别实测到 32480 与 32468）。

规范化后必须整串等于下面这段（1806 字节，39 行，每行以 `\n` 结尾，无 `\r`，原型在 HEAD 代码上连跑两次逐字节一致；中间的 `2a90d449-...` 来自假 CLI 的固定 JSON）：

```
发言顺序按字典序，重复的 id 已去重，只问一次。
成员  输入token  金额  冷启动  在飞  命令行余量  依据
rba_a  未标定  未标定  预报冷  不在飞  H  无样本
合计输入token 0（只含已标定的行）
另有 1 名未标定
扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。
本轮问 1 个: rba_a
outcome spoke  agent rba_a
新开了一条 provider 会话 2a90d449-a348-497a-8812-e73e74d6898e
seq 4  rba_a  claude/claude-fable-5-1/high
OK
用量 in=47751 cache_create=47749 cache_read=0 out=4 thinking=0
金额 0.9551999999999999 USD
冷暖 预报=冷 实测=冷  wall_ms=W  prompt_bytes=19  scanned_to=1
发言轮结算
确知起进程 1
分不清进程是否起来 0
用量 输入 47751  输出 4  cache_read 0  cache_create 47749  thinking 0
金额 0.9551999999999999 USD
冷启动 预报 1  实测 1
成员  输入token  金额  冷启动  在飞  命令行余量  依据
rba_j  未标定  未标定  预报冷  不在飞  H  无样本
合计输入token 0（只含已标定的行）
另有 1 名未标定
扇出期间 REPL 不接受任何命令，约 1 x 5 分钟（并发，最慢的那个成员说了算）。命令派发是串行的。
本轮问 1 个: rba_j
outcome spoke  agent rba_j
新开了一条 provider 会话 2a90d449-a348-497a-8812-e73e74d6898e
seq 8  rba_j  claude/claude-fable-5-1/high
OK
用量 in=47751 cache_create=47749 cache_read=0 out=4 thinking=0
金额 0.9551999999999999 USD
冷暖 预报=冷 实测=冷  wall_ms=W  prompt_bytes=30  scanned_to=5
裁判轮结算
确知起进程 1
分不清进程是否起来 0
用量 输入 47751  输出 4  cache_read 0  cache_create 47749  thinking 0
金额 0.9551999999999999 USD
冷启动 预报 1  实测 1
```

测试腿把它写成 Go 的反引号原始字符串（内容里没有反引号），首行紧跟在开头反引号之后、末尾带一个换行再接结束反引号。

### 3.5 其余逐字节字面量（生产代码只新增表头里的这几个；其余是测试要比对的既有文本）

| 用途 | 字面量 | 出处 |
|---|---|---|
| 段标签 | `发言轮`、`裁判轮` | `round.go:202`、`:235`（既有，原样透传） |
| 表头「回来」格 | `"  回来 %d/%d"` | 本刀新增，`fmt.Sprintf` |
| 表头 k/N | `" %d/%d"` | 本刀新增 |
| 表头 roundID | `" (" + 末6 + ")"` | 本刀新增 |
| 窗口尺寸注入 | `"\x1b[8;24;100t"`（测试里用 `tui1dESC + "[8;24;100t"`，源码不写 ESC 字面量） | W1 |
| 取消后的收尾行 | `已跑 1/2 轮，其余取消` | `round.go:192`（既有） |
| MaybeAdvanced 说明行 | `provider 那边可能已经吃下这批内容` | `ask.go:317`（既有） |
| Drifted 说明行 | `送出去的会话 id 与回来的不一样：CLI 悄悄开了新会话，前缀已经断了，钱已经花了` | `ask.go:331`（既有） |
| PostRejected 说明行 | `正文写不进库（已经付过钱），整段如下:` | `ask.go:358`（既有，冒号是 ASCII） |
| 成员块首行 | `"outcome " + 结局 + "  agent " + id`，结局取 `spoke` / `canceled` / `post_rejected` | `ask.go:313`、`:511-532`（既有） |

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；主代理落地后逐条跑，从副本还原，不用 `git checkout`。

| # | 变异（可编译） | 预期被谁杀 |
|---|---|---|
| 1 | round.go：RS 挪到「本轮问」那次 printf 之前 | R3 第 3 条；O1 第 2 条 |
| 2 | round.go：RS 挪到 `if !goRun` 之前（紧跟 renderBudget） | R5 |
| 3 | round.go：`Members: len(rows)` | R4 |
| 4 | round.go：`:202` 传 `rounds, k`（k、N 对调） | R3 第 2 条；O1 第 1 条 |
| 5 | round.go：`:235` 传 `1, 1` | R2；R3 第 2 条；O1 第 1 条 |
| 6 | round.go：MD 挪到 `writeAskRecord` 之前 | R3 第 4 条；O1 第 3 条 |
| 7 | round.go：MD 挪到 writeAskRecord 之后、`"/round %s: %s"` 那一行之前 | O2 第 3 条 |
| 8 | round.go：MD 只在 `rec.Err == nil` 时发 | O2 第 1 条；O3b |
| 9 | round.go：`MaybeAdvanced: false` | O2 第 2 条 |
| 10 | round.go：`Drifted: false` | O3a |
| 11 | round.go：`Dropped: false` | O3b |
| 12 | round.go：`WallMS: 0` | R3 第 4 条；O1 第 3 条 |
| 13 | round.go：`RoundID: ""` | R3 第 5 条；O1 第 4、6 条 |
| 14 | round.go：`Label: ""`（MD） | R3 第 5 条；O1 第 4 条 |
| 15 | round.go：RE 挪到结算 printf 之前 | R3 第 6 条；O1 第 5 条 |
| 16 | round.go：RE 放进 `defer`（每条 return 都发） | R5 |
| 17 | round.go：去掉三处 `obs != nil` 判断 | R1（nil 接口调用 panic） |
| 18 | round.go：RS 发射处多一句 `r.printf("开轮\n")` | R2 |
| 19 | run.go：删掉 `r.SetRoundObserver(...)` 那一行 | W1 第 3 条 |
| 20 | run.go：去掉 `if r != nil` | `TestTui1dRunFinalIsRealStdout`（既有，panic） |
| 21 | run.go：装 `newRoundObserver(func(tea.Msg) {})` | W1 第 3 条 |
| 22 | roundobs.go：`RoundEnd` 发 `roundStartMsg{}` | T6；O1 第 1 条 |
| 23 | roundobs.go：`MemberDone` 发 `memberDoneMsg{}`（丢掉 ev） | T6；O1 第 4 条 |
| 24 | `formatElapsed`：`s < 60` 改 `s <= 60` | T1（60s） |
| 25 | `formatElapsed`：`%02d` 改 `%d` | T1（65s、3720s） |
| 26 | `formatElapsed`：去掉负数夹紧 | T1（-1s） |
| 27 | tick 分支：去掉 `!busy` 判断（不忙也续订） | T3 最后一句；O4 第 2 条 |
| 28 | tick 分支：去掉 `msg.gen != tickGen` 判断 | T3 第一句 |
| 29 | tick 分支：`m.status = "正在运行"`（覆盖状态） | T2 H11c、H12b |
| 30 | tick 分支：已用时长 `+= time.Second`（按拍累加） | T2 H3 |
| 31 | E5：在开始提交的那次 Enter 就把 tick 并进 Cmd（最自然的写法 `tea.Batch(tea.Sequence(cmds...), tick)`，并保留「待装」） | `TestTui1bAcc6SubmitEchoesEachLineBeforeSubmitting`（既有）；O4。T4 不一定杀：保留待装时下一条 Update 仍会再装一次，Cmd1 恰好 1 拍、Cmd2 为 nil，照样绿（审查员实测） |
| 32 | E5：永不装 tick | T4；O4 第 1 条；W1 第 4 条 |
| 33 | E5：装完不清「待装」（每条 Update 都装） | T4 |
| 34 | beginSubmit：不做 `tickGen++` | T2 H3（代次 1 的拍被丢） |
| 35 | beginSubmit 与 submitDone 都不清 Run 状态 | T2 H15 |
| 36 | `roundStartMsg`：不看 busy | **等价，不列杀手**（见表后等价清单） |
| 37 | `memberDoneMsg`：每次都覆盖 roundID | T2 H6 |
| 38 | 末 6 位写成前 6 位 | T2 H5；O1 第 6 条 |
| 39 | 段的 `n > 1` 改 `n >= 1` | T2 H9；W1 第 3 条；O4 第 1 条 |
| 40 | 状态格在不忙时也拼已用时间 | T2 H1、H13a |
| 41 | 段信息拼成第二行（`"\n"` 分隔） | T5 的 View 行数那一句（frameHeight 那一句在这种写法下仍是 8，属回归钉）。X-39e A9 杀不到：它在空闲态跑，不拼段（集成员实测） |
| 42 | 新消息分支返回 `tea.Println(...)` | T7 |
| 43 | E5：去掉「处理后仍 busy」这一条件（提交后第一条 Update 是 submitDone 时也并进 tick） | `TestTui1bAcc11SubmitDoneWithQuitSentinelQuits`（既有：`tui1bRun` 倒序跑出 `[tick, quit]`，不等于 `["quit"]`）；`TestTui1bAcc12SubmitDoneWithoutErrorGoesIdle`（既有：返回非 nil Cmd） |
| 44 | beginSubmit：不把已用时长清零 | T8 第 2 步 |
| 45 | beginSubmit：只在起点为零值时才取时钟（`if 起点.IsZero() { 起点 = 时钟() }`） | T8 第 3 步 |
| 46 | onDone：writeAskRecord 失败那一支 return 之前照发 MD | R6a |
| 47 | onDone：`"/round %s: %s"` 那一行写失败后不 return（回到 HEAD 的写法），落到末尾的 MD | R6c |
| 48 | round.go：结算写失败的早退上照发 RE | R6b |
| 49 | E6：续订那一拍的代次写成 `tickGen + 1` | T3（主）；O4 第 1 条（辅） |
| 50 | E6：续订间隔写成 `time.Minute` | T3（主，1.5 秒超时）；O4 第 1 条（辅） |
| 51 | E5：装 tick 时丢掉原 Cmd，只返回 tick（`return out, elapsedTickCmd(out.tickGen)`） | T4b（v1.3 补；v1.2 下全绿，由落地后的独立审查实测发现） |

**判定为等价、不列杀手的变异**（写在这里，免得被当成漏测）：
- 只删 beginSubmit 的清空、或只删 submitDone 的清空其中一处：另一处照样清，任何可观测行为都不变。
- 适配器方法里加一把私有互斥锁：没有第二个持锁者，行为不变，只在审查时看。
- C2 改成三个发射点各自读一次字段：`SetRoundObserver` 只许在没有 Submit 在跑时调用，读一次与读三次在可观测行为上等价。
- **#36（`roundStartMsg` 不看 busy）以及 `memberDoneMsg` 去掉 `!busy`、两处同时去掉**：§3.1 规定不忙时状态格只有 status、不拼段，E4 在下一次 beginSubmit 时整体清空 Run 状态；空闲时写进去的段既不显示、也活不到下一次提交。T2 H13b 在忠实实现上是空断言，只作回归钉（审查员实测三种写法全绿）。
- E6 续订用 `msg.gen` 而不是 `tickGen`：走到续订这一步时两者必然相等。
- （v1.3 补，独立审查实测都活）E5 去掉「处理后 `tickGen` 未变」或去掉「处理前 busy」其中一道门：两道门互相兜底，单删各自等价（`tickGen` 只在 beginSubmit 变，而 beginSubmit 只在不忙时发生）。
- `roundEndMsg` 分支整个删掉、让它落进 textarea：textarea 忽略这条消息，行为不变。
- `memberDoneMsg` 去掉「当前没有段」这一门：不忙时被 busy 门挡住；忙时没有段就不显示段，回来数随下一次 RoundStart 整体清零。

**只在审查时看（没有用例能抓，集成员收割时逐字核对）**：
- run.go 里 F 那一行必须逐字是 `r.SetRoundObserver(newRoundObserver(p.Send))`。写成 `newRoundObserver(func(msg tea.Msg) { go p.Send(msg) })`、或者经带缓冲的 channel 转发，会破坏 §1.1 G.5 的顺序保证（MD 可能跑到自己的明细前面，甚至跑到 DONE 之后、改坏下一次提交的表头），但全部验收都绿（审查员实测 M103）：W1 只看表头子串，O 系列用自己的装置构造观察者，不经过 Run 那一行。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许改这 6 个文件：`internal/repl/observer.go`（新）、`internal/repl/repl.go`、`internal/repl/round.go`、`internal/tui/roundobs.go`（新，文件名可自定，不许以 `rba` 开头）、`internal/tui/model.go`、`internal/tui/run.go`。不写任何 `_test.go`，不改 `go.mod`。
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
  锚点必须从 HEAD `3143bc8` 的真文件逐字复制（含 tab 与中文标点），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。
- 新文件：`===FILE: internal/repl/observer.go===` 换行后接完整内容，末尾 `===END FILE===`。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- 代码必须能 `gofmt`（tab 缩进）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的字符串字面量只有 §3.1、§3.5 列出的那些。
- 生产代码的新标识符不许以 `rba` 开头（那是测试腿的前缀）。
- §1.1 里逐字给出的声明（A、D、F 的代码块，C1 的签名与调用点，C3/C4/C5 的结构体字面量）照抄；「名字实现方定」处自取。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：6 个文件是否都给了；§1.1 A-G 每一条落在哪个补丁块。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3.1、§3.2、§3.5 的字面量是否逐字照抄；有没有新增任何别的字符串字面量。
  ```

---

## 6. 落地后主代理要做的

1. 收割：越界扫描（主仓库与中转根有没有 grok 写出来的新文件）；符号禁令扫描；按锚点落盘；`gofmt -l`；`go vet ./internal/repl ./internal/tui`；`go build ./...`。**逐字核对 run.go 装配那一行是 `r.SetRoundObserver(newRoundObserver(p.Send))`**（§4「只在审查时看」）。
2. 合并测试腿的 6 个文件后，只跑 `./internal/repl` 与 `./internal/tui` 两个包（另一条流水线也在跑 go test；3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制）。既有用例必须全绿，`TestTui1bAcc8BusyCtrlCCancelsWithoutQuitting` 变慢到约 1 秒是预期。
3. 核对测试腿报告里 R1 在 HEAD 上单独跑绿的证据。
4. §4 的 50 条变异逐条跑（#36 是等价的，跑了也不会被杀；从副本还原）；任何一条没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
5. 文档：
   - `docs/00_DECISIONS.md` #41 的标题状态改成「第 1、2 刀已实现（RB-A，`<commit>`），第 3、4 刀未做」，在本条下补一段 RB-A 的落地记录（tick 装在「提交后下一条 Update」的理由；Run 里装观察者而不在 runWith 的理由；nil 门的来历）。
   - `docs/15_TUI_SPEC_MAPPING.md` §3.1 补一句：表头总览形态已实现，是 RB-B 在高度超限时的降级形态。
   - **TUI-1b 契约 5.2 被本刀收窄，要写明**：5.2 说 `WindowSizeMsg`（以及 busy 时的 Ctrl-C / Enter 等）返回 nil Cmd；E5 之后，**提交后的第一条 Update** 无论是什么消息都会带回一个 tick Cmd（T4 正是用 WindowSizeMsg 钉的）。既有 `TestTui1bAcc1WindowSizeFallsBackTo80x24`（`tui1b_model_test.go:407`）仍绿，只因为它在空闲态跑。#41 的落地记录里写成「5.2 的 nil Cmd 只对空闲态、以及提交后第一条 Update 之后成立」，并在 TUI-1b 契约原文旁注一句，不要让下一个人从 T4 反推。
   - 以下既有测试注释会因本刀过期，但属于既有 `_test.go`、本刀不许改，记进 #41 的剩余项：`session_inflight_test.go` 与 `smoke_test.go` 里「四处 / 三处要一起改」的假 JSON 拷贝计数（现在多了 tui 包这一份）；`tui1d_run_ctx_test.go:77`、`:167`「管道里看不到表头文字」（写入 `ESC[8;h;wt` 之后就看得到，见 W1）。
   - #41 剩余项另记：PasteMsg 在 busy 时仍把 status 改成「粘贴了 N 个字符」，盖掉「正在运行 / 正在取消」（既有行为，本刀不改）；既有 `tui1bIntFields` 会把新加的 `time.Duration` 字段也收进「按值配对」的候选（Acc1 仍绿，免得以后有人被绊）；表头 roundID 末 6 位不是 `/cost --round` 能直接接受的参数；RoundStartEvent 暂不带成员名单（RB-B 要追加时写清是新分配的切片）；被杀的路上 Submit 可能在 Run 返回后继续调观察者（`p.Send` 此时空操作、无害，RB-B 若要在收轮时 Println 要重新论证）。
   - **E7「submitDone 的 Cmd 一个字不变」只对 RB-A 冻结。** O2 探针（2026-09-24）判「只长不缩」按构造安全，但构造要求无错的 submitDone 那一次 Update 钉住光标、返回 `tea.Sequence(Println(一行非空的收尾行), finalize)`，并把 submitDone 到 finalize 之间算作 busy；RB-B 必须解冻 E7。别把 E7 当成永久约束。
6. 真机复验（Griffin，Windows Terminal + PowerShell 5.1，另在 conhost 上各跑一遍）：
   1. `council` 进 TUI，`/ask <一个成员>`：表头从「正在运行  0s」起每秒加 1；回来之后变回「空闲」。
   2. `/round a b --go`：表头出现「发言轮  回来 0/2  Ns」（N=1 不印 1/1）；第一个成员回来时出现 `(xxxxxx)` 与「回来 1/2」；括号里的 6 位与 `/cost` 里这一轮 round_id 的末 6 位一致。
   3. `/round a b j --go --rounds 2 --with-judge`：依次看到「发言轮 1/2」「发言轮 2/2」「裁判轮」三段；滚回历史，每个成员的明细与结算照旧完整。
   4. 一轮进行中按 Enter：表头变成「一轮正在跑，先等它回来或按 Ctrl-C 取消  ...  Ns」，秒数继续走；按 Ctrl-C：变成「正在取消  ...」，秒数继续走，随后出现「已跑 k/N 轮，其余取消」并回到「空闲」。
   5. 把窗口拉窄到 40 列左右再跑一次 /round：表头被裁掉右侧、不折行、活区不变高、退出（空闲 Ctrl-C）后提示符上方不留残影（X-39e 复验项）。
   6. 非终端的行式路径（`council < script.txt` 或管道）跑一次 /round：输出与改动前逐行一致，没有任何新行。
7. 清理本刀在中转根与 scratchpad 里的全部中间物。
