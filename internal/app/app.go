// Package app 装配 council 的启动流程：解析配置路径、加载并校验配置、
// 解析数据目录、打开本地库、打印启动横幅，最后把控制权交给 REPL。
// main 只负责把结果收敛成退出码，业务都在这里，这样整条启动路径可以在测试里
// 用内存中的 Reader / Writer 驱动，不需要真实的 TTY。
//
// Step 5 起本包会创建数据目录并打开 SQLite 库（由 internal/store 负责建目录与迁移）。
// 这是相对 Step 4「不创建任何目录」的一处有意改变：从这一步起程序真的要往那里写东西。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/headless"
	"council/internal/repl"
	"council/internal/store"
	"council/internal/tui"
	"council/internal/turn"

	tea "charm.land/bubbletea/v2"
)

// Options 是 Run 的全部外部依赖。In / Out / Err 由调用方注入，
// 测试时换成 strings.Reader 与 bytes.Buffer 即可，不碰进程的标准流。
type Options struct {
	ConfigPath string // 来自 --config，未给则空串
	Version    string // 打进启动横幅的版本号，由 cmd/council 传入；空串表示不显示这一行
	In         io.Reader
	Out        io.Writer
	Err        io.Writer

	// TUI 为 true 时走 internal/tui 的界面；为 false 时走行式 REPL.Run。
	// 由调用方（cmd/council）判 isatty 并结合 -no-tui 决定，本包不自己判：
	// Options.In / Out 是 io.Reader / io.Writer，拿不到句柄。
	TUI bool

	// InFile / OutFile 是 TUI 用的真句柄。TUI 为假时可以是 nil，本包不碰。
	// 不能从 In / Out 里取：那两个是接口，测试里是内存 Reader / Writer。
	InFile  *os.File
	OutFile *os.File

	// Verbose 为 true 时 REPL 用详细档（今天的输出）。
	// 零值是简洁，所以 app.Run 的默认是简洁（#45）。由 cmd/council 的 -verbose 设。
	// 测试助手 runAppForTest 设为 true（它的既有用例都按详细档标定）。
	Verbose bool

	// NoAuto 为 true 时裸文本只记下，不自动让成员回复（#47 对话模式）。
	// 零值是自动回复，所以 app.Run 的默认是对话模式。由 cmd/council 的 -no-auto 设。
	// 测试助手 runAppForTest 设为 true（它的既有用例都按只记下标定）。
	NoAuto bool

	// Headless 非 nil 时不做 TUI 也不进入行式 REPL，跑一次 ask 就返回。
	// 装配（配置、规则、库）仍走本函数的前半段，这样 headless 与交互模式
	// 看到的是同一份生效配置、同一个 data_dir 锁。
	Headless *headless.Options
}

// Run 按固定顺序做五件事：求配置文件路径 -> 加载并校验 -> 求数据目录 ->
// 打开 store（含建目录与迁移）-> 起 REPL，退出时关闭 store。
// 走默认位置而那个文件不存在时，求完路径先走首次运行（firstrun.go，#48），
// 生成配置之后照常往下；首次运行的两种失败结局在这里返回错误，被取消返回 nil。
//
// 配置加载或校验失败时，把全部错误逐行写进 opts.Err 并返回错误，不进 REPL。
// store 打不开同样是致命的：错误写进 opts.Err 并返回错误，不进 REPL ——
// 没有本地事实源的 REPL 只能对着用户干瞪眼，不如一开始就说清楚。
// 关闭 store 失败只往 opts.Err 记一行，不改返回值：数据已经提交了，关不上不影响正确性。
// ctx 被取消时 REPL 自行返回，Run 随之返回 nil，由调用方给出退出码 0。
func Run(ctx context.Context, opts Options) error {
	in := opts.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	errOut := opts.Err
	if errOut == nil {
		errOut = io.Discard
	}

	// TUI 时把屏幕出口换成 LineWriter，且必须在 writeBanner 之前。
	// 横幅、在飞对账、漂移通知、repl.New 的 out 全都写这个局部变量 out
	// （见本函数后半：writeBanner / writeLostNotice / writeDriftNotice / repl.New）。
	// 换掉 out，四处一起换掉，那三个 write* 一个字不用动。
	//
	// 此时 Program 还没造出来，出口传 nil：完整行进 pending，
	// tui.Run 在 p.Run() 之前把它们直接写到终端。
	// 写晚了（在 p.Run() 之后），横幅落在第一帧之后。
	//
	// errOut 不动，仍是 opts.Err。写 errOut 的地方都在 Program 起来之前
	// 或退出之后，直写不会撞进活区。
	if opts.TUI {
		lw := tui.NewLineWriter(nil)
		out = lw
	}

	path, err := config.ResolveConfigPath(opts.ConfigPath)
	if err != nil {
		fmt.Fprintf(errOut, "无法确定配置文件位置: %v\n", err)
		fmt.Fprintf(errOut, "可以用 --config <path> 直接指定配置文件，或设置环境变量 %s 指向它。\n", config.EnvConfigPath)
		return fmt.Errorf("解析配置文件路径失败: %w", err)
	}

	// explicit 为真表示用户自己指定过配置位置：--config，或非空的 COUNCIL_CONFIG
	// （空串与 ResolveConfigPath 同一口径，算没设）。
	explicit := opts.ConfigPath != "" || os.Getenv(config.EnvConfigPath) != ""
	layers, err := config.ResolveConfigLayers(opts.ConfigPath)
	if err != nil {
		fmt.Fprintf(errOut, "无法确定配置文件位置: %v\n", err)
		fmt.Fprintf(errOut, "可以用 --config <path> 直接指定配置文件，或设置环境变量 %s 指向它。\n", config.EnvConfigPath)
		return fmt.Errorf("解析配置文件路径失败: %w", err)
	}

	// 首次运行（#48）：走默认位置、而且那个文件不存在，才替用户生成配置。
	// 用户指定了路径却找不到，多半是打错字，照旧报错、不建文件。
	// Stat 的其它错误（例如没有权限、文件名不合法）不进首次运行，交给下面的
	// LoadMerged 照常报。默认路径不是绝对路径（USERPROFILE 被设成了相对路径）时
	// 也不进：那样会在启动目录下建文件。
	//
	// 只在「没有任何项目层」时进：项目里已经有 .council/config.toml 时，用户
	// 已经表达过配置在哪，不该再往家里写一份全局配置。layers 的长度与
	// Project 标记正好编码了这个判断——一份都没有时它是一条非项目的全局占位层。
	if !explicit && filepath.IsAbs(path) && onlyDefaultLayer(layers, path) {
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			if frErr := firstRun(ctx, errOut, path, opts.NoAuto); frErr != nil {
				if errors.Is(frErr, errFirstRunCanceled) {
					return nil
				}
				return frErr
			}
			// 首次运行刚把文件建好（或发现别人建好了），重新求一次层：
			// 上面那份 layers 还是启动时「什么都没有」的快照。
			layers, err = config.ResolveConfigLayers(opts.ConfigPath)
			if err != nil {
				fmt.Fprintf(errOut, "无法确定配置文件位置: %v\n", err)
				fmt.Fprintf(errOut, "可以用 --config <path> 直接指定配置文件，或设置环境变量 %s 指向它。\n", config.EnvConfigPath)
				return fmt.Errorf("解析配置文件路径失败: %w", err)
			}
		}
	}

	// 合并版加载：项目层覆盖全局层，最后跑同一套 Validate。repl 这一侧也走
	// 合并（internal/repl/configlayers.go）：/add 与 /config reload 按默认位置
	// 重新求层，与这里同一个口径；用户显式给了 -config 或 COUNCIL_CONFIG 时
	// 退回单层，只读他点名的那一份。config.Load 的单文件语义本身没有变。
	primaryPath := layers[0].Path
	merged, err := config.LoadMerged(layers)
	if err != nil {
		reportConfigError(errOut, primaryPath, err, explicit)
		return fmt.Errorf("加载配置失败 (%s): %w", primaryPath, err)
	}
	cfg := merged.Config
	// 合并提示只进详细档：默认档静默，否则每次启动都多一行「项目里的
	// data_dir 被忽略」，对没有写这个键的用户是纯噪声。
	if opts.Verbose {
		for _, note := range merged.Notices {
			fmt.Fprintln(errOut, note)
		}
	}

	// 规则文件跟着配置一起读，一次。读不出来与配置读不出来同一套处置：
	// 写 Err、不进 REPL。把读失败当成空规则，会让一次 I/O 故障静默地把
	// 成员的指令环境掏空，而下一轮照样花钱跑完、退出码 0（11 节 1.1）。
	// 相对路径按定义 rules_file 的那一层解析，不是按最高优先层猜。
	rules, err := config.LoadRules(merged.RulesFileBase, cfg.RulesFile)
	if err != nil {
		fmt.Fprintf(errOut, "规则文件读不出来: %v\n", err)
		fmt.Fprintf(errOut, "全局规则找的是 %s（可用环境变量 %s 改），局部规则由配置里的 rules_file 指定。\n",
			"~/.council/"+config.DefaultGlobalRulesName, config.EnvGlobalRules)
		fmt.Fprintf(errOut, "文件不存在是正常的（等于没有规则）；这里报错说明它存在却读不了。\n")
		return fmt.Errorf("加载规则文件失败: %w", err)
	}

	dataDir, err := config.ResolveDataDir(cfg.DataDir)
	if err != nil {
		fmt.Fprintf(errOut, "数据目录无效: %v\n", err)
		fmt.Fprintf(errOut, "data_dir 必须是绝对路径，可以写在配置文件的 data_dir 里，或设置环境变量 %s。\n", config.EnvDataDir)
		return fmt.Errorf("解析数据目录失败: %w", err)
	}

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		fmt.Fprintf(errOut, "无法打开本地库: %v\n", err)
		fmt.Fprintf(errOut, "数据目录是 %s，可以在配置文件的 data_dir 里改，或设置环境变量 %s。\n", dataDir, config.EnvDataDir)
		// data_dir 排他锁（未决 #8）拦下的这种失败，没有再单独补一句：
		// store.ErrDataDirLocked 的错误文本本身已经带了锁文件路径、
		// "同一个 data_dir 只能同时跑一个 council 实例"这句事实、以及
		// "确认没有其它进程还在用再重试"这个下一步动作，%v 会把这些原样打出来。
		// 这里再补一句只会是文字重复，不是额外信息。
		return fmt.Errorf("打开本地库失败 (%s): %w", dataDir, err)
	}
	// 紧跟在 Open 成功之后注册，保证下面每一条返回路径都会关库，
	// 包括 REPL 出错与 ctx 取消那两条。关闭失败只记一行，不动返回值。
	defer func() {
		if cerr := st.Close(); cerr != nil {
			fmt.Fprintf(errOut, "关闭本地库失败: %v\n", cerr)
		}
	}()

	// 横幅要报已有会话数，所以这里读一次。刚打开就读不出来，说明库虽然开着但不可用，
	// 按启动失败处理：与其让用户进了 REPL 每敲一条命令报一次错，不如在门口挡住。
	convs, err := st.Conversations(ctx)
	if err != nil {
		fmt.Fprintf(errOut, "读取会话列表失败: %v\n", err)
		fmt.Fprintf(errOut, "库文件是 %s。\n", st.Path())
		return fmt.Errorf("读取会话列表失败: %w", err)
	}

	// 启动目录就是项目目录，会话靠它归属。取不到不报错、不拦启动：
	// projectDir 保持空串，横幅写「未知」，/new 不盖章，/resume 列出全部。
	// 成员 CLI 的工作目录仍是下面的 filepath.Join(dataDir, "workdir")，
	// 与项目目录无关，那一行不要改成启动目录。
	projectDir := ""
	if wd, wdErr := os.Getwd(); wdErr == nil {
		projectDir = repl.NormalizeProjectDir(wd)
	}

	// headless 的 stdout 只能有那一个 JSON 对象（或 text 模式的结果），
	// 横幅与下面两类通知会把它变成「JSON 前面还有几行」的不可解析输出，
	// 所以横幅整段不写，通知改走 errOut。
	if opts.Headless == nil {
		// 合并了多层时把其余层写在同一行：用户才看得出家目录那份配置也
		// 参与了，不会以为项目文件里凭空多出了 provider。
		shownConfigPath := primaryPath
		if len(merged.Layers) > 1 {
			shownConfigPath += "（合并 " + strings.Join(merged.Layers[1:], "、") + "）"
		}
		writeBanner(out, opts.Version, shownConfigPath, dataDir, st.Path(), convs, projectDir, cfg)
	}
	noticeOut := out
	if opts.Headless != nil {
		noticeOut = errOut
	}

	svc := conversation.NewService(st)
	// 存储开关（未决 #9(a)）在装配阶段接上，只调一次：cfg.StoreThought 默认
	// true（config 层已经堵了「裸 bool 默认值拿不到」那个坑），这里原样传
	// 给 Service，由它决定 Post / RecordTurn 要不要把非空 Thought 落成空串。
	svc.SetThoughtPolicy(cfg.StoreThought)
	// 工作目录不再在启动时一次建齐：11c 契约第 4 节，有了会话维度之后
	// （WORKSPACE 档的路径含 convID）启动时根本不知道会有哪些会话，这个
	// 循环建不全。改成每轮起进程之前在 Runner.Ask 里现建（agentWorkDir +
	// os.MkdirAll），建不出来按 refuse 处理，不起进程。
	// svc 为 nil 时不要构造 Runner：那种情况下 /ask 与 /session 本来就该回「持久化未就绪」，
	// 还轮不到说 provider 调用未就绪。
	var rn *turn.Runner
	if svc != nil {
		workDir := filepath.Join(dataDir, "workdir")
		rn = turn.NewRunner(cfg, svc, workDir, turn.RulesBlocks{
			Global: rules.GlobalText,
			Local:  rules.LocalText,
		})
		// 启动对账排在漂移审计之前。顺序保持这样，理由有两条，都不是
		// 「今天调换就会算错」。
		//
		// 一是给人看的次序。先说「上次退出时发生了什么」（既成事实、更急），
		// 再说「下次 /ask 会发生什么」（预告）。倒过来读着别扭。
		//
		// 二是前向依赖，写下来是为了不被随手调换。AuditDrift 今天不读
		// baseline_suspect：它从 ProviderSession 上只取 ConvID、AgentID、
		// IdentityFP，而这三列 settle 整行回写时逐字节不动。今天把两段
		// 对调，漂移横幅的内容逐字节相同。哪天 AuditDrift 读了这一列
		// （比如漂移行想顺带标一句「这条的基线还可疑」），顺序就从「好看」
		// 变成「正确性」。依赖方向现在写明，那一天不必重新推一遍。
		//
		// 纯读失败只往 errOut 记一行，不阻止进 REPL，与下面漂移审计同体例：
		// 少一句提醒，不该把整个 REPL 拦在门外。
		lost, inflightErr := rn.AuditInflight(ctx)
		if inflightErr != nil {
			fmt.Fprintf(errOut, "启动在飞对账失败: %v\n", inflightErr)
		} else if len(lost) > 0 {
			// settle 失败同样不阻止进 REPL：拦住会让用户连 /log 都进不去，
			// 是更糟的处置。但失败必须响亮，因为 settle 失败意味着
			// baseline_suspect 留在 0，下一次 /ask 会自信地打印一个错数。
			marked, settleErrs := rn.SettleLostTurns(ctx, lost)
			for _, settleErr := range settleErrs {
				fmt.Fprintf(errOut, "启动在飞对账补账失败: %v\n", settleErr)
			}
			writeLostNotice(noticeOut, lost, marked)
		}

		// 启动漂移审计：把「静默作废重建」变响亮。这件事今天就在发生
		// （改 model / effort、或者规则文件被编辑之后没人告诉用户），
		// 只是除了这两种情况几乎没什么会触发；规则文件把触发条件变成了
		// 「编辑一个 md 文件」，所以现在必须在进 REPL 之前主动说一遍。
		//
		// -workspace 时这里不审：固定副本的 Runner 要等 convID 出来才构造，
		// 普通 Runner 算的是 agentWorkDir 的指纹。用它审会把 normal→workspace
		// 的静默重建漏掉，又会把 workspace→workspace 的相同指纹误报成漂移。
		// 这条审计改由 headless 用 fixed Runner 针对本次 convID 做。
		//
		// 审计本身读库出错，只往 errOut 记一行，不阻止进 REPL：这与上面
		// 「读不出规则就不进 REPL」不是同一类失败——那一条会让成员带着
		// 错误的指令环境去说话，这一条只是少一句提醒，不该因此把整个
		// REPL 拦在门外。
		if opts.Headless == nil || opts.Headless.Workspace == "" {
			if rows, auditErr := rn.AuditDrift(ctx); auditErr != nil {
				fmt.Fprintf(errOut, "启动漂移审计失败: %v\n", auditErr)
			} else if len(rows) > 0 {
				writeDriftNotice(noticeOut, rows)
			}
		}
	}
	rulesCtx := repl.RulesContext{ConfigPath: primaryPath, RulesFile: cfg.RulesFile, Rules: rules}
	// headless 分支必须在 TUI / 行式 REPL 之前：它复用上面已经打开的库与
	// 构造好的 Runner，只把控制权交给一次 ask，不再进任何读循环。
	// rn 在 svc 为 nil 时是 nil，headless.Run 会把它报成错误退 1。
	if opts.Headless != nil {
		return headless.Run(ctx, headless.Deps{
			Cfg:        cfg,
			Svc:        svc,
			Rn:         rn,
			DataDir:    dataDir,
			ProjectDir: projectDir,
			Out:        out,
			Err:        errOut,
			Verbose:    opts.Verbose,
		}, *opts.Headless)
	}
	// 复用本函数开头那个 err（config.ResolveConfigPath 那次声明的），不另开一个：
	// 到这里它已经被查过并且是 nil，而 var err error 会与它撞名编译不过。
	if opts.TUI {
		// 上面 TUI 分支已经把 out 换成 *tui.LineWriter。
		// 句柄用 InFile / OutFile，不用 In / Out：后者是接口，没有 Fd。
		lw := out.(*tui.LineWriter)
		r := repl.New(cfg, svc, rn, in, lw, rulesCtx)
		r.SetConcise(!opts.Verbose)
		r.SetAutoReply(!opts.NoAuto)
		r.SetCLIDetector(config.DetectProviders)
		r.SetLoginStatusExec(turn.RunLoginStatus)
		r.SetAddByName(true)
		r.SetProjectDir(projectDir)
		err = tui.Run(ctx, r, lw, opts.Version, opts.InFile, opts.OutFile)
	} else {
		// 行式与 TUI 都在 Run 之前设档位与对话模式。
		r := repl.New(cfg, svc, rn, in, out, rulesCtx)
		r.SetConcise(!opts.Verbose)
		r.SetAutoReply(!opts.NoAuto)
		r.SetCLIDetector(config.DetectProviders)
		r.SetLoginStatusExec(turn.RunLoginStatus)
		r.SetAddByName(true)
		r.SetProjectDir(projectDir)
		err = r.Run(ctx)
	}
	if err != nil {
		// 第一次 Ctrl+C 表现为 context 取消，那是规格 E 里的正常退出，不算失败。
		// tea.ErrProgramKilled 同样按正常退出：bubbletea 在外部 ctx 被取消
		// （SIGTERM、关窗）时把 ErrProgramKilled 和 ctx.Err() 包在一起返回。
		// 不认它的话，一次正常的关窗会被打成「REPL 异常结束」并以退出码 1 离开。
		if errors.Is(err, context.Canceled) || errors.Is(err, tea.ErrProgramKilled) || ctx.Err() != nil {
			return nil
		}
		fmt.Fprintf(errOut, "REPL 异常结束: %v\n", err)
		return fmt.Errorf("REPL 异常结束: %w", err)
	}
	return nil
}

// writeBanner 把启动信息写进 out：版本号、生效的配置文件路径、生效的数据目录、
// 库文件路径与已有会话数、项目目录与本项目会话数，以及配置里 agent 与 team 的数量。
// 写的是生效值而不是默认值，这样用户一眼能看出 --config 或环境变量到底有没有被吃掉；
// 库文件路径单独一行，是为了让人知道去哪里用 SQLite 查看器打开它。
// 项目目录行的数字用 repl.SameProjectDir，与 /resume 的过滤是同一个口径。
// 这里不打会话编号或列表，避免和 /resume 的排序对不上。
func writeBanner(out io.Writer, version, configPath, dataDir, dbPath string, convs []conversation.Conversation, projectDir string, cfg *config.Config) {
	if version != "" {
		fmt.Fprintf(out, "council %s\n", version)
	}
	fmt.Fprintf(out, "配置文件: %s\n", configPath)
	fmt.Fprintf(out, "数据目录: %s\n", dataDir)
	fmt.Fprintf(out, "库文件: %s (已有 %d 个会话)\n", dbPath, len(convs))
	if projectDir != "" {
		n, unbound := 0, 0
		for _, c := range convs {
			if repl.SameProjectDir(c.Cwd, projectDir) {
				n++
			}
			if c.Cwd == "" {
				unbound++
			}
		}
		fmt.Fprintf(out, "项目目录: %s（本项目 %d 个会话，/resume 列出并接上）\n", projectDir, n)
		if unbound > 0 {
			fmt.Fprintf(out, "另有 %d 个未绑定项目的旧会话，/resume --all 查看\n", unbound)
		}
	} else {
		fmt.Fprintf(out, "项目目录: 未知（/resume 将列出全部会话）\n")
	}
	fmt.Fprintf(out, "已加载 %d 个 agent，%d 个 team\n", len(cfg.Agents), len(cfg.Teams))
	fmt.Fprintf(out, "输入 /help 查看可用命令，/quit 退出。\n")
	fmt.Fprintln(out)
}

// writeDriftNotice 打启动漂移审计的提醒。头行按 AskRefused 拆成两句，各自冻结：
// n1（AskRefused 为假，身份已变、跨重启会重建付费）大于 0 时先印第一句；
// n2（AskRefused 为真，按当前配置必被拒、不花钱）大于 0 时再印第二句。
// 之后逐行列出全部 rows，顺序与 AuditDrift 返回的相同；明细格式不冻结。
func writeDriftNotice(out io.Writer, rows []turn.DriftRow) {
	n1, n2 := 0, 0
	for _, row := range rows {
		if row.AskRefused() {
			n2++
		} else {
			n1++
		}
	}
	if n1 > 0 {
		fmt.Fprintf(out, "注意: 有 %d 条 provider 会话的身份已变，下次 /ask 会各重建一次（每条一次入场费）\n", n1)
	}
	if n2 > 0 {
		fmt.Fprintf(out, "注意: 有 %d 条 provider 会话按当前配置 /ask 会被拒，不花钱；其中原因是「配置里已无此 agent」的，先 /use 那个会话再敲 /session rebuild <成员> 即可清掉\n", n2)
	}
	for _, row := range rows {
		fmt.Fprintf(out, "  会话 %s / 成员 %s: %s\n", row.ConvID, row.AgentID, row.Reason)
	}
	fmt.Fprintln(out)
}

// writeLostNotice 打启动在飞对账的提醒。第一行是冻结格式，测试会钉死。
// N 是清单长度，M 由调用方传入：不许在这一层用 phase 反推标了几条，
// bootstrap 轮的 phase 是 spawned，库里却没有那一行可标。
//
// M == 0 时照常打印「其中 0 轮」，不做特判。特判会多出一条分支，
// 而那条分支恰好只在最容易被忽略的情形下生效。
//
// N == 0 时一个字都不打印。库是干净的时候整个输出里不该出现
// 「在飞」「没记上账」任何字样。
//
// 逐行那部分按「随便改，测试不许钉」处理，只有第一行冻结。
// 超过 LostNoticeCap 条时其余折叠成一个计数，不让一次积压刷满一屏。
func writeLostNotice(out io.Writer, lost []turn.LostTurn, marked int) {
	n := len(lost)
	if n == 0 {
		return
	}
	fmt.Fprintf(out, "注意: 有 %d 轮在上次退出时没记上账，其中 %d 轮已把相关成员的成本基线标为可疑\n", n, marked)
	limit := n
	if limit > turn.LostNoticeCap {
		limit = turn.LostNoticeCap
	}
	for _, row := range lost[:limit] {
		started := row.Turn.StartedAt.UTC().Format(time.RFC3339Nano)
		fmt.Fprintf(out, "  会话 %s / 成员 %s / turn %s / 起于 %s / phase %s: %s\n",
			row.Turn.ConvID, row.Turn.AgentID, row.Turn.TurnID, started, row.Turn.Phase, row.Reason)
	}
	if n > limit {
		fmt.Fprintf(out, "  其余 %d 条已折叠\n", n-limit)
	}
	fmt.Fprintln(out)
}

// onlyDefaultLayer 判断 layers 是否只有「默认全局位置」这一条。
// 它不判断文件是否存在：全局配置真的存在时也是这一条，后面的 os.Stat
// 才是「要不要进首次运行」的那道门；这里只负责排除「有项目层」的情况——
// 项目里已经有 .council/config.toml 时，不该再往家里写一份全局配置。
func onlyDefaultLayer(layers []config.Layer, defaultPath string) bool {
	return len(layers) == 1 && !layers[0].Project && layers[0].Path == defaultPath
}

// reportConfigError 把配置加载或校验的错误逐行写进 w。
// ValidationErrors 的 Error() 本身就是多行的（规格 D-10 要求一次报全部错误），
// 这里原样写出去，不再逐条重排，免得两套格式打架。
// explicit 为真表示用户自己指定过配置位置（--config 或环境变量），这时点明
// 路径是他指定的。默认位置找不到文件的情况，Run 通常已经先走过首次运行（#48）；
// 还能走到这里的只有两种：竞态（首次运行建好或改用的文件，在 LoadAndValidate
// 之前又没了），与默认路径不是绝对路径（USERPROFILE 被设成了相对路径，Run 不进
// 首次运行，免得在启动目录下建文件）。两种都只打两行说明，不再打印模板。
func reportConfigError(w io.Writer, path string, err error, explicit bool) {
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(w, "找不到配置文件: %s\n", path)
		if explicit {
			fmt.Fprintf(w, "这个路径是你用 --config 或环境变量 %s 指定的，请确认它没写错。\n", config.EnvConfigPath)
			return
		}
		fmt.Fprintf(w, "可以用 --config <path> 指定别处的配置文件，或设置环境变量 %s 指向它。\n", config.EnvConfigPath)
		return
	}
	fmt.Fprintf(w, "配置文件有问题: %s\n", path)
	fmt.Fprintln(w, strings.TrimRight(err.Error(), "\n"))
}
