package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// tstOldMessagesDDL 是 486f5f3 之前那一版 messages 的建表语句，十一列，没有三个脑子列、
// 也没有那条 sender_id 的 CHECK。逐字取自 2fa21cb 的 internal/store/schema.go。
//
// 它存在的唯一理由是造一个「旧版二进制建出来的库」来验形状校验。
// 不要因为它和现在的 DDL 不一样就去「修」它 —— 它就该不一样。
const tstOldMessagesDDL = `CREATE TABLE messages (
    id         TEXT PRIMARY KEY,
    conv_id    TEXT NOT NULL REFERENCES conversations(id),
    seq        INTEGER NOT NULL,
    sender_id  TEXT NOT NULL,
    recipients TEXT NOT NULL DEFAULT '[]',
    kind       TEXT NOT NULL CHECK (kind IN ('user','agent','system','task','task_result')),
    text       TEXT NOT NULL DEFAULT '',
    thought    TEXT NOT NULL DEFAULT '',
    parent_id  TEXT,
    created_at TEXT NOT NULL,
    meta       TEXT NOT NULL DEFAULT '{}',
    UNIQUE (conv_id, seq)
)`

// tstBuildStaleDB 在 dir 下造一个「旧版二进制建出来」的 council.db：
// 除 messages 是旧的十一列之外，其余与当前完全相同，并且 schema_migrations 里记着 version = 1。
//
// 只换 messages 这一张，是为了让用例里只有一个变量 —— 校验若报错，
// 只可能是因为它抓到了这张表，不可能是别的差异顺带触发的。
func tstBuildStaleDB(t *testing.T, dir string) string {
	t.Helper()

	stmts := make([]string, 0, len(v1DDL))
	replaced := false
	for _, s := range v1DDL {
		if strings.HasPrefix(s, "CREATE TABLE messages") {
			stmts = append(stmts, tstOldMessagesDDL)
			replaced = true
			continue
		}
		stmts = append(stmts, s)
	}
	if !replaced {
		t.Fatal("v1DDL 里找不到 messages 的建表语句，本用例的前提已经不成立，先改用例")
	}

	dbPath := filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造旧库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造旧库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("造旧库时执行 DDL 失败: %v\n语句: %s", err, s)
		}
	}
	// 关键的一半：版本号记成 1。没有这一行，新二进制会当成空库重跑迁移、
	// 建出正确的表，也就验不到我们要验的东西。
	if _, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?)`, tstFixedCreatedAt); err != nil {
		t.Fatalf("造旧库时写 schema_migrations 失败: %v", err)
	}
	return dbPath
}

// TestOpenRejectsStaleSchema 是未决 #10 的端到端用例。
//
// 场景：库是 486f5f3 之前的二进制建的，messages 只有十一列，但 schema_migrations 记着
// version = 1。migrate 只比较 MAX(version)，会直接跳过，于是 Open **成功**、
// 启动横幅照常打印，直到第一次读或写才报 no such column —— 排查的人会去怀疑 store 实现。
//
// 有了形状校验，Open 必须当场失败，并且错误要能把人一次带到结论。
// 这条用例若当初就存在，那个问题根本不会留到审查阶段。
func TestOpenRejectsStaleSchema(t *testing.T) {
	dir := t.TempDir()
	tstBuildStaleDB(t, dir)

	s, err := Open(context.Background(), dir)
	if err == nil {
		if s != nil {
			_ = s.Close()
		}
		t.Fatal("旧 schema 的库竟然打开成功了：形状校验没生效")
	}

	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("错误应当能被 errors.Is 认出 ErrSchemaMismatch，实际: %v", err)
	}
	// 错误必须把人带到结论，而不是带去排查。至少要点名是哪张表、缺了哪一列。
	msg := err.Error()
	for _, want := range []string{"messages", "sender_provider"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息里没有 %q，看到它的人无从下手；完整信息: %v", want, err)
		}
	}
}

// TestOpenAcceptsFreshSchema 是上一条的对照组。
// 没有它的话，「旧库打不开」可能只是因为形状校验把所有库都拒了。
func TestOpenAcceptsFreshSchema(t *testing.T) {
	s, _ := tstStore(t)
	if s == nil {
		t.Fatal("新建库应当能打开")
	}
	// 重开一次：已存在的、正确的库同样要能过校验，且不重跑迁移。
	if err := s.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	reopened, err := Open(context.Background(), filepath.Dir(s.Path()))
	if err != nil {
		t.Fatalf("重开一个正确的库应当成功，实际: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("关闭重开的库失败: %v", err)
		}
	})
}

// TestSchemaShapeCoversEveryTable 堵住形状声明本身的漏洞。
//
// 形状校验只会比对**被声明了的**表。若有人往 v1DDL 里加一张表却忘了加进 shape，
// 校验不会吭声 —— 那张表此后永远不设防。所以这里反过来断言：
// shape 声明的表集合，必须等于库里真实存在的表集合（sqlite_master 里非 sqlite_ 前缀的那些）。
//
// 这条也是那份重复（DDL 与 shape 各写一遍）的自检：漏一张就红。
func TestSchemaShapeCoversEveryTable(t *testing.T) {
	if len(migrations) == 0 {
		t.Fatal("migrations 是空的")
	}
	last := migrations[len(migrations)-1]
	if len(last.shape) == 0 {
		t.Fatal("最后一版迁移没有声明 shape，形状校验形同虚设")
	}

	s, _ := tstStore(t)
	raw := tstRawDB(t, s.Path())

	rows, err := raw.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("读 sqlite_master 失败: %v", err)
	}
	defer rows.Close()

	actual := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("扫描表名失败: %v", err)
		}
		actual[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历表名失败: %v", err)
	}

	for name := range actual {
		if _, ok := last.shape[name]; !ok {
			t.Errorf("表 %q 真实存在却没有被 shape 声明，它此后不设防", name)
		}
	}
	for name := range last.shape {
		if !actual[name] {
			t.Errorf("shape 声明了表 %q，但新建库里没有这张表", name)
		}
	}
}
