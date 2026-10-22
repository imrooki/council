package tui

import (
	"context"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"council/internal/repl"
)

// program 是 Run 需要的 bubbletea Program 能力。
//
// 抽成接口只有一个目的：让「启动行在 Run 之前直写 final、运行期出口在 Run 之前装上」这条顺序有一条用例卡得住。
// 生产实现是 *tea.Program 本身。
type program interface {
	Send(msg tea.Msg)
	Run() (tea.Model, error)
}

// Run 起 TUI。in / stdout 必须是真句柄（isatty 已由调用方判过）。
//
// ctx 同时做两件事，缺一不可：
// 交给 tea.WithContext，根被取消时 p.Run 返回；
// 再交给 Model（Options.Ctx），在飞命令的 ctx 才能跟着取消，
// 走到 spawn.go 的 killProcessTree。只交给前者，进程退了子进程还在烧 token。
func Run(ctx context.Context, r *repl.REPL, out *LineWriter, version string, in, stdout *os.File) error {
	m := New(Options{
		Submit:  replAdapter{r: r},
		Out:     out,
		Version: version,
		Ctx:     ctx,
	})
	// 选项逐条冻结，不要再加：
	// WithContext：根 ctx 取消（SIGTERM、关窗）时 Run 返回 ErrProgramKilled。
	// WithInput / WithOutput：句柄由调用方判过 isatty，这里不再猜。
	// WithoutSignalHandler：不注册 bubbletea 自己那个 signal.Notify goroutine。
	// 信号由 main 的 signal.NotifyContext 管，经 WithContext 进来。
	// 不要换成 WithoutSignals：那个照常注册，只是收到之后忽略。
	// 不设 alt-screen。v2 没有 tea.WithAltScreen()；alt-screen 是 tea.View 的字段，
	// Model.View 平时把它写成 false（内联模式），只有复制模式那一帧临时写成 true
	// （见 copyscreen.go 的 View 分支）。
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(in),
		tea.WithOutput(stdout),
		tea.WithoutSignalHandler(),
	)
	// 观察者装在这里，不装进 runWith，也不挨着那里的两次 SetEmit。
	// runWith 的签名是 (program, *LineWriter, io.Writer)，手里没有 REPL，program 接口里也拿不到 REPL（只有 Send 与 Run）；
	// x39e 与 tui1d 直接调用 runWith，签名因此冻结。
	// 装在 p.Run 之前，与装在 runWith 第 1 步之前同一时序：都在任何 Submit 之前。
	// 第一个 Submit 只可能在事件循环起来之后由 Enter 触发，所以这里不会在 Run 之前 Send。
	// r != nil 这道门不能省：既有用例以 Run(ctx, nil, ...) 调用，没有它会空指针 panic。
	if r != nil {
		r.SetRoundObserver(newRoundObserver(p.Send))
	}
	// final 必须是 stdout 本身：启动横幅、在飞对账、漂移通知在 runWith 第 1 步直写它，
	// 换成别的句柄这些行就从屏幕上消失。TestTui1dRunFinalIsRealStdout 用管道钉着这一行。
	return runWith(p, out, stdout)
}

// runWith 是 Run 抽掉「怎么造 Program」之后的那部分，顺序全在这里。
// final 是 Run 返回之后的兜底出口（生产上就是真 stdout）。
//
// 顺序写反不会让编译失败，所以每一步为什么必须在这个位置，写在原地。
func runWith(p program, out *LineWriter, final io.Writer) error {
	// 第 1 步必须在 p.Run() 之前，而且不能经 p.Send。
	// 启动横幅、在飞对账、漂移审计写在 repl.New(...).Run(ctx) 之前，
	// 那时 Program 还没起来，LineWriter 的出口是 nil，完整行进 pending。
	// SetEmit 换上出口后立刻、同步把 pending 补吐。这些行直接写到 final
	// （生产上就是真 stdout）：内联模式下它们本来就该在活区上方，不经事件循环，
	// 所以这里不可能堵住。
	// 曾经把补吐接进 p.Send。(*tea.Program).Send 在 p.ctx 未取消时把消息送进无缓冲的
	// p.msgs；没人在读，因为 p.Run() 要等这次 SetEmit 返回才在下一行调用。
	// 补吐和 Send 又在同一个 goroutine 上，Send 永远堵住。根 ctx 被取消时 Send 才从
	// ctx.Done 那一支回来，所以 Ctrl-C 能退出、屏幕却一直空白。
	// 写反（先 Run 再把启动行补吐）时，横幅落在第一帧之后，位置漂移，甚至被活区盖住。
	out.SetEmit(func(line string) { fmt.Fprintln(final, line) })
	// 第 1b 步：换上运行期出口。此时 pending 已空，这一次 SetEmit 不会同步调用出口。
	// 第 1 步与第 1b 步之间没有别的写者（启动阶段 REPL 还没开始读输入）。
	// 运行期的行不直接写 final，否则会压坏内联活区。也不发一条自定义消息再让
	// Model.Update 返回 tea.Println 这个 Cmd：bubbletea 的 handleCommands 给每个 Cmd
	// 单开一个 goroutine，结果再各自 Send 回来，连续两行的到达顺序没有保证。
	// printLine 当场执行 tea.Println，把内部打印消息送进事件循环；事件循环在 Update
	// 之前按到达顺序处理它。一个写者 goroutine 连续写的行按写入顺序到达。
	// 空串在 printLine 里换成一个空格：bubbletea 的 insertAbove 对空串直接返回，
	// 分段空行会从屏幕上消失。
	// 程序退出后 Send 不会卡住：它等的是 Program 自己的 p.ctx，
	// 根 ctx 被取消时、或 Run 返回前 Program 自己 cancel 时，Send 都从 ctx.Done 那一支返回。
	out.SetEmit(func(line string) { p.Send(printLine(line)) })

	// 第 2 步。Run 的返回路径不止 quitCmd 那一条：
	// ctx 取消（SIGTERM、关窗）和 Run 自己出错时，Model.quitCmd 一次都没跑。
	model, err := p.Run()

	// 第 2b 步：擦掉留在屏幕上的最后一帧。必须在第 3 步换出口、第 4 步 Flush 之前：
	// 此后写到 final 的残段与收尾行才会从原帧的顶行开始，而不是落在旧帧下面。
	// 门与序列见 exitClearSeq；finalSupportsVT 只查询控制台模式，不改它。
	// 写失败不处理：清屏只是装饰，不能因为它丢掉第 3、4 步。
	if seq := exitClearSeq(model, err); seq != "" && finalSupportsVT(final) {
		_, _ = io.WriteString(final, seq)
	}

	// 第 3 步必须在 Run 返回之后、Flush 之前。
	//
	// 它买到的只有一件事：**此后再有人写 LineWriter，直接落到屏幕上，
	// 而不是投进一个已经死掉的事件循环**（p.Send 在 Run 返回后是空操作）。
	//
	// 它**买不到**「把事件循环死得太早、没送出去的行补吐回来」。这曾经写在
	// 冻结契约里，2026-09-23 被实测证伪：LineWriter 只在出口为 nil 时才把完整行
	// 放进 pending（writer.go 的 ingest，fn == nil 那一支）；第 1b 步装的是非 nil
	// 的 p.Send 包装，此后 pending 再也填不回去。行一旦交给一个空操作的 Send
	// 就消失了，LineWriter 手里不再有它，这里的 SetEmit 看到的是空 pending。
	// 要补上得让 program 带一个投递回执、让出口把落空的行退回来，
	// 而 (*tea.Program).Send 不返回任何东西。记为剩余项。
	//
	// 残余窗口，照实写下来：事件循环停掉、而 p.Run() 还没返回的那一瞬，
	// 此刻写进来的行会落空而丢失。微秒到毫秒级。Run 返回之后的写入由这一步接住，
	// 不在窗口内。今天关不掉，所以只记录，不声称已处理。
	//
	// 另外，正常退出这条路上它不是「第一次兜底」：Model.quitCmd 在 Quit 之前
	// 已经把出口换成自己的收集器并 Flush 过一遍，那些行由 tea.Sequence 打完了。
	// 这里只是把出口从那个收集器换成 stdout。
	// 真正靠它兜底的是 ctx 取消与 Run 出错两条路 —— 那两条上 quitCmd 一次都没跑。
	out.SetEmit(func(line string) { fmt.Fprintln(final, line) })

	// 第 4 步。残段不以换行结尾，任何 SetEmit 都拿不到它，必须再 Flush 一次。
	// 两步都要：只换出口不 Flush，残段留在 buf 里，进程一退就没了。
	out.Flush()

	// 第 5 步。原样返回，包括 nil。不要包一层。
	return err
}

// exitClearSeq 返回 Run 返回之后要写给 final 的清屏序列；不该写时返回 ""。
//
// bubbletea v2.0.9 退出时，close() 把光标移到帧的末行，只擦这一行
// （cursed_renderer.go 的 MoveTo(0, cellbuf 高-1) 加 ESC[J；cellbuf 高就是下面的 H），上面各行留在屏幕上。
// 这里从末行上移 H-1 行、回到行首、擦到屏尾。H 取 min(帧高, 真终端高)：
// 帧比终端高时渲染器丢掉帧顶的几行（cursed_renderer.go:349-351）。
//
// 四道门，全部满足才写：
//  1. Run 返回的是本包的 Model。Run 自己 recover 住 panic 时返回 nil，挡在这里。
//  2. err 为 nil，也就是正常退出（空闲时 2 秒内连按两次 Ctrl-C、/quit、出错退出都走 QuitMsg）：
//     退出前 bubbletea 一定先把最终帧画出来，光标停在帧末行。
//     任何非 nil 的 err 都不写：早退时一帧都没画；被杀（取消 ctx、SIGTERM、tty 读错误）时
//     若还没 flush 过，屏幕上也没有帧，而 termHeight > 0 只说明 Update 跑过，不是帧已上屏的证据；
//     Cmd goroutine 里的 panic 栈已经写进 stderr，光标位置未知。写了会擦掉用户的行。
//  3. Model 收到过宽高都 > 0 的 WindowSizeMsg（termHeight > 0）。宽 0 时渲染器什么都没画。
//  4. final 能解释 VT 序列：这一道在 runWith 里判（finalSupportsVT）。
func exitClearSeq(model tea.Model, err error) string {
	m, ok := model.(Model)
	if !ok {
		return ""
	}
	if err != nil {
		return ""
	}
	if m.termHeight <= 0 {
		return ""
	}
	h := min(m.frameHeight(), m.termHeight)
	if h >= 2 {
		return fmt.Sprintf("\x1b[%dA\r\x1b[J", h-1)
	}
	return "\r\x1b[J"
}

// replAdapter 把 *repl.REPL 适配成 Model 需要的 Submitter。
// IsQuit 是 repl 的包级函数、不是 REPL 的方法，所以需要这一层；
// 不在 repl 上加一个重复的方法，那会让同一件事有两个入口。
type replAdapter struct{ r *repl.REPL }

// Submit 原样转发。ctx 必须是调用方给的那个，不能换成 Background：
// 它的父是 tea.WithContext 用的根 ctx，根被取消时在飞命令才跟着取消。
func (a replAdapter) Submit(ctx context.Context, text string) error {
	return a.r.Submit(ctx, text)
}

// IsQuit 转发到包级函数 repl.IsQuit。用 errors.Is 的那一层在 repl 里，这里不再写一份。
func (a replAdapter) IsQuit(err error) bool {
	return repl.IsQuit(err)
}

// MentionCandidates 原样转发，让 replAdapter 满足 mentionSource。
// 只由 submitCmd 在 Submit 返回之后、同一个 goroutine 上调用。
func (a replAdapter) MentionCandidates(ctx context.Context) ([]repl.MentionCandidate, error) {
	return a.r.MentionCandidates(ctx)
}

// CopyRecent 原样转发，让 replAdapter 满足 copySource：TUI 的 /copy 不交给 Submit，
// 由 Model 在提交之前截下，再经这里取正文。读库只发生在这条 Cmd 的 goroutine 上。
func (a replAdapter) CopyRecent(ctx context.Context, args string) (string, error) {
	return a.r.CopyRecent(ctx, args)
}

// CopyScreen 原样转发，让 replAdapter 满足 copyScreenSource（TUI-2 的复制模式）。
// 与 CopyRecent 同一条：读库只发生在这条 Cmd 的 goroutine 上，Update 里不碰 SQLite。
func (a replAdapter) CopyScreen(ctx context.Context) ([]repl.CopyLine, bool, error) {
	return a.r.CopyScreen(ctx)
}
