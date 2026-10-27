package store

// open_path_test.go 钉住 DSN 的路径编码。
//
// 起因是一个真实失败：`go test` 的子测试名里带 `#` 时，临时 data_dir 里也就
// 带了 `#`，于是 `file:C:\...\with#hash\council.db?params` 被 SQLite 当成
// 「`#` 之前是路径、之后是片段」，库被**静默**建到截断后的路径上，而
// council.lock 仍然锁在真正的 data_dir 里。两个共前缀的 data_dir 会因此
// 静默共用一个库 —— 与「一个 data_dir 只能一个写进程」那把锁的初衷相反。
//
// 生产路径上 data_dir 可配置（配置键 / COUNCIL_DATA_DIR），所以这不是测试
// 环境的特例。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tstSpecialDataDir 造一个名字里带 # 与 % 的 data_dir。
//
// 不带 ?：Windows 文件名不允许问号，造不出来也验证不到。百分号与井号都是
// 合法文件名字符，而它们在 SQLite 的 URI 里有特殊含义。
func tstSpecialDataDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "with#hash", "and%pct", "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建特殊字符目录失败: %v", err)
	}
	return dir
}

// TestOpenDataDirWithSpecialCharsKeepsDBInside 钉住库文件落在 data_dir 里。
func TestOpenDataDirWithSpecialCharsKeepsDBInside(t *testing.T) {
	dir := tstSpecialDataDir(t)
	ctx := tstCtx(t)

	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("打开带 # 的 data_dir 失败: %v", err)
	}
	conv := tstNewConv(t, ctx, s, "特殊路径的会话")
	if err := s.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	dbPath := filepath.Join(dir, dbFileName)
	if _, err := os.Stat(dbPath); err != nil {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			t.Logf("data_dir 里实际有: %s", entry.Name())
		}
		t.Fatalf("council.db 没有落在 data_dir 里: %v", err)
	}

	// 父目录里不该出现截断名的库（with、with-wal、with-shm …）：
	// 那正是旧实现把库建错地方时留下的痕迹。
	parent := filepath.Dir(filepath.Dir(dir)) // <tmp>/with#hash
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("读父目录失败: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "and%pct" {
			t.Errorf("父目录里出现了多余条目 %q：库可能被建到了截断路径", entry.Name())
		}
	}

	// 重开同一个目录，数据还在（说明两次都定位到了同一份文件）。
	reopened, closeFn := tstOpenAt(t, dir)
	defer closeFn()
	got, err := reopened.Conversation(tstCtx(t), conv.ID)
	if err != nil {
		t.Fatalf("重开后读会话失败: %v", err)
	}
	if got.Title != "特殊路径的会话" {
		t.Errorf("重开后标题 = %q", got.Title)
	}
}

// TestOpenReadOnlyDataDirWithSpecialChars 钉住只读入口用同一套编码。
func TestOpenReadOnlyDataDirWithSpecialChars(t *testing.T) {
	dir := tstSpecialDataDir(t)
	ctx := tstCtx(t)

	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	conv := tstNewConv(t, ctx, s, "只读也要能找到它")
	if err := s.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	ro, err := OpenReadOnly(ctx, dir)
	if err != nil {
		t.Fatalf("只读打开带 # 的 data_dir 失败: %v", err)
	}
	defer func() {
		if err := ro.Close(); err != nil {
			t.Errorf("关闭只读库失败: %v", err)
		}
	}()
	got, err := ro.Conversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("只读读会话失败: %v", err)
	}
	if got.Title != "只读也要能找到它" {
		t.Errorf("只读读到的标题 = %q", got.Title)
	}
}

// TestSQLiteURIPathEscapes 是纯函数用例：顺序与幂等性比「看起来对」重要
// —— 先转 % 再转 #/?，否则 %23 会被二次转义成 %2523。
func TestSQLiteURIPathEscapes(t *testing.T) {
	cases := map[string]string{
		`C:\plain\path`: "C:/plain/path",
		`C:\a#b\c`:      "C:/a%23b/c",
		`C:\a%23b\c`:    "C:/a%2523b/c",
		`C:\a?b\c`:      "C:/a%3Fb/c",
		`C:\a%b#c?d\e`:  "C:/a%25b%23c%3Fd/e",
		`/home/me/a#b`:  "/home/me/a%23b",
	}
	for in, want := range cases {
		if got := sqliteURIPath(in); got != want {
			t.Errorf("sqliteURIPath(%q) = %q，期望 %q", in, got, want)
		}
	}
	if strings.Contains(sqliteURIPath(`C:\a#b`), "#") {
		t.Error("输出里还留着 #：SQLite 会把它当片段起点")
	}
}
