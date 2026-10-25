package store

// workspace.go 是 schema v9 的工作区（纯分组，不绑定目录）。
//
// 这一组方法刻意不写进 conversation.Store / conversation.Tx 两个接口：
// 改工作区目前只有 serve 的四个端点会调用，而扩接口会逼 internal/turn 与
// internal/repl 里每个假 Store+Tx 都补一个它们永远不会调用的方法。
// 与 renameTx / softDeleteTx / archiveTx 同一个理由，这里用 workspaceTx
// 把「只有本包认识它」写在类型上。
//
// 归属（conversations.workspace_id）是展示用的分组，不是会话的生存条件：
//   - 删工作区把成员的归属置 NULL，会话、消息、成员、用量一条都不删；
//   - 归属与归档正交：归档只换列表，不改变归属；
//   - 归属与 cwd 正交：一个工作区里的会话可以来自任意目录。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"council/internal/conversation"
)

// 工作区相关的哨兵错误。调用方一律用 errors.Is 判断，实现必须用 %w 包装，
// 不要让调用方去匹配错误字符串（与 conversation 包里那组哨兵同一纪律）。
var (
	// ErrWorkspaceNotFound 表示按 id 找不到工作区。serve 映射 404。
	ErrWorkspaceNotFound = errors.New("workspace not found")

	// ErrWorkspaceNameTaken 表示名字已被另一个工作区占用。
	//
	// 判重口径是「去空白之后、Unicode 大小写折叠后相同」：`Foo` 与 `foo`、
	// ` 工作区 ` 与 `工作区`、`İstanbul` 与 `istanbul` 都是同一个名字；
	// 全角 `Ａ` 与半角 `A` 不是（不做兼容折叠）。完整边界写在
	// requireWorkspaceNameFree 的注释里。
	//
	// 哨兵单独一个而不是复用 ErrInvalidWorkspaceName：名字合法但重名要
	// 映射 409，名字不合法映射 400，两件事在 HTTP 上不同。
	ErrWorkspaceNameTaken = errors.New("workspace name already taken")

	// ErrInvalidWorkspaceName 表示名字去空白之后不是 1–60 个字符。
	ErrInvalidWorkspaceName = errors.New("invalid workspace name")
)

// MaxWorkspaceNameChars 是工作区名字符数上限，按 rune 数而不是字节数：
// 一个汉字是 3 个字节，按字节算会让「30 个汉字的名字」被误拒。
// 60 个字符已经远超左栏一行能显示的长度，再长的内容用户也没法核对改对了没有。
//
// 导出它是因为 serve 的 400 文案要点名同一个数字。规则本身只在
// normalizeWorkspaceName 里实现一次，serve 那一道只是提前给出一条更具体的
// 说明，绝不替代这里的校验。
const MaxWorkspaceNameChars = 60

// Workspaces 列出全部工作区，按 sort_order 升序、name 升序。空结果是空切片不是 nil。
//
// 排序的第二键必须是 name：sort_order 相同的行（例如全是 DDL 默认值 0 的
// 老库）若只按第一键排，SQLite 给出的顺序是未定义的，同一份库两次请求可能
// 不一样，前端左栏会自己跳。
//
// ConversationCount 是每个工作区的会话数，口径：
//   - 已软删的会话不计（deleted_at 非 NULL 的会话对外不存在）；
//   - 已归档的会话照计（归档只换列表，不把会话移出分组）。
//
// 用相关子查询而不是 LEFT JOIN + GROUP BY：GROUP BY 版本要额外判 NULL 组，
// 而这里只要每张表一行；工作区数量是「用户在左栏里手动建的那些」的量级。
func (s *Store) Workspaces(ctx context.Context) ([]conversation.Workspace, error) {
	// v9 之前没有 workspaces 表。只读看板打开 v7/v8 旧库时，左栏应当按
	// 「全部未分组」画，而不是让 GET /api/workspaces 回 500、连带整次
	// refreshConversations 失败（前端把分组与会话列表放在同一个 Promise.all 里）。
	// 空切片不是 nil，与本文件其余 List 方法的约定一致。
	if !s.shape.workspaces {
		return []conversation.Workspace{}, nil
	}
	rows, err := s.read.QueryContext(ctx, `
SELECT w.id, w.name, w.sort_order, w.created_at,
       (SELECT COUNT(*) FROM conversations c WHERE c.workspace_id = w.id AND c.deleted_at IS NULL)
FROM workspaces w
ORDER BY w.sort_order ASC, w.name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	out := make([]conversation.Workspace, 0)
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, fmt.Errorf("list workspaces: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return out, nil
}

// CreateWorkspace 新建一个工作区。名字去空白后必须是 1–60 个字符，
// 且与现存工作区不重名（Unicode 大小写折叠，边界见
// requireWorkspaceNameFree）。返回补全后的 Workspace，其中
// ConversationCount 恒为 0（新建的工作区里还没有会话）。
//
// sort_order 取 MAX(sort_order)+1，让新工作区排在左栏末尾。DDL 上的
// DEFAULT 0 只在绕过本方法直插时生效；本方法显式写值，因为「新建的组排在
// 最后」是用户按创建顺序看到的顺序，而不是名字字典序。
//
// 判重与插入在同一个写事务里（BEGIN IMMEDIATE）：先 SELECT 再 INSERT
// 在事务外会有一个可乘之窗，两个并发同名请求会都插进去，而 DDL 上并没有
// 唯一索引（理由见 schema.go 的 v9DDL 注释）。
func (s *Store) CreateWorkspace(ctx context.Context, name string) (conversation.Workspace, error) {
	normalized, err := normalizeWorkspaceName(name)
	if err != nil {
		return conversation.Workspace{}, err
	}

	var created conversation.Workspace
	err = s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(workspaceTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持工作区", tx)
		}
		var err error
		created, err = impl.createWorkspace(ctx, normalized)
		return err
	})
	if err != nil {
		return conversation.Workspace{}, fmt.Errorf("create workspace %q: %w", normalized, err)
	}
	return created, nil
}

// RenameWorkspace 改工作区的名字。名字规则与 CreateWorkspace 相同
// （Unicode 大小写折叠判重），判重时排除自己：把一个工作区改成它现在的
// 名字（哪怕大小写不同）不算重名。
//
// 只改 name 一列：sort_order、created_at 与成员的归属都不动，改个名字不是
// 一次工作区的重新定义。返回改完之后的 Workspace（含真实 ConversationCount），
// 让 HTTP 层不必自己再拼一遍去空白后的名字。
//
// 工作区不存在返回包了 ErrWorkspaceNotFound 的错误。
func (s *Store) RenameWorkspace(ctx context.Context, id, name string) (conversation.Workspace, error) {
	normalized, err := normalizeWorkspaceName(name)
	if err != nil {
		return conversation.Workspace{}, err
	}

	var renamed conversation.Workspace
	err = s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(workspaceTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持工作区", tx)
		}
		var err error
		renamed, err = impl.renameWorkspace(ctx, id, normalized)
		return err
	})
	if err != nil {
		return conversation.Workspace{}, fmt.Errorf("rename workspace %s: %w", id, err)
	}
	return renamed, nil
}

// DeleteWorkspace 删掉一个工作区，并在**同一个事务**里把它名下所有会话的
// workspace_id 置回 NULL。
//
// 为什么必须同一事务：两条语句分开提交时，中间任何一次读都会看到「工作区
// 已经不在了、会话还指着一个不存在的 id」的中间态。前端那一刻刷一下左栏，
// 这些会话就会从任何分组里掉出来，而分组本身已经没了 —— 既看不见、也修不回来。
//
// 置 NULL 的范围不筛 deleted_at：已软删的会话也曾经属于这个组，留着悬空
// 引用只会在以后某次恢复里变成一个指向不存在工作区的归属。会话行本身一条
// 都不删 —— 工作区是展示用的分组，删组不等于删会话。
//
// 工作区不存在返回包了 ErrWorkspaceNotFound 的错误（不幂等：删一个不存在的
// 组就是调用方拿错了 id，HTTP 上映射 404，与「已删第二次」共用一个哨兵）。
func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(workspaceTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持工作区", tx)
		}
		return impl.deleteWorkspace(ctx, id)
	})
	if err != nil {
		return fmt.Errorf("delete workspace %s: %w", id, err)
	}
	return nil
}

// SetConversationWorkspace 改一场会话的归属。workspaceID 为 nil 或指向空串
// 表示移出分组（写 NULL）；非空时目标工作区必须存在。
//
// 空串与 nil 是同一个意思：workspace_id 列上 NULL 读作「未分组」，没有
// 第二种含义，所以不需要像 SetConversationTeam 那样拒绝空串解绑。
// 反方向——把一个不存在的 id 写进去——必须拒绝：归属里出现一个查不到的名字，
// 前端只能把这场会话画成一个空分组。
//
// 已软删的会话按不存在处理（返回包了 conversation.ErrConversationNotFound
// 的错误）：它对外已经不存在，不能经工作区这条路重新露出来。已归档的会话
// 正常放行：归档与归属正交。
func (s *Store) SetConversationWorkspace(ctx context.Context, convID string, workspaceID *string) error {
	target := ""
	if workspaceID != nil {
		target = strings.TrimSpace(*workspaceID)
	}
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(workspaceTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持工作区", tx)
		}
		return impl.setConversationWorkspace(ctx, convID, target)
	})
	if err != nil {
		return fmt.Errorf("set conversation workspace %s: %w", convID, err)
	}
	return nil
}

// workspaceTx 是工作区四个动作在 WriteTx 里需要的最小事务能力。
// 与 renameTx / softDeleteTx / archiveTx 同一个理由，见文件头。
type workspaceTx interface {
	createWorkspace(ctx context.Context, name string) (conversation.Workspace, error)
	renameWorkspace(ctx context.Context, id, name string) (conversation.Workspace, error)
	deleteWorkspace(ctx context.Context, id string) error
	setConversationWorkspace(ctx context.Context, convID, workspaceID string) error
}

// normalizeWorkspaceName 是名字规则的唯一实现：去空白后 1–60 个 rune。
// 返回规范化之后的名字，调用方存的是它，不是原始入参：
// 「 工作区 」与「工作区」若一个存原样、一个存去空白，判重就永远判不出来。
func normalizeWorkspaceName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("workspace name %q: %w（去空白后不能为空）", name, ErrInvalidWorkspaceName)
	}
	if n := utf8.RuneCountInString(trimmed); n > MaxWorkspaceNameChars {
		return "", fmt.Errorf("workspace name: %w（最长 %d 个字符，实际 %d 个）",
			ErrInvalidWorkspaceName, MaxWorkspaceNameChars, n)
	}
	return trimmed, nil
}

// createWorkspace 是 CreateWorkspace 的事务内动作。
func (t *Tx) createWorkspace(ctx context.Context, name string) (conversation.Workspace, error) {
	if err := t.requireWorkspaceNameFree(ctx, name, ""); err != nil {
		return conversation.Workspace{}, err
	}

	id, err := newID()
	if err != nil {
		return conversation.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	var next int
	if err := t.tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM workspaces`,
	).Scan(&next); err != nil {
		return conversation.Workspace{}, fmt.Errorf("create workspace: 分配 sort_order: %w", err)
	}
	createdAt := formatTime(time.Now().UTC())
	parsed, err := parseTime(createdAt)
	if err != nil {
		return conversation.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	if _, err := t.tx.ExecContext(ctx,
		`INSERT INTO workspaces (id, name, sort_order, created_at) VALUES (?, ?, ?, ?)`,
		id, name, next, createdAt,
	); err != nil {
		return conversation.Workspace{}, fmt.Errorf("insert workspace: %w", err)
	}
	return conversation.Workspace{
		ID:                id,
		Name:              name,
		SortOrder:         next,
		CreatedAt:         parsed,
		ConversationCount: 0,
	}, nil
}

// renameWorkspace 是 RenameWorkspace 的事务内动作。
func (t *Tx) renameWorkspace(ctx context.Context, id, name string) (conversation.Workspace, error) {
	ws, err := t.workspaceRow(ctx, id)
	if err != nil {
		return conversation.Workspace{}, err
	}
	if err := t.requireWorkspaceNameFree(ctx, name, id); err != nil {
		return conversation.Workspace{}, err
	}
	if _, err := t.tx.ExecContext(ctx,
		`UPDATE workspaces SET name = ? WHERE id = ?`, name, id,
	); err != nil {
		return conversation.Workspace{}, fmt.Errorf("update workspace name %s: %w", id, err)
	}
	ws.Name = name
	if ws.ConversationCount, err = t.workspaceConversationCount(ctx, id); err != nil {
		return conversation.Workspace{}, err
	}
	return ws, nil
}

// deleteWorkspace 是 DeleteWorkspace 的事务内动作，两条语句同生共死。
func (t *Tx) deleteWorkspace(ctx context.Context, id string) error {
	if _, err := t.workspaceRow(ctx, id); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx,
		`UPDATE conversations SET workspace_id = NULL WHERE workspace_id = ?`, id,
	); err != nil {
		return fmt.Errorf("clear conversations workspace_id %s: %w", id, err)
	}
	if _, err := t.tx.ExecContext(ctx,
		`DELETE FROM workspaces WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("delete workspace %s: %w", id, err)
	}
	return nil
}

// setConversationWorkspace 是 SetConversationWorkspace 的事务内动作。
// workspaceID 已经由外层去空白，空串表示移出分组。
//
// 先读 deleted_at 再 UPDATE，而不是直接 UPDATE 看 RowsAffected：
// 那样分不开「行不存在」与「已软删」，而这两件事在这里都是 404，
// 却也都必须是「没有写进去」。多一次主键查询换来的是不写的确定性。
func (t *Tx) setConversationWorkspace(ctx context.Context, convID, workspaceID string) error {
	var deletedAt sql.NullString
	err := t.tx.QueryRowContext(ctx,
		`SELECT deleted_at FROM conversations WHERE id = ?`, convID,
	).Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("conversation %s: %w", convID, conversation.ErrConversationNotFound)
	}
	if err != nil {
		return fmt.Errorf("check conversation %s: %w", convID, err)
	}
	if deletedAt.Valid {
		return fmt.Errorf("conversation %s: %w", convID, conversation.ErrConversationNotFound)
	}

	if workspaceID == "" {
		if _, err := t.tx.ExecContext(ctx,
			`UPDATE conversations SET workspace_id = NULL WHERE id = ?`, convID,
		); err != nil {
			return fmt.Errorf("clear conversation workspace %s: %w", convID, err)
		}
		return nil
	}

	var dummy string
	err = t.tx.QueryRowContext(ctx,
		`SELECT id FROM workspaces WHERE id = ?`, workspaceID,
	).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("workspace %s: %w", workspaceID, ErrWorkspaceNotFound)
	}
	if err != nil {
		return fmt.Errorf("check workspace %s: %w", workspaceID, err)
	}
	if _, err := t.tx.ExecContext(ctx,
		`UPDATE conversations SET workspace_id = ? WHERE id = ?`, workspaceID, convID,
	); err != nil {
		return fmt.Errorf("set conversation workspace %s: %w", convID, err)
	}
	return nil
}

// requireWorkspaceNameFree 判定名字有没有被占用。excludeID 非空时排除它自己
// （改名的场景：把工作区改成它现在的名字——哪怕只改大小写——不算重名）。
//
// 判重不用 SQL 的 COLLATE NOCASE：那个只折 ASCII，能挡住 `Work` 与 `work`，
// 挡不住 `İstanbul` 与 `istanbul`，也挡不住希腊/西里尔字母的大小写。这里在
// 写事务里把现有名字全读出来，用 Go 的字符串比较：
//
//   - 两边都先 strings.TrimSpace，与 normalizeWorkspaceName 存下来的形态对齐；
//   - strings.EqualFold 做 Unicode 简单大小写折叠：`Café`/`CAFÉ` 相等，
//     `K`（U+212A KELVIN SIGN）与 `k` 相等；
//   - 再用 strings.ToLower 兜底一次：少数大写字符没有简单折叠，但能折成
//     单个小写 rune。典型是 U+0130 `İ`（LATIN CAPITAL LETTER I WITH DOT
//     ABOVE）：EqualFold("İstanbul","istanbul") 为 false，而两者的 ToLower
//     都是 "istanbul"，所以本实现判为重名。
//
// 已知边界（不要读成「完整 Unicode 大小写不敏感」）：
//
//   - 不做 Unicode 规范化：预组合的 `é`（U+00E9）与 `e` + U+0301 是两个名字；
//   - 不做兼容折叠（NFKC/NFKD）：全角 `Ａ`（U+FF21）与半角 `A` 不相等；
//   - 不做变音符号剥离：`café` 与 `cafe` 是两个名字（只折大小写，
//     `café` 与 `CAFÉ` 相等）；
//   - 不展开 ß、ﬁ 这类需要全量 case folding 的字符：`Straße` 与 `STRASSE`
//     不相等；
//   - strings.ToLower 是语言无关的逐 rune 映射，不是土耳其语区域规则
//     （土耳其语里 İ→i、I→ı 是两套不同规则；这里只按 Unicode 默认映射走）。
//
// 中文没有大小写，逐字相等即同名，不受上述折叠影响。
//
// 读全表而不是一条 SELECT ... WHERE：现有工作区是「用户在左栏里手动建的
// 那些」的量级，几十行；换来的是判重口径只由 Go 这一处决定，不会出现
// SQLite 的 NOCASE 与 Go 的 EqualFold 各说各话。判重与插入仍在同一个写事务
// （BEGIN IMMEDIATE）里，并发同名请求不会都插进去（见 CreateWorkspace）。
func (t *Tx) requireWorkspaceNameFree(ctx context.Context, name, excludeID string) error {
	rows, err := t.tx.QueryContext(ctx, `SELECT id, name FROM workspaces`)
	if err != nil {
		return fmt.Errorf("check workspace name %q: %w", name, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, existing string
		if err := rows.Scan(&id, &existing); err != nil {
			return fmt.Errorf("check workspace name %q: %w", name, err)
		}
		if excludeID != "" && id == excludeID {
			continue
		}
		if workspaceNamesEqual(name, existing) {
			return fmt.Errorf("workspace name %q（与现有工作区的名字 %q 折叠后相同）: %w",
				name, existing, ErrWorkspaceNameTaken)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("check workspace name %q: %w", name, err)
	}
	return nil
}

// workspaceNamesEqual 是工作区判重的唯一比较函数。
//
// 调用方传进来的 name 已经由 normalizeWorkspaceName 去过空白；existing 来自
// 库，正常路径也是去空白之后的形态，但直插的行可能带空白，所以这里两边
// 都再 trim 一次。比较顺序是先 EqualFold 再 ToLower 兜底，语义与边界见
// requireWorkspaceNameFree 的注释。
func workspaceNamesEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b || strings.EqualFold(a, b) {
		return true
	}
	return strings.ToLower(a) == strings.ToLower(b)
}

// workspaceRow 按 id 读一行工作区（不含会话数），不存在时返回包了
// ErrWorkspaceNotFound 的错误。
func (t *Tx) workspaceRow(ctx context.Context, id string) (conversation.Workspace, error) {
	var ws conversation.Workspace
	var createdAt string
	err := t.tx.QueryRowContext(ctx,
		`SELECT id, name, sort_order, created_at FROM workspaces WHERE id = ?`, id,
	).Scan(&ws.ID, &ws.Name, &ws.SortOrder, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Workspace{}, fmt.Errorf("workspace %s: %w", id, ErrWorkspaceNotFound)
	}
	if err != nil {
		return conversation.Workspace{}, fmt.Errorf("get workspace %s: %w", id, err)
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return conversation.Workspace{}, fmt.Errorf("workspace %s created_at: %w", id, err)
	}
	ws.CreatedAt = ts
	return ws, nil
}

// workspaceConversationCount 是 Count 的唯一实现（口径见 Workspaces 的注释），
// 给改名后的返回值用。
func (t *Tx) workspaceConversationCount(ctx context.Context, id string) (int, error) {
	var n int
	if err := t.tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM conversations WHERE workspace_id = ? AND deleted_at IS NULL`, id,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count conversations in workspace %s: %w", id, err)
	}
	return n, nil
}

func scanWorkspace(sc rowScanner) (conversation.Workspace, error) {
	var id, name, createdAt string
	var sortOrder, count int
	if err := sc.Scan(&id, &name, &sortOrder, &createdAt, &count); err != nil {
		return conversation.Workspace{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return conversation.Workspace{}, fmt.Errorf("workspace %s created_at: %w", id, err)
	}
	return conversation.Workspace{
		ID:                id,
		Name:              name,
		SortOrder:         sortOrder,
		CreatedAt:         ts,
		ConversationCount: count,
	}, nil
}
