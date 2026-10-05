package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// cwdSpyCall 记下一次 SetConversationCwd 实际收到的参数。
type cwdSpyCall struct {
	convID string
	cwd    string
}

// cwdSpyStore 包住 fakeStore，把写事务里的 Tx 换成 cwdSpyTx。
// 内嵌 *fakeStore 提供 Store 的其余方法；WriteTx 必须盖住，
// 否则 Service 拿到的还是不记账的原 Tx。
type cwdSpyStore struct {
	*fakeStore
	cwdCalls  []cwdSpyCall
	stampErr  error
	createErr error
}

func (s *cwdSpyStore) WriteTx(ctx context.Context, fn func(context.Context, Tx) error) error {
	return s.fakeStore.WriteTx(ctx, func(ctx context.Context, tx Tx) error {
		return fn(ctx, cwdSpyTx{Tx: tx, spy: s})
	})
}

// cwdSpyTx 只拦建会话和盖章。盖章失败要发生在 CreateConversation 之后，
// 才能断言「建失败了就不再盖章」和「盖章失败时错误被包成 create conversation」。
type cwdSpyTx struct {
	Tx
	spy *cwdSpyStore
}

func (t cwdSpyTx) SetConversationCwd(ctx context.Context, convID, cwd string) error {
	t.spy.cwdCalls = append(t.spy.cwdCalls, cwdSpyCall{convID: convID, cwd: cwd})
	if t.spy.stampErr != nil {
		return t.spy.stampErr
	}
	return t.Tx.SetConversationCwd(ctx, convID, cwd)
}

func (t cwdSpyTx) CreateConversation(ctx context.Context, title string) (Conversation, error) {
	if t.spy.createErr != nil {
		return Conversation{}, t.spy.createErr
	}
	return t.Tx.CreateConversation(ctx, title)
}

func TestCwdCreateInStamps(t *testing.T) {
	spy := &cwdSpyStore{fakeStore: newFakeStore()}
	svc := NewService(spy)
	ctx := context.Background()

	got, err := svc.CreateIn(ctx, "标题", "/proj/a")
	if err != nil {
		t.Fatalf("CreateIn 失败: %v", err)
	}
	if got.Cwd != "/proj/a" {
		t.Errorf("返回的 Cwd = %q，期望 %q", got.Cwd, "/proj/a")
	}
	if got.Title != "标题" {
		t.Errorf("返回的 Title = %q，期望 %q", got.Title, "标题")
	}
	if spy.writeTxCalls != 1 {
		t.Errorf("writeTxCalls = %d，期望 1（建会话和盖章应在同一个事务里）", spy.writeTxCalls)
	}
	if len(spy.cwdCalls) != 1 {
		t.Fatalf("cwdCalls 有 %d 条，期望 1 条", len(spy.cwdCalls))
	}
	if spy.cwdCalls[0].convID != got.ID {
		t.Errorf("盖章的 convID = %q，期望返回的 ID %q", spy.cwdCalls[0].convID, got.ID)
	}
	if spy.cwdCalls[0].cwd != "/proj/a" {
		t.Errorf("盖章的 cwd = %q，期望 %q", spy.cwdCalls[0].cwd, "/proj/a")
	}
	if len(spy.convs) != 1 {
		t.Fatalf("fakeStore.convs 有 %d 场，期望 1 场", len(spy.convs))
	}
	if spy.convs[0].Cwd != "/proj/a" {
		t.Errorf("f.convs[0].Cwd = %q，期望 %q", spy.convs[0].Cwd, "/proj/a")
	}
	read, err := svc.Conversation(ctx, got.ID)
	if err != nil {
		t.Fatalf("Conversation 读回失败: %v", err)
	}
	if read.Cwd != "/proj/a" {
		t.Errorf("读回的 Cwd = %q，期望 %q", read.Cwd, "/proj/a")
	}
}

func TestCwdCreateInEmptyDoesNotStamp(t *testing.T) {
	spy := &cwdSpyStore{fakeStore: newFakeStore()}
	svc := NewService(spy)
	ctx := context.Background()

	got, err := svc.CreateIn(ctx, "标题", "")
	if err != nil {
		t.Fatalf("CreateIn 空 cwd 失败: %v", err)
	}
	if got.Cwd != "" {
		t.Errorf("返回的 Cwd = %q，期望空串", got.Cwd)
	}
	if len(spy.cwdCalls) != 0 {
		t.Errorf("cwdCalls 有 %d 条，期望 0 条（空 cwd 不该调 SetConversationCwd）", len(spy.cwdCalls))
	}
	if spy.writeTxCalls != 1 {
		t.Errorf("writeTxCalls = %d，期望 1", spy.writeTxCalls)
	}
}

func TestCwdCreateEqualsCreateInEmpty(t *testing.T) {
	ctx := context.Background()
	createSpy := &cwdSpyStore{fakeStore: newFakeStore()}
	createInSpy := &cwdSpyStore{fakeStore: newFakeStore()}

	viaCreate, err := NewService(createSpy).Create(ctx, "t")
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	viaCreateIn, err := NewService(createInSpy).CreateIn(ctx, "t", "")
	if err != nil {
		t.Fatalf("CreateIn 空 cwd 失败: %v", err)
	}
	if createSpy.writeTxCalls != 1 {
		t.Errorf("Create 的 writeTxCalls = %d，期望 1", createSpy.writeTxCalls)
	}
	if createInSpy.writeTxCalls != 1 {
		t.Errorf("CreateIn 的 writeTxCalls = %d，期望 1", createInSpy.writeTxCalls)
	}
	if len(createSpy.cwdCalls) != 0 {
		t.Errorf("Create 的 cwdCalls 有 %d 条，期望 0 条", len(createSpy.cwdCalls))
	}
	if len(createInSpy.cwdCalls) != 0 {
		t.Errorf("CreateIn 的 cwdCalls 有 %d 条，期望 0 条", len(createInSpy.cwdCalls))
	}
	if viaCreate.Title != viaCreateIn.Title {
		t.Errorf("Title = %q 与 %q，期望两者相同", viaCreate.Title, viaCreateIn.Title)
	}
	if viaCreate.Cwd != viaCreateIn.Cwd {
		t.Errorf("Cwd = %q 与 %q，期望两者都是空串且相同", viaCreate.Cwd, viaCreateIn.Cwd)
	}
	if viaCreate.TeamID != viaCreateIn.TeamID {
		t.Errorf("TeamID = %q 与 %q，期望两者相同", viaCreate.TeamID, viaCreateIn.TeamID)
	}
	if viaCreate.ID != viaCreateIn.ID {
		t.Errorf("ID = %q 与 %q，期望两个独立 fake 从头计数后相同", viaCreate.ID, viaCreateIn.ID)
	}
}

func TestCwdCreateInStampFailure(t *testing.T) {
	sentinel := errors.New("stamp failed")
	spy := &cwdSpyStore{fakeStore: newFakeStore(), stampErr: sentinel}
	got, err := NewService(spy).CreateIn(context.Background(), "标题", "/proj/a")
	if !errors.Is(err, sentinel) {
		t.Fatalf("盖章失败时 errors.Is(哨兵) = false，实际错误: %v", err)
	}
	if got != (Conversation{}) {
		t.Errorf("盖章失败时返回 %+v，期望零值 Conversation", got)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "create conversation:") {
		t.Errorf("错误文本 = %q，期望以 %q 开头", err, "create conversation:")
	}
}

func TestCwdCreateInCreateFailureSkipsStamp(t *testing.T) {
	sentinel := errors.New("create failed")
	spy := &cwdSpyStore{fakeStore: newFakeStore(), createErr: sentinel}
	got, err := NewService(spy).CreateIn(context.Background(), "标题", "/proj/a")
	if !errors.Is(err, sentinel) {
		t.Fatalf("建会话失败时 errors.Is(哨兵) = false，实际错误: %v", err)
	}
	if got != (Conversation{}) {
		t.Errorf("建会话失败时返回 %+v，期望零值 Conversation", got)
	}
	if len(spy.cwdCalls) != 0 {
		t.Errorf("cwdCalls 有 %d 条，期望 0 条（建会话失败不该再盖章）", len(spy.cwdCalls))
	}
}
