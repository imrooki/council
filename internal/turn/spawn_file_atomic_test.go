package turn

// P2：身份文件用临时文件 + Rename，读到的永远是全文，不能是半份。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSpawnFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "council-role.txt")
	first := []byte("first-role")
	if err := writeSpawnFile(path, first); err != nil {
		t.Fatalf("writeSpawnFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(first) {
		t.Fatalf("读取 = %q, err=%v", got, err)
	}

	large := make([]byte, 256*1024)
	for i := range large {
		large[i] = 'x'
	}
	if err := writeSpawnFile(path, large); err != nil {
		t.Fatalf("覆盖写: %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil || len(got) != len(large) {
		t.Fatalf("覆盖后长度 = %d, err=%v", len(got), err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(path) {
			t.Errorf("临时文件没有清掉: %s", entry.Name())
		}
	}
}
