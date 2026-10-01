// 本文件除了 Open / Close / Path 的用例，还存放 internal/store 全部测试共用的
// 辅助函数与共用常量，一律以 tst 前缀命名。
// 放这里而不是另开一个 helpers 文件，是为了让测试文件与 08_STEP5_SPEC.md B 节列的
// 实现文件一一对应（open / schema / tx / read，外加并发回归一份），不额外多出一个。
//
// 全部用例只在 t.TempDir() 下建库，绝不触碰真实的数据目录（08 规格 J 节的硬性要求）。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"

	_ "modernc.org/sqlite"
)

// tstCallTimeout 是单次 store 调用的时间上限。
// 写句柄是单连接（D-10），事务一旦泄漏，下一次写就会一直等连接。
// 有了超时，泄漏表现为一条明确的失败，而不是让 go test 挂到整体超时才被发现。
const tstCallTimeout = 30 * time.Second

// tstDBFileName 是 08a 契约里写明的库文件名：Open 打开 dataDir 下的 council.db。
const tstDBFileName = "council.db"

// tstCtx 返回一个带超时的上下文，取消登记进 t.Cleanup。
func tstCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), tstCallTimeout)
	t.Cleanup(cancel)
	return ctx
}

// tstDataDir 返回一个位于 t.TempDir() 之下、尚未创建的数据目录路径。
// 用子路径而不是 t.TempDir() 本身，是为了让 Open 每次都真的走一遍 MkdirAll。
func tstDataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "council-data")
}

// tstOpenAt 在指定目录上打开 store，返回 store 与一个幂等的关闭函数。
// 关闭函数同时登记进 t.Cleanup：重启类用例可以自己提前调用，
// 没调用的也不会把打开的库文件留给 t.TempDir 的清理去撞 Windows 的文件占用。
func tstOpenAt(t *testing.T, dir string) (*Store, func()) {
	t.Helper()
	s, err := Open(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("Open(%q) 返回错误: %v", dir, err)
	}
	if s == nil {
		t.Fatalf("Open(%q) 返回了 nil store 且没有报错", dir)
	}
	var once sync.Once
	closeFn := func() {
		once.Do(func() {
			if err := s.Close(); err != nil {
				t.Errorf("Close() 返回错误: %v", err)
			}
		})
	}
	t.Cleanup(closeFn)
	return s, closeFn
}

// tstStore 在一个全新的临时数据目录上打开 store。
func tstStore(t *testing.T) (*Store, func()) {
	t.Helper()
	return tstOpenAt(t, tstDataDir(t))
}

// tstRawDB 用测试自己的连接打开同一个库文件，用来直接验证 DDL 约束。
// 故意不带 journal_mode：journal_mode 是写进库文件头的持久属性，
// 不指定才能如实读出 Open 当初把它设成了什么。
// 带 foreign_keys(1) 是为了让外键约束在这条连接上可见 —— 它验证的是 DDL 里有没有
// REFERENCES，store 自己那条连接上 pragma 有没有生效由行为用例另行覆盖
// （见 schema_test.go 的「写到不存在的会话必须失败」）。
func tstRawDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.ToSlash(dbPath) +
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("打开底层连接 %q 失败: %v", dsn, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭底层连接失败: %v", err)
		}
	})
	return db
}

// tstScanInt 在底层连接上跑一条只返回一个整数的查询。
func tstScanInt(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("查询 %q 失败: %v", query, err)
	}
	return n
}

// tstScanText 在底层连接上跑一条只返回一个字符串的查询。
func tstScanText(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var s string
	if err := db.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("查询 %q 失败: %v", query, err)
	}
	return s
}

// tstNewConv 在一个写事务里建一个会话，返回建好的会话。
func tstNewConv(t *testing.T, ctx context.Context, s *Store, title string) conversation.Conversation {
	t.Helper()
	var conv conversation.Conversation
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, title)
		return err
	})
	if err != nil {
		t.Fatalf("建会话 %q 失败: %v", title, err)
	}
	if conv.ID == "" {
		t.Fatalf("建会话 %q 返回的 ID 为空", title)
	}
	return conv
}

// tstAppend 追加一条消息，返回 store 补全后的那条。
func tstAppend(t *testing.T, ctx context.Context, s *Store, m conversation.Message) conversation.Message {
	t.Helper()
	var got conversation.Message
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var err error
		got, err = tx.AppendMessage(ctx, m)
		return err
	})
	if err != nil {
		t.Fatalf("追加消息 %q 失败: %v", m.Text, err)
	}
	return got
}

// tstUserMsg 造一条最小的用户消息。
// sender 用保留的 "user"：它不是 participant，不需要先 join（规格 G-3）。
func tstUserMsg(convID, text string) conversation.Message {
	return conversation.Message{
		ConvID:   convID,
		SenderID: conversation.ReservedSenderUser,
		Kind:     conversation.KindUser,
		Text:     text,
	}
}

// tstJoin 把一个 agent 加进会话。
func tstJoin(t *testing.T, ctx context.Context, s *Store, p conversation.Participant) {
	t.Helper()
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.AddParticipant(ctx, p)
	})
	if err != nil {
		t.Fatalf("加入 participant %q 失败: %v", p.AgentID, err)
	}
}

// tstLeave 让一个 agent 退出会话（软删除）。
func tstLeave(t *testing.T, ctx context.Context, s *Store, convID, agentID string, at time.Time) {
	t.Helper()
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.DeactivateParticipant(ctx, convID, agentID, at)
	})
	if err != nil {
		t.Fatalf("退出 participant %q 失败: %v", agentID, err)
	}
}

// tstSetCursor 写一个游标。
func tstSetCursor(t *testing.T, ctx context.Context, s *Store, c conversation.Cursor) {
	t.Helper()
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetCursor(ctx, c)
	})
	if err != nil {
		t.Fatalf("写游标 (%q, %q) 失败: %v", c.ConvID, c.AgentID, err)
	}
}

// tstErrMentionsPath 报告错误信息里是否提到了这个路径。
// 原生分隔符与斜杠两种写法都算：DSN 里的路径是 filepath.ToSlash 过的（08a），
// 错误从哪一层冒上来决定了它长什么样，而契约只要求「信息含路径」。
func tstErrMentionsPath(err error, path string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, path) || strings.Contains(msg, filepath.ToSlash(path))
}

// tstNoPanic 跑 fn 并把它的错误带回来；fn 若 panic 就直接判失败。
// 用在「Close 之后再用」这类用例上：契约要的是报错，不是 panic。
func tstNoPanic(t *testing.T, what string, fn func() error) error {
	t.Helper()
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s 发生了 panic，期望返回错误: %v", what, r)
			}
		}()
		err = fn()
	}()
	return err
}

func TestOpenCreatesDatabase(t *testing.T) {
	t.Run("新目录建库成功且 Path 指向真实存在的文件", func(t *testing.T) {
		dir := tstDataDir(t)
		s, _ := tstOpenAt(t, dir)

		p := s.Path()
		if p == "" {
			t.Fatal("Path() 返回空串")
		}
		if !filepath.IsAbs(p) {
			t.Errorf("Path() = %q，不是绝对路径", p)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("Path() 返回的 %q 不存在: %v", p, err)
		}
		if info.IsDir() {
			t.Errorf("Path() 返回的 %q 是个目录，期望是库文件", p)
		}
	})

	t.Run("库文件落在 dataDir 下且文件名是 council.db", func(t *testing.T) {
		dir := tstDataDir(t)
		s, _ := tstOpenAt(t, dir)

		p := s.Path()
		if got := filepath.Base(p); got != tstDBFileName {
			t.Errorf("库文件名 = %q，期望 %q", got, tstDBFileName)
		}
		gotDir := filepath.Clean(filepath.Dir(p))
		wantDir := filepath.Clean(dir)
		// 路径大小写在 Windows 上不区分，用 EqualFold 比较，避免临时目录写法差异造成假红。
		if !strings.EqualFold(gotDir, wantDir) {
			t.Errorf("库文件所在目录 = %q，期望 %q", gotDir, wantDir)
		}
	})

	t.Run("dataDir 不存在时 Open 负责创建", func(t *testing.T) {
		// 多套一层子目录，顺带验证 MkdirAll 会把中间目录一起建出来。
		dir := filepath.Join(t.TempDir(), "nested", "council-data")
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("前置条件不成立：%q 在 Open 之前就已经存在（err=%v）", dir, err)
		}

		tstOpenAt(t, dir)

		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Open 之后 %q 仍不存在: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("%q 不是目录", dir)
		}
	})

	t.Run("库文件的 journal_mode 是 wal", func(t *testing.T) {
		// D-10 的回归点之一：WAL 没真的开起来时写会慢两个数量级。
		// journal_mode 是持久属性，用一条不指定 journal_mode 的连接读回来才算数。
		s, closeFn := tstStore(t)
		dbPath := s.Path()
		closeFn()

		raw := tstRawDB(t, dbPath)
		if got := strings.ToLower(tstScanText(t, raw, "PRAGMA journal_mode")); got != "wal" {
			t.Errorf("journal_mode = %q，期望 wal；参见 D-10 与 docs/reference/sqlite-probe-results.md", got)
		}
	})
}

// tstMigrationRow 是 schema_migrations 里的一行。
type tstMigrationRow struct {
	version   int64
	appliedAt string
}

// tstMigrationRows 读出 schema_migrations 的全部行，按 version 升序。
func tstMigrationRows(t *testing.T, dbPath string) []tstMigrationRow {
	t.Helper()
	db := tstRawDB(t, dbPath)
	rows, err := db.Query(`SELECT version, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("读 schema_migrations 失败: %v", err)
	}
	defer rows.Close()

	var out []tstMigrationRow
	for rows.Next() {
		var r tstMigrationRow
		if err := rows.Scan(&r.version, &r.appliedAt); err != nil {
			t.Fatalf("扫描 schema_migrations 失败: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 schema_migrations 失败: %v", err)
	}
	return out
}

// TestOpenDoesNotRerunMigrations 覆盖规格 D 的「迁移必须幂等」：
// 开一次、关、再开，schema_migrations 里的行数与内容完全不变，连 applied_at 都不变。
//
// 行数与版本号都不写死，改成跟着 migrations 这张表走（Step 7 加 v2 时这条
// 原本写死 1 的断言红了一次）。写死一个数字的用例每加一版迁移就要改一次，
// 而它真正要锁的是「幂等」，不是「一共有几版」。
func TestOpenDoesNotRerunMigrations(t *testing.T) {
	dir := tstDataDir(t)

	s1, closeFirst := tstOpenAt(t, dir)
	dbPath := s1.Path()
	first := tstMigrationRows(t, dbPath)
	closeFirst()

	if len(first) != len(migrations) {
		t.Fatalf("首次 Open 之后 schema_migrations 有 %d 行，期望 %d 行（migrations 的版数）: %+v", len(first), len(migrations), first)
	}
	for i, row := range first {
		if row.version != migrations[i].version {
			t.Errorf("第 %d 行迁移版本号 = %d，期望 %d", i, row.version, migrations[i].version)
		}
		if row.appliedAt == "" {
			t.Errorf("第 %d 行 applied_at 为空串", i)
		}
	}

	s2, closeSecond := tstOpenAt(t, dir)
	if got := s2.Path(); got != dbPath {
		t.Errorf("重开后 Path() = %q，期望仍是 %q", got, dbPath)
	}
	second := tstMigrationRows(t, dbPath)
	closeSecond()

	if len(second) != len(first) {
		t.Fatalf("重开之后 schema_migrations 有 %d 行，期望仍是 %d 行: %+v", len(second), len(first), second)
	}
	// 逐行比 applied_at：迁移重跑会刷新时间戳，只比行数抓不住「删掉再重建」那种重跑。
	for i := range first {
		if second[i].version != first[i].version || second[i].appliedAt != first[i].appliedAt {
			t.Errorf("重开之后第 %d 行迁移记录变成了 %+v，期望与首次的 %+v 一致（迁移不该重跑）",
				i, second[i], first[i])
		}
	}
}

// TestOpenFailsWithPathInError 覆盖规格 J 的「非法路径时报错且信息含路径」。
// 造法：把 dataDir 指到一个已经存在的普通文件上，MkdirAll 必然失败。
func TestOpenFailsWithPathInError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "i-am-a-file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("准备阻塞文件失败: %v", err)
	}

	// 返回的 store 一律丢弃：万一实现在出错时也返回了非 nil 的句柄，
	// 碰它反而会在 Windows 上卡住 t.TempDir 的清理。
	_, err := Open(tstCtx(t), blocker)
	if err == nil {
		t.Fatalf("Open(%q) 期望报错，实际返回 nil", blocker)
	}
	if !tstErrMentionsPath(err, blocker) {
		t.Errorf("Open 的错误信息里没有提到路径 %q: %v", blocker, err)
	}
}

// TestUseAfterCloseReturnsError 覆盖「Close 之后再用应当报错而不是 panic」。
func TestUseAfterCloseReturnsError(t *testing.T) {
	s, closeFn := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "关闭之前建的会话")
	closeFn()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "Conversations",
			call: func() error {
				_, err := s.Conversations(ctx)
				return err
			},
		},
		{
			name: "Conversation",
			call: func() error {
				_, err := s.Conversation(ctx, conv.ID)
				return err
			},
		},
		{
			name: "Participants",
			call: func() error {
				_, err := s.Participants(ctx, conv.ID, true)
				return err
			},
		},
		{
			name: "Messages",
			call: func() error {
				_, err := s.Messages(ctx, conv.ID, 0, 0)
				return err
			},
		},
		{
			name: "MaxSeq",
			call: func() error {
				_, err := s.MaxSeq(ctx, conv.ID)
				return err
			},
		},
		{
			name: "Cursor",
			call: func() error {
				_, err := s.Cursor(ctx, conv.ID, "reviewer")
				return err
			},
		},
		{
			name: "Cursors",
			call: func() error {
				_, err := s.Cursors(ctx, conv.ID)
				return err
			},
		},
		{
			name: "WriteTx",
			call: func() error {
				return s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
					_, err := tx.CreateConversation(ctx, "关闭之后不该建成的会话")
					return err
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run("关闭后调用 "+tc.name+" 返回错误", func(t *testing.T) {
			if err := tstNoPanic(t, fmt.Sprintf("Close 之后调用 %s", tc.name), tc.call); err == nil {
				t.Errorf("Close 之后调用 %s 期望报错，实际返回 nil", tc.name)
			}
		})
	}
}
