package store

// P2：时间列按 TEXT 排序时，RFC3339Nano 去掉尾零会让 ".1Z" 排在 ".1001Z"
// 之后；按 julianday 排序才是真实时间顺序。

import (
	"testing"
)

func TestTimeTextOrderUsesInstant(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)
	raw := tstRawDB(t, s.Path())
	earlier := "2026-01-01T00:00:00.1Z"
	later := "2026-01-01T00:00:00.19Z"
	if _, err := raw.Exec(`INSERT INTO conversations (id, title, created_at) VALUES
		('a', 'earlier', ?), ('b', 'later', ?)`, earlier, later); err != nil {
		t.Fatalf("插会话: %v", err)
	}

	list, err := s.Conversations(ctx)
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("会话数 = %d，期望 2", len(list))
	}
	if list[0].ID != "b" {
		t.Fatalf("排序 = %q 在前，期望时间更晚的 b（TEXT 排序会错成 a）", list[0].ID)
	}
}
