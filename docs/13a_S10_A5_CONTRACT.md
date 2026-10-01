# S10A5 冻结契约：别人的在飞记录先拒（§3.8）+ /session 可疑基线列（§3.11）（v1.1，2026-09-27）

> **归档说明（2026-09-27）**：这是 Step 10 A 之五（`13` §3.8 闸 + §3.11 `/session` 列）派工时的冻结契约原文 v1.1，从中转根 `E:\multi_model_cowork\s10a5_contract.md` 原样归档。文中行号指 HEAD `92dfc89`（生产代码与 `750c2d4` 相同）；落地提交 `8c00ec9`。文中的 `E:\` 路径是当时的中转与隔离目录，已清理。
> **落地后的订正（正文未改，以这里为准）**：
> - §1.1 E1 注释要点 1 里「persistTurn 与成功路径都用 Background 落库」不实：只有 `failClosed` 与成功路径用 Background，`BeginTurn` 之后走 `refuse` 的几支用调用方 ctx；「闸空闲时还留着的行」还要算上「`BeginTurn` 之后、起进程之前的一次取消」。落地的注释已按此订正，剩余项记在 `00_DECISIONS.md` #43(b)。
> - §4 的 #6、#7 照原文写会让 `sqlite3` import 变成未使用、编译不过；集成员实际用的是 `-1*sqlite3.SQLITE_CONSTRAINT_UNIQUE` 与 `|| true`，语义相同。
> - 独立审查实测的两处测试缺口（读失败那一支 refuse 传 true 时 `turn_usage` 会丢；`/session` 缺「在飞且可疑」那一格）与两条「只在审查时看」（检查在闸内、用调用方 ctx）已在落地后另补用例，见其后的测试提交。
> - §6 第 7 步 (ii)「对账失败留行时 `/ask` 被拒」已由本刀的 E1 覆盖，C2 只补了 (i) 的 `/session` 列。

> **v1.1 相对 v1 的改动**（派工前对抗审 W1 的 1 条 major、4 条 minor，major 经一个证伪者确认并补正了修法；Griffin 2026-09-27 裁了三条）。生产代码的签名、判据、位置、SQL、哨兵一个字没变；变的只有 §3.1 那句话的措辞与 E1 的两条注释要点（第 1 条补前提、第 3 条收窄）。
>
> 1. **§3.1 拒绝那句话改措辞**（Griffin 裁）：末尾「对账失败的原因在启动输出的「启动在飞对账补账失败」那一行」改成「对账失败的原因在启动输出里以「启动在飞对账」开头的那一行」。理由（W1 minor）：`AuditInflight` 读失败时 `app.go:190` 打的是 `启动在飞对账失败: `，不是 `:197` 的 `启动在飞对账补账失败: `，原句会指向一条不存在的行；两条共有的前缀是「启动在飞对账」。§2 各测试文件的常量逐字复制 §3.1，随之一起变；§3.5 补 `:190` 那一条。
> 2. **两条维持 v1**（Griffin 裁，照写）：`/session` 新列表头 `BASELINE`、格值 `suspect` / `ok` / `-`（§3.3 不动）；定点读走 `Store` 接口加 `InflightFor`、六个假件各补一个方法（§1.1 A、C、G 不动），不改成 Service 层过滤。
> 3. **外来行按 phase 参数化（W1 major）**：v1 里所有打进 E1 检查的外来行夹具都是 `PhaseSpawned`，于是「闸按 phase 过滤」（例如 `&& prior.Phase != conversation.PhaseBegun`，或放过 `PhaseReturned`）的写法活过全部验收。§2.0 夹具的 `Phase` 改成参数；G1 改成三个场景 × `{PhaseBegun, PhaseSpawned, PhaseReturned}` 共 9 个子测试；§4 新增 #19，杀手点名 G1 `bootstrap_begun` / `bootstrap_returned` 的断言 3（History 多出 `session_opening`）与断言 6（`identity_fp` 非空）。
> 4. **订正 v1 对 begun 行后果的夸大**：§0 第 2 条、§1.1 E1 注释要点 3、§6 第 5 步里「指纹不符时删掉了 settle 要标的那一行」只对 spawned / returned 行成立。begun 行的对账（`recover.go:143-148`）只删在飞行、不读也不标 `provider_sessions`；begun 行在「不拦」下的真实后果是一条假的 `session_opening`、被拒那一轮 `identity_fp` 非空、多一次 `MkdirAll`。`provider_sessions` 那一行在 begun 下照样会被 `DropProviderSession` 删掉（原型实测 `fpmismatch_begun` 断言 4 红），只是对账本来也不会去标它 —— 夸大的是「要标」，不是「删了」。
> 5. **检查位置漂移（W1 minor 1）**：挪到 `MkdirAll` 之后、或挪到 `cfg.Agents` 查找之后，v1 的验收全活。G1 的 `bootstrap_*` 加断言 7「拒绝之后 `r.workDir` 不存在」；新增 G1e（配置里删掉该成员、库里留着外来行，错误是 `ErrInflightConflict` 不是 `ErrAgentUnknown`）；§4 新增 #20、#21；§2 通用纪律的 turn 引用清单补 `Runner.workDir`。
> 6. **store 把扫描错误当「没有」（W1 minor 2）**：S1 加一例，经既有的 `tstRawDB` 直写一行 `started_at = 'not-a-time'`，断言 `InflightFor` 返回 `err != nil` 且 `ok == false`；§4 新增 #22；§2 通用纪律的 store 引用清单补 `tstRawDB` 与 `(*Store).Path`。
> 7. **E1 注释要点 1 的隐含前提（W1 minor 3）**：补一句「生产里每个进程（每个 data_dir）只有一个 Runner（`app.go:169`），闸在 Runner 上」。
> 8. **`/session` 在 InFlight 时一律印 `suspect` 的变异活着（W1 minor 4）**：R4 换成 `newInFlightRunner`，加一个在飞、基线不可疑的成员 `ag_wait`，断言它那一格印 `ok`；§2 通用纪律的 repl 引用清单补 `inFlightProbeDelay`；§4 新增 #23。
> 9. **§4 #1、#2 的杀手表述跟着 G1 改名**（原型重推）；**§6 加一步**（新第 7 步，清理顺延成第 8 步）：A 之五落地后按 13 §6.2 订正末尾写的两处加强 C2 —— 留给主代理，不进本刀验收；第 4 步的变异范围 `#1-#18` 改 `#1-#23`。
>
> **HEAD**：主仓库现在是 `750c2d4`，相对 v1 引用的 `92dfc89` 只多了 `internal/app/s10c2_crash_e2e_test.go`（C2，前缀 `s10c`）与文档；全部生产 `.go` 逐字节相同，本文里的生产代码行号与锚点仍以 `92dfc89` 为准，在 `750c2d4` 上同样有效。本刀 app 包的新文件与 `s10c2` 共用 `smoke_test.go` 的 `TestMain`，前缀 `s10a` 与 `s10c` 不相交。
>
> **v1.1 的期望值**（新句子、G1 九个子测试与断言 7、G1e、S1 坏行、R4 的 `ag_wait` 行、#1 / #2 的新表述、#19-#23 的杀法）已在隔离拷贝 `E:\Isolating_test\council\s10a5_v11\` 里用一份新原型（§1.1 全部生产改动 + §2 全部用例，含 v1.1 新增）实跑得出：原型下全量 `go test -count=1 ./...` 八包全绿；§4 #1-#23 逐条在原型上跑过（锚点恰好一次、从字节副本还原）；§2.3 / §2.4 两个文件在 HEAD `750c2d4` 的拷贝上单独跑，红绿与 §2 通用纪律末尾写的一致。原型已删除。

- 仓库：`D:\DeskTop\My_Project\council`，分支 `main`，HEAD `92dfc89`。Go 用全路径 `"C:\Program Files\Go\bin\go.exe"`，`GOMODCACHE=E:\Go\modcache`，`GOPROXY=off`，`modernc.org/sqlite v1.59.0`（`go.mod`）。本机没有 gcc，不能 `-race`。
- 来源：
  - `docs/13_STEP10_CONTRACT.md` §3.8（661-689 行）：settle 失败时受影响的 `(conv, agent)` 下一轮直接 refuse；检查位置「拿到该 key 的 gate 之后、`BeginTurn` 之前」；「实现上不需要新状态」；撞 `UNIQUE(conv_id, agent_id)` 时「与 3.8 这条拒绝是同一个结局，用户看到的是同一句话」。
  - 同文件 §3.11（770-785 行）里 `SessionInfo` 加 `BaselineSuspect bool`、`/session` 多一列那两句；§4 表第 6 行与 §4a 第 6 条（`SessionInfo.BaselineSuspect` 登记进 `09a`）；§3.15 第 10 条（settle 失败则该成员 `/ask` 被拒）。
  - §3.11a（787-823 行）、§3.11b（825-855 行）之后的订正**只改启动横幅首行与对账/漂移的先后**，现查不涉及 `/session` 列，也不涉及 §3.8 的闸。`docs/00_DECISIONS.md` 里现查没有针对 §3.8 / §3.11 的订正（`grep "3\.8|3\.11|BaselineSuspect"` 零命中）；`#40(a)`（`/session` 的 NEXT 列不看漂移分类）是同族问题，本刀不修，见 open items。
  - `docs/12_STEP9_CONTRACT.md` 片 1 的按 sessionKey 的闸，今天在 `internal/turn/runner.go:55-73`（`sessionGate`）、`:141-156`（`gate()`）、`:337-340`（`askUpToWith` 取闸）。
  - `.handoff/HANDOFF.md:1727-1732`：「要加一个按 `(conv, agent)` 的定点读，`AllInflight` 全表扫不适合热路径」；「之四落地而之五没落时……用户看到的是裸 SQLite 错误而不是 §3.8 的话」。
  - `docs/README.md` 路线图 Step 10 那一行：A 之一到之四已落，**之五未落**。
- **不需要 schema 迁移**：v5 已有 `CREATE UNIQUE INDEX idx_inflight_turns_conv_agent ON inflight_turns (conv_id, agent_id)`（`internal/store/schema.go:404`），`internal/store/inflight_test.go:312` 的 `TestBeginTurnRejectsSecondRowForSameConvAgent` 已钉住它。本刀只加一条走这条唯一索引的定点读。
- 与本批另一刀的关系：C2（§6.2 崩溃对账端到端）在 `internal/app` 写测试、走的是 **settle 成功** 那条路；本刀的生产改动不碰 `internal/app`，本刀的闸只在 settle **失败** 后才生效，两边的期望值互不改变。本刀在 `internal/app` 只新增一个 `s10a` 前缀的测试文件，与 C2 的标识符不相交。D 之二（`/log` 标记）改 `internal/repl/commands.go` 的 `cmdLog`，本刀改 `internal/repl/ask.go`，不相交。
- 本契约的全部期望值（屏幕逐字节、`Record`、库里的行、HEAD 上撞 UNIQUE 的原文）已在隔离拷贝 `E:\Isolating_test\council\s10a5_draft\` 里用一份原型实现实跑得出；原型下全量 `go test -count=1 ./...` 八包全绿（既有用例零红，只有 §1.1 G 那六个假件补了一个方法）；§4 里标「原型实测」的变异都在原型上跑过、全部被点名的杀手杀掉。原型不交付，已删除。

---

## 0. 病根 / 动机（行号以 HEAD `92dfc89` 为准）

1. **今天撞 UNIQUE 时实际走哪条路、打什么（原型在 HEAD 上实测）。** 库里有一条 `(conv, agent)` 的在飞记录没被清掉（上次崩溃、settle 失败），下一次 `/ask` 一路走到 `runner.go:605-623` 的 `BeginTurn`，插入撞 `idx_inflight_turns_conv_agent`，`:622` 以 `refuse(..., fmt.Errorf("begin turn: %w", err))` 返回。结局是 `OutcomeRefused`、假 Spawner 零次调用、`turn_usage` 多一行 outcome=7。`Record.Err.Error()` 原文：

   ```
   begin turn: begin turn <turnID>: begin turn <turnID>: constraint failed: UNIQUE constraint failed: inflight_turns.conv_id, inflight_turns.agent_id (2067)
   ```

   屏幕上 `/ask` 打 `outcome refused  agent <id>`，接着 `askCause`（`internal/repl/ask.go:539-554`）认不出任何哨兵、落进 `:551-552` 的 `default` 原样打这一串。**结局对，那句话是裸 SQLite 错误。**

2. **撞之前已经做了不该做的事。** 现在唯一的挡板在 `:605`，而它上游还有副作用：
   - bootstrap 轮在 `:547-559` 先追加一条 `session_opening` 面包屑（原型实测：拒绝之后 `messages` 里多了一条 `event=session_opening`，而进程一个都没起）；
   - 指纹不符时 `:445-464` 先 `postRebuildNotice` 再 `DropProviderSession` —— **把 settle 以后要整行读回、标 `baseline_suspect` 的那一行先删掉了**（v1.1 订正：「要标」只对 spawned / returned 行成立。begun 行的对账 `recover.go:143-148` 只删在飞行、不读也不标 `provider_sessions`，那一行照样会被删，但对账本来也不会去标它；begun 行在不拦时的真实后果是一条假的 `session_opening`（bootstrap 轮）、被拒那一轮 `identity_fp` 非空、多一次 `MkdirAll`；指纹不符时另有一条假的 `session_rebuilt` 并经 `DropProviderSession` 删掉 `provider_sessions` 那一行（原型 `fpmismatch_begun` 断言 3、4 红）。以上均为原型实测）；
   - `:393` 的 `MkdirAll`、`:402` 的指纹计算都已发生，`turn_usage` 那一行 refused 带着非空 `identity_fp`。
   只追加的事实源里留下一条「已开启」而那条会话根本没开，和 3.3a 要躲的是同一类假账。

3. **没有按 `(conv, agent)` 的定点读。** `conversation.Store`（`internal/conversation/store.go:17-80`）只有 `AllInflight`（`:47-50`），`internal/store/read.go:221-226` 的注释自己写着「每次读完整张表，不适合在热路径上反复调用」。

4. **`/session` 看不到基线可疑。** `writeAskMeter` 在 `ask.go:412-414` 打「金额未计（上一轮结局未知，基线可疑）」，但 `SessionInfo`（`runner.go:887-898`）没有这一格，`cmdSession`（`ask.go:188-239`）只有 `AGENT / PROVIDER_SESSION / LAST_TURN_AT / NEXT` 四列。13 §3.11 原话：否则用户看到一个「金额：不知道」时无从知道为什么。

5. **今天用户能用的解开路径只有一条：重启。** 启动时 `internal/app/app.go:188-200` 跑 `AuditInflight` -> `SettleLostTurns`，失败时往 Err 打 `启动在飞对账补账失败: %v`（`:197`）。`AuditInflight` / `SettleLostTurns` 结构性地只在启动时跑、不导出给 REPL（`internal/turn/recover.go:23-31`，13 §3.5 与 §1.4 的豁免依据）。**`/session rebuild` 解不开**：`Rebuild`（`runner.go:235-256`）只删 `provider_sessions` 那一行并把 `g.st` 置 nil，一个字不碰 `inflight_turns`；闸空闲时 `TryLock` 成功，它会「成功」，下一次 `/ask` 照样被拒。所以拒绝那句话**不许**指向 `/session rebuild`。

---

## 1. 范围

### 1.1 做

凡写「名字实现方定」处实现方可自取名字（生产代码新标识符不许以 `s10a` 开头）；其余名字、签名、字面量、顺序、判据全部冻结。注释用中文、可以扩写，但必须覆盖每条列出的「注释要点」。

**缩进说明**：本文代码块里的缩进可能被排版成空格。锚点一律以 HEAD 真文件为准（每条都给了行号），逐字复制真文件那几行（含 tab）。

#### A. `internal/conversation/store.go`

A1. `Store` 接口里，紧跟 `AllInflight`（`:47-50`）之后加一个方法（签名冻结）：

```go
	// （注释要点见下）
	InflightFor(ctx context.Context, convID, agentID string) (InflightTurn, bool, error)
```

注释要点：读回 `(convID, agentID)` 那一行在飞记录；第二个返回值 false 表示没有，不是错误；走 `(conv_id, agent_id)` 的唯一索引，是给每一轮起进程之前的那道检查用的定点读（不像 `AllInflight` 读全表）；一个 `(conv, agent)` 最多一行，由 v5 的唯一索引保证。

A2. 哨兵块里，紧跟 `ErrInflightMissing`（`:180-189`）之后加（名字与文本冻结）：

```go
	ErrInflightConflict = errors.New("an unsettled inflight turn already exists for this conversation and agent")
```

注释要点：同一 `(会话, 成员)` 已有一条**不属于本轮**的在飞记录；两个产出方 —— `Tx.BeginTurn` 撞 `(conv_id, agent_id)` 唯一索引、`turn` 包起进程之前的检查 —— 共用这一个哨兵，所以屏幕上是同一句话；`turn_id` 主键冲突**不是**这个哨兵。

A3. `Tx.BeginTurn` 的接口注释（`:133-137`）补一句：`(conv_id, agent_id)` 冲突时返回的错误链里带 `ErrInflightConflict`，同时保留底层驱动错误。签名不动。

#### B. `internal/conversation/service.go`

紧跟 `AllInflight` 直通（`:529-532`）之后加（签名冻结）：

```go
func (s *Service) InflightFor(ctx context.Context, convID, agentID string) (InflightTurn, bool, error) {
	return s.store.InflightFor(ctx, convID, agentID)
}
```

注释要点：只是直通，理由同 `AllInflight`。**不加** `s == nil` 判（13 §3.13a：`askUpToWith` 的 refuse 路径本来就无保护地解引用 `r.svc`，这里多一个判只会造出一个永远走不到的分支）。

#### C. `internal/store/read.go`

紧跟 `AllInflight`（`:221-250`）之后、`// UsageSamples 见 conversation.Store 上同名方法的注释。`（`:252`）之前加 `func (s *Store) InflightFor(ctx context.Context, convID, agentID string) (conversation.InflightTurn, bool, error)`。冻结要求：

- 走读句柄 `s.read.QueryRowContext`，不进写事务。
- SQL 逐字（空白可按 gofmt 之外的原样保留）：

  ```
  SELECT turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id,
         cursor_seq, scanned_to, started_at, phase, tier, work_dir
  FROM inflight_turns WHERE conv_id = ? AND agent_id = ?
  ```
  参数顺序 `convID, agentID`。
- 解析复用 `scanInflightTurn`（`:577`），不写第二份。
- `errors.Is(err, sql.ErrNoRows)` -> `(conversation.InflightTurn{}, false, nil)`；其它错误 -> `(conversation.InflightTurn{}, false, fmt.Errorf("get inflight turn %s/%s: %w", convID, agentID, err))`；成功 -> `(t, true, nil)`。与 `ProviderSession`（`:175-190`）同构。

#### D. `internal/store/tx.go`

- import 块（`:3-11`）加两行（`errors` 已经在）：`"modernc.org/sqlite"` 与 `sqlite3 "modernc.org/sqlite/lib"`（别名冻结）。
- `BeginTurn`（`:407-435`）里 INSERT 失败的那一处（`:431-433`，原文 `return fmt.Errorf("begin turn %s: %w", turn.TurnID, err)`）改成：

  ```go
	); err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return fmt.Errorf("begin turn %s: %w: %w", turn.TurnID, conversation.ErrInflightConflict, err)
		}
		return fmt.Errorf("begin turn %s: %w", turn.TurnID, err)
	}
  ```
  判据冻结：`errors.As` 到 `*sqlite.Error` **且** `Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE`（值 2067，`modernc.org/sqlite@v1.59.0/lib/sqlite.go:3234`）。原型实测：同 `(conv, agent)` 第二次插入报 2067；同 `turn_id` 插入报 1555（`SQLITE_CONSTRAINT_PRIMARYKEY`），**1555 原样返回、不译成哨兵**。两个 `%w` 都要（哨兵 + 原错误），不许只包哨兵丢掉驱动错误。
- 注释要点：inflight_turns 上除主键外唯一的 UNIQUE 就是 `(conv_id, agent_id)`，所以 2067 在这里只可能是它；为什么在 store 这一层译（只有本包认识 modernc，`open.go:2`）；主键冲突不译的理由（两次用同一个 turn_id 是调用方记重了，不是「别人的在飞记录」）。
- `requireConversation` 失败那一支（`:413-415`）不动。

#### E. `internal/turn/runner.go`

E1. **起进程之前的检查（本刀的主体）。** 插在 `askUpToWith` 里 `began := false`（`:350`）那一行**之后**、`if cfg == nil || cfg.Agents == nil {`（`:352`）**之前**。锚点（真文件 `:350-352`，恰好出现一次）：

```go
	began := false

	if cfg == nil || cfg.Agents == nil {
```

插入的逻辑冻结如下（三个局部变量名实现方定；错误文本措辞实现方定，但必须用 `%w` 包住所示的那个值）：

```go
	prior, priorFound, priorErr := r.svc.InflightFor(ctx, convID, agentID)
	if priorErr != nil {
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("read inflight %s/%s: %w", convID, agentID, priorErr))
	}
	if priorFound && prior.TurnID != rec.TurnID {
		return r.refuse(ctx, &rec, st, began, fmt.Errorf("agent %q: unsettled turn %s (phase %s): %w", agentID, prior.TurnID, prior.Phase, conversation.ErrInflightConflict))
	}
```

冻结要点：
- **位置**：闸已拿到（`:338-340`），任何副作用之前：早于 `:393` 的 `MkdirAll`、`:402` 的指纹、`:433` 的 `ProviderSession` 读回与 `:445-464` 的作废重建、`:478` 的 `readAfter`、`:547-559` 的 `session_opening`。所以此刻 `st` 恒为 nil、`began` 恒为 false，refuse 按它们原样传（`persistTurn` 于是不碰 `provider_sessions`，`RecordTurn` 的 `expectInflight` 是 false，只清本轮自己的 turn_id、清不到也照常提交）。
- **ctx 用调用方的**，与 `BeginTurn` 同理（13 §3.12：spawn 之前取消就该什么都不做）。
- **读失败就拒绝**：「读不出来」是「不知道」，不是「没有」，与 `:434-439` 读 `ProviderSession` 失败的处置同体例。
- 不改 `:605-623` 的 `BeginTurn` 调用与它的错误处理：store 译出的哨兵经 `fmt.Errorf("begin turn: %w", err)` 原样留在链里，第二道保险因此自动是同一个结局、同一句话。
- 注释要点（写在这段上方）：
  1. 为什么查到的那一行一定「不属于本轮」：本轮的 turn_id 刚铸好、自己的 `BeginTurn` 还没跑，库里不可能有本轮的行；生产里每个进程（每个 data_dir）只有一个 Runner（`app.go:169`），闸在 Runner 上，所以同一 `(conv, agent)` 在本进程里的每一轮都在同一把 `g.mu` 下写入并在释放闸之前经 `RecordTurn` 清掉（`persistTurn` / 成功路径都用 `Background()` 落库），所以闸空闲时还留着的行只可能是没对上账的 —— 上次崩溃、settle 失败、或本进程里一次清理也失败的双重故障（13 §3.14 那一支）；不同 key 的行 `(conv_id, agent_id)` 不同，定点读看不到；另一个 council 进程开不了同一个库（`store.ErrDataDirLocked`，`internal/store/open.go:49`，锁在 `lock_windows.go` / `lock_other.go`）；`SettleLostTurns` 只在启动时、进 REPL 之前跑。所以不需要内存里的 turn id 清单，「查库里有没有这一行」就是结构性的判据（13 §3.8「实现上不需要新状态」）。
  2. 为什么在闸之内：放在闸外，同一 key 的两次并发调用会都看到「没有」然后都去 `BeginTurn`（13 §3.8 原文）；唯一索引是第二道。
  3. 为什么这么早：见本契约 §0 第 2 条（原来撞 UNIQUE 之前已经发了 `session_opening`、指纹不符时已经删了 settle 要标的那一行 —— 后者只对 spawned / returned 行成立，begun 行只是提前删了一行对账不会去标的记录，见 §0 第 2 条的 v1.1 订正）。
  4. 怎么解开：重启 council，启动对账成功就删掉这一行；`/session rebuild` 解不开（它不碰 `inflight_turns`）。
  5. 同一 `(conv, agent)` 被拒的这一轮照常记一行 refused 的 `turn_usage`，`identity_fp` 与 `provider_session_id` 是空串 —— 与「配置里没有这个 agent」那几支早拒绝同一种形状，接受。
- 覆盖面：`Ask`（`:264-273`）、`AskUpTo`（`:306-309`）、`RunRoundWithStream`（`internal/turn/scheduler.go:214`，`RunRound` `:49`、`RunRoundPrepared` `:89`、`RunRoundPreparedStream` `:95`、`RunRoundWith` `:144` 全部经它）都只经 `askUpToWith` 到达 `BeginTurn`；生产代码里 `BeginTurn` 的调用点现查只有 `runner.go:605` 一处。REPL 的入口 `/ask`（`ask.go:166`）与 `/round`（`round.go:325`）、TUI（经 repl）因此全部被覆盖，不加任何别处的检查。

E2. **`SessionInfo.BaselineSuspect`。** `SessionInfo` 结构体（`:887-898`）在 `InFlight bool` 之后加字段 `BaselineSuspect bool`（名字、类型、位置冻结）。注释要点：「下一轮进场时会看到的基线状态」—— true 表示下一轮的金额差值一律 nil（`writeAskMeter` 那句「金额未计」）；与 `Record.BaselineSuspect`（进轮快照）、`sessionState.baselineSuspect`（活状态）、`ProviderSession.BaselineSuspect`（落库的那份）四个同名，时态见 13 §3.7a；它**不看** `inflight_turns`（那件事由 E1 的拒绝负责）。

`SessionInfo` 方法（`:831-885`）三个 `ok == true` 的返回点各赋值，判据冻结：

| 返回点 | 赋值 |
|---|---|
| 拿到闸、`g.st != nil`（`:840-845`） | `g.st.baselineSuspect` |
| 拿到闸、`g.st == nil`、库里有行（`:860-865`） | `loaded.BaselineSuspect` |
| 拿不到闸（`InFlight: true`，`:879-884`） | `loaded.BaselineSuspect` |

`ok == false` 的三处（`SessionInfo{}`）不动。方法注释补一句 `BaselineSuspect` 的来源随分支而定。

#### F. `internal/repl/ask.go`

F1. import 块（`:3-11`）加 `"council/internal/conversation"`。

F2. `askCause`（`:539-554`）在 `switch {` 之后、`case errors.Is(err, turn.ErrNoDelta):` **之前**加第一个分支：

```go
	case errors.Is(err, conversation.ErrInflightConflict):
		return "<§3.1 的那句话，逐字>"
```

注释要点：两个产出方共用一个哨兵，所以 `/ask` 与 `/round`（`round.go:338` 经同一个 `askCause`）、起进程前的检查与 BeginTurn 撞唯一索引，打的都是同一句话；为什么这句话不提 `/session rebuild`（§0 第 5 条）。`askCause` 的函数注释若列了认得的哨兵，补上这一个。

F3. `cmdSession`（`:188-239`）：
- 表头（`:203`）换成 §3.3 的字面量（在 `LAST_TURN_AT` 与 `NEXT` 之间插 `BASELINE`）。
- fresh 那一行（`:207`）换成五格：`fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.AgentID, emptyCell, emptyCell, emptyCell, "fresh")`。
- 其余行（`:225-236`）：`next` 的算法一个字不改；BASELINE 格取 `info.BaselineSuspect` 为真时 `"suspect"`、否则 `"ok"`；输出顺序 `AGENT, PROVIDER_SESSION, LAST_TURN_AT, BASELINE, NEXT`，格式串 `"%s\t%s\t%s\t%s\t%s\n"`。
- **新列必须在 NEXT 之前**：既有 `session_inflight_test.go:171-183` 的 `nextColumnFor` 取「最后一个字段」当 NEXT；新格的三个取值都不含空白、不含 `warm` / `cold` / `busy` / `fresh` 子串、不像 UUID（既有 `ask_test.go:397-420` 的断言）。
- 函数注释补一句：BASELINE 一列回答「下一轮的金额会不会报『不知道』」（13 §3.11），它不回答「下一轮会不会被拒」。

#### G. 六个既有测试假件各补一个方法（授权修改，只为编译）

`conversation.Store` 多了一个方法，下面六个既有实现不补就编译不过。**只许加这一个方法**，放在各自 `AllInflight` 方法之前或之后（紧挨着），一字不改其它内容。有状态的三个必须读自己的 map（仓库纪律：同一个假件不许对「这一行在不在」给两个答案，见 `recording_store_test.go:191-193` 那段注释）；无状态的三个本来 `AllInflight` 就恒答 nil，照样恒答「没有」。

| 文件（HEAD 上 `AllInflight` 所在行） | 补的方法 |
|---|---|
| `internal/conversation/service_test.go:198`（`*fakeStore`，无锁，map 字段 `inflight`） | `func (f *fakeStore) InflightFor(ctx context.Context, convID, agentID string) (InflightTurn, bool, error)`：遍历 `f.inflight`，`it.ConvID == convID && it.AgentID == agentID` 就返回 `it, true, nil`；否则 `InflightTurn{}, false, nil` |
| `internal/repl/conversation_test.go:326`（`*fakeStore`，有 `f.mu`，map 字段 `inflight`） | 同上，先 `f.mu.Lock(); defer f.mu.Unlock()`；类型写 `conversation.InflightTurn` |
| `internal/turn/recording_store_test.go:194`（`*recordingStore`，map 字段 `inflightTurns`） | 同上，遍历 `s.inflightTurns`；类型写 `conversation.InflightTurn` |
| `internal/turn/drift_test.go:296`（`erroringAllSessionsStore`，值接收者） | `return conversation.InflightTurn{}, false, nil` |
| `internal/turn/runner_cursor_test.go:274`（`*prefixRewriteStore`） | `return conversation.InflightTurn{}, false, nil` |
| `internal/turn/session_info_readback_test.go:67`（`erroringSessionStore`，值接收者） | `return conversation.InflightTurn{}, false, nil` |

每个方法上方一行注释说明它是为 `InflightFor` 补的（有状态的三个注明「读同一份 map」）。`step10a3_fixtures_test.go` 里三个包装类型内嵌 `*recordingStore`，自动继承，不用改。

### 1.2 不做（两条腿都不许越界）

- 不改 schema、不加迁移、不改 `schema.go`（理由见文件头）。
- 不改 `Rebuild` / `/session rebuild`（它不清在飞记录是已知缺口，记 open item，不在本刀修）；不加任何能在 REPL 里触发对账的命令；不改 `internal/app`（启动对账、横幅）、`recover.go`、`record.go`、`scheduler.go`、`round.go`、`commands.go`、`internal/tui`、`cmd/`、`go.mod`。
- 不改 `/round` 预算表（它的「在飞」列此时仍印「不在飞」并照常预报金额，原型实测；记 open item）。
- 不改 `/session` 的 NEXT 列算法（不加 `refused` 之类的状态；与 `00_DECISIONS.md` #40(a) 同族，记 open item）。
- 不改 `Service.BeginTurn` / `Service.SetInflightPhase` / `Tx.ClearInflight` 的行为；不在 `Service.BeginTurn` 里加预读（它 `:710-712` 的注释照旧成立：撞了就说明调用方绕过了 E1）。
- 不加 `r.svc == nil` 判（13 §3.13a）。
- 不做 C2 端到端（另一路在做），不做 `/log` 标记（D 之二）。
- **既有 `_test.go` 只许改 §1.1 G 的六处**，每处只加那一个方法；其余一个字不动（含注释）。原型实测：只做 G 的六处，全量既有用例全绿，没有任何一条因本刀变红。
- 测试腿不写生产代码、不改既有 `_test.go`；实现腿除 G 的六处外不写 `_test.go`。

---

## 2. 验收（测试腿新写的文件）

通用纪律：

- 新文件共四个（§2.1-2.4）；包级标识符（类型、函数、变量、常量）一律以 `s10a` 开头，测试函数名 `TestS10a...`。不新写 `TestMain`：repl 包复用 `session_inflight_test.go:94` 的，app 包复用 `smoke_test.go:40` 的。
- 不起真 provider：假 CLI 一律是测试二进制自己（`COUNCIL_FAKE_CLI=1`、Command 写 `os.Args[0]`）或 turn 包的 `fakeSpawner`。store 一律 `store.Open` 到 `t.TempDir()` 之下，不碰 `C:\Users\25142\.council\data`。
- **闸与库的交互至少一条跑在真 store 上**：§2.1 全部、§2.2、§2.4 都是真 SQLite；repl 包的屏幕用例用 repl 的 `fakeStore`（它不强制唯一索引，所以撞不到第二道，恰好证明第一道），另有 R1 的第一个产出方走真 store。
- 断言整行相等或 `strings.Fields` 切片整体相等（`reflect.DeepEqual`）；不写子串断言。唯一例外是 `Fatalf` 前提守卫（前提不成立即停、不是被测断言）可以用子串或前缀，例如 E1 的「Err 里有 `启动在飞对账补账失败: ` 开头的一行」。
- 每条注释写【判别力】：它能不能为假、只差一处的对照是哪条。
- 允许只读引用同包既有测试辅助（不许改它们）：
  - turn：`newRealTurnStore` / `realTurnStore.rawDB`（`step10a3_fixtures_test.go:51`、`:70`）、`newConv`、`joinAgent`、`postUser`、`newTestConfig`、`newTestRunner`、`newTestRunnerSameWorkDir`、`fakeSpawner`（`enqueue` / `enqueueSuccess` / `calls`）、`spawnResult`、`claudeT1JSON`、`agyT1JSON`、`testAgentClaude`（`"alice"`）、`testAgentAgy`（`"nova"`）、`sessionKey`、`(*Runner).gate`、`sessionGate.mu` / `.st`、`sessionState` 的 `providerSession` / `baselineSuspect` 字段（白盒）、`Runner.workDir` 字段（白盒，G1 断言 7 用；v1.1）。
  - store：`tstStore`、`tstCtx`、`tstNewConv`、`tstBaseInflight`、`tstBeginTurn`、`tstClearInflight`、`tstAssertInflightEqual`（`inflight_test.go`）；`tstRawDB`（`open_test.go:84`）与 `(*Store).Path`（S1 坏行那一例用；v1.1）。
  - repl：`newTestService`（返回 `*conversation.Service, *fakeStore`）、`f.seedConversation` / `seedParticipant` / `seedMessage` / `seedProviderSession`、`f.msgs`、`newInFlightRunner`、`inFlightAgentID`（`"ag_wait"`）、`inFlightProbeDelay`（R4 用；v1.1）、`runREPLRunner`、`lineContaining`、`newTestRunner`（repl 版）、`askCause`。
  - app：`isolateAppEnv`、`tomlLiteral`、`writeTestConfig`、`runAppForTest`（`app_test.go:42-101`）。
- **HEAD 上的状态**：§2.1、§2.2 引用本刀新增的标识符，在 HEAD 上整包编译不过，这是预期的红。**§2.3（repl）与 §2.4（app）两个文件只用 HEAD 已有的标识符**，测试腿交付前必须在 HEAD 的隔离拷贝里**只放这两个新文件**各跑一次，报告里写出命令与每条用例的红/绿。原型实测的 HEAD 结果：R1 红（产出方一打出裸 SQLite 错误文本 `begin turn s10a-t2: begin turn s10a-t2: constraint failed: UNIQUE constraint failed: inflight_turns.conv_id, inflight_turns.agent_id (2067)` 这一形状，比 §0 第 1 条少 runner 加的那一层前缀）；R2 红（假件不强制唯一索引，HEAD 真的起了假 CLI、打 `outcome spoke`）；R3 红；R4 红（表头只有四列、fresh 行四格、v1.1 新增的 `ag_wait` 行也只有四格）；E1 的 `settle_fails` 红（结局已是 refused，但缺那句话）、`settle_ok` 绿。

### 2.0 共用装置（冻结）

- **别人的在飞记录**（turn 包与 store 包用）：经 `svc.BeginTurn`（或 store 包的 `tstBeginTurn`）写入
  `conversation.InflightTurn{TurnID: "s10a-stale-turn", ConvID: <会话>, AgentID: <成员>, Provider: "claude", PlannedSessionID: "s10a-planned", ResumeSessionID: "", CursorSeq: 0, ScannedTo: 1, StartedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Phase: <phase>, Tier: "read", WorkDir: "s10a-wd"}`。`<phase>`（v1.1）：G1 里是子测试参数，取 conversation 的 `InflightPhase` 合法值全集 `{conversation.PhaseBegun, conversation.PhaseSpawned, conversation.PhaseReturned}`（`types.go` 的三态，`InflightPhase.Valid` 认的就是这三个）；其余用例（G1e、G2、R1-R3）一律 `conversation.PhaseSpawned`。
- **§3.1 那句话**在每个用到它的测试文件里定义成包级常量（名字 `s10a...`），逐字复制 §3.1。

### 2.1 `internal/turn/s10a_inflight_gate_test.go`（真 store：`newRealTurnStore`）

**G1 `TestS10aAskRefusedByForeignInflight`**，三个场景 × 三个 phase 共 9 个子测试（v1.1），名字 `<场景>_<phase>`：`bootstrap_begun`、`bootstrap_spawned`、`bootstrap_returned`、`resume_begun`、`resume_spawned`、`resume_returned`、`fpmismatch_begun`、`fpmismatch_spawned`、`fpmismatch_returned`（phase 段取 `string(conversation.PhaseXxx)`）；每个子测试写入的「别人的在飞记录」是 §2.0 的夹具、`Phase` 取该子测试的 phase。共同前置：`rs := newRealTurnStore(t)`、`convID := newConv`、`joinAgent(alice)`、`postUser("hello")`、`cfg := newTestConfig()`、`r := newTestRunner(t, cfg, rs.svc)`、`fs := &fakeSpawner{}`、`r.spawn = fs.spawn`。
- `bootstrap`：直接写入别人的在飞记录（alice）。
- `resume`：先 `fs.enqueueSuccess(claudeT1JSON)` 跑一轮 `r.Ask` 成功（前提守卫），再 `postUser("second")`，再写入别人的在飞记录。
- `fpmismatch`：同 `resume` 的前半，然后 `cfg2 := newTestConfig()`，把 alice 的 `Model` 改成 `"claude-other"`，`r = newTestRunnerSameWorkDir(r, cfg2, rs.svc)`、`r.spawn = fs.spawn`，再写入别人的在飞记录。
然后记下「之前」：`svc.ProviderSession(alice)` 的 `(值, found)`、`svc.History(ctx, convID, 0, 100)` 的条数、`len(fs.calls)`；调 `rec, err := r.Ask(ctx, convID, alice)`。断言（每个子测试全部）：
1. `rec.Outcome == OutcomeRefused` 且 `errors.Is(err, conversation.ErrInflightConflict)`；
2. `len(fs.calls)` 与之前相等；
3. `History` 条数与之前相等（没有 `session_opening`、没有 `session_rebuilt`）；
4. `ProviderSession` 的 `found` 与值都与之前相等（结构体 `==`）；
5. `svc.InflightFor(ctx, convID, alice)` 返回 `ok == true`，12 个字段与写入的那条逐一相等（`StartedAt` 用 `Equal`）；
6. `rs.rawDB(t)` 查 `turn_usage WHERE turn_id = rec.TurnID`：恰好 1 行，`outcome == 7`，`identity_fp == ""`；
7. （v1.1，仅 `bootstrap_*`）拒绝之后 `r.workDir` 不存在：`_, statErr := os.Stat(r.workDir)`，`errors.Is(statErr, os.ErrNotExist)`。拒绝发生在 `:393` 的 `MkdirAll` 之前，`newTestRunner` 给的 workDir 此前没有任何东西建过；`resume_*` / `fpmismatch_*` 的前一轮已经建过，不断言。
【判别力】原型实测（v1.1 重推）：「检查挪到 BeginTurn 之前」（§4 #2）下三个 phase 结果一致：`bootstrap_*` 第 3 条（History 1 -> 2，多出 `session_opening`）、第 6 条（`identity_fp` 非空）、第 7 条（workDir 已建）红；`resume_*` 第 6 条红；`fpmismatch_*` 第 3 条（4 -> 6，多出 `session_rebuilt` 与 `session_opening`）、第 4 条（`provider_sessions` 那一行被删）、第 6 条红。phase 参数化防的是 §4 #19（闸按 phase 过滤）：只放过 begun 时三个 `*_begun` 红、只放过 returned 时三个 `*_returned` 红，其余六个绿。第 7 条防的是 §4 #20（检查挪到 `MkdirAll` 之后，G1 只有它红）。对照是 G1c。

**G1c `TestS10aAskControlWithoutForeignInflightSpeaks`**：同 `bootstrap` 前置，但**不写**在飞记录，`fs.enqueueSuccess(claudeT1JSON)`；断言 `err == nil`、`rec.Outcome == OutcomeSpoke`、`len(fs.calls) == 1`、`svc.InflightFor(alice)` 返回 `ok == false`。

**G1e `TestS10aForeignInflightCheckedBeforeAgentLookup`**（v1.1 新增，真 store）：同 `bootstrap` 前置（`joinAgent(alice)`、`postUser("hello")`），写入别人的在飞记录（alice，`PhaseSpawned`），但 `cfg := newTestConfig()` 之后先 `delete(cfg.Agents, "alice")` 再 `r := newTestRunner(t, cfg, rs.svc)`、`r.spawn = fs.spawn`；`rec, err := r.Ask(ctx, convID, "alice")`。断言：
1. `rec.Outcome == OutcomeRefused`；
2. `errors.Is(err, conversation.ErrInflightConflict)`；
3. `!errors.Is(err, ErrAgentUnknown)`；
4. `len(fs.calls) == 0`。
【判别力】检查挪到 `cfg.Agents` 查找之后（§4 #21）或 `MkdirAll` 之后（§4 #20）时，错误变成 `agent "alice": agent is not in the config`，第 2、3 条红（原型实测）。它钉的是「E1 在配置查找那几支早拒绝之前」；G1 各子测试里 alice 都在配置里，抓不到这一种漂移。对照是 G1 的 `bootstrap_spawned`（只差配置里有没有 alice，结局同为 refused + `ErrInflightConflict`）。

**G2 `TestS10aAskUpToAndRunRoundAreGated`**：
- (i) G1 `bootstrap` 前置，调 `r.AskUpTo(ctx, convID, alice, 0, "s10a-round-id")`：`Outcome == OutcomeRefused`、`errors.Is(err, conversation.ErrInflightConflict)`、`len(fs.calls) == 0`。
- (ii) 新会话：`joinAgent` alice 与 nova、`postUser("hello")`，只给 alice 写入别人的在飞记录，`fs.enqueueSuccess(agyT1JSON)` 一次；`recs := r.RunRound(ctx, convID, Plan{Speakers: []string{"alice", "nova"}}, 0)`。断言 `len(recs) == 2`；`recs[0].AgentID == "alice"`、`recs[0].Outcome == OutcomeRefused`、`errors.Is(recs[0].Err, conversation.ErrInflightConflict)`；`recs[1].AgentID == "nova"`、`recs[1].Outcome == OutcomeSpoke`；`len(fs.calls) == 1`。
【判别力】一人被拒不取消别人（scheduler 的既有设计）；nova 那一行是对照。

**G3 `TestS10aUniqueCollisionCarriesSameSentinel`**：包级包装类型 `s10a...`，内嵌 `*store.Store`，只重写 `InflightFor`：**先调内嵌的真实现拿到结果**，第一次被调用时再用内嵌的 `WriteTx` 执行 `tx.BeginTurn(InflightTurn{TurnID: "s10a-race-turn", ConvID: convID, AgentID: agentID, Provider: "claude", StartedAt: time.Now().UTC(), Phase: PhaseBegun, Tier: "read"})`（写失败就 `t.Fatalf` 或 panic），然后返回**第一步的真实结果**（模拟「检查之后、BeginTurn 之前，这一行出现了」；它不撒谎，读的那一刻确实没有）。`svc := conversation.NewService(包装)`，G1 `bootstrap` 那样建会话（不写 `s10a-stale-turn`），`r.Ask`。断言：
- 前提守卫：`var se *sqlite.Error; errors.As(err, &se) && se.Code() == 2067`（证明真的走到了第二道，import `"modernc.org/sqlite"`）；
- `rec.Outcome == OutcomeRefused`、`errors.Is(err, conversation.ErrInflightConflict)`、`len(fs.calls) == 0`。
【判别力】store 不译哨兵时第二条红；译的时候丢掉驱动错误时前提守卫红。

**G4 `TestS10aInflightReadErrorRefuses`**：包装类型内嵌 `*store.Store`，`InflightFor` 恒返回一个包级错误变量 `s10a...`（`errors.New("s10a injected read failure")`）。建会话、`joinAgent`、`postUser`，`fs.enqueueSuccess(claudeT1JSON)`（不拒的话会真的成功）。断言 `rec.Outcome == OutcomeRefused`、`errors.Is(err, 注入的错误)`、`!errors.Is(err, conversation.ErrInflightConflict)`、`len(fs.calls) == 0`。

**G5 `TestS10aSessionInfoBaselineSuspect`**（真 store，白盒两步）：`key := sessionKey{convID: convID, agentID: "alice"}`。
1. `enqueueSuccess(claudeT1JSON)`，`r.Ask` 成功；`r.SessionInfo` -> `ok`、`!BaselineSuspect`（内存分支，假）。
2. `postUser("second")`；`fs.enqueue(spawnResult{killed: true, err: context.DeadlineExceeded})`；`r.Ask`，前提守卫 `rec.Outcome == OutcomeTimeout`；`r.SessionInfo` -> `ok`、`!InFlight`、`BaselineSuspect`（内存分支，真）。前提守卫：`rawDB` 查 `provider_sessions` 该行 `baseline_suspect == 1`。
3. `r2 := newTestRunnerSameWorkDir(r, cfg, svc)`；`r2.SessionInfo` -> `ok`、`!InFlight`、`BaselineSuspect`（拿到闸、读库分支，真）。
4. `r2.gate(key).mu.Lock()`，调 `r2.SessionInfo`，再 `Unlock`（或 `defer`）-> `ok`、`InFlight`、`BaselineSuspect`（拿不到闸分支，真）。
5. 对照会话 `conv2`：`joinAgent`、`postUser`、一轮成功（库里 `baseline_suspect == 0`，前提守卫）；`r3 := newTestRunnerSameWorkDir(r, cfg, svc)`：`r3.SessionInfo(conv2)` -> `!BaselineSuspect`；`r3.gate(conv2 的 key).mu.Lock()` 下 -> `InFlight`、`!BaselineSuspect`。
6. 白盒：`r4 := newTestRunnerSameWorkDir(...)`，`r4.gate(key).st = &sessionState{providerSession: "s10a-mem", baselineSuspect: false}`（库里是 1）-> `!BaselineSuspect`；`r4.gate(conv2 的 key).st = &sessionState{providerSession: "s10a-mem", baselineSuspect: true}`（库里是 0）-> `BaselineSuspect`。
【判别力】三个分支各有真、假两个值；第 6 步证明内存分支读的是内存，不是库。

### 2.2 `internal/store/s10a_inflight_for_test.go`（`package store`）

**S1 `TestS10aInflightForFindsOnlyExactKey`**：会话 A、B（`tstNewConv`）。写入三行（`tstBaseInflight` 改 turn id）：`(A, "alpha", "s10a-a-alpha")`、`(A, "beta", "s10a-a-beta")`、`(B, "gamma", "s10a-b-gamma")`。断言：
- `(A, alpha)`：`ok`、`err == nil`、`tstAssertInflightEqual` 与写入的那条相等；
- `(A, beta)`：`ok`、`TurnID == "s10a-a-beta"`；
- `(A, gamma)`：`!ok`、`err == nil`、返回零值；
- `(B, alpha)`：`!ok`、`err == nil`；
- `("s10a-no-such-conv", alpha)`：`!ok`、`err == nil`；
- `tstClearInflight(t, ctx, s, "s10a-a-alpha")` 之后 `(A, alpha)`：`!ok`。
- （v1.1）坏行：`db := tstRawDB(t, s.Path())`，直写一行（SQL 逐字，参数 `A.ID`；省略的列走 DDL 的 DEFAULT，`inflight_turns` 没有外键）：`INSERT INTO inflight_turns (turn_id, conv_id, agent_id, provider, cursor_seq, scanned_to, started_at) VALUES ('s10a-bad-time', ?, 'delta', 'claude', 0, 0, 'not-a-time')`；`(A, "delta")`：`err != nil` 且 `!ok`。
【判别力】SQL 漏 `agent_id` 谓词时 `(A, gamma)` 红；漏 `conv_id` 谓词时 `(B, alpha)` 红；把扫描错误当成「没有」（§4 #22）时坏行那一例红（`ok == false`、`err == nil`，原型实测）。

**S2 `TestS10aBeginTurnConflictCarriesSentinel`**：会话 A，先写 `(A, "alpha", "s10a-first")`。
- (a) 再写 `(A, "alpha", "s10a-second")`：`errors.Is(err, conversation.ErrInflightConflict)`，且 `var se *sqlite.Error; errors.As(err, &se) && se.Code() == 2067`；第一行原样（`tstAssertInflightEqual`）。
- (b) 写 `(A, "beta", "s10a-first")`（主键冲突）：`err != nil`、`!errors.Is(err, conversation.ErrInflightConflict)`。
- (c) 写 `(A, "beta", "s10a-third")`：`err == nil`。
- (d) 经 `conversation.NewService(s).BeginTurn` 重复 (a) 的冲突：`errors.Is(err, conversation.ErrInflightConflict)`（证明 Service 那一层的 `%w` 没把它吞掉）。
【判别力】(b) 是 (a) 的对照：只差「哪条约束撞了」。

### 2.3 `internal/repl/s10a_gate_screen_test.go`（只用 HEAD 已有标识符）

**R1 `TestS10aAskCauseSameSentenceForBothProducers`**：
- 产出方一（store 撞唯一索引，真库）：`store.Open(ctx, filepath.Join(t.TempDir(), "s10a-data"))`，`conversation.NewService`，`Create` 一个会话，同一 `(conv, "s10a-g")` 用 `"s10a-t1"`、`"s10a-t2"` 两次 `BeginTurn`（`Provider: "claude"`、`Phase: conversation.PhaseBegun`），第二次的错误 `e1`：`askCause(e1) == §3.1`。
- 产出方二（起进程之前的检查）：`svc, f := newTestService(t)`、`rn := newInFlightRunner(t, svc)`，`f.seedConversation(convID, "s10a")`、`f.seedParticipant(t, convID, inFlightAgentID)`、`f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "seed-msg")`，经 `svc.BeginTurn` 写入别人的在飞记录（`AgentID: inFlightAgentID`，其余字段照 §2.0），`_, e2 := rn.Ask(ctx, convID, inFlightAgentID)`：先断言 `e2 != nil`（为 nil 就 `Fatalf`，不许把 nil 交给 `askCause`，它会在 `err.Error()` 上 panic），再断言 `askCause(e2) == §3.1`。
- 对照：`askCause(errors.New("s10a-plain")) == "s10a-plain"`。

**R2 `TestS10aAskScreenForeignInflight`**，两个子测试，装置同 R1 产出方二：
- `foreign`：写入别人的在飞记录后 `out, err := runREPLRunner(t, svc, rn, "/use "+convID+"\n/ask "+inFlightAgentID+"\n")`。断言 `err == nil`；按 `"\n"` 切行后，存在下标 i 使第 i 行整行等于 `outcome refused  agent ag_wait`、第 i+1 行整行等于 §3.1；没有任何一行以 `用量`、`金额`、`冷暖` 开头；`len(f.msgs[convID]) == 1`（只有那条种子消息）。
- `control`：不写在飞记录，同一命令；存在整行 `outcome spoke  agent ag_wait`；没有任何一行等于 §3.1。

**R3 `TestS10aRoundScreenForeignInflight`**：`t.Setenv("COUNCIL_FAKE_CLI", "1")`；配置 `Providers: {"claude": {Command: os.Args[0]}}`，两个 agent `"s10a_a"`、`"s10a_b"`：`DisplayName` 各自 `"A"` / `"B"`、`Provider: "claude"`、`Model: "claude-fable-5-1"`、`Effort: "high"`、`Role: "s10a"`、`Permissions: config.PermissionRead`、`Mode: config.ModeDiscussion`；`rn := turn.NewRunner(cfg, svc, filepath.Join(t.TempDir(), "workdir"), turn.RulesBlocks{})`，`svc, f := newTestService(t)`；种会话、两个成员、一条 `seed-msg`；只给 `s10a_a` 写入别人的在飞记录；`runREPLRunner(t, svc, rn, "/use "+convID+"\n/round s10a_a s10a_b --go\n")`。断言：存在下标 i 使第 i 行整行等于 `outcome refused  agent s10a_a`、第 i+1 行整行等于 `"/round s10a_a: " + §3.1`；存在整行 `outcome spoke  agent s10a_b`。不断言两个成员之间的先后（完成顺序不定）。

**R4 `TestS10aSessionBaselineColumn`**：`svc, f := newTestService(t)`、`rn := newInFlightRunner(t, svc)`（v1.1：换成能真起假 CLI 的那个，理由见下面的 `ag_wait`；另三个成员的格子只读库，与换哪个 Runner 无关，原型实测）；种会话与四个活跃成员 `ag_alpha`、`ag_mid`、`ag_zeta`、`inFlightAgentID`（`ag_wait`）；`lt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)`；`f.seedProviderSession` 两条：`ag_alpha`（`ProviderSessionID: "sess-alpha"`、`CursorSeq: 1`、`LastTurnAt: lt`、`IdentityFP: "fp"`、`Spoken: true`、`BaselineSuspect: true`）与 `ag_mid`（同上但 `"sess-mid"`、`BaselineSuspect: false`）；`ag_zeta` 不种。（v1.1）`ag_wait` 做成「在飞、基线不可疑」：`f.seedMessage(t, convID, conversation.ReservedSenderUser, conversation.KindUser, "seed-msg")`；`rn.Ask(ctx, convID, inFlightAgentID)` 首轮，前提守卫 `err == nil` 且 `rec.Outcome == turn.OutcomeSpoke`；`ps, found, err := svc.ProviderSession(ctx, convID, inFlightAgentID)`，前提守卫 `err == nil && found && !ps.BaselineSuspect`；再 `seedMessage` 一条 `"second-msg"`，另起 goroutine 调 `rn.Ask`（`sync.WaitGroup` + `t.Cleanup(wg.Wait)`，体例照 `session_inflight_test.go:238-247`），`time.Sleep(inFlightProbeDelay)` 之后跑下面的 `/session`，跑完 `wg.Wait()`，前提守卫第二轮 `err == nil`。`out := runREPLRunner(..., "/use "+convID+"\n/session\n")`。断言（`strings.Fields` 后 `reflect.DeepEqual`）：
- 含 `PROVIDER_SESSION` 的那一行，从等于 `"AGENT"` 的那个字段起截到末尾，等于 `[AGENT PROVIDER_SESSION LAST_TURN_AT BASELINE NEXT]`（前面可能有提示符 `council>`，截掉）；
- `lineContaining(t, out, "ag_alpha")` -> `[ag_alpha sess-alpha 2026-01-02T03:04:05Z suspect cold]`；
- `ag_mid` -> `[ag_mid sess-mid 2026-01-02T03:04:05Z ok cold]`；
- `ag_zeta` -> `[ag_zeta - - - fresh]`；
- （v1.1）`lineContaining(t, out, "ag_wait")` -> `[ag_wait 2a90d449-a348-497a-8812-e73e74d6898e <ps.LastTurnAt.UTC().Format(time.RFC3339)> ok busy]`（会话 id 是假 CLI 回放的 `fakeClaudeSuccessJSON` 里的 `session_id`；时间取上面读回的 `ps.LastTurnAt`，InFlight 分支从库里读的正是这一行）。
【判别力】`ag_alpha` 与 `ag_mid` 只差 `BaselineSuspect` 一处。`cold` 是 `lt` 远早于 CacheTTL 算出来的，原型实测。`ag_wait` 是「在飞但基线不可疑」：BASELINE 在 InFlight 时一律印 `suspect`（§4 #23）只让它这一格变红（原型实测），它与 `ag_alpha` 互为对照。时序沿用 `TestSessionNextColumnBusyDuringInFlightThenWarm` 的 700ms 睡眠 / 350ms 探测窗口，不另定常数。

### 2.4 `internal/app/s10a_settle_fail_blocks_ask_test.go`（只用 HEAD 已有标识符）

**E1 `TestS10aSettleFailureBlocksAskUntilSettled`**，两个子测试 `settle_fails` / `settle_ok`。每个子测试：`isolateAppEnv(t)`；`t.Setenv("COUNCIL_FAKE_CLI", "1")`；`dir := t.TempDir()`、`dataDir := filepath.Join(dir, "data")`；配置文本（一个 agent `c1`，照 `smoke_test.go:91-104` 的体例）：

```
data_dir = <tomlLiteral(dataDir)>

[providers.claude]
command = <tomlLiteral(os.Args[0])>

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "s10a"
permissions = "read"
mode = "discussion"
```

种库（`store.Open(ctx, dataDir)` + `conversation.NewService`，种完 `Close`）：`Create("s10a")`；`Join(c1, Provider "claude", JoinedAt now)`；`Post` 一条 user 消息 `"hi"`；**仅 `settle_fails`**：先 `svc.RecordTurn(ctx, nil, conversation.TurnUsage{TurnID: "s10a-stale-turn", ConvID: conv.ID, AgentID: "c1", Outcome: 7, CreatedAt: time.Now().UTC()}, nil, false)`（让启动对账补 `turn_usage` 时撞主键、整笔回滚，从而 settle 失败、在飞记录留下）；然后两个子测试都 `svc.BeginTurn(InflightTurn{TurnID: "s10a-stale-turn", ConvID: conv.ID, AgentID: "c1", Provider: "claude", StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, Tier: "read"})`。先 `RecordTurn` 后 `BeginTurn` 的顺序冻结（反过来 `RecordTurn` 会把在飞记录清掉）。

`out, errOut, err := runAppForTest(t, ctx, cfgPath, strings.NewReader("/use "+conv.ID+"\n/ask c1\n/quit\n"))`，`err == nil`。另开 `sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dataDir, "council.db")))` 查库。

- `settle_fails`：前提守卫 `errOut` 里有一行以 `启动在飞对账补账失败: ` 开头；`out` 里有整行 `outcome refused  agent c1` 与整行 §3.1；没有整行 `outcome spoke  agent c1`；`inflight_turns` 恰好一行，`turn_id = 's10a-stale-turn'`、`phase = 'spawned'`；`turn_usage` 里 `outcome = 1` 的 0 行、`outcome = 7` 的 2 行（种下的一行 + 本次被拒的一行）。
- `settle_ok`：`errOut` 里没有以 `启动在飞对账补账失败` 开头的行；`out` 里有整行 `outcome spoke  agent c1`、没有整行 §3.1；`inflight_turns` 0 行；`turn_usage` 里 `outcome = 9` 的 1 行、`outcome = 1` 的 1 行。
【判别力】两个子测试只差「启动对账成不成功」一处：这条用例同时证明「settle 失败则被拒」（13 §3.15 第 10 条，全栈、真库）与「对账成功就解开」（§0 第 5 条的唯一解开路径）。原型实测两条与上述逐项一致。

---

## 3. 冻结的字面量

### 3.1 拒绝那句话（`askCause` 新分支的返回值，逐字节）

```
这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行
```

（全角逗号 U+FF0C、顿号 U+3001、分号 U+FF1B、直角引号 U+300C / U+300D；`council` 前后各一个 ASCII 空格；无句号、无换行。）`/ask` 下它单独成一行，紧跟在 `outcome refused  agent <id>` 之后；`/round` 下是既有格式 `"/round %s: %s\n"`（`round.go:338`）拼出来的 `/round <id>: ` + 这句话。

### 3.2 哨兵

```go
ErrInflightConflict = errors.New("an unsettled inflight turn already exists for this conversation and agent")
```

在 `internal/conversation`（`store.go` 的哨兵块）。

### 3.3 `/session` 表头与新格

```go
"AGENT\tPROVIDER_SESSION\tLAST_TURN_AT\tBASELINE\tNEXT"
```

BASELINE 格的取值只有三个：`suspect`（`info.BaselineSuspect` 为真）、`ok`（为假）、`-`（fresh 行，即既有常量 `emptyCell`）。

### 3.4 `InflightFor` 的 SQL

见 §1.1 C。

### 3.5 测试要比对的既有文本（不是本刀新增）

`outcome refused  agent %s`、`outcome spoke  agent %s`（`ask.go:313`，`outcome` 与 `agent` 之间两个空格）；`启动在飞对账补账失败: `（`app.go:197` 的前缀，只作前提守卫）；`启动在飞对账失败: `（`app.go:190`，`AuditInflight` 读失败时打；v1.1 补：§3.1 那句话说的「以「启动在飞对账」开头的那一行」是这两条之一，本刀没有用例比对它）；`/round %s: %s`（`round.go:338`）。常量：`SQLITE_CONSTRAINT_UNIQUE = 2067`、主键冲突 1555（原型实测）。

**本刀新增的用户可见字面量只有 §3.1 那一句、§3.3 的表头与 `suspect` / `ok` 两个格值。** 错误值的 `Error()` 文本（E1 的两个 `fmt.Errorf`、store 的包装）不进屏幕（`/ask` 与 `/round` 都经 `askCause`），措辞不冻结，但 `%w` 冻结。

---

## 4. 集成员的变异清单

「预期被谁杀」是至少这些；主代理落地后逐条跑，**从副本还原，不用 `git checkout`**。「原型实测」表示已在原型上跑过、点名的杀手确实红了。

| # | 变异（可编译） | 预期被谁杀 |
|---|---|---|
| 1 | runner.go：E1 的检查整段失效（`if false && priorFound ...`） | G1 九个子测试；G1e；R2 `foreign`；R1 产出方二；R3（原型实测，v1.1 重推）。**E1 杀不到**：真库下会落到第二道，照样同一句话 —— 这正是第二道存在的意义 |
| 2 | runner.go：检查挪到 `BeginTurn`（`:605`）正前方 | G1（三个 phase 一致）：`bootstrap_*` 第 3、6、7 条，`resume_*` 第 6 条，`fpmismatch_*` 第 3、4、6 条；G1e（原型实测，v1.1 重推） |
| 3 | runner.go：`priorErr != nil` 那一支失效（读失败当「没有」） | G4（原型实测） |
| 4 | runner.go：`prior.TurnID != rec.TurnID` 改 `==` | G1（原型实测） |
| 5 | runner.go：检查里的 refuse 传 `true` 代替 `began` | G1 第 6 条（`RecordTurn` 报 `ErrInflightMissing` 整笔回滚，本轮那行 refused 不见了；原型实测） |
| 6 | tx.go：不译哨兵（`se.Code() == -1`） | G3；S2 (a)(d)；R1 产出方一（原型实测 G3、S2、R1） |
| 7 | tx.go：任何 `*sqlite.Error` 都译成哨兵（去掉 `Code()` 判据） | S2 (b)（原型实测） |
| 8 | tx.go：译的时候只 `%w` 哨兵、丢掉驱动错误 | G3 前提守卫；S2 (a) 的 `errors.As` |
| 9 | ask.go：`askCause` 的新分支失效 | R1；R2；R3；E1 `settle_fails`（原型实测 R1-R3；E1 在 HEAD 上实测同样红） |
| 10 | read.go：SQL 去掉 `agent_id` 谓词（`AND (agent_id = ? OR 1=1)`） | S1 `(A, gamma)`（原型实测） |
| 11 | read.go：SQL 去掉 `conv_id` 谓词 | S1 `(B, alpha)`（原型实测） |
| 12 | runner.go：内存分支 `BaselineSuspect: false` | G5 第 2 步、第 6 步（原型实测） |
| 13 | runner.go：拿到闸读库分支 `BaselineSuspect: false` | G5 第 3 步（原型实测） |
| 14 | runner.go：拿不到闸分支 `BaselineSuspect: false` | G5 第 4 步（原型实测） |
| 15 | ask.go：`suspect` / `ok` 恒为 `ok` | R4（原型实测） |
| 16 | ask.go：fresh 行仍是四格 | R4 `ag_zeta`（原型实测） |
| 17 | ask.go：表头没加 `BASELINE` | R4 表头 |
| 18 | ask.go：新列放在 NEXT 之后（表头与行同时调换） | R4；既有 `TestSessionNextColumnBusyDuringInFlightThenWarm`（原型实测两者都红） |
| 19 | runner.go：E1 按任一 phase 值过滤或放行（`if priorFound && prior.TurnID != rec.TurnID && prior.Phase != conversation.PhaseBegun {`；或 `!= conversation.PhaseReturned`、`!= conversation.PhaseSpawned`）（v1.1） | 放过 begun：G1 `bootstrap_begun` 断言 3（History 多出 `session_opening`）与断言 6（`identity_fp` 非空）；放过 returned：G1 `bootstrap_returned` 同两条。同时红的还有该 phase 的 `bootstrap_*` 断言 7、`resume_*` 断言 6、`fpmismatch_*` 断言 3、4、6；放过 spawned 时另有 G1e、R1 产出方二、R2 `foreign`、R3 红（原型实测三种写法）。**R2、R3、E1 对 begun / returned 两种写法杀不到**：它们的外来行是 spawned，照样被闸拒掉 —— v1 漏掉它就是因为这个；放过 spawned 时 E1 仍绿，靠的是真库的第二道 |
| 20 | runner.go：检查挪到 `MkdirAll`（`:393-395`）之后（v1.1） | G1 `bootstrap_*` 断言 7（三个 phase）；G1e（挪到这里也就落在了 `cfg.Agents` 查找之后）。G1 其余断言全绿：此刻指纹没算、`session_opening` 没发（原型实测） |
| 21 | runner.go：检查挪到 `cfg.Agents` 查找（`:357-362`）之后、`provider.Lookup`（`:363`）之前（v1.1） | G1e（原型实测；G1 全绿） |
| 22 | read.go：`InflightFor` 把扫描错误当成「没有」（非 `sql.ErrNoRows` 的错误也返回 `(conversation.InflightTurn{}, false, nil)`）（v1.1） | S1 坏行那一例（原型实测） |
| 23 | ask.go：InFlight 时 BASELINE 一律印 `suspect`（`if info.BaselineSuspect \|\| info.InFlight`）（v1.1） | R4 `ag_wait`（原型实测） |

§1.1 G 的六个假件不是生产代码，不列变异；但若 repl `fakeStore` 的 `InflightFor` 被写成恒答「没有」，R2 `foreign` 会起假 CLI 而红（按构造），集成员收割时照 §1.1 G 的表逐个核对三个有状态假件确实读自己的 map。

**判定为等价、不列杀手的变异**（写在这里，免得被当成漏测）：
- 去掉 `prior.TurnID != rec.TurnID` 这一半、只剩 `priorFound`：检查在本轮自己的 `BeginTurn` 之前，库里不可能有本轮的行，两种写法可观测行为相同。保留这一半只为防将来有人把检查挪到 `BeginTurn` 之后。
- 检查里的 refuse 传 `nil` 代替 `st`：此刻 `st` 恒为 nil。
- `askCause` 里新分支排在别的哨兵分支之后：没有任何错误链同时带两个哨兵。

**只在审查时看（没有用例能确定地抓，集成员收割时逐字核对）**：
- 检查必须在 `g.mu.Lock()`（`:339`）之后。挪到取闸之前，同一 key 的两次并发调用会都看到「没有」，只有真并发的竞态能抓，本机又不能 `-race`；第二道（唯一索引）会兜住结局，但那是兜底，不是这道闸。
- 检查用的是调用方 `ctx`，不是 `context.Background()`：两种写法在「ctx 已取消」时都以 refused 收场（后面的 `readAfter` 同样会失败），可观测结局相同，只有错误链不同。
- 注释里不许写「`/session rebuild` 可以解开」。

---

## 5. 实现腿（grok）交付格式与纪律

- **严禁用你的工具写盘，只在回复正文输出。** 不创建、不修改、不删除任何文件。
- 只许改这 12 个文件：生产 6 个 —— `internal/conversation/store.go`、`internal/conversation/service.go`、`internal/store/read.go`、`internal/store/tx.go`、`internal/turn/runner.go`、`internal/repl/ask.go`；授权的既有测试假件 6 个（§1.1 G，每个只加一个方法）—— `internal/conversation/service_test.go`、`internal/repl/conversation_test.go`、`internal/turn/recording_store_test.go`、`internal/turn/drift_test.go`、`internal/turn/runner_cursor_test.go`、`internal/turn/session_info_readback_test.go`。不新建文件，不改 `go.mod`。生产改动估计约 90 行，一次交付。
- 补丁块，每个文件可以有多个：

  ```
  ===PATCH: internal/turn/runner.go===
  <<<<ANCHOR
  （从 HEAD 92dfc89 真文件里原样复制的若干行，含行首 tab 与中文标点，在该文件里恰好出现一次）
  ====
  （替换后的完整文本）
  >>>>
  ===END PATCH===
  ```
  锚点必须从 HEAD `92dfc89` 的真文件逐字复制（本文代码块里的缩进可能被排成空格，以真文件为准），在该文件里恰好出现一次；同一文件的多个锚点互不重叠。注意 `SessionInfo` 的三个返回点里 `ColdExpected:` / `InFlight:` 两行的文本彼此相近，锚点要带上足以区分的上下文（例如带上 `g.st.lastTurnAt` 或 `InFlight:        true,`）。
- 本刀不新建文件；若确需新文件，用 `===FILE: 路径===` 换行接完整内容、末尾 `===END FILE===`，并在报告里说明为什么。不许用 markdown 围栏包代码，不许写「……」或「其余不变」之类的省略。
- 代码必须能 `gofmt`（tab 缩进）。
- **符号禁令**（源码、注释、字符串、标识符一律适用）：不许出现 Emoji U+1F300-1FAFF、Dingbats U+2700-27BF、杂项符号 U+2600-26FF 与 U+2B00-2BFF、带圈字母数字 U+2460-24FF、U+FE0F、U+200D。放行：中文与全角标点（含 U+300C / U+300D 直角引号）、希腊字母、数学算符、普通箭头 U+2190-21FF、Box Drawing。本刀新增的字符串字面量只有 §3.1、§3.2、§3.3、§1.1 C 的 SQL 与错误包装文本。
- 生产代码新标识符不许以 `s10a` 开头（那是测试腿的前缀）。
- §1.1 里给出的签名、判据、代码块照抄；「名字实现方定」处自取。
- 回复末尾必须有：

  ```
  ===REPORT===
  补全性：12 个文件是否都给了；§1.1 A-G 每一条落在哪个补丁块。
  位置：E1 的检查插在哪两行之间（贴出锚点的第一行）。
  歧义与矛盾：契约里你认为有歧义、矛盾或做不到的地方，逐条写出你的取舍；没有就写「无」。
  字面量：§3.1、§3.2、§3.3 是否逐字照抄；有没有新增任何别的用户可见字面量。
  ```

---

## 6. 落地后主代理要做的

1. **收割**：越界扫描（主仓库、`internal/*`、中转根有没有 grok 写出来的新文件；按内容核对，不凭 mtime 归因）；符号禁令扫描；按锚点落盘；`gofmt -l`；`go vet ./internal/conversation ./internal/store ./internal/turn ./internal/repl ./internal/app`；`go build ./...`。逐字核对：E1 在 `g.mu.Lock()` 之后、`if cfg == nil` 之前；§3.1 那句话逐字节；表头逐字节；六个假件每个只多一个方法（`git diff --stat` 每个文件只有新增行）。
2. **合并测试腿的 4 个文件**之后，只跑 `./internal/store ./internal/conversation ./internal/turn ./internal/repl ./internal/app`（另一条流水线也在跑 go test；3 分钟无输出只停自己的 go 进程，改 `go test -c` 再跑二进制）。既有用例必须全绿（原型实测全绿）。
3. **核对测试腿报告里 §2.3 / §2.4 两个文件在 HEAD 上单独跑的结果**与 §2 通用纪律末尾写的原型结果一致（R1-R4 红、E1 `settle_fails` 红 `settle_ok` 绿）。
4. **§4 的变异逐条跑**（#1-#23；从副本还原）；任何一条没被点名杀手杀掉，先判是用例空、还是契约写错，契约错就改契约再改任务书。
5. **文档**：
   - `docs/13_STEP10_CONTRACT.md` §3.8 后追加订正记录（2026-09-27）：检查的具体位置是「取闸之后最早处、任何副作用之前」，理由是 §0 第 2 条的实测（HEAD 上撞唯一索引之前已经发了 `session_opening`、指纹不符时已经删了 settle 要标的那一行 —— 后者只对 spawned / returned 行成立，begun 行的对账只删在飞行、不标 `provider_sessions`，见本契约 §0 第 2 条的 v1.1 订正）；哨兵住在 `conversation`（`ErrInflightConflict`），store 按扩展码 2067 译；那句话不提 `/session rebuild` 的理由。§3.15 第 10 条标「已由 G1 / E1 担保」。
   - `docs/09a_STEP6_API_CONTRACT.md`：登记 `SessionInfo.BaselineSuspect`（13 §4 表第 6 行、§4a 第 6 条要求的，与 `InFlight` 一批）；`askCause` 认得的哨兵多一个。
   - `docs/10a_STEP7_API_CONTRACT.md`：登记 `Store.InflightFor` / `Service.InflightFor` 与哨兵 `ErrInflightConflict`；`Tx.BeginTurn` 的错误链多一个哨兵。
   - `docs/README.md` 路线图 Step 10 那一行：A 之五已落（写 commit）。
   - `docs/00_DECISIONS.md`：把本刀的 open items 记成新条目（`/session rebuild` 解不开、没有 REPL 内的对账入口、`/round` 预算表的「在飞」列与预报不知道这件事、`/session` NEXT 列不报「会被拒」、启动横幅在 settle 失败时仍说「其中 0 轮已标」而 stdout 不提失败）。
   - `.handoff/HANDOFF.md` 收工时记一条。
6. **真机复验**（Griffin，Windows Terminal + PowerShell 5.1；用一个一次性的 `data_dir`，不碰日常库）：
   1. `council` 进 TUI 与行式各一次，`/session`：表头多一列 `BASELINE`，开过口的成员印 `ok`，没开过口的印 `-` 与 `fresh`，列对齐。
   2. 让一个成员的一轮超时或被 Ctrl-C 取消（非首轮），再 `/session`：它那一行印 `suspect`；紧接着 `/ask` 它，金额一行印「金额未计（上一轮结局未知，基线可疑）」。
   3. 按 13 §3.15 第 14 条的探针（子进程还在跑时 `taskkill /F` 杀 council，不加 `/T`）制造一条在飞记录，重启：横幅照常报丢账、Err 没有「启动在飞对账补账失败」；紧接着 `/ask` 那个成员**不被拒**、正常起进程（对账成功即解开）。
   4. 「settle 失败 -> 被拒」这一支真机上很难自然造出来（要让启动对账本身失败），由 E1 在真 `app.Run` + 真 SQLite 上担保，MVP 判据表里标 `[用例]TestS10aSettleFailureBlocksAskUntilSettled`，不标真机。
7. **加强 C2（v1.1；主代理做，不进本刀验收）**：A 之五落地后，按 `docs/13_STEP10_CONTRACT.md` §6.2 的 2026-09-27 订正末尾写的两处，给 `internal/app/s10c2_crash_e2e_test.go`（C2，前缀 `s10c`，与本刀 app 包的新文件共用 `smoke_test.go` 的 `TestMain`）补断言：(i) `/session` 的可疑基线列 —— 对账标了可疑的那个成员，`/session` 那一行 BASELINE 格印 `suspect`；(ii)「对账失败、行留着」时 `/ask` 被拒且不起进程。(ii) 与本刀 E1 `settle_fails` 是同一个场景，写之前先看 E1 已经担保到哪一步，不重复造装置。标识符沿用 `s10c` 前缀，不与 `s10a` 撞名；做完照 §6 第 4 步的纪律对新断言做变异。
8. 清理本刀在中转根与 scratchpad 里的全部中间物。
