package i18n

// copy_tone_test.go 钉住一条产品约定：界面文案说的是「用户能看到什么」，
// 不是「代码里怎么实现的」。
//
// 起因是设置页「关于」的第一版导语写着「版本与构建信息来自 /api/health，不在
// 前端写死」—— 那是一句代码注释跑到了用户面前。用户看到的应该是一个应用在
// 介绍自己，而不是在读这个应用的实现说明。
//
// 这条用例只拦最明显的实现词，不去评判风格；它防的是「又写回去」。

import (
	"os"
	"strings"
	"testing"
)

func TestUICopyDoesNotLeakImplementation(t *testing.T) {
	// 这些词出现在 ui.* 里，几乎一定意味着把实现细节写给了用户。
	banned := []string{"/api/", "前端", "后端", "写死", "schema 版本"}

	data, err := os.ReadFile("messages.go")
	if err != nil {
		t.Fatalf("读 messages.go: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	checked := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, `"ui.`) {
			continue
		}
		// 只取值：键名本身可能含 Schema 之类的字样，不该被误伤。
		parts := strings.SplitN(trimmed, `":`, 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.Trim(parts[0], `"`)
		value := parts[1]
		checked++
		for _, word := range banned {
			if strings.Contains(value, word) {
				t.Errorf("%s 的值里出现了实现词 %q：%s", key, word, strings.TrimSpace(value))
			}
		}
	}
	if checked < 300 {
		t.Fatalf("只扫到 %d 条 ui.* 文案，行格式可能变了，这条用例已经失效", checked)
	}
}
