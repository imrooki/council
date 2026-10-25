// workspace_test.go 覆盖 schema v9 的工作区分组语义：
//
//   - Workspaces 按 (sort_order, name) 排序，空库回空切片；
//   - CreateWorkspace / RenameWorkspace 去空白、1–60 字符、重名（大小写不敏感）
//     报 ErrWorkspaceNameTaken；
//   - DeleteWorkspace 在同一个事务里把成员会话的 workspace_id 置 NULL，
//     会话、消息、用量一条都不少；
//   - SetConversationWorkspace 设置 / 清除归属，未知工作区与已删会话都拒绝；
//   - 默认列表与归档列表都带 workspace_id（归档与工作区正交）；
//   - ConversationCount 的口径：已软删的不计，已归档的照计。
package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"council/internal/conversation"
)

// tstWorkspaceIDOf 读库里原始的 workspace_id。NULL 用 Valid=false 表示。
func tstWorkspaceIDOf(t *testing.T, raw *sql.DB, convID string) sql.NullString {
	t.Helper()
	var id sql.NullString
	if err := raw.QueryRow(
		`SELECT workspace_id FROM conversations WHERE id = ?`, convID,
	).Scan(&id); err != nil {
		t.Fatalf("读 %s 的 workspace_id 失败: %v", convID, err)
	}
	return id
}

// tstMustCreateWorkspace 建一个工作区并在失败时直接终止用例。
func tstMustCreateWorkspace(t *testing.T, ctx context.Context, s *Store, name string) conversation.Workspace {
	t.Helper()
	ws, err := s.CreateWorkspace(ctx, name)
	if err != nil {
		t.Fatalf("CreateWorkspace(%q) 失败: %v", name, err)
	}
	return ws
}

// TestWorkspacesEmptyIsSlice 钉住空库的形状：空切片不是 nil，也不是错误。
func TestWorkspacesEmptyIsSlice(t *testing.T) {
	s, _ := tstStore(t)
	got, err := s.Workspaces(tstCtx(t))
	if err != nil {
		t.Fatalf("空库 Workspaces 失败: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("空库 Workspaces = %#v，期望非 nil 的空切片", got)
	}
}

// TestCreateWorkspaceTrimsAndAppends 钉住新建的三件事：名字去空白、
// id 非空、sort_order 依次递增（新组排在末尾）。
func TestCreateWorkspaceTrimsAndAppends(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)

	first := tstMustCreateWorkspace(t, ctx, s, "  工作区一  ")
	if first.Name != "工作区一" {
		t.Errorf("第一个工作区的 Name = %q，期望去空白之后的 %q", first.Name, "工作区一")
	}
	if first.ID == "" {
		t.Error("CreateWorkspace 返回的 ID 是空的")
	}
	if first.SortOrder != 0 {
		t.Errorf("第一个工作区的 SortOrder = %d，期望 0", first.SortOrder)
	}
	if first.CreatedAt.IsZero() {
		t.Error("CreateWorkspace 返回的 CreatedAt 是零值")
	}
	if first.ConversationCount != 0 {
		t.Errorf("新工作区的 ConversationCount = %d，期望 0", first.ConversationCount)
	}

	second := tstMustCreateWorkspace(t, ctx, s, "工作区二")
	if second.SortOrder <= first.SortOrder {
		t.Errorf("第二个工作区的 SortOrder = %d，期望大于 %d（新组排在末尾）", second.SortOrder, first.SortOrder)
	}

	list, err := s.Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces 失败: %v", err)
	}
	if len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Errorf("Workspaces = %+v，期望按创建顺序 [%s, %s]", list, first.ID, second.ID)
	}
	if !list[0].CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("读回的 CreatedAt = %v，期望 %v", list[0].CreatedAt, first.CreatedAt)
	}
}

// TestWorkspacesOrderBySortOrderThenName 钉住第二排序键：sort_order 相同时
// 按 name 升序，而不是让 SQLite 给出未定义的顺序。用直插造两行相同
// sort_order 的数据（正常创建路径永远递增，造不出来）。
//
// 这里的名字都用小写：name 列没有 COLLATE NOCASE，排序走 SQLite 的 BINARY
// 逐字节比较（大写字母排在小写之前）。判重是大小写不敏感的，排序不是 ——
// 两件事分开实现，用例也分开钉：这条只关心「顺序是确定的」。
func TestWorkspacesOrderBySortOrderThenName(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	raw := tstRawDB(t, s.Path())

	for _, row := range []struct {
		id, name string
		order    int
	}{
		{"ws-b", "bravo", 0},
		{"ws-a", "alpha", 0},
		{"ws-c", "charlie", 5},
	} {
		if _, err := raw.ExecContext(ctx,
			`INSERT INTO workspaces (id, name, sort_order, created_at) VALUES (?, ?, ?, ?)`,
			row.id, row.name, row.order, tstFixedCreatedAt,
		); err != nil {
			t.Fatalf("直插工作区 %s 失败: %v", row.id, err)
		}
	}

	list, err := s.Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces 失败: %v", err)
	}
	var order []string
	for _, w := range list {
		order = append(order, w.ID)
	}
	want := []string{"ws-a", "ws-b", "ws-c"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("工作区顺序 = %v，期望 %v（sort_order 相同按 name）", order, want)
	}
	if list[2].SortOrder != 5 {
		t.Errorf("ws-c 的 SortOrder = %d，期望 5", list[2].SortOrder)
	}
}

// TestWorkspaceNameValidation 钉住名字规则：去空白后 1–60 个 rune。
// 60 个汉字（180 字节）必须能过，按字节算会误拒。
func TestWorkspaceNameValidation(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)

	for _, bad := range []string{"", "   ", "\t\n", strings.Repeat("字", MaxWorkspaceNameChars+1)} {
		if _, err := s.CreateWorkspace(ctx, bad); !errors.Is(err, ErrInvalidWorkspaceName) {
			t.Errorf("CreateWorkspace(%q) = %v，期望 ErrInvalidWorkspaceName", bad, err)
		}
	}

	if _, err := s.CreateWorkspace(ctx, strings.Repeat("字", MaxWorkspaceNameChars)); err != nil {
		t.Errorf("%d 个汉字的名字应当能建，实际: %v", MaxWorkspaceNameChars, err)
	}

	ws := tstMustCreateWorkspace(t, ctx, s, "可以改的名字")
	for _, bad := range []string{"", "  ", strings.Repeat("x", MaxWorkspaceNameChars+1)} {
		if _, err := s.RenameWorkspace(ctx, ws.ID, bad); !errors.Is(err, ErrInvalidWorkspaceName) {
			t.Errorf("RenameWorkspace(%q) = %v，期望 ErrInvalidWorkspaceName", bad, err)
		}
	}
	if name, err := workspaceNameOf(t, s, ws.ID); err != nil || name != "可以改的名字" {
		t.Errorf("被拒的改名之后名字 = %q, err=%v，期望原样", name, err)
	}
}

// workspaceNameOf 从 Workspaces 里找出某个 id 的名字，供「被拒的写没有生效」断言。
func workspaceNameOf(t *testing.T, s *Store, id string) (string, error) {
	t.Helper()
	list, err := s.Workspaces(tstCtx(t))
	if err != nil {
		return "", err
	}
	for _, w := range list {
		if w.ID == id {
			return w.Name, nil
		}
	}
	return "", nil
}

// TestCreateWorkspaceDuplicateIsSentinel 钉住重名：大小写不敏感、去空白之后比较，
// 返回可 errors.Is 识别的 ErrWorkspaceNameTaken，且失败时不留下第二行。
func TestCreateWorkspaceDuplicateIsSentinel(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	tstMustCreateWorkspace(t, ctx, s, "Work")

	for _, dup := range []string{"Work", "work", "WORK", "  Work  "} {
		if _, err := s.CreateWorkspace(ctx, dup); !errors.Is(err, ErrWorkspaceNameTaken) {
			t.Errorf("CreateWorkspace(%q) = %v，期望 ErrWorkspaceNameTaken", dup, err)
		}
	}
	if list, err := s.Workspaces(ctx); err != nil || len(list) != 1 {
		t.Errorf("重名失败后工作区数 = %d, err=%v，期望 1", len(list), err)
	}

	other := tstMustCreateWorkspace(t, ctx, s, "另一个")
	if _, err := s.RenameWorkspace(ctx, other.ID, "wOrK"); !errors.Is(err, ErrWorkspaceNameTaken) {
		t.Errorf("改名撞上已存在的名字 = %v，期望 ErrWorkspaceNameTaken", err)
	}
	// 改成自己现在的名字（只改大小写）不算重名。
	if _, err := s.RenameWorkspace(ctx, other.ID, "另一个"); err != nil {
		t.Errorf("改成自己现在的名字 = %v，期望 nil", err)
	}
}

// TestRenameWorkspace 钉住成功路径：只改 name，sort_order / created_at 与
// 成员的归属都不动，返回值带真实的会话数。
func TestRenameWorkspace(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	ws := tstMustCreateWorkspace(t, ctx, s, "旧名字")
	conv := tstNewConv(t, ctx, s, "组里的会话")
	if err := s.SetConversationWorkspace(ctx, conv.ID, &ws.ID); err != nil {
		t.Fatalf("SetConversationWorkspace 失败: %v", err)
	}

	renamed, err := s.RenameWorkspace(ctx, ws.ID, "  新名字  ")
	if err != nil {
		t.Fatalf("RenameWorkspace 失败: %v", err)
	}
	if renamed.ID != ws.ID || renamed.Name != "新名字" {
		t.Errorf("改名返回 = %+v，期望 id=%s name=新名字", renamed, ws.ID)
	}
	if renamed.SortOrder != ws.SortOrder {
		t.Errorf("改名动了 SortOrder: %d -> %d", ws.SortOrder, renamed.SortOrder)
	}
	if !renamed.CreatedAt.Equal(ws.CreatedAt) {
		t.Errorf("改名动了 CreatedAt: %v -> %v", ws.CreatedAt, renamed.CreatedAt)
	}
	if renamed.ConversationCount != 1 {
		t.Errorf("改名返回的 ConversationCount = %d，期望 1", renamed.ConversationCount)
	}

	got, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("改名后 Conversation 失败: %v", err)
	}
	if got.WorkspaceID != ws.ID {
		t.Errorf("改名后会话的归属 = %q，期望 %q（改名不该动归属）", got.WorkspaceID, ws.ID)
	}
	if _, err := s.RenameWorkspace(ctx, "ws-does-not-exist", "任意"); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("给未知工作区改名 = %v，期望 ErrWorkspaceNotFound", err)
	}
}

// TestDeleteWorkspaceKeepsConversations 是本片最硬的一条：删组不删会话。
// 同一事务把 workspace_id 置 NULL；会话、消息、用量一条不少；软删的会话
// 也一并清掉悬空引用；再删一次是 404 哨兵。
func TestDeleteWorkspaceKeepsConversations(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	ws := tstMustCreateWorkspace(t, ctx, s, "要删的组")

	kept := tstNewConv(t, ctx, s, "组里留下的会话")
	tstAppend(t, ctx, s, tstUserMsg(kept.ID, "删组之后还要读得到"))
	if err := tstRecordUsage(t, ctx, s, tstBaseUsage(kept.ID, "alpha", "turn-"+kept.ID)); err != nil {
		t.Fatalf("造用量失败: %v", err)
	}
	gone := tstNewConv(t, ctx, s, "组里已软删的会话")
	for _, id := range []string{kept.ID, gone.ID} {
		if err := s.SetConversationWorkspace(ctx, id, &ws.ID); err != nil {
			t.Fatalf("设置 %s 的归属失败: %v", id, err)
		}
	}
	if err := s.SoftDeleteConversation(ctx, gone.ID); err != nil {
		t.Fatalf("SoftDeleteConversation 失败: %v", err)
	}
	raw := tstRawDB(t, s.Path())

	if err := s.DeleteWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("DeleteWorkspace 失败: %v", err)
	}

	if list, err := s.Workspaces(ctx); err != nil || len(list) != 0 {
		t.Errorf("删组后 Workspaces = %+v, err=%v，期望空", list, err)
	}
	for _, id := range []string{kept.ID, gone.ID} {
		if got := tstWorkspaceIDOf(t, raw, id); got.Valid {
			t.Errorf("删组后 %s 的 workspace_id = %q，期望 NULL", id, got.String)
		}
	}

	one, err := s.Conversation(ctx, kept.ID)
	if err != nil {
		t.Fatalf("删组后 Conversation 失败: %v（会话不该跟着没）", err)
	}
	if one.WorkspaceID != "" || one.Title != kept.Title {
		t.Errorf("删组后会话 = %+v，期望标题原样且未分组", one)
	}
	if msgs, err := s.Messages(ctx, kept.ID, 0, 0); err != nil || len(msgs) != 1 {
		t.Errorf("删组后 Messages = %+v, err=%v，期望那条消息还在", msgs, err)
	}
	if usages, err := s.TurnUsages(ctx, kept.ID, "", -1); err != nil || len(usages) != 1 {
		t.Errorf("删组后 TurnUsages = %+v, err=%v，期望那笔账还在", usages, err)
	}
	// 已软删的会话仍然不可见，但它的行还在库里（只清了归属）。
	if _, err := s.Conversation(ctx, gone.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("删组后已删会话的 Conversation = %v，期望 ErrConversationNotFound", err)
	}
	if got := tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations WHERE id = ?`, gone.ID); got != 1 {
		t.Errorf("已删会话的行数 = %d，期望 1（删组不删行）", got)
	}

	if err := s.DeleteWorkspace(ctx, ws.ID); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("再删一次 = %v，期望 ErrWorkspaceNotFound", err)
	}
	if err := s.DeleteWorkspace(ctx, ""); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("删空 id = %v，期望 ErrWorkspaceNotFound", err)
	}
}

// TestDeleteWorkspaceFailureLeavesConversations 钉住拒绝路径不产生写：
// 删一个不存在的组，不该顺手把任何会话的归属清掉。
func TestDeleteWorkspaceFailureLeavesConversations(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	ws := tstMustCreateWorkspace(t, ctx, s, "还在的组")
	conv := tstNewConv(t, ctx, s, "还有归属的会话")
	if err := s.SetConversationWorkspace(ctx, conv.ID, &ws.ID); err != nil {
		t.Fatalf("SetConversationWorkspace 失败: %v", err)
	}
	raw := tstRawDB(t, s.Path())

	if err := s.DeleteWorkspace(ctx, "ws-does-not-exist"); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("删未知组 = %v，期望 ErrWorkspaceNotFound", err)
	}
	if got := tstWorkspaceIDOf(t, raw, conv.ID); !got.Valid || got.String != ws.ID {
		t.Errorf("被拒的删除之后归属 = %#v，期望仍是 %s", got, ws.ID)
	}
}

// TestSetConversationWorkspace 钉住设置与清除：nil 与空串都表示移出，
// 未知工作区、未知会话、已软删会话都被拒绝，且被拒时不写库。
func TestSetConversationWorkspace(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	ws := tstMustCreateWorkspace(t, ctx, s, "归属测试组")
	conv := tstNewConv(t, ctx, s, "会被移进移出的会话")
	raw := tstRawDB(t, s.Path())

	if got := tstWorkspaceIDOf(t, raw, conv.ID); got.Valid {
		t.Fatalf("新建会话的 workspace_id = %q，期望 NULL", got.String)
	}

	if err := s.SetConversationWorkspace(ctx, conv.ID, &ws.ID); err != nil {
		t.Fatalf("设置归属失败: %v", err)
	}
	if got := tstWorkspaceIDOf(t, raw, conv.ID); !got.Valid || got.String != ws.ID {
		t.Fatalf("设置后 workspace_id = %#v，期望 %s", got, ws.ID)
	}

	empty := ""
	if err := s.SetConversationWorkspace(ctx, conv.ID, &empty); err != nil {
		t.Fatalf("用空串移出失败: %v", err)
	}
	if got := tstWorkspaceIDOf(t, raw, conv.ID); got.Valid {
		t.Errorf("空串移出后 workspace_id = %q，期望 NULL", got.String)
	}

	if err := s.SetConversationWorkspace(ctx, conv.ID, &ws.ID); err != nil {
		t.Fatalf("再次设置归属失败: %v", err)
	}
	if err := s.SetConversationWorkspace(ctx, conv.ID, nil); err != nil {
		t.Fatalf("用 nil 移出失败: %v", err)
	}
	if got := tstWorkspaceIDOf(t, raw, conv.ID); got.Valid {
		t.Errorf("nil 移出后 workspace_id = %q，期望 NULL", got.String)
	}

	unknownWS := "ws-does-not-exist"
	if err := s.SetConversationWorkspace(ctx, conv.ID, &unknownWS); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("指向未知工作区 = %v，期望 ErrWorkspaceNotFound", err)
	}
	if got := tstWorkspaceIDOf(t, raw, conv.ID); got.Valid {
		t.Errorf("被拒的归属之后 workspace_id = %q，期望仍是 NULL", got.String)
	}

	if err := s.SetConversationWorkspace(ctx, "conv-does-not-exist", &ws.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("给未知会话设置归属 = %v，期望 ErrConversationNotFound", err)
	}

	gone := tstNewConv(t, ctx, s, "已删会话")
	if err := s.SoftDeleteConversation(ctx, gone.ID); err != nil {
		t.Fatalf("SoftDeleteConversation 失败: %v", err)
	}
	if err := s.SetConversationWorkspace(ctx, gone.ID, &ws.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Errorf("给已删会话设置归属 = %v，期望 ErrConversationNotFound", err)
	}
	if got := tstWorkspaceIDOf(t, raw, gone.ID); got.Valid {
		t.Errorf("被拒之后已删会话的 workspace_id = %q，期望 NULL", got.String)
	}
}

// TestConversationListsCarryWorkspaceID 钉住归档与工作区正交：
// 默认列表与归档列表都带 workspace_id，改归属不改归档状态，也不把会话
// 从任何一个列表里拿走。
func TestConversationListsCarryWorkspaceID(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	ws := tstMustCreateWorkspace(t, ctx, s, "正交测试组")
	active := tstNewConv(t, ctx, s, "默认列表里的会话")
	archived := tstNewConv(t, ctx, s, "归档列表里的会话")
	ungrouped := tstNewConv(t, ctx, s, "没分组的会话")

	for _, id := range []string{active.ID, archived.ID} {
		if err := s.SetConversationWorkspace(ctx, id, &ws.ID); err != nil {
			t.Fatalf("设置 %s 归属失败: %v", id, err)
		}
	}
	if err := s.ArchiveConversation(ctx, archived.ID); err != nil {
		t.Fatalf("ArchiveConversation 失败: %v", err)
	}

	main, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations 失败: %v", err)
	}
	got := map[string]string{}
	for _, c := range main {
		got[c.ID] = c.WorkspaceID
	}
	if len(main) != 2 {
		t.Fatalf("默认列表有 %d 条，期望 2 条", len(main))
	}
	if got[active.ID] != ws.ID {
		t.Errorf("默认列表里分组会话的 WorkspaceID = %q，期望 %q", got[active.ID], ws.ID)
	}
	if got[ungrouped.ID] != "" {
		t.Errorf("未分组会话的 WorkspaceID = %q，期望空串", got[ungrouped.ID])
	}

	arch, err := s.ArchivedConversations(ctx)
	if err != nil {
		t.Fatalf("ArchivedConversations 失败: %v", err)
	}
	if len(arch) != 1 || arch[0].ID != archived.ID {
		t.Fatalf("归档列表 = %+v，期望只有 %s", arch, archived.ID)
	}
	if arch[0].WorkspaceID != ws.ID {
		t.Errorf("归档列表里会话的 WorkspaceID = %q，期望 %q（归档列表也带归属）", arch[0].WorkspaceID, ws.ID)
	}
	if !arch[0].Archived {
		t.Error("归档列表里的会话 Archived = false")
	}

	// 归档之后再把会话移出分组：归档状态不受影响。
	if err := s.SetConversationWorkspace(ctx, archived.ID, nil); err != nil {
		t.Fatalf("归档后移出分组失败: %v", err)
	}
	arch, err = s.ArchivedConversations(ctx)
	if err != nil || len(arch) != 1 {
		t.Fatalf("移出后归档列表 = %+v, err=%v，期望仍有一条", arch, err)
	}
	if arch[0].WorkspaceID != "" || !arch[0].Archived {
		t.Errorf("移出后归档会话 = %+v，期望未分组且仍归档", arch[0])
	}
}

// TestWorkspaceConversationCount 钉住 Count 的口径：已软删的不计，
// 已归档的照计；删组之后原来的组当然也不再存在。
func TestWorkspaceConversationCount(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	ws := tstMustCreateWorkspace(t, ctx, s, "计数组")
	active := tstNewConv(t, ctx, s, "活跃会话")
	archived := tstNewConv(t, ctx, s, "归档会话")
	deleted := tstNewConv(t, ctx, s, "软删会话")
	for _, id := range []string{active.ID, archived.ID, deleted.ID} {
		if err := s.SetConversationWorkspace(ctx, id, &ws.ID); err != nil {
			t.Fatalf("设置 %s 归属失败: %v", id, err)
		}
	}
	if err := s.ArchiveConversation(ctx, archived.ID); err != nil {
		t.Fatalf("ArchiveConversation 失败: %v", err)
	}
	if err := s.SoftDeleteConversation(ctx, deleted.ID); err != nil {
		t.Fatalf("SoftDeleteConversation 失败: %v", err)
	}

	list, err := s.Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces 失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("工作区数 = %d，期望 1", len(list))
	}
	if list[0].ConversationCount != 2 {
		t.Errorf("ConversationCount = %d，期望 2（已归档的照计、已软删的不计）", list[0].ConversationCount)
	}
}

// TestWorkspacePersistsAcrossReopen 钉住工作区与归属落在库里：关掉再开，
// 名字、顺序、会话数与每个会话的归属都原样。
func TestWorkspacePersistsAcrossReopen(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	ws := tstMustCreateWorkspace(t, ctx, s, "重开后还在的组")
	conv := tstNewConv(t, ctx, s, "重开后还在组里的会话")
	if err := s.SetConversationWorkspace(ctx, conv.ID, &ws.ID); err != nil {
		t.Fatalf("SetConversationWorkspace 失败: %v", err)
	}
	closeFn()

	s2, _ := tstOpenAt(t, dir)
	ctx2 := tstCtx(t)
	list, err := s2.Workspaces(ctx2)
	if err != nil {
		t.Fatalf("重开后 Workspaces 失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != ws.ID || list[0].Name != "重开后还在的组" {
		t.Fatalf("重开后工作区 = %+v，期望仍是 %s", list, ws.ID)
	}
	if list[0].ConversationCount != 1 {
		t.Errorf("重开后 ConversationCount = %d，期望 1", list[0].ConversationCount)
	}
	got, err := s2.Conversation(ctx2, conv.ID)
	if err != nil || got.WorkspaceID != ws.ID {
		t.Errorf("重开后会话归属 = %+v, err=%v，期望 %s", got, ws.ID, ws.ID)
	}
}
