package store

// localagent.go 是 v10 两张租约表（local_leases / local_lease_events）的
// 全部 SQL。按 Plan §4.3 ④ 的分工，SQL 只活在本包里；资源键规范化、TTL 上下限
// 这类策略在 internal/localagent/lease.go，两者都不碰对方的职责。
//
// 为什么租约只在**写事务**里改：本仓库的写句柄是单连接 + BEGIN IMMEDIATE
// （open.go:142-154），一个 data_dir 同时只有一个写进程（council.lock）。
// 于是「同一资源不能有两条 granted 的写租约」既有 SQLite 的部分唯一索引兜底，
// 又有单写者这一层串行化；不需要另造仲裁进程（Plan §8.3 明写这一点）。
//
// 过期不靠后台扫描：没有常驻循环（S-01），所以过期是在下一次 claim / release
// 碰这个资源时顺手改写的（expireLeasesFor）。这带来一个诚实但必须说清的
// 结果：一条租约到期后，库里可能还留着 state='granted'，直到有人再申请同一
// 资源。读入口因此按「过期即无效」判断，不把过期的行当成持有者。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"council/internal/conversation"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// LeaseState 是 local_leases.state 的取值。
//
// 没有写进 DDL 的 CHECK：Plan §8.3 要给等待队列留 state='queued'，而 SQLite
// 改不了 CHECK（只能重建表）。枚举在这里守。
type LeaseState string

const (
	// LeaseGranted 是「此刻持有」。同一个 resource_key 上最多只能有一条
	// access_mode='write' 的 granted 行（部分唯一索引），read 可以并存。
	LeaseGranted LeaseState = "granted"
	// LeaseReleased 是持有者主动交还。这一行留着，是因为「谁在什么时候
	// 占用过这个目录」是排查冲突时唯一有用的事实。
	LeaseReleased LeaseState = "released"
	// LeaseExpired 是过了 expires_at 之后被下一次申请/交还改写的状态。
	LeaseExpired LeaseState = "expired"
	// LeaseUncertain 留给「申请发出了、结果不明」的场景（L4 的跨进程协调）。
	// L2 不会产生它：本地事务要么提交要么回滚，没有中间态。
	LeaseUncertain LeaseState = "uncertain"
)

// LeaseAccess 是租约的读写模式。读共享、写独占（Plan §8.3）。
type LeaseAccess string

const (
	LeaseRead  LeaseAccess = "read"
	LeaseWrite LeaseAccess = "write"
)

// LeaseOwnerKind 是持有者的类别。它只用来展示与分组，**不参与任何鉴权**
// （Plan §9.2：agent_id 一类自述身份不用于权限判定）。
const (
	// LeaseOwnerManaged 是 Council 自己调度起来的成员进程。
	LeaseOwnerManaged = "managed"
	// LeaseOwnerRegistered 预留给将来自愿接入的外部 agent（v1 不实现，
	// Plan §4.4 第 2 条）。先定义好，避免以后改协议。
	LeaseOwnerRegistered = "registered"
	// LeaseOwnerCLI 是 `council claim` 手工或脚本申请的租约：身份自述，
	// 不校验。把它与 managed 分开，是为了让「这条租约是谁要的」一眼可读。
	LeaseOwnerCLI = "cli"
	// LeaseOwnerDashboard 是看板页面上点「申请」拿到的租约：申请人就是
	// 这个看板进程背后的用户。与 cli 分开的理由同上 —— 排查时一眼看得出
	// 这条是从窗口里点的还是从脚本里敲的。身份同样不参与鉴权（Plan §9.2）。
	LeaseOwnerDashboard = "dashboard"
)

// Lease 是 local_leases 的一行。
type Lease struct {
	ID          string
	ResourceKey string
	OwnerKind   string
	OwnerID     string
	Access      LeaseAccess
	// Generation 是同一 resource_key 上单调递增的代数。旧授权拿着一个更小的
	// generation，就不会被误当成新的那次授权（Plan §8.3）。
	Generation int64
	State      LeaseState
	GrantedAt  time.Time
	ExpiresAt  time.Time
	// HeartbeatAt 是最后一次心跳。L2 只在申请与续约时写它，没有后台心跳
	// 循环；TTL 到期就失效，不靠心跳续命。
	HeartbeatAt time.Time
}

// LeaseRequest 是一次申请。TTL 由调用方给（上限与下限在 localagent 包）。
type LeaseRequest struct {
	ResourceKey string
	OwnerKind   string
	OwnerID     string
	Access      LeaseAccess
	TTL         time.Duration
}

// LeaseEvent 是 local_lease_events 的一行。Detail 是原始 JSON 文本（默认
// "{}"）；本包不解释它，读的人自己解。
type LeaseEvent struct {
	ID      string
	LeaseID string
	At      time.Time
	Kind    string
	Detail  string
}

// ErrLeaseConflict 表示资源已被别人占着。用 errors.Is 判断，不要比字符串。
var ErrLeaseConflict = errors.New("lease conflict")

// ErrLeaseNotFound 表示库里没有这条租约。
var ErrLeaseNotFound = errors.New("lease not found")

// LeaseConflictError 带上持有者，让调用方能说清「谁拿着、到什么时候」。
// Unwrap 让 errors.Is(err, ErrLeaseConflict) 成立。
type LeaseConflictError struct {
	ResourceKey string
	Wanted      LeaseAccess
	Holder      Lease
}

func (e *LeaseConflictError) Error() string {
	// Holder 为空只在「插入撞上部分唯一索引、回读又扑空」这一条路径上出现
	// （本仓库单写者，正常到不了）。这时不能编一个持有者出来，只说结论。
	if e.Holder.ID == "" {
		return fmt.Sprintf("资源 %s 已被占用：没有拿到 %s 租约", e.ResourceKey, e.Wanted)
	}
	return fmt.Sprintf("资源 %s 已被占用：%s/%s 持有 %s 租约（lease %s，到 %s，generation %d）",
		e.ResourceKey, e.Holder.OwnerKind, e.Holder.OwnerID, e.Holder.Access,
		e.Holder.ID, formatLeaseTime(e.Holder.ExpiresAt), e.Holder.Generation)
}

func (e *LeaseConflictError) Unwrap() error { return ErrLeaseConflict }

// validLeaseAccess / validLeaseOwnerKind 是 DDL 没有写 CHECK 的那部分枚举
// （理由见 LeaseState 的注释）：写入前在 Go 侧守一遍，拒绝垃圾值。
func validLeaseAccess(a LeaseAccess) bool {
	return a == LeaseRead || a == LeaseWrite
}

func validLeaseOwnerKind(kind string) bool {
	switch kind {
	case LeaseOwnerManaged, LeaseOwnerRegistered, LeaseOwnerCLI, LeaseOwnerDashboard:
		return true
	default:
		return false
	}
}

// ClaimLease 在同一资源上申请一条租约。被占用时返回 *LeaseConflictError
// （errors.Is(err, ErrLeaseConflict) 为真），不返回租约。
//
// 整个申请是一个 BEGIN IMMEDIATE 事务：先把这个资源上已经过期的租约改写掉
// （否则部分唯一索引会把一条早该失效的写租约当成仍在持有），再判冲突，
// 再插入。generation 取该资源历史上的最大值 +1，跨 released / expired 行
// 一起算，所以旧授权不会因为新一代已经释放而「复活」。
func (s *Store) ClaimLease(ctx context.Context, req LeaseRequest) (Lease, error) {
	var lease Lease
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(leaseTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持租约", tx)
		}
		var err error
		lease, err = impl.claimLease(ctx, req, time.Now().UTC())
		return err
	})
	if err != nil {
		return Lease{}, fmt.Errorf("claim lease %s: %w", req.ResourceKey, err)
	}
	return lease, nil
}

// ReleaseLease 按 lease_id 交还租约。
//
// 返回的 lease 是交还之后这一行的真实状态；changed 表示这次调用是否真的
// 改了状态。已经 released / expired 的租约再交还一次不算错误（脚本重试是
// 正常行为），changed=false，也不写事件。
func (s *Store) ReleaseLease(ctx context.Context, leaseID string) (Lease, bool, error) {
	var (
		lease   Lease
		changed bool
	)
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(leaseTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持租约", tx)
		}
		var err error
		lease, changed, err = impl.releaseLeaseByID(ctx, leaseID, time.Now().UTC())
		return err
	})
	if err != nil {
		return Lease{}, false, fmt.Errorf("release lease %s: %w", leaseID, err)
	}
	return lease, changed, nil
}

// ReleaseLeasesByOwner 交出某个持有者在这个资源上的全部 granted 租约。
//
// 为什么要有这一条：忘记 lease_id 的脚本不该被迫去查库；但它必须自己说清
// 是谁，本方法**只**释放 owner_id 完全匹配的行 —— 没有 «force» 参数，
// 释放别人的租约不在本模块的能力里（Plan §8.2：自愿遵守，不强制接管）。
func (s *Store) ReleaseLeasesByOwner(ctx context.Context, resourceKey, ownerID string) ([]Lease, error) {
	var released []Lease
	err := s.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		impl, ok := tx.(leaseTx)
		if !ok {
			return fmt.Errorf("事务实现 %T 不支持租约", tx)
		}
		var err error
		released, err = impl.releaseLeasesByOwner(ctx, resourceKey, ownerID, time.Now().UTC())
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("release leases on %s for %s: %w", resourceKey, ownerID, err)
	}
	return released, nil
}

// GrantedLeases 返回**仍然有效**的租约（state='granted' 且未过期）。
//
// 过期的行不在这里现身：它们要等下一次 claim / release 碰这个资源时才会被
// 改写成 expired（没有后台循环，S-01）。这个读入口因此不写库 —— 用只读句柄
// 打开库的调用方也能安全调它。
func (s *Store) GrantedLeases(ctx context.Context) ([]Lease, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT `+leaseColumns+` FROM local_leases WHERE state = 'granted' ORDER BY resource_key`)
	if err != nil {
		return nil, fmt.Errorf("list granted leases: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	leases := make([]Lease, 0, 8)
	for rows.Next() {
		lease, err := scanLease(rows)
		if err != nil {
			return nil, err
		}
		if !lease.ExpiresAt.After(now) {
			continue
		}
		leases = append(leases, lease)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate granted leases: %w", err)
	}
	return leases, nil
}

// LeaseEvents 返回一条租约的历史（按时间升序）。
func (s *Store) LeaseEvents(ctx context.Context, leaseID string) ([]LeaseEvent, error) {
	rows, err := s.read.QueryContext(ctx,
		`SELECT id, lease_id, at, kind, detail FROM local_lease_events
		 WHERE lease_id = ? ORDER BY at, id`, leaseID)
	if err != nil {
		return nil, fmt.Errorf("list lease events %s: %w", leaseID, err)
	}
	defer rows.Close()

	events := make([]LeaseEvent, 0, 4)
	for rows.Next() {
		var (
			event LeaseEvent
			at    string
		)
		if err := rows.Scan(&event.ID, &event.LeaseID, &at, &event.Kind, &event.Detail); err != nil {
			return nil, fmt.Errorf("scan lease event of %s: %w", leaseID, err)
		}
		parsed, err := parseTime(at)
		if err != nil {
			return nil, err
		}
		event.At = parsed
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lease events %s: %w", leaseID, err)
	}
	return events, nil
}

// leaseTx 是 ClaimLease / Release* 在 WriteTx 里需要的最小事务能力。
// 与 workspaceMoveTx 同一体例：把「只有本包认识它」写在类型上，不扩
// conversation.Tx 的接口（那会逼所有假实现补三个永远不用的方法）。
type leaseTx interface {
	claimLease(ctx context.Context, req LeaseRequest, now time.Time) (Lease, error)
	releaseLeaseByID(ctx context.Context, leaseID string, now time.Time) (Lease, bool, error)
	releaseLeasesByOwner(ctx context.Context, resourceKey, ownerID string, now time.Time) ([]Lease, error)
}

// leaseColumns 是 Lease 的列顺序，查询与 scanLease 必须同序。
const leaseColumns = `lease_id, resource_key, owner_kind, owner_id, access_mode,
	generation, state, granted_at, expires_at, heartbeat_at`

func scanLease(row interface {
	Scan(dest ...any) error
}) (Lease, error) {
	var (
		lease                             Lease
		access, state                     string
		grantedAt, expiresAt, heartbeatAt string
	)
	if err := row.Scan(&lease.ID, &lease.ResourceKey, &lease.OwnerKind, &lease.OwnerID,
		&access, &lease.Generation, &state, &grantedAt, &expiresAt, &heartbeatAt); err != nil {
		return Lease{}, fmt.Errorf("scan lease: %w", err)
	}
	lease.Access = LeaseAccess(access)
	lease.State = LeaseState(state)

	var err error
	if lease.GrantedAt, err = parseTime(grantedAt); err != nil {
		return Lease{}, err
	}
	if lease.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return Lease{}, err
	}
	if lease.HeartbeatAt, err = parseTime(heartbeatAt); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

// formatLeaseTime 是租约时间在错误文案里的样子：秒级 UTC，不显示纳秒
// （纳秒对「到什么时候」没有意义，只会让人读不下去）。
func formatLeaseTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// claimLease 是 ClaimLease 的事务内动作。
func (t *Tx) claimLease(ctx context.Context, req LeaseRequest, now time.Time) (Lease, error) {
	key := strings.TrimSpace(req.ResourceKey)
	if key == "" {
		return Lease{}, errors.New("resource_key 不能为空")
	}
	if !validLeaseAccess(req.Access) {
		return Lease{}, fmt.Errorf("access_mode %q 不合法（可选 read / write）", req.Access)
	}
	if !validLeaseOwnerKind(req.OwnerKind) {
		return Lease{}, fmt.Errorf("owner_kind %q 不合法（可选 managed / registered / cli）", req.OwnerKind)
	}
	if strings.TrimSpace(req.OwnerID) == "" {
		return Lease{}, errors.New("owner_id 不能为空")
	}
	if req.TTL <= 0 {
		return Lease{}, fmt.Errorf("ttl 必须为正，实际 %s", req.TTL)
	}

	// 先把这个资源上已经过期的租约改写成 expired：部分唯一索引只看 state，
	// 不改写的话一条早该失效的写租约会把新申请挡在 SQLite 那一层。
	if err := t.expireLeasesFor(ctx, key, now); err != nil {
		return Lease{}, err
	}

	holder, err := t.grantedLeaseFor(ctx, key, req.Access)
	if err != nil {
		return Lease{}, err
	}
	if holder != nil {
		return Lease{}, &LeaseConflictError{ResourceKey: key, Wanted: req.Access, Holder: *holder}
	}

	var generation int64
	if err := t.tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(generation), 0) + 1 FROM local_leases WHERE resource_key = ?`, key,
	).Scan(&generation); err != nil {
		return Lease{}, fmt.Errorf("read lease generation of %s: %w", key, err)
	}

	id, err := newID()
	if err != nil {
		return Lease{}, err
	}
	lease := Lease{
		ID:          id,
		ResourceKey: key,
		OwnerKind:   req.OwnerKind,
		OwnerID:     req.OwnerID,
		Access:      req.Access,
		Generation:  generation,
		State:       LeaseGranted,
		GrantedAt:   now,
		ExpiresAt:   now.Add(req.TTL),
		HeartbeatAt: now,
	}
	if _, err := t.tx.ExecContext(ctx,
		`INSERT INTO local_leases (`+leaseColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lease.ID, lease.ResourceKey, lease.OwnerKind, lease.OwnerID, string(lease.Access),
		lease.Generation, string(lease.State), formatTime(lease.GrantedAt),
		formatTime(lease.ExpiresAt), formatTime(lease.HeartbeatAt),
	); err != nil {
		// 同资源上的写租约在上一步已经判过；走到这里只可能是另一种模式
		// 的租约在并发窗口里插了进来。把 SQLite 的唯一约束译成同一个哨兵，
		// 调用方不需要知道是 Go 判的还是索引判的。
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return Lease{}, &LeaseConflictError{ResourceKey: key, Wanted: req.Access}
		}
		return Lease{}, fmt.Errorf("insert lease on %s: %w", key, err)
	}

	if err := t.appendLeaseEvent(ctx, lease.ID, now, "granted", fmt.Sprintf(
		`{"owner_kind":%q,"owner_id":%q,"access_mode":%q,"generation":%d,"ttl_seconds":%d}`,
		lease.OwnerKind, lease.OwnerID, lease.Access, lease.Generation, int64(req.TTL/time.Second),
	)); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

// releaseLeaseByID 是 ReleaseLease 的事务内动作。
func (t *Tx) releaseLeaseByID(ctx context.Context, leaseID string, now time.Time) (Lease, bool, error) {
	lease, err := t.leaseRow(ctx, leaseID)
	if err != nil {
		return Lease{}, false, err
	}
	if lease.State != LeaseGranted {
		return lease, false, nil
	}
	// 这一行还挂在部分唯一索引上，但已经过期：按过期处理，不写 released
	// （它不是被持有者交还的，是自然失效的）。
	if !lease.ExpiresAt.After(now) {
		if err := t.markLeaseExpired(ctx, lease, now); err != nil {
			return Lease{}, false, err
		}
		lease.State = LeaseExpired
		return lease, false, nil
	}
	if err := t.markLeaseReleased(ctx, lease, now); err != nil {
		return Lease{}, false, err
	}
	lease.State = LeaseReleased
	return lease, true, nil
}

// releaseLeasesByOwner 是 ReleaseLeasesByOwner 的事务内动作。
// 只匹配 owner_id 完全相等的行；一条都没匹配到返回空切片，不报错
// （脚本重试也算正常）。
func (t *Tx) releaseLeasesByOwner(ctx context.Context, resourceKey, ownerID string, now time.Time) ([]Lease, error) {
	key := strings.TrimSpace(resourceKey)
	if key == "" {
		return nil, errors.New("resource_key 不能为空")
	}
	if strings.TrimSpace(ownerID) == "" {
		return nil, errors.New("owner_id 不能为空（本命令只释放自己的租约）")
	}
	if err := t.expireLeasesFor(ctx, key, now); err != nil {
		return nil, err
	}

	rows, err := t.tx.QueryContext(ctx,
		`SELECT `+leaseColumns+` FROM local_leases
		 WHERE state = 'granted' AND resource_key = ? AND owner_id = ?
		 ORDER BY granted_at`, key, ownerID)
	if err != nil {
		return nil, fmt.Errorf("find granted leases on %s for %s: %w", key, ownerID, err)
	}
	var found []Lease
	for rows.Next() {
		lease, err := scanLease(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		found = append(found, lease)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate granted leases on %s: %w", key, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close granted lease query: %w", err)
	}

	released := make([]Lease, 0, len(found))
	for _, lease := range found {
		if err := t.markLeaseReleased(ctx, lease, now); err != nil {
			return nil, err
		}
		lease.State = LeaseReleased
		released = append(released, lease)
	}
	return released, nil
}

// grantedLeaseFor 返回挡住这次申请的持有者，没有则返回 nil。
// read 只与 write 冲突，write 与任何 granted 租约冲突。
func (t *Tx) grantedLeaseFor(ctx context.Context, resourceKey string, want LeaseAccess) (*Lease, error) {
	query := `SELECT ` + leaseColumns + ` FROM local_leases
		WHERE state = 'granted' AND resource_key = ?`
	if want == LeaseRead {
		query += ` AND access_mode = 'write'`
	}
	query += ` ORDER BY granted_at LIMIT 1`

	lease, err := scanLease(t.tx.QueryRowContext(ctx, query, resourceKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find lease holder of %s: %w", resourceKey, err)
	}
	return &lease, nil
}

// expireLeasesFor 把某个资源上已经过期的 granted 行改写成 expired。
// 每条改写都留一条事件，这样「它是怎么没的」在历史里能查到。
func (t *Tx) expireLeasesFor(ctx context.Context, resourceKey string, now time.Time) error {
	rows, err := t.tx.QueryContext(ctx,
		`SELECT `+leaseColumns+` FROM local_leases
		 WHERE state = 'granted' AND resource_key = ?`, resourceKey)
	if err != nil {
		return fmt.Errorf("find expiring leases on %s: %w", resourceKey, err)
	}
	var expired []Lease
	for rows.Next() {
		lease, err := scanLease(rows)
		if err != nil {
			_ = rows.Close()
			return err
		}
		if lease.ExpiresAt.After(now) {
			continue
		}
		expired = append(expired, lease)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate expiring leases on %s: %w", resourceKey, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close expiring lease query: %w", err)
	}

	for _, lease := range expired {
		if err := t.markLeaseExpired(ctx, lease, now); err != nil {
			return err
		}
	}
	return nil
}

// leaseRow 按 id 读一行，读不到返回 ErrLeaseNotFound。
func (t *Tx) leaseRow(ctx context.Context, leaseID string) (Lease, error) {
	lease, err := scanLease(t.tx.QueryRowContext(ctx,
		`SELECT `+leaseColumns+` FROM local_leases WHERE lease_id = ?`, leaseID))
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, fmt.Errorf("%w: %s", ErrLeaseNotFound, leaseID)
	}
	if err != nil {
		return Lease{}, fmt.Errorf("read lease %s: %w", leaseID, err)
	}
	return lease, nil
}

func (t *Tx) markLeaseReleased(ctx context.Context, lease Lease, now time.Time) error {
	return t.setLeaseState(ctx, lease, LeaseReleased, now, "released", `{}`)
}

func (t *Tx) markLeaseExpired(ctx context.Context, lease Lease, now time.Time) error {
	detail := fmt.Sprintf(`{"expires_at":%q}`, formatTime(lease.ExpiresAt))
	return t.setLeaseState(ctx, lease, LeaseExpired, now, "expired", detail)
}

// setLeaseState 改一次状态并留一条事件。两条写都在同一个事务里：
// 状态改了而没有事件，等于租约历史说了谎。
func (t *Tx) setLeaseState(ctx context.Context, lease Lease, state LeaseState, now time.Time, kind, detail string) error {
	res, err := t.tx.ExecContext(ctx,
		`UPDATE local_leases SET state = ? WHERE lease_id = ? AND state = 'granted'`,
		string(state), lease.ID)
	if err != nil {
		return fmt.Errorf("set lease %s state %s: %w", lease.ID, state, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set lease %s state %s: %w", lease.ID, state, err)
	}
	if n != 1 {
		// 已经被别的事务改掉了（本仓库单写者，正常路径不会发生）。
		return fmt.Errorf("set lease %s state %s: 更新了 %d 行", lease.ID, state, n)
	}
	return t.appendLeaseEvent(ctx, lease.ID, now, kind, detail)
}

func (t *Tx) appendLeaseEvent(ctx context.Context, leaseID string, at time.Time, kind, detail string) error {
	id, err := newID()
	if err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx,
		`INSERT INTO local_lease_events (id, lease_id, at, kind, detail) VALUES (?, ?, ?, ?, ?)`,
		id, leaseID, formatTime(at), kind, detail,
	); err != nil {
		return fmt.Errorf("record lease event %s of %s: %w", kind, leaseID, err)
	}
	return nil
}
