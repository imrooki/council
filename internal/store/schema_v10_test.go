// schema_v10_test.go 覆盖第四次「只加表」的在线升级：停留在 v9 的库打开后
// 建出 local_leases 与 local_lease_events 两张表。
//
// 与 v7 / v8 / v9 三次不同的地方要说清楚：那三次都给既有表加过列，所以
// **旧二进制打开新库会失败**（形状校验报「多出列」）。v10 一个既有表都没碰，
// 于是 v9 二进制打开 v10 库**不会失败**，那两张表对它不可见 —— 这不是漏洞，
// 是「只加表」换来的可回滚性（删键 + DROP 两表即回到出厂状态，Plan §2.1
// S-10）。这里用一个用例把这件事钉成事实，免得以后有人把它当成「形状校验
// 失效」去修。
package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// tstV10BuildV9Database 在 dir 下造一个只跑到 v9 的 council.db。
// 不引用 v10DDL：造库路径必须停在升级之前，否则测到的是空操作。
func tstV10BuildV9Database(t *testing.T, dir string) {
	t.Helper()
	if len(migrations) < 10 || migrations[8].version != 9 || migrations[9].version != 10 {
		t.Fatalf("migrations 的形状变了: len=%d，本辅助只该造 v9 库（下标 8 的 version 必须是 9，下标 9 必须是 10）", len(migrations))
	}

	dbPath := filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v9 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v9 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for i, m := range migrations[:9] {
		for _, stmt := range m.stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatalf("造 v9 库时执行 migrations[%d] version %d 失败: %v\n语句: %s", i, m.version, err, stmt)
			}
		}
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO conversations (id, title, created_at) VALUES (?, ?, ?)`,
		"c-v9-1", "v9 的会话", "2026-10-20T00:00:01Z",
	); err != nil {
		t.Fatalf("造 v9 库时写入会话失败: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO workspaces (id, name, sort_order, created_at) VALUES (?, ?, ?, ?)`,
		"ws-v9-1", "v9 的工作区", 1, "2026-10-20T00:00:02Z",
	); err != nil {
		t.Fatalf("造 v9 库时写入工作区失败: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE conversations SET workspace_id = 'ws-v9-1' WHERE id = 'c-v9-1'`,
	); err != nil {
		t.Fatalf("造 v9 库时设置工作区归属失败: %v", err)
	}

	for version := 1; version <= 9; version++ {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			version, tstFixedCreatedAt,
		); err != nil {
			t.Fatalf("造 v9 库时写 schema_migrations v%d 失败: %v", version, err)
		}
	}
}

// tstV10LeaseTables 是 v10 新增的两张表。
var tstV10LeaseTables = []string{"local_leases", "local_lease_events"}

// TestSchemaV10MigrationV9ToV10 是主用例：造 v9 库 → Open → 断言只多了
// 两张空表、既有数据一字未改、账本只追加一行。
func TestSchemaV10MigrationV9ToV10(t *testing.T) {
	dir := t.TempDir()
	tstV10BuildV9Database(t, dir)

	before, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, dbFileName))+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("取迁移前快照时打开连接失败: %v", err)
	}
	wantConv := tstCwdDump(t, before, `SELECT * FROM conversations ORDER BY id`)
	wantWorkspace := tstCwdDump(t, before, `SELECT * FROM workspaces ORDER BY id`)
	if err := before.Close(); err != nil {
		t.Fatalf("取完迁移前快照后关闭连接失败: %v", err)
	}

	s, closeFn := tstOpenAt(t, dir)
	defer closeFn()
	ctx := tstCtx(t)
	raw := tstRawDB(t, s.Path())

	t.Run("库版本升到 v10 且账本只追加一行", func(t *testing.T) {
		if got, want := s.SchemaVersion(), migrations[len(migrations)-1].version; got != want {
			t.Errorf("SchemaVersion = %d，期望 %d", got, want)
		}
		if got := tstScanInt(t, raw, `SELECT MAX(version) FROM schema_migrations`); got != 10 {
			t.Errorf("MAX(version) = %d，期望 10", got)
		}
		if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM schema_migrations`); got != int64(len(migrations)) {
			t.Errorf("schema_migrations 行数 = %d，期望 %d（每版一行）", got, len(migrations))
		}
	})

	t.Run("两张租约表建出来且是空的", func(t *testing.T) {
		for _, table := range tstV10LeaseTables {
			if got := tstScanInt(t, raw,
				`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table); got != 1 {
				t.Fatalf("%s 表数 = %d，期望 1", table, got)
			}
			if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM `+table); got != 0 {
				t.Errorf("%s 行数 = %d，期望 0（迁移不该凭空造租约）", table, got)
			}
		}
	})

	t.Run("既有表一字未改", func(t *testing.T) {
		tstCwdEqualLines(t, "conversations",
			tstCwdDump(t, raw, `SELECT * FROM conversations ORDER BY id`), wantConv)
		tstCwdEqualLines(t, "workspaces",
			tstCwdDump(t, raw, `SELECT * FROM workspaces ORDER BY id`), wantWorkspace)
	})

	t.Run("列序与 notnull 与 shape 一致", func(t *testing.T) {
		for table, wantCols := range map[string][]string{
			"local_leases": {"lease_id", "resource_key", "owner_kind", "owner_id", "access_mode",
				"generation", "state", "granted_at", "expires_at", "heartbeat_at"},
			"local_lease_events": {"id", "lease_id", "at", "kind", "detail"},
		} {
			rows, err := raw.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
			if err != nil {
				t.Fatalf("读 pragma_table_info(%s) 失败: %v", table, err)
			}
			var got []string
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					_ = rows.Close()
					t.Fatalf("扫描 pragma_table_info(%s) 失败: %v", table, err)
				}
				got = append(got, name)
			}
			if err := rows.Close(); err != nil {
				t.Fatalf("关闭 pragma_table_info(%s) 失败: %v", table, err)
			}
			if !reflect.DeepEqual(got, wantCols) {
				t.Errorf("%s 列序 = %v，期望 %v", table, got, wantCols)
			}
		}
	})

	t.Run("三个索引都建出来了", func(t *testing.T) {
		for _, name := range []string{
			"idx_local_leases_live", "idx_local_leases_write", "idx_local_lease_events_lease",
		} {
			if got := tstScanInt(t, raw,
				`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name); got != 1 {
				t.Errorf("索引 %s 的数量 = %d，期望 1", name, got)
			}
		}
	})

	t.Run("validateSchema 通过", func(t *testing.T) {
		if err := s.validateSchema(ctx); err != nil {
			t.Errorf("validateSchema 返回错误，期望 nil: %v", err)
		}
	})

	t.Run("升级后的库能直接申请租约", func(t *testing.T) {
		lease, err := s.ClaimLease(ctx, LeaseRequest{
			ResourceKey: `d:\work\proj`,
			OwnerKind:   LeaseOwnerCLI,
			OwnerID:     "manual",
			Access:      LeaseWrite,
			TTL:         time.Minute,
		})
		if err != nil {
			t.Fatalf("升级后 ClaimLease 失败: %v", err)
		}
		if lease.Generation != 1 || lease.State != LeaseGranted {
			t.Errorf("首条租约 = generation %d / state %q，期望 1 / granted", lease.Generation, lease.State)
		}
		events, err := s.LeaseEvents(ctx, lease.ID)
		if err != nil {
			t.Fatalf("LeaseEvents 失败: %v", err)
		}
		if len(events) != 1 || events[0].Kind != "granted" {
			t.Errorf("租约事件 = %+v，期望恰好一条 granted", events)
		}
	})
}

// TestSchemaV10PartialIndexBlocksSecondWriteLease 直接往库里插两行 granted
// 写租约，证明「同一资源不能有两份写授权」不只是 Go 里的判断，SQLite 的
// 部分唯一索引自己也挡得住。
func TestSchemaV10PartialIndexBlocksSecondWriteLease(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	raw := tstRawDB(t, s.Path())

	insert := func(id, access string) error {
		_, err := raw.Exec(
			`INSERT INTO local_leases
			 (lease_id, resource_key, owner_kind, owner_id, access_mode, generation, state, granted_at, expires_at, heartbeat_at)
			 VALUES (?, 'res-a', 'cli', 'x', ?, 1, 'granted', ?, ?, ?)`,
			id, access, "2026-10-27T00:00:00Z", "2026-10-27T01:00:00Z", "2026-10-27T00:00:00Z")
		return err
	}

	if err := insert("lease-1", "write"); err != nil {
		t.Fatalf("第一条写租约插入失败: %v", err)
	}
	if err := insert("lease-2", "write"); err == nil {
		t.Fatal("第二条写租约竟然插入成功：部分唯一索引没生效")
	}
	// 读租约不受写租约的唯一索引限制（读共享由 store 层的冲突判定管，
	// 不在索引里）。这里只证明索引没有误伤它。
	if err := insert("lease-3", "read"); err != nil {
		t.Errorf("写租约存在时插入一条读租约失败: %v（索引只该约束 write）", err)
	}

	// 交还之后同一个键可以再拿一次写租约。
	if _, err := raw.Exec(`UPDATE local_leases SET state = 'released' WHERE lease_id = 'lease-1'`); err != nil {
		t.Fatalf("把第一条写租约改成 released 失败: %v", err)
	}
	if err := insert("lease-4", "write"); err != nil {
		t.Errorf("释放后再次申请写租约失败: %v", err)
	}
}

// TestSchemaV10V9BinaryStillOpensDatabase 钉住「只加表」换来的可回滚性：
// 把 migrations 截到 v9 当作旧二进制，它打开 v10 库**不报错**，租约行原样
// 留着；换回当前二进制后仍然读得到。
//
// 与 TestSchemaV9V8BinaryCannotOpenV9Database 的差别是刻意的：v9 给
// conversations 加了列，旧二进制必然撞形状校验；v10 一列都没加。
func TestSchemaV10V9BinaryStillOpensDatabase(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	lease, err := s.ClaimLease(ctx, LeaseRequest{
		ResourceKey: "tcp:127.0.0.1:8765",
		OwnerKind:   LeaseOwnerCLI,
		OwnerID:     "port-holder",
		Access:      LeaseWrite,
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("ClaimLease 失败: %v", err)
	}
	closeFn()

	orig := migrations
	migrations = orig[:9]
	t.Cleanup(func() { migrations = orig })

	old, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("v9 二进制打开 v10 库失败: %v（v10 没有给既有表加列，本该能打开）", err)
	}
	if got := old.SchemaVersion(); got != 9 {
		t.Errorf("v9 二进制看到的 SchemaVersion = %d，期望 9", got)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("关闭 v9 二进制的库失败: %v", err)
	}

	migrations = orig
	reopened, closeReopened := tstOpenAt(t, dir)
	defer closeReopened()
	grants, err := reopened.GrantedLeases(ctx)
	if err != nil {
		t.Fatalf("重开后 GrantedLeases 失败: %v", err)
	}
	if len(grants) != 1 || grants[0].ID != lease.ID {
		t.Errorf("重开后的有效租约 = %+v，期望仍只有 %s", grants, lease.ID)
	}
}

// TestSchemaV10ReadOnlyPaths 钉住 Plan §12 那条「只读打开 v9 库仍然可用」，
// 以及 §4.3 ④ 的口径：租约读接口只保证在 v10 库上能用。
//
// 只读看板**不迁库**，所以它可能对着一个 v9 库；那时 local_leases 表根本
// 不存在，GrantedLeases 会以一句点名的 SQL 错误失败。这不是缺陷：本模块的
// 租约接口只在 -rw 下存在（§4.3 ④）。将来真要把租约画进看板，必须先给
// readShape 加布尔位并在 open_readonly 里探测，否则旧库会 500 —— 这条用例
// 把「现在还没有那个探测」写成事实。
func TestSchemaV10ReadOnlyPaths(t *testing.T) {
	ctx := tstCtx(t)

	t.Run("v9 库只读打开可用，但租约读会点名缺表", func(t *testing.T) {
		dir := t.TempDir()
		tstV10BuildV9Database(t, dir)

		ro, err := OpenReadOnly(ctx, dir)
		if err != nil {
			t.Fatalf("OpenReadOnly(v9 库) 失败: %v", err)
		}
		defer func() {
			if err := ro.Close(); err != nil {
				t.Errorf("关闭只读库失败: %v", err)
			}
		}()
		if got := ro.SchemaVersion(); got != 9 {
			t.Errorf("只读打开 v9 库的 SchemaVersion = %d，期望 9", got)
		}
		if !ro.SchemaLag() {
			t.Error("只读打开 v9 库时 SchemaLag 应为 true")
		}
		if _, err := ro.GrantedLeases(ctx); err == nil {
			t.Error("v9 库没有 local_leases 表，GrantedLeases 应当报错而不是假装没有租约")
		} else if !strings.Contains(err.Error(), "local_leases") {
			t.Errorf("错误文案没有点名 local_leases: %v", err)
		}
		// 既有读路径不受影响：会话仍然读得出来。
		if _, err := ro.Conversations(ctx); err != nil {
			t.Errorf("只读打开 v9 库后 Conversations 失败: %v", err)
		}
	})

	t.Run("v10 库只读打开后能读租约", func(t *testing.T) {
		dir := tstDataDir(t)
		s, closeFn := tstOpenAt(t, dir)
		lease, err := s.ClaimLease(ctx, LeaseRequest{
			ResourceKey: `d:\work\readonly`,
			OwnerKind:   LeaseOwnerCLI,
			OwnerID:     "reader",
			Access:      LeaseRead,
			TTL:         time.Hour,
		})
		if err != nil {
			t.Fatalf("ClaimLease 失败: %v", err)
		}
		closeFn()

		ro, err := OpenReadOnly(ctx, dir)
		if err != nil {
			t.Fatalf("OpenReadOnly(v10 库) 失败: %v", err)
		}
		defer func() {
			if err := ro.Close(); err != nil {
				t.Errorf("关闭只读库失败: %v", err)
			}
		}()
		if ro.SchemaLag() {
			t.Error("v10 库只读打开时 SchemaLag 应为 false")
		}
		grants, err := ro.GrantedLeases(ctx)
		if err != nil {
			t.Fatalf("只读库上 GrantedLeases 失败: %v", err)
		}
		if len(grants) != 1 || grants[0].ID != lease.ID {
			t.Errorf("只读库上的有效租约 = %+v，期望只有 %s", grants, lease.ID)
		}
	})
}

// TestSchemaV10ShapeIsV9PlusTwoTables 钉住 v10Shape = v9Shape + 两张表，
// 其它表的列不变；并且 v10Shape 是深拷贝，不是 v9Shape 的裸别名。
func TestSchemaV10ShapeIsV9PlusTwoTables(t *testing.T) {
	if len(v10Shape) != len(v9Shape)+2 {
		t.Fatalf("v10Shape 有 %d 张表，v9Shape 有 %d 张，期望正好多两张", len(v10Shape), len(v9Shape))
	}
	for table := range v9Shape {
		if _, ok := v10Shape[table]; !ok {
			t.Errorf("v9Shape 有表 %q，v10Shape 里没有", table)
		}
		assertColumnShapesEqual(t, table, v10Shape[table], v9Shape[table])
	}
	for _, table := range tstV10LeaseTables {
		if _, ok := v9Shape[table]; ok {
			t.Errorf("v9Shape 里不该有 %q（它是 v10 才建的）", table)
		}
		if len(v10Shape[table]) == 0 {
			t.Errorf("v10Shape 没声明 %q 的列", table)
		}
	}

	assertColumnShapesEqual(t, `v10Shape["local_leases"]`, v10Shape["local_leases"], []columnShape{
		{name: "lease_id", typ: "TEXT", notnull: false},
		{name: "resource_key", typ: "TEXT", notnull: true},
		{name: "owner_kind", typ: "TEXT", notnull: true},
		{name: "owner_id", typ: "TEXT", notnull: true},
		{name: "access_mode", typ: "TEXT", notnull: true},
		{name: "generation", typ: "INTEGER", notnull: true},
		{name: "state", typ: "TEXT", notnull: true},
		{name: "granted_at", typ: "TEXT", notnull: true},
		{name: "expires_at", typ: "TEXT", notnull: true},
		{name: "heartbeat_at", typ: "TEXT", notnull: true},
	})
	assertColumnShapesEqual(t, `v10Shape["local_lease_events"]`, v10Shape["local_lease_events"], []columnShape{
		{name: "id", typ: "TEXT", notnull: false},
		{name: "lease_id", typ: "TEXT", notnull: true},
		{name: "at", typ: "TEXT", notnull: true},
		{name: "kind", typ: "TEXT", notnull: true},
		{name: "detail", typ: "TEXT", notnull: true},
	})

	// 按下标改一张既有表的元素：裸别名时 v9Shape 会一起变。
	t.Run("改既有表元素不影响 v9Shape", func(t *testing.T) {
		var probe string
		var candidates []string
		for table, cols := range v10Shape {
			if len(cols) == 0 || len(cols) != len(v9Shape[table]) {
				continue
			}
			candidates = append(candidates, table)
		}
		sort.Strings(candidates)
		if len(candidates) == 0 {
			t.Fatal("找不到一张列数与 v9Shape 相同的非空表，本用例依赖的前提不成立")
		}
		probe = candidates[0]
		wantV9 := cloneColumnShapes(v9Shape[probe])
		origElem := v10Shape[probe][0]
		t.Cleanup(func() { v10Shape[probe][0] = origElem })
		v10Shape[probe][0] = columnShape{
			name: "tst_v10_alias_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}
		assertColumnShapesEqual(t, "v9Shape["+probe+"]", v9Shape[probe], wantV9)
	})
}

// TestSchemaV10RollbackByDroppingTables 钉住 S-10 的回滚路径：
// 删掉 local_agent 键并 DROP 掉两张表之后，库回到 v9 也能用的状态
// （这里只验表这一半：形状校验对剩下的表仍然通过）。
func TestSchemaV10RollbackByDroppingTables(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)
	if _, err := s.ClaimLease(ctx, LeaseRequest{
		ResourceKey: "res-rollback",
		OwnerKind:   LeaseOwnerCLI,
		OwnerID:     "x",
		Access:      LeaseWrite,
		TTL:         time.Minute,
	}); err != nil {
		t.Fatalf("ClaimLease 失败: %v", err)
	}
	raw := tstRawDB(t, s.Path())
	for _, stmt := range []string{
		`DROP TABLE local_lease_events`,
		`DROP TABLE local_leases`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("回滚时执行 %q 失败: %v", stmt, err)
		}
	}
	// 表没了，形状校验当然会报缺表 —— 这正是「回滚必须连着换回旧二进制」
	// 的含义。这里断言错误点名的是那两张表，而不是别的什么。
	err := s.validateSchema(ctx)
	if err == nil {
		t.Fatal("DROP 之后 validateSchema 竟然通过")
	}
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("errors.Is(ErrSchemaMismatch) = false，实际: %v", err)
	}
	if !strings.Contains(err.Error(), "local_lease") {
		t.Errorf("错误文案没有点名 local_lease* 表: %v", err)
	}
}
