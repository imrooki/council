package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"
)

// testDBFileName 是数据目录下的库文件名（08a：dataDir 下的 council.db）。
// 它是契约面的一部分，用户要能自己拿 SQLite 查看器打开它。
const testDBFileName = "council.db"

// normalizePath 把路径规整成可比较的形式：统一成正斜杠、统一成小写。
// 实现可能为了 DSN 把路径 ToSlash 过一遍再打进横幅，横幅里到底是哪种分隔符不属于契约，
// 所以断言路径包含关系时两边都先规整。大小写统一是为了 Windows。
func normalizePath(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, `\`, "/"))
}

// containsPath 判断 haystack 里有没有出现 path，忽略分隔符方向与大小写。
func containsPath(haystack, path string) bool {
	return strings.Contains(normalizePath(haystack), normalizePath(path))
}

// TestRunStoreOpenFailure 锁住规格 I 的失败路径：store 打不开时，
// 错误写进 Err、Run 返回错误、不进 REPL。
//
// 制造失败的办法是把数据目录指向一个已经存在的普通文件：
// os.MkdirAll 撞上非目录会确定性地失败，两个平台都一样
// （Windows 上靠只读目录或非法字符制造失败都不可靠）。
//
// 「不进 REPL」的证据是 In 给了一个永不结束的 reader：
// 真进了 REPL，Run 就会挂住并被 runAppForTest 的看门狗判失败。
func TestRunStoreOpenFailure(t *testing.T) {
	isolateAppEnv(t)

	dir := t.TempDir()
	blocker := filepath.Join(dir, "data-dir-is-actually-a-file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("写占位文件失败: %v", err)
	}
	cfgPath := writeTestConfig(t, dir, validConfigBody(blocker))

	_, errOut, err := runAppForTest(t, context.Background(), cfgPath, newTestBlockingReader(t))
	if err == nil {
		t.Fatalf("store 打不开时 Run 应当返回错误，实际返回 nil；Err 输出：\n%s", errOut)
	}
	if strings.TrimSpace(errOut) == "" {
		t.Errorf("store 打不开时 Err 里应当有可读信息，实际是空的")
	}
	if !containsPath(errOut, blocker) {
		t.Errorf("Err 里应当含那个打不开的路径 %q；完整输出：\n%s", blocker, errOut)
	}
}

// TestBannerShowsDatabasePath 锁住横幅里那一行库文件路径（规格 I）。
// 跑两次：第一次建库，第二次开的是已经存在的库，两次都要报出同一个路径。
// 第二次顺带证明了重开已有库这条路在 app 这一层是通的 —— 关闭失败或残留句柄会在这里现形
// （Windows 上库文件没关掉，t.TempDir 的清理会报文件被占用）。
func TestBannerShowsDatabasePath(t *testing.T) {
	isolateAppEnv(t)

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cfgPath := writeTestConfig(t, dir, validConfigBody(dataDir))
	dbPath := filepath.Join(dataDir, testDBFileName)

	for _, round := range []string{"新建库", "重开已有库"} {
		t.Run(round, func(t *testing.T) {
			out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(""))
			if err != nil {
				t.Fatalf("Run 应当返回 nil，实际 %v；Err 输出：\n%s", err, errOut)
			}
			if !containsPath(out, dbPath) {
				t.Errorf("横幅里应当含库文件路径 %q；Out 完整输出：\n%s", dbPath, out)
			}
			if _, statErr := os.Stat(dbPath); statErr != nil {
				t.Errorf("库文件 %s 应当存在，实际: %v", dbPath, statErr)
			}
		})
	}
}

// TestRunCreatesDataDirFromEnv 断言数据目录来自环境变量时同样会被创建。
// Step 4 的 TestRunDataDirResolution 只看横幅里的路径对不对，
// 不关心目录有没有真的建出来；建目录是 Step 5 才有的行为，单独锁一条。
func TestRunCreatesDataDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	envDataDir := filepath.Join(dir, "data-from-env")

	t.Setenv(config.EnvConfigPath, "")
	t.Setenv(config.EnvDataDir, envDataDir)

	cfgPath := writeTestConfig(t, dir, validConfigBody(""))

	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；Err 输出：\n%s", err, errOut)
	}
	if _, statErr := os.Stat(filepath.Join(envDataDir, testDBFileName)); statErr != nil {
		t.Errorf("环境变量指定的数据目录下应当有库文件，实际: %v；Out 完整输出：\n%s", statErr, out)
	}
}
