// open_readonly_v7_test.go 覆盖 A3 的核心验收：一个真正停在 v7 的旧库
// （缺 archived_at 与 workspace_id）必须能在只读模式下列出会话、读详情、
// 读消息，并报告 schema_lag=true。
//
// 造库方式照既有 schema_v7_test.go 的做法：把 migrations[:7] 的 DDL 逐条
// 执行，再种数据，最后自己写 schema_migrations 的 1–7 行。不能先 Open 再
// 删列：那测到的是「最新结构 + 旧版本号」，扫不出按列名取列会失败这件事。
package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/conversation"
)

// tstReadOnlyBuildV7Database 在 dir 下造一个只跑到 v7 的 council.db。
// v7 的 conversations 有 deleted_at，但没有 archived_at 与 workspace_id，
// 也没有 workspaces 表；这正是迁移到 v8/v9 之前那些库的形状。
func tstReadOnlyBuildV7Database(t *testing.T, dir string) {
	t.Helper()
	if len(migrations) < 7 || migrations[6].version != 7 {
		t.Fatalf("migrations 的形状变了: len=%d，本辅助只该造 v7 库（下标 6 的 version 必须是 7）", len(migrations))
	}

	dbPath := filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v7 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v7 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for i, m := range migrations[:7] {
		for _, stmt := range m.stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatalf("造 v7 库时执行 migrations[%d] version %d 失败: %v\n语句: %s", i, m.version, err, stmt)
			}
		}
	}

	// 两条会话：一条可见、一条已软删。v7 已经有 deleted_at，列表必须过滤后者。
	if _, err := db.ExecContext(ctx, `
INSERT INTO conversations (id, title, created_at, team_id, cwd, deleted_at)
VALUES
    ('c-v7-1', 'v7 里的会话', '2026-09-20T00:00:01Z', 'team-v7', 'D:\proj\v7', NULL),
    ('c-v7-2', 'v7 里已软删的会话', '2026-09-20T00:00:02Z', '', '', '2026-09-21T00:00:00Z')`,
	); err != nil {
		t.Fatalf("造 v7 库时写入会话失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO participants (conv_id, agent_id, display_name, provider, active, joined_at, left_at)
VALUES ('c-v7-1', 'alpha', 'Alpha', 'claude', 1, '2026-09-20T01:00:00Z', NULL)`,
	); err != nil {
		t.Fatalf("造 v7 库时写入成员失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO messages
    (id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort,
     recipients, kind, text, thought, parent_id, created_at, meta)
VALUES
    ('m-v7-1', 'c-v7-1', 1, 'user', '', '', '', '[]', 'user',
     'v7 的第一条', '', NULL, '2026-09-20T03:00:01Z', '{}'),
    ('m-v7-2', 'c-v7-1', 2, 'alpha', 'claude', 'opus', 'high', '["alpha"]', 'agent',
     'v7 的回复', '', NULL, '2026-09-20T03:00:02Z', '{"k":"v"}')`,
	); err != nil {
		t.Fatalf("造 v7 库时写入消息失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO schema_migrations (version, applied_at) VALUES
    (1, '2026-09-20T00:00:00Z#1'),
    (2, '2026-09-20T00:00:00Z#2'),
    (3, '2026-09-20T00:00:00Z#3'),
    (4, '2026-09-20T00:00:00Z#4'),
    (5, '2026-09-20T00:00:00Z#5'),
    (6, '2026-09-20T00:00:00Z#6'),
    (7, '2026-09-20T00:00:00Z#7')`,
	); err != nil {
		t.Fatalf("造 v7 库时写 schema_migrations 失败: %v", err)
	}
}

// TestOpenReadOnlyV7DatabaseReadsWithLag 是 A3 的验收用例：v7 库在只读模式下
// 列表、详情与消息都必须正常，archived_at / workspace_id 按零值读，并且
// SchemaLag 为 true。同时钉住「只读进程没有写库」：迁移账本仍是 v7，
// conversations 也没有被补上 v8/v9 的列。
func TestOpenReadOnlyV7DatabaseReadsWithLag(t *testing.T) {
	dir := t.TempDir()
	tstReadOnlyBuildV7Database(t, dir)

	ro, err := OpenReadOnly(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("只读打开 v7 库失败: %v", err)
	}
	ctx := tstCtx(t)

	if got := ro.SchemaVersion(); got != 7 {
		t.Errorf("SchemaVersion = %d，期望 7", got)
	}
	if got, want := ro.ExpectedSchemaVersion(), expectedSchemaVersion(); got != want {
		t.Errorf("ExpectedSchemaVersion = %d，期望 %d", got, want)
	}
	if !ro.SchemaLag() {
		t.Errorf("SchemaLag = false，期望 true（v7 < v%d）", expectedSchemaVersion())
	}

	t.Run("列表", func(t *testing.T) {
		list, err := ro.Conversations(ctx)
		if err != nil {
			t.Fatalf("v7 库的 Conversations 失败: %v", err)
		}
		if len(list) != 1 || list[0].ID != "c-v7-1" {
			t.Fatalf("Conversations = %+v，期望只有 c-v7-1（c-v7-2 已软删）", list)
		}
		got := list[0]
		if got.Title != "v7 里的会话" || got.TeamID != "team-v7" || got.Cwd != `D:\proj\v7` {
			t.Errorf("v7 会话的既有列 = %+v，期望标题/team/cwd 原样", got)
		}
		if got.Archived {
			t.Error("v7 库缺 archived_at，会话却读成已归档；缺列应按「没归档」的零值读")
		}
		if got.WorkspaceID != "" {
			t.Errorf("v7 库缺 workspace_id，会话归属 = %q，期望空串（未分组）", got.WorkspaceID)
		}
	})

	t.Run("归档列表为空", func(t *testing.T) {
		arch, err := ro.ArchivedConversations(ctx)
		if err != nil {
			t.Fatalf("v7 库的 ArchivedConversations 失败: %v", err)
		}
		if len(arch) != 0 {
			t.Errorf("v7 库没有归档概念，归档列表 = %+v，期望空", arch)
		}
	})

	t.Run("详情", func(t *testing.T) {
		one, err := ro.Conversation(ctx, "c-v7-1")
		if err != nil {
			t.Fatalf("v7 库的 Conversation(c-v7-1) 失败: %v", err)
		}
		if one.ID != "c-v7-1" || one.Title != "v7 里的会话" || one.TeamID != "team-v7" {
			t.Errorf("详情 = %+v，期望既有列原样", one)
		}
		if one.Archived || one.WorkspaceID != "" {
			t.Errorf("详情里缺列读成了 %+v，期望 Archived=false、WorkspaceID 空串", one)
		}
		if _, err := ro.Conversation(ctx, "c-v7-2"); !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("已软删会话的详情 = %v，期望 ErrConversationNotFound", err)
		}
	})

	t.Run("消息与成员", func(t *testing.T) {
		msgs, err := ro.Messages(ctx, "c-v7-1", 0, 0)
		if err != nil {
			t.Fatalf("v7 库的 Messages 失败: %v", err)
		}
		if len(msgs) != 2 || msgs[0].Seq != 1 || msgs[0].Text != "v7 的第一条" ||
			msgs[1].Seq != 2 || msgs[1].Text != "v7 的回复" {
			t.Fatalf("Messages = %+v，期望按 seq 读回两条 v7 消息", msgs)
		}
		if msgs[1].SenderProvider != "claude" || msgs[1].SenderModel != "opus" {
			t.Errorf("消息的 sender 三列 = %+v，期望原样", msgs[1])
		}
		if msgs, err := ro.Messages(ctx, "c-v7-2", 0, 0); err != nil || len(msgs) != 0 {
			t.Errorf("已软删会话的 Messages = %+v, err=%v，期望空切片", msgs, err)
		}
		parts, err := ro.Participants(ctx, "c-v7-1", true)
		if err != nil || len(parts) != 1 || parts[0].AgentID != "alpha" {
			t.Errorf("Participants = %+v, err=%v，期望 alpha", parts, err)
		}
	})

	t.Run("没有 workspaces 表时返回空列表", func(t *testing.T) {
		ws, err := ro.Workspaces(ctx)
		if err != nil {
			t.Fatalf("v7 库的 Workspaces 失败: %v", err)
		}
		if ws == nil || len(ws) != 0 {
			t.Errorf("Workspaces = %#v，期望非 nil 的空切片", ws)
		}
	})

	if err := ro.Close(); err != nil {
		t.Fatalf("关闭只读 Store 失败: %v", err)
	}

	raw := tstRawDB(t, filepath.Join(dir, dbFileName))
	t.Run("只读打开没有补迁移", func(t *testing.T) {
		if got := tstScanInt(t, raw, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`); got != 7 {
			t.Errorf("只读打开之后迁移账本 = %d，期望仍是 7", got)
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
		for _, banned := range []string{"archived_at", "workspace_id"} {
			for _, col := range cols {
				if col == banned {
					t.Errorf("只读打开之后 conversations 多出了 %q 列；只读进程不该迁移", banned)
				}
			}
		}
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'`); got != 0 {
			t.Errorf("只读打开之后出现了 workspaces 表，期望没有（v7 库不该被升级）")
		}
		if !strings.Contains(strings.Join(cols, ","), "deleted_at") {
			t.Errorf("v7 库的 conversations 列里没有 deleted_at: %v", cols)
		}
	})
}
