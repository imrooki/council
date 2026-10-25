// schema_lag_health_test.go 是 A3 在 HTTP 层的端到端验收：一个真正缺
// archived_at / workspace_id、也没有 workspaces 表的 v7 库，用只读模式挂到
// handler 上之后，/api/health 必须给出 schema_version=7、schema_lag=true，
// 会话列表与分组列表仍然能读。
package serve

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"
)

// TestServerHealthSchemaLagOnOldDatabase 造一个 v7 库（从最新库删掉 v8/v9
// 的结构），只读打开之后检查 health 与两个读端点。
//
// 造法用 DROP COLUMN 而不是在 serve 包里重抄一遍 v1–v7 的 DDL：这里要验的
// 是 handler 的接线，不是迁移本身；真正的 v7 建库路径由 store 包的
// TestOpenReadOnlyV7DatabaseReadsWithLag 逐条钉住。
func TestServerHealthSchemaLagOnOldDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()

	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatalf("建测试库失败: %v", err)
	}
	var conv conversation.Conversation
	if err := st.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, "v7 库里的会话")
		return err
	}); err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	if err := st.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		_, err := tx.AppendMessage(ctx, conversation.Message{
			ConvID:   conv.ID,
			SenderID: conversation.ReservedSenderUser,
			Kind:     conversation.KindUser,
			Text:     "v7 库里的一条消息",
		})
		return err
	}); err != nil {
		t.Fatalf("追加消息失败: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关闭测试库失败: %v", err)
	}

	// 把最新库退回 v7 的形状：删掉 v9 的索引/列/表，再删掉账本里 v8/v9 两行。
	dbPath := filepath.Join(dir, "council.db")
	raw, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("打开原始库失败: %v", err)
	}
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_conversations_workspace`,
		`ALTER TABLE conversations DROP COLUMN workspace_id`,
		`ALTER TABLE conversations DROP COLUMN archived_at`,
		`DROP TABLE workspaces`,
		`DELETE FROM schema_migrations WHERE version >= 8`,
	} {
		if _, err := raw.ExecContext(ctx, stmt); err != nil {
			_ = raw.Close()
			t.Fatalf("把库退回 v7 时执行 %q 失败: %v", stmt, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("关闭原始库失败: %v", err)
	}

	ro, err := store.OpenReadOnly(ctx, dir)
	if err != nil {
		t.Fatalf("只读打开 v7 库失败: %v", err)
	}
	defer func() {
		if err := ro.Close(); err != nil {
			t.Errorf("关闭只读库失败: %v", err)
		}
	}()

	srv := NewServer(ro, "test-version", nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var health struct {
		SchemaVersion         int64 `json:"schema_version"`
		SchemaExpectedVersion int64 `json:"schema_expected_version"`
		SchemaLag             bool  `json:"schema_lag"`
		Writable              bool  `json:"writable"`
	}
	getJSON(t, ts.URL+"/api/health", http.StatusOK, &health)
	if health.SchemaVersion != 7 {
		t.Errorf("health.schema_version = %d，期望 7", health.SchemaVersion)
	}
	if want := ro.ExpectedSchemaVersion(); health.SchemaExpectedVersion != want {
		t.Errorf("health.schema_expected_version = %d，期望 %d", health.SchemaExpectedVersion, want)
	}
	if !health.SchemaLag {
		t.Error("v7 库的 health.schema_lag = false，期望 true")
	}
	if health.Writable {
		t.Error("只读 serve 的 health.writable = true，期望不出现/false")
	}

	var list []map[string]any
	getJSON(t, ts.URL+"/api/conversations", http.StatusOK, &list)
	if len(list) != 1 || list[0]["id"] != conv.ID {
		t.Fatalf("只读 v7 库的会话列表 = %v，期望只有 %s", list, conv.ID)
	}
	if list[0]["archived"] != false || list[0]["workspace_id"] != "" {
		t.Errorf("v7 会话的零值列读成了 %v，期望 archived=false、workspace_id 空串", list[0])
	}

	var page messagesResponse
	getJSON(t, ts.URL+"/api/conversations/"+conv.ID+"/messages", http.StatusOK, &page)
	if len(page.Messages) != 1 || page.Messages[0].Text != "v7 库里的一条消息" {
		t.Errorf("v7 库的消息 = %+v，期望一条", page.Messages)
	}

	// v7 没有 workspaces 表：左栏必须拿到 []，不能让整次刷新 500。
	var workspaces []map[string]any
	getJSON(t, ts.URL+"/api/workspaces", http.StatusOK, &workspaces)
	if workspaces == nil || len(workspaces) != 0 {
		t.Errorf("v7 库的工作区列表 = %#v，期望非 nil 的空数组", workspaces)
	}
}
