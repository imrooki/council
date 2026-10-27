package store

// 本文件是 SRV1 的只读入口。它与 Open 的差别不是为了省几行代码：
// 看板要在 TUI 正开着的时候读同一个库，而 Open 会在 data_dir 上抢
// council.lock、会跑迁移、会在目录不存在时建目录。这三件事对看板全是错的
// ——抢锁会让看板把 TUI 挤掉（或者自己被拒绝），迁移会让一个只读进程写库。
// 所以这里复制的是 Open 的「打开读句柄」这一半，另外两件事一件都不做。
//
// A3 放宽了版本门：旧库（库版本 ≤ 本二进制版本）现在允许只读打开。
// 只读进程绝不自动迁移：它不拿 council.lock，自动迁移会和其它进程抢写。
// 旧库缺的列由 probeReadOnlyShape 按 PRAGMA 探出来，读路径补零值；
// 库比二进制新时仍然拒绝（那说明有更新的进程写过，读它可能读到不认识的语义）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrReadOnlyDBMissing 表示 data_dir 或 council.db 还不存在。
// 看板没有「先建一个空库」这条路：建库要跑迁移，而本片不写库。
// 要能把这条错误和「库里没数据」分开，调用方才知道该让用户去终端跑一次 council。
var ErrReadOnlyDBMissing = errors.New("库文件不存在，先跑一次 council")

// ErrSchemaVersionMismatch 表示库的 schema 版本超出了本二进制能读的范围。
//
// 只读模式只拒绝两种：库比二进制新（actual > expected），以及版本号是 0
// （没有 schema_migrations 记录，这个文件不像是 council 建的）。旧库
// （actual < expected）现在允许只读打开，不再走到这条错误。
//
// 与 ErrSchemaMismatch 分开：那个说的是「表结构不对」，这个说的是「版本号
// 不对」。两条错误要求用户做的事不同——版本不对应当换对应的 council 版本，
// 而表结构不对应当先备份库；混成一个哨兵会让调用方只能猜。
var ErrSchemaVersionMismatch = errors.New("schema version mismatch")

// OpenReadOnly 以只读方式打开 dataDir 下的 council.db。
//
// 不建目录、不建库、不跑迁移、不碰 council.lock。于是 TUI 占着数据目录时，
// 看板仍然能读同一个库；库不存在时返回明确的错误而不是替用户建一个。
//
// 版本门只有一条：库版本 ≤ 本二进制版本。旧库放行，读完 probeReadOnlyShape
// 之后缺的列按零值读（见 readShape）；比二进制新的库仍然拒绝，因为那说明
// 有更新的进程写过，当前进程读不懂它新增的语义。库版本为 0（没有
// schema_migrations 记录）同样拒绝：那不是「旧库」，是一个不是 council
// 建的库，放进来只会让看板在每个读入口各报一次 no such table。
//
// 句柄本身用 SQLite 的 mode=ro 打开：即使本文件的调用方以后误走写路径，
// SQLite 也会在驱动层拒绝，而不是悄悄把看板变成一个写进程。
// SetMaxOpenConns 与 Open 的读句柄一致（4），因为这里就是那条读路径。
func OpenReadOnly(ctx context.Context, dataDir string) (*Store, error) {
	dbPath, err := filepath.Abs(filepath.Join(dataDir, dbFileName))
	if err != nil {
		return nil, fmt.Errorf("resolve db path %s: %w", filepath.Join(dataDir, dbFileName), err)
	}

	// 先自己判存在，而不是让 SQLite 在 mode=ro 下报「打不开」：
	// 只有这条前置判断能给出「先跑一次 council」这个下一步动作。
	if _, statErr := os.Stat(dbPath); statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			if _, dirErr := os.Stat(dataDir); errors.Is(dirErr, fs.ErrNotExist) {
				return nil, fmt.Errorf("只读打开 %s: %w（数据目录 %s 也不存在）", dbPath, ErrReadOnlyDBMissing, dataDir)
			}
			return nil, fmt.Errorf("只读打开 %s: %w", dbPath, ErrReadOnlyDBMissing)
		}
		return nil, fmt.Errorf("查看库文件 %s: %w", dbPath, statErr)
	}

	// 只读 DSN 三项：mode=ro 让 SQLite 拒绝对主库的写；query_only 再把
	// 「SQL 层允许但会写」的入口一起关掉（只读连接上 CREATE TEMP TABLE 与
	// ATTACH 本身是成功的，query_only 让它们同样被拒）；busy_timeout 让看板
	// 在 TUI 正提交一轮时等一会儿而不是立刻报 database is locked。
	// 不写 journal_mode=WAL：journal_mode 是库文件头的持久属性，只读连接
	// 也不该试图改它；库是 WAL 的话 SQLite 自己按 WAL 读。
	dsn := "file:" + sqliteURIPath(dbPath) +
		"?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("只读打开 %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(4)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接只读库 %s: %w", dbPath, err)
	}

	s := &Store{path: dbPath, read: db}
	version, err := s.validateReadOnlySchema(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("只读打开 %s: %w", dbPath, err)
	}
	s.schemaVersion = version
	s.expectedVersion = expectedSchemaVersion()
	if err := s.probeReadOnlyShape(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("只读打开 %s: %w", dbPath, err)
	}
	return s, nil
}

// validateReadOnlySchema 只读地核对 schema 版本，不做任何修正，返回库的
// 实际版本（迁移账本与 user_version 里较大的那个）。
//
// 任务书写的是读 PRAGMA user_version，但本项目的迁移账本从来只写
// schema_migrations 表（见 schema.go 的 migrate），user_version 一直是 0。
// 真按字面只读 user_version，每个正常库都会被判成「版本 0」，看板永远打不开。
// 所以这里两边都看，并取较大的那个当「库版本」：两个来源里只要有一个声称
// 比本二进制新，就必须拒绝——可能已经有更新的进程把新语义写进去了。
//
// 规则只有一条：库版本 ≤ 本二进制期望的版本就放行。旧库放行之后缺的列由
// probeReadOnlyShape 补零值；这里不尝试、也不允许补迁移（只读进程不拿锁，
// 自动迁移会和其它进程抢写）。
func (s *Store) validateReadOnlySchema(ctx context.Context) (int64, error) {
	expected := expectedSchemaVersion()

	var userVersion int64
	if err := s.read.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
		return 0, fmt.Errorf("读 PRAGMA user_version: %w", err)
	}

	ledgerVersion, err := s.readOnlySchemaVersion(ctx)
	if err != nil {
		return 0, err
	}

	actual := ledgerVersion
	source := "schema_migrations"
	if userVersion > actual {
		actual = userVersion
		source = "PRAGMA user_version"
	}

	if actual > expected {
		return 0, newerSchemaVersionError(actual, expected, source, ledgerVersion)
	}
	if actual == 0 {
		return 0, fmt.Errorf("%w: schema_migrations 里没有任何版本记录，这个文件不像是 council 建的库；先跑一次 council", ErrSchemaVersionMismatch)
	}
	return actual, nil
}

// probeReadOnlyShape 用 PRAGMA 探测实际结构，只读、不写库。
//
// 为什么必须探测：v7 库缺 archived_at 与 workspace_id，v8 库缺 workspace_id。
// read.go 的 SELECT 是按列名写的，直接查会以「no such column」在 scan 之前
// 就失败；而这里不能再走 Open 的 validateSchema（它要求表结构与最新版完全
// 一致，旧库当然不一致）。探测结果只影响读路径怎么拼 SELECT / WHERE，
// 不改变任何列的真实语义。
func (s *Store) probeReadOnlyShape(ctx context.Context) error {
	rows, err := s.read.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, "conversations")
	if err != nil {
		return fmt.Errorf("探测 conversations 的列: %w", err)
	}
	present := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("读 conversations 的列: %w", err)
		}
		present[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("遍历 conversations 的列: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("关闭 conversations 的列探测: %w", err)
	}

	s.shape = readShape{
		teamID:      present["team_id"],
		cwd:         present["cwd"],
		deletedAt:   present["deleted_at"],
		archivedAt:  present["archived_at"],
		workspaceID: present["workspace_id"],
	}

	var n int
	if err := s.read.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("探测 workspaces 表: %w", err)
	}
	s.shape.workspaces = n > 0
	return nil
}

// readOnlySchemaVersion 读迁移账本的最大版本。没有 schema_migrations 表时
// 返回 0，不返回错误：那是「一个旧库」，与「表存在但一行都没有」走同一条
// 版本不符的处置，不需要在调用方那里分成两档。
func (s *Store) readOnlySchemaVersion(ctx context.Context) (int64, error) {
	var n int
	if err := s.read.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("检查 schema_migrations 表: %w", err)
	}
	if n == 0 {
		return 0, nil
	}

	var v sql.NullInt64
	if err := s.read.QueryRowContext(ctx,
		`SELECT MAX(version) FROM schema_migrations`,
	).Scan(&v); err != nil {
		return 0, fmt.Errorf("读 schema 版本: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return v.Int64, nil
}

// expectedSchemaVersion 是本二进制能读的最新 schema 版本，也就是最后一版
// 迁移的版本号。只读模式允许库版本 ≤ 它（旧库），写模式会迁移到恰好这一版。
func expectedSchemaVersion() int64 {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].version
}

// newerSchemaVersionError 是「库比二进制新」时的说明。
//
// 这个方向必须拒绝而不是迁就着读：更新的进程可能已经把新语义写进库
// （新列、新枚举值、新的唯一性口径），旧二进制的读路径会把它们按零值
// 或旧含义解释，看起来读得出来、实际读错了。错误里点名是哪个来源声称
// 更新的，以及迁移账本的真实值，用户才知道该升级哪一侧。
func newerSchemaVersionError(actual, expected int64, source string, ledgerVersion int64) error {
	detail := fmt.Sprintf("%s 显示 %d，本二进制期望 %d", source, actual, expected)
	if source != "schema_migrations" {
		detail += fmt.Sprintf("（schema_migrations 是 %d）", ledgerVersion)
	}
	return fmt.Errorf("%w: %s；库比这个 council 新，当前版本读不了，先用新版 council 打开一次", ErrSchemaVersionMismatch, detail)
}
