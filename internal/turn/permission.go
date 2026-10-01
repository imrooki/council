package turn

import (
	"fmt"

	"council/internal/config"
	"council/internal/provider"
)

// TranslatePermission 把 config.Permission 翻译成 provider.PermissionTier。
//
// 11c 契约第 3.3 节：read 与 workspace 都放行，只有 full（以及拼错的档）
// 在起进程之前被拒绝，返回 ErrTierNotOffered。这是 06 第 2 节定的编排策略
// （「FULL 档在 MVP 里不提供给编排器自动使用」），不是 provider 能力问题——
// 判「这家 provider 围不围得住 workspace」是 provider.validateRequest 那道
// 独立的闸，本函数只管「编排器这一层要不要把这个档放出去」，两道闸并存，
// 拦的不是同一件事。workspace 曾经在这里被一刀切拒绝（旧哨兵名
// ErrTierNotInStep6），那是 Step 6 的临时收紧，Step 8 第二片按 06 第 2 节
// 那张实测过的写入边界表把它解开，改动记录见 09a 里的订正。
func TranslatePermission(p config.Permission) (provider.PermissionTier, error) {
	switch p {
	case config.PermissionRead:
		return provider.PermissionRead, nil
	case config.PermissionWorkspace:
		return provider.PermissionWorkspace, nil
	case config.PermissionFull:
		// 不返回 provider.PermissionFull 再附带错误：调用方若忽略 error 就会
		// 拿着 full 去 BuildTurn。空档 + 哨兵，fail-closed。
		return "", fmt.Errorf("permission %q: %w", p, ErrTierNotOffered)
	default:
		// 空串、拼错、将来多出来的档，一律拒绝。降档成 read 看起来能跑，
		// 但配置写的是另一档、实际按只读跑，行为与用户预期相反且不报错。
		return "", fmt.Errorf("permission %q: %w", p, ErrTierNotOffered)
	}
}
