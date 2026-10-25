package serve

// fs_test.go 覆盖工作目录选择器的后端：只列目录、只读模式 405、路径不合法
// 回 400。前端点击（进目录、上一级、选择此文件夹）由页面自己驱动，这里只钉
// 接口形状。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFSDirsRequiresWriteMode(t *testing.T) {
	env := newWriteTestEnv(t)
	ro := NewServer(env.store, "test-version", nil)

	rec := httptest.NewRecorder()
	ro.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fs/dirs", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式 GET /api/fs/dirs = %d，期望 405；body=%s", rec.Code, rec.Body.String())
	}
}

func TestFSDirsListsSubdirectories(t *testing.T) {
	env := newWriteTestEnv(t)
	root := t.TempDir()
	for _, name := range []string{"beta", "Alpha"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("建测试目录失败: %v", err)
		}
	}
	// 文件不该出现在列表里：选择器只选目录。
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("建测试文件失败: %v", err)
	}

	res, err := http.Get(env.http.URL + "/api/fs/dirs?path=" + urlQueryEscape(root))
	if err != nil {
		t.Fatalf("GET /api/fs/dirs 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d，期望 200", res.StatusCode)
	}
	var body struct {
		Path    string `json:"path"`
		Parent  string `json:"parent"`
		Entries []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if body.Path != filepath.Clean(root) {
		t.Errorf("path = %q，期望 %q", body.Path, filepath.Clean(root))
	}
	if body.Parent == "" {
		t.Errorf("parent 为空：%q 不是根目录", root)
	}
	if len(body.Entries) != 2 {
		t.Fatalf("entries = %+v，期望恰好两个目录（不含文件）", body.Entries)
	}
	if body.Entries[0].Name != "Alpha" || body.Entries[1].Name != "beta" {
		t.Errorf("目录顺序 = %v，期望按名字不区分大小写排序", []string{body.Entries[0].Name, body.Entries[1].Name})
	}
}

func TestFSDirsRejectsNonDirectory(t *testing.T) {
	env := newWriteTestEnv(t)
	root := t.TempDir()
	file := filepath.Join(root, "note.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("建测试文件失败: %v", err)
	}

	res, err := http.Get(env.http.URL + "/api/fs/dirs?path=" + urlQueryEscape(file))
	if err != nil {
		t.Fatalf("GET /api/fs/dirs 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d，期望 400", res.StatusCode)
	}
}

// urlQueryEscape 是给路径用的最小转义：Windows 路径里有反斜杠与冒号，
// 直接拼进 query 会被解析坏。
func urlQueryEscape(path string) string {
	out := make([]byte, 0, len(path))
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '/':
			out = append(out, c)
		default:
			out = append(out, '%', hexDigit(c>>4), hexDigit(c&0x0f))
		}
	}
	return string(out)
}

func hexDigit(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'A' + (v - 10)
}
