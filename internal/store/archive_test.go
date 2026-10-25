// archive_test.go 覆盖 schema v8 的归档语义：
//
//   - ArchiveConversation / UnarchiveConversation 幂等，未知或已删会话报
//     ErrConversationNotFound；
//   - 默认列表不含已归档，ArchivedConversations 只含已归档，两边都不含已删；
//   - **只有列表**过滤归档：详情、消息、成员、游标、用量与续接照常可读。
//
// 最后一条是本片的全部意义。归档不是软删除，它只回答「主列表里还出不出现」；
// 若把 archived_at 加进 visibleConv，已归档的会话会打不开，正好与需求相反。
package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"council/internal/conversation"
)

// tstArchiveSeed 给一场会话铺上成员、消息、游标与用量，作为归档前后
// 逐条比对读路径的样本。只铺会被读方法看到的四张表：归档不该碰它们。
func tstArchiveSeed(t *testing.T, ctx context.Context, s *Store, title string) conversation.Conversation {
	t.Helper()
	conv := tstNewConv(t, ctx, s, title)
	tstJoin(t, ctx, s, conversation.Participant{
		ConvID:      conv.ID,
		AgentID:     "alpha",
		DisplayName: "Alpha",
		Provider:    "claude",
		Active:      true,
	})
	tstAppend(t, ctx, s, tstUserMsg(conv.ID, "归档前后都要读得到"))
	tstSetCursor(t, ctx, s, conversation.Cursor{
		ConvID:  conv.ID,
		AgentID: "alpha",
		Seq:     1,
	})
	if err := tstRecordUsage(t, ctx, s, tstBaseUsage(conv.ID, "alpha", "turn-"+conv.ID)); err != nil {
		t.Fatalf("造归档样本的用量失败: %v", err)
	}
	return conv
}

// tstArchivedAt 读库里原始的 archived_at。NULL 用 Valid=false 表示。
func tstArchivedAt(t *testing.T, raw *sql.DB, convID string) sql.NullString {
	t.Helper()
	var at sql.NullString
	if err := raw.QueryRow(
		`SELECT archived_at FROM conversations WHERE id = ?`, convID,
	).Scan(&at); err != nil {
		t.Fatalf("读 %s 的 archived_at 失败: %v", convID, err)
	}
	return at
}

// TestArchiveConversationHidesOnlyFromDefaultList 是本片的核心边界：
// 归档之后默认列表看不见它、归档列表看得见，而所有带 convID 的读路径
// 一条都不少。
func TestArchiveConversationHidesOnlyFromDefaultList(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstArchiveSeed(t, ctx, s, "会被归档的会话")
	kept := tstArchiveSeed(t, ctx, s, "留下的会话")

	if err := s.ArchiveConversation(ctx, conv.ID); err != nil {
		t.Fatalf("ArchiveConversation 失败: %v", err)
	}

	t.Run("默认列表只剩未归档", func(t *testing.T) {
		list, err := s.Conversations(ctx)
		if err != nil {
			t.Fatalf("Conversations 失败: %v", err)
		}
		if len(list) != 1 || list[0].ID != kept.ID {
			t.Errorf("默认列表 = %+v，期望只剩 %s", list, kept.ID)
		}
		if list[0].Archived {
			t.Errorf("未归档会话的 Archived = true")
		}
	})

	t.Run("归档列表只含已归档", func(t *testing.T) {
		list, err := s.ArchivedConversations(ctx)
		if err != nil {
			t.Fatalf("ArchivedConversations 失败: %v", err)
		}
		if len(list) != 1 || list[0].ID != conv.ID {
			t.Fatalf("归档列表 = %+v，期望只有 %s", list, conv.ID)
		}
		if !list[0].Archived {
			t.Errorf("归档会话的 Archived = false，期望 true")
		}
		if list[0].Title != conv.Title {
			t.Errorf("归档列表的标题 = %q，期望 %q（归档不该改标题）", list[0].Title, conv.Title)
		}
	})

	t.Run("详情照常可读且标出已归档", func(t *testing.T) {
		got, err := s.Conversation(ctx, conv.ID)
		if err != nil {
			t.Fatalf("归档后 Conversation 失败: %v", err)
		}
		if !got.Archived {
			t.Error("归档后 Conversation 的 Archived = false，期望 true")
		}
		if got.Title != conv.Title || got.Cwd != conv.Cwd || got.TeamID != conv.TeamID {
			t.Errorf("归档改动了会话字段: %+v，期望标题/cwd/team 原样", got)
		}
	})

	t.Run("消息、成员、游标、用量照常可读", func(t *testing.T) {
		msgs, err := s.Messages(ctx, conv.ID, 0, 0)
		if err != nil {
			t.Fatalf("归档后 Messages 失败: %v", err)
		}
		if len(msgs) != 1 || msgs[0].Text != "归档前后都要读得到" {
			t.Errorf("归档后 Messages = %+v，期望仍读到那一条", msgs)
		}
		if seq, err := s.MaxSeq(ctx, conv.ID); err != nil || seq != 1 {
			t.Errorf("归档后 MaxSeq = %d, err=%v，期望 1", seq, err)
		}
		parts, err := s.Participants(ctx, conv.ID, true)
		if err != nil || len(parts) != 1 {
			t.Errorf("归档后 Participants = %+v, err=%v，期望 alpha", parts, err)
		}
		cursors, err := s.Cursors(ctx, conv.ID)
		if err != nil || len(cursors) != 1 {
			t.Errorf("归档后 Cursors = %+v, err=%v，期望一条", cursors, err)
		}
		usages, err := s.TurnUsages(ctx, conv.ID, "", -1)
		if err != nil || len(usages) != 1 {
			t.Errorf("归档后 TurnUsages = %+v, err=%v，期望一条", usages, err)
		}
	})

	t.Run("只写 archived_at，别的列一个不动", func(t *testing.T) {
		raw := tstRawDB(t, s.Path())
		var title, createdAt, deletedAt sql.NullString
		var archivedAt sql.NullString
		if err := raw.QueryRow(
			`SELECT title, created_at, deleted_at, archived_at FROM conversations WHERE id = ?`, conv.ID,
		).Scan(&title, &createdAt, &deletedAt, &archivedAt); err != nil {
			t.Fatalf("读归档后的行失败: %v", err)
		}
		if title.String != conv.Title {
			t.Errorf("title = %q，期望 %q", title.String, conv.Title)
		}
		if createdAt.String != formatTime(conv.CreatedAt) {
			t.Errorf("created_at = %q，期望 %q", createdAt.String, formatTime(conv.CreatedAt))
		}
		if deletedAt.Valid {
			t.Errorf("deleted_at = %q，归档不该软删会话", deletedAt.String)
		}
		if !archivedAt.Valid || archivedAt.String == "" {
			t.Fatalf("archived_at = %#v，期望一个非空时间戳", archivedAt)
		}
		if _, err := parseTime(archivedAt.String); err != nil {
			t.Errorf("archived_at = %q 不是可解析的 RFC3339Nano 时间: %v", archivedAt.String, err)
		}
	})
}

// TestArchiveConversationIsIdempotent 钉住「重复归档不报错、不改第一次
// 的时间」，以及取消归档把 archived_at 写回 NULL、再来一次仍是 nil。
func TestArchiveConversationIsIdempotent(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "幂等归档")
	raw := tstRawDB(t, s.Path())

	if err := s.ArchiveConversation(ctx, conv.ID); err != nil {
		t.Fatalf("第一次 ArchiveConversation 失败: %v", err)
	}
	first := tstArchivedAt(t, raw, conv.ID)
	if !first.Valid {
		t.Fatal("第一次归档后 archived_at 是 NULL")
	}

	if err := s.ArchiveConversation(ctx, conv.ID); err != nil {
		t.Errorf("第二次 ArchiveConversation = %v，期望 nil（幂等）", err)
	}
	second := tstArchivedAt(t, raw, conv.ID)
	if second != first {
		t.Errorf("重复归档把 archived_at 从 %q 改成 %q，期望保留第一次的时间", first.String, second.String)
	}

	if err := s.UnarchiveConversation(ctx, conv.ID); err != nil {
		t.Fatalf("UnarchiveConversation 失败: %v", err)
	}
	if got := tstArchivedAt(t, raw, conv.ID); got.Valid {
		t.Errorf("取消归档后 archived_at = %q，期望 NULL", got.String)
	}
	list, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("取消归档后 Conversations 失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != conv.ID || list[0].Archived {
		t.Errorf("取消归档后默认列表 = %+v，期望会话回来且 Archived=false", list)
	}
	if archived, err := s.ArchivedConversations(ctx); err != nil || len(archived) != 0 {
		t.Errorf("取消归档后归档列表 = %+v, err=%v，期望空切片", archived, err)
	}

	if err := s.UnarchiveConversation(ctx, conv.ID); err != nil {
		t.Errorf("第二次 UnarchiveConversation = %v，期望 nil（幂等）", err)
	}
}

// TestArchiveConversationNotFound 钉住两种「不存在」：未知 id，以及已经
// 软删的会话。后者即便库里还留着行，也不能被归档/取消归档复活。
func TestArchiveConversationNotFound(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)

	for _, id := range []string{"conv-does-not-exist", ""} {
		if err := s.ArchiveConversation(ctx, id); !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("ArchiveConversation(%q) = %v，期望 ErrConversationNotFound", id, err)
		}
		if err := s.UnarchiveConversation(ctx, id); !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("UnarchiveConversation(%q) = %v，期望 ErrConversationNotFound", id, err)
		}
	}

	conv := tstNewConv(t, ctx, s, "删掉的会话不能归档")
	if err := s.SoftDeleteConversation(ctx, conv.ID); err != nil {
		t.Fatalf("SoftDeleteConversation 失败: %v", err)
	}
	if err := s.ArchiveConversation(ctx, conv.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("归档已删会话 = %v，期望 ErrConversationNotFound", err)
	}
	if err := s.UnarchiveConversation(ctx, conv.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("取消归档已删会话 = %v，期望 ErrConversationNotFound", err)
	}
	if got := tstArchivedAt(t, tstRawDB(t, s.Path()), conv.ID); got.Valid {
		t.Errorf("被拒的归档在已删会话上写下了 archived_at = %q", got.String)
	}
}

// TestArchiveDeletedConversationStaysHidden 覆盖「先归档后删除」这条真实
// 顺序：删除的优先级必须高于归档，已经删掉的归档会话既不在归档列表里，
// 也不能被取消归档重新拿出来。
func TestArchiveDeletedConversationStaysHidden(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	active := tstNewConv(t, ctx, s, "正常的会话")
	gone := tstNewConv(t, ctx, s, "归档后又删掉的会话")

	if err := s.ArchiveConversation(ctx, gone.ID); err != nil {
		t.Fatalf("ArchiveConversation 失败: %v", err)
	}
	if err := s.SoftDeleteConversation(ctx, gone.ID); err != nil {
		t.Fatalf("SoftDeleteConversation 失败: %v", err)
	}

	list, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations 失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != active.ID {
		t.Errorf("默认列表 = %+v，期望只剩未删未归档的 %s", list, active.ID)
	}
	archived, err := s.ArchivedConversations(ctx)
	if err != nil {
		t.Fatalf("ArchivedConversations 失败: %v", err)
	}
	if len(archived) != 0 {
		t.Errorf("归档列表 = %+v，期望不含已删会话", archived)
	}
	if _, err := s.Conversation(ctx, gone.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("已删会话的 Conversation = %v，期望 ErrConversationNotFound", err)
	}
	if err := s.UnarchiveConversation(ctx, gone.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("取消归档已删会话 = %v，期望 ErrConversationNotFound", err)
	}
}

// TestArchiveListsOnEmptyDatabase 钉住两个列表在空库上返回空切片不是 nil。
func TestArchiveListsOnEmptyDatabase(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)

	active, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations 失败: %v", err)
	}
	if active == nil || len(active) != 0 {
		t.Errorf("空库默认列表 = %#v，期望非 nil 的空切片", active)
	}
	archived, err := s.ArchivedConversations(ctx)
	if err != nil {
		t.Fatalf("ArchivedConversations 失败: %v", err)
	}
	if archived == nil || len(archived) != 0 {
		t.Errorf("空库归档列表 = %#v，期望非 nil 的空切片", archived)
	}
}

// TestArchivePersistsAcrossReopen 钉住归档状态落在库里：关掉再开，默认列表
// 仍然不含它、归档列表仍然含它，archived_at 一个字没改。
func TestArchivePersistsAcrossReopen(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "重开后仍然归档")
	if err := s.ArchiveConversation(ctx, conv.ID); err != nil {
		t.Fatalf("ArchiveConversation 失败: %v", err)
	}
	before := tstArchivedAt(t, tstRawDB(t, s.Path()), conv.ID)
	closeFn()

	s2, _ := tstOpenAt(t, dir)
	ctx2 := tstCtx(t)
	active, err := s2.Conversations(ctx2)
	if err != nil {
		t.Fatalf("重开后 Conversations 失败: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("重开后默认列表 = %+v，期望不含已归档会话", active)
	}
	archived, err := s2.ArchivedConversations(ctx2)
	if err != nil {
		t.Fatalf("重开后 ArchivedConversations 失败: %v", err)
	}
	if len(archived) != 1 || archived[0].ID != conv.ID || !archived[0].Archived {
		t.Fatalf("重开后归档列表 = %+v，期望只有 %s 且 Archived=true", archived, conv.ID)
	}
	after := tstArchivedAt(t, tstRawDB(t, s2.Path()), conv.ID)
	if after != before {
		t.Errorf("重开后 archived_at = %q，期望 %q（重开/迁移不许改写它）", after.String, before.String)
	}
}
