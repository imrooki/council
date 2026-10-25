package store

// workspace_move.go 只做一件事：把工作区与它在左栏里的相邻者交换
// sort_order（B1）。这是 `sort_order` 唯一会被改写的地方。
//
// 为什么不放在 conversation.Tx 接口上：与 workspace.go 文件头同一个理由
// （只有 serve 的 move 端点会调它，扩接口会逼 turn / repl 的假实现补一个
// 永远不用的方法）。这里用 workspaceMoveTx 把「只有本包认识它」写在类型上。
//
// 为什么只交换相邻两行、不做自由拖动排序：按位置插入要重排整列
// sort_order，收益与复杂度不成比例（docs/29_FIXES_PLAN.md B1）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"council/internal/conversation"
)

// MoveWorkspace 把工作区 id 与它在 (sort_order, name) 顺序里的相邻者交换
// sort_order。up 为 true 是与前一个交换，false 是与后一个交换。
//
// 返回的 moved 表示这次调用是否真的改变了顺序：
//   - 已经在最前 / 最后 → false, nil（HTTP 层据此回 200 + moved:false，
//     调用方不刷新列表）；
//   - 相邻两行的 sort_order 相同 → false, nil。正常情况下 CreateWorkspace
//     分配 MAX+1，sort_order 不会重复；只有绕过本包直插的老数据才会撞上。
//     sort_order 相等时交换两个相同的数不会改变任何一行，报 true 是撒谎。
//
// 两条 UPDATE 与「找相邻行」在同一个 BEGIN IMMEDIATE 写事务里：分开提交时，
// 两个并发的 move 请求会各自基于同一份旧顺序去交换，最后可能把一个组
// 移丢或让两行拿到同一个 sort_order。工作区不存在返回包了
// ErrWorkspaceNotFound 的错误（HTTP 映射 404，与 rename / delete 一致）。
func (s *Store) MoveWorkspace(ctx context.Context, id string, up bool) (bool, error) {
	var moved bool
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(workspaceMoveTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持工作区排序", tx)
		}
		var err error
		moved, err = impl.moveWorkspace(ctx, id, up)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("move workspace %s: %w", id, err)
	}
	return moved, nil
}

// workspaceMoveTx 是 MoveWorkspace 在 WriteTx 里需要的最小事务能力。
type workspaceMoveTx interface {
	moveWorkspace(ctx context.Context, id string, up bool) (bool, error)
}

// moveWorkspace 是 MoveWorkspace 的事务内动作。
//
// 相邻行按 (sort_order, name) 找，与 Workspaces 的 ORDER BY 同序：只按
// sort_order 会让并列行的先后变成未定义，而「相邻」必须和用户看到的顺序
// 一致。两个 UPDATE 都影响恰好一行；任何一行没被更新到都说明事务读到的
// 顺序在写之前变了，返回错误整体回滚。
func (t *Tx) moveWorkspace(ctx context.Context, id string, up bool) (bool, error) {
	current, err := t.workspaceRow(ctx, id)
	if err != nil {
		return false, err
	}

	var neighborID string
	var neighborOrder int
	var query string
	if up {
		query = `SELECT id, sort_order FROM workspaces
			WHERE sort_order < ? OR (sort_order = ? AND name < ?)
			ORDER BY sort_order DESC, name DESC LIMIT 1`
	} else {
		query = `SELECT id, sort_order FROM workspaces
			WHERE sort_order > ? OR (sort_order = ? AND name > ?)
			ORDER BY sort_order ASC, name ASC LIMIT 1`
	}
	err = t.tx.QueryRowContext(ctx, query,
		current.SortOrder, current.SortOrder, current.Name,
	).Scan(&neighborID, &neighborOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find neighbor of workspace %s: %w", id, err)
	}

	// sort_order 相等时交换是无操作。正常路径不会出现，直插的老数据可能。
	if neighborOrder == current.SortOrder {
		return false, nil
	}

	for _, pair := range [][2]any{
		{neighborOrder, id},
		{current.SortOrder, neighborID},
	} {
		res, err := t.tx.ExecContext(ctx,
			`UPDATE workspaces SET sort_order = ? WHERE id = ?`, pair[0], pair[1])
		if err != nil {
			return false, fmt.Errorf("swap workspace sort_order %s <-> %s: %w", id, neighborID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return false, fmt.Errorf("swap workspace sort_order %s <-> %s: %w", id, neighborID, err)
		}
		if n != 1 {
			return false, fmt.Errorf("swap workspace sort_order %s <-> %s: 更新了 %d 行", id, neighborID, n)
		}
	}
	return true, nil
}
