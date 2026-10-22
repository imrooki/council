package store

// 本文件是 SRV1 的只读入口。它与 Open 的差别不是为了省几行代码：
// 看板要在 TUI 正开着的时候读同一个库，而 Open 会在 data_dir 上抢
// council.lock、会跑迁移、会在目录不存在时建目录。这三件事对看板全是错的
// ——抢锁会让看板把 TUI 挤掉（或者自己被拒绝），迁移会让一个只读进程写库。
// 所以这里复制的是 Open 的「打开读句柄」这一半，另外两件事一件都不做。

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

// ErrSchemaVersionMismatch 表示库里的 schema 版本与本二进制期望的不一致。
// 与 ErrSchemaMismatch 分开：那个说的是「表结构不对」，这个说的是「版本号不对」。
// 两条错误要求用户做的事不同——版本不对应当换对应的 council 版本，
// 而表结构不对应当先备份库；混成一个哨兵会让调用方只能猜。
var ErrSchemaVersionMismatch = errors.New("schema version mismatch")

// OpenReadOnly 以只读方式打开 dataDir 下的 council.db。
//
// 不建目录、不建库、不跑迁移、不碰 council.lock。于是 TUI 占着数据目录时，
// 看板仍然能读同一个库；库不存在时返回明确的错误而不是替用户建一个。
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
	dsn := "file:" + filepath.ToSlash(dbPath) +
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
	if err := s.validateReadOnlySchema(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("只读打开 %s: %w", dbPath, err)
	}
	return s, nil
}

// validateReadOnlySchema 只读地核对 schema 版本，不做任何修正。
//
// 任务书写的是读 PRAGMA user_version，但本项目的迁移账本从来只写
// schema_migrations 表（见 schema.go 的 migrate），user_version 一直是 0。
// 真按字面只读 user_version，每个正常库都会被判成「版本 0」，看板永远打不开。
// 所以这里两边都看：
//   - user_version 非 0 时，它本身就是别的版本留下的印章，必须等于期望值；
//   - 迁移账本的最大版本必须等于期望值。
//
// 两个来源都符合规则时才算通过：user_version 为 0 或等于期望值，且迁移账本
// 的最大版本等于期望值。这样既不会误伤现在这些 user_version=0 的库，
// 也能挡住「库比二进制新」的那种（将来的版本若开始盖 user_version，这里照样拦）。
func (s *Store) validateReadOnlySchema(ctx context.Context) error {
	expected := expectedSchemaVersion()

	var userVersion int64
	if err := s.read.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
		return fmt.Errorf("读 PRAGMA user_version: %w", err)
	}

	ledgerVersion, err := s.readOnlySchemaVersion(ctx)
	if err != nil {
		return err
	}

	if userVersion != 0 && userVersion != expected {
		return schemaVersionError(userVersion, ledgerVersion, expected,
			"PRAGMA user_version")
	}
	if ledgerVersion != expected {
		return schemaVersionError(ledgerVersion, ledgerVersion, expected,
			"schema_migrations")
	}
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

// expectedSchemaVersion 是本二进制能读的 schema 版本，也就是最后一版迁移的版本号。
// 看板不迁移，所以只能读「恰好这一版」；别的版本都让用户先回终端处理。
func expectedSchemaVersion() int64 {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].version
}

// schemaVersionError 把「低版本」和「高版本」分开说。
//
// 两个方向要用户做的事看起来一样（换一版 council），但原因相反：
// 低版本是库旧了，高版本是二进制旧了。只说「版本不匹配」会让用户去升级
// 一个本来就该升级的方向，或者反过来把新库降级。
func schemaVersionError(actual, ledgerVersion, expected int64, source string) error {
	detail := fmt.Sprintf("%s 显示 %d，本二进制期望 %d", source, actual, expected)
	if source != "schema_migrations" {
		detail += fmt.Sprintf("（schema_migrations 是 %d）", ledgerVersion)
	}
	if actual < expected {
		return fmt.Errorf("%w: %s；库是旧版本，先用新版 council 打开一次", ErrSchemaVersionMismatch, detail)
	}
	return fmt.Errorf("%w: %s；库比这个 council 新，当前版本读不了，先用新版 council 打开一次", ErrSchemaVersionMismatch, detail)
}
