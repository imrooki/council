package store

// 本文件是 Step 10 A 之五（契约 s10a5 v1.1 §2.2）的 store 包验收：
// InflightFor 的定点读，以及 BeginTurn 撞 (conv_id, agent_id) 唯一索引时译出的哨兵。

import (
	"errors"
	"testing"

	"council/internal/conversation"

	"modernc.org/sqlite"
)

// TestS10aInflightForFindsOnlyExactKey 是 S1。
//
// 【判别力】SQL 漏 agent_id 谓词（§4 #10）时 (A, gamma) 会读到 A 下的某一行而红；
// 漏 conv_id 谓词（§4 #11）时 (B, alpha) 会读到 A 的 alpha 而红；把扫描错误当成
// 「没有」（§4 #22）时坏行那一例 err == nil 而红。(A, beta) 只断言 TurnID，
// 防「恒返回第一行」这类写法（那样会拿到 s10a-a-alpha）。
func TestS10aInflightForFindsOnlyExactKey(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	convA := tstNewConv(t, ctx, s, "s10a-A")
	convB := tstNewConv(t, ctx, s, "s10a-B")

	aAlpha := tstBaseInflight(convA.ID, "alpha", "s10a-a-alpha")
	aBeta := tstBaseInflight(convA.ID, "beta", "s10a-a-beta")
	bGamma := tstBaseInflight(convB.ID, "gamma", "s10a-b-gamma")
	for _, it := range []conversation.InflightTurn{aAlpha, aBeta, bGamma} {
		if err := tstBeginTurn(t, ctx, s, it); err != nil {
			t.Fatalf("前提不成立：写入 %s 失败: %v", it.TurnID, err)
		}
	}

	got, ok, err := s.InflightFor(ctx, convA.ID, "alpha")
	if err != nil || !ok {
		t.Errorf("(A, alpha)：ok=%v err=%v，期望找到", ok, err)
	} else {
		tstAssertInflightEqual(t, got, aAlpha, "(A, alpha)")
	}

	got, ok, err = s.InflightFor(ctx, convA.ID, "beta")
	if err != nil || !ok || got.TurnID != "s10a-a-beta" {
		t.Errorf("(A, beta)：ok=%v err=%v TurnID=%q，期望找到 s10a-a-beta", ok, err, got.TurnID)
	}

	got, ok, err = s.InflightFor(ctx, convA.ID, "gamma")
	if err != nil || ok || got != (conversation.InflightTurn{}) {
		t.Errorf("(A, gamma)：ok=%v err=%v got=%+v，期望没有、无错、零值", ok, err, got)
	}

	_, ok, err = s.InflightFor(ctx, convB.ID, "alpha")
	if err != nil || ok {
		t.Errorf("(B, alpha)：ok=%v err=%v，期望没有、无错", ok, err)
	}

	_, ok, err = s.InflightFor(ctx, "s10a-no-such-conv", "alpha")
	if err != nil || ok {
		t.Errorf("(不存在的会话, alpha)：ok=%v err=%v，期望没有、无错", ok, err)
	}

	if _, err := tstClearInflight(t, ctx, s, "s10a-a-alpha"); err != nil {
		t.Fatalf("前提不成立：清掉 s10a-a-alpha 失败: %v", err)
	}
	_, ok, err = s.InflightFor(ctx, convA.ID, "alpha")
	if err != nil || ok {
		t.Errorf("清掉之后 (A, alpha)：ok=%v err=%v，期望没有、无错", ok, err)
	}

	// 坏行：started_at 解析不了。
	db := tstRawDB(t, s.Path())
	if _, err := db.Exec(`INSERT INTO inflight_turns (turn_id, conv_id, agent_id, provider, cursor_seq, scanned_to, started_at) VALUES ('s10a-bad-time', ?, 'delta', 'claude', 0, 0, 'not-a-time')`, convA.ID); err != nil {
		t.Fatalf("前提不成立：直写坏行失败: %v", err)
	}
	_, ok, err = s.InflightFor(ctx, convA.ID, "delta")
	if err == nil || ok {
		t.Errorf("坏行 (A, delta)：ok=%v err=%v，期望报错且 ok 为假（读不出来不是「没有」）", ok, err)
	}
}

// TestS10aBeginTurnConflictCarriesSentinel 是 S2。
//
// 【判别力】(b) 是 (a) 的对照，只差「哪条约束撞了」：任何 *sqlite.Error 都译成哨兵
// （§4 #7）时 (b) 红；不译（§4 #6）时 (a)(d) 的 errors.Is 红；只包哨兵丢驱动错误
// （§4 #8）时 (a) 的 errors.As 红。(c) 证明 (b) 之后事务没有卡住、换一个 turn_id 可写。
func TestS10aBeginTurnConflictCarriesSentinel(t *testing.T) {
	s, _ := tstStore(t)
	ctx := tstCtx(t)
	convA := tstNewConv(t, ctx, s, "s10a-A")
	first := tstBaseInflight(convA.ID, "alpha", "s10a-first")
	if err := tstBeginTurn(t, ctx, s, first); err != nil {
		t.Fatalf("前提不成立：写入 s10a-first 失败: %v", err)
	}

	// (a) 同 (conv, agent) 第二行。
	err := tstBeginTurn(t, ctx, s, tstBaseInflight(convA.ID, "alpha", "s10a-second"))
	if !errors.Is(err, conversation.ErrInflightConflict) {
		t.Errorf("(a)：错误链里没有 ErrInflightConflict: %v", err)
	}
	var se *sqlite.Error
	if !(errors.As(err, &se) && se.Code() == 2067) {
		t.Errorf("(a)：错误链里没有 Code()==2067 的 *sqlite.Error（驱动错误被丢了）: %v", err)
	}
	got, ok, rerr := s.InflightFor(ctx, convA.ID, "alpha")
	if rerr != nil || !ok {
		t.Errorf("(a)：冲突之后读第一行 ok=%v err=%v", ok, rerr)
	} else {
		tstAssertInflightEqual(t, got, first, "(a) 第一行原样")
	}

	// (b) 主键冲突。
	err = tstBeginTurn(t, ctx, s, tstBaseInflight(convA.ID, "beta", "s10a-first"))
	if err == nil {
		t.Errorf("(b)：主键冲突应当报错")
	}
	if errors.Is(err, conversation.ErrInflightConflict) {
		t.Errorf("(b)：主键冲突不该译成 ErrInflightConflict: %v", err)
	}

	// (c) 不冲突。
	if err := tstBeginTurn(t, ctx, s, tstBaseInflight(convA.ID, "beta", "s10a-third")); err != nil {
		t.Errorf("(c)：err = %v，期望 nil", err)
	}

	// (d) 经 Service 那一层重复 (a)。
	err = conversation.NewService(s).BeginTurn(ctx, tstBaseInflight(convA.ID, "alpha", "s10a-fourth"))
	if !errors.Is(err, conversation.ErrInflightConflict) {
		t.Errorf("(d)：经 Service.BeginTurn 之后错误链里没有 ErrInflightConflict: %v", err)
	}
}
