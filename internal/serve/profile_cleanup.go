package serve

// profile_cleanup.go 是 docs/29_FIXES_PLAN.md B5 的落点：`-app` 用的
// Chromium profile（<data_dir>\ui-profile）只会变大、从来不清理，用久了
// 就是一份没人回收的窗口缓存。
//
// 清理口径刻意保守：
//   - 只删「最后修改时间早于 7 天」的子条目，目录本身永远留着。serve 退出后
//     应用窗口仍然开着是预期行为（见 appwindow.go），连同目录一起删会把正在
//     用的窗口弄坏；正在用的 profile 一直在写，mtime 是新的，自然不会被选中。
//   - 读失败、删不掉都只打一行，绝不影响启动：清理回收的是可再生的缓存，
//     没有理由让它挡住看板。
//   - 删除之前必须能证明目标仍在 profile 目录之内（filepath.Rel 判断），
//     解析到外部的符号链接不跟随、也不删——清理不能变成「顺着一条链接删掉
//     data_dir 以外的东西」的入口。

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// appProfileMaxAge 是 -app profile 里条目的保留时长。7 天比「隔了一个周末
// 再打开看板」宽松得多，又足够把几个月前的窗口缓存收掉。
const appProfileMaxAge = 7 * 24 * time.Hour

// cleanUIOProfileFn 是包级变量，测试可以换掉它来断言「清理失败只打一行」；
// 生产走 cleanUIOProfile。与 openAppWindow / openBrowser 的注入方式一致。
var cleanUIOProfileFn = cleanUIOProfile

// cleanUIOProfileAtStartup 是启动路径上的薄壳：两种模式都在 listenAndServe
// 里经过它一次（见其中的调用点）。它自己吞掉所有错误——清理失败只是少回收
// 一点磁盘，服务必须照常起来；但也不是完全无声，失败打一行就够，不需要
// -verbose。
func cleanUIOProfileAtStartup(dataDir string, errOut io.Writer) {
	if dataDir == "" {
		// 没有 data_dir 就没有确定的 profile 位置；退一步，别拿相对路径
		// "ui-profile" 去删进程当前目录里的东西。
		return
	}
	dir := filepath.Join(dataDir, appProfileDirName)
	if _, err := cleanUIOProfileFn(dir, appProfileMaxAge); err != nil {
		// 多个条目删不掉时 errors.Join 会带换行；压成一行，启动日志才不会被
		// 拆成几条没有前缀的消息。
		fmt.Fprintf(errOut, "council serve: 清理 -app profile 失败（可忽略）: %s\n",
			strings.ReplaceAll(err.Error(), "\n", "; "))
	}
}

// cleanUIOProfile 删除 dir 下最后修改时间早于 olderThan 的顶层条目（文件、
// 符号链接或整棵目录树），返回删掉的顶层条目数。dir 本身不删。
//
// 返回的 err 是「读目录失败」与「若干条目删不掉」的合并：调用方只该说一声
// 然后继续启动。单个条目删不掉（典型：正被另一个进程占用）不会中途中止，
// 剩下的照删——能回收多少是多少。
//
// olderThan 从参数进来而不是写死成常量，测试才能用 1 纳秒把「全都过期」
// 变成一条确定的用例。
func cleanUIOProfile(dir string, olderThan time.Duration) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// 第一次用 -app 之前这个目录本来就不存在，不是错误。
			return 0, nil
		}
		return 0, err
	}

	// 判定基准用解析过符号链接的路径：data_dir 自身可能就在一条链接后面
	// （用户把数据目录放到另一块盘、macOS 的 /tmp 等），拿未解析的 dir 去比
	// 会把每个条目都判成「在外部」，一个都删不掉。
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-olderThan)
	removed := 0
	var errs []error
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		// Lstat 而不是 Stat：符号链接自己的 mtime 才是「多久没动过」的证据，
		// 跟随目标会拿外部目录的时间当依据。
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // 读目录之后被别的东西删了，没什么可做的
			}
			errs = append(errs, err)
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		ok, err := isInsideDir(root, path)
		if err != nil || !ok {
			// 解析不出的条目（断链）与解析到外部的条目都跳过：删除的前提是
			// 能证明它在 dir 里，证明不了就不动。这不属于「清理失败」，不报。
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}

// isInsideDir 判断 path 解析符号链接之后的真实路径是否仍在 root（调用方已
// 解析过的真实路径）之内。用 filepath.Rel 而不是 strings.HasPrefix：前缀比较
// 会把 "<root>-x" 当成 <root> 的子路径，还会在大小写不敏感的 Windows 上判错。
//
// 解析不出真实路径（断链、中途被删）时返回错误，调用方按「不在里面」处理。
func isInsideDir(root, path string) (bool, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		// 不同卷（D: 与 E:）连相对路径都算不出来，按「在外部」处理。
		return false, nil
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) {
		return false, nil
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}
