package provider_test

import (
	"regexp"
	"strings"
	"testing"
)

// v4UUIDPattern 校验 RFC 4122 version 4 的 8-4-4-4-12 形状：版本位固定是 4，
// variant 位落在 8/9/a/b 之一。newV4UUID（internal/provider/uuid.go）用
// fmt.Sprintf("%x", ...) 输出小写，但契约没有冻结大小写，这里用 (?i) 兼容两种。
var v4UUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func isV4UUID(s string) bool {
	return v4UUIDPattern.MatchString(s)
}

// mustReplaceOnce 在一份真实探针原件上做一处最小、可追溯的字节改动，用来构造
// "同一份真实输出，只改一个字段"的派生 fixture（例如把会话 id 字段清空来测
// ErrNoSessionID）。用 strings.Count 强制断言 old 在 s 里恰好出现一次：
// 如果以后有人改了上面那批常量、这处替换悄悄失效（0 次命中）或者悄悄改了
// 别的地方（多次命中），测试要当场炸掉，而不是安静地测着一份跟预期不一样的
// 输入。
func mustReplaceOnce(t *testing.T, s, old, new string) string {
	t.Helper()
	if n := strings.Count(s, old); n != 1 {
		t.Fatalf("mustReplaceOnce: %q occurs %d times in fixture, want exactly 1", old, n)
	}
	return strings.Replace(s, old, new, 1)
}

// containsExact 断言 args 里存在某个元素与 want 逐字节相等，不关心它在哪个下标——
// 09a 冻结的是每个 provider 的命令行"形态"（B 节的格式字符串），没有一处显式
// 冻结"prompt 一定是 Args 的最后一个元素"这条位置关系（claude 那一行的格式字符串
// 结尾写了 <prompt>，但 grok / agy 那两行没有），所以用内容匹配而不是下标匹配。
func containsExact(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// containsSubstring 断言 list 里有某一项包含给定子串。Usage.Unreported 的具体
// 措辞不在契约面（09a："Capabilities.UsageFormula 的具体字符串措辞...不在"一节
// 没有点名 Unreported，但同一份文档反复强调"错误信息的具体测辞...不在契约面"，
// 本包对人类可读的诊断性字符串统一按这个尺度处理：断言语义关键词存在，不锁死
// 精确拼写）。
func containsSubstring(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// indexOf 返回 want 在 args 里第一次出现的下标，找不到返回 -1。
func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

// removeFlagPair 断言 flag 在 args 里恰好出现一次，并返回去掉 [flag, 紧随的值]
// 这两个元素之后剩下的切片。这是"首轮 vs 后续轮唯一的差别是会话 flag"那组测试
// 的核心工具：把两条 Args 各自摘掉会话 flag 段之后再比较，若剩下的部分逐元素
// 相等，就证明了这一整段格式字符串里只有会话 flag 那两格会随 ProviderSession
// 是否为空而变化，别的任何一个字节都不受影响——恰好是 D-4 缓存前缀稳定性要求
// 测试钉住的那件事。
func removeFlagPair(t *testing.T, args []string, flag string) []string {
	t.Helper()
	i := indexOf(args, flag)
	if i < 0 {
		t.Fatalf("removeFlagPair: flag %q not found in %v", flag, args)
	}
	if i+1 >= len(args) {
		t.Fatalf("removeFlagPair: flag %q has no following value in %v", flag, args)
	}
	// 也顺带确认只出现一次：BuildTurn 每轮只发一次会话 flag，重复出现说明
	// 拼接逻辑本身就坏了，这里没有必要单独开一条测试。
	rest := append(append([]string{}, args[:i]...), args[i+2:]...)
	if indexOf(rest, flag) >= 0 {
		t.Fatalf("removeFlagPair: flag %q appears more than once in %v", flag, args)
	}
	return rest
}
