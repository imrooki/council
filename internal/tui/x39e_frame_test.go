package tui

// 本文件覆盖冻结契约「X-39e」（E:\multi_model_cowork\x39e_contract.md，v1.2）
// 第 2.3 节的 A9、A10：frameHeight 与 View 同源、finalSupportsVT 的查询判据。跨平台。
//
// 引用本刀新增的 frameHeight、finalSupportsVT，HEAD 上编译不过，这是契约 2.5 预期的红。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成。

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

// ═══ A9 ═══════════════════════════════════════════════════════════

// A9：frameHeight 等于 View 的行数；横线宽度两个字面量（下限 20、上限 120）在 chrome 里不变。
//
// 【判别力，逐句】
//   - frameHeight == View 行数 == 8：frameHeight 只返回 ta.Height()（6）红；
//     View 与 chrome 行数脱节（View 多拼一行或少拼一行）红。
//   - SetHeight(3) 之后两者都等于 5：frameHeight 写成常数 8 时红，这一句是它唯一的杀手。
//     对照：同一个 Model 在 SetHeight 之前是 8，两句只差 textarea 的高度。
//   - 横线宽度：四种尺寸依次 80、80、20、120。上限改成别的值时 {200,50} 红；
//     下限改成别的值时 {10,3} 红；{80,24} 与 {0,0} 是两者都不动的对照。
//     要 TrimRight：JoinVertical 会把各行补到最宽行的宽度。
func TestX39eFrameHeightMatchesView(t *testing.T) {
	for _, tc := range []struct {
		w, h     int
		ruleWant int
	}{
		{80, 24, 80},
		{0, 0, 80},
		{10, 3, 20},
		{200, 50, 120},
	} {
		t.Run(fmt.Sprintf("%dx%d", tc.w, tc.h), func(t *testing.T) {
			m := x39eSized(t, tc.w, tc.h)

			viewLines := strings.Count(m.View().Content, "\n") + 1
			if fh := m.frameHeight(); fh != viewLines || fh != 8 {
				t.Errorf("默认 textarea 高度下 frameHeight() = %d，View 的行数是 %d，两者都应当是 8。\n"+
					"frameHeight 与 View 必须共用 chrome()（契约 1.1 M3）", fh, viewLines)
			}

			rule := strings.TrimRight(strings.Split(m.View().Content, "\n")[1], " ")
			if want := strings.Repeat("─", tc.ruleWant); rule != want {
				t.Errorf("尺寸 %dx%d 下 View 第 2 行（横线）是 %q（%d 个字符），应当是 %d 个 ─。\n"+
					"横线宽度是 min(max(width,20),120)，M3 只挪位置、View 的输出逐字节不变",
					tc.w, tc.h, rule, len([]rune(rule)), tc.ruleWant)
			}

			m.ta.SetHeight(3)
			viewLines = strings.Count(m.View().Content, "\n") + 1
			if fh := m.frameHeight(); fh != viewLines || fh != 5 {
				t.Errorf("textarea 高度设成 3 之后 frameHeight() = %d，View 的行数是 %d，两者都应当是 5。\n"+
					"frameHeight 不能写成常数，必须跟着 textarea 的高度走", fh, viewLines)
			}
		})
	}
}

// ═══ A10 ═══════════════════════════════════════════════════════════

// A10：finalSupportsVT 对管道、缓冲、nil *os.File 的判定。
//
// 【判别力】恒真时管道那句在 Windows 上红；「非 *os.File 当成不支持」时缓冲那句红。
// 对照：管道与缓冲两句只差 w 是不是 *os.File。
// 不用 os.Stdout 做被测对象：go test 下它本来就是管道，碰巧成立。
// 非 Windows 上 finalSupportsVT 恒真（vt_other.go），管道那句期望 true，nil 句柄那句不跑。
func TestX39eVTQuery(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提没成立：os.Pipe 失败：%v", err)
	}
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	if got, want := finalSupportsVT(pw), runtime.GOOS != "windows"; got != want {
		t.Errorf("finalSupportsVT(管道写端) = %v，应当是 %v。\n"+
			"Windows 上管道查不到控制台模式，写进去的 ESC 会原样落进对端，必须判不支持（契约 1.1 V1）", got, want)
	}
	if !finalSupportsVT(&bytes.Buffer{}) {
		t.Errorf("finalSupportsVT(*bytes.Buffer) = false，应当是 true：不是 *os.File 时当作支持（契约 1.1 V1）")
	}
	if runtime.GOOS == "windows" {
		if finalSupportsVT((*os.File)(nil)) {
			t.Errorf("finalSupportsVT((*os.File)(nil)) = true，应当是 false：nil 句柄上 GetConsoleMode 失败即不支持")
		}
	}
}
