// Package repl 实现 council 的交互循环：读一行、分派、把结果写回注入的 io.Writer。
//
// Step 4 时本包是只读的壳，只把已加载的配置列出来。Step 5 起它多了一件事：
// 经 conversation.Service 读写本地事实源（建会话、加人、退人、发言、看历史）。
// 仍然不触发任何外部进程、不做任何 provider 调用 —— agent 说话是 Step 6 的事。
//
// 所有输出都走 New 注入的 io.Writer，包内禁止直接使用 fmt.Println 或 os.Stdout，
// 这样测试不需要 TTY。本包不认识 SQL，也不导入 internal/store：
// 持久化只经 conversation.Service 这一个入口。
package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/turn"
)

// prompt 是每次读取输入前写到 out 的提示符。
// 固定字面量、不含动态内容，便于测试对输出做稳定断言。
const prompt = "council> "

// multiPrompt 是多行输入模式下的提示符。它与 prompt 明显不同是有意的：
// 多行模式里不解释任何命令，用户必须一眼看出自己已经不在命令行上了。
const multiPrompt = "   ...> "

// msgFarewell 是 /quit、输入流结束、context 取消三种退出路径共用的告别行。
// 三者语义相同（规格 E 节：stdin 关闭等同 /quit），所以不区分措辞。
const msgFarewell = "再见"

// msgUnknownCommand 是未知斜杠命令的答复。
//
// 这里曾经还有一个 msgNotChat（「Step 4 尚不支持对话」），Step 5 删掉了：
// 从这一步起非斜杠输入不是错误，而是直接以 user 身份发言
// （docs/08_STEP5_SPEC.md H 节「裸文本即发言」）。
const msgUnknownCommand = "未知命令，试 /help"

// REPL 是一次交互会话的状态：配置快照、持久化入口、输入流、输出流、命令表、当前会话。
// 零值不可用，必须经 New 构造。
type REPL struct {
	cfg *config.Config
	svc *conversation.Service
	// rn 是编排层。nil 表示 provider 调用未就绪：/ask 与 /session 由
	// runCommand 凭 needsRunner 拦下，不会 panic。New 不得把 nil 换成别的 ——
	// nil 是合法状态，不是漏传。
	rn   *turn.Runner
	in   io.Reader
	out  io.Writer
	cmds *registry

	// cur 是当前会话，nil 表示还没选。/new、/use 会改它；/team use
	// 绑完之后也会用 Conversation 的读回整份替换它（库才是 TeamID 的真相，
	// 缓存副本不会自己跟着 BindTeam 变）。
	// 需要当前会话的命令在 dispatch 里被统一拦下，所以命令函数里可以直接解引用。
	cur *conversation.Conversation

	// inDraft 为 true 表示正处在多行输入模式，draft 是已经收到的行。
	//
	// 这个模式存在的理由是「裸文本即发言」对手敲的输入是对的、对粘贴的输入是错的：
	// 读循环按行切，粘贴一段三行的需求会变成三条各自独立的永久消息，
	// 而 Canonical Conversation 只追加不回收。多行模式把一次粘贴收成一条消息。
	//
	// draft 里的行一律原样保存，不做 TrimSpace，也不解释斜杠命令 ——
	// 缩进与以 / 开头的行都是正文的一部分。只有两行是特殊的，见 msgDraftEnd 与 msgDraftCancel。
	inDraft bool
	draft   []string
	// inPaste 为 true 表示正处在括号粘贴收集中，paste 是已经收到的行。
	// 与 inDraft **分开**：END 只结束粘贴层，绝不能拿去结束草稿层 ——
	// 用户已经在 /say 草稿里、中途粘一段进来时，用 END 去结束草稿会把他
	// 还没写完的那份提前发出去，而事实源只追加不回收。
	inPaste bool
	paste   []string
	// pastePoisoned 表示这份粘贴里出现过非法 UTF-8，整份作废。
	pastePoisoned bool

	// draftPoisoned 表示这份草稿已经作废（目前唯一的原因是读到非法 UTF-8），
	// 但仍留在多行模式里继续把后续行吃掉。
	//
	// 为什么不直接退出模式：粘贴剩下的行会退回单行模式，一条条变成永久消息 ——
	// 正是多行模式要消灭的那个问题。为什么不只丢坏的那一行：那会把一段粘贴
	// 悄悄截去一截，用户看不出少了什么。所以吃掉、不发送、让用户显式退出。
	draftPoisoned bool

	// rulesCtx 是 /rules 与 /rules reload 需要的、REPL 之外的上下文：
	// 生效的配置文件路径（reload 时解析 rules_file 的相对路径要用）与
	// 启动时读到的那份 config.Rules（显示路径与 found 状态要用）。
	// /rules reload --rebuild 成功之后会更新 rulesCtx.Rules，否则之后的
	// /rules 会显示旧的——它不是只读快照，是 REPL 手里权威的"上一次读到
	// 的规则是什么"。
	rulesCtx RulesContext

	// cfgLayers 是当前 r.cfg 合并自哪几层，高优先级在前（LoadMerged 的
	// MergedConfig.Layers）。空表示还没重读过：app 装配时只交一份合并结果，
	// 没有层信息可传。它只用于诊断（/config 显示「合并了哪几份」），
	// 不参与任何判定；/config reload --apply 与 /add 写入成功后更新。
	cfgLayers []string
	// rulesFileBase 是 rules_file 相对路径的解析基准：定义 rules_file 的
	// 那一层所在目录（LoadMerged 的 MergedConfig.RulesFileBase）。空串时
	// /rules reload 回落到 rulesCtx.ConfigPath 所在目录——单文件下两者
	// 本来就是同一个目录，合并之后才可能分叉（rules_file 写在全局层、
	// 项目层没写）。与 cfgLayers 一样，只在重读成功后更新。
	rulesFileBase string

	// roundObs 是 /round 的观察者。零值 nil：行式路径不装，runOneRound 一个事件都不发。
	// 只许在还没有 Submit 在跑的时候经 SetRoundObserver 改（TUI 在 p.Run 之前装）。
	roundObs RoundObserver

	// concise 是输出档位。零值 false 是详细（今天的输出）。
	// 只经 SetConcise 与 /verbose 改，读它的打印点见 verbosity.go 文件头。
	concise bool
	// now 是总用时用的时钟。nil 时用 time.Now，测试注入。
	now func() time.Time
	// autoReply 为 true 时裸文本落库之后自动问一轮（#47 对话模式，见 autoreply.go）。
	// 零值 false 是只记下（今天的行为），所以直接 New 出来的 REPL（既有测试）行为不变；
	// app 装配时按 Options.NoAuto 设定（默认打开）。只经 SetAutoReply 与 /auto 改。
	autoReply bool

	// discover 是识别模型用的执行器。New 装成 turn.RunDiscovery，测试换成夹具。
	discover provider.DiscoveryExec
	// discoverTimeout 是每一家识别的超时。New 装成 discoveryTimeout。
	discoverTimeout time.Duration
	// modelSnap 是本会话的识别快照。nil 表示还没识别过。
	modelSnap *modelSnapshot
	// detectCLIs 是 /add 用来找「本机装了、配置里还没有」的工具的检测（#48）。
	// 零值 nil 是不检测、不打提示；只经 SetCLIDetector 装，app 装 config.DetectProviders。
	detectCLIs func() []config.DetectedProvider
	// loginStatus 是 /add 跑登录状态命令用的执行器（#48，OB3，见 addlogin.go）。
	// 零值 nil 是不做登录检测、不打登录段；只经 SetLoginStatusExec 装，app 装 turn.RunLoginStatus。
	loginStatus provider.LoginStatusExec
	// addByName 为 true 时，/add <名字> 在配置里对不上就到识别快照里按名字找（AD1，见 addname.go）。
	// 零值 false 是关：/add 的行为与 AD1 之前逐字节相同（splitAddArgs 按全角空格切段那一条除外，
	// 它不看开关）；只经 SetAddByName 改，app 装配时打开。
	addByName bool

	// projectDir 是启动目录的规范化绝对路径（NormalizeProjectDir 的结果）。
	// /new 建会话时盖在会话上；/resume 默认只列这个目录下建的会话。
	// 零值空串表示没有项目目录（取不到启动目录，或直接 New 出来的 REPL），
	// 此时 /new 不盖章、/resume 列出全部，行为与没有这个功能时一致，
	// 所以所有既有测试不受影响。只经 SetProjectDir 设，只许在 Run 或第一次 Submit 之前调。
	projectDir string
}

// RulesContext 是 REPL 显示与重载规则文件所需的上下文，由 app 在启动时
// 一次性构造好交给 New。
type RulesContext struct {
	// ConfigPath 是启动时生效的最高优先层路径（多层合并时是项目层）：
	// /config reload 与 /add 要靠它认出「这次启动读的就是这一份」，
	// /rules reload 在没有 RulesFileBase 时拿它的目录当相对 rules_file
	// 的基准。
	ConfigPath string
	RulesFile  string       // 配置里写的 rules_file 原值（可能是相对路径），转交给 config.LoadRules
	Rules      config.Rules // 启动时读到的那一份
}

// New 构造一个 REPL。out 是唯一的输出出口。
// cfg 为 nil 时按空配置处理，这样 REPL 不会因为调用方传 nil 而 panic；
// 正常路径上 app 一定传一份已校验的配置。
//
// svc 为 nil 表示持久化没起来：此时 Step 4 的五个只读命令照常可用，
// Step 5 新增的八个命令一律回一句提示而不是 panic（见 08a 契约）。
//
// rulesCtx 是 Step 8 加的第六个参数，测试里传 RulesContext{} 即可 ——
// 零值是合法状态：ConfigPath 空串时 /rules reload 会去读
// filepath.Dir("") 也就是 "."，那是测试自己要避免撞到的边界，不是本函数
// 要兜底的情形（生产路径上 app 永远传一个非空的生效配置路径）。
func New(cfg *config.Config, svc *conversation.Service, rn *turn.Runner, in io.Reader, out io.Writer, rulesCtx RulesContext) *REPL {
	if cfg == nil {
		cfg = &config.Config{}
	}
	// out 与 cfg、in 一样做兜底：几个参数的容错风格保持一致，
	// 免得别人照着 cfg 传 nil 的写法传 out，在 io.WriteString 那里 panic。
	if out == nil {
		out = io.Discard
	}
	return &REPL{
		cfg:      cfg,
		svc:      svc,
		rn:       rn,
		in:       in,
		out:      out,
		cmds:     newCommandRegistry(),
		rulesCtx: rulesCtx,

		discover:        turn.RunDiscovery,
		discoverTimeout: discoveryTimeout,
	}
}

// Run 阻塞直到 /quit、输入流结束（EOF 等同 /quit）、或 ctx 被取消。
// 这三条路径都属于正常结束，返回 nil；只有写输出失败才返回非 nil 错误。
//
// ctx 一路传给 dispatch 再传给命令函数：需要 ctx 的命令用的是这一个，
// 不许在命令里自己开 context.Background()，否则 Ctrl+C 取消不掉正在跑的查询。
func (r *REPL) Run(ctx context.Context) error {
	lines := make(chan string)
	go readLines(r.in, lines)

	for {
		if err := r.write(r.currentPrompt()); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			// 第一次 Ctrl+C 走到这里。此刻光标停在提示符后面且没有回显换行，
			// 所以先补一个换行再打告别行。
			if err := r.write("\n"); err != nil {
				return err
			}
			return r.farewell()

		case line, ok := <-lines:
			if !ok {
				// 输入流结束（管道读完、Ctrl+Z 回车），等同 /quit。
				if err := r.write("\n"); err != nil {
					return err
				}
				return r.farewell()
			}
			// 公平性再检查（规格 E）：上面的 select 在「有一行输入」与「ctx 已取消」
			// 同时就绪时是随机挑的，可能在 Ctrl+C 之后还把这一行当命令执行。
			// 这里非阻塞地再查一次，取消了就直接走退出路径，不执行这一行。
			if ctx.Err() != nil {
				if err := r.write("\n"); err != nil {
					return err
				}
				return r.farewell()
			}

			err := r.dispatch(ctx, line)
			if errors.Is(err, errQuit) {
				return r.farewell()
			}
			if err != nil {
				return err
			}
		}
	}
}

// readLines 在独立 goroutine 里做阻塞读，把每一行送进 lines，读完或出错时关闭 lines。
//
// 权衡（规格 E 节已认可）：Windows 上没有可移植的办法打断一个正卡在 console read 上的读操作。
// 因此 ctx 被取消时，本 goroutine 不会被唤醒，而是继续卡在 Read 里，直到进程退出随之消失。
// 具体后果是：主循环返回后，本 goroutine 可能永远卡在 Read 上，
// 也可能卡在往无人接收的 lines 上发送（channel 无缓冲）。
// 两种情况都不持有需要释放的资源，主循环也不等待它结束，所以泄漏止于进程生命周期。
// 代价是在测试进程里这些 goroutine 会累积到测试结束；这是刻意接受的，
// 换取的是 ctx 取消能被立刻响应，而不是被一次阻塞读挡住。
//
// 扫描错误（例如单行超长）与 EOF 一样按输入结束处理：这里的交互只有短命令与短发言，
// 没有必要为这种输入设计一条恢复路径。
func readLines(in io.Reader, lines chan<- string) {
	defer close(lines)

	if in == nil {
		return
	}

	sc := bufio.NewScanner(in)
	// bufio.Scanner 默认单行上限 64 KiB，这里放宽到 1 MiB，
	// 免得粘贴一长段文字时整个输入流被当成结束。
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	// bufio.ScanLines 会顺带去掉行尾的 CR，所以 CRLF 输入不需要另外处理。
	for sc.Scan() {
		lines <- sc.Text()
	}
}

// currentPrompt 返回当前该打的提示符：多行输入模式下换一个，其余时候是常规提示符。
func (r *REPL) currentPrompt() string {
	if r.inDraft {
		return multiPrompt
	}
	return prompt
}

// farewell 打印告别行，用于三条正常退出路径。
//
// 退出时若还攒着一份多行草稿，**丢掉，不发送**，并明说丢了。
// Canonical Conversation 只追加不回收，把一份被 Ctrl+C 或 EOF 打断的、
// 用户从没确认过的草稿写成永久记录，比丢掉它糟得多。
func (r *REPL) farewell() error {
	if r.inPaste {
		if err := r.printf("%s\n", msgPasteDiscarded); err != nil {
			return err
		}
	}
	if r.inDraft {
		if err := r.printf("%s\n", msgDraftDiscarded); err != nil {
			return err
		}
	}
	return r.write(msgFarewell + "\n")
}

// write 把一段文本写到注入的 out，写失败时带上下文包装返回。
func (r *REPL) write(s string) error {
	if _, err := io.WriteString(r.out, s); err != nil {
		return fmt.Errorf("repl: 写输出失败: %w", err)
	}
	return nil
}

// printf 是 write 的格式化版本。
func (r *REPL) printf(format string, args ...any) error {
	return r.write(fmt.Sprintf(format, args...))
}

// reportErr 把一次领域调用的失败打给用户，然后按「已处理」返回。
// 持久化出错不该把整个 REPL 打死：和未知命令一样，打一行继续读下一条。
// 只有写输出本身失败才会向上冒泡，因为那时连提示都送不出去。
func (r *REPL) reportErr(what string, err error) error {
	return r.printf("%s: %v\n", what, err)
}

// newTabWriter 返回列表命令共用的 tabwriter 配置：左对齐、列间两个空格。
// 调用方必须记得 Flush，否则什么都不会写出去。
//
// 注意 tabwriter 按 rune 数算列宽（text/tabwriter 里是 utf8.RuneCount），
// 而中文在终端里占两格，于是每个中文字符都会被少算一格，
// 所以可能含中文的字段（display_name、会话标题、消息正文）一律放最后一列，
// 后面没有列可以被它撑歪。（这里原先写的是「按字节数算」，结论对、机制错，2026-09-23 订正。）
//
// 参数是 io.Writer 而不是取 r.out：/help 的格式要被 council commands 复用，
// 那份代码拿到的只有输出流，没有 REPL。列宽只有这一处定义。
func newTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

// newTabWriter 是 newTabWriter(r.out) 的便捷写法，供各命令使用。
func (r *REPL) newTabWriter() *tabwriter.Writer {
	return newTabWriter(r.out)
}
