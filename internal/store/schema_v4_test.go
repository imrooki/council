// 本文件覆盖 docs/12_STEP9_CONTRACT.md 3.9 第 2 条（片 2a 的一部分）：
// "v3 旧库跑迁移：Open 成功、不报 ErrSchemaMismatch、identity_fp 没被碰过"。
//
// 造法照 schema_v3_test.go 的体例：先跑到 v3（v1DDL + v2DDL + v3DDL 全部
// 执行过、schema_migrations 记着 1、2、3 三行），塞一条种子 provider_sessions，
// 再用当前二进制 Open——current 二进制此刻应当把它从 v3 迁到 v4。
//
// 任务书要求：本片的符号（v4Shape、v4DDL、片 2a 的 migrations 第 4 条）此刻
// 一个都不存在，本文件只用到已经落地的 v1DDL / v2DDL / v3DDL /
// tstLegacyFourSegmentFP（后者是 schema_v3_test.go 声明的包级常量，
// Go 允许同包多文件共享），不读、不猜 schema.go 里 v4 相关的实现。
package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// tstBuildV3DatabaseWithProviderSession 在 dir 下造一个"只跑到 v3"的
// council.db：v1DDL + v2DDL 建表、写入一条带着迁移前四段 identity_fp 的
// provider_sessions，再跑 v3DDL 把它变成六段（与 schema_v3_test.go 的
// tstBuildV2DatabaseWithLegacySessions 造 v2 库的思路一致，只是多跑一版），
// 最后把 schema_migrations 补成 1、2、3 三行。
//
// 复用 v3DDL 本身去把种子行从四段变成六段，而不是手写一个六段字面量：
// 期望值因此始终等于"真的跑过 v3DDL"这件事本身产出的值，不会因为我手打错
// 一个字符就让用例锁住一个错误的期望。
func tstBuildV3DatabaseWithProviderSession(t *testing.T, dir string) (dbPath, convID string) {
	t.Helper()

	dbPath = filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v3 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v3 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for _, stmt := range v1DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v3 库时执行 v1DDL 失败: %v\n语句: %s", err, stmt)
		}
	}
	for _, stmt := range v2DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v3 库时执行 v2DDL 失败: %v\n语句: %s", err, stmt)
		}
	}

	convID = "conv-from-real-v3-user"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO conversations (id, title, created_at) VALUES (?, ?, ?)`,
		convID, "停在 v3 的会话", tstFixedCreatedAt,
	); err != nil {
		t.Fatalf("造 v3 库时写入种子会话失败: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_sessions
    (conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd, have_cost_total, last_turn_at, identity_fp, spoken)
VALUES (?, ?, ?, 0, NULL, 0, ?, ?, 0)`,
		convID, "alpha", "sess-alpha-v3", tstFixedCreatedAt, tstLegacyFourSegmentFP,
	); err != nil {
		t.Fatalf("造 v3 库时写入 provider_sessions 失败: %v", err)
	}

	// 这一步把刚插入的四段指纹变成六段——用真实的 v3DDL 语句跑一遍，
	// 而不是手工拼字符串，保证种子状态与"一个真实存在过的 v3 库"完全一致。
	for _, stmt := range v3DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v3 库时执行 v3DDL 失败: %v\n语句: %s", err, stmt)
		}
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?), (2, ?), (3, ?)`,
		tstFixedCreatedAt, tstFixedCreatedAt, tstFixedCreatedAt,
	); err != nil {
		t.Fatalf("造 v3 库时写 schema_migrations 失败: %v", err)
	}
	return dbPath, convID
}

// TestSchemaV4MigratesV3DatabaseWithoutRebuildingProviderSessions 是
// 12 契约 3.9 第 2 条的完整验收：
//
//  1. Open 成功、不报 ErrSchemaMismatch；
//  2. schema_migrations 的 MAX(version) 是 migrations 里最后一项登记的
//     version（此刻是 4——Open 会一路迁到最新版，不是恰好停在 4；
//     断言不写死这个数字，理由见下面 t.Run 内的注释）；
//  3. 迁移之后这一条既有 provider 会话一个字节都没被碰过——这是正面断言，
//     不是"没报错就算过"。v4DDL 按 12 契约 3.3 节只是三条 ALTER TABLE ...
//     ADD COLUMN，没有任何一条碰 provider_sessions.identity_fp 这一列的
//     数据；若实现方哪一步不小心又跑了一次类似 v3DDL 那种 UPDATE，这条
//     会先红。不这样测的代价见 v3DDL 那段注释：每条活着的会话都会悄悄
//     重建一次、各付一次入场费（实测 0.4 到 1.0 美元），没有任何一层会说话。
func TestSchemaV4MigratesV3DatabaseWithoutRebuildingProviderSessions(t *testing.T) {
	dir := t.TempDir()
	_, convID := tstBuildV3DatabaseWithProviderSession(t, dir)

	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("停在 v3 的旧库打不开了（期望迁移到 v4 成功）: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("关闭失败: %v", err)
		}
	})

	t.Run("schema_migrations 的 MAX(version) 是 migrations 里最后一版", func(t *testing.T) {
		// 原来这里写的是字面量 4：这条用例想钉的是"Open 会把库迁到最新版"，
		// 但字面量 4 把"当前一共登记了几版 schema"焊进了断言里。v5 落地后
		// migrations 变成五项，Open 正确地迁到 5，这条本该继续绿的用例却
		// 因为字面量对不上而报红——它测的从来不是"停在 4"，而是"迁到底"。
		// 每加一版（v6、v7……）都要有人回来把数字改掉，是典型的版本锁死。
		//
		// 改成与 migrations[len(migrations)-1].version 比较：version 本身
		// 已经在 migrations 里声明过一次，这里不再抄一份会随时间漂移的
		// 副本，加多少版都不用改这条断言，且依旧能挡住"迁移中途停下"
		// 这类真实的坏实现（那样 MAX(version) 会小于最后一版）。
		raw := tstRawDB(t, s.Path())
		want := migrations[len(migrations)-1].version
		got := tstScanInt(t, raw, `SELECT MAX(version) FROM schema_migrations`)
		if got != want {
			t.Errorf("MAX(version) = %d，期望 %d（migrations 里最后一版的 version）", got, want)
		}
	})

	t.Run("provider_sessions.identity_fp 逐字节没被 v4 碰过", func(t *testing.T) {
		raw := tstRawDB(t, s.Path())
		got := tstScanText(t, raw,
			`SELECT identity_fp FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`, convID, "alpha")
		want := tstLegacyFourSegmentFP + "0:;0:;" // v3DDL 追加的两个空段，与 schema_v3_test.go 的期望值同一个算法
		if got != want {
			t.Errorf("identity_fp = %q，期望 %q（v4 迁移只应当加列，不该碰这一列的数据，12 契约 3.3 节）", got, want)
		}
	})

	t.Run("升级之前就有的会话，升级之后原样读得回来", func(t *testing.T) {
		got, err := s.Conversation(context.Background(), convID)
		if err != nil {
			t.Fatalf("读升级前就存在的会话失败: %v", err)
		}
		if got.Title != "停在 v3 的会话" {
			t.Errorf("Title = %q，期望 %q", got.Title, "停在 v3 的会话")
		}
	})
}
