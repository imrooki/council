package repl

// 本文件覆盖冻结契约 E:\multi_model_cowork\s10d2_contract.md（v1.1）§2.2：
// /log 标记判据的边界，两种 Meta 形状各跑一遍。
//
// 两种形状：真库经 JSON 读回（denied_actions 是 []any、数字是 float64、typed nil 读回 nil、
// map 读回 map[string]any）；本包的 fakeStore 不经 JSON，Meta 原样存
// （denied_actions 原样是 []provider.DeniedAction / []string / typed nil）。
// 只对着其中一种写判据，另一种上标记会静默消失，所以 L1 与 L2 必须同时存在。
//
// 背对背纪律：本文件不引用任何新标识符，在 HEAD 上编译得过；三条用例在 HEAD 上
// 都红在「期望有标记的那一行」上，没有标记的行在 HEAD 上就与期望逐字节相同。

import (
	"context"
	"strings"
	"testing"

	"council/internal/conversation"
	"council/internal/provider"
)

// s10dRHdr 是契约 §3.2 的 R_HDR（种子用例的表头，前面带提示符）。
const s10dRHdr = "council> SEQ  SENDER  KIND    TO  BRAIN  TEXT"

// s10dDA 是契约 §3.4 的 da。只读，各条种子共用。
var s10dDA = []provider.DeniedAction{{Action: "write_file", DisplayName: "WriteToFile"}}

// s10dSeed 是一条种子消息。user 为 true 时按 user 消息写（契约 §3.4 第 18 行），否则按 system。
type s10dSeed struct {
	text string
	meta map[string]any
	user bool
}

// s10dLogSeeds 返回契约 §3.4 的 20 条种子，顺序冻结。每次调用新造一份 map，L1 与 L2 不共用。
func s10dLogSeeds() []s10dSeed {
	return []s10dSeed{
		{text: "ev-turn-lost", meta: map[string]any{"event": "turn_lost"}},
		{text: "ev-empty", meta: map[string]any{"event": ""}},
		{text: "ev-num", meta: map[string]any{"event": 5}},
		{text: "ev-nl", meta: map[string]any{"event": "a\nb\tc"}},
		{text: "ev-space", meta: map[string]any{"event": "  "}},
		{text: "td-false", meta: map[string]any{"thought_dropped": false}},
		{text: "td-str", meta: map[string]any{"thought_dropped": "true"}},
		{text: "td-true", meta: map[string]any{"thought_dropped": true}},
		{text: "da-empty", meta: map[string]any{"denied_actions": []any{}}},
		{text: "da-one", meta: map[string]any{"denied_actions": s10dDA}},
		{text: "da-str", meta: map[string]any{"denied_actions": "x"}},
		{text: "da-strs", meta: map[string]any{"denied_actions": []string{"x"}}},
		{text: "da-nil", meta: map[string]any{"denied_actions": []provider.DeniedAction(nil)}},
		{text: "raw-only", meta: map[string]any{"denied_actions_raw": []any{map[string]any{"action": "w"}}}},
		{text: "all3", meta: map[string]any{"event": "turn_failed", "thought_dropped": true, "denied_actions": s10dDA}},
		{text: "", meta: map[string]any{"denied_actions": s10dDA}},
		{text: "plain", meta: nil},
		{text: "ev-on-user", meta: map[string]any{"event": "team_bound", "thought_dropped": true, "denied_actions": s10dDA}, user: true},
		{text: "", meta: map[string]any{"event": "turn_failed", "thought_dropped": true}},
		{text: "da-map", meta: map[string]any{"denied_actions": map[string]any{"action": "w"}}},
	}
}

// s10dLogWant 是契约 §3.4 R_HDR 之后紧跟的 20 行，L1 与 L2 相同，逐字节照抄。
var s10dLogWant = []string{
	"1    system  system  -   -      ev-turn-lost [event=turn_lost]",
	"2    system  system  -   -      ev-empty",
	"3    system  system  -   -      ev-num",
	"4    system  system  -   -      ev-nl [event=a b c]",
	"5    system  system  -   -      ev-space",
	"6    system  system  -   -      td-false",
	"7    system  system  -   -      td-str",
	"8    system  system  -   -      td-true [thought_dropped]",
	"9    system  system  -   -      da-empty",
	"10   system  system  -   -      da-one [denied_actions]",
	"11   system  system  -   -      da-str",
	"12   system  system  -   -      da-strs [denied_actions]",
	"13   system  system  -   -      da-nil",
	"14   system  system  -   -      raw-only",
	"15   system  system  -   -      all3 [event=turn_failed] [thought_dropped] [denied_actions]",
	"16   system  system  -   -      - [denied_actions]",
	"17   system  system  -   -      plain",
	"18   user    user    -   -      ev-on-user [event=team_bound] [thought_dropped] [denied_actions]",
	"19   system  system  -   -      - [event=turn_failed] [thought_dropped]",
	"20   system  system  -   -      da-map",
}

// s10dEventFullSet 是契约 §3.5 的事件全集，顺序冻结。
var s10dEventFullSet = []string{
	"session_opening",
	"turn_failed",
	"post_rejected",
	"session_rebuilt",
	"turn_lost",
	"round_started",
	"round_finished",
	"team_bound",
}

// s10dEventWant 是契约 §3.5 的期望 8 行，逐字节照抄。
var s10dEventWant = []string{
	"1    system  system  -   -      e [event=session_opening]",
	"2    system  system  -   -      e [event=turn_failed]",
	"3    system  system  -   -      e [event=post_rejected]",
	"4    system  system  -   -      e [event=session_rebuilt]",
	"5    system  system  -   -      e [event=turn_lost]",
	"6    system  system  -   -      e [event=round_started]",
	"7    system  system  -   -      e [event=round_finished]",
	"8    system  system  -   -      e [event=team_bound]",
}

// s10dSeedMessage 把一条种子拼成 conversation.Message（字段同 L1 / L2）。
func s10dSeedMessage(convID string, s s10dSeed) conversation.Message {
	m := conversation.Message{
		ConvID:   convID,
		SenderID: conversation.ReservedSenderSystem,
		Kind:     conversation.KindSystem,
		Text:     s.text,
		Meta:     s.meta,
	}
	if s.user {
		m.SenderID = conversation.ReservedSenderUser
		m.Kind = conversation.KindUser
	}
	return m
}

// s10dCheckBlock 断言 out 里 hdr 恰好一行，且紧跟其后的 len(want) 行逐行整行等于 want。
// 表头数不对或后面的行不够就 Fatalf 并贴全量输出；逐行不等用 Errorf，带行号、实际与期望，
// 一次跑完就能看出红在哪几行。
func s10dCheckBlock(t *testing.T, out, hdr string, want []string) {
	t.Helper()
	lines := strings.Split(out, "\n")
	idx := -1
	count := 0
	for i, l := range lines {
		if l == hdr {
			if idx < 0 {
				idx = i
			}
			count++
		}
	}
	if count != 1 {
		t.Fatalf("表头 %q 应恰好一行，实际 %d 行；输出：\n%s", hdr, count, out)
	}
	if idx+1+len(want) > len(lines) {
		t.Fatalf("表头之后应至少还有 %d 行，实际只有 %d 行；输出：\n%s", len(want), len(lines)-idx-1, out)
	}
	for k, w := range want {
		if got := lines[idx+1+k]; got != w {
			t.Errorf("表头后第 %d 行不符：\n实际 %q\n期望 %q", k+1, got, w)
		}
	}
	if t.Failed() {
		t.Logf("完整输出：\n%s", out)
	}
}

// TestS10dLogMarksRealStore 是 L1：真库，按契约 §3.4 的顺序 Post 20 条，再 /log 50。
//
// 【判别力】每条种子与相邻种子只差一个值，对照写在契约 §3.4 表的「对照」列；逐行整行比对。
// 真库把 []provider.DeniedAction 编成 JSON 再读回 []any、把 5 读回 float64、把 typed nil 读回 nil：
// 「D 只认 []provider.DeniedAction」的实现在这里红（第 10、12、15、16 行）。
// 第 18 行（user 消息三键俱全）杀「只对 system 打 E」「user 消息上不打 T」「user 消息上不打 D」；
// 第 19 行（正文为空、E 加 T）杀「正文为空时只打 D」；第 20 行（denied_actions 是 map）杀「D 也认 map」。
// 第 4 行杀「E 不过 oneLine」「只 TrimSpace」；第 5 行杀「先判空再 oneLine」；第 3 行杀 fmt.Sprint 兜底；
// 第 7 行杀「T 用 fmt.Sprint」；第 6、7 行杀「T 键在就打」；第 9、11 行杀「D 键在就打」；
// 第 14 行杀「D 读 denied_actions_raw」；第 15 行杀任何别的 E/T/D 排列。
// 没有标记的行在 HEAD 上就与期望相同，它们是「不许误标」的反向钉。
func TestS10dLogMarksRealStore(t *testing.T) {
	svc := newRoundStreamService(t)
	ctx := context.Background()
	conv, err := svc.Create(ctx, "s10d")
	if err != nil {
		t.Fatalf("前提：Create 失败：%v", err)
	}
	for i, s := range s10dLogSeeds() {
		if _, err := svc.Post(ctx, s10dSeedMessage(conv.ID, s)); err != nil {
			t.Fatalf("前提：第 %d 条种子 Post 失败：%v", i+1, err)
		}
	}

	out, err := runREPLSvc(t, svc, "/use "+conv.ID+"\n/log 50\n")
	if err != nil {
		t.Fatalf("前提：REPL.Run 应返回 nil，实际 %v；输出：\n%s", err, out)
	}
	s10dCheckBlock(t, out, s10dRHdr, s10dLogWant)
}

// TestS10dLogMarksFakeStore 是 L2：fakeStore（不经 JSON），同一组 20 条种子，期望的 20 行与 L1 完全相同。
//
// 【判别力】fakeStore 里 denied_actions 原样是 []provider.DeniedAction / []string / typed nil：
// 「D 只认 []any」的实现在这里红（第 10、12、15、16 行）；「D 键在就打」「D 非 nil 就打」
// 在第 13 行红（typed nil 切片装进 any 之后不等于 nil）；「D 不看 Len」在第 9、13 行红。
// 与 L1 缺一条就有一种 Meta 形状没人担保。
func TestS10dLogMarksFakeStore(t *testing.T) {
	svc, f := newTestService(t)
	ctx := context.Background()
	conv := f.seedConversation("s10dconv", "s10d")
	for i, s := range s10dLogSeeds() {
		if _, err := f.AppendMessage(ctx, s10dSeedMessage(conv.ID, s)); err != nil {
			t.Fatalf("前提：第 %d 条种子 AppendMessage 失败：%v", i+1, err)
		}
	}

	out, err := runREPLSvc(t, svc, "/use s10dconv\n/log 50\n")
	if err != nil {
		t.Fatalf("前提：REPL.Run 应返回 nil，实际 %v；输出：\n%s", err, out)
	}
	s10dCheckBlock(t, out, s10dRHdr, s10dLogWant)
}

// TestS10dLogMarksEventFullSet 是 L3：真库，按契约 §3.5 的顺序 Post 8 条 system 消息，
// 各带一个已知 event 值。
//
// 【判别力】担保「没有白名单、没有翻译表」：漏掉任何一个已知事件（例如白名单里没写
// team_bound，它是 conversation 包的常量，不在 internal/turn/record.go 那条注释里）
// 或把值翻译成别的字都会红在那一行。HEAD 上 8 行全红（没有标记）。
func TestS10dLogMarksEventFullSet(t *testing.T) {
	svc := newRoundStreamService(t)
	ctx := context.Background()
	conv, err := svc.Create(ctx, "s10d-ev")
	if err != nil {
		t.Fatalf("前提：Create 失败：%v", err)
	}
	for i, ev := range s10dEventFullSet {
		m := conversation.Message{
			ConvID:   conv.ID,
			SenderID: conversation.ReservedSenderSystem,
			Kind:     conversation.KindSystem,
			Text:     "e",
			Meta:     map[string]any{"event": ev},
		}
		if _, err := svc.Post(ctx, m); err != nil {
			t.Fatalf("前提：第 %d 条（event=%s）Post 失败：%v", i+1, ev, err)
		}
	}

	out, err := runREPLSvc(t, svc, "/use "+conv.ID+"\n/log\n")
	if err != nil {
		t.Fatalf("前提：REPL.Run 应返回 nil，实际 %v；输出：\n%s", err, out)
	}
	s10dCheckBlock(t, out, s10dRHdr, s10dEventWant)
}
