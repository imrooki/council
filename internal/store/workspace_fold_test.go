// workspace_fold_test.go 钉住 B2：工作区重名判重从 SQL 的 COLLATE NOCASE
// （只折 ASCII）换成 Go 的 Unicode 折叠。每条断言都要能对上
// workspaceNamesEqual / requireWorkspaceNameFree 注释里写明的语义与边界：
//
//   - İstanbul vs istanbul：EqualFold 不折 U+0130，但 ToLower 兜底把它折成
//     ASCII i，所以算重名；
//   - 全角 Ａ vs 半角 A：不做 NFKC/兼容折叠，不算重名；
//   - 中文同名：没有大小写，逐字相等，算重名；
//   - 其余几条钉「没有假装全覆盖」：é 的组合形式、变音剥离、ß 展开都不做。
package store

import (
	"errors"
	"testing"
)

// TestWorkspaceNameDuplicateUnicodeFolding 覆盖 Create 与 Rename 两条写路径：
// 该判重的两个方向都报 ErrWorkspaceNameTaken，不该判重的都能写进去。
func TestWorkspaceNameDuplicateUnicodeFolding(t *testing.T) {
	cases := []struct {
		name      string
		first     string
		second    string
		duplicate bool
		why       string
	}{
		{
			name:      "İstanbul 与 istanbul",
			first:     "İstanbul",
			second:    "istanbul",
			duplicate: true,
			why:       "EqualFold 不折 U+0130，ToLower 兜底把两者都折成 istanbul",
		},
		{
			name:      "全角 Ａ 与半角 A",
			first:     "Ａ",
			second:    "A",
			duplicate: false,
			why:       "不做 NFKC/兼容折叠：全角 Ａ 的小写是全角 ａ，不是 ASCII a",
		},
		{
			name:      "中文同名",
			first:     "工作区",
			second:    "工作区",
			duplicate: true,
			why:       "中文没有大小写，EqualFold 逐 rune 相等",
		},
		{
			name:      "带变音符号的同字母大小写",
			first:     "Café",
			second:    "CAFÉ",
			duplicate: true,
			why:       "é/É 有简单大小写折叠",
		},
		{
			name:      "变音符号不被剥离",
			first:     "café",
			second:    "cafe",
			duplicate: false,
			why:       "只折大小写，不做 NFD 去变音",
		},
		{
			name:      "预组合与分解形式",
			first:     "café",
			second:    "cafe\u0301",
			duplicate: false,
			why:       "不做 NFC/NFD 规范化",
		},
		{
			name:      "ß 不展开为 ss",
			first:     "Straße",
			second:    "STRASSE",
			duplicate: false,
			why:       "简单大小写折叠与 ToLower 都不把 ß 展开成 ss",
		},
		{
			name:      "开尔文符号与 k",
			first:     "\u212A",
			second:    "k",
			duplicate: true,
			why:       "EqualFold 按 Unicode 简单折叠把 KELVIN SIGN 折成 k",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := workspaceNamesEqual(tc.first, tc.second); got != tc.duplicate {
				t.Errorf("workspaceNamesEqual(%q, %q) = %v，期望 %v（%s）",
					tc.first, tc.second, got, tc.duplicate, tc.why)
			}

			// 新建路径单独一个库，改名路径另一个库：否则「新建成功」本身
			// 就会让同一次用例里的改名撞上刚建出来的名字。
			createStore, _ := tstStore(t)
			createCtx := tstCtx(t)
			first := tstMustCreateWorkspace(t, createCtx, createStore, tc.first)
			_, createErr := createStore.CreateWorkspace(createCtx, tc.second)
			if tc.duplicate {
				if !errors.Is(createErr, ErrWorkspaceNameTaken) {
					t.Errorf("CreateWorkspace(%q) = %v，期望 ErrWorkspaceNameTaken（%s）",
						tc.second, createErr, tc.why)
				}
			} else if createErr != nil {
				t.Errorf("CreateWorkspace(%q) = %v，期望成功（%s）", tc.second, createErr, tc.why)
			}
			if got := first.Name; got != tc.first {
				t.Errorf("第一个工作区的名字 = %q，期望 %q（判重不该改写已存在的行）", got, tc.first)
			}
			createList, err := createStore.Workspaces(createCtx)
			if err != nil {
				t.Fatalf("新建路径的 Workspaces 失败: %v", err)
			}
			wantCount := 2
			if tc.duplicate {
				wantCount = 1
			}
			if len(createList) != wantCount {
				t.Errorf("新建路径之后工作区数 = %d，期望 %d", len(createList), wantCount)
			}

			renameStore, _ := tstStore(t)
			renameCtx := tstCtx(t)
			target := tstMustCreateWorkspace(t, renameCtx, renameStore, tc.first)
			other := tstMustCreateWorkspace(t, renameCtx, renameStore, "另一个工作区")
			_, renameErr := renameStore.RenameWorkspace(renameCtx, other.ID, tc.second)
			if tc.duplicate {
				if !errors.Is(renameErr, ErrWorkspaceNameTaken) {
					t.Errorf("RenameWorkspace(..., %q) = %v，期望 ErrWorkspaceNameTaken（%s）",
						tc.second, renameErr, tc.why)
				}
			} else if renameErr != nil {
				t.Errorf("RenameWorkspace(..., %q) = %v，期望成功（%s）", tc.second, renameErr, tc.why)
			}
			renameList, err := renameStore.Workspaces(renameCtx)
			if err != nil {
				t.Fatalf("改名路径的 Workspaces 失败: %v", err)
			}
			got := ""
			for _, w := range renameList {
				if w.ID == other.ID {
					got = w.Name
				}
			}
			want := "另一个工作区"
			if !tc.duplicate {
				want = tc.second
			}
			if got != want {
				t.Errorf("改名之后 other 的名字 = %q，期望 %q", got, want)
			}
			if target.Name != tc.first {
				t.Errorf("改名路径里第一个工作区的名字 = %q，期望 %q", target.Name, tc.first)
			}
		})
	}
}

// TestWorkspaceRenameToSelfWithUnicodeCase 钉住「排除自己」在 Unicode 上
// 同样成立：把 İstanbul 改成 İSTANBUL 不算撞名，返回的 Name 是新写法。
func TestWorkspaceRenameToSelfWithUnicodeCase(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	ws := tstMustCreateWorkspace(t, ctx, s, "İstanbul")

	renamed, err := s.RenameWorkspace(ctx, ws.ID, "İSTANBUL")
	if err != nil {
		t.Fatalf("改成自己的大写形式失败: %v（应当排除自己）", err)
	}
	if renamed.Name != "İSTANBUL" {
		t.Errorf("改名返回的 Name = %q，期望 İSTANBUL", renamed.Name)
	}
}
