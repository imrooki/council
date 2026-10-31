package turn

// P2：unpackFingerprint 的 start+n 在巨大长度上会溢出成负数，随后切片 panic。

import (
	"fmt"
	"math"
	"testing"
)

func TestUnpackFingerprintHugeLengthDoesNotPanic(t *testing.T) {
	bad := fmt.Sprintf("%d:x", math.MaxInt)
	parts, ok := unpackFingerprint(bad)
	if ok || parts != nil {
		t.Fatalf("畸形指纹应 fail-closed，实际 parts=%v ok=%v", parts, ok)
	}
}
