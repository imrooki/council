// 本文件将来落到 internal/store/schema_v3_test.go。
//
// 覆盖 11a_STEP8A_API_CONTRACT.md 第 2 节 schema v3、验收表第 2 条前半
// （"造 v2 库 + 一条四段 identity_fp，v3 打开后变六段、前四段不变"）。
// 只照契约写，不看 internal/store/schema.go 的实现——那是本片改动、任务书
// 明令不读的文件；v1DDL / v2DDL / migrations 这几个符号的存在与用法完全学自
// 既有的 schema_v2_test.go / schema_shape_test.go（*_test.go 文件任务书明确
// 允许读，只为学风格与辅助函数），不是读了 schema.go 才知道的。
//
// 验收表第 2 条后半（"规则为空时新算出的指纹与迁移后的值相等"）**不在本文件**：
// internal/turn 依赖 internal/store（Runner 经 conversation.Service 读写），
// 本包若反过来 import internal/turn 会成环，Go 编译器会直接拒绝。那半句断言
// 挪到 internal_turn_session_test.go / internal_turn_runner_rules_test.go 里，
// 用契约给出的字面量 "0:;0:;" 与 turn.IdentityFingerprint 的结果比较，
// 两处配合起来才是完整的验收表第 2 条。
package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// tstLegacyFourSegmentFP 是 schema v3 迁移之前、真实的四段指纹编码。
// 格式是"十进制长度 + 冒号 + 正文 + 分号"（与既有 session_test.go
// TestPackUnpackFingerprintRoundTrip 描述的编码一致），这里按该格式手工
// 拼出 "6:role-a;7:model-a;4:high;4:read;"，不依赖 internal/turn 的
// packFingerprint（避免引入包依赖环）。
const tstLegacyFourSegmentFP = "6:role-a;7:model-a;4:high;4:read;"

// tstLegacyFourSegmentFPAlt 是第二条种子行用的指纹，与上面那条内容不同，
// 用来证明 UPDATE 命中的是全表而不是巧合地只改对了一行。
const tstLegacyFourSegmentFPAlt = "8:role-beta;9:model-beta;3:low;9:workspace;"

// tstBuildV2DatabaseWithLegacySessions 在 dir 下造一个"只跑到 v2"的
// council.db：完整的 v1DDL 加 v2DDL、schema_migrations 只有两行
// （version=1、version=2），外加一个会话与两条 provider_sessions，
// 每条都带着迁移前真实存在过的四段 identity_fp。
//
// 两条种子行、两个不同的指纹内容，是为了让"迁移命中全表"这条断言有意义——
// 只造一行的话，UPDATE 语句哪怕漏了 WHERE 条件、只巧合改中这一行，
// 用例也测不出来。
func tstBuildV2DatabaseWithLegacySessions(t *testing.T, dir string) (dbPath, convID string) {
	t.Helper()

	dbPath = filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v2 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v2 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for _, stmt := range v1DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v2 库时执行 v1DDL 失败: %v\n语句: %s", err, stmt)
		}
	}
	for _, stmt := range v2DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v2 库时执行 v2DDL 失败: %v\n语句: %s", err, stmt)
		}
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?), (2, ?)`,
		tstFixedCreatedAt, tstFixedCreatedAt,
	); err != nil {
		t.Fatalf("造 v2 库时写 schema_migrations 失败: %v", err)
	}

	convID = "conv-from-real-v2-user"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO conversations (id, title, created_at) VALUES (?, ?, ?)`,
		convID, "升级之前就存在的会话", tstFixedCreatedAt,
	); err != nil {
		t.Fatalf("造 v2 库时写入种子会话失败: %v", err)
	}

	seedRows := []struct {
		agentID string
		fp      string
	}{
		{agentID: "alpha", fp: tstLegacyFourSegmentFP},
		{agentID: "beta", fp: tstLegacyFourSegmentFPAlt},
	}
	for _, row := range seedRows {
		if _, err := db.ExecContext(ctx, `
INSERT INTO provider_sessions
    (conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd, have_cost_total, last_turn_at, identity_fp, spoken)
VALUES (?, ?, ?, 0, NULL, 0, ?, ?, 0)`,
			convID, row.agentID, "sess-"+row.agentID, tstFixedCreatedAt, row.fp,
		); err != nil {
			t.Fatalf("造 v2 库时写入 provider_sessions(%s) 失败: %v", row.agentID, err)
		}
	}
	return dbPath, convID
}

// TestSchemaV3MigratesLegacyFourSegmentFingerprints 是验收表第 2 条的前半：
// 造一个真实存在过的 v2 库（记着 version=1、2，且已经有两条带四段指纹的
// provider_sessions），用正式的 Open 打开，每一条 identity_fp 都必须变成
// 六段，且前四段逐字节不变——契约第 2 节原话"追加的是空段"，v3DDL 的字面量
// 是 `identity_fp || '0:;0:;'`，这里直接断言字符串拼接的结果，不猜测
// 实现是怎么拼出这两个字符的。
func TestSchemaV3MigratesLegacyFourSegmentFingerprints(t *testing.T) {
	dir := t.TempDir()
	_, convID := tstBuildV2DatabaseWithLegacySessions(t, dir)

	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("老库（v2）打不开了: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("关闭失败: %v", err)
		}
	})

	t.Run("每一版迁移各落一行，v1、v2 那两行没有被重跑", func(t *testing.T) {
		rows := tstMigrationRows(t, s.Path())
		if len(rows) != len(migrations) {
			t.Fatalf("schema_migrations 有 %d 行，期望 %d 行（每版一行）: %+v",
				len(rows), len(migrations), rows)
		}
		for i, m := range migrations {
			if rows[i].version != m.version {
				t.Errorf("第 %d 行 version = %d，期望 %d", i+1, rows[i].version, m.version)
			}
		}
		if rows[0].appliedAt != tstFixedCreatedAt || rows[1].appliedAt != tstFixedCreatedAt {
			t.Errorf("v1/v2 的 applied_at 应当仍是造库时写的 %q，实际 v1=%q v2=%q（说明被重跑了）",
				tstFixedCreatedAt, rows[0].appliedAt, rows[1].appliedAt)
		}
	})

	raw := tstRawDB(t, s.Path())

	t.Run("alpha 的四段指纹迁移之后变成六段，前四段逐字节不变", func(t *testing.T) {
		got := tstScanText(t, raw,
			`SELECT identity_fp FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`, convID, "alpha")
		want := tstLegacyFourSegmentFP + "0:;0:;"
		if got != want {
			t.Errorf("identity_fp = %q，期望 %q", got, want)
		}
	})

	t.Run("beta 的四段指纹同样被迁移，证明 UPDATE 命中的是全表不是单行", func(t *testing.T) {
		got := tstScanText(t, raw,
			`SELECT identity_fp FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`, convID, "beta")
		want := tstLegacyFourSegmentFPAlt + "0:;0:;"
		if got != want {
			t.Errorf("identity_fp = %q，期望 %q", got, want)
		}
	})

	t.Run("升级之前就有的会话，升级之后原样读得回来", func(t *testing.T) {
		got, err := s.Conversation(context.Background(), convID)
		if err != nil {
			t.Fatalf("读升级前就存在的会话失败: %v", err)
		}
		if got.Title != "升级之前就存在的会话" {
			t.Errorf("Title = %q，期望 %q", got.Title, "升级之前就存在的会话")
		}
	})
}

// TestSchemaV3ProviderSessionsShapeUnchanged 覆盖契约第 2 节
// "v3Shape 与 v2 完全相同：这一版一列都没动"：迁移之后 provider_sessions
// 的前九列（v2 时代这张表的全部列）必须逐列（name/type/notnull）、逐位置
// 与迁移前一致，只改数据不改列。
//
// 原来这里断言的是"总共恰好九列"：这在 v3 落地时等价于"前九列不变"，
// 因为那时 provider_sessions 也确实只有九列。但这条用例真正想钉的是
// "v3 这一步没有重建 provider_sessions"，与这张表眼下一共有几列无关——
// v5 在表尾追加了 baseline_suspect（见 schema.go 的 v5DDL），列数变成十，
// 于是"恰好九列"这条与 v3 无关的断言就跟着报红，变成每有一版迁移往这张
// 表追加新列都要回来改一次数字的版本锁死写法。现在改成"前九列不变、
// 允许后面追加新列"，这才是该断言的准确表达：DROP+CREATE 式的重建会
// 打乱或改写这前九列的内容/顺序，而单纯在表尾 ALTER ... ADD COLUMN
// 不会——挡的正是"重建"这件事，不是"列数"这件事。
func TestSchemaV3ProviderSessionsShapeUnchanged(t *testing.T) {
	dir := t.TempDir()
	tstBuildV2DatabaseWithLegacySessions(t, dir)

	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("关闭失败: %v", err)
		}
	})

	raw := tstRawDB(t, s.Path())
	rows, err := raw.Query(`SELECT name, type, "notnull" FROM pragma_table_info('provider_sessions') ORDER BY cid`)
	if err != nil {
		t.Fatalf("读 provider_sessions 的列信息失败: %v", err)
	}
	defer rows.Close()

	// want 是 v2 时代 provider_sessions 的全部九列，逐列 name/type/notnull，
	// 按 cid 顺序手写字面量——不引用 schema.go 里的 v2Shape，保持这条用例
	// 是独立于实现的 ground truth（与本文件其它用例一致的体例）。
	//
	// 判别力自检：假设有人把 v3 实现成"重建 provider_sessions"而不是单纯
	// UPDATE，最常见的两种坏法——
	//   (a) 重建时把列顺序打乱（比如 identity_fp 挪到了 spoken 前面）：
	//       下面按下标逐列比对 name，第 7 列会拿到 "spoken" 却期望
	//       "identity_fp"，直接报错，挡住。
	//   (b) 重建时某列的约束变了（比如 identity_fp 建成了可空）：
	//       下面同一位置还会比 typ 与 notnull，notnull 从 true 变 false
	//       会被这里的第二个 if 挡住。
	// 这两种坏法都不需要总列数变化就能体现"表被重建过"，而前九列逐列比对
	// 两种都挡得住，所以判别力不弱于原来的"总列数 == 9"。
	want := []columnShape{
		{name: "conv_id", typ: "TEXT", notnull: true},
		{name: "agent_id", typ: "TEXT", notnull: true},
		{name: "provider_session_id", typ: "TEXT", notnull: true},
		{name: "cursor_seq", typ: "INTEGER", notnull: true},
		{name: "cost_total_usd", typ: "REAL", notnull: false},
		{name: "have_cost_total", typ: "INTEGER", notnull: true},
		{name: "last_turn_at", typ: "TEXT", notnull: true},
		{name: "identity_fp", typ: "TEXT", notnull: true},
		{name: "spoken", typ: "INTEGER", notnull: true},
	}
	var got []columnShape
	for rows.Next() {
		var name, typ string
		var notnull int
		if err := rows.Scan(&name, &typ, &notnull); err != nil {
			t.Fatalf("扫描列信息失败: %v", err)
		}
		got = append(got, columnShape{name: name, typ: typ, notnull: notnull != 0})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历列信息失败: %v", err)
	}
	if len(got) < len(want) {
		t.Fatalf("provider_sessions 列数 = %d，至少要有前 %d 列（v2 时代的全部列）；实际列: %+v",
			len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.name != w.name {
			t.Errorf("第 %d 列名 = %q，期望 %q（v3 不改列，顺序也不该变）", i, g.name, w.name)
			continue
		}
		if g.typ != w.typ || g.notnull != w.notnull {
			t.Errorf("列 %s 的定义 = (type=%s, notnull=%v)，期望 (type=%s, notnull=%v)（v3 不该改列定义）",
				g.name, g.typ, g.notnull, w.typ, w.notnull)
		}
	}
}
