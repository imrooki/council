// open_readonly_v8_test.go 覆盖「停在 v8 的真库」：v8 有 archived_at，
// 但没有 v9 的 workspaces 表与 conversations.workspace_id。只读打开必须
// 能读、能报 schema_lag=true，且不把库升上去；再用 Open 走正常迁移，
// 应当升到 v9 并补齐两样新结构。
//
// 造库方式照 open_readonly_v7_test.go / schema_v9_test.go：把 migrations[:8]
// 的 DDL 逐条执行，再种数据，最后自己写 schema_migrations 的 1–8 行。
// 不能先 Open 最新库再 DROP COLUMN：那测到的是「最新结构 + 旧版本号」，
// 扫不出「按列名读 archived_at 能读到、按列名读 workspace_id 读不到」
// 这类真实旧库才会出现的问题。
package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// tstReadOnlyBuildV8Database 在 dir 下造一个只跑到 v8 的 council.db。
// 两条会话：一条没归档、一条已归档；v8 还没有工作区概念。
func tstReadOnlyBuildV8Database(t *testing.T, dir string) {
	t.Helper()
	if len(migrations) < 8 || migrations[6].version != 7 || migrations[7].version != 8 {
		t.Fatalf("migrations 的形状变了: len=%d，本辅助只该造 v8 库（下标 7 的 version 必须是 8）", len(migrations))
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

	// v8 的 conversations 这时只有 v1–v8 的列（id/title/created_at/team_id/
	// cwd/deleted_at/archived_at），没有 workspace_id。字段名写全，新列漏了
	// 会让造库直接失败——那正是要的效果：迁移形状一变，这个辅助先红。
	if _, err := db.ExecContext(ctx, `
INSERT INTO conversations (id, title, created_at, team_id, cwd, deleted_at, archived_at)
VALUES
    ('c-v8-1', 'v8 里的会话', '2026-10-05T00:00:01Z', '', 'D:\proj\v8', NULL, NULL),
    ('c-v8-2', 'v8 里已归档的会话', '2026-10-05T00:00:02Z', 'team-v8', '', NULL, '2026-10-06T00:00:00Z')`,
	); err != nil {
		t.Fatalf("造 v8 库时写入会话失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO participants (conv_id, agent_id, display_name, provider, active, joined_at, left_at)
VALUES ('c-v8-1', 'alpha', 'Alpha', 'claude', 1, '2026-10-05T01:00:00Z', NULL)`,
	); err != nil {
		t.Fatalf("造 v8 库时写入成员失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO messages
    (id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort,
     recipients, kind, text, thought, parent_id, created_at, meta)
VALUES
    ('m-v8-1', 'c-v8-1', 1, 'user', '', '', '', '[]', 'user',
     'v8 的第一条', '', NULL, '2026-10-05T03:00:01Z', '{}'),
    ('m-v8-2', 'c-v8-1', 2, 'alpha', 'claude', 'opus', 'high', '["alpha"]', 'agent',
     'v8 的回复', '', NULL, '2026-10-05T03:00:02Z', '{"k":"v"}')`,
	); err != nil {
		t.Fatalf("造 v8 库时写入消息失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO schema_migrations (version, applied_at) VALUES
    (1, '2026-10-05T00:00:00Z#1'),
    (2, '2026-10-05T00:00:00Z#2'),
    (3, '2026-10-05T00:00:00Z#3'),
    (4, '2026-10-05T00:00:00Z#4'),
    (5, '2026-10-05T00:00:00Z#5'),
    (6, '2026-10-05T00:00:00Z#6'),
    (7, '2026-10-05T00:00:00Z#7'),
    (8, '2026-10-05T00:00:00Z#8')`,
	); err != nil {
		t.Fatalf("造 v8 库时写 schema_migrations 失败: %v", err)
	}
}

// TestOpenReadOnlyV8DatabaseReadsWithLag 是第 5 条真机验收的库层落点：
// v8 库只读打开能读、archived_at 有值、workspace_id 缺列按空串读、
// SchemaLag=true，并且只读进程没有把库升到 v9。
func TestOpenReadOnlyV8DatabaseReadsWithLag(t *testing.T) {
	dir := t.TempDir()
	tstReadOnlyBuildV8Database(t, dir)

	ro, err := OpenReadOnly(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("只读打开 v8 库失败: %v", err)
	}
	ctx := tstCtx(t)

	if got := ro.SchemaVersion(); got != 8 {
		t.Errorf("SchemaVersion = %d，期望 8", got)
	}
	if got, want := ro.ExpectedSchemaVersion(), expectedSchemaVersion(); got != want {
		t.Errorf("ExpectedSchemaVersion = %d，期望 %d", got, want)
	}
	if !ro.SchemaLag() {
		t.Errorf("SchemaLag = false，期望 true（v8 < v%d）", expectedSchemaVersion())
	}

	t.Run("列表与归档", func(t *testing.T) {
		list, err := ro.Conversations(ctx)
		if err != nil {
			t.Fatalf("v8 库的 Conversations 失败: %v", err)
		}
		if len(list) != 1 || list[0].ID != "c-v8-1" {
			t.Fatalf("Conversations = %+v，期望只有未归档的 c-v8-1", list)
		}
		if list[0].Archived {
			t.Error("c-v8-1 的 archived_at 是 NULL，却读成已归档")
		}
		if list[0].WorkspaceID != "" {
			t.Errorf("v8 库缺 workspace_id，归属 = %q，期望空串", list[0].WorkspaceID)
		}
		arch, err := ro.ArchivedConversations(ctx)
		if err != nil {
			t.Fatalf("v8 库的 ArchivedConversations 失败: %v", err)
		}
		if len(arch) != 1 || arch[0].ID != "c-v8-2" || !arch[0].Archived {
			t.Fatalf("ArchivedConversations = %+v，期望只有已归档的 c-v8-2", arch)
		}
		if arch[0].WorkspaceID != "" {
			t.Errorf("已归档会话在 v8 下的 workspace_id = %q，期望空串", arch[0].WorkspaceID)
		}
	})

	t.Run("消息与成员照常", func(t *testing.T) {
		msgs, err := ro.Messages(ctx, "c-v8-1", 0, 0)
		if err != nil {
			t.Fatalf("v8 库的 Messages 失败: %v", err)
		}
		if len(msgs) != 2 || msgs[1].SenderProvider != "claude" || msgs[1].SenderModel != "opus" {
			t.Fatalf("Messages = %+v，期望按 seq 读回两条 v8 消息", msgs)
		}
		parts, err := ro.Participants(ctx, "c-v8-1", true)
		if err != nil || len(parts) != 1 || parts[0].AgentID != "alpha" {
			t.Fatalf("Participants = %+v, err=%v，期望 alpha", parts, err)
		}
	})

	t.Run("没有 workspaces 表时返回空列表", func(t *testing.T) {
		ws, err := ro.Workspaces(ctx)
		if err != nil {
			t.Fatalf("v8 库的 Workspaces 失败: %v", err)
		}
		if ws == nil || len(ws) != 0 {
			t.Errorf("Workspaces = %#v，期望非 nil 的空切片", ws)
		}
	})

	if err := ro.Close(); err != nil {
		t.Fatalf("关闭只读 Store 失败: %v", err)
	}

	raw := tstRawDB(t, filepath.Join(dir, dbFileName))
	if got := tstScanInt(t, raw, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`); got != 8 {
		t.Errorf("只读打开之后迁移账本 = %d，期望仍是 8（只读不该迁移）", got)
	}
	var cols []string
	rows, err := raw.Query(`SELECT name FROM pragma_table_info('conversations')`)
	if err != nil {
		t.Fatalf("读 conversations 列失败: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("扫描 conversations 列失败: %v", err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 conversations 列失败: %v", err)
	}
	if strings.Contains(strings.Join(cols, ","), "workspace_id") {
		t.Errorf("只读打开之后 conversations 多出了 workspace_id；只读进程不该迁移：%v", cols)
	}
	if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'`); got != 0 {
		t.Errorf("只读打开之后出现了 workspaces 表，期望没有（v8 库不该被升级）")
	}
}

// TestOpenUpgradesV8ToV9 从另一面钉「-rw 升级」：同一个 v8 库走正规 Open
// 之后必须是 v9、SchemaLag=false，并且 v9 的两样结构真的补上了。
func TestOpenUpgradesV8ToV9(t *testing.T) {
	dir := t.TempDir()
	tstReadOnlyBuildV8Database(t, dir)

	st, err := Open(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("Open 升级 v8 库失败: %v", err)
	}
	if got := st.SchemaVersion(); got != expectedSchemaVersion() {
		t.Errorf("升级后 SchemaVersion = %d，期望 %d", got, expectedSchemaVersion())
	}
	if st.SchemaLag() {
		t.Errorf("升级后 SchemaLag = true，期望 false")
	}
	// 旧会话必须还在、归档标记不能丢。
	arch, err := st.ArchivedConversations(tstCtx(t))
	if err != nil {
		t.Fatalf("升级后读归档列表失败: %v", err)
	}
	if len(arch) != 1 || arch[0].ID != "c-v8-2" || !arch[0].Archived {
		t.Fatalf("升级后的归档列表 = %+v，期望 c-v8-2 仍然已归档", arch)
	}
	if arch[0].WorkspaceID != "" {
		t.Errorf("升级后旧会话的 workspace_id = %q，期望仍是空串（未分组）", arch[0].WorkspaceID)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关闭升级后的 Store 失败: %v", err)
	}

	raw := tstRawDB(t, filepath.Join(dir, dbFileName))
	if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'`); got != 1 {
		t.Errorf("升级到 v9 之后没有 workspaces 表")
	}
	if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_conversations_workspace'`); got != 1 {
		t.Errorf("升级到 v9 之后没有 idx_conversations_workspace 索引")
	}
}
