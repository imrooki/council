package repl

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"council/internal/config"
)

// TestRunDoesNotDispatchAfterCancel 锁住规格 E 的公平性再检查。
//
// Run 的主循环里，一行输入与 ctx.Done() 可能同时就绪，此时 select 随机挑一个分支。
// 没有再检查的话，Ctrl+C 之后还会再执行一条命令。这个用例重复足够多轮，
// 让随机性必然暴露：只要有任何一轮执行了 /help，输出里就会出现它的表头。
func TestRunDoesNotDispatchAfterCancel(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{"claude": {Command: "claude"}},
	}

	const rounds = 50
	for i := 0; i < rounds; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 先取消，再给它一行随时可取的输入

		var out bytes.Buffer
		// 喂多行，让 readLines 的 goroutine 一定有货可送，
		// 从而制造「两个 case 同时就绪」的局面。
		in := strings.NewReader("/help\n/providers\n/agents\n")

		if err := New(cfg, nil, nil, in, &out, RulesContext{}).Run(ctx); err != nil {
			t.Fatalf("第 %d 轮：ctx 已取消时 Run 应当返回 nil，实际: %v", i, err)
		}

		got := out.String()
		for _, forbidden := range []string{"COMMAND", "PROVIDER"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("第 %d 轮：ctx 取消后仍然执行了命令，输出里出现了 %q：\n%s",
					i, forbidden, got)
			}
		}
	}
}

// TestNewToleratesNilOut 锁住四个参数一致兜底：cfg、svc、in、out 传 nil 都不应当 panic。
func TestNewToleratesNilOut(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("out 为 nil 时不应当 panic，实际: %v", r)
		}
	}()

	if err := New(nil, nil, nil, strings.NewReader("/help\n/quit\n"), nil, RulesContext{}).Run(context.Background()); err != nil {
		t.Fatalf("out 为 nil 时 Run 应当返回 nil，实际: %v", err)
	}
}
