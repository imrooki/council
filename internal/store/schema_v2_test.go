// schema_v2_test.go 覆盖 Step 7 契约 10a §1.2 与规格 10 §9 验收表最后一行：
// 「旧 v1 库打开时迁移到 v2 并通过形状指纹」。
//
// 这是本步最要紧的一条：它验的是「用户的老库打得开」，不是「新库从空跑一遍两版迁移」——
// 后者 open_test.go 的 TestOpenDoesNotRerunMigrations 已经覆盖（它按 migrations 的
// 版数断言行数，Step 7 加 v2 时自动变成 2，不用改）。这里手工造一个只跑到 v1 的库，
// 再用正式的 Open 打开它，逼真地重现一个真实用户升级二进制时会经历的路径。
package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// tstBuildV1Database 在 dir 下造一个「只跑到 v1」的 council.db：完整的 v1DDL、
// schema_migrations 只有一行 version=1，外加一条真实的会话，用来证明升级之后
// 老数据还在，而不是只验证了一个空库能升级。
//
// 与 tstBuildStaleDB（tx_test.go）不同：那个函数故意把 messages 换成更老的十一列版本，
// 用来验形状指纹兜底；这里要的是「正宗的 v1」，一字不改地照抄 v1DDL，因为要验的是
// v1 -> v2 这一步迁移本身会不会真的跑，不是形状校验兜底会不会拦住不一致的库。
func tstBuildV1Database(t *testing.T, dir string) (dbPath, seedConvID string) {
	t.Helper()

	dbPath = filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v1 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v1 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for _, stmt := range v1DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v1 库时执行 DDL 失败: %v\n语句: %s", err, stmt)
		}
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?)`, tstFixedCreatedAt,
	); err != nil {
		t.Fatalf("造 v1 库时写 schema_migrations 失败: %v", err)
	}

	seedConvID = "conv-from-real-v1-user"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO conversations (id, title, created_at) VALUES (?, ?, ?)`,
		seedConvID, "升级之前就存在的会话", tstFixedCreatedAt,
	); err != nil {
		t.Fatalf("造 v1 库时写入种子会话失败: %v", err)
	}
	return dbPath, seedConvID
}

// TestSchemaV2MigratesFromRealV1Database 是本步最重要的一条用例：
// 一个真实存在过的 v1 库（记着 version=1、且已经有数据），用正式的 Open 打开，
// 必须成功升级到 v2、通过形状校验，而且升级之前的数据还在、还能接着往下写。
func TestSchemaV2MigratesFromRealV1Database(t *testing.T) {
	dir := t.TempDir()
	_, seedConvID := tstBuildV1Database(t, dir)

	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("老库（v1）打不开了: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("关闭失败: %v", err)
		}
	})

	t.Run("每一版各落一行，v1 那行没有被重跑", func(t *testing.T) {
		// 断言跟着 migrations 的声明走，不写死行数。
		// 原来写死的是「2 行」，那在当时是对的，但它把「有几版迁移」这个
		// 会变的事实编进了断言：加第三版时这条会红，而红的原因与它真正
		// 想守的两件事（v1 没被重跑、没有哪一版落了两行）毫无关系。
		rows := tstMigrationRows(t, s.Path())
		if len(rows) != len(migrations) {
			t.Fatalf("schema_migrations 有 %d 行，期望 %d 行（每版一行，不重不漏）: %+v",
				len(rows), len(migrations), rows)
		}
		for i, m := range migrations {
			if rows[i].version != m.version {
				t.Errorf("第 %d 行 version = %d，期望 %d", i+1, rows[i].version, m.version)
			}
			if rows[i].appliedAt == "" {
				t.Errorf("version %d 的 applied_at 是空串", rows[i].version)
			}
		}
		// v1 是造库时就写好的那一行，它的时间戳必须原封不动。
		// 这是「已经跑过的迁移不会被重跑」唯一直接可观察的证据。
		if rows[0].appliedAt != tstFixedCreatedAt {
			t.Errorf("v1 的 applied_at = %q，期望仍是造库时写的 %q（说明 v1 被重跑了）",
				rows[0].appliedAt, tstFixedCreatedAt)
		}
	})

	t.Run("turn_usage 真的被 DROP+CREATE 成了 v2 的形状，不是版本号空转", func(t *testing.T) {
		raw := tstRawDB(t, s.Path())
		col := tstScanText(t, raw, `SELECT name FROM pragma_table_info('turn_usage') WHERE cid = 0`)
		if col != "turn_id" {
			t.Errorf("turn_usage 的第 0 列是 %q，期望 turn_id（v1 的第 0 列是 conv_id，"+
				"若还是它说明 DROP+CREATE 没有真的跑）", col)
		}
	})

	t.Run("升级之前就有的会话，升级之后原样读得回来", func(t *testing.T) {
		got, err := s.Conversation(context.Background(), seedConvID)
		if err != nil {
			t.Fatalf("读升级前就存在的会话失败: %v", err)
		}
		if got.Title != "升级之前就存在的会话" {
			t.Errorf("Title = %q，期望 %q", got.Title, "升级之前就存在的会话")
		}
	})

	t.Run("升级之后还能接着往下写", func(t *testing.T) {
		ctx := tstCtx(t)
		got := tstAppend(t, ctx, s, tstUserMsg(seedConvID, "升级之后写的第一条"))
		if got.Seq != 1 {
			t.Errorf("升级之后新会话第一条消息的 Seq = %d，期望 1", got.Seq)
		}
	})
}
