package serve

// 服务补审：盘符列表按进程缓存一次，不能每个 /api/fs/dirs 请求都探测 A–Z。

import (
	"io/fs"
	"sync"
	"testing"
)

func TestBVolumeRootsCached(t *testing.T) {
	oldOnce, oldCache, oldStat := volumeRootsOnce, volumeRootsCache, volumeRootsStat
	defer func() {
		volumeRootsOnce, volumeRootsCache, volumeRootsStat = oldOnce, oldCache, oldStat
	}()
	volumeRootsOnce = sync.Once{}
	volumeRootsCache = nil
	calls := 0
	volumeRootsStat = func(string) (fs.FileInfo, error) {
		calls++
		return nil, fs.ErrNotExist
	}

	volumeRoots()
	first := calls
	volumeRoots()
	if calls != first {
		t.Fatalf("第二次调用又探测了 %d 次盘符；结果应当缓存", calls-first)
	}
}
