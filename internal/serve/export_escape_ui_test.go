package serve

// 服务补审：Markdown 导出不能把标题/正文里的 HTML 原样写进文件。
// 导出的 .md 常被支持内嵌 HTML 的查看器打开，<script> 会被执行。

import (
	"strings"
	"testing"
)

func TestExportMarkdownEscapesHTML(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		"function exportMarkdownText",
		"exportMarkdownText((detail && detail.title)",
		"exportMarkdownText(message.text",
		"exportMarkdownText(exportSenderName",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("导出代码里缺少 %q：HTML 元字符没有转义", want)
		}
	}
	if strings.Contains(html, "lines.push(message.text || '')") {
		t.Error("导出仍把 message.text 原样写进 Markdown")
	}
}
