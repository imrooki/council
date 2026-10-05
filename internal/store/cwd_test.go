// 本文件覆盖 conversations.cwd 的读写：SetConversationCwd、两条读路径，
// 以及 Service.CreateIn 在真库上的盖章与回滚。
package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"council/internal/conversation"
)

// errTstCwdInjected 是盖章失败装饰器故意返回的哨兵。
// 用独立错误而不是复用库里的哨兵，才能分清「注入点被打到了」和「会话本来就不存在」。
var errTstCwdInjected = errors.New("tst cwd injected stamp failure")

// tstCwdFailStore 包住真 Store，只把写事务里的 Tx 换成会让盖章失败的那一层。
// 读方法仍走内嵌 Store：回滚断言要读的是真库，不是另一份内存。
type tstCwdFailStore struct{ *Store }

func (w tstCwdFailStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return w.Store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return fn(ctx, tstCwdFailTx{Tx: tx})
	})
}

// tstCwdFailTx 只拦 SetConversationCwd。其它写仍落到真事务上，
// 这样 CreateConversation 真的插了行，盖章失败时才能看见整笔回滚。
type tstCwdFailTx struct{ conversation.Tx }

func (t tstCwdFailTx) SetConversationCwd(ctx context.Context, convID, cwd string) error {
	return errTstCwdInjected
}

func tstCwdSet(t *testing.T, ctx context.Context, s *Store, id, cwd string) {
	t.Helper()
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetConversationCwd(ctx, id, cwd)
	})
	if err != nil {
		t.Fatalf("SetConversationCwd(%q, %q) 失败: %v", id, cwd, err)
	}
}

func tstCwdInList(t *testing.T, list []conversation.Conversation, id string) conversation.Conversation {
	t.Helper()
	for _, c := range list {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("Conversations 里没有 id=%s，实际有 %d 场", id, len(list))
	return conversation.Conversation{}
}

func tstCwdConvCount(t *testing.T, s *Store) int64 {
	t.Helper()
	raw := tstRawDB(t, s.Path())
	return tstScanInt(t, raw, `SELECT COUNT(*) FROM conversations`)
}

// TestCwdSetAndReadBack 钉住设值、覆盖、清空，以及 Title / TeamID / CreatedAt 不被连带改掉。
func TestCwdSetAndReadBack(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "保持标题")

	got, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("读初始会话失败: %v", err)
	}
	if got.Cwd != "" {
		t.Errorf("初始 Cwd = %q，期望空串（新建会话没绑项目）", got.Cwd)
	}

	err = s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetConversationTeam(ctx, conv.ID, "team-1")
	})
	if err != nil {
		t.Fatalf("SetConversationTeam 失败: %v", err)
	}

	const first = `D:\项目 目录\sub dir`
	tstCwdSet(t, ctx, s, conv.ID, first)
	tstCwdAssertSame(t, ctx, s, conv, "team-1", first)

	const second = `E:\另一处\dir`
	tstCwdSet(t, ctx, s, conv.ID, second)
	tstCwdAssertSame(t, ctx, s, conv, "team-1", second)

	tstCwdSet(t, ctx, s, conv.ID, "")
	tstCwdAssertSame(t, ctx, s, conv, "team-1", "")
}

// tstCwdAssertSame 同时走单条读和列表读。两条路径各写一份 SELECT，
// 只断言其中一条会漏掉另一条忘了带 cwd 列。
func tstCwdAssertSame(t *testing.T, ctx context.Context, s *Store, conv conversation.Conversation, teamID, cwd string) {
	t.Helper()
	one, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Conversation(%s) 失败: %v", conv.ID, err)
	}
	list, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations 失败: %v", err)
	}
	fromList := tstCwdInList(t, list, conv.ID)
	for _, item := range []struct {
		label string
		got   conversation.Conversation
	}{
		{"Conversation", one},
		{"Conversations", fromList},
	} {
		if item.got.Cwd != cwd {
			t.Errorf("%s 的 Cwd = %q，期望 %q", item.label, item.got.Cwd, cwd)
		}
		if item.got.Title != conv.Title {
			t.Errorf("%s 的 Title = %q，期望 %q（设 cwd 不该改标题）", item.label, item.got.Title, conv.Title)
		}
		if item.got.TeamID != teamID {
			t.Errorf("%s 的 TeamID = %q，期望 %q（设 cwd 不该改 team）", item.label, item.got.TeamID, teamID)
		}
		if !item.got.CreatedAt.Equal(conv.CreatedAt) {
			t.Errorf("%s 的 CreatedAt = %s，期望 %s（设 cwd 不该改创建时间）", item.label, item.got.CreatedAt, conv.CreatedAt)
		}
	}
}

func TestCwdSetUnknownConversation(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	before := tstCwdConvCount(t, s)

	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetConversationCwd(ctx, "conv-does-not-exist", `D:\nope`)
	})
	if !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Fatalf("对不存在的会话设 cwd：errors.Is(ErrConversationNotFound) = false，实际错误: %v", err)
	}
	after := tstCwdConvCount(t, s)
	if after != before {
		t.Errorf("conversations 行数从 %d 变成 %d，期望不变（不存在的 id 不该插入行）", before, after)
	}
}

func TestCwdSetEmptyOnFreshConversation(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "从没设过")

	tstCwdSet(t, ctx, s, conv.ID, "")
	got, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("设空串后读回失败: %v", err)
	}
	if got.Cwd != "" {
		t.Errorf("从没设过的会话再设空串，Cwd = %q，期望空串", got.Cwd)
	}
}

func TestCwdSetOnlyTouchesTargetAndKeepsOrder(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	for _, title := range []string{"甲", "乙", "丙"} {
		tstNewConv(t, ctx, s, title)
	}
	before, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("设 cwd 前 Conversations 失败: %v", err)
	}
	if len(before) != 3 {
		t.Fatalf("Conversations 返回 %d 场，期望 3 场", len(before))
	}
	ids := []string{before[0].ID, before[1].ID, before[2].ID}
	const stamped = `D:\只改中间`
	tstCwdSet(t, ctx, s, ids[1], stamped)

	after, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("设 cwd 后 Conversations 失败: %v", err)
	}
	if len(after) != 3 {
		t.Fatalf("设 cwd 后 Conversations 返回 %d 场，期望 3 场", len(after))
	}
	for i := range ids {
		if after[i].ID != ids[i] {
			t.Errorf("列表第 %d 个 ID = %s，期望 %s（只改 cwd 不该改排序）", i, after[i].ID, ids[i])
		}
	}
	if after[1].Cwd != stamped {
		t.Errorf("中间会话 Cwd = %q，期望 %q", after[1].Cwd, stamped)
	}
	if after[0].Cwd != "" {
		t.Errorf("第一场 Cwd = %q，期望空串（不该被连带改）", after[0].Cwd)
	}
	if after[2].Cwd != "" {
		t.Errorf("第三场 Cwd = %q，期望空串（不该被连带改）", after[2].Cwd)
	}
}

// TestCwdReadsColumnDirectly 绕开 Tx，直接改列。
// 读路径若没把 cwd 放进 SELECT，这里会读回空串，而写路径的测试仍是绿的。
func TestCwdReadsColumnDirectly(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "直改列")
	const direct = `C:\绕开写路径\dir`

	raw := tstRawDB(t, s.Path())
	res, err := raw.ExecContext(ctx, `UPDATE conversations SET cwd = ? WHERE id = ?`, direct, conv.ID)
	if err != nil {
		t.Fatalf("直接 UPDATE cwd 失败: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("取 RowsAffected 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("直接 UPDATE 影响 %d 行，期望 1 行", n)
	}

	one, err := s.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Conversation 失败: %v", err)
	}
	if one.Cwd != direct {
		t.Errorf("Conversation.Cwd = %q，期望 %q（读路径应直接读列，不依赖写方法）", one.Cwd, direct)
	}
	list, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations 失败: %v", err)
	}
	fromList := tstCwdInList(t, list, conv.ID)
	if fromList.Cwd != direct {
		t.Errorf("Conversations 里的 Cwd = %q，期望 %q", fromList.Cwd, direct)
	}
}

func TestCwdServiceCreateIn(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(s)
	const dir = `D:\项目 目录\sub dir`

	made, err := svc.CreateIn(ctx, "带目录", dir)
	if err != nil {
		t.Fatalf("CreateIn 非空 cwd 失败: %v", err)
	}
	if made.ID == "" {
		t.Error("CreateIn 返回的 ID 为空")
	}
	if made.Title != "带目录" {
		t.Errorf("返回的 Title = %q，期望 %q", made.Title, "带目录")
	}
	if made.CreatedAt.IsZero() {
		t.Error("返回的 CreatedAt 是零值，期望 store 填上创建时间")
	}
	if made.Cwd != dir {
		t.Errorf("返回的 Cwd = %q，期望 %q", made.Cwd, dir)
	}
	one, err := s.Conversation(ctx, made.ID)
	if err != nil {
		t.Fatalf("读回 CreateIn 的会话失败: %v", err)
	}
	if one.Cwd != dir {
		t.Errorf("Conversation.Cwd = %q，期望 %q", one.Cwd, dir)
	}
	list, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations 失败: %v", err)
	}
	if fromList := tstCwdInList(t, list, made.ID); fromList.Cwd != dir {
		t.Errorf("Conversations 里的 Cwd = %q，期望 %q", fromList.Cwd, dir)
	}

	empty, err := svc.CreateIn(ctx, "不带目录", "")
	if err != nil {
		t.Fatalf("CreateIn 空 cwd 失败: %v", err)
	}
	if empty.Cwd != "" {
		t.Errorf("空 cwd 的 CreateIn 返回 Cwd = %q，期望空串", empty.Cwd)
	}
	emptyRead, err := s.Conversation(ctx, empty.ID)
	if err != nil {
		t.Fatalf("读回空 cwd 会话失败: %v", err)
	}
	if emptyRead.Cwd != "" {
		t.Errorf("空 cwd 读回 Cwd = %q，期望空串", emptyRead.Cwd)
	}

	const title = "同形"
	viaCreate, err := svc.Create(ctx, title)
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	viaCreateIn, err := svc.CreateIn(ctx, title, "")
	if err != nil {
		t.Fatalf("CreateIn 空串失败: %v", err)
	}
	tstCwdAssertCreatedShape(t, ctx, s, viaCreate, title)
	tstCwdAssertCreatedShape(t, ctx, s, viaCreateIn, title)
}

func tstCwdAssertCreatedShape(t *testing.T, ctx context.Context, s *Store, made conversation.Conversation, title string) {
	t.Helper()
	if made.Cwd != "" {
		t.Errorf("返回的 Cwd = %q，期望空串", made.Cwd)
	}
	if made.TeamID != "" {
		t.Errorf("返回的 TeamID = %q，期望空串", made.TeamID)
	}
	if made.Title != title {
		t.Errorf("返回的 Title = %q，期望 %q", made.Title, title)
	}
	got, err := s.Conversation(ctx, made.ID)
	if err != nil {
		t.Fatalf("读回 %s 失败: %v", made.ID, err)
	}
	if got.Cwd != made.Cwd || got.TeamID != made.TeamID || got.Title != made.Title {
		t.Errorf("读回 = Title %q TeamID %q Cwd %q，期望与返回值 Title %q TeamID %q Cwd %q 一致",
			got.Title, got.TeamID, got.Cwd, made.Title, made.TeamID, made.Cwd)
	}
}

func TestCwdServiceCreateInRollsBackWhenStampFails(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	svc := conversation.NewService(tstCwdFailStore{s})

	got, err := svc.CreateIn(ctx, "会被回滚", `D:\x`)
	if !errors.Is(err, errTstCwdInjected) {
		t.Fatalf("盖章失败时 errors.Is(哨兵) = false，实际错误: %v", err)
	}
	if got != (conversation.Conversation{}) {
		t.Errorf("盖章失败时返回 %+v，期望零值 Conversation", got)
	}
	list, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("回滚后 Conversations 失败: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("回滚后 Conversations 有 %d 场，期望 0 场（不该留下没盖章的会话）", len(list))
	}
	if n := tstCwdConvCount(t, s); n != 0 {
		t.Errorf("回滚后 conversations 行数 = %d，期望 0", n)
	}

	// 空 cwd 不调 SetConversationCwd，装饰器拦不住这条路。
	// 它若也失败，上面的失败就不能说明是注入点打中的。
	okConv, err := svc.CreateIn(ctx, "空目录不盖章", "")
	if err != nil {
		t.Fatalf("空 cwd 的 CreateIn 不该走到注入的盖章失败，实际: %v", err)
	}
	if okConv.ID == "" {
		t.Error("空 cwd 的 CreateIn 返回的 ID 为空")
	}
	if n := tstCwdConvCount(t, s); n != 1 {
		t.Errorf("空 cwd 成功后 conversations 行数 = %d，期望 1", n)
	}
}

func TestCwdSchemaMismatchMessageDoesNotAdviseDeletion(t *testing.T) {
	err := checkTableShape("conversations", v5Shape["conversations"], v6Shape["conversations"])
	if err == nil {
		t.Fatal("旧形状对照多出 cwd 的 conversations 应当报错，实际 nil")
	}
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("errors.Is(ErrSchemaMismatch) = false，实际错误: %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"conversations", "多出列 cwd", "不要删库", "-wal", "-shm"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误文本不含 %q；实际: %s", want, msg)
		}
	}
	if strings.Contains(msg, "删掉重建") {
		t.Errorf("错误文本不该含 %q；实际: %s", "删掉重建", msg)
	}
	if err := checkTableShape("conversations", v6Shape["conversations"], v6Shape["conversations"]); err != nil {
		t.Errorf("同一份 v6 形状对照应返回 nil，实际: %v", err)
	}
}

func TestCwdOpenV6DatabaseWithOlderBinaryIsRejected(t *testing.T) {
	dir := tstDataDir(t)
	s, closeFn := tstOpenAt(t, dir)
	ctx := tstCtx(t)
	const (
		title = "旧二进制别删我"
		cwd   = `D:\项目 目录\sub dir`
	)
	made, err := conversation.NewService(s).CreateIn(ctx, title, cwd)
	if err != nil {
		t.Fatalf("CreateIn 失败: %v", err)
	}
	id := made.ID
	closeFn()

	// 本包没有 t.Parallel，短时间换掉包级 migrations 是安全的。
	// migrations 的最后一项决定 validateSchema 用哪份形状：只留到 v5，
	// 就是旧二进制。库里版本已是 6，迁移不会再跑。
	orig := migrations
	migrations = migrations[:5]
	t.Cleanup(func() { migrations = orig })

	opened, err := Open(tstCtx(t), dir)
	if opened != nil {
		if cerr := opened.Close(); cerr != nil {
			t.Errorf("失败的 Open 仍返回了 store，Close 出错: %v", cerr)
		}
	}
	if err == nil {
		t.Fatal("旧二进制打开 v6 库应当失败，实际成功")
	}
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("errors.Is(ErrSchemaMismatch) = false，实际错误: %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"多出列 cwd", "不要删库"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误文本不含 %q；实际: %s", want, msg)
		}
	}
	if strings.Contains(msg, "删掉重建") {
		t.Errorf("错误文本不该含 %q；实际: %s", "删掉重建", msg)
	}

	// 重开之前显式还原：下面的 Open 必须按当前二进制的形状校验。
	migrations = orig
	reopened, _ := tstOpenAt(t, dir)
	got, err := reopened.Conversation(tstCtx(t), id)
	if err != nil {
		t.Fatalf("重开后读回 %s 失败: %v", id, err)
	}
	if got.Title != title {
		t.Errorf("重开后 Title = %q，期望 %q", got.Title, title)
	}
	if got.Cwd != cwd {
		t.Errorf("重开后 Cwd = %q，期望 %q", got.Cwd, cwd)
	}
}
