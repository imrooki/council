// schema_v9_test.go 覆盖第三次「在线加表加列」：停留在 v8 的库打开后建出
// workspaces 表，并给 conversations 补上 workspace_id。
//
// 断言的重点与 v7 / v8 相同——除了新表与新列，七张业务表的老数据一条不少、
// 一个字不改；老会话的 workspace_id 全是 NULL；已经软删的会话仍然不可见。
// 另外多钉两条本片特有的：旧二进制（migrations 只到 v8）打开 v9 库必须报
// ErrSchemaMismatch；v9Shape 必须是 v8 的深拷贝，不是裸别名。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"council/internal/conversation"
)

// tstV9BuildV8Database 在 dir 下造一个只跑到 v8 的 council.db：
// migrations[0] 到 migrations[7] 的语句全部执行，再种一份跨全部七张业务表的
// 数据（每张表至少一行），schema_migrations 补齐 1 到 8 八行。
// 不引用 v9DDL：造库路径必须停在升级之前，否则测到的是空操作。
func tstV9BuildV8Database(t *testing.T, dir string) {
	t.Helper()
	if len(migrations) < 9 || migrations[7].version != 8 || migrations[8].version != 9 {
		t.Fatalf("migrations 的形状变了: len=%d，本辅助只该造 v8 库（下标 7 的 version 必须是 8，下标 8 必须是 9）", len(migrations))
	}

	dbPath := filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v8 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v8 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for i, m := range migrations[:8] {
		for _, stmt := range m.stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatalf("造 v8 库时执行 migrations[%d] version %d 失败: %v\n语句: %s", i, m.version, err, stmt)
			}
		}
	}

	convs := []struct {
		id, title, createdAt, teamID, cwd string
		deletedAt, archivedAt             any
	}{
		{"c-v8-1", "旧会话一", "2026-10-05T00:00:01Z", "", `D:\proj\one`, nil, nil},
		{"c-v8-2", "旧会话 二 (含括号)", "2026-10-05T00:00:02Z", "team-x", "", nil, nil},
		{"c-v8-3", "v8 里已经删掉的会话", "2026-10-05T00:00:03Z", "", "", "2026-10-06T00:00:00Z", nil},
		{"c-v8-4", "v8 里已经归档的会话", "2026-10-05T00:00:04Z", "", "", nil, "2026-10-06T01:00:00Z"},
	}
	for _, c := range convs {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO conversations (id, title, created_at, team_id, cwd, deleted_at, archived_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			c.id, c.title, c.createdAt, c.teamID, c.cwd, c.deletedAt, c.archivedAt,
		); err != nil {
			t.Fatalf("造 v8 库时写入会话 %s 失败: %v", c.id, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO participants (conv_id, agent_id, display_name, provider, active, joined_at, left_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"c-v8-1", "alpha", "Alpha", "claude", 1, "2026-10-05T01:00:00Z", nil,
	); err != nil {
		t.Fatalf("造 v8 库时写入成员失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO messages
    (id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort, recipients, kind, text, thought, parent_id, created_at, meta)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"m-v8-1", "c-v8-1", 1, "alpha", "claude", "opus", "high", `["alpha"]`, "agent",
		"v8 的正文", "", nil, "2026-10-05T03:00:01Z", `{"k":"v"}`,
	); err != nil {
		t.Fatalf("造 v8 库时写入消息失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO cursors (conv_id, agent_id, seq, pending, updated_at)
VALUES (?, ?, ?, ?, ?)`,
		"c-v8-1", "alpha", 1, "[1]", "2026-10-05T04:00:00Z",
	); err != nil {
		t.Fatalf("造 v8 库时写入游标失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO turn_usage (
    turn_id, conv_id, agent_id, provider_session_id, seq, outcome,
    input_tokens, cache_create, cache_read, output_tokens, thinking_tokens,
    cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes,
    delivered_lo, delivered_hi, created_at, identity_fp, round_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"turn-v8-1", "c-v8-1", "alpha", "ps-v8-1", 1, 1,
		200, 0, 50, 30, 5,
		0.25, 0.75, 0, 1500, 700,
		nil, 1, "2026-10-05T05:00:00Z", "fp-v8", "round-v8",
	); err != nil {
		t.Fatalf("造 v8 库时写入 turn_usage 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_sessions (
    conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd,
    have_cost_total, last_turn_at, identity_fp, spoken, baseline_suspect
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"c-v8-1", "alpha", "ps-v8-1", 1, 0.75,
		1, "2026-10-05T05:01:00Z", "fp-v8", 1, 0,
	); err != nil {
		t.Fatalf("造 v8 库时写入 provider_sessions 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO inflight_turns (
    turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id,
    cursor_seq, scanned_to, started_at, phase, tier, work_dir
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"inflight-v8-1", "c-v8-2", "alpha", "claude", "planned-v8", "", 0, 0,
		"2026-10-05T06:00:00Z", "begun", "default", `D:\work`,
	); err != nil {
		t.Fatalf("造 v8 库时写入 inflight_turns 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?), (2, ?), (3, ?), (4, ?), (5, ?), (6, ?), (7, ?), (8, ?)`,
		tstFixedCreatedAt+"#1", tstFixedCreatedAt+"#2", tstFixedCreatedAt+"#3",
		tstFixedCreatedAt+"#4", tstFixedCreatedAt+"#5", tstFixedCreatedAt+"#6",
		tstFixedCreatedAt+"#7", tstFixedCreatedAt+"#8",
	); err != nil {
		t.Fatalf("造 v8 库时写 schema_migrations 失败: %v", err)
	}
}

// tstV9Snapshot 是迁移前后要比对的七张业务表。conversations 只取 v8 就有的
// 七列：workspace_id 是新列，老数据那七列（含 deleted_at / archived_at）
// 必须一字不变。其余表 SELECT *：v9 不碰它们，列数变了本身就该是一条失败。
func tstV9Snapshot(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	return map[string][]string{
		"conversations":     tstCwdDump(t, db, `SELECT id, title, created_at, team_id, cwd, deleted_at, archived_at FROM conversations ORDER BY id`),
		"participants":      tstCwdDump(t, db, `SELECT * FROM participants ORDER BY conv_id, agent_id`),
		"messages":          tstCwdDump(t, db, `SELECT * FROM messages ORDER BY conv_id, seq`),
		"cursors":           tstCwdDump(t, db, `SELECT * FROM cursors ORDER BY conv_id, agent_id`),
		"turn_usage":        tstCwdDump(t, db, `SELECT * FROM turn_usage ORDER BY turn_id`),
		"provider_sessions": tstCwdDump(t, db, `SELECT * FROM provider_sessions ORDER BY conv_id, agent_id`),
		"inflight_turns":    tstCwdDump(t, db, `SELECT * FROM inflight_turns ORDER BY turn_id`),
		"schema_migrations": tstCwdDump(t, db, `SELECT version, applied_at FROM schema_migrations ORDER BY version`),
	}
}

// tstV9BusinessTables 是 v9 迁移前后要比对内容的七张业务表。
var tstV9BusinessTables = []string{
	"conversations", "participants", "messages", "cursors",
	"turn_usage", "provider_sessions", "inflight_turns",
}

// tstV9TableCounts 数每张业务表的行数。
func tstV9TableCounts(t *testing.T, db *sql.DB) map[string]int64 {
	t.Helper()
	out := make(map[string]int64, len(tstV9BusinessTables))
	for _, table := range tstV9BusinessTables {
		out[table] = tstScanInt(t, db, `SELECT COUNT(*) FROM `+table)
	}
	return out
}

// TestSchemaV9MigrationV8ToV9 造一个停在 v8 的库，用正式的 Open 升到 v9，
// 钉住：账本只追加一行、七张表的老数据一行不少、workspaces 建出来了且是空的、
// workspace_id 是 conversations 的最后一列且老行全为 NULL、v8 里已经删掉的
// 会话仍然不可见、升级后的库能正常建组与设置归属。
func TestSchemaV9MigrationV8ToV9(t *testing.T) {
	dir := t.TempDir()
	tstV9BuildV8Database(t, dir)

	beforeDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, dbFileName))+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("取迁移前快照时打开连接失败: %v", err)
	}
	before := tstV9Snapshot(t, beforeDB)
	beforeCounts := tstV9TableCounts(t, beforeDB)
	if err := beforeDB.Close(); err != nil {
		t.Fatalf("取完迁移前快照后关闭连接失败: %v", err)
	}

	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	raw := tstRawDB(t, s.Path())

	t.Run("schema_migrations 追加 v9 且旧行不被重写", func(t *testing.T) {
		wantVer := migrations[len(migrations)-1].version
		if got := tstScanInt(t, raw, `SELECT MAX(version) FROM schema_migrations`); got != wantVer {
			t.Errorf("MAX(version) = %d，期望 %d（migrations 里最后一版的 version）", got, wantVer)
		}
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM schema_migrations`); got != int64(len(migrations)) {
			t.Errorf("schema_migrations 行数 = %d，期望 %d（每版一行）", got, len(migrations))
		}
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

	t.Run("七张业务表的老数据一行不少、一字未变", func(t *testing.T) {
		afterCounts := tstV9TableCounts(t, raw)
		for table, want := range beforeCounts {
			if got := afterCounts[table]; got != want {
				t.Errorf("%s 行数 = %d，期望 %d（迁移只许加表加列）", table, got, want)
			}
		}
		after := tstV9Snapshot(t, raw)
		for _, table := range tstV9BusinessTables {
			tstCwdEqualLines(t, table, after[table], before[table])
		}
		if got := tstScanText(t, raw,
			`SELECT deleted_at FROM conversations WHERE id = 'c-v8-3'`); got != "2026-10-06T00:00:00Z" {
			t.Errorf("v8 里已删会话的 deleted_at = %q，期望原样保留", got)
		}
		if got := tstScanText(t, raw,
			`SELECT archived_at FROM conversations WHERE id = 'c-v8-4'`); got != "2026-10-06T01:00:00Z" {
			t.Errorf("v8 里已归档会话的 archived_at = %q，期望原样保留", got)
		}
	})

	t.Run("workspaces 表建出来且是空的", func(t *testing.T) {
		if got := tstScanInt(t, raw,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'`); got != 1 {
			t.Errorf("workspaces 表数 = %d，期望 1", got)
		}
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM workspaces`); got != 0 {
			t.Errorf("workspaces 行数 = %d，期望 0（迁移不该凭空造分组）", got)
		}
		rows, err := raw.Query(`SELECT name, type, "notnull", dflt_value FROM pragma_table_info('workspaces') ORDER BY cid`)
		if err != nil {
			t.Fatalf("读 pragma_table_info('workspaces') 失败: %v", err)
		}
		defer rows.Close()
		var names []string
		var sawSortDefault bool
		for rows.Next() {
			var name, typ string
			var notnull int64
			var dflt any
			if err := rows.Scan(&name, &typ, &notnull, &dflt); err != nil {
				t.Fatalf("扫描 pragma_table_info 失败: %v", err)
			}
			names = append(names, name)
			if name == "sort_order" {
				sawSortDefault = dflt != nil && fmt.Sprint(dflt) == "0"
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("遍历 pragma_table_info 失败: %v", err)
		}
		wantNames := []string{"id", "name", "sort_order", "created_at"}
		if !reflect.DeepEqual(names, wantNames) {
			t.Errorf("workspaces 列序 = %v，期望 %v", names, wantNames)
		}
		if !sawSortDefault {
			t.Error("workspaces.sort_order 的 dflt_value 不是 0，DDL 与契约不符")
		}
	})

	t.Run("idx_conversations_workspace 建出来了", func(t *testing.T) {
		if got := tstScanInt(t, raw,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_conversations_workspace'`); got != 1 {
			t.Errorf("idx_conversations_workspace 的数量 = %d，期望 1", got)
		}
	})

	t.Run("workspace_id 是 conversations 的最后一列且可空、无默认值", func(t *testing.T) {
		rows, err := raw.Query(`SELECT name, type, "notnull", dflt_value FROM pragma_table_info('conversations') ORDER BY cid`)
		if err != nil {
			t.Fatalf("读 pragma_table_info('conversations') 失败: %v", err)
		}
		defer rows.Close()
		var names []string
		var sawWorkspace bool
		var workspaceType string
		var workspaceNotNull int64
		var workspaceDefault any
		for rows.Next() {
			var name, typ string
			var notnull int64
			var dflt any
			if err := rows.Scan(&name, &typ, &notnull, &dflt); err != nil {
				t.Fatalf("扫描 pragma_table_info 失败: %v", err)
			}
			names = append(names, name)
			if name == "workspace_id" {
				sawWorkspace = true
				workspaceType = typ
				workspaceNotNull = notnull
				workspaceDefault = dflt
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("遍历 pragma_table_info 失败: %v", err)
		}
		wantNames := []string{"id", "title", "created_at", "team_id", "cwd", "deleted_at", "archived_at", "workspace_id"}
		if !reflect.DeepEqual(names, wantNames) {
			t.Errorf("conversations 列序 = %v，期望 %v（ALTER 把 workspace_id 加在最后一列）", names, wantNames)
		}
		if !sawWorkspace {
			t.Fatal("pragma_table_info 里没有 workspace_id 列")
		}
		if workspaceType != "TEXT" {
			t.Errorf("workspace_id type = %q，期望 TEXT", workspaceType)
		}
		if workspaceNotNull != 0 {
			t.Errorf("workspace_id notnull = %d，期望 0（NULL 表示未分组）", workspaceNotNull)
		}
		if workspaceDefault != nil {
			t.Errorf("workspace_id dflt_value = %#v，期望 NULL（不要给未分组的旧行编一个空串默认值）", workspaceDefault)
		}
	})

	t.Run("老会话的 workspace_id 全是 NULL", func(t *testing.T) {
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations WHERE workspace_id IS NOT NULL`); got != 0 {
			t.Errorf("迁移后有 %d 行 workspace_id 非 NULL，期望 0", got)
		}
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations`); got != int64(beforeCounts["conversations"]) {
			t.Errorf("conversations 行数 = %d，期望 %d", got, beforeCounts["conversations"])
		}
	})

	t.Run("validateSchema 通过", func(t *testing.T) {
		if err := s.validateSchema(ctx); err != nil {
			t.Errorf("validateSchema 返回错误，期望 nil: %v", err)
		}
	})

	t.Run("升级后的库能读老数据、已删的仍然不可见", func(t *testing.T) {
		list, err := s.Conversations(ctx)
		if err != nil {
			t.Fatalf("Conversations 失败: %v", err)
		}
		// c-v8-3 已删、c-v8-4 已归档，默认列表只剩两场。
		if len(list) != 2 {
			t.Fatalf("Conversations 返回 %d 场，期望 2 场（已删的不在、已归档的不在）", len(list))
		}
		for _, conv := range list {
			if conv.WorkspaceID != "" {
				t.Errorf("%s 的 WorkspaceID = %q，v8 老行应当全是未分组", conv.ID, conv.WorkspaceID)
			}
			if conv.Archived {
				t.Errorf("%s 的 Archived = true，它不在默认列表里", conv.ID)
			}
		}
		archived, err := s.ArchivedConversations(ctx)
		if err != nil {
			t.Fatalf("ArchivedConversations 失败: %v", err)
		}
		if len(archived) != 1 || archived[0].ID != "c-v8-4" {
			t.Errorf("归档列表 = %+v，期望只有 c-v8-4", archived)
		}
		if archived[0].WorkspaceID != "" {
			t.Errorf("归档会话的 WorkspaceID = %q，期望空串（归档列表也带 workspace_id）", archived[0].WorkspaceID)
		}
		one, err := s.Conversation(ctx, "c-v8-2")
		if err != nil {
			t.Fatalf("Conversation(c-v8-2) 失败: %v", err)
		}
		if one.Title != "旧会话 二 (含括号)" || one.TeamID != "team-x" || one.WorkspaceID != "" {
			t.Errorf("c-v8-2 = %+v，期望标题/team 原样且未分组", one)
		}
		if _, err := s.Conversation(ctx, "c-v8-3"); !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("升级后 Conversation(c-v8-3) = %v，期望 ErrConversationNotFound", err)
		}
		if msgs, err := s.Messages(ctx, "c-v8-1", 0, 0); err != nil || len(msgs) != 1 {
			t.Errorf("升级后 Messages(c-v8-1) = %+v, err=%v，期望一条", msgs, err)
		}
		if usages, err := s.TurnUsages(ctx, "c-v8-1", "", -1); err != nil || len(usages) != 1 {
			t.Errorf("升级后 TurnUsages(c-v8-1) = %+v, err=%v，期望一条", usages, err)
		}
	})

	t.Run("升级后的库能建组、设置归属、删组后会话仍在", func(t *testing.T) {
		ws, err := s.CreateWorkspace(ctx, " 升级后新建 ")
		if err != nil {
			t.Fatalf("升级后 CreateWorkspace 失败: %v", err)
		}
		if ws.Name != "升级后新建" {
			t.Errorf("CreateWorkspace 的名字 = %q，期望去空白之后的 %q", ws.Name, "升级后新建")
		}
		id := ws.ID
		if err := s.SetConversationWorkspace(ctx, "c-v8-1", &id); err != nil {
			t.Fatalf("升级后 SetConversationWorkspace 失败: %v", err)
		}
		one, err := s.Conversation(ctx, "c-v8-1")
		if err != nil || one.WorkspaceID != ws.ID {
			t.Errorf("设置归属后 Conversation = %+v, err=%v，期望 WorkspaceID=%s", one, err, ws.ID)
		}
		if err := s.DeleteWorkspace(ctx, ws.ID); err != nil {
			t.Fatalf("升级后 DeleteWorkspace 失败: %v", err)
		}
		one, err = s.Conversation(ctx, "c-v8-1")
		if err != nil {
			t.Fatalf("删组后 Conversation 失败: %v（会话不该跟着没）", err)
		}
		if one.WorkspaceID != "" {
			t.Errorf("删组后 WorkspaceID = %q，期望空串", one.WorkspaceID)
		}
	})

	closeFn()
	s2, _ := tstOpenAt(t, dir)
	ctx2 := tstCtx(t)
	raw2 := tstRawDB(t, s2.Path())

	t.Run("重开后迁移不重复执行、老数据仍在", func(t *testing.T) {
		if got := tstScanInt(t, raw2, `SELECT COUNT(*) FROM schema_migrations`); got != int64(len(migrations)) {
			t.Errorf("重开后 schema_migrations 行数 = %d，期望 %d（CREATE TABLE / ALTER 重复执行会失败，Open 本身就不会成功）", got, len(migrations))
		}
		archived, err := s2.ArchivedConversations(ctx2)
		if err != nil {
			t.Fatalf("重开后 ArchivedConversations 失败: %v", err)
		}
		if len(archived) != 1 || archived[0].ID != "c-v8-4" {
			t.Errorf("重开后归档列表 = %+v，期望仍然只有 c-v8-4", archived)
		}
		if got := tstScanInt(t, raw2, `SELECT COUNT(*) FROM workspaces`); got != 0 {
			t.Errorf("重开后 workspaces 行数 = %d，期望 0（上一段已把唯一那个删掉）", got)
		}
	})
}

// TestSchemaV9V8BinaryCannotOpenV9Database 模拟旧二进制：migrations 只留到
// v8，打开 v9 库应当以 ErrSchemaMismatch 失败并点名多出 workspace_id。
// 这是预期行为：v8 二进制不认识工作区，让它继续跑会把已经分组的会话
// 全画成未分组，而且它自己的迁移账本写着 8，永远不会补上 workspaces 表。
func TestSchemaV9V8BinaryCannotOpenV9Database(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "v9 库里的会话")
	ws, err := s.CreateWorkspace(ctx, "v9 分组")
	if err != nil {
		t.Fatalf("CreateWorkspace 失败: %v", err)
	}
	if err := s.SetConversationWorkspace(ctx, conv.ID, &ws.ID); err != nil {
		t.Fatalf("SetConversationWorkspace 失败: %v", err)
	}
	closeFn()

	orig := migrations
	migrations = migrations[:8]
	t.Cleanup(func() { migrations = orig })

	opened, err := Open(tstCtx(t), dir)
	if opened != nil {
		if cerr := opened.Close(); cerr != nil {
			t.Errorf("失败的 Open 仍返回了 store，Close 出错: %v", cerr)
		}
	}
	if err == nil {
		t.Fatal("v8 二进制打开 v9 库应当失败，实际成功")
	}
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("errors.Is(ErrSchemaMismatch) = false，实际错误: %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"多出列 workspace_id", "不要删库"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误文本不含 %q；实际: %s", want, msg)
		}
	}

	// 还原成当前二进制再开一次：库没有被旧二进制改坏，分组与归属仍在。
	migrations = orig
	reopened, _ := tstOpenAt(t, dir)
	reopenedConv, err := reopened.Conversation(tstCtx(t), conv.ID)
	if err != nil {
		t.Fatalf("重开后 Conversation 失败: %v", err)
	}
	if reopenedConv.WorkspaceID != ws.ID {
		t.Errorf("重开后会话的 workspace_id = %q，期望 %q", reopenedConv.WorkspaceID, ws.ID)
	}
	all, err := reopened.Workspaces(tstCtx(t))
	if err != nil {
		t.Fatalf("重开后 Workspaces 失败: %v", err)
	}
	if len(all) != 1 || all[0].ID != ws.ID || all[0].ConversationCount != 1 {
		t.Errorf("重开后工作区 = %+v，期望只有 %s 且会话数为 1", all, ws.ID)
	}
}

// TestSchemaV9ShapeIsV8PlusWorkspacesAndWorkspaceID 钉住 v9 相对 v8 只多出
// workspaces 表与 conversations 末尾那一列，其它表的列都不变；同时钉住
// v9Shape 是深拷贝，不是 v8Shape 的裸别名。
func TestSchemaV9ShapeIsV8PlusWorkspacesAndWorkspaceID(t *testing.T) {
	if len(v9Shape) != len(v8Shape)+1 {
		t.Fatalf("v9Shape 有 %d 张表，v8Shape 有 %d 张，期望正好多一张 workspaces", len(v9Shape), len(v8Shape))
	}
	for table := range v8Shape {
		if _, ok := v9Shape[table]; !ok {
			t.Errorf("v8Shape 有表 %q，v9Shape 里没有", table)
		}
	}
	if _, ok := v9Shape["workspaces"]; !ok {
		t.Error("v9Shape 里没有 workspaces")
	}
	if _, ok := v8Shape["workspaces"]; ok {
		t.Error("v8Shape 里不该有 workspaces（它是 v9 才建的）")
	}

	wantConv := append(cloneColumnShapes(v8Shape["conversations"]), columnShape{
		name: "workspace_id", typ: "TEXT", notnull: false,
	})
	assertColumnShapesEqual(t, `v9Shape["conversations"]`, v9Shape["conversations"], wantConv)

	wantWorkspaces := []columnShape{
		{name: "id", typ: "TEXT", notnull: false},
		{name: "name", typ: "TEXT", notnull: true},
		{name: "sort_order", typ: "INTEGER", notnull: true},
		{name: "created_at", typ: "TEXT", notnull: true},
	}
	assertColumnShapesEqual(t, `v9Shape["workspaces"]`, v9Shape["workspaces"], wantWorkspaces)

	var others []string
	for table := range v8Shape {
		if table == "conversations" {
			continue
		}
		others = append(others, table)
	}
	sort.Strings(others)
	for _, table := range others {
		assertColumnShapesEqual(t, fmt.Sprintf("v9Shape[%q]", table), v9Shape[table], v8Shape[table])
	}

	// 改 conversations 的键不影响 v8Shape：append 一定换新数组，但整体
	// 重新赋值 v9Shape["conversations"] 在裸别名时会把 v8Shape 一起改掉。
	t.Run("改 conversations 键不影响 v8Shape", func(t *testing.T) {
		wantV8 := cloneColumnShapes(v8Shape["conversations"])
		orig := v9Shape["conversations"]
		t.Cleanup(func() { v9Shape["conversations"] = orig })
		v9Shape["conversations"] = append(cloneColumnShapes(orig),
			columnShape{name: "tst_v9_alias_probe_key", typ: "TEXT", notnull: false})
		assertColumnShapesEqual(t, `v8Shape["conversations"]`, v8Shape["conversations"], wantV8)
	})

	// 挑一张 v9 没加列的表中按下标改元素，抓「只 make 了新 map、切片仍共享
	// 底层数组」这种坏法。conversations 因为 append 过，按下标改测不出来。
	t.Run("改未加列表的元素不影响 v8Shape", func(t *testing.T) {
		var candidates []string
		for table, cols := range v9Shape {
			if table == "conversations" || table == "workspaces" {
				continue
			}
			if len(cols) > 0 && len(cols) == len(v8Shape[table]) {
				candidates = append(candidates, table)
			}
		}
		if len(candidates) == 0 {
			t.Fatal("v9Shape 里找不到一张列数与 v8Shape 相同的非空表, 本用例依赖的前提不成立")
		}
		sort.Strings(candidates)
		probe := candidates[0]
		wantV8 := cloneColumnShapes(v8Shape[probe])
		origElem := v9Shape[probe][0]
		t.Cleanup(func() { v9Shape[probe][0] = origElem })
		v9Shape[probe][0] = columnShape{
			name: "tst_v9_alias_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}
		assertColumnShapesEqual(t, fmt.Sprintf(`v8Shape[%q]`, probe), v8Shape[probe], wantV8)
	})
}
