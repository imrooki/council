// lock_test.go 是未决 #8 的回归用例：data_dir 排他锁。
//
// 背景与动机全在 open.go 里 ErrDataDirLocked 的注释与 docs/00_DECISIONS.md 未决 #8：
// 没有这把锁之前，同一个 data_dir 上第二个 council 进程会静默打开、静默写入、
// 退出码 0，没有任何一层会报错，直到事后翻库才发现两次入场费、一条孤儿 provider
// 会话、错位的游标。这份用例锁住两件事：(1) 第二个 Open 必须被拒绝且报错可辨认；
// (2) Open 在取锁之后的 early return 上失败时，锁必须被释放，不能留下一把
// 谁也拿不掉的锁——覆盖两条位置不同的 early return，证明放锁的是 defer 机制本身，
// 不是某一条具体分支上手写的一行 release()。
package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenRejectsSecondProcessOnSameDataDir 覆盖锁的核心行为：
// 同一个 data_dir 上，第一个 Store 还开着时，第二次 Open 必须失败并报
// ErrDataDirLocked；第一个 Close 之后，第二次 Open 必须能成功。
//
// 名字里的"第二个进程"是对着未决 #8 原始场景写的，但这条用例本身是**同一个测试
// 进程**里连续调用两次 Open，不是两个真实的操作系统进程。这仍然是排他性的有效
// 证据——LockFileEx / flock 的排他锁是按文件句柄计的，同一个进程里两个不同的
// os.File / windows.Handle 一样会互斥，不会因为"同进程"就互相谅解。
// 但它没有覆盖"跨进程"这一半，那一半靠 E:\Isolating_test\council\oq8-lock\RESULTS.md
// 里两个真实进程的真机记录补上：持锁进程还活着时，另一个真实进程去 Open 同一个
// data_dir，立刻拿到 ErrDataDirLocked，不阻塞、不重试。
func TestOpenRejectsSecondProcessOnSameDataDir(t *testing.T) {
	dir := tstDataDir(t)

	_, closeFirst := tstOpenAt(t, dir)

	_, err := Open(tstCtx(t), dir)
	if err == nil {
		t.Fatal("同一个 data_dir 上第二次 Open 竟然成功了：锁没有生效（未决 #8 的实测损坏会重现）")
	}
	if !errors.Is(err, ErrDataDirLocked) {
		t.Errorf("错误应当能被 errors.Is 认出 ErrDataDirLocked，实际: %v", err)
	}
	if !tstErrMentionsPath(err, lockFileName) {
		t.Errorf("错误信息里没有提到锁文件名 %q，用户不知道该找哪个文件: %v", lockFileName, err)
	}

	// 第一个 Store 关掉之后，锁必须已经释放，第二次 Open 应当能正常成功。
	closeFirst()

	s2, err := Open(tstCtx(t), dir)
	if err != nil {
		t.Fatalf("第一个 Store 关闭之后，同一个 data_dir 上再次 Open 应当成功，实际: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Errorf("关闭第二个 Store 失败: %v", err)
	}
}

// TestOpenReleasesLockOnValidateSchemaFailure 覆盖「取锁之后、离成功最远的一条
// early return 也必须放锁」：validateSchema 失败发生在 lockOwned 已经转交给 Store
// 之后，靠的是失败分支里显式调用的 s.Close()，不是 Open 顶部那个 defer。
//
// 造法：先用 tstBuildStaleDB（TestOpenRejectsStaleSchema 同款辅助）造一个
// validateSchema 一定会拒绝的旧库。关键断言在第二次 Open：如果上一次失败时锁
// 没放，这次会先撞 ErrDataDirLocked；如果锁放了，这次会照常一路开到
// validateSchema、报同一个 ErrSchemaMismatch（库本身还是旧的，这次当然也开不
// 成功，但失败的位置证明了锁已经不在了）。
func TestOpenReleasesLockOnValidateSchemaFailure(t *testing.T) {
	dir := t.TempDir()
	tstBuildStaleDB(t, dir)

	_, err1 := Open(context.Background(), dir)
	if !errors.Is(err1, ErrSchemaMismatch) {
		t.Fatalf("前置条件不成立：第一次 Open 期望 ErrSchemaMismatch，实际: %v", err1)
	}

	_, err2 := Open(context.Background(), dir)
	if errors.Is(err2, ErrDataDirLocked) {
		t.Fatalf("锁在 validateSchema 失败之后没有被释放，第二次 Open 撞的是 ErrDataDirLocked: %v", err2)
	}
	if !errors.Is(err2, ErrSchemaMismatch) {
		t.Fatalf("第二次 Open 期望仍是 ErrSchemaMismatch（证明锁已经放了，只是库本身还是旧的），实际: %v", err2)
	}
}

// TestOpenReleasesLockOnWriteHandlePingFailure 覆盖「取锁之后、离取锁最近的一条
// early return 也必须放锁」：这里靠的是 Open 顶部那个 defer（lockOwned 尚未转交
// 给 Store 之前失败），与上一条覆盖的 Store.Close 那条路径互补。
//
// 造法：把 council.db 预先建成一个目录而不是文件，writeDB.PingContext 会在真正
// 访问文件时失败——这条 early return 在 lockOwned = false 那一行之前。
// 判据不绕回 Open（同一个损坏的 db 会一直失败，测不出锁的状态），而是直接调用
// 包内部的 acquireDataDirLock 探测：能立刻拿到锁，就证明上一次失败已经放干净了。
func TestOpenReleasesLockOnWriteHandlePingFailure(t *testing.T) {
	dir := tstDataDir(t)
	if err := os.MkdirAll(filepath.Join(dir, dbFileName), 0o755); err != nil {
		t.Fatalf("准备阻塞目录失败: %v", err)
	}

	_, err := Open(tstCtx(t), dir)
	if err == nil {
		t.Fatal("council.db 路径被目录占用时 Open 应当失败")
	}
	if errors.Is(err, ErrDataDirLocked) {
		t.Fatalf("前置条件不成立：这次失败不该是 ErrDataDirLocked，实际: %v", err)
	}
	// 确认失败确实发生在 writeDB.PingContext 这一步，而不是别的早退分支
	// 顺带碰巧也失败了——否则这条用例锁住的位置就不是它名字说的那条。
	if !strings.Contains(err.Error(), "ping write db") {
		t.Fatalf("期望错误来自 ping write db 这条 early return，实际: %v", err)
	}

	lock, err := acquireDataDirLock(dir)
	if err != nil {
		t.Fatalf("上一次 Open 失败之后锁应当已经释放，重新取锁却失败: %v", err)
	}
	if err := lock.release(); err != nil {
		t.Errorf("释放探测用的锁失败: %v", err)
	}
}
