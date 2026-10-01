package repl

// 验收 7-14：/ask 在「成员 ∪ 配置」全集上解析，X-1 的闸一个字不改（契约第 6.2 节）。
//
// 「进程没起」一律按 turn_usage 行数判，而且歧义类用例对每个候选 id 与原始引用
// 各取一次：只数候选挡不住「拿原始引用去调 Ask」的坏实现 —— Runner 对不认识的
// id 同样会写一行 turn_usage，只是记在原始引用名下。

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// 验收 7：前缀可用，走到 Ask，「正在问」那一行是全名。
// turn_usage +1 同时证明本文件的自定义配置下观测通道成立（它是后面「不变」类断言的前提）。
func TestUx2rAskPrefixReachesAsk(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "grok-4.7")
	fx.seedActive(t, "opus-a", "")
	beforeFull, beforeRef := fx.usage(t, "opus-a"), fx.usage(t, "op")

	out := fx.run(t, "/ask op\n")

	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAsking, "opus-a"))
	if got := fx.usage(t, "opus-a"); got != beforeFull+1 {
		t.Errorf("/ask op 应当以全名 opus-a 走到 Ask（turn_usage +1），实际 %d -> %d；完整输出:\n%s", beforeFull, got, out)
	}
	if got := fx.usage(t, "op"); got != beforeRef {
		t.Errorf("Ask 不许用原始引用 op 调用，op 名下 turn_usage %d -> %d；完整输出:\n%s", beforeRef, got, out)
	}
}

// 敲全名时行为与今天相同：全名同时是另一个 id 的前缀，也必须问全名本人。
func TestUx2rAskExactFullNameAmongPrefixPair(t *testing.T) {
	fx := ux2rNewFix(t, "opus-5-0", "opus-5-0-max")
	fx.seedActive(t, "opus-5-0", "")
	fx.seedActive(t, "opus-5-0-max", "")
	before := fx.usages(t, "opus-5-0", "opus-5-0-max")

	out := fx.run(t, "/ask opus-5-0\n")

	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAsking, "opus-5-0"))
	if got := fx.usage(t, "opus-5-0"); got != before["opus-5-0"]+1 {
		t.Errorf("opus-5-0 应当被问（turn_usage +1），实际 %d -> %d；完整输出:\n%s", before["opus-5-0"], got, out)
	}
	if got := fx.usage(t, "opus-5-0-max"); got != before["opus-5-0-max"] {
		t.Errorf("opus-5-0-max 不该被问，turn_usage %d -> %d；完整输出:\n%s", before["opus-5-0-max"], got, out)
	}
}

// ux2rAskAmbiguous 跑一次 /ask opus，断言：报歧义那一行逐字、进程没起、没有「正在问」。
func ux2rAskAmbiguous(t *testing.T, fx *ux2rFix, input string) string {
	t.Helper()
	before := fx.usages(t, "opus-a", "opus-b", "opus")
	out := fx.run(t, input)
	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAmbiguous, "opus", "opus-a, opus-b"))
	fx.requireUsageUnchanged(t, before, out)
	if strings.Contains(out, "正在问") {
		t.Errorf("歧义路径上不该出现「正在问」；完整输出:\n%s", out)
	}
	return out
}

// 验收 8：两个候选都在场，歧义，不起进程。
func TestUx2rAskAmbiguousBothActive(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "opus-b")
	fx.seedActive(t, "opus-a", "")
	fx.seedActive(t, "opus-b", "")
	ux2rAskAmbiguous(t, fx, "/ask opus\n")
}

// 验收 9：歧义候选里有一个已退出。只在活跃成员里判歧义的实现会直接问 opus-b。
func TestUx2rAskAmbiguousIncludesLeft(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "opus-b")
	fx.seedLeft(t, "opus-a", "")
	fx.seedActive(t, "opus-b", "")
	ux2rAskAmbiguous(t, fx, "/ask opus\n")
}

// 验收 10（本版裁决）：歧义候选里有一个只在配置里、从没加入。
// 只在成员里判歧义的实现会直接问 opus-b，这条必红。
func TestUx2rAskAmbiguousIncludesConfigOnly(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "opus-b")
	fx.seedActive(t, "opus-b", "")
	ux2rAskAmbiguous(t, fx, "/ask opus\n")
}

// 验收 11：前缀命中已退出的成员，打 X-1 的「已退出」那句，两个 %s 都是全名。
func TestUx2rAskPrefixHitsLeftMember(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "grok-4.7")
	fx.seedLeft(t, "opus-a", "")
	before := fx.usages(t, "opus-a", "op")

	out := fx.run(t, "/ask op\n")

	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAskLeft, "opus-a", "opus-a"))
	fx.requireUsageUnchanged(t, before, out)
	if strings.Contains(out, "正在问") {
		t.Errorf("已退出路径上不该出现「正在问」；完整输出:\n%s", out)
	}
}

// 验收 12：前缀命中配置里未加入的，打 C1 那句，两个 %s 都是全名。
func TestUx2rAskPrefixHitsConfiguredNotJoined(t *testing.T) {
	fx := ux2rNewFix(t, "grok-4.7", "opus-a")
	before := fx.usages(t, "grok-4.7", "gr")

	out := fx.run(t, "/ask gr\n")

	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAskNotJoined, "grok-4.7", "grok-4.7"))
	fx.requireUsageUnchanged(t, before, out)
	if strings.Contains(out, "正在问") {
		t.Errorf("未加入路径上不该出现「正在问」；完整输出:\n%s", out)
	}
}

// 没找到：打 X-1 的 C2 那句，%s 用原始引用（契约 6.2 第 3 步）。
func TestUx2rAskNotFoundUsesC2WithRawRef(t *testing.T) {
	fx := ux2rNewFix(t, "grok-4.7", "opus-a")
	fx.seedActive(t, "opus-a", "")
	before := fx.usages(t, "zz", "opus-a", "grok-4.7")

	out := fx.run(t, "/ask zz\n")

	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAskUnknown, "zz"))
	fx.requireUsageUnchanged(t, before, out)
}

// 验收 13：带文本的歧义，正文不落库。
func TestUx2rAskAmbiguousWithTextNotPersisted(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "opus-b")
	fx.seedActive(t, "opus-a", "")
	fx.seedActive(t, "opus-b", "")
	beforeSeq, err := fx.svc.MaxSeq(context.Background(), ux2rConvID)
	if err != nil {
		t.Fatalf("读 MaxSeq 失败: %v", err)
	}
	beforeTotal := fx.st.totalMessages()

	out := ux2rAskAmbiguous(t, fx, "/ask opus 你好\n")

	afterSeq, err := fx.svc.MaxSeq(context.Background(), ux2rConvID)
	if err != nil {
		t.Fatalf("读 MaxSeq 失败: %v", err)
	}
	if afterSeq != beforeSeq || fx.st.totalMessages() != beforeTotal {
		t.Errorf("歧义时「你好」不该落库：MaxSeq %d -> %d，消息总数 %d -> %d；完整输出:\n%s",
			beforeSeq, afterSeq, beforeTotal, fx.st.totalMessages(), out)
	}
	if strings.Contains(out, "已记下") {
		t.Errorf("歧义时不该出现落库回显「已记下」；完整输出:\n%s", out)
	}
}

// 验收 14（B1，W1 blocker）：配置有 opus-5-0 与 opus-5-0-max，只有 max 在场。
// /ask opus-5-0 必须先在全集上判精确，打 C1 那句且句中是 opus-5-0；
// 先在成员里做前缀的实现会问成 opus-5-0-max，花一次入场费。
func TestUx2rAskB1ExactOnUniverseBeforeMemberPrefix(t *testing.T) {
	fx := ux2rNewFix(t, "opus-5-0", "opus-5-0-max")
	fx.seedActive(t, "opus-5-0-max", "")
	before := fx.usages(t, "opus-5-0", "opus-5-0-max")

	out := fx.run(t, "/ask opus-5-0\n")

	// 整行相等本身带词边界；再用词边界正则确认 opus-5-0 确实作为一个完整的词出现。
	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAskNotJoined, "opus-5-0", "opus-5-0"))
	if !ux2rWordRe("opus-5-0").MatchString(out) {
		t.Errorf("输出里 opus-5-0 没有作为完整的词出现；完整输出:\n%s", out)
	}
	if strings.Contains(out, "opus-5-0-max") {
		t.Errorf("输出里不该出现 opus-5-0-max；完整输出:\n%s", out)
	}
	fx.requireUsageUnchanged(t, before, out)
	if strings.Contains(out, "正在问") {
		t.Errorf("B1 路径上不该出现「正在问」；完整输出:\n%s", out)
	}
}
