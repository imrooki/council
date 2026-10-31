package serve

// 服务补审：分页没有前进时不能静默截断导出件。

import (
	"strings"
	"testing"
)

func TestExportPaginationMarksTruncatedOnStalledPage(t *testing.T) {
	html := string(indexHTML)
	const guard = "if (!Number.isFinite(lastSeq) || lastSeq <= after) {"
	start := strings.Index(html, guard)
	if start < 0 {
		t.Fatalf("找不到分页防御分支 %q", guard)
	}
	block := html[start : start+600]
	if !strings.Contains(block, "truncated = true;") || !strings.Contains(block, "break;") {
		t.Fatalf("分页停滞时既没有标 truncated 也没有退出：%s", block)
	}
	if strings.Contains(html, "lastSeq <= after) break;") {
		t.Error("仍存在静默 break 的旧写法")
	}
}
