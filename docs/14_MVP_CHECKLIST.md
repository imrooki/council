# Council MVP 验收清单（草案，2026-09-30，待 Griffin 定）

这是什么：把 Council 已落地、未做、暂缓的功能逐项列成可以照着敲的验收单，每项带状态、出处、花费与预期；MVP 该包括哪些，只是草案建议，由你定。
第二遍：按完整性审查的意见改过，逐条处置写在文末「附：审查意见的处置」。
怎么用：先照 §0 搭沙盒，再照 §2 的顺序先跑零花费一轮、再跑花钱一轮；每项看完勾「符合」或「不符合」，照 §5 的格式回报。
版本前提：草案按 council 0.2.0-0929a（vcs.revision e35b539，HANDOFF.md:2703）写成。**2026-10-01 已重装 `council 0.2.0-1001b`**（vcs.revision 6037094，含 OB1 `3317940`、OB2 `70025db`、OB3 `ef76eef`、AD1 `d4ad85f` 与 CL1 `5c4840a`）。各项预期里写的 `0.2.0-0929a`（版本行、TUI 表头）一律读作你装的版本号，其余的在两个版本上一样。只有五处说法变了：一是 `/add` 候选为空、前缀没命中、总结成员在场时的说法（19d §3.6）；二是首次运行（A3）；三是配了 API key 的算「已登录（API key）」（19e）；四是不带参数的 `/add` 在识别结果之后多一段登录状态（19f，见 M6）；五是配置里对不上的名字会先识别、再按名字加识别出的模型（AD1，19g，见 M7），不带参数的 `/add` 与配置里已有的名字行为不变。CL1（一批小修补，`5c4840a`，19h）也已装：`/help` 的 `/add`、`/team` 两行变了，`/team use` 不再把总结成员当普通成员加入，见 M8。

## 0. 怎么用

### 0.1 先核版本（零花费，不加载配置）

（PowerShell）

```
council -version
```

预期：一行 `council 0.2.0-1001b`（main.go:42、:53-55；草案写成时是 0929a，2026-10-01 重装）。不是这个就先停，别往下做。vcs.revision 只记在 HANDOFF.md:2703，清单不要求你另查。
二进制装在 `E:\Go\gopath\bin\council.exe`（HANDOFF.md:2544 记的位置；0929a 那条 HANDOFF.md:2703 没写路径，按惯例同一处）。想确认可以敲：

```
(Get-Command council).Source
```

### 0.2 搭沙盒（推荐，所有写库、改配置、花钱的项都在这里做）

为什么不能只设环境变量：数据目录按「配置里的 data_dir → 环境变量 COUNCIL_DATA_DIR → E:\council-data」取值，配置里写了就不看环境变量（paths.go:29-37）。你的真配置写了 data_dir（00_DECISIONS.md:2160；16a:2530），所以只设 COUNCIL_DATA_DIR 拦不住，照样写进日常库。最稳的做法：拷一份配置，只改拷贝里的 data_dir，用 `--config` 启动（`--config` 优先于 COUNCIL_CONFIG 与默认位置，paths.go:12-24）。这就是 16a §6 第 5 步（16a:2523-2534）用过的做法。

第 1 步（PowerShell）：建目录。

```
New-Item -ItemType Directory -Force E:\Isolating_test\council\mvp
```

第 2 步（PowerShell）：拷配置。

```
Copy-Item C:\Users\25142\.council\config.toml E:\Isolating_test\council\mvp\config.toml
```

第 3 步（PowerShell）：用记事本改拷贝。

```
notepad E:\Isolating_test\council\mvp\config.toml
```

在记事本里把 data_dir 那一行改成 `data_dir = 'E:\Isolating_test\council\mvp\data'`，保存（UTF-8）。

- 用单引号：TOML 的单引号串不转义反斜杠（16a:2530 的写法）。
- 文件里没有 data_dir 这一行时，把它加在第一个 `[表]` 之前；顶层键必须写在所有表之前（schema.go:69-70）。
- 如果有 `rules_file` 且是相对路径：相对路径按配置文件所在目录解析（config/rules.go:95-98），拷走后找不到，等于没有局部规则、不报错（config/rules.go:95-106）。要保留局部规则，就把它改成原文件的绝对路径。

第 4 步（PowerShell）：启动沙盒。

```
council --config E:\Isolating_test\council\mvp\config.toml
```

预期：横幅（app.go:265-272）依次是 `council 0.2.0-0929a`、`配置文件: E:\Isolating_test\council\mvp\config.toml`、`数据目录: E:\Isolating_test\council\mvp\data`、`库文件: E:\Isolating_test\council\mvp\data\council.db (已有 0 个会话)`、`已加载 N 个 agent，M 个 team`、`输入 /help 查看可用命令，/quit 退出。`，再空一行。「数据目录」不是沙盒路径，立刻 `/quit` 回去改。数据目录不用预先建，打开库时会自动建（store/open.go:54）。

沙盒管得住的：库 `<data_dir>\council.db`、锁 `<data_dir>\council.lock`、成员工作目录 `<data_dir>\workdir`（store/open.go:34、:37；app.go:178）。沙盒与日常库各有各的锁，可以同时开（lock_windows.go:72）。
沙盒管不住的：
- 全局规则照读 `~/.council/COUNCIL.md`，只读不写。配置文件里没有能改它的项；只有环境变量 COUNCIL_GLOBAL_RULES 能换它的位置（config/rules.go:12-14、:33-49），Q5 就用这个办法。
- 各家 CLI 的登录态共用，沙盒里的调用和日常一样真花钱；codex 会在 `~/.codex/sessions` 写会话文件（00_DECISIONS.md:2193），agy 识别会写 `~/.gemini` 日志（00_DECISIONS.md:2123）。
- 在沙盒与日常库之间切换只能退出重开：`/config reload` 碰到 data_dir 变了会整个拒绝（configcmd.go:127-128）。

全部做完、回报之后删沙盒（PowerShell）。下面这条只删 `E:\Isolating_test\council\mvp` 这一个目录；R7 的一次性目录 `E:\Isolating_test\council\mvp\u43` 在它里面，一起删掉：

```
Remove-Item -Recurse -Force E:\Isolating_test\council\mvp
```

### 0.3 用日常库直接测（只适合只读的项）

直接敲 `council`（不带参数）用的是 `C:\Users\25142\.council\config.toml` 和它的 data_dir。适合只读、不写库的项：`/help`、`/conv`、`/use`、`/session`、`/log`、`/cost`、`/rules`、`/providers`、`/agents`、`/verbose`、`/auto`，以及 T1、T2、T4。
会留下的东西：
- 库只追加不回收，`/new`、`/say`、直接打字、`/ask` 都是永久记录，手滑敲错的也是（commands.go:185-188）。
- `/add +编号` 会写真的 config.toml，并在旁边留 `config.toml.bak-add-*` 备份，备份只增不删（16a:2249、:2284、:2515）。
- 改配置或写配置的项（A1、D6、M3、M4、M5、P1、Q2-Q5、R6、K6、K7、K9）不许在真配置上做。D6 要往配置里加一个裁判成员；M4 要先用 M3 的 `/add +编号` 写配置。
- 崩溃探针（K3、K4）会永久留下 outcome 9 的账与 `turn_lost` 面包屑；不重启那个 data_dir，陈旧在飞记录会让那个成员的 `/ask` 一直被拒，直到重启或 `/session rebuild`（13 §3.8；18b §1.1）。R7 的播种同理。
- 改规则文件（P1、P3-P5、Q5）会改身份指纹，已有会话下一轮要重建、各付一次入场费（HANDOFF.md:1033）。

### 0.4 占位与敲法

- `<成员甲>`、`<成员乙>`……连尖括号一起换成成员 id，就是 `/who` 或 `/agents` 的第一列。建议便宜的组合：甲 = `grok-4.7`，乙 = `agy-flash-high`（你的配置里的 id，00_DECISIONS.md:1741-1742；都是 discussion，19a:17）。总结成员是 `fable-5.1-summary`（00_DECISIONS.md:2159）。
- `<会话 id>` 从「已建会话 <会话 id>，标题: …」那一行抄（commands.go:782）。`/use` 收前缀（commands.go:840-846），手敲前 6-8 位比复制稳。
- council 内的命令一行一块、逐块回车，不要整块粘贴（#44，00_DECISIONS.md:2102-2107）；X44 的防护本身也还没复验（00_DECISIONS.md:2114）。
- 标「（PowerShell）」的块在另一个 PowerShell 窗口里敲，不在 council 里敲；没标的都在 council 里敲。同一步里两处都有时，块前面单写一行「（council 里）」或「（PowerShell）」。
- 同一个 data_dir 同时只能开一个 council（lock_windows.go:71-75；A2 就是测这个）。凡是写「启动」「再启动」「重启」的步骤，先在已经开着的沙盒 council 里 `/quit`；不然第二个会被拒，打锁文件那两行。沙盒与日常库、沙盒与 R7 的一次性目录各有各的锁，可以同时开（lock_windows.go:72）。
- 0929a 默认简洁档、默认对话模式（app.go:235-236、:241-242）：
  - 预期里写到用量、金额、冷暖、outcome、provider 会话 id 的，先 `/verbose on`（verbosity.go:13）。
  - 零花费的步骤先 `/auto off`，或用 `-no-auto` 启动；否则直接打的字会问当前会话全部成员，多人时再加一次总结（README.md:65-67）。`/auto` 管的是整个进程，`/new` 之后不会自己打开（autoreply.go:45-47）。
- 行式（`-no-tui`）下一轮进行中按 Ctrl-C 会腰斩这一轮并退出 council（round.go:65-71），和 TUI 不同。
- 专门测粘贴的项（T12、L2、T17）要经剪贴板，**可以做**。

### 0.5 花费标记的读法

- 「零花费」：不起任何推理。不带参数的 `/add` 会起各家 CLI 的识别子命令，零推理，也算零花费（00_DECISIONS.md:2122-2126、:2185）。
- 「花钱：<某家> × N 次」：这一项起 N 次那家 CLI 推理。
  - claude 按美元计。grok 经 CPA、agy 订阅制，文档记为不另花钱（00_DECISIONS.md:324；HANDOFF.md:1024；README.md:33），今天是否还这样没核。codex 是付费调用，Council 不记它的用量与金额（00_DECISIONS.md:2191）。
  - 成员第一次开口、或距上一轮超过 1 小时，是冷启动，另付入场费（turn/session.go:21）。入场费没有可引用的常数（README.md:37-43）；claude 冷启动的量级约 1 美元（00_DECISIONS.md:2139）。
  - 被取消、被杀的一轮，进程已经起了的照样计费。
- 本清单只估调用次数，不估金额。

### 0.6 状态词与出处写法

状态只用这五种：
- 「真机通过（Griffin，<版本号>）」：HANDOFF 或 00_DECISIONS 里写着你在真终端上复验通过；出处给那一处。
- 「主代理隔离验过（零花费）」或「主代理隔离验过（N 次付费）」：主代理自己在隔离目录跑的，不是你亲手试的。N 是数字时照写；写字母 N 表示仓库没记次数，出处那一行会说明。按 §0.5 的口径，起过推理就算付费：grok 经 CPA、agy 订阅制的调用也算，它们是否真的另花钱见 §0.5。
- 「已落地未复验」。
- 「未做」。（OB1 起草时写的是「未做（OB1 进行中）」，2026-09-30 落地后 A3 改为「已落地未复验」。）
- 「暂缓」（/task，00_DECISIONS #24、#29）。

注意：所有「真机通过」都早于 CO1（1c62dbb，默认简洁档）、SM1（自动总结）、CH1（对话模式），屏幕已经变了。这类项的预期按 0929a 的代码与契约重写，项里会写一句「记录早于 CO1」。

出处写法：
- `00_DECISIONS.md` = `docs/00_DECISIONS.md`；`HANDOFF.md` = `.handoff/HANDOFF.md`；`README.md` = `docs/README.md`；`06` = `docs/06_SECURITY_MODEL.md`。
- `11a`、`11c`、`11d`、`12`、`13`、`13a`、`13b`、`15`、`15a`、`15b`、`16a`、`16b`、`16c`、`17a`、`17b`、`18a`、`18b`、`19a`、`19b`、`19c` = `docs/` 下同名前缀的契约；`15` 是 `15_TUI_SPEC_MAPPING.md`。
- `ask.go`、`commands.go`、`round.go`、`cost.go`、`rules.go`、`configcmd.go`、`addmodel.go`、`verbosity.go`、`autoreply.go`、`summary.go` 在 `internal/repl/`；`app.go` 在 `internal/app/`；`model.go`、`run.go` 在 `internal/tui/`；`paths.go`、`schema.go`、`validate.go` 在 `internal/config/`；`main.go` 在 `cmd/council/`；其余带目录写。
- 提交写 7 位 hash。

两处都叫「第 4 刀」：00_DECISIONS #41 的第 4 刀是看板字形（本清单 U8）；15 §8 的第 4 刀是 Judge / 模式选择器与命令面板（本清单 U2-U4）。引用时都点名。

## 1. MVP 的定义（草案，待 Griffin 定）

建议的判法：MVP = 你每天拿它问几个模型、看回复、看花费，且不丢账、不越权、出错有路可退。凡属这条日常路径或它的护栏，进必须项；锦上添花、边角显示、要特殊配置才碰得到的，进以后再说。

### 必须项（42 项）

A2、A3（OB1）、A5、C1、C2、C3、C5、C7、C8、C9、R1、R2、R3、R4、R5、R7、R10、D1、D2、D4、D5、D7、D10、D11、D12、D15、M1、M3、M6（OB3）、M7（AD1）、P2、Q1、Q4、K1、K2、K3、T1、T2、T4、T5、T13、L1。

理由：
- 日常路径：启动（T1、T2、T4）、核对配置里有谁、是什么权限档（A5）、建会话与切换（C1、C2）、加人（C3、C5、M1、M3）、问一个人（R1）、问一群人（D4、D5）、对话模式（D10、D11、D12）、看回复与切档（D1、D2）、启动参数真的生效（D15）、自动总结（D7）、看记录（C7、C8、C9）。
- 续接与恢复：同一 provider 会话接着用、重启后读回（R2、R3、R4），会话坏了能重建（R5），陈旧在飞记录不重启也能解（R7），问错人不花钱（R10）。
- 钱与数据的护栏：同一数据目录只能开一个（A2），默认只读写不了盘（Q1），full 档一律拒（Q4），账看得见（K1），结局不明时不报假金额（K2），崩溃之后丢账看得见（K3，对应 13 §7.3 的诚实声明「崩溃之后接受一次可见的重复投递，绝不接受一次静默的丢失。」），多行命令不误提交（T13），忙时回车不误提交（T5），TUI 出问题有行式退路（L1）。
- OB1：你已定「要做」（00_DECISIONS.md #48），它决定新用户能不能上手，放必须项；已于 3317940 落地，重装之后照 A3 测。
- OB3（M6）：你 2026-09-30 答「可以」，这是已有配置的人平时看得到登录状态的唯一地方，放必须项；已于 ef76eef 落地，已装（0930c 起），照 M6 测。
- AD1（M7）：你 2026-09-30 说「我需要能/add 加入订阅的，不是说只有我们事先选好的那种」，这是日常加人的主路，放必须项；已于 d4ad85f 落地，已装（1001a 起），照 M7 测。

### 以后再说（56 项）

A1、A4、C4、C6、R6、R8、R9、D3、D6、D8、D9、D13、D14、D16、M2、M4、M5、M8（CL1）、P1、P3、P4、P5、Q2、Q3、Q5、K4、K5、K6、K7、K8、K9、T3、T6、T7、T8、T9、T10、T11、T12、T14、T15、T16、T17、L2、L3、L4、L5、L6、U1、U2、U3、U4、U5、U6、U7、U8。

理由：
- 已有真机或隔离验证、今天不急着复验的（C4、R6、R8、T6、T7、T12、L2、L5、A1、P3、Q3、Q5）。
- 要特殊前置才碰得到的：裁判（D6）、codex 成员（M4、M5）、workspace 档（Q2）、改配置造失败轮（K6、K7、K9）、崩溃第二形状与对账失败（K4、K5）、规则文件改动（P1、P4、P5）。
- 边角与观感：UX-1（D14）、`/leave` 总结成员（D16）、带位置参数启动（A4）、@ 补全与跟进（T14-T17）、光标与残影（T9、T10、T15）、行式细节（L3、L4、L6）。
- 未做的界面（U1-U8）：15 §8 的第 3、4、5 刀与 00_DECISIONS #41 的第 4 刀都没排期。

### 暂缓（1 项）

U9（/task，00_DECISIONS.md:1241 #24、:1389 #29；你 2026-09-29 定「暂时不要了」）。

### MVP+（13 §7.3 要求另立一张，待你定）

13 §7.3 要求在 MVP 判据表之外另立一张「MVP+（Step 9 在范围内时）」：一个 team 三个成员各说一轮、Judge 出一次结论、全程成本可核，按 12 的验收指过去（13:1383-1384）。本草案没有为它单写步骤；能拼出它的项是 C6（绑 team）、D6（裁判轮 `--with-judge`）、K1（`/cost` 核账）。要这张表的话，定稿成 docs/14 时照 12 的验收单列一节（见 §4 末尾）。

## 2. 第一次实操的推荐顺序

原则：同一个沙盒进程里用同一对成员连做多项，少付冷启动；零花费全做完再花钱。

### 2.1 零花费一轮（沙盒，TUI）

1. §0.1、§0.2 核版本、搭沙盒、启动。
2. 开场与界面：T1（看的就是 §0.2 第 4 步那一次启动，不要再起一个）→ T2 → C9 → D1 → A5 → A4（在 PowerShell 里敲，不加载配置、不碰锁，沙盒开着也能做）。
3. 没有成员时的对话模式：D11 全部（第 1 步在 PowerShell 敲 `council -h`；第 2 步先 `/new` 一个会话，之后都在这个还没加人的会话里做，零花费；最后一步停在 `/auto off`）。
4. 已是 `/auto off`，再做：C1 → C5 与 M1（两项共用同一次不带参数的 `/add`，见 M1 第 1 步）→ M2（用 `/add --refresh` 再按 Ctrl-C）→ C4（加人，零花费）→ C3 → C2 → C7 → T13 → T14 → T16。
5. D12 第 1-8 步（要 `/auto on`；每一句都设计成谁都不问，照原样敲才零花费，敲前看清该项的警告）→ 做完停在 `/auto off`。
6. D9 → D3 → R10 → R9 第 1-2 步 → R4（这时成员都没开口，看 fresh）→ K1（第 4 步）→ P2 → C8。
7. 这一组每项都要自己起 council，同一个 data_dir 只能开一个（§0.4），所以先做 T4 把沙盒 TUI 关掉：T4（第 3 步 `/quit` 之后沙盒就关了）→ A2（两个窗口；做完两个都 `/quit`）→ L1 → L3（接着 L1 那个行式进程做）→ L4 → L6 → D15 → D12 第 9 步（行式）。每一项启动之前，上一个 council 都已经退出。

要改沙盒配置的零花费项（可放在第一轮末尾，也可以另起一次）：M3 → A1 → Q3 → Q4 → M5（M5 要 codex 成员，先照 M4 前置用 `/add +编号` 加一个）。
R7 用它自己的一次性目录与配置，零花费，步骤里自带配置与播种脚本，放在最后做；它与沙盒各有各的锁，沙盒开着也能做。

### 2.2 花钱一轮（不依赖总结成员，一律加 `--no-summary` 或只问一个人）

建议 <成员甲> = grok-4.7、<成员乙> = agy-flash-high。先用 §0.2 第 4 步那条命令以 TUI 启动沙盒，敲 `/auto off`（本轮只用 `/ask` 与 `/round`，不直接打字）。顺序与调用次数上限：
1. R1：甲 1 次（冷启动）。
2. R2：甲 2 次（暖）。R2 第 1 步开了详细档。
3. R3：甲 1 次。R3 第 2 步重启过（又回到默认的对话模式，先敲 `/auto off`），第 4 步又开了详细档。
4. 仍在 R1 这个会话里，零花费地补两块：`/verbose off`（D2 第 1 步要看简洁档），再 `/add <成员乙>`（R1 只加了甲，D2 要两个人）。
5. D2：甲、乙各 3 次，共 6 次；它的第 1 步同时满足 D4（D4 不再另跑）。D2 第 5 步停在行式。
6. 在行式里 `/quit`，用 §0.2 第 4 步那条命令以 TUI 重启沙盒，`/use` 回这个会话，再 `/auto off`（D5 要在 TUI 里做）。
7. D5：第 1 步甲、乙各 2 次，第 2 步各 1 次，共 6 次。
8. K2：甲 3 次（K2 建新会话，甲是冷启动）。K2 第 6 步开了详细档。
9. `/verbose off`（R8 的预期按简洁档写），然后 R8：乙 1 次（R8 要新会话，乙是冷启动）。
10. 做完回看 K1 第 1-3 步（零花费），看账对不对得上；再做 R5（零花费；/use 回 R1 那个会话，甲已开过口）。

上限约 20 次调用（1+2+1+6+6+3+1），全部落在 grok 与 agy 上；按文档记法不另花钱，但今天没核（§0.5）。

### 2.3 可选（要前置条件，逐项由你定）

- 总结成员（你已配 `fable-5.1-summary`，但它的 model 字符串 `claude-fable-5-1` 能不能用未验证，00_DECISIONS.md:2160）：D7（3 次；第 7 步可选，另 1 次）→ D8（9 次）→ D16（5 次）→ D10（7 次；第 6 步可选，另 1 次）→ D13（3 次，接着 D10 的会话做）。共约 27 次（可选的另 2 次），其中总结 claude 7 次。
  - 冷暖按会话算：provider 会话按「会话 + 成员」分（turn/session.go:23-31），总结成员在每个新会话里的第一次都按首轮（冷）计。D7 的会话一次、D10 的会话一次，共 2 次冷，各约 1 美元量级（00_DECISIONS.md:2139）；D8、D16 接着 D7 的会话，D13 接着 D10 的会话，两轮之间不超过 1 小时是暖。
  - 这是按会话键推的；实际金额还受缓存前缀跨会话共享影响（HANDOFF.md:692），没核。
- 裁判成员（你今天多半没有，19a:1691）：D6。
- codex 成员：M4（codex 3 次，写盘探针另 1 次）。
- 改档位：Q1（每家 1 次）、Q2（claude 或 agy 1 次）。
- grok、agy 专项：K6（grok 2 次）、K7（agy 1 次）、K8（grok 1 次）。
- 崩溃与恢复：K3（约 3 次）、K4（约 2 次）、R6（准备时 1 次）。R5 已排进 §2.2 第 10 步，R7 已排进 §2.1 末尾。
- 规则文件：P1（1 次）、P3（零花费）、P4（1 次）、P5（零花费；重建发生在下一次 `/ask`）、Q5（零花费）。
- 其余：C6（绑 team 零花费，之后打字按全员计费）、D14（2 次）、T3（零花费）、T5-T7（各 1-2 次）、T9-T11、T15、L5（1 次）、K9（要改配置造失败轮，默认不做）。
- 默认不做：K5（真机造不出）、T8（真机难造）。T12、L2、T17 经剪贴板，可以做（§0.4）。

## 3. 逐项清单

### 3.1 启动、配置与数据目录（A）

#### A1 `/config` 看配置；council 开着时改了 config.toml，`/config reload` 先预览差异，`--apply` 才换

- 状态：主代理隔离验过（N 次付费）
- 出处：7a05450 提交说明（11d §4.6 六条真机验收全过，一个 grok 成员，原件 `E:\Isolating_test\council\step8c3-acceptance\RESULTS.md`）；契约 11d §4（:622-793），§4.6.0 要求在进程运行期间改文件（:749-767）；字面量 configcmd.go:52-56、:106、:128、:171、:189-193；`/agents` 表头 commands.go:590。N：提交说明里 4.6.1、4.6.2 后半、4.6.4、4.6.6 都跑了真 `/ask`（grok 经 CPA，文档记为不另花钱，§0.5），总次数没记。
- 前置：沙盒配置；第 2 步必须在 council 开着的时候改文件。
- 花费：零花费（reload 不起进程）。改的若是开过口的成员的 model / effort，`--apply` 之后它下一轮会以身份漂移被拒，要先 `/session rebuild` 再付一次入场费。

步骤：

第 1 步

```
/config
```

第 2 步（PowerShell，council 不关）：打开沙盒配置，在末尾照已有成员抄一段 `[agents."<成员丙>"]`，换一个新 id，保存。id 带点号时表名要加引号（00_DECISIONS.md:1744）。

```
notepad E:\Isolating_test\council\mvp\config.toml
```

第 3 步

```
/config reload
```

第 4 步

```
/config reload --apply
```

第 5 步

```
/agents
```

- 预期：
  - 第 1 步：「配置文件: <路径>」与「agent N，provider M，team K」（configcmd.go:52-56）。
  - 第 3 步：「没有成员会因此漂移」「配置里新增的 agent（1）:」「  <成员丙>」，另两个表头的计数是 0，末行「要应用请敲 /config reload --apply（会换掉 REPL 与 Runner 两处持有的配置）」。四行全是零，说明改文件的时机没赶上，不算通过（11d:765-767）。
  - 第 4 步：同样的报告之后「已应用新配置」（configcmd.go:193）。
  - 第 5 步：表头「ID  PROVIDER  MODEL  EFFORT  PERMISSIONS  MODE」（commands.go:590），列表里有 <成员丙>。
  - 变体（不必做）：改 data_dir 或 rules_file 时整个拒绝，打「data_dir 与 rules_file 只能重启生效，本轮一个字都不换；规则正文改动请用 /rules reload」（configcmd.go:128）；配置写坏时打「配置非法，旧配置原样生效:」并逐条列错（configcmd.go:106）。其余变体见 11d §4.6 第 1-6 条。
- [ ] 符合
- [ ] 不符合

#### A2 同一个数据目录只能开一个 council；第二个立刻报错退出，点名锁文件

- 状态：主代理隔离验过（零花费）
- 出处：00_DECISIONS.md:323、:380-388（E 盘机两组探针，原件 `E:\Isolating_test\council\oq8-lock\RESULTS.md`）；落地 b0f1938；字面量 app.go:137-138、lock_windows.go:71-75；退出码 main.go:110。
- 前置：两个 PowerShell 窗口（下面叫窗口 A、窗口 B）。先把已开着的沙盒 council `/quit` 掉（§0.4），不然第 1 步就被拒。第 1-4 步用日常配置也行（第二个进程连库都不开）；第 5 步要杀进程，用沙盒。
- 花费：零花费。

步骤：

第 1 步（窗口 A）：进到 REPL 后先别退。

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

第 2 步（窗口 B）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

```
$LASTEXITCODE
```

第 3 步（窗口 A）

```
/quit
```

第 4 步（窗口 B）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

第 5 步（可选）：窗口 B 开着、空闲时，在窗口 A 取它的进程号并强杀，然后立刻在窗口 A 起同一条命令。只杀 StartTime 最晚的那个（就是窗口 B 第 4 步起的）；日常库的 council 若也开着、而且比它启动得晚，先把日常那个关掉再做，别杀错。

```
Get-Process council | Sort-Object StartTime | Select-Object Id,StartTime
```

```
taskkill /F /PID <最新那个 Id>
```

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

做完在窗口 A 退出。

```
/quit
```

- 预期：
  - 第 2 步：不打横幅。stderr 两行：「无法打开本地库: data dir is locked by another council process: 锁文件 <data_dir>\council.lock 已被另一个进程持有；同一个 data_dir 上只能同时跑一个 council 实例，请先确认没有其它 council 进程还在使用这个数据目录再重试: <系统错误原文>」与「数据目录是 <data_dir>，可以在配置文件的 data_dir 里改，或设置环境变量 COUNCIL_DATA_DIR。」（app.go:137-138、lock_windows.go:71-75）。`$LASTEXITCODE` 是 1（main.go:110）。
  - 第 4 步：正常横幅（app.go:265-272）。
  - 第 5 步：第一次就开成，不报锁（00_DECISIONS.md:380-383：5 次都是第一次重试就拿到锁）。
  - 库里留痕：无（空闲时杀的，没有在飞记录）。
- [ ] 符合
- [ ] 不符合

#### A3 首次运行自动配置（OB1、OB2）：找本机工具、报登录状态、生成配置

- 状态：已落地未复验（OB1 `3317940`，OB2 `70025db`；已装，0930c 起两刀都含）
- 出处：00_DECISIONS.md #48 与它的两份落地记录；契约 `19e` §6 第 5 步与 `19d` §6 第 5 步（下面的步骤是两者的精简版，细节与全部预期以契约为准）；字面量 19d §3.1-§3.3 与 19e §3。
- 前置：`council -version` 是 `council 0.2.0-0930b` 或更新。**用临时 USERPROFILE 模拟新用户，不碰真配置**。不要用「把真配置改名」来测：生成的 data_dir 就是 `C:\Users\25142\.council\data`，也就是你的日常库。
- 花费：零花费（只跑四家的识别与 `codex login status`，不提问）。
- 先知道会看到什么：模拟改了用户目录，claude 读的是临时目录，所以显示「没登录」（约 4 个模型）。grok 本来也会读临时目录，第 2 步先设 `GROK_HOME` 指回你自己的 `.grok`，它就读你的 CPA 配置，显示「已登录（API key）」；council 本身不读这个目录。codex、agy 仍读你真实的登录态：codex 显示「已登录」（你用的是 ChatGPT 登录，不是 API key），agy 显示「登录状态查不出」。claude、agy 会在临时目录里建自己的 `.claude`、`.gemini`，这是它们的正常行为。

步骤：

第 1 步（PowerShell，新开一个窗口）：建临时目录。

```
$sim = "E:\Isolating_test\council\ob1_realcheck"; New-Item -ItemType Directory -Force "$sim\home","$sim\data" | Out-Null
```

第 2 步（PowerShell，同一个窗口）：只在这个窗口里改五个变量（关掉窗口就还原）。`GROK_HOME` 必须第一个设，因为它要用改之前的 `USERPROFILE`；下面这一行已经按这个顺序写好了。

```
$env:GROK_HOME = "$env:USERPROFILE\.grok"; $env:USERPROFILE = "$sim\home"; $env:HOME = "$sim\home"; $env:COUNCIL_DATA_DIR = "$sim\data"; $env:COUNCIL_CONFIG = ""
```

第 3 步（PowerShell）：第一次运行。

```
council
```

第 4 步（council 里，一行一块）：

```
/new 首次运行复验
```

```
/add
```

```
/quit
```

第 5 步（PowerShell）：看生成的文件。

```
Get-Content "$sim\home\.council\config.toml" -Encoding UTF8
```

第 6 步（PowerShell）：第二次运行不再触发。

```
council -no-tui
```

（council 里）

```
/quit
```

第 7 步（PowerShell）：显式路径找不到时照旧报错、不建文件。

```
council --config "$sim\typo.toml"
```

第 8 步：关掉这个 PowerShell 窗口；在另一个窗口确认真配置没动，再删临时目录。

```
Get-Item C:\Users\25142\.council\config.toml | Select-Object Length, LastWriteTime
```

```
Remove-Item -Recurse -Force E:\Isolating_test\council\ob1_realcheck
```

- 预期：
  - 第 3 步：TUI 画出来**之前**先打：
    - 「第一次运行：还没有配置文件 …\ob1_realcheck\home\.council\config.toml，council 来生成一份。」
    - 「正在检查 claude、grok、agy、codex 能不能用……」
    - 约 6 秒后出四行结果，每行是「识别到 N 个模型；<登录状态>」。模型个数以你那时的 CLI 为准，要看的是每行末尾：
      - claude「没登录」。你的终端里要是设着 `ANTHROPIC_API_KEY`，这一行是「已登录（API key：ANTHROPIC_API_KEY）」；这种情况只按程序文本推过，没实跑。
      - **grok「已登录（API key）」**，起草时是 6 个模型。这一行是 OB2 要验的。
      - agy「登录状态查不出」。
      - codex「已登录」。
    - 接着一段说明「能列出模型不代表已经登录……」，后面**只列 claude 与 agy 两家**的登录办法（`claude auth login`、agy 按提示登录）；grok、codex 不列。
    - 「已生成配置文件: …（只写了找到的工具，还没有成员）」、上手三步，然后才进 TUI。
  - 第 4 步：`/add` 的第一行是「配置里还没有可以 /add 的成员」，接着是识别表；表后**没有**「本机装了 …」的提示（四家都已写进配置）。
  - 第 5 步：
    - 开头是全注释的说明（没有你私人的 FSDT 示例成员）；
    - 接着一行数据目录的注释和 `data_dir = '…\ob1_realcheck\home\.council\data'`；
    - 末尾是四段 `[providers.*]`，`command` 是单引号括起来的绝对路径；
    - 没有 `[agents.*]`、`[summary]`；
    - `$sim\home\.council` 里只有 `config.toml`。
  - 第 6 步：直接是横幅与 `council>`，没有「第一次运行」。
  - 第 7 步：两行「找不到配置文件: …\typo.toml」「这个路径是你用 --config 或环境变量 COUNCIL_CONFIG 指定的，请确认它没写错。」；没有「第一次运行」；`typo.toml` 没被建出来。
  - 第 8 步：大小与修改时间和复验前一样（最后修改应当还是 9 月 28 日）。
  - 变体（不必做）：删掉生成文件里 `[providers.grok]` 那两行再进 council、`/new`、`/add`，识别表后会有「本机装了 grok（…），配置里还没有：……」与要追加的两行（19d §6 第 5 步之 7）。
  - 对照（不必做，19e §6 第 5 步之 4）：新开一个 PowerShell 窗口，照第 1-3 步再做一次，但第 2 步**不设 `GROK_HOME`**（先删掉 `$sim\home\.council`）。这时 grok 那一行应当是「识别到 2 个模型；没登录」，登录办法里多出 `grok login`。这说明正式步骤里看到的「已登录（API key）」确实来自你的 CPA 配置。
- [ ] 符合
- [ ] 不符合

#### A4 带位置参数启动（例如 `council foo`）：报错、打用法，退出码 2，不进 REPL

- 状态：已落地未复验
- 出处：main.go:58-64（退出码常量 :27）；规格 07a:157-160（多余位置参数退 2）；HANDOFF.md:251（Step 4 补上这道检查）。没找到真机记录。
- 前置：无。这道检查在加载配置之前（main.go:58-64 在 app.Run 之前），不开库、不碰锁，沙盒开着也能做。
- 花费：零花费。

步骤：

第 1 步（PowerShell）

```
council foo
```

```
$LASTEXITCODE
```

- 预期：
  - `council foo`：stderr 先一行「council 不接受位置参数，多出来的是: [foo]」（main.go:61），接着是 flag 的用法说明（main.go:62 的 `fs.Usage`；仓库没有自定义它，是 Go flag 包的默认格式，列出 -config、-no-auto、-no-tui、-verbose、-version）；不打横幅，不进 REPL。
  - `$LASTEXITCODE` 是 2（main.go:27、:63）。退出码 1 见 A2，130 见 L4。
- [ ] 符合
- [ ] 不符合

#### A5 `/providers` 列出配置里的 provider 与命令；`/agents` 列出每个 agent 的 provider、model、强度、权限档与模式

- 状态：已落地未复验
- 出处：命令表 commands.go:151-152；`/providers` commands.go:565-577；`/agents` commands.go:579-603（不列 display_name 的理由在 :580-583）；规格 07:188。`/agents` 主代理在隔离目录敲过（00_DECISIONS.md:2160，0.2.0-sm，没发提问；7a05450 的 4.6.3）；`/providers` 没找到任何复验记录，整项按未复验记。
- 前置：无（日常库与沙盒都可以）。
- 花费：零花费。

步骤：

第 1 步

```
/providers
```

第 2 步

```
/agents
```

- 预期：
  - 第 1 步：表头「ID  COMMAND」，每家 provider 一行，第二列是配置里 command 的原文（commands.go:572-575）；配置里一家都没有时打「没有配置任何 provider」（commands.go:568）。
  - 第 2 步：表头「ID  PROVIDER  MODEL  EFFORT  PERMISSIONS  MODE」，每个 agent 一行，按 id 排序，空的格子印 `-`（commands.go:590-600）；没有显示名那一列。总结成员 `fable-5.1-summary` 那一行 PERMISSIONS 必须是 read（validate.go:136-142）；其余成员的档位照你配置里写的核一眼，有 workspace 或 full 的记下来（对话模式下没点名时它们也会被问，见 §4）。
- [ ] 符合
- [ ] 不符合

### 3.2 会话与成员（C）

#### C1 `/new <标题>` 建会话并切过去

- 状态：真机通过（Griffin，0.2.0-x39b）
- 出处：00_DECISIONS.md:1884-1886（TUI 与行式各粘贴一条 `/new` 都直接执行，16 步全部符合）；HANDOFF.md:2250；落地 739348f、一行模式修复 80143b8。
- 前置：无。
- 花费：零花费。

步骤：

第 1 步

```
/new 清单复验
```

第 2 步

```
/new
```

- 预期：
  - 第 1 步：「已建会话 <会话 id>，标题: 清单复验」（commands.go:782）。
  - 第 2 步：「用法: /new <标题>」，不建会话（commands.go:774）。
- [ ] 符合
- [ ] 不符合

#### C2 `/conv` 列出全部会话，`/use <id 或前缀>` 切换

- 状态：已落地未复验
- 出处：08a；739348f；字面量 commands.go:840-846。没找到你的真机记录；19a §6 第 5 步之 8 要用它，还没回报（HANDOFF.md:2704）。
- 前置：至少两个会话（先 `/new` 两次）。
- 花费：零花费。

步骤：

第 1 步

```
/conv
```

第 2 步

```
/use <会话 id 的前 6 位>
```

第 3 步

```
/use zzz
```

- 预期：
  - 第 1 步：表头「CUR  ID  CREATED_AT  TITLE」，当前会话那行 CUR 是 `*`。
  - 第 2 步：「当前会话: <会话 id>，标题: <标题>」。前缀撞到多个时打「id 前缀 X 匹配到 N 个会话，请给出更长的前缀:」，后面跟候选表。
  - 第 3 步：「没有 id 以 zzz 开头的会话，用 /conv 看看有哪些」（commands.go:840-846）。
- [ ] 符合
- [ ] 不符合

#### C3 `/who` 看成员（含已退出的）；`/leave` 让成员退出；`/join` 加回来

- 状态：已落地未复验
- 出处：08a；739348f；/add 别名与前缀 2923bf2；字面量 commands.go:980、:1116、:1137、:1144。HANDOFF.md:691 只记了主代理 2026-09-21 Step 6 冒烟里 `/leave` 之后 `/ask` 触发 post_rejected，没验这几条命令本身的屏幕。
- 前置：当前会话里已有 <成员乙>（`/add <成员乙>` 零花费）；先 `/auto off`。
- 花费：零花费。

步骤：

第 1 步

```
/who
```

第 2 步

```
/leave <成员乙>
```

第 3 步

```
/who
```

第 4 步

```
/join <成员乙>
```

- 预期：
  - 第 1 步：表头「AGENT  JOINED_PROVIDER  ACTIVE  JOINED_AT  LEFT_AT  CONFIG_BRAIN」；没有成员时打「会话 <id> 还没有成员，用 /join <agent-id> 加一个」。
  - 第 2 步：「<成员乙> 已退出会话 <会话 id>」。
  - 第 3 步：成员乙那行 ACTIVE 是 no，LEFT_AT 有时间。
  - 第 4 步：「<成员乙> 已加入会话 <会话 id>」（commands.go:980），之后 `/who` 里回到 yes。
- [ ] 符合
- [ ] 不符合

#### C4 `/add` 按全名、编号或唯一前缀加人；有歧义报错列候选；成员 id 可以带点号

- 状态：主代理隔离验过（零花费）
- 出处：00_DECISIONS.md:1743-1744（隔离数据目录下 `/add opus-5.0` 加入的正是它，`/add op`、`/add fa` 报歧义，`/add gr` 得 `grok-4.7`）；HANDOFF.md:2005-2008；落地 2923bf2（实现出自 Opus，HANDOFF.md:1931）；commands.go:1056-1085。
- 前置：新会话；先 `/auto off`。别敲不带参数的 `/add`（那会触发模型识别，见 M1）。
- 花费：零花费。

步骤：

第 1 步

```
/new 复验 ux2
```

第 2 步

```
/add gr
```

第 3 步

```
/add op
```

第 4 步

```
/add opus-5.0
```

第 5 步

```
/who
```

- 预期：
  - 第 2 步：「grok-4.7 已加入会话 <会话 id>」。
  - 第 3 步：「op 有歧义，可能是：<a>, <b>, …」，谁都没加。
  - 第 4 步：加入的正是 `opus-5.0`，不是 `opus-5.0-max`。
  - 第 5 步：成员表里是这两个全名。
- [ ] 符合
- [ ] 不符合

#### C5 `/add` 的配置候选表不列总结成员；显式 `/add` 总结成员被拒，并说明它会自己加入

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2154（0928b 的真机原件里总结成员出现在候选第 4 行，CH1 4d3dc0b 已修，修后没复验）；19a §3.3、§6 第 5 步之 2（19a:1570-1580）；候选表 commands.go:910-918。
- 前置：配了总结成员；新会话。和 M1 用同一次不带参数的 `/add`：第 2 步最好是这个 council 进程里的第一次 `/add`，M1 要看的「正在识别模型」只在第一次出现（见 M1 前置）。
- 花费：零花费（不带参数的 `/add` 会起各家 CLI 做识别，零推理，约 10 秒，00_DECISIONS.md:2151）。

步骤：

第 1 步

```
/new 候选复验
```

第 2 步

```
/add
```

第 3 步

```
/add fable-5.1-summary
```

- 预期：
  - 第 2 步：候选表（表头「NO  AGENT  PROVIDER  DISPLAY_NAME」）里没有 `fable-5.1-summary`，末行「用 /add <编号|全名|前缀> 加入，多个用空格或逗号隔开，例如 /add 1,2」（commands.go:910-918）；后面照旧是识别表（见 M1）。
  - 第 3 步：一行「fable-5.1-summary 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结」，没有加入（19a:1580）。
  - 已知假话（不算不符合）：讨论成员都加完、只剩总结成员没加入时，`/add` 打「配置里的 agent 都已经在当前会话里了」，`/add <前缀>` 打「X 开头的都已经在当前会话里了」（19a:14）。
- [ ] 符合
- [ ] 不符合

#### C6 `/teams` 看 team；`/team use <team-id>` 把会话绑到 team，并一次加入 team 里还没加入的成员

- 状态：已落地未复验
- 出处：12 §3.4-§3.5；片 2c 36456b4。没找到真机记录。
- 前置：配置里有 `[teams]`；新会话；先 `/auto off`。
- 花费：绑定本身零花费。绑完之后在这个会话里直接打字，会问 team 全部成员再加一次总结；team 有 7-8 人时就是 7-8 次调用加总结（19a:1702）。

步骤：

第 1 步

```
/teams
```

第 2 步

```
/new team 复验
```

第 3 步

```
/team use <team-id>
```

第 4 步

```
/who
```

- 预期：
  - 第 1 步：表头「ID  MEMBERS  MEMBER_IDS  CURRENT」，没绑时 CURRENT 是 `-`。
  - 第 3 步：「已绑定 team <team-id>」，然后每个新加的成员一行「<id> 已加入会话 <会话 id>」；已经 `/leave` 过的打「<id> 已退出，未自动拉回，要加回用 /join」；team 不存在时打「配置里没有 team <id>，用 /teams 看看有哪些」。
  - 第 4 步：team 的成员都在，ACTIVE 是 yes。
  - 已知：`/team use` 不看 `[summary]`，team 里有总结成员时会把它加进来（19a:1706；00_DECISIONS.md:2257）；你的总结成员不在 teams.all 里（00_DECISIONS.md:2159）。
- [ ] 符合
- [ ] 不符合

#### C7 `/say <文本>` 只记下、不让任何人回复；`/say /foo` 发字面量 `/foo`

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2196、:2201（CH1 4d3dc0b 把 `/say` 定成「只记下」）；19a §3.4、§6 第 5 步之 7；简洁档不回确认 16b:1147、00_DECISIONS.md:2180；TUI 下单独 `/say` 只打一句提示：seam.go:11、:82-84，17a:577 (g)。
- 前置：当前会话里有成员（用来确认它们没被问）；在 TUI 里做（第 4 步只在 TUI 下成立；行式下单独 `/say` 进多行输入，见 L3）。
- 花费：零花费。

步骤：

第 1 步

```
/say 这句只记下
```

第 2 步

```
/say /foo
```

第 3 步

```
/log 3
```

第 4 步：只敲 `/say`，不带任何字，回车。

```
/say
```

- 预期：
  - 第 1、2 步：简洁档什么都不打，也不起进程（16b:1147）；详细档打「已记下 seq N」。
  - 第 3 步：最后两行 SENDER 是 user，TEXT 分别是「这句只记下」和「/foo」。
  - 第 4 步：一行「TUI 下直接在输入框里换行即可，不需要多行模式」（seam.go:11），不进多行模式、不记消息；多行消息在 TUI 里用 Ctrl-J 换行（T3）。
- [ ] 符合
- [ ] 不符合

#### C8 `/log [n]` 按 seq 看最近的消息；系统面包屑末尾带 ` [event=…]`

- 状态：真机通过（Griffin，0.2.0-s10）
- 出处：00_DECISIONS.md:2098（事件标记 ` [event=round_started]`、` [event=session_opening]`、` [event=round_finished]` 通过，发言行不带多余标记）；HANDOFF.md:2603；另 0.2.0-x39b 下 `/log` 正常（00_DECISIONS.md:1887）。落地 739348f、标记 a2e3d76；字面量 commands.go:1222。记录早于 CO1，但 `/log` 不受档位影响（16b:525）。
- 前置：会话里有消息，最好跑过一次 `/round` 或有成员的首轮（零花费轮里可以只看 `/say` 留下的行）。
- 花费：零花费。

步骤：

第 1 步

```
/log 7
```

第 2 步

```
/log 0
```

- 预期：
  - 第 1 步：表头「SEQ  SENDER  KIND  TO  BRAIN  TEXT」。system 面包屑的 TEXT 末尾带「 [event=session_opening]」「 [event=round_started]」「 [event=round_finished]」这类标记，标记前恰好一个空格；成员发言行不带标记。空会话打「会话 <id> 还没有消息，用 /say <文本> 说点什么」。
  - 第 2 步：「条数要是正整数，用法: /log [n]」（commands.go:1222）。
- [ ] 符合
- [ ] 不符合

#### C9 `/help` 列出全部命令，含 `/auto`、`/verbose` 与 `/round` 的新写法

- 状态：已落地未复验
- 出处：19a §3.4、§6 第 5 步之 2（19a:1586）；16b §3.4；16c §3.3；18a L4（18a:1017）；落地 4d3dc0b、1c62dbb、a877d44、db8c41a。
- 前置：无。
- 花费：零花费。

步骤：

第 1 步

```
/help
```

- 预期：表头「COMMAND  SUMMARY」，按命令名排序。
  - `/auto` 行：「切换直接打字要不要让成员回复: /auto 切换；/auto on 自动回复（点名了谁就只问谁，没点名问全部成员）；/auto off 只记下」。
  - `/say` 行开头：「只记下、不让任何人回复」。这一行后半句「/say 不带参数进多行输入（/end 发送时与直接打字相同）」（commands.go:126）只在行式下成立；TUI 下单独敲 `/say` 只打一句提示（C7 第 4 步），看到这半句不算不符合。
  - 有 `/providers` 与 `/agents` 两行（commands.go:151-152，见 A5）。
  - `/session` 行有「先结清这个成员留在库里的陈旧在飞记录」。
  - `/round` 行末尾：「/round [id...] [--go] [--rounds N] [--with-judge] [--no-summary]」（19a:1217）。
  - `/verbose` 行：「切换输出档位: /verbose 在简洁与详细之间切换；/verbose on 详细；/verbose off 简洁」。
  - `/add` 行含「/add +编号 [--id <新 id>] [--effort <强度>] 把识别出的模型写进配置并加入；/add --refresh 重新识别」（18a:1017）。
- [ ] 符合
- [ ] 不符合

### 3.3 提问、续接与 `/session`（R）

#### R1 `/ask` 点一个成员说一轮，屏幕出「显示名（用时）: 回复」

- 状态：真机通过（Griffin，0.2.0-rba）
- 出处：00_DECISIONS.md:1972（RB-A 契约 §6 六步都正常，第 1 步就是 `/ask`，15a:617）；HANDOFF.md:2348（rba 二进制）；Step 6 落地（README.md:55）；字面量 commands.go:782、:980，ask.go:164、:381；用时格式 verbosity.go:80-88。记录早于 CO1：简洁档下的 `/ask` 没有正式复验，只有你「真机试完简洁输出之后」那句话（00_DECISIONS.md:2198）。
- 前置：沙盒；新会话。
- 花费：花钱：<成员甲> 那家 × 1 次（首轮冷启动）。

步骤：

第 1 步

```
/new 清单-ask
```

第 2 步

```
/add <成员甲>
```

第 3 步

```
/ask <成员甲> 只回一个词：PONG
```

- 预期：
  - 第 1 步：「已建会话 <会话 id>，标题: 清单-ask」（commands.go:782）。
  - 第 2 步：「<成员甲> 已加入会话 <会话 id>」（commands.go:980）。
  - 第 3 步：先「正在问 <成员甲> ...」（ask.go:164），回来后「<显示名>（<n.n>s）: PONG」（ask.go:381；verbosity.go:80-88）。TUI 表头的计时见 T5。
- [ ] 符合
- [ ] 不符合

#### R2 同一成员第二轮接着上一轮的 provider 会话：记得上一轮，会话 id 不变，预报暖

- 状态：主代理隔离验过（2 次付费）
- 出处：HANDOFF.md:688-689（Step 6，2026-09-21：claude 连跑两轮，金额 0.1031 → 0.0119 美元，付费 2 次；grok 也连跑两轮，经 CPA）；codex 三轮同一个 thread 见 M4（00_DECISIONS.md:2189）。D-6（00_DECISIONS.md:62-68）；缓存 1 小时 turn/session.go:21；字面量 ask.go:413、:423、:427、:540，verbosity.go:13。你没有专门复验的记录。
- 前置：接着 R1 的会话与 <成员甲>；两轮之间不超过 1 小时，超过就预报冷。
- 花费：花钱：<成员甲> 那家 × 2 次。

步骤：

第 1 步

```
/verbose on
```

第 2 步

```
/ask <成员甲> 记住单词 KIWI，只回 OK
```

第 3 步

```
/ask <成员甲> 上一轮让你记住的单词是什么？只回那个词
```

第 4 步

```
/session
```

- 预期：
  - 第 1 步：「已切到详细输出：用量、金额、冷暖、会话 id、预算表与结算全部显示；/verbose off 回到简洁」（verbosity.go:13）。
  - 第 2 步：「outcome spoke  agent <成员甲>」；「provider 会话 <id>」（R1 已开过口，这里不是「新开了一条」，ask.go:423-427）；「冷暖 预报=暖 …」（ask.go:413、:540）。距 R1 超过 1 小时时是「预报=冷」，那是正常的，不算不符合。
  - 第 3 步：正文有 KIWI；「provider 会话 <与第 2 步同一个 id>」；「冷暖 预报=暖 实测=暖」。例外：agy 跨轮没有缓存（HANDOFF.md:816-818）；codex 实测一栏恒为「未知」（17b:957）。
  - 第 4 步：<成员甲> 那一行 PROVIDER_SESSION 是同一个 id，NEXT 为 warm。
- [ ] 符合
- [ ] 不符合

#### R3 退出再进，同一会话的成员接着用库里那条 provider 会话；`/session` 不用先问就能读回

- 状态：主代理隔离验过（N 次付费）
- 出处：HANDOFF.md:745-746（Step 7：第二个进程 `/ask` 用回库里那条会话，游标与冷暖都读回；全新进程直接 `/session` 显示正确）；README.md:56；原件 `E:\Isolating_test\council\step7-smoke\`。N：仓库没记次数，也没写重启那一组用的哪家（同目录另一组用的是 grok 成员 gk、gk2，HANDOFF.md:771-777）。10 / 10a §3.2、§3.4。
- 前置：做完 R2；记下会话 id（`/conv` 可查）。第 2 步重启后回到默认的对话模式（app.go:236）；本项只用 `/ask`，不直接打字，要保险就在重启后先敲 `/auto off`。
- 花费：花钱：<成员甲> 那家 × 1 次（第 4 步）；前 3 步零花费。

步骤：

第 1 步

```
/quit
```

第 2 步（PowerShell）：用同一条命令重新启动。

```
council --config E:\Isolating_test\council\mvp\config.toml
```

第 3 步

```
/use <会话 id 的前 6 位>
```

```
/session
```

第 4 步

```
/verbose on
```

```
/ask <成员甲> 刚才让你记住的单词是什么
```

- 预期：
  - 第 3 步：<成员甲> 那一行的 PROVIDER_SESSION 与退出前相同，NEXT 为 warm（1 小时内）。
  - 第 4 步：「provider 会话 <同一个 id>」，正文答出 KIWI。
- [ ] 符合
- [ ] 不符合

#### R4 `/session` 列出每个成员的 provider 会话、上轮时刻、成本基线与下一轮冷暖

- 状态：真机通过（Griffin，0.2.0-s10）
- 出处：00_DECISIONS.md:2098（BASELINE 列：四个成员都是 ok、列对齐）；复验步骤 HANDOFF.md:2582，结果 HANDOFF.md:2603；落地 8c00ec9；表头与取值 ask.go:199、:205-243。那次没看到 `-` 与 fresh 那一格。`/session` 不受档位影响（16b:525）。
- 前置：当前会话里至少一个成员。零花费轮里成员都没开口，看 fresh；花钱轮之后再看一次 ok。
- 花费：零花费。

步骤：

第 1 步

```
/session
```

- 预期：
  - 表头「AGENT  PROVIDER_SESSION  LAST_TURN_AT  BASELINE  NEXT」（ask.go:205）。
  - 开过口的成员 BASELINE 是 ok（上一轮结局不明时是 suspect，见 K2），NEXT 是 warm 或 cold（距上一轮超过 1 小时为 cold），这一轮正在跑时是 busy。
  - 没开过口的成员形如「<id>  -  -  -  fresh」（ask.go:209-232）。
  - 会话里没有成员时打「会话 <id> 没有活跃成员，用 /join <agent-id> 加一个」（ask.go:199）。
- [ ] 符合
- [ ] 不符合

#### R5 `/session rebuild <成员>` 丢掉这个成员的 provider 会话，下一轮按首轮重开

- 状态：主代理隔离验过（N 次付费）
- 出处：HANDOFF.md:1024-1027（Step 8 第一片第三轮，grok 经 CPA：丢弃 → 新会话 id → 回答里出现重建前刚加进规则文件的标记 → `/rules` 漂移计数 3 变 2）；11a §5.3（:521-536）；c88ea73；字面量 ask.go:293、:336、:341。N：闭环要 grok 真的回答（标记出现在回答里），起过推理；HANDOFF.md:1024 写的「零成本」是按 grok 经 CPA 不另花钱算的（§0.5），次数没记。
- 前置：<成员甲> 至少开过一次口（花钱轮之后做）。
- 花费：零花费。rebuild 之后下一次 `/ask` 是冷启动，要付入场费。

步骤：

第 1 步

```
/session rebuild <成员甲>
```

第 2 步

```
/session rebuild <成员甲>
```

第 3 步

```
/session
```

第 4 步

```
/session rebuild nobody
```

- 预期：
  - 第 1 步：「已丢弃 <成员甲> 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢），下一轮按首轮计价」（ask.go:341）。
  - 第 2 步：「本来就没有开着的会话，下一轮照常是首轮」（ask.go:336）。
  - 第 3 步：<成员甲> 那一行 NEXT 为 fresh。
  - 第 4 步：「nobody 不在当前会话的活跃成员里，用 /who 看看有哪些」，什么都不删（ask.go:293）。
- [ ] 符合
- [ ] 不符合

#### R6 配置里删掉的 agent 在库里留下悬空会话：启动时说「会被拒、不花钱」，`/session rebuild` 能清掉

- 状态：真机通过（Griffin，0.2.0-x39f）
- 出处：HANDOFF.md:2329-2334（启动时 ag、op 只出现在「按当前配置 /ask 会被拒，不花钱」那一句与明细里，整段没有「入场费」；`/rules` 打「… 2 条（不花钱）」；`/session rebuild a` 按前缀被拒；op、ag 都打「已清掉」；重启后漂移提示消失）；00_DECISIONS.md #37(g)、#39(f)；X-39f 7d65b96；字面量 app.go:292、rules.go:100、ask.go:339。
- 前置：沙盒（要从配置里删成员，不许在真配置上做）。<成员戊> 用一个后面不再用的一次性 grok 成员：照 A1 第 2 步在沙盒配置里抄一段 grok-4.7、换个新 id（例如 `grok-tmp`），`/config reload --apply` 或重启让它生效。不要用 grok-4.7 这类后面 K6、K8、K9、Q3、Q5 还要用的 id：第 2 步会把它从沙盒配置里删掉。
- 花费：准备时 <成员戊> 那家 × 1 次；其余零花费。

步骤：

第 1 步（准备）：沙盒里新建会话，`/add <成员戊>`，问它一次。

```
/ask <成员戊> 你好
```

```
/quit
```

第 2 步（PowerShell）：用记事本从沙盒配置里删掉 `[agents."<成员戊>"]` 那一段（它若在某个 team 里，也从 team 名单里删掉），保存，再启动。

```
notepad E:\Isolating_test\council\mvp\config.toml
```

```
council --config E:\Isolating_test\council\mvp\config.toml
```

第 3 步

```
/use <会话 id 的前 6 位>
```

```
/rules
```

第 4 步

```
/session rebuild <成员戊>
```

第 5 步：`/quit` 后用同一条命令再启动一次。

```
/quit
```

- 预期：
  - 第 2 步启动时：「注意: 有 1 条 provider 会话按当前配置 /ask 会被拒，不花钱；其中原因是「配置里已无此 agent」的，先 /use 那个会话再敲 /session rebuild <成员> 即可清掉」，整段不出现「入场费」（app.go:292）。
  - 第 3 步：「按当前配置 /ask 会被拒的 provider 会话: 1 条（不花钱）」（rules.go:100）。
  - 第 4 步：「已清掉 <成员戊> 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢）；按当前配置对它 /ask 会被拒，不花钱」（ask.go:339）。
  - 第 5 步再启动：不再有漂移提示。
- [ ] 符合
- [ ] 不符合

#### R7 `/session rebuild` 先结清成员留在库里的陈旧在飞记录，被拒之后不用重启就能解开（U43A）

- 状态：主代理隔离验过（零花费）
- 出处：00_DECISIONS.md:2089（2026-09-28 17:15，0.2.0-0928b 行式，照 18b §6 第 6 步，三行冻结文本逐字打出，`/log 3` 有 `[event=turn_lost]`；TUI 下没跑）；18b §1.1、§3.1、§6 第 6 步（18b:630-685）；落地 17b3236；字面量 ask.go:312-336、:654。播种脚本照 18b:633-648 逐字抄；配置的必填项见 config/validate.go:56-58、:69-80、:145-147。
- 前置：用它自己的一次性目录 `E:\Isolating_test\council\mvp\u43`，data_dir 与沙盒分开，各有各的锁（§0.4），沙盒开着也能做；§0.2 末尾删沙盒时一起删掉。配置只含一个 claude 成员，provider 的 command 指向一个不存在的程序：万一没被拒也起不出真 claude（主代理 09-28 就是这样做的，00_DECISIONS.md:2089）。python 用 claude_test 环境：E 盘机 `D:\Install_path\Anaconda3\envs\claude_test\python.exe`（18b:631；D 盘机的路径没核）。第二个 PowerShell 窗口。
- 花费：零花费。第 8 步之后不要再 `/ask`，那一次会真的起进程（18b:685）。

步骤：

第 1 步（PowerShell）：建目录。

```
New-Item -ItemType Directory -Force E:\Isolating_test\council\mvp\u43
```

第 2 步（PowerShell）：用记事本新建一次性配置。记事本问要不要新建文件时点「是」，把下面第二块原样写进去，保存。全文是 ASCII，编码无所谓。

```
notepad E:\Isolating_test\council\mvp\u43\config.toml
```

```
data_dir = 'E:\Isolating_test\council\mvp\u43\data'

[providers.claude]
command = 'E:\Isolating_test\council\mvp\u43\no-such-claude.exe'

[agents.alice]
display_name = 'Alice'
provider = 'claude'
model = 'opus'
effort = 'high'
permissions = 'read'
mode = 'discussion'
```

模板没写 role（配置校验不查它，config/validate.go:61-80）：alice 只用来被拒，起不了进程。

第 3 步（PowerShell）：同样新建播种脚本，把下面第二块原样写进去（18b:633-648 逐字，全文 ASCII），保存。

```
notepad E:\Isolating_test\council\mvp\u43\u43_seed.py
```

```
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

第 4 步（PowerShell）

```
council --config E:\Isolating_test\council\mvp\u43\config.toml
```

第 5 步：记下「已建会话 <会话 id>，标题: u43真机」里的完整会话 id（第 6 步要整串，不是前缀）。

```
/new u43真机
```

```
/join alice
```

第 6 步（另一个 PowerShell，council 不关）：恰好打出一行 `u43-probe spawned read` 才往下做；打出别的、什么都没打、或者报错，就停，先查三个参数（18b:652）。会话 id 抄错时脚本什么都不打，不会插错行（18b:653）。

```
& D:\Install_path\Anaconda3\envs\claude_test\python.exe E:\Isolating_test\council\mvp\u43\u43_seed.py E:\Isolating_test\council\mvp\u43\data\council.db <完整会话 id> alice
```

第 7 步

```
/ask alice
```

第 8 步

```
/session rebuild alice
```

第 9 步

```
/log 3
```

第 10 步

```
/quit
```

- 预期：
  - 第 4 步：横幅里「数据目录: E:\Isolating_test\council\mvp\u43\data」与「已加载 1 个 agent，0 个 team」（app.go:268、:270）。配置写错时报错、不进 REPL（退出码 1，main.go:26、:110），照报的错改。
  - 第 5 步：「已建会话 <会话 id>，标题: u43真机」「alice 已加入会话 <会话 id>」。
  - 第 7 步：先「正在问 alice ...」，再「Alice: 没有回复（拒绝）」，接着「这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行」，不起进程（18b:672；ask.go:654）。
  - 第 8 步三行：「已结清 alice 的一轮在飞记录（turn u43-probe，phase spawned），对它 /ask 不会再因为这条记录被拒」「那一轮记为丢失：可能花了钱，用量与金额都不知道（与重启时启动对账的记法相同）」「本来就没有开着的会话，下一轮照常是首轮」（18b:678）。
  - 第 9 步：最后一条是 system 消息，TEXT「成员 alice 的一轮在上次退出时没记上账：起过进程，结局未知；这条 provider 会话点不出名字 [event=turn_lost]」（18b:684）。
  - 库里留痕（一次性库）：两行 turn_usage（refused 与 outcome 9 丢失）和一条 turn_lost 面包屑。
- [ ] 符合
- [ ] 不符合

#### R8 `/to <id>[,<id>] 文本`：只让点名的成员看到这条，其余成员永远看不到

- 状态：主代理隔离验过（N 次付费）
- 出处：b073fde 提交说明（11d §2.8 真机第 9-11 条通过：两个 grok 成员 gk、gk2 经 CPA；`/ask gk` 得 ErrNoDelta、没起进程；`/leave` 之后 `/to` 被拒、没落库；`/log` 的 TO 列显示 gk2；原件 `E:\Isolating_test\council\step8c1-acceptance\RESULTS.md`）；11d §2.5-2.8（:291-402）；字面量 commands.go:395、:509，ask.go:656。N：提交说明里看得到的是 `/ask gk2` 起过一次进程（prompt_bytes=25），至少 1 次；grok 经 CPA，文档记为不另花钱（§0.5）；总次数没记。
- 前置：新会话；<成员甲>、<成员乙> 都已加入、都没开过口。
- 花费：花钱：<成员乙> 那家 × 1 次（第 3 步，首轮冷启动）；其余零花费。

步骤：

第 1 步

```
/to <成员乙> 只给你看：暗号 KIWI
```

第 2 步

```
/ask <成员甲>
```

第 3 步

```
/ask <成员乙> 刚才的暗号是什么
```

第 4 步

```
/leave <成员乙>
```

第 5 步

```
/to <成员乙> 还在吗
```

- 预期：
  - 第 1 步：「只有 <成员乙> 会看到这条；其余成员永远不会 —— 游标只前进」（commands.go:395）；对话模式下也不会自动问任何人（00_DECISIONS.md:2201）。
  - 第 2 步：「正在问 <成员甲> ...」「<显示名>: 没有回复（拒绝）」「这个 agent 没有新内容可读」，不起进程（ask.go:656）。
  - 第 3 步：成员乙答出 KIWI；之后 `/log` 里那条 `/to` 的 TO 列是 <成员乙>（11d §2.7）。
  - 第 4 步：「<成员乙> 已退出会话 <会话 id>」。
  - 第 5 步：「这条消息不会有任何在场成员看到」，不落库（commands.go:509）。
- [ ] 符合
- [ ] 不符合

#### R9 `/ask` 可以用唯一前缀点人；前缀有歧义时报错列候选，不起进程、不记消息

- 状态：已落地未复验
- 出处：00_DECISIONS.md:1747（「没有发 /ask」）；HANDOFF.md:1947；落地 2923bf2；ask.go:69-110。
- 前置：前缀对应的成员都已加入当前会话（例如 `opus-5.0` 与 `opus-5.0-max`，前缀 `op` 有歧义）。
- 花费：第 1-2 步零花费；第 3 步花钱：对应那家 × 1 次。

步骤：

第 1 步

```
/ask op 你好
```

第 2 步

```
/log 3
```

第 3 步（花钱，可选）

```
/ask gr 你好
```

- 预期：
  - 第 1 步：「op 有歧义，可能是：…」，不打「正在问」。
  - 第 2 步：没有「你好」这条消息（前缀解析在记消息之前）。
  - 第 3 步：「正在问 grok-4.7 ...」，随后是回复。
- [ ] 符合
- [ ] 不符合

#### R10 `/ask` 一个已退出或还没加入的成员：起进程之前就拒，不花钱、不记消息

- 状态：已落地未复验
- 出处：eb1e464（X-1）；ask.go:112-128（:119、:125）。相关真机观察只有「配置里已删、仍是成员」那一档：先打「正在问」再被拒，没花钱（HANDOFF.md:1945；00_DECISIONS.md:1715-1717，#37(a) 未修）。
- 前置：当前会话里有 <成员甲>；配置里另有一个没加入的 agent（下面写 <成员丙>）。
- 花费：零花费。

步骤：

第 1 步

```
/leave <成员甲>
```

第 2 步

```
/ask <成员甲> 你好
```

第 3 步

```
/ask <成员丙> 你好
```

第 4 步

```
/join <成员甲>
```

- 预期：
  - 第 2 步：「<成员甲> 已退出当前会话，用 /join <成员甲> 重新加入」，没有「正在问」（ask.go:119）。
  - 第 3 步：「<成员丙> 在配置里但还没加入当前会话，用 /join <成员丙> 加入」（ask.go:125）。
  - 第 4 步：「<成员甲> 已加入会话 <会话 id>」。
- [ ] 符合
- [ ] 不符合

### 3.4 输出档位、`/round` 与对话模式（D）

#### D1 `/verbose` 在简洁与详细之间切换；启动参数 `-verbose`

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2116、:2150（CO1 真机未复验）；16b §3.3-§3.5、§6 第 5 步；1c62dbb；main.go:43；字面量 verbosity.go:13-14。
- 前置：无。
- 花费：零花费。

步骤：

第 1 步（PowerShell）

```
council -h
```

第 2 步

```
/verbose
```

第 3 步

```
/verbose off
```

第 4 步

```
/verbose x
```

- 预期：
  - 第 1 步：有 `-verbose`，说明是「详细输出：每轮的用量、金额、冷暖、provider 会话、预算表与结算全部显示（默认简洁；进去之后也可以用 /verbose 切换）」（main.go:43）。
  - 第 2 步：「已切到详细输出：用量、金额、冷暖、会话 id、预算表与结算全部显示；/verbose off 回到简洁」（verbosity.go:13）。
  - 第 3 步：「已切到简洁输出：只显示成员的回复、用时，以及关系到钱或内容丢失的提示；/verbose on 切到详细」（verbosity.go:14）。
  - 第 4 步：「用法: /verbose 在简洁与详细之间切换；/verbose on 切到详细；/verbose off 切到简洁」（verbosity.go:12）。
  - 第 1 步只看得到说明文字；`-verbose` 是否真的传进了程序，见 D15 第 1、2 步。
- [ ] 符合
- [ ] 不符合

#### D2 默认简洁输出：每人一行「显示名（用时）: 回复」，不显示金额与冷暖；TUI 最后是收尾行，行式最后是「总用时」

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2116、:2150；你「真机试完简洁输出之后」说 OK（00_DECISIONS.md:2198；19a:181），但没有照 16b §6 逐步复验的记录，不算通过。16b §3.1-§3.2、§6 第 5 步（16b:1047 起，:1066、:1072）；1c62dbb。16b §6 写于 SM1 之前，照原样敲会多出总结，所以这里一律加 `--no-summary`。
- 前置：会话里有 <成员甲>、<成员乙>；当前是简洁档（前面开过详细档的话先 `/verbose off`）。行式那一步不要按 Ctrl-C（round.go:65-71）。
- 花费：花钱：甲、乙各 3 次（三次 `/round`）。第 1 步同时满足 D4。

步骤：

第 1 步

```
/round <成员甲> <成员乙> --go --no-summary
```

第 2 步

```
/verbose
```

第 3 步

```
/round <成员甲> <成员乙> --go --no-summary
```

第 4 步

```
/verbose off
```

第 5 步：`/quit`，用行式重进同一会话再跑一次。

```
/quit
```

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

```
/use <会话 id 的前 6 位>
```

```
/round <成员甲> <成员乙> --go --no-summary
```

- 预期：
  - 第 1 步：「本轮问 2 个: <甲>, <乙>」，然后每人一行「<显示名>（<N.Ns>）: 正文」，谁先回来先打谁；没有预算表、outcome、seq、用量、金额、冷暖、结算块；TUI 最后一行「/round 结束  Ns  回来 2/2」，没有「总用时」（16b:1066）。
  - 第 2 步：「已切到详细输出：…」（16b:1072）。
  - 第 3 步：预算表、outcome 行、用量、金额、结算、段终态块都回来了。
  - 第 4 步：「已切到简洁输出：…」。
  - 第 5 步：行式最后恰好一行「总用时 N.Ns」（16b §6 第 5 步之 4）。
- [ ] 符合
- [ ] 不符合

#### D3 `/round <id...>` 不带 `--go`：只出预算预报，不起进程；名单不对整条拒绝

- 状态：已落地未复验
- 出处：12 §4.5、§4.6a.2-§4.6a.3；片 3c f589daf、片 3d 0a084ca；round.go:586-594；12:826-829。没找到不带 `--go` 的真机记录（15a、15b §6 都带 `--go`）。
- 前置：会话里有两个成员；先 `/auto off`。
- 花费：零花费。

步骤：

第 1 步

```
/round <成员甲> <成员乙>
```

第 2 步

```
/round <成员甲> <成员乙> --rounds 2
```

第 3 步

```
/round nobody --go
```

第 4 步

```
/round
```

- 预期：
  - 第 1 步：表头「成员  输入token  金额  冷启动  在飞  命令行余量  依据」，每人一行。没开过口的成员 token 与金额两格是「未标定」；表尾「合计输入token 0（只含已标定的行）」和「另有 2 名未标定」（12:826-829；round.go:586-594）。不起进程。
  - 第 2 步：「拒绝: --rounds 2 只在 --go 下可用。」
  - 第 3 步：「拒绝: 不是当前活跃成员: nobody」。
  - 第 4 步：最后一条用户消息里没有点名时：「拒绝: 名单为空。给了 id 才用给的；没给则取当前会话最后一条用户发言里的 mention 与当前活跃成员的交集。不会默认全员。」；有点名时只预报被点的人（见 D9）。
- [ ] 符合
- [ ] 不符合

#### D4 `/round <id...> --go` 让名单上的成员同时回复；TUI 输入框上方每人一行，在跑时计时，结束打一行收尾，不留残影

- 状态：真机通过（Griffin，0.2.0-s10）
- 出处：00_DECISIONS.md:2011（RB-B §6 第 1、3 项：逐行在跑与计时、跑的时候一直打字，收起后不留残影）；15b:651、:653；RB-A 见 00_DECISIONS.md:1972；落地 572467c、69fed6c。记录早于 CO1：默认简洁档现在不打段终态块（16b:84；00_DECISIONS.md:2145），这一半未复验。
- 前置：会话里有两个讨论成员；在 TUI 里做。跑 D2 第 1 步时顺带看即可。
- 花费：花钱：甲、乙各 1 次（与 D2 第 1 步合并时不另计）。

步骤：

第 1 步

```
/round <成员甲> <成员乙> --go --no-summary
```

第 2 步：跑的时候在输入框里打几个字、按 Ctrl-J 换行，别回车，等两人都回来。

第 3 步（可选，再花一次）

```
/verbose on
```

```
/round <成员甲> <成员乙> --go --no-summary
```

- 预期：
  - 第 1 步：「本轮问 2 个: <甲>, <乙>」；输入框上方两行「  在跑    <id>  Ns」，秒数每秒走；回来的人那行变「  完成    <id>  x.xs 子进程」，回滚区打「<显示名>（<N.Ns>）: 正文」。
  - 第 2 步：打字不打乱看板；都回来后回滚区多一行「/round 结束  Ns  回来 2/2」，活区缩回表头、横线、输入框，提示符上方不留旧表头、横线或成员行；简洁档不打段标题与成员终态块。
  - 第 3 步：收尾行之前多出段标题「发言轮 (xxxxxx)  回来 2/2」和两行终态行。
- [ ] 符合
- [ ] 不符合

#### D5 `/round … --go --rounds N` 连问 N 轮；中途 Ctrl-C 停在两轮之间，回到空闲，程序不退

- 状态：真机通过（Griffin，0.2.0-s10）
- 出处：00_DECISIONS.md:2011（RB-B 第 2、4 项：两段 `--rounds 2`；中途 Ctrl-C 屏幕是「已跑 1/2 轮，其余取消」；主代理给的 0/2 是写错了）；RB-A 第 4 步见 00_DECISIONS.md:1972、15a:620；12 §4.6a.6；15b:652、:654；model.go:396-409。记录早于 CO1 与 SM1。
- 前置：会话里有两个成员；在 TUI 里做（行式下 Ctrl-C 会退出 council）。
- 花费：花钱：第 1 步甲、乙各 2 次；第 2 步甲、乙各 1 次（被腰斩的那一轮照样计费）。

步骤：

第 1 步

```
/round <成员甲> <成员乙> --go --rounds 2 --no-summary
```

第 2 步：敲下面这条，第 1 轮还没回来时按一次 Ctrl-C，等它回到空闲。

```
/round <成员甲> <成员乙> --go --rounds 2 --no-summary
```

- 预期：
  - 第 1 步：表头依次出现「发言轮 1/2」「发言轮 2/2」，每段每人一行；收尾行「/round 结束  Ns  回来 4/4」（预期待核：按 15b:171 的计法推的，没实测）。
  - 第 2 步：按 Ctrl-C 后表头变「council 0.2.0-0929a   正在取消  发言轮 1/2 …  Ns」，秒数照走；然后「已跑 1/2 轮，其余取消」，再「/round 已取消  Ns  回来 x/2」，表头回到「空闲」，程序没退（00_DECISIONS.md:2011）。取消后不总结（summary.go:73-75）。
  - 屏幕回到空闲，不等于子进程真的停了。想顺带查，照 K2 第 2 步与第 4 步那条 `Get-Process` 命令，在按 Ctrl-C 之前、之后各跑一次（15 §6 的 P1 探针，15:178-188）。
- [ ] 符合
- [ ] 不符合

#### D6 裁判成员默认不问、打一行提示；加 `--with-judge` 时发言轮之后另跑一段裁判轮

- 状态：真机通过（Griffin，0.2.0-rba）
- 出处：00_DECISIONS.md:1972（RB-A §6 六步都正常，第 3 步是 `/round a b j --go --rounds 2 --with-judge`，15a:619）；s10 的 RB-B 第 2 项同样带 `--with-judge`（15b:652；00_DECISIONS.md:2011）；12 §4.6a.7、§4.7；9b7824d、e2218d3；提示行 round.go:549-554。第 1 步那行提示没有真机记录。记录早于 CO1 与 SM1。
- 前置：配置里要有一个 `mode = "judge"` 的成员并已加入会话；你 09-28 的配置里多半没有（19a:1691），要在沙盒配置里加一个。
- 花费：第 1 步零花费；第 2 步花钱：甲、乙、裁判各 1 次（加了 `--no-summary`，不总结）。

步骤：

第 1 步

```
/round <成员甲> <成员乙> <裁判>
```

第 2 步

```
/round <成员甲> <成员乙> <裁判> --go --with-judge --no-summary
```

- 预期：
  - 第 1 步：「裁判 <裁判> 本命令不问，要用 --with-judge 才问。」，预算表里只有甲、乙，不起进程（round.go:549-554）。
  - 第 2 步：先发言轮（甲、乙），再「裁判轮」一段；收尾行「/round 结束  Ns  回来 3/3」（预期待核：2+1 是推算的，没实测）。
- [ ] 符合
- [ ] 不符合

#### D7 多个成员回复之后，总结成员自动加入，出一段「总的来看（…做总结）：…」，只给你看

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2116、:2150、:2155；HANDOFF.md:2680；16c §3.2、§6 第 6 步（16c:1125 起）；a877d44；配了总结成员时收尾行是 3/3（smx_summary_tui_test.go:76、:128）。
- 前置：配了总结成员（你已配 `fable-5.1-summary`，00_DECISIONS.md:2159；它的 model 字符串能不能用未验证，:2160）；会话里两个讨论成员。
- 花费：花钱：甲、乙各 1 次 + 总结 claude 1 次（总结成员在这个会话里第一次开口是冷启动，约 1 美元量级，00_DECISIONS.md:2139）。第 7 步可选，另花甲 1 次。

步骤：

第 1 步

```
/new 总结复验
```

第 2 步

```
/add <成员甲>,<成员乙>
```

第 3 步

```
/round <成员甲> <成员乙> --go
```

第 4 步

```
/who
```

第 5 步

```
/log 30
```

第 6 步

```
/cost
```

第 7 步（可选，花钱：甲 1 次）：检验「只给你看」。退出后用同一条命令重启，回到这个会话问成员甲。

```
/quit
```

（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml
```

```
/use <会话 id 的前 6 位>
```

```
/ask <成员甲> 刚才有没有人对这一轮做过总结？如果有，原话是什么？
```

- 预期：
  - 第 2 步：两行「<id> 已加入会话 <会话 id>」。
  - 第 3 步：两个成员头之后「本轮问 1 个: fable-5.1-summary」，再一行「总的来看（Fable 5.1 做总结）：…」（总结头不带用时，00_DECISIONS.md:2181）；看板多出一段「总结轮」；收尾行「/round 结束  Ns  回来 3/3」（smx_summary_tui_test.go:76、:128）。
  - 第 4 步：总结成员已经在表里，你没手动 `/add` 过它。
  - 第 5 步：总结那一行 TO 列是总结成员自己的 id，其余行 TO 是 `-`（00_DECISIONS.md:2155）。
  - 第 6 步：总结成员单独一行，金额计入（16c:1197-1199）。
  - 第 7 步：成员甲复述不出总结的内容：它的 prompt 里没有那条消息（总结的 Recipients 只有总结成员自己，00_DECISIONS.md:2155；16c:1204-1209）。它答得出总结原话就算不符合。
- [ ] 符合
- [ ] 不符合

#### D8 `--no-summary` 跳过总结；只问一个人时不总结；总结成员不许出现在 `/round` 名单里

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2116、:2150、:2155；16c §3.2 第 7 行、§3.5、§6 第 6 步之 3、之 5（16c:1170-1199、:1210）；a877d44；round.go:157-158。
- 前置：做完 D7（总结成员已是活跃成员），接着 D7 的会话做。
- 花费：第 1 步甲、乙各 1 次；第 2 步甲 1 次；第 3 步零花费；第 4、5 步各是甲、乙各 1 次 + 总结 claude 1 次。共 9 次。

步骤：

第 1 步

```
/round <成员甲> <成员乙> --go --no-summary
```

第 2 步

```
/round <成员甲> --go
```

第 3 步

```
/round <成员甲> fable-5.1-summary --go
```

第 4 步：详细档下的总结段。

```
/verbose on
```

```
/round <成员甲> <成员乙> --go
```

```
/verbose off
```

第 5 步：行式下总结的位置。退出后用行式重进这个会话。

```
/quit
```

（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

```
/use <会话 id 的前 6 位>
```

```
/round <成员甲> <成员乙> --go
```

- 预期：
  - 第 1 步：没有总结，收尾行「回来 2/2」。
  - 第 2 步：只有成员甲，没有总结，「回来 1/1」。
  - 第 3 步：「拒绝: fable-5.1-summary 是总结成员（config.toml 的 [summary]），不在 /round 里发言；它的话只给你看，其余成员看不到。」，不起进程。总结成员还没加入过时，先打的是「拒绝: 不是当前活跃成员: fable-5.1-summary」（round.go:157-158）。
  - 第 4 步：总结段照裁判轮的详细格式：预算表、「本轮问 1 个」、`outcome spoke  agent fable-5.1-summary` 整块、`总结轮结算`、段终态块「总结轮 (xxxxxx)  回来 1/1」（16c:1194）。
  - 第 5 步：两个成员头、总结那一行，最后一行「总用时 N.Ns」；总结在「总用时」之前（16c:1210）。D16 可以接着在这个行式进程里做。
- [ ] 符合
- [ ] 不符合

#### D9 消息里写 `@成员id` 就是点名；`/round` 不带名单时取最后一条用户消息里的点名

- 状态：已落地未复验
- 出处：00_DECISIONS.md:1747（带点号 id 的 @ 解析只有单测，没上过真机）；11d（8c-1）b073fde；12 §4.6a.2 第 2 步；字面量 autoreply.go:16。
- 前置：会话里有两个成员。第 2 步把 <成员甲> 连尖括号一起换成真 id（例如 `@grok-4.7`），@ 前面空一格或在行首。
- 花费：零花费。

步骤：

第 1 步

```
/auto off
```

第 2 步

```
@<成员甲> 看一下
```

第 3 步

```
/round
```

- 预期：
  - 第 1 步：「已关闭自动回复：直接打字只记下，不问任何人；要成员说话用 /ask 或 /round；/auto on 打开」（autoreply.go:16）。
  - 第 2 步：简洁档什么都不打（详细档打「点名了 <成员甲>（用 /ask <成员甲> 让他说话）」），不起进程。
  - 第 3 步：只出预算表，表里只有成员甲一行，不起进程。
- [ ] 符合
- [ ] 不符合

#### D10 直接打字就让成员回复：没点名问全部成员（不含总结成员），@ 了谁只问谁，多人回复后自动总结

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2196、:2238-2242；HANDOFF.md:2703-2704、:2719；19a §3.2、§3.7、§6 第 5 步（19a:1562-1612）；4d3dc0b；README.md:63-80。19a:1604 写的「回来 2/2」只对没配总结成员的装置成立（chx_program_test.go:19）；配了总结成员是 3/3（smx_summary_tui_test.go:76、:128），这里取 3/3。
- 前置：配了总结成员；新会话；两个讨论成员；`/auto on`（默认就是开的）；在 TUI 里做（前面停在行式的话先 `/quit`，用 TUI 重启）。第 5 步把 <成员乙> 连尖括号一起换成真 id，否则这句谁都不问、白敲一次（19a:1562）。
- 花费：第 3、4 步各是甲、乙各 1 次 + 总结 claude 1 次；第 5 步乙 1 次。共约 7 次。第 6 步可选，另花乙 1 次。
  - 第 3 步是总结成员在这个新会话里第一次开口，按首轮（冷）计，约 1 美元量级（00_DECISIONS.md:2139）：provider 会话按「会话 + 成员」分（turn/session.go:23-31），在 D7 的会话里开过口不算。这是按会话键推的，实际金额受缓存前缀跨会话共享影响（HANDOFF.md:692），没核。

步骤：

第 1 步

```
/new 对话模式复验
```

第 2 步

```
/add <成员甲>,<成员乙>
```

第 3 步

```
用一句话介绍你自己
```

第 4 步

```
再用一句话说说你们的分歧
```

第 5 步

```
@<成员乙> 你再补一句
```

第 6 步（可选，花钱：乙 1 次）：行式下复验一次点名（19a §6 第 5 步之 9，19a:1679）。退出后用行式重进这个会话；行式下这一轮进行中不要按 Ctrl-C。

```
/quit
```

（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

```
/use <会话 id 的前 6 位>
```

```
@<成员乙> 行式下再补一句
```

- 预期：
  - 第 3 步：「本轮问 2 个: …」，两人的回复（带用时），「本轮问 1 个: fable-5.1-summary」，「总的来看（… 做总结）：…」，收尾行「/round 结束  Ns  回来 3/3」。自动一轮的收尾行仍写「/round 结束」（19a:1689）。
  - 第 4 步：与第 3 步同形，没有「拒绝: … 是总结成员」（19a:1611）。
  - 第 5 步：只问成员乙，没有总结，「回来 1/1」。
  - 第 6 步：只问成员乙，没有总结；行式没有收尾行，最后一行「总用时 N.Ns」（16b:1106）。
- [ ] 符合
- [ ] 不符合

#### D11 `/auto` 切换直接打字要不要让成员回复；没有成员时打字只记下并给提示；`council -h` 里有 `-no-auto`

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2196；19a §3.1、§3.2 第 2 句、§3.5、§6 第 5 步之 1、3、7（19a:1563、:1587-1593）；4d3dc0b；字面量 main.go:45，autoreply.go:11、:14-16、:25；用法分支 autoreply.go:52-62。
- 前置：TUI；从第 2 步起都在第 2 步新建的、还没加人的会话里做。会话里没有成员时，打开自动回复打字会出第 3 步那一行，关掉就什么都不打，照样看得出开关有没有生效，而且两边都不起进程。
- 花费：零花费。

步骤：

第 1 步（PowerShell）

```
council -h
```

第 2 步

```
/new 对话模式-空
```

第 3 步

```
还没人
```

第 4 步

```
/auto off
```

第 5 步

```
关掉之后这句也只记下
```

第 6 步

```
/auto x
```

第 7 步

```
/auto on
```

第 8 步

```
/auto off
```

- 预期：
  - 第 1 步：有 `-no-auto`，说明是「直接打字只记下、不让成员回复（默认会问当前会话的成员；进去之后也可以用 /auto 切换）」（main.go:45）。`-no-auto` 是否真的生效，见 D15 第 3 步。
  - 第 2 步：「已建会话 <会话 id>，标题: 对话模式-空」（commands.go:782）。不先建会话的话，第 3 步打的是「先 /new 或 /use 选一个会话」（commands.go:66）。
  - 第 3 步：「当前会话还没有能回复的成员，这条只记下了；用 /add 加成员之后，直接打字就会有人回复」，不起进程（autoreply.go:25）。
  - 第 4 步：「已关闭自动回复：直接打字只记下，不问任何人；要成员说话用 /ask 或 /round；/auto on 打开」（autoreply.go:16）。
  - 第 5 步：什么都不打，没有第 3 步那一行，不起进程。
  - 第 6 步：「用法: /auto 在自动回复与只记下之间切换；/auto on 直接打字就让成员回复；/auto off 直接打字只记下」（autoreply.go:11），开关不变。
  - 第 7 步：配了总结成员时是「已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁），多个成员回复之后自动总结；/auto off 关掉」（autoreply.go:14）。
  - 第 8 步：同第 4 步。停在只记下，免得后面随手打字花钱。
- [ ] 符合
- [ ] 不符合

#### D12 冲着成员却写错的 @，或点的不是成员：这句谁都不问，打一行说明，给出 `/round` 出路

- 状态：已落地未复验
- 出处：19a 文件头第 9 行（判据改过四次）、§3.2 第 1、3 句、§7.1 第 12 条、§6 第 5 步之 7（19a:1619-1677，v1.6、v1.7 两种写法在 :1633）、之 9（:1679）；4d3dc0b；README.md:70-79；字面量 autoreply.go:23、:30。只在原型与单测里验过。
- 前置：会话里有成员；`/auto on`。警告：这一项要开着自动回复，每一句都设计成谁都不问；敲错一个字就可能问全员并总结。第 3 步要用中文输入法打全角 `＠`（半角的 `@所有人` 会问全员，要花钱）。第 4 步的显示名取配置里的 display_name（例如 `Fable 5.1`，00_DECISIONS.md:2159）。
- 花费：零花费（照原样敲时）。

步骤：

第 1 步

```
@nobody 这句谁都不该回
```

第 2 步

```
@ <成员乙> 这句也谁都不该回
```

第 3 步

```
看一下 ＠所有人
```

第 4 步

```
@ <某成员的显示名> 这句也谁都不该回
```

第 5 步（v1.6）：半角 `@` 紧跟中文输入法打的全角括号，括号里是成员乙的 id，例如 `@（grok-4.7）这句也谁都不该回`。

```
@（<成员乙>）这句也谁都不该回
```

第 6 步（v1.7；显示名里有英文字母才做，全是中文就跳过）：`@`、空格、大小写换过的显示名。例如显示名是 `Fable 5.1`，就敲 `@ fable 5.1 这句也谁都不该回`。显示名拼错会问全员并总结（花钱），没把握就跳过。

```
@ <大小写换过的显示名> 这句也谁都不该回
```

第 7 步

```
/log 7
```

第 8 步

```
/auto off
```

第 9 步（行式，零花费；放在 §2.1 第 7 组做）：行式下复验一次（19a §6 第 5 步之 9，19a:1679）。沙盒 council 还开着的话先在它里面 `/quit`（§0.4），再用行式启动（默认对话模式），回到这个会话。

（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

```
/use <会话 id 的前 6 位>
```

```
@nobody 这句谁都不该回
```

```
/quit
```

- 预期：
  - 第 1 步：「nobody 不是当前会话的成员」，再一行「点名的人都不是当前会话里能回复的成员，这条只记下了、没有问任何人；不点名就会问全部成员；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复」（autoreply.go:23），不起进程。
  - 第 2-6 步：都是一行「有 @ 没认出点的是谁，这条只记下了、没有问任何人；点名写半角 @ 紧跟成员 id（/who 第一列），不带尖括号，不是显示名，@ 前面空一格或在行首；这条本来就不是点名的话，接着敲 /round、空格、成员 id（多个用空格隔开）、空格、--go 让成员回复；TUI 里敲 @ 会列出成员的 id 与显示名」（autoreply.go:30），不起进程。第 5、6 步在 v1.5、v1.6 下会问全员加一次总结（19a:1633），0929a 应当谁都不问。
  - 第 7 步：六句都记下了（第 6 步跳过时是五句）。
  - 第 9 步：与第 1 步相同的两行，不起进程。
- [ ] 符合
- [ ] 不符合

#### D13 贴进来的代码里的 @（PowerShell `@{`、numpy `A @ B`、`价格 @ 5 元`）不算点名，照问全部成员

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2196、:2246-2247；19a §7.1 第 12 条、§7.3 第 5 条（19a:1733-1736）；§6 第 5 步之 7；4d3dc0b；README.md:74-78。
- 前置：会话里有两个讨论成员，配了总结成员；`/auto on`；在 TUI 里做（接着 D10 的会话；做过 D10 第 6 步的话先 `/quit`，用 TUI 重启、`/use` 回来）。
- 花费：花钱：全部讨论成员各 1 次 + 总结 claude 1 次。

步骤：

第 1 步

```
$h = @{ Name = 'x' } 这行为什么报错
```

- 预期：与 D10 第 3 步同形：本轮问 2 个、两人回复、总结、「回来 3/3」；没有「有 @ 没认出」那一行（19a:1736 原型探针）。
- [ ] 符合
- [ ] 不符合

#### D14 一轮多个成员时谁先回来先打谁，不等最慢的；`/ask` 起进程前先打「正在问 <id> ...」

- 状态：已落地未复验
- 出处：b34bbe4（UX-1，README.md:59：不在 Step 10 原契约里）；HANDOFF.md:1840（两条提前返回路径「代码证实、无用例担保」）；ask.go:164。RB-A、RB-B 真机看到成员逐个回来（00_DECISIONS.md:1972、:2011），但没有单独点名 UX-1 的复验记录。
- 前置：会话里一快一慢两个成员（例如一个 claude 高强度成员配一个 grok 成员）。可以和 D4 合成一次跑。
- 花费：花钱：两人各 1 次。

步骤：

第 1 步

```
/round <慢成员> <快成员> --go --no-summary
```

- 预期：快的那个人的回复先上屏（「<显示名>（x.xs）: …」）；慢的那个还在跑时，看板上他那行仍是「在跑」；两人的输出不互相穿插。
- [ ] 符合
- [ ] 不符合

#### D15 启动参数 `-verbose` 与 `-no-auto` 真的生效（不只是 `council -h` 里有说明）

- 状态：已落地未复验
- 出处：main.go:43、:45、:106-107（两个 flag 经 Verbose、NoAuto 传进 app.Options）；app.go:235-236、:241-242（TUI 与行式都按它们设档位与对话模式）；16b §6 第 5 步之 5、之 6（16b:1111-1125），16b:1142 写明 main.go 里这段传递只在审查时看过、要靠真机；19a §6 第 5 步之 8（19a:1678）。没找到真机记录。
- 前置：先在已开着的沙盒 council 里 `/quit`（§0.4）。
- 花费：零花费。这里有两处有意不照契约原样做：
  - 16b:1111-1125 用 `/round` 实跑来看详细输出，要花钱；这里改用切换命令的回话判断启动时的档位。`/verbose`、`/auto` 不带参数是切换（verbosity.go:60-75；autoreply.go:52-70），回的那一句说明切换之前是哪一档。
  - 19a:1678 在有成员的会话里打一句；flag 没生效时那一句会问全员并总结，要花钱。这里改在没有成员的会话里打：flag 没生效时打的是 D11 第 3 步那一行（autoreply.go:25），同样不花钱。

步骤：

第 1 步（PowerShell）：TUI、详细档启动，然后切一次档位、退出。

```
council --config E:\Isolating_test\council\mvp\config.toml -verbose
```

```
/verbose
```

```
/quit
```

第 2 步（PowerShell）：行式、详细档启动，同样切一次、退出。

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui -verbose
```

```
/verbose
```

```
/quit
```

第 3 步（PowerShell）：TUI、只记下启动，在一个还没有成员的新会话里打一句，再切一次开关。

```
council --config E:\Isolating_test\council\mvp\config.toml -no-auto
```

```
/new 启动参数
```

```
开着 -no-auto 这句只记下
```

```
/auto
```

```
/auto off
```

```
/quit
```

- 预期：
  - 第 1、2 步：`/verbose` 回「已切到简洁输出：只显示成员的回复、用时，以及关系到钱或内容丢失的提示；/verbose on 切到详细」（verbosity.go:14），说明启动时是详细档。回的是「已切到详细输出：…」就是 `-verbose` 没传进去。
  - 第 3 步：「已建会话 <会话 id>，标题: 启动参数」；打那一句之后什么都不打，没有「当前会话还没有能回复的成员…」那一行（出了就是 `-no-auto` 没生效，autoreply.go:25）；`/auto` 回「已打开自动回复：直接打字会问当前会话的成员（点名了谁就只问谁），多个成员回复之后自动总结；/auto off 关掉」（autoreply.go:14），说明启动时是关的；`/auto off` 回「已关闭自动回复：…」（autoreply.go:16）。
- [ ] 符合
- [ ] 不符合

#### D16 `/leave` 总结成员之后这个会话不再总结，打一行说怎么恢复；`/join` 总结成员能恢复

- 状态：已落地未复验
- 出处：summary.go:94-96（总结成员已退出时打那一行、不拉回）；16c §7.1 第 4 条（16c:1222）；恢复的路：显式 `/add`、`/join` 总结成员只在它是这个会话里已退出的成员时放行（commands.go:1029-1033；19a §7.1 第 8 条，19a:1694）。`/leave` 不拦总结成员（commands.go:1107-1117）。没找到真机记录。
- 前置：做完 D7（总结成员已是这个会话的成员），接着 D7 的会话做（D8 第 5 步之后在行式里接着做也行）；会话里甲、乙两个讨论成员。
- 花费：第 2 步甲、乙各 1 次；第 4 步甲、乙各 1 次 + 总结 claude 1 次。共 5 次。

步骤：

第 1 步

```
/leave fable-5.1-summary
```

第 2 步

```
/round <成员甲> <成员乙> --go
```

第 3 步

```
/join fable-5.1-summary
```

第 4 步

```
/round <成员甲> <成员乙> --go
```

- 预期：
  - 第 1 步：「fable-5.1-summary 已退出会话 <会话 id>」（commands.go:1116）。
  - 第 2 步：两个成员回复之后一行「总的来看：没有总结（fable-5.1-summary 已退出当前会话；要恢复总结用 /join fable-5.1-summary）」（summary.go:95）；没有「本轮问 1 个: fable-5.1-summary」，不起总结的进程。
  - 第 3 步：「fable-5.1-summary 已加入会话 <会话 id>」，不是「… 是总结成员（config.toml 的 [summary]），不用 /add …」那句拒绝（commands.go:1032-1033）。
  - 第 4 步：总结回来了，同 D7 第 3 步（行式下没有收尾行，总结在「总用时」之前）。
- [ ] 符合
- [ ] 不符合

### 3.5 模型识别与 codex 成员（M）

#### M1 不带参数的 `/add`：先列配置候选，再识别各家 CLI 的模型，列出配置里还没有的并编号

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2151 记了你 2026-09-28 17:30 在 Windows Terminal 的 TUI 里、用真配置、0.2.0-0928b 跑出的识别表原件（约 10 秒出表，33 行，已配置的都藏掉了），内容与 16a §6 第 5 步的期望逐项相符；但原文没写「通过」，#45 标题与 :2150 仍写「真机没有复验」，同一次还撞出总结成员误列在候选里（:2154，CH1 已修、修后没复验），所以这里不记「真机通过」。契约 16a §3.2（:2220-2252）、§6 第 5 步（:2523-2558）；18a §3 L1-L4（:1003-1019）、§6 第 6 步（:1139-1157）；MD1 e9afff9、CXH db8c41a。
- 前置：配置里写了 `[providers.*]` 的那几家才会识别（18a:103）；这一步不写配置，日常库与沙盒都可以。和 C5 用同一次 `/add`：识别结果按进程缓存（快照是 REPL 进程的字段，repl.go:118-119；`/new` 不清它，只有 `/add --refresh` 或重启才清，commands.go:886-891），只有这个 council 进程里第一次不带参数的 `/add` 才起识别、打「正在识别模型」（addmodel.go:166-176）。做过 C5 第 1、2 步就别再 `/new`、`/add`，第 1 步直接看 C5 第 2 步那次的输出。
- 花费：零花费（识别不推理：claude 走握手，没有 /v1/messages，00_DECISIONS.md:2124-2125；codex 用本地 debug models，:2185）。agy 识别会写 `~/.gemini` 日志（:2123）。

步骤：

第 1 步：就是 C5 第 1、2 步（`/new 候选复验`，然后不带参数的 `/add`）。没做 C5 时照那两块敲；做过就只看那次的输出。

第 2 步

```
/add
```

- 预期：
  - 第 1 步（这个进程里第一次 `/add`）：先打配置候选表（见 C5）；再「正在识别模型: agy, claude, codex, grok」；接着「识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:」，表头「NO PROVIDER ID MODEL EFFORTS SOURCE」。SOURCE 列：claude 为「订阅」，grok 为「本地配置」，agy 的 Claude / GPT-OSS 行为「走 agy 额度」，codex 为「codex 目录」。提示行「用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档」；另有「codex 目录里还有 2 个 custom/ 开头的模型没有列出：…」一行（18a:1008、:1013）。某家失败时打「识别 <家> 的模型失败: …」。你已配置的 `claude-fable-5-1`、`grok-4.7`、`Gemini 3.8 Flash (High)` 不在表里（00_DECISIONS.md:2151）。
  - 第 2 步：不再出「正在识别模型」，秒出，编号与上次相同（16a:2558）。
  - 装了含 OB3 的版本时，两步都在识别结果之后、「本机装了 …」提示之前多一段「各家的登录状态」，这一段按 M6 判。
- [ ] 符合
- [ ] 不符合

#### M2 识别结果按进程缓存成快照（`/new` 不清它）；`/add --refresh` 丢掉快照重新识别；TUI 里识别途中 Ctrl-C 取消，取消的结果不缓存

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2147、:2150（MD1「真机没有复验」，:2151 只记了识别表本身）；16a §3.2（:2224、:2230-2231）、§6 第 5 步（:2546、:2552、:2590）；MD1 e9afff9。快照是 REPL 进程的字段（repl.go:118-119），只有 `/add --refresh`（commands.go:886-891）或重启才清；取消时不赋值（addmodel.go:166-176）。16a 里写的「本会话」指这个 council 进程，不是 `/new` 建的会话。
- 前置：TUI（Ctrl-C 那一步）；接着 M1 做就行，不用新进程：第 1 步用 `/add --refresh` 先把 M1 留下的快照清掉，再取消。
- 花费：零花费。

步骤：

第 1 步：敲下面这条，看到「正在识别模型: …」后、表出来前按一次 Ctrl-C（agy 一家就要 4-5 秒，16a:2546）。

```
/add --refresh
```

第 2 步

```
/add
```

第 3 步

```
/add --refresh
```

- 预期：
  - 第 1 步：先打配置候选表（`--refresh` 也先打候选再识别，commands.go:920-925），再「正在识别模型: …」；按下 Ctrl-C 之后只多一行「识别被取消；下次 /add 会重新识别」（addmodel.go:171-172），不出识别表、不出失败行。按晚了、表已经出来，这一步作废，记下来即可（16a:2546）。
  - 第 2 步：重新出现「正在识别模型: …」，然后出表；没出现就说明取消被缓存了（16a:2552）。
  - 第 3 步：又出现「正在识别模型」一行（16a:2590）。某家失败时表后打「失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh」（16a:2230）。
- [ ] 符合
- [ ] 不符合

#### M3 `/add +编号 [--id <新 id>] [--effort <强度>]`：把识别出的模型追加写进 config.toml，写前自动备份，并加入会话

- 状态：主代理隔离验过（零花费）
- 出处：00_DECISIONS.md:2187（codex 复验时在隔离配置里跑 `/add +1 --effort low --id gpt-6-sol-low`，「同时真机验了 MD1 的追加写配置与自动备份」，只有 codex 一家）；claude 行的 `--effort`、grok / agy 行拒绝 `--effort`、各种拒绝文本只有单测。16a §1.1 B、§3.2（:2235-2249）、§3.3（:2256-2262）、§3.5-3.6（:2276-2284）、§6 第 5 步（:2560-2596）；MD1 e9afff9；addmodel.go:473。
- 前置：必须用沙盒配置，它会写生效的那份 config.toml（16a:2523）。先在这个 council 进程里跑过一次不带参数的 `/add`（M1），拿到编号；快照按进程，重启就没了（repl.go:118-119）。
- 花费：零花费（写配置不调模型）。新成员第一次 `/ask` 是冷启动，另计。

步骤：

第 1 步：挑一个 claude 行，而且它的 EFFORTS 列里有 `max`（第 3 步要用同一个编号换 max 强度），把它的编号记为 N。哪几行有 max 以屏幕为准；haiku 那一行 EFFORTS 是 `-`，opus-4.7、opus-4.6 两行没有 xhigh（00_DECISIONS.md:2151）。

```
/add +N
```

第 2 步（PowerShell）：看沙盒配置末尾，并找备份。

```
notepad E:\Isolating_test\council\mvp\config.toml
```

```
Get-ChildItem E:\Isolating_test\council\mvp\config.toml.bak-add-*
```

第 3 步

```
/add +N --effort max
```

第 4 步：挑一个 grok 或 agy 行，记为 M。

```
/add +M --effort max
```

第 5 步

```
/agents
```

- 预期：
  - 第 1 步两行：「已把 agent <建议 id> 追加进 <沙盒配置路径>（原文件备份在 <备份路径>）」与「<建议 id> 已加入会话 <会话 id>」（16a:2249；addmodel.go:473）。
  - 第 2 步：原有内容一字没动，末尾多一段以「# council /add 追加：<provider> 识别出的 <model>（<来源>）」开头的 `[agents.…]` 表，其中 `role = ""`、`permissions = "read"`、`mode = "discussion"`；备份名是 `<配置文件>.bak-add-YYYYMMDD-HHMMSS`（16a:2280、:2284）。
  - 第 3 步：加进一个以 `-max` 结尾的新成员（同一模型换强度是新 id，16a:2584）。挑的那一行 EFFORTS 里没有 max 时打「<model> 支持的强度是 …，没有 max」；EFFORTS 是 `-` 时打「<model> 没有可选的强度，不能带 --effort」（addmodel.go:353-364），都不加成员。那是挑错了行，换一行重做，不算不符合。
  - 第 4 步：「<provider> 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort」。<provider> 是那一行的 provider（addmodel.go:348 的 `%s` 是 `m.Provider`），屏幕上是「grok 的强度不在 council 里选…」或「agy 的强度不在 council 里选…」（16a:1932、:1935）。
  - 第 5 步：表头「ID  PROVIDER  MODEL  EFFORT  PERMISSIONS  MODE」（commands.go:590）；新成员在，EFFORT 为 high 与 max，PERMISSIONS 为 read，MODE 为 discussion（16a:2572）。
  - 其它拒绝（不必专门造）：id 不合法时「id <x> 不合法（要匹配 …）；用 --id <新 id> 换一个」（16a:2257）；配置在 council 外被改过时「配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add +N」（16a:2245）。
- [ ] 符合
- [ ] 不符合

#### M4 codex 当成员：`/ask` 起 codex、续同一个 thread；read 档下 shell 与 apply_patch 写不了盘

- 状态：主代理隔离验过（5 次付费）
- 出处：00_DECISIONS.md:2186-2192（2026-09-28，0.2.0-0928b 行式，gpt-6-sol 用 low 强度：第 6-8 步同一个 thread、答出 KIWI；两次写盘探针 shell、apply_patch、`mcp__node_repl__js` 全被拒）；#46 标题（:2162）的「真机未复验」指你没复验。没验的：TUI、high 强度、失败形状（:2193）。17b §3（:852-958）、§6 第 5-17 步（:1111-1252）；CX1 c0e6a27、CXH db8c41a。
- 前置：沙盒配置里有 `[providers.codex]`（你的真配置已有，00_DECISIONS.md:2194），先用 M3 的 `/add +编号` 加一个 codex 成员（建议 `--effort low`，默认 high 比 codex 自己的默认 medium 贵，18a:6）。MCP 风险你已接受（00_DECISIONS.md:2178）。
- 花费：花钱：codex × 3 次（第 2-4 步）。Council 不记 codex 的用量与金额，只存原始对象（00_DECISIONS.md:2191）。

步骤：

第 1 步

```
/verbose on
```

第 2 步

```
/ask <codex 成员> 先逐行列出你此刻能调用的全部工具名，列完另起一行只写 PONG，并记住单词 KIWI
```

第 3 步

```
/ask <codex 成员> 上一轮让你记住的单词是什么？只回答那一个词
```

第 4 步

```
/ask <codex 成员> 在当前目录建一个 PROOF.txt 并写入 WRITTEN，如实报告成功还是被拒
```

第 5 步（PowerShell）

```
Test-Path E:\Isolating_test\council\mvp\data\workdir\<codex 成员>\PROOF.txt
```

- 预期：
  - 第 2 步：「outcome spoke」「新开了一条 provider 会话 <UUID>」，正文有工具清单和单独一行 PONG；用量一行「用量 in=? cache_create=? cache_read=? out=? thinking=?」，然后「金额未报」「冷暖 预报=冷 实测=未知 …」（17b:957、:1165）。
  - 第 3 步：正文 KIWI；「provider 会话 <同一个 UUID>」；「冷暖 预报=暖」（17b:1172）。
  - 第 4 步：模型说写不了。第 5 步：False。任何一个 True 就到此为止，去掉 codex 成员（17b:1194）。
  - 任何一步出现「送出去的会话 id 与回来的不一样」，先 `/session rebuild <codex 成员>` 再往下做：codex 的身份只随空会话首轮发（17b:1154-1157、:1285）。MCP 与空会话形状的写盘变体见 17b §6 第 10-15 步。
- [ ] 符合
- [ ] 不符合

#### M5 codex 成员配错时在起进程之前就拒、不花钱：强度不是合法档，或权限写成 workspace / full

- 状态：已落地未复验
- 出处：只有单测（17b §3.7 T6；cxx_codex_test.go:270-271）；17b §1.1 A 注释要点 5、P4，§3.4-3.5（:890-912）、§3.7（:948-951）；codex.go、provider.go:142；CX1 c0e6a27。#46 的真机复验只跑了 read 档（00_DECISIONS.md:2186-2193）。
- 前置：沙盒配置里有 codex 成员（见 M4 前置）；只能改沙盒配置；council 开着时改文件。
- 花费：零花费。

步骤：

第 1 步（PowerShell）：把沙盒配置里 <codex 成员> 的 effort 改成 `turbo`，保存。

```
notepad E:\Isolating_test\council\mvp\config.toml
```

第 2 步

```
/config reload --apply
```

```
/ask <codex 成员> 你好
```

第 3 步（PowerShell）：effort 改回原值，再把 permissions 改成 `workspace`，保存。

```
notepad E:\Isolating_test\council\mvp\config.toml
```

第 4 步

```
/config reload --apply
```

```
/rules
```

```
/ask <codex 成员> 你好
```

- 预期：
  - 第 2 步：「<显示名>: 没有回复（拒绝）」，下一行含「effort "turbo" is not a codex reasoning level (want one of minimal, low, medium, high, xhigh, max, ultra): effort value not supported by this provider」，不起进程（17b:899、:950）。
  - 第 4 步：`/rules` 里 codex 那一行末格是「不适用（permission tier "workspace": no verified write boundary: permission tier not supported by this provider）」；`/ask` 打「<显示名>: 没有回复（拒绝）」，拒绝文本含同一句英文（cxx_codex_test.go:270），不起进程。
  - 若这个成员已开过口，`--apply` 之后先报身份漂移，要先 `/session rebuild`。改成 full 时拒得更早，打的是 Q4 那一句。
- [ ] 符合
- [ ] 不符合

#### M6 不带参数的 `/add` 在识别结果之后显示各家的登录状态（OB3）

- 状态：已落地未复验（`ef76eef`；已装，0930c 起含它）
- 出处：00_DECISIONS.md #48 的 OB3 落地记录；契约 `19f` §1.1 D1-D9、§3（字面量）、§6 第 5 步（下面的步骤是它的精简版）。判据与首次运行相同，实测出处是 19d §0 与 19e §0。
- 前置：装了含 OB3 的版本。用你自己的真配置就行，这一项只读不写配置；`/add` 要有当前会话。
- 花费：零花费（识别本来就在 `/add` 里做；只多跑一次 `codex login status`，19d 实测 0.16 s）。

步骤：

第 1 步：开 council（TUI 或 `council -no-tui` 都行），开一个会话。`/new` 会在库里多一个空会话；不想多，就 `/conv` 之后 `/use <已有会话>`。

```
/new 登录状态复验
```

第 2 步

```
/add
```

第 3 步

```
/add
```

第 4 步

```
/add --refresh
```

- 预期：
  - 第 2 步：候选、「正在识别模型: agy, claude, codex, grok」、识别表与后面那几行照旧；然后是新的一段（顺序同「正在识别模型」）：
    - 「各家的登录状态（本会话的快照，/add --refresh 重新检查）:」
    - 「  agy     登录状态查不出」（不带登录办法）
    - 「  claude  已登录（订阅：Claude Max）」（订阅名是你账户的原值）
    - 「  codex   已登录」（你用的是 ChatGPT 登录，不是「已登录（API key）」）
    - 「  grok    已登录（API key）」（经 CPA 的 key）
    - 某一家「没登录」时，那一行末尾是「；在终端里运行 <登录办法>，登录之后 /add --refresh」。
  - 第 3 步：不再出「正在识别模型」，登录段与第 2 步逐字相同（快照）。
  - 第 4 步：重新出「正在识别模型」，登录段重新检查一次。
  - Mac 上的 CPA 睡着时 grok 那一行会不会变，没测（19e §7 (g)）；「已登录（API key）」只说明配了 key，不说明 key 能用。
- [ ] 符合
- [ ] 不符合

#### M7 `/add` 按名字加识别出的模型（AD1）

- 状态：已落地未复验（`d4ad85f`；已装，1001a 起含它）
- 出处：00_DECISIONS.md #49；契约 `19g` §1.1 D1-D17、§3（字面量）、§6 第 5 步（下面的步骤是它的精简版，细节与全部预期以 19g 为准）。gpt-6.1-sol 的强度档（low、medium、high、xhigh、max、ultra）是主代理 2026-09-30 在 E 盘机上跑 `codex debug models` 实测的。
- 前置：装了含 AD1 的版本。用你自己的真配置，**会写配置，写前自动备份**。**只敲斜杠命令、不要直接打字**：对话模式默认打开，直接打字会让会话里的成员回复，要花钱。
- 花费：零花费（只做识别，与不带参数的 `/add` 同一套，不提问）。

步骤：

第 1 步：照常开 council（TUI 或 `council -no-tui` 都行），开一个新会话。

```
/new AD1复验
```

第 2 步：第一次就敲（本会话还没有识别过，要自动识别）。

```
/add gpt-6.1-sol-xhigh
```

第 3 步

```
/who
```

第 4 步：敲两次同一条。

```
/add gpt-6.1-sol
```

```
/add gpt-6.1-sol
```

第 5 步：撤回（只是退出这个会话，配置里那一段还在）。

```
/leave gpt-6.1-sol-xhigh
```

第 6 步（可选，要连配置一起撤回）：先 `/quit`，再在 PowerShell 里用第 2 步打出来的备份覆盖回去。

```
Copy-Item -LiteralPath '<第 2 步那一行里的备份路径>' -Destination '<第 2 步那一行里的配置文件路径>'
```

第 7 步（可选，两个词的写法，要在配置里还没有 gpt-6.1-sol 时做，也就是第 6 步撤回之后）：重开 council，新会话里第一次就敲，一行一块。

```
/new AD1复验2
```

```
/add gpt-6.1-sol xigh
```

```
/add gpt-6.1-sol xhigh
```

- 预期：
  - 第 2 步三行：「正在识别模型: agy, claude, codex, grok」（以你配置里写了的为准，要几秒到十几秒）；「已把 agent gpt-6.1-sol-xhigh 追加进 <配置文件>（原文件备份在 <配置文件>.bak-add-<日期>-<时间>）」；「gpt-6.1-sol-xhigh 已加入会话 <会话 id>」。**把备份路径记下来**。配置里若已经有同一个模型、同一档的成员（别的 id），打的是「配置里已经有同一个模型、同一强度的 agent <id>，直接 /add <id>」，什么都不写。你之前若已经用 `/add +编号 --effort xhigh` 把它写进了配置，这一步只打「gpt-6.1-sol-xhigh 已加入会话 <会话 id>」一行（配置里的全名，不识别、不写）；想看自动识别与写配置，就换一档没配过的，例如 `/add gpt-6.1-sol-max`。
  - 第 3 步：成员里有 gpt-6.1-sol-xhigh（codex、gpt-6.1-sol、xhigh）。
  - 第 4 步：第一条打「gpt-6.1-sol 对上的模型已经有成员在当前会话里：gpt-6.1-sol-xhigh；要另加一档强度，用 /add gpt-6.1-sol <强度> 或 --effort <强度>」，第二条一模一样，两条都不写配置。你的配置里若另有 gpt-6.1-sol 开头、不在场的成员，第一条可能按前缀加入它或者打「有歧义」，以屏幕为准。
  - 第 5 步：「gpt-6.1-sol-xhigh 已退出会话 <会话 id>」。
  - 第 7 步：`xigh` 那一条先打「正在识别模型: …」，再打「gpt-6.1-sol 支持的强度是 low|medium|high|xhigh|max|ultra，没有 xigh」，什么都不写（档的顺序以屏幕为准）。`xhigh` 那一条打追加与加入两行，不再有「正在识别模型」，因为快照已经拍过。
  - 不带强度的 `/add gpt-6.1-sol`（配置里还没有这个模型时）加的是 gpt-6.1-sol-high：council 的默认档是 high，不是 codex 自己的 medium。
- [ ] 符合
- [ ] 不符合

#### M8 一批小修补（CL1）：`/help` 两行、`/team use` 不随 team 加入总结成员

- 状态：已落地未复验（`5c4840a`；2026-10-01 已装 `council 0.2.0-1001b`，含它）
- 出处：00_DECISIONS.md #50；契约 `19h` §3（字面量）、§6 第 5 步（下面的步骤是它的精简版）。同一批里的 taskkill 改动与 Ctrl+Alt+C 用例没有零花费、看得见的真机步骤，以单元测试为准（19h §6 第 5 步之 5）。
- 前置：装了含 CL1 的版本。用一份**沙盒配置**，不碰你的真配置与日常库。**只敲斜杠命令**。
- 花费：零花费（不提问、不识别、不起任何 provider 进程）。

步骤：

第 1 步（PowerShell）：建沙盒并启动。这一整块一起粘贴、回车。

```
$sb = "$env:TEMP\cl1check"; New-Item -ItemType Directory -Force $sb | Out-Null; $cfg = @("data_dir = '$sb\data'", "", "[providers.claude]", "command = 'claude'", "", "[agents.cl1-a]", "provider = 'claude'", "model = 'sonnet'", "", "[agents.cl1-b]", "provider = 'claude'", "model = 'sonnet'", "", "[agents.cl1-sum]", "provider = 'claude'", "model = 'sonnet'", "", "[summary]", "agent = 'cl1-sum'", "", "[teams.cl1]", "members = ['cl1-a', 'cl1-b', 'cl1-sum']") -join "`n"; [System.IO.File]::WriteAllText("$sb\config.toml", $cfg + "`n", (New-Object System.Text.UTF8Encoding $false)); council -no-tui -config "$sb\config.toml"
```

第 2 步（council 里，一行一块）：

```
/new cl1check
```

```
/help
```

```
/team use cl1
```

```
/who
```

```
/quit
```

第 3 步（PowerShell）：删沙盒。

```
Remove-Item -Recurse -Force "$env:TEMP\cl1check"
```

- 预期：
  - 第 1 步：开头几行里有「已加载 3 个 agent，1 个 team」。配置里的 claude 只是为了过校验，下面几条命令都不起它。
  - `/help`：`/add` 那一行里有「识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号」；`/team` 那一行是「……并把尚未加入的成员（总结成员除外）一次性加进来: /team use <team-id>」。
  - `/team use cl1` 四行：「已绑定 team cl1」「cl1-a 已加入会话 <会话 id>」「cl1-b 已加入会话 <会话 id>」「cl1-sum 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结」。
  - `/who`：只有 cl1-a、cl1-b，没有 cl1-sum。
- [ ] 符合
- [ ] 不符合

### 3.6 规则文件（P）

#### P1 Council 自己的全局规则（`~/.council/COUNCIL.md`）与局部规则（rules_file）注入每个成员的指令环境

- 状态：主代理隔离验过（N 次付费）
- 出处：HANDOFF.md:939-948、:1020（Step 8 第一片三轮真机：GLOBAL-OK / PROJECT-OK 三家都出现；你的 CLAUDE.md 首行规则只在 grok 出现，印证「三家不对等」；claude 那轮 in=29537、$0.118394）；原件 `E:\Isolating_test\council\step8a-smoke\RESULTS.md`。N：仓库没记总次数。11 §1、11a §1、§3（:24-107、:150-340）；08b9e5f、c88ea73；config/rules.go:33-147；app.go:119-126。
- 前置：沙盒配置。TOML 顶层键必须写在所有表之前（schema.go:69-70）；rules_file 的相对路径按配置文件所在目录解析（rules.go:95-98）。
- 花费：花钱：<成员甲> 那家 × 1 次（首轮冷启动）。改规则会改身份指纹，别在日常库上做（HANDOFF.md:1033）。

步骤：

第 1 步（PowerShell）：在沙盒目录建局部规则文件，写一行规则（UTF-8 无 BOM）。

```
[System.IO.File]::WriteAllText('E:\Isolating_test\council\mvp\COUNCIL.md', "每次回答末尾单独一行写 PROJECT-OK`n", (New-Object System.Text.UTF8Encoding $false))
```

第 2 步：rules_file 只能重启生效（configcmd.go:128），先在已开着的沙盒 council 里退出（§0.4）；再用记事本在沙盒配置里第一个 `[表]` 之前加一行 `rules_file = "COUNCIL.md"`（原来有 rules_file 就改掉那一行），保存，再启动。

（council 里）

```
/quit
```

（PowerShell）

```
notepad E:\Isolating_test\council\mvp\config.toml
```

```
council --config E:\Isolating_test\council\mvp\config.toml
```

第 3 步

```
/rules
```

第 4 步

```
/new 清单-rules
```

```
/add <成员甲>
```

```
/ask <成员甲> 你好
```

- 预期：
  - 第 3 步：「局部规则: E:\Isolating_test\council\mvp\COUNCIL.md (<N> 字节)」（rules.go:48、:245-250），「合计 <N> 字节」，每家 provider 一行（见 P2）。
  - 第 4 步：回复末尾有单独一行 PROJECT-OK。
  - 文件存在却读不出来时不进 REPL，打「规则文件读不出来: …」（app.go:119-126）；文件不存在按没有规则处理。
- [ ] 符合
- [ ] 不符合

#### P2 `/rules` 显示两个规则文件的路径与字节数、每家 provider 的隔离与缓存、命令行余量，以及漂移计数

- 状态：已落地未复验
- 出处：你在 x39f 上真机看过的只有漂移计数「按当前配置 /ask 会被拒的 provider 会话: 2 条（不花钱）」那一行（HANDOFF.md:2332），这一行在 R6 第 3 步复验，R6 记真机通过；路径、字节数、各家 provider 行、「身份已变的 provider 会话」那一行（rules.go:95）都没有你的记录，所以整项记未复验。主代理 Step 8a 看过当时的 `/rules` 渲染（HANDOFF.md:1017-1018），之后 X-39f 订正（11a:461-462）与 codex 行（17b:958）都改过，对不上今天的屏幕。11a §5.1（:443-505）；11c §6；rules.go:43-105、:269-303。`/rules` 不受档位影响（16b:525）。
- 前置：无（日常库与沙盒都可以）。
- 花费：零花费。

步骤：

第 1 步

```
/rules
```

- 预期：
  - 「全局规则: <路径> (<N> 字节)」或「<路径> (不存在)」；「局部规则: <路径> (<N> 字节)」或「(未配置)」；「合计 <N> 字节」。
  - 然后每家一行「<provider>  <档>  <隔离>  <缓存>  命令行余量 <N> 字节」，组不出来时印「不适用（<原因>）」。隔离一格：claude 为「唯一指令源」，grok 与 codex 为「叠加在用户全局规则之上」，未知为「未验证，按叠加处理」；缓存一格：「进缓存前缀，首轮付」或「无缓存，按轮数乘着付」（rules.go:269-303；17b:958）。
  - 有漂移时多一行：「身份已变的 provider 会话: N 条（要重建才能接着用，各付一次入场费）」或「按当前配置 /ask 会被拒的 provider 会话: N 条（不花钱）」（rules.go:100）。
- [ ] 符合
- [ ] 不符合

#### P3 `/rules reload`：council 开着时改了规则文件，先预览哪些会话会因此漂移，不改任何状态

- 状态：主代理隔离验过（零花费）
- 出处：HANDOFF.md:1028-1030（第三轮用 System.Diagnostics.Process 重定向 stdin，在进程运行期间改文件，reload 不调任何 provider）；11a §5.2（:507-519，含 X-39f 订正）；c88ea73；rules.go:141-213。
- 前置：做完 P1（沙盒里有 COUNCIL.md，且 <成员甲> 开过口）。
- 花费：零花费。

步骤：

第 1 步（PowerShell，council 不关）：改沙盒 COUNCIL.md 的正文，保存。

```
notepad E:\Isolating_test\council\mvp\COUNCIL.md
```

第 2 步

```
/rules reload
```

- 预期：「全局规则: <旧> 字节 <摘要> -> <新> 字节 <摘要>」「局部规则: <旧> 字节 <摘要> -> <新> 字节 <摘要>」（rules.go:145-151）。有已开口的成员时打「会因此漂移的会话（1 条）:」和「  <会话 id> / <成员甲>: <原因>」，末行「要应用请敲 /rules reload --rebuild（会重建上列会话，各付一次入场费）」；没有会话时打「没有会话会因此漂移」和「要应用请敲 /rules reload --rebuild」（rules.go:182-213）。
- [ ] 符合
- [ ] 不符合

#### P4 `/rules reload --rebuild`：应用新规则，逐条丢掉会漂移的会话，下一轮按新规则重开

- 状态：已落地未复验
- 出处：HANDOFF.md:1032（「仍未真机验的只剩 /rules reload --rebuild 那一支（单测覆盖）」）；它与 `/session rebuild` 走同一个 Runner.Rebuild，那条已验过（HANDOFF.md:1034）。11a §5.2（:512-519）；c88ea73；rules.go:215-240；X-39f 措辞 00_DECISIONS.md:1757-1758。
- 前置：做完 P3。
- 花费：第 1 步零花费；第 3 步花钱：<成员甲> 那家 × 1 次（冷启动）。

步骤：

第 1 步

```
/rules reload --rebuild
```

第 2 步

```
/verbose on
```

第 3 步

```
/ask <成员甲> 你好
```

- 预期：
  - 第 1 步：「已重建 <会话 id>/<成员甲>（existed=true）」；配置里已没有的 agent 打「已清掉 <会话>/<成员>（existed=…；按当前配置 /ask 会被拒，不花钱）」（rules.go:233-238）。
  - 第 3 步：「新开了一条 provider 会话 <新 id>」，「冷暖 预报=冷」，回复按新规则走。
- [ ] 符合
- [ ] 不符合

#### P5 启动漂移提示的「身份已变」类：改了规则或配置后重启，横幅下说明哪些会话要重建（付费）

- 状态：已落地未复验
- 出处：「会被拒、不花钱」那一类是真机通过的，见 R6；「身份已变」这一类没找到横幅本身的真机记录：主代理 Step 8a 只验到 `/rules` 的漂移计数（HANDOFF.md:1024-1025），HANDOFF.md:1143-1146 记启动漂移审计「只验到 AuditDrift 这一层」。11a §6（:551-559，X-39f 订正）；app.go:198-225、:279-298；X-39f 7d65b96。
- 前置：做完 P1（<成员甲> 开过口，规则文件在沙盒里）。
- 花费：零花费。重建发生在下一次 `/ask`，冷启动另计。

步骤：

第 1 步（PowerShell，council 开着）：改沙盒 COUNCIL.md 的正文，保存。

```
notepad E:\Isolating_test\council\mvp\COUNCIL.md
```

第 2 步

```
/quit
```

第 3 步（PowerShell）：用同一条命令重新启动。

```
council --config E:\Isolating_test\council\mvp\config.toml
```

- 预期：第 3 步横幅之后：「注意: 有 1 条 provider 会话的身份已变，下次 /ask 会各重建一次（每条一次入场费）」，逐行明细「  会话 <id> / 成员 <成员甲>: <原因>」（app.go:289、:295）。没有漂移时这一段一个字都不打。
- [ ] 符合
- [ ] 不符合

### 3.7 权限与安全（Q）

#### Q1 read 档（默认）：成员能讨论，但写不了盘

- 状态：主代理隔离验过（N 次付费）
- 出处：claude、grok 的 read 档在 6530839 修过、逐条实测（HANDOFF.md:836-856；06:25-29）；claude 至少 3 次（06:166）；agy 不发 --mode，建 PROOF.txt 失败，三处目录都没有文件（HANDOFF.md:811-814）；codex 两次写盘探针全被拒（00_DECISIONS.md:2192）。N：grok、agy 的次数仓库没记。06 §2 表（:34-40）；11c §3；工作目录 turn/session.go:124-126。
- 前置：沙盒；成员 permissions 为 read（`/add +编号` 加进来的默认就是 read）。
- 花费：花钱：<成员甲> 那家 × 1 次（首轮冷启动）；四家各试一次就是 4 次。

步骤：

第 1 步

```
/new 清单-read
```

```
/add <成员甲>
```

第 2 步

```
/ask <成员甲> 在当前目录建一个 PROOF.txt 并写入 WRITTEN，报告成功还是被拒
```

第 3 步（PowerShell）

```
Test-Path E:\Isolating_test\council\mvp\data\workdir\<成员甲>\PROOF.txt
```

- 预期：
  - 第 2 步：模型说被拒或做不到（只是自述，不作数）。agy 可能正文为空，并打「provider 报了 1 个被拒的工具调用（可能不止这些）：write_file(WriteToFile)。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败」（ask.go:590-593）。
  - 第 3 步：False。read 档的工作目录不带会话 id（turn/session.go:124-126）。任何一次 True 立刻停。
  - 残留风险照实写：codex 的 read 档只试过 node_repl 这一个 MCP 工具（00_DECISIONS.md:2192）；agy 的只读「验过一次、未穷尽」（06:45-49）。
- [ ] 符合
- [ ] 不符合

#### Q2 workspace 档（claude 与 agy）：能在自己的工作目录里写，越界被拒；claude 只能编辑、不能执行

- 状态：主代理隔离验过（N 次付费）
- 出处：445b809 提交说明（11c §8.2 第 8 条 claude WORKSPACE：目录内写入成功、兄弟目录被拒；第 11 条 READ 既有会话漂移 0 条）；7ab093e 提交说明（第 9 条 agy：INSIDE.txt 建出来、OUTSIDE.txt 不存在，原件 `E:\Isolating_test\council\step8b-agy-rerun`）；README.md:57「真机验收 4/4」。N：付费次数仓库没记。06 §2、§3；11c §2-§8；工作目录 turn/session.go:124-129。
- 前置：沙盒配置里给一个 claude 或 agy 成员写 `permissions = "workspace"`。先 `/auto off`：对话模式下没点名直接打字也会带着写盘权限问到它（00_DECISIONS.md:2260）。
- 花费：花钱：<成员丁> 那家 × 1 次（claude 按美元计）。

步骤：

第 1 步

```
/new 清单-ws
```

```
/add <成员丁>
```

第 2 步

```
/ask <成员丁> 在当前目录建 INSIDE.txt 写入 OK；再在上一级目录建 OUTSIDE.txt 写入 LEAK；逐个报告成功还是被拒
```

第 3 步（PowerShell）

```
Test-Path E:\Isolating_test\council\mvp\data\workdir\<会话 id>\<成员丁>\INSIDE.txt
```

```
Test-Path E:\Isolating_test\council\mvp\data\workdir\<会话 id>\OUTSIDE.txt
```

- 预期：
  - 第 2 步：claude 报告界内成功、越界被拒；agy 可能正文为空，并打「provider 报了 N 个被拒的工具调用（可能不止这些）：…」（ask.go:590）；claude 不打这句，它的 permission_denials 没接进来（7ab093e）。
  - 第 3 步：第一条 True，第二条 False。workspace 档的目录带会话 id（turn/session.go:128）。判据是文件，不是模型自述（11c:201-203）。
  - workspace 档的 claude 没有 Bash 与 PowerShell，只能改文件、不能跑（06 §3）。
- [ ] 符合
- [ ] 不符合

#### Q3 grok 成员配成 workspace：起进程之前就拒，理由是「证明不了写入边界」

- 状态：主代理隔离验过（零花费）
- 出处：445b809 提交说明（11c §8.2 第 10 条：「Council 在起进程之前拒绝，文本是「证明不了边界」。通过」）；06 §2；11c §3.2；provider.go:141-143；rules.go:299-303。
- 前置：沙盒配置里给 grok 成员写 `permissions = "workspace"`（CPA 可以不在线，起进程之前就拒了），重启或 `/config reload --apply`。
- 花费：零花费。

步骤：

第 1 步

```
/rules
```

第 2 步

```
/new 档位拒绝
```

```
/add <grok 成员>
```

```
/ask <grok 成员> 你好
```

- 预期：
  - 第 1 步：grok workspace 那一行末格是「不适用（permission tier "workspace": no verified write boundary: permission tier not supported by this provider）」（rules.go:299-303）。
  - 第 2 步：先「正在问 <grok 成员> ...」与「<显示名>: 没有回复（拒绝）」，再原样一行 `permission tier "workspace": no verified write boundary: permission tier not supported by this provider`（provider.go:142），不起进程。
- [ ] 符合
- [ ] 不符合

#### Q4 任何成员配成 full 档，都在起进程之前拒，不把「全部绕过」的 flag 交给 CLI

- 状态：已落地未复验
- 出处：只有单测（11c §8.1 第 4 条；provider/build_test.go 的 FullRejectedForAllThree、cxx_codex_test.go:271），没找到真机拒绝记录。06 §2（:62-75，agy 的 full 映射实测把文件写到了 --add-dir 之外）；11c §3.3（:94-101）；turn/permission.go:20-35；ask.go:663-664；validate.go:155-157；落地 445b809。
- 前置：只在沙盒配置上改；总结成员必须是 read（validate.go:140-141），别改它。council 开着时改文件。
- 花费：零花费。

步骤：

第 1 步（PowerShell）：把沙盒配置里某个成员（<成员己>）的 permissions 改成 `full`，保存。

```
notepad E:\Isolating_test\council\mvp\config.toml
```

第 2 步

```
/config reload --apply
```

第 3 步

```
/rules
```

第 4 步

```
/add <成员己>
```

```
/ask <成员己> 你好
```

- 预期：
  - 第 2 步：配置校验通过（full 是合法取值，validate.go:155-157）。
  - 第 3 步：这个成员所在那一行末格是「不适用（当前权限档不放行: permission "full": this permission tier is not offered to the orchestrator）」（rules_report.go:145-148、runner.go:973）。
  - 第 4 步：先「正在问 <成员己> ...」，再「<显示名>: 没有回复（拒绝）」，然后「full 权限档不提供给编排器自动使用（06 第 2 节）；拼错或未知的档同样被拒绝」（ask.go:664），不起进程。
- [ ] 符合
- [ ] 不符合

#### Q5 规则文件太大、整条命令行会超过 Windows 上限时，起进程之前就拒，并点名最长的参数；`/rules` 能提前看到余量

- 状态：主代理隔离验过（零花费）
- 出处：00_DECISIONS.md:779-781（step8a-smoke 第四轮：40621 字节的规则文件下 grok 报「命令行 41287 字节，上限 32767；最长的参数是 --system-prompt-override 的值（40898 字节）」，wall_ms=0，一个进程都没起）；HANDOFF.md:1020；#15、#19；spawn.go:127-134；落地 08b9e5f、621bada。
- 前置：沙盒配置里有 grok 成员；另开一个 PowerShell 窗口，环境变量只在那个窗口里生效。第 2 步要在新窗口里启动沙盒，先在已开着的沙盒 council 里退出（§0.4）。
- 花费：零花费。改规则文件会改身份指纹，别在日常库上做。

步骤：

第 0 步（已开着的沙盒 council 里）

```
/quit
```

第 1 步（新 PowerShell 窗口）

```
[System.IO.File]::WriteAllText('E:\Isolating_test\council\mvp\big.md', ('x' * 40000), (New-Object System.Text.UTF8Encoding $false))
```

```
$env:COUNCIL_GLOBAL_RULES = 'E:\Isolating_test\council\mvp\big.md'
```

第 2 步（同一个窗口）

```
council --config E:\Isolating_test\council\mvp\config.toml
```

```
/rules
```

第 3 步

```
/new 超长探针
```

```
/add <grok 成员>
```

```
/ask <grok 成员> 你好
```

第 4 步：退出后关掉这个窗口（环境变量随窗口消失）。

```
/quit
```

- 预期：
  - 第 2 步：「全局规则: 」那一行指向 big.md；grok 那一行末格是「命令行余量 <负数> 字节」（rules_report.go:187）；claude 那一行不受影响（身份走文件、正文走 stdin，#16）。
  - 第 3 步：先「<显示名>: 没有回复（拒绝）」，再一行以「命令行 」开头的错误，形如「命令行 <N> 字节，上限 32767；最长的参数是 --system-prompt-override 的值（<M> 字节）: assembled command line exceeds the OS limit」（spawn.go:131-133），不起进程。
  - agy 成员撞的是另一道 prompt 超长闸，文本不同；这道闸偏松，卡在边上几十字节的越界挡不住（#15 两条残余）。
- [ ] 符合
- [ ] 不符合

### 3.8 记账与崩溃恢复（K）

#### K1 `/cost` 按成员汇总当前会话的账；`/cost <成员>` 只看一个；`/cost --round <round-id>` 只看一轮；没报的数印 `?`

- 状态：已落地未复验
- 出处：13 §7.1 及 X-38bd 订正；落地 e18f02b、X-35a e695ede、X-38bd 772ce1d；cost.go:246-302（表头 :260，:248、:302）。RB-A 真机第 2 步（15a:618）要拿 `/cost` 对 round_id 末 6 位，但 `/cost` 的表不印 round_id（cost.go:260），那次不能算 `/cost` 的证据。
- 前置：第 1-3 步要当前会话跑过 `/ask` 或 `/round`（花钱轮之后做）；第 4 步在新会话里做。
- 花费：零花费。

步骤：

第 1 步

```
/cost
```

第 2 步：在 `/log` 里找「一轮开始 round_id=…」那一行，抄完整 id（TUI 表头只显示末 6 位，`/cost --round` 不收，00_DECISIONS.md:1980）。

```
/log 10
```

```
/cost --round <完整 round_id>
```

第 3 步

```
/cost <成员甲>
```

```
/cost a b
```

```
/cost --round x
```

第 4 步

```
/new 空账
```

```
/cost
```

- 预期：
  - 第 1 步：表头「AGENT  TURNS  SPOKE  LOST  REFUSED  IN  CACHE_READ  CACHE_CREATE  OUT  COST_USD  COST_UNKNOWN  COLD  USAGE_UNKNOWN  COLD_UNKNOWN」（cost.go:260），每个成员一行；某一列这个成员一个数都没报时印 `?`，不印 0。只有 grok、agy 说过话的那行 CACHE_CREATE 是 `?`（00_DECISIONS.md:1778、:1784）；codex 成员的 token 列与 COST_USD 应当全是 `?`（00_DECISIONS.md:2191）。被拒的轮只进 REFUSED，不进 COST_UNKNOWN。
  - 第 2 步：只统计那一轮。
  - 第 3 步：`/cost <成员甲>` 只剩这一行；`/cost a b` 打「用法: /cost [--round <round-id> | <agent-id>]」；`/cost --round x` 打「这一轮（round_id=x）没有记账行」（cost.go:248）。库里有 round_id 为空的行时另打「未计入本轮：另有 N 行 round_id 为空，不知道属于哪一轮…；不带 --round 的 /cost 能看到它们」（cost.go:302）。
  - 第 4 步：先「已建会话 <id>，标题: 空账」，再「本会话还没有记过账」。
- [ ] 符合
- [ ] 不符合

#### K2 一轮被取消或超时后，`/session` 里这个成员的 BASELINE 变 suspect，下一轮金额报「未计」；下一轮成功就回到 ok

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2098（你 2026-09-27 只看到过 ok）；suspect 由用例担保（turn 包的 G5，C2：8c00ec9、758371a）；13 §2、§3.7a、§3.11 及 2026-09-27 订正；13a §1.1 E2、F3，§6 第 6 步第 2 项；字面量 ask.go:405、:417、:519。
- 前置：沙盒；TUI（忙时按 Ctrl-C 只取消这一轮；行式下 Ctrl-C 会连 REPL 一起退，main.go:81-95）；一个会说话的成员；第二个 PowerShell 窗口（第 2、4 步查子进程）。
- 花费：花钱：<成员甲> 那家 × 3 次（第 3 步被取消时进程已经起了，钱可能已花）。

步骤：

第 1 步

```
/new 可疑基线
```

```
/add <成员甲>
```

```
/ask <成员甲> 你好
```

第 2 步（另一个 PowerShell）：记下现有的 CLI 进程（与 T11 第 1 步同一条命令）。

```
Get-Process claude,grok,agy,codex -ErrorAction SilentlyContinue | Select-Object Name,Id,StartTime
```

第 3 步：敲下面这条，回复出来之前按一次 Ctrl-C。

```
/ask <成员甲> 写一篇 1500 字的短文
```

第 4 步（另一个 PowerShell）：表头回到「空闲」之后等 10 秒，再跑一次第 2 步那条命令。

```
Get-Process claude,grok,agy,codex -ErrorAction SilentlyContinue | Select-Object Name,Id,StartTime
```

第 5 步

```
/session
```

第 6 步

```
/verbose on
```

```
/ask <成员甲> 你好
```

第 7 步

```
/session
```

- 预期：
  - 第 3 步：表头先变「正在取消」；然后「<显示名>: 没有回复（取消）」和「provider 那边可能已经吃下这批内容」（ask.go:405、:417）。
  - 第 4 步：没有第 3 步之后新起、还活着的那家 CLI 进程。取消时 Council 先 killProcessTree 再等进程退出（spawn.go:190-197）。第 2 步里本来就有的进程（Claude Code 等）不算；进程叫什么名字仓库里只见过 grok.exe（HANDOFF.md:2597），对不上名字时按 StartTime 找第 3 步之后起的。有、而且一直不退，请记下来：这就是 15 §6 的 P1 探针要抓的情况，孤儿进程接着烧钱，没有任何记录对得上（15:178-188）。
  - 第 5 步：<成员甲> 的 BASELINE 是 suspect。
  - 第 6 步：这一轮 outcome spoke，金额一行是「金额未计（上一轮结局未知，基线可疑）」（ask.go:519；简洁档不打这一行）。
  - 第 7 步：BASELINE 回到 ok。
  - 变体（不必做）：第 5 步之后 `/quit` 再启动、`/use` 回来，`/session` 仍是 suspect（13a G5 第 3 步）。
  - 库里留痕：取消不留在飞记录（runner.go:1030-1053），会照常留一行 canceled 的 turn_usage 与一条 turn_failed 面包屑。
- [ ] 符合
- [ ] 不符合

#### K3 成员回复途中 council 被强杀：重启后横幅报丢账，基线标可疑，`/log` 留一条丢账记录，下一轮金额报「未计」，游标没被偷推

- 状态：已落地未复验
- 出处：你 2026-09-27 在 0.2.0-s10 上试过一次，没造出崩溃（opus 约 4 秒就回来，00_DECISIONS.md:2098）；端到端靠 C2 用例担保（internal/app/s10c2_crash_e2e_test.go，750c2d4、758371a）。13 §3.3、§3.3a、§3.5-§3.7、§3.11a、§3.15 第 14 条（探针 2a）；13a §6 第 6 步第 3 项；落地 204e615、90bad1c、92db175、9e85bea、8c00ec9；字面量 app.go:317、recover.go:103、cost.go:248、:302、ask.go:519。13 §7.3 的诚实声明：「崩溃之后接受一次可见的重复投递，绝不接受一次静默的丢失。」
- 前置：沙盒（崩溃会在库里永久留痕）；一个会说话的成员，建议 grok（窗口宽、会预铸 id）；第二个 PowerShell 窗口。做法照 13 §3.15 第 14 条（13:981-983）：先成功跑一轮，第二轮子进程还在跑时用 `taskkill /F` 单独杀 council、不加 `/T`，并用会慢慢回的长请求。09-27 你照 HANDOFF.md:2584 关窗那次没造出来，是因为 opus 约 4 秒就回来，关窗时那一轮已经记账（00_DECISIONS.md:2098；HANDOFF.md:2603），不是关窗这个办法本身不行。
- 花费：花钱：<成员甲> 那家约 3 次（被杀那一轮的子进程照样跑完、照样计费）。换 claude 时一次入场费约 0.4-1.0 美元（00_DECISIONS.md:988-989）。

步骤：

第 1 步：先在已开着的沙盒 council 里退出（§0.4），再在 PowerShell 里行式、详细档启动。

（council 里）

```
/quit
```

（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui -verbose
```

```
/new 崩溃2a
```

```
/add <成员甲>
```

```
/ask <成员甲> 你好
```

第 2 步：敲下面这条；看到「正在问 <成员甲> ...」之后、回复出来之前，在另一个窗口取最新的 council 进程号并强杀。只杀 StartTime 最晚的那个；日常库的 council 若也开着、而且比沙盒启动得晚，就先把它关掉再做，别杀错。

```
/ask <成员甲> 写一篇 1500 字的短文
```

```
Get-Process council | Sort-Object StartTime | Select-Object Id,StartTime
```

```
taskkill /F /PID <最新那个 Id>
```

第 3 步：等被杀那一轮的 CLI 子进程自己结束（可用 `Get-Process grok,claude,agy,codex -ErrorAction SilentlyContinue` 看），再在原窗口用同一条命令重启。

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui -verbose
```

第 4 步

```
/use <会话 id 的前 6 位>
```

```
/session
```

```
/log 5
```

```
/cost
```

```
/cost --round 0
```

第 5 步

```
/ask <成员甲> 你好
```

```
/session
```

- 预期：
  - 第 1 步：启动输出里没有「在飞」「没记上账」字样；首轮打「outcome spoke  agent <成员甲>」和「新开了一条 provider 会话 <sid>」。
  - 第 2 步：council 退回提示符；成员的 CLI 子进程不会被连带杀掉。回复已经出来才杀到的，库里没有丢账，换更长的请求重做。
  - 第 3 步：横幅首行逐字「注意: 有 1 轮在上次退出时没记上账，其中 1 轮已把相关成员的成本基线标为可疑」（app.go:317）；下一行形如「  会话 <会话 id> / 成员 <成员甲> / turn <id> / 起于 <时刻> / phase spawned: 起过进程，结局未知；provider 会话 <sid> 可能已经多了一轮」（明细不冻结，原因子句取自 recover.go:103）。不应有以「启动在飞对账」开头的行。
  - 第 4 步：`/session` 里 <成员甲> 的 BASELINE 是 suspect；`/log` 最后一行是 system 消息「成员 <成员甲> 的一轮在上次退出时没记上账：起过进程，结局未知；provider 会话 <sid> 可能已经多了一轮 [event=turn_lost]」；`/cost` 里 LOST 是 1；`/cost --round 0` 先打「这一轮（round_id=0）没有记账行」，再打「未计入本轮：另有 1 行 round_id 为空，不知道属于哪一轮（其中 1 行是崩溃后启动对账补的账）；不带 --round 的 /cost 能看到它们」（cost.go:248、:302）。
  - 第 5 步：不被拒，正常起进程；「provider 会话 <sid>」还是崩溃前那个 id；金额一行「金额未计（上一轮结局未知，基线可疑）」（ask.go:519）。成员可能把崩溃那一轮的问题再答一遍：游标没推，重投看得见（13 §7.3）。之后 `/session` 回到 ok。
  - 库里留痕：永久留下一行 turn_usage（outcome 9，token 与金额全是 NULL）与一条 turn_lost 面包屑。
- [ ] 符合
- [ ] 不符合

#### K4 成员的第一轮（bootstrap）就被强杀：重启后照样补一条丢账；grok 点得出孤儿会话 id，claude、agy 说点不出名字；下一轮重新开会话

- 状态：已落地未复验
- 出处：13 §3.15 第 15、16 条真机没人跑过；grok bootstrap 形状由 C2 用例担保（750c2d4）。13 §3.6、§3.6.1、§3.7、§3.11a（探针 2b）；落地 9e85bea；recover.go:101-109；ask.go:209。
- 前置：同 K3；新会话里一个还没开过口的成员 <成员乙>（grok 验孤儿 id，claude 验「点不出名字」）。
- 花费：花钱：<成员乙> 那家约 2 次，两次都是首轮冷启动。

步骤：

第 1 步：先在已开着的沙盒 council 里退出（§0.4；紧接着 K3 做时，K3 那个进程也要先 `/quit`），再在 PowerShell 里启动。

（council 里）

```
/quit
```

（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui -verbose
```

```
/new 崩溃2b
```

```
/add <成员乙>
```

第 2 步：敲下面这条，回复出来之前在另一个窗口强杀最新的 council（同 K3 第 2 步的两条命令）。

```
/ask <成员乙> 写一篇 1500 字的短文
```

第 3 步：等子进程自己结束，再用同一条命令重启。

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui -verbose
```

第 4 步

```
/use <会话 id 的前 6 位>
```

```
/session
```

```
/log 5
```

```
/cost
```

第 5 步

```
/ask <成员乙> 你好
```

- 预期：
  - 第 3 步：横幅首行逐字「注意: 有 1 轮在上次退出时没记上账，其中 0 轮已把相关成员的成本基线标为可疑」（bootstrap 轮没有 provider_sessions 那一行可标，13 §3.11a）。原因子句：grok 是「起过进程，结局未知；provider 侧可能有一条孤儿会话 <预铸 id>」，claude 与 agy 是「起过进程，结局未知；这条 provider 会话点不出名字」（recover.go:106、:108）。
  - 第 4 步：`/session` 里 <成员乙> 形如「<成员乙>  -  -  -  fresh」，不是 suspect（ask.go:209）；`/log` 最后一行是「成员 <成员乙> 的一轮在上次退出时没记上账：<同上原因> [event=turn_lost]」；`/cost` 里 LOST 是 1，COST_UNKNOWN 至少 1。
  - 第 5 步：不被拒；「新开了一条 provider 会话 <新 id>」，新 id 与孤儿 id 不同（13 §3.7：绝不拿预铸 id 去 resume）。
- [ ] 符合
- [ ] 不符合

#### K5 启动对账补账失败时不拦 REPL，但受影响成员的 `/ask` 被拒，直到某次对账成功

- 状态：已落地未复验
- 出处：真机造不出来（要让启动对账本身失败，13a §6 第 6 步第 4 项，13a:505）；由用例 TestS10aSettleFailureBlocksAskUntilSettled 在真 app.Run 与真 SQLite 上担保（internal/app/s10a_settle_fail_blocks_ask_test.go，8c00ec9）；13 §3.8；13a §2.4 E1；00_DECISIONS #43(e)(f)；app.go:207。
- 前置：无。
- 花费：零花费。
- 步骤：真机不做。按 13a:505，定稿的 MVP 判据表里这一条标 `[用例]TestS10aSettleFailureBlocksAskUntilSettled`，不标真机。
- 预期（真遇到时）：stderr 有以「启动在飞对账补账失败: 」开头的行（app.go:207）；横幅照样打「注意: 有 N 轮…其中 0 轮…」，stdout 不提失败（#43(e) 未修，00_DECISIONS.md:2095）；对这个成员 `/ask` 打 A 之五那句拒绝；`/session rebuild` 走同一条路，同样会失败（18b §7.1 第 2 条），只能手改库。
- 真机不做（不勾选）。

#### K6 配置 `store_thought = false` 时，thought 被丢掉的那条发言在 `/log` 末尾打 ` [thought_dropped]`

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2098（「` [thought_dropped]` 要改配置，真机没测」）；用例 E1a-E1c 担保（13b §2.1）；13 §7.2；13b §3.3；落地 a2e3d76。
- 前置：沙盒配置顶层（data_dir 那一行下面）加一行 `store_thought = false`；有 grok 成员，CPA 可达。只有 grok 会给 thought（13b §0 第 3 条）。
- 花费：花钱：grok × 2 次（第 1 次是首轮冷启动）。

步骤：

第 1 步：先在已开着的沙盒 council 里退出（§0.4）；再用记事本改好配置，启动。

（council 里）

```
/quit
```

（PowerShell）

```
notepad E:\Isolating_test\council\mvp\config.toml
```

```
council --config E:\Isolating_test\council\mvp\config.toml
```

第 2 步

```
/new 思考丢弃
```

```
/add <grok 成员>
```

```
/ask <grok 成员> 你好
```

第 3 步

```
/log
```

第 4 步：`/quit`，用记事本把那一行改成 `store_thought = true`，保存，用同一条命令重启，回到这个会话再问一次。

```
/quit
```

（PowerShell）

```
notepad E:\Isolating_test\council\mvp\config.toml
```

```
council --config E:\Isolating_test\council\mvp\config.toml
```

（council 里）

```
/use <会话 id 的前 6 位>
```

```
/ask <grok 成员> 再说一句
```

```
/log
```

- 预期：
  - 第 3 步：grok 发言那一行 TEXT 末尾是「 [thought_dropped]」，形如 13b §3.3 的 G3_OFF：「3  <grok 成员>  agent  -  grok/<model>/<effort>  <正文> [thought_dropped]」；seq 2 的面包屑带「 [event=session_opening]」。正文为空时 TEXT 是「- [thought_dropped]」。
  - 第 4 步：新的那条发言不带 [thought_dropped]，第 3 步那一行照旧带。
  - grok 这一轮本来就没给 thought 时两次都不会打，那是巧合，不算通过（13 §7.2）。
- [ ] 符合
- [ ] 不符合

#### K7 成员有工具调用被拒时，屏幕上当场说一句，`/log` 里那条发言末尾打 ` [denied_actions]`，重启后还看得到

- 状态：已落地未复验
- 出处：`/log` 标记本身没有真机记录，由用例 E2a、L1、L2 担保（13b §2）；被拒工具调用那句提示与 meta 落库，主代理 2026-09-21 真机验过（7ab093e 提交说明），当时 `/log` 还没有标记。13 §7.2；13b §2.1、§3.3；11c §7；落地 a2e3d76、7ab093e；ask.go:590-594。
- 前置：沙盒配置里 agy 成员 `permissions = "workspace"`；先 `/auto off`。
- 花费：花钱：agy × 1 次（首轮冷启动）。

步骤：

第 1 步

```
/new 越界探针
```

```
/add <agy 成员>
```

第 2 步

```
/ask <agy 成员> 在你当前工作目录的上一级目录新建 OUTSIDE.txt，内容写 X
```

第 3 步

```
/log 3
```

第 4 步（PowerShell）

```
Test-Path E:\Isolating_test\council\mvp\data\workdir\<会话 id>\OUTSIDE.txt
```

- 预期：
  - 第 2 步：「provider 报了 1 个被拒的工具调用（可能不止这些）：write_file(WriteToFile)。界内的事可能已经做完，只是这一轮的正文被清空了，不代表整轮失败」（ask.go:590-594；动作名以 agy 实际报的为准）；正文为空时简洁档在成员头下面打「本轮为空」。
  - 第 3 步：agy 那一行 TEXT 是「- [denied_actions]」（13b §3.3 的 A3_DENIED）。
  - 第 4 步：False（判据是文件系统，不是正文为空）。
  - agy 这一轮根本没去写时没有 denied_actions，这一步不成立，换个说法重试。
- [ ] 符合
- [ ] 不符合

#### K8 `/ask` 的用量行里 provider 没报的格子印 `?`，与库里的 NULL 一一对应

- 状态：已落地未复验
- 出处：00_DECISIONS #38(a)（:1782-1785）；0a41890；ask.go:498。
- 前置：会话里有 grok 或 agy 成员；`/verbose on`。
- 花费：花钱：grok（或 agy）× 1 次。

步骤：

第 1 步

```
/verbose on
```

第 2 步

```
/ask <grok 成员> 你好
```

- 预期：第 2 步用量一行「用量 in=… cache_create=? cache_read=… out=… thinking=…」（grok 每个成功轮的 cache_create 在库里是 NULL，00_DECISIONS.md:1778；agy 成员同样是 `cache_create=?`，:1784）。
- [ ] 符合
- [ ] 不符合

#### K9 失败轮的显示：预算表不把失败轮的 token 缩成 0（X-35b）；冷暖与用量报「未知」（X-36ab）；结算带「k 名成员用量未知」（X-38e）

- 状态：已落地未复验
- 出处：X-35b 4f01dc7（00_DECISIONS.md:1655-1661，由 x35b_sample_test.go 担保）；X-36ab 09bf271（:1677-1681；ask.go:498-540）；X-38e 9729e94（:1815-1821；契约建议的真机 `/round` 没做，HANDOFF.md:2294）。
- 前置：要造失败轮就得改沙盒配置（把一个 grok 成员的 model 改成不存在的名字，做法见 HANDOFF.md:2294），默认不做，你点头再做；`/verbose on`（简洁档不打这些行）。这个成员在当前会话开过口的话，改 model 会先以身份漂移被拒、不起进程，要先 `/session rebuild <grok 成员>`，或换一个新会话。
- 花费：花钱：可能 1-2 次（失败轮计不计费看 provider，没验过）。

步骤：

第 1 步（PowerShell）：把沙盒配置里 <grok 成员> 的 model 改成一个不存在的名字，保存；council 里 `/config reload --apply`。

```
notepad E:\Isolating_test\council\mvp\config.toml
```

```
/config reload --apply
```

第 2 步

```
/verbose on
```

```
/ask <grok 成员> 你好
```

第 3 步

```
/round <grok 成员> <成员乙> --go --no-summary
```

第 4 步

```
/round <grok 成员> <成员乙>
```

- 预期：
  - 第 2 步：「冷暖 预报=… 实测=未知 …」；没解析到用量时打「用量 未知（没解析到用量）」（ask.go:498-540）。
  - 第 3 步：结算合计带「（1 名成员用量未知，合计只含其余人）」（00_DECISIONS.md:1817）。
  - 第 4 步：预算表照常出来、不起进程；<grok 成员> 那行的 token 预报不是 0，金额格照旧「未标定」（00_DECISIONS.md:1660）。
- [ ] 符合
- [ ] 不符合

### 3.9 TUI 外壳、运行看板与输入（T）

#### T1 敲 `council` 进 TUI：启动信息立刻出现在输入框上方；活区是表头、横线、输入框；内联模式，不占全屏

- 状态：真机通过（Griffin，0.2.0-x39b）
- 出处：00_DECISIONS.md:1884-1885（「TUI 启动信息立刻出现、在输入框上方」）；HANDOFF.md:2250（16 步全部符合）；15 §4 #1、§8 第 1 刀、§12、§16；ecea495、c414363、e069063、89b0c7c；启动死锁修 da88d20。
- 前置：沙盒配置。照 §2.1 做时，第 1 步就是 §0.2 第 4 步那一次启动，不要再起一个（同一个 data_dir 只能开一个，§0.4）。
- 花费：零花费。

步骤：

第 1 步（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml
```

第 2 步：看活区。第 3 步：往上滚动窗口，选中一段启动信息。

- 预期：
  - 第 1 步：启动信息（横幅，app.go:265-271）立刻出现，不空白、不卡。
  - 第 2 步：自上而下是表头「council 0.2.0-0929a   空闲」（版本号后三个空格）、一条横线「─」、输入框。
  - 第 3 步：启动信息在终端自己的回滚区里，能滚动、能选中（内联模式，不是全屏）。
- [ ] 符合
- [ ] 不符合

#### T2 输入框编辑：←/→ 移光标，退格删一个字，单行时 ↑ 不调历史，回车提交并回显

- 状态：真机通过（Griffin，0.2.0-x39b）
- 出处：00_DECISIONS.md:1885（「←/→ 移光标、退格一次删一个字、↑ 在单行输入框里无反应」）；e069063；model.go:171-178、:416-424。
- 前置：无。
- 花费：零花费。

步骤：

第 1 步：在输入框敲 `abc`，按 ← 两次、→ 一次。第 2 步：按退格一次。第 3 步：按 ↑。第 4 步：用退格把输入框里的字删光（TUI 没有一键清空，空闲时按 Ctrl-C 会退出，17a:571 (d)；x39b 的真机记录里没有「删光」这个动作，00_DECISIONS.md:1885 记的是退格一次删一个字），敲下面这条回车。

```
/help
```

- 预期：
  - 第 1 步：光标跟着左右移。
  - 第 2 步：删掉光标左边一个字。
  - 第 3 步：单行时 ↑ 没反应。
  - 第 4 步：回滚区先出回显「> /help」，再出命令表；输入框清空，表头回到「council 0.2.0-0929a   空闲」。
- [ ] 符合
- [ ] 不符合

#### T3 空闲时 Ctrl-J 在输入框里换行，回车才把整段作为一次提交

- 状态：已落地未复验
- 出处：e069063；model.go:172-173（Enter 解绑 InsertNewline）、:411-414。「跑的时候打字、按 Ctrl-J」在 RB-B 第 3 项里真机过了（00_DECISIONS.md:2011；15b:653，见 D4），但空闲时多行一次提交没有单独的真机记录。
- 前置：已 `/use` 一个会话；先 `/auto off`（不关的话第 2 步会问全部成员再总结，花钱）。
- 花费：零花费。

步骤：

第 1 步

```
/auto off
```

第 2 步：敲 `第一行`，按 Ctrl-J，敲 `第二行`，回车。

第 3 步

```
/log 3
```

- 预期：
  - 第 1 步：「已关闭自动回复：直接打字只记下，不问任何人；要成员说话用 /ask 或 /round；/auto on 打开」。
  - 第 2 步：Ctrl-J 后输入框变两行；回车后回滚区两行回显「> 第一行」「> 第二行」；简洁档不打「已记下 seq N」。
  - 第 3 步：最后一条消息的 TEXT 是一行「第一行 第二行」：`/log` 把换行显示成空格（commands.go:1257、:1372-1384），库里存的仍是两行。
- [ ] 符合
- [ ] 不符合

#### T4 空闲时按 Ctrl-C 或敲 `/quit` 退出，退出后屏幕上不留最后一帧

- 状态：真机通过（Griffin，0.2.0-x39f）
- 出处：00_DECISIONS.md:1893（空闲 Ctrl-C 与 `/quit` 退出后提示符上方不留表头、横线、`┃`，没有乱码）；HANDOFF.md:2331；X-39e 4edced9；run.go 的 exitClearSeq。
- 前置：无。
- 花费：零花费。

步骤：

第 1 步：在 TUI 里空闲时按一次 Ctrl-C。

第 2 步（PowerShell）：再进 TUI。

```
council --config E:\Isolating_test\council\mvp\config.toml
```

第 3 步

```
/quit
```

- 预期：第 1、3 步：表头先变「正在退出」，程序退出；提示符上方不留表头、横线、`┃` 输入框行，没有 `[7A` 之类乱码，终端回显正常。已知代价：关窗、被杀的路不擦，残影照旧（00_DECISIONS.md:1898）。
- [ ] 符合
- [ ] 不符合

#### T5 `/ask` 跑的时候表头每秒计时；这时按回车不提交，表头说明原因

- 状态：真机通过（Griffin，0.2.0-rba）
- 出处：00_DECISIONS.md:1972（15a §6 第 1、4 步，15a:617、:620）；69fed6c；model.go:416-420。记录早于 CO1。
- 前置：当前会话里有 <成员甲>。
- 花费：花钱：<成员甲> 那家 × 1 次。

步骤：

第 1 步

```
/ask <成员甲> 用一句话介绍你自己
```

第 2 步：它回来之前在输入框敲 `x`，回车。第 3 步：等它回来。

- 预期：
  - 第 1 步：回滚区「> /ask …」与「正在问 <成员甲> ...」；表头「council 0.2.0-0929a   正在运行  0s」，每秒加 1；`/ask` 不出成员行。
  - 第 2 步：表头变「council 0.2.0-0929a   一轮正在跑，先等它回来或按 Ctrl-C 取消  Ns」，秒数照走；`x` 留在输入框里，没提交。
  - 第 3 步：回复上屏（简洁档「<显示名>（x.xs）: …」），表头回到「council 0.2.0-0929a   空闲」。
- [ ] 符合
- [ ] 不符合

#### T6 `/round` 跑的时候表头显示总览：发言轮 k/N、round_id 末 6 位、回来 x/y、已用时间

- 状态：真机通过（Griffin，0.2.0-rba）
- 出处：00_DECISIONS.md:1972（15a §6 第 2、3 步，15a:618-619）；15a §3.1（15a:370-377）；69fed6c。记录早于 CO1。
- 前置：会话里有两个讨论成员。可以和 D2 第 1 步一起看。
- 花费：花钱：甲、乙各 1 次。

步骤：

第 1 步：敲下面这条，看第一个人回来那一刻的表头。

```
/round <成员甲> <成员乙> --go --no-summary
```

- 预期：刚开始表头「council 0.2.0-0929a   正在运行  发言轮  回来 0/2  Ns」（只有一轮时不印 1/1）；第一个人回来时变「… 发言轮 (xxxxxx)  回来 1/2  Ns」，括号里是 round_id 末 6 位。多轮与裁判轮依次切换「发言轮 1/2」「发言轮 2/2」「裁判轮」，见 D5、D6。
- [ ] 符合
- [ ] 不符合

#### T7 窗口太窄时表头被裁、不折行；太矮时不出成员行、只剩表头总览；结束照样打收尾行、不留残影

- 状态：真机通过（Griffin，0.2.0-s10）
- 出处：矮：00_DECISIONS.md:2011（RB-B 第 5 项：12 行左右降级、中途拉矮都不留残影）；窄：00_DECISIONS.md:1972（RB-A 第 5 步，15a:621）。15b §1.1 D9；572467c；model.go:527-533。
- 前置：会话里有两个讨论成员。
- 花费：花钱：每跑一次甲、乙各 1 次（两步共 4 次）。

步骤：

第 1 步：把窗口拉窄到 40 列左右。

```
/round <成员甲> <成员乙> --go --no-summary
```

第 2 步：拉回宽度，把窗口高度拉到 10 行。

```
/round <成员甲> <成员乙> --go --no-summary
```

- 预期：
  - 第 1 步：表头右侧被裁掉、不折行、活区不变高；之后空闲 Ctrl-C 退出，提示符上方不留残影（15a:621）。窄窗口下已用时间可能看不见（00_DECISIONS.md:1981）。
  - 第 2 步：不出成员行，只有表头「… 发言轮  回来 x/2  Ns」；照样打「/round 结束  Ns  回来 2/2」，不留残影。两人时看板上限 = 终端高 − 9，10 行放不下（model.go:532）。中途拉矮的已知代价见 15b §6 第 5 项与 00_DECISIONS.md:2014。
- [ ] 符合
- [ ] 不符合

#### T8 成员失败时看板显示状态词与标记

- 状态：已落地未复验
- 出处：572467c；15b §3.3（15b:406-422）；00_DECISIONS.md:1974。真机只见过「在跑」「完成」与取消路径（00_DECISIONS.md:2011）。
- 前置：无。
- 花费：零花费。
- 步骤：真机难造，默认不做；字面量见 15b §3.3。
- 预期（真遇到时对照）：状态词：未入库、报错、认不出、超时、没起来（时间格 `?`）、拒绝（`-`）、取消、丢失、未知；标记按顺序：可能已吃下、正文见上方、(前缀已断)。
- 默认不做（不勾选）。

#### T9 有看板时命令出错退出不留帧；`/round` 收尾后自定义光标形状会被复位（已知代价）

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2011（RB-B 第 6 项手动造不出来，由 VT 守卫 G4 担保；第 7 项没测）、:2017、:2221；15b §1.1 F.5、§6 第 6、7 项（15b:656-657）；19b §7 (a)。
- 前置：会话里有两个讨论成员。第 6 项（出错退出）真机造不出来，不做。
- 花费：花钱：甲、乙各 1 次。

步骤：

第 1 步（PowerShell）：先在已开着的沙盒 council 里 `/quit`（§0.4）；在 PowerShell 里把光标换成闪烁下划线，再进 TUI。

```
[Console]::Write([string][char]27 + '[3 q')
```

```
council --config E:\Isolating_test\council\mvp\config.toml
```

第 2 步

```
/use <会话 id 的前 6 位>
```

```
/round <成员甲> <成员乙> --go --no-summary
```

- 预期：第 2 步结束后光标形状被复位成终端默认，不再是第 1 步设的下划线（`ESC[0 q`，真终端没测，00_DECISIONS.md:2017）。请记下实际看到什么。@ 列表开着、收起时同理，见 T14。
- [ ] 符合
- [ ] 不符合

#### T10 输入框里打中文、希腊字母与模糊宽度符号时，光标位置对得上

- 状态：已落地未复验
- 出处：15:325（「真终端里的中文光标定位也没验证过」）；conhost 加中文代码页下可能漂是推断（15:319-324，未实测）；15 §13；e069063。
- 前置：无。
- 花费：零花费。

步骤：

第 1 步：在 Windows Terminal 的 TUI 输入框敲 `中文α×…∂`（不回车）。第 2 步：按 ← 三次，敲 `X`。第 3 步（可选）：直接开 powershell.exe 窗口（conhost，不经 Windows Terminal）启动 council，重复第 1-2 步。

- 预期：第 2 步光标停在要插入的那一格，`X` 插在光标处，字不错位。第 3 步 conhost 下光标每过一个 α、×、… 就右漂一格的话，请记下来（15 §13 推断的失效模式）。
- [ ] 符合
- [ ] 不符合

#### T11 TUI 跑着时关掉窗口，在飞的 provider CLI 跟着停，不跑满 5 分钟硬超时

- 状态：已落地未复验
- 出处：89b0c7c 接上 Options.Ctx；d4e3d76 只在管道上取消根 ctx 验过（00_DECISIONS.md:1877-1880）；TUI 人工验收单第 5 条没有回报（HANDOFF.md:1868、:2301）；15 §6（P1）、§14.3；run.go:37-48；main.go:66。另有一条推断没核实：main.go:66 只登记了 os.Interrupt，TUI 又关了 bubbletea 自己的信号处理（run.go:48），关窗时根 ctx 可能不会取消，只能靠这一项看。
- 前置：当前会话里有 <成员甲>；另一个 PowerShell 窗口；先分清第 1 步列出的进程里哪些是 Claude Code 等别的程序。
- 花费：花钱：<成员甲> 那家 × 1 次（被腰斩也照付）。

步骤：

第 1 步（另一个 PowerShell）：记下现有进程。

```
Get-Process claude,grok,agy,codex -ErrorAction SilentlyContinue | Select-Object Name,Id,StartTime
```

第 2 步（council TUI）

```
/ask <成员甲> 从 1 数到 300，每行一个数
```

第 3 步：它回来之前直接关掉这个 Windows Terminal 标签页。第 4 步：10 秒后在另一个窗口再跑一次第 1 步的命令。

- 预期：第 4 步没有第 2 步之后新起、还在跑的那家 CLI 进程（HANDOFF.md:1868 ⑤）。如果有、而且一直不退，请记下来：这笔钱对不上任何记录。重开 council 时可能出现丢账横幅（见 K3）。
- [ ] 符合
- [ ] 不符合

#### T12 TUI 里把一条斜杠命令粘贴进输入框：不自动提交，回车才执行，照常执行

- 状态：真机通过（Griffin，0.2.0-x39b）
- 出处：00_DECISIONS.md:1885（粘贴 `/new 复验TUI` 回车执行）。13 §5（B 片）；00_DECISIONS #12 订正、#39(b)(c)；e069063。本项只收你真机看过的这一半；粘贴时表头「粘贴了 N 个字符」（model.go:442）没有你的真机记录，归 T17 看（19c:439 第 1 项），不在本项里。
- 前置：经剪贴板，可以做（§0.4）。
- 花费：零花费。

步骤：

第 1 步：在记事本写 `/help`，全选复制。第 2 步：在 TUI 输入框按 Ctrl-V（或右键）。第 3 步：回车。

- 预期：第 2 步 `/help` 留在输入框里，没有自动提交。第 3 步回显「> /help」，命令表照常出来。
- [ ] 符合
- [ ] 不符合

#### T13 多行提交、第 2 行起有 `/` 开头的行时不提交，表头提示逐条回车；第 1 行只写 `/say` 时整段当一条消息发（X44）

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2100、:2110、:2114（e85d1bc，「真机没有复验」）；17a §1.1、§3.1、§6 第 5 步（17a:520 起，:538、:540）；model.go:685、:698-701；四条待定已由主代理定（00_DECISIONS.md:2182）。
- 前置：有一个会话。
- 花费：零花费（`/say` 只记下，不触发自动回复）。

步骤：

第 1 步

```
/use <会话 id 的前 6 位>
```

第 2 步：在输入框敲 `/new 真机复验 x44`，按 Ctrl-J，再敲 `/add <成员甲>`，回车。

第 3 步：敲一个字母再退格，按 Esc，再回车。

第 4 步：按 Ctrl-Home 回到开头，敲 `/say`，按 Ctrl-J，回车。

第 5 步

```
/log 3
```

- 预期：
  - 第 2 步：回滚区什么都不多；输入框两行原样还在；表头「council 0.2.0-0929a   一次只能一条命令，逐条回车；整段当消息发，首行只写 /say」（17a:538）。
  - 第 3 步：提示不消失，仍不提交。
  - 第 4 步：回显三行「> /say」「> /new 真机复验 x44」「> /add <成员甲>」；没建新会话，没加成员；简洁档不打「已记下 seq N」（17a:540）。
  - 第 5 步：最后一条消息的 TEXT 是一行「/new 真机复验 x44 /add <成员甲>」：`/log` 把换行显示成空格（commands.go:1257、:1372-1384），库里存的仍是那两行。缩进行被挡、长文照常、忙时、行式不变见 17a §6 第 5 步第 5、7、8、9 项；第 6 项整块粘贴经剪贴板，可以做（§0.4）。
- [ ] 符合
- [ ] 不符合

#### T14 输入框敲 `@` 弹出当前会话成员列表：上下键选，Tab 或回车补全，Esc 或 Ctrl-C 收起（AT1）

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2204-2224（4202b6c，「真机未复验」）；HANDOFF.md:2719；19b §1.1、§3.1、§6 第 5 步（19b:968-982）；model.go:56（最多 5 行）、:554-682。
- 前置：会话里有两个以上活跃成员；候选列表要在本次启动后第一次提交之后才有（00_DECISIONS.md:2214），第 1 步的 `/use` 就是这次提交。
- 花费：零花费（先 `/auto off`；不关的话第 3 步会问被 @ 的那个成员，花一次）。

步骤：

第 1 步

```
/use <会话 id 的前 6 位>
```

```
/auto off
```

第 2 步：敲一个 `@`（不回车）。

第 3 步：按 ↓ 两次、↑ 一次，回车；再敲 `你好`，回车。

第 4 步：敲 `@` 加某个 id 的前两个字母，再敲一个不存在的字母，退格，按 Esc；再敲一个空格和 `@`，按 Ctrl-C。

- 预期：
  - 第 2 步：输入框下方出现成员列表，选中行前面是「> 」，每行「@<id>  <显示名>」；总结成员不在里面；闪烁竖线光标停在 `@` 后面；成员多于 5 个时只显示放得下的行，末行「  共 N 个，上下键翻看」。
  - 第 3 步：「> 」跟着移动；回车后输入框变「@<那个 id> 」（后面带一个空格），列表消失，没有提交；再回车才提交（auto off 下只记下）。
  - 第 4 步：列表只剩以前缀开头的成员；敲不存在的字母列表消失，退格后回来；Esc 收起，字不变；Ctrl-C 只收起列表，程序不退。
  - 句末点名要回车两次、残影检查、忙时不弹见 19b §6 第 5 步第 4、7、9 项。
- [ ] 符合
- [ ] 不符合

#### T15 `@` 列表在窗口变矮、拉回时按放得下的行数显示；收起后回滚区不留残影

- 状态：已落地未复验
- 出处：00_DECISIONS.md:2213（真终端缩放没测，只在模拟器里量过帧高）；19b §0 第 3 条、§6 第 5 步第 7-8 项（19b:980）；4202b6c。
- 前置：会话里至少一个活跃成员。
- 花费：零花费。

步骤：

第 1 步

```
/use <会话 id 的前 6 位>
```

第 2 步：把窗口拉到十来行高，敲 `@`。第 3 步：再拉到 9 行以下，然后拉回原高度。第 4 步：按 Esc，往上滚动回滚区。

- 预期：
  - 第 2 步：只显示放得下的几行（要高到 15 行左右才出现末行「共 N 个，上下键翻看」）。
  - 第 3 步：9 行以下列表不出现；拉回后列表回来。
  - 第 4 步：回滚区没有多出来的「council …」表头行、横线或 `@…` 列表行。有的话，不开列表同样拉一次作对照（19b:980）。
- [ ] 符合
- [ ] 不符合

#### T16 列表开着时，带修饰键的回车、Tab 只补全不提交；带修饰键的 ↑↓、Esc 照常（AT1-FU）

- 状态：已落地未复验
- 出处：19c:3（ee247c6，「真机未复验」）；19c §0 R1、R5，§6 第 5 步第 3-4 项（19c:438-442）；00_DECISIONS.md:2226-2228。Windows Terminal 下这些键送进来是什么形状没测（19c:453）。
- 前置：会话里至少一个活跃成员；`/auto off`。
- 花费：零花费（列表不在时按 Shift+Enter 会提交一句，自动回复开着就会花钱）。

步骤：

第 1 步

```
/use <会话 id 的前 6 位>
```

```
/auto off
```

第 2 步：敲 `@`，按 Shift+Enter。

第 3 步：用退格把输入框里的字逐个删光（没有一键清空：列表收起之后 Esc 什么都不做，空闲时按 Ctrl-C 会直接退出程序，17a:571 (d)），敲 `/help`，按 Shift+Enter。

第 4 步：敲 `@`，按 Shift+↓，再按 Shift+Esc。

- 预期：
  - 第 2 步：只补全成「@<id> 」，不提交。
  - 第 3 步：列表不在时 Shift+Enter 和回车一样，执行 `/help`。
  - 第 4 步：Shift+↓ 让选中项下移一格（Windows Terminal 常占着 Shift+↓，看不出变化不算坏）；Shift+Esc 只收起列表。Ctrl+Shift+C 不在真机步骤里（19c:442）。
- [ ] 符合
- [ ] 不符合

#### T17 Ctrl-V 不再由输入框自己读剪贴板，粘贴只走终端的括号粘贴（AT1-FU）

- 状态：已落地未复验
- 出处：19c:3（ee247c6）；19c §0 R0、§6 第 5 步第 1-2 项；00_DECISIONS.md:2227；model.go:174-178。
- 前置：经剪贴板，可以做（§0.4）。
- 花费：零花费。
- 步骤：要做时照 19c §6 第 5 步第 1、2 项（19c:439-440）。
- 预期：第 1 项 Windows Terminal 下照常粘进来，表头「粘贴了 N 个字符」；第 2 项列表开着时粘贴，列表收起、字粘进来。粘不进来或列表没收起，说明终端把 Ctrl-V 原样交给了程序（这一刀之后它什么都不做）。
- [ ] 符合
- [ ] 不符合

### 3.10 行式模式 `-no-tui`（L）

#### L1 `council -no-tui` 走行式：方向键、退格、控制台历史正常；含控制字符的行不记、不执行

- 状态：真机通过（Griffin，0.2.0-x39b）
- 出处：00_DECISIONS.md:1886-1887（退格一个字一个字删、← 不再打出 `^[[D`、↑ 调出控制台历史、Ctrl+A 回车打出冻结提示、不记录）；主代理另用管道喂入验过（HANDOFF.md:2214）；89b0c7c、80143b8；commands.go:103。
- 前置：知道一个会话 id；已开着的沙盒 council 先 `/quit`（§0.4）。
- 花费：零花费（用 `-no-auto`）。

步骤：

第 1 步（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui -no-auto
```

第 2 步

```
/use <会话 id 的前 6 位>
```

第 3 步：敲 `abc`，按 ← 两次、退格一次；再按 ↑；最后按 Esc 清掉这一行（别回车）。

第 4 步：按 Ctrl+A，再回车。

第 5 步

```
/log 3
```

- 预期：
  - 第 1 步：不画 TUI，出行式提示。
  - 第 3 步：光标左移，退格一次删一个字，不出现 `^[[D`；↑ 调出控制台历史（刚才的 `/use …`）。
  - 第 4 步：「这一行含有控制字符，没有记录，也没有当成命令执行」（00_DECISIONS.md:1863；commands.go:103）。
  - 第 5 步：没有第 4 步那一行。
- [ ] 符合
- [ ] 不符合

#### L2 行式粘贴一行命令直接执行；多行粘贴逐行变成多条

- 状态：真机通过（Griffin，0.2.0-x39b）
- 出处：00_DECISIONS.md:1886（粘贴 `/new 复验一行` 直接执行，不再有 `^[[200~`）；13 §4b、§5；03dcef4、10124a9；80143b8（X-39b 删掉 EnablePaste）；00_DECISIONS.md:609-613（#12 订正）。
- 前置：经剪贴板，可以做（§0.4）。已开着的沙盒 council 先 `/quit`（§0.4）。
- 花费：零花费。

步骤：

第 1 步：在记事本写 `/help`，全选复制。

第 2 步（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui -no-auto
```

第 3 步：右键粘贴，回车。

- 预期：第 3 步直接执行 `/help`，行里没有 `^[[200~`。多行粘贴会变成多条，`/` 开头的行照样执行（00_DECISIONS.md:611-612）；要整块记成一条用 `/say` … `/end`（L3）。
- [ ] 符合
- [ ] 不符合

#### L3 行式多行输入：`/say` 进入多行，`/end` 整段作为一条消息发出，草稿里的斜杠行不执行；`/cancel` 放弃草稿；空草稿 `/end` 不发

- 状态：已落地未复验
- 出处：486f5f3（Step 5 多行输入）；08；commands.go:71-99、:126；两个出口的分支 commands.go:301-326（空草稿 :311-314，放弃 :318-320）。没找到真机记录。
- 前置：用 `-no-auto` 启动（不加的话 `/end` 发送与直接打字相同，会问全员，commands.go:126、:315-316；19a:1701），并已 `/use` 一个会话。接着 L1 那个行式进程做即可。
- 花费：零花费。

步骤：

第 1 步

```
/say
```

第 2 步

```
第一行
```

第 3 步

```
/log 3
```

第 4 步

```
/end
```

第 5 步

```
/log 3
```

第 6 步：再进一次多行，写一行，然后放弃。

```
/say
```

```
这一行会被放弃
```

```
/cancel
```

第 7 步：再进一次多行，什么都不写就发送。

```
/say
```

```
/end
```

第 8 步

```
/log 3
```

- 预期：
  - 第 1 步：「进入多行输入：整段粘贴或逐行敲；单独一行 /end 发送，单独一行 /cancel 放弃」（commands.go:86）。
  - 第 3 步：草稿里的 `/log 3` 只是正文，不执行。
  - 第 4 步：发出一条消息（简洁档不打 seq）。
  - 第 5 步：最后一条消息的 TEXT 是一行「第一行 /log 3」：`/log` 把换行显示成空格（commands.go:1257、:1372-1384），库里存的仍是两行。
  - 第 6 步：`/cancel` 之后打「已放弃这份草稿，未发送」（commands.go:90），回到单行模式。
  - 第 7 步：`/end` 之后打「草稿是空的，没有发送」（commands.go:88）。
  - 第 8 步：最后一条消息仍是第 4 步发的那条；「这一行会被放弃」不在里面。
- [ ] 符合
- [ ] 不符合

#### L4 行式下按一次 Ctrl-C 优雅退出，连按两次硬退（退出码 130）

- 状态：已落地未复验
- 出处：main.go:22-28、:81-95；89b0c7c；TUI 人工验收单没有回报（HANDOFF.md:1868）；round.go:65-67 的实测是改 TUI 前的串行循环。
- 前置：已开着的沙盒 council 先 `/quit`（§0.4；接着 L3 做时就是 L1 那个行式进程）。
- 花费：零花费。

步骤：

第 1 步（PowerShell）

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

第 2 步：空闲时按一次 Ctrl-C。

第 3 步（PowerShell）：再启动，快速连按两次 Ctrl-C。

```
council --config E:\Isolating_test\council\mvp\config.toml -no-tui
```

第 4 步

```
$LASTEXITCODE
```

- 预期：第 2 步程序退出，终端正常。第 4 步：来得及按第二次时退出码是 130（main.go:28、:93）；只按到一次时是 0。退出码 1 见 A2，2 见 A4。注意：行式下一轮进行中按 Ctrl-C 会腰斩当前轮并退出 council（round.go:65-67），和 TUI 不同。
- [ ] 符合
- [ ] 不符合

#### L5 用脚本喂 stdin 时不画 TUI，走行式；没有看板，也没有收尾行

- 状态：真机通过（Griffin，0.2.0-rba）
- 出处：00_DECISIONS.md:1972（15a §6 第 6 步：非终端路径 `/round` 输出逐行一致，15a:622）。RB-B 那一项（没有块、没有收尾行）没有回报（00_DECISIONS.md:2011；15b:658）。15 §11；89b0c7c；main.go:69-70；简洁档总用时 16b:855。记录早于 CO1。
- 前置：用 cmd 重定向，不用 PowerShell 管道（PS 5.1 管道会往 stdin 首行插 BOM）。已开着的沙盒 council 先 `/quit`（§0.4）。
- 花费：花钱：<成员甲> × 1 次。

步骤：

第 1 步（PowerShell）：写脚本（UTF-8 无 BOM）。

```
[System.IO.File]::WriteAllText('E:\Isolating_test\council\mvp\s.txt', "/use <会话 id 的前 6 位>`n/round <成员甲> --go`n", (New-Object System.Text.UTF8Encoding $false))
```

第 2 步（PowerShell）

```
cmd /c "council --config E:\Isolating_test\council\mvp\config.toml < E:\Isolating_test\council\mvp\s.txt"
```

- 预期：不画表头与输入框；`/round` 照常跑完；没有段终态块，也没有「/round 结束 …」收尾行；只问一个成员，不总结；简洁档末尾一行「总用时 x.xs」（16b:855）。
- [ ] 符合
- [ ] 不符合

#### L6 stdout 重定向到文件时不画 TUI

- 状态：已落地未复验
- 出处：main.go:69-70（stdin、stdout 两个句柄都判）；89b0c7c；TUI 人工验收单第 4 条没有回报（HANDOFF.md:1868）。
- 前置：已开着的沙盒 council 先 `/quit`（§0.4）。
- 花费：零花费。

步骤：

第 1 步（PowerShell）

```
cmd /c "council --config E:\Isolating_test\council\mvp\config.toml > E:\Isolating_test\council\mvp\out.txt"
```

第 2 步：直接敲下面这条回车（屏幕上可能看不到提示）。

```
/quit
```

第 3 步（PowerShell）

```
notepad E:\Isolating_test\council\mvp\out.txt
```

- 预期：第 1 步屏幕上不画 TUI。第 3 步 out.txt 里是行式输出（横幅等），没有表头、横线，也没有 ESC 控制串。
- [ ] 符合
- [ ] 不符合

### 3.11 未做与暂缓（U）

这一组没有步骤，不勾选；列出来是为了让 MVP 的边界看得见。

#### U1 Agent 选择器（可视化挑成员）

- 状态：未做
- 出处：15:140（§4 #3「选择器 UI 未做」）；15 §8 第 3 刀。行式部分已落地：2923bf2（`/add` 前缀）、eb1e464（X-1）。
- 前置、花费、步骤、预期：无。

#### U2 Judge 选择器（15 §8 的第 4 刀）

- 状态：未做
- 出处：15:141（§4 #4）；「改配置还是改本会话」待裁（15 §5 第 2 条；HANDOFF.md:1869 记主代理先按「改本会话」走）；15:238。后端已有 `--with-judge`（D6）。另：你的规格 §18 把 Ctrl+J 定为 judge 选择器（docs/reference/griffin-tui-spec.md:249、:601），而 Council 现在 Ctrl-J 是换行（model.go:172），做的时候要另选键。
- 前置、花费、步骤、预期：无。

#### U3 协作模式选择器（sequential、review、debate 等；15 §8 的第 4 刀）

- 状态：未做
- 出处：15:142（§4 #5）；15 §3.4 四种模式推后；15:238。
- 前置、花费、步骤、预期：无。

#### U4 命令面板（15 §8 的第 4 刀）

- 状态：未做
- 出处：15:143（§4 #6）；15:238；规格 §17。
- 前置、花费、步骤、预期：无。

#### U5 Compare view（几个成员的回复并排对比；15 §8 的第 5 刀）

- 状态：未做
- 出处：15:144（§4 #7）；规格 §8。
- 前置、花费、步骤、预期：无。

#### U6 展开、折叠单个成员的结果

- 状态：未做
- 出处：15:145（§4 #8）；RB-A、RB-B 都写明不做（15b:17）。
- 前置、花费、步骤、预期：无。

#### U7 配色

- 状态：未做
- 出处：规格 §20 要少量语义色（docs/reference/griffin-tui-spec.md:633-661），没有任何裁决或契约。
- 前置、花费、步骤、预期：无。

#### U8 看板字形与三档标记（● ◐ × 等；00_DECISIONS #41 的第 4 刀）

- 状态：未做
- 出处：00_DECISIONS.md:1928、:1959（要等你在 conhost 与 Windows Terminal 上各试一次 ● ◐ ×）；HANDOFF.md:2431、:2509。与 15 §8 的第 4 刀（U2-U4）不是一回事。
- 前置、花费、步骤、预期：无。

#### U9 `/task`（8c-4，产出补丁草案）

- 状态：暂缓
- 出处：00_DECISIONS.md:1241（#24「2026-09-29 Griffin：『/task 暂时不要了』，暂缓」）、:1389（#29 随 #24 一起暂缓）；72c7e56；README.md:57。
- 前置、花费、步骤、预期：无。

## 4. 已知问题与待定（不影响 MVP 判定的）

验证覆盖面：
- 你的真机记录都早于 CO1、SM1、CH1（x39b 00_DECISIONS.md:1884；x39f HANDOFF.md:2329；rba 00_DECISIONS.md:1972；s10 00_DECISIONS.md:2011、:2098）；0929a 上还没有任何真机结果（HANDOFF.md:2703）。
- conhost 从没复验过，15a:616、15b:650 都要求另跑一遍（HANDOFF.md:2378）。
- rba、s10 两次付费复验的调用次数与金额，记录里都没写；本清单不替它们填数。
- 两个 model 字符串 `claude-fable-5-1`（总结成员）与 `claude-opus-5-5`（opus-5.5）CLI 认不认，未验证，第一次问它们才知道（00_DECISIONS.md:2153、:2160）。
- P1 探针（raw 模式下 Ctrl-C 会不会广播到子进程组）从没跑过（HANDOFF.md:1865-1867、:2301）。K2 第 2、4 步与 T11 从进程层面看得到一部分：Ctrl-C 或关窗之后那家 CLI 进程停没停；看不到退出码（15:180-184）。
- 关窗那条路：main.go:66 只登记了 os.Interrupt，run.go:48 关了 bubbletea 的信号处理，而 main.go:78-79 的注释说 SIGTERM 会走 NotifyContext；关窗时根 ctx 会不会取消是推断，未实测，靠 T11 看。
- `-race` 只在云端 Linux 跑过一次，不能读成「Windows 二进制没有竞态」（00_DECISIONS.md:2072-2076）。

契约之间的写法冲突（本清单已取定）：
- 配了总结成员、两人一轮时，TUI 收尾行是「回来 3/3」（smx_summary_tui_test.go:76、:128；16c:1159）；19a:1604 写的「回来 2/2」只对没配总结成员的装置成立（chx_program_test.go:19）。
- 16b §6 第 5 步写于 SM1 之前，照原样敲会多一次总结与一次 claude 调用；本清单一律加 `--no-summary`。
- `--with-judge` 按 00_DECISIONS.md:1972 与 15a:619 记真机通过，但 19a:1691 记你 09-28 的配置里没有 judge 成员，当时谁当的裁判没有记。

对话模式与 @（待你定，都先取了保守读法）：
- 没点名时 worker、manager 模式与 workspace 权限的成员也会被问，而且带写盘权限（19a:17、:1730；00_DECISIONS.md:2260）。
- 只有 judge 的会话每句打「这一轮没有人可问」；judge 行与「没配总结成员」提示每句都打（19a:1729、:1731）。
- `@` 左边紧贴字母数字时照问全员，例如 `hi@grok-4.7`（19a:1732）。
- `@params`、`@all` 这类解析得出名字的谁都不问，`@所有人` 却问全员（19a:1739；00_DECISIONS.md:2264）。
- `@a@b` 紧贴时第二个静默漏问（19a:16）。
- 只剩总结成员没加入时 `/add` 的两句提示是假话（19a:14）；`/team use` 会把总结成员加进来（00_DECISIONS.md:2257）。
- AT1 观感：双光标 (a)、忙时不弹 (b)、裁判也列出 (c)、句末点名要回车两次 (g)（19b:991-997）。

恢复与账：
- A 之五那句拒绝只提重启、不提 `/session rebuild`（18b:694，待你定）。
- 启动对账持续失败时 `/session rebuild` 也解不开；结清总会连带丢掉 provider 会话（00_DECISIONS.md:2090）。
- #43 (b)-(f) 没修：取消留下 begun 行、预算表对被锁成员印「不在飞」、NEXT 列不报「会被拒」、对账失败只在 stderr、同进程双重故障锁到重启（00_DECISIONS.md:2091-2096）。
- #37(a)：对配置里已删、仍是成员的 agent，`/ask` 先打「正在问」再被拒，没花钱（00_DECISIONS.md:1715-1717）。
- 13 §7.3 要的 `docs/14_MVP_CHECKLIST.md` 还不存在（README.md:59）；它要求每条只挂 `[用例]`、`[真机]`、`[人跑一次]` 三种标记之一，「金额『不知道』不被当成 0」要落在第 8 条（或回头改 13 §3.6、§7.1 两处引用），并必须带诚实声明（13:1345-1381）；另要一张「MVP+（Step 9 在范围内时）」：一个 team 三个成员各说一轮、Judge 出一次结论、全程成本可核，按 12 的验收指过去（13:1383-1384；本草案 §1 末尾只列了能拼出它的 C6、D6、K1）。本草案的格式不同，定稿成 docs/14 时要换算。

界面：
- RB-A 剩余 (a)-(c)：忙时粘贴盖掉「正在运行」；表头 6 位 id 不能直接给 `/cost --round`；窄窗口下已用时间可能看不见（00_DECISIONS.md:1979-1981）。
- RB-B 已知代价：中途拉矮窗口留表头片段；收尾窗口里被挡的 Enter 是静默的；被杀的路上段块与收尾行会丢；`ESC[0 q` 复位光标形状；(h) 未修（00_DECISIONS.md:2014-2021）。
- X-39e：关窗、被杀的路不清屏（00_DECISIONS.md:1898）。
- 行式多行粘贴逐行变成多条，`/` 开头的行照样执行（00_DECISIONS.md:611-612）。

codex：
- 失败轮的 usage_raw 不落库；续会话悄悄开新 thread 时 codex / agy 成员会永久丢身份；用量是 thread 累计，Council 不显示（00_DECISIONS.md:2173、:2191）。
- MCP 风险你已接受；全局 `~/.codex/AGENTS.md` 隔离不掉（00_DECISIONS.md:2169、:2178）。
- 06 §2 的权限表还没有 codex 列（06:56 仍写「codex 尚无适配器，整列删掉」），17b:1105 要求的订正没做。

配置：
- `/add +编号` 的备份只增不删（16a:2515 (n)）。
- OB1 落地前，默认位置没有配置时只打模板、不建文件（app.go:339-347）；模板里是你私人的示例成员与路径（00_DECISIONS.md:2273），OB1 会去掉。

## 5. 回报格式

每项每步一行，与 #41、#43 当时的回报同一形状：

- `<编号> 第 N 步 符合：看到了什么`
- `<编号> 第 N 步 不符合：看到了什么`

例子：

- `R1 第 3 步 符合：先打 正在问 grok-4.7 ...，然后 <显示名>（3.2s）: PONG`
- `T13 第 2 步 不符合：回滚区多了一行 > /new 真机复验 x44，建了新会话`

- 一项整体都符合时可以只写「<编号> 全部符合」。
- 不符合时尽量把屏幕上那几行原样抄下来（手敲或截图），会话 id 这类长串以库为准。
- 按晚了、造不出来的步骤写「<编号> 第 N 步 作废：原因」；没做的写「<编号> 没做」。
- 花钱的项顺带报一句实际问了几次；有 `/cost` 的话把那一行抄上。

## 附：审查意见的处置

审查意见的列表里缺项 13 条（审查员的 notes 写 12 条，这里按列表数）、错处 8 条、卡点 12 条，共 33 条。逐条回原文核过，全部成立，已照改。处置方式与审查员的建议不同的，各写了一句理由。

缺项：
1. `/providers`：成立，照改。新增 A5（与 `/agents` 合为一项，见第 13 条）。
2. 行式 `/cancel` 与空 `/end`：成立，照改。并进 L3 第 6-8 步（commands.go:88、:90、:311-320）；没另开编号，它们与 L3 是同一个多行模式、同一个状态。
3. TUI 单独 `/say`：成立，照改。并进 C7 第 4 步（seam.go:11）；C9 预期补一句：`/say` 行的后半句只在行式下成立。
4. 位置参数退出码 2：成立，照改。新增 A4。
5. `/auto x`：成立，照改。D11 第 6 步（autoreply.go:11）。
6. `-verbose` 真的生效：成立，照改。新增 D15 第 1、2 步。与契约不同：不照 16b:1111-1125 用 `/round` 实跑（要花钱），改看 `/verbose` 切换的回话（零花费）；理由写在 D15 花费行。
7. `-no-auto` 真的生效：成立，照改。D15 第 3 步。与契约不同：19a:1678 在有成员的会话里打字，flag 失效时要花钱；这里改在没有成员的会话里打，失效时只多打一行、不花钱，再用 `/auto` 切换的回话佐证。
8. 总结「只给你看」与三条预期：成立，照改。D7 第 6 步（`/cost` 里总结成员一行）、第 7 步（可选：重进后问成员甲）；D8 第 4 步（详细档的总结段）、第 5 步（行式下总结在「总用时」之前）。
9. `/leave` 总结成员与 `/join` 恢复：成立，照改。新增 D16。
10. 19a 之 7 的两种写法与之 9 的行式复验：成立，照改。D12 第 5、6 步；D12 第 9 步（行式，零花费）；D10 第 6 步（行式，可选，乙 1 次）。
11. Ctrl-C 之后子进程停没停：成立，照改。K2 第 2、4 步查进程；D5 预期指到 K2；§4 的 P1 探针那一条补一句。
12. MVP+ 表：成立，照改。§1 末尾新增「MVP+」一节，§4 的 docs/14 那一条补上。没写新步骤：13 要它按 12 的验收指过去，这里只列能拼出它的 C6、D6、K1。
13. `/agents`：成立，照改。与 `/providers` 合为 A5；A1 第 5 步、M3 第 5 步补表头。A5 整项记「已落地未复验」：`/agents` 有主代理的隔离记录（00_DECISIONS.md:2160），`/providers` 没有；一项只有一个状态，取弱的那个。

错处：
1. A1、R5、R8 的状态：成立，照改。三项都改成「主代理隔离验过（N 次付费）」，出处注明 grok 经 CPA 与看得到的次数。R8 没写成 1 次：提交说明里只看得到 `/ask gk2` 那一次，没说总共几次，出处写「至少 1 次」。§0.6 补一句「起过推理就算付费」。
2. M3 第 4 步的占位：成立，照改。改成 <provider>，引 addmodel.go:348、16a:1932、:1935。
3. M2「快照按会话」：成立，照改。M1、M2、M3 的前置都改成按进程（repl.go:118-119；commands.go:886-891）。
4. K3 的归因：成立，照改。改成 opus 约 4 秒就回来（00_DECISIONS.md:2098；HANDOFF.md:2603）；做法引 13:981-983。
5. `/log` 的多行正文：成立，照改。L3 第 5 步、T3 第 3 步、T13 第 5 步都改成一行（commands.go:1257、:1372-1384），并注明库里存的仍是多行。
6. 横幅漏一行：成立，照改。§0.2 第 4 步补「输入 /help 查看可用命令，/quit 退出。」与空行（app.go:271-272）；A2 第 4 步的引文改成 app.go:265-272。
7. T12、P2：成立，处置不同。没给状态加限定：状态词只许五个。T12 收窄到你真机看过的那一半（粘贴后回车才执行），表头字数那一半归 T17（19c:439 第 1 项）。P2 改成「已落地未复验」：你看过的那一行漂移计数在 R6 第 3 步复验，R6 记真机通过，所以没再拆新项。
8. §2.3 与 D10 的花费：成立，照改。§2.3、D7、D10 改成按「会话 + 成员」算冷暖（turn/session.go:23-31），注明这是按会话键推的，实际金额受跨会话缓存影响（HANDOFF.md:692），没核。

卡点：
1. 撞锁：成立，照改。§0.4 加一条「同一个 data_dir 只能开一个」；§2.1 第 7 组先做 T4；T1、A2、Q5、K3、K4、P1、K6、T9、L1、L2、L4、L5、L6 补上先 `/quit`。
2. D11 没建会话：成立，照改。D11 第 2 步先 `/new`，整项改在没有成员的会话里做。
3. §2.2 衔接不上：成立，照改。§2.2 第 4 步补 `/verbose off` 与 `/add <成员乙>`，第 6 步补 `/quit` 与 TUI 重启；另补 R3 重启后的 `/auto off`、R8 之前的 `/verbose off`。
4. M2 做不出来：成立，照改。M2 改用 `/add --refresh` 再按 Ctrl-C；M1 第 1 步就是 C5 第 1、2 步那一次 `/add`。
5. R7：成立，处置不同。没移出必须项，改成步骤里自带配置模板与播种脚本全文（18b:633-648），占位全换成实路径；一次性目录放在 mvp 目录里，随沙盒一起删；§2.1、§2.3 里「可选、要主代理备料」删掉。
6. A2 可能杀错进程：成立，照改。第 5 步按 StartTime 排序，加「日常库先关掉」的提醒，末尾补 `/quit`。
7. §0.3 漏了 D6、M4：成立，照改。
8. R6 会删掉 grok-4.7：成立，照改。<成员戊> 改成照 A1 抄一段 grok-4.7 的一次性成员（例如 `grok-tmp`）。
9. M3 挑行：成立，照改。第 1 步要求 EFFORTS 列含 max；第 3 步补挑错时的两种回话（addmodel.go:353-364），不点名哪一行有 max。
10. K6 第 4 步缺块：成立，照改。补记事本与启动两块。
11. T16 清空输入框：成立，照改。写明用退格逐字删（17a:571 (d)）。同类的 T2 第 4 步一并改了。
12. §0.2 全局规则的说法：成立，照改。补上环境变量 COUNCIL_GLOBAL_RULES（config/rules.go:12-14）。

顺手改的（审查没提）：§0.2 三处 rules.go 的引文其实在 internal/config/rules.go，照 §0.6 的写法改成 config/rules.go。

未采纳：无。
