package store

import (
	"context"
	"strings"
	"testing"

	"council/internal/conversation"
)

// tstInsertBrainMessageSQL 直插一条带脑子的消息，三列由调用方给值。
// 存在的理由是验证 messages 上那条 CHECK 真的拦得住**绕过 Service 的**写入路径 ——
// 走 Service 的路径由 internal/conversation 的用例覆盖，这里覆盖的是另一层。
const tstInsertBrainMessageSQL = `INSERT INTO messages
	(id, conv_id, seq, sender_id, sender_provider, sender_model, sender_effort, recipients, kind, text, thought, parent_id, created_at, meta)
	VALUES (?, ?, ?, ?, ?, ?, ?, '[]', ?, '', '', NULL, ?, '{}')`

// tstMessageColumns 是 messages 的十四列，按 v1 DDL 里的顺序。
//
// 说清楚它守的是什么、不守什么，免得下一个人照着错的理由去改：
//
// 它守的是「v1 DDL 的列序别被人随手重排」。INSERT 与 SELECT 都显式列了列名，
// 所以单是列序变了**不会**把值错位读进相邻字段。真正会造成那种静默损坏的，
// 是 SELECT 的列顺序与 scanMessage 的 Scan 实参顺序走失同步 —— 那一条由
// TestSenderBrainRoundTrip 与 TestSenderBrainDoesNotBleedBetweenMessages 守着
// （变异测试实测：任一边单独对调两个字段，这两个用例就红）。
//
// 它只对**新建库**成立。SQLite 的 ALTER TABLE ADD COLUMN 只能往末尾加，
// 将来由 v2 升上来的库列序会与新建库不同，那时这条断言要跟着改，见未决 #10。
var tstMessageColumns = []string{
	"id", "conv_id", "seq", "sender_id",
	"sender_provider", "sender_model", "sender_effort",
	"recipients", "kind", "text", "thought", "parent_id", "created_at", "meta",
}

// TestMessagesColumnsAndOrder 锁住 messages 的列集合与列序。
func TestMessagesColumnsAndOrder(t *testing.T) {
	s, _ := tstStore(t)
	raw := tstRawDB(t, s.Path())

	rows, err := raw.Query(`SELECT name FROM pragma_table_info('messages') ORDER BY cid`)
	if err != nil {
		t.Fatalf("读 messages 的列信息失败: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("扫描列名失败: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历列信息失败: %v", err)
	}

	if len(got) != len(tstMessageColumns) {
		t.Fatalf("messages 有 %d 列，期望 %d 列；实际列序: %v", len(got), len(tstMessageColumns), got)
	}
	for i, want := range tstMessageColumns {
		if got[i] != want {
			t.Errorf("第 %d 列是 %q，期望 %q；实际列序: %v", i, got[i], want, got)
		}
	}
}

// TestSenderBrainRoundTrip 写进去什么就要读回来什么。
//
// 这是三列的基本用例：AppendMessage 的 INSERT、Messages 的 SELECT、scanMessage 的 Scan
// 任意一处漏掉或错位，这里都会红。
func TestSenderBrainRoundTrip(t *testing.T) {
	cases := []struct {
		name                    string
		senderID                string
		kind                    conversation.MessageKind
		provider, model, effort string
	}{
		{
			name: "三段齐全", senderID: "alpha", kind: conversation.KindAgent,
			provider: "claude", model: "opus", effort: "high",
		},
		{
			// 有的 provider 根本没有 effort 这个概念，只填前两段是合法的。
			name: "只有 provider 与 model", senderID: "beta", kind: conversation.KindAgent,
			provider: "grok", model: "grok-4.6", effort: "",
		},
		{
			// user 消息三列必须空，这是 CHECK 约束的另一面：空值也要能正常往返。
			name: "user 三列全空", senderID: conversation.ReservedSenderUser, kind: conversation.KindUser,
			provider: "", model: "", effort: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := tstStore(t)
			ctx := tstCtx(t)
			conv := tstNewConv(t, ctx, s, "脑子往返用例")

			var appended conversation.Message
			err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
				var err error
				appended, err = tx.AppendMessage(ctx, conversation.Message{
					ConvID:         conv.ID,
					SenderID:       tc.senderID,
					Kind:           tc.kind,
					Text:           "位移场的推导",
					SenderProvider: tc.provider,
					SenderModel:    tc.model,
					SenderEffort:   tc.effort,
				})
				return err
			})
			if err != nil {
				t.Fatalf("追加消息失败: %v", err)
			}

			// 先看 AppendMessage 自己返回的那份：它是调用方立刻拿到的结果，
			// 不该等到重新读一次才正确。
			assertBrain(t, "AppendMessage 返回值", appended, tc.provider, tc.model, tc.effort)

			msgs, err := s.Messages(ctx, conv.ID, 0, 0)
			if err != nil {
				t.Fatalf("读消息失败: %v", err)
			}
			if len(msgs) != 1 {
				t.Fatalf("读回 %d 条消息，期望 1 条", len(msgs))
			}
			assertBrain(t, "从库里读回", msgs[0], tc.provider, tc.model, tc.effort)

			if wantEmpty := tc.provider == "" && tc.model == "" && tc.effort == ""; msgs[0].SenderBrainEmpty() != wantEmpty {
				t.Errorf("SenderBrainEmpty() = %v，期望 %v", msgs[0].SenderBrainEmpty(), wantEmpty)
			}
		})
	}
}

// TestSenderBrainDoesNotBleedBetweenMessages 锁住「每条消息各自带着自己的脑子」。
//
// 这条是本次改动的全部理由：换 model 或 effort 要重建会话（D-4），
// 同一个 agent 在一场对话里会换脑子，旧消息必须保留旧脑子。
// 若哪天有人把这三列挪到 participants 上，这条会立刻红。
func TestSenderBrainDoesNotBleedBetweenMessages(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "换脑子用例")

	// 同一个 agent 先后用两个不同的脑子说话。
	brains := []struct{ provider, model, effort string }{
		{"claude", "sonnet", "low"},
		{"claude", "opus", "high"},
	}
	for i, b := range brains {
		err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
			_, err := tx.AppendMessage(ctx, conversation.Message{
				ConvID:         conv.ID,
				SenderID:       "alpha",
				Kind:           conversation.KindAgent,
				Text:           "第几句都一样",
				SenderProvider: b.provider,
				SenderModel:    b.model,
				SenderEffort:   b.effort,
			})
			return err
		})
		if err != nil {
			t.Fatalf("追加第 %d 条失败: %v", i+1, err)
		}
	}

	msgs, err := s.Messages(ctx, conv.ID, 0, 0)
	if err != nil {
		t.Fatalf("读消息失败: %v", err)
	}
	if len(msgs) != len(brains) {
		t.Fatalf("读回 %d 条，期望 %d 条", len(msgs), len(brains))
	}
	for i, b := range brains {
		assertBrain(t, "第 "+string(rune('1'+i))+" 条", msgs[i], b.provider, b.model, b.effort)
	}
}

// TestSenderBrainCheckConstraint 验证 messages 上那条 CHECK。
//
// 它锁的是「sender 是保留 id user 时三列必须为空」。
// 这一层是结构兜底：Service 那层的友好错误由 internal/conversation 的用例覆盖，
// 这里走底层连接直插，刻意绕开 Service。
func TestSenderBrainCheckConstraint(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "CHECK 用例的会话")
	raw := tstRawDB(t, s.Path())

	// 对照组一：agent 带脑子，必须插得进去。
	// 没有这一条的话，下面「插不进去」可能只是因为语句本身写错了。
	t.Run("对照组：agent 带脑子能插进去", func(t *testing.T) {
		_, err := raw.ExecContext(ctx, tstInsertBrainMessageSQL,
			"brain-ok", conv.ID, 910, "alpha", "claude", "opus", "high",
			string(conversation.KindAgent), tstFixedCreatedAt)
		if err != nil {
			t.Fatalf("合法直插失败，说明用例的 INSERT 语句本身有问题: %v", err)
		}
	})

	// 对照组二：user 不带脑子，必须插得进去。
	t.Run("对照组：user 不带脑子能插进去", func(t *testing.T) {
		_, err := raw.ExecContext(ctx, tstInsertBrainMessageSQL,
			"user-ok", conv.ID, 911, conversation.ReservedSenderUser, "", "", "",
			string(conversation.KindUser), tstFixedCreatedAt)
		if err != nil {
			t.Fatalf("user 不带脑子竟然插不进去: %v", err)
		}
	})

	rejects := []struct {
		name                    string
		kind                    conversation.MessageKind
		provider, model, effort string
	}{
		{"user 带 provider", conversation.KindUser, "claude", "", ""},
		{"user 带 model", conversation.KindUser, "", "opus", ""},
		{"user 带 effort", conversation.KindUser, "", "", "high"},
		{"user 三个都带", conversation.KindUser, "claude", "opus", "high"},
		// 这条锁住「CHECK 只看 sender_id，完全不看 kind」。
		// docs/08_STEP5_SPEC.md 曾经把这一格写反，说 sender=user 配 kind=agent
		// 能带脑子写进去；实际照样被拦。上面四条全是 kind=user，踩不到这个格子。
		{"user 配 kind=agent 也照样带脑子", conversation.KindAgent, "claude", "opus", "high"},
	}
	for i, tc := range rejects {
		t.Run(tc.name+" 被拦住", func(t *testing.T) {
			_, err := raw.ExecContext(ctx, tstInsertBrainMessageSQL,
				"user-bad-"+tc.name, conv.ID, 920+i, conversation.ReservedSenderUser,
				tc.provider, tc.model, tc.effort,
				string(tc.kind), tstFixedCreatedAt)
			if err == nil {
				t.Fatal("带脑子的 user 消息竟然插进去了，CHECK 约束没生效")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
				t.Errorf("错误信息不像约束失败: %v", err)
			}
		})
	}
}

// assertBrain 比对一条消息的三个脑子字段。
func assertBrain(t *testing.T, where string, m conversation.Message, provider, model, effort string) {
	t.Helper()
	if m.SenderProvider != provider {
		t.Errorf("%s: SenderProvider = %q，期望 %q", where, m.SenderProvider, provider)
	}
	if m.SenderModel != model {
		t.Errorf("%s: SenderModel = %q，期望 %q", where, m.SenderModel, model)
	}
	if m.SenderEffort != effort {
		t.Errorf("%s: SenderEffort = %q，期望 %q", where, m.SenderEffort, effort)
	}
}
