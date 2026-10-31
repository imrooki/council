package tui

// B19：复制模式的显示列必须用与 View 相同的字形簇宽度算法。
// 逐 rune 的 go-runewidth 会把 "⚠️" 算成一格，而 ansi.Truncate 按两格渲染，
// 拖选 "ab" 会错位成 "bc"。

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestB19CopyCellsMatchRenderedWidth(t *testing.T) {
	line := "⚠️abc"
	if got, want := ansi.StringWidth(line), 5; got != want {
		t.Fatalf("前提不成立：ansi.StringWidth(%q) = %d，期望 %d", line, got, want)
	}
	if got, want := copyLineCells(line, 2, 3), "ab"; got != want {
		t.Errorf("copyLineCells(%q, 2, 3) = %q，期望 %q（按字形簇宽度）", line, got, want)
	}

	// 中文宽字符仍按两格映射，不能因为换算法切出半个字。
	zh := "中文ab"
	if got, want := copyLineCells(zh, 2, 3), "文"; got != want {
		t.Errorf("copyLineCells(%q, 2, 3) = %q，期望 %q", zh, got, want)
	}
}
