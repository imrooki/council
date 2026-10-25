// schema_v8_test.go 覆盖第二次在线加列：停留在 v7 的库打开后补上
// conversations.archived_at。断言的重点与 v7 相同——除了这一列，七张业务表
// 的老数据一条不少、一个字不改；老会话的 archived_at 全是 NULL，且
// 已经软删的会话仍然不可见。
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

// tstV8BuildV7Database 在 dir 下造一个只跑到 v7 的 council.db：
// migrations[0] 到 migrations[6] 的语句全部执行，再种一份跨全部七张业务表的
// 数据（每张表至少一行），schema_migrations 补齐 1 到 7 七行。
// 不引用 v8DDL：造库路径必须停在升级之前，否则测到的是空操作。
func tstV8BuildV7Database(t *testing.T, dir string) {
	t.Helper()
	if len(migrations) < 8 || migrations[6].version != 7 || migrations[7].version != 8 {
		t.Fatalf("migrations 的形状变了: len=%d，本辅助只该造 v7 库（下标 6 的 version 必须是 7，下标 7 必须是 8）", len(migrations))
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

	convs := []struct {
		id, title, createdAt, teamID, cwd string
		deletedAt                         any
	}{
		{"c-v7-1", "旧会话一", "2026-10-01T00:00:01Z", "", `D:\proj\one`, nil},
		{"c-v7-2", "旧会话 二 (含括号)", "2026-10-01T00:00:02Z", "team-x", "", nil},
		{"c-v7-3", "v7 里已经删掉的会话", "2026-10-01T00:00:03Z", "", "", "2026-10-02T00:00:00Z"},
	}
	for _, c := range convs {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO conversations (id, title, created_at, team_id, cwd, deleted_at) VALUES (?, ?, ?, ?, ?, ?)`,
			c.id, c.title, c.createdAt, c.teamID, c.cwd, c.deletedAt,
		); err != nil {
			t.Fatalf("造 v7 库时写入会话 %s 失败: %v", c.id, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO participants (conv_id, agent_id, display_name, provider, active, joined_at, left_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"c-v7-1", "alpha", "Alpha", "claude", 1, "2026-10-01T01:00:00Z", nil,
	); err != nil {
		t.Fatalf("造 v7 库时写入成员失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO messages
    (id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort, recipients, kind, text, thought, parent_id, created_at, meta)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"m-v7-1", "c-v7-1", 1, "alpha", "claude", "opus", "high", `["alpha"]`, "agent",
		"v7 的正文", "", nil, "2026-10-01T03:00:01Z", `{"k":"v"}`,
	); err != nil {
		t.Fatalf("造 v7 库时写入消息失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO cursors (conv_id, agent_id, seq, pending, updated_at)
VALUES (?, ?, ?, ?, ?)`,
		"c-v7-1", "alpha", 1, "[1]", "2026-10-01T04:00:00Z",
	); err != nil {
		t.Fatalf("造 v7 库时写入游标失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO turn_usage (
    turn_id, conv_id, agent_id, provider_session_id, seq, outcome,
    input_tokens, cache_create, cache_read, output_tokens, thinking_tokens,
    cost_usd, cost_cumulative_usd, cold_start, wall_ms, prompt_bytes,
    delivered_lo, delivered_hi, created_at, identity_fp, round_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"turn-v7-1", "c-v7-1", "alpha", "ps-v7-1", 1, 1,
		200, 0, 50, 30, 5,
		0.25, 0.75, 0, 1500, 700,
		nil, 1, "2026-10-01T05:00:00Z", "fp-v7", "round-v7",
	); err != nil {
		t.Fatalf("造 v7 库时写入 turn_usage 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_sessions (
    conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd,
    have_cost_total, last_turn_at, identity_fp, spoken, baseline_suspect
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"c-v7-1", "alpha", "ps-v7-1", 1, 0.75,
		1, "2026-10-01T05:01:00Z", "fp-v7", 1, 0,
	); err != nil {
		t.Fatalf("造 v7 库时写入 provider_sessions 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO inflight_turns (
    turn_id, conv_id, agent_id, provider, planned_session_id, resume_session_id,
    cursor_seq, scanned_to, started_at, phase, tier, work_dir
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"inflight-v7-1", "c-v7-2", "alpha", "claude", "planned-v7", "", 0, 0,
		"2026-10-01T06:00:00Z", "begun", "default", `D:\work`,
	); err != nil {
		t.Fatalf("造 v7 库时写入 inflight_turns 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?), (2, ?), (3, ?), (4, ?), (5, ?), (6, ?), (7, ?)`,
		tstFixedCreatedAt+"#1", tstFixedCreatedAt+"#2", tstFixedCreatedAt+"#3",
		tstFixedCreatedAt+"#4", tstFixedCreatedAt+"#5", tstFixedCreatedAt+"#6",
		tstFixedCreatedAt+"#7",
	); err != nil {
		t.Fatalf("造 v7 库时写 schema_migrations 失败: %v", err)
	}
}

// tstV8Snapshot 是迁移前后要比对的七张业务表。conversations 只取 v7 就有的
// 六列：archived_at 是新列，老数据那六列（含 deleted_at）必须一字不变。
// 其余表 SELECT *：v8 不碰它们，列数变了本身就该是一条失败。
func tstV8Snapshot(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	return map[string][]string{
		"conversations":     tstCwdDump(t, db, `SELECT id, title, created_at, team_id, cwd, deleted_at FROM conversations ORDER BY id`),
		"participants":      tstCwdDump(t, db, `SELECT * FROM participants ORDER BY conv_id, agent_id`),
		"messages":          tstCwdDump(t, db, `SELECT * FROM messages ORDER BY conv_id, seq`),
		"cursors":           tstCwdDump(t, db, `SELECT * FROM cursors ORDER BY conv_id, agent_id`),
		"turn_usage":        tstCwdDump(t, db, `SELECT * FROM turn_usage ORDER BY turn_id`),
		"provider_sessions": tstCwdDump(t, db, `SELECT * FROM provider_sessions ORDER BY conv_id, agent_id`),
		"inflight_turns":    tstCwdDump(t, db, `SELECT * FROM inflight_turns ORDER BY turn_id`),
		"schema_migrations": tstCwdDump(t, db, `SELECT version, applied_at FROM schema_migrations ORDER BY version`),
	}
}

// tstV8BusinessTables 是 v8 迁移前后要比对内容的七张业务表。
var tstV8BusinessTables = []string{
	"conversations", "participants", "messages", "cursors",
	"turn_usage", "provider_sessions", "inflight_turns",
}

// tstV8TableCounts 数每张业务表的行数。
func tstV8TableCounts(t *testing.T, db *sql.DB) map[string]int64 {
	t.Helper()
	out := make(map[string]int64, len(tstV8BusinessTables))
	for _, table := range tstV8BusinessTables {
		out[table] = tstScanInt(t, db, `SELECT COUNT(*) FROM `+table)
	}
	return out
}

// TestSchemaV8MigrationV7ToV8 造一个停在 v7 的库，用正式的 Open 升到 v8，
// 钉住：账本只追加一行、七张表的老数据一行不少、archived_at 是最后一列且
// 老行全为 NULL、v7 里已经删掉的会话仍然不可见、升级后的库能正常归档与读写。
func TestSchemaV8MigrationV7ToV8(t *testing.T) {
	dir := t.TempDir()
	tstV8BuildV7Database(t, dir)

	beforeDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, dbFileName))+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("取迁移前快照时打开连接失败: %v", err)
	}
	before := tstV8Snapshot(t, beforeDB)
	beforeCounts := tstV8TableCounts(t, beforeDB)
	if err := beforeDB.Close(); err != nil {
		t.Fatalf("取完迁移前快照后关闭连接失败: %v", err)
	}

	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	raw := tstRawDB(t, s.Path())

	t.Run("schema_migrations 追加 v8 且旧行不被重写", func(t *testing.T) {
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
		afterCounts := tstV8TableCounts(t, raw)
		for table, want := range beforeCounts {
			if got := afterCounts[table]; got != want {
				t.Errorf("%s 行数 = %d，期望 %d（迁移只许加列）", table, got, want)
			}
		}
		after := tstV8Snapshot(t, raw)
		for _, table := range tstV8BusinessTables {
			tstCwdEqualLines(t, table, after[table], before[table])
		}
		if got := tstScanText(t, raw,
			`SELECT deleted_at FROM conversations WHERE id = 'c-v7-3'`); got != "2026-10-02T00:00:00Z" {
			t.Errorf("v7 里已删会话的 deleted_at = %q，期望原样保留", got)
		}
	})

	t.Run("archived_at 是 conversations 的最后一列且可空、无默认值", func(t *testing.T) {
		rows, err := raw.Query(`SELECT name, type, "notnull", dflt_value FROM pragma_table_info('conversations') ORDER BY cid`)
		if err != nil {
			t.Fatalf("读 pragma_table_info('conversations') 失败: %v", err)
		}
		defer rows.Close()
		var names []string
		var sawArchived bool
		var archivedType string
		var archivedNotNull int64
		var archivedDefault any
		for rows.Next() {
			var name, typ string
			var notnull int64
			var dflt any
			if err := rows.Scan(&name, &typ, &notnull, &dflt); err != nil {
				t.Fatalf("扫描 pragma_table_info 失败: %v", err)
			}
			names = append(names, name)
			if name == "archived_at" {
				sawArchived = true
				archivedType = typ
				archivedNotNull = notnull
				archivedDefault = dflt
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("遍历 pragma_table_info 失败: %v", err)
		}
		wantNames := []string{"id", "title", "created_at", "team_id", "cwd", "deleted_at", "archived_at"}
		if !reflect.DeepEqual(names, wantNames) {
			t.Errorf("conversations 列序 = %v，期望 %v（ALTER 把 archived_at 加在最后一列）", names, wantNames)
		}
		if !sawArchived {
			t.Fatal("pragma_table_info 里没有 archived_at 列")
		}
		if archivedType != "TEXT" {
			t.Errorf("archived_at type = %q，期望 TEXT", archivedType)
		}
		if archivedNotNull != 0 {
			t.Errorf("archived_at notnull = %d，期望 0（NULL 表示没归档）", archivedNotNull)
		}
		if archivedDefault != nil {
			t.Errorf("archived_at dflt_value = %#v，期望 NULL（不要给没归档的旧行编一个默认时间）", archivedDefault)
		}
	})

	t.Run("老会话的 archived_at 全是 NULL", func(t *testing.T) {
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations WHERE archived_at IS NOT NULL`); got != 0 {
			t.Errorf("迁移后有 %d 行 archived_at 非 NULL，期望 0", got)
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
		if len(list) != 2 {
			t.Fatalf("Conversations 返回 %d 场，期望 2 场（c-v7-3 已删）", len(list))
		}
		for _, conv := range list {
			if conv.Archived {
				t.Errorf("%s 的 Archived = true，v7 老行应当全是 NULL", conv.ID)
			}
		}
		one, err := s.Conversation(ctx, "c-v7-2")
		if err != nil {
			t.Fatalf("Conversation(c-v7-2) 失败: %v", err)
		}
		if one.Title != "旧会话 二 (含括号)" || one.TeamID != "team-x" {
			t.Errorf("c-v7-2 = %+v，期望标题/team 原样", one)
		}
		if _, err := s.Conversation(ctx, "c-v7-3"); !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("升级后 Conversation(c-v7-3) = %v，期望 ErrConversationNotFound", err)
		}
		if msgs, err := s.Messages(ctx, "c-v7-1", 0, 0); err != nil || len(msgs) != 1 {
			t.Errorf("升级后 Messages(c-v7-1) = %+v, err=%v，期望一条", msgs, err)
		}
		if usages, err := s.TurnUsages(ctx, "c-v7-1", "", -1); err != nil || len(usages) != 1 {
			t.Errorf("升级后 TurnUsages(c-v7-1) = %+v, err=%v，期望一条", usages, err)
		}
		if ps, found, err := s.ProviderSession(ctx, "c-v7-1", "alpha"); err != nil || !found || ps.ProviderSessionID != "ps-v7-1" {
			t.Errorf("升级后 ProviderSession(c-v7-1, alpha) = %+v, found=%v, err=%v，期望读回 ps-v7-1", ps, found, err)
		}
	})

	t.Run("升级后的库能归档、归档后读路径仍然通", func(t *testing.T) {
		if err := s.ArchiveConversation(ctx, "c-v7-1"); err != nil {
			t.Fatalf("升级后 ArchiveConversation(c-v7-1) 失败: %v", err)
		}
		active, err := s.Conversations(ctx)
		if err != nil {
			t.Fatalf("归档后 Conversations 失败: %v", err)
		}
		if len(active) != 1 || active[0].ID != "c-v7-2" {
			t.Errorf("归档后默认列表 = %+v，期望只剩 c-v7-2", active)
		}
		archived, err := s.ArchivedConversations(ctx)
		if err != nil {
			t.Fatalf("归档后 ArchivedConversations 失败: %v", err)
		}
		if len(archived) != 1 || archived[0].ID != "c-v7-1" || !archived[0].Archived {
			t.Errorf("归档后归档列表 = %+v，期望只有 c-v7-1 且标记已归档", archived)
		}
		if got := tstScanInt(t, raw,
			`SELECT COUNT(*) FROM conversations WHERE id = 'c-v7-1' AND archived_at IS NOT NULL`); got != 1 {
			t.Errorf("c-v7-1 的 archived_at 标记数 = %d，期望 1", got)
		}
		if msgs, err := s.Messages(ctx, "c-v7-1", 0, 0); err != nil || len(msgs) != 1 {
			t.Errorf("归档后 Messages(c-v7-1) = %+v, err=%v，期望一条（归档不阻止打开）", msgs, err)
		}
	})

	closeFn()
	s2, _ := tstOpenAt(t, dir)
	ctx2 := tstCtx(t)
	raw2 := tstRawDB(t, s2.Path())

	t.Run("重开后迁移不重复执行、归档状态仍在", func(t *testing.T) {
		if got := tstScanInt(t, raw2, `SELECT COUNT(*) FROM schema_migrations`); got != int64(len(migrations)) {
			t.Errorf("重开后 schema_migrations 行数 = %d，期望 %d（ALTER 重复执行会因 duplicate column 失败，Open 本身就不会成功）", got, len(migrations))
		}
		active, err := s2.Conversations(ctx2)
		if err != nil {
			t.Fatalf("重开后 Conversations 失败: %v", err)
		}
		if len(active) != 1 || active[0].ID != "c-v7-2" {
			t.Errorf("重开后默认列表 = %+v，期望只剩 c-v7-2", active)
		}
		archived, err := s2.ArchivedConversations(ctx2)
		if err != nil {
			t.Fatalf("重开后 ArchivedConversations 失败: %v", err)
		}
		if len(archived) != 1 || archived[0].ID != "c-v7-1" {
			t.Errorf("重开后归档列表 = %+v，期望仍然只有 c-v7-1", archived)
		}
	})
}

// TestSchemaV8V7BinaryCannotOpenV8Database 模拟旧二进制：migrations 只留到
// v7，打开 v8 库应当以 ErrSchemaMismatch 失败并点名多出 archived_at。
// 这是预期行为：v7 二进制不认识归档，让它继续跑会把已归档的会话当成
// 普通会话全部列出来，前端也没有取消归档的入口。
func TestSchemaV8V7BinaryCannotOpenV8Database(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "v8 库里的会话")
	if err := s.ArchiveConversation(ctx, conv.ID); err != nil {
		t.Fatalf("ArchiveConversation 失败: %v", err)
	}
	closeFn()

	orig := migrations
	migrations = migrations[:7]
	t.Cleanup(func() { migrations = orig })

	opened, err := Open(tstCtx(t), dir)
	if opened != nil {
		if cerr := opened.Close(); cerr != nil {
			t.Errorf("失败的 Open 仍返回了 store，Close 出错: %v", cerr)
		}
	}
	if err == nil {
		t.Fatal("v7 二进制打开 v8 库应当失败，实际成功")
	}
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("errors.Is(ErrSchemaMismatch) = false，实际错误: %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"多出列 archived_at", "不要删库"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误文本不含 %q；实际: %s", want, msg)
		}
	}

	// 还原成当前二进制再开一次：库没有被旧二进制改坏，归档状态仍在。
	migrations = orig
	reopened, _ := tstOpenAt(t, dir)
	archived, err := reopened.ArchivedConversations(tstCtx(t))
	if err != nil {
		t.Fatalf("重开后 ArchivedConversations 失败: %v", err)
	}
	if len(archived) != 1 || archived[0].ID != conv.ID {
		t.Errorf("重开后归档列表 = %+v，期望仍是 %s", archived, conv.ID)
	}
}

// TestSchemaV8ShapeIsV7PlusArchivedAt 钉住 v8 相对 v7 只多出 conversations
// 末尾那一列，表集合与其它表的列都不变；同时钉住 v8Shape 是深拷贝，
// 不是 v7Shape 的裸别名。
func TestSchemaV8ShapeIsV7PlusArchivedAt(t *testing.T) {
	if len(v8Shape) != len(v7Shape) {
		t.Fatalf("v8Shape 有 %d 张表，v7Shape 有 %d 张，期望表集合相同", len(v8Shape), len(v7Shape))
	}
	for table := range v7Shape {
		if _, ok := v8Shape[table]; !ok {
			t.Errorf("v7Shape 有表 %q，v8Shape 里没有", table)
		}
	}
	for table := range v8Shape {
		if _, ok := v7Shape[table]; !ok {
			t.Errorf("v8Shape 有表 %q，v7Shape 里没有", table)
		}
	}

	wantConv := append(cloneColumnShapes(v7Shape["conversations"]), columnShape{
		name: "archived_at", typ: "TEXT", notnull: false,
	})
	assertColumnShapesEqual(t, `v8Shape["conversations"]`, v8Shape["conversations"], wantConv)

	var others []string
	for table := range v7Shape {
		if table == "conversations" {
			continue
		}
		others = append(others, table)
	}
	sort.Strings(others)
	for _, table := range others {
		assertColumnShapesEqual(t, fmt.Sprintf("v8Shape[%q]", table), v8Shape[table], v7Shape[table])
	}

	// 改 conversations 的键不影响 v7Shape：append 一定换新数组，但整体
	// 重新赋值 v8Shape["conversations"] 在裸别名时会把 v7Shape 一起改掉。
	t.Run("改 conversations 键不影响 v7Shape", func(t *testing.T) {
		wantV7 := cloneColumnShapes(v7Shape["conversations"])
		orig := v8Shape["conversations"]
		t.Cleanup(func() { v8Shape["conversations"] = orig })
		v8Shape["conversations"] = append(cloneColumnShapes(orig),
			columnShape{name: "tst_v8_alias_probe_key", typ: "TEXT", notnull: false})
		assertColumnShapesEqual(t, `v7Shape["conversations"]`, v7Shape["conversations"], wantV7)
	})

	// 挑一张 v8 没加列的表中按下标改元素，抓「只 make 了新 map、切片仍共享
	// 底层数组」这种坏法。conversations 因为 append 过，按下标改测不出来。
	t.Run("改未加列表的元素不影响 v7Shape", func(t *testing.T) {
		var candidates []string
		for table, cols := range v8Shape {
			if table == "conversations" {
				continue
			}
			if len(cols) > 0 && len(cols) == len(v7Shape[table]) {
				candidates = append(candidates, table)
			}
		}
		if len(candidates) == 0 {
			t.Fatal("v8Shape 里找不到一张列数与 v7Shape 相同的非空表, 本用例依赖的前提不成立")
		}
		sort.Strings(candidates)
		probe := candidates[0]
		wantV7 := cloneColumnShapes(v7Shape[probe])
		origElem := v8Shape[probe][0]
		t.Cleanup(func() { v8Shape[probe][0] = origElem })
		v8Shape[probe][0] = columnShape{
			name: "tst_v8_alias_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}
		assertColumnShapesEqual(t, fmt.Sprintf(`v7Shape[%q]`, probe), v7Shape[probe], wantV7)
	})
}
