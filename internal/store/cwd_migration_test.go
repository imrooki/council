// 本文件覆盖 schema v6：停在 v5 的库打开后补上 conversations.cwd，
// 老行读出空串，其余表一字不改；v6Shape 是 v5Shape 的深拷贝再加这一列。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"council/internal/conversation"
)

// tstCwdBuildV5Database 在 dir 下造一个只跑到 v5 的 council.db：
// migrations[0] 到 migrations[4] 的语句全部执行，再种老用户的真实数据，
// schema_migrations 补齐 1 到 5 五行。不引用 v6DDL：造库路径必须停在
// 升级之前，否则后面的迁移测试测的是空操作。
func tstCwdBuildV5Database(t *testing.T, dir string) {
	t.Helper()
	if len(migrations) < 6 || migrations[4].version != 5 || migrations[5].version != 6 {
		t.Fatalf("migrations 的形状变了: len=%d，本辅助只该造 v5 库（下标 4 的 version 必须是 5，下标 5 必须是 6）", len(migrations))
	}

	dbPath := filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v5 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v5 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for i, m := range migrations[:5] {
		for _, stmt := range m.stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatalf("造 v5 库时执行 migrations[%d] version %d 失败: %v\n语句: %s", i, m.version, err, stmt)
			}
		}
	}

	convs := []struct {
		id, title, createdAt, teamID string
	}{
		{"c-old-1", "旧会话一", "2026-09-20T00:00:01Z", ""},
		{"c-old-2", "旧会话 二 (含括号)", "2026-09-20T00:00:02Z", "team-x"},
		{"c-old-3", "旧会话三", "2026-09-20T00:00:03Z", ""},
	}
	for _, c := range convs {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO conversations (id, title, created_at, team_id) VALUES (?, ?, ?, ?)`,
			c.id, c.title, c.createdAt, c.teamID,
		); err != nil {
			t.Fatalf("造 v5 库时写入会话 %s 失败: %v", c.id, err)
		}
	}

	parts := []struct {
		convID, agentID, displayName, provider, joinedAt string
		active                                           int
		leftAt                                           any
	}{
		{"c-old-1", "alpha", "Alpha", "claude", "2026-09-20T01:00:00Z", 1, nil},
		{"c-old-1", "beta", "Beta", "codex", "2026-09-20T01:00:01Z", 0, "2026-09-20T02:00:00Z"},
		{"c-old-2", "alpha", "Alpha", "claude", "2026-09-20T01:00:02Z", 1, nil},
	}
	for _, p := range parts {
		if _, err := db.ExecContext(ctx, `
INSERT INTO participants (conv_id, agent_id, display_name, provider, active, joined_at, left_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			p.convID, p.agentID, p.displayName, p.provider, p.active, p.joinedAt, p.leftAt,
		); err != nil {
			t.Fatalf("造 v5 库时写入成员 %s/%s 失败: %v", p.convID, p.agentID, err)
		}
	}

	msgs := []struct {
		id, convID, senderID, senderProvider, senderModel, senderEffort, recipients, kind, text, thought, createdAt, meta string
		seq                                                                                                               int64
		parentID                                                                                                          any
	}{
		{"m-1", "c-old-1", "user", "", "", "", "[]", "user", "第一条用户话", "", "2026-09-20T03:00:01Z", "{}", 1, nil},
		{"m-2", "c-old-1", "alpha", "claude", "opus", "high", `["alpha"]`, "agent", "代理回复含中文", "", "2026-09-20T03:00:02Z", `{"k":"v"}`, 2, nil},
		{"m-3", "c-old-1", "user", "", "", "", "[]", "user", "第三条", "", "2026-09-20T03:00:03Z", "{}", 3, nil},
		{"m-4", "c-old-2", "alpha", "claude", "sonnet", "", "[]", "agent", "另一场的一条", "", "2026-09-20T03:00:04Z", "{}", 1, nil},
	}
	for _, m := range msgs {
		if _, err := db.ExecContext(ctx, `
INSERT INTO messages
    (id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort, recipients, kind, text, thought, parent_id, created_at, meta)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.id, m.convID, m.seq, m.senderID, m.senderProvider, m.senderModel, m.senderEffort,
			m.recipients, m.kind, m.text, m.thought, m.parentID, m.createdAt, m.meta,
		); err != nil {
			t.Fatalf("造 v5 库时写入消息 %s 失败: %v", m.id, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO cursors (conv_id, agent_id, seq, pending, updated_at)
VALUES (?, ?, ?, ?, ?)`,
		"c-old-1", "alpha", 3, "[]", "2026-09-20T04:00:00Z",
	); err != nil {
		t.Fatalf("造 v5 库时写入游标失败: %v", err)
	}

	seed := tstV5ProviderSessionSeedFixture("c-old-1")
	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_sessions
    (conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd, have_cost_total, last_turn_at, identity_fp, spoken)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seed.convID, seed.agentID, seed.providerSessionID, seed.cursorSeq,
		seed.costTotalUSD, seed.haveCostTotal, seed.lastTurnAt, seed.identityFP, seed.spoken,
	); err != nil {
		t.Fatalf("造 v5 库时写入 provider_sessions 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?), (2, ?), (3, ?), (4, ?), (5, ?)`,
		tstFixedCreatedAt+"#1", tstFixedCreatedAt+"#2", tstFixedCreatedAt+"#3",
		tstFixedCreatedAt+"#4", tstFixedCreatedAt+"#5",
	); err != nil {
		t.Fatalf("造 v5 库时写 schema_migrations 失败: %v", err)
	}
}

// tstCwdDump 把查询的每一行收成一条字符串。列值里 []byte 转 string，
// NULL 写成 <NULL>，其余用 fmt.Sprint。列间用 " | " 连接。
// 顺序就是查询给出的顺序，调用方的查询必须自带 ORDER BY。
func tstCwdDump(t *testing.T, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatalf("查询失败: %v\n语句: %s", err, query)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("取列名失败: %v\n语句: %s", err, query)
	}
	var out []string
	for rows.Next() {
		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("扫描行失败: %v\n语句: %s", err, query)
		}
		parts := make([]string, len(cols))
		for i, v := range dest {
			switch x := v.(type) {
			case nil:
				parts[i] = "<NULL>"
			case []byte:
				parts[i] = string(x)
			default:
				parts[i] = fmt.Sprint(x)
			}
		}
		line := ""
		for i, p := range parts {
			if i > 0 {
				line += " | "
			}
			line += p
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历查询失败: %v\n语句: %s", err, query)
	}
	return out
}

func tstCwdEqualLines(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s 行数 = %d，期望 %d\n实际: %v\n期望: %v", label, len(got), len(want), got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s 第 %d 行 = %q，期望 %q", label, i, got[i], want[i])
		}
	}
}

// tstCwdSnapshotTables 是迁移前后要比对的六张表。conversations 只取
// 升级前就有的四列：cwd 是新列，老数据那四列必须一字不变。
func tstCwdSnapshotTables(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	return map[string][]string{
		"conversations":     tstCwdDump(t, db, `SELECT id, title, created_at, team_id FROM conversations ORDER BY id`),
		"participants":      tstCwdDump(t, db, `SELECT * FROM participants ORDER BY conv_id, agent_id`),
		"messages":          tstCwdDump(t, db, `SELECT * FROM messages ORDER BY conv_id, seq`),
		"cursors":           tstCwdDump(t, db, `SELECT * FROM cursors ORDER BY conv_id, agent_id`),
		"provider_sessions": tstCwdDump(t, db, `SELECT * FROM provider_sessions ORDER BY conv_id, agent_id`),
		"schema_migrations": tstCwdDump(t, db, `SELECT version, applied_at FROM schema_migrations ORDER BY version`),
	}
}

// TestCwdMigrationV5ToV6 造一个停在 v5 的库，用正式的 Open 升到当前版本，
// 钉住：老行没被改写，cwd 落在最后一列且旧行读出空串，迁移不会重跑。
func TestCwdMigrationV5ToV6(t *testing.T) {
	dir := t.TempDir()
	tstCwdBuildV5Database(t, dir)

	beforeDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, dbFileName))+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("取迁移前快照时打开连接失败: %v", err)
	}
	before := tstCwdSnapshotTables(t, beforeDB)
	if err := beforeDB.Close(); err != nil {
		t.Fatalf("取完迁移前快照后关闭连接失败: %v", err)
	}

	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	raw := tstRawDB(t, s.Path())

	t.Run("schema_migrations 记到 migrations 最后一版且行数相等", func(t *testing.T) {
		wantVer := migrations[len(migrations)-1].version
		gotVer := tstScanInt(t, raw, `SELECT MAX(version) FROM schema_migrations`)
		if gotVer != wantVer {
			t.Errorf("MAX(version) = %d，期望 %d（migrations 里最后一版的 version）", gotVer, wantVer)
		}
		gotN := tstScanInt(t, raw, `SELECT COUNT(*) FROM schema_migrations`)
		if gotN != int64(len(migrations)) {
			t.Errorf("schema_migrations 行数 = %d，期望 %d（每版一行）", gotN, len(migrations))
		}
	})

	t.Run("version 1 到 5 的 applied_at 没被重写", func(t *testing.T) {
		after := tstCwdDump(t, raw, `SELECT version, applied_at FROM schema_migrations ORDER BY version`)
		wantOld := before["schema_migrations"]
		if len(after) < len(wantOld)+1 {
			t.Fatalf("迁移后 schema_migrations 有 %d 行，期望至少 %d 行（旧的 %d 行加新的一行）",
				len(after), len(wantOld)+1, len(wantOld))
		}
		tstCwdEqualLines(t, "schema_migrations 旧行", after[:len(wantOld)], wantOld)
		if after[len(wantOld)] == "" {
			t.Errorf("schema_migrations 第 %d 行是空的，期望有新版本那一行", len(wantOld))
		}
	})

	t.Run("cwd 在 team_id 之后、deleted_at/archived_at 之前，且 NOT NULL DEFAULT ''", func(t *testing.T) {
		rows, err := raw.Query(`SELECT name, type, "notnull", dflt_value FROM pragma_table_info('conversations') ORDER BY cid`)
		if err != nil {
			t.Fatalf("读 pragma_table_info('conversations') 失败: %v", err)
		}
		defer rows.Close()
		var names []string
		var cwdType string
		var cwdNotNull int64
		var cwdDefault any
		var sawCwd bool
		for rows.Next() {
			var name, typ string
			var notnull int64
			var dflt any
			if err := rows.Scan(&name, &typ, &notnull, &dflt); err != nil {
				t.Fatalf("扫描 pragma_table_info 失败: %v", err)
			}
			names = append(names, name)
			if name == "cwd" {
				sawCwd = true
				cwdType = typ
				cwdNotNull = notnull
				cwdDefault = dflt
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("遍历 pragma_table_info 失败: %v", err)
		}
		// v7 把 deleted_at 加在 cwd 之后、v8 再把 archived_at 加在 deleted_at
		// 之后（ALTER 只能加在列尾），所以 cwd 不再是最末列；这条断言仍要
		// 钉住它在 ALTER 序列里的位置。
		wantNames := []string{"id", "title", "created_at", "team_id", "cwd", "deleted_at", "archived_at"}
		if !reflect.DeepEqual(names, wantNames) {
			t.Errorf("conversations 列序 = %v，期望 %v（cwd 由 v6 加在 team_id 之后，deleted_at/archived_at 依次由 v7/v8 追加）", names, wantNames)
		}
		if !sawCwd {
			t.Fatal("pragma_table_info 里没有 cwd 列")
		}
		if cwdType != "TEXT" {
			t.Errorf("cwd type = %q，期望 TEXT", cwdType)
		}
		if cwdNotNull != 1 {
			t.Errorf("cwd notnull = %d，期望 1（DDL 是 NOT NULL）", cwdNotNull)
		}
		// SQLite 把 DEFAULT '' 记成两个单引号的 SQL 字面量，不是空串本身。
		gotDefault := ""
		switch x := cwdDefault.(type) {
		case string:
			gotDefault = x
		case []byte:
			gotDefault = string(x)
		default:
			t.Fatalf("cwd dflt_value 类型 = %T，值 = %#v，期望字符串 ''", cwdDefault, cwdDefault)
		}
		if gotDefault != "''" {
			t.Errorf("cwd dflt_value = %q，期望 \"''\"（两个单引号的 SQL 字面量）", gotDefault)
		}
	})

	t.Run("六张表的老数据一字未变", func(t *testing.T) {
		after := tstCwdSnapshotTables(t, raw)
		for _, name := range []string{"conversations", "participants", "messages", "cursors", "provider_sessions"} {
			tstCwdEqualLines(t, name, after[name], before[name])
		}
	})

	t.Run("老会话的 cwd 全是空串", func(t *testing.T) {
		got := tstCwdDump(t, raw, `SELECT id, cwd FROM conversations ORDER BY id`)
		want := []string{
			"c-old-1 | ",
			"c-old-2 | ",
			"c-old-3 | ",
		}
		tstCwdEqualLines(t, "conversations.cwd", got, want)

		list, err := s.Conversations(ctx)
		if err != nil {
			t.Fatalf("Conversations 失败: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("Conversations 返回 %d 场，期望 3 场", len(list))
		}
		for _, c := range list {
			if c.Cwd != "" {
				t.Errorf("会话 %s 的 Cwd = %q，期望空串（老会话没绑项目）", c.ID, c.Cwd)
			}
		}
		one, err := s.Conversation(ctx, "c-old-2")
		if err != nil {
			t.Fatalf("Conversation(c-old-2) 失败: %v", err)
		}
		if one.Cwd != "" {
			t.Errorf("c-old-2 的 Cwd = %q，期望空串", one.Cwd)
		}
		if one.TeamID != "team-x" {
			t.Errorf("c-old-2 的 TeamID = %q，期望 %q", one.TeamID, "team-x")
		}
		if one.Title != "旧会话 二 (含括号)" {
			t.Errorf("c-old-2 的 Title = %q，期望 %q", one.Title, "旧会话 二 (含括号)")
		}
	})

	t.Run("validateSchema 通过", func(t *testing.T) {
		if err := s.validateSchema(ctx); err != nil {
			t.Errorf("validateSchema 返回错误，期望 nil: %v", err)
		}
	})

	t.Run("升级后的库能盖章也能读回老消息", func(t *testing.T) {
		const stamped = `D:\proj\old-one`
		err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
			return tx.SetConversationCwd(ctx, "c-old-1", stamped)
		})
		if err != nil {
			t.Fatalf("SetConversationCwd 失败: %v", err)
		}
		one, err := s.Conversation(ctx, "c-old-1")
		if err != nil {
			t.Fatalf("盖章后 Conversation(c-old-1) 失败: %v", err)
		}
		if one.Cwd != stamped {
			t.Errorf("c-old-1 的 Cwd = %q，期望 %q", one.Cwd, stamped)
		}
		msgs, err := s.Messages(ctx, "c-old-1", 0, 0)
		if err != nil {
			t.Fatalf("Messages(c-old-1) 失败: %v", err)
		}
		if len(msgs) != 3 {
			t.Errorf("c-old-1 的消息数 = %d，期望 3", len(msgs))
		}
		seq, err := s.MaxSeq(ctx, "c-old-1")
		if err != nil {
			t.Fatalf("MaxSeq(c-old-1) 失败: %v", err)
		}
		if seq != 3 {
			t.Errorf("c-old-1 的 MaxSeq = %d，期望 3", seq)
		}
		// 读回核对完再设回空串：重开要再验「三行 cwd 都是空串」，
		// 盖章留下的值会让那条断言对不上，而空串本身就是合法的清空。
		if err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
			return tx.SetConversationCwd(ctx, "c-old-1", "")
		}); err != nil {
			t.Fatalf("把 c-old-1 的 cwd 设回空串失败: %v", err)
		}
	})

	closeFn()
	s2, _ := tstOpenAt(t, dir)
	ctx2 := tstCtx(t)
	raw2 := tstRawDB(t, s2.Path())

	t.Run("重开后迁移不重复执行", func(t *testing.T) {
		gotN := tstScanInt(t, raw2, `SELECT COUNT(*) FROM schema_migrations`)
		if gotN != int64(len(migrations)) {
			t.Errorf("重开后 schema_migrations 行数 = %d，期望 %d（迁移被重复执行时 ALTER 会因 duplicate column 失败，Open 本身就不会成功）", gotN, len(migrations))
		}
	})

	t.Run("重开后老会话的 cwd 仍是空串", func(t *testing.T) {
		got := tstCwdDump(t, raw2, `SELECT id, cwd FROM conversations ORDER BY id`)
		want := []string{
			"c-old-1 | ",
			"c-old-2 | ",
			"c-old-3 | ",
		}
		tstCwdEqualLines(t, "重开后 conversations.cwd", got, want)

		list, err := s2.Conversations(ctx2)
		if err != nil {
			t.Fatalf("重开后 Conversations 失败: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("重开后 Conversations 返回 %d 场，期望 3 场", len(list))
		}
		for _, c := range list {
			if c.Cwd != "" {
				t.Errorf("重开后会话 %s 的 Cwd = %q，期望空串", c.ID, c.Cwd)
			}
		}
		one, err := s2.Conversation(ctx2, "c-old-2")
		if err != nil {
			t.Fatalf("重开后 Conversation(c-old-2) 失败: %v", err)
		}
		if one.Cwd != "" {
			t.Errorf("重开后 c-old-2 的 Cwd = %q，期望空串", one.Cwd)
		}
		if one.TeamID != "team-x" {
			t.Errorf("重开后 c-old-2 的 TeamID = %q，期望 %q", one.TeamID, "team-x")
		}
		if one.Title != "旧会话 二 (含括号)" {
			t.Errorf("重开后 c-old-2 的 Title = %q，期望 %q", one.Title, "旧会话 二 (含括号)")
		}
	})

	t.Run("重开后 validateSchema 仍通过", func(t *testing.T) {
		if err := s2.validateSchema(ctx2); err != nil {
			t.Errorf("重开后 validateSchema 返回错误，期望 nil: %v", err)
		}
	})
}

// TestCwdMigrationFreshDatabaseIsLatest 钉住全新库直接建到最新版，
// cwd 在列尾，新建会话读回的 Cwd 是空串。
func TestCwdMigrationFreshDatabaseIsLatest(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	raw := tstRawDB(t, s.Path())

	wantVer := migrations[len(migrations)-1].version
	gotVer := tstScanInt(t, raw, `SELECT MAX(version) FROM schema_migrations`)
	if gotVer != wantVer {
		t.Errorf("MAX(version) = %d，期望 %d（migrations 里最后一版的 version）", gotVer, wantVer)
	}

	// v8 起 conversations 的列尾是 archived_at（ALTER 追加），它前面是
	// v7 的 deleted_at，cwd 仍是 v6 加在 team_id 之后的那一列。末列断言
	// 跟着最新迁移走，同时保留 cwd、deleted_at 与 archived_at 的类型/可空检查。
	last := tstScanText(t, raw, `SELECT name FROM pragma_table_info('conversations') ORDER BY cid DESC LIMIT 1`)
	if last != "archived_at" {
		t.Errorf("conversations 末列 = %q，期望 archived_at", last)
	}
	deletedType := tstScanText(t, raw, `SELECT type FROM pragma_table_info('conversations') WHERE name = 'deleted_at'`)
	if deletedType != "TEXT" {
		t.Errorf("deleted_at type = %q，期望 TEXT", deletedType)
	}
	deletedNotNull := tstScanInt(t, raw, `SELECT "notnull" FROM pragma_table_info('conversations') WHERE name = 'deleted_at'`)
	if deletedNotNull != 0 {
		t.Errorf("deleted_at notnull = %d，期望 0（可空，NULL 表示没删）", deletedNotNull)
	}
	archivedType := tstScanText(t, raw, `SELECT type FROM pragma_table_info('conversations') WHERE name = 'archived_at'`)
	if archivedType != "TEXT" {
		t.Errorf("archived_at type = %q，期望 TEXT", archivedType)
	}
	archivedNotNull := tstScanInt(t, raw, `SELECT "notnull" FROM pragma_table_info('conversations') WHERE name = 'archived_at'`)
	if archivedNotNull != 0 {
		t.Errorf("archived_at notnull = %d，期望 0（可空，NULL 表示没归档）", archivedNotNull)
	}
	typ := tstScanText(t, raw, `SELECT type FROM pragma_table_info('conversations') WHERE name = 'cwd'`)
	if typ != "TEXT" {
		t.Errorf("cwd type = %q，期望 TEXT", typ)
	}
	notnull := tstScanInt(t, raw, `SELECT "notnull" FROM pragma_table_info('conversations') WHERE name = 'cwd'`)
	if notnull != 1 {
		t.Errorf("cwd notnull = %d，期望 1", notnull)
	}

	if err := s.validateSchema(ctx); err != nil {
		t.Errorf("validateSchema 返回错误，期望 nil: %v", err)
	}

	conv := tstNewConv(t, ctx, s, "全新会话")
	got, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Conversation(%s) 失败: %v", conv.ID, err)
	}
	if got.Cwd != "" {
		t.Errorf("新建会话的 Cwd = %q，期望空串（没传目录）", got.Cwd)
	}
}

// TestCwdV6ShapeConversationsIsNotAliasOfV5 钉住 v6Shape 不是 v5Shape
// 的裸别名。选 conversations：v6 对它 append 了 cwd。
//
// 这个函数里"改元素"那个子用例结构性地测不出"浅拷贝但 append 触发扩容"
// 的坏法：conversations 是 v6 用 append 加了 cwd 的表，append 到一个
// len == cap 的 slice 必然换一块新的底层数组，不管 buildV6Shape 那一步
// 是深拷贝还是浅拷贝，按下标改元素都碰不到 v5。真正会暴露浅拷贝的是
// TestCwdV6ShapeUntouchedTableElementIsNotAliasOfV5。
func TestCwdV6ShapeConversationsIsNotAliasOfV5(t *testing.T) {
	wantV5 := cloneColumnShapes(v5Shape["conversations"])
	wantV4 := cloneColumnShapes(v4Shape["conversations"])
	wantV3 := cloneColumnShapes(v3Shape["conversations"])
	wantV2 := cloneColumnShapes(v2Shape["conversations"])
	if len(wantV5) == 0 {
		t.Fatal("v5Shape 里 conversations 是空的, 本用例的前提不成立")
	}

	t.Run("改键(对 v6Shape 的某个键整体重新赋值)不影响 v5Shape/v4Shape/v3Shape/v2Shape", func(t *testing.T) {
		if _, ok := v6Shape["conversations"]; !ok {
			t.Fatal(`v6Shape 里没有 "conversations" 这张表, 本用例的前提不成立`)
		}
		// 存的是原 slice 本身(引用)，不是深拷贝：下面重新赋值
		// v6Shape["conversations"] 之前从不往这个原 slice 的底层数组写
		// 字节(append 到的是 cloneColumnShapes 另开的一块新内存)，恢复这个
		// 引用是安全的。
		orig := v6Shape["conversations"]
		t.Cleanup(func() {
			v6Shape["conversations"] = orig
		})

		mutated := append(cloneColumnShapes(v6Shape["conversations"]),
			columnShape{name: "tst_cwd_shape_alias_probe_key", typ: "TEXT", notnull: false})
		v6Shape["conversations"] = mutated

		assertColumnShapesEqual(t, `v5Shape["conversations"]`, v5Shape["conversations"], wantV5)
		assertColumnShapesEqual(t, `v4Shape["conversations"]`, v4Shape["conversations"], wantV4)
		assertColumnShapesEqual(t, `v3Shape["conversations"]`, v3Shape["conversations"], wantV3)
		assertColumnShapesEqual(t, `v2Shape["conversations"]`, v2Shape["conversations"], wantV2)
	})

	t.Run("改元素(对 v6Shape 某个键的 slice 按下标赋值)不影响 v5Shape/v4Shape/v3Shape/v2Shape", func(t *testing.T) {
		cols, ok := v6Shape["conversations"]
		if !ok || len(cols) == 0 {
			t.Fatal(`v6Shape["conversations"] 为空或不存在, 本用例的前提不成立`)
		}
		origElem := cols[0]
		t.Cleanup(func() {
			v6Shape["conversations"][0] = origElem
		})

		v6Shape["conversations"][0] = columnShape{
			name: "tst_cwd_shape_alias_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}

		assertColumnShapesEqual(t, `v5Shape["conversations"]`, v5Shape["conversations"], wantV5)
		assertColumnShapesEqual(t, `v4Shape["conversations"]`, v4Shape["conversations"], wantV4)
		assertColumnShapesEqual(t, `v3Shape["conversations"]`, v3Shape["conversations"], wantV3)
		assertColumnShapesEqual(t, `v2Shape["conversations"]`, v2Shape["conversations"], wantV2)
	})
}

// TestCwdV6ShapeUntouchedTableElementIsNotAliasOfV5 补上上一个函数的
// 结构性盲点：挑一张 v6 没 append 过列的表，按下标改元素，才能抓住
// "只 make 了新 map、切片仍共享底层数组" 这种坏法。
func TestCwdV6ShapeUntouchedTableElementIsNotAliasOfV5(t *testing.T) {
	var candidates []string
	for table, v6cols := range v6Shape {
		v5cols, ok := v5Shape[table]
		if !ok {
			continue
		}
		if len(v6cols) == len(v5cols) && len(v6cols) > 0 {
			candidates = append(candidates, table)
		}
	}
	if len(candidates) == 0 {
		t.Fatal("v6Shape 里找不到一张列数与 v5Shape 相同的非空表, " +
			"本用例依赖的前提(v6 至少留一张表没有 append 新列)不成立")
	}
	sort.Strings(candidates)
	probe := candidates[0]

	wantV5 := cloneColumnShapes(v5Shape[probe])
	wantV4 := cloneColumnShapes(v4Shape[probe])
	wantV3 := cloneColumnShapes(v3Shape[probe])
	wantV2 := cloneColumnShapes(v2Shape[probe])

	t.Run("改元素(对 v6Shape 里 v6 未新增列的表按下标赋值)不影响 v5Shape/v4Shape/v3Shape/v2Shape", func(t *testing.T) {
		cols, ok := v6Shape[probe]
		if !ok || len(cols) == 0 {
			t.Fatalf("v6Shape[%q] 为空或不存在, 本用例的前提不成立", probe)
		}
		origElem := cols[0]
		t.Cleanup(func() {
			v6Shape[probe][0] = origElem
		})

		v6Shape[probe][0] = columnShape{
			name: "tst_cwd_shape_untouched_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}

		assertColumnShapesEqual(t, fmt.Sprintf("v5Shape[%q]", probe), v5Shape[probe], wantV5)
		if _, ok := v4Shape[probe]; ok {
			assertColumnShapesEqual(t, fmt.Sprintf("v4Shape[%q]", probe), v4Shape[probe], wantV4)
		}
		if _, ok := v3Shape[probe]; ok {
			assertColumnShapesEqual(t, fmt.Sprintf("v3Shape[%q]", probe), v3Shape[probe], wantV3)
		}
		if _, ok := v2Shape[probe]; ok {
			assertColumnShapesEqual(t, fmt.Sprintf("v2Shape[%q]", probe), v2Shape[probe], wantV2)
		}
	})
}

// TestCwdV6ShapeEqualsV5PlusCwd 钉住 v6 相对 v5 只多出 conversations
// 末尾那一列 cwd，表集合与其它表的列都不变。
func TestCwdV6ShapeEqualsV5PlusCwd(t *testing.T) {
	if len(v6Shape) != len(v5Shape) {
		t.Fatalf("v6Shape 有 %d 张表，v5Shape 有 %d 张，期望表集合相同", len(v6Shape), len(v5Shape))
	}
	for table := range v5Shape {
		if _, ok := v6Shape[table]; !ok {
			t.Errorf("v5Shape 有表 %q，v6Shape 里没有", table)
		}
	}
	for table := range v6Shape {
		if _, ok := v5Shape[table]; !ok {
			t.Errorf("v6Shape 有表 %q，v5Shape 里没有", table)
		}
	}

	wantConv := append(cloneColumnShapes(v5Shape["conversations"]), columnShape{
		name: "cwd", typ: "TEXT", notnull: true,
	})
	assertColumnShapesEqual(t, `v6Shape["conversations"]`, v6Shape["conversations"], wantConv)

	var others []string
	for table := range v5Shape {
		if table == "conversations" {
			continue
		}
		others = append(others, table)
	}
	sort.Strings(others)
	for _, table := range others {
		assertColumnShapesEqual(t, fmt.Sprintf("v6Shape[%q]", table), v6Shape[table], v5Shape[table])
	}
}
