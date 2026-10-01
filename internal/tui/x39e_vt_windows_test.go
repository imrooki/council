package tui

// 本文件覆盖冻结契约「X-39e」（E:\multi_model_cowork\x39e_contract.md，v1.2）
// 第 2.3 节的 A10b：控制台句柄上的 VT 查询。只在 Windows 上编译。
//
// 只查询，不许 SetConsoleMode。
// 杀伤随环境变（契约 4 节第 13、17 条）：
//   - VT 位判据反转：任何打得开 CONOUT$ 的环境都红；
//   - 只看查询成败、不看 VT 位：只在控制台 VT 关着的环境红，VT 开着时存活。
// 打不开 CONOUT$（没有控制台）时 Skip，两条都测不到，按工装缺口记。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成。

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// A10b：finalSupportsVT 在真控制台句柄上的结果，等于独立算出的 VT 位。
//
// 【能不能为假】want 取自同一个句柄上独立的 GetConsoleMode，不取自被测函数；
// 判据反转时 got 恰好是 !want，必红。失败信息打出 mode 的十六进制，便于判断环境。
func TestX39eVTQueryConsole(t *testing.T) {
	name, err := windows.UTF16PtrFromString("CONOUT$")
	if err != nil {
		t.Fatalf("前提没成立：UTF16PtrFromString 失败：%v", err)
	}
	h, err := windows.CreateFile(name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Skipf("打不开 CONOUT$（没有控制台的环境）：%v。A10b 在这里测不到，按工装缺口记", err)
	}
	f := os.NewFile(uintptr(h), "CONOUT$")
	t.Cleanup(func() { _ = f.Close() })

	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		t.Skipf("CONOUT$ 打开了但 GetConsoleMode 失败：%v。A10b 在这里测不到，按工装缺口记", err)
	}
	want := mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0
	t.Logf("CONOUT$ 的控制台模式是 0x%X，VT 位 %v", mode, want)

	if got := finalSupportsVT(f); got != want {
		t.Errorf("finalSupportsVT(CONOUT$) = %v，应当是 %v（控制台模式 0x%X，ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x%X）。\n"+
			"判据是 GetConsoleMode 成功且 VT 位开着（契约 1.1 V1）",
			got, want, mode, windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
