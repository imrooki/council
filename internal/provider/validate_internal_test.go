package provider

// validate_internal_test.go 是本包唯一的白盒测试文件（package provider，
// 不是 provider_test）：它存在的唯一理由是 validateRequest 未导出，而
// 11c 契约第 8.1 节验收第 2 条要测的正是它本身的 fail-closed 行为——
// 用一个真实适配器测不出来，三家登记的 Capabilities 里 WriteBoundary
// 从来不是零值（TestRegistry_CapabilitiesDeclareEveryField 已经钉死这
// 一点），没有一条自然路径能让 validateRequest 看到 WriteBoundary ==
// SupportUnknown。这里手搓一份假 Capabilities 直接调它。

import (
	"errors"
	"testing"
)

// TestValidateRequest_ZeroWriteBoundaryFailsClosedForWorkspace 覆盖
// 11c 契约第 8.1 节验收第 2 条：构造一份 Permissions 列了 workspace、
// 但 WriteBoundary 留零值（SupportUnknown，即「漏填」）的假 Capabilities，
// validateRequest 必须拒绝 workspace 请求，而不是把「没实测」当成
// 「支持」放行。
//
// 复用 ErrUnsupportedTier：这道闸与「本家没有这个档」共用同一个哨兵，
// 差别只在错误文本（provider.go validateRequest 的注释）。
func TestValidateRequest_ZeroWriteBoundaryFailsClosedForWorkspace(t *testing.T) {
	caps := Capabilities{
		// 故意列出 workspace，但不设 WriteBoundary：零值就是 SupportUnknown。
		Permissions: []PermissionTier{PermissionRead, PermissionWorkspace},
	}
	req := TurnRequest{
		WorkDir:    `C:\fake\workdir`,
		Permission: PermissionWorkspace,
	}

	err := validateRequest(caps, req)
	if err == nil {
		t.Fatal("WriteBoundary 是零值（未声明）时请求 workspace 档必须被拒绝，实际放行")
	}
	if !errors.Is(err, ErrUnsupportedTier) {
		t.Fatalf("err = %v, want ErrUnsupportedTier", err)
	}
}

// TestValidateRequest_ZeroWriteBoundaryFailsClosedForFull 覆盖同一条规则
// 对 FULL 档同样成立：11c 契约第 3.2 节的闸写的是
// "tier 属于 {WORKSPACE, FULL} 且 WriteBoundary != SupportYes"，不是只挡
// workspace 一档。
func TestValidateRequest_ZeroWriteBoundaryFailsClosedForFull(t *testing.T) {
	caps := Capabilities{
		Permissions: []PermissionTier{PermissionRead, PermissionFull},
	}
	req := TurnRequest{
		WorkDir:    `C:\fake\workdir`,
		Permission: PermissionFull,
	}

	err := validateRequest(caps, req)
	if err == nil {
		t.Fatal("WriteBoundary 是零值时请求 full 档必须被拒绝，实际放行")
	}
	if !errors.Is(err, ErrUnsupportedTier) {
		t.Fatalf("err = %v, want ErrUnsupportedTier", err)
	}
}

// TestValidateRequest_WriteBoundaryYesAllowsWorkspace 是上面两条的反例：
// WriteBoundary 显式声明 SupportYes 且 Permissions 列了 workspace 时，
// 请求必须放行（走到本函数末尾的 nil），证明这道闸真的是按 WriteBoundary
// 的值判断，不是无条件拒绝 workspace/full。
func TestValidateRequest_WriteBoundaryYesAllowsWorkspace(t *testing.T) {
	caps := Capabilities{
		Permissions:   []PermissionTier{PermissionRead, PermissionWorkspace},
		WriteBoundary: SupportYes,
		Prompt:        PromptTransportStdin, // 避免另一道 Unknown 能力闸干扰本用例
	}
	req := TurnRequest{
		WorkDir:    `C:\fake\workdir`,
		Permission: PermissionWorkspace,
	}

	if err := validateRequest(caps, req); err != nil {
		t.Fatalf("WriteBoundary=SupportYes 且 Permissions 列了 workspace 时应当放行，实际: %v", err)
	}
}
