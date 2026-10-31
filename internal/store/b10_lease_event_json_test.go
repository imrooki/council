package store

// B10：租约事件 detail 是 JSON 文本，不能用 Go 的 %q 拼。%q 会把 NUL 等
// 控制字符写成 \x00，那不是合法 JSON，读事件的调用方会解析失败。

import (
	"encoding/json"
	"testing"
	"time"
)

func TestB10LeaseEventDetailIsValidJSON(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	lease, err := s.ClaimLease(ctx, LeaseRequest{
		ResourceKey: "repo:b10",
		OwnerKind:   LeaseOwnerCLI,
		OwnerID:     "owner-\x00-\x1f-\t",
		Access:      LeaseWrite,
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("ClaimLease 失败: %v", err)
	}
	events, err := s.LeaseEvents(ctx, lease.ID)
	if err != nil {
		t.Fatalf("LeaseEvents 失败: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("没有租约事件")
	}
	for _, event := range events {
		if !json.Valid([]byte(event.Detail)) {
			t.Errorf("事件 %s 的 detail 不是合法 JSON（B10）: %q", event.Kind, event.Detail)
		}
	}
}
