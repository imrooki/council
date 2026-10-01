# SQLite 持久化探针实测（2026-09-20，E 盘机）

Step 5 动手之前跑的四轮探针。脚本在 `E:\Isolating_test\council\sqlite-probe\`，
每轮一个 `main.go`（`main.go` / `v2` / `v3` / `v4`），带自己的 `go.mod`，可重跑。

环境：go1.27.0 windows/amd64，`CGO_ENABLED=0`，`modernc.org/sqlite v1.59.0`。

统一负载：8 个 goroutine 各写 40 行，共 320 次尝试。每次写是一个事务：
`SELECT COALESCE(MAX(seq),0)+1` 算出下一个 seq，再 INSERT。
表上有 `UNIQUE (conv_id, seq)`，所以撞号会变成可见的错误而不是悄悄写坏。

## 轮 1：纯 Go 驱动能不能用

能。开库、建表、WAL、外键、读写、关掉重开数据还在，全部 OK。
`journal_mode` 读回 `wal`，`foreign_keys` 读回 `1`。

但并发写 **320 次只成功 47 次，273 次 `database is locked`**，
尽管代码里明明执行过 `PRAGMA busy_timeout = 5000`。

## 轮 2：四种配置对照

| 配置 | 成功/320 | busy | 撞号 | 耗时 |
|---|---|---|---|---|
| A 基线：`PRAGMA` 走 `db.Exec`，连接池不限 | 37 | 283 | 0 | 25 ms |
| B DSN 里写 `_pragma=...`，连接池不限 | 80 | 240 | 0 | 7 ms |
| **C DSN `_pragma` + `SetMaxOpenConns(1)`** | **320** | **0** | **0** | **14 ms** |
| D 只 `SetMaxOpenConns(1)`，不写 DSN pragma | 320 | 0 | 0 | **2493 ms** |

两条结论：

1. **`PRAGMA` 经 `db.Exec` 设置只作用于连接池里当时那一条连接。**
   `database/sql` 会按需新建连接，新连接没有那些 pragma。
   A 组之所以几乎全挂，就是因为大多数写落在没有 `busy_timeout` 的连接上。
   要让每条连接都带上，必须写进 DSN 的 `_pragma=`。
2. **D 组慢 178 倍，因为它的 `journal_mode` 退回了 `delete`。**
   WAL 没开起来时每次提交都要重写回滚日志。所以「串行化」和「开 WAL」是两件事，
   缺一不可：单连接负责消除竞争，WAL 负责让它别慢成那样。

## 轮 3：busy 的真正成因，以及读会不会被写堵住

| 组 | 配置 | 结果 |
|---|---|---|
| E | DSN `_pragma` + 池不限 + **手写 `BEGIN IMMEDIATE`** | 320/320，0 busy，85 ms |
| F | 写句柄单连接 + 读句柄连接池，读写同时跑 | 写 320/320，18 ms；**同期 3787 次读，0 失败** |

E 组解释了轮 2 的 B 组为什么还有 240 次 busy：
`database/sql` 的 `Begin` 默认发 `BEGIN DEFERRED`，事务先以读身份开始，
真要写时才升级成写事务。两个已经各自持有读快照的事务同时想升级，
SQLite 直接返回 `SQLITE_BUSY` —— **这是死锁，不是排队，`busy_timeout` 等多久都解不开**。
改成一上来就取写锁的 `BEGIN IMMEDIATE`，冲突退化成可以等待的普通竞争，busy 归零。

F 组证明 WAL 下读拿的是快照，不被写阻塞：写满负荷跑的同时，
4 个读 goroutine 转了 3787 次，一次没失败。所以读写分两个句柄是有实际收益的，
不必让读跟着写一起串行。

## 轮 4：能不能用标准的 `db.BeginTx` 拿到 IMMEDIATE

能，靠 DSN 参数 `_txlock=immediate`。业务代码写法完全不变，
仍然是 `BeginTx` / `tx.Query` / `tx.Exec` / `tx.Commit`，不用像轮 3 那样在裸 `*sql.Conn` 上
手写 `BEGIN` / `COMMIT` / `ROLLBACK`。

| 组 | 配置 | 成功/320 | busy | 耗时 |
|---|---|---|---|---|
| G | `BeginTx` 默认（DEFERRED）+ 池不限 | 68 | 252 | 6 ms |
| H | `BeginTx` + `_txlock=immediate` + 池不限 | 320 | 0 | 134 ms |
| **I** | **`BeginTx` + `_txlock=immediate` + 单连接** | **320** | **0** | **15 ms** |

H 与 I 都正确，但 I 快 9 倍：池不限时多条连接互相排队等写锁，
纯属白白让操作系统调度；写本来就只能一条一条来，不如在连接池层面就说清楚。

## 最终配置（已写进 `00_DECISIONS.md` D-10）

```
写句柄 DSN:
file:<绝对路径>?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)
  &_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate
写句柄: SetMaxOpenConns(1)

读句柄 DSN: 同上，但不带 _txlock（读事务就该是 DEFERRED）
读句柄: SetMaxOpenConns(4)
```

`seq = MAX(seq)+1` 在这套配置下四轮共 1280 次并发写，**一次都没撞号**，
且 seq 连续无空洞。配合 `UNIQUE (conv_id, seq)` 作为结构性兜底，足够 Step 5 使用。

## 这套结论的边界

- **只覆盖单进程。** 四轮探针都是一个进程内的多个 goroutine。
  两个 council 进程开同一个库文件当时没测过，标了 `[未验证]`；后来测了
  （未决 #8，原件 `E:\Isolating_test\council\oq8-multiproc\RESULTS.md`），
  结果与这里原来的推测相反：`BEGIN IMMEDIATE` + `UNIQUE` 约束**没有**让第二个进程
  响亮地失败——它静默打开、静默写入、退出码 0，两次入场费、一条孤儿 provider 会话、
  游标错位，四条损坏全部发生在没有任何 SQLite 冲突的前提下。互斥本来就不在
  SQLite 这一层，D-10 与未决 #8 已经订正、并加了 `data_dir` 级的 OS 排他文件锁
  （`internal/store/lock_windows.go` / `lock_other.go`）。
  Step 5 的设计前提仍是单写进程，只是「第二个写进程会怎样」这句话已经改成了实测结论。
- 没有测大数据量下的表现（这里每轮只有 320 行）。
- 没有测 `-race`：本机 `CGO_ENABLED=0` 且没装 gcc，race 检测跑不起来。

## 代价：依赖从 1 个变成 11 个

`modernc.org/sqlite` 拉进来 10 个传递依赖（`modernc.org/libc`、`modernc.org/memory`、
`golang.org/x/sys`、`github.com/google/uuid` 等），`go build` 产物从约 3 MB 涨到 10 MB。
这是纯 Go 驱动的固有成本 —— 它把整个 SQLite 用 C-to-Go 转译了进来。
换成 cgo 的 `mattn/go-sqlite3` 能省掉这些，但要装 gcc、要开 `CGO_ENABLED=1`、
交叉编译会变麻烦，与 D-8「单静态二进制、无运行时依赖」的理由冲突。维持 D-8 的选择。
