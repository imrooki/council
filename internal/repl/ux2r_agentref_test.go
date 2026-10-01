package repl

// 验收 1-6：resolveAgentRef 这个纯函数（契约第 5、6.1 节）。

import (
	"errors"
	"strings"
	"testing"
)

// ux2rRequireRefErr 断言 err 是 *agentRefError，返回它。
func ux2rRequireRefErr(t *testing.T, err error) *agentRefError {
	t.Helper()
	if err == nil {
		t.Fatalf("期望 *agentRefError，实际 nil")
	}
	var re *agentRefError
	if !errors.As(err, &re) {
		t.Fatalf("期望 *agentRefError，实际 %T: %v", err, err)
	}
	return re
}

// 验收 1：完全相同优先。候选同时有 gk 与 gk2，gk 若只走前缀就永远是歧义。
func TestUx2rResolveExactWins(t *testing.T) {
	for _, cands := range [][]string{{"gk", "gk2"}, {"gk2", "gk"}} {
		got, err := resolveAgentRef("gk", cands)
		if err != nil {
			t.Fatalf("候选 %v、ref=gk：完全相同必须优先，实际报错 %v", cands, err)
		}
		if got != "gk" {
			t.Fatalf("候选 %v、ref=gk：期望 gk，实际 %q", cands, got)
		}
	}
}

// 验收 2：唯一前缀。
func TestUx2rResolveUniquePrefix(t *testing.T) {
	got, err := resolveAgentRef("op", []string{"opus-a", "grok-4.7"})
	if err != nil || got != "opus-a" {
		t.Fatalf("期望 (opus-a, nil)，实际 (%q, %v)", got, err)
	}
	// 点号 id 的前缀同样可用：/ask gr 是本刀的动机。
	got, err = resolveAgentRef("gr", []string{"opus-a", "grok-4.7"})
	if err != nil || got != "grok-4.7" {
		t.Fatalf("期望 (grok-4.7, nil)，实际 (%q, %v)", got, err)
	}
}

// 验收 3：歧义。候选故意乱序：Matches 必须是排序后的全部，且不返回任何一个。
// 另外钉住「不修改 candidates」：原地排序的实现会把调用方的切片改掉。
func TestUx2rResolveAmbiguousSortedAndNoPick(t *testing.T) {
	cands := []string{"opus-b", "opus-a"}
	got, err := resolveAgentRef("opus", cands)
	re := ux2rRequireRefErr(t, err)
	if got != "" {
		t.Errorf("歧义时不许返回任何一个候选，实际返回 %q", got)
	}
	if !ux2rSameStrings(re.Matches, []string{"opus-a", "opus-b"}) {
		t.Errorf("Matches 应当恰好是排序后的 [opus-a opus-b]，实际 %v", re.Matches)
	}
	if re.Ref != "opus" {
		t.Errorf("Ref 应当是原始引用 opus，实际 %q", re.Ref)
	}
	msg := re.Error()
	for _, want := range []string{"opus", "opus-a", "opus-b"} {
		if !strings.Contains(msg, want) {
			t.Errorf("歧义的 Error() 必须含 Ref 与全部 Matches，缺 %q：%q", want, msg)
		}
	}
	if !ux2rSameStrings(cands, []string{"opus-b", "opus-a"}) {
		t.Errorf("resolveAgentRef 不许修改 candidates，调用后变成了 %v", cands)
	}
}

// 验收 4：没找到时 Matches 为空。
func TestUx2rResolveNotFound(t *testing.T) {
	got, err := resolveAgentRef("zz", []string{"opus-a", "grok-4.7"})
	re := ux2rRequireRefErr(t, err)
	if got != "" {
		t.Errorf("没找到时不许返回候选，实际 %q", got)
	}
	if len(re.Matches) != 0 {
		t.Errorf("没找到时 Matches 应当为空，实际 %v", re.Matches)
	}
	if re.Ref != "zz" {
		t.Errorf("Ref 应当是 zz，实际 %q", re.Ref)
	}
	if !strings.Contains(re.Error(), "zz") {
		t.Errorf("Error() 必须含 Ref：%q", re.Error())
	}
}

// 验收 5：空串引用。候选非空时空串是所有 id 的前缀：只有一个候选时「唯一前缀」
// 会把它当成命中，多个时会变成「全部歧义」，两种都不许。
func TestUx2rResolveEmptyRef(t *testing.T) {
	for _, cands := range [][]string{{"gk"}, {"gk", "opus-a"}} {
		got, err := resolveAgentRef("", cands)
		re := ux2rRequireRefErr(t, err)
		if got != "" {
			t.Errorf("候选 %v、ref 为空串：不许返回候选，实际 %q", cands, got)
		}
		if len(re.Matches) != 0 {
			t.Errorf("候选 %v、ref 为空串：Matches 应当为空（不是歧义），实际 %v", cands, re.Matches)
		}
		if re.Ref != "" {
			t.Errorf("Ref 应当是空串，实际 %q", re.Ref)
		}
	}
}

// 验收 6：候选有重复不算歧义。
func TestUx2rResolveDuplicateCandidates(t *testing.T) {
	got, err := resolveAgentRef("gk", []string{"gk2", "gk2"})
	if err != nil || got != "gk2" {
		t.Fatalf("候选 [gk2 gk2]、ref=gk：期望 (gk2, nil)，实际 (%q, %v)", got, err)
	}
	// 重复与真歧义混在一起时，Matches 也要去重。
	_, err = resolveAgentRef("opus", []string{"opus-b", "opus-a", "opus-b"})
	re := ux2rRequireRefErr(t, err)
	if !ux2rSameStrings(re.Matches, []string{"opus-a", "opus-b"}) {
		t.Errorf("Matches 应当去重且排序为 [opus-a opus-b]，实际 %v", re.Matches)
	}
}
