package config

import (
	"os"
	"path/filepath"
)

// projectDirs 返回从 cwd 到仓库根（含 .git 的那一层）的目录链，离 cwd 最近的在前。
//
// 为什么找不到 .git 时只返回 cwd、不一直走到盘符根：项目配置与 Claude agents
// 都是「这个仓库自己的东西」。没有仓库根就没有向上走的依据，走到 C:\ 会把
// 用户主目录、盘符根下面偶然存在的 .council / .claude 也当成这个项目的一部分，
// 那正是项目级配置要避免的污染。
//
// .git 是文件也算仓库根：git worktree 与 submodule 里 .git 是一个文本文件，
// 不是目录；按「是否存在」判断，两种都能认出来。
func projectDirs(cwd string) []string {
	if cwd == "" {
		return nil
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		// 取不到绝对路径时仍按原样向上走：Abs 失败通常只是路径畸形，
		// 直接放弃会让用户连当前目录的配置都读不到。
		abs = filepath.Clean(cwd)
	}

	chain := make([]string, 0, 4)
	for dir := abs; ; dir = filepath.Dir(dir) {
		chain = append(chain, dir)
		if isRepoRoot(dir) {
			return chain
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// 走到盘符根（或 Unix 的 /）也没见到 .git。
			break
		}
	}
	return []string{abs}
}

// isRepoRoot 判断 dir 是不是仓库根。只看 .git 是否存在，不看它是文件还是目录。
func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// firstProjectConfig 从 dirs 里由近到远找第一份存在的 .council/config.toml。
//
// 「由近到远、只取第一份」是任务书 §1 的明确规则：近的那份就是给这个项目看的，
// 不再把远的那份也叠上来 —— 叠上来会让「项目根写一份、子目录再写一份」变成
// 隐式继承，排查时说不清某个 agent 到底从哪来。
//
// Stat 的 ENOENT 继续向上找；别的错误（例如权限）按「存在」处理并返回，
// 让后面的读取报出带路径的真实错误，而不是静默跳过一份读不了的配置。
func firstProjectConfig(dirs []string) (string, bool) {
	for _, dir := range dirs {
		path := filepath.Join(dir, ".council", "config.toml")
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return path, true
		}
		if !info.IsDir() {
			return path, true
		}
	}
	return "", false
}
