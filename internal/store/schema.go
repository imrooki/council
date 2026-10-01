package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrSchemaMismatch 表示磁盘上的表结构与当前二进制期望的不一致。
var ErrSchemaMismatch = errors.New("schema mismatch")

type columnShape struct {
	name    string
	typ     string
	notnull bool
}

type migration struct {
	version int64
	stmts   []string
	// shape 是这一版迁移跑完之后，每张表应当长什么样：表名 -> 有序的列定义。
	shape map[string][]columnShape
}

var migrations = []migration{
	{version: 1, stmts: v1DDL, shape: v1Shape},
	{version: 2, stmts: v2DDL, shape: v2Shape},
	{version: 3, stmts: v3DDL, shape: v3Shape},
	{version: 4, stmts: v4DDL, shape: v4Shape},
	{version: 5, stmts: v5DDL, shape: v5Shape},
}

// v1DDL 一次建齐 schema_migrations 与六张业务表。
// turn_usage 与 provider_sessions 本步只建表，不写方法。
// messages 的 sender_provider / sender_model / sender_effort 三列记的是「发言当时的脑子」，是消息的属性不是 participant 的属性。
// 那条 CHECK 与 kind 的 CHECK 属于同一类结构兜底，锁的是「sender 是保留 id user（见 conversation.ReservedSenderUser）时这三列必须为空」，因为人不是模型，没有脑子可记。
var v1DDL = []string{
	`CREATE TABLE schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
)`,
	`CREATE TABLE conversations (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL,
    created_at TEXT NOT NULL
)`,
	`CREATE TABLE participants (
    conv_id      TEXT NOT NULL REFERENCES conversations(id),
    agent_id     TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    provider     TEXT NOT NULL DEFAULT '',
    active       INTEGER NOT NULL DEFAULT 1,
    joined_at    TEXT NOT NULL,
    left_at      TEXT,
    PRIMARY KEY (conv_id, agent_id)
)`,
	`CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    conv_id         TEXT NOT NULL REFERENCES conversations(id),
    seq             INTEGER NOT NULL,
    sender_id       TEXT NOT NULL,
    sender_provider TEXT NOT NULL DEFAULT '',
    sender_model    TEXT NOT NULL DEFAULT '',
    sender_effort   TEXT NOT NULL DEFAULT '',
    recipients      TEXT NOT NULL DEFAULT '[]',
    kind            TEXT NOT NULL CHECK (kind IN ('user','agent','system','task','task_result')),
    text            TEXT NOT NULL DEFAULT '',
    thought         TEXT NOT NULL DEFAULT '',
    parent_id       TEXT,
    created_at      TEXT NOT NULL,
    meta            TEXT NOT NULL DEFAULT '{}',
    UNIQUE (conv_id, seq),
    CHECK (sender_id <> 'user' OR (sender_provider = '' AND sender_model = '' AND sender_effort = ''))
)`,
	`CREATE INDEX idx_messages_conv_seq ON messages (conv_id, seq)`,
	`CREATE TABLE cursors (
    conv_id    TEXT NOT NULL REFERENCES conversations(id),
    agent_id   TEXT NOT NULL,
    seq        INTEGER NOT NULL DEFAULT 0,
    pending    TEXT NOT NULL DEFAULT '[]',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (conv_id, agent_id)
)`,
	`CREATE TABLE turn_usage (
    conv_id         TEXT NOT NULL REFERENCES conversations(id),
    agent_id        TEXT NOT NULL,
    seq             INTEGER NOT NULL,
    input_tokens    INTEGER NOT NULL DEFAULT 0,
    cache_create    INTEGER NOT NULL DEFAULT 0,
    cache_read      INTEGER NOT NULL DEFAULT 0,
    output_tokens   INTEGER NOT NULL DEFAULT 0,
    thinking_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd        REAL,
    cold_start      INTEGER NOT NULL DEFAULT 0,
    wall_ms         INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (conv_id, agent_id, seq)
)`,
	`CREATE TABLE provider_sessions (
    conv_id             TEXT NOT NULL REFERENCES conversations(id),
    agent_id            TEXT NOT NULL,
    provider_session_id TEXT NOT NULL DEFAULT '',
    last_turn_at        TEXT,
    cold                INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (conv_id, agent_id)
)`,
}

var v1Shape = map[string][]columnShape{
	"schema_migrations": {
		{name: "version", typ: "INTEGER", notnull: false},
		{name: "applied_at", typ: "TEXT", notnull: true},
	},
	"conversations": {
		{name: "id", typ: "TEXT", notnull: false},
		{name: "title", typ: "TEXT", notnull: true},
		{name: "created_at", typ: "TEXT", notnull: true},
	},
	"participants": {
		{name: "conv_id", typ: "TEXT", notnull: true},
		{name: "agent_id", typ: "TEXT", notnull: true},
		{name: "display_name", typ: "TEXT", notnull: true},
		{name: "provider", typ: "TEXT", notnull: true},
		{name: "active", typ: "INTEGER", notnull: true},
		{name: "joined_at", typ: "TEXT", notnull: true},
		{name: "left_at", typ: "TEXT", notnull: false},
	},
	"messages": {
		{name: "id", typ: "TEXT", notnull: false},
		{name: "conv_id", typ: "TEXT", notnull: true},
		{name: "seq", typ: "INTEGER", notnull: true},
		{name: "sender_id", typ: "TEXT", notnull: true},
		{name: "sender_provider", typ: "TEXT", notnull: true},
		{name: "sender_model", typ: "TEXT", notnull: true},
		{name: "sender_effort", typ: "TEXT", notnull: true},
		{name: "recipients", typ: "TEXT", notnull: true},
		{name: "kind", typ: "TEXT", notnull: true},
		{name: "text", typ: "TEXT", notnull: true},
		{name: "thought", typ: "TEXT", notnull: true},
		{name: "parent_id", typ: "TEXT", notnull: false},
		{name: "created_at", typ: "TEXT", notnull: true},
		{name: "meta", typ: "TEXT", notnull: true},
	},
	"cursors": {
		{name: "conv_id", typ: "TEXT", notnull: true},
		{name: "agent_id", typ: "TEXT", notnull: true},
		{name: "seq", typ: "INTEGER", notnull: true},
		{name: "pending", typ: "TEXT", notnull: true},
		{name: "updated_at", typ: "TEXT", notnull: true},
	},
	"turn_usage": {
		{name: "conv_id", typ: "TEXT", notnull: true},
		{name: "agent_id", typ: "TEXT", notnull: true},
		{name: "seq", typ: "INTEGER", notnull: true},
		{name: "input_tokens", typ: "INTEGER", notnull: true},
		{name: "cache_create", typ: "INTEGER", notnull: true},
		{name: "cache_read", typ: "INTEGER", notnull: true},
		{name: "output_tokens", typ: "INTEGER", notnull: true},
		{name: "thinking_tokens", typ: "INTEGER", notnull: true},
		{name: "cost_usd", typ: "REAL", notnull: false},
		{name: "cold_start", typ: "INTEGER", notnull: true},
		{name: "wall_ms", typ: "INTEGER", notnull: true},
	},
	"provider_sessions": {
		{name: "conv_id", typ: "TEXT", notnull: true},
		{name: "agent_id", typ: "TEXT", notnull: true},
		{name: "provider_session_id", typ: "TEXT", notnull: true},
		{name: "last_turn_at", typ: "TEXT", notnull: false},
		{name: "cold", typ: "INTEGER", notnull: true},
	},
}

// v2DDL 整表重建 turn_usage 与 provider_sessions。
//
// 先 DROP 再 CREATE，不写 ALTER：主键从 (conv_id, agent_id, seq) 换成 turn_id，
// ALTER 改不了主键。Step 5 建这两张表时明写「本步只建表，不写方法」，
// 线上不存在任何一行数据，重建是安全的。后来人看到 DROP 不要改回 ALTER。
//
// 列设计的逐条理由：
//
// turn_id TEXT PRIMARY KEY：一轮有了不依赖任何其它表的身份。
// v1 的主键是 (conv_id, agent_id, seq)，而 seq 只有 messages 产得出，
// 于是失败轮要靠一条 turn_failed 的 system 消息当锚点 —— 那是把记账需求
// 塞进对话记录，哪天有人决定某类失败不发面包屑，账就静默少一笔。
//
// seq INTEGER（可空）：成功轮指向它产生的那条消息，失败轮为 NULL。
//
// outcome INTEGER NOT NULL：存 turn.Outcome 的整数值。不存字符串：
// 枚举的字面量不在契约面（09a 明写），存字符串等于把一个可以改的措辞
// 焊进历史行。读出来要翻译时用 Go 那边的 String()。
//
// 七个用量列改可空：NULL 读作「这家没报这个数」，0 读作「实测为 0」。
// Step 6 用真实字节证明了三种情况在 v1 里长得一样：agy 的 usage 里根本
// 没有 cache_creation；grok 的 cache_creation_input_tokens 恒为 0 而
// cache_read 明明从 0 涨到 20608；而三份失败原件里 claude 与 agy 的五项
// 是货真价实的 0。
//
// cost_cumulative_usd REAL：provider 报的累计原值，与算好的 cost_usd
// 分开存。有了它，将来对账可以不依赖差值算法的正确性。
//
// provider_session_id：cost 的累计作用域是 provider 会话，不是
// (conv_id, agent_id)。没有这一列，「这一轮属于哪个会话」在 SQL 里不可查。
//
// delivered_lo / delivered_hi：本轮真正渲染进 prompt 的 seq 区间，
// 对应 04 §7「Router 必须能回答上一轮送了什么给谁」。存区间而不是列表，
// 因为 Step 7 第 2 节那条裁决保证了它一定是一段连续区间；存成 JSON 列表
// 就等于默许将来有人把它变成非连续的。空批次两列都是 NULL。
//
// provider_sessions 把游标搬了进来（cursor_seq）：五样东西同生共死。
// cursors 表保留不动（Step 5 已有，Cursors(convID) 还在用），
// 但本步不再往它写 agent 的会话游标 —— 两处都写就有了两个事实源。
// 不要在 RecordTurn 或 Runner 里再调 SetCursor 去记同一份游标。
//
// last_turn_at TEXT NOT NULL 与 identity_fp TEXT NOT NULL：
// v1 的 last_turn_at 可空，v2 改成必填 —— 一条存在的会话必然有上一轮的时刻，
// 可空只会让「读回时缺一样」这个判断多一种说不清的中间态。
var v2DDL = []string{
	`DROP TABLE IF EXISTS turn_usage`,
	`DROP TABLE IF EXISTS provider_sessions`,
	`CREATE TABLE turn_usage (
    turn_id             TEXT PRIMARY KEY,
    conv_id             TEXT NOT NULL REFERENCES conversations(id),
    agent_id            TEXT NOT NULL,
    provider_session_id TEXT NOT NULL DEFAULT '',
    seq                 INTEGER,
    outcome             INTEGER NOT NULL,
    input_tokens        INTEGER,
    cache_create        INTEGER,
    cache_read          INTEGER,
    output_tokens       INTEGER,
    thinking_tokens     INTEGER,
    cost_usd            REAL,
    cost_cumulative_usd REAL,
    cold_start          INTEGER NOT NULL DEFAULT 0,
    wall_ms             INTEGER NOT NULL DEFAULT 0,
    prompt_bytes        INTEGER NOT NULL DEFAULT 0,
    delivered_lo        INTEGER,
    delivered_hi        INTEGER,
    created_at          TEXT NOT NULL
)`,
	`CREATE TABLE provider_sessions (
    conv_id             TEXT NOT NULL REFERENCES conversations(id),
    agent_id            TEXT NOT NULL,
    provider_session_id TEXT NOT NULL,
    cursor_seq          INTEGER NOT NULL,
    cost_total_usd      REAL,
    have_cost_total     INTEGER NOT NULL DEFAULT 0,
    last_turn_at        TEXT NOT NULL,
    identity_fp         TEXT NOT NULL,
    spoken              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (conv_id, agent_id)
)`,
	`CREATE INDEX idx_turn_usage_session ON turn_usage(provider_session_id)`,
}

// v2Shape 用 v1 的 map 做基底再覆盖 turn_usage 与 provider_sessions。
// 不要复制粘贴一份新的全表 map：改其它表时一定会漏改一处。
// Open 在迁移成功后逐表比对 pragma_table_info，形状必须包含全部表，
// 否则迁移成功后立刻以 ErrSchemaMismatch 失败。
var v2Shape = buildV2Shape()

func buildV2Shape() map[string][]columnShape {
	shape := make(map[string][]columnShape, len(v1Shape))
	for table, cols := range v1Shape {
		copied := make([]columnShape, len(cols))
		copy(copied, cols)
		shape[table] = copied
	}
	shape["turn_usage"] = []columnShape{
		{name: "turn_id", typ: "TEXT", notnull: false},
		{name: "conv_id", typ: "TEXT", notnull: true},
		{name: "agent_id", typ: "TEXT", notnull: true},
		{name: "provider_session_id", typ: "TEXT", notnull: true},
		{name: "seq", typ: "INTEGER", notnull: false},
		{name: "outcome", typ: "INTEGER", notnull: true},
		{name: "input_tokens", typ: "INTEGER", notnull: false},
		{name: "cache_create", typ: "INTEGER", notnull: false},
		{name: "cache_read", typ: "INTEGER", notnull: false},
		{name: "output_tokens", typ: "INTEGER", notnull: false},
		{name: "thinking_tokens", typ: "INTEGER", notnull: false},
		{name: "cost_usd", typ: "REAL", notnull: false},
		{name: "cost_cumulative_usd", typ: "REAL", notnull: false},
		{name: "cold_start", typ: "INTEGER", notnull: true},
		{name: "wall_ms", typ: "INTEGER", notnull: true},
		{name: "prompt_bytes", typ: "INTEGER", notnull: true},
		{name: "delivered_lo", typ: "INTEGER", notnull: false},
		{name: "delivered_hi", typ: "INTEGER", notnull: false},
		{name: "created_at", typ: "TEXT", notnull: true},
	}
	shape["provider_sessions"] = []columnShape{
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
	return shape
}

// v3DDL 这一版不改列，只改数据：给 provider_sessions.identity_fp 追加两段空字段。
//
// 背景：turn.IdentityFingerprint 从四段变成六段，而校验函数会检查段数，
// 于是库里现存的每一条四段指纹都会被判成「整体不符」。
// 「不符」在从库读回那一支的处置是静默作废重建，也就是升级之后第一次调用时
// 每条活着的会话都会悄悄重建一次、各付一次入场费（实测 0.4 到 1.0 美元），
// 没有任何一层会说话。这就是必须有这一步迁移的原因。
//
// 追加的是两个空段（0:;0:;）而不是空串的哈希：turn.RulesDigest("") 规定返回
// 空串，两边正好对上，于是规则为空的用户升级之后一条会话都不重建。
// 规则非空时指纹正确地不符、会话重建，那是对的，前缀真的变了。
//
// 这句「升级不重建」只对某次安全修复（commit 6530839）之后建立的会话成立：
// 那次提交改了实际发出的命令行参数而没有同时动指纹，更早的会话本来就带着
// 过期的前缀，本迁移修不了它们，也不假装修得了。详见 00_DECISIONS 未决 #18。
//
// 不走代码层兼容垫片（让校验函数容忍四段）：垫片是永久的，而且它把段数校验
// 从「严格」变成「看情况」，下一次加字段时没人还记得为什么。
var v3DDL = []string{
	`UPDATE provider_sessions SET identity_fp = identity_fp || '0:;0:;'`,
}

// v3Shape 直接复用 v2Shape，不复制一份新的全表 map：这一版没有改列，
// 形状与 v2 相同；复制会在以后改其它表时漏改一处（与 v2Shape 不复制
// v1Shape 是同一个理由）。两份变量指向同一张 map，初始化之后不会再写。
var v3Shape = v2Shape

// v4DDL 三列一次加齐：conversations.team_id、turn_usage.identity_fp、
// turn_usage.round_id。用 ALTER 而不是 DROP+CREATE：主键没变，
// conversations 里有真数据(docs/12_STEP9_CONTRACT.md 3.3)。
//
// 不拆成两个版本号：schema 版本是一条单调序列，片 2a 只加 team_id、
// 片 2b 再加另外两列会造出 v4/v5 两次迁移，而这三列本来就属于
// 同一次契约面改动。
//
// 这一版不碰 provider_sessions.identity_fp：v3 已经把四段指纹补成
// 六段，再改会让升级后的会话被判不符、静默重建、各付一次入场费。
// ALTER 把新列加在末尾，下面 v4Shape 的追加顺序必须与这三条
// 语句的执行顺序一致，否则 pragma_table_info 按 cid 比对会报
// ErrSchemaMismatch。
var v4DDL = []string{
	`ALTER TABLE conversations ADD COLUMN team_id     TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE turn_usage    ADD COLUMN identity_fp TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE turn_usage    ADD COLUMN round_id    TEXT NOT NULL DEFAULT ''`,
}

// v4Shape 必须从 v3 的形状深拷贝，不能写成 v4Shape = v3Shape。
// v3Shape 是 v2Shape 的别名(两份变量指向同一张 map)，照抄那一行
// 再改键会连 v2 / v3 一起改掉，那两版的形状校验从此对不上历史库，
// 而且不会有任何一层报错：迁移照跑、Open 照样成功，只有拿着
// v2/v3 旧库的人会炸。v5 若要改列，也必须走这条拷贝，不许再抄 v3Shape。
var v4Shape = buildV4Shape()

func buildV4Shape() map[string][]columnShape {
	shape := make(map[string][]columnShape, len(v3Shape))
	for table, cols := range v3Shape {
		copied := make([]columnShape, len(cols))
		copy(copied, cols)
		shape[table] = copied
	}
	// notnull 填 true：DDL 是 NOT NULL DEFAULT ''，pragma_table_info
	// 对这样的列返回 notnull=1。
	shape["conversations"] = append(shape["conversations"], columnShape{
		name: "team_id", typ: "TEXT", notnull: true,
	})
	shape["turn_usage"] = append(shape["turn_usage"],
		columnShape{name: "identity_fp", typ: "TEXT", notnull: true},
		columnShape{name: "round_id", typ: "TEXT", notnull: true},
	)
	return shape
}

// v5DDL 记下正在飞的轮，并给 provider_sessions 补 baseline_suspect。
//
// (conv_id, agent_id) 的 UNIQUE 做成独立唯一索引。turn_id 主键挡不住
// 同一个 (conv, agent) 两行同时在飞（契约 3.8）。十二个列定义要与契约
// 文本逐字一致，写进 CREATE TABLE 会迫使 work_dir 那一行多一个逗号。
// 表级 UNIQUE 在 SQLite 里本来就会建成唯一索引，约束力相同。
// pragma_table_info 只按 cid 给出 name / type / notnull / dflt_value / pk，
// 独立索引不出现在里面，表级 UNIQUE 也不改这些列字段，所以 v5Shape
// 不为这道约束加列。
var v5DDL = []string{
	`CREATE TABLE inflight_turns (
    turn_id            TEXT PRIMARY KEY,
    conv_id            TEXT NOT NULL,
    agent_id           TEXT NOT NULL,
    provider           TEXT NOT NULL,
    planned_session_id TEXT NOT NULL DEFAULT '',
    resume_session_id  TEXT NOT NULL DEFAULT '',
    cursor_seq         INTEGER NOT NULL,
    scanned_to         INTEGER NOT NULL,
    started_at         TEXT NOT NULL,
    phase              TEXT NOT NULL DEFAULT 'begun',
    tier               TEXT NOT NULL DEFAULT '',
    work_dir           TEXT NOT NULL DEFAULT ''
)`,
	`ALTER TABLE provider_sessions ADD COLUMN baseline_suspect INTEGER NOT NULL DEFAULT 0`,
	`CREATE UNIQUE INDEX idx_inflight_turns_conv_agent ON inflight_turns (conv_id, agent_id)`,
}

// v5Shape 从 v4 深拷贝：新的 map，并且每张表一份新的列切片。
// v5Shape = v4Shape 是裸别名，两份变量同一张 map，加 inflight_turns
// 或换掉 provider_sessions 的切片头会把 v4Shape 一起改掉。
// 只 make 新 map、切片头仍指向 v4 时，底层数组还是共享的。
// v4 对 conversations 和 turn_usage 做过 append，那两份的 cap 大于 len，
// 再 append 会把新列写进 v4 的数组。v3Shape = v2Shape 就是这个坑，
// buildV4Shape 已写明改列必须走拷贝。
var v5Shape = buildV5Shape()

func buildV5Shape() map[string][]columnShape {
	shape := make(map[string][]columnShape, len(v4Shape)+1)
	for table, cols := range v4Shape {
		copied := make([]columnShape, len(cols))
		copy(copied, cols)
		shape[table] = copied
	}
	// notnull 填 true：DDL 是 INTEGER NOT NULL DEFAULT 0，
	// pragma_table_info 对这样的列返回 notnull=1。加在切片末尾，
	// 因为 ALTER 把新列加在表的最后一列，cid 按这个顺序比对。
	shape["provider_sessions"] = append(shape["provider_sessions"], columnShape{
		name: "baseline_suspect", typ: "INTEGER", notnull: true,
	})
	// turn_id 的 notnull 必须是 false：DDL 写的是 TEXT PRIMARY KEY，
	// 没有 NOT NULL。SQLite 只对 INTEGER PRIMARY KEY 在 pragma_table_info
	// 里报 notnull=1；TEXT 主键报 notnull=0。conversations.id、
	// messages.id、turn_usage.turn_id 同一体例。其余十一列都写了 NOT NULL。
	shape["inflight_turns"] = []columnShape{
		{name: "turn_id", typ: "TEXT", notnull: false},
		{name: "conv_id", typ: "TEXT", notnull: true},
		{name: "agent_id", typ: "TEXT", notnull: true},
		{name: "provider", typ: "TEXT", notnull: true},
		{name: "planned_session_id", typ: "TEXT", notnull: true},
		{name: "resume_session_id", typ: "TEXT", notnull: true},
		{name: "cursor_seq", typ: "INTEGER", notnull: true},
		{name: "scanned_to", typ: "INTEGER", notnull: true},
		{name: "started_at", typ: "TEXT", notnull: true},
		{name: "phase", typ: "TEXT", notnull: true},
		{name: "tier", typ: "TEXT", notnull: true},
		{name: "work_dir", typ: "TEXT", notnull: true},
	}
	return shape
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migrate tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
		if p := recover(); p != nil {
			panic(p)
		}
	}()

	current, err := currentSchemaVersion(ctx, tx)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		for _, stmt := range m.stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migrate version %d: %w", m.version, err)
			}
		}
		appliedAt := formatTime(time.Now().UTC())
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			m.version, appliedAt,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrate tx: %w", err)
	}
	committed = true
	return nil
}

func currentSchemaVersion(ctx context.Context, tx *sql.Tx) (int64, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("check schema_migrations: %w", err)
	}
	if n == 0 {
		return 0, nil
	}

	var v sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return v.Int64, nil
}

func (s *Store) validateSchema(ctx context.Context) error {
	if len(migrations) == 0 {
		return nil
	}
	shape := migrations[len(migrations)-1].shape
	tables := make([]string, 0, len(shape))
	for table := range shape {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		actual, err := s.tableColumns(ctx, table)
		if err != nil {
			return err
		}
		if err := checkTableShape(table, shape[table], actual); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) tableColumns(ctx context.Context, table string) ([]columnShape, error) {
	rows, err := s.write.QueryContext(ctx,
		`SELECT name, type, "notnull" FROM pragma_table_info(?) ORDER BY cid`,
		table,
	)
	if err != nil {
		return nil, fmt.Errorf("pragma_table_info %s: %w", table, err)
	}
	defer rows.Close()

	var cols []columnShape
	for rows.Next() {
		var name, typ string
		var notnull int
		if err := rows.Scan(&name, &typ, &notnull); err != nil {
			return nil, fmt.Errorf("scan pragma_table_info %s: %w", table, err)
		}
		cols = append(cols, columnShape{name: name, typ: typ, notnull: notnull != 0})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pragma_table_info %s: %w", table, err)
	}
	return cols, nil
}

func checkTableShape(table string, expected, actual []columnShape) error {
	detail := shapeDiff(expected, actual)
	if detail == "" {
		return nil
	}
	return fmt.Errorf("table %s: %s: 这个库是旧版二进制建的，删掉重建（486f5f3 之前不存在任何真实数据）: %w", table, detail, ErrSchemaMismatch)
}

func shapeDiff(expected, actual []columnShape) string {
	expByName := make(map[string]columnShape, len(expected))
	actByName := make(map[string]columnShape, len(actual))
	for _, c := range expected {
		expByName[c.name] = c
	}
	for _, c := range actual {
		actByName[c.name] = c
	}

	var missing, extra []string
	for _, c := range expected {
		if _, ok := actByName[c.name]; !ok {
			missing = append(missing, c.name)
		}
	}
	for _, c := range actual {
		if _, ok := expByName[c.name]; !ok {
			extra = append(extra, c.name)
		}
	}

	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "缺少列 "+strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		parts = append(parts, "多出列 "+strings.Join(extra, ", "))
	}
	if len(missing) == 0 && len(extra) == 0 && len(expected) != len(actual) {
		// 列名集合相同、数量却不同，只可能是 expected 里有重复列名 —— 手写 shape 时的笔误
		// （SQLite 本身不允许一张表有重名列）。下面是按下标配对的，长度不等会越界 panic，
		// 所以先在这里响亮地报出来。
		parts = append(parts, fmt.Sprintf(
			"期望 %d 列、实际 %d 列，但列名集合相同：检查 shape 声明里是不是写重了某一列",
			len(expected), len(actual)))
	} else if len(missing) == 0 && len(extra) == 0 {
		for i := range expected {
			e := expected[i]
			a := actual[i]
			if e.name != a.name {
				parts = append(parts, fmt.Sprintf("cid %d 期望列 %s 实际为 %s", i, e.name, a.name))
				continue
			}
			if e.typ != a.typ || e.notnull != a.notnull {
				parts = append(parts, fmt.Sprintf("列 %s 期望 %s 实际为 %s", e.name, formatShape(e), formatShape(a)))
			}
		}
	}
	return strings.Join(parts, "; ")
}

func formatShape(c columnShape) string {
	if c.notnull {
		return c.typ + " NOT NULL"
	}
	return c.typ
}
