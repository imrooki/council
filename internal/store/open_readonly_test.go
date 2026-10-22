package store

// open_readonly_test.go 覆盖 SRV1 §2 与 §6 的 store 侧验收：
// 库不存在时明确报错且不建文件、版本不符时不迁移只报错、普通库读得到既有会话，
// 以及本片的核心——TUI（用 Open 模拟）占着 council.lock 时 OpenReadOnly 仍能读。
// 全部用例只在 t.TempDir() 下建库，与 open_test.go 的纪律一致。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/conversation"
)

// tstReadOnlyAt 打开 dataDir 的只读库，并把关闭登记进 t.Cleanup。
func tstReadOnlyAt(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := OpenReadOnly(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("OpenReadOnly(%q) 返回错误: %v", dir, err)
	}
	if s == nil {
		t.Fatalf("OpenReadOnly(%q) 返回 nil store 且没有报错", dir)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("只读 Store.Close() 返回错误: %v", err)
		}
	})
	return s
}

// TestOpenReadOnlyMissingDatabase 是验收里的「库不存在时退 1 且不建文件」在 store 层的落点。
func TestOpenReadOnlyMissingDatabase(t *testing.T) {
	t.Run("数据目录不存在时报错且不建目录", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "absent", "council-data")
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("前置条件不成立：%q 在调用之前就存在（err=%v）", dir, err)
		}

		_, err := OpenReadOnly(tstCtx(t), dir)
		if err == nil {
			t.Fatal("OpenReadOnly 在数据目录不存在时期望报错，实际返回 nil")
		}
		if !errors.Is(err, ErrReadOnlyDBMissing) {
			t.Errorf("错误链里应当有 ErrReadOnlyDBMissing，实际: %v", err)
		}
		if _, statErr := os.Stat(dir); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("OpenReadOnly 不该创建目录 %q（Stat err=%v）", dir, statErr)
		}
	})

	t.Run("目录在但库文件不存在时报错且不建文件", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, dbFileName)
		if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("前置条件不成立：%q 已经存在（err=%v）", dbPath, err)
		}

		_, err := OpenReadOnly(tstCtx(t), dir)
		if err == nil {
			t.Fatal("OpenReadOnly 在库文件不存在时期望报错，实际返回 nil")
		}
		if !errors.Is(err, ErrReadOnlyDBMissing) {
			t.Errorf("错误链里应当有 ErrReadOnlyDBMissing，实际: %v", err)
		}
		if _, statErr := os.Stat(dbPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("OpenReadOnly 不该创建库文件 %q（Stat err=%v）", dbPath, statErr)
		}
		if _, statErr := os.Stat(filepath.Join(dir, lockFileName)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("OpenReadOnly 不该碰锁文件（Stat err=%v）", statErr)
		}
	})
}

// TestOpenReadOnlySchemaVersionMismatch 覆盖「版本不符也不迁移」。
// 造法一：删掉迁移账本里最高的几行，模拟旧库；造法二：塞一行未来的版本号。
// 两种都必须报 ErrSchemaVersionMismatch，而且库本身一个字节都不该被改回去。
func TestOpenReadOnlySchemaVersionMismatch(t *testing.T) {
	expected := expectedSchemaVersion()
	if expected <= 0 {
		t.Fatalf("本二进制没有可用的 schema 版本: %d", expected)
	}
	older := expected - 1

	openAndMutate := func(t *testing.T, stmt string, args ...any) string {
		t.Helper()
		s, closeFn := tstStore(t)
		dir := filepath.Dir(s.Path())
		dbPath := s.Path()
		closeFn()
		raw := tstRawDB(t, dbPath)
		if _, err := raw.Exec(stmt, args...); err != nil {
			t.Fatalf("制造版本不符失败 (%q): %v", stmt, err)
		}
		return dir
	}

	t.Run("旧库", func(t *testing.T) {
		dir := openAndMutate(t, `DELETE FROM schema_migrations WHERE version >= ?`, expected)
		_, err := OpenReadOnly(tstCtx(t), dir)
		if !errors.Is(err, ErrSchemaVersionMismatch) {
			t.Fatalf("旧库在 OpenReadOnly 下期望 ErrSchemaVersionMismatch，实际: %v", err)
		}
		if !strings.Contains(err.Error(), "旧版本") {
			t.Errorf("旧库的错误应当说明「旧版本」，实际: %v", err)
		}
	})

	t.Run("新库", func(t *testing.T) {
		dir := openAndMutate(t,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, '2026-10-22T00:00:00Z')`,
			expected+1)
		_, err := OpenReadOnly(tstCtx(t), dir)
		if !errors.Is(err, ErrSchemaVersionMismatch) {
			t.Fatalf("新库在 OpenReadOnly 下期望 ErrSchemaVersionMismatch，实际: %v", err)
		}
		if !strings.Contains(err.Error(), "比这个 council 新") {
			t.Errorf("新库的错误应当说明「比这个 council 新」，实际: %v", err)
		}
	})

	// user_version 这条单独造：任务书点名要读它。本项目的正常库永远是 0，
	// 所以非 0 且不等于期望值就必须拦下，哪怕迁移账本看上去还是对的。
	t.Run("user_version 被改成别的版本", func(t *testing.T) {
		dir := openAndMutate(t, fmt.Sprintf(`PRAGMA user_version = %d`, expected+99))
		_, err := OpenReadOnly(tstCtx(t), dir)
		if !errors.Is(err, ErrSchemaVersionMismatch) {
			t.Fatalf("user_version 不符时期望 ErrSchemaVersionMismatch，实际: %v", err)
		}
	})

	t.Run("版本不符时不会把迁移补回去", func(t *testing.T) {
		dir := openAndMutate(t, `DELETE FROM schema_migrations WHERE version >= ?`, expected)
		if _, err := OpenReadOnly(tstCtx(t), dir); !errors.Is(err, ErrSchemaVersionMismatch) {
			t.Fatalf("前置条件不成立：期望版本错误，实际 %v", err)
		}
		raw := tstRawDB(t, filepath.Join(dir, dbFileName))
		if got := tstScanInt(t, raw, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`); got != older {
			t.Errorf("只读打开之后 schema_migrations 的最大版本 = %d，期望仍是 %d（不该被迁移补成 %d）",
				got, older, expected)
		}
		if got := tstScanInt(t, raw, `PRAGMA user_version`); got != 0 {
			t.Errorf("只读打开之后 PRAGMA user_version = %d，期望仍是 0（只读进程不该写库）", got)
		}
	})
}

// TestOpenReadOnlyReadsExistingData 覆盖「普通库 → 能读到既有会话」，
// 四类读方法各走一遍：Conversations、Messages、Participants、TurnUsages。
func TestOpenReadOnlyReadsExistingData(t *testing.T) {
	s, closeFn := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, s, "只读能看到的会话")
	tstJoin(t, ctx, s, conversation.Participant{
		ConvID:      conv.ID,
		AgentID:     "alpha",
		DisplayName: "Alpha",
		Provider:    "claude",
		Active:      true,
	})
	msg := tstAppend(t, ctx, s, tstUserMsg(conv.ID, "只读能看到的正文"))
	if err := tstRecordUsage(t, ctx, s, tstBaseUsage(conv.ID, "alpha", "turn-readonly-1")); err != nil {
		t.Fatalf("写用量失败: %v", err)
	}
	dir := filepath.Dir(s.Path())
	closeFn()

	ro := tstReadOnlyAt(t, dir)

	convs, err := ro.Conversations(ctx)
	if err != nil {
		t.Fatalf("只读 Conversations 失败: %v", err)
	}
	if len(convs) != 1 || convs[0].ID != conv.ID {
		t.Fatalf("只读 Conversations = %+v，期望只有 %q", convs, conv.ID)
	}

	msgs, err := ro.Messages(ctx, conv.ID, 0, 0)
	if err != nil {
		t.Fatalf("只读 Messages 失败: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Seq != msg.Seq || msgs[0].Text != msg.Text {
		t.Fatalf("只读 Messages = %+v，期望读回 %+v", msgs, msg)
	}

	parts, err := ro.Participants(ctx, conv.ID, true)
	if err != nil {
		t.Fatalf("只读 Participants 失败: %v", err)
	}
	if len(parts) != 1 || parts[0].AgentID != "alpha" {
		t.Fatalf("只读 Participants = %+v，期望 alpha", parts)
	}

	usages, err := ro.TurnUsages(ctx, conv.ID, "", -1)
	if err != nil {
		t.Fatalf("只读 TurnUsages 失败: %v", err)
	}
	if len(usages) != 1 || usages[0].TurnID != "turn-readonly-1" {
		t.Fatalf("只读 TurnUsages = %+v，期望 turn-readonly-1", usages)
	}

	ps, ok, err := ro.ProviderSession(ctx, conv.ID, "alpha")
	if err != nil {
		t.Fatalf("只读 ProviderSession 失败: %v", err)
	}
	if ok {
		t.Fatalf("没写过 provider_sessions，却读回了 %+v", ps)
	}
}

// TestOpenReadOnlyWhileLockHeld 是本片的核心验收：先 Open 拿到 council.lock，
// 再 OpenReadOnly，必须成功，而且读得到拿锁那一侧写下的东西。
func TestOpenReadOnlyWhileLockHeld(t *testing.T) {
	writer, closeWriter := tstStore(t)
	ctx := tstCtx(t)
	conv := tstNewConv(t, ctx, writer, "TUI 开着也能看")
	first := tstAppend(t, ctx, writer, tstUserMsg(conv.ID, "锁在写句柄手上"))
	dir := filepath.Dir(writer.Path())

	ro, err := OpenReadOnly(ctx, dir)
	if err != nil {
		t.Fatalf("data_dir 被 Open 占着时 OpenReadOnly 必须成功，实际: %v", err)
	}
	defer func() {
		if err := ro.Close(); err != nil {
			t.Errorf("只读 Store.Close() 返回错误: %v", err)
		}
	}()

	msgs, err := ro.Messages(ctx, conv.ID, 0, 0)
	if err != nil {
		t.Fatalf("看板读 Messages 失败: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Seq != first.Seq {
		t.Fatalf("看板读回 %+v，期望 seq=%d", msgs, first.Seq)
	}

	// 反向也要成立：看板读的同时，TUI 那一侧还能继续写。
	second := tstAppend(t, ctx, writer, tstUserMsg(conv.ID, "看板读着，写句柄照常写"))
	msgs, err = ro.Messages(ctx, conv.ID, first.Seq, 0)
	if err != nil {
		t.Fatalf("看板增量读失败: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Seq != second.Seq {
		t.Fatalf("看板增量读回 %+v，期望 seq=%d", msgs, second.Seq)
	}

	closeWriter()
}

// TestOpenReadOnlyDoesNotHoldLock 从另一面证明不抢锁：只读句柄开着的时候，
// Open 仍然能拿到 council.lock。实现若为了「防第二个进程」顺手加了锁，这里会红。
func TestOpenReadOnlyDoesNotHoldLock(t *testing.T) {
	s, closeFn := tstStore(t)
	dir := filepath.Dir(s.Path())
	closeFn()

	ro := tstReadOnlyAt(t, dir)
	if _, err := ro.Conversations(tstCtx(t)); err != nil {
		t.Fatalf("只读库读不了: %v", err)
	}

	writer, err := Open(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("OpenReadOnly 开着时 Open 必须能拿到锁，实际: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Errorf("写 Store.Close() 返回错误: %v", err)
	}
}

// TestOpenReadOnlyDoesNotMigrate 覆盖「不跑迁移」：只读打开前后迁移账本逐行不变。
func TestOpenReadOnlyDoesNotMigrate(t *testing.T) {
	s, closeFn := tstStore(t)
	dbPath := s.Path()
	dir := filepath.Dir(dbPath)
	before := tstMigrationRows(t, dbPath)
	closeFn()

	ro, err := OpenReadOnly(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("OpenReadOnly 失败: %v", err)
	}
	if err := ro.Close(); err != nil {
		t.Fatalf("只读 Store.Close() 失败: %v", err)
	}

	after := tstMigrationRows(t, dbPath)
	if len(after) != len(before) {
		t.Fatalf("只读打开之后迁移账本有 %d 行，期望仍是 %d 行: %+v", len(after), len(before), after)
	}
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("只读打开之后第 %d 行迁移记录从 %+v 变成 %+v", i, before[i], after[i])
		}
	}
}

// TestOpenReadOnlyEmptySlices 钉住空库的读回形状：空切片，不是 nil，也不是错误。
func TestOpenReadOnlyEmptySlices(t *testing.T) {
	dir := tstDataDir(t)
	writer, closeFn := tstOpenAt(t, dir)
	conv := tstNewConv(t, tstCtx(t), writer, "空会话")
	closeFn()

	ro := tstReadOnlyAt(t, dir)
	ctx := tstCtx(t)

	msgs, err := ro.Messages(ctx, conv.ID, 0, 0)
	if err != nil {
		t.Fatalf("Messages 失败: %v", err)
	}
	if msgs == nil {
		t.Error("空会话的 Messages 返回 nil，期望空切片")
	}
	parts, err := ro.Participants(ctx, conv.ID, true)
	if err != nil {
		t.Fatalf("Participants 失败: %v", err)
	}
	if parts == nil {
		t.Error("没有成员的 Participants 返回 nil，期望空切片")
	}
	usages, err := ro.TurnUsages(ctx, conv.ID, "", -1)
	if err != nil {
		t.Fatalf("TurnUsages 失败: %v", err)
	}
	if usages == nil {
		t.Error("没有账的 TurnUsages 返回 nil，期望空切片")
	}
}
