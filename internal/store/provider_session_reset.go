package store

// provider_session_reset.go 是「按成员重置 provider 会话」的唯一实现。
//
// 它和 Tx.DropProviderSession 不是同一件事：那个删的是一条会话里的一员
// （/session rebuild 用），这个删的是一个成员在**全库所有会话**上的记录。
// 设置页热换身份字段（display_name / role / provider / model / effort /
// prompt / permissions）时，旧记录在每一条会话里都会算出与新配置不符的
// 身份指纹：进程内还有内存快照的那条会直接 ErrIdentityDrift 拒绝发言，
// 只留在库里的那条会在下一轮读回时静默作废重建。只清当前会话不够，
// 必须一次清干净。
//
// 为什么放在 Store 上而不是 conversation.Store / Tx 接口上：
//   - conversation.Tx 是「记一轮」那四张表的事务边界，为它加一个跨会话的
//     DELETE 会把接口变成 CRUD 清单，且所有假实现都要跟着改；
//   - 这是设置页专用的维护动作，调用方（internal/serve）手里本来就是
//     具体的 *store.Store，不需要经过领域层。
//
// 单条 DELETE 在 SQLite 里本身就是原子的，这里仍然显式开一个写事务：
// 「同一事务」这件事要写在结构里而不是靠「这条 SQL 刚好只有一句」的
// 巧合——将来若要在删除前后补一次读或计数，事务边界已经在了。

import (
	"context"
	"errors"
	"fmt"
)

// errResetProviderSessionsEmptyAgent 挡住空 id：空串会让 DELETE 变成
// 「按 agent_id 为空串删」，虽然今天不可能匹配到任何行，但它意味着调用方
// 传错了一个变量，静默返回 0 行比报错更难查。
var errResetProviderSessionsEmptyAgent = errors.New("reset provider sessions: empty agent id")

// ResetProviderSessions 删掉 agentID 在全库所有会话上的 provider_sessions
// 记录，返回删掉的行数。
//
// 只删 provider_sessions 这一张表：messages / cursors / turn_usage /
// inflight_turns 一条都不动。历史发言与账目是只追加的事实源，重置会话
// 不等于抹掉这个人说过的话；成本基线跟着那一行一起消失是刻意的——重建
// 意味着 provider 那边计数器从零开始，留着旧基线会让下一轮的差值是负数。
//
// 行本来就不存在时返回 (0, nil)：这个成员只是还没开过口，不是错误。
//
// 调用方必须先确认没有轮次在飞：这里没有、也不该有「在飞闸」。在飞那一轮
// 结束时的 SetProviderSession 是 upsert，会把刚删掉的行按旧身份写回来，
// 于是删除看起来成功了、下一轮照样撞指纹；那道判断在 internal/serve 的
// reloadMu + runningCount 上（settingsHotReload）。
func (s *Store) ResetProviderSessions(ctx context.Context, agentID string) (int, error) {
	if agentID == "" {
		return 0, errResetProviderSessionsEmptyAgent
	}
	if s.write == nil {
		// 只读打开（OpenReadOnly）的 Store 没有写句柄。这里不假装删了 0 行：
		// 那会让调用方以为「本来就没有会话」，而真相是这次重置根本没执行。
		return 0, errors.New("reset provider sessions: store is read-only")
	}

	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("reset provider sessions %s: begin write tx: %w", agentID, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, `DELETE FROM provider_sessions WHERE agent_id = ?`, agentID)
	if err != nil {
		return 0, fmt.Errorf("reset provider sessions %s: %w", agentID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reset provider sessions %s: rows affected: %w", agentID, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("reset provider sessions %s: commit: %w", agentID, err)
	}
	committed = true
	return int(n), nil
}
