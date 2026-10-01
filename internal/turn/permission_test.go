package turn

// permission.go 的测试面很窄：TranslatePermission 只有一件事要证明——
// read 与 workspace 都放行（11c 契约第二片），只有 full 在起进程之前就被
// 拒绝，而且拒绝时返回的档位是空字符串，不是悄悄降级成 read
// （09_STEP6_SPEC.md C 节：「降级成更低的档只是变慢，降级成更高的档是安全
// 事故，而两者在代码里长得一样」——所以连「降级成更低的档」这种看似无害的
// 写法都不许出现，返回值必须是空档，逼调用方去处理错误，而不是给一个能被
// 误用的档位当兜底）。
//
// 这里只测「编排器要不要放这个档出去」这一件事，不测「这家 provider 能不能
// 证明自己围得住 workspace」——后者是 provider.validateRequest 的写入边界闸，
// 两道闸并存但拦的不是同一件事，那边的用例在 internal/provider 包里。

import (
	"errors"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

func TestTranslatePermission(t *testing.T) {
	cases := []struct {
		name string
		in   config.Permission
		// wantTier 只在 wantErr 为 nil 时才检查；wantErr 非 nil 时固定断言
		// 返回的档位是空串（见上面的理由）。
		wantTier provider.PermissionTier
		wantErr  error
	}{
		{
			name:     "read 放行",
			in:       config.PermissionRead,
			wantTier: provider.PermissionRead,
		},
		{
			name:     "workspace 放行（11c 第二片）",
			in:       config.PermissionWorkspace,
			wantTier: provider.PermissionWorkspace,
		},
		{
			name:    "full 在起进程之前拒绝",
			in:      config.PermissionFull,
			wantErr: ErrTierNotOffered,
		},
		{
			name:    "空串或拼错的档一律拒绝，不降级成 read",
			in:      config.Permission("workspce"),
			wantErr: ErrTierNotOffered,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, err := TranslatePermission(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("errors.Is(err, %v) 应当为真，实际 err=%v", tc.wantErr, err)
				}
				if tier != "" {
					t.Errorf("拒绝时返回的档位应当是空串（不许悄悄降级），实际 %q", tier)
				}
				return
			}
			if err != nil {
				t.Fatalf("TranslatePermission(%q) 不应报错，实际: %v", tc.in, err)
			}
			if tier != tc.wantTier {
				t.Errorf("TranslatePermission(%q) = %q，期望 %q", tc.in, tier, tc.wantTier)
			}
		})
	}
}

// TestTranslatePermissionErrIsRunnerErrTierNotOffered 钉住 09a 的一条明文规定
// （改名自 ErrTierNotInStep6，订正记录见 docs/09a_STEP6_API_CONTRACT.md）：
// permission.go 里 TranslatePermission 返回的哨兵与 runner.go 里
// ErrTierNotOffered 是同一个变量，不是两处各声明一份同名不同值的哨兵。
// 用指针恒等而不是字符串比较，防止将来有人在 permission.go 里手滑写一个
// 新的 errors.New(...)——内容一样但不是同一个 error 值，errors.Is 在两个
// 哨兵不是同一变量时依然会通过（因为 errors.New 的值各自可比较），这里要抓
// 的恰恰是「同一个变量」这件事，所以直接比较返回的 error 与包级变量是否是
// 同一个底层值。
func TestTranslatePermissionErrIsRunnerErrTierNotOffered(t *testing.T) {
	_, err := TranslatePermission(config.PermissionFull)
	if !errors.Is(err, ErrTierNotOffered) {
		t.Fatalf("TranslatePermission 的错误应当能被 errors.Is(err, ErrTierNotOffered) 认出，实际: %v", err)
	}
}
