# Council 决策记录

一页纸。任何人（包括下一个会话的我）接手前先读这份，不要重新推导。

日期：2026-09-20 · 机器：E 盘机（COMPUTERNAME = LONG）

## D-1 对话由 JSON 协议拥有，不由 PTY 刮屏拥有

**决定**：Council 的运行时建在各 CLI 的 headless / JSON 接口上。

**为什么**：这是单向选择。JSON 运行时之后可以接任何 UI（终端、小网页、ntfy 推送、
甚至把 `council` 注册成 many-ai-cli 的 custom provider 开个面板）；
而 PTY 刮屏拥有的对话只能喂那一个 Hub，因为对话本身就是那个 VT 缓冲区的副产品。
**运行时的层决定 UI 的可能性，UI 不决定运行时。**

**代价，已知并接受**：失去 many-ai-cli 现成的手机推送与审批面板。
审批的实际缺口比想象小 —— 五个角色里只有 Executor 需要写盘权限，
而四家 CLI 都有原生权限档（见 `06_SECURITY_MODEL.md`）。

## D-2 many-ai-cli 是参考材料，不是地基

不 fork、不切 remote、不建 `dev/council`、不在它内部泛化 relay。
它的 `relay.go`（5475 行）是一个硬编码的 Council，读它、抄它的教训，不继承它的架构。

具体抄哪四样，见 `04_CONTEXT_ROUTER.md` 的「从 relay 继承的四个模式」。

## D-3 Agent 不是常驻进程

**Step 0 实测结论**：`--resume` / `--conversation` / `exec resume` 都是**每轮重新起进程**，
CLI 自己的会话文件携带上下文，Council 这边只存 per-agent cursor。

这改写了原计划 Step 6 里 `AgentSession` 的形状：它不是一个 start/bind/cancel 的长活对象，
而是「按 id 恢复、跑一轮、退出」的无状态调用封装。

## D-4 缓存前缀的稳定性是一等约束

**Step 0 实测**（claude，完整数据见 `E:\Isolating_test\council\step0-probe\STEP0-RESULTS.md`）：

| 轮次 | cache_create | cache_read | 本轮增量 |
|---|---|---|---|
| T1 冷启动 | 47,749 | 0 | $0.955 |
| T2 resume | 19,363 | 28,436 | +$0.395 |
| T3 resume | 95 | 47,752 | +$0.014 |

$0.014 是**暖缓存 + 零新上下文**的边际值，不是"每轮成本"。真实每轮落在两者之间。

推论：任何打断前缀稳定性的做法 —— 重建会话切 model/effort、往历史中间插消息、
并发写同一会话 —— 都会把该 agent 打回接近冷启动。这必须是 Context Router 的首要约束。

**另有硬约束**：缓存走 `ephemeral_1h`（1 小时 TTL）。
agent 闲置超过 1 小时，下次说话重付冷启动价。跨午休的讨论要重付全队入场费。

## D-5 每个 agent 的入场费是固定成本

回答一个 "OK" 所需 input：claude 47,749 / grok 20,625 / agy 18,180。
按构成 18k–48k 一人，5 人团队 90k–240k token，**与它们说多少话无关**。

其中有一部分是用户的全局 `~/.claude/CLAUDE.md`（grok 的 `thought` 字段证实它读到了那些规则）。
那份文件是为主代理写的，对一个力学专家 agent 全是噪声。
**Council 成员必须跑在干净的指令环境里** —— 这是设计要求，不是优化项。

## D-6 三家的会话 id 语义不同，编排要适配

- **grok**：`--session-id <uuid>` 可**预先指定**（必须合法 UUID 且不得已存在），spawn 前就能决定 id
- **claude**：`--resume <session_id>`，id 只能从首轮输出的 `session_id` 取回
- **agy**：`--conversation <conversation_id>`，同样只能从首轮输出取回

Provider 适配器必须声明自己属于哪一类（见 `02` 的 `SessionIDPolicy`）。

## D-7 执行者按会话核对，不写死

**决定**：每次会话开始先核对主代理的型号与强度，再据此定委派链；不沿用上一次的结论。

- 主代理在 **Claude Fable 系列**上 → 全局 `CLAUDE.md` 的代码委派链生效：
  首选 grok，回退 Opus 5 子代理，Fable 只做规格 / 审查 / 落盘 / 编译测试 / 验收。
- 主代理在**其它型号**上 → 委派链不生效，主代理直接写码；
  但全局规则的 advisor 逐命令征询反过来对这些型号生效
  （只有 Fable 5.1 在 max / xhigh 下豁免逐命令）。

核对方式：`mcp__ccd_session_mgmt__get_session("self")` 的 `model` 与 `effort` 字段，不靠自述。

| 会话 | 日期 | 型号 / 强度 | 代码委派链 | advisor |
|---|---|---|---|---|
| #2 | 2026-09-20 | `claude-opus-5` / xhigh | **不生效**，主代理直接写码 | **逐个状态变更命令前征询** |

第二条对 Step 4 起的实现节奏有实际影响：每次写盘 / 改配置 / 跑命令前都要先征询。
若改用 Workflow 子代理承载实现，子代理继承主代理型号（同样非 Fable，可直接写码）。

## D-8 实现语言是 Go

**决定**：Step 4 起用 Go 实现 council。

**为什么**：原计划选 Go 的理由（复用 many-ai-cli）已被 D-2 取消，所以是重新选的，不是惯性。
重选后仍是 Go：`02` / `04` / `05` 三份设计的接口伪码本就是 Go，落地不用翻译；
`exec.CommandContext` + `context` 取消 + goroutine 正好对应 D-3 的形状
（每轮起进程、有超时、有取消、同一会话严格 FIFO 门控）；单静态二进制、无运行时依赖；
本机 go1.27.0 刚在 many-ai-cli 上验证过 `go build` / `go vet` / `go test` 全通。

**已知代价**：
- go.exe 不在 PATH，一律全路径 `C:\Program Files\Go\bin\go.exe`
- SQLite 用纯 Go 驱动（`modernc.org/sqlite`）避开 cgo，不用 `mattn/go-sqlite3`。
  **这条的代价在 Step 5 量出来了**：依赖从 1 个（BurntSushi/toml）变成 11 个，
  二进制从约 3 MB 涨到 10 MB。纯 Go 驱动把整个 SQLite 转译进了 Go，这是固有成本。
  仍然维持这个选择，因为换 cgo 要装 gcc、开 `CGO_ENABLED=1`、交叉编译变麻烦，
  与「单静态二进制、无运行时依赖」这条本来的理由直接冲突
- 模块路径暂用本地名 `council`；将来定了 remote 再 `go mod edit -module` 改一次
- `go.mod` 的 `go` 指令写的是 **1.25.0**，本机 toolchain 是 **1.27.0**。两者不是一回事：
  前者是语言版本下限，写低一点让另一台机器上较旧的 Go 也能编；后者是实际编译器。
  看到这两个数字不一致不要"修"它

## D-9 配置在 C，数据在 E

**决定**：

| 内容 | 位置 |
|---|---|
| 配置文件 | `C:\Users\25142\.council\config.toml`（即 `~/.council/`，与 `.claude` / `.codex` 同惯例） |
| SQLite 库、日志、会话导出 | `E:\council-data\`（固定，不随用户目录走） |

**为什么分开**：配置是手改的小文件，跟其它 CLI 放一起才找得到；
对话记录会长期增长（`05` §5 的六张表），不占系统盘。

两者都可被配置覆盖：`config.toml` 里 `data_dir` 优先，其次环境变量 `COUNCIL_DATA_DIR`，
最后才是上表的默认值。配置文件本身的位置用 `COUNCIL_CONFIG` 覆盖。

## D-10 SQLite 的打开方式是实测定的，不是抄来的

**决定**（完整证据见 `reference/sqlite-probe-results.md`，四轮探针共 1280 次并发写）：

```
写句柄 DSN: file:<绝对路径>?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)
            &_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate
写句柄:     SetMaxOpenConns(1)
读句柄 DSN: 同上但不带 _txlock
读句柄:     SetMaxOpenConns(4)
```

**三条都不能省，各自挡一个坑**：

1. **pragma 必须写进 DSN**，不能用 `db.Exec("PRAGMA ...")`。
   后者只作用于连接池里当时那一条连接，`database/sql` 新建的连接没有。
   实测：320 次并发写只成功 37 次，283 次 `database is locked`。
2. **`_txlock=immediate` 必须有**。`BeginTx` 默认发 `BEGIN DEFERRED`，
   事务先以读身份开始、要写时才升级；两个都持有读快照的事务同时升级会直接
   `SQLITE_BUSY` —— **那是死锁不是排队，`busy_timeout` 等多久都没用**。
   实测：加上之后 320/320 全过，busy 归零。
3. **WAL 必须真的开起来**。只做串行化不开 WAL 的那组虽然也 320/320，但慢 **178 倍**
   （2493 ms vs 14 ms），因为退回了 `delete` 模式，每次提交重写回滚日志。

**读写分两个句柄**：WAL 下读拿快照、不被写阻塞。实测写满负荷跑的同时
4 个读 goroutine 转了 3787 次、0 失败。让读跟着写一起串行是白白浪费。

**`seq = MAX(seq)+1` 在这套配置下安全**：1280 次并发写没有一次撞号，seq 连续无空洞。
再加 `UNIQUE (conv_id, seq)` 作为结构性兜底。

**边界，必须写清楚**：以上全部只覆盖**单进程内的多 goroutine**。
Council 的设计前提就是**单写进程** —— 同一个 `data_dir` 上跑第二个 council 实例
不在支持范围内。

**订正（2026-09-21，实测推翻了本节原来的一句话）。** 这里原本写着
「`BEGIN IMMEDIATE` 加唯一约束意味着第二个进程会**响亮地失败**而不是悄悄写坏数据」。
**那句是错的，而且错在最危险的方向上。** 实测（未决 #8，原件
`E:\Isolating_test\council\oq8-multiproc\RESULTS.md`）：第二个进程**静默打开**、
静默写入、退出码 0；两个进程对同一个 (会话, 成员) 各跑一轮之后，
付了两次入场费、留下一条谁也找不到的孤儿 provider 会话、游标错位，
而两边的 `/session` 各自显示一个互相矛盾的「正确」答案。

**这四条损坏全都发生在没有任何 SQLite 冲突的前提下** —— 也就是说，
互斥本来就不在 SQLite 这一层，`BEGIN IMMEDIATE` 与唯一约束从头到尾都管不着它。
真要支持多实例得加 `data_dir` 级的 OS 文件锁，处置写在未决问题 #8。

## D-11 不做 TUI —— **已被推翻**（2026-09-22，用户裁定）

**原决定**（`13_STEP10_CONTRACT.md` 7.4）：不做 bubbletea 一类全屏 TUI。
理由是 D-1 加「D-8 量出来的依赖代价（1 个变 11 个、二进制 3 MB 变 10 MB）」。
推翻条件写的是「真的需要同时盯五个成员的**流式**输出，
或需要在一屏里做成员间的并排比对」，并写明「要在 `/round` 真跑起来之后复看一次」。

**现在：`/round` 已经跑起来了（Step 9 片 3），复看到期。用户 2026-09-22 裁定：做。**

### 推翻时发现原理由里有一笔重复计费

「依赖 1 个变 11 个、二进制 3 MB 变 10 MB」那笔账是 **D-8 里 sqlite 纯 Go 驱动的代价**，
**Step 5 就已经付过了**。现查 2026-09-22：`go.mod` 是 3 个直接依赖 + 8 个间接，
二进制 12.2 MB。加 TUI 框架是在这个基线上的**边际成本**，不是从 3 MB 起跳。

**拿一笔已付的账去反对一件没做的事，是这条决定的一处实际缺陷。**
记在这里，因为下一个人会重读那条理由。D-1（运行时的层决定 UI 的可能性）仍然成立，
但它支持的是「不在 PTY 层建运行时」，不是「不能给自己的 REPL 做界面」。

### 推翻同时要写明的一件事：TUI 解决不了静默

用户要的是「像 claude code 那样对话」。而那个手感的主体是**流式输出**，
不是输入框。**Council 今天根本不会流式**：`claude.go:91` 与 `grok.go:97`
都是 `--output-format json` 一次性调用，进程退出后整块解析。
套上 TUI 之后，那 11 秒仍然是先静默、再哗地全出来。

**所以拆成两件，不要合成一件期待**：

1. **TUI（本次裁定）** —— 输入框、并发输出不互相撞坏、
   **每个成员一行活的计时器**。光这一件就能解决「我不知道它有没有反应」。
2. **流式输出（另立，未裁）** —— 要改 `Spawner` 的返回形态、
   `ParseTurn` 的一次性契约、`Runner` 的结局判定。**一个 Step 的量级。**

### 参考实现

`xai-org/grok-build` 是 SpaceXAI 的全屏 TUI，Apache 2.0，**但是 Rust 写的**，
而且不收外部 PR。**代码借不了（语言不同），只能借设计。**
Go 这边对应的是 bubbletea 系。

### 这条推翻会废掉什么

- `13` 7.4 的「代替方案：都在现有 REPL 皮肤里」作废。
- `internal/repl` 今天整个建在 `io.Writer` 上（`r.printf` 直写 stdout），
  TUI 要接管屏幕，这一层要重构。
- `EnablePaste` / `splitPasteMarkers`（B 片）会被 TUI 框架自带的粘贴处理顶掉。
  **那一片不算白做**：它量出了本机终端的真实行为，但代码会被取代。
  （2026-09-23 补：`EnablePaste` 已由 X-39b 删掉，原因不是被 TUI 顶掉，而是它在一行模式里毁了方向键与退格，见 #39(b)；
  `splitPasteMarkers` 仍在一行模式的读路径上，只剥终端自己发来的标记。）
- 正在做的进度反馈那一刀（`RoundProgress` 回调）**不白做，恰好是 TUI 需要的那个钩子**。

---

## 明确不做的事

- 不做 PTY 包装、不做 VT 模拟器、不做 idle 启发式 —— turn 边界在 JSON 里是显式的
- 不做通用 Provider Registry 去改 many-ai-cli 的 46 个 Go 决策点
- 不重写任何 CLI 的登录机制，不读取、不复制、不存储任何 provider 凭据
- MVP 不做 embedding / 向量检索的上下文路由，用确定性规则

## 未决问题

1. ~~**同一 provider 开多个 session 时缓存前缀是否共享**~~ —— **已测，共享**（2026-09-21，
   E 盘机，grok 经 CPA）。两次 council 进程、两条互不相干的会话、两个不同的
   provider session id，送的身份块与首条正文逐字节相同：第一次 `cache_read = 0`
   （`in = 19619`），几分钟后第二次 `cache_read = 19584`（`in` 仍是 19619）。
   唯一共享的东西只有前缀内容本身，所以**缓存按内容命中，不按会话**。
   结论：「同 provider 多 agent」的入场费确实能摊薄，这条不再是成本模型里的未知数。
   只在 grok 上测过，claude 与 agy `[未验证]`。

   **连带发现一条反直觉的**：`Record.ColdExpected`（按「新会话，或距 `LastTurnAt`
   超过 `CacheTTL`」推算）**不能预测** `Record.Cold`（实测 `cache_read == 0`）。
   上面那次第二轮就是「预报=冷、实测=暖」：会话是全新的，前缀却是热的。
   两者本来就在量不同的东西 —— `ColdExpected` 量的是我们这条会话的年龄，
   `Cold` 量的是 provider 那边这段前缀的年龄，而后者是跨会话共享的。
   规格 G 节把 `ColdExpected` 定义成「下一轮的价格预报」，这个定位没错，
   但它**偏保守**：会把很多实际上暖的轮次预报成冷。
   只有 `UnexpectedCold`（预报暖、实测冷）方向的意外要告警；反方向便宜了，不是问题。

   **第二次观测（2026-09-21，换了一家 provider）**：第二片的 agy WORKSPACE 真机验收
   （`E:\Isolating_test\council\step8b-agy-rerun`）是一条**全新的 provider 会话**，
   屏幕打的是 `预报=冷 实测=暖`，`cache_read=12216`、`in=21482`。
   同日直接调 agy CLI 的探针（`oq-agy-denied-count`）却是 `cache_read_tokens: 0`（冷）。
   两者的差别在于 Council 送的是完整身份块 + 规则 + delta，探针只送一句裸指令 ——
   所以更可能是**提供商侧按内容命中了别的东西**，不是 Council 这边的会话复用。
   不在这里下结论，只记两个数字：这条「预报不了实测」的性质现在在
   **grok 与 agy 两家**上各观测到一次，不再是单点。
   `11d` 第 3.2 节据此要求 `/round` 的计划行只印「预报」、不印美元数。
2. ~~**`thought` 字段要不要进 Canonical Conversation**~~ —— **已关闭**（2026-09-21）。
   这条原文只是一句指针「见 `04` 的专节」，而 `04` §3 早就表过态，
   Step 5 / 6 / 7 已经把它逐条落成代码，还有用例守着：

   - **存**：`messages` 表有 thought 列（`store/schema.go:70`，形状声明在 `:140`），
     `AppendMessage` 无条件写（`store/tx.go:119-121`），
     Runner 把解析结果原样带进落库的消息（`turn/runner.go:421`）。
   - **不路由**：`RenderDelta` 不写（`turn/render.go:49-51`）。
   - **开关位置已留好**：`NewRouter` 收 `routeThought`（`turn/router.go:59`），
     生产路径恒传 false（`turn/runner.go:289`）。

   所以答案是「**存，且默认不路由**」，出处 `04_CONTEXT_ROUTER.md:75-77`。

   **两处措辞要说准**（对抗复核订正的，记下来免得下次又说岔）：
   `router.go:156-159` 恰恰是**会**写 thought 的那三行
   （`if r.routeThought && m.Thought != ""`），它不写只因为生产恒传 false ——
   把一个条件分支说成无条件事实，正是本项目最该避免的那种复述。

   **留下一个覆盖缺口，不阻塞关闭，并进 #9 一起补**：今天**没有任何一条用例**
   断言生产路径传的是 false。把 `runner.go:289` 改成 `NewRouter(true)`，
   全套用例照样绿（这是静态推断，不是实测），而 thought 会真的进别人的 prompt。
   补法：用已有的假 Spawner 拿到 `provider.Command`，断言任何一条消息的 Thought
   文本不出现在 argv 任一元素、也不出现在 stdin 里。**判据是送出去的字节。**

   **不归本条的残余**：`04` §3 还说「要让某个 role 看到，必须在 Team 配置里显式打开」。
   那个配置面今天不存在（Teams 没接进会话），归 #9 的路由那一半 + Step 9。
3. ~~谁执行后续实现步骤~~ —— **已决**，见 D-7（按会话核对型号，不写死）。
4. ~~实现语言~~ —— **已决**，见 D-8（Go）。
5. ~~配置与数据目录放哪~~ —— **已决**，见 D-9（配置在 C，数据在 E）。
6. ~~Go 模块代理是否可达~~ —— **已测，可达**（2026-09-20，E 盘机）。
   `proxy.golang.org` 1 秒返回版本列表，`goproxy.cn` 2 秒，默认 GOPROXY 不用改。
   `github.com/BurntSushi/toml v1.6.0` 已拉下并进 go.sum。
7. ~~**`02` 里 judge 的定义有两说**~~ —— **已裁并落地**（`44ebeb1` 文档侧、`f7640c6` 之后补 `router.go` 注释）：**留 `Mode`，删掉 `02` 里「Judge 就是
   `Role == "judge"` 的普通 Agent」那半句。判别器是 `Mode`，`Role` 只注入给模型看。**

   证据（经对抗复核逐条核过）：`Role` 在 `internal/turn` 之外**零处被读**
   （`config/schema.go:63` 字段声明、`config/defaults.go:17` 与 `:25` 两处赋值，
   值是整句中文不是枚举串）；turn 包内唯一读点是 `render.go:108`，
   原样拼进身份文本；`identityFPFor`（`session.go:101-104`）全程不读 `ac.Mode`。
   也就是说**今天没有任何一处按 `Role` 分支**，而 `Role` 进身份块、进指纹，
   改它会触发 `ErrIdentityDrift`（`runner.go:228`）—— 拿它当判别器等于让
   「换个裁判」变成一次会话重建。

   `Mode` 反过来：全仓非测试读点三处（`validate.go:63-64` 枚举校验、
   `load.go:66-69` 缺省与小写规范化、`commands.go:375` 打印），
   **三处都不是按 `Mode` 分支的行为**，在 `internal/turn` / `internal/provider` /
   `internal/app` / `cmd` 四个目录零命中。所以留它零代价。

   **落地动作**（零行为改动、零用例变红）：
   (a) 改 `docs/02_AGENT_PROVIDER_ARCHITECTURE.md:100` 删那半句；
   (b) 改 `docs/04_CONTEXT_ROUTER.md:77` 与 `internal/turn/router.go:45-47`
       两处同源的陈旧措辞（role -> mode）；
   (c) `config.Mode` 枚举**一个字不动**（四个测试钉子在
       `load_test.go:224`、`validate_test.go:35`、`repl_test.go:61`、
       `defaults_test.go:106-107`），`07a` 冻结的五值不变；
   (d) `validate.go` 不加新校验。
   `manager` 与 judge 同型，一并按 `Mode` 处置。

   **下面是原文，留作背景。**

   原文：§2 把 `judge` 同时放进 `RuntimeMode` 枚举和
   「Judge 就是 `Role == "judge"` 的普通 Agent」这句话里。
   Step 4 对 `role` 不做任何校验，所以 `mode=judge` 配任意 role、或 `role=judge` 配
   `mode=discussion`，眼下都合法。Step 9 做 Judge 之前必须先把 `02` 改成一说。
8. ~~**同一个 `data_dir` 上的第二个 council 进程**~~ —— **已落地**（`b0f1938`）。事实已实测，锁已加，真机复验过：同一个两进程探针，之前静默写坏，现在第二个进程退出码 1 并点名锁文件；A 退出后第三个进程正常开。原件
   `E:\Isolating_test\council\oq8-multiproc\RESULTS.md`，零成本（只用 grok，经 CPA）。

   **D-10 里那句「第二个进程会响亮地失败而不是悄悄写坏数据」是错的。**
   实测：第二个进程**静默打开**同一个库、看得见第一个建的会话、自己也照常写入，
   退出码 0，两边都没有任何提示。

   **两个进程对同一个 (会话, 成员) 各跑一轮之后的实际损坏**（逐条都在库里查过）：

   - 屏幕上两边各自「新开了一条 provider 会话」，`/session` 各自显示自己那条，
     **互相矛盾而从任一侧看都正常**；
   - `provider_sessions` 只剩**一行**（后写的那个），`cursor_seq = 3`；
   - `turn_usage` 有**两行**，两条不同的 provider 会话 —— **付了两次入场费**；
   - 先写的那条 provider 会话**成了孤儿**：它在 grok 那边真实存在、钱花了，
     本地再没有任何一行指向它，没有任何命令列得出来；
   - 游标停在 3 而消息已到 seq 6，下次恢复会把 4-6 重投一遍，
     正好是 D-4 最怕的那种前缀打断。

   **这四条全都发生在没有任何 SQLite 冲突的前提下**，所以「SQLite 层会不会报
   `SQLITE_BUSY`」这条已经不重要了 —— 互斥本来就不在那一层。

   **处置：在 `data_dir` 上加一把 OS 排他文件锁。** 锁 `<data_dir>/council.lock`，
   **不锁 `council.db`**（测试里有多处在 `Open` 之前或期间用自己的连接开同一个库文件，
   锁库文件会误伤它们）。取锁点在 `store.Open` 的 `MkdirAll` 之后、拼 DSN 之前；
   取锁之后的 **7 条** early-return 路径都要放锁
   （`open.go:41`、`63`、`69`、`81`、`87`、`94`/`96`、`107`/`109`；
   `MkdirAll` 那条早退在取锁之前，不用放）。新增哨兵 `store.ErrDataDirLocked`。

   **否决的两个候选**：pid 文件 —— 本项目把 `taskkill /T /F` 当常规路径
   （`killtree_windows.go`），异常终止会留陈旧 pid；`PRAGMA locking_mode=EXCLUSIVE` ——
   它覆盖不到 `<data_dir>/workdir/<agentID>/council-role.txt` 与 provider 会话，
   而上面四条损坏里有三条不在 SQLite 层。

   **落地时必须连带改的两处措辞**：本条上方 D-10 的「响亮地失败」那句，
   与 `reference/sqlite-probe-results.md` 里对应的一句。

   **仍 `[未验证]`**：异常终止（`taskkill`）之后锁能否即时释放。
   验收判据要写成「taskkill 之后重试起 B，记录重试次数与间隔」，
   不要写成一次必过。

   **已落地**（commit 待填）。Windows 用 `golang.org/x/sys/windows` 的
   `LockFileEx`（`LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY`，立即失败不阻塞），
   非 Windows 用 `flock`（`LOCK_EX | LOCK_NB`），按 `killtree_windows.go` /
   `killtree_other.go` 的先例拆成 `lock_windows.go` / `lock_other.go` 两个 build tag 文件；
   非 Windows 那一支只过了 `go vet` 与交叉编译类型检查，没有真机跑过。

   **取锁点与放锁方式，与裁决原文有一处刻意的偏离，记在这里而不是埋在代码注释里**：
   裁决原文列了 7 条 early-return 行号、暗示要逐条手写 `release()`。实现改用
   `defer` 加一个 `lockOwned` 旗标：早退在"构造出 `Store` 之前"的那 5 条
   （`filepath.Abs`、两次 `sql.Open`、两次 `PingContext`）统一靠这个 `defer` 放锁；
   剩下 2 条（`migrate`、`validateSchema` 失败）发生在 `lockOwned` 已经置 false、
   锁的归属转交给 `Store` 之后，靠这两条分支里本来就有的 `s.Close()` 放锁。
   两类相加仍是 7 条，行为等价，但不需要走一遍全部 early return 去数行号补
   `release()`——好处是以后谁在 `Open` 里加一条新的 early return，只要还在
   "构造出 `Store` 之前"，会自动被同一个 `defer` 兜住，不会因为没人记得再补一行
   而漏放。`Store.Close` 同时关写句柄、读句柄、放锁，三者任一失败都作为错误返回。

   **`[未验证]` 已转正**：E 盘机（`$env:COMPUTERNAME` = `LONG`）真机测过两组。
   第一组验证释放时机：`Start-Process` 起一个持锁进程、确认它 `OPEN_OK`、
   `taskkill /T /F` 杀掉整棵树、立刻循环尝试重新取锁——5/5 次都在第一次尝试就
   成功（40 多毫秒，基本是新进程自身启动开销，不是等锁）。第二组验证排他性
   本身：持锁进程还活着时，另起一个**真实的第二个进程**（不是同进程内连续两次
   `Open`）去 `Open` 同一个 `data_dir`，立刻拿到 `errors.Is(err, ErrDataDirLocked)
   == true`，不阻塞、不重试——这才是未决 #8 原始损坏场景（两个真实进程）的直接
   对照组。原件 `E:\Isolating_test\council\oq8-lock\RESULTS.md`。样本量小、
   只在一台机器上连续测过，按「已验证一次、未穷尽」记。

   **顺带一句关于 go.mod / go.sum**：这两个文件不在裁决点名要改的范围内，
   但引入 `golang.org/x/sys/windows` 这个直接 import 之后跑了一次 `go mod tidy`，
   连带把本来就该是直接依赖、却一直被标成 `// indirect` 的 `modernc.org/sqlite`
   也归位了（`open.go` 一直在直接 `import _ "modernc.org/sqlite"`，这处不一致是
   历史遗留，不是这次改动引入的）；`go.sum` 因为 tidy 会走一遍"全平台"的模块图，
   多出约 30 行别的模块的校验和条目，这些条目都不是这次改动实际拉进来的代码，
   只是 tidy 为了让跨平台构建可复现而记录的图节点。

   **下面是原文，留作背景。**

   原文：D-10 的实测只覆盖单进程。
   真要支持多实例，得加文件锁（或 `PRAGMA locking_mode`）并测跨进程行为。
   Step 10 做「恢复」时会碰到这个假设，别到那时才发现。
9. **已裁；(a) 已落地，(b) 仍待 Step 9**（2026-09-21 裁，(a) 落地 commit 待填）：
   **拆成两半，分开处置。**
   (a) **存储开关现在做**，不等 Step 8 的任何一片；
   (b) **路由开关随 Team 配置归 Step 9**，不是 Step 8 第三片。

   **(a) 已落地**：`Config` 加根级 `StoreThought bool`（`toml:"store_thought"`），
   紧跟 `RulesFile` 之后，默认 true；`validate.go` 不校验，与 `RulesFile`
   同待遇。默认值那个坑按实施要点第 1 条选的是「预置再 Decode」这条路：
   `config/load.go` 的 `Load` 在 `toml.Decode` 之前把 `cfg` 预置成
   `Config{StoreThought: true}`（不是改 `*bool` + `md.IsDefined`），
   `config/defaults.go` 的 `Default()` 同步补了 `StoreThought: true`。
   钉住默认值的用例是 `config/load_test.go` 的
   `TestLoadDefaultsStoreThoughtToTrue`（不写这个键必须读出 true）与
   `TestLoadStoreThoughtExplicitFalse`（显式写 false 必须原样读出，
   证明预置值真的会被 Decode 覆盖，不是恰好没被碰过）。

   契约面那条按实施要点第 2 条选的是**加 setter**，不改
   `NewService(s Store) *Service` 的冻结签名：新增
   `Service.SetThoughtPolicy(store bool)`，仿 `turn.Runner.SetRules` 的
   形状（构造之后再配置），只在 `internal/app/app.go` 装配阶段按
   `cfg.StoreThought` 调一次。已在 `08a_STEP5_API_CONTRACT.md` 补一条
   订正登记：新方法本身，以及它让 `Post` / `RecordTurn` 签名不变但行为
   随策略改变这一点，都点名写明，不算零契约改动。

   落点按实施要点第 3 条放在 `conversation.Service`，不放 `turn.Runner`：
   `Post` 与 `RecordTurn` 各调一次无副作用的 `applyThoughtPolicy(m Message)
   Message`（返回处理后的副本，Meta 是 map，写新键前先克隆，不改调用方
   持有的原始结构体——`RecordTurn` 收 `*Message`，这一条尤其要紧）。
   `Service` 上的字段是 `dropThought bool` 而不是 `storeThought bool`：
   零值必须等于「存」，否则全仓三十多处不经过 `SetThoughtPolicy` 的
   `NewService(...)` 调用点（几乎全是测试）会在零值下静默开始丢 thought，
   与 `Config.StoreThought` 那个坑是同一种病、长在另一层，`08a` 的订正
   与 `service.go` 的注释都点名了这条极性选择。

   关掉时按实施要点第 4 条不静默丢：`conversation.MetaThoughtDropped`
   （`"thought_dropped"`）只在 `m.Thought` 真的非空且策略要求丢时才写进
   `Meta`；`Thought` 本来就是空串（这家 provider 没有这个概念）时不写这个
   键，避免把「本来没有」误报成「我们丢了」（未决 #11 同一个裁法）。

   实施要点第 5 条命中，按它处理了：`config/load_test.go` 的
   `TestLoadValidFile` 用 `reflect.DeepEqual` 比对整个 `Config`，
   不补 `StoreThought: true` 就会因为一个 bool 字段不等而必然报红——
   这条是确定性的（`DeepEqual` 对结构体字段级比较不等即为不等），
   没有必要、也没有专门跑一次「不补会红」的对照来"观测"一个数学上
   已经确定的结果。补上之后这条用例照常通过。其余全仓 `NewService(...)`
   调用点（三十多处，几乎全是测试）都没调 `SetThoughtPolicy`，零值即
   「存」，行为与改动之前逐字节一致，这条是实测过的（全套 `go test ./...`
   在改动前后都是全绿）。

   端到端验收判据按任务书原话做了两个方向，用同一份 `grokT1JSON`（真实
   带 thought 的原件），不是靠「这轮本来就没 thought」的巧合：
   `internal/turn/thought_policy_test.go` 的
   `TestAskStoresThoughtByDefault`（默认策略，读回库的 Thought 非空、
   Meta 无 `thought_dropped`）与 `TestAskDropsThoughtWhenPolicyOff`
   （关掉策略，读回库的 Thought 为空串、`Meta[thought_dropped] == true`）。
   两条判据都是 `svc.History` 读回来的那一行，不是 `Ask` 返回的 `Record`。
   `internal/conversation/thought_policy_test.go` 另有一组更细的用例，
   覆盖 `Post`（默认存 / 关掉丢 / 本来就空不误报 / 不污染调用方 Meta）与
   `RecordTurn`（同样的策略应用，外加「不改调用方持有的 `*Message`」）。

   **顺带补上未决 #2 留下的覆盖缺口**：
   `internal/turn/thought_policy_test.go` 的
   `TestRouterProductionPathDoesNotLeakThoughtIntoOutgoingBytes`
   用真实的 `spawnProcess` 命令行断言 `runner.go` 生产路径的
   `NewRouter(false)` 确实生效——判据是 zulu（grok）先说一轮带 thought
   的话，第二个成员（表驱动分别覆盖 stdin 通路的 claude 与 argv 通路的
   agy）再问一轮时，那条真正要起进程的 `Command` 的 `Args` / `Stdin` /
   `Files` 里都不出现 thought 独有的那段文本。反向对照做过、不是纸面
   论证：把 `runner.go:320` 临时从 `NewRouter(false)` 改成 `NewRouter(true)`
   跑这条用例，两个子用例都如期变红（`FAIL` 行分别点名 "alice 的 stdin
   里出现了 thought 文本" 与 "nova 的 argv[1] 里出现了 thought 文本"，
   后者把泄漏的整段身份块 + thought 原样打了出来），改回来再跑，
   `go test -count=1 ./...` 全绿。

   **仍是剩余项，不因这次落地而消失**（原样保留裁决里第 6、8 两条）：
   开关只影响此后写入的行，已存的 thought 不回溯（不洗历史，`05` §6 的
   保留期清理是另一件事）；`thought_dropped` 今天仍然没有任何读者
   （`/log` 不打印 thought），所以「不许静默丢」目前只兑现了机器可读那
   一半。

   **第三条剩余项，实现方自己标出来的**：`internal/app/app.go` 里那行
   `svc.SetThoughtPolicy(cfg.StoreThought)` **删掉之后全套 `go test ./...`
   仍然全绿** —— 两个方向的用例都直接调 `SetThoughtPolicy`，绕过了装配层那一行。
   这与本条刚刚补掉的 #2 缺口**是同一类问题**：一行接线，没有任何用例
   断言它真的被接上了。

   眼下唯一会经过它的验证是真机那一轮（关掉开关跑一次、读库核对）。
   **归一类处置**：`internal/app` 里这种「配置到运行时的接线」将来会越来越多
   （已有的 `rules` 也是同一形状），该有一条通用的装配层用例断言
   「配置里每一项都真的到达了它该到的地方」，而不是每加一行补一条。
   记在这里，别当成漏测。

   **(a) 为什么现在可以做**：原文捆绑的唯一理由是「免得现在为一个开关去解冻
   `07a` 的配置 schema」。这个理由已经被 Step 8 第一片消耗掉了 ——
   `08b9e5f` 往 `Config` 加了根级 `rules_file` 键（`config/schema.go:43-46`），
   而 `11a` 开头就立了「改本文件要留记录」的程序并当场示范过一次订正。
   根级键这条路已经开过一次，第二次不是解冻，是照同一条程序再走一遍。

   **(b) 为什么归 Step 9**：开关长在 Team 上，而 Team 进会话是 Step 9
   （首要出处 `10a_STEP7_API_CONTRACT.md`「`RouteThought bool` 作为 Router
   实现的构造参数，Step 7 恒传 false，Step 9 接 Team 配置」那段；
   `router.go:46-48` 与 `04:77` 是佐证）。`11_STEP8_SPEC.md` 把它派给
   @mention 那一片，**那一行要改**，别让第三片按错的前提排活——

   **已改**：当初这里点的行号是 449-450，落地时核实那两行其实是
   `/rules` 余量探针（`RulesReport` 按 `(provider, tier)` 分组那条），
   本文档写这两行时的行号已经漂移；thought 路由那句实际落在 478-479，
   现已改成指向 Step 9、说明与 @mention 无关（见 `11_STEP8_SPEC.md`
   「不在本步做」小节）。

   **两半能拆，因为影响面不相交**：thought 从来没进过 prompt，
   所以存储开关对缓存前缀、入场费零影响，与 D-4 无冲突，
   也不触发身份指纹那一摊（#18）。

   **(a) 的实施要点**（都是对抗复核挑出来的，照做，别踩）：

   1. **`bool` 默认 true 在这份代码里拿不到，而且失败方向是静默的。**
      `config/load.go:20` 是 `cfg := Config{}` 后 `toml.Decode`，
      `applyDefaults` 够不着根级 bool。一个裸 `StoreThought bool` 在用户省略该键时
      读出 `false` = **不存**，正好是 `04` §3 默认值的反面，且无声。
      要真拿到默认 true，必须在 `load.go:20` 预置 `Config{StoreThought: true}`
      再 Decode，并同步 `defaults.go` 的 `Default()`；或改 `*bool` + `md.IsDefined`。
   2. **这会动一个冻结签名，必须登记**：`08a_STEP5_API_CONTRACT.md:159` 冻结了
      `func NewService(s Store) *Service`。要么改它并在 `08a` 留订正记录，
      要么改用 `Service.SetThoughtPolicy(...)` 这种仿 `Runner.SetRules` 的 setter ——
      **后者同样是契约面新增，同样要登记**。别当成零契约改动。
   3. **落点放 `conversation.Service`，不放 Runner**：照 #11 已裁过的同一条思路
      （`ErrMissingSenderBrain` 在 Service 边界拦，不靠列的可空性事后追究）。
      `Post` 与 `RecordTurn` 两处各应用一次 —— 这两处的校验本来就逐条重复。
      只改 `runner.go:421` 的话，将来第二个写 agent 消息的调用方会静默绕开。
   4. **关掉时不许静默丢**：thought 列照写空串（不动 schema、不迁移），
      同时往那条消息的 `meta` 写 `thought_dropped: true`，
      **只在真的丢掉了非空 thought 时写**。理由直接引 #11 的裁法：
      空串不许同时表示「这家没有 thought 这个概念」与「我们按用户要求丢了」。
   5. **既有用例会红一处**：`config/load_test.go:70-93` 的 want 字面量要补
      `StoreThought: true`，否则 `reflect.DeepEqual` 当场红。
   6. **不回溯**：开关只影响此后写入的行，已存的不动（事实源只追加）。
      洗历史是 `05` §6 的保留期清理，本条不捎带，记成剩余项。
   7. **验收判据必须是行为**：关掉跑一轮 grok，直接读库核对那一行 thought 为空
      且 meta 里有 `thought_dropped`；**再把开关打开跑一轮，同一位置必须非空**。
      缺了后半，一次「这轮 grok 本来就没 thought」的巧合会把验收骗过去。
   8. **`thought_dropped` 今天没有任何读者**（`/log` 不打印 thought），
      所以「不许静默丢」在导出 / 审计命令做出来之前只兑现了机器可读那一半，
      一并记进剩余项。

   **紧迫性靠承诺与信息面，不靠外泄叙事**：`05` §6 早就承诺过这个开关，
   而 `06` §4 的信息面问题在 grok 上无法从源头切断（它切不断继承），
   于是用户全局规则的正文会经 grok 的 thought 落进本地库。这是事实，
   但本机 `E:\council-data` 不存在，只在隔离根的冒烟库里，与 #18 一样是流程账。

   **顺带补上 #2 留下的那条用例**（同一片代码）：断言生产路径的 `routeThought`
   确实是 false，判据是送出去的字节。

   **下面是原文，留作背景。**

   原文：`05` §6 说「若用户要求不存，提供开关」。
   Step 5 **无条件存**，开关推迟到 Step 8：那时 Team 配置要加 `04` §3 说的
   「让某个 role 看到 thought」的路由开关，两个开关一起加才合理，
   免得现在为一个开关去解冻 `07a` 的配置 schema。

10. ~~**`migrate` 不探测 schema 漂移**~~ —— **已修**。每一版迁移自带「这一版跑完后
    每张表应当长什么样」（`migration.shape`，与产生它的 DDL 放在一起，不另起全局表），
    `Open` 在迁移成功后逐表比对 `pragma_table_info` 的 name / type / notnull 与顺序，
    不一致返回包了 `store.ErrSchemaMismatch` 的错误，信息里点名哪张表、缺了什么、怎么处置。
    端到端用例 `TestOpenRejectsStaleSchema` 造一个旧的十一列 `messages` 库验它真的拦得住；
    `TestSchemaShapeCoversEveryTable` 堵住「加了表却忘了声明形状」这个二阶漏洞。
    **仍未解决的部分**：三级 `data_dir` 核实只在 E 盘机做过，D 盘机 `[未验证]`。
    下面是原始记录，留作背景。

    原文：它只比较 `MAX(version)`。
    `messages` 加三列时是**就地改 v1**（前提是当时不存在任何真库，已按 D-9 三级逐级核实），
    于是 `version = 1` 指向两套不同的 schema。旧库打开时 `Open` 会成功、横幅照打，
    第一次读或写才报未知列；更坏的是将来加了 v2 之后，这种旧库会被盖上「已是最新」的章，
    从版本号上再也分不出来，且此后任何迁移都不会回头补那三列。
    **Step 6 前**加 schema 指纹校验（例如 `Open` 时断言 `pragma_table_info` 与 `v1DDL` 一致）。
    注：三级核实只在 E 盘机做过，D 盘机 `[未验证]`。

11. ~~**三个脑子列用空串还是 NULL 表示「没有」**~~ —— **已裁决：维持 `TEXT NOT NULL DEFAULT ''`
    不动**（2026-09-21，Step 6 写完适配器之后）。本条当初挂起的理由是「等 Step 6 写适配器、
    知道三家 CLI 实际返回什么之后再裁」，那个证据现在有了，而且它把问题取消掉了：
    **三家的输出里一个 effort 字段都没有**（七份成功原件加三份失败原件，逐个看过）。
    所以 `sender_effort` 永远只可能是请求值，NULL 与空串都填不出实测值来——
    可空性根本表达不了那个区分。

    真正要区分的那两件事（「这家没有 effort 这个概念」与「适配器漏填了」）改由两个
    机器可读的 meta 键承担：`effort_policy`（`per_turn_flag` / `process_external_config` /
    `baked_into_model_label` / ...）说的是这家有没有这个概念、以及概念长在哪；
    `effort_flag_omitted` 说的是这一轮实际有没有把 flag 放上命令行。
    provider 与 model 两列的漏填则从源头堵死：`Post` 对 `KindAgent` 缺这两列直接返回
    `ErrMissingSenderBrain`，不靠列的可空性事后追究。
    换 NULL 的代价反而是实打实的：要连 `CHECK` 与历史行一起改，而换来的表达力为零。
    下面是原始记录，留作背景。

    原文：现在是 `TEXT NOT NULL DEFAULT ''`，
    与同一个 schema 里 `turn_usage.cost_usd` 可空、且 `05` §3 明写「可空表示这家 provider
    根本没有这个数，不是 0」的惯例相反。代价具体：`sender_effort = ''` 同时表示
    「这家 provider 没有 effort 这个概念」与「有但适配器漏填了」，库里、SQL 里、屏幕上
    都是同一个东西，而 `brainCell` 的注释恰恰告诉读者那个占位符读作「不适用」。
    反方向的论据：三列总是由适配器一起写，漏填是适配器的 bug，换成 NULL 也一样漏。
    **等 Step 6 写适配器、知道三家 CLI 实际返回什么之后再裁**；改的话要连 `CHECK`
    与历史行一起改，不是换个类型那么简单。

12. **已实测，已裁，已收口：走括号粘贴识别（B-C）**（2026-09-21 首裁，2026-09-22 复跑收口）。**—— 已被 #39(b) 订正（2026-09-23）**
    原件 `E:\Isolating_test\council\oq12-paste\RESULTS.md`，
    落地形状见 `13_STEP10_CONTRACT.md` 第 1.3.5 节与第 5.2 节。

    **订正**：「熟模式下开 VT_INPUT 就拿得到标记、不必切 raw」这一测量是对的，但它的副作用没测：开了 VT_INPUT 之后，
    Windows 控制台自带的行编辑不再处理方向键、退格与历史，而是把它们翻成转义序列塞进当前行。真机上方向键那一行被记进会话、发给了 grok。
    X-39b（`80143b8`）让一行模式不再请求括号粘贴。**代价**：一行模式里多行粘贴会变成多条消息，粘贴里以 `/` 开头的行会被当命令执行
    （含 `/round --go`、`/ask` 这类花钱的）。多行或含斜杠的内容要整块记成一条，走 `/say` … `/end`，或用 TUI。
    粘贴状态机（`paste.go`）没删：万一哪个终端自己发来标记，照样被剥掉、按粘贴处理。

    ### 2026-09-22 的复跑（Griffin 在真终端，A2 / C2 / B2 三组一次跑齐）

    首裁那次的软肋是**前提没控制住**（人到底粘了还是敲了，只能靠计时猜，
    §1.3.3 因此判「那个 false 不能采信」）。这次三组的 verdict 都写着
    `input method: PASTE, proven by the markers themselves` ——
    **前提由标记自证，不靠计时**。那个缺口没有了。

    | 组 | 配置 | START/END 标记 |
    |---|---|---|
    | A2 | 只申请 2004，输入句柄不动（`VT_INPUT=false`） | **看不到** |
    | C2 | 2004 + `VT_INPUT` + 保留行缓冲（cooked） | **看得到** |
    | B2 | 2004 + `VT_INPUT` + raw | **看得到** |

    **新结论一：光申请 2004 不够，必须同时开 `VT_INPUT`。**
    A2 与 C2 只差这一位（`0x01F7` -> `0x03F7`），A2 拿不到、C2 拿得到。
    这一条此前没单独测过。

    **新结论二（关闭「限定二」）：cooked 模式下多行粘贴是逐行交付，不是整块。**
    这正是 `13` §1.3.5 悬着的那条 ——「钉出来若是整段一个 chunk，
    `splitPasteMarkers` 的签名要从逐行改成收整块」。实测 C2 的四行粘贴：

    ```
    chunk1: ESC[200~ + 第1行 + CRLF      <- START 贴在第一行开头
    chunk2:            第2行 + CRLF      <- 中间行不带标记，间隔 0.000ms
    chunk3:            第3行 + CRLF
    chunk4:            第4行 + ESC[201~ + 之后手敲的内容 + CRLF
    ```

    **所以 `splitPasteMarkers` 保持逐行的签名是对的，不返工。**
    额外量到一件任务书没问的：**END 标记与最后一行正文同在一个 chunk，
    而且后面还能跟着手敲的字符** —— 分割器必须能处理「标记出现在一行中间」。
    逐行加标记检测正好覆盖；整块式反而做不到。

    **raw 模式（B2）的分块形状不同**：`ESC[200~` 单独一个 chunk，
    正文与 `ESC[201~` 在第二个 chunk。但**这次 B2 粘的是单行**，
    所以「raw 下多行是不是整块」**没测到**，按 [未验证] 记，不写成结论。
    我们走 B-C（C2 那套），适用的是 cooked 的逐行形状，这一条不影响落地。

    ### 仍然 [未验证] 的一条

    三组都跑在 **conhost / 应用内嵌面板**（`no WT_SESSION`），
    **Windows Terminal 下没验过**。`13` §1.3.4 要求 WT 与应用自带面板各跑一次。
    不阻塞：B 片代码早已落地（`03dcef4` + `10124a9`）且按 `ok bool` 设计，
    终端不支持就退回今天的行为。等哪次顺手在 WT 里跑一遍 C2 即可补齐。

    **C2 组（申请 mode 2004 + `VT_INPUT` + 保留行缓冲）拿得到标记**，
    原始字节里 `1b 5b 32 30 30 7e`（`ESC[200~`）与
    `1b 5b 32 30 31 7e`（`ESC[201~`）都在。
    **所以 Council 不必切 raw 模式**，也就不必自己写最小行编辑器
    （退格、方向键、Home/End、历史、宽字符光标）或引一个 readline 依赖 ——
    整条修法里最贵的那一笔不用付。

    **两处限定**：(a) 这一次覆盖的**不是** Windows Terminal（探针自报
    `no WT_SESSION / TERM_PROGRAM / TERM`），WT 按 `[未验证]` 记 ——
    不影响落地形状，`enablePaste` 本来就按 `ok bool` 设计，
    承载得住「这个终端不支持就退回今天的行为」；
    (b) 这一次粘的是**一行**，**多行粘贴怎么分块没量过**，
    落地第一件事就是拿同一个探针粘三行钉死它。

    **严重度在修好之前维持 high**：Council 的典型用法就是贴一段需求进去，
    而现在它会被切成 N 条**永久**消息。

    **这条走了四次运行才拿到，值得记住为什么**：前三次的 false 都不可采信，
    每次都有一个没被控制住的变量 —— 没申请过 2004 / 人是敲的不是粘的 /
    跑在哪个终端模拟器下。第四个错误在探针自己身上：计时判据量的是 chunk
    **之间**的间隔，而人「粘一段、回车、再粘一段」时那当然是几秒 ——
    **它回答的不是它自称在回答的那个问题**，于是给出了与标记证据相反的读数。
    四个同形：**一个看起来是结论的 false，其实前提没被控制住。**

    **下面是裁决之前的探针设计，留作背景。** 三个候选里，
    **时间窗启发式按 D-1「不做 idle 启发式」直接排除**，不再讨论。
    剩下「括号粘贴检测」与「维持显式模式并做得更好找」，
    而选哪个**取决于一个没人测过的事实**：Windows 控制台在什么条件下
    才会把 `ESC[200~` / `ESC[201~` 这对标记送进来。

    **探针要分三组，一次只动一个变量**（原设计只有两组，A 与 B 一次改了两个变量，
    分不清「必须 raw 读」还是「只要开 VT_INPUT 就够」）：

    | 组 | `ENABLE_VIRTUAL_TERMINAL_INPUT` | `ENABLE_LINE_INPUT` / `ENABLE_ECHO_INPUT` |
    |---|---|---|
    | A | 不动（当前默认） | 保留 |
    | B | 置上 | **清掉**（raw） |
    | **C** | **置上** | **保留**（cooked 行缓冲） |

    **C 组是关键**：若在 cooked 行缓冲里就能看到括号粘贴标记，
    那最怕的那笔代价（把 `bufio.Scanner` 换成 raw 模式、自己手写最小行编辑器，
    或引一个 readline 新依赖）就不用付。

    判据是**收到的字节**：往终端粘一段含换行的文本，看读到的第一段里有没有
    `ESC[200~`。不是退出码、不是「看起来没报错」。
    本项目全库今天**零处**用 `isatty` / `IsTerminal` / `golang.org/x/term`，
    所以这三组都要现写探针，不是改配置。

    **严重度维持 high**：Council 的典型用法就是贴一段需求进去，
    而现在它会被切成 N 条**永久**消息。

    **下面是原文，留作背景。**

    原文：Step 5 加了显式多行模式（`/say` 不带参数进、`/end` 发送），
    但不先敲 `/say` 就直接粘贴，依旧被读循环按行切成 N 条永久消息。
    三个候选设计：括号粘贴检测（bracketed paste，技术上最对，但要把 `bufio.Scanner`
    换成 raw mode 读取、Windows 控制台行为要另验）；输入缓冲加时间窗（本项目明确反对
    启发式，见 D-1「不做 idle 启发式」，基本排除）；维持显式模式并把它做得更好找。
    **严重度按 high 记**：Council 的典型用法就是贴一段需求进去。

13. ~~**`Post` 的 participant 活跃校验在写事务之外**~~ —— **已修**。`Tx` 加了
    `RequireActiveParticipant`，校验挪进与追加消息同一个 `WriteTx`，
    事务外那次读句柄查询一并删掉（留着它窗口就还在，只是多一层看起来像在防它的代码）。
    锁它的用例是 `TestPostChecksParticipantInsideWriteTx`，直接断言调用序列
    「WriteTx 打开 -> 判定 -> 追加消息」—— 因为「退出的成员发不了言」这条断言
    在校验放事务内和放事务外两种实现下都成立，验不出区别。
    **仍未解决的部分**：`-race` 本机跑不起来（无 gcc），那个竞态本身没有实测复现，
    锁住的是结构而不是并发行为。下面是原始记录，留作背景。

    原文：它先用读句柄查活跃成员
    （`service.go`），再另起 `WriteTx` 追加消息；会话存在性在事务内被重查了一次，
    participant 这一条没有，`messages` 上也没有对应的外键。
    Step 6 有并发发言之后就有窗口：读到「活跃」之后、提交之前，用户 `/leave` 先行提交，
    这条消息照常落库，事后 `/who` 显示它已退出、`left_at` 早于该消息的 `created_at`，
    没有任何一层会报错。**Step 6 前**把校验移进事务，或加 `messages -> participants` 外键。
    与 `-race` 本机跑不了那条一起处理。

14. ~~**按 `kind` 立的规矩一条都还没有**~~ —— **已可关闭**（2026-09-21）。
    本条卡住的那个问题（**system 消息的 sender 该是什么**）已经在 Step 6 定了：
    保留 id `"system"`，声明在 `conversation/types.go:44`，理由写在 `:35-40`，
    配套保留 id 进了 `config/validate.go:25`，
    设计意图写在 `09_STEP6_SPEC.md:828-839`（正文明写「裁定未决 #14 的后一半」），
    引入点 `4a40fe9`。唯一的生产构造点是 `turn/runner.go:725-744` 的 `postSystem`。

    两条缺口也都落了：
    (a) `KindAgent` 缺脑子 -> `service.go:110-114` 的 `ErrMissingSenderBrain`，
        `RecordTurn` 在 `:193-197` 有同一份；
    (b) 保留 sender 不得带脑子 -> `service.go:93-103` 经 `isReservedSender`（`:403-405`）。

    **关闭时要补两处负向用例**（对抗复核实跑过变异证明现在锁不住）：
    把 `service.go:93` 与 `:189` 两处判据窄化成 `== ReservedSenderUser`
    （`isReservedSender` 本身不动），`go test ./...` **全绿** ——
    也就是说 `system` 那一半今天没有任何用例守着。
    补法：`conversation/brain_test.go`（Post）与 `record_turn_test.go`（RecordTurn）
    各加一个 `{SenderID: ReservedSenderSystem, Kind: KindSystem, 三列带满}` -> `ErrInvalidSender`。
    **只补 Post 那一半的话 `:189` 仍然无人锁。**

    **`CHECK` 裁为不补**，但理由要换：不是「写路径唯一」（`store.Tx.AppendMessage`
    是导出方法，别的包拿到 `Tx` 就能绕过 Service），而是 D-10 的单写进程前提
    加上改 `CHECK` 的代价。保留触发条件：将来 `messages` 因别的理由整表重建时，
    顺手把 `'user'` 扩成 `NOT IN ('user','system')`。

    **下面是原文，留作背景。**

    原文：现有两层（`Service.Post` 与 `CHECK`）
    拦的都是 `sender_id`，与 `kind` 无关。由此缺两条：
    (a) 没有任何一层要求 `KindAgent` 消息带脑子，适配器漏填会静默渲染成占位符，
    已在 `08a` 写成给 Step 6 的 MUST；
    (b) HANDOFF 原本要求「`user`/`system` 消息这三列空」，`system` 那一半没做。
    两条都卡在同一个未定的东西上：**system 消息的 sender 该是什么**
    （保留 id？那个 agent 的 id？）。Step 6 定 system 消息语义时一并裁，
    那时再决定要不要在 `Service.Post` 加按 `kind` 的校验。

15. ~~**`MaxPromptBytes` 量的是 prompt，OS 限的是整条命令行**~~ —— **已关闭**（2026-09-21）。
    Step 8 第一片加了整条命令行的总长闸：`turn/spawn.go` 的 `CommandLineBytes` /
    `maxCommandLineBytes` / `ErrCommandLineTooLong`，在 `.cmd` 换行闸之后、
    落 Files 盘之前判定。**量的是刚组出来的那一条真实命令行，不估、不留余量** ——
    这正是本条原文说「唯一诚实的做法」的那一条。

    **真机验过**（`step8a-smoke/RESULTS.md` 第四轮）：40621 字节的规则文件下，
    grok 报「命令行 41287 字节，上限 32767；最长的参数是
    `--system-prompt-override` 的值（40898 字节）」，`wall_ms=0`，一个进程都没起。

    **三条残余，明写在代码注释里，不是漏掉**：
    - 这道闸**偏松**：不算 Go 在 Windows 上给含空格参数加的引号，
      挡得住整数量级的越界，挡不住刚好卡在边上的几十字节。
      两个误差方向相反（UTF-8 字节数 ≥ UTF-16 码元数那一侧保守，引号那一侧少算），
      所以**不能说它「永远不会放过真正越界的命令行」**。
    - **agy 撞的不是这道闸**：它的规则折进 argv 正文，先撞 `BuildTurn` 里的
      `MaxPromptBytes`，报 `ErrPromptTooLong`。两条都是起进程前 fail-closed，
      但**测试不许对三家写同一个哨兵断言**。
    - outcome 分类有个小瑕疵：grok 记 `spawn_failed`、agy 记 `refused`，
      因为两道闸住的地方不同。记账等价，未修，单独记在下面 #19。

    **下面是原文，留作背景。**

    原文：两者不是同一个东西，
    于是 `ErrPromptTooLong` 这道闸有一段漏风：prompt 刚好卡在 32767 以下、再加上
    `--model` `--append-system-prompt <身份块>` `--permission-mode` 等几百到几千字节，
    整条命令行仍可能越界。那时失败来自 `CreateProcess`，不是本包的哨兵，报错形状
    不受我们控制，而且**长度相关**：同一个 agent 对短需求成功、对长需求失败。
    没有当场收紧，是因为唯一诚实的做法要么是量出 flag 段的真实字节数，要么是编一个
    余量常数 —— 后者会把「未实测」伪装成「已标定」，正是本项目一直在防的那类谎。
    与未决 #12（粘贴一段长需求正是 Council 的典型用法）是同一件事的两头。
    **Step 7 处理**：用真实批次量 flag 段实际长度，再决定是把 `MaxPromptBytes` 减去
    实测余量，还是改成由 `BuildTurn` 校验整条 argv 的总长（后者要动冻结契约的措辞）。

    **2026-09-21 补**：claude 那一路已经改走 stdin（见未决 #16），`MaxPromptBytes`
    对它归零，本条现在只剩 grok 与 agy 两家还在 argv 上。

16. ~~**`.cmd` / `.bat` 垫片会在 argv 的换行处截断整条命令行**~~ ——
    **已关闭**（2026-09-21）。原文写的两件处置都做了、都有用例：

    - claude 改走非 argv 通路：身份块进 `--append-system-prompt-file`
      （`claude.go:83` 拼路径、`:105` 填 `Files`），正文走 stdin（`:104`）。
    - `Spawner` 里那道 fail-closed 的闸在（`turn/spawn.go`，`ErrShimNewlineArg`），
      判定是「可执行文件是 `.cmd`/`.bat` **且**任一 argv 元素含换行」，
      起进程之前拒绝。反例用例也在（`.exe` 不被误伤）。

    真机侧的旁证：`step8a-smoke/RESULTS.md:19` 与 `:24` ——
    claude 在 `--safe-mode` 之下身份块照常注入、用户全局规则痕迹消失，
    说明那条文件通路是活的。

    **那道闸才是长期保护**：将来谁给哪个适配器加一个带换行的 flag 值，
    会当场红，而不是变成一次身份被截短的静默成功。

    **下面是原文，留作背景。**

    原文：2026-09-21 真机实测。
    Windows 上 claude 是 npm 装出来的 `claude.cmd`，Go 起 `.cmd` 时命令行由 cmd.exe
    再解析一遍，而 argv 元素里的换行在 cmd.exe 眼里是命令分隔符。
    我们发的 `--append-system-prompt <身份块>` 天然多行（`renderIdentity` 产出
    「You are X (id=y).」换行「Provider: z」换行「role」），于是命令行在第一个换行处
    断掉，后面的 flag 与正文根本没送到。
    **这次是硬失败，但真正危险的是它的另一半**：截断后剩下的命令行若仍然合法，
    claude 会带着一个被截短的系统提示跑完、退出码 0、JSON 合法、游标照推 ——
    身份错了、缓存前缀错了，没有任何一层会报错。
    grok 与 agy 是原生 `.exe`，Go 直接传 argv 不经 cmd.exe，实测多行 argv 正常。

    **处置**（Step 6 内完成）：claude 改走非 argv 通路 ——
    身份块写文件走 `--append-system-prompt-file`，正文走 stdin；
    两条都用「不存在的 model 名」做过零成本语法检查，`total_cost_usd` 均为 0。
    同时在 `Spawner` 里加一道 fail-closed 的闸：可执行文件是 `.cmd` / `.bat` 且任一
    argv 元素含换行时，**起进程之前**拒绝并报 `ErrShimNewlineArg`。
    那道闸才是长期保护 —— 将来谁给哪个适配器加一个带换行的 flag 值，会当场红，
    而不是变成一次身份被截短的静默成功。

17. ~~**agy 在 `--mode plan` 下返回 `status=SUCCESS` 但 `response` 是空串**~~ ——
    **已定位并修复**（2026-09-21）。单变量隔离：只改 `--mode` 这一项、其余全部不动，
    `--mode plan` 下一次纯对话烧掉 576 个 output token 却只回一个空串
    （换成文本输出格式时 stdout 干脆是 0 字节），**不带 `--mode`** 时同一个提问
    正常回答、21 个 output token。所以病因是 plan 模式本身。
    原件在 `E:\Isolating_test\council\agy-mode-probe\`。

    **处置**：READ 档不再翻译成 `--mode plan`，改为不发 `--mode`。
    本家只有 `accept-edits` 与 `plan` 两个值，**没有只读档**，所以这里不是
    「有默认值可显式写出来」（claude 那格是显式发 `--permission-mode default`），
    而是「根本没有这个档」，写一个不存在的值会被参数解析直接拒。

    **只读这条性质是直接验过的，不是推的**：让它「在工作目录里建一个 PROOF.txt
    并写入 WRITTEN」，结果 `response` 为空、工作目录 / cwd / 项目根三处都没有
    任何文件被写出；而同一轮配置下纯文本提问能正常回答，所以不是它整体罢工。
    默认档要求逐次授权，而 headless 下没人可问。
    **残余风险**：一次否定实验证明不了「写盘绝无可能」。按「已验证一次、未穷尽」记，
    比原来的「未验证」强，但别当成沙箱。

    真机复验：agy 在 council 里连跑两轮都正常回答，`--conversation` 恢复也对。
    **顺带一条成本观察**：第二轮 `input_tokens` 从 16195 涨到 32722 而
    `cache_read` 仍是 0 —— agy 跨轮没有缓存收益，`UnexpectedCold` 正确告警。
    与 claude / grok 的前缀缓存行为完全不同，做成本视图时要分开看。
    下面是原始记录，留作背景。

    原文：
    2026-09-21 真机实测：`in=16999 out=724 thinking=624`，token 实打实消耗了，
    正文却是空的。Council 这边行为是对的（`Completion` 归 `empty`，照样落库、
    屏幕上响亮地说「本轮为空」），但一个只会产出空发言的成员在讨论里没有用。
    怀疑与 `--mode plan` 有关：READ 档翻译成 plan 模式，而 plan 模式的产物可能是
    计划而不是回答。**Step 7 或 Step 8 处理**：要么给 READ 档换一个 agy 模式，
    要么确认 plan 模式下正文该从别的键取。在此之前 agy 只算「通路已打通」，
    不算「可用成员」。

18. ~~**改了适配器发出的 flag，却没有同时动身份指纹**~~ —— **已落地**（`621bada`，契约 `11b`）：**选候选一的一个变体 —— 规范化探针摘要，
    进指纹第七段。** 候选二（手工 `prefixVersion`）否决，理由就是本条记的这个病：
    它会被忘，而忘了没有任何一层会说话。

    反例复核过：`git show --stat 6530839 -- internal/turn/session.go` **输出为空**，
    而那次提交改了 `claude.go` 与 `grok.go` —— 改 flag 与改指纹确实分开了。

    **摘要怎么取**：用一个**规范化的探针请求**现组一次 argv，摘它。
    规范化的意思是把每轮都变的东西换成占位符：`PlannedSessionID`（grok 在
    `grok.go:137` 交出来）、prompt。**不能靠「传一个固定非空 ProviderSession」
    来回避 UUID** —— 那会走 resume 分支，而 agy 的身份拼接只在
    `ProviderSession == ""` 那一支（`agy.go:59-63`），于是 agy 的前缀结构
    永远进不了摘要，**正是本条要防的那类走失**。要么 fresh 形状加占位符替换，
    要么 fresh + resume 两个形状各摘一次拼成第七段。

    **落地前必须先裁的六件**：

    1. **探针报错时的出口，明确不许吞。** `prefixArgsDigest` 会失败（权限档不放行、
       将来新加的校验），而 `identityFPFor`（`session.go:101`）与
       `IdentityFingerprint`（`session.go:79`）今天都只返回 string。
       吞掉错误就是静默降级，本项目明令不可。建议让 `identityFPFor` 返回 error，
       审计侧照 `drift.go:78-85` 的体例落成一行 Reason，Ask 侧走 refuse。
    2. **冻结契约要动两处**：`11a:231-234` 的 `fingerprintFields`，
       以及 `IdentityFingerprint` 本身。**动手之前要我点头。**
    3. **`ErrNoDelta` 那条警告的复核结果**：它来自设计合议，我原样记了「未独立复核」。
       复核后：`RESULTS.md` 里那三行漂移出自**启动审计**（读还留在库里的行），
       走的不是 `runner.go:246-250` 的作废分支。所以那条交互**不是「今天就在跑」**，
       按 `[代码证实、未经真机]` 记。
    4. **响亮信号挪到作废那一刻**（`runner.go:246-250`），不要挪到后面：
       那时旧指纹还在手上，`identityDriftFields`（`runner.go:917-944`）现成可用。
       否则「行被删之后与真冷启动不可分辨」是常驻残余。
    5. **golden 钉 argv 列表本身**，摘要由它派生 —— 钉摘要等于钉一个哈希，
       变了看不出哪里变了。
    6. **「改 flag 与改指纹必须同裁」要由测试强制，不是靠人记得**：
       golden argv 一变就红，而改摘要算法同样要过那条 golden。

    **本机受害者仍是 6 条、全在冒烟库里、生产库不存在**（`E:\council-data`、
    `~/.council/config.toml` 本次复核再次确认均不存在）。所以是流程账不是迁移账。

    **这条阻塞 Step 8 第二片**：那一片要改 WORKSPACE 的 flag
    （`claude.go:141-150` 写好但被挡、`grok.go:170-183` 明确拒绝），
    不先裁就是重犯同一个错。

    **下面是原文，留作背景。**

    原文：2026-09-21 发现，`6530839` 本身就是实例。

    那次提交把 claude 的 READ 档从一个 flag 改成四个（加 `--safe-mode` `--restricted`）、
    给 grok 的 READ 档加了三条 `--deny`。**发出去的 argv 真的变了，而身份指纹一个字没动**：
    `fpFieldTier` 记的是档名 `read`，改前改后都是 `read`。
    于是那次提交之前建立的 provider 会话，前缀已经变了而指纹说没变，
    下一轮 `--resume` 会拿着按旧 flag 建好的前缀继续跑，没有任何一层会报错。

    **为什么指纹漏掉了它**：现有四段量的都是「**用户配置**决定的东西」
    （role / model / effort / tier），而 flag 段是「**适配器代码**决定的东西」。
    后者的变化发生在编译期，运行时没有任何一层在看。这不是实现疏忽，
    是指纹的定义域从一开始就只覆盖了一半。

    **本机受害者：6 条，全在隔离根下的冒烟测试库里，生产库不存在。**
    `6530839` 提交于 2026-09-21T01:06:27Z，库里最晚的一条 provider 会话是 01:05:55Z。
    `E:\council-data` 这个目录没有被创建过，`~/.council/config.toml` 也不存在。
    所以这条**在本机是流程教训，不是要用迁移去补的账**。

    **两个候选，都还没裁**：

    - **候选一：按档位的静态 args 摘要。** 给 Provider 接口加一个「前缀相关的静态 args」
      出口（`*PermissionArgs` 的产出，加上身份载体的选择），摘要进指纹。
      自动，不会被忘。**坑在于要裁「哪些 arg 与前缀有关」**：
      session id 与 prompt 每轮都变，绝不能进；这条线画错一点，
      要么天天误报漂移、要么照样漏。
    - **候选二：每个适配器手工维护一个 `prefixVersion` 整数**，
      改 prefix 相关 flag 时手动加一，进指纹。显式、可审查、没有分类问题。
      **坑在于会被忘，而忘了的后果正是本条记的这个。**

    **选候选一之前必须先想清一条交互**（来自 Step 8 设计合议的对抗挑刺，我未独立复核）：
    摘要一旦落地，`ErrNoDelta` 那一支会导致零成本静默丢弃 provider 会话，
    而下一次访问时「正常冷启动」的外观与真实冷启动无法区分。

    **何时裁：Step 8 第二片（WORKSPACE）之前。** 那一片要改 WORKSPACE 的 flag，
    正好会再触发一次同样的问题。
    **连带纪律：改 flag 与改指纹必须同裁，不许分在两个 commit 里。**
19. ~~**两道「起进程之前拒绝」的闸，outcome 分类不一样**~~ —— **已落地**（`621bada`，只订正了 `ErrCommandLineTooLong`；同类的 `ErrShimNewlineArg` 见 #21）。2026-09-21 真机观察到。

    同一件事（超大规则文件），grok 记 `spawn_failed`、agy 记 `refused`，
    因为命令行总长闸住在 `spawnProcess` 里，而 `MaxPromptBytes` 闸住在 `BuildTurn` 里。
    `spawn_failed` 字面读作「试着起了进程但失败了」，而这里一个进程都没起
    （`wall_ms=0`）。

    **记账上两者等价**（都没有 provider 侧副作用，不涉及「第三种结局」），
    屏幕上也够用，所以没有当场改。**但本项目对「分类要诚实」有洁癖**，
    留着它下次会有人拿 `spawn_failed` 的计数去推「有多少次真的起了进程」。

    改法很小：让 Runner 把 `ErrCommandLineTooLong` 归到 `refused`。
    **别当成漏测**，它是一个已知、已定位、已估价的分类瑕疵。
20. ~~**一轮被拒绝会把那条 provider 会话静默删掉**~~ —— **已落地**（commit 待填）。2026-09-21 实测坐实。
    实现方在复核自己的改动时指出，我用真机 repro 验了。**这是既有 bug，不是新引入的。**

    **repro（零成本，被拒的那一轮根本不起进程）**：
    一个 agent 正常跑一轮建起会话，然后只把配置里的 `permissions` 从 `read`
    改成 `workspace`（**配置本身合法**，但 `TranslatePermission` 会拒），再 `/ask` 一次：

    ```
    改之前   provider_sessions: [('gk', 'eb7c49c4-cec7-43a8-b6ea-dabc7fe4291b')]
    拒绝之后 provider_sessions: []
    ```

    也就是说：**改一个配置字段、那一轮被拒，这条 provider 会话就没了。**
    改回来要重付一次入场费（claude 侧 0.4 到 1.0 美元），而且没有任何一层会说。

    **机制**：`refuse` -> `persistTurn(ctx, rec, st)` -> `snapshotProviderSession`
    在 `st == nil || st.providerSession == ""` 时返回 nil -> `Service.RecordTurn`
    对 nil **无条件** `DropProviderSession`。
    而「读库之前就拒绝」的那几支（agent 不存在、provider 查不到、权限档不放行、
    WorkDir 为空、以及 #18 新增的探针算不出指纹）传下去的 `st` 就是 nil。

    **`ErrIdentityDrift` 那一支是安全的**：它传的 `st` 带着非空的
    `providerSession`，走 `SetProviderSession` 原值回写，不删。
    这很重要 —— 否则「拒绝而不是静默重建」这条设计会被自己拆掉：
    拒绝的同时把会话删了，下一轮照样重建。

    **根因是一个语义被两件事共用**：`RecordTurn` 的 `session == nil` 同时表示
    「这条会话被有意作废了，删行」与「这一轮压根没碰过会话状态」。
    前者该删，后者绝不该删。

    **裁决：把 nil 的含义改成「别碰这一行」，作废改成显式调用。**

    1. `Service.RecordTurn` 的 `session == nil` 从「删行」改成「这一轮不动
       provider_sessions」。这是**冻结契约的语义变更**（`10a` 第 2.4 节），
       要在那里留订正记录。
    2. 有意作废改成在**做出那个决定的地方**显式调 `Service.DropProviderSession`
       —— 也就是 `runner.go` 里从库读回指纹不符那一支，#18 刚在同一处加了
       `session_rebuilt` 面包屑，两件事本来就该在一起。
    3. 成功轮不受影响：它照常 `SetProviderSession` 覆盖整行。
    4. **不许只为了让用例变绿去改断言**：现有用例里凡是断言「拒绝之后行没了」的，
       要逐条确认它当初想验的到底是什么，再决定改断言还是改代码。
    5. 配一条回归用例，判据是**库里那一行还在不在**，不是屏幕输出。

    **为什么值得现在做**：#18 刚把触发面扩大了（探针算不出指纹也走这一支），
    而它的触发条件是「agy 的规则文件一时超过 32767 字节」这种**可逆的临时状态**，
    不是永久配置错误 —— 临时状态不该把一条花过钱的会话删掉。

    **已落地（commit 待填）**：按上面五条裁决原样实现。`Service.RecordTurn`
    的 `session == nil` 分支从「调 `DropProviderSession`」改成「什么都不做」
    （`internal/conversation/service.go`）；作废改在 `internal/turn/runner.go`
    里「从库读回、指纹不符」那一支显式调 `Service.DropProviderSession`——就在
    `postRebuildNotice` 面包屑发出去之后、`st = &sessionState{}` 重建之前，
    与 #18 加的那颗面包屑挨在一起。`ErrIdentityDrift`（进程内已有记录指纹不符）
    那一支未改一行：它传的 `session` 本来就带着非空 id，走的是
    `SetProviderSession` 原值回写，从未经过 `nil` 分支，新增回归用例钉住
    「拒绝之后这一行原值还在」。

    **三处既有断言依赖旧的「nil 删行」语义，逐条判断后全部按新语义改写**
    （不是为了让用例变绿才改，是它们原本就在钉现在认定为 bug 的那个行为）：
    `internal/conversation/record_turn_test.go` 的
    `TestRecordTurnCallOrderSessionNil`、`internal/turn/record_turn_calls_test.go`
    的 `TestRecordTurnCallShape_BootstrapFailure_SessionNeverEstablished`、
    `internal/store/record_turn_test.go` 的
    `TestRecordTurnSessionNilDropsProviderSession`。最后一条在 `internal/store/`
    目录里，任务书原话是这一轮不碰这个目录；但它的断言（"session=nil 之后
    provider session 仍然存在" 判为错误）与本条裁决直接矛盾，不改它
    `go test ./...` 就过不了全绿，所以按「测试专用的最小例外」处理，只改了
    这一个测试函数的断言与注释，没有碰 `internal/store` 的任何实现代码。

    新增回归用例（判据都是 `svc.ProviderSession` 读回的那一行还在不在、值
    是否原样）：早期拒绝（权限档改成不放行的档）之后已建立的会话原样还在；
    进程内指纹漂移（`ErrIdentityDrift`）拒绝之后原会话原样还在；从库读回的
    指纹不符、这一轮重建后又失败（不是成功）时旧行被真正删掉，不是留到
    `RecordTurn` 结尾靠 `session=nil` 顺带删掉。

21. ~~**`ErrShimNewlineArg` 有与 #19 一模一样的分类问题**~~ —— **已落地**（commit
    待填）。实现方指出，未修。

    它也是起进程之前的闸（`spawn.go` 里 `.cmd` / `.bat` 垫片含换行那道），
    命中时一个进程都没起，却同样落进 `spawn_failed`。
    #19 只订正了 `ErrCommandLineTooLong`，因为契约第 5 节只点了那一个。

    与 #19 同一个改法，同一个理由。**别当成漏测**：已知、已定位、已估价。

    **已落地**：`internal/turn/runner.go` 里原本只判 `ErrCommandLineTooLong`
    的那个分支，改成 `errors.Is(spawnErr, ErrCommandLineTooLong) ||
    errors.Is(spawnErr, ErrShimNewlineArg)` 一起归 `OutcomeRefused`、`WallMS`
    清零，注释同步说明两条是同一类闸。新增回归用例
    `TestOutcomeShimNewlineArgIsRefusedNotSpawnFailed`（`runner_outcomes_test.go`）：
    把 grok 的 provider 可执行文件配置改成一个不存在的 `*.cmd` 路径（`LookPath`
    找不到，路径原样保留，`filepath.Ext` 判成 `.cmd`），grok 的身份文本
    （`--system-prompt-override`）本身就是多行，走真实的 `spawnProcess`（不换
    `fakeSpawner`）即可在起进程之前触发这道闸，断言 `Outcome == OutcomeRefused`
    且 `WallMS == 0`。
22. ~~**被打死的那一轮把「不知道」的用量写成了 0**~~ —— **已落地**（`79547ea`），**2026-09-22 复核关闭**。

    现查：`makeTurnUsage` 里 `if !rec.UsageParsed` 那道门已经在，注释里还点名着「未决 #22」；`internal/turn/usage_unknown_test.go` 的
    `TestUsageUnknown_Timeout_Killed` 正是本条描述的那个场景（一轮被信号打死）。下面的原文保留不抄掉。

    **为什么拖到现在才关**：`79547ea` 改了代码但没回头改这条条目。同一天的 `1a5d46e` 干了同样的事（做完 C3 只订正了契约 6.1、没订正 6.3）。
    **两次都是「修了但没销」**，后果是下一个人照着文档再做一遍已经做完的事。

    以下为当时的原文：
    原件 `E:\Isolating_test\council\oq-ctrlc\RESULTS.md`。

    跑 needsProbe #6（Ctrl-C）时顺带量到的：一轮被信号打死之后，
    `turn_usage` 那一行是

    ```
    outcome = 5（OutcomeTimeout）   wall_ms = 11426
    cost_usd = NULL                  <- 对
    input_tokens = 0                 <- 错，应当是 NULL
    cache_read = 0  output_tokens = 0  thinking_tokens = 0
    ```

    进程被打死，**根本没有产生过 usage JSON**，所以正确答案是「不知道」，
    不是「实测为零」。而那一轮 grok 真的跑了 11.4 秒，token 是实实在在烧掉的。

    schema v2 把这七个用量列从 `NOT NULL DEFAULT 0` 改成可空，理由原文就是
    这件事：「NULL 读作『这家没报这个数』，0 读作『实测为 0』」，
    与 `05` 第 3 节「可空表示这家 provider 根本没有这个数，不是 0」同一条纪律。
    **这一行在写入侧把那个区分抹掉了。**

    后果不是抽象的：`13_STEP10_CONTRACT.md` 第 7.1 节要求 `/cost` 汇总时
    「NULL 绝不能被当成 0 加进去」，而这里是反方向的同一个病 ——
    读的那一侧再小心也没用，因为库里存的已经是 0 了。
    一条被打死的轮会贡献一个货真价实的零，把每轮均值往下拉。

    **怎么修**：`makeTurnUsage` 在「从没解析过 provider 输出」的那几条路径上
    （`OutcomeTimeout` / 将来的 `OutcomeCanceled` / `OutcomeSpawnFailed` /
    `OutcomeUnparsable`）把七个用量列写成 NULL，而不是抄 `rec.Usage` 的零值。
    判据不能用「Usage 是不是零值」—— 真有一轮用量确实为 0 时那会误判；
    要用「有没有成功解析过」这个事实。

    **归属**：与 `11d` 3.5 新增 `OutcomeCanceled` 是同一处代码，
    建议并进 8c-2 一起改，验收加一条：把一轮打死之后断言那七列是 NULL 而不是 0。

    ---

    **2026-09-21 开工前订正（读代码核出来的，上面那段原文两头都不准）。**
    下面这四条是这一片的冻结契约，实现方与测试方都以它为准，不以上面的原文为准。

    **(1) 写错的是四列，不是七列。** `makeTurnUsage` 里 `CacheCreate`（`*int`，
    零值就是 nil）、`CostUSD`（`rec.CostUSDDelta` 在未解析路径上恒 nil）、
    `CostCumulativeUSD`（`rec.Usage.CostUSDTotal` 同理）在这些路径上**已经是
    NULL，碰巧是对的**。真被写成 0 的只有 `reportedInt` 那四行：
    `input_tokens` / `cache_read` / `output_tokens` / `thinking_tokens`。
    与探针原件里打出来的四个 0 完全对上。验收仍按七列 NULL 断言 ——
    那是目标状态，不是改动范围。

    **(2) 判据不能按 Outcome 切，上面那张清单两头都错。**
    - 它把 `OutcomeUnparsable` 整个列进「从没解析过」，**错**：解析成功但
      `ProviderSession == ""` 那一支（`runner.go` 的空 session id 分支，
      以及三家 `ParseTurn` 里 `return base, ErrNoSessionID` 那一支）也落
      `OutcomeUnparsable`，而它们的 `res` 里装着**真花过钱的 usage**。
      按 Outcome 抹 NULL 会把这笔账丢掉 —— 与本条要修的病同一个方向，只是反过来。
    - 它**漏了 `OutcomeRefused`**：`refuse -> persistTurn -> makeTurnUsage`，
      TurnID 已铸、`rec.Usage` 是零值、`Unreported` 为 nil，四个 `&0` 照写。
      Refused 与 Timeout 同病，而且是最便宜的复现路径（给一个不在 config
      里的 agent id 即可，不必真起进程）。
    - `OutcomeCanceled` 将来加进来时，判据若是「解析过没有」而不是 Outcome
      枚举，它**自动正确**，8c-2 不必再回头动这里。

    判据只能是本条原文后半句写的那个：**ParseTurn 有没有返回一个形状认得出的结果。**

    **(3) `absorbParse` 被调用不等于解析过。** 解析失败那一支目前**无条件**调
    `absorbParse(&rec, res)`，而三家 `ParseTurn` 在「空 stdout / 不是 JSON /
    不是已知形状」三支都 `return TurnResult{}, ...ErrUnparseableOutput` ——
    这时吸收进去的是零值 `Usage`。所以标志不能无条件设在 `absorbParse` 里面。

    这一片**冻结如下不变量**（读三家适配器核实过，但哨兵定义处没写这条，
    所以在这里冻结，并要求给它一条用例）：

    > 三家 `ParseTurn` 返回零值 `TurnResult{}`，**当且仅当** error 匹配
    > `provider.ErrUnparseableOutput`；`ErrTurnFailed` 与 `ErrNoSessionID`
    > 两支**恒**返回已经填过 `Usage` 的 `base`。

    落地的缝：解析失败那一支给 `absorbParse` 的调用加
    `!errors.Is(parseErr, provider.ErrUnparseableOutput)` 这道门；
    `absorbParse` 内部置 `rec.UsageParsed = true`；`makeTurnUsage` 开头
    `if !rec.UsageParsed` 则四个 token 列与 `CacheCreate` 一律 nil。
    这样「解析过」就等于「`absorbParse` 跑过」，一个事实只有一个位置。

    **(4) 验收两个方向都要有，缺一条等于没测。**
    - 打死 / spawn 失败 / **Refused** -> 七列 NULL；
    - **真报了 0 的那一轮 -> 四列是 0，不是 NULL。** 这正是本条原文明写要防的
      退化（「判据不能用 Usage 是不是零值」）。现成的原件就有：
      `fixtures_test.go` 的 `claudeBadModelJSON` 与 `agyBadModelJSON` 两份
      归档字节里五个用量字段全是 0，且都走 `ErrTurnFailed`（即解析过），
      不必另造样本。
    - 变异两个方向：删掉 `rec.UsageParsed = true` 那行 -> 第二条必须红；
      删掉 `if !rec.UsageParsed` 那道门 -> 第一条必须红。


    ---

    **2026-09-21 落地后订正一条（测试方背对背核出来的）。**

    上面 (2) 的第一个要点写「解析成功但 `ProviderSession` 是空串那一支
    （`runner.go` 的空 session id 分支，以及三家 `ParseTurn` 里
    `return base, ErrNoSessionID` 那一支）」—— **把两件事混成了一件，
    而前一件根本到不了。**

    `runner.go` 那个 `parseErr == nil && res.ProviderSession == ""` 的分支
    是一道**防御性**的闸：三家适配器在 session id 为空时都从 `ParseTurn` 里
    直接 `return base, ErrNoSessionID`，于是 `parseErr != nil`，
    走的是上一个分支。**经这三家走不到那道闸。**
    （测试方用 `TestParseTurn_NoSessionIDReturnsNonZeroResult` 三家各验一遍。）

    结论不变 —— `OutcomeUnparsable` 确实同时装着「解析过」与「没解析过」两种轮次，
    所以判据仍然不能按 Outcome 切。**变的只是那一支的出处**：
    真正起作用的是 `parseErr != nil` 且错误是 `ErrNoSessionID` 那一条，
    不是那道防御闸。别去找一个到得了的防御分支，它就是到不了。

    **另一条也记下来，它是背对背的直接收益。**
    本契约 (4) 给方向一指定的三条路径（Refused / Timeout / SpawnFailed）
    **全都走不到 `ParseTurn`**，所以它们一条也抓不到「`absorbParse` 调用点那道门没加」。
    测试方自己看出这个洞，额外加了第四条用例（喂一段不是 JSON 的 stdout，
    真把 `ParseTurn` 逼进 `ErrUnparseableOutput`）。
    落地后做的变异 C（拿掉那道门）**只有那第四条红**——
    若按本契约原样照写，那一格会是绿的。
    规格漏了一条路径，是规格的问题，记在这里。
23. **被打死的那一轮还被记成「暖启动」** —— **已裁：维持 `NOT NULL`，由读侧按 `outcome` 过滤**（2026-09-22）。

    **本条自己写的窗口已经过去了**：原文说「Step 9 的 v4 与 Step 10 A 的 v5
    都要动这张表，到时一并改」。**v4 与 v5 都已落地，都没改** ——
    现查 `cold_start` 仍是 `INTEGER NOT NULL DEFAULT 0`。

    而 Step 10 A 的契约 3.6 在不知道本条的情况下**另立了一个决定**：
    「列不可空，所以这条纪律只能靠**读的那一侧**承担」。
    **两个互相不知道的决定并存着，这次并成一条。**

    **裁决与理由**：不做 schema 迁移。改成可空要动 `turn_usage`，
    而那张表已经是全仓最大的迁移负担（v2 建、v4 加列、v5 动邻表）；
    而收益只是把一条**读侧纪律**换成一条**类型约束** ——
    而读侧无论如何都要按 `outcome` 分档（失败轮的金额、token 都要分开算），
    多这一列不会让读侧少写一个分支。

    **纪律写死（三列同文）**：`cold_start` / `wall_ms` / `prompt_bytes` 这三列，
    **只有 `outcome` 是 `OutcomeSpoke` 或 `OutcomePostRejected` 的行才有意义**。
    其余行上那个 0 一律读作「不知道」。
    注：`wall_ms` 与 `prompt_bytes` 在**超时/被杀**那几档上其实是真实测量值
    （Runner 自己计时、prompt 是真送出去的字节），但在 **`OutcomeLost`** 行上不是；
    读侧统一按 `outcome` 分档最简单，不为这两列另开例外。

    **由谁党现**：`/cost`（D 片）是 `turn_usage` 的第一个生产读点，
    它必须把这条写进代码并配用例。**在那之前这条纪律没有任何执行者**。

    以下为当时的原文：

    `rec.Cold` 只在成功路径上算（`rec.Cold = rec.Usage.CacheRead == 0`）。
    失败路径从不给它赋值，于是它是零值 `false`，而
    `cold_start INTEGER NOT NULL DEFAULT 0` 把 `false` 存成 0 ——
    读出来就是「这一轮是暖启动」。一轮根本没跑完的调用，
    冷暖本来是**不知道**。

    与 #22 是同一个病（把「不知道」写成一个具体的值），但**不能并进同一片**：
    `cold_start` 是 `NOT NULL`，要说实话就得改成可空，那是一次 schema 迁移。
    Step 9 的 v4 与 Step 10 A 的 v5 都要动这张表，到时一并改，
    改的时候连 `wall_ms` / `prompt_bytes` 一起看一眼 —— 那两列在失败轮上
    是真实测量值（Runner 自己计时、prompt 是真送出去的字节），
    不在此列，别顺手一起改掉。

    在此之前，读 `cold_start` 的任何一侧都要先看 `outcome`：
    非 `OutcomeSpoke` / `OutcomePostRejected` 的那几行，这一列没有意义。

24. **`/task`（8c-4）到底还值不值得做** —— **已裁：做，但定位收窄成「补丁草案」；2026-09-29 Griffin：「/task 暂时不要了」，暂缓**
    （2026-09-22 裁。合议：Claude + Codex gpt-6-astra/max 两份，agy 与 grok 本轮缺席）。
    **暂缓说明（2026-09-29，主代理记）**：下面的裁决内容不变，只是不排期；#29 的收集路径加固方案跟着一起搁置。`11d` 是冻结契约，不改，暂缓只记在这里。要重新启动，先问 Griffin。

    背景是 5.6 实测的那条结构性取舍：**要写入边界就必须加 `--restricted`，
    加了它 agent 就不能执行任何命令**（见 [[restricted-flag-removes-bash]] 与
    `11d` 5.6）。于是 `/task` 的产出是一份**没有任何一层执行过**的 diff。

    **裁法**：照 (a) 做 —— `/task` 交付「一份可审阅、有出处、保住主仓库的补丁草案」。
    「未跑过验证」这件事必须**逐条进每一条结果与 `/task show`**，
    只写进帮助文本不够。不提供 `/task apply`（5.5 原裁法不变）。

    **被否掉的中间路 (c)「Council 收完 diff 自己跑一条配置好的命令」**，
    理由不是我原先想的那两条（clone 不一定能构建、超时常数没标定）——
    那两条都成立但不致命。致命的是 Codex 给的这条：

    > 把命令挪到 Council 这一侧**并不自动获得安全性**：`go test` 执行的正是
    > **模型改过的测试代码**，而现有材料没给这个宿主侧执行器定义任何隔离边界。

    也就是说 (c) 等于让 Council **无沙箱地执行模型产出的代码**，
    而 Council 正是那个「不能给它任意写盘档」的编排器。
    同一条纪律此前只用在了写盘上，没用在执行上。
    要做 (c) 必须先有一份隔离验证器的契约，那是独立一片，不塞进 8c-4。

    **本次合议纠正了我的一条错误论据**：我说选 (a)「零契约改动」——
    **不成立**。`11d` 里有两处 needsProbe #3 之后该订正而没订正的残留
    （5.6 的作废处方、5.7 第 15 条的「编辑-运行-再编辑」），两处都已核实并订正。

    **8c-4 因此多一条硬前置**：needsProbe #3b，见 `11d` 5.6 —— 收集路径
    （Council 自己跑 `git add -A`）可能被模型用 `.gitattributes` + `.git/config`
    的 filter 驱动劫持成执行入口。在证明收集路径安全之前不许落地 8c-4。


31. **`--restricted` 那次 A/B 的两组默认模型不同，结论要复测** —— **待测**
    （原本误编为 25，与下面的 `r.cfg` 那条撞号。2026-09-22 发现时，仓库内外全部对「#25」的引用指的都是 `r.cfg` 那条，所以改的是本条。位置保持原样不挪，只改号。）
    （2026-09-22 记，Codex 复核点出，核对属实）。

    `oq-task-git\RESULTS.md:74` 我自己记了这一条：A 组是 `claude-fable-5-1`，
    B 组是 `claude-opus-5`，因为 `--restricted` 会忽略 settings 文件而那里钉着模型。
    原文接着论证「不影响本次结论（工具列表的 A/B 只改了一个 flag）」。

    **那个论证滑了一下**：flag 是唯一**主动**改的东西，模型是它的**副作用**跟着变的。
    严格说那一次是两个变量。结论几乎肯定仍然成立（工具列表归 CLI flag 管、
    不归模型管），但「几乎肯定」不是「测过」，而本项目整条纪律就是不许这么放过 ——
    尤其这条结论是 `/task` 整个设计的地基。

    **复测方法**：两组都显式发同一个 `--model`，其余一字不改，重跑一次工具清单。
    成本接近零。在复测之前，`restricted-flag-removes-bash` 那条结论按
    **「已验证一次、存在一个未控变量」** 记，不按「已证实」记。

25. **`r.cfg` 在拆闸之后是裸读裸写** —— **已知缺口，Step 9 片 1 明确不修**
    （2026-09-22，片 1 落地时由实现方与主代理各自点名）。

    片 1 把 `Runner.mu` 拆成按 `sessionKey` 的闸之后，**结构上已经允许并发 `Ask`**。
    `Ask`/`AskUpTo` 会读 `r.cfg.Agents[agentID]`，而 `SetConfig`
    （`/config reload --apply`，预览路径还会换上去再换回来）**裸写** `r.cfg` 这个指针。
    `r.rules` 这一半由本片新加的 `rulesMu` 守住了，`r.cfg` 这一半没有。

    **为什么不顺手一起加**：`SetConfig` 的注释是冻结说明的一部分
    （「不加锁：Runner 的全部方法本来就只从 REPL 那一条 goroutine 调用」），
    改它是契约面动作，要留订正记录；而且要做对不止是加一把 `cfgMu`——
    与 `rules` 一样，**快照粒度必须是轮**（`RunRound` 进来取一次，
    传给每个 `AskUpTo`），否则同一轮里两个成员用了两份配置，
    指纹会对不上，而漂移只在下一轮才被发现。那是一片的工作量，不是一行。

    **今天不可达**：REPL 的命令派发仍是单 goroutine 串行，`/round`（8c-2）
    也仍是串行扇出。**片 3 真正并发扇出之前必须补上**——
    片 3 是第一个会让两条 goroutine 同时进 `Ask` 的调用方。

    代码里已经留了话：`runner.go` 的 `SetConfig` 注释点名这是已知缺口。

26. **`RulesReport()` 的规则快照粒度是「次」不是「一次输出」** —— **已知窄缝，不修**
    （2026-09-22，片 1 落地后主代理审查产出时核出）。

    `rulesMu` 的第五个点落在 `probeHeadroom` 上，而 `RulesReport()` 按
    (provider, tier) **逐行**调 `probeHeadroom`，每行各取一次读锁。
    与 `SetRules` 并发时，同一张 `/rules` 表的两行可能算自两份不同的规则文本；
    `/rules` 同时调的 `Rules()` 显示的那一份也可能与表里的行不一致。

    **与 `upTo`/`rules` 快照那条原则是同一条**（「粒度是轮不是次」），
    正解是 `RulesReport()` 进来取一次快照往下传。

    **不修的理由**：它只影响**给人看的显示** —— 不进指纹、不进 argv、不花钱、
    不改任何持久状态。片 1 的范围是拆锁，不扩。
    与 #25 一起处理；两条都要在片 3 真正并发扇出之前重新过一遍。

27. **`/round` 的冷启动预报在 `InFlight` 时是拿上一轮的时刻算的** —— **片 3 做了一半，仍开着**（2026-09-22 复核）

    **现查**：片 3d 的 `0a084ca` 给 `/round` 的预算表加了一列在飞
    （`internal/repl/round.go` 的 `budgetFlight`，印「在飞」/「不在飞」）。
    **但预报本身仍照常打印**（`budgetCold` 不看 `InFlight`），
    没按下面说的「对在飞成员印『在飞，预报不可用』」做。
    于是用户看到的是「在飞」与「预报非冷」**并列两列**。

    **仍不可达，所以不算缺陷**：片 3d 的 `--rounds N` 是**串行**的
    （上一轮收完才发下一轮），预报点仍然碰不到本进程自己在飞的成员。
    **触发条件因此收窄成一条**：哪天允许轮次**重叠**
    （第 N 轮还在跑就发第 N+1 轮），这一条立刻变成真缺陷，
    那时要么让预报对在飞成员收声，要么明确裁决重叠轮次不允许。

    以下为当时的原文：

27a. **原文**（片 1 不修，片 3 必须回头看）
    （2026-09-22，片 1 补 `/session` 的 `busy` 列时由实现方（grok）点名，主代理采信）。

    `round.go` 的预报只读 `ok` 与 `info.ColdExpected`，不看 `InFlight`。
    `InFlight == true` 时 `LastTurnAt` 是库里**上一次提交**的值,
    `ColdExpected` 是拿那个旧时刻算的 —— 预报的是「上一轮之后冷不冷」,
    不是「这一轮之后冷不冷」。

    **片 1 不算缺陷，理由（实现方给的，主代理核过）**：`/round` 的时序是
    「先对整份名单做完 `SessionInfo` 预报，再决定要不要 `--go`」。
    今天 REPL 派发串行，预报这一刻这条 (会话, 成员) 不可能正被本进程的 `Ask`
    占着闸；片 3 的并发扇出也是「先预报、再对**不同成员**并发 `Ask`」,
    同一成员仍由闸串行，预报时它自己还没起飞。
    `InFlight` 在预报点为真，只可能来自**另一条调用方**已经占着同一把闸。

    **那正是片 3 要处理的那件事**：一旦允许轮次重叠（第 N 轮还在跑就发第 N+1 轮）,
    预报就会撞上在飞的成员，而此时它报出来的 `cold` / `warm` 是过期的。
    与 `/session` 不同，这里**没有死键问题**（`InFlight` 已有 `/session` 这个消费方）,
    所以不为它现在造分支；但片 3 开工时这一条必须重新过一遍,
    要么让预报对在飞成员印「在飞，预报不可用」，要么明确裁决重叠轮次不允许。

28. **同一段 claude 成功样本的字节，全仓有四份独立副本** —— **只记不修**
    （2026-09-22，片 2a 期间给 `/session` 的 `busy` 列补用例时由测试方点名）。

    `internal/provider/fixtures_test.go`、`internal/turn/fixtures_test.go`、
    `internal/app/smoke_test.go`、`internal/repl/session_inflight_test.go`
    各持一份**逐字节相同**的 claude 成功 JSON。

    **为什么会这样**：Go 的 `_test.go` 不能跨包复用，而这四个包都需要一份
    「真机抓过的、能过 `ParseTurn` 那道 fail-closed 五字段判据」的样本。
    契约 13 §6.1 明写「那份 JSON 要从既有的成功样本**原样搬字节**，不要手写」——
    手写的形状正是这类用例会烂掉的地方。所以复制是对的，独立副本是代价。

    **会怎么咬人**：哪天重新抓包（provider 改了输出形状、加了字段），
    四处要一起改。**漏改一处不会有任何一层报错** —— 那个包的用例继续用旧形状
    跑绿，而生产代码已经按新形状解析，两者的分歧要等真机才暴露。

    **不修的理由**：正解是把样本挪进一个**非测试**的包（例如
    `internal/provider/testdata` 加一个导出的 `SampleClaudeSuccess`），
    但那是往生产代码里塞一个只有测试用的导出口子，与「不交付死键」相抵。
    另一条是 `//go:embed` 一份 `testdata/*.json`，四个包各自 embed 同一个文件——
    这条可行且不脏生产代码，**但要先确认四份副本今天确实逐字节相同**
    （本条只核过它们都源自同一份，没有逐字节 diff 过四份）。

    **在此之前的纪律**：重新抓包时 `grep -rln` 找全四处，一次改完再跑全量。

29. **8c-4 的硬前置已解除：收集路径加固方案已裁并有双探针支撑** —— **裁决已定，实现未落；2026-09-29 随 #24 暂缓**
    （2026-09-22。方案与全部证据见 `11d` 5.4a。）

    **原阻塞**：`11d` 5.4 让 Council 在副本里跑 `git add -A` / `git diff --cached`,
    而 git 的 filter / textconv 由 `.gitattributes`（声明）+ config（命令）两半组成,
    **两半都是模型用 Edit / Write 就写得出的普通文件**。第一个探针
    （`oq-task-git-filter`）实测两条命令各自都执行了模型给的命令、**一条错误都没报**。

    **裁决：修法 3 —— 收集时把 git 目录放到工作区之外，并关掉全局与系统配置。**

    ```
    git --git-dir=<dataDir>/tasks/<taskID>/gitdir --work-tree=<workspace> add -A
    git --git-dir=<dataDir>/tasks/<taskID>/gitdir --work-tree=<workspace> diff --cached
    环境：GIT_CONFIG_NOSYSTEM=1  GIT_CONFIG_GLOBAL=<空文件>
    ```

    承重的是**一条结构性事实，不是枚举**：filter 的命令必须来自 config，
    而 config 归我们。工作树里的 `.gitattributes` 指向未定义驱动时 git 直接忽略
    （实测：rc=0、不报错、内容 `cmp` 一致）。

    **为什么否掉「收集前校验 `.git` 未被动」**：第二个探针
    （`oq-task-git-hardening`）新探出**两个原先清单里没有的执行面** ——
    **任意子目录里的 `.gitattributes`** 与 **`.git/info/attributes`**，
    两者都会让 `add` 与 `diff` 执行命令。按直觉实现的校验（根 `.gitattributes`
    + `.git/config`）两条都绕得过去。而**前一个探针找到 2 个面、这个又找到 2 个** ——
    它证明的不是「只剩这两个」，而是**枚举本身不可靠**。校验是白名单式安全。

    **修法 3 相对「干净 clone」的结构性优势**：工作区里**根本没有 `.git`**,
    于是 `.git/info/attributes` 与 `.git/hooks/` 从「要检查」变成「写不出来」。
    附带好处是不用拷整棵树。

    **全局 gitconfig 是独立的一条，双向验过**：不加那两个环境变量时，
    用户自己 `~/.gitconfig` 里的 filter 定义**照样被执行**（HIT/HIT）；
    加上之后 NO/NO。**它们不是可选项。**

    **本条只是裁决，8c-4 的实现仍未落地。** 落地时连带要做的三件：
    工作区创建把 git 目录分出去、`/task drop` 两处都删、
    以及 `11d` 5.4 那句「Council 是普通进程、不受限制」作为安全理由的撤回
    （已写进 5.4a）。

    **落地时不要写成已验证的三条**（`11d` 5.4a 末尾有同一份清单）：
    `GIT_CONFIG_NOSYSTEM` 在 Windows 上的覆盖面没单独测；
    `.gitmodules` / 符号链接 / `core.attributesFile` 指进工作树没测；
    `--separate-git-dir` 在工作树里留下的那个 `.git` **文件**可写性没测。

30. **`RunRoundWith` / `BudgetWith` 把铸 UUID 与分配 `out` 推给了调用方** —— **已裁（2026-09-22）：选 (b)，片 3d 落地前加 `RunRoundPrepared`**

    片 3c 为了让「看过价再决定起不起进程」成立，把快照从 `RunRound` 里提了出来：

    ```go
    func (r *Runner) TakeRoundSnap(ctx, convID) (RoundSnap, error)
    func (r *Runner) BudgetWith(ctx, convID, plan, snap) ([]BudgetRow, error)
    func (r *Runner) RunRoundWith(ctx, convID, plan, maxParallel, roundID string, out []Record, snap RoundSnap) []Record
    ```

    公开的 `Budget` 与 `RunRound` 仍各自 `TakeRoundSnap` 一次，签名没变，
    既有调用方不受影响。但**片 3d 的 `/round` 要走 `*With` 这一支**
    （否则预报与执行各拍一次快照，中间的 `SetRules` / `SetConfig` 会让
    报出去的价与真正送出去的前缀不是同一段），于是 3d 必须自己：

    - 铸 `roundID`（v4 UUID），
    - 预先分配并填好 `out []Record`（`RunRound` 原本在铸 id 那一步就把
      `out` 按 `plan.Speakers` 建好了，失败时好把错误逐行塞进去）。

    **这是 3d 要背的 API 代价，先记下来再写 3d 的规格。** 两条路：
    (a) 3d 照此调用，多写十来行铺垫；
    (b) 再加一个 `RunRoundPrepared(ctx, convID, plan, maxParallel, snap)`
    把铸 id 与分配 `out` 收回 turn 包，3d 只传 snap。

    (b) 更干净，但会让「一轮一个 roundID」这件事有两个入口，
    而 `roundID` 同时是 `Record.RoundID` 与两条面包屑的键。
    **写 3d 规格时一并裁，不要落地之后再改签名。**

    ### 裁决（2026-09-22，查过 `RunRound` 的错误填充路径之后）

    **选 (b)。** 依据是现有 `RunRound` 开头那段：

    ```go
    n := len(plan.Speakers)
    out := make([]Record, n)
    for i, id := range plan.Speakers {
        out[i] = Record{ConvID: convID, AgentID: id, Outcome: OutcomeRefused}
    }
    roundID, mintErr := newV4UUID()
    if mintErr != nil { for i := range out { out[i].Err = ... }; return out }
    for i := range out { out[i].RoundID = roundID }
    ```

    调用 `RunRoundWith` 的人必须同时知道**四件包内约定**：
    `out` 的长度等于 `len(plan.Speakers)`；每行预填 `ConvID` 与 `AgentID`、
    且顺序与 `Speakers` 一致；**每行的默认结局是 `OutcomeRefused`**；
    出错时逐行塞 `.Err` 再整体返回。

    让 `internal/repl` 复制这四条，等于把「一个从没跑过的 Record 默认是 refused」
    这条不变量放进两个包。那是 `internal/turn` 自己的知识，漏出去以后
    两边任何一处改了，另一处都不会跟着改。

    **原本反对 (b) 的理由（「一轮一个 roundID 会有两个入口」）站不住**：
    加了 `RunRoundPrepared` 之后，铸 id 只剩它**一处**，
    `RunRound` 退化成 `TakeRoundSnap` + `RunRoundPrepared` 的薄包装。
    入口是**减少**了，不是增加。

    **片 3d 落地前要加的签名**：

    ```go
    func (r *Runner) RunRoundPrepared(ctx context.Context, convID string, plan Plan, maxParallel int, snap RoundSnap) []Record
    ```

    它铸 `roundID`、分配并预填 `out`、再转给 `RunRoundWith`。
    `/round` 于是只需：`TakeRoundSnap` 一次 -> `BudgetWith(snap)` -> `RunRoundPrepared(snap)`，
    预报与执行共用同一份快照，中间的 `SetRules` / `SetConfig` 撬不开这条缝。


32. **片 3b 把快照挪到取闸之前，改变的是「用户什么时候被告知」** —— **已核，不改行为，记在这里等 3d 复核**

    **评审最初把它定性成「绕过 identityFP 漂移的 fail-closed 拒绝」。那个定性是错的**，
    2026-09-22 核实后改判。经过写在这里，因为下次再看到这段代码的人会重走一遍同样的推理。

    **事实**：`AskUpTo` / `Ask` 现在在 `g.mu.Lock()` **之前**拍规则与配置快照。
    于是同一 `(conv, agent)` 上排队等闸的调用方，整轮用的是**它入队那一刻**的快照。
    期间的 `SetRules` / `SetConfig --apply` 对这一轮不生效。

    **为什么不是安全漏洞**：旧快照算出**旧**指纹，与 `g.st.identityFP` **相符**，
    于是 `st.identityFP != fp` 那道拒绝**不触发**。但这一轮是自洽的 ——
    旧规则配旧前缀，`--resume` 接的就是按旧身份建好的那条会话。
    那道拒绝自己写的理由是「静默重建会白烧一次入场费（0.4 到 1.0 美元）且他不知情」，
    而这条路径**一分钱都不烧**。**既定目的没有被违反。**

    **真正变了的**：用户的 reload 对那一轮静默地不生效，`turn_usage.identity_fp`
    落库为旧值，而且**不报 drift**。变的是告知时机，不是安全性。

    **这是契约要求的设计，不是实现擅自改的**：契约 2.6 原文就是「一进来取一次」，
    挪到取闸之前还顺手消掉了 `g.mu` -> `rulesMu`/`cfgMu` 的嵌套，
    而全局锁顺序正在意这个。片 3b 的实现方在**交付报告**里写明了这个代价 ——
    主代理把那句话误当成了代码注释，写进 `dba2e0f` 的 commit message
    （「代价写进了注释」），那是一句假披露。现已把代价真的写进注释。

    **今天不可达**：一轮之内每个 `(conv, agent)` 只出现一次，
    两个 goroutine 不会争同一把闸；REPL 也是串行派发。

    **3d 要复核什么**：`/round --rounds N` 会让同一个 `(conv, agent)`
    在**连续多轮**里反复进闸。那时窗口仍不可达（轮与轮之间是串行的），
    但如果 3d 或以后让轮次重叠，这一条就变成可达，
    届时要裁的是「排队者该用哪一份快照」，不是「要不要拒绝」。


33. **`phase = 'spawned'` 写在 `r.spawn` 之前，不是之后** —— 契约订正，2026-09-22

    `13_STEP10_CONTRACT.md` 3.3 的表格与 3.15 第 3 条原本写「`r.spawn` **返回之后**
    立刻写 `spawned`」。**这条与同一节的验收 14 / 15 正面冲突**，A 片之三开工前订正，
    经过见该文件新增的 3.3a。

    **根因是一句读错了的代码**：`spawnProcess`（`internal/turn/spawn.go:97`）在
    同一次调用里 `exec.Command` -> `Start` -> `Wait`。`r.spawn` 不是「把进程起起来」，
    是「起起来**并等它跑完**」。于是「返回之后才写 `spawned`」把**整个子进程生命期**
    （硬超时 5 分钟）留在了 `begun` 这一档里 —— 而 `begun` 被读作
    「进程没起，不标 `baseline_suspect`、不发面包屑」。

    **失败方向是静默少报**：验收 14 说的正是「子进程还在跑时 `taskkill /F` council，
    重启后断言 `baseline_suspect == 1`」，按原文必然判成「没花钱」，
    下一笔金额会被自信地算出来 —— 恰好是契约第 2 节要修的那个洞本身。

    **裁决**：`spawned` 写在 `r.spawn` 调用之前（`spawnCtx` 取到之后）。
    代价是 `ErrCommandLineTooLong` / `ErrShimNewlineArg` 这两条
    「在 `exec.Command` 之前返回」的路径会短暂顶着 `spawned`；
    它们正常走 `refuse` -> `ClearInflight` 把行删掉，
    **只有清理也失败的双重故障**才漏得下一条假阳性。

    **这笔交换的方向是有意的**：宁可在双重故障时多说一次「可能花了钱」，
    不可在单次崩溃时少说一次。把 argv 预检从 `spawnProcess` 提到 Runner 侧、
    在 `spawned` 之前跑，可以把假阳性也消掉，记为剩余项，不在本片做。

    **两条都改了，不只改一条**：3.3 的表格与 3.15 第 3 条的调用序列
    （现在是 `BeginTurn -> phase=spawned -> spawn -> phase=returned -> ...`）。
    背对背的两条腿从同一份契约工作，只改任务书不改契约，
    测试腿会照旧序列写，红出来会长得像实现的 bug。

34. **TUI 的规格换成 Griffin 那份 27 节文档；三节做不到的照实拒付** —— 2026-09-23

    D-11「不做 TUI」先被推翻（`f6e4023`），随后的第一版设计（N 行计时板当整个 TUI）
    也被 Griffin 停掉，换成他交来的 27 节 TUI 规格。映射见 `15_TUI_SPEC_MAPPING.md`。

    **上一版调研没有作废**：被停的是「TUI 长什么样」，六路调研交付的是
    「怎么把 TUI 装进这个仓库不炸掉 126 个用例」——选框架、三条缝、两个探针、
    183 处输出漏斗。后者压在前者下面，两份都在用。

    **规格有三节 Council 结构性做不到，照实不做并写明理由，不假装实现**：

    - **流式（§13）**。`spawn.go:155-181` 是 `bytes.Buffer` + `Start` + `Wait`，
      三家 CLI 一律 headless 一次性调用，**进程退出之后才解析**。
      规格列的九个状态里 `thinking` / `streaming` / `tool-running` / `waiting`
      四个在 Council 的数据里**不存在**。板上画一个观测不到的状态就是编。
      画得出来的是 running（活计时器）到终态，以及 Council 比参照物多出来的
      三档「花了钱但没拿到东西」（PostRejected / MaybeAdvanced / Drifted）。
    - **工具活动树（§14）**。同一条根因：拿到的是终稿 JSON，没有工具事件流。
      三家的 JSON 里有没有可复原的工具轨迹 **[未验证]**，查证之前不画。
    - **补丁审批流（§15）**。规格说「preserve any existing patch approval workflow
      if one already exists」—— **不存在**。Council 的 agent 在自己的 workdir 里
      按权限档直接干活，没有 propose-then-apply 这一层。

    **规格与仓库的一处命名正交，任务书必须带**：规格的 Role（planner / reviewer /
    judge）= Council 的 **`Mode`**；Council 自己的 `Role` 是身份块里的 system prompt
    正文。不点破，实现方会造第二个 Role 类型。

    附带一条：`Mode` 不进身份指纹、`Role` 进（`schema.go:104-112` 已裁），
    所以规格 §26 Scenario C（Judge 换人不必重编译）**今天就成立**。

    **一处我自己判错又自己纠正的**：我按 `scheduler.go:39` 那句「不跑 plan.Judges」
    报了「Judge 后端没接上」。那句说的是 `RunRoundWith` 这一层不跑；
    `--with-judge` 接在上一层的 `/round`（`repl/round.go:217-236`，
    发言轮跑完后另取快照、`Speakers: plan.Judges` 另起一轮）。
    **Judge 执行早就有了，缺的只是选择器 UI。**

35. **两条已落地代码里的真缺陷，grok 在 D 片之一交付时报的，本会话核实** —— 2026-09-23，**读侧已修：(a) `e695ede`、(b) `4f01dc7`；写侧仍是剩余项**

    ### (a) `lostUsage` 把 `RoundID` 写成空串，Lost 行在按轮看账时整批消失

    `internal/turn/recover.go:212` 写 `RoundID: ""`；`/cost --round <id>`
    （`internal/repl/cost.go:214-222` 的 `filterCostByRound`）按字符串相等过滤。
    于是崩溃对账补上的那些轮**不带 `--round` 时在 Lost 栏里，带了就整批不见**。
    同一批数据两种口径。

    **根因不是写错，是数据源里没有**：`conversation.InflightTurn`
    （`types.go:214-227`）十二个字段里**根本没有 `RoundID`**。
    对账只读得到 inflight 行，那里没记过轮。所以 `""` 是当下唯一诚实的值。

    **真正要裁的是读的那一侧**：`--round` 应该把「轮未知」的行**点名报出来**
    （例如末行加一句「另有 N 行轮次未知，不计入本轮」），而不是静默滤掉。
    静默滤掉正是本项目一路在防的那件事 —— 把「不知道」写成「没有」。

    修 `InflightTurn` 加一列是另一回事，量级大得多（要改 schema、写入点、对账），
    而且它只在崩溃补账这一条路上有用。**本条先修读侧，写侧记为剩余项。**

    **已修（2026-09-23，`e695ede`，X-35a）**：`/cost --round` 在表后点名报出 `round_id` 为空的行数
    （其中几行是崩溃后对账补的账也分开说），表里一行都没有时区分「本会话还没有记过账」与「这一轮没有记账行」。
    写侧（`InflightTurn` 加 `RoundID`）仍没做。

    ### (b) `pickUsageSample` 读 `cold_start` 不按 `outcome` 过滤 —— 违反未决 #23

    `internal/turn/budget.go:276`：`if cache == provider.SupportYes && u.ColdStart != cold { continue }`。

    未决 #23 裁的是：`cold_start` 列是 `INTEGER NOT NULL DEFAULT 0`，
    **一轮没跑完时冷暖本来不知道，而列不可空，false 只能存成 0**；
    所以只有 `Outcome.Ingested()` 为真的行，这一列才是实测值。
    我在 #23 的裁决里写了「`/cost` 是它唯一的执行者」—— **那句是错的**，
    `pickUsageSample` 从一开始就是第二个读者。

    **这条今天可达**：`UsageSamples` 的 SQL（`read.go:264`）只要求
    `input_tokens IS NOT NULL`，不看 outcome。而 `OutcomeProviderFailed` 的行
    **可以**有非 nil 的 `input_tokens` —— 适配器契约明写「provider 自报失败 ->
    包 ErrTurnFailed，StopRaw 与 Usage 仍尽量填上」（`provider.go` BuildTurn 注释）。
    这种行的 `cold_start` 存的是那个没有含义的 0，`cold=false` 的取样会把它
    当成一个确凿的「暖样本」选中，拿去缩放下一轮的预报。

    **注意 `budget.go:263-265` 那段既有注释不是在为这件事辩护**：
    它说的是「不另滤 outcome」用于**样本资格**（4.5 那句「成功轮」以样本口径为准，
    `input_tokens` 非 NULL 即算）。那是另一个问题。
    冷暖列在非 Ingested 行上没有含义，是 #23 之后才裁清楚的，
    这段注释写在那之前。**机制与后果要分开判：注释辩护的机制成立，缺口仍在。**

    **修法**：`SupportYes` 那一支在比 `ColdStart` 之前先要求
    `turn.Outcome(u.Outcome).Ingested()`。`SupportNo` 那一支不动
    （它根本不读 `cold_start`）。

    两条都没修。它们与 TUI 无关，排在 TUI 第 1 刀之后单独一刀，不混进界面改动里。

    **订正（2026-09-23，(b) 已由 X-35b 修掉）**：上面那段「修法」是**错的，只修了一半**。
    派工前第二轮审查（W1b）指出：失败轮的用量是**货真价实的 0**（`provider.go:279-280` 的失败探针），
    `SupportNo` 那一支同样会选中它、把预报缩成 0 token 并标成已标定 —— 问题从来不只是冷暖，是「失败轮不能当样本」。
    落地的修法是**两支都要求 `Ingested()`**，恢复 Step 9 契约「成功轮」的本意（PostRejected 仍算）。
    我在本条原文里写「`SupportNo` 不动（它根本不读 `cold_start`）」—— 那是只看了冷暖、没看用量。
    另：我曾对 Griffin 说预报会被缩成「$0」，也不对 —— 失败轮的 `CostUSD` 是 nil（`computeCostDelta` 只在成功路径 `runner.go:733`），
    金额那一格仍是「未标定」，被缩成 0 的只是 token 数。实现腿（grok）发现了这一点，没有把错话照抄进源码。

36. **派工前对抗审查（W1）挖出、不在本批三刀范围内的真问题** —— 2026-09-23，**(a)(b) 已修（X-36ab，`09bf271`）；(c)(f)(g) 未修；(d) 作废；(e) 已在 `4f01dc7` 照做**

    本批（UX-2 修订版、未决 35 的 a 与 b）派工前，七个只读审查员审了三份冻结契约。
    契约内的问题已全部改进契约第二版；下面这些在契约之外，记在这里免得只活在一份 workflow 的 JSON 里。

    **(a) 失败轮的「实测=暖」是编的。** `internal/repl/ask.go` 的 `writeAskMeter` 只对 Refused / Unknown 提前返回（`:306`），
    对 ProviderFailed 的轮照样打「冷暖 预报=… 实测=暖」（`:339-343`）。而那一轮的 `rec.Cold` 从来没被计算过：
    `runner.go:702` 吸收用量之后 `:704` 失败返回，`:737` 的 `rec.Cold = rec.Usage.CacheRead == 0` 走不到。
    **这是一句屏幕上的假话**，与未决 35(b) 同一个病根。修法：非 Ingested 的轮打「实测=未知」。

    **(b) 轮结算的冷启动计数是下界。** `internal/turn/scheduler.go:350` 的 `Settle` 只要求 `UsageParsed`，
    被吸收了用量的失败轮正好过得了这道门，读到零值 `Cold=false`。方向只会少数、不会多数，
    但「cold=N」读起来像精确值。修法要给结算加一个「冷暖未知」计数，改动面包括 `round_finished` 的 meta，另开一刀。

    **(a)(b) 已修（2026-09-23，`09bf271`，X-36ab）**：`turn.Record.MeasuredCold()` 是判「冷暖是不是实测值」的入口（Ingested、用量已解析、cache_read 已报三条同时成立）。
    `/ask` 打「实测=未知」；没解析到用量的轮打「用量 未知（没解析到用量）」，不再印全零。
    `Settle` 新增 `ColdUnknown`，数**全部**冷暖不是实测值的成员（含没解析到用量的，与 `usage_unknown` 可以重叠，与 `cost_unreported` 同一种写法），
    `round_finished` 无条件落 `cold_unknown` 键，`/round` 印「实测 N（k 名成员冷暖未知，只数其余人）」。写侧（`runner.go:737`、`cold_start`）按设计不动。
    上面 (a) 说的修法「非 Ingested 的轮打实测=未知」照做了；(b) 说的「另开一刀」就是这一刀。

    **(c) `created_at` 的文本序不是时间序。** `internal/store/open.go:210` 的 `formatTime` 是 `t.UTC().Format(time.RFC3339Nano)`，
    RFC3339Nano 会去掉小数尾零，于是同一秒内 `…:03.5Z` 在文本上大于 `…:03.55Z`（`Z` 的码位大于数字），时间上却更早；
    整秒的时间戳连小数点都不写，排序同样错。`internal/store/read.go` 里五条 `ORDER BY created_at`（`:34` 会话列表、
    `:265` `UsageSamples`、`:328` `:330` `:336` `TurnUsages`）在同一秒内都可能乱序。
    影响小（排序，不影响任何求和），但 `UsageSamples` 取的是「倒序第一条合格的」，可能取到一条稍旧的样本。
    修法要换成定宽格式**并迁移已有行**（新旧格式混排仍会乱），另开一刀。

    **(d) ~~未决 #23 三列条款的一个有意例外~~ —— 作废（2026-09-23）。** 这一条写于 X-35b 契约第二版：当时打算让 `SupportNo` 支继续读失败轮、
    并把它读 `prompt_bytes` 写成「对 #23 的有意例外」。第二轮审查指出失败轮的用量是真实的 0、根本不能当样本，
    契约第三版改成两支都要求 `Ingested()`。于是 `pickUsageSample` 不再读任何非 Ingested 行，**不需要例外，#23 原文不改**。

    **(e) X-35b 取代 `12_STEP9_CONTRACT.md` §4.5a 第四条对 `SupportYes` 两支的「三支统一」。**
    `internal/turn/step9p3c_budget_usage_test.go:953`、`:1039-1041` 的注释会因此过期；契约禁止实现腿与测试腿改既有 `_test.go`，
    **由主代理在落地时改注释**（只改注释，不动断言）。

    **(f) 其余取 agent id 的命令仍只认全名。** UX-2 之后 `/ask`、`/add`、`/join` 接受前缀（`/add` 还接受编号），
    而 `/leave`、`/session rebuild`、`/cost <agent-id>`、`/round` 的显式名单、`/to` 仍只认全名。都是 fail-closed（认不出就拒绝，不花钱），
    不一致但无害。统一留给后面单独一刀，UX-2 契约写明了范围边界，防止实现腿为「一致性」越界。

    **(g) [未核实] claude CLI 按 cwd 派生项目目录名时会把非字母数字字符规整成 `-`**（本机可见 `_` 被换成 `-`）。
    点号是否同样被规整、`workdir\grok-4.7` 与 `workdir\grok-4-7` 会不会落进同一个 claude 项目目录，没测。
    同类碰撞此前已存在于 `_` 与 `-` 之间，会话按 UUID 区分，`--resume` 大概率不受影响。

    **补记（2026-09-23）：点号确实被换成 `-`**。证据是现成的目录名，不是专门做的实验：数据目录 `C:\Users\25142\.council\data\workdir\op`
    在 `~/.claude/projects` 下对应的是 `C--Users-25142--council-data-workdir-op` —— `.council` 的点号变成了 `-`。
    所以 `workdir\opus-5.0` 会落进 `…-workdir-opus-5-0`。只有「只差点号和连字符」的两个 agent 同时存在才会撞；
    Griffin 的配置改名之后没有连字符版本，`opus-5.0` 与 `opus-5.0-max` 各自落进不同目录，今天撞不上。配置文件里留了一行提醒。

37. **本批三刀落地后的审查与真机使用挖出的剩余项** —— 2026-09-23，**(f) 已照做（改名）；(g) 已修（X-39f，`7d65b96`）；其余未修**

    来源与 #36 不同：#36 是派工前审契约挖出的，这里是落地之后（独立审查员、真机 `/ask`、配置核对）挖出的。

    **(a) 从配置删掉、但仍是成员的 agent，`/ask` 会先打「正在问 X」再被拒。** 真机实测（2026-09-23，遗留成员 `op`）：
    X-1 的成员闸只拦「从没加入过」的 agent；`op` 仍是会话成员，过了闸，打出「正在问」，然后 `Ask` 在起进程之前拒掉。
    **没花钱**，只是先说了一句要做的事、紧接着说做不了。`internal/repl/ask.go` 里原先声称「闸会挡住它」的注释已在 `1464dc4` 订正。
    修法是让闸对「成员但配置里已无此 agent」也提前给一句明白话（`configcmd.go` 的 `goneButStillMembers` 已经在报这种状态）。
    这是行为改动，要契约，没顺手做。

    **(b) `/add` 的一处措辞边角。** 前缀只命中一个「已退出、且配置里已没有」的成员时，会错打「都已经在当前会话里了」。
    不花钱、不改状态，审查员建议可以不补。

    **(c) 测试夹具保真缺口。** UX-2 用例的 fake store 只在新值非空时覆盖 `display_name`，真 store 是无条件覆盖。
    今天没有用例因此给出错的结论，但以后测「刷新快照」时，夹具会比真实现宽容。

    **(d) `/cost` 表后 `tw.Flush()` 的错误返回没有用例担保**（X-35a 独立审查员的 major）。把它改成吞错，
    REPL 会在输出已坏的情况下继续跑，而没有用例会红。改前的 `return tw.Flush()` 同样零覆盖，不是 `e695ede` 引入的回归。
    后果只在输出写失败时出现（管道被关），影响小。

    **(e) 数据目录的错误提示在一种情况下是假话。** `internal/app/app.go:128` 在打不开本地库时说
    「可以在配置文件的 data_dir 里改，或设置环境变量 COUNCIL_DATA_DIR」。而取值顺序是配置优先（`config/paths.go:26`）：
    **配置里已经写了 `data_dir` 时，设环境变量不起作用。** 2026-09-22 我照这句话去设环境变量，没生效。
    修法：配置里写了 `data_dir` 时只说前半句。

    **(f) 改名的代价此刻是零。** 查本机库（只读，2026-09-23）：`participants` 只有遗留的 `op`，`provider_sessions` 只有 `ag` 与 `op`，
    `turn_usage` 只有 `ag` `ok` `op`。配置里六个 agent（`opus-5-0` 等）**一条会话都还没有**。
    所以现在把它们改成带点号的 id（例如 `grok-4.7`、`opus-5.0`）不会让任何 provider 会话作废；第一次问它们本来就要付入场费。
    一旦开始用，改名就要多付一次。**改不改是 Griffin 的决定**，记在这里是因为这个窗口会关。

    **已改（2026-09-23，Griffin 同意）**：`opus-5.0`、`opus-5.0-max`、`fable-5.1`、`fable-5.1-max`、`grok-4.7`，表名加了引号，两个 team 的成员跟着改；
    `agy-flash-high` 没动，agent 的字段一个没动。改前原样在 `C:\Users\25142\.council\config.toml.bak-20260923`。
    改之前又只读查了一次库，窗口还开着。验证：隔离数据目录下加载并 `/add` 实测（`/add opus-5.0` 加入的正是 `opus-5.0`、不是 `-max`；
    `/add op`、`/add fa` 报歧义；`/add gr` 得 `grok-4.7`）；故意写成不加引号的 `[agents.grok-4.7]`，报错给出正确写法；真配置与真数据目录启动一次，六个 agent 全在。
    另派三个只读审查员（文件路径 / 字符串解析 / 外部 CLI 参数与指纹三个角度，共读七十多处）查「带点号的 id 在运行时会不会与连字符写法行为不同」，
    报上来两条，都被独立的反驳者驳回（一条只是报错文案里点号分不清层级；一条是下面的 (h)，与点号无关）。**没有确认的问题。**
    没有发 `/ask`（会起进程花钱）；@mention 带点号的解析只有单测担保，没真机跑。

    **(g) 启动横幅对「配置里已无此 agent」的行说了一句不成立的话。** `internal/app/app.go:265` 的头一行是
    「注意: 有 N 条 provider 会话的身份已变，下次 /ask 会各重建一次（每条一次入场费）」，下面逐行列出原因。
    原因是「配置里已无此 agent」的那几行，下次 `/ask` **不会**重建、也不会花入场费 —— 会被拒（见本条 (a)）；
    只有把那个 agent 加回配置才会重建。2026-09-23 真配置启动时 `ag`、`op` 两行正是这种。
    方向是「把用户吓得以为要花钱」，不是真花钱。头一行的格式是 `11a_STEP8A_API_CONTRACT.md` 冻结的（`:535`），
    `/rules` 里还有一句同样的（`internal/repl/rules.go:86`，契约 `:451`），改要先改契约。

    **(g) 已修（2026-09-24，`7d65b96`，X-39f；#39(f) 是它的重复登记，处置统一记在这里）**。
    查下来范围比这里写的大：AuditDrift 有六种原因，四种（配置里已无此 agent、provider 不可用、权限档不放行、算不出指纹）下次 Ask 都被拒、不花钱；
    同样的错话在启动横幅、`/rules`、`/rules reload` 预览、`--rebuild` 逐行、`/config reload` 预览、`/session rebuild` 成功文案六处。
    `DriftRow` 加了 `Kind` 与 `AskRefused()`，六处一律按它分两类说；Griffin 裁定清理入口放宽 `/session rebuild`（活跃成员，或者本会话精确 id 且
    `Kind == DriftAgentGone`），不加新命令。契约 `11a` 的相关冻结行已订正（§3.5、§5.1、§5.2、§5.3、§6）。
    **真机复验通过（2026-09-24，Griffin，`council 0.2.0-x39f`）**：启动时 `ag`、`op` 只出现在「会被拒、不花钱」那一句、整段没有「入场费」；
    `/rules` 打「按当前配置 /ask 会被拒的 provider 会话: 2 条（不花钱）」；`/session rebuild a` 按前缀被拒、不删行；
    `/session rebuild op`（活跃成员）与 `/session rebuild ag`（非成员，走放宽的闸）都打「已清掉」；重启后漂移提示消失，库里只剩 `grok-4.7` 一行。
    （原复验清单：启动时 `ag`、`op` 只出现在「会被拒、不花钱」那一句；`/session rebuild op`、`/session rebuild ag` 都打「已清掉」；重启后提示消失。）
    同类、本刀没做的剩余项登在 #40。

    **(h) id 可以是 Windows 保留设备名。** `config/validate.go` 的正则放得过 `nul`、`con`、`aux`、`prn`、`com1`、`lpt1`，
    read 档的工作目录是 `filepath.Join(<data_dir>/workdir, agentID)`（`internal/turn/session.go:124-129`），于是这类 id 的工作目录会是一个设备路径。
    后果应当是 fail-closed：`runner.go` 在起进程之前 `MkdirAll`，失败就拒掉这一轮，不花钱；只是报错不会说「这个名字不能用」。
    **这是读代码的推断，没在本机建一个叫 `nul` 的 agent 实测过。**
    **与点号无关**：改点号之前这几个名字就合法。审查员核对了本机（Windows 11 build 26200）上带扩展名的 `nul.1`、`com1.5` 已不算保留名
    （Go 标准库 `internal/filepathlite/path_windows.go:117-120` 的注释说的也是这件事），所以点号没有让问题变多。
    修法：配置校验时拒掉这几个名字，报一句人话。没人会给 agent 起这种名字，优先级最低。

38. **X-36ab 落地后的剩余项** —— 2026-09-23，**(a) 已修（X-38a，`0a41890`）；(b)(d) 已修（X-38bd，`772ce1d`）；(c) 只记不修；(e) 已修（X-38e，`9729e94`）；(f) 未修**

    **(a) `/ask` 用量行不看 `Usage.Unreported`。** `UsageParsed` 为真时，provider 标了未报的字段仍印成数字：
    grok **每个成功轮**都印 `cache_create=0`，库里是 NULL（`grok.go` 给 `CacheCreate` 填 `&0` 同时标未报）；
    usage 对象缺失的成功轮印 `in=0 ... cache_read=0 ...`，下面紧跟 `实测=未知`。X-36ab 契约有意没修（要从 `turn` 包导出一个
    「这个字段报没报」的判据，是第四个改动面）。修法：未报的格子印 `?`，与 `round_finished` 文本里 `shown` 的写法一致。**建议作为下一刀。**

    **已修（2026-09-23，`0a41890`，X-38a）**：`turn.Record.ReportedUsage()` 给出五个指针，nil 读作库里的 NULL；`/ask`（及 `/round` 每个成员的读数）
    NULL 的格子印 `?`，有数印数，五格都有数时整行与之前逐字节相同。与 `makeTurnUsage` 逐列一致由 `x38a_reported_test.go` 拿写库函数当判据扫全部组合钉住。
    **代价**：agy 每一轮的 `cache_create` 从 `-` 变成 `?`。两者在库里都是 NULL；要区分「这家没有这一项」与「这一轮没报」得看 provider 能力声明，没做。
    这一行的 `?` 冻结、验收逐字断言，只对这一行覆盖 `09a` / `08a` 的「占位符不做字面断言」。

    **(b) `/cost` 的冷启动数对「Ingested 但 cache_read 为 NULL」的行仍会数成冷启动。** `internal/repl/cost.go:138` 只按 `Ingested()` 门控，
    usage 对象缺失的成功轮 `cold_start` 落的是 1（`CacheRead == 0` 算出来的），而那一轮的冷暖其实不知道。罕见。
    修它要在读侧加 `CacheRead != nil`，并给那一格加限定语，否则只是把高估换成无声的低估。

    **(c) 老的 `round_finished` 行没有 `cold_unknown` 键。** 事实源只追加，X-36ab 之前的行不会补键。
    **键不存在读作「那一行早于 X-36ab、没记这个数」，不是 0**；那些行里的 `cold` 是下界。今天没有代码读这个键。

    **(d) `/cost` 整列都是 NULL 时印 0。** `internal/repl/cost.go` 累加时跳过 NULL，但每列合计用 `%d` 印，
    「几行用量未知」（`TokensUnknown`）只算不印。于是 grok、agy 的 `cache_create` 那一列永远印 0 —— X-38a 之后 `/ask` 对同一轮印 `?`，
    两处对不上，错的是 `/cost`。X-38a 派工前审查（W1）挖出，契约里我原先写的「/cost 已对」是错的。
    修法：某列一行都没报时印「未知」，有部分行没报时带限定语（与 `/round` 结算的写法一致）。**与 (b) 同在 `/cost`，建议合成下一刀。**

    **(b)(d) 已修（2026-09-23，`772ce1d`，X-38bd）**：
    - Refused 不算适用行（它没起进程）。IN / CACHE_READ / CACHE_CREATE / OUT / COST_USD / COLD 六格在「适用行数 > 0 且这一列一个数都没报」时印 `?`。
    - 冷启动只数「Ingested 且 `cache_read` 非 NULL」的行；其余适用行进新增的 `COLD_UNKNOWN`。
    - 新增 `USAGE_UNKNOWN`：五个 token 列全 NULL 的适用行数（只看 token，不看金额）。
    - **屏幕语义变了**：`COST_UNKNOWN` 从「金额为 NULL 的轮数（含 Refused）」改成只数适用行。
      旧写法下只有 Refused 的成员印「COST_USD 0.00、COST_UNKNOWN 2」，两格自相矛盾。`CostUnknown`、`TokensUnknown` 两个字段语义不动，只是不再印。
    - **局限：部分和不标记。** 同一列在一部分适用行上有数、其余是 NULL 时，合计照印部分和，本格不加记号。
      旁边的 `USAGE_UNKNOWN` 只在五列全 NULL 时才加一，所以「某一列缺、别的列有」的行（例如 grok 的 cache_create）不会被它标出来。
      要逐列标记得给每列加一个计数，表会宽一倍，没做。
    - 只有 Refused 的成员仍印全 0 与 `0.00`：被拒绝的轮什么都没花，这是真的 0。

    **(e) `/round` 的结算对 usage 对象缺失的成员不计 `usage_unknown`。** `Settle` 只在 `UsageParsed` 为假时数 `UsageUnknown`；
    usage 对象缺失的成功轮与 grok 错误信封轮 `UsageParsed` 为真、五个字段全标未报，不进任何合计，也不进 `usage_unknown`。
    于是 `/round` 的合计不带「k 名成员未解析用量」的限定，`round_finished` 写 `usage_unknown=0`。X-38bd 派工前审查（W1）挖出，本刀不修。
    修法：`Settle` 的判据改成与 `/cost` 的 `USAGE_UNKNOWN` 同一种（五个字段全未报），`round_finished` 的键名不变。

    **(e) 已修（2026-09-24，`9729e94`，X-38e）**：判据换成 `rec.ReportedUsage()` 五个指针全 nil，与 `makeTurnUsage` 写库五列全 NULL、
    `/cost` 的 `USAGE_UNKNOWN` 是同一个逐行谓词。注意没有照上面「五个字段全未报」的字面写：那个写法在「四项标未报、CacheCreate 为 nil 但没标」
    这种形状上与库不一致（库里五列全 NULL），X-38e 的扫描用例专门钉着。屏幕限定语改成「（k 名成员用量未知，合计只含其余人）」。
    - **人群差异是设计**：SpawnFailed / OutcomeUnknown / Lost 在 `/cost` 里进 `USAGE_UNKNOWN`，在 `round_finished` 里不进 `usage_unknown`
      （SpawnFailed 另记 `spawn_failed`）。同一轮两处的数可以不同。
    - **老行读法**（照 (c)）：**修复提交之前写的 `round_finished` 行，`usage_unknown` 只数 `UsageParsed` 为假的成员，是下界，不是「其余人都报了」**。
      新老行只能靠提交时间区分，面包屑里没有版本字段。

    **(f) `79547ea`（2026-09-21）之前写的非 Ingested 行，token 列是 `&0` 不是 NULL。** 那次修复之前，Timeout、SpawnFailed、Unparsable、Refused 行的
    in / cache_read / out / thinking 四列写的是 0（cache_create 与金额是 NULL）。只有这类旧行的成员，`/cost` 的 IN / CACHE_READ / OUT 印 `0`、
    `USAGE_UNKNOWN` 也印 `0`，等于说这几轮的用量都报了、都是零。X-38bd 审查实跑：一个成员只有两行旧的 Timeout 与 SpawnFailed，IN 到 COLD_UNKNOWN 九格印 `0 0 ? 0 ? 2 ? 0 2`。
    本刀之前这张表同样印 0，不是本刀引入的退化；但新列让这句假话更具体了。写侧遗留，读侧的 `?` 机制修不了。
    修法：一次性迁移旧行，按 `outcome` 把非 Ingested 行的 `&0` 改回 NULL。要先确认 `79547ea` 之前的 Ingested 行没有同样的问题，另开一刀。
    读侧能管的那一半已由 X-38bd 的 B16 用例钉住（`25141ae`，审查后补）：旧的 Refused 行（`&0`）不许进「报了的轮数」，否则 IN 那一格会从 `?` 变回 `0`。

39. **真机验收（2026-09-23，Griffin 在 Windows Terminal + PowerShell 5.1）挖出的三件事** —— **(a)(b) 已修并真机复验通过（2026-09-23，`0.2.0-x39b`），(c) 随之消失；(d) 已补用例；(e)(f) 复验时新发现，已修（X-39e `4edced9`、X-39f `7d65b96`），2026-09-24 真机复验通过**

    这是第一次在真终端里用 TUI，也是第一次在 Windows Terminal 里用一行模式。自动化测试一件都没抓到。

    **(a) TUI 一启动就死锁。** 敲 `council`，屏幕一片空白，进程 CPU 时间不动；Ctrl-C 能退出。
    `internal/tui/run.go` 的 `runWith` 在 `p.Run()` 之前、同一个 goroutine 上，经 `LineWriter.SetEmit` 同步补吐启动横幅，
    补吐调 `(*tea.Program).Send`，而 `Send` 往无缓冲的 `p.msgs` 发、要等事件循环消费 —— 事件循环要等这一步返回才启动。
    **tui1c 冻结契约的验收 1 把它写成了要求**（「进 Run() 那一刻横幅已经经 Send 交出去」），测试用的假 program 的 `Send` 从不阻塞，所以全绿。
    同源的第二个问题：运行期每行输出在 `Update` 里各返回一个 `tea.Println` Cmd，bubbletea 给每个 Cmd 单开 goroutine，行的到达顺序没有保证。
    修复：契约 `tui1d_startup_contract.md`（启动行在 Run 之前直接写终端；运行期每行作为一条打印消息按序进事件循环；新增一条用**真** tea.Program 的用例）。

    **(a) 已修（2026-09-23，`da88d20`，TUI-1d）**：`runWith` 第 1 步把启动行在 `p.Run()` 之前直写 final（生产上是 stdout）；
    第 1b 步把运行期出口换成 `p.Send(printLine(line))`，`printLine` 当场执行 `tea.Println`，打印消息在 `Update` 之前按到达顺序处理；
    空串换成一个空格（`insertAbove` 丢空串）。`tui1d_startup_test.go` 的 A1、A2 用无终端的真 `tea.Program`，A1 先在旧代码上跑红才算数。
    设计记录见 `15_TUI_SPEC_MAPPING.md` §16。
    **契约勘误**（契约落地后删，勘误留在这里）：§1.1 写「Send 在根 ctx 取消后返回」，机制错了 —— `Send` 等的是 Program 自己的 `p.ctx`，
    正常退出时根 ctx 没取消，是 `Run` 返回前 Program 自己 cancel；结论（退出后不卡住）对。§4 说 M3 由 A2「概率性」杀掉，实测是确定性的（靠空行那一句）；
    A2 顺序那一句单独的判别力用保留空串处理的纯并发变体测，100 次里 9 次红。
    **已知的装置窗口**：A2 在第 1 步补吐完、第 1b 步换出口之前有一个纳秒级窗口，测试若恰好此刻写哨兵，哨兵被直写、不带 `ESC[L`，5 秒后误判红
    （好实现上 53 次一次没撞上）。A2 偶发红而去 ANSI 后哨兵其实在输出里时，是这个窗口，不是死锁回来了。

    **(b) 一行模式（`-no-tui`）里方向键、退格、历史全坏，粘贴的命令不执行。**
    方向键打出 `^[[A`，而且这一行被当成一句话**记进了会话、发给了 grok**；退格一下删掉一整句；
    粘贴 `/new 真机验收` 打出 `^[[200~/new 真机验收^[[201~`，被当成消息而不是命令（粘贴的 `/quit` 也记成了一条消息）。
    原因：`repl.EnablePaste` 为了拿到括号粘贴标记，给输入句柄打开了 `ENABLE_VIRTUAL_TERMINAL_INPUT`，
    这个模式下 Windows 控制台自带的行编辑不再处理方向键，而是把它们翻成转义序列塞进这一行。
    **本条订正 #12 的结论**：#12 的测量是对的（开 VT_INPUT 才拿得到标记），但「所以不必切 raw、不必自己写行编辑器」没有测过副作用 ——
    它从没验过开了 VT_INPUT 之后方向键、退格、历史还灵不灵，也没在 Windows Terminal 里跑过。
    修复方向：一行模式不再开 VT_INPUT（放弃一行模式下的括号粘贴，多行输入用 `/say` … `/end`）；含控制字符的单行不记进会话、不当命令执行。
    主界面是 TUI，它有自己的输入框处理方向键、退格与粘贴。

    **(b) 已修（2026-09-23，`80143b8`，X-39b）**：`main.go` 一行模式不再调 `EnablePaste`（函数已删）；
    `dispatch` 在粘贴、草稿两支之后、剥首尾空白与判空之后、斜杠判断之前加一道闸：`text` 里含控制字符
    （码位 < 0x20 且不是 tab，或 0x7F；按 rune 判；有意放行 C1）时打印「这一行含有控制字符，没有记录，也没有当成命令执行」并返回。
    TUI 的 `Submit` 不接这道闸（它把 0x0a 算作控制字符，TUI 多行提交本来就有换行），注释写在 `seam.go`。
    **没法自动验的**：真正修掉方向键的是删掉那一次调用，`cmd/council` 没有测试，只能读 diff 与真机验收。
    控制台模式由父进程继承：启动 council 的 shell 若自己开着 VT_INPUT，方向键仍会以 ESC 进来，这时这道闸是唯一保障 [未验证]。
    代价见 #12 的订正。

    **(c) 粘贴的斜杠命令不执行**：这是 #12 落地时的有意设计（粘贴块一律当正文，防止多行粘贴里的斜杠行被误执行），不是 bug；
    但单行粘贴一条命令时不符合直觉。(b) 修完之后一行模式里不再有括号粘贴，这一条随之消失；TUI 里粘贴进输入框、按 Enter 才提交，也不涉及。
    **(c) 随 (b) 消失（`80143b8`）** [未在真终端验证]。

    **(d) 新开：`Run` 把根 ctx 交给 Model（`Options.Ctx`）那一行仍没有用例。** `tui1c_wiring_test.go` 当时判「Run 要真句柄，单测进不去」；
    TUI-1d 的审查后补（`460ef10`）用 `os.Pipe` 两端驱动了真 `Run`，证明这个判断不成立。这一行的后果是钱（在飞命令的 ctx 跟不上根 ctx，
    Ctrl-C 之后子进程还在烧 token），值得照同样的办法补一条。未做。

    **(d) 已补用例（2026-09-23）**：`tui1d_run_ctx_test.go` 用管道驱动真 `Run` 与真 `*repl.REPL`（`conversation.NewService` 收导出的 `Store` 接口，
    假 Store 只实现 `Conversations`），经管道提交 `/conv\r` 让命令阻塞在假 Store 里，取消根 ctx 后断言它从 `ctx.Done` 返回 `context.Canceled`。
    `Ctx` 写成 `context.Background()`、`context.WithoutCancel(ctx)`、漏掉这个字段，以及 `replAdapter.Submit` 把 ctx 换成 Background，四个变异都红在主断言上；
    30 次零偶发。判别不了的：交给 `tea.WithContext` 的 ctx 被换掉而 `Options.Ctx` 仍对（用例文件头写明）。

    **教训**：人工真机验收抓到的正是自动化测试结构上看不见的东西 —— 假件比真依赖宽松（a），探针只测了想要的效果没测副作用（b）。

    **真机复验（2026-09-23，Griffin，Windows Terminal + PowerShell 5.1，`council 0.2.0-x39b`，`vcs.revision=677e9e0`）**，16 步全部符合：
    TUI 启动信息立刻出现、在输入框上方；←/→ 移光标、退格一次删一个字、↑ 在单行输入框里无反应；粘贴 `/new 复验TUI` 回车执行；Ctrl-C 退出后终端正常。
    一行模式：粘贴 `/new 复验一行` 直接执行（不再有 `^[[200~`）；退格一个字一个字删、← 不再打出 `^[[D`；↑ 调出控制台历史；
    Ctrl+A 回车打出冻结提示、不记录；`/log` 只有一条。

    **(e) TUI 退出后最后一帧留在屏幕上。** 复验第 8 步：Ctrl-C 之后状态栏（「council 0.2.0-x39b 正在退出」）、分隔线、带行号 `1` 的输入框留在 PowerShell 提示符上方，没被擦掉。
    `run.go` 第 3 步的注释说「内联模式退出时活区被 `\r\x1b[J` 抹掉，最后一帧不留在屏幕上」，与真终端不符；
    TUI-1d 审查员在无终端的真 Program 上看到的是擦掉了（`\x1b[J\x1b[A\x1b[J`），差别可能在真终端的尺寸或退出时的最后一次渲染。未查。

    **(e) 已修（2026-09-24，`4edced9`，X-39e），真机复验通过**（Griffin，`0.2.0-x39f`：空闲 Ctrl-C 与 `/quit` 退出后提示符上方不留表头、横线、`┃`，没有乱码——这台机器的 Windows Terminal + PS 5.1 退出后控制台 VT 位是开的）。病根：bubbletea v2.0.9 正常退出时把最终帧再画一次，`close()` 只擦帧末行，上面 H-1 行留着；
    上面那句「审查员看到擦掉了」不能当对照——无终端时宽度是 0，渲染器一个字都没画。「退出时 View 返回空」在这版 ultraviolet 上无效（内联缩高时光标被静默夹紧），已实测。
    修法（Griffin 选 D）：`runWith` 在 Run 返回之后、换出口与 Flush 之前，按 min(帧高, 真终端高) 补一段 CUU+CR+ED。四道门：Run 返回本包的 Model；
    `err == nil`（正常退出）；Model 收到过真尺寸；final 能解释 VT（只查询 `GetConsoleMode`，不改模式）。
    派工前审查抓到的 blocker：v1 放行「被杀」的 err，而第一次 flush 之前被杀时屏幕上没有帧，清屏会擦掉启动横幅（实测 55/60）——门收紧为只放行 `err == nil`。
    剩余项：(1) 被杀的路（SIGTERM、关窗、tty 读错误）不清屏，残影照旧；要清得先有「帧确实上过屏」的证据，要改 `Run` 的选项。
    (2) ultraviolet 内联缩高时静默夹紧光标（`terminal_renderer.go:1306-1311` 配合 `:457-463`），上游修好之后「退出时 View 返回空」才会生效；
    帧比终端高时，退出前经 `insertAbove` 打出的行会把一行 `┃` 挤进回滚区（上游行为，HEAD 同样）。
    (3) 控制台 VT 位的查询用例（A10b）的杀伤随环境变：经 PowerShell 起的 go test 控制台是 VT 开（0x7），经 node 起是 VT 关（0x3）。
    (4) `internal/app/app.go` 只要 `errors.Is(err, tea.ErrProgramKilled)` 就当正常退出返回 nil，于是 Cmd goroutine 里 panic（Killed+Panic）退出码是 0、也不打「REPL 异常结束」，应先判 `ErrProgramPanic`。

    **(f) 启动时的漂移提示对已删掉的 agent 说「下次 /ask 会各重建一次（每条一次入场费）」。** 会话 `test` 的成员 `ag`、`op` 在配置里已经没有了
    （`配置里已无此 agent`），对它们 `/ask` 会被拒、不花钱；这句提示把「作废」说成了「要付入场费」，而且每次启动都印。
    **是 #37(g) 的重复登记；已修（`7d65b96`，X-39f），处置记在 #37(g)。**

    （复验时 Griffin 贴回来的 TUI 会话 id 与库里不一致，查明是本机环境的问题，与 Council 无关；已单独告知 Griffin。）

40. **X-38e / X-39e / X-39f 这一批（2026-09-24）的审查挖出、不在本批范围的同类问题** —— **全部未修**

    **(a) `/session` 列表的 NEXT 列不看漂移分类。** `cmdSession`（`internal/repl/ask.go`）只看 `SessionInfo`：库里有行印 warm / cold，没有行印 fresh。
    对「已删但仍是活跃成员」的 agent（真机上的 `op`）它暗示下一轮照常跑、会花钱，实际 `/ask` 会被拒；启动后第 5、6 类的行印 warm / cold，而下一轮其实会作废重建、付入场费。

    **(b) 已删但仍是活跃成员、库里没有行（从没开过口）的 agent，`/session rebuild` 仍说「本来就没有开着的会话，下一轮照常是首轮」**，而它的下一轮是被拒。
    AuditDrift 没有行可分类，X-39f 的机制够不到；要在 repl 里按配置判。与 #37(a)（`askCause` 叫人去 `/agents` 找一个已经不在配置里的 agent）放一起修。

    **(c) `/session rebuild` 清行失败时前缀仍是「重建失败」**，而 `/rules reload --rebuild` 同一种情形已经改成「清掉 … 失败」（X-39f 契约冻结了 ask.go 的错误处理不改）。

    **(d) `/session` 的帮助摘要（`internal/repl/commands.go`）写「/session rebuild <agent-id> 丢弃并重建」**，对已删 agent 的悬空行它只是清掉。

    **(e) `/rules reload --rebuild` 仍会删掉第 2、3 类的行**（provider 不可用、权限档不放行）。它们往往只是一时写错配置，改回去本可 resume；X-39f 只改了措辞。

    **(f) `printConfigDrift` 与 `printGoneMembers` 对同一个已删成员各报一次**；非成员的旧 id 在 `/ask` 里走前缀解析，可能问到别的付费 agent（`ask.go` 的 `resolveAgentRef` 那一段）。

    **(g) `--rebuild` 逐行结果里 `existed=false` 没有任何用例覆盖**（原句 B1 与新句 B2 都是）：只会在 AuditDrift 与 Rebuild 之间行被并发删掉时出现。

41. **TUI 第 2 刀：Run board（运行看板）的设计裁决** —— 2026-09-24，**第 1、2 刀已实现（RB-A，`69fed6c`，真机复验通过）；第 3 刀已实现（RB-B，`572467c`，真机复验通过；剩余项 (i)(j)(k) 已修，(h) 未修）；第 4 刀未做**

    上游：Griffin 的 TUI 规格 §7（每次多成员调用在时间线里是一组「运行」，每成员一行：图标、名字、用时或状态）；映射文档 `15_TUI_SPEC_MAPPING.md` §3.1、§4。
    依据：一次只读调查（规格对照、数据通道、渲染约束实测、参照物与既有约定四路并行，外加一个综合员），结论与证据摘在本条。

    **Griffin 裁的两条**：
    - **布局与高度（D1）**：看板放在输入框上方（照规格草图）。一条命令执行期间活区**只长不缩**，少的行用空行垫齐；各段收轮时把终态行打进回滚区；
      整条命令结束（submitDoneMsg）时只缩高一次，前一刻的最后一次写屏必须是 insertAbove，并冻结计时器与输入框的视觉变化。
      残影率在契约冻结前用 VT 回放装置实测；压不住就退到「只在表头显示总览」（下面第 2 刀本来就要实现这个形态）。
    - **用时口径（D4）**：成员回来后显示 `rec.WallMS`（与 `turn_usage.wall_ms` 同源，只量子进程那一段），标注「子进程」；refuse 与 argv 闸路径显示「-」，SpawnFailed 显示「?」。
      跑完那一刻读数会比活计时小、往回跳一下，接受。

    **主代理按惯例定的两条**（可以改）：
    - **Run 的身份（D2）**：一条命令一个 Run，下面分「发言轮 k/N」「裁判轮」几段；段上显示该轮 round_id 末 6 位（第一条 Record 回来后补上），可拿去 `/cost --round` 查。不引入库里没有的本地 #n。
    - **「行不自动清」与 Drifted（D3）**：见 `15` §3.1 的 2026-09-24 订正。

    **硬约束（调查实测或既有裁决）**：
    - 缩高残影：残影行数 = max(0, 缩高前渲染器光标相对帧顶的行 - (新帧高 - 1))（ultraviolet 先 Resize、再把光标夹到新高-1，却不发光标移动）；insertAbove 结束时光标在 (0,0)，所以紧跟在 Println 后面的缩高是干净的；中间插进一次写到新帧高以下的渲染就不干净，而且残影一旦出现，之后再长高也清不掉。
    - 帧高必须 ≤ 终端高 - 1；看板行数上限 = 终端高 - 底帧高(8) - 1，超出就降级成一行总览。
    - 看板行必须进 View 与 frameHeight 共用的 chrome()，否则 X-39e 的退出清屏会少擦。
    - 每条看板行是单行字符串，宽度用 `ansi.StringWidth` / `ansi.Truncate`，不用 lipgloss `Style.Width`（它会把长中文名折成多行，帧高就算错了）。
    - REPL 还可能继续写的时候，看板行进回滚区只能由观察者适配器在发射方的 goroutine 上当场 `p.Send(printLine)`，不能由 Update 返回 `tea.Println` Cmd（Cmd 各起 goroutine，与 LineWriter 的文本不保序）。
    - 数据通道：repl 包加一个晚绑定的 RoundObserver（开轮 / 成员结束 / 收轮三种事件），发射点在 `round.go` 已有的 happens-before 链上（开轮在打出成员名单之后、成员结束在 onDone 闭包里且在 writeAskRecord 之后、收轮在结算打完之后）；行式路径不装、输出逐字节不变；不改 turn 包、不改冻结签名、不改 `repl.New`。
      数据只来自 RoundProgress 交来的 Record 与起进程前的名单；不刮文本、不轮询 inflight、不回读面包屑。
    - 符号：源码里不许出现 ✓ ✔ ✗ ✘ ⚠，⏳ ⌛ 点名禁用，▶ 后面不许跟 U+FE0F。● ◐ ○ × 属于模糊宽度（conhost + 中文代码页下 runewidth 按 2 格算），第一版用 ASCII 标签，等 Griffin 在真终端上试过再换。
    - /ask 不出看板，只在表头显示已用时间（规格 Scenario E）；第 2 刀不加快捷键、不做展开折叠、不做 Compare view。

    **分刀**：
    1. 表头已用时间（只改 TUI；/ask 与 /round 都显示；tick 只在 busy 时续订，用代次 id 防旧 tick 泄漏；帧高不变）。
    2. 观察者通道 + 表头总览（「发言轮 k/N  回来 x/y  12s」；活区高度不变）。
    3. 成员行进活区 + 只长不缩的高度策略 + 各段终态行进回滚区 + VT 回放用例钉住残影为 0。契约冻结前先做残影率探针。
    4. 字形与三档标记打磨（等 Griffin 在 conhost 与 Windows Terminal 上各试一次 ● ◐ ×）。

    **RB-A 落地记录（第 1、2 刀合一，2026-09-24，`69fed6c`）**：
    - 表头：`council <版本>   <状态格>`。不忙时只有状态文字；忙时是「状态文字 [两空格 + 段] 两空格 + 已用时间」，段 = 「发言轮 / 裁判轮」+（N>1 时才印 ` k/N`）+（有 round_id 时印 ` (末 6 位)`）+ `  回来 x/y`。
      已用时间：不到 1 分钟印 `12s`，不到 1 小时印 `1m05s`，再长印 `1h02m`（丢掉秒）。/ask 只显示已用时间。tick 只改已用时长，不覆盖「正在取消」「一轮正在跑，先等它回来或按 Ctrl-C 取消」「发生错误，正在退出」。
    - **tick 装在「提交后的下一条 Update」上，不并进 beginSubmit 返回的 Cmd**：既有冻结用例 Acc6 要求那条 Cmd 展开后恰好是回显接 submit、没有 BatchMsg；用 Batch 会撞它，放进 Sequence 会多一片叶子，还会让 Submit 晚 1 秒。
      真程序里回显那条 printLine 必然紧跟着进 Update，tick 在毫秒内装上。代次 `tickGen` 丢掉旧提交的拍；不忙时不续订。
      **TUI-1b 契约 5.2 因此被收窄**：5.2 说 WindowSizeMsg（以及 busy 时的 Ctrl-C / Enter 等）返回 nil Cmd；现在「提交后的第一条 Update」无论是什么消息，都会带回一个 tick Cmd。
      5.2 的 nil Cmd 只对空闲态、以及提交后第一条 Update 之后成立。既有 Acc1 仍绿，只因为它在空闲态跑；别从 T4 反推 5.2。副作用：Acc8 从约 0 秒变成约 1 秒。
    - **观察者装在 `tui.Run` 里（`p := tea.NewProgram` 之后），不装在 runWith 里**：runWith 手里没有 REPL，x39e 与 tui1d 的用例直接调用它，签名冻结；两处都在 p.Run 之前、任何 Submit 之前，时序等价。
      `r != nil` 门的来历：既有用例 `TestTui1dRunFinalIsRealStdout` 以 `Run(ctx, nil, ...)` 调用。装配那一行必须逐字是 `r.SetRoundObserver(newRoundObserver(p.Send))`：包一层 goroutine 或缓冲 channel 会打乱事件与明细文本的顺序，而且没有任何用例抓得到（集成与审查各实测一次），只能靠审查。
    - MemberDone 通常在 turn 的 worker goroutine 上、progressMu 里调用；铸 id 失败或 round_started 写不进时 turn 走 notifyEach，在 Submit goroutine 上、不持锁。两种都同段串行。写屏幕失败时不发事件（/round 那一行写失败因此改成即 return）。
    - 验证等级：单元与真 tea.Program 用例全绿，契约 50 条变异 49 条被杀（#36 等价）；落地后独立审查补出 #51（E5 丢掉原 Cmd 没有用例抓，实现本身正确），紧接的文档提交补了 T4b（Opus 补写，主代理在主仓库上复做变异 #51，确认只有它变红）。
      **真机复验通过**（2026-09-24，Griffin，`council 0.2.0-rba`，Windows Terminal + PS 5.1，走完契约 §6 的 6 步，都正常）。
    - **RB-B 开工前 Griffin 裁的两条**（2026-09-24）：收尾行用「一行运行小结」（`/round 结束  1m05s  回来 5/6`；取消时 `/round 已取消 …`，由主代理定）；
      成员行状态词与标记照 RB-B 契约 v1.1 §3.3（在跑、完成、未入库、报错、认不出、超时、没起来、拒绝、取消、丢失、未知；可能已吃下、正文见上方、(前缀已断)）。
    - 契约原文（v1.3）归档在 `15a_RUN_BOARD_RBA_CONTRACT.md`，RB-B 起草时复用它的 §1.1、§3.1 与变异清单。
    - 委派链：实现 grok-4.7（一次交出、零机械修正）；测试 Opus 测试腿，背对背；契约派工前两轮对抗审（W1 的 3 条 major 经证伪者确认）。

    **RB-A 剩余项**（都不急）：
    - (a) PasteMsg 在 busy 时仍把状态文字改成「粘贴了 N 个字符」，盖掉「正在运行 / 正在取消」（既有行为）。
    - (b) 表头的 round_id 末 6 位不是 `/cost --round` 能直接接受的参数（它要完整 id）；要不要给 /cost 加后缀匹配未裁。
    - (c) 窄窗口下表头右侧被渲染器裁掉，已用时间可能看不见。
    - (d) RoundStartEvent 不带成员名单；RB-B 追加时写明是新分配的切片、不与 plan 共享底层数组。
    - (e) 被杀的路（SIGTERM、根 ctx 取消）上 Submit 可能在 Run 返回后继续调观察者：p.Send 此时是空操作、不阻塞，无害；RB-B 若要在收轮时 Println 要重新论证。
    - (f) 过期的既有测试注释（契约不许改既有 `_test.go`）：`session_inflight_test.go` 与 `smoke_test.go` 里「四处要一起改」的假 JSON 拷贝计数（tui 包 `rba_fakecli_test.go` 是第 5 份）；`tui1d_run_ctx_test.go:77`、`:167`「管道里看不到表头文字」（写入 `ESC[8;h;wt` 之后就看得到，见 `rba_wiring_test.go`）。
    - (g) 既有 `tui1bIntFields` 会把 Model 新加的 `time.Duration` 字段也收进「按值配对」的候选；Acc1 仍绿，记一笔免得以后被绊。

    **第 3 刀（RB-B）前置探针：「只长不缩」按构造安全**（2026-09-24，隔离装置 + VT 回放，bubbletea v2.0.9 / ultraviolet 源码行为；conhost 与 Windows Terminal 未测）：
    - 构造（探针里叫 C6）：无错的 submitDoneMsg 那一次 Update 里，(1) 把 `View.Cursor` 钉在新帧高以内（(0,0) 或 (0,1)）；(2) 返回 `tea.Sequence(Println(一行非空的收尾行), finalize)`；(3) finalize 的 Update 清看板行、把高度下限归零、取消光标钉、作废 tick 代次。finalize 是唯一的缩高。
      原理：每次 flush 结尾都 MoveTo(view.Cursor)（`cursed_renderer.go:510-512`），钉住之后不管闪烁、打字、tick、窗口尺寸引起的 flush，光标都停在第 0 行；insertAbove 又同步把光标放回 (0,0)（`:801`）。缩高那次 flush 因此从第 0 行开始，残影 = max(0, 0-7) = 0。
      实测 C6 各变体（含把窗口拉宽到 40ms、慢 finalize、窗口里插同尺寸 WindowSizeMsg、真实计时）共 3240 次，残影全部为 0；不钉光标、不做缓解的基线 237/840 次有残影。
    - 只钉光标、不打收尾行：88/240 有残影；只打收尾行、不钉光标：窗口一拉宽 171/240 有残影。两样缺一不可。**代价：每条命令结束时往回滚区多打一行非空的收尾行**（写什么待 RB-B 契约时问 Griffin）。
    - RB-B 契约要写进的硬要求：唯一的缩高点只在上面那条路上（/add、/remove、窗口变化引起的降级若要缩，也得走同一条路）；收尾行必须非空（insertAbove 对空串直接返回，不会复位光标）；Sequence 里 Println 在 finalize 之前，不许用 Batch；
      submitDone 到 finalize 之间算忙，Enter 不许提交，Ctrl-C 要么挡住、要么先取消光标钉再退出（否则退出时最后一帧带真光标，X-39e 的尾巴断言会变，推断未实测）；
      出错退出不缩高、不钉光标，frameHeight 要把看板行算进去，exitClearSeq 按 min(8+看板行数, 终端高) 清（实测 120/120 干净）；
      看板行数 ≤ 终端高 - (2 + 输入框高) - 1，超出降成一行总览，否则帧比终端高、每秒 60 帧整屏重画（实测 71/120 有残影）；
      把 VT 回放装置移进仓库做守卫用例（至少覆盖缩高前后一直打字、8 行看板两种场景，拉宽窗口，屏幕读数与公式读数互相核对），并用变异证明去掉钉光标、去掉收尾行、收尾行为空各自会红；升级 bubbletea 或 ultraviolet 时重跑。
      字节副作用：只在 finalize 窗口里恰好有一次 flush 时，才多出 `ESC[1 q` 与 `?25h`，缩高时再出 `?25l` 与 `ESC[0 q`；没有 OSC 12/112，退出尾巴不变。`ESC[0 q` 会把用户自定义的光标形状复位成终端默认，未在真终端上测。
    - **因此 RB-A 里「submitDone 的 Cmd 一个字不变」（契约 E7）只对 RB-A 冻结，RB-B 必须解冻它。**

    **RB-B 落地记录（第 3 刀，2026-09-25，`572467c`；契约原文 v1.1 归档在 `15b_RUN_BOARD_RBB_CONTRACT.md`）**：
    - 形态：跑 /round 时活区里「表头、横线、每个成员一行、输入框」，不加段标题行（段信息仍在 RB-A 的表头）。成员行 = 状态词 + id + 时间格 + 标记；在跑时是本段已用时间，回来后是 `rec.WallMS`（不到 60 秒截断到十分之一秒）加「子进程」，被拒 / 丢失 / 未知印 `-`，起不来印 `?`。
      每段结束，适配器在 Submit goroutine 上当场把该段的块（段标题 + 成员终态行）打进回滚区，位置在该段结算之后、下一段预算表之前；整条 /round 无错结束时打收尾行 `/round 结束  <已用>  回来 x/y`（Ctrl-C 过的是 `已取消`）。
    - 高度：一次提交内 floor 只长不缩，超过「终端高 - (2 + 输入框高) - 1」降级为 0 行；唯一的正常缩高在 finalize：无错 submitDone 那次 Update 把光标钉在 (0,0)、返回 `Sequence(Println(收尾行), finalize)`。
      这就是探针的 C6。仓库里的 VT 回放守卫（`rbb_vt_test.go`）：拉宽收尾窗口时残影为 0，去掉光标钉留 2 行残影、被抓。
    - 几个取舍的理由：
      - 降级（0 行）的 /round 也打收尾行：行为一致；此时 REPL 已写完，Println 顺序安全；窗口挡住 Enter，下一条命令的回显不会跑到收尾行前面。
      - 收尾窗口里 Ctrl-C 选择挡住，而不是先取消光标钉再退出：挡住不改任何退出字节，Q1 实测之后退出的尾巴与 X-39e A1 逐字节相同；不挡时尾巴变形（出现 `ESC[1 q`，清屏变 `ESC[10A`）。
      - 看板上限用 `termHeight` 不用 `height`：`height` 在 `WindowSizeMsg{0,0}` 之后被兜底成 24，会按假终端高出看板。
      - 「已取消」看 `canceling` 标志，不看状态文字：忙时按 Enter 或粘贴会改掉状态文字。
      - RB-A 的两个测试文件改了 15 处：追加切片字段后事件结构体不可比较，`!=` 是编译错误，绕不开；只比 RB-A 冻结的字段，判别力不变，改后在不含 RB-B 实现的 HEAD 上也全绿（测试腿与集成员各跑一次）。
    - 验证等级：go test ./... 全绿；真 Program 与 VT 回放用例 -count=3、落地后 -count=2 全绿；契约 66 条变异全部被点名杀手杀掉，7 条等价确认存活。**真机复验通过**（2026-09-27，Griffin，Windows Terminal + PS 5.1，`council 0.2.0-s10`）：契约 §6 第 6 步的第 1-5 项通过（逐行在跑与计时、两段 `--rounds 2`、跑的时候一直打字、中途 Ctrl-C、窗口 12 行左右降级与中途拉矮），收起后都不留残影；第 6 项（出错退出）手动造不出来，跳过，由 VT 守卫 G4 担保；第 7 项（自定义光标形状）、第 8 项（行式路径）没有回报结果，记为没测。第 4 项屏幕上是「已跑 1/2 轮，其余取消」：被取消的那一轮起过进程，算跑过，RB-A 的 O2 一直这么钉，是主代理给的预期写错了（写成 0/2）。
    - 委派链：实现 grok-4.7（一次交出、零机械修正）；测试 Opus 测试腿（背对背，VT 模拟器是它重写的）；契约起草人用原型实跑出期望值，派工前 W1 审查员忠实实现、实测残影为 0；落地后独立审查 3 条 minor（下面 (h)(i)(j)）。
    - **已知代价**：
      - 提交中途把窗口拉矮：帧高不缩（不许缩），`termHeight == 8 + floor` 时回滚区留一条表头片段，`termHeight < 8 + floor` 时帧比终端高、每秒 60 帧整屏重画、回滚区可能留表头与横线片段，持续到本次提交结束；finalize 之后屏幕干净（W1 在 VT 模拟器上实测 18 次，真终端的重排没测）。
      - 被杀的路（SIGTERM、根 ctx 取消）上 Run 返回后到达的段块与收尾行会丢失（p.Send 是空操作）；REPL 自己的明细与结算照样落到 stdout。
      - 收尾窗口里被挡的 Enter 是静默的（输入框保留文字，要再按一次）；Ctrl-C 按在最后一个成员回来之后，收尾行也印「已取消」。
      - `ESC[0 q` 会把用户自定义的光标形状复位成终端默认（只在收尾窗口里恰有一次 flush 时出现），真终端没测。

    **RB-B 剩余项**（RB-A 剩余项 (d) 已解决：Speakers 是新分配的拷贝，RB3 担保；(e) 已重新论证：块由适配器在 Submit goroutine 上发、收尾行由 Update 的 Sequence 发，被杀路上都是空操作，见上面已知代价）：
    - (h) 出错或 IsQuit 退出时，submitDone 返回的 Sequence 两次 Send 之间若进来一次 Enter，会开新提交（起 provider 进程）并在 finalize 之外缩高，exitClearSeq 按变矮后的帧高清，擦不到缩高留下的残影。
      根因是 RB-A 起就不挡「退出中」这个间隙的 Enter；真终端上窗口只有一次通道交接那么长，要用户恰在出错那一刻已经打好字并按 Enter。代码注释已补例外；要修得单独一刀给退出中的状态加门（改契约）。
    - (i) Model 的写时拷贝在 RoundStart 路径上没有用例钉住（复用旧数组的写法全绿；实现用的是新分配，正确）。补法：B1-7 前后存 `saved2` 断言不变，列为变异 #67。
      **已补（2026-09-25，`6797ce4`、`1a85a03`）**：B1-7 前后存 `saved2`，#67 此前全包全绿、现在只红在这一条。之后的独立验证又找到四个破坏写时拷贝却全绿的变异
      （等长名单换段时原地覆盖；memberDone 只在第一个人回来时拷贝、在 i > 0 时原地写、在只有一行时原地写），`rbb_cow_test.go` 补了四处快照，七个写时拷贝变异逐个实测全被杀。
    - (j) repl 包测试假件 `fakeStore`（`conversation_test.go`）的 map 读写不加锁，/round 起多个 worker 时偶发 `fatal error: concurrent map writes`（测试腿在纯净 HEAD 上 -count=200 复现）。既有问题，与 RB-B 无关；遇到就重跑。修法：给 fakeStore 加一把 `sync.Mutex`。
      **已修（2026-09-25，`6797ce4`）**：每个读写状态的方法入口取锁，互相调用的部分拆成调用方持锁的内部函数；`WriteTx` 不取锁（fn 回调本对象，Mutex 不可重入），所以不模仿真 store 的事务隔离。
      `-race` 在修之前报出的竞态（12 到 15 处，随调度变）全在它身上，修之后 repl 包零报告（见 #42 (c)）。仍说「fakeStore 不加锁」的五处注释在 `1a85a03` 订正。
    - (k) `x/ansi` 现在由 tui 直接 import，go.mod 里它的 `// indirect` 标记不准（按约定没跑 go mod tidy）。
      **已修（2026-09-25，`6797ce4`）**：挪进直接依赖，与 `go mod tidy` 的结果逐字相同，go.sum 不变。

42. **第一次在云端 Linux 会话里跑全量测试与 `-race`（2026-09-25）** —— **(a)(b)(c)(e) 已修（`d589334`、`6797ce4`、`1a85a03`）；(d) 只记不修；(f) 是结论**

    会话 #10 起在 claude.ai 的云端容器里开发（linux/amd64，Go 1.25.0，**有 gcc、`CGO_ENABLED=1`**），改动经 GitHub 分支同步回本机。
    此前没人在 Linux 上跑过这套测试，本机也跑不了 `-race`（`13` §6.4 / C4）。这一次两件都做了，挖出的东西如下。
    落地后另跑了一轮独立验证（五个验证者加一个查漏的，变异都在各自的 git worktree 里做），下面的结论都经它复核过。

    **(a) RB-B 的 VT 回放守卫在 Linux 上读出假残影。** `internal/tui/rbb_vt_test.go` 的 G1-G4 在 Linux 上稳定红（G1 屏幕残影 7-11 行，G2、G3 各 1 行，G4 留表头与横线），
    而公式读数 30 次全是 0 —— 屏幕与公式对不上，本身就说明是模拟器读错。
    原因：bubbletea v2.0.9 `tea.go:1084` 是 `mapNl := runtime.GOOS != "windows" && p.ttyInput == nil`，非 Windows 且没有 tty 输入时，
    渲染器假定终端的 ONLCR 把 `\n` 映射成 `\r\n`，向下移动只写裸 `\n`（ultraviolet `terminal_renderer.go` 的 `relativeCursorMove`；一次 G2 录下的流里 136 个 LF 有 110 个是裸的）；
    模拟器照契约 `15b` §2.5 把 `\n` 当成「只下移不回车」（那一句本来就注明是 Windows 的语义），于是读错。**不是产品在 Linux 上有残影。**
    已修（`d589334`）：模拟器加 `mapNL` 字段按 `runtime.GOOS != "windows"` 设置，自检两种语义各测一次；Windows 上走原来的代码，行为逐字不变。
    - 复核：Linux 上契约点名的 #46、#66（G1、G2 残影 2，10/10）、#50、#51（G1-G3 收尾行缺失，30/30）、#55、#62（G4）全部照契约被杀；
      强制 `mapNL = false` 的对照在正确实现上复现假残影。别的平台差异在这个装置上不起作用：滚动优化只在全屏时才跑（Council 的 TUI 是内联的），
      hard tab 与退格优化在没有 tty 时是关的；Linux 上 G1-G4 的输出里没有 DECSTBM、没有模拟器不认识的序列。
    - 两处与契约原文的出入（契约是在 Windows 原型上量的）：#50 的「公式残影 2 每个场景至少 9/10」在 Linux 上 G2、G3 只有 5-8/10，随时序变，杀不杀不靠它；
      #55 在 Linux 上只由 `exitClearSeq` 的整串比较杀掉，回放后的屏幕是干净的（渲染器在那次 Update 里自己缩了帧），G4 注释里「屏幕留下看板行」在 Linux 上不成立。
    - **读法限制**：Linux 上这组守卫验的是渲染器「没有 tty 输入」的那条路（mapNl 为真）；真的在 Linux 终端里交互时 stdin 是 tty，mapNl 为假，与 Windows 同一种语义。
      所以 Linux 上的绿不能读作「Linux 终端里没有残影」；Windows 上两者重合，那才是本项目的目标平台。

    **(b) `internal/config` 七条用例把 `DefaultDataDir`（`E:\council-data`）当合法绝对路径用**，Linux 上 `filepath.IsAbs` 为假，`Validate` 拒掉。
    已修（`d589334`、`1a85a03`，只动测试）：`tstAbsDataDir` 在 `DefaultDataDir` 是绝对路径时原样返回它，否则取本平台的一个绝对路径；
    `TestResolveDataDir` 取默认值的两条在非 Windows 上改为断言「返回空串、报错、点名 DefaultDataDir 与它的值」（第一版只查几个字，验证者实测四个变异活下来，`1a85a03` 加强后全杀）。
    复核：Windows 上逐字等价（去掉新增部分就是 `40d1cf7` 的原文；wine 下跑 Windows 测试二进制，121 条 PASS 与改前相同）。
    **D-9 不动**：非 Windows 上没有默认数据目录，配置里必须写 `data_dir` 或设环境变量，这是现状。

    **(c) `-race` 在 repl 包报出的竞态全在测试假件 `fakeStore` 上**（我的首跑 12 处，验证者在父提交上复跑 14、15 处，次数随调度变）（AppendMessage 与 id、BeginTurn 与 ClearInflight、SetProviderSession 与 ProviderSession），就是 #41 RB-B 剩余项 (j)。
    已修（`6797ce4`）：加 `sync.Mutex`，repl 包 `-race` 零报告。注意报告里有几处读写两方在墙钟上并不重叠（读的 goroutine 已经结束），
    原先靠「时间线错开」的论证在墙钟上成立，但时间上错开不构成 happens-before，`-race` 照样报。

    **(d) tui 包剩下的 3 处竞态在上游 `github.com/muesli/cancelreader` v0.2.2 的 Linux epoll 实现里**：`wait()` 在 epoll 返回后读 `r.file.Fd()` 与
    `r.cancelSignalReader.Fd()`，与 bubbletea `shutdown` 里的 `Close()`（以及用例自己关管道）并发。报在 `TestRbaW1RunWiresObserver`、
    `TestTui1dRunFinalIsRealStdout`、`TestTui1dRunCtxReachesInFlightCommand` 上（哪几条撞上随次数变），撞上的那几条在 `-race` 下因此判红，不开 `-race` 全绿。
    **是偶发的**：三次全量 `-race` 里前两次各撞上 3 处，第三次（`1a85a03` 上）一处没有。
    **不在 council 的代码里**；Windows 走 `cancelreader_windows.go`，是另一份实现；BSD 的 kqueue 实现写法相同。不修，升级 bubbletea 时复查。

    **(e) `TestRoundGoPerMemberOutputPrecedesSettlement` 在 `-race` 下必红**：它在 350ms 处探测快成员的明细已经打出，假 CLI 就是测试二进制自己；
    `-race` 构建的二进制退出前默认睡 1 秒（race 运行时的 `GORACE atexit_sleep_ms=1000`），快成员在探测点之前回不来。时序问题，不是竞态。
    （`6797ce4` 的提交信息写成「-race 版二进制起得慢」，是错的：实测首字节 14ms，退出 1017ms。）
    已修（`1a85a03`）：`newRoundStreamRunner` 给子进程设 `GORACE=atexit_sleep_ms=0`（子进程继承环境），repl 包 `-race` 全绿；不开 `-race` 时这个变量没有作用。

    **(f) 结论：生产代码在 `-race` 下没有报出竞态**（turn、store、conversation、app、provider、config 六包全绿；repl、tui 两包的报告全在测试假件与上游库里）。
    照 `13` §6.4 的读法：只能读作「在 linux/amd64、走 `lock_other.go` 与 `killtree_other.go` 那一支的构建下没测到竞态」，不能读作「Windows 上那个二进制没有竞态」。
    §6.4 说的附带收益只兑现了一半：覆盖率实测 `lock_other.go` 12 个块跑到 9 个（含争锁那一支），`killtree_other.go` 只跑到 `prepareCmd`，
    `killProcessTree` 一次都没被执行（turn 包的用例用假 Spawner，走不到真超时杀进程）。
    复跑命令：`go test -race -count=1 ./...`。`1a85a03` 上跑过一次，八包全绿、零竞态报告；再红只应当是 (d) 那种上游偶发，报告里看得到 `cancelreader`。

43. **Step 10 A 之五、C2、D 之二落地（2026-09-27）与剩余项** —— **已落地（C2 `750c2d4`、A 之五 `8c00ec9`、D 之二 `a2e3d76`），真机复验部分通过；剩余项 (a) 已修（U43A，`17b3236`，2026-09-28），(b)-(f) 未修**

    契约原文：A 之五 `13a_S10_A5_CONTRACT.md`、D 之二 `13b_S10_D2_CONTRACT.md`；对 `13` 的订正写在 §3.8、§3.11、§6.2、§7.2 之后。
    Griffin 2026-09-27 裁：拒绝那句话（见 `13` §3.8 订正）；`/session` 新列 `BASELINE`（`suspect` / `ok` / `-`）；`/log` 标记用英文键名。
    委派链：两刀的生产代码都出自 grok（grok-4.7，各一次交出、零机械修正）；测试出自 Opus 测试腿（背对背）；C2 用例出自 Opus 探针；集成、变异、独立审查都是 Opus 子代理。
    验证等级：单测 + 真 store + 真 `app.Run`；A 之五契约 23 条变异、D 之二 32 条（35 种写法）全杀；两份契约派工前各过 W1 与 W1b。

    **剩余项**（A 之五的起草人、审查员实测或读码得出，本批不修）：
    - (a) **被拒之后解不开，只能重启**：`/session rebuild` 只删 `provider_sessions`、不碰 `inflight_turns`，闸空闲时它会「成功」，下一次 `/ask` 照样被拒。
      启动对账持续失败时（例如 `turn_usage` 主键冲突），REPL 里没有任何入口，只能手改 SQLite。要修得给 `/session rebuild` 或新命令一条清在飞记录的路，会碰到 `13` §1.4 / §3.5「对账只在启动时跑」那条结构性约定，要单独一刀、先改契约。
      **已修（2026-09-28，U43A，`17b3236`；契约 `18b_U43A_SETTLE_STALE_INFLIGHT_CONTRACT.md`，Griffin 授权主代理定）**：`/session rebuild <id>` 先经 `Runner.SettleStaleInflight` 结清这个成员在当前会话的陈旧在飞记录，再照旧丢 provider 会话。主代理冻结的四条不变量：本进程这一 key 的闸被占着时拒绝（闸跨过读与结清，确定性探针 T4b / T4c 钉住）；与启动对账同一条路结清（settleOne：lost 那一行 turn_usage、面包屑、标可疑基线，不裸删）；只动这个 agent 当前会话的行；屏幕每句冻结。顺序先结清再丢（fail closed）。对「对账只在启动时跑」的推翻只限用户明确敲命令这一条路。
      **真机复验通过（2026-09-28 17:15，主代理在隔离目录跑，`council 0.2.0-0928b` 行式模式，零花费）**：配置里 claude 的 command 指向不存在的程序（万一没被拒也起不出真 claude）；照 `18b` §6 第 6 步，council 开着时用冻结的播种语句从外面插一行陈旧在飞记录（读回恰好 `u43-probe spawned read`）。`/ask alice` 被拒（简洁档「Alice: 没有回复（拒绝）」加 A 之五那句），没有起进程，那一轮 `turn_usage` 记 refused、金额 NULL；`/session rebuild alice` 逐字打出契约冻结的三行（已结清…turn u43-probe，phase spawned / 那一轮记为丢失… / 本来就没有开着的会话…）；`/log 3` 最后一条是 system 面包屑「成员 alice 的一轮在上次退出时没记上账：起过进程，结局未知；这条 provider 会话点不出名字 [event=turn_lost]」；之后库里 `inflight_turns` 为空，`u43-probe` 在 `turn_usage` 里是 lost（outcome 9）。TUI 下没跑。
      仍解不开的（`18b` §7.1，待 Griffin）：启动对账持续失败（主键冲突）时这条路同样会失败，仍要手改；结清总会连带丢掉 provider 会话（没有「只结清」的入口）；面包屑「在上次退出时」对 (f) 那种同进程留下的行措辞不准。(f) 的行现在也能用这条路解开。真机没有复验 [未验证]。
    - (b) **`BeginTurn` 之后、起进程之前的一次取消会留下一条 begun 行**：`refuse` 用调用方 ctx，被取消时清理与这一轮 refused 的 `turn_usage` 都写不进。
      下一轮被 A 之五拒绝（以前是撞唯一索引、打裸 SQLite 错误），重启对账能解开。A 之五之前就有；修法是这几支 refuse 改用 Background 落库，要单独论证。
    - (c) `/round` 的预算表对这种成员照样印「不在飞」并预报金额，到执行时才被拒。
    - (d) `/session` 的 NEXT 列不报「下一轮会被拒」，与 #40(a) 同族。
    - (e) 启动对账失败时，横幅在 stdout 照样打「注意: 有 N 轮……」并逐行点名，失败本身只在 Err 里（A 之四的既有行为）。
    - (f) 同进程里的双重故障（`13` §3.14：两次 `RecordTurn` 都失败）留下的行，会把这个成员锁到重启为止。这是 §3.8 设计上的结果，以前没人写下来。

    **真机复验（2026-09-27，Griffin，`council 0.2.0-s10`）**：`/session` 的 BASELINE 列（四个成员都是 `ok`、列对齐）与 `/log` 的事件标记（` [event=round_started]`、` [event=session_opening]`、` [event=round_finished]`，发言行不带多余标记）通过。崩溃那一步没造出来：opus 约 4 秒就回来，关窗时那一轮已经记账，重开后没有丢失的轮；这一支由 C2 端到端用例（`s10c2_crash_e2e_test.go`）担保，真机没验。` [thought_dropped]` 要改配置，真机没测。

44. **TUI 里整块粘贴多行命令，会被当成一条提交，而回显看起来像逐条执行了（2026-09-27 真机复验撞出）** —— **Griffin 2026-09-27 夜裁「第 2 行起有 / 开头的行就不提交」；已落地（X44，`e85d1bc`），真机未复验**

    主代理给的复验步骤把几行命令放在一个代码块里，Griffin 整块粘进 TUI、按一次回车：
    - `/new 真机复验` 换行 `/add opus-5.0,grok-4.7` 成了一条 `/new`，会话标题变成「真机复验 /add opus-5.0,grok-4.7」，成员没加进去；
    - 「一句话」换行 `/round … --go` 被整段记成一条用户消息（seq 1-3），`/round` 根本没执行（没花钱）。
    多行提交是有意支持的（未决 #12，长文粘贴），这本身不是 bug；问题在于**回显把每一行都印成 `> /new …`、`> /add …`**，看起来像两条命令都跑了，用户没有任何提示。
    可选的防护：多行提交里第 2 行起有以 `/` 开头的行时，不提交、在状态栏提示「一次只能一条命令；要发多行消息请……」；或者只在第 1 行是命令时拒绝多行。要不要做、做哪种，等 Griffin 定。
    主代理这边已改做法：给 Griffin 的 council 内命令一行一块，并写明逐行回车。

    **落地记录（2026-09-28，X44，`e85d1bc`；契约 v1.2 归档为 `17a_X44_MULTILINE_GUARD_CONTRACT.md`）**：
    - 判据（`repl.HasLaterCommandLine`，只写一份，TUI 调用）：整段先按 Submit 的规矩剥首尾，按 `\n` 切行；第 2 行起任意一行剥掉行首空白与四个不可见字符后以 `/` 开头就不提交。输入框文字与光标原样、不回显、不进忙，状态栏「一次只能一条命令，逐条回车；整段当消息发，首行只写 /say」，留到下一次改写状态栏的事件（成功提交、粘贴、空闲 Ctrl-C）为止；打字、Esc、焦点消息、tick、窗口尺寸都不动它。单行与第 2 行起没有 / 的长文粘贴与改动前逐字节相同；行式模式不改。
    - 生产代码出自 grok（一次交付），4 个测试文件出自 Opus 测试腿（含真 tea.Program 与真 tui.Run 的括号粘贴）；变异 48 条全杀。
    - **待 Griffin**（契约 §7，都先取了保守读法）：(a) `/say` 出路留着（第 1 行只写 /say，下面整段当一条消息；不要就删一个 if）；(b) `/ask <id>`、`/to <ids>` 不给同样的出路；(c) `//` 开头的行照原话一律挡；(d) Esc 不绑「清空输入框」。
    - 独立审查余项（代码都没错）：一处注释把 restOf 说成「原样」发出（它会剥首尾空白）；「/ask、/to 不开出路」与「# 等别的标点开头的行不挡」没有用例。
    - 真机没有复验 [未验证]；契约 §6 的复验步骤里，CO1 默认简洁档下提交成功不打「已记下 seq N」，改用 `/log 3` 确认。

45. **两个新功能的设计裁决：模型识别后直接 /add（写进 config.toml）、简洁输出 + 自动总结（2026-09-27，Griffin）** —— **三件都已落地、真机未复验：简洁输出（CO1，`1c62dbb`）、模型识别（MD1，`e9afff9`）、自动总结（SM1，`a877d44`）；Griffin 的 config.toml 已加总结成员**

    **一、模型识别**
    - Griffin 要：Council 识别他订阅里有哪些模型，按识别出的模型直接 /add 成员。
    - **写进 config.toml**（他裁）：选中的模型整段追加到文件末尾（不重写，原有注释与格式一个字不动），写前备份，写后 Load + Validate，Runner 与 REPL 两处一起换指针，再 Join。
      **这推翻了 `11d_STEP8C_CONTRACT.md` §（:626-632）与 `configcmd.go` 头注释「不许运行时新增 agent、不写回 toml」的裁决**：那条的两个理由（写回会丢注释与格式；不写回就是第二事实源）都被「只追加、落盘」回应了。
    - 三家的识别方式（2026-09-27 只读探针，零推理、零花费）：
      - **agy**：`agy --output-format json models`（flag 必须在子命令前），解析 `command.data.models[]{id,label}`，按账号从服务器拉，反映订阅；每次 3-4 秒、写 `~/.gemini` 日志；stdin 要关、要超时。label 就是配置里的 model 写法（例如 `Gemini 3.8 Flash (High)`），强度编在 label 里。
      - **claude**：没有列表命令；用 SDK 控制协议的 initialize 握手（`-p --safe-mode --input-format stream-json --output-format stream-json --verbose --no-session-persistence --strict-mcp-config`，stdin 只发一条 initialize，收到 control_response 就关），`response.models[]` 带 value / resolvedModel / displayName / supportedEffortLevels。
        Griffin 授权用真实登录跑过一次：约 1 秒，`subscriptionType = "Claude Max"`，10 个模型；debug 日志里没有 `/v1/messages`，`~/.claude/settings.json` 未被改。haiku 没有 supportedEffortLevels（不能发 `--effort`）。
      - **grok**：`grok models` 只回显 `~/.grok/config.toml` 里手写的模型，不反映订阅；真实清单要带 CPA 密钥问中转的 `/v1/models`。**Griffin 裁：只列本地配置，不碰密钥。**
    - 其余裁定：成员 id **跟现有写法**「模型-版本-强度」（例如 `sonnet-5-high`、`gemini-3.8-flash-high`），只在两家重名时加来源前缀；加入前显示名字，可改。
      agy 清单里的 Claude / GPT-OSS 行**显示并标注走 agy 额度**。主代理按惯例定：/add 时按需识别（不在启动时跑）；claude 加入时选强度（默认 high，只给该模型支持的档）；grok 不问强度（命令行上不生效）；agy 的强度就在名字里；新成员 read / discussion；**已有成员一个不动，换模型只新建 id**（同一 id 换 model 会作废会话、多付一次入场费）。
    - 设计过但没落地的 `ModelDiscovery` 能力位（`02_AGENT_PROVIDER_ARCHITECTURE.md:64`、三级回退 :73-79）在这一刀补上。

    **二、简洁输出与自动总结**
    - Griffin 要：默认只显示「成员名: 回复」与最后一段「总的来看（Fable 5.1 做总结）：…」；冷暖、金额、用量等默认不出现。
    - 主代理定：REPL 层加输出档位，**默认简洁**，`-verbose` 启动参数或 `/verbose` 切回今天的详细输出；零值是详细（直接构造 REPL 的既有测试不受影响）；格式只有一份（档位判断写在 writeAskRecord 等既有函数内部，不另写一套）。
      成员名用配置里的 `display_name`（空则用 id），只改屏幕、不改送给模型的 prompt。回复仍按「谁先回来先打谁」上屏。
    - **简洁模式里照样上屏的**（关系到钱或内容丢失，出处见本次只读调查）：失败结局压成「<名>: 没有回复（<结局>）」加原因；「provider 那边可能已经吃下这批内容」；正文写不进库时整段原文；
      漂移、被拒工具调用、本轮为空或被截断的一行提示；跳过谁、谁没被问、「已跑 k/N 轮，其余取消」、所有拒绝句与持久化失败。金额整类不显示；任何地方要显示合计金额，必须带「k 名成员未报金额」的限定语。
    - **用时**（Griffin 裁「显示每个模型的用时」）：成员名后带子进程用时，例如「Opus 5.0（6.8s）: …」，最后一行总用时。
    - **自动总结**（Griffin 裁「每次自动」）：多个成员的 /round 跑完（`--rounds N` 时在最后）自动由指定的总结成员出一段「总的来看（<名> 做总结）：…」，`--no-summary` 跳过。
      **这推翻 `12_STEP9_CONTRACT.md` §4.7「不自动触发」**（:1148、:1289）与 round.go 头注释「自动链入口必须收显式上限」：上限是「每条 /round 恰好多一次调用」，有界。代价照实写：每条 /round 多一次串行调用；总结成员首次开口或间隔超过 1 小时是冷启动，claude 约 1 美元量级。
    - **总结只给 Griffin 看**（他裁）：总结不进成员们下一轮的上下文（避免被带偏，也省输入）。总结成员看不到失败（system 消息不进 prompt），所以某成员没发言时由 Council 自己在总结上方注明。
    - 已知现状：`gpt-6.0-sol` 今天不能当成员（只有 claude / grok / agy 三个适配器）。**Griffin 裁：接入 codex 排在这两个功能之后**，先做真机探针。

    **落地记录（2026-09-28 通宵，Griffin 睡前授权：每刀落地后推 GitHub、SM 落地后备份并写总结配置、codex 允许两条最短 prompt、加班只做 #44）**：
    - 契约：MD1 `16a_MD1_MODEL_DISCOVERY_CONTRACT.md`、CO1 `16b_CO1_CONCISE_OUTPUT_CONTRACT.md`（均 v1.1，派工前 W1 + 主代理裁决书 + W1b 两轮对抗审查；主代理裁定写在 CO1 §7.4 开头）。
    - **CO1 已落地（`1c62dbb`）**：默认简洁、`-verbose` / `/verbose`、成员头「<显示名>（<用时>）:」、行式一行「总用时」、TUI 简洁档不打段终态块。生产代码出自 grok（一次交付），7 个测试文件出自 Opus 测试腿；变异 #1-#83 全杀。
    - **MD1 已落地（`e9afff9`）**：`/add` 识别三家模型、`/add +编号 [--id] [--effort]`、`/add --refresh`，追加写配置（备份、原子替换、失败恢复、写前守卫）。生产代码出自 grok（五块各一次交付），5 个测试文件出自两条 Opus 测试腿；变异 M1-M109 共 104 条全杀。与 CO1 叠加时 `repl.go` 结构体末尾一处双插冲突，两边逐字保留。
    - 主代理按惯例定的（契约 open items 里标「主代理定」）：总用时只在 /round 真进入过扇出时打（预览、没人可问、被拒不打；与 TUI 收尾行的门一致）；claude 写进配置的 model 是 resolvedModel（钉死，不随别名漂）；识别结果在本会话缓存成快照、失败行后提示 `--refresh`、取消不缓存。
    - **待 Griffin**（都先取了保守读法）：`/add` 新成员 role 写空串（要不要默认抄一句）；已配置的 claude 模型不编号、换强度只能手改配置（要不要也编号；2026-09-30 起换强度用 /add <ID>，AD1，#49）；`/help` 里 /add 的说明没改（看不到 `+编号`）；简洁档 PostRejected 的头「没有回复（未入库）」后面紧跟整段正文的措辞；简洁档 `/say` 成功时什么都不打；「正在问 X」「本轮问 N 个」保留 id；详细档行式不打总用时。
    - 独立审查余项（代码都没错，SM1 落地后一刀补）：CO1 详细档那一侧 no_delta 门与「点名了」门没有用例、cmdRound 注释把「进入扇出」说成「真起过进程」、「总用时」写失败没有用例；MD1 两处防崩判据（外层 / 内层 response 为 nil）与守卫的读失败分支没有用例、三处注释措辞（未实测的后果写成事实、AppendBlock 说调用方会恢复、按 HEAD 行号引用 configcmd.go）。契约等价清单各有一条过时（CO1「start 挪到 TakeRoundSnap 之前」、MD1「三家并发识别」），集成员实测都能被杀。
    - **真机没有复验 [未验证]**。复验步骤见 16a §6 第 5 步、16b §6 第 5 步（council 内命令一行一块）。
    - **`/add` 识别表真机原件（2026-09-28 17:30，Griffin 在 Windows Terminal 的 TUI 里、真配置、`council 0.2.0-0928b`；MD1 契约 open item (a) 要的就是这张）**：先打配置里 7 个候选，再「正在识别模型: agy, claude, codex, grok」，约 10 秒出表，共 33 行：agy 13 行（10 个 Gemini 3.8 / 3.7 / 3.6 Flash 与 3.1 Pro 的 High/Medium/Low 标「订阅」；`agy-sonnet-4.6-thinking`、`agy-opus-4.6-thinking`、`gpt-oss-120b-medium` 标「走 agy 额度」）；claude 8 行，标「订阅」：`sonnet-5-high`（claude-sonnet-5）、`haiku-4.5`（claude-haiku-4-5-20251001，强度 `-`）、`opus-5-high`（claude-opus-5）、`fable-5-high`（claude-fable-5）、`opus-4.8-high`、`opus-4.7-high`、`opus-4.6-high`（后两个没有 xhigh）、`sonnet-4.6-high`；codex 7 行（同 #46 的隔离复验）；grok 5 行标「本地配置」（grok-4.6 / 4.5 / 4.3、grok-composer-2.5-fast、grok-build）；表后 custom/ 说明行。已配置的 `claude-fable-5-1`、`grok-4.7`、`Gemini 3.8 Flash (High)` 都藏掉了。
      **`opus-5-high` 那一行在**：说明 Max 订阅下那一行的 value 不是 `opus`，他的 `opus-5.0`（model 写别名 `opus`）没藏住它。别名 `opus` 多半解析到 claude-opus-5 [推断]，加 +16 大概率是同一个脑子的第二个成员。
      **表里没有 Opus 5.5**：claude 那 8 行是本机 claude CLI 2.1.283（9 月 26 日装的）握手报的模型选单，没有 `claude-opus-5-5` [推断：这一版 CLI 的选单没收它，他的订阅本身能用 Opus 5.5]。Griffin 让主代理手加：2026-09-28 17:40 先备份 `config.toml.bak-20260928-opus55`，末尾追加 `[agents."opus-5.5"]`（model `claude-opus-5-5`、read、discussion、role 同他别的成员；强度先写 high，他随即要「Extra」，17:45 只改那一行为 `xhigh`，改前备份 `config.toml.bak-20260928-opus55-high`）与一段说明注释，原有 8369 字节不变；隔离副本加载核对通过（8 个 agent）。CLI 认不认这个名字 [未验证]，第一次 `/ask opus-5.5` 才知道。
      同时发现：`/add` 的配置候选里列出了总结成员 `fable-5.1-summary`（第 4 行，显示名与 `fable-5.1` 同为「Fable 5.1」）；它不该被当普通成员加（放进 /round 会被拒）。下一刀从候选里藏掉（待主代理）。**已修（CH1 `4d3dc0b`）**：候选里不再列总结成员，显式 `/add` 它会拒绝并说明它会自己加入；它在当前会话里是已退出成员时放行，恢复总结的路 `/join <id>` 留着。
    - **SM1 已落地（`a877d44`，契约 v1.1 归档为 `16c_SM1_AUTO_SUMMARY_CONTRACT.md`）**：`[summary] agent = "<id>"`；多成员 /round --go 之后自动总结（总结轮，单独 round_id，走与裁判轮同一条路、不绕任何闸）；总结只给用户看（Recipients=[总结成员自己]，其余成员的 prompt 里没有）；自动 Join、已 /leave 不拉回、出现在 /round 名单里整条拒绝；role 为空时补 DefaultSummaryRole；`--no-summary`。生产代码出自 grok（一次交付），6 个测试文件与 CO1 用例的授权期望更新出自 Opus 测试腿；变异 74 条全杀。
      主代理定（契约 §7.1）：「多个成员」按本条命令真问到的成员（含裁判）去重计数 ≥ 2；writeAskRecord 拆成 writeRecord + 两个薄包装（格式只有一份）。
      **待 Griffin**（契约 §7.2，都先照 #45 原话）：总结头不带用时（他另裁过「显示每个模型的用时」）；简洁档总结之前照打「本轮问 1 个: <总结成员 id>」作进度信号；DefaultSummaryRole 的措辞（改它会让总结成员冷启动一次）。
      独立审查余项（代码都没错）：`--rounds N` 时「前几轮都回了、最后一轮全失败」这个取值没有夹具；总结段写屏幕失败没有用例；四处注释没跟上（头行漏 `[--no-summary]`、「裁判轮传 0」漏总结轮、render 回调、writeAskRecord 的调用方）；一处注释把「多读成员表会变红」说成有担保。
    - **Griffin 的 config.toml 已写（2026-09-28 08:55，他睡前授权「今晚直接写」）**：先原样备份成 `C:\Users\25142\.council\config.toml.bak-20260928`（逐字节比对相同），只在末尾追加 1021 字节（LF、无 BOM；原有 6513 字节逐字节不变）：一段说明注释、`[agents."fable-5.1-summary"]`（display_name 'Fable 5.1'、claude、model 'claude-fable-5-1'、effort 'high'、read、discussion、不写 role）与 `[summary] agent = 'fable-5.1-summary'`。没有用他现有的 `fable-5.1`：它在 `teams.all` 里，总结成员不许进 /round 名单。
      核对：把这份配置拷到隔离目录、只把 data_dir 换成隔离路径，用 `council 0.2.0-sm` 行式启动敲 `/agents`：加载与校验通过，7 个 agent 全在。没有发任何提问；`claude-fable-5-1` 这个 model 字符串本身能不能用仍是 [未验证]（与 fable-5.1 相同）。

46. **接入 codex（CX1，codex provider 适配器）** —— 2026-09-28，**已落地（`c0e6a27`），真机未复验；MCP 风险 Griffin 已接受（同日 11:30）**

    上游：#45 末尾 Griffin 裁「接入 codex 排在两个功能之后」，他要能把 `gpt-6-sol` 这类模型当成员（codex 里的 slug 是 `gpt-6-sol`，不是 gpt-6.0-sol）。契约 v1.2 归档为 `17b_CX1_CODEX_ADAPTER_CONTRACT.md`（W1 + 主代理裁决书 + W1b）。
    - **探针**（codex-cli 0.147.0，E 盘机，2026-09-28）：Griffin 授权的两条最短付费 prompt 已用完（`gpt-6-sol` low，新会话 4.7 s 回「PONG」、续会话 4.9 s 答出上一轮的词，都 rc 0）；其余全是零花费（`--help`、`debug models`、`debug prompt-input`、`features list`、`mcp list`、`sandbox`，后几项在隔离 CODEX_HOME 里跑）。JSONL 事件：`thread.started{thread_id}` → `turn.started` → `item.completed{item:{type:"agent_message",text}}` → `turn.completed{usage:{input_tokens,cached_input_tokens,cache_write_input_tokens,output_tokens,reasoning_output_tokens}}`，没有金额。
    - **冻结的做法**：正文走 stdin（`-`）；身份照 agy 只在空会话拼进 stdin；read 档 `-c sandbox_mode=read-only -c approval_policy=never`（exec resume 没有 `-s`；两者在真 exec / resume 上是否生效**未实测**）；隔离参数 `--ignore-rules --disable plugins --disable apps -c skills.include_instructions=false`；workspace / full 两档拒绝（没有核实过的写入边界）；强度经 `-c model_reasoning_effort=`，适配器自己校验取值（codex 对非法值静默接受）；usage 五项全部标未报、原始对象进 `meta.usage_raw`（[推断] usage 是 thread 累计值：两轮 out 6 → 12；没单独验证，所以不把它记进只追加的 `turn_usage`）。
    - **风险与待 Griffin**：
      - **MCP**：他的 codex 开着 `node_repl`（Node 代码执行）、`cua_repl`（桌面版电脑 / 浏览器操作）、`drawio`（`codex_app` 关着）。零花费试遍了 features list 的全部开关，找不到能整类关掉它们的；按名字关（`-c mcp_servers.<name>.enabled=false`）在名字不存在时会让 codex 启动失败，不能写死进命令行。read 档多半管不住 MCP 工具 [推断]。要么他在自己的 codex 配置里关掉（也会影响他平时直接用 codex），要么接受风险、复验时专门试一次经 MCP 写盘。可以先零花费跑 `codex --disable plugins --disable apps mcp list` 看 Council 实际带着哪些。
      - 全局 `~/.codex/AGENTS.md` 隔离不掉（换 CODEX_HOME 会带走登录态），每个 codex 成员每轮都带着它。
      - ultra 档会自动分派子任务、花费可能成倍，本刀放行。
      - 用量口径：`~/.codex/sessions` 下那两次付费会话的 rollout 文件里可能有 token_count 事件，能零花费判定累计还是本轮；文件含对话内容，要他自己看或明确授权。
      - 真机复验要花钱（契约 §6 第 6-8 步三次、写盘探针另两次），每一步跑不跑由他定。
    - **待主代理**：失败轮（turn.failed / error）的 `usage_raw` 不落库——runner 在失败路径只吸收 thread_id（独立审查 major，codex.go 注释说反了，要改得动 `internal/turn`，与用量口径一起另开一刀）；续会话悄悄开新 thread 时 codex / agy 成员会永久丢身份（既有问题，警告措辞两家一起改）；codex 模型识别（`codex debug models` 接进 MD1 框架）另开一刀，届时 `ModelDiscoveryUnknown` 改成 Subcommand。
    - 生产代码与授权的测试改动（registry_test.go 登记四家、MD1 P1 放行 codex）出自 grok（一次交付）；3 个新测试文件出自 Opus 测试腿，夹具是两次付费探针录下的真 JSONL；变异 61 条全杀。独立审查另有 6 条 minor，都是几处注释把未实测写成事实（approval_policy、`--disable plugins/apps` 去掉了什么、失败事件形状的出处、workspace-write 的说法前后不一）。
    - 给 Griffin 手加的配置示例在 17b §6；Council 不碰他的 codex 配置。

    **Griffin 2026-09-28 11:30 裁（醒来后第一句：「接受，这些你可以决定」）**：
    - **MCP 风险：接受。** codex 成员照常带着他 codex 配置里开着的 MCP 服务器（node_repl、cua_repl、drawio），Council 不关、不改他的配置；read 档对 MCP 工具不设防是已知且被接受的残留风险。codex 真机复验时写盘探针仍建议做（17b §6），结论照实记。
    - 其余待定项授权主代理定，裁定如下（**主代理定，Griffin 授权**）：
      - #45：`/add` 新成员 role **留空**（Council 不替他往配置里塞看不见的提示，要加手改）；已配置的 claude 行**不编号**（换强度手改配置；2026-09-30 起换强度用 /add <ID>，AD1，#49）；**`/help` 补上 `/add +编号` 与 `--refresh`**（唯一要动代码的一条，下一批做）；简洁档 PostRejected 措辞、`/say` 成功不回确认（TUI 的回显 `> …` 已经说明发出去了）、进度行保留 id、详细档行式不打总用时：**维持**。
      - #45 自动总结：总结头**不带用时**（他给的例子里没有，最后一行总用时已含它）；总结前照打「本轮问 1 个: <总结成员 id>」**维持**；DefaultSummaryRole 措辞**维持**。
      - #44：`/say` 出路**留**；`/ask`、`/to` **不给**出路；`//` 行**照挡**；Esc **不绑**清空。
      - #46：ultra 档放行、workspace 档不开放、全局 AGENTS.md 隔离不掉：**维持**。用量口径的零花费判定要读 `~/.codex/sessions`，他没有明说授权，**不读**，等真机复验的第三轮数据。

    **CXH 落地（2026-09-28，`db8c41a`；契约 `18a_CXH_CODEX_DISCOVERY_CONTRACT.md`）**：配置里写了 `[providers.codex]` 时，不带参数的 `/add` 也识别 codex 的模型（`codex debug models`，本地目录、不推理）；visibility 不是 list 的跳过；`custom/` 开头的（他在 codex 配置里自定义转发的别家模型）不列、表后一行说明；来源标「codex 目录」（是否反映订阅没验证）；每个模型的强度取自它自己的 supported_reasoning_levels，默认 high（同 claude；比 codex 自己给 gpt-6-sol 的默认 medium 贵）。codex 的 ModelDiscovery 改成 Subcommand。`/help` 的 /add 说明补上 `+编号` 与 `--refresh`。生产代码出自 grok（补丁在契约里逐字冻结），变异 46 条全杀。
    **codex 真机复验（2026-09-28 17:06-17:09，Griffin 授权「跑」；主代理在隔离目录里跑，`council 0.2.0-0928b` 行式模式，不碰他的真配置与真库）**：
    - 做法：隔离配置只有 `data_dir`（隔离路径）与 `[providers.codex]`（他本机的 `codex.cmd`），所以 `/add` 只识别 codex、不碰 claude 登录。先零花费跑一遍 `/add`，再 `/add +1 --effort low --id gpt-6-sol-low`（这一步同时真机验了 MD1 的追加写配置与自动备份），然后照 17b §6 第 6-8、10、14 步问了 **5 次**（`gpt-6-sol`、low 强度），共约 84 秒。
    - **CXH 识别表**（真机原件）：7 行，`gpt-6-sol-high`、`gpt-6-astra-high`、`gpt-6-luna-high`、`gpt-5.6-sol-high`、`gpt-5.6-terra-high`、`gpt-5.6-luna-high`、`gpt-5.5-high`，来源都是「codex 目录」，EFFORTS 按模型各自的档；表后一行「codex 目录里还有 2 个 custom/ 开头的模型没有列出……」；不到 1 秒。
    - **会话**：第 6 步 `outcome spoke`、新开 provider 会话 `01a0e6d6-…`（14.1 s）；第 7 步同一个 id、答出 KIWI（4.1 s）；第 8 步同一个 id（5.7 s）；没有漂移、没有失败。**stdin 与 `-`、经 codex.cmd 起进程、`-c sandbox_mode` / `approval_policy` / `--ignore-rules` / `skills.include_instructions` 在真 exec 与 exec resume 上都被接受**（原先 [未验证] 的几条就此验过一次）。回复开头**没有**「收到，Griffin。」。
    - **工具清单**（第 6 步模型自报）：`shell_command`、`apply_patch`、`functions.exec`、`web__run`、`image_gen__imagegen`、`collaboration.*`（含 spawn_agent）、`mcp__node_repl__js` 等三个、`mcp__drawio__*` 十个等；**没有** `cua_repl`（多半被 `--disable plugins --disable apps` 去掉）。
    - **用量口径：累计**。三轮 `meta.usage_raw`（同一个 provider_session_id）：output_tokens 327 → 333 → 339、reasoning_output_tokens 48 → 48 → 48、input_tokens 28346 → 42923 → 57522。第 7、8 步只回一个词，本轮口径预测 out 约 6，实测是上一轮加 6，reasoning 不再增长；与早上两次探针（6 → 12）一致。**本轮口径被排除，结论是 thread 累计值**（契约判据「有项变小才排除累计」没触发；这里用的是反向的排除，数据写在上面，可复核）。Council 对 codex 五项 token 全标未报，没有记错账；要显示 codex 的用量得存上一轮的累计值再求差（待主代理第 1 条，与失败轮 usage_raw 一起另开一刀）。
    - **只读档写盘探针**：第 10 步（续会话，exec resume）与第 14 步（新会话 `01a0e6d8-…`，exec）各问一次，让模型分别用 shell、apply_patch、`mcp__node_repl__js` 往成员工作目录（绝对路径）写 PROOF.txt / PROOF_PATCH.txt / PROOF_MCP1.txt：**两次都三条全被拒**（shell 访问被拒绝、apply_patch 只读沙箱拒绝、node_repl 返回 EPERM），三个文件两次都不存在。所以 **read 档的 shell 与 apply_patch 在 exec 与 exec resume 上都围得住（验过一次），`mcp__node_repl__js` 这一个 MCP 工具两次都被拒**；drawio 等其它 MCP 工具没试，不推广。
    - 没验的：TUI 下的 codex 成员、high 强度（Griffin 用默认 high 时更慢更贵）、codex 失败形状（仍零观测）。codex 自己在 `~/.codex/sessions` 下为这 5 轮写了会话文件（CLI 的正常行为，主代理没有读）。
    **Griffin 的 config.toml 又追加一段（2026-09-28 15:15，按上面授权，主代理定）**：先备份成 `config.toml.bak-20260928-codex`（逐字节相同），只在末尾追加 835 字节：一段说明注释（含「上面『没写进来的两个』那段注释已经过时」、MCP 已接受、默认 high 比 codex 默认贵）与 `[providers.codex] command = 'C:\Users\25142\AppData\Roaming\npm\codex.cmd'`；原有 7534 字节逐字节不变。追加前查过文件里没有任何 `[providers.codex]`。隔离副本用 `council 0.2.0-0928b` 加载核对通过（7 个 agent）。没有加任何 codex 成员；要加由他用 `/add` 自己挑。

47. **对话模式：直接打字就让成员回复（2026-09-28，Griffin）** —— **已落地，真机未复验**：CH1 对话模式 `4d3dc0b`（`19a`）、AT1 @ 补全 `4202b6c`（`19b`）、AT1-FU `ee247c6`（`19c`）

    Griffin 原话（真机试完简洁输出之后）：「现在 OK 了对话框也简单了，就是每次都要 /ask 这样，能否默认就是对话状态啊，通过 add 加不同的 agent，然后如果一个状态下 agent 很多，那么每个都要回复，如果用户明确只有某个回复，那么就只有某个就行」。
    - **方向（Griffin 定）**：不以 `/` 开头的输入（裸文本），记下之后自动让成员回复——没点名就是当前会话全部讨论成员；消息里 `@` 了谁就只有谁。多个成员回复时照 SM1 自动总结。
    - **这推翻了 `12_STEP9_CONTRACT.md` 与 `round.go` 头注释「名单为空则拒绝，绝不默认全员」**，但**只对裸文本**：`/round` 名单为空时照旧拒绝。代价照实写：每发一句话就是 N 次调用加一次总结，没有预览。
    - **主代理定（Griffin 授权定小事）**：复用 `/round <ids> --go` 那一条路（闸、在飞记录、简洁输出、总结、总用时一个不绕）；REPL 层零值是「不自动」、app 默认打开、测试助手退出（照 CO1 的 `Verbose` 先例），另给一个会话内切换的命令；消息里有 mention 但一个都不是当前活跃成员时**谁都不问**（照打「不是当前会话的成员」），只有一个 mention 都没有才问全员；总结成员（`[summary] agent`）永远不当发言者；judge 模式的成员照 `PlanRound` 的既有规则；`/say` 保持「只记下、不让任何人回」；`/to`、`/ask <id> 文本` 不变；三条裸文本路径（单行、TUI 通过 X44 之后的多行提交、行式多行草稿发送）都算；会话里没有成员时照常记下并打一行提示去 `/add`。
    - 同刀顺带：`/add` 的候选里藏掉总结成员（显式 `/add` 它时说明它会自己加入）；`/help` 里 `/session` 那一行补上「先结清陈旧在飞记录」。
    - **补充（同日，Griffin）**：「@ 的时候，后面出现 @ 可选的模型，你懂吧」—— TUI 输入框里敲 `@` 弹出可选成员的补全列表。另开一刀 AT1（只在 TUI 层，与 CH1 并行）。主代理定：候选 = 当前会话活跃成员（不含总结成员），显示 id 与显示名；`@` 之后按已输入前缀筛选；上下键选、Tab 或回车补全（补上完整 id 加一个空格）、Esc 关；列表开着时回车只补全、不提交；列表放在输入框**下方**（放上方时关列表缩高，按 RB-B 实测的残影规律容易留残影，起草时用 VT 回放装置实测定）。
    - **AT1 落地记录（2026-09-28 23:30，`4202b6c`，真机未复验）**。契约归档 `19b_AT1_MENTION_COMPLETION_CONTRACT.md`（先读文件头的「落地后订正」）。委派链：
      - 生产代码 `internal/repl/mentioncand.go`（新）、`internal/tui/model.go`、`internal/tui/run.go` 由 grok-4.7 实现，与契约 §1.1 逐字节相同；
      - 五个 `atx_*_test.go` 由 Opus 测试腿写成；
      - Opus 集成员：§4 的 57 条变异全杀、4 条等价，没有机械修正；
      - Opus 独立审查：2 major、3 minor，都不是实现偏离契约，交后续一刀 AT1-FU。
      - 验证：全量 `go test ./... -count=1` 九个包全绿。
      - **放法与钉光标**（契约 §0 第 3 条，VT 回放实测）：
        - 列表放输入框上方时，两种放法退出都留残影；放下方、把真光标钉在 `@` 后面，残影为 0。
        - 另测的场景（空闲时插一行、粘贴收起、Ctrl-C 收起、8 行输入、长行折行、中文前缀）在钉光标下残影都是 0。
        - 「窗口变尺寸」两行只给程序发了 `WindowSizeMsg`，模拟器没有跟着缩；量的是帧高变化本身。**真终端缩放没测**，以真机步骤 8 为准。
      - **候选怎么取**：只在一次提交（`Submit`）返回之后，在执行它的同一个 goroutine 上取（`submitCmd`，用根 ctx）。所以与命令串行，不在 `Update` 里读库，不用 `p.Send`。候选快照要到第一次提交之后才有。
      - **按键**（契约 D7）：
        - 上下键选；Tab 或回车补成 `@<id> `；Esc 与列表开着时的第一次 Ctrl-C 只收起。
        - 列表开着时回车不提交。
        - 忙时不弹：忙时敲的 `@`，要回到空闲后重敲。粘贴进来的 `@` 不开补全。
        - 只按 id 前缀筛、大小写折叠，不按显示名筛（点名解析只认 id）。
      - **已知代价**：
        - `ESC[5 q` / `ESC[0 q` 会把用户自己设的光标形状复位成终端默认。
        - 句末是点名、已经敲完整个 id 时，第一次回车只补一个空格，第二次才提交。至少有 1 个匹配：敲完的 id 若是别的 id 的开头部分（`opus-5.0` 与 `opus-5.0-max`），列表有两行，敲完的那个排第一、默认选中。
        - 列表开着时，`@` 后面那一格可能同时看到输入框的反白方块与钉住的闪烁竖线。
        - `@` 前面是括号、中文或全角标点时也弹列表（与点名解析同一条左边界）。
      - 待 Griffin 真机时说观感的几条见 `19b` §7。
    - **AT1-FU 落地记录（2026-09-29，`ee247c6`，真机未复验）**。契约归档 `19c_AT1FU_FOLLOWUP_CONTRACT.md`，处理 AT1 独立审查的发现（`19b` 文件头）。
      - **Ctrl-V 解绑**：textarea 自己的 Ctrl-V 读剪贴板之后走的那条路，既不收起列表，也不换掉 X44 的提示。现在粘贴只剩终端的括号粘贴一条路。代价：终端把 Ctrl-V 原样交给程序时它什么都不做。Windows Terminal 下走不到这条路，主代理定不加提示。
      - **补的担保**（只加测试）：列表开着时，带修饰键的回车与 Tab 只补全、不提交；带修饰键的上下键、Esc 照常；Ctrl+Shift+C 只收起、不退出；Ctrl-T 挪走 `@` 时列表收起。
      - 四处过时注释改准。
      - **测试纪律**：所有 Ctrl-V、Ctrl+Shift+C 返回的命令一律不执行，只比函数指针：单元测试不碰真剪贴板。集成员在第一次跑测试之前先审计。
      - 委派链：
        - `model.go` 五个补丁由 grok-4.7 实现，与契约逐字节相同；
        - `atf_review_test.go` 由 Opus 测试腿写成；
        - Opus 集成员：8 条变异全杀；
        - Opus 独立审查 1 条 minor：列表分支 Ctrl-C 判据的 Alt 那一半没有用例，改坏了 Ctrl+Alt+C 会退出程序，不花钱。这是契约接受的缺口，记在 `19c` 文件头。（2026-10-01：CL1 已补用例，连同 NumLock、CapsLock 开着时的 Ctrl+C，见 #50。）
      - 验证：全量九个包绿。
      - ~~真机里「复制文字再按 Ctrl-V」那一步默认不做。~~ **订正（2026-09-30，主代理）**：这一步可以做；标「默认不做」是主代理的记忆索引写过时了。
    - **CH1 落地记录（2026-09-29，`4d3dc0b`，真机未复验）**。契约归档 `19a_CH1_CONVERSATION_MODE_CONTRACT.md`（先读文件头的「落地后订正」）；用法写在 `README.md`「对话模式怎么用」。
      - **行为**：
        - 裸文本记下之后自动问：没点名问全部活跃成员（去掉总结成员），`@` 了谁只问谁，多人回复后自动总结。
        - 经 `/round <ids> --go` 那一条路，闸、在飞记录、记账一个不绕；手敲 `/round` 名单为空照旧拒绝。
        - REPL 零值只记下，app 默认打开，`-no-auto` 与 `/auto on|off` 切换，`/say` 只记下。
        - `/add` 候选藏总结成员；`/help` 加 `/auto`，`/session` 一行补「先结清」。
      - **「想点名却一个都没认出来」谁都不问**（`hasStrayAt`，Griffin 23:00 的原话「如果不 @ 成员，默认是所有加入的都回复」定了方向）。判据改了四次，定为按成员判：
        - 全角 `＠` 一律算没认出；
        - 半角 `@` 跳过空白、标点、符号（遇换行停）之后，若是配置 agent id、会话成员 id 或配置显示名（不分大小写），就算没认出；
        - 否则是没点名，问全员。所以贴进来的代码里的 `@`、`@所有人` 都问全员。
      - **点的不是成员**（`@params`、`@staticmethod`、`@all`）：照 v1 的主代理裁定谁都不问，提示里给了 `/round` 出路。
      - **委派链**：
        - 生产代码 8 个文件与 6 处授权测试改动由 grok-4.7 实现（只拿契约 §1、§3、§5 的摘录），与契约逐字节相同；
        - 四个 `chx_*_test.go` 由 Opus 测试腿写成；
        - Opus 集成员：86 条变异全杀；
        - Opus 独立审查 4 条 minor，记在 `19a` 文件头。
      - **验证**：全量九个包绿；22 行 slowA `-count=10`；全部 TestChx `-count=3`。
      - **主代理定的细节**见 `19a` §7.1；**待主代理**见 `19a` §7.2 与文件头：
        - `/add` 提示没排除总结成员，后续小刀；
        - ~~`/team use` 会 Join 总结成员~~：CL1 已修（#50）。
      - **待 Griffin**（`19a` §7.3 与文件头，真机用过再定）：
        - (1) 只有 judge 的会话每句打「这一轮没有人可问」；
        - (2) 没点名时 worker、manager 模式与 workspace 权限的成员也会被问，而且带写盘权限（他今天的成员全是 discussion）；
        - (3) judge 行与「没配总结成员」提示每句都打；
        - (4) `@` 左边紧贴字母数字时问全员；
        - (5)-(9) 各种 `@` 写法的实测清单；
        - `@params` 这类要不要改成问全员（代价：敲错 id 也问全员）。

48. **首次运行自动配置（OB1，2026-09-29，Griffin）** —— **已落地（OB1 `3317940`，契约 `19d`；OB2 `70025db`，契约 `19e`；OB3 `ef76eef`，契约 `19f`），真机未复验**

    Griffin 原话：「就是说用户直接安装我们的东西，就能直接自动检测本地的登录的订阅对吧」；主代理核对后答复「只对一半」（下面四条缺口），他说「要做」。
    - **现状（核过代码）**：
      - 没有配置文件就不启动，只打印一份模板（`internal/app/app.go:339-347`，模板里写着「council 不会替你创建这个文件」，是 Step 4 的设计）；
      - `/add` 只识别 `[providers.*]` 里写了的工具（`internal/repl/addmodel.go:59`），不看本机装了什么；
      - 不单独检查登录，没登录时识别表那一家是「一个模型都没列出来（没登录？）」；
      - 模板是 Griffin 私人的 FSDT 示例成员，注释里有他本机的路径。
    - **方向（Griffin 定）**：用户装上 Council、第一次运行，就自动找出本机装了哪几家工具（claude、grok、agy、codex），检查能不能用，生成配置文件。**这推翻了 Step 4「不替用户创建配置文件」的做法**，但只在默认位置、文件不存在时；用户用 `--config` 或环境变量指定了路径却找不到，照旧报错。
    - **主代理定的范围（契约 OB1 冻结细节）**：
      - 只在默认配置位置、文件不存在时触发；不覆盖任何已有文件（`O_EXCL`）；
      - 检测按固定顺序，先查 PATH，再查几处已知安装目录；配置里写解析出的绝对路径；
      - 「能不能用」复用现有的模型识别（零花费），屏幕上照识别实际证明的说法写（「识别到 N 个模型」），不写成「已登录」；
      - 一家都没找到时不写文件，说明去哪装、退出；写不进去时把内容打出来让用户手存、退出；成功后打三行上手说明（`/new` → `/add` → 直接打字），然后照常启动；
      - 生成的配置只有 `[providers.*]`，没有成员、没有 `[summary]`，成员照旧用 `/add` 挑（v1.2 主代理补：另写一行 `data_dir = '<home>\.council\data'`，因为内置默认 `E:\council-data` 只在这台机器上有，新机器会在打开库时失败）；
      - 模板去掉私人示例与路径；
      - 已有配置的用户：`/add` 在本机装了、配置里没写的工具旁边打一行提示，给出要追加的那几行，不自动写。
    - **OB1 落地记录（2026-09-30，`3317940`，真机未复验）**。契约归档 `19d_OB1_FIRST_RUN_CONTRACT.md`（先读文件头的「落地后订正」）。
      - **四家「没登录时」的实测**（隔离环境，只改子进程的配置目录，零花费）：没登录的 claude、grok、codex 照样能列出一部分模型，agy 查不出。所以「识别到 N 个模型」不等于登录了。
      - **登录状态按证据分三档**：
        - claude 读 initialize 回应里的 `account`：有 `subscriptionType` 算已登录，并显示它的原值（本机是 Claude Max）；`tokenSource` 为 none、既没有 `subscriptionType` 也没有 `apiKeySource` 算没登录；其余查不出。
        - grok：`models` 首行是 `You are not authenticated.` 算没登录，其余查不出。Griffin 的 grok 走 API key（首行 `Model '…' is using its own API key.`），显示「查不出」。
        - codex：跑 `codex login status`，退出码 0 且有 `Logged in` 开头的行算已登录，退出码非 0 且有 `Not logged in` 算没登录。
        - agy 一律查不出。
      - **待 Griffin（一个问题）**：配了 API key 算不算「已登录」？包括 grok 的几种 key 首行、claude 的 `apiKeySource`。现在显示「查不出」并附登录办法，是实话、不花钱。
      - **委派链**：
        - 生产代码分四块，四路 grok-4.7 并行，与契约逐字节相同；
        - 5 个 obx 测试文件由两条 Opus 测试腿写成；
        - Opus 集成员：98 条变异全杀，每次运行后四项泄漏核对都干净；
        - Opus 独立审查 5 条 minor，记在 `19d` 头。
        - 起草期间有一次测试泄漏，建出了 `E:\council-data`，主代理核实不是 Griffin 的数据后删了；此后契约加了 canary 运行纪律。
      - **验证**：全量九个包绿（在 canary 隔离环境下跑）。
      - **真机复验**：用临时 `USERPROFILE` 模拟新用户，零花费，步骤在 `19d` §6 第 5 步与 `14_MVP_CHECKLIST.md` 的 A3。
        - 模拟时 claude、grok 会显示「没登录」（它们读的是临时 home），codex、agy 仍读真实登录态。
        - 不要用「把真配置改名」来测：生成的 `data_dir` 就是 `C:\Users\25142\.council\data`，也就是日常库。
    - **追加裁决（2026-09-30，Griffin）**：主代理问「配了 API key 算不算已登录」，他答「支持CPA的那种吧」。
      - 配了 API key（包括 grok 经 CPA 走的 key）算正面状态，显示「已登录（API key）」这一类，不再显示「登录状态查不出」，也不附登录办法。
      - 主代理定的细节（契约 OB2）：
        - grok：首行 `Model '<模型>' is using its own API key.`（本机实测，CPA 就是这种）、`You are using XAI_API_KEY`、`You are authenticated via deployment key` 算「已登录（API key）」；`You are logged in with …` 算「已登录」。后三句只在程序文本里见过，没实跑，契约照实标注。
        - claude：`apiKeySource` 非空算「已登录（API key）」，显示它的原值。
        - codex：API key 登录与 ChatGPT 登录能分就分，分不了就照旧「已登录」。
        - agy：不变。
      - 同一刀顺带补 OB1 审查剩的三处测试覆盖与一处注释。`/add` 识别表里也显示登录状态是另一件事，没排进来。
    - **OB2 落地记录（2026-09-30，`70025db`，已装 `council 0.2.0-0930b`，真机未复验）**。契约归档为 `19e_OB2_API_KEY_LOGIN_CONTRACT.md`，读之前先看文件头的「落地后订正」。
      - **主代理定的细节**（19e §1.1 E1-E12）：
        - **第四档**：新加 `LoginAPIKey`。屏幕上是「已登录（API key）」；claude 还带 key 来源的原值，例如「已登录（API key：ANTHROPIC_API_KEY）」。这一档不列登录办法。
        - **claude**：按顺序取第一个成立的判据：
          - 有订阅（原判据）→「已登录（订阅：…）」。订阅与 key 同时在时显示订阅。
          - `apiKeySource` 非空、不是 `none`、不含控制字符 →「已登录（API key）」。
          - 原来的「没登录」判据不变。
          - 其余 →「查不出」。
        - **grok**：看首行，去掉首尾空白后比对：
          - `Model '<名>' is using its own API key.`（前后两头卡住，名字非空）、`You are using XAI_API_KEY.`、`You are authenticated via deployment key.`（后两句要整句相等）→「已登录（API key）」；
          - `You are logged in with <…>.` →「已登录」；
          - `You are not authenticated.` →「没登录」（不变）；
          - 其余 →「查不出」。
        - **codex**：`codex login status` 退出码 0，且有一行以 `Logged in using an API key` 开头，或整行是 `Logged in using Amazon Bedrock API key` →「已登录（API key）」。其余的 `Logged in` 仍算「已登录」（Griffin 用的是 ChatGPT 登录，属于这一种）。
        - **agy**：不变，查不出。
      - **证据等级**：
        - 实测过的：grok 的 CPA 那一句、codex 的 ChatGPT 那一句，都是本机零花费跑的。
        - 只在程序本体里读到、没实跑的：grok 另三句、codex 两句 API key、claude 的 `apiKeySource`。
        - 判据全是整句相等或两头卡住。CLI 改了措辞，一律退回「查不出」。版本是 claude 2.1.283、grok 1.0.41、codex-cli 0.147.0，升级之后按 19e §0 第 3-5 条重核。
      - **补上 OB1 审查剩下的**（19d 头的 R0、R1、R2、R4）：
        - `RunLoginStatus` 超时杀进程：L5、L5b；
        - 首次运行里 codex login status 受单家超时约束：B3；
        - `cmd.Dir`：L6；
        - `reportConfigError` 头注释：只改注释。
      - **委派链**：
        - 冻结契约 v1.1：W1 报 4 条 minor，全收，v1.1 经定向核验通过。
        - 生产代码分两块，两路 grok-4.7 并行实现，与契约逐字节相同。
        - 3 个 oby 测试文件由一条 Opus 测试腿写成。
        - Opus 集成员：40 条变异全杀，N31、M59 核过是等价的；每次运行后四项泄漏核对都干净。
        - Opus 独立审查：3 条 minor，记在 `19e` 头。其中一条是「不安全方向缺负对照」：grok 的 CPA 句与 XAI_API_KEY 句放宽成句前可多字时，没有用例拦得住；生产代码是对的，留给后续小刀。
      - **验证**：worktree 与 main 上全量九个包绿（canary 隔离环境）；`TestOb[xy]` 三包 `-count=3` 绿。
      - **真机复验**：步骤在 `19e` §6 第 5 步与 `14_MVP_CHECKLIST.md` 的 A3。模拟时要先设 `GROK_HOME` 指回自己的 `.grok`，grok 那一行才会是「已登录（API key）」；不设的话 grok 读临时 home，显示「没登录」。
      - **待 Griffin**（19e §7）：
        - (b) codex 的 Bedrock 那一句算 API key；
        - (c) 订阅与 key 同时在时显示订阅；
        - (d) 正面判据大多只有程序文本。
        这三条都不影响他本人的配置。
      - **待主代理**：
        - ~~`/add` 的识别表显示登录状态（候选的下一刀）~~：OB3 已做，见下；
        - CPA 不通时 grok 的首行（「配了 key」不等于「key 能用」）。2026-10-01 真撞上了一次：经 CPA 的 grok 调用被上游以 HTTP 426 拒绝（见 #50），这时首次运行与 `/add` 照样显示「已登录（API key）」；
        - ~~`killProcessTree` 按 PATH 找 taskkill~~：CL1 已改成系统目录里的 taskkill.exe（#50）；
        - ~~上面审查报的三行负对照~~：OB3 已补。
    - **OB3：`/add` 显示各家的登录状态（2026-09-30，Griffin）**。主代理问要不要在 `/add` 的识别表里也显示登录状态，他答「可以」。细节由主代理定。
    - **OB3 落地记录（2026-09-30，`ef76eef`，他说「装」后装为 `council 0.2.0-0930c`，真机未复验）**。契约归档为 `19f_OB3_ADD_LOGIN_STATUS_CONTRACT.md`，读之前先看文件头的「落地后订正」。
      - **主代理定的细节**（19f §1.1 D1-D12）：
        - **在哪显示**：不带参数的 `/add`、`/add --refresh`（连同 `/join`，同一个 handler）的识别结果之后，多一段「各家的登录状态（本会话的快照，/add --refresh 重新检查）:」，位置在「本机装了 …」提示之前。配置里能识别的每一家各一行，顺序同「正在识别模型: …」，对齐同首次运行。`/add +编号`、`/add <id>` 不打这一段。
        - **判据与首次运行完全相同**：
          - claude、grok 从识别那一次的输出判；
          - codex 另跑一次 `codex login status`，与识别共用这一家的一个超时，父 ctx 取消能传到它；
          - agy 查不出；
          - 识别失败的那一家也有一行。
        - **只有一份说法**：四档说法与登录办法从 app 挪到 repl（`LoginText`、`LoginCommand`），首次运行改用它们，屏幕输出一个字节不变。例外是一家都没找到时的安装说明 `msgFirstRunNoneFound`，它另写着登录办法（19f §7 (j)）。
        - **只有「没登录」的那一家在行末带登录办法**：格式是「在终端里运行 <命令>，登录之后 /add --refresh」。查不出的不带，因为 agy 永远查不出，每次都提示是噪声；已登录、已登录（API key）不带。
        - **快照**：登录状态随识别快照留到本会话结束，只有 `/add --refresh` 重新检查。取消时整份快照丢掉，与原来一样。快照之后从配置删掉的 provider 照打，与失败行同一个处置。
        - **开关**：`REPL.SetLoginStatusExec` 零值 nil 时不检测、不打这一段、不多起进程，所以直接 New 出来的 REPL 输出不变。app 在 TUI 与行式两处都装 `turn.RunLoginStatus`。
        - **代价**：每次重拍快照多跑一次 `codex login status`（19d 实测 0.16 s），零花费。
      - **顺带补上**：OB2 审查剩下的 grok 三行负对照（句前多字）；oby 测试文件头一行注释。
      - **既有测试的授权改动**：A10 `TestObxAddHintLineMode` 的期望。它用真 `app.Run` 敲 `/add` 并钉住结尾，登录段正好夹在识别表与「本机装了」提示之间。
      - **委派链**：
        - 冻结契约 v1.1：W1 报 1 条 major（「共用超时」与「取消传到登录状态命令」没有用例钉住，反驳者证实）、2 条 minor，全收；v1.1 经定向核验通过。
        - 生产代码分两块，两路 grok-4.7 并行实现，与契约逐字节相同。
        - 3 个 obz 测试文件由一条 Opus 测试腿写成。
        - Opus 集成员：36 条变异全杀，每次运行后四项泄漏核对都干净。
        - Opus 独立审查 1 条 minor，记在 `19f` 头。
      - **验证**：worktree 与 main 上全量九个包绿（canary 隔离环境）；`TestOb[xyz]` 五个包 `-count=3` 绿。
      - **真机复验**：用他自己的真配置，零花费，步骤在 `19f` §6 第 5 步与 `14_MVP_CHECKLIST.md` 的 M6。预期是：
        - agy 登录状态查不出；
        - claude 已登录（订阅：Claude Max）；
        - codex 已登录；
        - grok 已登录（API key）。
        已装 `0.2.0-0930c`（含 OB3）。
      - **待主代理**（19f §7）：
        - ~~`LoginCommand` 的注释说多了（19f §7 (k)）~~：CL1 已改（#50）；
        - 改 `loginCommands` 时要同步 `msgFirstRunNoneFound`（19f §7 (j)）。

49. **`/add` 按名字加识别出的模型（AD1，2026-09-30，Griffin）** —— **已落地（与本条同一个提交），2026-10-01 他说「装」后装为 `council 0.2.0-1001a`，真机未复验**

    Griffin 在真机（0.2.0-0930c）上敲了下面几条，都被「配置里没有可加入的 …，不带参数的 /add 能列出可加的」挡了回来：
    - `/add gpt-6.1-sol xigh`
    - `/add gpt-6.1-sol-xhigh`
    - `/add gpt-6.1-sol-high`

    他问「这不行啊？？？什么情况」。主代理解释之后，他说：「我需要能/add 加入订阅的，不是说只有我们事先选好的那种，你懂吧」。

    主代理在 E 盘机上跑 `codex debug models`（零花费），实测 gpt-6.1-sol 在列表里，强度档是 low、medium、high、xhigh、max、ultra，codex 自己的默认档是 medium。
    - **方向（Griffin 定）**：识别出来的订阅模型，都能直接 `/add` 加入，不必先写进配置，也不只限于配置里事先选好的成员。
    - **主代理定的细节**（契约 AD1，归档为 `19g`；D1-D17）：
      - 怎么敲：
        - 识别表 ID 列的名字，例如 `/add gpt-6.1-sol-xhigh`；
        - 模型名加强度，`/add gpt-6.1-sol --effort xhigh`；
        - 两个词，`/add gpt-6.1-sol xhigh`，中间可以是全角空格。
        名字不区分大小写。
      - 加入的过程与 `+编号` 是同一套写配置流程：同脑子检查、id 检查、守卫、先备份再追加、加载校验、失败回滚、换指针、Join。屏幕上的输出也相同。
      - 快照里的所有行都能按名字加，包括已在配置里、没有编号的行。16a 的 open item (c)（已在配置里的模型换一档强度只能手改配置）就此解决。
      - 本会话还没识别过时，先自动识别一次（打「正在识别模型」）。取消语义与不带参数的 `/add` 相同。
      - 配置里的解析（总结成员、全名、编号、候选前缀）照旧优先，不识别、不起进程，行为不变。只有给了明确的强度（带强度的 ID、两个词、`--effort`）时，识别结果才压过配置里的「只是前缀命中」。
      - **只敲模型名、不写强度**时，先看配置里同一个 provider、同一个模型的成员（按模型判，不按 id 前缀判）：
        - 没有：按默认档 high 新建，写配置；
        - 有在场的：拒绝，点名这些成员，不写；
        - 恰好一个不在场：加入它，不写；
        - 好几个都不在场：列出来，不写。
        所以同一条命令敲几次，结果都一样，也不会给会话里已有的模型再加一个重复的成员。只给 `--id` 算作明确要新建。
      - 强度敲错（`/add gpt-6.1-sol xigh`）时，打「<模型> 支持的强度是 …，没有 xigh」，不写配置。
      - 一个名字对上多个模型时，整条拒绝，逐行列出候选，不猜。
      - 多段里只要有一段要按识别结果加，就整条拒绝：识别出的模型一次加一个。
      - 配置里、识别结果里都对不上时，新说法同时指出两条路。
      - 开关：`REPL.SetAddByName`，零值关着时与 AD1 之前逐字节相同。例外是全角空格那一条，它不看开关。app 在 TUI 与行式两处都打开。`/join` 与 `/add` 是同一个 handler，同样生效。
      - 不做：
        - 不加识别结果里没有的模型（不盲写模型名）；
        - 不改识别本身，不改识别表；
        - 不改 `+编号`；
        - 不改 `/help`，因为它被既有测试逐字钉着（19g §7 (e)）；
        - 新成员的权限默认值不变，仍是 read、discussion。
    - **委派链**：
      - **冻结契约**：经四轮修订，定稿 v1.3：
        - W1 报 1 条 major：开关打开时，配置里的拒绝型命中没有用例担保；
        - W1b 报 1 条 major：成员在场时，只敲模型名会新建重复成员，敲两次结果不同；
        - 两条都由反驳者证实，照主代理的统一规则改了；
        - W1c 按写死的严重度判法，没有 major；
        - v1.3 经定向核验通过。
      - **生产代码**：分四块，三路 grok-4.7 并行实现，与契约逐字节相同。
      - **测试**：2 个 adn 测试文件由一条 Opus 测试腿写成。
      - **Opus 集成员**：
        - §4 的 92 条变异里 90 条被杀，V7、V70 等价；
        - 774 次点名杀手检查全部变红，没有漏的，也没有多的；
        - 每次运行后四项泄漏核对都干净。
      - **Opus 独立审查**：3 条 minor，记在 `19g` 头。
    - **验证**：
      - worktree 与 main 上全量九个包绿（canary 隔离环境）；`TestAdn` 在两个包上 `-count=3` 绿。
      - 审查员另做了差分探针：开关关着时约 70 条命令，与 HEAD 逐字节相同。
    - **真机复验**：用他自己的真配置，零花费，会写配置、写前自动备份。步骤在 `19g` §6 第 5 步与 `14_MVP_CHECKLIST.md` 的 M7。已装 `0.2.0-1001a`（含 AD1）。
    - **待定**（19g §7）：
      - ~~`/help` 要不要补一句~~：CL1 已补（#50），19g 头的 `--id` 缺值守卫用例与两处注释也一并做了；
      - 按名字加自动识别时不打失败行；
      - 敲错配置里的名字，本会话第一次要先等一次识别；
      - 带强度的单个 ID 在配置里只是前缀命中时，仍按 HEAD 的前缀语义加入（可能是另一个模型）；
      - 已从配置删掉、但仍在场的成员不进同模型判断。

50. **CL1 一批小修补（2026-10-01，Griffin）** —— **已落地（与本条同一个提交，`5c4840a`），2026-10-01 他说「装」后装为 `council 0.2.0-1001b`，真机未复验**

    主代理列了几件不急的小修补，Griffin 说「开始做那批小修补吧」。范围与细节由主代理定，契约 CL1 归档为 `19h`。
    - **改了什么**（七处）：
      1. `/help` 里 `/add` 那一行补上按名字加：「识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号……」（#49 的待定项）。
      2. `/help` 里 `/team` 那一行补「（总结成员除外）」。
      3. `/team use` 不再把总结成员当普通成员加入。team 的 members 里有 `[summary]` 的 agent、它在本会话里又还不是成员时，打一行「<id> 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结」，不 Join。已在场、已退出的两支不变。这是 #47 的规则在 `/team use` 上的漏口；其余四处 Join 都核过，只有这一处。
      4. `killProcessTree` 不再按 PATH 找 taskkill：改用 `windows.GetSystemDirectory()` 拼出系统目录里的 taskkill.exe，不读 SystemRoot；出错时什么都不起，绝不退回按 PATH 找，三处调用方的 `Kill` 兜底不变。以前 PATH 前面若有同名的 taskkill，就会执行它（HEAD 实测）。加了一个包级接缝 `taskkillPath` 供测试用。
      5. AD1 的 `--id` 缺值守卫补用例；AD1 的两处注释写准；OB3 的 `LoginCommand` 注释写准。
      6. 列表开着时按 Ctrl+Alt+C，以及 NumLock、CapsLock 开着时按 Ctrl+C，只收起列表、不退出，补了用例（19c 头的缺口；锁定键那一半是 W1 审查员查出来的）。
      7. 两处过时的测试注释写准。既有测试的授权改动：`/help` 两处期望，以及 L5b 改用接缝（不再靠清空 PATH）。
    - **委派链**：
      - **冻结契约**：v1 → W1（没有 major，5 条 minor，全收）→ v1.1 → 定向核验（两处计数写错，主代理直接改了契约文字）。文件头一开始就写死了严重度判法。
      - **生产代码**：分两块。**这一次两路 grok 都没交出代码**，原因是基础设施，不是任务本身：经 CPA 的 grok 调用被上游以 HTTP 426 拒绝，报「Grok CLI version (0.2.120) is outdated」，而本机 grok 是 1.0.41；连只让它回「OK」的极小提问也一样。按委派链，两块改由 Opus 实现，与契约逐字节相同。
      - **测试**：3 个 clx 测试文件由一条 Opus 测试腿写成。
      - **Opus 集成员**：22 条变异中 20 条被杀，E1、E2 等价；每次运行后四项泄漏核对、残留进程核对都干净。
      - **Opus 独立审查**：1 条 minor，记在 `19h` 头。
    - **验证**：worktree 与 main 上全量九个包绿（canary 隔离环境）；`GOOS=linux go vet` 干净。
    - **真机复验**：零花费，用沙盒配置，不碰真配置。步骤在 `19h` §6 第 5 步与 `14_MVP_CHECKLIST.md` 的 M8。已装 `0.2.0-1001b`（含 CL1）。
    - **经 CPA 的 grok 报 426：已解决（2026-10-01）**。这不是 Council 的代码问题，过程如下：
      - 他先把本机 grok CLI 从 1.0.41 升到 1.0.46，主代理用极小提问测，grok-4.7、grok-4.6 仍是同一个 426（报的仍是 0.2.120）；
      - 他接着在 Mac 上更新了 CPA，再测，两个型号都回「OK」。
      根因是 CPA 转发时自报的客户端版本太旧。以后再见到 426，先更新 CPA。
    - **待定**（19h §7）：
      - 守卫提示让重敲的命令换档；
      - `msgFirstRunNoneFound` 与 `loginCommands` 的同步；
      - 当前目录同名文件遮住 PATH 上的安装；
      - codex 失败轮 usage_raw；
      - 「三家」这类计数；
      - SystemRoot 坏掉时 taskkill.exe 自身失败、只剩 Kill 兜底（HEAD 相同）。

51. **GitHub 上的仓库换成不带历史的干净版本（2026-10-01，Griffin）** —— **已强推（本条所在的提交是仓库里唯一的提交），仓库保持私有**

    Griffin 问能不能上线。主代理答：自己用已经在用；改公开之前，要清掉局域网 IP 和本机环境问题的记录，而且这些在旧提交里还看得到，只删文件不够。他定：清干净，用不带历史的版本覆盖 GitHub 上的私有仓库，仓库保持私有。
    - **清了什么**：
      - `docs/reference/step0-probe-results.md` 里 CPA 的局域网地址；
      - `.handoff/HANDOFF.md`、`00_DECISIONS.md`、`14_MVP_CHECKLIST.md`、`19c` 里本机环境问题的细节，改成中性的说法；
      - `17b` 里记的本机时区。
      - 每个文件的行数不变，别处「文件:行号」的引用不会错位。
      - 改完之后，两个只读检查员各扫了一遍：一个查密钥与地址，一个查残留的说法与个人信息。没有发现密钥、令牌或私钥。
      - 检查员报了、但没改的低风险项：CPA 跑在同一 WiFi 的 Mac 上这种说法（约 9 行，不含地址）；Claude Max、codex 用 ChatGPT 登录这类订阅说法；测试夹具里真实探针抄来的会话 id、request id（不是凭证）；作者邮箱、用户名 25142 的路径、机器代号、「Griffin」这个名字。
    - **历史去哪了**：
      - 274 个旧提交（到 `883a1cd` 为止）只留在本地：分支 `history-backup-20261001`，以及仓库外的 `D:\DeskTop\My_Project\council_history_20261001.bundle`。
      - **这个分支和 bundle 都不推。**
      - 云端会话留下的远端分支 `claude/clever-pascal-jq28k2` 已经全部合进旧 main，一起删了。
    - **对读文档的影响**：各文档里引用的提交号（`4202b6c`、`883a1cd` 之类）在 GitHub 上查不到了，要在本地备份分支里 `git show`。
    - **还没解决的**：被强推覆盖掉的旧提交，在 GitHub 上按提交号直接访问可能还能看到一段时间，要等 GitHub 自己清理，或者请 GitHub 支持清除。将来要公开时，更稳的做法是新建一个仓库推过去，而不是把这个仓库直接改成公开。

52. **Griffin 自用版收尾：真机用过之后说「可以 ok 了」；他的配置里 Opus 5.0 换成 Opus 5.5 max 与 Sonnet 5.5 max（2026-10-03，Griffin）** —— **已改配置（不在仓库里），代码没动，仍是 `council 0.2.0-1001b`**

    Griffin 定的用途是「我自己用」。主代理给了 A3、M6、M7、M8 的精简复验步骤，他在真机上用过之后说「我觉得可以 ok 了」。
    - **验证等级**：他没有逐项回报。能核对的只有一条：他配置旁边多了 `config.toml.bak-add-20261003-210359`，配置末尾多了 `gpt-6.1-sol-xhigh`，说明 M7 第 2 步（按名字加、自动识别、写配置前备份）做过、而且写对了。其余各项按他的总体判断记，不算逐项复验通过。
    - **配置改动**（`C:\Users\25142\.council\config.toml`，改前原样在同目录的 `config.toml.bak-20261003-opus55max-sonnet55`）：
      - 删掉 `opus-5.0`（`model = 'opus'`，high）与 `opus-5.0-max`；
      - 加 `opus-5.5-max`（`claude-opus-5-5`，max）与 `sonnet-5.5-max`（`claude-sonnet-5-5`，max）；
      - `teams.all` 与 `teams.max` 里的 opus-5.0 换成这两个。原有的 `opus-5.5`（xhigh）不动。
      - 主代理用配置的一份拷贝（data_dir 指到隔离目录）启动 council，`/agents` 列出 9 个 agent、2 个 team，加载与校验都过了。
    - **改之前查的**：用 claude CLI 2.1.283 的初始化握手（与 `/add` 识别同一套，不推理、不花钱）：
      - 别名 `opus` 已经解析成 `claude-opus-5-5`，所以原来叫 `opus-5.0` 的两个成员其实早就在跑 Opus 5.5；
      - Opus 5.5 支持 low/medium/high/xhigh/max；
      - 别名 `sonnet` 解析成 `claude-sonnet-5`，列表里**没有 Sonnet 5.5**。`sonnet-5.5-max` 照他的要求写了 `claude-sonnet-5-5`。**同日他在终端里实跑 `claude -p … --model claude-sonnet-5-5 --effort max`，回了 OK，能用**（CLI 只警告「这个版本的目录里没有这个型号」，不拒绝）。实跑前他的 claude 登录曾过期（`OAuth session expired`），他重新登录后才通；Council 里是否走同一条路没有另测，第一次 `/ask sonnet-5.5-max` 即是。
    - **对旧会话的影响**：旧会话里的 `opus-5.0`、`opus-5.0-max` 成员还在，但配置里没有了，`/ask` 它们会被拒，不花钱。

53. **Ctrl-C 改成 2 秒内连按两次才退出或取消本轮（CC2，2026-10-04，Griffin）** —— **已落地（与本条同一个提交 `509e651`，2026-10-04 他说「合并到main吧」后经 PR #1 快进合并进 main），同日他说「审完直接装上」后装为 `council 0.2.0-1004a`，2026-10-05 他真机复验后说「都ok了」**

    Griffin 在 VS Code 终端里用 council，一选中文字就退出，他没有按 Ctrl+C。主代理查到原因：本机 Cherry Studio 2.1.4 开着划词助手，它用的 selection-hook 2.1.1 取不到选中文字时，先模拟 Ctrl+Insert，约 100ms 内剪贴板没变化再模拟一次 Ctrl+C（上游 `src/windows/core/engine.cc` 的 `GetTextViaClipboard`）。council 的 TUI 把 Ctrl+C 当普通按键收（ultraviolet 关掉了 `ENABLE_PROCESSED_INPUT`），空闲时单按一次立即退出，忙时单按一次立即取消正在跑的一轮。他定：退出和取消都改成 2 秒内连按两次。
    - **新语义**：
      - 列表不在屏幕上时，第一次 Ctrl-C 只武装：空闲时状态栏「再按一次 Ctrl-C 退出」，忙时「再按一次 Ctrl-C 取消本轮」。2 秒内（含 2 秒，按 Model 的 now 时钟）第二次才退出或取消。
      - 其它按键、粘贴、2 秒到期都会解除武装并恢复状态栏；submitDone 也解除。
      - 修饰键残影不算别的键：单独按 Ctrl、Shift、Alt 时控制台可能送来一条字符为 0 的按下记录，被写成 NUL、解成 Ctrl+Space（或 Ctrl+@）；它和修饰键码本身都不解除武装，否则「按 Ctrl-C、松开 Ctrl、再按 Ctrl-C」会永远退不出。这一条是审核员读代码查出的风险，没有实测哪个终端会送这条记录。
      - 不变的：@ 补全列表在屏幕上时 Ctrl-C 只收起列表；收口窗口里 Ctrl-C 仍被挡；`/quit` 照旧立即退出；行式 REPL（管道模式）的 Ctrl+C 走信号，不受影响。
      - 忙时按 Enter 的提示改成「一轮正在跑，先等它回来或连按两次 Ctrl-C 取消」。
    - **代码**：`internal/tui/model.go`，新文件 `internal/tui/ctrlc.go`；`internal/tui/run.go` 只改一句注释。
    - **测试**：
      - 新文件 `internal/tui/ccx_ctrlc_test.go`，钉住窗口边界、到期与代次、各类解除、划词按键序列、修饰键残影、忙时取消。
      - 18 个旧测试函数按新语义改成连按两次：只换刺激，断言一条没删，输出字节断言一条没动。
      - 另改 B2-6：它不失败，但单按不再取消之后，它就不再验证「下一次提交清 canceling」；改成连按两次，并加了 canceling 为真的前提断言。
      - 改准十余处过时注释。
    - **委派链**：Griffin 指定这次由 grok 写码，不走 code-writer。grok-4.7 同一会话分 6 刀：生产代码、新测试、两批旧测试、两轮审核修正。code-reviewer（Opus）初审「可采用」，另提次要项与两个疑点；修正后复审仍为「可采用」（两轮都没有阻塞项和重要项）；最后按审核建议改准两处注释、补三条钉住判定条件的测试。
    - **验证**：隔离环境下 `go test ./... -count=1` 九个包全绿；`GOOS=linux go vet ./...` 干净；装饰性符号扫描零命中。
    - **推测过的局限（真机没有复现）**：xterm.js 源码显示带 Ctrl 或 Shift 的 Insert 不发任何字节（查的是本机另一款 VS Code 系 IDE 自带的 xterm.js）。据此推测在 VS Code 里每次划词 council 只收到一个 Ctrl-C，2 秒内划词两次仍会退出。2026-10-05 真机上这一项回报 ok、没有退出，推测没有复现；为什么没复现（VS Code 的 xterm.js 转发了 Ctrl+Insert，还是 VS Code 先复制了、划词助手没再发 Ctrl+C）没有查。从源头避免仍可用：selection-hook 支持按程序名过滤（`setGlobalFilterMode`）；Cherry Studio 设置里若提供应用过滤，把 `Code.exe` 排除掉。
    - **真机复验（2026-10-05，Griffin）**：他在 VS Code 终端里按下面四项试过（`council -no-auto`，划词助手开着），回「都ok了」，没有逐项细报：
      - (a) 空闲时划词一次：状态栏变成「再按一次 Ctrl-C 退出」，council 不退出。
      - (b) 2 秒内划词两次：不退出。
      - (c) 按住 Ctrl 连点两下 C：退出。
      - (d) 按 Ctrl-C、松开 Ctrl、2 秒内再按 Ctrl-C：退出。所以修饰键残影那条兜底至少没有挡住正常连按。
      - 没覆盖到：Windows Terminal；忙时连按两次取消本轮（要真跑一轮，花钱）。
    - **没改的文档**：契约文档（15、15a、15b、19b、19c、19h 等）与 `14_MVP_CHECKLIST.md` 里「单按 Ctrl-C 退出 / 取消」的说法没有逐份改，以本条为准。

54. **打开即用与 /resume 的第一刀：会话记下项目目录，新命令 /resume（CC-R1，2026-10-05，Griffin）** —— **已落地（与本条同一个提交，分支 `feat/resume-cut1`）；2026-10-05 他说「提交并装上」后装为 `council 0.2.0-1005a`（装前备份了真库到 `C:\Users\25142\.council\backups\pre-v6-20261005`，并用备份副本演练过 v5 到 v6 升级：2 个旧会话原样保留、`/resume` 与 `/resume --all` 正常；真库在用户第一次启动新版时自动升级），真机未复验**

    Griffin 问：在 VS Code 里打开 council，它知道当前项目吗？每次都要先 `/new` 不方便，想像 Claude Code 那样打开即用，用 `/resume` 在本机已有会话里选，不再靠 `/use` 敲 id。
    - **调研结论**（4 路读者加 4 路核查，44 条结论：29 条确认、15 条修正、0 条推翻）：
      - 之前 council 不认识项目：全仓非测试代码没有 `os.Getwd`；成员 CLI 固定跑在 `<data_dir>\workdir\...`（`spawn.go` 空 Dir 直接拒绝，理由是继承启动目录会读到项目级指令文件、写盘成员会把文件写进那个仓库）；`conversations` 只有 id、标题、创建时间、team_id。
      - 「每次 /new」其实是 `/new` 加 `/add`（或 `/team use`）：新会话没有成员，直接打字只会被记下；全仓没有默认班底的机制。所以 `/resume` 解决的是继续旧会话，开新会话要「打开即用」还需要默认班底（第二刀）。
      - 同一个 data_dir 同时只能跑一个 council（排他锁）：两个 VS Code 窗口各开一个，第二个启动失败。按项目隔离今天零代码也能做：每个项目用 `--config` 指向各自配置里的 data_dir（配置里的 data_dir 优先于环境变量）。
    - **拍板**（Griffin）：第一刀做「会话记项目目录」加「/resume」；默认班底走配置里的 `default_team`、启动时总是新建且第一句话才创建，这两条放第二刀；`/resume` 默认只列当前项目、`--all` 看全部；`/use` 保留；**成员 CLI 的工作目录不动**；项目目录取启动时的 cwd（规范化后），不向上找 `.git`。
      - 不动成员工作目录的理由：换 cwd 每个成员要重付一次入场费（`session.go` 记过实测 18945 对 1 个 input token）；claude 每轮往 cwd 写 `council-role.txt`，会污染项目仓库，它的会话还会混进用户在该项目里自己用的 `claude --resume` 列表；codex 会读入项目的 AGENTS.md。让成员知道项目背景，现成的办法是规则文件（`rules_file`）。
    - **改了什么**：
      - schema v6：`conversations` 加 `cwd TEXT NOT NULL DEFAULT ''`，单条 ALTER，和其它待跑的迁移在 `migrate` 的同一个事务里，失败整体回滚；老会话 `cwd` 为空即「未绑定」。
      - `conversation.Tx.SetConversationCwd`（空串允许，表示清空）、`Conversation.Cwd`、`Service.CreateIn`（同一事务建会话并盖章）。`CreateConversation` 与 `Create` 的签名没改（冻结契约 08a）。
      - `repl.NormalizeProjectDir`、`repl.SameProjectDir`、`REPL.SetProjectDir`（沿用 `SetConcise` 的 setter 体例，`repl.New` 签名不变）；`/new` 建会话时盖上启动目录，`/use` 不盖。
      - `/resume [--all] [<编号|id 前缀>]`：不需要当前会话。无参数列表按最后活动倒序，列有编号、当前标记、短 id、最后活动（本地时区）、成员数、消息数、标题，`--all`（或取不到启动目录时）多一列项目；默认过滤藏起的未绑定旧会话有提示行。≤3 位纯数字当编号（越界不回落成前缀），其余当 id 前缀并在全部会话里匹配；编号每次重新计算。接上后回显成员、消息数、最近 6 条。纯读，不触发成员调用，不写库。读库失败的会话该列显示 `?`，表后说明。
      - 启动横幅在「库文件」行之后加「项目目录」行与本项目会话数；既有各行不变。
      - 形状不符的错误文案不再建议「删掉重建」：那条建议只在 `486f5f3` 之前成立，现在旧版二进制打开 v6 库也会报这条错，照旧建议会让人删掉真数据。同步订正 `08a` 与 `08_STEP5_SPEC` 里的两句。`README.md` 的「第一次用」加了 `/resume`。
    - **委派链**（新规则 2026-10-04 的第一次完整跑通）：code-writer（Sonnet 5.5 max 把关，grok-4.7 分 10 刀执笔，会话 id `e33824bd-f66f-4565-93a7-837cb31c965a`）→ code-reviewer（Opus 5.5 max）初审「修改后可采用」（1 条重要：读库失败时最后活动列该显示 `?` 却显示了创建时间，且测试把偏差钉死；4 条次要：三处注释不准、一个排序键没被测试钉住、两处摆设断言）→ 回改一轮 → 复审第 1 轮「可采用」，没有遗留的阻塞或重要项；审核员对 code-writer 多做的一处（History 没报错却一条没返回时，该行最后活动也打 `?` 并加说明行）表示合理、同意保留。
    - **验证**：隔离环境下 `go test ./... -count=1` 九个包全绿；`GOOS=linux go vet ./...` 干净；改动与新增文件的装饰性符号扫描零命中；code-writer 在临时拷贝里注入 30 个错误（形状浅拷贝、读路径丢 `cwd`、`CreateIn` 不盖章、编号越界回落前缀、不过滤项目、两个装配分支漏调 `SetProjectDir`、`r.cur` 指向 store 返回的切片元素等），测试全部能抓到（这是 code-writer 的自报，主代理和审核员都没有复跑；已知测不出的一种：`r.cur` 指向 `loadResumeRows` 内部切片的元素，只靠 `attachResume` 按值收参数保证）。
    - **已知局限**：
      - 符号链接解析没验证（本机权限不足，该子用例被跳过）。code-writer 最初写过「EvalSymlinks 不展开 8.3 短名」，后来在有短名的 C 盘实测，已存在目录的短名会被展开成长名，那条局限不成立。
      - 目录联接（junction）、`subst` 盘、UNC 写法、macOS 上大小写不同的写法，会被当成不同项目。
      - 项目列（`--all` 或取不到启动目录时才有）含中文路径时，tabwriter 按 rune 算宽，标题列略有错位。
      - 会话很多时 `/resume` 每个会话读 2 到 3 次库，没有分页，没测过一千个会话的耗时。
      - 旧版二进制（`0.2.0-1004a` 及更早）打开 v6 库仍会打印旧文案「删掉重建」，那是旧代码，改不了。
    - **第二刀的待定点**：
      - 接上未绑定的旧会话时，是否顺手绑到当前项目（现在 `/resume` 是纯读，旧会话在默认列表里永远看不到，只能用 `--all` 或 id 前缀接上）；
      - `default_team` 与懒创建（第一句话才创建会话）；
      - 方向键选择器（复用 `@` 补全列表）；
      - 横幅是否列最近会话（现在只有计数，避免与 `/resume` 的排序不一致）。
    - **部署注意**：新版第一次打开真库会自动升到 v6；装之前先备份 `C:\Users\25142\.council\data` 里的 `council.db` 连同 `-wal`、`-shm`，并确认没有 council 进程在跑。回退就是还原备份，旧版二进制打不开 v6 库；升级之后新建的会话和新写的消息会随还原一起丢掉，想保住只能继续用新版。
    - **没改的文档**：`08a` 的接口清单没有登记 `CreateIn`、`SetConversationCwd`、`Conversation.Cwd`（同 `SetConversationTeam` 的先例，以本条为准）。
