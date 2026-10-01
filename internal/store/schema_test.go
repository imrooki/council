// schema_test.go 验证 08 规格 D 节那套 DDL 真的建出来了，而且约束真的生效。
// 约束类用例走测试自己的底层连接直插，绕开 store 的入口校验 ——
// 要验的是数据库层面的兜底，不是 Go 里的 if。
package store

import (
	"context"
	"strings"
	"testing"

	"council/internal/conversation"
)

// tstSchemaTables 是 05 §5 列的六张表。schema_migrations 另算，单独一条用例。
var tstSchemaTables = []string{
	"conversations",
	"participants",
	"messages",
	"cursors",
	"turn_usage",
	"provider_sessions",
}

// tstInsertMessageSQL 是底层直插消息用的语句。
//
// 它只列本组用例关心的列，三个脑子列（sender_provider / sender_model / sender_effort）
// 不在其中，取的是 DDL 给的默认空字符串 —— 正因如此它插出来的永远是「不带脑子」的消息，
// 拿它去试那条 CHECK 是试不出来的。要控制脑子列，用 brain_test.go 的
// tstInsertBrainMessageSQL。messages 的十四列与列序在那边的 tstMessageColumns 里钉着。
const tstInsertMessageSQL = `INSERT INTO messages
	(id, conv_id, seq, sender_id, recipients, kind, text, thought, parent_id, created_at, meta)
	VALUES (?, ?, ?, ?, '[]', ?, '', '', NULL, ?, '{}')`

// tstFixedCreatedAt 是直插时用的时间戳，RFC3339Nano、UTC。
const tstFixedCreatedAt = "2026-09-20T00:00:00Z"

func TestSchemaHasAllTables(t *testing.T) {
	s, _ := tstStore(t)
	raw := tstRawDB(t, s.Path())

	for _, name := range tstSchemaTables {
		t.Run("表 "+name+" 存在", func(t *testing.T) {
			n := tstScanInt(t, raw,
				`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name)
			if n != 1 {
				t.Errorf("sqlite_master 里名为 %q 的表有 %d 张，期望 1 张", name, n)
			}
		})
	}

	t.Run("表 schema_migrations 存在", func(t *testing.T) {
		n := tstScanInt(t, raw,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`)
		if n != 1 {
			t.Errorf("schema_migrations 有 %d 张，期望 1 张", n)
		}
	})
}

func TestSchemaConstraints(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "约束用例的会话")
	raw := tstRawDB(t, s.Path())

	// 对照组：同样的语句、合法的 kind，必须插得进去。
	// 没有这一条的话，下面几条「插不进去」可能只是因为语句本身写错了。
	t.Run("对照组：合法的 kind 能插进去", func(t *testing.T) {
		_, err := raw.ExecContext(ctx, tstInsertMessageSQL,
			"ctrl-ok", conv.ID, 900, "user", string(conversation.KindSystem), tstFixedCreatedAt)
		if err != nil {
			t.Fatalf("合法直插失败，说明用例的 INSERT 语句本身有问题: %v", err)
		}
	})

	t.Run("kind 的 CHECK 约束拦住非法值", func(t *testing.T) {
		_, err := raw.ExecContext(ctx, tstInsertMessageSQL,
			"bad-kind", conv.ID, 901, "user", "bogus", tstFixedCreatedAt)
		if err == nil {
			t.Fatal("kind = 'bogus' 竟然插进去了，CHECK 约束没生效")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
			t.Errorf("错误信息不像约束失败: %v", err)
		}
	})

	t.Run("UNIQUE(conv_id, seq) 拦住重复的 seq", func(t *testing.T) {
		if _, err := raw.ExecContext(ctx, tstInsertMessageSQL,
			"dup-first", conv.ID, 910, "user", string(conversation.KindUser), tstFixedCreatedAt); err != nil {
			t.Fatalf("第一条直插失败: %v", err)
		}
		_, err := raw.ExecContext(ctx, tstInsertMessageSQL,
			"dup-second", conv.ID, 910, "user", string(conversation.KindUser), tstFixedCreatedAt)
		if err == nil {
			t.Fatal("同一个 (conv_id, seq) 插了第二行，UNIQUE 约束没生效")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
			t.Errorf("错误信息不像约束失败: %v", err)
		}
	})

	t.Run("外键拦住指向不存在会话的消息", func(t *testing.T) {
		_, err := raw.ExecContext(ctx, tstInsertMessageSQL,
			"orphan", "conv-does-not-exist", 1, "user", string(conversation.KindUser), tstFixedCreatedAt)
		if err == nil {
			t.Fatal("指向不存在会话的消息插进去了：DDL 里缺 REFERENCES，或者外键没被打开")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
			t.Errorf("错误信息不像约束失败: %v", err)
		}
	})
}

// TestForeignKeysEnforcedOnStoreConnection 是 foreign_keys pragma 的行为回归。
// pragma 是连连接的，测试的底层句柄看不到 store 自己那条连接上的设置；
// 而 pragma 没生效时外键是静默失效的，不会报错。所以换一个角度验：
// 让 store 去写两张带外键的表、指向一个不存在的会话，写不进去才说明兜底还在。
// 这条红了先看 DSN 里的 _pragma=foreign_keys(1) 还在不在（D-10）。
func TestForeignKeysEnforcedOnStoreConnection(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	const missingConv = "conv-does-not-exist"

	tests := []struct {
		name string
		call func(ctx context.Context, tx conversation.Tx) error
	}{
		{
			name: "AddParticipant 指向不存在的会话",
			call: func(ctx context.Context, tx conversation.Tx) error {
				return tx.AddParticipant(ctx, conversation.Participant{
					ConvID:  missingConv,
					AgentID: "reviewer",
					Active:  true,
				})
			},
		},
		{
			name: "SetCursor 指向不存在的会话",
			call: func(ctx context.Context, tx conversation.Tx) error {
				return tx.SetCursor(ctx, conversation.Cursor{
					ConvID:  missingConv,
					AgentID: "reviewer",
					Seq:     1,
				})
			},
		},
		{
			name: "AppendMessage 指向不存在的会话",
			call: func(ctx context.Context, tx conversation.Tx) error {
				_, err := tx.AppendMessage(ctx, tstUserMsg(missingConv, "孤儿消息"))
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+"必须失败", func(t *testing.T) {
			err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
				return tc.call(ctx, tx)
			})
			if err == nil {
				t.Fatalf("%s 竟然成功了", tc.name)
			}
		})
	}

	t.Run("失败之后库里一行都没多", func(t *testing.T) {
		raw := tstRawDB(t, s.Path())
		for _, table := range []string{"participants", "cursors", "messages"} {
			if n := tstScanInt(t, raw, `SELECT COUNT(*) FROM `+table); n != 0 {
				t.Errorf("表 %s 有 %d 行，期望 0 行", table, n)
			}
		}
	})
}
