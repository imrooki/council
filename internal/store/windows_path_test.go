//go:build windows

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// P2：`\\?\` 长路径与 UNC 不能被拼成 SQLite 的 file: URI authority。
func TestOpenExtendedLengthDataDir(t *testing.T) {
	dir := t.TempDir()
	ext := `\\?\` + dir
	ctx := tstCtx(t)
	s, err := Open(ctx, ext)
	if err != nil {
		t.Fatalf("Open(%q): %v", ext, err)
	}
	if _, err := s.Conversations(ctx); err != nil {
		t.Errorf("打开后读取失败: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ext, dbFileName)); err != nil {
		t.Errorf("库文件没有建在扩展路径下: %v", err)
	}
}
