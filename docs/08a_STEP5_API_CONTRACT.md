# Step 5 API 契约（冻结）

`08_STEP5_SPEC.md` 说「做什么」，本文件说「导出的符号长什么样」。
多个执行者并行写不同的包，这是它们之间唯一的接口约定。
**实现者不得改动本文件里的签名**；觉得签名有问题，退回来说，不要自行改。

Step 4 的教训已经吸收进来了：本文件末尾有「不属于契约面的东西」一节，
那些东西实现可以自由决定，测试不得对它们做字面断言。

## internal/conversation

领域层。**不导入 `database/sql`，不导入 `internal/store`。**

```go
package conversation

type MessageKind string

const (
    KindUser       MessageKind = "user"
    KindAgent      MessageKind = "agent"
    KindSystem     MessageKind = "system"
    KindTask       MessageKind = "task"
    KindTaskResult MessageKind = "task_result"
)

// ReservedSenderUser 是人在对话里的 sender id。它不是 participant，
// 不需要 join，也不出现在成员列表里，但可以作为 Message.SenderID。
const ReservedSenderUser = "user"

type Conversation struct {
    ID        string
    Title     string
    CreatedAt time.Time
}

type Participant struct {
    ConvID      string
    AgentID     string
    DisplayName string
    Provider    string
    Active      bool
    JoinedAt    time.Time
    LeftAt      *time.Time // nil 表示还在
}

type Message struct {
    ID       string
    ConvID   string
    Seq      int64 // 由 store 在事务里分配，调用方传进来的值一律忽略
    SenderID string

    // 这一次发言当时用的「脑子」。是消息的属性，不是 participant 的属性 —— 理由见下。
    // SenderID 是 ReservedSenderUser 时这三个字段必须为空（拦的是 SenderID，不是 Kind）。
    SenderProvider string
    SenderModel    string
    SenderEffort   string

    Recipients []string // 空 = 广播给当前活跃成员
    Kind       MessageKind
    Text       string
    Thought    string
    ParentID   string // 空表示没有父消息
    CreatedAt  time.Time
    Meta       map[string]any
}

// SenderBrainEmpty 报告三个字段是不是都空。不要求三者同时填满：
// 有的 provider 没有 effort 这个概念，只填 provider 与 model 是合法的。
func (m Message) SenderBrainEmpty() bool
```

**为什么脑子记在 message 上而不是 participant 上**（这是本次改动的全部理由，不要"优化"回去）：

D-4 与 `02` §4 定死了「换 model 或 effort 必须重建会话」，所以同一个 agent 在一场对话里
完全可能换过脑子。若把 model/effort 记在 `participants` 行上，换一次脑子就会把它**过去说过的
每一句话**追溯性地重新标注成新脑子说的 —— 那是在伪造历史。记在消息上，每条消息各自带着
它当时的脑子，换脑子只影响之后的消息。

由此派生一条给 Step 6/7 的要求，现在写进契约：**因换 model/effort 而重建会话时，
必须往 Canonical Conversation 追加一条 `system` 消息记录这次切换**
（`04` §0 规则 3：重建是有标价的操作，要显式记录）。否则 `/log` 里只能看到脑子变了，
看不到什么时候、为什么变的。

**这三列记的是「请求时声明的档位」，不是「实测生效的档位」**。会话 #1 已实证：
grok 的 effort 由 `~/.grok/config.toml` 决定，命令行参数经 CPA 会被能力门静默丢弃
（`02` §1.1 把这类 provider 的 EffortPolicy 声明为「进程外配置」）。
Step 6 的适配器只能把 `AgentConfig.Effort` 抄进这一列。**不要把这列当 ground truth。**

```go

type Cursor struct {
    ConvID    string
    AgentID   string
    Seq       int64
    Pending   []int64
    UpdatedAt time.Time
}

// Valid 报告这个 kind 是否属于五个合法值之一。
func (k MessageKind) Valid() bool
```

### Store 接口（消费方定义，由 internal/store 实现）

```go
// Store 是 Canonical Conversation 的持久化出口。
// 接口定义在这里而不是 internal/store，是为了让领域层能在测试里换一个内存实现。
type Store interface {
    WriteTx(ctx context.Context, fn func(context.Context, Tx) error) error

    Conversation(ctx context.Context, id string) (Conversation, error)
    Conversations(ctx context.Context) ([]Conversation, error)
    Participants(ctx context.Context, convID string, includeInactive bool) ([]Participant, error)
    Messages(ctx context.Context, convID string, afterSeq int64, limit int) ([]Message, error)
    MaxSeq(ctx context.Context, convID string) (int64, error)
    Cursor(ctx context.Context, convID, agentID string) (Cursor, error)
    Cursors(ctx context.Context, convID string) ([]Cursor, error)
}

// Tx 是一个写事务里能做的全部动作。Step 7 会在这里加 RecordUsage 与
// SetProviderSession，让「记一轮」的四张表写在同一个事务里。
type Tx interface {
    CreateConversation(ctx context.Context, title string) (Conversation, error)
    AppendMessage(ctx context.Context, m Message) (Message, error)
    AddParticipant(ctx context.Context, p Participant) error
    DeactivateParticipant(ctx context.Context, convID, agentID string, at time.Time) error
    // 在同一个写事务内确认 agentID 是 convID 当前的活跃成员，不是就返回
    // 包了 ErrParticipantNotFound 的错误。必须在事务内，理由见下面 Post 那一节。
    RequireActiveParticipant(ctx context.Context, convID, agentID string) error
    SetCursor(ctx context.Context, c Cursor) error
}
```

`AppendMessage` 返回的是**补全后的** Message：`ID`、`Seq`、`CreatedAt` 由 store 填，
传进来的这三个字段一律忽略。

### 哨兵错误

```go
var (
    ErrConversationNotFound = errors.New("conversation not found")
    ErrParticipantNotFound  = errors.New("participant not found")
    ErrInvalidKind          = errors.New("invalid message kind")
    ErrInvalidSender        = errors.New("invalid message sender")
    ErrNoCursor             = errors.New("cursor not found")
)
```

调用方用 `errors.Is` 判断。store 实现必须用 `%w` 包装这些哨兵，
不要让调用方去匹配错误字符串。

### Service

```go
// Service 在 Store 之上提供领域动作。它不认识 SQL，也不认识配置文件。
type Service struct { ... }

func NewService(s Store) *Service

func (s *Service) Create(ctx context.Context, title string) (Conversation, error)
func (s *Service) Join(ctx context.Context, convID string, p Participant) error
func (s *Service) Leave(ctx context.Context, convID, agentID string, at time.Time) error
func (s *Service) Post(ctx context.Context, m Message) (Message, error)
func (s *Service) Recipients(ctx context.Context, m Message) ([]string, error)
func (s *Service) History(ctx context.Context, convID string, afterSeq int64, limit int) ([]Message, error)

// 下面三个是读路径的转发。分层要求上层（REPL）只认 Service 这一个持久化入口，
// 不该为了列个会话就自己去拿 Store。它们刻意不校验会话是否存在：
// 读路径每次多查一次纯属浪费，不存在的会话自然读出空结果。
func (s *Service) Conversations(ctx context.Context) ([]Conversation, error)
func (s *Service) Participants(ctx context.Context, convID string, includeInactive bool) ([]Participant, error)
func (s *Service) MaxSeq(ctx context.Context, convID string) (int64, error)

// SetThoughtPolicy 是未决 #9(a) 落地时补的第十个方法，见下方订正。
func (s *Service) SetThoughtPolicy(store bool)
```

这三个是补录的：本文件第一版把 `Service` 冻结成六个方法、漏了它们，
导致 REPL 编译不过（`cmdConv` / `cmdWho` / `cmdLog` 都要用）。
那是契约的漏洞，不是实现者越界。**现在补齐了，以本清单为准，九个方法。**

**订正（2026-09-21，未决 #9(a)，契约面新增）**：`SetThoughtPolicy(store bool)`
是第十个方法，`NewService(s Store) *Service` 本身一个字没改——那个签名仍然
冻结。加它是因为 `00_DECISIONS.md` #9(a) 裁的落点是「配置之后再设置」，
仿 `internal/turn.Runner.SetRules` 的形状（构造之后再配置），而不是把
`store bool` 塞进 `NewService` 的参数列表：那样会牵动全仓三十多处
`NewService(...)` 调用点（几乎全是测试），且与「`NewService` 只认识
`Store`，不认识配置」这条既有分工冲突。

**这不是零契约改动，`Post` / `RecordTurn` 两个已冻结方法的签名没变，但
行为随这个新方法变了**：`SetThoughtPolicy(false)` 之后，`Post` 与
`RecordTurn` 会把消息的 `Thought` 落成空串，并在真的丢了非空内容时把
`conversation.MetaThoughtDropped`（`"thought_dropped"`）写进那条消息的
`Meta`。不调用 `SetThoughtPolicy`（本文件与全仓既有测试的默认状态）时
两个方法的行为与之前完全一致——`Service` 的这个字段零值即「存」，
不是「丢」，这一点是有意设计成这样的：反过来会让全仓那三十多处不知情的
`NewService(...)` 调用点从这次改动起悄悄开始丢 thought。

`Recipients` 的语义（规格 §G）：`m.Recipients` 非空就原样返回，但要先校验每个 id
都是该会话的 participant，否则返回 `ErrParticipantNotFound` 包装的错误；
为空则返回当前**活跃** participant 的 id 列表（按字典序，保证稳定）。

`Post` 的校验**分两处**，这是有意的：

**事务外**（便宜、不需要原子性，失败时连写事务都不必开）：会话存在、`Kind` 合法、
`SenderID` 非空、**`SenderID` 是 `ReservedSenderUser` 时三个脑子字段必须全空**
（违反时返回包了 `ErrInvalidSender` 的错误 —— 人不是模型，没有脑子可记；
真出现说明上层把 agent 的回复当成用户发言写进来了，要响亮地失败）。
会话存在这一条留在事务外，是因为会话从不删除、不存在 TOCTOU，
而先查一次能给出比事务内失败更友好的错误。

**事务内**（与追加消息同一个 `WriteTx`）：`SenderID` 不是 `ReservedSenderUser` 时，
必须经 `Tx.RequireActiveParticipant` 确认它是该会话**当前的活跃** participant。

这一条**必须**在事务内。原先它在事务外用读句柄查一次再另起写事务，中间有窗口：
并发发言时读到「活跃」之后、提交之前，用户 `/leave` 可能先行提交，这条消息照样落库，
事后 `/who` 显示它已退出、`left_at` 还早于这条消息的 `created_at`，
而没有任何一层会报错。Step 6 起真有并发，所以现在挪进来（原未决 #13）。

`messages` 表上另有一条同义的 `CHECK` 约束作为结构兜底，与 `kind` 的 `CHECK` 属于同一类。
两层是有意的：`Service` 给出可被 `errors.Is` 认出的友好错误，表约束保证绕过 Service
的写入路径也写不坏数据。

**`store` 层不重复做 sender / 脑子这一组校验**，那是 Service 的职责 ——
注意这句话只管这一组。`Tx.AppendMessage` 里仍然有一条 `Kind.Valid()` 的 Go 层校验，
那是刻意留的：`kind` 的 `CHECK` 与 `UNIQUE(conv_id, seq)`、
以及本次新加的那条 `CHECK` 都是匿名约束，驱动只给得出
`CHECK constraint failed: messages` 这种分不出是哪一条的错误，
所以 `kind` 这一条在 Go 层先拦，好让调用方能 `errors.Is` 出 `ErrInvalidKind`。
sender 那一条没有对应的 store 层哨兵 —— 绕过 Service 直插时拿到的是裸的约束错误，
`errors.Is(err, ErrInvalidSender)` 匹配不上。这是已知的不对等，Step 7 若要在
`Tx` 上直接拼「记一轮」，得先想好这个错误怎么分类。

### 给 Step 6 的硬性要求（MUST）

provider 适配器 `Post` 的 `KindAgent` 消息**必须**填 `SenderProvider` 与 `SenderModel`
（`SenderEffort` 可空 —— 有的 provider 没有这个概念）。

Step 5 **没有**任何一层强制这一条：两层校验只管「sender 是 `user` 时三列必须空」，
反方向一条约束都没有，agent 消息三列全空照样落库。而渲染侧把三列全空渲染成占位符，
占位符的定义又是「不适用」而不是「数据丢了」—— 于是适配器漏填会落在一个
代码自己认定为合法的显示上，不报错、测试全绿、用户要的「知道是哪个脑子在说话」当场落空，
且 Canonical Conversation 只追加，这批消息的归属此后无法回填。

Step 6 落地时在 `Service.Post` 加这条校验（按 `Kind` 立规矩，与现有的按 `SenderID`
那条并存），并同时定下 system 消息的 sender 语义。见未决 #14。

## internal/store

只有这个包认识 SQLite。

```go
package store

// Open 打开（必要时创建）dataDir 下的 council.db，跑完待应用的迁移，
// **校验真实表结构与当前版本应有的形状一致**，返回可用的 Store。
// dataDir 不存在时会创建（os.MkdirAll）—— 这是相对 Step 4 的一处有意改变。
func Open(ctx context.Context, dataDir string) (*Store, error)

// ErrSchemaMismatch 表示库里的表结构与当前代码期望的不一致。
var ErrSchemaMismatch = errors.New("schema does not match the expected shape")

func (s *Store) Close() error

// Path 返回库文件的绝对路径，给启动横幅用。
func (s *Store) Path() string
```

### 迁移之后必须校验 schema 形状

`migrate` 只比较 `schema_migrations` 的 `MAX(version)`，没有漂移探测。
只靠它的话，旧版二进制建出来的库记着 `version = 1`、表结构却是旧的，
`Open` 会**成功**、启动横幅照常打印，直到第一次读或写才报「no such column」；
将来加了 v2 之后，这种坏库还会被盖上「已是最新」的章，从版本号上再也分不出来。

所以每一版迁移**自带**「这一版跑完后每张表应当长什么样」，作为 `migration` 结构体的一个
字段，而不是另起一个全局表 —— 期望值必须紧挨着产生它的 DDL，隔开几百行一定会漂移。
`migrate` 跑完后按最后一版的形状逐表比对 `pragma_table_info`，
比 **name、type、notnull 三项并且按 `cid` 顺序**。只比列名不够：旧库可能只在
`NOT NULL` 上不一样。`CHECK` 约束在 `table_info` 里看不见，不要求比对。

不一致时返回包了 `ErrSchemaMismatch` 的错误，信息必须说清三件事：**哪张表、
缺了或多了什么、以及处置**（这个库是旧版二进制建的，删掉重建）。
看到这条错误的人应当就此结束排查，而不是开始排查。

`*Store` 必须满足 `conversation.Store`。在包里放一行编译期断言：

```go
var _ conversation.Store = (*Store)(nil)
```

### DSN 与连接池：逐字照抄，不要改

这两串是探针实测出来的（`reference/sqlite-probe-results.md`，D-10），
**改动其中任何一项都会让并发写大面积 `database is locked`，或者慢两个数量级**：

```go
// 写句柄
writeDSN := "file:" + filepath.ToSlash(dbPath) +
    "?_pragma=journal_mode(WAL)" +
    "&_pragma=busy_timeout(5000)" +
    "&_pragma=foreign_keys(1)" +
    "&_pragma=synchronous(NORMAL)" +
    "&_txlock=immediate"
writeDB.SetMaxOpenConns(1)

// 读句柄：同样的 pragma，但不带 _txlock（读事务就该是 DEFERRED）
readDSN := "file:" + filepath.ToSlash(dbPath) +
    "?_pragma=journal_mode(WAL)" +
    "&_pragma=busy_timeout(5000)" +
    "&_pragma=foreign_keys(1)" +
    "&_pragma=synchronous(NORMAL)"
readDB.SetMaxOpenConns(4)
```

三条各自挡一个坑，缺一不可：

1. pragma 写进 DSN 而不是 `db.Exec("PRAGMA ...")`。后者只作用于当时那条连接，
   池里新建的连接没有。实测 320 次并发写只成功 37 次。
2. `_txlock=immediate`。`BeginTx` 默认 `BEGIN DEFERRED`，事务先以读身份开始、
   要写时才升级；两个都持读快照的事务同时升级会直接 `SQLITE_BUSY`，
   **那是死锁不是排队，`busy_timeout` 等多久都没用**。
3. 写句柄 `SetMaxOpenConns(1)`。不限池也能对（因为有 `_txlock`），但慢 9 倍，
   多条连接互相排队等写锁纯属浪费。

### 追加消息的形状：照抄

探针轮 4 验过 1280 次并发写零撞号：

```go
var next int64
if err := tx.QueryRowContext(ctx,
    `SELECT COALESCE(MAX(seq),0)+1 FROM messages WHERE conv_id = ?`, convID,
).Scan(&next); err != nil {
    return conversation.Message{}, fmt.Errorf("分配 seq: %w", err)
}
```

### 其它硬性说明

- 时间一律以 **UTC** 存，格式 `time.RFC3339Nano`。读出来用 `time.Parse` 还原。
- id 用 `crypto/rand` 取 16 字节转十六进制（32 字符）。**不要引入 uuid 依赖** ——
  `github.com/google/uuid` 确实已经在依赖树里，但那是 `modernc.org/sqlite` 的传递依赖，
  直接用等于把别人的传递依赖当成自己的 API。
- `Recipients`、`Pending` 存 JSON 文本；nil 与空切片都要存成 `[]`，读回来是空切片**不是 nil**。
- `Meta` 存 JSON 对象；nil 存成 `{}`，读回来是空 map 不是 nil。
- `WriteTx` 必须用 `defer` 保证 panic 时也回滚，然后把 panic 继续抛上去。
- 唯一允许新增的依赖是 `modernc.org/sqlite`。除它与标准库外不得引入任何包。

## internal/app 与 internal/repl 的改动

```go
// Options 新增一个字段，其余不变。
type Options struct {
    ConfigPath string
    Version    string
    In         io.Reader
    Out        io.Writer
    Err        io.Writer
}
```

`app.Run` 的新顺序见规格 §I。横幅多一行：库文件路径与已有会话数。
store 打不开 -> 错误写 `Err`、返回错误、不进 REPL。退出时 `Close`，
关闭失败只往 `Err` 记一行，不改退出码。

```go
// repl.New 增加一个参数。
func New(cfg *config.Config, svc *conversation.Service, in io.Reader, out io.Writer) *REPL
```

`svc` 为 nil 时，八个新命令一律回一句「持久化未就绪」，**不得 panic** ——
和现有的 `cfg == nil` / `in == nil` / `out == nil` 兜底保持一致的风格。

### 裸文本即发言（行为契约）

REPL 对一行输入的处置，按这个顺序（前两条是闸门，任何模式下都先过）：

| 输入 | 行为 |
|---|---|
| 非法 UTF-8 | 拒绝，不入库。多行模式下标记草稿作废、继续吃后续行，不发送 |
| 正处在多行输入模式 | 见下一节，只有两个终止行被解释，其余原样进草稿 |
| 空行（去首尾空白**与不可见字符**后为空） | 什么都不做，不发言、不报错 |
| 以 `/` 开头 | 查命令表；查不到提示未知命令 |
| **其余（非空、不以 `/` 开头）** | **以 `ReservedSenderUser` 身份发言，等同 `/say <原文>`** |

「去首尾空白**与不可见字符**」是硬性的：只用 `TrimSpace` 的话，
`unicode.IsSpace` 认不出 U+FEFF 与零宽字符，带 BOM 的 `/join` 会绕过 `/` 前缀判断
变成一条不可回收的发言。至少要覆盖 U+FEFF 与 U+200B/200C/200D 这四个。

硬性要求：裸文本走的**必须是与 `/say` 同一套前置条件与同一个处理函数**。
不得为它另写一份「持久化就绪吗」「选会话了吗」的判断 —— 两份判断一定会走失同步，
出现「`/say` 拦住了而裸文本没拦住」这种只在某一条路径上暴露的洞。
实现手段不限（共用表项、共用函数都行），但**测试必须锁住两条路径行为一致**。

`/say` 因此获得第二个用途：**转义出口**。要发一句以 `/` 开头的字面量就写 `/say /foo`，
发出去的正文是 `/foo`。这是 `/say` 在裸文本自动发言之后仍然保留的唯一理由。

已知并接受的代价：Canonical Conversation 只追加、不回收，手滑敲错的一行也会成为永久记录。

### 多行输入模式（行为契约）

`/say` **不带参数**进入多行输入模式。模式内只有两个终止行被解释：

| 行 | 行为 |
|---|---|
| `/end` | 结束，把收到的行用 `\n` 拼成**一条**消息发送 |
| `/cancel` | 结束，丢弃，不发送 |
| 其余任何行 | **原样**进草稿：不去首尾空白，不解释斜杠命令 |

硬性要求，测试必须锁住：

- 三行输入经 `/end` 是**一条**消息，正文保留换行
- 草稿里以 `/` 开头的行是正文，**不得**被当成命令执行
- `/cancel`、只有空白的草稿、输入流结束时还攒着草稿 —— 三者**一条都不落库**
- 进入模式与 `/say <文本>` 共用同一套前置条件（没选会话时进不去）

终止符的具体字面量不属于契约面（措辞历来不属于），但**不得选一个在粘贴的散文里
常见的字符串** —— 那会把消息拦腰截断，而截断后的剩余行退回单行模式变成一条条消息，
正是这个模式要消灭的问题。

**已知限制（写进契约，免得被当成 bug）**：不先敲 `/say` 就直接粘贴，仍然一行一条。
本模式是显式的，没有任何粘贴检测。根治要做括号粘贴或输入缓冲，见未决 #12。

### 发言归属的渲染（行为契约，措辞与排版不在契约内）

- `/log` 必须显示每条消息**当时记录的**脑子（来自 `Message` 的三个字段）
- `/who` 必须显示每个成员**配置里当前的**脑子（来自 `config.AgentConfig`）

两者**必须能被用户区分开**：它们本来就会不一致（消息记的是历史，配置是现状），
把它们渲染成同一个样子、让人以为是同一件事，正是这次改动要避免的陷阱。
列名、分隔符、空值占位符怎么写不在契约内，**「两处各自取自哪个数据源」在契约内**。

配置里已经没有的 agent（被删掉但仍是旧会话的成员）在 `/who` 里的脑子列显示为空占位，
不报错 —— `/leave` 的注释已经说明这种成员必须仍然可见、可操作。

## 不属于契约面的东西

实现可以自由决定，**测试不得对它们做字面断言**：

- REPL 提示符、告别语、各种提示的具体措辞
- 表格的列名、列序、对齐、空字段占位符
- 启动横幅的具体排版（但必须含库文件路径）
- 斜杠命令是否区分大小写（沿用 Step 4：区分）
- 错误信息的具体中英文措辞（但必须能被 `errors.Is` 认出哨兵）
- id 的具体长度与字符集（只要求全局唯一且不含空白）

## 符号禁令（写代码时必须遵守）

代码文件里（含注释、字符串字面量、打印输出、标识符）禁用装饰性符号：
Emoji U+1F300-U+1FAFF、Dingbats U+2700-U+27BF、杂项符号 U+2600-U+26FF、
杂项符号与箭头 U+2B00-U+2BFF、带圈字母数字 U+2460-U+24FF、U+FE0F、U+200D。

放行：希腊字母、数学算符 U+2200-U+22FF、度数正负乘除微符号、上下标、
普通箭头 U+2190-U+21FF、中文、ASCII 制表符。
注释写中文可以，写 `输入 -> 输出` 可以，写带对勾的完成标记不可以。

原因：本机控制台默认 GBK，打印 emoji 会崩或乱码，落进日志后污染 grep 与 diff。
