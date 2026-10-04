package tui

// clx_ctrlaltc_test.go 是 CL1（一批小修补，契约 §2.2）第 7 条在 tui 包的验收：@ 补全列表开着时按 Ctrl+Alt+C
// 只收起列表，不退出、不提交（19c 文件头：列表分支 Ctrl-C 判据的 Alt 那一半没有用例）；v1.1 起同样核 Ctrl 加
// NumLock、CapsLock（Windows 控制台把锁定键的状态放进 Mod）。纯 Model 驱动，不起 Program。
// 只读复用 atx_model_test.go（atxNew、atxCands、atxMembers、atxB3、atxH、atxWant*）与 tui1b_model_test.go
// （tui1bType、tui1bStep、tui1bEsc），不改它们。
//
// Cmd 纪律（同 19c §2.0）：按键之后拿到的 Cmd 一律不执行，只有三种用法：与 nil 比较；作为 reflect.ValueOf 的唯一
// 实参并紧跟 .Pointer()；传给 atxWantNilCmd。失败消息里只报「cmd == nil」这个布尔值。

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// clxCtrlCKeys 是 C3 的三种键：Ctrl+C 再加 Alt、NumLock、CapsLock 之一。str 是键的 String()：锁定键不进 String()，
// 所以后两种都是 ctrl+c。
var clxCtrlCKeys = []struct {
	name string
	key  tea.KeyPressMsg
	str  string
}{
	{"alt", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModAlt}, "ctrl+alt+c"},
	{"numlock", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModNumLock}, "ctrl+c"},
	{"capslock", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModCapsLock}, "ctrl+c"},
}

// TestClxCtrlCModsClosesList 是 C3：列表开着时 Ctrl+C 带上 Alt、NumLock 或 CapsLock，只收起列表，Cmd 为 nil、值不变、
// 表头仍是「空闲」（不退出、不取消、不提交）。每种键各起新 Model。第 3 步是同装置对照（只差列表开没开）：列表不在时
// 同一个键连按两次交回 tea.Quit、表头是「正在退出」，证明这个键确实到了 Council 的 Ctrl-C 判据，第 2 步的 nil 不是「键没到」。
// CC2（2026-10-04）：退出与取消改为 2 秒内连按两次 Ctrl-C，这里按两次。
// 【判别力】V15（列表分支的判据写成只放过 Shift：msg.Mod&^tea.ModShift == tea.ModCtrl，19c 文件头审查员的那条）与
// V16（A60，收窄成 msg.Mod == tea.ModCtrl）下，三种键都掉进空闲的 Ctrl-C 分支：第 2 步只武装，Cmd 是到期 tick、行仍是 atxB3、
// 表头变成「再按一次 Ctrl-C 退出」，三处红。X4（只放过 Shift 与 Alt：msg.Mod&^(tea.ModShift|tea.ModAlt) == tea.ModCtrl）下
// numlock、capslock 两种红，alt 照绿。V15 下 TestAtfCtrlShiftCClosesList 照绿（这正是 19c 头说的缺口）。
func TestClxCtrlCModsClosesList(t *testing.T) {
	for _, k := range clxCtrlCKeys {
		t.Run(k.name, func(t *testing.T) {
			// 第 1 步（前提）：键的写法。
			if s := k.key.String(); s != k.str {
				t.Fatalf("第 1 步（前提）：键的 String() 应当是 %q，实际 %q", k.str, s)
			}

			// 第 2 步：列表开着。
			m, _ := atxNew(t, 80, 24, atxCands(atxMembers))
			m = tui1bType(t, m, "@b")
			atxWantLines(t, "第 2 步（前提）：敲 @b", m, atxB3)
			atxWantHead(t, "第 2 步（前提）：敲 @b", m, atxH("空闲"))
			m, cmd := tui1bStep(t, m, k.key)
			atxWantNilCmd(t, "第 2 步：列表开着时按下", cmd)
			atxWantLines(t, "第 2 步：列表开着时按下", m, nil)
			atxWantValue(t, "第 2 步：列表开着时按下", m, "@b")
			atxWantHead(t, "第 2 步：列表开着时按下", m, atxH("空闲"))

			// 第 3 步：对照（列表不在）。另起新 Model。
			m3, _ := atxNew(t, 80, 24, atxCands(atxMembers))
			m3 = tui1bType(t, m3, "@b")
			m3, _ = tui1bStep(t, m3, tui1bEsc())
			atxWantLines(t, "第 3 步（前提）：敲 @b 再按 Esc", m3, nil)
			m3, cmdArm := tui1bStep(t, m3, k.key)
			if cmdArm == nil || reflect.ValueOf(cmdArm).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
				t.Errorf("第 3 步：列表不在时第一次按下应当只武装、不是退出（cmd == nil：%v）", cmdArm == nil)
			}
			atxWantHead(t, "第 3 步：列表不在时第一次按下", m3, atxH("再按一次 Ctrl-C 退出"))
			m3, cmd3 := tui1bStep(t, m3, k.key)
			if reflect.ValueOf(cmd3).Pointer() != reflect.ValueOf(tea.Quit).Pointer() {
				t.Errorf("第 3 步：列表不在时按下，Cmd 应当是 tea.Quit（只比函数指针，不执行），实际不是（cmd == nil：%v）", cmd3 == nil)
			}
			atxWantHead(t, "第 3 步：列表不在时按下", m3, atxH("正在退出"))
			atxWantValue(t, "第 3 步：列表不在时按下", m3, "@b")
		})
	}
}
