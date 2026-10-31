package headless

// P2：git status --porcelain 默认会给空格/引号/非 ASCII 路径加引号，
// 必须改用 -z 并解析 NUL；重命名只取新路径。

import (
	"reflect"
	"testing"
)

func TestParseStatusPathsZ(t *testing.T) {
	out := []byte("M  a b.txt\x00R  new name.txt\x00old name.txt\x00?? ü.txt\x00")
	got := parseStatusPaths(out)
	want := []string{"a b.txt", "new name.txt", "ü.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseStatusPaths = %v，期望 %v", got, want)
	}
}
