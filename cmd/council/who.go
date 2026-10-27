// who.go 是 `council who` 的进程边界与输出层。
//
// 这个子命令存在的理由：它是**唯一一个可以在别的 council 正在跑的时候
// 安全执行**的本机视图 —— 不拿 council.lock，只以只读模式打开库
// （internal/store.OpenReadOnly），拿不到库也照样把本机进程那一段打出来。
//
// 开启（observe）时输出始终是两段：
//  1. Council 管理的成员：从库里读在飞的 turn（只读打开；失败只影响这一段）；
//  2. 本机其他 agent 进程：来自 internal/localagent（local_agent = observe
//     才会枚举；off 时一个字都不扫，只打印一句怎么开启）。
//
// 与 TUI 的 /who 不是一回事：/who 列的是当前会话的成员，本命令看的是
// 本机进程。帮助文案与输出末尾都点明这一点，避免同名误导。
//
// L1 **不做** managed ↔ observed 的合并。`cmd.Start()` 起 .cmd 垫片时
// 返回的是 cmd.exe 的 PID，不是真正干活的进程；靠 PID 直接对会把观察到的
// 进程误标成 Council 管理的成员。两段是并列事实：managed 行来自库里的
// 在飞记录（没有 PID），observed 行的 certainty 只回答「这个进程有多确定
// 是那家 CLI」。合并规则等 L2 有了进程树 / 启动时间窗口再做。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/i18n"
	"council/internal/localagent"
	"council/internal/store"
)

// whoScanTimeout 是两段读取共用的上限。
//
// 有上限的理由与看板的 busy_timeout 一致：council who 可能在 TUI 正提交
// 一轮时运行，SQLite 会等锁；等到一个明确的错误再打印「库不可用」，
// 比让终端看起来卡死好。进程枚举本身通常在几十毫秒内结束。
const whoScanTimeout = 10 * time.Second

// whoDeps 收拢 who 的两条外部依赖，测试可以注入假枚举器与假库打开函数：
//   - Enumerate：localagent.Enumerate（Windows 上走 Toolhelp32）；
//   - OpenReadOnly：store.OpenReadOnly（不拿锁的只读句柄）。
//
// 为什么不在 runWho 里直接调那两个函数：off 路径要能被测试证明「一次枚举、
// 一次开库都没有发生」。注入一个会 panic 的实现比字符串断言更硬。
type whoDeps struct {
	Enumerate    localagent.Enumerator
	OpenReadOnly func(context.Context, string) (*store.Store, error)
}

// whoManagedRow 是 JSON 里「Council 管理的成员」的一行。
//
// Phase 用 conversation.InflightPhase 的字符串（begun / spawned / returned），
// 与库里的值一字不改：这是排查在飞对账时最需要的一个字段。
type whoManagedRow struct {
	ConversationID string `json:"conversation_id"`
	AgentID        string `json:"agent_id"`
	TurnID         string `json:"turn_id"`
	Provider       string `json:"provider"`
	Phase          string `json:"phase"`
	StartedAt      string `json:"started_at"`
	WorkDir        string `json:"work_dir"`
	Source         string `json:"source"`
}

// whoObservedRow 是 JSON 里「本机其他 agent 进程」的一行。
//
// StartedAt 拿不到时是空串：JSON 里不编一个 1601 年或 1970 年的时间。
// ExePath 是 exact/probable 判定的依据；拿不到时为空串。
// Certainty 只描述「这个进程有多确定是那家 CLI」，不表示它归 Council 管。
//
// MergeEvidence / ConversationID / AgentID / TurnID 只在 `-server` 时出现：
// PID 合并发生在持锁那个进程里（⑨ 的钩子表在内存），跨进程读库拿不到。
// 直接扫描本机的那条路不写这四个键 —— 它不知道谁是谁。
type whoObservedRow struct {
	Kind           string `json:"kind"`
	PID            int    `json:"pid"`
	PPID           int    `json:"ppid"`
	ExeName        string `json:"exe_name"`
	ExePath        string `json:"exe_path"`
	StartedAt      string `json:"started_at"`
	Source         string `json:"source"`
	Certainty      string `json:"certainty"`
	MergeEvidence  string `json:"merge_evidence,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	AgentID        string `json:"agent_id,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
}

// whoLeaseRow 是 JSON 里「本机租约」的一行。
//
// 只有仍然有效的租约出现在这里（store.GrantedLeases 已经滤掉过期的行）。
// 租约是自愿遵守的提醒，不是文件锁：本行存在不代表任何进程被挡住。
type whoLeaseRow struct {
	LeaseID     string `json:"lease_id"`
	ResourceKey string `json:"resource_key"`
	OwnerKind   string `json:"owner_kind"`
	OwnerID     string `json:"owner_id"`
	Mode        string `json:"mode"`
	Generation  int64  `json:"generation"`
	ExpiresAt   string `json:"expires_at"`
}

// whoReport 是 -json 的完整形状。
//
// 三段数组永远存在（off 时是空数组而不是 null）：调用方不必为「没有这一段」
// 写第二条判空路径。错误与提示是可选字段，不出现时就是没有。
type whoReport struct {
	LocalAgent    string           `json:"local_agent"`
	Managed       []whoManagedRow  `json:"managed"`
	Observed      []whoObservedRow `json:"observed"`
	Leases        []whoLeaseRow    `json:"leases"`
	ManagedError  string           `json:"managed_error,omitempty"`
	ObservedError string           `json:"observed_error,omitempty"`
	LeasesError   string           `json:"leases_error,omitempty"`
	Notice        string           `json:"notice,omitempty"`
	// Via 说明这份事实是直接读本机还是来自看板快照（-server）。
	Via string `json:"via,omitempty"`
}

// runWho 是 `council who` 的进程边界：生产依赖写死在这里，输出走标准流。
func runWho(args []string) int {
	return runWhoWith(args, whoDeps{
		Enumerate:    localagent.Enumerate,
		OpenReadOnly: store.OpenReadOnly,
	}, os.Stdout, os.Stderr)
}

// runWhoWith 是可测试的那一半：deps 与两个输出流都由调用方注入。
// 退出码约定与其它子命令一致：用法错误 2，配置/写输出失败 1，其余 0。
func runWhoWith(args []string, deps whoDeps, out, errOut io.Writer) int {
	// 语言要在建 FlagSet 之前定：-h 在 Parse 里就返回 ErrHelp，那时再按
	// 解析结果选说明文字已经晚了（与 ask / serve / mcp 同一套写法）。
	lang := applyLanguageFlag(prescanFlag(args, "lang"), prescanFlag(args, "config"))
	text := whoTextFunc(lang)

	fs := flag.NewFlagSet("council who", flag.ContinueOnError)
	// flag 包自己的输出丢掉：帮助与解析错误都由下面那一份 usage 统一打印。
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", text("cli.who.flag.config",
		"配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml"))
	langFlag := fs.String("lang", "auto", text("cli.who.flag.lang",
		"输出语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）"))
	asJSON := fs.Bool("json", false, text("cli.who.flag.json",
		"按 JSON 输出（local_agent / managed / observed / leases / notice）"))
	server := fs.String("server", "", text("cli.who.flag.server",
		"改问正在跑的看板要一份快照（例如 http://127.0.0.1:8765）：只有它拿得到 PID 与合并结果"))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(errOut, whoUsageLang(lang))
			return exitOK
		}
		fmt.Fprintf(errOut, "council who: %v\n", err)
		fmt.Fprint(errOut, whoUsageLang(lang))
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprint(errOut, text("cli.who.error.positional",
			"council who 不接受位置参数，多出来的是: %v\n", fs.Args()))
		fmt.Fprint(errOut, whoUsageLang(lang))
		return exitUsage
	}

	// -lang 的正式结果要在读配置之前落地，输出语言与配置里的 language
	// 走同一次解析（与 serve / mcp 一致）。
	lang = applyLanguageFlag(*langFlag, *configPath)
	text = whoTextFunc(lang)

	// -server：问那个握着 council.lock 的看板。它手里的快照比跨进程读库多
	// 两样东西 —— 子进程 PID 与合并结果（⑨ 的钩子表只在内存），这两样正是
	// 「两段并列」那个老问题的解药。只读 GET，不过写闸，也不需要层级。
	if serverArg := strings.TrimSpace(*server); serverArg != "" {
		base, err := normalizeLeaseServer(serverArg)
		if err != nil {
			fmt.Fprintf(errOut, "council who: %v\n", err)
			return exitUsage
		}
		return runWhoOverServer(out, errOut, text, base, *asJSON)
	}

	level, dataDir, err := loadWhoConfig(*configPath)
	if err != nil {
		// 配置坏掉时不再猜开关状态：宁可退 1，也不把「配置写坏了」显示成
		// 「本机没有 agent」。缺配置文件是另一条路（出厂状态 = off），
		// 见 loadWhoConfig。
		fmt.Fprint(errOut, text("cli.who.configBroken", "council who: 配置文件有问题: %v\n", err))
		return exitConfigFailed
	}

	// off：不枚举进程、不开库、不读任何东西，只说明怎么开启，退出 0。
	// 这是 S-01 在 CLI 上的落点；测试用一个会 panic 的枚举器证明它没被调用。
	// observe 与 coordinate 都扫（协调需要同一份进程事实），两档的区别在
	// council claim 的申请权上，不在本命令里。
	if !level.Scans() {
		notice := text("cli.who.off", `本机 agent 未开启（config 里 local_agent = "observe" 开启观察，"coordinate" 同时开启资源租约）`)
		if *asJSON {
			return writeWhoJSON(out, errOut, whoReport{
				LocalAgent: string(localagent.LevelOff),
				Managed:    []whoManagedRow{},
				Observed:   []whoObservedRow{},
				Leases:     []whoLeaseRow{},
				Notice:     notice,
			})
		}
		fmt.Fprintln(out, notice)
		return exitOK
	}

	ctx, cancel := context.WithTimeout(context.Background(), whoScanTimeout)
	defer cancel()

	managed, managedErr := readManagedTurns(ctx, deps.OpenReadOnly, dataDir)
	agents, observedErr := localagent.Scan(level, deps.Enumerate)
	leases, leasesErr := readGrantedLeases(ctx, deps.OpenReadOnly, dataDir)

	report := whoReport{
		LocalAgent: string(level),
		Managed:    managed,
		Observed:   observedRows(agents),
		Leases:     leaseRows(leases),
	}
	if managedErr != nil {
		report.ManagedError = managedErr.Error()
	}
	if observedErr != nil {
		report.ObservedError = observedErr.Error()
	}
	if leasesErr != nil {
		report.LeasesError = leasesErr.Error()
	}

	if *asJSON {
		return writeWhoJSON(out, errOut, report)
	}
	writeWhoText(out, whoView{
		text:        text,
		managed:     managed,
		managedErr:  managedErr,
		agents:      report.Observed,
		observedErr: observedErr,
		leases:      report.Leases,
		leasesErr:   leasesErr,
	})
	return exitOK
}

// whoView 是纯文本输出需要的一组事实。用结构体而不是九个位置参数：
// 三段事实各自带一个错误，位置参数一多就没人记得住顺序。
//
// agents 用的是 whoObservedRow（与 -json 同一份），不是 localagent.Agent：
// 两条数据源（本机扫描 / 看板快照）里后者多带 PID 合并结果，统一成同一种行
// 才能让「打印什么」与「JSON 里是什么」永远一致。
type whoView struct {
	text        func(key, fallback string, args ...any) string
	managed     []whoManagedRow
	managedErr  error
	agents      []whoObservedRow
	observedErr error
	leases      []whoLeaseRow
	leasesErr   error
	// servedFrom 非空表示这一份事实来自看板快照（-server），文本里要标出来：
	// 两份事实的来源不同，读的人必须知道自己在看哪一份。
	servedFrom string
}

// whoTextFunc 绑定一种语言的 TF，只在 runWhoWith 里换两次（解析前后）。
func whoTextFunc(lang i18n.Lang) func(key, fallback string, args ...any) string {
	return func(key, fallback string, args ...any) string {
		return i18n.TF(lang, key, fallback, args...)
	}
}

// loadWhoConfig 只读地求两样东西：local_agent 的层级与 data_dir。
//
// 配置文件一份都不存在时返回 off：那是出厂状态，与显式写 off 等价，
// 不该报错（`council who` 在没跑过 council 的机器上也应当能用）。
// 配置存在但读不出来（语法错误、未知键、非法 local_agent）时返回错误，
// 由调用方退 1 —— 猜一个层级会让关闭状态变得不可验证。
func loadWhoConfig(configPath string) (localagent.Level, string, error) {
	layers, err := config.ResolveConfigLayers(configPath)
	if err != nil {
		return localagent.LevelOff, "", err
	}
	if len(layers) == 0 {
		return localagent.LevelOff, "", nil
	}
	if _, statErr := os.Stat(layers[0].Path); statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return localagent.LevelOff, "", nil
		}
		return localagent.LevelOff, "", statErr
	}

	merged, err := config.LoadMerged(layers)
	if err != nil {
		return localagent.LevelOff, "", err
	}
	dataDir, err := config.ResolveDataDir(merged.Config.DataDir)
	if err != nil {
		return localagent.LevelOff, "", err
	}
	return localagent.LevelOf(merged.Config.LocalAgent), dataDir, nil
}

// readManagedTurns 以只读方式打开库并列出在飞的 turn。
//
// 失败不是致命的：返回空表与错误，调用方在「Council 管理的成员」那一段
// 打印「库不可用」，本机进程那一段照常显示（这正是本命令存在的意义：
// serve -rw 拿着写锁时它也能用）。
func readManagedTurns(ctx context.Context, open func(context.Context, string) (*store.Store, error), dataDir string) ([]whoManagedRow, error) {
	if open == nil {
		return []whoManagedRow{}, errors.New("只读打开库的入口未注入")
	}
	if dataDir == "" {
		return []whoManagedRow{}, errors.New("数据目录未知")
	}
	st, err := open(ctx, dataDir)
	if err != nil {
		return []whoManagedRow{}, err
	}
	defer func() {
		// 只读句柄关不上不影响已经读到的结果，也不改退出码。
		_ = st.Close()
	}()

	turns, err := st.AllInflight(ctx)
	if err != nil {
		return []whoManagedRow{}, err
	}
	rows := make([]whoManagedRow, 0, len(turns))
	for _, turn := range turns {
		rows = append(rows, whoManagedRow{
			ConversationID: turn.ConvID,
			AgentID:        turn.AgentID,
			TurnID:         turn.TurnID,
			Provider:       turn.Provider,
			Phase:          string(turn.Phase),
			StartedAt:      formatWhoTime(turn.StartedAt),
			WorkDir:        turn.WorkDir,
			Source:         string(localagent.SourceManaged),
		})
	}
	return rows, nil
}

// readGrantedLeases 以只读方式打开库并列出仍然有效的租约。
//
// 与 readManagedTurns 分开打开一次，换来两件事：一个段落失败不影响另一个
// （旧库没有 local_leases 表时只有租约段报错），以及两个函数各自可以单独
// 注入。只读句柄不拿 council.lock，多开一次的代价是几毫秒。
func readGrantedLeases(ctx context.Context, open func(context.Context, string) (*store.Store, error), dataDir string) ([]store.Lease, error) {
	if open == nil {
		return []store.Lease{}, errors.New("只读打开库的入口未注入")
	}
	if dataDir == "" {
		return []store.Lease{}, errors.New("数据目录未知")
	}
	st, err := open(ctx, dataDir)
	if err != nil {
		return []store.Lease{}, err
	}
	defer func() {
		// 只读句柄关不上不影响已经读到的结果，也不改退出码。
		_ = st.Close()
	}()
	leases, err := st.GrantedLeases(ctx)
	if err != nil {
		// 只读模式不迁移库：v9 及更早的库根本没有这两张表。把版本号写进
		// 错误里，用户才知道下一步是「跑一次写模式的 council」而不是去查
		// 这条 SQL。
		if version := st.SchemaVersion(); version < 10 {
			return []store.Lease{}, fmt.Errorf(
				"库是 v%d，没有租约表（租约要 schema v10），先跑一次写模式的 council 升级：%w", version, err)
		}
		return []store.Lease{}, err
	}
	return leases, nil
}

// leaseRows 把租约映射成 JSON 行。
func leaseRows(leases []store.Lease) []whoLeaseRow {
	rows := make([]whoLeaseRow, 0, len(leases))
	for _, lease := range leases {
		rows = append(rows, whoLeaseRow{
			LeaseID:     lease.ID,
			ResourceKey: lease.ResourceKey,
			OwnerKind:   lease.OwnerKind,
			OwnerID:     lease.OwnerID,
			Mode:        string(lease.Access),
			Generation:  lease.Generation,
			ExpiresAt:   formatWhoTime(lease.ExpiresAt),
		})
	}
	return rows
}

// observedRows 把 localagent 的结果映射成 JSON 行。StartAt 为空的字符串
// 表示系统没给启动时间，与 localagent.Agent 的零值一一对应。
func observedRows(agents []localagent.Agent) []whoObservedRow {
	rows := make([]whoObservedRow, 0, len(agents))
	for _, agent := range agents {
		rows = append(rows, whoObservedRow{
			Kind:      string(agent.Kind),
			PID:       agent.PID,
			PPID:      agent.PPID,
			ExeName:   agent.ExeName,
			ExePath:   agent.ExePath,
			StartedAt: formatWhoTime(agent.StartedAt),
			Source:    string(agent.Source),
			Certainty: string(agent.Certainty),
		})
	}
	return rows
}

// formatWhoTime 统一输出 UTC 的 RFC3339；零值给空串（「拿不到」），
// 不给一个会被误读成事实的时间。
func formatWhoTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// whoTimeText 给纯文本用：拿不到时显示「未知」而不是空字段。
func whoTimeText(t time.Time, unknown string) string {
	if t.IsZero() {
		return unknown
	}
	return t.UTC().Format(time.RFC3339)
}

// writeWhoText 按三段打印。三段的标题永远出现（observe 模式下），
// 这样用户一眼能区分「这一段没有内容」与「这一段没跑」。
func writeWhoText(out io.Writer, view whoView) {
	text := view.text
	unknown := text("cli.who.unknown", "未知")

	if view.servedFrom != "" {
		fmt.Fprintln(out, text("cli.who.servedFrom",
			"（以下事实来自看板 %s 的快照：PID 与合并结果只有它拿得到）", view.servedFrom))
	}

	fmt.Fprintln(out, text("cli.who.headManaged", "Council 管理的成员（%d）", len(view.managed)))
	switch {
	case view.managedErr != nil:
		fmt.Fprintln(out, text("cli.who.managedUnavailable", "  （库不可用：%v；只显示本机进程部分）", view.managedErr))
	case len(view.managed) == 0:
		fmt.Fprintln(out, text("cli.who.managedEmpty", "  （没有在飞的成员）"))
	default:
		for _, row := range view.managed {
			started := row.StartedAt
			if started == "" {
				started = unknown
			}
			fmt.Fprintln(out, text("cli.who.managedRow",
				"  会话 %s / 成员 %s / turn %s / phase %s / 起于 %s",
				row.ConversationID, row.AgentID, row.TurnID, row.Phase, started))
		}
	}

	fmt.Fprintln(out, text("cli.who.headObserved", "本机其他 agent 进程（%d）", len(view.agents)))
	switch {
	case view.observedErr != nil:
		fmt.Fprintln(out, text("cli.who.observedUnavailable", "  （进程枚举不可用：%v）", view.observedErr))
	case len(view.agents) == 0:
		fmt.Fprintln(out, text("cli.who.observedEmpty", "  （没有发现本机 agent 进程）"))
	default:
		for _, row := range view.agents {
			started := row.StartedAt
			if started == "" {
				started = unknown
			}
			extra := ""
			if row.MergeEvidence != "" {
				extra = text("cli.who.observedMerged",
					" merged=%s（turn %s）", row.MergeEvidence, row.TurnID)
			}
			fmt.Fprintln(out, text("cli.who.observedRow",
				"  %s  pid=%d ppid=%d exe=%s source=%s certainty=%s 起于 %s%s",
				row.Kind, row.PID, row.PPID, row.ExeName, row.Source,
				row.Certainty, started, extra))
		}
	}

	fmt.Fprintln(out, text("cli.who.headLeases", "本机租约（%d）", len(view.leases)))
	switch {
	case view.leasesErr != nil:
		fmt.Fprintln(out, text("cli.who.leasesUnavailable",
			"  （租约读不出来：%v）", view.leasesErr))
	case len(view.leases) == 0:
		fmt.Fprintln(out, text("cli.who.leasesEmpty", "  （没有有效租约）"))
	default:
		for _, lease := range view.leases {
			fmt.Fprintln(out, text("cli.who.leaseRow",
				"  资源 %s / 持有者 %s/%s / 模式 %s / 代数 %d / 到期 %s / lease %s",
				lease.ResourceKey, lease.OwnerKind, lease.OwnerID, lease.Mode,
				lease.Generation, lease.ExpiresAt, lease.LeaseID))
		}
	}

	// 尾注随数据源换：直读本机时确实不做合并，看板快照里已经合过了 ——
	// 同一句尾注在两种情况下不可能都对。
	if view.servedFrom != "" {
		fmt.Fprintln(out, text("cli.who.noteServer",
			"这份快照由看板进程给出：它把「这一轮起的子进程」与进程表里的那一行按 PID（含 .cmd 垫片）对过；不读取命令行、环境变量或其它 CLI 的配置；租约是提醒，不是文件锁。"))
		return
	}
	fmt.Fprintln(out, text("cli.who.note",
		"本命令看的是本机进程，不是会话成员；不读取命令行、环境变量或其它 CLI 的配置；三段不做 PID 合并（.cmd 垫片拿到的是 cmd.exe 的 PID）；租约是提醒，不是文件锁。"))
}

// writeWhoJSON 输出一行 JSON（与 council commands -json 同体例：不缩进，
// 不转义 HTML 尖括号，便于人核对字段）。
func writeWhoJSON(out, errOut io.Writer, report whoReport) int {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(errOut, "council who: %v\n", err)
		return exitConfigFailed
	}
	return exitOK
}

// whoUsageLang 是 -h 与用法错误共用的那一段。
func whoUsageLang(lang i18n.Lang) string {
	return i18n.TF(lang, "cli.who.usage", `用法: council who [flags]

只读列出三段本机事实，不拿 council.lock，可以在 TUI 或 serve -rw 开着的时候跑：
  1. Council 管理的成员：库里正在飞的 turn
  2. 本机其他 agent 进程：claude / codex / grok / agy（需要 config 里 local_agent = "observe" 或 "coordinate"）
  3. 本机租约：仍然有效的 council claim（租约要 schema v10；旧库这一段只说读不出来）
本命令看的是本机进程，不是会话成员（那是在 council 里敲 /who）。
certainty=exact 只给落在已知 CLI 安装目录里的进程；路径拿不到或不在安装目录里的标 probable，
桌面客户端（Claude Desktop 等）的同名 claude.exe 不报。
不读取命令行、环境变量或其它 CLI 的配置；local_agent = "off"（默认）时一次进程都不枚举。

flags:
  -json                按 JSON 输出（local_agent / managed / observed / leases / notice）
  -config <path>       配置文件路径，与全局 -config 含义相同
  -lang zh|en|auto     输出语言，默认 auto
`)
}
