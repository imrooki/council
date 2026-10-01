package tui

// 本文件补 RB-B 契约 §1.1 D1「改看板行之前先整表拷贝」的写时拷贝快照用例（2026-09-25，#41 RB-B 剩余项 (i) 的延伸）。
//
// 不是背对背写的：落地后的独立验证在 B1-4（saved）与 B1-7（saved2）两处快照之外，找出四个破坏写时拷贝、
// 全包却全绿的变异，本文件照着那四处缺口各补一个快照。做法都一样：在某一步之前存下 Model 值与它的看板，
// 走这一步，断言存下的那个值的看板逐行不变；另断言当前看板确实变了，免得「这一步被忽略」也能让用例绿。
// Model 的时钟读 rig 的 now，两次渲染之间不动 now，所以存下的值两次渲染只可能因为底层数组被改而不同。

import (
	"testing"

	"council/internal/repl"
	"council/internal/turn"
)

func rbbCowDone(label, id string) memberDoneMsg {
	return memberDoneMsg{ev: repl.MemberDoneEvent{Label: label, AgentID: id, Outcome: turn.OutcomeSpoke, WallMS: 1500, RoundID: rbaRID}}
}

// rbbCowStep 存下当前 Model 与它的看板，喂 msg，然后断言存下的值看板不变、当前看板变了。
func rbbCowStep(t *testing.T, r *rbbModelRig, where string, msg any) {
	t.Helper()
	saved := r.m
	before := rbbBoardOf(saved)
	r.step(msg)
	rbbWantBoard(t, where+" 存下的 Model 值（COW）", rbbBoardOf(saved), before...)
	after := rbbBoardOf(r.m)
	same := len(after) == len(before)
	for i := 0; same && i < len(after); i++ {
		same = after[i] == before[i]
	}
	if same {
		t.Errorf("%s：前提不成立，这一步之后当前看板应当变化，实际与之前逐行相同 %q", where, after)
	}
}

// TestRbbCowSnapshots：四处写时拷贝快照。
//
// 【判别力】（变异在主仓库上逐个实测，2026-09-25）：
//   - 等长名单换段时原地覆盖旧行（`len(m.rows) == len(Speakers)` 时 `rows = m.rows` 再逐行赋值）→「等长换段」红；
//     B1-7 从 3 行换到 1 行，杀不到它。#67（`m.rows[:0]` 再 append）在这里同样红。
//   - memberDone 只在本段第一个人回来时拷贝、之后原地写 →「第二个回来」红；B1-4 的 saved 存在第一次拷贝之前，杀不到。
//   - memberDone 在 i > 0 时原地写 →「非首行先回来」「第二个回来」红。
//   - memberDone 在只有一行时原地写 →「单行裁判段」红；B1-8 的裁判段没有存快照。
func TestRbbCowSnapshots(t *testing.T) {
	abc := []string{"rbb_a", "rbb_b", "rbb_c"}

	t.Run("等长换段", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 24)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 2, Members: 3, Speakers: abc}})
		for _, id := range abc {
			r.step(rbbCowDone("发言轮", id))
		}
		rbbCowStep(t, r, "等长换段", roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 2, N: 2, Members: 3, Speakers: abc}})
	})

	t.Run("第二个回来", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 24)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 3, Speakers: abc}})
		r.step(rbbCowDone("发言轮", "rbb_a"))
		rbbCowStep(t, r, "第二个回来", rbbCowDone("发言轮", "rbb_b"))
	})

	t.Run("非首行先回来", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 24)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "发言轮", K: 1, N: 1, Members: 3, Speakers: abc}})
		rbbCowStep(t, r, "非首行先回来", rbbCowDone("发言轮", "rbb_c"))
	})

	t.Run("单行裁判段", func(t *testing.T) {
		r := rbbNewModelRig(t, 100, 24)
		r.submitConsume()
		r.step(roundStartMsg{ev: repl.RoundStartEvent{Label: "裁判轮", Members: 1, Speakers: []string{"rbb_j"}}})
		rbbCowStep(t, r, "单行裁判段", rbbCowDone("裁判轮", "rbb_j"))
	})
}
