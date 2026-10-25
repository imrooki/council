package serve

// profile_cleanup_test.go 覆盖 B5 的清理逻辑：只清过期条目、保留目录本身、
// 目录不存在不算错、极短阈值能全清，以及符号链接指向外部时不跟随删除。
// 时间用 os.Chtimes 回拨，不靠真实等待；只有两处 2ms 的 sleep，用来绕开
// Windows 文件时间戳的时钟粒度，具体见对应用例的注释。

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newProfileDir 造一个 <data_dir>\ui-profile，两个路径都返回：需要断言
// 「目录本身还在」或用启动薄壳的用例会用到 dataDir。
func newProfileDir(t *testing.T) (dataDir, profile string) {
	t.Helper()
	dataDir = t.TempDir()
	profile = filepath.Join(dataDir, appProfileDirName)
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatalf("建 profile 目录失败: %v", err)
	}
	return dataDir, profile
}

// writeProfileFile 写一个文件并自动建父目录，用来造「旧目录里套文件」。
func writeProfileFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建父目录失败 (%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败 (%s): %v", path, err)
	}
}

// backdateProfilePath 把路径的 mtime 回拨 age，用来造「过期」与「未过期」。
func backdateProfilePath(t *testing.T, path string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("回拨 %s 的修改时间失败: %v", path, err)
	}
}

// TestCleanUIOProfileRemovesOnlyExpired 是 B5 的主用例：8 天前的子树整棵
// 删掉，1 天前的文件与刚写的文件原样保留，ui-profile 目录本身还在。
// 「目录还在」这条尤其重要——serve 退出后应用窗口可能仍开着。
func TestCleanUIOProfileRemovesOnlyExpired(t *testing.T) {
	_, profile := newProfileDir(t)

	// 8 天前的目录与其内部的文件：整棵都该没。
	oldDir := filepath.Join(profile, "Default")
	oldFile := filepath.Join(oldDir, "History")
	writeProfileFile(t, oldFile, "old")
	backdateProfilePath(t, oldFile, 8*24*time.Hour)
	backdateProfilePath(t, oldDir, 8*24*time.Hour)

	// 1 天前：没过 7 天，必须保留。
	dayOld := filepath.Join(profile, "Preferences")
	writeProfileFile(t, dayOld, "recent")
	backdateProfilePath(t, dayOld, 24*time.Hour)

	// 刚写：mtime 是新的，同样保留。
	fresh := filepath.Join(profile, "Local State")
	writeProfileFile(t, fresh, "fresh")

	removed, err := cleanUIOProfile(profile, appProfileMaxAge)
	if err != nil {
		t.Fatalf("cleanUIOProfile 返回错误: %v", err)
	}
	if removed != 1 {
		t.Errorf("删掉的顶层条目数 = %d，期望 1（只有 8 天前那棵树）", removed)
	}

	if _, err := os.Stat(oldDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("8 天前的目录应当被整棵删掉，Stat 结果 err=%v", err)
	}
	if _, err := os.Stat(oldFile); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("8 天前目录里的文件应当一并删掉，Stat 结果 err=%v", err)
	}

	// 未过期的两个文件必须还在，内容也没被动过。
	for _, tc := range []struct{ path, content string }{
		{dayOld, "recent"},
		{fresh, "fresh"},
	} {
		got, err := os.ReadFile(tc.path)
		if err != nil {
			t.Errorf("未过期的 %s 被删了: %v", tc.path, err)
			continue
		}
		if string(got) != tc.content {
			t.Errorf("%s 的内容 = %q，期望 %q", tc.path, got, tc.content)
		}
	}

	info, err := os.Stat(profile)
	if err != nil {
		t.Fatalf("profile 目录本身必须保留: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("profile 路径还在，但已经不是目录: mode=%v", info.Mode())
	}
}

// TestCleanUIOProfileMissingDirIsNotAnError 钉住「第一次 -app 之前没有这个
// 目录」不算失败：返回 0 且 err 为 nil，启动薄壳因此不会打印任何东西。
func TestCleanUIOProfileMissingDirIsNotAnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-such-data-dir", appProfileDirName)

	removed, err := cleanUIOProfile(dir, appProfileMaxAge)
	if err != nil {
		t.Fatalf("目录不存在不该返回错误: %v", err)
	}
	if removed != 0 {
		t.Errorf("目录不存在时 removed = %d，期望 0", removed)
	}
}

// TestCleanUIOProfileTinyThresholdRemovesAllButDir 用 1 纳秒把「刚写的也算
// 过期」变成确定的用例：所有子条目都没了，ui-profile 目录本身仍在。
func TestCleanUIOProfileTinyThresholdRemovesAllButDir(t *testing.T) {
	_, profile := newProfileDir(t)
	writeProfileFile(t, filepath.Join(profile, "Default", "History"), "old")
	writeProfileFile(t, filepath.Join(profile, "Local State"), "fresh")
	// Windows 的文件时间戳按系统时钟粒度取整，可能与 time.Now() 落在同一个
	// tick 上；等 2ms 保证「刚写」的 mtime 严格早于 now-1ns。
	time.Sleep(2 * time.Millisecond)

	removed, err := cleanUIOProfile(profile, time.Nanosecond)
	if err != nil {
		t.Fatalf("cleanUIOProfile 返回错误: %v", err)
	}
	if removed != 2 {
		t.Errorf("删掉的顶层条目数 = %d，期望 2（Default 与 Local State）", removed)
	}

	entries, err := os.ReadDir(profile)
	if err != nil {
		t.Fatalf("profile 目录必须保留，却读不了: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("极小阈值下应当清空，剩下的条目: %v", entries)
	}
	if info, err := os.Stat(profile); err != nil || !info.IsDir() {
		t.Errorf("profile 目录本身必须保留: info=%v err=%v", info, err)
	}
}

// TestCleanUIOProfileSkipsSymlinkEscape 是安全边界：profile 里有一条指向
// 外部目录的符号链接时，清理既不跟随它去删外部内容，也不把链接本身当成
// 可删条目——解析出的路径不在 dir 内，删除的前置条件不成立。
// Windows 默认不允许普通用户建符号链接（要开发者模式或管理员权限），
// 建不出来就跳过：这是环境限制，不是逻辑没覆盖。
func TestCleanUIOProfileSkipsSymlinkEscape(t *testing.T) {
	outside := t.TempDir()
	keep := filepath.Join(outside, "keep.txt")
	writeProfileFile(t, keep, "keep")

	_, profile := newProfileDir(t)
	link := filepath.Join(profile, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("这台机器建不了目录符号链接（Windows 需要开发者模式或管理员权限），跳过: %v", err)
	}
	time.Sleep(2 * time.Millisecond) // 同上面的时钟粒度说明

	removed, err := cleanUIOProfile(profile, time.Nanosecond)
	if err != nil {
		t.Fatalf("cleanUIOProfile 返回错误: %v", err)
	}
	if removed != 0 {
		t.Errorf("指向外部的符号链接不该被清理，removed = %d", removed)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("外部目录里的文件被删了: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("外部目录被删了: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("指向外部的符号链接应当整条跳过（不跟随、也不删），Lstat: %v", err)
	}
}

// TestIsInsideDirRejectsPrefixAndSelf 直接钉删除的前置条件：名字只差一个
// 后缀的兄弟目录（ui-profile-x）与 root 自己都不算「在里面」。生产里
// cleanUIOProfile 传进来的 root 已经是真实路径，这里也先解析再比。
func TestIsInsideDirRejectsPrefixAndSelf(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("解析临时目录失败: %v", err)
	}
	root := filepath.Join(base, appProfileDirName)
	child := filepath.Join(root, "Default")
	sibling := filepath.Join(base, appProfileDirName+"-x")
	for _, dir := range []string{root, child, sibling} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("建目录 %s 失败: %v", dir, err)
		}
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "子目录在内部", path: child, want: true},
		{name: "名字带前缀的兄弟目录在外部", path: sibling, want: false},
		{name: "root 自己不算在内部", path: root, want: false},
		{name: "解析不出的条目按外部处理", path: filepath.Join(root, "gone"), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// 解析不出的条目会带回错误；这里只看「是否允许删除」的结论。
			got, _ := isInsideDir(root, tc.path)
			if got != tc.want {
				t.Errorf("isInsideDir(%q) = %v，期望 %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestCleanUIOProfileAtStartupIsSilent 钉住「正常清理不打印」：目录不存在
// 与真的清掉过期条目两种情况都必须零输出，启动日志保持安静。
func TestCleanUIOProfileAtStartupIsSilent(t *testing.T) {
	t.Run("目录不存在", func(t *testing.T) {
		var log bytes.Buffer
		cleanUIOProfileAtStartup(t.TempDir(), &log)
		if log.Len() != 0 {
			t.Errorf("没有 profile 目录时不该打印任何东西，实际:\n%s", log.String())
		}
	})

	t.Run("正常清理过期条目", func(t *testing.T) {
		dataDir, profile := newProfileDir(t)
		old := filepath.Join(profile, "Cache")
		writeProfileFile(t, filepath.Join(old, "data_0"), "old")
		backdateProfilePath(t, old, 8*24*time.Hour)

		var log bytes.Buffer
		cleanUIOProfileAtStartup(dataDir, &log)
		if log.Len() != 0 {
			t.Errorf("正常清理不该打印任何东西，实际:\n%s", log.String())
		}
		if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("过期条目应当被清掉，Stat 结果 err=%v", err)
		}
	})
}

// TestCleanUIOProfileAtStartupPrintsOneLineOnError 钉住错误路径：不用
// -verbose 也要说一声，而且只占一行（多条失败压进同一行）。真实的读失败
// 在 Windows 上很难稳定造出来（同名普通文件会被 os.ReadDir 报成「不存在」），
// 所以这里换掉包级函数注入一个双错误。
func TestCleanUIOProfileAtStartupPrintsOneLineOnError(t *testing.T) {
	old := cleanUIOProfileFn
	cleanUIOProfileFn = func(string, time.Duration) (int, error) {
		return 0, errors.Join(errors.New("第一处失败"), errors.New("第二处失败"))
	}
	t.Cleanup(func() { cleanUIOProfileFn = old })

	var log bytes.Buffer
	cleanUIOProfileAtStartup(t.TempDir(), &log)

	got := log.String()
	if !strings.HasPrefix(got, "council serve: ") {
		t.Errorf("提示应当以启动前缀开头，实际: %q", got)
	}
	if strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "\n") {
		t.Errorf("错误提示必须只有一行，实际:\n%s", got)
	}
	for _, want := range []string{"清理", "-app profile", "第一处失败", "第二处失败"} {
		if !strings.Contains(got, want) {
			t.Errorf("错误提示里缺少 %q: %s", want, got)
		}
	}
}

// TestListenAndServeCleansProfileAtStartup 是接线证明：清理挂在只读与 -rw
// 共用的 listenAndServe 上，不是某个模式里的私有分支。这里直接走那个入口，
// 用真实监听器与可取消的 ctx，确认过期条目在服务起来时被清掉。
func TestListenAndServeCleansProfileAtStartup(t *testing.T) {
	dataDir, profile := newProfileDir(t)
	old := filepath.Join(profile, "Cache")
	writeProfileFile(t, filepath.Join(old, "data_0"), "old")
	backdateProfilePath(t, old, 8*24*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newSyncBuffer()
	done := make(chan error, 1)
	go func() {
		done <- listenAndServe(ctx, "127.0.0.1:0", http.NotFoundHandler(),
			browserOptions{DataDir: dataDir}, out, io.Discard, nil)
	}()

	select {
	case <-out.wrote:
	case <-time.After(5 * time.Second):
		t.Fatal("5 秒内没有打印启动行")
	}
	// 启动行打印在清理之前，所以不能只等这一行；轮询等清理落地。
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(old); errors.Is(err, fs.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("listenAndServe 起来之后过期条目还在，启动清理没有接上")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("取消 ctx 之后 listenAndServe 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取消 ctx 之后 listenAndServe 超过 5 秒没有返回")
	}
}
