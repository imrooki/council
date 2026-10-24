// schema_v7_test.go 覆盖第一次在线改表：停留在 v6 的库打开后补上
// conversations.deleted_at。断言的重点不是「迁移能跑」，而是「除了这一列，
// 其余数据一条不少、一个字不改」——这是全项目第一次动 schema，v7 之后
// 任何一次误改都可能把用户已有的会话、消息、账与在飞记录留在旧结构里。
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

// tstV7BuildV6Database 在 dir 下造一个只跑到 v6 的 council.db：
// migrations[0] 到 migrations[5] 的语句全部执行，再种一份跨全部七张业务表的
// 数据（每张表至少一行），schema_migrations 补齐 1 到 6 六行。
// 不引用 v7DDL：造库路径必须停在升级之前，否则测到的是空操作。
func tstV7BuildV6Database(t *testing.T, dir string) {
	t.Helper()
	if len(migrations) < 7 || migrations[5].version != 6 || migrations[6].version != 7 {
		t.Fatalf("migrations 的形状变了: len=%d，本辅助只该造 v6 库（下标 5 的 version 必须是 6，下标 6 必须是 7）", len(migrations))
	}

	dbPath := filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v6 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v6 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for i, m := range migrations[:6] {
		for _, stmt := range m.stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatalf("造 v6 库时执行 migrations[%d] version %d 失败: %v\n语句: %s", i, m.version, err, stmt)
			}
		}
	}

	convs := []struct {
		id, title, createdAt, teamID, cwd string
	}{
		{"c-v6-1", "旧会话一", "2026-09-20T00:00:01Z", "", `D:\proj\one`},
		{"c-v6-2", "旧会话 二 (含括号)", "2026-09-20T00:00:02Z", "team-x", ""},
		{"c-v6-3", "旧会话三", "2026-09-20T00:00:03Z", "", ""},
	}
	for _, c := range convs {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO conversations (id, title, created_at, team_id, cwd) VALUES (?, ?, ?, ?, ?)`,
			c.id, c.title, c.createdAt, c.teamID, c.cwd,
		); err != nil {
			t.Fatalf("造 v6 库时写入会话 %s 失败: %v", c.id, err)
		}
	}

	parts := []struct {
		convID, agentID, displayName, provider, joinedAt string
		active                                           int
		leftAt                                           any
	}{
		{"c-v6-1", "alpha", "Alpha", "claude", "2026-09-20T01:00:00Z", 1, nil},
		{"c-v6-1", "beta", "Beta", "codex", "2026-09-20T01:00:01Z", 0, "2026-09-20T02:00:00Z"},
	}
	for _, p := range parts {
		if _, err := db.ExecContext(ctx, `
INSERT INTO participants (conv_id, agent_id, display_name, provider, active, joined_at, left_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			p.convID, p.agentID, p.displayName, p.provider, p.active, p.joinedAt, p.leftAt,
		); err != nil {
			t.Fatalf("造 v6 库时写入成员 %s/%s 失败: %v", p.convID, p.agentID, err)
		}
	}

	msgs := []struct {
		id, convID, senderID, senderProvider, senderModel, senderEffort, recipients, kind, text, thought, createdAt, meta string
		seq                                                                                                               int64
		parentID                                                                                                          any
	}{
		{"m-1", "c-v6-1", "user", "", "", "", "[]", "user", "第一条用户话", "", "2026-09-20T03:00:01Z", "{}", 1, nil},
		{"m-2", "c-v6-1", "alpha", "claude", "opus", "high", `["alpha"]`, "agent", "代理回复含中文", "", "2026-09-20T03:00:02Z", `{"k":"v"}`, 2, nil},
		{"m-3", "c-v6-2", "alpha", "claude", "sonnet", "", "[]", "agent", "另一场的一条", "", "2026-09-20T03:00:03Z", "{}", 1, nil},
	}
	for _, m := range msgs {
		if _, err := db.ExecContext(ctx, `
INSERT INTO messages
    (id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort, recipients, kind, text, thought, parent_id, created_at, meta)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.id, m.convID, m.seq, m.senderID, m.senderProvider, m.senderModel, m.senderEffort,
			m.recipients, m.kind, m.text, m.thought, m.parentID, m.createdAt, m.meta,
		); err != nil {
			t.Fatalf("造 v6 库时写入消息 %s 失败: %v", m.id, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO cursors (conv_id, agent_id, seq, pending, updated_at)
VALUES (?, ?, ?, ?, ?)`,
		"c-v6-1", "alpha", 2, "[2]", "2026-09-20T04:00:00Z",
	); err != nil {
		t.Fatalf("造 v6 库时写入游标失败: %v", err)
	}

	// turn_usage 的 21 列按 v6 的实际形状逐个写，不用 SELECT * 依赖顺序。
	if _, err := db.ExecContext(ctx, `
INSERT INTO turn_usage (
    turn_id, conv_id, agent_id, provider_session_id, seq, outcome,
    input_tokens, cache_create, cache_read, output_tokens, thinking_tokens,
    cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes,
    delivered_lo, delivered_hi, created_at, identity_fp, round_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"turn-v6-1", "c-v6-1", "alpha", "ps-v6-1", 2, 1,
		100, 0, 40, 20, 5,
		0.125, 0.5, 0, 1234, 600,
		1, 2, "2026-09-20T05:00:00Z", "fp-v6", "round-v6",
	); err != nil {
		t.Fatalf("造 v6 库时写入 turn_usage 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_sessions (
    conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd,
    have_cost_total, last_turn_at, identity_fp, spoken, baseline_suspect
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"c-v6-1", "alpha", "ps-v6-1", 2, 0.5,
		1, "2026-09-20T05:01:00Z", "fp-v6", 1, 0,
	); err != nil {
		t.Fatalf("造 v6 库时写入 provider_sessions 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO inflight_turns (
    turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id,
    cursor_seq, scanned_to, started_at, phase, tier, work_dir
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"inflight-v6-1", "c-v6-2", "alpha", "claude", "planned-v6", "", 1, 1,
		"2026-09-20T06:00:00Z", "spawned", "default", `D:\work`,
	); err != nil {
		t.Fatalf("造 v6 库时写入 inflight_turns 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?), (2, ?), (3, ?), (4, ?), (5, ?), (6, ?)`,
		tstFixedCreatedAt+"#1", tstFixedCreatedAt+"#2", tstFixedCreatedAt+"#3",
		tstFixedCreatedAt+"#4", tstFixedCreatedAt+"#5", tstFixedCreatedAt+"#6",
	); err != nil {
		t.Fatalf("造 v6 库时写 schema_migrations 失败: %v", err)
	}
}

// tstV7Snapshot 是迁移前后要比对的七张业务表。conversations 只取 v6 就有的
// 五列：deleted_at 是新列，老数据那五列必须一字不变。其余表 SELECT *：
// v7 不碰它们，列数变了本身就该是一条失败。
func tstV7Snapshot(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	return map[string][]string{
		"conversations":     tstCwdDump(t, db, `SELECT id, title, created_at, team_id, cwd FROM conversations ORDER BY id`),
		"participants":      tstCwdDump(t, db, `SELECT * FROM participants ORDER BY conv_id, agent_id`),
		"messages":          tstCwdDump(t, db, `SELECT * FROM messages ORDER BY conv_id, seq`),
		"cursors":           tstCwdDump(t, db, `SELECT * FROM cursors ORDER BY conv_id, agent_id`),
		"turn_usage":        tstCwdDump(t, db, `SELECT * FROM turn_usage ORDER BY turn_id`),
		"provider_sessions": tstCwdDump(t, db, `SELECT * FROM provider_sessions ORDER BY conv_id, agent_id`),
		"inflight_turns":    tstCwdDump(t, db, `SELECT * FROM inflight_turns ORDER BY turn_id`),
		"schema_migrations": tstCwdDump(t, db, `SELECT version, applied_at FROM schema_migrations ORDER BY version`),
	}
}

// tstV7BusinessTables 是 v7 迁移前后要比对内容的七张业务表。
// schema_migrations 不在其中：它本来就要多一行 v7，由账本用例单独比对。
var tstV7BusinessTables = []string{
	"conversations", "participants", "messages", "cursors",
	"turn_usage", "provider_sessions", "inflight_turns",
}

// tstV7TableCounts 数每张业务表的行数。快照比的是内容，这里再单独比行数，
// 让「数据一条不少」在失败信息里能直接读出来。
func tstV7TableCounts(t *testing.T, db *sql.DB) map[string]int64 {
	t.Helper()
	out := make(map[string]int64, len(tstV7BusinessTables))
	for _, table := range tstV7BusinessTables {
		out[table] = tstScanInt(t, db, `SELECT COUNT(*) FROM `+table)
	}
	return out
}

// TestSchemaV7MigrationV6ToV7 造一个停在 v6 的库，用正式的 Open 升到 v7，
// 钉住：账本只追加一行、七张表的老数据一行不少、deleted_at 是最后一列且
// 老行全为 NULL、升级后的库能正常读写、软删之后读入口看不见它。
func TestSchemaV7MigrationV6ToV7(t *testing.T) {
	dir := t.TempDir()
	tstV7BuildV6Database(t, dir)

	beforeDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, dbFileName))+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("取迁移前快照时打开连接失败: %v", err)
	}
	before := tstV7Snapshot(t, beforeDB)
	beforeCounts := tstV7TableCounts(t, beforeDB)
	if err := beforeDB.Close(); err != nil {
		t.Fatalf("取完迁移前快照后关闭连接失败: %v", err)
	}

	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	raw := tstRawDB(t, s.Path())

	t.Run("schema_migrations 追加 v7 且旧行不被重写", func(t *testing.T) {
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
		afterCounts := tstV7TableCounts(t, raw)
		for table, want := range beforeCounts {
			if got := afterCounts[table]; got != want {
				t.Errorf("%s 行数 = %d，期望 %d（迁移只许加列）", table, got, want)
			}
		}
		after := tstV7Snapshot(t, raw)
		// schema_migrations 本来就要多一行 v7，它由上一组用例单独比对；
		// 这里只比七张业务表。
		for _, table := range tstV7BusinessTables {
			tstCwdEqualLines(t, table, after[table], before[table])
		}
	})

	t.Run("deleted_at 是 conversations 的最后一列且可空、无默认值", func(t *testing.T) {
		rows, err := raw.Query(`SELECT name, type, "notnull", dflt_value FROM pragma_table_info('conversations') ORDER BY cid`)
		if err != nil {
			t.Fatalf("读 pragma_table_info('conversations') 失败: %v", err)
		}
		defer rows.Close()
		var names []string
		var sawDeleted bool
		var deletedType string
		var deletedNotNull int64
		var deletedDefault any
		for rows.Next() {
			var name, typ string
			var notnull int64
			var dflt any
			if err := rows.Scan(&name, &typ, &notnull, &dflt); err != nil {
				t.Fatalf("扫描 pragma_table_info 失败: %v", err)
			}
			names = append(names, name)
			if name == "deleted_at" {
				sawDeleted = true
				deletedType = typ
				deletedNotNull = notnull
				deletedDefault = dflt
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("遍历 pragma_table_info 失败: %v", err)
		}
		wantNames := []string{"id", "title", "created_at", "team_id", "cwd", "deleted_at"}
		if !reflect.DeepEqual(names, wantNames) {
			t.Errorf("conversations 列序 = %v，期望 %v（ALTER 把 deleted_at 加在最后一列）", names, wantNames)
		}
		if !sawDeleted {
			t.Fatal("pragma_table_info 里没有 deleted_at 列")
		}
		if deletedType != "TEXT" {
			t.Errorf("deleted_at type = %q，期望 TEXT", deletedType)
		}
		if deletedNotNull != 0 {
			t.Errorf("deleted_at notnull = %d，期望 0（NULL 表示没删）", deletedNotNull)
		}
		if deletedDefault != nil {
			t.Errorf("deleted_at dflt_value = %#v，期望 NULL（不要给没删的旧行编一个默认时间）", deletedDefault)
		}
	})

	t.Run("老会话的 deleted_at 全是 NULL", func(t *testing.T) {
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations WHERE deleted_at IS NOT NULL`); got != 0 {
			t.Errorf("迁移后有 %d 行 deleted_at 非 NULL，期望 0", got)
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

	t.Run("升级后的库能读老数据", func(t *testing.T) {
		list, err := s.Conversations(ctx)
		if err != nil {
			t.Fatalf("Conversations 失败: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("Conversations 返回 %d 场，期望 3 场", len(list))
		}
		one, err := s.Conversation(ctx, "c-v6-2")
		if err != nil {
			t.Fatalf("Conversation(c-v6-2) 失败: %v", err)
		}
		if one.Title != "旧会话 二 (含括号)" || one.TeamID != "team-x" || one.Cwd != "" {
			t.Errorf("c-v6-2 = %+v，期望标题/team 原样、cwd 为空串", one)
		}
		withCwd, err := s.Conversation(ctx, "c-v6-1")
		if err != nil {
			t.Fatalf("Conversation(c-v6-1) 失败: %v", err)
		}
		if withCwd.Cwd != `D:\proj\one` {
			t.Errorf("c-v6-1 的 Cwd = %q，期望 %q", withCwd.Cwd, `D:\proj\one`)
		}
		msgs, err := s.Messages(ctx, "c-v6-1", 0, 0)
		if err != nil {
			t.Fatalf("Messages(c-v6-1) 失败: %v", err)
		}
		if len(msgs) != 2 {
			t.Errorf("c-v6-1 的消息数 = %d，期望 2", len(msgs))
		}
		if seq, err := s.MaxSeq(ctx, "c-v6-1"); err != nil || seq != 2 {
			t.Errorf("MaxSeq(c-v6-1) = %d, err=%v，期望 2", seq, err)
		}
		parts, err := s.Participants(ctx, "c-v6-1", true)
		if err != nil {
			t.Fatalf("Participants(c-v6-1) 失败: %v", err)
		}
		if len(parts) != 2 {
			t.Errorf("c-v6-1 的成员数 = %d，期望 2", len(parts))
		}
		usages, err := s.TurnUsages(ctx, "c-v6-1", "", -1)
		if err != nil {
			t.Fatalf("TurnUsages(c-v6-1) 失败: %v", err)
		}
		if len(usages) != 1 || usages[0].TurnID != "turn-v6-1" {
			t.Errorf("c-v6-1 的用量 = %+v，期望只有 turn-v6-1", usages)
		}
		if ps, found, err := s.ProviderSession(ctx, "c-v6-1", "alpha"); err != nil || !found || ps.ProviderSessionID != "ps-v6-1" {
			t.Errorf("ProviderSession(c-v6-1, alpha) = %+v, found=%v, err=%v，期望读回 ps-v6-1", ps, found, err)
		}
		if it, found, err := s.InflightFor(ctx, "c-v6-2", "alpha"); err != nil || !found || it.TurnID != "inflight-v6-1" {
			t.Errorf("InflightFor(c-v6-2, alpha) = %+v, found=%v, err=%v，期望读回 inflight-v6-1", it, found, err)
		}
	})

	t.Run("升级后的库能写、软删之后读入口看不见", func(t *testing.T) {
		appended := tstAppend(t, ctx, s, tstUserMsg("c-v6-1", "升级之后追加的一条"))
		if appended.Seq != 3 {
			t.Errorf("追加消息的 Seq = %d，期望 3", appended.Seq)
		}
		if err := s.RenameConversation(ctx, "c-v6-3", "升级之后改的名字"); err != nil {
			t.Fatalf("RenameConversation(c-v6-3) 失败: %v", err)
		}
		if err := s.SoftDeleteConversation(ctx, "c-v6-3"); err != nil {
			t.Fatalf("SoftDeleteConversation(c-v6-3) 失败: %v", err)
		}
		if again := s.SoftDeleteConversation(ctx, "c-v6-3"); again != nil {
			t.Errorf("重复软删返回 %v，期望 nil（幂等）", again)
		}
		list, err := s.Conversations(ctx)
		if err != nil {
			t.Fatalf("软删后 Conversations 失败: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("软删后 Conversations 有 %d 场，期望 2 场", len(list))
		}
		if _, err := s.Conversation(ctx, "c-v6-3"); !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("软删后 Conversation(c-v6-3) 的 err = %v，期望 ErrConversationNotFound", err)
		}
		if msgs, err := s.Messages(ctx, "c-v6-3", 0, 0); err != nil || len(msgs) != 0 {
			t.Errorf("软删后 Messages(c-v6-3) = %v, err=%v，期望空切片", msgs, err)
		}
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations`); got != 3 {
			t.Errorf("软删后 conversations 物理行数 = %d，期望仍是 3（不物理删）", got)
		}
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations WHERE id = 'c-v6-3' AND deleted_at IS NOT NULL`); got != 1 {
			t.Errorf("c-v6-3 的 deleted_at 标记数 = %d，期望 1", got)
		}
	})

	closeFn()
	s2, _ := tstOpenAt(t, dir)
	ctx2 := tstCtx(t)
	raw2 := tstRawDB(t, s2.Path())

	t.Run("重开后迁移不重复执行、软删仍然生效", func(t *testing.T) {
		if got := tstScanInt(t, raw2, `SELECT COUNT(*) FROM schema_migrations`); got != int64(len(migrations)) {
			t.Errorf("重开后 schema_migrations 行数 = %d，期望 %d（ALTER 重复执行会因 duplicate column 失败，Open 本身就不会成功）", got, len(migrations))
		}
		list, err := s2.Conversations(ctx2)
		if err != nil {
			t.Fatalf("重开后 Conversations 失败: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("重开后 Conversations 有 %d 场，期望 2 场（已删的 c-v6-3 仍不可见）", len(list))
		}
		if _, err := s2.Conversation(ctx2, "c-v6-3"); !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("重开后 Conversation(c-v6-3) 的 err = %v，期望 ErrConversationNotFound", err)
		}
		one, err := s2.Conversation(ctx2, "c-v6-1")
		if err != nil {
			t.Fatalf("重开后 Conversation(c-v6-1) 失败: %v", err)
		}
		if one.Cwd != `D:\proj\one` {
			t.Errorf("重开后 c-v6-1 的 Cwd = %q，期望 %q", one.Cwd, `D:\proj\one`)
		}
		msgs, err := s2.Messages(ctx2, "c-v6-1", 0, 0)
		if err != nil {
			t.Fatalf("重开后 Messages(c-v6-1) 失败: %v", err)
		}
		if len(msgs) != 3 {
			t.Errorf("重开后 c-v6-1 的消息数 = %d，期望 3（原有两条加升级后追加的一条）", len(msgs))
		}
	})
}

// TestSchemaV7V6BinaryCannotOpenV7Database 模拟旧二进制：migrations 只留到
// v6，打开 v7 库应当以 ErrSchemaMismatch 失败并点名多出 deleted_at。
// 这是预期行为，不是缺陷：旧二进制不认识软删，让它继续跑会绕过所有
// deleted_at 过滤，把用户已经删掉的会话重新显示出来。
func TestSchemaV7V6BinaryCannotOpenV7Database(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "v7 库里的会话")
	closeFn()

	orig := migrations
	migrations = migrations[:6]
	t.Cleanup(func() { migrations = orig })

	opened, err := Open(tstCtx(t), dir)
	if opened != nil {
		if cerr := opened.Close(); cerr != nil {
			t.Errorf("失败的 Open 仍返回了 store，Close 出错: %v", cerr)
		}
	}
	if err == nil {
		t.Fatal("旧二进制打开 v7 库应当失败，实际成功")
	}
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("errors.Is(ErrSchemaMismatch) = false，实际错误: %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"多出列 deleted_at", "不要删库"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误文本不含 %q；实际: %s", want, msg)
		}
	}

	// 还原成当前二进制再开一次：库没有被旧二进制改坏，会话仍可读。
	migrations = orig
	reopened, _ := tstOpenAt(t, dir)
	got, err := reopened.Conversation(tstCtx(t), conv.ID)
	if err != nil {
		t.Fatalf("重开后读回 %s 失败: %v", conv.ID, err)
	}
	if got.Title != "v7 库里的会话" {
		t.Errorf("重开后 Title = %q，期望「v7 库里的会话」", got.Title)
	}
}

// TestSchemaV7ShapeIsV6PlusDeletedAt 钉住 v7 相对 v6 只多出 conversations
// 末尾那一列，表集合与其它表的列都不变；同时钉住 v7Shape 是深拷贝，
// 不是 v6Shape 的裸别名。
func TestSchemaV7ShapeIsV6PlusDeletedAt(t *testing.T) {
	if len(v7Shape) != len(v6Shape) {
		t.Fatalf("v7Shape 有 %d 张表，v6Shape 有 %d 张，期望表集合相同", len(v7Shape), len(v6Shape))
	}
	for table := range v6Shape {
		if _, ok := v7Shape[table]; !ok {
			t.Errorf("v6Shape 有表 %q，v7Shape 里没有", table)
		}
	}
	for table := range v7Shape {
		if _, ok := v6Shape[table]; !ok {
			t.Errorf("v7Shape 有表 %q，v6Shape 里没有", table)
		}
	}

	wantConv := append(cloneColumnShapes(v6Shape["conversations"]), columnShape{
		name: "deleted_at", typ: "TEXT", notnull: false,
	})
	assertColumnShapesEqual(t, `v7Shape["conversations"]`, v7Shape["conversations"], wantConv)

	var others []string
	for table := range v6Shape {
		if table == "conversations" {
			continue
		}
		others = append(others, table)
	}
	sort.Strings(others)
	for _, table := range others {
		assertColumnShapesEqual(t, fmt.Sprintf("v7Shape[%q]", table), v7Shape[table], v6Shape[table])
	}

	// 改 conversations 的键不影响 v6Shape：append 一定换新数组，但整体
	// 重新赋值 v7Shape["conversations"] 在裸别名时会把 v6Shape 一起改掉。
	t.Run("改 conversations 键不影响 v6Shape", func(t *testing.T) {
		wantV6 := cloneColumnShapes(v6Shape["conversations"])
		orig := v7Shape["conversations"]
		t.Cleanup(func() { v7Shape["conversations"] = orig })
		v7Shape["conversations"] = append(cloneColumnShapes(orig),
			columnShape{name: "tst_v7_alias_probe_key", typ: "TEXT", notnull: false})
		assertColumnShapesEqual(t, `v6Shape["conversations"]`, v6Shape["conversations"], wantV6)
	})

	// 挑一张 v7 没加列的表中按下标改元素，抓「只 make 了新 map、切片仍共享
	// 底层数组」这种坏法。conversations 因为 append 过，按下标改测不出来。
	t.Run("改未加列表的元素不影响 v6Shape", func(t *testing.T) {
		var candidates []string
		for table, cols := range v7Shape {
			if table == "conversations" {
				continue
			}
			if len(cols) > 0 && len(cols) == len(v6Shape[table]) {
				candidates = append(candidates, table)
			}
		}
		if len(candidates) == 0 {
			t.Fatal("v7Shape 里找不到一张列数与 v6Shape 相同的非空表, 本用例依赖的前提不成立")
		}
		sort.Strings(candidates)
		probe := candidates[0]
		wantV6 := cloneColumnShapes(v6Shape[probe])
		origElem := v7Shape[probe][0]
		t.Cleanup(func() { v7Shape[probe][0] = origElem })
		v7Shape[probe][0] = columnShape{
			name: "tst_v7_alias_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}
		assertColumnShapesEqual(t, fmt.Sprintf(`v6Shape[%q]`, probe), v6Shape[probe], wantV6)
	})
}
