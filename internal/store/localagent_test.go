package store

// localagent_test.go 是 v10 租约表的读写用例。它验的是 Plan §11 P3 的验收
// 口径，加上 §8.3 的语义：
//   - 同一 resource_key 上不会出现两条冲突的 granted 租约（写独占、读共享）；
//   - 进程崩溃留下的租约到期之后，新的申请能拿到，旧行变成 expired 而不是
//     永久 granted；
//   - generation 单调递增，旧授权不会因为新授权释放而复活；
//   - 交还只影响自己的租约；重复交还不报错、也不重复写事件。

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func tstLeaseReq(key, owner string, access LeaseAccess) LeaseRequest {
	return LeaseRequest{
		ResourceKey: key,
		OwnerKind:   LeaseOwnerCLI,
		OwnerID:     owner,
		Access:      access,
		TTL:         time.Minute,
	}
}

// TestClaimLeaseGrantsAndRecordsEvent 是正向基线：申请一条写租约，字段齐全，
// generation 从 1 开始，事件表留下一行 granted。
func TestClaimLeaseGrantsAndRecordsEvent(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	lease, err := s.ClaimLease(ctx, tstLeaseReq(`D:\work\proj`, "owner-a", LeaseWrite))
	if err != nil {
		t.Fatalf("ClaimLease: %v", err)
	}
	if lease.ID == "" {
		t.Error("lease_id 是空的")
	}
	if lease.ResourceKey != `D:\work\proj` {
		t.Errorf("resource_key = %q", lease.ResourceKey)
	}
	if lease.State != LeaseGranted || lease.Access != LeaseWrite {
		t.Errorf("state/access = %q/%q，期望 granted/write", lease.State, lease.Access)
	}
	if lease.Generation != 1 {
		t.Errorf("generation = %d，期望 1（这个资源上的第一次）", lease.Generation)
	}
	if got := lease.ExpiresAt.Sub(lease.GrantedAt); got != time.Minute {
		t.Errorf("TTL = %s，期望 1m（granted_at 到 expires_at）", got)
	}
	if !lease.GrantedAt.Equal(lease.HeartbeatAt) {
		t.Errorf("heartbeat_at = %s，期望与 granted_at 相同", lease.HeartbeatAt)
	}

	events, err := s.LeaseEvents(ctx, lease.ID)
	if err != nil {
		t.Fatalf("LeaseEvents: %v", err)
	}
	if len(events) != 1 || events[0].Kind != "granted" {
		t.Fatalf("事件 = %+v，期望恰好一条 granted", events)
	}
	if !strings.Contains(events[0].Detail, `"owner_id":"owner-a"`) {
		t.Errorf("事件 detail 没记下申请者: %s", events[0].Detail)
	}
}

// TestClaimLeaseWriteExclusive 钉住写独占：第二条写租约（不同 owner）必须
// 冲突，错误能 errors.Is 认出哨兵，并且带着持有者信息 —— 调用方才能说清
// 「谁拿着、到什么时候」。
func TestClaimLeaseWriteExclusive(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	first, err := s.ClaimLease(ctx, tstLeaseReq("res-w", "owner-a", LeaseWrite))
	if err != nil {
		t.Fatalf("第一条写租约: %v", err)
	}
	_, err = s.ClaimLease(ctx, tstLeaseReq("res-w", "owner-b", LeaseWrite))
	if err == nil {
		t.Fatal("第二条写租约竟然成功了")
	}
	if !errors.Is(err, ErrLeaseConflict) {
		t.Errorf("errors.Is(ErrLeaseConflict) = false，实际: %v", err)
	}
	var conflict *LeaseConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("错误不是 *LeaseConflictError: %v", err)
	}
	if conflict.Holder.ID != first.ID || conflict.Holder.OwnerID != "owner-a" {
		t.Errorf("冲突里的持有者 = %+v，期望 %s/owner-a", conflict.Holder, first.ID)
	}
	for _, want := range []string{"owner-a", first.ID} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误文案没有提到 %q: %v", want, err)
		}
	}
}

// TestClaimLeaseReadSharedWriteExclusive 钉住 §8.3 的读写语义：
// read+read 可以并存，read 与 write 互斥（两个方向都试）。
func TestClaimLeaseReadSharedWriteExclusive(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	if _, err := s.ClaimLease(ctx, tstLeaseReq("res-rw", "reader-1", LeaseRead)); err != nil {
		t.Fatalf("第一条读租约: %v", err)
	}
	if _, err := s.ClaimLease(ctx, tstLeaseReq("res-rw", "reader-2", LeaseRead)); err != nil {
		t.Fatalf("第二条读租约应当与第一条共存: %v", err)
	}
	if _, err := s.ClaimLease(ctx, tstLeaseReq("res-rw", "writer-1", LeaseWrite)); !errors.Is(err, ErrLeaseConflict) {
		t.Errorf("读租约存在时申请写租约 = %v，期望 ErrLeaseConflict", err)
	}

	grants, err := s.GrantedLeases(ctx)
	if err != nil {
		t.Fatalf("GrantedLeases: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("有效租约 = %d 条，期望 2 条读租约: %+v", len(grants), grants)
	}

	// 另一条资源上的写租约不受影响（冲突判定按 resource_key 分桶）。
	if _, err := s.ClaimLease(ctx, tstLeaseReq("res-rw-other", "writer-1", LeaseWrite)); err != nil {
		t.Errorf("别的资源上的写租约失败: %v", err)
	}
}

// TestClaimLeaseWriteBlocksRead 是上一个用例的反方向：先写后读也要冲突。
func TestClaimLeaseWriteBlocksRead(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	if _, err := s.ClaimLease(ctx, tstLeaseReq("res-wr", "writer", LeaseWrite)); err != nil {
		t.Fatalf("写租约: %v", err)
	}
	if _, err := s.ClaimLease(ctx, tstLeaseReq("res-wr", "reader", LeaseRead)); !errors.Is(err, ErrLeaseConflict) {
		t.Errorf("写租约存在时申请读租约 = %v，期望 ErrLeaseConflict", err)
	}
}

// TestReleaseLeaseAndReclaim 覆盖交还再申请：generation 递增（旧授权不会
// 复活），重复交还不算错误也不重复写事件。
func TestReleaseLeaseAndReclaim(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	first, err := s.ClaimLease(ctx, tstLeaseReq("res-rel", "owner-a", LeaseWrite))
	if err != nil {
		t.Fatalf("申请: %v", err)
	}
	released, changed, err := s.ReleaseLease(ctx, first.ID)
	if err != nil {
		t.Fatalf("交还: %v", err)
	}
	if !changed || released.State != LeaseReleased {
		t.Errorf("交还结果 = changed:%v state:%q，期望 true/released", changed, released.State)
	}

	again, changed, err := s.ReleaseLease(ctx, first.ID)
	if err != nil {
		t.Fatalf("重复交还不该报错: %v", err)
	}
	if changed || again.State != LeaseReleased {
		t.Errorf("重复交还 = changed:%v state:%q，期望 false/released", changed, again.State)
	}
	events, err := s.LeaseEvents(ctx, first.ID)
	if err != nil {
		t.Fatalf("LeaseEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("事件 = %d 条（%+v），期望 granted + released 两条", len(events), events)
	}

	second, err := s.ClaimLease(ctx, tstLeaseReq("res-rel", "owner-b", LeaseWrite))
	if err != nil {
		t.Fatalf("交还后再次申请失败: %v", err)
	}
	if second.Generation != first.Generation+1 {
		t.Errorf("generation = %d，期望 %d（单调递增，跨 released 行算）",
			second.Generation, first.Generation+1)
	}
	if _, _, err := s.ReleaseLease(ctx, "no-such-lease"); !errors.Is(err, ErrLeaseNotFound) {
		t.Errorf("交还不存在的租约 = %v，期望 ErrLeaseNotFound", err)
	}
}

// TestExpiredLeaseIsCollectedOnNextClaim 是 Plan §11 P3 那条验收：
// 「进程崩溃后到期的租约变成 expired 而不是永久 granted」。
//
// 没有后台循环，所以过期是在下一次申请同一资源时顺手改写的；这里用一个
// 20ms 的 TTL 加一次短暂等待来走真实路径，不打桩时间。
func TestExpiredLeaseIsCollectedOnNextClaim(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	crashed := tstLeaseReq("res-expired", "crashed-owner", LeaseWrite)
	crashed.TTL = 20 * time.Millisecond
	first, err := s.ClaimLease(ctx, crashed)
	if err != nil {
		t.Fatalf("申请: %v", err)
	}

	// 到期之前 GrantedLeases 还能看到它；到期之后看不到（读路径不写库，
	// 但不会把过期的行当成持有者）。
	time.Sleep(40 * time.Millisecond)
	grants, err := s.GrantedLeases(ctx)
	if err != nil {
		t.Fatalf("GrantedLeases: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("过期之后 GrantedLeases 还返回 %+v，期望空", grants)
	}

	second, err := s.ClaimLease(ctx, tstLeaseReq("res-expired", "next-owner", LeaseWrite))
	if err != nil {
		t.Fatalf("过期之后新申请应当成功: %v", err)
	}
	if second.Generation != 2 {
		t.Errorf("新租约 generation = %d，期望 2", second.Generation)
	}

	raw := tstRawDB(t, s.Path())
	if got := tstScanText(t, raw, `SELECT state FROM local_leases WHERE lease_id = ?`, first.ID); got != string(LeaseExpired) {
		t.Errorf("崩溃那条租约的 state = %q，期望 expired（不能永久 granted）", got)
	}
	events, err := s.LeaseEvents(ctx, first.ID)
	if err != nil {
		t.Fatalf("LeaseEvents: %v", err)
	}
	if len(events) != 2 || events[1].Kind != "expired" {
		t.Errorf("事件 = %+v，期望 granted + expired", events)
	}
}

// TestReleaseLeasesByOwnerOnlyOwnRows 钉住「只释放自己的」：同资源上两条
// 读租约分属两人，只交出其中一人的那条。
func TestReleaseLeasesByOwnerOnlyOwnRows(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	mine, err := s.ClaimLease(ctx, tstLeaseReq("res-own", "me", LeaseRead))
	if err != nil {
		t.Fatalf("自己的读租约: %v", err)
	}
	other, err := s.ClaimLease(ctx, tstLeaseReq("res-own", "other", LeaseRead))
	if err != nil {
		t.Fatalf("别人的读租约: %v", err)
	}

	released, err := s.ReleaseLeasesByOwner(ctx, "res-own", "me")
	if err != nil {
		t.Fatalf("ReleaseLeasesByOwner: %v", err)
	}
	if len(released) != 1 || released[0].ID != mine.ID {
		t.Fatalf("交还了 %+v，期望只有 %s", released, mine.ID)
	}
	grants, err := s.GrantedLeases(ctx)
	if err != nil {
		t.Fatalf("GrantedLeases: %v", err)
	}
	if len(grants) != 1 || grants[0].ID != other.ID {
		t.Errorf("剩余有效租约 = %+v，期望只有 %s（别人的不能被顺手释放）", grants, other.ID)
	}

	// owner_id 为空是用法错误，不是「释放全部」。
	if _, err := s.ReleaseLeasesByOwner(ctx, "res-own", " "); err == nil {
		t.Error("owner_id 为空时应当报错（本模块没有 force 释放）")
	}
	// 一条都没匹配到不算错误。
	if released, err := s.ReleaseLeasesByOwner(ctx, "res-none", "me"); err != nil || len(released) != 0 {
		t.Errorf("空资源 = %+v, err=%v，期望空表与 nil", released, err)
	}
}

// TestClaimLeaseRejectsInvalidRequest 是参数门槛：垃圾值不许写进库。
func TestClaimLeaseRejectsInvalidRequest(t *testing.T) {
	s, closeFn := tstStore(t)
	defer closeFn()
	ctx := tstCtx(t)

	cases := []struct {
		name string
		req  LeaseRequest
		want string
	}{
		{"空资源键", LeaseRequest{OwnerKind: LeaseOwnerCLI, OwnerID: "x", Access: LeaseWrite, TTL: time.Minute}, "resource_key"},
		{"非法 access", LeaseRequest{ResourceKey: "r", OwnerKind: LeaseOwnerCLI, OwnerID: "x", Access: "rw", TTL: time.Minute}, "access_mode"},
		{"非法 owner_kind", LeaseRequest{ResourceKey: "r", OwnerKind: "human", OwnerID: "x", Access: LeaseWrite, TTL: time.Minute}, "owner_kind"},
		{"空 owner_id", LeaseRequest{ResourceKey: "r", OwnerKind: LeaseOwnerCLI, OwnerID: "  ", Access: LeaseWrite, TTL: time.Minute}, "owner_id"},
		{"非正 TTL", LeaseRequest{ResourceKey: "r", OwnerKind: LeaseOwnerCLI, OwnerID: "x", Access: LeaseWrite, TTL: 0}, "ttl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.ClaimLease(ctx, tc.req)
			if err == nil {
				t.Fatalf("非法申请竟然成功: %+v", tc.req)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误文案没有提到 %q: %v", tc.want, err)
			}
		})
	}

	if grants, err := s.GrantedLeases(ctx); err != nil || len(grants) != 0 {
		t.Errorf("非法申请写进了库: %+v, err=%v", grants, err)
	}
}
