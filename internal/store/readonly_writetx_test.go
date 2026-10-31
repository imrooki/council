package store

// P2：只读 Store 的 WriteTx 不能 nil panic，要返回可识别的错误。

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"council/internal/conversation"
)

func TestReadOnlyStoreWriteTxReturnsError(t *testing.T) {
	s, closeFn := tstStore(t)
	dir := filepath.Dir(s.Path())
	closeFn()

	ro, err := OpenReadOnly(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()
	err = ro.WriteTx(context.Background(), func(context.Context, conversation.Tx) error {
		t.Fatal("只读 Store 的 WriteTx 不应执行回调")
		return nil
	})
	if !errors.Is(err, ErrReadOnlyStore) {
		t.Fatalf("WriteTx 错误 = %v，期望 ErrReadOnlyStore", err)
	}
}
