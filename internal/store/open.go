// Package store 是 Canonical Conversation 的 SQLite 持久化层。
// 只有本包认识 database/sql 与 modernc.org/sqlite。
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"council/internal/conversation"
	_ "modernc.org/sqlite"
)

// Store 是 conversation.Store 的 SQLite 实现。
// 写走单连接 IMMEDIATE 句柄，读走独立连接池。
// lock 是 data_dir 级的 OS 排他文件锁（未决 #8），锁着 <data_dir>/council.lock，
// 不锁 council.db 本身；Close 负责释放。
type Store struct {
	path  string
	write *sql.DB
	read  *sql.DB
	lock  *dataDirLock
}

var _ conversation.Store = (*Store)(nil)

const dbFileName = "council.db"

// lockFileName 是 data_dir 排他锁借用的专用文件名，与 council.db 分开。
const lockFileName = "council.lock"

// ErrDataDirLocked 表示这个 data_dir 已被另一个 council 进程占用。
//
// 存在的理由（未决 #8，原件 E:\Isolating_test\council\oq8-multiproc\RESULTS.md）：
// D-10 原来认为 SQLite 的 BEGIN IMMEDIATE + 唯一约束会让第二个进程"响亮地失败"，
// 实测推翻了这句话——第二个进程静默打开同一个库、静默写入、退出码 0；两个进程
// 对同一个 (会话, 成员) 各跑一轮之后：付了两次入场费（turn_usage 两行）、
// 先写的那条 provider 会话成了孤儿（provider 那边真实存在、钱花了，本地再没有
// 任何一行指向它）、游标错位（cursor_seq 落后于实际 seq，下次恢复会重投）、
// 两个进程各自的 /session 都"对"却互相矛盾。这四条全都发生在没有任何 SQLite
// 冲突的前提下——互斥本来就不在 SQLite 这一层，得在 data_dir 级另加一把锁。
var ErrDataDirLocked = errors.New("data dir is locked by another council process")

// Open 打开（必要时创建）dataDir 下的 council.db，建立读写双句柄，
// 跑完待应用的迁移，返回可用的 Store。dataDir 不存在时会以 0o755 创建。
func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", dataDir, err)
	}

	// data_dir 排他锁，取锁点定在这里：MkdirAll 之后（没有目录无处放锁文件），
	// 拼 DSN 之前（宁可锁没拿到就完全不碰库文件）。为什么要有这把锁、
	// 锁的是什么，见上面 ErrDataDirLocked 的注释与 docs/00_DECISIONS.md 未决 #8。
	//
	// lockOwned 管的是这把锁在"本次 Open 调用尚未把它交给 Store"这段窗口期里
	// 谁负责释放：只要还是 true，下面任何一条 early return 都会经这个 defer 放锁。
	// 一旦下面构造出 s 并把 lock 赋给 s.lock，就把 lockOwned 置为 false——
	// 从那一刻起锁的生命周期转交给 Store，之后无论是 migrate/validateSchema
	// 失败时代码显式调用的 s.Close()，还是调用方后续正常调用的 s.Close()，
	// 都只有那一处会释放锁，不会因为这里的 defer 也去释放而重复处理。
	// 这样写而不是在任务书列出的每一条 early return 上手动补一行 release()，
	// 是因为 defer 天然覆盖"全部"提前返回，不需要人工数返回语句、也不会在
	// 以后有人往这段加一条新的 early return 时被漏掉。
	lock, err := acquireDataDirLock(dataDir)
	if err != nil {
		return nil, err
	}
	lockOwned := true
	defer func() {
		if lockOwned {
			_ = lock.release()
		}
	}()

	dbPath, err := filepath.Abs(filepath.Join(dataDir, dbFileName))
	if err != nil {
		return nil, fmt.Errorf("resolve db path %s: %w", filepath.Join(dataDir, dbFileName), err)
	}

	// pragma 必须写进 DSN，而不是用 db.Exec 执行 PRAGMA。
	// 后者只作用于连接池里当时那一条连接，database/sql 按需新建的连接没有那些设置。
	// 实测 320 次并发写只成功 37 次。
	//
	// _txlock=immediate 必须有。BeginTx 默认发 BEGIN DEFERRED，事务先以读身份开始、
	// 真要写时才升级成写事务；两个都持有读快照的事务同时想升级，SQLite 直接返回
	// SQLITE_BUSY。那是死锁不是排队，busy_timeout 等多久都解不开。
	//
	// WAL 必须真的开起来。只做串行化而不开 WAL 的配置虽然也能全过，但慢 178 倍，
	// 因为退回了 delete 模式，每次提交要重写回滚日志。
	writeDSN := "file:" + filepath.ToSlash(dbPath) +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_txlock=immediate"

	writeDB, err := sql.Open("sqlite", writeDSN)
	if err != nil {
		return nil, fmt.Errorf("open write db %s: %w", dbPath, err)
	}
	// 写本身只能串行。池不限时多条连接互相排队等写锁，实测慢 9 倍。
	writeDB.SetMaxOpenConns(1)
	if err := writeDB.PingContext(ctx); err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("ping write db %s: %w", dbPath, err)
	}

	readDSN := "file:" + filepath.ToSlash(dbPath) +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"

	readDB, err := sql.Open("sqlite", readDSN)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open read db %s: %w", dbPath, err)
	}
	readDB.SetMaxOpenConns(4)
	if err := readDB.PingContext(ctx); err != nil {
		_ = writeDB.Close()
		_ = readDB.Close()
		return nil, fmt.Errorf("ping read db %s: %w", dbPath, err)
	}

	s := &Store{path: dbPath, write: writeDB, read: readDB, lock: lock}
	// 锁的生命周期从这里起转交给 s：下面 migrate / validateSchema 失败时
	// 显式调用的 s.Close()，以及调用方成功拿到 s 之后自己调用的 Close()，
	// 都会经 Store.Close() 释放这把锁——不需要再靠上面那个 defer。
	lockOwned = false
	if err := s.migrate(ctx); err != nil {
		closeErr := s.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("migrate %s: %w (close: %v)", dbPath, err, closeErr)
		}
		return nil, fmt.Errorf("migrate %s: %w", dbPath, err)
	}
	// 迁移跑完之后校验真实表结构。migrate 只比较 MAX(version)，
	// 旧版二进制建的库记着相同的版本号却是旧结构，不校验的话这里会成功、
	// 直到第一次读或写才报未知列（原未决 #10）。
	//
	// 错误前缀是 check schema 不是 migrate：失败发生在校验而不是迁移，
	// 而这条消息的全部价值就是让看到它的人一次走到结论、不要去翻迁移代码。
	if err := s.validateSchema(ctx); err != nil {
		closeErr := s.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("check schema %s: %w (close: %v)", dbPath, err, closeErr)
		}
		return nil, fmt.Errorf("check schema %s: %w", dbPath, err)
	}
	return s, nil
}

// Close 关闭写句柄、读句柄，并释放 data_dir 排他锁。三者都会做，任一失败都作为
// 错误返回；lock 为 nil（理论上不该发生，只有 Open 内部构造 Store 才会赋值）时
// release() 本身是幂等的空操作，不需要在这里另外判空。
func (s *Store) Close() error {
	var writeErr, readErr, lockErr error
	if s.write != nil {
		writeErr = s.write.Close()
	}
	if s.read != nil {
		readErr = s.read.Close()
	}
	// release() 对 nil 接收者本身就是安全的空操作（见 lock_windows.go /
	// lock_other.go 里 l == nil 的判空），这里不用像 write/read 那样先判 s.lock
	// 是否为 nil 才敢调用。
	lockErr = s.lock.release()
	switch {
	case writeErr != nil && readErr != nil && lockErr != nil:
		return fmt.Errorf("close write handle: %w; close read handle: %w; release data dir lock: %w", writeErr, readErr, lockErr)
	case writeErr != nil && readErr != nil:
		return fmt.Errorf("close write handle: %w; close read handle: %w", writeErr, readErr)
	case writeErr != nil && lockErr != nil:
		return fmt.Errorf("close write handle: %w; release data dir lock: %w", writeErr, lockErr)
	case readErr != nil && lockErr != nil:
		return fmt.Errorf("close read handle: %w; release data dir lock: %w", readErr, lockErr)
	case writeErr != nil:
		return fmt.Errorf("close write handle: %w", writeErr)
	case readErr != nil:
		return fmt.Errorf("close read handle: %w", readErr)
	case lockErr != nil:
		return fmt.Errorf("release data dir lock: %w", lockErr)
	default:
		return nil
	}
}

// Path 返回库文件的绝对路径。
func (s *Store) Path() string {
	return s.path
}

func newID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", s, err)
	}
	return t.UTC(), nil
}

func encodeStringSlice(v []string) (string, error) {
	if v == nil {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode string slice: %w", err)
	}
	return string(b), nil
}

func decodeStringSlice(s string) ([]string, error) {
	if s == "" || s == "null" {
		return []string{}, nil
	}
	var v []string
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("decode string slice: %w", err)
	}
	if v == nil {
		return []string{}, nil
	}
	return v, nil
}

func encodeInt64Slice(v []int64) (string, error) {
	if v == nil {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode int64 slice: %w", err)
	}
	return string(b), nil
}

func decodeInt64Slice(s string) ([]int64, error) {
	if s == "" || s == "null" {
		return []int64{}, nil
	}
	var v []int64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("decode int64 slice: %w", err)
	}
	if v == nil {
		return []int64{}, nil
	}
	return v, nil
}

func encodeMeta(m map[string]any) (string, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("encode meta: %w", err)
	}
	return string(b), nil
}

func decodeMeta(s string) (map[string]any, error) {
	if s == "" || s == "null" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("decode meta: %w", err)
	}
	if m == nil {
		return map[string]any{}, nil
	}
	return m, nil
}
