package turn

// procid.go 是「子进程真的起来了」那条通知的挂载点（Plan §4.3 ⑨ 的方案 B）。
//
// 为什么走 context 而不是改 Spawner 签名：Spawner 是所有测试替身共享的签名
// （`type Spawner func(context.Context, provider.Command) (stdout, stderr []byte,
// exitCode int, killed bool, err error)`），把 PID 加进返回值要改每一个替身，
// 而这条信息只有生产实现（spawnProcess）拿得到。计划里把它列为「方案 A，
// 改动面大」，选 B 就是为了避开这个代价。
//
// 为什么 ctx 不会串台：一次 askUpToWith 自己造一个 spawnCtx（带超时），
// 钩子挂在那一个上，子进程结束就随之作废；父 ctx 不带钩子。

import "context"

// ProcessIDFunc 是 PID 通知的回调。
//
// 它在子进程的父进程（也就是 Council 自己）里同步执行一次，必须立刻返回：
// 执行时机在 cmd.Start() 之后、等 Waited 之前，任何阻塞都会拖慢这一轮。
type ProcessIDFunc func(pid int)

// processIDCtxKey 是私有类型，避免与别的包往同一个 ctx 里塞键时撞名。
type processIDCtxKey struct{}

// WithProcessIDObserver 把一次 PID 通知挂进 ctx。fn 为 nil 时原样返回 ctx
// （零值零开销）。同一 ctx 上后挂的覆盖先挂的：一次 spawn 只该有一个观察者。
func WithProcessIDObserver(ctx context.Context, fn ProcessIDFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, processIDCtxKey{}, fn)
}

// processIDObserverFrom 取出挂在 ctx 上的回调；没有就返回 nil。
func processIDObserverFrom(ctx context.Context) ProcessIDFunc {
	if ctx == nil {
		return nil
	}
	fn, _ := ctx.Value(processIDCtxKey{}).(ProcessIDFunc)
	return fn
}
