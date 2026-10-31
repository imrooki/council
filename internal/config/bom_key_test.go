package config

// P2：首行 BOM 不能被当成键名的一部分，否则已有顶层键认不出、会被重复插入。

import (
	"strings"
	"testing"
)

func TestBOMTopLevelKeyNotDuplicated(t *testing.T) {
	content := "\ufefflanguage = \"auto\"\n[providers.claude]\ncommand = 'claude'\n"
	next, found, err := setTopLevelKey([]byte(content), "language", "en")
	if err != nil || !found {
		t.Fatalf("setTopLevelKey: found=%v err=%v", found, err)
	}
	got := string(next)
	if !strings.HasPrefix(got, "\ufeff") {
		t.Errorf("BOM 丢失: %q", got)
	}
	if strings.Count(got, "language =") != 1 {
		t.Errorf("language 行出现了多次:\n%s", got)
	}
	if !strings.Contains(got, `language = "en"`) {
		t.Errorf("language 没有更新:\n%s", got)
	}
}
