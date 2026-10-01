# Step 8 第一片 冻结契约：规则文件与 argv 总长闸

配套 `11_STEP8_SPEC.md` 第 1 节、第 2 节。**第 3 节（WORKSPACE）不在本片**，另出契约。

本文件是实现方与测试方**共同的唯一权威**。测试方不看实现，只照这份写用例；
实现方不看测试，只照这份写代码。两边对不上时以本文件为准，改本文件要留记录。

## 0. 本片改动的包

**订正（2026-09-21，实现之后）**：本表当初漏了两行，补在下面并标出来。
一个包改动表漏行的代价是实打实的：测试方照它判断「哪些是契约面」，
漏掉的那些就没有用例守着。

| 包 | 改什么 |
|---|---|
| `internal/config` | 加 `rules_file` 键、`Rules` 类型、`LoadRules`、`ResolveGlobalRulesPath` |
| `internal/store` | schema v3（只改数据，不改列） |
| `internal/conversation` | `Store.AllProviderSessions`（**接口**）、`Service.AllProviderSessions`（**漏了，补**）、`Service.DropProviderSession`、`Tx.DropProviderSession` 改签名报 `existed` |
| `internal/provider` | **漏了，补**：`Capabilities` 加 `CacheAcrossTurns`（三家实测，见 §5.1 的订正） |
| `internal/turn` | `RulesBlocks`、`renderIdentity` 加参、指纹六段、`RulesDigest`、`Runner.SetRules` / `Rebuild` / `AuditDrift`、命令行总长闸 |
| `internal/repl` | `/rules`、`/rules reload [--rebuild]`、`/session rebuild <agentID>` |
| `internal/app` | 读规则、传进 `NewRunner`、启动漂移审计打一行 |

## 1. internal/config

### 1.1 新的配置键

`Config` 加一个根级字段，**位置放在 `DataDir` 之后、`Providers` 之前**：

```go
// RulesFile 是局部规则文件的路径。空串表示没有局部规则，这是正常情况。
// 相对路径相对**配置文件所在目录**解析，不是相对当前工作目录 ——
// 06 第 4 节明令不按 cwd 发现，否则入场费与行为随启动目录漂移。
RulesFile string `toml:"rules_file"`
```

`DefaultTOML()` 里加一行注释掉的示例，紧跟在 `data_dir` 之后：

```toml
# 局部规则文件，相对本文件所在目录解析。全局规则在 ~/.council/COUNCIL.md，不用在这里写。
# rules_file = "COUNCIL.md"
```

**校验**：`validate.go` 对 `rules_file` **不做任何校验**。它是不是指向一个存在的文件，
由 `LoadRules` 在真正读的时候回答；在校验期再 stat 一次只会制造两个事实源。

### 1.2 新常量

```go
// EnvGlobalRules 覆盖全局规则文件的位置。
// 只给测试与异机用：全局规则**不允许由配置文件指定**，
// 一份从别处拿来的项目配置不该能重新定义「什么算全局」。
EnvGlobalRules = "COUNCIL_GLOBAL_RULES"

// DefaultGlobalRulesName 是全局规则文件在 ~/.council/ 下的文件名。
DefaultGlobalRulesName = "COUNCIL.md"
```

### 1.3 `Rules` 类型

```go
// Rules 是两级规则文件的内容与出处。Text 字段已按 1.5 归一化。
type Rules struct {
	GlobalPath  string // 解析后的路径，恒非空（文件不存在时也有值，给 /rules 显示）
	GlobalText  string // 归一化后的正文；文件不存在时是空串
	GlobalFound bool
	LocalPath   string // 配置里没写 rules_file 时是空串
	LocalText   string
	LocalFound  bool
}
```

### 1.4 两个函数

```go
// ResolveGlobalRulesPath 按 环境变量 COUNCIL_GLOBAL_RULES -> ~/.council/COUNCIL.md 取值。
// 空字符串视为未设置，继续向下回落。返回的路径不保证文件存在。
// 取用户主目录失败时返回带上下文的错误。
// 与 ResolveConfigPath 同构，但**没有 flag 一级** —— 全局规则没有命令行开关。
func ResolveGlobalRulesPath() (string, error)

// LoadRules 读两份规则文件。
//
// configPath 是**生效的**配置文件路径（app 已经解析过的那个），
// 只用来解析 rulesFile 的相对路径，不读它本身。
// rulesFile 来自 Config.RulesFile，空串表示没有局部规则。
//
// 文件不存在**不是错误**：Found 为 false、Text 为空串、路径照填。
// 文件存在但读不出来（权限、是目录、I/O 错）**是错误**，带上下文返回。
// 这条区分是硬性的：把读失败当成空规则，会让一次 I/O 故障静默地把成员的
// 指令环境掏空，而下一轮照样花钱跑完、退出码 0。
func LoadRules(configPath string, rulesFile string) (Rules, error)
```

### 1.5 归一化：三步，顺序固定

`LoadRules` 读到字节之后按此顺序处理，**三步都是确定性的**：

1. **去掉开头的 UTF-8 BOM**（`EF BB BF`）。PS 5.1 的默认写法会插 BOM，
   而 BOM 落进系统提示是纯垃圾字节，还会让两台机器上「同一份」规则文件指纹不同。
2. **CRLF -> LF**。编辑器换一次行尾就让全队重建一次会话，那是一次**看不见的**
   字节变化换一笔真金白银，正是本项目一直在防的那类静默失败。
3. **去掉结尾的全部换行**（`strings.TrimRight(s, "\n")`）。段间分隔由渲染层负责
   （第 3.1 节），文件末尾有几个空行不该影响指纹。

**不做的**：不 Trim 开头、不动行内空白、不做任何 markdown 解析、不做变量替换。
规则正文是用户的，除了上面三样看不见的噪声之外原样送。

## 2. internal/store：schema v3

```go
var migrations = []migration{
	{version: 1, stmts: v1DDL, shape: v1Shape},
	{version: 2, stmts: v2DDL, shape: v2Shape},
	{version: 3, stmts: v3DDL, shape: v3Shape},
}

// v3DDL 不改任何列，只改数据：把 provider_sessions 里每一条四段指纹
// 追加两个空段，变成六段。
//
// 为什么必须有这一步：turn.IdentityFingerprint 从四段变六段，
// fingerprintFields 校验段数，于是库里现存的每一条都会 ok = false，
// 被当成「指纹整体不符」。从库读回那一支的处置是**静默作废重建**
// （runner.go 那段注释），也就是升级之后第一次 /ask 每条活着的会话
// 都会悄悄重建一次、各付一次入场费（D-4：0.4 到 1.0 美元），没有任何一层会说话。
//
// 追加的是空段而不是空串的哈希：turn.RulesDigest("") 规定返回空串，
// 两边正好对上，于是**规则为空的用户升级之后一条会话都不重建**。
// 规则非空时指纹正确地不符、会话重建 —— 那是对的，前缀真的变了。
//
// 这句「升级不重建」只对 6530839 之后建立的会话成立。那次提交改了 READ 档
// 实际发出的 argv 而没有动指纹（未决 #18），更早的会话本来就带着过期的前缀，
// 本迁移修不了它们，也不假装修得了。
//
// 不走代码层兼容垫片（让 fingerprintFields 容忍四段）：垫片是永久的，
// 而且它把段数校验从「严格」变成「看情况」，下一次加字段时没人还记得为什么。
var v3DDL = []string{
	`UPDATE provider_sessions SET identity_fp = identity_fp || '0:;0:;'`,
}

// v3Shape 与 v2 完全相同：这一版一列都没动。直接复用同一个 map，
// 不复制一份 —— 复制出来的副本迟早与 v2 的真相漂移。
var v3Shape = v2Shape
```

**测试方注意**：`Open` 在迁移之后会逐表比对形状，v3 不改列所以形状校验应当照常通过。
要验的是**数据**：造一个 v2 库、写一条四段 `identity_fp`、用 v3 二进制打开、
读回那一行，断言它变成了六段且前四段逐字节不变。

## 3. internal/turn

### 3.1 `RulesBlocks` 与 `renderIdentity`

```go
// RulesBlocks 是要渲染进身份块的两段规则正文，已由 config.LoadRules 归一化。
// 用独立类型而不是两个 string 参数：这两段总是一起传，拆开传迟早有人传反。
type RulesBlocks struct {
	Global string
	Local  string
}
```

`renderIdentity` 签名改为：

```go
func renderIdentity(agentID string, ac config.AgentConfig, rules RulesBlocks) string
```

**产出的确切字节**，按顺序拼接：

```
若 rules.Global != ""：  "# Council rules (global)\n" + rules.Global + "\n\n"
若 rules.Local  != ""：  "# Council rules (project)\n" + rules.Local  + "\n\n"
然后是今天已有的那段，一个字节都不改：
    "You are " + name + " (id=" + agentID + ").\n"
    "Provider: " + ac.Provider
    若 ac.Role != ""：  "\n" + ac.Role
```

**硬性要求（用例必须直接钉住）：`rules` 两段都为空时，
输出与本片改动之前逐字节相同。** 这条加上第 2 节的迁移，
共同保证不用规则文件的人升级之后一条会话都不重建。

**顺序为什么是共享在前、专有在后**：未决 #1 已测，缓存**按内容命中不按会话**，
所以同一家 provider 的多个 agent 共享的前缀越长越省。排序本身零成本。

**标签为什么留着**：grok 那边切不断用户全局规则的继承，出问题时唯一的窗口
是让成员自述它看到了什么；没有标签它没法说清那条指令来自哪个文件。
标签是 ASCII，两行加起来二十几字节。

`renderIdentity` 仍然是纯函数：不含时钟、不含会话 id、不含轮次。

### 3.2 指纹：四段变六段

```go
const (
	fpFieldRole        = 0
	fpFieldModel       = 1
	fpFieldEffort      = 2
	fpFieldTier        = 3
	fpFieldRulesGlobal = 4
	fpFieldRulesLocal  = 5
	fpFieldCount       = 6
)

// RulesDigest 把一段规则正文压成 "<字节数>/<sha256 前 12 位小写十六进制>"，
// 例如 "4821/3f2a1b9c4d5e"。
//
// **空串返回空串**，不是空串的哈希。schema v3 的迁移靠这条对上，别「修」它。
//
// 存摘要不存全文，与 role 的处置相反，理由是两者要回答的问题不同：
// drift 报错要能点名 role 的旧值（那是一行字，看得过来），
// 而一段几 KB 的旧规则原文在终端上没法给人看，能给人看的是「变了多少、变成了什么」。
// 全文进 identity_fp 这一列只会把库撑大而换不到表达力。
func RulesDigest(text string) string

// IdentityFingerprint 的签名**在本片修改**（10a 第 3 节冻结过四参数版本，
// 本文件取代它）。新增一个参数，内部自己算摘要 —— 只有一处算摘要，
// 调用方不可能把两段传反之外再把摘要算错。
func IdentityFingerprint(
	role, model, effortFlagEmitted string,
	tier provider.PermissionTier,
	rules RulesBlocks,
) string
```

实现即 `packFingerprint(role, model, effortFlagEmitted, string(tier),
RulesDigest(rules.Global), RulesDigest(rules.Local))`。
新字段追加在末尾，前四个下标保持不变 —— 迁移才能是一次纯追加。

`fingerprintFields` 的返回值相应加两个：

```go
func fingerprintFields(fp string) (role, model, effort, tier, rulesGlobal, rulesLocal string, ok bool)
```

`identityDriftError` 加两条比对，**措辞按用户看得懂的说**：

```go
if oldRulesGlobal != newRulesGlobal {
	parts = append(parts, fmt.Sprintf("全局规则 %q -> %q", oldRulesGlobal, newRulesGlobal))
}
if oldRulesLocal != newRulesLocal {
	parts = append(parts, fmt.Sprintf("局部规则 %q -> %q", oldRulesLocal, newRulesLocal))
}
```

不要写成 `rules_global`：用户改的是一个 md 文件，报错里出现的应当是他认得的东西。

### 3.3 Runner 拿到规则

```go
// NewRunner 的签名在本片修改，末尾加一个参数。
func NewRunner(cfg *config.Config, svc *conversation.Service, workDir string, rules RulesBlocks) *Runner

// SetRules 换掉 Runner 手里的规则。只由 /rules reload --rebuild 调用。
//
// 不加锁：Runner 的全部方法本来就只从 REPL 那一条 goroutine 调用
// （Step 7 起没有变过）。哪天真有第二条 goroutine，要加的锁不止这一处。
func (r *Runner) SetRules(rules RulesBlocks)
```

~~`Ask` 里那一行改成 `identity := renderIdentity(agentID, ac, r.rules)`，
`fp := IdentityFingerprint(identity, ac.Model, emitted, tier, r.rules)`。~~

**订正（2026-09-21，实现期间）。上面那两行是错的，照它写会毁掉整个摘要设计。**
实现方在照契约写代码时指出：`IdentityFingerprint` 把 `role` 段**按全文**存进
`identity_fp` 那一列，若把带规则的整块喂进去，规则全文会再进一次那一列 ——
两段摘要就白设了（库照样被撑大），而且规则一改，`identityDriftError`
会先把几 KB 的旧规则原文糊在屏幕上，再打那两行本来是给人看的摘要。

**正确的形状**：身份分两块渲染，只有**不含规则**的那块进指纹。

```go
// renderIdentityCore 是不含规则的那半块（本片改动之前 renderIdentity 的全部内容）。
func renderIdentityCore(agentID string, ac config.AgentConfig) string

// renderIdentity = 规则两段 + core，这才是送进 TurnRequest.Role 的东西。
func renderIdentity(agentID string, ac config.AgentConfig, rules RulesBlocks) string

// identityFPFor 是 Ask 与 AuditDrift 共用的那一处指纹计算。
// 两处各写一份的话，任何一处漏改都会让审计报出不存在的漂移、
// 或反过来漏报真实的漂移，而这种不一致不会在编译期或运行时报错，
// 只表现成「审计的数字」与「Ask 实际拒不拒」对不上。
func identityFPFor(agentID string, ac config.AgentConfig,
	caps provider.Capabilities, tier provider.PermissionTier, rules RulesBlocks) string
```

`Ask` 里送出去的仍是 `renderIdentity(...)` 的产出，进指纹的是 `core`。
**其余流程一个字不动。**

### 3.4 `Rebuild`：漂移之后的出路

```go
// Rebuild 丢弃 (convID, agentID) 的 provider 会话，让下一轮走 Fresh。
//
// **五样必须全丢**：会话 id / 游标 / 成本基线 / lastTurnAt / 指纹。
// 做法是删掉 provider_sessions 里那一整行，不是只清指纹 ——
// 10_STEP7_SPEC.md 第 4 节那条原样适用：重建意味着 provider 那边计数器从零开始，
// 只作废会话 id 而留着旧基线，下一轮差值是负数、computeCostDelta 返回 nil，
// 那一笔真实花掉的钱就静默从账上消失了。
//
// 同时删掉内存里 sess[key] 那一条，否则下一轮还会读到旧的。
//
// existed 报告本来有没有这条会话。不存在**不是错误**：
// 用户对一个还没说过话的成员敲 rebuild，回一句「本来就没有」就够了。
func (r *Runner) Rebuild(ctx context.Context, convID, agentID string) (existed bool, err error)
```

### 3.5 `AuditDrift`：把静默重建变响亮

```go
// DriftRow 是一条「身份已变、下次 /ask 会重建」的 provider 会话。
type DriftRow struct {
	ConvID  string
	AgentID string
	Reason  string // 人话。见下面三种取值
}

// AuditDrift 把库里全部 provider 会话逐条与「当前配置 + 当前规则」算出的指纹比对。
//
// 纯读，不写任何东西，也不碰 r.sess —— /rules 和启动横幅都会调它，
// 一个看一眼的命令不该产生副作用（runner.go 里 SessionInfo 那条注释同理）。
//
// 返回按 (ConvID, AgentID) 升序，保证输出稳定、可以写 golden 用例。
//
// Reason 的三种取值：
//   - 指纹解得开且能算出新值 -> identityDriftError 那套逐项措辞，例如「全局规则 "..." -> "..."」
//   - 指纹解不开（四段的旧行，理论上迁移之后不该有）-> "指纹格式不可解"
//   - 配置里已经没有这个 agent -> "配置里已无此 agent"（这条也要报，
//     因为那条会话还在库里占着位，用户删了 agent 又加回来时它会复活）
func (r *Runner) AuditDrift(ctx context.Context) ([]DriftRow, error)
```

**X-39f 订正（2026-09-24，`7d65b96`）**：实现里 Reason 不止上面三种，而是六种；「身份已变、下次 /ask 会重建」只对其中两种成立。
`DriftRow` 加了 `Kind DriftKind`（`DriftAgentGone`=1 配置里已无此 agent、`DriftProviderUnavailable`=2、`DriftPermissionRefused`=3 权限档不放行、
`DriftFingerprintUnavailable`=4 算不出指纹、`DriftFingerprintUnparsable`=5 指纹格式不可解、`DriftFieldsChanged`=6 逐字段差异；0 为未填）
与方法 `AskRefused()`：前四类为真——按当前配置下次 Ask 会在读进程内状态之前被拒，不起进程、不花钱；后两类为假——跨重启的下次 Ask 会作废重建、
付一次入场费，进程内已开过口的会被 `ErrIdentityDrift` 拒绝、要 `/session rebuild` 之后才按首轮付。Reason 文本不变。
提示层（§5.1、§5.2、§5.3、§6）一律按 `AskRefused()` 分两类说。

### 3.6 命令行总长闸（未决 #15 到期）

放在 `spawn.go`，与 `ErrShimNewlineArg` 那道闸**并排**，理由相同：
一处判定，将来加第四家 provider 时结构上不可能漏掉。

```go
// ErrCommandLineTooLong 表示组出来的整条命令行超过 OS 上限。
var ErrCommandLineTooLong = errors.New("assembled command line exceeds the OS limit")

// maxCommandLineBytes 是 Windows CreateProcess 的命令行上限。
//
// 与 provider.MaxPromptBytes 不是一回事，两者量的是不同的东西：
// 那个说「这家的正文通路有上限」，这个说「这一条具体的命令行太长了」。
// 未决 #15 记的漏风就在两者的差值里，规则文件让它到期了。
//
// 本项目只在 Windows 上跑过。别的平台上这个值偏严（Linux 的 ARG_MAX 大得多），
// 真要跨平台时按 GOOS 分档，别在这里编一个折中值。
const maxCommandLineBytes = 32767

// CommandLineBytes 量一条真实命令行的长度：可执行文件路径，
// 加上每个参数前面一个分隔空格。
//
// **这个数偏小，是有意的、也是已知的**：Go 的 os/exec 在 Windows 上
// 会按自己的规则给含空格的参数加引号，加引号之后比这个数长。
// 我们量的是加引号前的长度，所以这道闸能挡住「规则文件太大」这类
// 整数量级的越界，挡不住刚好卡在边上的那几十字节。
// 不要为了「更准」去编一个余量常数 —— 那会把「未实测」伪装成「已标定」。
func CommandLineBytes(path string, args []string) int
```

闸的行为，在 `Spawner` 起进程之前、与换行那道闸同一处：

```
总长 = CommandLineBytes(c.Path, c.Args)
总长 > maxCommandLineBytes -> 返回错误，包 ErrCommandLineTooLong
```

**错误文本必须点名最大的那个参数**，形如：

```
命令行 41203 字节，上限 32767；最长的参数是 --system-prompt-override 的值（38104 字节）
```

「最长的那个参数是哪个 flag」的判定：找出 `len` 最大的那个 `args[i]`；
若 `i > 0` 且 `args[i-1]` 以 `-` 开头，就报成「`args[i-1]` 的值」，否则直接报下标。
这样用户看到的是「砍规则文件」，而不是一个 `CreateProcess` 的数字。

**订正（2026-09-21，冻结之后）**：这里原本写的是「以 `--` 开头」，那会漏掉
grok 与 agy —— 它们的正文都绑在**单横杠 `-p`** 上，于是最长参数是正文时
报出来的是 `args[1]` 这种没用的东西。放宽到 `-`。
**要在注释里写明这是启发式不是解析**：它会把一个以减号开头的**值**误认成 flag，
本项目三个适配器里不存在这种值，但这条判断本身没有任何语法依据。

**另一条同期订正**：`ErrCommandLineTooLong` 在 **agy 上多数情况下不会触发**。
agy 把 Role 折进 argv 的 prompt，`BuildTurn` 里已有的 `checkArgvPrompt`
（`MaxPromptBytes`，同样是 32767）会先拦下，报 `ErrPromptTooLong`。
两条都是起进程之前 fail-closed，行为对，但**测试不要对三家写同一个哨兵断言**。

`checkArgvPrompt` 与 `MaxPromptBytes` **原样保留**，两条闸并存。

## 4. internal/conversation

**订正（2026-09-21，冻结之后）**：本节原文把 `AllProviderSessions` 写成
`func (s *Store) ...`，措辞含混。`conversation.Store` 是**接口**，
真正的实现在 `internal/store`，所以这一条要分三处落，下面写清楚。

```go
// 一、conversation.Store 接口加一个方法（store.go 的接口声明里）：
//
// AllProviderSessions 列出全库的 provider 会话，按 (conv_id, agent_id) 升序。
// 只读，给启动漂移审计与 /rules 用。库里一条都没有时返回空切片与 nil，不是错误。
AllProviderSessions(ctx context.Context) ([]ProviderSession, error)

// 二、internal/store 实现它（read.go，紧挨着既有的 ProviderSession）。
//    SQL 与那个单行版本同构，去掉 WHERE、加 ORDER BY conv_id, agent_id，
//    复用现成的 scanProviderSession。

// 三、Service 上开一个只读直通与一个写入口（service.go）：
func (s *Service) AllProviderSessions(ctx context.Context) ([]ProviderSession, error)

// DropProviderSession 删掉一条 provider 会话，供 /session rebuild 用。
// 在自己的 WriteTx 里调已有的 Tx.DropProviderSession。
// 返回 existed 报告本来有没有这一行 —— 先读一次再删，两次都在同一个 WriteTx 里。
func (s *Service) DropProviderSession(ctx context.Context, convID, agentID string) (existed bool, err error)
```

`ProviderSession` 类型已有 `ConvID` / `AgentID` 两个字段，直接用，不另造行类型。

**已知代价，不要绕**：给接口加方法会让**六个测试假实现一起编译不过**
（`internal/conversation/record_turn_test.go`、`service_test.go`、
`internal/repl/conversation_test.go`、`internal/turn/recording_store_test.go`、
`runner_cursor_test.go`、`session_info_readback_test.go`）。
**这是对的，要的就是它响亮**：新增一项必需能力时，每个假实现都应当被迫表态。
不许用类型断言（`if lister, ok := s.store.(interface{ ... })`）绕过去 ——
那样假实现会静默地走「没有这个能力」那一支，审计恒报 0 条，
而屏幕上看起来一切正常。这正是本项目一路在防的那类静默失败。

假实现补上的方法可以直接 `return nil, nil`，除非那个用例本来就要验审计。

## 5. internal/repl

### 5.1 `/rules`

注册：`{name: "rules", summary: "显示规则文件与逐 provider 的指令环境", needsRunner: true, run: cmdRules}`。

不带参数时输出（顺序固定，可以写 golden）：

```
全局规则: <绝对路径> (<N> 字节)        或  <绝对路径> (不存在)
局部规则: <绝对路径> (<N> 字节)        或  (未配置)
合计 <N> 字节

claude   唯一指令源            进缓存前缀，首轮付        命令行余量 <N> 字节
grok     叠加在用户全局规则之上  进缓存前缀，首轮付        命令行余量 <N> 字节
agy      未验证，按叠加处理      无缓存，按轮数乘着付      命令行余量 <N> 字节

身份已变的 provider 会话: <N> 条（下次 /ask 会各重建一次）
```

（X-39f 订正：上面这行只数 `AskRefused()` 为假的那类，括号改成「要重建才能接着用，各付一次入场费」——进程内命令不断言时机；
被拒类另起一行 `按当前配置 /ask 会被拒的 provider 会话: <N> 条（不花钱）`。两类都为 0 时不印。）

逐行的取值来源：

- **隔离位**取 `p.Capabilities().CleanInstructionEnv`，三态直译：
  `SupportYes` -> `唯一指令源`、`SupportNo` -> `叠加在用户全局规则之上`、
  `SupportUnknown` -> `未验证，按叠加处理`。
  **Unknown 按最坏情况显示**，这是 fail-closed 的显示面写法。
  不要在 repl 里写任何 provider 专有的话术，逐家的实测细节在 `06` 4.1 与 `11` 1.2。
- **计价措辞**取 `p.Capabilities().CacheAcrossTurns`（2026-09-21 补，见下），
  三态直译：`SupportYes` -> `进缓存前缀，首轮付`、
  `SupportNo` -> `无缓存，按轮数乘着付`、`SupportUnknown` -> `缓存行为未测`。
  不要在 repl 里写任何 provider 的名字，逐家的实测细节在 `06` 4.1 与 `11` 1.2.1。

  **这一位补上之前的历史**：本节最初写着「若当前 `Capabilities` 没有一个能
  直接表达跨轮有没有缓存收益的位，不要为这行新造一个能力位，改成对所有
  provider 都印同一句固定文本」，实现方当时确实照办了——三行印的是同一个
  常量 `pricingNote`，文本是
  `进缓存前缀，首轮付（agy 无缓存，按轮数乘着付，见 11 节 1.2.1）`。
  **这句文本本身违反了本条开头「不要在 repl 里写任何 provider 专有话术」
  这条要求**：它在每一行里都点名了 agy，是这次修订之前遗留的自相矛盾。
  三家现在都有了实测支撑（claude 见 D-4，grok 见未决 #1，agy 见未决 #17，
  三条数据与出处逐条写在 `provider.Capabilities.CacheAcrossTurns` 的注释里），
  于是新增了这一位能力位，按三态分开印。
  **当初那条禁令禁的是「编一个没有标定过、假装支持的能力位」，不是「禁止
  新增能力位」**——没有证据时才该退而求其次印一句诚实但笼统的固定文本；
  证据补齐之后继续印同一句话，反而是把已经测出来的区分抹平，且明知那句
  固定文本点名 agy 还留着不改，与本条开头的要求正面冲突。
- **命令行余量** = `maxCommandLineBytes - CommandLineBytes(c.Path, c.Args)`，
  其中 `c` 用一个**空正文**的探针请求现组：真实身份块（含规则）、真实权限档、
  `ProviderSession` 传空串（首轮，argv 最长的那一轮）、`Prompt` 传空串。
  组不出来（例如该 provider 拒绝当前权限档）就印 `不适用（<原因>）`，不要让整条命令失败。

  **这样调 `BuildTurn` 是安全的，已核对过**：`BuildTurn` 只组字符串、不起进程、
  不落盘（`Command.Files` 是待落盘的内容，由 `Spawner` 落，不是 `BuildTurn` 落）。
  唯一的非纯净处是 grok 在 `ProviderSession == ""` 时会铸一个 v4 UUID
  （`provider/uuid.go`，纯 `crypto/rand`，不写文件不记日志），探针铸出来直接丢掉，
  不会与任何真实会话冲突——grok 的规矩本来就是「没有被输出确认过的 id 一律作废」。
  实现方不要为了躲这个 UUID 去传一个假的 `ProviderSession`：
  那会走 resume 分支，量到的是**较短**的那一条命令行，余量报大了反而危险。
- **最后一行**来自 `Runner.AuditDrift`，0 条时**整行不印**。

**token 估算不印**。没有标定过的 byte-per-token 换算，印出来就是编的。
需要量级参照时看字节数，外加 `11` 1.8 记的那个参照值。

### 5.2 `/rules reload [--rebuild]`

- **不带 `--rebuild`**：重读两个文件，**不改 Runner 手里的规则**，只报告。
  印新旧字节数与摘要、以及会因此漂移的成员名单（用新规则算一遍指纹再比）。
  末尾提示 `要应用请敲 /rules reload --rebuild（会重建上列会话，各付一次入场费）`。
- **带 `--rebuild`**：重读、`SetRules`、对上列每一条调 `Runner.Rebuild`，逐条印结果。

分两步是因为一条命令可能一次烧掉几个入场费，那种事必须是用户按下去的。
读文件失败时两种形态都**不改任何状态**，原样把错误印出来。

（X-39f 订正：末尾提示按两类计数分四种——只有重建类用原句；两类都有时说「其中 N 条会重建，各付一次入场费；另 M 条只会被清掉，不花钱」；
只有被拒类说「上列会话只会被清掉，不花钱」；都没有时只印「要应用请敲 /rules reload --rebuild」。`--rebuild` 仍对全部行调 Rebuild，
逐行结果被拒类印「已清掉 <conv>/<agent>（existed=…；按当前配置 /ask 会被拒，不花钱）」，失败前缀「清掉 … 失败」。）

### 5.3 `/session rebuild <agentID>`

`cmdSession` 加子命令。不带参数时行为**一个字不改**（仍是列表）。

```
/session rebuild <agentID>
```

- agentID 不在当前会话的活跃成员里：报错，不动任何东西。
- 调 `Runner.Rebuild`。`existed` 为 false 时印「本来就没有开着的会话，下一轮照常是首轮」。
- 成功时印「已丢弃 <agentID> 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢），下一轮按首轮计价」。

（X-39f 订正，Griffin 裁定「放宽 /session rebuild，不加新命令」：闸放宽为「是活跃成员，**或者** `AuditDrift` 对（当前会话, 这个 id 精确匹配，
不走前缀、不折叠大小写）报出 `Kind == DriftAgentGone`」——这样已删 agent 留下的非成员悬空行也能清；第 2、3、4 类的非成员仍被拒
（多半是一时写错配置，改回去还能 resume）。先调 `AuditDrift`，失败就报「身份漂移审计失败」、成员与非成员都不 Rebuild（fail-closed）。
成功时该行 `AskRefused()` 为真印「已清掉 <agentID> 的 provider 会话（…五样全丢）；按当前配置对它 /ask 会被拒，不花钱」，否则印上面的原句。）

## 6. internal/app

`Run` 里，`config.LoadAndValidate` 成功之后、`store.Open` 之前：

```go
rules, err := config.LoadRules(path, cfg.RulesFile)
```

失败的处置**与配置加载失败同一套**：把错误写进 `opts.Err`、返回错误、**不进 REPL**。
理由写在 `11` 1.1 末：规则文件读不出来不能当空的。

`NewRunner` 那一行加上 `rules`。

横幅之后加**一行**漂移审计，`AuditDrift` 返回 0 条时**什么都不印**：

```
注意: 有 <N> 条 provider 会话的身份已变，下次 /ask 会各重建一次（每条一次入场费）
```

（X-39f 订正：这一句只数 `AskRefused()` 为假的那类、计数为 0 时不印（启动时所有进程内状态都是空的，第 5、6 类的下次 Ask 一定走重建，所以「下次 /ask」在这里成立）；
被拒类另起一句 `注意: 有 <N> 条 provider 会话按当前配置 /ask 会被拒，不花钱；其中原因是「配置里已无此 agent」的，先 /use 那个会话再敲 /session rebuild <成员> 即可清掉`；
两句之后逐行明细照旧全列。只有被拒类时整段不出现「入场费」。）

审计本身失败（读库出错）只往 `Err` 记一行，**不阻止进 REPL**：
它是一个提醒，不是启动的前提条件。这与「读不出规则就不进 REPL」不同，
因为那一条会让成员带着错误的指令环境跑，而这一条只是少一句提醒。

## 7. 不属于契约面

以下随便改，测试不许钉：

- 所有输出的**具体措辞**（除了第 5 节明确给了格式的那几行）
- `RulesDigest` 里用 sha256 还是别的摘要算法（但「空串返回空串」和「12 位十六进制」是契约）
- `AuditDrift` 内部怎么组织循环
- 错误信息的中文措辞（但**哨兵**是契约：`ErrCommandLineTooLong` 必须可 `errors.Is`）

## 8. 验收（测试方照这条写）

用例必须覆盖：

1. **空规则逐字节不变**：`renderIdentity` 两段都空时的输出与 golden 相等。
2. **v3 迁移**：造 v2 库 + 一条四段 `identity_fp`，v3 打开后变六段、前四段不变；
   且规则为空时新算出的指纹与迁移后的值**相等**（这条是「升级不花钱」的直接断言）。
3. **`RulesDigest("") == ""`**，非空串形如 `<数字>/<12 位十六进制>`。
4. **`LoadRules`**：文件不存在 -> `Found=false`、无错误；
   路径指向一个目录 -> **有错误**；BOM 被去掉；CRLF 变 LF；结尾换行被去掉。
5. **指纹漂移点名规则**：只改全局规则正文时，`identityDriftError` 的文本里
   出现「全局规则」四个字且**不**出现「role」。
6. **`Rebuild` 五样全丢**：断言走的是删行（`DropProviderSession`），
   不是只清指纹；不存在时 `existed=false` 且不报错。
7. **`AuditDrift`**：0 条、N 条、指纹解不开、agent 已从配置里删掉，四种各一例；
   返回顺序按 (ConvID, AgentID) 升序。
8. **命令行总长闸**：造一份超大规则文件，grok 与 agy 在**起进程之前**
   以 `ErrCommandLineTooLong` 失败，错误文本含最长参数对应的 flag 名；
   claude 同样规则下**不**触发（它走文件与 stdin）。
9. **`/rules reload` 不带 `--rebuild` 不改状态**：调用前后 `Runner` 手里的规则相同。

真机验收（不在单测里，由主代理跑）见 `11_STEP8_SPEC.md` 第 5 节。

## 9. 符号禁令（写代码时必须遵守）

代码文件里（源码、注释、docstring、字符串字面量、打印输出、变量名）
**禁用装饰性符号**：

| 禁 | 码位 |
|---|---|
| Emoji / 象形图 | U+1F300–U+1FAFF |
| Dingbats | U+2700–U+27BF |
| 杂项符号 | U+2600–U+26FF |
| 杂项符号与箭头 | U+2B00–U+2BFF |
| 带圈字母数字 | U+2460–U+24FF |
| 隐形伴随字符 | U+FE0F 变体选择符、U+200D ZWJ |

**放行**：希腊字母 U+0370–U+03FF、数学算符 U+2200–U+22FF、`°` `±` `×` `÷` `µ`、
上下标、普通箭头 U+2190–U+21FF（注释里写「输入 -> 输出」正常）、中文、ASCII 制表符。

禁的是装饰符号，不是要求纯 ASCII。中文注释照写。
