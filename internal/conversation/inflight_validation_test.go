package conversation

import (
	"context"
	"errors"
	"testing"
)

// 本文件补 Step 10 A 片之三遗留的空白：BeginTurn / SetInflightPhase
// 在进写事务之前做的入参校验，此前只能从 internal/turn 间接触发
// （那边传的 TurnID 与 Phase 永远合法），四条校验路径实际上没有被测过。
//
// 判别力的核心是 fakeStore.writeTxCalls（在 Store.WriteTx 里自增）：
// 校验若真的挡在事务之前，这个计数器在被拒绝的调用前后应当不变；
// 若挡不住（例如照常开事务、让底层报错），计数器会多 1。
// 只断言「返回了错误」测不出这个区别。

// TestInflightPhaseValid 锁住 InflightPhase.Valid 的判定集合：
// 只有三态合法，零值空串（最可能真实发生的漏填）与其它字符串都不合法。
func TestInflightPhaseValid(t *testing.T) {
	valid := []InflightPhase{PhaseBegun, PhaseSpawned, PhaseReturned}
	for _, p := range valid {
		t.Run("合法 "+string(p), func(t *testing.T) {
			if !p.Valid() {
				t.Errorf("InflightPhase(%q).Valid() = false，它是三态之一", string(p))
			}
		})
	}

	invalid := []InflightPhase{"", " ", "Begun", "BEGUN", "spawned ", "unknown"}
	for _, p := range invalid {
		t.Run("非法 "+string(p), func(t *testing.T) {
			if p.Valid() {
				t.Errorf("InflightPhase(%q).Valid() = true，它不属于三态", string(p))
			}
		})
	}
}

// TestBeginTurnRejectsEmptyTurnID 断言 TurnID 为空时 BeginTurn 在进写事务
// 之前就拒绝：errors.Is 命中 ErrMissingTurnID，写事务没被打开，也没有行
// 落进假实现的 inflight 状态。
//
// 判别力：一个「照常开事务、让底层报错」的坏实现也能让 err 非 nil，
// 但会让 writeTxCalls 多 1、或者让 fakeStore.BeginTurn 把这行插进
// f.inflight——下面两条状态断言就是为了挡住这种坏实现。
func TestBeginTurnRejectsEmptyTurnID(t *testing.T) {
	svc, f, _ := newServiceForTest(t)
	before := f.writeTxCalls

	err := svc.BeginTurn(context.Background(), InflightTurn{
		TurnID: "",
		Phase:  PhaseBegun, // 只破坏 TurnID 这一处，Phase 合法，避免撞错断言
	})
	if !errors.Is(err, ErrMissingTurnID) {
		t.Fatalf("errors.Is(err, ErrMissingTurnID) 应当为真，实际: %v", err)
	}
	if f.writeTxCalls != before {
		t.Errorf("空 TurnID 校验没过时不应当开写事务，writeTxCalls 从 %d 变成了 %d", before, f.writeTxCalls)
	}
	if len(f.inflight) != 0 {
		t.Errorf("空 TurnID 校验没过时不应当有行落进 inflight，实际: %+v", f.inflight)
	}
}

// TestBeginTurnRejectsInvalidPhase 覆盖非法 Phase，含最可能真实发生的
// 漏填——零值空串。BeginTurn 对非法 Phase 不返回 ErrMissingTurnID
// （那是另一条校验的哨兵），所以这里断言 err 非 nil、且没有被
// TurnID 那条校验误判，同时写事务没被打开、没有行落进 inflight。
func TestBeginTurnRejectsInvalidPhase(t *testing.T) {
	cases := []struct {
		name  string
		phase InflightPhase
	}{
		{"零值空串（最可能的漏填）", ""},
		{"拼写错误的字符串", "bogus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, f, _ := newServiceForTest(t)
			before := f.writeTxCalls

			err := svc.BeginTurn(context.Background(), InflightTurn{
				TurnID: "turn-1", // 只破坏 Phase 这一处，TurnID 合法
				Phase:  tc.phase,
			})
			if err == nil {
				t.Fatal("BeginTurn 应当报错，实际返回 nil")
			}
			if errors.Is(err, ErrMissingTurnID) {
				t.Fatalf("非法 Phase 不应当被误判成 ErrMissingTurnID: %v", err)
			}
			if f.writeTxCalls != before {
				t.Errorf("非法 Phase 校验没过时不应当开写事务，writeTxCalls 从 %d 变成了 %d", before, f.writeTxCalls)
			}
			if len(f.inflight) != 0 {
				t.Errorf("非法 Phase 校验没过时不应当有行落进 inflight，实际: %+v", f.inflight)
			}
		})
	}
}

// TestBeginTurnAcceptsValid 是对照组：TurnID 非空 + Phase 合法（三态各测
// 一次）时，BeginTurn 确实走到了 WriteTx、把行落进了 store。
//
// 少了这组对照，上面两条「拒绝」用例容易退化成单向断言——比如一个把
// BeginTurn 写成不管输入永远报错的坏实现，也能让「拒绝」那两条全绿。
func TestBeginTurnAcceptsValid(t *testing.T) {
	for _, phase := range []InflightPhase{PhaseBegun, PhaseSpawned, PhaseReturned} {
		t.Run(string(phase), func(t *testing.T) {
			svc, f, _ := newServiceForTest(t)
			before := f.writeTxCalls

			turn := InflightTurn{TurnID: "turn-" + string(phase), Phase: phase}
			if err := svc.BeginTurn(context.Background(), turn); err != nil {
				t.Fatalf("BeginTurn 应当成功，实际: %v", err)
			}
			if f.writeTxCalls != before+1 {
				t.Errorf("合法输入应当走一次 WriteTx，writeTxCalls 从 %d 变成了 %d", before, f.writeTxCalls)
			}
			got, ok := f.inflight[turn.TurnID]
			if !ok {
				t.Fatalf("合法输入应当把行落进 inflight，实际没有 turn_id=%q", turn.TurnID)
			}
			if got.Phase != phase {
				t.Errorf("落库的 Phase 是 %q，应当是 %q", got.Phase, phase)
			}
		})
	}
}

// TestSetInflightPhaseRejectsEmptyTurnID 断言 turnID 为空时
// SetInflightPhase 在进写事务之前就拒绝，判别力同
// TestBeginTurnRejectsEmptyTurnID。
func TestSetInflightPhaseRejectsEmptyTurnID(t *testing.T) {
	svc, f, _ := newServiceForTest(t)
	before := f.writeTxCalls

	err := svc.SetInflightPhase(context.Background(), "", PhaseSpawned)
	if !errors.Is(err, ErrMissingTurnID) {
		t.Fatalf("errors.Is(err, ErrMissingTurnID) 应当为真，实际: %v", err)
	}
	if f.writeTxCalls != before {
		t.Errorf("空 turnID 校验没过时不应当开写事务，writeTxCalls 从 %d 变成了 %d", before, f.writeTxCalls)
	}
}

// TestSetInflightPhaseRejectsInvalidPhase 覆盖非法 phase（含零值空串）。
//
// 关键判别力：先用 BeginTurn 造一行已存在的在飞记录（phase=begun），
// 再对同一个 turnID 传非法 phase。若只断言「err 非 nil」，一个删掉了
// Service 这层校验、直接转调 Tx.SetInflightPhase 的实现，在 turnID
// 不存在时也会报错（fakeStore.SetInflightPhase 自己对查不到的 turnID
// 返回 ErrInflightMissing）——那样「err 非 nil」会恒真，测不出 Service
// 这层校验是否真的还在。让 turnID 先存在，就排除了这条混淆路径。
//
// 同时断言：
//  1. err 非 nil，且不是 ErrMissingTurnID / ErrInflightMissing（排除误判）；
//  2. 写事务没有多开一次；
//  3. 那一行的 phase 原样留在 begun，没被改写——这一条专门拦住
//     「Service 校验被删掉、非法值直接写进 fakeStore.inflight」的坏
//     实现，因为 fakeStore.SetInflightPhase 本身对任何字符串值都
//     来者不拒，不会替 Service 把空串拦下来。
func TestSetInflightPhaseRejectsInvalidPhase(t *testing.T) {
	cases := []struct {
		name  string
		phase InflightPhase
	}{
		{"零值空串（最可能的漏填）", ""},
		{"拼写错误的字符串", "bogus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, f, _ := newServiceForTest(t)
			const turnID = "turn-1"
			if err := svc.BeginTurn(context.Background(), InflightTurn{
				TurnID: turnID, Phase: PhaseBegun,
			}); err != nil {
				t.Fatalf("准备在飞记录失败: %v", err)
			}
			before := f.writeTxCalls

			err := svc.SetInflightPhase(context.Background(), turnID, tc.phase)
			if err == nil {
				t.Fatal("SetInflightPhase 应当报错，实际返回 nil")
			}
			if errors.Is(err, ErrMissingTurnID) {
				t.Fatalf("非法 phase 不应当被误判成 ErrMissingTurnID: %v", err)
			}
			if errors.Is(err, ErrInflightMissing) {
				t.Fatalf("非法 phase 不应当被误判成 ErrInflightMissing（turnID 其实存在）: %v", err)
			}
			if f.writeTxCalls != before {
				t.Errorf("非法 phase 校验没过时不应当开写事务，writeTxCalls 从 %d 变成了 %d", before, f.writeTxCalls)
			}
			if got := f.inflight[turnID].Phase; got != PhaseBegun {
				t.Errorf("非法 phase 校验没过时不应当改写这一行，phase 从 begun 变成了 %q", got)
			}
		})
	}
}

// TestSetInflightPhaseAcceptsValid 是对照组：turnID 非空且行存在 + phase
// 合法（三态各测一次）时，SetInflightPhase 确实走到了 WriteTx、把这一行
// 的 phase 真的改了。
//
// 种子 phase 特意选得与目标 phase 不同：若种子与目标相同，即便
// SetInflightPhase 是个空操作，「phase 被改成了目标值」这条断言也会
// 巧合地为真，测不出「真的写了」还是「本来就是这个值」。
func TestSetInflightPhaseAcceptsValid(t *testing.T) {
	for _, phase := range []InflightPhase{PhaseBegun, PhaseSpawned, PhaseReturned} {
		t.Run(string(phase), func(t *testing.T) {
			svc, f, _ := newServiceForTest(t)
			const turnID = "turn-1"
			seed := PhaseBegun
			if phase == PhaseBegun {
				seed = PhaseReturned
			}
			if err := svc.BeginTurn(context.Background(), InflightTurn{
				TurnID: turnID, Phase: seed,
			}); err != nil {
				t.Fatalf("准备在飞记录失败: %v", err)
			}
			before := f.writeTxCalls

			if err := svc.SetInflightPhase(context.Background(), turnID, phase); err != nil {
				t.Fatalf("SetInflightPhase 应当成功，实际: %v", err)
			}
			if f.writeTxCalls != before+1 {
				t.Errorf("合法输入应当走一次 WriteTx，writeTxCalls 从 %d 变成了 %d", before, f.writeTxCalls)
			}
			if got := f.inflight[turnID].Phase; got != phase {
				t.Errorf("phase 应当被改成 %q，实际 %q", phase, got)
			}
		})
	}
}
