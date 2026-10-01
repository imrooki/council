package repl

import (
	"errors"
	"fmt"
	"testing"
)

// 本文件补的是冻结契约 6.1 节的三条：IsQuit 至今没有任何用例，
// 而它是 TUI 的 Model 判「这一轮要不要退出」的唯一依据。
//
// 三条里只有第 17 条真有判别力，另外两条结构上几乎不可能为假，理由各自写在
// 对应的用例上方。为了让 16 与 18 不至于完全空转，这里各加了一句反向断言：
// 有了反向那一句，「IsQuit 恒返回 true」或「恒返回 false」这两种退化实现
// 才会被某一条抓住。

// 验收 16：IsQuit(errQuit) 为 true。
//
// 【判别力弱】errQuit 是 cmdQuit 直接返回的那个值，IsQuit 的实现无论写成
// errors.Is 还是 ==，这一条都过。它唯一能抓的是「IsQuit 恒为 false」或者
// 有人把 IsQuit 里的哨兵换成了另一个 error 值。
func TestTui1bIsQuitAcceptsSentinel(t *testing.T) {
	if !IsQuit(errQuit) {
		t.Fatalf("IsQuit(errQuit) 返回 false，应当为 true；errQuit = %v", errQuit)
	}
}

// 验收 17：包一层 %w 之后仍然认得出。
//
// 【有判别力】这一条钉的是「用 errors.Is 而不是 ==」。把 seam.go 里的
// errors.Is(err, errQuit) 改成 err == errQuit，本用例立刻红，而 16 与 18 照过。
// 第二段再包一层，钉的是「任意深度都要认得出」，而不是只剥一层。
// 第三段是反向的：%v 只把哨兵展开成文本，链子断了，IsQuit 必须返回 false。
// 没有第三段的话，「IsQuit 恒为 true」能把 16 和 17 一起骗过去。
func TestTui1bIsQuitSeesThroughWrapping(t *testing.T) {
	once := fmt.Errorf("外面包一层: %w", errQuit)
	if !IsQuit(once) {
		t.Errorf("IsQuit(%q) 返回 false；包一层 %%w 之后仍应认得出（要用 errors.Is，不能用 ==）", once)
	}

	twice := fmt.Errorf("再包一层: %w", once)
	if !IsQuit(twice) {
		t.Errorf("IsQuit(%q) 返回 false；包两层之后仍应认得出", twice)
	}

	// %v 不是 %w：错误链在这里断掉，只剩下一段文本。
	detached := fmt.Errorf("只展开成文本: %v", errQuit)
	if IsQuit(detached) {
		t.Errorf("IsQuit(%q) 返回 true；%%v 没有接上错误链，不该被认成 quit（这说明实现可能在比字符串）", detached)
	}
}

// 验收 18：别的错误与 nil 都是 false。
//
// 【判别力弱】errors.Is 对一个无关的 errors.New 和对 nil 本来就返回 false，
// 任何用 errors.Is 或 == 写出来的实现都过。它能抓的只有「IsQuit 恒为 true」。
// 附带钉住两条容易被写错的：同文本不同值的 error 不算 quit（不许比字符串），
// 以及包一层无关错误也不算。
func TestTui1bIsQuitRejectsOthers(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"另一个错误", errors.New("别的错")},
		{"nil", nil},
		{"文本相同但不是同一个值", errors.New(errQuit.Error())},
		{"包了一层无关错误", fmt.Errorf("外面包一层: %w", errors.New("别的错"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if IsQuit(tc.err) {
				t.Errorf("IsQuit(%v) 返回 true，应当为 false", tc.err)
			}
		})
	}
}
