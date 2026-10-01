# Step 5 规格：Canonical Conversation 与动态 participant

Step 4 交付的是一个能读配置、能列出配置的壳。Step 5 给它装上**本地事实源**：
一个 SQLite 库，能建会话、加人、退人、写消息、按游标读、重启后恢复。

**仍然不调用任何 provider，不起任何子进程。** 消息由人在 REPL 里敲进去，
agent 说话是 Step 6 的事。本步要保证的是：等 Step 6 真的把 agent 的回复接进来时，
写进去的地方已经是对的。

前提：`00_DECISIONS.md` 的 D-9（数据目录）与 **D-10（SQLite 的打开方式，实测定的）**。
实测证据在 `reference/sqlite-probe-results.md`，不要重新推导，也不要"优化"掉 D-10 的任何一条。

## A. 这一步真正的难点是事务边界，不是表

`05_MESSAGE_PROTOCOL.md` §5 列了六张表和一串操作，但那个列表会误导人：
照着它写，很容易做出一套「每张表一组 CRUD」的 API，然后在 Step 7 发现
**一轮 agent 调用要同时写四张表且必须原子**，只好把 `*sql.Tx` 漏到 store 外面，
抽象当场作废。

所以本步的设计以**原子单元**为中心：

| 原子单元 | 一个事务里做什么 | 什么时候需要 |
|---|---|---|
| 追加消息 | **确认 sender 仍是活跃 participant** + 算 `seq = MAX(seq)+1` + 插入 messages | Step 5 |
| 加入 participant | 插入，或把之前退出的那条重新激活 | Step 5 |
| 退出 participant | 软删除：`active=0` + `left_at`，**不动任何消息** | Step 5 |
| 写游标 | upsert `cursors` 的 `seq` 与 `pending` | Step 5 |
| **记一轮** | 消息 + 推游标 + `turn_usage` + `provider_sessions`，**四张表一个事务** | Step 7 |

最后一行现在不实现，但**接口形状必须现在就能容纳它**。做法见 §E：
store 对外暴露一个 `WriteTx`，所有写操作都是 `*Tx` 上的方法；
`Store` 上的同名方法只是「开一个事务、调一次、提交」的薄封装。
Step 7 要的是在 `*Tx` 上加两个方法，不是重构。

## B. 包结构

```
internal/conversation/    领域层，不认识 SQL
  types.go                Message / MessageKind / Participant / Conversation / Cursor
  store.go                Store 接口（消费方定义的接口，由 internal/store 实现）
  service.go              Service：动态 participant、投递对象解析、未读查询
internal/store/           持久化层，只这一个包认识 SQLite
  open.go                 Open / Close / DSN 构造 / 双句柄
  schema.go               DDL 与迁移
  tx.go                   WriteTx 与 *Tx 上的写方法
  read.go                 读方法
```

`internal/store` 导入 `internal/conversation` 取类型；**反向不导入**。
接口定义在 `conversation` 里（Go 的惯例是消费方定义接口），
这样领域层可以在测试里换一个内存实现，不必拖着 SQLite 跑。

## C. 领域类型

以 `05_MESSAGE_PROTOCOL.md` §2 为准，补上本步定下的细节：

```go
type MessageKind string   // 不是 int，理由见 §D

const (
    KindUser       MessageKind = "user"
    KindAgent      MessageKind = "agent"
    KindSystem     MessageKind = "system"
    KindTask       MessageKind = "task"
    KindTaskResult MessageKind = "task_result"
)
```

`SenderID` 是 participant id。**`"user"` 是保留 participant**（`05` §1），
它不在 `participants` 表里也能发消息 —— 人不是 agent，不需要 join 才能说话。

## D. Schema

一次建齐 `05` §5 列的**六张表**，即使 Step 5 只有其中四张有方法。
理由：`turn_usage` 与 `provider_sessions` 的形状 `05` 已经定好了，
现在建掉，Step 6 与 Step 7 就不用写 DDL 迁移，只写方法。
迁移机制照样要有（见下），将来真要改结构时用得上。

### 类型约定，逐条有理由

| 决定 | 怎么存 | 为什么 |
|---|---|---|
| 时间戳 | `TEXT`，RFC3339Nano，**UTC** | 人用 SQLite 查看器打开就能读；字典序即时间序；不踩时区 |
| `messages.kind` | `TEXT` + `CHECK` 约束五个值 | 存整数更省，但这是个本地优先的工具，用户会自己开库看。`"agent"` 比 `2` 有用得多 |
| `messages.meta` | `TEXT`，JSON 对象，默认 `'{}'` | `05` §2 的 `map[string]any`。不建 EAV 表，这东西只整读整写 |
| `messages.thought` | `TEXT NOT NULL DEFAULT ''` | `04` §3：**默认存但默认不路由**。Step 5 无条件存，开关是 Step 8 的事（未决 #9） |
| `messages.recipients` | `TEXT`，JSON 字符串数组，默认 `'[]'` | 空数组 = 广播给活跃成员（`05` §2）。同样只整读整写 |
| `cursors.pending` | `TEXT`，JSON 的 int64 数组，默认 `'[]'` | 另建一张 `cursor_pending` 表更"正规"，但 pending 永远是按 (conv, agent) 整体读写的，拆表只会在同一个事务里多一次写，换不来任何查询能力。**若将来要按 seq 反查谁欠着，再拆不迟** |
| id | 16 字节 `crypto/rand` 的十六进制，32 字符 | 不引入 uuid 依赖。`modernc.org/sqlite` 的依赖树里虽然已经有 `github.com/google/uuid`，但那是它的传递依赖，直接用等于把别人的传递依赖当自己的 API |
| `messages.sender_provider` / `sender_model` / `sender_effort` | 三列 `TEXT NOT NULL DEFAULT ''` | 见下面「发言归属」一节。**记在 messages 上而不是 participants 上**，这是本步唯一一处非平凡的建模决定 |

### 发言归属：脑子是消息的属性，不是 participant 的属性

用户的要求是「不同模型和思考程度，让用户知道具体是谁在说话」。
照直觉会把 `model` / `effort` 加到 `participants` 表上 —— 那是错的，而且错得不显眼。

D-4 与 `02` §4 定死了「换 model 或 effort 必须重建会话」（缓存前缀被打断，要重付冷启动价）。
所以同一个 agent 在一场对话里**完全可能换过脑子**。
若把脑子记在 participant 行上，换一次就把它**过去说过的每一句话**追溯性地重新标注成
新脑子说的 —— Canonical Conversation 是事实源，事实源不能这样被改写。

记在消息上，每条消息各自带着它当时的脑子；换脑子只影响之后的消息，历史不动。

`SenderID` 是保留 id `user` 的消息，这三列**必须为空**：人不是模型，没有脑子可记。
这条由两层保证 —— `conversation.Service.Post` 返回 `ErrInvalidSender`（友好、可 `errors.Is`），
表上的 `CHECK` 兜底（绕过 Service 的路径也写不坏）。

**两层拦的都是 `sender_id`，与 `kind` 无关。** 这句容易读反，所以两个方向都写全：

- `kind='user'` 但 sender 是某个 agent 的消息 —— **不受**这条约束，能带脑子
- `sender_id='user'` 配 `kind='agent'` 的消息 —— **一律受**这条约束，`kind` 是什么都照拦

第二条曾经在这份文档里被写反过（说成「也照样能带脑子写进去」），实测证伪：
`CHECK` 表达式里根本不出现 `kind` 这个列，`Service.Post` 那层走的也是
`m.SenderID == ReservedSenderUser` 分支。两层现在各有一条 `kind='agent'` 的用例钉着
（`store/brain_test.go` 与 `conversation/brain_test.go`），写反了会红。

**Step 6 若要按 `kind` 立规矩，得另加一层** —— 例如「`KindAgent` 必须带脑子」，
那是 `kind` 方向的规则，现有两层一条都管不着，见下面那段与未决 #14。

**HANDOFF 要求的「`system` 消息这三列也空」只做了一半，是有记录的推迟。**
本步只按 `sender_id` 立规矩，`kind='system'` 那一半没有任何一层拦。
理由：没有任何代码路径造得出 system 消息，而 system 消息的 sender 语义本身还没定
（它该挂在哪个 id 上？保留 id 还是那个 agent 的 id？）。
按 `kind` 立规矩要与那个语义一起定，推迟到 Step 6，记为未决 #14。
`CHECK` 里的 `'user'` 与 `conversation.ReservedSenderUser` 是同一个值，
和 `kind` 那条 `CHECK` 一样，属于「领域常量在 DDL 里出现第二次」的已知重复，接受。

**这三列记的是请求时声明的档位，不是实测生效的档位。** 会话 #1 已实证 grok 的 effort
由 `~/.grok/config.toml` 决定、命令行参数经 CPA 被静默丢弃（`02` §1.1 的
EffortPolicy「进程外配置」）。Step 6 的适配器只能把 `AgentConfig.Effort` 抄进来。
不要把这列当 ground truth，也不要据它去断言某一轮真的跑在哪个档位上。

**给 Step 6/7 的连带要求**：因换 model/effort 而重建会话时，必须追加一条 `system` 消息
记录这次切换（`04` §0 规则 3：重建是有标价的操作，要显式记录）。
否则 `/log` 里只看得到脑子变了，看不到什么时候变、为什么变。

### 表

```sql
CREATE TABLE schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE conversations (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE participants (
    conv_id      TEXT NOT NULL REFERENCES conversations(id),
    agent_id     TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    provider     TEXT NOT NULL DEFAULT '',
    active       INTEGER NOT NULL DEFAULT 1,
    joined_at    TEXT NOT NULL,
    left_at      TEXT,
    PRIMARY KEY (conv_id, agent_id)
);

CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    conv_id         TEXT NOT NULL REFERENCES conversations(id),
    seq             INTEGER NOT NULL,
    sender_id       TEXT NOT NULL,
    sender_provider TEXT NOT NULL DEFAULT '',
    sender_model    TEXT NOT NULL DEFAULT '',
    sender_effort   TEXT NOT NULL DEFAULT '',
    recipients      TEXT NOT NULL DEFAULT '[]',
    kind            TEXT NOT NULL CHECK (kind IN ('user','agent','system','task','task_result')),
    text            TEXT NOT NULL DEFAULT '',
    thought         TEXT NOT NULL DEFAULT '',
    parent_id       TEXT,
    created_at      TEXT NOT NULL,
    meta            TEXT NOT NULL DEFAULT '{}',
    UNIQUE (conv_id, seq),
    CHECK (sender_id <> 'user' OR (sender_provider = '' AND sender_model = '' AND sender_effort = ''))
);
CREATE INDEX idx_messages_conv_seq ON messages (conv_id, seq);

CREATE TABLE cursors (
    conv_id    TEXT NOT NULL REFERENCES conversations(id),
    agent_id   TEXT NOT NULL,
    seq        INTEGER NOT NULL DEFAULT 0,
    pending    TEXT NOT NULL DEFAULT '[]',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (conv_id, agent_id)
);

-- 下面两张 Step 5 只建表、不写方法。形状取自 05 §4 与 02 §3。
CREATE TABLE turn_usage (
    conv_id         TEXT NOT NULL REFERENCES conversations(id),
    agent_id        TEXT NOT NULL,
    seq             INTEGER NOT NULL,
    input_tokens    INTEGER NOT NULL DEFAULT 0,
    cache_create    INTEGER NOT NULL DEFAULT 0,
    cache_read      INTEGER NOT NULL DEFAULT 0,
    output_tokens   INTEGER NOT NULL DEFAULT 0,
    thinking_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd        REAL,
    cold_start      INTEGER NOT NULL DEFAULT 0,
    wall_ms         INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (conv_id, agent_id, seq)
);

CREATE TABLE provider_sessions (
    conv_id             TEXT NOT NULL REFERENCES conversations(id),
    agent_id            TEXT NOT NULL,
    provider_session_id TEXT NOT NULL DEFAULT '',
    last_turn_at        TEXT,
    cold                INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (conv_id, agent_id)
);
```

`cost_usd` 用 `REAL` 且**可空** —— `05` §3：只有 claude 报金额，而且那是会话累计值，
要存的是差值。可空表示"这家 provider 根本没有这个数"，不是 0。

### 迁移

`schema_migrations` 一张表记版本号。`Open` 时在**一个 `BEGIN IMMEDIATE` 事务里**
把未应用的迁移按序跑完并记录版本。Step 5 只有 version 1（上面全部 DDL）。

**v1 被就地修订过一次，没有 v2。** 加 `messages` 那三列时不存在任何真库，
v1 从未在真库上生效，所以改的是初始 schema 本身，不是加一版迁移。

前提是**按 D-9 的三级优先级逐级核实**过的，不是只看了默认路径：
`data_dir`（`~/.council/config.toml` 不存在，`COUNCIL_CONFIG` 未设）>
`COUNCIL_DATA_DIR`（进程级、User 级、Machine 级三个作用域全未设置）>
默认路径（`E:/D:/C:/F:\council-data` 四个都不存在）。
全机递归搜索 `council.db` 只有一个，在隔离测试目录里，且已是新 schema。
**以上只在 E 盘机（`COMPUTERNAME = LONG`）核实过；D 盘机没有 E 盘、`data_dir` 覆盖是必须项，
那台机器上的结论按 `[未验证]` 处理。**

**代价**：`739348f` 那一版代码建出来的库记着 `version = 1` 却是旧的十一列 `messages`。
新二进制打开它不会重跑迁移，`Open` 会**成功**、启动横幅照常打印，
直到第一次读或写才失败 —— **读也会炸**，不只是写：`Messages` 的 `SELECT`
显式列了那三列，`/log` 同样报未知列。那种库一律删掉重建，**不要把它当 bug 排查**。

**曾经还有一层更隐蔽的，现在堵上了**：`migrate` 只比较 `MAX(version)`，
本身没有漂移探测；将来加了 v2 之后，这种旧库会被 v2 盖上「已是最新」的章，
从版本号上再也和健康的库分不开，而且此后任何迁移都不会回头补那三列（原未决 #10）。

所以每一版迁移现在**自带**「这一版跑完后每张表应当长什么样」，作为 `migration`
结构体的一个字段，与产生它的 DDL 放在一起 —— 不另起一个全局表，
那种写法隔开几百行一定会与 DDL 走失同步。`Open` 在迁移成功之后按最后一版的形状
逐表比对 `pragma_table_info`，比 **name、type、notnull 三项并且按 `cid` 顺序**。
只比列名不够：旧库可能只在 `NOT NULL` 上不一样。`CHECK` 约束在 `table_info` 里看不见，
不比对它（那一条另有直插用例守着）。

不一致时 `Open` **当场失败**，返回包了 `ErrSchemaMismatch` 的错误，
信息里点名哪张表、缺了或多了什么、以及处置。看到它的人应当就此结束排查。

下一次改 schema 时若真库已经存在，就必须老老实实加 v2 —— 并且记得给它写 `shape`，
不写的话 `TestSchemaShapeCoversEveryTable` 会红。

迁移必须幂等地判断：读 `MAX(version)`，只跑比它大的。
库文件不存在时 `Open` 负责**创建数据目录**（`os.MkdirAll`）——
注意这是对 Step 4 的一处改变：Step 4 明确不创建任何目录，Step 5 开始创建，
因为从这一步起程序真的要往那里写东西了。

## E. 原子写单元（本步的核心接口）

```go
// WriteTx 把 fn 里的所有写操作放进同一个 BEGIN IMMEDIATE 事务。
// fn 返回非 nil 就整体回滚。fn 内部只许用 *Tx 上的方法，不许自己拿连接。
func (s *Store) WriteTx(ctx context.Context, fn func(context.Context, *Tx) error) error
```

`*Tx` 上的方法就是 §A 那张表的前四行。

这里原先还写着「`Store` 上有同名的便捷方法」，那是假的：`*Store` 上只有
`Open` / `Close` / `Path` / `WriteTx` 与七个读方法，从来没有过
`CreateConversation` / `AppendMessage` 这类非事务快捷入口，`08a` 冻结的 `Store` 接口里也没有。
写操作的唯一入口就是 `WriteTx`，上层的便捷封装在 `conversation.Service` 那一层。
照原话去写 `st.AppendMessage(...)` 会编译不过。

真正要守的是那半句：同一件事**不得另写一套 SQL**，两份 SQL 一定会走失同步。

Step 7 要加「记一轮」时，加的是 `(*Tx).RecordUsage` 与 `(*Tx).SetProviderSession`，
然后在一个 `WriteTx` 里把四件事串起来。本步不写这两个方法，但要留下这条路。

### 追加消息的实现形状（照抄，不要自己发明）

这段是探针轮 4 验过的形状，1280 次并发写零撞号：

```go
var next int64
err := tx.QueryRowContext(ctx,
    `SELECT COALESCE(MAX(seq),0)+1 FROM messages WHERE conv_id = ?`, convID).Scan(&next)
// ... 然后 INSERT，seq 用 next
```

能这么简单，靠的是 D-10 的三条配置同时生效（DSN pragma + `_txlock=immediate` + 写句柄单连接）。
**任何一条被拿掉，这段就会大面积 `database is locked`。** 不要"优化"。

`UNIQUE (conv_id, seq)` 是结构性兜底：真出现撞号，是响亮的错误，不是静默写坏。

## F. 读操作

- 按 id 取会话；列全部会话（按 `created_at` 倒序）
- 列 participant，可选是否含已退出的
- 取消息：`(convID, afterSeq, limit)`，**按 seq 升序**，这是 `04` §1 的游标读法
- 取某会话的最大 seq
- 取游标；列某会话的全部游标

读走读句柄，不进 `WriteTx`。

## G. 动态 participant 的语义

1. **加入已存在但已退出的 agent = 重新激活**，不是插入第二行。
   `active=1`、`left_at=NULL`、`joined_at` 更新为本次加入时间。
   主键是 `(conv_id, agent_id)`，本来也插不进第二行。
2. **退出是软删除，永不删消息**（`05` §5 的硬性要求）。
   退出后它说过的话仍在 `messages` 里，仍能被读出来。
3. **`"user"` 不是 participant**，不需要 join，也不出现在 `/who` 里。
   但它可以是 `sender_id`。
4. 广播的收件人解析：`Recipients` 为空 = 投给**当前活跃的** participant。
   注意「当前」是读的时候算的，不是写的时候固化的 —— 消息行里存的仍是空数组。
5. 给一个不存在的 agent 发定向消息：**报错**，不要静默丢弃。

## H. REPL 集成

Step 4 的五个命令保留，新增八个。新增的都要能在没有任何 provider 的情况下跑通：

| 命令 | 行为 |
|---|---|
| `/new <标题>` | 建会话并切为当前会话 |
| `/conv` | 列出全部会话，标出当前那个 |
| `/use <id>` | 切当前会话；id 可用前缀匹配，不唯一就报错并列出候选 |
| `/join <agent-id>` | 把配置里的某个 agent 加进当前会话；配置里没有就报错 |
| `/leave <agent-id>` | 退出（软删除） |
| `/who` | 列当前会话的 participant，含 `active` 状态、加入时间、**配置里当前的脑子** |
| `/say <文本>` | 以 `user` 身份追加一条消息。裸文本已经能发言，所以它现在的主要用途是**转义出口** |
| `/log [n]` | 按 seq 升序打印最近 n 条（默认 20），显示 seq、sender、kind、**消息当时记录的脑子**、文本 |

没有当前会话时，除 `/new` `/conv` `/use` 外都提示「先 /new 或 /use 选一个会话」。
**`/log` 不显示 `thought`** —— 存了不等于要往人脸上糊，`04` §3 的立场是默认保守。

### 裸文本即发言

一行输入的处置顺序：空行什么都不做；以 `/` 开头查命令表；**其余一律以 `user` 身份发言**。

Step 4 时最后那一档是打印一句「尚不支持对话」。Step 5 有了事实源，那句话既过时又指错文档，
本步删掉（连同 `07_STEP4_SPEC.md` F 节那条规则一并作废，该文件已加前向注记）。

硬性要求：**裸文本必须与 `/say` 共用同一套前置条件与同一个处理函数**。
不得为它另写一份「持久化就绪吗 / 选会话了吗」的判断 —— 两份判断一定会走失同步，
出现只在某一条路径上暴露的洞。测试要锁住两条路径行为一致。

`/say` 保留了两个用途：转义（`/say /foo` 发出去的正文是字面量 `/foo`），
以及**不带参数时进入多行输入模式**，见下一节。

代价，已当面告知用户并接受：Canonical Conversation 只追加、不回收，
手滑敲错的一行也会成为永久记录。

### 多行输入模式

「裸文本即发言」对**手敲**的输入是对的，对**粘贴**的输入是错的：读循环按行切，
粘贴一段三行的需求会变成三条各自独立的永久消息，而 Canonical Conversation 只追加不回收。
用户接受的代价是「手滑一行成永久记录」，不是「粘贴三段变三条」。

所以 `/say` 不带参数进入多行输入模式：

| 多行模式下的一行 | 行为 |
|---|---|
| `/end` | 结束，把收到的行用换行拼成**一条**消息发出去 |
| `/cancel` | 结束，丢弃，不发送 |
| **其余任何行** | **原样**进草稿：不去首尾空白，不解释斜杠命令 |

「其余任何行原样进草稿」是这个模式的全部意义 —— 粘贴进来的东西不该被再解释一遍。
草稿里的 `/join` 是正文，不是命令；缩进是正文的一部分。

终止符用 `/end` 而不是常见的单个句点，是因为这个模式的意义就是粘贴安全：
粘贴来的散文里出现一行孤立的句点完全可能，那会把消息拦腰截断，
而截断之后的剩余行会退回单行模式、被一条条当成新消息 —— 正是本模式要消灭的问题。

**不发送的三条路径**，都必须一条消息都不落库：`/cancel`；草稿只有空白；
以及输入流结束或 Ctrl+C 时还攒着草稿（退出时明说丢了）。
把一份用户从没确认过的草稿写成永久记录，比丢掉它糟得多。

**已知限制**：不先敲 `/say` 就直接粘贴，仍然是一行一条。多行模式是显式的，
没有任何粘贴检测。要根治得做括号粘贴（bracketed paste）或输入缓冲，那是另一块工程，
见未决 #12。这条限制有用例钉着（`TestBareTextLinesStillPostSeparately`），
免得有人以为多行模式把裸文本也改成了缓冲。

### 入口的两道闸门

裸文本成为主要输入路径之后，两类脏输入会直接落进事实源，两道闸门都在 `dispatch` 最前面：

1. **非法 UTF-8 一律拒绝，不入库。** SQLite 的 `TEXT` 不校验编码，而 Step 6 把历史拼成
   JSON 送给 provider 时 `json.Marshal` 会把坏字节换成 U+FFFD 且**不报错** ——
   于是库里存的、屏幕上显示的、agent 实际看到的三者永久不一致。宁可让用户重敲一行。
   多行模式下不能只丢坏的那一行（会把粘贴悄悄截去一截），也不能就此退出模式
   （剩余行会退回单行模式变成一条条消息），所以是：标记草稿作废、继续吃后续行、
   等用户显式 `/end` 或 `/cancel`，任何情况下都不发送。

2. **首尾的不可见字符与空白一起剥掉。** `TrimSpace` 认不出 U+FEFF 与零宽字符
   （`unicode.IsSpace` 对它们全是 false），于是一个带 BOM 的 `/join` 过不了 `/` 前缀判断，
   落进裸文本分支变成一条不可回收的发言，而屏幕上打的是「已记下 seq N」这种正常成功行，
   扫一眼发现不了。本机 PowerShell 5.1 用管道喂 stdin 时会在流首插 BOM，是已实证的高发场景。
   只收 BOM 与三个零宽字符这四个，不穷举所有不可见码位 —— 那会吃掉正文里合法的格式字符。

### `/log` 与 `/who` 的脑子取自不同数据源，这是有意的

- `/log` 的脑子来自 **`Message` 的三个字段** —— 那条消息**当时**的脑子
- `/who` 的脑子来自 **`config.AgentConfig`** —— 这个 agent **现在**配置成什么脑子

两者**本来就会不一致**：消息记的是历史，配置是现状；改一次配置，`/who` 立刻变，
`/log` 里的历史一条都不动（这正是把脑子记在消息上的目的）。
所以两处的列名要让人一眼看出不是同一件事，**不要渲染成同一个样子**。
把它们混为一谈，正是这次改动要消灭的陷阱。

配置里已经没有的 agent（被删掉但仍是旧会话的成员）在 `/who` 里脑子列显示占位符，不报错。

## I. 生命周期变化

`app.Run` 的顺序变成：解析配置路径 -> 加载校验 -> 解析数据目录 ->
**打开 store（含建目录与迁移）** -> 横幅（多一行显示库文件路径与已有会话数）->
起 REPL -> **退出时关闭 store**。

store 打不开时：错误写进 `Err`，退出码 1，不进 REPL。
关闭失败只记录不改退出码 —— 数据已经提交了，关闭失败不影响正确性。

## J. 测试矩阵

`go test ./...` 全绿。测试一律用 `t.TempDir()`，**不得碰真实的 `E:\council-data`**。

| 组 | 用例 |
|---|---|
| open | 新目录建库成功；目录不存在时会创建；重开已有库不重跑迁移；迁移版本号正确；只读目录或非法路径时报错且信息含路径 |
| schema | 六张表都在；`messages.kind` 的 CHECK 约束真的拦住非法值；`UNIQUE(conv_id,seq)` 真的拦住重复；外键真的拦住指向不存在会话的消息 |
| 消息 | 追加返回的 seq 从 1 开始且连续；按 `afterSeq` 读只返回更大的；升序；limit 生效；空会话读回空切片而不是 nil |
| **并发** | **8 个 goroutine 并发追加，全部成功、seq 连续无重复无空洞**。这条是 D-10 的回归测试，它红了说明有人动了 DSN 或连接池设置 |
| participant | 加入；重复加入不产生第二行；退出后 `active=0` 且 `left_at` 非空；**退出后它的消息仍读得到**；重新加入变回 `active=1` 且 `left_at` 清空；列表可选含已退出 |
| 游标 | 初始读不到；写后读回一致；`pending` 的 JSON 往返；同一 (conv, agent) 重复写是 upsert 不是插两行 |
| 事务 | `WriteTx` 里返回错误 -> 整体回滚，一行都没落库；`WriteTx` 里 panic 也要回滚而不是泄漏事务 |
| 重启 | 写入、关闭、重开、数据仍在（这是 `05` §5 的「重启后恢复」） |
| service | 广播收件人 = 当前活跃成员；定向给不存在的 agent 报错；`user` 不需要 join 就能发言 |
| repl | 八个新命令各一个输出断言；没有当前会话时的提示；`/use` 前缀不唯一时报错并列候选 |
| **发言归属** | 三列往返（写进去什么读回来什么）；sender 是 `user` 的消息三列为空；`Service.Post` 对带脑子的 `user` 消息返回 `ErrInvalidSender`；表上的 `CHECK` 真的拦得住绕过 Service 的写入；**两层对 `kind='agent'` 的 `user` 消息同样开火**（这一格曾被文档写反） |
| **裸文本** | 裸文本与 `/say <同一文本>` 行为一致（同一套前置条件、同一个结果）；`/say /foo` 发出字面量 `/foo` 而不是当命令；空行仍然什么都不做；没有当前会话时裸文本给的是「先选会话」而不是静默丢弃 |
| **渲染** | `/log` 显示的是消息里记的脑子；`/who` 显示的是配置里当前的脑子；同一个 agent 的消息脑子与配置脑子不同时，两处各自显示自己那一份（`TestLogAndWhoReadBrainFromDifferentSources`）；`oneLine` 把 tabwriter 的五种分隔符全部中和（`\t \n \r \v \f`） |
| **多行输入** | 三行粘贴经 `/say` + `/end` 成为**一条**消息且正文保留换行；草稿里的 `/join` 是正文不是命令、不产生成员；`/cancel`、只有空白、输入流结束三种情况一条都不落库；没选会话时 `/say` 被前置条件拦下、不会把人留在发不出去的模式里；**不先敲 `/say` 直接粘贴仍是一行一条**（已知限制，用例钉住） |
| **入口闸门** | 非法 UTF-8 单行模式下拒绝入库、多行模式下整份草稿作废且后续行不变成消息；带 BOM 或零宽前缀的 `/help` 与不带前缀的输出**逐字节相同**（断言不碰措辞） |
| **schema 形状** | 造一个旧版十一列 `messages` 的库、`schema_migrations` 记 `version=1`，`Open` 必须失败、错误能被 `errors.Is` 认出 `ErrSchemaMismatch` 且点名表与缺失列；对照组：新建库与重开正确的库都要成功；**shape 声明的表集合必须等于库里真实的表集合**（堵住「加了表却忘了声明」） |
| **事务内成员判定** | 判定语义：活跃通过、没加入过被拒、同名但在别的会话被拒、退出后被拒、重新加入又通过（最后两条必须成对，只验前者会漏掉「只查行存在不查 active」的实现）；**结构**：直接断言 `Post` 的调用序列是「`WriteTx` 打开 -> 判定 -> 追加消息」，并反向断言事务外不再调用 `Participants` —— 因为「退出的成员发不了言」在校验放事务内外两种实现下都成立，验不出区别 |

## K. 不在 Step 5 做

- 不调用任何 provider，不起子进程，不做 `--resume`
- 不实现 Context Router 的路由规则（`04` §2）—— 本步只提供游标的读写，不决定送什么
- 不实现 `@mention` 解析
- 不实现保留期清理 / 脱敏开关（`05` §6，见未决 #9）
- 不给 `turn_usage` 与 `provider_sessions` 写方法，只建表
- 不做 TUI
- 不支持同一个库文件上的第二个 council 进程（D-10 的边界，未决 #8）
