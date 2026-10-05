// 本文件覆盖 schema v5 迁移(片段：inflight_turns 新表、
// provider_sessions.baseline_suspect 新列、UNIQUE(conv_id, agent_id))的
// 验收用例。冻结契约见任务书：
//
//	migrations 末尾追加 {version: 5, stmts: v5DDL, shape: v5Shape}；
//	v5DDL 两条语句——CREATE TABLE inflight_turns(十二列，turn_id 没写
//	NOT NULL) 与 ALTER TABLE provider_sessions ADD COLUMN
//	baseline_suspect INTEGER NOT NULL DEFAULT 0；外加一条
//	UNIQUE(conv_id, agent_id)（表内约束或独立索引，二选一）；
//	v5Shape 必须是 v4Shape 的深拷贝再加列，不能是裸别名。
//
// 造法照 schema_v4_test.go / schema_v4_shape_test.go 的体例：造一个旧版本
// 的库、用当前二进制打开、查 pragma_table_info、钉别名。本文件只用到已经
// 落地的 v1DDL / v2DDL / v3DDL / v4DDL / v2Shape / v3Shape / v4Shape /
// tstFixedCreatedAt / tstLegacyFourSegmentFP（后两个是别的 _test.go 文件
// 声明的包级符号，Go 允许同包多文件共享）以及 schema_v4_shape_test.go 里的
// cloneColumnShapes / assertColumnShapesEqual，不读、不猜 schema.go 里
// v5 相关的实现——那是背对背的另一条腿正在写的东西。
//
// v5DDL / v5Shape / buildV5Shape 此刻都不存在，本文件引用它们会编译不过，
// 这是预期状态，不是本文件的 bug。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// tstV5ProviderSessionSeed 是"迁移之前必须原样保留"的那九个值的唯一来源：
// 造库时拿它去 INSERT，断言时拿同一份字面量去比对，不在两处分别手打一遍，
// 也就不会因为抄错一位数字而让期望值和造库值悄悄改成同一个错误答案。
type tstV5ProviderSessionSeed struct {
	convID            string
	agentID           string
	providerSessionID string
	cursorSeq         int64
	costTotalUSD      float64
	haveCostTotal     int64
	lastTurnAt        string
	identityFP        string
	spoken            int64
}

// tstV5ProviderSessionSeedFixture 造一份九列都可辨认、互不相同的取值。
// identityFP 复用 tstLegacyFourSegmentFP + "0:;0:;"（schema_v3_test.go /
// schema_v4_test.go 同款字面量）：这是 v3DDL 真的跑过一次之后产出的六段
// 指纹，不是我手工拼的新格式，与"这是一个真实存在过的 v4 库"这件事一致。
func tstV5ProviderSessionSeedFixture(convID string) tstV5ProviderSessionSeed {
	return tstV5ProviderSessionSeed{
		convID:            convID,
		agentID:           "alpha",
		providerSessionID: "sess-alpha-v4-real",
		cursorSeq:         4001,
		costTotalUSD:      12.75,
		haveCostTotal:     1,
		lastTurnAt:        "2026-09-21T00:00:00Z",
		identityFP:        tstLegacyFourSegmentFP + "0:;0:;",
		spoken:            1,
	}
}

// tstBuildV4DatabaseWithProviderSession 在 dir 下造一个"只跑到 v4"的
// council.db：v1DDL 到 v4DDL 依序全部执行过(与真实升级路径一致，v3DDL 的
// UPDATE 在空表上跑是安全的空操作)，塞一条 provider_sessions 种子行，
// schema_migrations 补齐 1、2、3、4 四行。造法与 schema_v4_test.go 的
// tstBuildV3DatabaseWithProviderSession 是同一个思路，只是多跑一版 DDL、
// 且种子行在 v3DDL/v4DDL 全部跑完之后才插入——这样它的 identity_fp
// 不会再被 v3DDL 的 UPDATE 碰到，插入的九个值就是断言要核对的最终值，
// 不需要再借用 v3DDL 去二次加工。
func tstBuildV4DatabaseWithProviderSession(t *testing.T, dir string) (dbPath string, seed tstV5ProviderSessionSeed) {
	t.Helper()

	dbPath = filepath.Join(dir, dbFileName)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("造 v4 库时打开连接失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("造 v4 库后关闭连接失败: %v", err)
		}
	}()

	ctx := context.Background()
	for _, stmt := range v1DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v4 库时执行 v1DDL 失败: %v\n语句: %s", err, stmt)
		}
	}
	for _, stmt := range v2DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v4 库时执行 v2DDL 失败: %v\n语句: %s", err, stmt)
		}
	}
	for _, stmt := range v3DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v4 库时执行 v3DDL 失败: %v\n语句: %s", err, stmt)
		}
	}
	for _, stmt := range v4DDL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("造 v4 库时执行 v4DDL 失败: %v\n语句: %s", err, stmt)
		}
	}

	convID := "conv-from-real-v4-user"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO conversations (id, title, created_at) VALUES (?, ?, ?)`,
		convID, "停在 v4 的会话", tstFixedCreatedAt,
	); err != nil {
		t.Fatalf("造 v4 库时写入种子会话失败: %v", err)
	}

	seed = tstV5ProviderSessionSeedFixture(convID)
	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_sessions
    (conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd, have_cost_total, last_turn_at, identity_fp, spoken)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		seed.convID, seed.agentID, seed.providerSessionID, seed.cursorSeq,
		seed.costTotalUSD, seed.haveCostTotal, seed.lastTurnAt, seed.identityFP, seed.spoken,
	); err != nil {
		t.Fatalf("造 v4 库时写入 provider_sessions 失败: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?), (2, ?), (3, ?), (4, ?)`,
		tstFixedCreatedAt, tstFixedCreatedAt, tstFixedCreatedAt, tstFixedCreatedAt,
	); err != nil {
		t.Fatalf("造 v4 库时写 schema_migrations 失败: %v", err)
	}
	return dbPath, seed
}

// TestSchemaV5MigratesV4DatabaseProviderSessionsUntouched 是本组迁移验收的
// 主体：造一个真实存在过的 v4 库、用正式的 Open 打开，四条子用例分别挡：
//
//  1. provider_sessions 原有九列一个字节都不该被 v5 碰——v5DDL 只 ALTER
//     ADD COLUMN，没有任何一条语句碰这张表已有的九列数据；
//  2. baseline_suspect 新列在旧行上读出 0——DDL 是 NOT NULL DEFAULT 0，
//     旧行不该被塞成别的值；
//  3. inflight_turns 建出来了、且是空表——v5 只建表，不该塞数据；
//  4. schema_migrations 的 MAX(version) 等于 migrations 里最后一版。
func TestSchemaV5MigratesV4DatabaseProviderSessionsUntouched(t *testing.T) {
	dir := t.TempDir()
	_, seed := tstBuildV4DatabaseWithProviderSession(t, dir)

	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("停在 v4 的旧库打不开了(期望迁移到 v5 成功): %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("关闭失败: %v", err)
		}
	})

	raw := tstRawDB(t, s.Path())

	t.Run("provider_sessions 原有九列逐字节不变", func(t *testing.T) {
		var gotConvID, gotAgentID, gotProviderSessionID, gotLastTurnAt, gotIdentityFP string
		var gotCursorSeq, gotHaveCostTotal, gotSpoken int64
		var gotCostTotalUSD float64
		err := raw.QueryRow(`
SELECT conv_id, agent_id, provider_session_id, cursor_seq, cost_total_usd, have_cost_total, last_turn_at, identity_fp, spoken
FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`, seed.convID, seed.agentID).Scan(
			&gotConvID, &gotAgentID, &gotProviderSessionID, &gotCursorSeq, &gotCostTotalUSD,
			&gotHaveCostTotal, &gotLastTurnAt, &gotIdentityFP, &gotSpoken)
		if err != nil {
			t.Fatalf("读迁移后的 provider_sessions 失败: %v", err)
		}
		// 九列逐一比对，缺一列都可能漏掉 v5 不该碰却碰了的那一列。
		if gotConvID != seed.convID {
			t.Errorf("conv_id = %q, 期望 %q", gotConvID, seed.convID)
		}
		if gotAgentID != seed.agentID {
			t.Errorf("agent_id = %q, 期望 %q", gotAgentID, seed.agentID)
		}
		if gotProviderSessionID != seed.providerSessionID {
			t.Errorf("provider_session_id = %q, 期望 %q", gotProviderSessionID, seed.providerSessionID)
		}
		if gotCursorSeq != seed.cursorSeq {
			t.Errorf("cursor_seq = %d, 期望 %d", gotCursorSeq, seed.cursorSeq)
		}
		if gotCostTotalUSD != seed.costTotalUSD {
			t.Errorf("cost_total_usd = %v, 期望 %v", gotCostTotalUSD, seed.costTotalUSD)
		}
		if gotHaveCostTotal != seed.haveCostTotal {
			t.Errorf("have_cost_total = %d, 期望 %d", gotHaveCostTotal, seed.haveCostTotal)
		}
		if gotLastTurnAt != seed.lastTurnAt {
			t.Errorf("last_turn_at = %q, 期望 %q", gotLastTurnAt, seed.lastTurnAt)
		}
		if gotIdentityFP != seed.identityFP {
			t.Errorf("identity_fp = %q, 期望 %q", gotIdentityFP, seed.identityFP)
		}
		if gotSpoken != seed.spoken {
			t.Errorf("spoken = %d, 期望 %d", gotSpoken, seed.spoken)
		}
	})

	t.Run("baseline_suspect 新列在旧行上读出 0", func(t *testing.T) {
		got := tstScanInt(t, raw,
			`SELECT baseline_suspect FROM provider_sessions WHERE conv_id = ? AND agent_id = ?`,
			seed.convID, seed.agentID)
		if got != 0 {
			t.Errorf("baseline_suspect = %d, 期望 0(DDL 是 NOT NULL DEFAULT 0，旧行不该被填成别的值)", got)
		}
	})

	t.Run("inflight_turns 存在且为空", func(t *testing.T) {
		n := tstScanInt(t, raw,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'inflight_turns'`)
		if n != 1 {
			t.Fatalf("sqlite_master 里名为 inflight_turns 的表有 %d 张, 期望 1 张"+
				"(表都不存在的话，后面直接 COUNT(*) 拿到的是查询失败而不是有意义的 0)", n)
		}
		got := tstScanInt(t, raw, `SELECT COUNT(*) FROM inflight_turns`)
		if got != 0 {
			t.Errorf("inflight_turns 有 %d 行, 期望 0 行(v5 迁移只建表，不该塞数据)", got)
		}
	})

	t.Run("schema_migrations 的 MAX(version) 是 migrations 里最后一版", func(t *testing.T) {
		want := migrations[len(migrations)-1].version
		got := tstScanInt(t, raw, `SELECT MAX(version) FROM schema_migrations`)
		if got != want {
			t.Errorf("MAX(version) = %d，期望 %d（migrations 里最后一版的 version）", got, want)
		}
	})
}

// tstInflightTurnsDDLColumnOrder 与冻结契约里 inflight_turns 的 DDL 列序
// 逐一对应，是下面查 pragma_table_info 时核对列序与 notnull 的唯一依据。
//
// 只有 turn_id 是 false：DDL 写的是 "turn_id TEXT PRIMARY KEY"，没有额外的
// NOT NULL 关键字，SQLite 对这样声明的列，pragma_table_info 返回
// notnull = 0，哪怕它是主键。这不是本文件的新猜测——本包已经见过两次同样的
// 事实：v1Shape["conversations"] 的 "id"（v1DDL 写的是
// "id TEXT PRIMARY KEY"）与 v2Shape["turn_usage"] 的 "turn_id"（v2DDL 写的
// 是 "turn_id TEXT PRIMARY KEY"）都是这个写法，schema.go 里这两份 shape
// 都把它们声明成 notnull: false；而现在的实现代码(schema.go 里
// tableColumns/checkTableShape 那段)正是拿 pragma_table_info 的 notnull
// 列去跟 shape 比对的——如果 "TEXT PRIMARY KEY、不写 NOT NULL" 在 pragma
// 里读回 notnull=1，这两条既有断言早该常年报 ErrSchemaMismatch，不会是
// v5 才第一次遇到的新情况。
var tstInflightTurnsDDLColumnOrder = []struct {
	name    string
	notnull bool
}{
	{"turn_id", false},
	{"conv_id", true},
	{"agent_id", true},
	{"provider", true},
	{"planned_session_id", true},
	{"resume_session_id", true},
	{"cursor_seq", true},
	{"scanned_to", true},
	{"started_at", true},
	{"phase", true},
	{"tier", true},
	{"work_dir", true},
}

// TestSchemaV5InflightTurnsNotNullMatchesRealDatabase 挡的坏实现：
// v5Shape 把 turn_id 写成 notnull: true（那样 validateSchema 会在真库上报
// mismatch；而如果 shape 常量与 DDL 字面量一起写错，就可能自洽地都错，
// 校验通过反而会被误读成"这一列真的该是 NOT NULL"）。所以这里不查
// v5Shape 常量，直接查真库的 pragma_table_info，逐列核对列序与 notnull，
// 十二列一个不漏——12 列里只有 turn_id 该是 notnull = 0，其余十一列都是 1，
// 且列序必须与冻结契约给出的 DDL 顺序一致。
func TestSchemaV5InflightTurnsNotNullMatchesRealDatabase(t *testing.T) {
	s, _ := tstStore(t)
	raw := tstRawDB(t, s.Path())

	rows, err := raw.Query(`SELECT name, "notnull" FROM pragma_table_info('inflight_turns') ORDER BY cid`)
	if err != nil {
		t.Fatalf("读 inflight_turns 的列信息失败: %v", err)
	}
	defer rows.Close()

	type col struct {
		name    string
		notnull bool
	}
	var got []col
	for rows.Next() {
		var name string
		var notnull int
		if err := rows.Scan(&name, &notnull); err != nil {
			t.Fatalf("扫描 pragma_table_info 失败: %v", err)
		}
		got = append(got, col{name: name, notnull: notnull != 0})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 pragma_table_info 失败: %v", err)
	}

	if len(got) != len(tstInflightTurnsDDLColumnOrder) {
		t.Fatalf("inflight_turns 有 %d 列, 期望 %d 列; 实际: %+v",
			len(got), len(tstInflightTurnsDDLColumnOrder), got)
	}
	for i, want := range tstInflightTurnsDDLColumnOrder {
		if got[i].name != want.name {
			t.Errorf("cid %d 列名 = %q, 期望 %q(列序必须与冻结契约的 DDL 一致)", i, got[i].name, want.name)
			continue
		}
		if got[i].notnull != want.notnull {
			t.Errorf("列 %s 的 notnull = %v, 期望 %v", got[i].name, got[i].notnull, want.notnull)
		}
	}
}

// TestSchemaV5InflightTurnsUniqueConvIDAgentID 覆盖冻结契约里"外加一条
// UNIQUE(conv_id, agent_id)"。三条对照插入必须先成功——同 conv_id 换
// agent_id、同 agent_id 换 conv_id——用来证明挡住重复插入的确实是这一对
// 列的组合约束，不是一条更宽的 UNIQUE(conv_id) 或 UNIQUE(agent_id)（那样
// 任何一条对照插入都会被误伤，提前在 Fatalf 里说清楚，不会被最后一条
// "必须失败"的断言悄悄盖过去）。第四条插入与第一条 (conv_id, agent_id)
// 相同、turn_id 不同，必须失败。
//
// inflight_turns.conv_id 在冻结契约给出的 DDL 里没有 REFERENCES，所以
// 不用先插入一条 conversations 行；provider 列没有默认值，固定填字面量
// claude 占位。
func TestSchemaV5InflightTurnsUniqueConvIDAgentID(t *testing.T) {
	s, _ := tstStore(t)
	raw := tstRawDB(t, s.Path())
	ctx := context.Background()

	const insertSQL = `INSERT INTO inflight_turns
	(turn_id, conv_id, agent_id, provider, cursor_seq, scanned_to, started_at)
	VALUES (?, ?, ?, ?, 0, 0, ?)`

	insert := func(turnID, convID, agentID string) error {
		_, err := raw.ExecContext(ctx, insertSQL, turnID, convID, agentID, "claude", tstFixedCreatedAt)
		return err
	}

	if err := insert("turn-1", "conv-a", "agent-a"); err != nil {
		t.Fatalf("对照组：第一条插入失败，说明用例的 INSERT 语句本身有问题: %v", err)
	}
	if err := insert("turn-2", "conv-a", "agent-b"); err != nil {
		t.Fatalf("对照组：同 conv_id 换 agent_id 应当能插入，实际失败: %v"+
			"(说明挡住重复的是一条只看 conv_id 的更宽约束)", err)
	}
	if err := insert("turn-3", "conv-b", "agent-a"); err != nil {
		t.Fatalf("对照组：同 agent_id 换 conv_id 应当能插入，实际失败: %v"+
			"(说明挡住重复的是一条只看 agent_id 的更宽约束)", err)
	}

	err := insert("turn-4", "conv-a", "agent-a")
	if err == nil {
		t.Fatal("(conv_id, agent_id) = (conv-a, agent-a) 插了第二条，UNIQUE(conv_id, agent_id) 没生效")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Errorf("错误信息不像约束失败: %v", err)
	}
}

// TestV5ShapeProviderSessionsIsNotAliasOfV4 钉住冻结契约要求的
// "v5Shape 必须是 v4Shape 的深拷贝再加列，不能是裸别名"。选
// provider_sessions 这张表——它是冻结契约里唯一一张 v5 真的 append 了新列
// (baseline_suspect)的既有表，取法与 schema_v4_shape_test.go 的
// TestV4ShapeConversationsIsNotAliasOfV2OrV3 选 "conversations"（12 契约
// 点名、且 v4 append 了 team_id）是同一个理由。
//
// 顺带比对 v2Shape/v3Shape：冻结契约只提了"不是 v4Shape 的裸别名"，但
// v2Shape/v3Shape/v4Shape 是同一条别名链上的三层(v3Shape = v2Shape 那份
// 别名，schema.go:331)，多比这两层不是画蛇添足——万一 buildV5Shape 哪天
// 抄错写成跳过 v4Shape、直接 v5Shape := v3Shape，只查 v4Shape 未必能
// 分清是这种更离谱的坏法还是"忘了深拷贝"，这里能把坏法定位到具体哪一层。
//
// 这个函数里"改元素"那个子用例结构性地测不出"浅拷贝、没有真的换一块底层
// 数组"这种坏法：provider_sessions 是 v5 用 append 加了 baseline_suspect
// 的表，append 到一个 len == cap 的 slice(buildV4Shape 用
// make(len(cols)) 造出来的，len 与 cap 总是相等)必然触发扩容、换一块
// 全新的底层数组——不管 buildV5Shape 那一步的拷贝写对了还是写成了浅拷贝，
// append 这一步本身就已经把 v5Shape["provider_sessions"] 与
// v4Shape["provider_sessions"] 切开了。这跟 schema_v4_shape_test.go 里
// "conversations" 探测到的结构性盲点是同一回事；真正会暴露浅拷贝的是
// TestV5ShapeUntouchedTableElementIsNotAliasOfV4。两个子用例都要有，
// 缺一个都测不出另一种坏法。
func TestV5ShapeProviderSessionsIsNotAliasOfV4(t *testing.T) {
	wantV4 := cloneColumnShapes(v4Shape["provider_sessions"])
	wantV3 := cloneColumnShapes(v3Shape["provider_sessions"])
	wantV2 := cloneColumnShapes(v2Shape["provider_sessions"])
	if len(wantV4) == 0 {
		t.Fatal("v4Shape 里 provider_sessions 是空的, 本用例的前提不成立")
	}

	t.Run("改键(对 v5Shape 的某个键整体重新赋值)不影响 v4Shape/v3Shape/v2Shape", func(t *testing.T) {
		if _, ok := v5Shape["provider_sessions"]; !ok {
			t.Fatal(`v5Shape 里没有 "provider_sessions" 这张表, 本用例的前提不成立`)
		}
		// 存的是原 slice 本身(引用)，不是深拷贝：下面重新赋值
		// v5Shape["provider_sessions"] 之前从不往这个原 slice 的底层数组写
		// 字节(append 到的是 cloneColumnShapes 另开的一块新内存)，恢复这个
		// 引用是安全的。理由与 schema_v4_shape_test.go 同名注释完全一致。
		orig := v5Shape["provider_sessions"]
		t.Cleanup(func() {
			v5Shape["provider_sessions"] = orig
		})

		mutated := append(cloneColumnShapes(v5Shape["provider_sessions"]),
			columnShape{name: "tst_v5_shape_alias_probe_key", typ: "TEXT", notnull: false})
		v5Shape["provider_sessions"] = mutated

		assertColumnShapesEqual(t, `v4Shape["provider_sessions"]`, v4Shape["provider_sessions"], wantV4)
		assertColumnShapesEqual(t, `v3Shape["provider_sessions"]`, v3Shape["provider_sessions"], wantV3)
		assertColumnShapesEqual(t, `v2Shape["provider_sessions"]`, v2Shape["provider_sessions"], wantV2)
	})

	t.Run("改元素(对 v5Shape 某个键的 slice 按下标赋值)不影响 v4Shape/v3Shape/v2Shape", func(t *testing.T) {
		cols, ok := v5Shape["provider_sessions"]
		if !ok || len(cols) == 0 {
			t.Fatal(`v5Shape["provider_sessions"] 为空或不存在, 本用例的前提不成立`)
		}
		origElem := cols[0]
		t.Cleanup(func() {
			v5Shape["provider_sessions"][0] = origElem
		})

		v5Shape["provider_sessions"][0] = columnShape{
			name: "tst_v5_shape_alias_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}

		assertColumnShapesEqual(t, `v4Shape["provider_sessions"]`, v4Shape["provider_sessions"], wantV4)
		assertColumnShapesEqual(t, `v3Shape["provider_sessions"]`, v3Shape["provider_sessions"], wantV3)
		assertColumnShapesEqual(t, `v2Shape["provider_sessions"]`, v2Shape["provider_sessions"], wantV2)
	})
}

// TestV5ShapeUntouchedTableElementIsNotAliasOfV4 补上
// TestV5ShapeProviderSessionsIsNotAliasOfV4 留下的结构性盲点(见该函数的
// doc comment)：要抓到"浅拷贝、buildV5Shape 没有真的 make+copy 出一块
// 新底层数组"这种坏法，必须挑一张 v5 完全没有 append 过新列的表。
//
// 动态挑表，不写死表名：遍历 v5Shape，与 v4Shape 逐表比列数
// (len(v5cols) == len(v4cols) && len(v5cols) > 0)，列数相同说明 v5 没有
// 对它 append 过，入选。冻结契约下有两张表结构性地被这个条件排除：
//   - provider_sessions：v5DDL 给它 ALTER ADD COLUMN 了 baseline_suspect，
//     v5Shape 里这张表的列数比 v4Shape 多 1，长度比较那一步直接不相等；
//   - inflight_turns：v5DDL 全新建的表，v4Shape 里压根没有这张表，
//     v4cols, ok := v4Shape[table]; if !ok { continue } 这一步就跳过了。
//
// 截至这版冻结契约，候选是 schema_migrations、conversations、
// participants、messages、cursors、turn_usage 六张——v1 到 v4 历次迁移都
// 没有再碰过它们的列。排序后取字典序第一个，今天算出来是 "conversations"，
// 但选择逻辑本身不写死这个结果：哪天 v6 给这六张里的某一张也加了列，
// 这条用例会自动避开它、改选下一张还没被碰过的表，不用人工去改探针指向
// 哪张表；若六张全被碰过，前提不成立，用例响亮地 t.Fatal，不悄悄测别的。
func TestV5ShapeUntouchedTableElementIsNotAliasOfV4(t *testing.T) {
	var candidates []string
	for table, v5cols := range v5Shape {
		v4cols, ok := v4Shape[table]
		if !ok {
			continue
		}
		if len(v5cols) == len(v4cols) && len(v5cols) > 0 {
			candidates = append(candidates, table)
		}
	}
	if len(candidates) == 0 {
		t.Fatal("v5Shape 里找不到一张列数与 v4Shape 相同的非空表, " +
			"本用例依赖的前提(v5 至少留一张表没有 append 新列)不成立")
	}
	sort.Strings(candidates)
	probeTable := candidates[0]

	wantV4 := cloneColumnShapes(v4Shape[probeTable])
	wantV3 := cloneColumnShapes(v3Shape[probeTable])
	wantV2 := cloneColumnShapes(v2Shape[probeTable])

	t.Run("改元素(对 v5Shape 里 v5 未新增列的表按下标赋值)不影响 v4Shape/v3Shape/v2Shape", func(t *testing.T) {
		cols, ok := v5Shape[probeTable]
		if !ok || len(cols) == 0 {
			t.Fatalf("v5Shape[%q] 为空或不存在, 本用例的前提不成立", probeTable)
		}
		origElem := cols[0]
		t.Cleanup(func() {
			v5Shape[probeTable][0] = origElem
		})

		v5Shape[probeTable][0] = columnShape{
			name: "tst_v5_shape_untouched_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}

		assertColumnShapesEqual(t, fmt.Sprintf("v4Shape[%q]", probeTable), v4Shape[probeTable], wantV4)
		assertColumnShapesEqual(t, fmt.Sprintf("v3Shape[%q]", probeTable), v3Shape[probeTable], wantV3)
		assertColumnShapesEqual(t, fmt.Sprintf("v2Shape[%q]", probeTable), v2Shape[probeTable], wantV2)
	})
}
