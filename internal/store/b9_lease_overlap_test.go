package store

// B9：租约冲突不能只按 resource_key 全等判。父目录与子目录会互相踩；
// tcp 的 localhost / 127.0.0.1 在规范化之后是同一个键。

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"council/internal/localagent"
)

func b9Claim(t *testing.T, s *Store, raw, owner string, access LeaseAccess) (Lease, error) {
	t.Helper()
	key, err := localagent.NormalizeResource(raw)
	if err != nil {
		t.Fatalf("NormalizeResource(%q): %v", raw, err)
	}
	return s.ClaimLease(tstCtx(t), LeaseRequest{
		ResourceKey: key,
		OwnerKind:   LeaseOwnerCLI,
		OwnerID:     owner,
		Access:      access,
		TTL:         time.Hour,
	})
}

func b9ExpectConflict(t *testing.T, err error) *LeaseConflictError {
	t.Helper()
	var conflict *LeaseConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("期望租约冲突，实际: %v", err)
	}
	return conflict
}

func TestB9LeaseConflictUsesPathOverlap(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "repo")
	child := filepath.Join(parent, "sub")

	t.Run("父目录写挡子目录写", func(t *testing.T) {
		s, closeFn := tstStore(t)
		defer closeFn()
		if _, err := b9Claim(t, s, parent, "parent", LeaseWrite); err != nil {
			t.Fatalf("父目录写租约失败: %v", err)
		}
		conflict := b9ExpectConflict(t, secondErr(b9Claim(t, s, child, "child", LeaseWrite)))
		parentKey, _ := localagent.NormalizeResource(parent)
		if conflict.Holder.ResourceKey != parentKey {
			t.Errorf("冲突持有者 = %q，期望父目录 %q", conflict.Holder.ResourceKey, parentKey)
		}
	})

	t.Run("父目录写挡子目录读", func(t *testing.T) {
		s, closeFn := tstStore(t)
		defer closeFn()
		if _, err := b9Claim(t, s, parent, "parent", LeaseWrite); err != nil {
			t.Fatalf("父目录写租约失败: %v", err)
		}
		b9ExpectConflict(t, secondErr(b9Claim(t, s, child, "child", LeaseRead)))
	})

	t.Run("父目录读与子目录读可以共存", func(t *testing.T) {
		s, closeFn := tstStore(t)
		defer closeFn()
		if _, err := b9Claim(t, s, parent, "parent", LeaseRead); err != nil {
			t.Fatalf("父目录读租约失败: %v", err)
		}
		if _, err := b9Claim(t, s, child, "child", LeaseRead); err != nil {
			t.Fatalf("读与读不应冲突: %v", err)
		}
	})
}

func TestB9TCPAliasesConflictAfterNormalization(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()

	if _, err := b9Claim(t, s, "tcp:localhost:8765", "local", LeaseWrite); err != nil {
		t.Fatalf("localhost 租约失败: %v", err)
	}
	b9ExpectConflict(t, secondErr(b9Claim(t, s, "tcp:127.0.0.1:8765", "loopback", LeaseWrite)))
}

// secondErr 把「一次调用 + error」压成 error，避免每个断言写两行。
func secondErr(_ Lease, err error) error { return err }
