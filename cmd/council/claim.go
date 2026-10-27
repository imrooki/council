// claim.go 是 `council claim` / `council release` 的进程边界与输出层。
//
// 这两个子命令实现 Plan §8 的资源租约：一条**自愿遵守的记录**，不是文件锁。
// 三件事决定了它们能解决什么问题，必须写在最前面：
//
//  1. 租约存在 council.db 的 local_leases 表（schema v10），所以要拿
//     council.lock。桌面壳的 serve -rw、TUI 或 council mcp 正在跑时，本命令
//     会明确拒绝，而不是另开一个库或绕过锁（C-1 单写者）。
//  2. **申请**需要 local_agent = coordinate；**交还**不需要 —— 把开关调小
//     不该让已经发出去的授权无法收回。
//  3. 申请与交还零模型调用、零进程扫描：它们只读写两张表。这也是为什么
//     `council who`（只读、不拿锁）与本命令是两个子命令：一个看，一个写。
//
// 租约挡不住任何进程写文件（Plan §8.2：子进程按自己的 cmd.Dir 跑，权限只
// 翻译成 CLI flag），所以本命令的文案只说事实，不承诺「已锁住」。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"council/internal/i18n"
	"council/internal/localagent"
	"council/internal/store"
)

// exitLeaseDenied 是「本命令没有做成任何事，但环境是好的」的退出码：
// 资源被占用、层级未开启、没有匹配的租约。
//
// 为什么单开一个码而不是混进 1：`council claim . && 干活` 这种写法必须能区分
// 「没拿到」与「命令坏了」。两者都是非零，所以脚本不会误干活；分开是为了
// 日志与重试逻辑能看懂原因。0 / 1 / 2 与其它子命令一致。
const exitLeaseDenied = 3

// leaseTimeout 是一次 claim / release 的上限。SQLite 自己的 busy_timeout 是
// 5 秒（open.go 的 DSN），这里留出余量；超时说明数据目录被别的进程掐住了，
// 而不是本命令慢。
const leaseTimeout = 15 * time.Second

// leaseDeps 收拢唯一的边界，测试可以注入「一调用就失败」的实现来证明
// 「层级未开启时连库都不开」。
type leaseDeps struct {
	Open func(context.Context, string) (*store.Store, error)
}

// leaseRow 是 JSON 里的一行租约。时间用 UTC 的 RFC3339，与 council who 一致。
type leaseRow struct {
	LeaseID     string `json:"lease_id"`
	ResourceKey string `json:"resource_key"`
	OwnerKind   string `json:"owner_kind"`
	OwnerID     string `json:"owner_id"`
	Mode        string `json:"mode"`
	Generation  int64  `json:"generation"`
	State       string `json:"state"`
	GrantedAt   string `json:"granted_at"`
	ExpiresAt   string `json:"expires_at"`
	HeartbeatAt string `json:"heartbeat_at"`
}

// claimReport 是 `council claim -json` 的形状。
//
// granted=false 时 lease 缺席、holder 或 notice 说明原因；调用方只判 granted，
// 不必解析 denied 的枚举（它给日志看）。
type claimReport struct {
	Granted    bool      `json:"granted"`
	LocalAgent string    `json:"local_agent"`
	Resource   string    `json:"resource"`
	Mode       string    `json:"mode"`
	Owner      string    `json:"owner"`
	Lease      *leaseRow `json:"lease,omitempty"`
	Holder     *leaseRow `json:"holder,omitempty"`
	Denied     string    `json:"denied,omitempty"`
	Notice     string    `json:"notice,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// releaseReport 是 `council release -json` 的形状。
type releaseReport struct {
	LocalAgent string     `json:"local_agent"`
	Resource   string     `json:"resource,omitempty"`
	Released   []leaseRow `json:"released"`
	Notice     string     `json:"notice,omitempty"`
	Error      string     `json:"error,omitempty"`
}

func toLeaseRow(lease store.Lease) leaseRow {
	return leaseRow{
		LeaseID:     lease.ID,
		ResourceKey: lease.ResourceKey,
		OwnerKind:   lease.OwnerKind,
		OwnerID:     lease.OwnerID,
		Mode:        string(lease.Access),
		Generation:  lease.Generation,
		State:       string(lease.State),
		GrantedAt:   formatWhoTime(lease.GrantedAt),
		ExpiresAt:   formatWhoTime(lease.ExpiresAt),
		HeartbeatAt: formatWhoTime(lease.HeartbeatAt),
	}
}

// defaultLeaseOwner 是没给 -owner 时的持有者名字。
//
// 用「cli@主机名」而不是进程号：release -resource 要能找回同一个人上一次
// 申请的那条租约，而进程号每次都不一样。身份是自述的，不参与鉴权
// （Plan §9.2），它只回答「这条租约是谁要的」。
func defaultLeaseOwner() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "cli"
	}
	return "cli@" + strings.ToLower(strings.TrimSpace(host))
}

// runClaim 是 `council claim` 的进程边界。
func runClaim(args []string) int {
	return runClaimWith(args, leaseDeps{Open: store.Open}, os.Stdout, os.Stderr)
}

// runClaimWith 是可测试的那一半。
func runClaimWith(args []string, deps leaseDeps, out, errOut io.Writer) int {
	lang := applyLanguageFlag(prescanFlag(args, "lang"), prescanFlag(args, "config"))
	text := whoTextFunc(lang)

	fs := flag.NewFlagSet("council claim", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", text("cli.claim.flag.config",
		"配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml"))
	langFlag := fs.String("lang", "auto", text("cli.claim.flag.lang",
		"输出语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）"))
	asJSON := fs.Bool("json", false, text("cli.claim.flag.json",
		"按 JSON 输出（granted / resource / mode / lease / holder / notice）"))
	mode := fs.String("mode", string(localagent.AccessWrite), text("cli.claim.flag.mode",
		"租约模式：write（独占，默认）或 read（共享）"))
	ttl := fs.Duration("ttl", 0, text("cli.claim.flag.ttl",
		"租约有效期，例如 5m / 1h（默认 2m，最短 5s，最长 24h）"))
	owner := fs.String("owner", "", text("cli.claim.flag.owner",
		"持有者名字（默认 cli@主机名）；它只是展示与交还时的匹配条件，不参与鉴权"))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(errOut, claimUsageLang(lang))
			return exitOK
		}
		fmt.Fprintf(errOut, "council claim: %v\n", err)
		fmt.Fprint(errOut, claimUsageLang(lang))
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprint(errOut, text("cli.claim.error.positional",
			"council claim 需要恰好一个资源参数（目录路径，或 tcp:<host>:<port>），多出来的是: %v\n", fs.Args()))
		fmt.Fprint(errOut, claimUsageLang(lang))
		return exitUsage
	}

	// 参数在开库之前全部折好：资源键不合法、mode 打错、ttl 越界都是用法错误，
	// 不该先写库再报错。
	resource, err := localagent.NormalizeResource(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(errOut, "council claim: %v\n", err)
		return exitUsage
	}
	access, err := localagent.ParseAccessMode(*mode)
	if err != nil {
		fmt.Fprintf(errOut, "council claim: %v\n", err)
		return exitUsage
	}
	ttlValue, err := localagent.NormalizeTTL(*ttl)
	if err != nil {
		fmt.Fprintf(errOut, "council claim: %v\n", err)
		return exitUsage
	}
	who := strings.TrimSpace(*owner)
	if who == "" {
		who = defaultLeaseOwner()
	}

	lang = applyLanguageFlag(*langFlag, *configPath)
	text = whoTextFunc(lang)

	level, dataDir, err := loadWhoConfig(*configPath)
	if err != nil {
		fmt.Fprint(errOut, text("cli.lease.configBroken", "%s: 配置文件有问题: %v\n", "council claim", err))
		return exitConfigFailed
	}
	// 层级不够时连库都不开：coordinate 之外的配置下，本命令一个字节都不写。
	// 这是 Plan §3.2「逐层授权」在 CLI 上的落点；测试用一个会 fatal 的开库
	// 函数证明它没被调用。
	if !level.Claims() {
		notice := text("cli.claim.notEnabled",
			`本机 agent 协调未开启（config 里 local_agent = "coordinate" 可开启）：本命令不写任何租约。`)
		if *asJSON {
			return writeJSONReport(out, errOut, "council claim", claimReport{
				Granted:    false,
				LocalAgent: string(level),
				Resource:   resource,
				Mode:       string(access),
				Owner:      who,
				Denied:     "not_enabled",
				Notice:     notice,
			}, exitLeaseDenied)
		}
		fmt.Fprintln(errOut, notice)
		return exitLeaseDenied
	}

	ctx, cancel := context.WithTimeout(context.Background(), leaseTimeout)
	defer cancel()

	st, openErr := deps.Open(ctx, dataDir)
	if openErr != nil {
		return reportLeaseOpenFailure(errOut, text, "council claim", dataDir, openErr)
	}
	defer func() {
		// 关库失败不影响「租约已经写进去了」这个事实，也不改退出码。
		_ = st.Close()
	}()

	lease, err := st.ClaimLease(ctx, store.LeaseRequest{
		ResourceKey: resource,
		OwnerKind:   store.LeaseOwnerCLI,
		OwnerID:     who,
		Access:      store.LeaseAccess(access),
		TTL:         ttlValue,
	})
	if err != nil {
		var conflict *store.LeaseConflictError
		if errors.As(err, &conflict) {
			report := claimReport{
				Granted:    false,
				LocalAgent: string(level),
				Resource:   resource,
				Mode:       string(access),
				Owner:      who,
				Denied:     "busy",
			}
			// Holder 为空只出现在「插入撞上唯一约束、回读又扑空」那条路径上
			// （单写者下正常到不了）。那时不编一个持有者出来，只报结论。
			if conflict.Holder.ID != "" {
				row := toLeaseRow(conflict.Holder)
				report.Holder = &row
			}
			if *asJSON {
				return writeJSONReport(out, errOut, "council claim", report, exitLeaseDenied)
			}
			if report.Holder == nil {
				fmt.Fprintln(errOut, err)
				return exitLeaseDenied
			}
			// 拒绝的说明走 stderr：它解释「为什么什么都没发生」，与成功结果
			// （stdout）分开，管道里 `council claim . > 结果` 才不会把提示吞掉。
			fmt.Fprint(errOut, text("cli.claim.busy", `资源被占用，没有拿到租约
  资源    %s
  持有者  %s/%s（%s 租约）
  lease   %s
  到期    %s
先等它到期，或请持有者 council release %s 之后再申请。租约是自愿遵守的提醒，不会阻止任何进程写文件。
`, resource, conflict.Holder.OwnerKind, conflict.Holder.OwnerID,
				conflict.Holder.Access, conflict.Holder.ID,
				formatWhoTime(conflict.Holder.ExpiresAt), conflict.Holder.ID))
			return exitLeaseDenied
		}
		fmt.Fprint(errOut, text("cli.claim.failed", "council claim: %v\n", err))
		return exitConfigFailed
	}

	if *asJSON {
		row := toLeaseRow(lease)
		return writeJSONReport(out, errOut, "council claim", claimReport{
			Granted:    true,
			LocalAgent: string(level),
			Resource:   resource,
			Mode:       string(lease.Access),
			Owner:      who,
			Lease:      &row,
		}, exitOK)
	}
	fmt.Fprint(out, text("cli.claim.granted", `已获得租约
  资源    %s
  模式    %s
  持有者  %s/%s
  lease   %s
  代数    %d
  到期    %s（%s 之后）
交还：council release %s。租约只写库，不扫描进程、不调用任何模型。
`, resource, lease.Access, lease.OwnerKind, lease.OwnerID, lease.ID,
		lease.Generation, formatWhoTime(lease.ExpiresAt),
		lease.ExpiresAt.Sub(lease.GrantedAt).Round(time.Second), lease.ID))
	return exitOK
}

// runRelease 是 `council release` 的进程边界。
func runRelease(args []string) int {
	return runReleaseWith(args, leaseDeps{Open: store.Open}, os.Stdout, os.Stderr)
}

// runReleaseWith 是可测试的那一半。
//
// 交还**不看** local_agent 层级：关小开关不该让一条已经发出去的授权无法
// 收回。它仍然要拿 council.lock，因为租约在同一张库里。
func runReleaseWith(args []string, deps leaseDeps, out, errOut io.Writer) int {
	lang := applyLanguageFlag(prescanFlag(args, "lang"), prescanFlag(args, "config"))
	text := whoTextFunc(lang)

	fs := flag.NewFlagSet("council release", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", text("cli.release.flag.config",
		"配置文件路径，优先于环境变量 COUNCIL_CONFIG 与默认的 ~/.council/config.toml"))
	langFlag := fs.String("lang", "auto", text("cli.release.flag.lang",
		"输出语言：zh / en / auto（默认 auto：依次看 COUNCIL_LANG、配置里的 language、系统区域）"))
	asJSON := fs.Bool("json", false, text("cli.release.flag.json",
		"按 JSON 输出（released / resource / notice）"))
	leaseID := fs.String("lease", "", text("cli.release.flag.lease",
		"要交还的 lease_id（council claim 的输出里有）"))
	resourceArg := fs.String("resource", "", text("cli.release.flag.resource",
		"按资源交还自己在这个资源上的全部租约（需要 -owner 匹配，默认 cli@主机名）"))
	owner := fs.String("owner", "", text("cli.release.flag.owner",
		"持有者名字，与 claim 时给的一致（默认 cli@主机名）；只释放名字完全相同的租约"))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(errOut, releaseUsageLang(lang))
			return exitOK
		}
		fmt.Fprintf(errOut, "council release: %v\n", err)
		fmt.Fprint(errOut, releaseUsageLang(lang))
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprint(errOut, text("cli.release.error.positional",
			"council release 不接受位置参数，用 -lease 或 -resource 指定目标；多出来的是: %v\n", fs.Args()))
		fmt.Fprint(errOut, releaseUsageLang(lang))
		return exitUsage
	}
	byID := strings.TrimSpace(*leaseID)
	byResource := strings.TrimSpace(*resourceArg)
	switch {
	case byID == "" && byResource == "":
		fmt.Fprint(errOut, text("cli.release.error.target",
			"council release 需要一个目标：-lease <lease_id> 或 -resource <目录|tcp:端口>\n"))
		fmt.Fprint(errOut, releaseUsageLang(lang))
		return exitUsage
	case byID != "" && byResource != "":
		fmt.Fprint(errOut, text("cli.release.error.both",
			"council release 只接受一个目标：-lease 与 -resource 不能同时给\n"))
		fmt.Fprint(errOut, releaseUsageLang(lang))
		return exitUsage
	}

	who := strings.TrimSpace(*owner)
	if who == "" {
		who = defaultLeaseOwner()
	}
	resource := ""
	if byResource != "" {
		var err error
		resource, err = localagent.NormalizeResource(byResource)
		if err != nil {
			fmt.Fprintf(errOut, "council release: %v\n", err)
			return exitUsage
		}
	}

	lang = applyLanguageFlag(*langFlag, *configPath)
	text = whoTextFunc(lang)

	level, dataDir, err := loadWhoConfig(*configPath)
	if err != nil {
		fmt.Fprint(errOut, text("cli.lease.configBroken", "%s: 配置文件有问题: %v\n", "council release", err))
		return exitConfigFailed
	}

	ctx, cancel := context.WithTimeout(context.Background(), leaseTimeout)
	defer cancel()

	st, openErr := deps.Open(ctx, dataDir)
	if openErr != nil {
		return reportLeaseOpenFailure(errOut, text, "council release", dataDir, openErr)
	}
	defer func() {
		_ = st.Close()
	}()

	released := make([]store.Lease, 0, 1)
	if byID != "" {
		lease, changed, err := st.ReleaseLease(ctx, byID)
		if errors.Is(err, store.ErrLeaseNotFound) {
			notice := text("cli.release.notFound", "没有这条租约：%s（它可能已经被删掉，或 lease_id 抄错了）", byID)
			if *asJSON {
				return writeJSONReport(out, errOut, "council release",
					releaseReport{LocalAgent: string(level), Released: []leaseRow{}, Notice: notice},
					exitLeaseDenied)
			}
			fmt.Fprintln(errOut, notice)
			return exitLeaseDenied
		}
		if err != nil {
			fmt.Fprint(errOut, text("cli.release.failed", "council release: %v\n", err))
			return exitConfigFailed
		}
		if changed {
			released = append(released, lease)
		} else {
			// 已经是 released / expired：重试是正常行为，不报错。
			notice := text("cli.release.already", "租约 %s 的状态是 %s，不需要再交还（没有改动，也没有写事件）。",
				lease.ID, lease.State)
			if *asJSON {
				return writeJSONReport(out, errOut, "council release",
					releaseReport{LocalAgent: string(level), Released: []leaseRow{}, Notice: notice}, exitOK)
			}
			fmt.Fprintln(out, notice)
			return exitOK
		}
	} else {
		batch, err := st.ReleaseLeasesByOwner(ctx, resource, who)
		if err != nil {
			fmt.Fprint(errOut, text("cli.release.failed", "council release: %v\n", err))
			return exitConfigFailed
		}
		released = append(released, batch...)
	}

	if len(released) == 0 {
		notice := text("cli.release.nothing",
			"这个资源上没有 %s 的租约可交还（只释放 owner_id 完全相同的租约，没有 force）。", who)
		if *asJSON {
			return writeJSONReport(out, errOut, "council release",
				releaseReport{LocalAgent: string(level), Resource: resource, Released: []leaseRow{}, Notice: notice},
				exitLeaseDenied)
		}
		fmt.Fprintln(errOut, notice)
		return exitLeaseDenied
	}

	rows := make([]leaseRow, 0, len(released))
	for _, lease := range released {
		rows = append(rows, toLeaseRow(lease))
	}
	if *asJSON {
		return writeJSONReport(out, errOut, "council release", releaseReport{
			LocalAgent: string(level),
			Resource:   resource,
			Released:   rows,
		}, exitOK)
	}
	fmt.Fprint(out, text("cli.release.released", "已交还 %d 条租约\n", len(rows)))
	for _, row := range rows {
		fmt.Fprintf(out, "  %s  %s  %s/%s  代数 %d\n",
			row.LeaseID, row.ResourceKey, row.OwnerKind, row.OwnerID, row.Generation)
	}
	return exitOK
}

// reportLeaseOpenFailure 把开库失败翻译成一句能行动的话。
//
// 最常见的失败是 council.lock 被占：租约与主库同一份文件，而本仓库的规则是
// 一个 data_dir 同时只能一个写进程（C-1）。这里必须说清「谁在占、去哪停」，
// 否则用户只会看到一句 database is locked 的英文。
func reportLeaseOpenFailure(errOut io.Writer,
	text func(key, fallback string, args ...any) string,
	command, dataDir string, err error) int {
	if errors.Is(err, store.ErrDataDirLocked) {
		fmt.Fprint(errOut, text("cli.lease.locked", `%s: 数据目录正被另一个 council 进程占用（council.lock）。
租约与主库在同一个 council.db 里，写它必须先拿到那把锁；本命令不会绕过锁另开一个库。
  数据目录  %s
先停下 serve -rw（含桌面壳自己拉起的那个）、TUI 或 council mcp，再重跑本命令。
`, command, dataDir))
		return exitConfigFailed
	}
	fmt.Fprint(errOut, text("cli.lease.openFailed", "%s: 打不开本地库（数据目录 %s）: %v\n",
		command, dataDir, err))
	return exitConfigFailed
}

// writeJSONReport 输出一行 JSON，返回给定退出码。编码失败按 1 处理。
func writeJSONReport(out, errOut io.Writer, command string, report any, code int) int {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", command, err)
		return exitConfigFailed
	}
	return code
}

// claimUsageLang 是 -h 与用法错误共用的那一段。
func claimUsageLang(lang i18n.Lang) string {
	return i18n.TF(lang, "cli.claim.usage", `用法: council claim <目录|tcp:端口> [flags]

在 council.db（schema v10）里申请一条资源租约。租约是**自愿遵守的提醒**，
不是文件锁：它不阻止任何进程写文件，只是让下一个 agent 知道这个资源有人占着。
本命令零模型调用、零进程扫描，只读写两张表。

开启方式：config 里 local_agent = "coordinate"（设置页第 6 类也能改）。
写库要拿 council.lock，所以 serve -rw / TUI / council mcp 正在跑时会明确拒绝。

退出码: 0 拿到租约；3 被拒（被占用 / 层级未开启 / 没有这条租约）；1 出错；2 用法错误。

flags:
  -mode read|write     租约模式，默认 write（独占）；read 可并存
  -ttl <duration>      有效期，例如 5m / 1h（默认 2m，最短 5s，最长 24h）
  -owner <name>        持有者名字，默认 cli@主机名（只是展示与交还匹配，不参与鉴权）
  -json                按 JSON 输出（granted / resource / mode / lease / holder / notice）
  -config <path>       配置文件路径，与全局 -config 含义相同
  -lang zh|en|auto     输出语言，默认 auto
`)
}

// releaseUsageLang 是 -h 与用法错误共用的那一段。
func releaseUsageLang(lang i18n.Lang) string {
	return i18n.TF(lang, "cli.release.usage", `用法: council release -lease <lease_id>
       council release -resource <目录|tcp:端口> [-owner <name>]

交还自己申请的租约（state 改成 released 并留一条事件）。交还**不需要**
local_agent = coordinate：关小开关不该让已经发出去的授权无法收回。

只释放 owner_id 完全相同的租约，没有 force 参数 —— 释放别人的租约不在本模块
的能力里。写库同样要拿 council.lock。

退出码: 0 交还成功（或本来就已交还）；3 没找到可交还的租约；1 出错；2 用法错误。

flags:
  -lease <id>          要交还的 lease_id（council claim 的输出里有）
  -resource <dir|port> 按资源交还自己在这个资源上的全部租约
  -owner <name>        持有者名字，默认 cli@主机名
  -json                按 JSON 输出（released / resource / notice）
  -config <path>       配置文件路径，与全局 -config 含义相同
  -lang zh|en|auto     输出语言，默认 auto
`)
}
